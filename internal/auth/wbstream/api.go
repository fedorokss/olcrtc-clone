package wbstream

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sync"

	"github.com/openlibrecommunity/olcrtc/internal/protect"
)

const defaultWSURL = "wss://rtc-el-02.wb.ru"

var apiBase = "https://stream.wb.ru" //nolint:gochecknoglobals

var (
	errGuestRegister = errors.New("guest register failed")
	errJoinRoom      = errors.New("join room failed")
	errGetToken      = errors.New("get token failed")
)

var (
	sharedClient  = sync.OnceValue(protect.NewHTTPClient)
	emptyJSONBody = []byte("{}")
)

type guestRegisterRequest struct {
	DisplayName string `json:"displayName"`
	Device      device `json:"device"`
}

type device struct {
	DeviceName string `json:"deviceName"`
	DeviceType string `json:"deviceType"`
}

type guestRegisterResponse struct {
	AccessToken string `json:"accessToken"`
}

type tokenResponse struct {
	RoomToken string `json:"roomToken"`
	ServerURL string `json:"serverUrl"`
}

func registerGuest(ctx context.Context, displayName string) (string, error) {
	u := apiBase + "/auth/api/v1/auth/user/guest-register"
	body, err := json.Marshal(guestRegisterRequest{
		DisplayName: displayName,
		Device: device{
			DeviceName: "Linux",
			DeviceType: "PARTICIPANT_DEVICE_TYPE_WEB_DESKTOP",
		},
	})
	if err != nil {
		return "", fmt.Errorf("marshal request body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Linux x86_64)")

	resp, err := sharedClient().Do(req)
	if err != nil {
		return "", fmt.Errorf("do request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("guest register status: %w", protect.StatusError(errGuestRegister, resp, 4096))
	}

	var res guestRegisterResponse
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return "", fmt.Errorf("decode response: %w", err)
	}
	return res.AccessToken, nil
}

func joinRoom(ctx context.Context, accessToken, roomID string) error {
	u := apiBase + "/api-room/api/v1/room/" + roomID + "/join"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(emptyJSONBody))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Linux x86_64)")

	resp, err := sharedClient().Do(req)
	if err != nil {
		return fmt.Errorf("do request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("join room status: %w", protect.StatusError(errJoinRoom, resp, 4096))
	}
	return nil
}

func getToken(ctx context.Context, accessToken, roomID, displayName string) (tokenResponse, error) {
	u := apiBase + "/api-room-manager/v2/room/" + roomID + "/connection-details"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return tokenResponse{}, fmt.Errorf("create request: %w", err)
	}
	req.URL.RawQuery = url.Values{
		"deviceType":  {"PARTICIPANT_DEVICE_TYPE_WEB_DESKTOP"},
		"displayName": {displayName},
	}.Encode()
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Linux x86_64)")

	resp, err := sharedClient().Do(req)
	if err != nil {
		return tokenResponse{}, fmt.Errorf("do request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return tokenResponse{}, fmt.Errorf("get token status: %w", protect.StatusError(errGetToken, resp, 4096))
	}

	var res tokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return tokenResponse{}, fmt.Errorf("decode response: %w", err)
	}
	return res, nil
}

func createRoom(ctx context.Context, wbToken, wbCookie string) (string, error) {
	body := []byte(`{"roomType":"ROOM_TYPE_ALL_ON_SCREEN","roomPrivacy":"ROOM_PRIVACY_FREE"}`)
	u := apiBase + "/api-room/api/v2/room"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+wbToken)
	if wbCookie != "" {
		req.Header.Set("Cookie", wbCookie)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Linux x86_64)")

	resp, err := sharedClient().Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("create room status %d", resp.StatusCode)
	}
	var r struct {
		RoomID string `json:"roomId"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return "", err
	}
	return r.RoomID, nil
}

func joinRoomKeeper(ctx context.Context, wbToken, wbCookie, roomID string) error {
	u := apiBase + "/api-room/api/v1/room/" + roomID + "/join"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(emptyJSONBody))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+wbToken)
	if wbCookie != "" {
		req.Header.Set("Cookie", wbCookie)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Linux x86_64)")

	resp, err := sharedClient().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("join room status %d", resp.StatusCode)
	}
	return nil
}

func warmupKeeper(ctx context.Context, wbToken, wbCookie, roomID string) {
	doGet := func(path string) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiBase+path, nil)
		if err != nil {
			return
		}
		req.Header.Set("Authorization", "Bearer "+wbToken)
		if wbCookie != "" {
			req.Header.Set("Cookie", wbCookie)
		}
		req.Header.Set("User-Agent", "Mozilla/5.0 (Linux x86_64)")
		resp, err := sharedClient().Do(req)
		if err == nil {
			resp.Body.Close()
		}
	}
	doGet("/api-room/api/v1/room/" + roomID)
	doGet("/api-chat/api/v1/connection-token")
}

func getTokenKeeper(ctx context.Context, wbToken, wbCookie, roomID, displayName string) (string, string, error) {
	u := apiBase + "/api-room-manager/v2/room/" + roomID + "/connection-details"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return "", "", err
	}
	req.URL.RawQuery = url.Values{
		"deviceType":  {"PARTICIPANT_DEVICE_TYPE_WEB_DESKTOP"},
		"displayName": {displayName},
	}.Encode()
	req.Header.Set("Authorization", "Bearer "+wbToken)
	if wbCookie != "" {
		req.Header.Set("Cookie", wbCookie)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Linux x86_64)")

	resp, err := sharedClient().Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("get token status %d", resp.StatusCode)
	}

	var res tokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return "", "", err
	}
	return res.RoomToken, res.ServerURL, nil
}
