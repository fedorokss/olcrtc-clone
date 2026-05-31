package auth

import (
	"context"
	"errors"
	"sync"
)

var (
	ErrAuthNotFound            = errors.New("auth provider not found")
	ErrRoomCreationUnsupported = errors.New("auth provider does not support room creation")
	ErrRoomIDRequired          = errors.New("room ID required")
)

type Credentials struct {
	URL   string
	Token string
	Extra map[string]string
}

type Config struct {
	RoomURL   string
	Name      string
	DNSServer string
	ProxyAddr string
	ProxyPort int
	WBToken   string
	WBCookie  string
}

type Provider interface {
	Engine() string
	DefaultServiceURL() string
	Issue(ctx context.Context, cfg Config) (Credentials, error)
}

type RoomCreator interface {
	CreateRoom(ctx context.Context, cfg Config) (roomID string, token string, err error)
}

type Keeper interface {
	KeepAlive(ctx context.Context, cfg Config)
}

var (
	registryMu sync.RWMutex
	registry   = make(map[string]Provider)
)

func Register(name string, p Provider) {
	registryMu.Lock()
	registry[name] = p
	registryMu.Unlock()
}

func Get(name string) (Provider, error) {
	registryMu.RLock()
	p, ok := registry[name]
	registryMu.RUnlock()
	if !ok {
		return nil, ErrAuthNotFound
	}
	return p, nil
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
