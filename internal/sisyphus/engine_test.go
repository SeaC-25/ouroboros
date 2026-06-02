package sisyphus

import (
	"testing"

	"github.com/ouroboros/ouroboros/internal/blackboard"
)

func TestStepSchedulesBootstrapForNewProject(t *testing.T) {
	engine := Engine{}

	task := engine.Step(blackboard.ProjectState{})

	if task.Kind != TaskBootstrap {
		t.Fatalf("expected bootstrap task, got %s", task.Kind)
	}
	if task.Reason == "" {
		t.Fatalf("expected scheduling reason")
	}
}

func TestStepSchedulesExploreForOpenIntent(t *testing.T) {
	engine := Engine{}
	state := blackboard.ProjectState{
		Facts: []blackboard.Fact{
			{ID: "fact-target", Kind: "target", Statement: "target observed"},
		},
		Intents: []blackboard.Intent{
			{
				ID:    "intent-1",
				Kind:  "explore",
				Goal:  "enumerate routes",
				State: blackboard.IntentOpen,
			},
		},
	}

	task := engine.Step(state)

	if task.Kind != TaskExplore {
		t.Fatalf("expected explore task, got %s", task.Kind)
	}
	if task.IntentID != "intent-1" {
		t.Fatalf("expected intent-1, got %s", task.IntentID)
	}
}

func TestStepSchedulesReasonWhenNoOpenIntentExists(t *testing.T) {
	engine := Engine{}
	state := blackboard.ProjectState{
		Facts: []blackboard.Fact{
			{ID: "fact-target", Kind: "target", Statement: "target observed"},
		},
		Intents: []blackboard.Intent{
			{
				ID:    "intent-claimed",
				Kind:  "explore",
				Goal:  "already being explored",
				State: blackboard.IntentClaimed,
			},
		},
	}

	task := engine.Step(state)

	if task.Kind != TaskReason {
		t.Fatalf("expected reason task, got %s", task.Kind)
	}
}

func TestTaskModelIncludesConcludeFallbackTypes(t *testing.T) {
	result := TaskResult{
		Task: Task{
			Kind: TaskConclude,
		},
		Attempt: Attempt{
			ID: "attempt-1",
		},
		Failure: FailureReason{
			Kind:    "timeout",
			Message: "worker timed out with partial transcript",
		},
		MutationHints: []MutationHint{
			{Kind: "reduce_scope", Message: "focus on authenticated routes"},
		},
	}

	if result.Task.Kind != TaskConclude {
		t.Fatalf("expected conclude task kind")
	}
	if result.Failure.Kind != "timeout" {
		t.Fatalf("unexpected failure kind: %s", result.Failure.Kind)
	}
	if len(result.MutationHints) != 1 {
		t.Fatalf("expected mutation hint")
	}
}
