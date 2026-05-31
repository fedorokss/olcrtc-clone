package livekit

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"

	protoLogger "github.com/livekit/protocol/logger"
	lksdk "github.com/livekit/server-sdk-go/v2"
	"github.com/openlibrecommunity/olcrtc/internal/engine"
	"github.com/openlibrecommunity/olcrtc/internal/logger"
	"github.com/pion/webrtc/v4"
)

const (
	defaultSendQueueSize    = 5000
	defaultSendQueueCapHard = 4000
	dataPublishTopic        = "olcrtc"
	videoTrackName          = "videochannel"
	reconnectWindow         = 5 * time.Minute
	maxReconnects           = 10
)

var (
	ErrSessionClosed    = errors.New("livekit session closed")
	ErrSendQueueFull    = errors.New("livekit send queue full")
	ErrRoomNotConnected = errors.New("livekit room not connected")
	ErrURLRequired      = errors.New("livekit signaling URL required")
	ErrTokenRequired    = errors.New("livekit access token required")
)

var (
	dataPublishOpts = []lksdk.DataPublishOption{
		lksdk.WithDataPublishTopic(dataPublishTopic),
		lksdk.WithDataPublishReliable(true),
	}
	videoTrackPublishOpts = &lksdk.TrackPublicationOptions{Name: videoTrackName}
)

type roomHandle interface {
	publishData(data []byte) error
	publishTrack(track webrtc.TrackLocal) error
	unpublishLocalTracks()
	disconnect()
	connectionState() lksdk.ConnectionState
}

type sdkRoom struct {
	room *lksdk.Room
}

func (r *sdkRoom) publishData(data []byte) error {
	if err := r.room.LocalParticipant.PublishDataPacket(lksdk.UserData(data), dataPublishOpts...); err != nil {
		return fmt.Errorf("publish data packet: %w", err)
	}
	return nil
}

func (r *sdkRoom) publishTrack(track webrtc.TrackLocal) error {
	_, err := r.room.LocalParticipant.PublishTrack(track, videoTrackPublishOpts)
	if err != nil {
		return fmt.Errorf("publish track: %w", err)
	}
	return nil
}

func (r *sdkRoom) unpublishLocalTracks() {
	if r.room == nil || r.room.LocalParticipant == nil {
		return
	}
	for _, publication := range r.room.LocalParticipant.TrackPublications() {
		sid := publication.SID()
		if sid == "" {
			continue
		}
		if err := r.room.LocalParticipant.UnpublishTrack(sid); err != nil {
			log.Printf("livekit unpublish track error: %v", err)
		}
	}
}

func (r *sdkRoom) disconnect() {
	r.room.Disconnect()
	time.Sleep(2 * time.Second)
}

func (r *sdkRoom) connectionState() lksdk.ConnectionState {
	return r.room.ConnectionState()
}

type connectRoomFunc func(url, token string, callback *lksdk.RoomCallback) (roomHandle, error)

func connectSDKRoom(url, token string, callback *lksdk.RoomCallback) (roomHandle, error) {
	room, err := lksdk.ConnectToRoomWithToken(
		url,
		token,
		callback,
		lksdk.WithAutoSubscribe(true),
		lksdk.WithLogger(protoLogger.GetDiscardLogger()),
	)
	if err != nil {
		return nil, fmt.Errorf("connect to livekit room: %w", err)
	}
	return &sdkRoom{room: room}, nil
}

type Session struct {
	url             string
	token           string
	name            string
	refresh         func(ctx context.Context) (engine.Credentials, error)
	connectRoom     connectRoomFunc
	room            atomic.Pointer[roomHandle]
	onData          func([]byte)
	onReconnect     func(*webrtc.DataChannel)
	shouldReconnect func() bool
	onEnded         func(string)
	reconnectCh     chan struct{}
	closeCh         chan struct{}
	lastReconnect   time.Time
	reconnectCount  int
	sendQueue       chan []byte
	closed          atomic.Bool
	reconnecting    atomic.Bool
	done            chan struct{}
	cancel          context.CancelFunc
	shutdownOnce    sync.Once
	sendWorkerOnce  sync.Once
	videoTrackMu    sync.RWMutex
	videoTracks     []webrtc.TrackLocal
	onVideoTrack    func(*webrtc.TrackRemote, *webrtc.RTPReceiver)
	wg              sync.WaitGroup
}

func New(ctx context.Context, cfg engine.Config) (engine.Session, error) {
	if cfg.URL == "" {
		return nil, ErrURLRequired
	}
	if cfg.Token == "" {
		return nil, ErrTokenRequired
	}
	_, cancel := context.WithCancel(ctx)
	return &Session{
		url:         cfg.URL,
		token:       cfg.Token,
		name:        cfg.Name,
		refresh:     cfg.Refresh,
		connectRoom: connectSDKRoom,
		onData:      cfg.OnData,
		reconnectCh: make(chan struct{}, 1),
		closeCh:     make(chan struct{}),
		sendQueue:   make(chan []byte, defaultSendQueueSize),
		done:        make(chan struct{}),
		cancel:      cancel,
	}, nil
}

func (s *Session) Capabilities() engine.Capabilities {
	return engine.Capabilities{ByteStream: true, VideoTrack: true}
}

func (s *Session) Connect(ctx context.Context) error {
	s.closed.Store(false)
	if err := s.connectSession(ctx); err != nil {
		return err
	}
	s.startSendWorker()
	return nil
}

func (s *Session) connectSession(_ context.Context) error {
	roomCB := &lksdk.RoomCallback{
		ParticipantCallback: lksdk.ParticipantCallback{
			OnDataReceived: func(data []byte, _ lksdk.DataReceiveParams) {
				if s.onData != nil {
					s.onData(data)
				}
			},
			OnTrackSubscribed: func(track *webrtc.TrackRemote, _ *lksdk.RemoteTrackPublication, _ *lksdk.RemoteParticipant) {
				if track.Kind() != webrtc.RTPCodecTypeVideo {
					return
				}
				s.videoTrackMu.RLock()
				cb := s.onVideoTrack
				s.videoTrackMu.RUnlock()
				if cb != nil {
					cb(track, nil)
				}
			},
		},
		OnDisconnected: func() {
			if s.closed.Load() || s.reconnecting.Load() {
				return
			}
			if !s.queueReconnect() {
				s.signalEnded("disconnected from livekit")
			}
		},
	}

	room, err := s.connectRoom(s.url, s.token, roomCB)
	if err != nil {
		return fmt.Errorf("connect to room: %w", err)
	}
	s.setRoom(room)
	if err := s.publishPendingTracks(); err != nil {
		return err
	}
	return nil
}

func (s *Session) publishPendingTracks() error {
	room := s.currentRoom()
	if room == nil {
		return ErrRoomNotConnected
	}
	s.videoTrackMu.RLock()
	defer s.videoTrackMu.RUnlock()
	for _, track := range s.videoTracks {
		if err := room.publishTrack(track); err != nil {
			return fmt.Errorf("failed to publish track: %w", err)
		}
	}
	return nil
}

func (s *Session) startSendWorker() {
	s.sendWorkerOnce.Do(func() {
		s.wg.Add(1)
		go s.processSendQueue()
	})
}

func (s *Session) processSendQueue() {
	defer s.wg.Done()
	for {
		select {
		case <-s.done:
			return
		case data, ok := <-s.sendQueue:
			if !ok {
				return
			}
			room := s.waitForConnectedRoom()
			if room == nil {
				return
			}
			if err := room.publishData(data); err != nil {
				log.Printf("livekit publish data error: %v", err)
			}
		}
	}
}

func (s *Session) waitForConnectedRoom() roomHandle {
	if room := s.currentRoom(); room != nil && room.connectionState() == lksdk.ConnectionStateConnected {
		return room
	}
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-s.done:
			return nil
		case <-ticker.C:
			if room := s.currentRoom(); room != nil && room.connectionState() == lksdk.ConnectionStateConnected {
				return room
			}
		}
	}
}

func (s *Session) Send(data []byte) error {
	if s.closed.Load() {
		return ErrSessionClosed
	}
	select {
	case s.sendQueue <- data:
		return nil
	default:
		return ErrSendQueueFull
	}
}

func (s *Session) Close() error {
	s.closed.Store(true)
	s.shutdown()
	return nil
}

func (s *Session) shutdown() {
	s.shutdownOnce.Do(func() {
		if s.cancel != nil {
			s.cancel()
		}
		closeSignal(s.closeCh)
		closeSignal(s.done)
		if room := s.swapRoom(nil); room != nil {
			room.unpublishLocalTracks()
			room.disconnect()
		}
		s.wg.Wait()
	})
}

func (s *Session) SetReconnectCallback(cb func(*webrtc.DataChannel)) { s.onReconnect = cb }

func (s *Session) SetShouldReconnect(fn func() bool) { s.shouldReconnect = fn }

func (s *Session) SetEndedCallback(cb func(string)) { s.onEnded = cb }

func (s *Session) WatchConnection(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.closeCh:
			return
		case <-s.reconnectCh:
			if s.handleReconnectAttempt(ctx) {
				return
			}
		}
	}
}

func (s *Session) handleReconnectAttempt(ctx context.Context) bool {
	if time.Since(s.lastReconnect) > reconnectWindow {
		s.reconnectCount = 0
	}
	s.reconnectCount++
	s.lastReconnect = time.Now()

	if s.reconnectCount > maxReconnects {
		s.signalEnded("reconnect limit reached")
		return true
	}

	backoff := time.Duration(s.reconnectCount) * 2 * time.Second
	if backoff > 30*time.Second {
		backoff = 30 * time.Second
	}

	for {
		if err := s.reconnect(ctx); err != nil {
			logger.Debugf("livekit reconnect failed: %v", err)
			select {
			case <-ctx.Done():
				return true
			case <-s.closeCh:
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
	s.reconnecting.Store(true)
	defer s.reconnecting.Store(false)

	if room := s.swapRoom(nil); room != nil {
		room.unpublishLocalTracks()
		room.disconnect()
	}

	if s.refresh != nil {
		creds, err := s.refresh(ctx)
		if err != nil {
			return fmt.Errorf("refresh credentials: %w", err)
		}
		s.applyRefreshedCredentials(creds)
	}

	if err := s.connectSession(ctx); err != nil {
		return err
	}
	if s.onReconnect != nil {
		s.onReconnect(nil)
	}
	return nil
}

func (s *Session) applyRefreshedCredentials(creds engine.Credentials) {
	if creds.URL != "" {
		s.url = creds.URL
	}
	if creds.Token != "" {
		s.token = creds.Token
	}
}

func (s *Session) queueReconnect() bool {
	if s.closed.Load() || s.reconnecting.Load() {
		return false
	}
	if s.shouldReconnect != nil && !s.shouldReconnect() {
		return false
	}
	select {
	case s.reconnectCh <- struct{}{}:
	default:
	}
	return true
}

func (s *Session) Reconnect(reason string) {
	if s.closed.Load() {
		return
	}
	logger.Infof("livekit reconnect requested: %s", reason)
	s.queueReconnect()
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

func (s *Session) signalEnded(reason string) {
	s.closed.Store(true)
	s.shutdown()
	if s.onEnded != nil {
		s.onEnded(reason)
	}
}

func (s *Session) CanSend() bool {
	if s.closed.Load() || s.reconnecting.Load() || len(s.sendQueue) >= defaultSendQueueCapHard {
		return false
	}
	room := s.currentRoom()
	return room != nil && room.connectionState() == lksdk.ConnectionStateConnected
}

func (s *Session) GetSendQueue() chan []byte { return s.sendQueue }

func (s *Session) GetBufferedAmount() uint64 { return 0 }

func (s *Session) AddVideoTrack(track webrtc.TrackLocal) error {
	s.videoTrackMu.Lock()
	s.videoTracks = append(s.videoTracks, track)
	s.videoTrackMu.Unlock()

	room := s.currentRoom()
	if room == nil {
		return nil
	}
	if err := room.publishTrack(track); err != nil {
		return fmt.Errorf("failed to publish track: %w", err)
	}
	return nil
}

func (s *Session) SetVideoTrackHandler(cb func(*webrtc.TrackRemote, *webrtc.RTPReceiver)) {
	s.videoTrackMu.Lock()
	defer s.videoTrackMu.Unlock()
	s.onVideoTrack = cb
}

func (s *Session) currentRoom() roomHandle {
	if p := s.room.Load(); p != nil {
		return *p
	}
	return nil
}

func (s *Session) setRoom(room roomHandle) {
	s.room.Store(&room)
}

func (s *Session) swapRoom(room roomHandle) roomHandle {
	var p *roomHandle
	if room != nil {
		p = &room
	}
	if old := s.room.Swap(p); old != nil {
		return *old
	}
	return nil
}

func closeSignal(ch chan struct{}) {
	select {
	case <-ch:
	default:
		close(ch)
	}
}

func init() {
	engine.Register("livekit", New)
}
