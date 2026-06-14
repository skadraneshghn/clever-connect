package client

import (
	"context"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"clever-connect/internal/bonding/frame"
	"clever-connect/internal/bonding/session"
	"clever-connect/internal/logger"

	"github.com/gorilla/websocket"
)

// ArteryConn wraps a WebSocket connection to the combiner through one
// xray dokodemo-door artery. It handles frame writing, reading, and reconnection.
type ArteryConn struct {
	mu sync.Mutex

	tag       string // "artery-0", "artery-1", ...
	localPort int    // dokodemo-door port (21001, 21002, ...)
	wsConn    *websocket.Conn
	tcpConn   net.Conn
	alive     bool

	// Reconnect settings
	maxBackoff time.Duration
	baseDelay  time.Duration
}

// NewArteryConn creates a new artery connection wrapper.
func NewArteryConn(tag string, localPort int) *ArteryConn {
	return &ArteryConn{
		tag:        tag,
		localPort:  localPort,
		alive:      false,
		maxBackoff: 30 * time.Second,
		baseDelay:  500 * time.Millisecond,
	}
}

// Connect establishes a TCP connection to the local dokodemo-door port.
func (ac *ArteryConn) Connect() error {
	ac.mu.Lock()
	defer ac.mu.Unlock()

	if ac.tcpConn != nil {
		_ = ac.tcpConn.Close()
	}

	addr := net.JoinHostPort("127.0.0.1", itoa(ac.localPort))
	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		ac.alive = false
		return err
	}

	ac.tcpConn = conn
	ac.alive = true

	logger.Info("Bonding", "Artery connected",
		"tag", ac.tag, "port", ac.localPort)
	return nil
}

// ConnectWithBackoff tries to connect with exponential backoff.
func (ac *ArteryConn) ConnectWithBackoff(ctx context.Context) error {
	delay := ac.baseDelay

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if err := ac.Connect(); err != nil {
			logger.Warn("Bonding", "Artery connect failed, retrying",
				"tag", ac.tag, "delay", delay, "error", err)

			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(delay):
			}

			delay = delay * 2
			if delay > ac.maxBackoff {
				delay = ac.maxBackoff
			}
			continue
		}
		return nil
	}
}

// WriteFrame sends a frame through this artery's TCP connection.
// Thread-safe.
func (ac *ArteryConn) WriteFrame(f *frame.Frame) error {
	ac.mu.Lock()
	defer ac.mu.Unlock()

	if ac.tcpConn == nil || !ac.alive {
		return io.ErrClosedPipe
	}

	return frame.WriteFrame(ac.tcpConn, f)
}

// ReadFrameLoop reads frames from this artery and dispatches them to the session.
// Runs as a goroutine; exits on error or context cancellation.
func (ac *ArteryConn) ReadFrameLoop(ctx context.Context, sess *session.Session, pingCB func(float64)) {
	ac.mu.Lock()
	conn := ac.tcpConn
	ac.mu.Unlock()

	if conn == nil {
		return
	}

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		f, err := frame.ReadFrame(conn)
		if err != nil {
			if err != io.EOF {
				logger.Warn("Bonding", "Artery read error",
					"tag", ac.tag, "error", err)
			}
			ac.mu.Lock()
			ac.alive = false
			ac.mu.Unlock()
			return
		}

		// Handle PING/PONG for RTT measurement
		if f.Type == frame.TypePING {
			rtt := extractPingRTT(f.Payload)
			if rtt > 0 && pingCB != nil {
				pingCB(rtt)
			}
			continue
		}

		// Dispatch to session for dedup/reorder
		_, _, err = sess.Dispatch(f)
		if err != nil {
			logger.Warn("Bonding", "Frame dispatch error",
				"tag", ac.tag, "stream", f.StreamID,
				"type", frame.TypeName(f.Type), "error", err)
		}
	}
}

// IsAlive returns whether this artery connection is currently healthy.
func (ac *ArteryConn) IsAlive() bool {
	ac.mu.Lock()
	defer ac.mu.Unlock()
	return ac.alive
}

// Close shuts down the artery connection.
func (ac *ArteryConn) Close() {
	ac.mu.Lock()
	defer ac.mu.Unlock()

	ac.alive = false
	if ac.tcpConn != nil {
		_ = ac.tcpConn.Close()
		ac.tcpConn = nil
	}
	if ac.wsConn != nil {
		_ = ac.wsConn.Close()
		ac.wsConn = nil
	}
}

// Tag returns the artery tag.
func (ac *ArteryConn) Tag() string {
	return ac.tag
}

// LocalPort returns the dokodemo-door port.
func (ac *ArteryConn) LocalPort() int {
	return ac.localPort
}

// extractPingRTT extracts the RTT from a PONG frame's payload (client-side).
func extractPingRTT(payload []byte) float64 {
	if len(payload) < 12 {
		return 0
	}
	// The payload format is [nonce:4B][sendTimeNs:8B]
	sendTimeNs := uint64(0)
	for i := 4; i < 12; i++ {
		sendTimeNs = sendTimeNs<<8 | uint64(payload[i])
	}
	nowNs := uint64(time.Now().UnixNano())
	if nowNs <= sendTimeNs {
		return 0
	}
	return float64(nowNs-sendTimeNs) / 1e6 // ns → ms
}

// Simple int to string helper to avoid importing strconv just for this.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := false
	if n < 0 {
		neg = true
		n = -n
	}
	buf := make([]byte, 0, 10)
	for n > 0 {
		buf = append(buf, byte('0'+n%10))
		n /= 10
	}
	if neg {
		buf = append(buf, '-')
	}
	// reverse
	for i, j := 0, len(buf)-1; i < j; i, j = i+1, j-1 {
		buf[i], buf[j] = buf[j], buf[i]
	}
	return string(buf)
}

// streamIDCounter is a global monotonic counter for client-originated stream IDs.
// Odd numbers for client-initiated streams.
var streamIDCounter uint32

// nextStreamID allocates the next stream ID.
func nextStreamID() uint32 {
	return atomic.AddUint32(&streamIDCounter, 2) - 1 // 1, 3, 5, 7, ...
}
