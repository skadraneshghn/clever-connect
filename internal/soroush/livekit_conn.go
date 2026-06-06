// Package soroush implements the Soroush SFU RTP-based QUIC tunnel engine.
package soroush

import (
	"net"
	"time"

	"github.com/pion/webrtc/v4"
	"github.com/pion/webrtc/v4/pkg/media"
)

// fakeAddr is the synthetic UDP address used to satisfy QUIC's net.PacketConn interface.
// QUIC expects a UDP-like socket, but we're actually routing through LiveKit RTP tracks.
var fakeAddr = &net.UDPAddr{IP: net.ParseIP("0.0.0.0"), Port: 1}

// ──────────────────────────────────────────────────────────────────────────────
// RtpPacketConn: Bridges QUIC UDP datagrams to WebRTC Audio Samples
//
// This struct implements net.PacketConn, tricking QUIC into thinking it's
// talking to a UDP socket when it's actually writing Opus audio frames
// through a LiveKit SFU Audio Track.
//
// Architecture:
//   WriteTo() → QUIC datagram → prepend 0x51 tag → WriteSample() → LiveKit SFU
//   ReadFrom() ← rxQueue ← PushRx() ← remote TrackRemote.ReadRTP() callback
//
// The 0x51 ('Q') prefix byte tags QUIC frames, filtering out any real audio
// that might be present on the track (e.g., silence generators).
// ──────────────────────────────────────────────────────────────────────────────

// RtpPacketConn bridges QUIC UDP payloads to WebRTC Audio Samples
type RtpPacketConn struct {
	localTrack *webrtc.TrackLocalStaticSample
	rxQueue    chan []byte
	closed     bool
}

// NewRtpPacketConn creates a new RTP-based packet connection backed by a
// WebRTC audio track for transmission.
func NewRtpPacketConn(track *webrtc.TrackLocalStaticSample) *RtpPacketConn {
	return &RtpPacketConn{
		localTrack: track,
		rxQueue:    make(chan []byte, 2048), // Deep buffer for incoming RTP
	}
}

// PushRx receives raw RTP payloads from the WebRTC subscription callback.
// It filters for QUIC-tagged frames (0x51 prefix) and drops the rest.
func (c *RtpPacketConn) PushRx(payload []byte) {
	if len(payload) == 0 {
		return
	}
	// We use 'Q' (0x51) to tag QUIC frames, filtering out actual voice audio
	if payload[0] == 0x51 {
		select {
		case c.rxQueue <- payload[1:]:
		default:
			// Buffer full — drop it. QUIC's internal ARQ will resend it automatically.
		}
	}
}

// ReadFrom feeds QUIC the datagrams we received from WebRTC.
// Blocks until a packet arrives or the connection is closed.
func (c *RtpPacketConn) ReadFrom(p []byte) (n int, addr net.Addr, err error) {
	data, ok := <-c.rxQueue
	if !ok {
		return 0, nil, net.ErrClosed
	}
	n = copy(p, data)
	return n, fakeAddr, nil
}

// WriteTo takes QUIC datagrams and writes them to the LiveKit Audio Track.
// The 0x51 prefix byte is prepended so the receiver can distinguish QUIC
// frames from real audio content.
func (c *RtpPacketConn) WriteTo(p []byte, addr net.Addr) (n int, err error) {
	if c.closed {
		return 0, net.ErrClosed
	}

	// Prepend the 'Q' tag (1 byte)
	payload := make([]byte, 1+len(p))
	payload[0] = 0x51
	copy(payload[1:], p)

	// Write as an audio sample. Duration prevents LiveKit from throttling.
	err = c.localTrack.WriteSample(media.Sample{
		Data:     payload,
		Duration: time.Millisecond * 20,
	})

	return len(p), err
}

// Close shuts down the packet connection and closes the receive queue.
func (c *RtpPacketConn) Close() error {
	c.closed = true
	close(c.rxQueue)
	return nil
}

func (c *RtpPacketConn) LocalAddr() net.Addr                { return fakeAddr }
func (c *RtpPacketConn) SetDeadline(t time.Time) error      { return nil }
func (c *RtpPacketConn) SetReadDeadline(t time.Time) error  { return nil }
func (c *RtpPacketConn) SetWriteDeadline(t time.Time) error { return nil }

// Compile-time check that RtpPacketConn implements net.PacketConn
var _ net.PacketConn = (*RtpPacketConn)(nil)
