package compiler

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/ouroboros/ouroboros/internal/blackboard"
	"github.com/ouroboros/ouroboros/internal/eventlog"
	"github.com/ouroboros/ouroboros/internal/evidence"
	"github.com/ouroboros/ouroboros/internal/sisyphus"
)

func TestBindParserOutputConvertsParsedJSONToFactAndEvents(t *testing.T) {
	store := blackboard.NewMemoryStore()

	result, err := BindParserOutput(ParserBindingRequest{
		RunID: "run-parser-success",
		PlanNode: PlanNode{
			ID:         "discover_assets:01",
			Parser:     "route_observation_parser",
			Capability: "web.route_enumeration",
		},
		Spec: ParserSpec{
			Name:            "route_observation_parser",
			FactKind:        "surface",
			StatementField:  "route",
			ConfidenceField: "confidence",
			RequiredFields:  []string{"route"},
		},
		Output: []byte(`{"route":"/admin","status":403,"confidence":0.91}`),
		Store:  store,
	})
	if err != nil {
		t.Fatalf("bind parser output: %v", err)
	}

	if result.Fallback != nil {
		t.Fatalf("did not expect fallback on successful parse")
	}
	if result.Fact.ID == "" {
		t.Fatalf("expected fact id")
	}
	if result.Fact.Kind != "surface" {
		t.Fatalf("expected surface fact, got %s", result.Fact.Kind)
	}
	if result.Fact.Statement != "/admin" {
		t.Fatalf("expected parsed route statement, got %q", result.Fact.Statement)
	}
	if result.Fact.Confidence != 0.91 {
		t.Fatalf("expected parsed confidence, got %f", result.Fact.Confidence)
	}

	graph := evidence.Project(store.State())
	node, ok := graph.Node(result.Fact.ID)
	if !ok {
		t.Fatalf("expected fact projected into evidence graph")
	}
	if node.Type != evidence.NodeSurface {
		t.Fatalf("expected surface evidence node, got %s", node.Type)
	}

	if len(result.Events) != 2 {
		t.Fatalf("expected parser and fact events, got %d", len(result.Events))
	}
	if result.Events[0].Type != eventlog.EventParser || result.Events[1].Type != eventlog.EventFact {
		t.Fatalf("unexpected event sequence: %#v", result.Events)
	}
	for _, event := range result.Events {
		if event.RunID != "run-parser-success" {
			t.Fatalf("expected run id on event, got %s", event.RunID)
		}
		if !json.Valid(event.Payload) {
			t.Fatalf("event payload must be JSON: %s", string(event.Payload))
		}
	}
}

func TestBindParserOutputTriggersConcludeFallbackOnParseFailure(t *testing.T) {
	store := blackboard.NewMemoryStore()
	intent, err := store.OpenIntent(blackboard.Intent{
		Kind:        "explore",
		Goal:        "parse route enumeration output",
		SourceFacts: []string{"fact-target"},
	})
	if err != nil {
		t.Fatalf("open intent: %v", err)
	}
	lease, err := store.ClaimIntent(intent.ID, "agent-web", time.Minute)
	if err != nil {
		t.Fatalf("claim intent: %v", err)
	}

	result, err := BindParserOutput(ParserBindingRequest{
		RunID: "run-parser-failure",
		PlanNode: PlanNode{
			ID:     "enumerate_paths:01",
			Parser: "route_observation_parser",
		},
		Spec: ParserSpec{
			Name:           "route_observation_parser",
			FactKind:       "surface",
			StatementField: "route",
			RequiredFields: []string{"route"},
		},
		Output:   []byte("observed /admin returns 403\nlead: retry with authenticated context"),
		Store:    store,
		IntentID: intent.ID,
		LeaseID:  lease.ID,
		Attempt: sisyphus.Attempt{
			ID:       "attempt-parse-1",
			IntentID: intent.ID,
			TaskKind: sisyphus.TaskExplore,
		},
	})
	if err != nil {
		t.Fatalf("bind parser output: %v", err)
	}
	if result.Fallback == nil {
		t.Fatalf("expected conclude fallback")
	}
	if result.Fallback.Task.Kind != sisyphus.TaskConclude {
		t.Fatalf("expected conclude task, got %s", result.Fallback.Task.Kind)
	}
	if result.Fallback.Failure.Kind != sisyphus.FailureParseFailed {
		t.Fatalf("expected parse failed reason, got %s", result.Fallback.Failure.Kind)
	}
	if !strings.Contains(result.Fallback.Fact.Statement, "/admin returns 403") {
		t.Fatalf("expected partial transcript in fallback fact, got %q", result.Fallback.Fact.Statement)
	}
	if len(result.Fallback.MutationHints) != 1 {
		t.Fatalf("expected reusable lead, got %#v", result.Fallback.MutationHints)
	}

	storedIntent, ok := store.Intent(intent.ID)
	if !ok {
		t.Fatalf("expected stored intent")
	}
	if storedIntent.State != blackboard.IntentConcluded {
		t.Fatalf("expected concluded intent, got %s", storedIntent.State)
	}

	if len(result.Events) != 2 {
		t.Fatalf("expected parser and fallback fact events, got %d", len(result.Events))
	}
	if result.Events[0].Type != eventlog.EventParser || result.Events[1].Type != eventlog.EventFact {
		t.Fatalf("unexpected fallback event sequence: %#v", result.Events)
	}
}
