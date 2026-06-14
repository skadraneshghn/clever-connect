package session

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"clever-connect/internal/bonding/frame"
)

// Session is the top-level multiplexer that manages all active streams.
// It maps StreamID → *Stream and handles frame dispatching.
type Session struct {
	mu sync.RWMutex

	streams     map[uint32]*Stream
	nextStreamID uint32 // monotonic counter for client-originated streams
	reorderCap  int

	// Global stats
	totalFramesIn  uint64
	totalFramesOut uint64
	totalDedups    uint64

	// Lifecycle
	closed    bool
	createdAt time.Time
}

// NewSession creates a new session multiplexer.
// reorderCap controls the per-stream reorder buffer size.
func NewSession(reorderCap int) *Session {
	if reorderCap <= 0 {
		reorderCap = DefaultReorderCap
	}
	return &Session{
		streams:     make(map[uint32]*Stream),
		nextStreamID: 1, // odd = client-initiated, even = server-initiated (future)
		reorderCap:  reorderCap,
		createdAt:   time.Now(),
	}
}

// OpenStream creates a new stream with the given ID.
// If id is 0, the session allocates the next available ID.
// Returns the stream and its ID.
func (s *Session) OpenStream(id uint32) (*Stream, uint32, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil, 0, fmt.Errorf("session: closed")
	}

	if id == 0 {
		id = s.nextStreamID
		s.nextStreamID += 2 // skip by 2 to maintain initiator parity
	}

	if _, exists := s.streams[id]; exists {
		return nil, 0, ErrStreamAlreadyExists
	}

	stream := NewStream(id, s.reorderCap)
	s.streams[id] = stream
	return stream, id, nil
}

// GetStream returns the stream for the given ID, or nil if not found.
func (s *Session) GetStream(id uint32) *Stream {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.streams[id]
}

// GetOrCreateStream returns an existing stream or creates one if it doesn't exist.
// This is used on the receiving side when a remote peer opens a new stream.
func (s *Session) GetOrCreateStream(id uint32) (*Stream, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil, fmt.Errorf("session: closed")
	}

	if stream, exists := s.streams[id]; exists {
		return stream, nil
	}

	stream := NewStream(id, s.reorderCap)
	s.streams[id] = stream
	return stream, nil
}

// Dispatch routes an incoming frame to the appropriate stream.
// For OPEN frames, it auto-creates the stream if needed.
// Returns the stream the frame was dispatched to and the number of frames delivered.
func (s *Session) Dispatch(f *frame.Frame) (*Stream, int, error) {
	atomic.AddUint64(&s.totalFramesIn, 1)

	// PING frames are session-level, not stream-level
	if f.Type == frame.TypePING {
		return nil, 0, nil // handled by the caller
	}

	var stream *Stream
	var err error

	if f.Type == frame.TypeOPEN {
		stream, err = s.GetOrCreateStream(f.StreamID)
	} else {
		stream = s.GetStream(f.StreamID)
		if stream == nil {
			return nil, 0, fmt.Errorf("%w: %d", ErrStreamNotFound, f.StreamID)
		}
	}

	if err != nil {
		return nil, 0, err
	}

	delivered, err := stream.Accept(f)
	if err == ErrDuplicateSeq {
		atomic.AddUint64(&s.totalDedups, 1)
		return stream, 0, nil // dedup is silent, not an error
	}
	if err != nil {
		return stream, 0, err
	}

	atomic.AddUint64(&s.totalFramesOut, uint64(delivered))
	return stream, delivered, nil
}

// CloseStream closes and removes a specific stream.
func (s *Session) CloseStream(id uint32) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if stream, exists := s.streams[id]; exists {
		stream.Close()
		delete(s.streams, id)
	}
}

// Close shuts down the entire session, closing all active streams.
func (s *Session) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return
	}
	s.closed = true

	for id, stream := range s.streams {
		stream.Close()
		delete(s.streams, id)
	}
}

// ActiveStreams returns the number of currently open streams.
func (s *Session) ActiveStreams() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.streams)
}

// Stats returns session-level statistics.
type SessionStats struct {
	ActiveStreams  int    `json:"active_streams"`
	TotalFramesIn  uint64 `json:"total_frames_in"`
	TotalFramesOut uint64 `json:"total_frames_out"`
	TotalDedups    uint64 `json:"total_dedups"`
	Uptime         string `json:"uptime"`
}

func (s *Session) Stats() SessionStats {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return SessionStats{
		ActiveStreams:  len(s.streams),
		TotalFramesIn:  atomic.LoadUint64(&s.totalFramesIn),
		TotalFramesOut: atomic.LoadUint64(&s.totalFramesOut),
		TotalDedups:    atomic.LoadUint64(&s.totalDedups),
		Uptime:         time.Since(s.createdAt).String(),
	}
}

// StreamIDs returns a list of all active stream IDs (for diagnostics).
func (s *Session) StreamIDs() []uint32 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	ids := make([]uint32, 0, len(s.streams))
	for id := range s.streams {
		ids = append(ids, id)
	}
	return ids
}
