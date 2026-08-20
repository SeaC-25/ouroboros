package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ouroboros/ouroboros/internal/sisyphus"
)

func TestSisyphusStatusImplementsBubbleTeaModel(t *testing.T) {
	var _ tea.Model = SisyphusStatus{}
}

func TestSisyphusStatusRendersStrategyFailureAnalysisAndMutationHistory(t *testing.T) {
	panel := NewSisyphusStatus(SisyphusStatusSnapshot{
		CurrentStrategy: "RalphLoop",
		NextStrategy:    "Explore-Plan-Execute",
		Failures: []sisyphus.FailureReason{
			{Kind: sisyphus.FailureTimeout},
			{Kind: sisyphus.FailureTimeout},
			{Kind: sisyphus.FailureTimeout},
			{Kind: sisyphus.FailureParseFailed},
			{Kind: sisyphus.FailureParseFailed},
		},
		MutationHistory: []sisyphus.MutationHint{
			{Kind: "+param"},
			{Kind: "-header"},
			{Kind: "*payload"},
		},
	})

	view := panel.View()
	for _, expected := range []string{
		"策略: RalphLoop → Explore-Plan-Execute",
		"失败分析: 3 次超时, 2 次解析失败",
		"变异历史: [+param, -header, *payload]",
	} {
		if !strings.Contains(view, expected) {
			t.Fatalf("sisyphus status missing %q:\n%s", expected, view)
		}
	}
}

func TestSisyphusStatusUpdatesStrategySwitchInRealTime(t *testing.T) {
	panel := NewSisyphusStatus(SisyphusStatusSnapshot{CurrentStrategy: "RalphLoop"})

	updated, cmd := panel.Update(SisyphusStrategySwitchMsg{
		From: "RalphLoop",
		To:   "Explore-Plan-Execute",
	})
	if cmd != nil {
		t.Fatalf("strategy switch should not schedule async work")
	}
	view := updated.(SisyphusStatus).View()
	if !strings.Contains(view, "策略: RalphLoop → Explore-Plan-Execute") {
		t.Fatalf("expected live strategy switch in view:\n%s", view)
	}

	updated, _ = updated.Update(SisyphusStrategySwitchMsg{
		From: "Explore-Plan-Execute",
		To:   "RalphLoop",
	})
	view = updated.(SisyphusStatus).View()
	if !strings.Contains(view, "策略: Explore-Plan-Execute → RalphLoop") {
		t.Fatalf("expected refreshed strategy switch in view:\n%s", view)
	}
}
