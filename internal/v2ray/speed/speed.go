package speed

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptrace"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"clever-connect/internal/models"
	"clever-connect/internal/v2ray/compiler"
	"clever-connect/internal/v2ray/core"
)

// ProfileTestResult represents the diagnosis result of a proxy config
type ProfileTestResult struct {
	ConfigID      uint   `json:"config_id"`
	Name          string `json:"name"`
	OK            bool   `json:"ok"`
	PingMs        int    `json:"ping_ms"` // TCP ping to server
	RelayMs       int    `json:"relay_ms"` // HTTP probe time
	DownKbps      int    `json:"down_kbps"`
	UpKbps        int    `json:"up_kbps"`
	HTTPStatus    int    `json:"http_status"`
	Colo          string `json:"colo"` // Cloudflare colo datacenter
	Error         string `json:"error"`
}

type fakeReader struct {
	left int
	buf  [32 * 1024]byte
}

func (r *fakeReader) Read(p []byte) (int, error) {
	if r.left <= 0 {
		return 0, io.EOF
	}
	n := len(p)
	if n > r.left {
		n = r.left
	}
	if n > len(r.buf) {
		n = len(r.buf)
	}
	if r.buf[0] == 0 {
		_, _ = rand.Read(r.buf[:])
	}
	copy(p, r.buf[:n])
	r.left -= n
	return n, nil
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

func fetchThroughSocks(socksHost string, socksPort int, url string, timeout time.Duration) (int, int, error) {
	client := socksHTTPClient(socksHost, socksPort, timeout)
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
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	return resp.StatusCode, len(body), nil
}

// MeasureDownload measures downstream speed in kbps
func MeasureDownload(socksHost string, socksPort, wantBytes int, timeout time.Duration) (int, error) {
	if wantBytes <= 0 {
		wantBytes = 2_000_000
	}
	url := "https://speed.cloudflare.com/__down?bytes=" + strconv.Itoa(wantBytes)
	c := socksHTTPClient(socksHost, socksPort, timeout)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")
	t0 := time.Now()
	resp, err := c.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	n, err := io.Copy(io.Discard, resp.Body)
	elapsed := time.Since(t0).Seconds()
	if err != nil && n == 0 {
		return 0, err
	}
	if elapsed <= 0 || n <= 0 {
		return 0, errors.New("no bytes received")
	}
	return int(float64(n*8) / elapsed / 1000), nil
}

// MeasureUpload measures upstream speed in kbps
func MeasureUpload(socksHost string, socksPort, sendBytes int, timeout time.Duration) (int, error) {
	if sendBytes <= 0 {
		sendBytes = 1_000_000
	}
	url := "https://speed.cloudflare.com/__up"
	c := socksHTTPClient(socksHost, socksPort, timeout)
	reader := &fakeReader{left: sendBytes}
	req, err := http.NewRequest("POST", url, reader)
	if err != nil {
		return 0, err
	}
	req.ContentLength = int64(sendBytes)
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("User-Agent", "Mozilla/5.0")
	t0 := time.Now()
	resp, err := c.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	elapsed := time.Since(t0).Seconds()
	if elapsed <= 0 {
		return 0, errors.New("zero elapsed")
	}
	return int(float64(sendBytes*8) / elapsed / 1000), nil
}

// DetectColo retrieves Cloudflare trace information
func DetectColo(socksHost string, socksPort int, timeout time.Duration) string {
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

// TestProfile runs a short-lived xray client to measure connectivity and speed
func TestProfile(cfg models.V2RayClientConfig, socksPort, httpPort int, measureSpeed bool, timeoutSec int) ProfileTestResult {
	res := ProfileTestResult{ConfigID: cfg.ID, Name: cfg.Name, PingMs: -1, RelayMs: -1, DownKbps: -1, UpKbps: -1}

	binPath := core.GetXrayBinPath()
	if _, err := os.Stat(binPath); err != nil {
		coreName := core.GetSelectedCoreName()
		if path, err := exec.LookPath(coreName); err == nil {
			binPath = path
		} else {
			res.Error = fmt.Sprintf("%s binary not found", coreName)
			return res
		}
	}

	// Direct TCP ping to proxy server first
	t0 := time.Now()
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(cfg.Address, strconv.Itoa(cfg.Port)), 3*time.Second)
	if err == nil {
		res.PingMs = int(time.Since(t0).Milliseconds())
		conn.Close()
	}

	configBytes, err := compiler.CompileClientConfig(cfg, socksPort, httpPort, true, "")
	if err != nil {
		res.Error = "config compile error: " + err.Error()
		return res
	}

	tempPath := filepath.Join(os.TempDir(), fmt.Sprintf("xray_test_%d_%d.json", cfg.ID, socksPort))
	_ = os.WriteFile(tempPath, configBytes, 0644)
	defer os.Remove(tempPath)

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutSec+10)*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, binPath, "run", "-c", tempPath)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	absBinDir, err := filepath.Abs(filepath.Dir(binPath))
	if err == nil {
		cmd.Dir = absBinDir
	}

	if err := cmd.Start(); err != nil {
		res.Error = "xray launch failed: " + err.Error()
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

	// Wait SOCKS ready
	socksAddr := net.JoinHostPort("127.0.0.1", strconv.Itoa(socksPort))
	ready := false
	deadline := time.Now().Add(6 * time.Second)
	for time.Now().Before(deadline) {
		if c, e := net.DialTimeout("tcp", socksAddr, 200*time.Millisecond); e == nil {
			c.Close()
			ready = true
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	if !ready {
		res.Error = "xray SOCKS port failed to open"
		return res
	}

	time.Sleep(200 * time.Millisecond)
	medianMs, err := MeasureMedianRTT("127.0.0.1", socksPort, "http://www.gstatic.com/generate_204", 3*time.Second)
	if err != nil {
		res.Error = "Latency probe failed: " + err.Error()
		return res
	}

	res.RelayMs = medianMs
	res.HTTPStatus = 204
	res.OK = true

	if res.OK {
		res.Colo = DetectColo("127.0.0.1", socksPort, 3*time.Second)
		if measureSpeed {
			spTimeout := time.Duration(timeoutSec) * time.Second
			if down, err := MeasureDownload("127.0.0.1", socksPort, 2_000_000, spTimeout); err == nil {
				res.DownKbps = down
			}
			if up, err := MeasureUpload("127.0.0.1", socksPort, 1_000_000, spTimeout); err == nil {
				res.UpKbps = up
			}
		}
	}

	return res
}

// MeasureMedianRTT runs 3 probes to a test URL via a proxy and takes the median RTT
func MeasureMedianRTT(socksHost string, socksPort int, testURL string, timeout time.Duration) (int, error) {
	client := socksHTTPClient(socksHost, socksPort, timeout)
	var rtts []time.Duration
	successCount := 0

	for i := 0; i < 3; i++ {
		t0 := time.Now()
		req, err := http.NewRequest("HEAD", testURL, nil)
		if err != nil {
			rtts = append(rtts, timeout)
			continue
		}
		req.Header.Set("User-Agent", "Mozilla/5.0")
		resp, err := client.Do(req)
		if err == nil {
			rtts = append(rtts, time.Since(t0))
			resp.Body.Close()
			successCount++
		} else {
			rtts = append(rtts, timeout)
		}
	}

	if successCount == 0 {
		return -1, fmt.Errorf("all probes failed")
	}

	// Sort RTTs (3 items)
	if rtts[0] > rtts[1] {
		rtts[0], rtts[1] = rtts[1], rtts[0]
	}
	if rtts[1] > rtts[2] {
		rtts[1], rtts[2] = rtts[2], rtts[1]
	}
	if rtts[0] > rtts[1] {
		rtts[0], rtts[1] = rtts[1], rtts[0]
	}

	median := rtts[1]
	if median >= timeout && successCount < 2 {
		return -1, fmt.Errorf("majority of probes failed")
	}

	return int(median.Milliseconds()), nil
}

// MassTestProfiles runs tests concurrently across multiple profiles
func MassTestProfiles(configs []models.V2RayClientConfig, concurrency int, measureSpeed bool, timeoutSec int) []ProfileTestResult {
	if concurrency <= 0 {
		concurrency = 3
	}
	if timeoutSec <= 0 {
		timeoutSec = 10
	}

	results := make([]ProfileTestResult, len(configs))
	var wg sync.WaitGroup
	sem := make(chan struct{}, concurrency)

	var basePort int64 = 23000

	for i, cfg := range configs {
		wg.Add(1)
		sem <- struct{}{}
		go func(idx int, c models.V2RayClientConfig) {
			defer wg.Done()
			defer func() { <-sem }()

			portOffset := int(atomic.AddInt64(&basePort, 2))
			results[idx] = TestProfile(c, portOffset, portOffset+1, measureSpeed, timeoutSec)
		}(i, cfg)
	}

	wg.Wait()
	return results
}

// SpeedTestBreakdown contains detailed timing and throughput metrics
type SpeedTestBreakdown struct {
	TotalMbps       float64 `json:"total_mbps"`
	DNSResolveMs    int64   `json:"dns_resolve_ms"`
	TCPConnectMs    int64   `json:"tcp_connect_ms"`
	TLSHandshakeMs  int64   `json:"tls_handshake_ms"`
	FirstByteMs     int64   `json:"first_byte_ms"`
	DownloadTimeMs  int64   `json:"download_time_ms"`
	TotalTimeMs     int64   `json:"total_time_ms"`
	DownloadedBytes int64   `json:"downloaded_bytes"`
	Colo            string  `json:"colo"`
}

// RunSpeedTestWithBreakdown downloads a file through the local proxy and measures timings using httptrace
func RunSpeedTestWithBreakdown(socksPort int, sizeBytes int, timeout time.Duration) (SpeedTestBreakdown, error) {
	var breakdown SpeedTestBreakdown
	if sizeBytes <= 0 {
		sizeBytes = 10_000_000 // 10MB default
	}

	testURL := fmt.Sprintf("https://speed.cloudflare.com/__down?bytes=%d", sizeBytes)
	client := socksHTTPClient("127.0.0.1", socksPort, timeout)

	var dnsStart, dnsDone time.Time
	var connStart, connDone time.Time
	var tlsStart, tlsDone time.Time
	var gotFirstByte time.Time
	t0 := time.Now()

	trace := &httptrace.ClientTrace{
		DNSStart: func(_ httptrace.DNSStartInfo) {
			dnsStart = time.Now()
		},
		DNSDone: func(_ httptrace.DNSDoneInfo) {
			dnsDone = time.Now()
		},
		ConnectStart: func(_, _ string) {
			connStart = time.Now()
		},
		ConnectDone: func(_, _ string, _ error) {
			connDone = time.Now()
		},
		TLSHandshakeStart: func() {
			tlsStart = time.Now()
		},
		TLSHandshakeDone: func(_ tls.ConnectionState, _ error) {
			tlsDone = time.Now()
		},
		GotFirstResponseByte: func() {
			gotFirstByte = time.Now()
		},
	}

	req, err := http.NewRequest("GET", testURL, nil)
	if err != nil {
		return breakdown, err
	}
	req = req.WithContext(httptrace.WithClientTrace(req.Context(), trace))
	req.Header.Set("User-Agent", "Mozilla/5.0")

	resp, err := client.Do(req)
	if err != nil {
		return breakdown, err
	}
	defer resp.Body.Close()

	tBodyStart := time.Now()
	n, err := io.Copy(io.Discard, resp.Body)
	tBodyEnd := time.Now()

	totalTime := time.Since(t0)

	if dnsDone.After(dnsStart) {
		breakdown.DNSResolveMs = dnsDone.Sub(dnsStart).Milliseconds()
	}
	if connDone.After(connStart) {
		breakdown.TCPConnectMs = connDone.Sub(connStart).Milliseconds()
	}
	if tlsDone.After(tlsStart) {
		breakdown.TLSHandshakeMs = tlsDone.Sub(tlsStart).Milliseconds()
	}
	if gotFirstByte.After(t0) {
		breakdown.FirstByteMs = gotFirstByte.Sub(t0).Milliseconds()
	}
	breakdown.DownloadTimeMs = tBodyEnd.Sub(tBodyStart).Milliseconds()
	breakdown.TotalTimeMs = totalTime.Milliseconds()
	breakdown.DownloadedBytes = n

	if totalTime.Seconds() > 0 && n > 0 {
		breakdown.TotalMbps = (float64(n*8) / totalTime.Seconds()) / 1_000_000.0
	}
	breakdown.Colo = DetectColo("127.0.0.1", socksPort, 3*time.Second)

	return breakdown, nil
}
