package soroushlib

import (
	"io"
	"net"
	"sync"
	"time"
)

const (
	// MaxChunkSize is the maximum payload size per LiveKit data packet.
	// Empirically safe limit to avoid SFU-side fragmentation and drops.
	MaxChunkSize = 15_000 // 15KB per packet
)

// VConn wraps LiveKit's packet-based PublishData/OnDataReceived
// into a net.Conn interface that yamux can consume as a streaming socket.
//
// Write() slices outgoing data into MaxChunkSize chunks and calls publishFn.
// Read() pulls from a thread-safe synchronized ring buffer filled by OnData().
type VConn struct {
	// Write side: slices data into chunks → PublishData
	publishFn func(data []byte) error

	// Read side: OnDataReceived callback pushes into ring buffer
	readBuf  *RingBuffer
	readCond *sync.Cond

	closed chan struct{}

	// Concurrency polish: prevents late OnData calls from panicking
	mu       sync.Mutex
	isClosed bool
}

// NewVConn creates a new VConn adapter.
// publishFn is called for each outgoing chunk (typically room.LocalParticipant.PublishData).
// bufSize is the ring buffer capacity in bytes (recommended: 4MB = 4*1024*1024).
func NewVConn(publishFn func([]byte) error, bufSize int) *VConn {
	condMu := &sync.Mutex{}
	return &VConn{
		publishFn: publishFn,
		readBuf:   NewRingBuffer(bufSize),
		readCond:  sync.NewCond(condMu),
		closed:    make(chan struct{}),
	}
}

// Write slices data into MaxChunkSize packets and publishes each.
// This is called by yamux when it wants to send multiplexed stream data.
func (vc *VConn) Write(p []byte) (int, error) {
	select {
	case <-vc.closed:
		return 0, io.ErrClosedPipe
	default:
	}

	total := 0
	for len(p) > 0 {
		chunk := p
		if len(chunk) > MaxChunkSize {
			chunk = p[:MaxChunkSize]
		}
		if err := vc.publishFn(chunk); err != nil {
			return total, err
		}
		total += len(chunk)
		p = p[len(chunk):]
	}
	return total, nil
}

// Read pulls from the ring buffer, blocking if empty.
// This is called by yamux when it wants to receive multiplexed stream data.
func (vc *VConn) Read(p []byte) (int, error) {
	vc.readCond.L.Lock()
	defer vc.readCond.L.Unlock()

	for vc.readBuf.Len() == 0 {
		select {
		case <-vc.closed:
			return 0, io.EOF
		default:
		}
		vc.readCond.Wait()
	}
	return vc.readBuf.Read(p)
}

// OnData is the callback registered with LiveKit's OnDataReceived.
// It pushes incoming packets into the ring buffer and wakes up Read().
// Safe to call after Close() — late packets are silently dropped.
func (vc *VConn) OnData(data []byte) {
	vc.mu.Lock()
	if vc.isClosed {
		vc.mu.Unlock()
		return // Safely drop late packets after close
	}
	vc.mu.Unlock()

	vc.readCond.L.Lock()
	vc.readBuf.Write(data)
	vc.readCond.L.Unlock()
	vc.readCond.Signal()
}

// Close shuts down the VConn, waking any blocked Read() calls.
// Safe to call multiple times.
func (vc *VConn) Close() error {
	vc.mu.Lock()
	if vc.isClosed {
		vc.mu.Unlock()
		return nil
	}
	vc.isClosed = true
	vc.mu.Unlock()

	close(vc.closed)
	vc.readCond.Broadcast()
	return nil
}

// net.Conn interface stubs — VConn has no real network addresses
func (vc *VConn) LocalAddr() net.Addr                { return vconnAddr{} }
func (vc *VConn) RemoteAddr() net.Addr               { return vconnAddr{} }
func (vc *VConn) SetDeadline(t time.Time) error      { return nil }
func (vc *VConn) SetReadDeadline(t time.Time) error  { return nil }
func (vc *VConn) SetWriteDeadline(t time.Time) error { return nil }

type vconnAddr struct{}

func (vconnAddr) Network() string { return "livekit-vconn" }
func (vconnAddr) String() string  { return "livekit://sfu" }

// Compile-time check that VConn implements net.Conn
var _ net.Conn = (*VConn)(nil)
