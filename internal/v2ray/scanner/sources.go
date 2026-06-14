package scanner

import (
	"context"
	crand "crypto/rand"
	"encoding/binary"
	"io"
	"math/big"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"clever-connect/internal/db"
	"clever-connect/internal/models"
)

// DNSCacheEntry preserves resolved hosts to prevent DNS rate limits
type DNSCacheEntry struct {
	IPs      []string
	ExpireAt time.Time
}

var (
	dnsCache   = make(map[string]DNSCacheEntry)
	dnsCacheMu sync.RWMutex
)

// FetchEnabledSourcesConcurrently retrieves lists from database and fetches/resolves them
func FetchEnabledSourcesConcurrently(ctx context.Context) ([]string, []string, error) {
	if db.DB == nil {
		return DefaultCloudflareRanges, nil, nil
	}

	var sources []models.ScannerSource
	if err := db.DB.Where("is_enabled = ?", true).Find(&sources).Error; err != nil {
		return nil, nil, err
	}

	var cidrs []string
	var directIPs []string
	var mu sync.Mutex
	var wg sync.WaitGroup

	// Concurrent fetch limit
	sem := make(chan struct{}, 5)

	for _, src := range sources {
		select {
		case <-ctx.Done():
			return nil, nil, ctx.Err()
		default:
		}

		wg.Add(1)
		sem <- struct{}{}

		go func(s models.ScannerSource) {
			defer wg.Done()
			defer func() { <-sem }()

			lines, err := fetchURL(ctx, s.URL)
			if err != nil {
				return
			}

			// Update last fetched timestamp
			now := time.Now()
			db.DB.Model(&s).Update("last_fetched", &now)

			switch s.Type {
			case "cidr":
				parsed := parseCIDRLines(lines)
				mu.Lock()
				cidrs = append(cidrs, parsed...)
				mu.Unlock()
			case "proxyip":
				ips := parseProxyIPLines(lines)
				mu.Lock()
				directIPs = append(directIPs, ips...)
				mu.Unlock()
			case "domain":
				domains := parseDomainLines(lines)
				ips := resolveDomainsCached(ctx, domains)
				mu.Lock()
				directIPs = append(directIPs, ips...)
				mu.Unlock()
			}
		}(src)
	}

	wg.Wait()

	if len(cidrs) == 0 {
		cidrs = DefaultCloudflareRanges
	}

	return cidrs, directIPs, nil
}

func fetchURL(ctx context.Context, url string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", err
	}
	client := &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			DisableKeepAlives: true,
		},
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(body), nil
}

func parseCIDRLines(text string) []string {
	var cidrs []string
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "//") {
			continue
		}
		if _, _, err := net.ParseCIDR(line); err == nil {
			cidrs = append(cidrs, line)
		}
	}
	return cidrs
}

func parseProxyIPLines(text string) []string {
	seen := make(map[string]struct{})
	var ips []string
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "//") {
			continue
		}
		if strings.Contains(line, "#") {
			line = strings.Split(line, "#")[0]
			line = strings.TrimSpace(line)
		}
		var ip string
		if strings.Contains(line, ":") {
			parts := strings.Split(line, ":")
			ip = strings.TrimSpace(parts[0])
		} else {
			ip = line
		}
		if net.ParseIP(ip) != nil {
			if _, exists := seen[ip]; !exists {
				seen[ip] = struct{}{}
				ips = append(ips, ip)
			}
		}
	}
	return ips
}

func parseDomainLines(text string) []string {
	var domains []string
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "//") {
			continue
		}
		if strings.Contains(line, "#") {
			line = strings.Split(line, "#")[0]
			line = strings.TrimSpace(line)
		}
		if line != "" {
			domains = append(domains, line)
		}
	}
	return domains
}

func resolveDomainsCached(ctx context.Context, domains []string) []string {
	var ips []string
	var mu sync.Mutex
	var wg sync.WaitGroup

	sem := make(chan struct{}, 10)

	limit := 100
	if len(domains) < limit {
		limit = len(domains)
	}

	ttl := 5 * time.Minute

	for _, domain := range domains[:limit] {
		select {
		case <-ctx.Done():
			return ips
		default:
		}

		wg.Add(1)
		sem <- struct{}{}

		go func(d string) {
			defer wg.Done()
			defer func() { <-sem }()

			dnsCacheMu.RLock()
			entry, exists := dnsCache[d]
			dnsCacheMu.RUnlock()

			if exists && time.Now().Before(entry.ExpireAt) {
				mu.Lock()
				ips = append(ips, entry.IPs...)
				mu.Unlock()
				return
			}

			addrs, err := net.LookupHost(d)
			if err != nil {
				return
			}

			var ipv4s []string
			for _, addr := range addrs {
				if net.ParseIP(addr) != nil && !strings.Contains(addr, ":") {
					ipv4s = append(ipv4s, addr)
				}
			}

			dnsCacheMu.Lock()
			dnsCache[d] = DNSCacheEntry{
				IPs:      ipv4s,
				ExpireAt: time.Now().Add(ttl),
			}
			dnsCacheMu.Unlock()

			mu.Lock()
			ips = append(ips, ipv4s...)
			mu.Unlock()
		}(domain)
	}

	wg.Wait()
	return ips
}

// StreamRandomIPs yields randomized IP addresses stream-style to the worker pool keeping memory near zero
func StreamRandomIPs(ctx context.Context, cidrs []string, maxCount int) <-chan string {
	out := make(chan string, 100)
	go func() {
		defer close(out)
		if len(cidrs) == 0 {
			return
		}

		type subnet struct {
			start uint32
			end   uint32
			total uint32
		}
		var subnets []subnet
		for _, c := range cidrs {
			start, end, err := parseCIDR(c)
			if err != nil {
				continue
			}
			total := end - start
			if total >= 2 {
				subnets = append(subnets, subnet{start: start, end: end, total: total})
			}
		}
		if len(subnets) == 0 {
			return
		}

		generated := 0
		limit := maxCount
		if limit <= 0 {
			limit = 100000
		}

		seen := make(map[string]struct{})

		for generated < limit {
			select {
			case <-ctx.Done():
				return
			default:
			}

			idx := cryptoRandIntn(len(subnets))
			sub := subnets[idx]

			offset := uint32(0)
			if sub.total > 1 {
				n, err := crand.Int(crand.Reader, big.NewInt(int64(sub.total)))
				if err == nil {
					offset = uint32(n.Int64())
				}
			}

			ipVal := sub.start + 1 + offset%sub.total
			ipStr := uint32ToIPv4(ipVal).String()

			if len(seen) > 50000 {
				seen = make(map[string]struct{})
			}
			if _, ok := seen[ipStr]; ok {
				continue
			}
			seen[ipStr] = struct{}{}

			select {
			case out <- ipStr:
				generated++
			case <-ctx.Done():
				return
			}
		}
	}()
	return out
}

func parseCIDR(cidr string) (uint32, uint32, error) {
	_, ipnet, err := net.ParseCIDR(cidr)
	if err != nil {
		return 0, 0, err
	}
	ones, bits := ipnet.Mask.Size()
	start := ipToUint32(ipnet.IP)
	count := uint32(1) << (bits - ones)
	end := start + count - 1
	return start, end, nil
}

func ipToUint32(ip net.IP) uint32 {
	ip = ip.To4()
	if ip == nil {
		return 0
	}
	return binary.BigEndian.Uint32(ip)
}
