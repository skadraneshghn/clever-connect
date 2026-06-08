package scanner

import (
	"context"
	crand "crypto/rand"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"clever-connect/internal/config"
	"clever-connect/internal/db"
	"clever-connect/internal/db/pebble"
	"clever-connect/internal/models"
	"clever-connect/internal/v2ray/compiler"
	"clever-connect/internal/v2ray/core"
	"clever-connect/internal/v2ray/traffic"

	rawpebble "github.com/cockroachdb/pebble"

	"github.com/gin-gonic/gin"
	box "github.com/sagernet/sing-box"
	"github.com/sagernet/sing-box/include"
	boxOption "github.com/sagernet/sing-box/option"
	"crypto/tls"
	"golang.org/x/time/rate"
)

// DefaultCloudflareRanges represents default Cloudflare edge IP blocks
var DefaultCloudflareRanges = []string{
	"173.245.48.0/20", "103.21.244.0/22", "103.22.200.0/22",
	"103.31.4.0/22", "141.101.64.0/18", "108.162.192.0/18",
	"190.93.240.0/20", "188.114.96.0/20", "197.234.240.0/22",
	"198.41.128.0/17", "162.158.0.0/15", "104.16.0.0/13",
	"104.24.0.0/14", "172.64.0.0/13", "131.0.72.0/22",
}

var cfIPNets []*net.IPNet

func init() {
	for _, cidr := range DefaultCloudflareRanges {
		_, ipNet, err := net.ParseCIDR(cidr)
		if err == nil {
			cfIPNets = append(cfIPNets, ipNet)
		}
	}
}

// CDNConfigRow represents a tested IP with diagnostics info

// PortProbeResult holds the outcome of a single port probe
type PortProbeResult struct {
	IP       string `json:"ip"`
	Port     int    `json:"port"`
	Protocol string `json:"protocol"` // "tcp" or "udp"
	Open     bool   `json:"open"`
	Latency  int64  `json:"latency_ms,omitempty"`
	Error    string `json:"error,omitempty"`
}

// ProbePort tests if a TCP or UDP port is open
func ProbePort(ip string, port int, protocol string, timeout time.Duration) PortProbeResult {
	res := PortProbeResult{IP: ip, Port: port, Protocol: protocol}
	t0 := time.Now()

	if protocol == "udp" {
		addr := net.JoinHostPort(ip, strconv.Itoa(port))
		conn, err := net.DialTimeout("udp", addr, timeout)
		if err != nil {
			res.Open = false
			res.Error = err.Error()
			return res
		}
		defer conn.Close()
		_ = conn.SetDeadline(time.Now().Add(timeout))
		_, err = conn.Write([]byte{0x00})
		if err != nil {
			res.Open = false
			res.Error = err.Error()
			return res
		}
		res.Open = true
		res.Latency = time.Since(t0).Milliseconds()
		return res
	}

	// TCP
	addr := net.JoinHostPort(ip, strconv.Itoa(port))
	conn, err := net.DialTimeout("tcp", addr, timeout)
	if err != nil {
		res.Open = false
		res.Error = err.Error()
		return res
	}
	conn.Close()
	res.Open = true
	res.Latency = time.Since(t0).Milliseconds()
	return res
}

// ProbePorts runs concurrent probes for multiple ports
func ProbePorts(ip string, ports []int, protocol string, timeout time.Duration) []PortProbeResult {
	results := make([]PortProbeResult, len(ports))
	var wg sync.WaitGroup
	sem := make(chan struct{}, 10) // Concurrency limit of 10

	for i, port := range ports {
		wg.Add(1)
		sem <- struct{}{}
		go func(idx int, p int) {
			defer wg.Done()
			defer func() { <-sem }()
			results[idx] = ProbePort(ip, p, protocol, timeout)
		}(i, port)
	}

	wg.Wait()
	return results
}

// SendWakeOnLAN sends a magic packet to the target MAC address via UDP broadcast
func SendWakeOnLAN(macStr string, broadcastIP string) error {
	mac, err := net.ParseMAC(macStr)
	if err != nil {
		return fmt.Errorf("invalid MAC address: %w", err)
	}

	if len(mac) != 6 {
		return fmt.Errorf("MAC address must be 6 bytes")
	}

	// Magic packet payload
	packet := make([]byte, 6+16*6)
	for i := 0; i < 6; i++ {
		packet[i] = 0xFF
	}
	for i := 1; i <= 16; i++ {
		copy(packet[i*6:(i+1)*6], mac)
	}

	if broadcastIP == "" {
		broadcastIP = "255.255.255.255"
	}

	conn, err := net.Dial("udp", net.JoinHostPort(broadcastIP, "9"))
	if err != nil {
		return fmt.Errorf("failed to dial UDP broadcast: %w", err)
	}
	defer conn.Close()

	_, err = conn.Write(packet)
	if err != nil {
		return fmt.Errorf("failed to send magic packet: %w", err)
	}
	return nil
}

// DiscoveredDevice represents a device found on the local network
type DiscoveredDevice struct {
	IP       string `json:"ip"`
	Hostname string `json:"hostname,omitempty"`
	PingMs   int64  `json:"ping_ms"`
	Active   bool   `json:"active"`
}

// DiscoverDevices scans the local network subnet of the first active non-loopback network interface
func DiscoverDevices(timeout time.Duration) ([]DiscoveredDevice, error) {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return nil, err
	}

	var localIP net.IP
	var localSubnet *net.IPNet
	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				localIP = ipnet.IP
				localSubnet = ipnet
				break
			}
		}
	}

	if localIP == nil || localSubnet == nil {
		return nil, fmt.Errorf("no active IPv4 interface found")
	}

	ipBase := localIP.Mask(localSubnet.Mask)
	var ipsToScan []string
	for i := 1; i < 255; i++ {
		ip := make(net.IP, len(ipBase))
		copy(ip, ipBase)
		ip[3] = byte(i)
		ipsToScan = append(ipsToScan, ip.String())
	}

	var wg sync.WaitGroup
	var mu sync.Mutex
	var discovered []DiscoveredDevice
	sem := make(chan struct{}, 30)

	for _, ipStr := range ipsToScan {
		wg.Add(1)
		sem <- struct{}{}
		go func(ip string) {
			defer wg.Done()
			defer func() { <-sem }()

			t0 := time.Now()
			conn, err := net.DialTimeout("tcp", net.JoinHostPort(ip, "80"), 150*time.Millisecond)
			active := false
			if err == nil {
				active = true
				conn.Close()
			} else {
				conn, err = net.DialTimeout("tcp", net.JoinHostPort(ip, "22"), 100*time.Millisecond)
				if err == nil {
					active = true
					conn.Close()
				}
			}

			if active {
				hostname := ""
				names, err := net.LookupAddr(ip)
				if err == nil && len(names) > 0 {
					hostname = strings.TrimSuffix(names[0], ".")
				}
				mu.Lock()
				discovered = append(discovered, DiscoveredDevice{
					IP:       ip,
					Hostname: hostname,
					PingMs:   time.Since(t0).Milliseconds(),
					Active:   true,
				})
				mu.Unlock()
			}
		}(ipStr)
	}

	wg.Wait()
	return discovered, nil
}

// ──────────────────────────────────────────────────────────────────────────────
// V2Ray Network Scanner Sweep (New Scanner Engine)
// ──────────────────────────────────────────────────────────────────────────────

// JobStats represents the live stats of the network scanner sweep
type JobStats struct {
	Tested       int64  `json:"tested"`
	Healthy      int64  `json:"healthy"`
	Failed       int64  `json:"failed"`
	InFlight     int64  `json:"in_flight"`
	TotalTargets int64  `json:"total_targets"`
	RemainingSec int64  `json:"remaining_sec"`
	Phase        string `json:"phase"`
}

// ScanConfig defines the operational bounds for a live network verification sweep
type ScanConfig struct {
	TargetCIDRs        []string      `json:"target_cidrs"`
	SelectedPorts      []int         `json:"selected_ports"`
	ConcurrencyLimit   int           `json:"concurrency_limit"`
	MaxRateLimit       float64       `json:"max_rate_limit"`
	NetworkTimeout     time.Duration `json:"network_timeout"`
	ProbeAttempts      int           `json:"probe_attempts"`
	TargetMode         string        `json:"target_mode"`
	TargetSNI          string        `json:"target_sni"`
	WebSocketHost      string        `json:"websocket_host"`
	WebSocketPath      string        `json:"websocket_path"`
	RequireWS          bool          `json:"require_ws"`
	EnableNeighbors    bool          `json:"enable_neighbors"`
	TopLimit           int           `json:"top_limit"`
	TotalTargetCount   int           `json:"total_target_count"`
	ScanDiscoveredOnly bool          `json:"scan_discovered_only"`
}

type ScannerListener func(stats JobStats, event string, details interface{})

// ScannerEngine orchestrates the live network verification sweep
type ScannerEngine struct {
	mu         sync.RWMutex
	isRunning  bool
	stats      JobStats
	cancelFunc context.CancelFunc
	listeners  map[string]ScannerListener
}

var (
	engineOnce   sync.Once
	globalEngine *ScannerEngine
)

// GetEngine returns the singleton ScannerEngine instance
func GetEngine() *ScannerEngine {
	engineOnce.Do(func() {
		globalEngine = &ScannerEngine{
			listeners: make(map[string]ScannerListener),
		}
	})
	return globalEngine
}

func (s *ScannerEngine) RegisterListener(id string, l ScannerListener) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.listeners == nil {
		s.listeners = make(map[string]ScannerListener)
	}
	s.listeners[id] = l
}

func (s *ScannerEngine) UnregisterListener(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.listeners != nil {
		delete(s.listeners, id)
	}
}

func (s *ScannerEngine) broadcast(event string, details interface{}) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	stats := s.GetLiveStats()
	for _, l := range s.listeners {
		l(stats, event, details)
	}
}

// IsRunning returns whether the scanner engine is currently active
func (s *ScannerEngine) IsRunning() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.isRunning
}

// GetLiveStats returns a copy of the current statistics
func (s *ScannerEngine) GetLiveStats() JobStats {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return JobStats{
		Tested:       atomic.LoadInt64(&s.stats.Tested),
		Healthy:      atomic.LoadInt64(&s.stats.Healthy),
		Failed:       atomic.LoadInt64(&s.stats.Failed),
		InFlight:     atomic.LoadInt64(&s.stats.InFlight),
		TotalTargets: atomic.LoadInt64(&s.stats.TotalTargets),
		RemainingSec: atomic.LoadInt64(&s.stats.RemainingSec),
		Phase:        s.stats.Phase,
	}
}

const lockFilePath = "scanner.lock"

func acquireLock() bool {
	f, err := os.OpenFile(lockFilePath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return false
	}
	f.Close()
	return true
}

func releaseLock() {
	_ = os.Remove(lockFilePath)
}

// CancelActiveScan cancels the running scanner engine sweep
func (s *ScannerEngine) CancelActiveScan() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cancelFunc != nil {
		s.cancelFunc()
		s.cancelFunc = nil
	}
	s.isRunning = false
}

// StartScan triggers the network scan sweep in a background goroutine
func (s *ScannerEngine) StartScan(parentCtx context.Context, cfg *ScanConfig, isRetry bool) error {
	s.mu.Lock()
	if s.isRunning {
		s.mu.Unlock()
		return fmt.Errorf("a scan sweep is already running")
	}

	if isRetry {
		cachedCfg, err := s.LoadLastSettings()
		if err == nil && cachedCfg != nil {
			cfg = cachedCfg
		} else {
			s.mu.Unlock()
			return fmt.Errorf("failed to load historical scan settings from cache keys: %v", err)
		}
	} else {
		s.SaveLastSettings(cfg)
	}

	if !acquireLock() {
		s.mu.Unlock()
		return fmt.Errorf("a scan sweep is already running (lock acquired by another process)")
	}

	ctx, cancel := context.WithCancel(parentCtx)
	s.cancelFunc = cancel
	s.isRunning = true
	s.stats = JobStats{}
	s.mu.Unlock()

	go func() {
		defer func() {
			s.mu.Lock()
			s.isRunning = false
			s.cancelFunc = nil
			s.mu.Unlock()
			releaseLock()
			cancel()
		}()
		s.runScanLoop(ctx, cfg)
	}()

	return nil
}

type configProbeJob struct {
	ip   net.IP
	port int
}

var defaultEdgeSNIs = []string{
	"speed.cloudflare.com",
	"www.cloudflare.com",
	"cloudflare.com",
	"1.1.1.1.cdn.cloudflare.net",
	"blog.cloudflare.com",
}

func shuffleStrings(slice []string) {
	for i := len(slice) - 1; i > 0; i-- {
		nBig, err := crand.Int(crand.Reader, big.NewInt(int64(i+1)))
		if err != nil {
			continue
		}
		j := int(nBig.Int64())
		slice[i], slice[j] = slice[j], slice[i]
	}
}

func (s *ScannerEngine) runScanLoop(ctx context.Context, cfg *ScanConfig) {
	s.broadcast("scanner.log", fmt.Sprintf("Initiating scanner engine sweep. Target Mode: %s | Probe Ports: %v", cfg.TargetMode, cfg.SelectedPorts))
	s.broadcast("scanner.log", fmt.Sprintf("Parameters: Concurrency=%d | RateLimit=%.1f/s | Timeout=%v | Neighbors=%v", cfg.ConcurrencyLimit, cfg.MaxRateLimit, cfg.NetworkTimeout, cfg.EnableNeighbors))

	// Fetch base template client configuration from DB
	baseConfig, baseErr := getBaseClientConfig()
	if baseErr != nil {
		s.broadcast("scanner.error", baseErr.Error())
		s.broadcast("scanner.log", fmt.Sprintf("Warning: Failed to fetch base client template configuration: %v. Throughput test will be skipped.", baseErr))
	} else {
		s.broadcast("scanner.log", fmt.Sprintf("Loaded base client template config: '%s' (Protocol: %s)", baseConfig.Name, baseConfig.Protocol))
	}

	var cidrs []string
	var directIPs []string
	var err error

	if cfg.ScanDiscoveredOnly {
		configs, _ := pebble.ListClientConfigs(pebble.ConfigFilter{}, 0, 0)
		for _, c := range configs {
			if strings.HasPrefix(c.Name, "Discovered-") && c.LatencyMs > 0 {
				directIPs = append(directIPs, fmt.Sprintf("%s:%d", c.Address, c.Port))
			}
		}
		if len(directIPs) == 0 {
			s.broadcast("scanner.log", "No previously discovered nodes found in Pebble DB to rescan.")
			s.broadcast("scanner.finished", gin.H{
				"stats": s.GetLiveStats(),
				"event": "scanner.finished",
			})
			return
		}
		s.broadcast("scanner.log", fmt.Sprintf("Found %d saved discovered nodes to rescan.", len(directIPs)))
	} else {
		s.mu.Lock()
		s.stats.Phase = "Ingestion & Cache-DNS Lookup"
		s.mu.Unlock()
		s.broadcast("scanner.phase", s.stats.Phase)

		s.broadcast("scanner.log", "Ingesting scan sources...")
		cidrs, directIPs, err = FetchEnabledSourcesConcurrently(ctx)
		if err != nil {
			s.broadcast("scanner.log", fmt.Sprintf("Warning: Source ingestion failed: %v. Using defaults.", err))
			cidrs = cfg.TargetCIDRs
			if len(cidrs) == 0 {
				cidrs = DefaultCloudflareRanges
			}
		} else {
			s.broadcast("scanner.log", fmt.Sprintf("Ingested %d subnets and %d direct endpoints.", len(cidrs), len(directIPs)))
		}
	}

	// Calculate total targets
	var totalTargets int64 = int64(len(directIPs))
	if !cfg.ScanDiscoveredOnly {
		for _, c := range cidrs {
			if start, end, err := parseCIDR(c); err == nil && end >= start {
				totalTargets += int64(end-start+1) * int64(len(cfg.SelectedPorts))
			}
		}
	}
	if cfg.TotalTargetCount > 0 && totalTargets > int64(cfg.TotalTargetCount) {
		totalTargets = int64(cfg.TotalTargetCount)
	}
	atomic.StoreInt64(&s.stats.TotalTargets, totalTargets)

	// Concurrency settings
	phase1Concurrency := cfg.ConcurrencyLimit
	if phase1Concurrency <= 0 {
		phase1Concurrency = 500
	}
	phase2Concurrency := 100

	phase1Chan := make(chan configProbeJob, phase1Concurrency*2)
	phase2Chan := make(chan configProbeJob, 1000)

	// Start generator
	go func() {
		defer close(phase1Chan)
		if cfg.ScanDiscoveredOnly {
			for _, addr := range directIPs {
				host, portStr, err := net.SplitHostPort(addr)
				if err != nil {
					continue
				}
				port, _ := strconv.Atoi(portStr)
				ip := net.ParseIP(host)
				if ip == nil {
					continue
				}
				select {
				case <-ctx.Done():
					return
				case phase1Chan <- configProbeJob{ip: ip, port: port}:
				}
			}
		} else {
			// Submit direct IPs (from domains/proxy IPs)
			for _, ipStr := range directIPs {
				ip := net.ParseIP(ipStr)
				if ip == nil {
					continue
				}
				for _, port := range cfg.SelectedPorts {
					select {
					case <-ctx.Done():
						return
					case phase1Chan <- configProbeJob{ip: ip, port: port}:
					}
				}
			}
			// Submit streamed IPs from CIDRs
			streamChan := StreamRandomIPs(ctx, cidrs, cfg.TotalTargetCount)
			for ipStr := range streamChan {
				ip := net.ParseIP(ipStr)
				if ip == nil {
					continue
				}
				for _, port := range cfg.SelectedPorts {
					select {
					case <-ctx.Done():
						return
					case phase1Chan <- configProbeJob{ip: ip, port: port}:
					}
				}
			}
		}
	}()

	var limiter *rate.Limiter
	if cfg.MaxRateLimit > 0 {
		limiter = rate.NewLimiter(rate.Limit(cfg.MaxRateLimit), int(cfg.MaxRateLimit)+1)
	}

	// Start live telemetry/ETA reporter background goroutine
	startTime := time.Now()
	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				tested := atomic.LoadInt64(&s.stats.Tested)
				if tested > 0 {
					elapsed := time.Since(startTime).Seconds()
					avgSec := elapsed / float64(tested)
					remaining := totalTargets - tested
					if remaining > 0 {
						atomic.StoreInt64(&s.stats.RemainingSec, int64(float64(remaining)*avgSec))
					} else {
						atomic.StoreInt64(&s.stats.RemainingSec, 0)
					}
				}
				s.broadcast("scanner.telemetry", s.GetLiveStats())
			}
		}
	}()

	// Phase 1: High-Speed TCP Sweep
	s.mu.Lock()
	s.stats.Phase = "Phase 1: High-Speed Port Sweep"
	s.mu.Unlock()
	s.broadcast("scanner.phase", s.stats.Phase)

	var phase1WG sync.WaitGroup
	for i := 0; i < phase1Concurrency; i++ {
		phase1WG.Add(1)
		go func() {
			defer phase1WG.Done()
			for job := range phase1Chan {
				if ctx.Err() != nil {
					return
				}
				if limiter != nil {
					_ = limiter.Wait(ctx)
				}

				// Introduce microsecond jitter if in server mode
				if appCfg := config.LoadConfig(); appCfg != nil && appCfg.AppMode == "server" {
					jitter := time.Duration(cryptoRandIntn(200)+50) * time.Microsecond
					time.Sleep(jitter)
				}

				atomic.AddInt64(&s.stats.InFlight, 1)
				_, err := probeTCP(ctx, job.ip, job.port, 2*time.Second)
				atomic.AddInt64(&s.stats.InFlight, -1)

				if err == nil {
					select {
					case <-ctx.Done():
						return
					case phase2Chan <- job:
					}
				} else {
					atomic.AddInt64(&s.stats.Tested, 1)
					atomic.AddInt64(&s.stats.Failed, 1)
				}
			}
		}()
	}

	go func() {
		phase1WG.Wait()
		close(phase2Chan)
	}()

	// Phase 2: Cryptographic Deep Test
	s.mu.Lock()
	s.stats.Phase = "Phase 2: Cryptographic Deep Test"
	s.mu.Unlock()
	s.broadcast("scanner.phase", s.stats.Phase)

	var phase2WG sync.WaitGroup
	discoveredConfigs := make([]models.V2RayClientConfig, 0)
	var discoveredMu sync.Mutex

	for i := 0; i < phase2Concurrency; i++ {
		phase2WG.Add(1)
		go func() {
			defer phase2WG.Done()
			for job := range phase2Chan {
				if ctx.Err() != nil {
					return
				}

				atomic.AddInt64(&s.stats.InFlight, 1)

				sni := cfg.TargetSNI
				if sni == "" {
					sni = selectRandomSNI(defaultEdgeSNIs)
				}

				// 1. 3-Strike TLS handshake verification
				tlsLatency, tlsErr := probeTLS3Strike(ctx, job.ip, job.port, sni, cfg.NetworkTimeout)
				ok := tlsErr == nil
				var finalLatency time.Duration = tlsLatency

				// 2. HTTP 1-byte read test to bypass TCP blackholes
				if ok {
					httpLatency, httpErr := probeHTTP1Byte(ctx, job.ip, job.port, sni, cfg.NetworkTimeout, cfg.WebSocketPath)
					if httpErr != nil {
						ok = false
					} else {
						finalLatency = httpLatency
					}
				}

				atomic.AddInt64(&s.stats.InFlight, -1)
				atomic.AddInt64(&s.stats.Tested, 1)

				latencyMs := int(finalLatency.Milliseconds())
				var speed float64

				if ok {
					if baseConfig != nil {
						l, sp, errProxy := testProxyThroughput(ctx, *baseConfig, job.ip.String(), job.port)
						if errProxy == nil {
							latencyMs = l
							speed = sp
						} else {
							s.broadcast("scanner.log", fmt.Sprintf("Proxy validation failed for %s:%d: %v. Raw latency used.", job.ip.String(), job.port, errProxy))
						}
					}

					atomic.AddInt64(&s.stats.Healthy, 1)

					if baseConfig != nil {
						newCfg := *baseConfig
						newCfg.ID = 0
						newCfg.Address = job.ip.String()
						newCfg.Port = job.port
						newCfg.LatencyMs = latencyMs
						newCfg.Name = fmt.Sprintf("Discovered-Edge-%s:%d", job.ip.String(), job.port)
						newCfg.IsActive = false
						newCfg.Priority = 100

						if err := pebble.SaveClientConfig(&newCfg); err != nil {
							s.broadcast("scanner.log", fmt.Sprintf("Failed to save clean node %s:%d: %v", job.ip.String(), job.port, err))
						} else {
							s.broadcast("scanner.log", fmt.Sprintf("Auto-provisioned clean node: %s:%d (Latency: %d ms | Throughput: %.2f Mbps)", job.ip.String(), job.port, latencyMs, speed))
							
							// Trigger Hot-Reload of Core
							appCfg := config.LoadConfig()
							if appCfg != nil {
								if appCfg.AppMode == "server" {
									_ = traffic.ReloadCoreConfig()
								} else if appCfg.AppMode == "client" {
									_ = reloadClientCore()
								}
							}
						}

						discoveredMu.Lock()
						discoveredConfigs = append(discoveredConfigs, newCfg)
						discoveredMu.Unlock()
					}

					s.broadcast("scanner.candidate", gin.H{
						"stats": s.GetLiveStats(),
						"event": "scanner.candidate",
						"data": gin.H{
							"ip":         job.ip.String(),
							"port":       job.port,
							"protocol":   cfg.TargetMode,
							"latency_ms": latencyMs,
							"speed_mbps": speed,
						},
					})
				} else {
					atomic.AddInt64(&s.stats.Failed, 1)
					s.broadcast("scanner.candidate", gin.H{
						"stats": s.GetLiveStats(),
						"event": "scanner.candidate",
						"data": gin.H{
							"ip":         job.ip.String(),
							"port":       job.port,
							"protocol":   cfg.TargetMode,
							"latency_ms": 0,
							"speed_mbps": 0.0,
						},
					})
				}
			}
		}()
	}

	phase2WG.Wait()

	// Rescan cleanup mode: delete failed saved discovered nodes
	if cfg.ScanDiscoveredOnly && len(directIPs) > 0 {
		configs, _ := pebble.ListClientConfigs(pebble.ConfigFilter{}, 0, 0)
		verifiedMap := make(map[string]bool)
		discoveredMu.Lock()
		for _, c := range discoveredConfigs {
			verifiedMap[fmt.Sprintf("%s:%d", c.Address, c.Port)] = true
		}
		discoveredMu.Unlock()

		var toDelete []uint
		for _, c := range configs {
			if strings.HasPrefix(c.Name, "Discovered-") {
				key := fmt.Sprintf("%s:%d", c.Address, c.Port)
				wasInTargets := false
				for _, t := range directIPs {
					if t == key {
						wasInTargets = true
						break
					}
				}
				if wasInTargets && !verifiedMap[key] {
					toDelete = append(toDelete, c.ID)
				}
			}
		}
		if len(toDelete) > 0 {
			s.broadcast("scanner.log", fmt.Sprintf("Cleanup: removing %d dead/failed discovered nodes from Pebble DB...", len(toDelete)))
			for _, id := range toDelete {
				_ = pebble.DeleteClientConfig(id)
			}
			s.broadcast("scanner.log", "Cleanup complete.")
		}
	}

	exportVerifiedIPs()

	finalStats := s.GetLiveStats()
	s.broadcast("scanner.log", fmt.Sprintf("Scanner engine sweep completed. Tested: %d | Healthy: %d | Failed: %d", finalStats.Tested, finalStats.Healthy, finalStats.Failed))
	s.broadcast("scanner.finished", gin.H{
		"stats": finalStats,
		"event": "scanner.finished",
	})
}

// SaveLastSettings persists current scanner targets inside Pebble storage keys
func (s *ScannerEngine) SaveLastSettings(cfg *ScanConfig) {
	bytes, err := json.Marshal(cfg)
	if err == nil && pebble.DB != nil {
		_ = pebble.DB.Set([]byte("cache:last_scan_config"), bytes, rawpebble.Sync)
	}
}

// LoadLastSettings retrieves previous parameter vectors from cache keys
func (s *ScannerEngine) LoadLastSettings() (*ScanConfig, error) {
	if pebble.DB == nil {
		return nil, fmt.Errorf("pebble engine offline")
	}
	bytes, closer, err := pebble.DB.Get([]byte("cache:last_scan_config"))
	if err != nil {
		return nil, err
	}
	defer closer.Close()
	var cfg ScanConfig
	err = json.Unmarshal(bytes, &cfg)
	return &cfg, err
}

func cryptoRandIntn(n int) int {
	if n <= 0 {
		return 0
	}
	bigN := big.NewInt(int64(n))
	val, err := crand.Int(crand.Reader, bigN)
	if err != nil {
		return 0
	}
	return int(val.Int64())
}

func selectRandomSNI(defaultSNIs []string) string {
	if len(defaultSNIs) == 0 {
		return ""
	}
	return defaultSNIs[cryptoRandIntn(len(defaultSNIs))]
}

func probeTCP(ctx context.Context, ip net.IP, port int, timeout time.Duration) (time.Duration, error) {
	addr := net.JoinHostPort(ip.String(), strconv.Itoa(port))
	d := net.Dialer{Timeout: timeout / 4} // 1/4 TCP handshake split
	start := time.Now()
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return 0, err
	}
	defer conn.Close()
	return time.Since(start), nil
}

func probeTLS(ctx context.Context, ip net.IP, port int, sni string, timeout time.Duration) (time.Duration, error) {
	addr := net.JoinHostPort(ip.String(), strconv.Itoa(port))
	dl := time.Now().Add(timeout)
	dialCtx, cancel := context.WithDeadline(ctx, dl)
	defer cancel()

	d := tls.Dialer{
		NetDialer: &net.Dialer{},
		Config: &tls.Config{
			ServerName:         sni,
			InsecureSkipVerify: true,
			MinVersion:         tls.VersionTLS12,
		},
	}

	start := time.Now()
	conn, err := d.DialContext(dialCtx, "tcp", addr)
	if err != nil {
		return 0, err
	}
	defer conn.Close()
	return time.Since(start), nil
}

var fallbackTraceSNIs = []string{
	"speed.cloudflare.com",
	"www.cloudflare.com",
	"cloudflare.com",
}

func getTraceHostsForProbe(primary string) []string {
	seen := make(map[string]struct{})
	var hosts []string
	add := func(h string) {
		h = strings.TrimSpace(h)
		if h == "" {
			return
		}
		if _, ok := seen[h]; ok {
			return
		}
		seen[h] = struct{}{}
		hosts = append(hosts, h)
	}
	add(primary)
	for _, h := range fallbackTraceSNIs {
		add(h)
	}
	return hosts
}

func probeTrace(ctx context.Context, ip net.IP, port int, host string, timeout time.Duration) (time.Duration, bool, int, string, error) {
	addr := net.JoinHostPort(ip.String(), strconv.Itoa(port))

	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
			return (&net.Dialer{Timeout: timeout / 4}).DialContext(ctx, network, addr)
		},
		TLSClientConfig: &tls.Config{
			ServerName:         host,
			MinVersion:         tls.VersionTLS12,
			InsecureSkipVerify: true,
		},
		DisableKeepAlives:   true,
		TLSHandshakeTimeout: timeout / 2,
	}

	client := &http.Client{
		Timeout:   timeout,
		Transport: transport,
	}

	scheme := "https"
	if port == 80 {
		scheme = "http"
	}
	reqURL := fmt.Sprintf("%s://%s/cdn-cgi/trace", scheme, host)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return 0, false, 0, "", err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")
	req.Host = host

	start := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		return 0, false, 0, "", err
	}
	defer resp.Body.Close()

	latency := time.Since(start)
	tlsOk := resp.TLS != nil
	httpStatus := resp.StatusCode

	bodyBytes, err := io.ReadAll(io.LimitReader(resp.Body, 1024))
	if err != nil {
		return 0, false, 0, "", err
	}
	bodyStr := string(bodyBytes)
	colo := parseTraceColo(bodyStr)

	return latency, tlsOk, httpStatus, colo, nil
}

func probeHTTP(ctx context.Context, ip net.IP, port int, sni string, timeout time.Duration, requireWS bool, wsHost, wsPath string) (time.Duration, string, bool, error) {
	var lat time.Duration
	var httpStatus int
	var colo string
	var err error

	traceSNI := sni
	for _, host := range getTraceHostsForProbe(sni) {
		lat, _, httpStatus, colo, err = probeTrace(ctx, ip, port, host, timeout)
		if err == nil && httpStatus >= 200 && httpStatus < 400 && colo != "" {
			traceSNI = host
			break
		}
	}

	if err != nil || httpStatus < 200 || httpStatus >= 400 || colo == "" {
		if err == nil {
			err = fmt.Errorf("status code %d, colo %s", httpStatus, colo)
		}
		return 0, "", false, err
	}

	wsOk := false
	if requireWS {
		wsOk = probeWebSocket(ctx, ip, port, traceSNI, wsHost, wsPath, timeout)
	}

	return lat, colo, wsOk, nil
}

func probeWebSocket(ctx context.Context, ip net.IP, port int, sni, host, path string, timeout time.Duration) bool {
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	wsCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	deadline, _ := wsCtx.Deadline()

	addr := net.JoinHostPort(ip.String(), strconv.Itoa(port))
	if host == "" {
		host = sni
	}
	if path == "" {
		path = "/"
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}

	dialer := &net.Dialer{Timeout: timeout / 4}
	conn, err := dialer.DialContext(wsCtx, "tcp", addr)
	if err != nil {
		return false
	}
	defer conn.Close()

	tlsConn := tls.Client(conn, &tls.Config{
		ServerName:         sni,
		MinVersion:         tls.VersionTLS12,
		InsecureSkipVerify: true,
	})

	_ = tlsConn.SetDeadline(deadline)
	if err := tlsConn.HandshakeContext(wsCtx); err != nil {
		return false
	}

	// Phase 1: idle hold
	idleHold := 2 * time.Second
	if remaining := time.Until(deadline); remaining < 2*idleHold {
		idleHold = remaining / 2
	}
	if idleHold > 0 {
		_ = tlsConn.SetReadDeadline(time.Now().Add(idleHold))
		oneByte := make([]byte, 1)
		if _, err := tlsConn.Read(oneByte); err != nil {
			if netErr, ok := err.(net.Error); !ok || !netErr.Timeout() {
				return false
			}
		}
	}

	// Phase 2: send WebSocket upgrade
	wsReq := fmt.Sprintf(
		"GET %s HTTP/1.1\r\n"+
			"Host: %s\r\n"+
			"Upgrade: websocket\r\n"+
			"Connection: Upgrade\r\n"+
			"Sec-WebSocket-Key: c2VucGFpc2Nhbm5lcg==\r\n"+
			"Sec-WebSocket-Version: 13\r\n"+
			"\r\n", path, host)

	_ = tlsConn.SetWriteDeadline(time.Now().Add(timeout / 2))
	if _, err := tlsConn.Write([]byte(wsReq)); err != nil {
		return false
	}

	respBuf := make([]byte, 1024)
	_ = tlsConn.SetReadDeadline(time.Now().Add(timeout / 3))
	n, err := tlsConn.Read(respBuf)
	if err != nil || n == 0 {
		return false
	}

	return strings.Contains(string(respBuf[:n]), "HTTP/")
}

func parseTraceColo(body string) string {
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimRight(line, "\r")
		if strings.HasPrefix(line, "colo=") {
			return strings.TrimSpace(strings.TrimPrefix(line, "colo="))
		}
	}
	return ""
}

// NeighborsAround returns up to 10 IPv4 addresses near ip (+/- 5 hosts) that fall inside targetNets
func NeighborsAround(ip net.IP, targetNets []*net.IPNet) []net.IP {
	ip4 := ip.To4()
	if ip4 == nil {
		return nil
	}
	base := binary.BigEndian.Uint32(ip4)
	var out []net.IP

	for offset := int32(-5); offset <= 5; offset++ {
		if offset == 0 {
			continue
		}
		next, ok := offsetIPv4(base, offset)
		if !ok {
			continue
		}
		candidate := uint32ToIPv4(next)
		if candidate.Equal(ip) {
			continue
		}
		// Check if contained in any target net
		inNet := false
		for _, n := range targetNets {
			if n.Contains(candidate) {
				inNet = true
				break
			}
		}
		if inNet {
			out = append(out, candidate)
		}
	}
	return out
}

func offsetIPv4(base uint32, delta int32) (uint32, bool) {
	if delta >= 0 {
		sum := uint64(base) + uint64(delta)
		if sum > 0xFFFFFFFF {
			return 0, false
		}
		return uint32(sum), true
	}
	d := uint32(-delta)
	if d > base {
		return 0, false
	}
	return base - d, true
}

func uint32ToIPv4(v uint32) net.IP {
	ip := make(net.IP, 4)
	binary.BigEndian.PutUint32(ip, v)
	return ip
}

// getFreePort returns a free TCP port
func getFreePort() (int, error) {
	addr, err := net.ResolveTCPAddr("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	l, err := net.ListenTCP("tcp", addr)
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}

func getBaseClientConfig() (*models.V2RayClientConfig, error) {
	if pebble.DB == nil {
		return nil, fmt.Errorf("pebble DB not initialized")
	}
	configs, total := pebble.ListClientConfigs(pebble.ConfigFilter{}, 0, 1)
	if total > 0 && len(configs) > 0 {
		return &configs[0], nil
	}
	return nil, fmt.Errorf("no base client configurations found")
}

func saveDiscoveredEndpoint(baseCfg *models.V2RayClientConfig, ip string, port int, latency int, speed float64) {
	if pebble.DB == nil {
		return
	}
	newCfg := *baseCfg
	newCfg.ID = 0
	newCfg.Address = ip
	newCfg.Port = port
	newCfg.LatencyMs = latency
	newCfg.Name = fmt.Sprintf("Discovered-%s:%d", ip, port)
	newCfg.IsActive = false
	newCfg.Priority = 100

	_ = pebble.SaveClientConfig(&newCfg)
}

func exportVerifiedIPs() {
	if pebble.DB == nil {
		return
	}
	configs, _ := pebble.ListClientConfigs(pebble.ConfigFilter{}, 0, 0)
	var activeConfigs []models.V2RayClientConfig
	for _, cfg := range configs {
		if cfg.LatencyMs > 0 && strings.HasPrefix(cfg.Name, "Discovered-") {
			activeConfigs = append(activeConfigs, cfg)
		}
	}
	
	sort.Slice(activeConfigs, func(i, j int) bool {
		return activeConfigs[i].LatencyMs < activeConfigs[j].LatencyMs
	})

	var lines []string
	var cleanLines []string
	var csvLines []string
	csvLines = append(csvLines, "IP,Port,Latency(ms),Speed(Mbps)")

	for _, cfg := range activeConfigs {
		lines = append(lines, fmt.Sprintf("%s:%d (latency: %dms)", cfg.Address, cfg.Port, cfg.LatencyMs))
		cleanLines = append(cleanLines, fmt.Sprintf("%s:%d", cfg.Address, cfg.Port))
		csvLines = append(csvLines, fmt.Sprintf("%s,%d,%d,%.2f", cfg.Address, cfg.Port, cfg.LatencyMs, 0.0))
	}
	
	content := strings.Join(lines, "\n")
	cleanContent := strings.Join(cleanLines, "\n")
	csvContent := strings.Join(csvLines, "\n")

	_ = os.WriteFile("ips.txt", []byte(content), 0644)
	_ = os.WriteFile("ips_clean.txt", []byte(cleanContent), 0644)
	_ = os.WriteFile("ips.csv", []byte(csvContent), 0644)

	_ = os.MkdirAll("data", 0755)
	_ = os.WriteFile("data/ips.txt", []byte(content), 0644)
	_ = os.WriteFile("data/ips_clean.txt", []byte(cleanContent), 0644)
	_ = os.WriteFile("data/ips.csv", []byte(csvContent), 0644)
}

func socks5Dial(proxyAddr, targetAddr string, timeout time.Duration) (net.Conn, error) {
	conn, err := net.DialTimeout("tcp", proxyAddr, timeout)
	if err != nil {
		return nil, err
	}

	_, err = conn.Write([]byte{5, 1, 0})
	if err != nil {
		conn.Close()
		return nil, err
	}

	res := make([]byte, 2)
	_, err = io.ReadFull(conn, res)
	if err != nil || res[0] != 5 || res[1] != 0 {
		conn.Close()
		return nil, fmt.Errorf("socks5 handshake failed")
	}

	host, portStr, err := net.SplitHostPort(targetAddr)
	if err != nil {
		conn.Close()
		return nil, err
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		conn.Close()
		return nil, err
	}

	reqBuf := []byte{5, 1, 0, 3, byte(len(host))}
	reqBuf = append(reqBuf, []byte(host)...)
	reqBuf = append(reqBuf, byte(port>>8), byte(port&0xff))

	_, err = conn.Write(reqBuf)
	if err != nil {
		conn.Close()
		return nil, err
	}

	reply := make([]byte, 10)
	_, err = io.ReadFull(conn, reply[:4])
	if err != nil || reply[0] != 5 || reply[1] != 0 {
		conn.Close()
		return nil, fmt.Errorf("socks5 request failed")
	}

	var boundLen int
	switch reply[3] {
	case 1:
		boundLen = 6
	case 3:
		lenBuf := make([]byte, 1)
		_, _ = io.ReadFull(conn, lenBuf)
		boundLen = int(lenBuf[0]) + 2
	case 4:
		boundLen = 18
	}

	if boundLen > 0 {
		boundBuf := make([]byte, boundLen)
		_, _ = io.ReadFull(conn, boundBuf)
	}

	return conn, nil
}

func testProxyThroughput(ctx context.Context, baseConfig models.V2RayClientConfig, ip string, port int) (int, float64, error) {
	socksPort, err := getFreePort()
	if err != nil {
		return 0, 0, err
	}

	testConfig := baseConfig
	testConfig.Address = ip
	testConfig.Port = port

	configBytes, err := compiler.CompileSingBoxClientConfig(testConfig, socksPort, socksPort+1, false, "")
	if err != nil {
		return 0, 0, err
	}

	var options boxOption.Options
	if err := json.Unmarshal(configBytes, &options); err != nil {
		return 0, 0, err
	}

	sbCtx := include.Context(ctx)
	instance, err := box.New(box.Options{
		Context: sbCtx,
		Options: options,
	})
	if err != nil {
		return 0, 0, err
	}

	if err := instance.Start(); err != nil {
		return 0, 0, err
	}
	defer instance.Close()

	socksAddr := net.JoinHostPort("127.0.0.1", strconv.Itoa(socksPort))
	ready := false
	for i := 0; i < 20; i++ {
		conn, err := net.DialTimeout("tcp", socksAddr, 100*time.Millisecond)
		if err == nil {
			conn.Close()
			ready = true
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if !ready {
		return 0, 0, fmt.Errorf("socks proxy did not start")
	}

	dial := func(ctx context.Context, _, addr string) (net.Conn, error) {
		return socks5Dial(socksAddr, addr, 3*time.Second)
	}

	client := &http.Client{
		Transport: &http.Transport{
			DialContext:           dial,
			DisableKeepAlives:     true,
			TLSHandshakeTimeout:   2 * time.Second,
			ResponseHeaderTimeout: 2 * time.Second,
		},
		Timeout: 5 * time.Second,
	}

	t0 := time.Now()
	req, err := http.NewRequestWithContext(ctx, "GET", "https://speed.cloudflare.com/cdn-cgi/trace", nil)
	if err != nil {
		return 0, 0, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")
	resp, err := client.Do(req)
	if err != nil {
		return 0, 0, err
	}
	ttfb := int(time.Since(t0).Milliseconds())
	resp.Body.Close()

	downURL := "https://speed.cloudflare.com/__down?bytes=100000"
	reqDown, err := http.NewRequestWithContext(ctx, "GET", downURL, nil)
	if err != nil {
		return ttfb, 0, err
	}
	reqDown.Header.Set("User-Agent", "Mozilla/5.0")

	tDownStart := time.Now()
	respDown, err := client.Do(reqDown)
	if err != nil {
		return ttfb, 0, err
	}
	defer respDown.Body.Close()

	buf := make([]byte, 8192)
	var totalBytes int64
	for {
		n, err := respDown.Body.Read(buf)
		if n > 0 {
			totalBytes += int64(n)
		}
		if err != nil {
			if err == io.EOF {
				break
			}
			return ttfb, 0, err
		}
	}

	elapsed := time.Since(tDownStart).Seconds()
	var mbps float64
	if elapsed > 0 && totalBytes > 0 {
		mbps = (float64(totalBytes*8) / elapsed) / 1_000_000.0
	}

	return ttfb, mbps, nil
}

// reloadClientCore compiles and hot-reloads the client core engine using the active configuration.
func reloadClientCore() error {
	if !core.IsClientRunning() {
		return nil
	}

	// Fetch active config
	configs, _ := pebble.ListClientConfigs(pebble.ConfigFilter{}, 0, 0)
	var activeConfig *models.V2RayClientConfig
	for _, cfg := range configs {
		if cfg.IsActive {
			activeCopy := cfg
			activeConfig = &activeCopy
			break
		}
	}
	if activeConfig == nil {
		return fmt.Errorf("no active configuration found in pebble db")
	}

	// Fetch settings
	var socksPort, httpPort int
	socksPort = 10808
	httpPort = 10809
	evasion := true

	if db.DB != nil {
		var socksPortSetting models.V2RayClientSetting
		if err := db.DB.Where("key = ?", "socks_port").First(&socksPortSetting).Error; err == nil {
			socksPort, _ = strconv.Atoi(socksPortSetting.Value)
		}
		var httpPortSetting models.V2RayClientSetting
		if err := db.DB.Where("key = ?", "http_port").First(&httpPortSetting).Error; err == nil {
			httpPort, _ = strconv.Atoi(httpPortSetting.Value)
		}
		var evasionSetting models.V2RayClientSetting
		if err := db.DB.Where("key = ?", "evasion_enabled").First(&evasionSetting).Error; err == nil {
			evasion = evasionSetting.Value == "true"
		}
	}

	socksPortPublic := core.FindAvailablePort(socksPort)
	socksPortInternal := core.FindAvailablePort(socksPortPublic+1000, socksPortPublic)
	httpPortPublic := core.FindAvailablePort(httpPort, socksPortPublic, socksPortInternal)
	httpPortInternal := core.FindAvailablePort(httpPortPublic+1000, socksPortPublic, socksPortInternal, httpPortPublic)

	configBytes, err := compiler.CompileClientConfig(*activeConfig, socksPortInternal, httpPortInternal, evasion, "")
	if err != nil {
		return fmt.Errorf("failed to compile client config: %w", err)
	}

	tempPath := filepath.Join(os.TempDir(), "xray_client.json")
	_ = os.WriteFile(tempPath, configBytes, 0644)

	_ = core.StopClientCore()
	if err := core.StartClientCore(tempPath); err != nil {
		return fmt.Errorf("failed to start client core: %w", err)
	}

	core.StartLocalProxyEngine(socksPortPublic, socksPortInternal, httpPortPublic, httpPortInternal)
	return nil
}

// probeTLS3Strike verifies a TLS handshake to the target IP:port, attempting up to 3 times.
func probeTLS3Strike(ctx context.Context, ip net.IP, port int, sni string, timeout time.Duration) (time.Duration, error) {
	var lastErr error
	var latency time.Duration
	addr := net.JoinHostPort(ip.String(), strconv.Itoa(port))

	for attempt := 0; attempt < 3; attempt++ {
		select {
		case <-ctx.Done():
			return 0, ctx.Err()
		default:
		}

		t0 := time.Now()
		dialer := &net.Dialer{Timeout: timeout}
		conn, err := dialer.DialContext(ctx, "tcp", addr)
		if err != nil {
			lastErr = err
			continue
		}

		tlsConn := tls.Client(conn, &tls.Config{
			ServerName:         sni,
			InsecureSkipVerify: true,
		})

		_ = tlsConn.SetDeadline(time.Now().Add(timeout))
		err = tlsConn.HandshakeContext(ctx)
		if err != nil {
			_ = tlsConn.Close()
			lastErr = err
			continue
		}

		latency = time.Since(t0)
		_ = tlsConn.Close()
		return latency, nil
	}
	return 0, fmt.Errorf("tls handshake failed after 3 attempts: %w", lastErr)
}

// probeHTTP1Byte writes a minimal HTTP GET request and verifies at least 1 byte of response is received.
func probeHTTP1Byte(ctx context.Context, ip net.IP, port int, sni string, timeout time.Duration, path string) (time.Duration, error) {
	addr := net.JoinHostPort(ip.String(), strconv.Itoa(port))
	t0 := time.Now()

	dialer := &net.Dialer{Timeout: timeout}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return 0, err
	}
	defer conn.Close()

	var reqConn net.Conn = conn
	if port == 443 || port == 8443 {
		tlsConn := tls.Client(conn, &tls.Config{
			ServerName:         sni,
			InsecureSkipVerify: true,
		})
		_ = tlsConn.SetDeadline(time.Now().Add(timeout))
		if err := tlsConn.HandshakeContext(ctx); err != nil {
			return 0, err
		}
		reqConn = tlsConn
	}

	_ = reqConn.SetDeadline(time.Now().Add(timeout))

	if path == "" {
		path = "/"
	}
	reqStr := fmt.Sprintf("GET %s HTTP/1.1\r\nHost: %s\r\nConnection: close\r\n\r\n", path, sni)
	_, err = reqConn.Write([]byte(reqStr))
	if err != nil {
		return 0, err
	}

	buf := make([]byte, 1)
	_, err = io.ReadFull(reqConn, buf)
	if err != nil {
		return 0, err
	}

	return time.Since(t0), nil
}
