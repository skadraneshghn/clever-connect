package selector

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"clever-connect/internal/logger"
	"clever-connect/internal/v2ray/compiler"
	"clever-connect/internal/v2ray/core"
)

// compileAndStartCore generates a multi-outbound xray config with balancer + observatory
// and (re)starts the core process. This is the key integration point with the existing
// compiler infrastructure.
func (e *Engine) compileAndStartCore() error {
	e.mu.RLock()
	arteries := make([]*ArteryEntry, len(e.arteries))
	copy(arteries, e.arteries)
	cfg := e.config
	e.mu.RUnlock()

	if len(arteries) == 0 {
		return fmt.Errorf("no active arteries to compile")
	}

	// Determine evasion settings from first artery
	evasionEnabled := true
	tcpDecoySni := ""

	// Build multi-outbound xray config using the existing compiler infrastructure
	coreName := core.GetSelectedCoreName()

	// Build the xray config struct manually for multi-outbound balancer setup
	xrayConfig := compiler.XrayConfig{
		Log: &compiler.LogConfig{
			LogLevel: "warning",
		},
		Api: &compiler.ApiConfig{
			Tag:      "api",
			Services: []string{"StatsService", "LoggerService", "HandlerService"},
		},
		Stats:  &compiler.StatsConfig{},
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

	// User-facing inbounds: SOCKS5 and HTTP proxy
	socksPort := cfg.SocksPort
	if socksPort <= 0 {
		socksPort = 10646
	}
	httpPort := cfg.HTTPPort
	if httpPort <= 0 {
		httpPort = 10545
	}

	xrayConfig.Inbounds = []compiler.InboundConfig{
		{
			Listen:   "127.0.0.1",
			Port:     socksPort,
			Protocol: "socks",
			Settings: map[string]interface{}{
				"auth": "noauth",
				"udp":  true,
			},
			Tag: "socks-in",
			Sniffing: &compiler.SniffingConfig{
				Enabled:      true,
				DestOverride: []string{"http", "tls"},
			},
		},
		{
			Listen:   "127.0.0.1",
			Port:     httpPort,
			Protocol: "http",
			Settings: map[string]interface{}{
				"allowRedirect": true,
			},
			Tag: "http-in",
		},
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

	// Build one outbound per artery using the existing CompileOutbound function
	var balancerTags []string
	for _, a := range arteries {
		outbound := compiler.CompileOutbound(a.Config, evasionEnabled, a.Tag)
		xrayConfig.Outbounds = append(xrayConfig.Outbounds, outbound)
		balancerTags = append(balancerTags, a.Tag)
	}

	// Add direct and block outbounds
	xrayConfig.Outbounds = append(xrayConfig.Outbounds,
		compiler.OutboundConfig{Protocol: "freedom", Tag: "direct"},
		compiler.OutboundConfig{Protocol: "blackhole", Tag: "block"},
	)

	// Balancer + Observatory configuration
	strategy := "leastPing"
	xrayConfig.Routing = &compiler.RoutingConfig{
		DomainStrategy: "IPOnDemand",
		Balancers: []compiler.BalancerConfig{
			{
				Tag:      "bonding-balancer",
				Selector: balancerTags,
				Strategy: strategy,
			},
		},
		Rules: []compiler.RoutingRule{
			{
				Type:        "field",
				InboundTag:  []string{"api"},
				OutboundTag: "api",
			},
			{
				Type:        "field",
				Domain:      []string{"geosite:private", "geosite:ir", "regexp:.*\\.ir$"},
				OutboundTag: "direct",
			},
			{
				Type:        "field",
				IP:          []string{"geoip:private", "geoip:ir"},
				OutboundTag: "direct",
			},
			{
				Type:        "field",
				Port:        "53",
				OutboundTag: "direct",
			},
			{
				Type:        "field",
				Network:     "tcp,udp",
				BalancerTag: "bonding-balancer",
			},
		},
	}

	// Observatory for automatic health probing
	xrayConfig.Observatory = &compiler.ObservatoryConfig{
		SubjectSelector: balancerTags,
		ProbeURL:        "http://www.gstatic.com/generate_204",
		ProbeInterval:   "5s",
	}

	// DNS configuration
	xrayConfig.DNS = &compiler.DnsConfig{
		Servers: []interface{}{
			"https://1.1.1.1/dns-query",
			compiler.DnsServerConfig{
				Address: "8.8.8.8",
				Domains: []string{"geosite:ir", "regexp:.*\\.ir$"},
			},
			"localhost",
		},
	}

	// Serialize config
	configBytes, err := json.MarshalIndent(xrayConfig, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal bonding config: %w", err)
	}

	// Clean for v2ray if needed
	if coreName == "v2ray" {
		configBytes, err = compiler.CleanXrayConfigForV2Ray(configBytes)
		if err != nil {
			return fmt.Errorf("failed to clean config for v2ray: %w", err)
		}
	}

	// Validate
	if err := compiler.ValidateXrayConfig(configBytes); err != nil {
		return fmt.Errorf("config validation failed: %w", err)
	}

	// Write config to temp location
	tempDir := filepath.Join(os.TempDir(), "clever-connect-data")
	if err := os.MkdirAll(tempDir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	configPath := filepath.Join(tempDir, "bonding_selector.json")
	if err := os.WriteFile(configPath, configBytes, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	logger.Info("Bonding", "Compiled selector config",
		"arteries", len(arteries),
		"strategy", strategy,
		"socks_port", socksPort,
		"http_port", httpPort,
		"config_path", configPath,
		"core", coreName,
		"evasion", evasionEnabled,
		"tcp_decoy_sni", tcpDecoySni,
	)

	// Start or restart core with the new config
	return core.StartCore(configPath)
}
