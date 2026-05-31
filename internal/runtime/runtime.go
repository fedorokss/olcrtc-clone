package runtime

import (
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/openlibrecommunity/olcrtc/internal/control"
	"github.com/openlibrecommunity/olcrtc/internal/crypto"
	"github.com/openlibrecommunity/olcrtc/internal/transport"
	"github.com/xtaci/smux"
)

const (
	SmuxFrameOverhead  = 8
	SmuxWireOverhead   = crypto.WireOverhead + SmuxFrameOverhead
	MinSmuxWirePayload = SmuxWireOverhead + 1
)

var ErrKeyRequired = errors.New("key required (use -key <hex>)")

var ErrKeySize = errors.New("key must be 32 bytes")

func SetupCipher(keyHex string) (*crypto.Cipher, error) {
	if keyHex == "" {
		return nil, ErrKeyRequired
	}
	key, err := hex.DecodeString(keyHex)
	if err != nil {
		return nil, fmt.Errorf("failed to decode key: %w", err)
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("%w, got %d", ErrKeySize, len(key))
	}
	cipher, err := crypto.NewCipher(string(key))
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}
	return cipher, nil
}

func SmuxConfig(maxWirePayload int) *smux.Config {
	cfg := smux.DefaultConfig()
	cfg.Version = 2
	cfg.KeepAliveDisabled = false
	cfg.MaxFrameSize = 32768
	if maxWirePayload >= MinSmuxWirePayload {
		maxFrameSize := maxWirePayload - SmuxWireOverhead
		if maxFrameSize < cfg.MaxFrameSize {
			cfg.MaxFrameSize = maxFrameSize
		}
	}
	cfg.MaxReceiveBuffer = 16 * 1024 * 1024
	cfg.MaxStreamBuffer = 1024 * 1024
	cfg.KeepAliveInterval = 10 * time.Second
	cfg.KeepAliveTimeout = 30 * time.Second
	return cfg
}

func MaxPayload(tr transport.Transport) int {
	return tr.Features().MaxPayloadSize
}

type HealthTracker struct {
	mu     sync.RWMutex
	status control.Status
	notify func(control.Status)
}

func NewHealthTracker(notify func(control.Status)) *HealthTracker {
	if notify == nil {
		notify = func(control.Status) {}
	}
	return &HealthTracker{notify: notify}
}

func (h *HealthTracker) Status() control.Status {
	if h == nil {
		return control.Status{}
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.status
}

func (h *HealthTracker) RecordSession(id string) {
	if h == nil {
		return
	}
	h.mu.Lock()
	h.status.SessionID = id
	h.status.MissedPongs = 0
	h.commit()
}

func (h *HealthTracker) RecordPong(p control.Health) {
	if h == nil {
		return
	}
	h.mu.Lock()
	h.status.LastPong = p.LastSeen
	h.status.LastRTT = p.RTT
	h.status.MissedPongs = 0
	h.commit()
}

func (h *HealthTracker) RecordMissed(missed int) {
	if h == nil {
		return
	}
	h.mu.Lock()
	h.status.MissedPongs = missed
	h.commit()
}

func (h *HealthTracker) RecordUnhealthy(missed int) {
	if h == nil {
		return
	}
	h.mu.Lock()
	h.status.MissedPongs = missed
	h.status.UnhealthyEvents++
	h.status.LastUnhealthy = time.Now()
	h.commit()
}

func (h *HealthTracker) RecordReconnect() {
	if h == nil {
		return
	}
	h.mu.Lock()
	h.status.Reconnects++
	h.commit()
}

func (h *HealthTracker) commit() {
	snapshot := h.status
	h.mu.Unlock()
	h.notify(snapshot)
}
