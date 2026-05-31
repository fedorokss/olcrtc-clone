package tunnel

import (
	"context"
	"fmt"

	"github.com/fedorokss/olcrtc-clone/internal/app/session"
	"github.com/fedorokss/olcrtc-clone/internal/handshake"
	"github.com/fedorokss/olcrtc-clone/internal/server"
	"github.com/fedorokss/olcrtc-clone/internal/transport"
)

type TransportOptions = transport.Options

type AuthFunc = handshake.AuthFunc

type SessionOpenFunc = server.SessionOpenFunc

type SessionCloseFunc = server.SessionCloseFunc

type TrafficFunc = server.TrafficFunc

type Config struct {
	Transport string
	Carrier   string
	RoomURL   string

	Engine string
	URL    string
	Token  string

	KeyHex         string
	DNSServer      string
	SOCKSProxyAddr string
	SOCKSProxyPort int
	SOCKSProxyUser string
	SOCKSProxyPass string

	TransportOptions TransportOptions

	AuthHook       AuthFunc
	OnSessionOpen  SessionOpenFunc
	OnSessionClose SessionCloseFunc
	OnTraffic      TrafficFunc
}

type Server struct {
	cfg Config
}

func New(cfg Config) *Server {
	return &Server{cfg: cfg}
}

func (s *Server) Run(ctx context.Context) error {
	if err := server.Run(ctx, server.Config{
		Transport:        s.cfg.Transport,
		Carrier:          s.cfg.Carrier,
		RoomURL:          s.cfg.RoomURL,
		Engine:           s.cfg.Engine,
		URL:              s.cfg.URL,
		Token:            s.cfg.Token,
		KeyHex:           s.cfg.KeyHex,
		DNSServer:        s.cfg.DNSServer,
		SOCKSProxyAddr:   s.cfg.SOCKSProxyAddr,
		SOCKSProxyPort:   s.cfg.SOCKSProxyPort,
		SOCKSProxyUser:   s.cfg.SOCKSProxyUser,
		SOCKSProxyPass:   s.cfg.SOCKSProxyPass,
		TransportOptions: s.cfg.TransportOptions,
		AuthHook:         s.cfg.AuthHook,
		OnSessionOpen:    s.cfg.OnSessionOpen,
		OnSessionClose:   s.cfg.OnSessionClose,
		OnTraffic:        s.cfg.OnTraffic,
	}); err != nil {
		return fmt.Errorf("tunnel: %w", err)
	}
	return nil
}

func RegisterDefaults() {
	session.RegisterDefaults()
}
