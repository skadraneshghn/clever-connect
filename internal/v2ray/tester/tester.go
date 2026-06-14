package tester

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
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
)

type TestOptions struct {
	IDs                []uint        `json:"ids"`
	TestType           string        `json:"test_type"` // "tcp_ping", "tls_ping", "real_url"
	Concurrency        int           `json:"concurrency"`
	Timeout            time.Duration `json:"timeout"`
	URL                string        `json:"url"`
	Core               string        `json:"core"` // "xray", "sing-box", "v2ray", "current"
	DelayBetweenSameIP time.Duration `json:"delay_ms"`
}

type ConfigTestResult struct {
	ConfigID   uint   `json:"config_id"`
	Name       string `json:"name"`
	OK         bool   `json:"ok"`
	PingMs     int    `json:"ping_ms"`
	RelayMs    int    `json:"relay_ms"`
	HTTPStatus int    `json:"http_status"`
	Colo       string `json:"colo"`
	Error      string `json:"error"`
}

type ResultMessage struct {
	Type    string            `json:"type"`             // "status" or "result"
	Status  string            `json:"status,omitempty"` // "running", "completed", "stopped", "error"
	Total   int               `json:"total"`
	Current int               `json:"current"`
	Result  *ConfigTestResult `json:"result,omitempty"`
}

var (
	activeJobCancel context.CancelFunc
	activeJobMu     sync.Mutex
	isRunning       bool

	lastIPAccess   = make(map[string]time.Time)
	lastIPAccessMu sync.Mutex
)

// StartTesting triggers the testing run asynchronously
func StartTesting(opts TestOptions, onProgress func(ResultMessage)) error {
	activeJobMu.Lock()
	if isRunning {
		activeJobMu.Unlock()
		return fmt.Errorf("a test job is already running")
	}
	isRunning = true
	ctx, cancel := context.WithCancel(context.Background())
	activeJobCancel = cancel
	activeJobMu.Unlock()

	go func() {
		defer func() {
			activeJobMu.Lock()
			isRunning = false
			activeJobCancel = nil
			activeJobMu.Unlock()
		}()

		err := RunTests(ctx, opts, onProgress)
		if err != nil {
			onProgress(ResultMessage{
				Type:   "status",
				Status: "error",
				Result: &ConfigTestResult{Error: err.Error()},
			})
		}
	}()

	return nil
}

// StopTesting halts any active testing run
func StopTesting() {
	activeJobMu.Lock()
	defer activeJobMu.Unlock()
	if activeJobCancel != nil {
		activeJobCancel()
		activeJobCancel = nil
	}
	isRunning = false
}

// RunTests processes the configuration sweep
func RunTests(ctx context.Context, opts TestOptions, onProgress func(ResultMessage)) error {
	configs := make([]models.V2RayClientConfig, 0)
	if len(opts.IDs) > 0 {
		for _, id := range opts.IDs {
			if cfg, err := pebble.GetClientConfig(id); err == nil {
				configs = append(configs, *cfg)
			}
		}
	} else {
		configs, _ = pebble.ListClientConfigs(pebble.ConfigFilter{}, 0, 0)
	}

	if len(configs) == 0 {
		return fmt.Errorf("no configurations found to test")
	}

	total := len(configs)
	onProgress(ResultMessage{
		Type:    "status",
		Status:  "running",
		Total:   total,
		Current: 0,
	})

	// Shuffle configurations for Iran GFW rate limit protection
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	r.Shuffle(len(configs), func(i, j int) {
		configs[i], configs[j] = configs[j], configs[i]
	})

	concurrency := opts.Concurrency
	if concurrency <= 0 {
		concurrency = runtime.NumCPU()
	}
	if concurrency > len(configs) {
		concurrency = len(configs)
	}

	type task struct {
		cfg models.V2RayClientConfig
	}
	taskChan := make(chan task, total)
	for _, cfg := range configs {
		taskChan <- task{cfg: cfg}
	}
	close(taskChan)

	var wg sync.WaitGroup
	var currentCount int32
	var toUpdate []models.V2RayClientConfig
	var toUpdateMu sync.Mutex

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case t, ok := <-taskChan:
					if !ok {
						return
					}

					// Enforce per-IP rate-limiting/cooldown
					targetIP := getIP(t.cfg.Address)
					enforceIPCooldown(ctx, targetIP, opts.DelayBetweenSameIP)

					// Perform the test
					res := TestSingleConfig(ctx, t.cfg, opts)

					// Update latency DB (Pebble)
					latency := -1
					if res.OK {
						latency = res.RelayMs
					}
					toUpdateMu.Lock()
					if dbCfg, err := pebble.GetClientConfig(t.cfg.ID); err == nil {
						dbCfg.LatencyMs = latency
						toUpdate = append(toUpdate, *dbCfg)
					}
					toUpdateMu.Unlock()

					// Increment finished count and report
					done := int(atomic.AddInt32(&currentCount, 1))
					onProgress(ResultMessage{
						Type:    "result",
						Total:   total,
						Current: done,
						Result:  res,
					})
				}
			}
		}()
	}

	wg.Wait()

	// Save all latencies in bulk
	if len(toUpdate) > 0 {
		_ = pebble.SaveClientConfigsBulk(toUpdate)
	}

	// Send final status message
	status := "completed"
	if ctx.Err() != nil {
		status = "stopped"
	}
	onProgress(ResultMessage{
		Type:    "status",
		Status:  status,
		Total:   total,
		Current: int(currentCount),
	})

	return nil
}

func TestSingleConfig(ctx context.Context, cfg models.V2RayClientConfig, opts TestOptions) *ConfigTestResult {
	res := &ConfigTestResult{
		ConfigID:   cfg.ID,
		Name:       cfg.Name,
		PingMs:     -1,
		RelayMs:    -1,
		HTTPStatus: -1,
	}

	// Always measure raw TCP ping to the server
	t0 := time.Now()
	dialer := &net.Dialer{Timeout: opts.Timeout}
	conn, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(cfg.Address, strconv.Itoa(cfg.Port)))
	if err == nil {
		res.PingMs = int(time.Since(t0).Milliseconds())
		conn.Close()
	}

	if opts.TestType == "tcp_ping" {
		if err == nil {
			res.OK = true
			res.RelayMs = res.PingMs
		} else {
			res.Error = "TCP connection failed: " + err.Error()
		}
		return res
	}

	// Extract SNI
	var tlsMap map[string]interface{}
	_ = json.Unmarshal([]byte(cfg.TLSSettings), &tlsMap)
	sni := ""
	if tlsMap != nil {
		if s, ok := tlsMap["sni"].(string); ok && s != "" {
			sni = s
		}
	}
	if sni == "" {
		sni = cfg.Address
	}

	if opts.TestType == "tls_ping" {
		t0 = time.Now()
		tcpConn, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(cfg.Address, strconv.Itoa(cfg.Port)))
		if err != nil {
			res.Error = "TCP connection failed: " + err.Error()
			return res
		}
		defer tcpConn.Close()

		tlsConn := tls.Client(tcpConn, &tls.Config{
			ServerName:         sni,
			InsecureSkipVerify: true,
		})
		_ = tlsConn.SetDeadline(time.Now().Add(opts.Timeout))
		err = tlsConn.HandshakeContext(ctx)
		if err != nil {
			res.Error = "TLS handshake failed: " + err.Error()
			return res
		}

		res.OK = true
		res.RelayMs = int(time.Since(t0).Milliseconds())
		return res
	}

	if opts.TestType == "real_url" {
		coreName := opts.Core
		if coreName == "" || coreName == "current" {
			coreName = core.GetSelectedCoreName()
		}

		binPath, err := getBinPath(coreName)
		if err != nil {
			res.Error = err.Error()
			return res
		}

		socksPort, httpPort, err := reservePortPair()
		if err != nil {
			res.Error = "failed to reserve port pair: " + err.Error()
			return res
		}
		defer releasePortPair(socksPort, httpPort)

		configBytes, err := compiler.CompileClientConfigForCore(coreName, cfg, socksPort, httpPort, true, "")
		if err != nil {
			res.Error = "compile error: " + err.Error()
			return res
		}

		tempPath := filepath.Join(os.TempDir(), fmt.Sprintf("core_test_%d_%d.json", cfg.ID, socksPort))
		_ = os.WriteFile(tempPath, configBytes, 0644)
		defer os.Remove(tempPath)

		testCtx, cancel := context.WithTimeout(ctx, opts.Timeout+2*time.Second)
		defer cancel()

		if abs, err := filepath.Abs(binPath); err == nil {
			binPath = abs
		}

		var logBuf bytes.Buffer
		var cmd *exec.Cmd
		if coreName == "v2ray" {
			cmd = exec.CommandContext(testCtx, binPath, "-config", tempPath)
		} else {
			cmd = exec.CommandContext(testCtx, binPath, "run", "-c", tempPath)
		}
		cmd.Env = append(os.Environ(),
			"ENABLE_DEPRECATED_LEGACY_DNS_SERVERS=true",
			"ENABLE_DEPRECATED_MISSING_DOMAIN_RESOLVER=true",
			"ENABLE_DEPRECATED_SPECIAL_OUTBOUNDS=true",
		)
		cmd.Stdout = &logBuf
		cmd.Stderr = &logBuf
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		absBinDir, err := filepath.Abs(filepath.Dir(binPath))
		if err == nil {
			cmd.Dir = absBinDir
		}

		if err := cmd.Start(); err != nil {
			res.Error = "launch failed: " + err.Error()
			return res
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
			select {
			case <-testCtx.Done():
				res.Error = "timeout waiting for SOCKS port to open"
				return res
			default:
				time.Sleep(100 * time.Millisecond)
			}
		}

		if !ready {
			res.Error = "SOCKS port failed to open"
			if logStr := logBuf.String(); logStr != "" {
				res.Error += ": " + strings.TrimSpace(logStr)
			}
			return res
		}

		client := socksHTTPClient("127.0.0.1", socksPort, opts.Timeout)
		reqURL := opts.URL
		if reqURL == "" {
			reqURL = "http://www.gstatic.com/generate_204"
		}

		req, err := http.NewRequestWithContext(testCtx, "GET", reqURL, nil)
		if err != nil {
			res.Error = "request build failed: " + err.Error()
			return res
		}
		req.Header.Set("User-Agent", "Mozilla/5.0")

		t0 = time.Now()
		resp, err := client.Do(req)
		if err != nil {
			res.Error = "HTTP probe failed: " + err.Error()
			return res
		}
		defer resp.Body.Close()

		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))

		res.OK = true
		res.RelayMs = int(time.Since(t0).Milliseconds())
		res.HTTPStatus = resp.StatusCode
		res.Colo = detectColo("127.0.0.1", socksPort, 2*time.Second)
	}

	return res
}

func getIP(address string) string {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		host = address
	}
	ips, err := net.LookupIP(host)
	if err != nil || len(ips) == 0 {
		return host
	}
	return ips[0].String()
}

func enforceIPCooldown(ctx context.Context, ip string, delay time.Duration) {
	if delay <= 0 {
		return
	}
	lastIPAccessMu.Lock()
	lastTime, exists := lastIPAccess[ip]
	var waitTime time.Duration
	if exists {
		elapsed := time.Since(lastTime)
		if elapsed < delay {
			waitTime = delay - elapsed
		}
	}
	lastIPAccess[ip] = time.Now().Add(waitTime).Add(delay)
	lastIPAccessMu.Unlock()

	if waitTime > 0 {
		select {
		case <-time.After(waitTime):
		case <-ctx.Done():
		}
	}
}

func getBinPath(coreName string) (string, error) {
	return core.GetBinPathForCore(coreName)
}

// portLeaseMu guards the leased ports registry against concurrent tester goroutines.
var (
	portLeaseMu     sync.Mutex
	portLeasedPorts = make(map[int]struct{})
)

// reservePortPair atomically finds and reserves two free consecutive-ish ports
// (socksPort and httpPort) under the global mutex so concurrent testers never
// receive the same port. The caller MUST call releasePortPair when done.
func reservePortPair() (socksPort, httpPort int, err error) {
	portLeaseMu.Lock()
	defer portLeaseMu.Unlock()

	// Safe range outside local ephemeral ports (typically 32768-60999 on Linux)
	minPort := 20000
	maxPort := 30000

	reserveOne := func() (int, error) {
		for attempt := 0; attempt < 500; attempt++ {
			p := minPort + rand.Intn(maxPort-minPort+1)
			if _, leased := portLeasedPorts[p]; leased {
				continue
			}
			// Verify port is free to listen
			l, e := net.Listen("tcp", "127.0.0.1:"+strconv.Itoa(p))
			if e != nil {
				continue
			}
			l.Close()
			
			portLeasedPorts[p] = struct{}{}
			return p, nil
		}
		return 0, fmt.Errorf("could not find a free port after 500 attempts")
	}

	socksPort, err = reserveOne()
	if err != nil {
		return
	}
	httpPort, err = reserveOne()
	if err != nil {
		delete(portLeasedPorts, socksPort)
		return
	}
	return
}

// releasePortPair removes the two ports from the lease registry.
func releasePortPair(socksPort, httpPort int) {
	portLeaseMu.Lock()
	delete(portLeasedPorts, socksPort)
	delete(portLeasedPorts, httpPort)
	portLeaseMu.Unlock()
}

// getFreePort returns a single free port. For concurrent use, prefer reservePortPair.
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

func socksHTTPClient(socksHost string, socksPort int, timeout time.Duration) *http.Client {
	dial := func(_ context.Context, _, addr string) (net.Conn, error) {
		return socks5Dial(net.JoinHostPort(socksHost, strconv.Itoa(socksPort)), addr, timeout)
	}
	tr := &http.Transport{
		DialContext:           dial,
		DisableKeepAlives:     true,
		TLSHandshakeTimeout:   timeout,
		ResponseHeaderTimeout: timeout,
	}
	return &http.Client{Transport: tr, Timeout: timeout}
}

func socks5Dial(proxyAddr, target string, timeout time.Duration) (net.Conn, error) {
	host, portStr, err := net.SplitHostPort(target)
	if err != nil {
		return nil, err
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return nil, err
	}
	c, err := net.DialTimeout("tcp", proxyAddr, timeout)
	if err != nil {
		return nil, err
	}
	_ = c.SetDeadline(time.Now().Add(timeout))

	if _, err := c.Write([]byte{0x05, 0x01, 0x00}); err != nil {
		c.Close()
		return nil, err
	}
	rep := make([]byte, 2)
	if _, err := io.ReadFull(c, rep); err != nil {
		c.Close()
		return nil, err
	}
	if rep[0] != 0x05 || rep[1] != 0x00 {
		c.Close()
		return nil, fmt.Errorf("socks5: method rejected")
	}

	if len(host) > 255 {
		c.Close()
		return nil, errors.New("socks5: host too long")
	}
	buf := []byte{0x05, 0x01, 0x00, 0x03, byte(len(host))}
	buf = append(buf, host...)
	var pb [2]byte
	binary.BigEndian.PutUint16(pb[:], uint16(port))
	buf = append(buf, pb[:]...)
	if _, err := c.Write(buf); err != nil {
		c.Close()
		return nil, err
	}

	head := make([]byte, 4)
	if _, err := io.ReadFull(c, head); err != nil {
		c.Close()
		return nil, err
	}
	if head[1] != 0x00 {
		c.Close()
		return nil, fmt.Errorf("socks5: connect failed (code %d)", head[1])
	}
	var skip int
	switch head[3] {
	case 0x01:
		skip = 4
	case 0x04:
		skip = 16
	case 0x03:
		ln := make([]byte, 1)
		if _, err := io.ReadFull(c, ln); err != nil {
			c.Close()
			return nil, err
		}
		skip = int(ln[0])
	default:
		c.Close()
		return nil, errors.New("socks5: bad atyp")
	}
	if _, err := io.ReadFull(c, make([]byte, skip+2)); err != nil {
		c.Close()
		return nil, err
	}
	_ = c.SetDeadline(time.Time{})
	return c, nil
}

func detectColo(socksHost string, socksPort int, timeout time.Duration) string {
	c := socksHTTPClient(socksHost, socksPort, timeout)
	req, err := http.NewRequest("GET", "https://speed.cloudflare.com/cdn-cgi/trace", nil)
	if err != nil {
		return ""
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")
	resp, err := c.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
	for _, line := range strings.Split(string(body), "\n") {
		if v, ok := strings.CutPrefix(strings.TrimSpace(line), "colo="); ok {
			return v
		}
	}
	return ""
}
