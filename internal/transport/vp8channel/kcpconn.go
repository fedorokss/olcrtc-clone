package vp8channel

import (
	"net"
	"sync"
	"sync/atomic"
	"time"
)

var sharedUDPAddr = &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 1}

func fakeUDPAddr() *net.UDPAddr { return sharedUDPAddr }

var timerPool = sync.Pool{New: func() any { return time.NewTimer(0) }}

func acquireTimer(d time.Duration) *time.Timer {
	t := timerPool.Get().(*time.Timer)
	if !t.Stop() {
		select {
		case <-t.C:
		default:
		}
	}
	t.Reset(d)
	return t
}

func releaseTimer(t *time.Timer) {
	if !t.Stop() {
		select {
		case <-t.C:
		default:
		}
	}
	timerPool.Put(t)
}

type kcpConn struct {
	out       chan<- []byte
	in        chan []byte
	closed    chan struct{}
	closeOnce sync.Once
	epochHdr  [epochHdrLen]byte
	rDeadline atomic.Pointer[time.Time]
	wDeadline atomic.Pointer[time.Time]
}

func newKCPConn(out chan<- []byte, inboundCap int, epochHdr [epochHdrLen]byte) *kcpConn {
	if inboundCap <= 0 {
		inboundCap = 1024
	}
	return &kcpConn{
		out:      out,
		in:       make(chan []byte, inboundCap),
		closed:   make(chan struct{}),
		epochHdr: epochHdr,
	}
}

func (c *kcpConn) deliver(payload []byte) {
	cp := make([]byte, len(payload))
	copy(cp, payload)
	select {
	case c.in <- cp:
	case <-c.closed:
	default:
	}
}

func (c *kcpConn) ReadFrom(p []byte) (int, net.Addr, error) {
	if dl := c.rDeadline.Load(); dl != nil && !dl.IsZero() {
		d := time.Until(*dl)
		if d <= 0 {
			return 0, nil, TimeoutError{}
		}
		t := acquireTimer(d)
		defer releaseTimer(t)
		select {
		case msg := <-c.in:
			return copy(p, msg), sharedUDPAddr, nil
		case <-c.closed:
			return 0, nil, net.ErrClosed
		case <-t.C:
			return 0, nil, TimeoutError{}
		}
	}
	select {
	case msg := <-c.in:
		return copy(p, msg), sharedUDPAddr, nil
	case <-c.closed:
		return 0, nil, net.ErrClosed
	}
}

func (c *kcpConn) WriteTo(p []byte, _ net.Addr) (int, error) {
	buf := make([]byte, epochHdrLen+len(p))
	copy(buf, c.epochHdr[:])
	copy(buf[epochHdrLen:], p)
	if dl := c.wDeadline.Load(); dl != nil && !dl.IsZero() {
		d := time.Until(*dl)
		if d <= 0 {
			return 0, TimeoutError{}
		}
		t := acquireTimer(d)
		defer releaseTimer(t)
		select {
		case c.out <- buf:
			return len(p), nil
		case <-c.closed:
			return 0, net.ErrClosed
		case <-t.C:
			return 0, TimeoutError{}
		}
	}
	select {
	case c.out <- buf:
		return len(p), nil
	case <-c.closed:
		return 0, net.ErrClosed
	}
}

func (c *kcpConn) Close() error {
	c.closeOnce.Do(func() { close(c.closed) })
	return nil
}

func (c *kcpConn) LocalAddr() net.Addr { return sharedUDPAddr }

func (c *kcpConn) SetDeadline(t time.Time) error {
	c.rDeadline.Store(&t)
	c.wDeadline.Store(&t)
	return nil
}

func (c *kcpConn) SetReadDeadline(t time.Time) error {
	c.rDeadline.Store(&t)
	return nil
}

func (c *kcpConn) SetWriteDeadline(t time.Time) error {
	c.wDeadline.Store(&t)
	return nil
}

type TimeoutError struct{}

func (TimeoutError) Error() string   { return "i/o timeout" }
func (TimeoutError) Timeout() bool   { return true }
func (TimeoutError) Temporary() bool { return true }
