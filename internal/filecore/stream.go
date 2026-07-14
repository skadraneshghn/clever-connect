package filecore

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"clever-connect/internal/logger"
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
//   - If the file still exists locally, it is returned as-is with a no-op cleanup.
//   - Otherwise, if the file is archived in S3 (stateless leecher flow), it is
//     streamed from object storage into a temp file whose path is returned; the
//     caller MUST invoke cleanup() to remove it once the upload finishes.
//
// The temp file preserves the original file extension so media probing (ffprobe
// etc.) keeps working. Returns an error when the file is neither on disk nor in
// S3.
func MaterializeForUpload(absPath string) (localPath string, cleanup func(), err error) {
	if absPath == "" {
		return "", nil, fmt.Errorf("empty path")
	}

	if info, statErr := os.Stat(absPath); statErr == nil {
		if info.IsDir() {
			return "", nil, fmt.Errorf("target is a directory: %s", absPath)
		}
		return absPath, func() {}, nil
	}

	// Local copy missing — fall back to S3 object storage.
	reg, ok := LookupRegistryByPath(absPath)
	if !ok || !IsS3Stored(reg) {
		return "", nil, fmt.Errorf("file not found locally and not archived in S3: %s", absPath)
	}

	ext := filepath.Ext(absPath)
	tmp, err := os.CreateTemp("", "s3-upload-*"+ext)
	if err != nil {
		return "", nil, fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpPath := tmp.Name()
	cleanup = func() { _ = os.Remove(tmpPath) }

	ctx, cancel := context.WithTimeout(context.Background(), archiveUploadCtxTimeout)
	defer cancel()

	out, gErr := GetObject(ctx, reg.S3Key, "")
	if gErr != nil {
		tmp.Close()
		cleanup()
		return "", nil, fmt.Errorf("S3 fetch failed: %w", gErr)
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
		"key", reg.S3Key, "temp", tmpPath)
	return tmpPath, cleanup, nil
}
