package server

import (
	"context"
	"encoding/binary"
	"time"

	"clever-connect/internal/bonding/frame"
	"clever-connect/internal/logger"

	"github.com/gorilla/websocket"
)

// defaultKeepaliveInterval is the default PING interval to survive
// Cloudflare (~100s) and Sozu reverse proxy idle timeouts.
const defaultKeepaliveInterval = 25 * time.Second

// keepaliveLoop sends periodic PING frames on an artery WebSocket to prevent
// idle timeout disconnections by upstream reverse proxies (nginx, Sozu, Cloudflare).
func keepaliveLoop(ctx context.Context, conn *websocket.Conn, interval time.Duration, arteryID string) {
	if interval <= 0 {
		interval = defaultKeepaliveInterval
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	var nonce uint32

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			nonce++
			sendTimeNs := uint64(time.Now().UnixNano())
			pingFrame := frame.NewPingFrame(nonce, sendTimeNs)

			data, err := pingFrame.Encode()
			if err != nil {
				logger.Error("Combiner", "Failed to encode keepalive PING",
					"artery", arteryID, "error", err)
				continue
			}

			if err := conn.WriteMessage(websocket.BinaryMessage, data); err != nil {
				logger.Warn("Combiner", "Keepalive PING write failed, artery may be disconnected",
					"artery", arteryID, "error", err)
				return
			}
		}
	}
}

// handlePing processes an incoming PING frame by echoing it back as a PONG
// (same nonce + original send timestamp), allowing the sender to compute RTT.
func handlePing(conn *websocket.Conn, f *frame.Frame) {
	// Echo the PING payload back — the sender extracts RTT from the timestamp
	pongFrame := &frame.Frame{
		Version:  frame.Version,
		Type:     frame.TypePING,
		StreamID: 0,
		Seq:      0,
		Payload:  f.Payload,
	}

	data, err := pongFrame.Encode()
	if err != nil {
		return
	}
	_ = conn.WriteMessage(websocket.BinaryMessage, data)
}

// extractPingRTT extracts the RTT from a PONG frame's payload.
// Returns the RTT in milliseconds, or 0 if the payload is malformed.
func extractPingRTT(payload []byte) float64 {
	if len(payload) < 12 {
		return 0
	}
	sendTimeNs := binary.BigEndian.Uint64(payload[4:12])
	nowNs := uint64(time.Now().UnixNano())
	if nowNs <= sendTimeNs {
		return 0
	}
	return float64(nowNs-sendTimeNs) / 1e6 // nanoseconds → milliseconds
}
