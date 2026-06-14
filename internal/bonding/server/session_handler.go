package server

import (
	"context"
	"fmt"
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

// BondingSession represents one logical bonding group on the server side.
// Multiple artery WebSocket connections from the same client are grouped
// into a single session, sharing dedup/reorder state and destination connections.
type BondingSession struct {
	mu sync.RWMutex

	OriginID  string
	session   *session.Session
	dedup     *DedupRing
	arteries  map[string]*arteryWS // arteryID → WS connection
	destConns map[uint32]net.Conn  // StreamID → destination TCP connection
	destMu    sync.RWMutex

	// Downstream seq counter (server→client direction)
	downSeq uint64

	// Caps
	maxStreams       int
	maxReorderBytes int

	// Lifecycle
	ctx    context.Context
	cancel context.CancelFunc

	createdAt time.Time
}

// arteryWS wraps a single artery WebSocket connection.
type arteryWS struct {
	conn     *websocket.Conn
	arteryID string
	writeMu  sync.Mutex
}

// NewBondingSession creates a new bonding session for a client origin.
func NewBondingSession(originID string, reorderCap int) *BondingSession {
	ctx, cancel := context.WithCancel(context.Background())
	return &BondingSession{
		OriginID:        originID,
		session:         session.NewSession(reorderCap),
		dedup:           NewDedupRing(8192),
		arteries:        make(map[string]*arteryWS),
		destConns:       make(map[uint32]net.Conn),
		maxStreams:       1000,
		maxReorderBytes: 10 * 1024 * 1024, // 10MB
		ctx:             ctx,
		cancel:          cancel,
		createdAt:       time.Now(),
	}
}

// AddArtery registers a new artery WS connection to this bonding session.
func (bs *BondingSession) AddArtery(arteryID string, conn *websocket.Conn) {
	bs.mu.Lock()
	defer bs.mu.Unlock()

	ws := &arteryWS{
		conn:     conn,
		arteryID: arteryID,
	}
	bs.arteries[arteryID] = ws

	logger.Info("Combiner", "Artery joined bonding session",
		"origin", bs.OriginID, "artery", arteryID,
		"total_arteries", len(bs.arteries))
}

// RemoveArtery unregisters an artery from this session.
func (bs *BondingSession) RemoveArtery(arteryID string) {
	bs.mu.Lock()
	defer bs.mu.Unlock()
	delete(bs.arteries, arteryID)

	logger.Info("Combiner", "Artery left bonding session",
		"origin", bs.OriginID, "artery", arteryID,
		"remaining", len(bs.arteries))
}

// ArteryCount returns the number of active arteries.
func (bs *BondingSession) ArteryCount() int {
	bs.mu.RLock()
	defer bs.mu.RUnlock()
	return len(bs.arteries)
}

// Close shuts down the bonding session and all destination connections.
func (bs *BondingSession) Close() {
	bs.cancel()
	bs.session.Close()

	bs.destMu.Lock()
	for id, conn := range bs.destConns {
		_ = conn.Close()
		delete(bs.destConns, id)
	}
	bs.destMu.Unlock()

	logger.Info("Combiner", "Bonding session closed", "origin", bs.OriginID)
}

// ReadArteryLoop reads frames from one artery WebSocket and processes them.
// This runs as a goroutine per artery connection.
func (bs *BondingSession) ReadArteryLoop(conn *websocket.Conn, arteryID string) {
	for {
		select {
		case <-bs.ctx.Done():
			return
		default:
		}

		_, message, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				logger.Warn("Combiner", "Artery read error",
					"origin", bs.OriginID, "artery", arteryID, "error", err)
			}
			return
		}

		f, err := frame.Decode(message)
		if err != nil {
			logger.Error("Combiner", "Failed to decode frame from artery",
				"origin", bs.OriginID, "artery", arteryID, "error", err)
			continue
		}

		// Handle PING at session level (not dispatched to streams)
		if f.Type == frame.TypePING {
			handlePing(conn, f)
			continue
		}

		// Pre-check dedup before dispatching to session
		if f.Type == frame.TypeDATA || f.Type == frame.TypeOPEN {
			if !bs.dedup.Check(f.StreamID, f.Seq) {
				continue // duplicate, silently drop
			}
		}

		// Enforce stream cap
		if f.Type == frame.TypeOPEN && bs.session.ActiveStreams() >= bs.maxStreams {
			logger.Warn("Combiner", "Stream limit reached, rejecting OPEN",
				"origin", bs.OriginID, "max", bs.maxStreams)
			bs.sendRST(f.StreamID, 0x01) // reason: capacity
			continue
		}

		// Dispatch to session (handles reorder + dedup at stream level)
		stream, _, err := bs.session.Dispatch(f)
		if err != nil {
			logger.Error("Combiner", "Frame dispatch error",
				"origin", bs.OriginID, "artery", arteryID,
				"stream", f.StreamID, "type", frame.TypeName(f.Type),
				"error", err)
			continue
		}

		// For OPEN frames, start the stream processing goroutine
		if f.Type == frame.TypeOPEN && stream != nil {
			go bs.processStream(stream, f.StreamID)
		}
	}
}

// processStream runs as a goroutine per stream. It reads ordered frames
// from the stream's delivery channel and handles them:
//   - OPEN: dial the real destination
//   - DATA: write to destination
//   - FIN: half-close destination
//   - RST: abort destination
func (bs *BondingSession) processStream(stream *session.Stream, streamID uint32) {
	defer func() {
		bs.cleanupStream(streamID)
		bs.session.CloseStream(streamID)
	}()

	for f := range stream.Ordered {
		switch f.Type {
		case frame.TypeOPEN:
			targetAddr := string(f.Payload)
			if targetAddr == "" {
				logger.Warn("Combiner", "OPEN frame with empty target",
					"origin", bs.OriginID, "stream", streamID)
				bs.sendRST(streamID, 0x02) // reason: bad target
				return
			}

			conn, err := net.DialTimeout("tcp", targetAddr, 10*time.Second)
			if err != nil {
				logger.Warn("Combiner", "Failed to dial destination",
					"origin", bs.OriginID, "stream", streamID,
					"target", targetAddr, "error", err)
				bs.sendRST(streamID, 0x03) // reason: connect failed
				return
			}

			bs.destMu.Lock()
			bs.destConns[streamID] = conn
			bs.destMu.Unlock()

			logger.Info("Combiner", "Stream opened to destination",
				"origin", bs.OriginID, "stream", streamID,
				"target", targetAddr)

			// Start downstream relay (destination → client via arteries)
			go bs.downstreamRelay(conn, streamID)

		case frame.TypeDATA:
			bs.destMu.RLock()
			conn, ok := bs.destConns[streamID]
			bs.destMu.RUnlock()
			if !ok || conn == nil {
				continue // destination not connected yet (shouldn't happen with ordered delivery)
			}

			if _, err := conn.Write(f.Payload); err != nil {
				logger.Warn("Combiner", "Write to destination failed",
					"origin", bs.OriginID, "stream", streamID, "error", err)
				bs.sendRST(streamID, 0x04) // reason: write error
				return
			}

		case frame.TypeFIN:
			bs.destMu.RLock()
			conn, ok := bs.destConns[streamID]
			bs.destMu.RUnlock()
			if ok && conn != nil {
				if tcpConn, ok := conn.(*net.TCPConn); ok {
					_ = tcpConn.CloseWrite()
				}
			}
			return

		case frame.TypeRST:
			return
		}
	}
}

// downstreamRelay reads from a destination TCP connection and sends
// framed DATA/FIN frames back through all active artery WebSocket connections.
func (bs *BondingSession) downstreamRelay(destConn net.Conn, streamID uint32) {
	buf := make([]byte, 4096)

	for {
		select {
		case <-bs.ctx.Done():
			return
		default:
		}

		n, err := destConn.Read(buf)
		if n > 0 {
			seq := atomic.AddUint64(&bs.downSeq, 1)
			dataFrame := frame.NewDataFrame(streamID, seq, buf[:n])
			bs.broadcastFrame(dataFrame)
		}
		if err != nil {
			if err != io.EOF {
				logger.Warn("Combiner", "Destination read error",
					"origin", bs.OriginID, "stream", streamID, "error", err)
			}
			// Send FIN to client
			seq := atomic.AddUint64(&bs.downSeq, 1)
			finFrame := frame.NewFinFrame(streamID, seq)
			bs.broadcastFrame(finFrame)
			return
		}
	}
}

// broadcastFrame sends a frame to all active artery WebSocket connections.
// Used for downstream (server→client) traffic duplication.
func (bs *BondingSession) broadcastFrame(f *frame.Frame) {
	data, err := f.Encode()
	if err != nil {
		return
	}

	bs.mu.RLock()
	arteries := make([]*arteryWS, 0, len(bs.arteries))
	for _, a := range bs.arteries {
		arteries = append(arteries, a)
	}
	bs.mu.RUnlock()

	for _, a := range arteries {
		a.writeMu.Lock()
		err := a.conn.WriteMessage(websocket.BinaryMessage, data)
		a.writeMu.Unlock()
		if err != nil {
			logger.Warn("Combiner", "Failed to broadcast frame to artery",
				"origin", bs.OriginID, "artery", a.arteryID, "error", err)
		}
	}
}

// sendRST sends an RST frame for a stream to the client via all arteries.
func (bs *BondingSession) sendRST(streamID uint32, reason byte) {
	seq := atomic.AddUint64(&bs.downSeq, 1)
	rstFrame := frame.NewRstFrame(streamID, seq, reason)
	bs.broadcastFrame(rstFrame)
}

// cleanupStream removes and closes the destination connection for a stream.
func (bs *BondingSession) cleanupStream(streamID uint32) {
	bs.destMu.Lock()
	if conn, ok := bs.destConns[streamID]; ok {
		_ = conn.Close()
		delete(bs.destConns, streamID)
	}
	bs.destMu.Unlock()
}

// Stats returns session statistics for telemetry.
func (bs *BondingSession) Stats() SessionStats {
	bs.mu.RLock()
	arteryCount := len(bs.arteries)
	bs.mu.RUnlock()

	bs.destMu.RLock()
	destCount := len(bs.destConns)
	bs.destMu.RUnlock()

	sessStats := bs.session.Stats()

	return SessionStats{
		OriginID:     bs.OriginID,
		ArteryCount:  arteryCount,
		ActiveStreams: destCount,
		FramesIn:     sessStats.TotalFramesIn,
		FramesOut:    sessStats.TotalFramesOut,
		Dedups:       sessStats.TotalDedups,
		Uptime:       time.Since(bs.createdAt).String(),
	}
}

// SessionStats is the telemetry snapshot for a bonding session.
type SessionStats struct {
	OriginID     string `json:"origin_id"`
	ArteryCount  int    `json:"artery_count"`
	ActiveStreams int    `json:"active_streams"`
	FramesIn     uint64 `json:"frames_in"`
	FramesOut    uint64 `json:"frames_out"`
	Dedups       uint64 `json:"dedups"`
	Uptime       string `json:"uptime"`
}

// nextArteryID generates a unique artery identifier.
var arteryCounter uint64

func nextArteryID() string {
	id := atomic.AddUint64(&arteryCounter, 1)
	return fmt.Sprintf("artery-%d", id)
}
