package spotify

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"clever-connect/internal/db"
	"clever-connect/internal/logger"
	"clever-connect/internal/models"
)

// ──────────────────────────────────────────────────────────────────────────────
// Spotify Download Engine — Singleton with concurrent queue worker
// ──────────────────────────────────────────────────────────────────────────────

var (
	Manager  *Engine
	initOnce sync.Once
)

// Engine orchestrates the Spotify download pipeline
type Engine struct {
	activeJobs map[string]context.CancelFunc
	mu         sync.RWMutex
	stopChan   chan struct{}
}

// Init initializes the singleton Spotify download engine
func Init() {
	initOnce.Do(func() {
		Manager = &Engine{
			activeJobs: make(map[string]context.CancelFunc),
			stopChan:   make(chan struct{}),
		}

		// Reset stale jobs from a previous crash
		db.DB.Model(&models.SpotifyJob{}).
			Where("status IN ?", []string{"downloading", "converting", "tagging", "matching", "fetching_meta"}).
			Updates(map[string]interface{}{
				"status":        "error",
				"error_message": "Server restarted during operation",
			})

		go Manager.startQueueWorker()
		logger.Info("Spotify", "Spotify Download Engine initialized")
	})
}

// Close stops the queue worker
func (e *Engine) Close() {
	close(e.stopChan)
}

// getAbsoluteSavePath resolves the download path under data/manager
func getAbsoluteSavePath(saveDir string) string {
	absBase, _ := filepath.Abs("./data/manager")
	absSave, err := filepath.Abs(saveDir)
	if err == nil && strings.HasPrefix(absSave, absBase) {
		return absSave
	}
	clean := filepath.Clean("/" + saveDir)
	return filepath.Join(absBase, clean)
}

// FetchInfo fetches metadata for a Spotify track or album URL
func (e *Engine) FetchInfo(spotifyURL string) (interface{}, string, error) {
	linkType, id, err := ParseSpotifyURL(spotifyURL)
	if err != nil {
		return nil, "", err
	}

	var cfg models.SpotifyConfig
	hasCredentials := false
	if err := db.DB.First(&cfg).Error; err == nil && cfg.ClientID != "" && cfg.ClientSecret != "" {
		hasCredentials = true
	}

	if hasCredentials {
		client := NewSpotifyClient(cfg.ClientID, cfg.ClientSecret)
		logger.Info("Spotify", "Attempting metadata retrieval via official Spotify Web API", "type", linkType, "id", id)
		switch linkType {
		case "track":
			track, err := client.GetTrack(id)
			if err == nil {
				return track, "track", nil
			}
			logger.Warn("Spotify", "Official API fetch failed, falling back to keyless scraper", "error", err)
		case "album":
			album, err := client.GetAlbum(id)
			if err == nil {
				return album, "album", nil
			}
			logger.Warn("Spotify", "Official API fetch failed, falling back to keyless scraper", "error", err)
		}
	}

	logger.Info("Spotify", "Attempting keyless metadata scraping from public web player", "type", linkType, "id", id)
	scraped, err := ScrapeSpotifyEmbed(linkType, id)
	if err == nil {
		return scraped, linkType, nil
	}

	// If both failed, return a detailed helpful error
	if hasCredentials {
		return nil, "", fmt.Errorf("Spotify metadata lookup failed: %w", err)
	}
	return nil, "", fmt.Errorf("Spotify metadata lookup failed: %v.\n\n"+
		"Note: Keyless scraping is often blocked on cloud/VPS servers by Spotify's CDN. "+
		"To resolve this, go to Settings → Spotify and configure a free Spotify Client ID & Client Secret "+
		"(no Spotify Premium account required!)", err)
}

// AddTrackJob creates a new download job for a single Spotify track
func (e *Engine) AddTrackJob(track *TrackMeta, saveDir, format, bitrate, albumJobID string) (string, error) {
	jobID := fmt.Sprintf("sp_%d", time.Now().UnixNano())

	if saveDir == "" {
		var cfg models.SpotifyConfig
		if err := db.DB.First(&cfg).Error; err == nil && cfg.DefaultSavePath != "" {
			saveDir = cfg.DefaultSavePath
		} else {
			saveDir = "./downloads/spotify/audios"
		}
	}
	if format == "" {
		var cfg models.SpotifyConfig
		if err := db.DB.First(&cfg).Error; err == nil && cfg.DefaultFormat != "" {
			format = cfg.DefaultFormat
		} else {
			format = "mp3"
		}
	}
	if bitrate == "" {
		var cfg models.SpotifyConfig
		if err := db.DB.First(&cfg).Error; err == nil && cfg.DefaultBitrate != "" {
			bitrate = cfg.DefaultBitrate
		} else {
			bitrate = "320k"
		}
	}

	safeTitle := sanitizeFilename(fmt.Sprintf("%s - %s", track.Artist, track.Title))
	filename := safeTitle + "." + format

	artistsJSON, _ := json.Marshal(track.Artists)

	job := &models.SpotifyJob{
		ID:            jobID,
		SpotifyURL:    track.SpotifyURL,
		SpotifyID:     track.ID,
		Title:         track.Title,
		Artist:        track.Artist,
		Artists:       string(artistsJSON),
		Album:         track.Album,
		AlbumArtist:   track.AlbumArtist,
		CoverURL:      track.CoverURL,
		ReleaseDate:   track.ReleaseDate,
		TrackNumber:   track.TrackNumber,
		TotalTracks:   track.TotalTracks,
		DiscNumber:    track.DiscNumber,
		DurationMs:    track.DurationMs,
		ISRC:          track.ISRC,
		Genre:         track.Genre,
		Explicit:      track.Explicit,
		Popularity:    track.Popularity,
		Filename:      filename,
		SaveDirectory: saveDir,
		Format:        format,
		Bitrate:       bitrate,
		Status:        "pending",
		AlbumJobID:    albumJobID,
	}

	if err := db.DB.Create(job).Error; err != nil {
		return "", err
	}

	logger.Info("Spotify", "Added Spotify download job",
		"id", jobID,
		"title", track.Title,
		"artist", track.Artist,
		"format", format,
		"bitrate", bitrate,
	)
	return jobID, nil
}

// AddAlbumJobs creates download jobs for all tracks in an album
func (e *Engine) AddAlbumJobs(album *AlbumMeta, saveDir, format, bitrate string) ([]string, error) {
	albumJobID := fmt.Sprintf("spa_%d", time.Now().UnixNano())

	// Put album tracks in a subfolder
	if saveDir == "" {
		var cfg models.SpotifyConfig
		if err := db.DB.First(&cfg).Error; err == nil && cfg.DefaultSavePath != "" {
			saveDir = cfg.DefaultSavePath
		} else {
			saveDir = "./downloads/spotify/audios"
		}
	}
	albumDir := filepath.Join(saveDir, sanitizeFilename(fmt.Sprintf("%s - %s", album.Artist, album.Name)))

	var jobIDs []string
	for _, track := range album.Tracks {
		jobID, err := e.AddTrackJob(&track, albumDir, format, bitrate, albumJobID)
		if err != nil {
			logger.Error("Spotify", "Failed to add album track job", "track", track.Title, "error", err)
			continue
		}
		jobIDs = append(jobIDs, jobID)
	}

	logger.Info("Spotify", "Added album download jobs",
		"album", album.Name,
		"artist", album.Artist,
		"tracks", len(jobIDs),
	)
	return jobIDs, nil
}

// CancelJob cancels an active download
func (e *Engine) CancelJob(jobID string) {
	e.mu.Lock()
	cancel, exists := e.activeJobs[jobID]
	e.mu.Unlock()

	if exists {
		cancel()
		e.mu.Lock()
		delete(e.activeJobs, jobID)
		e.mu.Unlock()
	}

	db.DB.Model(&models.SpotifyJob{}).Where("id = ?", jobID).Updates(map[string]interface{}{
		"status":        "error",
		"speed":         0,
		"error_message": "Cancelled by user",
	})
}

// DeleteJob cancels and removes a job
func (e *Engine) DeleteJob(jobID string, deleteFiles bool) {
	e.CancelJob(jobID)

	var job models.SpotifyJob
	if err := db.DB.First(&job, "id = ?", jobID).Error; err == nil {
		if deleteFiles {
			absSaveDir := getAbsoluteSavePath(job.SaveDirectory)
			destPath := filepath.Join(absSaveDir, job.Filename)
			_ = os.Remove(destPath)
		}
		db.DB.Unscoped().Delete(&job)
	}
}

// RetryJob re-queues a failed job
func (e *Engine) RetryJob(jobID string) {
	db.DB.Model(&models.SpotifyJob{}).Where("id = ? AND status = ?", jobID, "error").Updates(map[string]interface{}{
		"status":        "pending",
		"progress":      0,
		"speed":         0,
		"error_message": "",
	})
}

// executeJob runs the full download pipeline for a single track
func (e *Engine) executeJob(ctx context.Context, job *models.SpotifyJob) {
	defer func() {
		e.mu.Lock()
		delete(e.activeJobs, job.ID)
		e.mu.Unlock()
	}()

	// Phase 1: YouTube matching
	db.DB.Model(&models.SpotifyJob{}).Where("id = ?", job.ID).Updates(map[string]interface{}{
		"status":   "matching",
		"progress": 5,
	})

	track := &TrackMeta{
		ID:     job.SpotifyID,
		Title:  job.Title,
		Artist: job.Artist,
		ISRC:   job.ISRC,
	}

	ytURL, err := matchYouTube(ctx, track)
	if err != nil {
		e.failJob(job.ID, fmt.Sprintf("YouTube match failed: %s", err))
		return
	}

	db.DB.Model(&models.SpotifyJob{}).Where("id = ?", job.ID).Updates(map[string]interface{}{
		"youtube_url": ytURL,
		"progress":    10,
	})

	// Phase 2: Download raw audio from YouTube
	absSaveDir := getAbsoluteSavePath(job.SaveDirectory)
	if err := os.MkdirAll(absSaveDir, 0755); err != nil {
		e.failJob(job.ID, fmt.Sprintf("Failed to create directory: %s", err))
		return
	}

	tempDir := filepath.Join(absSaveDir, ".spotify_tmp")
	os.MkdirAll(tempDir, 0755)

	rawPath, _, err := downloadRawAudio(ctx, job.ID, ytURL, tempDir)
	if err != nil {
		e.failJob(job.ID, fmt.Sprintf("Download failed: %s", err))
		return
	}
	defer os.Remove(rawPath)

	// Phase 3: FFmpeg transcode to target format/bitrate
	outputPath := filepath.Join(absSaveDir, job.Filename)

	if err := transcodeAudio(ctx, job.ID, rawPath, outputPath, job.Format, job.Bitrate, job.DurationMs); err != nil {
		e.failJob(job.ID, fmt.Sprintf("Transcoding failed: %s", err))
		return
	}

	// Phase 4: Embed metadata and cover art
	var cfg models.SpotifyConfig
	embedMeta := true
	if err := db.DB.First(&cfg).Error; err == nil {
		embedMeta = cfg.EmbedMetadata
	}

	if embedMeta {
		fullTrack := &TrackMeta{
			ID:          job.SpotifyID,
			Title:       job.Title,
			Artist:      job.Artist,
			Album:       job.Album,
			AlbumArtist: job.AlbumArtist,
			CoverURL:    job.CoverURL,
			ReleaseDate: job.ReleaseDate,
			TrackNumber: job.TrackNumber,
			TotalTracks: job.TotalTracks,
			DiscNumber:  job.DiscNumber,
			DurationMs:  job.DurationMs,
			ISRC:        job.ISRC,
			Genre:       job.Genre,
			Explicit:    job.Explicit,
			Popularity:  job.Popularity,
			SpotifyURL:  job.SpotifyURL,
		}
		json.Unmarshal([]byte(job.Artists), &fullTrack.Artists)
		if len(fullTrack.Artists) == 0 {
			fullTrack.Artists = []string{job.Artist}
		}

		if err := embedMetadata(ctx, job.ID, fullTrack, outputPath, job.Format); err != nil {
			logger.Warn("Spotify", "Metadata embedding failed (file kept)", "id", job.ID, "error", err)
		}
	}

	// Phase 5: Complete
	fi, _ := os.Stat(outputPath)
	finalSize := int64(0)
	if fi != nil {
		finalSize = fi.Size()
	}

	db.DB.Model(&models.SpotifyJob{}).Where("id = ?", job.ID).Updates(map[string]interface{}{
		"status":      "completed",
		"progress":    100,
		"speed":       0,
		"total_bytes": finalSize,
		"downloaded":  finalSize,
	})

	// Cleanup temp dir if empty
	os.Remove(tempDir)

	logger.Info("Spotify", "Download completed",
		"id", job.ID,
		"title", job.Title,
		"artist", job.Artist,
		"format", job.Format,
		"size_mb", fmt.Sprintf("%.1f", float64(finalSize)/(1024*1024)),
	)
}

// failJob marks a job as failed
func (e *Engine) failJob(jobID, errMsg string) {
	db.DB.Model(&models.SpotifyJob{}).Where("id = ?", jobID).Updates(map[string]interface{}{
		"status":        "error",
		"speed":         0,
		"error_message": errMsg,
	})
	logger.Error("Spotify", "Job failed", "id", jobID, "error", errMsg)
}

// startQueueWorker processes pending jobs with concurrency limits
func (e *Engine) startQueueWorker() {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-e.stopChan:
			return
		case <-ticker.C:
			var cfg models.SpotifyConfig
			maxConcurrent := 3
			if err := db.DB.First(&cfg).Error; err == nil && cfg.MaxConcurrent > 0 {
				maxConcurrent = cfg.MaxConcurrent
			}

			var activeCount int64
			db.DB.Model(&models.SpotifyJob{}).
				Where("status IN ?", []string{"downloading", "converting", "tagging", "matching"}).
				Count(&activeCount)

			if int(activeCount) >= maxConcurrent {
				continue
			}

			slots := maxConcurrent - int(activeCount)
			var pendingJobs []models.SpotifyJob
			db.DB.Where("status = ?", "pending").
				Order("created_at asc").
				Limit(slots).
				Find(&pendingJobs)

			for _, job := range pendingJobs {
				e.mu.Lock()
				if _, active := e.activeJobs[job.ID]; active {
					e.mu.Unlock()
					continue
				}
				ctx, cancel := context.WithCancel(context.Background())
				e.activeJobs[job.ID] = cancel
				e.mu.Unlock()

				jobCopy := job
				go e.executeJob(ctx, &jobCopy)
			}
		}
	}
}

// sanitizeFilename removes invalid filename characters
func sanitizeFilename(name string) string {
	replacer := strings.NewReplacer(
		"/", "_", "\\", "_", ":", "_", "*", "_",
		"?", "_", "\"", "_", "<", "_", ">", "_", "|", "_",
	)
	safe := replacer.Replace(name)
	safe = strings.TrimSpace(safe)
	if len(safe) > 200 {
		safe = safe[:200]
	}
	if safe == "" {
		safe = "spotify_track"
	}
	return safe
}
