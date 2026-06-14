package client

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"clever-connect/internal/db/pebble"
	"clever-connect/internal/logger"
	"clever-connect/internal/models"
	"clever-connect/internal/v2ray/compiler"
	"clever-connect/internal/v2ray/core"
)

// getBaseTemplateConfig loads the first valid config from PebbleDB that has
// Protocol/UUID/Network/TLSSettings set. All discovered endpoints share the same
// protocol template but differ only in Address and Port.
func getBaseTemplateConfig() (*models.V2RayClientConfig, error) {
	if pebble.DB == nil {
		return nil, fmt.Errorf("pebble DB not initialized")
	}
	configs, total := pebble.ListClientConfigs(pebble.ConfigFilter{}, 0, 0)
	if total == 0 {
		return nil, fmt.Errorf("no client configurations found in PebbleDB")
	}
	for _, cfg := range configs {
		if cfg.Protocol != "" && cfg.UUID != "" {
			return &cfg, nil
		}
	}
	if len(configs) > 0 {
		return &configs[0], nil
	}
	return nil, fmt.Errorf("no base template configuration found")
}

func mergeArteryWithBase(artery models.V2RayClientConfig, base *models.V2RayClientConfig) models.V2RayClientConfig {
	merged := artery
	if merged.Protocol == "" {
		merged.Protocol = base.Protocol
	}
	if merged.UUID == "" {
		merged.UUID = base.UUID
	}
	if merged.Network == "" {
		merged.Network = base.Network
	}
	if merged.TLSSettings == "" {
		merged.TLSSettings = base.TLSSettings
	}
	if !merged.MuxEnabled && base.MuxEnabled {
		merged.MuxEnabled = base.MuxEnabled
	}
	return merged
}

// CompileBondingClientConfig generates a multi-inbound xray config with one
// SOCKS5 inbound per artery, each strictly routed to its own outbound proxy node.
//
// Flow (Mode B):
//
//	Go ArteryConn (SOCKS5 client)
//	     │  SOCKS5 CONNECT ondata.ir:80
//	     ▼
//	127.0.0.1:21001  (xray SOCKS5 inbound "artery-0-in")
//	     │  routed via routing rules → outbound "artery-0"
//	     ▼
//	VLESS/Reality/... CDN Edge Node
//	     │
//	     ▼
//	Clever Cloud combiner WebSocket endpoint
func CompileBondingClientConfig(nodes []models.V2RayClientConfig, combinerAddr string, basePort int, socksPort int, httpPort int) (string, error) {
	if len(nodes) == 0 {
		return "", fmt.Errorf("no nodes provided for bonding client config")
	}

	if socksPort <= 0 {
		socksPort = 10646
	}
	if httpPort <= 0 {
		httpPort = 10545
	}
	if basePort <= 0 {
		basePort = 21001
	}

	coreName := core.GetSelectedCoreName()

	baseTemplate, err := getBaseTemplateConfig()
	if err != nil {
		return "", fmt.Errorf("failed to load base template config: %w", err)
	}

	config := compiler.XrayConfig{
		Log: &compiler.LogConfig{
			LogLevel: "warning",
		},
		Api: &compiler.ApiConfig{
			Tag:      "api",
			Services: []string{"StatsService", "LoggerService", "HandlerService"},
		},
		Stats: &compiler.StatsConfig{},
		Policy: &compiler.PolicyConfig{
			Levels: map[string]compiler.PolicyLevelConfig{
				"0": {
					StatsUserUplink:   true,
					StatsUserDownlink: true,
				},
			},
			System: compiler.PolicyUserConfig{
				StatsInboundUplink:    true,
				StatsInboundDownlink:  true,
				StatsOutboundUplink:   true,
				StatsOutboundDownlink: true,
			},
		},
	}

	// xray API inbound for gRPC stats management
	config.Inbounds = []compiler.InboundConfig{
		{
			Listen:   "127.0.0.1",
			Port:     10085,
			Protocol: "dokodemo-door",
			Settings: map[string]interface{}{
				"address": "127.0.0.1",
			},
			Tag: "api",
		},
	}

	// One SOCKS5 inbound per artery.
	// The Go ArteryConn dials: proxy.SOCKS5("tcp", "127.0.0.1:2100x").Dial(combinerHost)
	// xray receives a SOCKS5 CONNECT request for the real combiner host and
	// routes it through the matching outbound (the scanned proxy node).
	var routingRules []compiler.RoutingRule

	// API routing rule first
	routingRules = append(routingRules, compiler.RoutingRule{
		Type:        "field",
		InboundTag:  []string{"api"},
		OutboundTag: "api",
	})

	for i, node := range nodes {
		tag := fmt.Sprintf("artery-%d", i)
		localPort := basePort + i

		mergedNode := mergeArteryWithBase(node, baseTemplate)

		// SOCKS5 inbound: Go client performs standard SOCKS5 CONNECT to the
		// real combiner host. xray forwards through the artery outbound.
		config.Inbounds = append(config.Inbounds, compiler.InboundConfig{
			Listen:   "127.0.0.1",
			Port:     localPort,
			Protocol: "socks",
			Settings: map[string]interface{}{
				"auth": "noauth",
				"udp":  false,
				"ip":   "127.0.0.1",
			},
			Tag: fmt.Sprintf("artery-%d-in", i),
		})

		// One outbound per artery using existing compiler with merged config
		outbound := compiler.CompileOutbound(mergedNode, true, tag)
		config.Outbounds = append(config.Outbounds, outbound)

		// Strict inbound → outbound routing (no balancer)
		routingRules = append(routingRules, compiler.RoutingRule{
			Type:        "field",
			InboundTag:  []string{fmt.Sprintf("artery-%d-in", i)},
			OutboundTag: tag,
		})
	}

	// Add direct and block outbounds
	config.Outbounds = append(config.Outbounds,
		compiler.OutboundConfig{Protocol: "freedom", Tag: "direct"},
		compiler.OutboundConfig{Protocol: "blackhole", Tag: "block"},
	)

	// Domestic direct routing
	routingRules = append(routingRules,
		compiler.RoutingRule{
			Type:        "field",
			Domain:      []string{"geosite:private", "geosite:ir", "regexp:.*\\.ir$"},
			OutboundTag: "direct",
		},
		compiler.RoutingRule{
			Type:        "field",
			IP:          []string{"geoip:private", "geoip:ir"},
			OutboundTag: "direct",
		},
		compiler.RoutingRule{
			Type:        "field",
			Port:        "53",
			OutboundTag: "direct",
		},
	)

	config.Routing = &compiler.RoutingConfig{
		DomainStrategy: "IPOnDemand",
		Rules:          routingRules,
	}

	// DNS
	config.DNS = &compiler.DnsConfig{
		Servers: []interface{}{
			"https://1.1.1.1/dns-query",
			compiler.DnsServerConfig{
				Address: "8.8.8.8",
				Domains: []string{"geosite:ir", "regexp:.*\\.ir$"},
			},
			"localhost",
		},
	}

	// Serialize
	configBytes, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal bonding client config: %w", err)
	}

	// Clean for v2ray if needed
	if coreName == "v2ray" {
		configBytes, err = compiler.CleanXrayConfigForV2Ray(configBytes)
		if err != nil {
			return "", fmt.Errorf("failed to clean config for v2ray: %w", err)
		}
	}

	// Validate
	if err := compiler.ValidateXrayConfig(configBytes); err != nil {
		return "", fmt.Errorf("bonding client config validation failed: %w", err)
	}

	// Write to temp file
	tempDir := filepath.Join(os.TempDir(), "clever-connect-data")
	if err := os.MkdirAll(tempDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create config directory: %w", err)
	}

	configPath := filepath.Join(tempDir, "bonding_client.json")
	if err := os.WriteFile(configPath, configBytes, 0644); err != nil {
		return "", fmt.Errorf("failed to write config file: %w", err)
	}

	logger.Info("Bonding", "Compiled bonding client config",
		"arteries", len(nodes),
		"combiner", combinerAddr,
		"base_port", basePort,
		"config_path", configPath,
		"core", coreName,
	)

	return configPath, nil
}
