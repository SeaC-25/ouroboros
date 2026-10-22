package cognition

import (
	"errors"
	"testing"
)

// --- IGR window boundaries ---

func TestRecordStepStallsExactlyAtWindowFill(t *testing.T) {
	g := NewProgressGate(ProgressConfig{})
	for range DefaultWindowSteps - 1 {
		if got := g.RecordStep(0); got {
			t.Fatal("stalled before window filled")
		}
	}
	// 5th consecutive zero-gain step fills the window → stall.
	if !g.RecordStep(0) {
		t.Fatal("5th zero-gain step should trigger stall")
	}
	if !g.Stalled() {
		t.Fatal("Stalled() should report true after window filled")
	}
}

func TestMidWindowGainResetsStall(t *testing.T) {
	g := NewProgressGate(ProgressConfig{})
	for range DefaultWindowSteps - 1 {
		g.RecordStep(0)
	}
	// Gain on the would-be 5th step resets the window — no stall.
	if g.RecordStep(3) {
		t.Fatal("gain on last window slot must reset stall detection")
	}
	if g.Stalled() {
		t.Fatal("Stalled() must be false after mid-window gain")
	}
	// After the mid-window gain, exactly 5 fresh zero-gain steps are
	// needed to stall again: the first 4 must not stall.
	for range DefaultWindowSteps - 1 {
		if g.RecordStep(0) {
			t.Fatal("stalled before a full fresh zero-gain window accumulated")
		}
	}
	if !g.RecordStep(0) {
		t.Fatal("5th fresh zero-gain step should stall again")
	}
}

func TestGainAfterStallClearsCondition(t *testing.T) {
	g := NewProgressGate(ProgressConfig{})
	for range DefaultWindowSteps {
		g.RecordStep(0)
	}
	if !g.Stalled() {
		t.Fatal("precondition: stalled")
	}
	if g.RecordStep(1) {
		t.Fatal("gain step must clear stall, not report it")
	}
	if g.Stalled() {
		t.Fatal("gain must clear Stalled()")
	}
}

func TestWindowOneStallsImmediatelyOnZeroGain(t *testing.T) {
	g := NewProgressGate(ProgressConfig{WindowSteps: 1})
	if !g.RecordStep(0) {
		t.Fatal("window=1: first zero-gain step should stall")
	}
}

func TestOnStallFiresOncePerStallEpisode(t *testing.T) {
	fired := 0
	g := NewProgressGate(ProgressConfig{OnStall: func() { fired++ }})
	for range DefaultWindowSteps {
		g.RecordStep(0)
	}
	if fired != 1 {
		t.Fatalf("OnStall fired %d times, want 1", fired)
	}
	// Additional zero-gain steps while still stalled must not re-fire.
	g.RecordStep(0)
	g.RecordStep(0)
	if fired != 1 {
		t.Fatalf("OnStall re-fired while stalled: %d", fired)
	}
	// A gain step clears the stall; a fresh window of zero-gain re-fires.
	g.RecordStep(2)
	for range DefaultWindowSteps {
		g.RecordStep(0)
	}
	if fired != 2 {
		t.Fatalf("OnStall fired %d times after second stall, want 2", fired)
	}
}

// --- Completion adjudication ---

func TestShouldRejectCompletion(t *testing.T) {
	tests := []struct {
		name       string
		queueEmpty bool
		pivotsUsed int
		entityID   string
		want       bool
	}{
		{
			name:       "queue non-empty rejects regardless of pivot budget",
			queueEmpty: false,
			pivotsUsed: 2,
			entityID:   "e1",
			want:       true,
		},
		{
			name:       "queue empty but pivots under cap rejects",
			queueEmpty: true,
			pivotsUsed: 1,
			entityID:   "e1",
			want:       true,
		},
		{
			name:       "queue empty and pivots exhausted allows completion",
			queueEmpty: true,
			pivotsUsed: 2,
			entityID:   "e1",
			want:       false,
		},
		{
			name:       "queue empty no entity named allows completion",
			queueEmpty: true,
			pivotsUsed: 0,
			entityID:   "",
			want:       false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewProgressGate(ProgressConfig{})
			for range tt.pivotsUsed {
				if err := g.RecordPivot(tt.entityID); err != nil {
					t.Fatalf("RecordPivot setup: %v", err)
				}
			}
			if got := g.ShouldRejectCompletion(tt.queueEmpty, tt.entityID); got != tt.want {
				t.Fatalf("ShouldRejectCompletion(queueEmpty=%v, entity=%q) = %v, want %v",
					tt.queueEmpty, tt.entityID, got, tt.want)
			}
		})
	}
}

// --- Per-entity pivot accounting ---

func TestRecordPivotPerEntityCountAndCap(t *testing.T) {
	g := NewProgressGate(ProgressConfig{})
	if err := g.RecordPivot("a"); err != nil {
		t.Fatalf("first pivot: %v", err)
	}
	if err := g.RecordPivot("a"); err != nil {
		t.Fatalf("second pivot: %v", err)
	}
	if err := g.RecordPivot("a"); !errors.Is(err, ErrPivotBudgetExhausted) {
		t.Fatalf("third pivot = %v, want ErrPivotBudgetExhausted", err)
	}
	if got := g.PivotsUsed("a"); got != 2 {
		t.Fatalf("PivotsUsed(a) = %d, want 2 (refused pivot must not count)", got)
	}
	// Independent budgets per entity.
	if err := g.RecordPivot("b"); err != nil {
		t.Fatalf("pivot on second entity: %v", err)
	}
	if got := g.PivotsUsed("b"); got != 1 {
		t.Fatalf("PivotsUsed(b) = %d, want 1", got)
	}
	if got := g.PivotsUsed("c"); got != 0 {
		t.Fatalf("PivotsUsed(c) = %d, want 0", got)
	}
}
