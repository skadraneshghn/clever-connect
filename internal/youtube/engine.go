package youtube

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"clever-connect/internal/db"
	"clever-connect/internal/logger"
	"clever-connect/internal/models"

	ytdl "github.com/kkdai/youtube/v2"
)

var (
	Manager  *Engine
	initOnce sync.Once
)

// VideoFormat represents a selectable download format from YouTube
type VideoFormat struct {
	ITag         int    `json:"itag"`
	QualityLabel string `json:"quality_label"`
	MimeType     string `json:"mime_type"`
	Bitrate      int    `json:"bitrate"`
	FPS          int    `json:"fps"`
	Width        int    `json:"width"`
	Height       int    `json:"height"`
	ContentLength int64 `json:"content_length"`
	AudioChannels int   `json:"audio_channels"`
	HasAudio     bool   `json:"has_audio"`
	HasVideo     bool   `json:"has_video"`
}

// VideoInfo represents fetched YouTube video metadata
type VideoInfo struct {
	VideoID         string        `json:"video_id"`
	Title           string        `json:"title"`
	Author          string        `json:"author"`
	Duration        string        `json:"duration"`
	DurationSeconds int64         `json:"duration_seconds"`
	Thumbnail       string        `json:"thumbnail"`
	Formats         []VideoFormat `json:"formats"`
}

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
	client     *ytdl.Client
	activeJobs map[string]context.CancelFunc
	mu         sync.RWMutex
	stopChan   chan struct{}
}

// Init initializes the singleton YouTube download engine
func Init() {
	initOnce.Do(func() {
		client := &ytdl.Client{}

		Manager = &Engine{
			client:     client,
			activeJobs: make(map[string]context.CancelFunc),
			stopChan:   make(chan struct{}),
		}

		// Clean up any stale downloading statuses from a previous crash
		db.DB.Model(&models.YouTubeJob{}).Where("status IN ?", []string{"downloading", "fetching", "converting"}).Updates(map[string]interface{}{
			"status": "error",
			"error_message": "Server restarted during operation",
		})

		// Start background queue daemon
		go Manager.startQueueWorker()
		logger.Info("YouTube", "YouTube Download Engine initialized")
	})
}

// Close stops the queue worker
func (e *Engine) Close() {
	close(e.stopChan)
}

// FetchVideoInfo retrieves video metadata and available formats from YouTube
func (e *Engine) FetchVideoInfo(videoURL string) (*VideoInfo, error) {
	video, err := e.client.GetVideo(videoURL)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch video info: %w", err)
	}

	info := &VideoInfo{
		VideoID:         video.ID,
		Title:           video.Title,
		Author:          video.Author,
		Duration:        formatDuration(video.Duration),
		DurationSeconds: int64(video.Duration.Seconds()),
	}

	// Get best thumbnail
	if len(video.Thumbnails) > 0 {
		bestThumb := video.Thumbnails[0]
		for _, t := range video.Thumbnails {
			if t.Width > bestThumb.Width {
				bestThumb = t
			}
		}
		info.Thumbnail = bestThumb.URL
	}

	// Parse available formats - only show combined audio+video formats for simplicity
	seen := make(map[int]bool)
	for _, f := range video.Formats {
		if seen[f.ItagNo] {
			continue
		}
		seen[f.ItagNo] = true

		hasAudio := f.AudioChannels > 0
		hasVideo := f.QualityLabel != ""

		// Only show useful formats
		if !hasVideo && !hasAudio {
			continue
		}

		vf := VideoFormat{
			ITag:          f.ItagNo,
			QualityLabel:  f.QualityLabel,
			MimeType:      f.MimeType,
			Bitrate:       f.Bitrate,
			FPS:           f.FPS,
			Width:         f.Width,
			Height:        f.Height,
			ContentLength: f.ContentLength,
			AudioChannels: f.AudioChannels,
			HasAudio:      hasAudio,
			HasVideo:      hasVideo,
		}
		info.Formats = append(info.Formats, vf)
	}

	return info, nil
}

// AddJob creates a new YouTube download job
func (e *Engine) AddJob(videoURL, saveDir string, selectedITag int, qualityLabel, mimeType string, convertToTV bool, videoInfo *VideoInfo) (string, error) {
	jobID := fmt.Sprintf("yt_%d", time.Now().UnixNano())

	if saveDir == "" {
		var cfg models.YouTubeConfig
		if err := db.DB.First(&cfg).Error; err == nil {
			saveDir = cfg.DefaultSavePath
		} else {
			saveDir = "./downloads/youtube"
		}
	}

	// Sanitize title for filename
	safeTitle := sanitizeFilename(videoInfo.Title)
	ext := ".mp4"
	if strings.Contains(mimeType, "webm") {
		ext = ".webm"
	}
	filename := safeTitle + ext

	job := &models.YouTubeJob{
		ID:              jobID,
		VideoURL:        videoURL,
		VideoID:         videoInfo.VideoID,
		Title:           videoInfo.Title,
		Author:          videoInfo.Author,
		Duration:        videoInfo.Duration,
		DurationSeconds: videoInfo.DurationSeconds,
		Thumbnail:       videoInfo.Thumbnail,
		Filename:        filename,
		SaveDirectory:   saveDir,
		SelectedITag:    selectedITag,
		QualityLabel:    qualityLabel,
		MimeType:        mimeType,
		Status:          "pending",
		ConvertToTV:     convertToTV,
		Progress:        0,
	}

	if err := db.DB.Create(job).Error; err != nil {
		return "", err
	}

	logger.Info("YouTube", "Added new YouTube download job",
		"id", jobID,
		"title", videoInfo.Title,
		"quality", qualityLabel,
		"convertTV", convertToTV,
	)
	return jobID, nil
}

// StartJob initiates downloading a specific YouTube video
func (e *Engine) StartJob(jobID string) error {
	var job models.YouTubeJob
	if err := db.DB.First(&job, "id = ?", jobID).Error; err != nil {
		return err
	}

	e.mu.Lock()
	if _, active := e.activeJobs[jobID]; active {
		e.mu.Unlock()
		return nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	e.activeJobs[jobID] = cancel
	e.mu.Unlock()

	go e.executeDownload(ctx, &job)
	return nil
}

// executeDownload performs the actual video download
func (e *Engine) executeDownload(ctx context.Context, job *models.YouTubeJob) {
	defer func() {
		e.mu.Lock()
		delete(e.activeJobs, job.ID)
		e.mu.Unlock()
	}()

	// Update status to downloading
	db.DB.Model(&models.YouTubeJob{}).Where("id = ?", job.ID).Update("status", "downloading")

	// Fetch video info
	video, err := e.client.GetVideoContext(ctx, job.VideoURL)
	if err != nil {
		e.failJob(job.ID, fmt.Sprintf("Failed to fetch video: %s", err))
		return
	}

	// Find the selected format by itag
	var selectedFormat *ytdl.Format
	for i, f := range video.Formats {
		if f.ItagNo == job.SelectedITag {
			selectedFormat = &video.Formats[i]
			break
		}
	}

	if selectedFormat == nil {
		// Fallback to first format
		if len(video.Formats) > 0 {
			selectedFormat = &video.Formats[0]
		} else {
			e.failJob(job.ID, "No video formats available")
			return
		}
	}

	// Create download directory
	absSaveDir := getAbsoluteSavePath(job.SaveDirectory)
	if err := os.MkdirAll(absSaveDir, 0755); err != nil {
		e.failJob(job.ID, fmt.Sprintf("Failed to create directory: %s", err))
		return
	}

	// Get download stream
	stream, size, err := e.client.GetStreamContext(ctx, video, selectedFormat)
	if err != nil {
		e.failJob(job.ID, fmt.Sprintf("Failed to get stream: %s", err))
		return
	}
	defer stream.Close()

	// Update total bytes
	db.DB.Model(&models.YouTubeJob{}).Where("id = ?", job.ID).Update("total_bytes", size)

	// Build final destination path
	destPath := filepath.Join(absSaveDir, job.Filename)

	// If converting to TV, download to a temp file first
	downloadPath := destPath
	if job.ConvertToTV {
		downloadPath = destPath + ".ytdl_temp"
	}

	// Create output file
	outFile, err := os.Create(downloadPath)
	if err != nil {
		e.failJob(job.ID, fmt.Sprintf("Failed to create output file: %s", err))
		return
	}

	// Download with progress tracking
	downloaded := int64(0)
	lastUpdate := time.Now()
	lastBytes := int64(0)
	buf := make([]byte, 256*1024) // 256KB buffer for high speed

	for {
		select {
		case <-ctx.Done():
			outFile.Close()
			os.Remove(downloadPath)
			db.DB.Model(&models.YouTubeJob{}).Where("id = ?", job.ID).Updates(map[string]interface{}{
				"status": "error",
				"speed":  0,
				"error_message": "Download cancelled",
			})
			return
		default:
		}

		n, readErr := stream.Read(buf)
		if n > 0 {
			if _, writeErr := outFile.Write(buf[:n]); writeErr != nil {
				outFile.Close()
				e.failJob(job.ID, fmt.Sprintf("Write error: %s", writeErr))
				return
			}
			downloaded += int64(n)

			// Update progress every 500ms
			if time.Since(lastUpdate) > 500*time.Millisecond {
				elapsed := time.Since(lastUpdate).Seconds()
				speed := float64(downloaded-lastBytes) / elapsed / (1024 * 1024) // MB/s

				progress := float64(0)
				if size > 0 {
					progress = float64(downloaded) / float64(size) * 100
				}

				db.DB.Model(&models.YouTubeJob{}).Where("id = ?", job.ID).Updates(map[string]interface{}{
					"downloaded": downloaded,
					"progress":   progress,
					"speed":      speed,
					"status":     "downloading",
				})

				lastUpdate = time.Now()
				lastBytes = downloaded
			}
		}

		if readErr != nil {
			if readErr == io.EOF {
				break
			}
			outFile.Close()
			e.failJob(job.ID, fmt.Sprintf("Read error: %s", readErr))
			return
		}
	}

	outFile.Close()

	// Final progress update
	db.DB.Model(&models.YouTubeJob{}).Where("id = ?", job.ID).Updates(map[string]interface{}{
		"downloaded": downloaded,
		"progress":   100.0,
		"speed":      0,
	})

	// Check if we need TV conversion
	if job.ConvertToTV {
		logger.Info("YouTube", "Starting TV conversion for Sony Bravia", "id", job.ID, "title", job.Title)
		db.DB.Model(&models.YouTubeJob{}).Where("id = ?", job.ID).Updates(map[string]interface{}{
			"status":         "converting",
			"convert_status": "converting",
		})

		err := e.convertForTV(ctx, job.ID, downloadPath, destPath, job.DurationSeconds)
		if err != nil {
			// Keep the original file if conversion fails
			os.Rename(downloadPath, destPath)
			db.DB.Model(&models.YouTubeJob{}).Where("id = ?", job.ID).Updates(map[string]interface{}{
				"status":          "completed",
				"convert_status":  "error",
				"error_message":   fmt.Sprintf("Conversion failed (original file kept): %s", err),
				"convert_progress": 0,
			})
			logger.Error("YouTube", "TV conversion failed", "id", job.ID, "error", err)
			return
		}

		// Remove temp file after successful conversion
		os.Remove(downloadPath)
		db.DB.Model(&models.YouTubeJob{}).Where("id = ?", job.ID).Updates(map[string]interface{}{
			"convert_status":   "completed",
			"convert_progress": 100.0,
		})
	}

	// Mark completed
	db.DB.Model(&models.YouTubeJob{}).Where("id = ?", job.ID).Updates(map[string]interface{}{
		"status":   "completed",
		"speed":    0,
		"progress": 100.0,
	})

	logger.Info("YouTube", "Download completed successfully", "id", job.ID, "title", job.Title)
}

// convertForTV transcodes video to Sony Bravia 46W700A compatible format using FFmpeg
// Container: MP4, Video: H.264 AVC High@4.0, Resolution: 1920×1080 or lower, Audio: AAC-LC stereo or AAC 5.1
// Uses all CPU cores for maximum parallel encoding speed
func (e *Engine) convertForTV(ctx context.Context, jobID, inputPath, outputPath string, durationSec int64) error {
	// Check if ffmpeg is available
	ffmpegPath, err := exec.LookPath("ffmpeg")
	if err != nil {
		return fmt.Errorf("ffmpeg is not installed or not in PATH")
	}

	// Number of CPU threads for parallel encoding
	numThreads := runtime.NumCPU()
	if numThreads < 1 {
		numThreads = 1
	}

	// Build FFmpeg command for Sony Bravia 46W700A compatibility
	// H.264 AVC High Profile Level 4.0, AAC-LC stereo, MP4 container
	args := []string{
		"-y",                          // Overwrite output
		"-i", inputPath,               // Input file
		"-c:v", "libx264",            // H.264 AVC codec
		"-profile:v", "high",          // High profile
		"-level:v", "4.0",            // Level 4.0
		"-preset", "fast",             // Fast preset for speed (good balance)
		"-crf", "18",                  // High quality (lower = better, 18 is visually lossless)
		"-maxrate", "20M",            // Max bitrate for TV compatibility
		"-bufsize", "25M",            // Buffer size
		"-pix_fmt", "yuv420p",        // Pixel format for maximum compatibility
		"-vf", "scale='min(1920,iw)':'min(1080,ih)':force_original_aspect_ratio=decrease,pad=ceil(iw/2)*2:ceil(ih/2)*2", // Scale down to 1080p max
		"-c:a", "aac",                // AAC audio codec
		"-ac", "2",                    // Stereo (AAC-LC stereo for max compatibility)
		"-b:a", "192k",              // Audio bitrate
		"-ar", "48000",               // Audio sample rate
		"-movflags", "+faststart",    // Enable fast start for streaming
		"-threads", strconv.Itoa(numThreads), // Use all CPU cores
		"-progress", "pipe:1",        // Output progress to stdout
		outputPath,                    // Output file
	}

	cmd := exec.CommandContext(ctx, ffmpegPath, args...)
	cmd.Stderr = nil // Suppress stderr

	// Capture stdout for progress parsing
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start ffmpeg: %w", err)
	}

	// Parse progress from ffmpeg output
	go func() {
		scanner := bufio.NewScanner(stdout)
		progressRegex := regexp.MustCompile(`out_time_us=(\d+)`)

		for scanner.Scan() {
			line := scanner.Text()
			matches := progressRegex.FindStringSubmatch(line)
			if len(matches) > 1 {
				timeUs, err := strconv.ParseInt(matches[1], 10, 64)
				if err == nil && durationSec > 0 {
					progress := float64(timeUs) / float64(durationSec*1000000) * 100
					if progress > 100 {
						progress = 100
					}
					db.DB.Model(&models.YouTubeJob{}).Where("id = ?", jobID).Update("convert_progress", progress)
				}
			}
		}
	}()

	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("ffmpeg conversion failed: %w", err)
	}

	return nil
}

// PauseJob cancels an active download
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

	db.DB.Model(&models.YouTubeJob{}).Where("id = ?", jobID).Updates(map[string]interface{}{
		"status": "error",
		"speed":  0,
		"error_message": "Cancelled by user",
	})
	logger.Info("YouTube", "Job cancelled", "id", jobID)
}

// DeleteJob cancels and deletes download records/files
func (e *Engine) DeleteJob(jobID string, deleteFiles bool) {
	e.PauseJob(jobID)

	var job models.YouTubeJob
	if err := db.DB.First(&job, "id = ?", jobID).Error; err == nil {
		if deleteFiles {
			absSaveDir := getAbsoluteSavePath(job.SaveDirectory)
			destPath := filepath.Join(absSaveDir, job.Filename)
			_ = os.Remove(destPath)
			_ = os.Remove(destPath + ".ytdl_temp")
		}
		db.DB.Unscoped().Delete(&job)
	}
	logger.Info("YouTube", "Job deleted", "id", jobID, "deleteFiles", deleteFiles)
}

// failJob sets an error status on a job
func (e *Engine) failJob(jobID, errMsg string) {
	db.DB.Model(&models.YouTubeJob{}).Where("id = ?", jobID).Updates(map[string]interface{}{
		"status":        "error",
		"speed":         0,
		"error_message": errMsg,
	})
	logger.Error("YouTube", "Job failed", "id", jobID, "error", errMsg)
}

// startQueueWorker runs a background loop checking for pending downloads
func (e *Engine) startQueueWorker() {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-e.stopChan:
			return
		case <-ticker.C:
			// Fetch configuration limits
			var cfg models.YouTubeConfig
			maxConcurrent := 2
			if err := db.DB.First(&cfg).Error; err == nil {
				maxConcurrent = cfg.MaxConcurrent
			}

			// Get active downloads count
			var activeCount int64
			db.DB.Model(&models.YouTubeJob{}).Where("status IN ?", []string{"downloading", "converting"}).Count(&activeCount)

			// If we have free slots, start pending jobs
			if int(activeCount) < maxConcurrent {
				slotsAvailable := maxConcurrent - int(activeCount)
				var pendingJobs []models.YouTubeJob
				db.DB.Where("status = ?", "pending").Order("created_at asc").Limit(slotsAvailable).Find(&pendingJobs)

				for _, job := range pendingJobs {
					logger.Info("YouTube", "Queue starting pending job", "id", job.ID, "title", job.Title)
					if err := e.StartJob(job.ID); err != nil {
						e.failJob(job.ID, err.Error())
					}
				}
			}
		}
	}
}

// Helper functions

func formatDuration(d time.Duration) string {
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	s := int(d.Seconds()) % 60

	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%d:%02d", m, s)
}

func sanitizeFilename(name string) string {
	// Remove invalid filename characters
	replacer := strings.NewReplacer(
		"/", "_",
		"\\", "_",
		":", "_",
		"*", "_",
		"?", "_",
		"\"", "_",
		"<", "_",
		">", "_",
		"|", "_",
	)
	safe := replacer.Replace(name)

	// Trim whitespace and limit length
	safe = strings.TrimSpace(safe)
	if len(safe) > 200 {
		safe = safe[:200]
	}
	if safe == "" {
		safe = "youtube_video"
	}
	return safe
}
