package torrent

// pipeline.go — Stateful Torrent → S3 → Telegram scheduler pipeline
//
// Design rationale:
//
//  - QueueTorrentS3MoveJob is a function-pointer bridge (nil until set by
//    scheduler.Init). This avoids a circular import between torrent ↔ scheduler
//    and is the same pattern used for telegram.QueueUploadJob.
//
//  - RunTorrentS3MoveJob is the real handler registered as "torrent_s3_move"
//    in the scheduler's built-in job registry. It lives here (in the torrent
//    package) and is imported by scheduler/engine.go. The import direction is:
//      scheduler → torrent (safe; torrent does NOT import scheduler)
//
// Two-level idempotency:
//  1. In-memory registeredFiles map (fast path, session-scoped). The caller sets
//     this before launching the submission goroutine.
//  2. DB COUNT query (slow path) — prevents duplicate jobs after a restart.

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"clever-connect/internal/db"
	"clever-connect/internal/filecore"
	"clever-connect/internal/logger"
	"clever-connect/internal/models"
)

// TorrentS3MovePayload is the canonical JSON payload for a torrent_s3_move job.
type TorrentS3MovePayload struct {
	InfoHash      string `json:"info_hash"`
	FilePath      string `json:"file_path"`      // relative path inside SaveDirectory
	SaveDirectory string `json:"save_directory"`
	ChatID        int64  `json:"chat_id"` // 0 → use default at runtime
}

// QueueTorrentS3MoveJob is a function-pointer bridge set by scheduler.Init().
// It submits a torrent_s3_move scheduler job for a single completed torrent file.
// Callers must never invoke it while it is nil (check before calling).
var QueueTorrentS3MoveJob func(infoHash, saveDir, filePath string, chatID int64) error

// submitTorrentS3MoveJob is the internal helper called (in a goroutine) from
// updateStats() and onTorrentCompleted(). It enforces DB-level idempotency
// before delegating to the QueueTorrentS3MoveJob callback.
//
// MUST be called from a goroutine — it performs a DB query and never holds
// the manager mutex.
func submitTorrentS3MoveJob(m *TorrentManager, infoHash, saveDir, filePath string, chatID int64) {
	fileKey := infoHash + ":" + filePath

	// Fast nil-check: scheduler not yet initialised (brief window at startup).
	if QueueTorrentS3MoveJob == nil {
		// Re-enable the in-memory guard so the next stats tick retries.
		m.mu.Lock()
		delete(m.registeredFiles, fileKey)
		m.mu.Unlock()
		logger.Warn("Torrent", "Scheduler not ready yet — will retry torrent_s3_move submission",
			"info_hash", infoHash, "file", filePath)
		return
	}

	// DB-level idempotency — survive restarts.
	// Match on JSON payload fragments; escape backslashes for Windows paths.
	hashFrag := fmt.Sprintf(`"info_hash":"%s"`, infoHash)
	escapedPath := strings.ReplaceAll(filePath, `\`, `\\`)
	pathFrag := fmt.Sprintf(`"file_path":"%s"`, escapedPath)

	var jobCount int64
	db.DB.Model(&models.SchedulerJob{}).
		Where(
			"type = ? AND status NOT IN ? AND payload LIKE ? AND payload LIKE ?",
			"torrent_s3_move",
			[]string{models.JobStatusFailed, models.JobStatusCancelled},
			"%"+hashFrag+"%",
			"%"+pathFrag+"%",
		).
		Count(&jobCount)

	if jobCount > 0 {
		logger.Info("Torrent", "torrent_s3_move already queued/running — skipping duplicate",
			"info_hash", infoHash, "file", filePath)
		return
	}

	// Submit the scheduler job via the bridge callback.
	if err := QueueTorrentS3MoveJob(infoHash, saveDir, filePath, chatID); err != nil {
		// Re-enable the in-memory guard so the next tick can retry.
		m.mu.Lock()
		delete(m.registeredFiles, fileKey)
		m.mu.Unlock()
		logger.Error("Torrent", "Failed to submit torrent_s3_move job",
			"info_hash", infoHash, "file", filePath, "error", err)
	} else {
		logger.Info("Torrent", "Submitted torrent_s3_move scheduler job",
			"info_hash", infoHash, "file", filePath)
	}
}

// RunTorrentS3MoveJob is the scheduler handler registered as "torrent_s3_move".
// It is invoked by the scheduler worker pool — never by torrent goroutines directly.
//
// Pipeline:
//  1. Parse payload & validate file on disk.
//  2. Upload to S3 via filecore.RegisterAndArchiveToS3 (keepSparse=true so
//     anacrolix does not re-request pieces for the missing file).
//  3. Free local data blocks via punchHole (sparse stub preserves logical size).
//  4. Submit a downstream telegram_upload job.
func RunTorrentS3MoveJob(ctx context.Context, job *models.SchedulerJob, logFn func(string, string)) error {
	if !filecore.IsS3Enabled() {
		return fmt.Errorf("S3 object storage is disabled — torrent_s3_move requires S3")
	}

	// ── 1. Parse payload ──────────────────────────────────────────────────────
	var payload TorrentS3MovePayload
	if err := json.Unmarshal([]byte(job.Payload), &payload); err != nil {
		return fmt.Errorf("failed to unmarshal torrent_s3_move payload: %w", err)
	}
	if payload.SaveDirectory == "" {
		payload.SaveDirectory = "./data/manager/downloads"
	}

	absSaveDir, err := filepath.Abs(payload.SaveDirectory)
	if err != nil {
		absSaveDir = payload.SaveDirectory
	}

	torrentFilePath := filepath.Clean(filepath.Join(absSaveDir, payload.FilePath))

	logFn("INFO", fmt.Sprintf("torrent_s3_move: verifying file: %s", torrentFilePath))

	// ── 2. File existence check ───────────────────────────────────────────────
	info, err := os.Stat(torrentFilePath)
	if err != nil {
		// File missing from local disk. This typically means the Clever Cloud
		// container restarted (ephemeral disk wiped) after the job was queued
		// but before it could run — or the file was already archived by another
		// pipeline component (the file leecher) under a different registry path.
		//
		// Three-tier lookup strategy (most specific → broadest):
		//  1. Exact file_path match — file was previously archived by this job.
		//  2. torrent_hash match  — single-file torrent archived in an earlier run.
		//  3. Basename LIKE match — file leecher already uploaded the same file
		//     to S3 under a different local path (common when leecher and torrent
		//     client both downloaded the same content).
		baseName := filepath.Base(torrentFilePath)
		var reg models.FileRegistry

		if db.DB.Where("file_path = ? AND s3_key != ''", torrentFilePath).First(&reg).Error == nil {
			logFn("INFO", fmt.Sprintf("File already archived by exact path (s3_key=%s) — chaining Telegram", reg.S3Key))
			return chainTelegramUpload(job, logFn, torrentFilePath, payload.ChatID)
		}

		if db.DB.Where("torrent_hash = ? AND s3_key != ''", payload.InfoHash).First(&reg).Error == nil {
			logFn("INFO", fmt.Sprintf("File already archived via torrent_hash (s3_key=%s) — chaining Telegram", reg.S3Key))
			return chainTelegramUpload(job, logFn, torrentFilePath, payload.ChatID)
		}

		// Broadest fallback: match by filename — catches the case where the
		// file leecher downloaded and archived the same file but to a different
		// local path. LIKE with leading % is intentional (we match the suffix).
		if db.DB.Where("file_path LIKE ? AND s3_key != ''", "%/"+baseName).First(&reg).Error == nil {
			logFn("INFO", fmt.Sprintf(
				"File found in S3 registry by filename match (leecher path=%s, s3_key=%s) — chaining Telegram",
				reg.FilePath, reg.S3Key,
			))
			return chainTelegramUpload(job, logFn, torrentFilePath, payload.ChatID)
		}

		// File is unrecoverable: not on disk, not in any S3 registry entry.
		// Bumping RetryCount past the limit prevents the scheduler from
		// retrying this job endlessly — there is nothing to upload.
		logFn("ERROR", fmt.Sprintf(
			"File not found on disk and not in S3 registry (path=%s). "+
				"Most likely the container restarted and the ephemeral disk was wiped. "+
				"Marking job as permanently failed to prevent futile retries.",
			torrentFilePath,
		))
		job.RetryCount = 9999 // sentinel: force permanent failure in engine
		return fmt.Errorf(
			"file lost from ephemeral disk and not found in S3 registry by path, "+
				"torrent_hash, or filename — cannot recover (path=%s): %w",
			torrentFilePath, err,
		)
	}
	if info.IsDir() {
		return fmt.Errorf("target path is a directory, expected a file: %s", torrentFilePath)
	}

	// Sparse stub detection: first 4 KiB all-zero means punchHole already ran.
	if isTorrentFileSparse(torrentFilePath) {
		logFn("INFO", "File is a sparse stub — checking registry for existing S3 key")
		var reg models.FileRegistry
		if db.DB.Where("torrent_hash = ? AND s3_key != ''", payload.InfoHash).First(&reg).Error == nil {
			logFn("INFO", fmt.Sprintf("S3 key confirmed via torrent_hash (%s) — chaining Telegram upload", reg.S3Key))
			return chainTelegramUpload(job, logFn, torrentFilePath, payload.ChatID)
		}
		// Try basename match — leecher may have archived the file under a different path.
		if db.DB.Where("file_path LIKE ? AND s3_key != ''", "%/"+filepath.Base(torrentFilePath)).First(&reg).Error == nil {
			logFn("INFO", fmt.Sprintf("S3 key found via filename match (%s) — chaining Telegram upload", reg.S3Key))
			return chainTelegramUpload(job, logFn, torrentFilePath, payload.ChatID)
		}
		// Sparse stub but no S3 key found anywhere — previous upload failed mid-way.
		// Allow the scheduler to retry (do NOT set RetryCount sentinel here because
		// the stub still exists: we can retry the upload from the sparse stub path).
		return fmt.Errorf("file is a sparse stub but has no S3 key in registry — will retry upload")
	}

	// ── 3. Upload to S3 ───────────────────────────────────────────────────────
	logFn("INFO", fmt.Sprintf("Uploading %s to S3 (info_hash=%s, size=%s)",
		filepath.Base(torrentFilePath), payload.InfoHash, formatBytes(info.Size())))
	db.DB.Model(job).Update("progress", 10)

	reg, err := filecore.RegisterAndArchiveToS3(torrentFilePath, "", "", 0, payload.InfoHash, true)
	if err != nil {
		return fmt.Errorf("S3 upload failed: %w", err)
	}

	logFn("INFO", fmt.Sprintf("S3 upload complete (s3_key=%s)", reg.S3Key))
	db.DB.Model(job).Update("progress", 70)

	// ── 4. Free local disk blocks ─────────────────────────────────────────────
	logFn("INFO", "Freeing local disk blocks via punchHole (sparse stub preserved)")
	if err := punchHole(torrentFilePath); err != nil {
		// Non-fatal: S3 copy is the source of truth. Log and continue.
		logFn("WARN", fmt.Sprintf("punchHole failed — local blocks not freed (S3 succeeded): %v", err))
	} else {
		logFn("INFO", "Local blocks freed. Sparse stub in place. Disk space recovered.")
	}
	db.DB.Model(job).Update("progress", 85)

	// ── 5. Chain downstream Telegram upload ──────────────────────────────────
	return chainTelegramUpload(job, logFn, torrentFilePath, payload.ChatID)
}

// chainTelegramUpload creates a telegram_upload SchedulerJob record directly in
// the DB (mirroring how engine.SubmitJob works) and returns nil on success.
//
// It deliberately does NOT import the scheduler package to keep the import graph
// clean (scheduler already imports torrent for this file). The wakeup signal is
// not needed: the queue scanner's 2-second ticker will pick up the new job.
//
// Failure is non-fatal: the file is already safe in S3.
func chainTelegramUpload(job *models.SchedulerJob, logFn func(string, string), filePath string, chatID int64) error {
	type tgPayload struct {
		FilePath string `json:"file_path"`
		ChatID   int64  `json:"chat_id"`
	}

	payloadBytes, err := json.Marshal(tgPayload{FilePath: filePath, ChatID: chatID})
	if err != nil {
		logFn("WARN", fmt.Sprintf("Failed to marshal telegram_upload payload: %v — file is safe in S3", err))
		return nil
	}

	telegramJob := &models.SchedulerJob{
		UUID:        generateUUID(),
		Type:        "telegram_upload",
		Name:        fmt.Sprintf("Upload Torrent: %s", filepath.Base(filePath)),
		Description: fmt.Sprintf("Auto-upload of torrent file '%s' to Telegram after S3 archive", filepath.Base(filePath)),
		Category:    "files",
		Priority:    job.Priority,
		Status:      models.JobStatusQueued,
		Payload:     string(payloadBytes),
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	if err := db.DB.Create(telegramJob).Error; err != nil {
		// Non-fatal: file is in S3. Mark torrent_s3_move as succeeded anyway.
		logFn("WARN", fmt.Sprintf("telegram_upload job submission failed: %v — file is safe in S3", err))
		db.DB.Model(job).Update("progress", 100)
		return nil
	}

	logFn("INFO", fmt.Sprintf("Queued telegram_upload job (id=%d) for '%s'", telegramJob.ID, filepath.Base(filePath)))
	db.DB.Model(job).Update("progress", 100)
	return nil
}

// isTorrentFileSparse checks whether the first 4 KiB of a file are all zeroes
// — a reliable heuristic for detecting a punchHole sparse stub. Returns false
// on any read error (safe: causes a redundant S3 put of zeroes at worst).
func isTorrentFileSparse(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()

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

// generateUUID produces a UUID v4 string using crypto/rand.
func generateUUID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		// Fallback: timestamp-based pseudo-unique string.
		return fmt.Sprintf("torrent-%d", time.Now().UnixNano())
	}
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// formatBytes returns a human-readable byte count (e.g. "1.23 GiB").
func formatBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for v := n / unit; v >= unit; v /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.2f %ciB", float64(n)/float64(div), "KMGTPE"[exp])
}
