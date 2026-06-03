package handlers

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"clever-connect/internal/config"
	"clever-connect/internal/db"
	"clever-connect/internal/filecore"
	"clever-connect/internal/models"
	"clever-connect/internal/youtube"

	"github.com/gin-gonic/gin"
)

type YouTubeHandler struct {
	cfg *config.Config
}

func NewYouTubeHandler(cfg *config.Config) *YouTubeHandler {
	return &YouTubeHandler{cfg: cfg}
}

// proxyToServer automatically forwards requests from the Client Panel to the remote Clever Cloud server.
func (h *YouTubeHandler) proxyToServer(c *gin.Context, method string, apiPath string) bool {
	if h.cfg.AppMode == "server" {
		return false
	}

	var remoteURLTarget string
	var remoteToken string

	if h.cfg.ServerURL != "" {
		remoteURLTarget = h.cfg.ServerURL
		remoteToken = h.cfg.ServerAuthToken
	} else {
		var clientCfg models.EhcoClientConfig
		if err := db.DB.First(&clientCfg).Error; err != nil || clientCfg.RemoteURL == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "No remote server connection configured in client panel"})
			return true
		}
		remoteURLTarget = clientCfg.RemoteURL
		remoteToken = clientCfg.AuthToken
	}

	remoteHost := remoteURLTarget
	remoteHost = strings.Replace(remoteHost, "wss://", "https://", 1)
	remoteHost = strings.Replace(remoteHost, "ws://", "http://", 1)
	if idx := strings.Index(remoteHost, "/ws"); idx != -1 {
		remoteHost = remoteHost[:idx]
	}
	if idx := strings.Index(remoteHost, "/tunnel"); idx != -1 {
		remoteHost = remoteHost[:idx]
	}
	remoteHost = strings.TrimSuffix(remoteHost, "/")

	remoteURL := remoteHost + apiPath
	if c.Request.URL.RawQuery != "" {
		remoteURL += "?" + c.Request.URL.RawQuery
	}

	req, err := http.NewRequest(method, remoteURL, c.Request.Body)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create proxy request", "details": err.Error()})
		return true
	}

	for k, vv := range c.Request.Header {
		for _, v := range vv {
			req.Header.Add(k, v)
		}
	}
	if remoteToken != "" {
		req.Header.Set("Authorization", "Bearer "+remoteToken)
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "Remote server connection refused or timed out", "details": err.Error()})
		return true
	}
	defer resp.Body.Close()

	for k, vv := range resp.Header {
		for _, v := range vv {
			c.Writer.Header().Add(k, v)
		}
	}
	c.Writer.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(c.Writer, resp.Body)
	return true
}

// FetchInfo fetches YouTube video metadata and available formats
func (h *YouTubeHandler) FetchInfo(c *gin.Context) {
	if h.proxyToServer(c, c.Request.Method, c.Request.URL.Path) {
		return
	}

	var input struct {
		URL string `json:"url" binding:"required"`
	}

	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body", "details": err.Error()})
		return
	}

	if youtube.Manager == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "YouTube engine not initialized"})
		return
	}

	info, err := youtube.Manager.FetchVideoInfo(input.URL)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to fetch video info", "details": err.Error()})
		return
	}

	c.JSON(http.StatusOK, info)
}

// AddJob starts a new YouTube download job
func (h *YouTubeHandler) AddJob(c *gin.Context) {
	if h.proxyToServer(c, c.Request.Method, c.Request.URL.Path) {
		return
	}

	var input struct {
		URL           string `json:"url" binding:"required"`
		SaveDirectory string `json:"save_directory"`
		SelectedITag  int    `json:"selected_itag" binding:"required"`
		QualityLabel  string `json:"quality_label"`
		MimeType      string `json:"mime_type"`
		ConvertToTV   bool   `json:"convert_to_tv"`
		// Video info fields
		VideoID         string `json:"video_id"`
		Title           string `json:"title"`
		Author          string `json:"author"`
		Duration        string `json:"duration"`
		DurationSeconds int64  `json:"duration_seconds"`
		Thumbnail       string `json:"thumbnail"`
	}

	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body", "details": err.Error()})
		return
	}

	if youtube.Manager == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "YouTube engine not initialized"})
		return
	}

	videoInfo := &youtube.VideoInfo{
		VideoID:         input.VideoID,
		Title:           input.Title,
		Author:          input.Author,
		Duration:        input.Duration,
		DurationSeconds: input.DurationSeconds,
		Thumbnail:       input.Thumbnail,
	}

	jobID, err := youtube.Manager.AddJob(input.URL, input.SaveDirectory, input.SelectedITag, input.QualityLabel, input.MimeType, input.ConvertToTV, videoInfo)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create YouTube job", "details": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "queued", "job_id": jobID})
}

// ListJobs returns all YouTube download jobs
func (h *YouTubeHandler) ListJobs(c *gin.Context) {
	if h.proxyToServer(c, c.Request.Method, c.Request.URL.Path) {
		return
	}

	var jobs []models.YouTubeJob
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

// DeleteJob deletes a YouTube job and optionally its files
func (h *YouTubeHandler) DeleteJob(c *gin.Context) {
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

	if youtube.Manager == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "YouTube engine not initialized"})
		return
	}

	youtube.Manager.DeleteJob(input.ID, input.DeleteFiles)
	c.JSON(http.StatusOK, gin.H{"status": "deleted", "job_id": input.ID})
}

// CancelJob cancels an active download
func (h *YouTubeHandler) CancelJob(c *gin.Context) {
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

	if youtube.Manager == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "YouTube engine not initialized"})
		return
	}

	youtube.Manager.PauseJob(input.ID)
	c.JSON(http.StatusOK, gin.H{"status": "cancelled", "job_id": input.ID})
}

// GetConfig returns the YouTube download configuration
func (h *YouTubeHandler) GetConfig(c *gin.Context) {
	if h.proxyToServer(c, c.Request.Method, c.Request.URL.Path) {
		return
	}

	var cfg models.YouTubeConfig
	if err := db.DB.First(&cfg).Error; err != nil {
		c.JSON(http.StatusOK, models.YouTubeConfig{
			DefaultSavePath: "./downloads/youtube",
			MaxConcurrent:   2,
		})
		return
	}

	c.JSON(http.StatusOK, cfg)
}

// SaveConfig updates the YouTube download configuration
func (h *YouTubeHandler) SaveConfig(c *gin.Context) {
	if h.proxyToServer(c, c.Request.Method, c.Request.URL.Path) {
		return
	}

	var input models.YouTubeConfig
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid config payload", "details": err.Error()})
		return
	}

	var cfg models.YouTubeConfig
	if err := db.DB.First(&cfg).Error; err == nil {
		cfg.DefaultSavePath = input.DefaultSavePath
		cfg.MaxConcurrent = input.MaxConcurrent
		cfg.ProxyURL = input.ProxyURL
		db.DB.Save(&cfg)
	} else {
		db.DB.Create(&input)
	}

	c.JSON(http.StatusOK, gin.H{"status": "saved"})
}
