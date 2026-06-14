package handlers

import (
	"net/http"

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
func (h *CombinerHandler) AutoStartCombiner() {
	var cfg models.BondingEngineConfig
	if err := db.DB.First(&cfg).Error; err != nil || !cfg.IsActive {
		return
	}

	h.combiner = combiner.NewCombiner(cfg.OriginID, cfg.PSKHex)
	h.combiner.Start()
	logger.Info("Combiner", "Auto-started server combiner", "origin", cfg.OriginID)
}
