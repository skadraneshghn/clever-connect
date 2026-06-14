package handlers

import (
	"net/http"

	bonding_client "clever-connect/internal/bonding/client"
	"clever-connect/internal/bonding/selector"
	"clever-connect/internal/config"
	"clever-connect/internal/db"
	"clever-connect/internal/logger"
	"clever-connect/internal/models"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

// BondingHandler handles all DMB Engine API requests.
type BondingHandler struct {
	cfg *config.Config
}

// NewBondingHandler creates a new bonding handler.
func NewBondingHandler(cfg *config.Config) *BondingHandler {
	return &BondingHandler{cfg: cfg}
}

// GetConfig returns the current bonding engine configuration.
func (h *BondingHandler) GetConfig(c *gin.Context) {
	var cfg models.BondingEngineConfig
	if err := db.DB.First(&cfg).Error; err != nil {
		// Return defaults if no config exists
		cfg = models.BondingEngineConfig{
			Mode:          "selector",
			StripingMode:  "auto",
			MaxArteries:   h.cfg.BondingMaxArteries,
			MinArteries:   2,
			SocksPort:     h.cfg.BondingSocksPort,
			HTTPPort:      h.cfg.BondingHTTPPort,
			FrameSize:     h.cfg.BondingFrameSize,
			CombinerURL:   h.cfg.BondingCombinerURL,
			EvalWindowMs:  5000,
			DemoteRTTx:    1.5,
			PromoteRTTx:   1.2,
			LossDemotePct: 5.0,
			CooldownSec:   30,
			ErrorBudget:   5,
		}
	}
	c.JSON(http.StatusOK, cfg)
}

// SaveConfig updates the bonding engine configuration.
func (h *BondingHandler) SaveConfig(c *gin.Context) {
	var input models.BondingEngineConfig
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Upsert: find or create
	var existing models.BondingEngineConfig
	if err := db.DB.First(&existing).Error; err != nil {
		// Create new
		input.ID = 0
		if err := db.DB.Create(&input).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	} else {
		// Update existing
		input.ID = existing.ID
		if err := db.DB.Save(&input).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{"status": "ok", "config": input})
}

// StartEngine starts the bonding engine (selector or bonding mode).
func (h *BondingHandler) StartEngine(c *gin.Context) {
	var cfg models.BondingEngineConfig
	if err := db.DB.First(&cfg).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no bonding configuration found; save a configuration first"})
		return
	}

	var startErr error
	if cfg.Mode == "bonding" {
		// Mode B: full multipath bonding
		engine := bonding_client.GetBondingEngine()
		startErr = engine.StartEngine(&cfg)
	} else {
		// Mode A: selector/failover (default)
		engine := selector.GetEngine()
		startErr = engine.StartEngine(&cfg)
	}

	if startErr != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": startErr.Error()})
		return
	}

	// Mark as active in DB
	cfg.IsActive = true
	db.DB.Save(&cfg)

	c.JSON(http.StatusOK, gin.H{
		"status": "started",
		"mode":   cfg.Mode,
	})
}

// StopEngine stops the bonding engine.
func (h *BondingHandler) StopEngine(c *gin.Context) {
	// Stop both engines (only the running one will do work)
	selectorEngine := selector.GetEngine()
	_ = selectorEngine.StopEngine()

	bondingEngine := bonding_client.GetBondingEngine()
	_ = bondingEngine.StopEngine()

	// Mark as inactive in DB
	var cfg models.BondingEngineConfig
	if err := db.DB.First(&cfg).Error; err == nil {
		cfg.IsActive = false
		db.DB.Save(&cfg)
	}

	c.JSON(http.StatusOK, gin.H{"status": "stopped"})
}

// GetStatus returns the current engine status with artery details.
func (h *BondingHandler) GetStatus(c *gin.Context) {
	// Check which engine is running
	selectorEngine := selector.GetEngine()
	if selectorEngine.State() == selector.EngineStateRunning {
		c.JSON(http.StatusOK, selectorEngine.GetStatus())
		return
	}

	bondingEngine := bonding_client.GetBondingEngine()
	if bondingEngine.State() == bonding_client.BondingStateRunning {
		c.JSON(http.StatusOK, bondingEngine.GetStatus())
		return
	}

	// Neither is running — return selector status (shows "stopped")
	c.JSON(http.StatusOK, selectorEngine.GetStatus())
}

// ListArteries returns all arteries in the bonding pool from the database.
func (h *BondingHandler) ListArteries(c *gin.Context) {
	var arteries []models.BondingArtery
	if err := db.DB.Find(&arteries).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, arteries)
}

// ServeTelemetryWS serves a WebSocket connection for live bonding telemetry.
func (h *BondingHandler) ServeTelemetryWS(c *gin.Context) {
	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true },
	}

	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		logger.Error("Bonding", "Failed to upgrade WebSocket", "error", err)
		return
	}
	defer conn.Close()

	engine := selector.GetEngine()

	// Read from telemetry channel and write to WS
	for {
		select {
		case status, ok := <-engine.TelemetryChan:
			if !ok {
				return
			}
			if err := conn.WriteJSON(status); err != nil {
				return
			}
		}
	}
}
