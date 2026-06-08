package scanner

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"
	"time"
)

func TestNeighborsAround(t *testing.T) {
	// Initialize with test networks
	cfIPNets = nil
	_, ipNet1, _ := net.ParseCIDR("192.168.1.0/24")
	cfIPNets = append(cfIPNets, ipNet1)

	ip := net.ParseIP("192.168.1.10")
	neighbors := NeighborsAround(ip, 5, 2)
	if len(neighbors) != 2 {
		t.Fatalf("expected 2 neighbors, got %d", len(neighbors))
	}
	n1 := neighbors[0].String()
	n2 := neighbors[1].String()
	if n1 != "192.168.1.11" && n1 != "192.168.1.9" {
		t.Errorf("unexpected neighbor: %s", n1)
	}
	if n2 != "192.168.1.11" && n2 != "192.168.1.9" {
		t.Errorf("unexpected neighbor: %s", n2)
	}
}

func TestScannerEngineStartStop(t *testing.T) {
	engine := GetEngine()
	if engine.IsRunning() {
		t.Fatal("engine should not be running initially")
	}

	// Setup a mock HTTPS server
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Add delay to ensure engine is running when checked
		time.Sleep(300 * time.Millisecond)
		if r.URL.Path == "/cdn-cgi/trace" {
			_, _ = w.Write([]byte("h=speed.cloudflare.com\ncolo=TEST\n"))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	// Disable verification of mock server certificates for the client inside probeHTTP
	http.DefaultTransport.(*http.Transport).TLSClientConfig = &tls.Config{InsecureSkipVerify: true}

	u, err := url.Parse(ts.URL)
	if err != nil {
		t.Fatal(err)
	}
	host, portStr, err := net.SplitHostPort(u.Host)
	if err != nil {
		t.Fatal(err)
	}
	port, _ := strconv.Atoi(portStr)

	cfg := &ScanConfig{
		TargetCIDRs:      []string{host},
		SelectedPorts:    []int{port},
		ConcurrencyLimit: 2,
		MaxRateLimit:     0,
		NetworkTimeout:   2 * time.Second,
		ProbeAttempts:    1,
		TargetMode:       "http",
		TargetSNI:        host,
		RequireWS:        false,
		EnableNeighbors:  false,
		TopLimit:         10,
	}

	err = engine.StartScan(context.Background(), cfg)
	if err != nil {
		t.Fatalf("failed to start scan: %v", err)
	}

	// Wait briefly and verify status is running
	time.Sleep(100 * time.Millisecond)
	if !engine.IsRunning() {
		t.Fatal("engine should be running during the delay")
	}

	engine.CancelActiveScan()
	time.Sleep(50 * time.Millisecond)
	if engine.IsRunning() {
		t.Fatal("engine should have stopped after cancel")
	}
}
