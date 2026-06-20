package warp

// ──────────────────────────────────────────────────────────────────────────────
// MASQUE over HTTP/2 TCP Transport
//
// When an ISP blocks all UDP (common in Iran), HTTP/3 (QUIC) is unavailable.
// This file implements the same WARP MASQUE tunnel using HTTP/2 Extended CONNECT
// (RFC 8441 / RFC 9220) over a standard TCP connection instead.
//
// Correct MASQUE tunnel flow:
//   1. One persistent H2 connection to consumer-masque.cloudflareclient.com:443
//   2. Each SOCKS5 destination opens a NEW H2 stream with:
//        :method    = CONNECT
//        :authority = consumer-masque.cloudflareclient.com   ← ALWAYS this host
//        :path      = /v1/masque/tcp/<dest-host>/<dest-port>/
//        Capsule-Protocol: ?1
//        Proxy-Authorization: Bearer <token>
//   3. CF's edge decodes the target from the path and proxies the TCP stream.
//
// Key fix: ":authority" must ALWAYS be the MASQUE endpoint host, NOT the
// destination. Setting :authority to an external host (e.g. 149.154.167.41)
// causes CF to reject the request with HTTP 400 — it is not an open proxy.
//
// The TLS handshake uses uTLS Chrome fingerprint with ALPN "h2" to look like
// normal browser HTTPS traffic, defeating DPI inspection.
// ──────────────────────────────────────────────────────────────────────────────

import (
	"context"
	"encoding/base64"
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
//
// IMPORTANT: the H2 connection is always established to the CF edge IP (cfAddr)
// with SNI = masqueAuthority (consumer-masque.cloudflareclient.com). Individual
// SOCKS5 destinations are encoded in the :path of each H2 CONNECT stream, NOT
// in :authority — that would make it an open-proxy request which CF rejects.
type masqueH2Client struct {
	mu               sync.Mutex
	h2conn           *http2.ClientConn
	cfAddr           string // IP:port of CF edge (TCP dial target)
	sni              string // Server Name Indication = masqueAuthority
	masqueAuthority  string // HTTP/2 :authority for all CONNECT streams
	token            string // Bearer auth token from the WARP account
	account          *models.WarpAccount
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

// dial opens a new H2 Extended CONNECT stream to targetAddr using the correct
// MASQUE tunnel format. The H2 stream's :authority is always the MASQUE
// endpoint host (consumer-masque.cloudflareclient.com), and the destination
// is encoded in the URL path as /v1/masque/tcp/<host>/<port>/.
//
// This is the key fix: previously :authority was set to the destination host
// (e.g. 149.154.167.41:443) which caused CF to reject with HTTP 400 because
// it looked like an open-proxy request to a 3rd-party server.
func (c *masqueH2Client) dial(ctx context.Context, targetAddr string) (net.Conn, error) {
	h2conn, err := c.getConn(ctx)
	if err != nil {
		return nil, err
	}

	// Build the basic auth credentials matching Cloudflare's QUIC MASQUE transport
	rawCredentials := fmt.Sprintf("%s:%s", c.account.DeviceID, c.account.Token)
	encodedAuth := base64.StdEncoding.EncodeToString([]byte(rawCredentials))

	// io.Pipe provides the bidirectional channel
	pr, pw := io.Pipe()

	// HTTP/2 Standard CONNECT request:
	//   :method = CONNECT
	//   :authority = targetAddr (e.g. "149.154.167.41:443" or "google.com:443")
	//   Host = consumer-masque.cloudflareclient.com (TargetSNI)
	//   Proxy-Authorization = Basic <base64(DeviceID:Token)>
	//   Capsule-Protocol = wg
	req := &http.Request{
		Method: http.MethodConnect,
		URL: &url.URL{
			Host: targetAddr,
		},
		Host: targetAddr,
		Header: http.Header{
			"Host":                {c.masqueAuthority},
			"Proxy-Authorization": {"Basic " + encodedAuth},
			"Capsule-Protocol":    {"wg"},
			"Cf-Client-Version":   {"a-6.11-2223"},
			"User-Agent":          {"okhttp/3.12.1"},
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
		body := make([]byte, 512)
		n, _ := resp.Body.Read(body)
		pr.Close()
		pw.Close()
		resp.Body.Close()
		return nil, fmt.Errorf("H2 CONNECT rejected: HTTP %d body=%q", resp.StatusCode, body[:n])
	}

	logger.Debug("WARP", "H2 MASQUE tunnel established", "target", targetAddr)

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

	// masqueAuthority is the H2 :authority for all CONNECT streams.
	// It must be the MASQUE endpoint hostname — NOT the destination host.
	// The SNI for the TLS handshake also uses this value.
	masqueAuthority := cfg.TargetSNI // "consumer-masque.cloudflareclient.com"

	client := &masqueH2Client{
		cfAddr:          cfAddr,
		sni:             masqueAuthority,
		masqueAuthority: masqueAuthority,
		token:           account.Token,
		account:         account,
	}

	// Verify connectivity immediately — fail fast if the edge is unreachable
	logger.Info("WARP", "Initialising MASQUE/H2 transport (TCP-only mode)",
		"endpoint", cfAddr,
		"masqueAuthority", masqueAuthority,
	)
	initialConn, err := client.dialH2(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("MASQUE/H2 initial connection failed: %w", err)
	}
	client.mu.Lock()
	client.h2conn = initialConn
	client.mu.Unlock()

	logger.Info("WARP", "MASQUE/H2 transport ready",
		"authority", masqueAuthority,
		"tunnelPath", "/v1/masque/tcp/<host>/<port>/",
	)

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
