package warp

import (
	"bufio"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"math/rand"
	"net"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"clever-connect/internal/db/pebble"
	"clever-connect/internal/logger"
	"clever-connect/internal/models"

	"github.com/quic-go/quic-go"
)

// ──────────────────────────────────────────────────────────────────────────────
// High-Performance Cloudflare Endpoint Scanner (Pillar 3)
//
// Dual-probe design:
//   • Primary probe:   TCP/TLS handshake — works even when UDP is ISP-blocked
//   • Secondary probe: QUIC/UDP handshake — determines if WARP tunnel can run
//
// If QUIC fails but TCP succeeds, endpoint is saved as "restricted" so the
// user knows their ISP is blocking UDP before they even try to tunnel.
// ──────────────────────────────────────────────────────────────────────────────

// Default Cloudflare WARP CIDR ranges
var defaultWarpCIDRs = []string{
	"162.159.192.0/24",
	"162.159.193.0/24",
	"162.159.195.0/24",
	"162.159.204.0/24",
	"188.114.96.0/24",
	"188.114.97.0/24",
	"188.114.98.0/24",
	"188.114.99.0/24",
}

// Top ports to scan — ordered by most commonly open on CF WARP edges
var defaultWarpPorts = []int{
	443, 2408, 1701, 500, 854, 880, 890, 891, 8319, 8742, 8854, 8886,
	864, 878, 859, 894, 903, 908, 928, 934, 939, 942, 943, 945, 946,
	955, 968, 987, 988, 1002, 1010, 1014, 1018,
}

// ──────────────────────────────────────────────────────────────────────────────
// Types
// ──────────────────────────────────────────────────────────────────────────────

// ScanProgress tracks live progress of a running scan.
type ScanProgress struct {
	IsRunning    bool    `json:"is_running"`
	TotalTargets int     `json:"total_targets"`
	Scanned      int     `json:"scanned"`
	Passed       int     `json:"passed"`
	Failed       int     `json:"failed"`
	Progress     float64 `json:"progress"`
	UDPBlocked   bool    `json:"udp_blocked"` // true when ISP appears to block UDP
}

// ScanEvent is a single probe outcome for real-time log streaming.
// Clients poll GET /api/v2ray/warp/scan/events?since=<last_index>.
type ScanEvent struct {
	Index     int64   `json:"index"`
	Time      string  `json:"time"`
	IP        string  `json:"ip"`
	Port      int     `json:"port"`
	Status    string  `json:"status"`              // "pass" | "tcp_ok" | "fail"
	LatencyMs float64 `json:"latency_ms,omitempty"` // TCP TLS latency
	Note      string  `json:"note,omitempty"`
}

// scanTarget is a single IP:Port to probe.
type scanTarget struct {
	IP   string
	Port int
}

// ──────────────────────────────────────────────────────────────────────────────
// WarpScanner
// ──────────────────────────────────────────────────────────────────────────────

// WarpScanner is a high-concurrency Cloudflare endpoint discovery engine.
type WarpScanner struct {
	cfg       *models.WarpGlobalConfig
	workers   int
	timeoutMs int

	// Runtime state
	mu       sync.Mutex
	cancel   context.CancelFunc
	ctx      context.Context
	running  atomic.Bool
	progress ScanProgress

	// Real-time event stream (ring buffer, last 1000 events)
	eventsMu     sync.Mutex
	events       []ScanEvent
	nextEventIdx atomic.Int64

	// UDP block heuristics
	quicOKCount   atomic.Int64
	quicFailCount atomic.Int64
}

// NewWarpScanner creates a new scanner.
//   workers   — parallel goroutines (0 = runtime.NumCPU() × 4)
//   timeoutMs — per-probe TCP timeout in milliseconds (0 = 2000ms)
func NewWarpScanner(cfg *models.WarpGlobalConfig, workers, timeoutMs int) *WarpScanner {
	if workers <= 0 {
		workers = runtime.NumCPU() * 4
	}
	if workers > 500 {
		workers = 500
	}
	if timeoutMs <= 0 {
		timeoutMs = 2000
	}
	return &WarpScanner{
		cfg:       cfg,
		workers:   workers,
		timeoutMs: timeoutMs,
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// Package-level singleton
// ──────────────────────────────────────────────────────────────────────────────

var (
	globalScannerMu sync.Mutex
	globalScanner   *WarpScanner
)

// GetScanner returns the current global scanner (may be nil if not started).
func GetScanner() *WarpScanner {
	globalScannerMu.Lock()
	defer globalScannerMu.Unlock()
	return globalScanner
}

// SetScanner replaces the global scanner instance.
func SetScanner(s *WarpScanner) {
	globalScannerMu.Lock()
	defer globalScannerMu.Unlock()
	globalScanner = s
}

// ──────────────────────────────────────────────────────────────────────────────
// Public API
// ──────────────────────────────────────────────────────────────────────────────

// StartScan begins scanning; returns immediately. Monitor via GetProgress().
func (s *WarpScanner) StartScan(parentCtx context.Context) error {
	if s.running.Load() {
		return fmt.Errorf("scan already in progress")
	}

	s.mu.Lock()
	s.ctx, s.cancel = context.WithCancel(parentCtx)
	s.running.Store(true)
	s.progress = ScanProgress{IsRunning: true}
	s.mu.Unlock()

	// Reset event buffer and UDP counters
	s.eventsMu.Lock()
	s.events = s.events[:0]
	s.eventsMu.Unlock()
	s.quicOKCount.Store(0)
	s.quicFailCount.Store(0)

	targets := s.generateTargets()
	s.mu.Lock()
	s.progress.TotalTargets = len(targets)
	s.mu.Unlock()

	if len(targets) == 0 {
		s.running.Store(false)
		return fmt.Errorf("no scan targets generated")
	}

	logger.Info("WARP", "Endpoint scan starting",
		"targets", len(targets),
		"workers", s.workers,
		"timeoutMs", s.timeoutMs,
		"mode", s.cfg.TransportMode,
	)

	go s.runScan(targets)
	return nil
}

// StopScan cancels the running scan immediately.
func (s *WarpScanner) StopScan() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cancel != nil {
		s.cancel()
	}
	s.running.Store(false)
	s.progress.IsRunning = false
}

// GetProgress returns a snapshot of current scan metrics.
func (s *WarpScanner) GetProgress() ScanProgress {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.progress.TotalTargets > 0 {
		s.progress.Progress = float64(s.progress.Scanned) / float64(s.progress.TotalTargets) * 100.0
	}
	total := s.quicOKCount.Load() + s.quicFailCount.Load()
	if total >= 10 {
		s.progress.UDPBlocked = float64(s.quicFailCount.Load())/float64(total) > 0.92
	}
	return s.progress
}

// IsRunning reports whether a scan is active.
func (s *WarpScanner) IsRunning() bool { return s.running.Load() }

// GetEvents returns events with index > since (cursor-based polling).
// Returns the events slice and the last index seen (for the next call's since param).
func (s *WarpScanner) GetEvents(since int64) ([]ScanEvent, int64) {
	s.eventsMu.Lock()
	defer s.eventsMu.Unlock()

	var result []ScanEvent
	for _, ev := range s.events {
		if ev.Index > since {
			result = append(result, ev)
			if len(result) >= 100 { // cap single poll at 100 events
				break
			}
		}
	}

	lastIdx := since
	if len(result) > 0 {
		lastIdx = result[len(result)-1].Index
	}
	return result, lastIdx
}

// ──────────────────────────────────────────────────────────────────────────────
// Internal: event buffer
// ──────────────────────────────────────────────────────────────────────────────

func (s *WarpScanner) addEvent(ip string, port int, status string, latencyMs float64, note string) {
	ev := ScanEvent{
		Index:     s.nextEventIdx.Add(1),
		Time:      time.Now().Format("15:04:05.000"),
		IP:        ip,
		Port:      port,
		Status:    status,
		LatencyMs: latencyMs,
		Note:      note,
	}

	s.eventsMu.Lock()
	s.events = append(s.events, ev)
	// Ring buffer — keep last 1000 events
	if len(s.events) > 1000 {
		s.events = s.events[len(s.events)-800:]
	}
	s.eventsMu.Unlock()
}

// ──────────────────────────────────────────────────────────────────────────────
// Internal: scan loop
// ──────────────────────────────────────────────────────────────────────────────

func (s *WarpScanner) generateTargets() []scanTarget {
	var targets []scanTarget

	for _, cidr := range defaultWarpCIDRs {
		ips := expandCIDR(cidr)
		// Sample up to 64 IPs per /24 for speed; a /24 has 254 usable IPs
		if len(ips) > 64 {
			rand.Shuffle(len(ips), func(i, j int) { ips[i], ips[j] = ips[j], ips[i] })
			ips = ips[:64]
		}

		// Use top N ports
		ports := defaultWarpPorts
		if len(ports) > 12 {
			ports = ports[:12]
		}

		for _, ip := range ips {
			for _, port := range ports {
				targets = append(targets, scanTarget{IP: ip, Port: port})
			}
		}
	}

	rand.Shuffle(len(targets), func(i, j int) { targets[i], targets[j] = targets[j], targets[i] })
	return targets
}

func (s *WarpScanner) runScan(targets []scanTarget) {
	defer func() {
		s.running.Store(false)
		s.mu.Lock()
		s.progress.IsRunning = false
		s.mu.Unlock()
		logger.Info("WARP", "Endpoint scan finished",
			"scanned", s.progress.Scanned,
			"passed", s.progress.Passed,
			"failed", s.progress.Failed,
			"quicOK", s.quicOKCount.Load(),
			"quicFail", s.quicFailCount.Load(),
		)
	}()

	sem := make(chan struct{}, s.workers)
	var wg sync.WaitGroup

	for _, target := range targets {
		// Honour cancellation before dispatching each worker
		if s.ctx.Err() != nil {
			break
		}

		wg.Add(1)
		sem <- struct{}{} // acquire

		go func(t scanTarget) {
			defer wg.Done()
			defer func() { <-sem }() // release

			result, err := s.probeEndpoint(s.ctx, t)

			s.mu.Lock()
			s.progress.Scanned++
			if err != nil {
				s.progress.Failed++
				s.mu.Unlock()
				s.addEvent(t.IP, t.Port, "fail", 0, err.Error())
			} else {
				s.progress.Passed++
				s.mu.Unlock()
				_ = pebble.SaveWarpScanResult(s.cfg.TransportMode, result)

				if result.IsRestricted {
					// TCP OK, but QUIC/UDP blocked by ISP
					s.addEvent(t.IP, t.Port, "tcp_ok", result.LatencyMs, "TCP OK — QUIC/UDP blocked by ISP")
				} else {
					// Both TCP and QUIC work
					s.addEvent(t.IP, t.Port, "pass", result.LatencyMs, "TCP+QUIC both OK")
				}
			}
		}(target)
	}

	wg.Wait()
}

// ──────────────────────────────────────────────────────────────────────────────
// Internal: dual probe
// ──────────────────────────────────────────────────────────────────────────────

// probeEndpoint performs a dual-protocol probe:
//
//   Step 1 — TCP/TLS:  Primary connectivity + latency measurement.
//             Works even when the ISP blocks UDP. If this fails, the endpoint
//             is unreachable by any protocol.
//
//   Step 2 — QUIC/UDP: Detects whether the ISP permits UDP to this endpoint.
//             WARP uses QUIC, so a QUIC failure means WARP will not function
//             even if TCP is fine. The result is saved with IsRestricted=true
//             so the UI can warn the user.
func (s *WarpScanner) probeEndpoint(ctx context.Context, t scanTarget) (*pebble.WarpScanResult, error) {
	addr := fmt.Sprintf("%s:%d", t.IP, t.Port)
	timeout := time.Duration(s.timeoutMs) * time.Millisecond

	// ── Step 1: TCP/TLS ──────────────────────────────────────────────────────
	tcpCtx, tcpCancel := context.WithTimeout(ctx, timeout)
	defer tcpCancel()

	t0 := time.Now()
	rawConn, err := (&net.Dialer{}).DialContext(tcpCtx, "tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("TCP connect: %w", err)
	}

	tlsConn := tls.Client(rawConn, &tls.Config{
		ServerName:         s.cfg.TargetSNI,
		InsecureSkipVerify: true,
		NextProtos:         []string{"http/1.1"},
	})
	if err := tlsConn.HandshakeContext(tcpCtx); err != nil {
		rawConn.Close()
		return nil, fmt.Errorf("TLS handshake: %w", err)
	}
	tlsConn.Close()

	tcpLatency := time.Since(t0)

	// Quality gate — generous for users with high-latency ISPs (e.g. Iran → CF)
	if tcpLatency > 2000*time.Millisecond {
		return nil, fmt.Errorf("latency %dms exceeds 2000ms threshold", tcpLatency.Milliseconds())
	}

	// ── Step 2: QUIC/UDP ─────────────────────────────────────────────────────
	quicTimeout := timeout
	if quicTimeout > 2*time.Second {
		quicTimeout = 2 * time.Second
	}
	quicCtx, quicCancel := context.WithTimeout(ctx, quicTimeout)
	defer quicCancel()

	quicOK := false
	quicConn, quicErr := quic.DialAddr(quicCtx, addr, &tls.Config{
		NextProtos:         []string{"h3", "h3-29"},
		ServerName:         s.cfg.TargetSNI,
		InsecureSkipVerify: true,
	}, &quic.Config{
		MaxIdleTimeout: quicTimeout,
	})
	if quicErr == nil {
		quicConn.CloseWithError(0, "scan complete")
		quicOK = true
		s.quicOKCount.Add(1)
	} else {
		s.quicFailCount.Add(1)
	}

	alpns := []string{"http/1.1"}
	if quicOK {
		alpns = []string{"h3", "http/1.1"}
	}

	return &pebble.WarpScanResult{
		IPAddress:             t.IP,
		Port:                  t.Port,
		LatencyMs:             float64(tcpLatency.Milliseconds()),
		PacketLoss:            0,
		ThroughputBytesPerSec: 0,
		SupportedALPNs:        alpns,
		LastScanned:           time.Now().UTC().Format(time.RFC3339Nano),
		IsRestricted:          !quicOK,
	}, nil
}

// ──────────────────────────────────────────────────────────────────────────────
// CIDR expansion utilities
// ──────────────────────────────────────────────────────────────────────────────

func expandCIDR(cidr string) []string {
	ip, ipNet, err := net.ParseCIDR(cidr)
	if err != nil {
		return nil
	}

	var ips []string
	for ip := ip.Mask(ipNet.Mask); ipNet.Contains(ip); incrementIP(ip) {
		ips = append(ips, ip.String())
	}
	if len(ips) > 2 {
		ips = ips[1 : len(ips)-1]
	}
	return ips
}

func incrementIP(ip net.IP) {
	for j := len(ip) - 1; j >= 0; j-- {
		ip[j]++
		if ip[j] > 0 {
			break
		}
	}
}

func loadCIDRsFromReader(r io.Reader) []string {
	var cidrs []string
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		cidrs = append(cidrs, line)
	}
	return cidrs
}
