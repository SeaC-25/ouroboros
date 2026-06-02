package modelruntime

import (
	"testing"
	"time"
)

func TestLedgerAggregatesUsageByAgentRoleRunAndModel(t *testing.T) {
	ledger := NewLedger()
	ledger.Record(UsageRecord{
		RunID:            "run-a",
		AgentID:          "agent-web",
		Role:             "web-api",
		Provider:         "openai",
		Model:            "gpt-x",
		InputTokens:      1000,
		OutputTokens:     200,
		CacheReadTokens:  50,
		CacheWriteTokens: 25,
		Cost:             0.12,
		Latency:          2 * time.Second,
	})
	ledger.Record(UsageRecord{
		RunID:        "run-a",
		AgentID:      "agent-web",
		Role:         "web-api",
		Provider:     "openai",
		Model:        "gpt-x",
		InputTokens:  500,
		OutputTokens: 100,
		Cost:         0.06,
		Latency:      time.Second,
		Failed:       true,
	})
	ledger.Record(UsageRecord{
		RunID:        "run-b",
		AgentID:      "agent-infra",
		Role:         "infra",
		Provider:     "openai",
		Model:        "gpt-y",
		InputTokens:  2000,
		OutputTokens: 500,
		Cost:         0.50,
		Latency:      3 * time.Second,
	})

	agent := ledger.ByAgent("agent-web")
	if agent.InputTokens != 1500 || agent.OutputTokens != 300 || agent.FailureCount != 1 {
		t.Fatalf("unexpected agent aggregate: %#v", agent)
	}
	if agent.CacheReadTokens != 50 || agent.CacheWriteTokens != 25 {
		t.Fatalf("unexpected cache aggregate: %#v", agent)
	}
	if agent.Cost != 0.18 {
		t.Fatalf("unexpected cost: %f", agent.Cost)
	}

	role := ledger.ByRole("web-api")
	if role.InputTokens != 1500 || role.Latency != 3*time.Second {
		t.Fatalf("unexpected role aggregate: %#v", role)
	}

	run := ledger.ByRun("run-a")
	if run.InputTokens != 1500 || run.FailureCount != 1 {
		t.Fatalf("unexpected run aggregate: %#v", run)
	}

	model := ledger.ByModel("gpt-y")
	if model.InputTokens != 2000 || model.Cost != 0.50 {
		t.Fatalf("unexpected model aggregate: %#v", model)
	}
}

func TestLedgerReturnsZeroAggregateForMissingKey(t *testing.T) {
	ledger := NewLedger()
	aggregate := ledger.ByAgent("missing")

	if aggregate.InputTokens != 0 || aggregate.Cost != 0 {
		t.Fatalf("expected zero aggregate, got %#v", aggregate)
	}
}
