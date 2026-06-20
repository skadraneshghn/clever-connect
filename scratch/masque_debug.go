//go:build ignore

package main

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"net"
	"os"
	"time"

	"clever-connect/internal/config"
	"clever-connect/internal/db"
	"clever-connect/internal/db/pebble"
	"clever-connect/internal/models"
	"clever-connect/internal/warp"
)

func main() {
	fmt.Println("=== WARP MASQUE H2 Debug Tool ===")

	// Load config & DB
	cfg := config.LoadConfig()
	database := db.InitDB(cfg)
	_ = database

	if err := pebble.InitPebble("data/pebble_nodes"); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: pebble init: %v\n", err)
	}
	defer pebble.Close()

	// 1. Load active WARP config
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

	// Validate the account
	fmt.Println("\n[Validation] Validating active account...")
	valResult := warp.ValidateAccount(context.Background(), &account, warpCfg.TargetSNI)
	fmt.Printf("  Success:      %v\n", valResult.Success)
	fmt.Printf("  KeyState:     %s\n", valResult.KeyState)
	fmt.Printf("  ErrorMessage: %s\n", valResult.ErrorMessage)
	if !valResult.Success {
		fmt.Println("[WARN] Account is not valid, attempting test anyway...")
	}

	// Try using account 9 or account 10 (since account 9 is warp_plus and account 10 is the active one)
	// We can let the script try both accounts!
	accounts := []models.WarpAccount{account}
	var acct9 models.WarpAccount
	if err := db.DB.First(&acct9, 9).Error; err == nil {
		accounts = append(accounts, acct9)
	}

	// Try all accounts loaded
	for idx, act := range accounts {
		fmt.Printf("\n--- Testing Account %d (DeviceID: %s, Type: %s) ---\n", idx+1, act.DeviceID, act.AccountType)

		// Formulate credential options
		rawCredentials := fmt.Sprintf("%s:%s", act.DeviceID, act.Token)
		basicAuth := base64.StdEncoding.EncodeToString([]byte(rawCredentials))

		combinations := []struct {
			name        string
			hostHeader  string
			authHeader  string
			capsuleHeader string
		}{
			{
				name:        "Basic Auth + cloudflareaccess.com Host + Capsule: wg",
				hostHeader:  "cloudflareaccess.com",
				authHeader:  "Basic " + basicAuth,
				capsuleHeader: "wg",
			},
			{
				name:        "Bearer Auth + cloudflareaccess.com Host + Capsule: wg",
				hostHeader:  "cloudflareaccess.com",
				authHeader:  "Bearer " + act.Token,
				capsuleHeader: "wg",
			},
			{
				name:        "Basic Auth + cloudflareaccess.com Host + Capsule: ?1",
				hostHeader:  "cloudflareaccess.com",
				authHeader:  "Basic " + basicAuth,
				capsuleHeader: "?1",
			},
			{
				name:        "Bearer Auth + cloudflareaccess.com Host + Capsule: ?1",
				hostHeader:  "cloudflareaccess.com",
				authHeader:  "Bearer " + act.Token,
				capsuleHeader: "?1",
			},
			{
				name:        "Basic Auth + consumer-masque Host + Capsule: wg",
				hostHeader:  warpCfg.TargetSNI,
				authHeader:  "Basic " + basicAuth,
				capsuleHeader: "wg",
			},
			{
				name:        "Basic Auth + api.cloudflareclient.com Host + Capsule: wg",
				hostHeader:  "api.cloudflareclient.com",
				authHeader:  "Basic " + basicAuth,
				capsuleHeader: "wg",
			},
			{
				name:        "Bearer Auth + consumer-masque Host + Capsule: wg",
				hostHeader:  warpCfg.TargetSNI,
				authHeader:  "Bearer " + act.Token,
				capsuleHeader: "wg",
			},
			{
				name:        "Bearer Auth + api.cloudflareclient.com Host + Capsule: wg",
				hostHeader:  "api.cloudflareclient.com",
				authHeader:  "Bearer " + act.Token,
				capsuleHeader: "wg",
			},
			{
				name:        "Basic Auth + consumer-masque Host + Capsule: ?1",
				hostHeader:  warpCfg.TargetSNI,
				authHeader:  "Basic " + basicAuth,
				capsuleHeader: "?1",
			},
			{
				name:        "Bearer Auth + consumer-masque Host + Capsule: ?1",
				hostHeader:  warpCfg.TargetSNI,
				authHeader:  "Bearer " + act.Token,
				capsuleHeader: "?1",
			},
			{
				name:        "Basic Auth + consumer-masque Host + No Capsule",
				hostHeader:  warpCfg.TargetSNI,
				authHeader:  "Basic " + basicAuth,
				capsuleHeader: "",
			},
			{
				name:        "Bearer Auth + consumer-masque Host + No Capsule",
				hostHeader:  warpCfg.TargetSNI,
				authHeader:  "Bearer " + act.Token,
				capsuleHeader: "",
			},
		}

		masqueHost := "162.159.198.2"
		masquePort := 443
		addr := fmt.Sprintf("%s:%d", masqueHost, masquePort)

		for _, comb := range combinations {
			fmt.Printf("\n[Test] Running combination: %s...\n", comb.name)

			conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
			if err != nil {
				fmt.Printf("  TCP Dial failed: %v\n", err)
				continue
			}

			tlsConn := tls.Client(conn, &tls.Config{
				ServerName:         warpCfg.TargetSNI,
				InsecureSkipVerify: true,
			})
			err = tlsConn.Handshake()
			if err != nil {
				fmt.Printf("  TLS Handshake failed: %v\n", err)
				conn.Close()
				continue
			}

			// We will test three target formats:
			// 1. Standard host:port
			// 2. Path-based: /v1/masque/tcp/<host>/<port>/
			// 3. Query-based: /v1/connect/tcp?target=<host>:<port>
			targetFormats := []struct {
				label  string
				target string
			}{
				{label: "Standard Host", target: "connectivity.cloudflareclient.com:443"},
				{label: "Path-based", target: "/v1/masque/tcp/connectivity.cloudflareclient.com/443/"},
				{label: "Query-based", target: "/v1/connect/tcp?target=connectivity.cloudflareclient.com:443"},
			}

			for _, tf := range targetFormats {
				fmt.Printf("    * Testing target format: %s...\n", tf.label)
				var connectReq string
				if comb.capsuleHeader != "" {
					connectReq = fmt.Sprintf(
						"CONNECT %s HTTP/1.1\r\n"+
							"Host: %s\r\n"+
							"Proxy-Authorization: %s\r\n"+
							"Capsule-Protocol: %s\r\n"+
							"\r\n",
						tf.target,
						comb.hostHeader,
						comb.authHeader,
						comb.capsuleHeader,
					)
				} else {
					connectReq = fmt.Sprintf(
						"CONNECT %s HTTP/1.1\r\n"+
							"Host: %s\r\n"+
							"Proxy-Authorization: %s\r\n"+
							"\r\n",
						tf.target,
						comb.hostHeader,
						comb.authHeader,
					)
				}

				conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
				if err != nil {
					fmt.Printf("      TCP Dial failed: %v\n", err)
					continue
				}

				tlsConn := tls.Client(conn, &tls.Config{
					ServerName:         warpCfg.TargetSNI,
					InsecureSkipVerify: true,
				})
				err = tlsConn.Handshake()
				if err != nil {
					fmt.Printf("      TLS Handshake failed: %v\n", err)
					conn.Close()
					continue
				}

				_, err = tlsConn.Write([]byte(connectReq))
				if err != nil {
					fmt.Printf("      Write failed: %v\n", err)
					tlsConn.Close()
					continue
				}

				buf := make([]byte, 1024)
				tlsConn.SetReadDeadline(time.Now().Add(3 * time.Second))
				n, err := tlsConn.Read(buf)
				if err != nil {
					fmt.Printf("      Read failed: %v\n", err)
				} else {
					firstLine := ""
					responseStr := string(buf[:n])
					if idx := 0; idx < len(responseStr) {
						lines := 0
						for i, char := range responseStr {
							if char == '\n' {
								lines++
								if lines == 1 {
									firstLine = responseStr[:i]
									break
								}
							}
						}
					}
					fmt.Printf("      Status: %s\n", firstLine)
					if !contains(firstLine, "400") {
						fmt.Printf("      Full response snippet:\n%s\n", responseStr[:min(128, len(responseStr))])
					}
				}
				tlsConn.Close()
			}
		}
	}
	fmt.Println("\n=== MASQUE Debug complete ===")
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
