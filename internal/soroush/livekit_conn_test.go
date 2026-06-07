package soroush

import (
	"bytes"
	"net"
	"testing"

	"github.com/pion/webrtc/v4"
)

func TestRtpPacketConnDemux(t *testing.T) {
	// Create local track with dummy codec info to satisfy initialization
	localTrack, err := webrtc.NewTrackLocalStaticSample(webrtc.RTPCodecCapability{
		MimeType:  webrtc.MimeTypeOpus,
		ClockRate: 48000,
		Channels:  2,
	}, "tunnel-quic", "tunnel")
	if err != nil {
		t.Fatalf("Failed to create local track: %v", err)
	}

	conn := NewRtpPacketConn(localTrack)
	defer conn.Close()

	// Push packet from Client A with 'Q' tag (0x51)
	payloadA := []byte{0x51, 0x01, 0x02, 0x03}
	conn.PushRx(payloadA, "client-A")

	// Push packet from Client B with 'Q' tag (0x51)
	payloadB := []byte{0x51, 0x0a, 0x0b, 0x0c}
	conn.PushRx(payloadB, "client-B")

	// Push invalid packet (no 'Q' tag) - should be ignored
	payloadNoise := []byte{0x00, 0xff, 0xee}
	conn.PushRx(payloadNoise, "client-noise")

	// Read first packet - should be from client-A
	buf := make([]byte, 100)
	n, addr, err := conn.ReadFrom(buf)
	if err != nil {
		t.Fatalf("ReadFrom error: %v", err)
	}
	if addr.String() != "client-A" {
		t.Errorf("Expected sender 'client-A', got '%s'", addr.String())
	}
	if n != 3 || !bytes.Equal(buf[:n], []byte{0x01, 0x02, 0x03}) {
		t.Errorf("Expected clean data [1, 2, 3], got %v", buf[:n])
	}

	// Read second packet - should be from client-B
	n, addr, err = conn.ReadFrom(buf)
	if err != nil {
		t.Fatalf("ReadFrom error: %v", err)
	}
	if addr.String() != "client-B" {
		t.Errorf("Expected sender 'client-B', got '%s'", addr.String())
	}
	if n != 3 || !bytes.Equal(buf[:n], []byte{0x0a, 0x0b, 0x0c}) {
		t.Errorf("Expected clean data [10, 11, 12], got %v", buf[:n])
	}

	// Ensure no more packets are in the queue (non-blocking select)
	select {
	case p := <-conn.rxQueue:
		t.Fatalf("Unexpected packet in queue: %v from %s", p.data, p.addr.String())
	default:
		// Queue is correctly empty
	}
}

func TestLiveKitAddr(t *testing.T) {
	addr := &LiveKitAddr{Identity: "test-user"}
	if addr.Network() != "livekit" {
		t.Errorf("Expected network 'livekit', got '%s'", addr.Network())
	}
	if addr.String() != "test-user" {
		t.Errorf("Expected address string 'test-user', got '%s'", addr.String())
	}

	var _ net.Addr = addr
}

func TestCloseSafety(t *testing.T) {
	localTrack, err := webrtc.NewTrackLocalStaticSample(webrtc.RTPCodecCapability{
		MimeType:  webrtc.MimeTypeOpus,
		ClockRate: 48000,
		Channels:  2,
	}, "tunnel-quic", "tunnel")
	if err != nil {
		t.Fatalf("Failed to create local track: %v", err)
	}

	conn := NewRtpPacketConn(localTrack)

	// Close first
	if err := conn.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	// Double close safety check
	if err := conn.Close(); err != nil {
		t.Fatalf("Second Close failed: %v", err)
	}

	// Recover helper to capture panic if it happens
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("PushRx panicked on closed connection: %v", r)
		}
	}()

	// Push after close should not panic, it should return silently
	payload := []byte{0x51, 0x01, 0x02, 0x03}
	conn.PushRx(payload, "client-A")
}

