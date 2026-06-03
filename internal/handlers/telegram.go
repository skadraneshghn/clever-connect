package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"

	"clever-connect/internal/config"
	"clever-connect/internal/db"
	"clever-connect/internal/logger"
	"clever-connect/internal/models"
	"clever-connect/internal/scheduler"
	"clever-connect/internal/telegram"

	"github.com/gin-gonic/gin"
	tele "gopkg.in/telebot.v4"
)

// TelegramHandler provides REST API endpoints for the Telegram bot core.
// On client mode, ALL requests are proxied to the remote server — the client
// never runs any Telegram operations locally.
type TelegramHandler struct {
	cfg *config.Config
}

// NewTelegramHandler creates a new handler instance.
func NewTelegramHandler(cfg *config.Config) *TelegramHandler {
	return &TelegramHandler{cfg: cfg}
}

// proxyToServer forwards requests from the Client Panel to the remote server.
// Returns true if the request was proxied (client mode), false if local (server mode).
func (h *TelegramHandler) proxyToServer(c *gin.Context) bool {
	if h.cfg.AppMode == "server" {
		return false
	}

	var remoteURLTarget string
	var remoteToken string

	// 1. Check if configured via environment variables
	if h.cfg.ServerURL != "" {
		remoteURLTarget = h.cfg.ServerURL
		remoteToken = h.cfg.ServerAuthToken
	} else {
		// 2. Fall back to reading remote server client config from database
		var clientCfg models.EhcoClientConfig
		if err := db.DB.First(&clientCfg).Error; err != nil || clientCfg.RemoteURL == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "No remote server connection configured in client panel"})
			return true
		}
		remoteURLTarget = clientCfg.RemoteURL
		remoteToken = clientCfg.AuthToken
	}

	// Convert ws/wss to http/https
	remoteHost := remoteURLTarget
	remoteHost = strings.Replace(remoteHost, "wss://", "https://", 1)
	remoteHost = strings.Replace(remoteHost, "ws://", "http://", 1)

	// Strip trailing path segments like /ws or /tunnel
	if idx := strings.Index(remoteHost, "/ws"); idx != -1 {
		remoteHost = remoteHost[:idx]
	}
	if idx := strings.Index(remoteHost, "/tunnel"); idx != -1 {
		remoteHost = remoteHost[:idx]
	}
	remoteHost = strings.TrimSuffix(remoteHost, "/")

	// Build remote URL using the original request path
	apiPath := c.Request.URL.Path
	remoteURL := remoteHost + apiPath
	if c.Request.URL.RawQuery != "" {
		remoteURL += "?" + c.Request.URL.RawQuery
	}

	// Create proxy request
	req, err := http.NewRequest(c.Request.Method, remoteURL, c.Request.Body)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create proxy request", "details": err.Error()})
		return true
	}

	// Copy original request headers
	for k, vv := range c.Request.Header {
		for _, v := range vv {
			req.Header.Add(k, v)
		}
	}

	// Overwrite local credentials with the remote server auth token
	if remoteToken != "" {
		req.Header.Set("Authorization", "Bearer "+remoteToken)
	}

	// Execute proxy request to remote server
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "Remote server connection refused or timed out", "details": err.Error()})
		return true
	}
	defer resp.Body.Close()

	// Copy response headers
	for k, vv := range resp.Header {
		for _, v := range vv {
			c.Writer.Header().Add(k, v)
		}
	}
	c.Writer.WriteHeader(resp.StatusCode)

	// Pipe remote response back directly
	_, _ = io.Copy(c.Writer, resp.Body)
	return true
}

// GetConfig returns the current Telegram configuration from the database.
// GET /api/telegram/config
func (h *TelegramHandler) GetConfig(c *gin.Context) {
	if h.proxyToServer(c) {
		return
	}

	var cfg models.TelegramConfig
	if err := db.DB.First(&cfg).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{
			"config":  models.TelegramConfig{PollingInterval: 10, MaxFileSize: 50, EnableFileSharing: true, EnableNotifications: true},
			"running": false,
		})
		return
	}

	// Mask the bot token for security — only show last 8 chars
	maskedToken := cfg.BotToken
	if len(maskedToken) > 8 {
		maskedToken = "••••••••" + maskedToken[len(maskedToken)-8:]
	}

	running := telegram.IsRunning()
	var stats map[string]interface{}
	if running {
		if eng := telegram.GetEngine(); eng != nil {
			stats = eng.Stats()
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"config":       cfg,
		"masked_token": maskedToken,
		"running":      running,
		"stats":        stats,
	})
}

// SaveConfig persists the Telegram configuration to the database.
// POST /api/telegram/config
func (h *TelegramHandler) SaveConfig(c *gin.Context) {
	if h.proxyToServer(c) {
		return
	}

	var req models.TelegramConfig
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request payload"})
		return
	}

	if req.PollingInterval < 1 {
		req.PollingInterval = 10
	}
	if req.MaxFileSize < 1 {
		req.MaxFileSize = 50
	}

	var existing models.TelegramConfig
	if err := db.DB.First(&existing).Error; err != nil {
		if err := db.DB.Create(&req).Error; err != nil {
			logger.Error("Telegram", "Failed to create Telegram config", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save configuration"})
			return
		}
	} else {
		if req.BotToken == "" {
			req.BotToken = existing.BotToken
		}
		req.Model = existing.Model
		if err := db.DB.Save(&req).Error; err != nil {
			logger.Error("Telegram", "Failed to update Telegram config", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save configuration"})
			return
		}
	}

	if eng := telegram.GetEngine(); eng != nil {
		_ = eng.ReloadConfig()
	}

	logger.Info("Telegram", "Configuration saved", "ip", c.ClientIP())
	c.JSON(http.StatusOK, gin.H{"status": "success", "message": "Telegram configuration saved"})
}

// TestConnection tests the bot token by calling Telegram's getMe API.
// POST /api/telegram/test
func (h *TelegramHandler) TestConnection(c *gin.Context) {
	if h.proxyToServer(c) {
		return
	}

	var req struct {
		BotToken string `json:"bot_token" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Bot token is required"})
		return
	}

	pref := tele.Settings{
		Token:   req.BotToken,
		Offline: true,
	}

	bot, err := tele.NewBot(pref)
	if err != nil {
		logger.Warn("Telegram", "Bot token validation failed", "error", err, "ip", c.ClientIP())
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	logger.Info("Telegram", "Bot token validated successfully",
		"bot_username", bot.Me.Username,
		"bot_id", bot.Me.ID,
		"ip", c.ClientIP(),
	)

	c.JSON(http.StatusOK, gin.H{
		"success":         true,
		"bot_username":    bot.Me.Username,
		"bot_id":          bot.Me.ID,
		"first_name":      bot.Me.FirstName,
		"can_join_groups": bot.Me.CanJoinGroups,
	})
}

// StartBot starts the Telegram bot engine.
// POST /api/telegram/start
func (h *TelegramHandler) StartBot(c *gin.Context) {
	if h.proxyToServer(c) {
		return
	}

	if telegram.IsRunning() {
		c.JSON(http.StatusConflict, gin.H{"error": "Bot is already running"})
		return
	}

	var cfg models.TelegramConfig
	if err := db.DB.First(&cfg).Error; err != nil || cfg.BotToken == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No bot token configured. Save a configuration first."})
		return
	}

	if err := telegram.StartEngine(&cfg); err != nil {
		logger.Error("Telegram", "Failed to start bot engine", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to start bot: " + err.Error()})
		return
	}

	db.DB.Model(&cfg).Update("is_active", true)

	logger.Info("Telegram", "Bot started via API", "ip", c.ClientIP())
	c.JSON(http.StatusOK, gin.H{"status": "success", "message": "Telegram bot started"})
}

// StopBot stops the running Telegram bot engine.
// POST /api/telegram/stop
func (h *TelegramHandler) StopBot(c *gin.Context) {
	if h.proxyToServer(c) {
		return
	}

	if err := telegram.StopEngine(); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	db.DB.Model(&models.TelegramConfig{}).Where("1 = 1").Update("is_active", false)

	logger.Info("Telegram", "Bot stopped via API", "ip", c.ClientIP())
	c.JSON(http.StatusOK, gin.H{"status": "success", "message": "Telegram bot stopped"})
}

// GetStatus returns the current bot engine status and metrics.
// GET /api/telegram/status
func (h *TelegramHandler) GetStatus(c *gin.Context) {
	if h.proxyToServer(c) {
		return
	}

	eng := telegram.GetEngine()
	if eng == nil || !telegram.IsRunning() {
		c.JSON(http.StatusOK, gin.H{
			"running": false,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"running": true,
		"stats":   eng.Stats(),
	})
}

// SendFile sends a file from the server file manager to the Telegram bot's admin chat.
// POST /api/telegram/send-file
func (h *TelegramHandler) SendFile(c *gin.Context) {
	if h.proxyToServer(c) {
		return
	}

	var req struct {
		FilePath string `json:"file_path" binding:"required"`
		ChatID   int64  `json:"chat_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "file_path is required"})
		return
	}

	eng := telegram.GetEngine()
	if eng == nil || !telegram.IsRunning() {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Telegram bot is not running"})
		return
	}

	// Prepare payload for job scheduler
	payloadBytes, err := json.Marshal(map[string]interface{}{
		"file_path": req.FilePath,
		"chat_id":   req.ChatID,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to serialize upload payload"})
		return
	}

	// Submit job to enterprise job scheduler
	job, err := scheduler.Engine.SubmitJob(
		"telegram_upload",
		fmt.Sprintf("Telegram Upload: %s", filepath.Base(req.FilePath)),
		fmt.Sprintf("Parallel upload of %s to Telegram", req.FilePath),
		"files", // category
		5,       // priority
		string(payloadBytes),
		"",      // cronExpr
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to queue upload job in scheduler: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": "File upload job queued in scheduler",
		"job_id":  job.ID,
	})
}
