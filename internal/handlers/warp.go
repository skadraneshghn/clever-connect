package handlers

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"runtime"
	"strconv"

	"clever-connect/internal/config"
	"clever-connect/internal/db"
	"clever-connect/internal/db/pebble"
	"clever-connect/internal/logger"
	"clever-connect/internal/models"
	"clever-connect/internal/warp"

	"github.com/gin-gonic/gin"
)

// warpLicenseKeyRE matches the WARP+ key format: three groups of 8 alphanumeric
// characters separated by dashes, e.g. XXXXXXXX-XXXXXXXX-XXXXXXXX (26 chars total).
var warpLicenseKeyRE = regexp.MustCompile(`^[A-Za-z0-9]{8}-[A-Za-z0-9]{8}-[A-Za-z0-9]{8}$`)

// ──────────────────────────────────────────────────────────────────────────────
// Cloudflare WARP+ Engine REST API Handler (Pillar 6)
//
// Registers endpoints within the Gin engine, exposing administrative actions,
// port modifications, account pool additions, scan triggers, and real-time
// engine telemetry to the authenticated client dashboard.
// ──────────────────────────────────────────────────────────────────────────────

// WarpHandler handles all WARP+ Engine API requests.
type WarpHandler struct {
	cfg *config.Config
}

// NewWarpHandler creates a new WARP handler.
func NewWarpHandler(cfg *config.Config) *WarpHandler {
	return &WarpHandler{cfg: cfg}
}

// ──────────────────────────────────────────────────────────────────────────────
// GET /api/v2ray/warp/config — Returns WarpGlobalConfig + Active Account
// ──────────────────────────────────────────────────────────────────────────────

func (h *WarpHandler) GetConfig(c *gin.Context) {
	var cfg models.WarpGlobalConfig
	if err := db.DB.First(&cfg).Error; err != nil {
		// Return defaults if no config exists
		cfg = models.WarpGlobalConfig{
			TransportMode: "masque",
			TargetSNI:     "consumer-masque.cloudflareclient.com",
			SocksPort:     10880,
			HTTPPort:      10881,
		}
	}

	// Build response with active account info
	response := gin.H{
		"transport_mode": cfg.TransportMode,
		"target_sni":     cfg.TargetSNI,
		"socks_port":     cfg.SocksPort,
		"http_port":      cfg.HTTPPort,
		"is_active":      cfg.IsActive,
		"last_trace_ok":  cfg.LastTraceOK,
		"updated_at":     cfg.UpdatedAt,
	}

	// Include active account summary if set
	if cfg.ActiveAccountID > 0 {
		var account models.WarpAccount
		if err := db.DB.First(&account, cfg.ActiveAccountID).Error; err == nil {
			response["active_account"] = gin.H{
				"id":           account.ID,
				"account_type": account.AccountType,
				"total_quota":  account.TotalQuota,
				"used_quota":   account.UsedQuota,
				"is_functional": account.IsFunctional,
				"device_id":    account.DeviceID,
			}
		}
	}

	c.JSON(http.StatusOK, response)
}

// ──────────────────────────────────────────────────────────────────────────────
// POST /api/v2ray/warp/config — Updates Ports, Transport Mode, SNI
// ──────────────────────────────────────────────────────────────────────────────

func (h *WarpHandler) SaveConfig(c *gin.Context) {
	var input struct {
		TransportMode string `json:"transport_mode"`
		TargetSNI     string `json:"target_sni"`
		SocksPort     int    `json:"socks_port"`
		HTTPPort      int    `json:"http_port"`
	}

	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Validate transport mode
	if input.TransportMode != "" && input.TransportMode != "masque" && input.TransportMode != "wireguard" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid transport mode. Must be 'masque' or 'wireguard'."})
		return
	}

	// Validate port ranges
	if input.SocksPort > 0 && (input.SocksPort < 1024 || input.SocksPort > 65535) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "SOCKS port must be between 1024 and 65535."})
		return
	}
	if input.HTTPPort > 0 && (input.HTTPPort < 1024 || input.HTTPPort > 65535) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "HTTP port must be between 1024 and 65535."})
		return
	}

	// Validate ports don't conflict
	if input.SocksPort > 0 && input.HTTPPort > 0 && input.SocksPort == input.HTTPPort {
		c.JSON(http.StatusBadRequest, gin.H{"error": "SOCKS and HTTP ports cannot be the same."})
		return
	}

	// Upsert configuration
	var existing models.WarpGlobalConfig
	if err := db.DB.First(&existing).Error; err != nil {
		// Create new
		existing = models.WarpGlobalConfig{
			TransportMode: "masque",
			TargetSNI:     "consumer-masque.cloudflareclient.com",
			SocksPort:     10880,
			HTTPPort:      10881,
		}
	}

	if input.TransportMode != "" {
		existing.TransportMode = input.TransportMode
	}
	if input.TargetSNI != "" {
		existing.TargetSNI = input.TargetSNI
	}
	if input.SocksPort > 0 {
		existing.SocksPort = input.SocksPort
	}
	if input.HTTPPort > 0 {
		existing.HTTPPort = input.HTTPPort
	}

	if err := db.DB.Save(&existing).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// If engine is running, restart with new config
	engine := warp.GetEngine()
	if engine.IsRunning() {
		logger.Info("WARP", "Configuration changed while engine running — restarting")
		_ = engine.StopEngine()

		if existing.ActiveAccountID > 0 {
			var account models.WarpAccount
			if err := db.DB.First(&account, existing.ActiveAccountID).Error; err == nil {
				go func() {
					if err := engine.StartEngine(&existing, &account); err != nil {
						logger.Error("WARP", "Failed to restart engine after config change", "error", err)
					}
				}()
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{"message": "Global WARP parameters applied successfully."})
}

// ──────────────────────────────────────────────────────────────────────────────
// GET /api/v2ray/warp/accounts — Lists Fleet Pool
// ──────────────────────────────────────────────────────────────────────────────

func (h *WarpHandler) ListAccounts(c *gin.Context) {
	var accounts []models.WarpAccount
	if err := db.DB.Find(&accounts).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Mask sensitive fields
	for i := range accounts {
		if len(accounts[i].Token) > 20 {
			accounts[i].Token = accounts[i].Token[:10] + "..." + accounts[i].Token[len(accounts[i].Token)-10:]
		}
		if len(accounts[i].PrivateKey) > 10 {
			accounts[i].PrivateKey = accounts[i].PrivateKey[:5] + "..."
		}
	}

	c.JSON(http.StatusOK, accounts)
}

// ──────────────────────────────────────────────────────────────────────────────
// POST /api/v2ray/warp/accounts — Register New Account via uTLS Client
// ──────────────────────────────────────────────────────────────────────────────

func (h *WarpHandler) AddAccount(c *gin.Context) {
	var input struct {
		LicenseKey string `json:"license_key"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Validate license key format if provided.
	// WARP+ keys look like: XXXXXXXX-XXXXXXXX-XXXXXXXX (3×8 alphanumeric, 26 chars).
	if input.LicenseKey != "" && !warpLicenseKeyRE.MatchString(input.LicenseKey) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid license key format. Expected XXXXXXXX-XXXXXXXX-XXXXXXXX (three groups of 8 alphanumeric characters)."})
		return
	}

	// Load current config for SNI
	var cfg models.WarpGlobalConfig
	if err := db.DB.First(&cfg).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No WARP configuration found. Please save configuration first."})
		return
	}

	// Create obfuscated client and register
	client := warp.NewObfuscatedClient(cfg.TargetSNI)

	account, err := client.RegisterDevice(input.LicenseKey)
	if err != nil {
		logger.Error("WARP", "Account registration failed", "error", err)
		c.JSON(http.StatusBadGateway, gin.H{"error": fmt.Sprintf("Cloudflare API rejected registration: %v", err)})
		return
	}

	// Save to database
	if err := db.DB.Create(account).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// If this is the first account, set it as active
	var accountCount int64
	db.DB.Model(&models.WarpAccount{}).Count(&accountCount)
	if accountCount == 1 {
		cfg.ActiveAccountID = account.ID
		db.DB.Save(&cfg)
	}

	logger.Info("WARP", "Account registered successfully",
		"deviceID", account.DeviceID,
		"type", account.AccountType,
	)

	c.JSON(http.StatusCreated, account)
}

// ──────────────────────────────────────────────────────────────────────────────
// DELETE /api/v2ray/warp/accounts/:id — Remove Account
// ──────────────────────────────────────────────────────────────────────────────

func (h *WarpHandler) DeleteAccount(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid account ID."})
		return
	}

	// Check if this is the active account
	var cfg models.WarpGlobalConfig
	if err := db.DB.First(&cfg).Error; err == nil && cfg.ActiveAccountID == uint(id) {
		// Stop engine if running with this account
		engine := warp.GetEngine()
		if engine.IsRunning() {
			_ = engine.StopEngine()
			cfg.IsActive = false
		}
		cfg.ActiveAccountID = 0
		db.DB.Save(&cfg)
	}

	if err := db.DB.Delete(&models.WarpAccount{}, id).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "deleted"})
}

// ──────────────────────────────────────────────────────────────────────────────
// POST /api/v2ray/warp/accounts/:id/activate — Set Account as Active
// ──────────────────────────────────────────────────────────────────────────────

func (h *WarpHandler) SetActiveAccount(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid account ID."})
		return
	}

	// Verify account exists
	var account models.WarpAccount
	if err := db.DB.First(&account, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Account not found."})
		return
	}
	if !account.IsFunctional {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Cannot activate a non-functional account."})
		return
	}

	// Load or create the global config
	var cfg models.WarpGlobalConfig
	if err := db.DB.First(&cfg).Error; err != nil {
		// No config yet — create a minimal one
		cfg = models.WarpGlobalConfig{
			TransportMode: "masque",
			TargetSNI:     "consumer-masque.cloudflareclient.com",
			SocksPort:     10880,
			HTTPPort:      10881,
		}
	}

	// If engine is running with a different account, stop it first
	engine := warp.GetEngine()
	if engine.IsRunning() && cfg.ActiveAccountID != uint(id) {
		logger.Info("WARP", "Stopping engine to switch active account",
			"from", cfg.ActiveAccountID, "to", id)
		_ = engine.StopEngine()
		cfg.IsActive = false
	}

	cfg.ActiveAccountID = uint(id)
	if err := db.DB.Save(&cfg).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	logger.Info("WARP", "Active account changed", "account_id", id, "type", account.AccountType)

	c.JSON(http.StatusOK, gin.H{
		"status":     "activated",
		"account_id": id,
		"account_type": account.AccountType,
	})
}

// ──────────────────────────────────────────────────────────────────────────────
// POST /api/v2ray/warp/scan — Trigger Manual Endpoint Scan
// ──────────────────────────────────────────────────────────────────────────────

func (h *WarpHandler) StartScan(c *gin.Context) {
	var cfg models.WarpGlobalConfig
	if err := db.DB.First(&cfg).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No WARP configuration found."})
		return
	}

	// Accept optional JSON body with scan parameters
	var input struct {
		Workers   int `json:"workers"`
		TimeoutMs int `json:"timeout_ms"`
	}
	_ = c.ShouldBindJSON(&input) // OK if body is absent

	// Also honour legacy ?workers= query param
	if input.Workers == 0 {
		if w := c.Query("workers"); w != "" {
			if parsed, err := strconv.Atoi(w); err == nil {
				input.Workers = parsed
			}
		}
	}

	if input.Workers <= 0 || input.Workers > 500 {
		input.Workers = runtime.NumCPU() * 4
	}
	if input.TimeoutMs <= 0 || input.TimeoutMs > 10000 {
		input.TimeoutMs = 2000
	}

	// Stop any existing scan first
	if existing := warp.GetScanner(); existing != nil && existing.IsRunning() {
		existing.StopScan()
	}

	// Clear existing scan results
	_ = pebble.DeleteWarpScanResults()

	// Create scanner, register as singleton, and start
	scanner := warp.NewWarpScanner(&cfg, input.Workers, input.TimeoutMs)
	warp.SetScanner(scanner)

	if err := scanner.StartScan(context.Background()); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":    "scan_started",
		"message":   "WARP endpoint scan initiated.",
		"workers":   input.Workers,
		"timeout_ms": input.TimeoutMs,
	})
}

// ──────────────────────────────────────────────────────────────────────────────
// POST /api/v2ray/warp/scan/stop — Cancel a Running Scan
// ──────────────────────────────────────────────────────────────────────────────

func (h *WarpHandler) StopScan(c *gin.Context) {
	scanner := warp.GetScanner()
	if scanner == nil || !scanner.IsRunning() {
		c.JSON(http.StatusOK, gin.H{"status": "not_running", "message": "No scan is currently active."})
		return
	}
	scanner.StopScan()
	c.JSON(http.StatusOK, gin.H{"status": "stopped", "message": "Scan cancelled successfully."})
}

// ──────────────────────────────────────────────────────────────────────────────
// GET /api/v2ray/warp/scan/events — Cursor-Based Real-Time Log Polling
// ──────────────────────────────────────────────────────────────────────────────

func (h *WarpHandler) GetScanEvents(c *gin.Context) {
	scanner := warp.GetScanner()
	if scanner == nil {
		c.JSON(http.StatusOK, gin.H{"events": []interface{}{}, "last_index": 0})
		return
	}

	since, _ := strconv.ParseInt(c.DefaultQuery("since", "0"), 10, 64)
	events, lastIdx := scanner.GetEvents(since)

	c.JSON(http.StatusOK, gin.H{
		"events":     events,
		"last_index": lastIdx,
		"count":      len(events),
	})
}

// ──────────────────────────────────────────────────────────────────────────────
// GET /api/v2ray/warp/scan/results — Get Scan Results from PebbleDB
// ──────────────────────────────────────────────────────────────────────────────

func (h *WarpHandler) GetScanResults(c *gin.Context) {
	mode := c.DefaultQuery("mode", "masque")

	results, err := pebble.ListWarpScanResults(mode)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Include scan progress from global singleton if running
	var progress *warp.ScanProgress
	if scanner := warp.GetScanner(); scanner != nil && scanner.IsRunning() {
		p := scanner.GetProgress()
		progress = &p
	}

	c.JSON(http.StatusOK, gin.H{
		"results":  results,
		"total":    len(results),
		"progress": progress,
	})
}


// ──────────────────────────────────────────────────────────────────────────────
// POST /api/v2ray/warp/tunnel/start — Start Engine
// ──────────────────────────────────────────────────────────────────────────────

func (h *WarpHandler) StartTunnel(c *gin.Context) {
	var cfg models.WarpGlobalConfig
	if err := db.DB.First(&cfg).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No WARP configuration found."})
		return
	}

	if cfg.ActiveAccountID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No active account selected. Register an account first."})
		return
	}

	var account models.WarpAccount
	if err := db.DB.First(&account, cfg.ActiveAccountID).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Active account not found in database."})
		return
	}

	if !account.IsFunctional {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Active account is not functional. Register a new account or check license."})
		return
	}

	engine := warp.GetEngine()

	if engine.IsRunning() {
		// Stop existing engine first
		_ = engine.StopEngine()
	}

	if err := engine.StartEngine(&cfg, &account); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Mark as active in DB
	cfg.IsActive = true
	db.DB.Save(&cfg)

	// Run initial trace check in background
	go func() {
		if err := warp.AutoRotateOnFailure(&cfg); err != nil {
			logger.Warn("WARP", "Initial trace check failed", "error", err)
		}
	}()

	c.JSON(http.StatusOK, gin.H{
		"status":  "processed",
		"message": "Core context transition complete.",
	})
}

// ──────────────────────────────────────────────────────────────────────────────
// POST /api/v2ray/warp/tunnel/stop — Stop Engine
// ──────────────────────────────────────────────────────────────────────────────

func (h *WarpHandler) StopTunnel(c *gin.Context) {
	engine := warp.GetEngine()

	if err := engine.StopEngine(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Mark as inactive in DB
	var cfg models.WarpGlobalConfig
	if err := db.DB.First(&cfg).Error; err == nil {
		cfg.IsActive = false
		db.DB.Save(&cfg)
	}

	c.JSON(http.StatusOK, gin.H{
		"status":  "processed",
		"message": "Core context transition complete.",
	})
}

// ──────────────────────────────────────────────────────────────────────────────
// GET /api/v2ray/warp/status — Engine Status + Trace Result
// ──────────────────────────────────────────────────────────────────────────────

func (h *WarpHandler) GetStatus(c *gin.Context) {
	engine := warp.GetEngine()
	status := engine.GetStatus()

	// Include scan progress from global singleton if running
	var scanProgress *warp.ScanProgress
	if scanner := warp.GetScanner(); scanner != nil && scanner.IsRunning() {
		p := scanner.GetProgress()
		scanProgress = &p
	}

	// Count available endpoints
	masqueEndpoints, _ := pebble.ListWarpScanResults("masque")
	wgEndpoints, _ := pebble.ListWarpScanResults("wireguard")

	c.JSON(http.StatusOK, gin.H{
		"engine":           status,
		"scan_progress":    scanProgress,
		"masque_endpoints": len(masqueEndpoints),
		"wg_endpoints":     len(wgEndpoints),
	})
}

