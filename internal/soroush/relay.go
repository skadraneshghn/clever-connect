package soroush

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"time"

	"clever-connect/internal/logger"

	"github.com/quic-go/quic-go"
)

// ──────────────────────────────────────────────────────────────────────────────
// Server-side relay: HandleServerRelay
//
// Each QUIC stream carries a single proxy request. Protocol:
//   1. Read 2-byte big-endian length prefix
//   2. Read target address string (e.g., "google.com:443")
//   3. Dial the real internet destination
//   4. Bidirectional io.Copy between QUIC stream and destination
// ──────────────────────────────────────────────────────────────────────────────

// HandleServerRelay handles a single incoming QUIC stream on the server side.
// It reads the target address, dials the real destination, and proxies bytes
// bidirectionally until either side closes.
func HandleServerRelay(stream *quic.Stream) {
	defer stream.Close()

	// 1. Read target address header: [2-byte length][target string]
	lenBuf := make([]byte, 2)
	if _, err := io.ReadFull(stream, lenBuf); err != nil {
		logger.Debug(component, "Relay: failed to read target header", "error", err)
		return
	}
	targetLen := binary.BigEndian.Uint16(lenBuf)
	if targetLen == 0 || targetLen > 512 {
		logger.Warn(component, "Relay: invalid target length", "length", targetLen)
		return
	}

	// 2. Read the target address (e.g., "youtube.com:443")
	targetBuf := make([]byte, targetLen)
	if _, err := io.ReadFull(stream, targetBuf); err != nil {
		logger.Debug(component, "Relay: failed to read target address", "error", err)
		return
	}
	targetAddr := string(targetBuf)

	// 3. Dial the real internet destination
	destConn, err := net.DialTimeout("tcp", targetAddr, 10*time.Second)
	if err != nil {
		logger.Warn(component, "Relay: failed to dial target",
			"target", targetAddr, "error", err,
		)
		return
	}
	defer destConn.Close()

	logger.Debug(component, "Relay: connected to target", "target", targetAddr)

	// 4. Bidirectional copy
	done := make(chan struct{}, 2)

	go func() {
		n, _ := io.Copy(destConn, stream)
		bytesRelayed.Add(n)
		done <- struct{}{}
	}()

	go func() {
		n, _ := io.Copy(stream, destConn)
		bytesRelayed.Add(n)
		done <- struct{}{}
	}()

	<-done

	totalStreams.Add(1)
	logger.Debug(component, "Relay: stream completed",
		"target", targetAddr,
		"total", fmt.Sprintf("%d", totalStreams.Load()),
	)
}

// ──────────────────────────────────────────────────────────────────────────────
// Client-side relay: dialQuicStream
//
// When a local SOCKS5 app connects, the client opens a new QUIC stream,
// writes the target address header, and bridges the connections.
// ──────────────────────────────────────────────────────────────────────────────

// dialQuicStream opens a new QUIC stream for a specific proxy request
// and bridges it to the local client connection.
func dialQuicStream(ctx context.Context, quicConn *quic.Conn, targetAddr string, clientConn net.Conn) {
	// 1. Open a new stream for this specific proxy request
	stream, err := quicConn.OpenStreamSync(ctx)
	if err != nil {
		logger.Warn(component, "QUIC: failed to open stream", "error", err, "target", targetAddr)
		clientConn.Close()
		return
	}
	defer stream.Close()

	// 2. Write target address as a header: [2-byte length][target string]
	targetBytes := []byte(targetAddr)
	header := make([]byte, 2+len(targetBytes))
	binary.BigEndian.PutUint16(header[:2], uint16(len(targetBytes)))
	copy(header[2:], targetBytes)

	if _, err := stream.Write(header); err != nil {
		logger.Warn(component, "QUIC: failed to write target header", "error", err, "target", targetAddr)
		return
	}

	// 3. Bridge the connections
	done := make(chan struct{}, 2)

	go func() {
		n, _ := io.Copy(stream, clientConn)
		bytesRelayed.Add(n)
		done <- struct{}{}
	}()

	go func() {
		n, _ := io.Copy(clientConn, stream)
		bytesRelayed.Add(n)
		done <- struct{}{}
	}()

	<-done

	totalStreams.Add(1)
}
