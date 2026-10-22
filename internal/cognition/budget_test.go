package cognition

import (
	"sync"
	"testing"
	"time"
)

// --- Per-ceiling triggers ---

func TestStepCeilingTriggers(t *testing.T) {
	g := NewBudgetGate(BudgetConfig{MaxSteps: 3})
	for range 2 {
		if g.Step() {
			t.Fatal("HardStop before step ceiling reached")
		}
	}
	if !g.Step() {
		t.Fatal("HardStop expected exactly at MaxSteps")
	}
	if g.StoppedBy() != "steps" {
		t.Fatalf("StoppedBy = %q, want steps", g.StoppedBy())
	}
	if g.StepsLeft() != 0 {
		t.Fatalf("StepsLeft after stop = %d, want 0", g.StepsLeft())
	}
}

func TestTokenCeilingTriggers(t *testing.T) {
	g := NewBudgetGate(BudgetConfig{MaxTokens: 1000})
	if g.SpendTokens(600) {
		t.Fatal("HardStop before token ceiling reached")
	}
	if g.TokensSpent() != 600 || g.TokensLeft() != 400 {
		t.Fatalf("accounting = %d spent / %d left, want 600/400", g.TokensSpent(), g.TokensLeft())
	}
	if !g.SpendTokens(400) {
		t.Fatal("HardStop expected exactly at MaxTokens")
	}
	if g.StoppedBy() != "tokens" {
		t.Fatalf("StoppedBy = %q, want tokens", g.StoppedBy())
	}
	if g.TokensLeft() != 0 {
		t.Fatalf("TokensLeft after stop = %d, want 0", g.TokensLeft())
	}
}

func TestWallClockCeilingTriggers(t *testing.T) {
	g := NewBudgetGate(BudgetConfig{MaxWallClock: 10 * time.Millisecond})
	time.Sleep(15 * time.Millisecond)
	if !g.HardStop() {
		t.Fatal("HardStop expected after wall clock elapsed")
	}
	if g.StoppedBy() != "wall_clock" {
		t.Fatalf("StoppedBy = %q, want wall_clock", g.StoppedBy())
	}
	if g.WallClockLeft() != 0 {
		t.Fatal("WallClockLeft after stop should be 0")
	}
}

func TestWallClockDisabledByNegativeConfig(t *testing.T) {
	g := NewBudgetGate(BudgetConfig{MaxWallClock: -1})
	time.Sleep(5 * time.Millisecond)
	if g.HardStop() {
		t.Fatal("negative MaxWallClock must disable wall-clock enforcement")
	}
}

// --- Accounting accuracy ---

func TestTokenAccountingAccumulates(t *testing.T) {
	g := NewBudgetGate(BudgetConfig{MaxTokens: 10_000})
	g.SpendTokens(100)
	g.SpendTokens(250)
	if got := g.TokensSpent(); got != 350 {
		t.Fatalf("TokensSpent = %d, want 350", got)
	}
	if got := g.TokensLeft(); got != 10_000-350 {
		t.Fatalf("TokensLeft = %d, want %d", got, 10_000-350)
	}
	if got := g.StepsUsed(); got != 0 {
		t.Fatalf("StepsUsed = %d, want 0", got)
	}
}

func TestStepAccounting(t *testing.T) {
	g := NewBudgetGate(BudgetConfig{MaxSteps: 10})
	for range 4 {
		g.Step()
	}
	if got := g.StepsUsed(); got != 4 {
		t.Fatalf("StepsUsed = %d, want 4", got)
	}
	if got := g.StepsLeft(); got != 6 {
		t.Fatalf("StepsLeft = %d, want 6", got)
	}
}

func TestNonPositiveSpendIgnored(t *testing.T) {
	g := NewBudgetGate(BudgetConfig{MaxTokens: 100})
	g.SpendTokens(0)
	g.SpendTokens(-5)
	if got := g.TokensSpent(); got != 0 {
		t.Fatalf("non-positive spends must not change ledger, got %d", got)
	}
}

func TestHardStopIsMonotonic(t *testing.T) {
	g := NewBudgetGate(BudgetConfig{MaxTokens: 100})
	g.SpendTokens(100)
	if !g.HardStop() {
		t.Fatal("precondition: stopped")
	}
	// There is no API to un-spend; a stop must stay latched.
	if !g.SpendTokens(1) {
		t.Fatal("stop must remain latched")
	}
	if g.StoppedBy() != "tokens" {
		t.Fatalf("StoppedBy = %q, want tokens (first cause wins)", g.StoppedBy())
	}
}

func TestDefaultsAppliedOnZeroConfig(t *testing.T) {
	g := NewBudgetGate(BudgetConfig{})
	if got := g.StepsLeft(); got != DefaultMaxSteps {
		t.Fatalf("StepsLeft with zero config = %d, want %d", got, DefaultMaxSteps)
	}
	if got := g.TokensLeft(); got != DefaultMaxTokens {
		t.Fatalf("TokensLeft with zero config = %d, want %d", got, DefaultMaxTokens)
	}
	if got := g.WallClockLeft(); got <= 0 || got > DefaultMaxWallClock {
		t.Fatalf("WallClockLeft with zero config = %v, want within (0, %v]", got, DefaultMaxWallClock)
	}
}

// --- Defaults constants sanity ---

func TestDefaultConstants(t *testing.T) {
	if DefaultMaxSteps != 500 {
		t.Fatalf("DefaultMaxSteps = %d, want 500", DefaultMaxSteps)
	}
	if DefaultMaxTokens != 2_000_000 {
		t.Fatalf("DefaultMaxTokens = %d, want 2000000", DefaultMaxTokens)
	}
	if DefaultMaxWallClock != 4*time.Hour {
		t.Fatalf("DefaultMaxWallClock = %v, want 4h", DefaultMaxWallClock)
	}
	if DefaultWindowSteps != 5 {
		t.Fatalf("DefaultWindowSteps = %d, want 5", DefaultWindowSteps)
	}
	if DefaultMaxPivotsPerEntity != 2 {
		t.Fatalf("DefaultMaxPivotsPerEntity = %d, want 2", DefaultMaxPivotsPerEntity)
	}
}

// --- Concurrency smoke ---

func TestBudgetGateConcurrentAccounting(t *testing.T) {
	g := NewBudgetGate(BudgetConfig{MaxTokens: 1_000_000})
	var wg sync.WaitGroup
	const workers, perWorker = 8, 100
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range perWorker {
				g.SpendTokens(10)
			}
		}()
	}
	wg.Wait()
	if got := g.TokensSpent(); got != workers*perWorker*10 {
		t.Fatalf("TokensSpent under concurrency = %d, want %d", got, workers*perWorker*10)
	}
}
