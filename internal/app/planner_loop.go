package app

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/ouroboros/ouroboros/internal/compiler"
	"github.com/ouroboros/ouroboros/internal/eventlog"
	"github.com/ouroboros/ouroboros/internal/sisyphus"
)

const (
	closedLoopPlannerSource  = "closed-loop-planner"
	closedLoopPlannerStageID = "closed-loop-planner-preview"
)

func runClosedLoopPlannerPreviewStage(ctx context.Context, root string, runID string, store *eventlog.SQLiteStore, options RunOptions, persistedEvents *[]eventlog.Event) error {
	if err := appendRunEvent(ctx, store, stageEvent(runID, closedLoopPlannerStageID, "Review replay state for the next closed-loop planner step", "started"), options, persistedEvents); err != nil {
		return err
	}

	snapshot, err := eventlog.BuildReplaySnapshot(*persistedEvents)
	if err != nil {
		return fmt.Errorf("build closed-loop planner preview snapshot: %w", err)
	}
	task := sisyphus.Engine{}.Step(snapshot.Blackboard)
	compilePreview, err := createClosedLoopPlannerCompilePreview(root, task, *persistedEvents)
	if err != nil {
		return err
	}
	statement := closedLoopPlannerSuggestion(task, compilePreview)
	plannerReviewPayload := map[string]any{
		"attempt_id":     "attempt-closed-loop-planner-preview",
		"kind":           "planner_review",
		"status":         "completed",
		"task_kind":      string(task.Kind),
		"intent_id":      task.IntentID,
		"reason":         task.Reason,
		"execution_mode": "metadata-only",
		"executed_tools": 0,
	}
	if compilePreview.Found {
		plannerReviewPayload["selected_action"] = string(compilePreview.Action)
		plannerReviewPayload["compile_preview_action"] = string(compilePreview.Action)
		plannerReviewPayload["compile_preview_mode"] = "metadata-only"
		plannerReviewPayload["compile_preview_review_path"] = compilePreview.Review.Path
		plannerReviewPayload["compile_preview_selection_path"] = compilePreview.Selection.Path
		plannerReviewPayload["compile_preview_nodes"] = len(compilePreview.Review.Preview.Nodes)
		plannerReviewPayload["compile_preview_warnings"] = len(compilePreview.Review.Preview.Warnings)
	}

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
			RunID:   runID,
			Type:    eventlog.EventAttempt,
			Payload: mustJSON(plannerReviewPayload),
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

type closedLoopPlannerCompilePreview struct {
	Found     bool
	Action    compiler.SemanticAction
	Review    ToolCompilePreviewReview
	Selection ToolCompilePreviewReviewSelection
}

type closedLoopPlannerIntentPayload struct {
	ID       string `json:"id"`
	IntentID string `json:"intent_id"`
	Action   string `json:"action"`
}

func createClosedLoopPlannerCompilePreview(root string, task sisyphus.Task, events []eventlog.Event) (closedLoopPlannerCompilePreview, error) {
	action, found, err := closedLoopPlannerSelectedAction(task, events)
	if err != nil {
		return closedLoopPlannerCompilePreview{}, err
	}
	if !found {
		return closedLoopPlannerCompilePreview{}, nil
	}
	review, err := CreateToolCompilePreviewReview(root, ToolCompilePreviewOptions{Action: action})
	if err != nil {
		return closedLoopPlannerCompilePreview{}, fmt.Errorf("create closed-loop planner compile preview review: %w", err)
	}
	selection, err := selectClosedLoopPlannerCompilePreviewReview(root, review, task)
	if err != nil {
		return closedLoopPlannerCompilePreview{}, fmt.Errorf("select closed-loop planner compile preview review: %w", err)
	}
	return closedLoopPlannerCompilePreview{
		Found:     true,
		Action:    action,
		Review:    review,
		Selection: selection,
	}, nil
}

func selectClosedLoopPlannerCompilePreviewReview(root string, review ToolCompilePreviewReview, task sisyphus.Task) (ToolCompilePreviewReviewSelection, error) {
	paths, err := filepath.Glob(filepath.Join(toolCompilePreviewReviewDir(root), "*.json"))
	if err != nil {
		return ToolCompilePreviewReviewSelection{}, fmt.Errorf("glob compile preview reviews: %w", err)
	}
	superseded := make([]string, 0, len(paths))
	for _, path := range paths {
		if strings.TrimSpace(path) == "" || strings.TrimSpace(path) == strings.TrimSpace(review.Path) {
			continue
		}
		superseded = append(superseded, path)
	}
	sort.Strings(superseded)
	selection := ToolCompilePreviewReviewSelection{
		Path:            toolCompilePreviewReviewSelectionPath(root),
		Status:          "current",
		Mode:            "metadata-only",
		ReviewPath:      review.Path,
		Action:          review.Preview.Action,
		ReviewNodes:     len(review.Preview.Nodes),
		ReviewWarnings:  len(review.Preview.Warnings),
		SupersededPaths: superseded,
		SelectedBy:      closedLoopPlannerSource,
		SelectedAt:      time.Now().UTC().Format(time.RFC3339),
		Reason:          fmt.Sprintf("selected by closed-loop planner from intent %s", strings.TrimSpace(task.IntentID)),
	}
	if err := writeToolCompilePreviewReviewSelection(selection); err != nil {
		return ToolCompilePreviewReviewSelection{}, err
	}
	return selection, nil
}

func closedLoopPlannerSelectedAction(task sisyphus.Task, events []eventlog.Event) (compiler.SemanticAction, bool, error) {
	intentID := strings.TrimSpace(task.IntentID)
	if intentID == "" {
		return "", false, nil
	}
	var selected compiler.SemanticAction
	found := false
	for _, event := range events {
		if event.Type != eventlog.EventIntent {
			continue
		}
		var payload closedLoopPlannerIntentPayload
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			return "", false, fmt.Errorf("decode closed-loop planner intent payload: %w", err)
		}
		id := strings.TrimSpace(payload.IntentID)
		if id == "" {
			id = strings.TrimSpace(payload.ID)
		}
		if id != intentID {
			continue
		}
		action := compiler.SemanticAction(strings.TrimSpace(payload.Action))
		if strings.TrimSpace(string(action)) == "" {
			continue
		}
		selected = action
		found = true
	}
	return selected, found, nil
}

func closedLoopPlannerSuggestion(task sisyphus.Task, compilePreview closedLoopPlannerCompilePreview) string {
	selected := string(task.Kind)
	if task.IntentID != "" {
		selected = fmt.Sprintf("%s intent %s", selected, task.IntentID)
	}
	if compilePreview.Found {
		return fmt.Sprintf(
			"closed-loop planner preview selected next task %s for operator review; planner-selected compile preview action %s produced metadata-only compile preview review with %d nodes and %d warnings; execution remains metadata-only and no second tool round was run",
			selected,
			compilePreview.Action,
			len(compilePreview.Review.Preview.Nodes),
			len(compilePreview.Review.Preview.Warnings),
		)
	}
	return fmt.Sprintf("closed-loop planner preview selected next task %s for operator review; execution remains metadata-only and no second tool round was run", selected)
}
