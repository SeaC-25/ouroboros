package app

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ouroboros/ouroboros/internal/compiler"
	"github.com/ouroboros/ouroboros/internal/eventlog"
	"github.com/ouroboros/ouroboros/internal/frontier"
	"github.com/ouroboros/ouroboros/internal/tui"
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
		if kind != "compile-preview" && kind != "compile-preview-review-artifact" && kind != "compile-preview-review-selection" && kind != "report-projection-dispatch-artifact" && kind != "report-projection-dispatch-execution-artifact" && kind != "report-projection-execution-selection" && kind != "replay-workbench-session-artifact" && kind != "replay-workbench-session-selection" && kind != "replay-workbench-collaboration-artifact" && kind != "replay-workbench-collaboration-selection" && kind != "replay-workbench-collaboration-note" && kind != "replay-workbench-collaboration-note-selection" && kind != "install-dispatch-artifact" && kind != "install-retry-dispatch-artifact" && kind != "worker-lifecycle-action-plan-artifact" && kind != "worker-lifecycle-action-plan-selection" && kind != "worker-lifecycle-policy-artifact" && kind != "worker-lifecycle-policy-selection" && kind != "worker-lease-result-reconciliation-artifact" && kind != "worker-lease-reclaim-artifact" && kind != "worker-placement-artifact" && kind != "worker-placement-dispatch-artifact" && kind != "worker-queue-claim-artifact" && kind != "worker-transport-request-artifact" && kind != "worker-transport-daemon-artifact" {
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
		WithRuntimeActionExecutor(tuiRuntimeActionExecutor(root))
}

func tuiModelWithWorkspaceRuntimeRows(root string, model tui.Model) tui.Model {
	model = tuiModelWithProviderUsageSummaryRows(root, model)
	model = tuiModelWithInstallRequestRows(root, model)
	model = tuiModelWithToolchainContractRows(root, model)
	model = tuiModelWithCompilePreviewSignalRows(root, model)
	model = tuiModelWithCompilePreviewReviewRows(root, model)
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

func tuiModelWithProviderUsageSummaryRows(root string, model tui.Model) tui.Model {
	summary, err := SummarizeProviderUsageRecords(root)
	if err != nil {
		return model.AppendRuntimeRows(tui.RuntimeTabModelRuntime, []string{
			"provider usage summary error: mode=metadata-only error=" + compactRuntimeRowValue(err.Error()),
		})
	}
	return model.AppendRuntimeRows(tui.RuntimeTabModelRuntime, providerUsageSummaryRuntimeRows(root, summary))
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
			"replay workbench session selection: status=%s mode=%s %s reason=%s primary=%s selected=%s runs=%d filters=%d endpoints=%d superseded=%d session=%s",
			installRequestDisplayValue(selection.Status, "current"),
			installRequestDisplayValue(selection.Mode, "metadata-only"),
			actorFields,
			reason,
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
			"replay workbench session artifact: status=%s mode=%s primary=%s selected=%s runs=%d filters=%d endpoints=%d created_by=%s created_at=%s reason=%s decision=%s reviewer=%s reviewed_at=%s review_reason=%s current=%s path=%s",
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
			decision,
			reviewer,
			reviewedAt,
			reviewReason,
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
			"report projection dispatch artifact: %s status=%s mode=%s run=%s events=%d planned_report=%s actor=%s created_at=%s reason=%s",
			dispatch.Path,
			status,
			mode,
			reportProjectionDispatchRowValue(dispatch.RunID, "unknown"),
			dispatch.EventCount,
			reportProjectionDispatchRowValue(dispatch.ReportPath, "not-generated"),
			actor,
			reportProjectionDispatchRowValue(dispatch.CreatedAt, "unknown"),
			reason,
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
		actorFields := fmt.Sprintf(
			"selected_by=%s selected_at=%s",
			reportProjectionDispatchRowValue(selection.SelectedBy, "unknown"),
			reportProjectionDispatchRowValue(selection.SelectedAt, "unknown"),
		)
		if strings.TrimSpace(selection.Status) == "cleared" {
			actorFields = fmt.Sprintf(
				"cleared_by=%s cleared_at=%s",
				reportProjectionDispatchRowValue(selection.ClearedBy, "unknown"),
				reportProjectionDispatchRowValue(selection.ClearedAt, "unknown"),
			)
		}
		isCurrent := strings.TrimSpace(selection.Status) == "current"
		rows = append(rows, fmt.Sprintf(
			"report projection execution selection: status=%s mode=%s current=%t %s reason=%s execution=%s selection=%s dispatch=%s run=%s report=%s written=%t events=%d superseded=%d",
			reportProjectionDispatchRowValue(selection.Status, "unknown"),
			reportProjectionDispatchRowValue(selection.Mode, "metadata-only"),
			isCurrent,
			actorFields,
			reportProjectionDispatchRowValue(selection.Reason, "none"),
			reportProjectionDispatchRowValue(selection.ExecutionPath, "unknown"),
			reportProjectionDispatchRowValue(selection.Path, "unknown"),
			reportProjectionDispatchRowValue(selection.DispatchPath, "unknown"),
			reportProjectionDispatchRowValue(selection.RunID, "unknown"),
			reportProjectionDispatchRowValue(selection.ReportPath, "not-generated"),
			selection.Written,
			selection.EventCount,
			len(selection.SupersededPaths),
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
		if execution.Annotation != nil {
			decision = reportProjectionDispatchRowValue(execution.Annotation.Decision, "unreviewed")
			reviewer = reportProjectionDispatchRowValue(execution.Annotation.Reviewer, "unknown")
			reviewedAt = reportProjectionDispatchRowValue(execution.Annotation.ReviewedAt, "unknown")
			reviewReason = reportProjectionDispatchRowValue(execution.Annotation.Reason, "none")
		}
		currentValue := "false"
		if selectionFound && strings.TrimSpace(selection.Status) == "current" && strings.TrimSpace(selection.ExecutionPath) == strings.TrimSpace(execution.Path) {
			currentValue = "true"
		}
		rows = append(rows, fmt.Sprintf(
			"report projection dispatch execution artifact: %s status=%s mode=%s dispatch=%s run=%s events=%d report=%s written=%t actor=%s executed_at=%s reason=%s decision=%s reviewer=%s reviewed_at=%s review_reason=%s current=%s",
			execution.Path,
			status,
			mode,
			reportProjectionDispatchRowValue(execution.DispatchPath, "unknown"),
			reportProjectionDispatchRowValue(execution.RunID, "unknown"),
			execution.EventCount,
			reportProjectionDispatchRowValue(execution.ReportPath, "not-generated"),
			execution.Written,
			actor,
			executedAt,
			reason,
			decision,
			reviewer,
			reviewedAt,
			reviewReason,
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
				"report projection execution review: mode=metadata-only decision=%s reviewer=%s reviewed_at=%s review_reason=%s execution=%s dispatch=%s run=%s report=%s review=%s",
				decision,
				reviewer,
				reviewedAt,
				reviewReason,
				reportProjectionDispatchRowValue(execution.Path, "unknown"),
				reportProjectionDispatchRowValue(execution.DispatchPath, "unknown"),
				reportProjectionDispatchRowValue(execution.RunID, "unknown"),
				reportProjectionDispatchRowValue(execution.ReportPath, "not-generated"),
				reportProjectionDispatchRowValue(execution.Path, "unknown"),
			))
			rows = append(rows, fmt.Sprintf(
				"report projection dispatch execution annotation: decision=%s reviewer=%s reviewed_at=%s reason=%s",
				decision,
				reviewer,
				reviewedAt,
				reviewReason,
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
		"report projection execution superseded:",
		"report projection dispatch execution artifact:",
		"report projection dispatch execution compact:",
		"report projection execution review:",
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
			"replay workbench collaboration selection: status=%s mode=%s collaboration=%s session=%s primary=%s selected=%s runs=%d filters=%d endpoints=%d participants=%d superseded=%d %s reason=%s",
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
			"replay workbench collaboration artifact: status=%s mode=%s primary=%s selected=%s runs=%d filters=%d endpoints=%d participants=%d created_by=%s created_at=%s reason=%s decision=%s reviewer=%s reviewed_at=%s review_reason=%s current=%s path=%s session_path=%s",
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
			decision,
			reviewer,
			reviewedAt,
			reviewReason,
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
			"replay workbench collaboration presence artifact: status=%s mode=%s primary=%s selected=%s participant=%s seen_by=%s seen_at=%s reason=%s path=%s collaboration=%s session_path=%s",
			status,
			installRequestDisplayValue(item.Mode, "metadata-only"),
			installRequestDisplayValue(item.PrimaryRunID, "unknown"),
			installRequestDisplayValue(item.SelectedRunID, "unknown"),
			participant,
			seenBy,
			seenAt,
			reason,
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
			"replay workbench collaboration note selection: status=%s mode=%s note=%s collaboration=%s session=%s primary=%s selected=%s author=%s target=%s target_kind=%s target_id=%s superseded=%d %s reason=%s",
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
			"replay workbench collaboration note artifact: status=%s mode=%s primary=%s selected=%s author=%s target=%s target_kind=%s target_id=%s created_at=%s reason=%s body=%s decision=%s reviewer=%s reviewed_at=%s review_reason=%s current=%s path=%s collaboration=%s session_path=%s",
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
			installRequestDisplayValue(strings.TrimSpace(note.Body), "none"),
			decision,
			reviewer,
			reviewedAt,
			reviewReason,
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

func compilePreviewReviewRuntimeRowsWithSelection(reviews []ToolCompilePreviewReview, selection ToolCompilePreviewReviewSelection, selectionFound bool) []string {
	rows := []string{fmt.Sprintf("compile-preview reviews: total=%d", len(reviews))}
	if selectionFound {
		actorFields := fmt.Sprintf(
			"selected_by=%s selected_at=%s",
			installRequestDisplayValue(selection.SelectedBy, "unknown"),
			installRequestDisplayValue(selection.SelectedAt, "unknown"),
		)
		if toolCompilePreviewReviewSelectionIsCleared(selection) {
			actorFields = fmt.Sprintf(
				"cleared_by=%s cleared_at=%s",
				installRequestDisplayValue(selection.ClearedBy, "unknown"),
				installRequestDisplayValue(selection.ClearedAt, "unknown"),
			)
		}
		rows = append(rows, fmt.Sprintf(
			"compile-preview review selection: status=%s mode=%s action=%s superseded=%d %s reason=%s review=%s",
			installRequestDisplayValue(selection.Status, "current"),
			installRequestDisplayValue(selection.Mode, "metadata-only"),
			installRequestDisplayValue(string(selection.Action), "unknown"),
			len(selection.SupersededPaths),
			actorFields,
			installRequestDisplayValue(strings.TrimSpace(selection.Reason), "none"),
			installRequestDisplayValue(selection.ReviewPath, "unknown"),
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
			reviewer = installRequestDisplayValue(review.Annotation.Reviewer, "unknown")
			reviewedAt = installRequestDisplayValue(review.Annotation.ReviewedAt, "unknown")
			reason = strings.TrimSpace(review.Annotation.Reason)
		}
		current := "false"
		if selectionFound && toolCompilePreviewReviewSelectionIsCurrent(selection) && strings.TrimSpace(selection.ReviewPath) == strings.TrimSpace(review.Path) {
			current = "true"
		}
		rows = append(rows, fmt.Sprintf(
			"compile-preview review artifact: status=%s mode=%s action=%s nodes=%d warnings=%d current=%s decision=%s reviewer=%s reason=%s reviewed_at=%s path=%s",
			installRequestDisplayValue(review.Status, "unknown"),
			installRequestDisplayValue(review.Mode, "metadata-only"),
			installRequestDisplayValue(string(review.Preview.Action), "unknown"),
			len(review.Preview.Nodes),
			len(review.Preview.Warnings),
			current,
			decision,
			reviewer,
			installRequestDisplayValue(reason, "none"),
			reviewedAt,
			installRequestDisplayValue(review.Path, "unknown"),
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
	rows := []string{fmt.Sprintf(
		"compile-preview review comparison: mode=%s action=%s node_delta=%d warning_delta=%d added=%d removed=%d changed=%d reordered=%d score_changed=%d signal_changed=%d base=%s candidate=%s",
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
		installRequestDisplayValue(comparison.Base.Path, "unknown"),
		installRequestDisplayValue(comparison.Candidate.Path, "unknown"),
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
			"worker orchestration artifact: %s status=%s mode=%s transport=%s placement_ready=%d placement_skipped=%d claimed=%d dispatch_ready=%d dispatch_skipped=%d dispatched=%d failed=%d retry_dispatch_executions_created=%d retry_dispatch_claims_created=%d placement_plan=%s placement_dispatch=%s placement_batch=%s lifecycle_maintenance=%s lifecycle_audit=%s",
			orchestration.Path,
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
			installRequestDisplayValue(orchestration.PlacementDispatchPlanPath, "none"),
			installRequestDisplayValue(orchestration.PlacementDispatchBatchPath, "none"),
			installRequestDisplayValue(orchestration.LifecycleMaintenancePath, "none"),
			installRequestDisplayValue(orchestration.LifecycleAuditPath, "none"),
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
			"worker orchestration supervisor artifact: %s status=%s mode=%s transport=%s runs=%d max_runs=%d orchestrations_created=%d lifecycle_maintenance=%d lifecycle_audit=%d%s claimed=%d dispatched=%d skipped=%d failed=%d orchestration_path=%s",
			supervisor.Path,
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
			"worker placement artifact: %s status=%s mode=%s ready=%d skipped=%d run=%s",
			plan.Path,
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
			"worker placement dispatch artifact: %s status=%s mode=%s transport=%s ready=%d skipped=%d",
			plan.Path,
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
				"worker queue claim artifact: %s status=%s mode=%s worker=%s run=%s execution=%s",
				claim.Path,
				installRequestDisplayValue(claim.Status, "unknown"),
				installRequestDisplayValue(claim.Mode, "unknown"),
				installRequestDisplayValue(claim.WorkerID, "unknown"),
				installRequestDisplayValue(claim.RunID, "unknown"),
				installRequestDisplayValue(claim.ExecutionID, "unknown"),
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
			"worker queue dispatch execution artifact: %s status=%s mode=%s transport=%s claim=%s worker=%s execution=%s",
			execution.Path,
			installRequestDisplayValue(execution.Status, "unknown"),
			installRequestDisplayValue(execution.Mode, "unknown"),
			installRequestDisplayValue(execution.TransportMode, "unknown"),
			installRequestDisplayValue(execution.ClaimPath, "unknown"),
			installRequestDisplayValue(execution.WorkerID, "unknown"),
			installRequestDisplayValue(execution.ExecutionID, "unknown"),
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
			"worker transport request: %s status=%s transport=%s dispatch=%s claim=%s worker=%s execution=%s",
			request.Path,
			installRequestDisplayValue(request.Status, "unknown"),
			installRequestDisplayValue(request.TransportMode, "unknown"),
			installRequestDisplayValue(request.DispatchExecutionPath, "unknown"),
			installRequestDisplayValue(request.ClaimPath, "unknown"),
			installRequestDisplayValue(request.WorkerID, "unknown"),
			installRequestDisplayValue(request.ExecutionID, "unknown"),
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
			"worker queue dispatch result artifact: %s status=%s mode=%s dispatch=%s worker=%s execution=%s",
			result.Path,
			installRequestDisplayValue(result.Status, "unknown"),
			installRequestDisplayValue(result.Mode, "unknown"),
			installRequestDisplayValue(result.DispatchExecutionPath, "unknown"),
			installRequestDisplayValue(result.WorkerID, "unknown"),
			installRequestDisplayValue(result.ExecutionID, "unknown"),
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
			"worker dispatch retry execution artifact: %s status=%s mode=%s retry_dispatch=%s claimed=%d dispatched=%d skipped=%d failed=%d",
			execution.Path,
			installRequestDisplayValue(execution.Status, "unknown"),
			installRequestDisplayValue(execution.Mode, "execute"),
			installRequestDisplayValue(execution.RetryDispatchPath, "none"),
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
				"worker dispatch retry execution result: source_result=%s source_dispatch=%s source_claim=%s retry_claim=%s queue_dispatch=%s request_path=%s transport=%s worker=%s run=%s execution=%s status=%s reason=%s",
				installRequestDisplayValue(result.SourceResultPath, "none"),
				installRequestDisplayValue(result.SourceDispatchPath, "none"),
				installRequestDisplayValue(result.SourceClaimPath, "none"),
				installRequestDisplayValue(result.RetryClaimPath, "none"),
				installRequestDisplayValue(result.RetryQueueDispatchPath, "none"),
				installRequestDisplayValue(result.RetryTransportRequestPath, "none"),
				installRequestDisplayValue(result.RetryTransportMode, "none"),
				installRequestDisplayValue(result.WorkerID, "none"),
				installRequestDisplayValue(result.RunID, "none"),
				installRequestDisplayValue(result.ExecutionID, "none"),
				installRequestDisplayValue(result.Status, "unknown"),
				installRequestDisplayValue(result.Reason, "none"),
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
			"worker dispatch retry scheduler run artifact: %s status=%s mode=%s max_runs=%d dispatch_retry_claims=%t retry_transport=%s retry_plans_created=%d retry_dispatches_created=%d retry_dispatch_executions_created=%d retry_dispatch_claims_created=%d retry_dispatch_claims_dispatched=%d retry_dispatch_transport_requests_created=%d skip=%s error=%s",
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
			errorValue,
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
				"worker lifecycle audit item: kind=%s status=%s mode=%s gate=%s worker=%s run=%s execution=%s path=%s linked=%s reason=%s%s",
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
			"selected_by=%s selected_at=%s",
			installRequestDisplayValue(selection.SelectedBy, "unknown"),
			installRequestDisplayValue(selection.SelectedAt, "unknown"),
		)
		if workerLifecyclePolicySelectionIsCleared(selection) {
			actorFields = fmt.Sprintf(
				"cleared_by=%s cleared_at=%s",
				installRequestDisplayValue(selection.ClearedBy, "unknown"),
				installRequestDisplayValue(selection.ClearedAt, "unknown"),
			)
		}
		rows = append(rows, fmt.Sprintf(
			"worker lifecycle policy selection: status=%s mode=%s policy=%s selection=%s transport=%s poll_interval_ms=%d max_runs=%d stale_after=%d worker_stale_after=%d include_lifecycle_maintenance=%t execute_lifecycle_retries=%t execute_lease_recovery=%t superseded=%d %s reason=%s",
			installRequestDisplayValue(selection.Status, "unknown"),
			installRequestDisplayValue(selection.Mode, "metadata-only"),
			installRequestDisplayValue(selection.PolicyPath, "none"),
			installRequestDisplayValue(selection.Path, "none"),
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
			"worker lifecycle policy artifact: %s status=%s mode=%s transport=%s stale_after=%d worker_stale_after=%d poll_interval_ms=%d max_runs=%d lifecycle_maintenance=%t lifecycle_retries=%t lease_recovery=%t current=%s workers=%d stale_workers=%d stale_leases=%d",
			policy.Path,
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
			"worker lifecycle policy run artifact: %s status=%s mode=%s policy=%s selection=%s max_runs=%d poll_interval_ms=%d transport=%s include_lifecycle_maintenance=%t execute_lifecycle_retries=%t execute_lease_recovery=%t supervisor_created=%d skip=%s",
			run.Path,
			status,
			mode,
			policyPath,
			selectionPath,
			run.MaxRuns,
			run.PollIntervalMilliseconds,
			transport,
			run.IncludeLifecycleMaintenance,
			run.ExecuteLifecycleRetries,
			run.ExecuteLeaseRecovery,
			supervisorCreated,
			skipReason,
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
		skipReason := installRequestDisplayValue(compactRuntimeRowValue(workerLifecyclePolicySchedulerRunValue(payload, "skip_reason", "skip")), "none")
		errorValue := installRequestDisplayValue(compactRuntimeRowValue(workerLifecyclePolicySchedulerRunValue(payload, "error")), "none")
		rows = append(rows, fmt.Sprintf(
			"worker lifecycle policy scheduler run artifact: %s status=%s mode=%s policy=%s selection=%s policy_basename=%s selection_basename=%s max_runs=%d poll_interval_ms=%d transport=%s include_lifecycle_maintenance=%s execute_lifecycle_retries=%s execute_lease_recovery=%s policy_runs_created=%d supervisor_created=%s skip=%s error=%s",
			path,
			status,
			mode,
			policyPath,
			selectionPath,
			policyBase,
			selectionBase,
			maxRuns,
			pollInterval,
			transport,
			includeLifecycleMaintenance,
			executeLifecycleRetries,
			executeLeaseRecovery,
			policyRunsCreated,
			supervisorCreated,
			skipReason,
			errorValue,
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
			"selected_by=%s selected_at=%s",
			reportProjectionDispatchRowValue(selection.SelectedBy, "unknown"),
			reportProjectionDispatchRowValue(selection.SelectedAt, "unknown"),
		)
		if strings.TrimSpace(selection.Status) == "cleared" {
			actorFields = fmt.Sprintf(
				"cleared_by=%s cleared_at=%s",
				reportProjectionDispatchRowValue(selection.ClearedBy, "unknown"),
				reportProjectionDispatchRowValue(selection.ClearedAt, "unknown"),
			)
		}
		isCurrent := strings.TrimSpace(selection.Status) == "current"
		summary := selection.Summary
		rows = append(rows, fmt.Sprintf(
			"worker lifecycle action plan selection: status=%s mode=%s current=%t %s reason=%s plan=%s selection=%s review=%s actions=%d ack=%d run=%d daemon_control=%d retry_dispatch_execute=%d reclaim=%d reconcile=%d superseded=%d",
			reportProjectionDispatchRowValue(selection.Status, "unknown"),
			reportProjectionDispatchRowValue(selection.Mode, "metadata-only"),
			isCurrent,
			actorFields,
			reportProjectionDispatchRowValue(selection.Reason, "none"),
			reportProjectionDispatchRowValue(selection.PlanPath, "unknown"),
			reportProjectionDispatchRowValue(selection.Path, "unknown"),
			reportProjectionDispatchRowValue(selection.ReviewPath, "none"),
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
		reviewedAt := "unknown"
		reviewReason := "none"
		if plan.Annotation != nil {
			decision = reportProjectionDispatchRowValue(plan.Annotation.Decision, "unreviewed")
			reviewer = reportProjectionDispatchRowValue(plan.Annotation.Reviewer, "unknown")
			reviewedAt = reportProjectionDispatchRowValue(plan.Annotation.ReviewedAt, "unknown")
			reviewReason = reportProjectionDispatchRowValue(plan.Annotation.Reason, "none")
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
			"worker lifecycle action plan artifact: %s status=%s mode=%s actions=%d ack=%d run=%d reclaim=%d reconcile=%d daemon_control=%d retry_dispatch_execute=%d review=%s decision=%s reviewer=%s reviewed_at=%s review_reason=%s current=%s",
			plan.Path,
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
			decision,
			reviewer,
			reviewedAt,
			reviewReason,
			currentValue,
		))
		rows = append(rows, fmt.Sprintf(
			"worker lifecycle action plan artifact state: %s mode=%s actions=%d",
			installRequestDisplayValue(plan.Status, "unknown"),
			installRequestDisplayValue(plan.Mode, "unknown"),
			summary.Total,
		))
		rows = append(rows, fmt.Sprintf(
			"worker lifecycle action plan artifact review: current=%s decision=%s reviewer=%s reviewed_at=%s review_reason=%s",
			currentValue,
			decision,
			reviewer,
			reviewedAt,
			reviewReason,
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
				"worker lifecycle action plan review: mode=metadata-only decision=%s reviewer=%s reviewed_at=%s review_reason=%s plan=%s review=%s",
				decision,
				reviewer,
				reviewedAt,
				reviewReason,
				reportProjectionDispatchRowValue(plan.Path, "unknown"),
				reportProjectionDispatchRowValue(plan.Path, "unknown"),
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
				"worker lifecycle action: type=%s gate=%s worker=%s run=%s execution=%s status=%s mode=%s reason=%s request_path=%s lease_path=%s result_path=%s dispatch=%s recommended=%s",
				installRequestDisplayValue(action.Type, "unknown"),
				installRequestDisplayValue(action.Gate, "unknown"),
				installRequestDisplayValue(action.WorkerID, "unknown"),
				installRequestDisplayValue(action.RunID, "unknown"),
				installRequestDisplayValue(action.ExecutionID, "unknown"),
				installRequestDisplayValue(action.Status, "unknown"),
				installRequestDisplayValue(action.Mode, "metadata-only"),
				installRequestDisplayValue(action.Reason, "none"),
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
			"worker lease result reconciliation artifact: %s status=%s mode=%s ready=%d skipped=%d",
			dispatch.Path,
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
			"worker lease result reconciliation execution artifact: %s status=%s mode=%s results=%d skipped=%d",
			execution.Path,
			installRequestDisplayValue(execution.Status, "unknown"),
			installRequestDisplayValue(execution.Mode, "unknown"),
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
				"lease result reconciliation execution result run=%s execution=%s result=%s lease=%s worker=%s",
				installRequestDisplayValue(result.RunID, "unknown"),
				installRequestDisplayValue(result.ExecutionID, "unknown"),
				installRequestDisplayValue(result.ResultStatus, "unknown"),
				installRequestDisplayValue(result.Status, "unknown"),
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
			"worker lease reclaim artifact: %s status=%s mode=%s ready=%d skipped=%d",
			dispatch.Path,
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
			"worker lease reclaim execution artifact: %s status=%s mode=%s dispatch=%s results=%d skipped=%d",
			execution.Path,
			installRequestDisplayValue(execution.Status, "unknown"),
			installRequestDisplayValue(execution.Mode, "unknown"),
			execution.DispatchPath,
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
				"worker lease reclaim execution result run=%s execution=%s status=%s worker=%s tool=%s capability=%s",
				result.RunID,
				result.ExecutionID,
				installRequestDisplayValue(result.Status, "unknown"),
				installRequestDisplayValue(result.WorkerID, "unknown"),
				installRequestDisplayValue(result.Tool, "unknown"),
				installRequestDisplayValue(result.Capability, "unknown"),
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
			"dispatch artifact: %s status=%s mode=%s ready=%d skipped=%d",
			dispatch.Path,
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
			"install orchestration artifact: %s status=%s mode=%s ready=%d skipped=%d results=%d dispatch=%s execution=%s retry_dispatch=%s error=%s",
			orchestration.Path,
			installRequestDisplayValue(orchestration.Status, "unknown"),
			installRequestDisplayValue(orchestration.Mode, "unknown"),
			orchestration.DispatchReady,
			orchestration.DispatchSkipped,
			orchestration.ExecutionResults,
			installRequestDisplayValue(orchestration.DispatchPath, "none"),
			installRequestDisplayValue(orchestration.ExecutionPath, "none"),
			installRequestDisplayValue(orchestration.RetryDispatchPath, "none"),
			compactRuntimeRowValue(orchestration.Error),
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
			"execution artifact: %s status=%s mode=%s dispatch=%s results=%d skipped=%d",
			execution.Path,
			installRequestDisplayValue(execution.Status, "unknown"),
			installRequestDisplayValue(execution.Mode, "unknown"),
			execution.DispatchPath,
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
			rows = append(rows, fmt.Sprintf(
				"execution result %s/%s: %s artifact=%s dispatch=%s command=%s output=%s",
				result.Pack,
				result.Dependency,
				installRequestDisplayValue(result.Status, "unknown"),
				execution.Path,
				execution.DispatchPath,
				strings.Join(result.Command, " "),
				compactRuntimeRowValue(result.Output),
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
			"retry dispatch artifact: %s status=%s mode=%s ready=%d skipped=%d",
			dispatch.Path,
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
			"retry execution artifact: %s status=%s mode=%s retry_dispatch=%s results=%d skipped=%d",
			execution.Path,
			installRequestDisplayValue(execution.Status, "unknown"),
			installRequestDisplayValue(execution.Mode, "unknown"),
			execution.RetryDispatchPath,
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
			rows = append(rows, fmt.Sprintf(
				"retry execution result %s/%s: %s output=%s error=%s artifact=%s retry_dispatch=%s source_execution=%s dispatch=%s command=%s",
				result.Pack,
				result.Dependency,
				installRequestDisplayValue(result.Status, "unknown"),
				compactRuntimeRowValue(result.Output),
				compactRuntimeRowValue(result.Error),
				execution.Path,
				execution.RetryDispatchPath,
				result.ExecutionPath,
				result.DispatchPath,
				strings.Join(result.Command, " "),
			))
			rows = append(rows, fmt.Sprintf(
				"retry execution result refs summary %s/%s: source_execution=%s dispatch=%s artifact=%s retry_dispatch=%s",
				result.Pack,
				result.Dependency,
				filepath.Base(result.ExecutionPath),
				filepath.Base(result.DispatchPath),
				filepath.Base(execution.Path),
				filepath.Base(execution.RetryDispatchPath),
			))
			rows = append(rows, fmt.Sprintf(
				"retry execution result refs %s/%s: source_execution=%s dispatch=%s artifact=%s retry_dispatch=%s command=%s",
				result.Pack,
				result.Dependency,
				result.ExecutionPath,
				result.DispatchPath,
				execution.Path,
				execution.RetryDispatchPath,
				strings.Join(result.Command, " "),
			))
		}
	}
	return rows
}

func compactRuntimeRowValue(value string) string {
	return strings.Join(strings.Fields(value), " ")
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
