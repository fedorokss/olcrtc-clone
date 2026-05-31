package vp8channel

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"hash/fnv"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/fedorokss/olcrtc-clone/internal/engine"
	enginebuiltin "github.com/fedorokss/olcrtc-clone/internal/engine/builtin"
	"github.com/fedorokss/olcrtc-clone/internal/logger"
	"github.com/fedorokss/olcrtc-clone/internal/transport"
	"github.com/fedorokss/olcrtc-clone/internal/transport/common"
	"github.com/pion/rtp"
	"github.com/pion/rtp/codecs"
	"github.com/pion/webrtc/v4"
	"github.com/pion/webrtc/v4/pkg/media"
)

const (
	defaultMaxPayloadSize = 60 * 1024
	defaultConnectTimeout = 60 * time.Second
	rtpBufSize            = 65536
	outboundQueueSize     = 2048
	inboundQueueSize      = 8192
	canSendHighWatermark  = 90
	keepaliveIdlePeriod   = 100 * time.Millisecond
)

var (
	ErrVideoTrackUnsupported = errors.New("carrier does not support video tracks")
	ErrTransportClosed       = errors.New("vp8channel transport closed")
)

var vp8Keepalive = []byte{ //nolint:gochecknoglobals
	0x30, 0x01, 0x00, 0x9d, 0x01, 0x2a, 0x10, 0x00,
	0x10, 0x00, 0x00, 0x47, 0x08, 0x85, 0x85, 0x88,
	0x99, 0x84, 0x88, 0xfc,
}

const (
	tokenOff    = 20
	epochOff    = 24
	crcOff      = 28
	epochHdrLen = 32
)

var kcpBatchMagic = [4]byte{'O', 'L', 'K', 'B'} //nolint:gochecknoglobals

type videoSession interface {
	Connect(ctx context.Context) error
	Close() error
	SetReconnectCallback(cb func())
	SetShouldReconnect(fn func() bool)
	SetEndedCallback(cb func(string))
	WatchConnection(ctx context.Context)
	CanSend() bool
	Reconnect(reason string)
	AddTrack(track webrtc.TrackLocal) error
	SetTrackHandler(cb func(*webrtc.TrackRemote, *webrtc.RTPReceiver))
}

type streamTransport struct {
	stream        videoSession
	track         *webrtc.TrackLocalStaticSample
	onData        func([]byte)
	onPeerData    func(peerID string, data []byte)
	outbound      chan []byte
	closeCh       chan struct{}
	writerDone    chan struct{}
	closed        atomic.Bool
	writerUp      atomic.Bool
	writerOnce    sync.Once
	kcpOnce       sync.Once
	frameInterval time.Duration
	batchSize     int
	perTickBytes  int
	batchScratch  []byte

	bindingToken uint32
	epochMu      sync.RWMutex
	localEpoch   uint32
	headerCache  atomic.Pointer[[epochHdrLen]byte]
	peerEpoch    atomic.Uint32

	kcp           *kcpRuntime
	kcpMu         sync.RWMutex
	reconnectMu   sync.Mutex
	reconnectFn   func()
	peerConfirmed atomic.Bool

	peersMu sync.RWMutex
	peers   map[uint32]*kcpRuntime
	peerOut map[uint32]chan []byte
}

func New(ctx context.Context, cfg transport.Config) (transport.Transport, error) {
	opts, err := optionsFrom(cfg)
	if err != nil {
		return nil, err
	}
	session, err := enginebuiltin.Open(ctx, cfg.Carrier, enginebuiltin.Config{
		RoomURL:   cfg.RoomURL,
		Name:      cfg.Name,
		OnData:    nil,
		DNSServer: cfg.DNSServer,
		ProxyAddr: cfg.ProxyAddr,
		ProxyPort: cfg.ProxyPort,
		Engine:    cfg.Engine,
		URL:       cfg.URL,
		Token:     cfg.Token,
	})
	if err != nil {
		return nil, fmt.Errorf("open engine session: %w", err)
	}
	vt, ok := session.(engine.VideoTrackCapable)
	if !ok || !session.Capabilities().VideoTrack {
		_ = session.Close()
		return nil, ErrVideoTrackUnsupported
	}
	stream := &engineVideoSession{session: session, vt: vt}
	track, err := webrtc.NewTrackLocalStaticSample(
		webrtc.RTPCodecCapability{
			MimeType:  webrtc.MimeTypeVP8,
			ClockRate: 90000,
		},
		"vp8channel-"+common.RandomID(),
		"olcrtc-"+common.RandomID(),
	)
	if err != nil {
		return nil, fmt.Errorf("create local video track: %w", err)
	}
	tr := newStreamTransport(stream, track, cfg, opts)
	if err := stream.AddTrack(track); err != nil {
		return nil, fmt.Errorf("attach local video track: %w", err)
	}
	stream.SetTrackHandler(tr.handleRemoteTrack)
	return tr, nil
}

func newStreamTransport(
	stream *engineVideoSession,
	track *webrtc.TrackLocalStaticSample,
	cfg transport.Config,
	opts Options,
) *streamTransport {
	fps := opts.FPS
	batchSize := opts.BatchSize
	if fps <= 0 {
		fps = defaultFPS
	}
	if batchSize <= 0 {
		batchSize = defaultBatchSize
	}
	byteRate := opts.MaxBytesPerSec
	if byteRate <= 0 {
		byteRate = defaultMaxBytesPerSec
	}
	perTickBytes := byteRate / fps
	if perTickBytes < epochHdrLen {
		perTickBytes = epochHdrLen
	}
	tr := &streamTransport{
		stream:        stream,
		track:         track,
		onData:        cfg.OnData,
		onPeerData:    cfg.OnPeerData,
		outbound:      make(chan []byte, outboundQueueSize),
		closeCh:       make(chan struct{}),
		writerDone:    make(chan struct{}),
		frameInterval: time.Second / time.Duration(fps),
		batchSize:     batchSize,
		perTickBytes:  perTickBytes,
		batchScratch:  make([]byte, 0, defaultMaxPayloadSize),
		bindingToken:  bindingToken(cfg.RoomURL),
		localEpoch:    randomEpoch(),
		peers:         make(map[uint32]*kcpRuntime),
		peerOut:       make(map[uint32]chan []byte),
	}
	tr.refreshHeaderCache(tr.localEpoch)
	if cfg.OnData != nil && cfg.OnPeerData == nil {
		inner := cfg.OnData
		tr.onData = func(data []byte) {
			if !tr.peerConfirmed.Swap(true) {
				epoch := tr.peerEpoch.Load()
				logger.Infof("vp8channel: peer confirmed epoch=0x%08x", epoch)
			}
			inner(data)
		}
	} else {
		tr.onData = cfg.OnData
	}
	return tr
}

func (p *streamTransport) Connect(ctx context.Context) error {
	connectCtx, cancel := context.WithTimeout(ctx, defaultConnectTimeout)
	defer cancel()
	if err := p.stream.Connect(connectCtx); err != nil {
		return fmt.Errorf("connect stream: %w", err)
	}
	p.kcpOnce.Do(func() {
		rt, err := startKCP(p.outbound, p.onData, p.epochHeader())
		if err != nil {
			logger.Infof("vp8channel: startKCP failed: %v", err)
			return
		}
		p.kcpMu.Lock()
		p.kcp = rt
		p.kcpMu.Unlock()
		logger.Infof("vp8channel: KCP started localEpoch=0x%08x", p.localEpochValue())
	})
	p.writerOnce.Do(func() {
		p.writerUp.Store(true)
		go p.writerLoop()
	})
	return nil
}

func (p *streamTransport) epochHeader() [epochHdrLen]byte {
	if h := p.headerCache.Load(); h != nil {
		return *h
	}
	return buildEpochHeader(p.bindingToken, p.localEpochValue())
}

func (p *streamTransport) refreshHeaderCache(epoch uint32) [epochHdrLen]byte {
	hdr := buildEpochHeader(p.bindingToken, epoch)
	cached := hdr
	p.headerCache.Store(&cached)
	return hdr
}

func buildEpochHeader(token, epoch uint32) [epochHdrLen]byte {
	var hdr [epochHdrLen]byte
	copy(hdr[:], vp8Keepalive)
	binary.BigEndian.PutUint32(hdr[tokenOff:epochOff], token)
	binary.BigEndian.PutUint32(hdr[epochOff:crcOff], epoch)
	binary.BigEndian.PutUint32(hdr[crcOff:epochHdrLen], epochCRC(token, epoch))
	return hdr
}

func (p *streamTransport) rotateEpochHeader() [epochHdrLen]byte {
	p.epochMu.Lock()
	for {
		next := randomEpoch()
		if next != p.localEpoch {
			p.localEpoch = next
			break
		}
	}
	epoch := p.localEpoch
	p.epochMu.Unlock()
	return p.refreshHeaderCache(epoch)
}

func (p *streamTransport) localEpochValue() uint32 {
	p.epochMu.RLock()
	defer p.epochMu.RUnlock()
	return p.localEpoch
}

func epochCRC(token, epoch uint32) uint32 {
	var buf [8]byte
	binary.BigEndian.PutUint32(buf[0:4], token)
	binary.BigEndian.PutUint32(buf[4:8], epoch)
	return crc32.ChecksumIEEE(buf[:])
}

func parseEpochHeader(frame []byte) (uint32, uint32, bool) {
	if len(frame) < epochHdrLen {
		return 0, 0, false
	}
	token := binary.BigEndian.Uint32(frame[tokenOff:epochOff])
	epoch := binary.BigEndian.Uint32(frame[epochOff:crcOff])
	gotCRC := binary.BigEndian.Uint32(frame[crcOff:epochHdrLen])
	return token, epoch, gotCRC == epochCRC(token, epoch)
}

func bindingToken(clientID string) uint32 {
	h := fnv.New32a()
	_, _ = h.Write([]byte(clientID))
	token := h.Sum32()
	if token == 0 {
		token = 1
	}
	return token
}

func randomEpoch() uint32 {
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		return uint32(time.Now().UnixNano()) //nolint:gosec
	}
	e := binary.BigEndian.Uint32(b[:])
	if e == 0 {
		e = 1
	}
	return e
}

func (p *streamTransport) Send(data []byte) error {
	if p.closed.Load() {
		return ErrTransportClosed
	}
	p.kcpMu.RLock()
	rt := p.kcp
	p.kcpMu.RUnlock()
	if rt == nil {
		return ErrTransportClosed
	}
	return rt.send(data)
}

func (p *streamTransport) SendTo(peerID string, data []byte) error {
	if p.closed.Load() {
		return ErrTransportClosed
	}
	epoch, err := parsePeerID(peerID)
	if err != nil {
		return fmt.Errorf("vp8channel: invalid peerID %q: %w", peerID, err)
	}
	p.peersMu.RLock()
	rt := p.peers[epoch]
	p.peersMu.RUnlock()
	if rt == nil {
		return ErrTransportClosed
	}
	return rt.send(data)
}

func (p *streamTransport) SupportsPeerRouting() bool {
	return p.onPeerData != nil
}

func (p *streamTransport) Close() error {
	if p.closed.CompareAndSwap(false, true) {
		close(p.closeCh)
		p.kcpMu.RLock()
		rt := p.kcp
		p.kcpMu.RUnlock()
		if rt != nil {
			rt.close()
		}
		p.peersMu.Lock()
		for _, prt := range p.peers {
			prt.close()
		}
		p.peers = make(map[uint32]*kcpRuntime)
		p.peerOut = make(map[uint32]chan []byte)
		p.peersMu.Unlock()
		if p.writerUp.Load() {
			<-p.writerDone
		}
		if err := p.stream.Close(); err != nil {
			return fmt.Errorf("close stream: %w", err)
		}
	}
	return nil
}

func (p *streamTransport) drainOutbound() {
	for {
		select {
		case <-p.outbound:
		default:
			return
		}
	}
}

func (p *streamTransport) ResetPeer() {
	p.peerConfirmed.Store(false)
	p.peerEpoch.Store(0)
	p.restartKCP(p.rotateEpochHeader())
}

func (p *streamTransport) Reconnect(reason string) {
	p.stream.Reconnect(reason)
}

func (p *streamTransport) SetReconnectCallback(cb func()) {
	p.reconnectMu.Lock()
	p.reconnectFn = cb
	p.reconnectMu.Unlock()
	p.stream.SetReconnectCallback(func() {
		p.resetKCP()
		if cb != nil {
			cb()
		}
	})
}

func (p *streamTransport) SetShouldReconnect(fn func() bool) {
	p.stream.SetShouldReconnect(fn)
}

func (p *streamTransport) SetEndedCallback(cb func(string)) {
	p.stream.SetEndedCallback(cb)
}

func (p *streamTransport) WatchConnection(ctx context.Context) {
	p.stream.WatchConnection(ctx)
}

func (p *streamTransport) CanSend() bool {
	if p.closed.Load() {
		return false
	}
	p.kcpMu.RLock()
	hasKCP := p.kcp != nil
	p.kcpMu.RUnlock()
	return hasKCP && p.stream.CanSend() &&
		len(p.outbound) < cap(p.outbound)*canSendHighWatermark/100
}

func (p *streamTransport) Features() transport.Features {
	return transport.Features{
		Reliable:        true,
		Ordered:         true,
		MessageOriented: true,
		MaxPayloadSize:  defaultMaxPayloadSize,
	}
}

func (p *streamTransport) writerLoop() {
	defer close(p.writerDone)
	ticker := time.NewTicker(p.frameInterval)
	defer ticker.Stop()
	keepaliveEvery := max(int(keepaliveIdlePeriod/p.frameInterval), 1)
	idleTicks := 0
	for {
		select {
		case <-p.closeCh:
			return
		case <-ticker.C:
			var sample []byte
			select {
			case frame := <-p.outbound:
				sample = p.batchSample(frame, p.perTickBytes)
				idleTicks = 0
			default:
				idleTicks++
				if idleTicks < keepaliveEvery {
					continue
				}
				idleTicks = 0
				hdr := p.epochHeader()
				sample = hdr[:]
			}
			_ = p.track.WriteSample(media.Sample{
				Data:     sample,
				Duration: p.frameInterval,
			})
		}
	}
}

func (p *streamTransport) batchSample(first []byte, maxBytes int) []byte {
	if maxBytes <= 0 || maxBytes > defaultMaxPayloadSize {
		maxBytes = defaultMaxPayloadSize
	}
	if len(first) <= epochHdrLen || p.batchSize <= 1 {
		return first
	}
	sample := p.batchScratch[:0]
	sample = append(sample, first[:epochHdrLen]...)
	sample = append(sample, kcpBatchMagic[:]...)
	sample = appendBatchPacket(sample, first[epochHdrLen:])
	for packets := 1; packets < p.batchSize; packets++ {
		select {
		case frame := <-p.outbound:
			if len(frame) <= epochHdrLen {
				continue
			}
			payload := frame[epochHdrLen:]
			if len(sample)+2+len(payload) > maxBytes {
				p.batchScratch = sample
				return sample
			}
			sample = appendBatchPacket(sample, payload)
		default:
			p.batchScratch = sample
			return sample
		}
	}
	p.batchScratch = sample
	return sample
}

func appendBatchPacket(dst, packet []byte) []byte {
	if len(packet) > 0xffff {
		return dst
	}
	var lenBuf [2]byte
	binary.BigEndian.PutUint16(lenBuf[:], uint16(len(packet))) //nolint:gosec
	dst = append(dst, lenBuf[:]...)
	return append(dst, packet...)
}

func (p *streamTransport) resetKCP() {
	p.peerConfirmed.Store(false)
	p.peerEpoch.Store(0)
	p.restartKCP(p.rotateEpochHeader())
}

func (p *streamTransport) restartKCP(epochHdr [epochHdrLen]byte) {
	p.drainOutbound()
	p.kcpMu.Lock()
	old := p.kcp
	p.kcp = nil
	p.kcpMu.Unlock()
	if old != nil {
		old.close()
	}
	rt, err := startKCP(p.outbound, p.onData, epochHdr)
	if err != nil {
		return
	}
	p.kcpMu.Lock()
	p.kcp = rt
	p.kcpMu.Unlock()
}

func (p *streamTransport) handleRemoteTrack(track *webrtc.TrackRemote, _ *webrtc.RTPReceiver) {
	if track.Codec().MimeType != webrtc.MimeTypeVP8 {
		go p.drainTrack(track)
		return
	}
	go p.readVP8Track(track)
}

func (p *streamTransport) drainTrack(track *webrtc.TrackRemote) {
	buf := make([]byte, rtpBufSize)
	for {
		if _, _, err := track.Read(buf); err != nil {
			return
		}
	}
}

type vp8FrameState struct {
	vp8Pkt      codecs.VP8Packet
	frameBuf    []byte
	lastSeq     uint16
	haveLastSeq bool
	frameValid  bool
}

func (s *vp8FrameState) processRTPPacket(pkt *rtp.Packet) []byte {
	if s.haveLastSeq && pkt.SequenceNumber != s.lastSeq+1 {
		s.frameValid = false
		s.frameBuf = s.frameBuf[:0]
	}
	s.lastSeq = pkt.SequenceNumber
	s.haveLastSeq = true
	vp8Payload, err := s.vp8Pkt.Unmarshal(pkt.Payload)
	if err != nil {
		s.frameValid = false
		s.frameBuf = s.frameBuf[:0]
		return nil
	}
	if s.vp8Pkt.S == 1 {
		s.frameBuf = s.frameBuf[:0]
		s.frameValid = true
	}
	if !s.frameValid {
		return nil
	}
	s.frameBuf = append(s.frameBuf, vp8Payload...)
	if !pkt.Marker {
		return nil
	}
	defer func() {
		s.frameBuf = s.frameBuf[:0]
		s.frameValid = false
	}()
	if len(s.frameBuf) >= epochHdrLen {
		frame := make([]byte, len(s.frameBuf))
		copy(frame, s.frameBuf)
		return frame
	}
	return nil
}

func (p *streamTransport) readVP8Track(track *webrtc.TrackRemote) {
	var state vp8FrameState
	var pkt rtp.Packet
	buf := make([]byte, rtpBufSize)
	for {
		n, _, err := track.Read(buf)
		if err != nil {
			return
		}
		if pkt.Unmarshal(buf[:n]) != nil {
			continue
		}
		frame := state.processRTPPacket(&pkt)
		if frame == nil {
			continue
		}
		p.handleIncomingFrame(frame)
	}
}

func (p *streamTransport) handleFirstPeer(peerEpoch uint32) {
	p.peerEpoch.Store(peerEpoch)
	p.peerConfirmed.Store(true)
	logger.Infof("vp8channel: peer latched epoch=0x%08x", peerEpoch)
}

func (p *streamTransport) handleIncomingFrame(frame []byte) {
	frameToken, peerEpoch, ok := parseEpochHeader(frame)
	if !ok {
		return
	}
	if frameToken != p.bindingToken {
		return
	}
	kcpPayload := frame[epochHdrLen:]
	if peerEpoch == p.localEpochValue() {
		return
	}
	if p.onPeerData != nil {
		p.handlePeerFrame(peerEpoch, kcpPayload)
		return
	}
	if !p.peerConfirmed.Load() {
		p.handleFirstPeer(peerEpoch)
	} else if peerEpoch != p.peerEpoch.Load() {
		return
	}
	if len(kcpPayload) == 0 {
		return
	}
	p.kcpMu.RLock()
	rt := p.kcp
	p.kcpMu.RUnlock()
	if rt != nil {
		deliverKCPPayload(rt, kcpPayload)
	}
}

func (p *streamTransport) handlePeerFrame(peerEpoch uint32, kcpPayload []byte) {
	if len(kcpPayload) == 0 {
		p.getOrCreatePeerKCP(peerEpoch)
		return
	}
	rt := p.getOrCreatePeerKCP(peerEpoch)
	if rt != nil {
		deliverKCPPayload(rt, kcpPayload)
	}
}

func (p *streamTransport) getOrCreatePeerKCP(epoch uint32) *kcpRuntime {
	p.peersMu.RLock()
	rt := p.peers[epoch]
	p.peersMu.RUnlock()
	if rt != nil {
		return rt
	}
	p.peersMu.Lock()
	defer p.peersMu.Unlock()
	if rt = p.peers[epoch]; rt != nil {
		return rt
	}
	peerID := formatPeerID(epoch)
	out := make(chan []byte, outboundQueueSize)
	hdr := buildEpochHeader(p.bindingToken, p.localEpochValue())
	rt, err := startKCP(out, func(data []byte) {
		if p.onPeerData != nil {
			p.onPeerData(peerID, data)
		}
	}, hdr)
	if err != nil {
		logger.Warnf("vp8channel: startKCP for peer 0x%08x failed: %v", epoch, err)
		return nil
	}
	p.peers[epoch] = rt
	p.peerOut[epoch] = out
	logger.Infof("vp8channel: peer session created epoch=0x%08x", epoch)
	go p.peerWriterPump(epoch, out)
	return rt
}

func (p *streamTransport) peerWriterPump(_ uint32, out chan []byte) {
	for {
		select {
		case <-p.closeCh:
			return
		case frame, ok := <-out:
			if !ok {
				return
			}
			_ = p.track.WriteSample(media.Sample{
				Data:     frame,
				Duration: p.frameInterval,
			})
		}
	}
}

func formatPeerID(epoch uint32) string {
	return fmt.Sprintf("%08x", epoch)
}

func parsePeerID(peerID string) (uint32, error) {
	v, err := strconv.ParseUint(peerID, 16, 32)
	if err != nil {
		return 0, fmt.Errorf("parse peer ID %q: %w", peerID, err)
	}
	return uint32(v), nil
}

func deliverKCPPayload(rt *kcpRuntime, payload []byte) {
	if rt == nil || len(payload) == 0 {
		return
	}
	splitKCPPayload(payload, rt.deliver)
}

func splitKCPPayload(payload []byte, deliver func([]byte)) {
	m := len(kcpBatchMagic)
	if len(payload) < m ||
		payload[0] != kcpBatchMagic[0] ||
		payload[1] != kcpBatchMagic[1] ||
		payload[2] != kcpBatchMagic[2] ||
		payload[3] != kcpBatchMagic[3] {
		deliver(payload)
		return
	}
	rest := payload[m:]
	for len(rest) > 0 {
		if len(rest) < 2 {
			return
		}
		size := int(binary.BigEndian.Uint16(rest[:2]))
		rest = rest[2:]
		if size == 0 || len(rest) < size {
			return
		}
		deliver(rest[:size])
		rest = rest[size:]
	}
}
