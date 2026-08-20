package goal

import (
	"math"
	"testing"
)

func TestGoalStackPushPeekPopHonorsPriorityAndDependencies(t *testing.T) {
	stack := validGoalStack()

	if err := stack.Push(GoalNode{
		ID:       "coverage",
		Type:     GoalTypeCoverage,
		Title:    "map authenticated routes",
		Priority: PriorityHigh,
	}); err != nil {
		t.Fatalf("push coverage: %v", err)
	}
	if err := stack.Push(GoalNode{
		ID:           "evidence",
		Type:         GoalTypeEvidence,
		Title:        "prove the best finding",
		Priority:     PriorityCritical,
		Dependencies: []GoalID{"coverage"},
	}); err != nil {
		t.Fatalf("push evidence: %v", err)
	}
	if err := stack.Push(GoalNode{
		ID:       "report",
		Type:     GoalTypeReport,
		Title:    "draft report shell",
		Priority: PriorityLow,
	}); err != nil {
		t.Fatalf("push report: %v", err)
	}

	got, ok := stack.Peek()
	if !ok {
		t.Fatalf("expected a ready goal")
	}
	if got.ID != "coverage" {
		t.Fatalf("expected coverage first because evidence is blocked, got %q", got.ID)
	}

	popped, ok := stack.Pop()
	if !ok {
		t.Fatalf("expected pop to return coverage")
	}
	if popped.ID != "coverage" || popped.Status != GoalStatusActive {
		t.Fatalf("expected active coverage, got %+v", popped)
	}

	got, ok = stack.Peek()
	if !ok {
		t.Fatalf("expected report to remain ready while coverage is active")
	}
	if got.ID != "report" {
		t.Fatalf("expected report while evidence remains blocked, got %q", got.ID)
	}

	if err := stack.Complete("coverage"); err != nil {
		t.Fatalf("complete coverage: %v", err)
	}

	got, ok = stack.Peek()
	if !ok {
		t.Fatalf("expected evidence to become ready")
	}
	if got.ID != "evidence" {
		t.Fatalf("expected evidence after dependency completion, got %q", got.ID)
	}
}

func TestGoalStackTracksProgressAndCompletion(t *testing.T) {
	stack := validGoalStack()

	if err := stack.Push(GoalNode{ID: "mission", Type: GoalTypeMission, Priority: PriorityHigh}); err != nil {
		t.Fatalf("push mission: %v", err)
	}
	if err := stack.Push(GoalNode{ID: "report", Type: GoalTypeReport, Priority: PriorityMedium}); err != nil {
		t.Fatalf("push report: %v", err)
	}

	if err := stack.UpdateProgress("mission", 0.50); err != nil {
		t.Fatalf("update mission progress: %v", err)
	}
	if err := stack.Complete("report"); err != nil {
		t.Fatalf("complete report: %v", err)
	}

	snapshot := stack.Completion()
	if snapshot.Total != 2 {
		t.Fatalf("expected total 2, got %d", snapshot.Total)
	}
	if snapshot.Completed != 1 {
		t.Fatalf("expected one completed goal, got %d", snapshot.Completed)
	}
	if math.Abs(snapshot.Progress-0.75) > 0.000001 {
		t.Fatalf("expected aggregate progress 0.75, got %.6f", snapshot.Progress)
	}
	if snapshot.ByType[GoalTypeReport].Completed != 1 {
		t.Fatalf("expected report completion by type, got %+v", snapshot.ByType)
	}
}

func TestGoalStackRejectsDuplicateGoalsAndInvalidProgress(t *testing.T) {
	stack := validGoalStack()

	if err := stack.Push(GoalNode{ID: "mission", Type: GoalTypeMission}); err != nil {
		t.Fatalf("push mission: %v", err)
	}
	if err := stack.Push(GoalNode{ID: "mission", Type: GoalTypeMission}); err == nil {
		t.Fatalf("expected duplicate goal to fail")
	}
	if err := stack.UpdateProgress("mission", 1.20); err == nil {
		t.Fatalf("expected progress over 1.0 to fail")
	}
	if err := stack.Complete("missing"); err == nil {
		t.Fatalf("expected missing goal completion to fail")
	}
}
