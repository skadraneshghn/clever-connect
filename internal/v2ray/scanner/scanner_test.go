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
	_, ipNet1, _ := net.ParseCIDR("192.168.1.0/24")
	targetNets := []*net.IPNet{ipNet1}

	ip := net.ParseIP("192.168.1.10")
	neighbors := NeighborsAround(ip, targetNets)
	if len(neighbors) != 10 {
		t.Fatalf("expected 10 neighbors, got %d", len(neighbors))
	}
	for _, n := range neighbors {
		val := n.String()
		if val == "192.168.1.10" {
			t.Errorf("neighbor should not include the ip itself")
		}
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
