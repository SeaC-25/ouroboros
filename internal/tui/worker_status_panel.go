package tui

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/progress"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/muesli/termenv"
	"github.com/ouroboros/ouroboros/internal/swarm"
)

const workerStatusBarWidth = 10

type WorkerStatusPanel struct {
	Snapshot WorkerStatusSnapshot
	width    int
	height   int
}

type WorkerStatusSnapshot struct {
	Workers []WorkerStatusRow
}

type WorkerStatusRow struct {
	Load   swarm.WorkerLoad
	Status swarm.WorkerStatus
}

type WorkerStatusSnapshotMsg struct {
	Snapshot WorkerStatusSnapshot
}

func NewWorkerStatusPanel(snapshot WorkerStatusSnapshot) WorkerStatusPanel {
	return WorkerStatusPanel{Snapshot: snapshot}
}

func (p WorkerStatusPanel) Init() tea.Cmd {
	return nil
}

func (p WorkerStatusPanel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch typed := msg.(type) {
	case tea.WindowSizeMsg:
		p.width = typed.Width
		p.height = typed.Height
	case WorkerStatusSnapshotMsg:
		p.Snapshot = typed.Snapshot
	}
	return p, nil
}

func (p WorkerStatusPanel) View() string {
	return p.Render(p.width, p.height)
}

func (p WorkerStatusPanel) Render(width int, height int) string {
	rows := p.Rows()
	if len(rows) == 0 {
		rows = []string{"No swarm workers"}
	}
	if width <= 0 && height <= 0 {
		return strings.Join(rows, "\n")
	}
	if width <= 0 {
		width = maxWorkerStatusRowWidth(rows)
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

func (p WorkerStatusPanel) Rows() []string {
	rows := make([]string, 0, len(p.Snapshot.Workers))
	for index, worker := range p.Snapshot.Workers {
		rows = append(rows, formatWorkerStatusRow(index+1, worker))
	}
	return rows
}

func formatWorkerStatusRow(index int, row WorkerStatusRow) string {
	load := clampWorkerLoad(row.Load.Load)
	percent := int(math.Round(load * 100))
	taskCount := row.Load.ActiveTasks
	if taskCount < 0 {
		taskCount = 0
	}
	return fmt.Sprintf(
		"Worker #%s [%s] %d%% %s (%d %s)",
		workerDisplayNumber(index, row.Load.WorkerID),
		renderWorkerLoadBar(load),
		percent,
		workerStatusLabel(row.Status),
		taskCount,
		taskNoun(taskCount),
	)
}

func renderWorkerLoadBar(load float64) string {
	bar := progress.New(
		progress.WithWidth(workerStatusBarWidth),
		progress.WithFillCharacters('█', '░'),
		progress.WithColorProfile(termenv.Ascii),
		progress.WithoutPercentage(),
	)
	return bar.ViewAs(clampWorkerLoad(load))
}

func clampWorkerLoad(load float64) float64 {
	if load < 0 {
		return 0
	}
	if load > 1 {
		return 1
	}
	return load
}

func workerStatusLabel(status swarm.WorkerStatus) string {
	switch status {
	case swarm.WorkerActive:
		return "active"
	case swarm.WorkerBlocked:
		return "blocked"
	case swarm.WorkerStopped:
		return "stopped"
	default:
		return "idle"
	}
}

func workerDisplayNumber(index int, workerID string) string {
	workerID = strings.TrimSpace(workerID)
	if workerID == "" {
		return strconv.Itoa(index)
	}
	matches := workerIDNumberPattern.FindStringSubmatch(workerID)
	if len(matches) == 2 {
		return matches[1]
	}
	if numericWorkerIDPattern.MatchString(workerID) {
		return workerID
	}
	return workerID
}

func taskNoun(count int) string {
	if count == 1 {
		return "task"
	}
	return "tasks"
}

func maxWorkerStatusRowWidth(rows []string) int {
	width := 0
	for _, row := range rows {
		if rowWidth := len([]rune(row)); rowWidth > width {
			width = rowWidth
		}
	}
	return width
}

var workerIDNumberPattern = regexp.MustCompile(`(?i)(?:^|[-_# ])worker[-_# ]*(\d+)$`)
var numericWorkerIDPattern = regexp.MustCompile(`^\d+$`)
