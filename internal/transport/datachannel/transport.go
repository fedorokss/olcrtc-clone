package datachannel

import (
	"context"
	"errors"
	"fmt"

	"github.com/fedorokss/olcrtc-clone/internal/engine"
	enginebuiltin "github.com/fedorokss/olcrtc-clone/internal/engine/builtin"
	"github.com/fedorokss/olcrtc-clone/internal/transport"
	"github.com/pion/webrtc/v4"
)

const defaultMaxPayloadSize = 12 * 1024

var ErrByteStreamUnsupported = errors.New("engine does not support byte stream")

var datachannelFeatures = transport.Features{
	Reliable:        true,
	Ordered:         true,
	MessageOriented: true,
	MaxPayloadSize:  defaultMaxPayloadSize,
}

type streamTransport struct {
	session  engine.Session
	peer     engine.PeerSession
	resetter interface{ ResetPeer() }
}

func New(ctx context.Context, cfg transport.Config) (transport.Transport, error) {
	sess, err := enginebuiltin.Open(ctx, cfg.Carrier, enginebuiltin.Config{
		RoomURL:    cfg.RoomURL,
		Name:       cfg.Name,
		OnData:     cfg.OnData,
		OnPeerData: cfg.OnPeerData,
		DNSServer:  cfg.DNSServer,
		ProxyAddr:  cfg.ProxyAddr,
		ProxyPort:  cfg.ProxyPort,
		Engine:     cfg.Engine,
		URL:        cfg.URL,
		Token:      cfg.Token,
	})
	if err != nil {
		return nil, fmt.Errorf("open engine session: %w", err)
	}
	if !sess.Capabilities().ByteStream {
		_ = sess.Close()
		return nil, ErrByteStreamUnsupported
	}
	t := &streamTransport{session: sess}
	t.peer, _ = sess.(engine.PeerSession)
	t.resetter, _ = sess.(interface{ ResetPeer() })
	return t, nil
}

func (p *streamTransport) Connect(ctx context.Context) error {
	if err := p.session.Connect(ctx); err != nil {
		return fmt.Errorf("session connect: %w", err)
	}
	return nil
}

func (p *streamTransport) Send(data []byte) error {
	if err := p.session.Send(data); err != nil {
		return fmt.Errorf("session send: %w", err)
	}
	return nil
}

func (p *streamTransport) SendTo(peerID string, data []byte) error {
	if p.peer == nil {
		return p.Send(data)
	}
	if err := p.peer.SendTo(peerID, data); err != nil {
		return fmt.Errorf("session send to peer: %w", err)
	}
	return nil
}

func (p *streamTransport) SupportsPeerRouting() bool {
	return p.peer != nil
}

func (p *streamTransport) Close() error {
	if err := p.session.Close(); err != nil {
		return fmt.Errorf("session close: %w", err)
	}
	return nil
}

func (p *streamTransport) ResetPeer() {
	if p.resetter != nil {
		p.resetter.ResetPeer()
	}
}

func (p *streamTransport) Reconnect(reason string) { p.session.Reconnect(reason) }

func (p *streamTransport) SetReconnectCallback(cb func()) {
	if cb == nil {
		p.session.SetReconnectCallback(nil)
		return
	}
	p.session.SetReconnectCallback(func(*webrtc.DataChannel) {
		cb()
	})
}

func (p *streamTransport) SetShouldReconnect(fn func() bool) {
	p.session.SetShouldReconnect(fn)
}

func (p *streamTransport) SetEndedCallback(cb func(string)) {
	p.session.SetEndedCallback(cb)
}

func (p *streamTransport) WatchConnection(ctx context.Context) {
	p.session.WatchConnection(ctx)
}

func (p *streamTransport) CanSend() bool {
	return p.session.CanSend()
}

func (p *streamTransport) Features() transport.Features {
	return datachannelFeatures
}
