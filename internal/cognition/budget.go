package cognition

import (
	"sync"
	"time"
)

// Budget ceiling defaults. Every value is a configuration fallback; callers
// override through BudgetConfig fields.
const (
	// DefaultMaxSteps caps the whole run at 500 ledger steps.
	DefaultMaxSteps = 500
	// DefaultMaxTokens caps cumulative token spend at 2,000,000.
	DefaultMaxTokens = 2_000_000
	// DefaultMaxWallClock caps wall-clock duration at 4 hours.
	DefaultMaxWallClock = 4 * time.Hour
)

// BudgetConfig configures BudgetGate. Zero fields fall back to the Default*
// constants above.
type BudgetConfig struct {
	// MaxSteps caps the number of Step() calls (ledger steps) before
	// HardStop engages.
	MaxSteps int
	// MaxTokens caps cumulative SpendTokens() before HardStop engages.
	MaxTokens int64
	// MaxWallClock caps elapsed time since NewBudgetGate before HardStop
	// engages. Zero falls back to DefaultMaxWallClock; a negative value
	// disables wall-clock enforcement entirely.
	MaxWallClock time.Duration
}

// BudgetGate is the Budget door: three independent hard ceilings (steps,
// tokens, wall clock). Any one reaching its limit means HardStop — physical
// termination, independent of model will. All accounting is thread-safe.
type BudgetGate struct {
	mu          sync.Mutex
	cfg         BudgetConfig
	start       time.Time
	tokensSpent int64
	stepsTaken  int
	stopped     bool
	stoppedBy   string
}

// NewBudgetGate builds a gate from cfg, applying defaults for zero fields.
// The wall-clock window opens at construction.
func NewBudgetGate(cfg BudgetConfig) *BudgetGate {
	if cfg.MaxSteps <= 0 {
		cfg.MaxSteps = DefaultMaxSteps
	}
	if cfg.MaxTokens <= 0 {
		cfg.MaxTokens = DefaultMaxTokens
	}
	if cfg.MaxWallClock == 0 {
		cfg.MaxWallClock = DefaultMaxWallClock
	}
	return &BudgetGate{cfg: cfg, start: time.Now()}
}

// Step records one completed ledger step and reports whether any hard
// ceiling is now reached.
func (b *BudgetGate) Step() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.stepsTaken++
	return b.hardStopLocked()
}

// SpendTokens records n tokens of cumulative spend and reports whether any
// hard ceiling is now reached. Non-positive spends are ignored for
// accounting but the latched stop state is still reported.
func (b *BudgetGate) SpendTokens(n int64) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if n > 0 {
		b.tokensSpent += n
	}
	return b.hardStopLocked()
}

// HardStop reports whether any of the three ceilings is reached. It is
// monotonic: once true it stays true even if the caller falls back under
// the limits.
func (b *BudgetGate) HardStop() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.hardStopLocked()
}

// hardStopLocked evaluates all three ceilings and latches the result.
// Caller holds b.mu.
func (b *BudgetGate) hardStopLocked() bool {
	if b.stopped {
		return true
	}
	switch {
	case b.stepsTaken >= b.cfg.MaxSteps:
		b.stopped, b.stoppedBy = true, "steps"
	case b.tokensSpent >= b.cfg.MaxTokens:
		b.stopped, b.stoppedBy = true, "tokens"
	case b.cfg.MaxWallClock > 0 && time.Since(b.start) >= b.cfg.MaxWallClock:
		b.stopped, b.stoppedBy = true, "wall_clock"
	}
	return b.stopped
}

// StepsUsed reports consumed steps.
func (b *BudgetGate) StepsUsed() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.stepsTaken
}

// StepsLeft reports remaining step allowance (0 once HardStop).
func (b *BudgetGate) StepsLeft() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.stopped {
		return 0
	}
	return b.cfg.MaxSteps - b.stepsTaken
}

// TokensSpent reports cumulative token spend.
func (b *BudgetGate) TokensSpent() int64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.tokensSpent
}

// TokensLeft reports remaining token allowance (0 once HardStop).
func (b *BudgetGate) TokensLeft() int64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.stopped {
		return 0
	}
	return b.cfg.MaxTokens - b.tokensSpent
}

// Elapsed reports wall-clock time since the gate was constructed.
func (b *BudgetGate) Elapsed() time.Duration {
	b.mu.Lock()
	defer b.mu.Unlock()
	return time.Since(b.start)
}

// WallClockLeft reports remaining wall-clock allowance (0 once HardStop).
func (b *BudgetGate) WallClockLeft() time.Duration {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.stopped {
		return 0
	}
	return b.cfg.MaxWallClock - time.Since(b.start)
}

// StoppedBy names the ceiling that fired ("steps", "tokens" or
// "wall_clock"), or "" while no ceiling has fired.
func (b *BudgetGate) StoppedBy() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.stoppedBy
}
