package evidence

import (
	"testing"

	"github.com/ouroboros/ouroboros/internal/blackboard"
)

func TestProjectFactToEvidenceNode(t *testing.T) {
	graph := Project(blackboard.ProjectState{
		Facts: []blackboard.Fact{
			{
				ID:         "fact-target",
				Kind:       "target",
				Statement:  "https://example.test is in scope",
				Confidence: 0.95,
				Source:     "mission",
			},
			{
				ID:         "fact-finding",
				Kind:       "finding",
				Statement:  "admin route denies user role",
				Confidence: 0.90,
				Source:     "verifier",
			},
		},
	})

	target, ok := graph.Node("fact-target")
	if !ok {
		t.Fatalf("expected target node")
	}
	if target.Type != NodeTarget {
		t.Fatalf("expected target node type, got %s", target.Type)
	}
	if target.Label != "https://example.test is in scope" {
		t.Fatalf("unexpected target label: %q", target.Label)
	}

	finding, ok := graph.Node("fact-finding")
	if !ok {
		t.Fatalf("expected finding node")
	}
	if finding.Type != NodeFinding {
		t.Fatalf("expected finding node type, got %s", finding.Type)
	}
}

func TestProjectIntentToEvidenceEdges(t *testing.T) {
	graph := Project(blackboard.ProjectState{
		Facts: []blackboard.Fact{
			{ID: "fact-credential", Kind: "identity", Statement: "user token observed"},
			{ID: "fact-service", Kind: "surface", Statement: "admin route discovered"},
		},
		Intents: []blackboard.Intent{
			{
				ID:          "intent-admin-check",
				Kind:        "explore",
				Goal:        "test whether token unlocks admin route",
				SourceFacts: []string{"fact-credential", "fact-service"},
				State:       blackboard.IntentClaimed,
			},
		},
	})

	intentNode, ok := graph.Node("intent-admin-check")
	if !ok {
		t.Fatalf("expected intent node")
	}
	if intentNode.Type != NodeHypothesis {
		t.Fatalf("expected hypothesis node type, got %s", intentNode.Type)
	}

	requires := graph.EdgesOfType(EdgeRequires)
	if len(requires) != 2 {
		t.Fatalf("expected two requires edges, got %d", len(requires))
	}
	assertEdge(t, requires, "intent-admin-check", "fact-credential")
	assertEdge(t, requires, "intent-admin-check", "fact-service")
}

func TestProjectConcludedIntentUnlocksResultFact(t *testing.T) {
	graph := Project(blackboard.ProjectState{
		Facts: []blackboard.Fact{
			{ID: "fact-service", Kind: "surface", Statement: "admin route discovered"},
			{ID: "fact-result", Kind: "finding", Statement: "admin route is not accessible"},
		},
		Intents: []blackboard.Intent{
			{
				ID:          "intent-admin-check",
				Kind:        "explore",
				Goal:        "test admin route",
				SourceFacts: []string{"fact-service"},
				State:       blackboard.IntentConcluded,
				ResultFact:  "fact-result",
			},
		},
	})

	derived := graph.EdgesOfType(EdgeDerivedFrom)
	if len(derived) != 1 {
		t.Fatalf("expected one derived_from edge, got %d", len(derived))
	}
	assertEdge(t, derived, "fact-result", "intent-admin-check")
}

func TestGraphReturnsSnapshotCollections(t *testing.T) {
	graph := Project(blackboard.ProjectState{
		Facts: []blackboard.Fact{
			{ID: "fact-target", Kind: "target", Statement: "target"},
		},
	})

	nodes := graph.Nodes()
	nodes[0].Label = "mutated"

	node, ok := graph.Node("fact-target")
	if !ok {
		t.Fatalf("expected node")
	}
	if node.Label != "target" {
		t.Fatalf("graph nodes must be immutable snapshots")
	}
}

func assertEdge(t *testing.T, edges []Edge, from string, to string) {
	t.Helper()
	for _, edge := range edges {
		if edge.From == from && edge.To == to {
			return
		}
	}
	t.Fatalf("expected edge %s -> %s in %#v", from, to, edges)
}
