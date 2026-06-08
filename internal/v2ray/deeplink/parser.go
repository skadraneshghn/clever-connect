package deeplink

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"clever-connect/internal/models"
)

// ParseConfigLink parses a configuration link (vless:// or trojan://) and maps it to models.V2RayClientConfig
func ParseConfigLink(link string) (models.V2RayClientConfig, error) {
	var cfg models.V2RayClientConfig

	u, err := url.Parse(link)
	if err != nil {
		return cfg, fmt.Errorf("failed to parse URL: %w", err)
	}

	scheme := strings.ToLower(u.Scheme)
	if scheme != "vless" && scheme != "trojan" {
		return cfg, fmt.Errorf("unsupported connection profile scheme: %s", scheme)
	}

	cfg.Protocol = scheme
	cfg.UUID = u.User.Username()
	cfg.Address = u.Hostname()
	portStr := u.Port()
	if portStr != "" {
		if p, err := strconv.Atoi(portStr); err == nil {
			cfg.Port = p
		}
	}

	// Fragment is the descriptive name
	name := u.Fragment
	if name == "" {
		name = fmt.Sprintf("%s_%s", strings.ToUpper(scheme), cfg.Address)
	} else {
		if decoded, err := url.PathUnescape(name); err == nil {
			name = decoded
		}
	}
	cfg.Name = name

	// Query string parameters
	params := u.Query()
	
	// transport mechanisms
	network := params.Get("type")
	if network == "" {
		network = "tcp"
	}
	cfg.Network = network

	// transport security settings, SNI targets, WebSocket host paths
	tlsMap := make(map[string]interface{})
	security := params.Get("security")
	if security == "" {
		if scheme == "trojan" {
			security = "tls"
		} else {
			security = "none"
		}
	}
	tlsMap["security"] = security
	
	sni := params.Get("sni")
	if sni == "" {
		sni = params.Get("peer") // trojan standard parameter peer is also SNI
	}
	tlsMap["sni"] = sni

	path := params.Get("path")
	tlsMap["path"] = path

	if security == "reality" {
		tlsMap["publicKey"] = params.Get("pbk")
		tlsMap["shortId"] = params.Get("sid")
	}

	tlsBytes, err := json.Marshal(tlsMap)
	if err == nil {
		cfg.TLSSettings = string(tlsBytes)
	}

	return cfg, nil
}
