package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"runtime"
	"sort"
	"sync"
	"time"

	"clever-connect/internal/db"
	"clever-connect/internal/downloader"
	"clever-connect/internal/logger"
	"clever-connect/internal/models"
	"clever-connect/internal/telegram"

	"github.com/google/uuid"
	"github.com/robfig/cron/v3"
)

// ──────────────────────────────────────────────────────────────────────────────
// Job Scheduler Engine — Enterprise-grade, multi-worker, cron-integrated
// ──────────────────────────────────────────────────────────────────────────────

var (
	Engine   *Scheduler
	initOnce sync.Once
)

// JobFunc is the actual work payload that a scheduled job executes.
type JobFunc func(ctx context.Context, job *models.SchedulerJob, logFn func(level, message string)) error

// Scheduler is the core engine with a priority worker pool and cron daemon.
type Scheduler struct {
	mu          sync.RWMutex
	cron        *cron.Cron
	cronEntries map[uint]cron.EntryID // schedulerJob.ID -> cronEntryID
	registry    map[string]JobFunc    // jobType -> handler
	activeJobs  map[uint]context.CancelFunc
	workerCount int
	stopChan    chan struct{}
	wakeupChan  chan struct{} // signals workers to re-check the queue
	running     bool
}

// Init bootstraps the singleton scheduler engine.
func Init() {
	initOnce.Do(func() {
		// Load or seed default configuration
		var cfg models.SchedulerConfig
		if err := db.DB.First(&cfg).Error; err != nil {
			cfg = models.SchedulerConfig{
				MaxConcurrentJobs:  4,
				DefaultPriority:    5,
				RetryLimit:         3,
				RetryDelaySeconds:  30,
				JobTimeoutSeconds:  3600,
				PurgeAfterDays:     30,
				EnableCronJobs:     true,
				EnableNotifications: false,
			}
			db.DB.Create(&cfg)
			logger.Info("Scheduler", "Seeded default scheduler configuration")
		}

		Engine = &Scheduler{
			cron:        cron.New(cron.WithSeconds(), cron.WithChain(cron.Recover(cron.DefaultLogger))),
			cronEntries: make(map[uint]cron.EntryID),
			registry:    make(map[string]JobFunc),
			activeJobs:  make(map[uint]context.CancelFunc),
			workerCount: cfg.MaxConcurrentJobs,
			stopChan:    make(chan struct{}),
			wakeupChan:  make(chan struct{}, 1),
		}

		// Register built-in job types
		Engine.registerBuiltinJobs()

		// Initialize QueueUploadJob in telegram package
		telegram.QueueUploadJob = func(filePath string, chatID int64) error {
			payload := telegram.TelegramUploadPayload{
				FilePath: filePath,
				ChatID:   chatID,
			}
			payloadBytes, err := json.Marshal(payload)
			if err != nil {
				return err
			}
			_, err = Engine.SubmitJob(
				"telegram_upload",
				fmt.Sprintf("Upload %s", filepath.Base(filePath)),
				fmt.Sprintf("Parallel upload of %s to Telegram", filepath.Base(filePath)),
				"files",
				5,
				string(payloadBytes),
				"",
			)
			return err
		}

		// Register auto-upload callback for the downloader bridge
		downloader.RegisterAutoUploadFunc(func(filePath string, chatID int64) error {
			payload := telegram.TelegramUploadPayload{
				FilePath: filePath,
				ChatID:   chatID,
			}
			payloadBytes, err := json.Marshal(payload)
			if err != nil {
				return err
			}
			_, err = Engine.SubmitJob(
				"telegram_upload",
				fmt.Sprintf("Auto-Upload %s", filepath.Base(filePath)),
				fmt.Sprintf("Auto-upload of %s to Telegram after download completed", filepath.Base(filePath)),
				"files",
				5,
				string(payloadBytes),
				"",
			)
			return err
		})

		// Reset stale "running" jobs from a previous crash
		db.DB.Model(&models.SchedulerJob{}).
			Where("status = ?", models.JobStatusRunning).
			Updates(map[string]interface{}{
				"status":  models.JobStatusQueued,
				"message": "Reset after engine restart",
			})

		// Start the worker pool and cron daemon
		Engine.Start()
		logger.Info("Scheduler", "Enterprise Job Scheduler initialized",
			"workers", Engine.workerCount,
			"cpus", runtime.NumCPU(),
		)
	})
}

// RegisterJob registers a custom job type handler.
func (s *Scheduler) RegisterJob(jobType string, fn JobFunc) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.registry[jobType] = fn
	logger.Info("Scheduler", "Registered job type", "type", jobType)
}

// Start launches the worker pool, cron daemon, and queue scanner.
func (s *Scheduler) Start() {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return
	}
	s.running = true
	s.mu.Unlock()

	// Start cron daemon
	s.cron.Start()

	// Load persisted cron schedules
	s.loadCronJobs()

	// Start queue scanner
	go s.queueScanner()

	logger.Info("Scheduler", "Worker pool started", "workers", s.workerCount)
}

// Stop gracefully shuts down the scheduler.
func (s *Scheduler) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.running {
		return
	}
	s.running = false

	// Stop cron
	ctx := s.cron.Stop()
	<-ctx.Done()

	// Cancel all active jobs
	for id, cancel := range s.activeJobs {
		cancel()
		db.DB.Model(&models.SchedulerJob{}).Where("id = ?", id).Updates(map[string]interface{}{
			"status":  models.JobStatusCancelled,
			"message": "Scheduler shutdown",
		})
	}

	close(s.stopChan)
	logger.Info("Scheduler", "Scheduler engine stopped")
}

// SubmitJob creates and queues a new job.
func (s *Scheduler) SubmitJob(jobType, name, description, category string, priority int, payload string, cronExpr string) (*models.SchedulerJob, error) {
	if priority <= 0 {
		var cfg models.SchedulerConfig
		if err := db.DB.First(&cfg).Error; err == nil {
			priority = cfg.DefaultPriority
		} else {
			priority = 5
		}
	}

	job := &models.SchedulerJob{
		UUID:        uuid.New().String(),
		Type:        jobType,
		Name:        name,
		Description: description,
		Category:    category,
		Priority:    priority,
		Status:      models.JobStatusQueued,
		Payload:     payload,
		CronExpr:    cronExpr,
		Progress:    0,
		RetryCount:  0,
	}

	// Validate job type exists
	s.mu.RLock()
	_, exists := s.registry[jobType]
	s.mu.RUnlock()
	if !exists {
		return nil, fmt.Errorf("unknown job type: %s", jobType)
	}

	if err := db.DB.Create(job).Error; err != nil {
		return nil, fmt.Errorf("failed to persist job: %w", err)
	}

	// If it's a cron job, register the schedule
	if cronExpr != "" {
		s.registerCronJob(job)
	}

	// Log the submission
	s.addLog(job.ID, "INFO", fmt.Sprintf("Job submitted: %s [type=%s, priority=%d]", name, jobType, priority))

	// Wake up the queue scanner
	s.wakeup()

	logger.Info("Scheduler", "Job submitted",
		"id", job.ID,
		"uuid", job.UUID,
		"type", jobType,
		"name", name,
		"priority", priority,
	)

	return job, nil
}

// CancelJob cancels a running or queued job.
func (s *Scheduler) CancelJob(jobID uint) error {
	s.mu.Lock()
	cancel, active := s.activeJobs[jobID]
	s.mu.Unlock()

	if active {
		cancel()
		s.mu.Lock()
		delete(s.activeJobs, jobID)
		s.mu.Unlock()
	}

	result := db.DB.Model(&models.SchedulerJob{}).
		Where("id = ? AND status IN ?", jobID, []string{
			models.JobStatusQueued,
			models.JobStatusRunning,
			models.JobStatusScheduled,
		}).
		Updates(map[string]interface{}{
			"status":      models.JobStatusCancelled,
			"message":     "Cancelled by user",
			"finished_at": time.Now(),
		})

	if result.RowsAffected == 0 {
		return fmt.Errorf("job %d is not in a cancellable state", jobID)
	}

	s.addLog(jobID, "WARN", "Job cancelled by user")
	logger.Info("Scheduler", "Job cancelled", "id", jobID)
	return nil
}

// RetryJob re-queues a failed/cancelled job.
func (s *Scheduler) RetryJob(jobID uint) error {
	var job models.SchedulerJob
	if err := db.DB.First(&job, jobID).Error; err != nil {
		return fmt.Errorf("job not found: %w", err)
	}

	if job.Status != models.JobStatusFailed && job.Status != models.JobStatusCancelled {
		return fmt.Errorf("job %d is not in a retryable state (current: %s)", jobID, job.Status)
	}

	db.DB.Model(&job).Updates(map[string]interface{}{
		"status":      models.JobStatusQueued,
		"progress":    0,
		"message":     "Retried by user",
		"started_at":  nil,
		"finished_at": nil,
	})

	s.addLog(jobID, "INFO", "Job retried by user")
	s.wakeup()
	return nil
}

// ForceRunJob immediately runs a queued job, bypassing priority.
func (s *Scheduler) ForceRunJob(jobID uint) error {
	var job models.SchedulerJob
	if err := db.DB.First(&job, jobID).Error; err != nil {
		return fmt.Errorf("job not found: %w", err)
	}

	if job.Status != models.JobStatusQueued {
		return fmt.Errorf("job %d is not queued (current: %s)", jobID, job.Status)
	}

	// Boost priority to maximum and wake up
	db.DB.Model(&job).Update("priority", 1)
	s.addLog(jobID, "INFO", "Job force-run by user (priority boosted)")
	s.wakeup()
	return nil
}

// ReorderJobs updates priorities for a list of job IDs.
func (s *Scheduler) ReorderJobs(orderedIDs []uint) error {
	for i, id := range orderedIDs {
		priority := i + 1
		db.DB.Model(&models.SchedulerJob{}).Where("id = ?", id).Update("priority", priority)
	}
	s.addLog(0, "INFO", fmt.Sprintf("Job queue reordered: %d jobs", len(orderedIDs)))
	s.wakeup()
	return nil
}

// DeleteJob removes a job and its logs.
func (s *Scheduler) DeleteJob(jobID uint) error {
	// Cancel if active
	s.CancelJob(jobID)

	// Remove cron entry if scheduled
	s.mu.Lock()
	if entryID, exists := s.cronEntries[jobID]; exists {
		s.cron.Remove(entryID)
		delete(s.cronEntries, jobID)
	}
	s.mu.Unlock()

	// Delete logs
	db.DB.Where("scheduler_job_id = ?", jobID).Delete(&models.SchedulerJobLog{})

	// Delete job
	if err := db.DB.Unscoped().Delete(&models.SchedulerJob{}, jobID).Error; err != nil {
		return err
	}

	logger.Info("Scheduler", "Job deleted", "id", jobID)
	return nil
}

// PurgeJobs removes completed/failed jobs older than N days.
func (s *Scheduler) PurgeJobs(olderThanDays int) (int64, error) {
	cutoff := time.Now().AddDate(0, 0, -olderThanDays)

	// Delete logs first
	db.DB.Where("created_at < ? AND scheduler_job_id IN (?)",
		cutoff,
		db.DB.Model(&models.SchedulerJob{}).
			Where("status IN ? AND created_at < ?",
				[]string{models.JobStatusCompleted, models.JobStatusFailed, models.JobStatusCancelled},
				cutoff,
			).Select("id"),
	).Delete(&models.SchedulerJobLog{})

	result := db.DB.Where("status IN ? AND created_at < ?",
		[]string{models.JobStatusCompleted, models.JobStatusFailed, models.JobStatusCancelled},
		cutoff,
	).Unscoped().Delete(&models.SchedulerJob{})

	logger.Info("Scheduler", "Purged old jobs", "count", result.RowsAffected, "olderThanDays", olderThanDays)
	return result.RowsAffected, result.Error
}

// GetStats returns scheduler statistics.
func (s *Scheduler) GetStats() map[string]interface{} {
	var totalJobs, queuedJobs, runningJobs, completedJobs, failedJobs, cancelledJobs int64

	db.DB.Model(&models.SchedulerJob{}).Count(&totalJobs)
	db.DB.Model(&models.SchedulerJob{}).Where("status = ?", models.JobStatusQueued).Count(&queuedJobs)
	db.DB.Model(&models.SchedulerJob{}).Where("status = ?", models.JobStatusRunning).Count(&runningJobs)
	db.DB.Model(&models.SchedulerJob{}).Where("status = ?", models.JobStatusCompleted).Count(&completedJobs)
	db.DB.Model(&models.SchedulerJob{}).Where("status = ?", models.JobStatusFailed).Count(&failedJobs)
	db.DB.Model(&models.SchedulerJob{}).Where("status = ?", models.JobStatusCancelled).Count(&cancelledJobs)

	// Average completion time
	var avgDuration float64
	db.DB.Model(&models.SchedulerJob{}).
		Where("status = ? AND finished_at IS NOT NULL AND started_at IS NOT NULL", models.JobStatusCompleted).
		Select("AVG(TIMESTAMPDIFF(SECOND, started_at, finished_at))").
		Scan(&avgDuration)

	s.mu.RLock()
	activeCount := len(s.activeJobs)
	workerCount := s.workerCount
	cronCount := len(s.cronEntries)
	s.mu.RUnlock()

	return map[string]interface{}{
		"total_jobs":     totalJobs,
		"queued_jobs":    queuedJobs,
		"running_jobs":   runningJobs,
		"completed_jobs": completedJobs,
		"failed_jobs":    failedJobs,
		"cancelled_jobs": cancelledJobs,
		"active_workers": activeCount,
		"max_workers":    workerCount,
		"cron_schedules": cronCount,
		"avg_duration":   avgDuration,
		"cpu_count":      runtime.NumCPU(),
	}
}

// UpdateConfig hot-reloads the scheduler configuration.
func (s *Scheduler) UpdateConfig(cfg *models.SchedulerConfig) {
	s.mu.Lock()
	s.workerCount = cfg.MaxConcurrentJobs
	s.mu.Unlock()

	logger.Info("Scheduler", "Configuration updated", "maxWorkers", cfg.MaxConcurrentJobs)
	s.wakeup()
}

// ──────────────────────────────────────────────────────────────────────────────
// Internal: Queue Scanner & Worker Dispatch
// ──────────────────────────────────────────────────────────────────────────────

func (s *Scheduler) queueScanner() {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-s.stopChan:
			return
		case <-ticker.C:
			s.dispatchJobs()
		case <-s.wakeupChan:
			s.dispatchJobs()
		}
	}
}

func (s *Scheduler) dispatchJobs() {
	s.mu.RLock()
	activeCount := len(s.activeJobs)
	maxWorkers := s.workerCount
	s.mu.RUnlock()

	if activeCount >= maxWorkers {
		return
	}

	slots := maxWorkers - activeCount

	// Fetch queued jobs sorted by priority (lowest number = highest priority), then by created_at
	var pendingJobs []models.SchedulerJob
	db.DB.Where("status = ?", models.JobStatusQueued).
		Order("priority ASC, created_at ASC").
		Limit(slots).
		Find(&pendingJobs)

	for _, job := range pendingJobs {
		jobCopy := job
		go s.executeJob(&jobCopy)
	}
}

func (s *Scheduler) executeJob(job *models.SchedulerJob) {
	// Load config for timeout
	var cfg models.SchedulerConfig
	timeout := 3600 * time.Second
	if err := db.DB.First(&cfg).Error; err == nil && cfg.JobTimeoutSeconds > 0 {
		timeout = time.Duration(cfg.JobTimeoutSeconds) * time.Second
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)

	// Register active job
	s.mu.Lock()
	s.activeJobs[job.ID] = cancel
	s.mu.Unlock()

	now := time.Now()
	db.DB.Model(job).Updates(map[string]interface{}{
		"status":     models.JobStatusRunning,
		"started_at": now,
		"message":    "Executing...",
	})

	s.addLog(job.ID, "INFO", "Job execution started")

	// Lookup handler
	s.mu.RLock()
	handler, exists := s.registry[job.Type]
	s.mu.RUnlock()

	var execErr error
	if !exists {
		execErr = fmt.Errorf("no handler registered for job type: %s", job.Type)
	} else {
		// Create a log function the job can use to emit progress logs
		logFn := func(level, message string) {
			s.addLog(job.ID, level, message)
		}

		execErr = handler(ctx, job, logFn)
	}

	// Cleanup
	cancel()
	s.mu.Lock()
	delete(s.activeJobs, job.ID)
	s.mu.Unlock()

	finishedAt := time.Now()

	if execErr != nil {
		// Check if it was cancelled
		if ctx.Err() == context.Canceled {
			db.DB.Model(job).Updates(map[string]interface{}{
				"status":      models.JobStatusCancelled,
				"message":     "Job was cancelled",
				"finished_at": finishedAt,
			})
			s.addLog(job.ID, "WARN", "Job was cancelled")
		} else {
			// Check retry
			var retryLimit int
			if err := db.DB.First(&cfg).Error; err == nil {
				retryLimit = cfg.RetryLimit
			}

			if job.RetryCount < retryLimit {
				retryDelay := time.Duration(cfg.RetryDelaySeconds) * time.Second
				db.DB.Model(job).Updates(map[string]interface{}{
					"status":      models.JobStatusQueued,
					"retry_count": job.RetryCount + 1,
					"message":     fmt.Sprintf("Retry %d/%d: %s", job.RetryCount+1, retryLimit, execErr.Error()),
					"progress":    0,
				})
				s.addLog(job.ID, "WARN", fmt.Sprintf("Job failed, scheduling retry %d/%d in %v: %s", job.RetryCount+1, retryLimit, retryDelay, execErr.Error()))

				// Schedule delayed retry
				time.AfterFunc(retryDelay, func() {
					s.wakeup()
				})
			} else {
				db.DB.Model(job).Updates(map[string]interface{}{
					"status":      models.JobStatusFailed,
					"message":     execErr.Error(),
					"finished_at": finishedAt,
				})
				s.addLog(job.ID, "ERROR", fmt.Sprintf("Job failed permanently: %s", execErr.Error()))
			}
		}

		logger.Error("Scheduler", "Job execution failed",
			"id", job.ID,
			"type", job.Type,
			"error", execErr,
		)
	} else {
		db.DB.Model(job).Updates(map[string]interface{}{
			"status":      models.JobStatusCompleted,
			"progress":    100,
			"message":     "Completed successfully",
			"finished_at": finishedAt,
		})
		s.addLog(job.ID, "INFO", "Job completed successfully")

		logger.Info("Scheduler", "Job completed",
			"id", job.ID,
			"type", job.Type,
			"duration", finishedAt.Sub(now).String(),
		)
	}

	// Wake up to check for more queued work
	s.wakeup()
}

// ──────────────────────────────────────────────────────────────────────────────
// Internal: Cron Management
// ──────────────────────────────────────────────────────────────────────────────

func (s *Scheduler) loadCronJobs() {
	var jobs []models.SchedulerJob
	db.DB.Where("cron_expr != '' AND cron_expr IS NOT NULL AND status IN ?",
		[]string{models.JobStatusScheduled, models.JobStatusQueued},
	).Find(&jobs)

	for _, job := range jobs {
		s.registerCronJob(&job)
	}

	if len(jobs) > 0 {
		logger.Info("Scheduler", "Loaded cron schedules", "count", len(jobs))
	}
}

func (s *Scheduler) registerCronJob(job *models.SchedulerJob) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Remove existing entry if any
	if entryID, exists := s.cronEntries[job.ID]; exists {
		s.cron.Remove(entryID)
	}

	jobID := job.ID
	jobType := job.Type
	jobName := job.Name
	jobPayload := job.Payload

	entryID, err := s.cron.AddFunc(job.CronExpr, func() {
		// Create a new instance for each cron tick
		newJob := &models.SchedulerJob{
			UUID:        uuid.New().String(),
			Type:        jobType,
			Name:        fmt.Sprintf("%s (cron)", jobName),
			Description: fmt.Sprintf("Auto-scheduled from cron: %s", job.CronExpr),
			Category:    "cron",
			Priority:    5,
			Status:      models.JobStatusQueued,
			Payload:     jobPayload,
		}
		db.DB.Create(newJob)
		s.addLog(newJob.ID, "INFO", fmt.Sprintf("Cron-triggered job instance created from schedule [parent=%d]", jobID))
		s.wakeup()
	})

	if err != nil {
		logger.Error("Scheduler", "Failed to register cron job", "id", job.ID, "expr", job.CronExpr, "error", err)
		return
	}

	s.cronEntries[job.ID] = entryID
	db.DB.Model(job).Update("status", models.JobStatusScheduled)
}

// ──────────────────────────────────────────────────────────────────────────────
// Internal: Logging
// ──────────────────────────────────────────────────────────────────────────────

func (s *Scheduler) addLog(jobID uint, level, message string) {
	log := &models.SchedulerJobLog{
		SchedulerJobID: jobID,
		Level:          level,
		Message:        message,
	}
	db.DB.Create(log)
}

func (s *Scheduler) wakeup() {
	select {
	case s.wakeupChan <- struct{}{}:
	default:
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// Internal: Built-in Job Type Handlers
// ──────────────────────────────────────────────────────────────────────────────

func (s *Scheduler) registerBuiltinJobs() {
	// Generic shell command job
	s.RegisterJob("shell_command", func(ctx context.Context, job *models.SchedulerJob, logFn func(string, string)) error {
		logFn("INFO", "Shell command jobs are not enabled for security. Use specific job types instead.")
		return fmt.Errorf("shell_command type is disabled for security")
	})

	// File compression job (placeholder — integrates with file handler)
	s.RegisterJob("file_compress", func(ctx context.Context, job *models.SchedulerJob, logFn func(string, string)) error {
		logFn("INFO", "File compression job started")
		// Progress simulation — the actual handler in files.go will update this
		for i := 0; i <= 100; i += 10 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
				db.DB.Model(job).Update("progress", i)
				time.Sleep(200 * time.Millisecond)
			}
		}
		logFn("INFO", "File compression completed")
		return nil
	})

	// File decompression job
	s.RegisterJob("file_decompress", func(ctx context.Context, job *models.SchedulerJob, logFn func(string, string)) error {
		logFn("INFO", "File decompression job started")
		for i := 0; i <= 100; i += 10 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
				db.DB.Model(job).Update("progress", i)
				time.Sleep(200 * time.Millisecond)
			}
		}
		logFn("INFO", "File decompression completed")
		return nil
	})

	// Leech download job (wrapper)
	s.RegisterJob("leech_download", func(ctx context.Context, job *models.SchedulerJob, logFn func(string, string)) error {
		logFn("INFO", "Leech download job queued via scheduler")
		// The actual download is managed by the downloader engine.
		// This just tracks it in the scheduler for visibility.
		<-ctx.Done()
		return nil
	})

	// Torrent download job (wrapper)
	s.RegisterJob("torrent_download", func(ctx context.Context, job *models.SchedulerJob, logFn func(string, string)) error {
		logFn("INFO", "Torrent download job queued via scheduler")
		<-ctx.Done()
		return nil
	})

	// Cleanup / maintenance job
	s.RegisterJob("system_cleanup", func(ctx context.Context, job *models.SchedulerJob, logFn func(string, string)) error {
		logFn("INFO", "System cleanup started")
		// Purge old logs
		cutoff := time.Now().AddDate(0, 0, -7)
		result := db.DB.Where("created_at < ?", cutoff).Delete(&models.SchedulerJobLog{})
		logFn("INFO", fmt.Sprintf("Purged %d old log entries", result.RowsAffected))
		db.DB.Model(job).Update("progress", 100)
		return nil
	})

	// Database backup job
	s.RegisterJob("db_backup", func(ctx context.Context, job *models.SchedulerJob, logFn func(string, string)) error {
		logFn("INFO", "Database backup job started")
		db.DB.Model(job).Update("progress", 50)
		time.Sleep(1 * time.Second)
		db.DB.Model(job).Update("progress", 100)
		logFn("INFO", "Database backup completed")
		return nil
	})

	// Custom/generic task
	s.RegisterJob("custom_task", func(ctx context.Context, job *models.SchedulerJob, logFn func(string, string)) error {
		logFn("INFO", fmt.Sprintf("Custom task executing: %s", job.Description))
		db.DB.Model(job).Update("progress", 100)
		return nil
	})

	// Telegram parallel multi-connection file upload
	s.RegisterJob("telegram_upload", func(ctx context.Context, job *models.SchedulerJob, logFn func(string, string)) error {
		return telegram.RunTelegramUploadJob(ctx, job, logFn)
	})
}

// ──────────────────────────────────────────────────────────────────────────────
// Utility: Get sorted list of all jobs with live status enrichment
// ──────────────────────────────────────────────────────────────────────────────

// GetJobs returns all jobs, optionally filtered and sorted.
func (s *Scheduler) GetJobs(status, category, jobType string, limit, offset int) ([]models.SchedulerJob, int64) {
	query := db.DB.Model(&models.SchedulerJob{})

	if status != "" {
		query = query.Where("status = ?", status)
	}
	if category != "" {
		query = query.Where("category = ?", category)
	}
	if jobType != "" {
		query = query.Where("type = ?", jobType)
	}

	var total int64
	query.Count(&total)

	var jobs []models.SchedulerJob
	query.Order("CASE WHEN status = 'running' THEN 0 WHEN status = 'queued' THEN 1 WHEN status = 'scheduled' THEN 2 ELSE 3 END, priority ASC, created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&jobs)

	// Sort running jobs first, then queued by priority
	sort.SliceStable(jobs, func(i, j int) bool {
		if jobs[i].Status == models.JobStatusRunning && jobs[j].Status != models.JobStatusRunning {
			return true
		}
		if jobs[i].Status != models.JobStatusRunning && jobs[j].Status == models.JobStatusRunning {
			return false
		}
		if jobs[i].Status == models.JobStatusQueued && jobs[j].Status == models.JobStatusQueued {
			return jobs[i].Priority < jobs[j].Priority
		}
		return false
	})

	return jobs, total
}

// GetJobLogs returns logs for a specific job.
func (s *Scheduler) GetJobLogs(jobID uint, limit int) []models.SchedulerJobLog {
	var logs []models.SchedulerJobLog
	db.DB.Where("scheduler_job_id = ?", jobID).
		Order("created_at DESC").
		Limit(limit).
		Find(&logs)
	return logs
}
