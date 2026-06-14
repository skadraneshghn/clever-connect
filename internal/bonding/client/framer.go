package client

import (
	"io"
	"sync/atomic"

	"clever-connect/internal/bonding/frame"
	"clever-connect/internal/bonding/session"
	"clever-connect/internal/logger"
)

// Framer handles the conversion between raw TCP connections and DMB frames.
// For each user connection, it:
//  1. Sends an OPEN frame with the target host:port
//  2. Chunks raw bytes into DATA frames with monotonic Seq
//  3. Sends FIN on EOF, RST on error
//  4. Reassembles response frames back into clean TCP bytes

// HandleUpstream reads from a user TCP connection, frames the data,
// and dispatches it through the bonding dispatcher.
// targetAddr is the SOCKS5/HTTP-extracted destination "host:port".
func HandleUpstream(conn io.ReadCloser, streamID uint32, targetAddr string,
	dispatcher *Dispatcher, seq *uint64, frameSize int) {

	if frameSize <= 0 {
		frameSize = 4096
	}

	// 1. Send OPEN frame
	openSeq := atomic.AddUint64(seq, 1)
	openFrame := frame.NewOpenFrame(streamID, openSeq, targetAddr)
	if err := dispatcher.DispatchFrame(openFrame); err != nil {
		logger.Warn("Bonding", "Failed to dispatch OPEN frame",
			"stream", streamID, "target", targetAddr, "error", err)
		return
	}

	// 2. Read loop: chunk raw bytes into DATA frames
	buf := make([]byte, frameSize)
	for {
		n, err := conn.Read(buf)
		if n > 0 {
			dataSeq := atomic.AddUint64(seq, 1)
			payload := make([]byte, n)
			copy(payload, buf[:n])
			dataFrame := frame.NewDataFrame(streamID, dataSeq, payload)

			if dispErr := dispatcher.DispatchFrame(dataFrame); dispErr != nil {
				logger.Warn("Bonding", "Failed to dispatch DATA frame",
					"stream", streamID, "error", dispErr)
			}
		}
		if err != nil {
			if err == io.EOF {
				// 3. Send FIN on clean EOF
				finSeq := atomic.AddUint64(seq, 1)
				finFrame := frame.NewFinFrame(streamID, finSeq)
				_ = dispatcher.DispatchFrame(finFrame)
			} else {
				// 4. Send RST on error
				rstSeq := atomic.AddUint64(seq, 1)
				rstFrame := frame.NewRstFrame(streamID, rstSeq, 0x00)
				_ = dispatcher.DispatchFrame(rstFrame)
			}
			return
		}
	}
}

// HandleDownstream reads ordered frames from a stream's delivery channel
// and writes the reassembled bytes to the user TCP connection.
// It blocks until the stream is closed (FIN/RST received).
func HandleDownstream(conn io.WriteCloser, stream *session.Stream) {
	defer conn.Close()

	for f := range stream.Ordered {
		switch f.Type {
		case frame.TypeDATA:
			if len(f.Payload) > 0 {
				if _, err := conn.Write(f.Payload); err != nil {
					logger.Warn("Bonding", "Write to user connection failed",
						"stream", stream.ID, "error", err)
					return
				}
			}

		case frame.TypeFIN:
			// Clean close — we're done
			return

		case frame.TypeRST:
			// Abort
			return

		case frame.TypeWINDOW:
			// Flow control — update credits (future)
			continue
		}
	}
}
