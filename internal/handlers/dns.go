package handlers

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"clever-connect/internal/config"
	"clever-connect/internal/db"
	pebbledb "clever-connect/internal/db/pebble"
	"clever-connect/internal/dns"
	"clever-connect/internal/geo"
	"clever-connect/internal/logger"
	"clever-connect/internal/models"
	"clever-connect/internal/v2ray/compiler"
	"clever-connect/internal/v2ray/core"

	"github.com/cockroachdb/pebble"
	"github.com/gin-gonic/gin"
	miekgdns "github.com/miekg/dns"
	"gorm.io/gorm"
)

type DNSHandler struct {
	cfg *config.Config
}

func NewDNSHandler(cfg *config.Config) *DNSHandler {
	return &DNSHandler{cfg: cfg}
}

// ListResolvers handles GET /api/dns/resolvers
func (h *DNSHandler) ListResolvers(c *gin.Context) {
	if db.DB == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database not initialized"})
		return
	}

	category := c.DefaultQuery("category", "")
	protocol := c.DefaultQuery("protocol", "")
	search := c.DefaultQuery("search", "")

	query := db.DB.Model(&models.DNSResolver{})

	if category != "" {
		query = query.Where("category = ?", category)
	}
	if protocol != "" {
		switch protocol {
		case "udp":
			query = query.Where("support_udp = ?", true)
		case "tcp":
			query = query.Where("support_tcp = ?", true)
		case "dot":
			query = query.Where("support_dot = ?", true)
		case "doh":
			query = query.Where("support_doh = ?", true)
		case "doq":
			query = query.Where("support_doq = ?", true)
		}
	}
	if search != "" {
		query = query.Where("ip LIKE ? OR provider_name LIKE ?", "%"+search+"%", "%"+search+"%")
	}

	var resolvers []models.DNSResolver
	if err := query.Find(&resolvers).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	type DNSResolverResponse struct {
		models.DNSResolver
		LatencyMs      int64     `json:"latency_ms"`
		JitterMs       float64   `json:"jitter_ms"`
		PacketLossPct  float64   `json:"packet_loss_pct"`
		SuccessRatePct float64   `json:"success_rate_pct"`
		CleverScore    int       `json:"clever_score"`
		ErrorMessage   string    `json:"error_message"`
		CompletedAt    time.Time `json:"completed_at"`
		ResolvedIP     string    `json:"resolved_ip"`
		City           string    `json:"city"`
		IsCDN          bool      `json:"is_cdn"`
		CDNProvider    string    `json:"cdn_provider"`
		ExpectedMatch  bool      `json:"expected_match"`
	}

	responseList := make([]DNSResolverResponse, len(resolvers))
	for i, r := range resolvers {
		responseList[i] = DNSResolverResponse{
			DNSResolver:   r,
			ExpectedMatch: true, // default
		}
		if pebbledb.DB != nil {
			metricsKey := fmt.Sprintf("dns:metric:%s:%s", r.IP, r.Protocol)
			valBytes, closer, err := pebbledb.DB.Get([]byte(metricsKey))
			if err == nil {
				if len(valBytes) > 0 {
					var payload pebbledb.DNSMetricPayload
					if err := json.Unmarshal(valBytes, &payload); err == nil {
						responseList[i].LatencyMs = int64(payload.AvgLatencyMs)
						responseList[i].JitterMs = payload.JitterMs
						responseList[i].PacketLossPct = payload.PacketLossPct
						responseList[i].SuccessRatePct = payload.SuccessRatePct
						responseList[i].ErrorMessage = payload.ErrorMessage
						responseList[i].CompletedAt = payload.LastChecked
						responseList[i].ResolvedIP = payload.ResolvedIP
						responseList[i].City = payload.City
						responseList[i].IsCDN = payload.IsCDN
						responseList[i].CDNProvider = payload.CDNProvider
						responseList[i].ExpectedMatch = payload.ExpectedMatch

						// Calculate CleverScore dynamically on retrieval
						isDnssec := !r.DNSSECOverride
						responseList[i].CleverScore = dns.CalculateCleverScore(
							int64(payload.AvgLatencyMs),
							payload.JitterMs,
							payload.PacketLossPct,
							r.CensorshipStatus,
							isDnssec,
						)
					}
				}
				closer.Close()
			}
		}
	}

	c.JSON(http.StatusOK, responseList)
}

// AddResolver handles POST /api/dns/resolvers
func (h *DNSHandler) AddResolver(c *gin.Context) {
	if db.DB == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database not initialized"})
		return
	}

	var req models.DNSResolver
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.IP == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Resolver IP address is required"})
		return
	}

	if req.ProviderName == "" {
		req.ProviderName = "Custom Resolver"
	}
	if req.Category == "" {
		req.Category = "custom"
	}

	// Save or Update Resolver in DB
	var existing models.DNSResolver
	err := db.DB.Where("ip = ? AND protocol = ?", req.IP, req.Protocol).First(&existing).Error
	if err == nil {
		req.ID = existing.ID
		if err := db.DB.Save(&req).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	} else {
		if err := db.DB.Create(&req).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}

	c.JSON(http.StatusCreated, req)
}

// DeleteResolver handles DELETE /api/dns/resolvers/:id
func (h *DNSHandler) DeleteResolver(c *gin.Context) {
	if db.DB == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database not initialized"})
		return
	}

	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid resolver ID"})
		return
	}

	if err := db.DB.Delete(&models.DNSResolver{}, id).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "deleted"})
}

// BatchDeleteResolvers handles POST /api/dns/resolvers/batch-delete
func (h *DNSHandler) BatchDeleteResolvers(c *gin.Context) {
	if db.DB == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database not initialized"})
		return
	}

	var req struct {
		IDs []int `json:"ids"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request payload"})
		return
	}

	if len(req.IDs) == 0 {
		c.JSON(http.StatusOK, gin.H{"status": "no-op", "deleted_count": 0})
		return
	}

	if err := db.DB.Where("id IN ?", req.IDs).Delete(&models.DNSResolver{}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "deleted", "deleted_count": len(req.IDs)})
}

// FetchPublicResolvers handles POST /api/dns/resolvers/fetch-public
func (h *DNSHandler) FetchPublicResolvers(c *gin.Context) {
	if db.DB == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database not initialized"})
		return
	}

	var req struct {
		Source string `json:"source"` // "curated", "bls", "trickest"
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request payload"})
		return
	}

	var ips []string
	var providerMap = make(map[string]string) // IP -> Provider name

	switch req.Source {
	case "curated":
		// Hardcoded high-quality, well-known anycast DNS servers
		curated := []struct {
			IP       string
			Provider string
		}{
			{"8.8.8.8", "Google DNS"},
			{"8.8.4.4", "Google DNS"},
			{"1.1.1.1", "Cloudflare DNS"},
			{"1.0.0.1", "Cloudflare DNS"},
			{"9.9.9.9", "Quad9 DNS"},
			{"149.112.112.112", "Quad9 DNS"},
			{"208.67.222.222", "OpenDNS"},
			{"208.67.220.220", "OpenDNS"},
			{"94.140.14.14", "AdGuard DNS (Default)"},
			{"94.140.15.15", "AdGuard DNS (Default)"},
			{"185.228.168.9", "CleanBrowsing"},
			{"185.228.169.9", "CleanBrowsing"},
			{"185.228.168.168", "DNS.SB"},
			{"185.228.169.168", "DNS.SB"},
			{"76.76.2.0", "Control D"},
			{"76.76.10.0", "Control D"},
			{"4.2.2.1", "Level3 DNS"},
			{"4.2.2.2", "Level3 DNS"},
			{"8.26.56.26", "Comodo Secure"},
			{"8.20.247.20", "Comodo Secure"},
			{"194.242.2.2", "Mullvad DNS"},
		}
		for _, item := range curated {
			ips = append(ips, item.IP)
			providerMap[item.IP] = item.Provider
		}

	case "bls":
		url := "https://raw.githubusercontent.com/blacklanternsecurity/public-dns-servers/master/nameservers.txt"
		resp, err := http.Get(url)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch from remote source: " + err.Error()})
			return
		}
		defer resp.Body.Close()

		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			ip := strings.TrimSpace(scanner.Text())
			if ip != "" && !strings.HasPrefix(ip, "#") && !strings.Contains(ip, "Source:") && !strings.Contains(ip, "---") {
				// Parse IP to check if it's valid IPv4/IPv6
				parsedIP := net.ParseIP(ip)
				if parsedIP != nil {
					ips = append(ips, ip)
					providerMap[ip] = "BLS Public DNS"
				}
			}
		}

	case "trickest":
		url := "https://raw.githubusercontent.com/trickest/resolvers/main/resolvers.txt"
		resp, err := http.Get(url)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch from remote source: " + err.Error()})
			return
		}
		defer resp.Body.Close()

		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			ip := strings.TrimSpace(scanner.Text())
			if ip != "" && !strings.HasPrefix(ip, "#") {
				parsedIP := net.ParseIP(ip)
				if parsedIP != nil {
					ips = append(ips, ip)
					providerMap[ip] = "Trickest Resolver"
				}
			}
		}

	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "Unknown DNS source type"})
		return
	}

	if len(ips) == 0 {
		c.JSON(http.StatusOK, gin.H{"status": "success", "added_count": 0})
		return
	}

	// Fetch existing resolvers to avoid duplicates (IP + protocol)
	var existing []models.DNSResolver
	if err := db.DB.Find(&existing).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error: " + err.Error()})
		return
	}

	existingMap := make(map[string]bool)
	for _, r := range existing {
		// key: IP:protocol
		key := fmt.Sprintf("%s:%s", r.IP, strings.ToLower(r.Protocol))
		existingMap[key] = true
	}

	// Insert new resolvers (supporting both UDP and TCP by default for public DNS)
	var newResolvers []models.DNSResolver
	for _, ip := range ips {
		for _, proto := range []string{"udp", "tcp"} {
			key := fmt.Sprintf("%s:%s", ip, proto)
			if !existingMap[key] {
				provider := providerMap[ip]
				if provider == "" {
					provider = "Public DNS"
				}
				newResolvers = append(newResolvers, models.DNSResolver{
					IP:           ip,
					Protocol:     proto,
					ProviderName: provider,
					Category:     "public",
					SupportUDP:   proto == "udp",
					SupportTCP:   proto == "tcp",
					IsCustom:     true,
				})
			}
		}
	}

	if len(newResolvers) > 0 {
		// Batch insert in chunks of 500 to avoid SQLite limits or memory overhead
		chunkSize := 500
		for i := 0; i < len(newResolvers); i += chunkSize {
			end := i + chunkSize
			if end > len(newResolvers) {
				end = len(newResolvers)
			}
			if err := db.DB.Create(newResolvers[i:end]).Error; err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Database insert error: " + err.Error()})
				return
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"status":      "success",
		"added_count": len(newResolvers),
		"total_found": len(ips) * 2, // 2 protocols per IP
	})
}

// TestSingleResolver handles POST /api/dns/resolvers/:id/test
func (h *DNSHandler) TestSingleResolver(c *gin.Context) {
	if db.DB == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database not initialized"})
		return
	}

	var resolver models.DNSResolver
	if err := db.DB.First(&resolver, c.Param("id")).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Resolver not found"})
		return
	}

	var req struct {
		Domain         string `json:"domain"`
		QueryType      string `json:"query_type"`
		DNSClass       string `json:"dns_class"`
		TimeoutMs      int    `json:"timeout_ms"`
		Attempts       int    `json:"attempts"`
		CacheBusting   bool   `json:"cache_busting"`
		ExpectResponse string `json:"expect_response"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request payload"})
		return
	}

	// Set defaults
	if req.Domain == "" {
		req.Domain = "google.com"
	}
	if req.QueryType == "" {
		req.QueryType = "A"
	}
	if req.DNSClass == "" {
		req.DNSClass = "IN"
	}
	if req.TimeoutMs <= 0 {
		req.TimeoutMs = 3000
	}
	if req.Attempts <= 0 {
		req.Attempts = 3
	}

	qTypeVal, exists := miekgdns.StringToType[req.QueryType]
	if !exists {
		qTypeVal = miekgdns.TypeA
	}

	qClassVal, exists := miekgdns.StringToClass[req.DNSClass]
	if !exists {
		qClassVal = miekgdns.ClassINET
	}

	queryDomain := req.Domain
	if req.CacheBusting {
		queryDomain = fmt.Sprintf("cc-%d-%s", time.Now().UnixNano(), req.Domain)
	}

	timeout := time.Duration(req.TimeoutMs) * time.Millisecond
	ctx, cancel := context.WithTimeout(context.Background(), timeout*time.Duration(req.Attempts)+2*time.Second)
	defer cancel()

	type queryOutcome struct {
		resp *miekgdns.Msg
		rtt  time.Duration
		err  error
	}

	attemptsCh := make(chan queryOutcome, req.Attempts)

	for i := 0; i < req.Attempts; i++ {
		go func() {
			msg := new(miekgdns.Msg)
			msg.SetQuestion(miekgdns.Fqdn(queryDomain), qTypeVal)
			msg.Question[0].Qclass = qClassVal
			msg.RecursionDesired = true
			msg.SetEdns0(4096, true) // Enable DNSSEC DO bit

			var resp *miekgdns.Msg
			var rtt time.Duration
			var err error

			switch strings.ToLower(resolver.Protocol) {
			case "udp":
				resp, rtt, err = dns.QueryUDP(ctx, resolver.IP, msg, timeout)
			case "tcp":
				resp, rtt, err = dns.QueryTCP(ctx, resolver.IP, msg, timeout)
			case "dot":
				resp, rtt, err = dns.QueryDoT(ctx, resolver.IP, msg, timeout)
			case "doh":
				resp, rtt, err = dns.QueryDoH(ctx, resolver.IP, msg, timeout)
			case "doq":
				resp, rtt, err = dns.QueryDoQ(ctx, resolver.IP, msg, timeout)
			default:
				resp, rtt, err = dns.QueryUDP(ctx, resolver.IP, msg, timeout)
			}

			attemptsCh <- queryOutcome{resp: resp, rtt: rtt, err: err}
		}()
	}

	var latencies []int64
	var lastErr error
	successes := 0
	dnssecCount := 0
	var resolvedIPs []string
	var rawResponseText string
	var rawRcode string
	var rawAnswers []gin.H

	for i := 0; i < req.Attempts; i++ {
		outcome := <-attemptsCh
		if outcome.err == nil && outcome.resp != nil {
			successes++
			latencies = append(latencies, outcome.rtt.Milliseconds())
			if dns.CheckDNSSECValidation(outcome.resp) {
				dnssecCount++
			}

			for _, ans := range outcome.resp.Answer {
				header := ans.Header()
				recordType := miekgdns.TypeToString[header.Rrtype]
				recordClass := miekgdns.ClassToString[header.Class]

				ansData := ""
				switch rr := ans.(type) {
				case *miekgdns.A:
					ansData = rr.A.String()
					resolvedIPs = append(resolvedIPs, ansData)
				case *miekgdns.AAAA:
					ansData = rr.AAAA.String()
					resolvedIPs = append(resolvedIPs, ansData)
				case *miekgdns.CNAME:
					ansData = rr.Target
				case *miekgdns.MX:
					ansData = fmt.Sprintf("%d %s", rr.Preference, rr.Mx)
				case *miekgdns.TXT:
					ansData = strings.Join(rr.Txt, " ")
				case *miekgdns.NS:
					ansData = rr.Ns
				case *miekgdns.SOA:
					ansData = fmt.Sprintf("%s %s %d %d %d %d %d", rr.Ns, rr.Mbox, rr.Serial, rr.Refresh, rr.Retry, rr.Expire, rr.Minttl)
				case *miekgdns.SRV:
					ansData = fmt.Sprintf("%d %d %d %s", rr.Priority, rr.Weight, rr.Port, rr.Target)
				default:
					ansData = ans.String()
				}

				rawAnswers = append(rawAnswers, gin.H{
					"name":  header.Name,
					"type":  recordType,
					"class": recordClass,
					"ttl":   header.Ttl,
					"data":  ansData,
				})
			}

			rawResponseText = outcome.resp.String()
			rawRcode = miekgdns.RcodeToString[outcome.resp.Rcode]
		} else if outcome.err != nil {
			lastErr = outcome.err
		}
	}

	var avgLatency int64
	if len(latencies) > 0 {
		var sum int64
		for _, l := range latencies {
			sum += l
		}
		avgLatency = sum / int64(len(latencies))
	}

	var jitter float64
	if len(latencies) > 1 {
		var diffSum float64
		for i := 1; i < len(latencies); i++ {
			diff := latencies[i] - latencies[i-1]
			if diff < 0 {
				diff = -diff
			}
			diffSum += float64(diff)
		}
		jitter = diffSum / float64(len(latencies)-1)
	}

	packetLoss := float64(req.Attempts-successes) / float64(req.Attempts) * 100.0

	isCensored := "clean"
	isDnssec := dnssecCount > 0
	hasRebinding := false

	expectedMatch := true
	if req.ExpectResponse != "" {
		expectedMatch = false
		expectLower := strings.ToLower(req.ExpectResponse)
		for _, val := range resolvedIPs {
			if strings.Contains(strings.ToLower(val), expectLower) {
				expectedMatch = true
				break
			}
		}
	}

	var resolvedIP string
	var countryCode, countryName, city, isp, cdnProvider, asn string
	var isCDN bool

	if len(resolvedIPs) > 0 {
		resolvedIP = resolvedIPs[0]

		parsed := net.ParseIP(resolvedIP)
		if parsed != nil && (parsed.IsLoopback() || parsed.IsPrivate() || parsed.IsUnspecified()) {
			hasRebinding = true
		}

		if geoEngine := geo.GetEngine(); geoEngine != nil {
			reg, err := geoEngine.ResolveIP(resolvedIP, false)
			if err == nil && reg != nil {
				countryCode = reg.CountryCode
				countryName = reg.CountryName
				city = reg.City
				isp = reg.ISP
				isCDN = reg.IsCDN
				cdnProvider = reg.CDNProvider

				asn = ""
				if strings.HasPrefix(reg.ISP, "AS") {
					parts := strings.SplitN(reg.ISP, " ", 2)
					asn = parts[0]
					if len(parts) > 1 {
						isp = parts[1]
					}
				}
			}
		}
	}

	if successes == 0 {
		isCensored = "timeout"
	}

	score := dns.CalculateCleverScore(avgLatency, jitter, packetLoss, isCensored, isDnssec)

	errMsg := ""
	if successes == 0 && lastErr != nil {
		errMsg = lastErr.Error()
	}

	// Update SQLite compliant fields
	resolver.CountryCode = countryCode
	resolver.CountryName = countryName
	resolver.ISP = isp
	resolver.ASN = asn
	resolver.DNSRebindingVuln = hasRebinding
	resolver.CensorshipStatus = isCensored

	if err := db.DB.Save(&resolver).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update resolver record: " + err.Error()})
		return
	}

	// Update PebbleDB metrics
	if pebbledb.DB != nil {
		metricsKey := fmt.Sprintf("dns:metric:%s:%s", resolver.IP, resolver.Protocol)
		payload := pebbledb.DNSMetricPayload{
			IP:               resolver.IP,
			Protocol:         resolver.Protocol,
			MinLatencyMs:     float64(avgLatency),
			AvgLatencyMs:     float64(avgLatency),
			MaxLatencyMs:     float64(avgLatency),
			JitterMs:         jitter,
			PacketLossPct:    packetLoss,
			SuccessRatePct:   float64(successes) / float64(req.Attempts) * 100.0,
			QueryType:        req.QueryType,
			DNSClass:         req.DNSClass,
			Domain:           req.Domain,
			DNSRebindingVuln: hasRebinding,
			LastChecked:      time.Now(),
			ErrorMessage:     errMsg,
			ResolvedIP:       resolvedIP,
			City:             city,
			IsCDN:            isCDN,
			CDNProvider:      cdnProvider,
			ExpectedMatch:    expectedMatch,
		}

		valBytes, err := json.Marshal(payload)
		if err == nil {
			_ = pebbledb.DB.Set([]byte(metricsKey), valBytes, nil)
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"status":        "success",
		"resolver_id":   resolver.ID,
		"latency_ms":    float64(avgLatency),
		"jitter_ms":     jitter,
		"success_rate":  float64(successes) / float64(req.Attempts) * 100.0,
		"packet_loss":   packetLoss,
		"resolved_ip":   resolvedIP,
		"dnssec":        isDnssec,
		"rebinding":     hasRebinding,
		"clever_score":  score,
		"rcode":         rawRcode,
		"answers":       rawAnswers,
		"raw_response":  rawResponseText,
		"error_message": errMsg,
		"geoip": gin.H{
			"country_code": countryCode,
			"country_name": countryName,
			"city":         city,
			"isp":          isp,
			"is_cdn":       isCDN,
			"cdn_provider": cdnProvider,
		},
	})
}

// GetConfig handles GET /api/dns/config
func (h *DNSHandler) GetConfig(c *gin.Context) {
	if db.DB == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database not initialized"})
		return
	}

	var config models.DNSTesterConfig
	if err := db.DB.First(&config).Error; err != nil {
		// Populate seeded default if query fails
		config = models.DNSTesterConfig{
			ConcurrencyLimit: 100,
			QPSLimit:         0,
			TimeoutMs:        3000,
			Attempts:         3,
			CacheBusting:     true,
			ReferenceDomain:  "google.com",
		}
		db.DB.Create(&config)
	}

	c.JSON(http.StatusOK, config)
}

// SaveConfig handles POST /api/dns/config
func (h *DNSHandler) SaveConfig(c *gin.Context) {
	if db.DB == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database not initialized"})
		return
	}

	var req models.DNSTesterConfig
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var existing models.DNSTesterConfig
	if err := db.DB.First(&existing).Error; err == nil {
		req.ID = existing.ID
		if err := db.DB.Save(&req).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	} else {
		if err := db.DB.Create(&req).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}

	c.JSON(http.StatusOK, req)
}

// ResetConfig handles POST /api/dns/config/reset
func (h *DNSHandler) ResetConfig(c *gin.Context) {
	if db.DB == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database not initialized"})
		return
	}

	_ = db.DB.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&models.DNSTesterConfig{})

	defaultConfig := models.DNSTesterConfig{
		ConcurrencyLimit: 100,
		QPSLimit:         0,
		TimeoutMs:        3000,
		Attempts:         3,
		CacheBusting:     true,
		ReferenceDomain:  "google.com",
	}
	db.DB.Create(&defaultConfig)

	c.JSON(http.StatusOK, defaultConfig)
}

// GetMetrics handles GET /api/dns/metrics
func (h *DNSHandler) GetMetrics(c *gin.Context) {
	if pebbledb.DB == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "PebbleDB metric engine not initialized"})
		return
	}

	ip := c.Query("ip")
	protocol := c.Query("protocol")

	if ip == "" || protocol == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "IP and protocol query parameters are required"})
		return
	}

	metricsKey := fmt.Sprintf("dns:metric:%s:%s", ip, protocol)
	valBytes, closer, err := pebbledb.DB.Get([]byte(metricsKey))
	if err != nil {
		if err == pebble.ErrNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "metrics history not found for this resolver profile"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer closer.Close()

	var payload pebbledb.DNSMetricPayload
	if err := json.Unmarshal(valBytes, &payload); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to decode metrics cache payload"})
		return
	}

	c.JSON(http.StatusOK, payload)
}

// ApplyActiveResolver handles POST /api/dns/core/apply
func (h *DNSHandler) ApplyActiveResolver(c *gin.Context) {
	if db.DB == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database not initialized"})
		return
	}

	var req struct {
		IP       string `json:"ip"`
		Protocol string `json:"protocol"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.IP == "" || req.Protocol == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "IP and protocol fields are required"})
		return
	}

	// Format DOH/DOT/DOQ URL address mapping or standard IP for Core settings
	resolverAddress := req.IP
	if req.Protocol == "doh" {
		resolverAddress = fmt.Sprintf("https://%s/dns-query", req.IP)
	} else if req.Protocol == "dot" {
		resolverAddress = fmt.Sprintf("tcp-tls://%s:853", req.IP)
	} else if req.Protocol == "doq" {
		resolverAddress = fmt.Sprintf("quic://%s:853", req.IP)
	}

	// 1. Update `dns_doh_url` key in settings table
	var setting models.V2RayClientSetting
	err := db.DB.Where("key = ?", "dns_doh_url").First(&setting).Error
	if err != nil {
		setting = models.V2RayClientSetting{
			Key:   "dns_doh_url",
			Value: resolverAddress,
		}
		if err := db.DB.Create(&setting).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save DNS setting: " + err.Error()})
			return
		}
	} else {
		setting.Value = resolverAddress
		if err := db.DB.Save(&setting).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update DNS setting: " + err.Error()})
			return
		}
	}

	// 2. If client proxy engine is running, compile new config and restart it
	restartSuccessful := false
	if core.IsClientRunning() {
		logger.Info("DNS", "Recompiling and reloading client core configuration with new DNS resolver settings", "resolver", resolverAddress)

		// Fetch active client configuration
		configs, _ := pebbledb.ListClientConfigs(pebbledb.ConfigFilter{}, 0, 0)
		var activeConfig *models.V2RayClientConfig
		for _, cfg := range configs {
			if cfg.IsActive {
				activeCopy := cfg
				activeConfig = &activeCopy
				break
			}
		}

		if activeConfig != nil {
			socksPort := 10808
			httpPort := 10809
			evasion := true

			var socksPortSetting models.V2RayClientSetting
			if err := db.DB.Where("key = ?", "socks_port").First(&socksPortSetting).Error; err == nil {
				socksPort, _ = strconv.Atoi(socksPortSetting.Value)
			}
			var httpPortSetting models.V2RayClientSetting
			if err := db.DB.Where("key = ?", "http_port").First(&httpPortSetting).Error; err == nil {
				httpPort, _ = strconv.Atoi(httpPortSetting.Value)
			}
			var evasionSetting models.V2RayClientSetting
			if err := db.DB.Where("key = ?", "evasion_enabled").First(&evasionSetting).Error; err == nil {
				evasion = evasionSetting.Value == "true"
			}

			socksPortPublic := core.FindAvailablePort(socksPort)
			socksPortInternal := core.FindAvailablePort(socksPortPublic + 1000, socksPortPublic)
			httpPortPublic := core.FindAvailablePort(httpPort, socksPortPublic, socksPortInternal)
			httpPortInternal := core.FindAvailablePort(httpPortPublic + 1000, socksPortPublic, socksPortInternal, httpPortPublic)

			configBytes, err := compiler.CompileClientConfig(*activeConfig, socksPortInternal, httpPortInternal, evasion, "")
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to recompile client config: " + err.Error()})
				return
			}

			tempPath := filepath.Join(os.TempDir(), "xray_client.json")
			_ = os.WriteFile(tempPath, configBytes, 0644)

			_ = core.StopClientCore()
			if err := core.StartClientCore(tempPath); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to start client core daemon: " + err.Error()})
				return
			}

			core.StartLocalProxyEngine(socksPortPublic, socksPortInternal, httpPortPublic, httpPortInternal)
			restartSuccessful = true
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"message":            "Fastest DNS Resolver applied successfully",
		"resolver_applied":   resolverAddress,
		"core_reloaded":      restartSuccessful,
	})
}

type BulkAddReq struct {
	Text string `json:"text"`
}

type BulkAddProgress struct {
	Total      int  `json:"total"`
	Processed  int  `json:"processed"`
	Added      int  `json:"added"`
	Duplicates int  `json:"duplicates"`
	Active     bool `json:"active"`
}

var (
	bulkProgress   BulkAddProgress
	bulkProgressMu sync.Mutex
)

func (h *DNSHandler) GetBulkProgress(c *gin.Context) {
	bulkProgressMu.Lock()
	defer bulkProgressMu.Unlock()
	c.JSON(http.StatusOK, bulkProgress)
}

func (h *DNSHandler) AddResolverBulk(c *gin.Context) {
	if db.DB == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database not initialized"})
		return
	}

	var textContent string

	// Check if it is a multipart file upload
	file, err := c.FormFile("file")
	if err == nil {
		opened, err := file.Open()
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to open uploaded file"})
			return
		}
		defer opened.Close()

		buf := new(bytes.Buffer)
		if _, err := io.Copy(buf, opened); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to read uploaded file"})
			return
		}
		textContent = buf.String()
	} else {
		// Bind JSON
		var req BulkAddReq
		if err := c.ShouldBindJSON(&req); err == nil {
			textContent = req.Text
		} else {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Provide either text field or upload a file"})
			return
		}
	}

	// Clean lines
	var lines []string
	for _, rawLine := range strings.Split(textContent, "\n") {
		trimmed := strings.TrimSpace(rawLine)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "//") {
			continue
		}
		lines = append(lines, trimmed)
	}

	if len(lines) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No valid DNS lines found"})
		return
	}

	// If bulk import is already running, return error
	bulkProgressMu.Lock()
	if bulkProgress.Active {
		bulkProgressMu.Unlock()
		c.JSON(http.StatusConflict, gin.H{"error": "Another bulk import is currently active"})
		return
	}

	// Initialize progress
	bulkProgress = BulkAddProgress{
		Total:      len(lines),
		Processed:  0,
		Added:      0,
		Duplicates: 0,
		Active:     true,
	}
	bulkProgressMu.Unlock()

	// Broadcast initial progress
	dns.GetEngine().Broadcast("dns.bulk_progress", bulkProgress)

	// Process in background
	go h.processBulkImport(lines)

	c.JSON(http.StatusAccepted, gin.H{
		"message": "Bulk import started in background",
		"total":   len(lines),
	})
}

func (h *DNSHandler) incrementProgress(added bool, duplicate bool) {
	bulkProgressMu.Lock()
	bulkProgress.Processed++
	if added {
		bulkProgress.Added++
	}
	if duplicate {
		bulkProgress.Duplicates++
	}
	progCopy := bulkProgress
	bulkProgressMu.Unlock()

	dns.GetEngine().Broadcast("dns.bulk_progress", progCopy)
}

func (h *DNSHandler) processBulkImport(lines []string) {
	defer func() {
		bulkProgressMu.Lock()
		bulkProgress.Active = false
		progCopy := bulkProgress
		bulkProgressMu.Unlock()
		dns.GetEngine().Broadcast("dns.bulk_progress", progCopy)
	}()

	type bulkImportJob struct {
		lineIndex int
		line      string
	}

	type bulkImportResult struct {
		lineIndex int
		resolvers []models.DNSResolver
	}

	// 1. Load existing resolvers to skip database queries during matching
	knownResolvers := make(map[string]bool)
	var existingList []models.DNSResolver
	if db.DB != nil {
		if err := db.DB.Select("ip, protocol").Find(&existingList).Error; err == nil {
			for _, r := range existingList {
				knownResolvers[fmt.Sprintf("%s:%s", r.IP, r.Protocol)] = true
			}
		}
	}

	// 2. Read concurrency limit from configuration
	var testerCfg models.DNSTesterConfig
	concurrencyLimit := runtime.NumCPU() * 32
	if concurrencyLimit < 128 {
		concurrencyLimit = 128
	}
	if db.DB != nil {
		if err := db.DB.First(&testerCfg).Error; err == nil && testerCfg.ConcurrencyLimit > 0 {
			concurrencyLimit = testerCfg.ConcurrencyLimit
		}
	}

	totalJobs := len(lines)
	jobsChan := make(chan bulkImportJob, totalJobs)
	resultsChan := make(chan bulkImportResult, totalJobs)

	// Start workers
	var wg sync.WaitGroup
	for w := 0; w < concurrencyLimit; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for job := range jobsChan {
				parts := strings.Split(job.line, ",")
				ipOrUrl := strings.TrimSpace(parts[0])
				if ipOrUrl == "" {
					resultsChan <- bulkImportResult{lineIndex: job.lineIndex}
					continue
				}

				name := "Custom Import"
				if len(parts) > 1 {
					name = strings.TrimSpace(parts[1])
				}
				userProto := ""
				if len(parts) > 2 {
					userProto = strings.ToLower(strings.TrimSpace(parts[2]))
				}

				isDoH := strings.HasPrefix(ipOrUrl, "http://") || strings.HasPrefix(ipOrUrl, "https://")
				var targetIP string
				if isDoH {
					targetIP = ipOrUrl
				} else {
					if host, _, err := net.SplitHostPort(ipOrUrl); err == nil {
						targetIP = host
					} else {
						targetIP = ipOrUrl
					}
				}

				var detectedProtos []string
				if userProto != "" {
					detectedProtos = []string{userProto}
				} else if isDoH {
					detectedProtos = []string{"doh"}
				} else {
					detectedProtos = h.probeResolverProtocols(targetIP)
					if len(detectedProtos) == 0 {
						detectedProtos = []string{"udp"}
					}
				}

				var geoInfo *models.IPRegistry
				if !isDoH {
					if resolvedGeo, err := geo.GetEngine().ResolveIP(targetIP, false); err == nil && resolvedGeo != nil {
						geoInfo = resolvedGeo
					}
				}

				var resolvers []models.DNSResolver
				for _, proto := range detectedProtos {
					resolver := models.DNSResolver{
						IP:           targetIP,
						Protocol:     proto,
						ProviderName: name,
						IsCustom:     true,
						Category:     "custom",
						SupportUDP:   proto == "udp",
						SupportTCP:   proto == "tcp",
						SupportDoT:   proto == "dot",
						SupportDoH:   proto == "doh",
						SupportDoQ:   proto == "doq",
					}

					if geoInfo != nil {
						resolver.ISP = geoInfo.ISP
						resolver.CountryCode = geoInfo.CountryCode
						resolver.CountryName = geoInfo.CountryName
						if strings.HasPrefix(geoInfo.ISP, "AS") {
							parts := strings.SplitN(geoInfo.ISP, " ", 2)
							resolver.ASN = parts[0]
							if len(parts) > 1 {
								resolver.ISP = parts[1]
							}
						}
					}
					resolvers = append(resolvers, resolver)
				}

				resultsChan <- bulkImportResult{
					lineIndex: job.lineIndex,
					resolvers: resolvers,
				}
			}
		}()
	}

	// Queue jobs in background to avoid deadlock
	go func() {
		for i, line := range lines {
			jobsChan <- bulkImportJob{lineIndex: i, line: line}
		}
		close(jobsChan)
	}()

	// Close resultsChan when workers are done
	go func() {
		wg.Wait()
		close(resultsChan)
	}()

	// Collect results and batch write to databases
	var batchQueue []models.DNSResolver
	const maxBatchSize = 100
	flushInterval := 250 * time.Millisecond
	ticker := time.NewTicker(flushInterval)
	defer ticker.Stop()

	flushBatch := func() {
		if len(batchQueue) == 0 {
			return
		}
		// Batch insert to GORM SQLite
		if db.DB != nil {
			if err := db.DB.Create(&batchQueue).Error; err != nil {
				logger.Error("DNS", "Failed to batch create resolvers in GORM during bulk import", "error", err.Error())
			}
		}
		// Batch insert to PebbleDB
		if pebbledb.DB != nil {
			pebbleBatch := pebbledb.DB.NewBatch()
			for _, r := range batchQueue {
				pebbleKey := fmt.Sprintf("dns:resolver:%s:%s", r.IP, r.Protocol)
				jsonBytes, err := json.Marshal(r)
				if err == nil {
					_ = pebbleBatch.Set([]byte(pebbleKey), jsonBytes, nil)
				}
			}
			if err := pebbleBatch.Commit(pebble.Sync); err != nil {
				logger.Error("DNS", "Failed to batch commit to PebbleDB during bulk import", "error", err.Error())
			}
		}
		batchQueue = nil
	}

	// Reset counters
	bulkProgressMu.Lock()
	bulkProgress.Processed = 0
	bulkProgress.Added = 0
	bulkProgress.Duplicates = 0
	bulkProgressMu.Unlock()

	var lastBroadcastTime time.Time
	var lastBroadcastPct int = -1

	incrementStats := func(added bool, duplicate bool) {
		bulkProgressMu.Lock()
		bulkProgress.Processed++
		if added {
			bulkProgress.Added++
		}
		if duplicate {
			bulkProgress.Duplicates++
		}
		pct := 0
		if totalJobs > 0 {
			pct = (bulkProgress.Processed * 100) / totalJobs
		}
		now := time.Now()
		shouldBroadcast := false
		if bulkProgress.Processed == totalJobs || pct != lastBroadcastPct || now.Sub(lastBroadcastTime) >= 100*time.Millisecond {
			shouldBroadcast = true
			lastBroadcastPct = pct
			lastBroadcastTime = now
		}
		progCopy := bulkProgress
		bulkProgressMu.Unlock()

		if shouldBroadcast {
			dns.GetEngine().Broadcast("dns.bulk_progress", progCopy)
		}
	}

	// Track seen items inside this run
	seenInRun := make(map[string]bool)

	for {
		select {
		case res, ok := <-resultsChan:
			if !ok {
				flushBatch()
				return
			}

			addedCount := 0
			dupeCount := 0

			for _, r := range res.resolvers {
				key := fmt.Sprintf("%s:%s", r.IP, r.Protocol)
				if knownResolvers[key] || seenInRun[key] {
					dupeCount++
					continue
				}
				seenInRun[key] = true

				batchQueue = append(batchQueue, r)
				addedCount++

				if len(batchQueue) >= maxBatchSize {
					flushBatch()
				}
			}

			isAdded := addedCount > 0
			isDupe := len(res.resolvers) > 0 && addedCount == 0

			incrementStats(isAdded, isDupe)

		case <-ticker.C:
			flushBatch()
		}
	}
}

func (h *DNSHandler) probeResolverProtocols(ip string) []string {
	var detected []string
	ctx, cancel := context.WithTimeout(context.Background(), 1200*time.Millisecond)
	defer cancel()

	msg := new(miekgdns.Msg)
	msg.SetQuestion(miekgdns.Fqdn("google.com."), miekgdns.TypeA)

	udpAddr := net.JoinHostPort(ip, "53")
	clientUDP := &miekgdns.Client{Net: "udp", Timeout: 800 * time.Millisecond}
	if _, _, err := clientUDP.ExchangeContext(ctx, msg, udpAddr); err == nil {
		detected = append(detected, "udp")
	}

	clientTCP := &miekgdns.Client{Net: "tcp", Timeout: 800 * time.Millisecond}
	if _, _, err := clientTCP.ExchangeContext(ctx, msg, udpAddr); err == nil {
		detected = append(detected, "tcp")
	}

	tlsAddr := net.JoinHostPort(ip, "853")
	tlsConfig := &tls.Config{InsecureSkipVerify: true}
	clientDoT := &miekgdns.Client{Net: "tcp-tls", Timeout: 800 * time.Millisecond, TLSConfig: tlsConfig}
	if _, _, err := clientDoT.ExchangeContext(ctx, msg, tlsAddr); err == nil {
		detected = append(detected, "dot")
	}

	return detected
}

