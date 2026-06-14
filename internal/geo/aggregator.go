package geo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"clever-connect/internal/db"
	"clever-connect/internal/logger"
	"clever-connect/internal/models"
)

// UnifiedIPResult is the standardized model returned to the client
type UnifiedIPResult struct {
	IP          string    `json:"ip"`
	Country     string    `json:"country"`
	CountryCode string    `json:"country_code"`
	City        string    `json:"city"`
	ASN         string    `json:"asn"`
	ISP         string    `json:"isp"`
	Latitude    float64   `json:"latitude"`
	Longitude   float64   `json:"longitude"`
	ProxyStatus string    `json:"proxy_status"` // VPN/Tor/DCH/Clean
	IsProxy     bool      `json:"is_proxy"`
	Provider    string    `json:"provider"`
	RawJSON     string    `json:"raw_json"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// UnifiedWhoisResult is the standardized domain whois info
type UnifiedWhoisResult struct {
	DomainName   string    `json:"domain_name"`
	Registrar    string    `json:"registrar"`
	CreationDate string    `json:"creation_date"`
	ExpiryDate   string    `json:"expiry_date"`
	NameServers  []string  `json:"name_servers"`
	RawJSON      string    `json:"raw_json"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// UnifiedLookupResponse is the overall API response
type UnifiedLookupResponse struct {
	Target      string              `json:"target"`
	Type        string              `json:"type"` // ip, domain
	ResolvedIP  string              `json:"resolved_ip,omitempty"`
	Geo         *UnifiedIPResult    `json:"geo,omitempty"`
	Whois       *UnifiedWhoisResult `json:"whois,omitempty"`
	Source      string              `json:"source"` // cache, api
	QuotaError  bool                `json:"quota_error"`
	ErrorMsg    string              `json:"error_msg,omitempty"`
}

// GetIPLookupConfig fetches the singleton config
func GetIPLookupConfig() (*models.IPLookupConfig, error) {
	if db.DB == nil {
		return nil, errors.New("database not initialized")
	}
	var cfg models.IPLookupConfig
	err := db.DB.First(&cfg).Error
	if err != nil {
		// return default config
		cfg = models.IPLookupConfig{
			EnableIP2Location:   true,
			EnableIpApi:         true,
			EnableIpGeolocation: true,
			EnableIpWhois:       true,
			EnableFindIP:        true,
		}
		db.DB.Create(&cfg)
	}
	return &cfg, nil
}

// SaveIPLookupConfig saves the singleton config
func SaveIPLookupConfig(newCfg *models.IPLookupConfig) error {
	if db.DB == nil {
		return errors.New("database not initialized")
	}
	var existing models.IPLookupConfig
	err := db.DB.First(&existing).Error
	if err != nil {
		return db.DB.Create(newCfg).Error
	}
	newCfg.ID = existing.ID
	return db.DB.Save(newCfg).Error
}

// ClassifyTarget classifies string as ip, domain, or invalid
func ClassifyTarget(target string) string {
	target = strings.TrimSpace(target)
	if ip := net.ParseIP(target); ip != nil {
		return "ip"
	}
	if strings.Contains(target, ".") && !strings.Contains(target, " ") && !strings.Contains(target, "/") {
		return "domain"
	}
	return "invalid"
}

// QueryIPIntelligenceCache checks level 1 database cache
func QueryIPIntelligenceCache(ip string) (*UnifiedIPResult, bool) {
	if db.DB == nil {
		return nil, false
	}
	var cache models.IPIntelligenceCache
	err := db.DB.Where("ip = ?", ip).First(&cache).Error
	if err == nil {
		// TTL: 10 days
		if time.Since(cache.UpdatedAt) < 10*24*time.Hour {
			return &UnifiedIPResult{
				IP:          cache.IP,
				Country:     cache.Country,
				CountryCode: cache.CountryCode,
				City:        cache.City,
				ASN:         cache.ASN,
				ISP:         cache.ISP,
				Latitude:    cache.Latitude,
				Longitude:   cache.Longitude,
				ProxyStatus: cache.ProxyStatus,
				IsProxy:     cache.ProxyStatus != "" && cache.ProxyStatus != "Clean",
				Provider:    "Local DB Cache",
				RawJSON:     cache.RawJSON,
				UpdatedAt:   cache.UpdatedAt,
			}, true
		}
	}
	return nil, false
}

// QueryDomainWhoisCache checks level 1 database cache
func QueryDomainWhoisCache(domain string) (*UnifiedWhoisResult, bool) {
	if db.DB == nil {
		return nil, false
	}
	domain = strings.ToLower(strings.TrimSpace(domain))
	var cache models.DomainWhoisCache
	err := db.DB.Where("domain_name = ?", domain).First(&cache).Error
	if err == nil {
		// TTL: 24 hours
		if time.Since(cache.UpdatedAt) < 24*time.Hour {
			var ns []string
			if cache.NameServers != "" {
				ns = strings.Split(cache.NameServers, ",")
			}
			return &UnifiedWhoisResult{
				DomainName:   cache.DomainName,
				Registrar:    cache.Registrar,
				CreationDate: cache.CreationDate,
				ExpiryDate:   cache.ExpiryDate,
				NameServers:  ns,
				RawJSON:      cache.RawJSON,
				UpdatedAt:    cache.UpdatedAt,
			}, true
		}
	}
	return nil, false
}

// SaveIPToCache writes resolved IP geo details to cache
func SaveIPToCache(res *UnifiedIPResult) {
	if db.DB == nil {
		return
	}
	cache := models.IPIntelligenceCache{
		IP:          res.IP,
		Country:     res.Country,
		CountryCode: res.CountryCode,
		City:        res.City,
		ASN:         res.ASN,
		ISP:         res.ISP,
		Latitude:    res.Latitude,
		Longitude:   res.Longitude,
		ProxyStatus: res.ProxyStatus,
		RawJSON:     res.RawJSON,
		UpdatedAt:   time.Now(),
	}
	if err := db.DB.Save(&cache).Error; err != nil {
		logger.Error("IPLookup", "Failed to cache IP details", "ip", res.IP, "error", err)
	}
}

// SaveDomainToCache writes resolved domain WHOIS details to cache
func SaveDomainToCache(res *UnifiedWhoisResult) {
	if db.DB == nil {
		return
	}
	cache := models.DomainWhoisCache{
		DomainName:   strings.ToLower(res.DomainName),
		Registrar:    res.Registrar,
		CreationDate: res.CreationDate,
		ExpiryDate:   res.ExpiryDate,
		NameServers:  strings.Join(res.NameServers, ","),
		RawJSON:      res.RawJSON,
		UpdatedAt:    time.Now(),
	}
	if err := db.DB.Save(&cache).Error; err != nil {
		logger.Error("IPLookup", "Failed to cache domain whois", "domain", res.DomainName, "error", err)
	}
}

// ConcurrentGeoResolver runs multiple API calls in parallel and takes the fastest successful result
func ConcurrentGeoResolver(ctx context.Context, ip string, cfg *models.IPLookupConfig) (*UnifiedIPResult, error) {
	type apiResult struct {
		res *UnifiedIPResult
		err error
	}

	resultChan := make(chan apiResult, 5)
	var wg sync.WaitGroup
	ctxCancel, cancel := context.WithCancel(ctx)
	defer cancel()

	// 1. IP2Location.io (Primary API key requested)
	if cfg.EnableIP2Location && cfg.IP2LocationKey != "" {
		wg.Add(1)
		go func() {
			defer wg.Done()
			res, err := fetchIP2Location(ctxCancel, ip, cfg.IP2LocationKey)
			if err == nil {
				select {
				case resultChan <- apiResult{res: res}:
					cancel() // Cancel other requests immediately
				case <-ctxCancel.Done():
				}
			} else {
				logger.Warn("IPLookup", "IP2Location.io query failed", "ip", ip, "error", err)
			}
		}()
	}

	// 2. IP-API.com (Free or Pro, uses free here)
	if cfg.EnableIpApi {
		wg.Add(1)
		go func() {
			defer wg.Done()
			res, err := fetchIpApi(ctxCancel, ip, cfg.IpApiKey)
			if err == nil {
				select {
				case resultChan <- apiResult{res: res}:
					cancel()
				case <-ctxCancel.Done():
				}
			} else {
				logger.Warn("IPLookup", "IP-API.com query failed", "ip", ip, "error", err)
			}
		}()
	}

	// 3. FindIP.net
	if cfg.EnableFindIP {
		wg.Add(1)
		go func() {
			defer wg.Done()
			res, err := fetchFindIP(ctxCancel, ip, cfg.FindIPKey)
			if err == nil {
				select {
				case resultChan <- apiResult{res: res}:
					cancel()
				case <-ctxCancel.Done():
				}
			} else {
				logger.Warn("IPLookup", "FindIP.net query failed", "ip", ip, "error", err)
			}
		}()
	}

	// 4. IPGeolocation.io
	if cfg.EnableIpGeolocation && cfg.IpGeolocationKey != "" {
		wg.Add(1)
		go func() {
			defer wg.Done()
			res, err := fetchIpGeolocation(ctxCancel, ip, cfg.IpGeolocationKey)
			if err == nil {
				select {
				case resultChan <- apiResult{res: res}:
					cancel()
				case <-ctxCancel.Done():
				}
			} else {
				logger.Warn("IPLookup", "IPGeolocation.io query failed", "ip", ip, "error", err)
			}
		}()
	}

	// 5. IPWhois.io
	if cfg.EnableIpWhois {
		wg.Add(1)
		go func() {
			defer wg.Done()
			res, err := fetchIpWhois(ctxCancel, ip, cfg.IpWhoisKey)
			if err == nil {
				select {
				case resultChan <- apiResult{res: res}:
					cancel()
				case <-ctxCancel.Done():
				}
			} else {
				logger.Warn("IPLookup", "IPWhois.io query failed", "ip", ip, "error", err)
			}
		}()
	}

	// Wait in background and close result channel if all APIs finish without success
	go func() {
		wg.Wait()
		close(resultChan)
	}()

	// Read first result
	select {
	case item, ok := <-resultChan:
		if ok && item.res != nil {
			return item.res, nil
		}
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	return nil, errors.New("all geolocation APIs failed to resolve or quota finished")
}

// ResolveDomainWhois queries IP2Whois for domain registration data
func ResolveDomainWhois(ctx context.Context, domain string, apiKey string) (*UnifiedWhoisResult, error) {
	if apiKey == "" {
		return nil, errors.New("IP2Location API key missing for WHOIS lookup")
	}

	client := &http.Client{Timeout: 8 * time.Second}
	url := fmt.Sprintf("https://api.ip2whois.com/v2?key=%s&domain=%s", apiKey, domain)
	
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ip2whois API error (status %d): %s", resp.StatusCode, string(bodyBytes))
	}

	var raw map[string]interface{}
	if err := json.Unmarshal(bodyBytes, &raw); err != nil {
		return nil, err
	}

	// Error check
	if errorObj, ok := raw["error"]; ok {
		if errorMap, isMap := errorObj.(map[string]interface{}); isMap {
			if msg, hasMsg := errorMap["error_message"]; hasMsg {
				return nil, fmt.Errorf("ip2whois error: %v", msg)
			}
		}
	}

	result := &UnifiedWhoisResult{
		DomainName: domain,
		RawJSON:    string(bodyBytes),
		UpdatedAt:  time.Now(),
	}

	if reg, ok := raw["registrar"].(map[string]interface{}); ok {
		if name, ok := reg["name"].(string); ok {
			result.Registrar = name
		}
	}
	if cDate, ok := raw["create_date"].(string); ok {
		result.CreationDate = cDate
	}
	if eDate, ok := raw["expire_date"].(string); ok {
		result.ExpiryDate = eDate
	}
	if nsArr, ok := raw["nameservers"].([]interface{}); ok {
		for _, ns := range nsArr {
			if nsStr, ok := ns.(string); ok {
				result.NameServers = append(result.NameServers, nsStr)
			}
		}
	}

	return result, nil
}

// --- Specific API implementations ---

func fetchIP2Location(ctx context.Context, ip, key string) (*UnifiedIPResult, error) {
	client := &http.Client{Timeout: 6 * time.Second}
	url := fmt.Sprintf("https://api.ip2location.io/?key=%s&ip=%s&format=json", key, ip)
	
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("IP2Location response status %d", resp.StatusCode)
	}

	var data map[string]interface{}
	if err := json.Unmarshal(bodyBytes, &data); err != nil {
		return nil, err
	}

	// check error code (e.g. key expired or quota finished)
	if errObj, ok := data["error"]; ok {
		if errMap, ok := errObj.(map[string]interface{}); ok {
			if code, ok := errMap["error_code"].(float64); ok && code == 10001 {
				return nil, errors.New("quota limit reached or key expired")
			}
			return nil, fmt.Errorf("IP2Location error: %v", errMap["error_message"])
		}
	}

	res := &UnifiedIPResult{
		IP:        ip,
		Provider:  "ip2location.io",
		RawJSON:   string(bodyBytes),
		UpdatedAt: time.Now(),
	}

	if cName, ok := data["country_name"].(string); ok {
		res.Country = cName
	}
	if cCode, ok := data["country_code"].(string); ok {
		res.CountryCode = cCode
	}
	if cityName, ok := data["city_name"].(string); ok {
		res.City = cityName
	}
	if region, ok := data["region_name"].(string); ok && region != "" && res.City != "" {
		res.City = res.City + ", " + region
	}
	if isp, ok := data["isp"].(string); ok {
		res.ISP = isp
	}
	if asn, ok := data["asn"].(string); ok {
		res.ASN = asn
	} else if asnF, ok := data["asn"].(float64); ok {
		res.ASN = fmt.Sprintf("AS%.0f", asnF)
	}
	if lat, ok := data["latitude"].(float64); ok {
		res.Latitude = lat
	}
	if lng, ok := data["longitude"].(float64); ok {
		res.Longitude = lng
	}

	// Parse Proxy Status
	res.ProxyStatus = "Clean"
	if isProxy, ok := data["is_proxy"].(bool); ok && isProxy {
		res.IsProxy = true
		res.ProxyStatus = "Proxy"
		if proxyObj, ok := data["proxy"].(map[string]interface{}); ok {
			if isVPN, ok := proxyObj["is_vpn"].(bool); ok && isVPN {
				res.ProxyStatus = "VPN"
			} else if isTor, ok := proxyObj["is_tor"].(bool); ok && isTor {
				res.ProxyStatus = "Tor"
			} else if isDch, ok := proxyObj["is_data_center"].(bool); ok && isDch {
				res.ProxyStatus = "DCH"
			}
		}
	}

	return res, nil
}

func fetchIpApi(ctx context.Context, ip, key string) (*UnifiedIPResult, error) {
	client := &http.Client{Timeout: 5 * time.Second}
	url := fmt.Sprintf("http://ip-api.com/json/%s?fields=status,message,country,countryCode,regionName,city,lat,lon,isp,as,query", ip)
	if key != "" {
		url = fmt.Sprintf("https://pro.ip-api.com/json/%s?key=%s&fields=status,message,country,countryCode,regionName,city,lat,lon,isp,as,query", ip, key)
	}

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var data map[string]interface{}
	if err := json.Unmarshal(bodyBytes, &data); err != nil {
		return nil, err
	}

	if data["status"] != "success" {
		return nil, fmt.Errorf("ip-api error: %v", data["message"])
	}

	res := &UnifiedIPResult{
		IP:        ip,
		Provider:  "ip-api.com",
		RawJSON:   string(bodyBytes),
		UpdatedAt: time.Now(),
	}

	if cName, ok := data["country"].(string); ok {
		res.Country = cName
	}
	if cCode, ok := data["countryCode"].(string); ok {
		res.CountryCode = cCode
	}
	if city, ok := data["city"].(string); ok {
		res.City = city
	}
	if region, ok := data["regionName"].(string); ok && region != "" && res.City != "" {
		res.City = res.City + ", " + region
	}
	if isp, ok := data["isp"].(string); ok {
		res.ISP = isp
	}
	if asn, ok := data["as"].(string); ok {
		parts := strings.Split(asn, " ")
		if len(parts) > 0 {
			res.ASN = parts[0]
		}
	}
	if lat, ok := data["lat"].(float64); ok {
		res.Latitude = lat
	}
	if lon, ok := data["lon"].(float64); ok {
		res.Longitude = lon
	}

	res.ProxyStatus = "Clean" // ip-api free does not return proxy indicators
	return res, nil
}

func fetchFindIP(ctx context.Context, ip, token string) (*UnifiedIPResult, error) {
	client := &http.Client{Timeout: 5 * time.Second}
	url := fmt.Sprintf("https://api.findip.net/%s/", ip)
	if token != "" {
		url = fmt.Sprintf("https://api.findip.net/%s/?token=%s", ip, token)
	}

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var data map[string]interface{}
	if err := json.Unmarshal(bodyBytes, &data); err != nil {
		return nil, err
	}

	res := &UnifiedIPResult{
		IP:        ip,
		Provider:  "findip.net",
		RawJSON:   string(bodyBytes),
		UpdatedAt: time.Now(),
	}

	if cObj, ok := data["country"].(map[string]interface{}); ok {
		if names, ok := cObj["names"].(map[string]interface{}); ok {
			if en, ok := names["en"].(string); ok {
				res.Country = en
			}
		}
		if code, ok := cObj["iso_code"].(string); ok {
			res.CountryCode = code
		}
	}

	if cObj, ok := data["city"].(map[string]interface{}); ok {
		if names, ok := cObj["names"].(map[string]interface{}); ok {
			if en, ok := names["en"].(string); ok {
				res.City = en
			}
		}
	}

	if traits, ok := data["traits"].(map[string]interface{}); ok {
		if isp, ok := traits["isp"].(string); ok {
			res.ISP = isp
		}
		if asn, ok := traits["autonomous_system_number"].(float64); ok {
			res.ASN = fmt.Sprintf("AS%.0f", asn)
		}
		
		// Proxy flags
		res.ProxyStatus = "Clean"
		isVpn := false
		isTor := false
		isDch := false
		if val, ok := traits["is_vpn"].(bool); ok {
			isVpn = val
		}
		if val, ok := traits["is_tor_exit_node"].(bool); ok {
			isTor = val
		}
		if val, ok := traits["is_hosting_provider"].(bool); ok {
			isDch = val
		}
		if isVpn {
			res.ProxyStatus = "VPN"
			res.IsProxy = true
		} else if isTor {
			res.ProxyStatus = "Tor"
			res.IsProxy = true
		} else if isDch {
			res.ProxyStatus = "DCH"
			res.IsProxy = true
		}
	}

	if loc, ok := data["location"].(map[string]interface{}); ok {
		if lat, ok := loc["latitude"].(float64); ok {
			res.Latitude = lat
		}
		if lng, ok := loc["longitude"].(float64); ok {
			res.Longitude = lng
		}
	}

	return res, nil
}

func fetchIpGeolocation(ctx context.Context, ip, key string) (*UnifiedIPResult, error) {
	client := &http.Client{Timeout: 5 * time.Second}
	url := fmt.Sprintf("https://api.ipgeolocation.io/ipgeo?apiKey=%s&ip=%s", key, ip)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ipgeolocation response status %d", resp.StatusCode)
	}

	var data map[string]interface{}
	if err := json.Unmarshal(bodyBytes, &data); err != nil {
		return nil, err
	}

	res := &UnifiedIPResult{
		IP:        ip,
		Provider:  "ipgeolocation.io",
		RawJSON:   string(bodyBytes),
		UpdatedAt: time.Now(),
	}

	if cName, ok := data["country_name"].(string); ok {
		res.Country = cName
	}
	if cCode, ok := data["country_code2"].(string); ok {
		res.CountryCode = cCode
	}
	if city, ok := data["city"].(string); ok {
		res.City = city
	}
	if region, ok := data["state_prov"].(string); ok && region != "" && res.City != "" {
		res.City = res.City + ", " + region
	}
	if isp, ok := data["isp"].(string); ok {
		res.ISP = isp
	}
	if asn, ok := data["asn"].(string); ok {
		res.ASN = asn
	}
	if latStr, ok := data["latitude"].(string); ok {
		var l float64
		_, _ = fmt.Sscanf(latStr, "%f", &l)
		res.Latitude = l
	}
	if lngStr, ok := data["longitude"].(string); ok {
		var l float64
		_, _ = fmt.Sscanf(lngStr, "%f", &l)
		res.Longitude = l
	}

	res.ProxyStatus = "Clean"
	return res, nil
}

func fetchIpWhois(ctx context.Context, ip, key string) (*UnifiedIPResult, error) {
	client := &http.Client{Timeout: 5 * time.Second}
	url := fmt.Sprintf("https://ipwhois.app/json/%s", ip)
	if key != "" {
		url = fmt.Sprintf("https://ipwhois.pro/%s?key=%s", ip, key)
	}

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var data map[string]interface{}
	if err := json.Unmarshal(bodyBytes, &data); err != nil {
		return nil, err
	}

	success := false
	if s, ok := data["success"].(bool); ok {
		success = s
	} else if sStr, ok := data["success"].(string); ok {
		success = sStr == "true"
	} else {
		// fallback
		success = data["country"] != nil
	}

	if !success {
		return nil, fmt.Errorf("ipwhois error: %v", data["message"])
	}

	res := &UnifiedIPResult{
		IP:        ip,
		Provider:  "ipwhois.io",
		RawJSON:   string(bodyBytes),
		UpdatedAt: time.Now(),
	}

	if cName, ok := data["country"].(string); ok {
		res.Country = cName
	}
	if cCode, ok := data["country_code"].(string); ok {
		res.CountryCode = cCode
	}
	if city, ok := data["city"].(string); ok {
		res.City = city
	}
	if region, ok := data["region"].(string); ok && region != "" && res.City != "" {
		res.City = res.City + ", " + region
	}
	if isp, ok := data["isp"].(string); ok {
		res.ISP = isp
	}
	if asn, ok := data["asn"].(string); ok {
		res.ASN = asn
	}
	if lat, ok := data["latitude"].(float64); ok {
		res.Latitude = lat
	} else if latStr, ok := data["latitude"].(string); ok {
		var l float64
		_, _ = fmt.Sscanf(latStr, "%f", &l)
		res.Latitude = l
	}
	if lng, ok := data["longitude"].(float64); ok {
		res.Longitude = lng
	} else if lngStr, ok := data["longitude"].(string); ok {
		var l float64
		_, _ = fmt.Sscanf(lngStr, "%f", &l)
		res.Longitude = l
	}

	res.ProxyStatus = "Clean"
	// Check security if present (ipwhois pro has it)
	if sec, ok := data["security"].(map[string]interface{}); ok {
		isVpn := false
		isTor := false
		isHosting := false
		if val, ok := sec["vpn"].(bool); ok {
			isVpn = val
		}
		if val, ok := sec["tor"].(bool); ok {
			isTor = val
		}
		if val, ok := sec["hosting"].(bool); ok {
			isHosting = val
		}

		if isVpn {
			res.ProxyStatus = "VPN"
			res.IsProxy = true
		} else if isTor {
			res.ProxyStatus = "Tor"
			res.IsProxy = true
		} else if isHosting {
			res.ProxyStatus = "DCH"
			res.IsProxy = true
		}
	}

	return res, nil
}
