package frontier

type BudgetState string

const (
	BudgetHealthy   BudgetState = "healthy"
	BudgetModerate  BudgetState = "moderate"
	BudgetCritical  BudgetState = "critical"
	BudgetExhausted BudgetState = "exhausted"
)

type BudgetSnapshot struct {
	Total     float64
	Remaining float64
	Percent   float64
}

type BudgetAllocator struct {
	total     float64
	remaining float64
	strategy  Strategy
}

func NewBudgetAllocator(total float64) *BudgetAllocator {
	return &BudgetAllocator{
		total:     total,
		remaining: total,
		strategy:  StrategyBreadth,
	}
}

func (a *BudgetAllocator) Snapshot() BudgetSnapshot {
	percent := 0.0
	if a.total > 0 {
		percent = (a.remaining / a.total) * 100.0
	}
	return BudgetSnapshot{
		Total:     a.total,
		Remaining: a.remaining,
		Percent:   percent,
	}
}

func (a *BudgetAllocator) State() BudgetState {
	snapshot := a.Snapshot()
	switch {
	case snapshot.Remaining <= 0:
		return BudgetExhausted
	case snapshot.Percent < 20.0:
		return BudgetCritical
	case snapshot.Percent < 50.0:
		return BudgetModerate
	default:
		return BudgetHealthy
	}
}

func (a *BudgetAllocator) Spend(cost float64) bool {
	if cost > a.remaining {
		return false
	}
	a.remaining -= cost
	return true
}

func (a *BudgetAllocator) RecommendedStrategy() Strategy {
	state := a.State()
	switch state {
	case BudgetCritical, BudgetExhausted:
		return StrategyDepth
	case BudgetModerate:
		return StrategyMixed
	default:
		return a.strategy
	}
}

func (a *BudgetAllocator) ShouldDowngrade() bool {
	return a.State() == BudgetCritical || a.State() == BudgetExhausted
}
