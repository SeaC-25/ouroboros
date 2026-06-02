package blackboard

import (
	"testing"
	"time"
)

func TestAppendFactIsAppendOnly(t *testing.T) {
	store := NewMemoryStore()
	fact, err := store.AppendFact(Fact{
		Kind:       "target",
		Statement:  "https://example.test is in scope",
		Confidence: 0.95,
		Source:     "mission",
	})
	if err != nil {
		t.Fatalf("append fact: %v", err)
	}

	facts := store.Facts()
	if len(facts) != 1 {
		t.Fatalf("expected one fact, got %d", len(facts))
	}
	if facts[0].ID == "" {
		t.Fatalf("expected generated fact id")
	}
	if facts[0].ID != fact.ID {
		t.Fatalf("stored fact id mismatch: %q != %q", facts[0].ID, fact.ID)
	}

	facts[0].Statement = "mutated by caller"
	again := store.Facts()
	if again[0].Statement != "https://example.test is in scope" {
		t.Fatalf("facts must be returned as immutable snapshots")
	}
}

func TestIntentClaimHeartbeatReleaseLifecycle(t *testing.T) {
	store := NewMemoryStore()
	intent, err := store.OpenIntent(Intent{
		Kind:        "explore",
		Goal:        "enumerate authenticated routes",
		SourceFacts: []string{"fact-1"},
	})
	if err != nil {
		t.Fatalf("open intent: %v", err)
	}
	if intent.State != IntentOpen {
		t.Fatalf("expected open intent, got %s", intent.State)
	}

	lease, err := store.ClaimIntent(intent.ID, "agent-web", time.Minute)
	if err != nil {
		t.Fatalf("claim intent: %v", err)
	}
	if lease.AgentID != "agent-web" {
		t.Fatalf("unexpected lease agent: %s", lease.AgentID)
	}

	claimed, ok := store.Intent(intent.ID)
	if !ok {
		t.Fatalf("expected intent")
	}
	if claimed.State != IntentClaimed {
		t.Fatalf("expected claimed intent, got %s", claimed.State)
	}

	renewed, err := store.Heartbeat(intent.ID, lease.ID, time.Hour)
	if err != nil {
		t.Fatalf("heartbeat: %v", err)
	}
	if !renewed.ExpiresAt.After(lease.ExpiresAt) {
		t.Fatalf("expected heartbeat to extend lease")
	}

	released, err := store.ReleaseIntent(intent.ID, lease.ID)
	if err != nil {
		t.Fatalf("release intent: %v", err)
	}
	if released.State != IntentReleased {
		t.Fatalf("expected released intent, got %s", released.State)
	}
}

func TestConcludeIntentRecordsResultFact(t *testing.T) {
	store := NewMemoryStore()
	intent, err := store.OpenIntent(Intent{
		Kind:        "explore",
		Goal:        "test admin endpoint access",
		SourceFacts: []string{"fact-target", "fact-identity"},
	})
	if err != nil {
		t.Fatalf("open intent: %v", err)
	}
	lease, err := store.ClaimIntent(intent.ID, "agent-web", time.Minute)
	if err != nil {
		t.Fatalf("claim intent: %v", err)
	}

	result, err := store.ConcludeIntent(intent.ID, lease.ID, Fact{
		Kind:       "finding",
		Statement:  "admin endpoint denies user role",
		Confidence: 0.9,
		Source:     "agent-web",
	})
	if err != nil {
		t.Fatalf("conclude intent: %v", err)
	}
	if result.Intent.State != IntentConcluded {
		t.Fatalf("expected concluded intent, got %s", result.Intent.State)
	}
	if result.Fact.ID == "" {
		t.Fatalf("expected result fact id")
	}
	if got := len(store.Facts()); got != 1 {
		t.Fatalf("expected one result fact, got %d", got)
	}
}

func TestOpenIntentPreservesMultipleSourceFacts(t *testing.T) {
	store := NewMemoryStore()
	intent, err := store.OpenIntent(Intent{
		Kind:        "explore",
		Goal:        "combine credential and service observations",
		SourceFacts: []string{"fact-credential", "fact-service", "fact-network-position"},
	})
	if err != nil {
		t.Fatalf("open intent: %v", err)
	}

	if len(intent.SourceFacts) != 3 {
		t.Fatalf("expected three source facts, got %d", len(intent.SourceFacts))
	}
	intent.SourceFacts[0] = "mutated"

	stored, ok := store.Intent(intent.ID)
	if !ok {
		t.Fatalf("expected stored intent")
	}
	if stored.SourceFacts[0] != "fact-credential" {
		t.Fatalf("source facts must be snapshot copies")
	}
}
