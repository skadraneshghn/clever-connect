package client

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync/atomic"

	"clever-connect/internal/bonding/session"
	"clever-connect/internal/logger"

	"github.com/armon/go-socks5"
)

// Frontend manages the user-facing proxy listeners (SOCKS5 and HTTP CONNECT).
// Each incoming connection is assigned a StreamID, framed, and dispatched
// through the bonding dispatcher.
type Frontend struct {
	socksPort  int
	httpPort   int
	frameSize  int
	dispatcher *Dispatcher
	session    *session.Session
	seq        uint64 // global monotonic sequence counter (per direction: c→s)

	socksListener net.Listener
	httpListener  net.Listener
}

// NewFrontend creates a new proxy frontend.
func NewFrontend(socksPort, httpPort, frameSize int, dispatcher *Dispatcher, sess *session.Session) *Frontend {
	return &Frontend{
		socksPort:  socksPort,
		httpPort:   httpPort,
		frameSize:  frameSize,
		dispatcher: dispatcher,
		session:    sess,
	}
}

// Start launches both SOCKS5 and HTTP CONNECT listeners.
func (fe *Frontend) Start(ctx context.Context) error {
	// Start SOCKS5 listener
	if err := fe.startSOCKS5(ctx); err != nil {
		return fmt.Errorf("failed to start SOCKS5 listener: %w", err)
	}

	// Start HTTP CONNECT listener
	if err := fe.startHTTPConnect(ctx); err != nil {
		return fmt.Errorf("failed to start HTTP CONNECT listener: %w", err)
	}

	logger.Info("Bonding", "Frontend listeners started",
		"socks_port", fe.socksPort, "http_port", fe.httpPort)

	return nil
}

// Stop shuts down both listeners.
func (fe *Frontend) Stop() {
	if fe.socksListener != nil {
		_ = fe.socksListener.Close()
	}
	if fe.httpListener != nil {
		_ = fe.httpListener.Close()
	}
}

// startSOCKS5 launches the SOCKS5 server using the go-socks5 library.
func (fe *Frontend) startSOCKS5(ctx context.Context) error {
	addr := fmt.Sprintf("127.0.0.1:%d", fe.socksPort)

	// Create a custom dialer that intercepts connections and routes them through bonding
	resolver := &bondingResolver{
		frontend: fe,
	}

	conf := &socks5.Config{
		Dial:     resolver.dialBonding,
	}

	server, err := socks5.New(conf)
	if err != nil {
		return fmt.Errorf("failed to create SOCKS5 server: %w", err)
	}

	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", addr, err)
	}
	fe.socksListener = listener

	go func() {
		<-ctx.Done()
		_ = listener.Close()
	}()

	go func() {
		if err := server.Serve(listener); err != nil {
			if ctx.Err() == nil {
				logger.Error("Bonding", "SOCKS5 server error", "error", err)
			}
		}
	}()

	return nil
}

// bondingResolver intercepts SOCKS5 dial requests and routes them through bonding.
type bondingResolver struct {
	frontend *Frontend
}

// dialBonding is called by the SOCKS5 library for each connection request.
// Instead of dialing the target directly, we create a bonding stream.
func (br *bondingResolver) dialBonding(ctx context.Context, network, addr string) (net.Conn, error) {
	fe := br.frontend

	// Allocate a new stream
	streamID := nextStreamID()
	stream, _, err := fe.session.OpenStream(streamID)
	if err != nil {
		return nil, fmt.Errorf("failed to open stream: %w", err)
	}

	// Create a pipe: the SOCKS5 library writes to one end, we read from the other
	clientConn, bondingConn := net.Pipe()

	// Start upstream framing (reads from bondingConn → dispatches frames)
	var seq uint64
	go func() {
		HandleUpstream(bondingConn, streamID, addr, fe.dispatcher, &seq, fe.frameSize)
		fe.dispatcher.CleanupFlow(streamID)
	}()

	// Start downstream reassembly (reads frames from stream → writes to bondingConn)
	go HandleDownstream(bondingConn, stream)

	return clientConn, nil
}

// startHTTPConnect launches an HTTP CONNECT proxy server.
func (fe *Frontend) startHTTPConnect(ctx context.Context) error {
	addr := fmt.Sprintf("127.0.0.1:%d", fe.httpPort)

	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", addr, err)
	}
	fe.httpListener = listener

	go func() {
		<-ctx.Done()
		_ = listener.Close()
	}()

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				if ctx.Err() == nil {
					logger.Error("Bonding", "HTTP proxy accept error", "error", err)
				}
				return
			}
			go fe.handleHTTPConnection(ctx, conn)
		}
	}()

	return nil
}

// handleHTTPConnection handles one HTTP proxy connection.
// Supports HTTP CONNECT (tunneling) and regular HTTP proxying.
func (fe *Frontend) handleHTTPConnection(ctx context.Context, conn net.Conn) {
	defer conn.Close()

	reader := bufio.NewReader(conn)
	req, err := http.ReadRequest(reader)
	if err != nil {
		return
	}

	if req.Method == http.MethodConnect {
		fe.handleHTTPConnect(ctx, conn, req)
	} else {
		fe.handleHTTPProxy(ctx, conn, req)
	}
}

// handleHTTPConnect handles HTTPS tunneling via CONNECT method.
func (fe *Frontend) handleHTTPConnect(ctx context.Context, conn net.Conn, req *http.Request) {
	targetAddr := req.Host
	if !strings.Contains(targetAddr, ":") {
		targetAddr = targetAddr + ":443"
	}

	// Respond with 200 OK to establish the tunnel
	_, _ = conn.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n"))

	// Create bonding stream
	streamID := nextStreamID()
	stream, _, err := fe.session.OpenStream(streamID)
	if err != nil {
		logger.Warn("Bonding", "Failed to open stream for CONNECT",
			"target", targetAddr, "error", err)
		return
	}

	var seq uint64

	// Upstream: conn → frames → dispatcher
	go func() {
		HandleUpstream(io.NopCloser(conn), streamID, targetAddr, fe.dispatcher, &seq, fe.frameSize)
		fe.dispatcher.CleanupFlow(streamID)
	}()

	// Downstream: frames → conn
	HandleDownstream(conn, stream)
}

// handleHTTPProxy handles regular HTTP proxy requests (non-CONNECT).
func (fe *Frontend) handleHTTPProxy(ctx context.Context, conn net.Conn, req *http.Request) {
	targetAddr := req.Host
	if !strings.Contains(targetAddr, ":") {
		targetAddr = targetAddr + ":80"
	}

	streamID := nextStreamID()
	stream, _, err := fe.session.OpenStream(streamID)
	if err != nil {
		logger.Warn("Bonding", "Failed to open stream for HTTP proxy",
			"target", targetAddr, "error", err)
		return
	}

	// For regular HTTP, we need to first send the request as raw bytes
	var seq uint64

	// Start upstream with the full connection
	go func() {
		// First, reconstruct and send the original request as OPEN + DATA
		openSeq := atomic.AddUint64(&seq, 1)
		openFrame := newOpenFrame(streamID, openSeq, targetAddr)
		_ = fe.dispatcher.DispatchFrame(openFrame)

		// Forward remaining request body
		HandleUpstream(io.NopCloser(conn), streamID, targetAddr, fe.dispatcher, &seq, fe.frameSize)
		fe.dispatcher.CleanupFlow(streamID)
	}()

	// Downstream
	HandleDownstream(conn, stream)
}
