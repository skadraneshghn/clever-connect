package handlers

import (
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"time"

	"clever-connect/internal/bonding/combiner"
	"clever-connect/internal/config"
	"clever-connect/internal/db"
	"clever-connect/internal/logger"
	"clever-connect/internal/models"

	"github.com/gin-gonic/gin"
)

// CombinerHandler handles server-side DMB combiner API requests.
type CombinerHandler struct {
	cfg      *config.Config
	combiner *combiner.Combiner
}

// NewCombinerHandler creates a new combiner handler.
func NewCombinerHandler(cfg *config.Config) *CombinerHandler {
	return &CombinerHandler{cfg: cfg}
}

// StartCombiner initializes and starts the server combiner.
func (h *CombinerHandler) StartCombiner(c *gin.Context) {
	if h.combiner != nil && h.combiner.IsRunning() {
		c.JSON(http.StatusOK, gin.H{"status": "already running"})
		return
	}

	var cfg models.BondingEngineConfig
	if err := db.DB.First(&cfg).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no bonding configuration found"})
		return
	}

	h.combiner = combiner.NewCombiner(cfg.OriginID, cfg.PSKHex)
	h.combiner.Start()

	cfg.IsActive = true
	db.DB.Save(&cfg)

	c.JSON(http.StatusOK, gin.H{"status": "started", "origin_id": cfg.OriginID})
}

// StopCombiner stops the server combiner.
func (h *CombinerHandler) StopCombiner(c *gin.Context) {
	if h.combiner == nil || !h.combiner.IsRunning() {
		c.JSON(http.StatusOK, gin.H{"status": "already stopped"})
		return
	}

	h.combiner.Stop()

	var cfg models.BondingEngineConfig
	if err := db.DB.First(&cfg).Error; err == nil {
		cfg.IsActive = false
		db.DB.Save(&cfg)
	}

	c.JSON(http.StatusOK, gin.H{"status": "stopped"})
}

// GetCombinerStatus returns combiner statistics.
func (h *CombinerHandler) GetCombinerStatus(c *gin.Context) {
	if h.combiner == nil {
		c.JSON(http.StatusOK, combiner.CombinerStats{Running: false})
		return
	}
	c.JSON(http.StatusOK, h.combiner.Stats())
}

// ServeCombinerWS handles the artery WebSocket connections.
// This is the endpoint clients connect to: /ws/bonding/combiner
func (h *CombinerHandler) ServeCombinerWS(c *gin.Context) {
	if h.combiner == nil || !h.combiner.IsRunning() {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "combiner not running"})
		return
	}
	h.combiner.HandleWebSocket(c.Writer, c.Request)
}

// GetCombinerConfig returns the bonding config from server perspective.
func (h *CombinerHandler) GetCombinerConfig(c *gin.Context) {
	var cfg models.BondingEngineConfig
	if err := db.DB.First(&cfg).Error; err != nil {
		cfg = models.BondingEngineConfig{
			Mode:     "combiner",
			OriginID: "default",
		}
	}
	c.JSON(http.StatusOK, cfg)
}

// SaveCombinerConfig saves the combiner configuration.
func (h *CombinerHandler) SaveCombinerConfig(c *gin.Context) {
	var input models.BondingEngineConfig
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var existing models.BondingEngineConfig
	if err := db.DB.First(&existing).Error; err != nil {
		input.ID = 0
		if err := db.DB.Create(&input).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	} else {
		input.ID = existing.ID
		if err := db.DB.Save(&input).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{"status": "ok", "config": input})
}

// AutoStartCombiner starts the combiner if configured and active.
// Called from main.go during server boot.
//
// On Clever Cloud (and similar ephemeral-disk platforms) the SQLite database
// is wiped on every deployment. To prevent the combiner from silently staying
// offline after a redeploy, we fall back to a hard-coded production baseline
// whenever no DB record exists, and persist it so the UI shows the right state.
func (h *CombinerHandler) AutoStartCombiner() {
	var cfg models.BondingEngineConfig
	if err := db.DB.First(&cfg).Error; err != nil {
		// DB was wiped (fresh deploy) — seed a baseline production config.
		psk := h.cfg.BondingPSKHex // read from env: BONDING_PSK_HEX
		if psk == "" {
			logger.Warn("Combiner", "BONDING_PSK env var not set; combiner will start without HMAC validation")
		}
		cfg = models.BondingEngineConfig{
			OriginID: "clever-cloud-prod",
			PSKHex:   psk,
			IsActive: true,
		}
		if createErr := db.DB.Create(&cfg).Error; createErr != nil {
			logger.Error("Combiner", "Failed to seed fallback combiner config", "error", createErr)
		}
		logger.Info("Combiner", "Seeded fallback combiner config (fresh deployment detected)", "origin", cfg.OriginID)
	} else if !cfg.IsActive {
		// Config exists but was explicitly disabled — honour that choice.
		logger.Info("Combiner", "Combiner is disabled in config (IsActive=false); skipping auto-start")
		return
	}

	h.combiner = combiner.NewCombiner(cfg.OriginID, cfg.PSKHex)
	h.combiner.Start()
	logger.Info("Combiner", "Auto-started server combiner", "origin", cfg.OriginID)
}

// DiagnoseCombiner runs step-by-step diagnostic checks on server combiner configuration.
func (h *CombinerHandler) DiagnoseCombiner(c *gin.Context) {
	var steps []DiagnosticStep

	// 1. Database Configuration
	var cfg models.BondingEngineConfig
	if c.Request.Method == "POST" {
		if err := c.ShouldBindJSON(&cfg); err != nil {
			steps = append(steps, DiagnosticStep{
				Name:         "Database Configuration",
				Description:  "Verify engine configuration exists in database",
				Status:       "error",
				ErrorMessage: fmt.Sprintf("Invalid diagnostic input configuration: %v", err),
			})
			c.JSON(http.StatusOK, steps)
			return
		}
	} else {
		if err := db.DB.First(&cfg).Error; err != nil {
			steps = append(steps, DiagnosticStep{
				Name:         "Database Configuration",
				Description:  "Verify engine configuration exists in database",
				Status:       "error",
				ErrorMessage: "No bonding configuration found. Please save settings first.",
			})
			c.JSON(http.StatusOK, steps)
			return
		}
	}

	steps = append(steps, DiagnosticStep{
		Name:        "Database Configuration",
		Description: "Verify engine configuration exists in database",
		Status:      "success",
		Details:     fmt.Sprintf("Configuration loaded (OriginID: %s).", cfg.OriginID),
	})

	// 2. Pre-Shared Key format
	if cfg.PSKHex != "" {
		_, err := hex.DecodeString(cfg.PSKHex)
		if err != nil {
			steps = append(steps, DiagnosticStep{
				Name:         "Pre-Shared Key Format",
				Description:  "Verify Pre-Shared Key is valid hexadecimal format",
				Status:       "error",
				ErrorMessage: fmt.Sprintf("PSK is not a valid hex string: %v", err),
			})
		} else {
			steps = append(steps, DiagnosticStep{
				Name:        "Pre-Shared Key Format",
				Description: "Verify Pre-Shared Key is valid hexadecimal format",
				Status:      "success",
				Details:     "Pre-Shared Key is valid hexadecimal.",
			})
		}
	} else {
		steps = append(steps, DiagnosticStep{
			Name:        "Pre-Shared Key Format",
			Description: "Verify Pre-Shared Key is valid hexadecimal format",
			Status:      "warning",
			Details:     "PSK is empty. Combiner is running in open/dev mode (no HMAC validation).",
		})
	}

	// 3. Combiner Engine State
	if h.combiner != nil && h.combiner.IsRunning() {
		steps = append(steps, DiagnosticStep{
			Name:        "Combiner Engine State",
			Description: "Verify whether the combiner engine is active and running",
			Status:      "success",
			Details:     "Combiner engine is currently active.",
		})
	} else {
		steps = append(steps, DiagnosticStep{
			Name:        "Combiner Engine State",
			Description: "Verify whether the combiner engine is active and running",
			Status:      "warning",
			Details:     "Combiner engine is stopped. Diagnostics can still check routing, but clients cannot connect.",
		})
	}

	// 4. Exit Target Routing (resolve & tcp dial 1.1.1.1:53)
	conn, err := net.DialTimeout("tcp", "1.1.1.1:53", 2*time.Second)
	if err != nil {
		steps = append(steps, DiagnosticStep{
			Name:         "Exit Target Routing",
			Description:  "Verify exit internet egress from the server (dials 1.1.1.1:53)",
			Status:       "error",
			ErrorMessage: fmt.Sprintf("Server internet egress failed: %v", err),
		})
	} else {
		conn.Close()
		steps = append(steps, DiagnosticStep{
			Name:        "Exit Target Routing",
			Description: "Verify exit internet egress from the server (dials 1.1.1.1:53)",
			Status:      "success",
			Details:     "Successfully reached DNS target 1.1.1.1:53.",
		})
	}

	c.JSON(http.StatusOK, steps)
}
