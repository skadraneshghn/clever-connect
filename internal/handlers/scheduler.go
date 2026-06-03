package handlers

import (
	"net/http"
	"strconv"

	"clever-connect/internal/config"
	"clever-connect/internal/db"
	"clever-connect/internal/models"
	"clever-connect/internal/scheduler"

	"github.com/gin-gonic/gin"
)

// ──────────────────────────────────────────────────────────────────────────────
// Enterprise Job Scheduler REST API Handler
// ──────────────────────────────────────────────────────────────────────────────

type SchedulerHandler struct {
	cfg *config.Config
}

func NewSchedulerHandler(cfg *config.Config) *SchedulerHandler {
	return &SchedulerHandler{cfg: cfg}
}

// ListJobs returns all scheduler jobs with optional filters.
// GET /api/scheduler/jobs?status=running&category=files&type=file_compress&limit=50&offset=0
func (h *SchedulerHandler) ListJobs(c *gin.Context) {
	status := c.Query("status")
	category := c.Query("category")
	jobType := c.Query("type")

	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "100"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))

	if limit <= 0 || limit > 500 {
		limit = 100
	}

	if scheduler.Engine == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Scheduler engine not initialized"})
		return
	}

	jobs, total := scheduler.Engine.GetJobs(status, category, jobType, limit, offset)

	c.JSON(http.StatusOK, gin.H{
		"jobs":  jobs,
		"total": total,
	})
}

// CreateJob submits a new job to the scheduler.
// POST /api/scheduler/jobs
func (h *SchedulerHandler) CreateJob(c *gin.Context) {
	var input struct {
		Type        string `json:"type" binding:"required"`
		Name        string `json:"name" binding:"required"`
		Description string `json:"description"`
		Category    string `json:"category"`
		Priority    int    `json:"priority"`
		Payload     string `json:"payload"`
		CronExpr    string `json:"cron_expr"`
	}

	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body", "details": err.Error()})
		return
	}

	if scheduler.Engine == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Scheduler engine not initialized"})
		return
	}

	job, err := scheduler.Engine.SubmitJob(
		input.Type,
		input.Name,
		input.Description,
		input.Category,
		input.Priority,
		input.Payload,
		input.CronExpr,
	)

	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to submit job", "details": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "submitted",
		"job":    job,
	})
}

// CancelJob cancels a running or queued job.
// POST /api/scheduler/jobs/:id/cancel
func (h *SchedulerHandler) CancelJob(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid job ID"})
		return
	}

	if scheduler.Engine == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Scheduler engine not initialized"})
		return
	}

	if err := scheduler.Engine.CancelJob(uint(id)); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "cancelled", "job_id": id})
}

// RetryJob retries a failed or cancelled job.
// POST /api/scheduler/jobs/:id/retry
func (h *SchedulerHandler) RetryJob(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid job ID"})
		return
	}

	if scheduler.Engine == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Scheduler engine not initialized"})
		return
	}

	if err := scheduler.Engine.RetryJob(uint(id)); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "retried", "job_id": id})
}

// ForceRunJob force-runs a queued job immediately.
// POST /api/scheduler/jobs/:id/force
func (h *SchedulerHandler) ForceRunJob(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid job ID"})
		return
	}

	if scheduler.Engine == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Scheduler engine not initialized"})
		return
	}

	if err := scheduler.Engine.ForceRunJob(uint(id)); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "force_run", "job_id": id})
}

// DeleteJob removes a job and its logs.
// POST /api/scheduler/jobs/:id/delete
func (h *SchedulerHandler) DeleteJob(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid job ID"})
		return
	}

	if scheduler.Engine == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Scheduler engine not initialized"})
		return
	}

	if err := scheduler.Engine.DeleteJob(uint(id)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "deleted", "job_id": id})
}

// ReorderJobs updates priorities for job queue reordering.
// POST /api/scheduler/jobs/reorder
func (h *SchedulerHandler) ReorderJobs(c *gin.Context) {
	var input struct {
		OrderedIDs []uint `json:"ordered_ids" binding:"required"`
	}

	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body", "details": err.Error()})
		return
	}

	if scheduler.Engine == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Scheduler engine not initialized"})
		return
	}

	if err := scheduler.Engine.ReorderJobs(input.OrderedIDs); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "reordered"})
}

// GetJobLogs returns execution logs for a specific job.
// GET /api/scheduler/jobs/:id/logs?limit=100
func (h *SchedulerHandler) GetJobLogs(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid job ID"})
		return
	}

	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "100"))
	if limit <= 0 || limit > 500 {
		limit = 100
	}

	if scheduler.Engine == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Scheduler engine not initialized"})
		return
	}

	logs := scheduler.Engine.GetJobLogs(uint(id), limit)
	c.JSON(http.StatusOK, logs)
}

// GetConfig returns the scheduler configuration.
// GET /api/scheduler/config
func (h *SchedulerHandler) GetConfig(c *gin.Context) {
	var cfg models.SchedulerConfig
	if err := db.DB.First(&cfg).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load scheduler config"})
		return
	}

	c.JSON(http.StatusOK, cfg)
}

// SaveConfig updates the scheduler configuration.
// POST /api/scheduler/config
func (h *SchedulerHandler) SaveConfig(c *gin.Context) {
	var input models.SchedulerConfig
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid config payload", "details": err.Error()})
		return
	}

	// Validate constraints
	if input.MaxConcurrentJobs < 1 {
		input.MaxConcurrentJobs = 1
	}
	if input.MaxConcurrentJobs > 32 {
		input.MaxConcurrentJobs = 32
	}
	if input.DefaultPriority < 1 {
		input.DefaultPriority = 1
	}
	if input.DefaultPriority > 10 {
		input.DefaultPriority = 10
	}

	var cfg models.SchedulerConfig
	if err := db.DB.First(&cfg).Error; err == nil {
		cfg.MaxConcurrentJobs = input.MaxConcurrentJobs
		cfg.DefaultPriority = input.DefaultPriority
		cfg.RetryLimit = input.RetryLimit
		cfg.RetryDelaySeconds = input.RetryDelaySeconds
		cfg.JobTimeoutSeconds = input.JobTimeoutSeconds
		cfg.PurgeAfterDays = input.PurgeAfterDays
		cfg.EnableCronJobs = input.EnableCronJobs
		cfg.EnableNotifications = input.EnableNotifications
		db.DB.Save(&cfg)
	} else {
		db.DB.Create(&input)
		cfg = input
	}

	// Hot-reload the scheduler engine
	if scheduler.Engine != nil {
		scheduler.Engine.UpdateConfig(&cfg)
	}

	c.JSON(http.StatusOK, gin.H{"status": "saved"})
}

// GetStats returns scheduler statistics.
// GET /api/scheduler/stats
func (h *SchedulerHandler) GetStats(c *gin.Context) {
	if scheduler.Engine == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Scheduler engine not initialized"})
		return
	}

	stats := scheduler.Engine.GetStats()
	c.JSON(http.StatusOK, stats)
}

// PurgeJobs removes old completed/failed jobs.
// POST /api/scheduler/purge
func (h *SchedulerHandler) PurgeJobs(c *gin.Context) {
	var input struct {
		OlderThanDays int `json:"older_than_days"`
	}

	if err := c.ShouldBindJSON(&input); err != nil {
		input.OlderThanDays = 30
	}

	if input.OlderThanDays <= 0 {
		input.OlderThanDays = 30
	}

	if scheduler.Engine == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Scheduler engine not initialized"})
		return
	}

	count, err := scheduler.Engine.PurgeJobs(input.OlderThanDays)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "purged", "count": count})
}
