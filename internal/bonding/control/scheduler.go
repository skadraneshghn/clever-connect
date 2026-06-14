package control

import (
	"sort"
	"sync"
)

// ──────────────────────────────────────────────────────────────────────────────
// Scheduler — decides which path(s) a frame is dispatched to
// ──────────────────────────────────────────────────────────────────────────────

// ScheduleMode controls how frames are distributed across paths.
type ScheduleMode int

const (
	// ModeDuplicate sends the frame to ALL active paths (racing).
	// First arrival wins; duplicates are silently dropped by dedup.
	// Best for: interactive flows, small transfers, low-latency-critical.
	ModeDuplicate ScheduleMode = iota

	// ModeStripe sends the frame to exactly ONE path chosen by the scheduler.
	// Achieves aggregate throughput > any single path.
	// Best for: bulk transfers, video streaming, large downloads.
	ModeStripe

	// ModeAuto starts with Duplicate for small/interactive flows and
	// upgrades to Stripe when a flow crosses the threshold. One-way upgrade only.
	ModeAuto
)

// String returns the human-readable name of the schedule mode.
func (m ScheduleMode) String() string {
	switch m {
	case ModeDuplicate:
		return "duplicate"
	case ModeStripe:
		return "stripe"
	case ModeAuto:
		return "auto"
	default:
		return "unknown"
	}
}

// ParseScheduleMode converts a string to a ScheduleMode.
func ParseScheduleMode(s string) ScheduleMode {
	switch s {
	case "duplicate", "dup":
		return ModeDuplicate
	case "stripe":
		return ModeStripe
	case "auto":
		return ModeAuto
	default:
		return ModeAuto
	}
}

// AutoModeThresholds controls when ModeAuto upgrades from duplicate to stripe.
type AutoModeThresholds struct {
	ByteThreshold    int64 // upgrade after this many bytes (default: 1MB)
	DurationThreshold int64 // upgrade after this many milliseconds (default: 3000ms)
}

// DefaultAutoThresholds returns the default auto-mode upgrade thresholds.
func DefaultAutoThresholds() AutoModeThresholds {
	return AutoModeThresholds{
		ByteThreshold:    1 << 20, // 1 MB
		DurationThreshold: 3000,    // 3 seconds
	}
}

// Scheduler selects which path(s) a frame should be sent on.
type Scheduler struct {
	mu sync.RWMutex

	mode       ScheduleMode
	paths      []*PathMetrics
	autoThresh AutoModeThresholds

	// Per-flow state for auto mode
	flowBytes    map[uint32]int64 // StreamID → cumulative bytes
	flowUpgraded map[uint32]bool  // StreamID → already upgraded to stripe?
}

// NewScheduler creates a scheduler with the given mode and path metrics.
func NewScheduler(mode ScheduleMode, paths []*PathMetrics) *Scheduler {
	return &Scheduler{
		mode:         mode,
		paths:        paths,
		autoThresh:   DefaultAutoThresholds(),
		flowBytes:    make(map[uint32]int64),
		flowUpgraded: make(map[uint32]bool),
	}
}

// SetMode changes the scheduling mode.
func (s *Scheduler) SetMode(mode ScheduleMode) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.mode = mode
}

// GetMode returns the current scheduling mode.
func (s *Scheduler) GetMode() ScheduleMode {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.mode
}

// SetPaths updates the set of available paths.
func (s *Scheduler) SetPaths(paths []*PathMetrics) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.paths = paths
}

// ScheduleResult indicates which paths a frame should be sent on.
type ScheduleResult struct {
	Paths    []*PathMetrics // the selected path(s)
	Mode     ScheduleMode   // the actual mode used for this frame
}

// Schedule selects path(s) for a frame with the given streamID and payload size.
// Returns the selected paths and the effective mode used.
func (s *Scheduler) Schedule(streamID uint32, payloadSize int) ScheduleResult {
	s.mu.Lock()
	defer s.mu.Unlock()

	activePaths := s.getActivePaths()
	if len(activePaths) == 0 {
		return ScheduleResult{Paths: nil, Mode: s.mode}
	}

	effectiveMode := s.mode

	// Auto mode: decide based on per-flow state
	if s.mode == ModeAuto {
		s.flowBytes[streamID] += int64(payloadSize)
		if s.flowUpgraded[streamID] {
			effectiveMode = ModeStripe
		} else if s.flowBytes[streamID] > s.autoThresh.ByteThreshold {
			// One-way upgrade: dup → stripe
			s.flowUpgraded[streamID] = true
			effectiveMode = ModeStripe
		} else {
			effectiveMode = ModeDuplicate
		}
	}

	switch effectiveMode {
	case ModeDuplicate:
		return ScheduleResult{Paths: activePaths, Mode: ModeDuplicate}
	case ModeStripe:
		best := s.selectStripePath(activePaths)
		return ScheduleResult{Paths: []*PathMetrics{best}, Mode: ModeStripe}
	default:
		return ScheduleResult{Paths: activePaths, Mode: ModeDuplicate}
	}
}

// selectStripePath implements the ECF/BLEST-inspired stripe scheduler.
// It selects the path with the lowest estimated completion time (minRTT + cwnd gating).
func (s *Scheduler) selectStripePath(paths []*PathMetrics) *PathMetrics {
	if len(paths) == 1 {
		return paths[0]
	}

	// Sort by estimated completion time: minRTT / (cwnd - inflight)
	type candidate struct {
		path *PathMetrics
		ect  float64 // estimated completion time
	}

	candidates := make([]candidate, 0, len(paths))
	for _, p := range paths {
		snap := p.Snapshot()
		available := snap.CWnd - snap.InFlight
		if available <= 0 {
			continue // path is congested, skip
		}
		minRTT := snap.MinRTT
		if minRTT <= 0 {
			minRTT = snap.SRTT
		}
		if minRTT <= 0 {
			minRTT = 1000 // fallback: 1s if no RTT measured yet
		}
		ect := minRTT / float64(available)
		candidates = append(candidates, candidate{path: p, ect: ect})
	}

	if len(candidates) == 0 {
		// All paths congested — pick the one with lowest SRTT
		best := paths[0]
		bestSRTT := paths[0].Snapshot().SRTT
		for _, p := range paths[1:] {
			if snap := p.Snapshot(); snap.SRTT < bestSRTT {
				best = p
				bestSRTT = snap.SRTT
			}
		}
		return best
	}

	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].ect < candidates[j].ect
	})

	return candidates[0].path
}

// getActivePaths returns paths that are alive and have capacity.
func (s *Scheduler) getActivePaths() []*PathMetrics {
	active := make([]*PathMetrics, 0, len(s.paths))
	for _, p := range s.paths {
		if p.Snapshot().Alive {
			active = append(active, p)
		}
	}
	return active
}

// CleanupFlow removes per-flow tracking state for a closed stream.
func (s *Scheduler) CleanupFlow(streamID uint32) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.flowBytes, streamID)
	delete(s.flowUpgraded, streamID)
}

// MedianSRTT returns the median SRTT across all active paths.
func (s *Scheduler) MedianSRTT() float64 {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var srtts []float64
	for _, p := range s.paths {
		snap := p.Snapshot()
		if snap.Alive && snap.SRTT > 0 {
			srtts = append(srtts, snap.SRTT)
		}
	}

	if len(srtts) == 0 {
		return 0
	}

	sort.Float64s(srtts)
	mid := len(srtts) / 2
	if len(srtts)%2 == 0 {
		return (srtts[mid-1] + srtts[mid]) / 2
	}
	return srtts[mid]
}

// BestSRTT returns the lowest SRTT across all active paths.
func (s *Scheduler) BestSRTT() float64 {
	s.mu.RLock()
	defer s.mu.RUnlock()

	best := float64(0)
	for _, p := range s.paths {
		snap := p.Snapshot()
		if snap.Alive && snap.SRTT > 0 {
			if best == 0 || snap.SRTT < best {
				best = snap.SRTT
			}
		}
	}
	return best
}
