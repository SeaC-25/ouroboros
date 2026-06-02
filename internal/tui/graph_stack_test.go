package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ouroboros/ouroboros/internal/evidence"
	"github.com/ouroboros/ouroboros/internal/swarm"
)

func TestAttackGraphRendersTextGraphAndSelection(t *testing.T) {
	graph := AttackGraph{
		Nodes: []evidence.Node{
			{ID: "target-1", Type: evidence.NodeTarget, Label: "app.example", SourceKind: "target"},
			{ID: "surface-1", Type: evidence.NodeSurface, Label: "/admin", SourceKind: "surface"},
		},
		Edges: []evidence.Edge{
			{From: "target-1", To: "surface-1", Type: evidence.EdgeObservedBy},
		},
	}.SelectNode("surface-1")

	view := graph.Render(54, 8)
	for _, text := range []string{
		"Attack Graph",
		"  target-1 [Target] app.example",
		"> surface-1 [Surface] /admin",
		"target-1 -observed_by-> surface-1",
	} {
		if !strings.Contains(view, text) {
			t.Fatalf("attack graph missing %q:\n%s", text, view)
		}
	}
}

func TestAgentGraphRendersStatusAndProgress(t *testing.T) {
	graph := AgentGraph{
		Agents: []AgentGraphNode{
			{AgentID: "agent-web-1", Role: swarm.RoleWebAPI, State: swarm.AgentRunning, ProgressPercent: 67},
			{AgentID: "agent-verifier", Role: swarm.RoleVerifier, State: swarm.AgentIdle, ProgressPercent: 12},
		},
	}.SelectAgent("agent-web-1")

	view := graph.Render(54, 6)
	for _, text := range []string{
		"Agent Graph",
		"> agent-web-1 web-api running 67%",
		"  agent-verifier verifier idle 12%",
	} {
		if !strings.Contains(view, text) {
			t.Fatalf("agent graph missing %q:\n%s", text, view)
		}
	}
}

func TestInspectorShowsSelectedAttackNodeDetails(t *testing.T) {
	stack := GraphStack{
		Attack: AttackGraph{
			Nodes: []evidence.Node{
				{ID: "finding-1", Type: evidence.NodeFinding, Label: "SQL injection candidate", SourceKind: "finding"},
			},
		},
	}.SelectAttackNode("finding-1")

	view := stack.Render(56, 14)
	for _, text := range []string{
		"Inspector",
		"selected: finding-1",
		"type: Finding",
		"label: SQL injection candidate",
		"source: finding",
	} {
		if !strings.Contains(view, text) {
			t.Fatalf("inspector missing %q:\n%s", text, view)
		}
	}
}

func TestGraphStackFocusAndAgentSelection(t *testing.T) {
	stack := GraphStack{
		Agents: AgentGraph{
			Agents: []AgentGraphNode{
				{AgentID: "agent-verifier", Role: swarm.RoleVerifier, State: swarm.AgentBlocked, ProgressPercent: 40},
			},
		},
	}.FocusPane(GraphFocusAgent).SelectAgent("agent-verifier")

	if stack.Focus != GraphFocusAgent {
		t.Fatalf("expected agent focus, got %s", stack.Focus)
	}
	view := stack.Render(56, 14)
	for _, text := range []string{
		"> agent-verifier verifier blocked 40%",
		"selected: agent-verifier",
		"role: verifier",
		"state: blocked",
	} {
		if !strings.Contains(view, text) {
			t.Fatalf("focused graph stack missing %q:\n%s", text, view)
		}
	}
}

func TestGraphStackInspectorShowsRuntimeActionDetails(t *testing.T) {
	stack := DefaultGraphStack().WithRuntimeAction(RuntimeAction{
		Kind:    "install",
		Name:    "httpx",
		State:   "missing",
		Command: "ouro-toolchain install httpx",
		Hint:    "install bundled httpx pack",
	})

	if stack.Focus != GraphFocusRuntimeAction {
		t.Fatalf("expected runtime action focus, got %s", stack.Focus)
	}
	if stack.RuntimeTabs.Active != RuntimeTabNodeInspector {
		t.Fatalf("expected node inspector tab, got %s", stack.RuntimeTabs.Active)
	}
	view := stack.Render(72, 20)
	for _, text := range []string{
		"[Node Inspector]",
		"runtime action: install",
		"name: httpx",
		"state: missing",
		"command: ouro-toolchain install httpx",
		"hint: install bundled httpx pack",
		"mode: metadata-only",
	} {
		if !strings.Contains(view, text) {
			t.Fatalf("runtime action inspector missing %q:\n%s", text, view)
		}
	}
}

func TestModelRendersGraphStackInRightColumn(t *testing.T) {
	model := NewModel().WithGraphStack(GraphStack{
		Attack: AttackGraph{
			Nodes: []evidence.Node{
				{ID: "target-1", Type: evidence.NodeTarget, Label: "app.example", SourceKind: "target"},
			},
		},
	}.SelectAttackNode("target-1"))

	updated, _ := model.Update(tea.WindowSizeMsg{Width: 118, Height: 28})
	view := updated.(Model).View()
	for _, text := range []string{"Attack Graph", "Inspector", "selected: target-1"} {
		if !strings.Contains(view, text) {
			t.Fatalf("model graph stack missing %q:\n%s", text, view)
		}
	}
}
