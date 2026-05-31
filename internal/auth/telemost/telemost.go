package telemost

import (
	"context"
	"fmt"
	"strings"

	"github.com/openlibrecommunity/olcrtc/internal/auth"
)

const (
	roomURLPrefix     = "https://telemost.yandex.ru/j/"
	defaultServiceURL = "https://telemost.yandex.ru"
	httpsPrefix       = "https://"
)

type Provider struct{}

func (Provider) Engine() string { return "goolom" }

func (Provider) DefaultServiceURL() string { return defaultServiceURL }

func (Provider) Issue(ctx context.Context, cfg auth.Config) (auth.Credentials, error) {
	roomURL := cfg.RoomURL
	if roomURL == "" {
		return auth.Credentials{}, auth.ErrRoomIDRequired
	}

	if len(roomURL) < len(httpsPrefix) || !strings.HasPrefix(roomURL, httpsPrefix) {
		var sb strings.Builder
		sb.Grow(len(roomURLPrefix) + len(roomURL))
		sb.WriteString(roomURLPrefix)
		sb.WriteString(roomURL)
		roomURL = sb.String()
	}

	info, err := GetConnectionInfo(ctx, roomURL, cfg.Name)
	if err != nil {
		return auth.Credentials{}, fmt.Errorf("get connection info: %w", err)
	}

	extra := make(map[string]string, 4)
	extra["roomID"] = info.RoomID
	extra["credentials"] = info.Credentials
	extra["roomURL"] = roomURL
	extra["telemetryReferer"] = roomURL

	return auth.Credentials{
		URL:   info.ClientConfig.MediaServerURL,
		Token: info.PeerID,
		Extra: extra,
	}, nil
}

func init() {
	auth.Register("telemost", Provider{})
}
