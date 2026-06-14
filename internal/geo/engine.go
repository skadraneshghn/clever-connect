package geo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"clever-connect/internal/db"
	"clever-connect/internal/logger"
	"clever-connect/internal/models"

	"github.com/ip2location/ip2location-go/v9"
	"github.com/oschwald/geoip2-golang"
	"github.com/yinheli/qqwry"
)

type ResultListener func(ip string, data *models.IPRegistry)

// trieNode represents a node in our custom high-performance IP Radix Trie
type trieNode struct {
	children [2]*trieNode
	value    string
}

// IPTrie is a high-performance Patricia tree/radix tree for sub-microsecond CIDR lookups
type IPTrie struct {
	v4Root *trieNode
	v6Root *trieNode
}

func NewIPTrie() *IPTrie {
	return &IPTrie{
		v4Root: &trieNode{},
		v6Root: &trieNode{},
	}
}

func (t *IPTrie) Insert(cidrStr string, value string) error {
	_, ipNet, err := net.ParseCIDR(cidrStr)
	if err != nil {
		return err
	}

	ones, _ := ipNet.Mask.Size()
	ip := ipNet.IP

	var root *trieNode
	if ip.To4() != nil {
		root = t.v4Root
		ip = ip.To4()
	} else {
		root = t.v6Root
	}

	curr := root
	for i := 0; i < ones; i++ {
		byteIdx := i / 8
		bitIdx := 7 - (i % 8)
		bit := (ip[byteIdx] >> bitIdx) & 1

		if curr.children[bit] == nil {
			curr.children[bit] = &trieNode{}
		}
		curr = curr.children[bit]
	}
	curr.value = value
	return nil
}

func (t *IPTrie) Lookup(ip net.IP) (string, bool) {
	var curr *trieNode
	var bits int
	if ip.To4() != nil {
		curr = t.v4Root
		ip = ip.To4()
		bits = 32
	} else {
		curr = t.v6Root
		bits = 128
	}

	var bestValue string
	var found bool

	for i := 0; i < bits; i++ {
		if curr.value != "" {
			bestValue = curr.value
			found = true
		}

		byteIdx := i / 8
		bitIdx := 7 - (i % 8)
		bit := (ip[byteIdx] >> bitIdx) & 1

		if curr.children[bit] == nil {
			break
		}
		curr = curr.children[bit]
	}

	if curr != nil && curr.value != "" {
		bestValue = curr.value
		found = true
	}

	return bestValue, found
}

type Engine struct {
	mu          sync.RWMutex
	cityPath    string
	asnPath     string
	qqwryPath   string
	cityDB      *geoip2.Reader
	asnDB       *geoip2.Reader
	ip2locDB    *ip2location.DB
	qqwryDB     *qqwry.QQwry
	qqwryMu     sync.Mutex
	cidrTrie    *IPTrie
	initialized bool

	// Worker Queue
	queue       chan string
	workerCount int
	wg          sync.WaitGroup
	ctx         context.Context
	cancelFunc  context.CancelFunc

	// WS Listeners
	listMu    sync.RWMutex
	listeners map[string]ResultListener
}

var instance *Engine
var once sync.Once

// GetEngine returns the singleton Geo Engine instance
func GetEngine() *Engine {
	once.Do(func() {
		instance = &Engine{
			workerCount: 5,
			listeners:   make(map[string]ResultListener),
			queue:       make(chan string, 10000),
			cidrTrie:    NewIPTrie(),
		}
	})
	return instance
}

// Init initializes the Geo Engine and ensures local databases are loaded in memory
func (e *Engine) Init(dataDir string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.initialized {
		return nil
	}

	geoDir := filepath.Join(dataDir, "geo")
	if err := os.MkdirAll(geoDir, 0755); err != nil {
		return fmt.Errorf("failed to create geo directory: %w", err)
	}

	// 0. Reset cached IP registries table once for the updated stack schema
	resetLock := filepath.Join(geoDir, "db_reset_v2.lock")
	if _, err := os.Stat(resetLock); os.IsNotExist(err) {
		if db.DB != nil {
			logger.Info("GeoEngine", "Resetting cached IP records in database for GeoIP stack update...")
			db.DB.Exec("DELETE FROM ip_registries")
		}
		_ = os.WriteFile(resetLock, []byte("reset completed"), 0644)
	}

	e.cityPath = filepath.Join(geoDir, "GeoLite2-City.mmdb")
	e.asnPath = filepath.Join(geoDir, "GeoLite2-ASN.mmdb")
	e.qqwryPath = filepath.Join(geoDir, "qqwry.dat")

	logger.Info("GeoEngine", "Checking local IP databases...")

	// 1. Download GeoLite2-City.mmdb
	if _, err := os.Stat(e.cityPath); os.IsNotExist(err) {
		logger.Info("GeoEngine", "GeoLite2-City.mmdb database not found. Downloading...")
		err = downloadWithRetry("https://github.com/P3TERX/GeoLite.mmdb/raw/download/GeoLite2-City.mmdb", e.cityPath)
		if err != nil {
			logger.Error("GeoEngine", "Failed to download GeoLite2-City.mmdb", "error", err)
			return err
		}
	}

	// 2. Download GeoLite2-ASN.mmdb
	if _, err := os.Stat(e.asnPath); os.IsNotExist(err) {
		logger.Info("GeoEngine", "GeoLite2-ASN.mmdb database not found. Downloading...")
		err = downloadWithRetry("https://github.com/P3TERX/GeoLite.mmdb/raw/download/GeoLite2-ASN.mmdb", e.asnPath)
		if err != nil {
			logger.Error("GeoEngine", "Failed to download GeoLite2-ASN.mmdb", "error", err)
			return err
		}
	}

	// 3. Download qqwry.dat for Nali
	if _, err := os.Stat(e.qqwryPath); os.IsNotExist(err) {
		logger.Info("GeoEngine", "qqwry.dat database not found. Downloading...")
		err = downloadWithRetry("https://github.com/metowolf/qqwry.dat/releases/latest/download/qqwry.dat", e.qqwryPath)
		if err != nil {
			logger.Error("GeoEngine", "Failed to download qqwry.dat", "error", err)
			return err
		}
	}

	// 4. Download hosting/CDN provider CIDR files
	cidrDir := filepath.Join(geoDir, "cidrs")
	_ = os.MkdirAll(cidrDir, 0755)

	providers := map[string]string{
		"Cloudflare":      "https://raw.githubusercontent.com/disposable/cloud-ip-ranges/master/json/cloudflare.json",
		"Fastly":          "https://raw.githubusercontent.com/disposable/cloud-ip-ranges/master/json/fastly.json",
		"Akamai":          "https://raw.githubusercontent.com/disposable/cloud-ip-ranges/master/json/akamai.json",
		"AWS / CloudFront": "https://raw.githubusercontent.com/disposable/cloud-ip-ranges/master/json/aws.json",
		"Google Cloud":    "https://raw.githubusercontent.com/disposable/cloud-ip-ranges/master/json/google-cloud.json",
		"Microsoft Azure": "https://raw.githubusercontent.com/disposable/cloud-ip-ranges/master/json/microsoft-azure.json",
		"Oracle Cloud":    "https://raw.githubusercontent.com/disposable/cloud-ip-ranges/master/json/oracle-cloud.json",
		"Hetzner":         "https://raw.githubusercontent.com/disposable/cloud-ip-ranges/master/json/hetzner.json",
		"OVH":             "https://raw.githubusercontent.com/disposable/cloud-ip-ranges/master/json/ovh.json",
		"DigitalOcean":    "https://raw.githubusercontent.com/disposable/cloud-ip-ranges/master/json/digitalocean.json",
		"Linode":          "https://raw.githubusercontent.com/disposable/cloud-ip-ranges/master/json/linode.json",
		"Vultr":           "https://raw.githubusercontent.com/disposable/cloud-ip-ranges/master/json/vultr.json",
	}

	type providerJSON struct {
		IPv4 []string `json:"ipv4"`
		IPv6 []string `json:"ipv6"`
	}

	for name, url := range providers {
		localPath := filepath.Join(cidrDir, strings.ToLower(strings.ReplaceAll(name, " ", "_"))+".json")
		if _, err := os.Stat(localPath); os.IsNotExist(err) {
			logger.Info("GeoEngine", "Downloading provider CIDR list...", "provider", name)
			_ = downloadWithRetry(url, localPath)
		}

		// Read and load into IPTrie
		fileBytes, err := os.ReadFile(localPath)
		if err == nil {
			var data providerJSON
			if err := json.Unmarshal(fileBytes, &data); err == nil {
				for _, cidr := range data.IPv4 {
					_ = e.cidrTrie.Insert(cidr, name)
				}
				for _, cidr := range data.IPv6 {
					_ = e.cidrTrie.Insert(cidr, name)
				}
			}
		}
	}

	// 5. Load MaxMind Databases
	var err error
	e.cityDB, err = geoip2.Open(e.cityPath)
	if err != nil {
		logger.Error("GeoEngine", "Failed to load GeoLite2-City.mmdb", "error", err)
		return err
	}

	e.asnDB, err = geoip2.Open(e.asnPath)
	if err != nil {
		logger.Error("GeoEngine", "Failed to load GeoLite2-ASN.mmdb", "error", err)
		e.cityDB.Close()
		return err
	}

	// 6. Load IP2Location BIN Database if present
	var ip2locPath string
	files, err := os.ReadDir(geoDir)
	if err == nil {
		for _, f := range files {
			if !f.IsDir() {
				nameLower := strings.ToLower(f.Name())
				if strings.HasPrefix(nameLower, "ip2location") && strings.HasSuffix(nameLower, ".bin") {
					ip2locPath = filepath.Join(geoDir, f.Name())
					break
				}
			}
		}
	}

	if ip2locPath != "" {
		logger.Info("GeoEngine", "Found IP2Location database", "path", ip2locPath)
		db, err := ip2location.OpenDB(ip2locPath)
		if err == nil {
			e.ip2locDB = db
			logger.Info("GeoEngine", "IP2Location database loaded successfully")
		} else {
			logger.Error("GeoEngine", "Failed to open IP2Location database", "error", err)
		}
	} else {
		logger.Info("GeoEngine", "IP2Location database (.bin) not found, skipping...")
	}

	// 7. Load Nali QQwry
	e.qqwryDB = qqwry.NewQQwry(e.qqwryPath)

	e.initialized = true
	logger.Info("GeoEngine", "GeoIP Stack (MaxMind + CIDR Trie + Nali + IP2Location) initialized successfully")

	// Start worker pool
	e.ctx, e.cancelFunc = context.WithCancel(context.Background())
	for i := 0; i < e.workerCount; i++ {
		e.wg.Add(1)
		go e.worker(e.ctx)
	}

	return nil
}

// Close stops the engine workers and closes databases
func (e *Engine) Close() {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.cancelFunc != nil {
		e.cancelFunc()
	}
	e.wg.Wait()

	if e.cityDB != nil {
		e.cityDB.Close()
		e.cityDB = nil
	}
	if e.asnDB != nil {
		e.asnDB.Close()
		e.asnDB = nil
	}
	if e.ip2locDB != nil {
		e.ip2locDB.Close()
		e.ip2locDB = nil
	}

	e.initialized = false
}

// QueueResolveIP queues an IP for asynchronous non-blocking resolution
func (e *Engine) QueueResolveIP(ip string) {
	ip = strings.TrimSpace(ip)
	if ip == "" || net.ParseIP(ip) == nil {
		return
	}

	select {
	case e.queue <- ip:
	default:
		// Queue full
	}
}

// RegisterListener registers a callback to receive real-time updates when an IP is resolved
func (e *Engine) RegisterListener(id string, listener ResultListener) {
	e.listMu.Lock()
	defer e.listMu.Unlock()
	e.listeners[id] = listener
}

// UnregisterListener removes a registered listener
func (e *Engine) UnregisterListener(id string) {
	e.listMu.Lock()
	defer e.listMu.Unlock()
	delete(e.listeners, id)
}

func (e *Engine) broadcast(ip string, data *models.IPRegistry) {
	e.listMu.RLock()
	defer e.listMu.RUnlock()
	for _, listener := range e.listeners {
		go listener(ip, data)
	}
}

func (e *Engine) worker(ctx context.Context) {
	defer e.wg.Done()
	for {
		select {
		case <-ctx.Done():
			return
		case ip, ok := <-e.queue:
			if !ok {
				return
			}
			_, _ = e.ResolveIP(ip, false)
		}
	}
}

// cleanIP2LocStr normalizes IP2Location field values
func cleanIP2LocStr(val string) string {
	s := strings.TrimSpace(val)
	sLower := strings.ToLower(s)
	if s == "" || strings.Contains(sLower, "not_supported") || strings.Contains(sLower, "unavailable") || strings.Contains(sLower, "invalid") {
		return ""
	}
	return s
}

// ResolveIP queries an IP location and CDN provider. If force is true, it bypasses database cache.
func (e *Engine) ResolveIP(ip string, force bool) (*models.IPRegistry, error) {
	ip = strings.TrimSpace(ip)
	if ip == "" {
		return nil, errors.New("empty IP address")
	}

	parsedIP := net.ParseIP(ip)
	if parsedIP == nil {
		return nil, fmt.Errorf("invalid IP address: %s", ip)
	}

	// Intercept local loopback, link-local, and private IP ranges
	isPrivate := parsedIP.IsPrivate() || parsedIP.IsLoopback() || parsedIP.IsLinkLocalUnicast() || parsedIP.IsLinkLocalMulticast()
	if isPrivate {
		reg := &models.IPRegistry{
			IP:          ip,
			CountryCode: "PV",
			CountryName: "Private Network",
			City:        "Local Intranet",
			ISP:         "Private Server IP",
			Latitude:    0.0,
			Longitude:   0.0,
			IsCDN:       false,
			LastUpdated: time.Now(),
		}
		if parsedIP.IsLoopback() {
			reg.City = "Localhost"
			reg.ISP = "System Loopback"
		} else if strings.HasPrefix(ip, "192.168.") {
			reg.ISP = "Home/Office Router Subnet"
		} else if strings.HasPrefix(ip, "10.") {
			reg.ISP = "Corporate Private Subnet"
		} else if strings.HasPrefix(ip, "172.16.") || strings.HasPrefix(ip, "172.17.") || strings.HasPrefix(ip, "172.18.") || strings.HasPrefix(ip, "172.19.") || strings.HasPrefix(ip, "172.2") || strings.HasPrefix(ip, "172.3") {
			reg.ISP = "Local Virtual/Container Subnet"
		}

		if db.DB != nil {
			_ = db.DB.Save(reg)
		}
		e.broadcast(ip, reg)
		return reg, nil
	}

	e.mu.RLock()
	init := e.initialized
	e.mu.RUnlock()

	if !init {
		return nil, errors.New("geo engine not initialized")
	}

	// 1. Check local database cache if not forced
	if !force {
		var cache models.IPRegistry
		if db.DB != nil {
			err := db.DB.Where("ip = ?", ip).First(&cache).Error
			if err == nil {
				// Check if cache is fresh (less than 30 days old)
				if time.Since(cache.LastUpdated) < 30*24*time.Hour {
					return &cache, nil
				}
			}
		}
	}

	// 2. Perform offline lookup across stack
	e.mu.RLock()
	cityDB := e.cityDB
	asnDB := e.asnDB
	ip2loc := e.ip2locDB
	e.mu.RUnlock()

	var countryCode string
	var countryName string
	var city string
	var isp string
	var latitude float64
	var longitude float64
	var isCDN bool
	var cdnProvider string

	// Step A: Check CIDR Trie first for Hosting / CDN
	if provider, matched := e.cidrTrie.Lookup(parsedIP); matched {
		// Set provider details
		isCDN = true
		cdnProvider = provider
		isp = provider

		// Map traditional CDNs specifically
		lowerProv := strings.ToLower(provider)
		if lowerProv == "cloudflare" || lowerProv == "fastly" || lowerProv == "akamai" || strings.Contains(lowerProv, "cloudfront") {
			// Pure CDN
		} else {
			// VPS / Hosting
		}
	}

	// Step B: IP2Location Lookup
	if ip2loc != nil {
		res, err := ip2loc.Get_all(ip)
		if err == nil {
			countryCode = cleanIP2LocStr(res.Country_short)
			countryName = cleanIP2LocStr(res.Country_long)
			regStr := cleanIP2LocStr(res.Region)
			cityStr := cleanIP2LocStr(res.City)
			if regStr != "" && cityStr != "" {
				city = cityStr + ", " + regStr
			} else if cityStr != "" {
				city = cityStr
			} else if regStr != "" {
				city = regStr
			}

			if cleanIP2LocStr(res.Isp) != "" {
				isp = cleanIP2LocStr(res.Isp)
			}
			if res.Latitude != 0 || res.Longitude != 0 {
				latitude = float64(res.Latitude)
				longitude = float64(res.Longitude)
			}
		}
	}

	// Step C: GeoLite2-City Lookup
	if cityDB != nil && (countryCode == "" || city == "" || (latitude == 0 && longitude == 0)) {
		cityRecord, err := cityDB.City(parsedIP)
		if err == nil && cityRecord != nil {
			if countryCode == "" {
				countryCode = cityRecord.Country.IsoCode
			}
			if countryName == "" {
				countryName = cityRecord.Country.Names["en"]
			}
			if city == "" {
				cityName := cityRecord.City.Names["en"]
				var regionName string
				if len(cityRecord.Subdivisions) > 0 {
					regionName = cityRecord.Subdivisions[0].Names["en"]
				}
				if cityName != "" && regionName != "" {
					city = cityName + ", " + regionName
				} else if cityName != "" {
					city = cityName
				} else if regionName != "" {
					city = regionName
				}
			}
			if latitude == 0 && longitude == 0 {
				latitude = cityRecord.Location.Latitude
				longitude = cityRecord.Location.Longitude
			}
		}
	}

	// Step D: GeoLite2-ASN Lookup
	if asnDB != nil && (isp == "" || isp == "Unknown ISP") {
		asnRecord, err := asnDB.ASN(parsedIP)
		if err == nil && asnRecord != nil {
			asOrg := asnRecord.AutonomousSystemOrganization
			asNum := asnRecord.AutonomousSystemNumber
			if asOrg != "" {
				isp = fmt.Sprintf("AS%d %s", asNum, asOrg)
			} else {
				isp = fmt.Sprintf("AS%d", asNum)
			}
		}
	}

	// Step E: Fallback to Nali / QQwry (IPv4 English translation fallback)
	if (countryName == "" || city == "" || isp == "") && parsedIP.To4() != nil {
		e.qqwryMu.Lock()
		if e.qqwryDB != nil {
			e.qqwryDB.Find(ip)
			qCountry := e.qqwryDB.Country
			qCity := e.qqwryDB.City

			// Translate Country
			var matchedCode string
			var matchedName string
			var matchedLat float64
			var matchedLng float64

			var bestMatch string
			for k, info := range countryMap {
				if strings.Contains(qCountry, k) {
					if len(k) > len(bestMatch) {
						bestMatch = k
						matchedCode = info.Code
						matchedName = info.Name
						matchedLat = info.Lat
						matchedLng = info.Lng
					}
				}
			}

			if countryCode == "" && matchedCode != "" {
				countryCode = matchedCode
				countryName = matchedName
				if latitude == 0 && longitude == 0 {
					latitude = matchedLat
					longitude = matchedLng
				}
			}

			// Translate City/Area
			if qCity != "" {
				translatedCity := ""
				if trans, ok := cityTranslationMap[qCity]; ok {
					translatedCity = trans
				} else {
					translatedCity = translateToEnglish(qCity)
				}

				if city == "" {
					city = translatedCity
				}
				if isp == "" || isp == "Unknown ISP" {
					isp = translatedCity
				}
			}
		}
		e.qqwryMu.Unlock()
	}

	// Fallbacks if unresolved
	if countryCode == "" {
		countryCode = "GL"
		countryName = "Global / Anycast"
	}
	if city == "" {
		city = "Global"
	}
	if isp == "" {
		isp = "Unknown ISP"
	}

	// If CIDR trie matched provider, override ISP / CDN if empty
	if isCDN {
		if cdnProvider == "" {
			cdnProvider = isp
		}
	} else {
		// Heuristic checks from ISP text
		ispLower := strings.ToLower(isp)
		if strings.Contains(ispLower, "cloudflare") {
			isCDN = true
			cdnProvider = "Cloudflare"
		} else if strings.Contains(ispLower, "akamai") {
			isCDN = true
			cdnProvider = "Akamai"
		} else if strings.Contains(ispLower, "fastly") {
			isCDN = true
			cdnProvider = "Fastly"
		} else if strings.Contains(ispLower, "cloudfront") || strings.Contains(ispLower, "amazon technologies") {
			isCDN = true
			cdnProvider = "Amazon CloudFront"
		} else if strings.Contains(ispLower, "hetzner") {
			isCDN = true
			cdnProvider = "Hetzner"
		} else if strings.Contains(ispLower, "ovh") {
			isCDN = true
			cdnProvider = "OVH"
		} else if strings.Contains(ispLower, "digitalocean") {
			isCDN = true
			cdnProvider = "DigitalOcean"
		} else if strings.Contains(ispLower, "linode") {
			isCDN = true
			cdnProvider = "Linode"
		} else if strings.Contains(ispLower, "vultr") {
			isCDN = true
			cdnProvider = "Vultr"
		} else if strings.Contains(ispLower, "oracle") {
			isCDN = true
			cdnProvider = "Oracle Cloud"
		} else if strings.Contains(ispLower, "google") {
			isCDN = true
			cdnProvider = "Google Cloud"
		} else if strings.Contains(ispLower, "azure") || strings.Contains(ispLower, "microsoft") {
			isCDN = true
			cdnProvider = "Microsoft Azure"
		}
	}

	reg := &models.IPRegistry{
		IP:          ip,
		CountryCode: countryCode,
		CountryName: countryName,
		City:        city,
		ISP:         isp,
		IsCDN:       isCDN,
		CDNProvider: cdnProvider,
		Latitude:    latitude,
		Longitude:   longitude,
		LastUpdated: time.Now(),
	}

	// 5. Save to DB
	if db.DB != nil {
		err := db.DB.Save(reg).Error
		if err != nil {
			logger.Error("GeoEngine", "Failed to save resolved IP to registry", "ip", ip, "error", err)
		}
	}

	// 6. Broadcast to UI
	e.broadcast(ip, reg)

	return reg, nil
}

// downloadWithRetry retrieves a file from a URL, writing it to dest, with retry mechanism
func downloadWithRetry(url, dest string) error {
	var lastErr error
	client := &http.Client{Timeout: 10 * time.Minute}

	for attempt := 1; attempt <= 3; attempt++ {
		resp, err := client.Get(url)
		if err == nil {
			defer resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				out, err := os.Create(dest)
				if err != nil {
					return err
				}
				defer out.Close()
				_, err = io.Copy(out, resp.Body)
				return err
			}
			lastErr = fmt.Errorf("HTTP status: %s", resp.Status)
		} else {
			lastErr = err
		}
		logger.Warn("GeoEngine", "Download failed, retrying...", "url", url, "attempt", attempt, "error", lastErr)
		time.Sleep(3 * time.Second)
	}

	return lastErr
}

// --- Translation Dictionary and Helpers for QQwry ---

type CountryInfo struct {
	Code string
	Name string
	Lat  float64
	Lng  float64
}

var countryMap = map[string]CountryInfo{
	"美国":   {"US", "United States", 37.0902, -95.7129},
	"中国":   {"CN", "China", 35.8617, 104.1954},
	"伊朗":   {"IR", "Iran", 32.4279, 53.6880},
	"德国":   {"DE", "Germany", 51.1657, 10.4515},
	"英国":   {"GB", "United Kingdom", 55.3781, -3.4360},
	"法国":   {"FR", "France", 46.2276, 2.2137},
	"新加坡":  {"SG", "Singapore", 1.3521, 103.8198},
	"日本":   {"JP", "Japan", 36.2048, 138.2529},
	"韩国":   {"KR", "South Korea", 35.9078, 127.7669},
	"荷兰":   {"NL", "Netherlands", 52.1326, 5.2913},
	"芬兰":   {"FI", "Finland", 61.9241, 25.7482},
	"加拿大":  {"CA", "Canada", 56.1304, -106.3468},
	"香港":   {"HK", "Hong Kong", 22.3964, 114.1095},
	"澳门":   {"MO", "Macao", 22.1987, 113.5439},
	"台湾":   {"TW", "Taiwan", 23.6978, 120.9605},
	"土耳其":  {"TR", "Turkey", 38.9637, 35.2433},
	"俄罗斯":  {"RU", "Russia", 61.5240, 105.3188},
	"澳大利亚": {"AU", "Australia", -25.2744, 133.7751},
	"阿联酋":  {"AE", "United Arab Emirates", 23.4241, 53.8478},
	"沙特":   {"SA", "Saudi Arabia", 23.8859, 45.0792},
	"伊拉克":  {"IQ", "Iraq", 33.2232, 43.6793},
	"阿富汗":  {"AF", "Afghanistan", 33.9391, 67.7100},
	"印度":   {"IN", "India", 20.5937, 78.9629},
	"巴西":   {"BR", "Brazil", -14.2350, -51.9253},
	"南非":   {"ZA", "South Africa", -30.5595, 22.9375},
	"马来西亚": {"MY", "Malaysia", 4.2105, 101.9758},
	"泰国":   {"TH", "Thailand", 15.8700, 100.9925},
	"越南":   {"VN", "Vietnam", 14.0583, 108.2772},
	"印度尼西亚": {"ID", "Indonesia", -0.7893, 113.9213},
	"菲律宾":  {"PH", "Philippines", 12.8797, 121.7740},
	"瑞典":   {"SE", "Sweden", 60.1282, 18.6435},
	"瑞士":   {"CH", "Switzerland", 46.8182, 8.2275},
	"意大利":  {"IT", "Italy", 41.8719, 12.5674},
	"西班牙":  {"ES", "Spain", 40.4637, -3.7492},
	"乌克兰":  {"UA", "Ukraine", 48.3794, 31.1656},
	"波兰":   {"PL", "Poland", 51.9194, 19.1451},
	"奥地利":  {"AT", "Austria", 47.5162, 14.5501},
	"比利时":  {"BE", "Belgium", 50.5039, 4.4699},
	"爱尔兰":  {"IE", "Ireland", 53.4129, -8.2439},
	"丹麦":   {"DK", "Denmark", 56.2639, 9.5018},
	"挪威":   {"NO", "Norway", 60.4720, 8.4689},
	"卢森堡":  {"LU", "Luxembourg", 49.8153, 6.1296},
	"局域网":  {"PV", "Private Network", 0.0, 0.0},
	"本机":   {"PV", "Localhost", 0.0, 0.0},
}

var cityTranslationMap = map[string]string{
	"北京":     "Beijing",
	"上海":     "Shanghai",
	"广州":     "Guangzhou",
	"深圳":     "Shenzhen",
	"杭州":     "Hangzhou",
	"山景城":    "Mountain View",
	"圣克拉拉":  "Santa Clara",
	"西雅图":    "Seattle",
	"伦敦":     "London",
	"巴黎":     "Paris",
	"法兰克福":  "Frankfurt",
	"新加坡":    "Singapore",
	"东京":     "Tokyo",
	"首尔":     "Seoul",
	"香港":     "Hong Kong",
	"台北":     "Taipei",
	"阿姆斯特丹": "Amsterdam",
	"赫尔辛基":  "Helsinki",
	"德黑兰":    "Tehran",
	"伊斯法罕":  "Isfahan",
	"设拉子":    "Shiraz",
	"马什哈德":  "Mashhad",
	"大不里士":  "Tabriz",
}

type replacementRule struct {
	cn string
	en string
}

var replacements = []replacementRule{
	{"局域网", "LAN"},
	{"本机", "Localhost"},
	{"互联网", "Internet"},
	{"数据中心", " Data Center"},
	{"机房", " Data Center"},
	{"服务器", " Server"},
	{"运营商", " ISP"},
	{"公司", " Company"},
	{"专用", " Private"},
	{"共享", " Shared"},
	{"公共", " Public "},
	{"骨干网", "Backbone"},
	{"多线", "Multi-line"},
	{"高防", "DDoS Protected"},
	{"节点", " Node"},

	// US States
	{"加利福尼亚州", "California"},
	{"加利福尼亚", "California"},
	{"加州", "California"},
	{"弗吉尼亚州", "Virginia"},
	{"弗吉尼亚", "Virginia"},
	{"德克萨斯州", "Texas"},
	{"德克萨斯", "Texas"},
	{"德州", "Texas"},
	{"华盛顿州", "Washington State"},
	{"华盛顿", "Washington"},
	{"纽约州", "New York State"},
	{"纽约市", "New York City"},
	{"纽约", "New York"},
	{"俄勒冈州", "Oregon"},
	{"俄勒冈", "Oregon"},
	{"伊利诺伊州", "Illinois"},
	{"伊利诺伊", "Illinois"},
	{"佐治亚州", "Georgia"},
	{"佐治亚", "Georgia"},
	{"马萨诸塞州", "Massachusetts"},
	{"马萨诸塞", "Massachusetts"},
	{"新泽西州", "New Jersey"},
	{"新泽西", "New Jersey"},
	{"科罗拉多州", "Colorado"},
	{"科罗拉多", "Colorado"},
	{"北卡罗来纳州", "North Carolina"},
	{"北卡罗来纳", "North Carolina"},
	{"密歇根州", "Michigan"},
	{"密歇根", "Michigan"},
	{"亚利桑那州", "Arizona"},
	{"亚利桑那", "Arizona"},
	{"佛罗里达州", "Florida"},
	{"佛罗里达", "Florida"},
	{"俄亥俄州", "Ohio"},
	{"俄亥俄", "Ohio"},
	{"犹他州", "Utah"},
	{"犹他", "Utah"},
	{"宾夕法尼亚州", "Pennsylvania"},
	{"宾夕法尼亚", "Pennsylvania"},
	{"内华达州", "Nevada"},
	{"内华达", "Nevada"},
	{"马里兰州", "Maryland"},
	{"马里兰", "Maryland"},
	{"堪萨斯州", "Kansas"},
	{"堪萨斯", "Kansas"},
	{"明尼苏达州", "Minnesota"},
	{"明尼苏达", "Minnesota"},
	{"威斯康星州", "Wisconsin"},
	{"威斯康星", "Wisconsin"},

	// Chinese Provinces/Cities
	{"北京市", "Beijing"},
	{"北京", "Beijing"},
	{"上海市", "Shanghai"},
	{"上海", "Shanghai"},
	{"天津市", "Tianjin"},
	{"天津", "Tianjin"},
	{"重庆市", "Chongqing"},
	{"重庆", "Chongqing"},
	{"广东省", "Guangdong"},
	{"广东", "Guangdong"},
	{"广州市", "Guangzhou"},
	{"广州", "Guangzhou"},
	{"深圳市", "Shenzhen"},
	{"深圳", "Shenzhen"},
	{"浙江省", "Zhejiang"},
	{"浙江", "Zhejiang"},
	{"杭州市", "Hangzhou"},
	{"杭州", "Hangzhou"},
	{"江苏省", "Jiangsu"},
	{"江苏", "Jiangsu"},
	{"南京市", "Nanjing"},
	{"南京", "Nanjing"},
	{"四川省", "Sichuan"},
	{"四川", "Sichuan"},
	{"成都市", "Chengdu"},
	{"成都", "Chengdu"},
	{"湖北省", "Hubei"},
	{"湖北", "Hubei"},
	{"武汉市", "Wuhan"},
	{"武汉", "Wuhan"},
	{"陕西省", "Shaanxi"},
	{"陕西", "Shaanxi"},
	{"西安市", "Xi'an"},
	{"西安", "Xi'an"},
	{"福建省", "Fujian"},
	{"福建", "Fujian"},
	{"山东省", "Shandong"},
	{"山东", "Shandong"},
	{"河南省", "Henan"},
	{"河南", "Henan"},
	{"河北省", "Hebei"},
	{"河北", "Hebei"},
	{"山西省", "Shanxi"},
	{"山西", "Shanxi"},
	{"辽宁省", "Liaoning"},
	{"辽宁", "Liaoning"},
	{"吉林省", "Jilin"},
	{"吉林", "Jilin"},
	{"黑龙江省", "Heilongjiang"},
	{"黑龙江", "Heilongjiang"},
	{"安徽省", "Anhui"},
	{"安徽", "Anhui"},
	{"江西省", "Jiangxi"},
	{"江西", "Jiangxi"},
	{"湖南省", "Hunan"},
	{"湖南", "Hunan"},
	{"海南省", "Hainan"},
	{"海南", "Hainan"},
	{"贵州省", "Guizhou"},
	{"贵州", "Guizhou"},
	{"云南省", "Yunnan"},
	{"云南", "Yunnan"},
	{"甘肃省", "Gansu"},
	{"甘肃", "Gansu"},
	{"青海省", "Qinghai"},
	{"青海", "Qinghai"},
	{"内蒙古自治区", "Inner Mongolia"},
	{"内蒙古", "Inner Mongolia"},
	{"广西壮族自治区", "Guangxi"},
	{"广西", "Guangxi"},
	{"西藏自治区", "Tibet"},
	{"西藏", "Tibet"},
	{"宁夏回族自治区", "Ningxia"},
	{"宁夏", "Ningxia"},
	{"新疆维吾尔自治区", "Xinjiang"},
	{"新疆", "Xinjiang"},
	{"香港特别行政区", "Hong Kong"},
	{"香港", "Hong Kong"},
	{"澳门特别行政区", "Macao"},
	{"澳门", "Macao"},
	{"台湾省", "Taiwan"},
	{"台湾", "Taiwan"},

	// Common International Cities
	{"东京", "Tokyo"},
	{"大阪", "Osaka"},
	{"首尔", "Seoul"},
	{"伦敦", "London"},
	{"巴黎", "Paris"},
	{"法兰克福", "Frankfurt"},
	{"阿姆斯特丹", "Amsterdam"},
	{"赫尔辛基", "Helsinki"},
	{"山景城", "Mountain View"},
	{"圣克拉拉", "Santa Clara"},
	{"西雅图", "Seattle"},
	{"芝加哥", "Chicago"},
	{"洛杉矶", "Los Angeles"},
	{"旧金山", "San Francisco"},
	{"达拉斯", "Dallas"},
	{"休斯敦", "Houston"},
	{"迈阿密", "Miami"},
	{"波士顿", "Boston"},
	{"亚特兰大", "Atlanta"},
	{"阿什本", "Ashburn"},
	{"波特兰", "Portland"},
	{"丹佛", "Denver"},
	{"凤凰城", "Phoenix"},
	{"盐湖城", "Salt Lake City"},
	{"德黑兰", "Tehran"},
	{"伊斯法罕", "Isfahan"},
	{"设拉子", "Shiraz"},
	{"马什哈德", "Mashhad"},
	{"大不里士", "Tabriz"},

	// ISPs
	{"联通", "China Unicom"},
	{"电信", "China Telecom"},
	{"移动", "China Mobile"},
	{"铁通", "China Tietong"},
	{"广电", "China Broadband"},
	{"教育网", "CERNET"},
	{"阿里云", "Alibaba Cloud"},
	{"腾讯云", "Tencent Cloud"},
	{"华为云", "Huawei Cloud"},
	{"金山云", "Kingsoft Cloud"},
	{"百度云", "Baidu Cloud"},
	{"世纪互联", "21Vianet"},
	{"谷歌公司", "Google LLC"},
	{"谷歌", "Google"},
	{"微软公司", "Microsoft Corp"},
	{"微软", "Microsoft"},
	{"亚马逊公司", "Amazon.com"},
	{"亚马逊", "Amazon"},
	{"脸书", "Facebook"},
	{"推特", "Twitter"},
	{"甲骨文", "Oracle"},
}

func translateToEnglish(text string) string {
	if text == "" {
		return ""
	}

	result := text
	for _, r := range replacements {
		if strings.Contains(result, r.cn) {
			result = strings.ReplaceAll(result, r.cn, r.en)
		}
	}

	// Guarantee absolutely NO Chinese characters ever remain in the database
	var sb strings.Builder
	for _, r := range result {
		// Filter CJK Unified Ideographs, Extension A, and Compatibility Ideographs
		if (r >= 0x4E00 && r <= 0x9FFF) || (r >= 0x3400 && r <= 0x4DBF) || (r >= 0xF900 && r <= 0xFAFF) {
			continue
		}
		sb.WriteRune(r)
	}
	result = sb.String()

	result = strings.TrimSpace(result)
	for strings.Contains(result, "  ") {
		result = strings.ReplaceAll(result, "  ", " ")
	}
	return result
}
