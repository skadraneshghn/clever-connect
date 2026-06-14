package control

import (
	"sync"
	"time"
)

// ──────────────────────────────────────────────────────────────────────────────
// Path State Machine — governs promotion, demotion, quarantine, and replacement
// ──────────────────────────────────────────────────────────────────────────────

// PathState represents the lifecycle state of an artery in the bonding pool.
type PathState int

const (
	// PathActive — fully participating in traffic dispatch (stripe or dup).
	PathActive PathState = iota

	// PathShadow — still connected but receives only PING keepalives + empty-payload
	// frames for RTT/loss measurement. No heavy traffic dispatched.
	PathShadow

	// PathProbation — was demoted from Active, must prove stability for N eval
	// windows before being re-promoted. Receives warm-probe traffic.
	PathProbation

	// PathDead — connection failed or consistently exceeded error budget.
	// Will be replaced by a fresh node from the scanner pool.
	PathDead

	// PathQuarantined — exceeded error budget K times. Held in cooldown before
	// any re-attempt. Requires cooldown expiry before moving to Dead/replaced.
	PathQuarantined
)

// String returns a human-readable name for the path state.
func (s PathState) String() string {
	switch s {
	case PathActive:
		return "active"
	case PathShadow:
		return "shadow"
	case PathProbation:
		return "probation"
	case PathDead:
		return "dead"
	case PathQuarantined:
		return "quarantined"
	default:
		return "unknown"
	}
}

// ParsePathState converts a string to PathState.
func ParsePathState(s string) PathState {
	switch s {
	case "active":
		return PathActive
	case "shadow":
		return PathShadow
	case "probation":
		return PathProbation
	case "dead":
		return PathDead
	case "quarantined":
		return PathQuarantined
	default:
		return PathActive
	}
}

// Thresholds controls the state machine transition parameters.
// These are tunable from the BondingEngineConfig database row.
type Thresholds struct {
	DemoteRTTx    float64       // srtt > DemoteRTTx × median → demote signal
	PromoteRTTx   float64       // srtt within PromoteRTTx × best → promote signal
	LossDemotePct float64       // loss% threshold for demotion signal
	CooldownDur   time.Duration // minimum time in quarantine before re-evaluation
	EvalWindow    time.Duration // evaluation window duration
	ConfirmWindows int          // number of consecutive windows to confirm a transition
	ErrorBudget   int           // K failures within a window → quarantine
}

// DefaultThresholds returns production-safe default thresholds.
func DefaultThresholds() Thresholds {
	return Thresholds{
		DemoteRTTx:     1.5,
		PromoteRTTx:    1.2,
		LossDemotePct:  5.0,
		CooldownDur:    30 * time.Second,
		EvalWindow:     5 * time.Second,
		ConfirmWindows: 3,
		ErrorBudget:    5,
	}
}

// PathEntry represents a single path managed by the state machine.
type PathEntry struct {
	mu sync.Mutex

	PathID  string
	State   PathState
	Metrics *PathMetrics

	// Transition tracking
	demoteSignals  int       // consecutive windows with demotion signal
	promoteSignals int       // consecutive windows with promotion signal
	lastDemotedAt  time.Time // timestamp of last demotion (for cooldown)
	lastPromotedAt time.Time // timestamp of last promotion
	errorCount     int       // current error budget counter
	quarantineAt   time.Time // when quarantine started
}

// NewPathEntry creates a new state machine entry for a path.
func NewPathEntry(pathID string, metrics *PathMetrics) *PathEntry {
	return &PathEntry{
		PathID:  pathID,
		State:   PathActive,
		Metrics: metrics,
	}
}

// StateMachine manages all path entries and runs evaluations.
type StateMachine struct {
	mu sync.RWMutex

	entries    map[string]*PathEntry
	thresholds Thresholds
}

// NewStateMachine creates a state machine with the given thresholds.
func NewStateMachine(thresholds Thresholds) *StateMachine {
	return &StateMachine{
		entries:    make(map[string]*PathEntry),
		thresholds: thresholds,
	}
}

// AddPath registers a path with the state machine.
func (sm *StateMachine) AddPath(entry *PathEntry) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.entries[entry.PathID] = entry
}

// RemovePath removes a path from the state machine.
func (sm *StateMachine) RemovePath(pathID string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	delete(sm.entries, pathID)
}

// GetEntry returns a path entry by ID.
func (sm *StateMachine) GetEntry(pathID string) *PathEntry {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.entries[pathID]
}

// TransitionEvent represents a state change for external handling.
type TransitionEvent struct {
	PathID   string
	OldState PathState
	NewState PathState
	Reason   string
}

// Evaluate runs one evaluation cycle across all paths.
// It returns a list of state transitions that occurred.
// This should be called once per EvalWindow interval.
func (sm *StateMachine) Evaluate(medianSRTT, bestSRTT float64) []TransitionEvent {
	sm.mu.RLock()
	entries := make([]*PathEntry, 0, len(sm.entries))
	for _, e := range sm.entries {
		entries = append(entries, e)
	}
	sm.mu.RUnlock()

	var events []TransitionEvent

	for _, entry := range entries {
		entry.mu.Lock()

		snap := entry.Metrics.Snapshot()
		oldState := entry.State

		switch entry.State {
		case PathActive:
			events = append(events, sm.evaluateActive(entry, snap, medianSRTT)...)

		case PathShadow:
			events = append(events, sm.evaluateShadow(entry, snap, bestSRTT)...)

		case PathProbation:
			events = append(events, sm.evaluateProbation(entry, snap, bestSRTT)...)

		case PathQuarantined:
			events = append(events, sm.evaluateQuarantined(entry)...)

		case PathDead:
			// Dead paths need external replacement; no automatic transitions
		}

		if entry.State != oldState {
			// Reset signal counters on any transition
			entry.demoteSignals = 0
			entry.promoteSignals = 0
		}

		entry.mu.Unlock()
	}

	return events
}

// evaluateActive checks if an active path should be demoted.
// Must be called with entry.mu held.
func (sm *StateMachine) evaluateActive(entry *PathEntry, snap MetricsSnapshot, medianSRTT float64) []TransitionEvent {
	// Check demotion conditions
	shouldDemote := false
	reason := ""

	if medianSRTT > 0 && snap.SRTT > sm.thresholds.DemoteRTTx*medianSRTT {
		shouldDemote = true
		reason = "srtt exceeds demote threshold"
	}

	if snap.LossPct > sm.thresholds.LossDemotePct {
		shouldDemote = true
		reason = "loss exceeds demote threshold"
	}

	if !snap.Alive {
		// Immediate demotion on liveness failure
		entry.State = PathDead
		return []TransitionEvent{{
			PathID: entry.PathID, OldState: PathActive, NewState: PathDead,
			Reason: "liveness failure",
		}}
	}

	if shouldDemote {
		entry.demoteSignals++
		if entry.demoteSignals >= sm.thresholds.ConfirmWindows {
			entry.State = PathShadow
			entry.lastDemotedAt = time.Now()
			return []TransitionEvent{{
				PathID: entry.PathID, OldState: PathActive, NewState: PathShadow,
				Reason: reason,
			}}
		}
	} else {
		entry.demoteSignals = 0
	}

	// Check error budget
	if entry.errorCount >= sm.thresholds.ErrorBudget {
		entry.State = PathQuarantined
		entry.quarantineAt = time.Now()
		return []TransitionEvent{{
			PathID: entry.PathID, OldState: PathActive, NewState: PathQuarantined,
			Reason: "error budget exhausted",
		}}
	}

	return nil
}

// evaluateShadow checks if a shadow path should be promoted back or killed.
// Must be called with entry.mu held.
func (sm *StateMachine) evaluateShadow(entry *PathEntry, snap MetricsSnapshot, bestSRTT float64) []TransitionEvent {
	if !snap.Alive {
		entry.State = PathDead
		return []TransitionEvent{{
			PathID: entry.PathID, OldState: PathShadow, NewState: PathDead,
			Reason: "liveness failure in shadow",
		}}
	}

	// Check promotion: must be within PromoteRTTx × bestSRTT AND loss below threshold
	// with the cooldown enforced
	if time.Since(entry.lastDemotedAt) < sm.thresholds.CooldownDur {
		return nil // still in cooldown
	}

	shouldPromote := false
	if bestSRTT > 0 && snap.SRTT > 0 && snap.SRTT <= sm.thresholds.PromoteRTTx*bestSRTT {
		if snap.LossPct <= sm.thresholds.LossDemotePct/2 { // stricter for promotion
			shouldPromote = true
		}
	}

	if shouldPromote {
		entry.promoteSignals++
		if entry.promoteSignals >= sm.thresholds.ConfirmWindows {
			entry.State = PathProbation
			return []TransitionEvent{{
				PathID: entry.PathID, OldState: PathShadow, NewState: PathProbation,
				Reason: "metrics recovered, entering probation",
			}}
		}
	} else {
		entry.promoteSignals = 0
	}

	return nil
}

// evaluateProbation checks if a probation path can be fully promoted to active.
// Must be called with entry.mu held.
func (sm *StateMachine) evaluateProbation(entry *PathEntry, snap MetricsSnapshot, bestSRTT float64) []TransitionEvent {
	if !snap.Alive {
		entry.State = PathDead
		return []TransitionEvent{{
			PathID: entry.PathID, OldState: PathProbation, NewState: PathDead,
			Reason: "liveness failure in probation",
		}}
	}

	// Two-signal gate: RTT AND loss must be good for ConfirmWindows consecutive windows
	rttGood := bestSRTT > 0 && snap.SRTT > 0 && snap.SRTT <= sm.thresholds.PromoteRTTx*bestSRTT
	lossGood := snap.LossPct <= sm.thresholds.LossDemotePct/2

	if rttGood && lossGood {
		entry.promoteSignals++
		if entry.promoteSignals >= sm.thresholds.ConfirmWindows {
			entry.State = PathActive
			entry.lastPromotedAt = time.Now()
			entry.errorCount = 0
			return []TransitionEvent{{
				PathID: entry.PathID, OldState: PathProbation, NewState: PathActive,
				Reason: "probation passed, promoted to active",
			}}
		}
	} else {
		// Failed during probation — back to shadow
		entry.promoteSignals = 0
		entry.State = PathShadow
		entry.lastDemotedAt = time.Now()
		return []TransitionEvent{{
			PathID: entry.PathID, OldState: PathProbation, NewState: PathShadow,
			Reason: "failed probation check, back to shadow",
		}}
	}

	return nil
}

// evaluateQuarantined checks if a quarantined path's cooldown has expired.
// Must be called with entry.mu held.
func (sm *StateMachine) evaluateQuarantined(entry *PathEntry) []TransitionEvent {
	if time.Since(entry.quarantineAt) >= sm.thresholds.CooldownDur {
		entry.State = PathDead // ready for replacement by the pool manager
		entry.errorCount = 0
		return []TransitionEvent{{
			PathID: entry.PathID, OldState: PathQuarantined, NewState: PathDead,
			Reason: "quarantine cooldown expired, ready for replacement",
		}}
	}
	return nil
}

// RecordError increments the error counter for a path.
func (sm *StateMachine) RecordError(pathID string) {
	sm.mu.RLock()
	entry, ok := sm.entries[pathID]
	sm.mu.RUnlock()
	if !ok {
		return
	}

	entry.mu.Lock()
	defer entry.mu.Unlock()
	entry.errorCount++
}

// ResetErrors resets the error counter for a path (e.g., after successful swap).
func (sm *StateMachine) ResetErrors(pathID string) {
	sm.mu.RLock()
	entry, ok := sm.entries[pathID]
	sm.mu.RUnlock()
	if !ok {
		return
	}

	entry.mu.Lock()
	defer entry.mu.Unlock()
	entry.errorCount = 0
}

// ActivePaths returns the path IDs of all currently active paths.
func (sm *StateMachine) ActivePaths() []string {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	var ids []string
	for _, e := range sm.entries {
		e.mu.Lock()
		if e.State == PathActive {
			ids = append(ids, e.PathID)
		}
		e.mu.Unlock()
	}
	return ids
}

// DeadPaths returns the path IDs of all dead paths (ready for replacement).
func (sm *StateMachine) DeadPaths() []string {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	var ids []string
	for _, e := range sm.entries {
		e.mu.Lock()
		if e.State == PathDead {
			ids = append(ids, e.PathID)
		}
		e.mu.Unlock()
	}
	return ids
}

// AllEntries returns a snapshot of all path entries and their states.
type PathSnapshot struct {
	PathID  string    `json:"path_id"`
	State   string    `json:"state"`
	Metrics MetricsSnapshot `json:"metrics"`
}

func (sm *StateMachine) AllEntries() []PathSnapshot {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	snapshots := make([]PathSnapshot, 0, len(sm.entries))
	for _, e := range sm.entries {
		e.mu.Lock()
		snapshots = append(snapshots, PathSnapshot{
			PathID:  e.PathID,
			State:   e.State.String(),
			Metrics: e.Metrics.Snapshot(),
		})
		e.mu.Unlock()
	}
	return snapshots
}
