package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ouroboros/ouroboros/internal/compiler"
	"github.com/ouroboros/ouroboros/internal/eventlog"
)

type WorkspaceWorkflowStatus struct {
	Status               string
	Mode                 string
	Stage                string
	NextGate             string
	Reason               string
	Command              string
	Action               compiler.SemanticAction
	PreviewPath          string
	AcceptancePath       string
	ReviewPath           string
	SelectionPath        string
	DispatchPath         string
	ExecutionPath        string
	ReportDispatchPath   string
	ReportExecutionPath  string
	ReportSelectionPath  string
	ReportRunID          string
	ReportPath           string
	PreviewCount         int
	AcceptanceCount      int
	DispatchCount        int
	ExecutionCount       int
	ReportDispatchCount  int
	ReportExecutionCount int
	SafetyErrorCount     int
	SafetyErrors         []string
}

func SummarizeWorkspaceWorkflow(root string) (WorkspaceWorkflowStatus, error) {
	doctor, err := doctorWorkspaceMetadataOnly(root)
	if err != nil {
		return WorkspaceWorkflowStatus{}, err
	}
	if !doctor.Ready {
		return WorkspaceWorkflowStatus{}, fmt.Errorf("workspace is not ready: %v", doctor.Errors)
	}

	status := WorkspaceWorkflowStatus{
		Status: "ready",
		Mode:   "read-only",
		Stage:  "model-tool-plan",
	}

	previews, err := ListModelToolPlanPreviewArtifacts(root)
	if err != nil {
		return WorkspaceWorkflowStatus{}, err
	}
	acceptances, err := ListModelToolPlanPreviewAcceptances(root)
	if err != nil {
		return WorkspaceWorkflowStatus{}, err
	}
	dispatches, err := ListToolCompilePreviewReviewExecutionDispatches(root)
	if err != nil {
		return WorkspaceWorkflowStatus{}, err
	}
	executions, err := ListToolCompilePreviewReviewExecutions(root)
	if err != nil {
		return WorkspaceWorkflowStatus{}, err
	}
	reportDispatches, err := listWorkflowReportProjectionDispatches(root)
	if err != nil {
		return WorkspaceWorkflowStatus{}, err
	}
	reportExecutions, err := listWorkflowReportProjectionDispatchExecutions(root)
	if err != nil {
		return WorkspaceWorkflowStatus{}, err
	}
	reportSelection, reportSelectionFound, err := readWorkflowReportProjectionSelection(root)
	if err != nil {
		return WorkspaceWorkflowStatus{}, err
	}

	status.PreviewCount = len(previews)
	status.AcceptanceCount = len(acceptances)
	status.DispatchCount = len(dispatches)
	status.ExecutionCount = len(executions)
	status.ReportDispatchCount = len(reportDispatches)
	status.ReportExecutionCount = len(reportExecutions)

	if preview, ok := latestPendingWorkflowPreview(previews, acceptances); ok {
		status.Action = preview.Action
		status.PreviewPath = preview.Path
		status.ReviewPath = preview.ReviewPath
		status.NextGate = "accept-model-tool-plan-preview"
		status.Reason = "valid-preview-awaits-acceptance"
		status.Command = fmt.Sprintf("ouro provider tool-plan-preview-accept %s %s --accept", root, filepath.Base(preview.Path))
		return status, nil
	}

	selection, found, err := readWorkflowToolCompilePreviewReviewSelection(root)
	if err != nil {
		return WorkspaceWorkflowStatus{}, err
	}

	if acceptance, ok := latestWorkflowAcceptance(acceptances); ok && strings.TrimSpace(acceptance.ReviewPath) != "" {
		_, hasDispatch := latestWorkflowDispatchForReview(dispatches, acceptance.ReviewPath)
		selectionMatchesAcceptance := found && strings.TrimSpace(selection.Status) == "current" && sameWorkflowPath(selection.ReviewPath, acceptance.ReviewPath)
		if hasDispatch || selectionMatchesAcceptance {
			status.Action = acceptance.Action
			status.AcceptancePath = acceptance.Path
			status.PreviewPath = acceptance.PreviewArtifactPath
			status.ReviewPath = acceptance.ReviewPath
			status.SelectionPath = acceptance.SelectionPath
			if workflowStatus, ok := workflowCurrentReviewStatus(root, status, dispatches, executions, reportDispatches, reportExecutions, reportSelection, reportSelectionFound); ok {
				return workflowStatus, nil
			}
		}
	}

	if found && strings.TrimSpace(selection.Status) == "current" && strings.TrimSpace(selection.ReviewPath) != "" {
		status.Action = selection.Action
		status.ReviewPath = selection.ReviewPath
		status.SelectionPath = selection.Path
		if acceptance, ok := latestWorkflowAcceptanceForReview(acceptances, selection.ReviewPath); ok {
			status.AcceptancePath = acceptance.Path
			status.PreviewPath = acceptance.PreviewArtifactPath
		}
		if workflowStatus, ok := workflowCurrentReviewStatus(root, status, dispatches, executions, reportDispatches, reportExecutions, reportSelection, reportSelectionFound); ok {
			return workflowStatus, nil
		}
	}

	status.NextGate = "run-provider-tool-plan-preview"
	status.Reason = "no-current-model-tool-plan"
	status.Command = fmt.Sprintf("ouro run %s --provider-generate --provider-generate-tool-plan-preview --provider-generate-prompt <prompt>", root)
	return status, nil
}

func workflowCurrentReviewStatus(root string, status WorkspaceWorkflowStatus, dispatches []ToolCompilePreviewReviewExecutionDispatch, executions []ToolCompilePreviewReviewExecution, reportDispatches []ReportProjectionDispatch, reportExecutions []ReportProjectionDispatchExecution, reportSelection ReportProjectionDispatchExecutionSelection, reportSelectionFound bool) (WorkspaceWorkflowStatus, bool) {
	dispatch, hasDispatch := latestWorkflowDispatchForReview(dispatches, status.ReviewPath)
	if !hasDispatch {
		status.NextGate = "create-compile-preview-review-execution-dispatch"
		status.Reason = "current-review-awaits-dispatch"
		status.Command = fmt.Sprintf("ouro toolchain compile-preview-review-execution-dispatch %s %s", root, status.ReviewPath)
		return status, true
	}

	status.DispatchPath = dispatch.Path
	execution, hasExecution := latestWorkflowExecutionForDispatch(executions, dispatch.Path)
	if !hasExecution {
		if safetyErrors := workflowSafetyAuthorizationErrors(root); len(safetyErrors) > 0 {
			status.Status = "blocked"
			status.NextGate = "safety-authorization"
			status.Reason = "compile-execution-requires-current-safety-authorization"
			status.Command = fmt.Sprintf("ouro doctor %s --require-safety-authorization", root)
			status.SafetyErrors = safetyErrors
			status.SafetyErrorCount = len(safetyErrors)
			return status, true
		}
		status.NextGate = "execute-compile-preview-review-execution-dispatch"
		status.Reason = "dispatch-awaits-execution"
		status.Command = fmt.Sprintf("ouro toolchain execute-compile-preview-review-execution-dispatch %s %s --execute", root, dispatch.Path)
		return status, true
	}

	status.ExecutionPath = execution.Path
	if reportStatus, ok := workflowReportProjectionStatus(root, status, reportDispatches, reportExecutions, reportSelection, reportSelectionFound); ok {
		return reportStatus, true
	}

	status.NextGate = "review-compile-preview-execution"
	status.Reason = "execution-audit-awaits-review"
	status.Command = fmt.Sprintf("ouro toolchain compile-preview-review-executions %s %s", root, filepath.Base(execution.Path))
	return status, true
}

func readWorkflowToolCompilePreviewReviewSelection(root string) (ToolCompilePreviewReviewSelection, bool, error) {
	doctor, err := doctorWorkspaceMetadataOnly(root)
	if err != nil {
		return ToolCompilePreviewReviewSelection{}, false, err
	}
	if !doctor.Ready {
		return ToolCompilePreviewReviewSelection{}, false, fmt.Errorf("workspace is not ready: %v", doctor.Errors)
	}
	return readToolCompilePreviewReviewSelectionForExecution(root)
}

func workflowReportProjectionStatus(root string, status WorkspaceWorkflowStatus, dispatches []ReportProjectionDispatch, executions []ReportProjectionDispatchExecution, selection ReportProjectionDispatchExecutionSelection, selectionFound bool) (WorkspaceWorkflowStatus, bool) {
	status.Stage = "report-projection"

	if selectionFound && strings.TrimSpace(selection.Status) == "current" {
		status.ReportSelectionPath = selection.Path
		status.ReportExecutionPath = selection.ExecutionPath
		status.ReportDispatchPath = selection.DispatchPath
		status.ReportRunID = selection.RunID
		status.ReportPath = selection.ReportPath
		status.NextGate = "review-current-report-projection"
		status.Reason = "report-projection-selection-current"
		status.Command = fmt.Sprintf("ouro report-project-dispatch-execution-selection %s", root)
		return status, true
	}

	if execution, ok := latestWorkflowReportExecutionAwaitingSelection(executions); ok {
		status.ReportExecutionPath = execution.Path
		status.ReportDispatchPath = execution.DispatchPath
		status.ReportRunID = execution.RunID
		status.ReportPath = execution.ReportPath
		if execution.Annotation == nil || strings.TrimSpace(execution.Annotation.Decision) == "" {
			status.NextGate = "review-report-projection-execution"
			status.Reason = "report-execution-awaits-review"
			status.Command = fmt.Sprintf("ouro report-project-dispatch-execution-annotate %s %s --decision accepted", root, filepath.Base(execution.Path))
			return status, true
		}
		if strings.TrimSpace(execution.Annotation.Decision) == "accepted" {
			status.NextGate = "select-report-projection-execution"
			status.Reason = "accepted-report-execution-awaits-selection"
			status.Command = fmt.Sprintf("ouro report-project-dispatch-execution-select %s %s", root, filepath.Base(execution.Path))
			return status, true
		}
	}

	if dispatch, ok := latestWorkflowReportDispatchAwaitingExecution(dispatches, executions); ok {
		status.ReportDispatchPath = dispatch.Path
		status.ReportRunID = dispatch.RunID
		status.ReportPath = dispatch.ReportPath
		status.NextGate = "execute-report-projection-dispatch"
		status.Reason = "report-dispatch-awaits-execution"
		status.Command = fmt.Sprintf("ouro execute-report-project-dispatch %s %s --execute", root, filepath.Base(dispatch.Path))
		return status, true
	}

	if runID, ok := latestWorkflowRunAwaitingReportProjection(root, dispatches, executions); ok {
		status.ReportRunID = runID
		if path, err := reportProjectionPath(root, runID); err == nil {
			status.ReportPath = path
		}
		status.NextGate = "create-report-projection-dispatch"
		status.Reason = "compile-execution-awaits-report-dispatch"
		status.Command = fmt.Sprintf("ouro report-project-dispatch %s %s", root, runID)
		return status, true
	}

	return WorkspaceWorkflowStatus{}, false
}

func listWorkflowReportProjectionDispatches(root string) ([]ReportProjectionDispatch, error) {
	exists, err := workflowReplayEventLogExists(root)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, nil
	}
	return ListReportProjectionDispatches(root)
}

func listWorkflowReportProjectionDispatchExecutions(root string) ([]ReportProjectionDispatchExecution, error) {
	exists, err := workflowReplayEventLogExists(root)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, nil
	}
	return ListReportProjectionDispatchExecutions(root)
}

func readWorkflowReportProjectionSelection(root string) (ReportProjectionDispatchExecutionSelection, bool, error) {
	exists, err := workflowReplayEventLogExists(root)
	if err != nil {
		return ReportProjectionDispatchExecutionSelection{}, false, err
	}
	if !exists {
		return ReportProjectionDispatchExecutionSelection{}, false, nil
	}
	return ReadReportProjectionDispatchExecutionSelection(root)
}

func workflowReplayEventLogExists(root string) (bool, error) {
	_, err := os.Lstat(filepath.Join(root, "replay", "events.db"))
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("inspect replay event log for workflow status: %w", err)
	}
	return true, nil
}

func latestPendingWorkflowPreview(previews []ModelToolPlanPreviewArtifact, acceptances []ModelToolPlanPreviewAcceptance) (ModelToolPlanPreviewArtifact, bool) {
	accepted := map[string]bool{}
	for _, acceptance := range acceptances {
		path := strings.TrimSpace(acceptance.PreviewArtifactPath)
		if path != "" {
			accepted[path] = true
			accepted[filepath.Base(path)] = true
		}
	}

	var latest ModelToolPlanPreviewArtifact
	found := false
	for _, preview := range previews {
		if !preview.Valid || strings.TrimSpace(preview.Status) != "completed" || strings.TrimSpace(preview.ExecutionMode) != "metadata-only" {
			continue
		}
		path := strings.TrimSpace(preview.Path)
		if accepted[path] || accepted[filepath.Base(path)] {
			continue
		}
		if !found || workflowTimestampAfter(preview.CreatedAt, latest.CreatedAt) {
			latest = preview
			found = true
		}
	}
	return latest, found
}

func latestWorkflowAcceptanceForReview(acceptances []ModelToolPlanPreviewAcceptance, reviewPath string) (ModelToolPlanPreviewAcceptance, bool) {
	var latest ModelToolPlanPreviewAcceptance
	found := false
	for _, acceptance := range acceptances {
		if sameWorkflowPath(acceptance.ReviewPath, reviewPath) {
			if !found || workflowTimestampAfter(acceptance.AcceptedAt, latest.AcceptedAt) {
				latest = acceptance
				found = true
			}
		}
	}
	return latest, found
}

func latestWorkflowAcceptance(acceptances []ModelToolPlanPreviewAcceptance) (ModelToolPlanPreviewAcceptance, bool) {
	var latest ModelToolPlanPreviewAcceptance
	found := false
	for _, acceptance := range acceptances {
		if strings.TrimSpace(acceptance.Status) != "accepted" || strings.TrimSpace(acceptance.ExecutionMode) != "metadata-only" {
			continue
		}
		if !found || workflowTimestampAfter(acceptance.AcceptedAt, latest.AcceptedAt) {
			latest = acceptance
			found = true
		}
	}
	return latest, found
}

func latestWorkflowDispatchForReview(dispatches []ToolCompilePreviewReviewExecutionDispatch, reviewPath string) (ToolCompilePreviewReviewExecutionDispatch, bool) {
	var latest ToolCompilePreviewReviewExecutionDispatch
	found := false
	for _, dispatch := range dispatches {
		if strings.TrimSpace(dispatch.Status) != "dry-run" || strings.TrimSpace(dispatch.Mode) != "metadata-only" {
			continue
		}
		if sameWorkflowPath(dispatch.ReviewPath, reviewPath) {
			if !found || workflowTimestampAfter(dispatch.CreatedAt, latest.CreatedAt) {
				latest = dispatch
				found = true
			}
		}
	}
	return latest, found
}

func latestWorkflowExecutionForDispatch(executions []ToolCompilePreviewReviewExecution, dispatchPath string) (ToolCompilePreviewReviewExecution, bool) {
	var latest ToolCompilePreviewReviewExecution
	found := false
	for _, execution := range executions {
		if sameWorkflowPath(execution.DispatchPath, dispatchPath) {
			if !found || workflowTimestampAfter(execution.ExecutedAt, latest.ExecutedAt) {
				latest = execution
				found = true
			}
		}
	}
	return latest, found
}

func latestWorkflowReportDispatchAwaitingExecution(dispatches []ReportProjectionDispatch, executions []ReportProjectionDispatchExecution) (ReportProjectionDispatch, bool) {
	executed := map[string]bool{}
	for _, execution := range executions {
		path := strings.TrimSpace(execution.DispatchPath)
		if path != "" {
			executed[path] = true
			executed[filepath.Base(path)] = true
		}
	}

	var latest ReportProjectionDispatch
	found := false
	for _, dispatch := range dispatches {
		if strings.TrimSpace(dispatch.Status) != "dry-run" || strings.TrimSpace(dispatch.Mode) != "metadata-only" {
			continue
		}
		path := strings.TrimSpace(dispatch.Path)
		if path == "" || executed[path] || executed[filepath.Base(path)] {
			continue
		}
		if !found || workflowTimestampAfter(dispatch.CreatedAt, latest.CreatedAt) {
			latest = dispatch
			found = true
		}
	}
	return latest, found
}

func latestWorkflowReportExecutionAwaitingSelection(executions []ReportProjectionDispatchExecution) (ReportProjectionDispatchExecution, bool) {
	var latest ReportProjectionDispatchExecution
	found := false
	for _, execution := range executions {
		if strings.TrimSpace(execution.Status) != "completed" || strings.TrimSpace(execution.Mode) != "execute" || !execution.Written {
			continue
		}
		if execution.Annotation == nil || strings.TrimSpace(execution.Annotation.Decision) == "" || strings.TrimSpace(execution.Annotation.Decision) == "accepted" {
			if !found || workflowTimestampAfter(execution.ExecutedAt, latest.ExecutedAt) {
				latest = execution
				found = true
			}
		}
	}
	return latest, found
}

func latestWorkflowRunAwaitingReportProjection(root string, dispatches []ReportProjectionDispatch, executions []ReportProjectionDispatchExecution) (string, bool) {
	ctx := context.Background()
	store, err := eventlog.OpenSQLiteReadOnly(filepath.Join(root, "replay", "events.db"))
	if err != nil {
		return "", false
	}
	defer store.Close()

	runIDs, err := store.ListRunIDs(ctx)
	if err != nil {
		return "", false
	}

	dispatched := map[string]bool{}
	for _, dispatch := range dispatches {
		if runID := strings.TrimSpace(dispatch.RunID); runID != "" {
			dispatched[runID] = true
		}
	}
	for _, execution := range executions {
		if runID := strings.TrimSpace(execution.RunID); runID != "" {
			dispatched[runID] = true
		}
	}

	for i := len(runIDs) - 1; i >= 0; i-- {
		runID := strings.TrimSpace(runIDs[i])
		if runID == "" || dispatched[runID] {
			continue
		}
		events, err := store.Replay(ctx, runID)
		if err != nil || len(events) == 0 {
			continue
		}
		if strings.TrimSpace(replayReportPath(events)) != "" {
			continue
		}
		return runID, true
	}
	return "", false
}

func workflowTimestampAfter(candidateValue, currentValue string) bool {
	candidate := workflowTimestamp(candidateValue)
	current := workflowTimestamp(currentValue)
	if !candidate.IsZero() && !current.IsZero() {
		return candidate.After(current)
	}
	if !candidate.IsZero() {
		return true
	}
	if current.IsZero() {
		return strings.TrimSpace(candidateValue) > strings.TrimSpace(currentValue)
	}
	return false
}

func workflowTimestamp(value string) time.Time {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed
		}
	}
	return time.Time{}
}

func sameWorkflowPath(left, right string) bool {
	left = strings.TrimSpace(left)
	right = strings.TrimSpace(right)
	if left == "" || right == "" {
		return false
	}
	return filepath.Clean(left) == filepath.Clean(right) || filepath.Base(left) == filepath.Base(right)
}

// workflowSafetyAuthorizationErrors returns strict-mode safety bypass errors
// for the workspace scope, mirroring doctor --require-safety-authorization.
func workflowSafetyAuthorizationErrors(root string) []string {
	config, err := LoadScopeConfig(filepath.Join(root, "scope.yaml"))
	if err != nil {
		if !os.IsNotExist(err) {
			return []string{fmt.Sprintf("load scope config for workflow safety gate: %v", err)}
		}
		config = defaultRuntimeWorkerSafetyScopeConfig()
	}
	if err := ValidateScopeConfig(config); err != nil {
		return []string{fmt.Sprintf("invalid scope config for workflow safety gate: %v", err)}
	}
	authorization, authorizationFound, err := loadWorkspaceAuthorization(filepath.Join(root, "authorization.yaml"))
	if err != nil {
		return []string{fmt.Sprintf("invalid authorization.yaml for workflow safety gate: %v", err)}
	}
	return scopeConfigSafetyAuthorizationErrors(config, authorization, authorizationFound, time.Now().UTC())
}
