package warp

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"clever-connect/internal/db/pebble"
	"clever-connect/internal/logger"
	"clever-connect/internal/models"

	socks5 "github.com/armon/go-socks5"
	"github.com/quic-go/quic-go"
)

// ──────────────────────────────────────────────────────────────────────────────
// Pure Userspace Multi-Transport Engine (Pillar 4)
//
// Maps local SOCKS5/HTTP proxy endpoints onto Cloudflare WARP transport
// strategies. Supports two modes:
//   Mode A (masque):    HTTP/3 CONNECT tunnel over QUIC (RFC 9298-inspired)
//   Mode B (wireguard): WireGuard over gVisor userspace netstack
//
// Runs entirely in userspace memory — zero root/host modifications required.
// ──────────────────────────────────────────────────────────────────────────────

// EngineState represents the current state of the WARP engine.
type EngineState int32

const (
	EngineStateStopped  EngineState = 0
	EngineStateStarting EngineState = 1
	EngineStateRunning  EngineState = 2
)

// EngineStatus is the public status snapshot returned by GetStatus().
type EngineStatus struct {
	State          string  `json:"state"`           // "stopped", "starting", "running"
	TransportMode  string  `json:"transport_mode"`  // "masque", "masque_tcp", or "wireguard"
	SocksPort      int     `json:"socks_port"`
	HTTPPort       int     `json:"http_port"`
	ActiveEndpoint string  `json:"active_endpoint"` // IP:Port of current CF edge node
	AccountType    string  `json:"account_type"`
	LastTraceOK    bool    `json:"last_trace_ok"`
	Uptime         string  `json:"uptime"`
	TCPFallback    bool    `json:"tcp_fallback"`    // true when operating in TCP-only mode
}

// WarpEngine is the singleton tunnel engine.
type WarpEngine struct {
	mu sync.Mutex

	state     atomic.Int32
	ctx       context.Context
	cancel    context.CancelFunc
	startTime time.Time

	// Configuration
	cfg     *models.WarpGlobalConfig
	account *models.WarpAccount

	// Active transport
	activeEndpoint *pebble.WarpScanResult
	socksListener  net.Listener
	httpListener   net.Listener

	// MASQUE/QUIC transport
	quicConn *quic.Conn

	// TCP mode flag
	tcpFallback bool

	// WireGuard userspace device cancel func
	wgCancel func()

	// MASQUE/H2 transport cancel func (tears down persistent H2 connection)
	h2Cancel func()
}

var (
	engineOnce     sync.Once
	engineInstance *WarpEngine
)

// GetEngine returns the singleton WarpEngine instance.
func GetEngine() *WarpEngine {
	engineOnce.Do(func() {
		engineInstance = &WarpEngine{}
	})
	return engineInstance
}

// StartEngine initializes and starts the WARP tunnel engine.
func (e *WarpEngine) StartEngine(cfg *models.WarpGlobalConfig, account *models.WarpAccount) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	currentState := EngineState(e.state.Load())
	if currentState == EngineStateRunning || currentState == EngineStateStarting {
		// Engine is already up (or stuck in Starting after a panic).
		// Stop it first so the caller can restart cleanly.
		logger.Warn("WARP", "Engine already active — performing stop before restart", "state", currentState)
		e.stopLocked()
	}

	e.state.Store(int32(EngineStateStarting))

	logger.Info("WARP", "Starting WARP+ engine",
		"mode", cfg.TransportMode,
		"socksPort", cfg.SocksPort,
		"httpPort", cfg.HTTPPort,
	)

	// Verify port availability before touching network
	if err := checkPortAvailable(cfg.SocksPort); err != nil {
		e.state.Store(int32(EngineStateStopped))
		return fmt.Errorf("SOCKS port %d unavailable: %w", cfg.SocksPort, err)
	}
	if err := checkPortAvailable(cfg.HTTPPort); err != nil {
		e.state.Store(int32(EngineStateStopped))
		return fmt.Errorf("HTTP port %d unavailable: %w", cfg.HTTPPort, err)
	}

	// ── Ranked endpoint retry loop ────────────────────────────────────────────
	// GetRankedEndpoints returns all stored endpoints in quality order:
	//   Tier 1: QUIC-capable,  zero fails,  highest score first
	//   Tier 2: TCP-only,      zero fails,  highest score first
	//   Tier 3: any endpoint with prior failures (last resort)
	//
	// On each attempt: try to establish the transport. If it fails, increment
	// the endpoint's FailCount in PebbleDB (permanently lowers its rank), then
	// move to the next candidate.
	const maxEndpointAttempts = 5

	candidates, err := pebble.GetRankedEndpoints(cfg.TransportMode)
	if err != nil {
		e.state.Store(int32(EngineStateStopped))
		return fmt.Errorf("no available endpoints: %w", err)
	}

	if len(candidates) > maxEndpointAttempts {
		candidates = candidates[:maxEndpointAttempts]
	}

	var lastErr error
	for i, endpoint := range candidates {
		ep := endpoint // capture loop variable
		logger.Info("WARP", "Trying endpoint",
			"attempt", fmt.Sprintf("%d/%d", i+1, len(candidates)),
			"ip", ep.IPAddress,
			"port", ep.Port,
			"latency", fmt.Sprintf("%.0fms", ep.LatencyMs),
			"score", fmt.Sprintf("%.1f", ep.Score),
			"failCount", ep.FailCount,
			"restricted", ep.IsRestricted,
		)

		// Set up fresh context for this attempt
		e.ctx, e.cancel = context.WithCancel(context.Background())
		e.cfg = cfg
		e.account = account
		e.activeEndpoint = &ep
		e.startTime = time.Now()

		// Attempt transport dial
		var transportErr error
		switch cfg.TransportMode {
		case "masque":
			transportErr = e.startMASQUETransport()
		case "masque_h2":
			transportErr = e.startMASQUEH2Transport()
		case "wireguard":
			transportErr = e.startWireGuardTransport()
		default:
			e.cancel()
			e.state.Store(int32(EngineStateStopped))
			return fmt.Errorf("unsupported transport mode: %s (valid: masque, masque_h2, wireguard)", cfg.TransportMode)
		}

		if transportErr == nil {
			// Success — engine is up on this endpoint
			e.state.Store(int32(EngineStateRunning))
			logger.Info("WARP", "WARP+ engine started successfully",
				"mode", cfg.TransportMode,
				"endpoint", fmt.Sprintf("%s:%d", ep.IPAddress, ep.Port),
				"score", fmt.Sprintf("%.1f", ep.Score),
				"socksPort", cfg.SocksPort,
			)
			return nil
		}

		// Failure — penalise this endpoint and try the next one
		lastErr = transportErr
		logger.Warn("WARP", "Endpoint failed, trying next",
			"ip", ep.IPAddress,
			"port", ep.Port,
			"error", transportErr.Error(),
		)
		// Cancel context before next attempt
		e.cancel()
		e.cancel = nil

		// Increment failure count in PebbleDB (lowers score for future connections)
		_ = pebble.IncrementEndpointFailCount(cfg.TransportMode, ep.IPAddress, ep.Port)
	}

	// All candidates exhausted
	e.state.Store(int32(EngineStateStopped))
	return fmt.Errorf("all %d endpoints failed for mode %s; last error: %w", len(candidates), cfg.TransportMode, lastErr)
}

// StopEngine safely tears down the WARP tunnel engine.
func (e *WarpEngine) StopEngine() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.stopLocked()
}

// stopLocked performs the actual engine teardown.
// MUST be called with e.mu held.
func (e *WarpEngine) stopLocked() error {
	if EngineState(e.state.Load()) == EngineStateStopped {
		return nil
	}

	logger.Info("WARP", "Stopping WARP+ engine")

	// Cancel context to signal all goroutines
	if e.cancel != nil {
		e.cancel()
		e.cancel = nil
	}

	// Close QUIC connection
	if e.quicConn != nil {
		e.quicConn.CloseWithError(0, "engine shutdown")
		e.quicConn = nil
	}

	// Close WireGuard userspace device
	// NOTE: wgCancel calls wgDev.Close() which internally closes the TUN device.
	// Do NOT call tunDev.Close() separately — it will panic ("close of closed channel").
	if e.wgCancel != nil {
		e.wgCancel()
		e.wgCancel = nil
	}

	// Close MASQUE/H2 connection
	if e.h2Cancel != nil {
		e.h2Cancel()
		e.h2Cancel = nil
	}

	// Close listeners
	if e.socksListener != nil {
		e.socksListener.Close()
		e.socksListener = nil
	}
	if e.httpListener != nil {
		e.httpListener.Close()
		e.httpListener = nil
	}

	e.state.Store(int32(EngineStateStopped))
	e.tcpFallback = false
	e.activeEndpoint = nil
	e.cfg = nil
	e.account = nil

	logger.Info("WARP", "WARP+ engine stopped successfully")
	return nil
}

// IsRunning returns whether the engine is currently running.
func (e *WarpEngine) IsRunning() bool {
	return EngineState(e.state.Load()) == EngineStateRunning
}

// GetStatus returns the current engine status.
func (e *WarpEngine) GetStatus() EngineStatus {
	state := EngineState(e.state.Load())
	status := EngineStatus{
		State: "stopped",
	}

	switch state {
	case EngineStateStarting:
		status.State = "starting"
	case EngineStateRunning:
		status.State = "running"
	}

	if e.cfg != nil {
		mode := e.cfg.TransportMode
		if e.tcpFallback {
			mode = mode + "_tcp" // e.g. "masque_tcp"
		}
		status.TransportMode = mode
		status.SocksPort = e.cfg.SocksPort
		status.HTTPPort = e.cfg.HTTPPort
		status.LastTraceOK = e.cfg.LastTraceOK
		status.TCPFallback = e.tcpFallback
	}

	if e.account != nil {
		status.AccountType = e.account.AccountType
	}

	if e.activeEndpoint != nil {
		status.ActiveEndpoint = fmt.Sprintf("%s:%d", e.activeEndpoint.IPAddress, e.activeEndpoint.Port)
	}

	if state == EngineStateRunning {
		status.Uptime = time.Since(e.startTime).Truncate(time.Second).String()
	}

	return status
}

// ──────────────────────────────────────────────────────────────────────────────
// Mode A2: MASQUE over HTTP/2 TCP Transport (UDP-blocked networks)
// ──────────────────────────────────────────────────────────────────────────────

// startMASQUEH2Transport starts the HTTP/2 MASQUE tunnel for networks where
// all UDP is blocked. Uses TCP port 443 with uTLS Chrome fingerprint + ALPN h2.
func (e *WarpEngine) startMASQUEH2Transport() error {
	// Fix #1: split host from any port suffix in the stored IP address
	host := e.activeEndpoint.IPAddress
	if h, _, err := net.SplitHostPort(e.activeEndpoint.IPAddress); err == nil {
		host = h
	}
	// H2/MASQUE always uses TCP port 443
	port := 443

	logger.Info("WARP", "Starting MASQUE/H2 transport (TCP-only — UDP is blocked)",
		"endpoint", fmt.Sprintf("%s:%d", host, port),
		"sni", e.cfg.TargetSNI,
	)

	dialFn, cancel, err := StartMASQUEH2Transport(e.ctx, e.cfg, e.account, host, port)
	if err != nil {
		return fmt.Errorf("MASQUE/H2 init failed: %w", err)
	}
	e.h2Cancel = cancel

	// ── SOCKS5 server ─────────────────────────────────────────────────────────
	socksConf := &socks5.Config{Dial: dialFn}
	socksServer, err := socks5.New(socksConf)
	if err != nil {
		cancel()
		return fmt.Errorf("failed to create SOCKS5 server: %w", err)
	}

	socksAddr := fmt.Sprintf("127.0.0.1:%d", e.cfg.SocksPort)
	e.socksListener, err = net.Listen("tcp", socksAddr)
	if err != nil {
		cancel()
		return fmt.Errorf("failed to bind SOCKS5 port %d: %w", e.cfg.SocksPort, err)
	}

	go func() {
		logger.Info("WARP", "MASQUE/H2 SOCKS5 proxy listening", "addr", socksAddr)
		if err := socksServer.Serve(e.socksListener); err != nil {
			if e.ctx.Err() == nil {
				logger.Error("WARP", "MASQUE/H2 SOCKS5 server error", "error", err)
			}
		}
	}()

	go e.startHTTPProxy(dialFn)

	return nil
}

// ──────────────────────────────────────────────────────────────────────────────
// Mode A: MASQUE HTTP/3 Tunnel Transport (RFC 9298-inspired)
// ──────────────────────────────────────────────────────────────────────────────


// startMASQUETransport initializes the QUIC/H3 MASQUE tunnel.
// Fix #4: ALPN is explicitly ["h3","h3-29"] and SNI is set from TargetSNI config.
// If QUIC is unavailable (ISP blocks UDP), returns an error directing the user
// to switch to wireguard transport which supports UDP-free operation.
func (e *WarpEngine) startMASQUETransport() error {
	// Fix #1: split IP and port explicitly to prevent duplication
	host := e.activeEndpoint.IPAddress
	if h, _, err := net.SplitHostPort(e.activeEndpoint.IPAddress); err == nil {
		host = h
	}
	addr := fmt.Sprintf("%s:%d", host, e.activeEndpoint.Port)

	if e.activeEndpoint.IsRestricted {
		return fmt.Errorf(
			"endpoint %s is restricted (UDP/QUIC blocked by ISP) — switch transport to 'wireguard' mode which handles restricted networks",
			addr,
		)
	}

	// Fix #4: explicit ALPN ["h3","h3-29"] + SNI from config (not InsecureSkipVerify)
	tlsConf := &tls.Config{
		NextProtos: []string{"h3", "h3-29"},
		ServerName: e.cfg.TargetSNI,
		// InsecureSkipVerify intentionally false — Cloudflare has valid certs
		InsecureSkipVerify: false,
	}

	quicConf := &quic.Config{
		MaxIdleTimeout:  120 * time.Second,
		KeepAlivePeriod: 30 * time.Second,
	}

	conn, err := quic.DialAddr(e.ctx, addr, tlsConf, quicConf)
	if err != nil {
		return fmt.Errorf(
			"QUIC dial to %s failed (UDP may be blocked): %w — try switching to 'wireguard' transport mode",
			addr, err,
		)
	}

	e.quicConn = conn
	e.tcpFallback = false
	logger.Info("WARP", "QUIC/H3 session established",
		"endpoint", addr,
		"protocol", conn.ConnectionState().TLS.NegotiatedProtocol,
		"sni", e.cfg.TargetSNI,
	)

	// ── Dial function: each SOCKS5 connection opens a new QUIC stream ────────
	dialFn := func(ctx context.Context, network, targetAddr string) (net.Conn, error) {
		return e.dialViaMASQUE(ctx, targetAddr)
	}

	// ── Start local SOCKS5 proxy server ──────────────────────────────────────
	socksConf := &socks5.Config{Dial: dialFn}
	socksServer, err := socks5.New(socksConf)
	if err != nil {
		return fmt.Errorf("failed to create SOCKS5 server: %w", err)
	}

	socksAddr := fmt.Sprintf("127.0.0.1:%d", e.cfg.SocksPort)
	e.socksListener, err = net.Listen("tcp", socksAddr)
	if err != nil {
		return fmt.Errorf("failed to bind SOCKS5 port %d: %w", e.cfg.SocksPort, err)
	}

	go func() {
		logger.Info("WARP", "MASQUE SOCKS5 proxy listening", "addr", socksAddr)
		if err := socksServer.Serve(e.socksListener); err != nil {
			if e.ctx.Err() == nil {
				logger.Error("WARP", "SOCKS5 server error", "error", err)
			}
		}
	}()

	go e.startHTTPProxy(dialFn)
	go e.monitorConnection()

	return nil
}

// dialViaMASQUE creates a new QUIC stream for a SOCKS5 connection request,
// wrapping it in an HTTP CONNECT request through the MASQUE tunnel.
func (e *WarpEngine) dialViaMASQUE(ctx context.Context, targetAddr string) (net.Conn, error) {
	if e.quicConn == nil {
		return nil, fmt.Errorf("QUIC connection not established")
	}

	// Open a new multiplexed QUIC stream
	stream, err := e.quicConn.OpenStreamSync(ctx)
	if err != nil {
		// If stream open fails, try to re-establish the QUIC session
		logger.Warn("WARP", "QUIC stream open failed, attempting reconnect", "error", err)
		if reconnErr := e.reconnectMASQUE(); reconnErr != nil {
			return nil, fmt.Errorf("QUIC reconnect failed: %w", reconnErr)
		}
		stream, err = e.quicConn.OpenStreamSync(ctx)
		if err != nil {
			return nil, fmt.Errorf("QUIC stream open failed after reconnect: %w", err)
		}
	}

	// Send HTTP CONNECT request through the stream
	rawCredentials := fmt.Sprintf("%s:%s", e.account.DeviceID, e.account.Token)
	encodedAuth := base64.StdEncoding.EncodeToString([]byte(rawCredentials))

	connectReq := fmt.Sprintf(
		"CONNECT %s HTTP/1.1\r\n"+
			"Host: %s\r\n"+
			"Proxy-Authorization: Basic %s\r\n"+
			"Capsule-Protocol: wg\r\n"+
			"\r\n",
		targetAddr,
		e.cfg.TargetSNI,
		encodedAuth,
	)

	if _, err := stream.Write([]byte(connectReq)); err != nil {
		stream.Close()
		return nil, fmt.Errorf("failed to send CONNECT request: %w", err)
	}

	// Read the CONNECT response
	buf := make([]byte, 4096)
	n, err := stream.Read(buf)
	if err != nil && err != io.EOF {
		stream.Close()
		return nil, fmt.Errorf("failed to read CONNECT response: %w", err)
	}

	response := string(buf[:n])
	if len(response) < 12 || response[9:12] != "200" {
		stream.Close()
		return nil, fmt.Errorf("CONNECT request rejected: %s", response)
	}

	// Return a net.Conn adapter wrapping the QUIC stream
	return &quicStreamConn{
		stream:     stream,
		localAddr:  e.socksListener.Addr(),
		remoteAddr: e.socksListener.Addr(),
	}, nil
}

// reconnectMASQUE re-establishes the QUIC session to the active endpoint.
func (e *WarpEngine) reconnectMASQUE() error {
	if e.quicConn != nil {
		e.quicConn.CloseWithError(0, "reconnecting")
	}

	addr := fmt.Sprintf("%s:%d", e.activeEndpoint.IPAddress, e.activeEndpoint.Port)

	tlsConf := &tls.Config{
		NextProtos:         []string{"h3", "h3-29"},
		ServerName:         e.cfg.TargetSNI,
		InsecureSkipVerify: true,
	}

	quicConf := &quic.Config{
		MaxIdleTimeout:  120 * time.Second,
		KeepAlivePeriod: 30 * time.Second,
	}

	conn, err := quic.DialAddr(e.ctx, addr, tlsConf, quicConf)
	if err != nil {
		return err
	}

	e.quicConn = conn
	logger.Info("WARP", "QUIC session re-established", "endpoint", addr)
	return nil
}

// monitorConnection periodically checks the QUIC connection health.
func (e *WarpEngine) monitorConnection() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-e.ctx.Done():
			return
		case <-ticker.C:
			if e.quicConn == nil {
				continue
			}
			// Check if connection is still alive by opening a test stream
			testCtx, cancel := context.WithTimeout(e.ctx, 5*time.Second)
			stream, err := e.quicConn.OpenStreamSync(testCtx)
			cancel()
			if err != nil {
				logger.Warn("WARP", "Connection health check failed, attempting reconnect", "error", err)
				if reconnErr := e.reconnectMASQUE(); reconnErr != nil {
					logger.Error("WARP", "Reconnect failed, attempting endpoint rotation", "error", reconnErr)
					e.rotateEndpoint()
				}
			} else {
				stream.Close()
			}
		}
	}
}

// rotateEndpoint switches to the next best endpoint from PebbleDB.
func (e *WarpEngine) rotateEndpoint() {
	if e.activeEndpoint != nil {
		// Mark current endpoint as restricted
		_ = pebble.MarkWarpEndpointRestricted(
			e.cfg.TransportMode,
			e.activeEndpoint.IPAddress,
			e.activeEndpoint.Port,
		)
	}

	// Get next best endpoint
	endpoint, err := pebble.GetBestWarpEndpoint(e.cfg.TransportMode)
	if err != nil {
		logger.Error("WARP", "No backup endpoints available for rotation", "error", err)
		return
	}

	e.activeEndpoint = endpoint
	logger.Info("WARP", "Rotated to new endpoint",
		"ip", endpoint.IPAddress,
		"port", endpoint.Port,
	)

	// Reconnect with new endpoint
	if e.cfg.TransportMode == "masque" {
		if err := e.reconnectMASQUE(); err != nil {
			logger.Error("WARP", "Failed to reconnect after endpoint rotation", "error", err)
		}
	}
}

// startHTTPProxy starts an HTTP CONNECT proxy.
// dialFn is the transport-specific dial function (WireGuard or MASQUE).
func (e *WarpEngine) startHTTPProxy(dialFn func(ctx context.Context, network, addr string) (net.Conn, error)) {
	httpAddr := fmt.Sprintf("127.0.0.1:%d", e.cfg.HTTPPort)

	var err error
	e.httpListener, err = net.Listen("tcp", httpAddr)
	if err != nil {
		logger.Error("WARP", "Failed to bind HTTP proxy port", "port", e.cfg.HTTPPort, "error", err)
		return
	}

	logger.Info("WARP", "HTTP proxy listening", "addr", httpAddr)

	server := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodConnect {
				e.handleHTTPConnect(w, r, dialFn)
			} else {
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			}
		}),
	}

	if err := server.Serve(e.httpListener); err != nil {
		if e.ctx.Err() == nil {
			logger.Error("WARP", "HTTP proxy server error", "error", err)
		}
	}
}

// handleHTTPConnect handles HTTP CONNECT proxy requests using the provided dialFn.
func (e *WarpEngine) handleHTTPConnect(w http.ResponseWriter, r *http.Request, dialFn func(ctx context.Context, network, addr string) (net.Conn, error)) {
	targetConn, err := dialFn(r.Context(), "tcp", r.Host)
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	defer targetConn.Close()

	w.WriteHeader(http.StatusOK)

	hijacker, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "Hijacking not supported", http.StatusInternalServerError)
		return
	}

	clientConn, _, err := hijacker.Hijack()
	if err != nil {
		return
	}
	defer clientConn.Close()

	// Bidirectional proxy
	go io.Copy(targetConn, clientConn)
	io.Copy(clientConn, targetConn)
}

// ──────────────────────────────────────────────────────────────────────────────
// Mode B: WireGuard/gVisor Userspace Netstack Transport
// ──────────────────────────────────────────────────────────────────────────────

// startWireGuardTransport initializes a real userspace WireGuard tunnel using
// wireguard-go + gVisor netstack. Traffic exits via Cloudflare WARP edge over UDP.
// Works even when MASQUE/HTTP3 is blocked because WireGuard uses raw UDP on port 2408.
func (e *WarpEngine) startWireGuardTransport() error {
	// Fix #1: extract plain host to prevent port duplication
	host := e.activeEndpoint.IPAddress
	if h, _, err := net.SplitHostPort(e.activeEndpoint.IPAddress); err == nil {
		host = h
	}
	// WireGuard ALWAYS runs on UDP port 2408 on Cloudflare's edge.
	// The scanner probes TCP port 443 for MASQUE mode and stores those results
	// in the wireguard namespace too, so we must ignore the scanned port and
	// always use 2408 — the only port CF accepts WireGuard UDP handshakes on.
	port := 2408

	logger.Info("WARP", "Starting real userspace WireGuard tunnel",
		"endpoint", fmt.Sprintf("%s:%d", host, port),
		"mtu", 1280,
	)

	// StartWireGuardUserspace (wireguard.go) applies Fix #1, #2, #3
	dialFn, cancel, err := StartWireGuardUserspace(e.ctx, e.account, host, port)
	if err != nil {
		return fmt.Errorf("WireGuard userspace init failed: %w", err)
	}
	e.wgCancel = cancel

	// ── Start local SOCKS5 proxy ──────────────────────────────────────────────
	socksConf := &socks5.Config{Dial: dialFn}
	socksServer, err := socks5.New(socksConf)
	if err != nil {
		cancel()
		return fmt.Errorf("failed to create SOCKS5 server: %w", err)
	}

	socksAddr := fmt.Sprintf("127.0.0.1:%d", e.cfg.SocksPort)
	e.socksListener, err = net.Listen("tcp", socksAddr)
	if err != nil {
		cancel()
		return fmt.Errorf("failed to bind SOCKS5 port %d: %w", e.cfg.SocksPort, err)
	}

	go func() {
		logger.Info("WARP", "WireGuard SOCKS5 proxy listening", "addr", socksAddr)
		if err := socksServer.Serve(e.socksListener); err != nil {
			if e.ctx.Err() == nil {
				logger.Error("WARP", "WireGuard SOCKS5 server error", "error", err)
			}
		}
	}()

	go e.startHTTPProxy(dialFn)

	return nil
}

// ──────────────────────────────────────────────────────────────────────────────
// QUIC Stream → net.Conn Adapter
// ──────────────────────────────────────────────────────────────────────────────

// quicStreamConn wraps a *quic.Stream into a net.Conn interface.
type quicStreamConn struct {
	stream     *quic.Stream
	localAddr  net.Addr
	remoteAddr net.Addr
}

func (c *quicStreamConn) Read(b []byte) (int, error)         { return c.stream.Read(b) }
func (c *quicStreamConn) Write(b []byte) (int, error)        { return c.stream.Write(b) }
func (c *quicStreamConn) Close() error                       { return c.stream.Close() }
func (c *quicStreamConn) LocalAddr() net.Addr                { return c.localAddr }
func (c *quicStreamConn) RemoteAddr() net.Addr               { return c.remoteAddr }
func (c *quicStreamConn) SetDeadline(t time.Time) error      { return c.stream.SetDeadline(t) }
func (c *quicStreamConn) SetReadDeadline(t time.Time) error  { return c.stream.SetReadDeadline(t) }
func (c *quicStreamConn) SetWriteDeadline(t time.Time) error { return c.stream.SetWriteDeadline(t) }

// ──────────────────────────────────────────────────────────────────────────────
// Utility Functions
// ──────────────────────────────────────────────────────────────────────────────

// checkPortAvailable verifies that a local port is free to bind.
func checkPortAvailable(port int) error {
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("port %d is already in use", port)
	}
	listener.Close()
	return nil
}
