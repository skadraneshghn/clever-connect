package downloader

import (
	"context"
	"fmt"
	"io"
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
func (e *Engine) AddJob(downloadURL, saveDir, filename, username, password string, threads int, usePremium bool) (string, error) {
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
		UsePremium:    usePremium,
		Progress:      0,
	}

	if err := db.DB.Create(job).Error; err != nil {
		return "", err
	}

	logger.Info("Downloader", "Added new leech job with auth check", "id", jobID, "url", downloadURL, "premium", usePremium)
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

	var downloadURL = job.URL
	var resolvedFilename = job.Filename

	if job.UsePremium {
		var cfg models.LeechConfig
		if err := db.DB.First(&cfg).Error; err != nil || cfg.PremiumUserID == "" || cfg.PremiumAPIKey == "" {
			cancel()
			return fmt.Errorf("Premium.to credentials (User ID and API Key) are not configured in settings")
		}

		apiURL := fmt.Sprintf("http://api.premium.to/api/2/getfile.php?userid=%s&apikey=%s&link=%s",
			url.QueryEscape(cfg.PremiumUserID),
			url.QueryEscape(cfg.PremiumAPIKey),
			url.QueryEscape(job.URL),
		)

		var httpClient *http.Client
		if e.client != nil {
			if hc, ok := e.client.HTTPClient.(*http.Client); ok {
				httpClient = hc
			}
		}
		if httpClient == nil {
			httpClient = http.DefaultClient
		}

		resolvedURL, finalFilename, err := resolvePremiumURL(apiURL, httpClient)
		if err != nil {
			cancel()
			return fmt.Errorf("failed to resolve premium.to link: %w", err)
		}
		downloadURL = resolvedURL
		if finalFilename != "" && finalFilename != "getfile.php" {
			resolvedFilename = finalFilename
			db.DB.Model(&models.LeechJob{}).Where("id = ?", jobID).Update("filename", resolvedFilename)
		}
	}

	// Create request
	var destPath string
	if resolvedFilename == "getfile.php" || resolvedFilename == "downloaded_file" || resolvedFilename == "" {
		destPath = absSaveDir
	} else {
		destPath = filepath.Join(absSaveDir, resolvedFilename)
	}

	req, err := grab.NewRequest(destPath, downloadURL)
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

			updates := map[string]interface{}{
				"status":      "downloading",
				"downloaded":  bytesComplete,
				"total_bytes": totalBytes,
				"progress":    progress,
				"speed":       speed,
			}

			if resp.Filename != "" {
				updates["filename"] = filepath.Base(resp.Filename)
			}

			db.DB.Model(&models.LeechJob{}).Where("id = ?", jobID).Updates(updates)

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

			var job models.LeechJob
			_ = db.DB.First(&job, "id = ?", jobID)
			finalName := job.Filename
			if resp.Filename != "" {
				finalName = filepath.Base(resp.Filename)
			}

			db.DB.Model(&models.LeechJob{}).Where("id = ?", jobID).Updates(map[string]interface{}{
				"status":        status,
				"downloaded":    resp.BytesComplete(),
				"total_bytes":   resp.Size(),
				"progress":      100.0,
				"speed":         0.0,
				"error_message": errMsg,
				"filename":      finalName,
			})

			e.mu.Lock()
			delete(e.activeJobs, jobID)
			e.mu.Unlock()
			return
		}
	}
}

// resolvePremiumURL follows premium.to redirects and parses response links and filenames
func resolvePremiumURL(apiURL string, client *http.Client) (string, string, error) {
	currentURL := apiURL
	var finalFilename string

	noRedirectClient := &http.Client{
		Transport: client.Transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
		Timeout: 15 * time.Second,
	}

	// Limit to maximum 10 redirects to prevent infinite loops
	for i := 0; i < 10; i++ {
		req, err := http.NewRequest("GET", currentURL, nil)
		if err != nil {
			return "", "", err
		}

		resp, err := noRedirectClient.Do(req)
		if err != nil {
			return "", "", err
		}

		// Capture Content-Disposition if present
		if disp := resp.Header.Get("Content-Disposition"); disp != "" {
			if fn := parseContentDispositionFilename(disp); fn != "" {
				finalFilename = fn
			}
		}

		isRedirect := resp.StatusCode == http.StatusFound || resp.StatusCode == http.StatusMovedPermanently || 
			resp.StatusCode == http.StatusTemporaryRedirect || resp.StatusCode == http.StatusSeeOther || 
			resp.StatusCode == 308

		if isRedirect {
			resp.Body.Close()
			loc := resp.Header.Get("Location")
			if loc != "" {
				// Resolve relative URLs relative to currentURL
				base, err := url.Parse(currentURL)
				if err == nil {
					locURL, err := base.Parse(loc)
					if err == nil {
						currentURL = locURL.String()
					} else {
						currentURL = loc
					}
				} else {
					currentURL = loc
				}
				continue
			}
			break
		}

		// If it's the first request, we need to handle plain text URL or JSON error in body
		if i == 0 {
			bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
			resp.Body.Close()
			bodyStr := strings.TrimSpace(string(bodyBytes))

			if strings.HasPrefix(bodyStr, "{") {
				if strings.Contains(bodyStr, "\"error\"") || strings.Contains(bodyStr, "\"err\"") {
					return "", "", fmt.Errorf("premium.to error response: %s", bodyStr)
				}
			}

			if strings.HasPrefix(bodyStr, "http://") || strings.HasPrefix(bodyStr, "https://") {
				currentURL = bodyStr
				continue
			}
		} else {
			resp.Body.Close()
		}

		// If we reach here and it was not a redirect (and not a URL in body of the first request), we are done.
		break
	}

	// If filename is still empty, parse from final URL path
	if finalFilename == "" {
		parsed, err := url.Parse(currentURL)
		if err == nil {
			finalFilename = filepath.Base(parsed.Path)
		}
	}

	return currentURL, finalFilename, nil
}

func parseContentDispositionFilename(disp string) string {
	parts := strings.Split(disp, ";")
	var filename string
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if strings.HasPrefix(part, "filename*=") {
			val := strings.TrimPrefix(part, "filename*=")
			val = strings.Trim(val, "\"")
			subParts := strings.SplitN(val, "'", 3)
			if len(subParts) == 3 {
				decoded, err := url.PathUnescape(subParts[2])
				if err == nil {
					return decoded
				}
			} else {
				decoded, err := url.PathUnescape(val)
				if err == nil {
					return decoded
				}
			}
		} else if strings.HasPrefix(part, "filename=") {
			val := strings.TrimPrefix(part, "filename=")
			filename = strings.Trim(val, "\"")
		}
	}
	return filename
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
