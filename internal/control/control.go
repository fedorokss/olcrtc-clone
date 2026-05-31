package control

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/fedorokss/olcrtc-clone/internal/framing"
)

const (
	ProtoVersion    = 1
	MaxMessageSize  = 16 * 1024
	DefaultInterval = 10 * time.Second
	DefaultTimeout  = 15 * time.Second
	DefaultFailures = 4
)

type MsgType string

const (
	TypePing  MsgType = "CONTROL_PING"
	TypePong  MsgType = "CONTROL_PONG"
	TypeClose MsgType = "CONTROL_CLOSE"
)

var (
	ErrUnhealthy         = errors.New("control stream unhealthy")
	ErrClosedByPeer      = errors.New("control stream closed by peer")
	ErrProtocolVersion   = errors.New("incompatible control protocol version")
	ErrUnexpectedMessage = errors.New("unexpected control message")
	ErrFrameTooLarge     = framing.ErrFrameTooLarge

	errBadJSON = errors.New("invalid control json")
)

type Message struct {
	Version      int     `json:"version"`
	Type         MsgType `json:"type"`
	Seq          uint64  `json:"seq,omitempty"`
	SentUnixNano int64   `json:"sent_unix_nano,omitempty"`
}

func (m Message) MarshalJSON() ([]byte, error) {
	buf := make([]byte, 0, 80)
	buf = append(buf, `{"version":`...)
	buf = strconv.AppendInt(buf, int64(m.Version), 10)
	buf = append(buf, `,"type":`...)
	buf = appendJSONString(buf, string(m.Type))
	if m.Seq != 0 {
		buf = append(buf, `,"seq":`...)
		buf = strconv.AppendUint(buf, m.Seq, 10)
	}
	if m.SentUnixNano != 0 {
		buf = append(buf, `,"sent_unix_nano":`...)
		buf = strconv.AppendInt(buf, m.SentUnixNano, 10)
	}
	buf = append(buf, '}')
	return buf, nil
}

type Health struct {
	Seq      uint64
	RTT      time.Duration
	LastSeen time.Time
}

type Status struct {
	SessionID       string
	LastPong        time.Time
	LastRTT         time.Duration
	MissedPongs     int
	Reconnects      uint64
	UnhealthyEvents uint64
	LastUnhealthy   time.Time
}

type Config struct {
	Interval     time.Duration
	Timeout      time.Duration
	Failures     int
	OnPong       func(Health)
	OnMissedPong func(missed int)
	OnUnhealthy  func(missed int)
}

func (cfg Config) withDefaults() Config {
	if cfg.Interval <= 0 {
		cfg.Interval = DefaultInterval
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = DefaultTimeout
	}
	if cfg.Failures <= 0 {
		cfg.Failures = DefaultFailures
	}
	return cfg
}

func Run(ctx context.Context, rw io.ReadWriteCloser, cfg Config) error {
	cfg = cfg.withDefaults()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	state := &state{
		rw:      rw,
		cfg:     cfg,
		pending: make([]pendingProbe, 0, cfg.Failures+1),
		now:     time.Now,
		out:     make(chan Message, 16),
	}

	errCh := make(chan error, 3)
	go func() {
		<-ctx.Done()
		_ = rw.Close()
	}()
	go func() { errCh <- state.readLoop(ctx) }()
	go func() { errCh <- state.probeLoop(ctx) }()
	go func() { errCh <- state.writeLoop(ctx) }()

	err := <-errCh
	cancel()
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return nil
	}
	return err
}

type pendingProbe struct {
	seq  uint64
	sent time.Time
}

type state struct {
	rw  io.ReadWriteCloser
	cfg Config
	now func() time.Time
	out chan Message

	mu       sync.Mutex
	pending  []pendingProbe
	nextSeq  uint64
	failures int
}

func (s *state) readLoop(ctx context.Context) error {
	for {
		raw, err := readFrame(s.rw)
		if err != nil {
			return readLoopErr(ctx, err)
		}
		msg, err := parseMessage(raw)
		if err != nil {
			return err
		}
		if err := s.handleReadMessage(ctx, msg); err != nil {
			return err
		}
	}
}

func readLoopErr(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		return fmt.Errorf("read loop canceled: %w", ctx.Err())
	}
	return err
}

func (s *state) handleReadMessage(ctx context.Context, msg Message) error {
	switch msg.Type {
	case TypePing:
		return s.enqueuePong(ctx, msg)
	case TypePong:
		s.handlePong(msg)
		return nil
	case TypeClose:
		return ErrClosedByPeer
	default:
		return fmt.Errorf("%w: got %q", ErrUnexpectedMessage, msg.Type)
	}
}

func (s *state) enqueuePong(ctx context.Context, ping Message) error {
	err := s.enqueue(ctx, Message{
		Version:      ProtoVersion,
		Type:         TypePong,
		Seq:          ping.Seq,
		SentUnixNano: ping.SentUnixNano,
	})
	if err != nil {
		return readLoopErr(ctx, err)
	}
	return nil
}

func (s *state) probeLoop(ctx context.Context) error {
	ticker := time.NewTicker(s.cfg.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("probe loop canceled: %w", ctx.Err())
		case <-ticker.C:
			if err := s.sendProbe(ctx); err != nil {
				return err
			}
		}
	}
}

func (s *state) sendProbe(ctx context.Context) error {
	now := s.now()
	s.mu.Lock()
	k := 0
	for k < len(s.pending) && now.Sub(s.pending[k].sent) >= s.cfg.Timeout {
		k++
	}
	missedNow := k
	if k > 0 {
		s.pending = s.pending[k:]
		s.failures += k
	}
	missed := s.failures
	if s.failures >= s.cfg.Failures {
		s.mu.Unlock()
		if missedNow > 0 && s.cfg.OnMissedPong != nil {
			s.cfg.OnMissedPong(missed)
		}
		if s.cfg.OnUnhealthy != nil {
			s.cfg.OnUnhealthy(missed)
		}
		return fmt.Errorf("%w: missed %d pong(s)", ErrUnhealthy, missed)
	}
	s.nextSeq++
	seq := s.nextSeq
	s.pending = append(s.pending, pendingProbe{seq: seq, sent: now})
	s.mu.Unlock()

	if missedNow > 0 && s.cfg.OnMissedPong != nil {
		s.cfg.OnMissedPong(missed)
	}
	return s.enqueue(ctx, Message{
		Version:      ProtoVersion,
		Type:         TypePing,
		Seq:          seq,
		SentUnixNano: now.UnixNano(),
	})
}

func (s *state) handlePong(msg Message) {
	now := s.now()
	s.mu.Lock()
	var sent time.Time
	found := false
	for i := range s.pending {
		if s.pending[i].seq == msg.Seq {
			sent = s.pending[i].sent
			s.pending = append(s.pending[:i], s.pending[i+1:]...)
			s.failures = 0
			found = true
			break
		}
	}
	s.mu.Unlock()
	if !found || s.cfg.OnPong == nil {
		return
	}
	s.cfg.OnPong(Health{
		Seq:      msg.Seq,
		RTT:      now.Sub(sent),
		LastSeen: now,
	})
}

func (s *state) enqueue(ctx context.Context, msg Message) error {
	select {
	case <-ctx.Done():
		return fmt.Errorf("enqueue canceled: %w", ctx.Err())
	case s.out <- msg:
		return nil
	}
}

func (s *state) writeLoop(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("write loop canceled: %w", ctx.Err())
		case msg := <-s.out:
			if err := writeFrame(s.rw, msg); err != nil {
				if ctx.Err() != nil {
					return fmt.Errorf("write loop canceled: %w", ctx.Err())
				}
				return err
			}
		}
	}
}

func parseMessage(raw []byte) (Message, error) {
	msg, err := decodeMessage(raw)
	if err != nil {
		return Message{}, fmt.Errorf("parse control message: %w", err)
	}
	if msg.Version != ProtoVersion {
		return Message{}, fmt.Errorf("%w: peer v%d, local v%d",
			ErrProtocolVersion, msg.Version, ProtoVersion)
	}
	if msg.Type != TypePing && msg.Type != TypePong && msg.Type != TypeClose {
		return Message{}, fmt.Errorf("%w: got %q", ErrUnexpectedMessage, msg.Type)
	}
	return msg, nil
}

func SendClose(w io.Writer) error {
	return writeFrame(w, Message{Version: ProtoVersion, Type: TypeClose})
}

func writeFrame(w io.Writer, msg Message) error {
	if err := framing.WriteJSON(w, msg, MaxMessageSize); err != nil {
		return fmt.Errorf("control: %w", err)
	}
	return nil
}

func readFrame(r io.Reader) ([]byte, error) {
	body, err := framing.ReadBytes(r, MaxMessageSize)
	if err != nil {
		return nil, fmt.Errorf("control: %w", err)
	}
	return body, nil
}

func appendJSONString(b []byte, s string) []byte {
	const hex = "0123456789abcdef"
	b = append(b, '"')
	start := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 0x20 && c != '"' && c != '\\' && c != '<' && c != '>' && c != '&' {
			continue
		}
		if start < i {
			b = append(b, s[start:i]...)
		}
		switch c {
		case '"':
			b = append(b, '\\', '"')
		case '\\':
			b = append(b, '\\', '\\')
		case '\n':
			b = append(b, '\\', 'n')
		case '\r':
			b = append(b, '\\', 'r')
		case '\t':
			b = append(b, '\\', 't')
		case '<':
			b = append(b, '\\', 'u', '0', '0', '3', 'c')
		case '>':
			b = append(b, '\\', 'u', '0', '0', '3', 'e')
		case '&':
			b = append(b, '\\', 'u', '0', '0', '2', '6')
		default:
			b = append(b, '\\', 'u', '0', '0', hex[c>>4], hex[c&0xf])
		}
		start = i + 1
	}
	if start < len(s) {
		b = append(b, s[start:]...)
	}
	return append(b, '"')
}

func decodeMessage(b []byte) (Message, error) {
	var msg Message
	n := len(b)
	i := skipSpace(b, 0)
	if i >= n || b[i] != '{' {
		return msg, errBadJSON
	}
	i++
	for {
		i = skipSpace(b, i)
		if i < n && b[i] == '}' {
			i++
			break
		}
		key, ni, err := scanString(b, i)
		if err != nil {
			return msg, err
		}
		i = skipSpace(b, ni)
		if i >= n || b[i] != ':' {
			return msg, errBadJSON
		}
		i = skipSpace(b, i+1)

		switch key {
		case "version":
			v, ni, err := scanNumber(b, i)
			if err != nil {
				return msg, err
			}
			msg.Version, i = int(v), ni
		case "seq":
			v, ni, err := scanNumber(b, i)
			if err != nil {
				return msg, err
			}
			msg.Seq, i = uint64(v), ni
		case "sent_unix_nano":
			v, ni, err := scanNumber(b, i)
			if err != nil {
				return msg, err
			}
			msg.SentUnixNano, i = v, ni
		case "type":
			s, ni, err := scanString(b, i)
			if err != nil {
				return msg, err
			}
			msg.Type, i = MsgType(s), ni
		default:
			ni, err := skipValue(b, i)
			if err != nil {
				return msg, err
			}
			i = ni
		}

		i = skipSpace(b, i)
		if i < n && b[i] == ',' {
			i++
			continue
		}
		if i < n && b[i] == '}' {
			i++
			break
		}
		return msg, errBadJSON
	}
	return msg, nil
}

func skipSpace(b []byte, i int) int {
	for i < len(b) {
		switch b[i] {
		case ' ', '\t', '\n', '\r':
			i++
		default:
			return i
		}
	}
	return i
}

func scanNumber(b []byte, i int) (int64, int, error) {
	n := len(b)
	start := i
	for i < n {
		c := b[i]
		if (c >= '0' && c <= '9') || c == '-' || c == '+' || c == '.' || c == 'e' || c == 'E' {
			i++
			continue
		}
		break
	}
	if start == i {
		return 0, i, errBadJSON
	}
	v, err := strconv.ParseInt(string(b[start:i]), 10, 64)
	if err != nil {
		return 0, i, errBadJSON
	}
	return v, i, nil
}

func scanString(b []byte, i int) (string, int, error) {
	n := len(b)
	if i >= n || b[i] != '"' {
		return "", i, errBadJSON
	}
	i++
	start := i
	for i < n {
		c := b[i]
		if c == '"' {
			return string(b[start:i]), i + 1, nil
		}
		if c == '\\' {
			break
		}
		i++
	}
	if i >= n {
		return "", i, errBadJSON
	}
	sb := make([]byte, 0, n-start)
	sb = append(sb, b[start:i]...)
	for i < n {
		c := b[i]
		if c == '"' {
			return string(sb), i + 1, nil
		}
		if c != '\\' {
			sb = append(sb, c)
			i++
			continue
		}
		i++
		if i >= n {
			return "", i, errBadJSON
		}
		switch b[i] {
		case '"':
			sb = append(sb, '"')
		case '\\':
			sb = append(sb, '\\')
		case '/':
			sb = append(sb, '/')
		case 'n':
			sb = append(sb, '\n')
		case 't':
			sb = append(sb, '\t')
		case 'r':
			sb = append(sb, '\r')
		case 'b':
			sb = append(sb, '\b')
		case 'f':
			sb = append(sb, '\f')
		case 'u':
			if i+4 >= n {
				return "", i, errBadJSON
			}
			r, ok := parseHex4(b[i+1 : i+5])
			if !ok {
				return "", i, errBadJSON
			}
			sb = utf8.AppendRune(sb, r)
			i += 4
		default:
			return "", i, errBadJSON
		}
		i++
	}
	return "", i, errBadJSON
}

func parseHex4(b []byte) (rune, bool) {
	var r rune
	for _, c := range b {
		var v rune
		switch {
		case c >= '0' && c <= '9':
			v = rune(c - '0')
		case c >= 'a' && c <= 'f':
			v = rune(c-'a') + 10
		case c >= 'A' && c <= 'F':
			v = rune(c-'A') + 10
		default:
			return 0, false
		}
		r = r<<4 | v
	}
	return r, true
}

func skipValue(b []byte, i int) (int, error) {
	n := len(b)
	if i >= n {
		return i, errBadJSON
	}
	switch b[i] {
	case '"':
		_, ni, err := scanString(b, i)
		return ni, err
	case '{', '[':
		open := b[i]
		closeCh := byte('}')
		if open == '[' {
			closeCh = ']'
		}
		depth := 0
		for i < n {
			c := b[i]
			if c == '"' {
				_, ni, err := scanString(b, i)
				if err != nil {
					return i, err
				}
				i = ni
				continue
			}
			if c == open {
				depth++
			} else if c == closeCh {
				depth--
				if depth == 0 {
					return i + 1, nil
				}
			}
			i++
		}
		return i, errBadJSON
	default:
		for i < n {
			c := b[i]
			if c == ',' || c == '}' || c == ']' || c == ' ' || c == '\t' || c == '\n' || c == '\r' {
				break
			}
			i++
		}
		return i, nil
	}
}
