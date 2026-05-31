package protect

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"regexp"
	"strings"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
)

const (
	defaultDialTimeout       = 10 * time.Second
	defaultKeepAlive         = 30 * time.Second
	defaultIdleConnTimeout   = 30 * time.Second
	defaultTLSHandshake      = 10 * time.Second
	defaultResponseHeader    = 10 * time.Second
	defaultWebSocketTimeout  = 10 * time.Second
	defaultHTTPClientTimeout = 30 * time.Second
	defaultStatusBodyLimit   = 1024
)

var (
	sensitiveFieldRE = regexp.MustCompile(
		`(?i)((?:access[_-]?token|room[_-]?token|token|credentials)"?\s*[:=]\s*"?)` +
			`[^",\s}]+`,
	)
	sensitiveBearerRE = regexp.MustCompile(`(?i)(bearer\s+)[A-Za-z0-9._~+/=-]+`)
)

var Protector func(fd int) bool

func controlFunc(network, _ string, c syscall.RawConn) error {
	if Protector == nil {
		return nil
	}
	var err error
	controlErr := c.Control(func(fd uintptr) {
		if !Protector(int(fd)) {
			err = &net.OpError{Op: "protect", Net: network, Err: net.ErrClosed}
		}
	})
	if controlErr != nil {
		return fmt.Errorf("control failed: %w", controlErr)
	}
	return err
}

func NewDialer() *net.Dialer {
	return &net.Dialer{
		Timeout:   defaultDialTimeout,
		KeepAlive: defaultKeepAlive,
		Control:   controlFunc,
	}
}

func NewTLSConfig() *tls.Config {
	return &tls.Config{MinVersion: tls.VersionTLS12}
}

func NewHTTPTransport() *http.Transport {
	dialer := NewDialer()
	return &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           dialer.DialContext,
		TLSClientConfig:       NewTLSConfig(),
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          10,
		IdleConnTimeout:       defaultIdleConnTimeout,
		TLSHandshakeTimeout:   defaultTLSHandshake,
		ResponseHeaderTimeout: defaultResponseHeader,
	}
}

func NewHTTPClient() *http.Client {
	return &http.Client{
		Transport: &retryTransport{base: NewHTTPTransport()},
		Timeout:   defaultHTTPClientTimeout,
	}
}

type retryTransport struct {
	base http.RoundTripper
}

func (t *retryTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	const maxRetries = 3
	ctx := req.Context()
	var resp *http.Response
	var err error
	for i := range maxRetries {
		if i > 0 {
			timer := time.NewTimer(time.Duration(i) * 500 * time.Millisecond)
			select {
			case <-ctx.Done():
				timer.Stop()
				return resp, fmt.Errorf("round trip after %d retries: %w", maxRetries, err)
			case <-timer.C:
			}
		}
		resp, err = t.base.RoundTrip(req)
		if err == nil || !isRetriableError(err) {
			if err != nil {
				return resp, fmt.Errorf("round trip: %w", err)
			}
			return resp, nil
		}
	}
	return resp, fmt.Errorf("round trip after %d retries: %w", maxRetries, err)
}

func isRetriableError(err error) bool {
	if err == nil {
		return false
	}
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return true
	}
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		return opErr.Timeout() ||
			errors.Is(opErr.Err, syscall.ECONNREFUSED) ||
			strings.Contains(opErr.Error(), "connection refused")
	}
	s := err.Error()
	return strings.Contains(s, "no such host") ||
		strings.Contains(s, "connection reset") ||
		strings.Contains(s, "i/o timeout")
}

func NewWebSocketDialer(handshakeTimeout time.Duration) websocket.Dialer {
	if handshakeTimeout <= 0 {
		handshakeTimeout = defaultWebSocketTimeout
	}
	return websocket.Dialer{
		NetDialContext:   DialContext,
		Proxy:            http.ProxyFromEnvironment,
		TLSClientConfig:  NewTLSConfig(),
		HandshakeTimeout: handshakeTimeout,
	}
}

func StatusError(base error, resp *http.Response, limit int64) error {
	if limit <= 0 {
		limit = defaultStatusBodyLimit
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, limit))
	bodyText := RedactSensitive(strings.TrimSpace(string(body)))
	if bodyText == "" {
		return fmt.Errorf("%w: status %d", base, resp.StatusCode)
	}
	return fmt.Errorf("%w: status %d: %s", base, resp.StatusCode, bodyText)
}

func RedactSensitive(text string) string {
	if text == "" {
		return ""
	}
	if strings.Contains(text, "earer") || strings.Contains(text, "EARER") {
		text = sensitiveBearerRE.ReplaceAllString(text, "${1}<redacted>")
	}
	return sensitiveFieldRE.ReplaceAllString(text, "${1}<redacted>")
}

func DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	conn, err := NewDialer().DialContext(ctx, network, address)
	if err != nil {
		return nil, fmt.Errorf("dial failed: %w", err)
	}
	return conn, nil
}

type ProxyDialer struct{}

func (d *ProxyDialer) Dial(network, addr string) (net.Conn, error) {
	conn, err := NewDialer().Dial(network, addr)
	if err != nil {
		return nil, fmt.Errorf("dial failed: %w", err)
	}
	return conn, nil
}

func NewProxyDialer() *ProxyDialer {
	return &ProxyDialer{}
}
