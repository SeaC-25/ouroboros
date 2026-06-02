package app

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type WorkerLifecycleMaintenanceOptions struct {
	Execute                    bool
	ExecuteRetryDispatches     bool
	DispatchRetryClaims        bool
	RetryDispatchTransportMode string
	ExecuteLeaseRecovery       bool
	Now                        time.Time
	StaleAfter                 time.Duration
}

type WorkerLifecycleMaintenanceSummary struct {
	ActionsTotal                           int `json:"actions_total"`
	AckActions                             int `json:"ack_actions"`
	RunActions                             int `json:"run_actions"`
	DaemonControlActions                   int `json:"daemon_control_actions"`
	RetryDispatchExecuteActions            int `json:"retry_dispatch_execute_actions"`
	ReclaimActions                         int `json:"reclaim_actions"`
	ReconciliationActions                  int `json:"reconciliation_actions"`
	SkippedUnsafeActions                   int `json:"skipped_unsafe_actions"`
	ReviewArtifactsCreated                 int `json:"review_artifacts_created"`
	ActionPlanArtifactsCreated             int `json:"action_plan_artifacts_created"`
	ReclaimDispatchArtifactsCreated        int `json:"reclaim_dispatch_artifacts_created"`
	ReconciliationDispatchArtifactsCreated int `json:"reconciliation_dispatch_artifacts_created"`
	RetryDispatchExecutionsCreated         int `json:"retry_dispatch_executions_created"`
	RetryDispatchClaimsCreated             int `json:"retry_dispatch_claims_created"`
	RetryDispatchClaimsDispatched          int `json:"retry_dispatch_claims_dispatched,omitempty"`
	RetryDispatchTransportRequestsCreated  int `json:"retry_dispatch_transport_requests_created,omitempty"`
	ReclaimExecutionsCreated               int `json:"reclaim_executions_created"`
	ReclaimResultsCreated                  int `json:"reclaim_results_created"`
	ReconciliationExecutionsCreated        int `json:"reconciliation_executions_created"`
	ReconciliationResultsCreated           int `json:"reconciliation_results_created"`
}

type WorkerLifecycleMaintenanceSkippedAction struct {
	Type         string `json:"type"`
	Gate         string `json:"gate"`
	Reason       string `json:"reason"`
	WorkerID     string `json:"worker_id,omitempty"`
	RunID        string `json:"run_id,omitempty"`
	ExecutionID  string `json:"execution_id,omitempty"`
	RequestPath  string `json:"request_path,omitempty"`
	LeasePath    string `json:"lease_path,omitempty"`
	ResultPath   string `json:"result_path,omitempty"`
	DispatchPath string `json:"dispatch_path,omitempty"`
}

type WorkerLifecycleMaintenance struct {
	Version                     int                                       `json:"version"`
	Path                        string                                    `json:"path,omitempty"`
	Status                      string                                    `json:"status"`
	Mode                        string                                    `json:"mode"`
	CreatedAt                   string                                    `json:"created_at"`
	StaleAfterSeconds           int64                                     `json:"stale_after_seconds"`
	ReviewPath                  string                                    `json:"review_path,omitempty"`
	ActionPlanPath              string                                    `json:"action_plan_path,omitempty"`
	ReclaimDispatchPath         string                                    `json:"reclaim_dispatch_path,omitempty"`
	ReconciliationDispatchPath  string                                    `json:"reconciliation_dispatch_path,omitempty"`
	ReclaimExecutionPath        string                                    `json:"reclaim_execution_path,omitempty"`
	ReconciliationExecutionPath string                                    `json:"reconciliation_execution_path,omitempty"`
	RetryDispatchExecutionPaths []string                                  `json:"retry_dispatch_execution_paths,omitempty"`
	DispatchRetryClaims         bool                                      `json:"dispatch_retry_claims,omitempty"`
	RetryDispatchTransportMode  string                                    `json:"retry_dispatch_transport_mode,omitempty"`
	ExecuteLeaseRecovery        bool                                      `json:"execute_lease_recovery,omitempty"`
	Summary                     WorkerLifecycleMaintenanceSummary         `json:"summary"`
	SkippedActions              []WorkerLifecycleMaintenanceSkippedAction `json:"skipped_actions,omitempty"`
	Error                       string                                    `json:"error,omitempty"`
}

func CreateWorkerLifecycleMaintenance(root string, options WorkerLifecycleMaintenanceOptions) (WorkerLifecycleMaintenance, error) {
	now := options.Now.UTC()
	if now.IsZero() {
		now = time.Now().UTC()
	}
	staleAfter := options.StaleAfter
	if staleAfter <= 0 {
		staleAfter = DefaultWorkerLeaseStaleAfter
	}
	retryDispatchTransportMode, err := normalizeWorkerLifecycleRetryHandoffOptions("worker lifecycle maintenance", options.Execute, options.ExecuteRetryDispatches, options.DispatchRetryClaims, options.RetryDispatchTransportMode)
	if err != nil {
		return WorkerLifecycleMaintenance{}, err
	}
	preview, err := PlanWorkerLifecycleActionPlan(root, "", now, staleAfter)
	if err != nil {
		return WorkerLifecycleMaintenance{}, err
	}
	maintenance := WorkerLifecycleMaintenance{
		Version:                    1,
		Status:                     "dry-run",
		Mode:                       "metadata-only",
		CreatedAt:                  now.Format(time.RFC3339Nano),
		StaleAfterSeconds:          int64(staleAfter / time.Second),
		DispatchRetryClaims:        options.DispatchRetryClaims,
		RetryDispatchTransportMode: retryDispatchTransportMode,
		ExecuteLeaseRecovery:       options.ExecuteLeaseRecovery,
		Summary:                    workerLifecycleMaintenanceSummaryFromActionPlan(preview),
		SkippedActions:             workerLifecycleMaintenanceSkippedUnsafeActions(preview.Actions, options.ExecuteRetryDispatches),
	}
	maintenance.Summary.SkippedUnsafeActions = len(maintenance.SkippedActions)
	if !options.Execute {
		if err := writeWorkerLifecycleMaintenance(root, &maintenance); err != nil {
			return maintenance, err
		}
		return maintenance, nil
	}

	maintenance.Status = "completed"
	review, err := CreateWorkerLifecycleReview(root, now, staleAfter)
	if err != nil {
		return failWorkerLifecycleMaintenance(root, maintenance, err)
	}
	maintenance.ReviewPath = review.Path
	maintenance.Summary.ReviewArtifactsCreated = 1

	actionPlan, err := CreateWorkerLifecycleActionPlan(root, filepath.Base(review.Path), now, staleAfter)
	if err != nil {
		return failWorkerLifecycleMaintenance(root, maintenance, err)
	}
	maintenance.ActionPlanPath = actionPlan.Path
	maintenance.Summary = workerLifecycleMaintenanceSummaryFromActionPlan(actionPlan)
	maintenance.Summary.ReviewArtifactsCreated = 1
	maintenance.Summary.ActionPlanArtifactsCreated = 1
	maintenance.SkippedActions = workerLifecycleMaintenanceSkippedUnsafeActions(actionPlan.Actions, options.ExecuteRetryDispatches)
	maintenance.Summary.SkippedUnsafeActions = len(maintenance.SkippedActions)

	if actionPlan.Summary.ReclaimStaleLease > 0 {
		dispatch, err := CreateWorkerLeaseReclaimDispatch(root, now, staleAfter)
		if err != nil {
			return failWorkerLifecycleMaintenance(root, maintenance, err)
		}
		maintenance.ReclaimDispatchPath = dispatch.Path
		maintenance.Summary.ReclaimDispatchArtifactsCreated = 1
	}
	if actionPlan.Summary.ReconcileResultLease > 0 {
		dispatch, err := CreateWorkerLeaseResultReconciliationDispatch(root, now)
		if err != nil {
			return failWorkerLifecycleMaintenance(root, maintenance, err)
		}
		maintenance.ReconciliationDispatchPath = dispatch.Path
		maintenance.Summary.ReconciliationDispatchArtifactsCreated = 1
	}
	if options.ExecuteLeaseRecovery {
		if maintenance.ReclaimDispatchPath != "" {
			execution, err := ExecuteWorkerLeaseReclaimDispatch(root, maintenance.ReclaimDispatchPath, WorkerLeaseReclaimExecutionOptions{
				Execute:    true,
				Now:        now.Add(time.Nanosecond),
				StaleAfter: staleAfter,
			})
			if err != nil {
				return failWorkerLifecycleMaintenance(root, maintenance, err)
			}
			maintenance.ReclaimExecutionPath = execution.Path
			maintenance.Summary.ReclaimExecutionsCreated = 1
			maintenance.Summary.ReclaimResultsCreated = len(execution.Results)
		}
		if maintenance.ReconciliationDispatchPath != "" {
			execution, err := ExecuteWorkerLeaseResultReconciliationDispatch(root, maintenance.ReconciliationDispatchPath, WorkerLeaseResultReconciliationExecutionOptions{
				Execute:    true,
				Now:        now.Add(2 * time.Nanosecond),
				ReleasedBy: "worker-lifecycle-maintenance",
				Reason:     "worker lifecycle maintenance lease recovery",
			})
			if err != nil {
				return failWorkerLifecycleMaintenance(root, maintenance, err)
			}
			maintenance.ReconciliationExecutionPath = execution.Path
			maintenance.Summary.ReconciliationExecutionsCreated = 1
			maintenance.Summary.ReconciliationResultsCreated = len(execution.Results)
		}
	}
	if options.ExecuteRetryDispatches {
		for index, action := range actionPlan.Actions {
			if strings.TrimSpace(action.Type) != "execute-dispatch-retry-dispatch" {
				continue
			}
			execution, err := ExecuteRuntimeWorkerDispatchRetryDispatch(root, action.DispatchPath, RuntimeWorkerDispatchRetryDispatchExecutionOptions{
				Execute:        true,
				Now:            now.Add(time.Duration(index) * time.Nanosecond),
				DispatchClaims: options.DispatchRetryClaims,
				TransportMode:  retryDispatchTransportMode,
			})
			if err != nil {
				return failWorkerLifecycleMaintenance(root, maintenance, err)
			}
			maintenance.RetryDispatchExecutionPaths = append(maintenance.RetryDispatchExecutionPaths, execution.Path)
			maintenance.Summary.RetryDispatchExecutionsCreated++
			maintenance.Summary.RetryDispatchClaimsCreated += execution.Summary.Claimed
			maintenance.Summary.RetryDispatchClaimsDispatched += execution.Summary.Dispatched
			maintenance.Summary.RetryDispatchTransportRequestsCreated += runtimeWorkerDispatchRetryTransportRequestsCreated(execution.Results)
		}
	}
	if err := writeWorkerLifecycleMaintenance(root, &maintenance); err != nil {
		return maintenance, err
	}
	return maintenance, nil
}

func ListWorkerLifecycleMaintenances(root string) ([]WorkerLifecycleMaintenance, error) {
	paths, err := filepath.Glob(filepath.Join(workerLifecycleMaintenanceDir(root), "*.json"))
	if err != nil {
		return nil, fmt.Errorf("glob worker lifecycle maintenance artifacts: %w", err)
	}
	sort.Strings(paths)
	maintenances := make([]WorkerLifecycleMaintenance, 0, len(paths))
	for _, path := range paths {
		maintenance, err := readWorkerLifecycleMaintenanceFile(path)
		if err != nil {
			return nil, err
		}
		maintenances = append(maintenances, maintenance)
	}
	return maintenances, nil
}

func ReadWorkerLifecycleMaintenance(root, maintenancePath string) (WorkerLifecycleMaintenance, error) {
	path, err := resolveWorkerLifecycleMaintenancePath(root, maintenancePath)
	if err != nil {
		return WorkerLifecycleMaintenance{}, err
	}
	return readWorkerLifecycleMaintenanceFile(path)
}

func failWorkerLifecycleMaintenance(root string, maintenance WorkerLifecycleMaintenance, err error) (WorkerLifecycleMaintenance, error) {
	maintenance.Status = "failed"
	maintenance.Error = err.Error()
	if writeErr := writeWorkerLifecycleMaintenance(root, &maintenance); writeErr != nil {
		return maintenance, writeErr
	}
	return maintenance, err
}

func workerLifecycleMaintenanceSummaryFromActionPlan(plan WorkerLifecycleActionPlan) WorkerLifecycleMaintenanceSummary {
	return WorkerLifecycleMaintenanceSummary{
		ActionsTotal:                plan.Summary.Total,
		AckActions:                  plan.Summary.AckPendingTransport,
		RunActions:                  plan.Summary.RunAcknowledgedTransport,
		DaemonControlActions:        plan.Summary.ApplyDaemonControl,
		RetryDispatchExecuteActions: plan.Summary.ExecuteDispatchRetryDispatch,
		ReclaimActions:              plan.Summary.ReclaimStaleLease,
		ReconciliationActions:       plan.Summary.ReconcileResultLease,
	}
}

func normalizeWorkerLifecycleRetryHandoffOptions(label string, execute bool, executeRetryDispatches bool, dispatchRetryClaims bool, transportMode string) (string, error) {
	if !dispatchRetryClaims {
		return "", nil
	}
	if !execute {
		return "", fmt.Errorf("%s retry-dispatch handoff requires explicit execute", label)
	}
	if !executeRetryDispatches {
		return "", fmt.Errorf("%s retry-dispatch handoff requires retry-dispatch execution", label)
	}
	normalized, err := normalizeWorkerLifecycleRetryTransportMode(transportMode)
	if err != nil {
		return "", err
	}
	return normalized, nil
}

func normalizeWorkerLifecycleRetryTransportMode(transportMode string) (string, error) {
	trimmed := strings.TrimSpace(transportMode)
	if trimmed == "" {
		return "metadata-only", nil
	}
	switch trimmed {
	case "metadata-only", "file-outbox":
		return trimmed, nil
	case "http-json":
		return "", fmt.Errorf("worker lifecycle retry-dispatch handoff supports metadata-only or file-outbox transport")
	default:
		return "", fmt.Errorf("unsupported worker lifecycle retry-dispatch handoff transport: %s", transportMode)
	}
}

func runtimeWorkerDispatchRetryTransportRequestsCreated(results []RuntimeWorkerDispatchRetryDispatchExecutionResult) int {
	created := 0
	for _, result := range results {
		if strings.TrimSpace(result.RetryTransportRequestPath) != "" {
			created++
		}
	}
	return created
}

func workerLifecycleMaintenanceSkippedUnsafeActions(actions []WorkerLifecycleAction, executeRetryDispatches bool) []WorkerLifecycleMaintenanceSkippedAction {
	skipped := []WorkerLifecycleMaintenanceSkippedAction{}
	for _, action := range actions {
		switch strings.TrimSpace(action.Type) {
		case "ack-transport-request", "run-transport-request", "apply-daemon-control":
			skipped = append(skipped, WorkerLifecycleMaintenanceSkippedAction{
				Type:         action.Type,
				Gate:         action.Gate,
				Reason:       "requires-explicit-operator-gate",
				WorkerID:     action.WorkerID,
				RunID:        action.RunID,
				ExecutionID:  action.ExecutionID,
				RequestPath:  action.RequestPath,
				LeasePath:    action.LeasePath,
				ResultPath:   action.ResultPath,
				DispatchPath: action.DispatchPath,
			})
		case "execute-dispatch-retry-dispatch":
			if executeRetryDispatches {
				continue
			}
			skipped = append(skipped, WorkerLifecycleMaintenanceSkippedAction{
				Type:         action.Type,
				Gate:         action.Gate,
				Reason:       "requires-explicit-operator-gate",
				WorkerID:     action.WorkerID,
				RunID:        action.RunID,
				ExecutionID:  action.ExecutionID,
				RequestPath:  action.RequestPath,
				LeasePath:    action.LeasePath,
				ResultPath:   action.ResultPath,
				DispatchPath: action.DispatchPath,
			})
		}
	}
	return skipped
}

func writeWorkerLifecycleMaintenance(root string, maintenance *WorkerLifecycleMaintenance) error {
	if err := os.MkdirAll(workerLifecycleMaintenanceDir(root), 0o755); err != nil {
		return fmt.Errorf("create worker lifecycle maintenance directory: %w", err)
	}
	if strings.TrimSpace(maintenance.Path) == "" {
		maintenance.Path = filepath.Join(workerLifecycleMaintenanceDir(root), safeWorkerLifecycleMaintenanceID(*maintenance)+".json")
	}
	data, err := json.MarshalIndent(maintenance, "", "  ")
	if err != nil {
		return fmt.Errorf("encode worker lifecycle maintenance: %w", err)
	}
	data = append(data, '\n')
	tmp := maintenance.Path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("write worker lifecycle maintenance: %w", err)
	}
	if err := os.Rename(tmp, maintenance.Path); err != nil {
		return fmt.Errorf("publish worker lifecycle maintenance: %w", err)
	}
	return nil
}

func readWorkerLifecycleMaintenanceFile(path string) (WorkerLifecycleMaintenance, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return WorkerLifecycleMaintenance{}, fmt.Errorf("read worker lifecycle maintenance %s: %w", path, err)
	}
	var maintenance WorkerLifecycleMaintenance
	if err := json.Unmarshal(data, &maintenance); err != nil {
		return WorkerLifecycleMaintenance{}, fmt.Errorf("decode worker lifecycle maintenance %s: %w", path, err)
	}
	maintenance.Path = path
	return maintenance, nil
}

func resolveWorkerLifecycleMaintenancePath(root, maintenancePath string) (string, error) {
	trimmed := strings.TrimSpace(maintenancePath)
	if trimmed == "" {
		return "", fmt.Errorf("worker lifecycle maintenance path is required")
	}
	dir, err := filepath.Abs(workerLifecycleMaintenanceDir(root))
	if err != nil {
		return "", fmt.Errorf("resolve worker lifecycle maintenance directory: %w", err)
	}
	candidate := trimmed
	if !filepath.IsAbs(candidate) {
		if filepath.Ext(candidate) == "" && !strings.Contains(candidate, string(os.PathSeparator)) {
			candidate += ".json"
		}
		candidate = filepath.Join(dir, candidate)
	}
	candidate, err = filepath.Abs(candidate)
	if err != nil {
		return "", fmt.Errorf("resolve worker lifecycle maintenance path: %w", err)
	}
	rel, err := filepath.Rel(dir, candidate)
	if err != nil {
		return "", fmt.Errorf("resolve worker lifecycle maintenance relative path: %w", err)
	}
	if rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("worker lifecycle maintenance path must stay under %s", dir)
	}
	if filepath.Ext(candidate) != ".json" {
		return "", fmt.Errorf("worker lifecycle maintenance path must be a json file")
	}
	return candidate, nil
}

func workerLifecycleMaintenanceDir(root string) string {
	return filepath.Join(root, "runtime", "worker-lifecycle-maintenance")
}

func safeWorkerLifecycleMaintenanceID(maintenance WorkerLifecycleMaintenance) string {
	return safeInstallRequestName(strings.Join([]string{
		"worker-lifecycle-maintenance",
		maintenance.Status,
		maintenance.Mode,
		maintenance.CreatedAt,
		fmt.Sprintf("actions-%d", maintenance.Summary.ActionsTotal),
		fmt.Sprintf("reclaim-%d", maintenance.Summary.ReclaimActions),
		fmt.Sprintf("reconcile-%d", maintenance.Summary.ReconciliationActions),
		fmt.Sprintf("lease-recovery=%t", maintenance.ExecuteLeaseRecovery),
		fmt.Sprintf("reclaim-executions-%d", maintenance.Summary.ReclaimExecutionsCreated),
		fmt.Sprintf("reconcile-executions-%d", maintenance.Summary.ReconciliationExecutionsCreated),
	}, "|"))
}
