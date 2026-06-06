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
	decodedBytes, err := base64.StdEncoding.DecodeString(strings.TrimSpace(rawContent))
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

		tlsSettings := map[string]interface{}{
			"security":  params.Get("security"),
			"sni":       params.Get("sni"),
			"publicKey": params.Get("pbk"),
			"shortId":   params.Get("sid"),
			"path":      params.Get("path"),
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
		tlsSettings := map[string]interface{}{
			"security": "tls",
			"sni":      params.Get("sni"),
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

		tlsSettings := map[string]interface{}{
			"security": tlsMode,
			"sni":      vmessMap["host"],
			"path":     vmessMap["path"],
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

func decodeB64(s string) (string, error) {
	s = strings.TrimSpace(s)
	for _, enc := range []*base64.Encoding{
		base64.StdEncoding, base64.RawStdEncoding,
		base64.URLEncoding, base64.RawURLEncoding,
	} {
		if dec, err := enc.DecodeString(s); err == nil {
			return string(dec), nil
		}
	}
	if pad := len(s) % 4; pad != 0 {
		s += strings.Repeat("=", 4-pad)
		if dec, err := base64.StdEncoding.DecodeString(s); err == nil {
			return string(dec), nil
		}
	}
	return "", fmt.Errorf("failed to decode base64")
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
		configs, err := FetchAndImportSubscription(s.URL)
		if err != nil {
			logger.Error("SubUpdater", "Failed to auto-update subscription", "url", s.URL, "error", err)
			continue
		}

		tx := db.DB.Begin()

		// Fetch current configs for this subscription
		var currentConfigs []models.V2RayClientConfig
		tx.Where("subscription_id = ?", s.ID).Find(&currentConfigs)

		newLookup := make(map[string]models.V2RayClientConfig)
		for _, cfg := range configs {
			key := fmt.Sprintf("%s:%s:%d", cfg.UUID, cfg.Address, cfg.Port)
			newLookup[key] = cfg
		}

		currentLookup := make(map[string]models.V2RayClientConfig)
		for _, cfg := range currentConfigs {
			key := fmt.Sprintf("%s:%s:%d", cfg.UUID, cfg.Address, cfg.Port)
			currentLookup[key] = cfg
		}

		var deletedActive bool

		// Delete config records that are no longer in the subscription
		for key, cfg := range currentLookup {
			if _, ok := newLookup[key]; !ok {
				if cfg.IsActive {
					deletedActive = true
				}
				tx.Delete(&cfg)
			}
		}

		// Insert new ones, or update existing fields
		for key, cfg := range newLookup {
			cfg.SubscriptionID = s.ID
			if existing, ok := currentLookup[key]; ok {
				existing.Name = cfg.Name
				existing.TLSSettings = cfg.TLSSettings
				tx.Save(&existing)
			} else {
				tx.Create(&cfg)
			}
		}

		s.LastUpdatedAt = time.Now()
		tx.Save(&s)
		tx.Commit()

		// Fallback to first available active server if current active server was deleted
		if deletedActive {
			var first models.V2RayClientConfig
			if err := db.DB.Order("priority asc, id asc").First(&first).Error; err == nil {
				first.IsActive = true
				db.DB.Save(&first)
				logger.Info("SubUpdater", "Active client server deleted from subscription. Auto-selected alternative active server", "name", first.Name)
			}
		}
	}
}

