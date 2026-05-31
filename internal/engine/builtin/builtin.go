package builtin

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/openlibrecommunity/olcrtc/internal/auth"
	authJitsi "github.com/openlibrecommunity/olcrtc/internal/auth/jitsi"
	authTelemost "github.com/openlibrecommunity/olcrtc/internal/auth/telemost"
	authWBStream "github.com/openlibrecommunity/olcrtc/internal/auth/wbstream"
	"github.com/openlibrecommunity/olcrtc/internal/engine"
	_ "github.com/openlibrecommunity/olcrtc/internal/engine/goolom"
	_ "github.com/openlibrecommunity/olcrtc/internal/engine/jitsi"
	_ "github.com/openlibrecommunity/olcrtc/internal/engine/livekit"
)

var (
	ErrCarrierNotFound = errors.New("carrier not found")
	ErrAuthFailed      = errors.New("carrier auth failed")
)

type Config struct {
	RoomURL    string
	Name       string
	OnData     func([]byte)
	OnPeerData func(peerID string, data []byte)
	DNSServer  string
	ProxyAddr  string
	ProxyPort  int
	WBToken    string
	WBCookie   string
	Engine     string
	URL        string
	Token      string
}

type Factory func(ctx context.Context, cfg Config) (engine.Session, error)

var (
	registryMu sync.RWMutex
	registry   = make(map[string]Factory, 8)
)

func Register(name string, f Factory) {
	registryMu.Lock()
	registry[name] = f
	registryMu.Unlock()
}

func Open(ctx context.Context, name string, cfg Config) (engine.Session, error) {
	registryMu.RLock()
	f, ok := registry[name]
	registryMu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrCarrierNotFound, name)
	}
	return f(ctx, cfg)
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

func RegisterDefaults() {
	registerEngineAuth("wbstream", authWBStream.Provider{})
	registerEngineAuth("telemost", authTelemost.Provider{})
	registerEngineAuth("jitsi", authJitsi.Provider{})
	registerDirect("none")
}

func registerDirect(name string) {
	Register(name, func(ctx context.Context, cfg Config) (engine.Session, error) {
		engineName := cfg.Engine
		if engineName == "" {
			engineName = "livekit"
		}
		sess, err := engine.New(ctx, engineName, engine.Config{
			URL:        cfg.URL,
			Token:      cfg.Token,
			Name:       cfg.Name,
			OnData:     cfg.OnData,
			OnPeerData: cfg.OnPeerData,
			DNSServer:  cfg.DNSServer,
			ProxyAddr:  cfg.ProxyAddr,
			ProxyPort:  cfg.ProxyPort,
		})
		if err != nil {
			return nil, fmt.Errorf("engine new: %w", err)
		}
		return sess, nil
	})
}

func registerEngineAuth(name string, provider auth.Provider) {
	engineName := provider.Engine()
	Register(name, func(ctx context.Context, cfg Config) (engine.Session, error) {
		authCfg := auth.Config{
			RoomURL:   cfg.RoomURL,
			Name:      cfg.Name,
			DNSServer: cfg.DNSServer,
			ProxyAddr: cfg.ProxyAddr,
			ProxyPort: cfg.ProxyPort,
			WBToken:   cfg.WBToken,
			WBCookie:  cfg.WBCookie,
		}
		creds, err := provider.Issue(ctx, authCfg)
		if err != nil {
			return nil, fmt.Errorf("%w: %w", ErrAuthFailed, err)
		}
		sess, err := engine.New(ctx, engineName, engine.Config{
			URL:        creds.URL,
			Token:      creds.Token,
			Name:       cfg.Name,
			Extra:      creds.Extra,
			OnData:     cfg.OnData,
			OnPeerData: cfg.OnPeerData,
			DNSServer:  cfg.DNSServer,
			ProxyAddr:  cfg.ProxyAddr,
			ProxyPort:  cfg.ProxyPort,
			Refresh: func(ctx context.Context) (engine.Credentials, error) {
				fresh, err := provider.Issue(ctx, authCfg)
				if err != nil {
					return engine.Credentials{}, fmt.Errorf("auth refresh: %w", err)
				}
				return engine.Credentials{URL: fresh.URL, Token: fresh.Token, Extra: fresh.Extra}, nil
			},
		})
		if err != nil {
			return nil, fmt.Errorf("engine new: %w", err)
		}
		return sess, nil
	})
}
