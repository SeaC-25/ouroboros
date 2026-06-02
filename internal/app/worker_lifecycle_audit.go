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

type WorkerLifecycleAuditSummary struct {
	ArtifactTotal                   int `json:"artifact_total"`
	WorkerRegistrations             int `json:"worker_registrations"`
	StaleWorkerRegistrations        int `json:"stale_worker_registrations"`
	OfflineWorkerRegistrations      int `json:"offline_worker_registrations"`
	PlacementPlans                  int `json:"placement_plans"`
	PlacementClaimExecutions        int `json:"placement_claim_executions"`
	PlacementDispatchPlans          int `json:"placement_dispatch_plans"`
	PlacementDispatchExecutions     int `json:"placement_dispatch_executions"`
	PlacementDispatchBatches        int `json:"placement_dispatch_batches"`
	WorkerOrchestrations            int `json:"worker_orchestrations"`
	WorkerOrchestrationSupervisors  int `json:"worker_orchestration_supervisors"`
	QueueClaims                     int `json:"queue_claims"`
	QueueDispatchExecutions         int `json:"queue_dispatch_executions"`
	TransportRequests               int `json:"transport_requests"`
	TransportRequestsPending        int `json:"transport_requests_pending"`
	TransportRequestsAcknowledged   int `json:"transport_requests_acknowledged"`
	TransportRequestsCompleted      int `json:"transport_requests_completed"`
	TransportRequestsFailed         int `json:"transport_requests_failed"`
	QueueDispatchResults            int `json:"queue_dispatch_results"`
	DispatchRetryPlans              int `json:"dispatch_retry_plans"`
	DispatchRetryDispatches         int `json:"dispatch_retry_dispatches"`
	DispatchRetryDispatchExecutions int `json:"dispatch_retry_dispatch_executions"`
	WorkerLeases                    int `json:"worker_leases"`
	ActiveLeases                    int `json:"active_leases"`
	StaleActiveLeases               int `json:"stale_active_leases"`
	ReclaimReviews                  int `json:"reclaim_reviews"`
	ReclaimExecutions               int `json:"reclaim_executions"`
	ReconciliationReviews           int `json:"reconciliation_reviews"`
	ReconciliationExecutions        int `json:"reconciliation_executions"`
	LifecycleReviews                int `json:"lifecycle_reviews"`
	ActionPlans                     int `json:"action_plans"`
	ActionPlanAnnotations           int `json:"action_plan_annotations"`
	ActionPlanSelections            int `json:"action_plan_selections"`
	ActionPlanActions               int `json:"action_plan_actions"`
	LifecyclePolicies               int `json:"lifecycle_policies"`
	LifecyclePolicySelections       int `json:"lifecycle_policy_selections"`
	LifecyclePolicyRuns             int `json:"lifecycle_policy_runs"`
	LifecyclePolicySchedulerRuns    int `json:"lifecycle_policy_scheduler_runs"`
	LifecycleMaintenances           int `json:"lifecycle_maintenances"`
	LifecycleMaintenanceSupervisors int `json:"lifecycle_maintenance_supervisors"`
	DaemonRuns                      int `json:"daemon_runs"`
	DaemonSupervisorRuns            int `json:"daemon_supervisor_runs"`
	DaemonControls                  int `json:"daemon_controls"`
}

type WorkerLifecycleAuditItem struct {
	Kind                        string `json:"kind"`
	Status                      string `json:"status,omitempty"`
	Mode                        string `json:"mode,omitempty"`
	Gate                        string `json:"gate,omitempty"`
	WorkerID                    string `json:"worker_id,omitempty"`
	RunID                       string `json:"run_id,omitempty"`
	ExecutionID                 string `json:"execution_id,omitempty"`
	ArtifactPath                string `json:"artifact_path,omitempty"`
	LinkedPath                  string `json:"linked_path,omitempty"`
	Reason                      string `json:"reason,omitempty"`
	Decision                    string `json:"decision,omitempty"`
	Reviewer                    string `json:"reviewer,omitempty"`
	ReviewedAt                  string `json:"reviewed_at,omitempty"`
	SelectedBy                  string `json:"selected_by,omitempty"`
	SelectedAt                  string `json:"selected_at,omitempty"`
	ClearedBy                   string `json:"cleared_by,omitempty"`
	ClearedAt                   string `json:"cleared_at,omitempty"`
	SupersededCount             int    `json:"superseded_count,omitempty"`
	TotalCount                  int    `json:"total_count,omitempty"`
	ReadyCount                  int    `json:"ready_count,omitempty"`
	ResultCount                 int    `json:"result_count,omitempty"`
	ActionsTotal                int    `json:"actions_total,omitempty"`
	AckActions                  int    `json:"ack_actions,omitempty"`
	RunActions                  int    `json:"run_actions,omitempty"`
	DaemonControlActions        int    `json:"daemon_control_actions,omitempty"`
	RetryDispatchExecuteActions int    `json:"retry_dispatch_execute_actions,omitempty"`
	ReclaimActions              int    `json:"reclaim_actions,omitempty"`
	ReconciliationActions       int    `json:"reconciliation_actions,omitempty"`
	SkippedUnsafeActions        int    `json:"skipped_unsafe_actions,omitempty"`
	PlacementReady              int    `json:"placement_ready,omitempty"`
	PlacementSkipped            int    `json:"placement_skipped,omitempty"`
	DispatchReady               int    `json:"dispatch_ready,omitempty"`
	DispatchSkipped             int    `json:"dispatch_skipped,omitempty"`
	OrchestrationRuns           int    `json:"orchestration_runs,omitempty"`
	MaxRuns                     int    `json:"max_runs,omitempty"`
	OrchestrationsCreated       int    `json:"orchestrations_created,omitempty"`
	LifecycleMaintenanceCreated int    `json:"lifecycle_maintenance_created,omitempty"`
	LifecycleAuditCreated       int    `json:"lifecycle_audit_created,omitempty"`
	ClaimedCount                int    `json:"claimed_count,omitempty"`
	DispatchedCount             int    `json:"dispatched_count,omitempty"`
	SkippedCount                int    `json:"skipped_count,omitempty"`
	FailedCount                 int    `json:"failed_count,omitempty"`
}

type WorkerLifecycleAudit struct {
	Version           int                         `json:"version"`
	Path              string                      `json:"-"`
	Status            string                      `json:"status"`
	Mode              string                      `json:"mode"`
	CreatedAt         string                      `json:"created_at"`
	StaleAfterSeconds int64                       `json:"stale_after_seconds"`
	Summary           WorkerLifecycleAuditSummary `json:"summary"`
	Items             []WorkerLifecycleAuditItem  `json:"items,omitempty"`
}

func PlanWorkerLifecycleAudit(root string, now time.Time, staleAfter time.Duration) (WorkerLifecycleAudit, error) {
	if now.IsZero() {
		now = time.Now().UTC()
	} else {
		now = now.UTC()
	}
	if staleAfter <= 0 {
		staleAfter = DefaultWorkerLeaseStaleAfter
	}
	audit := WorkerLifecycleAudit{
		Version:           1,
		Status:            "dry-run",
		Mode:              "metadata-only",
		CreatedAt:         now.Format(time.RFC3339Nano),
		StaleAfterSeconds: int64(staleAfter / time.Second),
	}
	workerIDs := map[string]bool{}

	workers, err := ListRuntimeWorkersWithStaleness(root, now, DefaultRuntimeWorkerStaleAfter)
	if err != nil {
		return WorkerLifecycleAudit{}, err
	}
	audit.Summary.WorkerRegistrations = len(workers)
	for _, worker := range workers {
		workerIDs[worker.WorkerID] = true
		if worker.Stale {
			audit.Summary.StaleWorkerRegistrations++
		}
		if strings.TrimSpace(worker.Status) == "offline" {
			audit.Summary.OfflineWorkerRegistrations++
		}
		audit.Items = append(audit.Items, workerLifecycleAuditItem("worker-registration", worker.Status, worker.Mode, worker.WorkerID, "", "", worker.Path, "", ""))
	}

	placementPlans, err := ListRuntimeWorkerPlacementPlans(root)
	if err != nil {
		return WorkerLifecycleAudit{}, err
	}
	audit.Summary.PlacementPlans = len(placementPlans)
	for _, plan := range placementPlans {
		audit.Items = append(audit.Items, workerLifecycleAuditItem("placement-plan", plan.Status, plan.Mode, "", plan.RunID, "", plan.Path, "", ""))
		for _, candidate := range plan.Candidates {
			if candidate.WorkerID != "" {
				workerIDs[candidate.WorkerID] = true
			}
		}
	}
	placementClaimExecutions, err := ListRuntimeWorkerPlacementClaimExecutions(root)
	if err != nil {
		return WorkerLifecycleAudit{}, err
	}
	audit.Summary.PlacementClaimExecutions = len(placementClaimExecutions)
	for _, execution := range placementClaimExecutions {
		workerIDs[execution.WorkerID] = true
		audit.Items = append(audit.Items, workerLifecycleAuditItem("placement-claim-execution", execution.Status, execution.Mode, execution.WorkerID, execution.RunID, execution.ExecutionID, execution.Path, execution.ClaimPath, execution.SkipReason))
	}
	placementDispatchPlans, err := ListRuntimeWorkerPlacementDispatchPlans(root)
	if err != nil {
		return WorkerLifecycleAudit{}, err
	}
	audit.Summary.PlacementDispatchPlans = len(placementDispatchPlans)
	for _, plan := range placementDispatchPlans {
		audit.Items = append(audit.Items, workerLifecycleAuditItem("placement-dispatch-plan", plan.Status, plan.Mode, "", "", "", plan.Path, "", ""))
		for _, candidate := range plan.Candidates {
			if candidate.WorkerID != "" {
				workerIDs[candidate.WorkerID] = true
			}
		}
	}
	placementDispatchExecutions, err := ListRuntimeWorkerPlacementDispatchExecutions(root)
	if err != nil {
		return WorkerLifecycleAudit{}, err
	}
	audit.Summary.PlacementDispatchExecutions = len(placementDispatchExecutions)
	for _, execution := range placementDispatchExecutions {
		workerIDs[execution.WorkerID] = true
		audit.Items = append(audit.Items, workerLifecycleAuditItem("placement-dispatch-execution", execution.Status, execution.Mode, execution.WorkerID, execution.RunID, execution.ExecutionID, execution.Path, execution.QueueDispatchExecutionPath, execution.SkipReason))
	}
	placementDispatchBatches, err := ListRuntimeWorkerPlacementDispatchBatches(root)
	if err != nil {
		return WorkerLifecycleAudit{}, err
	}
	audit.Summary.PlacementDispatchBatches = len(placementDispatchBatches)
	for _, batch := range placementDispatchBatches {
		audit.Items = append(audit.Items, workerLifecycleAuditItem("placement-dispatch-batch", batch.Status, batch.Mode, "", "", "", batch.Path, batch.PlacementDispatchPlanPath, batch.Error))
	}
	orchestrations, err := ListRuntimeWorkerOrchestrations(root)
	if err != nil {
		return WorkerLifecycleAudit{}, err
	}
	audit.Summary.WorkerOrchestrations = len(orchestrations)
	for _, orchestration := range orchestrations {
		item := workerLifecycleAuditItem("worker-orchestration", orchestration.Status, orchestration.Mode, "", orchestration.RunID, "", orchestration.Path, orchestration.PlacementDispatchBatchPath, orchestration.Error)
		item.PlacementReady = orchestration.PlacementReady
		item.PlacementSkipped = orchestration.PlacementSkipped
		item.ClaimedCount = orchestration.ClaimedCount
		item.DispatchReady = orchestration.DispatchReady
		item.DispatchSkipped = orchestration.DispatchSkipped
		item.DispatchedCount = orchestration.DispatchedCount
		item.SkippedCount = orchestration.ClaimSkipped + orchestration.DispatchSkipped
		item.FailedCount = orchestration.ClaimFailed + orchestration.DispatchFailed
		if strings.TrimSpace(orchestration.LifecycleMaintenancePath) != "" {
			item.LifecycleMaintenanceCreated = 1
		}
		if strings.TrimSpace(orchestration.LifecycleAuditPath) != "" {
			item.LifecycleAuditCreated = 1
		}
		audit.Items = append(audit.Items, item)
	}
	orchestrationSupervisors, err := ListRuntimeWorkerOrchestrationSupervisors(root)
	if err != nil {
		return WorkerLifecycleAudit{}, err
	}
	audit.Summary.WorkerOrchestrationSupervisors = len(orchestrationSupervisors)
	for _, supervisor := range orchestrationSupervisors {
		linkedPath := ""
		if len(supervisor.OrchestrationPaths) > 0 {
			linkedPath = supervisor.OrchestrationPaths[0]
		}
		item := workerLifecycleAuditItem("worker-orchestration-supervisor", supervisor.Status, supervisor.Mode, "", supervisor.RunIDPrefix, "", supervisor.Path, linkedPath, supervisor.Error)
		item.OrchestrationRuns = supervisor.Summary.Runs
		item.MaxRuns = supervisor.Summary.MaxRuns
		item.OrchestrationsCreated = supervisor.Summary.OrchestrationArtifactsCreated
		item.LifecycleMaintenanceCreated = supervisor.Summary.LifecycleMaintenanceArtifactsCreated
		item.LifecycleAuditCreated = supervisor.Summary.LifecycleAuditArtifactsCreated
		item.ClaimedCount = supervisor.Summary.ClaimedCount
		item.DispatchedCount = supervisor.Summary.DispatchedCount
		item.SkippedCount = supervisor.Summary.SkippedCount
		item.FailedCount = supervisor.Summary.FailedCount
		audit.Items = append(audit.Items, item)
	}

	claims, err := ListRuntimeWorkerQueueClaims(root)
	if err != nil {
		return WorkerLifecycleAudit{}, err
	}
	audit.Summary.QueueClaims = len(claims)
	for _, claim := range claims {
		workerIDs[claim.WorkerID] = true
		audit.Items = append(audit.Items, workerLifecycleAuditItem("queue-claim", claim.Status, claim.Mode, claim.WorkerID, claim.RunID, claim.ExecutionID, claim.Path, "", ""))
	}
	queueDispatches, err := ListRuntimeWorkerQueueDispatchExecutions(root)
	if err != nil {
		return WorkerLifecycleAudit{}, err
	}
	audit.Summary.QueueDispatchExecutions = len(queueDispatches)
	for _, dispatch := range queueDispatches {
		workerIDs[dispatch.WorkerID] = true
		audit.Items = append(audit.Items, workerLifecycleAuditItem("queue-dispatch-execution", dispatch.Status, dispatch.Mode, dispatch.WorkerID, dispatch.RunID, dispatch.ExecutionID, dispatch.Path, dispatch.TransportRequestPath, dispatch.SkipReason))
	}
	requests, err := ListRuntimeWorkerTransportDispatchRequests(root, "")
	if err != nil {
		return WorkerLifecycleAudit{}, err
	}
	audit.Summary.TransportRequests = len(requests)
	for _, request := range requests {
		workerIDs[request.WorkerID] = true
		switch strings.TrimSpace(request.Status) {
		case "pending":
			audit.Summary.TransportRequestsPending++
		case "acknowledged":
			audit.Summary.TransportRequestsAcknowledged++
		case "completed":
			audit.Summary.TransportRequestsCompleted++
		case "failed":
			audit.Summary.TransportRequestsFailed++
		}
		audit.Items = append(audit.Items, workerLifecycleAuditItem("transport-request", request.Status, request.TransportMode, request.WorkerID, request.RunID, request.ExecutionID, request.Path, request.DispatchExecutionPath, ""))
	}
	results, err := ListRuntimeWorkerQueueDispatchResults(root)
	if err != nil {
		return WorkerLifecycleAudit{}, err
	}
	audit.Summary.QueueDispatchResults = len(results)
	for _, result := range results {
		workerIDs[result.WorkerID] = true
		audit.Items = append(audit.Items, workerLifecycleAuditItem("queue-dispatch-result", result.Status, result.Mode, result.WorkerID, result.RunID, result.ExecutionID, result.Path, result.DispatchExecutionPath, result.Error))
	}
	dispatchRetryPlans, err := ListRuntimeWorkerDispatchRetryPlans(root)
	if err != nil {
		return WorkerLifecycleAudit{}, err
	}
	audit.Summary.DispatchRetryPlans = len(dispatchRetryPlans)
	for _, plan := range dispatchRetryPlans {
		linkedPath := ""
		for _, candidate := range plan.Ready {
			if candidate.WorkerID != "" {
				workerIDs[candidate.WorkerID] = true
			}
			if linkedPath == "" {
				linkedPath = candidate.ResultPath
			}
		}
		for _, skip := range plan.Skipped {
			if skip.WorkerID != "" {
				workerIDs[skip.WorkerID] = true
			}
			if linkedPath == "" {
				linkedPath = skip.ResultPath
			}
		}
		item := workerLifecycleAuditItem(
			"worker-dispatch-retry-plan",
			plan.Status,
			plan.Mode,
			"",
			"",
			"",
			plan.Path,
			linkedPath,
			fmt.Sprintf("ready=%d,skipped=%d", plan.Summary.Ready, plan.Summary.Skipped),
		)
		item.Gate = "worker-dispatch-retry-review"
		item.TotalCount = plan.Summary.Total
		item.ReadyCount = plan.Summary.Ready
		item.SkippedCount = plan.Summary.Skipped
		audit.Items = append(audit.Items, item)
	}
	dispatchRetryDispatches, err := ListRuntimeWorkerDispatchRetryDispatches(root)
	if err != nil {
		return WorkerLifecycleAudit{}, err
	}
	audit.Summary.DispatchRetryDispatches = len(dispatchRetryDispatches)
	for _, dispatch := range dispatchRetryDispatches {
		for _, candidate := range dispatch.Ready {
			if candidate.WorkerID != "" {
				workerIDs[candidate.WorkerID] = true
			}
		}
		for _, skip := range dispatch.Skipped {
			if skip.WorkerID != "" {
				workerIDs[skip.WorkerID] = true
			}
		}
		item := workerLifecycleAuditItem(
			"worker-dispatch-retry-dispatch",
			dispatch.Status,
			dispatch.Mode,
			"",
			"",
			"",
			dispatch.Path,
			dispatch.RetryPlanPath,
			fmt.Sprintf("ready=%d,skipped=%d", dispatch.Summary.Ready, dispatch.Summary.Skipped),
		)
		item.Gate = "worker-dispatch-retry-dispatch-review"
		item.TotalCount = dispatch.Summary.Total
		item.ReadyCount = dispatch.Summary.Ready
		item.SkippedCount = dispatch.Summary.Skipped
		audit.Items = append(audit.Items, item)
	}
	dispatchRetryDispatchExecutions, err := ListRuntimeWorkerDispatchRetryDispatchExecutions(root)
	if err != nil {
		return WorkerLifecycleAudit{}, err
	}
	audit.Summary.DispatchRetryDispatchExecutions = len(dispatchRetryDispatchExecutions)
	for _, execution := range dispatchRetryDispatchExecutions {
		for _, result := range execution.Results {
			if result.WorkerID != "" {
				workerIDs[result.WorkerID] = true
			}
		}
		item := workerLifecycleAuditItem(
			"worker-dispatch-retry-dispatch-execution",
			execution.Status,
			execution.Mode,
			"",
			"",
			"",
			execution.Path,
			execution.RetryDispatchPath,
			fmt.Sprintf("claimed=%d,skipped=%d,failed=%d", execution.Summary.Claimed, execution.Summary.Skipped, execution.Summary.Failed),
		)
		item.Gate = "worker-dispatch-retry-dispatch-execute"
		item.TotalCount = execution.Summary.Total
		item.ReadyCount = execution.Summary.Ready
		item.ClaimedCount = execution.Summary.Claimed
		item.SkippedCount = execution.Summary.Skipped
		item.FailedCount = execution.Summary.Failed
		audit.Items = append(audit.Items, item)
	}

	leases, err := ListWorkerLeasesWithStaleness(root, now, staleAfter)
	if err != nil {
		return WorkerLifecycleAudit{}, err
	}
	audit.Summary.WorkerLeases = len(leases)
	for _, lease := range leases {
		workerIDs[lease.WorkerID] = true
		if strings.TrimSpace(lease.Status) == "active" {
			audit.Summary.ActiveLeases++
			if lease.Stale {
				audit.Summary.StaleActiveLeases++
			}
		}
		reason := ""
		if lease.Stale {
			reason = "stale-active-worker-lease"
		}
		audit.Items = append(audit.Items, workerLifecycleAuditItem("worker-lease", lease.Status, "", lease.WorkerID, lease.RunID, lease.ExecutionID, lease.Path, lease.ResultExecutionID, reason))
	}
	reclaimReviews, err := ListWorkerLeaseReclaimDispatches(root)
	if err != nil {
		return WorkerLifecycleAudit{}, err
	}
	audit.Summary.ReclaimReviews = len(reclaimReviews)
	for _, review := range reclaimReviews {
		linkedPath := ""
		for _, candidate := range review.Ready {
			if candidate.Lease.WorkerID != "" {
				workerIDs[candidate.Lease.WorkerID] = true
			}
			if linkedPath == "" {
				linkedPath = candidate.Lease.Path
			}
		}
		for _, skip := range review.Skipped {
			if skip.Lease.WorkerID != "" {
				workerIDs[skip.Lease.WorkerID] = true
			}
			if linkedPath == "" {
				linkedPath = skip.Lease.Path
			}
		}
		item := workerLifecycleAuditItem("worker-lease-reclaim-review", review.Status, review.Mode, "", "", "", review.Path, linkedPath, "")
		item.Gate = "worker-lease-reclaim-review"
		item.TotalCount = len(review.Ready) + len(review.Skipped)
		item.ReadyCount = len(review.Ready)
		item.SkippedCount = len(review.Skipped)
		audit.Items = append(audit.Items, item)
	}
	reclaimExecutions, err := ListWorkerLeaseReclaimExecutions(root)
	if err != nil {
		return WorkerLifecycleAudit{}, err
	}
	audit.Summary.ReclaimExecutions = len(reclaimExecutions)
	for _, execution := range reclaimExecutions {
		for _, result := range execution.Results {
			if result.WorkerID != "" {
				workerIDs[result.WorkerID] = true
			}
		}
		for _, skip := range execution.Skipped {
			if skip.Lease.WorkerID != "" {
				workerIDs[skip.Lease.WorkerID] = true
			}
		}
		item := workerLifecycleAuditItem("worker-lease-reclaim-execution", execution.Status, execution.Mode, "", "", "", execution.Path, execution.DispatchPath, "")
		item.Gate = "worker-lease-reclaim-execute"
		item.TotalCount = len(execution.Results) + len(execution.Skipped)
		item.ResultCount = len(execution.Results)
		item.SkippedCount = len(execution.Skipped)
		audit.Items = append(audit.Items, item)
	}
	reconciliationReviews, err := ListWorkerLeaseResultReconciliationDispatches(root)
	if err != nil {
		return WorkerLifecycleAudit{}, err
	}
	audit.Summary.ReconciliationReviews = len(reconciliationReviews)
	for _, review := range reconciliationReviews {
		linkedPath := ""
		for _, candidate := range review.Ready {
			if candidate.Result.WorkerID != "" {
				workerIDs[candidate.Result.WorkerID] = true
			}
			if linkedPath == "" {
				linkedPath = workerLeaseResultReconciliationDisplayPath(candidate.ResultPath, candidate.Result.Path)
			}
		}
		for _, skip := range review.Skipped {
			if skip.Result.WorkerID != "" {
				workerIDs[skip.Result.WorkerID] = true
			}
			if linkedPath == "" {
				linkedPath = workerLeaseResultReconciliationDisplayPath(skip.ResultPath, skip.Result.Path)
			}
		}
		item := workerLifecycleAuditItem("worker-lease-result-reconciliation-review", review.Status, review.Mode, "", "", "", review.Path, linkedPath, "")
		item.Gate = "worker-lease-result-reconciliation-review"
		item.TotalCount = len(review.Ready) + len(review.Skipped)
		item.ReadyCount = len(review.Ready)
		item.SkippedCount = len(review.Skipped)
		audit.Items = append(audit.Items, item)
	}
	reconciliationExecutions, err := ListWorkerLeaseResultReconciliationExecutions(root)
	if err != nil {
		return WorkerLifecycleAudit{}, err
	}
	audit.Summary.ReconciliationExecutions = len(reconciliationExecutions)
	for _, execution := range reconciliationExecutions {
		for _, result := range execution.Results {
			if result.WorkerID != "" {
				workerIDs[result.WorkerID] = true
			}
		}
		for _, skip := range execution.Skipped {
			if skip.Result.WorkerID != "" {
				workerIDs[skip.Result.WorkerID] = true
			}
		}
		item := workerLifecycleAuditItem("worker-lease-result-reconciliation-execution", execution.Status, execution.Mode, "", "", "", execution.Path, execution.DispatchPath, "")
		item.Gate = "worker-lease-result-reconciliation-execute"
		item.TotalCount = len(execution.Results) + len(execution.Skipped)
		item.ResultCount = len(execution.Results)
		item.SkippedCount = len(execution.Skipped)
		audit.Items = append(audit.Items, item)
	}
	lifecycleReviews, err := ListWorkerLifecycleReviews(root)
	if err != nil {
		return WorkerLifecycleAudit{}, err
	}
	audit.Summary.LifecycleReviews = len(lifecycleReviews)
	for _, review := range lifecycleReviews {
		audit.Items = append(audit.Items, workerLifecycleAuditItem("worker-lifecycle-review", review.Status, review.Mode, "", "", "", review.Path, "", ""))
	}
	actionPlans, err := ListWorkerLifecycleActionPlans(root)
	if err != nil {
		return WorkerLifecycleAudit{}, err
	}
	audit.Summary.ActionPlans = len(actionPlans)
	for _, plan := range actionPlans {
		audit.Summary.ActionPlanActions += len(plan.Actions)
		item := workerLifecycleAuditItem("worker-lifecycle-action-plan", plan.Status, plan.Mode, "", "", "", plan.Path, plan.ReviewPath, "")
		item.ActionsTotal = plan.Summary.Total
		item.AckActions = plan.Summary.AckPendingTransport
		item.RunActions = plan.Summary.RunAcknowledgedTransport
		item.DaemonControlActions = plan.Summary.ApplyDaemonControl
		item.RetryDispatchExecuteActions = plan.Summary.ExecuteDispatchRetryDispatch
		item.ReclaimActions = plan.Summary.ReclaimStaleLease
		item.ReconciliationActions = plan.Summary.ReconcileResultLease
		if plan.Annotation != nil {
			audit.Summary.ActionPlanAnnotations++
			item.Decision = strings.TrimSpace(plan.Annotation.Decision)
			item.Reviewer = strings.TrimSpace(plan.Annotation.Reviewer)
			item.ReviewedAt = strings.TrimSpace(plan.Annotation.ReviewedAt)
			item.Reason = strings.TrimSpace(plan.Annotation.Reason)
		}
		audit.Items = append(audit.Items, item)
		for _, action := range plan.Actions {
			workerIDs[action.WorkerID] = true
			item := workerLifecycleAuditItem("worker-lifecycle-action", action.Status, action.Mode, action.WorkerID, action.RunID, action.ExecutionID, plan.Path, action.RequestPath+action.LeasePath+action.ResultPath+action.DispatchPath, action.Reason)
			item.Gate = action.Gate
			audit.Items = append(audit.Items, item)
		}
	}
	if selection, found, err := ReadWorkerLifecycleActionPlanSelection(root); err != nil {
		return WorkerLifecycleAudit{}, err
	} else if found {
		audit.Summary.ActionPlanSelections = 1
		item := workerLifecycleAuditItem("worker-lifecycle-action-plan-selection", selection.Status, selection.Mode, "", "", "", selection.Path, selection.PlanPath, selection.Reason)
		item.ActionsTotal = selection.Summary.Total
		item.AckActions = selection.Summary.AckPendingTransport
		item.RunActions = selection.Summary.RunAcknowledgedTransport
		item.DaemonControlActions = selection.Summary.ApplyDaemonControl
		item.RetryDispatchExecuteActions = selection.Summary.ExecuteDispatchRetryDispatch
		item.ReclaimActions = selection.Summary.ReclaimStaleLease
		item.ReconciliationActions = selection.Summary.ReconcileResultLease
		item.SelectedBy = strings.TrimSpace(selection.SelectedBy)
		item.SelectedAt = strings.TrimSpace(selection.SelectedAt)
		item.ClearedBy = strings.TrimSpace(selection.ClearedBy)
		item.ClearedAt = strings.TrimSpace(selection.ClearedAt)
		item.SupersededCount = len(selection.SupersededPlanPaths)
		audit.Items = append(audit.Items, item)
	}
	policies, err := ListWorkerLifecyclePolicies(root)
	if err != nil {
		return WorkerLifecycleAudit{}, err
	}
	audit.Summary.LifecyclePolicies = len(policies)
	for _, policy := range policies {
		item := workerLifecycleAuditItem("worker-lifecycle-policy", policy.Status, policy.Mode, "", "", "", policy.Path, "", "")
		item.MaxRuns = policy.MaxSupervisorRuns
		audit.Items = append(audit.Items, item)
	}
	if selection, found, err := ReadWorkerLifecyclePolicySelection(root); err != nil {
		return WorkerLifecycleAudit{}, err
	} else if found {
		audit.Summary.LifecyclePolicySelections = 1
		audit.Items = append(audit.Items, workerLifecycleAuditItem("worker-lifecycle-policy-selection", selection.Status, selection.Mode, "", "", "", selection.Path, selection.PolicyPath, selection.Reason))
	}
	policyRuns, err := ListWorkerLifecyclePolicyRuns(root)
	if err != nil {
		return WorkerLifecycleAudit{}, err
	}
	audit.Summary.LifecyclePolicyRuns = len(policyRuns)
	for _, run := range policyRuns {
		item := workerLifecycleAuditItem("worker-lifecycle-policy-run", run.Status, run.Mode, "", "", "", run.Path, run.PolicyPath, run.SkipReason+run.Error)
		item.MaxRuns = run.MaxRuns
		if strings.TrimSpace(run.SupervisorPath) != "" {
			item.LifecycleMaintenanceCreated = 1
			item.LinkedPath = run.SupervisorPath
		}
		audit.Items = append(audit.Items, item)
	}
	schedulerRuns, err := ListWorkerLifecyclePolicySchedulerRuns(root)
	if err != nil {
		return WorkerLifecycleAudit{}, err
	}
	audit.Summary.LifecyclePolicySchedulerRuns = len(schedulerRuns)
	for _, run := range schedulerRuns {
		linkedPath := run.PolicyPath
		if len(run.PolicyRunPaths) > 0 {
			linkedPath = run.PolicyRunPaths[0]
		}
		item := workerLifecycleAuditItem("worker-lifecycle-policy-scheduler-run", run.Status, run.Mode, "", "", "", run.Path, linkedPath, run.SkipReason+run.Error)
		item.MaxRuns = run.MaxRunsRequested
		if item.MaxRuns == 0 {
			item.MaxRuns = run.MaxRuns
		}
		item.ResultCount = run.PolicyRunsCreated
		item.LifecycleMaintenanceCreated = run.SupervisorsCreated
		audit.Items = append(audit.Items, item)
	}
	maintenances, err := ListWorkerLifecycleMaintenances(root)
	if err != nil {
		return WorkerLifecycleAudit{}, err
	}
	audit.Summary.LifecycleMaintenances = len(maintenances)
	for _, maintenance := range maintenances {
		item := workerLifecycleAuditItem("worker-lifecycle-maintenance", maintenance.Status, maintenance.Mode, "", "", "", maintenance.Path, maintenance.ActionPlanPath, maintenance.Error)
		item.ActionsTotal = maintenance.Summary.ActionsTotal
		item.AckActions = maintenance.Summary.AckActions
		item.RunActions = maintenance.Summary.RunActions
		item.DaemonControlActions = maintenance.Summary.DaemonControlActions
		item.RetryDispatchExecuteActions = maintenance.Summary.RetryDispatchExecuteActions
		item.ReclaimActions = maintenance.Summary.ReclaimActions
		item.ReconciliationActions = maintenance.Summary.ReconciliationActions
		item.SkippedUnsafeActions = maintenance.Summary.SkippedUnsafeActions
		audit.Items = append(audit.Items, item)
	}
	maintenanceSupervisors, err := ListWorkerLifecycleMaintenanceSupervisors(root)
	if err != nil {
		return WorkerLifecycleAudit{}, err
	}
	audit.Summary.LifecycleMaintenanceSupervisors = len(maintenanceSupervisors)
	for _, supervisor := range maintenanceSupervisors {
		linkedPath := ""
		if len(supervisor.MaintenancePaths) > 0 {
			linkedPath = supervisor.MaintenancePaths[0]
		}
		item := workerLifecycleAuditItem("worker-lifecycle-maintenance-supervisor", supervisor.Status, supervisor.Mode, "", "", "", supervisor.Path, linkedPath, supervisor.Error)
		item.ActionsTotal = supervisor.Summary.ActionsTotal
		item.AckActions = supervisor.Summary.AckActions
		item.RunActions = supervisor.Summary.RunActions
		item.DaemonControlActions = supervisor.Summary.DaemonControlActions
		item.RetryDispatchExecuteActions = supervisor.Summary.RetryDispatchExecuteActions
		item.ReclaimActions = supervisor.Summary.ReclaimActions
		item.ReconciliationActions = supervisor.Summary.ReconciliationActions
		item.SkippedUnsafeActions = supervisor.Summary.SkippedUnsafeActions
		audit.Items = append(audit.Items, item)
	}

	for _, workerID := range workerLifecycleAuditWorkerIDs(root, workerIDs) {
		daemonRuns, err := ListRuntimeWorkerTransportDaemonRuns(root, workerID)
		if err != nil {
			return WorkerLifecycleAudit{}, err
		}
		audit.Summary.DaemonRuns += len(daemonRuns)
		for _, run := range daemonRuns {
			audit.Items = append(audit.Items, workerLifecycleAuditItem("worker-transport-daemon-run", run.Status, run.Mode, run.WorkerID, "", "", run.Path, run.ControlRequestPath, run.Error))
		}
		supervisorRuns, err := ListRuntimeWorkerTransportDaemonSupervisorRuns(root, workerID)
		if err != nil {
			return WorkerLifecycleAudit{}, err
		}
		audit.Summary.DaemonSupervisorRuns += len(supervisorRuns)
		for _, run := range supervisorRuns {
			audit.Items = append(audit.Items, workerLifecycleAuditItem("worker-transport-daemon-supervisor-run", run.Status, run.Mode, run.WorkerID, "", "", run.Path, "", run.Error))
		}
		controls, err := ListRuntimeWorkerTransportDaemonControlRequests(root, workerID)
		if err != nil {
			return WorkerLifecycleAudit{}, err
		}
		audit.Summary.DaemonControls += len(controls)
		for _, control := range controls {
			item := workerLifecycleAuditItem("worker-transport-daemon-control", control.Status, control.Mode, control.WorkerID, "", "", control.Path, control.AppliedDaemonRunPath, control.Reason)
			item.Gate = control.Action
			audit.Items = append(audit.Items, item)
		}
	}

	sort.SliceStable(audit.Items, func(i, j int) bool {
		return workerLifecycleAuditItemSortKey(audit.Items[i]) < workerLifecycleAuditItemSortKey(audit.Items[j])
	})
	audit.Summary.ArtifactTotal = len(audit.Items)
	return audit, nil
}

func CreateWorkerLifecycleAudit(root string, now time.Time, staleAfter time.Duration) (WorkerLifecycleAudit, error) {
	audit, err := PlanWorkerLifecycleAudit(root, now, staleAfter)
	if err != nil {
		return WorkerLifecycleAudit{}, err
	}
	dir := workerLifecycleAuditDir(root)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return WorkerLifecycleAudit{}, fmt.Errorf("create worker lifecycle audit directory: %w", err)
	}
	audit.Path = filepath.Join(dir, safeWorkerLifecycleAuditName(audit)+".json")
	data, err := json.MarshalIndent(audit, "", "  ")
	if err != nil {
		return WorkerLifecycleAudit{}, fmt.Errorf("encode worker lifecycle audit: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(audit.Path, data, 0o644); err != nil {
		return WorkerLifecycleAudit{}, fmt.Errorf("write worker lifecycle audit: %w", err)
	}
	return audit, nil
}

func ListWorkerLifecycleAudits(root string) ([]WorkerLifecycleAudit, error) {
	paths, err := filepath.Glob(filepath.Join(workerLifecycleAuditDir(root), "*.json"))
	if err != nil {
		return nil, fmt.Errorf("glob worker lifecycle audits: %w", err)
	}
	sort.Strings(paths)
	audits := make([]WorkerLifecycleAudit, 0, len(paths))
	for _, path := range paths {
		audit, err := readWorkerLifecycleAuditFile(path)
		if err != nil {
			return nil, err
		}
		audits = append(audits, audit)
	}
	return audits, nil
}

func ReadWorkerLifecycleAudit(root, auditPath string) (WorkerLifecycleAudit, error) {
	path, err := resolveWorkerLifecycleAuditPath(root, auditPath)
	if err != nil {
		return WorkerLifecycleAudit{}, err
	}
	return readWorkerLifecycleAuditFile(path)
}

func readWorkerLifecycleAuditFile(path string) (WorkerLifecycleAudit, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return WorkerLifecycleAudit{}, fmt.Errorf("read worker lifecycle audit %s: %w", path, err)
	}
	var audit WorkerLifecycleAudit
	if err := json.Unmarshal(data, &audit); err != nil {
		return WorkerLifecycleAudit{}, fmt.Errorf("decode worker lifecycle audit %s: %w", path, err)
	}
	audit.Path = path
	return audit, nil
}

func workerLifecycleAuditDir(root string) string {
	return filepath.Join(root, "runtime", "worker-lifecycle-audits")
}

func resolveWorkerLifecycleAuditPath(root, auditPath string) (string, error) {
	trimmed := strings.TrimSpace(auditPath)
	if trimmed == "" {
		return "", fmt.Errorf("worker lifecycle audit path is required")
	}
	dir, err := filepath.Abs(workerLifecycleAuditDir(root))
	if err != nil {
		return "", fmt.Errorf("resolve worker lifecycle audit directory: %w", err)
	}
	candidate := trimmed
	if !filepath.IsAbs(candidate) {
		candidate = filepath.Join(dir, candidate)
	}
	candidate, err = filepath.Abs(candidate)
	if err != nil {
		return "", fmt.Errorf("resolve worker lifecycle audit path: %w", err)
	}
	rel, err := filepath.Rel(dir, candidate)
	if err != nil {
		return "", fmt.Errorf("resolve worker lifecycle audit relative path: %w", err)
	}
	if rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("worker lifecycle audit path must stay under %s", dir)
	}
	if filepath.Ext(candidate) != ".json" {
		return "", fmt.Errorf("worker lifecycle audit path must be a json file")
	}
	return candidate, nil
}

func workerLifecycleAuditWorkerIDs(root string, known map[string]bool) []string {
	for _, dir := range []string{
		runtimeWorkerTransportDaemonRunDir(root),
		runtimeWorkerTransportDaemonSupervisorRunDir(root),
		runtimeWorkerTransportDaemonControlDir(root),
		runtimeWorkerTransportOutboxDir(root),
	} {
		entries, err := filepath.Glob(filepath.Join(dir, "*"))
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if info, err := os.Stat(entry); err == nil && info.IsDir() {
				known[filepath.Base(entry)] = true
			}
		}
	}
	var ids []string
	for id := range known {
		if strings.TrimSpace(id) != "" {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	return ids
}

func workerLifecycleAuditItem(kind, status, mode, workerID, runID, executionID, artifactPath, linkedPath, reason string) WorkerLifecycleAuditItem {
	return WorkerLifecycleAuditItem{
		Kind:         strings.TrimSpace(kind),
		Status:       strings.TrimSpace(status),
		Mode:         strings.TrimSpace(mode),
		WorkerID:     strings.TrimSpace(workerID),
		RunID:        strings.TrimSpace(runID),
		ExecutionID:  strings.TrimSpace(executionID),
		ArtifactPath: strings.TrimSpace(artifactPath),
		LinkedPath:   strings.TrimSpace(linkedPath),
		Reason:       strings.TrimSpace(reason),
	}
}

func workerLifecycleAuditItemSortKey(item WorkerLifecycleAuditItem) string {
	return strings.Join([]string{
		item.Kind,
		item.WorkerID,
		item.RunID,
		item.ExecutionID,
		item.ArtifactPath,
	}, "|")
}

func safeWorkerLifecycleAuditName(audit WorkerLifecycleAudit) string {
	summary := audit.Summary
	return safeInstallRequestName(fmt.Sprintf(
		"worker-lifecycle-audit|%s|workers=%d|claims=%d|dispatches=%d|requests=%d|results=%d|retry-plans=%d|retry-dispatches=%d|retry-dispatch-executions=%d|leases=%d|reviews=%d|actions=%d|maintenances=%d",
		audit.CreatedAt,
		summary.WorkerRegistrations,
		summary.QueueClaims,
		summary.QueueDispatchExecutions,
		summary.TransportRequests,
		summary.QueueDispatchResults,
		summary.DispatchRetryPlans,
		summary.DispatchRetryDispatches,
		summary.DispatchRetryDispatchExecutions,
		summary.WorkerLeases,
		summary.LifecycleReviews,
		summary.ActionPlanActions,
		summary.LifecycleMaintenances,
	))
}
