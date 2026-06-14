package control

import (
	"testing"
	"time"
)

// ──────────────────────────────────────────────────────────────────────────────
// Metrics Tests
// ──────────────────────────────────────────────────────────────────────────────

func TestPathMetrics_UpdateRTT(t *testing.T) {
	m := NewPathMetrics("test-0")

	// First measurement initializes SRTT
	m.UpdateRTT(100.0)
	snap := m.Snapshot()
	if snap.SRTT != 100.0 {
		t.Errorf("first SRTT: got %.2f, want 100.0", snap.SRTT)
	}
	if snap.RTTVar != 50.0 {
		t.Errorf("first RTTVar: got %.2f, want 50.0", snap.RTTVar)
	}

	// Second measurement applies EWMA
	m.UpdateRTT(80.0)
	snap = m.Snapshot()
	// SRTT = 0.875*100 + 0.125*80 = 97.5
	if snap.SRTT < 97.0 || snap.SRTT > 98.0 {
		t.Errorf("second SRTT: got %.2f, want ~97.5", snap.SRTT)
	}
}

func TestPathMetrics_MinRTT(t *testing.T) {
	m := NewPathMetrics("test-0")

	m.UpdateRTT(100.0)
	m.UpdateRTT(50.0)
	m.UpdateRTT(200.0)

	snap := m.Snapshot()
	if snap.MinRTT != 50.0 {
		t.Errorf("MinRTT: got %.2f, want 50.0", snap.MinRTT)
	}
}

func TestPathMetrics_LossTracking(t *testing.T) {
	m := NewPathMetrics("test-0")

	for i := 0; i < 10; i++ {
		m.RecordSend()
	}
	for i := 0; i < 2; i++ {
		m.RecordLoss()
	}

	snap := m.Snapshot()
	if snap.LossPct <= 0 {
		t.Errorf("LossPct should be > 0 after losses, got %.2f", snap.LossPct)
	}
	if snap.TotalSent != 10 {
		t.Errorf("TotalSent: got %d, want 10", snap.TotalSent)
	}
	if snap.TotalLost != 2 {
		t.Errorf("TotalLost: got %d, want 2", snap.TotalLost)
	}
}

func TestPathMetrics_WinRate(t *testing.T) {
	m := NewPathMetrics("test-0")

	// 3 wins out of 5 races
	m.RecordAck(1000, 50)
	m.RecordAck(1000, 50)
	m.RecordAck(1000, 50)
	m.RecordRaceLoss()
	m.RecordRaceLoss()

	snap := m.Snapshot()
	// 3 wins / 5 races = 60%
	if snap.WinRate < 59.0 || snap.WinRate > 61.0 {
		t.Errorf("WinRate: got %.2f, want ~60.0", snap.WinRate)
	}
}

func TestPathMetrics_HasCapacity(t *testing.T) {
	m := NewPathMetrics("test-0")

	if !m.HasCapacity() {
		t.Error("should have capacity initially")
	}

	// Fill up the congestion window
	for i := 0; i < initialCWnd; i++ {
		m.RecordSend()
	}

	if m.HasCapacity() {
		t.Error("should not have capacity when cwnd is full")
	}
}

func TestPathMetrics_RTO(t *testing.T) {
	m := NewPathMetrics("test-0")

	// Before any measurements
	rto := m.RTO()
	if rto != 1*time.Second {
		t.Errorf("initial RTO: got %v, want 1s", rto)
	}

	// After measurements
	m.UpdateRTT(100.0)
	rto = m.RTO()
	if rto < 200*time.Millisecond {
		t.Errorf("RTO after measurement: got %v, should be >= 200ms", rto)
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// Scheduler Tests
// ──────────────────────────────────────────────────────────────────────────────

func TestScheduler_DuplicateMode(t *testing.T) {
	p1 := NewPathMetrics("path-0")
	p2 := NewPathMetrics("path-1")
	p1.UpdateRTT(50)
	p2.UpdateRTT(100)

	sched := NewScheduler(ModeDuplicate, []*PathMetrics{p1, p2})
	result := sched.Schedule(1, 100)

	if len(result.Paths) != 2 {
		t.Errorf("duplicate mode should select all paths, got %d", len(result.Paths))
	}
	if result.Mode != ModeDuplicate {
		t.Errorf("mode: got %s, want duplicate", result.Mode)
	}
}

func TestScheduler_StripeMode(t *testing.T) {
	p1 := NewPathMetrics("path-0")
	p2 := NewPathMetrics("path-1")
	p1.UpdateRTT(50)
	p2.UpdateRTT(100)

	sched := NewScheduler(ModeStripe, []*PathMetrics{p1, p2})
	result := sched.Schedule(1, 100)

	if len(result.Paths) != 1 {
		t.Errorf("stripe mode should select exactly 1 path, got %d", len(result.Paths))
	}
	if result.Mode != ModeStripe {
		t.Errorf("mode: got %s, want stripe", result.Mode)
	}
	// Should pick the path with lower RTT
	if result.Paths[0].PathID != "path-0" {
		t.Errorf("stripe should pick lower-RTT path, got %s", result.Paths[0].PathID)
	}
}

func TestScheduler_AutoMode_UpgradeToStripe(t *testing.T) {
	p1 := NewPathMetrics("path-0")
	p2 := NewPathMetrics("path-1")
	p1.UpdateRTT(50)
	p2.UpdateRTT(100)

	sched := NewScheduler(ModeAuto, []*PathMetrics{p1, p2})
	sched.autoThresh = AutoModeThresholds{
		ByteThreshold:    100, // low threshold for testing
		DurationThreshold: 1000,
	}

	// First call: small payload → duplicate
	result := sched.Schedule(1, 50)
	if result.Mode != ModeDuplicate {
		t.Errorf("first call mode: got %s, want duplicate", result.Mode)
	}

	// Second call: pushes bytes over threshold → stripe
	result = sched.Schedule(1, 60)
	if result.Mode != ModeStripe {
		t.Errorf("after threshold mode: got %s, want stripe", result.Mode)
	}

	// Third call: should remain stripe (one-way upgrade)
	result = sched.Schedule(1, 10)
	if result.Mode != ModeStripe {
		t.Errorf("subsequent call mode: got %s, want stripe", result.Mode)
	}
}

func TestScheduler_AutoMode_IndependentStreams(t *testing.T) {
	p1 := NewPathMetrics("path-0")
	p1.UpdateRTT(50)

	sched := NewScheduler(ModeAuto, []*PathMetrics{p1})
	sched.autoThresh = AutoModeThresholds{ByteThreshold: 100, DurationThreshold: 1000}

	// Stream 1: push over threshold
	sched.Schedule(1, 200)

	// Stream 2: still small → should be duplicate
	result := sched.Schedule(2, 10)
	if result.Mode != ModeDuplicate {
		t.Errorf("independent stream should start as duplicate, got %s", result.Mode)
	}
}

func TestScheduler_NoActivePaths(t *testing.T) {
	p1 := NewPathMetrics("path-0")
	p1.SetAlive(false)

	sched := NewScheduler(ModeStripe, []*PathMetrics{p1})
	result := sched.Schedule(1, 100)

	if len(result.Paths) != 0 {
		t.Errorf("should return no paths when none alive, got %d", len(result.Paths))
	}
}

func TestScheduler_MedianSRTT(t *testing.T) {
	p1 := NewPathMetrics("p0")
	p2 := NewPathMetrics("p1")
	p3 := NewPathMetrics("p2")
	p1.UpdateRTT(50)
	p2.UpdateRTT(100)
	p3.UpdateRTT(200)

	sched := NewScheduler(ModeAuto, []*PathMetrics{p1, p2, p3})
	median := sched.MedianSRTT()
	if median != 100.0 {
		t.Errorf("MedianSRTT: got %.2f, want 100.0", median)
	}
}

func TestScheduler_BestSRTT(t *testing.T) {
	p1 := NewPathMetrics("p0")
	p2 := NewPathMetrics("p1")
	p1.UpdateRTT(150)
	p2.UpdateRTT(75)

	sched := NewScheduler(ModeAuto, []*PathMetrics{p1, p2})
	best := sched.BestSRTT()
	if best != 75.0 {
		t.Errorf("BestSRTT: got %.2f, want 75.0", best)
	}
}

func TestScheduler_CleanupFlow(t *testing.T) {
	p1 := NewPathMetrics("p0")
	p1.UpdateRTT(50)
	sched := NewScheduler(ModeAuto, []*PathMetrics{p1})

	sched.Schedule(42, 100)
	sched.CleanupFlow(42)

	// After cleanup, flow should start fresh
	sched.autoThresh = AutoModeThresholds{ByteThreshold: 200, DurationThreshold: 1000}
	result := sched.Schedule(42, 10)
	if result.Mode != ModeDuplicate {
		t.Errorf("after cleanup, flow should restart as duplicate, got %s", result.Mode)
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// State Machine Tests
// ──────────────────────────────────────────────────────────────────────────────

func TestStateMachine_ActiveToDemote(t *testing.T) {
	th := DefaultThresholds()
	th.ConfirmWindows = 2 // lower for testing

	sm := NewStateMachine(th)

	m := NewPathMetrics("artery-0")
	// Set high SRTT
	m.UpdateRTT(300)

	entry := NewPathEntry("artery-0", m)
	sm.AddPath(entry)

	// medianSRTT=100 → 300 > 1.5*100=150 → demotion signal
	events := sm.Evaluate(100, 50)
	if len(events) != 0 {
		t.Errorf("first eval should not transition (need %d windows), got %d events", th.ConfirmWindows, len(events))
	}

	// Second eval confirms
	events = sm.Evaluate(100, 50)
	if len(events) != 1 {
		t.Fatalf("expected 1 transition event, got %d", len(events))
	}
	if events[0].NewState != PathShadow {
		t.Errorf("expected transition to shadow, got %s", events[0].NewState)
	}
}

func TestStateMachine_NoDemoteWhenHealthy(t *testing.T) {
	th := DefaultThresholds()
	sm := NewStateMachine(th)

	m := NewPathMetrics("artery-0")
	m.UpdateRTT(90) // within demote threshold

	entry := NewPathEntry("artery-0", m)
	sm.AddPath(entry)

	// Run several evaluations — should never demote
	for i := 0; i < 10; i++ {
		events := sm.Evaluate(100, 80)
		if len(events) > 0 {
			t.Fatalf("healthy path should not transition, got event at eval %d: %+v", i, events[0])
		}
	}
}

func TestStateMachine_ShadowToPromote(t *testing.T) {
	th := DefaultThresholds()
	th.ConfirmWindows = 2
	th.CooldownDur = 0 // disable cooldown for testing

	sm := NewStateMachine(th)

	m := NewPathMetrics("artery-0")
	m.UpdateRTT(60) // good metrics

	entry := NewPathEntry("artery-0", m)
	entry.State = PathShadow
	entry.lastDemotedAt = time.Now().Add(-1 * time.Minute) // cooldown expired
	sm.AddPath(entry)

	// bestSRTT=50 → 60 <= 1.2*50=60 → promotion signal
	events := sm.Evaluate(100, 50)
	if len(events) != 0 {
		t.Errorf("first eval should not promote yet, got %d events", len(events))
	}

	events = sm.Evaluate(100, 50)
	if len(events) != 1 {
		t.Fatalf("expected 1 promotion event, got %d", len(events))
	}
	if events[0].NewState != PathProbation {
		t.Errorf("expected transition to probation, got %s", events[0].NewState)
	}
}

func TestStateMachine_ProbationToActive(t *testing.T) {
	th := DefaultThresholds()
	th.ConfirmWindows = 2

	sm := NewStateMachine(th)

	m := NewPathMetrics("artery-0")
	m.UpdateRTT(55) // good metrics

	entry := NewPathEntry("artery-0", m)
	entry.State = PathProbation
	sm.AddPath(entry)

	// bestSRTT=50 → 55 <= 1.2*50=60 AND loss=0 <= 2.5% → promotion signal
	events := sm.Evaluate(100, 50)
	if len(events) != 0 {
		t.Errorf("first eval should not promote, got %d events", len(events))
	}

	events = sm.Evaluate(100, 50)
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].NewState != PathActive {
		t.Errorf("expected promotion to active, got %s", events[0].NewState)
	}
}

func TestStateMachine_ProbationFail(t *testing.T) {
	th := DefaultThresholds()
	th.ConfirmWindows = 3

	sm := NewStateMachine(th)

	m := NewPathMetrics("artery-0")
	m.UpdateRTT(55)

	entry := NewPathEntry("artery-0", m)
	entry.State = PathProbation
	sm.AddPath(entry)

	// First eval: good → signal
	sm.Evaluate(100, 50)

	// Degrade metrics
	m.UpdateRTT(200) // now 200 > 1.2*50=60 → bad

	// Second eval: bad → reset signals, back to shadow
	events := sm.Evaluate(100, 50)
	if len(events) != 1 {
		t.Fatalf("expected 1 event on probation fail, got %d", len(events))
	}
	if events[0].NewState != PathShadow {
		t.Errorf("expected demotion to shadow, got %s", events[0].NewState)
	}
}

func TestStateMachine_ErrorBudgetQuarantine(t *testing.T) {
	th := DefaultThresholds()
	th.ErrorBudget = 3

	sm := NewStateMachine(th)

	m := NewPathMetrics("artery-0")
	m.UpdateRTT(50)

	entry := NewPathEntry("artery-0", m)
	sm.AddPath(entry)

	// Record errors
	sm.RecordError("artery-0")
	sm.RecordError("artery-0")
	sm.RecordError("artery-0")

	// Evaluate should quarantine
	events := sm.Evaluate(100, 50)
	if len(events) != 1 {
		t.Fatalf("expected 1 quarantine event, got %d", len(events))
	}
	if events[0].NewState != PathQuarantined {
		t.Errorf("expected quarantine, got %s", events[0].NewState)
	}
}

func TestStateMachine_QuarantineCooldown(t *testing.T) {
	th := DefaultThresholds()
	th.CooldownDur = 10 * time.Millisecond // fast for testing

	sm := NewStateMachine(th)

	m := NewPathMetrics("artery-0")
	m.UpdateRTT(50)

	entry := NewPathEntry("artery-0", m)
	entry.State = PathQuarantined
	entry.quarantineAt = time.Now()
	sm.AddPath(entry)

	// Before cooldown
	events := sm.Evaluate(100, 50)
	if len(events) != 0 {
		t.Errorf("should not transition before cooldown, got %d events", len(events))
	}

	// Wait for cooldown
	time.Sleep(15 * time.Millisecond)
	events = sm.Evaluate(100, 50)
	if len(events) != 1 {
		t.Fatalf("expected 1 event after cooldown, got %d", len(events))
	}
	if events[0].NewState != PathDead {
		t.Errorf("expected transition to dead, got %s", events[0].NewState)
	}
}

func TestStateMachine_LivenessFailure(t *testing.T) {
	th := DefaultThresholds()
	sm := NewStateMachine(th)

	m := NewPathMetrics("artery-0")
	m.SetAlive(false)

	entry := NewPathEntry("artery-0", m)
	sm.AddPath(entry)

	events := sm.Evaluate(100, 50)
	if len(events) != 1 {
		t.Fatalf("expected 1 event on liveness failure, got %d", len(events))
	}
	if events[0].NewState != PathDead {
		t.Errorf("expected immediate death on liveness failure, got %s", events[0].NewState)
	}
}

func TestStateMachine_NoOscillation(t *testing.T) {
	// Verify that asymmetric thresholds (demote=1.5× promote=1.2×) prevent flapping
	th := DefaultThresholds()
	th.ConfirmWindows = 2
	th.CooldownDur = 100 * time.Millisecond

	sm := NewStateMachine(th)

	m := NewPathMetrics("artery-0")
	entry := NewPathEntry("artery-0", m)
	sm.AddPath(entry)

	// Simulate jittery RTT between 120-140ms with median=100
	// 140 < 1.5*100=150 (no demote) but 140 > 1.2*100=120 (no promote if shadow)
	// This should NOT cause any transitions
	for i := 0; i < 20; i++ {
		rtt := 120.0 + float64(i%3)*10 // 120, 130, 140, ...
		m.UpdateRTT(rtt)
		events := sm.Evaluate(100, 80)
		if len(events) > 0 {
			t.Fatalf("jittery-but-stable path should not oscillate, got event at eval %d: %+v", i, events[0])
		}
	}
}

func TestStateMachine_ActivePaths(t *testing.T) {
	sm := NewStateMachine(DefaultThresholds())

	m1 := NewPathMetrics("a0")
	m2 := NewPathMetrics("a1")
	m3 := NewPathMetrics("a2")

	e1 := NewPathEntry("a0", m1)
	e2 := NewPathEntry("a1", m2)
	e3 := NewPathEntry("a2", m3)
	e3.State = PathShadow

	sm.AddPath(e1)
	sm.AddPath(e2)
	sm.AddPath(e3)

	active := sm.ActivePaths()
	if len(active) != 2 {
		t.Errorf("expected 2 active paths, got %d", len(active))
	}
}

func TestStateMachine_DeadPaths(t *testing.T) {
	sm := NewStateMachine(DefaultThresholds())

	m := NewPathMetrics("a0")
	e := NewPathEntry("a0", m)
	e.State = PathDead
	sm.AddPath(e)

	dead := sm.DeadPaths()
	if len(dead) != 1 {
		t.Errorf("expected 1 dead path, got %d", len(dead))
	}
}
