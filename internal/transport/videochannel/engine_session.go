package videochannel

import (
	"context"
	"fmt"

	"github.com/fedorokss/olcrtc-clone/internal/engine"
	"github.com/pion/webrtc/v4"
)

type engineVideoSession struct {
	session engine.Session
	vt      engine.VideoTrackCapable
}

func (v *engineVideoSession) Connect(ctx context.Context) error {
	if err := v.session.Connect(ctx); err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	return nil
}

func (v *engineVideoSession) Close() error {
	if err := v.session.Close(); err != nil {
		return fmt.Errorf("close: %w", err)
	}
	return nil
}

func (v *engineVideoSession) SetReconnectCallback(cb func()) {
	if cb == nil {
		v.session.SetReconnectCallback(nil)
		return
	}
	v.session.SetReconnectCallback(func(*webrtc.DataChannel) { cb() })
}

func (v *engineVideoSession) SetShouldReconnect(fn func() bool) {
	v.session.SetShouldReconnect(fn)
}

func (v *engineVideoSession) SetEndedCallback(cb func(string)) {
	v.session.SetEndedCallback(cb)
}

func (v *engineVideoSession) WatchConnection(ctx context.Context) {
	v.session.WatchConnection(ctx)
}

func (v *engineVideoSession) CanSend() bool {
	return v.session.CanSend()
}

func (v *engineVideoSession) Reconnect(reason string) {
	v.session.Reconnect(reason)
}

func (v *engineVideoSession) AddTrack(track webrtc.TrackLocal) error {
	if err := v.vt.AddVideoTrack(track); err != nil {
		return fmt.Errorf("add track: %w", err)
	}
	return nil
}

func (v *engineVideoSession) SetTrackHandler(cb func(*webrtc.TrackRemote, *webrtc.RTPReceiver)) {
	v.vt.SetVideoTrackHandler(cb)
}
