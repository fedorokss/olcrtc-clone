package vp8channel

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"sync"

	kcp "github.com/xtaci/kcp-go/v5"
)

const kcpConvID = 0xC0FFEE01

const (
	kcpMTU        = 1400
	kcpSndWnd     = 768
	kcpRcvWnd     = 1024
	kcpLenPrefix  = 4
	kcpMaxMessage = 8 * 1024 * 1024
)

var ErrKCPMessageTooLarge = errors.New("vp8channel: kcp message exceeds maximum size")

type kcpRuntime struct {
	conn      *kcpConn
	sess      *kcp.UDPSession
	readDone  chan struct{}
	writeMu   sync.Mutex
	closeOnce sync.Once
}

func startKCP(out chan<- []byte, onData func([]byte), epochHdr [epochHdrLen]byte) (*kcpRuntime, error) {
	c := newKCPConn(out, inboundQueueSize, epochHdr)
	sess, err := kcp.NewConn3(kcpConvID, fakeUDPAddr(), nil, 0, 0, c)
	if err != nil {
		_ = c.Close()
		return nil, fmt.Errorf("kcp new conn: %w", err)
	}
	sess.SetNoDelay(1, 5, 2, 1)
	sess.SetWindowSize(kcpSndWnd, kcpRcvWnd)
	sess.SetMtu(kcpMTU)
	sess.SetStreamMode(true)
	sess.SetACKNoDelay(true)
	sess.SetWriteDelay(false)
	rt := &kcpRuntime{
		conn:     c,
		sess:     sess,
		readDone: make(chan struct{}),
	}
	go rt.readLoop(onData)
	return rt, nil
}

func (r *kcpRuntime) readLoop(onData func([]byte)) {
	defer close(r.readDone)
	var hdr [kcpLenPrefix]byte
	for {
		if _, err := io.ReadFull(r.sess, hdr[:]); err != nil {
			return
		}
		size := binary.BigEndian.Uint32(hdr[:])
		if size == 0 {
			continue
		}
		if size > kcpMaxMessage {
			return
		}
		payload := make([]byte, size)
		if _, err := io.ReadFull(r.sess, payload); err != nil {
			return
		}
		if onData != nil {
			onData(payload)
		}
	}
}

func (r *kcpRuntime) deliver(payload []byte) {
	r.conn.deliver(payload)
}

func (r *kcpRuntime) send(msg []byte) error {
	n := len(msg)
	if n > kcpMaxMessage {
		return ErrKCPMessageTooLarge
	}
	var hdr [kcpLenPrefix]byte
	binary.BigEndian.PutUint32(hdr[:], uint32(n))
	r.writeMu.Lock()
	_, err := r.sess.WriteBuffers([][]byte{hdr[:], msg})
	r.writeMu.Unlock()
	if err != nil {
		return fmt.Errorf("kcp write: %w", err)
	}
	return nil
}

func (r *kcpRuntime) close() {
	r.closeOnce.Do(func() {
		_ = r.sess.Close()
		_ = r.conn.Close()
	})
}
