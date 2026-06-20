//go:build ignore

package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"clever-connect/internal/config"
	"clever-connect/internal/db"
	"clever-connect/internal/db/pebble"
	"clever-connect/internal/models"
	"clever-connect/internal/warp"
)

func main() {
	os.Setenv("GODEBUG", "http2xconnect=1")
	fmt.Println("=== End-to-End MASQUE H2 Tunnel Test ===")

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

	fmt.Printf("[Config] SNI=%s, DeviceID=%s\n", warpCfg.TargetSNI, account.DeviceID)

	// Use engage.cloudflareclient.com IP
	masqueHost := "162.159.192.1"
	masquePort := 443

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	fmt.Println("Starting MASQUE H2 transport...")
	dialFn, transportCancel, err := warp.StartMASQUEH2Transport(ctx, &warpCfg, &account, masqueHost, masquePort)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: failed to start transport: %v\n", err)
		os.Exit(1)
	}
	defer transportCancel()

	fmt.Println("Transport started successfully! Dialing internet target via tunnel...")
	conn, err := dialFn(ctx, "tcp", "1.1.1.1:80")
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: failed to dial target: %v\n", err)
		os.Exit(1)
	}
	defer conn.Close()

	fmt.Println("Dial succeeded! Sending HTTP request...")
	_, err = conn.Write([]byte("GET /cdn-cgi/trace HTTP/1.1\r\nHost: 1.1.1.1\r\nUser-Agent: curl/7.68.0\r\nAccept: */*\r\n\r\n"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: failed to write to connection: %v\n", err)
		os.Exit(1)
	}

	buf := make([]byte, 2048)
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	n, err := conn.Read(buf)
	if err != nil && err != io.EOF {
		fmt.Fprintf(os.Stderr, "ERROR: failed to read from connection: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("=== Response from Cloudflare cdn-cgi/trace ===")
	fmt.Println(string(buf[:n]))
	fmt.Println("==============================================")
}
