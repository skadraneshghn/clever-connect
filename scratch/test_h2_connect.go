package main

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"

	"clever-connect/internal/config"
	"clever-connect/internal/db"
	"clever-connect/internal/db/pebble"
	"clever-connect/internal/models"
	"golang.org/x/net/http2"
)

func main() {
	fmt.Println("=== Testing MASQUE CONNECT-IP over HTTP/2 ===")

	// Load config & DB
	cfg := config.LoadConfig()
	database := db.InitDB(cfg)
	_ = database

	if err := pebble.InitPebble("data/pebble_nodes"); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: pebble init: %v\n", err)
	}
	defer pebble.Close()

	var warpCfg models.WarpGlobalConfig
	if err := db.DB.First(&warpCfg).Error; err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: no WarpGlobalConfig: %v\n", err)
		os.Exit(1)
	}

	var account models.WarpAccount
	if err := db.DB.First(&account, warpCfg.ActiveAccountID).Error; err != nil {
		if err2 := db.DB.First(&account).Error; err2 != nil {
			fmt.Fprintf(os.Stderr, "ERROR: no WarpAccount: %v\n", err2)
			os.Exit(1)
		}
	}

	// We'll try to connect to the WARP IP
	masqueHost := "162.159.192.1" // engage.cloudflareclient.com
	masquePort := 443
	addr := fmt.Sprintf("%s:%d", masqueHost, masquePort)

	// Auth token
	rawCredentials := fmt.Sprintf("%s:%s", account.DeviceID, account.Token)
	basicAuth := base64.StdEncoding.EncodeToString([]byte(rawCredentials))

	fmt.Printf("[Config] IP=%s, SNI=%s, DeviceID=%s\n", addr, warpCfg.TargetSNI, account.DeviceID)

	// TLS Config
	tlsConfig := &tls.Config{
		ServerName:         warpCfg.TargetSNI,
		NextProtos:         []string{"h2"},
		InsecureSkipVerify: true,
	}

	// Setup custom HTTP/2 client
	transport := &http2.Transport{
		DialTLSContext: func(ctx context.Context, network, _ string, _ *tls.Config) (net.Conn, error) {
			dialer := &net.Dialer{}
			conn, err := dialer.DialContext(ctx, network, addr)
			if err != nil {
				return nil, err
			}
			tlsConn := tls.Client(conn, tlsConfig)
			if err := tlsConn.HandshakeContext(ctx); err != nil {
				conn.Close()
				return nil, err
			}
			return tlsConn, nil
		},
	}

	client := &http.Client{Transport: transport}

	// Create request
	pr, pw := io.Pipe()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodConnect, "https://"+warpCfg.TargetSNI, pr)
	if err != nil {
		fmt.Printf("Failed to create request: %v\n", err)
		return
	}
	req.Host = warpCfg.TargetSNI
	req.ContentLength = -1
	req.Header = http.Header{
		"Proxy-Authorization": {"Basic " + basicAuth},
		"cf-connect-proto":    {"cf-connect-ip"},
		"User-Agent":          {""},
	}

	fmt.Println("Sending HTTP/2 CONNECT request...")
	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("Request failed: %v\n", err)
		return
	}
	defer resp.Body.Close()

	fmt.Printf("Response Status: %s\n", resp.Status)
	fmt.Printf("Response Headers:\n")
	for k, v := range resp.Header {
		fmt.Printf("  %s: %v\n", k, v)
	}

	if resp.StatusCode == http.StatusOK {
		fmt.Println("SUCCESS! CONNECT-IP tunnel established over HTTP/2!")
		// Close pipe to end request body
		pw.Close()
	} else {
		body := make([]byte, 1024)
		n, _ := resp.Body.Read(body)
		fmt.Printf("Response Body: %s\n", string(body[:n]))
		pw.Close()
	}
}
