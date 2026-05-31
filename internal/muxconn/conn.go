package muxconn

import (
	"errors"
	"fmt"
	"io"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/openlibrecommunity/olcrtc/internal/crypto"
	"github.com/openlibrecommunity/olcrtc/internal/logger"
	"github.com/openlibrecommunity/olcrtc/internal/transport"
)

var ErrClosed = errors.New("muxconn: closed")

const (
	inboundQueue   = 256
	pooledFrameCap = 64 * 1024
)

var frameBufPool = sync.Pool{
	New: func() any {
		b := make([]byte, 0, pooledFrameCap)
		return &b
	},
}

func acquireFrameBuf() *[]byte {
	bp := frameBufPool.Get().(*[]byte)
	*bp = (*bp)[:0]
	return bp
}

func releaseFrameBuf(bp *[]byte) {
	if bp == nil {
		return
	}
	if cap(*bp) > pooledFrameCap*2 {
		return
	}
	*bp = (*bp)[:0]
	frameBufPool.Put(bp)
}

type Conn struct {
	ln     transport.Transport
	send   func([]byte) error
	cipher *crypto.Cipher

	in        chan *[]byte
	closeOnce sync.Once
	closeCh   chan struct{}
	closed    atomic.Bool

	leftoverBuf *[]byte
	leftover    []byte
}

func New(ln transport.Transport, cipher *crypto.Cipher) *Conn {
	return &Conn{
		ln:      ln,
		send:    ln.Send,
		cipher:  cipher,
		in:      make(chan *[]byte, inboundQueue),
		closeCh: make(chan struct{}),
	}
}

func NewPeer(ln transport.PeerTransport, cipher *crypto.Cipher, peerID string) *Conn {
	return &Conn{
		ln: ln,
		send: func(data []byte) error {
			return ln.SendTo(peerID, data)
		},
		cipher:  cipher,
		in:      make(chan *[]byte, inboundQueue),
		closeCh: make(chan struct{}),
	}
}

func (c *Conn) Push(ciphertext []byte) {
	if c.closed.Load() {
		return
	}
	bufPtr := acquireFrameBuf()
	pt, err := c.cipher.DecryptInto(*bufPtr, ciphertext)
	if err != nil {
		releaseFrameBuf(bufPtr)
		logger.Debugf("muxconn: decrypt failed, dropping frame: %v", err)
		return
	}
	*bufPtr = pt
	if c.closed.Load() {
		releaseFrameBuf(bufPtr)
		return
	}
	select {
	case c.in <- bufPtr:
	case <-c.closeCh:
		releaseFrameBuf(bufPtr)
	}
}

func (c *Conn) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if len(c.leftover) == 0 {
		bufPtr, ok := c.takeFrame()
		if !ok {
			return 0, io.EOF
		}
		c.leftoverBuf = bufPtr
		c.leftover = *bufPtr
	}
	n := copy(p, c.leftover)
	c.leftover = c.leftover[n:]
	if len(c.leftover) == 0 && c.leftoverBuf != nil {
		releaseFrameBuf(c.leftoverBuf)
		c.leftoverBuf = nil
	}
	for n < len(p) && len(c.leftover) == 0 {
		select {
		case bufPtr, ok := <-c.in:
			if !ok {
				return n, nil
			}
			data := *bufPtr
			m := copy(p[n:], data)
			n += m
			if m < len(data) {
				c.leftoverBuf = bufPtr
				c.leftover = data[m:]
			} else {
				releaseFrameBuf(bufPtr)
			}
		default:
			return n, nil
		}
	}
	return n, nil
}

func (c *Conn) takeFrame() (*[]byte, bool) {
	select {
	case bufPtr, ok := <-c.in:
		return bufPtr, ok
	case <-c.closeCh:
		select {
		case bufPtr, ok := <-c.in:
			return bufPtr, ok
		default:
			return nil, false
		}
	}
}

func (c *Conn) recycleIfDrained() {
	if len(c.leftover) == 0 && c.leftoverBuf != nil {
		releaseFrameBuf(c.leftoverBuf)
		c.leftoverBuf = nil
	}
}

func (c *Conn) Write(p []byte) (int, error) {
	const (
		fastSpinAttempts = 200
		slowPollDelay    = 2 * time.Millisecond
	)
	if c.closed.Load() {
		return 0, ErrClosed
	}
	for attempt := 0; !c.ln.CanSend(); attempt++ {
		if c.closed.Load() {
			return 0, ErrClosed
		}
		if attempt < fastSpinAttempts {
			runtime.Gosched()
		} else {
			time.Sleep(slowPollDelay)
		}
	}
	enc, err := c.cipher.Encrypt(p)
	if err != nil {
		return 0, fmt.Errorf("encrypt: %w", err)
	}
	if err := c.send(enc); err != nil {
		return 0, fmt.Errorf("send: %w", err)
	}
	return len(p), nil
}

func (c *Conn) Close() error {
	c.closeOnce.Do(func() {
		c.closed.Store(true)
		close(c.closeCh)
	})
	return nil
}
