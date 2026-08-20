package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ouroboros/ouroboros/internal/swarm"
)

func TestWorkerStatusPanelImplementsBubbleTeaModel(t *testing.T) {
	var _ tea.Model = WorkerStatusPanel{}
}

func TestWorkerStatusPanelRendersSwarmWorkerLoadStatusAndTaskCounts(t *testing.T) {
	panel := NewWorkerStatusPanel(WorkerStatusSnapshot{
		Workers: []WorkerStatusRow{
			{
				Load:   swarm.WorkerLoad{WorkerID: "worker-1", Load: 0.5, ActiveTasks: 3},
				Status: swarm.WorkerActive,
			},
			{
				Load:   swarm.WorkerLoad{WorkerID: "worker-2", Load: 1.25, ActiveTasks: 5},
				Status: swarm.WorkerBlocked,
			},
			{
				Load:   swarm.WorkerLoad{WorkerID: "worker-3", Load: 0.3, ActiveTasks: 1},
				Status: swarm.WorkerIdle,
			},
		},
	})

	view := panel.View()
	for _, expected := range []string{
		"Worker #1 [█████░░░░░] 50% active (3 tasks)",
		"Worker #2 [██████████] 100% blocked (5 tasks)",
		"Worker #3 [███░░░░░░░] 30% idle (1 task)",
	} {
		if !strings.Contains(view, expected) {
			t.Fatalf("worker status panel missing %q:\n%s", expected, view)
		}
	}
}

func TestWorkerStatusPanelUpdatesSnapshotMessage(t *testing.T) {
	panel := NewWorkerStatusPanel(WorkerStatusSnapshot{})

	updated, cmd := panel.Update(WorkerStatusSnapshotMsg{
		Snapshot: WorkerStatusSnapshot{
			Workers: []WorkerStatusRow{
				{
					Load:   swarm.WorkerLoad{WorkerID: "worker-9", Load: 0.7, ActiveTasks: 2},
					Status: swarm.WorkerActive,
				},
			},
		},
	})
	if cmd != nil {
		t.Fatalf("worker status updates should not schedule async work")
	}
	view := updated.(WorkerStatusPanel).View()
	if !strings.Contains(view, "Worker #9 [███████░░░] 70% active (2 tasks)") {
		t.Fatalf("expected updated worker snapshot in view:\n%s", view)
	}
}

func TestWorkerStatusPanelRenderFitsWidthAndHeight(t *testing.T) {
	panel := NewWorkerStatusPanel(WorkerStatusSnapshot{
		Workers: []WorkerStatusRow{
			{Load: swarm.WorkerLoad{WorkerID: "worker-1", Load: 0.5, ActiveTasks: 3}, Status: swarm.WorkerActive},
			{Load: swarm.WorkerLoad{WorkerID: "worker-2", Load: 1, ActiveTasks: 5}, Status: swarm.WorkerActive},
		},
	})

	view := panel.Render(24, 1)
	lines := strings.Split(view, "\n")
	if len(lines) != 1 {
		t.Fatalf("expected height-limited view to one line, got %d:\n%s", len(lines), view)
	}
	if width := lipgloss.Width(lines[0]); width != 24 {
		t.Fatalf("expected fitted line width 24, got %d in %q", width, lines[0])
	}
}
