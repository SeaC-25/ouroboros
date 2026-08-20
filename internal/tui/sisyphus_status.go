package tui

import (
	"fmt"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ouroboros/ouroboros/internal/sisyphus"
)

type SisyphusStatus struct {
	Snapshot SisyphusStatusSnapshot
	width    int
	height   int
}

type SisyphusStatusSnapshot struct {
	CurrentStrategy string
	NextStrategy    string
	Failures        []sisyphus.FailureReason
	MutationHistory []sisyphus.MutationHint
}

type SisyphusStatusMsg struct {
	Snapshot SisyphusStatusSnapshot
}

type SisyphusStrategySwitchMsg struct {
	From string
	To   string
}

func NewSisyphusStatus(snapshot SisyphusStatusSnapshot) SisyphusStatus {
	return SisyphusStatus{Snapshot: snapshot}
}

func (s SisyphusStatus) Init() tea.Cmd {
	return nil
}

func (s SisyphusStatus) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch typed := msg.(type) {
	case tea.WindowSizeMsg:
		s.width = typed.Width
		s.height = typed.Height
	case SisyphusStatusMsg:
		s.Snapshot = typed.Snapshot
	case SisyphusStrategySwitchMsg:
		s.Snapshot.CurrentStrategy = strings.TrimSpace(typed.From)
		s.Snapshot.NextStrategy = strings.TrimSpace(typed.To)
	}
	return s, nil
}

func (s SisyphusStatus) View() string {
	return s.Render(s.width, s.height)
}

func (s SisyphusStatus) Render(width int, height int) string {
	rows := s.Rows()
	if width <= 0 && height <= 0 {
		return strings.Join(rows, "\n")
	}
	if width <= 0 {
		width = maxSisyphusStatusRowWidth(rows)
	}
	if height <= 0 {
		height = len(rows)
	}
	lines := make([]string, 0, height)
	for _, row := range rows {
		if len(lines) >= height {
			break
		}
		lines = append(lines, fitWidth(row, width))
	}
	for len(lines) < height {
		lines = append(lines, strings.Repeat(" ", width))
	}
	return strings.Join(lines, "\n")
}

func (s SisyphusStatus) Rows() []string {
	return []string{
		"策略: " + formatSisyphusStrategy(s.Snapshot.CurrentStrategy, s.Snapshot.NextStrategy),
		sisyphusFailureStyle(s.Snapshot.Failures).Render("失败分析: " + formatSisyphusFailures(s.Snapshot.Failures)),
		"变异历史: " + formatSisyphusMutations(s.Snapshot.MutationHistory),
	}
}

func formatSisyphusStrategy(current string, next string) string {
	current = valueOrDefault(current, "unknown")
	next = strings.TrimSpace(next)
	if next == "" || next == current {
		return current
	}
	return current + " → " + next
}

func formatSisyphusFailures(failures []sisyphus.FailureReason) string {
	counts := make(map[string]int)
	for _, failure := range failures {
		kind := strings.TrimSpace(failure.Kind)
		if kind == "" {
			kind = "unknown"
		}
		counts[kind]++
	}
	if len(counts) == 0 {
		return "暂无失败"
	}

	kinds := make([]string, 0, len(counts))
	for kind := range counts {
		kinds = append(kinds, kind)
	}
	sort.SliceStable(kinds, func(i int, j int) bool {
		left := sisyphusFailureOrder(kinds[i])
		right := sisyphusFailureOrder(kinds[j])
		if left != right {
			return left < right
		}
		return kinds[i] < kinds[j]
	})

	parts := make([]string, 0, len(kinds))
	for _, kind := range kinds {
		parts = append(parts, fmt.Sprintf("%d 次%s", counts[kind], sisyphusFailureLabel(kind)))
	}
	return strings.Join(parts, ", ")
}

func formatSisyphusMutations(mutations []sisyphus.MutationHint) string {
	if len(mutations) == 0 {
		return "[]"
	}
	labels := make([]string, 0, len(mutations))
	for _, mutation := range mutations {
		labels = append(labels, formatSisyphusMutation(mutation))
	}
	return "[" + strings.Join(labels, ", ") + "]"
}

func formatSisyphusMutation(mutation sisyphus.MutationHint) string {
	kind := strings.TrimSpace(mutation.Kind)
	message := strings.TrimSpace(mutation.Message)
	if kind == "" {
		return valueOrDefault(message, "mutation")
	}
	switch {
	case strings.HasPrefix(kind, "+"), strings.HasPrefix(kind, "-"), strings.HasPrefix(kind, "*"):
		return kind
	case strings.HasPrefix(kind, "add_"):
		return "+" + strings.TrimPrefix(kind, "add_")
	case strings.HasPrefix(kind, "remove_"):
		return "-" + strings.TrimPrefix(kind, "remove_")
	case strings.HasPrefix(kind, "delete_"):
		return "-" + strings.TrimPrefix(kind, "delete_")
	case strings.HasPrefix(kind, "mutate_"):
		return "*" + strings.TrimPrefix(kind, "mutate_")
	case strings.HasPrefix(kind, "replace_"):
		return "*" + strings.TrimPrefix(kind, "replace_")
	default:
		return kind
	}
}

func sisyphusFailureLabel(kind string) string {
	switch kind {
	case sisyphus.FailureTimeout:
		return "超时"
	case sisyphus.FailureParseFailed:
		return "解析失败"
	case sisyphus.FailureWorkerExit:
		return "Worker 退出"
	default:
		return kind
	}
}

func sisyphusFailureOrder(kind string) int {
	switch kind {
	case sisyphus.FailureTimeout:
		return 0
	case sisyphus.FailureParseFailed:
		return 1
	case sisyphus.FailureWorkerExit:
		return 2
	default:
		return 100
	}
}

func sisyphusFailureStyle(failures []sisyphus.FailureReason) lipgloss.Style {
	if len(failures) == 0 {
		return lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	}
	return lipgloss.NewStyle().Foreground(lipgloss.Color("220")).Bold(true)
}

func maxSisyphusStatusRowWidth(rows []string) int {
	width := 0
	for _, row := range rows {
		if rowWidth := lipgloss.Width(row); rowWidth > width {
			width = rowWidth
		}
	}
	return width
}
