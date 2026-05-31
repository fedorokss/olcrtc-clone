package jitsi

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"encoding/xml"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/fedorokss/olcrtc-clone/internal/engine"
	"github.com/fedorokss/olcrtc-clone/internal/logger"
	pioninterceptor "github.com/pion/interceptor"
	"github.com/pion/rtcp"
	"github.com/pion/webrtc/v4"
	"github.com/zarazaex69/j"
)

const (
	defaultSendQueueSize = 5000
	bridgeMaxMessageSize = 16 * 1024
	bridgeOpenTimeout    = 30 * time.Second
	defaultNick          = "olcrtc"
	credentialKeyRoom    = "room"
	videoTrackName       = "videochannel"
	maxReconnects        = 5
	reconnectWindow      = 5 * time.Minute
	bridgeMagicLen       = 4
	epochHeaderLen       = 8
	frameHeaderLen       = bridgeMagicLen + epochHeaderLen
)

var bridgeMagic = [bridgeMagicLen]byte{'O', 'L', 'R', '1'} //nolint:gochecknoglobals
var fallbackEpoch atomic.Uint32                            //nolint:gochecknoglobals

var (
	ErrSessionClosed  = errors.New("jitsi session closed")
	ErrSendQueueFull  = errors.New("jitsi send queue full")
	ErrBridgeNotReady = errors.New("jitsi bridge not ready")
	ErrSendTooLarge   = errors.New("jitsi payload exceeds bridge max-message-size")
	ErrHostRequired   = errors.New("jitsi host required")
	ErrRoomRequired   = errors.New("jitsi room required")
)

type Session struct {
	host string
	room string
	name string

	onData          func([]byte)
	onPeerData      func(peerID string, data []byte)
	onReconnect     func(*webrtc.DataChannel)
	shouldReconnect func() bool
	onEnded         func(string)

	jSess atomic.Pointer[j.Session]

	pcMu sync.Mutex
	pc   *webrtc.PeerConnection

	sendQueue     chan []byte
	peerSendQueue chan bridgeOutbound
	bridgeReady   atomic.Bool
	closed        atomic.Bool
	reconnecting  atomic.Bool

	reconnectCh          chan struct{}
	reconnectMu          sync.Mutex
	reconnectWindowStart time.Time
	reconnectCount       int

	localEpoch atomic.Uint32
	peerEpoch  atomic.Uint32

	peerEndpoint atomic.Pointer[string]
	peerEpochMu  sync.Mutex
	peerEpochs   map[string]uint32

	done          chan struct{}
	doneOnce      sync.Once
	cancel        context.CancelFunc
	trickleCancel context.CancelFunc
	runCtx        context.Context //nolint:containedctx
	wg            sync.WaitGroup

	videoTrackMu sync.RWMutex
	videoTracks  []webrtc.TrackLocal
	onVideoTrack func(*webrtc.TrackRemote, *webrtc.RTPReceiver)

	peerVideoSSRC atomic.Uint32
}

type bridgeOutbound struct {
	to   string
	data []byte
}

func New(_ context.Context, cfg engine.Config) (engine.Session, error) {
	host := normaliseHost(cfg.URL)
	if host == "" {
		return nil, ErrHostRequired
	}
	var room string
	if cfg.Extra != nil {
		room = strings.TrimSpace(cfg.Extra[credentialKeyRoom])
	}
	if room == "" {
		return nil, ErrRoomRequired
	}
	name := sanitiseNick(cfg.Name)
	if name == "" {
		name = defaultNick
	}
	runCtx, cancel := context.WithCancel(context.Background())
	s := &Session{
		host:          host,
		room:          room,
		name:          name,
		onData:        cfg.OnData,
		onPeerData:    cfg.OnPeerData,
		sendQueue:     make(chan []byte, defaultSendQueueSize),
		peerSendQueue: make(chan bridgeOutbound, defaultSendQueueSize),
		peerEpochs:    make(map[string]uint32),
		reconnectCh:   make(chan struct{}, 1),
		done:          make(chan struct{}),
		cancel:        cancel,
		runCtx:        runCtx,
	}
	s.localEpoch.Store(randomEpoch())
	return s, nil
}

var cyrillicToLatin = map[rune]string{ //nolint:gochecknoglobals
	'Р С’': "A", 'Р В°': "a", 'Р вЂ': "B", 'Р В±': "b", 'Р вЂ™': "V", 'Р Р†': "v",
	'Р вЂњ': "G", 'Р С–': "g", 'Р вЂќ': "D", 'Р Т‘': "d", 'Р вЂў': "E", 'Р Вµ': "e",
	'Р Рѓ': "Yo", 'РЎвЂ': "yo", 'Р вЂ“': "Zh", 'Р В¶': "zh", 'Р вЂ”': "Z", 'Р В·': "z",
	'Р В': "I", 'Р С‘': "i", 'Р в„ў': "Y", 'Р в„–': "y", 'Р С™': "K", 'Р С”': "k",
	'Р вЂє': "L", 'Р В»': "l", 'Р Сљ': "M", 'Р С': "m", 'Р Сњ': "N", 'Р Р…': "n",
	'Р С›': "O", 'Р С•': "o", 'Р Сџ': "P", 'Р С—': "p", 'Р В ': "R", 'РЎР‚': "r",
	'Р РЋ': "S", 'РЎРѓ': "s", 'Р Сћ': "T", 'РЎвЂљ': "t", 'Р Р€': "U", 'РЎС“': "u",
	'Р В¤': "F", 'РЎвЂћ': "f", 'Р Тђ': "Kh", 'РЎвЂ¦': "kh", 'Р В¦': "Ts", 'РЎвЂ ': "ts",
	'Р В§': "Ch", 'РЎвЂЎ': "ch", 'Р РЃ': "Sh", 'РЎв‚¬': "sh", 'Р В©': "Shch", 'РЎвЂ°': "shch",
	'Р Р„': "", 'РЎР‰': "", 'Р В«': "Y", 'РЎвЂ№': "y", 'Р В¬': "", 'РЎРЉ': "",
	'Р В­': "E", 'РЎРЊ': "e", 'Р В®': "Yu", 'РЎР‹': "yu", 'Р Р‡': "Ya", 'РЎРЏ': "ya",
}

func sanitiseNick(raw string) string {
	const maxNickLen = 16
	var b strings.Builder
	b.Grow(min(len(raw), maxNickLen))
	prevDash := false
	for _, r := range raw {
		if b.Len() >= maxNickLen {
			break
		}
		if isNickRune(r) {
			b.WriteRune(r)
			prevDash = false
			continue
		}
		if lat, ok := cyrillicToLatin[r]; ok {
			if lat != "" {
				for i := 0; i < len(lat); i++ {
					if b.Len() >= maxNickLen {
						break
					}
					b.WriteByte(lat[i])
				}
				prevDash = false
			}
			continue
		}
		if !prevDash && b.Len() > 0 {
			b.WriteByte('-')
			prevDash = true
		}
	}
	return strings.Trim(b.String(), "-")
}

func isNickRune(r rune) bool {
	switch {
	case r >= 'a' && r <= 'z':
		return true
	case r >= 'A' && r <= 'Z':
		return true
	case r >= '0' && r <= '9':
		return true
	case r == '-' || r == '_':
		return true
	}
	return false
}

func randomEpoch() uint32 {
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		v := fallbackEpoch.Add(1)
		if v == 0 {
			return fallbackEpoch.Add(1)
		}
		return v
	}
	v := binary.BigEndian.Uint32(b[:])
	if v == 0 {
		return 1
	}
	return v
}

func (s *Session) Capabilities() engine.Capabilities {
	return engine.Capabilities{ByteStream: true, VideoTrack: true}
}

func (s *Session) Connect(ctx context.Context) error {
	if s.closed.Load() {
		return ErrSessionClosed
	}
	logger.Infof("jitsi: joining MUC %s/%s as %s РІР‚В¦", s.host, s.room, s.name)
	jSess, err := j.JoinMUC(ctx, j.Config{
		Host:  s.host,
		Room:  s.room,
		Nick:  s.name,
		Debug: logger.IsVerbose(),
	})
	if err != nil {
		return fmt.Errorf("jitsi join muc: %w", err)
	}
	s.jSess.Store(jSess)
	logger.Infof("jitsi: MUC joined %s/%s; waiting for peer РІР‚В¦", s.host, s.room)
	s.wg.Add(3)
	go s.sendLoop()
	go s.recvLoop()
	go s.waitForJingle()
	return nil
}

func (s *Session) waitForJingle() {
	defer s.wg.Done()
	jSess := s.jSess.Load()
	if jSess == nil {
		return
	}
	if _, err := jSess.Conn.WaitJingle(s.runCtx); err != nil {
		if s.closed.Load() || s.runCtx.Err() != nil {
			return
		}
		logger.Warnf("jitsi: wait jingle failed: %v", err)
		return
	}
	if err := s.completeJingleSetup(s.runCtx, jSess); err != nil {
		if !s.closed.Load() {
			logger.Warnf("jitsi: jingle setup failed: %v", err)
			s.requestReconnect("jingle setup failed")
		}
	}
}

func (s *Session) completeJingleSetup(ctx context.Context, jSess *j.Session) error {
	logger.Infof("jitsi: session-initiate received; colibri-ws=%s", jSess.ColibriWS)
	needBridge := s.onData != nil || s.onPeerData != nil
	sctpBridge := needBridge && jSess.ColibriWS == ""
	if needBridge && !sctpBridge {
		if err := s.openBridgeWS(ctx, jSess); err != nil {
			return err
		}
	}
	if s.shouldNegotiatePC() {
		if err := s.negotiatePC(ctx, jSess, sctpBridge); err != nil {
			return err
		}
	}
	if sctpBridge {
		if err := s.openBridgeSCTP(ctx, jSess); err != nil {
			return err
		}
	}
	s.wg.Add(1)
	go s.recvLoop()
	return nil
}

func (s *Session) joinAndOpenBridge(ctx context.Context) (*j.Session, error) { //nolint:cyclop
	logger.Infof("jitsi: joining %s/%s as %s РІР‚В¦", s.host, s.room, s.name)
	jSess, err := j.Join(ctx, j.Config{
		Host:  s.host,
		Room:  s.room,
		Nick:  s.name,
		Debug: logger.IsVerbose(),
	})
	if err != nil {
		return nil, fmt.Errorf("jitsi join: %w", err)
	}
	logger.Infof("jitsi: joined %s/%s; colibri-ws=%s", s.host, s.room, jSess.ColibriWS)
	needBridge := s.onData != nil || s.onPeerData != nil
	sctpBridge := needBridge && jSess.ColibriWS == ""
	if needBridge && !sctpBridge {
		if err := s.openBridgeWS(ctx, jSess); err != nil {
			_ = jSess.Close()
			return nil, err
		}
	}
	if s.shouldNegotiatePC() {
		if err := s.negotiatePC(ctx, jSess, sctpBridge); err != nil {
			_ = jSess.Close()
			return nil, err
		}
	}
	if sctpBridge {
		if err := s.openBridgeSCTP(ctx, jSess); err != nil {
			_ = jSess.Close()
			return nil, err
		}
	}
	return jSess, nil
}

func (s *Session) openBridgeWS(ctx context.Context, jSess *j.Session) error {
	bctx, bcancel := context.WithTimeout(ctx, bridgeOpenTimeout)
	err := jSess.OpenBridge(bctx)
	bcancel()
	if err != nil {
		return fmt.Errorf("open bridge: %w", err)
	}
	s.peerEndpoint.Store(nil)
	s.peerVideoSSRC.Store(0)
	s.bridgeReady.Store(true)
	logger.Infof("jitsi: bridge open colibri-ws (endpoints=%v)", jSess.Endpoints())
	return nil
}

func (s *Session) openBridgeSCTP(ctx context.Context, jSess *j.Session) error {
	bctx, bcancel := context.WithTimeout(ctx, bridgeOpenTimeout)
	err := jSess.WaitBridgeSCTP(bctx)
	bcancel()
	if err != nil {
		return fmt.Errorf("open bridge sctp: %w", err)
	}
	s.peerEndpoint.Store(nil)
	s.peerVideoSSRC.Store(0)
	s.bridgeReady.Store(true)
	logger.Infof("jitsi: bridge open sctp (endpoints=%v)", jSess.Endpoints())
	return nil
}

func (s *Session) shouldNegotiatePC() bool {
	if s.onData != nil || s.onPeerData != nil {
		return true
	}
	return s.shouldRequestVideo()
}

func (s *Session) shouldRequestVideo() bool {
	s.videoTrackMu.RLock()
	defer s.videoTrackMu.RUnlock()
	return len(s.videoTracks) > 0 || s.onVideoTrack != nil
}

func drainTrack(track *webrtc.TrackRemote) {
	buf := make([]byte, 1500)
	for {
		if _, _, err := track.Read(buf); err != nil {
			return
		}
	}
}

func (s *Session) videoTrackHandler() func(*webrtc.TrackRemote, *webrtc.RTPReceiver) {
	s.videoTrackMu.RLock()
	defer s.videoTrackMu.RUnlock()
	return s.onVideoTrack
}

//nolint:cyclop
func (s *Session) negotiatePC(ctx context.Context, jSess *j.Session, sctpBridge bool) error {
	settings := webrtc.SettingEngine{}
	settings.LoggerFactory = logger.NewPionLoggerFactory()
	registry := &pioninterceptor.Registry{}
	api := webrtc.NewAPI(
		webrtc.WithSettingEngine(settings),
		webrtc.WithInterceptorRegistry(registry),
	)
	pcConfig := jSess.IceConfig()
	pcConfig.SDPSemantics = webrtc.SDPSemanticsPlanB
	pc, err := api.NewPeerConnection(pcConfig)
	if err != nil {
		return fmt.Errorf("new pc: %w", err)
	}
	if _, err := pc.AddTransceiverFromKind(
		webrtc.RTPCodecTypeAudio,
		webrtc.RTPTransceiverInit{Direction: webrtc.RTPTransceiverDirectionRecvonly},
	); err != nil {
		_ = pc.Close()
		return fmt.Errorf("add audio recvonly: %w", err)
	}
	s.videoTrackMu.RLock()
	hasLocalTracks := len(s.videoTracks) > 0
	for _, track := range s.videoTracks {
		if _, addErr := pc.AddTrack(track); addErr != nil {
			s.videoTrackMu.RUnlock()
			_ = pc.Close()
			return fmt.Errorf("add track: %w", addErr)
		}
	}
	s.videoTrackMu.RUnlock()
	if !hasLocalTracks {
		if _, err := pc.AddTransceiverFromKind(
			webrtc.RTPCodecTypeVideo,
			webrtc.RTPTransceiverInit{Direction: webrtc.RTPTransceiverDirectionRecvonly},
		); err != nil {
			_ = pc.Close()
			return fmt.Errorf("add video recvonly: %w", err)
		}
	}
	pc.OnTrack(func(track *webrtc.TrackRemote, recv *webrtc.RTPReceiver) {
		if track.Kind() != webrtc.RTPCodecTypeVideo {
			return
		}
		ssrc := uint32(track.SSRC())
		if !s.peerVideoSSRC.CompareAndSwap(0, ssrc) && s.peerVideoSSRC.Load() != ssrc {
			go drainTrack(track)
			return
		}
		if cb := s.videoTrackHandler(); cb != nil {
			cb(track, recv)
		}
	})
	pc.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		logger.Debugf("jitsi pc state: %s", state.String())
		if state == webrtc.PeerConnectionStateFailed && !s.closed.Load() && s.onEnded != nil {
			s.onEnded("jitsi peer connection failed")
		}
	})
	neg := jSess.Negotiator()
	neg.PC = pc
	neg.OnIceConnectionStateChange = func(state webrtc.ICEConnectionState) {
		logger.Debugf("jitsi ICE state: %s", state)
	}
	s.wg.Add(1)
	trickleCtx, trickleCancel := context.WithCancel(ctx)
	s.trickleCancel = trickleCancel
	go s.trickleDrainLoop(trickleCtx, pc, neg, jSess.LowLevel().Stanzas())
	if sctpBridge {
		if err := jSess.PrepareBridgeSCTP(pc); err != nil {
			_ = pc.Close()
			return fmt.Errorf("prepare bridge sctp: %w", err)
		}
	}
	if err := neg.Accept(ctx); err != nil {
		_ = pc.Close()
		return fmt.Errorf("session-accept: %w", err)
	}
	logger.Debugf("jitsi: session-accept sent")
	if hasLocalTracks {
		if err := neg.SendSourceAddFromSDP(pc.LocalDescription().SDP); err != nil {
			logger.Debugf("jitsi: source-add (initial): %v", err)
		}
	}
	if s.shouldRequestVideo() {
		if err := jSess.RequestVideo(ctx, 720); err != nil {
			logger.Debugf("jitsi: request video: %v", err)
		}
	}
	s.pcMu.Lock()
	s.pc = pc
	s.pcMu.Unlock()
	s.wg.Add(1)
	go s.rtcpKeepalive(pc)
	return nil
}

type negotiator interface {
	HandleSourceAdd(stanza string) error
}

func (s *Session) rtcpKeepalive(pc *webrtc.PeerConnection) {
	defer s.wg.Done()
	const interval = 5 * time.Second
	const maxErrors = 3
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	pkts := []rtcp.Packet{&rtcp.ReceiverReport{}}
	errCount := 0
	for {
		select {
		case <-s.done:
			return
		case <-ticker.C:
			if err := pc.WriteRTCP(pkts); err != nil {
				if s.closed.Load() {
					return
				}
				errCount++
				logger.Debugf("jitsi: rtcp keepalive write (%d/%d): %v", errCount, maxErrors, err)
				if errCount >= maxErrors {
					logger.Warnf("jitsi: rtcp keepalive giving up after %d errors", maxErrors)
					s.requestReconnect("rtcp keepalive dead")
					return
				}
			} else {
				errCount = 0
			}
		}
	}
}

func (s *Session) trickleDrainLoop(
	ctx context.Context, pc *webrtc.PeerConnection, neg negotiator, stanzas <-chan string,
) {
	defer s.wg.Done()
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.done:
			return
		case raw, ok := <-stanzas:
			if !ok {
				return
			}
			switch {
			case strings.Contains(raw, "transport-info"):
				if err := s.applyTrickleICE(pc, raw); err != nil {
					logger.Debugf("jitsi trickle ICE: %v", err)
				}
			case strings.Contains(raw, "source-add"):
				if err := neg.HandleSourceAdd(raw); err != nil {
					logger.Debugf("jitsi source-add: %v", err)
				}
			}
		}
	}
}

type xmlCandidate struct {
	Component  string `xml:"component,attr"`
	Foundation string `xml:"foundation,attr"`
	Generation string `xml:"generation,attr"`
	IP         string `xml:"ip,attr"`
	Port       string `xml:"port,attr"`
	Priority   string `xml:"priority,attr"`
	Protocol   string `xml:"protocol,attr"`
	Type       string `xml:"type,attr"`
	RelAddr    string `xml:"rel-addr,attr"`
	RelPort    string `xml:"rel-port,attr"`
}

type xmlTransportInfo struct {
	XMLName xml.Name `xml:"iq"`
	Jingle  struct {
		Action   string `xml:"action,attr"`
		Contents []struct {
			Name      string `xml:"name,attr"`
			Transport struct {
				Candidates []xmlCandidate `xml:"candidate"`
			} `xml:"transport"`
		} `xml:"content"`
	} `xml:"jingle"`
}

func (s *Session) applyTrickleICE(pc *webrtc.PeerConnection, raw string) error {
	var ti xmlTransportInfo
	if err := xml.Unmarshal([]byte(raw), &ti); err != nil {
		return fmt.Errorf("parse transport-info: %w", err)
	}
	for i := range ti.Jingle.Contents {
		content := &ti.Jingle.Contents[i]
		mid := content.Name
		for j := range content.Transport.Candidates {
			sdpLine := buildSDPCandidate(&content.Transport.Candidates[j])
			if sdpLine == "" {
				continue
			}
			init := webrtc.ICECandidateInit{
				Candidate: sdpLine,
				SDPMid:    &mid,
			}
			if err := pc.AddICECandidate(init); err != nil {
				logger.Debugf("jitsi add ICE candidate (%s): %v", mid, err)
			}
		}
	}
	return nil
}

func buildSDPCandidate(c *xmlCandidate) string {
	if c.IP == "" || c.Port == "" {
		return ""
	}
	comp := c.Component
	if comp == "" {
		comp = "1"
	}
	proto := strings.ToLower(c.Protocol)
	if proto == "" {
		proto = "udp"
	}
	priority := c.Priority
	if priority == "" {
		priority = "1"
	}
	candType := c.Type
	if candType == "" {
		candType = "host"
	}
	var b strings.Builder
	b.Grow(48 + len(c.Foundation) + len(comp) + len(proto) + len(priority) +
		len(c.IP) + len(c.Port) + len(candType) + len(c.RelAddr) + len(c.RelPort) + len(c.Generation))
	b.WriteString("candidate:")
	b.WriteString(c.Foundation)
	b.WriteByte(' ')
	b.WriteString(comp)
	b.WriteByte(' ')
	b.WriteString(proto)
	b.WriteByte(' ')
	b.WriteString(priority)
	b.WriteByte(' ')
	b.WriteString(c.IP)
	b.WriteByte(' ')
	b.WriteString(c.Port)
	b.WriteString(" typ ")
	b.WriteString(candType)
	if c.RelAddr != "" && c.RelPort != "" {
		b.WriteString(" raddr ")
		b.WriteString(c.RelAddr)
		b.WriteString(" rport ")
		b.WriteString(c.RelPort)
	}
	if c.Generation != "" {
		b.WriteString(" generation ")
		b.WriteString(c.Generation)
	}
	return b.String()
}

func (s *Session) Send(data []byte) error {
	if s.closed.Load() {
		return ErrSessionClosed
	}
	if !s.bridgeReady.Load() {
		return ErrBridgeNotReady
	}
	framed, err := s.encodeBridgeFrame(data, "")
	if err != nil {
		return err
	}
	return s.enqueueBridgeFrame(framed)
}

func (s *Session) SendTo(peerID string, data []byte) error {
	if peerID == "" {
		return s.Send(data)
	}
	if s.closed.Load() {
		return ErrSessionClosed
	}
	if !s.bridgeReady.Load() {
		return ErrBridgeNotReady
	}
	framed, err := s.encodeBridgeFrame(data, peerID)
	if err != nil {
		return err
	}
	return s.enqueuePeerBridgeFrame(peerID, framed)
}

func (s *Session) encodeBridgeFrame(data []byte, peerID string) ([]byte, error) {
	if len(data)+frameHeaderLen > bridgeMaxMessageSize {
		return nil, ErrSendTooLarge
	}
	framed := make([]byte, frameHeaderLen+len(data))
	copy(framed, bridgeMagic[:])
	binary.BigEndian.PutUint32(framed[bridgeMagicLen:bridgeMagicLen+4], s.localEpoch.Load())
	binary.BigEndian.PutUint32(framed[bridgeMagicLen+4:frameHeaderLen], s.peerEpochFor(peerID))
	copy(framed[frameHeaderLen:], data)
	return framed, nil
}

func (s *Session) peerEpochFor(peerID string) uint32 {
	if peerID == "" || s.onPeerData == nil {
		return s.peerEpoch.Load()
	}
	s.peerEpochMu.Lock()
	v := s.peerEpochs[peerID]
	s.peerEpochMu.Unlock()
	return v
}

func (s *Session) enqueueBridgeFrame(framed []byte) error {
	if s.closed.Load() {
		return ErrSessionClosed
	}
	if !s.bridgeReady.Load() {
		return ErrBridgeNotReady
	}
	if len(framed) > bridgeMaxMessageSize {
		return ErrSendTooLarge
	}
	select {
	case s.sendQueue <- framed:
		return nil
	case <-s.done:
		return ErrSessionClosed
	default:
		return ErrSendQueueFull
	}
}

func (s *Session) enqueuePeerBridgeFrame(peerID string, framed []byte) error {
	if s.closed.Load() {
		return ErrSessionClosed
	}
	if !s.bridgeReady.Load() {
		return ErrBridgeNotReady
	}
	if len(framed) > bridgeMaxMessageSize {
		return ErrSendTooLarge
	}
	select {
	case s.peerSendQueue <- bridgeOutbound{to: peerID, data: framed}:
		return nil
	case <-s.done:
		return ErrSessionClosed
	default:
		return ErrSendQueueFull
	}
}

func (s *Session) sendLoop() {
	defer s.wg.Done()
	for {
		select {
		case <-s.done:
			return
		case data, ok := <-s.sendQueue:
			if !ok {
				return
			}
			s.sendBridgeFrame("", data)
		case frame, ok := <-s.peerSendQueue:
			if !ok {
				return
			}
			s.sendBridgeFrame(frame.to, frame.data)
		}
	}
}

func (s *Session) sendBridgeFrame(to string, data []byte) {
	if !s.outboundFrameCurrent(data) {
		return
	}
	jSess := s.waitJSession()
	if jSess == nil {
		return
	}
	if !s.outboundFrameCurrent(data) {
		return
	}
	if err := jSess.BridgeSendRaw(to, data); err != nil {
		if s.closed.Load() {
			return
		}
		logger.Debugf("jitsi bridge send: %v", err)
	}
}

func (s *Session) waitJSession() *j.Session {
	if jSess := s.jSess.Load(); jSess != nil {
		return jSess
	}
	const retryDelay = 10 * time.Millisecond
	timer := time.NewTimer(retryDelay)
	defer timer.Stop()
	for {
		if s.closed.Load() {
			return nil
		}
		if jSess := s.jSess.Load(); jSess != nil {
			return jSess
		}
		select {
		case <-s.done:
			return nil
		case <-timer.C:
			timer.Reset(retryDelay)
		}
	}
}

func (s *Session) outboundFrameCurrent(frame []byte) bool {
	if len(frame) < frameHeaderLen {
		return false
	}
	return binary.BigEndian.Uint32(frame[bridgeMagicLen:bridgeMagicLen+4]) == s.localEpoch.Load()
}

func (s *Session) recvLoop() {
	defer s.wg.Done()
	jSess := s.jSess.Load()
	if jSess == nil || (s.onData == nil && s.onPeerData == nil) || !s.bridgeReady.Load() {
		return
	}
	msgs := jSess.BridgeMessages()
	if msgs == nil {
		return
	}
	for {
		select {
		case <-s.done:
			return
		case msg, ok := <-msgs:
			if !s.deliverBridgeMessage(msg, ok) {
				return
			}
		}
	}
}

func (s *Session) deliverBridgeMessage(msg j.BridgeMessage, ok bool) bool {
	if !ok {
		if !s.closed.Load() {
			s.requestReconnect("jitsi bridge closed")
		}
		return false
	}
	payload, valid := bridgePayload(msg)
	if !valid {
		return true
	}
	if s.onPeerData != nil && msg.From != "" {
		return s.deliverPeerBridgePayload(msg.From, payload)
	}
	if !s.peerLatchAccepts(msg.From) {
		return true
	}
	data, ok := s.acceptEpochFrame(payload)
	if !ok || len(data) == 0 {
		return true
	}
	s.onData(data)
	return true
}

func bridgePayload(msg j.BridgeMessage) ([]byte, bool) {
	payload := decodeRaw(msg)
	if len(payload) < bridgeMagicLen || !bytes.Equal(payload[:bridgeMagicLen], bridgeMagic[:]) {
		return nil, false
	}
	return payload, true
}

func (s *Session) deliverPeerBridgePayload(from string, payload []byte) bool {
	data, ok := s.acceptPeerEpochFrame(from, payload)
	if !ok || len(data) == 0 {
		return true
	}
	s.onPeerData(from, data)
	return true
}

func (s *Session) acceptPeerEpochFrame(from string, payload []byte) ([]byte, bool) {
	if len(payload) < frameHeaderLen {
		return nil, false
	}
	local := s.localEpoch.Load()
	senderEpoch := binary.BigEndian.Uint32(payload[bridgeMagicLen : bridgeMagicLen+4])
	receiverEpoch := binary.BigEndian.Uint32(payload[bridgeMagicLen+4 : frameHeaderLen])
	if senderEpoch == 0 || senderEpoch == local {
		return nil, false
	}
	if receiverEpoch != 0 && receiverEpoch != local {
		logger.Debugf("jitsi: drop stale bridge frame peerEpoch=0x%08x localEpoch=0x%08x",
			receiverEpoch, local)
		return nil, false
	}
	s.peerEpochMu.Lock()
	if s.peerEpochs[from] != senderEpoch {
		s.peerEpochs[from] = senderEpoch
	}
	s.peerEpochMu.Unlock()
	return payload[frameHeaderLen:], true
}

func (s *Session) acceptEpochFrame(payload []byte) ([]byte, bool) {
	if len(payload) < frameHeaderLen {
		return nil, false
	}
	local := s.localEpoch.Load()
	senderEpoch := binary.BigEndian.Uint32(payload[bridgeMagicLen : bridgeMagicLen+4])
	receiverEpoch := binary.BigEndian.Uint32(payload[bridgeMagicLen+4 : frameHeaderLen])
	if senderEpoch == 0 || senderEpoch == local {
		return nil, false
	}
	if receiverEpoch != 0 && receiverEpoch != local {
		logger.Debugf("jitsi: drop stale bridge frame peerEpoch=0x%08x localEpoch=0x%08x",
			receiverEpoch, local)
		return nil, false
	}
	if prev := s.peerEpoch.Load(); prev == 0 {
		s.peerEpoch.Store(senderEpoch)
	} else if prev != senderEpoch {
		if s.peerEpoch.CompareAndSwap(prev, senderEpoch) {
			s.requestReconnect("jitsi peer epoch changed")
		}
		return nil, false
	}
	return payload[frameHeaderLen:], true
}

func (s *Session) peerLatchAccepts(from string) bool {
	if cur := s.peerEndpoint.Load(); cur != nil {
		return *cur == from
	}
	if from == "" {
		return true
	}
	s.peerEndpoint.CompareAndSwap(nil, &from)
	cur := s.peerEndpoint.Load()
	return cur == nil || *cur == from
}

func decodeRaw(m j.BridgeMessage) []byte {
	if m.Class != "EndpointMessage" {
		return nil
	}
	enc, ok := m.Fields["raw"].(string)
	if !ok {
		return nil
	}
	out, err := base64.StdEncoding.DecodeString(enc)
	if err != nil {
		return nil
	}
	return out
}

func (s *Session) Close() error {
	if !s.closed.CompareAndSwap(false, true) {
		return nil
	}
	jSess := s.jSess.Load()
	s.pcMu.Lock()
	pc := s.pc
	s.pc = nil
	s.pcMu.Unlock()
	if pc != nil {
		_ = pc.Close()
	}
	if jSess != nil {
		_ = jSess.Close()
	}
	s.jSess.Store(nil)
	s.bridgeReady.Store(false)
	if s.cancel != nil {
		s.cancel()
	}
	s.doneOnce.Do(func() { close(s.done) })
	stopped := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(stopped)
	}()
	select {
	case <-stopped:
	case <-time.After(2 * time.Second):
	}
	return nil
}

func (s *Session) ResetPeer() {
	s.peerEndpoint.Store(nil)
	s.peerEpoch.Store(0)
	s.resetPeerEpochs()
}

func (s *Session) SetReconnectCallback(cb func(*webrtc.DataChannel)) { s.onReconnect = cb }

func (s *Session) SetShouldReconnect(fn func() bool) { s.shouldReconnect = fn }

func (s *Session) SetEndedCallback(cb func(string)) { s.onEnded = cb }

func (s *Session) WatchConnection(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.done:
			return
		case <-s.reconnectCh:
			if s.handleReconnectAttempt(ctx) {
				return
			}
		}
	}
}

func (s *Session) Reconnect(reason string) { s.requestReconnect(reason) }

func (s *Session) requestReconnect(reason string) {
	s.bridgeReady.Store(false)
	if s.closed.Load() || s.reconnecting.Load() {
		return
	}
	if s.shouldReconnect != nil && !s.shouldReconnect() {
		s.signalEnded(reason)
		return
	}
	logger.Infof("jitsi reconnect requested: %s", reason)
	select {
	case s.reconnectCh <- struct{}{}:
	default:
	}
}

func (s *Session) handleReconnectAttempt(ctx context.Context) bool {
	now := time.Now()
	s.reconnectMu.Lock()
	if s.reconnectWindowStart.IsZero() || now.Sub(s.reconnectWindowStart) > reconnectWindow {
		s.reconnectWindowStart = now
		s.reconnectCount = 0
	}
	s.reconnectCount++
	count := s.reconnectCount
	s.reconnectMu.Unlock()
	if count > maxReconnects {
		s.signalEnded("jitsi reconnect limit reached")
		return true
	}
	backoff := time.Duration(count) * 2 * time.Second
	if backoff > 30*time.Second {
		backoff = 30 * time.Second
	}
	for {
		if err := s.reconnect(ctx); err != nil {
			logger.Warnf("jitsi reconnect failed: %v", err)
			select {
			case <-ctx.Done():
				return true
			case <-s.done:
				return true
			case <-time.After(backoff):
				continue
			}
		}
		s.drainReconnectQueue()
		return false
	}
}

func (s *Session) reconnect(ctx context.Context) error {
	if !s.reconnecting.CompareAndSwap(false, true) {
		return nil
	}
	defer s.reconnecting.Store(false)
	s.bridgeReady.Store(false)
	s.teardownPC()
	s.localEpoch.Store(randomEpoch())
	s.peerEpoch.Store(0)
	s.resetPeerEpochs()
	s.drainSendQueue()
	jSess := s.jSess.Load()
	if jSess == nil {
		return s.reconnectFull(ctx)
	}
	logger.Infof("jitsi: rejoin %s/%s (non-blocking) ...", s.host, s.room)
	if err := jSess.Rejoin(ctx, s.name); err != nil {
		logger.Warnf("jitsi: rejoin failed: %v - full reconnect", err)
		return s.reconnectFull(ctx)
	}
	logger.Infof("jitsi: waiting for session-initiate in %s/%s ...", s.host, s.room)
	if _, err := jSess.WaitJingleReinitiate(ctx); err != nil {
		logger.Warnf("jitsi: wait reinitiate failed: %v - full reconnect", err)
		return s.reconnectFull(ctx)
	}
	if err := s.reinitiateBridge(ctx, jSess); err != nil {
		return err
	}
	s.peerEndpoint.Store(nil)
	s.peerVideoSSRC.Store(0)
	s.bridgeReady.Store(true)
	s.wg.Add(1)
	go s.recvLoop()
	if err := s.Send(nil); err != nil {
		logger.Debugf("jitsi: epoch announce failed: %v", err)
	}
	if s.onReconnect != nil {
		s.onReconnect(nil)
	}
	logger.Infof("jitsi: reconnected %s/%s (reinitiate); colibri-ws=%s", s.host, s.room, jSess.ColibriWS)
	return nil
}

func (s *Session) teardownPC() {
	s.pcMu.Lock()
	oldPC := s.pc
	s.pc = nil
	s.pcMu.Unlock()
	if s.trickleCancel != nil {
		s.trickleCancel()
		s.trickleCancel = nil
	}
	if oldPC != nil {
		_ = oldPC.Close()
	}
}

func (s *Session) reinitiateBridge(ctx context.Context, jSess *j.Session) error {
	sctpBridge := jSess.ColibriWS == ""
	if err := s.negotiatePC(ctx, jSess, sctpBridge); err != nil {
		logger.Warnf("jitsi: negotiate after reinitiate failed: %v - full reconnect", err)
		return s.reconnectFull(ctx)
	}
	if sctpBridge {
		if err := s.openBridgeSCTP(ctx, jSess); err != nil {
			logger.Warnf("jitsi: bridge after reinitiate failed: %v - full reconnect", err)
			return s.reconnectFull(ctx)
		}
	} else {
		if err := s.openBridgeWS(ctx, jSess); err != nil {
			logger.Warnf("jitsi: bridge after reinitiate failed: %v - full reconnect", err)
			return s.reconnectFull(ctx)
		}
	}
	return nil
}

func (s *Session) reconnectFull(ctx context.Context) error {
	if old := s.jSess.Swap(nil); old != nil {
		_ = old.Close()
	}
	s.localEpoch.Store(randomEpoch())
	s.peerEpoch.Store(0)
	s.resetPeerEpochs()
	s.drainSendQueue()
	logger.Infof("jitsi: full reconnect %s/%s as %s ...", s.host, s.room, s.name)
	jSess, err := s.joinAndOpenBridge(ctx)
	if err != nil {
		return err
	}
	s.jSess.Store(jSess)
	s.peerEndpoint.Store(nil)
	s.peerVideoSSRC.Store(0)
	s.bridgeReady.Store(true)
	s.wg.Add(1)
	go s.recvLoop()
	if err := s.Send(nil); err != nil {
		logger.Debugf("jitsi: epoch announce failed: %v", err)
	}
	if s.onReconnect != nil {
		s.onReconnect(nil)
	}
	logger.Infof("jitsi: reconnected %s/%s (full); colibri-ws=%s", s.host, s.room, jSess.ColibriWS)
	return nil
}

func (s *Session) drainReconnectQueue() {
	for {
		select {
		case <-s.reconnectCh:
		default:
			return
		}
	}
}

func (s *Session) drainSendQueue() {
	for {
		select {
		case <-s.sendQueue:
		case <-s.peerSendQueue:
		default:
			return
		}
	}
}

func (s *Session) resetPeerEpochs() {
	s.peerEpochMu.Lock()
	clear(s.peerEpochs)
	s.peerEpochMu.Unlock()
}

func (s *Session) CanSend() bool {
	if s.closed.Load() {
		return false
	}
	if s.onData == nil && s.onPeerData == nil {
		s.pcMu.Lock()
		ready := s.pc != nil && s.pc.ConnectionState() == webrtc.PeerConnectionStateConnected
		s.pcMu.Unlock()
		return ready
	}
	return s.bridgeReady.Load()
}

func (s *Session) GetSendQueue() chan []byte { return s.sendQueue }

func (s *Session) GetBufferedAmount() uint64 {
	jSess := s.jSess.Load()
	if jSess == nil {
		return 0
	}
	depth := jSess.BridgeSendQueueDepth()
	if depth <= 0 {
		return 0
	}
	return uint64(depth) * uint64(bridgeMaxMessageSize)
}

func (s *Session) AddVideoTrack(track webrtc.TrackLocal) error {
	s.videoTrackMu.Lock()
	s.videoTracks = append(s.videoTracks, track)
	s.videoTrackMu.Unlock()
	s.pcMu.Lock()
	pc := s.pc
	s.pcMu.Unlock()
	if pc == nil {
		return nil
	}
	if _, err := pc.AddTrack(track); err != nil {
		return fmt.Errorf("add track: %w", err)
	}
	return nil
}

func (s *Session) SetVideoTrackHandler(cb func(*webrtc.TrackRemote, *webrtc.RTPReceiver)) {
	s.videoTrackMu.Lock()
	defer s.videoTrackMu.Unlock()
	s.onVideoTrack = cb
}

func (s *Session) signalEnded(reason string) {
	s.bridgeReady.Store(false)
	if s.onEnded != nil {
		s.onEnded(reason)
	}
}

func normaliseHost(raw string) string {
	raw = strings.TrimSpace(raw)
	if idx := strings.Index(raw, "://"); idx >= 0 {
		raw = raw[idx+3:]
	}
	raw = strings.TrimPrefix(raw, "//")
	raw = strings.TrimSuffix(raw, "/")
	if i := strings.IndexByte(raw, '/'); i >= 0 {
		raw = raw[:i]
	}
	return raw
}

func init() { //nolint:gochecknoinits
	engine.Register("jitsi", New)
}
