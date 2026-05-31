package transport

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"sync"
	"time"
)

var ErrTrafficPayloadTooLarge = errors.New("traffic payload exceeds max_payload_size")

var (
	errTrafficConnect = errors.New("traffic connect failed")
	errTrafficSend    = errors.New("traffic send failed")
	errTrafficClose   = errors.New("traffic close failed")
)

type trafficTransport struct {
	inner          Transport
	peer           PeerTransport
	isPeer         bool
	maxPayloadSize int
	minDelay       time.Duration
	delaySpan      int64
	paced          bool
	sendMu         sync.Mutex
}

func WithTraffic(tr Transport, cfg TrafficConfig) Transport {
	if tr == nil {
		return nil
	}
	cfg = effectiveTrafficConfig(tr.Features(), cfg)
	if cfg.MaxPayloadSize <= 0 && cfg.MinDelay <= 0 && cfg.MaxDelay <= 0 {
		return tr
	}
	peer, isPeer := tr.(PeerTransport)
	t := &trafficTransport{
		inner:          tr,
		peer:           peer,
		isPeer:         isPeer,
		maxPayloadSize: cfg.MaxPayloadSize,
		minDelay:       cfg.MinDelay,
		paced:          cfg.MinDelay > 0 || cfg.MaxDelay > 0,
	}
	if cfg.MaxDelay > cfg.MinDelay {
		t.delaySpan = int64(cfg.MaxDelay - cfg.MinDelay)
	}
	return t
}

func effectiveTrafficConfig(features Features, cfg TrafficConfig) TrafficConfig {
	if cfg.MaxPayloadSize > 0 && features.MaxPayloadSize > 0 && features.MaxPayloadSize < cfg.MaxPayloadSize {
		cfg.MaxPayloadSize = features.MaxPayloadSize
	}
	return cfg
}

func (t *trafficTransport) Connect(ctx context.Context) error {
	if err := t.inner.Connect(ctx); err != nil {
		return fmt.Errorf("%w: %w", errTrafficConnect, err)
	}
	return nil
}

func (t *trafficTransport) Send(data []byte) error {
	return t.pacedSend("", false, data)
}

func (t *trafficTransport) SendTo(peerID string, data []byte) error {
	if !t.isPeer || !t.peer.SupportsPeerRouting() {
		return t.pacedSend("", false, data)
	}
	return t.pacedSend(peerID, true, data)
}

func (t *trafficTransport) SupportsPeerRouting() bool {
	return t.isPeer && t.peer.SupportsPeerRouting()
}

func (t *trafficTransport) pacedSend(peerID string, toPeer bool, data []byte) error {
	if t.maxPayloadSize > 0 && len(data) > t.maxPayloadSize {
		return fmt.Errorf("%w: size=%d max=%d", ErrTrafficPayloadTooLarge, len(data), t.maxPayloadSize)
	}
	t.sendMu.Lock()
	if t.paced {
		if delay := t.nextDelay(); delay > 0 {
			time.Sleep(delay)
		}
	}
	var err error
	if toPeer {
		err = t.peer.SendTo(peerID, data)
	} else {
		err = t.inner.Send(data)
	}
	t.sendMu.Unlock()
	if err != nil {
		return fmt.Errorf("%w: %w", errTrafficSend, err)
	}
	return nil
}

func (t *trafficTransport) Close() error {
	if err := t.inner.Close(); err != nil {
		return fmt.Errorf("%w: %w", errTrafficClose, err)
	}
	return nil
}

func (t *trafficTransport) ResetPeer() {
	if resetter, ok := t.inner.(interface{ ResetPeer() }); ok {
		resetter.ResetPeer()
	}
}

func (t *trafficTransport) Reconnect(reason string)             { t.inner.Reconnect(reason) }
func (t *trafficTransport) SetReconnectCallback(cb func())      { t.inner.SetReconnectCallback(cb) }
func (t *trafficTransport) SetShouldReconnect(fn func() bool)   { t.inner.SetShouldReconnect(fn) }
func (t *trafficTransport) SetEndedCallback(cb func(string))    { t.inner.SetEndedCallback(cb) }
func (t *trafficTransport) WatchConnection(ctx context.Context) { t.inner.WatchConnection(ctx) }
func (t *trafficTransport) CanSend() bool                       { return t.inner.CanSend() }

func (t *trafficTransport) Features() Features {
	features := t.inner.Features()
	if t.maxPayloadSize > 0 &&
		(features.MaxPayloadSize == 0 || t.maxPayloadSize < features.MaxPayloadSize) {
		features.MaxPayloadSize = t.maxPayloadSize
	}
	return features
}

func (t *trafficTransport) nextDelay() time.Duration {
	if t.delaySpan <= 0 {
		return t.minDelay
	}
	return t.minDelay + time.Duration(rand.Int64N(t.delaySpan))
}
