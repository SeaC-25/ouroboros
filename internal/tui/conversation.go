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

type AIDialogMessage struct {
	Role            string
	Text            string
	Status          string
	Provider        string
	Model           string
	InputTokens     int
	OutputTokens    int
	TotalTokens     int
	MetadataOnly    bool
	PromptStored    bool
	ResponseStored  bool
	HistoryExcluded bool
	PreviewStatus   string
	PreviewAction   string
	PreviewMarker   string
	PreviewPath     string
	Error           string
}

type MissionConversation struct {
	GoalSummary      string
	SisyphusRound    int
	CurrentPlan      []string
	AttemptResult    string
	Failure          sisyphus.FailureReason
	Mutation         string
	Events           []MissionEvent
	AIDialogInput    string
	AIDialogEditing  bool
	AIDialogMessages []AIDialogMessage
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
	rows = append(rows, c.aiDialogRows()...)
	return rows
}

func (c MissionConversation) aiDialogRows() []string {
	rows := []string{"dialog: ai operator chat"}
	for _, message := range c.AIDialogMessages {
		role := strings.ToLower(strings.TrimSpace(message.Role))
		text := strings.TrimSpace(message.Text)
		switch role {
		case "user":
			if text != "" {
				rows = append(rows, "You > "+text)
			}
		default:
			status := strings.TrimSpace(message.Status)
			if strings.TrimSpace(message.Error) != "" {
				rows = append(rows, "AI > error: "+strings.TrimSpace(message.Error))
			} else if text != "" {
				rows = append(rows, "AI > "+text)
			} else if status != "" {
				rows = append(rows, "AI > "+status)
			}
			if strings.TrimSpace(message.Provider) != "" || strings.TrimSpace(message.Model) != "" || message.TotalTokens > 0 {
				rows = append(rows, fmt.Sprintf(
					"AI meta: provider=%s model=%s tokens=%d stored=%t",
					valueOrDefault(message.Provider, "unknown"),
					valueOrDefault(message.Model, "unknown"),
					message.TotalTokens,
					message.PromptStored || message.ResponseStored,
				))
			}
			if previewStatus := strings.TrimSpace(message.PreviewStatus); previewStatus != "" {
				rows = append(rows, fmt.Sprintf(
					"AI preview: status=%s action=%s marker=%s artifact_created=%t stored=false",
					previewStatus,
					valueOrDefault(message.PreviewAction, "none"),
					valueOrDefault(message.PreviewMarker, "none"),
					strings.TrimSpace(message.PreviewPath) != "",
				))
			}
		}
	}
	if c.AIDialogEditing {
		rows = append(rows, "You > "+c.AIDialogInput+"|")
	} else {
		rows = append(rows, "You > press i to chat")
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
