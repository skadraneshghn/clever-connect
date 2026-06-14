package client

import (
	"sync"
	"time"

	"clever-connect/internal/bonding/control"
	"clever-connect/internal/bonding/frame"
	"clever-connect/internal/logger"
)

// Dispatcher routes framed data across available artery connections
// using the adaptive scheduler from the control package.
//
// DESIGN NOTE (from expert review): The dispatcher now enforces cwnd-based
// backpressure. Before sending a frame on a selected path, it checks whether
// the path has available capacity (InFlight < CWnd). If all paths are
// congested, the dispatcher blocks with exponential backoff until a slot
// opens, preventing buffer bloat on weak routes.
type Dispatcher struct {
	mu sync.RWMutex

	scheduler *control.Scheduler
	arteries  []*ArteryConn
	metrics   map[string]*control.PathMetrics

	// Backpressure configuration
	maxPaceWait time.Duration // max time to wait for cwnd capacity (default: 500ms)
}

// NewDispatcher creates a frame dispatcher with the given scheduler and arteries.
func NewDispatcher(mode control.ScheduleMode, arteries []*ArteryConn, metrics map[string]*control.PathMetrics) *Dispatcher {
	pathMetrics := make([]*control.PathMetrics, 0, len(metrics))
	for _, m := range metrics {
		pathMetrics = append(pathMetrics, m)
	}

	return &Dispatcher{
		scheduler:   control.NewScheduler(mode, pathMetrics),
		arteries:    arteries,
		metrics:     metrics,
		maxPaceWait: 500 * time.Millisecond,
	}
}

// DispatchFrame sends a frame to the appropriate artery(s) based on
// the scheduler's current mode and per-path metrics.
// Enforces cwnd backpressure: blocks if all selected paths are congested.
func (d *Dispatcher) DispatchFrame(f *frame.Frame) error {
	d.mu.RLock()
	defer d.mu.RUnlock()

	result := d.scheduler.Schedule(f.StreamID, len(f.Payload))

	if len(result.Paths) == 0 {
		return ErrNoActiveArteries
	}

	// Map selected PathMetrics → ArteryConns with cwnd backpressure
	var lastErr error
	for _, selectedPath := range result.Paths {
		snap := selectedPath.Snapshot()
		artery := d.findArtery(snap.PathID)
		if artery == nil || !artery.IsAlive() {
			continue
		}

		// ── BACKPRESSURE GATE ─────────────────────────────────────────
		// Block if this path's congestion window is exhausted.
		// This prevents the engine from flooding packets continuously
		// without matching window constraints, which would cause buffer
		// bloat on weak routes and defeat the AIMD adjustments.
		if !d.waitForCapacity(selectedPath) {
			logger.Warn("Bonding", "Path congested, skipping frame dispatch",
				"artery", artery.Tag(),
				"in_flight", snap.InFlight,
				"cwnd", snap.CWnd,
			)
			continue
		}

		if err := artery.WriteFrame(f); err != nil {
			lastErr = err
			logger.Warn("Bonding", "Frame dispatch write failed",
				"artery", artery.Tag(), "error", err)
			// Record loss for scheduler
			selectedPath.RecordLoss()
			continue
		}

		// Record successful send (increments InFlight)
		selectedPath.RecordSend()
	}

	return lastErr
}

// waitForCapacity blocks until the path has room in its cwnd, or times out.
// Returns true if capacity became available, false if timed out.
func (d *Dispatcher) waitForCapacity(path *control.PathMetrics) bool {
	// Fast path: check immediately
	if path.HasCapacity() {
		return true
	}

	// Slow path: exponential backoff wait for capacity
	deadline := time.Now().Add(d.maxPaceWait)
	backoff := time.Millisecond // start at 1ms

	for time.Now().Before(deadline) {
		time.Sleep(backoff)

		if path.HasCapacity() {
			return true
		}

		// Exponential backoff: 1ms → 2ms → 4ms → 8ms → ... capped at 50ms
		backoff *= 2
		if backoff > 50*time.Millisecond {
			backoff = 50 * time.Millisecond
		}
	}

	return false // timed out — path still congested
}

// findArtery finds the ArteryConn matching a path ID.
func (d *Dispatcher) findArtery(pathID string) *ArteryConn {
	for _, a := range d.arteries {
		if a.Tag() == pathID {
			return a
		}
	}
	return nil
}

// UpdateArteries updates the set of available arteries and metrics.
func (d *Dispatcher) UpdateArteries(arteries []*ArteryConn, metrics map[string]*control.PathMetrics) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.arteries = arteries
	d.metrics = metrics

	pathMetrics := make([]*control.PathMetrics, 0, len(metrics))
	for _, m := range metrics {
		pathMetrics = append(pathMetrics, m)
	}
	d.scheduler.SetPaths(pathMetrics)
}

// SetMode changes the dispatch mode (stripe/duplicate/auto).
func (d *Dispatcher) SetMode(mode control.ScheduleMode) {
	d.scheduler.SetMode(mode)
}

// CleanupFlow removes per-flow tracking state for a closed stream.
func (d *Dispatcher) CleanupFlow(streamID uint32) {
	d.scheduler.CleanupFlow(streamID)
}
