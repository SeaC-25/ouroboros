package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ouroboros/ouroboros/internal/goal"
)

type GoalPanel struct {
	Queue  *goal.PriorityQueue
	width  int
	height int
}

type GoalQueueMsg struct {
	Queue *goal.PriorityQueue
}

func NewGoalPanel(queue *goal.PriorityQueue) GoalPanel {
	return GoalPanel{Queue: queue}
}

func (p GoalPanel) Init() tea.Cmd {
	return nil
}

func (p GoalPanel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch typed := msg.(type) {
	case tea.WindowSizeMsg:
		p.width = typed.Width
		p.height = typed.Height
	case GoalQueueMsg:
		p.Queue = typed.Queue
	}
	return p, nil
}

func (p GoalPanel) View() string {
	return p.Render(p.width, p.height)
}

func (p GoalPanel) Render(width int, height int) string {
	rows := p.Rows()
	if len(rows) == 0 {
		rows = []goalPanelRow{{Text: "No queued goals", Priority: goal.PriorityLow}}
	}

	if height > 0 && len(rows) > height {
		rows = rows[:height]
	}

	lines := make([]string, 0, len(rows))
	for _, row := range rows {
		text := row.Text
		if width > 0 {
			text = fitWidth(text, width)
		}
		lines = append(lines, goalPriorityStyle(row.Priority).Render(text))
	}
	for height > 0 && len(lines) < height {
		blank := ""
		if width > 0 {
			blank = strings.Repeat(" ", width)
		}
		lines = append(lines, blank)
	}
	return strings.Join(lines, "\n")
}

func (p GoalPanel) Rows() []goalPanelRow {
	if p.Queue == nil {
		return nil
	}
	nodes := p.Queue.Snapshot()
	rows := make([]goalPanelRow, 0, len(nodes))
	for _, node := range nodes {
		rows = append(rows, goalPanelRow{
			Text:     formatGoalPanelRow(node),
			Priority: node.Priority,
		})
	}
	return rows
}

type goalPanelRow struct {
	Text     string
	Priority goal.Priority
}

func formatGoalPanelRow(node goal.GoalNode) string {
	icon, label := goalPriorityDisplay(node.Priority)
	title := strings.TrimSpace(node.Title)
	if title == "" {
		title = strings.TrimSpace(string(node.ID))
	}
	if title == "" {
		title = "(untitled goal)"
	}

	row := fmt.Sprintf("%s %s: %s", icon, label, title)
	if count := len(node.Dependencies); count > 0 {
		row += fmt.Sprintf(" (%d %s)", count, dependencyNoun(count))
	}
	return row
}

func goalPriorityDisplay(priority goal.Priority) (string, string) {
	switch priority {
	case goal.PriorityCritical:
		return "🔴", "Critical"
	case goal.PriorityHigh:
		return "🟡", "High"
	case goal.PriorityLow:
		return "●", "Low"
	default:
		return "⚪", "Medium"
	}
}

func goalPriorityStyle(priority goal.Priority) lipgloss.Style {
	switch priority {
	case goal.PriorityCritical:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true)
	case goal.PriorityHigh:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("220")).Bold(true)
	case goal.PriorityLow:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	default:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("15"))
	}
}

func dependencyNoun(count int) string {
	if count == 1 {
		return "dependency"
	}
	return "dependencies"
}
