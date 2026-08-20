package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ouroboros/ouroboros/internal/goal"
)

func TestGoalPanelImplementsBubbleTeaModel(t *testing.T) {
	var _ tea.Model = GoalPanel{}
}

func TestGoalPanelRendersPriorityQueueRows(t *testing.T) {
	queue := goal.NewPriorityQueue()
	queue.Enqueue(goal.GoalNode{
		ID:           "admin-scan",
		Title:        "Scan admin panel",
		Priority:     goal.PriorityCritical,
		Dependencies: []goal.GoalID{"auth", "routes", "session"},
	})
	queue.Enqueue(goal.GoalNode{
		ID:           "subdomain-enum",
		Title:        "Enumerate subdomains",
		Priority:     goal.PriorityHigh,
		Dependencies: []goal.GoalID{"scope"},
	})
	queue.Enqueue(goal.GoalNode{
		ID:       "ssl-check",
		Title:    "Check SSL config",
		Priority: goal.PriorityMedium,
	})
	queue.Enqueue(goal.GoalNode{
		ID:       "archive-check",
		Title:    "Review archived endpoints",
		Priority: goal.PriorityLow,
	})

	panel := NewGoalPanel(queue)
	view := panel.View()

	expectedOrder := []string{
		"Critical: Scan admin panel (3 dependencies)",
		"High: Enumerate subdomains (1 dependency)",
		"Medium: Check SSL config",
		"Low: Review archived endpoints",
	}
	previous := -1
	for _, expected := range expectedOrder {
		index := strings.Index(view, expected)
		if index < 0 {
			t.Fatalf("goal panel missing %q:\n%s", expected, view)
		}
		if index <= previous {
			t.Fatalf("goal panel rendered %q out of priority order:\n%s", expected, view)
		}
		previous = index
	}

	if queue.Len() != 4 {
		t.Fatalf("goal panel should not consume queue entries, got len %d", queue.Len())
	}
}
