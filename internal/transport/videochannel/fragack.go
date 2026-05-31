package videochannel

import (
	"sync"
	"sync/atomic"
)

type fragAckTracker struct {
	mu      sync.RWMutex
	pending map[uint32]*fragWaiter
}

type fragWaiter struct {
	crc       uint32
	total     int
	words     []atomic.Uint64
	remaining atomic.Int64
	notify    chan struct{}
}

func newFragAckTracker() *fragAckTracker {
	return &fragAckTracker{pending: make(map[uint32]*fragWaiter)}
}

func (t *fragAckTracker) Register(seq, crc uint32, total int) *fragWaiter {
	w := &fragWaiter{
		crc:    crc,
		total:  total,
		words:  make([]atomic.Uint64, (total+63)>>6),
		notify: make(chan struct{}, 1),
	}
	w.remaining.Store(int64(total))
	t.mu.Lock()
	t.pending[seq] = w
	t.mu.Unlock()
	return w
}

func (t *fragAckTracker) Unregister(seq uint32) {
	t.mu.Lock()
	delete(t.pending, seq)
	t.mu.Unlock()
}

func (t *fragAckTracker) Mark(seq, crc uint32, fragIdx int) bool {
	t.mu.RLock()
	w, ok := t.pending[seq]
	t.mu.RUnlock()
	if !ok {
		return false
	}
	if w.crc != crc || fragIdx < 0 || fragIdx >= w.total {
		return false
	}
	word := fragIdx >> 6
	bit := uint64(1) << uint(fragIdx&63)
	for {
		old := w.words[word].Load()
		if old&bit != 0 {
			return false
		}
		if w.words[word].CompareAndSwap(old, old|bit) {
			break
		}
	}
	w.remaining.Add(-1)
	select {
	case w.notify <- struct{}{}:
	default:
	}
	return true
}

func (w *fragWaiter) Pending() []int {
	out := make([]int, 0, w.remaining.Load())
	for wi := range w.words {
		loaded := w.words[wi].Load()
		base := wi << 6
		end := 64
		if base+end > w.total {
			end = w.total - base
		}
		for b := 0; b < end; b++ {
			if loaded&(uint64(1)<<uint(b)) == 0 {
				out = append(out, base+b)
			}
		}
	}
	return out
}

func (w *fragWaiter) Done() bool {
	return w.remaining.Load() == 0
}

func (w *fragWaiter) Notify() <-chan struct{} {
	return w.notify
}
