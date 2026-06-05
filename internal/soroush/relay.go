package soroush

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"

	"clever-connect/internal/logger"

	"github.com/hashicorp/yamux"
)

// StartRelayHandler runs the server-side outbound relay.
// It accepts incoming yamux streams, reads the SOCKS5 CONNECT target from
// each stream's header, dials the destination, and runs bidirectional I/O.
//
// Each accepted stream is handled in its own goroutine, so this function
// blocks until the yamux session is closed or the context is cancelled.
func StartRelayHandler(ctx context.Context, yamuxSess *yamux.Session) {
	logger.Info(component, "Relay handler started, accepting streams")

	for {
		stream, err := yamuxSess.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				logger.Info(component, "Relay handler shutting down (context cancelled)")
				return
			default:
				if yamuxSess.IsClosed() {
					logger.Info(component, "Relay handler shutting down (yamux session closed)")
					return
				}
				logger.Warn(component, "Relay handler accept error", "error", err)
				return
			}
		}

		go handleRelayStream(ctx, stream)
	}
}

// handleRelayStream handles a single incoming yamux stream.
// Protocol:
//  1. Read 2-byte big-endian length prefix
//  2. Read target address string (e.g., "google.com:443")
//  3. Dial the target
//  4. Bidirectional io.Copy between yamux stream and destination
func handleRelayStream(ctx context.Context, stream net.Conn) {
	defer stream.Close()

	// Read target address header: [2-byte length][target string]
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

	targetBuf := make([]byte, targetLen)
	if _, err := io.ReadFull(stream, targetBuf); err != nil {
		logger.Debug(component, "Relay: failed to read target address", "error", err)
		return
	}
	target := string(targetBuf)

	// Dial the destination
	dialer := &net.Dialer{}
	destConn, err := dialer.DialContext(ctx, "tcp", target)
	if err != nil {
		logger.Warn(component, "Relay: failed to dial target",
			"target", target, "error", err,
		)
		return
	}
	defer destConn.Close()

	logger.Debug(component, "Relay: connected to target", "target", target)

	// Bidirectional copy
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

	select {
	case <-done:
	case <-ctx.Done():
	}

	totalStreams.Add(1)
	logger.Debug(component, "Relay: stream completed",
		"target", target,
		"total", fmt.Sprintf("%d", totalStreams.Load()),
	)
}
