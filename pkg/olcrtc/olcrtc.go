package olcrtc

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"

	"github.com/openlibrecommunity/olcrtc/internal/auth"
	"github.com/openlibrecommunity/olcrtc/internal/engine"
	enginebuiltin "github.com/openlibrecommunity/olcrtc/internal/engine/builtin"
)

var (
	ErrURLRequired             = errors.New("olcrtc: URL required when using direct engine mode")
	ErrTokenRequired           = errors.New("olcrtc: Token required when using direct engine mode")
	ErrRoomCreationUnsupported = errors.New("olcrtc: auth provider does not support room creation")
	ErrSessionEnded            = errors.New("olcrtc: session ended")
)

type Config struct {
	Auth   string
	RoomID string

	Engine string
	URL    string
	Token  string

	Name string

	DNSServer string

	ProxyAddr string
	ProxyPort int
}

type Session struct {
	inner        engine.Session
	pr           *io.PipeReader
	pw           *io.PipeWriter
	authProvider auth.Provider
	authCfg      auth.Config
}

func RegisterDefaults() {
	enginebuiltin.RegisterDefaults()
}

func New(ctx context.Context, cfg Config) (*Session, error) {
	if cfg.Auth != "" {
		return newWithAuth(ctx, cfg)
	}
	return newDirect(ctx, cfg)
}

func newWithAuth(ctx context.Context, cfg Config) (*Session, error) {
	p, err := auth.Get(cfg.Auth)
	if err != nil {
		return nil, fmt.Errorf("olcrtc: auth provider %q not registered: %w", cfg.Auth, err)
	}
	authCfg := auth.Config{
		RoomURL:   cfg.RoomID,
		Name:      cfg.Name,
		DNSServer: cfg.DNSServer,
		ProxyAddr: cfg.ProxyAddr,
		ProxyPort: cfg.ProxyPort,
	}
	creds, err := p.Issue(ctx, authCfg)
	if err != nil {
		return nil, fmt.Errorf("olcrtc: auth issue: %w", err)
	}
	engineName := p.Engine()
	pr, pw := io.Pipe()
	sess, err := engine.New(ctx, engineName, engine.Config{
		URL:       creds.URL,
		Token:     creds.Token,
		Name:      cfg.Name,
		Extra:     creds.Extra,
		OnData:    func(data []byte) { _, _ = pw.Write(data) },
		DNSServer: cfg.DNSServer,
		ProxyAddr: cfg.ProxyAddr,
		ProxyPort: cfg.ProxyPort,
		Refresh: func(rCtx context.Context) (engine.Credentials, error) {
			fresh, freshErr := p.Issue(rCtx, authCfg)
			if freshErr != nil {
				return engine.Credentials{}, fmt.Errorf("olcrtc: auth refresh: %w", freshErr)
			}
			return engine.Credentials{URL: fresh.URL, Token: fresh.Token, Extra: fresh.Extra}, nil
		},
	})
	if err != nil {
		_ = pw.CloseWithError(err)
		return nil, fmt.Errorf("olcrtc: engine %q: %w", engineName, err)
	}
	return &Session{inner: sess, pr: pr, pw: pw, authProvider: p, authCfg: authCfg}, nil
}

func newDirect(ctx context.Context, cfg Config) (*Session, error) {
	if cfg.URL == "" {
		return nil, ErrURLRequired
	}
	if cfg.Token == "" {
		return nil, ErrTokenRequired
	}
	engineName := cfg.Engine
	if engineName == "" {
		engineName = "livekit"
	}
	pr, pw := io.Pipe()
	sess, err := engine.New(ctx, engineName, engine.Config{
		URL:       cfg.URL,
		Token:     cfg.Token,
		Name:      cfg.Name,
		OnData:    func(data []byte) { _, _ = pw.Write(data) },
		DNSServer: cfg.DNSServer,
		ProxyAddr: cfg.ProxyAddr,
		ProxyPort: cfg.ProxyPort,
	})
	if err != nil {
		_ = pw.CloseWithError(err)
		return nil, fmt.Errorf("olcrtc: engine %q: %w", engineName, err)
	}
	return &Session{inner: sess, pr: pr, pw: pw}, nil
}

func (s *Session) Dial(ctx context.Context) (net.Conn, error) {
	s.inner.SetEndedCallback(func(_ string) {
		_ = s.pw.CloseWithError(ErrSessionEnded)
	})
	if err := s.Connect(ctx); err != nil {
		return nil, err
	}
	go s.inner.WatchConnection(ctx)
	return &conn{s: s}, nil
}

func (s *Session) Connect(ctx context.Context) error {
	if err := s.inner.Connect(ctx); err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	return nil
}

func (s *Session) Send(data []byte) error {
	if err := s.inner.Send(data); err != nil {
		return fmt.Errorf("send: %w", err)
	}
	return nil
}

func (s *Session) Close() error {
	if err := s.inner.Close(); err != nil {
		return fmt.Errorf("close: %w", err)
	}
	return nil
}

func (s *Session) WatchConnection(ctx context.Context) {
	s.inner.WatchConnection(ctx)
}

func (s *Session) CanSend() bool {
	return s.inner.CanSend()
}

func (s *Session) SetEndedCallback(cb func(reason string)) {
	s.inner.SetEndedCallback(cb)
}

func (s *Session) SetShouldReconnect(fn func() bool) {
	s.inner.SetShouldReconnect(fn)
}

func CreateRoom(ctx context.Context, authName string) (string, error) {
	p, err := auth.Get(authName)
	if err != nil {
		return "", fmt.Errorf("olcrtc: auth provider %q not registered: %w", authName, err)
	}
	creator, ok := p.(auth.RoomCreator)
	if !ok {
		return "", fmt.Errorf("%w: %s", ErrRoomCreationUnsupported, authName)
	}
	roomID, err := creator.CreateRoom(ctx, auth.Config{})
	if err != nil {
		return "", fmt.Errorf("olcrtc: create room: %w", err)
	}
	return roomID, nil
}
