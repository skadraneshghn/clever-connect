// Package combiner implements the server-side half of the DMB Bonding Engine.
//
// It runs as a WebSocket endpoint on the server panel (typically on Clever Cloud
// behind Sozu/Nginx reverse proxy on port 8080). Multiple client "arteries"
// connect to this combiner over independent WebSocket connections. The combiner:
//
//  1. Receives framed data from N artery connections
//  2. Deduplicates and reorders frames using the session multiplexer
//  3. Forwards reassembled streams to their target destinations
//  4. Returns response data back through the same frame protocol
//  5. Sends PING keepalives for RTT measurement
//
// Authentication is done via a PSK-based HMAC token in the WebSocket upgrade query.
package combiner

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
	"time"

	"clever-connect/internal/bonding/frame"
	"clever-connect/internal/bonding/session"
	"clever-connect/internal/logger"

	"github.com/gorilla/websocket"
)

const (
	// WebSocket configuration
	writeWait      = 10 * time.Second
	pongWait       = 60 * time.Second
	maxMessageSize = 65536

	// Keepalive / RTT measurement
	rttPingInterval = 3 * time.Second
	defaultReorderCap = 64
)

// Combiner is the server-side session reassembler.
type Combiner struct {
	mu sync.RWMutex

	originID  string // identifies this server origin
	pskHex    string // pre-shared key for HMAC authentication
	session   *session.Session
	arteries  map[string]*arteryConn // arteryID → WebSocket connection
	running   bool
	cancelFn  context.CancelFunc
}

// arteryConn represents one client WebSocket artery connection.
type arteryConn struct {
	mu       sync.Mutex
	conn     *websocket.Conn
	arteryID string
	lastPing time.Time
	lastPong time.Time
}

// NewCombiner creates a new server-side combiner.
func NewCombiner(originID, pskHex string) *Combiner {
	return &Combiner{
		originID: originID,
		pskHex:   pskHex,
		arteries: make(map[string]*arteryConn),
		session:  session.NewSession(defaultReorderCap),
	}
}

// IsRunning returns whether the combiner is accepting connections.
func (c *Combiner) IsRunning() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.running
}

// Start begins the combiner's background processing.
func (c *Combiner) Start() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.running {
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	c.cancelFn = cancel
	c.running = true

	// Start the session delivery processor
	go c.processDeliveredFrames(ctx)

	logger.Info("Combiner", "Server combiner started", "origin", c.originID)
}

// Stop gracefully shuts down the combiner.
func (c *Combiner) Stop() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.running {
		return
	}

	if c.cancelFn != nil {
		c.cancelFn()
	}

	// Close all artery connections
	for id, ac := range c.arteries {
		ac.conn.Close()
		delete(c.arteries, id)
	}

	c.session.Close()
	c.running = false

	logger.Info("Combiner", "Server combiner stopped")
}

// Stats returns current combiner statistics.
type CombinerStats struct {
	Running     bool          `json:"running"`
	OriginID    string        `json:"origin_id"`
	ArteryCount int           `json:"artery_count"`
	ArteryStats []ArteryStats `json:"artery_stats"`
}

type ArteryStats struct {
	ArteryID string    `json:"artery_id"`
	LastPing time.Time `json:"last_ping"`
	LastPong time.Time `json:"last_pong"`
}

func (c *Combiner) Stats() CombinerStats {
	c.mu.RLock()
	defer c.mu.RUnlock()

	stats := CombinerStats{
		Running:     c.running,
		OriginID:    c.originID,
		ArteryCount: len(c.arteries),
	}

	for _, ac := range c.arteries {
		stats.ArteryStats = append(stats.ArteryStats, ArteryStats{
			ArteryID: ac.arteryID,
			LastPing: ac.lastPing,
			LastPong: ac.lastPong,
		})
	}

	return stats
}

// validateToken checks the HMAC-SHA256 auth token from the WebSocket upgrade.
func (c *Combiner) validateToken(token, arteryID string) bool {
	if c.pskHex == "" {
		return true // no PSK configured = open (dev mode)
	}

	pskBytes, err := hex.DecodeString(c.pskHex)
	if err != nil {
		return false
	}

	now := time.Now().Unix()
	for delta := int64(0); delta <= 300; delta += 30 {
		ts := now - delta
		message := fmt.Sprintf("%s:%s:%d", c.originID, arteryID, ts)
		mac := hmac.New(sha256.New, pskBytes)
		mac.Write([]byte(message))
		expected := hex.EncodeToString(mac.Sum(nil))
		if hmac.Equal([]byte(token), []byte(expected)) {
			return true
		}
	}

	return false
}

// HandleWebSocket is the HTTP handler for artery WebSocket connections.
// Mount this at: /ws/bonding/combiner?artery=<id>&token=<hmac>
func (c *Combiner) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	arteryID := r.URL.Query().Get("artery")
	token := r.URL.Query().Get("token")

	if arteryID == "" {
		http.Error(w, "missing artery parameter", http.StatusBadRequest)
		return
	}

	if !c.validateToken(token, arteryID) {
		http.Error(w, "invalid token", http.StatusUnauthorized)
		return
	}

	upgrader := websocket.Upgrader{
		ReadBufferSize:  maxMessageSize,
		WriteBufferSize: maxMessageSize,
		CheckOrigin:     func(r *http.Request) bool { return true },
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		logger.Error("Combiner", "WebSocket upgrade failed", "artery", arteryID, "error", err)
		return
	}

	conn.SetReadLimit(maxMessageSize)
	conn.SetReadDeadline(time.Now().Add(pongWait))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	ac := &arteryConn{
		conn:     conn,
		arteryID: arteryID,
	}

	// Register artery (close existing if reconnect)
	c.mu.Lock()
	if old, ok := c.arteries[arteryID]; ok {
		old.conn.Close()
	}
	c.arteries[arteryID] = ac
	c.mu.Unlock()

	logger.Info("Combiner", "Artery connected", "artery", arteryID, "remote", conn.RemoteAddr())

	defer func() {
		c.mu.Lock()
		delete(c.arteries, arteryID)
		c.mu.Unlock()
		conn.Close()
		logger.Info("Combiner", "Artery disconnected", "artery", arteryID)
	}()

	// Start ping sender for RTT measurement
	go c.sendRTTPings(ac)

	// Read loop: receive frames from the client artery
	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				logger.Warn("Combiner", "Artery read error", "artery", arteryID, "error", err)
			}
			return
		}

		// Decode the frame
		f, err := frame.Decode(message)
		if err != nil {
			logger.Warn("Combiner", "Invalid frame from artery", "artery", arteryID, "error", err)
			continue
		}

		// Handle PING frames — reflect back for RTT measurement
		if f.Type == frame.TypePING {
			ac.mu.Lock()
			ac.lastPing = time.Now()
			ac.mu.Unlock()

			pongFrame := frame.NewPingFrame(0, f.Seq)
			encoded, err := pongFrame.Encode()
			if err == nil {
				ac.mu.Lock()
				ac.conn.SetWriteDeadline(time.Now().Add(writeWait))
				ac.conn.WriteMessage(websocket.BinaryMessage, encoded)
				ac.mu.Unlock()
			}
			continue
		}

		// Dispatch to session multiplexer
		c.session.Dispatch(f)

		// Handle OPEN frames: start a new stream connection to the target
		if f.Type == frame.TypeOPEN {
			go c.handleStreamOpen(f.StreamID, string(f.Payload), arteryID)
		}
	}
}

// handleStreamOpen establishes a TCP connection to the target and bridges data.
func (c *Combiner) handleStreamOpen(streamID uint32, target string, primaryArtery string) {
	logger.Info("Combiner", "Opening stream to target",
		"stream", streamID, "target", target, "artery", primaryArtery)

	// Connect to the target
	conn, err := net.DialTimeout("tcp", target, 10*time.Second)
	if err != nil {
		logger.Warn("Combiner", "Failed to connect to target",
			"stream", streamID, "target", target, "error", err)
		c.sendFrameToAllArteries(frame.NewRstFrame(streamID, 0, 0x01))
		return
	}
	defer conn.Close()

	stream := c.session.GetStream(streamID)
	if stream == nil {
		return
	}

	// Target → Client: read from target, frame, send back through arteries
	go func() {
		buf := make([]byte, 4096)
		var seq uint64
		for {
			n, err := conn.Read(buf)
			if n > 0 {
				payload := make([]byte, n)
				copy(payload, buf[:n])
				dataFrame := frame.NewDataFrame(streamID, seq, payload)
				seq++
				c.sendFrameToAllArteries(dataFrame)
			}
			if err != nil {
				if err != io.EOF {
					logger.Warn("Combiner", "Target read error", "stream", streamID, "error", err)
				}
				c.sendFrameToAllArteries(frame.NewFinFrame(streamID, seq))
				return
			}
		}
	}()

	// Client → Target: read ordered frames from stream.Ordered channel
	for f := range stream.Ordered {
		if f.Type == frame.TypeDATA {
			if _, err := conn.Write(f.Payload); err != nil {
				logger.Warn("Combiner", "Target write error", "stream", streamID, "error", err)
				break
			}
		} else if f.Type == frame.TypeFIN || f.Type == frame.TypeRST {
			break
		}
	}

	c.session.CloseStream(streamID)
}

// processDeliveredFrames handles background session processing.
func (c *Combiner) processDeliveredFrames(ctx context.Context) {
	<-ctx.Done()
}

// sendFrameToAllArteries sends a frame through ALL connected arteries (duplication mode).
func (c *Combiner) sendFrameToAllArteries(f *frame.Frame) {
	encoded, err := f.Encode()
	if err != nil {
		return
	}

	c.mu.RLock()
	defer c.mu.RUnlock()

	for _, ac := range c.arteries {
		ac.mu.Lock()
		ac.conn.SetWriteDeadline(time.Now().Add(writeWait))
		err := ac.conn.WriteMessage(websocket.BinaryMessage, encoded)
		ac.mu.Unlock()
		if err != nil {
			logger.Warn("Combiner", "Failed to send frame to artery",
				"artery", ac.arteryID, "error", err)
		}
	}
}

// sendRTTPings periodically sends PING frames for RTT measurement.
func (c *Combiner) sendRTTPings(ac *arteryConn) {
	ticker := time.NewTicker(rttPingInterval)
	defer ticker.Stop()

	var seq uint64
	for range ticker.C {
		ac.mu.Lock()
		if ac.conn == nil {
			ac.mu.Unlock()
			return
		}

		pingFrame := frame.NewPingFrame(0, seq)
		encoded, err := pingFrame.Encode()
		if err == nil {
			ac.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := ac.conn.WriteMessage(websocket.BinaryMessage, encoded); err != nil {
				ac.mu.Unlock()
				return
			}
		}
		ac.lastPing = time.Now()
		seq++
		ac.mu.Unlock()
	}
}
