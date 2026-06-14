package dns

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net"
	"net/http"
	"os"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"clever-connect/internal/db"
	pebbledb "clever-connect/internal/db/pebble"
	"clever-connect/internal/geo"
	"clever-connect/internal/logger"
	"clever-connect/internal/models"

	"github.com/cockroachdb/pebble"
	"github.com/miekg/dns"
	"golang.org/x/time/rate"
	"gorm.io/gorm"
)

var dnsClassMap = map[string]uint16{
	"IN":  dns.ClassINET,
	"CH":  dns.ClassCHAOS,
	"ANY": dns.ClassANY,
}

var dnsTypeMap = map[string]uint16{
	"A":     dns.TypeA,
	"AAAA":  dns.TypeAAAA,
	"CNAME": dns.TypeCNAME,
	"MX":    dns.TypeMX,
	"NS":    dns.TypeNS,
	"TXT":   dns.TypeTXT,
	"SOA":   dns.TypeSOA,
	"PTR":   dns.TypePTR,
	"HTTPS": dns.TypeHTTPS,
}

func getTargetDomains(config *models.DNSTesterConfig) []string {
	domains := []string{}
	switch config.DomainSource {
	case "custom":
		for _, d := range config.CustomDomains {
			if clean := strings.TrimSpace(d); clean != "" {
				domains = append(domains, clean)
			}
		}
	case "url":
		if config.WordlistURL != "" {
			client := http.Client{Timeout: 5 * time.Second}
			resp, err := client.Get(config.WordlistURL)
			if err == nil && resp.StatusCode == 200 {
				defer resp.Body.Close()
				scanner := bufio.NewScanner(resp.Body)
				for scanner.Scan() {
					if clean := strings.TrimSpace(scanner.Text()); clean != "" && !strings.HasPrefix(clean, "#") {
						domains = append(domains, clean)
						if len(domains) >= 50 { // safety bound to avoid unbounded memory allocations
							break
						}
					}
				}
			}
		}
	}
	// Fallback to reference domain if none populated
	if len(domains) == 0 {
		ref := strings.TrimSpace(config.ReferenceDomain)
		if ref == "" {
			ref = "google.com"
		}
		domains = append(domains, ref)
	}
	return domains
}

type DNSListener func(stats DNSJobStats, event string, details interface{})

type Engine struct {
	mu           sync.RWMutex
	isTesting    bool
	cancelFunc   context.CancelFunc
	stats        DNSJobStats
	listeners    map[string]DNSListener
	accumulating []DNSTestResult
	bufferMu     sync.Mutex
}

var (
	engineOnce   sync.Once
	globalEngine *Engine
)

// GetEngine returns the singleton DNS Tester Engine instance
func GetEngine() *Engine {
	engineOnce.Do(func() {
		globalEngine = &Engine{
			listeners: make(map[string]DNSListener),
		}
	})
	return globalEngine
}

func (e *Engine) RegisterListener(id string, l DNSListener) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.listeners[id] = l
}

func (e *Engine) UnregisterListener(id string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	delete(e.listeners, id)
}

func (e *Engine) IsTesting() bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.isTesting
}

func (e *Engine) GetLiveStats() DNSJobStats {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.stats
}

func (e *Engine) broadcast(event string, details interface{}) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	stats := e.stats
	logger.Info("DNS", "Broadcasting event to listeners", "event", event, "listenerCount", len(e.listeners))
	for _, l := range e.listeners {
		go l(stats, event, details)
	}
}

// Broadcast exports the event broadcasting logic to other packages
func (e *Engine) Broadcast(event string, details interface{}) {
	e.broadcast(event, details)
}

// GetSystemResolvers parses /etc/resolv.conf to find local system nameservers
func GetSystemResolvers() []models.DNSResolver {
	var list []models.DNSResolver
	file, err := os.Open("/etc/resolv.conf")
	if err != nil {
		return list
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "nameserver") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				ip := parts[1]
				// Filter out local loopbacks but keep gateway IPs
				if ip != "" && ip != "127.0.0.1" && ip != "::1" && !strings.HasPrefix(ip, "127.") {
					list = append(list, models.DNSResolver{
						IP:           ip,
						Protocol:     "udp",
						ProviderName: "System DNS (ISP)",
						Category:     "system",
						SupportUDP:   true,
						SupportTCP:   true,
					})
				}
			}
		}
	}
	return list
}

// StopTest cancels the active benchmark run
func (e *Engine) StopTest() {
	e.mu.Lock()
	if !e.isTesting {
		e.mu.Unlock()
		return
	}
	if e.cancelFunc != nil {
		e.cancelFunc()
	}
	e.isTesting = false
	e.stats.Phase = "idle"
	e.mu.Unlock()
}

// StartTest triggers a parallel benchmarks sweep
func (e *Engine) StartTest(config *models.DNSTesterConfig, customResolvers []models.DNSResolver, protocols []string) error {
	e.mu.Lock()
	if e.isTesting {
		e.mu.Unlock()
		return fmt.Errorf("a DNS test sweep is already running")
	}
	e.isTesting = true
	ctx, cancel := context.WithCancel(context.Background())
	e.cancelFunc = cancel
	e.mu.Unlock()

	// Adjust system file limits ephemerally
	SetHighOpenFileLimits()

	go e.runTestSweep(ctx, config, customResolvers, protocols)
	return nil
}

func (e *Engine) runTestSweep(ctx context.Context, config *models.DNSTesterConfig, customResolvers []models.DNSResolver, protocols []string) {
	defer func() {
		if r := recover(); r != nil {
			logger.Error("DNS", "Recovered from panic in runTestSweep", "panic", r)
		}
		e.mu.Lock()
		e.isTesting = false
		e.stats.Phase = "idle"
		e.mu.Unlock()
		if ctx.Err() != nil {
			e.broadcast("dns.stopped", "DNS sweep aborted by user request.")
		} else {
			e.broadcast("dns.finished", "DNS sweep completed.")
		}
	}()

	e.bufferMu.Lock()
	e.accumulating = nil
	e.bufferMu.Unlock()

	// 1. Gather resolvers from DB or custom list
	var targets []models.DNSResolver
	if len(customResolvers) > 0 {
		targets = customResolvers
	} else if db.DB != nil {
		if err := db.DB.Find(&targets).Error; err != nil {
			logger.Error("DNS", "Failed to retrieve resolvers from DB", "error", err)
			return
		}
	}

	// Dynamic detection of local system/ISP DNS (to guarantee working resolvers exist)
	if len(customResolvers) == 0 {
		sysResolvers := GetSystemResolvers()
		for _, sys := range sysResolvers {
			exists := false
			for _, t := range targets {
				if t.IP == sys.IP && t.Protocol == sys.Protocol {
					exists = true
					break
				}
			}
			if !exists {
				targets = append(targets, sys)
			}
		}
	}

	if len(targets) == 0 {
		logger.Warn("DNS", "No DNS target resolvers to benchmark")
		return
	}

	// Filter targeted protocols
	protoSet := make(map[string]bool)
	for _, p := range protocols {
		protoSet[strings.ToLower(p)] = true
	}
	if len(protoSet) == 0 {
		protoSet["udp"] = true
	}

	// Gather query types
	qTypes := config.QueryTypes
	if len(qTypes) == 0 {
		qTypes = []string{"A"}
	}

	// Gather domains
	domains := getTargetDomains(config)

	// 2. Generate jobs list
	var jobs []DNSTestJob
	for _, r := range targets {
		for _, qType := range qTypes {
			for _, domain := range domains {
				qTypeClean := strings.ToUpper(strings.TrimSpace(qType))
				if qTypeClean == "" {
					continue
				}

				// Verify if targeted protocol matches capabilities
				if protoSet["udp"] && (r.SupportUDP || r.IsCustom) {
					jobs = append(jobs, DNSTestJob{
						IP:           r.IP,
						Protocol:     "udp",
						ProviderName: r.ProviderName,
						Category:     r.Category,
						QueryType:    qTypeClean,
						DNSClass:     config.DNSClass,
						Domain:       domain,
						Config:       config,
					})
				}
				if protoSet["tcp"] && (r.SupportTCP || r.IsCustom) {
					jobs = append(jobs, DNSTestJob{
						IP:           r.IP,
						Protocol:     "tcp",
						ProviderName: r.ProviderName,
						Category:     r.Category,
						QueryType:    qTypeClean,
						DNSClass:     config.DNSClass,
						Domain:       domain,
						Config:       config,
					})
				}
				if protoSet["dot"] && (r.SupportDoT || r.IsCustom) {
					jobs = append(jobs, DNSTestJob{
						IP:           r.IP,
						Protocol:     "dot",
						ProviderName: r.ProviderName,
						Category:     r.Category,
						QueryType:    qTypeClean,
						DNSClass:     config.DNSClass,
						Domain:       domain,
						Config:       config,
					})
				}
				if protoSet["doh"] && (r.SupportDoH || r.IsCustom) {
					jobs = append(jobs, DNSTestJob{
						IP:           r.IP,
						Protocol:     "doh",
						ProviderName: r.ProviderName,
						Category:     r.Category,
						QueryType:    qTypeClean,
						DNSClass:     config.DNSClass,
						Domain:       domain,
						Config:       config,
					})
				}
				if protoSet["doq"] && (r.SupportDoQ || r.IsCustom) {
					jobs = append(jobs, DNSTestJob{
						IP:           r.IP,
						Protocol:     "doq",
						ProviderName: r.ProviderName,
						Category:     r.Category,
						QueryType:    qTypeClean,
						DNSClass:     config.DNSClass,
						Domain:       domain,
						Config:       config,
					})
				}
			}
		}
	}

	totalJobs := int64(len(jobs))
	e.mu.Lock()
	e.stats = DNSJobStats{
		Tested:       0,
		Healthy:      0,
		Failed:       0,
		InFlight:     0,
		TotalTargets: totalJobs,
		RemainingSec: totalJobs / int64(config.ConcurrencyLimit) * 2, // approximation
		Phase:        "benchmark_sweep",
	}
	e.mu.Unlock()

	e.broadcast("dns.started", "DNS benchmark sweep started.")

	saveResultChan := make(chan DNSTestResult, 2048)
	var saveWg sync.WaitGroup
	saveWg.Add(1)

	go func() {
		defer func() {
			if r := recover(); r != nil {
				logger.Error("DNS", "Recovered from panic in background saver", "panic", r)
			}
			saveWg.Done()
		}()
		var batch []DNSTestResult
		ticker := time.NewTicker(200 * time.Millisecond)
		defer ticker.Stop()

		flush := func() {
			if len(batch) == 0 {
				return
			}
			e.saveResultsBatch(batch)
			batch = nil
		}

		for {
			select {
			case res, ok := <-saveResultChan:
				if !ok {
					flush()
					e.flushAccumulated()
					return
				}
				batch = append(batch, res)
				if len(batch) >= 100 {
					flush()
				}
			case <-ticker.C:
				flush()
				e.flushAccumulated()
			}
		}
	}()

	// 3. Chunk Scheduler Setup
	concurrency := config.ConcurrencyLimit
	if concurrency <= 0 {
		concurrency = runtime.NumCPU() * 16
		if concurrency < 100 {
			concurrency = 100
		}
	}

	// QPS limiter
	var limiter *rate.Limiter
	if config.QPSLimit > 0 {
		limiter = rate.NewLimiter(rate.Limit(config.QPSLimit), config.QPSLimit)
	}

	t0 := time.Now()

	// Split total jobs into meaningful chunks of size `concurrency`
	chunkSize := concurrency
	for i := 0; i < len(jobs); i += chunkSize {
		if ctx.Err() != nil {
			break
		}
		end := i + chunkSize
		if end > len(jobs) {
			end = len(jobs)
		}
		chunk := jobs[i:end]

		var chunkWg sync.WaitGroup
		for _, job := range chunk {
			if ctx.Err() != nil {
				break
			}
			if limiter != nil {
				if err := limiter.Wait(ctx); err != nil {
					break
				}
			}

			chunkWg.Add(1)
			go func(j DNSTestJob) {
				defer func() {
					if r := recover(); r != nil {
						logger.Error("DNS", "Recovered from panic in job worker", "panic", r)
					}
					chunkWg.Done()
				}()

				atomic.AddInt64(&e.stats.InFlight, 1)
				res := e.runSingleDiagnostic(ctx, j)
				atomic.AddInt64(&e.stats.InFlight, -1)

				if ctx.Err() != nil {
					return
				}

				// Update atomic stats
				atomic.AddInt64(&e.stats.Tested, 1)
				if res.LatencyMs > 0 && res.PacketLossPct < 100 {
					atomic.AddInt64(&e.stats.Healthy, 1)
				} else {
					atomic.AddInt64(&e.stats.Failed, 1)
				}

				// Buffer candidate for batched progressive telemetry
				e.pushResult(res)

				// Queue outcome to be saved by the background batch saver
				select {
				case saveResultChan <- res:
				default:
					go e.saveResultsBatch([]DNSTestResult{res})
				}
			}(job)
		}
		chunkWg.Wait()

		// Recalculate remaining time after each chunk completes
		elapsed := time.Since(t0).Seconds()
		tested := atomic.LoadInt64(&e.stats.Tested)
		if tested > 0 {
			ratePerSec := float64(tested) / elapsed
			remaining := float64(totalJobs-tested) / ratePerSec
			atomic.StoreInt64(&e.stats.RemainingSec, int64(math.Round(remaining)))
		}
	}

	close(saveResultChan)
	saveWg.Wait()
}

func (e *Engine) pushResult(res DNSTestResult) {
	e.bufferMu.Lock()
	defer e.bufferMu.Unlock()
	e.accumulating = append(e.accumulating, res)
}

func (e *Engine) flushAccumulated() {
	e.bufferMu.Lock()
	defer e.bufferMu.Unlock()
	count := len(e.accumulating)
	if count == 0 {
		return
	}
	logger.Info("DNS", "Flushing accumulated candidates", "count", count)
	e.broadcast("dns.candidate", e.accumulating)
	e.accumulating = nil
}

func (e *Engine) runSingleDiagnostic(ctx context.Context, job DNSTestJob) DNSTestResult {
	timeout := time.Duration(job.Config.TimeoutMs) * time.Millisecond
	attempts := job.Config.Attempts
	if attempts <= 0 {
		attempts = 3
	}

	baseDomain := job.Domain
	if baseDomain == "" {
		baseDomain = "google.com"
	}

	qTypeVal := uint16(dns.TypeA)
	if t, exists := dnsTypeMap[strings.ToUpper(job.QueryType)]; exists {
		qTypeVal = t
	}

	qClassVal := uint16(dns.ClassINET)
	if c, exists := dnsClassMap[strings.ToUpper(job.DNSClass)]; exists {
		qClassVal = c
	}

	type queryOutcome struct {
		resp *dns.Msg
		rtt  time.Duration
		err  error
	}

	// 1. Run attempts in parallel
	attemptsCh := make(chan queryOutcome, attempts)
	for i := 0; i < attempts; i++ {
		go func(index int) {
			defer func() {
				if r := recover(); r != nil {
					logger.Error("DNS", "Recovered from panic in query attempt", "panic", r)
					attemptsCh <- queryOutcome{err: fmt.Errorf("panic in query attempt: %v", r)}
				}
			}()

			domain := baseDomain
			switch strings.ToLower(job.Config.QueryGenerator) {
			case "random":
				domain = GenerateCacheBustingDomain(baseDomain)
			case "sequential":
				domain = fmt.Sprintf("seq%d.%s", index+1, baseDomain)
			}

			msg := new(dns.Msg)
			msg.SetQuestion(dns.Fqdn(domain), qTypeVal)
			msg.Question[0].Qclass = qClassVal
			msg.RecursionDesired = true
			msg.SetEdns0(4096, true) // DNSSECDO bit enabled

			var resp *dns.Msg
			var rtt time.Duration
			var err error

			switch job.Protocol {
			case "udp":
				resp, rtt, err = QueryUDP(ctx, job.IP, msg, timeout)
			case "tcp":
				resp, rtt, err = QueryTCP(ctx, job.IP, msg, timeout)
			case "dot":
				resp, rtt, err = QueryDoT(ctx, job.IP, msg, timeout)
			case "doh":
				resp, rtt, err = QueryDoH(ctx, job.IP, msg, timeout)
			case "doq":
				resp, rtt, err = QueryDoQ(ctx, job.IP, msg, timeout)
			}

			attemptsCh <- queryOutcome{resp: resp, rtt: rtt, err: err}
		}(i)
	}

	latencies := []int64{}
	var lastErr error
	successes := 0
	dnssecCount := 0
	hasRebinding := false
	var resolvedIPs []string

	for i := 0; i < attempts; i++ {
		outcome := <-attemptsCh
		if outcome.err == nil && outcome.resp != nil && (outcome.resp.Rcode == dns.RcodeSuccess || outcome.resp.Rcode == dns.RcodeNameError) {
			successes++
			latencies = append(latencies, outcome.rtt.Milliseconds())
			if CheckDNSSECValidation(outcome.resp) {
				dnssecCount++
			}
			if CheckRebindingAttack(outcome.resp) {
				hasRebinding = true
			}
			for _, rr := range outcome.resp.Answer {
				if a, ok := rr.(*dns.A); ok {
					resolvedIPs = append(resolvedIPs, a.A.String())
				} else if aaaa, ok := rr.(*dns.AAAA); ok {
					resolvedIPs = append(resolvedIPs, aaaa.AAAA.String())
				} else if txt, ok := rr.(*dns.TXT); ok {
					resolvedIPs = append(resolvedIPs, strings.Join(txt.Txt, ""))
				} else if cname, ok := rr.(*dns.CNAME); ok {
					resolvedIPs = append(resolvedIPs, cname.Target)
				}
			}
		} else {
			if outcome.err != nil {
				lastErr = outcome.err
			} else if outcome.resp != nil {
				lastErr = fmt.Errorf("DNS error: %s", dns.RcodeToString[outcome.resp.Rcode])
			} else {
				lastErr = fmt.Errorf("unknown connection failure")
			}
		}
	}

	// Calculate loss & latency statistics
	packetLoss := (float64(attempts-successes) / float64(attempts)) * 100.0
	if successes == 0 {
		errMsg := "all connection attempts timed out"
		if lastErr != nil {
			errMsg = lastErr.Error()
		}
		return DNSTestResult{
			IP:               job.IP,
			Protocol:         job.Protocol,
			ProviderName:     job.ProviderName,
			Category:         job.Category,
			LatencyMs:        0,
			PacketLossPct:    100.0,
			SuccessRatePct:   0.0,
			Censorship:       "unverified",
			DNSSECValid:      false,
			DNSRebindingVuln: hasRebinding,
			QueryType:        job.QueryType,
			DNSClass:         job.DNSClass,
			Domain:           job.Domain,
			CleverScore:      -20000,
			CheckedAt:        time.Now(),
			Error:            errMsg,
		}
	}

	// Calculate average latency and jitter
	var sum int64
	for _, l := range latencies {
		sum += l
	}
	avgLatency := sum / int64(successes)

	var varianceSum float64
	for _, l := range latencies {
		diff := float64(l - avgLatency)
		varianceSum += diff * diff
	}
	jitter := math.Sqrt(varianceSum / float64(successes))

	// 2. Only if online, run censorship and sinkhole queries in parallel
	censorshipCh := make(chan queryOutcome, 1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				logger.Error("DNS", "Recovered from panic in censorship check", "panic", r)
				censorshipCh <- queryOutcome{err: fmt.Errorf("panic in censorship check: %v", r)}
			}
		}()

		nxMsg := new(dns.Msg)
		nxMsg.SetQuestion(dns.Fqdn("nonexistent-cc-dns-test-"+GenerateCacheBustingDomain("xyz")), dns.TypeA)
		nxMsg.RecursionDesired = true
		var resp *dns.Msg
		var rtt time.Duration
		var err error

		switch job.Protocol {
		case "udp":
			resp, rtt, err = QueryUDP(ctx, job.IP, nxMsg, timeout)
		case "tcp":
			resp, rtt, err = QueryTCP(ctx, job.IP, nxMsg, timeout)
		case "dot":
			resp, rtt, err = QueryDoT(ctx, job.IP, nxMsg, timeout)
		case "doh":
			resp, rtt, err = QueryDoH(ctx, job.IP, nxMsg, timeout)
		case "doq":
			resp, rtt, err = QueryDoQ(ctx, job.IP, nxMsg, timeout)
		}
		censorshipCh <- queryOutcome{resp: resp, rtt: rtt, err: err}
	}()

	sinkholeCh := make(chan queryOutcome, 1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				logger.Error("DNS", "Recovered from panic in sinkhole check", "panic", r)
				sinkholeCh <- queryOutcome{err: fmt.Errorf("panic in sinkhole check: %v", r)}
			}
		}()

		shMsg := new(dns.Msg)
		shMsg.SetQuestion(dns.Fqdn("ads.doubleclick.net."), dns.TypeA)
		shMsg.RecursionDesired = true
		var resp *dns.Msg
		var rtt time.Duration
		var err error

		switch job.Protocol {
		case "udp":
			resp, rtt, err = QueryUDP(ctx, job.IP, shMsg, timeout)
		case "tcp":
			resp, rtt, err = QueryTCP(ctx, job.IP, shMsg, timeout)
		case "dot":
			resp, rtt, err = QueryDoT(ctx, job.IP, shMsg, timeout)
		case "doh":
			resp, rtt, err = QueryDoH(ctx, job.IP, shMsg, timeout)
		case "doq":
			resp, rtt, err = QueryDoQ(ctx, job.IP, shMsg, timeout)
		}
		sinkholeCh <- queryOutcome{resp: resp, rtt: rtt, err: err}
	}()

	censorshipOutcome := <-censorshipCh
	censorshipStatus := "clean"
	if censorshipOutcome.err == nil && censorshipOutcome.resp != nil {
		if CheckNXDOMAINHijack(censorshipOutcome.resp) {
			censorshipStatus = "manipulated"
		}
	}

	sinkholeOutcome := <-sinkholeCh
	if censorshipStatus == "clean" && sinkholeOutcome.err == nil && sinkholeOutcome.resp != nil {
		if IsTelemetrySinkhole(sinkholeOutcome.resp) {
			censorshipStatus = "sinkhole"
		}
	}

	isDnssec := (float64(dnssecCount) / float64(successes)) >= 0.5
	score := CalculateCleverScore(avgLatency, jitter, packetLoss, censorshipStatus, isDnssec)

	var resolvedIP string
	var countryCode string
	var countryName string
	var city string
	var isp string
	var isCDN bool
	var cdnProvider string
	var expectedMatch bool = true

	if len(resolvedIPs) > 0 {
		resolvedIP = resolvedIPs[0]
		if geoEngine := geo.GetEngine(); geoEngine != nil {
			reg, err := geoEngine.ResolveIP(resolvedIP, false)
			if err == nil && reg != nil {
				countryCode = reg.CountryCode
				countryName = reg.CountryName
				city = reg.City
				isp = reg.ISP
				isCDN = reg.IsCDN
				cdnProvider = reg.CDNProvider
			}
		}

		if job.Config.ExpectResponse != "" {
			expectedMatch = false
			expectLower := strings.ToLower(job.Config.ExpectResponse)
			for _, val := range resolvedIPs {
				if strings.Contains(strings.ToLower(val), expectLower) {
					expectedMatch = true
					break
				}
			}
		}
	}

	return DNSTestResult{
		IP:               job.IP,
		Protocol:         job.Protocol,
		ProviderName:     job.ProviderName,
		Category:         job.Category,
		LatencyMs:        avgLatency,
		JitterMs:         jitter,
		PacketLossPct:    packetLoss,
		SuccessRatePct:   (float64(successes) / float64(attempts)) * 100.0,
		Censorship:       censorshipStatus,
		DNSSECValid:      isDnssec,
		DNSRebindingVuln: hasRebinding,
		QueryType:        job.QueryType,
		DNSClass:         job.DNSClass,
		Domain:           job.Domain,
		CleverScore:      score,
		CheckedAt:        time.Now(),
		ResolvedIP:       resolvedIP,
		CountryCode:      countryCode,
		CountryName:      countryName,
		City:             city,
		ISP:              isp,
		IsCDN:            isCDN,
		CDNProvider:      cdnProvider,
		ExpectedMatch:    expectedMatch,
	}
}

func (e *Engine) saveResultsBatch(batch []DNSTestResult) {
	if db.DB == nil || len(batch) == 0 {
		return
	}

	// 1. Resolve GeoIP for all IPs in the batch outside of the transaction!
	type ipGeoInfo struct {
		CountryCode string
		CountryName string
		ISP         string
		ASN         string
	}
	geoMap := make(map[string]ipGeoInfo)
	if geoEngine := geo.GetEngine(); geoEngine != nil {
		for _, res := range batch {
			if _, exists := geoMap[res.IP]; !exists {
				reg, err := geoEngine.ResolveIP(res.IP, false)
				if err == nil && reg != nil {
					asnVal := ""
					ispVal := reg.ISP
					if strings.HasPrefix(reg.ISP, "AS") {
						parts := strings.SplitN(reg.ISP, " ", 2)
						asnVal = parts[0]
						if len(parts) > 1 {
							ispVal = parts[1]
						}
					}
					geoMap[res.IP] = ipGeoInfo{
						CountryCode: reg.CountryCode,
						CountryName: reg.CountryName,
						ISP:         ispVal,
						ASN:         asnVal,
					}
				}
			}
		}
	}

	// 2. Now run the GORM transaction. No nested db.DB queries will happen!
	_ = db.DB.Transaction(func(tx *gorm.DB) error {
		for _, res := range batch {
			var resolver models.DNSResolver
			err := tx.Where("ip = ? AND protocol = ?", res.IP, res.Protocol).First(&resolver).Error
			if err != nil {
				// Create a new entry
				resolver = models.DNSResolver{
					IP:               res.IP,
					Protocol:         res.Protocol,
					ProviderName:     res.ProviderName,
					Category:         res.Category,
					CensorshipStatus: res.Censorship,
					DNSSECOverride:   !res.DNSSECValid,
					DNSRebindingVuln: res.DNSRebindingVuln,
				}
				// Set capabilities dynamically
				switch res.Protocol {
				case "udp":
					resolver.SupportUDP = true
				case "tcp":
					resolver.SupportTCP = true
				case "dot":
					resolver.SupportDoT = true
				case "doh":
					resolver.SupportDoH = true
				case "doq":
					resolver.SupportDoQ = true
				}

				if geoInfo, exists := geoMap[res.IP]; exists {
					resolver.CountryCode = geoInfo.CountryCode
					resolver.CountryName = geoInfo.CountryName
					resolver.ISP = geoInfo.ISP
					resolver.ASN = geoInfo.ASN
				}

				if err := tx.Create(&resolver).Error; err != nil {
					logger.Error("DNS", "Failed to create resolver during batch update", "error", err)
				}
			} else {
				// Update existing
				resolver.CensorshipStatus = res.Censorship
				resolver.DNSSECOverride = !res.DNSSECValid
				resolver.DNSRebindingVuln = res.DNSRebindingVuln

				// Update support flag dynamically if the test succeeded
				if res.LatencyMs > 0 && res.PacketLossPct < 100 {
					switch res.Protocol {
					case "udp":
						resolver.SupportUDP = true
					case "tcp":
						resolver.SupportTCP = true
					case "dot":
						resolver.SupportDoT = true
					case "doh":
						resolver.SupportDoH = true
					case "doq":
						resolver.SupportDoQ = true
					}
				}

				// Update GeoIP/ASN for the resolver IP if empty
				if resolver.CountryCode == "" || resolver.ISP == "" {
					if geoInfo, exists := geoMap[res.IP]; exists {
						resolver.CountryCode = geoInfo.CountryCode
						resolver.CountryName = geoInfo.CountryName
						resolver.ISP = geoInfo.ISP
						resolver.ASN = geoInfo.ASN
					}
				}

				if err := tx.Save(&resolver).Error; err != nil {
					logger.Error("DNS", "Failed to save resolver during batch update", "error", err)
				}
			}
		}
		return nil
	})

	// 3. Logaggregated performance history into PebbleDB in a single batch commit
	if pebbledb.DB != nil {
		pebbleBatch := pebbledb.DB.NewBatch()
		for _, res := range batch {
			metricsKey := fmt.Sprintf("dns:metric:%s:%s", res.IP, res.Protocol)
			payload := pebbledb.DNSMetricPayload{
				IP:               res.IP,
				Protocol:         res.Protocol,
				MinLatencyMs:     float64(res.LatencyMs),
				AvgLatencyMs:     float64(res.LatencyMs),
				MaxLatencyMs:     float64(res.LatencyMs),
				JitterMs:         res.JitterMs,
				PacketLossPct:    res.PacketLossPct,
				SuccessRatePct:   res.SuccessRatePct,
				QueryType:        res.QueryType,
				DNSClass:         res.DNSClass,
				Domain:           res.Domain,
				DNSRebindingVuln: res.DNSRebindingVuln,
				LastChecked:      res.CheckedAt,
				ErrorMessage:     res.Error,
			}

			valBytes, err := json.Marshal(payload)
			if err == nil {
				_ = pebbleBatch.Set([]byte(metricsKey), valBytes, nil)
			}
		}
		if err := pebbleBatch.Commit(pebble.Sync); err != nil {
			logger.Error("DNS", "Failed to batch commit metrics to PebbleDB", "error", err)
		}
	}
}

// TraceDNS performs an iterative DNS delegation trace starting from root servers
func (e *Engine) TraceDNS(ctx context.Context, domain string, resolverIP string) ([]DNSTraceStep, error) {
	domain = dns.Fqdn(domain)
	var steps []DNSTraceStep

	currentNS := "198.41.0.4:53"
	currentNSName := "a.root-servers.net"

	visited := make(map[string]bool)
	hop := 1

	for {
		if hop > 15 {
			break
		}
		if visited[currentNS] {
			break
		}
		visited[currentNS] = true

		msg := new(dns.Msg)
		msg.SetQuestion(domain, dns.TypeA)
		msg.RecursionDesired = false
		msg.SetEdns0(4096, true)

		start := time.Now()
		resp, _, err := QueryUDP(ctx, currentNS, msg, 2*time.Second)
		latency := time.Since(start).Milliseconds()

		if err != nil {
			steps = append(steps, DNSTraceStep{
				Hop:        hop,
				ServerIP:   currentNS,
				ServerName: currentNSName,
				LatencyMs:  latency,
				Rcode:      "TIMEOUT/ERROR",
				Delegated:  "",
			})
			break
		}

		rcodeStr := dns.RcodeToString[resp.Rcode]
		var delegatedNS string
		var delegatedNSIP string

		for _, rr := range resp.Ns {
			if ns, ok := rr.(*dns.NS); ok {
				delegatedNS = ns.Ns
				break
			}
		}

		for _, rr := range resp.Extra {
			if a, ok := rr.(*dns.A); ok && a.Header().Name == delegatedNS {
				delegatedNSIP = a.A.String()
				break
			}
		}

		if delegatedNS != "" && delegatedNSIP == "" {
			nsIPs, err := net.LookupHost(delegatedNS)
			if err == nil && len(nsIPs) > 0 {
				delegatedNSIP = nsIPs[0]
			}
		}

		step := DNSTraceStep{
			Hop:        hop,
			ServerIP:   currentNS,
			ServerName: currentNSName,
			LatencyMs:  latency,
			Rcode:      rcodeStr,
			Delegated:  delegatedNS,
		}
		steps = append(steps, step)

		if len(resp.Answer) > 0 || resp.Rcode == dns.RcodeNameError {
			break
		}

		if delegatedNSIP != "" {
			currentNS = net.JoinHostPort(delegatedNSIP, "53")
			currentNSName = delegatedNS
			hop++
		} else {
			break
		}
	}

	if len(steps) == 1 && steps[0].Rcode == "TIMEOUT/ERROR" && resolverIP != "" {
		msg := new(dns.Msg)
		msg.SetQuestion(domain, dns.TypeA)
		msg.RecursionDesired = true
		start := time.Now()
		resp, _, err := QueryUDP(ctx, resolverIP, msg, 3*time.Second)
		latency := time.Since(start).Milliseconds()
		if err == nil && resp != nil {
			steps = append(steps, DNSTraceStep{
				Hop:        2,
				ServerIP:   resolverIP,
				ServerName: "Recursive Fallback Resolver",
				LatencyMs:  latency,
				Rcode:      dns.RcodeToString[resp.Rcode],
				Delegated:  "",
			})
		}
	}

	return steps, nil
}

// TestAXFR checks zone transfer exposure on a domain for a resolver
func (e *Engine) TestAXFR(ctx context.Context, domain string, resolverIP string) (*DNSAXFRResult, error) {
	domain = dns.Fqdn(domain)
	targetAddr := resolverIP
	if _, _, err := net.SplitHostPort(resolverIP); err != nil {
		targetAddr = net.JoinHostPort(resolverIP, "53")
	}

	t := &dns.Transfer{}
	msg := new(dns.Msg)
	msg.SetAxfr(domain)

	channel, err := t.In(msg, targetAddr)
	if err != nil {
		return &DNSAXFRResult{
			ResolverIP: resolverIP,
			Domain:     domain,
			Allowed:    false,
			Error:      err.Error(),
		}, nil
	}

	var records []string
	allowed := false
	for envelope := range channel {
		if envelope.Error != nil {
			return &DNSAXFRResult{
				ResolverIP: resolverIP,
				Domain:     domain,
				Allowed:    false,
				Error:      envelope.Error.Error(),
			}, nil
		}
		allowed = true
		for _, rr := range envelope.RR {
			records = append(records, rr.String())
			if len(records) >= 100 {
				break
			}
		}
	}

	return &DNSAXFRResult{
		ResolverIP:   resolverIP,
		Domain:       domain,
		Allowed:      allowed,
		RecordsCount: len(records),
		Records:      records,
	}, nil
}
