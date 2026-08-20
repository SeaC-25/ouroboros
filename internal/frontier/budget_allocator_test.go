package frontier

import "testing"

func TestBudgetAllocator_InitialState(t *testing.T) {
	allocator := NewBudgetAllocator(1000.0)

	snapshot := allocator.Snapshot()
	if snapshot.Total != 1000.0 {
		t.Errorf("expected total 1000.0, got %v", snapshot.Total)
	}
	if snapshot.Remaining != 1000.0 {
		t.Errorf("expected remaining 1000.0, got %v", snapshot.Remaining)
	}
	if snapshot.Percent != 100.0 {
		t.Errorf("expected percent 100.0, got %v", snapshot.Percent)
	}
}

func TestBudgetAllocator_Spend(t *testing.T) {
	allocator := NewBudgetAllocator(1000.0)

	if !allocator.Spend(300.0) {
		t.Error("expected Spend to succeed")
	}

	snapshot := allocator.Snapshot()
	if snapshot.Remaining != 700.0 {
		t.Errorf("expected remaining 700.0, got %v", snapshot.Remaining)
	}
}

func TestBudgetAllocator_SpendExceedsRemaining(t *testing.T) {
	allocator := NewBudgetAllocator(100.0)

	if allocator.Spend(150.0) {
		t.Error("expected Spend to fail when cost exceeds remaining")
	}

	snapshot := allocator.Snapshot()
	if snapshot.Remaining != 100.0 {
		t.Errorf("expected remaining unchanged at 100.0, got %v", snapshot.Remaining)
	}
}

func TestBudgetAllocator_StateHealthy(t *testing.T) {
	allocator := NewBudgetAllocator(1000.0)

	if allocator.State() != BudgetHealthy {
		t.Errorf("expected state %v, got %v", BudgetHealthy, allocator.State())
	}
}

func TestBudgetAllocator_StateModerate(t *testing.T) {
	allocator := NewBudgetAllocator(1000.0)
	allocator.Spend(600.0)

	if allocator.State() != BudgetModerate {
		t.Errorf("expected state %v, got %v", BudgetModerate, allocator.State())
	}
}

func TestBudgetAllocator_StateCritical(t *testing.T) {
	allocator := NewBudgetAllocator(1000.0)
	allocator.Spend(850.0)

	if allocator.State() != BudgetCritical {
		t.Errorf("expected state %v, got %v", BudgetCritical, allocator.State())
	}
}

func TestBudgetAllocator_StateExhausted(t *testing.T) {
	allocator := NewBudgetAllocator(1000.0)
	allocator.Spend(1000.0)

	if allocator.State() != BudgetExhausted {
		t.Errorf("expected state %v, got %v", BudgetExhausted, allocator.State())
	}
}

func TestBudgetAllocator_RecommendedStrategy(t *testing.T) {
	allocator := NewBudgetAllocator(1000.0)

	if allocator.RecommendedStrategy() != StrategyBreadth {
		t.Errorf("expected default strategy %v", StrategyBreadth)
	}

	allocator.Spend(850.0)
	if allocator.RecommendedStrategy() != StrategyDepth {
		t.Errorf("expected strategy %v for critical budget", StrategyDepth)
	}
}

func TestBudgetAllocator_ShouldDowngrade(t *testing.T) {
	allocator := NewBudgetAllocator(1000.0)

	if allocator.ShouldDowngrade() {
		t.Error("expected ShouldDowngrade false for healthy budget")
	}

	allocator.Spend(850.0)
	if !allocator.ShouldDowngrade() {
		t.Error("expected ShouldDowngrade true for critical budget")
	}
}
