package soroush

import (
	"io"
	"net"
	"sync"
	"time"

	"github.com/hashicorp/yamux"
	"github.com/pion/webrtc/v4"
)

// WebRTCTransport manages the WebRTC P2P DataChannel connection.
type WebRTCTransport struct {
	pc       *webrtc.PeerConnection
	dc       *webrtc.DataChannel
	rawConn  net.Conn
	yamuxSes *yamux.Session
	mu       sync.Mutex
	closed   bool
}

// NewWebRTCTransport creates a new WebRTCTransport.
func NewWebRTCTransport() *WebRTCTransport {
	return &WebRTCTransport{}
}

// YamuxSession returns the active yamux session.
func (t *WebRTCTransport) YamuxSession() *yamux.Session {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.yamuxSes
}

// Close closes the transport and cleans up resources.
func (t *WebRTCTransport) Close() {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.closed {
		return
	}
	t.closed = true

	if t.yamuxSes != nil {
		t.yamuxSes.Close()
	}
	if t.rawConn != nil {
		t.rawConn.Close()
	}
	if t.pc != nil {
		t.pc.Close()
	}
}

// WebRTCAddr implements net.Addr for WebRTC connections.
type WebRTCAddr struct {
	network string
	address string
}

func (a *WebRTCAddr) Network() string { return a.network }
func (a *WebRTCAddr) String() string  { return a.address }

// DataChannelConn wraps a detached WebRTC DataChannel to satisfy net.Conn.
type DataChannelConn struct {
	io.ReadWriteCloser
	localAddr  net.Addr
	remoteAddr net.Addr
}

func (c *DataChannelConn) LocalAddr() net.Addr {
	return c.localAddr
}

func (c *DataChannelConn) RemoteAddr() net.Addr {
	return c.remoteAddr
}

func (c *DataChannelConn) SetDeadline(t time.Time) error {
	return nil
}

func (c *DataChannelConn) SetReadDeadline(t time.Time) error {
	return nil
}

func (c *DataChannelConn) SetWriteDeadline(t time.Time) error {
	return nil
}
