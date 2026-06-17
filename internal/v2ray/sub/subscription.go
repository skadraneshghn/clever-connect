package sub

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"clever-connect/internal/db"
	"clever-connect/internal/db/pebble"
	"clever-connect/internal/logger"
	"clever-connect/internal/models"
)

// GenerateSubscription returns base64-encoded proxy link list for a user by their token
func GenerateSubscription(subToken string, requestHost string) (string, error) {
	var user models.V2RayUser
	if err := db.DB.Where("sub_token = ? AND enabled = ?", subToken, true).First(&user).Error; err != nil {
		return "", fmt.Errorf("user not found or disabled")
	}

	var inbounds []models.V2RayInbound
	if err := db.DB.Where("enabled = ?", true).Find(&inbounds).Error; err != nil {
		return "", err
	}

	var sb strings.Builder
	for _, in := range inbounds {
		// Only link users to their assigned inbounds or support global linking
		if in.ID != user.InboundID && user.InboundID != 0 {
			continue
		}

		linkHost := requestHost
		if strings.Contains(linkHost, ":") {
			linkHost = strings.Split(linkHost, ":")[0]
		}
		if in.SNI != "" {
			linkHost = in.SNI
		}

		switch in.Protocol {
		case "vless":
			// vless://uuid@host:port?type=ws&security=reality&sni=sni&path=path&pbk=publickey#name
			link := fmt.Sprintf("vless://%s@%s:%d?", user.UUID, requestHost, in.Port)
			params := url.Values{}
			params.Add("type", in.Network)
			if in.TLSMode == "reality" {
				params.Add("security", "reality")
				params.Add("sni", in.SNI)
				params.Add("pbk", in.RealityPublicKey)
				if in.RealityShortIDs != "" {
					params.Add("sid", strings.Split(in.RealityShortIDs, ",")[0])
				}
			} else if in.TLSMode == "tls" {
				params.Add("security", "tls")
				params.Add("sni", in.SNI)
			} else {
				params.Add("security", "none")
			}

			if in.Network == "ws" || in.Network == "grpc" {
				params.Add("path", in.Path)
			}
			link += params.Encode()
			link += "#" + url.PathEscape(in.Tag)
			sb.WriteString(link + "\n")

		case "vmess":
			// vmess://base64_json
			configMap := map[string]interface{}{
				"v":    "2",
				"ps":   in.Tag,
				"add":  requestHost,
				"port": in.Port,
				"id":   user.UUID,
				"aid":  0,
				"net":  in.Network,
				"type": "none",
				"host": in.SNI,
				"path": in.Path,
				"tls":  in.TLSMode,
			}
			jsonBytes, _ := json.Marshal(configMap)
			b64 := base64.StdEncoding.EncodeToString(jsonBytes)
			sb.WriteString("vmess://" + b64 + "\n")

		case "trojan":
			// trojan://password@host:port?peer=sni#name
			link := fmt.Sprintf("trojan://%s@%s:%d?", user.UUID, requestHost, in.Port)
			params := url.Values{}
			if in.TLSMode == "tls" || in.TLSMode == "reality" {
				params.Add("sni", in.SNI)
			}
			link += params.Encode()
			link += "#" + url.PathEscape(in.Tag)
			sb.WriteString(link + "\n")
		}
	}

	return base64.StdEncoding.EncodeToString([]byte(sb.String())), nil
}

// FetchAndImportSubscription pulls base64 subscription URLs, parses them, and saves to DB
func FetchAndImportSubscription(subURL string) ([]models.V2RayClientConfig, error) {
	resp, err := http.Get(subURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to fetch subscription, status: %d", resp.StatusCode)
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	rawContent := string(bodyBytes)
	decodedBytes, err := RobustDecodeBase64(rawContent)
	if err != nil {
		// Attempt reading raw unencoded lines just in case
		decodedBytes = bodyBytes
	}

	var configs []models.V2RayClientConfig
	lines := strings.Split(string(decodedBytes), "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		cfg, err := ParseProxyLink(line)
		if err != nil {
			continue
		}
		configs = append(configs, cfg)
	}

	return configs, nil
}

// ParseProxyLink converts vless://, vmess://, or trojan:// URLs into ClientConfig structures
func ParseProxyLink(link string) (models.V2RayClientConfig, error) {
	var cfg models.V2RayClientConfig

	if strings.HasPrefix(link, "vless://") {
		u, err := url.Parse(link)
		if err != nil {
			return cfg, err
		}

		uuid := u.User.Username()
		host := u.Hostname()
		portStr := u.Port()
		port, _ := strconv.Atoi(portStr)

		params := u.Query()
		network := params.Get("type")
		if network == "" {
			network = "tcp"
		}

		fp := params.Get("fp")
		if fp == "" {
			fp = params.Get("fingerprint")
		}
		allowInsecureStr := params.Get("allowInsecure")
		if allowInsecureStr == "" {
			allowInsecureStr = params.Get("insecure")
		}
		allowInsecure := false
		if allowInsecureStr == "1" || strings.ToLower(allowInsecureStr) == "true" {
			allowInsecure = true
		}

		tlsSettings := map[string]interface{}{
			"security":      params.Get("security"),
			"sni":           params.Get("sni"),
			"publicKey":     params.Get("pbk"),
			"shortId":       params.Get("sid"),
			"path":          params.Get("path"),
			"fingerprint":   fp,
			"allowInsecure": allowInsecure,
		}
		tlsSettingsBytes, _ := json.Marshal(tlsSettings)

		name := u.Fragment
		if name == "" {
			name = "VLESS_" + host
		} else {
			if decoded, err := url.PathUnescape(name); err == nil {
				name = decoded
			}
		}

		cfg = models.V2RayClientConfig{
			Name:        name,
			Protocol:    "vless",
			Address:     host,
			Port:        port,
			UUID:        uuid,
			Network:     network,
			TLSSettings: string(tlsSettingsBytes),
		}
		return cfg, nil

	} else if strings.HasPrefix(link, "trojan://") {
		u, err := url.Parse(link)
		if err != nil {
			return cfg, err
		}

		password := u.User.Username()
		host := u.Hostname()
		portStr := u.Port()
		port, _ := strconv.Atoi(portStr)

		params := u.Query()
		fp := params.Get("fp")
		if fp == "" {
			fp = params.Get("fingerprint")
		}
		allowInsecureStr := params.Get("allowInsecure")
		if allowInsecureStr == "" {
			allowInsecureStr = params.Get("insecure")
		}
		allowInsecure := false
		if allowInsecureStr == "1" || strings.ToLower(allowInsecureStr) == "true" {
			allowInsecure = true
		}

		tlsSettings := map[string]interface{}{
			"security":      "tls",
			"sni":           params.Get("sni"),
			"fingerprint":   fp,
			"allowInsecure": allowInsecure,
		}
		tlsSettingsBytes, _ := json.Marshal(tlsSettings)

		name := u.Fragment
		if name == "" {
			name = "Trojan_" + host
		} else {
			if decoded, err := url.PathUnescape(name); err == nil {
				name = decoded
			}
		}

		cfg = models.V2RayClientConfig{
			Name:        name,
			Protocol:    "trojan",
			Address:     host,
			Port:        port,
			UUID:        password,
			Network:     "tcp",
			TLSSettings: string(tlsSettingsBytes),
		}
		return cfg, nil

	} else if strings.HasPrefix(link, "vmess://") {
		rawB64 := strings.TrimPrefix(link, "vmess://")
		jsonBytes, err := base64.StdEncoding.DecodeString(rawB64)
		if err != nil {
			return cfg, err
		}

		var vmessMap map[string]interface{}
		if err := json.Unmarshal(jsonBytes, &vmessMap); err != nil {
			return cfg, err
		}

		host, _ := vmessMap["add"].(string)
		portVal, _ := vmessMap["port"]
		var port int
		switch p := portVal.(type) {
		case float64:
			port = int(p)
		case string:
			port, _ = strconv.Atoi(p)
		}

		uuid, _ := vmessMap["id"].(string)
		network, _ := vmessMap["net"].(string)
		if network == "" {
			network = "tcp"
		}

		tlsMode, _ := vmessMap["tls"].(string)
		if tlsMode == "" {
			tlsMode = "none"
		}

		fp, _ := vmessMap["fp"].(string)
		if fp == "" {
			fp, _ = vmessMap["fingerprint"].(string)
		}
		allowInsecure := false
		if insVal, ok := vmessMap["insecure"]; ok {
			switch v := insVal.(type) {
			case bool:
				allowInsecure = v
			case float64:
				allowInsecure = (v == 1)
			case string:
				allowInsecure = (v == "1" || strings.ToLower(v) == "true")
			}
		}

		tlsSettings := map[string]interface{}{
			"security":      tlsMode,
			"sni":           vmessMap["host"],
			"path":          vmessMap["path"],
			"fingerprint":   fp,
			"allowInsecure": allowInsecure,
		}
		tlsSettingsBytes, _ := json.Marshal(tlsSettings)

		name, _ := vmessMap["ps"].(string)
		if name == "" {
			name = "VMess_" + host
		}

		cfg = models.V2RayClientConfig{
			Name:        name,
			Protocol:    "vmess",
			Address:     host,
			Port:        port,
			UUID:        uuid,
			Network:     network,
			TLSSettings: string(tlsSettingsBytes),
		}
		return cfg, nil
	} else if strings.HasPrefix(link, "ss://") {
		body := strings.TrimPrefix(link, "ss://")
		name := ""
		if idx := strings.IndexByte(body, '#'); idx >= 0 {
			name = body[idx+1:]
			body = body[:idx]
		}
		if idx := strings.IndexByte(body, '?'); idx >= 0 {
			body = body[:idx]
		}
		body = strings.TrimSpace(body)

		var method, password, host string
		var port int

		if at := strings.LastIndexByte(body, '@'); at >= 0 {
			userinfo := body[:at]
			hp := body[at+1:]
			
			mp := userinfo
			if dec, err := decodeB64(userinfo); err == nil && strings.Contains(dec, ":") {
				mp = dec
			}
			parts := strings.SplitN(mp, ":", 2)
			if len(parts) == 2 {
				method = parts[0]
				password = parts[1]
			}
			
			h, p, err := net.SplitHostPort(hp)
			if err == nil {
				host = h
				port, _ = strconv.Atoi(p)
			} else {
				idx := strings.LastIndexByte(hp, ':')
				if idx >= 0 {
					host = hp[:idx]
					port, _ = strconv.Atoi(hp[idx+1:])
				} else {
					host = hp
				}
			}
		} else {
			dec, err := decodeB64(body)
			if err == nil {
				at := strings.LastIndexByte(dec, '@')
				if at >= 0 {
					mp := dec[:at]
					hp := dec[at+1:]
					parts := strings.SplitN(mp, ":", 2)
					if len(parts) == 2 {
						method = parts[0]
						password = parts[1]
					}
					h, p, err := net.SplitHostPort(hp)
					if err == nil {
						host = h
						port, _ = strconv.Atoi(p)
					}
				}
			}
		}

		if host == "" {
			return cfg, fmt.Errorf("invalid shadowsocks link")
		}

		if name == "" {
			name = "SS_" + host
		} else {
			if decoded, err := url.PathUnescape(name); err == nil {
				name = decoded
			}
		}

		tlsSettings := map[string]interface{}{
			"method": method,
		}
		tlsSettingsBytes, _ := json.Marshal(tlsSettings)

		cfg = models.V2RayClientConfig{
			Name:        name,
			Protocol:    "shadowsocks",
			Address:     host,
			Port:        port,
			UUID:        password,
			Network:     "tcp",
			TLSSettings: string(tlsSettingsBytes),
		}
		return cfg, nil
	}

	return cfg, fmt.Errorf("unsupported proxy link format")
}

// RobustDecodeBase64 strips whitespace/newlines and decodes standard, url-safe, and unpadded base64 strings
func RobustDecodeBase64(s string) ([]byte, error) {
	s = strings.ReplaceAll(s, "\n", "")
	s = strings.ReplaceAll(s, "\r", "")
	s = strings.ReplaceAll(s, " ", "")
	s = strings.ReplaceAll(s, "\t", "")

	for _, enc := range []*base64.Encoding{
		base64.StdEncoding,
		base64.RawStdEncoding,
		base64.URLEncoding,
		base64.RawURLEncoding,
	} {
		if dec, err := enc.DecodeString(s); err == nil {
			return dec, nil
		}
	}

	// Fix missing padding character(s)
	if pad := len(s) % 4; pad != 0 {
		s += strings.Repeat("=", 4-pad)
		if dec, err := base64.StdEncoding.DecodeString(s); err == nil {
			return dec, nil
		}
	}

	return nil, fmt.Errorf("failed to decode base64")
}

func decodeB64(s string) (string, error) {
	dec, err := RobustDecodeBase64(s)
	if err != nil {
		return "", err
	}
	return string(dec), nil
}

// StartSubscriptionUpdater runs a background worker to periodically update subscriptions
func StartSubscriptionUpdater(ctx context.Context) {
	// Check every hour
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	// Run initially on startup
	UpdateAllSubscriptions()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			UpdateAllSubscriptions()
		}
	}
}

// UpdateSubscriptionByID fetches, parses, diffs and updates a single client-side subscription in place
func UpdateSubscriptionByID(subID uint) (int, error) {
	if db.DB == nil {
		return 0, fmt.Errorf("database not initialized")
	}

	var s models.V2RayClientSubscription
	if err := db.DB.First(&s, subID).Error; err != nil {
		return 0, err
	}

	configs, err := FetchAndImportSubscription(s.URL)
	if err != nil {
		return 0, err
	}

	// Fetch current configs for this subscription from PebbleDB
	currentConfigs, _ := pebble.ListClientConfigs(pebble.ConfigFilter{SubscriptionID: &s.ID}, 0, 0)

	newLookup := make(map[string]models.V2RayClientConfig)
	for _, cfg := range configs {
		key := fmt.Sprintf("%s-%s-%d", cfg.UUID, cfg.Address, cfg.Port)
		newLookup[key] = cfg
	}

	currentLookup := make(map[string]models.V2RayClientConfig)
	for _, cfg := range currentConfigs {
		key := fmt.Sprintf("%s-%s-%d", cfg.UUID, cfg.Address, cfg.Port)
		currentLookup[key] = cfg
	}

	var deletedActive bool

	// Delete config records that are no longer in the subscription
	for key, cfg := range currentLookup {
		if _, ok := newLookup[key]; !ok {
			if cfg.IsActive {
				deletedActive = true
			}
			_ = pebble.DeleteClientConfig(cfg.ID)
		}
	}

	// Find or create auto category for the subscription
	var cat models.NodeCategory
	catID := uint(0)
	if err := db.DB.Where("name = ? AND type = ?", s.Name, "auto").First(&cat).Error; err != nil {
		cat = models.NodeCategory{
			Name:     s.Name,
			Type:     "auto",
			ColorHex: "#3b82f6",
		}
		if err := db.DB.Create(&cat).Error; err == nil {
			catID = cat.ID
		}
	} else {
		catID = cat.ID
	}

	// Detect country code helper
	detectCountryCode := func(name string) string {
		name = strings.ToUpper(name)
		countries := map[string]string{
			"US": "US", "UNITED STATES": "US", "🇺🇸": "US",
			"HK": "HK", "HONG KONG": "HK", "🇭🇰": "HK",
			"DE": "DE", "GERMANY": "DE", "🇩🇪": "DE",
			"GB": "GB", "UNITED KINGDOM": "GB", "UK": "GB", "🇬🇧": "GB",
			"FR": "FR", "FRANCE": "FR", "🇫🇷": "FR",
			"NL": "NL", "NETHERLANDS": "NL", "🇳🇱": "NL",
			"SG": "SG", "SINGAPORE": "SG", "🇸🇬": "SG",
			"JP": "JP", "JAPAN": "JP", "🇯🇵": "JP",
			"KR": "KR", "KOREA": "KR", "🇰🇷": "KR",
			"TR": "TR", "TURKEY": "TR", "🇹🇷": "TR",
			"IR": "IR", "IRAN": "IR", "🇮🇷": "IR",
			"FI": "FI", "FINLAND": "FI", "🇫🇮": "FI",
			"SE": "SE", "SWEDEN": "SE", "🇸🇪": "SE",
			"CA": "CA", "CANADA": "CA", "🇨🇦": "CA",
		}
		for kw, cc := range countries {
			if strings.Contains(name, kw) {
				return cc
			}
		}
		return ""
	}

	// Insert new ones, or update existing fields
	var toSave []models.V2RayClientConfig
	for key, cfg := range newLookup {
		cfg.SubscriptionID = s.ID
		cfg.SourceVector = "subscription"
		cfg.CountryCode = detectCountryCode(cfg.Name)

		if existing, ok := currentLookup[key]; ok {
			existing.Name = cfg.Name
			existing.TLSSettings = cfg.TLSSettings
			existing.SubscriptionID = s.ID
			existing.SourceVector = "subscription"
			existing.CountryCode = cfg.CountryCode
			// Keep manual category assignments intact if custom type
			if existing.CategoryID > 0 {
				var nodeCat models.NodeCategory
				if err := db.DB.First(&nodeCat, existing.CategoryID).Error; err == nil && nodeCat.Type == "custom" {
					// Keep
				} else {
					existing.CategoryID = catID
				}
			} else {
				existing.CategoryID = catID
			}
			toSave = append(toSave, existing)
		} else {
			cfg.CategoryID = catID
			toSave = append(toSave, cfg)
		}
	}

	if len(toSave) > 0 {
		_ = pebble.SaveClientConfigsBulk(toSave)
	}

	s.LastUpdatedAt = time.Now()
	db.DB.Save(&s)

	// Fallback to first available active server if current active server was deleted
	if deletedActive {
		allCfgs, _ := pebble.ListClientConfigs(pebble.ConfigFilter{}, 0, 0)
		if len(allCfgs) > 0 {
			first := allCfgs[0]
			first.IsActive = true
			_ = pebble.SaveClientConfig(&first)
			logger.Info("SubUpdater", "Active client server deleted from subscription. Auto-selected alternative active server", "name", first.Name)
		}
	}

	return len(configs), nil
}

// UpdateAllSubscriptions fetches, parses, diffs and updates all client-side subscriptions
func UpdateAllSubscriptions() {
	if db.DB == nil {
		return
	}

	var subscriptions []models.V2RayClientSubscription
	if err := db.DB.Find(&subscriptions).Error; err != nil {
		return
	}

	for _, s := range subscriptions {
		// Enforce update interval (default 12 hours)
		interval := s.UpdateInterval
		if interval <= 0 {
			interval = 12
		}

		if time.Since(s.LastUpdatedAt) < time.Duration(interval)*time.Hour {
			continue
		}

		logger.Info("SubUpdater", "Periodically updating V2Ray subscription", "name", s.Name, "url", s.URL)
		_, err := UpdateSubscriptionByID(s.ID)
		if err != nil {
			logger.Error("SubUpdater", "Failed to auto-update subscription", "url", s.URL, "error", err)
		}
	}
}

