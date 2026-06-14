package session

import (
	"sync"
	"testing"
	"time"

	"clever-connect/internal/bonding/frame"
)

// TestSessionStress tests the session under high concurrent load.
func TestSessionStress(t *testing.T) {
	sess := NewSession(256)
	defer sess.Close()

	const numStreams = 100
	const framesPerStream = 50

	var wg sync.WaitGroup

	// Dispatch frames from multiple goroutines simultaneously
	for s := uint32(0); s < numStreams; s++ {
		wg.Add(1)
		go func(streamID uint32) {
			defer wg.Done()

			// Send OPEN
			open := frame.NewOpenFrame(streamID, 0, "target:443")
			sess.Dispatch(open)

			// Send DATA frames
			for seq := uint64(1); seq <= framesPerStream; seq++ {
				data := frame.NewDataFrame(streamID, seq, []byte("stress test payload"))
				sess.Dispatch(data)
			}

			// Send FIN
			fin := frame.NewFinFrame(streamID, framesPerStream+1)
			sess.Dispatch(fin)
		}(s)
	}

	wg.Wait()

	// Verify all streams were created
	stats := sess.Stats()
	if stats.TotalFramesIn == 0 {
		t.Error("expected non-zero total frames in")
	}
}

// TestSessionOutOfOrderWithDrain tests reorder with concurrent draining.
func TestSessionOutOfOrderWithDrain(t *testing.T) {
	sess := NewSession(64)
	defer sess.Close()

	streamID := uint32(42)

	// Open the stream first and drain the OPEN immediately
	open := frame.NewOpenFrame(streamID, 1, "target:443")
	sess.Dispatch(open)

	stream := sess.GetStream(streamID)
	if stream == nil {
		t.Fatal("stream not found after OPEN")
	}

	// Drain OPEN
	select {
	case <-stream.Ordered:
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for OPEN frame")
	}

	// Send frames in reverse in a goroutine so we can drain concurrently
	const n = 30
	go func() {
		for i := uint64(n + 1); i >= 2; i-- {
			data := frame.NewDataFrame(streamID, i, []byte("reorder"))
			sess.Dispatch(data)
		}
	}()

	// Drain all ordered frames
	var lastSeq uint64
	received := 0
	timeout := time.After(5 * time.Second)
	for received < n {
		select {
		case f := <-stream.Ordered:
			if f.Seq < lastSeq {
				t.Errorf("out-of-order: got seq %d after %d", f.Seq, lastSeq)
			}
			lastSeq = f.Seq
			received++
		case <-timeout:
			t.Fatalf("timeout: received only %d/%d frames", received, n)
		}
	}
}

// TestSessionDuplicateStorm tests dedup under heavy duplicate load.
func TestSessionDuplicateStorm(t *testing.T) {
	sess := NewSession(32)
	defer sess.Close()

	streamID := uint32(7)
	open := frame.NewOpenFrame(streamID, 1, "target:443")
	sess.Dispatch(open)

	stream := sess.GetStream(streamID)
	if stream == nil {
		t.Fatal("stream not found")
	}

	// Drain OPEN
	select {
	case <-stream.Ordered:
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for OPEN")
	}

	// Send the same frame 100 times (seq=2 so it's after OPEN which was seq=1)
	data := frame.NewDataFrame(streamID, 2, []byte("duplicate data"))
	for i := 0; i < 100; i++ {
		sess.Dispatch(data)
	}

	// Should get exactly 1 DATA (99 duplicates dropped)
	select {
	case <-stream.Ordered:
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for DATA")
	}

	// Channel should be empty now — give a small window
	time.Sleep(10 * time.Millisecond)
	select {
	case f := <-stream.Ordered:
		t.Errorf("unexpected extra frame: %s seq=%d", frame.TypeName(f.Type), f.Seq)
	default:
		// Good — no duplicates leaked through
	}

	stats := sess.Stats()
	if stats.TotalDedups == 0 {
		t.Error("expected dedup count > 0")
	}
}

// TestSessionCloseWhileDispatching tests graceful shutdown under load.
func TestSessionCloseWhileDispatching(t *testing.T) {
	sess := NewSession(32)

	done := make(chan struct{})

	// Start dispatching in background
	go func() {
		defer close(done)
		for i := uint64(0); i < 1000; i++ {
			data := frame.NewDataFrame(1, i, []byte("close test"))
			sess.Dispatch(data)
		}
	}()

	// Give dispatcher a moment to start
	time.Sleep(time.Millisecond)

	// Close mid-dispatch — should not panic
	sess.Close()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("dispatcher goroutine didn't finish after Close")
	}
}
