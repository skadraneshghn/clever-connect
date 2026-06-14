package dns

import (
	"time"

	"clever-connect/internal/models"
)

// DNSJobStats represents the live metrics and execution state of the active run
type DNSJobStats struct {
	Tested       int64  `json:"tested"`
	Healthy      int64  `json:"healthy"`
	Failed       int64  `json:"failed"`
	InFlight     int64  `json:"in_flight"`
	TotalTargets int64  `json:"total_targets"`
	RemainingSec int64  `json:"remaining_sec"`
	Phase        string `json:"phase"`
}

// DNSTestJob encapsulates a target server and settings for execution
type DNSTestJob struct {
	IP           string
	Protocol     string
	ProviderName string
	Category     string
	QueryType    string
	DNSClass     string
	Domain       string
	Config       *models.DNSTesterConfig
}

// DNSTestResult maps the performance, reliability, and security diagnostics of a run
type DNSTestResult struct {
	IP               string    `json:"ip"`
	Protocol         string    `json:"protocol"`
	ProviderName     string    `json:"provider_name"`
	Category         string    `json:"category"`
	LatencyMs        int64     `json:"latency_ms"`
	JitterMs         float64   `json:"jitter_ms"`
	PacketLossPct    float64   `json:"packet_loss_pct"`
	SuccessRatePct   float64   `json:"success_rate_pct"`
	Censorship       string    `json:"censorship"` // "clean", "hijacked", "manipulated", "sinkhole", "unverified"
	DNSSECValid      bool      `json:"dnssec_valid"`
	DNSRebindingVuln bool      `json:"dns_rebinding_vuln"`
	QueryType        string    `json:"query_type"`
	DNSClass         string    `json:"dns_class"`
	Domain           string    `json:"domain"`
	CleverScore      int       `json:"clever_score"`
	CheckedAt        time.Time `json:"checked_at"`
	Error            string    `json:"error,omitempty"`
	ResolvedIP       string    `json:"resolved_ip,omitempty"`
	CountryCode      string    `json:"country_code,omitempty"`
	CountryName      string    `json:"country_name,omitempty"`
	City             string    `json:"city,omitempty"`
	ISP              string    `json:"isp,omitempty"`
	IsCDN            bool      `json:"is_cdn,omitempty"`
	CDNProvider      string    `json:"cdn_provider,omitempty"`
	ExpectedMatch    bool      `json:"expected_match,omitempty"`
}

// DNSTraceStep represents a single hop in the iterative DNS delegation path
type DNSTraceStep struct {
	Hop        int    `json:"hop"`
	ServerIP   string `json:"server_ip"`
	ServerName string `json:"server_name"`
	LatencyMs  int64  `json:"latency_ms"`
	Rcode      string `json:"rcode"`
	Delegated  string `json:"delegated_to"`
}

// DNSAXFRResult holds the outcome of a zone transfer vulnerability test
type DNSAXFRResult struct {
	ResolverIP   string   `json:"resolver_ip"`
	Domain       string   `json:"domain"`
	Allowed      bool     `json:"allowed"`
	RecordsCount int      `json:"records_count"`
	Records      []string `json:"records"`
	Error        string   `json:"error,omitempty"`
}
