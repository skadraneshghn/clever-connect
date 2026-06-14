package pebble

import "time"

// DNSMetricPayload stores aggregated metric results for query benchmarks inside PebbleDB
type DNSMetricPayload struct {
	IP               string    `json:"ip"`
	Protocol         string    `json:"protocol"`
	MinLatencyMs     float64   `json:"min_latency_ms"`
	AvgLatencyMs     float64   `json:"avg_latency_ms"`
	MaxLatencyMs     float64   `json:"max_latency_ms"`
	JitterMs         float64   `json:"jitter_ms"`
	PacketLossPct    float64   `json:"packet_loss_pct"`
	SuccessRatePct   float64   `json:"success_rate_pct"`
	QueryType        string    `json:"query_type"`
	DNSClass         string    `json:"dns_class"`
	Domain           string    `json:"domain"`
	DNSRebindingVuln bool      `json:"dns_rebinding_vuln"`
	LastChecked      time.Time `json:"last_checked"`
	ErrorMessage     string    `json:"error_message,omitempty"`
	ResolvedIP       string    `json:"resolved_ip,omitempty"`
	City             string    `json:"city,omitempty"`
	IsCDN            bool      `json:"is_cdn,omitempty"`
	CDNProvider      string    `json:"cdn_provider,omitempty"`
	ExpectedMatch    bool      `json:"expected_match,omitempty"`
}
