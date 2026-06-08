package scanner

import (
	"context"
	crand "crypto/rand"
	"crypto/tls"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"clever-connect/internal/db/pebble"
	"clever-connect/internal/models"
	"clever-connect/internal/v2ray/compiler"
	"clever-connect/internal/v2ray/core"
	"clever-connect/internal/v2ray/speed"
	"clever-connect/internal/v2ray/sub"

	"github.com/gin-gonic/gin"
	box "github.com/sagernet/sing-box"
	"github.com/sagernet/sing-box/include"
	boxOption "github.com/sagernet/sing-box/option"
	utls "github.com/refraction-networking/utls"
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
type CDNConfigRow struct {
	IP         string  `json:"ip"`
	Port       int     `json:"port"`
	OK         bool    `json:"ok"`
	PingMs     int     `json:"ping_ms"`     // TLS handshake duration
	RelayMs    int     `json:"relay_ms"`    // HTTP probe response time
	DownKbps   int     `json:"down_kbps"`   // download speed
	UpKbps     int     `json:"up_kbps"`     // upload speed
	HTTPStatus int     `json:"http_status"`
	Colo       string  `json:"colo"`        // Cloudflare location code
	Score      float64 `json:"score"`
	Status     string  `json:"status"`      // "GOOD", "DL-only", etc.
	URI        string  `json:"uri"`         // rewritten URI
	Error      string  `json:"error"`
}

// CDNScanState holds the live status of an in-progress scan
type CDNScanState struct {
	mu         sync.Mutex
	Phase      int            `json:"phase"` // 0: idle, 1: ping, 2: speed, 3: finished
	PingTotal  int            `json:"ping_total"`
	PingDone   int            `json:"ping_done"`
	SpeedTotal int            `json:"speed_total"`
	SpeedDone  int            `json:"speed_done"`
	Rows       []CDNConfigRow `json:"rows"`
	Saved      []string       `json:"saved"`
	Best       string         `json:"best"`
	Finished   bool           `json:"finished"`
	Cancelled  bool           `json:"cancelled"`
	Paused     bool           `json:"paused"`
	Err        string         `json:"err"`
	StartedAt  time.Time      `json:"started_at"`
	cancelFunc context.CancelFunc
}

var (
	activeScan *CDNScanState
	scanMu     sync.Mutex
)

// GetActiveScan returns the current scanning state
func GetActiveScan() *CDNScanState {
	scanMu.Lock()
	defer scanMu.Unlock()
	return activeScan
}

// CancelActiveScan stops any currently running scan
func CancelActiveScan() {
	scanMu.Lock()
	defer scanMu.Unlock()
	if activeScan != nil && activeScan.cancelFunc != nil {
		activeScan.cancelFunc()
		activeScan.Cancelled = true
		activeScan.Finished = true
		activeScan.Phase = 3
	}
}

// Snapshot returns a copy of the scan state
func (s *CDNScanState) Snapshot() map[string]interface{} {
	s.mu.Lock()
	defer s.mu.Unlock()
	rows := make([]CDNConfigRow, len(s.Rows))
	copy(rows, s.Rows)
	sort.SliceStable(rows, func(a, b int) bool {
		if rows[a].OK != rows[b].OK {
			return rows[a].OK
		}
		return rows[a].Score > rows[b].Score
	})
	elapsed := 0
	if !s.StartedAt.IsZero() {
		elapsed = int(time.Since(s.StartedAt).Milliseconds())
	}
	return map[string]interface{}{
		"phase":       s.Phase,
		"ping_total":  s.PingTotal,
		"ping_done":   s.PingDone,
		"speed_total": s.SpeedTotal,
		"speed_done":  s.SpeedDone,
		"rows":        rows,
		"saved":       s.Saved,
		"best":        s.Best,
		"finished":    s.Finished,
		"cancelled":   s.Cancelled,
		"paused":      s.Paused,
		"err":         s.Err,
		"elapsed_ms":  elapsed,
	}
}

// FrontResult is the TLS ping report
type FrontResult struct {
	IP         string
	OK         bool
	TCPms      int
	TLSms      int
	PingMs     int
	HTTPStatus int
	Error      string
}

// ExpandCIDR returns all IP addresses in a CIDR block up to maxIPs
func ExpandCIDR(cidr string, maxIPs int) []string {
	p, err := netip.ParsePrefix(cidr)
	if err != nil {
		return nil
	}
	p = p.Masked()
	var ips []string
	addr := p.Addr()
	for i := 0; i < maxIPs; i++ {
		addr = addr.Next()
		if !p.Contains(addr) {
			break
		}
		ips = append(ips, addr.String())
	}
	return ips
}

// FrontTest performs domain fronting TCP+TLS ping against an IP
func FrontTest(ip string, port int, frontSNI, realHost string, timeout time.Duration) FrontResult {
	res := FrontResult{IP: ip, TCPms: -1, TLSms: -1, PingMs: -1}
	host := realHost
	if host == "" {
		host = frontSNI
	}
	if host == "" {
		host = ip
	}

	t0 := time.Now()
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(ip, strconv.Itoa(port)), timeout)
	if err != nil {
		res.Error = err.Error()
		return res
	}
	defer conn.Close()
	res.TCPms = int(time.Since(t0).Milliseconds())

	tc := tls.Client(conn, &tls.Config{ServerName: frontSNI, InsecureSkipVerify: true})
	_ = tc.SetDeadline(time.Now().Add(timeout))
	t1 := time.Now()
	if err := tc.Handshake(); err != nil {
		res.Error = err.Error()
		return res
	}
	res.TLSms = int(time.Since(t1).Milliseconds())
	res.OK = true
	res.PingMs = res.TLSms

	// Send HTTP HEAD request to verify endpoint
	_ = tc.SetDeadline(time.Now().Add(1500 * time.Millisecond))
	req := "HEAD / HTTP/1.1\r\nHost: " + host + "\r\nUser-Agent: Mozilla/5.0\r\nConnection: close\r\n\r\n"
	t2 := time.Now()
	if _, err := tc.Write([]byte(req)); err != nil {
		return res
	}
	buf := make([]byte, 256)
	n, _ := tc.Read(buf)
	if n > 0 {
		res.PingMs = int(time.Since(t2).Milliseconds())
		res.HTTPStatus = parseHTTPStatus(string(buf[:n]))
	}
	return res
}

func parseHTTPStatus(line string) int {
	if i := strings.IndexByte(line, '\n'); i >= 0 {
		line = line[:i]
	}
	f := strings.Fields(line)
	if len(f) >= 2 {
		if code, err := strconv.Atoi(f[1]); err == nil {
			return code
		}
	}
	return 0
}

// CDNConfigsOptions represents scanner settings
type CDNConfigsOptions struct {
	URI             string   `json:"uri"`
	Ranges          []string `json:"ranges"`
	PerRangeLimit   int      `json:"per_range_limit"`
	MaxScanCap      int      `json:"max_scan_cap"`
	Ports           []int    `json:"ports"`
	TopForSpeed     int      `json:"top_for_speed"`
	FinalCount      int      `json:"final_count"`
	DownloadBytes   int      `json:"download_bytes"`
	UploadBytes     int      `json:"upload_bytes"`
	PingTimeoutSec  int      `json:"ping_timeout_sec"`
	SpeedTimeoutSec int      `json:"speed_timeout_sec"`
	PingConcurrency int      `json:"ping_concurrency"`
	SpeedConc       int      `json:"speed_conc"`
	BasePort        int      `json:"base_port"`
}

// StartScan runs the CDN scan in the background (legacy)
func StartScan(opts CDNConfigsOptions) (*CDNScanState, error) {
	scanMu.Lock()
	defer scanMu.Unlock()

	if activeScan != nil && !activeScan.Finished {
		return nil, fmt.Errorf("a scan is already running")
	}

	ctx, cancel := context.WithCancel(context.Background())
	state := &CDNScanState{
		StartedAt:  time.Now(),
		Phase:      1,
		cancelFunc: cancel,
	}
	activeScan = state

	go func() {
		defer cancel()
		err := runCDNScan(ctx, state, opts)
		state.mu.Lock()
		state.Finished = true
		state.Phase = 3
		if err != nil {
			state.Err = err.Error()
		}
		state.mu.Unlock()
	}()

	return state, nil
}

func runCDNScan(ctx context.Context, state *CDNScanState, opts CDNConfigsOptions) error {
	bin := core.GetXrayBinPath()
	if _, err := os.Stat(bin); err != nil {
		coreName := core.GetSelectedCoreName()
		if path, err := exec.LookPath(coreName); err == nil {
			bin = path
		} else {
			return fmt.Errorf("%s binary not found", coreName)
		}
	}

	clientCfg, err := sub.ParseProxyLink(opts.URI)
	if err != nil {
		return fmt.Errorf("invalid template URI: %w", err)
	}

	if opts.PerRangeLimit <= 0 {
		opts.PerRangeLimit = 200
	}
	if opts.MaxScanCap <= 0 {
		opts.MaxScanCap = 50000
	}
	if len(opts.Ports) == 0 {
		opts.Ports = []int{443}
	}
	if opts.TopForSpeed <= 0 {
		opts.TopForSpeed = 20
	}
	if opts.FinalCount <= 0 {
		opts.FinalCount = 5
	}
	if opts.DownloadBytes <= 0 {
		opts.DownloadBytes = 1_000_000
	}
	if opts.UploadBytes <= 0 {
		opts.UploadBytes = 500_000
	}
	if opts.PingTimeoutSec <= 0 {
		opts.PingTimeoutSec = 3
	}
	if opts.SpeedTimeoutSec <= 0 {
		opts.SpeedTimeoutSec = 10
	}
	if opts.PingConcurrency <= 0 {
		opts.PingConcurrency = 64
	}
	if opts.SpeedConc <= 0 {
		opts.SpeedConc = 3
	}
	if opts.BasePort <= 0 {
		opts.BasePort = 25000
	}

	ranges := opts.Ranges
	if len(ranges) == 0 {
		ranges = DefaultCloudflareRanges
	}

	var ips []string
	for _, r := range ranges {
		if !strings.Contains(r, "/") {
			ips = append(ips, r)
			continue
		}
		ips = append(ips, ExpandCIDR(r, opts.PerRangeLimit)...)
	}

	if len(ips) > opts.MaxScanCap {
		ips = ips[:opts.MaxScanCap]
	}
	if len(ips) == 0 {
		return fmt.Errorf("no target IPs resolved")
	}

	type target struct {
		IP   string
		Port int
	}
	var targets []target
	for _, ip := range ips {
		for _, port := range opts.Ports {
			targets = append(targets, target{IP: ip, Port: port})
		}
	}

	// Extract SNI from template client config
	frontSNI := clientCfg.Address
	var tlsS map[string]interface{}
	if clientCfg.TLSSettings != "" {
		_ = json.Unmarshal([]byte(clientCfg.TLSSettings), &tlsS)
		if sniVal, ok := tlsS["sni"].(string); ok && sniVal != "" {
			frontSNI = sniVal
		}
	}

	state.mu.Lock()
	state.PingTotal = len(targets)
	state.mu.Unlock()

	type pingRow struct {
		IP   string
		Port int
		Ms   int
		OK   bool
	}

	pingRows := make([]pingRow, len(targets))
	var wg sync.WaitGroup
	sem := make(chan struct{}, opts.PingConcurrency)

	for i, tg := range targets {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		wg.Add(1)
		sem <- struct{}{}
		go func(idx int, t target) {
			defer wg.Done()
			defer func() { <-sem }()
			res := FrontTest(t.IP, t.Port, frontSNI, clientCfg.Address, time.Duration(opts.PingTimeoutSec)*time.Second)
			pingRows[idx] = pingRow{IP: t.IP, Port: t.Port, Ms: res.PingMs, OK: res.OK}

			state.mu.Lock()
			state.PingDone++
			state.mu.Unlock()
		}(i, tg)
	}
	wg.Wait()

	sort.SliceStable(pingRows, func(a, b int) bool {
		if pingRows[a].OK != pingRows[b].OK {
			return pingRows[a].OK
		}
		ma, mb := pingRows[a].Ms, pingRows[b].Ms
		if ma < 0 {
			ma = 1 << 30
		}
		if mb < 0 {
			mb = 1 << 30
		}
		return ma < mb
	})

	var candidates []pingRow
	for _, r := range pingRows {
		if r.OK {
			candidates = append(candidates, r)
		}
		if len(candidates) >= opts.TopForSpeed {
			break
		}
	}

	if len(candidates) == 0 {
		return fmt.Errorf("no reachable Cloudflare CDN IPs found")
	}

	state.mu.Lock()
	state.Phase = 2
	state.SpeedTotal = len(candidates)
	state.mu.Unlock()

	var portSeq int64 = int64(opts.BasePort) - 1
	var wg2 sync.WaitGroup
	sem2 := make(chan struct{}, opts.SpeedConc)

	for _, c := range candidates {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		wg2.Add(1)
		sem2 <- struct{}{}
		go func(cand pingRow) {
			defer wg2.Done()
			defer func() { <-sem2 }()

			socksPort := int(atomic.AddInt64(&portSeq, 2))
			httpPort := socksPort + 1

			row := cdnSpeedOne(ctx, bin, clientCfg, cand.IP, cand.Port, socksPort, httpPort, opts)
			row.PingMs = cand.Ms
			classify(&row)

			// Build rewritten URI
			row.URI = rewriteURI(opts.URI, cand.IP, cand.Port, row.Colo, row.PingMs)

			state.mu.Lock()
			state.Rows = append(state.Rows, row)
			state.SpeedDone++
			state.mu.Unlock()
		}(c)
	}
	wg2.Wait()

	// Build the Saved box
	state.mu.Lock()
	rowsCopy := make([]CDNConfigRow, len(state.Rows))
	copy(rowsCopy, state.Rows)
	sort.SliceStable(rowsCopy, func(a, b int) bool {
		if rowsCopy[a].OK != rowsCopy[b].OK {
			return rowsCopy[a].OK
		}
		return rowsCopy[a].Score > rowsCopy[b].Score
	})

	finalCount := opts.FinalCount
	if finalCount > len(rowsCopy) {
		finalCount = len(rowsCopy)
	}

	for i := 0; i < finalCount; i++ {
		if rowsCopy[i].OK && rowsCopy[i].URI != "" {
			state.Saved = append(state.Saved, rowsCopy[i].URI)
		}
	}
	if len(state.Saved) > 0 {
		state.Best = state.Saved[0]
	}
	state.mu.Unlock()

	return nil
}

func cdnSpeedOne(ctx context.Context, bin string, templateCfg models.V2RayClientConfig, ip string, port int, socksPort, httpPort int, opts CDNConfigsOptions) CDNConfigRow {
	row := CDNConfigRow{IP: ip, Port: port, RelayMs: -1, DownKbps: -1, UpKbps: -1, PingMs: -1}

	// Compile config replacing template's IP address with the specific Cloudflare IP
	cfgCopy := templateCfg
	cfgCopy.Address = ip
	cfgCopy.Port = port

	configBytes, err := compiler.CompileClientConfig(cfgCopy, socksPort, httpPort, true, "")
	if err != nil {
		row.Error = "compile: " + err.Error()
		return row
	}

	tempPath := filepath.Join(os.TempDir(), fmt.Sprintf("cdn_scan_%d.json", socksPort))
	_ = os.WriteFile(tempPath, configBytes, 0644)
	defer os.Remove(tempPath)

	runCtx, runCancel := context.WithTimeout(ctx, time.Duration(opts.SpeedTimeoutSec+8)*time.Second)
	defer runCancel()

	if abs, err := filepath.Abs(bin); err == nil {
		bin = abs
	}

	cmd := exec.CommandContext(runCtx, bin, "run", "-c", tempPath)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	absBinDir, err := filepath.Abs(filepath.Dir(bin))
	if err == nil {
		cmd.Dir = absBinDir
	}

	if err := cmd.Start(); err != nil {
		row.Error = "start: " + err.Error()
		return row
	}
	defer func() {
		pgid, err := syscall.Getpgid(cmd.Process.Pid)
		if err == nil {
			_ = syscall.Kill(-pgid, syscall.SIGKILL)
		} else {
			_ = cmd.Process.Kill()
		}
		_ = cmd.Wait()
	}()

	socksAddr := net.JoinHostPort("127.0.0.1", strconv.Itoa(socksPort))
	ready := false
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if c, e := net.DialTimeout("tcp", socksAddr, 200*time.Millisecond); e == nil {
			c.Close()
			ready = true
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	if !ready {
		row.Error = "SOCKS timeout"
		return row
	}

	time.Sleep(100 * time.Millisecond)
	spTimeout := time.Duration(opts.SpeedTimeoutSec) * time.Second

	t0 := time.Now()
	status, _, err := fetchThroughSocks("127.0.0.1", socksPort, "https://speed.cloudflare.com/", spTimeout)
	row.RelayMs = int(time.Since(t0).Milliseconds())
	row.HTTPStatus = status
	if err != nil {
		row.Error = "probe: " + err.Error()
		return row
	}

	row.Colo = speed.DetectColo("127.0.0.1", socksPort, 3*time.Second)

	down, err := speed.MeasureDownload("127.0.0.1", socksPort, opts.DownloadBytes, spTimeout)
	if err == nil {
		row.DownKbps = down
	}
	up, err := speed.MeasureUpload("127.0.0.1", socksPort, opts.UploadBytes, spTimeout)
	if err == nil {
		row.UpKbps = up
	}

	row.OK = down > 0
	return row
}

func fetchThroughSocks(socksHost string, socksPort int, url string, timeout time.Duration) (int, int, error) {
	dial := func(_ context.Context, _, addr string) (net.Conn, error) {
		return net.DialTimeout("tcp", net.JoinHostPort(socksHost, strconv.Itoa(socksPort)), timeout)
	}
	tr := &http.Transport{DialContext: dial, DisableKeepAlives: true}
	client := &http.Client{Transport: tr, Timeout: timeout}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return 0, 0, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")
	resp, err := client.Do(req)
	if err != nil {
		return 0, 0, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
	return resp.StatusCode, len(body), nil
}

func classify(r *CDNConfigRow) {
	dl := float64(r.DownKbps) / 1000.0
	ul := float64(r.UpKbps) / 1000.0
	if r.PingMs < 0 {
		return
	}
	r.Score = dl*0.75 + ul*0.25 - float64(r.PingMs)/50.0
	const dlMin, ulMin = 2.0, 1.0
	switch {
	case dl >= dlMin && ul >= ulMin:
		r.Status = "GOOD"
	case dl >= dlMin:
		r.Status = "DL-only"
	case ul >= ulMin:
		r.Status = "UL-only"
	default:
		r.Status = "Below"
	}
}

func rewriteURI(origURI string, ip string, port int, colo string, ping int) string {
	u, err := url.Parse(origURI)
	if err != nil {
		return ""
	}

	name := u.Fragment
	if name == "" {
		name = "Cloudflare IP"
	} else {
		if decoded, err := url.PathUnescape(name); err == nil {
			name = decoded
		}
	}

	// Format name: "OriginalName | COLO | Pingms | @CleverConnect"
	var parts []string
	parts = append(parts, name)
	if colo != "" {
		parts = append(parts, colo)
	}
	if ping >= 0 {
		parts = append(parts, strconv.Itoa(ping)+"ms")
	}
	parts = append(parts, "@CleverConnect")
	newName := strings.Join(parts, " | ")

	u.Host = net.JoinHostPort(ip, strconv.Itoa(port))
	u.Fragment = url.PathEscape(newName)
	return u.String()
}

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
	Tested   int64 `json:"tested"`
	Healthy  int64 `json:"healthy"`
	Failed   int64 `json:"failed"`
	InFlight int64 `json:"in_flight"`
}

// ScanConfig defines the operational bounds for a live network verification sweep
type ScanConfig struct {
	TargetCIDRs      []string      `json:"target_cidrs"`
	SelectedPorts    []int         `json:"selected_ports"`
	ConcurrencyLimit int           `json:"concurrency_limit"`
	MaxRateLimit     float64       `json:"max_rate_limit"`
	NetworkTimeout   time.Duration `json:"network_timeout"`
	ProbeAttempts    int           `json:"probe_attempts"`
	TargetMode       string        `json:"target_mode"`
	TargetSNI        string        `json:"target_sni"`
	WebSocketHost    string        `json:"websocket_host"`
	WebSocketPath    string        `json:"websocket_path"`
	RequireWS        bool          `json:"require_ws"`
	EnableNeighbors  bool          `json:"enable_neighbors"`
	TopLimit         int           `json:"top_limit"`
	TotalTargetCount int           `json:"total_target_count"`
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
	return JobStats{
		Tested:   atomic.LoadInt64(&s.stats.Tested),
		Healthy:  atomic.LoadInt64(&s.stats.Healthy),
		Failed:   atomic.LoadInt64(&s.stats.Failed),
		InFlight: atomic.LoadInt64(&s.stats.InFlight),
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
func (s *ScannerEngine) StartScan(parentCtx context.Context, cfg *ScanConfig) error {
	s.mu.Lock()
	if s.isRunning {
		s.mu.Unlock()
		return fmt.Errorf("a scan sweep is already running")
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
	// Parse Target CIDRs/Subnets for Neighborhood check
	var targetNets []*net.IPNet
	var sourceBuilder strings.Builder
	rangesToParse := cfg.TargetCIDRs
	if len(rangesToParse) == 0 {
		rangesToParse = DefaultCloudflareRanges
	}
	for i, c := range rangesToParse {
		if i > 0 {
			sourceBuilder.WriteString(",")
		}
		sourceBuilder.WriteString(c)

		if _, ipnet, err := net.ParseCIDR(c); err == nil {
			targetNets = append(targetNets, ipnet)
		} else {
			if parsedIP := net.ParseIP(c); parsedIP != nil {
				targetNets = append(targetNets, &net.IPNet{IP: parsedIP, Mask: net.CIDRMask(32, 32)})
			}
		}
	}

	// Fetch base template client configuration from DB
	baseConfig, baseErr := getBaseClientConfig()
	if baseErr != nil {
		s.broadcast("scanner.error", baseErr.Error())
	}

	// Stream IPs dynamically
	addrChan, err := StreamAddresses(ctx, sourceBuilder.String(), false)
	if err != nil {
		s.broadcast("scanner.error", err.Error())
		return
	}

	concurrency := cfg.ConcurrencyLimit
	if concurrency <= 0 {
		concurrency = 100
	}

	jobs := make(chan configProbeJob, concurrency*2)
	type probeResult struct {
		ip      net.IP
		port    int
		ok      bool
		latency int
		speed   float64
	}
	results := make(chan probeResult, concurrency)

	var seenMu sync.Mutex
	seen := make(map[string]struct{})

	var pending int64
	mainDone := make(chan struct{})
	submitChan := make(chan configProbeJob, 100000)

	submitJob := func(ip net.IP, port int) {
		atomic.AddInt64(&pending, 1)
		select {
		case <-ctx.Done():
			atomic.AddInt64(&pending, -1)
		case submitChan <- configProbeJob{ip: ip, port: port}:
		}
	}

	// Start main producer
	go func() {
		defer close(mainDone)
		count := 0
		for {
			select {
			case <-ctx.Done():
				return
			case ipStr, ok := <-addrChan:
				if !ok {
					return
				}
				ip := net.ParseIP(ipStr)
				if ip == nil {
					continue
				}
				for _, port := range cfg.SelectedPorts {
					submitJob(ip, port)
				}
				count++
				if cfg.TotalTargetCount > 0 && count >= cfg.TotalTargetCount {
					return
				}
			}
		}
	}()

	// Start coordinator
	go func() {
		defer close(jobs)
		for {
			select {
			case <-ctx.Done():
				return
			case job := <-submitChan:
				key := fmt.Sprintf("%s:%d", job.ip.String(), job.port)
				seenMu.Lock()
				if _, exists := seen[key]; exists {
					seenMu.Unlock()
					atomic.AddInt64(&pending, -1)
					continue
				}
				seen[key] = struct{}{}
				seenMu.Unlock()

				select {
				case <-ctx.Done():
					return
				case jobs <- job:
				}
			default:
				// If submitChan is empty, check if we are done
				select {
				case <-mainDone:
					if atomic.LoadInt64(&pending) == 0 {
						return
					}
				default:
				}
				time.Sleep(5 * time.Millisecond)
			}
		}
	}()

	var limiter *rate.Limiter
	if cfg.MaxRateLimit > 0 {
		limiter = rate.NewLimiter(rate.Limit(cfg.MaxRateLimit), int(cfg.MaxRateLimit)+1)
	}

	var wg sync.WaitGroup
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for job := range jobs {
				if ctx.Err() != nil {
					atomic.AddInt64(&pending, -1)
					continue
				}

				if limiter != nil {
					_ = limiter.Wait(ctx)
				}

				atomic.AddInt64(&s.stats.InFlight, 1)

				sni := cfg.TargetSNI
				if sni == "" {
					sni = selectRandomSNI(defaultEdgeSNIs)
				}

				var ok bool
				var probeErr error
				var probeLatency time.Duration

				if cfg.TargetMode == "tcp" {
					probeLatency, probeErr = probeTCP(ctx, job.ip, job.port, cfg.NetworkTimeout)
					ok = probeErr == nil
				} else if cfg.TargetMode == "tls" {
					probeLatency, probeErr = probeTLS(ctx, job.ip, job.port, sni, cfg.NetworkTimeout)
					ok = probeErr == nil
				} else { // http / default
					var wsOk bool
					probeLatency, _, wsOk, probeErr = probeHTTP(ctx, job.ip, job.port, sni, cfg.NetworkTimeout, cfg.RequireWS, cfg.WebSocketHost, cfg.WebSocketPath)
					ok = probeErr == nil
					if cfg.RequireWS && !wsOk {
						ok = false
					}
				}

				atomic.AddInt64(&s.stats.InFlight, -1)

				latency := int(probeLatency.Milliseconds())
				var speed float64

				// In-Memory proxy validation (Phase 5)
				if ok && baseConfig != nil {
					l, sp, errProxy := testProxyThroughput(ctx, *baseConfig, job.ip.String(), job.port)
					if errProxy == nil {
						latency = l
						speed = sp
					} else {
						// If in-memory proxy test fails, we classify it as failed proxy
						ok = false
					}
				}

				select {
				case results <- probeResult{ip: job.ip, port: job.port, ok: ok, latency: latency, speed: speed}:
				case <-ctx.Done():
					atomic.AddInt64(&pending, -1)
					return
				}
			}
		}()
	}

	var neighborsQueued int64
	maxNeighbors := int64(400)

	// Result processor loop
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case r, ok := <-results:
				if !ok {
					return
				}
				atomic.AddInt64(&s.stats.Tested, 1)

				if r.ok {
					atomic.AddInt64(&s.stats.Healthy, 1)

					// 1. Save immediately to DB
					if baseConfig != nil {
						saveDiscoveredEndpoint(baseConfig, r.ip.String(), r.port, r.latency, r.speed)
					}

					// 2. Queue neighbors
					if cfg.EnableNeighbors && atomic.LoadInt64(&neighborsQueued) < maxNeighbors {
						neighbors := NeighborsAround(r.ip, targetNets)
						for _, nip := range neighbors {
							if atomic.LoadInt64(&neighborsQueued) >= maxNeighbors {
								break
							}
							submitJob(nip, r.port)
							atomic.AddInt64(&neighborsQueued, 1)
						}
					}

					// 3. Broadcast update to websocket listeners
					s.broadcast("scanner.update", gin.H{
						"ip":         r.ip.String(),
						"port":       r.port,
						"ok":         true,
						"latency_ms": r.latency,
						"speed_mbps": r.speed,
					})
				} else {
					atomic.AddInt64(&s.stats.Failed, 1)
					s.broadcast("scanner.update", gin.H{
						"ip":   r.ip.String(),
						"port": r.port,
						"ok":   false,
					})
				}
			}
		}
	}()

	wg.Wait()
	close(results)

	// Export pipeline
	exportVerifiedIPs()
	s.broadcast("scanner.complete", nil)
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
	dialer := &net.Dialer{Timeout: timeout / 4} // 1/4 TCP handshake split
	start := time.Now()
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return 0, err
	}
	defer conn.Close()

	uConn := utls.UClient(conn, &utls.Config{
		ServerName:         sni,
		InsecureSkipVerify: true,
		MinVersion:         utls.VersionTLS12,
	}, utls.HelloChrome_Auto)

	_ = conn.SetDeadline(time.Now().Add(timeout / 2)) // 1/2 TLS validation split
	if err := uConn.Handshake(); err != nil {
		return 0, err
	}
	return time.Since(start), nil
}

func probeHTTP(ctx context.Context, ip net.IP, port int, sni string, timeout time.Duration, requireWS bool, wsHost, wsPath string) (time.Duration, string, bool, error) {
	addr := net.JoinHostPort(ip.String(), strconv.Itoa(port))

	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
			return (&net.Dialer{Timeout: timeout / 4}).DialContext(ctx, network, addr)
		},
		DialTLSContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
			conn, err := (&net.Dialer{Timeout: timeout / 4}).DialContext(ctx, network, addr)
			if err != nil {
				return nil, err
			}
			uConn := utls.UClient(conn, &utls.Config{
				ServerName:         sni,
				InsecureSkipVerify: true,
				MinVersion:         utls.VersionTLS12,
			}, utls.HelloChrome_Auto)
			_ = uConn.SetDeadline(time.Now().Add(timeout / 2))
			if err := uConn.Handshake(); err != nil {
				uConn.Close()
				return nil, err
			}
			return uConn, nil
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
	reqURL := fmt.Sprintf("%s://%s/cdn-cgi/trace", scheme, sni)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return 0, "", false, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")
	req.Host = sni

	start := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		return 0, "", false, err
	}
	defer resp.Body.Close()

	latency := time.Since(start)
	if resp.StatusCode < 200 || resp.StatusCode >= 400 {
		return 0, "", false, fmt.Errorf("status code %d", resp.StatusCode)
	}

	bodyBytes, err := io.ReadAll(io.LimitReader(resp.Body, 1024))
	if err != nil {
		return 0, "", false, err
	}
	bodyStr := string(bodyBytes)
	colo := parseTraceColo(bodyStr)

	wsOk := false
	if requireWS {
		wsOk = probeWebSocket(ctx, ip, port, sni, wsHost, wsPath, timeout)
	}

	return latency, colo, wsOk, nil
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

	uConn := utls.UClient(conn, &utls.Config{
		ServerName:         sni,
		MinVersion:         utls.VersionTLS12,
		InsecureSkipVerify: true,
	}, utls.HelloChrome_Auto)

	_ = uConn.SetDeadline(deadline)
	if err := uConn.Handshake(); err != nil {
		return false
	}

	wsReq := fmt.Sprintf(
		"GET %s HTTP/1.1\r\n"+
			"Host: %s\r\n"+
			"Upgrade: websocket\r\n"+
			"Connection: Upgrade\r\n"+
			"Sec-WebSocket-Key: c2VucGFpc2Nhbm5lcg==\r\n"+
			"Sec-WebSocket-Version: 13\r\n"+
			"\r\n", path, host)

	_ = uConn.SetWriteDeadline(time.Now().Add(timeout / 2))
	if _, err := uConn.Write([]byte(wsReq)); err != nil {
		return false
	}

	respBuf := make([]byte, 1024)
	_ = uConn.SetReadDeadline(time.Now().Add(timeout / 3))
	n, err := uConn.Read(respBuf)
	if err != nil || n == 0 {
		return false
	}

	respStr := string(respBuf[:n])
	if !strings.Contains(respStr, "101") && !strings.Contains(strings.ToLower(respStr), "switching protocols") {
		return false
	}

	// 2-second Stateful Idle Hold check
	idleHold := 2 * time.Second
	if remaining := time.Until(deadline); remaining < 2*idleHold {
		idleHold = remaining / 2
	}
	if idleHold > 0 {
		_ = uConn.SetReadDeadline(time.Now().Add(idleHold))
		oneByte := make([]byte, 1)
		_, errRead := uConn.Read(oneByte)
		if errRead != nil {
			if netErr, ok := errRead.(net.Error); ok && netErr.Timeout() {
				return true
			}
			return false
		}
		return true
	}

	return true
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
	for _, cfg := range activeConfigs {
		lines = append(lines, fmt.Sprintf("%s:%d (latency: %dms)", cfg.Address, cfg.Port, cfg.LatencyMs))
	}
	
	content := strings.Join(lines, "\n")
	_ = os.WriteFile("ips.txt", []byte(content), 0644)
	_ = os.MkdirAll("data", 0755)
	_ = os.WriteFile("data/ips.txt", []byte(content), 0644)
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
