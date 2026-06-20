package warp

// ──────────────────────────────────────────────────────────────────────────────
// Captive Portal Trace Validator (Pillar 5)
//
// Tests the performance and path integrity of a WARP proxy connection
// before routing production user traffic. Verifies that traffic is actually
// flowing through Cloudflare WARP by checking the /cdn-cgi/trace endpoint.
//
// Key design decisions for Iran (UDP-blocked, high-latency):
//
//   1. Settlement grace period — TCP+TLS+H2+CONNECT auth takes multiple RTTs.
//      We wait before probing so we don't trigger a false rotation.
//
//   2. Consecutive failure counter — don't rotate on the first failed check.
//      Transient congestion on CF's edge is common; require 2 consecutive
//      failures before deciding the endpoint is truly dead.
//
//   3. Fallback connectivity probe — if /cdn-cgi/trace fails (ISP DPI may
//      drop requests containing that path string), try a plain HEAD request
//      to cloudflare.com. If that succeeds the tunnel is up but the trace
//      URL is being filtered; we skip rotation in this case.
// ──────────────────────────────────────────────────────────────────────────────

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"clever-connect/internal/db"
	"clever-connect/internal/db/pebble"
	"clever-connect/internal/logger"
	"clever-connect/internal/models"

	"golang.org/x/net/proxy"
)

const (
	traceURL    = "https://connectivity.cloudflareclient.com/cdn-cgi/trace"
	fallbackURL = "https://cloudflare.com" // plain HEAD — unaffected by trace-path DPI
)

// TraceResult contains the parsed results from a Cloudflare trace check.
type TraceResult struct {
	WarpStatus string `json:"warp_status"` // "on", "plus", "off"
	Gateway    string `json:"gateway"`
	IP         string `json:"ip"`
	Colo       string `json:"colo"`     // Cloudflare datacenter code
	HTTP       string `json:"http"`     // HTTP protocol version
	TLS        string `json:"tls"`      // TLS version
	IsWarpOK   bool   `json:"is_warp_ok"`
}

// TraceCheck performs a captive portal validation by routing traffic through
// the local SOCKS5 proxy and checking the Cloudflare /cdn-cgi/trace response.
//
// Timeouts are generous (15/25s) to accommodate:
//   - masque_h2: TCP connect + uTLS handshake + H2 setup + CONNECT round-trip
//   - wireguard:  WireGuard handshake completion before first packet
func TraceCheck(socksPort int) (*TraceResult, error) {
	socksAddr := fmt.Sprintf("127.0.0.1:%d", socksPort)

	logger.Info("WARP", "Running trace check", "proxy", socksAddr)

	// Create SOCKS5 dialer targeting the local WARP proxy
	dialer, err := proxy.SOCKS5("tcp", socksAddr, nil, &net.Dialer{
		Timeout: 15 * time.Second, // increased: H2 triple-handshake is slower
	})
	if err != nil {
		return nil, fmt.Errorf("warp: failed to create SOCKS5 dialer: %w", err)
	}

	dialFn := func(ctx context.Context, network, addr string) (net.Conn, error) {
		return dialer.Dial(network, addr)
	}

	// Build HTTP client with the SOCKS5 transport
	transport := &http.Transport{
		DialContext:         dialFn,
		TLSHandshakeTimeout: 15 * time.Second, // increased for H2 chains
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   25 * time.Second, // increased: accounts for multi-stage negotiation
	}

	// ── Primary probe: /cdn-cgi/trace ────────────────────────────────────────
	resp, err := client.Get(traceURL)
	if err != nil {
		// ── Fallback probe: plain HEAD to cloudflare.com ──────────────────────
		// Some ISPs use DPI to drop requests containing "/cdn-cgi/trace" in the URL.
		// A successful HEAD to cloudflare.com proves the tunnel is alive even if
		// the trace URL is filtered.
		logger.Warn("WARP", "Primary trace check failed, trying fallback probe", "error", err)
		fbResp, fbErr := client.Head(fallbackURL)
		if fbErr == nil {
			fbResp.Body.Close()
			logger.Info("WARP", "Fallback probe succeeded — tunnel is UP (trace URL may be DPI-filtered)")
			// Return a synthetic result: tunnel is working but we can't read trace fields
			return &TraceResult{
				WarpStatus: "on",
				IsWarpOK:   true,
				Colo:       "DPI-filtered",
			}, nil
		}
		return nil, fmt.Errorf("warp: both trace and fallback probes failed: trace=%v, fallback=%v", err, fbErr)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("warp: trace returned status %d", resp.StatusCode)
	}

	// Parse the trace response (key=value format, one per line)
	result := &TraceResult{}
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		switch key {
		case "warp":
			result.WarpStatus = value
		case "gateway":
			result.Gateway = value
		case "ip":
			result.IP = value
		case "colo":
			result.Colo = value
		case "http":
			result.HTTP = value
		case "tls":
			result.TLS = value
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("warp: failed to parse trace response: %w", err)
	}

	// Validate WARP status
	result.IsWarpOK = result.WarpStatus == "on" || result.WarpStatus == "plus"

	if result.IsWarpOK {
		logger.Info("WARP", "Trace check passed",
			"warp", result.WarpStatus,
			"colo", result.Colo,
			"ip", result.IP,
			"gateway", result.Gateway,
		)
	} else {
		logger.Warn("WARP", "Trace check failed — WARP not active",
			"warp", result.WarpStatus,
			"colo", result.Colo,
			"ip", result.IP,
		)
	}

	return result, nil
}

// AutoRotateOnFailure performs a trace check and, if it fails, marks the
// current endpoint as restricted, pulls the next best endpoint from PebbleDB,
// and restarts the engine with the new endpoint.
//
// The consecutiveFails counter prevents rotating on transient failures.
// Only rotate after maxConsecutiveFails consecutive failures.
func AutoRotateOnFailure(cfg *models.WarpGlobalConfig, consecutiveFails *atomic.Int32) error {
	result, err := TraceCheck(cfg.SocksPort)

	// Update the trace status in the database
	if err != nil || (result != nil && !result.IsWarpOK) {
		fails := consecutiveFails.Add(1)
		const maxConsecutiveFails = 2

		if fails < maxConsecutiveFails {
			logger.Warn("WARP", "Trace check failed (transient?) — waiting for confirmation",
				"consecutiveFails", fails,
				"requiredToRotate", maxConsecutiveFails,
			)
			return nil // Don't rotate yet — wait for next interval
		}

		cfg.LastTraceOK = false
		db.DB.Save(cfg)

		logger.Warn("WARP", "Trace validation failed consecutively, initiating endpoint rotation",
			"consecutiveFails", fails,
		)

		engine := GetEngine()

		// Penalise the current endpoint — increments FailCount which lowers its score.
		// Do NOT call MarkWarpEndpointRestricted here; that flag is set by the scanner
		// and means "UDP/QUIC is blocked" — it has nothing to do with tunnel health.
		if engine.activeEndpoint != nil {
			_ = pebble.IncrementEndpointFailCount(
				cfg.TransportMode,
				engine.activeEndpoint.IPAddress,
				engine.activeEndpoint.Port,
			)
			logger.Info("WARP", "Penalised failed endpoint (FailCount++)",
				"ip", engine.activeEndpoint.IPAddress,
				"port", engine.activeEndpoint.Port,
			)
		}

		// Stop and restart — StartEngine's ranked retry loop will automatically
		// skip the penalised endpoint (lower score) and pick the next best one.
		_ = engine.StopEngine()

		var account models.WarpAccount
		if err := db.DB.First(&account, cfg.ActiveAccountID).Error; err != nil {
			return fmt.Errorf("warp: failed to load active account: %w", err)
		}

		err = engine.StartEngine(cfg, &account)
		if err == nil {
			consecutiveFails.Store(0)
		}
		return err
	}

	// Trace passed — reset consecutive failure counter
	consecutiveFails.Store(0)
	cfg.LastTraceOK = true
	db.DB.Save(cfg)

	return nil
}

// RunPeriodicTraceCheck starts a background goroutine that periodically
// validates the WARP connection and auto-rotates on failure.
//
// settlementDelay: extra wait before the FIRST probe. Use a larger value
// for masque_h2 (TCP+H2 handshake is slow) vs masque (QUIC is faster).
func RunPeriodicTraceCheck(ctx context.Context, cfg *models.WarpGlobalConfig, interval time.Duration) {
	if interval <= 0 {
		interval = 2 * time.Minute
	}

	// Settlement delay before the FIRST probe:
	// masque_h2 needs TCP+TLS+H2+CONNECT to fully establish before we probe.
	// A 30s delay prevents false-positive rotations right after engine start.
	initialDelay := 10 * time.Second
	if cfg.TransportMode == "masque_h2" || cfg.TransportMode == "wireguard" {
		initialDelay = 30 * time.Second
	}

	var consecutiveFails atomic.Int32

	go func() {
		// Wait for the tunnel to settle before first probe
		select {
		case <-ctx.Done():
			return
		case <-time.After(initialDelay):
		}

		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				engine := GetEngine()
				if !engine.IsRunning() {
					continue
				}

				if err := AutoRotateOnFailure(cfg, &consecutiveFails); err != nil {
					logger.Error("WARP", "Periodic trace check auto-rotation failed", "error", err)
				}
			}
		}
	}()
}
