package client

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fedorokss/olcrtc-clone/internal/control"
	"github.com/fedorokss/olcrtc-clone/internal/crypto"
	"github.com/fedorokss/olcrtc-clone/internal/handshake"
	"github.com/fedorokss/olcrtc-clone/internal/logger"
	"github.com/fedorokss/olcrtc-clone/internal/muxconn"
	"github.com/fedorokss/olcrtc-clone/internal/names"
	"github.com/fedorokss/olcrtc-clone/internal/runtime"
	"github.com/fedorokss/olcrtc-clone/internal/transport"
	"github.com/google/uuid"
	"github.com/xtaci/smux"
)

var (
	ErrConnectFailed           = errors.New("tunnel connection failed")
	ErrProxyAuth               = errors.New("SOCKS proxy auth failed")
	ErrKeySize                 = runtime.ErrKeySize
	ErrInvalidSOCKSVersion     = errors.New("invalid socks version")
	ErrUnsupportedSOCKSCommand = errors.New("unsupported socks command")
	ErrUnsupportedAddressType  = errors.New("unsupported address type")
	ErrRemoteNotReady          = errors.New("remote not ready")
	ErrSOCKSAuthFailed         = errors.New("SOCKS5 authentication failed")
	ErrSOCKSCredTooLong        = errors.New("socks5 user/pass exceeds 255 bytes")
)

var (
	successReply         = []byte{5, 0, 0, 1, 0, 0, 0, 0, 0, 0}
	hostUnreachableReply = []byte{5, 4, 0, 1, 0, 0, 0, 0, 0, 0}
)

var copyBufPool = sync.Pool{New: func() any { b := make([]byte, 32*1024); return &b }}

type connectRequest struct {
	Cmd  string `json:"cmd"`
	Addr string `json:"addr"`
	Port int    `json:"port"`
}

type Client struct {
	ln          transport.Transport
	cipher      *crypto.Cipher
	conn        *muxconn.Conn
	session     *smux.Session
	controlStrm *smux.Stream
	controlStop context.CancelFunc
	sessMu      sync.RWMutex
	reconnectMu sync.Mutex
	health      *runtime.HealthTracker
	deviceID    string
	sessionID   string
	claims      map[string]any
	dnsServer   string
	socksUser   string
	socksPass   string
}

type HealthFunc func(control.Status)

type Config struct {
	Transport        string
	Carrier          string
	RoomURL          string
	ChannelID        string
	KeyHex           string
	LocalAddr        string
	DNSServer        string
	SOCKSUser        string
	SOCKSPass        string
	TransportOptions transport.Options
	Engine           string
	URL              string
	Token            string
	WBToken          string
	WBCookie         string
	Liveness         control.Config
	Traffic          transport.TrafficConfig
	DeviceID         string
	DeviceIDPath     string
	Claims           map[string]any
	OnHealth         HealthFunc
}

func Run(ctx context.Context, cfg Config) error {
	return RunWithReady(ctx, cfg, nil)
}

func RunWithReady(ctx context.Context, cfg Config, onReady func()) error {
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	cipher, err := setupCipher(cfg.KeyHex)
	if err != nil {
		return fmt.Errorf("setupCipher failed: %w", err)
	}

	deviceID, err := resolveDeviceID(cfg.DeviceID, cfg.DeviceIDPath)
	if err != nil {
		return fmt.Errorf("resolve device id: %w", err)
	}

	c := &Client{
		cipher:    cipher,
		deviceID:  deviceID,
		claims:    cfg.Claims,
		dnsServer: cfg.DNSServer,
		socksUser: cfg.SOCKSUser,
		socksPass: cfg.SOCKSPass,
		health:    runtime.NewHealthTracker(cfg.OnHealth),
	}

	defer c.shutdown()

	if err := c.bringUpLink(runCtx, cfg, cancel); err != nil {
		return err
	}

	lc := net.ListenConfig{}
	listener, err := lc.Listen(runCtx, "tcp4", cfg.LocalAddr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", cfg.LocalAddr, err)
	}
	defer func() { _ = listener.Close() }()

	logger.Infof("SOCKS5 server listening on %s", cfg.LocalAddr)
	if onReady != nil {
		onReady()
	}

	go c.acceptLoop(runCtx, listener)
	<-runCtx.Done()
	return nil
}

func (c *Client) bringUpLink(
	ctx context.Context,
	cfg Config,
	cancel context.CancelFunc,
) error {
	ln, err := transport.New(ctx, cfg.Transport, transport.Config{
		Carrier:   cfg.Carrier,
		RoomURL:   cfg.RoomURL,
		Engine:    cfg.Engine,
		URL:       cfg.URL,
		Token:     cfg.Token,
		WBToken:   cfg.WBToken,
		WBCookie:  cfg.WBCookie,
		ChannelID: cfg.ChannelID,
		DeviceID:  c.deviceID,
		Name:      names.Generate(),
		OnData:    c.onData,
		DNSServer: cfg.DNSServer,
		Options:   cfg.TransportOptions,
		Traffic:   cfg.Traffic,
	})
	if err != nil {
		return fmt.Errorf("failed to create link: %w", err)
	}
	c.ln = ln

	ln.SetEndedCallback(func(reason string) {
		logger.Infof("Client link reported conference end: %s", reason)
		cancel()
	})
	ln.SetShouldReconnect(func() bool { return ctx.Err() == nil })
	ln.SetReconnectCallback(func() {
		if ctx.Err() != nil {
			return
		}
		c.handleReconnect(ctx, cfg, cancel, "carrier")
	})

	if err := ln.Connect(ctx); err != nil {
		return fmt.Errorf("failed to connect link: %w", err)
	}

	c.conn = muxconn.New(ln, c.cipher)
	sess, err := smux.Client(c.conn, smuxConfig(linkMaxPayload(ln)))
	if err != nil {
		return fmt.Errorf("smux client: %w", err)
	}

	control, sid, err := openControlStream(ctx, sess, c.deviceID, c.claims)
	if err != nil {
		_ = sess.Close()
		_ = c.conn.Close()
		return fmt.Errorf("handshake: %w", err)
	}
	logger.Infof("session %s opened (device=%s)", sid, c.deviceID)

	c.sessMu.Lock()
	c.session = sess
	c.controlStrm = control
	c.sessionID = sid
	c.sessMu.Unlock()

	c.recordSession(sid)
	c.startControlLoop(ctx, cfg, cancel, control)
	go ln.WatchConnection(ctx)
	return nil
}

func openControlStream(
	ctx context.Context,
	sess *smux.Session,
	deviceID string,
	claims map[string]any,
) (*smux.Stream, string, error) {
	return openControlStreamTimeout(ctx, sess, deviceID, claims, handshake.DefaultTimeout)
}

func openControlStreamTimeout(
	ctx context.Context,
	sess *smux.Session,
	deviceID string,
	claims map[string]any,
	timeout time.Duration,
) (*smux.Stream, string, error) {
	stream, err := sess.OpenStream()
	if err != nil {
		return nil, "", fmt.Errorf("open control stream: %w", err)
	}

	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = stream.Close()
		case <-done:
		}
	}()
	defer close(done)

	_ = stream.SetDeadline(time.Now().Add(timeout))
	sid, err := handshake.Client(stream, deviceID, claims)
	_ = stream.SetDeadline(time.Time{})
	if err != nil {
		_ = stream.Close()
		if ctx.Err() != nil {
			return nil, "", fmt.Errorf("handshake client: %w", ctx.Err())
		}
		return nil, "", fmt.Errorf("handshake client: %w", err)
	}
	return stream, sid, nil
}

func resolveDeviceID(deviceID, path string) (string, error) {
	if deviceID != "" {
		return deviceID, nil
	}
	if path == "" {
		return uuid.NewString(), nil
	}
	data, err := os.ReadFile(path)
	if err == nil {
		id := strings.TrimSpace(string(data))
		if id != "" {
			return id, nil
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("read device id %s: %w", path, err)
	}
	id := uuid.NewString()
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return "", fmt.Errorf("mkdir device id dir: %w", err)
	}
	if err := os.WriteFile(path, []byte(id+"\n"), 0o600); err != nil {
		return "", fmt.Errorf("write device id %s: %w", path, err)
	}
	return id, nil
}

func smuxConfig(maxWirePayload int) *smux.Config {
	return runtime.SmuxConfig(maxWirePayload)
}

func linkMaxPayload(tr transport.Transport) int {
	return runtime.MaxPayload(tr)
}

func (c *Client) handleReconnect(ctx context.Context, cfg Config, cancel context.CancelFunc, reason string) {
	c.reconnectMu.Lock()
	defer c.reconnectMu.Unlock()

	c.recordReconnect()
	logger.Infof("client reconnect reason=%s - tearing down smux session", reason)

	c.resetLinkPeer()

	newConn := muxconn.New(c.ln, c.cipher)

	c.sessMu.Lock()
	oldControl := c.controlStrm
	oldControlStop := c.controlStop
	oldSess := c.session
	oldConn := c.conn
	c.conn = newConn
	c.session = nil
	c.controlStrm = nil
	c.controlStop = nil
	c.sessionID = ""
	c.sessMu.Unlock()

	if oldControlStop != nil {
		oldControlStop()
	}
	if oldSess != nil {
		_ = oldSess.Close()
	}
	if oldConn != nil {
		_ = oldConn.Close()
	}
	if oldControl != nil {
		_ = oldControl.Close()
	}

	if reason == "liveness" && c.ln != nil {
		c.ln.Reconnect("liveness")
		return
	}

	c.retryHandshake(ctx, cfg, cancel, reason)
}

func (c *Client) retryHandshake(ctx context.Context, cfg Config, cancel context.CancelFunc, reason string) {
	const (
		initialDelay = 300 * time.Millisecond
		maxDelay     = 5 * time.Second
	)
	delay := initialDelay
	for attempt := 1; ; attempt++ {
		if ctx.Err() != nil {
			return
		}
		logger.Infof("client reconnect attempt=%d reason=%s", attempt, reason)
		if c.tryReopenSession(ctx, cfg, cancel, attempt) {
			return
		}
		if reason == "carrier" && attempt >= 5 {
			logger.Warnf("client reconnect: exhausted %d handshake attempts (reason=%s) - keeping listener up", attempt, reason)
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(delay):
		}
		if delay < maxDelay {
			delay *= 2
			if delay > maxDelay {
				delay = maxDelay
			}
		}
	}
}

func (c *Client) resetLinkPeer() {
	c.sessMu.RLock()
	ln := c.ln
	c.sessMu.RUnlock()
	if resetter, ok := ln.(interface{ ResetPeer() }); ok {
		resetter.ResetPeer()
	}
}

func (c *Client) tryReopenSession(
	ctx context.Context,
	cfg Config,
	cancel context.CancelFunc,
	attempt int,
) bool {
	conn := muxconn.New(c.ln, c.cipher)
	c.sessMu.Lock()
	old := c.conn
	c.conn = conn
	c.sessMu.Unlock()
	if old != nil {
		_ = old.Close()
	}

	sess, err := smux.Client(conn, smuxConfig(linkMaxPayload(c.ln)))
	if err != nil {
		logger.Warnf("smux re-init failed (attempt %d): %v", attempt, err)
		return false
	}

	control, sid, err := openControlStreamTimeout(ctx, sess, c.deviceID, c.claims, 2*time.Second)
	if err != nil {
		logger.Warnf("handshake on reconnect failed (attempt %d): %v", attempt, err)
		_ = sess.Close()
		return false
	}
	logger.Infof("session %s reopened (device=%s)", sid, c.deviceID)

	c.sessMu.Lock()
	c.session = sess
	c.controlStrm = control
	c.sessionID = sid
	c.sessMu.Unlock()

	c.recordSession(sid)
	c.startControlLoop(ctx, cfg, cancel, control)
	return true
}

func (c *Client) startControlLoop(
	ctx context.Context,
	cfg Config,
	cancel context.CancelFunc,
	stream *smux.Stream,
) {
	controlCtx, stop := context.WithCancel(ctx)
	c.sessMu.Lock()
	c.controlStop = stop
	c.sessMu.Unlock()

	liveness := cfg.Liveness
	onPong := liveness.OnPong
	onMissedPong := liveness.OnMissedPong
	onUnhealthy := liveness.OnUnhealthy

	liveness.OnPong = func(h control.Health) {
		c.sessMu.RLock()
		sid := c.sessionID
		c.sessMu.RUnlock()
		c.recordPong(h)
		logger.Debugf("control alive session=%s rtt=%v seq=%d", sid, h.RTT, h.Seq)
		if onPong != nil {
			onPong(h)
		}
	}
	liveness.OnMissedPong = func(missed int) {
		c.recordMissed(missed)
		logger.Warnf("control missed pong on client: missed_pongs=%d", missed)
		if onMissedPong != nil {
			onMissedPong(missed)
		}
	}
	liveness.OnUnhealthy = func(missed int) {
		c.recordUnhealthy(missed)
		logger.Warnf("control stream unhealthy on client: missed_pongs=%d", missed)
		if onUnhealthy != nil {
			onUnhealthy(missed)
		}
	}

	go func() {
		err := control.Run(controlCtx, stream, liveness)
		if controlCtx.Err() != nil || ctx.Err() != nil {
			return
		}
		if err != nil {
			logger.Warnf("client control stream ended: %v", err)
		}
		c.handleReconnect(ctx, cfg, cancel, "liveness")
	}()
}

func (c *Client) Status() control.Status {
	return c.health.Status()
}

func (c *Client) recordSession(sessionID string) { c.health.RecordSession(sessionID) }
func (c *Client) recordPong(h control.Health)    { c.health.RecordPong(h) }
func (c *Client) recordMissed(missed int)        { c.health.RecordMissed(missed) }
func (c *Client) recordUnhealthy(missed int)     { c.health.RecordUnhealthy(missed) }
func (c *Client) recordReconnect()               { c.health.RecordReconnect() }

func (c *Client) shutdown() {
	c.sessMu.Lock()
	control := c.controlStrm
	controlStop := c.controlStop
	sess := c.session
	conn := c.conn
	c.controlStrm = nil
	c.controlStop = nil
	c.session = nil
	c.conn = nil
	c.sessMu.Unlock()

	notifyControlClose(control)
	if controlStop != nil {
		controlStop()
	}
	if sess != nil {
		_ = sess.Close()
	}
	if conn != nil {
		_ = conn.Close()
	}
	if c.ln != nil {
		_ = c.ln.Close()
	}
	if control != nil {
		_ = control.Close()
	}
}

func notifyControlClose(stream *smux.Stream) {
	if stream == nil {
		return
	}
	_ = stream.SetWriteDeadline(time.Now().Add(2 * time.Second))
	if err := control.SendClose(stream); err == nil {
		time.Sleep(200 * time.Millisecond)
	}
	_ = stream.SetWriteDeadline(time.Time{})
	_ = stream.CloseWrite()
}

func setupCipher(keyHex string) (*crypto.Cipher, error) {
	cipher, err := runtime.SetupCipher(keyHex)
	if err != nil {
		return nil, fmt.Errorf("client: %w", err)
	}
	return cipher, nil
}

func (c *Client) onData(data []byte) {
	c.sessMu.RLock()
	conn := c.conn
	c.sessMu.RUnlock()
	if conn != nil {
		conn.Push(data)
	}
}

func (c *Client) acceptLoop(ctx context.Context, ln net.Listener) {
	for {
		conn, err := ln.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				return
			default:
				logger.Warnf("Accept error: %v", err)
				continue
			}
		}
		go c.handleSocks5(ctx, conn)
	}
}

func (c *Client) handleSocks5(_ context.Context, conn net.Conn) {
	defer func() { _ = conn.Close() }()

	if err := c.socks5Handshake(conn); err != nil {
		return
	}

	targetAddr, targetPort, err := c.socks5Request(conn)
	if err != nil {
		return
	}

	c.sessMu.RLock()
	sess := c.session
	c.sessMu.RUnlock()

	if sess == nil || sess.IsClosed() {
		_, _ = conn.Write(replyHostUnreachable())
		return
	}

	c.tunnel(conn, sess, targetAddr, targetPort)
}

func (c *Client) tunnel(conn net.Conn, sess *smux.Session, targetAddr string, targetPort int) {
	stream, err := sess.OpenStream()
	if err != nil {
		logger.Warnf("OpenStream failed: %v", err)
		_, _ = conn.Write(replyHostUnreachable())
		return
	}
	defer func() { _ = stream.Close() }()

	logger.Infof("sid=%d tunnel to %s:%d", stream.ID(), targetAddr, targetPort)

	if err := c.sendConnectRequest(stream, targetAddr, targetPort); err != nil {
		logger.Warnf("sid=%d connect failed: %v", stream.ID(), err)
		_, _ = conn.Write(replyHostUnreachable())
		return
	}

	if _, err := conn.Write(replySuccess()); err != nil {
		return
	}

	go func() {
		bufp := copyBufPool.Get().(*[]byte)
		_, _ = io.CopyBuffer(stream, conn, *bufp)
		copyBufPool.Put(bufp)
		_ = stream.Close()
	}()

	bufp := copyBufPool.Get().(*[]byte)
	_, _ = io.CopyBuffer(conn, stream, *bufp)
	copyBufPool.Put(bufp)
}

func (c *Client) sendConnectRequest(stream *smux.Stream, targetAddr string, targetPort int) error {
	connectReq, err := json.Marshal(connectRequest{Cmd: "connect", Addr: targetAddr, Port: targetPort})
	if err != nil {
		return fmt.Errorf("sid=%d marshal connect req: %w", stream.ID(), err)
	}
	_ = stream.SetWriteDeadline(time.Now().Add(10 * time.Second))
	if _, err := stream.Write(connectReq); err != nil {
		return fmt.Errorf("sid=%d write connect req: %w", stream.ID(), err)
	}
	_ = stream.SetWriteDeadline(time.Time{})

	ack := make([]byte, 1)
	_ = stream.SetReadDeadline(time.Now().Add(15 * time.Second))
	if _, err := io.ReadFull(stream, ack); err != nil || ack[0] != 0x00 {
		return fmt.Errorf("sid=%d: %w (read_err=%w ack=%v)", stream.ID(), ErrRemoteNotReady, err, ack)
	}
	_ = stream.SetReadDeadline(time.Time{})
	return nil
}

func (c *Client) socks5Handshake(conn net.Conn) error {
	buf := make([]byte, 2)
	if _, err := io.ReadFull(conn, buf); err != nil {
		return fmt.Errorf("read socks5 header: %w", err)
	}
	if buf[0] != 5 {
		return fmt.Errorf("%w: %d", ErrInvalidSOCKSVersion, buf[0])
	}
	methods := make([]byte, buf[1])
	if _, err := io.ReadFull(conn, methods); err != nil {
		return fmt.Errorf("read socks5 methods: %w", err)
	}
	if c.socksUser != "" {
		if _, err := conn.Write([]byte{5, 2}); err != nil {
			return fmt.Errorf("write socks5 auth method: %w", err)
		}
		if err := c.socks5UserPassAuth(conn); err != nil {
			return err
		}
		return nil
	}
	if _, err := conn.Write([]byte{5, 0}); err != nil {
		return fmt.Errorf("write socks5 auth: %w", err)
	}
	return nil
}

func (c *Client) socks5UserPassAuth(conn net.Conn) error {
	header := make([]byte, 2)
	if _, err := io.ReadFull(conn, header); err != nil {
		return fmt.Errorf("read socks5 auth header: %w", err)
	}
	if header[0] != 0x01 {
		return fmt.Errorf("%w: expected auth version 1, got %d", ErrInvalidSOCKSVersion, header[0])
	}
	ulen := int(header[1])
	userBuf := make([]byte, ulen)
	if _, err := io.ReadFull(conn, userBuf); err != nil {
		return fmt.Errorf("read socks5 username: %w", err)
	}
	plenBuf := make([]byte, 1)
	if _, err := io.ReadFull(conn, plenBuf); err != nil {
		return fmt.Errorf("read socks5 plen: %w", err)
	}
	plen := int(plenBuf[0])
	passBuf := make([]byte, plen)
	if _, err := io.ReadFull(conn, passBuf); err != nil {
		return fmt.Errorf("read socks5 password: %w", err)
	}
	if string(userBuf) != c.socksUser || string(passBuf) != c.socksPass {
		_, _ = conn.Write([]byte{0x01, 0x01})
		return ErrSOCKSAuthFailed
	}
	if _, err := conn.Write([]byte{0x01, 0x00}); err != nil {
		return fmt.Errorf("write socks5 auth success: %w", err)
	}
	return nil
}

func (c *Client) socks5Request(conn net.Conn) (string, int, error) {
	header := make([]byte, 4)
	if _, err := io.ReadFull(conn, header); err != nil {
		return "", 0, fmt.Errorf("read socks5 request: %w", err)
	}
	if header[1] != 1 {
		return "", 0, fmt.Errorf("%w: %d", ErrUnsupportedSOCKSCommand, header[1])
	}
	addr, err := c.readSocks5Addr(conn, header[3])
	if err != nil {
		return "", 0, err
	}
	portBuf := make([]byte, 2)
	if _, err := io.ReadFull(conn, portBuf); err != nil {
		return "", 0, fmt.Errorf("read socks5 port: %w", err)
	}
	port := int(binary.BigEndian.Uint16(portBuf))
	return addr, port, nil
}

func (c *Client) readSocks5Addr(conn net.Conn, addrType byte) (string, error) {
	switch addrType {
	case 1:
		buf := make([]byte, 4)
		if _, err := io.ReadFull(conn, buf); err != nil {
			return "", fmt.Errorf("read socks5 ipv4: %w", err)
		}
		return net.IP(buf).String(), nil
	case 3:
		lenBuf := make([]byte, 1)
		if _, err := io.ReadFull(conn, lenBuf); err != nil {
			return "", fmt.Errorf("read socks5 domain len: %w", err)
		}
		buf := make([]byte, lenBuf[0])
		if _, err := io.ReadFull(conn, buf); err != nil {
			return "", fmt.Errorf("read socks5 domain: %w", err)
		}
		return string(buf), nil
	default:
		return "", fmt.Errorf("%w: %d", ErrUnsupportedAddressType, addrType)
	}
}

func replySuccess() []byte {
	return successReply
}

func replyHostUnreachable() []byte {
	return hostUnreachableReply
}
