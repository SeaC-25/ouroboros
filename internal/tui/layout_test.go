package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func TestLayoutAllocatesThreeColumnsAndStatusBar(t *testing.T) {
	layout := NewLayout(120, 32)

	if !layout.Ready {
		t.Fatalf("expected layout to be ready: %#v", layout)
	}
	if layout.StatusBarHeight != 1 {
		t.Fatalf("expected one-line status bar, got %d", layout.StatusBarHeight)
	}
	if layout.ContentHeight != 31 {
		t.Fatalf("expected content height 31, got %d", layout.ContentHeight)
	}
	if layout.Workspace.Width < MinWorkspaceWidth {
		t.Fatalf("workspace below minimum: %d", layout.Workspace.Width)
	}
	if layout.Workbench.Width < MinWorkbenchWidth {
		t.Fatalf("workbench below minimum: %d", layout.Workbench.Width)
	}
	if layout.GraphStack.Width < MinGraphStackWidth {
		t.Fatalf("graph stack below minimum: %d", layout.GraphStack.Width)
	}
	total := layout.Workspace.Width + layout.Workbench.Width + layout.GraphStack.Width
	if total != layout.Width {
		t.Fatalf("expected columns to fill width %d, got %d", layout.Width, total)
	}
	if layout.Workspace.X != 0 || layout.Workbench.X != layout.Workspace.Width {
		t.Fatalf("unexpected left/center positions: %#v", layout)
	}
	if layout.GraphStack.X != layout.Workspace.Width+layout.Workbench.Width {
		t.Fatalf("unexpected graph stack position: %#v", layout)
	}
}

func TestLayoutReportsTooSmallViewport(t *testing.T) {
	narrow := NewLayout(MinTotalWidth-1, 24)
	if narrow.Ready {
		t.Fatalf("expected narrow viewport to be not ready")
	}
	if !strings.Contains(narrow.Message, "too small") {
		t.Fatalf("expected too-small message, got %q", narrow.Message)
	}

	short := NewLayout(100, MinTotalHeight-1)
	if short.Ready {
		t.Fatalf("expected short viewport to be not ready")
	}
}

func TestStatusBarFitsViewportWidth(t *testing.T) {
	line := RenderStatusBar(72, Status{
		Model:          "gpt-5",
		ContextPercent: 74,
		Agents:         7,
		BreadthPercent: 62,
		DepthChains:    3,
		Tools:          "healthy",
		Cost:           "$1.24",
	})

	if lipgloss.Width(line) != 72 {
		t.Fatalf("expected status bar width 72, got %d in %q", lipgloss.Width(line), line)
	}
	if !strings.Contains(line, "model:gpt-5") || !strings.Contains(line, "tools:healthy") {
		t.Fatalf("status bar missing expected runtime fields: %q", line)
	}

	truncated := RenderStatusBar(24, Status{Model: "gpt-5", Tools: "healthy"})
	if lipgloss.Width(truncated) != 24 {
		t.Fatalf("expected truncated status width 24, got %d in %q", lipgloss.Width(truncated), truncated)
	}
}

func TestModelHandlesWindowResizeAndRendersShell(t *testing.T) {
	model := NewModel()
	updated, cmd := model.Update(tea.WindowSizeMsg{Width: 110, Height: 28})
	if cmd != nil {
		t.Fatalf("window resize should not emit command")
	}
	resized, ok := updated.(Model)
	if !ok {
		t.Fatalf("expected Model after update, got %T", updated)
	}
	if !resized.layout.Ready {
		t.Fatalf("expected resized layout to be ready")
	}

	view := resized.View()
	for _, label := range []string{"Workspace", "Mission Conversation / Workbench", "Attack Graph", "model:"} {
		if !strings.Contains(view, label) {
			t.Fatalf("view missing %q:\n%s", label, view)
		}
	}
}
