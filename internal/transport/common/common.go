package common

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"hash/crc32"
	"sync"
	"time"
)

func RandomID() string {
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%08x", time.Now().UnixNano())
	}
	return hex.EncodeToString(b[:])
}

func FragmentPayload(data []byte, maxSize int) [][]byte {
	if len(data) == 0 {
		return [][]byte{{}}
	}
	out := make([][]byte, 0, (len(data)+maxSize-1)/maxSize)
	for start := 0; start < len(data); start += maxSize {
		end := start + maxSize
		if end > len(data) {
			end = len(data)
		}
		chunk := make([]byte, end-start)
		copy(chunk, data[start:end])
		out = append(out, chunk)
	}
	return out
}

type Fragment struct {
	Seq       uint32
	CRC       uint32
	TotalLen  uint32
	FragIdx   uint16
	FragTotal uint16
	Payload   []byte
}

type InboundMessage struct {
	TotalLen uint32
	CRC      uint32
	frags    [][]byte
	remain   int
}

type Reassembler struct {
	mu        sync.Mutex
	inbound   map[uint32]*InboundMessage
	delivered map[uint32]uint32
	maxRecent int
}

func NewReassembler(maxRecent int) *Reassembler {
	if maxRecent <= 0 {
		maxRecent = 256
	}
	return &Reassembler{
		inbound:   make(map[uint32]*InboundMessage),
		delivered: make(map[uint32]uint32),
		maxRecent: maxRecent,
	}
}

type Result int

const (
	ResultIgnore Result = iota
	ResultPartial
	ResultDuplicate
	ResultDelivered
)

func (r *Reassembler) Push(fragment Fragment) (Result, []byte) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if crc, ok := r.delivered[fragment.Seq]; ok && crc == fragment.CRC {
		return ResultDuplicate, nil
	}

	msg := r.upsert(fragment)
	if int(fragment.FragIdx) >= len(msg.frags) {
		return ResultIgnore, nil
	}

	r.storeChunk(msg, fragment)
	if msg.remain > 0 {
		return ResultPartial, nil
	}
	return r.deliver(fragment.Seq, msg)
}

func (r *Reassembler) upsert(fragment Fragment) *InboundMessage {
	msg, ok := r.inbound[fragment.Seq]
	if ok && msg.CRC == fragment.CRC && msg.TotalLen == fragment.TotalLen &&
		len(msg.frags) == int(fragment.FragTotal) {
		return msg
	}
	msg = &InboundMessage{
		TotalLen: fragment.TotalLen,
		CRC:      fragment.CRC,
		frags:    make([][]byte, fragment.FragTotal),
		remain:   int(fragment.FragTotal),
	}
	r.inbound[fragment.Seq] = msg
	return msg
}

func (r *Reassembler) storeChunk(msg *InboundMessage, fragment Fragment) {
	if msg.frags[fragment.FragIdx] != nil {
		return
	}
	chunk := make([]byte, len(fragment.Payload))
	copy(chunk, fragment.Payload)
	msg.frags[fragment.FragIdx] = chunk
	msg.remain--
}

func (r *Reassembler) deliver(seq uint32, msg *InboundMessage) (Result, []byte) {
	delete(r.inbound, seq)
	data := assemble(msg)
	if crc32.ChecksumIEEE(data) != msg.CRC {
		return ResultIgnore, nil
	}
	if len(r.delivered) > r.maxRecent {
		r.delivered = make(map[uint32]uint32)
	}
	r.delivered[seq] = msg.CRC
	return ResultDelivered, data
}

func assemble(msg *InboundMessage) []byte {
	out := make([]byte, msg.TotalLen)
	offset := 0
	for _, frag := range msg.frags {
		if offset >= len(out) {
			break
		}
		offset += copy(out[offset:], frag)
	}
	return out[:offset]
}

type AckRegistry struct {
	mu      sync.Mutex
	waiters map[uint32]chan uint32
}

func NewAckRegistry() *AckRegistry {
	return &AckRegistry{waiters: make(map[uint32]chan uint32)}
}

func (a *AckRegistry) Register(seq uint32) chan uint32 {
	ch := make(chan uint32, 1)
	a.mu.Lock()
	a.waiters[seq] = ch
	a.mu.Unlock()
	return ch
}

func (a *AckRegistry) Unregister(seq uint32) {
	a.mu.Lock()
	delete(a.waiters, seq)
	a.mu.Unlock()
}

func (a *AckRegistry) Resolve(seq, crc uint32) {
	a.mu.Lock()
	waiter := a.waiters[seq]
	a.mu.Unlock()
	if waiter == nil {
		return
	}
	select {
	case waiter <- crc:
	default:
	}
}
