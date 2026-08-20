package goal

import "testing"

func TestDependencyGraphRejectsCyclesAndReportsReadyGoals(t *testing.T) {
	graph := NewDependencyGraph()

	graph.AddGoal("coverage")
	graph.AddGoal("evidence")
	graph.AddGoal("report")
	if err := graph.AddDependency("evidence", "coverage"); err != nil {
		t.Fatalf("add evidence dependency: %v", err)
	}
	if err := graph.AddDependency("report", "evidence"); err != nil {
		t.Fatalf("add report dependency: %v", err)
	}

	if err := graph.AddDependency("coverage", "report"); err == nil {
		t.Fatalf("expected cycle to be rejected")
	}

	ready := graph.ReadyGoals(map[GoalID]bool{})
	if len(ready) != 1 || ready[0] != "coverage" {
		t.Fatalf("expected only coverage to be ready, got %v", ready)
	}

	ready = graph.ReadyGoals(map[GoalID]bool{"coverage": true})
	if len(ready) != 1 || ready[0] != "evidence" {
		t.Fatalf("expected evidence after coverage completion, got %v", ready)
	}
}

func TestDependencyGraphTopologicalOrderIsDeterministic(t *testing.T) {
	graph := NewDependencyGraph()

	graph.AddGoal("report")
	graph.AddGoal("coverage")
	graph.AddGoal("evidence")
	if err := graph.AddDependency("report", "evidence"); err != nil {
		t.Fatalf("add report dependency: %v", err)
	}
	if err := graph.AddDependency("evidence", "coverage"); err != nil {
		t.Fatalf("add evidence dependency: %v", err)
	}

	got, err := graph.TopologicalOrder()
	if err != nil {
		t.Fatalf("topological order: %v", err)
	}
	want := []GoalID{"coverage", "evidence", "report"}
	if len(got) != len(want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("expected %v, got %v", want, got)
		}
	}
}
