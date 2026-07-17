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
// It is a thin wrapper around MaterializeForUploadWithTorrent for callers that
// do not have a torrent info hash (manual uploads, leecher downloads). See
// that function for the full lookup strategy.
//
// The caller MUST invoke cleanup() to remove any materialised temp file once done.
func MaterializeForUpload(absPath string) (localPath string, cleanup func(), err error) {
	return MaterializeForUploadWithTorrent(absPath, "")
}

// MaterializeForUploadWithTorrent is the torrent-aware variant of
// MaterializeForUpload. The optional torrentHash lets the resolver fall back
// to a precise S3-key lookup by torrent info hash before guessing by filename,
// which matters for the torrent → S3 → Telegram pipeline: a completed torrent
// file is archived to S3 keyed by its BLAKE3 checksum, and the registry record
// may live under a different file_path (checksum dedup against a prior run, or
// a re-download after an ephemeral-disk wipe). Without the hash hint the
// basename fallback is the only way to reconnect the Telegram job's path to the
// archived S3 object.
//
// Lookup strategy (strictly ordered, no fall-through between mismatched branches):
//
//  1. File still exists on local disk:
//     - If it is registered by exact path AND archived in S3 → stream from S3
//       (S3 is authoritative; the local copy may be a punched sparse stub).
//     - If it is a torrent sparse stub (first 4 KiB all zero, torrentHash != "")
//       it is zeroed on disk after archiving — do NOT upload it; fall through to
//       the S3 recovery tiers so the real content is streamed from object storage.
//     - Otherwise the local copy is authoritative → returned as-is.
//
//  2a. Registry record found by exact file_path AND s3_key is set
//     → stream from S3 into a temp file.
//
//  2b. Registry record found by exact file_path BUT s3_key is EMPTY
//     → S3 upload for this file has not yet completed (or previously failed) and
//     the file is not on disk either. Return an actionable, retryable error
//     immediately; do NOT fall through to basename guessing (spurious S3 404).
//
//  3.  No registry record by exact file_path.
//     Resolve an S3 key through alternate lookups, in order:
//       a. torrent_hash match (precise, when torrentHash is provided),
//       b. file_path basename LIKE match (content archived under a different
//          path — checksum dedup, leecher flow, or re-download),
//       c. basename is itself the S3 key or the BLAKE3 checksum (stateless
//          leecher flow that queued the Telegram job with a hash-derived name).
//     Any hit → stream from S3.
//
//  4.  Last resort: basename looks like a hex-encoded key → fetch directly
//     from S3 without a registry record (orphaned reference).
//
//  5.  File present on disk (non-sparse) → return local copy.
//  6.  Nothing found → actionable error.
//
// The caller MUST invoke cleanup() to remove any materialised temp file once done.
func MaterializeForUploadWithTorrent(absPath, torrentHash string) (localPath string, cleanup func(), err error) {
	if absPath == "" {
		return "", nil, fmt.Errorf("empty path")
	}
	absPath = GetAbsolutePath(absPath)

	// 1. Inspect the local disk.
	onDisk := false
	if info, statErr := os.Stat(absPath); statErr == nil {
		if info.IsDir() {
			return "", nil, fmt.Errorf("target is a directory: %s", absPath)
		}
		onDisk = true
	}

	// A punched (sparse) stub left by the torrent pipeline after S3 archive is
	// all-zero on disk: punchHole frees the data blocks while preserving the
	// logical size so anacrolix does not re-request pieces. Uploading it would
	// send zeroes, so for torrent-originated uploads (torrentHash != "") we
	// detect the stub and fall through to the S3 recovery tiers instead. The
	// torrentHash guard keeps the behaviour unchanged for plain manual uploads
	// (which never produce sparse stubs).
	sparseStub := onDisk && torrentHash != "" && IsS3Enabled() && isSparseStubFile(absPath)

	// 2. Exact-path registry record.
	reg, regOK := LookupRegistryByPath(absPath)
	if regOK && IsS3Stored(reg) {
		// 2a. S3 key is set — stream from object storage (S3 is authoritative;
		// the local copy may be a sparse stub or already removed).
		return materializeFromS3(reg.S3Key, absPath)
	}

	// Local copy is authoritative when it is real content on disk (not a
	// torrent sparse stub). This preserves the original fast path for files
	// that are still on disk and not yet archived to S3.
	if onDisk && !sparseStub {
		return absPath, func() {}, nil
	}

	// From here the local file is either absent OR a torrent sparse stub.
	if regOK {
		// 2b. Record found by exact path but no S3 key, and the file is not
		// usable from disk. The S3 upload is still in progress or failed.
		// Return a retryable error — do NOT fall through to basename guessing
		// (which would make a spurious S3 request that always 404s).
		return "", nil, fmt.Errorf(
			"file registered but S3 upload not yet complete for %s — retry later", absPath)
	}

	// 3. No exact-path record. Resolve an S3 key through alternate lookups so
	//    content archived under a different path (checksum dedup / leecher /
	//    re-download after disk wipe) is still recoverable for the upload.
	if s3Key, tier, found := lookupAlternateS3Key(absPath, torrentHash); found {
		logger.Info("FileCore", "Materialized file from S3 via alternate lookup",
			"tier", tier, "path", absPath, "s3_key", s3Key)
		return materializeFromS3(s3Key, absPath)
	}

	// 3c. Basename is itself the S3 key or the BLAKE3 checksum — handles the
	//     stateless-leecher flow where the Telegram job was queued with a
	//     hash-derived filename (e.g. /downloads/<checksum>).
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

	// 5. A torrent sparse stub on disk with no recoverable S3 key anywhere —
	//    the S3 archive likely failed. Best effort: return the local stub so
	//    the caller can decide (preserves prior on-disk behaviour).
	if onDisk {
		return absPath, func() {}, nil
	}

	// 6. Genuinely unrecoverable: not on disk and not in any S3 registry entry.
	return "", nil, fmt.Errorf("file not found locally and not archived in S3: %s", absPath)
}

// lookupAlternateS3Key resolves an S3 object key for a path whose exact
// file_path is not registered, recovering content that was archived under a
// different path. It mirrors the three-tier recovery used by the torrent_s3_move
// scheduler handler so the Telegram upload pipeline can rehydrate the same
// archived file.
//
// Order:
//  1. torrent_hash match (precise — only when torrentHash is provided and the
//     registry record was tagged with that hash).
//  2. file_path basename LIKE match (content archived under a different path
//     that shares the same filename — checksum dedup, leecher flow, re-download).
//
// Returns the S3 key, a human-readable tier label, and true when found.
func lookupAlternateS3Key(absPath, torrentHash string) (s3Key, tier string, found bool) {
	if torrentHash != "" {
		var reg models.FileRegistry
		if err := db.DB.Where("torrent_hash = ? AND s3_key <> ''", torrentHash).First(&reg).Error; err == nil {
			return reg.S3Key, "torrent_hash", true
		}
	}
	basename := filepath.Base(absPath)
	if basename != "" && basename != "." {
		var reg models.FileRegistry
		if err := db.DB.Where("file_path LIKE ? AND s3_key <> ''", "%/"+basename).First(&reg).Error; err == nil {
			return reg.S3Key, "filename match", true
		}
	}
	return "", "", false
}

// isSparseStubFile reports whether the first 4 KiB of the file are all zeroes,
// a reliable heuristic for detecting a punchHole sparse stub left by the torrent
// pipeline after it archived the file to S3. Returns false on any read error or
// for a zero-length file (safe: those are never torrent data stubs).
func isSparseStubFile(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()

	fi, err := f.Stat()
	if err != nil || fi.Size() == 0 {
		return false
	}

	buf := make([]byte, 4096)
	n, err := f.Read(buf)
	if err != nil || n == 0 {
		return false
	}
	for i := 0; i < n; i++ {
		if buf[i] != 0 {
			return false
		}
	}
	return true
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

