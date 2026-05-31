package transport

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

var ErrTransportNotFound = errors.New("transport not found")

var ErrOptionsTypeMismatch = errors.New("transport options type mismatch")

type Features struct {
	Reliable        bool
	Ordered         bool
	MessageOriented bool
	MaxPayloadSize  int
}

type Transport interface {
	Connect(ctx context.Context) error
	Send(data []byte) error
	Close() error
	SetReconnectCallback(cb func())
	SetShouldReconnect(fn func() bool)
	SetEndedCallback(cb func(string))
	WatchConnection(ctx context.Context)
	CanSend() bool
	Features() Features
	Reconnect(reason string)
}

type PeerTransport interface {
	Transport
	SendTo(peerID string, data []byte) error
	SupportsPeerRouting() bool
}

type Options interface {
	TransportOptions()
}

type TrafficConfig struct {
	MaxPayloadSize int
	MinDelay       time.Duration
	MaxDelay       time.Duration
}

type Config struct {
	Carrier string
	RoomURL string

	Engine    string
	URL       string
	Token     string
	WBToken   string
	WBCookie  string
	ChannelID string
	DeviceID  string
	Name      string

	OnData     func([]byte)
	OnPeerData func(peerID string, data []byte)

	DNSServer string
	ProxyAddr string
	ProxyPort int

	Options Options

	Traffic TrafficConfig
}

type Factory func(ctx context.Context, cfg Config) (Transport, error)

var (
	registryMu sync.RWMutex
	registry   = make(map[string]Factory)
)

func Register(name string, factory Factory) {
	registryMu.Lock()
	registry[name] = factory
	registryMu.Unlock()
}

func New(ctx context.Context, name string, cfg Config) (Transport, error) {
	registryMu.RLock()
	factory, ok := registry[name]
	registryMu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrTransportNotFound, name)
	}
	tr, err := factory(ctx, cfg)
	if err != nil {
		return nil, err
	}
	return WithTraffic(tr, cfg.Traffic), nil
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
