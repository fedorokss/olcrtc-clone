package wbstream

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/fedorokss/olcrtc-clone/internal/auth"
)

type Provider struct{}

func (Provider) Engine() string { return "livekit" }

func (Provider) DefaultServiceURL() string { return "https://stream.wb.ru" }

func (Provider) Issue(ctx context.Context, cfg auth.Config) (auth.Credentials, error) {
	roomID := cfg.RoomURL
	if roomID == "" || roomID == "any" {
		return auth.Credentials{}, auth.ErrRoomIDRequired
	}

	var accessToken string
	var err error
	if cfg.WBToken != "" {
		accessToken = cfg.WBToken
	} else {
		accessToken, err = registerGuest(ctx, cfg.Name)
		if err != nil {
			return auth.Credentials{}, fmt.Errorf("register guest: %w", err)
		}
	}

	if err := joinRoom(ctx, accessToken, cfg.WBCookie, roomID); err != nil {
		return auth.Credentials{}, fmt.Errorf("join room: %w", err)
	}

	tok, err := getToken(ctx, accessToken, cfg.WBCookie, roomID, cfg.Name)
	if err != nil {
		return auth.Credentials{}, fmt.Errorf("get token: %w", err)
	}

	url := tok.ServerURL
	if url == "" {
		url = defaultWSURL
	}

	return auth.Credentials{
		URL:   url,
		Token: tok.RoomToken,
		Extra: map[string]string{"roomID": roomID},
	}, nil
}

func (Provider) CreateRoom(ctx context.Context, cfg auth.Config) (string, string, error) {
	return createRoom(ctx, cfg.WBToken, cfg.WBCookie, cfg.Name)
}

func (Provider) KeepAlive(ctx context.Context, cfg auth.Config) {
	if cfg.WBToken == "" || cfg.RoomURL == "" {
		return
	}

	name := cfg.Name
	if name == "" {
		name = "Keeper"
	}

	refresh := time.NewTicker(2 * time.Minute)
	defer refresh.Stop()

	keepalive := func() {
		if err := joinRoomKeeper(ctx, cfg.WBToken, cfg.WBCookie, cfg.RoomURL); err != nil {
			log.Printf("Keeper: join error: %v", err)
		}
		warmupKeeper(ctx, cfg.WBToken, cfg.WBCookie, cfg.RoomURL)
		if _, _, err := getTokenKeeper(ctx, cfg.WBToken, cfg.WBCookie, cfg.RoomURL, name); err != nil {
			log.Printf("Keeper: getToken error: %v", err)
		}
	}

	keepalive()
	for {
		select {
		case <-ctx.Done():
			log.Println("Keeper: shutting down")
			return
		case <-refresh.C:
			keepalive()
		}
	}
}

func init() {
	auth.Register("wbstream", Provider{})
}
