package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"clever-connect/internal/v2ray/sub"
	"clever-connect/internal/v2ray/tester"
)

func main() {
	link := "vless://931729a8-3c20-4841-89a1-f18dc9ce0a6f@77.74.123.163:8443?encryption=none&security=tls&sni=cdn3-87.vk-cdnvideo.com&fp=chrome&insecure=0&allowInsecure=0&type=tcp&headerType=none#%F0%9F%AA%AD9%40oneclickvpnkeys%2Bxray%3A9921KB%2Fs"

	cfg, err := sub.ParseProxyLink(link)
	if err != nil {
		log.Fatalf("failed to parse proxy link: %v", err)
	}

	fmt.Printf("Parsed Config:\nName: %s\nProtocol: %s\nAddress: %s\nPort: %d\nUUID: %s\nNetwork: %s\nTLSSettings: %s\n\n",
		cfg.Name, cfg.Protocol, cfg.Address, cfg.Port, cfg.UUID, cfg.Network, cfg.TLSSettings)

	opts := tester.TestOptions{
		TestType: "real_url",
		Timeout:  10 * time.Second,
		URL:      "http://www.gstatic.com/generate_204",
		Core:     "xray",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	fmt.Println("Running TestSingleConfig...")
	res := tester.TestSingleConfig(ctx, cfg, opts)
	fmt.Printf("Result:\nOK: %v\nPingMs: %d\nRelayMs: %d\nHTTPStatus: %d\nColo: %s\nError: %s\n",
		res.OK, res.PingMs, res.RelayMs, res.HTTPStatus, res.Colo, res.Error)
}
