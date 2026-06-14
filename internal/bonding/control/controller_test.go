package control

import (
	"testing"
)

func TestAdaptiveControllerBasic(t *testing.T) {
	// Create paths
	paths := map[string]*PathMetrics{
		"artery-0": NewPathMetrics("artery-0"),
		"artery-1": NewPathMetrics("artery-1"),
	}

	paths["artery-0"].UpdateRTT(50)
	paths["artery-1"].UpdateRTT(80)

	// Create state machine
	thresholds := Thresholds{
		DemoteRTTx:    1.5,
		PromoteRTTx:   1.2,
		LossDemotePct: 5.0,
		CooldownDur:   0,
		EvalWindow:    0,
		ConfirmWindows: 3,
		ErrorBudget:    5,
	}
	sm := NewStateMachine(thresholds)
	sm.AddPath(NewPathEntry("artery-0", paths["artery-0"]))
	sm.AddPath(NewPathEntry("artery-1", paths["artery-1"]))

	// Create scheduler
	pathSlice := []*PathMetrics{paths["artery-0"], paths["artery-1"]}
	sched := NewScheduler(ModeDuplicate, pathSlice)

	// Create controller
	ctrl := NewAdaptiveController(sm, sched, paths, thresholds)

	// Run evaluation
	events, transitions := ctrl.Evaluate()

	// Should have cwnd adjustments at minimum
	if ctrl.Stats().TotalEvals != 1 {
		t.Errorf("expected 1 eval, got %d", ctrl.Stats().TotalEvals)
	}

	// No transitions expected (both paths are healthy)
	if len(transitions) != 0 {
		t.Errorf("expected 0 transitions, got %d", len(transitions))
	}

	_ = events // cwnd events are expected
}

func TestAdaptiveControllerStabilityPromotion(t *testing.T) {
	paths := map[string]*PathMetrics{
		"artery-0": NewPathMetrics("artery-0"),
		"artery-1": NewPathMetrics("artery-1"),
	}

	// Seed with stable RTT
	for i := 0; i < 10; i++ {
		paths["artery-0"].UpdateRTT(50)
		paths["artery-1"].UpdateRTT(80)
	}

	thresholds := Thresholds{
		DemoteRTTx:     1.5,
		PromoteRTTx:    1.2,
		LossDemotePct:  5.0,
		CooldownDur:    0,
		EvalWindow:     0,
		ConfirmWindows: 3,
		ErrorBudget:    5,
	}
	sm := NewStateMachine(thresholds)
	sm.AddPath(NewPathEntry("artery-0", paths["artery-0"]))
	sm.AddPath(NewPathEntry("artery-1", paths["artery-1"]))

	pathSlice := []*PathMetrics{paths["artery-0"], paths["artery-1"]}
	sched := NewScheduler(ModeDuplicate, pathSlice)
	ctrl := NewAdaptiveController(sm, sched, paths, thresholds)

	// Run enough evaluations to trigger stability promotion
	var modeChanged bool
	for i := 0; i < 10; i++ {
		events, _ := ctrl.Evaluate()
		for _, e := range events {
			if e.Type == "mode_change" {
				modeChanged = true
			}
		}
	}

	// After sustained stability, mode should have been promoted from dup to auto
	if !modeChanged {
		t.Error("expected mode change to auto after stability, but didn't happen")
	}

	if ctrl.GetMode() != ModeAuto {
		t.Errorf("expected ModeAuto, got %s", ctrl.GetMode().String())
	}
}

func TestAdaptiveControllerCWndAIMD(t *testing.T) {
	paths := map[string]*PathMetrics{
		"artery-0": NewPathMetrics("artery-0"),
	}

	paths["artery-0"].UpdateRTT(50)

	// Simulate heavy loss to trigger AIMD decrease
	for i := 0; i < 50; i++ {
		paths["artery-0"].RecordSend()
	}
	// Record many losses so EWMA loss > 5%
	for i := 0; i < 30; i++ {
		paths["artery-0"].RecordLoss()
	}

	thresholds := Thresholds{
		DemoteRTTx:     1.5,
		PromoteRTTx:    1.2,
		LossDemotePct:  5.0,
		CooldownDur:    0,
		EvalWindow:     0,
		ConfirmWindows: 3,
		ErrorBudget:    5,
	}
	sm := NewStateMachine(thresholds)
	sm.AddPath(NewPathEntry("artery-0", paths["artery-0"]))

	pathSlice := []*PathMetrics{paths["artery-0"]}
	sched := NewScheduler(ModeStripe, pathSlice)
	ctrl := NewAdaptiveController(sm, sched, paths, thresholds)

	initialCwnd := paths["artery-0"].Snapshot().CWnd

	// Run evaluation — should trigger AIMD decrease due to high loss
	ctrl.Evaluate()

	newCwnd := paths["artery-0"].Snapshot().CWnd

	if newCwnd >= initialCwnd {
		t.Errorf("expected cwnd decrease due to loss, initial=%f new=%f", initialCwnd, newCwnd)
	}
}

func TestAdaptiveControllerCWndIncrease(t *testing.T) {
	paths := map[string]*PathMetrics{
		"artery-0": NewPathMetrics("artery-0"),
	}

	paths["artery-0"].UpdateRTT(50)
	// Very low loss: many sends, zero losses
	for i := 0; i < 100; i++ {
		paths["artery-0"].RecordSend()
		paths["artery-0"].RecordAck(1024, 50)
	}

	thresholds := Thresholds{
		DemoteRTTx:     1.5,
		PromoteRTTx:    1.2,
		LossDemotePct:  5.0,
		CooldownDur:    0,
		EvalWindow:     0,
		ConfirmWindows: 3,
		ErrorBudget:    5,
	}
	sm := NewStateMachine(thresholds)
	sm.AddPath(NewPathEntry("artery-0", paths["artery-0"]))

	pathSlice := []*PathMetrics{paths["artery-0"]}
	sched := NewScheduler(ModeStripe, pathSlice)
	ctrl := NewAdaptiveController(sm, sched, paths, thresholds)

	initialCwnd := paths["artery-0"].Snapshot().CWnd

	ctrl.Evaluate()

	newCwnd := paths["artery-0"].Snapshot().CWnd

	// Should increase by 1 (additive increase)
	if newCwnd <= initialCwnd {
		t.Errorf("expected cwnd increase, initial=%f new=%f", initialCwnd, newCwnd)
	}
}
