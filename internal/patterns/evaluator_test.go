package patterns

import (
	"path/filepath"
	"testing"
)

func TestEvaluatorRalphLoopRetry(t *testing.T) {
	pattern := loadPatternByName(t, "ralphloop")

	transition, err := Next(pattern, "evaluate", ConditionResults{
		"failed_retryable": true,
	})
	if err != nil {
		t.Fatalf("next: %v", err)
	}
	if transition.To != "mutate" {
		t.Fatalf("expected evaluate -> mutate, got %s -> %s", transition.From, transition.To)
	}
}

func TestEvaluatorExplorePlanExecuteProgression(t *testing.T) {
	pattern := loadPatternByName(t, "explore_plan_execute")

	first, err := Next(pattern, "explore", ConditionResults{"signal_found": true})
	if err != nil {
		t.Fatalf("next explore: %v", err)
	}
	if first.To != "plan" {
		t.Fatalf("expected explore -> plan, got %s", first.To)
	}

	second, err := Next(pattern, first.To, ConditionResults{"plan_ready": true})
	if err != nil {
		t.Fatalf("next plan: %v", err)
	}
	if second.To != "execute" {
		t.Fatalf("expected plan -> execute, got %s", second.To)
	}
}

func TestEvaluatorAntColonyDeposit(t *testing.T) {
	pattern := loadPatternByName(t, "ant_colony")

	transition, err := Next(pattern, "explore_path", ConditionResults{
		"signal_found": true,
	})
	if err != nil {
		t.Fatalf("next: %v", err)
	}
	if transition.To != "deposit" {
		t.Fatalf("expected explore_path -> deposit, got %s", transition.To)
	}
}

func TestEvaluatorUsesEdgeOrderDeterministically(t *testing.T) {
	pattern := Pattern{
		Name:     "ordered",
		MaxSteps: 3,
		Nodes: []Node{
			{ID: "start", Kind: "decision"},
			{ID: "first", Kind: "result", Terminal: true},
			{ID: "second", Kind: "result", Terminal: true},
		},
		Edges: []Edge{
			{From: "start", To: "first", Condition: "ready"},
			{From: "start", To: "second", Condition: "ready"},
		},
		Conditions: []Condition{
			{Name: "ready", Description: "both edges are available"},
		},
	}

	transition, err := Next(pattern, "start", ConditionResults{"ready": true})
	if err != nil {
		t.Fatalf("next: %v", err)
	}
	if transition.To != "first" {
		t.Fatalf("expected first matching edge, got %s", transition.To)
	}
}

func loadPatternByName(t *testing.T, name string) Pattern {
	t.Helper()
	patterns, err := LoadDir(filepath.Join("..", "..", "configs", "patterns"))
	if err != nil {
		t.Fatalf("load patterns: %v", err)
	}
	for _, pattern := range patterns {
		if pattern.Name == name {
			return pattern
		}
	}
	t.Fatalf("pattern not found: %s", name)
	return Pattern{}
}
