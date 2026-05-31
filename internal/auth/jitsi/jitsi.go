package jitsi

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/fedorokss/olcrtc-clone/internal/auth"
)

const CredentialKeyRoom = "room"

var ErrInvalidRoomURL = errors.New("jitsi: invalid room URL (expected host/room or https://host/room)")

type Provider struct{}

func (Provider) Engine() string { return "jitsi" }

const defaultServiceURL = "https://meet1.arbitr.ru"

func (Provider) DefaultServiceURL() string { return defaultServiceURL }

func (Provider) Issue(_ context.Context, cfg auth.Config) (auth.Credentials, error) {
	host, room, err := parseRoomURL(cfg.RoomURL)
	if err != nil {
		return auth.Credentials{}, err
	}
	return auth.Credentials{
		URL:   host,
		Token: "",
		Extra: map[string]string{CredentialKeyRoom: room},
	}, nil
}

func parseRoomURL(raw string) (string, string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", "", auth.ErrRoomIDRequired
	}
	if idx := strings.Index(raw, "://"); idx >= 0 {
		raw = raw[idx+3:]
	}
	raw = strings.TrimLeft(raw, "/")
	slash := strings.IndexByte(raw, '/')
	if slash <= 0 {
		return "", "", fmt.Errorf("%w: %q", ErrInvalidRoomURL, raw)
	}
	host := strings.TrimSpace(raw[:slash])
	room := strings.Trim(raw[slash+1:], "/")
	if host == "" || room == "" {
		return "", "", fmt.Errorf("%w: %q", ErrInvalidRoomURL, raw)
	}
	return host, room, nil
}

func init() {
	auth.Register("jitsi", Provider{})
}
