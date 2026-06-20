package warp

// ──────────────────────────────────────────────────────────────────────────────
// MASQUE over HTTP/2 TCP Transport
//
// When an ISP blocks all UDP (common in Iran), HTTP/3 (QUIC) is unavailable.
// This file implements the same WARP MASQUE tunnel using HTTP/2 Extended CONNECT
// (RFC 8441 / RFC 9220) over a standard TCP connection instead.
//
// Flow:
//   Browser → SOCKS5:10880 → [H2 CONNECT] → CF edge:443 (TCP/TLS) → Internet
//
// The TLS handshake uses uTLS Chrome fingerprint with ALPN "h2" to look like
// normal browser HTTPS traffic, defeating DPI inspection.
//
// Each SOCKS5 connection opens a new H2 stream on a shared, persistent H2
// connection to the Cloudflare edge. If the connection drops, it is
// transparently re-established before the next request.
// ──────────────────────────────────────────────────────────────────────────────

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"sync"
	"time"

	"clever-connect/internal/logger"
	"clever-connect/internal/models"

	utls "github.com/refraction-networking/utls"
	"golang.org/x/net/http2"
)

// ──────────────────────────────────────────────────────────────────────────────
// masqueH2Client — persistent H2 connection manager
// ──────────────────────────────────────────────────────────────────────────────

// masqueH2Client maintains a single, long-lived HTTP/2 connection to a
// Cloudflare edge node. Re-dials automatically if the connection is dropped.
type masqueH2Client struct {
	mu      sync.Mutex
	h2conn  *http2.ClientConn
	cfAddr  string // IP:port of CF edge
	sni     string // Server Name Indication (TargetSNI from config)
	token   string // Bearer auth token from the WARP account
	account *models.WarpAccount
}

// getConn returns an active H2 ClientConn, dialling a new one if needed.
func (c *masqueH2Client) getConn(ctx context.Context) (*http2.ClientConn, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.h2conn != nil && c.h2conn.CanTakeNewRequest() {
		return c.h2conn, nil
	}

	logger.Info("WARP", "Establishing new H2 connection to CF edge", "addr", c.cfAddr, "sni", c.sni)

	conn, err := c.dialH2(ctx)
	if err != nil {
		return nil, err
	}
	c.h2conn = conn
	return conn, nil
}

// dialH2 creates a fresh uTLS → H2 connection to the Cloudflare edge.
// Uses uTLS Chrome fingerprint to evade DPI that distinguishes Go's TLS stack.
func (c *masqueH2Client) dialH2(ctx context.Context) (*http2.ClientConn, error) {
	// Step 1: TCP connect
	rawConn, err := (&net.Dialer{Timeout: 15 * time.Second}).DialContext(ctx, "tcp", c.cfAddr)
	if err != nil {
		return nil, fmt.Errorf("TCP connect to %s failed: %w", c.cfAddr, err)
	}

	// Step 2: uTLS Chrome handshake, ALPN = "h2" only.
	// Requesting "h2" forces the server to negotiate HTTP/2. Dropping "h3"
	// and "h3-29" prevents the edge from trying to upgrade to QUIC.
	uConn := utls.UClient(rawConn, &utls.Config{
		ServerName: c.sni,
		NextProtos: []string{"h2"},
	}, utls.HelloChrome_Auto)

	if err := uConn.HandshakeContext(ctx); err != nil {
		rawConn.Close()
		return nil, fmt.Errorf("uTLS handshake failed: %w", err)
	}

	negotiated := uConn.ConnectionState().NegotiatedProtocol
	if negotiated != "h2" {
		uConn.Close()
		return nil, fmt.Errorf("expected h2 ALPN, got %q — CF edge may not support H2 MASQUE on this endpoint", negotiated)
	}

	logger.Info("WARP", "H2 TLS handshake complete", "addr", c.cfAddr, "alpn", negotiated)

	// Step 3: Wrap in http2.Transport and create ClientConn.
	// AllowHTTP=false because we already have the TLS conn — http2.Transport
	// just needs to drive the H2 framing layer.
	h2tr := &http2.Transport{
		// We pre-dial, so DialTLSContext is never called. This field just
		// tells the transport the conn is already TLS-wrapped.
		AllowHTTP: false,
	}

	h2conn, err := h2tr.NewClientConn(uConn)
	if err != nil {
		uConn.Close()
		return nil, fmt.Errorf("H2 ClientConn setup failed: %w", err)
	}

	return h2conn, nil
}

// dial opens a new H2 CONNECT stream to targetAddr.
// This becomes the read/write tunnel for one SOCKS5 connection.
func (c *masqueH2Client) dial(ctx context.Context, targetAddr string) (net.Conn, error) {
	h2conn, err := c.getConn(ctx)
	if err != nil {
		return nil, err
	}

	// io.Pipe provides the bidirectional channel:
	//   pw → (what we write goes to Cloudflare as H2 DATA frames)
	//   pr → fed to http.Request.Body (reads from our local writer)
	pr, pw := io.Pipe()

	// HTTP/2 Extended CONNECT (RFC 8441):
	//   :method = CONNECT
	//   :authority = target:port
	//   authorization = Bearer <token>
	//
	// Cloudflare's WARP edge treats this as a tunnel request and proxies
	// TCP traffic to targetAddr on behalf of the authenticated device.
	req := &http.Request{
		Method: http.MethodConnect,
		URL: &url.URL{
			Host:   targetAddr,
			Scheme: "https",
		},
		Host: targetAddr,
		Header: http.Header{
			"Authorization":     {"Bearer " + c.token},
			"Cf-Client-Version": {"a-6.11-2223"},
			"User-Agent":        {"okhttp/3.12.1"},
		},
		Body:       pr,
		Proto:      "HTTP/2.0",
		ProtoMajor: 2,
		ProtoMinor: 0,
	}

	resp, err := h2conn.RoundTrip(req)
	if err != nil {
		pr.Close()
		pw.Close()
		// Connection may be dead — force re-dial on next attempt
		c.mu.Lock()
		c.h2conn = nil
		c.mu.Unlock()
		return nil, fmt.Errorf("H2 CONNECT RoundTrip to %s failed: %w", targetAddr, err)
	}

	if resp.StatusCode != http.StatusOK {
		pr.Close()
		pw.Close()
		resp.Body.Close()
		return nil, fmt.Errorf("H2 CONNECT rejected by CF edge: HTTP %d (check token validity)", resp.StatusCode)
	}

	logger.Debug("WARP", "H2 CONNECT tunnel established", "target", targetAddr)

	return &h2StreamConn{
		reader: resp.Body,
		writer: pw,
		closeOnce: sync.Once{},
		closeFunc: func() {
			pr.Close()
			pw.Close()
			resp.Body.Close()
		},
	}, nil
}

// ──────────────────────────────────────────────────────────────────────────────
// h2StreamConn — net.Conn adapter over an H2 CONNECT stream
// ──────────────────────────────────────────────────────────────────────────────

type h2StreamConn struct {
	reader    io.ReadCloser  // H2 response body (server → client)
	writer    *io.PipeWriter // Pipe writer (client → server via request body)
	closeOnce sync.Once
	closeFunc func()
}

func (c *h2StreamConn) Read(b []byte) (int, error)  { return c.reader.Read(b) }
func (c *h2StreamConn) Write(b []byte) (int, error) { return c.writer.Write(b) }

func (c *h2StreamConn) Close() error {
	c.closeOnce.Do(c.closeFunc)
	return nil
}

func (c *h2StreamConn) LocalAddr() net.Addr            { return &net.TCPAddr{} }
func (c *h2StreamConn) RemoteAddr() net.Addr           { return &net.TCPAddr{} }
func (c *h2StreamConn) SetDeadline(_ time.Time) error      { return nil }
func (c *h2StreamConn) SetReadDeadline(_ time.Time) error  { return nil }
func (c *h2StreamConn) SetWriteDeadline(_ time.Time) error { return nil }

// ──────────────────────────────────────────────────────────────────────────────
// StartMASQUEH2Transport — public entry point called by engine.go
// ──────────────────────────────────────────────────────────────────────────────

// StartMASQUEH2Transport initialises the MASQUE-over-HTTP/2 transport for
// networks where UDP is completely blocked (Iran, etc.).
//
// Parameters:
//   - ctx     : parent context; cancel to tear down the transport
//   - cfg     : global WARP config (SNI, ports)
//   - account : active WARP account (token for auth)
//   - host    : Cloudflare edge IP (no port)
//   - port    : port to connect on (443 recommended for TCP/TLS)
//
// Returns a dialFn compatible with SOCKS5 server Dial config.
func StartMASQUEH2Transport(
	ctx context.Context,
	cfg *models.WarpGlobalConfig,
	account *models.WarpAccount,
	host string,
	port int,
) (dialFn func(ctx context.Context, network, addr string) (net.Conn, error), cancel func(), err error) {

	cfAddr := fmt.Sprintf("%s:%d", host, port)

	client := &masqueH2Client{
		cfAddr:  cfAddr,
		sni:     cfg.TargetSNI,
		token:   account.Token,
		account: account,
	}

	// Verify connectivity immediately — fail fast if the edge is unreachable
	logger.Info("WARP", "Initialising MASQUE/H2 transport (TCP-only mode)",
		"endpoint", cfAddr,
		"sni", cfg.TargetSNI,
	)
	initialConn, err := client.dialH2(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("MASQUE/H2 initial connection failed: %w", err)
	}
	client.mu.Lock()
	client.h2conn = initialConn
	client.mu.Unlock()

	logger.Info("WARP", "MASQUE/H2 transport ready — all traffic tunnelled over TCP/TLS/H2")

	dialFn = func(dialCtx context.Context, network, targetAddr string) (net.Conn, error) {
		return client.dial(dialCtx, targetAddr)
	}

	cancel = func() {
		client.mu.Lock()
		if client.h2conn != nil {
			client.h2conn.Close()
			client.h2conn = nil
		}
		client.mu.Unlock()
	}

	return dialFn, cancel, nil
}
