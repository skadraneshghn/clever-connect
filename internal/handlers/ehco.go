package handlers

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"

	"clever-connect/internal/config"
	"clever-connect/internal/db"
	"clever-connect/internal/ehcocore"
	"clever-connect/internal/logger"
	"clever-connect/internal/models"

	"github.com/gin-gonic/gin"
)

type EhcoHandler struct {
	cfg *config.Config
}

func NewEhcoHandler(cfg *config.Config) *EhcoHandler {
	return &EhcoHandler{cfg: cfg}
}

// GenerateRandomToken generates a hex-encoded random security token
func GenerateRandomToken() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "clever-connect-token-1234"
	}
	return hex.EncodeToString(b)
}

// GetConfig handles GET /api/ehco/config
func (h *EhcoHandler) GetConfig(c *gin.Context) {
	if h.cfg.AppMode == "server" {
		var serverCfg models.EhcoServerConfig
		if err := db.DB.First(&serverCfg).Error; err != nil {
			// Seed a default config record
			serverCfg = models.EhcoServerConfig{
				ListenPort: "3001",
				AuthToken:  GenerateRandomToken(),
				TargetMode: "direct",
				TargetHost: "127.0.0.1:80",
				EnableMux:  true,
				KeepAlive:  15,
				IsActive:   false,
			}
			db.DB.Create(&serverCfg)
		}

		c.JSON(http.StatusOK, gin.H{
			"app_mode":   "server",
			"config":     serverCfg,
			"is_running": ehcocore.IsRunning(),
		})
	} else {
		var clientCfg models.EhcoClientConfig
		if err := db.DB.First(&clientCfg).Error; err != nil {
			// Seed a default config record
			clientCfg = models.EhcoClientConfig{
				LocalPort:  "1080",
				RemoteURL:  "",
				AuthToken:  "",
				SNI:        "",
				EnableMux:  true,
				KeepAlive:  15,
				BypassIR:   true,
				IsActive:   false,
			}
			db.DB.Create(&clientCfg)
		}

		c.JSON(http.StatusOK, gin.H{
			"app_mode":   "client",
			"config":     clientCfg,
			"is_running": ehcocore.IsRunning(),
		})
	}
}

// SaveConfig handles POST /api/ehco/config
func (h *EhcoHandler) SaveConfig(c *gin.Context) {
	if h.cfg.AppMode == "server" {
		var req struct {
			ListenPort string `json:"listen_port"`
			AuthToken  string `json:"auth_token"`
			TargetMode string `json:"target_mode"`
			TargetHost string `json:"target_host"`
			EnableMux  bool   `json:"enable_mux"`
			KeepAlive  int    `json:"keep_alive"`
		}

		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
			return
		}

		var serverCfg models.EhcoServerConfig
		if err := db.DB.First(&serverCfg).Error; err == nil {
			serverCfg.ListenPort = req.ListenPort
			serverCfg.AuthToken = req.AuthToken
			serverCfg.TargetMode = req.TargetMode
			serverCfg.TargetHost = req.TargetHost
			serverCfg.EnableMux = req.EnableMux
			serverCfg.KeepAlive = req.KeepAlive
			db.DB.Save(&serverCfg)
		} else {
			serverCfg = models.EhcoServerConfig{
				ListenPort: req.ListenPort,
				AuthToken:  req.AuthToken,
				TargetMode: req.TargetMode,
				TargetHost: req.TargetHost,
				EnableMux:  req.EnableMux,
				KeepAlive:  req.KeepAlive,
				IsActive:   false,
			}
			db.DB.Create(&serverCfg)
		}

		// Auto-restart if already running to apply settings
		if ehcocore.IsRunning() {
			logger.Info("Ehco", "Configuration updated. Restarting server tunnel engine.")
			ehcocore.StopEngine()
			if err := ehcocore.StartServerEngine(&serverCfg); err != nil {
				serverCfg.IsActive = false
				db.DB.Save(&serverCfg)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Engine restarted but failed with error: " + err.Error()})
				return
			}
		}

		c.JSON(http.StatusOK, gin.H{"status": "saved", "config": serverCfg})
	} else {
		var req struct {
			LocalPort string `json:"local_port"`
			RemoteURL string `json:"remote_url"`
			AuthToken string `json:"auth_token"`
			SNI       string `json:"sni"`
			EnableMux bool   `json:"enable_mux"`
			KeepAlive int    `json:"keep_alive"`
			BypassIR  bool   `json:"bypass_ir"`
		}

		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
			return
		}

		var clientCfg models.EhcoClientConfig
		if err := db.DB.First(&clientCfg).Error; err == nil {
			clientCfg.LocalPort = req.LocalPort
			clientCfg.RemoteURL = req.RemoteURL
			clientCfg.AuthToken = req.AuthToken
			clientCfg.SNI = req.SNI
			clientCfg.EnableMux = req.EnableMux
			clientCfg.KeepAlive = req.KeepAlive
			clientCfg.BypassIR = req.BypassIR
			db.DB.Save(&clientCfg)
		} else {
			clientCfg = models.EhcoClientConfig{
				LocalPort:  req.LocalPort,
				RemoteURL:  req.RemoteURL,
				AuthToken:  req.AuthToken,
				SNI:        req.SNI,
				EnableMux:  req.EnableMux,
				KeepAlive:  req.KeepAlive,
				BypassIR:   req.BypassIR,
				IsActive:   false,
			}
			db.DB.Create(&clientCfg)
		}

		// Auto-restart if already running to apply settings
		if ehcocore.IsRunning() {
			logger.Info("Ehco", "Configuration updated. Restarting client tunnel engine.")
			ehcocore.StopEngine()
			if err := ehcocore.StartClientEngine(&clientCfg); err != nil {
				clientCfg.IsActive = false
				db.DB.Save(&clientCfg)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Engine restarted but failed with error: " + err.Error()})
				return
			}
		}

		c.JSON(http.StatusOK, gin.H{"status": "saved", "config": clientCfg})
	}
}

// StartEngine handles POST /api/ehco/start
func (h *EhcoHandler) StartEngine(c *gin.Context) {
	if h.cfg.AppMode == "server" {
		var serverCfg models.EhcoServerConfig
		if err := db.DB.First(&serverCfg).Error; err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "Ehco server configuration not initialized"})
			return
		}

		if err := ehcocore.StartServerEngine(&serverCfg); err != nil {
			logger.Error("Ehco", "Failed to start server tunnel", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		serverCfg.IsActive = true
		db.DB.Save(&serverCfg)

		c.JSON(http.StatusOK, gin.H{"status": "started", "is_running": true})
	} else {
		var clientCfg models.EhcoClientConfig
		if err := db.DB.First(&clientCfg).Error; err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "Ehco client configuration not initialized"})
			return
		}

		if err := ehcocore.StartClientEngine(&clientCfg); err != nil {
			logger.Error("Ehco", "Failed to start client tunnel", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		clientCfg.IsActive = true
		db.DB.Save(&clientCfg)

		c.JSON(http.StatusOK, gin.H{"status": "started", "is_running": true})
	}
}

// StopEngine handles POST /api/ehco/stop
func (h *EhcoHandler) StopEngine(c *gin.Context) {
	ehcocore.StopEngine()

	if h.cfg.AppMode == "server" {
		var serverCfg models.EhcoServerConfig
		if err := db.DB.First(&serverCfg).Error; err == nil {
			serverCfg.IsActive = false
			db.DB.Save(&serverCfg)
		}
	} else {
		var clientCfg models.EhcoClientConfig
		if err := db.DB.First(&clientCfg).Error; err == nil {
			clientCfg.IsActive = false
			db.DB.Save(&clientCfg)
		}
	}

	c.JSON(http.StatusOK, gin.H{"status": "stopped", "is_running": false})
}
