package olcrtc

import (
	"errors"
	"net"
	"time"
)

type conn struct {
	s *Session
}

var (
	localAddr  net.Addr = webrtcAddr("local")
	remoteAddr net.Addr = webrtcAddr("remote")
)

func (c *conn) Read(b []byte) (int, error) {
	return c.s.pr.Read(b)
}

func (c *conn) Write(b []byte) (int, error) {
	if err := c.s.inner.Send(b); err != nil {
		return 0, err
	}
	return len(b), nil
}

func (c *conn) Close() error {
	_ = c.s.pw.CloseWithError(net.ErrClosed)
	return c.s.inner.Close()
}

func (c *conn) LocalAddr() net.Addr  { return localAddr }
func (c *conn) RemoteAddr() net.Addr { return remoteAddr }

func (c *conn) SetDeadline(_ time.Time) error      { return errors.ErrUnsupported }
func (c *conn) SetReadDeadline(_ time.Time) error  { return errors.ErrUnsupported }
func (c *conn) SetWriteDeadline(_ time.Time) error { return errors.ErrUnsupported }

type webrtcAddr string

func (a webrtcAddr) Network() string { return "webrtc" }
func (a webrtcAddr) String() string  { return string(a) }
