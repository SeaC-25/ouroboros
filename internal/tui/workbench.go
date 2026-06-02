package tui

import (
	"strings"

	"github.com/ouroboros/ouroboros/internal/swarm"
)

type AgentWorkbench struct {
	AgentID        string
	Role           swarm.AgentRole
	State          swarm.AgentState
	Goal           string
	IntentID       string
	AttemptID      string
	OutputEvidence []string
	BlockedReason  string
}

func (w AgentWorkbench) Render(width int, height int) string {
	return renderPanel("Agent Workbench", w.Rows(), width, height)
}

func (w AgentWorkbench) Rows() []string {
	rows := []string{
		"agent: " + valueOrDefault(w.AgentID, "not selected"),
		"role: " + valueOrDefault(string(w.Role), "unknown"),
		"state: " + valueOrDefault(string(w.State), "unknown"),
	}
	if strings.TrimSpace(w.Goal) != "" {
		rows = append(rows, "goal: "+strings.TrimSpace(w.Goal))
	}
	if strings.TrimSpace(w.IntentID) != "" {
		rows = append(rows, "intent: "+strings.TrimSpace(w.IntentID))
	}
	if strings.TrimSpace(w.AttemptID) != "" {
		rows = append(rows, "attempt: "+strings.TrimSpace(w.AttemptID))
	}
	for _, evidence := range w.OutputEvidence {
		evidence = strings.TrimSpace(evidence)
		if evidence == "" {
			continue
		}
		if strings.HasPrefix(strings.ToLower(evidence), "evidence:") {
			rows = append(rows, evidence)
			continue
		}
		rows = append(rows, "evidence: "+evidence)
	}
	if strings.TrimSpace(w.BlockedReason) != "" {
		rows = append(rows, "blocked: "+strings.TrimSpace(w.BlockedReason))
	}
	return rows
}
