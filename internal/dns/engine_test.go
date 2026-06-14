package dns

import (
	"context"
	"net"
	"testing"
	"time"

	"clever-connect/internal/models"
	"github.com/miekg/dns"
)

// StartMockDNSServer boots a mock DNS server locally for testing.
func StartMockDNSServer(t *testing.T, handler dns.Handler) (*dns.Server, string) {
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen on udp: %v", err)
	}
	addr := pc.LocalAddr().String()
	_, port, _ := net.SplitHostPort(addr)

	server := &dns.Server{
		PacketConn: pc,
		Handler:    handler,
	}

	go func() {
		if err := server.ActivateAndServe(); err != nil {
			t.Logf("server terminated: %v", err)
		}
	}()

	return server, net.JoinHostPort("127.0.0.1", port)
}

func TestQueryUDPAndDiagnostic(t *testing.T) {
	// 1. Setup handler to mock A and AAAA record answers
	handler := dns.HandlerFunc(func(w dns.ResponseWriter, r *dns.Msg) {
		msg := new(dns.Msg)
		msg.SetReply(r)
		msg.Authoritative = true

		for _, q := range r.Question {
			if q.Qtype == dns.TypeA {
				rr, err := dns.NewRR(q.Name + " 3600 IN A 1.2.3.4")
				if err == nil {
					msg.Answer = append(msg.Answer, rr)
				}
			}
		}

		_ = w.WriteMsg(msg)
	})

	server, serverAddr := StartMockDNSServer(t, handler)
	defer server.Shutdown()

	engine := GetEngine()

	// 2. Set up test config
	config := &models.DNSTesterConfig{
		ConcurrencyLimit: 2,
		Attempts:         1,
		TimeoutMs:        1000,
		ReferenceDomain:  "google.com",
		QueryTypes:       []string{"A"},
		DNSClass:         "IN",
		QueryGenerator:   "static",
		DomainSource:     "default",
	}

	job := DNSTestJob{
		IP:           serverAddr,
		Protocol:     "udp",
		ProviderName: "MockProvider",
		Category:     "public",
		QueryType:    "A",
		DNSClass:     "IN",
		Domain:       "google.com",
		Config:       config,
	}

	ctx := context.Background()
	res := engine.runSingleDiagnostic(ctx, job)

	if res.CleverScore == 0 {
		t.Errorf("expected non-zero clever score, got %d", res.CleverScore)
	}

	if res.SuccessRatePct < 100 {
		t.Errorf("expected 100%% success rate, got %f", res.SuccessRatePct)
	}

	if res.DNSRebindingVuln {
		t.Errorf("expected no DNS rebinding vulnerability, but got vulnerable")
	}
}

func TestDNSRebindingVulnDetection(t *testing.T) {
	// Mock a DNS answer returning a private local IP to trigger DNS rebinding warning
	handler := dns.HandlerFunc(func(w dns.ResponseWriter, r *dns.Msg) {
		msg := new(dns.Msg)
		msg.SetReply(r)
		msg.Authoritative = true

		for _, q := range r.Question {
			if q.Qtype == dns.TypeA {
				// 127.0.0.1 is local/loopback and should flag rebinding warning
				rr, err := dns.NewRR(q.Name + " 3600 IN A 127.0.0.1")
				if err == nil {
					msg.Answer = append(msg.Answer, rr)
				}
			}
		}

		_ = w.WriteMsg(msg)
	})

	server, serverAddr := StartMockDNSServer(t, handler)
	defer server.Shutdown()

	engine := GetEngine()

	config := &models.DNSTesterConfig{
		ConcurrencyLimit: 1,
		Attempts:         1,
		TimeoutMs:        1000,
		ReferenceDomain:  "google.com",
		QueryTypes:       []string{"A"},
		DNSClass:         "IN",
		QueryGenerator:   "static",
		DomainSource:     "default",
	}

	job := DNSTestJob{
		IP:           serverAddr,
		Protocol:     "udp",
		ProviderName: "RebindingMock",
		Category:     "public",
		QueryType:    "A",
		DNSClass:     "IN",
		Domain:       "google.com",
		Config:       config,
	}

	ctx := context.Background()
	res := engine.runSingleDiagnostic(ctx, job)

	if !res.DNSRebindingVuln {
		t.Errorf("expected DNS rebinding vulnerability warning to be flagged")
	}
}

func TestDNSFullSweepBroadcast(t *testing.T) {
	handler := dns.HandlerFunc(func(w dns.ResponseWriter, r *dns.Msg) {
		msg := new(dns.Msg)
		msg.SetReply(r)
		msg.Authoritative = true
		for _, q := range r.Question {
			if q.Qtype == dns.TypeA {
				rr, err := dns.NewRR(q.Name + " 3600 IN A 9.9.9.9")
				if err == nil {
					msg.Answer = append(msg.Answer, rr)
				}
			}
		}
		_ = w.WriteMsg(msg)
	})

	server, serverAddr := StartMockDNSServer(t, handler)
	defer server.Shutdown()

	engine := GetEngine()

	var startedReceived bool
	var finishedReceived bool
	var candidatesReceived int

	engine.RegisterListener("test_listener", func(stats DNSJobStats, event string, details interface{}) {
		switch event {
		case "dns.started":
			startedReceived = true
		case "dns.finished":
			finishedReceived = true
		case "dns.candidate":
			if list, ok := details.([]DNSTestResult); ok {
				candidatesReceived += len(list)
			}
		}
	})
	defer engine.UnregisterListener("test_listener")

	config := &models.DNSTesterConfig{
		ConcurrencyLimit: 2,
		Attempts:         1,
		TimeoutMs:        1000,
		ReferenceDomain:  "google.com",
		QueryTypes:       []string{"A"},
		DNSClass:         "IN",
		QueryGenerator:   "static",
		DomainSource:     "default",
	}

	customResolvers := []models.DNSResolver{
		{
			IP:           serverAddr,
			Protocol:     "udp",
			ProviderName: "TestSweepProvider",
			SupportUDP:   true,
		},
	}

	err := engine.StartTest(config, customResolvers, []string{"udp"})
	if err != nil {
		t.Fatalf("failed to start test sweep: %v", err)
	}

	// Wait up to 2 seconds for sweep to complete
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if !engine.IsTesting() {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	if !startedReceived {
		t.Errorf("expected to receive dns.started event")
	}
	if !finishedReceived {
		t.Errorf("expected to receive dns.finished event")
	}
	if candidatesReceived == 0 {
		t.Errorf("expected to receive at least one dns.candidate result")
	}
}

func TestDNSTraceAndAXFR(t *testing.T) {
	handler := dns.HandlerFunc(func(w dns.ResponseWriter, r *dns.Msg) {
		msg := new(dns.Msg)
		msg.SetReply(r)
		msg.Authoritative = true
		for _, q := range r.Question {
			if q.Qtype == dns.TypeA {
				rr, err := dns.NewRR(q.Name + " 3600 IN A 9.9.9.9")
				if err == nil {
					msg.Answer = append(msg.Answer, rr)
				}
			} else if q.Qtype == dns.TypeAXFR {
				rr, err := dns.NewRR(q.Name + " 3600 IN A 9.9.9.9")
				if err == nil {
					msg.Answer = append(msg.Answer, rr)
				}
			}
		}
		_ = w.WriteMsg(msg)
	})

	server, serverAddr := StartMockDNSServer(t, handler)
	defer server.Shutdown()

	engine := GetEngine()

	// 1. Test TraceDNS
	steps, err := engine.TraceDNS(context.Background(), "google.com", serverAddr)
	if err != nil {
		t.Errorf("failed TraceDNS: %v", err)
	}
	if len(steps) == 0 {
		t.Errorf("expected at least one step in TraceDNS")
	}

	// 2. Test TestAXFR
	res, err := engine.TestAXFR(context.Background(), "google.com", serverAddr)
	if err != nil {
		t.Errorf("failed TestAXFR: %v", err)
	}
	if res == nil {
		t.Errorf("expected non-nil AXFR result")
	}
}
