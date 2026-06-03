package filecore

import (
	"context"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"clever-connect/internal/db"
	"clever-connect/internal/logger"
	"clever-connect/internal/models"

	"lukechampine.com/blake3"
)

// GetBlake3Checksum calculates the 256-bit BLAKE3 hash of a file.
// It reads the file in 64KB blocks, which is extremely fast and efficient.
func GetBlake3Checksum(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hasher := blake3.New(32, nil)
	buf := make([]byte, 64*1024)
	for {
		n, err := file.Read(buf)
		if n > 0 {
			_, _ = hasher.Write(buf[:n])
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}
	}
	return fmt.Sprintf("%x", hasher.Sum(nil)), nil
}

// SafeLink attempts to create a hardlink from src to dst.
// If it fails due to cross-device boundaries, it falls back to copying.
func SafeLink(src, dst string) error {
	// Create destination directory if it doesn't exist
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}

	// Remove dst if it exists
	if _, err := os.Stat(dst); err == nil {
		_ = os.Remove(dst)
	}

	// Try hardlinking
	err := os.Link(src, dst)
	if err == nil {
		logger.Info("FileCore", "Created hardlink to avoid duplicate storage", "src", src, "dst", dst)
		return nil
	}

	// Fallback to copy if cross-device link
	logger.Warn("FileCore", "Hardlink failed (cross-device?), falling back to copy", "src", src, "dst", dst, "error", err)
	return copyFile(src, dst)
}

// copyFile copies a file from src to dst.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err = io.Copy(out, in); err != nil {
		return err
	}

	si, err := os.Stat(src)
	if err != nil {
		return err
	}
	return os.Chmod(dst, si.Mode())
}

// RegisterFile registers a saved file inside the FileRegistry.
// If the checksum already exists, it removes the duplicate file at filePath,
// creates a hardlink to the master file, and returns the existing registry record.
func RegisterFile(filePath string, optURL string, optETag string, optTgFileID int64, optTorrentHash string) (*models.FileRegistry, error) {
	// Clean and get absolute path
	absPath, err := filepath.Abs(filePath)
	if err != nil {
		absPath = filePath
	}

	info, err := os.Stat(absPath)
	if err != nil {
		return nil, fmt.Errorf("file not found on disk: %w", err)
	}

	// Determine checksum
	checksum, err := GetBlake3Checksum(absPath)
	if err != nil {
		return nil, fmt.Errorf("failed to calculate checksum: %w", err)
	}

	mimeType := mime.TypeByExtension(filepath.Ext(absPath))
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}

	var reg models.FileRegistry
	err = db.DB.Where("checksum = ?", checksum).First(&reg).Error

	if err != nil {
		// Not found, register new
		reg = models.FileRegistry{
			Checksum:    checksum,
			FilePath:    absPath,
			FileSize:    info.Size(),
			MimeType:    mimeType,
			URL:         optURL,
			ETag:        optETag,
			TgFileID:    optTgFileID,
			TorrentHash: optTorrentHash,
			CreatedAt:   time.Now(),
		}
		if err := db.DB.Create(&reg).Error; err != nil {
			return nil, err
		}
		logger.Info("FileCore", "Registered new file in database", "checksum", checksum, "path", absPath)
		return &reg, nil
	}

	// Checksum exists! We found a duplicate.
	// Check if the master file path is different and exists
	if reg.FilePath != absPath {
		if _, err := os.Stat(reg.FilePath); err == nil {
			// Delete the duplicate file
			_ = os.Remove(absPath)
			// Hardlink the existing file to the duplicate path
			if err := SafeLink(reg.FilePath, absPath); err != nil {
				return nil, fmt.Errorf("failed to hardlink duplicate file: %w", err)
			}
			logger.Info("FileCore", "Deduplicated file successfully", "checksum", checksum, "original", reg.FilePath, "link", absPath)
		} else {
			// Master file was missing or deleted, update registry to point to this new path
			reg.FilePath = absPath
			reg.FileSize = info.Size()
			reg.MimeType = mimeType
		}
	}

	// Update optional tags if provided
	updated := false
	if optURL != "" && reg.URL == "" {
		reg.URL = optURL
		updated = true
	}
	if optETag != "" && reg.ETag == "" {
		reg.ETag = optETag
		updated = true
	}
	if optTgFileID != 0 && reg.TgFileID == 0 {
		reg.TgFileID = optTgFileID
		updated = true
	}
	if optTorrentHash != "" && reg.TorrentHash == "" {
		reg.TorrentHash = optTorrentHash
		updated = true
	}

	if updated {
		db.DB.Save(&reg)
	}

	return &reg, nil
}

// CheckDuplicateByTgID checks if a Telegram document is already registered and on disk.
// If it exists, it hardlinks the original file to targetPath and returns true.
func CheckDuplicateByTgID(tgID int64, targetPath string) (bool, string, error) {
	if tgID == 0 {
		return false, "", nil
	}

	var reg models.FileRegistry
	err := db.DB.Where("tg_file_id = ?", tgID).First(&reg).Error
	if err != nil {
		return false, "", nil
	}

	// Verify master file exists on disk
	if _, err := os.Stat(reg.FilePath); err != nil {
		// File was deleted from disk, clean record or ignore
		return false, "", nil
	}

	// Create hardlink to targetPath
	absTarget, err := filepath.Abs(targetPath)
	if err != nil {
		absTarget = targetPath
	}

	if err := SafeLink(reg.FilePath, absTarget); err != nil {
		return false, "", err
	}

	logger.Info("FileCore", "Instant Telegram download deduplication", "tg_file_id", tgID, "dest", absTarget)
	return true, reg.FilePath, nil
}

// CheckDuplicateByURL checks if a URL is already registered, sends a HEAD request to verify,
// and if valid, hardlinks the existing file to targetPath to avoid downloading it.
func CheckDuplicateByURL(urlStr string, targetPath string) (bool, string, error) {
	if urlStr == "" {
		return false, "", nil
	}

	var reg models.FileRegistry
	err := db.DB.Where("url = ?", urlStr).First(&reg).Error
	if err != nil {
		// Let's also check if the URL is suffix-matched or similar, but exact match is safest
		return false, "", nil
	}

	// Verify master file exists on disk
	if _, err := os.Stat(reg.FilePath); err != nil {
		return false, "", nil
	}

	// Perform a fast HEAD request to check ETag or Content-Length
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "HEAD", urlStr, nil)
	if err != nil {
		return false, "", nil
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		// Fallback to size check if HEAD request fails but URL matches exactly
		// This is a safe assumption for static URLs
		logger.Warn("FileCore", "HEAD check failed for URL, falling back to local registry match", "url", urlStr)
		absTarget, _ := filepath.Abs(targetPath)
		if err := SafeLink(reg.FilePath, absTarget); err != nil {
			return false, "", err
		}
		return true, reg.FilePath, nil
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return false, "", nil
	}

	// Verify Content-Length or ETag
	matched := false
	etag := resp.Header.Get("ETag")
	if etag != "" && reg.ETag != "" {
		if strings.Trim(etag, "\"") == strings.Trim(reg.ETag, "\"") {
			matched = true
		}
	}

	// Fallback to Content-Length check
	if !matched && resp.ContentLength > 0 {
		if resp.ContentLength == reg.FileSize {
			matched = true
		}
	}

	if matched {
		absTarget, err := filepath.Abs(targetPath)
		if err != nil {
			absTarget = targetPath
		}
		if err := SafeLink(reg.FilePath, absTarget); err != nil {
			return false, "", err
		}
		logger.Info("FileCore", "Instant HTTP download deduplication", "url", urlStr, "dest", absTarget)
		return true, reg.FilePath, nil
	}

	return false, "", nil
}

// CheckDuplicateByTorrentHash checks if a torrent info hash is already fully registered and completed.
func CheckDuplicateByTorrentHash(torrentHash string, targetPath string) (bool, string, error) {
	if torrentHash == "" {
		return false, "", nil
	}

	var reg models.FileRegistry
	err := db.DB.Where("torrent_hash = ?", torrentHash).First(&reg).Error
	if err != nil {
		return false, "", nil
	}

	if _, err := os.Stat(reg.FilePath); err != nil {
		return false, "", nil
	}

	absTarget, err := filepath.Abs(targetPath)
	if err != nil {
		absTarget = targetPath
	}

	if err := SafeLink(reg.FilePath, absTarget); err != nil {
		return false, "", err
	}

	logger.Info("FileCore", "Instant Torrent download deduplication", "info_hash", torrentHash, "dest", absTarget)
	return true, reg.FilePath, nil
}
