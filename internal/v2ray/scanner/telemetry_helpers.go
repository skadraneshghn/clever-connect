package scanner

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"clever-connect/internal/models"
	"clever-connect/internal/v2ray/compiler"

	box "github.com/sagernet/sing-box"
	"github.com/sagernet/sing-box/include"
	boxOption "github.com/sagernet/sing-box/option"
)

// trackingReader counts the bytes read from the underlying reader
type trackingReader struct {
	r          io.Reader
	ctx        context.Context
	totalBytes *int64
}

func (tr *trackingReader) Read(p []byte) (int, error) {
	if err := tr.ctx.Err(); err != nil {
		return 0, err
	}
	n, err := tr.r.Read(p)
	if n > 0 {
		atomic.AddInt64(tr.totalBytes, int64(n))
	}
	return n, err
}

// probeCdnPop performs a direct TLS/HTTP handshake to identify the CDN provider and POP location
func probeCdnPop(ctx context.Context, ip net.IP, port int, sni string, timeout time.Duration) (string, string, error) {
	// 1. Check registry first
	cdnProvider, _ := GlobalCDNRegistry.Lookup(ip)

	// 2. Build direct TLS connection with custom Host header and SNI
	dialer := &net.Dialer{
		Timeout: timeout,
	}

	transport := &http.Transport{
		DialContext: func(c context.Context, network, addr string) (net.Conn, error) {
			targetAddr := net.JoinHostPort(ip.String(), strconv.Itoa(port))
			return dialer.DialContext(c, network, targetAddr)
		},
		TLSClientConfig: &tls.Config{
			ServerName:         sni,
			InsecureSkipVerify: true,
		},
		DisableKeepAlives: true,
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   timeout,
	}

	// Try HEAD first, fallback to GET
	urlStr := fmt.Sprintf("https://%s/", sni)
	req, err := http.NewRequestWithContext(ctx, "HEAD", urlStr, nil)
	if err != nil {
		return cdnProvider, "", err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")

	resp, err := client.Do(req)
	if err != nil {
		reqGET, errGET := http.NewRequestWithContext(ctx, "GET", urlStr, nil)
		if errGET == nil {
			resp, err = client.Do(reqGET)
		}
	}

	if err != nil {
		return cdnProvider, "", err
	}
	defer resp.Body.Close()

	popLocation := ""

	// Parse Cloudflare
	if cfRay := resp.Header.Get("CF-Ray"); cfRay != "" {
		if cdnProvider == "" {
			cdnProvider = "Cloudflare"
		}
		parts := strings.Split(cfRay, "-")
		if len(parts) > 1 {
			popLocation = strings.ToUpper(parts[len(parts)-1])
		}
	}

	// Parse CloudFront
	if amzCfPop := resp.Header.Get("X-Amz-Cf-Pop"); amzCfPop != "" {
		if cdnProvider == "" {
			cdnProvider = "AWS CloudFront"
		}
		if len(amzCfPop) >= 3 {
			popLocation = strings.ToUpper(amzCfPop[:3])
		}
	}

	// Parse Fastly
	if fastlyDebug := resp.Header.Get("Fastly-Debug"); fastlyDebug != "" {
		if cdnProvider == "" {
			cdnProvider = "Fastly"
		}
		parts := strings.Split(fastlyDebug, "-")
		if len(parts) > 1 {
			popLocation = strings.ToUpper(parts[len(parts)-1])
		}
	} else if servedBy := resp.Header.Get("X-Served-By"); servedBy != "" {
		if cdnProvider == "" {
			cdnProvider = "Fastly"
		}
		parts := strings.Split(servedBy, "-")
		if len(parts) > 1 {
			popLocation = strings.ToUpper(parts[len(parts)-1])
		}
	}

	// Parse Bunny
	if edgeLoc := resp.Header.Get("X-Edge-Location"); edgeLoc != "" {
		if cdnProvider == "" {
			cdnProvider = "Bunny CDN"
		}
		popLocation = strings.ToUpper(edgeLoc)
	}

	// Parse Gcore
	if gcoreEdge := resp.Header.Get("X-Gcore-Edge"); gcoreEdge != "" {
		if cdnProvider == "" {
			cdnProvider = "Gcore"
		}
		popLocation = strings.ToUpper(gcoreEdge)
	}

	// Parse CDN77
	if cdn77Served := resp.Header.Get("X-CDN77-Served-By"); cdn77Served != "" {
		if cdnProvider == "" {
			cdnProvider = "CDN77"
		}
	}

	// Parse general Server header
	serverHeader := resp.Header.Get("Server")
	if cdnProvider == "" {
		if strings.Contains(strings.ToLower(serverHeader), "cloudflare") {
			cdnProvider = "Cloudflare"
		} else if strings.Contains(strings.ToLower(serverHeader), "bunnycdn") {
			cdnProvider = "Bunny CDN"
		} else if strings.Contains(strings.ToLower(serverHeader), "gcore") {
			cdnProvider = "Gcore"
		} else if strings.Contains(strings.ToLower(serverHeader), "akamai") {
			cdnProvider = "Akamai"
		}
	}

	return cdnProvider, popLocation, nil
}

// testProxyThroughput performs an actual download speed test via sing-box SOCKS5 proxy
func testProxyThroughput(ctx context.Context, baseConfig models.V2RayClientConfig, ip string, port int, downloadDuration time.Duration) (int, float64, error) {
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

	dial := func(c context.Context, _, addr string) (net.Conn, error) {
		return socks5DialContext(c, socksAddr, addr)
	}

	client := &http.Client{
		Transport: &http.Transport{
			DialContext:           dial,
			DisableKeepAlives:     true,
			TLSHandshakeTimeout:   2 * time.Second,
			ResponseHeaderTimeout: 2 * time.Second,
		},
		Timeout: 15 * time.Second,
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

	// Real 10-second download speed test
	downURL := "https://speed.cloudflare.com/__down?bytes=50000000" // 50MB
	speedCtx, speedCancel := context.WithTimeout(ctx, downloadDuration)
	defer speedCancel()

	reqDown, err := http.NewRequestWithContext(speedCtx, "GET", downURL, nil)
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

	var totalBytes int64
	tracker := &trackingReader{
		r:          respDown.Body,
		ctx:        speedCtx,
		totalBytes: &totalBytes,
	}

	buf := make([]byte, 32768)
	for {
		_, errRead := tracker.Read(buf)
		if errRead != nil {
			break
		}
	}

	elapsed := time.Since(tDownStart).Seconds()
	var speedMBps float64
	if elapsed > 0 && totalBytes > 0 {
		speedMBps = (float64(totalBytes) / 1024.0 / 1024.0) / elapsed
	}

	return ttfb, speedMBps, nil
}
