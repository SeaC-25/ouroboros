package app

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"math"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/ouroboros/ouroboros/internal/compiler"
	"github.com/ouroboros/ouroboros/internal/eventlog"
	"github.com/ouroboros/ouroboros/internal/sisyphus"
	"github.com/ouroboros/ouroboros/internal/toolchain"
)

const (
	closedLoopPlannerSource            = "closed-loop-planner"
	closedLoopPlannerStageID           = "closed-loop-planner-preview"
	closedLoopPlannerGenerationStageID = "closed-loop-planner-generation"
)

func validateRunProviderGenerationOptions(options RunOptions) error {
	if !options.EnableProviderGeneration {
		return nil
	}
	if strings.TrimSpace(options.ProviderGenerationPrompt) == "" {
		return fmt.Errorf("run provider generation prompt is required")
	}
	if options.ProviderGenerationMaxTokens < 0 {
		return fmt.Errorf("run provider generation max_tokens must be non-negative")
	}
	if options.ProviderGenerationMaxEstimatedCostUSD != nil {
		maxCost := *options.ProviderGenerationMaxEstimatedCostUSD
		if math.IsNaN(maxCost) || math.IsInf(maxCost, 0) {
			return fmt.Errorf("run provider generation max_estimated_cost_usd must be finite")
		}
		if maxCost < 0 {
			return fmt.Errorf("run provider generation max_estimated_cost_usd must be non-negative")
		}
	}
	return nil
}

func runClosedLoopPlannerPreviewStage(ctx context.Context, root string, runID string, provider ProviderRuntime, store *eventlog.SQLiteStore, options RunOptions, persistedEvents *[]eventlog.Event) error {
	if err := appendRunEvent(ctx, store, stageEvent(runID, closedLoopPlannerStageID, "Review replay state for the next closed-loop planner step", "started"), options, persistedEvents); err != nil {
		return err
	}

	snapshot, err := eventlog.BuildReplaySnapshot(*persistedEvents)
	if err != nil {
		return fmt.Errorf("build closed-loop planner preview snapshot: %w", err)
	}
	// Merge the durable observation journal (cross-run evidence the AI chat
	// layer persists): the reasoning task must see every unprocessed
	// observation, not only ones recorded inside this run.
	snapshot.Blackboard = mergeWorkspaceObservations(root, snapshot.Blackboard)
	// Observation-driven stepping: unprocessed observations (raw evidence
	// the model has not yet converted into concluded facts) take priority
	// over hypothesis verification — the judgement authority stays with the
	// model side. Falls back to the legacy intent loop only when no
	// observation backlog exists.
	task := sisyphus.Engine{}.StepFromObservations(snapshot.Blackboard)
	// Index intent→action once per stage; the compile preview lookup reads
	// the map instead of rescanning every intent event.
	intentActions, err := closedLoopPlannerIntentActionIndex(*persistedEvents)
	if err != nil {
		return err
	}
	compilePreview, err := createClosedLoopPlannerCompilePreview(root, task, *persistedEvents, intentActions, options.toolchainManifests)
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
		closedLoopPlannerModelUsagePreviewEvent(runID, provider),
		closedLoopPlannerModelUsagePreviewSuggestionEvent(runID, provider),
	}
	for _, event := range events {
		if err := appendRunEvent(ctx, store, event, options, persistedEvents); err != nil {
			return err
		}
	}
	if options.EnableProviderGeneration {
		// The reasoning task carries the unprocessed observation backlog:
		// the model analyzes this exact evidence (not a generic summary);
		// its conclusions close the loop when the resulting facts record
		// the source-observation backlink.
		obsContext := ""
		if pending := sisyphus.UnprocessedObservations(snapshot.Blackboard); len(pending) > 0 {
			// Caps mirror the chat-side context window: latest 5 entries,
			// 800 runes each, and a total byte budget — the backlog must
			// never grow the prompt without bound. Truncation is rune-safe
			// (byte slicing would cut UTF-8 mid-character and could split
			// the JSON payload mid-token). Each entry carries its ID so the
			// model can cite it in a BLACKBOARD conclusion (back-link).
			const maxEntries = 5
			const maxRunesPerEntry = 800
			const maxTotalBytes = 8 << 10
			var builder strings.Builder
			builder.WriteString("\n\n当前待分析原始观测（工具回传证据，未经任何判定；请逐一分析并给出结论，结论中将 obs-…/fact-… 观测ID写进 ACTION:BLACKBOARD 语句即可回链证据）：\n")
			start := 0
			if len(pending) > maxEntries {
				start = len(pending) - maxEntries
			}
			for i, obs := range pending[start:] {
				preview := []rune(obs.Payload)
				if len(preview) > maxRunesPerEntry {
					preview = preview[:maxRunesPerEntry]
				}
				entry := fmt.Sprintf("%d. id=%s [%s] target=%s\n%s\n", start+i+1, obs.ID, obs.Kind, obs.Target, string(preview))
				if builder.Len()+len(entry) > maxTotalBytes {
					builder.WriteString("（观测积压过大，已按预算截断；其余观测可在结论后再次运行查看）\n")
					break
				}
				builder.WriteString(entry)
			}
			obsContext = builder.String()
		}
		if err := runClosedLoopPlannerProviderGenerationStage(ctx, root, runID, store, options, persistedEvents, obsContext); err != nil {
			return err
		}
	}

	return appendRunEvent(ctx, store, stageEvent(runID, closedLoopPlannerStageID, "Review replay state for the next closed-loop planner step", "completed"), options, persistedEvents)
}

func runClosedLoopPlannerProviderGenerationStage(ctx context.Context, root string, runID string, store *eventlog.SQLiteStore, options RunOptions, persistedEvents *[]eventlog.Event, observationContext string) error {
	if err := appendRunEvent(ctx, store, stageEvent(runID, closedLoopPlannerGenerationStageID, "Generate closed-loop planner guidance with the configured provider", "started"), options, persistedEvents); err != nil {
		return err
	}
	result, err := GenerateProviderMessage(ctx, root, ProviderGenerationRequest{
		SystemPrompt:         options.ProviderGenerationSystemPrompt,
		UserPrompt:           options.ProviderGenerationPrompt + observationContext,
		MaxTokens:            options.ProviderGenerationMaxTokens,
		RequiredCapabilities: options.ProviderGenerationRequiredCapabilities,
		Stream:               options.ProviderGenerationStream,
	})
	if err != nil {
		return failClosedLoopPlannerProviderGenerationStage(ctx, runID, store, options, persistedEvents, err)
	}
	budget, err := EnforceProviderGenerationBudget(result, ProviderGenerationBudgetOptions{
		MaxEstimatedCostUSD: options.ProviderGenerationMaxEstimatedCostUSD,
	})
	if err != nil {
		return failClosedLoopPlannerProviderGenerationStage(ctx, runID, store, options, persistedEvents, err)
	}
	ledgerWrite := false
	if options.ProviderGenerationRecordUsage {
		if _, err := RecordProviderGenerationUsage(root, result, ProviderUsageRecordOptions{
			MaxEstimatedCostUSD: options.ProviderGenerationMaxEstimatedCostUSD,
		}); err != nil {
			return failClosedLoopPlannerProviderGenerationStage(ctx, runID, store, options, persistedEvents, err)
		}
		ledgerWrite = true
	}
	artifact, err := RecordModelGenerationArtifact(root, runID, result, budget, ledgerWrite, ModelGenerationArtifactOptions{})
	if err != nil {
		return failClosedLoopPlannerProviderGenerationStage(ctx, runID, store, options, persistedEvents, err)
	}
	if err := appendRunEvent(ctx, store, closedLoopPlannerModelGenerationEvent(runID, result, budget, ledgerWrite, artifact.Path), options, persistedEvents); err != nil {
		return err
	}
	if options.ProviderGenerationToolPlanPreview {
		if err := runClosedLoopPlannerModelToolPlanPreview(ctx, root, runID, store, options, persistedEvents, result, artifact.Path); err != nil {
			return failClosedLoopPlannerProviderGenerationStage(ctx, runID, store, options, persistedEvents, err)
		}
	}
	if err := appendRunEvent(ctx, store, closedLoopPlannerModelGenerationSuggestionEvent(runID, result, ledgerWrite), options, persistedEvents); err != nil {
		return err
	}
	return appendRunEvent(ctx, store, stageEvent(runID, closedLoopPlannerGenerationStageID, "Generate closed-loop planner guidance with the configured provider", "completed"), options, persistedEvents)
}

func runClosedLoopPlannerModelToolPlanPreview(ctx context.Context, root string, runID string, store *eventlog.SQLiteStore, options RunOptions, persistedEvents *[]eventlog.Event, result ProviderGenerationResult, generationArtifactPath string) error {
	extraction := ExtractModelToolPlanPreview(result.Text)
	if !extraction.Valid {
		return appendRunEvent(ctx, store, closedLoopPlannerModelToolPlanPreviewEvent(runID, result, extraction, ModelToolPlanPreviewArtifact{}), options, persistedEvents)
	}
	review, err := CreateToolCompilePreviewReview(root, ToolCompilePreviewOptions{Action: extraction.Action, Manifests: options.toolchainManifests})
	if err != nil {
		return fmt.Errorf("create model tool-plan compile preview review: %w", err)
	}
	artifact, err := RecordModelToolPlanPreviewArtifact(root, runID, result, generationArtifactPath, extraction, review, ModelToolPlanPreviewArtifactOptions{})
	if err != nil {
		return err
	}
	return appendRunEvent(ctx, store, closedLoopPlannerModelToolPlanPreviewEvent(runID, result, extraction, artifact), options, persistedEvents)
}

func failClosedLoopPlannerProviderGenerationStage(ctx context.Context, runID string, store *eventlog.SQLiteStore, options RunOptions, persistedEvents *[]eventlog.Event, cause error) error {
	if appendErr := appendRunEvent(ctx, store, stageEventWithReason(runID, closedLoopPlannerGenerationStageID, "Generate closed-loop planner guidance with the configured provider", "failed", cause.Error()), options, persistedEvents); appendErr != nil {
		return fmt.Errorf("record failed closed-loop planner provider generation: %w", appendErr)
	}
	return cause
}

func closedLoopPlannerModelUsagePreviewEvent(runID string, provider ProviderRuntime) eventlog.Event {
	return eventlog.Event{
		RunID: runID,
		Type:  eventlog.EventVerification,
		Payload: mustJSON(map[string]any{
			"kind":                  "model_usage_preview",
			"source":                closedLoopPlannerStageID,
			"agent_id":              "closed-loop-planner",
			"role":                  "planner",
			"status":                "recorded",
			"execution_mode":        "metadata-only",
			"budget_status":         "not_enforced",
			"ledger_write":          false,
			"routing_strategy":      strings.TrimSpace(provider.RoutingStrategy),
			"provider_attempts":     provider.ProviderAttemptCount,
			"provider_failover":     provider.ProviderFailover,
			"selected_provider":     strings.TrimSpace(provider.SelectedProviderName),
			"failed_providers":      append([]string{}, provider.FailedProviderNames...),
			"required_capabilities": append([]string{}, provider.RequiredCapabilities...),
			"capability_match":      provider.CapabilityMatch,
			"skipped_providers":     appendProviderSkippedProviders(nil, provider.SkippedProviders...),
			"model_usage": map[string]any{
				"run_id":        runID,
				"agent_id":      "closed-loop-planner",
				"role":          "planner",
				"provider":      strings.TrimSpace(provider.ProviderName),
				"model":         strings.TrimSpace(provider.Model),
				"input_tokens":  0,
				"output_tokens": 0,
				"cost":          0,
			},
		}),
	}
}

func closedLoopPlannerModelUsagePreviewSuggestionEvent(runID string, provider ProviderRuntime) eventlog.Event {
	providerName := strings.TrimSpace(provider.ProviderName)
	if providerName == "" {
		providerName = "unknown"
	}
	model := strings.TrimSpace(provider.Model)
	if model == "" {
		model = "not-set"
	}
	return eventlog.Event{
		RunID: runID,
		Type:  eventlog.EventFact,
		Payload: mustJSON(map[string]any{
			"fact_id":    "a-model-usage-accounting-preview",
			"kind":       "suggestion",
			"statement":  fmt.Sprintf("model usage accounting preview recorded metadata-only usage for provider %s model %s with 0 input and 0 output tokens; no provider generation call or provider usage ledger write was performed", providerName, model),
			"confidence": 1,
			"source":     "model-usage-accounting",
		}),
	}
}

func closedLoopPlannerModelGenerationEvent(runID string, result ProviderGenerationResult, budget ProviderGenerationBudgetDecision, ledgerWrite bool, reviewArtifactPath string) eventlog.Event {
	budgetStatus := "not_enforced"
	if budget.Enforced {
		budgetStatus = budget.Status
	}
	return eventlog.Event{
		RunID: runID,
		Type:  eventlog.EventVerification,
		Payload: mustJSON(map[string]any{
			"kind":                     "model_generation",
			"source":                   closedLoopPlannerGenerationStageID,
			"agent_id":                 "closed-loop-planner",
			"role":                     "planner",
			"status":                   "completed",
			"execution_mode":           "provider-generation",
			"budget_status":            budgetStatus,
			"ledger_write":             ledgerWrite,
			"review_artifact_path":     strings.TrimSpace(reviewArtifactPath),
			"assistant_text_persisted": false,
			"routing_strategy":         strings.TrimSpace(result.RoutingStrategy),
			"provider_attempts":        result.ProviderAttemptCount,
			"provider_failover":        result.ProviderFailover,
			"selected_provider":        strings.TrimSpace(result.SelectedProviderName),
			"failed_providers":         append([]string{}, result.FailedProviderNames...),
			"required_capabilities":    append([]string{}, result.RequiredCapabilities...),
			"capability_match":         result.CapabilityMatch,
			"skipped_providers":        appendProviderSkippedProviders(nil, result.SkippedProviders...),
			"streaming":                result.Streaming,
			"stream_chunk_count":       result.StreamChunkCount,
			"stream_completed":         result.StreamCompleted,
			"stream_digest":            strings.TrimSpace(result.StreamDigest),
			"stream_preview":           providerGenerationTextPreview(result.StreamPreview),
			"text_digest":              providerGenerationTextDigest(result.Text),
			"text_preview":             providerGenerationTextPreview(result.Text),
			"model_usage": map[string]any{
				"run_id":        runID,
				"agent_id":      "closed-loop-planner",
				"role":          "planner",
				"provider":      strings.TrimSpace(result.ProviderName),
				"model":         strings.TrimSpace(result.Model),
				"input_tokens":  result.InputTokens,
				"output_tokens": result.OutputTokens,
				"cost":          result.EstimatedCostUSD,
				"latency_ms":    int64(result.Latency / time.Millisecond),
			},
		}),
	}
}

func closedLoopPlannerModelToolPlanPreviewEvent(runID string, result ProviderGenerationResult, extraction ModelToolPlanPreviewExtraction, artifact ModelToolPlanPreviewArtifact) eventlog.Event {
	status := "skipped"
	validity := "invalid"
	if extraction.Valid {
		status = "completed"
		validity = "valid"
	}
	reason := strings.TrimSpace(extraction.Reason)
	if reason == "" {
		reason = "unknown"
	}
	return eventlog.Event{
		RunID: runID,
		Type:  eventlog.EventVerification,
		Payload: mustJSON(map[string]any{
			"kind":                           "model_tool_plan_preview",
			"source":                         closedLoopPlannerGenerationStageID,
			"agent_id":                       "closed-loop-planner",
			"role":                           "planner",
			"status":                         status,
			"execution_mode":                 "metadata-only",
			"provider":                       strings.TrimSpace(result.ProviderName),
			"model":                          strings.TrimSpace(result.Model),
			"action":                         string(extraction.Action),
			"semantic_action":                string(extraction.Action),
			"valid":                          extraction.Valid,
			"validity":                       validity,
			"reason":                         reason,
			"marker_preview":                 strings.TrimSpace(extraction.MarkerPreview),
			"model_output_digest":            strings.TrimSpace(extraction.ModelOutputDigest),
			"model_generation_artifact_path": strings.TrimSpace(artifact.ModelGenerationArtifactPath),
			"compile_preview_review_path":    strings.TrimSpace(artifact.ReviewPath),
			"compile_preview_nodes":          artifact.ReviewNodes,
			"compile_preview_warnings":       artifact.ReviewWarnings,
			"artifact_path":                  strings.TrimSpace(artifact.Path),
			"artifact_digest":                strings.TrimSpace(artifact.ArtifactDigest),
			"assistant_text_persisted":       false,
			"executed_tools":                 0,
			"worker_dispatch":                false,
			"target_contact":                 false,
		}),
	}
}

func closedLoopPlannerModelGenerationSuggestionEvent(runID string, result ProviderGenerationResult, ledgerWrite bool) eventlog.Event {
	providerName := strings.TrimSpace(result.ProviderName)
	if providerName == "" {
		providerName = "unknown"
	}
	model := strings.TrimSpace(result.Model)
	if model == "" {
		model = "not-set"
	}
	return eventlog.Event{
		RunID: runID,
		Type:  eventlog.EventFact,
		Payload: mustJSON(map[string]any{
			"fact_id":    "a-model-generation-provider-gate",
			"kind":       "suggestion",
			"statement":  fmt.Sprintf("closed-loop planner provider generation completed for provider %s model %s with %d input and %d output tokens; full assistant text and prompts were not persisted; usage ledger write=%t", providerName, model, result.InputTokens, result.OutputTokens, ledgerWrite),
			"confidence": 1,
			"source":     "model-generation",
		}),
	}
}

func providerGenerationTextDigest(text string) string {
	sum := sha256.Sum256([]byte(text))
	return fmt.Sprintf("%x", sum[:])
}

func providerGenerationTextPreview(text string) string {
	preview := strings.Join(strings.Fields(text), " ")
	if len(preview) > 120 {
		preview = strings.TrimSpace(preview[:120]) + "..."
	}
	return preview
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

// createClosedLoopPlannerCompilePreview resolves the planner-selected
// action for task via the prebuilt intent→action index (nil falls back to
// a direct event scan).
func createClosedLoopPlannerCompilePreview(root string, task sisyphus.Task, events []eventlog.Event, intentActions map[string]compiler.SemanticAction, manifests []toolchain.Manifest) (closedLoopPlannerCompilePreview, error) {
	action, found, err := closedLoopPlannerSelectedActionIndexed(task, intentActions, events)
	if err != nil {
		return closedLoopPlannerCompilePreview{}, err
	}
	if !found {
		return closedLoopPlannerCompilePreview{}, nil
	}
	review, err := CreateToolCompilePreviewReview(root, ToolCompilePreviewOptions{Action: action, Manifests: manifests})
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

// closedLoopPlannerSelectedActionIndexed resolves the latest action for the
// task's intent: O(1) via the prebuilt index when available, otherwise a
// direct scan of events with identical semantics.
func closedLoopPlannerSelectedActionIndexed(task sisyphus.Task, intentActions map[string]compiler.SemanticAction, events []eventlog.Event) (compiler.SemanticAction, bool, error) {
	if intentActions == nil {
		return closedLoopPlannerSelectedAction(task, events)
	}
	intentID := strings.TrimSpace(task.IntentID)
	if intentID == "" {
		return "", false, nil
	}
	action, found := intentActions[intentID]
	return action, found, nil
}

// closedLoopPlannerIntentActionIndex scans the event mirror once and maps
// each intent ID to its latest non-empty action, so per-task lookups are
// O(1) instead of rescanning and re-decoding every intent event.
func closedLoopPlannerIntentActionIndex(events []eventlog.Event) (map[string]compiler.SemanticAction, error) {
	index := map[string]compiler.SemanticAction{}
	for _, event := range events {
		if event.Type != eventlog.EventIntent {
			continue
		}
		var payload closedLoopPlannerIntentPayload
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			return nil, fmt.Errorf("decode closed-loop planner intent payload: %w", err)
		}
		id := strings.TrimSpace(payload.IntentID)
		if id == "" {
			id = strings.TrimSpace(payload.ID)
		}
		if id == "" {
			continue
		}
		action := compiler.SemanticAction(strings.TrimSpace(payload.Action))
		if strings.TrimSpace(string(action)) == "" {
			continue
		}
		index[id] = action
	}
	return index, nil
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
