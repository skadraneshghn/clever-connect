package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"clever-connect/internal/db"
	sqlite "clever-connect/internal/db/sqlite"
	"clever-connect/internal/v2ray/compiler"
	"clever-connect/internal/v2ray/sub"
	"clever-connect/internal/v2ray/tester"
	"gorm.io/gorm"
)

func cleanConfigForTesting(configJSON []byte, coreName string) ([]byte, error) {
	var m map[string]interface{}
	if err := json.Unmarshal(configJSON, &m); err != nil {
		return nil, err
	}

	if coreName == "sing-box" {
		delete(m, "dns")
		detourTarget := "proxy"
		if outbounds, ok := m["outbounds"].([]interface{}); ok {
			for _, out := range outbounds {
				if outMap, ok := out.(map[string]interface{}); ok {
					if tag, ok := outMap["tag"].(string); ok && tag == "balancer" {
						detourTarget = "balancer"
						break
					}
				}
			}
		}
		m["route"] = map[string]interface{}{
			"rules": []map[string]interface{}{
				{
					"outbound": detourTarget,
				},
			},
		}
	} else {
		delete(m, "dns")
		defaultOutboundTag := "proxy"
		isBalancer := false
		if routing, ok := m["routing"].(map[string]interface{}); ok {
			if balancers, ok := routing["balancers"].([]interface{}); ok && len(balancers) > 0 {
				defaultOutboundTag = "balancer"
				isBalancer = true
			}
		}
		rule := map[string]interface{}{
			"type":    "field",
			"network": "tcp,udp",
		}
		if isBalancer {
			rule["balancerTag"] = defaultOutboundTag
		} else {
			rule["outboundTag"] = defaultOutboundTag
		}
		m["routing"] = map[string]interface{}{
			"domainStrategy": "AsIs",
			"rules": []map[string]interface{}{
				{
					"type":        "field",
					"inboundTag":  []string{"api"},
					"outboundTag": "api",
				},
				rule,
			},
		}
	}
	return json.MarshalIndent(m, "", "  ")
}

func main() {
	gormDb, err := gorm.Open(sqlite.Open("data/client.db"), &gorm.Config{})
	if err != nil {
		log.Fatalf("failed to connect: %v", err)
	}
	db.DB = gormDb

	link := "vless://931729a8-3c20-4841-89a1-f18dc9ce0a6f@77.74.123.163:8443?encryption=none&security=tls&sni=cdn3-87.vk-cdnvideo.com&fp=chrome&insecure=0&allowInsecure=0&type=tcp&headerType=none#%F0%9F%AA%AD9%40oneclickvpnkeys%2Bxray%3A9921KB%2Fs"

	cfg, err := sub.ParseProxyLink(link)
	if err != nil {
		log.Fatalf("failed to parse proxy link: %v", err)
	}

	socksPort := 12345
	httpPort := 12346
	configBytes, err := compiler.CompileClientConfigForCore("xray", cfg, socksPort, httpPort, true, "")
	if err != nil {
		log.Fatalf("failed to compile config: %v", err)
	}

	cleanBytes, err := cleanConfigForTesting(configBytes, "xray")
	if err != nil {
		log.Fatalf("failed to clean config: %v", err)
	}

	fmt.Println("--- Cleaned Config JSON ---")
	fmt.Println(string(cleanBytes))
	fmt.Println("----------------------------")

	opts := tester.TestOptions{
		TestType: "real_url",
		Timeout:  5 * time.Second,
		URL:      "http://clients3.google.com/generate_204",
		Core:     "current",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	res := tester.TestSingleConfig(ctx, cfg, opts)
	fmt.Printf("Test Result: OK=%t Ping=%d Relay=%d Error=%q Colo=%s\n", res.OK, res.PingMs, res.RelayMs, res.Error, res.Colo)
}
