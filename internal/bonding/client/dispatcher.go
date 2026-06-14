package client

import (
	"sync"

	"clever-connect/internal/bonding/control"
	"clever-connect/internal/bonding/frame"
	"clever-connect/internal/logger"
)

// Dispatcher routes framed data across available artery connections
// using the adaptive scheduler from the control package.
type Dispatcher struct {
	mu sync.RWMutex

	scheduler *control.Scheduler
	arteries  []*ArteryConn
	metrics   map[string]*control.PathMetrics
}

// NewDispatcher creates a frame dispatcher with the given scheduler and arteries.
func NewDispatcher(mode control.ScheduleMode, arteries []*ArteryConn, metrics map[string]*control.PathMetrics) *Dispatcher {
	pathMetrics := make([]*control.PathMetrics, 0, len(metrics))
	for _, m := range metrics {
		pathMetrics = append(pathMetrics, m)
	}

	return &Dispatcher{
		scheduler: control.NewScheduler(mode, pathMetrics),
		arteries:  arteries,
		metrics:   metrics,
	}
}

// DispatchFrame sends a frame to the appropriate artery(s) based on
// the scheduler's current mode and per-path metrics.
func (d *Dispatcher) DispatchFrame(f *frame.Frame) error {
	d.mu.RLock()
	defer d.mu.RUnlock()

	result := d.scheduler.Schedule(f.StreamID, len(f.Payload))

	if len(result.Paths) == 0 {
		return ErrNoActiveArteries
	}

	// Map selected PathMetrics → ArteryConns
	var lastErr error
	for _, selectedPath := range result.Paths {
		snap := selectedPath.Snapshot()
		artery := d.findArtery(snap.PathID)
		if artery == nil || !artery.IsAlive() {
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

		// Record successful send
		selectedPath.RecordSend()
	}

	return lastErr
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
