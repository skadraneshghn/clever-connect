package scanner

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
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

	"clever-connect/internal/models"
	"clever-connect/internal/v2ray/compiler"
	"clever-connect/internal/v2ray/core"
	"clever-connect/internal/v2ray/speed"
	"clever-connect/internal/v2ray/sub"
)

// DefaultCloudflareRanges represents default Cloudflare edge IP blocks
var DefaultCloudflareRanges = []string{
	"173.245.48.0/20", "103.21.244.0/22", "103.22.200.0/22",
	"103.31.4.0/22", "141.101.64.0/18", "108.162.192.0/18",
	"190.93.240.0/20", "188.114.96.0/20", "197.234.240.0/22",
	"198.41.128.0/17", "162.158.0.0/15", "104.16.0.0/13",
	"104.24.0.0/14", "172.64.0.0/13", "131.0.72.0/22",
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

// StartScan runs the CDN scan in the background
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
	IP        string `json:"ip"`
	Hostname  string `json:"hostname,omitempty"`
	PingMs    int64  `json:"ping_ms"`
	Active    bool   `json:"active"`
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
