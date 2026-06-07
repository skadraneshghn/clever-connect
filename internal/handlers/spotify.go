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
	"clever-connect/internal/spotify"

	"github.com/gin-gonic/gin"
)

type SpotifyHandler struct {
	cfg *config.Config
}

func NewSpotifyHandler(cfg *config.Config) *SpotifyHandler {
	return &SpotifyHandler{cfg: cfg}
}

// proxyToServer forwards requests from client panel to the remote server
func (h *SpotifyHandler) proxyToServer(c *gin.Context, method string, apiPath string) bool {
	if h.cfg.AppMode == "server" {
		return false
	}
	var remoteURLTarget, remoteToken string
	if h.cfg.ServerURL != "" {
		remoteURLTarget = h.cfg.ServerURL
		remoteToken = h.cfg.ServerAuthToken
	} else {
		var clientCfg models.EhcoClientConfig
		if err := db.DB.First(&clientCfg).Error; err != nil || clientCfg.RemoteURL == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "No remote server configured"})
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
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create proxy request"})
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
		c.JSON(http.StatusBadGateway, gin.H{"error": "Remote server connection failed"})
		return true
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		c.JSON(http.StatusBadGateway, gin.H{"error": "Remote server rejected proxy token (401). Please update the remote server or verify your Auth Token."})
		return true
	}

	for k, vv := range resp.Header {
		for _, v := range vv {
			c.Writer.Header().Add(k, v)
		}
	}
	c.Writer.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(c.Writer, resp.Body)
	return true
}

// FetchInfo fetches track/album metadata from Spotify API
func (h *SpotifyHandler) FetchInfo(c *gin.Context) {
	if h.proxyToServer(c, c.Request.Method, c.Request.URL.Path) {
		return
	}
	var input struct {
		URL string `json:"url" binding:"required"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request", "details": err.Error()})
		return
	}
	if spotify.Manager == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Spotify engine not initialized"})
		return
	}
	info, linkType, err := spotify.Manager.FetchInfo(input.URL)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to fetch Spotify info", "details": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"type": linkType, "data": info})
}

// AddJob submits a single track or album for download
func (h *SpotifyHandler) AddJob(c *gin.Context) {
	if h.proxyToServer(c, c.Request.Method, c.Request.URL.Path) {
		return
	}
	var input struct {
		URL           string `json:"url" binding:"required"`
		SaveDirectory string `json:"save_directory"`
		Format        string `json:"format"`
		Bitrate       string `json:"bitrate"`
		// Pre-fetched track info (optional — avoids double API call)
		Type  string             `json:"type"`  // "track" or "album"
		Track *spotify.TrackMeta `json:"track"` // Single track data
		Album *spotify.AlbumMeta `json:"album"` // Album data with tracks
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request", "details": err.Error()})
		return
	}
	if spotify.Manager == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Spotify engine not initialized"})
		return
	}

	// If pre-fetched data is provided, use it directly
	if input.Type == "track" && input.Track != nil {
		jobID, err := spotify.Manager.AddTrackJob(input.Track, input.SaveDirectory, input.Format, input.Bitrate, "")
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create job", "details": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "queued", "job_ids": []string{jobID}})
		return
	}

	if input.Type == "album" && input.Album != nil {
		jobIDs, err := spotify.Manager.AddAlbumJobs(input.Album, input.SaveDirectory, input.Format, input.Bitrate)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create album jobs", "details": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "queued", "job_ids": jobIDs, "count": len(jobIDs)})
		return
	}

	// Otherwise, fetch info from URL first
	info, linkType, err := spotify.Manager.FetchInfo(input.URL)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to fetch info", "details": err.Error()})
		return
	}

	switch linkType {
	case "track":
		track := info.(*spotify.TrackMeta)
		jobID, err := spotify.Manager.AddTrackJob(track, input.SaveDirectory, input.Format, input.Bitrate, "")
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "queued", "job_ids": []string{jobID}})
	case "album":
		album := info.(*spotify.AlbumMeta)
		jobIDs, err := spotify.Manager.AddAlbumJobs(album, input.SaveDirectory, input.Format, input.Bitrate)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "queued", "job_ids": jobIDs, "count": len(jobIDs)})
	}
}

// ListJobs returns all Spotify download jobs
func (h *SpotifyHandler) ListJobs(c *gin.Context) {
	if h.proxyToServer(c, c.Request.Method, c.Request.URL.Path) {
		return
	}
	var jobs []models.SpotifyJob
	if err := db.DB.Order("created_at desc").Find(&jobs).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch jobs"})
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

// CancelJob cancels an active download
func (h *SpotifyHandler) CancelJob(c *gin.Context) {
	if h.proxyToServer(c, c.Request.Method, c.Request.URL.Path) {
		return
	}
	var input struct {
		ID string `json:"id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if spotify.Manager != nil {
		spotify.Manager.CancelJob(input.ID)
	}
	c.JSON(http.StatusOK, gin.H{"status": "cancelled"})
}

// DeleteJob deletes a job and optionally its files
func (h *SpotifyHandler) DeleteJob(c *gin.Context) {
	if h.proxyToServer(c, c.Request.Method, c.Request.URL.Path) {
		return
	}
	var input struct {
		ID          string `json:"id" binding:"required"`
		DeleteFiles bool   `json:"delete_files"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if spotify.Manager != nil {
		spotify.Manager.DeleteJob(input.ID, input.DeleteFiles)
	}
	c.JSON(http.StatusOK, gin.H{"status": "deleted"})
}

// RetryJob re-queues a failed job
func (h *SpotifyHandler) RetryJob(c *gin.Context) {
	if h.proxyToServer(c, c.Request.Method, c.Request.URL.Path) {
		return
	}
	var input struct {
		ID string `json:"id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if spotify.Manager != nil {
		spotify.Manager.RetryJob(input.ID)
	}
	c.JSON(http.StatusOK, gin.H{"status": "retried"})
}

// GetConfig returns the Spotify configuration
func (h *SpotifyHandler) GetConfig(c *gin.Context) {
	if h.proxyToServer(c, c.Request.Method, c.Request.URL.Path) {
		return
	}
	var cfg models.SpotifyConfig
	if err := db.DB.First(&cfg).Error; err != nil {
		c.JSON(http.StatusOK, models.SpotifyConfig{
			DefaultSavePath:  "./downloads/spotify/audios",
			DefaultFormat:    "mp3",
			DefaultBitrate:   "320k",
			MaxConcurrent:    3,
			EmbedMetadata:    true,
			EmbedLyrics:      true,
			FileNameTemplate: "{artist} - {title}",
		})
		return
	}
	c.JSON(http.StatusOK, cfg)
}

// SaveConfig saves the Spotify configuration
func (h *SpotifyHandler) SaveConfig(c *gin.Context) {
	if h.proxyToServer(c, c.Request.Method, c.Request.URL.Path) {
		return
	}
	var input models.SpotifyConfig
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	var cfg models.SpotifyConfig
	if err := db.DB.First(&cfg).Error; err == nil {
		cfg.ClientID = input.ClientID
		cfg.ClientSecret = input.ClientSecret
		cfg.DefaultSavePath = input.DefaultSavePath
		cfg.DefaultFormat = input.DefaultFormat
		cfg.DefaultBitrate = input.DefaultBitrate
		cfg.MaxConcurrent = input.MaxConcurrent
		cfg.EmbedMetadata = input.EmbedMetadata
		cfg.EmbedLyrics = input.EmbedLyrics
		cfg.OverwriteExist = input.OverwriteExist
		cfg.ProxyURL = input.ProxyURL
		cfg.FileNameTemplate = input.FileNameTemplate
		db.DB.Save(&cfg)
	} else {
		db.DB.Create(&input)
	}
	c.JSON(http.StatusOK, gin.H{"status": "saved"})
}
