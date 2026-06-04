package app

import (
	"context"
	"fmt"

	"github.com/ouroboros/ouroboros/internal/eventlog"
	"github.com/ouroboros/ouroboros/internal/sisyphus"
)

const (
	closedLoopPlannerSource  = "closed-loop-planner"
	closedLoopPlannerStageID = "closed-loop-planner-preview"
)

func runClosedLoopPlannerPreviewStage(ctx context.Context, runID string, store *eventlog.SQLiteStore, options RunOptions, persistedEvents *[]eventlog.Event) error {
	if err := appendRunEvent(ctx, store, stageEvent(runID, closedLoopPlannerStageID, "Review replay state for the next closed-loop planner step", "started"), options, persistedEvents); err != nil {
		return err
	}

	snapshot, err := eventlog.BuildReplaySnapshot(*persistedEvents)
	if err != nil {
		return fmt.Errorf("build closed-loop planner preview snapshot: %w", err)
	}
	task := sisyphus.Engine{}.Step(snapshot.Blackboard)
	statement := closedLoopPlannerSuggestion(task)

	events := []eventlog.Event{
		{
			RunID: runID,
			Type:  eventlog.EventAgent,
			Payload: mustJSON(map[string]any{
				"agent_id":         "closed-loop-planner",
				"role":             "planner",
				"state":            "Complete",
				"progress_percent": 100,
			}),
		},
		{
			RunID: runID,
			Type:  eventlog.EventAttempt,
			Payload: mustJSON(map[string]any{
				"attempt_id":     "attempt-closed-loop-planner-preview",
				"kind":           "planner_review",
				"status":         "completed",
				"task_kind":      string(task.Kind),
				"intent_id":      task.IntentID,
				"reason":         task.Reason,
				"execution_mode": "metadata-only",
				"executed_tools": 0,
			}),
		},
		{
			RunID: runID,
			Type:  eventlog.EventHint,
			Payload: mustJSON(map[string]any{
				"hint_id": "hint-closed-loop-planner-preview",
				"kind":    "planning",
				"message": statement,
				"source":  closedLoopPlannerSource,
			}),
		},
		{
			RunID: runID,
			Type:  eventlog.EventFact,
			Payload: mustJSON(map[string]any{
				"fact_id":    "a-closed-loop-preview",
				"kind":       "suggestion",
				"statement":  statement,
				"confidence": 1,
				"source":     closedLoopPlannerSource,
			}),
		},
	}
	for _, event := range events {
		if err := appendRunEvent(ctx, store, event, options, persistedEvents); err != nil {
			return err
		}
	}

	return appendRunEvent(ctx, store, stageEvent(runID, closedLoopPlannerStageID, "Review replay state for the next closed-loop planner step", "completed"), options, persistedEvents)
}

func closedLoopPlannerSuggestion(task sisyphus.Task) string {
	selected := string(task.Kind)
	if task.IntentID != "" {
		selected = fmt.Sprintf("%s intent %s", selected, task.IntentID)
	}
	return fmt.Sprintf("closed-loop planner preview selected next task %s for operator review; execution remains metadata-only and no second tool round was run", selected)
}
