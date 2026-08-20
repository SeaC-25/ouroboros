package app

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ouroboros/ouroboros/internal/compiler"
	"github.com/ouroboros/ouroboros/internal/contracts"
	"github.com/ouroboros/ouroboros/internal/eventlog"
	"github.com/ouroboros/ouroboros/internal/evidence"
	"github.com/ouroboros/ouroboros/internal/frontier"
	reportpkg "github.com/ouroboros/ouroboros/internal/report"
	"github.com/ouroboros/ouroboros/internal/toolchain"
	"github.com/ouroboros/ouroboros/internal/tui"
	"gopkg.in/yaml.v3"
)

func NewTUIModelForWorkspace(root string) tui.Model {
	return tuiModelWithWorkspaceRuntimeRows(root, baseTUIModelForWorkspace(root))
}

func tuiModelFromSnapshot(snapshot eventlog.ReplaySnapshot) tui.Model {
	return tui.NewModel().WithReplaySnapshot(snapshot)
}

func tuiModelFromEvents(events []eventlog.Event) tui.Model {
	return tui.NewModel().WithReplayEvents(events)
}

func tuiModelFromEventsForWorkspace(root string, events []eventlog.Event) tui.Model {
	return tuiModelWithWorkspaceRuntimeRows(root, baseTUIModelForWorkspace(root).WithReplayEvents(events))
}

func renderTUISnapshot(snapshot eventlog.ReplaySnapshot) string {
	return tuiModelFromSnapshot(snapshot).View()
}

func tuiRuntimeActionRequester(root string) tui.RuntimeActionRequester {
	return func(action tui.RuntimeAction) (tui.RuntimeActionRequestResult, error) {
		if strings.TrimSpace(action.Kind) != "install" {
			return tui.RuntimeActionRequestResult{}, fmt.Errorf("unsupported runtime action kind: %s", action.Kind)
		}
		pack := strings.TrimSpace(action.Pack)
		if pack == "" {
			return tui.RuntimeActionRequestResult{}, fmt.Errorf("install runtime action is missing pack metadata")
		}
		dependency := strings.TrimSpace(action.Name)
		if dependency == "" {
			return tui.RuntimeActionRequestResult{}, fmt.Errorf("install runtime action is missing dependency metadata")
		}
		request, err := CreateToolchainInstallRequest(root, pack, dependency)
		if err != nil {
			return tui.RuntimeActionRequestResult{}, err
		}
		return tui.RuntimeActionRequestResult{
			Status: request.Status,
			Path:   request.Path,
		}, nil
	}
}

func tuiRuntimeActionExecutor(root string) tui.RuntimeActionExecutor {
	return func(action tui.RuntimeAction) (tui.RuntimeActionExecutionResult, error) {
		kind := strings.TrimSpace(action.Kind)
		if kind != "compile-preview" && kind != "compile-preview-review-artifact" && kind != "compile-preview-review-selection" && kind != "compile-preview-review-execution-dispatch-artifact" && kind != "model-tool-plan-preview" && kind != "workflow-status" && kind != "report-projection-dispatch-artifact" && kind != "report-projection-dispatch-execution-artifact" && kind != "report-projection-execution-selection" && kind != "replay-workbench-session-artifact" && kind != "replay-workbench-session-selection" && kind != "replay-workbench-collaboration-artifact" && kind != "replay-workbench-collaboration-selection" && kind != "replay-workbench-collaboration-note" && kind != "replay-workbench-collaboration-note-selection" && kind != "install-dispatch-artifact" && kind != "install-retry-dispatch-artifact" && kind != "worker-lifecycle-action-plan-artifact" && kind != "worker-lifecycle-action-plan-selection" && kind != "worker-lifecycle-policy-artifact" && kind != "worker-lifecycle-policy-selection" && kind != "worker-lease-result-reconciliation-artifact" && kind != "worker-lease-reclaim-artifact" && kind != "worker-placement-artifact" && kind != "worker-placement-dispatch-artifact" && kind != "worker-queue-claim-artifact" && kind != "worker-transport-request-artifact" && kind != "worker-transport-daemon-artifact" {
			return tui.RuntimeActionExecutionResult{}, fmt.Errorf("unsupported runtime action kind: %s", action.Kind)
		}
		if kind == "compile-preview" {
			options := ToolCompilePreviewOptions{}
			if actionType := strings.TrimSpace(action.ActionType); actionType != "" {
				options.Action = compiler.SemanticAction(actionType)
			}
			review, err := CreateToolCompilePreviewReview(root, options)
			reviews := []ToolCompilePreviewReview{review}
			if listed, listErr := ListToolCompilePreviewReviews(root); listErr == nil {
				reviews = listed
			}
			result := tui.RuntimeActionExecutionResult{
				Status:      "completed",
				Path:        review.Path,
				Output:      "review created",
				RuntimeRows: compilePreviewReviewRuntimeRows(reviews),
			}
			if review.Path == "" {
				result.RuntimeRows = nil
			}
			if err != nil {
				result.Status = "failed"
				return result, err
			}
			return result, nil
		}
		if kind == "compile-preview-review-artifact" {
			reviewPath := strings.TrimSpace(action.ArtifactPath)
			if reviewPath == "" {
				return tui.RuntimeActionExecutionResult{}, fmt.Errorf("compile-preview review artifact runtime action is missing artifact path")
			}
			if strings.TrimSpace(action.Current) == "true" {
				dispatch, err := CreateToolCompilePreviewReviewExecutionDispatch(root, reviewPath, ToolCompilePreviewReviewExecutionDispatchOptions{
					Actor:  "tui",
					Reason: "dispatched from TUI",
				})
				dispatches := []ToolCompilePreviewReviewExecutionDispatch{dispatch}
				if listed, listErr := ListToolCompilePreviewReviewExecutionDispatches(root); listErr == nil {
					dispatches = listed
				}
				result := tui.RuntimeActionExecutionResult{
					Status:      "completed",
					Path:        dispatch.Path,
					Output:      "execution dispatch created",
					RuntimeRows: compilePreviewReviewExecutionDispatchRuntimeRows(dispatches),
				}
				if dispatch.Path == "" {
					result.RuntimeRows = nil
				}
				if err != nil {
					result.Status = "failed"
					return result, err
				}
				return result, nil
			}
			selection, err := SelectToolCompilePreviewReview(root, reviewPath, ToolCompilePreviewReviewSelectionOptions{
				SelectedBy: "tui",
				Reason:     "selected from TUI",
			})
			reviews, listErr := ListToolCompilePreviewReviews(root)
			refreshedSelection, found, readErr := ReadToolCompilePreviewReviewSelection(root)
			if found {
				selection = refreshedSelection
			}
			result := tui.RuntimeActionExecutionResult{
				Status:      "completed",
				Path:        selection.Path,
				Output:      "review selected",
				RuntimeRows: compilePreviewReviewRuntimeRowsWithSelection(reviews, selection, true),
			}
			if selection.Path == "" {
				result.RuntimeRows = nil
			}
			if err != nil {
				result.Status = "failed"
				return result, err
			}
			if listErr != nil {
				result.Status = "failed"
				return result, listErr
			}
			if readErr != nil {
				result.Status = "failed"
				return result, readErr
			}
			return result, nil
		}
		if kind == "compile-preview-review-selection" {
			selection, err := ClearToolCompilePreviewReviewSelection(root, ToolCompilePreviewReviewSelectionClearOptions{
				ClearedBy: "tui",
				Reason:    "cleared from TUI",
			})
			reviews, listErr := ListToolCompilePreviewReviews(root)
			refreshedSelection, found, readErr := ReadToolCompilePreviewReviewSelection(root)
			if found {
				selection = refreshedSelection
			}
			result := tui.RuntimeActionExecutionResult{
				Status:      "completed",
				Path:        selection.Path,
				Output:      "selection cleared",
				RuntimeRows: compilePreviewReviewRuntimeRowsWithSelection(reviews, selection, true),
			}
			if selection.Path == "" {
				result.RuntimeRows = nil
			}
			if err != nil {
				result.Status = "failed"
				return result, err
			}
			if listErr != nil {
				result.Status = "failed"
				return result, listErr
			}
			if readErr != nil {
				result.Status = "failed"
				return result, readErr
			}
			return result, nil
		}
		if kind == "model-tool-plan-preview" {
			if strings.TrimSpace(action.State) != "valid" {
				return tui.RuntimeActionExecutionResult{}, fmt.Errorf("model tool-plan preview runtime action must be valid")
			}
			previewPath := strings.TrimSpace(action.ArtifactPath)
			if previewPath == "" {
				return tui.RuntimeActionExecutionResult{}, fmt.Errorf("model tool-plan preview runtime action is missing artifact path")
			}
			acceptance, err := AcceptModelToolPlanPreviewArtifact(root, previewPath, ModelToolPlanPreviewAcceptanceOptions{
				AcceptedBy: "tui",
				Reason:     "accepted from TUI",
			})
			acceptances := []ModelToolPlanPreviewAcceptance{acceptance}
			if listed, listErr := ListModelToolPlanPreviewAcceptances(root); listErr == nil {
				acceptances = listed
			}
			result := tui.RuntimeActionExecutionResult{
				Status:      "completed",
				Path:        acceptance.Path,
				Output:      "model tool-plan preview accepted",
				RuntimeRows: modelToolPlanPreviewAcceptanceRuntimeRows(acceptances),
			}
			if acceptance.Path == "" {
				result.RuntimeRows = nil
			}
			if err != nil {
				result.Status = "failed"
				return result, err
			}
			return result, nil
		}
		if kind == "worker-lifecycle-policy-artifact" {
			policyPath := strings.TrimSpace(action.ArtifactPath)
			if policyPath == "" {
				return tui.RuntimeActionExecutionResult{}, fmt.Errorf("worker lifecycle policy artifact runtime action is missing artifact path")
			}
			selection, err := SelectWorkerLifecyclePolicy(root, policyPath, WorkerLifecyclePolicySelectionOptions{
				SelectedBy: "tui",
				Reason:     "selected from TUI",
			})
			policies, listErr := ListWorkerLifecyclePolicies(root)
			refreshedSelection, found, readErr := ReadWorkerLifecyclePolicySelection(root)
			if found {
				selection = refreshedSelection
			}
			result := tui.RuntimeActionExecutionResult{
				Status:      "completed",
				Path:        selection.Path,
				Output:      "policy selected",
				RuntimeRows: workerLifecyclePolicyArtifactRuntimeRowsWithSelection(policies, selection, true),
			}
			if selection.Path == "" {
				result.RuntimeRows = nil
			}
			if err != nil {
				result.Status = "failed"
				return result, err
			}
			if listErr != nil {
				result.Status = "failed"
				return result, listErr
			}
			if readErr != nil {
				result.Status = "failed"
				return result, readErr
			}
			return result, nil
		}
		if kind == "worker-lifecycle-policy-selection" {
			selection, err := ClearWorkerLifecyclePolicySelection(root, WorkerLifecyclePolicySelectionClearOptions{
				ClearedBy: "tui",
				Reason:    "cleared from TUI",
			})
			policies, listErr := ListWorkerLifecyclePolicies(root)
			refreshedSelection, found, readErr := ReadWorkerLifecyclePolicySelection(root)
			if found {
				selection = refreshedSelection
			}
			result := tui.RuntimeActionExecutionResult{
				Status:      "completed",
				Path:        selection.Path,
				Output:      "policy selection cleared",
				RuntimeRows: workerLifecyclePolicyArtifactRuntimeRowsWithSelection(policies, selection, true),
			}
			if selection.Path == "" {
				result.RuntimeRows = nil
			}
			if err != nil {
				result.Status = "failed"
				return result, err
			}
			if listErr != nil {
				result.Status = "failed"
				return result, listErr
			}
			if readErr != nil {
				result.Status = "failed"
				return result, readErr
			}
			return result, nil
		}
		if kind == "replay-workbench-session-artifact" {
			sessionPath := strings.TrimSpace(action.ArtifactPath)
			if sessionPath == "" {
				return tui.RuntimeActionExecutionResult{}, fmt.Errorf("replay workbench session artifact runtime action is missing artifact path")
			}
			selection, err := SelectReplayWorkbenchSession(root, sessionPath, ReplayWorkbenchSessionSelectionOptions{
				SelectedBy: "tui",
				Reason:     "selected from TUI",
			})
			sessions, listErr := ListReplayWorkbenchSessions(root)
			refreshedSelection, found, readErr := ReadReplayWorkbenchSessionSelection(root)
			if found {
				selection = refreshedSelection
			}
			result := tui.RuntimeActionExecutionResult{
				Status:      "completed",
				Path:        selection.Path,
				Output:      "session selected",
				RuntimeRows: replayWorkbenchSessionRuntimeRowsWithSelection(sessions, selection, true),
			}
			if selection.Path == "" {
				result.RuntimeRows = nil
			}
			if err != nil {
				result.Status = "failed"
				return result, err
			}
			if listErr != nil {
				result.Status = "failed"
				return result, listErr
			}
			if readErr != nil {
				result.Status = "failed"
				return result, readErr
			}
			return result, nil
		}
		if kind == "replay-workbench-session-selection" {
			selection, err := ClearReplayWorkbenchSessionSelection(root, ReplayWorkbenchSessionSelectionClearOptions{
				ClearedBy: "tui",
				Reason:    "cleared from TUI",
			})
			sessions, listErr := ListReplayWorkbenchSessions(root)
			refreshedSelection, found, readErr := ReadReplayWorkbenchSessionSelection(root)
			if found {
				selection = refreshedSelection
			}
			result := tui.RuntimeActionExecutionResult{
				Status:      "completed",
				Path:        selection.Path,
				Output:      "session selection cleared",
				RuntimeRows: replayWorkbenchSessionRuntimeRowsWithSelection(sessions, selection, true),
			}
			if selection.Path == "" {
				result.RuntimeRows = nil
			}
			if err != nil {
				result.Status = "failed"
				return result, err
			}
			if listErr != nil {
				result.Status = "failed"
				return result, listErr
			}
			if readErr != nil {
				result.Status = "failed"
				return result, readErr
			}
			return result, nil
		}
		if kind == "replay-workbench-collaboration-artifact" {
			collaborationPath := strings.TrimSpace(action.ArtifactPath)
			if collaborationPath == "" {
				return tui.RuntimeActionExecutionResult{}, fmt.Errorf("replay workbench collaboration artifact runtime action is missing artifact path")
			}
			selection, err := SelectReplayWorkbenchCollaboration(root, collaborationPath, ReplayWorkbenchCollaborationSelectionOptions{
				SelectedBy: "tui",
				Reason:     "selected from TUI",
			})
			collaborations, listErr := ListReplayWorkbenchCollaborations(root)
			refreshedSelection, found, readErr := ReadReplayWorkbenchCollaborationSelection(root)
			if found {
				selection = refreshedSelection
			}
			result := tui.RuntimeActionExecutionResult{
				Status:      "completed",
				Path:        selection.Path,
				Output:      "collaboration selected",
				RuntimeRows: replayWorkbenchCollaborationRuntimeRowsWithSelection(collaborations, selection, true),
			}
			if selection.Path == "" {
				result.RuntimeRows = nil
			}
			if err != nil {
				result.Status = "failed"
				return result, err
			}
			if listErr != nil {
				result.Status = "failed"
				return result, listErr
			}
			if readErr != nil {
				result.Status = "failed"
				return result, readErr
			}
			return result, nil
		}
		if kind == "replay-workbench-collaboration-selection" {
			selection, err := ClearReplayWorkbenchCollaborationSelection(root, ReplayWorkbenchCollaborationSelectionClearOptions{
				ClearedBy: "tui",
				Reason:    "cleared from TUI",
			})
			collaborations, listErr := ListReplayWorkbenchCollaborations(root)
			refreshedSelection, found, readErr := ReadReplayWorkbenchCollaborationSelection(root)
			if found {
				selection = refreshedSelection
			}
			result := tui.RuntimeActionExecutionResult{
				Status:      "completed",
				Path:        selection.Path,
				Output:      "collaboration selection cleared",
				RuntimeRows: replayWorkbenchCollaborationRuntimeRowsWithSelection(collaborations, selection, true),
			}
			if selection.Path == "" {
				result.RuntimeRows = nil
			}
			if err != nil {
				result.Status = "failed"
				return result, err
			}
			if listErr != nil {
				result.Status = "failed"
				return result, listErr
			}
			if readErr != nil {
				result.Status = "failed"
				return result, readErr
			}
			return result, nil
		}
		if kind == "replay-workbench-collaboration-note" {
			notePath := strings.TrimSpace(action.ArtifactPath)
			if notePath == "" {
				return tui.RuntimeActionExecutionResult{}, fmt.Errorf("replay workbench collaboration note runtime action is missing artifact path")
			}
			selection, err := SelectReplayWorkbenchCollaborationNote(root, notePath, ReplayWorkbenchCollaborationNoteSelectionOptions{
				SelectedBy: "tui",
				Reason:     "selected from TUI",
			})
			notes, listErr := ListReplayWorkbenchCollaborationNotes(root)
			refreshedSelection, found, readErr := ReadReplayWorkbenchCollaborationNoteSelection(root)
			if found {
				selection = refreshedSelection
			}
			result := tui.RuntimeActionExecutionResult{
				Status:      "completed",
				Path:        selection.Path,
				Output:      "collaboration note selected",
				RuntimeRows: replayWorkbenchCollaborationNoteRuntimeRowsWithSelection(notes, selection, true),
			}
			if selection.Path == "" {
				result.RuntimeRows = nil
			}
			if err != nil {
				result.Status = "failed"
				return result, err
			}
			if listErr != nil {
				result.Status = "failed"
				return result, listErr
			}
			if readErr != nil {
				result.Status = "failed"
				return result, readErr
			}
			return result, nil
		}
		if kind == "replay-workbench-collaboration-note-selection" {
			selection, err := ClearReplayWorkbenchCollaborationNoteSelection(root, ReplayWorkbenchCollaborationNoteSelectionClearOptions{
				ClearedBy: "tui",
				Reason:    "cleared from TUI",
			})
			notes, listErr := ListReplayWorkbenchCollaborationNotes(root)
			refreshedSelection, found, readErr := ReadReplayWorkbenchCollaborationNoteSelection(root)
			if found {
				selection = refreshedSelection
			}
			result := tui.RuntimeActionExecutionResult{
				Status:      "completed",
				Path:        selection.Path,
				Output:      "collaboration note selection cleared",
				RuntimeRows: replayWorkbenchCollaborationNoteRuntimeRowsWithSelection(notes, selection, true),
			}
			if selection.Path == "" {
				result.RuntimeRows = nil
			}
			if err != nil {
				result.Status = "failed"
				return result, err
			}
			if listErr != nil {
				result.Status = "failed"
				return result, listErr
			}
			if readErr != nil {
				result.Status = "failed"
				return result, readErr
			}
			return result, nil
		}
		if kind == "compile-preview-review-execution-dispatch-artifact" {
			dispatchPath := strings.TrimSpace(action.ArtifactPath)
			if dispatchPath == "" {
				return tui.RuntimeActionExecutionResult{}, fmt.Errorf("compile-preview review execution dispatch runtime action is missing artifact path")
			}
			execution, err := ExecuteToolCompilePreviewReviewExecutionDispatch(context.Background(), root, dispatchPath, ToolCompilePreviewReviewExecutionOptions{
				Execute: true,
				Actor:   "tui",
				Reason:  "executed from TUI",
			})
			executions := []ToolCompilePreviewReviewExecution{execution}
			if listed, listErr := ListToolCompilePreviewReviewExecutions(root); listErr == nil {
				executions = listed
			}
			result := tui.RuntimeActionExecutionResult{
				Status:      "completed",
				Path:        execution.Path,
				Output:      "compile preview review execution dispatch executed",
				RuntimeRows: compilePreviewReviewExecutionRuntimeRows(executions),
			}
			if execution.Path == "" {
				result.RuntimeRows = nil
			}
			if err != nil {
				result.Status = "failed"
				return result, err
			}
			return result, nil
		}
		if kind == "workflow-status" {
			if strings.TrimSpace(action.RequestPath) == "create-compile-preview-review-execution-dispatch" {
				if strings.TrimSpace(action.Source) != "model-tool-plan" {
					return tui.RuntimeActionExecutionResult{}, fmt.Errorf("workflow status compile dispatch runtime action requires model-tool-plan stage")
				}
				reviewPath := strings.TrimSpace(action.ReviewPath)
				if reviewPath == "" || reviewPath == "-" || strings.EqualFold(reviewPath, "none") {
					return tui.RuntimeActionExecutionResult{}, fmt.Errorf("workflow status compile dispatch runtime action is missing review")
				}
				dispatch, err := CreateToolCompilePreviewReviewExecutionDispatch(root, reviewPath, ToolCompilePreviewReviewExecutionDispatchOptions{
					Actor:  "tui",
					Reason: "created from TUI workflow status",
				})
				dispatches := []ToolCompilePreviewReviewExecutionDispatch{dispatch}
				if listed, listErr := ListToolCompilePreviewReviewExecutionDispatches(root); listErr == nil {
					dispatches = listed
				}
				result := tui.RuntimeActionExecutionResult{
					Status:      "completed",
					Path:        dispatch.Path,
					Output:      "workflow status compile dispatch created",
					RuntimeRows: compilePreviewReviewExecutionDispatchRuntimeRows(dispatches),
				}
				if dispatch.Path == "" {
					result.RuntimeRows = nil
				}
				if err != nil {
					result.Status = "failed"
					return result, err
				}
				return result, nil
			}
			if strings.TrimSpace(action.RequestPath) == "execute-compile-preview-review-execution-dispatch" {
				if strings.TrimSpace(action.Source) != "model-tool-plan" {
					return tui.RuntimeActionExecutionResult{}, fmt.Errorf("workflow status compile dispatch execution runtime action requires model-tool-plan stage")
				}
				dispatchPath := strings.TrimSpace(action.DispatchPath)
				if dispatchPath == "" || dispatchPath == "-" || strings.EqualFold(dispatchPath, "none") {
					return tui.RuntimeActionExecutionResult{}, fmt.Errorf("workflow status compile dispatch execution runtime action is missing dispatch")
				}
				execution, err := ExecuteToolCompilePreviewReviewExecutionDispatch(context.Background(), root, dispatchPath, ToolCompilePreviewReviewExecutionOptions{
					Execute: true,
					Actor:   "tui",
					Reason:  "executed from TUI workflow status",
				})
				executions := []ToolCompilePreviewReviewExecution{execution}
				if listed, listErr := ListToolCompilePreviewReviewExecutions(root); listErr == nil {
					executions = listed
				}
				result := tui.RuntimeActionExecutionResult{
					Status:      "completed",
					Path:        execution.Path,
					Output:      "workflow status compile dispatch executed",
					RuntimeRows: compilePreviewReviewExecutionRuntimeRows(executions),
				}
				if execution.Path == "" {
					result.RuntimeRows = nil
				}
				if err != nil {
					result.Status = "failed"
					return result, err
				}
				return result, nil
			}
			if strings.TrimSpace(action.Source) != "report-projection" {
				return tui.RuntimeActionExecutionResult{}, fmt.Errorf("workflow status report projection dispatch runtime action requires report-projection stage")
			}
			switch strings.TrimSpace(action.RequestPath) {
			case "create-report-projection-dispatch":
				runID := strings.TrimSpace(action.RunID)
				if runID == "" {
					return tui.RuntimeActionExecutionResult{}, fmt.Errorf("workflow status report projection dispatch runtime action is missing report run")
				}
				dispatch, err := CreateReportProjectionDispatch(context.Background(), root, runID, ReportProjectionDispatchOptions{
					Actor:  "tui",
					Reason: "created from TUI workflow status",
				})
				dispatches, listErr := ListReportProjectionDispatches(root)
				result := tui.RuntimeActionExecutionResult{
					Status:      "completed",
					Path:        dispatch.Path,
					Output:      "report projection dispatch created",
					RuntimeRows: reportProjectionDispatchRuntimeRows(dispatches),
				}
				if dispatch.Path == "" {
					result.RuntimeRows = nil
				}
				if err != nil {
					result.Status = "failed"
					return result, err
				}
				if listErr != nil {
					result.Status = "failed"
					return result, listErr
				}
				return result, nil
			case "execute-report-projection-dispatch":
				dispatchPath := strings.TrimSpace(action.ReportDispatchPath)
				if dispatchPath == "" {
					dispatchPath = strings.TrimSpace(action.DispatchPath)
				}
				if dispatchPath == "" || dispatchPath == "-" {
					return tui.RuntimeActionExecutionResult{}, fmt.Errorf("workflow status report projection dispatch execution runtime action is missing report dispatch")
				}
				execution, err := ExecuteReportProjectionDispatch(context.Background(), root, dispatchPath, ReportProjectionDispatchExecutionOptions{
					Execute: true,
					Actor:   "tui",
					Reason:  "executed from TUI workflow status",
				})
				executions, listErr := ListReportProjectionDispatchExecutions(root)
				selection, selectionFound, readErr := ReadReportProjectionDispatchExecutionSelection(root)
				result := tui.RuntimeActionExecutionResult{
					Status:      "completed",
					Path:        execution.Path,
					Output:      "report projection dispatch executed",
					RuntimeRows: reportProjectionDispatchExecutionRuntimeRowsWithSelection(executions, selection, selectionFound),
				}
				if execution.Path == "" {
					result.RuntimeRows = nil
				}
				if err != nil {
					result.Status = "failed"
					return result, err
				}
				if listErr != nil {
					result.Status = "failed"
					return result, listErr
				}
				if readErr != nil {
					result.Status = "failed"
					return result, readErr
				}
				return result, nil
			case "review-report-projection-execution":
				executionPath := strings.TrimSpace(action.ReportExecutionPath)
				if executionPath == "" || executionPath == "-" {
					return tui.RuntimeActionExecutionResult{}, fmt.Errorf("workflow status report projection execution review runtime action is missing report execution")
				}
				execution, err := AnnotateReportProjectionDispatchExecution(root, executionPath, ReportProjectionDispatchExecutionAnnotationOptions{
					Decision: "accepted",
					Reviewer: "tui",
					Reason:   "reviewed from TUI workflow status",
				})
				executions, listErr := ListReportProjectionDispatchExecutions(root)
				selection, selectionFound, readErr := ReadReportProjectionDispatchExecutionSelection(root)
				result := tui.RuntimeActionExecutionResult{
					Status:      "completed",
					Path:        execution.Path,
					Output:      "report projection execution reviewed",
					RuntimeRows: reportProjectionDispatchExecutionRuntimeRowsWithSelection(executions, selection, selectionFound),
				}
				if execution.Path == "" {
					result.RuntimeRows = nil
				}
				if err != nil {
					result.Status = "failed"
					return result, err
				}
				if listErr != nil {
					result.Status = "failed"
					return result, listErr
				}
				if readErr != nil {
					result.Status = "failed"
					return result, readErr
				}
				return result, nil
			case "select-report-projection-execution":
				executionPath := strings.TrimSpace(action.ReportExecutionPath)
				if executionPath == "" || executionPath == "-" {
					return tui.RuntimeActionExecutionResult{}, fmt.Errorf("workflow status report projection execution selection runtime action is missing report execution")
				}
				selection, err := SelectReportProjectionDispatchExecution(root, executionPath, ReportProjectionDispatchExecutionSelectionOptions{
					SelectedBy: "tui",
					Reason:     "selected from TUI workflow status",
				})
				executions, listErr := ListReportProjectionDispatchExecutions(root)
				refreshedSelection, found, readErr := ReadReportProjectionDispatchExecutionSelection(root)
				if found {
					selection = refreshedSelection
				}
				result := tui.RuntimeActionExecutionResult{
					Status:      "completed",
					Path:        selection.Path,
					Output:      "report projection execution selected",
					RuntimeRows: reportProjectionDispatchExecutionRuntimeRowsWithSelection(executions, selection, true),
				}
				if selection.Path == "" {
					result.RuntimeRows = nil
				}
				if err != nil {
					result.Status = "failed"
					return result, err
				}
				if listErr != nil {
					result.Status = "failed"
					return result, listErr
				}
				if readErr != nil {
					result.Status = "failed"
					return result, readErr
				}
				return result, nil
			default:
				return tui.RuntimeActionExecutionResult{}, fmt.Errorf("workflow status runtime action is not executable for next gate: %s", action.RequestPath)
			}
		}
		if kind == "report-projection-dispatch-artifact" {
			dispatchPath := strings.TrimSpace(action.ArtifactPath)
			if dispatchPath == "" {
				return tui.RuntimeActionExecutionResult{}, fmt.Errorf("report projection dispatch artifact runtime action is missing artifact path")
			}
			execution, err := ExecuteReportProjectionDispatch(context.Background(), root, dispatchPath, ReportProjectionDispatchExecutionOptions{
				Execute: true,
				Actor:   "tui",
				Reason:  "executed from TUI",
			})
			executions, listErr := ListReportProjectionDispatchExecutions(root)
			selection, selectionFound, readErr := ReadReportProjectionDispatchExecutionSelection(root)
			result := tui.RuntimeActionExecutionResult{
				Status:      "completed",
				Path:        execution.Path,
				Output:      "report projection dispatch executed",
				RuntimeRows: reportProjectionDispatchExecutionRuntimeRowsWithSelection(executions, selection, selectionFound),
			}
			if execution.Path == "" {
				result.RuntimeRows = nil
			}
			if err != nil {
				result.Status = "failed"
				return result, err
			}
			if listErr != nil {
				result.Status = "failed"
				return result, listErr
			}
			if readErr != nil {
				result.Status = "failed"
				return result, readErr
			}
			return result, nil
		}
		if kind == "report-projection-dispatch-execution-artifact" {
			executionPath := strings.TrimSpace(action.ArtifactPath)
			if executionPath == "" {
				return tui.RuntimeActionExecutionResult{}, fmt.Errorf("report projection dispatch execution artifact runtime action is missing artifact path")
			}
			selection, err := SelectReportProjectionDispatchExecution(root, executionPath, ReportProjectionDispatchExecutionSelectionOptions{
				SelectedBy: "tui",
				Reason:     "selected from TUI",
			})
			executions, listErr := ListReportProjectionDispatchExecutions(root)
			refreshedSelection, found, readErr := ReadReportProjectionDispatchExecutionSelection(root)
			if found {
				selection = refreshedSelection
			}
			result := tui.RuntimeActionExecutionResult{
				Status:      "completed",
				Path:        selection.Path,
				Output:      "report projection execution selected",
				RuntimeRows: reportProjectionDispatchExecutionRuntimeRowsWithSelection(executions, selection, true),
			}
			if selection.Path == "" {
				result.RuntimeRows = nil
			}
			if err != nil {
				result.Status = "failed"
				return result, err
			}
			if listErr != nil {
				result.Status = "failed"
				return result, listErr
			}
			if readErr != nil {
				result.Status = "failed"
				return result, readErr
			}
			return result, nil
		}
		if kind == "report-projection-execution-selection" {
			selection, err := ClearReportProjectionDispatchExecutionSelection(root, ReportProjectionDispatchExecutionSelectionClearOptions{
				ClearedBy: "tui",
				Reason:    "cleared from TUI",
			})
			executions, listErr := ListReportProjectionDispatchExecutions(root)
			refreshedSelection, found, readErr := ReadReportProjectionDispatchExecutionSelection(root)
			if found {
				selection = refreshedSelection
			}
			result := tui.RuntimeActionExecutionResult{
				Status:      "completed",
				Path:        selection.Path,
				Output:      "report projection selection cleared",
				RuntimeRows: reportProjectionDispatchExecutionRuntimeRowsWithSelection(executions, selection, true),
			}
			if selection.Path == "" {
				result.RuntimeRows = nil
			}
			if err != nil {
				result.Status = "failed"
				return result, err
			}
			if listErr != nil {
				result.Status = "failed"
				return result, listErr
			}
			if readErr != nil {
				result.Status = "failed"
				return result, readErr
			}
			return result, nil
		}
		if kind == "worker-lifecycle-action-plan-artifact" {
			planPath := strings.TrimSpace(action.ArtifactPath)
			if planPath == "" {
				return tui.RuntimeActionExecutionResult{}, fmt.Errorf("worker lifecycle action plan artifact runtime action is missing artifact path")
			}
			selection, err := SelectWorkerLifecycleActionPlan(root, planPath, WorkerLifecycleActionPlanSelectionOptions{
				SelectedBy: "tui",
				Reason:     "selected from TUI",
			})
			plans, listErr := ListWorkerLifecycleActionPlans(root)
			refreshedSelection, found, readErr := ReadWorkerLifecycleActionPlanSelection(root)
			if found {
				selection = refreshedSelection
			}
			result := tui.RuntimeActionExecutionResult{
				Status:      "completed",
				Path:        selection.Path,
				Output:      "worker lifecycle action plan selected",
				RuntimeRows: workerLifecycleActionPlanArtifactRuntimeRowsWithSelection(plans, selection, true),
			}
			if selection.Path == "" {
				result.RuntimeRows = nil
			}
			if err != nil {
				result.Status = "failed"
				return result, err
			}
			if listErr != nil {
				result.Status = "failed"
				return result, listErr
			}
			if readErr != nil {
				result.Status = "failed"
				return result, readErr
			}
			return result, nil
		}
		if kind == "worker-lifecycle-action-plan-selection" {
			selection, err := ClearWorkerLifecycleActionPlanSelection(root, WorkerLifecycleActionPlanSelectionClearOptions{
				ClearedBy: "tui",
				Reason:    "cleared from TUI",
			})
			plans, listErr := ListWorkerLifecycleActionPlans(root)
			refreshedSelection, found, readErr := ReadWorkerLifecycleActionPlanSelection(root)
			if found {
				selection = refreshedSelection
			}
			result := tui.RuntimeActionExecutionResult{
				Status:      "completed",
				Path:        selection.Path,
				Output:      "worker lifecycle action plan selection cleared",
				RuntimeRows: workerLifecycleActionPlanArtifactRuntimeRowsWithSelection(plans, selection, true),
			}
			if selection.Path == "" {
				result.RuntimeRows = nil
			}
			if err != nil {
				result.Status = "failed"
				return result, err
			}
			if listErr != nil {
				result.Status = "failed"
				return result, listErr
			}
			if readErr != nil {
				result.Status = "failed"
				return result, readErr
			}
			return result, nil
		}
		dispatchPath := strings.TrimSpace(action.ArtifactPath)
		if dispatchPath == "" {
			return tui.RuntimeActionExecutionResult{}, fmt.Errorf("%s runtime action is missing artifact path", kind)
		}
		if kind == "worker-transport-daemon-artifact" {
			workerID := strings.TrimSpace(action.Name)
			if workerID == "" {
				return tui.RuntimeActionExecutionResult{}, fmt.Errorf("worker transport daemon runtime action is missing worker metadata")
			}
			controlAction := strings.TrimSpace(action.ControlAction)
			if controlAction == "" {
				controlAction = "stop"
			}
			if _, err := normalizeRuntimeWorkerTransportDaemonControlAction(controlAction); err != nil {
				return tui.RuntimeActionExecutionResult{}, err
			}
			control, err := CreateRuntimeWorkerTransportDaemonControlRequest(root, workerID, RuntimeWorkerTransportDaemonControlRequest{
				Action:      controlAction,
				Reason:      "TUI " + controlAction + " requested for daemon run " + dispatchPath,
				RequestedBy: "tui",
			}, RuntimeWorkerTransportDaemonControlRequestOptions{Now: time.Now().UTC()})
			output := controlAction + " requested"
			result := tui.RuntimeActionExecutionResult{
				Status:      control.Status,
				Path:        control.Path,
				Output:      output,
				RuntimeRows: runtimeWorkerTransportDaemonControlRows([]RuntimeWorkerTransportDaemonControlRequest{control}),
			}
			if result.Status == "" {
				result.Status = "failed"
			}
			if control.Path == "" {
				result.RuntimeRows = nil
			}
			if err != nil {
				return result, err
			}
			return result, nil
		}
		if kind == "worker-transport-request-artifact" {
			workerID := strings.TrimSpace(action.Name)
			if workerID == "" {
				return tui.RuntimeActionExecutionResult{}, fmt.Errorf("worker transport request runtime action is missing worker metadata")
			}
			currentRequest, err := ReadRuntimeWorkerTransportDispatchRequest(root, workerID, dispatchPath)
			if err != nil {
				return tui.RuntimeActionExecutionResult{}, err
			}
			if strings.TrimSpace(currentRequest.Status) == "acknowledged" {
				result, err := ExecuteRuntimeWorkerTransportDispatchRequest(context.Background(), root, workerID, dispatchPath, RuntimeWorkerTransportRunOptions{
					Execute:           true,
					Now:               time.Now().UTC(),
					UseRequestCommand: true,
				})
				refreshedRequest, readErr := ReadRuntimeWorkerTransportDispatchRequest(root, workerID, dispatchPath)
				rows := []string{}
				if readErr == nil {
					rows = append(rows, runtimeWorkerTransportDispatchRequestRows([]RuntimeWorkerTransportDispatchRequest{refreshedRequest})...)
				}
				if strings.TrimSpace(result.Path) != "" {
					rows = append(rows, runtimeWorkerQueueDispatchResultRows([]RuntimeWorkerQueueDispatchResult{result})...)
				}
				status := result.Status
				if status == "" {
					status = "failed"
				}
				path := currentRequest.Path
				if strings.TrimSpace(path) == "" {
					path = result.Path
				}
				executionResult := tui.RuntimeActionExecutionResult{
					Status:      status,
					Path:        path,
					Output:      result.Output,
					RuntimeRows: rows,
				}
				if err != nil {
					return executionResult, err
				}
				return executionResult, nil
			}
			request, err := AcknowledgeRuntimeWorkerTransportDispatchRequest(root, workerID, dispatchPath, RuntimeWorkerTransportAckOptions{
				Ack: true,
				Now: time.Now().UTC(),
			})
			result := tui.RuntimeActionExecutionResult{
				Status:      request.Status,
				Path:        request.Path,
				Output:      "transport request acknowledged",
				RuntimeRows: runtimeWorkerTransportDispatchRequestRows([]RuntimeWorkerTransportDispatchRequest{request}),
			}
			if result.Status == "" {
				result.Status = "failed"
			}
			if request.Path == "" {
				result.RuntimeRows = nil
			}
			if err != nil {
				return result, err
			}
			return result, nil
		}
		if kind == "worker-queue-claim-artifact" {
			if strings.TrimSpace(action.ControlAction) == "expire" {
				claim, err := ExpireRuntimeWorkerQueueClaim(root, dispatchPath, RuntimeWorkerQueueClaimExpireOptions{
					Expire:    true,
					Now:       time.Now().UTC(),
					ExpiredBy: "tui",
					Reason:    "TUI expire requested for queue claim " + dispatchPath,
				})
				result := tui.RuntimeActionExecutionResult{
					Status:      claim.Status,
					Path:        claim.Path,
					Output:      "queue claim expired",
					RuntimeRows: runtimeWorkerQueueClaimRows([]RuntimeWorkerQueueClaim{claim}),
				}
				if result.Status == "" {
					result.Status = "failed"
				}
				if claim.Path == "" {
					result.RuntimeRows = nil
				}
				if err != nil {
					return result, err
				}
				return result, nil
			}
			if strings.TrimSpace(action.ControlAction) == "renew" {
				currentClaim, readErr := ReadRuntimeWorkerQueueClaim(root, dispatchPath)
				leaseDuration := time.Duration(currentClaim.LeaseSeconds) * time.Second
				if leaseDuration <= 0 {
					leaseDuration = DefaultRuntimeWorkerQueueClaimLeaseDuration
				}
				claim, renewErr := RenewRuntimeWorkerQueueClaim(root, dispatchPath, RuntimeWorkerQueueClaimRenewOptions{
					Renew:         true,
					Now:           time.Now().UTC(),
					LeaseDuration: leaseDuration,
					RenewedBy:     "tui",
					Reason:        "TUI renew requested for queue claim " + dispatchPath,
				})
				result := tui.RuntimeActionExecutionResult{
					Status:      "renewed",
					Path:        claim.Path,
					Output:      "queue claim renewed",
					RuntimeRows: runtimeWorkerQueueClaimRows([]RuntimeWorkerQueueClaim{claim}),
				}
				if result.Status == "" {
					result.Status = "failed"
				}
				if claim.Path == "" {
					result.RuntimeRows = nil
				}
				if readErr != nil {
					result.Status = "failed"
					result.Output = compactRuntimeRowValue(readErr.Error())
					return result, readErr
				}
				if renewErr != nil {
					result.Status = "failed"
					result.Output = compactRuntimeRowValue(renewErr.Error())
					return result, renewErr
				}
				return result, nil
			}
			if strings.TrimSpace(action.ControlAction) == "cancel" {
				claim, err := CancelRuntimeWorkerQueueClaim(root, dispatchPath, RuntimeWorkerQueueClaimCancelOptions{
					Cancel:     true,
					Now:        time.Now().UTC(),
					CanceledBy: "tui",
					Reason:     "TUI cancel requested for queue claim " + dispatchPath,
				})
				result := tui.RuntimeActionExecutionResult{
					Status:      claim.Status,
					Path:        claim.Path,
					Output:      "queue claim canceled",
					RuntimeRows: runtimeWorkerQueueClaimRows([]RuntimeWorkerQueueClaim{claim}),
				}
				if result.Status == "" {
					result.Status = "failed"
				}
				if claim.Path == "" {
					result.RuntimeRows = nil
				}
				if err != nil {
					return result, err
				}
				return result, nil
			}
			execution, err := ExecuteRuntimeWorkerQueueClaimDispatch(root, dispatchPath, RuntimeWorkerQueueDispatchExecutionOptions{Execute: true})
			result := tui.RuntimeActionExecutionResult{
				Status:      execution.Status,
				Path:        execution.Path,
				Output:      compactRuntimeRowValue(execution.SkipReason),
				RuntimeRows: runtimeWorkerQueueDispatchExecutionRows([]RuntimeWorkerQueueDispatchExecution{execution}),
			}
			if result.Status == "" {
				result.Status = "failed"
			}
			if execution.Path == "" {
				result.RuntimeRows = nil
			}
			if err != nil {
				return result, err
			}
			return result, nil
		}
		if kind == "worker-placement-artifact" {
			if runtimeActionReadyCount(action) > 1 {
				plan, readErr := ReadRuntimeWorkerPlacementPlan(root, dispatchPath)
				rows := []string{}
				var executions []RuntimeWorkerPlacementClaimExecution
				var claims []RuntimeWorkerQueueClaim
				var firstErr error
				now := time.Now().UTC()
				for _, candidate := range plan.Candidates {
					if strings.TrimSpace(candidate.Status) != "ready" {
						continue
					}
					execution, err := ExecuteRuntimeWorkerPlacementClaim(root, dispatchPath, RuntimeWorkerPlacementClaimExecutionOptions{
						Execute:              true,
						CandidateExecutionID: candidate.ExecutionID,
						WorkerID:             candidate.WorkerID,
						Now:                  now,
					})
					if err != nil && firstErr == nil {
						firstErr = err
					}
					executions = append(executions, execution)
					if strings.TrimSpace(execution.ClaimPath) == "" {
						continue
					}
					if claim, claimErr := ReadRuntimeWorkerQueueClaim(root, execution.ClaimPath); claimErr == nil {
						claims = append(claims, claim)
					}
				}
				if len(executions) > 0 {
					rows = append(rows, runtimeWorkerPlacementClaimExecutionRows(executions)...)
				}
				if len(claims) > 0 {
					rows = append(rows, runtimeWorkerQueueClaimRows(claims)...)
				}
				output := "queue claims created"
				if firstErr != nil {
					output = compactRuntimeRowValue(firstErr.Error())
				}
				result := tui.RuntimeActionExecutionResult{
					Status:      "claimed",
					Path:        dispatchPath,
					Output:      output,
					RuntimeRows: rows,
				}
				if readErr != nil {
					result.Status = "failed"
					result.Output = compactRuntimeRowValue(readErr.Error())
					return result, readErr
				}
				if firstErr != nil {
					result.Status = "failed"
					return result, firstErr
				}
				if len(claims) == 0 {
					result.Status = "skipped"
					result.Output = "no ready placement candidates"
				}
				return result, nil
			}
			execution, err := ExecuteRuntimeWorkerPlacementClaim(root, dispatchPath, RuntimeWorkerPlacementClaimExecutionOptions{Execute: true})
			rows := runtimeWorkerPlacementClaimExecutionRows([]RuntimeWorkerPlacementClaimExecution{execution})
			if strings.TrimSpace(execution.ClaimPath) != "" {
				if claim, readErr := ReadRuntimeWorkerQueueClaim(root, execution.ClaimPath); readErr == nil {
					rows = append(rows, runtimeWorkerQueueClaimRows([]RuntimeWorkerQueueClaim{claim})...)
				}
			}
			output := compactRuntimeRowValue(execution.SkipReason)
			if strings.TrimSpace(output) == "" && strings.TrimSpace(execution.ClaimPath) != "" {
				output = "queue claim created"
			}
			result := tui.RuntimeActionExecutionResult{
				Status:      execution.Status,
				Path:        execution.Path,
				Output:      output,
				RuntimeRows: rows,
			}
			if result.Status == "" {
				result.Status = "failed"
			}
			if execution.Path == "" {
				result.RuntimeRows = nil
			}
			if err != nil {
				return result, err
			}
			return result, nil
		}
		if kind == "worker-lease-reclaim-artifact" {
			execution, err := ExecuteWorkerLeaseReclaimDispatch(root, dispatchPath, WorkerLeaseReclaimExecutionOptions{Execute: true})
			result := tui.RuntimeActionExecutionResult{
				Status:      execution.Status,
				Path:        execution.Path,
				Output:      compactWorkerLeaseReclaimExecutionOutput(execution),
				RuntimeRows: workerLeaseReclaimExecutionRuntimeRows([]WorkerLeaseReclaimExecution{execution}),
			}
			if result.Status == "" {
				result.Status = "failed"
			}
			if execution.Path == "" && len(execution.Results) == 0 && len(execution.Skipped) == 0 {
				result.RuntimeRows = nil
			}
			if err != nil {
				return result, err
			}
			return result, nil
		}
		if kind == "worker-lease-result-reconciliation-artifact" {
			execution, err := ExecuteWorkerLeaseResultReconciliationDispatch(root, dispatchPath, WorkerLeaseResultReconciliationExecutionOptions{
				Execute:    true,
				Now:        time.Now().UTC(),
				ReleasedBy: "tui",
				Reason:     "TUI reviewed worker lease/result reconciliation",
			})
			result := tui.RuntimeActionExecutionResult{
				Status:      execution.Status,
				Path:        execution.Path,
				Output:      compactWorkerLeaseResultReconciliationExecutionOutput(execution),
				RuntimeRows: workerLeaseResultReconciliationExecutionRuntimeRows([]WorkerLeaseResultReconciliationExecution{execution}),
			}
			if result.Status == "" {
				result.Status = "failed"
			}
			if execution.Path == "" && len(execution.Results) == 0 && len(execution.Skipped) == 0 {
				result.RuntimeRows = nil
			}
			if err != nil {
				return result, err
			}
			return result, nil
		}
		if kind == "worker-placement-dispatch-artifact" {
			if runtimeActionReadyCount(action) > 1 {
				batch, err := ExecuteRuntimeWorkerPlacementDispatchBatch(root, dispatchPath, RuntimeWorkerPlacementDispatchBatchOptions{Execute: true})
				rows := runtimeWorkerPlacementDispatchBatchRows([]RuntimeWorkerPlacementDispatchBatch{batch})
				rows = append(rows, runtimeWorkerPlacementDispatchExecutionRows(batch.Executions)...)
				var queueDispatches []RuntimeWorkerQueueDispatchExecution
				for _, execution := range batch.Executions {
					if strings.TrimSpace(execution.QueueDispatchExecutionPath) == "" {
						continue
					}
					queueDispatch, readErr := ReadRuntimeWorkerQueueDispatchExecution(root, execution.QueueDispatchExecutionPath)
					if readErr == nil {
						queueDispatches = append(queueDispatches, queueDispatch)
					}
				}
				if len(queueDispatches) > 0 {
					rows = append(rows, runtimeWorkerQueueDispatchExecutionRows(queueDispatches)...)
				}
				output := compactRuntimeRowValue(batch.Error)
				if strings.TrimSpace(output) == "" && batch.DispatchedCount > 0 {
					output = "queue dispatch batch created"
				}
				result := tui.RuntimeActionExecutionResult{
					Status:      batch.Status,
					Path:        batch.Path,
					Output:      output,
					RuntimeRows: rows,
				}
				if result.Status == "" {
					result.Status = "failed"
				}
				if batch.Path == "" {
					result.RuntimeRows = nil
				}
				if err != nil {
					return result, err
				}
				return result, nil
			}
			execution, err := ExecuteRuntimeWorkerPlacementDispatch(root, dispatchPath, RuntimeWorkerPlacementDispatchExecutionOptions{Execute: true})
			rows := runtimeWorkerPlacementDispatchExecutionRows([]RuntimeWorkerPlacementDispatchExecution{execution})
			if strings.TrimSpace(execution.QueueDispatchExecutionPath) != "" {
				if queueDispatch, readErr := ReadRuntimeWorkerQueueDispatchExecution(root, execution.QueueDispatchExecutionPath); readErr == nil {
					rows = append(rows, runtimeWorkerQueueDispatchExecutionRows([]RuntimeWorkerQueueDispatchExecution{queueDispatch})...)
				}
			}
			output := compactRuntimeRowValue(execution.SkipReason)
			if strings.TrimSpace(output) == "" && strings.TrimSpace(execution.QueueDispatchExecutionPath) != "" {
				output = "queue dispatch created"
			}
			result := tui.RuntimeActionExecutionResult{
				Status:      execution.Status,
				Path:        execution.Path,
				Output:      output,
				RuntimeRows: rows,
			}
			if result.Status == "" {
				result.Status = "failed"
			}
			if execution.Path == "" {
				result.RuntimeRows = nil
			}
			if err != nil {
				return result, err
			}
			return result, nil
		}
		if kind == "install-retry-dispatch-artifact" {
			execution, err := ExecuteToolchainInstallRetryDispatch(root, dispatchPath, ToolchainInstallRetryDispatchExecutionOptions{Execute: true})
			result := tui.RuntimeActionExecutionResult{
				Status:      execution.Status,
				Path:        execution.Path,
				Output:      compactRetryExecutionOutput(execution),
				RuntimeRows: installRetryExecutionArtifactRuntimeRows([]ToolchainInstallRetryDispatchExecution{execution}),
			}
			if result.Status == "" {
				result.Status = "failed"
			}
			if execution.Path == "" && len(execution.Results) == 0 {
				result.RuntimeRows = nil
			}
			if err != nil {
				return result, err
			}
			return result, nil
		}
		execution, err := ExecuteToolchainInstallDispatch(root, dispatchPath, ToolchainInstallDispatchExecutionOptions{Execute: true})
		result := tui.RuntimeActionExecutionResult{
			Status:      execution.Status,
			Path:        execution.Path,
			Output:      compactExecutionOutput(execution),
			RuntimeRows: installExecutionArtifactRuntimeRows([]ToolchainInstallDispatchExecution{execution}),
		}
		if result.Status == "" {
			result.Status = "failed"
		}
		if execution.Path == "" && len(execution.Results) == 0 {
			result.RuntimeRows = nil
		}
		if err != nil {
			return result, err
		}
		return result, nil
	}
}

func runtimeActionReadyCount(action tui.RuntimeAction) int {
	ready, err := strconv.Atoi(strings.TrimSpace(action.Ready))
	if err != nil {
		return 0
	}
	return ready
}

func baseTUIModelForWorkspace(root string) tui.Model {
	return tui.NewModel().
		WithRuntimeActionRequester(tuiRuntimeActionRequester(root)).
		WithRuntimeActionExecutor(tuiRuntimeActionExecutor(root)).
		WithAIDialogSubmitter(tuiAIDialogSubmitter(root))
}

const tuiAIDialogSystemPrompt = "You are Ouroboros, a TUI-first IDE assistant for authorized security work. Help the operator reason about scoped objectives, evidence, toolchain state, and safe next gates. Do not turn model output into command execution; recommend operator-reviewed actions through existing Ouroboros gates. If a tool-plan preview is warranted, emit at most one marker line in the declared marker format using an allowed semantic action from the workspace context; otherwise answer normally."

func tuiAIDialogSystemPromptForWorkspace(root string) string {
	context := strings.TrimSpace(tuiAIDialogWorkspaceContext(root))
	if context == "" {
		return tuiAIDialogSystemPrompt
	}
	return tuiAIDialogSystemPrompt + "\n\nWorkspace context:\n" + context
}

func tuiAIDialogWorkspaceContext(root string) string {
	status, err := SummarizeWorkspaceWorkflow(root)
	if err != nil {
		return "workspace.context_status=unavailable\nworkspace.context_error=workflow_status_unavailable"
	}
	action := workflowStatusRowValue(string(status.Action), "none")
	safetyErrorCount := status.SafetyErrorCount
	if safetyErrorCount == 0 && len(status.SafetyErrors) > 0 {
		safetyErrorCount = len(status.SafetyErrors)
	}
	lines := []string{
		"workspace.context_status=available",
		"workspace.context_mode=read-only",
		"workflow.mode=" + workflowStatusRowValue(status.Mode, "read-only"),
		"workflow.status=" + workflowStatusRowValue(status.Status, "unknown"),
		"workflow.stage=" + workflowStatusRowValue(status.Stage, "unknown"),
		"workflow.next_gate=" + workflowStatusRowValue(status.NextGate, "none"),
		"workflow.reason=" + workflowStatusRowValue(status.Reason, "none"),
		"workflow.action=" + action,
		fmt.Sprintf("workflow.safety_errors=%d", safetyErrorCount),
		fmt.Sprintf(
			"workflow.counts previews=%d acceptances=%d compile_dispatches=%d compile_executions=%d report_dispatches=%d report_executions=%d",
			status.PreviewCount,
			status.AcceptanceCount,
			status.DispatchCount,
			status.ExecutionCount,
			status.ReportDispatchCount,
			status.ReportExecutionCount,
		),
	}
	lines = append(lines, tuiAIDialogSemanticActionContext()...)
	lines = append(lines, tuiAIDialogWorkspaceOverviewContext(root)...)
	return strings.Join(lines, "\n")
}

func tuiAIDialogWorkspaceOverviewContext(root string) []string {
	summary, err := tuiAIDialogOverviewSummary(root)
	if err != nil {
		return []string{
			"overview.status=unavailable",
			"overview.error=overview_unavailable",
		}
	}
	return []string{
		"overview.status=available",
		"overview.workflow_status=" + workflowStatusRowValue(summary.WorkflowStatus, "unknown"),
		"overview.stage=" + workflowStatusRowValue(summary.Stage, "unknown"),
		"overview.next_gate=" + workflowStatusRowValue(summary.NextGate, "none"),
		fmt.Sprintf("overview.mission_configured=%t", summary.MissionConfigured),
		"overview.target_policy=" + workflowStatusRowValue(summary.TargetPolicy, "unknown"),
		"overview.isolation=" + workflowStatusRowValue(summary.Isolation, "unknown"),
		fmt.Sprintf("overview.targets=%d", summary.Targets),
		fmt.Sprintf("overview.safety_ready=%t", summary.SafetyReady),
		fmt.Sprintf("overview.strict_ready=%t", summary.StrictReady),
		"overview.providers_status=" + workflowStatusRowValue(summary.ProvidersStatus, "unknown"),
		fmt.Sprintf("overview.providers=%d", summary.Providers),
		fmt.Sprintf("overview.provider_enabled=%d", summary.ProviderEnabled),
		fmt.Sprintf("overview.chat=%t", summary.Chat),
		fmt.Sprintf("overview.tool_planning=%t", summary.ToolPlanning),
		"overview.capabilities_status=" + workflowStatusRowValue(summary.CapabilitiesStatus, "blocked"),
		fmt.Sprintf("overview.domain_packs=%d", summary.DomainPacks),
		fmt.Sprintf("overview.tools=%d", summary.Tools),
		fmt.Sprintf("overview.evidence_runs=%d", summary.EvidenceRuns),
		fmt.Sprintf("overview.findings=%d", summary.Findings),
		fmt.Sprintf("overview.reports_current=%t", summary.ReportsCurrent),
		fmt.Sprintf("overview.workers_registered=%d", summary.WorkersRegistered),
		fmt.Sprintf("overview.worker_queue_claims=%d", summary.WorkerQueueClaims),
	}
}

func tuiAIDialogSemanticActionContext() []string {
	allowed := tuiAIDialogAllowedSemanticActionNames()
	if len(allowed) == 0 {
		return []string{
			"semantic_actions.status=unavailable",
			"semantic_actions.error=no_declared_actions",
		}
	}
	return []string{
		"semantic_actions.status=available",
		fmt.Sprintf("semantic_actions.count=%d", len(allowed)),
		"semantic_actions.allowed=" + strings.Join(allowed, ","),
		"semantic_actions.marker_format=semantic_action:<action>",
		"semantic_actions.mode=preview_only",
		"semantic_actions.acceptance=operator_review_required",
	}
}

func tuiAIDialogAllowedSemanticActionNames() []string {
	actions := compiler.DeclaredSemanticActions()
	allowed := make([]string, 0, len(actions))
	for _, action := range actions {
		value := strings.TrimSpace(string(action))
		if value == "" {
			continue
		}
		allowed = append(allowed, value)
	}
	return allowed
}

func tuiAIDialogSemanticActionCommandText() string {
	allowed := tuiAIDialogAllowedSemanticActionNames()
	if len(allowed) == 0 {
		return "semantic actions: status=unavailable error=no_declared_actions stored=false"
	}
	return fmt.Sprintf(
		"semantic actions: status=available count=%d allowed=%s marker_format=semantic_action:<action> mode=preview_only acceptance=operator_review_required stored=false",
		len(allowed),
		strings.Join(allowed, ","),
	)
}

type tuiAIDialogModelPreviewSummary struct {
	Previews          int
	Valid             int
	Completed         int
	Acceptances       int
	PendingAcceptance int
	LatestAction      string
	LatestStatus      string
	LatestValid       bool
	ReviewAvailable   bool
}

func tuiAIDialogModelPreviewSummaryForWorkspace(root string) (tuiAIDialogModelPreviewSummary, error) {
	previews, err := ListModelToolPlanPreviewArtifacts(root)
	if err != nil {
		return tuiAIDialogModelPreviewSummary{}, err
	}
	acceptances, err := ListModelToolPlanPreviewAcceptances(root)
	if err != nil {
		return tuiAIDialogModelPreviewSummary{}, err
	}
	accepted := map[string]struct{}{}
	for _, acceptance := range acceptances {
		path := strings.TrimSpace(acceptance.PreviewArtifactPath)
		if path == "" {
			continue
		}
		accepted[filepath.Clean(path)] = struct{}{}
		accepted[filepath.Base(path)] = struct{}{}
	}
	summary := tuiAIDialogModelPreviewSummary{
		Previews:     len(previews),
		Acceptances:  len(acceptances),
		LatestAction: "none",
		LatestStatus: "none",
	}
	for _, preview := range previews {
		if preview.Valid {
			summary.Valid++
		}
		if strings.TrimSpace(preview.Status) == "completed" {
			summary.Completed++
		}
		if preview.Valid && strings.TrimSpace(preview.Status) == "completed" && strings.TrimSpace(preview.ReviewPath) != "" {
			summary.ReviewAvailable = true
			if !tuiAIDialogPreviewAccepted(preview, accepted) {
				summary.PendingAcceptance++
			}
		}
	}
	if len(previews) > 0 {
		latest := previews[len(previews)-1]
		summary.LatestAction = workflowStatusRowValue(string(latest.Action), "none")
		summary.LatestStatus = workflowStatusRowValue(latest.Status, "none")
		summary.LatestValid = latest.Valid
	}
	return summary, nil
}

func tuiAIDialogPreviewAccepted(preview ModelToolPlanPreviewArtifact, accepted map[string]struct{}) bool {
	path := strings.TrimSpace(preview.Path)
	if path == "" {
		return false
	}
	if _, ok := accepted[filepath.Clean(path)]; ok {
		return true
	}
	if _, ok := accepted[filepath.Base(path)]; ok {
		return true
	}
	return false
}

func SubmitWorkspaceAIDialogPrompt(root string, prompt string) (tui.AIDialogResult, error) {
	trimmedPrompt := strings.TrimSpace(prompt)
	if trimmedPrompt == "" {
		return tui.AIDialogResult{}, fmt.Errorf("chat prompt is required")
	}
	return tuiAIDialogSubmitter(root)(tui.AIDialogRequest{Prompt: trimmedPrompt})
}

func tuiAIDialogSubmitter(root string) tui.AIDialogSubmitter {
	return func(request tui.AIDialogRequest) (tui.AIDialogResult, error) {
		if result, handled, err := tuiAIDialogLocalCommand(root, request); handled {
			return result, err
		}
		messages := providerGenerationMessagesForAIDialog(request)
		result, err := GenerateProviderMessage(context.Background(), root, ProviderGenerationRequest{
			SystemPrompt: tuiAIDialogSystemPromptForWorkspace(root),
			UserPrompt:   request.Prompt,
			Messages:     messages,
		})
		if err != nil {
			return tui.AIDialogResult{Status: "failed"}, fmt.Errorf("%s; %s", err.Error(), aiProviderSetupFailureHint(root))
		}
		dialogResult := tui.AIDialogResult{
			Status:         "completed",
			Provider:       result.ProviderName,
			Model:          result.Model,
			Text:           result.Text,
			InputTokens:    result.InputTokens,
			OutputTokens:   result.OutputTokens,
			TotalTokens:    result.TotalTokens,
			MetadataOnly:   true,
			PromptStored:   false,
			ResponseStored: false,
		}
		extraction := ExtractModelToolPlanPreview(result.Text)
		if extraction.Found {
			dialogResult.PreviewStatus = "skipped"
			dialogResult.PreviewAction = string(extraction.Action)
			dialogResult.PreviewMarker = extraction.MarkerPreview
		}
		if extraction.Valid {
			review, err := CreateToolCompilePreviewReview(root, ToolCompilePreviewOptions{Action: extraction.Action})
			if err != nil {
				return tui.AIDialogResult{Status: "failed"}, fmt.Errorf("create AI dialog tool-plan compile preview review: %w", err)
			}
			artifact, err := RecordModelToolPlanPreviewArtifact(root, "tui-ai-dialog", result, "", extraction, review, ModelToolPlanPreviewArtifactOptions{
				Source:  "tui-ai-dialog",
				AgentID: "tui-ai-dialog",
				Role:    "operator-assistant",
			})
			if err != nil {
				return tui.AIDialogResult{Status: "failed"}, err
			}
			dialogResult.PreviewStatus = "created"
			dialogResult.PreviewAction = string(artifact.Action)
			dialogResult.PreviewMarker = artifact.MarkerPreview
			dialogResult.PreviewPath = artifact.Path
			dialogResult.RuntimeRows = modelToolPlanPreviewArtifactRuntimeRows([]ModelToolPlanPreviewArtifact{artifact})
		}
		return dialogResult, nil
	}
}

func tuiAIDialogLocalCommand(root string, request tui.AIDialogRequest) (tui.AIDialogResult, bool, error) {
	switch strings.ToLower(strings.TrimSpace(request.Prompt)) {
	case "/doctor", "/readiness", "/health":
		summary, err := tuiAIDialogDoctorSummary(root)
		if err != nil {
			return tui.AIDialogResult{Status: "failed", MetadataOnly: true, HistoryExcluded: true}, true, fmt.Errorf("doctor: %w", err)
		}
		return tui.AIDialogResult{
			Status: "completed",
			Text: fmt.Sprintf(
				"doctor: workspace_ready=%t strict_ready=%t warnings=%d errors=%d strict_errors=%d safety_bypasses=%d providers_status=%s provider_enabled=%d chat=%t tool_planning=%t workflow_status=%s next_gate=%s toolchain_status=%s tools=%d stored=false",
				summary.WorkspaceReady,
				summary.StrictReady,
				summary.Warnings,
				summary.Errors,
				summary.StrictErrors,
				summary.SafetyBypasses,
				workflowStatusRowValue(summary.ProvidersStatus, "unknown"),
				summary.ProviderEnabled,
				summary.Chat,
				summary.ToolPlanning,
				workflowStatusRowValue(summary.WorkflowStatus, "unknown"),
				workflowStatusRowValue(summary.NextGate, "none"),
				workflowStatusRowValue(summary.ToolchainStatus, "blocked"),
				summary.Tools,
			),
			MetadataOnly:    true,
			PromptStored:    false,
			ResponseStored:  false,
			HistoryExcluded: true,
		}, true, nil
	case "/completion", "/progress", "/percent", "/acceptance", "/completion-status":
		return tui.AIDialogResult{
			Status: "completed",
			Text: "completion: local_ide_usable=true direct_chat=true completion_estimate=99.99 " +
				"claim_100_ready=false final_review_status=external_evidence_required production_acceptance_required=true " +
				"completion_audit_required=true attestation_required=true manifest_review_required=true operator_acceptance_required=true stored=false",
			MetadataOnly:    true,
			PromptStored:    false,
			ResponseStored:  false,
			HistoryExcluded: true,
		}, true, nil
	case "/final", "/final-evidence", "/final-review", "/audit", "/audit-completion", "/production-acceptance":
		return tui.AIDialogResult{
			Status: "completed",
			Text: "final evidence: production_acceptance=make_smoke-production-acceptance " +
				"inventory=<payload-inventory> policy=<deployment-policy> attestation=<completion-attestation> " +
				"completion_audit=make_audit-completion review=manifest_and_digest_sidecar claim_100_after=operator_acceptance stored=false",
			MetadataOnly:    true,
			PromptStored:    false,
			ResponseStored:  false,
			HistoryExcluded: true,
		}, true, nil
	case "/status", "/workflow", "/workflow-status":
		status, err := SummarizeWorkspaceWorkflow(root)
		if err != nil {
			return tui.AIDialogResult{Status: "failed", MetadataOnly: true, HistoryExcluded: true}, true, fmt.Errorf("workflow status: %w", err)
		}
		safetyErrorCount := status.SafetyErrorCount
		if safetyErrorCount == 0 && len(status.SafetyErrors) > 0 {
			safetyErrorCount = len(status.SafetyErrors)
		}
		return tui.AIDialogResult{
			Status: "completed",
			Text: fmt.Sprintf(
				"workflow status: status=%s stage=%s next_gate=%s reason=%s safety_errors=%d stored=false",
				workflowStatusRowValue(status.Status, "unknown"),
				workflowStatusRowValue(status.Stage, "unknown"),
				workflowStatusRowValue(status.NextGate, "none"),
				workflowStatusRowValue(status.Reason, "none"),
				safetyErrorCount,
			),
			MetadataOnly:    true,
			PromptStored:    false,
			ResponseStored:  false,
			HistoryExcluded: true,
		}, true, nil
	case "/next", "/next-gate":
		status, err := SummarizeWorkspaceWorkflow(root)
		if err != nil {
			return tui.AIDialogResult{Status: "failed", MetadataOnly: true, HistoryExcluded: true}, true, fmt.Errorf("workflow next gate: %w", err)
		}
		safetyErrorCount := status.SafetyErrorCount
		if safetyErrorCount == 0 && len(status.SafetyErrors) > 0 {
			safetyErrorCount = len(status.SafetyErrors)
		}
		return tui.AIDialogResult{
			Status: "completed",
			Text: fmt.Sprintf(
				"next gate: status=%s stage=%s next_gate=%s reason=%s action=%s safety_errors=%d command_available=%t stored=false",
				workflowStatusRowValue(status.Status, "unknown"),
				workflowStatusRowValue(status.Stage, "unknown"),
				workflowStatusRowValue(status.NextGate, "none"),
				workflowStatusRowValue(status.Reason, "none"),
				workflowStatusRowValue(string(status.Action), "none"),
				safetyErrorCount,
				strings.TrimSpace(status.Command) != "",
			),
			MetadataOnly:    true,
			PromptStored:    false,
			ResponseStored:  false,
			HistoryExcluded: true,
		}, true, nil
	case "/command", "/next-command", "/workflow-command":
		status, err := SummarizeWorkspaceWorkflow(root)
		if err != nil {
			return tui.AIDialogResult{Status: "failed", MetadataOnly: true, HistoryExcluded: true}, true, fmt.Errorf("workflow command: %w", err)
		}
		return tui.AIDialogResult{
			Status: "completed",
			Text: fmt.Sprintf(
				"next command: status=%s stage=%s next_gate=%s reason=%s command=%s stored=false",
				workflowStatusRowValue(status.Status, "unknown"),
				workflowStatusRowValue(status.Stage, "unknown"),
				workflowStatusRowValue(status.NextGate, "none"),
				workflowStatusRowValue(status.Reason, "none"),
				tuiAIDialogSanitizedWorkflowCommand(root, status.Command),
			),
			MetadataOnly:    true,
			PromptStored:    false,
			ResponseStored:  false,
			HistoryExcluded: true,
		}, true, nil
	case "/artifacts", "/artifact-counts", "/workflow-artifacts":
		status, err := SummarizeWorkspaceWorkflow(root)
		if err != nil {
			return tui.AIDialogResult{Status: "failed", MetadataOnly: true, HistoryExcluded: true}, true, fmt.Errorf("workflow artifacts: %w", err)
		}
		safetyErrorCount := status.SafetyErrorCount
		if safetyErrorCount == 0 && len(status.SafetyErrors) > 0 {
			safetyErrorCount = len(status.SafetyErrors)
		}
		return tui.AIDialogResult{
			Status: "completed",
			Text: fmt.Sprintf(
				"workflow artifacts: status=%s stage=%s previews=%d acceptances=%d compile_dispatches=%d compile_executions=%d report_dispatches=%d report_executions=%d safety_errors=%d stored=false",
				workflowStatusRowValue(status.Status, "unknown"),
				workflowStatusRowValue(status.Stage, "unknown"),
				status.PreviewCount,
				status.AcceptanceCount,
				status.DispatchCount,
				status.ExecutionCount,
				status.ReportDispatchCount,
				status.ReportExecutionCount,
				safetyErrorCount,
			),
			MetadataOnly:    true,
			PromptStored:    false,
			ResponseStored:  false,
			HistoryExcluded: true,
		}, true, nil
	case "/gates", "/review-gates", "/gate-status":
		status, err := SummarizeWorkspaceWorkflow(root)
		if err != nil {
			return tui.AIDialogResult{Status: "failed", MetadataOnly: true, HistoryExcluded: true}, true, fmt.Errorf("review gates: %w", err)
		}
		safetyErrorCount := status.SafetyErrorCount
		if safetyErrorCount == 0 && len(status.SafetyErrors) > 0 {
			safetyErrorCount = len(status.SafetyErrors)
		}
		return tui.AIDialogResult{
			Status: "completed",
			Text: fmt.Sprintf(
				"review gates: status=%s stage=%s next_gate=%s reason=%s action=%s previews=%d acceptances=%d compile_dispatches=%d compile_executions=%d report_dispatches=%d report_executions=%d report_selected=%t safety_errors=%d command_available=%t stored=false",
				workflowStatusRowValue(status.Status, "unknown"),
				workflowStatusRowValue(status.Stage, "unknown"),
				workflowStatusRowValue(status.NextGate, "none"),
				workflowStatusRowValue(status.Reason, "none"),
				workflowStatusRowValue(string(status.Action), "none"),
				status.PreviewCount,
				status.AcceptanceCount,
				status.DispatchCount,
				status.ExecutionCount,
				status.ReportDispatchCount,
				status.ReportExecutionCount,
				strings.TrimSpace(status.ReportSelectionPath) != "",
				safetyErrorCount,
				strings.TrimSpace(status.Command) != "",
			),
			MetadataOnly:    true,
			PromptStored:    false,
			ResponseStored:  false,
			HistoryExcluded: true,
		}, true, nil
	case "/previews", "/tool-plan-previews", "/model-previews":
		summary, err := tuiAIDialogModelPreviewSummaryForWorkspace(root)
		if err != nil {
			return tui.AIDialogResult{Status: "failed", MetadataOnly: true, HistoryExcluded: true}, true, fmt.Errorf("model previews: %w", err)
		}
		return tui.AIDialogResult{
			Status: "completed",
			Text: fmt.Sprintf(
				"model previews: previews=%d valid=%d completed=%d acceptances=%d pending_acceptance=%d latest_action=%s latest_status=%s latest_valid=%t review_available=%t stored=false",
				summary.Previews,
				summary.Valid,
				summary.Completed,
				summary.Acceptances,
				summary.PendingAcceptance,
				workflowStatusRowValue(summary.LatestAction, "none"),
				workflowStatusRowValue(summary.LatestStatus, "none"),
				summary.LatestValid,
				summary.ReviewAvailable,
			),
			MetadataOnly:    true,
			PromptStored:    false,
			ResponseStored:  false,
			HistoryExcluded: true,
		}, true, nil
	case "/runs", "/replay-runs", "/run-history":
		count, latest, err := tuiAIDialogReplayRuns(root)
		if err != nil {
			return tui.AIDialogResult{Status: "failed", MetadataOnly: true, HistoryExcluded: true}, true, fmt.Errorf("replay runs: %w", err)
		}
		return tui.AIDialogResult{
			Status: "completed",
			Text: fmt.Sprintf(
				"replay runs: runs=%d latest=%s stored=false",
				count,
				workflowStatusRowValue(latest, "none"),
			),
			MetadataOnly:    true,
			PromptStored:    false,
			ResponseStored:  false,
			HistoryExcluded: true,
		}, true, nil
	case "/evidence", "/findings", "/graph":
		summary, err := tuiAIDialogSummarizeEvidence(root)
		if err != nil {
			return tui.AIDialogResult{Status: "failed", MetadataOnly: true, HistoryExcluded: true}, true, fmt.Errorf("evidence: %w", err)
		}
		return tui.AIDialogResult{
			Status: "completed",
			Text: fmt.Sprintf(
				"evidence: runs=%d latest_available=%t events=%d facts=%d intents=%d hints=%d nodes=%d edges=%d findings=%d surfaces=%d artifacts=%d verified_findings=%d paths=%d evidence_refs=%d exhausted=%d suggestions=%d stored=false",
				summary.Runs,
				summary.LatestAvailable,
				summary.Events,
				summary.Facts,
				summary.Intents,
				summary.Hints,
				summary.Nodes,
				summary.Edges,
				summary.Findings,
				summary.Surfaces,
				summary.Artifacts,
				summary.VerifiedFindings,
				summary.Paths,
				summary.EvidenceRefs,
				summary.Exhausted,
				summary.Suggestions,
			),
			MetadataOnly:    true,
			PromptStored:    false,
			ResponseStored:  false,
			HistoryExcluded: true,
		}, true, nil
	case "/mission", "/targets", "/target-scope":
		summary, err := tuiAIDialogSummarizeMissionScope(root)
		if err != nil {
			return tui.AIDialogResult{Status: "failed", MetadataOnly: true, HistoryExcluded: true}, true, fmt.Errorf("mission: %w", err)
		}
		return tui.AIDialogResult{
			Status: "completed",
			Text: fmt.Sprintf(
				"mission: configured=%t goal_set=%t target_policy=%s isolation=%s targets=%d urls=%d domains=%d hosts=%d cidrs=%d artifacts=%d challenges=%d exclusions=%d stored=false",
				summary.Configured,
				summary.GoalSet,
				workflowStatusRowValue(summary.TargetPolicy, "unknown"),
				workflowStatusRowValue(summary.Isolation, "unknown"),
				summary.Targets,
				summary.URLs,
				summary.Domains,
				summary.Hosts,
				summary.CIDRs,
				summary.Artifacts,
				summary.Challenges,
				summary.Exclusions,
			),
			MetadataOnly:    true,
			PromptStored:    false,
			ResponseStored:  false,
			HistoryExcluded: true,
		}, true, nil
	case "/reports", "/report-status", "/current-report":
		summary, err := tuiAIDialogSummarizeReports(root)
		if err != nil {
			return tui.AIDialogResult{Status: "failed", MetadataOnly: true, HistoryExcluded: true}, true, fmt.Errorf("reports: %w", err)
		}
		return tui.AIDialogResult{
			Status: "completed",
			Text: fmt.Sprintf(
				"reports: dispatches=%d executions=%d current=%t run=%s events=%d written=%t stored=false",
				summary.Dispatches,
				summary.Executions,
				summary.Current,
				workflowStatusRowValue(summary.RunID, "none"),
				summary.EventCount,
				summary.Written,
			),
			MetadataOnly:    true,
			PromptStored:    false,
			ResponseStored:  false,
			HistoryExcluded: true,
		}, true, nil
	case "/toolchain", "/tools", "/toolchain-contract":
		check := tuiAIDialogToolchainSummary(root)
		return tui.AIDialogResult{
			Status: "completed",
			Text: fmt.Sprintf(
				"toolchain: status=%s manifests=%d tools=%d domain_packs=%d patterns=%d semantic_actions=%d errors=%d stored=false",
				workflowStatusRowValue(check.Status, "blocked"),
				check.ManifestCount,
				check.ToolCount,
				check.DomainPackCount,
				check.PatternCount,
				check.SemanticActionCount,
				len(check.Errors),
			),
			MetadataOnly:    true,
			PromptStored:    false,
			ResponseStored:  false,
			HistoryExcluded: true,
		}, true, nil
	case "/capabilities", "/packs", "/domains":
		summary := tuiAIDialogCapabilitiesSummary(root)
		return tui.AIDialogResult{
			Status: "completed",
			Text: fmt.Sprintf(
				"capabilities: status=%s domains=%d domain_packs=%d pack_capabilities=%d parsers=%d strategies=%d semantic_actions=%d semantic_capabilities=%d manifests=%d manifest_capabilities=%d manifest_dependencies=%d tools=%d tool_capabilities=%d tool_categories=%d target_kinds=%d managed_bundles=%d managed_installed=%d managed_blocked=%d patterns=%d errors=%d stored=false",
				workflowStatusRowValue(summary.Status, "blocked"),
				summary.Domains,
				summary.DomainPacks,
				summary.PackCapabilities,
				summary.Parsers,
				summary.Strategies,
				summary.SemanticActions,
				summary.SemanticCapabilities,
				summary.Manifests,
				summary.ManifestCapabilities,
				summary.ManifestDependencies,
				summary.Tools,
				summary.ToolCapabilities,
				summary.ToolCategories,
				summary.TargetKinds,
				summary.ManagedBundles,
				summary.ManagedInstalled,
				summary.ManagedBlocked,
				summary.Patterns,
				summary.Errors,
			),
			MetadataOnly:    true,
			PromptStored:    false,
			ResponseStored:  false,
			HistoryExcluded: true,
		}, true, nil
	case "/overview", "/summary", "/dashboard":
		summary, err := tuiAIDialogOverviewSummary(root)
		if err != nil {
			return tui.AIDialogResult{Status: "failed", MetadataOnly: true, HistoryExcluded: true}, true, fmt.Errorf("overview: %w", err)
		}
		return tui.AIDialogResult{
			Status: "completed",
			Text: fmt.Sprintf(
				"overview: workflow_status=%s stage=%s next_gate=%s mission_configured=%t target_policy=%s isolation=%s targets=%d safety_ready=%t strict_ready=%t providers_status=%s providers=%d provider_enabled=%d chat=%t tool_planning=%t capabilities_status=%s domain_packs=%d tools=%d evidence_runs=%d findings=%d reports_current=%t workers_registered=%d worker_queue_claims=%d stored=false",
				workflowStatusRowValue(summary.WorkflowStatus, "unknown"),
				workflowStatusRowValue(summary.Stage, "unknown"),
				workflowStatusRowValue(summary.NextGate, "none"),
				summary.MissionConfigured,
				workflowStatusRowValue(summary.TargetPolicy, "unknown"),
				workflowStatusRowValue(summary.Isolation, "unknown"),
				summary.Targets,
				summary.SafetyReady,
				summary.StrictReady,
				workflowStatusRowValue(summary.ProvidersStatus, "unknown"),
				summary.Providers,
				summary.ProviderEnabled,
				summary.Chat,
				summary.ToolPlanning,
				workflowStatusRowValue(summary.CapabilitiesStatus, "blocked"),
				summary.DomainPacks,
				summary.Tools,
				summary.EvidenceRuns,
				summary.Findings,
				summary.ReportsCurrent,
				summary.WorkersRegistered,
				summary.WorkerQueueClaims,
			),
			MetadataOnly:    true,
			PromptStored:    false,
			ResponseStored:  false,
			HistoryExcluded: true,
		}, true, nil
	case "/safety", "/scope", "/authorization":
		summary, err := tuiAIDialogSafetySummary(root)
		if err != nil {
			return tui.AIDialogResult{Status: "failed", MetadataOnly: true, HistoryExcluded: true}, true, fmt.Errorf("safety: %w", err)
		}
		return tui.AIDialogResult{
			Status: "completed",
			Text: fmt.Sprintf(
				"safety: ready=%t strict_ready=%t warnings=%d errors=%d strict_errors=%d bypasses=%d authorized=%d missing=%d expired=%d stored=false",
				summary.Ready,
				summary.StrictReady,
				summary.Warnings,
				summary.Errors,
				summary.StrictErrors,
				summary.Bypasses,
				summary.Authorized,
				summary.Missing,
				summary.Expired,
			),
			MetadataOnly:    true,
			PromptStored:    false,
			ResponseStored:  false,
			HistoryExcluded: true,
		}, true, nil
	case "/providers", "/provider", "/models":
		summary := tuiAIDialogSummarizeProviders(root)
		return tui.AIDialogResult{
			Status: "completed",
			Text: fmt.Sprintf(
				"providers: status=%s reason=%s providers=%d enabled=%d routing=%s first_type=%s first_model=%s chat=%t tool_planning=%t api_key_env_configured=%t api_key_env_set=%t api_key_value_stored=false stored=false",
				workflowStatusRowValue(summary.Status, "unknown"),
				workflowStatusRowValue(summary.Reason, "unknown"),
				summary.ProviderCount,
				summary.EnabledCount,
				workflowStatusRowValue(summary.Routing, "unknown"),
				workflowStatusRowValue(summary.FirstType, "none"),
				workflowStatusRowValue(summary.FirstModel, "none"),
				summary.Chat,
				summary.ToolPlanning,
				summary.APIKeyEnvConfigured,
				summary.APIKeyEnvSet,
			),
			MetadataOnly:    true,
			PromptStored:    false,
			ResponseStored:  false,
			HistoryExcluded: true,
		}, true, nil
	case "/workers", "/agents", "/runtime":
		summary, err := tuiAIDialogSummarizeWorkers(root)
		if err != nil {
			return tui.AIDialogResult{Status: "failed", MetadataOnly: true, HistoryExcluded: true}, true, fmt.Errorf("workers: %w", err)
		}
		return tui.AIDialogResult{
			Status: "completed",
			Text: fmt.Sprintf(
				"workers: registered=%d available=%d busy=%d draining=%d offline=%d stale=%d local=%d remote=%d queue_claims=%d queue_claimed=%d queue_dispatched=%d queue_completed=%d queue_failed=%d dispatch_executions=%d dispatch_dispatched=%d dispatch_completed=%d dispatch_failed=%d transport_requests=%d transport_pending=%d transport_ack=%d transport_completed=%d transport_failed=%d dispatch_results=%d dispatch_results_completed=%d dispatch_results_failed=%d leases=%d active_leases=%d stale_leases=%d completed_leases=%d failed_leases=%d reclaimed_leases=%d stored=false",
				summary.Registered,
				summary.Available,
				summary.Busy,
				summary.Draining,
				summary.Offline,
				summary.Stale,
				summary.Local,
				summary.Remote,
				summary.QueueClaims,
				summary.QueueClaimed,
				summary.QueueDispatched,
				summary.QueueCompleted,
				summary.QueueFailed,
				summary.DispatchExecutions,
				summary.DispatchDispatched,
				summary.DispatchCompleted,
				summary.DispatchFailed,
				summary.TransportRequests,
				summary.TransportPending,
				summary.TransportAck,
				summary.TransportCompleted,
				summary.TransportFailed,
				summary.DispatchResults,
				summary.DispatchResultsCompleted,
				summary.DispatchResultsFailed,
				summary.Leases,
				summary.ActiveLeases,
				summary.StaleLeases,
				summary.CompletedLeases,
				summary.FailedLeases,
				summary.ReclaimedLeases,
			),
			MetadataOnly:    true,
			PromptStored:    false,
			ResponseStored:  false,
			HistoryExcluded: true,
		}, true, nil
	case "/actions", "/semantic-actions", "/action-list":
		return tui.AIDialogResult{
			Status:          "completed",
			Text:            tuiAIDialogSemanticActionCommandText(),
			MetadataOnly:    true,
			PromptStored:    false,
			ResponseStored:  false,
			HistoryExcluded: true,
		}, true, nil
	case "/quickstart", "/use", "/runbook":
		return tui.AIDialogResult{
			Status: "completed",
			Text: "quickstart: init=./bin/ouro init <workspace> chat=./bin/ouro chat <workspace> " +
				"provider=ouro provider configure-openai-compatible <workspace> --name local-openai --base-url <base-url> --model <model> --api-key-env <ENV_VAR> --capability chat --capability tool_planning --enable " +
				"smoke=make smoke-ide commands=/help /overview /setup /status /next /command /evidence /mission /capabilities /workers api_key_value_stored=false stored=false",
			MetadataOnly:    true,
			PromptStored:    false,
			ResponseStored:  false,
			HistoryExcluded: true,
		}, true, nil
	case "/keys", "/shortcuts", "/controls":
		return tui.AIDialogResult{
			Status: "completed",
			Text: "keys: chat_focus=i submit=enter cancel_draft=esc quit=q|ctrl+c back=esc " +
				"runtime_select=up|down runtime_details=d runtime_execute=x_then_x runtime_request=r runtime_control=R_restart|L_renew|C_cancel|E_expire " +
				"replay_step=left|right replay_jump=home|end|g|G run_control=c_cancel|p_pause stored=false",
			MetadataOnly:    true,
			PromptStored:    false,
			ResponseStored:  false,
			HistoryExcluded: true,
		}, true, nil
	case "/help":
		return tui.AIDialogResult{
			Status:          "completed",
			Text:            "local commands: /quickstart /use /runbook /keys /shortcuts /controls /doctor /readiness /health /completion /progress /percent /acceptance /completion-status /final /final-evidence /final-review /audit /audit-completion /production-acceptance /status /workflow /workflow-status /overview /context /actions /usage /providers /workers /evidence /mission /next /command /artifacts /gates /previews /runs /reports /toolchain /capabilities /safety /setup /clear /help; normal prompts call configured provider; stored=false",
			MetadataOnly:    true,
			PromptStored:    false,
			ResponseStored:  false,
			HistoryExcluded: true,
		}, true, nil
	case "/context", "/workspace-context":
		contextText := strings.Join(strings.Fields(tuiAIDialogWorkspaceContext(root)), " ")
		if strings.TrimSpace(contextText) == "" {
			contextText = "workspace.context_status=unavailable"
		}
		return tui.AIDialogResult{
			Status:          "completed",
			Text:            "workspace context: " + contextText + " stored=false",
			MetadataOnly:    true,
			PromptStored:    false,
			ResponseStored:  false,
			HistoryExcluded: true,
		}, true, nil
	case "/usage", "/provider-usage":
		summary, err := SummarizeProviderUsageRecords(root)
		if err != nil {
			return tui.AIDialogResult{Status: "failed", MetadataOnly: true, HistoryExcluded: true}, true, fmt.Errorf("provider usage summary: %w", err)
		}
		return tui.AIDialogResult{
			Status: "completed",
			Text: fmt.Sprintf(
				"provider usage: records=%d providers=%s models=%s input_tokens=%d output_tokens=%d total_tokens=%d estimated_cost_usd=%.6f estimated_cost_records=%d budget_enforced_records=%d budget_within_limit_records=%d first_created_at=%s last_created_at=%s stored=false",
				summary.RecordCount,
				providerUsageSummaryRowList(summary.Providers),
				providerUsageSummaryRowList(summary.Models),
				summary.InputTokens,
				summary.OutputTokens,
				summary.TotalTokens,
				summary.EstimatedCostUSD,
				summary.EstimatedCostRecordCount,
				summary.BudgetEnforcedCount,
				summary.BudgetWithinLimitCount,
				installRequestDisplayValue(summary.FirstCreatedAt, "none"),
				installRequestDisplayValue(summary.LastCreatedAt, "none"),
			),
			MetadataOnly:    true,
			PromptStored:    false,
			ResponseStored:  false,
			HistoryExcluded: true,
		}, true, nil
	case "/setup":
		return tui.AIDialogResult{
			Status:          "completed",
			Text:            "AI setup: " + aiProviderSetupConversationMessage(aiProviderSetupGuidanceForWorkspace(root)) + " stored=false",
			MetadataOnly:    true,
			PromptStored:    false,
			ResponseStored:  false,
			HistoryExcluded: true,
		}, true, nil
	case "/clear":
		return tui.AIDialogResult{
			Status:          "completed",
			Text:            "dialog cleared; stored=false",
			MetadataOnly:    true,
			PromptStored:    false,
			ResponseStored:  false,
			HistoryExcluded: true,
			ClearHistory:    true,
		}, true, nil
	default:
		return tui.AIDialogResult{}, false, nil
	}
}

func tuiAIDialogReplayRuns(root string) (int, string, error) {
	eventDBPath := filepath.Join(root, "replay", "events.db")
	info, err := os.Lstat(eventDBPath)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, "none", nil
		}
		return 0, "", fmt.Errorf("stat replay events db: %w", err)
	}
	if !info.Mode().IsRegular() {
		return 0, "", fmt.Errorf("replay events db must be a regular file")
	}

	store, err := eventlog.OpenSQLiteReadOnly(eventDBPath)
	if err != nil {
		return 0, "", err
	}
	defer store.Close()

	runIDs, err := store.ListRunIDs(context.Background())
	if err != nil {
		return 0, "", err
	}
	latest := "none"
	if len(runIDs) > 0 {
		latest = runIDs[len(runIDs)-1]
	}
	return len(runIDs), latest, nil
}

type tuiAIDialogEvidenceStatus struct {
	Runs             int
	LatestAvailable  bool
	Events           int
	Facts            int
	Intents          int
	Hints            int
	Nodes            int
	Edges            int
	Findings         int
	Surfaces         int
	Artifacts        int
	VerifiedFindings int
	Paths            int
	EvidenceRefs     int
	Exhausted        int
	Suggestions      int
}

type tuiAIDialogOverviewStatus struct {
	WorkflowStatus     string
	Stage              string
	NextGate           string
	MissionConfigured  bool
	TargetPolicy       string
	Isolation          string
	Targets            int
	SafetyReady        bool
	StrictReady        bool
	ProvidersStatus    string
	Providers          int
	ProviderEnabled    int
	Chat               bool
	ToolPlanning       bool
	CapabilitiesStatus string
	DomainPacks        int
	Tools              int
	EvidenceRuns       int
	Findings           int
	ReportsCurrent     bool
	WorkersRegistered  int
	WorkerQueueClaims  int
}

type tuiAIDialogDoctorStatus struct {
	WorkspaceReady  bool
	StrictReady     bool
	Warnings        int
	Errors          int
	StrictErrors    int
	SafetyBypasses  int
	ProvidersStatus string
	ProviderEnabled int
	Chat            bool
	ToolPlanning    bool
	WorkflowStatus  string
	NextGate        string
	ToolchainStatus string
	Tools           int
}

func tuiAIDialogDoctorSummary(root string) (tuiAIDialogDoctorStatus, error) {
	safety, err := tuiAIDialogSafetySummary(root)
	if err != nil {
		return tuiAIDialogDoctorStatus{}, fmt.Errorf("summarize safety: %w", err)
	}
	providers := tuiAIDialogSummarizeProviders(root)
	toolchain := tuiAIDialogToolchainSummary(root)

	summary := tuiAIDialogDoctorStatus{
		WorkspaceReady:  safety.Ready,
		StrictReady:     safety.StrictReady,
		Warnings:        safety.Warnings,
		Errors:          safety.Errors,
		StrictErrors:    safety.StrictErrors,
		SafetyBypasses:  safety.Bypasses,
		ProvidersStatus: providers.Status,
		ProviderEnabled: providers.EnabledCount,
		Chat:            providers.Chat,
		ToolPlanning:    providers.ToolPlanning,
		WorkflowStatus:  "blocked",
		NextGate:        "doctor",
		ToolchainStatus: toolchain.Status,
		Tools:           toolchain.ToolCount,
	}
	if !safety.Ready {
		return summary, nil
	}

	workflow, err := SummarizeWorkspaceWorkflow(root)
	if err != nil {
		return summary, nil
	}
	summary.WorkflowStatus = workflow.Status
	summary.NextGate = workflow.NextGate
	return summary, nil
}

func tuiAIDialogOverviewSummary(root string) (tuiAIDialogOverviewStatus, error) {
	workflow, err := SummarizeWorkspaceWorkflow(root)
	if err != nil {
		return tuiAIDialogOverviewStatus{}, fmt.Errorf("summarize workflow: %w", err)
	}
	mission, err := tuiAIDialogSummarizeMissionScope(root)
	if err != nil {
		return tuiAIDialogOverviewStatus{}, fmt.Errorf("summarize mission: %w", err)
	}
	safety, err := tuiAIDialogSafetySummary(root)
	if err != nil {
		return tuiAIDialogOverviewStatus{}, fmt.Errorf("summarize safety: %w", err)
	}
	providers := tuiAIDialogSummarizeProviders(root)
	capabilities := tuiAIDialogCapabilitiesSummary(root)
	evidenceSummary, err := tuiAIDialogSummarizeEvidence(root)
	if err != nil {
		return tuiAIDialogOverviewStatus{}, fmt.Errorf("summarize evidence: %w", err)
	}
	reports, err := tuiAIDialogSummarizeReports(root)
	if err != nil {
		return tuiAIDialogOverviewStatus{}, fmt.Errorf("summarize reports: %w", err)
	}
	workers, err := tuiAIDialogSummarizeWorkers(root)
	if err != nil {
		return tuiAIDialogOverviewStatus{}, fmt.Errorf("summarize workers: %w", err)
	}

	return tuiAIDialogOverviewStatus{
		WorkflowStatus:     workflow.Status,
		Stage:              workflow.Stage,
		NextGate:           workflow.NextGate,
		MissionConfigured:  mission.Configured,
		TargetPolicy:       mission.TargetPolicy,
		Isolation:          mission.Isolation,
		Targets:            mission.Targets,
		SafetyReady:        safety.Ready,
		StrictReady:        safety.StrictReady,
		ProvidersStatus:    providers.Status,
		Providers:          providers.ProviderCount,
		ProviderEnabled:    providers.EnabledCount,
		Chat:               providers.Chat,
		ToolPlanning:       providers.ToolPlanning,
		CapabilitiesStatus: capabilities.Status,
		DomainPacks:        capabilities.DomainPacks,
		Tools:              capabilities.Tools,
		EvidenceRuns:       evidenceSummary.Runs,
		Findings:           evidenceSummary.Findings,
		ReportsCurrent:     reports.Current,
		WorkersRegistered:  workers.Registered,
		WorkerQueueClaims:  workers.QueueClaims,
	}, nil
}

type tuiAIDialogMissionScopeStatus struct {
	Configured   bool
	GoalSet      bool
	TargetPolicy string
	Isolation    string
	Targets      int
	URLs         int
	Domains      int
	Hosts        int
	CIDRs        int
	Artifacts    int
	Challenges   int
	Exclusions   int
}

func tuiAIDialogSummarizeMissionScope(root string) (tuiAIDialogMissionScopeStatus, error) {
	summary := tuiAIDialogMissionScopeStatus{
		TargetPolicy: "unknown",
		Isolation:    "unknown",
	}

	missionLoaded := false
	missionData, err := os.ReadFile(filepath.Join(root, "mission.yaml"))
	if err != nil {
		if !os.IsNotExist(err) {
			return tuiAIDialogMissionScopeStatus{}, fmt.Errorf("read mission config: %w", err)
		}
	} else {
		missionLoaded = true
		var missionConfig struct {
			Mission struct {
				Goal string `yaml:"goal"`
			} `yaml:"mission"`
		}
		if err := yaml.Unmarshal(missionData, &missionConfig); err != nil {
			return tuiAIDialogMissionScopeStatus{}, fmt.Errorf("parse mission config: %w", err)
		}
		summary.GoalSet = strings.TrimSpace(missionConfig.Mission.Goal) != ""
	}

	scopeConfig, err := LoadScopeConfig(filepath.Join(root, "scope.yaml"))
	if err != nil {
		if os.IsNotExist(err) {
			summary.Configured = false
			return summary, nil
		}
		return tuiAIDialogMissionScopeStatus{}, fmt.Errorf("read scope config: %w", err)
	}
	summary.Configured = missionLoaded
	if policy, ok := normalizeScopeTargetPolicy(scopeConfig.Scope.TargetPolicy); ok {
		summary.TargetPolicy = policy
	}
	if isolation, ok := normalizeExecutionIsolation(scopeConfig.Execution.Isolation); ok {
		summary.Isolation = isolation
	}
	summary.Targets = len(scopeConfig.Scope.Targets)
	summary.Exclusions = len(scopeConfig.Scope.Exclusions)
	for _, target := range scopeConfig.Scope.Targets {
		switch strings.ToLower(strings.TrimSpace(target.Kind)) {
		case "url":
			summary.URLs++
		case "domain":
			summary.Domains++
		case "host":
			summary.Hosts++
		case "cidr":
			summary.CIDRs++
		case "artifact":
			summary.Artifacts++
		case "challenge":
			summary.Challenges++
		}
	}
	return summary, nil
}

func tuiAIDialogSummarizeEvidence(root string) (tuiAIDialogEvidenceStatus, error) {
	eventDBPath := filepath.Join(root, "replay", "events.db")
	info, err := os.Lstat(eventDBPath)
	if err != nil {
		if os.IsNotExist(err) {
			return tuiAIDialogEvidenceStatus{}, nil
		}
		return tuiAIDialogEvidenceStatus{}, fmt.Errorf("stat replay events db: %w", err)
	}
	if !info.Mode().IsRegular() {
		return tuiAIDialogEvidenceStatus{}, fmt.Errorf("replay events db must be a regular file")
	}

	store, err := eventlog.OpenSQLiteReadOnly(eventDBPath)
	if err != nil {
		return tuiAIDialogEvidenceStatus{}, err
	}
	defer store.Close()

	ctx := context.Background()
	runIDs, err := store.ListRunIDs(ctx)
	if err != nil {
		return tuiAIDialogEvidenceStatus{}, err
	}
	summary := tuiAIDialogEvidenceStatus{Runs: len(runIDs)}
	if len(runIDs) == 0 {
		return summary, nil
	}
	summary.LatestAvailable = true

	events, err := store.Replay(ctx, runIDs[len(runIDs)-1])
	if err != nil {
		return tuiAIDialogEvidenceStatus{}, err
	}
	summary.Events = len(events)
	snapshot, err := eventlog.BuildReplaySnapshot(events)
	if err != nil {
		return tuiAIDialogEvidenceStatus{}, err
	}
	summary.Facts = len(snapshot.Blackboard.Facts)
	summary.Intents = len(snapshot.Blackboard.Intents)
	summary.Hints = len(snapshot.Blackboard.Hints)
	nodes := snapshot.Evidence.Nodes()
	summary.Nodes = len(nodes)
	summary.Edges = len(snapshot.Evidence.Edges())
	for _, node := range nodes {
		switch node.Type {
		case evidence.NodeFinding:
			summary.Findings++
		case evidence.NodeSurface:
			summary.Surfaces++
		case evidence.NodeArtifact:
			if strings.EqualFold(strings.TrimSpace(node.SourceKind), "artifact") {
				summary.Artifacts++
			}
		}
	}
	projected := reportpkg.Project(snapshot.Evidence)
	summary.VerifiedFindings = len(projected.VerifiedFindings)
	summary.Paths = len(projected.Paths)
	summary.EvidenceRefs = len(projected.EvidenceRefs)
	summary.Exhausted = len(projected.ExhaustedAreas)
	summary.Suggestions = len(projected.NextRunSuggestions)
	return summary, nil
}

type tuiAIDialogReportSummary struct {
	Dispatches int
	Executions int
	Current    bool
	RunID      string
	EventCount int
	Written    bool
}

func tuiAIDialogSummarizeReports(root string) (tuiAIDialogReportSummary, error) {
	if strings.TrimSpace(root) == "" {
		return tuiAIDialogReportSummary{}, fmt.Errorf("workspace path is required")
	}
	info, err := os.Stat(root)
	if err != nil {
		if os.IsNotExist(err) {
			return tuiAIDialogReportSummary{}, fmt.Errorf("workspace root missing")
		}
		return tuiAIDialogReportSummary{}, fmt.Errorf("stat workspace root: %w", err)
	}
	if !info.IsDir() {
		return tuiAIDialogReportSummary{}, fmt.Errorf("workspace root is not a directory")
	}

	dispatches, err := tuiAIDialogCountReportJSON(filepath.Join(root, "reports", "report-projection-dispatches"), "report projection dispatch directory")
	if err != nil {
		return tuiAIDialogReportSummary{}, err
	}
	executions, err := tuiAIDialogCountReportJSON(filepath.Join(root, "reports", "report-projection-dispatch-executions"), "report projection dispatch execution directory")
	if err != nil {
		return tuiAIDialogReportSummary{}, err
	}

	summary := tuiAIDialogReportSummary{
		Dispatches: dispatches,
		Executions: executions,
	}
	selection, found, err := tuiAIDialogReadReportSelection(filepath.Join(root, "reports", "report-projection-selection.json"))
	if err != nil {
		return tuiAIDialogReportSummary{}, err
	}
	if found && strings.EqualFold(strings.TrimSpace(selection.Status), "current") {
		summary.Current = true
		summary.RunID = selection.RunID
		summary.EventCount = selection.EventCount
		summary.Written = selection.Written
	}
	return summary, nil
}

func tuiAIDialogCountReportJSON(dir string, label string) (int, error) {
	info, err := os.Lstat(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, fmt.Errorf("inspect %s: %w", label, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return 0, fmt.Errorf("%s must not be a symlink", label)
	}
	if !info.IsDir() {
		return 0, fmt.Errorf("%s is not a directory", label)
	}
	paths, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		return 0, fmt.Errorf("glob %s: %w", label, err)
	}
	return len(paths), nil
}

func tuiAIDialogReadReportSelection(path string) (ReportProjectionDispatchExecutionSelection, bool, error) {
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return ReportProjectionDispatchExecutionSelection{}, false, nil
		}
		return ReportProjectionDispatchExecutionSelection{}, false, fmt.Errorf("inspect report projection selection: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return ReportProjectionDispatchExecutionSelection{}, false, fmt.Errorf("report projection selection must not be a symlink")
	}
	if info.IsDir() {
		return ReportProjectionDispatchExecutionSelection{}, false, fmt.Errorf("report projection selection is a directory")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ReportProjectionDispatchExecutionSelection{}, false, fmt.Errorf("read report projection selection: %w", err)
	}
	var selection ReportProjectionDispatchExecutionSelection
	if err := json.Unmarshal(data, &selection); err != nil {
		return ReportProjectionDispatchExecutionSelection{}, false, fmt.Errorf("decode report projection selection: %w", err)
	}
	return selection, true, nil
}

func tuiAIDialogToolchainSummary(root string) ToolchainContractCheck {
	check, err := CheckToolchainContracts(root)
	if err != nil {
		return ToolchainContractCheck{
			Status: "blocked",
			Errors: []string{
				"contract-check-unavailable",
			},
		}
	}
	if strings.TrimSpace(check.Status) == "" {
		check.Status = "blocked"
	}
	return check
}

type tuiAIDialogCapabilitiesStatus struct {
	Status               string
	Domains              int
	DomainPacks          int
	PackCapabilities     int
	Parsers              int
	Strategies           int
	SemanticActions      int
	SemanticCapabilities int
	Manifests            int
	ManifestCapabilities int
	ManifestDependencies int
	Tools                int
	ToolCapabilities     int
	ToolCategories       int
	TargetKinds          int
	ManagedBundles       int
	ManagedInstalled     int
	ManagedBlocked       int
	Patterns             int
	Errors               int
}

func tuiAIDialogCapabilitiesSummary(root string) tuiAIDialogCapabilitiesStatus {
	check := tuiAIDialogToolchainSummary(root)
	summary := tuiAIDialogCapabilitiesStatus{
		Status:          workflowStatusRowValue(check.Status, "blocked"),
		DomainPacks:     check.DomainPackCount,
		Manifests:       check.ManifestCount,
		Tools:           check.ToolCount,
		Patterns:        check.PatternCount,
		SemanticActions: check.SemanticActionCount,
		Errors:          len(check.Errors),
	}

	packs := contracts.BuiltInDomainPacks()
	if summary.DomainPacks == 0 {
		summary.DomainPacks = len(packs)
	}
	domains := map[string]bool{}
	for _, pack := range packs {
		if pack == nil {
			summary.Errors++
			continue
		}
		if domainName := strings.TrimSpace(string(pack.Domain())); domainName != "" {
			domains[domainName] = true
		}
		summary.PackCapabilities += len(pack.Capabilities())
		summary.Parsers += len(pack.Parsers())
		summary.Strategies += len(pack.StrategyPatterns())
	}
	summary.Domains = len(domains)

	actions := compiler.DeclaredSemanticActions()
	if summary.SemanticActions == 0 {
		summary.SemanticActions = len(actions)
	}
	for _, action := range actions {
		capabilities, ok := compiler.CapabilitiesForAction(action)
		if !ok {
			summary.Errors++
			continue
		}
		summary.SemanticCapabilities += len(capabilities)
	}

	manifests, err := toolchain.LoadManifests(filepath.Join(root, "toolchain"))
	if err != nil {
		summary.Errors++
	} else {
		summary.Manifests = len(manifests)
		for _, manifest := range manifests {
			summary.ManifestCapabilities += len(manifest.Capabilities)
			summary.ManifestDependencies += len(manifest.Dependencies)
		}
	}

	tools := toolchain.BuiltInAppSecTools()
	if summary.Tools == 0 {
		summary.Tools = len(tools)
	}
	toolCategories := map[string]bool{}
	targetKinds := map[string]bool{}
	for _, builtInTool := range tools {
		summary.ToolCapabilities += len(builtInTool.Capabilities)
		if category := strings.TrimSpace(builtInTool.Category); category != "" {
			toolCategories[category] = true
		}
		if targetKind := strings.TrimSpace(builtInTool.TargetKind); targetKind != "" {
			targetKinds[targetKind] = true
		}
	}
	summary.ToolCategories = len(toolCategories)
	summary.TargetKinds = len(targetKinds)

	bundles, err := ListManagedToolBundles(root)
	if err != nil {
		summary.Errors++
	} else {
		summary.ManagedBundles = len(bundles)
		for _, bundle := range bundles {
			switch strings.ToLower(strings.TrimSpace(bundle.Status)) {
			case "installed":
				summary.ManagedInstalled++
			case "blocked":
				summary.ManagedBlocked++
			}
		}
	}
	return summary
}

type tuiAIDialogSafetyStatus struct {
	Ready        bool
	StrictReady  bool
	Warnings     int
	Errors       int
	StrictErrors int
	Bypasses     int
	Authorized   int
	Missing      int
	Expired      int
}

func tuiAIDialogSafetySummary(root string) (tuiAIDialogSafetyStatus, error) {
	report, err := doctorWorkspaceMetadataOnly(root)
	if err != nil {
		return tuiAIDialogSafetyStatus{}, err
	}
	strictReport, err := doctorWorkspaceStrictMetadataOnly(root)
	if err != nil {
		return tuiAIDialogSafetyStatus{}, err
	}
	summary := tuiAIDialogSafetyStatus{
		Ready:        report.Ready,
		StrictReady:  strictReport.Ready,
		Warnings:     len(report.Warnings),
		Errors:       len(report.Errors),
		StrictErrors: len(strictReport.Errors),
		Bypasses:     len(report.SafetyBypasses),
	}
	for _, bypass := range report.SafetyBypasses {
		if bypass.Authorized {
			summary.Authorized++
		}
		if bypass.MissingAuthorization {
			summary.Missing++
		}
		if bypass.ExpiredAuthorization {
			summary.Expired++
		}
	}
	return summary, nil
}

type tuiAIDialogProviderStatus struct {
	Status              string
	Reason              string
	ProviderCount       int
	EnabledCount        int
	Routing             string
	FirstType           string
	FirstModel          string
	Chat                bool
	ToolPlanning        bool
	APIKeyEnvConfigured bool
	APIKeyEnvSet        bool
}

func tuiAIDialogSummarizeProviders(root string) tuiAIDialogProviderStatus {
	guidance := aiProviderSetupGuidanceForWorkspace(root)
	summary := tuiAIDialogProviderStatus{
		Status:     guidance.Status,
		Reason:     guidance.Reason,
		Routing:    "unknown",
		FirstType:  "none",
		FirstModel: "none",
	}
	config, err := LoadProviderConfig(filepath.Join(root, "providers.yaml"))
	if err != nil {
		if summary.Status == "" {
			summary.Status = "blocked"
		}
		if summary.Reason == "" {
			summary.Reason = "load-provider-config"
		}
		return summary
	}

	summary.ProviderCount = len(config.Providers)
	enabledProviders := EnabledProviders(config)
	summary.EnabledCount = len(enabledProviders)
	if routing, err := providerRoutingStrategy(config); err == nil {
		summary.Routing = routing
	}
	if len(enabledProviders) == 0 {
		return summary
	}

	first := enabledProviders[0]
	summary.FirstType = strings.TrimSpace(first.Type)
	summary.FirstModel = strings.TrimSpace(first.Model)
	for _, capability := range NormalizeProviderCapabilities(first.Capabilities) {
		switch capability {
		case "chat":
			summary.Chat = true
		case "tool_planning":
			summary.ToolPlanning = true
		}
	}
	apiKeyEnv := strings.TrimSpace(first.APIKeyEnv)
	summary.APIKeyEnvConfigured = validProviderAPIKeyEnvName(apiKeyEnv)
	if summary.APIKeyEnvConfigured {
		_, summary.APIKeyEnvSet = os.LookupEnv(apiKeyEnv)
	}
	return summary
}

type tuiAIDialogWorkerRuntimeStatus struct {
	Registered               int
	Available                int
	Busy                     int
	Draining                 int
	Offline                  int
	Stale                    int
	Local                    int
	Remote                   int
	QueueClaims              int
	QueueClaimed             int
	QueueDispatched          int
	QueueCompleted           int
	QueueFailed              int
	DispatchExecutions       int
	DispatchDispatched       int
	DispatchCompleted        int
	DispatchFailed           int
	TransportRequests        int
	TransportPending         int
	TransportAck             int
	TransportCompleted       int
	TransportFailed          int
	DispatchResults          int
	DispatchResultsCompleted int
	DispatchResultsFailed    int
	Leases                   int
	ActiveLeases             int
	StaleLeases              int
	CompletedLeases          int
	FailedLeases             int
	ReclaimedLeases          int
}

func tuiAIDialogSummarizeWorkers(root string) (tuiAIDialogWorkerRuntimeStatus, error) {
	now := time.Now().UTC()
	summary := tuiAIDialogWorkerRuntimeStatus{}

	workers, err := ListRuntimeWorkersWithStaleness(root, now, DefaultRuntimeWorkerStaleAfter)
	if err != nil {
		return tuiAIDialogWorkerRuntimeStatus{}, err
	}
	summary.Registered = len(workers)
	for _, worker := range workers {
		switch strings.TrimSpace(worker.Mode) {
		case "local":
			summary.Local++
		case "remote":
			summary.Remote++
		}
		if worker.Stale {
			summary.Stale++
			continue
		}
		switch strings.TrimSpace(worker.Status) {
		case "available":
			summary.Available++
		case "busy":
			summary.Busy++
		case "draining":
			summary.Draining++
		case "offline":
			summary.Offline++
		}
	}

	claims, err := ListRuntimeWorkerQueueClaims(root)
	if err != nil {
		return tuiAIDialogWorkerRuntimeStatus{}, err
	}
	summary.QueueClaims = len(claims)
	for _, claim := range claims {
		switch strings.TrimSpace(claim.Status) {
		case "claimed":
			summary.QueueClaimed++
		case "dispatched":
			summary.QueueDispatched++
		case "completed":
			summary.QueueCompleted++
		case "failed":
			summary.QueueFailed++
		}
	}

	dispatches, err := ListRuntimeWorkerQueueDispatchExecutions(root)
	if err != nil {
		return tuiAIDialogWorkerRuntimeStatus{}, err
	}
	summary.DispatchExecutions = len(dispatches)
	for _, dispatch := range dispatches {
		switch strings.TrimSpace(dispatch.Status) {
		case "dispatched":
			summary.DispatchDispatched++
		case "completed":
			summary.DispatchCompleted++
		case "failed":
			summary.DispatchFailed++
		}
	}

	requests, err := ListRuntimeWorkerTransportDispatchRequests(root, "")
	if err != nil {
		return tuiAIDialogWorkerRuntimeStatus{}, err
	}
	summary.TransportRequests = len(requests)
	for _, request := range requests {
		switch strings.TrimSpace(request.Status) {
		case "pending":
			summary.TransportPending++
		case "acknowledged":
			summary.TransportAck++
		case "completed":
			summary.TransportCompleted++
		case "failed":
			summary.TransportFailed++
		}
	}

	results, err := ListRuntimeWorkerQueueDispatchResults(root)
	if err != nil {
		return tuiAIDialogWorkerRuntimeStatus{}, err
	}
	summary.DispatchResults = len(results)
	for _, result := range results {
		switch strings.TrimSpace(result.Status) {
		case "completed":
			summary.DispatchResultsCompleted++
		case "failed":
			summary.DispatchResultsFailed++
		}
	}

	leases, err := ListWorkerLeasesWithStaleness(root, now, DefaultWorkerLeaseStaleAfter)
	if err != nil {
		return tuiAIDialogWorkerRuntimeStatus{}, err
	}
	summary.Leases = len(leases)
	for _, lease := range leases {
		if lease.Stale {
			summary.StaleLeases++
			continue
		}
		switch strings.TrimSpace(lease.Status) {
		case "active":
			summary.ActiveLeases++
		case "completed":
			summary.CompletedLeases++
		case "failed":
			summary.FailedLeases++
		case "reclaimed":
			summary.ReclaimedLeases++
		}
	}

	return summary, nil
}

func tuiAIDialogSanitizedWorkflowCommand(root string, command string) string {
	command = strings.TrimSpace(command)
	if command == "" {
		return "none"
	}
	workspace := strings.TrimSpace(root)
	if workspace != "" {
		replacements := []string{workspace}
		if cleaned := filepath.Clean(workspace); cleaned != workspace {
			replacements = append(replacements, cleaned)
		}
		for _, replacement := range replacements {
			command = strings.ReplaceAll(command, replacement, "<workspace>")
		}
	}
	return strings.Join(strings.Fields(command), " ")
}

func providerGenerationMessagesForAIDialog(request tui.AIDialogRequest) []ProviderGenerationMessage {
	messages := []ProviderGenerationMessage{}
	for _, history := range request.History {
		role := strings.ToLower(strings.TrimSpace(history.Role))
		content := strings.TrimSpace(history.Text)
		if content == "" {
			continue
		}
		if role != "user" && role != "assistant" {
			continue
		}
		messages = append(messages, ProviderGenerationMessage{Role: role, Content: content})
	}
	if prompt := strings.TrimSpace(request.Prompt); prompt != "" {
		messages = append(messages, ProviderGenerationMessage{Role: "user", Content: prompt})
	}
	return messages
}

type aiProviderSetupGuidance struct {
	Status    string
	Reason    string
	Provider  string
	Model     string
	APIKeyEnv string
	Command   string
	EnvHint   string
}

func tuiModelWithAIProviderSetupRows(root string, model tui.Model) tui.Model {
	guidance := aiProviderSetupGuidanceForWorkspace(root)
	if strings.TrimSpace(guidance.Status) == "" {
		return model
	}
	message := aiProviderSetupConversationMessage(guidance)
	if message != "" {
		model = model.AppendMissionEvents([]tui.MissionEvent{{
			Kind:    "AI setup",
			Message: message,
		}})
	}
	return model.PrependRuntimeRows(tui.RuntimeTabModelRuntime, []string{aiProviderSetupRuntimeRow(guidance)})
}

func aiProviderSetupGuidanceForWorkspace(root string) aiProviderSetupGuidance {
	command := aiProviderSetupConfigureCommand(root)
	config, err := LoadProviderConfig(filepath.Join(root, "providers.yaml"))
	if err != nil {
		return aiProviderSetupGuidance{
			Status:  "blocked",
			Reason:  "load-provider-config",
			Command: command,
		}
	}
	if err := ValidateProviderConfig(config); err != nil {
		return aiProviderSetupGuidance{
			Status:  "blocked",
			Reason:  "invalid-provider-config",
			Command: command,
		}
	}
	providers := EnabledProviders(config)
	if len(providers) == 0 {
		return aiProviderSetupGuidance{
			Status:  "needs-provider",
			Reason:  "no-enabled-provider",
			Command: command,
		}
	}
	provider := providers[0]
	providerType := strings.TrimSpace(provider.Type)
	if providerType == "offline" {
		return aiProviderSetupGuidance{
			Status:   "needs-provider",
			Reason:   "enabled-provider-offline",
			Provider: strings.TrimSpace(provider.Name),
			Model:    strings.TrimSpace(provider.Model),
			Command:  command,
		}
	}
	if providerType != providerTypeOpenAICompatible {
		return aiProviderSetupGuidance{
			Status:   "blocked",
			Reason:   "unsupported-provider-type",
			Provider: strings.TrimSpace(provider.Name),
			Model:    strings.TrimSpace(provider.Model),
			Command:  command,
		}
	}
	apiKeyEnv := strings.TrimSpace(provider.APIKeyEnv)
	if !validProviderAPIKeyEnvName(apiKeyEnv) {
		return aiProviderSetupGuidance{
			Status:   "needs-api-key",
			Reason:   "missing-api-key-env-name",
			Provider: strings.TrimSpace(provider.Name),
			Model:    strings.TrimSpace(provider.Model),
			Command:  command,
			EnvHint:  "set --api-key-env to an environment variable name",
		}
	}
	if _, ok := os.LookupEnv(apiKeyEnv); !ok {
		return aiProviderSetupGuidance{
			Status:    "needs-api-key",
			Reason:    "api-key-env-not-set",
			Provider:  strings.TrimSpace(provider.Name),
			Model:     strings.TrimSpace(provider.Model),
			APIKeyEnv: apiKeyEnv,
			Command:   command,
			EnvHint:   "export " + apiKeyEnv + "=<your-provider-key>",
		}
	}
	return aiProviderSetupGuidance{
		Status:    "ready",
		Reason:    "openai-compatible-ready",
		Provider:  strings.TrimSpace(provider.Name),
		Model:     strings.TrimSpace(provider.Model),
		APIKeyEnv: apiKeyEnv,
		Command:   command,
	}
}

func aiProviderSetupConfigureCommand(root string) string {
	return strings.Join([]string{
		"ouro",
		"provider",
		"configure-openai-compatible",
		strings.TrimSpace(root),
		"--name",
		"<provider-name>",
		"--base-url",
		"<https://provider.example/v1>",
		"--model",
		"<model>",
		"--api-key-env",
		"<ENV_VAR>",
		"--capability",
		"chat",
		"--capability",
		"tool_planning",
		"--enable",
	}, " ")
}

func aiProviderSetupConversationMessage(guidance aiProviderSetupGuidance) string {
	parts := []string{
		"status=" + workflowStatusRowValue(guidance.Status, "unknown"),
		"reason=" + workflowStatusRowValue(guidance.Reason, "unknown"),
	}
	if provider := strings.TrimSpace(guidance.Provider); provider != "" {
		parts = append(parts, "provider="+workflowStatusRowValue(provider, "unknown"))
	}
	if model := strings.TrimSpace(guidance.Model); model != "" {
		parts = append(parts, "model="+workflowStatusRowValue(model, "unknown"))
	}
	if strings.TrimSpace(guidance.Status) == "ready" {
		parts = append(parts, "api_key_value_stored=false")
		return strings.Join(parts, " ")
	}
	if envHint := strings.TrimSpace(guidance.EnvHint); envHint != "" {
		parts = append(parts, "hint="+envHint)
	}
	if command := strings.TrimSpace(guidance.Command); command != "" {
		parts = append(parts, "configure: "+command)
	}
	parts = append(parts, "api_key_value_stored=false")
	return strings.Join(parts, " ")
}

func aiProviderSetupRuntimeRow(guidance aiProviderSetupGuidance) string {
	fields := []string{
		"AI setup: status=" + workflowStatusRowValue(guidance.Status, "unknown"),
		"reason=" + workflowStatusRowValue(guidance.Reason, "unknown"),
		"mode=read-only",
	}
	if provider := strings.TrimSpace(guidance.Provider); provider != "" {
		fields = append(fields, "provider="+workflowStatusRowValue(provider, "unknown"))
	}
	if model := strings.TrimSpace(guidance.Model); model != "" {
		fields = append(fields, "model="+workflowStatusRowValue(model, "unknown"))
	}
	if apiKeyEnv := strings.TrimSpace(guidance.APIKeyEnv); apiKeyEnv != "" {
		fields = append(fields, "api_key_env="+workflowStatusRowValue(apiKeyEnv, "unknown"))
	}
	fields = append(fields, "api_key_value_stored=false")
	if envHint := strings.TrimSpace(guidance.EnvHint); envHint != "" {
		fields = append(fields, "hint="+compactRuntimeRowValue(envHint), "hint_b64="+runtimeRowValueB64(envHint))
	}
	if command := strings.TrimSpace(guidance.Command); command != "" && strings.TrimSpace(guidance.Status) != "ready" {
		fields = append(fields, "command="+command, "command_b64="+runtimeRowValueB64(command))
	}
	return strings.Join(fields, " ")
}

func aiProviderSetupFailureHint(root string) string {
	guidance := aiProviderSetupGuidanceForWorkspace(root)
	command := strings.TrimSpace(guidance.Command)
	if command == "" {
		command = aiProviderSetupConfigureCommand(root)
	}
	if envHint := strings.TrimSpace(guidance.EnvHint); envHint != "" {
		return "AI setup: " + envHint + "; configure provider with: " + command
	}
	return "AI setup: configure provider with: " + command
}

func tuiModelWithWorkspaceRuntimeRows(root string, model tui.Model) tui.Model {
	model = tuiModelWithAIProviderSetupRows(root, model)
	model = tuiModelWithProviderUsageSummaryRows(root, model)
	model = tuiModelWithModelGenerationArtifactRows(root, model)
	model = tuiModelWithModelToolPlanPreviewArtifactRows(root, model)
	model = tuiModelWithModelToolPlanPreviewAcceptanceRows(root, model)
	model = tuiModelWithWorkspaceWorkflowStatusRows(root, model)
	model = tuiModelWithManagedBundleRows(root, model)
	model = tuiModelWithReleaseManagedStatusRows(root, model)
	model = tuiModelWithManagedUpdateRequestRows(root, model)
	model = tuiModelWithManagedUpdateDownloadPlanRows(root, model)
	model = tuiModelWithManagedUpdateDownloadExecutionRows(root, model)
	model = tuiModelWithManagedUpdateApplyExecutionRows(root, model)
	model = tuiModelWithInstallRequestRows(root, model)
	model = tuiModelWithToolchainContractRows(root, model)
	model = tuiModelWithCompilePreviewSignalRows(root, model)
	model = tuiModelWithCompilePreviewReviewRows(root, model)
	model = tuiModelWithCompilePreviewReviewExecutionDispatchRows(root, model)
	model = tuiModelWithCompilePreviewReviewExecutionRows(root, model)
	model = tuiModelWithCompilePreviewRows(root, model)
	model = tuiModelWithReplayWorkbenchCollaborationNoteRows(root, model)
	model = tuiModelWithReplayWorkbenchCollaborationRows(root, model)
	model = tuiModelWithReplayWorkbenchCollaborationPresenceRows(root, model)
	model = tuiModelWithReplayWorkbenchSessionRows(root, model)
	model = tuiModelWithReportProjectionDispatchRows(root, model)
	model = tuiModelWithReportProjectionDispatchExecutionRows(root, model)
	model = tuiModelWithRuntimeWorkerRows(root, model)
	model = tuiModelWithRuntimeWorkerOrchestrationRows(root, model)
	model = tuiModelWithRuntimeWorkerDispatchRetryPlanRows(root, model)
	model = tuiModelWithRuntimeWorkerDispatchRetryDispatchRows(root, model)
	model = tuiModelWithRuntimeWorkerDispatchRetryDispatchExecutionRows(root, model)
	model = tuiModelWithRuntimeWorkerDispatchRetrySchedulerRunRows(root, model)
	model = tuiModelWithRuntimeWorkerOrchestrationSupervisorRows(root, model)
	model = tuiModelWithWorkerLifecyclePolicyRows(root, model)
	model = tuiModelWithWorkerLifecyclePolicyRunRows(root, model)
	model = tuiModelWithWorkerLifecyclePolicySchedulerRunRows(root, model)
	model = tuiModelWithWorkerLifecycleMaintenanceSupervisorRows(root, model)
	model = tuiModelWithWorkerLeaseResultReconciliationRows(root, model)
	model = tuiModelWithWorkerLifecycleMaintenanceRows(root, model)
	model = tuiModelWithWorkerLifecycleAuditRows(root, model)
	model = tuiModelWithWorkerLifecycleActionPlanRows(root, model)
	model = tuiModelWithWorkerLifecycleReviewRows(root, model)
	model = tuiModelWithRuntimeWorkerQueueClaimRows(root, model)
	model = tuiModelWithRuntimeWorkerPlacementClaimExecutionRows(root, model)
	model = tuiModelWithRuntimeWorkerQueueDispatchExecutionRows(root, model)
	model = tuiModelWithRuntimeWorkerPlacementDispatchRows(root, model)
	model = tuiModelWithRuntimeWorkerPlacementDispatchExecutionRows(root, model)
	model = tuiModelWithRuntimeWorkerPlacementDispatchBatchRows(root, model)
	model = tuiModelWithRuntimeWorkerTransportDaemonRunRows(root, model)
	model = tuiModelWithRuntimeWorkerTransportDaemonSupervisorRows(root, model)
	model = tuiModelWithRuntimeWorkerTransportDaemonControlRows(root, model)
	model = tuiModelWithRuntimeWorkerTransportDispatchRequestRows(root, model)
	model = tuiModelWithRuntimeWorkerQueueDispatchResultRows(root, model)
	model = tuiModelWithWorkerLeaseRows(root, model)
	model = tuiModelWithRuntimeWorkerPlacementRows(root, model)
	return model
}

func tuiModelWithWorkspaceWorkflowStatusRows(root string, model tui.Model) tui.Model {
	status, err := SummarizeWorkspaceWorkflow(root)
	if err != nil {
		return model.PrependRuntimeRows(tui.RuntimeTabModelRuntime, []string{
			"workflow status error: mode=read-only error=" + workflowStatusRowValue(err.Error(), "unknown"),
		})
	}
	row := strings.Join([]string{
		"workflow status:",
		"mode=" + workflowStatusRowValue(status.Mode, "read-only"),
		"status=" + workflowStatusRowValue(status.Status, "unknown"),
		"stage=" + workflowStatusRowValue(status.Stage, "unknown"),
		"next_gate=" + workflowStatusRowValue(status.NextGate, "none"),
		"reason=" + workflowStatusRowValue(status.Reason, "none"),
		fmt.Sprintf("safety_errors=%d", status.SafetyErrorCount),
		"action=" + workflowStatusRowValue(string(status.Action), "none"),
		"command=" + workflowStatusCommandRowValue(status.Command),
		workflowStatusPathRowField("preview", status.PreviewPath, "-"),
		workflowStatusPathRowField("acceptance", status.AcceptancePath, "-"),
		workflowStatusPathRowField("review", status.ReviewPath, "-"),
		workflowStatusPathRowField("selection", status.SelectionPath, "-"),
		workflowStatusPathRowField("dispatch", status.DispatchPath, "-"),
		workflowStatusPathRowField("execution", status.ExecutionPath, "-"),
		workflowStatusPathRowField("report_dispatch", status.ReportDispatchPath, "-"),
		workflowStatusPathRowField("report_execution", status.ReportExecutionPath, "-"),
		workflowStatusPathRowField("report_selection", status.ReportSelectionPath, "-"),
		"report_run=" + workflowStatusRowValue(status.ReportRunID, "-"),
		workflowStatusPathRowField("report", status.ReportPath, "-"),
	}, " ")
	links := fmt.Sprintf(
		"workflow status links: preview=%s acceptance=%s review=%s selection=%s dispatch=%s execution=%s report_dispatch=%s report_execution=%s report_selection=%s report_run=%s report=%s",
		workflowStatusRowValue(status.PreviewPath, "-"),
		workflowStatusRowValue(status.AcceptancePath, "-"),
		workflowStatusRowValue(status.ReviewPath, "-"),
		workflowStatusRowValue(status.SelectionPath, "-"),
		workflowStatusRowValue(status.DispatchPath, "-"),
		workflowStatusRowValue(status.ExecutionPath, "-"),
		workflowStatusRowValue(status.ReportDispatchPath, "-"),
		workflowStatusRowValue(status.ReportExecutionPath, "-"),
		workflowStatusRowValue(status.ReportSelectionPath, "-"),
		workflowStatusRowValue(status.ReportRunID, "-"),
		workflowStatusRowValue(status.ReportPath, "-"),
	)
	return model.PrependRuntimeRows(tui.RuntimeTabModelRuntime, []string{row, links})
}

func workflowStatusPathRowField(key string, value string, fallback string) string {
	visible := workflowStatusRowValue(value, fallback)
	raw := strings.TrimSpace(value)
	if raw == "" || raw == "-" || strings.EqualFold(raw, "none") || visible == "-" || strings.EqualFold(visible, "none") {
		return key + "=" + visible
	}
	return key + "=" + visible + " " + key + "_b64=" + runtimeRowValueB64(raw)
}

func workflowStatusRowValue(value string, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return strings.Join(strings.Fields(value), "_")
}

func workflowStatusCommandRowValue(command string) string {
	fields := strings.Fields(command)
	if len(fields) == 0 {
		return "-"
	}
	parts := []string{}
	for i, field := range fields {
		if i < 3 || strings.HasPrefix(field, "--") {
			parts = append(parts, field)
		}
	}
	return strings.Join(parts, "_")
}

func tuiModelWithProviderUsageSummaryRows(root string, model tui.Model) tui.Model {
	summary, err := SummarizeProviderUsageRecords(root)
	if err != nil {
		return model.AppendRuntimeRows(tui.RuntimeTabModelRuntime, []string{
			"provider usage summary error: mode=metadata-only error=" + compactRuntimeRowValue(err.Error()),
		})
	}
	return model.AppendRuntimeRows(tui.RuntimeTabModelRuntime, providerUsageSummaryRuntimeRows(root, summary))
}

func tuiModelWithModelGenerationArtifactRows(root string, model tui.Model) tui.Model {
	artifacts, err := ListModelGenerationArtifacts(root)
	if err != nil {
		return model.AppendRuntimeRows(tui.RuntimeTabModelRuntime, []string{
			"model generation artifact error: mode=metadata-only error=" + compactRuntimeRowValue(err.Error()),
		})
	}
	if len(artifacts) == 0 {
		return model
	}
	return model.AppendRuntimeRows(tui.RuntimeTabModelRuntime, modelGenerationArtifactRuntimeRows(artifacts))
}

func modelGenerationArtifactRuntimeRows(artifacts []ModelGenerationArtifact) []string {
	rows := make([]string, 0, len(artifacts))
	for _, artifact := range artifacts {
		cost := "unavailable"
		if artifact.EstimatedCostAvailable {
			cost = fmt.Sprintf("$%.6f", artifact.EstimatedCostUSD)
		}
		rows = append(rows, fmt.Sprintf(
			"model generation artifact: mode=metadata-only status=%s run=%s source=%s provider=%s model=%s tokens=%d/%d/%d cost=%s budget=%s ledger_write=%t assistant_text_persisted=%t%s path=%s artifact=%s digest=%s preview=%s preview_b64=%s",
			installRequestDisplayValue(artifact.Status, "unknown"),
			installRequestDisplayValue(artifact.RunID, "unknown"),
			installRequestDisplayValue(artifact.Source, "model-generation"),
			compactRuntimeRowValue(artifact.ProviderName),
			compactRuntimeRowValue(artifact.Model),
			artifact.InputTokens,
			artifact.OutputTokens,
			artifact.TotalTokens,
			cost,
			installRequestDisplayValue(artifact.BudgetStatus, "not_enforced"),
			artifact.LedgerWrite,
			artifact.AssistantTextPersisted,
			modelGenerationArtifactStreamingRowSuffix(artifact),
			artifact.Path,
			artifact.ArtifactID,
			installRequestDisplayValue(artifact.TextDigest, "none"),
			compactRuntimeRowValue(artifact.TextPreview),
			runtimeRowValueB64(artifact.TextPreview),
		))
		rows = append(rows, fmt.Sprintf(
			"model generation route: mode=metadata-only routing=%s attempts=%d failover=%t selected=%s failed=%s artifact=%s",
			installRequestDisplayValue(artifact.RoutingStrategy, providerRoutingFirstEnabled),
			modelGenerationArtifactAttemptCount(artifact),
			artifact.ProviderFailover,
			installRequestDisplayValue(artifact.SelectedProviderName, artifact.ProviderName),
			providerUsageSummaryRowList(artifact.FailedProviderNames),
			artifact.ArtifactID,
		))
		if len(artifact.RequiredCapabilities) > 0 || len(artifact.SkippedProviders) > 0 {
			rows = append(rows, fmt.Sprintf(
				"model generation capability route: mode=metadata-only required_capabilities=%s capability_match=%t skipped_providers=%s artifact=%s",
				providerCapabilityList(artifact.RequiredCapabilities),
				artifact.CapabilityMatch,
				providerSkippedProvidersSummary(artifact.SkippedProviders),
				artifact.ArtifactID,
			))
		}
	}
	return rows
}

func tuiModelWithModelToolPlanPreviewArtifactRows(root string, model tui.Model) tui.Model {
	artifacts, err := ListModelToolPlanPreviewArtifacts(root)
	if err != nil {
		return model.AppendRuntimeRows(tui.RuntimeTabModelRuntime, []string{
			"model tool plan preview error: mode=metadata-only error=" + compactRuntimeRowValue(err.Error()),
		})
	}
	if len(artifacts) == 0 {
		return model
	}
	return model.AppendRuntimeRows(tui.RuntimeTabModelRuntime, modelToolPlanPreviewArtifactRuntimeRows(artifacts))
}

func modelToolPlanPreviewArtifactRuntimeRows(artifacts []ModelToolPlanPreviewArtifact) []string {
	rows := make([]string, 0, len(artifacts))
	for _, artifact := range artifacts {
		validity := "invalid"
		if artifact.Valid {
			validity = "valid"
		}
		rows = append(rows, fmt.Sprintf(
			"model tool plan preview: source=%s agent=%s role=%s provider=%s model=%s action=%s validity=%s mode=%s digest=%s preview=%s preview_b64=%s path=%s path_b64=%s",
			installRequestDisplayValue(artifact.Source, closedLoopPlannerGenerationStageID),
			installRequestDisplayValue(artifact.AgentID, "closed-loop-planner"),
			installRequestDisplayValue(artifact.Role, "planner"),
			compactRuntimeRowValue(artifact.ProviderName),
			compactRuntimeRowValue(artifact.Model),
			installRequestDisplayValue(string(artifact.Action), "unknown"),
			validity,
			installRequestDisplayValue(artifact.ExecutionMode, "metadata-only"),
			installRequestDisplayValue(artifact.ArtifactDigest, "none"),
			compactRuntimeRowValue(artifact.MarkerPreview),
			runtimeRowValueB64(artifact.MarkerPreview),
			compactRuntimeRowValue(artifact.Path),
			runtimeRowValueB64(artifact.Path),
		))
	}
	return rows
}

func tuiModelWithModelToolPlanPreviewAcceptanceRows(root string, model tui.Model) tui.Model {
	acceptances, err := ListModelToolPlanPreviewAcceptances(root)
	if err != nil {
		return model.AppendRuntimeRows(tui.RuntimeTabModelRuntime, []string{
			"model tool plan preview acceptance error: mode=metadata-only error=" + compactRuntimeRowValue(err.Error()),
		})
	}
	if len(acceptances) == 0 {
		return model
	}
	return model.AppendRuntimeRows(tui.RuntimeTabModelRuntime, modelToolPlanPreviewAcceptanceRuntimeRows(acceptances))
}

func modelToolPlanPreviewAcceptanceRuntimeRows(acceptances []ModelToolPlanPreviewAcceptance) []string {
	rows := []string{fmt.Sprintf("model tool plan preview acceptances: total=%d", len(acceptances))}
	for _, acceptance := range acceptances {
		rows = append(rows, fmt.Sprintf(
			"model tool plan preview acceptance artifact: status=%s mode=%s accepted_by=%s accepted_by_b64=%s accepted_at=%s reason=%s reason_b64=%s action=%s nodes=%d warnings=%d assistant_text_persisted=%t executed_tools=%d worker_dispatch=%t target_contact=%t execution_dispatch_created=%t preview=%s preview_b64=%s review=%s review_b64=%s selection=%s selection_b64=%s path=%s path_b64=%s digest=%s",
			installRequestDisplayValue(acceptance.Status, "accepted"),
			installRequestDisplayValue(acceptance.ExecutionMode, "metadata-only"),
			compactRuntimeRowValue(acceptance.AcceptedBy),
			runtimeRowValueB64(acceptance.AcceptedBy),
			installRequestDisplayValue(acceptance.AcceptedAt, "unknown"),
			compactRuntimeRowValue(acceptance.Reason),
			runtimeRowValueB64(acceptance.Reason),
			installRequestDisplayValue(string(acceptance.Action), "unknown"),
			acceptance.ReviewNodes,
			acceptance.ReviewWarnings,
			acceptance.AssistantTextPersisted,
			acceptance.ExecutedTools,
			acceptance.WorkerDispatch,
			acceptance.TargetContact,
			acceptance.ExecutionDispatchCreated,
			compactRuntimeRowValue(acceptance.PreviewArtifactPath),
			runtimeRowValueB64(acceptance.PreviewArtifactPath),
			compactRuntimeRowValue(acceptance.ReviewPath),
			runtimeRowValueB64(acceptance.ReviewPath),
			compactRuntimeRowValue(acceptance.SelectionPath),
			runtimeRowValueB64(acceptance.SelectionPath),
			compactRuntimeRowValue(acceptance.Path),
			runtimeRowValueB64(acceptance.Path),
			installRequestDisplayValue(acceptance.AcceptanceDigest, "none"),
		))
	}
	return rows
}

func modelGenerationArtifactStreamingRowSuffix(artifact ModelGenerationArtifact) string {
	if !artifact.Streaming {
		return ""
	}
	return fmt.Sprintf(
		" streaming=true stream_completed=%t stream_chunk_count=%d stream_digest=%s stream_preview=%s stream_preview_b64=%s",
		artifact.StreamCompleted,
		artifact.StreamChunkCount,
		installRequestDisplayValue(artifact.StreamDigest, "none"),
		compactRuntimeRowValue(artifact.StreamPreview),
		runtimeRowValueB64(artifact.StreamPreview),
	)
}

func modelGenerationArtifactAttemptCount(artifact ModelGenerationArtifact) int {
	if artifact.ProviderAttemptCount > 0 {
		return artifact.ProviderAttemptCount
	}
	return 1
}

func providerUsageSummaryRuntimeRows(root string, summary ProviderUsageSummary) []string {
	source := providerUsageDirectory(root)
	return []string{fmt.Sprintf(
		"provider usage summary: mode=metadata-only records=%d providers=%s models=%s provider_count=%d model_count=%d input_tokens=%d output_tokens=%d total_tokens=%d estimated_cost_usd=%.6f estimated_cost_records=%d budget_enforced_records=%d budget_within_limit_records=%d first_created_at=%s last_created_at=%s source_b64=%s",
		summary.RecordCount,
		providerUsageSummaryRowList(summary.Providers),
		providerUsageSummaryRowList(summary.Models),
		len(summary.Providers),
		len(summary.Models),
		summary.InputTokens,
		summary.OutputTokens,
		summary.TotalTokens,
		summary.EstimatedCostUSD,
		summary.EstimatedCostRecordCount,
		summary.BudgetEnforcedCount,
		summary.BudgetWithinLimitCount,
		installRequestDisplayValue(summary.FirstCreatedAt, "none"),
		installRequestDisplayValue(summary.LastCreatedAt, "none"),
		base64.RawURLEncoding.EncodeToString([]byte(source)),
	)}
}

func providerUsageSummaryRowList(values []string) string {
	if len(values) == 0 {
		return "none"
	}
	return strings.Join(values, "|")
}

func tuiModelWithToolchainContractRows(root string, model tui.Model) tui.Model {
	check, err := CheckToolchainContracts(root)
	if err != nil {
		return model.AppendRuntimeRows(tui.RuntimeTabToolchainHealth, []string{
			"toolchain contract: status=blocked mode=metadata-only manifests=0 tools=0 domain_packs=0 patterns=0 semantic_actions=0 errors=1",
			"toolchain contract error: " + compactRuntimeRowValue(err.Error()),
		})
	}
	return model.AppendRuntimeRows(tui.RuntimeTabToolchainHealth, toolchainContractRuntimeRows(check))
}

func toolchainContractRuntimeRows(check ToolchainContractCheck) []string {
	const maxToolchainContractErrorRows = 3
	status := strings.TrimSpace(check.Status)
	if status == "" {
		status = "blocked"
	}
	rows := []string{fmt.Sprintf(
		"toolchain contract: status=%s mode=metadata-only manifests=%d tools=%d domain_packs=%d patterns=%d semantic_actions=%d errors=%d",
		status,
		check.ManifestCount,
		check.ToolCount,
		check.DomainPackCount,
		check.PatternCount,
		check.SemanticActionCount,
		len(check.Errors),
	)}
	for index, validationError := range check.Errors {
		if index >= maxToolchainContractErrorRows {
			remaining := len(check.Errors) - maxToolchainContractErrorRows
			rows = append(rows, fmt.Sprintf("toolchain contract error: additional_errors_omitted=%d run=ouro_toolchain_contract_check", remaining))
			break
		}
		if trimmed := compactRuntimeRowValue(validationError); trimmed != "" {
			rows = append(rows, "toolchain contract error: "+trimmed)
		}
	}
	return rows
}

func tuiModelWithReplayWorkbenchSessionRows(root string, model tui.Model) tui.Model {
	sessions, err := ListReplayWorkbenchSessions(root)
	if err != nil || len(sessions) == 0 {
		return model
	}
	selection, selectionFound, _ := ReadReplayWorkbenchSessionSelection(root)
	return model.AppendRuntimeRows(tui.RuntimeTabTimelineLanes, replayWorkbenchSessionRuntimeRowsWithSelection(sessions, selection, selectionFound))
}

func replayWorkbenchSessionRuntimeRows(sessions []ReplayWorkbenchSessionArtifact) []string {
	return replayWorkbenchSessionRuntimeRowsWithSelection(sessions, ReplayWorkbenchSessionSelection{}, false)
}

func replayWorkbenchSessionRuntimeRowsWithSelection(sessions []ReplayWorkbenchSessionArtifact, selection ReplayWorkbenchSessionSelection, selectionFound bool) []string {
	if len(sessions) == 0 {
		return nil
	}
	statusCounts := map[string]int{}
	annotated := 0
	for _, session := range sessions {
		statusCounts[installRequestDisplayValue(session.Status, "unknown")]++
		if session.Annotation != nil {
			annotated++
		}
	}
	rows := []string{fmt.Sprintf(
		"replay workbench sessions: total=%d saved=%d dry-run=%d annotated=%d",
		len(sessions),
		statusCounts["saved"],
		statusCounts["dry-run"],
		annotated,
	)}
	if selectionFound {
		actorFields := fmt.Sprintf(
			"selected_by=%s selected_at=%s",
			installRequestDisplayValue(selection.SelectedBy, "unknown"),
			installRequestDisplayValue(selection.SelectedAt, "unknown"),
		)
		if replayWorkbenchSessionSelectionIsCleared(selection) {
			actorFields = fmt.Sprintf(
				"cleared_by=%s cleared_at=%s",
				installRequestDisplayValue(selection.ClearedBy, "unknown"),
				installRequestDisplayValue(selection.ClearedAt, "unknown"),
			)
		}
		reason := installRequestDisplayValue(strings.TrimSpace(selection.Reason), "none")
		rows = append(rows, fmt.Sprintf(
			"replay workbench session selection: status=%s mode=%s %s reason=%s reason_b64=%s primary=%s selected=%s runs=%d filters=%d endpoints=%d superseded=%d session=%s",
			installRequestDisplayValue(selection.Status, "current"),
			installRequestDisplayValue(selection.Mode, "metadata-only"),
			actorFields,
			reason,
			runtimeRowValueB64(strings.TrimSpace(selection.Reason)),
			installRequestDisplayValue(selection.PrimaryRunID, "unknown"),
			installRequestDisplayValue(selection.SelectedRunID, "unknown"),
			selection.RunIDs,
			selection.Filters,
			selection.Endpoints,
			len(selection.SupersededPaths),
			installRequestDisplayValue(selection.SessionPath, "unknown"),
		))
		rows = append(rows,
			"replay workbench session selection actor: "+actorFields,
			"replay workbench session selection reason: "+reason,
		)
		for _, path := range selection.SupersededPaths {
			if trimmed := strings.TrimSpace(path); trimmed != "" {
				rows = append(rows, "replay workbench session superseded: "+trimmed)
			}
		}
	}
	for _, session := range sessions {
		decision := "unreviewed"
		reviewer := "unknown"
		reviewedAt := "unknown"
		reviewReason := "none"
		if session.Annotation != nil {
			decision = installRequestDisplayValue(session.Annotation.Decision, "unreviewed")
			reviewer = installRequestDisplayValue(session.Annotation.Reviewer, "unknown")
			reviewedAt = installRequestDisplayValue(session.Annotation.ReviewedAt, "unknown")
			reviewReason = installRequestDisplayValue(strings.TrimSpace(session.Annotation.Reason), "none")
		}
		current := "false"
		if selectionFound && replayWorkbenchSessionSelectionIsCurrent(selection) && strings.TrimSpace(selection.SessionPath) == strings.TrimSpace(session.Path) {
			current = "true"
		}
		rows = append(rows, fmt.Sprintf(
			"replay workbench session artifact: status=%s mode=%s primary=%s selected=%s runs=%d filters=%d endpoints=%d created_by=%s created_at=%s reason=%s reason_b64=%s decision=%s reviewer=%s reviewed_at=%s review_reason=%s review_reason_b64=%s current=%s path=%s",
			installRequestDisplayValue(session.Status, "unknown"),
			installRequestDisplayValue(session.Mode, "metadata-only"),
			installRequestDisplayValue(session.PrimaryRunID, "unknown"),
			installRequestDisplayValue(session.SelectedRunID, "unknown"),
			len(session.RunIDs),
			len(session.Filters),
			len(session.Endpoints),
			installRequestDisplayValue(session.CreatedBy, "unknown"),
			installRequestDisplayValue(session.CreatedAt, "unknown"),
			installRequestDisplayValue(strings.TrimSpace(session.Reason), "none"),
			runtimeRowValueB64(strings.TrimSpace(session.Reason)),
			decision,
			reviewer,
			reviewedAt,
			reviewReason,
			runtimeRowValueB64(reviewReason),
			current,
			installRequestDisplayValue(session.Path, "unknown"),
		))
		if session.Annotation != nil {
			rows = append(rows, fmt.Sprintf(
				"replay workbench session annotation: decision=%s reviewer=%s reviewed_at=%s reason=%s",
				decision,
				reviewer,
				reviewedAt,
				reviewReason,
			))
		}
		rows = append(rows, fmt.Sprintf(
			"replay workbench session review: decision=%s reviewer=%s reviewed_at=%s reason=%s current=%s",
			decision,
			reviewer,
			reviewedAt,
			reviewReason,
			current,
		))
		if reason := strings.TrimSpace(session.Reason); reason != "" {
			rows = append(rows, "replay workbench session reason: "+reason)
		}
		for _, key := range []string{"edge", "lane", "node", "q"} {
			if value := strings.TrimSpace(session.Filters[key]); value != "" {
				rows = append(rows, fmt.Sprintf("replay workbench session filter: %s=%s", key, value))
			}
		}
		for _, key := range []string{"html", "replay", "replays", "sessions", "provider_usage_summary", "graph", "graphs", "graph_ndjson", "timeline_ndjson"} {
			if value := strings.TrimSpace(session.Endpoints[key]); value != "" {
				rows = append(rows, fmt.Sprintf("replay workbench session endpoint: %s=%s", key, value))
			}
		}
	}
	return rows
}

func tuiModelWithReportProjectionDispatchRows(root string, model tui.Model) tui.Model {
	dispatches, err := ListReportProjectionDispatches(root)
	if err != nil || len(dispatches) == 0 {
		return model
	}
	return model.PrependRuntimeRows(tui.RuntimeTabTimelineLanes, reportProjectionDispatchRuntimeRows(dispatches))
}

func reportProjectionDispatchRuntimeRows(dispatches []ReportProjectionDispatch) []string {
	if len(dispatches) == 0 {
		return nil
	}
	rows := []string{fmt.Sprintf("report projection dispatches: total=%d", len(dispatches))}
	for _, dispatch := range dispatches {
		status := installRequestDisplayValue(dispatch.Status, "unknown")
		mode := installRequestDisplayValue(dispatch.Mode, "metadata-only")
		actor := reportProjectionDispatchRowValue(dispatch.Actor, "operator")
		reason := reportProjectionDispatchRowValue(dispatch.Reason, "none")
		rows = append(rows, fmt.Sprintf(
			"report projection dispatch artifact: %s artifact_b64=%s status=%s mode=%s run=%s events=%d planned_report=%s planned_report_b64=%s actor=%s actor_b64=%s created_at=%s created_at_b64=%s reason=%s reason_b64=%s",
			dispatch.Path,
			runtimeRowValueB64(dispatch.Path),
			status,
			mode,
			reportProjectionDispatchRowValue(dispatch.RunID, "unknown"),
			dispatch.EventCount,
			reportProjectionDispatchRowValue(dispatch.ReportPath, "not-generated"),
			runtimeRowValueB64(dispatch.ReportPath),
			actor,
			runtimeRowValueB64(dispatch.Actor),
			reportProjectionDispatchRowValue(dispatch.CreatedAt, "unknown"),
			runtimeRowValueB64(dispatch.CreatedAt),
			reason,
			runtimeRowValueB64(dispatch.Reason),
		))
		rows = append(rows, fmt.Sprintf(
			"report projection dispatch compact: status=%s mode=%s run=%s events=%d actor=%s reason=%s",
			status,
			mode,
			reportProjectionDispatchRowValue(dispatch.RunID, "unknown"),
			dispatch.EventCount,
			actor,
			reason,
		))
		if dispatch.Path != "" {
			rows = append(rows, "report projection dispatch path: "+filepath.Base(dispatch.Path))
		}
	}
	return rows
}

func reportProjectionDispatchRowValue(value string, fallback string) string {
	value = strings.Join(strings.Fields(value), "_")
	if value == "" {
		return fallback
	}
	return value
}

func tuiModelWithReportProjectionDispatchExecutionRows(root string, model tui.Model) tui.Model {
	executions, err := ListReportProjectionDispatchExecutions(root)
	if err != nil || len(executions) == 0 {
		return model
	}
	selection, selectionFound, _ := ReadReportProjectionDispatchExecutionSelection(root)
	return model.PrependRuntimeRows(tui.RuntimeTabTimelineLanes, reportProjectionDispatchExecutionRuntimeRowsWithSelection(executions, selection, selectionFound))
}

func reportProjectionDispatchExecutionRuntimeRows(executions []ReportProjectionDispatchExecution) []string {
	return reportProjectionDispatchExecutionRuntimeRowsWithSelection(executions, ReportProjectionDispatchExecutionSelection{}, false)
}

func reportProjectionDispatchExecutionRuntimeRowsWithSelection(executions []ReportProjectionDispatchExecution, selection ReportProjectionDispatchExecutionSelection, selectionFound bool) []string {
	if len(executions) == 0 {
		return nil
	}
	annotated := 0
	current := 0
	for _, execution := range executions {
		if execution.Annotation != nil {
			annotated++
		}
		if selectionFound && strings.TrimSpace(selection.Status) == "current" && strings.TrimSpace(selection.ExecutionPath) == strings.TrimSpace(execution.Path) {
			current++
		}
	}
	rows := []string{fmt.Sprintf("report projection dispatch executions: total=%d annotated=%d current=%d", len(executions), annotated, current)}
	if selectionFound {
		selectionActor := reportProjectionDispatchRowValue(selection.SelectedBy, "unknown")
		selectionAt := reportProjectionDispatchRowValue(selection.SelectedAt, "unknown")
		actorFields := fmt.Sprintf(
			"selected_by=%s selected_by_b64=%s selected_at=%s selected_at_b64=%s",
			selectionActor,
			runtimeRowValueB64(selection.SelectedBy),
			selectionAt,
			runtimeRowValueB64(selection.SelectedAt),
		)
		if strings.TrimSpace(selection.Status) == "cleared" {
			selectionActor = reportProjectionDispatchRowValue(selection.ClearedBy, "unknown")
			selectionAt = reportProjectionDispatchRowValue(selection.ClearedAt, "unknown")
			actorFields = fmt.Sprintf(
				"cleared_by=%s cleared_by_b64=%s cleared_at=%s cleared_at_b64=%s",
				selectionActor,
				runtimeRowValueB64(selection.ClearedBy),
				selectionAt,
				runtimeRowValueB64(selection.ClearedAt),
			)
		}
		selectionStatus := reportProjectionDispatchRowValue(selection.Status, "unknown")
		isCurrent := strings.TrimSpace(selection.Status) == "current"
		rows = append(rows, fmt.Sprintf(
			"report projection execution selection: status=%s mode=%s current=%t %s reason=%s reason_b64=%s execution=%s execution_b64=%s selection=%s selection_b64=%s dispatch=%s dispatch_b64=%s run=%s report=%s report_b64=%s written=%t events=%d superseded=%d",
			selectionStatus,
			reportProjectionDispatchRowValue(selection.Mode, "metadata-only"),
			isCurrent,
			actorFields,
			reportProjectionDispatchRowValue(selection.Reason, "none"),
			runtimeRowValueB64(selection.Reason),
			reportProjectionDispatchRowValue(selection.ExecutionPath, "unknown"),
			runtimeRowValueB64(selection.ExecutionPath),
			reportProjectionDispatchRowValue(selection.Path, "unknown"),
			runtimeRowValueB64(selection.Path),
			reportProjectionDispatchRowValue(selection.DispatchPath, "unknown"),
			runtimeRowValueB64(selection.DispatchPath),
			reportProjectionDispatchRowValue(selection.RunID, "unknown"),
			reportProjectionDispatchRowValue(selection.ReportPath, "not-generated"),
			runtimeRowValueB64(selection.ReportPath),
			selection.Written,
			selection.EventCount,
			len(selection.SupersededPaths),
		))
		rows = append(rows, fmt.Sprintf(
			"report projection selection compact: status=%s actor=%s current=%t at=%s reason=%s",
			selectionStatus,
			selectionActor,
			isCurrent,
			selectionAt,
			reportProjectionDispatchRowValue(selection.Reason, "none"),
		))
		for _, path := range selection.SupersededPaths {
			if trimmed := strings.TrimSpace(path); trimmed != "" {
				rows = append(rows, "report projection execution superseded: "+trimmed)
			}
		}
	}
	for _, execution := range executions {
		status := installRequestDisplayValue(execution.Status, "unknown")
		mode := installRequestDisplayValue(execution.Mode, "execute")
		actor := reportProjectionDispatchRowValue(execution.Actor, "operator")
		reason := reportProjectionDispatchRowValue(execution.Reason, "none")
		executedAt := reportProjectionDispatchRowValue(execution.ExecutedAt, "unknown")
		decision := "unreviewed"
		reviewer := "unknown"
		reviewedAt := "unknown"
		reviewReason := "none"
		reviewerRaw := ""
		reviewedAtRaw := ""
		reviewReasonRaw := ""
		if execution.Annotation != nil {
			decision = reportProjectionDispatchRowValue(execution.Annotation.Decision, "unreviewed")
			reviewer = reportProjectionDispatchRowValue(execution.Annotation.Reviewer, "unknown")
			reviewedAt = reportProjectionDispatchRowValue(execution.Annotation.ReviewedAt, "unknown")
			reviewReason = reportProjectionDispatchRowValue(execution.Annotation.Reason, "none")
			reviewerRaw = execution.Annotation.Reviewer
			reviewedAtRaw = execution.Annotation.ReviewedAt
			reviewReasonRaw = execution.Annotation.Reason
		}
		currentValue := "false"
		if selectionFound && strings.TrimSpace(selection.Status) == "current" && strings.TrimSpace(selection.ExecutionPath) == strings.TrimSpace(execution.Path) {
			currentValue = "true"
		}
		rows = append(rows, fmt.Sprintf(
			"report projection dispatch execution artifact: %s artifact_b64=%s status=%s mode=%s dispatch=%s dispatch_b64=%s run=%s events=%d report=%s report_b64=%s written=%t actor=%s actor_b64=%s executed_at=%s executed_at_b64=%s reason=%s reason_b64=%s decision=%s reviewer=%s reviewer_b64=%s reviewed_at=%s reviewed_at_b64=%s review_reason=%s review_reason_b64=%s current=%s",
			execution.Path,
			runtimeRowValueB64(execution.Path),
			status,
			mode,
			reportProjectionDispatchRowValue(execution.DispatchPath, "unknown"),
			runtimeRowValueB64(execution.DispatchPath),
			reportProjectionDispatchRowValue(execution.RunID, "unknown"),
			execution.EventCount,
			reportProjectionDispatchRowValue(execution.ReportPath, "not-generated"),
			runtimeRowValueB64(execution.ReportPath),
			execution.Written,
			actor,
			runtimeRowValueB64(execution.Actor),
			executedAt,
			runtimeRowValueB64(execution.ExecutedAt),
			reason,
			runtimeRowValueB64(execution.Reason),
			decision,
			reviewer,
			runtimeRowValueB64(reviewerRaw),
			reviewedAt,
			runtimeRowValueB64(reviewedAtRaw),
			reviewReason,
			runtimeRowValueB64(reviewReasonRaw),
			currentValue,
		))
		rows = append(rows, fmt.Sprintf(
			"report projection dispatch execution compact: status=%s mode=%s run=%s events=%d written=%t actor=%s reason=%s current=%s",
			status,
			mode,
			reportProjectionDispatchRowValue(execution.RunID, "unknown"),
			execution.EventCount,
			execution.Written,
			actor,
			reason,
			currentValue,
		))
		if execution.Annotation != nil {
			rows = append(rows, fmt.Sprintf(
				"report projection execution review: mode=metadata-only decision=%s reviewer=%s reviewer_b64=%s reviewed_at=%s reviewed_at_b64=%s review_reason=%s review_reason_b64=%s execution=%s execution_b64=%s dispatch=%s dispatch_b64=%s run=%s report=%s report_b64=%s review=%s review_b64=%s",
				decision,
				reviewer,
				runtimeRowValueB64(reviewerRaw),
				reviewedAt,
				runtimeRowValueB64(reviewedAtRaw),
				reviewReason,
				runtimeRowValueB64(reviewReasonRaw),
				reportProjectionDispatchRowValue(execution.Path, "unknown"),
				runtimeRowValueB64(execution.Path),
				reportProjectionDispatchRowValue(execution.DispatchPath, "unknown"),
				runtimeRowValueB64(execution.DispatchPath),
				reportProjectionDispatchRowValue(execution.RunID, "unknown"),
				reportProjectionDispatchRowValue(execution.ReportPath, "not-generated"),
				runtimeRowValueB64(execution.ReportPath),
				reportProjectionDispatchRowValue(execution.Path, "unknown"),
				runtimeRowValueB64(execution.Path),
			))
			rows = append(rows, fmt.Sprintf(
				"report projection review compact: decision=%s reviewer=%s reviewed_at=%s reason=%s",
				decision,
				reviewer,
				reviewedAt,
				reviewReason,
			))
			rows = append(rows, fmt.Sprintf(
				"report projection dispatch execution annotation: decision=%s reviewer=%s reviewed_at=%s reason=%s reason_b64=%s",
				decision,
				reviewer,
				reviewedAt,
				reviewReason,
				runtimeRowValueB64(execution.Annotation.Reason),
			))
		}
		if execution.Path != "" {
			rows = append(rows, "report projection dispatch execution path: "+filepath.Base(execution.Path))
		}
	}
	return rows
}

func reportProjectionDispatchExecutionRuntimeRowPrefixes() []string {
	return []string{
		"report projection dispatch executions:",
		"report projection execution selection:",
		"report projection selection compact:",
		"report projection execution superseded:",
		"report projection dispatch execution artifact:",
		"report projection dispatch execution compact:",
		"report projection execution review:",
		"report projection review compact:",
		"report projection dispatch execution annotation:",
		"report projection dispatch execution path:",
	}
}

func tuiModelWithReplayWorkbenchCollaborationRows(root string, model tui.Model) tui.Model {
	collaborations, err := ListReplayWorkbenchCollaborations(root)
	if err != nil || len(collaborations) == 0 {
		return model
	}
	selection, selectionFound, _ := ReadReplayWorkbenchCollaborationSelection(root)
	return model.AppendRuntimeRows(tui.RuntimeTabTimelineLanes, replayWorkbenchCollaborationRuntimeRowsWithSelection(collaborations, selection, selectionFound))
}

func replayWorkbenchCollaborationRuntimeRows(collaborations []ReplayWorkbenchCollaborationArtifact) []string {
	return replayWorkbenchCollaborationRuntimeRowsWithSelection(collaborations, ReplayWorkbenchCollaborationSelection{}, false)
}

func replayWorkbenchCollaborationRuntimeRowsWithSelection(collaborations []ReplayWorkbenchCollaborationArtifact, selection ReplayWorkbenchCollaborationSelection, selectionFound bool) []string {
	if len(collaborations) == 0 {
		return nil
	}
	statusCounts := map[string]int{}
	participants := 0
	for _, collaboration := range collaborations {
		statusCounts[installRequestDisplayValue(collaboration.Status, "unknown")]++
		participants += len(collaboration.Participants)
	}
	rows := []string{fmt.Sprintf(
		"replay workbench collaborations: total=%d saved=%d dry-run=%d participants=%d",
		len(collaborations),
		statusCounts["saved"],
		statusCounts["dry-run"],
		participants,
	)}
	currentPath := ""
	if selectionFound {
		actorFields := fmt.Sprintf("selected_by=%s selected_at=%s", selection.SelectedBy, selection.SelectedAt)
		if selection.Status == "cleared" {
			actorFields = fmt.Sprintf("cleared_by=%s cleared_at=%s", selection.ClearedBy, selection.ClearedAt)
		}
		rows = append(rows, fmt.Sprintf(
			"replay workbench collaboration selection: status=%s mode=%s collaboration=%s session=%s primary=%s selected=%s runs=%d filters=%d endpoints=%d participants=%d superseded=%d %s reason=%s reason_b64=%s",
			installRequestDisplayValue(selection.Status, "unknown"),
			installRequestDisplayValue(selection.Mode, "metadata-only"),
			installRequestDisplayValue(selection.CollaborationPath, "unknown"),
			installRequestDisplayValue(selection.SessionPath, "unknown"),
			installRequestDisplayValue(selection.PrimaryRunID, "unknown"),
			installRequestDisplayValue(selection.SelectedRunID, "unknown"),
			selection.RunIDs,
			selection.Filters,
			selection.Endpoints,
			selection.Participants,
			len(selection.SupersededPaths),
			actorFields,
			installRequestDisplayValue(strings.TrimSpace(selection.Reason), "none"),
			runtimeRowValueB64(strings.TrimSpace(selection.Reason)),
		))
		if selection.Status == "current" {
			currentPath = selection.CollaborationPath
			rows = append(rows, "replay workbench collaboration current marker: current=true collaboration="+installRequestDisplayValue(selection.CollaborationPath, "unknown"))
			rows = append(rows, fmt.Sprintf("replay workbench collaboration selection actor: selected_by=%s selected_at=%s", selection.SelectedBy, selection.SelectedAt))
		}
		if selection.Status == "cleared" {
			rows = append(rows, fmt.Sprintf("replay workbench collaboration selection actor: cleared_by=%s cleared_at=%s", selection.ClearedBy, selection.ClearedAt))
		}
		if reason := strings.TrimSpace(selection.Reason); reason != "" {
			rows = append(rows, "replay workbench collaboration selection reason: "+reason)
		}
		for _, path := range selection.SupersededPaths {
			rows = append(rows, "replay workbench collaboration superseded: "+path)
		}
	}
	if selectionFound {
		nonCurrent := len(collaborations)
		if currentPath != "" {
			nonCurrent--
		}
		if nonCurrent < 0 {
			nonCurrent = 0
		}
		rows = append(rows, fmt.Sprintf("replay workbench collaboration noncurrent marker: current=false count=%d", nonCurrent))
	}
	for _, collaboration := range collaborations {
		current := "false"
		if currentPath != "" && collaboration.Path == currentPath {
			current = "true"
		}
		decision := "unreviewed"
		reviewer := "unknown"
		reviewedAt := "unknown"
		reviewReason := "none"
		if collaboration.Annotation != nil {
			decision = installRequestDisplayValue(collaboration.Annotation.Decision, "unreviewed")
			reviewer = installRequestDisplayValue(collaboration.Annotation.Reviewer, "unknown")
			reviewedAt = installRequestDisplayValue(collaboration.Annotation.ReviewedAt, "unknown")
			reviewReason = installRequestDisplayValue(strings.TrimSpace(collaboration.Annotation.Reason), "none")
		}
		rows = append(rows, fmt.Sprintf(
			"replay workbench collaboration artifact: status=%s mode=%s primary=%s selected=%s runs=%d filters=%d endpoints=%d participants=%d created_by=%s created_at=%s reason=%s reason_b64=%s decision=%s reviewer=%s reviewed_at=%s review_reason=%s review_reason_b64=%s current=%s path=%s session_path=%s",
			installRequestDisplayValue(collaboration.Status, "unknown"),
			installRequestDisplayValue(collaboration.Mode, "metadata-only"),
			installRequestDisplayValue(collaboration.PrimaryRunID, "unknown"),
			installRequestDisplayValue(collaboration.SelectedRunID, "unknown"),
			collaboration.RunIDs,
			collaboration.Filters,
			len(collaboration.Endpoints),
			len(collaboration.Participants),
			installRequestDisplayValue(collaboration.CreatedBy, "unknown"),
			installRequestDisplayValue(collaboration.CreatedAt, "unknown"),
			installRequestDisplayValue(strings.TrimSpace(collaboration.Reason), "none"),
			runtimeRowValueB64(strings.TrimSpace(collaboration.Reason)),
			decision,
			reviewer,
			reviewedAt,
			reviewReason,
			runtimeRowValueB64(reviewReason),
			current,
			installRequestDisplayValue(collaboration.Path, "unknown"),
			installRequestDisplayValue(collaboration.SessionPath, "unknown"),
		))
		if collaboration.Annotation != nil {
			rows = append(rows, fmt.Sprintf(
				"replay workbench collaboration annotation: decision=%s reviewer=%s reviewed_at=%s reason=%s",
				decision,
				reviewer,
				reviewedAt,
				reviewReason,
			))
		}
		rows = append(rows, fmt.Sprintf(
			"replay workbench collaboration review: decision=%s reviewer=%s reviewed_at=%s reason=%s current=%s",
			decision,
			reviewer,
			reviewedAt,
			reviewReason,
			current,
		))
		if reason := strings.TrimSpace(collaboration.Reason); reason != "" {
			rows = append(rows, "replay workbench collaboration reason: "+reason)
		}
		for _, participant := range collaboration.Participants {
			if trimmed := strings.TrimSpace(participant); trimmed != "" {
				rows = append(rows, "replay workbench collaboration participant: "+trimmed)
			}
		}
		for _, key := range []string{"collaborations", "session", "sessions", "html", "replay", "replays", "graph", "graphs", "graph_ndjson", "timeline_ndjson"} {
			if value := strings.TrimSpace(collaboration.Endpoints[key]); value != "" {
				rows = append(rows, fmt.Sprintf("replay workbench collaboration endpoint: %s=%s", key, value))
			}
		}
	}
	return rows
}

func tuiModelWithReplayWorkbenchCollaborationPresenceRows(root string, model tui.Model) tui.Model {
	presence, err := ListReplayWorkbenchCollaborationPresence(root)
	if err != nil || len(presence) == 0 {
		return model
	}
	return model.AppendRuntimeRows(tui.RuntimeTabTimelineLanes, replayWorkbenchCollaborationPresenceRuntimeRows(presence))
}

func replayWorkbenchCollaborationPresenceRuntimeRows(presence []ReplayWorkbenchCollaborationPresence) []string {
	if len(presence) == 0 {
		return nil
	}
	statusCounts := map[string]int{}
	for _, item := range presence {
		statusCounts[installRequestDisplayValue(item.Status, "unknown")]++
	}
	rows := []string{fmt.Sprintf(
		"replay workbench collaboration presence: total=%d online=%d away=%d offline=%d",
		len(presence),
		statusCounts["online"],
		statusCounts["away"],
		statusCounts["offline"],
	)}
	for _, item := range presence {
		status := installRequestDisplayValue(item.Status, "unknown")
		participant := installRequestDisplayValue(item.Participant, "unknown")
		seenBy := installRequestDisplayValue(item.SeenBy, "unknown")
		seenAt := installRequestDisplayValue(item.SeenAt, "unknown")
		reason := installRequestDisplayValue(strings.TrimSpace(item.Reason), "none")
		rows = append(rows, fmt.Sprintf(
			"replay workbench collaboration presence artifact: status=%s mode=%s primary=%s selected=%s participant=%s seen_by=%s seen_at=%s reason=%s reason_b64=%s path=%s collaboration=%s session_path=%s",
			status,
			installRequestDisplayValue(item.Mode, "metadata-only"),
			installRequestDisplayValue(item.PrimaryRunID, "unknown"),
			installRequestDisplayValue(item.SelectedRunID, "unknown"),
			participant,
			seenBy,
			seenAt,
			reason,
			runtimeRowValueB64(strings.TrimSpace(item.Reason)),
			installRequestDisplayValue(item.Path, "unknown"),
			installRequestDisplayValue(item.CollaborationPath, "unknown"),
			installRequestDisplayValue(item.SessionPath, "unknown"),
		))
		rows = append(rows, fmt.Sprintf(
			"replay workbench collaboration presence participant: %s status=%s seen_by=%s seen_at=%s reason=%s",
			participant,
			status,
			seenBy,
			seenAt,
			reason,
		))
		if strings.TrimSpace(item.Reason) != "" {
			rows = append(rows, "replay workbench collaboration presence reason: "+strings.TrimSpace(item.Reason))
		}
	}
	return rows
}

func tuiModelWithReplayWorkbenchCollaborationNoteRows(root string, model tui.Model) tui.Model {
	notes, err := ListReplayWorkbenchCollaborationNotes(root)
	if err != nil || len(notes) == 0 {
		return model
	}
	selection, selectionFound, _ := ReadReplayWorkbenchCollaborationNoteSelection(root)
	return model.AppendRuntimeRows(tui.RuntimeTabTimelineLanes, replayWorkbenchCollaborationNoteRuntimeRowsWithSelection(notes, selection, selectionFound))
}

func replayWorkbenchCollaborationNoteRuntimeRows(notes []ReplayWorkbenchCollaborationNote) []string {
	return replayWorkbenchCollaborationNoteRuntimeRowsWithSelection(notes, ReplayWorkbenchCollaborationNoteSelection{}, false)
}

func replayWorkbenchCollaborationNoteRuntimeRowsWithSelection(notes []ReplayWorkbenchCollaborationNote, selection ReplayWorkbenchCollaborationNoteSelection, selectionFound bool) []string {
	if len(notes) == 0 {
		return nil
	}
	statusCounts := map[string]int{}
	authors := map[string]bool{}
	for _, note := range notes {
		statusCounts[installRequestDisplayValue(note.Status, "unknown")]++
		if author := strings.TrimSpace(note.Author); author != "" {
			authors[author] = true
		}
	}
	rows := []string{fmt.Sprintf(
		"replay workbench collaboration notes: total=%d saved=%d dry-run=%d authors=%d",
		len(notes),
		statusCounts["saved"],
		statusCounts["dry-run"],
		len(authors),
	)}
	currentPath := ""
	if selectionFound {
		actorFields := fmt.Sprintf("selected_by=%s selected_at=%s", selection.SelectedBy, selection.SelectedAt)
		if selection.Status == "cleared" {
			actorFields = fmt.Sprintf("cleared_by=%s cleared_at=%s", selection.ClearedBy, selection.ClearedAt)
		}
		rows = append(rows, fmt.Sprintf(
			"replay workbench collaboration note selection: status=%s mode=%s note=%s collaboration=%s session=%s primary=%s selected=%s author=%s target=%s target_kind=%s target_id=%s superseded=%d %s reason=%s reason_b64=%s",
			installRequestDisplayValue(selection.Status, "unknown"),
			installRequestDisplayValue(selection.Mode, "metadata-only"),
			installRequestDisplayValue(selection.NotePath, "unknown"),
			installRequestDisplayValue(selection.CollaborationPath, "unknown"),
			installRequestDisplayValue(selection.SessionPath, "unknown"),
			installRequestDisplayValue(selection.PrimaryRunID, "unknown"),
			installRequestDisplayValue(selection.SelectedRunID, "unknown"),
			installRequestDisplayValue(selection.Author, "operator"),
			replayWorkbenchCollaborationNoteSelectionTarget(selection),
			installRequestDisplayValue(selection.TargetKind, "none"),
			installRequestDisplayValue(selection.TargetID, "none"),
			len(selection.SupersededPaths),
			actorFields,
			installRequestDisplayValue(strings.TrimSpace(selection.Reason), "none"),
			runtimeRowValueB64(strings.TrimSpace(selection.Reason)),
		))
		if selection.Status == "current" {
			currentPath = selection.NotePath
			rows = append(rows, "replay workbench collaboration note current marker: current=true note="+installRequestDisplayValue(selection.NotePath, "unknown"))
			rows = append(rows, fmt.Sprintf("replay workbench collaboration note selection actor: selected_by=%s selected_at=%s", selection.SelectedBy, selection.SelectedAt))
		}
		if selection.Status == "cleared" {
			rows = append(rows, fmt.Sprintf("replay workbench collaboration note selection actor: cleared_by=%s cleared_at=%s", selection.ClearedBy, selection.ClearedAt))
		}
		if reason := strings.TrimSpace(selection.Reason); reason != "" {
			rows = append(rows, "replay workbench collaboration note selection reason: "+reason)
		}
		for _, path := range selection.SupersededPaths {
			rows = append(rows, "replay workbench collaboration note superseded: "+path)
		}
	}
	if selectionFound {
		nonCurrent := len(notes)
		if currentPath != "" {
			nonCurrent--
		}
		if nonCurrent < 0 {
			nonCurrent = 0
		}
		rows = append(rows, fmt.Sprintf("replay workbench collaboration note noncurrent marker: current=false count=%d", nonCurrent))
	}
	for _, note := range notes {
		target := replayWorkbenchCollaborationNoteTarget(note)
		reason := installRequestDisplayValue(strings.TrimSpace(note.Reason), "none")
		decision, reviewer, reviewedAt, reviewReason := replayWorkbenchCollaborationNoteReviewFields(note)
		current := "false"
		if currentPath != "" && note.Path == currentPath {
			current = "true"
		}
		rows = append(rows, fmt.Sprintf(
			"replay workbench collaboration note artifact: status=%s mode=%s primary=%s selected=%s author=%s target=%s target_kind=%s target_id=%s created_at=%s reason=%s reason_b64=%s body=%s body_b64=%s decision=%s reviewer=%s reviewed_at=%s review_reason=%s review_reason_b64=%s current=%s path=%s collaboration=%s session_path=%s",
			installRequestDisplayValue(note.Status, "unknown"),
			installRequestDisplayValue(note.Mode, "metadata-only"),
			installRequestDisplayValue(note.PrimaryRunID, "unknown"),
			installRequestDisplayValue(note.SelectedRunID, "unknown"),
			installRequestDisplayValue(note.Author, "operator"),
			target,
			installRequestDisplayValue(note.TargetKind, "none"),
			installRequestDisplayValue(note.TargetID, "none"),
			installRequestDisplayValue(note.CreatedAt, "unknown"),
			reason,
			runtimeRowValueB64(strings.TrimSpace(note.Reason)),
			installRequestDisplayValue(strings.TrimSpace(note.Body), "none"),
			runtimeRowValueB64(strings.TrimSpace(note.Body)),
			decision,
			reviewer,
			reviewedAt,
			reviewReason,
			runtimeRowValueB64(reviewReason),
			current,
			installRequestDisplayValue(note.Path, "unknown"),
			installRequestDisplayValue(note.CollaborationPath, "unknown"),
			installRequestDisplayValue(note.SessionPath, "unknown"),
		))
		rows = append(rows, fmt.Sprintf(
			"replay workbench collaboration note compact: author=%s target=%s created_at=%s reason=%s body=%s",
			installRequestDisplayValue(note.Author, "operator"),
			target,
			installRequestDisplayValue(note.CreatedAt, "unknown"),
			reason,
			installRequestDisplayValue(strings.TrimSpace(note.Body), "none"),
		))
		rows = append(rows, fmt.Sprintf(
			"replay workbench collaboration note review: decision=%s reviewer=%s reviewed_at=%s reason=%s",
			decision,
			reviewer,
			reviewedAt,
			reviewReason,
		))
		rows = append(rows, fmt.Sprintf(
			"replay workbench collaboration note author: %s target=%s reason=%s",
			installRequestDisplayValue(note.Author, "operator"),
			target,
			reason,
		))
		if body := strings.TrimSpace(note.Body); body != "" {
			rows = append(rows, "replay workbench collaboration note body: "+body)
		}
	}
	return rows
}

func replayWorkbenchCollaborationNoteSelectionTarget(selection ReplayWorkbenchCollaborationNoteSelection) string {
	kind := strings.TrimSpace(selection.TargetKind)
	id := strings.TrimSpace(selection.TargetID)
	if kind == "" && id == "" {
		return "none"
	}
	if kind == "" {
		return id
	}
	if id == "" {
		return kind
	}
	return kind + ":" + id
}

func replayWorkbenchCollaborationNoteReviewFields(note ReplayWorkbenchCollaborationNote) (string, string, string, string) {
	if note.Annotation == nil {
		return "unreviewed", "unknown", "unknown", "none"
	}
	return installRequestDisplayValue(note.Annotation.Decision, "unreviewed"),
		installRequestDisplayValue(note.Annotation.Reviewer, "unknown"),
		installRequestDisplayValue(note.Annotation.ReviewedAt, "unknown"),
		installRequestDisplayValue(note.Annotation.Reason, "none")
}

func replayWorkbenchCollaborationNoteTarget(note ReplayWorkbenchCollaborationNote) string {
	kind := strings.TrimSpace(note.TargetKind)
	id := strings.TrimSpace(note.TargetID)
	if kind == "" && id == "" {
		return "none"
	}
	if kind == "" {
		return id
	}
	if id == "" {
		return kind
	}
	return kind + ":" + id
}

func tuiModelWithCompilePreviewSignalRows(root string, model tui.Model) tui.Model {
	signals, found, err := LoadToolCompilePreviewSignals(root)
	if err != nil || !found {
		return model
	}
	return model.AppendRuntimeRows(tui.RuntimeTabToolchainHealth, compilePreviewSignalRuntimeRows(root, signals))
}

func tuiModelWithCompilePreviewReviewRows(root string, model tui.Model) tui.Model {
	reviews, err := ListToolCompilePreviewReviews(root)
	if err != nil || len(reviews) == 0 {
		return model
	}
	selection, selectionFound, _ := ReadToolCompilePreviewReviewSelection(root)
	rows := compilePreviewReviewRuntimeRowsWithSelection(reviews, selection, selectionFound)
	if len(reviews) >= 2 {
		comparison := compareToolCompilePreviewReviewValues(reviews[len(reviews)-2], reviews[len(reviews)-1])
		rows = append(rows, compilePreviewReviewComparisonRuntimeRows(comparison)...)
	}
	return model.AppendRuntimeRows(tui.RuntimeTabToolchainHealth, rows)
}

func tuiModelWithCompilePreviewReviewExecutionRows(root string, model tui.Model) tui.Model {
	executions, err := ListToolCompilePreviewReviewExecutions(root)
	if err != nil || len(executions) == 0 {
		return model
	}
	return model.AppendRuntimeRows(tui.RuntimeTabToolchainHealth, compilePreviewReviewExecutionRuntimeRows(executions))
}

func tuiModelWithCompilePreviewReviewExecutionDispatchRows(root string, model tui.Model) tui.Model {
	dispatches, err := ListToolCompilePreviewReviewExecutionDispatches(root)
	if err != nil || len(dispatches) == 0 {
		return model
	}
	return model.AppendRuntimeRows(tui.RuntimeTabToolchainHealth, compilePreviewReviewExecutionDispatchRuntimeRows(dispatches))
}

func tuiModelWithCompilePreviewRows(root string, model tui.Model) tui.Model {
	if hasToolchainReviewRows(root) {
		return model
	}
	preview, err := PreviewToolCompilePlan(root, ToolCompilePreviewOptions{})
	if err != nil {
		return model
	}
	return model.AppendRuntimeRows(tui.RuntimeTabToolchainHealth, compilePreviewRuntimeRows(preview))
}

func hasToolchainReviewRows(root string) bool {
	if requests, err := ListToolchainInstallRequests(root); err == nil && len(requests) > 0 {
		return true
	}
	if dispatches, err := ListToolchainInstallDispatches(root); err == nil && len(dispatches) > 0 {
		return true
	}
	if executions, err := ListToolchainInstallDispatchExecutions(root); err == nil && len(executions) > 0 {
		return true
	}
	if retryDispatches, err := ListToolchainInstallRetryDispatches(root); err == nil && len(retryDispatches) > 0 {
		return true
	}
	if retryExecutions, err := ListToolchainInstallRetryDispatchExecutions(root); err == nil && len(retryExecutions) > 0 {
		return true
	}
	if orchestrations, err := ListToolchainInstallOrchestrations(root); err == nil && len(orchestrations) > 0 {
		return true
	}
	if reviews, err := ListToolCompilePreviewReviews(root); err == nil && len(reviews) > 0 {
		return true
	}
	if dispatches, err := ListToolCompilePreviewReviewExecutionDispatches(root); err == nil && len(dispatches) > 0 {
		return true
	}
	if executions, err := ListToolCompilePreviewReviewExecutions(root); err == nil && len(executions) > 0 {
		return true
	}
	if requests, err := ListManagedToolBundleUpdateRequests(root); err == nil && len(requests) > 0 {
		return true
	}
	return false
}

func compilePreviewSignalRuntimeRows(root string, signals ToolCompilePreviewSignals) []string {
	path := toolCompilePreviewSignalsPath(root)
	rows := []string{fmt.Sprintf(
		"compile-preview signals: present mode=metadata-only max_plan_nodes=%d domain_priorities=%d frontier_scores=%d historical_stats=%d path=%s",
		signals.Budget.MaxPlanNodes,
		len(signals.DomainPriorities),
		len(signals.FrontierScores),
		len(signals.HistoricalStats),
		path,
	)}
	if signals.Budget.MaxPlanNodes > 0 {
		rows = append(rows, fmt.Sprintf("compile-preview signal budget: max_plan_nodes=%d", signals.Budget.MaxPlanNodes))
	}
	for _, priority := range signals.DomainPriorities {
		domain := strings.TrimSpace(priority.Domain)
		if domain == "" {
			continue
		}
		rows = append(rows, fmt.Sprintf(
			"compile-preview signal domain: domain=%s weight=%s",
			domain,
			strconv.FormatFloat(priority.Weight, 'g', -1, 64),
		))
	}
	for _, score := range signals.FrontierScores {
		manifest := strings.TrimSpace(score.ManifestName)
		capability := strings.TrimSpace(score.Capability)
		pathID := strings.TrimSpace(score.Path.ID)
		target := strings.TrimSpace(score.Path.Target)
		rows = append(rows, fmt.Sprintf(
			"compile-preview signal frontier: manifest=%s capability=%s path=%s target=%s score=%s",
			installRequestDisplayValue(manifest, "unknown"),
			installRequestDisplayValue(capability, "unknown"),
			installRequestDisplayValue(pathID, "unknown"),
			installRequestDisplayValue(target, "unknown"),
			strconv.FormatFloat(frontier.Score(score.Path.toCandidatePath()), 'g', -1, 64),
		))
	}
	for _, history := range signals.HistoricalStats {
		manifest := strings.TrimSpace(history.ManifestName)
		capability := strings.TrimSpace(history.Capability)
		rows = append(rows, fmt.Sprintf(
			"compile-preview signal history: manifest=%s capability=%s successes=%d failures=%d",
			installRequestDisplayValue(manifest, "unknown"),
			installRequestDisplayValue(capability, "unknown"),
			history.Successes,
			history.Failures,
		))
	}
	return rows
}

func compilePreviewReviewRuntimeRows(reviews []ToolCompilePreviewReview) []string {
	return compilePreviewReviewRuntimeRowsWithSelection(reviews, ToolCompilePreviewReviewSelection{}, false)
}

func compilePreviewReviewExecutionRuntimeRows(executions []ToolCompilePreviewReviewExecution) []string {
	rows := []string{fmt.Sprintf("compile-preview review executions: total=%d", len(executions))}
	for _, execution := range executions {
		fields := []string{
			fmt.Sprintf("status=%s", installRequestDisplayValue(execution.Status, "unknown")),
			fmt.Sprintf("mode=%s", installRequestDisplayValue(execution.Mode, "execute")),
			fmt.Sprintf("action=%s", installRequestDisplayValue(string(execution.Action), "unknown")),
			fmt.Sprintf("nodes=%d", execution.ReviewNodes),
			fmt.Sprintf("warnings=%d", execution.ReviewWarnings),
			fmt.Sprintf("events=%d", execution.EventCount),
			fmt.Sprintf("actor=%s", installRequestDisplayValue(execution.Actor, "operator")),
			fmt.Sprintf("executed_at=%s", installRequestDisplayValue(execution.ExecutedAt, "unknown")),
			fmt.Sprintf("run=%s", installRequestDisplayValue(execution.RunID, "unknown")),
			fmt.Sprintf("dispatch=%s", installRequestDisplayValue(execution.DispatchPath, "none")),
			fmt.Sprintf("review=%s", installRequestDisplayValue(execution.ReviewPath, "unknown")),
			fmt.Sprintf("selection=%s", installRequestDisplayValue(execution.SelectionPath, "none")),
			fmt.Sprintf("path=%s", installRequestDisplayValue(execution.Path, "unknown")),
		}
		if reason := strings.TrimSpace(execution.Reason); reason != "" {
			fields = append(fields, "reason="+compactRuntimeRowValue(reason))
			fields = append(fields, "reason_b64="+runtimeRowValueB64(reason))
		}
		if executionError := strings.TrimSpace(execution.Error); executionError != "" {
			fields = append(fields, "error="+compactRuntimeRowValue(executionError))
			fields = append(fields, "error_b64="+runtimeRowValueB64(executionError))
		}
		rows = append(rows, "compile-preview review execution artifact: "+strings.Join(fields, " "))
	}
	return rows
}

func compilePreviewReviewExecutionDispatchRuntimeRows(dispatches []ToolCompilePreviewReviewExecutionDispatch) []string {
	rows := []string{fmt.Sprintf("compile-preview review execution dispatches: total=%d", len(dispatches))}
	for _, dispatch := range dispatches {
		fields := []string{
			fmt.Sprintf("status=%s", installRequestDisplayValue(dispatch.Status, "unknown")),
			fmt.Sprintf("mode=%s", installRequestDisplayValue(dispatch.Mode, "metadata-only")),
			fmt.Sprintf("action=%s", installRequestDisplayValue(string(dispatch.Action), "unknown")),
			fmt.Sprintf("nodes=%d", dispatch.ReviewNodes),
			fmt.Sprintf("warnings=%d", dispatch.ReviewWarnings),
			fmt.Sprintf("actor=%s", installRequestDisplayValue(dispatch.Actor, "operator")),
			fmt.Sprintf("created_at=%s", installRequestDisplayValue(dispatch.CreatedAt, "unknown")),
			fmt.Sprintf("review=%s", installRequestDisplayValue(dispatch.ReviewPath, "unknown")),
			fmt.Sprintf("selection=%s", installRequestDisplayValue(dispatch.SelectionPath, "none")),
			fmt.Sprintf("path=%s", installRequestDisplayValue(dispatch.Path, "unknown")),
		}
		if reason := strings.TrimSpace(dispatch.Reason); reason != "" {
			fields = append(fields, "reason="+compactRuntimeRowValue(reason))
			fields = append(fields, "reason_b64="+runtimeRowValueB64(reason))
		}
		rows = append(rows, "compile-preview review execution dispatch artifact: "+strings.Join(fields, " "))
	}
	return rows
}

func compilePreviewReviewRuntimeRowsWithSelection(reviews []ToolCompilePreviewReview, selection ToolCompilePreviewReviewSelection, selectionFound bool) []string {
	rows := []string{fmt.Sprintf("compile-preview reviews: total=%d", len(reviews))}
	if selectionFound {
		selectedBy := strings.TrimSpace(selection.SelectedBy)
		selectedAt := strings.TrimSpace(selection.SelectedAt)
		clearedBy := strings.TrimSpace(selection.ClearedBy)
		clearedAt := strings.TrimSpace(selection.ClearedAt)
		actorFields := fmt.Sprintf(
			"selected_by=%s selected_by_b64=%s selected_at=%s selected_at_b64=%s",
			installRequestDisplayValue(compactRuntimeRowValue(selectedBy), "unknown"),
			runtimeRowValueB64(selectedBy),
			installRequestDisplayValue(compactRuntimeRowValue(selectedAt), "unknown"),
			runtimeRowValueB64(selectedAt),
		)
		if toolCompilePreviewReviewSelectionIsCleared(selection) {
			actorFields = fmt.Sprintf(
				"cleared_by=%s cleared_by_b64=%s cleared_at=%s cleared_at_b64=%s",
				installRequestDisplayValue(compactRuntimeRowValue(clearedBy), "unknown"),
				runtimeRowValueB64(clearedBy),
				installRequestDisplayValue(compactRuntimeRowValue(clearedAt), "unknown"),
				runtimeRowValueB64(clearedAt),
			)
		}
		reason := strings.TrimSpace(selection.Reason)
		reviewPath := strings.TrimSpace(selection.ReviewPath)
		rows = append(rows, fmt.Sprintf(
			"compile-preview review selection: status=%s mode=%s action=%s superseded=%d %s reason=%s reason_b64=%s review=%s review_b64=%s",
			installRequestDisplayValue(selection.Status, "current"),
			installRequestDisplayValue(selection.Mode, "metadata-only"),
			installRequestDisplayValue(string(selection.Action), "unknown"),
			len(selection.SupersededPaths),
			actorFields,
			installRequestDisplayValue(compactRuntimeRowValue(reason), "none"),
			runtimeRowValueB64(reason),
			installRequestDisplayValue(compactRuntimeRowValue(reviewPath), "unknown"),
			runtimeRowValueB64(reviewPath),
		))
		for _, path := range selection.SupersededPaths {
			if trimmed := strings.TrimSpace(path); trimmed != "" {
				rows = append(rows, "compile-preview review superseded: "+trimmed)
			}
		}
	}
	for _, review := range reviews {
		decision := "unreviewed"
		reviewer := "unknown"
		reviewedAt := "unknown"
		reason := ""
		if review.Annotation != nil {
			decision = installRequestDisplayValue(review.Annotation.Decision, "unreviewed")
			reviewer = strings.TrimSpace(review.Annotation.Reviewer)
			reviewedAt = strings.TrimSpace(review.Annotation.ReviewedAt)
			reason = strings.TrimSpace(review.Annotation.Reason)
		}
		current := "false"
		if selectionFound && toolCompilePreviewReviewSelectionIsCurrent(selection) && strings.TrimSpace(selection.ReviewPath) == strings.TrimSpace(review.Path) {
			current = "true"
		}
		reviewerReadable := installRequestDisplayValue(compactRuntimeRowValue(reviewer), "unknown")
		reviewedAtReadable := installRequestDisplayValue(compactRuntimeRowValue(reviewedAt), "unknown")
		reasonReadable := installRequestDisplayValue(compactRuntimeRowValue(reason), "none")
		pathReadable := installRequestDisplayValue(compactRuntimeRowValue(review.Path), "unknown")
		rows = append(rows, fmt.Sprintf(
			"compile-preview review artifact: status=%s mode=%s action=%s nodes=%d warnings=%d current=%s decision=%s reviewer=%s reviewer_b64=%s reason=%s reason_b64=%s reviewed_at=%s reviewed_at_b64=%s path=%s path_b64=%s",
			installRequestDisplayValue(review.Status, "unknown"),
			installRequestDisplayValue(review.Mode, "metadata-only"),
			installRequestDisplayValue(string(review.Preview.Action), "unknown"),
			len(review.Preview.Nodes),
			len(review.Preview.Warnings),
			current,
			decision,
			reviewerReadable,
			runtimeRowValueB64(reviewer),
			reasonReadable,
			runtimeRowValueB64(reason),
			reviewedAtReadable,
			runtimeRowValueB64(reviewedAt),
			pathReadable,
			runtimeRowValueB64(review.Path),
		))
		for _, warning := range review.Preview.Warnings {
			if trimmed := strings.TrimSpace(warning); trimmed != "" {
				rows = append(rows, "compile-preview review warning: "+trimmed)
			}
		}
		for _, node := range review.Preview.Nodes {
			rows = append(rows, fmt.Sprintf(
				"compile-preview review node: %s manifest=%s tool=%s adapter=%s operation=%s capability=%s target=%s dependency=%s dependency_kind=%s runner=%s parser=%s parser_id=%s score=%s signals=%s command=%s",
				node.ID,
				installRequestDisplayValue(node.ManifestName, "unknown"),
				installRequestDisplayValue(node.ToolID, "pack"),
				installRequestDisplayValue(node.AdapterID, "none"),
				installRequestDisplayValue(node.Operation, "none"),
				installRequestDisplayValue(node.Capability, "unknown"),
				installRequestDisplayValue(node.TargetKind, "unknown"),
				installRequestDisplayValue(node.DependencyName, "none"),
				installRequestDisplayValue(node.DependencyKind, "none"),
				installRequestDisplayValue(node.Runner, "unknown"),
				installRequestDisplayValue(node.Parser, "unknown"),
				installRequestDisplayValue(node.ParserID, "unknown"),
				strconv.FormatFloat(node.SelectionScore, 'g', -1, 64),
				compilePreviewSignalsDisplay(node.SelectionSignals),
				compilePreviewCommandJSON(node.RunCommand),
			))
		}
	}
	return rows
}

func compilePreviewReviewComparisonRuntimeRows(comparison ToolCompilePreviewReviewComparison) []string {
	basePath := strings.TrimSpace(comparison.Base.Path)
	candidatePath := strings.TrimSpace(comparison.Candidate.Path)
	rows := []string{fmt.Sprintf(
		"compile-preview review comparison: mode=%s action=%s node_delta=%d warning_delta=%d added=%d removed=%d changed=%d reordered=%d score_changed=%d signal_changed=%d base=%s base_b64=%s candidate=%s candidate_b64=%s",
		installRequestDisplayValue(comparison.Mode, "metadata-only"),
		installRequestDisplayValue(string(comparison.Candidate.Action), "unknown"),
		comparison.Summary.NodeDelta,
		comparison.Summary.WarningDelta,
		comparison.Summary.AddedNodes,
		comparison.Summary.RemovedNodes,
		comparison.Summary.ChangedNodes,
		comparison.Summary.ReorderedNodes,
		comparison.Summary.ScoreChangedNodes,
		comparison.Summary.SignalChangedNodes,
		installRequestDisplayValue(compactRuntimeRowValue(basePath), "unknown"),
		runtimeRowValueB64(basePath),
		installRequestDisplayValue(compactRuntimeRowValue(candidatePath), "unknown"),
		runtimeRowValueB64(candidatePath),
	)}
	for _, diff := range comparison.NodeDiffs {
		rows = append(rows, fmt.Sprintf(
			"compile-preview review comparison node: change=%s manifest=%s tool=%s adapter=%s operation=%s capability=%s target=%s dependency=%s dependency_kind=%s parser_id=%s base_index=%d candidate_index=%d base_score=%s candidate_score=%s base_signals=%s candidate_signals=%s",
			installRequestDisplayValue(diff.Change, "unknown"),
			installRequestDisplayValue(diff.ManifestName, "unknown"),
			installRequestDisplayValue(diff.ToolID, "pack"),
			installRequestDisplayValue(diff.AdapterID, "none"),
			installRequestDisplayValue(diff.Operation, "none"),
			installRequestDisplayValue(diff.Capability, "unknown"),
			installRequestDisplayValue(diff.TargetKind, "unknown"),
			installRequestDisplayValue(diff.DependencyName, "none"),
			installRequestDisplayValue(diff.DependencyKind, "none"),
			installRequestDisplayValue(diff.ParserID, "unknown"),
			diff.BaseIndex,
			diff.CandidateIndex,
			strconv.FormatFloat(diff.BaseScore, 'g', -1, 64),
			strconv.FormatFloat(diff.CandidateScore, 'g', -1, 64),
			compilePreviewSignalsDisplay(diff.BaseSignals),
			compilePreviewSignalsDisplay(diff.CandidateSignals),
		))
	}
	for _, diff := range comparison.WarningDiffs {
		rows = append(rows, fmt.Sprintf(
			"compile-preview review comparison warning: change=%s warning=%s",
			installRequestDisplayValue(diff.Change, "unknown"),
			installRequestDisplayValue(diff.Warning, "unknown"),
		))
	}
	return rows
}

func compilePreviewRuntimeRows(preview ToolCompilePreview) []string {
	rows := []string{fmt.Sprintf(
		"compile preview: action=%s mode=%s nodes=%d warnings=%d",
		preview.Action,
		installRequestDisplayValue(preview.Mode, "metadata-only"),
		len(preview.Nodes),
		len(preview.Warnings),
	)}
	for _, warning := range preview.Warnings {
		if trimmed := strings.TrimSpace(warning); trimmed != "" {
			rows = append(rows, "compile warning: "+trimmed)
		}
	}
	for _, node := range preview.Nodes {
		rows = append(rows, fmt.Sprintf(
			"compile candidate: %s manifest=%s tool=%s adapter=%s operation=%s capability=%s target=%s dependency=%s dependency_kind=%s runner=%s parser=%s parser_id=%s score=%s signals=%s command=%s",
			node.ID,
			installRequestDisplayValue(node.ManifestName, "unknown"),
			installRequestDisplayValue(node.ToolID, "pack"),
			installRequestDisplayValue(node.AdapterID, "none"),
			installRequestDisplayValue(node.Operation, "none"),
			installRequestDisplayValue(node.Capability, "unknown"),
			installRequestDisplayValue(node.TargetKind, "unknown"),
			installRequestDisplayValue(node.DependencyName, "none"),
			installRequestDisplayValue(node.DependencyKind, "none"),
			installRequestDisplayValue(node.Runner, "unknown"),
			installRequestDisplayValue(node.Parser, "unknown"),
			installRequestDisplayValue(node.ParserID, "unknown"),
			strconv.FormatFloat(node.SelectionScore, 'g', -1, 64),
			compilePreviewSignalsDisplay(node.SelectionSignals),
			compilePreviewCommandJSON(node.RunCommand),
		))
	}
	return rows
}

func compilePreviewSignalsDisplay(signals []string) string {
	filtered := make([]string, 0, len(signals))
	for _, signal := range signals {
		if trimmed := strings.TrimSpace(signal); trimmed != "" {
			filtered = append(filtered, trimmed)
		}
	}
	if len(filtered) == 0 {
		return "-"
	}
	return strings.Join(filtered, ",")
}

func compilePreviewCommandJSON(command []string) string {
	if len(command) == 0 {
		return "[]"
	}
	data, err := json.Marshal(command)
	if err != nil {
		return "[]"
	}
	return string(data)
}

func tuiModelWithRuntimeWorkerOrchestrationRows(root string, model tui.Model) tui.Model {
	orchestrations, err := ListRuntimeWorkerOrchestrations(root)
	if err != nil || len(orchestrations) == 0 {
		return model
	}
	return model.AppendRuntimeRows(tui.RuntimeTabAgentRuntime, runtimeWorkerOrchestrationRows(orchestrations))
}

func runtimeWorkerOrchestrationRows(orchestrations []RuntimeWorkerOrchestration) []string {
	if len(orchestrations) == 0 {
		return nil
	}
	statusCounts := map[string]int{}
	for _, orchestration := range orchestrations {
		statusCounts[installRequestDisplayValue(orchestration.Status, "unknown")]++
	}
	rows := []string{fmt.Sprintf(
		"worker orchestrations: total=%d completed=%d dry-run=%d skipped=%d failed=%d",
		len(orchestrations),
		statusCounts["completed"],
		statusCounts["dry-run"],
		statusCounts["skipped"],
		statusCounts["failed"],
	)}
	for _, orchestration := range orchestrations {
		rows = append(rows, fmt.Sprintf(
			"worker orchestration: status=%s mode=%s transport=%s placement_ready=%d placement_skipped=%d claimed=%d dispatch_ready=%d dispatched=%d failed=%d",
			installRequestDisplayValue(orchestration.Status, "unknown"),
			installRequestDisplayValue(orchestration.Mode, "unknown"),
			installRequestDisplayValue(orchestration.TransportMode, "metadata-only"),
			orchestration.PlacementReady,
			orchestration.PlacementSkipped,
			orchestration.ClaimedCount,
			orchestration.DispatchReady,
			orchestration.DispatchedCount,
			orchestration.DispatchFailed,
		))
		rows = append(rows, fmt.Sprintf(
			"worker orchestration artifact: %s artifact_b64=%s status=%s mode=%s transport=%s placement_ready=%d placement_skipped=%d claimed=%d dispatch_ready=%d dispatch_skipped=%d dispatched=%d failed=%d retry_dispatch_executions_created=%d retry_dispatch_claims_created=%d placement_plan=%s placement_plan_b64=%s placement_dispatch=%s placement_dispatch_b64=%s placement_batch=%s placement_batch_b64=%s lifecycle_maintenance=%s lifecycle_maintenance_b64=%s lifecycle_audit=%s lifecycle_audit_b64=%s",
			orchestration.Path,
			runtimeRowValueB64(orchestration.Path),
			installRequestDisplayValue(orchestration.Status, "unknown"),
			installRequestDisplayValue(orchestration.Mode, "unknown"),
			installRequestDisplayValue(orchestration.TransportMode, "metadata-only"),
			orchestration.PlacementReady,
			orchestration.PlacementSkipped,
			orchestration.ClaimedCount,
			orchestration.DispatchReady,
			orchestration.DispatchSkipped,
			orchestration.DispatchedCount,
			orchestration.DispatchFailed,
			orchestration.RetryDispatchExecutionsCreated,
			orchestration.RetryDispatchClaimsCreated,
			installRequestDisplayValue(orchestration.PlacementPlanPath, "none"),
			runtimeRowValueB64(installRequestDisplayValue(orchestration.PlacementPlanPath, "none")),
			installRequestDisplayValue(orchestration.PlacementDispatchPlanPath, "none"),
			runtimeRowValueB64(installRequestDisplayValue(orchestration.PlacementDispatchPlanPath, "none")),
			installRequestDisplayValue(orchestration.PlacementDispatchBatchPath, "none"),
			runtimeRowValueB64(installRequestDisplayValue(orchestration.PlacementDispatchBatchPath, "none")),
			installRequestDisplayValue(orchestration.LifecycleMaintenancePath, "none"),
			runtimeRowValueB64(installRequestDisplayValue(orchestration.LifecycleMaintenancePath, "none")),
			installRequestDisplayValue(orchestration.LifecycleAuditPath, "none"),
			runtimeRowValueB64(installRequestDisplayValue(orchestration.LifecycleAuditPath, "none")),
		))
		if orchestration.RetryDispatchExecutionsCreated > 0 || orchestration.RetryDispatchClaimsCreated > 0 {
			rows = append(rows, fmt.Sprintf(
				"worker orchestration retry dispatch: executions_created=%d claims_created=%d",
				orchestration.RetryDispatchExecutionsCreated,
				orchestration.RetryDispatchClaimsCreated,
			))
		}
		if orchestration.DispatchLifecycleRetryClaims || orchestration.RetryDispatchClaimsDispatched > 0 || orchestration.RetryDispatchTransportRequestsCreated > 0 {
			rows = append(rows, fmt.Sprintf(
				"worker orchestration retry handoff: dispatch_lifecycle_retry_claims=%t lifecycle_retry_transport=%s claims_dispatched=%d transport_requests=%d",
				orchestration.DispatchLifecycleRetryClaims,
				installRequestDisplayValue(orchestration.LifecycleRetryTransportMode, "metadata-only"),
				orchestration.RetryDispatchClaimsDispatched,
				orchestration.RetryDispatchTransportRequestsCreated,
			))
		}
		for _, path := range orchestration.RetryDispatchExecutionPaths {
			rows = append(rows, "worker orchestration retry dispatch execution: "+filepath.Base(path))
		}
		if strings.TrimSpace(orchestration.Error) != "" {
			rows = append(rows, "worker orchestration error: "+compactRuntimeRowValue(orchestration.Error))
		}
	}
	return rows
}

func tuiModelWithRuntimeWorkerOrchestrationSupervisorRows(root string, model tui.Model) tui.Model {
	supervisors, err := ListRuntimeWorkerOrchestrationSupervisors(root)
	if err != nil || len(supervisors) == 0 {
		return model
	}
	return model.AppendRuntimeRows(tui.RuntimeTabAgentRuntime, runtimeWorkerOrchestrationSupervisorRows(supervisors))
}

func runtimeWorkerOrchestrationSupervisorRows(supervisors []RuntimeWorkerOrchestrationSupervisor) []string {
	if len(supervisors) == 0 {
		return nil
	}
	completed := 0
	dryRun := 0
	failed := 0
	for _, supervisor := range supervisors {
		switch supervisor.Status {
		case "completed":
			completed++
		case "failed":
			failed++
		default:
			if supervisor.Status == "dry-run" {
				dryRun++
			}
		}
	}
	rows := []string{fmt.Sprintf(
		"worker orchestration supervisors: total=%d completed=%d dry-run=%d failed=%d",
		len(supervisors),
		completed,
		dryRun,
		failed,
	)}
	for _, supervisor := range supervisors {
		summary := supervisor.Summary
		orchestrationPath := "none"
		if len(supervisor.OrchestrationPaths) > 0 {
			orchestrationPath = supervisor.OrchestrationPaths[0]
		}
		retryFields := ""
		if summary.RetryDispatchExecutionsCreated > 0 || summary.RetryDispatchClaimsCreated > 0 {
			retryFields = fmt.Sprintf(
				" retry_dispatch_executions_created=%d retry_dispatch_claims_created=%d",
				summary.RetryDispatchExecutionsCreated,
				summary.RetryDispatchClaimsCreated,
			)
		}
		rows = append(rows, fmt.Sprintf(
			"worker orchestration supervisor artifact: %s artifact_b64=%s status=%s mode=%s transport=%s runs=%d max_runs=%d orchestrations_created=%d lifecycle_maintenance=%d lifecycle_audit=%d%s claimed=%d dispatched=%d skipped=%d failed=%d orchestration_path=%s orchestration_path_b64=%s",
			supervisor.Path,
			runtimeRowValueB64(supervisor.Path),
			installRequestDisplayValue(supervisor.Status, "unknown"),
			installRequestDisplayValue(supervisor.Mode, "unknown"),
			installRequestDisplayValue(supervisor.TransportMode, "metadata-only"),
			summary.Runs,
			summary.MaxRuns,
			summary.OrchestrationArtifactsCreated,
			summary.LifecycleMaintenanceArtifactsCreated,
			summary.LifecycleAuditArtifactsCreated,
			retryFields,
			summary.ClaimedCount,
			summary.DispatchedCount,
			summary.SkippedCount,
			summary.FailedCount,
			installRequestDisplayValue(orchestrationPath, "none"),
			runtimeRowValueB64(installRequestDisplayValue(orchestrationPath, "none")),
		))
		rows = append(rows, fmt.Sprintf(
			"worker orchestration supervisor state: %s mode=%s runs=%d max_runs=%d transport=%s",
			installRequestDisplayValue(supervisor.Status, "unknown"),
			installRequestDisplayValue(supervisor.Mode, "unknown"),
			summary.Runs,
			summary.MaxRuns,
			installRequestDisplayValue(supervisor.TransportMode, "metadata-only"),
		))
		rows = append(rows, fmt.Sprintf(
			"worker orchestration supervisor created: orchestrations=%d lifecycle_maintenance=%d lifecycle_audit=%d",
			summary.OrchestrationArtifactsCreated,
			summary.LifecycleMaintenanceArtifactsCreated,
			summary.LifecycleAuditArtifactsCreated,
		))
		if summary.RetryDispatchExecutionsCreated > 0 || summary.RetryDispatchClaimsCreated > 0 {
			rows = append(rows, fmt.Sprintf(
				"worker orchestration supervisor retry dispatch: executions_created=%d claims_created=%d",
				summary.RetryDispatchExecutionsCreated,
				summary.RetryDispatchClaimsCreated,
			))
		}
		if supervisor.DispatchLifecycleRetryClaims || summary.RetryDispatchClaimsDispatched > 0 || summary.RetryDispatchTransportRequestsCreated > 0 {
			rows = append(rows, fmt.Sprintf(
				"worker orchestration supervisor retry handoff: dispatch_lifecycle_retry_claims=%t lifecycle_retry_transport=%s claims_dispatched=%d transport_requests=%d",
				supervisor.DispatchLifecycleRetryClaims,
				installRequestDisplayValue(supervisor.LifecycleRetryTransportMode, "metadata-only"),
				summary.RetryDispatchClaimsDispatched,
				summary.RetryDispatchTransportRequestsCreated,
			))
		}
		for _, path := range supervisor.RetryDispatchExecutionPaths {
			rows = append(rows, "worker orchestration supervisor retry dispatch execution: "+filepath.Base(path))
		}
		rows = append(rows, fmt.Sprintf(
			"worker orchestration supervisor dispatch: claimed=%d dispatched=%d skipped=%d failed=%d",
			summary.ClaimedCount,
			summary.DispatchedCount,
			summary.SkippedCount,
			summary.FailedCount,
		))
		rows = append(rows, "worker orchestration supervisor path: "+filepath.Base(supervisor.Path))
		if strings.TrimSpace(supervisor.Error) != "" {
			rows = append(rows, "worker orchestration supervisor error: "+compactRuntimeRowValue(supervisor.Error))
		}
	}
	return rows
}

func tuiModelWithRuntimeWorkerRows(root string, model tui.Model) tui.Model {
	workers, err := ListRuntimeWorkersWithStaleness(root, time.Now().UTC(), DefaultRuntimeWorkerStaleAfter)
	if err != nil || len(workers) == 0 {
		return model
	}
	return model.AppendRuntimeRows(tui.RuntimeTabAgentRuntime, runtimeWorkerRows(workers))
}

func runtimeWorkerRows(workers []RuntimeWorkerArtifact) []string {
	statusCounts := map[string]int{}
	modeCounts := map[string]int{}
	stale := 0
	for _, worker := range workers {
		if worker.Stale {
			stale++
			modeCounts[installRequestDisplayValue(worker.Mode, "unknown")]++
			continue
		}
		statusCounts[installRequestDisplayValue(worker.Status, "unknown")]++
		modeCounts[installRequestDisplayValue(worker.Mode, "unknown")]++
	}
	rows := []string{fmt.Sprintf(
		"runtime workers: total=%d available=%d offline=%d local=%d remote=%d stale=%d",
		len(workers),
		statusCounts["available"],
		statusCounts["offline"],
		modeCounts["local"],
		modeCounts["remote"],
		stale,
	)}
	if statusCounts["busy"] > 0 || statusCounts["draining"] > 0 {
		rows = append(rows, fmt.Sprintf("runtime workers busy=%d draining=%d", statusCounts["busy"], statusCounts["draining"]))
	}
	for _, worker := range workers {
		status := installRequestDisplayValue(worker.Status, "unknown")
		if worker.Stale {
			status = "stale"
		}
		capabilities := strings.Join(worker.Capabilities, ",")
		if capabilities == "" {
			capabilities = "none"
		}
		row := fmt.Sprintf(
			"runtime worker %s %s %s capabilities=%s",
			installRequestDisplayValue(worker.WorkerID, "unknown"),
			installRequestDisplayValue(worker.Mode, "unknown"),
			status,
			capabilities,
		)
		if strings.TrimSpace(worker.Endpoint) != "" {
			row += " endpoint=" + strings.TrimSpace(worker.Endpoint)
		}
		rows = append(rows, row)
		rows = append(rows, fmt.Sprintf(
			"runtime worker caps: %s %s",
			installRequestDisplayValue(worker.WorkerID, "unknown"),
			capabilities,
		))
		if strings.TrimSpace(worker.Endpoint) != "" {
			rows = append(rows, fmt.Sprintf(
				"runtime worker endpoint: %s %s",
				installRequestDisplayValue(worker.WorkerID, "unknown"),
				strings.TrimSpace(worker.Endpoint),
			))
		}
		if strings.TrimSpace(worker.Path) != "" {
			rows = append(rows, "runtime worker path: "+worker.Path)
		}
		for _, key := range sortedRuntimeWorkerLabelKeys(worker.Labels) {
			rows = append(rows, fmt.Sprintf(
				"runtime worker label: %s %s=%s",
				installRequestDisplayValue(worker.WorkerID, "unknown"),
				key,
				worker.Labels[key],
			))
		}
	}
	return rows
}

func sortedRuntimeWorkerLabelKeys(labels map[string]string) []string {
	keys := make([]string, 0, len(labels))
	for key := range labels {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func tuiModelWithRuntimeWorkerPlacementRows(root string, model tui.Model) tui.Model {
	workers, err := ListRuntimeWorkers(root)
	if err == nil && len(workers) > 0 {
		plan, planErr := PlanRuntimeWorkerPlacements(root, RuntimeWorkerPlacementPlanOptions{Now: time.Now().UTC()})
		if planErr == nil && len(plan.Candidates) > 0 {
			model = model.AppendRuntimeRows(tui.RuntimeTabAgentRuntime, runtimeWorkerPlacementRows(plan))
		}
	}
	plans, err := ListRuntimeWorkerPlacementPlans(root)
	if err != nil || len(plans) == 0 {
		return model
	}
	return model.AppendRuntimeRows(tui.RuntimeTabAgentRuntime, runtimeWorkerPlacementArtifactRows(plans))
}

func runtimeWorkerPlacementRows(plan RuntimeWorkerPlacementPlan) []string {
	rows := []string{fmt.Sprintf("worker placements: ready=%d skipped=%d mode=%s run=%s", plan.ReadyCount, plan.SkippedCount, installRequestDisplayValue(plan.Mode, "metadata-only"), installRequestDisplayValue(plan.RunID, "unknown"))}
	for _, candidate := range plan.Candidates {
		if candidate.Status == "ready" {
			rows = append(rows, fmt.Sprintf(
				"worker placement %s %s: ready mode=%s worker=%s planned=%s execution=%s",
				installRequestDisplayValue(candidate.Tool, "unknown"),
				installRequestDisplayValue(candidate.Capability, "unknown"),
				installRequestDisplayValue(candidate.Mode, "metadata-only"),
				installRequestDisplayValue(candidate.WorkerID, "unknown"),
				installRequestDisplayValue(candidate.PlannedWorkerID, "unknown"),
				installRequestDisplayValue(candidate.ExecutionID, "unknown"),
			))
			rows = append(rows, fmt.Sprintf(
				"placement: %s/%s %s %s %s",
				installRequestDisplayValue(candidate.WorkerID, "unknown"),
				installRequestDisplayValue(candidate.ExecutionID, "unknown"),
				installRequestDisplayValue(candidate.Capability, "unknown"),
				installRequestDisplayValue(candidate.Status, "unknown"),
				installRequestDisplayValue(candidate.Mode, "metadata-only"),
			))
			continue
		}
		rows = append(rows, fmt.Sprintf(
			"worker placement skip %s %s: %s worker=%s planned=%s",
			installRequestDisplayValue(candidate.Tool, "unknown"),
			installRequestDisplayValue(candidate.Capability, "unknown"),
			installRequestDisplayValue(candidate.Reason, "unknown"),
			installRequestDisplayValue(candidate.WorkerID, "none"),
			installRequestDisplayValue(candidate.PlannedWorkerID, "unknown"),
		))
		rows = append(rows, fmt.Sprintf(
			"placement skip: %s %s %s worker=%s",
			installRequestDisplayValue(candidate.Capability, "unknown"),
			installRequestDisplayValue(candidate.Status, "unknown"),
			installRequestDisplayValue(candidate.Reason, "unknown"),
			installRequestDisplayValue(candidate.WorkerID, "none"),
		))
	}
	return rows
}

func runtimeWorkerPlacementArtifactRows(plans []RuntimeWorkerPlacementPlan) []string {
	rows := []string{fmt.Sprintf("worker placement artifacts: total=%d", len(plans))}
	for _, plan := range plans {
		rows = append(rows, fmt.Sprintf(
			"worker placement artifact: %s artifact_b64=%s status=%s mode=%s ready=%d skipped=%d run=%s",
			plan.Path,
			runtimeRowValueB64(plan.Path),
			installRequestDisplayValue(plan.Status, "unknown"),
			installRequestDisplayValue(plan.Mode, "unknown"),
			plan.ReadyCount,
			plan.SkippedCount,
			installRequestDisplayValue(plan.RunID, "unknown"),
		))
	}
	return rows
}

func tuiModelWithRuntimeWorkerPlacementClaimExecutionRows(root string, model tui.Model) tui.Model {
	executions, err := ListRuntimeWorkerPlacementClaimExecutions(root)
	if err != nil || len(executions) == 0 {
		return model
	}
	return model.AppendRuntimeRows(tui.RuntimeTabAgentRuntime, runtimeWorkerPlacementClaimExecutionRows(executions))
}

func runtimeWorkerPlacementClaimExecutionRows(executions []RuntimeWorkerPlacementClaimExecution) []string {
	if len(executions) == 0 {
		return nil
	}
	statusCounts := map[string]int{}
	for _, execution := range executions {
		statusCounts[installRequestDisplayValue(execution.Status, "unknown")]++
	}
	rows := []string{fmt.Sprintf(
		"worker placement claim executions: total=%d claimed=%d skipped=%d failed=%d",
		len(executions),
		statusCounts["claimed"],
		statusCounts["skipped"],
		statusCounts["failed"],
	)}
	for _, execution := range executions {
		rows = append(rows, fmt.Sprintf(
			"worker placement claim execution artifact: %s status=%s mode=%s plan=%s claim=%s worker=%s execution=%s",
			execution.Path,
			installRequestDisplayValue(execution.Status, "unknown"),
			installRequestDisplayValue(execution.Mode, "unknown"),
			installRequestDisplayValue(execution.PlacementPlanPath, "unknown"),
			installRequestDisplayValue(execution.ClaimPath, "none"),
			installRequestDisplayValue(execution.WorkerID, "unknown"),
			installRequestDisplayValue(execution.ExecutionID, "unknown"),
		))
		rows = append(rows, fmt.Sprintf(
			"placement claim: %s/%s %s %s",
			installRequestDisplayValue(execution.WorkerID, "unknown"),
			installRequestDisplayValue(execution.ExecutionID, "unknown"),
			installRequestDisplayValue(execution.Capability, "unknown"),
			installRequestDisplayValue(execution.Status, "unknown"),
		))
		if strings.TrimSpace(execution.SkipReason) != "" {
			rows = append(rows, "worker placement claim skip reason: "+strings.TrimSpace(execution.SkipReason))
		}
		if strings.TrimSpace(execution.Error) != "" {
			rows = append(rows, "worker placement claim error: "+compactRuntimeRowValue(execution.Error))
		}
	}
	return rows
}

func tuiModelWithRuntimeWorkerPlacementDispatchRows(root string, model tui.Model) tui.Model {
	plans, err := ListRuntimeWorkerPlacementDispatchPlans(root)
	if err != nil || len(plans) == 0 {
		return model
	}
	for _, plan := range plans {
		model = model.AppendRuntimeRows(tui.RuntimeTabAgentRuntime, runtimeWorkerPlacementDispatchRows(plan))
	}
	return model
}

func runtimeWorkerPlacementDispatchRows(plan RuntimeWorkerPlacementDispatchPlan) []string {
	rows := []string{fmt.Sprintf(
		"worker placement dispatches: ready=%d skipped=%d mode=%s transport=%s",
		plan.ReadyCount,
		plan.SkippedCount,
		installRequestDisplayValue(plan.Mode, "metadata-only"),
		installRequestDisplayValue(plan.TransportMode, "metadata-only"),
	)}
	if strings.TrimSpace(plan.Path) != "" {
		rows = append(rows, fmt.Sprintf(
			"worker placement dispatch artifact: %s artifact_b64=%s status=%s mode=%s transport=%s ready=%d skipped=%d",
			plan.Path,
			runtimeRowValueB64(plan.Path),
			installRequestDisplayValue(plan.Status, "unknown"),
			installRequestDisplayValue(plan.Mode, "unknown"),
			installRequestDisplayValue(plan.TransportMode, "unknown"),
			plan.ReadyCount,
			plan.SkippedCount,
		))
	}
	for _, candidate := range plan.Candidates {
		rows = append(rows, fmt.Sprintf(
			"worker placement dispatch: status=%s reason=%s claim=%s worker=%s execution=%s transport=%s",
			installRequestDisplayValue(candidate.Status, "unknown"),
			installRequestDisplayValue(candidate.Reason, "none"),
			installRequestDisplayValue(candidate.ClaimPath, "none"),
			installRequestDisplayValue(candidate.WorkerID, "unknown"),
			installRequestDisplayValue(candidate.ExecutionID, "unknown"),
			installRequestDisplayValue(candidate.TransportMode, "metadata-only"),
		))
		rows = append(rows, fmt.Sprintf(
			"placement dispatch: %s/%s %s %s %s",
			installRequestDisplayValue(candidate.WorkerID, "unknown"),
			installRequestDisplayValue(candidate.ExecutionID, "unknown"),
			installRequestDisplayValue(candidate.Capability, "unknown"),
			installRequestDisplayValue(candidate.Status, "unknown"),
			installRequestDisplayValue(candidate.TransportMode, "metadata-only"),
		))
	}
	return rows
}

func tuiModelWithRuntimeWorkerPlacementDispatchExecutionRows(root string, model tui.Model) tui.Model {
	executions, err := ListRuntimeWorkerPlacementDispatchExecutions(root)
	if err != nil || len(executions) == 0 {
		return model
	}
	return model.AppendRuntimeRows(tui.RuntimeTabAgentRuntime, runtimeWorkerPlacementDispatchExecutionRows(executions))
}

func tuiModelWithRuntimeWorkerPlacementDispatchBatchRows(root string, model tui.Model) tui.Model {
	batches, err := ListRuntimeWorkerPlacementDispatchBatches(root)
	if err != nil || len(batches) == 0 {
		return model
	}
	return model.AppendRuntimeRows(tui.RuntimeTabAgentRuntime, runtimeWorkerPlacementDispatchBatchRows(batches))
}

func runtimeWorkerPlacementDispatchBatchRows(batches []RuntimeWorkerPlacementDispatchBatch) []string {
	if len(batches) == 0 {
		return nil
	}
	statusCounts := map[string]int{}
	for _, batch := range batches {
		statusCounts[installRequestDisplayValue(batch.Status, "unknown")]++
	}
	rows := []string{fmt.Sprintf(
		"worker placement dispatch batches: total=%d completed=%d skipped=%d failed=%d",
		len(batches),
		statusCounts["completed"],
		statusCounts["skipped"],
		statusCounts["failed"],
	)}
	for _, batch := range batches {
		rows = append(rows, fmt.Sprintf(
			"worker placement dispatch batch artifact: %s status=%s mode=%s transport=%s plan=%s total=%d dispatched=%d skipped=%d failed=%d",
			batch.Path,
			installRequestDisplayValue(batch.Status, "unknown"),
			installRequestDisplayValue(batch.Mode, "unknown"),
			installRequestDisplayValue(batch.TransportMode, "metadata-only"),
			installRequestDisplayValue(batch.PlacementDispatchPlanPath, "unknown"),
			batch.TotalCount,
			batch.DispatchedCount,
			batch.SkippedCount,
			batch.FailedCount,
		))
		rows = append(rows, fmt.Sprintf(
			"placement dispatch batch: %s %s total=%d dispatched=%d skipped=%d failed=%d",
			installRequestDisplayValue(batch.Status, "unknown"),
			installRequestDisplayValue(batch.TransportMode, "metadata-only"),
			batch.TotalCount,
			batch.DispatchedCount,
			batch.SkippedCount,
			batch.FailedCount,
		))
		if strings.TrimSpace(batch.Error) != "" {
			rows = append(rows, "worker placement dispatch batch error: "+strings.TrimSpace(batch.Error))
		}
	}
	return rows
}

func runtimeWorkerPlacementDispatchExecutionRows(executions []RuntimeWorkerPlacementDispatchExecution) []string {
	if len(executions) == 0 {
		return nil
	}
	statusCounts := map[string]int{}
	for _, execution := range executions {
		statusCounts[installRequestDisplayValue(execution.Status, "unknown")]++
	}
	rows := []string{fmt.Sprintf(
		"worker placement dispatch executions: total=%d dispatched=%d skipped=%d failed=%d",
		len(executions),
		statusCounts["dispatched"],
		statusCounts["skipped"],
		statusCounts["failed"],
	)}
	for _, execution := range executions {
		rows = append(rows, fmt.Sprintf(
			"worker placement dispatch execution artifact: %s status=%s mode=%s transport=%s plan=%s claim=%s queue_dispatch=%s worker=%s execution=%s",
			execution.Path,
			installRequestDisplayValue(execution.Status, "unknown"),
			installRequestDisplayValue(execution.Mode, "unknown"),
			installRequestDisplayValue(execution.TransportMode, "metadata-only"),
			installRequestDisplayValue(execution.PlacementDispatchPlanPath, "unknown"),
			installRequestDisplayValue(execution.ClaimPath, "unknown"),
			installRequestDisplayValue(execution.QueueDispatchExecutionPath, "none"),
			installRequestDisplayValue(execution.WorkerID, "unknown"),
			installRequestDisplayValue(execution.ExecutionID, "unknown"),
		))
		rows = append(rows, fmt.Sprintf(
			"placement dispatch execution: %s/%s %s %s %s",
			installRequestDisplayValue(execution.WorkerID, "unknown"),
			installRequestDisplayValue(execution.ExecutionID, "unknown"),
			installRequestDisplayValue(execution.Capability, "unknown"),
			installRequestDisplayValue(execution.Status, "unknown"),
			installRequestDisplayValue(execution.TransportMode, "metadata-only"),
		))
		if strings.TrimSpace(execution.SkipReason) != "" {
			rows = append(rows, "worker placement dispatch skip reason: "+strings.TrimSpace(execution.SkipReason))
		}
		if strings.TrimSpace(execution.TransportRequestPath) != "" {
			rows = append(rows, "worker placement dispatch transport request: "+strings.TrimSpace(execution.TransportRequestPath))
		}
		if strings.TrimSpace(execution.Error) != "" {
			rows = append(rows, "worker placement dispatch error: "+compactRuntimeRowValue(execution.Error))
		}
	}
	return rows
}

func tuiModelWithRuntimeWorkerQueueClaimRows(root string, model tui.Model) tui.Model {
	claims, err := ListRuntimeWorkerQueueClaims(root)
	if err != nil || len(claims) == 0 {
		return model
	}
	return model.AppendRuntimeRows(tui.RuntimeTabAgentRuntime, runtimeWorkerQueueClaimRows(claims))
}

func runtimeWorkerQueueClaimRows(claims []RuntimeWorkerQueueClaim) []string {
	statusCounts := map[string]int{}
	for _, claim := range claims {
		statusCounts[installRequestDisplayValue(claim.Status, "unknown")]++
	}
	rows := []string{fmt.Sprintf(
		"worker queue claims: total=%d claimed=%d canceled=%d expired=%d dispatched=%d completed=%d failed=%d",
		len(claims),
		statusCounts["claimed"],
		statusCounts["canceled"],
		statusCounts["expired"],
		statusCounts["dispatched"],
		statusCounts["completed"],
		statusCounts["failed"],
	)}
	for _, claim := range claims {
		rows = append(rows, fmt.Sprintf(
			"worker queue claim %s %s %s run=%s execution=%s capability=%s mode=%s",
			installRequestDisplayValue(claim.WorkerID, "unknown"),
			installRequestDisplayValue(claim.WorkerMode, "unknown"),
			installRequestDisplayValue(claim.Status, "unknown"),
			installRequestDisplayValue(claim.RunID, "unknown"),
			installRequestDisplayValue(claim.ExecutionID, "unknown"),
			installRequestDisplayValue(claim.Capability, "unknown"),
			installRequestDisplayValue(claim.Mode, "unknown"),
		))
		rows = append(rows, fmt.Sprintf(
			"queue claim: %s/%s %s %s %s",
			installRequestDisplayValue(claim.WorkerID, "unknown"),
			installRequestDisplayValue(claim.ExecutionID, "unknown"),
			installRequestDisplayValue(claim.Capability, "unknown"),
			installRequestDisplayValue(claim.Status, "unknown"),
			installRequestDisplayValue(claim.Mode, "unknown"),
		))
		if strings.TrimSpace(claim.ExpiresAt) != "" {
			rows = append(rows, "worker queue claim expires: "+strings.TrimSpace(claim.ExpiresAt))
		}
		if strings.TrimSpace(claim.RenewedAt) != "" {
			rows = append(rows, fmt.Sprintf(
				"worker queue claim renewed: at=%s by=%s",
				strings.TrimSpace(claim.RenewedAt),
				installRequestDisplayValue(claim.RenewedBy, "unknown"),
			))
			rows = append(rows, fmt.Sprintf(
				"worker queue claim renewed reason=%s",
				compactRuntimeRowValue(claim.RenewReason),
			))
			rows = append(rows, fmt.Sprintf(
				"worker queue claim renewed expires=%s lease_seconds=%d",
				strings.TrimSpace(claim.ExpiresAt),
				claim.LeaseSeconds,
			))
		}
		if strings.TrimSpace(claim.CanceledAt) != "" {
			rows = append(rows, fmt.Sprintf(
				"worker queue claim canceled: at=%s by=%s reason=%s",
				strings.TrimSpace(claim.CanceledAt),
				installRequestDisplayValue(claim.CanceledBy, "unknown"),
				compactRuntimeRowValue(claim.CancelReason),
			))
		}
		if strings.TrimSpace(claim.ExpiredAt) != "" {
			rows = append(rows, fmt.Sprintf(
				"worker queue claim expired: at=%s by=%s reason=%s",
				strings.TrimSpace(claim.ExpiredAt),
				installRequestDisplayValue(claim.ExpiredBy, "unknown"),
				compactRuntimeRowValue(claim.ExpireReason),
			))
		}
		if !claim.ToolPlan.IsZero() {
			rows = append(rows, fmt.Sprintf(
				"worker queue claim tool plan: action=%s runner=%s parser=%s command=%s",
				installRequestDisplayValue(claim.ToolPlan.Action, "unknown"),
				installRequestDisplayValue(claim.ToolPlan.Runner, "unknown"),
				installRequestDisplayValue(claim.ToolPlan.Parser, "unknown"),
				compactRuntimeRowValue(strings.Join(claim.ToolPlan.RunCommand, " ")),
			))
		}
		if strings.TrimSpace(claim.Path) != "" {
			rows = append(rows, fmt.Sprintf(
				"worker queue claim artifact: %s artifact_b64=%s status=%s mode=%s worker=%s worker_b64=%s run=%s run_b64=%s execution=%s execution_b64=%s",
				claim.Path,
				runtimeRowValueB64(claim.Path),
				installRequestDisplayValue(claim.Status, "unknown"),
				installRequestDisplayValue(claim.Mode, "unknown"),
				installRequestDisplayValue(claim.WorkerID, "unknown"),
				runtimeRowValueB64(claim.WorkerID),
				installRequestDisplayValue(claim.RunID, "unknown"),
				runtimeRowValueB64(claim.RunID),
				installRequestDisplayValue(claim.ExecutionID, "unknown"),
				runtimeRowValueB64(claim.ExecutionID),
			))
			rows = append(rows, "worker queue claim path: "+claim.Path)
		}
	}
	return rows
}

func tuiModelWithRuntimeWorkerQueueDispatchExecutionRows(root string, model tui.Model) tui.Model {
	executions, err := ListRuntimeWorkerQueueDispatchExecutions(root)
	if err != nil || len(executions) == 0 {
		return model
	}
	return model.AppendRuntimeRows(tui.RuntimeTabAgentRuntime, runtimeWorkerQueueDispatchExecutionRows(executions))
}

func runtimeWorkerQueueDispatchExecutionRows(executions []RuntimeWorkerQueueDispatchExecution) []string {
	statusCounts := map[string]int{}
	for _, execution := range executions {
		statusCounts[installRequestDisplayValue(execution.Status, "unknown")]++
	}
	rows := []string{fmt.Sprintf(
		"worker queue dispatch executions: total=%d dispatched=%d skipped=%d",
		len(executions),
		statusCounts["dispatched"],
		statusCounts["skipped"],
	)}
	if statusCounts["failed"] > 0 {
		rows = append(rows, fmt.Sprintf("worker queue dispatch executions failed=%d", statusCounts["failed"]))
	}
	for _, execution := range executions {
		rows = append(rows, fmt.Sprintf(
			"worker queue dispatch execution artifact: %s artifact_b64=%s status=%s mode=%s transport=%s claim=%s claim_b64=%s worker=%s worker_b64=%s execution=%s execution_b64=%s",
			compactRuntimeRowValue(execution.Path),
			runtimeRowValueB64(execution.Path),
			installRequestDisplayValue(execution.Status, "unknown"),
			installRequestDisplayValue(execution.Mode, "unknown"),
			installRequestDisplayValue(execution.TransportMode, "unknown"),
			installRequestDisplayValue(compactRuntimeRowValue(execution.ClaimPath), "unknown"),
			runtimeRowValueB64(execution.ClaimPath),
			installRequestDisplayValue(compactRuntimeRowValue(execution.WorkerID), "unknown"),
			runtimeRowValueB64(execution.WorkerID),
			installRequestDisplayValue(compactRuntimeRowValue(execution.ExecutionID), "unknown"),
			runtimeRowValueB64(execution.ExecutionID),
		))
		rows = append(rows, fmt.Sprintf(
			"queue dispatch: %s/%s %s %s %s",
			installRequestDisplayValue(execution.WorkerID, "unknown"),
			installRequestDisplayValue(execution.ExecutionID, "unknown"),
			installRequestDisplayValue(execution.Capability, "unknown"),
			installRequestDisplayValue(execution.Status, "unknown"),
			installRequestDisplayValue(execution.TransportMode, "unknown"),
		))
		if strings.TrimSpace(execution.SkipReason) != "" {
			rows = append(rows, "worker queue dispatch skip reason: "+strings.TrimSpace(execution.SkipReason))
		}
		if strings.TrimSpace(execution.TransportRequestPath) != "" {
			rows = append(rows, "worker queue dispatch transport request: "+strings.TrimSpace(execution.TransportRequestPath))
		}
		if strings.TrimSpace(execution.TransportEndpoint) != "" {
			rows = append(rows, "worker queue dispatch http endpoint: "+strings.TrimSpace(execution.TransportEndpoint))
		}
		if execution.TransportHTTPStatus != 0 || strings.TrimSpace(execution.TransportResponseBody) != "" {
			rows = append(rows, fmt.Sprintf(
				"worker queue dispatch http response: status=%d body=%s",
				execution.TransportHTTPStatus,
				compactRuntimeRowValue(execution.TransportResponseBody),
			))
		}
		if strings.TrimSpace(execution.ResultCallbackEndpoint) != "" {
			rows = append(rows, "worker queue dispatch result callback: "+strings.TrimSpace(execution.ResultCallbackEndpoint))
		}
		if !execution.ToolPlan.IsZero() {
			rows = append(rows, fmt.Sprintf(
				"worker queue dispatch tool plan: action=%s runner=%s parser=%s command=%s",
				installRequestDisplayValue(execution.ToolPlan.Action, "unknown"),
				installRequestDisplayValue(execution.ToolPlan.Runner, "unknown"),
				installRequestDisplayValue(execution.ToolPlan.Parser, "unknown"),
				compactRuntimeRowValue(strings.Join(execution.ToolPlan.RunCommand, " ")),
			))
		}
		if strings.TrimSpace(execution.Error) != "" {
			rows = append(rows, "worker queue dispatch error: "+compactRuntimeRowValue(execution.Error))
		}
	}
	return rows
}

func tuiModelWithRuntimeWorkerTransportDispatchRequestRows(root string, model tui.Model) tui.Model {
	requests, err := ListRuntimeWorkerTransportDispatchRequests(root, "")
	if err != nil || len(requests) == 0 {
		return model
	}
	return model.AppendRuntimeRows(tui.RuntimeTabAgentRuntime, runtimeWorkerTransportDispatchRequestRows(requests))
}

func runtimeWorkerTransportDispatchRequestRows(requests []RuntimeWorkerTransportDispatchRequest) []string {
	statusCounts := map[string]int{}
	for _, request := range requests {
		statusCounts[installRequestDisplayValue(request.Status, "unknown")]++
	}
	rows := []string{fmt.Sprintf(
		"worker transport requests: total=%d pending=%d ack=%d completed=%d failed=%d",
		len(requests),
		statusCounts["pending"],
		statusCounts["acknowledged"],
		statusCounts["completed"],
		statusCounts["failed"],
	)}
	for _, request := range requests {
		rows = append(rows, fmt.Sprintf(
			"worker transport request: %s request_b64=%s status=%s transport=%s dispatch=%s dispatch_b64=%s claim=%s claim_b64=%s worker=%s worker_b64=%s execution=%s execution_b64=%s",
			compactRuntimeRowValue(request.Path),
			runtimeRowValueB64(request.Path),
			installRequestDisplayValue(request.Status, "unknown"),
			installRequestDisplayValue(request.TransportMode, "unknown"),
			installRequestDisplayValue(compactRuntimeRowValue(request.DispatchExecutionPath), "unknown"),
			runtimeRowValueB64(request.DispatchExecutionPath),
			installRequestDisplayValue(compactRuntimeRowValue(request.ClaimPath), "unknown"),
			runtimeRowValueB64(request.ClaimPath),
			installRequestDisplayValue(compactRuntimeRowValue(request.WorkerID), "unknown"),
			runtimeRowValueB64(request.WorkerID),
			installRequestDisplayValue(compactRuntimeRowValue(request.ExecutionID), "unknown"),
			runtimeRowValueB64(request.ExecutionID),
		))
		rows = append(rows, fmt.Sprintf(
			"transport request: %s/%s %s %s",
			installRequestDisplayValue(request.WorkerID, "unknown"),
			installRequestDisplayValue(request.ExecutionID, "unknown"),
			installRequestDisplayValue(request.Status, "unknown"),
			installRequestDisplayValue(request.TransportMode, "unknown"),
		))
		if strings.TrimSpace(request.AckedAt) != "" || strings.TrimSpace(request.AckedBy) != "" {
			rows = append(rows, fmt.Sprintf(
				"worker transport request ack: worker=%s at=%s",
				installRequestDisplayValue(request.AckedBy, "unknown"),
				installRequestDisplayValue(request.AckedAt, "unknown"),
			))
		}
		if strings.TrimSpace(request.ResultPath) != "" {
			rows = append(rows, "worker transport request result: "+strings.TrimSpace(request.ResultPath))
		}
		if !request.ToolPlan.IsZero() {
			rows = append(rows, fmt.Sprintf(
				"worker transport request tool plan: action=%s runner=%s parser=%s command=%s",
				installRequestDisplayValue(request.ToolPlan.Action, "unknown"),
				installRequestDisplayValue(request.ToolPlan.Runner, "unknown"),
				installRequestDisplayValue(request.ToolPlan.Parser, "unknown"),
				compactRuntimeRowValue(strings.Join(request.ToolPlan.RunCommand, " ")),
			))
		}
	}
	return rows
}

func tuiModelWithRuntimeWorkerTransportDaemonRunRows(root string, model tui.Model) tui.Model {
	workers, err := ListRuntimeWorkers(root)
	if err != nil || len(workers) == 0 {
		return model
	}
	var runs []RuntimeWorkerTransportRunDaemonResult
	for _, worker := range workers {
		workerRuns, err := ListRuntimeWorkerTransportDaemonRuns(root, worker.WorkerID)
		if err != nil {
			continue
		}
		runs = append(runs, workerRuns...)
	}
	if len(runs) == 0 {
		return model
	}
	return model.AppendRuntimeRows(tui.RuntimeTabAgentRuntime, runtimeWorkerTransportDaemonRunRows(runs))
}

func runtimeWorkerTransportDaemonRunRows(runs []RuntimeWorkerTransportRunDaemonResult) []string {
	statusCounts := map[string]int{}
	for _, run := range runs {
		statusCounts[installRequestDisplayValue(run.Status, "unknown")]++
	}
	rows := []string{fmt.Sprintf(
		"worker transport daemons: total=%d completed=%d failed=%d idle=%d",
		len(runs),
		statusCounts["completed"],
		statusCounts["failed"],
		statusCounts["idle"],
	)}
	for _, run := range runs {
		rows = append(rows, fmt.Sprintf(
			"worker transport daemon artifact: %s status=%s worker=%s source=%s",
			run.Path,
			installRequestDisplayValue(run.Status, "unknown"),
			installRequestDisplayValue(run.WorkerID, "unknown"),
			installRequestDisplayValue(run.CommandSource, "unknown"),
		))
		rows = append(rows, fmt.Sprintf(
			"transport daemon: %s %s processed=%d completed=%d failed=%d",
			installRequestDisplayValue(run.WorkerID, "unknown"),
			installRequestDisplayValue(run.Status, "unknown"),
			run.Processed,
			run.Completed,
			run.Failed,
		))
		if strings.TrimSpace(run.Error) != "" {
			rows = append(rows, "worker transport daemon error: "+compactRuntimeRowValue(run.Error))
		}
		if strings.TrimSpace(run.ControlAction) != "" {
			rows = append(rows, fmt.Sprintf(
				"worker transport daemon control: %s action=%s request=%s",
				installRequestDisplayValue(run.WorkerID, "unknown"),
				installRequestDisplayValue(run.ControlAction, "unknown"),
				compactRuntimeRowValue(run.ControlRequestPath),
			))
		}
		if strings.TrimSpace(run.StopReason) != "" {
			rows = append(rows, "worker transport daemon stop reason: "+compactRuntimeRowValue(run.StopReason))
		}
	}
	return rows
}

func tuiModelWithRuntimeWorkerTransportDaemonSupervisorRows(root string, model tui.Model) tui.Model {
	runs, err := listRuntimeWorkerTransportDaemonSupervisorRunsForWorkspace(root)
	if err != nil || len(runs) == 0 {
		return model
	}
	return model.AppendRuntimeRows(tui.RuntimeTabAgentRuntime, runtimeWorkerTransportDaemonSupervisorRows(runs))
}

func listRuntimeWorkerTransportDaemonSupervisorRunsForWorkspace(root string) ([]RuntimeWorkerTransportDaemonSupervisorRun, error) {
	paths, err := filepath.Glob(filepath.Join(runtimeWorkerTransportDaemonSupervisorRunDir(root), "*", "*.json"))
	if err != nil {
		return nil, fmt.Errorf("glob worker transport daemon supervisor runs: %w", err)
	}
	sort.Strings(paths)
	runs := make([]RuntimeWorkerTransportDaemonSupervisorRun, 0, len(paths))
	for _, path := range paths {
		run, err := readRuntimeWorkerTransportDaemonSupervisorRunFile(path)
		if err != nil {
			return nil, err
		}
		runs = append(runs, run)
	}
	sort.SliceStable(runs, func(i, j int) bool {
		if runs[i].StartedAt != runs[j].StartedAt {
			return runs[i].StartedAt < runs[j].StartedAt
		}
		return runs[i].Path < runs[j].Path
	})
	return runs, nil
}

func runtimeWorkerTransportDaemonSupervisorRows(runs []RuntimeWorkerTransportDaemonSupervisorRun) []string {
	statusCounts := map[string]int{}
	for _, run := range runs {
		statusCounts[installRequestDisplayValue(run.Status, "unknown")]++
	}
	rows := []string{fmt.Sprintf(
		"worker transport daemon supervisors: total=%d completed=%d stopped=%d restart-limit=%d failed=%d",
		len(runs),
		statusCounts["completed"],
		statusCounts["stopped"],
		statusCounts["restart-limit"],
		statusCounts["failed"],
	)}
	for _, run := range runs {
		rows = append(rows, fmt.Sprintf(
			"transport daemon supervisor: %s %s runs=%d restarts=%d processed=%d completed=%d failed=%d",
			installRequestDisplayValue(run.WorkerID, "unknown"),
			installRequestDisplayValue(run.Status, "unknown"),
			run.Runs,
			run.RestartCount,
			run.Processed,
			run.Completed,
			run.Failed,
		))
		rows = append(rows, fmt.Sprintf(
			"worker transport daemon supervisor artifact: %s status=%s worker=%s source=%s mode=%s",
			run.Path,
			installRequestDisplayValue(run.Status, "unknown"),
			installRequestDisplayValue(run.WorkerID, "unknown"),
			installRequestDisplayValue(run.CommandSource, "unknown"),
			installRequestDisplayValue(run.Mode, "unknown"),
		))
		if strings.TrimSpace(run.Error) != "" {
			rows = append(rows, "worker transport daemon supervisor error: "+compactRuntimeRowValue(run.Error))
		}
	}
	return rows
}

func tuiModelWithRuntimeWorkerTransportDaemonControlRows(root string, model tui.Model) tui.Model {
	controls, err := listRuntimeWorkerTransportDaemonControlRequestsForWorkspace(root)
	if err != nil || len(controls) == 0 {
		return model
	}
	return model.AppendRuntimeRows(tui.RuntimeTabAgentRuntime, runtimeWorkerTransportDaemonControlRows(controls))
}

func listRuntimeWorkerTransportDaemonControlRequestsForWorkspace(root string) ([]RuntimeWorkerTransportDaemonControlRequest, error) {
	paths, err := filepath.Glob(filepath.Join(runtimeWorkerTransportDaemonControlDir(root), "*", "*.json"))
	if err != nil {
		return nil, fmt.Errorf("glob worker transport daemon controls: %w", err)
	}
	return readRuntimeWorkerTransportDaemonControlRequestFiles(paths)
}

func runtimeWorkerTransportDaemonControlRows(controls []RuntimeWorkerTransportDaemonControlRequest) []string {
	statusCounts := map[string]int{}
	actionCounts := map[string]int{}
	for _, control := range controls {
		statusCounts[installRequestDisplayValue(control.Status, "unknown")]++
		actionCounts[installRequestDisplayValue(control.Action, "unknown")]++
	}
	rows := []string{fmt.Sprintf(
		"worker transport daemon controls: total=%d pending=%d stop=%d restart=%d applied=%d",
		len(controls),
		statusCounts["pending"],
		actionCounts["stop"],
		actionCounts["restart"],
		statusCounts["applied"],
	)}
	for _, control := range controls {
		rows = append(rows, fmt.Sprintf(
			"worker transport daemon control artifact: %s status=%s worker=%s action=%s mode=%s",
			control.Path,
			installRequestDisplayValue(control.Status, "unknown"),
			installRequestDisplayValue(control.WorkerID, "unknown"),
			installRequestDisplayValue(control.Action, "unknown"),
			installRequestDisplayValue(control.Mode, "unknown"),
		))
		rows = append(rows, fmt.Sprintf(
			"transport daemon control: %s %s %s",
			installRequestDisplayValue(control.WorkerID, "unknown"),
			installRequestDisplayValue(control.Action, "unknown"),
			installRequestDisplayValue(control.Status, "unknown"),
		))
		if strings.TrimSpace(control.Reason) != "" {
			rows = append(rows, "daemon control reason: "+compactRuntimeRowValue(control.Reason))
		}
		if strings.TrimSpace(control.RequestedBy) != "" {
			rows = append(rows, "daemon control requested_by: "+compactRuntimeRowValue(control.RequestedBy))
		}
	}
	return rows
}

func tuiModelWithRuntimeWorkerQueueDispatchResultRows(root string, model tui.Model) tui.Model {
	results, err := ListRuntimeWorkerQueueDispatchResults(root)
	if err != nil || len(results) == 0 {
		return model
	}
	return model.AppendRuntimeRows(tui.RuntimeTabAgentRuntime, runtimeWorkerQueueDispatchResultRows(results))
}

func runtimeWorkerQueueDispatchResultRows(results []RuntimeWorkerQueueDispatchResult) []string {
	statusCounts := map[string]int{}
	for _, result := range results {
		statusCounts[installRequestDisplayValue(result.Status, "unknown")]++
	}
	rows := []string{fmt.Sprintf(
		"worker queue dispatch results: total=%d completed=%d failed=%d",
		len(results),
		statusCounts["completed"],
		statusCounts["failed"],
	)}
	for _, result := range results {
		rows = append(rows, fmt.Sprintf(
			"worker queue dispatch result artifact: %s artifact_b64=%s status=%s mode=%s dispatch=%s dispatch_b64=%s worker=%s worker_b64=%s execution=%s execution_b64=%s",
			compactRuntimeRowValue(result.Path),
			runtimeRowValueB64(result.Path),
			installRequestDisplayValue(result.Status, "unknown"),
			installRequestDisplayValue(result.Mode, "unknown"),
			installRequestDisplayValue(compactRuntimeRowValue(result.DispatchExecutionPath), "unknown"),
			runtimeRowValueB64(result.DispatchExecutionPath),
			installRequestDisplayValue(compactRuntimeRowValue(result.WorkerID), "unknown"),
			runtimeRowValueB64(result.WorkerID),
			installRequestDisplayValue(compactRuntimeRowValue(result.ExecutionID), "unknown"),
			runtimeRowValueB64(result.ExecutionID),
		))
		rows = append(rows, fmt.Sprintf(
			"queue result: %s/%s %s",
			installRequestDisplayValue(result.WorkerID, "unknown"),
			installRequestDisplayValue(result.ExecutionID, "unknown"),
			installRequestDisplayValue(result.Status, "unknown"),
		))
		if strings.TrimSpace(result.Output) != "" {
			rows = append(rows, "worker queue dispatch result output: "+compactRuntimeRowValue(result.Output))
		}
		if strings.TrimSpace(result.Error) != "" {
			rows = append(rows, "worker queue dispatch result error: "+compactRuntimeRowValue(result.Error))
		}
	}
	return rows
}

func tuiModelWithRuntimeWorkerDispatchRetryPlanRows(root string, model tui.Model) tui.Model {
	plans, err := ListRuntimeWorkerDispatchRetryPlans(root)
	if err != nil || len(plans) == 0 {
		return model
	}
	return model.AppendRuntimeRows(tui.RuntimeTabAgentRuntime, runtimeWorkerDispatchRetryPlanRows(plans))
}

func runtimeWorkerDispatchRetryPlanRows(plans []RuntimeWorkerDispatchRetryPlan) []string {
	if len(plans) == 0 {
		return nil
	}
	ready := 0
	skipped := 0
	for _, plan := range plans {
		ready += plan.Summary.Ready
		skipped += plan.Summary.Skipped
	}
	rows := []string{fmt.Sprintf(
		"worker dispatch retry plans: total=%d ready=%d skipped=%d",
		len(plans),
		ready,
		skipped,
	)}
	for _, plan := range plans {
		rows = append(rows, fmt.Sprintf(
			"worker dispatch retry plan artifact: %s status=%s mode=%s ready=%d skipped=%d",
			plan.Path,
			installRequestDisplayValue(plan.Status, "unknown"),
			installRequestDisplayValue(plan.Mode, "metadata-only"),
			plan.Summary.Ready,
			plan.Summary.Skipped,
		))
		rows = append(rows, fmt.Sprintf(
			"dispatch retry plan: %s %s %s ready=%d skipped=%d",
			installRequestDisplayValue(filepath.Base(plan.Path), "unknown"),
			installRequestDisplayValue(plan.Status, "unknown"),
			installRequestDisplayValue(plan.Mode, "metadata-only"),
			plan.Summary.Ready,
			plan.Summary.Skipped,
		))
		for _, candidate := range plan.Ready {
			rows = append(rows, fmt.Sprintf(
				"dispatch retry candidate: %s/%s %s %s error=%s gate=%s result=%s dispatch=%s",
				installRequestDisplayValue(candidate.WorkerID, "unknown"),
				installRequestDisplayValue(candidate.ExecutionID, "unknown"),
				installRequestDisplayValue(candidate.Status, "unknown"),
				installRequestDisplayValue(candidate.Mode, "metadata-only"),
				compactRuntimeRowValue(candidate.LastError),
				installRequestDisplayValue(candidate.Gate, "worker-dispatch-retry-review"),
				installRequestDisplayValue(candidate.ResultPath, "unknown"),
				installRequestDisplayValue(candidate.DispatchPath, "unknown"),
			))
		}
		for _, skippedCandidate := range plan.Skipped {
			rows = append(rows, fmt.Sprintf(
				"dispatch retry skip: %s/%s status=%s reason=%s result=%s",
				installRequestDisplayValue(skippedCandidate.WorkerID, "unknown"),
				installRequestDisplayValue(skippedCandidate.ExecutionID, "unknown"),
				installRequestDisplayValue(skippedCandidate.Status, "unknown"),
				installRequestDisplayValue(skippedCandidate.Reason, "unknown"),
				installRequestDisplayValue(skippedCandidate.ResultPath, "unknown"),
			))
		}
	}
	return rows
}

func tuiModelWithRuntimeWorkerDispatchRetryDispatchRows(root string, model tui.Model) tui.Model {
	dispatches, err := ListRuntimeWorkerDispatchRetryDispatches(root)
	if err != nil || len(dispatches) == 0 {
		return model
	}
	return model.AppendRuntimeRows(tui.RuntimeTabAgentRuntime, runtimeWorkerDispatchRetryDispatchRows(dispatches))
}

func runtimeWorkerDispatchRetryDispatchRows(dispatches []RuntimeWorkerDispatchRetryDispatch) []string {
	if len(dispatches) == 0 {
		return nil
	}
	ready := 0
	skipped := 0
	for _, dispatch := range dispatches {
		ready += dispatch.Summary.Ready
		skipped += dispatch.Summary.Skipped
	}
	rows := []string{fmt.Sprintf(
		"worker dispatch retry dispatches: total=%d ready=%d skipped=%d",
		len(dispatches),
		ready,
		skipped,
	)}
	for _, dispatch := range dispatches {
		rows = append(rows, fmt.Sprintf(
			"worker dispatch retry dispatch artifact: %s status=%s mode=%s retry_plan=%s ready=%d skipped=%d",
			dispatch.Path,
			installRequestDisplayValue(dispatch.Status, "unknown"),
			installRequestDisplayValue(dispatch.Mode, "metadata-only"),
			installRequestDisplayValue(dispatch.RetryPlanPath, "none"),
			dispatch.Summary.Ready,
			dispatch.Summary.Skipped,
		))
		rows = append(rows, fmt.Sprintf(
			"dispatch retry dispatch: %s %s %s retry_plan=%s ready=%d skipped=%d",
			installRequestDisplayValue(filepath.Base(dispatch.Path), "unknown"),
			installRequestDisplayValue(dispatch.Status, "unknown"),
			installRequestDisplayValue(dispatch.Mode, "metadata-only"),
			installRequestDisplayValue(filepath.Base(dispatch.RetryPlanPath), "none"),
			dispatch.Summary.Ready,
			dispatch.Summary.Skipped,
		))
		rows = append(rows, fmt.Sprintf(
			"dispatch retry dispatch retry_plan=%s artifact=%s",
			installRequestDisplayValue(filepath.Base(dispatch.RetryPlanPath), "none"),
			installRequestDisplayValue(filepath.Base(dispatch.Path), "unknown"),
		))
	}
	return rows
}

func tuiModelWithRuntimeWorkerDispatchRetryDispatchExecutionRows(root string, model tui.Model) tui.Model {
	executions, err := ListRuntimeWorkerDispatchRetryDispatchExecutions(root)
	if err != nil || len(executions) == 0 {
		return model
	}
	return model.AppendRuntimeRows(tui.RuntimeTabAgentRuntime, runtimeWorkerDispatchRetryDispatchExecutionRows(executions))
}

func runtimeWorkerDispatchRetryDispatchExecutionRows(executions []RuntimeWorkerDispatchRetryDispatchExecution) []string {
	if len(executions) == 0 {
		return nil
	}
	claimed := 0
	dispatched := 0
	skipped := 0
	failed := 0
	for _, execution := range executions {
		claimed += execution.Summary.Claimed
		dispatched += execution.Summary.Dispatched
		skipped += execution.Summary.Skipped
		failed += execution.Summary.Failed
	}
	rows := []string{fmt.Sprintf(
		"worker dispatch retry executions: total=%d claimed=%d dispatched=%d skipped=%d failed=%d",
		len(executions),
		claimed,
		dispatched,
		skipped,
		failed,
	)}
	for _, execution := range executions {
		rows = append(rows, fmt.Sprintf(
			"worker dispatch retry execution artifact: %s artifact_b64=%s status=%s mode=%s retry_dispatch=%s retry_dispatch_b64=%s claimed=%d dispatched=%d skipped=%d failed=%d",
			compactRuntimeRowValue(execution.Path),
			runtimeRowValueB64(execution.Path),
			installRequestDisplayValue(execution.Status, "unknown"),
			installRequestDisplayValue(execution.Mode, "execute"),
			installRequestDisplayValue(compactRuntimeRowValue(execution.RetryDispatchPath), "none"),
			runtimeRowValueB64(execution.RetryDispatchPath),
			execution.Summary.Claimed,
			execution.Summary.Dispatched,
			execution.Summary.Skipped,
			execution.Summary.Failed,
		))
		rows = append(rows, fmt.Sprintf(
			"dispatch retry execution: %s %s %s retry_dispatch=%s claimed=%d dispatched=%d skipped=%d failed=%d",
			installRequestDisplayValue(filepath.Base(execution.Path), "unknown"),
			installRequestDisplayValue(execution.Status, "unknown"),
			installRequestDisplayValue(execution.Mode, "execute"),
			installRequestDisplayValue(filepath.Base(execution.RetryDispatchPath), "none"),
			execution.Summary.Claimed,
			execution.Summary.Dispatched,
			execution.Summary.Skipped,
			execution.Summary.Failed,
		))
		for _, result := range execution.Results {
			rows = append(rows, fmt.Sprintf(
				"worker dispatch retry execution result: source_result=%s source_result_b64=%s source_dispatch=%s source_dispatch_b64=%s source_claim=%s source_claim_b64=%s retry_claim=%s retry_claim_b64=%s queue_dispatch=%s queue_dispatch_b64=%s request_path=%s request_path_b64=%s transport=%s worker=%s worker_b64=%s run=%s run_b64=%s execution=%s execution_b64=%s status=%s reason=%s reason_b64=%s",
				installRequestDisplayValue(compactRuntimeRowValue(result.SourceResultPath), "none"),
				runtimeRowValueB64(result.SourceResultPath),
				installRequestDisplayValue(compactRuntimeRowValue(result.SourceDispatchPath), "none"),
				runtimeRowValueB64(result.SourceDispatchPath),
				installRequestDisplayValue(compactRuntimeRowValue(result.SourceClaimPath), "none"),
				runtimeRowValueB64(result.SourceClaimPath),
				installRequestDisplayValue(compactRuntimeRowValue(result.RetryClaimPath), "none"),
				runtimeRowValueB64(result.RetryClaimPath),
				installRequestDisplayValue(compactRuntimeRowValue(result.RetryQueueDispatchPath), "none"),
				runtimeRowValueB64(result.RetryQueueDispatchPath),
				installRequestDisplayValue(compactRuntimeRowValue(result.RetryTransportRequestPath), "none"),
				runtimeRowValueB64(result.RetryTransportRequestPath),
				installRequestDisplayValue(result.RetryTransportMode, "none"),
				installRequestDisplayValue(compactRuntimeRowValue(result.WorkerID), "none"),
				runtimeRowValueB64(result.WorkerID),
				installRequestDisplayValue(compactRuntimeRowValue(result.RunID), "none"),
				runtimeRowValueB64(result.RunID),
				installRequestDisplayValue(compactRuntimeRowValue(result.ExecutionID), "none"),
				runtimeRowValueB64(result.ExecutionID),
				installRequestDisplayValue(result.Status, "unknown"),
				installRequestDisplayValue(compactRuntimeRowValue(result.Reason), "none"),
				runtimeRowValueB64(result.Reason),
			))
			if result.RetryClaimPath != "" {
				rows = append(rows, fmt.Sprintf(
					"dispatch retry execution retry_claim=%s source_result=%s",
					installRequestDisplayValue(filepath.Base(result.RetryClaimPath), "none"),
					installRequestDisplayValue(filepath.Base(result.SourceResultPath), "none"),
				))
			}
			if result.RetryQueueDispatchPath != "" {
				rows = append(rows, fmt.Sprintf(
					"dispatch retry handoff status=%s transport=%s",
					installRequestDisplayValue(result.Status, "unknown"),
					installRequestDisplayValue(result.RetryTransportMode, "none"),
				))
				rows = append(rows, fmt.Sprintf(
					"dispatch retry handoff queue_dispatch=%s",
					installRequestDisplayValue(filepath.Base(result.RetryQueueDispatchPath), "none"),
				))
				rows = append(rows, fmt.Sprintf(
					"dispatch retry handoff request=%s",
					installRequestDisplayValue(filepath.Base(result.RetryTransportRequestPath), "none"),
				))
			}
		}
	}
	return rows
}

func tuiModelWithRuntimeWorkerDispatchRetrySchedulerRunRows(root string, model tui.Model) tui.Model {
	runs, err := ListRuntimeWorkerDispatchRetrySchedulerRuns(root)
	if err != nil || len(runs) == 0 {
		return model
	}
	return model.AppendRuntimeRows(tui.RuntimeTabAgentRuntime, runtimeWorkerDispatchRetrySchedulerRunRuntimeRows(runs))
}

func runtimeWorkerDispatchRetrySchedulerRunRuntimeRows(runs []RuntimeWorkerDispatchRetrySchedulerRun) []string {
	if len(runs) == 0 {
		return nil
	}
	rows := []string{fmt.Sprintf("worker dispatch retry scheduler runs: total=%d", len(runs))}
	for _, run := range runs {
		maxRuns := run.MaxRunsRequested
		if maxRuns == 0 {
			maxRuns = run.MaxRuns
		}
		status := installRequestDisplayValue(run.Status, "unknown")
		mode := installRequestDisplayValue(run.Mode, "metadata-only")
		transport := installRequestDisplayValue(run.RetryTransportMode, "metadata-only")
		retryPlansCreated := run.RetryPlansCreated
		if retryPlansCreated == 0 {
			retryPlansCreated = len(run.RetryPlanPaths)
		}
		retryDispatchesCreated := run.RetryDispatchesCreated
		if retryDispatchesCreated == 0 {
			retryDispatchesCreated = len(run.RetryDispatchPaths)
		}
		retryExecutionsCreated := run.RetryDispatchExecutionsCreated
		if retryExecutionsCreated == 0 {
			retryExecutionsCreated = len(run.RetryDispatchExecutionPaths)
		}
		skipReason := installRequestDisplayValue(compactRuntimeRowValue(run.SkipReason), "none")
		errorValue := installRequestDisplayValue(compactRuntimeRowValue(run.Error), "none")
		rows = append(rows, fmt.Sprintf(
			"worker dispatch retry scheduler run artifact: %s status=%s mode=%s max_runs=%d dispatch_retry_claims=%t retry_transport=%s retry_plans_created=%d retry_dispatches_created=%d retry_dispatch_executions_created=%d retry_dispatch_claims_created=%d retry_dispatch_claims_dispatched=%d retry_dispatch_transport_requests_created=%d skip=%s skip_b64=%s error=%s error_b64=%s",
			run.Path,
			status,
			mode,
			maxRuns,
			run.DispatchRetryClaims,
			transport,
			retryPlansCreated,
			retryDispatchesCreated,
			retryExecutionsCreated,
			run.RetryClaimsCreated,
			run.RetryClaimsDispatched,
			run.RetryTransportRequestsCreated,
			skipReason,
			runtimeRowValueB64(run.SkipReason),
			errorValue,
			runtimeRowValueB64(run.Error),
		))
		rows = append(rows, fmt.Sprintf(
			"worker dispatch retry scheduler run compact: status=%s mode=%s max_runs=%d retry_plans_created=%d retry_dispatches_created=%d retry_dispatch_executions_created=%d",
			status,
			mode,
			maxRuns,
			retryPlansCreated,
			retryDispatchesCreated,
			retryExecutionsCreated,
		))
		if run.DispatchRetryClaims || run.RetryClaimsCreated > 0 || run.RetryClaimsDispatched > 0 || run.RetryTransportRequestsCreated > 0 {
			rows = append(rows, fmt.Sprintf(
				"worker dispatch retry scheduler run handoff: dispatch_retry_claims=%t retry_transport=%s claims_created=%d claims_dispatched=%d transport_requests=%d",
				run.DispatchRetryClaims,
				transport,
				run.RetryClaimsCreated,
				run.RetryClaimsDispatched,
				run.RetryTransportRequestsCreated,
			))
		}
		if strings.TrimSpace(run.SkipReason) != "" {
			rows = append(rows, "worker dispatch retry scheduler run skip: "+compactRuntimeRowValue(run.SkipReason))
		}
		if strings.TrimSpace(run.Error) != "" {
			rows = append(rows, "worker dispatch retry scheduler run error: "+compactRuntimeRowValue(run.Error))
		}
		rows = append(rows, "worker dispatch retry scheduler run path: "+filepath.Base(run.Path))
	}
	return rows
}

func tuiModelWithWorkerLifecycleReviewRows(root string, model tui.Model) tui.Model {
	rows := []string{}
	review, err := PlanWorkerLifecycleReview(root, time.Now().UTC(), defaultWorkerLeaseStaleAfter)
	if err == nil && review.LeaseSummary.Total > 0 {
		rows = append(rows, workerLifecycleReviewRuntimeRows(review)...)
	}
	reviews, err := ListWorkerLifecycleReviews(root)
	if err == nil {
		rows = append(rows, workerLifecycleReviewArtifactRuntimeRows(reviews)...)
	}
	if len(rows) == 0 {
		return model
	}
	return model.AppendRuntimeRows(tui.RuntimeTabAgentRuntime, rows)
}

func workerLifecycleReviewRuntimeRows(review WorkerLifecycleReviewPlan) []string {
	summary := review.LeaseSummary
	queue := review.QueueSummary
	transport := review.TransportSummary
	rows := []string{
		fmt.Sprintf(
			"worker lifecycle review: leases=%d active=%d stale=%d completed=%d failed=%d reclaimed=%d fresh_active=%d other=%d",
			summary.Total,
			summary.Active,
			summary.StaleActive,
			summary.Completed,
			summary.Failed,
			summary.Reclaimed,
			summary.FreshActive,
			summary.Other,
		),
		fmt.Sprintf(
			"worker lifecycle review readiness: reclaim_ready=%d reclaim_skipped=%d reconciliation_ready=%d reconciliation_skipped=%d",
			len(review.Reclaim.Ready),
			len(review.Reclaim.Skipped),
			len(review.Reconciliation.Ready),
			len(review.Reconciliation.Skipped),
		),
		"worker lifecycle review mode=" + installRequestDisplayValue(review.Mode, "metadata-only"),
	}
	if queue.Total > 0 {
		rows = append(rows, fmt.Sprintf(
			"worker lifecycle review queue: claims=%d claimed=%d dispatched=%d completed=%d failed=%d canceled=%d expired=%d",
			queue.Total,
			queue.Claimed,
			queue.Dispatched,
			queue.Completed,
			queue.Failed,
			queue.Canceled,
			queue.Expired,
		))
		rows = append(rows, fmt.Sprintf(
			"worker lifecycle queue: claims=%d dispatched=%d completed=%d",
			queue.Total,
			queue.Dispatched,
			queue.Completed,
		))
		rows = append(rows, fmt.Sprintf(
			"worker lifecycle queue closed: failed=%d canceled=%d expired=%d",
			queue.Failed,
			queue.Canceled,
			queue.Expired,
		))
	}
	if transport.RequestsTotal > 0 || transport.DispatchResultsTotal > 0 {
		rows = append(rows, fmt.Sprintf(
			"worker lifecycle review transport: requests=%d pending=%d ack=%d completed=%d failed=%d results=%d result_completed=%d result_failed=%d",
			transport.RequestsTotal,
			transport.Pending,
			transport.Acknowledged,
			transport.Completed,
			transport.Failed,
			transport.DispatchResultsTotal,
			transport.DispatchResultsCompleted,
			transport.DispatchResultsFailed,
		))
		rows = append(rows, fmt.Sprintf(
			"worker lifecycle transport: requests=%d pending=%d ack=%d completed=%d failed=%d",
			transport.RequestsTotal,
			transport.Pending,
			transport.Acknowledged,
			transport.Completed,
			transport.Failed,
		))
		rows = append(rows, fmt.Sprintf(
			"worker lifecycle results: total=%d completed=%d failed=%d",
			transport.DispatchResultsTotal,
			transport.DispatchResultsCompleted,
			transport.DispatchResultsFailed,
		))
	}
	return rows
}

func workerLifecycleReviewArtifactRuntimeRows(reviews []WorkerLifecycleReview) []string {
	if len(reviews) == 0 {
		return nil
	}
	rows := []string{fmt.Sprintf("worker lifecycle review artifacts: total=%d", len(reviews))}
	for _, review := range reviews {
		summary := review.LeaseSummary
		queue := review.QueueSummary
		transport := review.TransportSummary
		rows = append(rows, fmt.Sprintf(
			"worker lifecycle review artifact status=%s mode=%s",
			installRequestDisplayValue(review.Status, "unknown"),
			installRequestDisplayValue(review.Mode, "unknown"),
		))
		rows = append(rows, fmt.Sprintf(
			"worker lifecycle review artifact state: %s mode=%s leases=%d stale=%d",
			installRequestDisplayValue(review.Status, "unknown"),
			installRequestDisplayValue(review.Mode, "unknown"),
			summary.Total,
			summary.StaleActive,
		))
		rows = append(rows, fmt.Sprintf(
			"worker lifecycle review artifact readiness: reclaim_ready=%d reconciliation_ready=%d",
			len(review.Reclaim.Ready),
			len(review.Reconciliation.Ready),
		))
		if queue.Total > 0 {
			rows = append(rows, fmt.Sprintf(
				"worker lifecycle artifact queue: claims=%d dispatched=%d completed=%d",
				queue.Total,
				queue.Dispatched,
				queue.Completed,
			))
		}
		if transport.RequestsTotal > 0 || transport.DispatchResultsTotal > 0 {
			rows = append(rows, fmt.Sprintf(
				"worker lifecycle artifact transport: requests=%d pending=%d ack=%d results=%d",
				transport.RequestsTotal,
				transport.Pending,
				transport.Acknowledged,
				transport.DispatchResultsTotal,
			))
		}
		rows = append(rows, fmt.Sprintf(
			"worker lifecycle review artifact: %s status=%s mode=%s leases=%d active=%d stale=%d completed=%d failed=%d reclaimed=%d reclaim_ready=%d reclaim_skipped=%d reconciliation_ready=%d reconciliation_skipped=%d",
			review.Path,
			installRequestDisplayValue(review.Status, "unknown"),
			installRequestDisplayValue(review.Mode, "unknown"),
			summary.Total,
			summary.Active,
			summary.StaleActive,
			summary.Completed,
			summary.Failed,
			summary.Reclaimed,
			len(review.Reclaim.Ready),
			len(review.Reclaim.Skipped),
			len(review.Reconciliation.Ready),
			len(review.Reconciliation.Skipped),
		))
		if queue.Total > 0 || transport.RequestsTotal > 0 || transport.DispatchResultsTotal > 0 {
			rows = append(rows, fmt.Sprintf(
				"worker lifecycle review artifact queue: claims=%d dispatched=%d completed=%d transport_requests=%d pending=%d ack=%d results=%d",
				queue.Total,
				queue.Dispatched,
				queue.Completed,
				transport.RequestsTotal,
				transport.Pending,
				transport.Acknowledged,
				transport.DispatchResultsTotal,
			))
		}
		rows = append(rows, "worker lifecycle review artifact path: "+filepath.Base(review.Path))
	}
	return rows
}

func tuiModelWithWorkerLifecycleAuditRows(root string, model tui.Model) tui.Model {
	audits, err := ListWorkerLifecycleAudits(root)
	if err != nil || len(audits) == 0 {
		return model
	}
	return model.AppendRuntimeRows(tui.RuntimeTabAgentRuntime, workerLifecycleAuditArtifactRuntimeRows(audits))
}

func workerLifecycleAuditArtifactRuntimeRows(audits []WorkerLifecycleAudit) []string {
	if len(audits) == 0 {
		return nil
	}
	rows := []string{fmt.Sprintf("worker lifecycle audit artifacts: total=%d", len(audits))}
	for _, audit := range audits {
		summary := audit.Summary
		rows = append(rows, fmt.Sprintf(
			"worker lifecycle audit artifact status=%s mode=%s",
			installRequestDisplayValue(audit.Status, "unknown"),
			installRequestDisplayValue(audit.Mode, "unknown"),
		))
		rows = append(rows, fmt.Sprintf(
			"worker lifecycle audit artifact: %s status=%s mode=%s workers=%d stale_workers=%d placements=%d claims=%d dispatches=%d requests=%d results=%d retry_plans=%d retry_dispatches=%d retry_dispatch_executions=%d leases=%d reclaims=%d reconciliations=%d lifecycle_reviews=%d lifecycle_policies=%d policy_selections=%d policy_runs=%d policy_scheduler_runs=%d action_plans=%d action_plan_annotations=%d action_plan_selections=%d maintenances=%d maintenance_supervisors=%d orchestration_supervisors=%d actions=%d daemon_runs=%d supervisor_runs=%d controls=%d",
			audit.Path,
			installRequestDisplayValue(audit.Status, "unknown"),
			installRequestDisplayValue(audit.Mode, "unknown"),
			summary.WorkerRegistrations,
			summary.StaleWorkerRegistrations,
			summary.PlacementPlans+summary.PlacementClaimExecutions+summary.PlacementDispatchPlans+summary.PlacementDispatchExecutions+summary.PlacementDispatchBatches,
			summary.QueueClaims,
			summary.QueueDispatchExecutions,
			summary.TransportRequests,
			summary.QueueDispatchResults,
			summary.DispatchRetryPlans,
			summary.DispatchRetryDispatches,
			summary.DispatchRetryDispatchExecutions,
			summary.WorkerLeases,
			summary.ReclaimReviews+summary.ReclaimExecutions,
			summary.ReconciliationReviews+summary.ReconciliationExecutions,
			summary.LifecycleReviews,
			summary.LifecyclePolicies,
			summary.LifecyclePolicySelections,
			summary.LifecyclePolicyRuns,
			summary.LifecyclePolicySchedulerRuns,
			summary.ActionPlans,
			summary.ActionPlanAnnotations,
			summary.ActionPlanSelections,
			summary.LifecycleMaintenances,
			summary.LifecycleMaintenanceSupervisors,
			summary.WorkerOrchestrationSupervisors,
			summary.ActionPlanActions,
			summary.DaemonRuns,
			summary.DaemonSupervisorRuns,
			summary.DaemonControls,
		))
		rows = append(rows, fmt.Sprintf(
			"worker lifecycle audit artifact state: %s mode=%s workers=%d stale=%d artifacts=%d",
			installRequestDisplayValue(audit.Status, "unknown"),
			installRequestDisplayValue(audit.Mode, "unknown"),
			summary.WorkerRegistrations,
			summary.StaleWorkerRegistrations,
			summary.ArtifactTotal,
		))
		rows = append(rows, fmt.Sprintf(
			"worker lifecycle audit placement: plans=%d claims=%d dispatch_plans=%d batches=%d",
			summary.PlacementPlans,
			summary.PlacementClaimExecutions,
			summary.PlacementDispatchPlans,
			summary.PlacementDispatchBatches,
		))
		rows = append(rows, fmt.Sprintf("worker lifecycle audit orchestration: orchestrations=%d", summary.WorkerOrchestrations))
		rows = append(rows, fmt.Sprintf("worker lifecycle audit orchestration supervisors: total=%d", summary.WorkerOrchestrationSupervisors))
		rows = append(rows, fmt.Sprintf(
			"worker lifecycle audit queue: claims=%d dispatches=%d",
			summary.QueueClaims,
			summary.QueueDispatchExecutions,
		))
		rows = append(rows, fmt.Sprintf(
			"worker lifecycle audit transport: requests=%d results=%d",
			summary.TransportRequests,
			summary.QueueDispatchResults,
		))
		rows = append(rows, fmt.Sprintf("worker lifecycle audit retry plans: total=%d retry_plans=%d", summary.DispatchRetryPlans, summary.DispatchRetryPlans))
		rows = append(rows, fmt.Sprintf("worker lifecycle audit retry dispatches: total=%d retry_dispatches=%d", summary.DispatchRetryDispatches, summary.DispatchRetryDispatches))
		rows = append(rows, fmt.Sprintf("worker lifecycle audit retry executions: total=%d retry_dispatch_executions=%d", summary.DispatchRetryDispatchExecutions, summary.DispatchRetryDispatchExecutions))
		rows = append(rows, fmt.Sprintf(
			"worker lifecycle audit policies: total=%d selections=%d policy_runs=%d scheduler_runs=%d",
			summary.LifecyclePolicies,
			summary.LifecyclePolicySelections,
			summary.LifecyclePolicyRuns,
			summary.LifecyclePolicySchedulerRuns,
		))
		rows = append(rows, fmt.Sprintf(
			"worker lifecycle audit policy fields: lifecycle_policies=%d policy_selections=%d",
			summary.LifecyclePolicies,
			summary.LifecyclePolicySelections,
		))
		rows = append(rows, fmt.Sprintf(
			"worker lifecycle audit policy run fields: policy_runs=%d policy_scheduler_runs=%d",
			summary.LifecyclePolicyRuns,
			summary.LifecyclePolicySchedulerRuns,
		))
		rows = append(rows, fmt.Sprintf(
			"worker lifecycle audit action-plan markers: action_plan_selections=%d action_plan_annotations=%d",
			summary.ActionPlanSelections,
			summary.ActionPlanAnnotations,
		))
		rows = append(rows, fmt.Sprintf(
			"worker lifecycle audit reviews: lifecycle=%d action_plans=%d action_plan_annotations=%d action_plan_selections=%d maintenances=%d maintenance_supervisors=%d actions=%d",
			summary.LifecycleReviews,
			summary.ActionPlans,
			summary.ActionPlanAnnotations,
			summary.ActionPlanSelections,
			summary.LifecycleMaintenances,
			summary.LifecycleMaintenanceSupervisors,
			summary.ActionPlanActions,
		))
		rows = append(rows, fmt.Sprintf("worker lifecycle audit maintenance supervisors: total=%d", summary.LifecycleMaintenanceSupervisors))
		rows = append(rows, "worker lifecycle audit artifact path: "+filepath.Base(audit.Path))
		for _, item := range audit.Items {
			rows = append(rows, fmt.Sprintf(
				"worker lifecycle audit item: kind=%s status=%s mode=%s gate=%s worker=%s run=%s execution=%s path=%s linked=%s reason=%s reason_b64=%s%s",
				installRequestDisplayValue(item.Kind, "unknown"),
				installRequestDisplayValue(item.Status, "unknown"),
				installRequestDisplayValue(item.Mode, "none"),
				installRequestDisplayValue(item.Gate, "none"),
				installRequestDisplayValue(item.WorkerID, "none"),
				installRequestDisplayValue(item.RunID, "none"),
				installRequestDisplayValue(item.ExecutionID, "none"),
				installRequestDisplayValue(item.ArtifactPath, "none"),
				installRequestDisplayValue(item.LinkedPath, "none"),
				installRequestDisplayValue(item.Reason, "none"),
				runtimeRowValueB64(item.Reason),
				workerLifecycleAuditItemAggregateRuntimeSuffix(item),
			))
			if workerLifecycleAuditItemHasAggregate(item) {
				rows = append(rows, workerLifecycleAuditItemAggregateRuntimeRows(item)...)
			}
		}
	}
	return rows
}

func workerLifecycleAuditItemAggregateRuntimeLabel(item WorkerLifecycleAuditItem) string {
	switch item.Kind {
	case "worker-lifecycle-action-plan":
		return "worker lifecycle audit action plan"
	case "worker-lifecycle-action-plan-selection":
		return "worker lifecycle audit action plan selection"
	case "worker-lifecycle-maintenance":
		return "worker lifecycle audit maintenance"
	case "worker-lifecycle-maintenance-supervisor":
		return "worker lifecycle audit supervisor"
	case "worker-dispatch-retry-plan":
		return "worker lifecycle audit retry plan"
	case "worker-dispatch-retry-dispatch":
		return "worker lifecycle audit retry dispatch"
	case "worker-dispatch-retry-dispatch-execution":
		return "worker lifecycle audit retry dispatch execution"
	case "worker-lease-reclaim-review":
		return "worker lifecycle audit reclaim review"
	case "worker-lease-reclaim-execution":
		return "worker lifecycle audit reclaim execution"
	case "worker-lease-result-reconciliation-review":
		return "worker lifecycle audit reconciliation review"
	case "worker-lease-result-reconciliation-execution":
		return "worker lifecycle audit reconciliation execution"
	case "worker-orchestration":
		return "worker lifecycle audit orchestration"
	case "worker-orchestration-supervisor":
		return "worker lifecycle audit orchestration supervisor"
	default:
		return "worker lifecycle audit item aggregate"
	}
}

func workerLifecycleAuditItemAggregateRuntimeRows(item WorkerLifecycleAuditItem) []string {
	label := workerLifecycleAuditItemAggregateRuntimeLabel(item)
	rows := []string{}
	if workerLifecycleAuditItemHasActionAggregate(item) {
		rows = append(rows, fmt.Sprintf("%s actions=%d", label, item.ActionsTotal))
		rows = append(rows, fmt.Sprintf("%s skipped_unsafe=%d", label, item.SkippedUnsafeActions))
		rows = append(rows, fmt.Sprintf("%s retry_dispatch_execute=%d", label, item.RetryDispatchExecuteActions))
	}
	if strings.TrimSpace(item.Decision) != "" {
		rows = append(rows, fmt.Sprintf(
			"%s review: decision=%s reviewer=%s reviewed_at=%s reason=%s",
			label,
			installRequestDisplayValue(item.Decision, "unknown"),
			installRequestDisplayValue(item.Reviewer, "unknown"),
			installRequestDisplayValue(item.ReviewedAt, "unknown"),
			installRequestDisplayValue(item.Reason, "none"),
		))
		rows = append(rows, fmt.Sprintf(
			"audit action-plan review decision=%s reviewer=%s",
			installRequestDisplayValue(item.Decision, "unknown"),
			installRequestDisplayValue(item.Reviewer, "unknown"),
		))
		rows = append(rows, fmt.Sprintf(
			"audit action-plan review reviewed_at=%s reason=%s",
			installRequestDisplayValue(item.ReviewedAt, "unknown"),
			installRequestDisplayValue(item.Reason, "none"),
		))
		rows = append(rows, "audit action-plan review reason="+installRequestDisplayValue(item.Reason, "none"))
	}
	if strings.TrimSpace(item.SelectedBy) != "" || strings.TrimSpace(item.ClearedBy) != "" {
		if strings.TrimSpace(item.ClearedBy) != "" {
			rows = append(rows, fmt.Sprintf(
				"%s marker: status=%s cleared_by=%s cleared_at=%s reason=%s linked=%s",
				label,
				installRequestDisplayValue(item.Status, "unknown"),
				installRequestDisplayValue(item.ClearedBy, "unknown"),
				installRequestDisplayValue(item.ClearedAt, "unknown"),
				installRequestDisplayValue(item.Reason, "none"),
				installRequestDisplayValue(filepath.Base(item.LinkedPath), "none"),
			))
			rows = append(rows, fmt.Sprintf(
				"audit action-plan marker cleared_by=%s cleared_at=%s",
				installRequestDisplayValue(item.ClearedBy, "unknown"),
				installRequestDisplayValue(item.ClearedAt, "unknown"),
			))
			rows = append(rows, "audit action-plan marker linked="+installRequestDisplayValue(filepath.Base(item.LinkedPath), "none"))
		} else {
			rows = append(rows, fmt.Sprintf(
				"%s marker: status=%s selected_by=%s selected_at=%s reason=%s linked=%s",
				label,
				installRequestDisplayValue(item.Status, "unknown"),
				installRequestDisplayValue(item.SelectedBy, "unknown"),
				installRequestDisplayValue(item.SelectedAt, "unknown"),
				installRequestDisplayValue(item.Reason, "none"),
				installRequestDisplayValue(filepath.Base(item.LinkedPath), "none"),
			))
			rows = append(rows, fmt.Sprintf(
				"audit action-plan marker selected_by=%s selected_at=%s",
				installRequestDisplayValue(item.SelectedBy, "unknown"),
				installRequestDisplayValue(item.SelectedAt, "unknown"),
			))
			rows = append(rows, "audit action-plan marker linked="+installRequestDisplayValue(filepath.Base(item.LinkedPath), "none"))
		}
	}
	if workerLifecycleAuditItemHasRetryAggregate(item) {
		if item.Kind == "worker-dispatch-retry-dispatch-execution" {
			rows = append(rows, fmt.Sprintf("%s: total=%d ready=%d claimed=%d", label, item.TotalCount, item.ReadyCount, item.ClaimedCount))
			if item.SkippedCount > 0 || item.FailedCount > 0 {
				rows = append(rows, fmt.Sprintf("%s outcome: skipped=%d failed=%d", label, item.SkippedCount, item.FailedCount))
			}
		} else {
			row := fmt.Sprintf("%s: total=%d ready=%d", label, item.TotalCount, item.ReadyCount)
			if item.SkippedCount > 0 {
				row += fmt.Sprintf(" skipped=%d", item.SkippedCount)
			}
			rows = append(rows, row)
		}
	}
	if workerLifecycleAuditItemHasLeaseRecoveryAggregate(item) {
		switch item.Kind {
		case "worker-lease-reclaim-execution", "worker-lease-result-reconciliation-execution":
			row := fmt.Sprintf("%s: total=%d results=%d", label, item.TotalCount, item.ResultCount)
			if item.SkippedCount > 0 {
				row += fmt.Sprintf(" skipped=%d", item.SkippedCount)
			}
			rows = append(rows, row)
		default:
			row := fmt.Sprintf("%s: total=%d ready=%d", label, item.TotalCount, item.ReadyCount)
			if item.SkippedCount > 0 {
				row += fmt.Sprintf(" skipped=%d", item.SkippedCount)
			}
			rows = append(rows, row)
		}
	}
	if workerLifecycleAuditItemHasOrchestrationAggregate(item) {
		if item.PlacementReady > 0 || item.PlacementSkipped > 0 {
			rows = append(rows, fmt.Sprintf("%s placement: ready=%d skipped=%d", label, item.PlacementReady, item.PlacementSkipped))
		}
		if item.DispatchReady > 0 || item.DispatchSkipped > 0 {
			rows = append(rows, fmt.Sprintf("%s dispatch ready=%d skipped=%d", label, item.DispatchReady, item.DispatchSkipped))
		}
		if item.OrchestrationRuns > 0 || item.MaxRuns > 0 {
			rows = append(rows, fmt.Sprintf("%s runs=%d max_runs=%d", label, item.OrchestrationRuns, item.MaxRuns))
		}
		if item.OrchestrationsCreated > 0 {
			rows = append(rows, fmt.Sprintf("%s created: orchestrations=%d", label, item.OrchestrationsCreated))
		}
		if item.LifecycleMaintenanceCreated > 0 || item.LifecycleAuditCreated > 0 {
			rows = append(rows, fmt.Sprintf("%s lifecycle created: maintenance=%d audit=%d", label, item.LifecycleMaintenanceCreated, item.LifecycleAuditCreated))
		}
		if item.ClaimedCount > 0 || item.DispatchedCount > 0 {
			rows = append(rows, fmt.Sprintf("%s dispatch: claimed=%d dispatched=%d", label, item.ClaimedCount, item.DispatchedCount))
		}
		if item.SkippedCount > 0 || item.FailedCount > 0 || item.DispatchSkipped > 0 {
			rows = append(rows, fmt.Sprintf("%s dispatch skipped=%d failed=%d", label, item.SkippedCount, item.FailedCount))
		}
	}
	return rows
}

func workerLifecycleAuditItemHasAggregate(item WorkerLifecycleAuditItem) bool {
	return workerLifecycleAuditItemHasActionAggregate(item) ||
		workerLifecycleAuditItemHasRetryAggregate(item) ||
		workerLifecycleAuditItemHasLeaseRecoveryAggregate(item) ||
		workerLifecycleAuditItemHasOrchestrationAggregate(item)
}

func workerLifecycleAuditItemHasActionAggregate(item WorkerLifecycleAuditItem) bool {
	return item.ActionsTotal > 0 ||
		item.AckActions > 0 ||
		item.RunActions > 0 ||
		item.DaemonControlActions > 0 ||
		item.RetryDispatchExecuteActions > 0 ||
		item.ReclaimActions > 0 ||
		item.ReconciliationActions > 0 ||
		item.SkippedUnsafeActions > 0
}

func workerLifecycleAuditItemHasRetryAggregate(item WorkerLifecycleAuditItem) bool {
	switch item.Kind {
	case "worker-dispatch-retry-plan", "worker-dispatch-retry-dispatch":
		return item.TotalCount > 0 ||
			item.ReadyCount > 0 ||
			item.SkippedCount > 0
	case "worker-dispatch-retry-dispatch-execution":
		return item.TotalCount > 0 ||
			item.ReadyCount > 0 ||
			item.ClaimedCount > 0 ||
			item.SkippedCount > 0 ||
			item.FailedCount > 0
	default:
		return false
	}
}

func workerLifecycleAuditItemHasLeaseRecoveryAggregate(item WorkerLifecycleAuditItem) bool {
	switch item.Kind {
	case "worker-lease-reclaim-review", "worker-lease-result-reconciliation-review":
		return item.TotalCount > 0 ||
			item.ReadyCount > 0 ||
			item.SkippedCount > 0
	case "worker-lease-reclaim-execution", "worker-lease-result-reconciliation-execution":
		return item.TotalCount > 0 ||
			item.ResultCount > 0 ||
			item.SkippedCount > 0
	default:
		return false
	}
}

func workerLifecycleAuditItemHasOrchestrationAggregate(item WorkerLifecycleAuditItem) bool {
	switch item.Kind {
	case "worker-orchestration", "worker-orchestration-supervisor":
	default:
		return false
	}
	return item.PlacementReady > 0 ||
		item.PlacementSkipped > 0 ||
		item.DispatchReady > 0 ||
		item.DispatchSkipped > 0 ||
		item.OrchestrationRuns > 0 ||
		item.MaxRuns > 0 ||
		item.OrchestrationsCreated > 0 ||
		item.LifecycleMaintenanceCreated > 0 ||
		item.LifecycleAuditCreated > 0 ||
		item.ClaimedCount > 0 ||
		item.DispatchedCount > 0 ||
		item.SkippedCount > 0 ||
		item.FailedCount > 0
}

func workerLifecycleAuditItemAggregateRuntimeSuffix(item WorkerLifecycleAuditItem) string {
	parts := make([]string, 0, 17)
	if item.TotalCount > 0 {
		parts = append(parts, fmt.Sprintf("total=%d", item.TotalCount))
	}
	if item.ReadyCount > 0 {
		parts = append(parts, fmt.Sprintf("ready=%d", item.ReadyCount))
	}
	if item.ResultCount > 0 {
		parts = append(parts, fmt.Sprintf("results=%d", item.ResultCount))
	}
	if item.ActionsTotal > 0 {
		parts = append(parts, fmt.Sprintf("actions=%d", item.ActionsTotal))
	}
	if strings.TrimSpace(item.Decision) != "" {
		parts = append(parts, fmt.Sprintf("decision=%s", item.Decision))
	}
	if strings.TrimSpace(item.Reviewer) != "" {
		parts = append(parts, fmt.Sprintf("reviewer=%s", item.Reviewer))
	}
	if strings.TrimSpace(item.ReviewedAt) != "" {
		parts = append(parts, fmt.Sprintf("reviewed_at=%s", item.ReviewedAt))
	}
	if strings.TrimSpace(item.SelectedBy) != "" {
		parts = append(parts, fmt.Sprintf("selected_by=%s", item.SelectedBy))
	}
	if strings.TrimSpace(item.SelectedAt) != "" {
		parts = append(parts, fmt.Sprintf("selected_at=%s", item.SelectedAt))
	}
	if strings.TrimSpace(item.ClearedBy) != "" {
		parts = append(parts, fmt.Sprintf("cleared_by=%s", item.ClearedBy))
	}
	if strings.TrimSpace(item.ClearedAt) != "" {
		parts = append(parts, fmt.Sprintf("cleared_at=%s", item.ClearedAt))
	}
	if item.SupersededCount > 0 {
		parts = append(parts, fmt.Sprintf("superseded=%d", item.SupersededCount))
	}
	if item.AckActions > 0 {
		parts = append(parts, fmt.Sprintf("ack_actions=%d", item.AckActions))
	}
	if item.RunActions > 0 {
		parts = append(parts, fmt.Sprintf("run_actions=%d", item.RunActions))
	}
	if item.DaemonControlActions > 0 {
		parts = append(parts, fmt.Sprintf("daemon_control_actions=%d", item.DaemonControlActions))
	}
	if item.RetryDispatchExecuteActions > 0 {
		parts = append(parts, fmt.Sprintf("retry_dispatch_execute=%d", item.RetryDispatchExecuteActions))
	}
	if item.ReclaimActions > 0 {
		parts = append(parts, fmt.Sprintf("reclaim_actions=%d", item.ReclaimActions))
	}
	if item.ReconciliationActions > 0 {
		parts = append(parts, fmt.Sprintf("reconciliation_actions=%d", item.ReconciliationActions))
	}
	if item.SkippedUnsafeActions > 0 {
		parts = append(parts, fmt.Sprintf("skipped_unsafe=%d", item.SkippedUnsafeActions))
	}
	if item.PlacementReady > 0 {
		parts = append(parts, fmt.Sprintf("placement_ready=%d", item.PlacementReady))
	}
	if item.PlacementSkipped > 0 {
		parts = append(parts, fmt.Sprintf("placement_skipped=%d", item.PlacementSkipped))
	}
	if item.DispatchReady > 0 {
		parts = append(parts, fmt.Sprintf("dispatch_ready=%d", item.DispatchReady))
	}
	if item.DispatchSkipped > 0 {
		parts = append(parts, fmt.Sprintf("dispatch_skipped=%d", item.DispatchSkipped))
	}
	if item.OrchestrationRuns > 0 {
		parts = append(parts, fmt.Sprintf("runs=%d", item.OrchestrationRuns))
	}
	if item.MaxRuns > 0 {
		parts = append(parts, fmt.Sprintf("max_runs=%d", item.MaxRuns))
	}
	if item.OrchestrationsCreated > 0 {
		parts = append(parts, fmt.Sprintf("orchestrations_created=%d", item.OrchestrationsCreated))
	}
	if item.LifecycleMaintenanceCreated > 0 {
		parts = append(parts, fmt.Sprintf("lifecycle_maintenance=%d", item.LifecycleMaintenanceCreated))
	}
	if item.LifecycleAuditCreated > 0 {
		parts = append(parts, fmt.Sprintf("lifecycle_audit=%d", item.LifecycleAuditCreated))
	}
	if item.ClaimedCount > 0 {
		parts = append(parts, fmt.Sprintf("claimed=%d", item.ClaimedCount))
	}
	if item.DispatchedCount > 0 {
		parts = append(parts, fmt.Sprintf("dispatched=%d", item.DispatchedCount))
	}
	if item.SkippedCount > 0 {
		parts = append(parts, fmt.Sprintf("skipped=%d", item.SkippedCount))
	}
	if item.FailedCount > 0 {
		parts = append(parts, fmt.Sprintf("failed=%d", item.FailedCount))
	}
	if len(parts) == 0 {
		return ""
	}
	return " " + strings.Join(parts, " ")
}

func tuiModelWithWorkerLifecyclePolicyRows(root string, model tui.Model) tui.Model {
	policies, err := ListWorkerLifecyclePolicies(root)
	if err != nil || len(policies) == 0 {
		return model
	}
	selection, selectionFound, _ := ReadWorkerLifecyclePolicySelection(root)
	return model.AppendRuntimeRows(tui.RuntimeTabAgentRuntime, workerLifecyclePolicyArtifactRuntimeRowsWithSelection(policies, selection, selectionFound))
}

func workerLifecyclePolicyArtifactRuntimeRows(policies []WorkerLifecyclePolicy) []string {
	return workerLifecyclePolicyArtifactRuntimeRowsWithSelection(policies, WorkerLifecyclePolicySelection{}, false)
}

func workerLifecyclePolicyArtifactRuntimeRowsWithSelection(policies []WorkerLifecyclePolicy, selection WorkerLifecyclePolicySelection, selectionFound bool) []string {
	if len(policies) == 0 {
		return nil
	}
	rows := []string{fmt.Sprintf("worker lifecycle policy artifacts: total=%d", len(policies))}
	if selectionFound {
		transport := installRequestDisplayValue(selection.TransportMode, "metadata-only")
		reason := installRequestDisplayValue(strings.TrimSpace(selection.Reason), "none")
		actorFields := fmt.Sprintf(
			"selected_by=%s selected_by_b64=%s selected_at=%s selected_at_b64=%s",
			installRequestDisplayValue(selection.SelectedBy, "unknown"),
			runtimeRowValueB64(selection.SelectedBy),
			installRequestDisplayValue(selection.SelectedAt, "unknown"),
			runtimeRowValueB64(selection.SelectedAt),
		)
		if workerLifecyclePolicySelectionIsCleared(selection) {
			actorFields = fmt.Sprintf(
				"cleared_by=%s cleared_by_b64=%s cleared_at=%s cleared_at_b64=%s",
				installRequestDisplayValue(selection.ClearedBy, "unknown"),
				runtimeRowValueB64(selection.ClearedBy),
				installRequestDisplayValue(selection.ClearedAt, "unknown"),
				runtimeRowValueB64(selection.ClearedAt),
			)
		}
		rows = append(rows, fmt.Sprintf(
			"worker lifecycle policy selection: status=%s mode=%s policy=%s policy_b64=%s selection=%s selection_b64=%s transport=%s poll_interval_ms=%d max_runs=%d stale_after=%d worker_stale_after=%d include_lifecycle_maintenance=%t execute_lifecycle_retries=%t execute_lease_recovery=%t superseded=%d %s reason=%s reason_b64=%s",
			installRequestDisplayValue(selection.Status, "unknown"),
			installRequestDisplayValue(selection.Mode, "metadata-only"),
			installRequestDisplayValue(selection.PolicyPath, "none"),
			runtimeRowValueB64(selection.PolicyPath),
			installRequestDisplayValue(selection.Path, "none"),
			runtimeRowValueB64(selection.Path),
			transport,
			selection.PollIntervalMilliseconds,
			selection.MaxSupervisorRuns,
			selection.StaleAfterSeconds,
			selection.WorkerStaleAfterSeconds,
			selection.IncludeLifecycleMaintenance,
			selection.ExecuteLifecycleRetries,
			selection.ExecuteLeaseRecovery,
			len(selection.SupersededPolicyPaths),
			actorFields,
			reason,
			runtimeRowValueB64(strings.TrimSpace(selection.Reason)),
		))
		rows = append(rows, fmt.Sprintf(
			"worker lifecycle policy selection cadence: transport=%s poll_interval_ms=%d max_runs=%d",
			transport,
			selection.PollIntervalMilliseconds,
			selection.MaxSupervisorRuns,
		))
		if selection.DispatchLifecycleRetryClaims {
			rows = append(rows, fmt.Sprintf(
				"worker lifecycle policy selection retry handoff: dispatch_lifecycle_retry_claims=%t lifecycle_retry_transport=%s",
				selection.DispatchLifecycleRetryClaims,
				installRequestDisplayValue(selection.LifecycleRetryTransportMode, "metadata-only"),
			))
		}
		if workerLifecyclePolicySelectionIsCleared(selection) {
			rows = append(rows, fmt.Sprintf(
				"worker lifecycle policy selection clear: cleared_by=%s cleared_at=%s reason=%s",
				installRequestDisplayValue(selection.ClearedBy, "unknown"),
				installRequestDisplayValue(selection.ClearedAt, "unknown"),
				reason,
			))
		} else {
			rows = append(rows, fmt.Sprintf(
				"worker lifecycle policy selection actor: selected_by=%s selected_at=%s reason=%s",
				installRequestDisplayValue(selection.SelectedBy, "unknown"),
				installRequestDisplayValue(selection.SelectedAt, "unknown"),
				reason,
			))
		}
		if trimmed := strings.TrimSpace(selection.PolicyPath); trimmed != "" {
			rows = append(rows, "worker lifecycle policy selected: policy="+trimmed)
		}
		if trimmed := strings.TrimSpace(selection.Path); trimmed != "" {
			rows = append(rows, "worker lifecycle policy selection path: selection="+trimmed)
		}
		for _, path := range selection.SupersededPolicyPaths {
			if trimmed := strings.TrimSpace(path); trimmed != "" {
				rows = append(rows, "worker lifecycle policy superseded: "+trimmed)
			}
		}
	}
	for _, policy := range policies {
		summary := policy.Summary
		status := installRequestDisplayValue(policy.Status, "unknown")
		mode := installRequestDisplayValue(policy.Mode, "metadata-only")
		transport := installRequestDisplayValue(policy.TransportMode, "metadata-only")
		current := "false"
		if selectionFound && workerLifecyclePolicySelectionIsCurrent(selection) && strings.TrimSpace(selection.PolicyPath) == strings.TrimSpace(policy.Path) {
			current = "true"
		}
		rows = append(rows, fmt.Sprintf(
			"worker lifecycle policy artifact: %s artifact_b64=%s status=%s mode=%s transport=%s stale_after=%d worker_stale_after=%d poll_interval_ms=%d max_runs=%d lifecycle_maintenance=%t lifecycle_retries=%t lease_recovery=%t current=%s workers=%d stale_workers=%d stale_leases=%d",
			policy.Path,
			runtimeRowValueB64(policy.Path),
			status,
			mode,
			transport,
			policy.StaleAfterSeconds,
			policy.WorkerStaleAfterSeconds,
			policy.PollIntervalMilliseconds,
			policy.MaxSupervisorRuns,
			policy.IncludeLifecycleMaintenance,
			policy.ExecuteLifecycleRetries,
			policy.ExecuteLeaseRecovery,
			current,
			summary.WorkerRegistrations,
			summary.StaleWorkerRegistrations,
			summary.StaleActiveLeases,
		))
		rows = append(rows, fmt.Sprintf(
			"worker lifecycle policy artifact current: current=%s policy=%s",
			current,
			policy.Path,
		))
		rows = append(rows, fmt.Sprintf(
			"worker lifecycle policy artifact state: %s mode=%s transport=%s max_runs=%d",
			status,
			mode,
			transport,
			policy.MaxSupervisorRuns,
		))
		rows = append(rows, fmt.Sprintf(
			"worker lifecycle policy artifact cadence: stale_after=%d worker_stale_after=%d poll_interval_ms=%d",
			policy.StaleAfterSeconds,
			policy.WorkerStaleAfterSeconds,
			policy.PollIntervalMilliseconds,
		))
		rows = append(rows, fmt.Sprintf(
			"worker lifecycle policy artifact gates: lifecycle_maintenance=%t lifecycle_retries=%t lease_recovery=%t",
			policy.IncludeLifecycleMaintenance,
			policy.ExecuteLifecycleRetries,
			policy.ExecuteLeaseRecovery,
		))
		if policy.DispatchLifecycleRetryClaims {
			rows = append(rows, fmt.Sprintf(
				"worker lifecycle policy artifact retry handoff: dispatch_lifecycle_retry_claims=%t lifecycle_retry_transport=%s",
				policy.DispatchLifecycleRetryClaims,
				installRequestDisplayValue(policy.LifecycleRetryTransportMode, "metadata-only"),
			))
		}
		rows = append(rows, fmt.Sprintf(
			"worker lifecycle policy artifact summary: workers=%d stale_workers=%d stale_leases=%d",
			summary.WorkerRegistrations,
			summary.StaleWorkerRegistrations,
			summary.StaleActiveLeases,
		))
		rows = append(rows, "worker lifecycle policy artifact path: "+filepath.Base(policy.Path))
	}
	return rows
}

func tuiModelWithWorkerLifecyclePolicyRunRows(root string, model tui.Model) tui.Model {
	runs, err := ListWorkerLifecyclePolicyRuns(root)
	if err != nil || len(runs) == 0 {
		return model
	}
	return model.AppendRuntimeRows(tui.RuntimeTabAgentRuntime, workerLifecyclePolicyRunRuntimeRows(runs))
}

func workerLifecyclePolicyRunRuntimeRows(runs []WorkerLifecyclePolicyRun) []string {
	if len(runs) == 0 {
		return nil
	}
	rows := []string{fmt.Sprintf("worker lifecycle policy runs: total=%d", len(runs))}
	for _, run := range runs {
		status := installRequestDisplayValue(run.Status, "unknown")
		mode := installRequestDisplayValue(run.Mode, "metadata-only")
		transport := installRequestDisplayValue(run.TransportMode, "metadata-only")
		policyPath := installRequestDisplayValue(run.PolicyPath, "none")
		selectionPath := installRequestDisplayValue(run.SelectionPath, "none")
		skipReason := installRequestDisplayValue(compactRuntimeRowValue(run.SkipReason), "none")
		supervisorCreated := 0
		supervisorRef := "none"
		if strings.TrimSpace(run.SupervisorPath) != "" {
			supervisorCreated = 1
			supervisorRef = filepath.Base(run.SupervisorPath)
		}
		rows = append(rows, fmt.Sprintf(
			"worker lifecycle policy run artifact: %s artifact_b64=%s status=%s mode=%s policy=%s policy_b64=%s selection=%s selection_b64=%s max_runs=%d poll_interval_ms=%d transport=%s include_lifecycle_maintenance=%t execute_lifecycle_retries=%t execute_lease_recovery=%t supervisor_created=%d skip=%s skip_b64=%s",
			run.Path,
			runtimeRowValueB64(run.Path),
			status,
			mode,
			policyPath,
			runtimeRowValueB64(run.PolicyPath),
			selectionPath,
			runtimeRowValueB64(run.SelectionPath),
			run.MaxRuns,
			run.PollIntervalMilliseconds,
			transport,
			run.IncludeLifecycleMaintenance,
			run.ExecuteLifecycleRetries,
			run.ExecuteLeaseRecovery,
			supervisorCreated,
			skipReason,
			runtimeRowValueB64(run.SkipReason),
		))
		rows = append(rows, fmt.Sprintf(
			"worker lifecycle policy run cadence: max_runs=%d poll_interval_ms=%d transport=%s",
			run.MaxRuns,
			run.PollIntervalMilliseconds,
			transport,
		))
		rows = append(rows, fmt.Sprintf(
			"worker lifecycle policy run gates: include_lifecycle_maintenance=%t execute_lifecycle_retries=%t execute_lease_recovery=%t supervisor_created=%d",
			run.IncludeLifecycleMaintenance,
			run.ExecuteLifecycleRetries,
			run.ExecuteLeaseRecovery,
			supervisorCreated,
		))
		if run.DispatchLifecycleRetryClaims || run.SupervisorSummary.RetryDispatchClaimsDispatched > 0 || run.SupervisorSummary.RetryDispatchTransportRequestsCreated > 0 {
			rows = append(rows, fmt.Sprintf(
				"worker lifecycle policy run retry handoff: dispatch_lifecycle_retry_claims=%t lifecycle_retry_transport=%s claims_dispatched=%d transport_requests=%d",
				run.DispatchLifecycleRetryClaims,
				installRequestDisplayValue(run.LifecycleRetryTransportMode, "metadata-only"),
				run.SupervisorSummary.RetryDispatchClaimsDispatched,
				run.SupervisorSummary.RetryDispatchTransportRequestsCreated,
			))
		}
		rows = append(rows, fmt.Sprintf(
			"worker lifecycle policy run refs: policy=%s selection=%s supervisor=%s",
			workerLifecyclePolicyRunBaseRef(run.PolicyPath),
			workerLifecyclePolicyRunBaseRef(run.SelectionPath),
			supervisorRef,
		))
		if strings.TrimSpace(run.SkipReason) != "" {
			rows = append(rows, "worker lifecycle policy run skip: "+compactRuntimeRowValue(run.SkipReason))
		}
		rows = append(rows, "worker lifecycle policy run path: "+filepath.Base(run.Path))
	}
	return rows
}

func workerLifecyclePolicyRunBaseRef(path string) string {
	if strings.TrimSpace(path) == "" {
		return "none"
	}
	return filepath.Base(path)
}

func tuiModelWithWorkerLifecyclePolicySchedulerRunRows(root string, model tui.Model) tui.Model {
	runs, err := ListWorkerLifecyclePolicySchedulerRuns(root)
	if err != nil || len(runs) == 0 {
		return model
	}
	return model.AppendRuntimeRows(tui.RuntimeTabAgentRuntime, workerLifecyclePolicySchedulerRunRuntimeRows(runs))
}

func workerLifecyclePolicySchedulerRunRuntimeRows(runs []WorkerLifecyclePolicySchedulerRun) []string {
	if len(runs) == 0 {
		return nil
	}
	rows := []string{fmt.Sprintf("worker lifecycle policy scheduler runs: total=%d", len(runs))}
	for _, run := range runs {
		payload := workerLifecyclePolicySchedulerRunPayload(run)
		path := installRequestDisplayValue(workerLifecyclePolicySchedulerRunValue(payload, "path"), "none")
		status := installRequestDisplayValue(workerLifecyclePolicySchedulerRunValue(payload, "status"), "unknown")
		mode := installRequestDisplayValue(workerLifecyclePolicySchedulerRunValue(payload, "mode"), "metadata-only")
		policyPath := installRequestDisplayValue(workerLifecyclePolicySchedulerRunValue(payload, "policy_path"), "none")
		selectionPath := installRequestDisplayValue(workerLifecyclePolicySchedulerRunValue(payload, "selection_path"), "none")
		policyBase := installRequestDisplayValue(workerLifecyclePolicySchedulerRunValue(payload, "policy_basename"), workerLifecyclePolicyRunBaseRef(policyPath))
		selectionBase := workerLifecyclePolicyRunBaseRef(selectionPath)
		maxRuns := workerLifecyclePolicySchedulerRunInt(payload, "max_runs_requested", "max_runs")
		pollInterval := workerLifecyclePolicySchedulerRunInt64(payload, "poll_interval_ms")
		transport := installRequestDisplayValue(workerLifecyclePolicySchedulerRunValue(payload, "transport"), "metadata-only")
		includeLifecycleMaintenance := installRequestDisplayValue(workerLifecyclePolicySchedulerRunValue(payload, "include_lifecycle_maintenance"), "false")
		executeLifecycleRetries := installRequestDisplayValue(workerLifecyclePolicySchedulerRunValue(payload, "execute_lifecycle_retries"), "false")
		executeLeaseRecovery := installRequestDisplayValue(workerLifecyclePolicySchedulerRunValue(payload, "execute_lease_recovery"), "false")
		policyRunsCreated := workerLifecyclePolicySchedulerRunInt(payload, "policy_runs_created")
		if policyRunsCreated == 0 {
			policyRunsCreated = workerLifecyclePolicySchedulerRunArrayLen(payload, "policy_run_paths")
		}
		supervisorCreated := installRequestDisplayValue(workerLifecyclePolicySchedulerRunValue(payload, "supervisor_created", "supervisors_created"), "0")
		skipRaw := workerLifecyclePolicySchedulerRunValue(payload, "skip_reason", "skip")
		errorRaw := workerLifecyclePolicySchedulerRunValue(payload, "error")
		skipReason := installRequestDisplayValue(compactRuntimeRowValue(skipRaw), "none")
		errorValue := installRequestDisplayValue(compactRuntimeRowValue(errorRaw), "none")
		rows = append(rows, fmt.Sprintf(
			"worker lifecycle policy scheduler run artifact: %s artifact_b64=%s status=%s mode=%s policy=%s policy_b64=%s selection=%s selection_b64=%s policy_basename=%s policy_basename_b64=%s selection_basename=%s selection_basename_b64=%s max_runs=%d poll_interval_ms=%d transport=%s include_lifecycle_maintenance=%s execute_lifecycle_retries=%s execute_lease_recovery=%s policy_runs_created=%d supervisor_created=%s skip=%s skip_b64=%s error=%s error_b64=%s",
			path,
			runtimeRowValueB64(path),
			status,
			mode,
			policyPath,
			runtimeRowValueB64(policyPath),
			selectionPath,
			runtimeRowValueB64(selectionPath),
			policyBase,
			runtimeRowValueB64(policyBase),
			selectionBase,
			runtimeRowValueB64(selectionBase),
			maxRuns,
			pollInterval,
			transport,
			includeLifecycleMaintenance,
			executeLifecycleRetries,
			executeLeaseRecovery,
			policyRunsCreated,
			supervisorCreated,
			skipReason,
			runtimeRowValueB64(skipRaw),
			errorValue,
			runtimeRowValueB64(errorRaw),
		))
		rows = append(rows, fmt.Sprintf(
			"worker lifecycle policy scheduler run compact: status=%s mode=%s max_runs=%d poll_interval_ms=%d policy=%s selection=%s policy_runs_created=%d supervisor_created=%s",
			status,
			mode,
			maxRuns,
			pollInterval,
			policyBase,
			selectionBase,
			policyRunsCreated,
			supervisorCreated,
		))
		rows = append(rows, fmt.Sprintf(
			"worker lifecycle policy scheduler run gates: include_lifecycle_maintenance=%s execute_lifecycle_retries=%s execute_lease_recovery=%s transport=%s",
			includeLifecycleMaintenance,
			executeLifecycleRetries,
			executeLeaseRecovery,
			transport,
		))
		dispatchLifecycleRetryClaims := installRequestDisplayValue(workerLifecyclePolicySchedulerRunValue(payload, "dispatch_lifecycle_retry_claims"), "false")
		lifecycleRetryTransport := installRequestDisplayValue(workerLifecyclePolicySchedulerRunValue(payload, "lifecycle_retry_transport_mode", "lifecycle_retry_transport"), "metadata-only")
		if dispatchLifecycleRetryClaims == "true" {
			rows = append(rows, fmt.Sprintf(
				"worker lifecycle policy scheduler run retry handoff: dispatch_lifecycle_retry_claims=%s lifecycle_retry_transport=%s",
				dispatchLifecycleRetryClaims,
				lifecycleRetryTransport,
			))
		}
		if strings.TrimSpace(workerLifecyclePolicySchedulerRunValue(payload, "skip_reason", "skip")) != "" {
			rows = append(rows, "worker lifecycle policy scheduler run skip: "+compactRuntimeRowValue(workerLifecyclePolicySchedulerRunValue(payload, "skip_reason", "skip")))
		}
		if strings.TrimSpace(workerLifecyclePolicySchedulerRunValue(payload, "error")) != "" {
			rows = append(rows, "worker lifecycle policy scheduler run error: "+compactRuntimeRowValue(workerLifecyclePolicySchedulerRunValue(payload, "error")))
		}
		rows = append(rows, "worker lifecycle policy scheduler run path: "+filepath.Base(path))
	}
	return rows
}

func workerLifecyclePolicySchedulerRunPayload(run WorkerLifecyclePolicySchedulerRun) map[string]any {
	data, err := json.Marshal(run)
	if err != nil {
		return nil
	}
	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil
	}
	return payload
}

func workerLifecyclePolicySchedulerRunValue(payload map[string]any, keys ...string) string {
	for _, key := range keys {
		if payload == nil {
			continue
		}
		value, ok := payload[key]
		if !ok {
			continue
		}
		rendered := workerLifecyclePolicySchedulerRunScalar(value)
		if strings.TrimSpace(rendered) != "" {
			return rendered
		}
	}
	return ""
}

func workerLifecyclePolicySchedulerRunScalar(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case bool:
		return strconv.FormatBool(typed)
	case float64:
		if typed == float64(int64(typed)) {
			return strconv.FormatInt(int64(typed), 10)
		}
		return strconv.FormatFloat(typed, 'f', -1, 64)
	case nil:
		return ""
	default:
		return strings.TrimSpace(fmt.Sprint(typed))
	}
}

func workerLifecyclePolicySchedulerRunInt(payload map[string]any, keys ...string) int {
	value, err := strconv.Atoi(workerLifecyclePolicySchedulerRunValue(payload, keys...))
	if err != nil {
		return 0
	}
	return value
}

func workerLifecyclePolicySchedulerRunInt64(payload map[string]any, keys ...string) int64 {
	value, err := strconv.ParseInt(workerLifecyclePolicySchedulerRunValue(payload, keys...), 10, 64)
	if err != nil {
		return 0
	}
	return value
}

func workerLifecyclePolicySchedulerRunArrayLen(payload map[string]any, key string) int {
	if payload == nil {
		return 0
	}
	values, ok := payload[key].([]any)
	if !ok {
		return 0
	}
	return len(values)
}

func tuiModelWithWorkerLifecycleActionPlanRows(root string, model tui.Model) tui.Model {
	rows := []string{}
	plans, err := ListWorkerLifecycleActionPlans(root)
	if err == nil {
		selection, selectionFound, _ := ReadWorkerLifecycleActionPlanSelection(root)
		rows = append(rows, workerLifecycleActionPlanArtifactRuntimeRowsWithSelection(plans, selection, selectionFound)...)
	}
	if len(rows) == 0 {
		return model
	}
	return model.AppendRuntimeRows(tui.RuntimeTabAgentRuntime, rows)
}

func tuiModelWithWorkerLifecycleMaintenanceRows(root string, model tui.Model) tui.Model {
	maintenances, err := ListWorkerLifecycleMaintenances(root)
	if err != nil || len(maintenances) == 0 {
		return model
	}
	return model.AppendRuntimeRows(tui.RuntimeTabAgentRuntime, workerLifecycleMaintenanceRuntimeRows(maintenances))
}

func workerLifecycleMaintenanceRuntimeRows(maintenances []WorkerLifecycleMaintenance) []string {
	rows := []string{fmt.Sprintf("worker lifecycle maintenance artifacts: total=%d", len(maintenances))}
	for _, maintenance := range maintenances {
		summary := maintenance.Summary
		rows = append(rows, fmt.Sprintf(
			"worker lifecycle maintenance artifact: %s status=%s mode=%s actions=%d ack=%d run=%d daemon_control=%d retry_dispatch_execute=%d reclaim=%d reconcile=%d skipped_unsafe=%d review_created=%d action_plan_created=%d reclaim_created=%d reconciliation_created=%d retry_dispatch_executions_created=%d retry_dispatch_claims_created=%d reclaim_executions_created=%d reclaim_results_created=%d reconciliation_executions_created=%d reconciliation_results_created=%d execute_lease_recovery=%t review=%s action_plan=%s reclaim=%s reconciliation=%s reclaim_execution=%s reconciliation_execution=%s review_path=%s action_plan_path=%s reclaim_path=%s reconciliation_path=%s",
			maintenance.Path,
			installRequestDisplayValue(maintenance.Status, "unknown"),
			installRequestDisplayValue(maintenance.Mode, "unknown"),
			summary.ActionsTotal,
			summary.AckActions,
			summary.RunActions,
			summary.DaemonControlActions,
			summary.RetryDispatchExecuteActions,
			summary.ReclaimActions,
			summary.ReconciliationActions,
			summary.SkippedUnsafeActions,
			summary.ReviewArtifactsCreated,
			summary.ActionPlanArtifactsCreated,
			summary.ReclaimDispatchArtifactsCreated,
			summary.ReconciliationDispatchArtifactsCreated,
			summary.RetryDispatchExecutionsCreated,
			summary.RetryDispatchClaimsCreated,
			summary.ReclaimExecutionsCreated,
			summary.ReclaimResultsCreated,
			summary.ReconciliationExecutionsCreated,
			summary.ReconciliationResultsCreated,
			maintenance.ExecuteLeaseRecovery,
			installRequestDisplayValue(maintenance.ReviewPath, "none"),
			installRequestDisplayValue(maintenance.ActionPlanPath, "none"),
			installRequestDisplayValue(maintenance.ReclaimDispatchPath, "none"),
			installRequestDisplayValue(maintenance.ReconciliationDispatchPath, "none"),
			installRequestDisplayValue(maintenance.ReclaimExecutionPath, "none"),
			installRequestDisplayValue(maintenance.ReconciliationExecutionPath, "none"),
			installRequestDisplayValue(maintenance.ReviewPath, "none"),
			installRequestDisplayValue(maintenance.ActionPlanPath, "none"),
			installRequestDisplayValue(maintenance.ReclaimDispatchPath, "none"),
			installRequestDisplayValue(maintenance.ReconciliationDispatchPath, "none"),
		))
		rows = append(rows, fmt.Sprintf(
			"worker lifecycle maintenance artifact state: %s mode=%s actions=%d",
			installRequestDisplayValue(maintenance.Status, "unknown"),
			installRequestDisplayValue(maintenance.Mode, "unknown"),
			summary.ActionsTotal,
		))
		rows = append(rows, fmt.Sprintf(
			"worker lifecycle maintenance artifact counts: ack=%d run=%d daemon_control=%d retry_dispatch_execute=%d reclaim=%d reconcile=%d skipped_unsafe=%d",
			summary.AckActions,
			summary.RunActions,
			summary.DaemonControlActions,
			summary.RetryDispatchExecuteActions,
			summary.ReclaimActions,
			summary.ReconciliationActions,
			summary.SkippedUnsafeActions,
		))
		if summary.RetryDispatchExecuteActions > 0 {
			rows = append(rows, fmt.Sprintf(
				"worker lifecycle maintenance retry dispatch executions: ready=%d",
				summary.RetryDispatchExecuteActions,
			))
		}
		if summary.RetryDispatchExecutionsCreated > 0 || summary.RetryDispatchClaimsCreated > 0 {
			rows = append(rows, fmt.Sprintf(
				"worker lifecycle maintenance retry dispatch executed: executions=%d claims=%d",
				summary.RetryDispatchExecutionsCreated,
				summary.RetryDispatchClaimsCreated,
			))
		}
		if maintenance.DispatchRetryClaims || summary.RetryDispatchClaimsDispatched > 0 || summary.RetryDispatchTransportRequestsCreated > 0 {
			rows = append(rows, fmt.Sprintf(
				"worker lifecycle maintenance retry handoff: dispatch_lifecycle_retry_claims=%t lifecycle_retry_transport=%s claims_dispatched=%d transport_requests=%d",
				maintenance.DispatchRetryClaims,
				installRequestDisplayValue(maintenance.RetryDispatchTransportMode, "metadata-only"),
				summary.RetryDispatchClaimsDispatched,
				summary.RetryDispatchTransportRequestsCreated,
			))
		}
		if summary.ReclaimExecutionsCreated > 0 || summary.ReconciliationExecutionsCreated > 0 {
			rows = append(rows, fmt.Sprintf(
				"worker lifecycle maintenance lease recovery executed: reclaim_executions=%d reclaim_results=%d reconciliation_executions=%d reconciliation_results=%d",
				summary.ReclaimExecutionsCreated,
				summary.ReclaimResultsCreated,
				summary.ReconciliationExecutionsCreated,
				summary.ReconciliationResultsCreated,
			))
		}
		rows = append(rows, fmt.Sprintf(
			"worker lifecycle maintenance counts: ack=%d run=%d daemon=%d reclaim=%d reconcile=%d skipped=%d",
			summary.AckActions,
			summary.RunActions,
			summary.DaemonControlActions,
			summary.ReclaimActions,
			summary.ReconciliationActions,
			summary.SkippedUnsafeActions,
		))
		rows = append(rows, fmt.Sprintf(
			"worker lifecycle maintenance artifact created: review=%d action_plan=%d reclaim=%d reconciliation=%d",
			summary.ReviewArtifactsCreated,
			summary.ActionPlanArtifactsCreated,
			summary.ReclaimDispatchArtifactsCreated,
			summary.ReconciliationDispatchArtifactsCreated,
		))
		rows = append(rows, fmt.Sprintf(
			"worker lifecycle maintenance created: review=%d action=%d reclaim=%d reconcile=%d",
			summary.ReviewArtifactsCreated,
			summary.ActionPlanArtifactsCreated,
			summary.ReclaimDispatchArtifactsCreated,
			summary.ReconciliationDispatchArtifactsCreated,
		))
		rows = append(rows, "worker lifecycle maintenance artifact path: "+filepath.Base(maintenance.Path))
		for _, path := range maintenance.RetryDispatchExecutionPaths {
			rows = append(rows, "worker lifecycle maintenance retry dispatch execution path: "+filepath.Base(path))
		}
		if strings.TrimSpace(maintenance.ReclaimExecutionPath) != "" {
			rows = append(rows, "worker lifecycle maintenance reclaim execution path: "+filepath.Base(maintenance.ReclaimExecutionPath))
		}
		if strings.TrimSpace(maintenance.ReconciliationExecutionPath) != "" {
			rows = append(rows, "worker lifecycle maintenance reconciliation execution path: "+filepath.Base(maintenance.ReconciliationExecutionPath))
		}
		for _, action := range maintenance.SkippedActions {
			rows = append(rows, fmt.Sprintf(
				"lifecycle maintenance skip: %s %s",
				installRequestDisplayValue(action.Type, "unknown"),
				installRequestDisplayValue(action.Gate, "unknown"),
			))
			if strings.TrimSpace(action.Type) == "execute-dispatch-retry-dispatch" {
				rows = append(rows, fmt.Sprintf(
					"retry dispatch maintenance skip: gate=%s",
					installRequestDisplayValue(action.Gate, "unknown"),
				))
			}
		}
		for _, action := range maintenance.SkippedActions {
			rows = append(rows, fmt.Sprintf(
				"worker lifecycle maintenance skip: type=%s gate=%s worker=%s run=%s execution=%s reason=%s request_path=%s lease_path=%s result_path=%s dispatch=%s",
				installRequestDisplayValue(action.Type, "unknown"),
				installRequestDisplayValue(action.Gate, "unknown"),
				installRequestDisplayValue(action.WorkerID, "unknown"),
				installRequestDisplayValue(action.RunID, "unknown"),
				installRequestDisplayValue(action.ExecutionID, "unknown"),
				installRequestDisplayValue(action.Reason, "none"),
				installRequestDisplayValue(action.RequestPath, "none"),
				installRequestDisplayValue(action.LeasePath, "none"),
				installRequestDisplayValue(action.ResultPath, "none"),
				installRequestDisplayValue(action.DispatchPath, "none"),
			))
		}
	}
	return rows
}

func tuiModelWithWorkerLifecycleMaintenanceSupervisorRows(root string, model tui.Model) tui.Model {
	supervisors, err := ListWorkerLifecycleMaintenanceSupervisors(root)
	if err != nil || len(supervisors) == 0 {
		return model
	}
	return model.AppendRuntimeRows(tui.RuntimeTabAgentRuntime, workerLifecycleMaintenanceSupervisorRuntimeRows(supervisors))
}

func workerLifecycleMaintenanceSupervisorRuntimeRows(supervisors []WorkerLifecycleMaintenanceSupervisor) []string {
	if len(supervisors) == 0 {
		return nil
	}
	completed := 0
	dryRun := 0
	failed := 0
	for _, supervisor := range supervisors {
		switch supervisor.Status {
		case "completed":
			completed++
		case "failed":
			failed++
		default:
			if supervisor.Status == "dry-run" {
				dryRun++
			}
		}
	}
	rows := []string{fmt.Sprintf(
		"worker lifecycle maintenance supervisors: total=%d completed=%d dry-run=%d failed=%d",
		len(supervisors),
		completed,
		dryRun,
		failed,
	)}
	for _, supervisor := range supervisors {
		summary := supervisor.Summary
		maintenancePath := "none"
		if len(supervisor.MaintenancePaths) > 0 {
			maintenancePath = supervisor.MaintenancePaths[0]
		}
		auditPath := "none"
		if len(supervisor.AuditPaths) > 0 {
			auditPath = supervisor.AuditPaths[0]
		}
		rows = append(rows, fmt.Sprintf(
			"worker lifecycle maintenance supervisor artifact: %s status=%s mode=%s runs=%d max_runs=%d actions=%d ack=%d run=%d daemon_control=%d retry_dispatch_execute=%d reclaim=%d reconcile=%d skipped_unsafe=%d maintenance_created=%d audit_created=%d retry_dispatch_executions_created=%d retry_dispatch_claims_created=%d reclaim_executions_created=%d reclaim_results_created=%d reconciliation_executions_created=%d reconciliation_results_created=%d execute_lease_recovery=%t maintenance_path=%s audit_path=%s",
			supervisor.Path,
			installRequestDisplayValue(supervisor.Status, "unknown"),
			installRequestDisplayValue(supervisor.Mode, "unknown"),
			summary.Runs,
			summary.MaxRuns,
			summary.ActionsTotal,
			summary.AckActions,
			summary.RunActions,
			summary.DaemonControlActions,
			summary.RetryDispatchExecuteActions,
			summary.ReclaimActions,
			summary.ReconciliationActions,
			summary.SkippedUnsafeActions,
			summary.MaintenanceArtifactsCreated,
			summary.AuditArtifactsCreated,
			summary.RetryDispatchExecutionsCreated,
			summary.RetryDispatchClaimsCreated,
			summary.ReclaimExecutionsCreated,
			summary.ReclaimResultsCreated,
			summary.ReconciliationExecutionsCreated,
			summary.ReconciliationResultsCreated,
			supervisor.ExecuteLeaseRecovery,
			installRequestDisplayValue(maintenancePath, "none"),
			installRequestDisplayValue(auditPath, "none"),
		))
		rows = append(rows, fmt.Sprintf(
			"worker lifecycle maintenance supervisor state: %s mode=%s runs=%d max_runs=%d",
			installRequestDisplayValue(supervisor.Status, "unknown"),
			installRequestDisplayValue(supervisor.Mode, "unknown"),
			summary.Runs,
			summary.MaxRuns,
		))
		if summary.ActionsTotal > 0 || summary.SkippedUnsafeActions > 0 {
			rows = append(rows, fmt.Sprintf(
				"worker lifecycle maintenance supervisor action totals: actions=%d skipped=%d retry_dispatch_execute=%d",
				summary.ActionsTotal,
				summary.SkippedUnsafeActions,
				summary.RetryDispatchExecuteActions,
			))
		}
		if summary.RetryDispatchExecuteActions > 0 {
			rows = append(rows, fmt.Sprintf(
				"maintenance supervisor retry dispatch: execute=%d skipped=%d",
				summary.RetryDispatchExecuteActions,
				summary.SkippedUnsafeActions,
			))
		}
		if summary.RetryDispatchExecutionsCreated > 0 || summary.RetryDispatchClaimsCreated > 0 {
			rows = append(rows, fmt.Sprintf(
				"maintenance supervisor retry dispatch executed: executions=%d claims=%d",
				summary.RetryDispatchExecutionsCreated,
				summary.RetryDispatchClaimsCreated,
			))
		}
		if supervisor.DispatchRetryClaims || summary.RetryDispatchClaimsDispatched > 0 || summary.RetryDispatchTransportRequestsCreated > 0 {
			rows = append(rows, fmt.Sprintf(
				"maintenance supervisor retry handoff: dispatch_lifecycle_retry_claims=%t lifecycle_retry_transport=%s claims_dispatched=%d transport_requests=%d",
				supervisor.DispatchRetryClaims,
				installRequestDisplayValue(supervisor.RetryDispatchTransportMode, "metadata-only"),
				summary.RetryDispatchClaimsDispatched,
				summary.RetryDispatchTransportRequestsCreated,
			))
		}
		if summary.ReclaimExecutionsCreated > 0 || summary.ReconciliationExecutionsCreated > 0 {
			rows = append(rows, fmt.Sprintf(
				"maintenance supervisor lease recovery executed: reclaim_executions=%d reclaim_results=%d reconciliation_executions=%d reconciliation_results=%d",
				summary.ReclaimExecutionsCreated,
				summary.ReclaimResultsCreated,
				summary.ReconciliationExecutionsCreated,
				summary.ReconciliationResultsCreated,
			))
		}
		rows = append(rows, fmt.Sprintf(
			"worker lifecycle maintenance supervisor created: maintenance=%d audit=%d",
			summary.MaintenanceArtifactsCreated,
			summary.AuditArtifactsCreated,
		))
		for _, path := range supervisor.RetryDispatchExecutionPaths {
			rows = append(rows, "worker lifecycle maintenance supervisor retry-dispatch execution: "+path)
		}
		for _, path := range supervisor.ReclaimExecutionPaths {
			rows = append(rows, "worker lifecycle maintenance supervisor reclaim execution: "+path)
		}
		for _, path := range supervisor.ReconciliationExecutionPaths {
			rows = append(rows, "worker lifecycle maintenance supervisor reconciliation execution: "+path)
		}
		rows = append(rows, "worker lifecycle maintenance supervisor path: "+filepath.Base(supervisor.Path))
		if strings.TrimSpace(supervisor.Error) != "" {
			rows = append(rows, "worker lifecycle maintenance supervisor error: "+compactRuntimeRowValue(supervisor.Error))
		}
	}
	return rows
}

func workerLifecycleActionPlanArtifactRuntimeRows(plans []WorkerLifecycleActionPlan) []string {
	return workerLifecycleActionPlanArtifactRuntimeRowsWithSelection(plans, WorkerLifecycleActionPlanSelection{}, false)
}

func workerLifecycleActionPlanArtifactRuntimeRowsWithSelection(plans []WorkerLifecycleActionPlan, selection WorkerLifecycleActionPlanSelection, selectionFound bool) []string {
	if len(plans) == 0 {
		return nil
	}
	annotated := 0
	current := 0
	for _, plan := range plans {
		if plan.Annotation != nil {
			annotated++
		}
		if selectionFound && strings.TrimSpace(selection.Status) == "current" && strings.TrimSpace(selection.PlanPath) == strings.TrimSpace(plan.Path) {
			current++
		}
	}
	rows := []string{fmt.Sprintf("worker lifecycle action plan artifacts: total=%d annotated=%d current=%d", len(plans), annotated, current)}
	if selectionFound {
		actorFields := fmt.Sprintf(
			"selected_by=%s selected_by_b64=%s selected_at=%s selected_at_b64=%s",
			reportProjectionDispatchRowValue(selection.SelectedBy, "unknown"),
			runtimeRowValueB64(selection.SelectedBy),
			reportProjectionDispatchRowValue(selection.SelectedAt, "unknown"),
			runtimeRowValueB64(selection.SelectedAt),
		)
		if strings.TrimSpace(selection.Status) == "cleared" {
			actorFields = fmt.Sprintf(
				"cleared_by=%s cleared_by_b64=%s cleared_at=%s cleared_at_b64=%s",
				reportProjectionDispatchRowValue(selection.ClearedBy, "unknown"),
				runtimeRowValueB64(selection.ClearedBy),
				reportProjectionDispatchRowValue(selection.ClearedAt, "unknown"),
				runtimeRowValueB64(selection.ClearedAt),
			)
		}
		isCurrent := strings.TrimSpace(selection.Status) == "current"
		summary := selection.Summary
		rows = append(rows, fmt.Sprintf(
			"worker lifecycle action plan selection: status=%s mode=%s current=%t %s reason=%s reason_b64=%s plan=%s plan_b64=%s selection=%s selection_b64=%s review=%s review_b64=%s actions=%d ack=%d run=%d daemon_control=%d retry_dispatch_execute=%d reclaim=%d reconcile=%d superseded=%d",
			reportProjectionDispatchRowValue(selection.Status, "unknown"),
			reportProjectionDispatchRowValue(selection.Mode, "metadata-only"),
			isCurrent,
			actorFields,
			reportProjectionDispatchRowValue(selection.Reason, "none"),
			runtimeRowValueB64(selection.Reason),
			reportProjectionDispatchRowValue(selection.PlanPath, "unknown"),
			runtimeRowValueB64(selection.PlanPath),
			reportProjectionDispatchRowValue(selection.Path, "unknown"),
			runtimeRowValueB64(selection.Path),
			reportProjectionDispatchRowValue(selection.ReviewPath, "none"),
			runtimeRowValueB64(selection.ReviewPath),
			summary.Total,
			summary.AckPendingTransport,
			summary.RunAcknowledgedTransport,
			summary.ApplyDaemonControl,
			summary.ExecuteDispatchRetryDispatch,
			summary.ReclaimStaleLease,
			summary.ReconcileResultLease,
			len(selection.SupersededPlanPaths),
		))
		for _, path := range selection.SupersededPlanPaths {
			if trimmed := strings.TrimSpace(path); trimmed != "" {
				rows = append(rows, "worker lifecycle action plan superseded: "+trimmed)
			}
		}
	}
	for _, plan := range plans {
		summary := plan.Summary
		reviewPath := installRequestDisplayValue(plan.ReviewPath, "none")
		decision := "unreviewed"
		reviewer := "unknown"
		reviewerRaw := ""
		reviewedAt := "unknown"
		reviewedAtRaw := ""
		reviewReason := "none"
		reviewReasonRaw := ""
		if plan.Annotation != nil {
			decision = reportProjectionDispatchRowValue(plan.Annotation.Decision, "unreviewed")
			reviewer = reportProjectionDispatchRowValue(plan.Annotation.Reviewer, "unknown")
			reviewerRaw = plan.Annotation.Reviewer
			reviewedAt = reportProjectionDispatchRowValue(plan.Annotation.ReviewedAt, "unknown")
			reviewedAtRaw = plan.Annotation.ReviewedAt
			reviewReason = reportProjectionDispatchRowValue(plan.Annotation.Reason, "none")
			reviewReasonRaw = plan.Annotation.Reason
		}
		currentValue := "false"
		if selectionFound && strings.TrimSpace(selection.Status) == "current" && strings.TrimSpace(selection.PlanPath) == strings.TrimSpace(plan.Path) {
			currentValue = "true"
		}
		rows = append(rows, fmt.Sprintf(
			"worker lifecycle action plan artifact status=%s mode=%s",
			installRequestDisplayValue(plan.Status, "unknown"),
			installRequestDisplayValue(plan.Mode, "unknown"),
		))
		rows = append(rows, fmt.Sprintf(
			"worker lifecycle action plan artifact: %s artifact_b64=%s status=%s mode=%s actions=%d ack=%d run=%d reclaim=%d reconcile=%d daemon_control=%d retry_dispatch_execute=%d review=%s review_b64=%s decision=%s reviewer=%s reviewer_b64=%s reviewed_at=%s reviewed_at_b64=%s review_reason=%s review_reason_b64=%s current=%s",
			plan.Path,
			runtimeRowValueB64(plan.Path),
			installRequestDisplayValue(plan.Status, "unknown"),
			installRequestDisplayValue(plan.Mode, "unknown"),
			summary.Total,
			summary.AckPendingTransport,
			summary.RunAcknowledgedTransport,
			summary.ReclaimStaleLease,
			summary.ReconcileResultLease,
			summary.ApplyDaemonControl,
			summary.ExecuteDispatchRetryDispatch,
			reviewPath,
			runtimeRowValueB64(plan.ReviewPath),
			decision,
			reviewer,
			runtimeRowValueB64(reviewerRaw),
			reviewedAt,
			runtimeRowValueB64(reviewedAtRaw),
			reviewReason,
			runtimeRowValueB64(reviewReasonRaw),
			currentValue,
		))
		rows = append(rows, fmt.Sprintf(
			"worker lifecycle action plan artifact state: %s mode=%s actions=%d",
			installRequestDisplayValue(plan.Status, "unknown"),
			installRequestDisplayValue(plan.Mode, "unknown"),
			summary.Total,
		))
		rows = append(rows, fmt.Sprintf(
			"worker lifecycle action plan artifact review: current=%s decision=%s reviewer=%s reviewed_at=%s review_reason=%s review_reason_b64=%s",
			currentValue,
			decision,
			reviewer,
			reviewedAt,
			reviewReason,
			runtimeRowValueB64(reviewReasonRaw),
		))
		rows = append(rows, fmt.Sprintf(
			"worker lifecycle action plan artifact counts: ack=%d run=%d reclaim=%d reconcile=%d daemon_control=%d retry_dispatch_execute=%d",
			summary.AckPendingTransport,
			summary.RunAcknowledgedTransport,
			summary.ReclaimStaleLease,
			summary.ReconcileResultLease,
			summary.ApplyDaemonControl,
			summary.ExecuteDispatchRetryDispatch,
		))
		if summary.ApplyDaemonControl > 0 {
			rows = append(rows, fmt.Sprintf("worker lifecycle action plan daemon controls: ready=%d", summary.ApplyDaemonControl))
		}
		if summary.ExecuteDispatchRetryDispatch > 0 {
			rows = append(rows, fmt.Sprintf("worker lifecycle action plan retry dispatch executions: ready=%d", summary.ExecuteDispatchRetryDispatch))
		}
		if plan.Annotation != nil {
			rows = append(rows, fmt.Sprintf(
				"worker lifecycle action plan review: mode=metadata-only decision=%s reviewer=%s reviewer_b64=%s reviewed_at=%s reviewed_at_b64=%s review_reason=%s review_reason_b64=%s plan=%s plan_b64=%s review=%s review_b64=%s",
				decision,
				reviewer,
				runtimeRowValueB64(plan.Annotation.Reviewer),
				reviewedAt,
				runtimeRowValueB64(plan.Annotation.ReviewedAt),
				reviewReason,
				runtimeRowValueB64(reviewReasonRaw),
				reportProjectionDispatchRowValue(plan.Path, "unknown"),
				runtimeRowValueB64(plan.Path),
				reportProjectionDispatchRowValue(plan.Path, "unknown"),
				runtimeRowValueB64(plan.Path),
			))
			rows = append(rows, fmt.Sprintf(
				"worker lifecycle action plan annotation: decision=%s reviewer=%s reviewed_at=%s reason=%s",
				decision,
				reviewer,
				reviewedAt,
				reviewReason,
			))
		}
		rows = append(rows, "worker lifecycle action plan artifact path: "+filepath.Base(plan.Path))
		for _, action := range plan.Actions {
			rows = append(rows, fmt.Sprintf(
				"worker lifecycle action: type=%s gate=%s worker=%s worker_b64=%s run=%s run_b64=%s execution=%s execution_b64=%s status=%s mode=%s reason=%s reason_b64=%s request_path=%s lease_path=%s result_path=%s dispatch=%s recommended=%s",
				installRequestDisplayValue(action.Type, "unknown"),
				installRequestDisplayValue(action.Gate, "unknown"),
				installRequestDisplayValue(action.WorkerID, "unknown"),
				runtimeRowValueB64(action.WorkerID),
				installRequestDisplayValue(action.RunID, "unknown"),
				runtimeRowValueB64(action.RunID),
				installRequestDisplayValue(action.ExecutionID, "unknown"),
				runtimeRowValueB64(action.ExecutionID),
				installRequestDisplayValue(action.Status, "unknown"),
				installRequestDisplayValue(action.Mode, "metadata-only"),
				installRequestDisplayValue(action.Reason, "none"),
				runtimeRowValueB64(action.Reason),
				installRequestDisplayValue(action.RequestPath, "none"),
				installRequestDisplayValue(action.LeasePath, "none"),
				installRequestDisplayValue(action.ResultPath, "none"),
				installRequestDisplayValue(action.DispatchPath, "none"),
				installRequestDisplayValue(action.RecommendedStatus, "none"),
			))
			if strings.TrimSpace(action.Type) == "apply-daemon-control" {
				rows = append(rows, fmt.Sprintf(
					"daemon control action: %s %s %s enable-control",
					installRequestDisplayValue(action.WorkerID, "unknown"),
					installRequestDisplayValue(action.RecommendedStatus, "unknown"),
					installRequestDisplayValue(action.Status, "ready"),
				))
			}
			if strings.TrimSpace(action.Type) == "execute-dispatch-retry-dispatch" {
				rows = append(rows, fmt.Sprintf(
					"retry dispatch action: %s %s gate=%s artifact=%s",
					installRequestDisplayValue(action.RecommendedStatus, "execute"),
					installRequestDisplayValue(action.Status, "ready"),
					installRequestDisplayValue(action.Gate, "unknown"),
					installRequestDisplayValue(filepath.Base(action.DispatchPath), "none"),
				))
			}
		}
	}
	return rows
}

func workerLifecycleActionPlanRuntimeRowPrefixes() []string {
	return []string{
		"worker lifecycle action plan artifacts:",
		"worker lifecycle action plan selection:",
		"worker lifecycle action plan superseded:",
		"worker lifecycle action plan artifact status=",
		"worker lifecycle action plan artifact:",
		"worker lifecycle action plan artifact state:",
		"worker lifecycle action plan artifact review:",
		"worker lifecycle action plan artifact counts:",
		"worker lifecycle action plan daemon controls:",
		"worker lifecycle action plan retry dispatch executions:",
		"worker lifecycle action plan review:",
		"worker lifecycle action plan annotation:",
		"worker lifecycle action plan artifact path:",
		"worker lifecycle action:",
		"daemon control action:",
		"retry dispatch action:",
	}
}

func tuiModelWithWorkerLeaseRows(root string, model tui.Model) tui.Model {
	leases, err := ListWorkerLeasesWithStaleness(root, time.Now().UTC(), defaultWorkerLeaseStaleAfter)
	if err != nil {
		return model
	}
	dispatches, _ := ListWorkerLeaseReclaimDispatches(root)
	executions, _ := ListWorkerLeaseReclaimExecutions(root)
	reconciliation, _ := PlanWorkerLeaseResultReconciliation(root)
	if len(leases) == 0 && len(dispatches) == 0 && len(executions) == 0 && len(reconciliation.Skipped) == 0 {
		return model
	}
	rows := []string{}
	if len(reconciliation.Skipped) > 0 {
		rows = append(rows, workerLeaseResultReconciliationRuntimeRows(WorkerLeaseResultReconciliationPlan{Skipped: reconciliation.Skipped})...)
	}
	if len(leases) > 0 {
		rows = append(rows, workerLeaseRuntimeRows(leases)...)
	}
	rows = append(rows, workerLeaseReclaimExecutionRuntimeRows(executions)...)
	rows = append(rows, workerLeaseReclaimDispatchRuntimeRows(dispatches)...)
	return model.AppendRuntimeRows(tui.RuntimeTabAgentRuntime, rows)
}

func tuiModelWithWorkerLeaseResultReconciliationRows(root string, model tui.Model) tui.Model {
	rows := []string{}
	reconciliation, err := PlanWorkerLeaseResultReconciliation(root)
	if err == nil && len(reconciliation.Ready) > 0 {
		rows = append(rows, workerLeaseResultReconciliationRuntimeRows(WorkerLeaseResultReconciliationPlan{Ready: reconciliation.Ready})...)
	}
	dispatches, err := ListWorkerLeaseResultReconciliationDispatches(root)
	if err == nil {
		rows = append(rows, workerLeaseResultReconciliationDispatchRuntimeRows(dispatches)...)
	}
	executions, err := ListWorkerLeaseResultReconciliationExecutions(root)
	if err == nil {
		rows = append(rows, workerLeaseResultReconciliationExecutionRuntimeRows(executions)...)
	}
	if len(rows) == 0 {
		return model
	}
	return model.AppendRuntimeRows(tui.RuntimeTabAgentRuntime, rows)
}

func workerLeaseResultReconciliationRuntimeRows(plan WorkerLeaseResultReconciliationPlan) []string {
	if len(plan.Ready) == 0 && len(plan.Skipped) == 0 {
		return nil
	}
	rows := []string{fmt.Sprintf("worker lease result reconciliation: ready=%d skipped=%d", len(plan.Ready), len(plan.Skipped))}
	for _, candidate := range plan.Ready {
		rows = append(rows, fmt.Sprintf(
			"worker lease result reconcile run=%s execution=%s result=%s lease=%s recommended=%s",
			installRequestDisplayValue(candidate.Result.RunID, "unknown"),
			installRequestDisplayValue(candidate.Result.ExecutionID, "unknown"),
			installRequestDisplayValue(candidate.Result.Status, "unknown"),
			installRequestDisplayValue(candidate.Lease.Status, "unknown"),
			installRequestDisplayValue(candidate.RecommendedStatus, "unknown"),
		))
		rows = append(rows, "worker lease result reconcile result="+installRequestDisplayValue(candidate.Result.Status, "unknown"))
		rows = append(rows, "worker lease result reconcile lease="+installRequestDisplayValue(candidate.Lease.Status, "unknown"))
		rows = append(rows, "worker lease result reconcile recommended="+installRequestDisplayValue(candidate.RecommendedStatus, "unknown"))
		rows = append(rows, "worker lease result reconcile reason="+installRequestDisplayValue(candidate.Reason, "unknown"))
	}
	for _, skipped := range plan.Skipped {
		rows = append(rows, fmt.Sprintf(
			"worker lease result reconcile skip run=%s execution=%s result=%s lease=%s reason=%s",
			installRequestDisplayValue(skipped.Result.RunID, "unknown"),
			installRequestDisplayValue(skipped.Result.ExecutionID, "unknown"),
			installRequestDisplayValue(skipped.Result.Status, "unknown"),
			installRequestDisplayValue(skipped.Lease.Status, "missing"),
			installRequestDisplayValue(skipped.Reason, "unknown"),
		))
	}
	return rows
}

func workerLeaseResultReconciliationDispatchRuntimeRows(dispatches []WorkerLeaseResultReconciliationDispatch) []string {
	if len(dispatches) == 0 {
		return nil
	}
	rows := []string{fmt.Sprintf("worker lease result reconciliation artifacts: total=%d", len(dispatches))}
	for _, dispatch := range dispatches {
		rows = append(rows, fmt.Sprintf(
			"worker lease result reconciliation artifact status=%s mode=%s ready=%d skipped=%d",
			installRequestDisplayValue(dispatch.Status, "unknown"),
			installRequestDisplayValue(dispatch.Mode, "unknown"),
			len(dispatch.Ready),
			len(dispatch.Skipped),
		))
		rows = append(rows, fmt.Sprintf(
			"worker lease result reconciliation artifact: %s artifact_b64=%s status=%s mode=%s ready=%d skipped=%d",
			compactRuntimeRowValue(dispatch.Path),
			runtimeRowValueB64(dispatch.Path),
			installRequestDisplayValue(dispatch.Status, "unknown"),
			installRequestDisplayValue(dispatch.Mode, "unknown"),
			len(dispatch.Ready),
			len(dispatch.Skipped),
		))
		rows = append(rows, "worker lease result reconciliation artifact path: "+filepath.Base(dispatch.Path))
		rows = append(rows, fmt.Sprintf(
			"lease result reconciliation artifact state: %s mode=%s ready=%d skipped=%d",
			installRequestDisplayValue(dispatch.Status, "unknown"),
			installRequestDisplayValue(dispatch.Mode, "unknown"),
			len(dispatch.Ready),
			len(dispatch.Skipped),
		))
	}
	return rows
}

func workerLeaseResultReconciliationExecutionRuntimeRows(executions []WorkerLeaseResultReconciliationExecution) []string {
	if len(executions) == 0 {
		return nil
	}
	rows := []string{fmt.Sprintf("worker lease result reconciliation execution artifacts: total=%d", len(executions))}
	for _, execution := range executions {
		rows = append(rows, fmt.Sprintf(
			"worker lease result reconciliation execution artifact status=%s mode=%s results=%d skipped=%d",
			installRequestDisplayValue(execution.Status, "unknown"),
			installRequestDisplayValue(execution.Mode, "unknown"),
			len(execution.Results),
			len(execution.Skipped),
		))
		rows = append(rows, fmt.Sprintf(
			"worker lease result reconciliation execution artifact: %s artifact_b64=%s status=%s mode=%s dispatch=%s dispatch_b64=%s results=%d skipped=%d",
			compactRuntimeRowValue(execution.Path),
			runtimeRowValueB64(execution.Path),
			installRequestDisplayValue(execution.Status, "unknown"),
			installRequestDisplayValue(execution.Mode, "unknown"),
			compactRuntimeRowValue(execution.DispatchPath),
			runtimeRowValueB64(execution.DispatchPath),
			len(execution.Results),
			len(execution.Skipped),
		))
		rows = append(rows, fmt.Sprintf(
			"lease result reconciliation execution state: %s mode=%s results=%d skipped=%d",
			installRequestDisplayValue(execution.Status, "unknown"),
			installRequestDisplayValue(execution.Mode, "unknown"),
			len(execution.Results),
			len(execution.Skipped),
		))
		for _, result := range execution.Results {
			rows = append(rows, fmt.Sprintf(
				"lease result reconciliation execution result run=%s run_b64=%s execution=%s execution_b64=%s result=%s lease=%s worker=%s worker_b64=%s lease_path=%s lease_path_b64=%s result_path=%s result_path_b64=%s dispatch=%s dispatch_b64=%s",
				compactRuntimeRowValue(installRequestDisplayValue(result.RunID, "unknown")),
				runtimeRowValueB64(result.RunID),
				compactRuntimeRowValue(installRequestDisplayValue(result.ExecutionID, "unknown")),
				runtimeRowValueB64(result.ExecutionID),
				installRequestDisplayValue(result.ResultStatus, "unknown"),
				installRequestDisplayValue(result.Status, "unknown"),
				compactRuntimeRowValue(installRequestDisplayValue(result.WorkerID, "unknown")),
				runtimeRowValueB64(result.WorkerID),
				compactRuntimeRowValue(result.LeasePath),
				runtimeRowValueB64(result.LeasePath),
				compactRuntimeRowValue(result.ResultPath),
				runtimeRowValueB64(result.ResultPath),
				compactRuntimeRowValue(result.DispatchPath),
				runtimeRowValueB64(result.DispatchPath),
			))
			rows = append(rows, fmt.Sprintf(
				"lease result reconciliation execution identity run=%s execution=%s worker=%s",
				installRequestDisplayValue(result.RunID, "unknown"),
				installRequestDisplayValue(result.ExecutionID, "unknown"),
				installRequestDisplayValue(result.WorkerID, "unknown"),
			))
			rows = append(rows, "lease result reconciliation execution result="+installRequestDisplayValue(result.ResultStatus, "unknown"))
			rows = append(rows, "lease result reconciliation execution lease="+installRequestDisplayValue(result.Status, "unknown"))
		}
		for _, skipped := range execution.Skipped {
			rows = append(rows, fmt.Sprintf(
				"lease result reconciliation execution skip run=%s execution=%s reason=%s",
				installRequestDisplayValue(skipped.Result.RunID, "unknown"),
				installRequestDisplayValue(skipped.Result.ExecutionID, "unknown"),
				installRequestDisplayValue(skipped.Reason, "unknown"),
			))
		}
	}
	return rows
}

func workerLeaseRuntimeRows(leases []WorkerLeaseArtifact) []string {
	active := 0
	stale := 0
	completed := 0
	failed := 0
	reclaimed := 0
	for _, lease := range leases {
		if lease.Stale {
			stale++
			continue
		}
		switch strings.TrimSpace(lease.Status) {
		case "active":
			active++
		case "failed":
			failed++
		case "completed":
			completed++
		case "reclaimed":
			reclaimed++
		}
	}
	rows := []string{fmt.Sprintf("worker leases: total=%d active=%d stale=%d completed=%d failed=%d reclaimed=%d", len(leases), active, stale, completed, failed, reclaimed)}
	for _, lease := range leases {
		status := installRequestDisplayValue(lease.Status, "unknown")
		if lease.Stale {
			status = "stale"
		}
		rows = append(rows, fmt.Sprintf(
			"worker lease %s %s %s worker=%s run=%s execution=%s",
			installRequestDisplayValue(lease.Tool, "unknown"),
			installRequestDisplayValue(lease.Capability, "unknown"),
			status,
			installRequestDisplayValue(lease.WorkerID, "unknown"),
			installRequestDisplayValue(lease.RunID, "unknown"),
			installRequestDisplayValue(lease.ExecutionID, "unknown"),
		))
		if strings.TrimSpace(lease.Path) != "" {
			rows = append(rows, "worker lease path: "+lease.Path)
		}
		heartbeatAt := strings.TrimSpace(lease.HeartbeatAt)
		heartbeatBy := strings.TrimSpace(lease.HeartbeatBy)
		heartbeatReason := strings.TrimSpace(lease.HeartbeatReason)
		if heartbeatAt != "" || heartbeatBy != "" || heartbeatReason != "" {
			rows = append(rows, fmt.Sprintf(
				"worker lease heartbeat: run=%s execution=%s at=%s by=%s reason=%s",
				installRequestDisplayValue(lease.RunID, "unknown"),
				installRequestDisplayValue(lease.ExecutionID, "unknown"),
				installRequestDisplayValue(heartbeatAt, "unknown"),
				installRequestDisplayValue(heartbeatBy, "unknown"),
				installRequestDisplayValue(heartbeatReason, "none"),
			))
			if heartbeatBy != "" {
				rows = append(rows, "worker lease heartbeat by="+heartbeatBy)
			}
			if heartbeatReason != "" {
				rows = append(rows, "worker lease heartbeat reason="+heartbeatReason)
			}
		}
		releasedAt := strings.TrimSpace(lease.ReleasedAt)
		releasedBy := strings.TrimSpace(lease.ReleasedBy)
		releaseReason := strings.TrimSpace(lease.ReleaseReason)
		if releasedAt != "" || releasedBy != "" || releaseReason != "" {
			rows = append(rows, fmt.Sprintf(
				"worker lease release: run=%s execution=%s at=%s by=%s reason=%s",
				installRequestDisplayValue(lease.RunID, "unknown"),
				installRequestDisplayValue(lease.ExecutionID, "unknown"),
				installRequestDisplayValue(releasedAt, "unknown"),
				installRequestDisplayValue(releasedBy, "unknown"),
				installRequestDisplayValue(releaseReason, "none"),
			))
			if releasedBy != "" {
				rows = append(rows, "worker lease release by="+releasedBy)
			}
			if releaseReason != "" {
				rows = append(rows, "worker lease release reason="+releaseReason)
			}
		}
		if strings.TrimSpace(lease.ResultExecutionID) != "" {
			rows = append(rows, "worker lease result="+strings.TrimSpace(lease.ResultExecutionID))
		}
		if strings.TrimSpace(lease.Error) != "" {
			rows = append(rows, "worker lease error: "+strings.TrimSpace(lease.Error))
		}
	}
	reclaimPlan := workerLeaseReclaimPlanFromLeases(leases)
	if len(reclaimPlan.Ready) > 0 {
		rows = append(rows, workerLeaseReclaimRuntimeRows(reclaimPlan)...)
	}
	return rows
}

func workerLeaseReclaimRuntimeRows(plan WorkerLeaseReclaimPlan) []string {
	rows := []string{fmt.Sprintf("worker lease reclaims: ready=%d skipped=%d", len(plan.Ready), len(plan.Skipped))}
	for _, candidate := range plan.Ready {
		lease := candidate.Lease
		rows = append(rows, fmt.Sprintf(
			"worker lease reclaim %s %s: ready mode=%s worker=%s run=%s execution=%s",
			installRequestDisplayValue(lease.Tool, "unknown"),
			installRequestDisplayValue(lease.Capability, "unknown"),
			installRequestDisplayValue(candidate.Mode, "metadata-only"),
			installRequestDisplayValue(lease.WorkerID, "unknown"),
			installRequestDisplayValue(lease.RunID, "unknown"),
			installRequestDisplayValue(lease.ExecutionID, "unknown"),
		))
		if strings.TrimSpace(lease.Path) != "" {
			rows = append(rows, "worker lease reclaim path: "+lease.Path)
		}
	}
	for _, skipped := range plan.Skipped {
		lease := skipped.Lease
		rows = append(rows, fmt.Sprintf(
			"worker lease reclaim skip %s %s: %s",
			installRequestDisplayValue(lease.Tool, "unknown"),
			installRequestDisplayValue(lease.Capability, "unknown"),
			installRequestDisplayValue(skipped.Reason, "unknown"),
		))
	}
	return rows
}

func workerLeaseReclaimDispatchRuntimeRows(dispatches []WorkerLeaseReclaimDispatch) []string {
	if len(dispatches) == 0 {
		return nil
	}
	rows := []string{fmt.Sprintf("worker lease reclaim artifacts: total=%d", len(dispatches))}
	for _, dispatch := range dispatches {
		rows = append(rows, fmt.Sprintf(
			"worker lease reclaim artifact: %s artifact_b64=%s status=%s mode=%s ready=%d skipped=%d",
			compactRuntimeRowValue(dispatch.Path),
			runtimeRowValueB64(dispatch.Path),
			installRequestDisplayValue(dispatch.Status, "unknown"),
			installRequestDisplayValue(dispatch.Mode, "unknown"),
			len(dispatch.Ready),
			len(dispatch.Skipped),
		))
		rows = append(rows, fmt.Sprintf(
			"worker lease reclaim artifact state: %s mode=%s ready=%d skipped=%d",
			installRequestDisplayValue(dispatch.Status, "unknown"),
			installRequestDisplayValue(dispatch.Mode, "unknown"),
			len(dispatch.Ready),
			len(dispatch.Skipped),
		))
		rows = append(rows, fmt.Sprintf(
			"reclaim artifact: %s %s ready=%d skipped=%d",
			installRequestDisplayValue(dispatch.Status, "unknown"),
			installRequestDisplayValue(dispatch.Mode, "unknown"),
			len(dispatch.Ready),
			len(dispatch.Skipped),
		))
	}
	return rows
}

func workerLeaseReclaimExecutionRuntimeRows(executions []WorkerLeaseReclaimExecution) []string {
	if len(executions) == 0 {
		return nil
	}
	rows := []string{fmt.Sprintf("worker lease reclaim execution artifacts: total=%d", len(executions))}
	for _, execution := range executions {
		rows = append(rows, fmt.Sprintf(
			"worker lease reclaim execution artifact: %s artifact_b64=%s status=%s mode=%s dispatch=%s dispatch_b64=%s results=%d skipped=%d",
			compactRuntimeRowValue(execution.Path),
			runtimeRowValueB64(execution.Path),
			installRequestDisplayValue(execution.Status, "unknown"),
			installRequestDisplayValue(execution.Mode, "unknown"),
			compactRuntimeRowValue(execution.DispatchPath),
			runtimeRowValueB64(execution.DispatchPath),
			len(execution.Results),
			len(execution.Skipped),
		))
		rows = append(rows, fmt.Sprintf(
			"reclaim execution: %s %s results=%d skipped=%d",
			installRequestDisplayValue(execution.Status, "unknown"),
			installRequestDisplayValue(execution.Mode, "unknown"),
			len(execution.Results),
			len(execution.Skipped),
		))
		for _, result := range execution.Results {
			rows = append(rows, fmt.Sprintf(
				"worker lease reclaim execution result run=%s run_b64=%s execution=%s execution_b64=%s status=%s worker=%s worker_b64=%s tool=%s capability=%s lease_path=%s lease_path_b64=%s",
				compactRuntimeRowValue(result.RunID),
				runtimeRowValueB64(result.RunID),
				compactRuntimeRowValue(result.ExecutionID),
				runtimeRowValueB64(result.ExecutionID),
				installRequestDisplayValue(result.Status, "unknown"),
				compactRuntimeRowValue(installRequestDisplayValue(result.WorkerID, "unknown")),
				runtimeRowValueB64(result.WorkerID),
				installRequestDisplayValue(result.Tool, "unknown"),
				installRequestDisplayValue(result.Capability, "unknown"),
				compactRuntimeRowValue(result.LeasePath),
				runtimeRowValueB64(result.LeasePath),
			))
			rows = append(rows, fmt.Sprintf(
				"reclaim result: %s/%s %s worker=%s",
				result.RunID,
				result.ExecutionID,
				installRequestDisplayValue(result.Status, "unknown"),
				installRequestDisplayValue(result.WorkerID, "unknown"),
			))
		}
		for _, skipped := range execution.Skipped {
			lease := skipped.Lease
			rows = append(rows, fmt.Sprintf(
				"worker lease reclaim execution skip run=%s execution=%s reason=%s",
				installRequestDisplayValue(lease.RunID, "unknown"),
				installRequestDisplayValue(lease.ExecutionID, "unknown"),
				installRequestDisplayValue(skipped.Reason, "unknown"),
			))
		}
	}
	return rows
}

func compactWorkerLeaseReclaimExecutionOutput(execution WorkerLeaseReclaimExecution) string {
	if len(execution.Results) == 0 && len(execution.Skipped) == 0 {
		return ""
	}
	return fmt.Sprintf("reclaimed=%d skipped=%d", len(execution.Results), len(execution.Skipped))
}

func compactWorkerLeaseResultReconciliationExecutionOutput(execution WorkerLeaseResultReconciliationExecution) string {
	if len(execution.Results) == 0 && len(execution.Skipped) == 0 {
		return ""
	}
	return fmt.Sprintf("released=%d skipped=%d", len(execution.Results), len(execution.Skipped))
}

func tuiModelWithInstallRequestRows(root string, model tui.Model) tui.Model {
	safetyRows := workspaceSafetyBypassRuntimeRows(root)
	if len(safetyRows) > 0 && strings.HasPrefix(strings.TrimSpace(safetyRows[0]), "workspace safety: errors=") {
		return model.AppendRuntimeRows(tui.RuntimeTabToolchainHealth, append([]string(nil), safetyRows...))
	}
	requests, err := ListToolchainInstallRequests(root)
	if err != nil {
		if len(safetyRows) > 0 {
			return model.AppendRuntimeRows(tui.RuntimeTabToolchainHealth, append([]string(nil), safetyRows...))
		}
		return model
	}
	dispatches, _ := ListToolchainInstallDispatches(root)
	executions, _ := ListToolchainInstallDispatchExecutions(root)
	retryDispatches, _ := ListToolchainInstallRetryDispatches(root)
	retryExecutions, _ := ListToolchainInstallRetryDispatchExecutions(root)
	orchestrations, _ := ListToolchainInstallOrchestrations(root)
	retryPlan := toolchainInstallRetryPlanFromExecutions(executions)
	if len(safetyRows) == 0 && len(requests) == 0 && len(dispatches) == 0 && len(executions) == 0 && len(retryDispatches) == 0 && len(retryExecutions) == 0 && len(orchestrations) == 0 {
		return model
	}
	rows := append([]string(nil), safetyRows...)
	summary := SummarizeToolchainInstallRequests(requests)
	dispatchPlan := toolchainInstallDispatchPlanFromRequests(requests)
	if len(requests) > 0 {
		requestRows := installRequestRuntimeRows(requests, summary)
		rows = append(rows, requestRows[:1]...)
		rows = append(rows, installOrchestrationArtifactRuntimeRows(orchestrations)...)
		rows = append(rows, installDispatchRuntimeRows(dispatchPlan)...)
		rows = append(rows, installExecutionArtifactRuntimeRows(executions)...)
		rows = append(rows, installRetryRuntimeRows(retryPlan)...)
		rows = append(rows, installRetryExecutionArtifactRuntimeRows(retryExecutions)...)
		rows = append(rows, installRetryDispatchArtifactRuntimeRows(retryDispatches)...)
		rows = append(rows, installDispatchArtifactRuntimeRows(dispatches)...)
		rows = append(rows, requestRows[1:]...)
	} else {
		rows = append(rows, installOrchestrationArtifactRuntimeRows(orchestrations)...)
		rows = append(rows, installExecutionArtifactRuntimeRows(executions)...)
		rows = append(rows, installRetryRuntimeRows(retryPlan)...)
		rows = append(rows, installRetryExecutionArtifactRuntimeRows(retryExecutions)...)
		rows = append(rows, installRetryDispatchArtifactRuntimeRows(retryDispatches)...)
		rows = append(rows, installDispatchArtifactRuntimeRows(dispatches)...)
	}
	model = model.AppendRuntimeRows(tui.RuntimeTabToolchainHealth, rows)
	if len(requests) > 0 {
		model = model.WithInstallRequestStatus(summary.Compact())
	}
	return model
}

func workspaceSafetyBypassRuntimeRows(root string) []string {
	config, err := LoadScopeConfig(filepath.Join(root, "scope.yaml"))
	if err != nil {
		return nil
	}
	authorization, authorizationFound, authorizationErr := loadWorkspaceAuthorization(filepath.Join(root, "authorization.yaml"))
	if authorizationErr != nil {
		return []string{
			"workspace safety: errors=1",
			"workspace safety strict gate: blocked errors=1",
			"safety error: invalid authorization.yaml: " + authorizationErr.Error(),
		}
	}
	now := doctorNow(DoctorOptions{})
	warnings := scopeConfigWarnings(config, authorization, authorizationFound, now)
	if len(warnings) == 0 {
		return nil
	}
	rows := []string{fmt.Sprintf("workspace safety: warnings=%d", len(warnings))}
	strictErrors := scopeConfigSafetyAuthorizationErrors(config, authorization, authorizationFound, now)
	if len(strictErrors) > 0 {
		rows = append(rows, fmt.Sprintf("workspace safety strict gate: blocked errors=%d", len(strictErrors)))
	} else {
		rows = append(rows, "workspace safety strict gate: ready")
	}
	for _, warning := range warnings {
		if trimmed := strings.TrimSpace(warning); trimmed != "" {
			rows = append(rows, "safety warning: "+trimmed)
		}
	}
	for _, strictError := range strictErrors {
		if trimmed := strings.TrimSpace(strictError); trimmed != "" {
			rows = append(rows, "safety strict error: "+trimmed)
		}
	}
	return rows
}

func tuiModelWithManagedBundleRows(root string, model tui.Model) tui.Model {
	bundles, err := ListManagedToolBundles(root)
	if err != nil {
		return model.AppendRuntimeRows(tui.RuntimeTabToolchainHealth, []string{
			"managed bundle error: mode=metadata-only error=" + compactRuntimeRowValue(err.Error()),
		})
	}
	if len(bundles) == 0 {
		return model
	}
	return model.AppendRuntimeRows(tui.RuntimeTabToolchainHealth, managedBundleRuntimeRows(bundles))
}

func managedBundleRuntimeRows(bundles []ManagedToolBundle) []string {
	rows := []string{fmt.Sprintf("managed bundles: total=%d", len(bundles))}
	for _, bundle := range bundles {
		rows = append(rows, fmt.Sprintf(
			"managed bundle: name=%s status=%s mode=metadata-only kind=%s version=%s source=%s source_b64=%s update_status=%s error=%s provenance_status=%s corpus_status=%s corpus_name=%s corpus_source=%s corpus_retrieved_at=%s corpus_file_count=%d corpus_sha256=%s corpus_digest_status=%s corpus_digest_algorithm=%s corpus_digest_scope=%s signature_status=%s signature_algorithm=%s signature_key_id=%s signature_signed_at=%s signature_signed_by=%s sha256=%s update_latest_version=%s update_latest_sha256=%s update_checked_at=%s update_message=%s provenance_source_url=%s provenance_upstream_version=%s provenance_retrieved_at=%s provenance_verified_by=%s payload=%s payload_b64=%s install=%s install_b64=%s manifest=%s manifest_b64=%s",
			installRequestDisplayValue(bundle.Name, "unknown"),
			installRequestDisplayValue(bundle.Status, "unknown"),
			installRequestDisplayValue(bundle.Kind, "unknown"),
			installRequestDisplayValue(bundle.Version, "unknown"),
			compactRuntimeRowValue(bundle.Source),
			runtimeRowValueB64(bundle.Source),
			installRequestDisplayValue(bundle.UpdateStatus, "unknown"),
			strings.Join(strings.Fields(bundle.Error), "_"),
			installRequestDisplayValue(bundle.ProvenanceStatus, "missing"),
			installRequestDisplayValue(bundle.CorpusStatus, "missing"),
			installRequestDisplayValue(bundle.CorpusName, "none"),
			installRequestDisplayValue(bundle.CorpusSource, "none"),
			installRequestDisplayValue(bundle.CorpusRetrievedAt, "none"),
			bundle.CorpusFileCount,
			installRequestDisplayValue(bundle.CorpusSHA256, "none"),
			installRequestDisplayValue(bundle.CorpusDigestStatus, "none"),
			installRequestDisplayValue(bundle.CorpusDigestAlgorithm, "none"),
			installRequestDisplayValue(bundle.CorpusDigestScope, "none"),
			installRequestDisplayValue(bundle.SignatureStatus, "missing"),
			installRequestDisplayValue(bundle.SignatureAlgorithm, "none"),
			installRequestDisplayValue(bundle.SignatureKeyID, "none"),
			installRequestDisplayValue(bundle.SignatureSignedAt, "none"),
			installRequestDisplayValue(bundle.SignatureSignedBy, "none"),
			installRequestDisplayValue(bundle.SHA256, "none"),
			installRequestDisplayValue(bundle.UpdateLatestVersion, "none"),
			installRequestDisplayValue(bundle.UpdateLatestSHA256, "none"),
			installRequestDisplayValue(bundle.UpdateCheckedAt, "none"),
			installRequestDisplayValue(bundle.UpdateMessage, "none"),
			installRequestDisplayValue(bundle.ProvenanceSourceURL, "none"),
			installRequestDisplayValue(bundle.ProvenanceUpstreamVersion, "none"),
			installRequestDisplayValue(bundle.ProvenanceRetrievedAt, "none"),
			installRequestDisplayValue(bundle.ProvenanceVerifiedBy, "none"),
			installRequestDisplayValue(compactRuntimeRowValue(bundle.PayloadPath), "none"),
			runtimeRowValueB64(bundle.PayloadPath),
			installRequestDisplayValue(compactRuntimeRowValue(bundle.InstallPath), "none"),
			runtimeRowValueB64(bundle.InstallPath),
			installRequestDisplayValue(compactRuntimeRowValue(bundle.ManifestPath), "none"),
			runtimeRowValueB64(bundle.ManifestPath),
		))
	}
	return rows
}

func tuiModelWithReleaseManagedStatusRows(root string, model tui.Model) tui.Model {
	source, found, err := ReadReleaseManagedToolBundleStatusSource(root)
	if err != nil {
		return model.AppendRuntimeRows(tui.RuntimeTabToolchainHealth, []string{
			"release managed status source error: mode=metadata-only error=" + compactRuntimeRowValue(err.Error()),
		})
	}
	if !found {
		return model
	}
	rows := releaseManagedStatusSourceRuntimeRows(source)
	if strings.TrimSpace(source.Status) != "current" || strings.TrimSpace(source.ReleaseRoot) == "" {
		return model.AppendRuntimeRows(tui.RuntimeTabToolchainHealth, rows)
	}
	statuses, err := listReleaseManagedToolBundleStatusesWithExpectedSHA(root, source.ReleaseRoot, source.ExpectedSHA256Bundle, source.ExpectedSHA256)
	if err != nil {
		rows = append(rows, "release managed status error: mode=read-only error="+compactRuntimeRowValue(err.Error()))
		return model.AppendRuntimeRows(tui.RuntimeTabToolchainHealth, rows)
	}
	rows = append(rows, releaseManagedStatusRuntimeRows(statuses)...)
	return model.AppendRuntimeRows(tui.RuntimeTabToolchainHealth, rows)
}

func tuiModelWithManagedUpdateRequestRows(root string, model tui.Model) tui.Model {
	requests, err := ListManagedToolBundleUpdateRequests(root)
	if err != nil || len(requests) == 0 {
		return model
	}
	return model.AppendRuntimeRows(tui.RuntimeTabToolchainHealth, managedUpdateRequestRuntimeRows(requests))
}

func tuiModelWithManagedUpdateDownloadPlanRows(root string, model tui.Model) tui.Model {
	plans, err := ListManagedToolBundleUpdateDownloadPlans(root)
	if err != nil || len(plans) == 0 {
		return model
	}
	return model.AppendRuntimeRows(tui.RuntimeTabToolchainHealth, managedUpdateDownloadPlanRuntimeRows(plans))
}

func tuiModelWithManagedUpdateDownloadExecutionRows(root string, model tui.Model) tui.Model {
	executions, err := ListManagedToolBundleUpdateDownloadExecutions(root)
	if err != nil || len(executions) == 0 {
		return model
	}
	return model.AppendRuntimeRows(tui.RuntimeTabToolchainHealth, managedUpdateDownloadExecutionRuntimeRows(executions))
}

func tuiModelWithManagedUpdateApplyExecutionRows(root string, model tui.Model) tui.Model {
	executions, err := ListManagedToolBundleUpdateApplyExecutions(root)
	if err != nil || len(executions) == 0 {
		return model
	}
	return model.AppendRuntimeRows(tui.RuntimeTabToolchainHealth, managedUpdateApplyExecutionRuntimeRows(executions))
}

func releaseManagedStatusSourceRuntimeRows(source ReleaseManagedToolBundleStatusSource) []string {
	return []string{fmt.Sprintf(
		"release managed status source: status=%s mode=%s expected_sha256_bundle=%s expected_sha256_status=%s expected_sha256=%s release_root=%s release_root_b64=%s total=%d available=%d blocked=%d not_imported=%d imported=%d linked=%d conflict=%d marker=%s marker_b64=%s reason=%s reason_b64=%s",
		installRequestDisplayValue(source.Status, "unknown"),
		installRequestDisplayValue(source.Mode, "metadata-only"),
		installRequestDisplayValue(source.ExpectedSHA256Bundle, "none"),
		installRequestDisplayValue(source.ExpectedSHA256Status, "none"),
		installRequestDisplayValue(source.ExpectedSHA256, "none"),
		installRequestDisplayValue(compactRuntimeRowValue(source.ReleaseRoot), "none"),
		runtimeRowValueB64(source.ReleaseRoot),
		source.Total,
		source.Available,
		source.Blocked,
		source.NotImported,
		source.Imported,
		source.Linked,
		source.Conflict,
		installRequestDisplayValue(compactRuntimeRowValue(source.Path), "none"),
		runtimeRowValueB64(source.Path),
		installRequestDisplayValue(compactRuntimeRowValue(source.Reason), "none"),
		runtimeRowValueB64(source.Reason),
	)}
}

func releaseManagedStatusRuntimeRows(statuses []ReleaseManagedToolBundleStatus) []string {
	summary := releaseManagedToolBundleStatusSourceFromStatuses("", "", statuses)
	rows := []string{fmt.Sprintf(
		"release managed statuses: total=%d available=%d blocked=%d not_imported=%d imported=%d linked=%d conflict=%d",
		summary.Total,
		summary.Available,
		summary.Blocked,
		summary.NotImported,
		summary.Imported,
		summary.Linked,
		summary.Conflict,
	)}
	for _, status := range statuses {
		rows = append(rows, fmt.Sprintf(
			"release managed status: name=%s status=%s mode=%s kind=%s version=%s expected_sha256_status=%s expected_sha256=%s digest_status=%s import_status=%s link_status=%s update_status=%s source=%s source_b64=%s sha256=%s provenance_status=%s corpus_status=%s corpus_name=%s corpus_source=%s corpus_retrieved_at=%s corpus_file_count=%d corpus_sha256=%s corpus_digest_status=%s corpus_digest_algorithm=%s corpus_digest_scope=%s signature_status=%s signature_algorithm=%s signature_key_id=%s signature_signed_at=%s signature_signed_by=%s update_latest_version=%s update_latest_sha256=%s update_checked_at=%s update_message=%s provenance_source_url=%s provenance_upstream_version=%s provenance_retrieved_at=%s provenance_verified_by=%s digest_algorithm=%s digest_scope=%s release_bundle=%s release_bundle_b64=%s release_payload=%s release_payload_b64=%s release_manifest=%s release_manifest_b64=%s workspace_bundle=%s workspace_bundle_b64=%s workspace_payload=%s workspace_payload_b64=%s workspace_manifest=%s workspace_manifest_b64=%s install=%s install_b64=%s error=%s error_b64=%s",
			installRequestDisplayValue(status.Name, "unknown"),
			installRequestDisplayValue(status.Status, "blocked"),
			installRequestDisplayValue(status.Mode, "read-only"),
			installRequestDisplayValue(status.Kind, "unknown"),
			installRequestDisplayValue(status.Version, "unknown"),
			installRequestDisplayValue(status.ExpectedSHA256Status, "none"),
			installRequestDisplayValue(status.ExpectedSHA256, "none"),
			installRequestDisplayValue(status.DigestStatus, "unknown"),
			installRequestDisplayValue(status.ImportStatus, "unknown"),
			installRequestDisplayValue(status.LinkStatus, "unknown"),
			installRequestDisplayValue(status.UpdateStatus, "unknown"),
			compactRuntimeRowValue(status.Source),
			runtimeRowValueB64(status.Source),
			installRequestDisplayValue(status.SHA256, "none"),
			installRequestDisplayValue(status.ProvenanceStatus, "missing"),
			installRequestDisplayValue(status.CorpusStatus, "missing"),
			installRequestDisplayValue(status.CorpusName, "none"),
			installRequestDisplayValue(status.CorpusSource, "none"),
			installRequestDisplayValue(status.CorpusRetrievedAt, "none"),
			status.CorpusFileCount,
			installRequestDisplayValue(status.CorpusSHA256, "none"),
			installRequestDisplayValue(status.CorpusDigestStatus, "none"),
			installRequestDisplayValue(status.CorpusDigestAlgorithm, "none"),
			installRequestDisplayValue(status.CorpusDigestScope, "none"),
			installRequestDisplayValue(status.SignatureStatus, "missing"),
			installRequestDisplayValue(status.SignatureAlgorithm, "none"),
			installRequestDisplayValue(status.SignatureKeyID, "none"),
			installRequestDisplayValue(status.SignatureSignedAt, "none"),
			installRequestDisplayValue(status.SignatureSignedBy, "none"),
			installRequestDisplayValue(status.UpdateLatestVersion, "none"),
			installRequestDisplayValue(status.UpdateLatestSHA256, "none"),
			installRequestDisplayValue(status.UpdateCheckedAt, "none"),
			installRequestDisplayValue(status.UpdateMessage, "none"),
			installRequestDisplayValue(status.ProvenanceSourceURL, "none"),
			installRequestDisplayValue(status.ProvenanceUpstreamVersion, "none"),
			installRequestDisplayValue(status.ProvenanceRetrievedAt, "none"),
			installRequestDisplayValue(status.ProvenanceVerifiedBy, "none"),
			installRequestDisplayValue(status.DigestAlgorithm, "sha256"),
			installRequestDisplayValue(status.DigestScope, "unknown"),
			installRequestDisplayValue(compactRuntimeRowValue(status.ReleaseBundleDir), "none"),
			runtimeRowValueB64(status.ReleaseBundleDir),
			installRequestDisplayValue(compactRuntimeRowValue(status.ReleasePayloadPath), "none"),
			runtimeRowValueB64(status.ReleasePayloadPath),
			installRequestDisplayValue(compactRuntimeRowValue(status.ReleaseManifestPath), "none"),
			runtimeRowValueB64(status.ReleaseManifestPath),
			installRequestDisplayValue(compactRuntimeRowValue(status.WorkspaceBundleDir), "none"),
			runtimeRowValueB64(status.WorkspaceBundleDir),
			installRequestDisplayValue(compactRuntimeRowValue(status.WorkspacePayloadPath), "none"),
			runtimeRowValueB64(status.WorkspacePayloadPath),
			installRequestDisplayValue(compactRuntimeRowValue(status.WorkspaceManifestPath), "none"),
			runtimeRowValueB64(status.WorkspaceManifestPath),
			installRequestDisplayValue(compactRuntimeRowValue(status.InstallPath), "none"),
			runtimeRowValueB64(status.InstallPath),
			compactRuntimeRowValue(status.Error),
			runtimeRowValueB64(status.Error),
		))
	}
	return rows
}

func managedUpdateRequestRuntimeRows(requests []ManagedToolBundleUpdateRequest) []string {
	summary := SummarizeManagedToolBundleUpdateRequests(requests)
	rows := []string{"managed update requests: " + summary.Line()}
	for _, request := range requests {
		rows = append(rows, fmt.Sprintf(
			"managed update request artifact: status=%s mode=%s name=%s kind=%s current_version=%s latest_version=%s update_status=%s requested_by=%s requested_by_b64=%s latest_sha256=%s current_sha256=%s update_checked_at=%s update_message=%s update_message_b64=%s release_root=%s release_root_b64=%s release_bundle=%s release_bundle_b64=%s release_manifest=%s release_manifest_b64=%s release_payload=%s release_payload_b64=%s artifact=%s artifact_b64=%s requested_at=%s reason=%s reason_b64=%s",
			installRequestDisplayValue(request.Status, "unknown"),
			installRequestDisplayValue(request.Mode, "metadata-only"),
			installRequestDisplayValue(request.Name, "unknown"),
			installRequestDisplayValue(request.Kind, "unknown"),
			installRequestDisplayValue(request.CurrentVersion, "unknown"),
			installRequestDisplayValue(request.LatestVersion, "none"),
			installRequestDisplayValue(request.UpdateStatus, "unknown"),
			installRequestDisplayValue(compactRuntimeRowValue(request.RequestedBy), "operator"),
			runtimeRowValueB64(request.RequestedBy),
			installRequestDisplayValue(request.LatestSHA256, "none"),
			installRequestDisplayValue(request.CurrentSHA256, "none"),
			installRequestDisplayValue(request.UpdateCheckedAt, "none"),
			installRequestDisplayValue(compactRuntimeRowValue(request.UpdateMessage), "none"),
			runtimeRowValueB64(request.UpdateMessage),
			installRequestDisplayValue(compactRuntimeRowValue(request.ReleaseRoot), "none"),
			runtimeRowValueB64(request.ReleaseRoot),
			installRequestDisplayValue(compactRuntimeRowValue(request.ReleaseBundleDir), "none"),
			runtimeRowValueB64(request.ReleaseBundleDir),
			installRequestDisplayValue(compactRuntimeRowValue(request.ReleaseManifestPath), "none"),
			runtimeRowValueB64(request.ReleaseManifestPath),
			installRequestDisplayValue(compactRuntimeRowValue(request.ReleasePayloadPath), "none"),
			runtimeRowValueB64(request.ReleasePayloadPath),
			installRequestDisplayValue(compactRuntimeRowValue(request.Path), "none"),
			runtimeRowValueB64(request.Path),
			installRequestDisplayValue(request.RequestedAt, "none"),
			installRequestDisplayValue(compactRuntimeRowValue(request.Reason), "none"),
			runtimeRowValueB64(request.Reason),
		))
		rows = append(rows, fmt.Sprintf(
			"managed update request digest: name=%s latest_sha256=%s current_sha256=%s",
			installRequestDisplayValue(request.Name, "unknown"),
			installRequestDisplayValue(request.LatestSHA256, "none"),
			installRequestDisplayValue(request.CurrentSHA256, "none"),
		))
	}
	return rows
}

func managedUpdateDownloadPlanRuntimeRows(plans []ManagedToolBundleUpdateDownloadPlan) []string {
	summary := SummarizeManagedToolBundleUpdateDownloadPlans(plans)
	rows := []string{"managed update download plans: " + summary.Line()}
	for _, plan := range plans {
		rows = append(rows, fmt.Sprintf(
			"managed update download plan artifact: status=%s mode=%s name=%s kind=%s current_version=%s latest_version=%s update_status=%s expected_sha256=%s expected_sha256_status=%s archive_format=%s archive_expected_sha256=%s archive_expected_sha256_status=%s source_url=%s source_url_b64=%s planned_by=%s planned_by_b64=%s latest_sha256=%s current_sha256=%s update_checked_at=%s update_message=%s update_message_b64=%s request=%s request_b64=%s release_root=%s release_root_b64=%s release_bundle=%s release_bundle_b64=%s release_manifest=%s release_manifest_b64=%s release_payload=%s release_payload_b64=%s artifact=%s artifact_b64=%s planned_at=%s reason=%s reason_b64=%s",
			installRequestDisplayValue(plan.Status, "unknown"),
			installRequestDisplayValue(plan.Mode, "metadata-only"),
			installRequestDisplayValue(plan.Name, "unknown"),
			installRequestDisplayValue(plan.Kind, "unknown"),
			installRequestDisplayValue(plan.CurrentVersion, "unknown"),
			installRequestDisplayValue(plan.LatestVersion, "none"),
			installRequestDisplayValue(plan.UpdateStatus, "unknown"),
			installRequestDisplayValue(plan.ExpectedSHA256, "none"),
			installRequestDisplayValue(plan.ExpectedSHA256Status, "unknown"),
			installRequestDisplayValue(plan.ArchiveFormat, "none"),
			installRequestDisplayValue(plan.ArchiveExpectedSHA256, "none"),
			installRequestDisplayValue(plan.ArchiveExpectedSHA256Status, "unknown"),
			installRequestDisplayValue(compactRuntimeRowValue(plan.SourceURL), "none"),
			runtimeRowValueB64(plan.SourceURL),
			installRequestDisplayValue(compactRuntimeRowValue(plan.PlannedBy), "operator"),
			runtimeRowValueB64(plan.PlannedBy),
			installRequestDisplayValue(plan.LatestSHA256, "none"),
			installRequestDisplayValue(plan.CurrentSHA256, "none"),
			installRequestDisplayValue(plan.UpdateCheckedAt, "none"),
			installRequestDisplayValue(compactRuntimeRowValue(plan.UpdateMessage), "none"),
			runtimeRowValueB64(plan.UpdateMessage),
			installRequestDisplayValue(compactRuntimeRowValue(plan.RequestPath), "none"),
			runtimeRowValueB64(plan.RequestPath),
			installRequestDisplayValue(compactRuntimeRowValue(plan.ReleaseRoot), "none"),
			runtimeRowValueB64(plan.ReleaseRoot),
			installRequestDisplayValue(compactRuntimeRowValue(plan.ReleaseBundleDir), "none"),
			runtimeRowValueB64(plan.ReleaseBundleDir),
			installRequestDisplayValue(compactRuntimeRowValue(plan.ReleaseManifestPath), "none"),
			runtimeRowValueB64(plan.ReleaseManifestPath),
			installRequestDisplayValue(compactRuntimeRowValue(plan.ReleasePayloadPath), "none"),
			runtimeRowValueB64(plan.ReleasePayloadPath),
			installRequestDisplayValue(compactRuntimeRowValue(plan.Path), "none"),
			runtimeRowValueB64(plan.Path),
			installRequestDisplayValue(plan.PlannedAt, "none"),
			installRequestDisplayValue(compactRuntimeRowValue(plan.Reason), "none"),
			runtimeRowValueB64(plan.Reason),
		))
		rows = append(rows, fmt.Sprintf(
			"managed update download plan digest: name=%s expected_sha256=%s latest_sha256=%s current_sha256=%s source_url=%s",
			installRequestDisplayValue(plan.Name, "unknown"),
			installRequestDisplayValue(plan.ExpectedSHA256, "none"),
			installRequestDisplayValue(plan.LatestSHA256, "none"),
			installRequestDisplayValue(plan.CurrentSHA256, "none"),
			installRequestDisplayValue(compactRuntimeRowValue(plan.SourceURL), "none"),
		))
	}
	return rows
}

func managedUpdateDownloadExecutionRuntimeRows(executions []ManagedToolBundleUpdateDownloadExecution) []string {
	summary := SummarizeManagedToolBundleUpdateDownloadExecutions(executions)
	rows := []string{"managed update download executions: " + summary.Line()}
	for _, execution := range executions {
		rows = append(rows, fmt.Sprintf(
			"managed update download execution artifact: status=%s mode=%s action=%s name=%s kind=%s current_version=%s latest_version=%s update_status=%s expected_sha256=%s actual_sha256=%s digest_status=%s digest_algorithm=%s digest_scope=%s archive_format=%s archive_expected_sha256=%s archive_actual_sha256=%s archive_digest_status=%s archive_digest_algorithm=%s archive_digest_scope=%s http_status=%d content_length=%d bytes_written=%d redirect_policy=%s redirect_location=%s redirect_location_b64=%s redirect_location_status=%s source_url=%s source_url_b64=%s actor=%s actor_b64=%s latest_sha256=%s current_sha256=%s update_checked_at=%s update_message=%s update_message_b64=%s plan=%s plan_b64=%s request=%s request_b64=%s release_root=%s release_root_b64=%s release_bundle=%s release_bundle_b64=%s release_manifest=%s release_manifest_b64=%s release_payload=%s release_payload_b64=%s archive=%s archive_b64=%s payload=%s payload_b64=%s artifact=%s artifact_b64=%s executed_at=%s reason=%s reason_b64=%s error=%s error_b64=%s",
			installRequestDisplayValue(execution.Status, "unknown"),
			installRequestDisplayValue(execution.Mode, "execute"),
			installRequestDisplayValue(execution.Action, "download"),
			installRequestDisplayValue(execution.Name, "unknown"),
			installRequestDisplayValue(execution.Kind, "unknown"),
			installRequestDisplayValue(execution.CurrentVersion, "unknown"),
			installRequestDisplayValue(execution.LatestVersion, "none"),
			installRequestDisplayValue(execution.UpdateStatus, "unknown"),
			installRequestDisplayValue(execution.ExpectedSHA256, "none"),
			installRequestDisplayValue(execution.ActualSHA256, "none"),
			installRequestDisplayValue(execution.DigestStatus, "unknown"),
			installRequestDisplayValue(execution.DigestAlgorithm, "unknown"),
			installRequestDisplayValue(execution.DigestScope, "unknown"),
			installRequestDisplayValue(execution.ArchiveFormat, "none"),
			installRequestDisplayValue(execution.ArchiveExpectedSHA256, "none"),
			installRequestDisplayValue(execution.ArchiveActualSHA256, "none"),
			installRequestDisplayValue(execution.ArchiveDigestStatus, "unknown"),
			installRequestDisplayValue(execution.ArchiveDigestAlgorithm, "unknown"),
			installRequestDisplayValue(execution.ArchiveDigestScope, "unknown"),
			execution.HTTPStatus,
			execution.ContentLength,
			execution.BytesWritten,
			installRequestDisplayValue(execution.RedirectPolicy, "none"),
			installRequestDisplayValue(compactRuntimeRowValue(execution.RedirectLocation), "none"),
			runtimeRowValueB64(execution.RedirectLocation),
			installRequestDisplayValue(execution.RedirectLocationStatus, "none"),
			installRequestDisplayValue(compactRuntimeRowValue(execution.SourceURL), "none"),
			runtimeRowValueB64(execution.SourceURL),
			installRequestDisplayValue(compactRuntimeRowValue(execution.Actor), "operator"),
			runtimeRowValueB64(execution.Actor),
			installRequestDisplayValue(execution.LatestSHA256, "none"),
			installRequestDisplayValue(execution.CurrentSHA256, "none"),
			installRequestDisplayValue(execution.UpdateCheckedAt, "none"),
			installRequestDisplayValue(compactRuntimeRowValue(execution.UpdateMessage), "none"),
			runtimeRowValueB64(execution.UpdateMessage),
			installRequestDisplayValue(compactRuntimeRowValue(execution.PlanPath), "none"),
			runtimeRowValueB64(execution.PlanPath),
			installRequestDisplayValue(compactRuntimeRowValue(execution.RequestPath), "none"),
			runtimeRowValueB64(execution.RequestPath),
			installRequestDisplayValue(compactRuntimeRowValue(execution.ReleaseRoot), "none"),
			runtimeRowValueB64(execution.ReleaseRoot),
			installRequestDisplayValue(compactRuntimeRowValue(execution.ReleaseBundleDir), "none"),
			runtimeRowValueB64(execution.ReleaseBundleDir),
			installRequestDisplayValue(compactRuntimeRowValue(execution.ReleaseManifestPath), "none"),
			runtimeRowValueB64(execution.ReleaseManifestPath),
			installRequestDisplayValue(compactRuntimeRowValue(execution.ReleasePayloadPath), "none"),
			runtimeRowValueB64(execution.ReleasePayloadPath),
			installRequestDisplayValue(compactRuntimeRowValue(execution.ArchivePath), "none"),
			runtimeRowValueB64(execution.ArchivePath),
			installRequestDisplayValue(compactRuntimeRowValue(execution.PayloadPath), "none"),
			runtimeRowValueB64(execution.PayloadPath),
			installRequestDisplayValue(compactRuntimeRowValue(execution.Path), "none"),
			runtimeRowValueB64(execution.Path),
			installRequestDisplayValue(execution.ExecutedAt, "none"),
			installRequestDisplayValue(compactRuntimeRowValue(execution.Reason), "none"),
			runtimeRowValueB64(execution.Reason),
			installRequestDisplayValue(compactRuntimeRowValue(execution.Error), "none"),
			runtimeRowValueB64(execution.Error),
		))
		rows = append(rows, fmt.Sprintf(
			"managed update download execution digest: name=%s digest_status=%s digest_algorithm=%s digest_scope=%s expected_sha256=%s actual_sha256=%s archive_digest_status=%s archive_digest_scope=%s bytes_written=%d http_status=%d",
			installRequestDisplayValue(execution.Name, "unknown"),
			installRequestDisplayValue(execution.DigestStatus, "unknown"),
			installRequestDisplayValue(execution.DigestAlgorithm, "unknown"),
			installRequestDisplayValue(execution.DigestScope, "unknown"),
			installRequestDisplayValue(execution.ExpectedSHA256, "none"),
			installRequestDisplayValue(execution.ActualSHA256, "none"),
			installRequestDisplayValue(execution.ArchiveDigestStatus, "unknown"),
			installRequestDisplayValue(execution.ArchiveDigestScope, "unknown"),
			execution.BytesWritten,
			execution.HTTPStatus,
		))
		rows = append(rows, fmt.Sprintf(
			"managed update download execution transfer: name=%s bytes_written=%d http_status=%d content_length=%d archive_format=%s archive_digest_status=%s redirect_policy=%s redirect_location_status=%s",
			installRequestDisplayValue(execution.Name, "unknown"),
			execution.BytesWritten,
			execution.HTTPStatus,
			execution.ContentLength,
			installRequestDisplayValue(execution.ArchiveFormat, "none"),
			installRequestDisplayValue(execution.ArchiveDigestStatus, "unknown"),
			installRequestDisplayValue(execution.RedirectPolicy, "none"),
			installRequestDisplayValue(execution.RedirectLocationStatus, "none"),
		))
	}
	return rows
}

func managedUpdateApplyExecutionRuntimeRows(executions []ManagedToolBundleUpdateApplyExecution) []string {
	summary := SummarizeManagedToolBundleUpdateApplyExecutions(executions)
	rows := []string{"managed update apply executions: " + summary.Line()}
	for _, execution := range executions {
		rows = append(rows, fmt.Sprintf(
			"managed update apply execution artifact: status=%s mode=%s action=%s name=%s kind=%s current_version=%s latest_version=%s update_status=%s expected_sha256=%s actual_sha256=%s digest_status=%s digest_algorithm=%s digest_scope=%s source_url=%s source_url_b64=%s actor=%s actor_b64=%s latest_sha256=%s current_sha256=%s update_checked_at=%s update_message=%s update_message_b64=%s download_execution=%s download_execution_b64=%s plan=%s plan_b64=%s request=%s request_b64=%s release_root=%s release_root_b64=%s release_bundle=%s release_bundle_b64=%s release_manifest=%s release_manifest_b64=%s release_payload=%s release_payload_b64=%s staged_payload=%s staged_payload_b64=%s workspace_bundle=%s workspace_bundle_b64=%s workspace_manifest=%s workspace_manifest_b64=%s workspace_payload=%s workspace_payload_b64=%s install=%s install_b64=%s artifact=%s artifact_b64=%s applied_at=%s reason=%s reason_b64=%s error=%s error_b64=%s",
			installRequestDisplayValue(execution.Status, "unknown"),
			installRequestDisplayValue(execution.Mode, "apply"),
			installRequestDisplayValue(execution.Action, "workspace-copy"),
			installRequestDisplayValue(execution.Name, "unknown"),
			installRequestDisplayValue(execution.Kind, "unknown"),
			installRequestDisplayValue(execution.CurrentVersion, "unknown"),
			installRequestDisplayValue(execution.LatestVersion, "none"),
			installRequestDisplayValue(execution.UpdateStatus, "unknown"),
			installRequestDisplayValue(execution.ExpectedSHA256, "none"),
			installRequestDisplayValue(execution.ActualSHA256, "none"),
			installRequestDisplayValue(execution.DigestStatus, "unknown"),
			installRequestDisplayValue(execution.DigestAlgorithm, "unknown"),
			installRequestDisplayValue(execution.DigestScope, "unknown"),
			installRequestDisplayValue(compactRuntimeRowValue(execution.SourceURL), "none"),
			runtimeRowValueB64(execution.SourceURL),
			installRequestDisplayValue(compactRuntimeRowValue(execution.Actor), "operator"),
			runtimeRowValueB64(execution.Actor),
			installRequestDisplayValue(execution.LatestSHA256, "none"),
			installRequestDisplayValue(execution.CurrentSHA256, "none"),
			installRequestDisplayValue(execution.UpdateCheckedAt, "none"),
			installRequestDisplayValue(compactRuntimeRowValue(execution.UpdateMessage), "none"),
			runtimeRowValueB64(execution.UpdateMessage),
			installRequestDisplayValue(compactRuntimeRowValue(execution.DownloadExecutionPath), "none"),
			runtimeRowValueB64(execution.DownloadExecutionPath),
			installRequestDisplayValue(compactRuntimeRowValue(execution.PlanPath), "none"),
			runtimeRowValueB64(execution.PlanPath),
			installRequestDisplayValue(compactRuntimeRowValue(execution.RequestPath), "none"),
			runtimeRowValueB64(execution.RequestPath),
			installRequestDisplayValue(compactRuntimeRowValue(execution.ReleaseRoot), "none"),
			runtimeRowValueB64(execution.ReleaseRoot),
			installRequestDisplayValue(compactRuntimeRowValue(execution.ReleaseBundleDir), "none"),
			runtimeRowValueB64(execution.ReleaseBundleDir),
			installRequestDisplayValue(compactRuntimeRowValue(execution.ReleaseManifestPath), "none"),
			runtimeRowValueB64(execution.ReleaseManifestPath),
			installRequestDisplayValue(compactRuntimeRowValue(execution.ReleasePayloadPath), "none"),
			runtimeRowValueB64(execution.ReleasePayloadPath),
			installRequestDisplayValue(compactRuntimeRowValue(execution.StagedPayloadPath), "none"),
			runtimeRowValueB64(execution.StagedPayloadPath),
			installRequestDisplayValue(compactRuntimeRowValue(execution.WorkspaceBundleDir), "none"),
			runtimeRowValueB64(execution.WorkspaceBundleDir),
			installRequestDisplayValue(compactRuntimeRowValue(execution.WorkspaceManifestPath), "none"),
			runtimeRowValueB64(execution.WorkspaceManifestPath),
			installRequestDisplayValue(compactRuntimeRowValue(execution.WorkspacePayloadPath), "none"),
			runtimeRowValueB64(execution.WorkspacePayloadPath),
			installRequestDisplayValue(compactRuntimeRowValue(execution.InstallPath), "none"),
			runtimeRowValueB64(execution.InstallPath),
			installRequestDisplayValue(compactRuntimeRowValue(execution.Path), "none"),
			runtimeRowValueB64(execution.Path),
			installRequestDisplayValue(execution.AppliedAt, "none"),
			installRequestDisplayValue(compactRuntimeRowValue(execution.Reason), "none"),
			runtimeRowValueB64(execution.Reason),
			installRequestDisplayValue(compactRuntimeRowValue(execution.Error), "none"),
			runtimeRowValueB64(execution.Error),
		))
		rows = append(rows, fmt.Sprintf(
			"managed update apply execution digest: name=%s digest_status=%s digest_algorithm=%s digest_scope=%s expected_sha256=%s actual_sha256=%s latest_version=%s",
			installRequestDisplayValue(execution.Name, "unknown"),
			installRequestDisplayValue(execution.DigestStatus, "unknown"),
			installRequestDisplayValue(execution.DigestAlgorithm, "unknown"),
			installRequestDisplayValue(execution.DigestScope, "unknown"),
			installRequestDisplayValue(execution.ExpectedSHA256, "none"),
			installRequestDisplayValue(execution.ActualSHA256, "none"),
			installRequestDisplayValue(execution.LatestVersion, "none"),
		))
	}
	return rows
}

func installRequestRuntimeRows(requests []ToolchainInstallRequest, summary ToolchainInstallRequestSummary) []string {
	rows := []string{"install requests: " + summary.Line()}
	for _, request := range requests {
		row := fmt.Sprintf(
			"request %s/%s: %s",
			request.Pack,
			request.Dependency,
			installRequestDisplayValue(request.Status, "unknown"),
		)
		if dependsOn := installDependsOnDisplay(request.DependsOn); dependsOn != "" {
			row += " depends_on=" + dependsOn
		}
		rows = append(rows, row)
		if dependsOn := installDependsOnDisplay(request.DependsOn); dependsOn != "" {
			rows = append(rows, fmt.Sprintf("request depends_on %s/%s: %s", request.Pack, request.Dependency, dependsOn))
		}
		if strings.TrimSpace(request.Path) != "" {
			rows = append(rows, "request path: "+request.Path)
		}
		if len(request.Command) > 0 {
			rows = append(rows, "request command: "+strings.Join(request.Command, " "))
		}
	}
	return rows
}

func installDispatchRuntimeRows(plan ToolchainInstallDispatchPlan) []string {
	rows := []string{fmt.Sprintf("install dispatch: ready=%d skipped=%d", len(plan.Ready), len(plan.Skipped))}
	for _, request := range plan.Ready {
		row := fmt.Sprintf(
			"dispatch %s/%s: ready mode=metadata-only",
			request.Pack,
			request.Dependency,
		)
		if dependsOn := installDependsOnDisplay(request.DependsOn); dependsOn != "" {
			row += " depends_on=" + dependsOn
		}
		rows = append(rows, row)
		if dependsOn := installDependsOnDisplay(request.DependsOn); dependsOn != "" {
			rows = append(rows, fmt.Sprintf("dispatch depends_on %s/%s: %s", request.Pack, request.Dependency, dependsOn))
		}
		if strings.TrimSpace(request.Path) != "" {
			rows = append(rows, "dispatch path: "+request.Path)
		}
		if len(request.Command) > 0 {
			rows = append(rows, "dispatch command: "+strings.Join(request.Command, " "))
		}
	}
	for _, skipped := range plan.Skipped {
		request := skipped.Request
		row := fmt.Sprintf(
			"dispatch skip %s/%s: %s",
			request.Pack,
			request.Dependency,
			skipped.Reason,
		)
		if dependsOn := installDependsOnDisplay(request.DependsOn); dependsOn != "" {
			row += " depends_on=" + dependsOn
		}
		rows = append(rows, row)
		if dependsOn := installDependsOnDisplay(request.DependsOn); dependsOn != "" {
			rows = append(rows, fmt.Sprintf("dispatch skip depends_on %s/%s: %s", request.Pack, request.Dependency, dependsOn))
		}
	}
	return rows
}

func installDependsOnDisplay(dependsOn []string) string {
	filtered := make([]string, 0, len(dependsOn))
	for _, dependency := range dependsOn {
		dependency = strings.TrimSpace(dependency)
		if dependency != "" {
			filtered = append(filtered, dependency)
		}
	}
	return strings.Join(filtered, ",")
}

func installDispatchArtifactRuntimeRows(dispatches []ToolchainInstallDispatch) []string {
	if len(dispatches) == 0 {
		return nil
	}
	rows := []string{fmt.Sprintf("install dispatch artifacts: total=%d", len(dispatches))}
	for _, dispatch := range dispatches {
		rows = append(rows, fmt.Sprintf(
			"dispatch artifact: %s artifact_b64=%s status=%s mode=%s ready=%d skipped=%d",
			compactRuntimeRowValue(dispatch.Path),
			runtimeRowValueB64(dispatch.Path),
			installRequestDisplayValue(dispatch.Status, "unknown"),
			installRequestDisplayValue(dispatch.Mode, "unknown"),
			len(dispatch.Ready),
			len(dispatch.Skipped),
		))
		rows = append(rows, fmt.Sprintf(
			"dispatch artifact state: %s mode=%s ready=%d skipped=%d",
			installRequestDisplayValue(dispatch.Status, "unknown"),
			installRequestDisplayValue(dispatch.Mode, "unknown"),
			len(dispatch.Ready),
			len(dispatch.Skipped),
		))
	}
	return rows
}

func installOrchestrationArtifactRuntimeRows(orchestrations []ToolchainInstallOrchestration) []string {
	if len(orchestrations) == 0 {
		return nil
	}
	rows := []string{fmt.Sprintf("install orchestration artifacts: total=%d", len(orchestrations))}
	for _, orchestration := range orchestrations {
		rows = append(rows, fmt.Sprintf(
			"install orchestration artifact: %s artifact_b64=%s status=%s mode=%s ready=%d skipped=%d results=%d dispatch=%s dispatch_b64=%s execution=%s execution_b64=%s retry_dispatch=%s retry_dispatch_b64=%s error=%s error_b64=%s",
			compactRuntimeRowValue(orchestration.Path),
			runtimeRowValueB64(orchestration.Path),
			installRequestDisplayValue(orchestration.Status, "unknown"),
			installRequestDisplayValue(orchestration.Mode, "unknown"),
			orchestration.DispatchReady,
			orchestration.DispatchSkipped,
			orchestration.ExecutionResults,
			compactRuntimeRowValue(installRequestDisplayValue(orchestration.DispatchPath, "none")),
			runtimeRowValueB64(orchestration.DispatchPath),
			compactRuntimeRowValue(installRequestDisplayValue(orchestration.ExecutionPath, "none")),
			runtimeRowValueB64(orchestration.ExecutionPath),
			compactRuntimeRowValue(installRequestDisplayValue(orchestration.RetryDispatchPath, "none")),
			runtimeRowValueB64(orchestration.RetryDispatchPath),
			compactRuntimeRowValue(orchestration.Error),
			runtimeRowValueB64(orchestration.Error),
		))
		rows = append(rows, fmt.Sprintf(
			"orchestration artifact state: %s mode=%s dispatch_ready=%d retry_ready=%d results=%d",
			installRequestDisplayValue(orchestration.Status, "unknown"),
			installRequestDisplayValue(orchestration.Mode, "unknown"),
			orchestration.DispatchReady,
			orchestration.RetryReady,
			orchestration.ExecutionResults,
		))
		rows = append(rows, fmt.Sprintf(
			"orchestration summary: %s %s d_ready=%d r_ready=%d results=%d",
			installRequestDisplayValue(orchestration.Status, "unknown"),
			installRequestDisplayValue(orchestration.Mode, "unknown"),
			orchestration.DispatchReady,
			orchestration.RetryReady,
			orchestration.ExecutionResults,
		))
	}
	return rows
}

func installExecutionArtifactRuntimeRows(executions []ToolchainInstallDispatchExecution) []string {
	if len(executions) == 0 {
		return nil
	}
	rows := []string{fmt.Sprintf("install execution artifacts: total=%d", len(executions))}
	for _, execution := range executions {
		rows = append(rows, fmt.Sprintf(
			"execution artifact: %s artifact_b64=%s status=%s mode=%s dispatch=%s dispatch_b64=%s results=%d skipped=%d",
			compactRuntimeRowValue(execution.Path),
			runtimeRowValueB64(execution.Path),
			installRequestDisplayValue(execution.Status, "unknown"),
			installRequestDisplayValue(execution.Mode, "unknown"),
			compactRuntimeRowValue(execution.DispatchPath),
			runtimeRowValueB64(execution.DispatchPath),
			len(execution.Results),
			len(execution.Skipped),
		))
		rows = append(rows, fmt.Sprintf(
			"execution artifact state: %s mode=%s results=%d skipped=%d",
			installRequestDisplayValue(execution.Status, "unknown"),
			installRequestDisplayValue(execution.Mode, "unknown"),
			len(execution.Results),
			len(execution.Skipped),
		))
		for _, result := range execution.Results {
			command := strings.Join(result.Command, " ")
			rows = append(rows, fmt.Sprintf(
				"execution result %s/%s: %s output=%s output_b64=%s artifact=%s artifact_b64=%s dispatch=%s dispatch_b64=%s command=%s command_b64=%s",
				result.Pack,
				result.Dependency,
				installRequestDisplayValue(result.Status, "unknown"),
				compactRuntimeRowValue(result.Output),
				runtimeRowValueB64(result.Output),
				compactRuntimeRowValue(execution.Path),
				runtimeRowValueB64(execution.Path),
				compactRuntimeRowValue(execution.DispatchPath),
				runtimeRowValueB64(execution.DispatchPath),
				command,
				runtimeRowValueB64(command),
			))
		}
	}
	return rows
}

func installRetryRuntimeRows(plan ToolchainInstallRetryPlan) []string {
	if plan.Summary.Total == 0 {
		return nil
	}
	rows := []string{fmt.Sprintf(
		"install retry plan: total=%d ready=%d skipped=%d",
		plan.Summary.Total,
		plan.Summary.Ready,
		plan.Summary.Skipped,
	)}
	for _, candidate := range plan.Ready {
		rows = append(rows, fmt.Sprintf(
			"retry install %s/%s: ready error=%s artifact=%s dispatch=%s command=%s",
			candidate.Pack,
			candidate.Dependency,
			compactRuntimeRowValue(candidate.LastError),
			candidate.ExecutionPath,
			candidate.DispatchPath,
			strings.Join(candidate.Command, " "),
		))
	}
	for _, skipped := range plan.Skipped {
		rows = append(rows, fmt.Sprintf(
			"retry skip %s/%s: %s artifact=%s",
			skipped.Pack,
			skipped.Dependency,
			skipped.Reason,
			skipped.ExecutionPath,
		))
	}
	return rows
}

func installRetryDispatchArtifactRuntimeRows(dispatches []ToolchainInstallRetryDispatch) []string {
	if len(dispatches) == 0 {
		return nil
	}
	rows := []string{fmt.Sprintf("install retry dispatch artifacts: total=%d", len(dispatches))}
	for _, dispatch := range dispatches {
		rows = append(rows, fmt.Sprintf(
			"retry dispatch artifact: %s artifact_b64=%s status=%s mode=%s ready=%d skipped=%d",
			compactRuntimeRowValue(dispatch.Path),
			runtimeRowValueB64(dispatch.Path),
			installRequestDisplayValue(dispatch.Status, "unknown"),
			installRequestDisplayValue(dispatch.Mode, "unknown"),
			len(dispatch.Ready),
			len(dispatch.Skipped),
		))
		rows = append(rows, fmt.Sprintf(
			"retry dispatch artifact state: %s mode=%s ready=%d skipped=%d",
			installRequestDisplayValue(dispatch.Status, "unknown"),
			installRequestDisplayValue(dispatch.Mode, "unknown"),
			len(dispatch.Ready),
			len(dispatch.Skipped),
		))
	}
	return rows
}

func installRetryExecutionArtifactRuntimeRows(executions []ToolchainInstallRetryDispatchExecution) []string {
	if len(executions) == 0 {
		return nil
	}
	rows := []string{fmt.Sprintf("install retry execution artifacts: total=%d", len(executions))}
	for _, execution := range executions {
		rows = append(rows, fmt.Sprintf(
			"retry execution artifact: %s artifact_b64=%s status=%s mode=%s retry_dispatch=%s retry_dispatch_b64=%s results=%d skipped=%d",
			compactRuntimeRowValue(execution.Path),
			runtimeRowValueB64(execution.Path),
			installRequestDisplayValue(execution.Status, "unknown"),
			installRequestDisplayValue(execution.Mode, "unknown"),
			compactRuntimeRowValue(execution.RetryDispatchPath),
			runtimeRowValueB64(execution.RetryDispatchPath),
			len(execution.Results),
			len(execution.Skipped),
		))
		rows = append(rows, fmt.Sprintf(
			"retry execution artifact state: %s mode=%s results=%d skipped=%d",
			installRequestDisplayValue(execution.Status, "unknown"),
			installRequestDisplayValue(execution.Mode, "unknown"),
			len(execution.Results),
			len(execution.Skipped),
		))
		for _, result := range execution.Results {
			command := strings.Join(result.Command, " ")
			rows = append(rows, fmt.Sprintf(
				"retry execution result %s/%s: %s output=%s output_b64=%s error=%s error_b64=%s artifact=%s artifact_b64=%s retry_dispatch=%s retry_dispatch_b64=%s source_execution=%s source_execution_b64=%s dispatch=%s dispatch_b64=%s command=%s command_b64=%s",
				result.Pack,
				result.Dependency,
				installRequestDisplayValue(result.Status, "unknown"),
				compactRuntimeRowValue(result.Output),
				runtimeRowValueB64(result.Output),
				compactRuntimeRowValue(result.Error),
				runtimeRowValueB64(result.Error),
				compactRuntimeRowValue(execution.Path),
				runtimeRowValueB64(execution.Path),
				compactRuntimeRowValue(execution.RetryDispatchPath),
				runtimeRowValueB64(execution.RetryDispatchPath),
				compactRuntimeRowValue(result.ExecutionPath),
				runtimeRowValueB64(result.ExecutionPath),
				compactRuntimeRowValue(result.DispatchPath),
				runtimeRowValueB64(result.DispatchPath),
				command,
				runtimeRowValueB64(command),
			))
			rows = append(rows, fmt.Sprintf(
				"retry execution result refs summary %s/%s: source_execution=%s source_execution_b64=%s dispatch=%s dispatch_b64=%s artifact=%s artifact_b64=%s retry_dispatch=%s retry_dispatch_b64=%s",
				result.Pack,
				result.Dependency,
				compactRuntimeRowValue(filepath.Base(result.ExecutionPath)),
				runtimeRowValueB64(filepath.Base(result.ExecutionPath)),
				compactRuntimeRowValue(filepath.Base(result.DispatchPath)),
				runtimeRowValueB64(filepath.Base(result.DispatchPath)),
				compactRuntimeRowValue(filepath.Base(execution.Path)),
				runtimeRowValueB64(filepath.Base(execution.Path)),
				compactRuntimeRowValue(filepath.Base(execution.RetryDispatchPath)),
				runtimeRowValueB64(filepath.Base(execution.RetryDispatchPath)),
			))
			rows = append(rows, fmt.Sprintf(
				"retry execution result refs %s/%s: source_execution=%s source_execution_b64=%s dispatch=%s dispatch_b64=%s artifact=%s artifact_b64=%s retry_dispatch=%s retry_dispatch_b64=%s command=%s command_b64=%s",
				result.Pack,
				result.Dependency,
				compactRuntimeRowValue(result.ExecutionPath),
				runtimeRowValueB64(result.ExecutionPath),
				compactRuntimeRowValue(result.DispatchPath),
				runtimeRowValueB64(result.DispatchPath),
				compactRuntimeRowValue(execution.Path),
				runtimeRowValueB64(execution.Path),
				compactRuntimeRowValue(execution.RetryDispatchPath),
				runtimeRowValueB64(execution.RetryDispatchPath),
				command,
				runtimeRowValueB64(command),
			))
		}
	}
	return rows
}

func compactRuntimeRowValue(value string) string {
	return strings.Join(strings.Fields(value), " ")
}

func runtimeRowValueB64(value string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(value))
}

func compactExecutionOutput(execution ToolchainInstallDispatchExecution) string {
	parts := make([]string, 0, len(execution.Results))
	for _, result := range execution.Results {
		if output := compactRuntimeRowValue(result.Output); output != "" {
			parts = append(parts, output)
		}
	}
	return strings.Join(parts, " | ")
}

func compactRetryExecutionOutput(execution ToolchainInstallRetryDispatchExecution) string {
	parts := make([]string, 0, len(execution.Results))
	for _, result := range execution.Results {
		if output := compactRuntimeRowValue(result.Output); output != "" {
			parts = append(parts, output)
		}
	}
	return strings.Join(parts, " | ")
}

func installRequestDisplayValue(value string, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}
