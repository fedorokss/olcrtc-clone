package handshake

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/openlibrecommunity/olcrtc/internal/framing"
)

const ProtoVersion = 1
const MaxMessageSize = 64 * 1024
const DefaultTimeout = 15 * time.Second

type MsgType string

const (
	TypeHello   MsgType = "CLIENT_HELLO"
	TypeWelcome MsgType = "SERVER_WELCOME"
	TypeReject  MsgType = "SERVER_REJECT"
)

type Hello struct {
	Version  int            `json:"version"`
	Type     MsgType        `json:"type"`
	DeviceID string         `json:"device_id"`
	Claims   map[string]any `json:"claims,omitempty"`
}

type Welcome struct {
	Version   int     `json:"version"`
	Type      MsgType `json:"type"`
	SessionID string  `json:"session_id"`
}

type Reject struct {
	Version int     `json:"version"`
	Type    MsgType `json:"type"`
	Reason  string  `json:"reason"`
}

var (
	ErrRejected          = errors.New("handshake rejected")
	ErrProtocolVersion   = errors.New("incompatible protocol version")
	ErrUnexpectedMessage = errors.New("unexpected handshake message")
	ErrFrameTooLarge     = framing.ErrFrameTooLarge
)

type AuthFunc func(deviceID string, claims map[string]any) (sessionID string, err error)

type replyEnvelope struct {
	Version   int     `json:"version"`
	Type      MsgType `json:"type"`
	SessionID string  `json:"session_id"`
	Reason    string  `json:"reason"`
}

func Client(rw io.ReadWriter, deviceID string, claims map[string]any) (string, error) {
	if err := writeFrame(rw, Hello{
		Version:  ProtoVersion,
		Type:     TypeHello,
		DeviceID: deviceID,
		Claims:   claims,
	}); err != nil {
		return "", fmt.Errorf("send hello: %w", err)
	}

	raw, err := readFrame(rw)
	if err != nil {
		return "", fmt.Errorf("read welcome: %w", err)
	}

	var env replyEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return "", fmt.Errorf("parse reply: %w", err)
	}

	switch env.Type {
	case TypeHello:
		return "", fmt.Errorf("%w: got %q", ErrUnexpectedMessage, env.Type)
	case TypeWelcome:
		if env.Version != ProtoVersion {
			return "", fmt.Errorf("%w: server v%d, client v%d", ErrProtocolVersion, env.Version, ProtoVersion)
		}
		return env.SessionID, nil
	case TypeReject:
		return "", fmt.Errorf("%w: %s", ErrRejected, env.Reason)
	default:
		return "", fmt.Errorf("%w: got %q", ErrUnexpectedMessage, env.Type)
	}
}

func parseWelcome(raw []byte) (string, error) {
	var w Welcome
	if err := json.Unmarshal(raw, &w); err != nil {
		return "", fmt.Errorf("parse welcome: %w", err)
	}
	if w.Version != ProtoVersion {
		return "", fmt.Errorf("%w: server v%d, client v%d", ErrProtocolVersion, w.Version, ProtoVersion)
	}
	return w.SessionID, nil
}

func parseReject(raw []byte) (string, error) {
	var r Reject
	if err := json.Unmarshal(raw, &r); err != nil {
		return "", fmt.Errorf("parse reject: %w", err)
	}
	return "", fmt.Errorf("%w: %s", ErrRejected, r.Reason)
}

func Server(rw io.ReadWriter, auth AuthFunc) (Hello, string, error) {
	raw, err := readFrame(rw)
	if err != nil {
		return Hello{}, "", fmt.Errorf("read hello: %w", err)
	}

	var h Hello
	if err := json.Unmarshal(raw, &h); err != nil {
		_ = writeFrame(rw, Reject{Version: ProtoVersion, Type: TypeReject, Reason: "malformed hello"})
		return Hello{}, "", fmt.Errorf("parse hello: %w", err)
	}

	if h.Type != TypeHello {
		_ = writeFrame(rw, Reject{Version: ProtoVersion, Type: TypeReject, Reason: "expected CLIENT_HELLO"})
		return h, "", fmt.Errorf("%w: got %q", ErrUnexpectedMessage, h.Type)
	}

	if h.Version != ProtoVersion {
		_ = writeFrame(rw, Reject{Version: ProtoVersion, Type: TypeReject, Reason: "protocol version mismatch"})
		return h, "", fmt.Errorf("%w: client v%d, server v%d", ErrProtocolVersion, h.Version, ProtoVersion)
	}

	sessionID, err := auth(h.DeviceID, h.Claims)
	if err != nil {
		_ = writeFrame(rw, Reject{Version: ProtoVersion, Type: TypeReject, Reason: err.Error()})
		return h, "", fmt.Errorf("auth: %w", err)
	}

	if err := writeFrame(rw, Welcome{Version: ProtoVersion, Type: TypeWelcome, SessionID: sessionID}); err != nil {
		return h, sessionID, fmt.Errorf("send welcome: %w", err)
	}

	return h, sessionID, nil
}

func writeFrame(w io.Writer, msg any) error {
	if err := framing.WriteJSON(w, msg, MaxMessageSize); err != nil {
		return fmt.Errorf("handshake: %w", err)
	}
	return nil
}

func readFrame(r io.Reader) ([]byte, error) {
	body, err := framing.ReadBytes(r, MaxMessageSize)
	if err != nil {
		return nil, fmt.Errorf("handshake: %w", err)
	}
	return body, nil
}
