package framing

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
)

var ErrFrameTooLarge = errors.New("frame too large")

var bufPool = sync.Pool{New: func() any { return new(bytes.Buffer) }}

func WriteJSON(w io.Writer, msg any, maxSize int) error {
	buf := bufPool.Get().(*bytes.Buffer)
	buf.Reset()
	if err := json.NewEncoder(buf).Encode(msg); err != nil {
		bufPool.Put(buf)
		return fmt.Errorf("marshal: %w", err)
	}
	body := buf.Bytes()
	if n := len(body); n > 0 && body[n-1] == '\n' {
		body = body[:n-1]
	}
	err := WriteBytes(w, body, maxSize)
	bufPool.Put(buf)
	return err
}

func WriteBytes(w io.Writer, body []byte, maxSize int) error {
	if maxSize > 0 && len(body) > maxSize {
		return fmt.Errorf("%w: %d > %d", ErrFrameTooLarge, len(body), maxSize)
	}
	var hdr [4]byte
	binary.BigEndian.PutUint32(hdr[:], uint32(len(body)))
	bufs := net.Buffers{hdr[:], body}
	if _, err := bufs.WriteTo(w); err != nil {
		return fmt.Errorf("write: %w", err)
	}
	return nil
}

func ReadBytes(r io.Reader, maxSize int) ([]byte, error) {
	var hdr [4]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return nil, fmt.Errorf("read hdr: %w", err)
	}
	n := binary.BigEndian.Uint32(hdr[:])
	if maxSize > 0 && n > uint32(maxSize) {
		return nil, fmt.Errorf("%w: %d > %d", ErrFrameTooLarge, n, maxSize)
	}
	buf := make([]byte, n)
	if _, err := io.ReadFull(r, buf); err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}
	return buf, nil
}
