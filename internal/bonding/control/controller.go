package control

import (
	"math"
	"sync"
	"time"
)

// ──────────────────────────────────────────────────────────────────────────────
// AdaptiveController — ties RTT/loss metrics, scheduler mode, and path states
// together into a single decision loop for the bonding engine.
// ──────────────────────────────────────────────────────────────────────────────

// AdaptiveController wraps a StateMachine and Scheduler to provide a
// unified evaluation interface. It:
//   - Feeds live RTT/loss into the state machine for path health
//   - Adjusts scheduler mode based on overall path stability
//   - Computes cwnd updates for the stripe scheduler
//   - Emits diagnostic events for telemetry
type AdaptiveController struct {
	mu sync.RWMutex

	stateMachine *StateMachine
	scheduler    *Scheduler
	paths        map[string]*PathMetrics
	thresholds   Thresholds

	// Stability tracking for auto-mode promotion
	stableWindows int     // consecutive windows where RTT variance was low
	lastMedianRTT float64 // for variance computation
	stablePromote int     // windows needed to promote dup→stripe globally

	// Diagnostics
	totalEvals  uint64
	totalSwaps  uint64
	lastEvalAt  time.Time
}

// ControllerEvent describes a decision made by the adaptive controller.
type ControllerEvent struct {
	Type      string  `json:"type"`       // "path_transition", "mode_change", "cwnd_adjust"
	PathID    string  `json:"path_id"`
	Detail    string  `json:"detail"`
	OldValue  string  `json:"old_value"`
	NewValue  string  `json:"new_value"`
	Timestamp time.Time `json:"timestamp"`
}

// NewAdaptiveController creates an adaptive controller wrapping the given
// state machine, scheduler, and path metrics.
func NewAdaptiveController(
	sm *StateMachine,
	sched *Scheduler,
	paths map[string]*PathMetrics,
	thresholds Thresholds,
) *AdaptiveController {
	return &AdaptiveController{
		stateMachine:  sm,
		scheduler:     sched,
		paths:         paths,
		thresholds:    thresholds,
		stablePromote: 5, // 5 consecutive stable windows to promote
	}
}

// Evaluate runs one full evaluation cycle:
//  1. Compute aggregate stats (median RTT, best RTT, loss)
//  2. Run state machine evaluation for path transitions
//  3. Assess stability for scheduler mode promotion
//  4. Update cwnd for each path
//
// Returns events for telemetry logging.
func (ac *AdaptiveController) Evaluate() ([]ControllerEvent, []TransitionEvent) {
	ac.mu.Lock()
	defer ac.mu.Unlock()

	ac.totalEvals++
	ac.lastEvalAt = time.Now()

	var events []ControllerEvent

	// 1. Compute aggregate stats
	var srtts []float64
	var totalLoss float64
	var pathCount int

	for _, pm := range ac.paths {
		snap := pm.Snapshot()
		if snap.Alive && snap.SRTT > 0 {
			srtts = append(srtts, snap.SRTT)
			totalLoss += snap.LossPct
			pathCount++
		}
	}

	if pathCount == 0 {
		return events, nil
	}

	medianRTT := ac.scheduler.MedianSRTT()
	bestRTT := ac.scheduler.BestSRTT()
	avgLoss := totalLoss / float64(pathCount)

	// 2. Run state machine evaluation
	transitions := ac.stateMachine.Evaluate(medianRTT, bestRTT)
	for _, t := range transitions {
		events = append(events, ControllerEvent{
			Type:      "path_transition",
			PathID:    t.PathID,
			Detail:    t.Reason,
			OldValue:  t.OldState.String(),
			NewValue:  t.NewState.String(),
			Timestamp: ac.lastEvalAt,
		})
	}

	// 3. Stability assessment for scheduler mode
	if ac.lastMedianRTT > 0 && medianRTT > 0 {
		rttVariance := math.Abs(medianRTT-ac.lastMedianRTT) / ac.lastMedianRTT

		if rttVariance < 0.15 && avgLoss < 2.0 {
			// RTT variance < 15% and loss < 2% → stable
			ac.stableWindows++
		} else {
			ac.stableWindows = 0
		}

		// Promote from duplicate to stripe if sustained stability
		if ac.stableWindows >= ac.stablePromote {
			oldMode := ac.scheduler.GetMode()
			if oldMode == ModeDuplicate || oldMode == ModeAuto {
				// Don't force global stripe — just ensure Auto mode is set
				// so per-flow decisions can upgrade based on byte threshold
				if oldMode == ModeDuplicate {
					ac.scheduler.SetMode(ModeAuto)
					events = append(events, ControllerEvent{
						Type:      "mode_change",
						Detail:    "stability threshold reached, enabling auto-upgrade",
						OldValue:  oldMode.String(),
						NewValue:  ModeAuto.String(),
						Timestamp: ac.lastEvalAt,
					})
				}
			}
		}
	}
	ac.lastMedianRTT = medianRTT

	// 4. CWnd updates for each path
	for id, pm := range ac.paths {
		snap := pm.Snapshot()
		if !snap.Alive {
			continue
		}

		newCwnd := ac.computeCwnd(snap)
		if newCwnd != snap.CWnd {
			pm.SetCWnd(newCwnd)
			events = append(events, ControllerEvent{
				Type:      "cwnd_adjust",
				PathID:    id,
				Detail:    "adaptive cwnd update",
				OldValue:  formatFloat(snap.CWnd),
				NewValue:  formatFloat(newCwnd),
				Timestamp: ac.lastEvalAt,
			})
		}
	}

	return events, transitions
}

// computeCwnd calculates the congestion window for a path using
// an AIMD (Additive Increase / Multiplicative Decrease) scheme.
func (ac *AdaptiveController) computeCwnd(snap MetricsSnapshot) float64 {
	cwnd := snap.CWnd
	if cwnd <= 0 {
		cwnd = 10 // initial window
	}

	// Loss-based AIMD
	if snap.LossPct > ac.thresholds.LossDemotePct {
		// Multiplicative decrease (halve on high loss)
		cwnd = cwnd * 0.5
		if cwnd < 2 {
			cwnd = 2
		}
	} else if snap.LossPct < 1.0 {
		// Additive increase (grow by 1 when loss is low)
		cwnd += 1
		maxCwnd := float64(128)
		if cwnd > maxCwnd {
			cwnd = maxCwnd
		}
	}

	// RTT-based scaling: penalize paths with high RTT variance
	if snap.SRTT > 0 && snap.MinRTT > 0 {
		rttRatio := snap.SRTT / snap.MinRTT
		if rttRatio > 3.0 {
			// Path is congested (SRTT >> MinRTT) — reduce cwnd
			cwnd = cwnd * 0.8
			if cwnd < 2 {
				cwnd = 2
			}
		}
	}

	return cwnd
}

// GetMode returns the current scheduler mode.
func (ac *AdaptiveController) GetMode() ScheduleMode {
	return ac.scheduler.GetMode()
}

// Stats returns diagnostic statistics.
type ControllerStats struct {
	TotalEvals    uint64    `json:"total_evals"`
	TotalSwaps    uint64    `json:"total_swaps"`
	StableWindows int       `json:"stable_windows"`
	LastEvalAt    time.Time `json:"last_eval_at"`
	SchedulerMode string   `json:"scheduler_mode"`
}

func (ac *AdaptiveController) Stats() ControllerStats {
	ac.mu.RLock()
	defer ac.mu.RUnlock()

	return ControllerStats{
		TotalEvals:    ac.totalEvals,
		TotalSwaps:    ac.totalSwaps,
		StableWindows: ac.stableWindows,
		LastEvalAt:    ac.lastEvalAt,
		SchedulerMode: ac.scheduler.GetMode().String(),
	}
}

func formatFloat(f float64) string {
	if f == 0 {
		return "0"
	}
	// Simple formatting without importing strconv
	whole := int(f)
	frac := int((f - float64(whole)) * 100)
	if frac == 0 {
		return itoa(whole)
	}
	if frac < 0 {
		frac = -frac
	}
	result := itoa(whole) + "."
	if frac < 10 {
		result += "0"
	}
	result += itoa(frac)
	return result
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	buf := make([]byte, 0, 10)
	for n > 0 {
		buf = append(buf, byte('0'+n%10))
		n /= 10
	}
	if neg {
		buf = append(buf, '-')
	}
	for i, j := 0, len(buf)-1; i < j; i, j = i+1, j-1 {
		buf[i], buf[j] = buf[j], buf[i]
	}
	return string(buf)
}
