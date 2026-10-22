package cognition

import (
	"errors"
	"sync"
)

// Progress-gate defaults, overridable through ProgressConfig.
const (
	// DefaultWindowSteps is the IGR sliding window: stall detection looks
	// at the most recent 5 steps.
	DefaultWindowSteps = 5
	// DefaultMaxPivotsPerEntity caps forced turnarounds on one entity at 2.
	DefaultMaxPivotsPerEntity = 2
)

// ErrPivotBudgetExhausted is returned by RecordPivot once an entity has
// consumed its full pivot allowance.
var ErrPivotBudgetExhausted = errors.New("cognition: pivot budget exhausted for entity")

// ProgressConfig configures ProgressGate. Zero fields fall back to the
// Default* constants.
type ProgressConfig struct {
	// WindowSteps is the IGR sliding window length in steps.
	WindowSteps int
	// MaxPivotsPerEntity caps forced pivots per entity.
	MaxPivotsPerEntity int
	// OnStall fires each time the IGR window (exactly the trailing
	// WindowSteps steps) is all zero-gain. Pure signal hook for freezing
	// quadrants / reversing weights — the gate takes no further action
	// itself. May be nil.
	OnStall func()
}

// ProgressGate is the Progress door: IGR (information-gain rate) stall
// detection plus adjudication of model completion claims and forced-pivot
// budgeting. Pure policy layer — it signals, it never acts.
type ProgressGate struct {
	mu          sync.Mutex
	window      int
	maxPivots   int
	onStall     func()
	recent      []bool // true = step produced new components
	zeroGain    int    // trailing consecutive zero-gain steps
	stalled     bool
	pivotCounts map[string]int
}

// NewProgressGate builds a gate from cfg, applying defaults for zero
// fields.
func NewProgressGate(cfg ProgressConfig) *ProgressGate {
	if cfg.WindowSteps <= 0 {
		cfg.WindowSteps = DefaultWindowSteps
	}
	if cfg.MaxPivotsPerEntity <= 0 {
		cfg.MaxPivotsPerEntity = DefaultMaxPivotsPerEntity
	}
	return &ProgressGate{
		window:      cfg.WindowSteps,
		maxPivots:   cfg.MaxPivotsPerEntity,
		onStall:     cfg.OnStall,
		recent:      make([]bool, 0, cfg.WindowSteps),
		pivotCounts: make(map[string]int),
	}
}

// RecordStep records one step that produced newComponents new information
// components (0 = no gain) and reports the current stall state.
func (g *ProgressGate) RecordStep(newComponents int) bool {
	g.mu.Lock()
	if newComponents > 0 {
		g.recent = append(g.recent, true)
		g.zeroGain = 0
	} else {
		g.recent = append(g.recent, false)
		g.zeroGain++
	}
	if len(g.recent) > g.window {
		g.recent = g.recent[1:]
	}
	stallNow := len(g.recent) == g.window && g.zeroGain >= g.window
	fire := stallNow && !g.stalled
	g.stalled = stallNow
	g.mu.Unlock()

	if fire && g.onStall != nil {
		g.onStall()
	}
	return stallNow
}

// Stalled reports whether the gate is currently in IGR stall: the trailing
// window is full and carries no new components. Mid-window a single
// productive step resets the condition.
func (g *ProgressGate) Stalled() bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.stalled
}

// ShouldRejectCompletion adjudicates a model termination claim. The claim is
// rejected while work demonstrably remains: the frontier queue is non-empty,
// or the entity of the claim has pivot budget left. queueEmpty must be
// reported by the caller (frontier owner); entityID is the entity the claim
// concerns (empty means the claim names no entity — then only the queue
// decides).
func (g *ProgressGate) ShouldRejectCompletion(queueEmpty bool, entityID string) bool {
	if !queueEmpty {
		return true // work remains on the frontier — completion refused.
	}
	if entityID == "" {
		return false
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	// Pivot budget remaining means the run may still be forced to turn
	// around and look from another angle; do not accept completion yet.
	return g.pivotCounts[entityID] < g.maxPivots
}

// RecordPivot consumes one unit of the per-entity pivot budget. Once
// MaxPivotsPerEntity pivots are recorded for an entity, further calls return
// ErrPivotBudgetExhausted and consume nothing.
func (g *ProgressGate) RecordPivot(entityID string) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.pivotCounts[entityID] >= g.maxPivots {
		return ErrPivotBudgetExhausted
	}
	g.pivotCounts[entityID]++
	return nil
}

// PivotsUsed reports how many pivots entityID has consumed.
func (g *ProgressGate) PivotsUsed(entityID string) int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.pivotCounts[entityID]
}
