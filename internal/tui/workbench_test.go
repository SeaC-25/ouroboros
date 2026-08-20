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

func TestMissionConversationAIDialogPreviewOmitsArtifactPath(t *testing.T) {
	artifactPath := "/tmp/ouro/workspace/runtime/model-tool-plan-previews/preview.json"
	conversation := MissionConversation{
		AIDialogMessages: []AIDialogMessage{
			{
				Role:          "assistant",
				Status:        "completed",
				Text:          "Prepared a metadata-only preview for operator review.",
				PreviewStatus: "created",
				PreviewAction: "enumerate_paths",
				PreviewMarker: "semantic_action: enumerate_paths",
				PreviewPath:   artifactPath,
			},
		},
	}

	rows := strings.Join(conversation.Rows(), "\n")
	want := "AI preview: status=created action=enumerate_paths marker=semantic_action: enumerate_paths artifact_created=true stored=false"
	if !strings.Contains(rows, want) {
		t.Fatalf("expected AI dialog preview summary %q:\n%s", want, rows)
	}
	for _, forbidden := range []string{
		artifactPath,
		"path=",
		"/tmp/ouro",
		"workspace/runtime/model-tool-plan-previews",
	} {
		if strings.Contains(rows, forbidden) {
			t.Fatalf("AI dialog preview summary leaked %q:\n%s", forbidden, rows)
		}
	}
}

func TestMissionConversationAIDialogComposerRendersAsOperatorInput(t *testing.T) {
	conversation := MissionConversation{
		AIDialogEditing: true,
		AIDialogInput:   "plan next step",
	}

	rows := strings.Join(conversation.Rows(), "\n")
	if !strings.Contains(rows, "You > plan next step|") {
		t.Fatalf("expected active composer to render as operator input:\n%s", rows)
	}
	if strings.Contains(rows, "AI > plan next step") {
		t.Fatalf("active composer should not render as an AI message:\n%s", rows)
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

func TestModelAIDialogInputSubmitsPrompt(t *testing.T) {
	var capturedPrompt string
	model := NewModel().WithAIDialogSubmitter(func(request AIDialogRequest) (AIDialogResult, error) {
		capturedPrompt = request.Prompt
		return AIDialogResult{
			Status:         "completed",
			Provider:       "local-openai",
			Model:          "gpt-test",
			Text:           "Prioritize scoped recon and wait for operator approval.",
			InputTokens:    11,
			OutputTokens:   7,
			TotalTokens:    18,
			MetadataOnly:   true,
			PromptStored:   false,
			ResponseStored: false,
		}, nil
	})
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 140, Height: 32})
	model = updated.(Model)

	editing, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}})
	if cmd != nil {
		t.Fatalf("start input should be synchronous")
	}
	model = editing.(Model)
	for _, ch := range "plan next step" {
		typed, typeCmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}})
		if typeCmd != nil {
			t.Fatalf("typing should be synchronous")
		}
		model = typed.(Model)
	}
	if !strings.Contains(model.View(), "You > plan next step|") {
		t.Fatalf("expected active AI input to render typed prompt:\n%s", model.View())
	}

	submitting, submitCmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if submitCmd == nil {
		t.Fatalf("enter should submit the AI dialog prompt")
	}
	model = submitting.(Model)
	if capturedPrompt != "" {
		t.Fatalf("submit command should run asynchronously, captured prompt early: %q", capturedPrompt)
	}
	completed, _ := model.Update(submitCmd())
	model = completed.(Model)

	if capturedPrompt != "plan next step" {
		t.Fatalf("unexpected submitted prompt %q", capturedPrompt)
	}
	view := model.View()
	for _, want := range []string{
		"You > plan next step",
		"AI > Prioritize scoped recon and wait for operator approval.",
		"provider=local-openai model=gpt-test tokens=18",
		"stored=false",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected AI dialog view to contain %q:\n%s", want, view)
		}
	}
}

func TestModelAIDialogComposerReopensAfterResult(t *testing.T) {
	model := NewModel().WithAIDialogSubmitter(func(request AIDialogRequest) (AIDialogResult, error) {
		return AIDialogResult{
			Status:         "completed",
			Provider:       "local-openai",
			Model:          "gpt-test",
			Text:           "Check the next gate.",
			MetadataOnly:   true,
			PromptStored:   false,
			ResponseStored: false,
		}, nil
	})
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 140, Height: 32})
	model = updated.(Model).StartAIDialogInput()
	for _, ch := range "first" {
		typed, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}})
		model = typed.(Model)
	}
	submitting, submitCmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if submitCmd == nil {
		t.Fatalf("enter should submit the AI dialog prompt")
	}
	completed, _ := submitting.Update(submitCmd())
	model = completed.(Model)

	if !strings.Contains(model.View(), "You > |") {
		t.Fatalf("expected composer to reopen after AI result:\n%s", model.View())
	}
	typed, typeCmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	if typeCmd != nil {
		t.Fatalf("typing follow-up should be synchronous")
	}
	model = typed.(Model)
	if !strings.Contains(model.View(), "You > n|") {
		t.Fatalf("expected follow-up typing without pressing i:\n%s", model.View())
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
