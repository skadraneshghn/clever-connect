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
// Lookup strategy (strictly ordered, no fall-through between mismatched branches):
//
//  1. File still exists on local disk → returned as-is with a no-op cleanup.
//
//  2a. Registry record found by exact file_path AND s3_key is set
//      → stream from S3 into a temp file.
//
//  2b. Registry record found by exact file_path BUT s3_key is EMPTY
//      → S3 upload for this file has not yet completed (or previously failed).
//      The file is also not on disk.  Return an actionable error immediately;
//      do NOT fall through to basename guessing (which would 404 on S3).
//
//  3.  No registry record found by file_path at all.
//      Try the path basename as an alternative S3 key / checksum lookup —
//      handles the stateless-leecher flow where the Telegram job was queued
//      with a hash-derived filename after the real local copy was removed.
//
//  4.  Last resort: basename looks like a hex-encoded key → fetch directly
//      from S3 without a registry record (orphaned reference).
//
// The caller MUST invoke cleanup() to remove any materialised temp file once done.
func MaterializeForUpload(absPath string) (localPath string, cleanup func(), err error) {
	if absPath == "" {
		return "", nil, fmt.Errorf("empty path")
	}

	// 1. File is still on local disk.
	if info, statErr := os.Stat(absPath); statErr == nil {
		if info.IsDir() {
			return "", nil, fmt.Errorf("target is a directory: %s", absPath)
		}
		// If the file is registered AND archived in S3, the local copy is no
		// longer authoritative: the torrent pipeline turns it into a sparse
		// stub (logical size preserved, data blocks freed by punchHole) right
		// after archiving, so reading it back would yield zeroes and a Telegram
		// upload would send an empty file. Stream the real content from object
		// storage instead. Files not yet archived (no S3 key) are still served
		// straight from local disk — the fast path.
		if reg, ok := LookupRegistryByPath(absPath); ok && IsS3Stored(reg) {
			return materializeFromS3(reg.S3Key, absPath)
		}
		return absPath, func() {}, nil
	}

	// 2. Look up the registry by exact file_path.
	reg, ok := LookupRegistryByPath(absPath)
	if ok {
		// 2a. S3 key is set — stream from object storage.
		if IsS3Stored(reg) {
			return materializeFromS3(reg.S3Key, absPath)
		}
		// 2b. Record found but no S3 key.  The file is NOT on disk either.
		//     The S3 upload is either still in progress or previously failed.
		//     Return immediately with a clear, retryable error — do NOT fall
		//     through to the basename-as-key stages below, which would make a
		//     spurious S3 request that always returns 404.
		return "", nil, fmt.Errorf(
			"file registered but S3 upload not yet complete for %s — retry later", absPath)
	}

	// 3. No record found by file_path. Try the basename as an S3 key or
	//    BLAKE3 checksum (handles the case where the Telegram job was queued
	//    with a hash-derived filename like /downloads/<checksum>).
	basename := filepath.Base(absPath)
	if basename != "" && basename != "." {
		var candidate models.FileRegistry
		if e := db.DB.Where("s3_key = ?", basename).First(&candidate).Error; e == nil && candidate.S3Key != "" {
			logger.Info("FileCore", "Materialized via s3_key basename match",
				"basename", basename, "s3_key", candidate.S3Key)
			return materializeFromS3(candidate.S3Key, absPath)
		}
		if e := db.DB.Where("checksum = ?", basename).First(&candidate).Error; e == nil && candidate.S3Key != "" {
			logger.Info("FileCore", "Materialized via checksum basename match",
				"basename", basename, "s3_key", candidate.S3Key)
			return materializeFromS3(candidate.S3Key, absPath)
		}
	}

	// 4. Last resort: basename looks like a hex-encoded S3 key — fetch
	//    directly from object storage (orphaned reference with no DB record).
	if IsS3Enabled() && isHexKey(basename) {
		logger.Warn("FileCore", "Materializing from S3 by raw key (no registry record)",
			"key", basename, "path", absPath)
		return materializeFromS3(basename, absPath)
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
// even-length run of [0-9a-f] ≥ 16 chars).
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

