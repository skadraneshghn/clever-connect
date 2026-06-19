package conn

import (
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"github.com/Ehco1996/ehco/pkg/buffer"
	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
	"go.uber.org/zap"
)

// wsConn represents a WebSocket connection to relay(io.Copy)
type wsConn struct {
	conn      net.Conn
	isServer  bool
	buf       []byte
	writeMu   sync.Mutex
	closeChan chan struct{}
	closeOnce sync.Once
}

func NewWSConn(conn net.Conn, isServer bool) *wsConn {
	c := &wsConn{
		conn:      conn,
		isServer:  isServer,
		buf:       buffer.BufferPool.Get(),
		closeChan: make(chan struct{}),
	}
	if !isServer {
		go c.pingLoop()
	}
	return c
}

func (c *wsConn) Read(b []byte) (n int, err error) {
	for {
		header, err := ws.ReadHeader(c.conn)
		if err != nil {
			return 0, err
		}
		if header.Length > int64(cap(c.buf)) {
			zap.S().Warnf("ws payload size:%d is larger than buffer size:%d", header.Length, cap(c.buf))
			return 0, fmt.Errorf("buffer size:%d too small to transport ws payload size:%d", len(b), header.Length)
		}
		payload := c.buf[:header.Length]
		_, err = io.ReadFull(c.conn, payload)
		if err != nil {
			return 0, err
		}
		if header.Masked {
			ws.Cipher(payload, header.Mask, 0)
		}

		switch header.OpCode {
		case ws.OpPing:
			c.writeMu.Lock()
			if c.isServer {
				err = wsutil.WriteServerMessage(c.conn, ws.OpPong, payload)
			} else {
				err = wsutil.WriteClientMessage(c.conn, ws.OpPong, payload)
			}
			c.writeMu.Unlock()
			if err != nil {
				return 0, err
			}
			continue

		case ws.OpPong:
			// Ignore pong response frames
			continue

		case ws.OpClose:
			return 0, io.EOF

		case ws.OpBinary, ws.OpText, ws.OpContinuation:
			if len(payload) > len(b) {
				return 0, fmt.Errorf("buffer size:%d too small to transport ws payload size:%d", len(b), len(payload))
			}
			copy(b, payload)
			return len(payload), nil

		default:
			zap.S().Warnf("received unknown websocket opcode: %v", header.OpCode)
			continue
		}
	}
}

func (c *wsConn) Write(b []byte) (n int, err error) {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	if c.isServer {
		err = wsutil.WriteServerBinary(c.conn, b)
	} else {
		err = wsutil.WriteClientBinary(c.conn, b)
	}
	if err != nil {
		return 0, err
	}
	return len(b), nil
}

func (c *wsConn) Close() error {
	c.closeOnce.Do(func() {
		close(c.closeChan)
	})
	defer buffer.BufferPool.Put(c.buf)
	return c.conn.Close()
}

func (c *wsConn) LocalAddr() net.Addr {
	return c.conn.LocalAddr()
}

func (c *wsConn) RemoteAddr() net.Addr {
	return c.conn.RemoteAddr()
}

func (c *wsConn) SetDeadline(t time.Time) error {
	return c.conn.SetDeadline(t)
}

func (c *wsConn) SetReadDeadline(t time.Time) error {
	return c.conn.SetReadDeadline(t)
}

func (c *wsConn) SetWriteDeadline(t time.Time) error {
	return c.conn.SetWriteDeadline(t)
}

func (c *wsConn) pingLoop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-c.closeChan:
			return
		case <-ticker.C:
			c.writeMu.Lock()
			var err error
			if c.isServer {
				err = wsutil.WriteServerMessage(c.conn, ws.OpPing, nil)
			} else {
				err = wsutil.WriteClientMessage(c.conn, ws.OpPing, nil)
			}
			c.writeMu.Unlock()
			if err != nil {
				return
			}
		}
	}
}
