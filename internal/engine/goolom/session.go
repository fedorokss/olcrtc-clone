package goolom

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/fedorokss/olcrtc-clone/internal/engine"
	"github.com/gorilla/websocket"
	"github.com/pion/webrtc/v4"
)

const (
	realDataChannelMessageLimit = 12288
	defaultSendDelayLow         = 2 * time.Millisecond
	defaultSendDelayMax         = 12 * time.Millisecond
	defaultTelemetryInterval    = 20 * time.Second
	defaultSendQueueSize        = 5000
	defaultBufferHighWaterMark  = 512 * 1024
	defaultSendQueueCapHard     = 4000

	wsReadTimeout      = 60 * time.Second
	wsHandshakeTimeout = 15 * time.Second

	keyUID         = "uid"
	keyDescription = "description"
	keyPcSeq       = "pcSeq"
	keyName        = "name"

	stateTerminated = "terminated"

	credentialKeyRoomID           = "roomID"
	credentialKeyCredentials      = "credentials"
	credentialKeyRoomURL          = "roomURL"
	credentialKeyTelemetryReferer = "telemetryReferer"

	sendEnqueueTimeout = 50 * time.Millisecond
)

var (
	ErrDataChannelTimeout      = errors.New("datachannel timeout")
	ErrDataChannelNotReady     = errors.New("datachannel not ready")
	ErrSendQueueClosed         = errors.New("send queue closed")
	ErrSessionClosed           = errors.New("session closed")
	ErrSendQueueTimeout        = errors.New("send queue timeout")
	ErrPeerClosed              = errors.New("peer closed")
	ErrSubscriberMediaTimeout  = errors.New("subscriber media timeout")
	ErrPublisherNotInitialized = errors.New("publisher peer connection not initialized")
	ErrURLRequired             = errors.New("goolom media server URL required")
	ErrRoomIDRequired          = errors.New("goolom room ID required")
	ErrPeerIDRequired          = errors.New("goolom peer ID required")
	ErrNoRefresh               = errors.New("goolom reconnect: no refresh callback supplied")
)

type TrafficShape struct {
	MaxMessageSize int
	MinDelay       time.Duration
	MaxDelay       time.Duration
}

type Session struct {
	name             string
	mediaServerURL   string
	peerID           string
	roomID           string
	credentials      string
	roomURL          string
	telemetryReferer string

	refresh func(ctx context.Context) (engine.Credentials, error)

	ws    *websocket.Conn
	wsMu  sync.Mutex
	pcSub *webrtc.PeerConnection
	pcPub *webrtc.PeerConnection
	dc    *webrtc.DataChannel

	onData          func([]byte)
	onReconnect     func(*webrtc.DataChannel)
	shouldReconnect func() bool
	onEnded         func(string)

	reconnectCh    chan struct{}
	closeCh        chan struct{}
	keepAliveCh    chan struct{}
	telemetryCh    chan struct{}
	sessionCloseCh chan struct{}

	lastReconnect  time.Time
	reconnectCount int
	sessionMu      sync.Mutex

	sendQueue       chan []byte
	sendQueueClosed atomic.Bool
	closed          atomic.Bool
	reconnecting    atomic.Bool
	telemetryActive atomic.Bool

	ackMu      sync.Mutex
	ackWaiters map[string]chan struct{}

	trafficShape TrafficShape

	videoTrackMu sync.RWMutex
	videoTracks  []webrtc.TrackLocal
	onVideoTrack func(*webrtc.TrackRemote, *webrtc.RTPReceiver)

	subscriberReady atomic.Bool
	publisherReady  atomic.Bool
	subscriberConn  chan struct{}
	publisherConn   chan struct{}

	wg sync.WaitGroup

	httpClient *http.Client
}

func New(_ context.Context, cfg engine.Config) (engine.Session, error) {
	if cfg.URL == "" {
		return nil, ErrURLRequired
	}
	peerID := cfg.Token
	if peerID == "" {
		return nil, ErrPeerIDRequired
	}
	var roomID, credentials, roomURL, telemetryReferer string
	if cfg.Extra != nil {
		roomID = cfg.Extra[credentialKeyRoomID]
		credentials = cfg.Extra[credentialKeyCredentials]
		roomURL = cfg.Extra[credentialKeyRoomURL]
		telemetryReferer = cfg.Extra[credentialKeyTelemetryReferer]
	}
	if roomID == "" {
		return nil, ErrRoomIDRequired
	}
	if telemetryReferer == "" {
		telemetryReferer = roomURL
	}
	return &Session{
		name:             cfg.Name,
		mediaServerURL:   cfg.URL,
		peerID:           peerID,
		roomID:           roomID,
		credentials:      credentials,
		roomURL:          roomURL,
		telemetryReferer: telemetryReferer,
		refresh:          cfg.Refresh,
		onData:           cfg.OnData,
		reconnectCh:      make(chan struct{}, 1),
		closeCh:          make(chan struct{}),
		keepAliveCh:      make(chan struct{}),
		sessionCloseCh:   make(chan struct{}),
		telemetryCh:      make(chan struct{}, 1),
		sendQueue:        make(chan []byte, defaultSendQueueSize),
		ackWaiters:       make(map[string]chan struct{}),
		subscriberConn:   make(chan struct{}),
		publisherConn:    make(chan struct{}),
		trafficShape: TrafficShape{
			MaxMessageSize: realDataChannelMessageLimit,
			MinDelay:       defaultSendDelayLow,
			MaxDelay:       defaultSendDelayMax,
		},
		httpClient: nil,
	}, nil
}

func (s *Session) Capabilities() engine.Capabilities {
	return engine.Capabilities{ByteStream: true, VideoTrack: true}
}

func (s *Session) SetTrafficShape(shape TrafficShape) {
	if shape.MaxMessageSize <= 0 {
		shape.MaxMessageSize = realDataChannelMessageLimit
	}
	if shape.MaxDelay < shape.MinDelay {
		shape.MaxDelay = shape.MinDelay
	}
	s.trafficShape = shape
}

func (s *Session) Send(data []byte) error {
	if s.dc == nil || s.dc.ReadyState() != webrtc.DataChannelStateOpen {
		return ErrDataChannelNotReady
	}
	if s.sendQueueClosed.Load() {
		return ErrSendQueueClosed
	}
	select {
	case s.sendQueue <- data:
		return nil
	default:
	}
	timer := time.NewTimer(sendEnqueueTimeout)
	defer timer.Stop()
	select {
	case s.sendQueue <- data:
		return nil
	case <-timer.C:
		return ErrSendQueueTimeout
	}
}

func (s *Session) GetSendQueue() chan []byte { return s.sendQueue }

func (s *Session) GetBufferedAmount() uint64 {
	if s.dc != nil {
		return s.dc.BufferedAmount()
	}
	return 0
}

func (s *Session) SetEndedCallback(cb func(string)) { s.onEnded = cb }

func (s *Session) SetReconnectCallback(cb func(*webrtc.DataChannel)) { s.onReconnect = cb }

func (s *Session) SetShouldReconnect(fn func() bool) { s.shouldReconnect = fn }

func (s *Session) CanSend() bool {
	if s.onData == nil {
		if !s.closed.Load() || !s.subscriberReady.Load() {
			if s.closed.Load() || !s.subscriberReady.Load() {
				return false
			}
		}
		if !s.subscriberReady.Load() || s.closed.Load() {
			return false
		}
		if s.hasLocalVideoTracks() {
			return s.publisherReady.Load()
		}
		return true
	}
	if s.dc == nil || s.dc.ReadyState() != webrtc.DataChannelStateOpen {
		return false
	}
	return len(s.sendQueue) < defaultSendQueueCapHard
}

func (s *Session) AddVideoTrack(track webrtc.TrackLocal) error {
	s.videoTrackMu.Lock()
	s.videoTracks = append(s.videoTracks, track)
	s.videoTrackMu.Unlock()
	if s.pcPub == nil {
		return nil
	}
	if _, err := s.pcPub.AddTrack(track); err != nil {
		return fmt.Errorf("failed to add track: %w", err)
	}
	return nil
}

func (s *Session) SetVideoTrackHandler(cb func(*webrtc.TrackRemote, *webrtc.RTPReceiver)) {
	s.videoTrackMu.Lock()
	s.onVideoTrack = cb
	s.videoTrackMu.Unlock()
}

func (s *Session) hasLocalVideoTracks() bool {
	s.videoTrackMu.RLock()
	n := len(s.videoTracks)
	s.videoTrackMu.RUnlock()
	return n > 0
}

func (s *Session) videoTrackHandler() func(*webrtc.TrackRemote, *webrtc.RTPReceiver) {
	s.videoTrackMu.RLock()
	h := s.onVideoTrack
	s.videoTrackMu.RUnlock()
	return h
}

func (s *Session) attachPendingVideoTracks() error {
	s.videoTrackMu.RLock()
	tracks := s.videoTracks
	s.videoTrackMu.RUnlock()
	for _, track := range tracks {
		if _, err := s.pcPub.AddTrack(track); err != nil {
			return fmt.Errorf("add video track: %w", err)
		}
	}
	return nil
}

func closeSignal(ch chan struct{}) {
	if ch == nil {
		return
	}
	select {
	case <-ch:
	default:
		close(ch)
	}
}

func init() {
	engine.Register("goolom", New)
}
