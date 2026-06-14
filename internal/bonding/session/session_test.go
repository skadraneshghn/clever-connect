package session

import (
	"sync"
	"testing"

	"clever-connect/internal/bonding/frame"
)

// TestStreamInOrderDelivery verifies that frames arriving in order are delivered immediately.
func TestStreamInOrderDelivery(t *testing.T) {
	s := NewStream(1, 32)

	for seq := uint64(1); seq <= 5; seq++ {
		f := frame.NewDataFrame(1, seq, []byte("data"))
		delivered, err := s.Accept(f)
		if err != nil {
			t.Fatalf("seq %d: Accept() error: %v", seq, err)
		}
		if delivered != 1 {
			t.Errorf("seq %d: delivered %d, want 1", seq, delivered)
		}
	}

	// Drain ordered channel
	count := 0
	for len(s.Ordered) > 0 {
		<-s.Ordered
		count++
	}
	if count != 5 {
		t.Errorf("total delivered: %d, want 5", count)
	}
}

// TestStreamReorder verifies that out-of-order frames are buffered and delivered
// sequentially once gaps are filled.
func TestStreamReorder(t *testing.T) {
	s := NewStream(1, 32)

	// Send frames out of order: 3, 1, 2
	f3 := frame.NewDataFrame(1, 3, []byte("third"))
	f1 := frame.NewDataFrame(1, 1, []byte("first"))
	f2 := frame.NewDataFrame(1, 2, []byte("second"))

	delivered, err := s.Accept(f3)
	if err != nil {
		t.Fatalf("seq 3: Accept() error: %v", err)
	}
	if delivered != 0 {
		t.Errorf("seq 3 delivered %d before gap filled", delivered)
	}

	delivered, err = s.Accept(f1)
	if err != nil {
		t.Fatalf("seq 1: Accept() error: %v", err)
	}
	// Should deliver seq 1 only (gap at 2)
	if delivered != 1 {
		t.Errorf("seq 1 delivered %d, want 1", delivered)
	}

	delivered, err = s.Accept(f2)
	if err != nil {
		t.Fatalf("seq 2: Accept() error: %v", err)
	}
	// Should deliver seq 2 AND seq 3 (gap filled, drain reorder buf)
	if delivered != 2 {
		t.Errorf("seq 2 delivered %d, want 2 (gap fill)", delivered)
	}

	// Verify ordered delivery
	expected := []string{"first", "second", "third"}
	for i, exp := range expected {
		select {
		case f := <-s.Ordered:
			if string(f.Payload) != exp {
				t.Errorf("frame %d: payload %q, want %q", i, f.Payload, exp)
			}
		default:
			t.Fatalf("frame %d: no frame available", i)
		}
	}
}

// TestStreamDeduplication verifies that duplicate frames are silently dropped.
func TestStreamDeduplication(t *testing.T) {
	s := NewStream(1, 32)

	f := frame.NewDataFrame(1, 1, []byte("data"))

	// First accept should succeed
	delivered, err := s.Accept(f)
	if err != nil {
		t.Fatalf("first Accept() error: %v", err)
	}
	if delivered != 1 {
		t.Errorf("first delivered %d, want 1", delivered)
	}

	// Second accept of same seq should be dedup'd
	delivered, err = s.Accept(f)
	if err != ErrDuplicateSeq {
		t.Errorf("duplicate Accept() error: got %v, want ErrDuplicateSeq", err)
	}
	if delivered != 0 {
		t.Errorf("duplicate delivered %d, want 0", delivered)
	}
}

// TestStreamBufferFull verifies that exceeding the reorder buffer cap returns an error.
func TestStreamBufferFull(t *testing.T) {
	cap := 4
	s := NewStream(1, cap)

	// Fill the buffer with out-of-order frames (skip seq 1)
	for i := 2; i <= cap+1; i++ {
		f := frame.NewDataFrame(1, uint64(i), []byte("data"))
		_, err := s.Accept(f)
		if err != nil {
			t.Fatalf("seq %d: unexpected error: %v", i, err)
		}
	}

	// Next out-of-order frame should fail
	f := frame.NewDataFrame(1, uint64(cap+2), []byte("overflow"))
	_, err := s.Accept(f)
	if err != ErrBufferFull {
		t.Errorf("expected ErrBufferFull, got %v", err)
	}
}

// TestStreamFIN verifies that FIN frames transition the stream state correctly.
func TestStreamFIN(t *testing.T) {
	s := NewStream(1, 32)

	// Deliver in order, then FIN
	f1 := frame.NewDataFrame(1, 1, []byte("data"))
	s.Accept(f1)

	fin := frame.NewFinFrame(1, 2)
	delivered, err := s.Accept(fin)
	if err != nil {
		t.Fatalf("FIN Accept() error: %v", err)
	}
	if delivered != 1 {
		t.Errorf("FIN delivered %d, want 1", delivered)
	}
	if s.State != StreamStateHalfClosed {
		t.Errorf("state after FIN: %s, want HALF_CLOSED", s.State)
	}
}

// TestStreamRST verifies that RST frames abort the stream.
func TestStreamRST(t *testing.T) {
	s := NewStream(1, 32)

	rst := frame.NewRstFrame(1, 1, 0x01)
	delivered, err := s.Accept(rst)
	if err != nil {
		t.Fatalf("RST Accept() error: %v", err)
	}
	if delivered != 1 {
		t.Errorf("RST delivered %d, want 1", delivered)
	}
	if s.State != StreamStateReset {
		t.Errorf("state after RST: %s, want RESET", s.State)
	}

	// Further frames should be rejected
	f := frame.NewDataFrame(1, 2, []byte("post-reset"))
	_, err = s.Accept(f)
	if err != ErrStreamClosed {
		t.Errorf("post-RST Accept() error: got %v, want ErrStreamClosed", err)
	}
}

// TestStreamReorderBufferLen verifies the ReorderBufferLen accessor.
func TestStreamReorderBufferLen(t *testing.T) {
	s := NewStream(1, 32)

	if s.ReorderBufferLen() != 0 {
		t.Errorf("initial buffer len: %d, want 0", s.ReorderBufferLen())
	}

	// Add out-of-order frame
	s.Accept(frame.NewDataFrame(1, 3, []byte("ooo")))
	if s.ReorderBufferLen() != 1 {
		t.Errorf("after ooo: buffer len %d, want 1", s.ReorderBufferLen())
	}
}

// TestSessionDispatch verifies that the session correctly routes frames to streams.
func TestSessionDispatch(t *testing.T) {
	sess := NewSession(32)

	// OPEN creates a new stream
	open := frame.NewOpenFrame(1, 1, "example.com:443")
	stream, delivered, err := sess.Dispatch(open)
	if err != nil {
		t.Fatalf("Dispatch OPEN error: %v", err)
	}
	if stream == nil {
		t.Fatal("Dispatch OPEN returned nil stream")
	}
	if delivered != 1 {
		t.Errorf("OPEN delivered %d, want 1", delivered)
	}

	// DATA to existing stream
	data := frame.NewDataFrame(1, 2, []byte("hello"))
	stream2, delivered, err := sess.Dispatch(data)
	if err != nil {
		t.Fatalf("Dispatch DATA error: %v", err)
	}
	if stream2 != stream {
		t.Error("DATA dispatched to wrong stream")
	}
	if delivered != 1 {
		t.Errorf("DATA delivered %d, want 1", delivered)
	}

	// Active streams count
	if sess.ActiveStreams() != 1 {
		t.Errorf("active streams: %d, want 1", sess.ActiveStreams())
	}
}

// TestSessionMultipleStreams verifies independent stream management.
func TestSessionMultipleStreams(t *testing.T) {
	sess := NewSession(32)

	// Open two streams
	sess.Dispatch(frame.NewOpenFrame(1, 1, "a.com:443"))
	sess.Dispatch(frame.NewOpenFrame(3, 1, "b.com:443"))

	if sess.ActiveStreams() != 2 {
		t.Errorf("active streams: %d, want 2", sess.ActiveStreams())
	}

	// Data to stream 1
	sess.Dispatch(frame.NewDataFrame(1, 2, []byte("stream-1-data")))

	// Data to stream 3
	sess.Dispatch(frame.NewDataFrame(3, 2, []byte("stream-3-data")))

	// Verify independence
	s1 := sess.GetStream(1)
	s3 := sess.GetStream(3)

	if s1 == nil || s3 == nil {
		t.Fatal("expected both streams to exist")
	}

	// Each should have 2 frames (OPEN + DATA)
	if s1.PendingDelivery() != 2 {
		t.Errorf("stream 1 pending: %d, want 2", s1.PendingDelivery())
	}
	if s3.PendingDelivery() != 2 {
		t.Errorf("stream 3 pending: %d, want 2", s3.PendingDelivery())
	}
}

// TestSessionDedup verifies session-level dedup counting.
func TestSessionDedup(t *testing.T) {
	sess := NewSession(32)

	sess.Dispatch(frame.NewOpenFrame(1, 1, "a.com:443"))

	// Send same DATA frame twice
	f := frame.NewDataFrame(1, 2, []byte("data"))
	sess.Dispatch(f)
	sess.Dispatch(f) // duplicate

	stats := sess.Stats()
	if stats.TotalDedups != 1 {
		t.Errorf("total dedups: %d, want 1", stats.TotalDedups)
	}
}

// TestSessionDispatchToNonExistent verifies error on dispatch to unknown stream.
func TestSessionDispatchToNonExistent(t *testing.T) {
	sess := NewSession(32)

	f := frame.NewDataFrame(99, 1, []byte("orphan"))
	_, _, err := sess.Dispatch(f)
	if err == nil {
		t.Fatal("expected error dispatching to non-existent stream")
	}
}

// TestSessionCloseStream verifies stream removal.
func TestSessionCloseStream(t *testing.T) {
	sess := NewSession(32)

	sess.Dispatch(frame.NewOpenFrame(1, 1, "a.com:443"))
	if sess.ActiveStreams() != 1 {
		t.Fatalf("active streams: %d, want 1", sess.ActiveStreams())
	}

	sess.CloseStream(1)
	if sess.ActiveStreams() != 0 {
		t.Errorf("active streams after close: %d, want 0", sess.ActiveStreams())
	}
}

// TestSessionClose verifies full session teardown.
func TestSessionClose(t *testing.T) {
	sess := NewSession(32)

	sess.Dispatch(frame.NewOpenFrame(1, 1, "a.com:443"))
	sess.Dispatch(frame.NewOpenFrame(3, 1, "b.com:443"))

	sess.Close()

	if sess.ActiveStreams() != 0 {
		t.Errorf("active streams after session close: %d, want 0", sess.ActiveStreams())
	}

	// Further dispatches should fail
	_, _, err := sess.Dispatch(frame.NewOpenFrame(5, 1, "c.com:443"))
	if err == nil {
		t.Error("expected error dispatching to closed session")
	}
}

// TestSessionOpenStream verifies manual stream ID allocation.
func TestSessionOpenStream(t *testing.T) {
	sess := NewSession(32)

	// Auto-allocate
	s1, id1, err := sess.OpenStream(0)
	if err != nil {
		t.Fatalf("OpenStream(0) error: %v", err)
	}
	if id1 != 1 {
		t.Errorf("first auto ID: %d, want 1", id1)
	}
	if s1 == nil {
		t.Fatal("OpenStream returned nil stream")
	}

	// Auto-allocate again (should skip by 2)
	_, id2, err := sess.OpenStream(0)
	if err != nil {
		t.Fatalf("OpenStream(0) error: %v", err)
	}
	if id2 != 3 {
		t.Errorf("second auto ID: %d, want 3", id2)
	}

	// Explicit ID
	_, id3, err := sess.OpenStream(100)
	if err != nil {
		t.Fatalf("OpenStream(100) error: %v", err)
	}
	if id3 != 100 {
		t.Errorf("explicit ID: %d, want 100", id3)
	}

	// Duplicate should fail
	_, _, err = sess.OpenStream(100)
	if err != ErrStreamAlreadyExists {
		t.Errorf("duplicate OpenStream: got %v, want ErrStreamAlreadyExists", err)
	}
}

// TestSessionConcurrency verifies thread safety of session operations.
func TestSessionConcurrency(t *testing.T) {
	sess := NewSession(256)

	// Open a stream
	sess.Dispatch(frame.NewOpenFrame(1, 1, "concurrent.test:443"))

	var wg sync.WaitGroup
	errCh := make(chan error, 200)

	// Concurrent writers
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(seq uint64) {
			defer wg.Done()
			f := frame.NewDataFrame(1, seq, []byte("concurrent"))
			_, _, err := sess.Dispatch(f)
			if err != nil && err != ErrDuplicateSeq {
				errCh <- err
			}
		}(uint64(i + 2)) // start from seq 2 (1 was OPEN)
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		t.Errorf("concurrent dispatch error: %v", err)
	}
}

// TestSessionPING verifies PING frames are handled at session level.
func TestSessionPING(t *testing.T) {
	sess := NewSession(32)

	ping := frame.NewPingFrame(0xBEEF, 12345)
	stream, delivered, err := sess.Dispatch(ping)
	if err != nil {
		t.Fatalf("Dispatch PING error: %v", err)
	}
	if stream != nil {
		t.Error("PING should not return a stream")
	}
	if delivered != 0 {
		t.Errorf("PING delivered %d, want 0", delivered)
	}
}

// TestHeavyReorder verifies correct reassembly under extreme reorder conditions.
func TestHeavyReorder(t *testing.T) {
	s := NewStream(1, 256)

	totalFrames := 100
	// Send in reverse order
	for seq := totalFrames; seq >= 1; seq-- {
		f := frame.NewDataFrame(1, uint64(seq), []byte("data"))
		s.Accept(f)
	}

	// All should be delivered in order
	delivered := 0
	for len(s.Ordered) > 0 {
		f := <-s.Ordered
		delivered++
		expectedSeq := uint64(delivered)
		if f.Seq != expectedSeq {
			t.Errorf("frame %d: seq %d, want %d", delivered, f.Seq, expectedSeq)
		}
	}
	if delivered != totalFrames {
		t.Errorf("total delivered: %d, want %d", delivered, totalFrames)
	}
}
