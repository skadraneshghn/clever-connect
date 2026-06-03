package spotify

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	"clever-connect/internal/db"
	"clever-connect/internal/logger"
	"clever-connect/internal/models"

	ytdl "github.com/kkdai/youtube/v2"
)

// ──────────────────────────────────────────────────────────────────────────────
// Download Pipeline — YT Music Matching → Raw Download → FFmpeg Transcode → ID3 Tag
// Replicates the spotDL algorithm: Spotify metadata + YouTube audio + FFmpeg conversion
// ──────────────────────────────────────────────────────────────────────────────

// searchYouTube performs a keyless search query on YouTube and returns the first matching video ID
func searchYouTube(ctx context.Context, query string) (string, error) {
	searchURL := "https://www.youtube.com/results?search_query=" + url.QueryEscape(query)

	req, err := http.NewRequestWithContext(ctx, "GET", searchURL, nil)
	if err != nil {
		return "", err
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected search status code: %d", resp.StatusCode)
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	body := string(bodyBytes)

	// Regex to extract video IDs from search results: /watch?v=XXXXXXXXXXX
	re := regexp.MustCompile(`/watch\?v=([a-zA-Z0-9_-]{11})`)
	matches := re.FindAllStringSubmatch(body, -1)
	for _, match := range matches {
		if len(match) > 1 && len(match[1]) == 11 {
			return match[1], nil
		}
	}

	// Fallback regex looking for videoId key in YouTube's JSON payload
	reJSON := regexp.MustCompile(`"videoId"\s*:\s*"([a-zA-Z0-9_-]{11})"`)
	matchesJSON := reJSON.FindAllStringSubmatch(body, -1)
	for _, match := range matchesJSON {
		if len(match) > 1 && len(match[1]) == 11 {
			return match[1], nil
		}
	}

	return "", fmt.Errorf("no videos found in YouTube search results for query: %s", query)
}

// matchYouTube searches YouTube Music for a matching audio using ISRC or title+artist
func matchYouTube(ctx context.Context, track *TrackMeta) (string, error) {
	// Strategy 1: Search by ISRC (highest accuracy, exactly like spotDL)
	if track.ISRC != "" {
		videoID, err := searchYouTube(ctx, track.ISRC)
		if err == nil && videoID != "" {
			url := "https://www.youtube.com/watch?v=" + videoID
			logger.Info("Spotify", "ISRC match found on YouTube", "isrc", track.ISRC, "yt_id", videoID)
			return url, nil
		}
		logger.Warn("Spotify", "ISRC match failed, falling back to title search", "isrc", track.ISRC, "error", err)
	}

	// Strategy 2: Search by "Artist - Title" (fallback, like spotDL)
	searchQuery := fmt.Sprintf("%s - %s", track.Artist, track.Title)
	videoID, err := searchYouTube(ctx, searchQuery)
	if err != nil {
		return "", fmt.Errorf("no YouTube match found for %s: %w", searchQuery, err)
	}

	ytURL := "https://www.youtube.com/watch?v=" + videoID
	logger.Info("Spotify", "Title match found on YouTube", "query", searchQuery, "yt_id", videoID)
	return ytURL, nil
}

// downloadRawAudio downloads the best audio stream from YouTube into a temp file
func downloadRawAudio(ctx context.Context, jobID, ytURL, tempDir string) (string, int64, error) {
	client := &ytdl.Client{}

	video, err := client.GetVideoContext(ctx, ytURL)
	if err != nil {
		return "", 0, fmt.Errorf("failed to fetch YouTube video: %w", err)
	}

	// Find best audio-only format (like spotDL's "bestaudio")
	var bestFormat *ytdl.Format
	bestBitrate := 0
	for i, f := range video.Formats {
		if f.AudioChannels > 0 && f.QualityLabel == "" {
			if f.Bitrate > bestBitrate {
				bestBitrate = f.Bitrate
				bestFormat = &video.Formats[i]
			}
		}
	}

	// Fallback: any format with audio
	if bestFormat == nil {
		for i, f := range video.Formats {
			if f.AudioChannels > 0 {
				if f.Bitrate > bestBitrate {
					bestBitrate = f.Bitrate
					bestFormat = &video.Formats[i]
				}
			}
		}
	}

	if bestFormat == nil {
		return "", 0, fmt.Errorf("no audio format available")
	}

	stream, size, err := client.GetStreamContext(ctx, video, bestFormat)
	if err != nil {
		return "", 0, fmt.Errorf("failed to get audio stream: %w", err)
	}
	defer stream.Close()

	// Determine temp file extension
	ext := ".webm"
	if strings.Contains(bestFormat.MimeType, "mp4") || strings.Contains(bestFormat.MimeType, "m4a") {
		ext = ".m4a"
	}

	tempPath := filepath.Join(tempDir, jobID+"_raw"+ext)
	outFile, err := os.Create(tempPath)
	if err != nil {
		return "", 0, fmt.Errorf("failed to create temp file: %w", err)
	}

	// Download with progress tracking
	downloaded := int64(0)
	lastUpdate := time.Now()
	lastBytes := int64(0)
	buf := make([]byte, 256*1024)

	for {
		select {
		case <-ctx.Done():
			outFile.Close()
			os.Remove(tempPath)
			return "", 0, ctx.Err()
		default:
		}

		n, readErr := stream.Read(buf)
		if n > 0 {
			if _, writeErr := outFile.Write(buf[:n]); writeErr != nil {
				outFile.Close()
				return "", 0, writeErr
			}
			downloaded += int64(n)

			if time.Since(lastUpdate) > 500*time.Millisecond {
				elapsed := time.Since(lastUpdate).Seconds()
				speed := float64(downloaded-lastBytes) / elapsed / (1024 * 1024)
				progress := float64(0)
				if size > 0 {
					progress = float64(downloaded) / float64(size) * 50 // Download = 0-50%
				}

				db.DB.Model(&models.SpotifyJob{}).Where("id = ?", jobID).Updates(map[string]interface{}{
					"downloaded": downloaded,
					"total_bytes": size,
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
			return "", 0, readErr
		}
	}

	outFile.Close()
	return tempPath, downloaded, nil
}

// transcodeAudio converts raw audio to target format/bitrate using FFmpeg
// This is the core of spotDL's convert() function
func transcodeAudio(ctx context.Context, jobID, inputPath, outputPath, format, bitrate string, durationMs int) error {
	ffmpegPath, err := exec.LookPath("ffmpeg")
	if err != nil {
		return fmt.Errorf("ffmpeg not found in PATH")
	}

	numThreads := runtime.NumCPU()
	if numThreads < 1 {
		numThreads = 1
	}

	// Build codec args based on format (mirrors spotDL's ffmpeg.py)
	args := []string{"-y", "-i", inputPath}

	switch format {
	case "mp3":
		args = append(args, "-c:a", "libmp3lame", "-q:a", "0")
		if bitrate != "" && bitrate != "auto" {
			args = append(args, "-b:a", bitrate)
		}
	case "flac":
		args = append(args, "-c:a", "flac", "-compression_level", "8")
	case "opus":
		args = append(args, "-c:a", "libopus")
		if bitrate != "" && bitrate != "auto" {
			args = append(args, "-b:a", bitrate)
		}
	case "m4a":
		args = append(args, "-c:a", "aac", "-vn")
		if bitrate != "" && bitrate != "auto" {
			args = append(args, "-b:a", bitrate)
		}
	case "ogg":
		args = append(args, "-c:a", "libvorbis")
		if bitrate != "" && bitrate != "auto" {
			args = append(args, "-b:a", bitrate)
		}
	case "wav":
		args = append(args, "-c:a", "pcm_s16le")
	default:
		args = append(args, "-c:a", "libmp3lame", "-b:a", "320k")
	}

	args = append(args,
		"-threads", strconv.Itoa(numThreads),
		"-progress", "pipe:1",
		outputPath,
	)

	cmd := exec.CommandContext(ctx, ffmpegPath, args...)
	cmd.Stderr = nil

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("ffmpeg stdout pipe failed: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("ffmpeg start failed: %w", err)
	}

	// Parse FFmpeg progress (50-90% range)
	go func() {
		scanner := bufio.NewScanner(stdout)
		progressRegex := regexp.MustCompile(`out_time_us=(\d+)`)
		for scanner.Scan() {
			line := scanner.Text()
			matches := progressRegex.FindStringSubmatch(line)
			if len(matches) > 1 && durationMs > 0 {
				timeUs, err := strconv.ParseInt(matches[1], 10, 64)
				if err == nil {
					durationUs := int64(durationMs) * 1000
					progress := 50 + (float64(timeUs)/float64(durationUs))*40
					if progress > 90 {
						progress = 90
					}
					db.DB.Model(&models.SpotifyJob{}).Where("id = ?", jobID).Updates(map[string]interface{}{
						"progress": progress,
						"status":   "converting",
					})
				}
			}
		}
	}()

	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("ffmpeg conversion failed: %w", err)
	}

	return nil
}

// embedMetadata injects Spotify metadata and cover art into the output file using FFmpeg
// This replicates spotDL's embed_metadata function
func embedMetadata(ctx context.Context, jobID string, track *TrackMeta, audioPath, format string) error {
	if format == "wav" {
		// WAV doesn't support metadata well via ffmpeg, skip
		return nil
	}

	db.DB.Model(&models.SpotifyJob{}).Where("id = ?", jobID).Updates(map[string]interface{}{
		"progress": 90,
		"status":   "tagging",
	})

	ffmpegPath, err := exec.LookPath("ffmpeg")
	if err != nil {
		return fmt.Errorf("ffmpeg not found")
	}

	// Download cover art to temp file
	var coverPath string
	if track.CoverURL != "" {
		coverResp, err := http.Get(track.CoverURL)
		if err == nil && coverResp.StatusCode == 200 {
			defer coverResp.Body.Close()
			coverPath = audioPath + "_cover.jpg"
			coverFile, err := os.Create(coverPath)
			if err == nil {
				io.Copy(coverFile, coverResp.Body)
				coverFile.Close()
				defer os.Remove(coverPath)
			}
		}
	}

	// Build metadata injection command
	tempTagged := audioPath + "_tagged" + filepath.Ext(audioPath)
	artistsJSON, _ := json.Marshal(track.Artists)

	args := []string{"-y", "-i", audioPath}

	// Add cover art as input
	if coverPath != "" {
		args = append(args, "-i", coverPath)
	}

	args = append(args, "-map", "0:a")
	if coverPath != "" {
		args = append(args, "-map", "1:v", "-c:v", "mjpeg", "-disposition:v:0", "attached_pic")
	}

	args = append(args, "-c:a", "copy") // Copy audio, only inject metadata

	// Add metadata tags (like spotDL's ID3 injection)
	args = append(args,
		"-metadata", fmt.Sprintf("title=%s", track.Title),
		"-metadata", fmt.Sprintf("artist=%s", strings.Join(track.Artists, ", ")),
		"-metadata", fmt.Sprintf("album=%s", track.Album),
		"-metadata", fmt.Sprintf("album_artist=%s", track.AlbumArtist),
		"-metadata", fmt.Sprintf("date=%s", track.ReleaseDate),
		"-metadata", fmt.Sprintf("track=%d/%d", track.TrackNumber, track.TotalTracks),
		"-metadata", fmt.Sprintf("disc=%d", track.DiscNumber),
		"-metadata", fmt.Sprintf("genre=%s", track.Genre),
		"-metadata", fmt.Sprintf("comment=Downloaded by CleverConnect | Artists: %s", string(artistsJSON)),
	)

	if track.ISRC != "" {
		args = append(args, "-metadata", fmt.Sprintf("isrc=%s", track.ISRC))
	}

	args = append(args, tempTagged)

	cmd := exec.CommandContext(ctx, ffmpegPath, args...)
	if output, err := cmd.CombinedOutput(); err != nil {
		logger.Warn("Spotify", "Metadata embedding failed (keeping untagged file)", "error", err, "output", string(output))
		return nil // Non-fatal: keep the audio even if tagging fails
	}

	// Replace original with tagged version
	os.Remove(audioPath)
	if err := os.Rename(tempTagged, audioPath); err != nil {
		return fmt.Errorf("failed to replace with tagged file: %w", err)
	}

	db.DB.Model(&models.SpotifyJob{}).Where("id = ?", jobID).Update("progress", 95)
	return nil
}
