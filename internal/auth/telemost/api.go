package telemost

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"github.com/fedorokss/olcrtc-clone/internal/protect"
	"github.com/google/uuid"
)

var apiBase = "https://cloud-api.yandex.ru/telemost_front/v2/telemost"

var ErrAPI = errors.New("api error")

type ConnectionInfo struct {
	RoomID       string `json:"room_id"`
	PeerID       string `json:"peer_id"`
	Credentials  string `json:"credentials"`
	ClientConfig struct {
		MediaServerURL string `json:"media_server_url"`
	} `json:"client_configuration"`
}

var (
	httpClientOnce sync.Once
	httpClient     *http.Client
)

func sharedClient() *http.Client {
	httpClientOnce.Do(func() {
		httpClient = protect.NewHTTPClient()
	})
	return httpClient
}

var baseHeader = http.Header{
	"User-Agent":                []string{"Mozilla/5.0 (X11; Linux x86_64; rv:149.0) Gecko/20100101 Firefox/149.0"},
	"Accept":                    []string{"*/*"},
	"Content-Type":              []string{"application/json"},
	"X-Telemost-Client-Version": []string{"187.1.0"},
	"Origin":                    []string{"https://telemost.yandex.ru"},
	"Referer":                   []string{"https://telemost.yandex.ru/"},
}

func GetConnectionInfo(ctx context.Context, roomURL, displayName string) (*ConnectionInfo, error) {
	escRoom := url.QueryEscape(roomURL)
	escName := url.QueryEscape(displayName)

	const pathMid = "/conferences/"
	const querySuffix = "/connection?next_gen_media_platform_allowed=true&waiting_room_supported=true&display_name="

	var sb strings.Builder
	sb.Grow(len(apiBase) + len(pathMid) + len(escRoom) + len(querySuffix) + len(escName))
	sb.WriteString(apiBase)
	sb.WriteString(pathMid)
	sb.WriteString(escRoom)
	sb.WriteString(querySuffix)
	sb.WriteString(escName)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, sb.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	h := make(http.Header, len(baseHeader)+2)
	for k, v := range baseHeader {
		h[k] = v
	}
	h["Client-Instance-Id"] = []string{uuid.NewString()}
	h["Idempotency-Key"] = []string{uuid.NewString()}
	req.Header = h

	resp, err := sharedClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to do request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("telemost api status: %w", protect.StatusError(ErrAPI, resp, 4096))
	}

	var info ConnectionInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}
	return &info, nil
}
