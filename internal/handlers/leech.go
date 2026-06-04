package handlers

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"clever-connect/internal/config"
	"clever-connect/internal/db"
	"clever-connect/internal/downloader"
	"clever-connect/internal/filecore"
	"clever-connect/internal/models"

	"github.com/gin-gonic/gin"
)

type LeechHandler struct {
	cfg *config.Config
}

func NewLeechHandler(cfg *config.Config) *LeechHandler {
	return &LeechHandler{cfg: cfg}
}

// proxyToServer automatically forwards requests from the Client Panel to the remote Clever Cloud server.
func (h *LeechHandler) proxyToServer(c *gin.Context, method string, apiPath string) bool {
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

	// Strip trailing path segments
	if idx := strings.Index(remoteHost, "/ws"); idx != -1 {
		remoteHost = remoteHost[:idx]
	}
	if idx := strings.Index(remoteHost, "/tunnel"); idx != -1 {
		remoteHost = remoteHost[:idx]
	}
	remoteHost = strings.TrimSuffix(remoteHost, "/")

	// Build remote URL
	remoteURL := remoteHost + apiPath
	if c.Request.URL.RawQuery != "" {
		remoteURL += "?" + c.Request.URL.RawQuery
	}

	// Create proxy request
	req, err := http.NewRequest(method, remoteURL, c.Request.Body)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create proxy request", "details": err.Error()})
		return true
	}

	// Copy original headers
	for k, vv := range c.Request.Header {
		for _, v := range vv {
			req.Header.Add(k, v)
		}
	}

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

	// Pipe remote stream/content back directly
	_, _ = io.Copy(c.Writer, resp.Body)
	return true
}

// ListJobs returns all active/inactive download jobs
func (h *LeechHandler) ListJobs(c *gin.Context) {
	if h.proxyToServer(c, c.Request.Method, c.Request.URL.Path) {
		return
	}

	var jobs []models.LeechJob
	if err := db.DB.Order("created_at desc").Find(&jobs).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch jobs", "details": err.Error()})
		return
	}

	// Populate FileExists for completed jobs
	for i := range jobs {
		jobs[i].FileExists = true
		if jobs[i].Status == "completed" {
			absSaveDir := filecore.GetAbsoluteSavePath(jobs[i].SaveDirectory)
			destPath := filepath.Join(absSaveDir, jobs[i].Filename)
			if _, err := os.Stat(destPath); os.IsNotExist(err) {
				jobs[i].FileExists = false
			}
		}
	}

	c.JSON(http.StatusOK, jobs)
}

// AddJob starts a new download job
func (h *LeechHandler) AddJob(c *gin.Context) {
	if h.proxyToServer(c, c.Request.Method, c.Request.URL.Path) {
		return
	}

	var input struct {
		URL           string `json:"url" binding:"required"`
		SaveDirectory string `json:"save_directory"`
		Filename      string `json:"filename"`
		Threads       int    `json:"threads"`
		Username      string `json:"username"`
		Password      string `json:"password"`
		UsePremium    bool   `json:"use_premium"`
	}

	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body", "details": err.Error()})
		return
	}

	if input.Threads <= 0 {
		input.Threads = 8
	}

	jobID, err := downloader.Manager.AddJob(input.URL, input.SaveDirectory, input.Filename, input.Username, input.Password, input.Threads, input.UsePremium)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create leech job", "details": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "queued", "job_id": jobID})
}

// PauseJob pauses an active job
func (h *LeechHandler) PauseJob(c *gin.Context) {
	if h.proxyToServer(c, c.Request.Method, c.Request.URL.Path) {
		return
	}

	var input struct {
		ID string `json:"id" binding:"required"`
	}

	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body", "details": err.Error()})
		return
	}

	downloader.Manager.PauseJob(input.ID)
	c.JSON(http.StatusOK, gin.H{"status": "paused", "job_id": input.ID})
}

// StartJob resumes/starts a download job
func (h *LeechHandler) StartJob(c *gin.Context) {
	if h.proxyToServer(c, c.Request.Method, c.Request.URL.Path) {
		return
	}

	var input struct {
		ID string `json:"id" binding:"required"`
	}

	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body", "details": err.Error()})
		return
	}

	// Change state to pending so queue worker picks it up
	if err := db.DB.Model(&models.LeechJob{}).Where("id = ?", input.ID).Update("status", "pending").Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to resume job", "details": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "queued", "job_id": input.ID})
}

// DeleteJob deletes a job and optionally its files
func (h *LeechHandler) DeleteJob(c *gin.Context) {
	if h.proxyToServer(c, c.Request.Method, c.Request.URL.Path) {
		return
	}

	var input struct {
		ID          string `json:"id" binding:"required"`
		DeleteFiles bool   `json:"delete_files"`
	}

	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body", "details": err.Error()})
		return
	}

	downloader.Manager.DeleteJob(input.ID, input.DeleteFiles)
	c.JSON(http.StatusOK, gin.H{"status": "deleted", "job_id": input.ID})
}

// GetConfig returns the download configurations
func (h *LeechHandler) GetConfig(c *gin.Context) {
	if h.proxyToServer(c, c.Request.Method, c.Request.URL.Path) {
		return
	}

	var cfg models.LeechConfig
	if err := db.DB.First(&cfg).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load downloader config", "details": err.Error()})
		return
	}

	c.JSON(http.StatusOK, cfg)
}

// SaveConfig updates the download configurations
func (h *LeechHandler) SaveConfig(c *gin.Context) {
	if h.proxyToServer(c, c.Request.Method, c.Request.URL.Path) {
		return
	}

	var input models.LeechConfig
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid config payload", "details": err.Error()})
		return
	}

	var cfg models.LeechConfig
	if err := db.DB.First(&cfg).Error; err == nil {
		cfg.DefaultSavePath = input.DefaultSavePath
		cfg.MaxConcurrent = input.MaxConcurrent
		cfg.ThreadsPerJob = input.ThreadsPerJob
		cfg.UserAgent = input.UserAgent
		cfg.ProxyURL = input.ProxyURL
		cfg.PremiumUserID = input.PremiumUserID
		cfg.PremiumAPIKey = input.PremiumAPIKey
		cfg.AutoUploadToTelegram = input.AutoUploadToTelegram
		cfg.AutoUploadChatID = input.AutoUploadChatID
		db.DB.Save(&cfg)
	} else {
		db.DB.Create(&input)
	}

	// Trigger grab client reload
	downloader.Init()

	c.JSON(http.StatusOK, gin.H{"status": "saved"})
}
