package dns

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"math"
	"net"
	"strings"

	"github.com/miekg/dns"
)

// GenerateCacheBustingDomain builds a unique high-entropy subdomain
func GenerateCacheBustingDomain(baseDomain string) string {
	randomBytes := make([]byte, 8)
	_, _ = rand.Read(randomBytes)
	randHex := hex.EncodeToString(randomBytes)
	return fmt.Sprintf("%s.%s", randHex, baseDomain)
}

// CheckNXDOMAINHijack queries an invalid hostname to verify if the resolver spoofs NXDOMAIN results.
// Standard clean resolvers must return RCODE 3 (NXDOMAIN) or empty response.
func CheckNXDOMAINHijack(resp *dns.Msg) bool {
	if resp == nil {
		return false
	}
	// If the server returns success (NOERROR) and provides IP records for a clearly non-existent random domain,
	// then it is hijacking/manipulating queries.
	if resp.Rcode == dns.RcodeSuccess && len(resp.Answer) > 0 {
		for _, rr := range resp.Answer {
			if _, ok := rr.(*dns.A); ok {
				return true
			}
			if _, ok := rr.(*dns.AAAA); ok {
				return true
			}
		}
	}
	return false
}

// CheckDNSSECValidation examines if a response has valid DNSSEC keys/signatures when DO bit was set
func CheckDNSSECValidation(resp *dns.Msg) bool {
	if resp == nil {
		return false
	}

	// Check if AD (Authenticated Data) flag is set
	if resp.AuthenticatedData {
		return true
	}

	// Scan Answer and Extra sections for RRSIG (Resource Record Signature) signatures
	for _, rr := range resp.Answer {
		if rr.Header().Rrtype == dns.TypeRRSIG {
			return true
		}
	}
	for _, rr := range resp.Ns {
		if rr.Header().Rrtype == dns.TypeRRSIG {
			return true
		}
	}
	for _, rr := range resp.Extra {
		if rr.Header().Rrtype == dns.TypeRRSIG {
			return true
		}
	}

	return false
}

// IsTelemetrySinkhole checks if a response to a tracking/telemetry domain was sinkholed (blocked)
func IsTelemetrySinkhole(resp *dns.Msg) bool {
	if resp == nil {
		return false
	}

	// Sinkholed response typically has Rcode success with NULL IP (0.0.0.0 / 127.0.0.1) or Refused/NXDomain
	if resp.Rcode == dns.RcodeNameError || resp.Rcode == dns.RcodeRefused {
		return true
	}

	if resp.Rcode == dns.RcodeSuccess {
		if len(resp.Answer) == 0 {
			return true
		}
		for _, rr := range resp.Answer {
			if a, ok := rr.(*dns.A); ok {
				ipStr := a.A.String()
				if ipStr == "0.0.0.0" || ipStr == "127.0.0.1" || strings.HasPrefix(ipStr, "10.0.") || strings.HasPrefix(ipStr, "192.168.") {
					return true
				}
			}
			if aaaa, ok := rr.(*dns.AAAA); ok {
				ipStr := aaaa.AAAA.String()
				if ipStr == "::" || ipStr == "::1" {
					return true
				}
			}
		}
	}

	return false
}

// CalculateCleverScore implements the composite scoring matrix for resolver ranking
func CalculateCleverScore(latencyMs int64, jitterMs float64, packetLossPct float64, censorship string, dnssec bool) int {
	if latencyMs <= 0 || packetLossPct == 100 {
		return -20000 // Inactive or dead node
	}

	if censorship == "hijacked" || censorship == "manipulated" {
		return -10000 // Highly compromised node, move to absolute bottom
	}

	wSpeed := 0.5
	wJitter := 0.2
	wLoss := 0.3

	// Normalize Speed: 1000ms base. 10ms -> 100 points, 100ms -> 10 points
	speedScore := 0.0
	if latencyMs > 0 {
		speedScore = (1000.0 / float64(latencyMs)) * wSpeed
	}

	jitterPenalty := jitterMs * wJitter
	lossPenalty := packetLossPct * wLoss

	score := (speedScore - jitterPenalty - lossPenalty) * 100.0

	// Security Bonuses
	if dnssec {
		score += 15.0
	}
	if censorship == "sinkhole" {
		score += 10.0 // Extra security boost for filtering malicious trackers
	}

	// Floor/Ceiling bounds
	rounded := int(math.Round(score))
	if rounded < -5000 {
		return -5000
	}
	return rounded
}

// CheckRebindingAttack checks if a response resolved a public domain to private IP space
func CheckRebindingAttack(resp *dns.Msg) bool {
	if resp == nil {
		return false
	}
	for _, rr := range resp.Answer {
		if a, ok := rr.(*dns.A); ok {
			if isPrivateIP(a.A) {
				return true
			}
		}
		if aaaa, ok := rr.(*dns.AAAA); ok {
			if isPrivateIP(aaaa.AAAA) {
				return true
			}
		}
	}
	return false
}

func isPrivateIP(ip net.IP) bool {
	if ip == nil {
		return false
	}
	if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsUnspecified() {
		return true
	}
	// IPv4 private ranges
	if ip4 := ip.To4(); ip4 != nil {
		return ip4[0] == 10 ||
			(ip4[0] == 172 && ip4[1] >= 16 && ip4[1] <= 31) ||
			(ip4[0] == 192 && ip4[1] == 168)
	}
	// IPv6 private/local ranges
	return ip.IsPrivate()
}
