// Package session implements per-stream reorder buffers and the session-level
// multiplexer for DMB bonding. Each user TCP connection maps to a StreamID,
// and each stream maintains independent sequence numbering and flow control.
package session

import (
	"errors"
	"sync"
	"time"

	"clever-connect/internal/bonding/frame"
)

// Errors
var (
	ErrStreamClosed       = errors.New("session: stream is closed")
	ErrBufferFull         = errors.New("session: reorder buffer capacity exceeded")
	ErrDuplicateSeq       = errors.New("session: duplicate sequence number")
	ErrStreamAlreadyExists = errors.New("session: stream ID already exists")
	ErrStreamNotFound     = errors.New("session: stream ID not found")
)

// DefaultReorderCap is the max number of out-of-order frames buffered per stream.
const DefaultReorderCap = 256

// DefaultWindowSize is the initial per-stream receive window (in frames).
const DefaultWindowSize uint32 = 128

// StreamState represents the lifecycle state of a stream.
type StreamState int

const (
	StreamStateOpen    StreamState = iota // Actively sending/receiving
	StreamStateHalfClosed                 // FIN sent or received (one direction closed)
	StreamStateClosed                     // Fully closed (both directions)
	StreamStateReset                      // Aborted by RST
)

// String returns a human-readable representation of the stream state.
func (s StreamState) String() string {
	switch s {
	case StreamStateOpen:
		return "OPEN"
	case StreamStateHalfClosed:
		return "HALF_CLOSED"
	case StreamStateClosed:
		return "CLOSED"
	case StreamStateReset:
		return "RESET"
	default:
		return "UNKNOWN"
	}
}

// Stream represents a single user connection multiplexed over the bonding session.
// It handles reorder buffering, deduplication, and flow control for one StreamID.
type Stream struct {
	mu sync.Mutex

	ID       uint32
	State    StreamState
	NextSeq  uint64            // next expected sequence number
	Window   uint32            // available receive credit (frames)
	InFlight uint32            // frames sent but not yet acknowledged

	// Reorder buffer: maps Seq → *frame.Frame for out-of-order arrivals
	reorderBuf map[uint64]*frame.Frame
	reorderCap int

	// Deduplication: tracks seen sequence numbers (ring of last N)
	seenSeqs map[uint64]struct{}
	seenCap  int

	// Ordered output channel for successfully reassembled frames
	Ordered chan *frame.Frame

	// Timestamps
	CreatedAt   time.Time
	LastActiveAt time.Time
}

// NewStream creates a new stream with the given ID and default configuration.
func NewStream(id uint32, reorderCap int) *Stream {
	if reorderCap <= 0 {
		reorderCap = DefaultReorderCap
	}
	return &Stream{
		ID:         id,
		State:      StreamStateOpen,
		NextSeq:    1,
		Window:     DefaultWindowSize,
		reorderBuf: make(map[uint64]*frame.Frame, reorderCap),
		reorderCap: reorderCap,
		seenSeqs:   make(map[uint64]struct{}, reorderCap*2),
		seenCap:    reorderCap * 2,
		Ordered:    make(chan *frame.Frame, reorderCap),
		CreatedAt:  time.Now(),
		LastActiveAt: time.Now(),
	}
}

// Accept processes an incoming frame for this stream.
// It handles deduplication, reorder buffering, and sequential delivery.
// Returns the number of frames successfully delivered to the Ordered channel.
func (s *Stream) Accept(f *frame.Frame) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.State == StreamStateClosed || s.State == StreamStateReset {
		return 0, ErrStreamClosed
	}

	s.LastActiveAt = time.Now()

	// Handle control frames immediately
	switch f.Type {
	case frame.TypeFIN:
		delivered := s.drainReorderBuf()
		s.deliverLocked(f)
		if s.State == StreamStateHalfClosed {
			s.State = StreamStateClosed
		} else {
			s.State = StreamStateHalfClosed
		}
		return delivered + 1, nil

	case frame.TypeRST:
		s.State = StreamStateReset
		s.deliverLocked(f)
		return 1, nil

	case frame.TypeWINDOW:
		// Window update doesn't go through reorder
		s.deliverLocked(f)
		return 1, nil
	}

	// DATA/OPEN frames go through the reorder pipeline

	// 1. Deduplication check
	if _, seen := s.seenSeqs[f.Seq]; seen {
		return 0, ErrDuplicateSeq
	}
	s.markSeen(f.Seq)

	// 2. If this is the next expected frame, deliver it and drain the reorder buffer
	if f.Seq == s.NextSeq {
		s.deliverLocked(f)
		s.NextSeq++
		delivered := 1 + s.drainReorderBuf()
		return delivered, nil
	}

	// 3. Out-of-order: buffer it
	if f.Seq < s.NextSeq {
		// Already delivered (late duplicate that missed the dedup window)
		return 0, ErrDuplicateSeq
	}

	if len(s.reorderBuf) >= s.reorderCap {
		return 0, ErrBufferFull
	}

	s.reorderBuf[f.Seq] = f
	return 0, nil
}

// drainReorderBuf delivers consecutive frames from the reorder buffer.
// Must be called with s.mu held.
func (s *Stream) drainReorderBuf() int {
	delivered := 0
	for {
		f, ok := s.reorderBuf[s.NextSeq]
		if !ok {
			break
		}
		delete(s.reorderBuf, s.NextSeq)
		s.deliverLocked(f)
		s.NextSeq++
		delivered++
	}
	return delivered
}

// deliverLocked sends a frame to the Ordered channel.
// Must be called with s.mu held. Non-blocking: drops if channel full.
func (s *Stream) deliverLocked(f *frame.Frame) {
	select {
	case s.Ordered <- f:
	default:
		// Channel full — this shouldn't happen with proper sizing,
		// but we never block the ingestion goroutine.
	}
}

// markSeen records a sequence number as seen for deduplication.
// If the seen set exceeds capacity, it's pruned (simple reset strategy).
func (s *Stream) markSeen(seq uint64) {
	s.seenSeqs[seq] = struct{}{}
	if len(s.seenSeqs) > s.seenCap {
		// Simple pruning: remove entries far below current NextSeq
		for k := range s.seenSeqs {
			if k < s.NextSeq-uint64(s.seenCap/2) {
				delete(s.seenSeqs, k)
			}
		}
	}
}

// ReorderBufferLen returns the current number of out-of-order frames buffered.
func (s *Stream) ReorderBufferLen() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.reorderBuf)
}

// PendingDelivery returns the number of frames ready in the ordered channel.
func (s *Stream) PendingDelivery() int {
	return len(s.Ordered)
}

// Close marks the stream as closed and drains the ordered channel.
func (s *Stream) Close() {
	s.mu.Lock()
	s.State = StreamStateClosed
	s.mu.Unlock()
	close(s.Ordered)
}
