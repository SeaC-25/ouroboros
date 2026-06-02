package swarm

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/ouroboros/ouroboros/internal/blackboard"
	"github.com/ouroboros/ouroboros/internal/evidence"
)

func TestOrchestratorWritesValidatedAgentOutputToBlackboardAndEvidence(t *testing.T) {
	store := blackboard.NewMemoryStore()
	driver := staticDriver{result: AgentResult{
		TaskID: "task-1",
		State:  AgentComplete,
		Output: StructuredOutput{Proposals: []Proposal{
			mustProposal(t, ProposalFact, blackboard.Fact{
				Kind:       "target",
				Statement:  "https://example.test is in scope",
				Confidence: 0.95,
				Source:     "agent-web",
			}),
			mustProposal(t, ProposalIntent, blackboard.Intent{
				Kind:        "explore",
				Goal:        "enumerate authenticated routes",
				SourceFacts: []string{"fact-seed"},
			}),
			mustProposal(t, ProposalHint, blackboard.Hint{
				Kind:    "priority",
				Message: "prioritize authenticated route coverage",
				Source:  "agent-web",
			}),
		}},
	}}
	orchestrator := NewOrchestrator(store, driver)

	result, err := orchestrator.Execute(context.Background(), AgentTask{
		ID:   "task-1",
		Role: RoleWebAPI,
		Goal: "discover initial web surface",
	})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if result.State != AgentComplete {
		t.Fatalf("expected complete result, got %s", result.State)
	}

	state := store.State()
	if len(state.Facts) != 1 {
		t.Fatalf("expected one fact, got %d", len(state.Facts))
	}
	if state.Facts[0].ID == "" || state.Facts[0].Statement != "https://example.test is in scope" {
		t.Fatalf("unexpected fact: %#v", state.Facts[0])
	}
	if len(state.Intents) != 1 {
		t.Fatalf("expected one intent, got %d", len(state.Intents))
	}
	if len(state.Hints) != 1 {
		t.Fatalf("expected one hint, got %d", len(state.Hints))
	}

	graph := orchestrator.EvidenceGraph()
	node, ok := graph.Node(state.Facts[0].ID)
	if !ok {
		t.Fatalf("expected evidence node for written fact")
	}
	if node.Type != evidence.NodeTarget {
		t.Fatalf("expected target node, got %s", node.Type)
	}
}

func TestOrchestratorRejectsMalformedOutputWithoutWritingState(t *testing.T) {
	store := blackboard.NewMemoryStore()
	driver := staticDriver{result: AgentResult{
		TaskID: "task-1",
		State:  AgentComplete,
		Output: StructuredOutput{Proposals: []Proposal{
			{Type: ProposalType("direct_state_write"), Payload: json.RawMessage(`{"unsafe":true}`)},
		}},
	}}
	orchestrator := NewOrchestrator(store, driver)

	_, err := orchestrator.Execute(context.Background(), AgentTask{ID: "task-1", Role: RolePlanner})
	if err == nil {
		t.Fatalf("expected malformed output to fail")
	}

	state := store.State()
	if len(state.Facts) != 0 || len(state.Intents) != 0 || len(state.Hints) != 0 {
		t.Fatalf("orchestrator must not write invalid output: %#v", state)
	}
}

func TestOrchestratorProgressAggregationAndBlockedState(t *testing.T) {
	store := blackboard.NewMemoryStore()
	driver := sequenceDriver{results: []AgentResult{
		{TaskID: "task-1", State: AgentComplete, Output: StructuredOutput{}},
		{TaskID: "task-2", State: AgentBlocked, Output: StructuredOutput{}},
	}}
	orchestrator := NewOrchestrator(store, &driver)

	if _, err := orchestrator.Execute(context.Background(), AgentTask{ID: "task-1", Role: RolePlanner}); err != nil {
		t.Fatalf("execute first: %v", err)
	}
	if _, err := orchestrator.Execute(context.Background(), AgentTask{ID: "task-2", Role: RoleInfra}); err != nil {
		t.Fatalf("execute second: %v", err)
	}

	progress := orchestrator.Progress()
	if progress.Total != 2 || progress.Complete != 1 || progress.Blocked != 1 {
		t.Fatalf("unexpected progress: %#v", progress)
	}
}

type staticDriver struct {
	result AgentResult
}

func (d staticDriver) Run(ctx context.Context, task AgentTask) (AgentResult, error) {
	return d.result, nil
}

type sequenceDriver struct {
	results []AgentResult
	next    int
}

func (d *sequenceDriver) Run(ctx context.Context, task AgentTask) (AgentResult, error) {
	result := d.results[d.next]
	d.next++
	return result, nil
}

func mustProposal(t *testing.T, proposalType ProposalType, payload any) Proposal {
	t.Helper()
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal proposal: %v", err)
	}
	return Proposal{Type: proposalType, Payload: data}
}
