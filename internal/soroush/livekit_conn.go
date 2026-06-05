package soroush

import (
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	lksdk "github.com/livekit/server-sdk-go/v2"
)

// ──────────────────────────────────────────────────────────────────────────────
// LiveKitConn: Bridges LiveKit SFU DataChannels to a standard net.Conn interface
//
// Each LiveKitConn maps to a specific remote participant identity in the SFU room.
// Write() publishes data targeted at that participant via the SFU.
// Read() pulls from an io.Pipe fed by the listener's data callback.
// ──────────────────────────────────────────────────────────────────────────────

type LiveKitConn struct {
	room           *lksdk.Room
	targetIdentity string // The specific participant we are writing to
	pr             *io.PipeReader
	pw             *io.PipeWriter
	closed         bool
	closeOnce      sync.Once
}

func NewLiveKitConn(room *lksdk.Room, target string) *LiveKitConn {
	pr, pw := io.Pipe()
	return &LiveKitConn{
		room:           room,
		targetIdentity: target,
		pr:             pr,
		pw:             pw,
	}
}

func (c *LiveKitConn) Read(b []byte) (n int, err error) {
	return c.pr.Read(b)
}

func (c *LiveKitConn) Write(b []byte) (n int, err error) {
	if c.closed {
		return 0, fmt.Errorf("cannot write to closed livekit connection")
	}

	payload := make([]byte, len(b))
	copy(payload, b)

	// Explicitly target the peer to prevent broadcasting SOCKS traffic to the whole room.
	// Uses the v2 SDK functional option API for destination targeting.
	err = c.room.LocalParticipant.PublishData(
		payload,
		lksdk.WithDataPublishDestination([]string{c.targetIdentity}),
		lksdk.WithDataPublishReliable(true),
	)
	if err != nil {
		return 0, err
	}

	return len(b), nil
}

func (c *LiveKitConn) Close() error {
	c.closeOnce.Do(func() {
		c.closed = true
		c.pr.Close()
		c.pw.Close()
	})
	return nil
}

// WriteIncoming injects data from the LiveKit data callback into the Read pipe.
// Called by the LiveKitListener when it routes incoming data to this connection.
func (c *LiveKitConn) WriteIncoming(b []byte) error {
	_, err := c.pw.Write(b)
	return err
}

func (c *LiveKitConn) LocalAddr() net.Addr                { return &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0} }
func (c *LiveKitConn) RemoteAddr() net.Addr               { return &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0} }
func (c *LiveKitConn) SetDeadline(t time.Time) error      { return nil }
func (c *LiveKitConn) SetReadDeadline(t time.Time) error  { return nil }
func (c *LiveKitConn) SetWriteDeadline(t time.Time) error { return nil }

// Compile-time check that LiveKitConn implements net.Conn
var _ net.Conn = (*LiveKitConn)(nil)

// ──────────────────────────────────────────────────────────────────────────────
// LiveKitListener: Simulates a net.Listener over LiveKit SFU Participants
//
// Implements the "Listener Pattern" (Option A) for multi-tenant SFU usage.
// When a new participant sends its first data packet, the listener spawns
// a LiveKitConn, maps it by ParticipantIdentity, and pushes it into the
// Accept() channel. Subsequent data from the same participant is routed
// directly to the existing connection's pipe.
// ──────────────────────────────────────────────────────────────────────────────

type LiveKitListener struct {
	room     *lksdk.Room
	conns    map[string]*LiveKitConn
	acceptCh chan *LiveKitConn
	mu       sync.Mutex
	closed   bool
}

// NewLiveKitListenerCallback creates a RoomCallback with OnDataReceived wired
// to the listener's routing logic. This callback MUST be passed to
// ConnectToRoomWithToken since the room's callback field is unexported.
func NewLiveKitListenerCallback(l *LiveKitListener) *lksdk.RoomCallback {
	cb := lksdk.NewRoomCallback()
	cb.OnDataReceived = func(data []byte, params lksdk.DataReceiveParams) {
		if params.Sender == nil {
			return
		}

		id := params.SenderIdentity

		l.mu.Lock()
		conn, exists := l.conns[id]

		// [CRITICAL FIX] If a connection exists but was previously closed
		// (e.g., Yamux drop, worker restart), purge it so a fresh pipe
		// can be spawned for the reconnecting worker. Without this, the
		// old closed PipeWriter silently drops all data, permanently
		// locking out the reconnecting worker.
		if exists && conn.closed {
			delete(l.conns, id)
			exists = false
		}

		if !exists {
			// First time seeing data from this participant (or after reconnect)
			conn = NewLiveKitConn(l.room, id)
			l.conns[id] = conn
			l.acceptCh <- conn
		}
		l.mu.Unlock()

		// Route data to the specific connection's pipe
		_ = conn.WriteIncoming(data)
	}
	return cb
}

// NewLiveKitListener creates a listener and returns it along with the
// RoomCallback that must be used when connecting to the LiveKit room.
// Usage:
//
//	listener, cb := NewLiveKitListener()
//	room, err := lksdk.ConnectToRoomWithToken(url, token, cb)
//	listener.BindRoom(room)
func NewLiveKitListener() (*LiveKitListener, *lksdk.RoomCallback) {
	l := &LiveKitListener{
		conns:    make(map[string]*LiveKitConn),
		acceptCh: make(chan *LiveKitConn, 100),
	}
	cb := NewLiveKitListenerCallback(l)
	return l, cb
}

// BindRoom sets the room reference after connection.
// Must be called after ConnectToRoomWithToken succeeds.
func (l *LiveKitListener) BindRoom(room *lksdk.Room) {
	l.room = room
}

func (l *LiveKitListener) Accept() (net.Conn, error) {
	conn, ok := <-l.acceptCh
	if !ok {
		return nil, fmt.Errorf("listener closed")
	}
	return conn, nil
}

func (l *LiveKitListener) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if !l.closed {
		l.closed = true
		close(l.acceptCh)
		for _, conn := range l.conns {
			conn.Close()
		}
	}
	return nil
}

func (l *LiveKitListener) Addr() net.Addr {
	return &net.TCPAddr{IP: net.ParseIP("0.0.0.0"), Port: 0}
}

// Compile-time check that LiveKitListener implements net.Listener
var _ net.Listener = (*LiveKitListener)(nil)
