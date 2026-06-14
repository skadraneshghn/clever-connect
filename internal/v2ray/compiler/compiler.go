package compiler

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"strings"

	"clever-connect/internal/db"
	"clever-connect/internal/models"
	"clever-connect/internal/v2ray/core"
)

// LogConfig defines logging settings
type LogConfig struct {
	Access   string `json:"access,omitempty"`
	Error    string `json:"error,omitempty"`
	LogLevel string `json:"loglevel,omitempty"`
}

// ApiConfig defines api service settings
type ApiConfig struct {
	Tag      string   `json:"tag"`
	Services []string `json:"services"`
}

// StatsConfig is empty to enable stats
type StatsConfig struct{}

// PolicyUserConfig defines system level policy settings
type PolicyUserConfig struct {
	StatsInboundUplink    bool `json:"statsInboundUplink,omitempty"`
	StatsInboundDownlink  bool `json:"statsInboundDownlink,omitempty"`
	StatsOutboundUplink   bool `json:"statsOutboundUplink,omitempty"`
	StatsOutboundDownlink bool `json:"statsOutboundDownlink,omitempty"`
}

// PolicyLevelConfig defines user level policy settings
type PolicyLevelConfig struct {
	Handshake         int  `json:"handshake,omitempty"`
	ConnIdle          int  `json:"connIdle,omitempty"`
	UplinkOnly        int  `json:"uplinkOnly,omitempty"`
	DownlinkOnly      int  `json:"downlinkOnly,omitempty"`
	StatsUserUplink   bool `json:"statsUserUplink,omitempty"`
	StatsUserDownlink bool `json:"statsUserDownlink,omitempty"`
	BufferSize        int  `json:"bufferSize,omitempty"`
}

// PolicyConfig defines policy settings
type PolicyConfig struct {
	Levels map[string]PolicyLevelConfig `json:"levels"`
	System PolicyUserConfig             `json:"system"`
}

// SniffingConfig defines sniffing settings
type SniffingConfig struct {
	Enabled      bool     `json:"enabled"`
	DestOverride []string `json:"destOverride"`
	MetadataOnly bool     `json:"metadataOnly,omitempty"`
}

// Certificate defines certificates settings
type Certificate struct {
	CertificateFile string   `json:"certificateFile,omitempty"`
	KeyFile         string   `json:"keyFile,omitempty"`
	Certificate     []string `json:"certificate,omitempty"`
	Key             []string `json:"key,omitempty"`
}

// WsSettings defines websocket transport settings
type WsSettings struct {
	Path    string            `json:"path"`
	Headers map[string]string `json:"headers,omitempty"`
}

// GrpcSettings defines grpc transport settings
type GrpcSettings struct {
	ServiceName string `json:"serviceName"`
	MultiMode   bool   `json:"multiMode,omitempty"`
}

// SockoptConfig defines socket options
type SockoptConfig struct {
	TcpFastOpen   bool   `json:"tcpFastOpen,omitempty"`
	TcpCongestion string `json:"tcpCongestion,omitempty"`
}

// EchConfig defines encrypted client hello settings
type EchConfig struct {
	Enabled bool   `json:"enabled"`
	Config  string `json:"config,omitempty"`
}

// PaddingSettings defines TLS padding configurations
type PaddingSettings struct {
	Type string `json:"type,omitempty"`
	Size string `json:"size,omitempty"`
}

// TlsSettings defines tls settings
type TlsSettings struct {
	ServerName    string           `json:"serverName,omitempty"`
	Certificates  []Certificate    `json:"certificates,omitempty"`
	MinVersion    string           `json:"minVersion,omitempty"`
	Alpn          []string         `json:"alpn,omitempty"`
	Fingerprint   string           `json:"fingerprint,omitempty"`
	Ech           *EchConfig       `json:"ech,omitempty"`
	AllowInsecure bool             `json:"allowInsecure,omitempty"`
	Padding       *PaddingSettings `json:"padding,omitempty"`
}

// RealitySettings defines REALITY settings
type RealitySettings struct {
	Show        bool             `json:"show"`
	Dest        string           `json:"dest"`
	ServerNames []string         `json:"serverNames"`
	PrivateKey  string           `json:"privateKey,omitempty"`
	PublicKey   string           `json:"publicKey,omitempty"`
	MinClient   string           `json:"minClient,omitempty"`
	MaxClient   string           `json:"maxClient,omitempty"`
	ShortIds    []string         `json:"shortIds"`
	ServerName  string           `json:"serverName,omitempty"`
	Fingerprint string           `json:"fingerprint,omitempty"`
	SpiderX     string           `json:"spiderX,omitempty"`
	Padding     *PaddingSettings `json:"padding,omitempty"`
}

// FragmentConfig defines fragmentation settings for uTLS desync/evasion
type FragmentConfig struct {
	Packets  string `json:"packets,omitempty"`
	Length   string `json:"length,omitempty"`
	Interval string `json:"interval,omitempty"`
}

// TcpSettings defines tcp transport settings
type TcpSettings struct {
	Header map[string]interface{} `json:"header,omitempty"`
}

// KcpSettings defines mKCP transport settings
type KcpSettings struct {
	Mtu              int                    `json:"mtu,omitempty"`
	Tti              int                    `json:"tti,omitempty"`
	UplinkCapacity   int                    `json:"uplinkCapacity,omitempty"`
	DownlinkCapacity int                    `json:"downlinkCapacity,omitempty"`
	Congestion       bool                   `json:"congestion,omitempty"`
	ReadBufferSize   int                    `json:"readBufferSize,omitempty"`
	WriteBufferSize  int                    `json:"writeBufferSize,omitempty"`
	Header           map[string]interface{} `json:"header,omitempty"`
}

// QuicSettings defines quic transport settings
type QuicSettings struct {
	Security string                 `json:"security,omitempty"`
	Key      string                 `json:"key,omitempty"`
	Header   map[string]interface{} `json:"header,omitempty"`
}

// StreamSettings defines network and transport settings
type StreamSettings struct {
	Network         string           `json:"network,omitempty"`
	Security        string           `json:"security,omitempty"`
	TlsSettings     *TlsSettings     `json:"tlsSettings,omitempty"`
	RealitySettings *RealitySettings `json:"realitySettings,omitempty"`
	WsSettings      *WsSettings      `json:"wsSettings,omitempty"`
	GrpcSettings    *GrpcSettings    `json:"grpcSettings,omitempty"`
	TcpSettings     *TcpSettings     `json:"tcpSettings,omitempty"`
	KcpSettings     *KcpSettings     `json:"kcpSettings,omitempty"`
	QuicSettings    *QuicSettings    `json:"quicSettings,omitempty"`
	Sockopt         *SockoptConfig   `json:"sockopt,omitempty"`
	Fragment        *FragmentConfig  `json:"fragment,omitempty"`
}

// InboundConfig defines inbound settings
type InboundConfig struct {
	Listen         string                 `json:"listen,omitempty"`
	Port           interface{}            `json:"port"`
	Protocol       string                 `json:"protocol"`
	Settings       map[string]interface{} `json:"settings,omitempty"`
	StreamSettings *StreamSettings        `json:"streamSettings,omitempty"`
	Tag            string                 `json:"tag,omitempty"`
	Sniffing       *SniffingConfig        `json:"sniffing,omitempty"`
}

// MuxConfig defines multiplexing settings for outbounds
type MuxConfig struct {
	Enabled     bool `json:"enabled"`
	Concurrency int  `json:"concurrency,omitempty"`
}

// OutboundConfig defines outbound settings
type OutboundConfig struct {
	Protocol       string                 `json:"protocol"`
	Settings       map[string]interface{} `json:"settings,omitempty"`
	StreamSettings *StreamSettings        `json:"streamSettings,omitempty"`
	Tag            string                 `json:"tag,omitempty"`
	Mux            *MuxConfig             `json:"mux,omitempty"`
}

// RoutingRule defines routing rules
type RoutingRule struct {
	Type        string   `json:"type"`
	InboundTag  []string `json:"inboundTag,omitempty"`
	Domain      []string `json:"domain,omitempty"`
	IP          []string `json:"ip,omitempty"`
	Port        string   `json:"port,omitempty"`
	Network     string   `json:"network,omitempty"`
	OutboundTag string   `json:"outboundTag,omitempty"`
	BalancerTag string   `json:"balancerTag,omitempty"`
}

// BalancerConfig defines balancers
type BalancerConfig struct {
	Tag      string      `json:"tag"`
	Selector []string    `json:"selector"`
	Strategy interface{} `json:"strategy,omitempty"` // leastPing (object or string)
}

// RoutingConfig defines routing settings
type RoutingConfig struct {
	DomainStrategy string           `json:"domainStrategy,omitempty"`
	Rules          []RoutingRule    `json:"rules"`
	Balancers      []BalancerConfig `json:"balancers,omitempty"`
}

// DnsServerConfig defines a DNS server entry
type DnsServerConfig struct {
	Address string   `json:"address"`
	Port    int      `json:"port,omitempty"`
	Domains []string `json:"domains,omitempty"`
}

// DnsConfig defines the DNS block in Xray
type DnsConfig struct {
	Servers []interface{} `json:"servers"`
}

// ObservatoryConfig defines Xray Observatory settings
type ObservatoryConfig struct {
	SubjectSelector []string `json:"subjectSelector"`
	ProbeURL        string   `json:"probeURL,omitempty"`
	ProbeInterval   string   `json:"probeInterval,omitempty"`
}

// XrayConfig is the master configuration struct for Xray
type XrayConfig struct {
	Log         *LogConfig         `json:"log,omitempty"`
	Api         *ApiConfig         `json:"api,omitempty"`
	Stats       *StatsConfig       `json:"stats,omitempty"`
	Policy      *PolicyConfig      `json:"policy,omitempty"`
	Inbounds    []InboundConfig    `json:"inbounds"`
	Outbounds   []OutboundConfig   `json:"outbounds"`
	Routing     *RoutingConfig     `json:"routing,omitempty"`
	DNS         *DnsConfig         `json:"dns,omitempty"`
	Observatory *ObservatoryConfig `json:"observatory,omitempty"`
}

// CompileServerConfig compiles the full server configuration JSON based on the selected core
func CompileServerConfig(
	inbounds []models.V2RayInbound,
	users []models.V2RayUser,
	rules []models.V2RayRoutingRule,
) ([]byte, error) {
	coreName := core.GetSelectedCoreName()
	if coreName == "sing-box" {
		return CompileSingBoxServerConfig(inbounds, users, rules)
	}

	configBytes, err := compileServerConfigXray(inbounds, users, rules)
	if err != nil {
		return nil, err
	}

	if coreName == "v2ray" {
		return CleanXrayConfigForV2Ray(configBytes)
	}

	return configBytes, nil
}

// compileServerConfigXray compiles the full Xray server configuration JSON
func compileServerConfigXray(
	inbounds []models.V2RayInbound,
	users []models.V2RayUser,
	rules []models.V2RayRoutingRule,
) ([]byte, error) {

	config := XrayConfig{
		Log: &LogConfig{
			LogLevel: "info",
		},
		Api: &ApiConfig{
			Tag:      "api",
			Services: []string{"StatsService"},
		},
		Stats:  &StatsConfig{},
		Policy: &PolicyConfig{
			Levels: map[string]PolicyLevelConfig{
				"0": {
					StatsUserUplink:   true,
					StatsUserDownlink: true,
				},
			},
			System: PolicyUserConfig{
				StatsInboundUplink:    true,
				StatsInboundDownlink:  true,
				StatsOutboundUplink:   true,
				StatsOutboundDownlink: true,
			},
		},
		Inbounds: []InboundConfig{
			// Loopback API inbound for stats polling
			{
				Listen:   "127.0.0.1",
				Port:     10085,
				Protocol: "dokodemo-door",
				Settings: map[string]interface{}{
					"address": "127.0.0.1",
				},
				Tag: "api",
			},
		},
		Outbounds: []OutboundConfig{
			{
				Protocol: "freedom",
				Tag:      "direct",
			},
			{
				Protocol: "blackhole",
				Tag:      "blocked",
			},
		},
	}

	// Compile Inbounds from DB
	for _, in := range inbounds {
		if !in.Enabled {
			continue
		}

		inbound := InboundConfig{
			Port:     in.Port,
			Protocol: in.Protocol,
			Tag:      in.Tag,
			Sniffing: &SniffingConfig{
				Enabled:      true,
				DestOverride: []string{"http", "tls"},
			},
		}

		// Configure Clients/Users for this inbound
		var clients []map[string]interface{}
		for _, u := range users {
			if !u.Enabled || u.InboundID != in.ID {
				continue
			}

			client := map[string]interface{}{
				"id":    u.UUID,
				"email": u.Name,
			}

			if in.Protocol == "vless" && (in.TLSMode == "reality" || in.TLSMode == "tls") {
				// XTLS flow for high performance
				client["flow"] = "xtls-rprx-vision"
			}
			clients = append(clients, client)
		}

		// Protocol Specific settings
		inbound.Settings = map[string]interface{}{
			"clients": clients,
		}

		if in.Protocol == "trojan" {
			// Trojan uses password instead of ID
			var trojanClients []map[string]interface{}
			for _, u := range users {
				if !u.Enabled || u.InboundID != in.ID {
					continue
				}
				trojanClients = append(trojanClients, map[string]interface{}{
					"password": u.UUID,
					"email":    u.Name,
				})
			}
			inbound.Settings = map[string]interface{}{
				"clients": trojanClients,
			}
		}

		// Fallbacks logic
		if in.FallbackDest != "" {
			parts := strings.Split(in.FallbackDest, ":")
			var portVal interface{} = 80
			destAddr := "127.0.0.1"
			if len(parts) == 2 {
				destAddr = parts[0]
				portVal = parts[1]
			} else if len(parts) == 1 {
				portVal = parts[0]
			}
			inbound.Settings["fallbacks"] = []map[string]interface{}{
				{
					"dest": destAddr,
					"port": portVal,
				},
			}
		}

		// StreamSettings configuration
		if in.TLSMode != "none" {
			ss := &StreamSettings{
				Network: in.Network,
			}

			if in.TLSMode == "reality" {
				ss.Security = "reality"
				realityNames := []string{in.SNI}
				if in.SNI == "" {
					realityNames = []string{"yahoo.com"} // Fallback benign domain
				}

				shortIds := []string{}
				if in.RealityShortIDs != "" {
					shortIds = strings.Split(in.RealityShortIDs, ",")
				}

				ss.RealitySettings = &RealitySettings{
					Show:        false,
					Dest:        in.FallbackDest,
					ServerNames: realityNames,
					PrivateKey:  in.RealityPrivateKey,
					ShortIds:    shortIds,
				}
				if ss.RealitySettings.Dest == "" {
					ss.RealitySettings.Dest = "127.0.0.1:80" // default fallback
				}
			} else if in.TLSMode == "tls" {
				ss.Security = "tls"
				ss.TlsSettings = &TlsSettings{
					ServerName: in.SNI,
				}
			}

			// Transport specific stream settings
			if in.Network == "ws" {
				path := in.Path
				if path == "" {
					path = "/ws"
				}
				ss.WsSettings = &WsSettings{
					Path: path,
				}
			} else if in.Network == "grpc" {
				serviceName := in.Path
				if serviceName == "" {
					serviceName = "TunService"
				}
				ss.GrpcSettings = &GrpcSettings{
					ServiceName: serviceName,
				}
			}

			inbound.StreamSettings = ss
		}

		config.Inbounds = append(config.Inbounds, inbound)
	}

	// Add routing rules from DB
	routing := &RoutingConfig{
		DomainStrategy: "AsIs",
		Rules: []RoutingRule{
			{
				Type:        "field",
				Port:        "10085",
				OutboundTag: "api",
			},
		},
	}

	for _, rule := range rules {
		r := RoutingRule{
			Type:        "field",
			OutboundTag: rule.Action,
		}
		if rule.RuleType == "domain" || rule.RuleType == "geosite" {
			r.Domain = []string{rule.Value}
		} else if rule.RuleType == "ip" || rule.RuleType == "geoip" {
			r.IP = []string{rule.Value}
		}
		routing.Rules = append(routing.Rules, r)
	}

	config.Routing = routing

	return json.MarshalIndent(config, "", "  ")
}

// CompileClientConfig compiles the client-side local config JSON based on the selected core
func CompileClientConfig(
	activeConfig models.V2RayClientConfig,
	socksPort int,
	httpPort int,
	evasionEnabled bool,
	tcpDecoySni string,
) ([]byte, error) {
	return CompileClientConfigForCore(core.GetSelectedCoreName(), activeConfig, socksPort, httpPort, evasionEnabled, tcpDecoySni)
}

// CompileClientConfigForCore compiles the client-side local config JSON for a specific core
func CompileClientConfigForCore(
	coreName string,
	activeConfig models.V2RayClientConfig,
	socksPort int,
	httpPort int,
	evasionEnabled bool,
	tcpDecoySni string,
) ([]byte, error) {
	if coreName == "sing-box" {
		return CompileSingBoxClientConfig(activeConfig, socksPort, httpPort, evasionEnabled, tcpDecoySni)
	}

	configBytes, err := compileClientConfigXray(activeConfig, socksPort, httpPort, evasionEnabled, tcpDecoySni)
	if err != nil {
		return nil, err
	}

	if coreName == "v2ray" {
		return CleanXrayConfigForV2Ray(configBytes)
	}

	return configBytes, nil
}


// compileClientConfigXray compiles the client-side local Xray config JSON
func compileClientConfigXray(
	activeConfig models.V2RayClientConfig,
	socksPort int,
	httpPort int,
	evasionEnabled bool,
	tcpDecoySni string,
) ([]byte, error) {

	var config XrayConfig
	useTemplate := false

	coreName := core.GetSelectedCoreName()
	var setting models.V2RayClientSetting
	key := "core_template_xray"
	if coreName == "v2ray" {
		key = "core_template_v2ray"
	}

	if db.DB != nil {
		if err := db.DB.Where("key = ?", key).First(&setting).Error; err == nil && setting.Value != "" {
			if json.Unmarshal([]byte(setting.Value), &config) == nil {
				useTemplate = true
			}
		}
	}

	if !useTemplate {
		config = XrayConfig{
			Log: &LogConfig{
				LogLevel: "warning",
			},
			Outbounds: []OutboundConfig{},
		}
	}

	foundSocks := false
	foundHttp := false
	for i, in := range config.Inbounds {
		if in.Tag == "socks-in" {
			config.Inbounds[i].Port = socksPort
			foundSocks = true
		} else if in.Tag == "http-in" {
			config.Inbounds[i].Port = httpPort
			foundHttp = true
		}
	}

	if !foundSocks {
		config.Inbounds = append(config.Inbounds, InboundConfig{
			Listen:   "127.0.0.1",
			Port:     socksPort,
			Protocol: "socks",
			Settings: map[string]interface{}{
				"auth": "noauth",
				"udp":  true,
			},
			Tag: "socks-in",
		})
	}

	if !foundHttp {
		config.Inbounds = append(config.Inbounds, InboundConfig{
			Listen:   "127.0.0.1",
			Port:     httpPort,
			Protocol: "http",
			Settings: map[string]interface{}{
				"allowRedirect": true,
			},
			Tag: "http-in",
		})
	}

	foundApiInbound := false
	for _, in := range config.Inbounds {
		if in.Tag == "api" {
			foundApiInbound = true
			break
		}
	}
	if !foundApiInbound {
		config.Inbounds = append(config.Inbounds, InboundConfig{
			Listen:   "127.0.0.1",
			Port:     10085,
			Protocol: "dokodemo-door",
			Settings: map[string]interface{}{
				"address": "127.0.0.1",
			},
			Tag: "api",
		})
	}

	// Inject Stats and API config
	if config.Stats == nil {
		config.Stats = &StatsConfig{}
	}
	if config.Api == nil {
		config.Api = &ApiConfig{
			Tag:      "api",
			Services: []string{"StatsService", "LoggerService"},
		}
	}
	if config.Policy == nil {
		config.Policy = &PolicyConfig{}
	}
	// PolicyUserConfig is not a pointer, just set its fields directly
	config.Policy.System = PolicyUserConfig{
		StatsInboundUplink:    true,
		StatsInboundDownlink:  true,
		StatsOutboundUplink:   true,
		StatsOutboundDownlink: true,
	}

	// ────────────────────────────────────────────────────────────────────────
	// 1. OUTBOUND CONFIGURATION (LOAD BALANCER vs SINGLE NODE)
	// ────────────────────────────────────────────────────────────────────────
	isAutoBalancer := activeConfig.Protocol == "balancer" || activeConfig.Address == "auto" || strings.Contains(activeConfig.Name, "Auto")

	if isAutoBalancer && db.DB != nil {
		// Fetch all configs from DB to load-balance
		var allConfigs []models.V2RayClientConfig
		if err := db.DB.Find(&allConfigs).Error; err == nil && len(allConfigs) > 0 {
			var balancerTargets []string
			for _, cfg := range allConfigs {
				if cfg.Protocol == "balancer" || cfg.Address == "auto" {
					continue
				}
				tag := fmt.Sprintf("proxy-node-%d", cfg.ID)
				outbound := CompileOutbound(cfg, evasionEnabled, tag)
				config.Outbounds = append(config.Outbounds, outbound)
				balancerTargets = append(balancerTargets, tag)
			}

			// Add Balancer Config
			strategy := "leastPing"
			// If all latencies are within 20% of each other, fall back to round-robin/random
			if len(allConfigs) > 1 {
				var lats []int
				for _, cfg := range allConfigs {
					if cfg.LatencyMs > 0 {
						lats = append(lats, cfg.LatencyMs)
					}
				}
				if len(lats) == len(allConfigs) && len(lats) > 0 {
					minL := lats[0]
					maxL := lats[0]
					for _, l := range lats {
						if l < minL {
							minL = l
						}
						if l > maxL {
							maxL = l
						}
					}
					if float64(maxL-minL) <= 0.20*float64(minL) {
						strategy = "random"
					}
				}
			}

			// Configure load balancer selector and strategy
			config.Routing = &RoutingConfig{
				DomainStrategy: "IPOnDemand",
				Balancers: []BalancerConfig{
					{
						Tag:      "balancer",
						Selector: balancerTargets,
						Strategy: map[string]string{"type": strategy},
					},
				},
			}

			// Configure Observatory block
			config.Observatory = &ObservatoryConfig{
				SubjectSelector: balancerTargets,
				ProbeURL:        "http://www.gstatic.com/generate_204",
				ProbeInterval:   "10s",
			}
		} else {
			// Fallback if no configurations exist
			config.Outbounds = append(config.Outbounds, CompileOutbound(activeConfig, evasionEnabled, "proxy"))
		}
	} else {
		// Single Proxy Outbound
		config.Outbounds = append(config.Outbounds, CompileOutbound(activeConfig, evasionEnabled, "proxy"))
	}

	foundDirect := false
	foundBlock := false
	for _, out := range config.Outbounds {
		if out.Tag == "direct" {
			foundDirect = true
		} else if out.Tag == "block" || out.Tag == "blocked" {
			foundBlock = true
		}
	}

	if !foundDirect {
		config.Outbounds = append(config.Outbounds, OutboundConfig{
			Protocol: "freedom",
			Tag:      "direct",
		})
	}
	if !foundBlock {
		config.Outbounds = append(config.Outbounds, OutboundConfig{
			Protocol: "blackhole",
			Tag:      "block",
		})
	}

	// ────────────────────────────────────────────────────────────────────────
	// 2. DNS SPLIT CONFIGURATION
	// ────────────────────────────────────────────────────────────────────────
	dohURL := "https://1.1.1.1/dns-query"
	if db.DB != nil {
		var setting models.V2RayClientSetting
		if err := db.DB.Where("key = ?", "dns_doh_url").First(&setting).Error; err == nil && setting.Value != "" {
			dohURL = setting.Value
		}
	}

	if !useTemplate || config.DNS == nil {
		config.DNS = &DnsConfig{
			Servers: []interface{}{
				dohURL,
				DnsServerConfig{
					Address: "8.8.8.8",
					Domains: []string{"geosite:ir", "regexp:.*\\.ir$"},
				},
				"localhost",
			},
		}
	}

	// ────────────────────────────────────────────────────────────────────────
	// 3. PRESET & CUSTOM ROUTING RULES
	// ────────────────────────────────────────────────────────────────────────
	routingMode := "bypass_domestic"
	if db.DB != nil {
		var setting models.V2RayClientSetting
		if err := db.DB.Where("key = ?", "routing_mode").First(&setting).Error; err == nil && setting.Value != "" {
			routingMode = setting.Value
		}
	}

	if config.Routing == nil {
		config.Routing = &RoutingConfig{
			DomainStrategy: "IPOnDemand",
		}
	}

	// Rules list
	rules := config.Routing.Rules

	// Preset modes
	switch routingMode {
	case "global":
		// All traffic routes to proxy / balancer. Only bypass local private addresses
		rules = append(rules, RoutingRule{
			Type:        "field",
			Domain:      []string{"geosite:private"},
			OutboundTag: "direct",
		})
		rules = append(rules, RoutingRule{
			Type:        "field",
			IP:          []string{"geoip:private"},
			OutboundTag: "direct",
		})

	case "bypass_domestic":
		// Bypass domestic Iranian websites and private local networks
		rules = append(rules, RoutingRule{
			Type:        "field",
			Domain:      []string{"geosite:private", "geosite:ir", "regexp:.*\\.ir$"},
			OutboundTag: "direct",
		})
		rules = append(rules, RoutingRule{
			Type:        "field",
			IP:          []string{"geoip:private", "geoip:ir"},
			OutboundTag: "direct",
		})

	case "block_ads":
		// Block ads and trackers, bypass domestic, rest via proxy
		rules = append(rules, RoutingRule{
			Type:        "field",
			Domain:      []string{"geosite:category-ads-all"},
			OutboundTag: "block",
		})
		rules = append(rules, RoutingRule{
			Type:        "field",
			Domain:      []string{"geosite:private", "geosite:ir", "regexp:.*\\.ir$"},
			OutboundTag: "direct",
		})
		rules = append(rules, RoutingRule{
			Type:        "field",
			IP:          []string{"geoip:private", "geoip:ir"},
			OutboundTag: "direct",
		})

	case "custom":
		// Read custom rules from settings
		if db.DB != nil {
			var setting models.V2RayClientSetting
			if err := db.DB.Where("key = ?", "custom_routing_rules").First(&setting).Error; err == nil && setting.Value != "" {
				var customRules []RoutingRule
				if err := json.Unmarshal([]byte(setting.Value), &customRules); err == nil {
					rules = append(rules, customRules...)
				}
			}
		}
		// Fallback direct for local addresses anyway
		rules = append(rules, RoutingRule{
			Type:        "field",
			Domain:      []string{"geosite:private"},
			OutboundTag: "direct",
		})
		rules = append(rules, RoutingRule{
			Type:        "field",
			IP:          []string{"geoip:private"},
			OutboundTag: "direct",
		})
	}

	// Route DNS traffic direct or proxy correctly
	rules = append(rules, RoutingRule{
		Type:        "field",
		Port:        "53",
		OutboundTag: "direct",
	})

	// Direct unmatched traffic to either load balancer or single proxy outbound
	defaultOutboundTag := "proxy"
	if isAutoBalancer && len(config.Routing.Balancers) > 0 {
		// Route to balancer
		rules = append(rules, RoutingRule{
			Type:        "field",
			Network:     "tcp,udp",
			BalancerTag: "balancer",
		})
	} else {
		// Route to single proxy outbound
		rules = append(rules, RoutingRule{
			Type:        "field",
			Network:     "tcp,udp",
			OutboundTag: defaultOutboundTag,
		})
	}

	foundApiRule := false
	for _, r := range rules {
		if r.OutboundTag == "api" {
			foundApiRule = true
			break
		}
	}
	if !foundApiRule {
		apiRule := RoutingRule{
			Type:        "field",
			InboundTag:  []string{"api"},
			OutboundTag: "api",
		}
		rules = append([]RoutingRule{apiRule}, rules...)
	}

	config.Routing.Rules = rules

	// Generate JSON config
	configBytes, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return nil, err
	}

	// Pre-flight schema validation
	if err := ValidateXrayConfig(configBytes); err != nil {
		return nil, fmt.Errorf("pre-flight schema validation failed: %w", err)
	}

	return configBytes, nil
}

// CompileOutbound builds a single outbound Xray configuration for a client node
func CompileOutbound(activeConfig models.V2RayClientConfig, evasionEnabled bool, tag string) OutboundConfig {
	outbound := OutboundConfig{
		Protocol: activeConfig.Protocol,
		Tag:      tag,
	}

	var clientTlsSettings *TlsSettings
	var clientRealitySettings *RealitySettings
	var clientStreamSettings *StreamSettings

	var dbTlsSettings map[string]interface{}
	if activeConfig.TLSSettings != "" {
		_ = json.Unmarshal([]byte(activeConfig.TLSSettings), &dbTlsSettings)
	}

	clientStreamSettings = &StreamSettings{
		Network: activeConfig.Network,
	}

	if activeConfig.Network == "ws" {
		path := "/ws"
		if p, ok := dbTlsSettings["path"].(string); ok && p != "" {
			path = p
		}
		headers := map[string]string{}
		if h, ok := dbTlsSettings["host"].(string); ok && h != "" {
			headers["Host"] = h
		}
		if hdrs, ok := dbTlsSettings["headers"].(map[string]interface{}); ok {
			for k, v := range hdrs {
				if vs, ok := v.(string); ok {
					headers[k] = vs
				}
			}
		}
		var wsHeaders map[string]string
		if len(headers) > 0 {
			wsHeaders = headers
		}
		clientStreamSettings.WsSettings = &WsSettings{
			Path:    path,
			Headers: wsHeaders,
		}
	} else if activeConfig.Network == "grpc" {
		svc := "TunService"
		if s, ok := dbTlsSettings["serviceName"].(string); ok && s != "" {
			svc = s
		}
		multiMode := false
		if mm, ok := dbTlsSettings["multiMode"].(bool); ok {
			multiMode = mm
		}
		clientStreamSettings.GrpcSettings = &GrpcSettings{
			ServiceName: svc,
			MultiMode:   multiMode,
		}
	} else if activeConfig.Network == "tcp" {
		if tcpMap, ok := dbTlsSettings["tcpSettings"].(map[string]interface{}); ok {
			var tcpSettings TcpSettings
			if jsonBytes, err := json.Marshal(tcpMap); err == nil {
				_ = json.Unmarshal(jsonBytes, &tcpSettings)
				clientStreamSettings.TcpSettings = &tcpSettings
			}
		} else if headerMap, ok := dbTlsSettings["header"].(map[string]interface{}); ok {
			clientStreamSettings.TcpSettings = &TcpSettings{
				Header: headerMap,
			}
		}
	} else if activeConfig.Network == "kcp" {
		if kcpMap, ok := dbTlsSettings["kcpSettings"].(map[string]interface{}); ok {
			var kcpSettings KcpSettings
			if jsonBytes, err := json.Marshal(kcpMap); err == nil {
				_ = json.Unmarshal(jsonBytes, &kcpSettings)
				clientStreamSettings.KcpSettings = &kcpSettings
			}
		}
	} else if activeConfig.Network == "quic" {
		if quicMap, ok := dbTlsSettings["quicSettings"].(map[string]interface{}); ok {
			var quicSettings QuicSettings
			if jsonBytes, err := json.Marshal(quicMap); err == nil {
				_ = json.Unmarshal(jsonBytes, &quicSettings)
				clientStreamSettings.QuicSettings = &quicSettings
			}
		}
	}

	security := "none"
	if s, ok := dbTlsSettings["security"].(string); ok {
		security = s
	}

	evasionFingerprint := "chrome"
	evasionFragment := true
	evasionTcpFastOpen := true
	evasionMixedCase := false
	evasionPadding := false

	if db.DB != nil {
		var setting models.V2RayClientSetting
		if err := db.DB.Where("key = ?", "evasion_fingerprint").First(&setting).Error; err == nil && setting.Value != "" {
			evasionFingerprint = setting.Value
		}
		setting = models.V2RayClientSetting{}
		if err := db.DB.Where("key = ?", "evasion_fragment").First(&setting).Error; err == nil {
			evasionFragment = setting.Value == "true"
		}
		setting = models.V2RayClientSetting{}
		if err := db.DB.Where("key = ?", "evasion_tcp_fast_open").First(&setting).Error; err == nil {
			evasionTcpFastOpen = setting.Value == "true"
		}
		setting = models.V2RayClientSetting{}
		if err := db.DB.Where("key = ?", "evasion_mixed_case").First(&setting).Error; err == nil {
			evasionMixedCase = setting.Value == "true"
		}
		setting = models.V2RayClientSetting{}
		if err := db.DB.Where("key = ?", "evasion_padding").First(&setting).Error; err == nil {
			evasionPadding = setting.Value == "true"
		}
	}

	if evasionPadding {
		evasionFragment = false
	}

	if security == "reality" {
		clientStreamSettings.Security = "reality"
		pubKey, _ := dbTlsSettings["publicKey"].(string)
		shortId, _ := dbTlsSettings["shortId"].(string)
		sni, _ := dbTlsSettings["sni"].(string)
		dest := ""
		spiderX := ""
		minClient := ""
		maxClient := ""

		if rMap, ok := dbTlsSettings["realitySettings"].(map[string]interface{}); ok {
			if pk, ok := rMap["publicKey"].(string); ok && pk != "" { pubKey = pk }
			if sid, ok := rMap["shortId"].(string); ok && sid != "" { shortId = sid }
			if sn, ok := rMap["serverName"].(string); ok && sn != "" { sni = sn }
			if d, ok := rMap["dest"].(string); ok { dest = d }
			if sp, ok := rMap["spiderX"].(string); ok { spiderX = sp }
			if minC, ok := rMap["minClient"].(string); ok { minClient = minC }
			if maxC, ok := rMap["maxClient"].(string); ok { maxClient = maxC }
		}

		shortIds := []string{}
		if shortId != "" {
			shortIds = []string{shortId}
		}

		if evasionMixedCase && sni != "" {
			sni = RandomizeCase(sni)
		}

		var paddingSettings *PaddingSettings
		if evasionPadding {
			paddingSettings = &PaddingSettings{
				Type: "random",
				Size: "100-500",
			}
		}

		clientRealitySettings = &RealitySettings{
			Show:        false,
			PublicKey:   pubKey,
			ShortIds:    shortIds,
			ServerName:  sni,
			Dest:        dest,
			MinClient:   minClient,
			MaxClient:   maxClient,
			SpiderX:     spiderX,
			Padding:     paddingSettings,
		}

		if evasionEnabled || evasionMixedCase || evasionPadding {
			clientRealitySettings.Fingerprint = evasionFingerprint
		}
		clientStreamSettings.RealitySettings = clientRealitySettings
	} else if security == "tls" {
		clientStreamSettings.Security = "tls"
		sni, _ := dbTlsSettings["sni"].(string)
		var alpn []string
		allowInsecure := false
		if tMap, ok := dbTlsSettings["tlsSettings"].(map[string]interface{}); ok {
			if sn, ok := tMap["serverName"].(string); ok && sn != "" { sni = sn }
			if alp, ok := tMap["alpn"].([]interface{}); ok {
				for _, a := range alp {
					if as, ok := a.(string); ok {
						alpn = append(alpn, as)
					}
				}
			}
			if ins, ok := tMap["allowInsecure"].(bool); ok {
				allowInsecure = ins
			}
		}

		if evasionMixedCase && sni != "" {
			sni = RandomizeCase(sni)
		}

		var paddingSettings *PaddingSettings
		if evasionPadding {
			paddingSettings = &PaddingSettings{
				Type: "random",
				Size: "100-500",
			}
		}

		clientTlsSettings = &TlsSettings{
			ServerName:    sni,
			Alpn:          alpn,
			AllowInsecure: allowInsecure,
			Padding:       paddingSettings,
		}
		if evasionEnabled || evasionMixedCase || evasionPadding {
			clientTlsSettings.Fingerprint = evasionFingerprint

			// Encrypted Client Hello (ECH) support
			echEnabled := false
			echConfig := ""
			if db.DB != nil {
				var setting models.V2RayClientSetting
				if err := db.DB.Where("key = ?", "evasion_ech_enabled").First(&setting).Error; err == nil {
					echEnabled = setting.Value == "true"
				}
				if err := db.DB.Where("key = ?", "evasion_ech_config").First(&setting).Error; err == nil {
					echConfig = setting.Value
				}
			}
			if echEnabled {
				clientTlsSettings.Ech = &EchConfig{
					Enabled: true,
					Config:  echConfig,
				}
			}
		}
		clientStreamSettings.TlsSettings = clientTlsSettings
	}

	if evasionEnabled {
		// TCP Brutal Congestion Control or TCP Fast Open
		tcpFastOpen := evasionTcpFastOpen
		tcpBrutal := false
		if db.DB != nil {
			var setting models.V2RayClientSetting
			if err := db.DB.Where("key = ?", "evasion_tcp_brutal").First(&setting).Error; err == nil {
				tcpBrutal = setting.Value == "true"
			}
		}

		sockopt := &SockoptConfig{
			TcpFastOpen: tcpFastOpen,
		}
		if tcpBrutal {
			sockopt.TcpCongestion = "brutal"
		}
		clientStreamSettings.Sockopt = sockopt

		if evasionFragment {
			fragMode := "default"
			fragPackets := "tlshello"
			fragLength := "100-200"
			fragInterval := "10-20"

			if db.DB != nil {
				var setting models.V2RayClientSetting
				if err := db.DB.Where("key = ?", "fragment_mode").First(&setting).Error; err == nil && setting.Value != "" {
					fragMode = setting.Value
				}
				if err := db.DB.Where("key = ?", "fragment_packets").First(&setting).Error; err == nil && setting.Value != "" {
					fragPackets = setting.Value
				}
				if err := db.DB.Where("key = ?", "fragment_length").First(&setting).Error; err == nil && setting.Value != "" {
					fragLength = setting.Value
				}
				if err := db.DB.Where("key = ?", "fragment_interval").First(&setting).Error; err == nil && setting.Value != "" {
					fragInterval = setting.Value
				}
			}

			switch fragMode {
			case "domain":
				// Targets the domain/SNI by splitting very early (e.g. 1-5 bytes) to desync the SNI record header
				clientStreamSettings.Fragment = &FragmentConfig{
					Packets:  fragPackets,
					Length:   "1-5",
					Interval: "5-15",
				}
			case "random":
				// Aggressive micro-chunks at random intervals
				clientStreamSettings.Fragment = &FragmentConfig{
					Packets:  fragPackets,
					Length:   "1-3",
					Interval: "1-5",
				}
			default: // "default" or custom
				clientStreamSettings.Fragment = &FragmentConfig{
					Packets:  fragPackets,
					Length:   fragLength,
					Interval: fragInterval,
				}
			}
		}
	}

	var outboundSettings map[string]interface{}
	if activeConfig.Protocol == "vless" {
		flowVal := ""
		if f, ok := dbTlsSettings["flow"].(string); ok {
			flowVal = f
		}
		encryptVal := "none"
		if e, ok := dbTlsSettings["encryption"].(string); ok {
			encryptVal = e
		}
		userMap := map[string]interface{}{
			"id":         activeConfig.UUID,
			"encryption": encryptVal,
		}
		if flowVal != "" {
			userMap["flow"] = flowVal
		}
		outboundSettings = map[string]interface{}{
			"vnext": []map[string]interface{}{
				{
					"address": activeConfig.Address,
					"port":    activeConfig.Port,
					"users": []map[string]interface{}{
						userMap,
					},
				},
			},
		}
	} else if activeConfig.Protocol == "vmess" {
		secVal := "auto"
		if s, ok := dbTlsSettings["vmess_security"].(string); ok {
			secVal = s
		} else if s, ok := dbTlsSettings["security_vmess"].(string); ok {
			secVal = s
		}
		alterIdVal := 0
		if aid, ok := dbTlsSettings["alterId"].(float64); ok {
			alterIdVal = int(aid)
		} else if aid, ok := dbTlsSettings["alterId"].(int); ok {
			alterIdVal = aid
		}
		outboundSettings = map[string]interface{}{
			"vnext": []map[string]interface{}{
				{
					"address": activeConfig.Address,
					"port":    activeConfig.Port,
					"users": []map[string]interface{}{
						{
							"id":       activeConfig.UUID,
							"security": secVal,
							"alterId":  alterIdVal,
						},
					},
				},
			},
		}
	} else if activeConfig.Protocol == "trojan" {
		outboundSettings = map[string]interface{}{
			"servers": []map[string]interface{}{
				{
					"address":  activeConfig.Address,
					"port":     activeConfig.Port,
					"password": activeConfig.UUID,
				},
			},
		}
	} else if activeConfig.Protocol == "shadowsocks" {
		method := "aes-256-gcm"
		if activeConfig.TLSSettings != "" {
			var m map[string]interface{}
			if err := json.Unmarshal([]byte(activeConfig.TLSSettings), &m); err == nil {
				if meth, ok := m["method"].(string); ok && meth != "" {
					method = meth
				}
			}
		}
		outboundSettings = map[string]interface{}{
			"servers": []map[string]interface{}{
				{
					"address":  activeConfig.Address,
					"port":     activeConfig.Port,
					"method":   method,
					"password": activeConfig.UUID,
				},
			},
		}
	}

	outbound.Settings = outboundSettings
	outbound.StreamSettings = clientStreamSettings

	// Connection Multiplexing (Mux)
	if activeConfig.MuxEnabled {
		concurrency := 8
		if muxC, ok := dbTlsSettings["muxConcurrency"].(float64); ok {
			concurrency = int(muxC)
		} else if muxC, ok := dbTlsSettings["muxConcurrency"].(int); ok {
			concurrency = muxC
		}
		outbound.Mux = &MuxConfig{
			Enabled:     true,
			Concurrency: concurrency,
		}
	}

	return outbound
}

// ValidateXrayConfig parses and validates config JSON against requirements
func ValidateXrayConfig(configJSON []byte) error {
	coreName := core.GetSelectedCoreName()
	if coreName == "sing-box" {
		var config SingBoxConfig
		if err := json.Unmarshal(configJSON, &config); err != nil {
			return fmt.Errorf("JSON schema syntax error: %w", err)
		}
		if len(config.Inbounds) == 0 {
			return fmt.Errorf("validation error: at least one inbound must be defined")
		}
		if len(config.Outbounds) == 0 {
			return fmt.Errorf("validation error: at least one outbound must be defined")
		}
		return nil
	}

	var config XrayConfig
	if err := json.Unmarshal(configJSON, &config); err != nil {
		return fmt.Errorf("JSON schema syntax error: %w", err)
	}

	if len(config.Inbounds) == 0 {
		return fmt.Errorf("validation error: at least one inbound must be defined")
	}

	for idx, inbound := range config.Inbounds {
		if inbound.Protocol == "" {
			return fmt.Errorf("validation error: inbound[%d] has empty protocol", idx)
		}
		if inbound.Port == nil {
			return fmt.Errorf("validation error: inbound[%d] has missing port", idx)
		}
	}

	if len(config.Outbounds) == 0 {
		return fmt.Errorf("validation error: at least one outbound must be defined")
	}

	for idx, outbound := range config.Outbounds {
		if outbound.Protocol == "" && outbound.Tag == "" {
			return fmt.Errorf("validation error: outbound[%d] has empty protocol and tag", idx)
		}
		if outbound.Tag == "" {
			return fmt.Errorf("validation error: outbound[%d] has empty tag", idx)
		}
	}

	return nil
}

// CleanXrayConfigForV2Ray parses the compiled Xray config JSON and recursively strips out Xray-only unsupported features for V2Ray core
func CleanXrayConfigForV2Ray(configJSON []byte) ([]byte, error) {
	var m map[string]interface{}
	if err := json.Unmarshal(configJSON, &m); err != nil {
		return nil, err
	}

	cleanMapForV2Ray(m)

	return json.MarshalIndent(m, "", "  ")
}

func cleanMapForV2Ray(m map[string]interface{}) {
	for k, v := range m {
		// 1. Remove VLESS XTLS flow
		if k == "flow" && (v == "xtls-rprx-vision" || v == "xtls-rprx-direct" || v == "xtls-rprx-vision-udp443") {
			delete(m, k)
			continue
		}
		// 2. Reality security -> none / downgrade
		if k == "security" && v == "reality" {
			m[k] = "none"
			delete(m, "realitySettings")
			continue
		}
		// 3. Evasion blocks not supported in V2Ray
		if k == "fragment" || k == "ech" {
			delete(m, k)
			continue
		}
		// 4. TCP Brutal congestion not supported in standard V2Ray
		if k == "tcpCongestion" && v == "brutal" {
			delete(m, k)
			continue
		}
		// 5. Observatory load balancing block not natively supported in standard V2Ray
		if k == "observatory" {
			delete(m, k)
			continue
		}

		// Recurse
		if childMap, ok := v.(map[string]interface{}); ok {
			cleanMapForV2Ray(childMap)
		} else if childSlice, ok := v.([]interface{}); ok {
			for _, item := range childSlice {
				if itemMap, ok := item.(map[string]interface{}); ok {
					cleanMapForV2Ray(itemMap)
				}
			}
		}
	}
}

// RandomizeCase dynamically toggles the case of alpha characters
// e.g., "www.google.com" -> "wWw.GoOgLe.CoM"
func RandomizeCase(domain string) string {
	var result strings.Builder
	for _, char := range domain {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') {
			if rand.Intn(2) == 0 {
				// Force lowercase
				result.WriteRune(char | 32)
			} else {
				// Force uppercase
				result.WriteRune(char &^ 32)
			}
		} else {
			result.WriteRune(char)
		}
	}
	return result.String()
}

