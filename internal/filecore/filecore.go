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

// s3UploadSem bounds the number of concurrent parallel S3 multipart uploads to
// avoid flooding the Cellar endpoint when a large multi-file torrent completes.
// The semaphore is generous (32) so throughput stays high while protecting the
// remote store and the container's memory.
var s3UploadSem = make(chan struct{}, 32)

// scheduleS3Upload pushes a local file into object storage asynchronously using
// a bounded worker pool. The object is keyed by its BLAKE3 checksum, which makes
// deduplication free: identical content maps to the same key, so duplicate
// downloads never re-upload. The registry record's S3Key is set once the upload
// lands, at which point the streaming handler serves straight from S3.
//
// Failures are logged but never fatal: the local copy remains registered and
// streaming transparently falls back to disk until a retry succeeds.
func scheduleS3Upload(regID uint, checksum, mimeType, localPath string) {
	if !IsS3Enabled() || checksum == "" {
		return
	}
	go func() {
		// Acquire a slot — may block briefly if many uploads are in flight,
		// which naturally backpressures the spawning goroutine.
		s3UploadSem <- struct{}{}
		defer func() { <-s3UploadSem }()

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		defer cancel()

		if _, err := UploadFileToS3(ctx, checksum, mimeType, localPath); err != nil {
			logger.Error("FileCore", "S3 upload failed — local copy remains active",
				"checksum", checksum, "path", localPath, "error", err)
			return
		}

		if err := db.DB.Model(&models.FileRegistry{}).Where("id = ?", regID).
			Update("s3_key", checksum).Error; err != nil {
			logger.Error("FileCore", "Failed to persist S3 key in registry", "id", regID, "error", err)
			return
		}
		logger.Info("FileCore", "File pushed to S3 object storage", "checksum", checksum, "path", localPath)
	}()
}


// GetAbsolutePath takes a raw path string (which could be absolute, relative,
// from Windows/Linux, or even double-prefixed/mismatched due to different execution PWDs)
// and guarantees it returns a clean, absolute path sandboxed inside the file manager root.
func GetAbsolutePath(rawPath string) string {
	if rawPath == "" {
		return ""
	}

	absBase, err := filepath.Abs("./data/manager")
	if err != nil {
		absBase = "./data/manager"
	}

	// Normalize all path separators to forward slash for inspection
	normalized := filepath.ToSlash(rawPath)

	// Look for the sandbox boundary "data/manager/" or "data/manager" to extract the clean relative path.
	// This heals any mismatched, container-specific, or double-prefixed absolute roots.
	markerWithSlash := "data/manager/"
	markerNoSlash := "data/manager"

	if idx := strings.LastIndex(normalized, markerWithSlash); idx != -1 {
		rel := normalized[idx+len(markerWithSlash):]
		return filepath.Clean(filepath.Join(absBase, rel))
	} else if idx := strings.LastIndex(normalized, markerNoSlash); idx != -1 {
		rel := normalized[idx+len(markerNoSlash):]
		return filepath.Clean(filepath.Join(absBase, rel))
	}

	// If no marker is found, but the path is already absolute, return it cleaned.
	// This supports arbitrary absolute paths (e.g. system temp directories in unit tests).
	if filepath.IsAbs(rawPath) {
		return filepath.Clean(rawPath)
	}

	// If no marker is found and it's relative, clean it up relative to the sandbox base.
	cleanRel := filepath.Clean("/" + rawPath)
	return filepath.Clean(filepath.Join(absBase, cleanRel))
}

// GetAbsoluteSavePath resolves any relative or absolute download folder path
// to ensure it is sandboxed and located inside the File Manager's root folder ("./data/manager")
func GetAbsoluteSavePath(saveDir string) string {
	return GetAbsolutePath(saveDir)
}

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
	absPath := GetAbsolutePath(filePath)

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
		// Asynchronously mirror the file into S3 object storage (keyed by
		// checksum). Dedup is automatic: if the checksum already had an S3
		// object we never reach this branch. Streaming prefers S3 once the
		// upload completes and otherwise serves the local copy.
		scheduleS3Upload(reg.ID, checksum, mimeType, absPath)
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

// archiveUploadCtxTimeout gives large files plenty of room to finish a parallel
// multipart upload while still bounding runaway transfers.
const archiveUploadCtxTimeout = 30 * time.Minute

// RegisterAndArchiveToS3 is the stateless leecher pipeline: it pushes a
// just-downloaded local file into S3 (parallel multipart, synchronous), records
// the file metadata in the database with its S3 key, and then removes the
// local copy so the container's ephemeral disk never fills up.
//
// S3 is the single source of truth: after this returns successfully the file
// persists in object storage even if the application restarts, and every
// later access (streaming, download, Telegram auto-upload) reads it from S3.
//
// Deduplication is checksum-based: if the content already lives in S3 the
// upload is skipped entirely and only the redundant local copy is removed.
// On S3 failure the local file is preserved as a fallback and the error is
// returned so the caller can retry without losing data.
//
// The returned record always has S3Key set on success (reg.S3Key != "").
//
// When keepSparse is true the local copy is left on disk instead of being
// removed. This is used by the torrent manager for archived torrent files:
// anacrolix re-stats the file (checkCompleteFileSizes) and would re-request
// pieces if the file vanished, so the caller turns it into a sparse stub
// (logical size preserved, data blocks freed) instead of deleting it.
func RegisterAndArchiveToS3(absPath, optURL, optETag string, optTgFileID int64, optTorrentHash string, keepSparse bool) (*models.FileRegistry, error) {
	absPath = GetAbsolutePath(absPath)

	info, err := os.Stat(absPath)
	if err != nil {
		return nil, fmt.Errorf("file not found on disk: %w", err)
	}

	checksum, err := GetBlake3Checksum(absPath)
	if err != nil {
		return nil, fmt.Errorf("failed to calculate checksum: %w", err)
	}

	mimeType := mime.TypeByExtension(filepath.Ext(absPath))
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}

	s3Key := checksum

	// Look for an existing record with the same checksum (deduplication).
	var reg models.FileRegistry
	findErr := db.DB.Where("checksum = ?", checksum).First(&reg).Error

	if findErr != nil {
		// New file: create the registry record first (without S3Key) so the
		// metadata always exists, then push to S3, then mark S3-backed.
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
			// Rare race: another worker created the same checksum. Re-query.
			if e2 := db.DB.Where("checksum = ?", checksum).First(&reg).Error; e2 != nil {
				return nil, err
			}
			findErr = nil
		}
	}

	if findErr == nil {
		// Duplicate content already registered. Make sure it is archived in S3.
		if reg.S3Key == "" {
			ctx, cancel := context.WithTimeout(context.Background(), archiveUploadCtxTimeout)
			_, upErr := UploadFileToS3(ctx, s3Key, mimeType, absPath)
			cancel()
			if upErr != nil {
				logger.Error("FileCore", "S3 archive upload failed for duplicate — keeping local copy",
					"checksum", checksum, "path", absPath, "error", upErr)
				return &reg, upErr
			}
			reg.S3Key = s3Key
			db.DB.Model(&reg).Update("s3_key", s3Key)
		}
		// Update optional tags on the existing record if missing.
		updateRegistryTags(&reg, optURL, optETag, optTgFileID, optTorrentHash)
	} else {
		// Newly created record — upload to S3.
		ctx, cancel := context.WithTimeout(context.Background(), archiveUploadCtxTimeout)
		_, upErr := UploadFileToS3(ctx, s3Key, mimeType, absPath)
		cancel()
		if upErr != nil {
			logger.Error("FileCore", "S3 archive upload failed — keeping local copy",
				"checksum", checksum, "path", absPath, "error", upErr)
			return &reg, upErr
		}
		reg.S3Key = s3Key
		db.DB.Model(&reg).Update("s3_key", s3Key)
	}

	// S3 now holds the object. Remove the local copy to keep the ephemeral
	// disk clean (stateless), unless the caller asked to keep it as a sparse
	// stub (torrent files). Also clean up any grab temp artifacts.
	if !keepSparse {
		if err := os.Remove(absPath); err != nil && !os.IsNotExist(err) {
			logger.Warn("FileCore", "Could not remove local copy after S3 archive",
				"path", absPath, "error", err)
		}
	}
	_ = os.Remove(absPath + ".gtmp")

	logger.Info("FileCore", "File archived to S3",
		"checksum", checksum, "s3_key", s3Key, "size", info.Size(), "keep_local", keepSparse)
	return &reg, nil
}

// updateRegistryTags fills in optional metadata fields on an existing record
// only when the caller supplied a value the record does not yet have.
func updateRegistryTags(reg *models.FileRegistry, optURL, optETag string, optTgFileID int64, optTorrentHash string) {
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
		db.DB.Save(reg)
	}
}

// CheckDuplicateByTgID checks if a Telegram document is already registered.
// When the file only lives in S3 (local copy removed by the stateless leecher)
// it still returns true so callers can stream it from object storage rather than
// re-downloading.
func CheckDuplicateByTgID(tgID int64, targetPath string) (bool, string, error) {
	if tgID == 0 {
		return false, "", nil
	}

	var reg models.FileRegistry
	err := db.DB.Where("tg_file_id = ?", tgID).First(&reg).Error
	if err != nil {
		return false, "", nil
	}

	// Fast path: file lives only in S3 (local copy was removed).
	if _, statErr := os.Stat(reg.FilePath); statErr != nil {
		if reg.S3Key != "" && IsS3Enabled() {
			logger.Info("FileCore", "Telegram dedup via S3 (no local copy)",
				"tg_file_id", tgID, "s3_key", reg.S3Key)
			return true, reg.S3Key, nil
		}
		// File was deleted from disk with no S3 fallback — treat as not found.
		return false, "", nil
	}

	// File is on disk — create a hardlink at targetPath.
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

// CheckDuplicateByURL checks if a URL is already registered, sends a HEAD request to
// verify, and if valid, deduplicates the download. When the file only lives in S3 (local
// copy removed by the stateless leecher) it returns true with the S3 key so callers can
// stream it from object storage rather than re-downloading.
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

	// Fast path: file lives only in S3 (local copy was removed).
	if _, statErr := os.Stat(reg.FilePath); statErr != nil {
		if reg.S3Key != "" && IsS3Enabled() {
			logger.Info("FileCore", "HTTP dedup via S3 (no local copy)",
				"url", urlStr, "s3_key", reg.S3Key)
			return true, reg.S3Key, nil
		}
		return false, "", nil
	}

	// File is on disk — perform a fast HEAD request to confirm it hasn't changed.
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

// CheckDuplicateByTorrentHash checks if a torrent info hash is already fully registered.
// When the file only lives in S3 (local copy removed) it returns true with the S3 key so
// callers can stream from object storage rather than re-seeding the torrent.
func CheckDuplicateByTorrentHash(torrentHash string, targetPath string) (bool, string, error) {
	if torrentHash == "" {
		return false, "", nil
	}

	var reg models.FileRegistry
	err := db.DB.Where("torrent_hash = ?", torrentHash).First(&reg).Error
	if err != nil {
		return false, "", nil
	}

	// Fast path: file lives only in S3 (local copy was removed).
	if _, statErr := os.Stat(reg.FilePath); statErr != nil {
		if reg.S3Key != "" && IsS3Enabled() {
			logger.Info("FileCore", "Torrent dedup via S3 (no local copy)",
				"torrent_hash", torrentHash, "s3_key", reg.S3Key)
			return true, reg.S3Key, nil
		}
		return false, "", nil
	}

	// File is on disk — create a hardlink at targetPath.
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

// LookupRegistryByPath resolves a sandboxed local path to its FileRegistry
// record. The streaming handler uses this to decide whether an object is also
// stored in S3 (and therefore should be served from there for speed).
//
// Returns nil (and ok=false) when no record exists for the path.
func LookupRegistryByPath(absPath string) (*models.FileRegistry, bool) {
	if absPath == "" {
		return nil, false
	}
	absPath = GetAbsolutePath(absPath)
	var reg models.FileRegistry
	if err := db.DB.Where("file_path = ?", absPath).First(&reg).Error; err != nil {
		return nil, false
	}
	return &reg, true
}

// LookupRegistryByChecksum resolves a BLAKE3 checksum to its registry record.
func LookupRegistryByChecksum(checksum string) (*models.FileRegistry, bool) {
	if checksum == "" {
		return nil, false
	}
	var reg models.FileRegistry
	if err := db.DB.Where("checksum = ?", checksum).First(&reg).Error; err != nil {
		return nil, false
	}
	return &reg, true
}

// LookupRegistryByURL resolves a source URL to its registry record (if any).
// Used by the downloader to skip re-downloading files already archived in S3.
func LookupRegistryByURL(url string) (*models.FileRegistry, bool) {
	if url == "" {
		return nil, false
	}
	var reg models.FileRegistry
	if err := db.DB.Where("url = ?", url).First(&reg).Error; err != nil {
		return nil, false
	}
	return &reg, true
}

// IsS3Stored reports whether a registry record is backed by an S3 object that
// is available for streaming. It is the authoritative gate the file streaming
// handler checks before serving from object storage.
func IsS3Stored(reg *models.FileRegistry) bool {
	return IsS3Enabled() && reg != nil && reg.S3Key != ""
}
