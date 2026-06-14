package selector

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
// Protocol/UUID/Network/TLSSettings set. This is the same template the scanner
// uses; all discovered endpoints share the same protocol template but differ
// only in Address and Port.
func getBaseTemplateConfig() (*models.V2RayClientConfig, error) {
	if pebble.DB == nil {
		return nil, fmt.Errorf("pebble DB not initialized")
	}
	configs, total := pebble.ListClientConfigs(pebble.ConfigFilter{}, 0, 0)
	if total == 0 {
		return nil, fmt.Errorf("no client configurations found in PebbleDB")
	}
	// Find the first config that has a valid protocol set
	for _, cfg := range configs {
		if cfg.Protocol != "" && cfg.UUID != "" {
			return &cfg, nil
		}
	}
	// Fallback: return the first config regardless
	if len(configs) > 0 {
		return &configs[0], nil
	}
	return nil, fmt.Errorf("no base template configuration found")
}

// mergeArteryWithBase creates a complete V2RayClientConfig by taking the artery's
// Address/Port/Latency and filling in Protocol/UUID/Network/TLSSettings from the
// base template. This ensures CompileOutbound receives a fully-populated config.
func mergeArteryWithBase(artery models.V2RayClientConfig, base *models.V2RayClientConfig) models.V2RayClientConfig {
	merged := artery
	// Only override fields that are empty in the artery but present in the base
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

// compileAndStartCore generates a multi-outbound xray config with balancer + observatory
// and (re)starts the core process. This is the key integration point with the existing
// compiler infrastructure.
func (e *Engine) compileAndStartCore() error {
	e.mu.RLock()
	state := e.state
	arteries := make([]*ArteryEntry, len(e.arteries))
	copy(arteries, e.arteries)
	cfg := e.config
	e.mu.RUnlock()

	if state != EngineStateRunning && state != EngineStateStarting {
		return fmt.Errorf("selector engine is not running")
	}

	if len(arteries) == 0 {
		return fmt.Errorf("no active arteries to compile")
	}

	// Load base template config so we can fill in Protocol/UUID/Network/TLS for
	// any arteries that only have Address+Port from the scanner.
	baseTemplate, err := getBaseTemplateConfig()
	if err != nil {
		return fmt.Errorf("failed to load base template config: %w", err)
	}

	logger.Info("Bonding", "Base template for outbound compilation",
		"protocol", baseTemplate.Protocol,
		"network", baseTemplate.Network,
		"uuid_set", baseTemplate.UUID != "",
		"tls_set", baseTemplate.TLSSettings != "",
	)

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
			Services: []string{"StatsService", "LoggerService", "HandlerService", "ObservatoryService"},
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

	// User-facing inbounds: SOCKS5 and HTTP proxy (xray binds to internal ports, Go wrapper binds to public ports)
	socksPort := cfg.SocksPort
	if socksPort <= 0 {
		socksPort = 10646
	}
	httpPort := cfg.HTTPPort
	if httpPort <= 0 {
		httpPort = 10545
	}
	socksInternalPort := socksPort + 2000
	httpInternalPort := httpPort + 2000

	xrayConfig.Inbounds = []compiler.InboundConfig{
		{
			Listen:   "127.0.0.1",
			Port:     socksInternalPort,
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
			Port:     httpInternalPort,
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

	// Build one outbound per artery using the existing CompileOutbound function.
	// Each artery's config is merged with the base template to ensure Protocol,
	// UUID, Network, and TLSSettings are populated even if the scanner-discovered
	// node only stored Address/Port/Latency.
	var balancerTags []string
	for _, a := range arteries {
		mergedConfig := mergeArteryWithBase(a.Config, baseTemplate)

		if mergedConfig.Protocol == "" {
			logger.Warn("Bonding", "Skipping artery with empty protocol after merge",
				"tag", a.Tag,
				"address", mergedConfig.Address,
				"port", mergedConfig.Port,
			)
			continue
		}

		logger.Debug("Bonding", "Compiling outbound for artery",
			"tag", a.Tag,
			"protocol", mergedConfig.Protocol,
			"address", mergedConfig.Address,
			"port", mergedConfig.Port,
			"network", mergedConfig.Network,
		)

		outbound := compiler.CompileOutbound(mergedConfig, evasionEnabled, a.Tag)
		xrayConfig.Outbounds = append(xrayConfig.Outbounds, outbound)
		balancerTags = append(balancerTags, a.Tag)
	}

	if len(balancerTags) == 0 {
		return fmt.Errorf("all arteries have empty protocol — check PebbleDB entries have valid Protocol/UUID fields")
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
				Strategy: map[string]string{"type": strategy},
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
