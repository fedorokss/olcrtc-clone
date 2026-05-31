package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

var apiBase = "https://stream.wb.ru"

func main() {
	ctx := context.Background()

	// 1. Register Guest
	u := apiBase + "/auth/api/v1/auth/user/guest-register"
	body := []byte(`{"displayName":"TestUser","device":{"deviceName":"Linux","deviceType":"PARTICIPANT_DEVICE_TYPE_WEB_DESKTOP"}}`)
	req, _ := http.NewRequestWithContext(ctx, "POST", u, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Linux x86_64)")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	fmt.Println("Guest Register:", resp.StatusCode, string(b))

	var res struct {
		AccessToken string `json:"accessToken"`
	}
	json.Unmarshal(b, &res)
	token := res.AccessToken

	// 2. Create Room
	u2 := apiBase + "/api-room/api/v2/room"
	body2 := []byte(`{"roomType":"ROOM_TYPE_ALL_ON_SCREEN","roomPrivacy":"ROOM_PRIVACY_FREE"}`)
	req2, _ := http.NewRequestWithContext(ctx, "POST", u2, bytes.NewReader(body2))
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("Authorization", "Bearer "+token)
	req2.Header.Set("User-Agent", "Mozilla/5.0 (Linux x86_64)")
	resp2, err := http.DefaultClient.Do(req2)
	if err != nil {
		panic(err)
	}
	defer resp2.Body.Close()
	b2, _ := io.ReadAll(resp2.Body)
	fmt.Println("Create Room:", resp2.StatusCode, string(b2))
}
