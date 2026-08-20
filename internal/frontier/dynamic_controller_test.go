package frontier

import "testing"

func TestDynamicController_Strategy(t *testing.T) {
	matrix := NewCoverageMatrix()
	queue := NewQueue()
	controller := NewDynamicController(matrix, queue)

	if controller.Strategy() != StrategyBreadth {
		t.Errorf("expected initial strategy %v, got %v", StrategyBreadth, controller.Strategy())
	}
}

func TestDynamicController_RecomputeBreadth(t *testing.T) {
	matrix := NewCoverageMatrix()
	matrix.Update(DomainWebAPI, DimensionRoutes, CoverageCounters{Discovered: 100, Tested: 10})

	queue := NewQueue()
	controller := NewDynamicController(matrix, queue)

	strategy := controller.Recompute()
	if strategy != StrategyBreadth {
		t.Errorf("expected %v for low coverage, got %v", StrategyBreadth, strategy)
	}
}

func TestDynamicController_RecomputeDepth(t *testing.T) {
	matrix := NewCoverageMatrix()
	matrix.Update(DomainWebAPI, DimensionRoutes, CoverageCounters{Discovered: 100, Tested: 80})

	queue := NewQueue()
	controller := NewDynamicController(matrix, queue)

	strategy := controller.Recompute()
	if strategy != StrategyDepth {
		t.Errorf("expected %v for high coverage, got %v", StrategyDepth, strategy)
	}
}

func TestDynamicController_RecomputeMixed(t *testing.T) {
	matrix := NewCoverageMatrix()
	matrix.Update(DomainWebAPI, DimensionRoutes, CoverageCounters{Discovered: 100, Tested: 50})

	queue := NewQueue()
	controller := NewDynamicController(matrix, queue)

	strategy := controller.Recompute()
	if strategy != StrategyMixed {
		t.Errorf("expected %v for mid coverage, got %v", StrategyMixed, strategy)
	}
}

func TestDynamicController_ShouldWiden(t *testing.T) {
	matrix := NewCoverageMatrix()
	queue := NewQueue()
	controller := NewDynamicController(matrix, queue)
	controller.strategy = StrategyBreadth

	if !controller.ShouldWiden() {
		t.Error("expected ShouldWiden to return true for breadth strategy")
	}
}

func TestDynamicController_ShouldDeepen(t *testing.T) {
	matrix := NewCoverageMatrix()
	queue := NewQueue()
	controller := NewDynamicController(matrix, queue)
	controller.strategy = StrategyDepth

	if !controller.ShouldDeepen() {
		t.Error("expected ShouldDeepen to return true for depth strategy")
	}
}
