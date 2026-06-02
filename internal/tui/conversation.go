package tui

import (
	"fmt"
	"strings"

	"github.com/ouroboros/ouroboros/internal/sisyphus"
)

type MissionEvent struct {
	Kind    string
	Message string
}

type MissionConversation struct {
	GoalSummary   string
	SisyphusRound int
	CurrentPlan   []string
	AttemptResult string
	Failure       sisyphus.FailureReason
	Mutation      string
	Events        []MissionEvent
}

func DefaultMissionConversation() MissionConversation {
	return MissionConversation{
		GoalSummary:   "not set",
		SisyphusRound: 0,
		CurrentPlan:   []string{"waiting for workspace run"},
	}
}

func (c MissionConversation) Render(width int, height int) string {
	return renderPanel("Mission Conversation / Workbench", c.Rows(), width, height)
}

func (c MissionConversation) Rows() []string {
	rows := []string{
		"goal: " + valueOrDefault(c.GoalSummary, "not set"),
		fmt.Sprintf("Sisyphus round: %d", c.SisyphusRound),
	}
	for _, step := range c.CurrentPlan {
		step = strings.TrimSpace(step)
		if step == "" {
			continue
		}
		rows = append(rows, "plan: "+step)
	}
	if strings.TrimSpace(c.AttemptResult) != "" {
		rows = append(rows, "attempt: "+strings.TrimSpace(c.AttemptResult))
	}
	if strings.TrimSpace(c.Failure.Kind) != "" {
		failure := "failure: " + strings.TrimSpace(c.Failure.Kind)
		if strings.TrimSpace(c.Failure.Message) != "" {
			failure += " " + strings.TrimSpace(c.Failure.Message)
		}
		rows = append(rows, failure)
	}
	if strings.TrimSpace(c.Mutation) != "" {
		rows = append(rows, "mutation: "+strings.TrimSpace(c.Mutation))
	}
	for _, event := range c.Events {
		kind := valueOrDefault(event.Kind, "event")
		message := strings.TrimSpace(event.Message)
		if message == "" {
			continue
		}
		rows = append(rows, kind+": "+message)
	}
	return rows
}

func valueOrDefault(value string, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}
