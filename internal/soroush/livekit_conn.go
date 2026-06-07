// Package soroush implements the Soroush SFU RTP-based QUIC tunnel engine.
package soroush

import (
	"net"
	"sync"
	"time"

	"github.com/pion/webrtc/v4"
	"github.com/pion/webrtc/v4/pkg/media"
)

// LiveKitAddr implements net.Addr to natively map QUIC sessions to distinct SFU identities.
type LiveKitAddr struct {
	Identity string
}

func (a *LiveKitAddr) Network() string { return "livekit" }
func (a *LiveKitAddr) String() string  { return a.Identity }

type rxPacket struct {
	data []byte
	addr net.Addr
}

// RtpPacketConn bridges QUIC UDP payloads to isolated WebRTC Audio Tracks.
type RtpPacketConn struct {
	localTrack *webrtc.TrackLocalStaticSample
	rxQueue    chan rxPacket
	mu         sync.RWMutex // Protects closed state to prevent runtime channel panics
	closed     bool
}

// NewRtpPacketConn creates a new RTP-based packet connection backed by a
// WebRTC audio track for transmission.
func NewRtpPacketConn(track *webrtc.TrackLocalStaticSample) *RtpPacketConn {
	return &RtpPacketConn{
		localTrack: track,
		rxQueue:    make(chan rxPacket, 4096),
	}
}

// PushRx captures payloads safely checking closed state to prevent runtime channel panics.
func (c *RtpPacketConn) PushRx(payload []byte, senderIdentity string) {
	if len(payload) == 0 {
		return
	}

	if payload[0] == 0x51 {
		cleanData := make([]byte, len(payload)-1)
		copy(cleanData, payload[1:])

		packet := rxPacket{
			data: cleanData,
			addr: &LiveKitAddr{Identity: senderIdentity},
		}

		// Read-lock to verify channel safety
		c.mu.RLock()
		defer c.mu.RUnlock()

		if c.closed {
			return
		}

		select {
		case c.rxQueue <- packet:
		default:
			// Queue full — drop frame safely.
		}
	}
}

// ReadFrom extracts frames and presents the true sender address to the QUIC multiplexer engine.
func (c *RtpPacketConn) ReadFrom(p []byte) (n int, addr net.Addr, err error) {
	packet, ok := <-c.rxQueue
	if !ok {
		return 0, nil, net.ErrClosed
	}
	n = copy(p, packet.data)
	return n, packet.addr, nil
}

// WriteTo transmits outbound frames into the LiveKit audio router track.
func (c *RtpPacketConn) WriteTo(p []byte, addr net.Addr) (n int, err error) {
	c.mu.RLock()
	isClosed := c.closed
	c.mu.RUnlock()

	if isClosed {
		return 0, net.ErrClosed
	}

	payload := make([]byte, 1+len(p))
	payload[0] = 0x51
	copy(payload[1:], p)

	err = c.localTrack.WriteSample(media.Sample{
		Data:     payload,
		Duration: time.Millisecond * 20,
	})

	return len(p), err
}

// Close explicitly locks and tears down the channel cleanly.
func (c *RtpPacketConn) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return nil
	}
	c.closed = true
	close(c.rxQueue)
	return nil
}

func (c *RtpPacketConn) LocalAddr() net.Addr                { return &LiveKitAddr{Identity: "local"} }
func (c *RtpPacketConn) SetDeadline(t time.Time) error      { return nil }
func (c *RtpPacketConn) SetReadDeadline(t time.Time) error  { return nil }
func (c *RtpPacketConn) SetWriteDeadline(t time.Time) error { return nil }

var _ net.PacketConn = (*RtpPacketConn)(nil)
