package engine

import (
	"context"
	"errors"
	"sync"

	"github.com/pion/webrtc/v4"
)

var (
	ErrEngineNotFound        = errors.New("engine not found")
	ErrByteStreamUnsupported = errors.New("engine does not support byte stream")
	ErrVideoTrackUnsupported = errors.New("engine does not support video tracks")
)

type Capabilities struct {
	ByteStream bool
	VideoTrack bool
}

type Credentials struct {
	URL   string
	Token string
	Extra map[string]string
}

type Config struct {
	URL        string
	Token      string
	Name       string
	Extra      map[string]string
	OnData     func([]byte)
	OnPeerData func(peerID string, data []byte)
	DNSServer  string
	ProxyAddr  string
	ProxyPort  int
	Refresh    func(ctx context.Context) (Credentials, error)
}

type Session interface {
	Connect(ctx context.Context) error
	Send(data []byte) error
	Close() error
	SetReconnectCallback(cb func(*webrtc.DataChannel))
	SetShouldReconnect(fn func() bool)
	SetEndedCallback(cb func(string))
	WatchConnection(ctx context.Context)
	CanSend() bool
	GetSendQueue() chan []byte
	GetBufferedAmount() uint64
	Capabilities() Capabilities
	Reconnect(reason string)
}

type PeerSession interface {
	SendTo(peerID string, data []byte) error
}

type VideoTrackCapable interface {
	AddVideoTrack(track webrtc.TrackLocal) error
	SetVideoTrackHandler(cb func(*webrtc.TrackRemote, *webrtc.RTPReceiver))
}

type Factory func(ctx context.Context, cfg Config) (Session, error)

var (
	registryMu sync.RWMutex
	registry   = make(map[string]Factory)
)

func Register(name string, factory Factory) {
	registryMu.Lock()
	registry[name] = factory
	registryMu.Unlock()
}

func New(ctx context.Context, name string, cfg Config) (Session, error) {
	registryMu.RLock()
	factory, ok := registry[name]
	registryMu.RUnlock()
	if !ok {
		return nil, ErrEngineNotFound
	}
	return factory(ctx, cfg)
}

func Available() []string {
	registryMu.RLock()
	names := make([]string, 0, len(registry))
	for name := range registry {
		names = append(names, name)
	}
	registryMu.RUnlock()
	return names
}
