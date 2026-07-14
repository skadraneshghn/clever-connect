package filecore

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"clever-connect/internal/db"
	"clever-connect/internal/logger"
	"clever-connect/internal/models"
)

// StreamS3Object streams an object from S3 straight to an http.ResponseWriter
// with full HTTP Range (206 Partial Content) passthrough — enabling instant
// video seeking and chunked downloads with near-zero backend memory.
//
// It mirrors the relevant S3 response headers (Content-Type, Content-Length,
// Content-Range, Accept-Ranges) onto the client response, picks the correct
// status code, and copies the body through a 64 KB ring buffer.
//
// disposition is an optional Content-Disposition value (e.g.
// `attachment; filename="movie.mp4"`) to force a client download name.
//
// Returns false when the object cannot be served from S3 (disabled, missing or
// transport error) so the caller can transparently fall back to local disk.
func StreamS3Object(ctx context.Context, w http.ResponseWriter, key, rangeHeader, fallbackType, disposition string) bool {
	if !IsS3Enabled() || key == "" {
		return false
	}

	out, err := GetObject(ctx, key, rangeHeader)
	if err != nil || out == nil {
		if err != nil {
			logger.Warn("FileCore", "S3 stream fetch failed — falling back to local disk",
				"key", key, "error", err)
		}
		return false
	}
	defer out.Body.Close()

	w.Header().Set("Accept-Ranges", "bytes")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	w.Header().Set("Connection", "keep-alive")

	if out.ContentType != nil && *out.ContentType != "" {
		w.Header().Set("Content-Type", *out.ContentType)
	} else if fallbackType != "" {
		w.Header().Set("Content-Type", fallbackType)
	} else {
		w.Header().Set("Content-Type", "application/octet-stream")
	}

	if out.ContentLength != nil {
		w.Header().Set("Content-Length", fmt.Sprintf("%d", *out.ContentLength))
	}

	if disposition != "" {
		w.Header().Set("Content-Disposition", disposition)
	}

	if out.ContentRange != nil && *out.ContentRange != "" {
		w.Header().Set("Content-Range", *out.ContentRange)
		w.WriteHeader(http.StatusPartialContent)
	} else {
		w.WriteHeader(http.StatusOK)
	}

	// Zero-allocation streaming copy with a 64 KB reusable buffer. An error
	// here usually means the client disconnected mid-stream, so it is ignored.
	buf := make([]byte, 64*1024)
	_, _ = io.CopyBuffer(w, out.Body, buf)
	return true
}

// PresignDownloadRedirect returns a presigned attachment URL valid for the
// given lifetime, or "" when S3 is unavailable. The handler 302-redirects to
// it so the client downloads directly from Cellar, fully offloading the server.
func PresignDownloadRedirect(ctx context.Context, key, filename string, lifetime time.Duration) string {
	if !IsS3Enabled() || key == "" {
		return ""
	}
	url, err := PresignDownloadURL(ctx, key, filename, lifetime)
	if err != nil {
		logger.Warn("FileCore", "Failed to presign S3 download URL", "key", key, "error", err)
		return ""
	}
	return url
}

// MaterializeForUpload guarantees a local-readable copy of a file for upload
// pipelines (e.g. the Telegram MTProto uploader) that can only read from disk.
//
// Lookup strategy (first match wins):
//  1. File still exists on local disk → returned as-is with a no-op cleanup.
//  2. Registry record found by exact file_path → stream from S3 into a temp file.
//  3. Basename of path matches a known S3 key or checksum in the registry
//     (happens when the stateless leecher queued the Telegram job with the
//     BLAKE3 checksum as the filename after removing the real local copy).
//  4. Basename looks like a hex string → try fetching it directly from S3 as a
//     key (last-resort for paths whose filename IS the S3 object key).
//
// The caller MUST invoke cleanup() to remove any materialised temp file once done.
// Returns an error when the file is neither on disk nor reachable via S3.
func MaterializeForUpload(absPath string) (localPath string, cleanup func(), err error) {
	if absPath == "" {
		return "", nil, fmt.Errorf("empty path")
	}

	// 1. File is still on local disk — serve it directly.
	if info, statErr := os.Stat(absPath); statErr == nil {
		if info.IsDir() {
			return "", nil, fmt.Errorf("target is a directory: %s", absPath)
		}
		return absPath, func() {}, nil
	}

	// 2. Look up the registry by exact file_path.
	reg, ok := LookupRegistryByPath(absPath)

	// 3. If not found by path, try the basename as an S3 key or checksum.
	//    This handles the stateless-leecher flow where the Telegram job was
	//    queued with a path like /downloads/<checksum> after the real file
	//    was removed.
	if !ok || !IsS3Stored(reg) {
		basename := filepath.Base(absPath)
		if basename != "" && basename != "." {
			// Try s3_key column first (exact match).
			var candidate models.FileRegistry
			if e := db.DB.Where("s3_key = ?", basename).First(&candidate).Error; e == nil && candidate.S3Key != "" {
				reg = &candidate
				ok = true
			} else if e := db.DB.Where("checksum = ?", basename).First(&candidate).Error; e == nil && candidate.S3Key != "" {
				// Try checksum column (BLAKE3 hex).
				reg = &candidate
				ok = true
			}
		}
	}

	// 4. If we now have an S3-backed record, stream it into a temp file.
	if ok && IsS3Stored(reg) {
		return materializeFromS3(reg.S3Key, absPath)
	}

	// 5. Last resort: if the basename looks like a hex key, fetch it from S3
	//    directly without a registry record (e.g. orphaned reference).
	if IsS3Enabled() {
		basename := filepath.Base(absPath)
		if isHexKey(basename) {
			logger.Warn("FileCore", "Materializing from S3 by raw key (no registry record)",
				"key", basename, "path", absPath)
			return materializeFromS3(basename, absPath)
		}
	}

	return "", nil, fmt.Errorf("file not found locally and not archived in S3: %s", absPath)
}

// materializeFromS3 streams an S3 object identified by key into a temp file
// whose extension is inferred from the reference path. Returns the temp path
// and a cleanup function the caller must call when done.
func materializeFromS3(s3Key, refPath string) (string, func(), error) {
	ext := filepath.Ext(refPath)
	tmp, err := os.CreateTemp("", "s3-upload-*"+ext)
	if err != nil {
		return "", nil, fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpPath := tmp.Name()
	cleanup := func() { _ = os.Remove(tmpPath) }

	ctx, cancel := context.WithTimeout(context.Background(), archiveUploadCtxTimeout)
	defer cancel()

	out, gErr := GetObject(ctx, s3Key, "")
	if gErr != nil {
		tmp.Close()
		cleanup()
		return "", nil, fmt.Errorf("S3 fetch failed for key %s: %w", s3Key, gErr)
	}
	defer out.Body.Close()

	if _, cErr := io.Copy(tmp, out.Body); cErr != nil {
		tmp.Close()
		cleanup()
		return "", nil, fmt.Errorf("S3 download to temp failed: %w", cErr)
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("temp file close failed: %w", err)
	}

	logger.Info("FileCore", "Materialized file from S3 for upload",
		"key", s3Key, "temp", tmpPath)
	return tmpPath, cleanup, nil
}

// isHexKey reports whether s looks like a lowercase hex-encoded hash that
// could be a valid S3 object key (BLAKE3 checksum: exactly 64 chars, or any
// multiple-of-2 run of [0-9a-f]).
func isHexKey(s string) bool {
	if len(s) < 16 || len(s)%2 != 0 {
		return false
	}
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return false
		}
	}
	return true
}
