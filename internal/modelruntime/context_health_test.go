package modelruntime

import "testing"

func TestContextHealthBelowRollingSummaryThreshold(t *testing.T) {
	health := EvaluateContext(ContextWindow{
		UsedTokens:    69,
		MaxTokens:     100,
		PinnedObjects: []string{"mission", "scope"},
	})

	if health.State != ContextOK {
		t.Fatalf("expected ok, got %s", health.State)
	}
	if health.NextThreshold != 70 {
		t.Fatalf("expected next threshold 70, got %d", health.NextThreshold)
	}
	if len(health.PinnedObjects) != 2 {
		t.Fatalf("expected pinned objects")
	}
}

func TestContextHealthRollingSummaryThreshold(t *testing.T) {
	health := EvaluateContext(ContextWindow{UsedTokens: 70, MaxTokens: 100})

	if health.State != ContextRollingSummary {
		t.Fatalf("expected rolling summary, got %s", health.State)
	}
	if health.NextThreshold != 85 {
		t.Fatalf("expected next threshold 85, got %d", health.NextThreshold)
	}
}

func TestContextHealthCompactAttemptsThreshold(t *testing.T) {
	health := EvaluateContext(ContextWindow{UsedTokens: 85, MaxTokens: 100})

	if health.State != ContextCompactAttempts {
		t.Fatalf("expected compact attempts, got %s", health.State)
	}
	if health.NextThreshold != 95 {
		t.Fatalf("expected next threshold 95, got %d", health.NextThreshold)
	}
}

func TestContextHealthCheckpointThreshold(t *testing.T) {
	health := EvaluateContext(ContextWindow{UsedTokens: 95, MaxTokens: 100})

	if health.State != ContextCheckpoint {
		t.Fatalf("expected checkpoint, got %s", health.State)
	}
	if health.NextThreshold != 100 {
		t.Fatalf("expected terminal threshold 100, got %d", health.NextThreshold)
	}
}

func TestContextHealthPercentUsesTokenWindow(t *testing.T) {
	health := EvaluateContext(ContextWindow{UsedTokens: 50_000, MaxTokens: 200_000})

	if health.PercentUsed != 25 {
		t.Fatalf("expected 25 percent, got %d", health.PercentUsed)
	}
}
