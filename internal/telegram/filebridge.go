package telegram

import (
	"context"
	"fmt"
	"io"
	"math"
	"mime"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"clever-connect/internal/logger"

	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/downloader"
	"github.com/gotd/td/telegram/uploader"
	"github.com/gotd/td/tg"
	tele "gopkg.in/telebot.v4"
)

// QueueUploadJob is a callback registered by the scheduler engine to queue Telegram upload jobs.
var QueueUploadJob func(filePath string, chatID int64) error

// fileManagerRoot is the base directory for the server file manager.
// This matches the FileHandler's rootDir in handlers/files.go.
var fileManagerRoot string

func init() {
	root, err := filepath.Abs("./data/manager")
	if err != nil {
		root = "./data/manager"
	}
	fileManagerRoot = root
}

// securePath ensures path stays within the file manager sandbox.
func securePath(requestedPath string) (string, error) {
	cleanRel := filepath.Clean("/" + requestedPath)
	fullPath := filepath.Clean(filepath.Join(fileManagerRoot, cleanRel))

	if !strings.HasPrefix(fullPath, fileManagerRoot) {
		return "", os.ErrPermission
	}
	return fullPath, nil
}

// handleFileBrowse sends an inline keyboard listing directory contents.
func (e *Engine) handleFileBrowse(c tele.Context, dirPath string) error {
	safePath, err := securePath(dirPath)
	if err != nil {
		return c.Send("⛔ Access denied: invalid path.")
	}

	entries, err := os.ReadDir(safePath)
	if err != nil {
		return c.Send("❌ Failed to read directory: " + err.Error())
	}

	// Sort: directories first, then files
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].IsDir() != entries[j].IsDir() {
			return entries[i].IsDir()
		}
		return entries[i].Name() < entries[j].Name()
	})

	displayPath := filepath.Clean("/" + dirPath)
	if displayPath == "." {
		displayPath = "/"
	}

	text := fmt.Sprintf("📁 *File Browser*\n📂 `%s`\n\n", displayPath)
	if len(entries) == 0 {
		text += "_Empty directory_"
	} else {
		text += fmt.Sprintf("_%d items_", len(entries))
	}

	// Build inline keyboard
	rows := []tele.Row{}

	// Parent directory button (if not at root)
	if dirPath != "/" && dirPath != "" {
		parentPath := filepath.Dir(dirPath)
		if parentPath == "." {
			parentPath = "/"
		}
		rows = append(rows, tele.Row{
			{Text: "⬆️ Parent Directory", Data: "fb:" + parentPath},
		})
	}

	// Limit to 30 items to avoid Telegram message limits
	maxItems := 30
	if len(entries) < maxItems {
		maxItems = len(entries)
	}

	for _, entry := range entries[:maxItems] {
		name := entry.Name()
		entryPath := filepath.Join(dirPath, name)
		if dirPath == "/" {
			entryPath = "/" + name
		}

		if entry.IsDir() {
			rows = append(rows, tele.Row{
				{Text: "📂 " + name, Data: "fb:" + entryPath},
			})
		} else {
			info, _ := entry.Info()
			sizeStr := formatFileSize(info.Size())
			icon := getFileIcon(name)
			rows = append(rows, tele.Row{
				{Text: fmt.Sprintf("%s %s (%s)", icon, name, sizeStr), Data: "send:" + entryPath},
			})
		}
	}

	if len(entries) > 30 {
		text += fmt.Sprintf("\n\n⚠️ _Showing first 30 of %d items_", len(entries))
	}

	markup := &tele.ReplyMarkup{}
	markup.InlineKeyboard = make([][]tele.InlineButton, len(rows))
	for i, row := range rows {
		markup.InlineKeyboard[i] = make([]tele.InlineButton, len(row))
		for j, btn := range row {
			markup.InlineKeyboard[i][j] = tele.InlineButton{
				Text: btn.Text,
				Data: btn.Data,
			}
		}
	}

	// If this is a callback, edit the existing message
	if c.Callback() != nil {
		return c.Edit(text, markup, tele.ModeMarkdown)
	}
	return c.Send(text, markup, tele.ModeMarkdown)
}

// sendFileToChat reads a file and sends it via the Telegram bot,
// automatically detecting whether it should be sent as a photo, video,
// audio, or generic document.
func (e *Engine) sendFileToChat(c tele.Context, filePath string) error {
	safePath, err := securePath(filePath)
	if err != nil {
		return c.Send("⛔ Access denied.")
	}

	info, err := os.Stat(safePath)
	if err != nil {
		return c.Send("❌ File not found: " + filepath.Base(filePath))
	}

	if info.IsDir() {
		return c.Send("📂 That's a directory, not a file. Use /files to browse.")
	}

	// Check file size (Telegram limit: 50MB for bots)
	e.mu.RLock()
	maxSizeMB := e.Config.MaxFileSize
	e.mu.RUnlock()
	if maxSizeMB <= 0 {
		maxSizeMB = 2000
	}
	maxBytes := int64(maxSizeMB) * 1024 * 1024

	if info.Size() > maxBytes {
		return c.Send(fmt.Sprintf("❌ File too large (%s). Maximum allowed: %dMB.",
			formatFileSize(info.Size()), maxSizeMB))
	}

	// Telegram standard Bot API limit is 50MB. If file is larger, upload via MTProto parallel uploader.
	if info.Size() > 50*1024*1024 {
		if QueueUploadJob != nil {
			err := QueueUploadJob(filePath, c.Chat().ID)
			if err != nil {
				return c.Send("❌ Failed to queue parallel upload: " + err.Error())
			}
			return nil // The job handles progress/completion notifications
		}
	}

	fileName := filepath.Base(safePath)
	ext := strings.ToLower(filepath.Ext(fileName))
	mimeType := mime.TypeByExtension(ext)

	caption := fmt.Sprintf("📁 %s\n📏 %s", filePath, formatFileSize(info.Size()))

	fileObj := tele.FromDisk(safePath)

	var sendErr error

	switch {
	case isImageExt(ext):
		photo := &tele.Photo{File: fileObj, Caption: caption}
		sendErr = c.Send(photo, tele.ModeMarkdown)

	case isVideoExt(ext):
		video := &tele.Video{File: fileObj, Caption: caption}
		sendErr = c.Send(video, tele.ModeMarkdown)

	case isAudioExt(ext):
		audio := &tele.Audio{File: fileObj, Caption: caption}
		sendErr = c.Send(audio, tele.ModeMarkdown)

	case isVoiceExt(ext):
		voice := &tele.Voice{File: fileObj, Caption: caption}
		sendErr = c.Send(voice, tele.ModeMarkdown)

	default:
		doc := &tele.Document{File: fileObj, Caption: caption, MIME: mimeType}
		sendErr = c.Send(doc, tele.ModeMarkdown)
	}

	if sendErr != nil {
		e.errors.Add(1)
		logger.Error("Telegram", "Failed to send file", "path", filePath, "error", sendErr)
		return c.Send("❌ Failed to send file: " + sendErr.Error())
	}

	e.filesSent.Add(1)
	logger.Info("Telegram", "File sent successfully", "path", filePath, "size", info.Size())
	return nil
}

// ──────────────────────────────────────────────────────────────
// File type helpers
// ──────────────────────────────────────────────────────────────

func isImageExt(ext string) bool {
	switch ext {
	case ".jpg", ".jpeg", ".png", ".gif", ".bmp", ".webp":
		return true
	}
	return false
}

func isVideoExt(ext string) bool {
	switch ext {
	case ".mp4", ".mkv", ".avi", ".mov", ".wmv", ".webm", ".flv", ".m4v":
		return true
	}
	return false
}

func isAudioExt(ext string) bool {
	switch ext {
	case ".mp3", ".flac", ".wav", ".aac", ".m4a", ".wma":
		return true
	}
	return false
}

func isVoiceExt(ext string) bool {
	switch ext {
	case ".ogg", ".oga":
		return true
	}
	return false
}

func getFileIcon(name string) string {
	ext := strings.ToLower(filepath.Ext(name))
	switch {
	case isImageExt(ext):
		return "🖼"
	case isVideoExt(ext):
		return "🎬"
	case isAudioExt(ext):
		return "🎵"
	case ext == ".pdf":
		return "📄"
	case ext == ".zip" || ext == ".tar" || ext == ".gz" || ext == ".rar" || ext == ".7z":
		return "📦"
	case ext == ".go" || ext == ".py" || ext == ".js" || ext == ".ts" || ext == ".tsx":
		return "💻"
	case ext == ".json" || ext == ".yaml" || ext == ".yml" || ext == ".toml":
		return "📋"
	case ext == ".txt" || ext == ".md" || ext == ".log":
		return "📝"
	default:
		return "📄"
	}
}

func formatFileSize(bytes int64) string {
	const (
		KB = 1024
		MB = 1024 * KB
		GB = 1024 * MB
	)
	switch {
	case bytes >= GB:
		return fmt.Sprintf("%.1f GB", float64(bytes)/float64(GB))
	case bytes >= MB:
		return fmt.Sprintf("%.1f MB", float64(bytes)/float64(MB))
	case bytes >= KB:
		return fmt.Sprintf("%.1f KB", float64(bytes)/float64(KB))
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}

// ──────────────────────────────────────────────────────────────
// High-Performance MTProto Transfer Functions
// Inspired by devgagantools ParallelTransferrer
// ──────────────────────────────────────────────────────────────

// calculateOptimalThreads determines the optimal number of concurrent upload/download
// goroutines based on file size. This mirrors devgagantools' get_appropriated_part_size logic:
//   - Files < 100MB  → 1 thread  (no parallelism needed)
//   - Files 100-500MB → 4 threads
//   - Files 500MB-1GB → 8 threads
//   - Files 1-2GB    → 16 threads
//   - Files > 2GB    → 20 threads (maximum for MTProto)
func calculateOptimalThreads(fileSize int64) int {
	const (
		MB100 = 100 * 1024 * 1024
		MB500 = 500 * 1024 * 1024
		GB1   = 1024 * 1024 * 1024
		GB2   = 2 * GB1
	)

	switch {
	case fileSize < MB100:
		return 1
	case fileSize < MB500:
		return 4
	case fileSize < GB1:
		return 8
	case fileSize < GB2:
		return 16
	default:
		// Cap at 20 — the practical limit before Telegram rate-limits
		threads := int(math.Ceil(float64(fileSize) / float64(MB100)))
		if threads > 20 {
			threads = 20
		}
		return threads
	}
}

// FastUploadFile uploads a file using concurrent goroutines via the gotd MTProto uploader.
// It automatically calculates optimal threads based on file size.
// The progress parameter is optional — pass nil to skip progress tracking.
func FastUploadFile(ctx context.Context, client *telegram.Client, filePath string, progress uploader.Progress) (tg.InputFileClass, error) {
	info, err := os.Stat(filePath)
	if err != nil {
		return nil, fmt.Errorf("file not found: %w", err)
	}

	threads := calculateOptimalThreads(info.Size())
	api := tg.NewClient(client)

	up := uploader.NewUploader(api).
		WithThreads(threads).
		WithPartSize(512 * 1024) // 512KB chunks — maximum for speed

	if progress != nil {
		up = up.WithProgress(progress)
	}

	logger.Info("Telegram", "Starting fast parallel upload",
		"file", filepath.Base(filePath),
		"size", formatFileSize(info.Size()),
		"threads", threads,
	)

	inputFile, err := up.FromPath(ctx, filePath)
	if err != nil {
		return nil, fmt.Errorf("parallel upload failed: %w", err)
	}

	logger.Info("Telegram", "Fast parallel upload completed",
		"file", filepath.Base(filePath),
		"threads", threads,
	)

	return inputFile, nil
}

// FastDownloadFile downloads a file from Telegram using concurrent goroutines via the
// gotd MTProto downloader. It streams chunks directly to the local filesystem.
func FastDownloadFile(ctx context.Context, client *telegram.Client, fileLocation tg.InputFileLocationClass, destPath string, fileSize int64) error {
	api := tg.NewClient(client)
	threads := calculateOptimalThreads(fileSize)

	dl := downloader.NewDownloader().WithPartSize(512 * 1024)

	logger.Info("Telegram", "Starting fast parallel download",
		"dest", filepath.Base(destPath),
		"threads", threads,
	)

	// Ensure destination directory exists
	if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
		return fmt.Errorf("failed to create download directory: %w", err)
	}

	_, err := dl.Download(api, fileLocation).ToPath(ctx, destPath)
	if err != nil {
		return fmt.Errorf("parallel download failed: %w", err)
	}

	logger.Info("Telegram", "Fast parallel download completed", "dest", filepath.Base(destPath))
	return nil
}

// ProgressReader wraps an io.Reader to track bytes read and report progress.
// This is useful for streaming uploads with real-time progress bars in the Web UI.
type ProgressReader struct {
	Reader    io.Reader
	Total     int64
	Read_     int64
	OnUpdate  func(bytesRead, totalBytes int64) // Called periodically
}

func (pr *ProgressReader) Read(p []byte) (int, error) {
	n, err := pr.Reader.Read(p)
	pr.Read_ += int64(n)
	if pr.OnUpdate != nil {
		pr.OnUpdate(pr.Read_, pr.Total)
	}
	return n, err
}
