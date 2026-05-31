package mobile

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/openlibrecommunity/olcrtc/internal/app/session"
	"github.com/openlibrecommunity/olcrtc/internal/client"
	"github.com/openlibrecommunity/olcrtc/internal/control"
	"github.com/openlibrecommunity/olcrtc/internal/logger"
	"github.com/openlibrecommunity/olcrtc/internal/protect"
	"github.com/openlibrecommunity/olcrtc/internal/transport/vp8channel"
	_ "golang.org/x/mobile/bind"
	_ "google.golang.org/genproto/protobuf/field_mask"
)

type SocketProtector interface {
	Protect(fd int) bool
}

type LogWriter interface {
	WriteLog(msg string)
}

var (
	errAlreadyRunning       = errors.New("olcRTC already running")
	errCarrierRequired      = errors.New("carrier is required")
	errRoomIDRequired       = errors.New("roomID is required")
	errClientIDRequired     = errors.New("clientID is required")
	errKeyHexRequired       = errors.New("keyHex is required")
	errNotRunning           = errors.New("olcRTC is not running")
	errStoppedBeforeReady   = errors.New("olcRTC stopped before becoming ready")
	errStartTimedOut        = errors.New("olcRTC start timed out")
	errHTTPPingTimedOut     = errors.New("HTTP ping timed out")
	errUnexpectedHTTPStatus = errors.New("unexpected HTTP status")
)

const (
	defaultTransport   = "vp8channel"
	dataTransport      = "datachannel"
	defaultDNSServer   = "8.8.8.8:53"
	defaultHTTPPingURL = "https://www.google.com/generate_204"
	defaultSocksHost   = "127.0.0.1"
	carrierWBStream    = "wbstream"
)

const (
	httpPingWarmupTimeout = 1500 * time.Millisecond
	httpPingSampleTimeout = 1500 * time.Millisecond
	httpPingSamples       = 3
	httpPingSampleDelay   = 80 * time.Millisecond
)

var (
	mu                 sync.Mutex
	defaults           mobileConfig
	defaultsSet        sync.Once
	registerSet        sync.Once
	runClientWithReady = client.RunWithReady
	cancel             context.CancelFunc
	done               chan struct{}
	ready              chan struct{}
	errRun             error
)

type mobileConfig struct {
	transport        string
	dnsServer        string
	socksListenHost  string
	vp8FPS           int
	vp8BatchSize     int
	livenessInterval time.Duration
	livenessTimeout  time.Duration
	livenessFailures int
}

func SetProtector(p SocketProtector) {
	if p == nil {
		protect.Protector = nil
		return
	}
	protect.Protector = func(fd int) bool {
		return p.Protect(fd)
	}
}

func SetLogWriter(w LogWriter) {
	if w != nil {
		log.SetOutput(&logBridge{w: w})
	}
}

func SetProviders() {
	registerDefaults()
}

func SetTransport(transport string) {
	mu.Lock()
	defer mu.Unlock()
	ensureDefaultConfigLocked()
	defaults.transport = normalizeTransport(transport)
}

func SetDNS(dnsServer string) {
	mu.Lock()
	defer mu.Unlock()
	ensureDefaultConfigLocked()
	defaults.dnsServer = dnsServer
}

func SetSocksListenHost(host string) {
	mu.Lock()
	defer mu.Unlock()
	ensureDefaultConfigLocked()
	defaults.socksListenHost = normalizeSocksListenHost(host)
}

func SetVP8Options(fps, batchSize int) {
	mu.Lock()
	defer mu.Unlock()
	ensureDefaultConfigLocked()
	defaults.vp8FPS = clampAtLeastOne(fps, 120)
	defaults.vp8BatchSize = clampAtLeastOne(batchSize, 64)
}

func SetLivenessOptions(intervalMillis, timeoutMillis, failures int) {
	mu.Lock()
	defer mu.Unlock()
	ensureDefaultConfigLocked()
	defaults.livenessInterval = durationFromMillisOrDefault(intervalMillis, control.DefaultInterval)
	defaults.livenessTimeout = durationFromMillisOrDefault(timeoutMillis, control.DefaultTimeout)
	if failures <= 0 {
		defaults.livenessFailures = control.DefaultFailures
		return
	}
	defaults.livenessFailures = failures
}

func SetDebug(enabled bool) {
	logger.SetVerbose(enabled)
	if enabled {
		log.SetFlags(log.Ltime | log.Lshortfile)
		return
	}
	log.SetFlags(log.Ltime)
}

func Start(carrierName, roomID, clientID, keyHex string, socksPort int, socksUser, socksPass string) error {
	mu.Lock()
	ensureDefaultConfigLocked()
	cfg := defaults
	mu.Unlock()
	return startWithConfig(carrierName, cfg.transport, roomID, clientID, keyHex, socksPort, socksUser, socksPass, cfg)
}

func StartWithTransport(
	carrierName, transportName, roomID, clientID, keyHex string,
	socksPort int,
	socksUser, socksPass string,
) error {
	mu.Lock()
	ensureDefaultConfigLocked()
	cfg := defaults
	cfg.transport = transportName
	mu.Unlock()
	return startWithConfig(carrierName, transportName, roomID, clientID, keyHex, socksPort, socksUser, socksPass, cfg)
}

func Check(
	carrierName, transportName, roomID, clientID, keyHex string,
	socksPort int,
	timeoutMillis int,
	vp8FPS int,
	vp8BatchSize int,
) (int64, error) {
	registerDefaults()
	mu.Lock()
	ensureDefaultConfigLocked()
	cfg := defaults
	mu.Unlock()
	carrierName = normalizeCarrier(carrierName)
	transportName = normalizeTransport(transportName)
	if err := validateStartArgs(carrierName, roomID, clientID, keyHex); err != nil {
		return 0, err
	}
	if timeoutMillis <= 0 {
		timeoutMillis = 8000
	}
	ctx, cancelFunc := context.WithCancel(context.Background())
	defer cancelFunc()
	startedAt := time.Now()
	readyCh, doneCh := runIsolatedClient(
		ctx,
		isolatedClientConfig(cfg, transportName, carrierName, roomID, clientID, keyHex, socksPort, vp8FPS, vp8BatchSize),
	)
	timer := time.NewTimer(time.Duration(timeoutMillis) * time.Millisecond)
	defer timer.Stop()
	select {
	case <-readyCh:
		elapsed := time.Since(startedAt).Milliseconds()
		cancelFunc()
		waitForCheckDone(doneCh)
		return elapsed, nil
	case err := <-doneCh:
		if err != nil {
			return 0, err
		}
		return 0, errStoppedBeforeReady
	case <-timer.C:
		cancelFunc()
		waitForCheckDone(doneCh)
		return 0, errStartTimedOut
	}
}

func Ping(
	carrierName, transportName, roomID, clientID, keyHex string,
	socksPort int,
	timeoutMillis int,
	pingURL string,
	vp8FPS int,
	vp8BatchSize int,
) (int64, error) {
	registerDefaults()
	mu.Lock()
	ensureDefaultConfigLocked()
	cfg := defaults
	mu.Unlock()
	carrierName = normalizeCarrier(carrierName)
	transportName = normalizeTransport(transportName)
	if err := validateStartArgs(carrierName, roomID, clientID, keyHex); err != nil {
		return 0, err
	}
	if timeoutMillis <= 0 {
		timeoutMillis = 10000
	}
	if pingURL == "" {
		pingURL = defaultHTTPPingURL
	}
	ctx, cancelFunc := context.WithTimeout(
		context.Background(),
		time.Duration(timeoutMillis)*time.Millisecond,
	)
	defer cancelFunc()
	readyCh, doneCh := runIsolatedClient(
		ctx,
		isolatedClientConfig(cfg, transportName, carrierName, roomID, clientID, keyHex, socksPort, vp8FPS, vp8BatchSize),
	)
	select {
	case <-readyCh:
		elapsed, err := httpPingThroughSocks(
			ctx,
			socksDialAddr(cfg.socksListenHost, socksPort),
			pingURL,
		)
		cancelFunc()
		waitForCheckDone(doneCh)
		if err != nil {
			return 0, err
		}
		return elapsed, nil
	case err := <-doneCh:
		if err != nil {
			return 0, err
		}
		return 0, errStoppedBeforeReady
	case <-ctx.Done():
		cancelFunc()
		waitForCheckDone(doneCh)
		return 0, errStartTimedOut
	}
}

func runIsolatedClient(ctx context.Context, cfg client.Config) (<-chan struct{}, <-chan error) {
	readyCh := make(chan struct{})
	doneCh := make(chan error, 1)
	var readyOnce sync.Once
	go func() {
		doneCh <- runClientWithReady(ctx, cfg, func() {
			readyOnce.Do(func() {
				close(readyCh)
			})
		})
	}()
	return readyCh, doneCh
}

func isolatedClientConfig(
	cfg mobileConfig,
	transportName, carrierName, roomID, clientID, keyHex string,
	socksPort, vp8FPS, vp8BatchSize int,
) client.Config {
	return client.Config{
		Transport: transportName,
		Carrier:   carrierName,
		RoomURL:   buildRoomURL(carrierName, roomID),
		KeyHex:    keyHex,
		DeviceID:  clientID,
		LocalAddr: socksListenAddr(cfg.socksListenHost, socksPort),
		DNSServer: defaultDNSServer,
		TransportOptions: vp8channel.Options{
			FPS:       clampAtLeastOne(vp8FPS, 120),
			BatchSize: clampAtLeastOne(vp8BatchSize, 64),
		},
		Liveness: livenessConfig(cfg),
	}
}

func httpPingThroughSocks(
	parentCtx context.Context,
	socksAddr string,
	targetURL string,
) (int64, error) {
	normalizedURL, err := normalizeHTTPPingURL(targetURL)
	if err != nil {
		return 0, err
	}
	client, closeClient := newHTTPPingClient(socksAddr)
	defer closeClient()
	_, _ = singleHTTPPingRequest(
		parentCtx,
		client,
		normalizedURL,
		httpPingWarmupTimeout,
	)
	return bestHTTPPingSample(parentCtx, client, normalizedURL)
}

func normalizeHTTPPingURL(targetURL string) (string, error) {
	if targetURL == "" {
		targetURL = defaultHTTPPingURL
	}
	if _, err := url.ParseRequestURI(targetURL); err != nil {
		return "", fmt.Errorf("parse HTTP ping URL: %w", err)
	}
	return targetURL, nil
}

func newHTTPPingClient(socksAddr string) (*http.Client, func()) {
	proxyURL := &url.URL{
		Scheme: "socks5",
		Host:   socksAddr,
	}
	transport := &http.Transport{
		Proxy:                 http.ProxyURL(proxyURL),
		DisableKeepAlives:     false,
		MaxIdleConns:          4,
		MaxIdleConnsPerHost:   4,
		IdleConnTimeout:       10 * time.Second,
		ForceAttemptHTTP2:     false,
		TLSHandshakeTimeout:   httpPingSampleTimeout,
		ResponseHeaderTimeout: httpPingSampleTimeout,
		ExpectContinueTimeout: 500 * time.Millisecond,
	}
	client := &http.Client{
		Transport: transport,
		Timeout:   httpPingSampleTimeout,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	return client, transport.CloseIdleConnections
}

func bestHTTPPingSample(
	parentCtx context.Context,
	client *http.Client,
	targetURL string,
) (int64, error) {
	var best int64
	var lastErr error
	for i := range httpPingSamples {
		elapsed, err := singleHTTPPingRequest(
			parentCtx,
			client,
			targetURL,
			httpPingSampleTimeout,
		)
		if err != nil {
			lastErr = err
		} else {
			best = bestPositiveLatency(best, elapsed)
		}
		if i < httpPingSamples-1 {
			select {
			case <-parentCtx.Done():
				if best > 0 {
					return best, nil
				}
				if lastErr != nil {
					return 0, lastErr
				}
				return 0, errHTTPPingTimedOut
			case <-time.After(httpPingSampleDelay):
			}
		}
	}
	if best > 0 {
		return best, nil
	}
	if lastErr != nil {
		return 0, lastErr
	}
	return 0, errHTTPPingTimedOut
}

func bestPositiveLatency(currentBest, next int64) int64 {
	if next <= 0 {
		return currentBest
	}
	if currentBest == 0 || next < currentBest {
		return next
	}
	return currentBest
}

func singleHTTPPingRequest(
	parentCtx context.Context,
	client *http.Client,
	targetURL string,
	timeout time.Duration,
) (int64, error) {
	ctx, cancel := context.WithTimeout(parentCtx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		return 0, fmt.Errorf("create HTTP ping request: %w", err)
	}
	req.Header.Set("User-Agent", "Olcbox-Android")
	req.Header.Set("Connection", "keep-alive")
	req.Header.Set("Cache-Control", "no-cache")
	startedAt := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("perform HTTP ping request: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	elapsed := time.Since(startedAt).Milliseconds()
	_, _ = io.Copy(io.Discard, resp.Body)
	if resp.StatusCode < http.StatusOK || resp.StatusCode > http.StatusPermanentRedirect {
		return 0, fmt.Errorf("%w: %d", errUnexpectedHTTPStatus, resp.StatusCode)
	}
	return elapsed, nil
}

func startWithConfig(
	carrierName, transportName, roomID, clientID, keyHex string,
	socksPort int,
	socksUser, socksPass string,
	cfg mobileConfig,
) error {
	mu.Lock()
	defer mu.Unlock()
	registerDefaults()
	carrierName = normalizeCarrier(carrierName)
	if transportName != "" {
		cfg.transport = normalizeTransport(transportName)
	}
	if cancel != nil {
		return errAlreadyRunning
	}
	if err := validateStartArgs(carrierName, roomID, clientID, keyHex); err != nil {
		return err
	}
	roomURL := buildRoomURL(carrierName, roomID)
	ctx, cancelFunc := context.WithCancel(context.Background())
	cancel = cancelFunc
	done = make(chan struct{})
	ready = make(chan struct{})
	localReady := ready
	localDone := done
	errRun = nil
	var readyOnce sync.Once
	clientCfg := client.Config{
		Transport: cfg.transport,
		Carrier:   carrierName,
		RoomURL:   roomURL,
		KeyHex:    keyHex,
		DeviceID:  clientID,
		LocalAddr: socksListenAddr(cfg.socksListenHost, socksPort),
		DNSServer: cfg.dnsServer,
		SOCKSUser: socksUser,
		SOCKSPass: socksPass,
		TransportOptions: vp8channel.Options{
			FPS:       cfg.vp8FPS,
			BatchSize: cfg.vp8BatchSize,
		},
		Liveness: livenessConfig(cfg),
	}
	go func() {
		defer cancelFunc()
		err := runClientWithReady(ctx, clientCfg, func() {
			readyOnce.Do(func() {
				close(localReady)
			})
		})
		mu.Lock()
		cancel = nil
		errRun = err
		mu.Unlock()
		close(localDone)
	}()
	return nil
}

func WaitReady(timeoutMillis int) error {
	mu.Lock()
	r := ready
	d := done
	runErr := errRun
	running := cancel != nil
	mu.Unlock()
	if r == nil {
		if runErr != nil {
			return runErr
		}
		return errNotRunning
	}
	select {
	case <-r:
		return nil
	default:
	}
	if !running {
		if runErr != nil {
			return runErr
		}
		return errStoppedBeforeReady
	}
	timer := time.NewTimer(time.Duration(timeoutMillis) * time.Millisecond)
	defer timer.Stop()
	select {
	case <-r:
		return nil
	case <-d:
		mu.Lock()
		runErr = errRun
		mu.Unlock()
		if runErr != nil {
			return runErr
		}
		return errStoppedBeforeReady
	case <-timer.C:
		return errStartTimedOut
	}
}

func Stop() {
	mu.Lock()
	cancelFunc := cancel
	doneCh := done
	mu.Unlock()
	if cancelFunc == nil {
		return
	}
	cancelFunc()
	if doneCh != nil {
		<-doneCh
	}
}

func IsRunning() bool {
	mu.Lock()
	defer mu.Unlock()
	return cancel != nil
}

func registerDefaults() {
	registerSet.Do(session.RegisterDefaults)
}

func waitForCheckDone(doneCh <-chan error) {
	select {
	case <-doneCh:
	case <-time.After(2 * time.Second):
	}
}

func ensureDefaultConfigLocked() {
	defaultsSet.Do(func() {
		defaults = mobileConfig{
			transport:        defaultTransport,
			dnsServer:        defaultDNSServer,
			socksListenHost:  defaultSocksHost,
			vp8FPS:           60,
			vp8BatchSize:     8,
			livenessInterval: control.DefaultInterval,
			livenessTimeout:  control.DefaultTimeout,
			livenessFailures: control.DefaultFailures,
		}
	})
}

func normalizeSocksListenHost(host string) string {
	host = strings.TrimSpace(host)
	if len(host) >= 2 && host[0] == '[' && host[len(host)-1] == ']' {
		host = host[1 : len(host)-1]
	}
	if host == "" {
		return defaultSocksHost
	}
	return host
}

func socksListenAddr(host string, port int) string {
	return net.JoinHostPort(normalizeSocksListenHost(host), strconv.Itoa(port))
}

func socksDialAddr(host string, port int) string {
	normalized := normalizeSocksListenHost(host)
	switch normalized {
	case "0.0.0.0", "::":
		normalized = defaultSocksHost
	}
	return net.JoinHostPort(normalized, strconv.Itoa(port))
}

func livenessConfig(cfg mobileConfig) control.Config {
	interval := cfg.livenessInterval
	if interval <= 0 {
		interval = control.DefaultInterval
	}
	timeout := cfg.livenessTimeout
	if timeout <= 0 {
		timeout = control.DefaultTimeout
	}
	failures := cfg.livenessFailures
	if failures <= 0 {
		failures = control.DefaultFailures
	}
	return control.Config{
		Interval: interval,
		Timeout:  timeout,
		Failures: failures,
	}
}

func normalizeTransport(value string) string {
	switch value {
	case dataTransport, "data", "dc":
		return dataTransport
	default:
		return defaultTransport
	}
}

func normalizeCarrier(carrierName string) string {
	return carrierName
}

func validateStartArgs(carrierName, roomID, clientID, keyHex string) error {
	switch {
	case carrierName == "":
		return errCarrierRequired
	case roomID == "":
		return errRoomIDRequired
	case clientID == "":
		return errClientIDRequired
	case keyHex == "":
		return errKeyHexRequired
	default:
		return nil
	}
}

func buildRoomURL(_ string, roomID string) string {
	return roomID
}

func clampAtLeastOne(value, maxValue int) int {
	if value < 1 {
		return 1
	}
	if value > maxValue {
		return maxValue
	}
	return value
}

func durationFromMillisOrDefault(value int, def time.Duration) time.Duration {
	if value <= 0 {
		return def
	}
	return time.Duration(value) * time.Millisecond
}

type logBridge struct {
	w LogWriter
}

func (b *logBridge) Write(p []byte) (int, error) {
	b.w.WriteLog(string(p))
	return len(p), nil
}
