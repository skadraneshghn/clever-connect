package frame

import (
	"bytes"
	"encoding/binary"
	"testing"
	"time"
)

// TestEncodeDecodeRoundTrip verifies that encoding then decoding a frame
// produces byte-identical results for all frame types.
func TestEncodeDecodeRoundTrip(t *testing.T) {
	tests := []struct {
		name  string
		frame Frame
	}{
		{
			name: "DATA frame with payload",
			frame: Frame{
				Version:  Version,
				Type:     TypeDATA,
				StreamID: 42,
				Seq:      100,
				Payload:  []byte("Hello, Multipath Bonding!"),
			},
		},
		{
			name: "OPEN frame with target address",
			frame: Frame{
				Version:  Version,
				Type:     TypeOPEN,
				StreamID: 1,
				Seq:      1,
				Payload:  []byte("youtube.com:443"),
			},
		},
		{
			name: "FIN frame (no payload)",
			frame: Frame{
				Version:  Version,
				Type:     TypeFIN,
				StreamID: 7,
				Seq:      999,
				Payload:  nil,
			},
		},
		{
			name: "RST frame with reason byte",
			frame: Frame{
				Version:  Version,
				Type:     TypeRST,
				StreamID: 3,
				Seq:      50,
				Payload:  []byte{0x01},
			},
		},
		{
			name: "PING frame",
			frame: Frame{
				Version:  Version,
				Type:     TypePING,
				StreamID: 0,
				Seq:      0,
				Payload:  make([]byte, 12),
			},
		},
		{
			name: "WINDOW frame",
			frame: Frame{
				Version:  Version,
				Type:     TypeWINDOW,
				StreamID: 5,
				Seq:      0,
				Payload:  make([]byte, 4),
			},
		},
		{
			name: "DATA frame with max-aligned payload (4096 bytes)",
			frame: Frame{
				Version:  Version,
				Type:     TypeDATA,
				StreamID: 100,
				Seq:      12345,
				Payload:  make([]byte, 4096),
			},
		},
		{
			name: "DATA frame with zero-length payload",
			frame: Frame{
				Version:  Version,
				Type:     TypeDATA,
				StreamID: 0,
				Seq:      0,
				Payload:  []byte{},
			},
		},
		{
			name: "StreamID with max uint32 value",
			frame: Frame{
				Version:  Version,
				Type:     TypeDATA,
				StreamID: 0xFFFFFFFF,
				Seq:      0xFFFFFFFFFFFFFFFF,
				Payload:  []byte{0xDE, 0xAD},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			encoded, err := tc.frame.Encode()
			if err != nil {
				t.Fatalf("Encode() error: %v", err)
			}

			decoded, err := Decode(encoded)
			if err != nil {
				t.Fatalf("Decode() error: %v", err)
			}

			if decoded.Version != tc.frame.Version {
				t.Errorf("Version: got %d, want %d", decoded.Version, tc.frame.Version)
			}
			if decoded.Type != tc.frame.Type {
				t.Errorf("Type: got %d, want %d", decoded.Type, tc.frame.Type)
			}
			if decoded.StreamID != tc.frame.StreamID {
				t.Errorf("StreamID: got %d, want %d", decoded.StreamID, tc.frame.StreamID)
			}
			if decoded.Seq != tc.frame.Seq {
				t.Errorf("Seq: got %d, want %d", decoded.Seq, tc.frame.Seq)
			}
			if !bytes.Equal(decoded.Payload, tc.frame.Payload) {
				t.Errorf("Payload mismatch: got %d bytes, want %d bytes", len(decoded.Payload), len(tc.frame.Payload))
			}
		})
	}
}

// TestReadWriteFrameStreaming verifies streaming read/write through an io.ReadWriter.
func TestReadWriteFrameStreaming(t *testing.T) {
	original := NewDataFrame(42, 100, []byte("streaming test payload"))

	var buf bytes.Buffer

	// Write
	if err := WriteFrame(&buf, original); err != nil {
		t.Fatalf("WriteFrame() error: %v", err)
	}

	// Read
	decoded, err := ReadFrame(&buf)
	if err != nil {
		t.Fatalf("ReadFrame() error: %v", err)
	}

	if decoded.StreamID != original.StreamID {
		t.Errorf("StreamID: got %d, want %d", decoded.StreamID, original.StreamID)
	}
	if decoded.Seq != original.Seq {
		t.Errorf("Seq: got %d, want %d", decoded.Seq, original.Seq)
	}
	if !bytes.Equal(decoded.Payload, original.Payload) {
		t.Errorf("Payload mismatch")
	}
}

// TestMultipleFramesStreaming verifies reading multiple consecutive frames from a stream.
func TestMultipleFramesStreaming(t *testing.T) {
	frames := []*Frame{
		NewOpenFrame(1, 1, "google.com:443"),
		NewDataFrame(1, 2, []byte("GET / HTTP/1.1\r\n")),
		NewDataFrame(1, 3, []byte("Host: google.com\r\n\r\n")),
		NewFinFrame(1, 4),
	}

	var buf bytes.Buffer
	for _, f := range frames {
		if err := WriteFrame(&buf, f); err != nil {
			t.Fatalf("WriteFrame() error: %v", err)
		}
	}

	for i, expected := range frames {
		decoded, err := ReadFrame(&buf)
		if err != nil {
			t.Fatalf("ReadFrame() frame %d error: %v", i, err)
		}
		if decoded.Type != expected.Type {
			t.Errorf("frame %d Type: got %d, want %d", i, decoded.Type, expected.Type)
		}
		if decoded.StreamID != expected.StreamID {
			t.Errorf("frame %d StreamID: got %d, want %d", i, decoded.StreamID, expected.StreamID)
		}
		if decoded.Seq != expected.Seq {
			t.Errorf("frame %d Seq: got %d, want %d", i, decoded.Seq, expected.Seq)
		}
		if !bytes.Equal(decoded.Payload, expected.Payload) {
			t.Errorf("frame %d Payload mismatch", i)
		}
	}
}

// TestDecodeErrors verifies that malformed input produces appropriate errors.
func TestDecodeErrors(t *testing.T) {
	tests := []struct {
		name      string
		data      []byte
		wantError error
	}{
		{
			name:      "empty data",
			data:      []byte{},
			wantError: ErrFrameTooShort,
		},
		{
			name:      "too short for header",
			data:      make([]byte, HeaderSize-1),
			wantError: ErrFrameTooShort,
		},
		{
			name: "wrong version",
			data: func() []byte {
				d := make([]byte, HeaderSize)
				d[0] = 99 // wrong version
				return d
			}(),
			wantError: ErrVersionMismatch,
		},
		{
			name: "invalid frame type",
			data: func() []byte {
				d := make([]byte, HeaderSize)
				d[0] = Version
				d[1] = 200 // invalid type
				return d
			}(),
			wantError: ErrInvalidType,
		},
		{
			name: "truncated payload",
			data: func() []byte {
				d := make([]byte, HeaderSize)
				d[0] = Version
				d[1] = TypeDATA
				binary.BigEndian.PutUint16(d[14:16], 100) // claims 100 bytes payload
				return d // but no payload data
			}(),
			wantError: ErrPayloadTruncated,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Decode(tc.data)
			if err == nil {
				t.Fatalf("expected error, got nil")
			}
			// Check that the error wraps or matches the expected error
			if tc.wantError != nil && !containsError(err, tc.wantError) {
				t.Errorf("expected error containing %q, got %q", tc.wantError, err)
			}
		})
	}
}

func containsError(err, target error) bool {
	return err.Error() == target.Error() || bytes.Contains([]byte(err.Error()), []byte(target.Error()))
}

// TestPayloadTooLarge verifies that encoding a frame with an oversized payload fails.
func TestPayloadTooLarge(t *testing.T) {
	f := &Frame{
		Version:  Version,
		Type:     TypeDATA,
		StreamID: 1,
		Seq:      1,
		Payload:  make([]byte, MaxPayloadSize+1),
	}
	_, err := f.Encode()
	if err != ErrPayloadTooLarge {
		t.Errorf("expected ErrPayloadTooLarge, got %v", err)
	}
}

// TestMaxPayloadSize verifies that a frame with exactly MaxPayloadSize bytes works.
func TestMaxPayloadSize(t *testing.T) {
	payload := make([]byte, MaxPayloadSize)
	for i := range payload {
		payload[i] = byte(i % 256)
	}

	f := NewDataFrame(1, 1, payload)
	encoded, err := f.Encode()
	if err != nil {
		t.Fatalf("Encode() error: %v", err)
	}

	decoded, err := Decode(encoded)
	if err != nil {
		t.Fatalf("Decode() error: %v", err)
	}

	if !bytes.Equal(decoded.Payload, payload) {
		t.Error("max-size payload mismatch after round-trip")
	}
}

// TestHelperConstructors verifies all frame constructor functions.
func TestHelperConstructors(t *testing.T) {
	t.Run("NewOpenFrame", func(t *testing.T) {
		f := NewOpenFrame(10, 1, "example.com:443")
		if f.Type != TypeOPEN {
			t.Errorf("Type: got %d, want %d", f.Type, TypeOPEN)
		}
		if string(f.Payload) != "example.com:443" {
			t.Errorf("Payload: got %q, want %q", f.Payload, "example.com:443")
		}
	})

	t.Run("NewFinFrame", func(t *testing.T) {
		f := NewFinFrame(5, 99)
		if f.Type != TypeFIN {
			t.Errorf("Type: got %d, want %d", f.Type, TypeFIN)
		}
		if len(f.Payload) != 0 {
			t.Errorf("FIN should have no payload, got %d bytes", len(f.Payload))
		}
	})

	t.Run("NewRstFrame", func(t *testing.T) {
		f := NewRstFrame(3, 50, 0x02)
		if f.Type != TypeRST {
			t.Errorf("Type: got %d, want %d", f.Type, TypeRST)
		}
		if len(f.Payload) != 1 || f.Payload[0] != 0x02 {
			t.Errorf("RST payload: got %v, want [0x02]", f.Payload)
		}
	})

	t.Run("NewPingFrame", func(t *testing.T) {
		now := uint64(time.Now().UnixNano())
		f := NewPingFrame(0xDEAD, now)
		if f.Type != TypePING {
			t.Errorf("Type: got %d, want %d", f.Type, TypePING)
		}
		if len(f.Payload) != 12 {
			t.Fatalf("PING payload length: got %d, want 12", len(f.Payload))
		}
		nonce := binary.BigEndian.Uint32(f.Payload[0:4])
		ts := binary.BigEndian.Uint64(f.Payload[4:12])
		if nonce != 0xDEAD {
			t.Errorf("PING nonce: got %x, want 0xDEAD", nonce)
		}
		if ts != now {
			t.Errorf("PING timestamp mismatch")
		}
	})

	t.Run("NewWindowFrame", func(t *testing.T) {
		f := NewWindowFrame(7, 65536)
		if f.Type != TypeWINDOW {
			t.Errorf("Type: got %d, want %d", f.Type, TypeWINDOW)
		}
		if len(f.Payload) != 4 {
			t.Fatalf("WINDOW payload length: got %d, want 4", len(f.Payload))
		}
		window := binary.BigEndian.Uint32(f.Payload)
		if window != 65536 {
			t.Errorf("WINDOW size: got %d, want 65536", window)
		}
	})
}

// TestFrameString verifies the debug string representation.
func TestFrameString(t *testing.T) {
	f := NewDataFrame(42, 100, []byte("test"))
	s := f.String()
	if s == "" {
		t.Error("String() returned empty")
	}
	// Should contain key info
	if !bytes.Contains([]byte(s), []byte("DATA")) {
		t.Errorf("String() should contain 'DATA', got: %s", s)
	}
}

// BenchmarkEncode benchmarks frame encoding performance.
func BenchmarkEncode(b *testing.B) {
	f := NewDataFrame(1, 1, make([]byte, 4096))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = f.Encode()
	}
}

// BenchmarkDecode benchmarks frame decoding performance.
func BenchmarkDecode(b *testing.B) {
	f := NewDataFrame(1, 1, make([]byte, 4096))
	data, _ := f.Encode()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = Decode(data)
	}
}

// BenchmarkReadFrame benchmarks streaming frame reads.
func BenchmarkReadFrame(b *testing.B) {
	f := NewDataFrame(1, 1, make([]byte, 4096))
	data, _ := f.Encode()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		reader := bytes.NewReader(data)
		_, _ = ReadFrame(reader)
	}
}
