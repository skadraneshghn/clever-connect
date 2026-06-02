package downloader

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"clever-connect/internal/db"
	"clever-connect/internal/logger"
	"clever-connect/internal/models"

	"github.com/cavaliergopher/grab/v3"
)

var (
	Manager  *Engine
	initOnce sync.Once
)

// getAbsoluteSavePath resolves any relative or absolute download folder path
// to ensure it is sandboxed and located inside the File Manager's root folder ("./data/manager")
func getAbsoluteSavePath(saveDir string) string {
	absBase, _ := filepath.Abs("./data/manager")
	
	// Check if already absolute and contains the data/manager path
	absSave, err := filepath.Abs(saveDir)
	if err == nil && strings.HasPrefix(absSave, absBase) {
		return absSave
	}

	// Clean path and ensure it's nested under the absolute base
	clean := filepath.Clean("/" + saveDir)
	return filepath.Join(absBase, clean)
}

type Engine struct {
	client     *grab.Client
	activeJobs map[string]context.CancelFunc
	mu         sync.RWMutex
	stopChan   chan struct{}
}

// Init initializes the singleton download engine and starts the queue worker
func Init() {
	initOnce.Do(func() {
		// Load config from DB
		var cfg models.LeechConfig
		if err := db.DB.First(&cfg).Error; err != nil {
			cfg.UserAgent = "CleverConnect/1.0"
		}

		// Create custom HTTP client with custom User-Agent
		httpClient := &http.Client{
			Transport: &http.Transport{
				Proxy: http.ProxyFromEnvironment,
			},
		}

		if cfg.ProxyURL != "" {
			if proxyURI, err := url.Parse(cfg.ProxyURL); err == nil {
				httpClient.Transport = &http.Transport{
					Proxy: http.ProxyURL(proxyURI),
				}
			}
		}

		grabClient := grab.NewClient()
		grabClient.HTTPClient = httpClient
		grabClient.UserAgent = cfg.UserAgent

		Manager = &Engine{
			client:     grabClient,
			activeJobs: make(map[string]context.CancelFunc),
			stopChan:   make(chan struct{}),
		}

		// Clean up any stale downloading statuses from a previous hard crash
		db.DB.Model(&models.LeechJob{}).Where("status = ?", "downloading").Update("status", "paused")

		// Start background queue daemon
		go Manager.startQueueWorker()
		logger.Info("Downloader", "Leech Engine and Queue Worker initialized")
	})
}

// Close stops the queue worker
func (e *Engine) Close() {
	close(e.stopChan)
}

// AddJob registers a new download task
func (e *Engine) AddJob(downloadURL, saveDir, filename, username, password string, threads int) (string, error) {
	// Generate unique Job ID
	jobID := fmt.Sprintf("job_%d", time.Now().UnixNano())

	// If filename is empty, extract from URL
	if filename == "" {
		parsed, err := url.Parse(downloadURL)
		if err == nil {
			filename = filepath.Base(parsed.Path)
		}
		if filename == "" || filename == "." || filename == "/" {
			filename = "downloaded_file"
		}
	}

	// Default save directory if empty
	if saveDir == "" {
		var cfg models.LeechConfig
		if err := db.DB.First(&cfg).Error; err == nil {
			saveDir = cfg.DefaultSavePath
		} else {
			saveDir = "./downloads"
		}
	}

	job := &models.LeechJob{
		ID:            jobID,
		URL:           downloadURL,
		Filename:      filename,
		SaveDirectory: saveDir,
		Status:        "pending",
		Threads:       threads,
		Username:      username,
		Password:      password,
		Progress:      0,
	}

	if err := db.DB.Create(job).Error; err != nil {
		return "", err
	}

	logger.Info("Downloader", "Added new leech job with auth check", "id", jobID, "url", downloadURL)
	return jobID, nil
}

// StartJob triggers/resumes a specific download task
func (e *Engine) StartJob(jobID string) error {
	var job models.LeechJob
	if err := db.DB.First(&job, "id = ?", jobID).Error; err != nil {
		return err
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	// If already active, do nothing
	if _, active := e.activeJobs[jobID]; active {
		return nil
	}

	// Create download workspace folder if it doesn't exist
	absSaveDir := getAbsoluteSavePath(job.SaveDirectory)
	if err := os.MkdirAll(absSaveDir, 0755); err != nil {
		return fmt.Errorf("failed to create download workspace: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	e.activeJobs[jobID] = cancel

	// Create request
	destPath := filepath.Join(absSaveDir, job.Filename)
	req, err := grab.NewRequest(destPath, job.URL)
	if err != nil {
		cancel()
		return err
	}
	req.HTTPRequest = req.HTTPRequest.WithContext(ctx)

	// Set credentials if provided
	if job.Username != "" || job.Password != "" {
		req.HTTPRequest.SetBasicAuth(job.Username, job.Password)
	}

	// Set configuration values
	var cfg models.LeechConfig
	if err := db.DB.First(&cfg).Error; err == nil {
		if cfg.UserAgent != "" {
			req.HTTPRequest.Header.Set("User-Agent", cfg.UserAgent)
		}
	}

	// Execute download async
	resp := e.client.Do(req)

	go e.monitorProgress(resp, jobID, ctx)

	return nil
}

// PauseJob requests cancellation of a downloading job
func (e *Engine) PauseJob(jobID string) {
	e.mu.Lock()
	cancel, exists := e.activeJobs[jobID]
	e.mu.Unlock()

	if exists {
		cancel()
		e.mu.Lock()
		delete(e.activeJobs, jobID)
		e.mu.Unlock()
	}

	// Force state change
	db.DB.Model(&models.LeechJob{}).Where("id = ?", jobID).Updates(map[string]interface{}{
		"status": "paused",
		"speed":  0,
	})
	logger.Info("Downloader", "Leech job paused", "id", jobID)
}

// DeleteJob cancels and deletes download records/files optionally
func (e *Engine) DeleteJob(jobID string, deleteFiles bool) {
	e.PauseJob(jobID)

	var job models.LeechJob
	if err := db.DB.First(&job, "id = ?", jobID).Error; err == nil {
		if deleteFiles {
			absSaveDir := getAbsoluteSavePath(job.SaveDirectory)
			destPath := filepath.Join(absSaveDir, job.Filename)
			_ = os.Remove(destPath)
			// Remove temporary grab files too (.grab files)
			_ = os.Remove(destPath + ".gtmp")
		}
		db.DB.Unscoped().Delete(&job)
	}
	logger.Info("Downloader", "Leech job deleted", "id", jobID, "deleteFiles", deleteFiles)
}

// monitorProgress updates database statistics every 500ms
func (e *Engine) monitorProgress(resp *grab.Response, jobID string, ctx context.Context) {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			// Cancelled/Paused context
			db.DB.Model(&models.LeechJob{}).Where("id = ?", jobID).Updates(map[string]interface{}{
				"status": "paused",
				"speed":  0,
			})
			return

		case <-ticker.C:
			// Calculate progress
			bytesComplete := resp.BytesComplete()
			totalBytes := resp.Size()
			progress := 100 * resp.Progress()
			speed := resp.BytesPerSecond() / (1024 * 1024) // MB/s

			db.DB.Model(&models.LeechJob{}).Where("id = ?", jobID).Updates(map[string]interface{}{
				"status":      "downloading",
				"downloaded":  bytesComplete,
				"total_bytes": totalBytes,
				"progress":    progress,
				"speed":       speed,
			})

		case <-resp.Done:
			var status = "completed"
			var errMsg = ""
			if err := resp.Err(); err != nil {
				status = "error"
				errMsg = err.Error()
				logger.Error("Downloader", "Leech job completed with error", "id", jobID, "error", err)
			} else {
				logger.Info("Downloader", "Leech job completed successfully", "id", jobID)
			}

			db.DB.Model(&models.LeechJob{}).Where("id = ?", jobID).Updates(map[string]interface{}{
				"status":        status,
				"downloaded":    resp.BytesComplete(),
				"total_bytes":   resp.Size(),
				"progress":      100.0,
				"speed":         0.0,
				"error_message": errMsg,
			})

			e.mu.Lock()
			delete(e.activeJobs, jobID)
			e.mu.Unlock()
			return
		}
	}
}

// startQueueWorker runs a queue tick loop checking for pending downloads
func (e *Engine) startQueueWorker() {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-e.stopChan:
			return
		case <-ticker.C:
			// Fetch configuration limits
			var cfg models.LeechConfig
			maxConcurrent := 3
			if err := db.DB.First(&cfg).Error; err == nil {
				maxConcurrent = cfg.MaxConcurrent
			}

			// Get active downloads count
			var activeCount int64
			db.DB.Model(&models.LeechJob{}).Where("status = ?", "downloading").Count(&activeCount)

			// If we have free slots, start pending jobs
			if int(activeCount) < maxConcurrent {
				slotsAvailable := maxConcurrent - int(activeCount)
				var pendingJobs []models.LeechJob
				db.DB.Where("status = ?", "pending").Order("created_at asc").Limit(slotsAvailable).Find(&pendingJobs)

				for _, job := range pendingJobs {
					logger.Info("Downloader", "Queue starting pending job", "id", job.ID)
					if err := e.StartJob(job.ID); err != nil {
						db.DB.Model(&models.LeechJob{}).Where("id = ?", job.ID).Updates(map[string]interface{}{
							"status":        "error",
							"error_message": err.Error(),
						})
					}
				}
			}
		}
	}
}
