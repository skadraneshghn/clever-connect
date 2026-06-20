package warp

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"clever-connect/internal/db"
	"clever-connect/internal/db/pebble"
	"clever-connect/internal/logger"
	"clever-connect/internal/models"

	"golang.org/x/net/proxy"
)

// ──────────────────────────────────────────────────────────────────────────────
// Captive Portal Trace Validator (Pillar 5)
//
// Tests the performance and path integrity of a WARP proxy connection
// before routing production user traffic. Verifies that traffic is actually
// flowing through Cloudflare WARP by checking the /cdn-cgi/trace endpoint.
// ──────────────────────────────────────────────────────────────────────────────

const (
	traceURL = "https://connectivity.cloudflareclient.com/cdn-cgi/trace"
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

	// Build HTTP client with the SOCKS5 transport
	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			return dialer.Dial(network, addr)
		},
		TLSHandshakeTimeout: 15 * time.Second, // increased for H2 chains
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   25 * time.Second, // increased: accounts for multi-stage negotiation
	}

	// Make the trace request
	resp, err := client.Get(traceURL)
	if err != nil {
		return nil, fmt.Errorf("warp: trace request failed: %w", err)
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
func AutoRotateOnFailure(cfg *models.WarpGlobalConfig) error {
	result, err := TraceCheck(cfg.SocksPort)

	// Update the trace status in the database
	if err != nil || (result != nil && !result.IsWarpOK) {
		cfg.LastTraceOK = false
		db.DB.Save(cfg)

		logger.Warn("WARP", "Trace validation failed, initiating endpoint rotation")

		engine := GetEngine()

		// Mark current endpoint as restricted
		if engine.activeEndpoint != nil {
			_ = pebble.MarkWarpEndpointRestricted(
				cfg.TransportMode,
				engine.activeEndpoint.IPAddress,
				engine.activeEndpoint.Port,
			)
			logger.Info("WARP", "Marked endpoint as restricted",
				"ip", engine.activeEndpoint.IPAddress,
				"port", engine.activeEndpoint.Port,
			)
		}

		// Get next best endpoint
		nextEndpoint, err := pebble.GetBestWarpEndpoint(cfg.TransportMode)
		if err != nil {
			return fmt.Errorf("warp: no backup endpoints available for rotation: %w", err)
		}

		logger.Info("WARP", "Rotating to next endpoint",
			"ip", nextEndpoint.IPAddress,
			"port", nextEndpoint.Port,
			"latency", fmt.Sprintf("%.0fms", nextEndpoint.LatencyMs),
		)

		// Stop and restart engine with new endpoint
		_ = engine.StopEngine()

		// Fetch the active account
		var account models.WarpAccount
		if err := db.DB.First(&account, cfg.ActiveAccountID).Error; err != nil {
			return fmt.Errorf("warp: failed to load active account: %w", err)
		}

		return engine.StartEngine(cfg, &account)
	}

	// Trace passed
	cfg.LastTraceOK = true
	db.DB.Save(cfg)

	return nil
}

// RunPeriodicTraceCheck starts a background goroutine that periodically
// validates the WARP connection and auto-rotates on failure.
func RunPeriodicTraceCheck(ctx context.Context, cfg *models.WarpGlobalConfig, interval time.Duration) {
	if interval <= 0 {
		interval = 2 * time.Minute
	}

	go func() {
		// Initial check after a short delay
		select {
		case <-ctx.Done():
			return
		case <-time.After(10 * time.Second):
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

				if err := AutoRotateOnFailure(cfg); err != nil {
					logger.Error("WARP", "Periodic trace check auto-rotation failed", "error", err)
				}
			}
		}
	}()
}
