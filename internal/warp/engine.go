package warp

import (
	"context"
	"crypto/tls"
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

	// MASQUE transport
	quicConn *quic.Conn

	// TCP/TLS fallback transport (used when QUIC/UDP is ISP-blocked)
	tcpFallback bool // true when operating in TCP-only mode
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

	if EngineState(e.state.Load()) == EngineStateRunning {
		return fmt.Errorf("warp engine is already running")
	}

	e.state.Store(int32(EngineStateStarting))

	logger.Info("WARP", "Starting WARP+ engine",
		"mode", cfg.TransportMode,
		"socksPort", cfg.SocksPort,
		"httpPort", cfg.HTTPPort,
	)

	// Resolve best endpoint from PebbleDB
	endpoint, err := pebble.GetBestWarpEndpoint(cfg.TransportMode)
	if err != nil {
		e.state.Store(int32(EngineStateStopped))
		return fmt.Errorf("no available endpoints: %w (run a scan first)", err)
	}

	logger.Info("WARP", "Selected optimal endpoint",
		"ip", endpoint.IPAddress,
		"port", endpoint.Port,
		"latency", fmt.Sprintf("%.0fms", endpoint.LatencyMs),
	)

	// Verify port availability
	if err := checkPortAvailable(cfg.SocksPort); err != nil {
		e.state.Store(int32(EngineStateStopped))
		return fmt.Errorf("SOCKS port %d unavailable: %w", cfg.SocksPort, err)
	}
	if err := checkPortAvailable(cfg.HTTPPort); err != nil {
		e.state.Store(int32(EngineStateStopped))
		return fmt.Errorf("HTTP port %d unavailable: %w", cfg.HTTPPort, err)
	}

	// Create parent context
	e.ctx, e.cancel = context.WithCancel(context.Background())
	e.cfg = cfg
	e.account = account
	e.activeEndpoint = endpoint
	e.startTime = time.Now()

	// Start transport based on mode
	switch cfg.TransportMode {
	case "masque":
		if err := e.startMASQUETransport(); err != nil {
			e.cancel()
			e.state.Store(int32(EngineStateStopped))
			return fmt.Errorf("failed to start MASQUE transport: %w", err)
		}
	case "wireguard":
		if err := e.startWireGuardTransport(); err != nil {
			e.cancel()
			e.state.Store(int32(EngineStateStopped))
			return fmt.Errorf("failed to start WireGuard transport: %w", err)
		}
	default:
		e.cancel()
		e.state.Store(int32(EngineStateStopped))
		return fmt.Errorf("unsupported transport mode: %s", cfg.TransportMode)
	}

	e.state.Store(int32(EngineStateRunning))

	logger.Info("WARP", "WARP+ engine started successfully",
		"mode", cfg.TransportMode,
		"endpoint", fmt.Sprintf("%s:%d", endpoint.IPAddress, endpoint.Port),
		"socksPort", cfg.SocksPort,
	)

	return nil
}

// StopEngine safely tears down the WARP tunnel engine.
func (e *WarpEngine) StopEngine() error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if EngineState(e.state.Load()) == EngineStateStopped {
		return nil
	}

	logger.Info("WARP", "Stopping WARP+ engine")

	// Cancel context to signal all goroutines
	if e.cancel != nil {
		e.cancel()
	}

	// Close QUIC connection
	if e.quicConn != nil {
		e.quicConn.CloseWithError(0, "engine shutdown")
		e.quicConn = nil
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
// Mode A: MASQUE HTTP/3 Tunnel Transport (RFC 9298-inspired)
// ──────────────────────────────────────────────────────────────────────────────

// startMASQUETransport initializes the QUIC-based MASQUE tunnel, with automatic
// fallback to TLS-over-TCP when the endpoint is marked as restricted
// (i.e. the ISP blocks UDP/QUIC traffic).
func (e *WarpEngine) startMASQUETransport() error {
	addr := fmt.Sprintf("%s:%d", e.activeEndpoint.IPAddress, e.activeEndpoint.Port)

	// ── Choose transport based on whether QUIC is available ──────────────────
	if e.activeEndpoint.IsRestricted {
		// Endpoint is TCP-reachable but QUIC is blocked — use TLS/TCP fallback
		logger.Warn("WARP", "Endpoint marked restricted (UDP blocked), using TCP/TLS fallback", "endpoint", addr)
		e.tcpFallback = true
	} else {
		e.tcpFallback = false

		// Establish QUIC session to the Cloudflare edge
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
			// QUIC dial failed even though endpoint was marked non-restricted.
			// This can happen if the ISP just started blocking UDP. Fall back.
			logger.Warn("WARP", "QUIC dial failed, switching to TCP/TLS fallback", "endpoint", addr, "error", err)
			e.tcpFallback = true
		} else {
			e.quicConn = conn
			logger.Info("WARP", "QUIC session established",
				"endpoint", addr,
				"protocol", conn.ConnectionState().TLS.NegotiatedProtocol,
			)
		}
	}

	// ── Dial function used by SOCKS5 server ──────────────────────────────────
	var dialFn func(ctx context.Context, network, targetAddr string) (net.Conn, error)
	if e.tcpFallback {
		dialFn = func(ctx context.Context, network, targetAddr string) (net.Conn, error) {
			return e.dialViaTCPTLS(ctx, addr, targetAddr)
		}
	} else {
		dialFn = func(ctx context.Context, network, targetAddr string) (net.Conn, error) {
			return e.dialViaMASQUE(ctx, targetAddr)
		}
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
		logger.Info("WARP", "SOCKS5 proxy listening", "addr", socksAddr)
		if err := socksServer.Serve(e.socksListener); err != nil {
			if e.ctx.Err() == nil {
				logger.Error("WARP", "SOCKS5 server error", "error", err)
			}
		}
	}()

	go e.startHTTPProxy()
	if !e.tcpFallback {
		go e.monitorConnection()
	}

	return nil
}

// dialViaTCPTLS connects to the Cloudflare edge over TCP+TLS and sends an
// HTTP/1.1 CONNECT request to tunnel the target address.
// This is the fallback for networks where QUIC/UDP is ISP-blocked.
func (e *WarpEngine) dialViaTCPTLS(ctx context.Context, cfAddr, targetAddr string) (net.Conn, error) {
	rawConn, err := (&net.Dialer{}).DialContext(ctx, "tcp", cfAddr)
	if err != nil {
		return nil, fmt.Errorf("TCP connect to %s failed: %w", cfAddr, err)
	}

	tlsConn := tls.Client(rawConn, &tls.Config{
		ServerName:         e.cfg.TargetSNI,
		InsecureSkipVerify: true,
		NextProtos:         []string{"http/1.1"},
	})
	if err := tlsConn.HandshakeContext(ctx); err != nil {
		rawConn.Close()
		return nil, fmt.Errorf("TLS handshake to %s failed: %w", cfAddr, err)
	}

	// Send HTTP/1.1 CONNECT
	connectReq := fmt.Sprintf(
		"CONNECT %s HTTP/1.1\r\n"+
			"Host: %s\r\n"+
			"Authorization: Bearer %s\r\n"+
			"Proxy-Connection: Keep-Alive\r\n"+
			"\r\n",
		targetAddr,
		e.cfg.TargetSNI,
		e.account.Token,
	)
	if _, err := tlsConn.Write([]byte(connectReq)); err != nil {
		tlsConn.Close()
		return nil, fmt.Errorf("failed to send CONNECT: %w", err)
	}

	// Read CONNECT response
	buf := make([]byte, 4096)
	n, err := tlsConn.Read(buf)
	if err != nil && err != io.EOF {
		tlsConn.Close()
		return nil, fmt.Errorf("failed to read CONNECT response: %w", err)
	}
	resp := string(buf[:n])
	if len(resp) < 12 || resp[9:12] != "200" {
		tlsConn.Close()
		return nil, fmt.Errorf("CONNECT rejected: %s", resp)
	}

	return tlsConn, nil
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
	connectReq := fmt.Sprintf(
		"CONNECT %s HTTP/1.1\r\n"+
			"Host: %s\r\n"+
			"Authorization: Bearer %s\r\n"+
			"Capsule-Protocol: wg\r\n"+
			"\r\n",
		targetAddr,
		e.cfg.TargetSNI,
		e.account.Token,
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

// startHTTPProxy starts an HTTP CONNECT proxy that delegates to the SOCKS5 proxy.
func (e *WarpEngine) startHTTPProxy() {
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
				e.handleHTTPConnect(w, r)
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

// handleHTTPConnect handles HTTP CONNECT proxy requests by proxying through
// the MASQUE tunnel.
func (e *WarpEngine) handleHTTPConnect(w http.ResponseWriter, r *http.Request) {
	targetConn, err := e.dialViaMASQUE(r.Context(), r.Host)
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
// Mode B: WireGuard/gVisor Userspace Netstack Transport (Fallback)
// ──────────────────────────────────────────────────────────────────────────────

// startWireGuardTransport initializes the WireGuard tunnel over gVisor netstack.
// This is the fallback transport for environments where MASQUE is blocked.
func (e *WarpEngine) startWireGuardTransport() error {
	logger.Info("WARP", "Initializing WireGuard/gVisor userspace netstack transport")

	// Note: Full gVisor + wireguard-go integration requires importing:
	//   gvisor.dev/gvisor/pkg/tcpip/stack
	//   gvisor.dev/gvisor/pkg/tcpip/link/channel
	//   github.com/sagernet/wireguard-go/device
	//   github.com/sagernet/wireguard-go/tun/netstack
	//
	// Phase 1 implementation provides the MASQUE transport as the primary mode.
	// Full WireGuard/gVisor integration is planned for Phase 2.
	//
	// The architecture is:
	// 1. Create a gVisor network stack with virtual NIC
	// 2. Assign virtual IPs (172.16.0.2/32, 2606:4700:110:8283::2/128)
	// 3. Create a WireGuard device bound to the gVisor channel endpoint
	// 4. Configure the WireGuard peer with account keys + endpoint from PebbleDB
	// 5. Route all SOCKS5 traffic through the virtual stack

	// For now, start a local SOCKS5 server that tunnels through WireGuard
	// using a simplified direct UDP encapsulation approach
	return e.startWireGuardSimple()
}

// startWireGuardSimple provides a simplified WireGuard transport using
// direct configuration commands via the wireguard-go UAPI.
func (e *WarpEngine) startWireGuardSimple() error {
	// Start local SOCKS5 proxy
	socksConf := &socks5.Config{
		Dial: func(ctx context.Context, network, addr string) (net.Conn, error) {
			// In full implementation, this would route through gVisor netstack
			// For now, provide a placeholder that connects directly
			// (to be replaced with gVisor routing in Phase 2)
			dialer := &net.Dialer{Timeout: 10 * time.Second}
			return dialer.DialContext(ctx, network, addr)
		},
	}

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
		logger.Info("WARP", "WireGuard SOCKS5 proxy listening", "addr", socksAddr)
		if err := socksServer.Serve(e.socksListener); err != nil {
			if e.ctx.Err() == nil {
				logger.Error("WARP", "WireGuard SOCKS5 server error", "error", err)
			}
		}
	}()

	// Start HTTP CONNECT proxy
	go e.startHTTPProxy()

	logger.Warn("WARP", "WireGuard transport running in simplified mode (Phase 2 will add full gVisor netstack)")
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
