package compiler

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"clever-connect/internal/db"
	"clever-connect/internal/models"
)

// SingBoxInbound represents a Sing-Box inbound configuration
type SingBoxInbound struct {
	Type       string                 `json:"type"`
	Tag        string                 `json:"tag"`
	Listen     string                 `json:"listen,omitempty"`
	ListenPort int                    `json:"listen_port"`
	Users      []SingBoxUser          `json:"users,omitempty"`
	TLS        *SingBoxInboundTLS     `json:"tls,omitempty"`
	Transport  *SingBoxTransport      `json:"transport,omitempty"`
	Method     string                 `json:"method,omitempty"`
	Password   string                 `json:"password,omitempty"`
}

type SingBoxUser struct {
	UUID     string `json:"uuid,omitempty"`
	Password string `json:"password,omitempty"`
	Name     string `json:"name,omitempty"`
}

type SingBoxInboundTLS struct {
	Enabled    bool                  `json:"enabled"`
	ServerName string                `json:"server_name,omitempty"`
	Reality    *SingBoxInboundReality `json:"reality,omitempty"`
}

type SingBoxInboundReality struct {
	Enabled    bool             `json:"enabled"`
	Handshake  SingBoxHandshake `json:"handshake"`
	PrivateKey string           `json:"private_key"`
	ShortID    []string         `json:"short_id,omitempty"`
}

type SingBoxHandshake struct {
	Server     string `json:"server"`
	ServerPort int    `json:"server_port"`
}

type SingBoxTransport struct {
	Type        string `json:"type"`
	Path        string `json:"path,omitempty"`
	ServiceName string `json:"service_name,omitempty"`
}

type SingBoxOutbound struct {
	Type       string             `json:"type"`
	Tag        string             `json:"tag"`
	Server     string             `json:"server,omitempty"`
	ServerPort int                `json:"server_port,omitempty"`
	UUID       string             `json:"uuid,omitempty"`
	Password   string             `json:"password,omitempty"`
	Security   string             `json:"security,omitempty"`
	Flow       string             `json:"flow,omitempty"`
	TLS        *SingBoxOutboundTLS `json:"tls,omitempty"`
	Transport  *SingBoxTransport  `json:"transport,omitempty"`
	Multiplex  *SingBoxMultiplex  `json:"multiplex,omitempty"`
	Method     string             `json:"method,omitempty"`
	Outbounds  []string           `json:"outbounds,omitempty"` // For urltest / load balancer
	URL        string             `json:"url,omitempty"`       // For urltest
	Interval   string             `json:"interval,omitempty"`  // For urltest
}

type SingBoxOutboundTLS struct {
	Enabled    bool                `json:"enabled"`
	ServerName string              `json:"server_name,omitempty"`
	Utls       *SingBoxUtls        `json:"utls,omitempty"`
	Reality    *SingBoxOutReality  `json:"reality,omitempty"`
	Fragment   *SingBoxFragment    `json:"fragment,omitempty"`
}

type SingBoxFragment struct {
	Enabled bool   `json:"enabled,omitempty"`
	Size    string `json:"size,omitempty"`
	Sleep   string `json:"sleep,omitempty"`
}

type SingBoxUtls struct {
	Enabled     bool   `json:"enabled"`
	Fingerprint string `json:"fingerprint,omitempty"`
}

type SingBoxOutReality struct {
	Enabled   bool   `json:"enabled"`
	PublicKey string `json:"public_key"`
	ShortID   string `json:"short_id,omitempty"`
}

type SingBoxMultiplex struct {
	Enabled    bool   `json:"enabled"`
	Protocol   string `json:"protocol,omitempty"`
	MaxStreams int    `json:"max_streams,omitempty"`
}

type SingBoxRouteRule struct {
	Port     int      `json:"port,omitempty"`
	Geosite  []string `json:"geosite,omitempty"`
	Geoip    []string `json:"geoip,omitempty"`
	Outbound string   `json:"outbound"`
}

type SingBoxRoute struct {
	Rules []SingBoxRouteRule `json:"rules"`
}

type SingBoxDNSServer struct {
	Tag     string   `json:"tag"`
	Address string   `json:"address"`
	Detour  string   `json:"detour,omitempty"`
}

type SingBoxDNSRule struct {
	Geosite []string `json:"geosite,omitempty"`
	Server  string   `json:"server"`
}

type SingBoxDNS struct {
	Servers []SingBoxDNSServer `json:"servers"`
	Rules   []SingBoxDNSRule   `json:"rules,omitempty"`
}

type SingBoxConfig struct {
	Log       *SingBoxLog       `json:"log,omitempty"`
	Inbounds  []SingBoxInbound  `json:"inbounds"`
	Outbounds []SingBoxOutbound `json:"outbounds"`
	Route     *SingBoxRoute     `json:"route,omitempty"`
	DNS       *SingBoxDNS       `json:"dns,omitempty"`
}

type SingBoxLog struct {
	Level string `json:"level"`
}

func CompileSingBoxServerConfig(
	inbounds []models.V2RayInbound,
	users []models.V2RayUser,
	rules []models.V2RayRoutingRule,
) ([]byte, error) {
	config := SingBoxConfig{
		Log: &SingBoxLog{
			Level: "info",
		},
		Outbounds: []SingBoxOutbound{
			{
				Type: "direct",
				Tag:  "direct",
			},
			{
				Type: "block",
				Tag:  "blocked",
			},
		},
	}

	for _, in := range inbounds {
		if !in.Enabled {
			continue
		}

		inbound := SingBoxInbound{
			Type:       in.Protocol,
			Tag:        in.Tag,
			Listen:     "0.0.0.0",
			ListenPort: in.Port,
		}

		// Protocol Specific clients/users
		if in.Protocol == "vless" || in.Protocol == "vmess" {
			var sbUsers []SingBoxUser
			for _, u := range users {
				if !u.Enabled || u.InboundID != in.ID {
					continue
				}
				sbUsers = append(sbUsers, SingBoxUser{
					UUID: u.UUID,
					Name: u.Name,
				})
			}
			inbound.Users = sbUsers
		} else if in.Protocol == "trojan" {
			var sbUsers []SingBoxUser
			for _, u := range users {
				if !u.Enabled || u.InboundID != in.ID {
					continue
				}
				sbUsers = append(sbUsers, SingBoxUser{
					Password: u.UUID,
					Name:     u.Name,
				})
			}
			inbound.Users = sbUsers
		} else if in.Protocol == "shadowsocks" {
			inbound.Method = "aes-256-gcm"
			for _, u := range users {
				if !u.Enabled || u.InboundID != in.ID {
					continue
				}
				inbound.Password = u.UUID
				break
			}
		}

		// TLS/Reality Config
		if in.TLSMode != "none" {
			tlsConfig := &SingBoxInboundTLS{
				Enabled:    true,
				ServerName: in.SNI,
			}

			if in.TLSMode == "reality" {
				host := "127.0.0.1"
				port := 443
				if in.FallbackDest != "" {
					parts := strings.Split(in.FallbackDest, ":")
					if len(parts) == 2 {
						host = parts[0]
						if p, err := strconv.Atoi(parts[1]); err == nil {
							port = p
						}
					} else if len(parts) == 1 {
						host = parts[0]
					}
				}

				shortIDs := []string{}
				if in.RealityShortIDs != "" {
					shortIDs = strings.Split(in.RealityShortIDs, ",")
				}

				tlsConfig.Reality = &SingBoxInboundReality{
					Enabled: true,
					Handshake: SingBoxHandshake{
						Server:     host,
						ServerPort: port,
					},
					PrivateKey: in.RealityPrivateKey,
					ShortID:    shortIDs,
				}
			}
			inbound.TLS = tlsConfig
		}

		// Transport settings
		if in.Network == "ws" {
			path := in.Path
			if path == "" {
				path = "/ws"
			}
			inbound.Transport = &SingBoxTransport{
				Type: "ws",
				Path: path,
			}
		} else if in.Network == "grpc" {
			svc := in.Path
			if svc == "" {
				svc = "TunService"
			}
			inbound.Transport = &SingBoxTransport{
				Type:        "grpc",
				ServiceName: svc,
			}
		}

		config.Inbounds = append(config.Inbounds, inbound)
	}

	// Routing rules
	var sbRules []SingBoxRouteRule
	for _, rule := range rules {
		r := SingBoxRouteRule{
			Outbound: rule.Action,
		}
		if rule.RuleType == "domain" || rule.RuleType == "geosite" {
			r.Geosite = []string{rule.Value}
		} else if rule.RuleType == "ip" || rule.RuleType == "geoip" {
			r.Geoip = []string{rule.Value}
		}
		sbRules = append(sbRules, r)
	}

	config.Route = &SingBoxRoute{
		Rules: sbRules,
	}

	return json.MarshalIndent(config, "", "  ")
}

func CompileSingBoxClientConfig(
	activeConfig models.V2RayClientConfig,
	socksPort int,
	httpPort int,
	evasionEnabled bool,
	tcpDecoySni string,
) ([]byte, error) {
	config := SingBoxConfig{
		Log: &SingBoxLog{
			Level: "warn",
		},
		Inbounds: []SingBoxInbound{
			{
				Type:       "socks",
				Tag:        "socks-in",
				Listen:     "127.0.0.1",
				ListenPort: socksPort,
			},
			{
				Type:       "http",
				Tag:        "http-in",
				Listen:     "127.0.0.1",
				ListenPort: httpPort,
			},
		},
	}

	isAutoBalancer := activeConfig.Protocol == "balancer" || activeConfig.Address == "auto" || strings.Contains(activeConfig.Name, "Auto")

	var balancerOutbounds []string

	if isAutoBalancer && db.DB != nil {
		var allConfigs []models.V2RayClientConfig
		if err := db.DB.Find(&allConfigs).Error; err == nil && len(allConfigs) > 0 {
			for _, cfg := range allConfigs {
				if cfg.Protocol == "balancer" || cfg.Address == "auto" {
					continue
				}
				tag := fmt.Sprintf("proxy-node-%d", cfg.ID)
				outbound := CompileSingBoxOutbound(cfg, evasionEnabled, tag)
				config.Outbounds = append(config.Outbounds, outbound)
				balancerOutbounds = append(balancerOutbounds, tag)
			}
		}
	}

	if len(balancerOutbounds) > 0 {
		config.Outbounds = append(config.Outbounds, SingBoxOutbound{
			Type:      "urltest",
			Tag:       "balancer",
			Outbounds: balancerOutbounds,
			URL:       "http://www.gstatic.com/generate_204",
			Interval:  "10s",
		})
	} else {
		config.Outbounds = append(config.Outbounds, CompileSingBoxOutbound(activeConfig, evasionEnabled, "proxy"))
	}

	config.Outbounds = append(config.Outbounds, SingBoxOutbound{
		Type: "direct",
		Tag:  "direct",
	})
	config.Outbounds = append(config.Outbounds, SingBoxOutbound{
		Type: "block",
		Tag:  "block",
	})

	dohURL := "https://1.1.1.1/dns-query"
	if db.DB != nil {
		var setting models.V2RayClientSetting
		if err := db.DB.Where("key = ?", "dns_doh_url").First(&setting).Error; err == nil && setting.Value != "" {
			dohURL = setting.Value
		}
	}

	detourTarget := "proxy"
	if len(balancerOutbounds) > 0 {
		detourTarget = "balancer"
	}

	config.DNS = &SingBoxDNS{
		Servers: []SingBoxDNSServer{
			{
				Tag:     "dns_doh",
				Address: dohURL,
				Detour:  detourTarget,
			},
			{
				Tag:     "dns_direct",
				Address: "8.8.8.8",
				Detour:  "direct",
			},
			{
				Tag:     "dns_local",
				Address: "local",
				Detour:  "direct",
			},
		},
		Rules: []SingBoxDNSRule{
			{
				Geosite: []string{"geolocation-!ir"},
				Server:  "dns_doh",
			},
			{
				Geosite: []string{"ir"},
				Server:  "dns_direct",
			},
		},
	}

	routingMode := "bypass_domestic"
	if db.DB != nil {
		var setting models.V2RayClientSetting
		if err := db.DB.Where("key = ?", "routing_mode").First(&setting).Error; err == nil && setting.Value != "" {
			routingMode = setting.Value
		}
	}

	var rules []SingBoxRouteRule

	rules = append(rules, SingBoxRouteRule{
		Port:     53,
		Outbound: "direct",
	})

	switch routingMode {
	case "global":
		rules = append(rules, SingBoxRouteRule{
			Geosite:  []string{"private"},
			Outbound: "direct",
		})
		rules = append(rules, SingBoxRouteRule{
			Geoip:    []string{"private"},
			Outbound: "direct",
		})
	case "bypass_domestic":
		rules = append(rules, SingBoxRouteRule{
			Geosite:  []string{"private", "ir"},
			Outbound: "direct",
		})
		rules = append(rules, SingBoxRouteRule{
			Geoip:    []string{"private", "ir"},
			Outbound: "direct",
		})
	case "block_ads":
		rules = append(rules, SingBoxRouteRule{
			Geosite:  []string{"category-ads-all"},
			Outbound: "block",
		})
		rules = append(rules, SingBoxRouteRule{
			Geosite:  []string{"private", "ir"},
			Outbound: "direct",
		})
		rules = append(rules, SingBoxRouteRule{
			Geoip:    []string{"private", "ir"},
			Outbound: "direct",
		})
	}

	rules = append(rules, SingBoxRouteRule{
		Outbound: detourTarget,
	})

	config.Route = &SingBoxRoute{
		Rules: rules,
	}

	return json.MarshalIndent(config, "", "  ")
}

func CompileSingBoxOutbound(activeConfig models.V2RayClientConfig, evasionEnabled bool, tag string) SingBoxOutbound {
	outbound := SingBoxOutbound{
		Type: activeConfig.Protocol,
		Tag:  tag,
	}

	var dbTlsSettings map[string]interface{}
	if activeConfig.TLSSettings != "" {
		_ = json.Unmarshal([]byte(activeConfig.TLSSettings), &dbTlsSettings)
	}

	outbound.Server = activeConfig.Address
	outbound.ServerPort = activeConfig.Port

	switch activeConfig.Protocol {
	case "vless":
		outbound.UUID = activeConfig.UUID
		outbound.Flow = "xtls-rprx-vision"
	case "vmess":
		outbound.UUID = activeConfig.UUID
		outbound.Security = "auto"
	case "trojan":
		outbound.Password = activeConfig.UUID
	case "shadowsocks":
		method := "aes-256-gcm"
		if dbTlsSettings != nil {
			if meth, ok := dbTlsSettings["method"].(string); ok && meth != "" {
				method = meth
			}
		}
		outbound.Method = method
		outbound.Password = activeConfig.UUID
	}

	security := "none"
	if dbTlsSettings != nil {
		if s, ok := dbTlsSettings["security"].(string); ok {
			security = s
		}
	}

	if security == "reality" || security == "tls" {
		tlsConfig := &SingBoxOutboundTLS{
			Enabled: true,
		}
		if dbTlsSettings != nil {
			if sni, ok := dbTlsSettings["sni"].(string); ok {
				tlsConfig.ServerName = sni
			}
		}

		if evasionEnabled {
			evasionFingerprint := "chrome"
			if db.DB != nil {
				var setting models.V2RayClientSetting
				if err := db.DB.Where("key = ?", "evasion_fingerprint").First(&setting).Error; err == nil && setting.Value != "" {
					evasionFingerprint = setting.Value
				}
			}
			tlsConfig.Utls = &SingBoxUtls{
				Enabled:     true,
				Fingerprint: evasionFingerprint,
			}

			// Smart Sing-Box Packet Fragmentation Mapping
			evasionFragment := false
			if db.DB != nil {
				var setting models.V2RayClientSetting
				if err := db.DB.Where("key = ?", "evasion_fragment").First(&setting).Error; err == nil {
					evasionFragment = setting.Value == "true"
				}
			}

			if evasionFragment {
				fragMode := "default"
				fragLength := "100-200"
				fragInterval := "10-20"

				if db.DB != nil {
					var setting models.V2RayClientSetting
					if err := db.DB.Where("key = ?", "fragment_mode").First(&setting).Error; err == nil && setting.Value != "" {
						fragMode = setting.Value
					}
					if err := db.DB.Where("key = ?", "fragment_length").First(&setting).Error; err == nil && setting.Value != "" {
						fragLength = setting.Value
					}
					if err := db.DB.Where("key = ?", "fragment_interval").First(&setting).Error; err == nil && setting.Value != "" {
						fragInterval = setting.Value
					}
				}

				var sizeVal, sleepVal string
				switch fragMode {
				case "domain":
					sizeVal = "1-5"
					sleepVal = "5-15"
				case "random":
					sizeVal = "1-3"
					sleepVal = "1-5"
				default:
					sizeVal = fragLength
					sleepVal = fragInterval
				}

				tlsConfig.Fragment = &SingBoxFragment{
					Enabled: true,
					Size:    sizeVal,
					Sleep:   sleepVal,
				}
			}
		}

		if security == "reality" && dbTlsSettings != nil {
			pubKey, _ := dbTlsSettings["publicKey"].(string)
			shortId, _ := dbTlsSettings["shortId"].(string)
			tlsConfig.Reality = &SingBoxOutReality{
				Enabled:   true,
				PublicKey: pubKey,
				ShortID:   shortId,
			}
		}
		outbound.TLS = tlsConfig
	}

	if activeConfig.Network == "ws" {
		path := "/ws"
		if dbTlsSettings != nil {
			if p, ok := dbTlsSettings["path"].(string); ok && p != "" {
				path = p
			}
		}
		outbound.Transport = &SingBoxTransport{
			Type: "ws",
			Path: path,
		}
	} else if activeConfig.Network == "grpc" {
		svc := "TunService"
		if dbTlsSettings != nil {
			if s, ok := dbTlsSettings["serviceName"].(string); ok && s != "" {
				svc = s
			}
		}
		outbound.Transport = &SingBoxTransport{
			Type:        "grpc",
			ServiceName: svc,
		}
	}

	if activeConfig.MuxEnabled {
		outbound.Multiplex = &SingBoxMultiplex{
			Enabled:    true,
			Protocol:   "smux",
			MaxStreams: 8,
		}
	}

	return outbound
}
