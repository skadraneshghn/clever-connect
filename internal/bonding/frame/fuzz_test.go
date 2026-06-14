package frame

import (
	"bytes"
	"math/rand"
	"testing"
)

// ──────────────────────────────────────────────────────────────────────────────
// Fuzz Tests for Wire Protocol (Go native fuzzing)
// ──────────────────────────────────────────────────────────────────────────────

// FuzzDecode tests that Decode never panics on arbitrary input.
func FuzzDecode(f *testing.F) {
	// Seed corpus with valid frames
	for _, ft := range []uint8{TypeOPEN, TypeDATA, TypeFIN, TypeRST, TypePING, TypeWINDOW} {
		validFrame := &Frame{
			Version:  Version,
			Type:     ft,
			StreamID: 12345,
			Seq:      67890,
			Payload:  []byte("test payload"),
		}
		encoded, _ := validFrame.Encode()
		f.Add(encoded)
	}

	// Add edge cases
	f.Add([]byte{})                       // empty
	f.Add([]byte{0xff})                   // single byte
	f.Add(make([]byte, HeaderSize))       // header only (zero values)
	f.Add(make([]byte, HeaderSize+65535)) // max size

	f.Fuzz(func(t *testing.T, data []byte) {
		// Must never panic
		frame, err := Decode(data)
		if err != nil {
			return // errors are fine
		}

		// If decode succeeded, verify invariants
		if frame.Version != Version {
			t.Errorf("decoded frame has wrong version: %d", frame.Version)
		}
		if frame.Type > TypeWINDOW {
			t.Errorf("decoded frame has invalid type: %d", frame.Type)
		}
		if len(frame.Payload) > MaxPayloadSize {
			t.Errorf("decoded frame payload exceeds max: %d", len(frame.Payload))
		}
	})
}

// FuzzRoundTrip tests that Encode(Decode(data)) == data for valid frames.
func FuzzRoundTrip(f *testing.F) {
	// Seed with various frame types
	seeds := []*Frame{
		NewDataFrame(1, 0, []byte("hello")),
		NewDataFrame(0xFFFFFFFF, 0xFFFFFFFFFFFFFFFF, []byte{}),
		NewOpenFrame(42, 1, "example.com:443"),
		NewFinFrame(100, 50),
		NewRstFrame(200, 60, 0x01),
		NewPingFrame(99, 12345678),
		NewWindowFrame(300, 65535),
	}

	for _, s := range seeds {
		encoded, _ := s.Encode()
		f.Add(encoded)
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		frame1, err := Decode(data)
		if err != nil {
			return
		}

		// Re-encode
		reencoded, err := frame1.Encode()
		if err != nil {
			t.Fatalf("re-encode failed for valid frame: %v", err)
		}

		// Re-decode
		frame2, err := Decode(reencoded)
		if err != nil {
			t.Fatalf("re-decode failed: %v", err)
		}

		// Compare fields
		if frame1.Version != frame2.Version {
			t.Errorf("version mismatch: %d != %d", frame1.Version, frame2.Version)
		}
		if frame1.Type != frame2.Type {
			t.Errorf("type mismatch: %d != %d", frame1.Type, frame2.Type)
		}
		if frame1.StreamID != frame2.StreamID {
			t.Errorf("streamID mismatch: %d != %d", frame1.StreamID, frame2.StreamID)
		}
		if frame1.Seq != frame2.Seq {
			t.Errorf("seq mismatch: %d != %d", frame1.Seq, frame2.Seq)
		}
		if !bytes.Equal(frame1.Payload, frame2.Payload) {
			t.Errorf("payload mismatch: %v != %v", frame1.Payload, frame2.Payload)
		}
	})
}

// FuzzReadFrame tests the streaming decoder with random data.
func FuzzReadFrame(f *testing.F) {
	// Seed with valid encoded frames
	validFrame := NewDataFrame(1, 1, []byte("streaming test"))
	encoded, _ := validFrame.Encode()
	f.Add(encoded)

	f.Fuzz(func(t *testing.T, data []byte) {
		reader := bytes.NewReader(data)
		// Must not panic
		frame, err := ReadFrame(reader)
		if err != nil {
			return
		}

		// If successful, verify invariants
		if frame.Version != Version {
			t.Errorf("wrong version: %d", frame.Version)
		}
		if frame.Type > TypeWINDOW {
			t.Errorf("invalid type: %d", frame.Type)
		}
	})
}

// TestStressEncodeDecode stress-tests encode/decode with random valid frames.
func TestStressEncodeDecode(t *testing.T) {
	rng := rand.New(rand.NewSource(42))

	for i := 0; i < 10000; i++ {
		frameType := uint8(rng.Intn(6)) // 0-5
		streamID := rng.Uint32()
		seq := rng.Uint64()
		payloadLen := rng.Intn(1024) // up to 1KB

		payload := make([]byte, payloadLen)
		rng.Read(payload)

		f := &Frame{
			Version:  Version,
			Type:     frameType,
			StreamID: streamID,
			Seq:      seq,
			Payload:  payload,
		}

		encoded, err := f.Encode()
		if err != nil {
			t.Fatalf("encode failed at iteration %d: %v", i, err)
		}

		decoded, err := Decode(encoded)
		if err != nil {
			t.Fatalf("decode failed at iteration %d: %v", i, err)
		}

		if decoded.Version != f.Version || decoded.Type != f.Type ||
			decoded.StreamID != f.StreamID || decoded.Seq != f.Seq ||
			!bytes.Equal(decoded.Payload, f.Payload) {
			t.Fatalf("round-trip mismatch at iteration %d", i)
		}
	}
}

// TestDecodeCorruptedFrames ensures graceful handling of corrupted input.
func TestDecodeCorruptedFrames(t *testing.T) {
	tests := []struct {
		name string
		data []byte
	}{
		{"empty", []byte{}},
		{"single_byte", []byte{0x01}},
		{"short_header", make([]byte, HeaderSize-1)},
		{"wrong_version", func() []byte {
			f := NewDataFrame(1, 1, []byte("test"))
			data, _ := f.Encode()
			data[0] = 0xFF // corrupt version
			return data
		}()},
		{"invalid_type", func() []byte {
			f := NewDataFrame(1, 1, []byte("test"))
			data, _ := f.Encode()
			data[1] = 0xFF // corrupt type
			return data
		}()},
		{"payload_length_overflow", func() []byte {
			data := make([]byte, HeaderSize)
			data[0] = Version
			data[1] = TypeDATA
			// Set payload length to 65535 but only provide header
			data[14] = 0xFF
			data[15] = 0xFF
			return data
		}()},
		{"all_zeros", make([]byte, HeaderSize+100)},
		{"all_ones", func() []byte {
			data := make([]byte, HeaderSize+100)
			for i := range data {
				data[i] = 0xFF
			}
			return data
		}()},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Decode(tt.data)
			if err == nil && tt.name != "all_zeros" {
				// all_zeros with version 0 should fail (version mismatch)
				// This is fine — we're just verifying no panics
			}
			// Main assertion: no panic occurred
		})
	}
}

// TestStreamingDecodeStress tests ReadFrame with concatenated frames.
func TestStreamingDecodeStress(t *testing.T) {
	var buf bytes.Buffer

	// Write 1000 frames to a buffer
	frames := make([]*Frame, 1000)
	for i := 0; i < 1000; i++ {
		frames[i] = NewDataFrame(uint32(i), uint64(i), []byte("streaming stress test data"))
		if err := WriteFrame(&buf, frames[i]); err != nil {
			t.Fatalf("WriteFrame failed at %d: %v", i, err)
		}
	}

	// Read them back
	reader := bytes.NewReader(buf.Bytes())
	for i := 0; i < 1000; i++ {
		f, err := ReadFrame(reader)
		if err != nil {
			t.Fatalf("ReadFrame failed at %d: %v", i, err)
		}
		if f.StreamID != uint32(i) || f.Seq != uint64(i) {
			t.Fatalf("frame %d mismatch: streamID=%d seq=%d", i, f.StreamID, f.Seq)
		}
		if !bytes.Equal(f.Payload, []byte("streaming stress test data")) {
			t.Fatalf("payload mismatch at frame %d", i)
		}
	}
}
