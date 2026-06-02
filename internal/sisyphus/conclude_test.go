package sisyphus

import (
	"strings"
	"testing"
	"time"

	"github.com/ouroboros/ouroboros/internal/blackboard"
)

func TestConcludeFallbackWritesUsefulFactForTimedOutAttempt(t *testing.T) {
	store := blackboard.NewMemoryStore()
	intent, err := store.OpenIntent(blackboard.Intent{
		Kind:        "explore",
		Goal:        "test admin access",
		SourceFacts: []string{"fact-target", "fact-identity"},
	})
	if err != nil {
		t.Fatalf("open intent: %v", err)
	}
	lease, err := store.ClaimIntent(intent.ID, "agent-web", time.Minute)
	if err != nil {
		t.Fatalf("claim intent: %v", err)
	}

	engine := Engine{}
	result, err := engine.ConcludeFallback(store, intent.ID, lease.ID, Attempt{
		ID:                "attempt-1",
		IntentID:          intent.ID,
		TaskKind:          TaskExplore,
		PartialTranscript: "observed /admin returns 403 for user token\nlead: retry with admin-like role if credential appears\nnext command: run heavier enumeration",
	}, FailureReason{
		Kind:    FailureTimeout,
		Message: "worker timed out",
	})
	if err != nil {
		t.Fatalf("conclude fallback: %v", err)
	}

	if result.Task.Kind != TaskConclude {
		t.Fatalf("expected conclude task, got %s", result.Task.Kind)
	}
	if result.Failure.Kind != FailureTimeout {
		t.Fatalf("expected timeout failure, got %s", result.Failure.Kind)
	}
	if result.Fact.ID == "" {
		t.Fatalf("expected result fact")
	}
	if result.Fact.Kind != "attempt_summary" {
		t.Fatalf("expected attempt_summary fact, got %s", result.Fact.Kind)
	}
	if !strings.Contains(result.Fact.Statement, "/admin returns 403") {
		t.Fatalf("expected useful transcript summary, got %q", result.Fact.Statement)
	}

	stored, ok := store.Intent(intent.ID)
	if !ok {
		t.Fatalf("expected stored intent")
	}
	if stored.State != blackboard.IntentConcluded {
		t.Fatalf("expected concluded intent, got %s", stored.State)
	}
	if stored.ResultFact != result.Fact.ID {
		t.Fatalf("expected result fact id on intent")
	}
}

func TestConcludeFallbackReturnsReusableLeadsOnly(t *testing.T) {
	store, intentID, leaseID := claimedIntent(t)
	engine := Engine{}

	result, err := engine.ConcludeFallback(store, intentID, leaseID, Attempt{
		ID:                "attempt-2",
		IntentID:          intentID,
		TaskKind:          TaskExplore,
		PartialTranscript: "confirmed login page exists\nlead: csrf token is stable across refreshes\nhint: candidate endpoint /api/admin\nnext command: brute force directory list",
	}, FailureReason{
		Kind:    FailureParseFailed,
		Message: "parser could not normalize output",
	})
	if err != nil {
		t.Fatalf("conclude fallback: %v", err)
	}

	if len(result.MutationHints) != 2 {
		t.Fatalf("expected two reusable leads, got %d: %#v", len(result.MutationHints), result.MutationHints)
	}
	for _, hint := range result.MutationHints {
		if strings.Contains(hint.Message, "next command") || strings.Contains(hint.Message, "brute force") {
			t.Fatalf("fallback must not return action commands as reusable leads: %#v", result.MutationHints)
		}
	}
}

func TestConcludeFallbackSupportsWorkerExitWithPartialTranscript(t *testing.T) {
	store, intentID, leaseID := claimedIntent(t)
	engine := Engine{}

	result, err := engine.ConcludeFallback(store, intentID, leaseID, Attempt{
		ID:                "attempt-3",
		IntentID:          intentID,
		TaskKind:          TaskExplore,
		PartialTranscript: "worker exited after collecting service banner\nreusable: banner says nginx/1.18",
	}, FailureReason{
		Kind:    FailureWorkerExit,
		Message: "worker exited before parser finished",
	})
	if err != nil {
		t.Fatalf("conclude fallback: %v", err)
	}

	if result.Failure.Kind != FailureWorkerExit {
		t.Fatalf("expected worker exit failure, got %s", result.Failure.Kind)
	}
	if !strings.Contains(result.Fact.Statement, "service banner") {
		t.Fatalf("expected partial transcript summary, got %q", result.Fact.Statement)
	}
}

func claimedIntent(t *testing.T) (*blackboard.MemoryStore, string, string) {
	t.Helper()
	store := blackboard.NewMemoryStore()
	intent, err := store.OpenIntent(blackboard.Intent{
		Kind:        "explore",
		Goal:        "collect evidence",
		SourceFacts: []string{"fact-target"},
	})
	if err != nil {
		t.Fatalf("open intent: %v", err)
	}
	lease, err := store.ClaimIntent(intent.ID, "agent-web", time.Minute)
	if err != nil {
		t.Fatalf("claim intent: %v", err)
	}
	return store, intent.ID, lease.ID
}
