//go:build ignore

package main

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"time"

	"clever-connect/internal/config"
	"clever-connect/internal/db"
	"clever-connect/internal/db/pebble"
	"clever-connect/internal/models"
	"clever-connect/internal/warp"
)

func main() {
	fmt.Println("=== WARP WireGuard Debug Tool ===")

	// Load config & DB
	cfg := config.LoadConfig()
	database := db.InitDB(cfg)
	_ = database

	if err := pebble.InitPebble("data/pebble_nodes"); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: pebble init: %v\n", err)
	}
	defer pebble.Close()

	// ── 1. Load active WARP account ──────────────────────────────────────────
	var warpCfg models.WarpGlobalConfig
	if err := db.DB.First(&warpCfg).Error; err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: no WarpGlobalConfig: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("\n[Config]\n")
	fmt.Printf("  IsActive:      %v\n", warpCfg.IsActive)
	fmt.Printf("  TransportMode: %s\n", warpCfg.TransportMode)
	fmt.Printf("  SocksPort:     %d\n", warpCfg.SocksPort)
	fmt.Printf("  HTTPPort:      %d\n", warpCfg.HTTPPort)
	fmt.Printf("  TargetSNI:     %s\n", warpCfg.TargetSNI)
	fmt.Printf("  ActiveAcctID:  %d\n", warpCfg.ActiveAccountID)

	var account models.WarpAccount
	if err := db.DB.First(&account, warpCfg.ActiveAccountID).Error; err != nil {
		// Fall back to any account
		if err2 := db.DB.First(&account).Error; err2 != nil {
			fmt.Fprintf(os.Stderr, "ERROR: no WarpAccount: %v\n", err2)
			os.Exit(1)
		}
		fmt.Println("[WARN] ActiveAccountID not found, using first account")
	}

	fmt.Printf("\n[Account]\n")
	fmt.Printf("  DeviceID:     %s\n", account.DeviceID)
	fmt.Printf("  AccountType:  %s\n", account.AccountType)
	fmt.Printf("  AssignedIPv4: %s\n", account.AssignedIPv4)
	fmt.Printf("  AssignedIPv6: %s\n", account.AssignedIPv6)
	fmt.Printf("  ClientID:     %s\n", account.ClientID)
	fmt.Printf("  PrivateKey:   %s...\n", account.PrivateKey[:8])
	fmt.Printf("  PeerPublicKey:%s\n", account.PeerPublicKey)

	// ── 2. Get ranked endpoints ───────────────────────────────────────────────
	candidates, err := pebble.GetRankedEndpoints("wireguard")
	if err != nil || len(candidates) == 0 {
		fmt.Printf("\nERROR: no wireguard endpoints in pebble: %v\n", err)
		fmt.Println("You need to run a WARP scan first via the UI.")
		os.Exit(1)
	}

	fmt.Printf("\n[Endpoints] found %d candidate(s)\n", len(candidates))
	for i, ep := range candidates {
		if i >= 5 {
			break
		}
		fmt.Printf("  [%d] %s:%d  latency=%.0fms  score=%.1f  fails=%d  restricted=%v\n",
			i+1, ep.IPAddress, ep.Port, ep.LatencyMs, ep.Score, ep.FailCount, ep.IsRestricted)
	}

	ep := candidates[0]
	host := ep.IPAddress
	if h, _, err2 := net.SplitHostPort(ep.IPAddress); err2 == nil {
		host = h
	}
	// WireGuard always uses UDP port 2408 — ignore the scanned TCP port
	port := 2408

	fmt.Printf("\n[WireGuard] Starting tunnel to %s:%d ...\n", host, port)

	// ── OVERRIDE: Use account 9 which has complete keys and is warp_plus ─────
	wgcfAccount := &models.WarpAccount{
		DeviceID:      "204b5162-ee6f-49a7-81d0-9cdff126123c",
		Token:         "a8c574f9-0501-46d4-b1cf-083e1dea974e",
		PrivateKey:    "sGPZS8/AVGikkrdJ8XcCSkaxRs1fUkgWuOavfhQA7Gg=",
		PeerPublicKey: "bmXOC+F1FxEMF9dyiK2H5/1SUtzH0JuVo51h2wPfgyo=",
		ClientID:      "7c/O",
		AssignedIPv4:  "172.16.0.2",
		AssignedIPv6:  "2606:4700:110:820e:5649:da4:fc:23d6",
		AccountType:   "warp_plus",
	}
	fmt.Printf("\n[Override] Using account9 %s (client_id=%s, type=%s)\n", wgcfAccount.DeviceID, wgcfAccount.ClientID, wgcfAccount.AccountType)

	// Use the official Cloudflare WARP endpoint (engage.cloudflareclient.com)
	wgHost := "162.159.192.1"
	wgPort := 2408

	fmt.Printf("[WireGuard] Starting tunnel to %s:%d with wgcf keys...\n", wgHost, wgPort)

	// ── 3. Start WireGuard tunnel ─────────────────────────────────────────────
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	dialFn, wgCancel, err := warp.StartWireGuardUserspace(ctx, wgcfAccount, wgHost, wgPort)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: WireGuard start failed: %v\n", err)
		os.Exit(1)
	}
	defer wgCancel()

	// ── 4. Test connectivity ──────────────────────────────────────────────────
	fmt.Printf("\n[Test 1] TCP connect to 1.1.1.1:443 through tunnel...\n")
	t1 := time.Now()
	conn, err := dialFn(context.Background(), "tcp", "1.1.1.1:443")
	if err != nil {
		fmt.Printf("  FAIL: %v\n", err)
	} else {
		conn.Close()
		fmt.Printf("  OK (%.0fms)\n", float64(time.Since(t1).Milliseconds()))
	}

	fmt.Printf("\n[Test 2] HTTP GET https://cloudflare.com/cdn-cgi/trace through tunnel...\n")
	transport := &http.Transport{
		DialContext: dialFn,
	}
	client := &http.Client{Transport: transport, Timeout: 15 * time.Second}
	resp, err := client.Get("https://cloudflare.com/cdn-cgi/trace")
	if err != nil {
		fmt.Printf("  FAIL: %v\n", err)
	} else {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		fmt.Printf("  OK (HTTP %d)\n", resp.StatusCode)
		fmt.Printf("  Body:\n%s\n", string(body))
	}

	fmt.Printf("\n[Test 3] DNS resolution of google.com through tunnel (via 1.1.1.1)...\n")
	dnsConn, err := dialFn(context.Background(), "udp", "1.1.1.1:53")
	if err != nil {
		fmt.Printf("  FAIL UDP to 1.1.1.1:53: %v\n", err)
	} else {
		dnsConn.Close()
		fmt.Printf("  OK UDP dial to DNS\n")
	}

	fmt.Println("\n=== Debug complete ===")
}
