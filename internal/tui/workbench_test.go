package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ouroboros/ouroboros/internal/sisyphus"
	"github.com/ouroboros/ouroboros/internal/swarm"
)

func TestMissionConversationRendersRuntimeState(t *testing.T) {
	conversation := MissionConversation{
		GoalSummary:   "verify admin route exposure",
		SisyphusRound: 4,
		CurrentPlan: []string{
			"enumerate authenticated routes",
			"verify admin-only response",
		},
		AttemptResult: "attempt-7 collected /admin 403 response",
		Failure: sisyphus.FailureReason{
			Kind:    sisyphus.FailureParseFailed,
			Message: "parser missed status field",
		},
		Mutation: "retry with route_observation_parser v2",
		Events: []MissionEvent{
			{Kind: "fact", Message: "target https://app.example confirmed"},
			{Kind: "intent", Message: "explore admin surface"},
		},
	}

	view := conversation.Render(52, 14)
	for _, text := range []string{
		"Mission Conversation",
		"goal: verify admin route exposure",
		"round: 4",
		"plan: enumerate authenticated routes",
		"attempt: attempt-7 collected /admin 403 response",
		"failure: parse_failed parser missed status field",
		"mutation: retry with route_observation_parser v2",
		"fact: target https://app.example confirmed",
	} {
		if !strings.Contains(view, text) {
			t.Fatalf("conversation missing %q:\n%s", text, view)
		}
	}
}

func TestModelDefaultsToMissionConversationCenterPanel(t *testing.T) {
	model := NewModel()
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 118, Height: 28})
	resized := updated.(Model)

	view := resized.View()
	if !strings.Contains(view, "Mission Conversation") {
		t.Fatalf("default view should show mission conversation:\n%s", view)
	}
	if !strings.Contains(view, "Sisyphus round") {
		t.Fatalf("default conversation should show Sisyphus round:\n%s", view)
	}
}

func TestModelSwitchesCenterPanelToSelectedAgentWorkbench(t *testing.T) {
	model := NewModel().FocusAgent(AgentWorkbench{
		AgentID:        "agent-web-1",
		Role:           swarm.RoleWebAPI,
		State:          swarm.AgentRunning,
		Goal:           "enumerate authenticated routes",
		IntentID:       "intent-42",
		AttemptID:      "attempt-9",
		OutputEvidence: []string{"evidence: /admin returned 403"},
		BlockedReason:  "waiting for verifier",
	})
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 118, Height: 28})
	resized := updated.(Model)

	view := resized.View()
	for _, text := range []string{
		"Agent Workbench",
		"agent: agent-web-1",
		"role: web-api",
		"state: running",
		"goal: enumerate authenticated routes",
		"intent: intent-42",
		"attempt: attempt-9",
		"evidence: /admin returned 403",
		"blocked: waiting for verifier",
	} {
		if !strings.Contains(view, text) {
			t.Fatalf("agent workbench missing %q:\n%s", text, view)
		}
	}
}

func TestEscReturnsFromAgentWorkbenchToMissionConversation(t *testing.T) {
	model := NewModel().FocusAgent(AgentWorkbench{AgentID: "agent-web-1", Role: swarm.RoleWebAPI})

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyEsc})
	resized := updated.(Model)

	if resized.HasAgentFocus() {
		t.Fatalf("expected esc to clear agent focus")
	}
	if !strings.Contains(resized.View(), "Mission Conversation") {
		t.Fatalf("expected mission conversation after esc:\n%s", resized.View())
	}
}
