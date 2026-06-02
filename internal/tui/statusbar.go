package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

type Status struct {
	Model           string
	ContextPercent  int
	Agents          int
	BreadthPercent  int
	DepthChains     int
	Tools           string
	InstallRequests string
	Cost            string
}

func DefaultStatus() Status {
	return Status{
		Model: "not-set",
		Tools: "unknown",
		Cost:  "$0.00",
	}
}

func RenderStatusBar(width int, status Status) string {
	if width <= 0 {
		return ""
	}
	if status.Model == "" {
		status.Model = "not-set"
	}
	if status.Tools == "" {
		status.Tools = "unknown"
	}
	if status.Cost == "" {
		status.Cost = "$0.00"
	}

	requests := strings.TrimSpace(status.InstallRequests)
	line := fmt.Sprintf(
		"model:%s | ctx:%d%% | agents:%d | breadth:%d%% | depth:%d chains | tools:%s%s | cost:%s",
		status.Model,
		status.ContextPercent,
		status.Agents,
		status.BreadthPercent,
		status.DepthChains,
		status.Tools,
		statusInstallRequestSegment(requests),
		status.Cost,
	)
	if lipgloss.Width(line) > width {
		line = fmt.Sprintf(
			"model:%s | ctx:%d%% | agents:%d | tools:%s%s | cost:%s",
			status.Model,
			status.ContextPercent,
			status.Agents,
			status.Tools,
			statusInstallRequestSegment(requests),
			status.Cost,
		)
	}
	return fitWidth(line, width)
}

func statusInstallRequestSegment(requests string) string {
	if requests == "" {
		return ""
	}
	return " | requests:" + requests
}

func fitWidth(value string, width int) string {
	if width <= 0 {
		return ""
	}
	for lipgloss.Width(value) > width {
		runes := []rune(value)
		if len(runes) == 0 {
			break
		}
		value = string(runes[:len(runes)-1])
	}
	if padding := width - lipgloss.Width(value); padding > 0 {
		value += strings.Repeat(" ", padding)
	}
	return value
}
