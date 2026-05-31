package wbstream

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/fedorokss/olcrtc-clone/internal/auth"
	lksdk "github.com/livekit/server-sdk-go/v2"
)

type Provider struct{}

func (Provider) Engine() string { return "livekit" }

func (Provider) DefaultServiceURL() string { return "https://stream.wb.ru" }

func (Provider) Issue(ctx context.Context, cfg auth.Config) (auth.Credentials, error) {
	roomID := cfg.RoomURL
	if roomID == "" || roomID == "any" {
		return auth.Credentials{}, auth.ErrRoomIDRequired
	}

	accessToken, err := registerGuest(ctx, cfg.Name)
	if err != nil {
		return auth.Credentials{}, fmt.Errorf("register guest: %w", err)
	}

	if err := joinRoom(ctx, accessToken, roomID); err != nil {
		return auth.Credentials{}, fmt.Errorf("join room: %w", err)
	}

	tok, err := getToken(ctx, accessToken, roomID, cfg.Name)
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

func (Provider) CreateRoom(ctx context.Context, cfg auth.Config) (string, error) {
	if cfg.WBToken == "" {
		return "", fmt.Errorf("wb_token required for room creation")
	}
	return createRoom(ctx, cfg.WBToken, cfg.WBCookie)
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

	connect := func() *lksdk.Room {
		// KeepAlive logic using the token
		if err := joinRoomKeeper(ctx, cfg.WBToken, cfg.WBCookie, cfg.RoomURL); err != nil {
			log.Printf("Keeper: join error: %v", err)
		}
		warmupKeeper(ctx, cfg.WBToken, cfg.WBCookie, cfg.RoomURL)

		tok, srv, err := getTokenKeeper(ctx, cfg.WBToken, cfg.WBCookie, cfg.RoomURL, name)
		if err != nil {
			log.Printf("Keeper: getToken error: %v", err)
			return nil
		}

		if srv == "" {
			srv = defaultWSURL
		}

		cb := &lksdk.RoomCallback{
			OnDisconnected: func() { log.Println("Keeper: disconnected") },
			OnReconnected:  func() { log.Println("Keeper: reconnected") },
		}

		room, err := lksdk.ConnectToRoomWithToken(srv, tok, cb, lksdk.WithAutoSubscribe(false))
		if err != nil {
			log.Printf("Keeper: livekit connect error: %v", err)
			return nil
		}
		log.Printf("Keeper: connected to %s as %s", room.Name(), room.LocalParticipant.Identity())
		return room
	}

	room := connect()
	for {
		select {
		case <-ctx.Done():
			log.Println("Keeper: shutting down")
			if room != nil {
				room.Disconnect()
			}
			return
		case <-refresh.C:
			if room != nil {
				room.Disconnect()
			}
			room = connect()
		}
	}
}

func init() {
	auth.Register("wbstream", Provider{})
}
