package soroushlib

import (
	"sync"
)

// RingBuffer is a simple thread-safe circular buffer providing FIFO semantics.
// It is used by VConn to bridge the gap between LiveKit's async packet
// callbacks (OnDataReceived) and yamux's blocking Read() calls.
type RingBuffer struct {
	mu   sync.Mutex
	buf  []byte
	r, w int
	full bool
	size int
}

// NewRingBuffer creates a new ring buffer with the given capacity in bytes.
func NewRingBuffer(size int) *RingBuffer {
	return &RingBuffer{
		buf:  make([]byte, size),
		size: size,
	}
}

// Write appends data to the ring buffer. If the buffer is full,
// oldest data is silently overwritten (lossy behavior — acceptable
// for high-throughput tunnel where yamux handles retransmission).
func (rb *RingBuffer) Write(p []byte) (int, error) {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	n := len(p)
	for i := 0; i < n; i++ {
		rb.buf[rb.w] = p[i]
		rb.w = (rb.w + 1) % rb.size

		if rb.full {
			// Overwrite: advance read pointer
			rb.r = (rb.r + 1) % rb.size
		}
		if rb.w == rb.r {
			rb.full = true
		}
	}
	return n, nil
}

// Read copies data from the ring buffer into p.
// Returns the number of bytes read. Does NOT block — caller
// should check Len() or use sync.Cond before calling Read.
func (rb *RingBuffer) Read(p []byte) (int, error) {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	available := rb.lenLocked()
	if available == 0 {
		return 0, nil
	}

	n := len(p)
	if n > available {
		n = available
	}

	for i := 0; i < n; i++ {
		p[i] = rb.buf[rb.r]
		rb.r = (rb.r + 1) % rb.size
	}
	rb.full = false
	return n, nil
}

// Len returns the number of unread bytes in the buffer.
func (rb *RingBuffer) Len() int {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	return rb.lenLocked()
}

func (rb *RingBuffer) lenLocked() int {
	if rb.full {
		return rb.size
	}
	if rb.w >= rb.r {
		return rb.w - rb.r
	}
	return rb.size - rb.r + rb.w
}

// Reset clears the buffer.
func (rb *RingBuffer) Reset() {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	rb.r = 0
	rb.w = 0
	rb.full = false
}
