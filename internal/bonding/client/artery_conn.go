package client

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/url"
	"sync"
	"sync/atomic"
	"time"

	"clever-connect/internal/bonding/frame"
	"clever-connect/internal/bonding/session"
	"clever-connect/internal/logger"

	"github.com/gorilla/websocket"
)

// ArteryConn wraps a WebSocket connection to the Clever Cloud combiner
// tunnelled through one local xray dokodemo-door artery.
//
// Flow (Mode B):
//
//	Go dispatcher
//	     │  (WebSocket binary frames)
//	     ▼
//	127.0.0.1:21001  (xray dokodemo-door)
//	     │  (transparent TCP relay)
//	     ▼
//	Clever Cloud combiner (WebSocket endpoint /ws/bonding/combiner)
//	     │  (dials final destination)
//	     ▼
//	Internet
type ArteryConn struct {
	mu sync.Mutex

	tag       string // "artery-0", "artery-1", …
	localPort int    // dokodemo-door local port (21001, 21002, …)
	wsConn    *websocket.Conn
	alive     bool

	// Combiner WebSocket path and optional PSK token
	combinerPath string
	pskToken     string
	pskHex       string
	originID     string

	// Reconnect settings
	maxBackoff time.Duration
	baseDelay  time.Duration
}

// NewArteryConn creates a new artery connection wrapper.
func NewArteryConn(tag string, localPort int) *ArteryConn {
	return &ArteryConn{
		tag:          tag,
		localPort:    localPort,
		alive:        false,
		combinerPath: "/ws/bonding/combiner",
		maxBackoff:   30 * time.Second,
		baseDelay:    500 * time.Millisecond,
	}
}

// SetCombinerPath sets the combiner WebSocket path (default: /ws/bonding/combiner).
func (ac *ArteryConn) SetCombinerPath(path string) {
	ac.mu.Lock()
	defer ac.mu.Unlock()
	if path != "" {
		ac.combinerPath = path
	}
}

// SetPSKToken stores the HMAC token for combiner authentication.
func (ac *ArteryConn) SetPSKToken(token string) {
	ac.mu.Lock()
	defer ac.mu.Unlock()
	ac.pskToken = token
}

// SetAuthCredentials sets the PSK and Client Origin ID for token generation.
func (ac *ArteryConn) SetAuthCredentials(pskHex string, originID string) {
	ac.mu.Lock()
	defer ac.mu.Unlock()
	ac.pskHex = pskHex
	ac.originID = originID
}

// Connect establishes a WebSocket connection through the local dokodemo-door port.
// The dokodemo-door transparently relays the TCP stream to the combiner server,
// which upgrades it to WebSocket.
func (ac *ArteryConn) Connect() error {
	ac.mu.Lock()
	defer ac.mu.Unlock()

	if ac.wsConn != nil {
		_ = ac.wsConn.Close()
		ac.wsConn = nil
	}

	// Build the WebSocket URL targeting the local dokodemo-door port.
	// The dokodemo-door relays this to the actual combiner server.
	u := url.URL{
		Scheme: "ws",
		Host:   fmt.Sprintf("127.0.0.1:%d", ac.localPort),
		Path:   ac.combinerPath,
	}

	// Generate fresh HMAC token if PSK and OriginID are provided
	token := ac.pskToken
	if ac.pskHex != "" && ac.originID != "" {
		pskBytes, err := hex.DecodeString(ac.pskHex)
		if err == nil {
			ts := time.Now().Unix()
			message := fmt.Sprintf("%s:%s:%d", ac.originID, ac.tag, ts)
			mac := hmac.New(sha256.New, pskBytes)
			mac.Write([]byte(message))
			token = hex.EncodeToString(mac.Sum(nil))
		}
	}

	// Add artery ID and optional token for combiner authentication
	q := u.Query()
	q.Set("artery", ac.tag)
	if token != "" {
		q.Set("token", token)
	}
	u.RawQuery = q.Encode()

	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}

	// The Host header must match what the combiner expects.
	// Since dokodemo-door is transparent, the combiner sees the real WS upgrade.
	reqHeader := http.Header{}
	reqHeader.Set("User-Agent", "CleverConnect-BondingClient/1.0")

	conn, resp, err := dialer.Dial(u.String(), reqHeader)
	if err != nil {
		status := 0
		if resp != nil {
			status = resp.StatusCode
		}
		ac.alive = false
		return fmt.Errorf("websocket dial to %s failed (HTTP %d): %w", u.String(), status, err)
	}

	ac.wsConn = conn
	ac.alive = true

	logger.Info("Bonding", "Artery WebSocket connected",
		"tag", ac.tag, "port", ac.localPort, "path", ac.combinerPath)
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

// WriteFrame sends a frame as a WebSocket binary message through this artery.
// Thread-safe.
func (ac *ArteryConn) WriteFrame(f *frame.Frame) error {
	ac.mu.Lock()
	defer ac.mu.Unlock()

	if ac.wsConn == nil || !ac.alive {
		return fmt.Errorf("artery %s: connection not established", ac.tag)
	}

	data, err := f.Encode()
	if err != nil {
		return fmt.Errorf("artery %s: frame encode error: %w", ac.tag, err)
	}

	_ = ac.wsConn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	if err := ac.wsConn.WriteMessage(websocket.BinaryMessage, data); err != nil {
		ac.alive = false
		return fmt.Errorf("artery %s: websocket write error: %w", ac.tag, err)
	}
	return nil
}

// ReadFrameLoop reads WebSocket binary messages from this artery and dispatches them.
// Runs as a goroutine; exits on error or context cancellation.
func (ac *ArteryConn) ReadFrameLoop(ctx context.Context, sess *session.Session, pingCB func(float64)) {
	ac.mu.Lock()
	conn := ac.wsConn
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

		_, message, err := conn.ReadMessage()
		if err != nil {
			if ctx.Err() == nil {
				logger.Warn("Bonding", "Artery read error",
					"tag", ac.tag, "error", err)
			}
			ac.mu.Lock()
			ac.alive = false
			ac.mu.Unlock()
			return
		}

		f, err := frame.Decode(message)
		if err != nil {
			logger.Warn("Bonding", "Failed to decode frame from artery",
				"tag", ac.tag, "error", err)
			continue
		}

		// Handle PING for RTT measurement (server reflects PING back)
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
	if ac.wsConn != nil {
		_ = ac.wsConn.WriteMessage(
			websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseNormalClosure, "engine stopped"),
		)
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

// extractPingRTT extracts the RTT from a PING echo frame's payload.
// The payload format is [nonce:4B][sendTimeNs:8B].
func extractPingRTT(payload []byte) float64 {
	if len(payload) < 12 {
		return 0
	}
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

// Simple int-to-string helper (avoids importing strconv just for this).
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
	for i, j := 0, len(buf)-1; i < j; i, j = i+1, j-1 {
		buf[i], buf[j] = buf[j], buf[i]
	}
	return string(buf)
}

// streamIDCounter is a global monotonic counter for client-originated stream IDs.
// Client uses odd numbers (1, 3, 5, …) to avoid collision with server-originated streams.
var streamIDCounter uint32

// nextStreamID allocates the next client-side stream ID.
func nextStreamID() uint32 {
	return atomic.AddUint32(&streamIDCounter, 2) - 1 // 1, 3, 5, 7, …
}
