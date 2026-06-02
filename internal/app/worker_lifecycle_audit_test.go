package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPlanWorkerLifecycleAuditSummarizesWorkerChainWithoutMutation(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	now := time.Date(2026, 5, 31, 9, 0, 0, 0, time.UTC)
	if _, err := RegisterRuntimeWorker(root, RuntimeWorkerArtifact{
		WorkerID:     "remote-web-1",
		Mode:         "remote",
		Status:       "available",
		Endpoint:     "file://remote-web-1/inbox",
		Capabilities: []string{"web.discovery"},
	}, RuntimeWorkerRegistrationOptions{Now: now}); err != nil {
		t.Fatalf("register runtime worker: %v", err)
	}
	placement, err := CreateRuntimeWorkerPlacementPlan(root, RuntimeWorkerPlacementPlanOptions{Now: now, RunID: "run-audit-placement"})
	if err != nil {
		t.Fatalf("create placement plan: %v", err)
	}
	if _, err := ExecuteRuntimeWorkerPlacementClaim(root, placement.Path, RuntimeWorkerPlacementClaimExecutionOptions{
		Execute:              true,
		Now:                  now.Add(time.Minute),
		CandidateExecutionID: "tool-discover_assets-01",
		WorkerID:             "remote-web-1",
		LeaseDuration:        10 * time.Minute,
	}); err != nil {
		t.Fatalf("execute placement claim: %v", err)
	}
	dispatchPlan, err := CreateRuntimeWorkerPlacementDispatchPlan(root, RuntimeWorkerPlacementDispatchPlanOptions{
		Now:           now.Add(2 * time.Minute),
		TransportMode: "metadata-only",
	})
	if err != nil {
		t.Fatalf("create placement dispatch plan: %v", err)
	}
	if _, err := ExecuteRuntimeWorkerPlacementDispatchBatch(root, dispatchPlan.Path, RuntimeWorkerPlacementDispatchBatchOptions{
		Execute: true,
		Now:     now.Add(3 * time.Minute),
	}); err != nil {
		t.Fatalf("execute placement dispatch batch: %v", err)
	}
	fileDispatch := createFileOutboxRuntimeWorkerQueueDispatchForExecution(t, root, now.Add(4*time.Minute), "run-audit-file", "tool-audit-file")
	if _, err := SubmitRuntimeWorkerQueueDispatchResult(root, filepath.Base(fileDispatch.Path), RuntimeWorkerQueueDispatchResult{
		Status: "completed",
		Output: "audit result",
	}, RuntimeWorkerQueueDispatchResultOptions{Now: now.Add(5 * time.Minute)}); err != nil {
		t.Fatalf("submit dispatch result: %v", err)
	}
	if err := writeWorkerLeaseArtifact(root, WorkerLeaseArtifact{
		Version:     1,
		RunID:       fileDispatch.RunID,
		ExecutionID: fileDispatch.ExecutionID,
		Tool:        fileDispatch.Tool,
		Capability:  fileDispatch.Capability,
		WorkerID:    fileDispatch.WorkerID,
		Status:      "active",
		AcquiredAt:  now.Format(time.RFC3339Nano),
		HeartbeatAt: now.Format(time.RFC3339Nano),
	}); err != nil {
		t.Fatalf("write active worker lease fixture: %v", err)
	}
	if err := writeWorkerLeaseArtifact(root, WorkerLeaseArtifact{
		Version:     1,
		RunID:       "run-audit-stale",
		ExecutionID: "tool-audit-stale",
		Tool:        "web-api-pack",
		Capability:  "web.discovery",
		WorkerID:    "remote-web-1",
		Status:      "active",
		AcquiredAt:  now.Add(-30 * time.Minute).Format(time.RFC3339Nano),
		HeartbeatAt: now.Add(-30 * time.Minute).Format(time.RFC3339Nano),
	}); err != nil {
		t.Fatalf("write stale worker lease fixture: %v", err)
	}
	if _, err := CreateWorkerLeaseReclaimDispatch(root, now.Add(6*time.Minute), DefaultWorkerLeaseStaleAfter); err != nil {
		t.Fatalf("create reclaim review: %v", err)
	}
	if _, err := CreateWorkerLeaseResultReconciliationDispatch(root, now.Add(7*time.Minute)); err != nil {
		t.Fatalf("create reconciliation review: %v", err)
	}
	review, err := CreateWorkerLifecycleReview(root, now.Add(8*time.Minute), DefaultWorkerLeaseStaleAfter)
	if err != nil {
		t.Fatalf("create lifecycle review: %v", err)
	}
	if _, err := CreateWorkerLifecycleActionPlan(root, filepath.Base(review.Path), now.Add(9*time.Minute), DefaultWorkerLeaseStaleAfter); err != nil {
		t.Fatalf("create lifecycle action plan: %v", err)
	}
	maintenance, err := CreateWorkerLifecycleMaintenance(root, WorkerLifecycleMaintenanceOptions{
		Now:        now.Add(9*time.Minute + 30*time.Second),
		StaleAfter: DefaultWorkerLeaseStaleAfter,
	})
	if err != nil {
		t.Fatalf("create lifecycle maintenance: %v", err)
	}

	audit, err := PlanWorkerLifecycleAudit(root, now.Add(10*time.Minute), DefaultWorkerLeaseStaleAfter)
	if err != nil {
		t.Fatalf("plan worker lifecycle audit: %v", err)
	}
	if audit.Path != "" || audit.Status != "dry-run" || audit.Mode != "metadata-only" {
		t.Fatalf("planning must stay dry-run metadata-only without artifact path, got %#v", audit)
	}
	summary := audit.Summary
	if summary.WorkerRegistrations != 1 || summary.PlacementPlans != 1 || summary.PlacementClaimExecutions != 1 || summary.PlacementDispatchPlans != 1 || summary.PlacementDispatchBatches != 1 {
		t.Fatalf("unexpected placement/registration summary: %#v", summary)
	}
	if summary.QueueClaims < 2 || summary.QueueDispatchExecutions < 2 || summary.TransportRequests != 1 || summary.QueueDispatchResults != 1 {
		t.Fatalf("unexpected queue/transport summary: %#v", summary)
	}
	if summary.WorkerLeases != 2 || summary.ActiveLeases != 2 || summary.StaleActiveLeases != 1 {
		t.Fatalf("unexpected lease summary: %#v", summary)
	}
	if summary.ReclaimReviews != 1 || summary.ReconciliationReviews != 1 || summary.LifecycleReviews != 1 || summary.ActionPlans != 1 || summary.ActionPlanActions == 0 || summary.LifecycleMaintenances != 1 {
		t.Fatalf("unexpected review/action summary: %#v", summary)
	}
	if !workerLifecycleAuditHasItem(audit.Items, "worker-lifecycle-maintenance", maintenance.Path) {
		t.Fatalf("expected audit to include lifecycle maintenance item for %s, got %#v", maintenance.Path, audit.Items)
	}
	if len(audit.Items) == 0 {
		t.Fatalf("expected audit items")
	}
	if _, err := ListWorkerLifecycleAudits(root); err != nil {
		t.Fatalf("list worker lifecycle audits: %v", err)
	} else if countJSON(t, filepath.Join(root, "runtime", "worker-lifecycle-audits")) != 0 {
		t.Fatalf("planning must not write lifecycle audit artifacts")
	}
	requests, err := ListRuntimeWorkerTransportDispatchRequests(root, "remote-web-1")
	if err != nil {
		t.Fatalf("list requests after audit planning: %v", err)
	}
	if len(requests) != 1 || requests[0].Status != "completed" {
		t.Fatalf("audit planning must not mutate transport requests, got %#v", requests)
	}
}

func TestCreateWorkerLifecycleAuditWritesMetadataOnlyArtifact(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Date(2026, 5, 31, 9, 45, 0, 0, time.UTC)
	dispatch := createFileOutboxRuntimeWorkerQueueDispatch(t, root, now)

	audit, err := CreateWorkerLifecycleAudit(root, now.Add(time.Minute), DefaultWorkerLeaseStaleAfter)
	if err != nil {
		t.Fatalf("create worker lifecycle audit: %v", err)
	}
	if audit.Path == "" || !strings.Contains(audit.Path, filepath.Join("runtime", "worker-lifecycle-audits")) {
		t.Fatalf("expected worker lifecycle audit path, got %s", audit.Path)
	}
	if audit.Status != "dry-run" || audit.Mode != "metadata-only" || audit.Summary.WorkerRegistrations != 1 || audit.Summary.TransportRequests != 1 {
		t.Fatalf("unexpected persisted audit: %#v", audit)
	}
	data, err := os.ReadFile(audit.Path)
	if err != nil {
		t.Fatalf("read worker lifecycle audit artifact: %v", err)
	}
	var decoded WorkerLifecycleAudit
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("decode worker lifecycle audit artifact: %v", err)
	}
	if decoded.Summary.QueueDispatchExecutions != 1 || !workerLifecycleAuditHasItem(decoded.Items, "transport-request", dispatch.TransportRequestPath) {
		t.Fatalf("expected decoded audit to include dispatch request, got %#v", decoded)
	}
	audits, err := ListWorkerLifecycleAudits(root)
	if err != nil {
		t.Fatalf("list worker lifecycle audits: %v", err)
	}
	if len(audits) != 1 || audits[0].Path != audit.Path {
		t.Fatalf("unexpected listed worker lifecycle audits: %#v", audits)
	}
	read, err := ReadWorkerLifecycleAudit(root, filepath.Base(audit.Path))
	if err != nil {
		t.Fatalf("read worker lifecycle audit: %v", err)
	}
	if read.Path != audit.Path || read.Summary.TransportRequests != 1 {
		t.Fatalf("unexpected read worker lifecycle audit: %#v", read)
	}
	if _, err := ReadWorkerLifecycleAudit(root, filepath.Join(t.TempDir(), "outside.json")); err == nil || !strings.Contains(err.Error(), "worker lifecycle audit path must stay under") {
		t.Fatalf("expected worker lifecycle audit path confinement error, got %v", err)
	}
}

func TestPlanWorkerLifecycleAuditIncludesMaintenanceSupervisorRuns(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Date(2026, 5, 31, 10, 30, 0, 0, time.UTC)
	fixture := createWorkerLifecycleMaintenanceFixture(t, root, now)
	supervisor, err := CreateWorkerLifecycleMaintenanceSupervisor(root, WorkerLifecycleMaintenanceSupervisorOptions{
		Execute:    true,
		Now:        now.Add(5 * time.Minute),
		StaleAfter: 5 * time.Minute,
		MaxRuns:    1,
	})
	if err != nil {
		t.Fatalf("create maintenance supervisor fixture: %v", err)
	}

	audit, err := PlanWorkerLifecycleAudit(root, now.Add(6*time.Minute), 5*time.Minute)
	if err != nil {
		t.Fatalf("plan worker lifecycle audit: %v", err)
	}
	if audit.Summary.LifecycleMaintenanceSupervisors != 1 {
		t.Fatalf("expected one maintenance supervisor in audit summary, got %#v", audit.Summary)
	}
	if !workerLifecycleAuditHasItem(audit.Items, "worker-lifecycle-maintenance-supervisor", supervisor.Path) {
		t.Fatalf("expected audit to include maintenance supervisor item for %s, got %#v", supervisor.Path, audit.Items)
	}
	assertWorkerLifecycleMaintenanceFixtureUnmutated(t, root, fixture)
}

func TestPlanWorkerLifecycleAuditIncludesPolicyAndSchedulerRunArtifacts(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Date(2026, 6, 1, 22, 0, 0, 0, time.UTC)
	createWorkerLifecycleMaintenanceFixture(t, root, now)
	policy := createSelectedWorkerLifecyclePolicyForScheduler(t, root, now, time.Second, true)
	selectionPath := filepath.Join(root, "runtime", "worker-lifecycle-policy-selection.json")
	policyRun, err := CreateWorkerLifecyclePolicyRun(root, WorkerLifecyclePolicyRunOptions{
		Now:    now.Add(10 * time.Minute),
		Actor:  "audit-test",
		Reason: "audit policy-run visibility",
	})
	if err != nil {
		t.Fatalf("create policy run fixture: %v", err)
	}
	schedulerRun, err := CreateWorkerLifecyclePolicySchedulerRun(root, WorkerLifecyclePolicySchedulerRunOptions{
		Now:     now.Add(11 * time.Minute),
		Actor:   "audit-test",
		Reason:  "audit scheduler visibility",
		MaxRuns: 2,
	})
	if err != nil {
		t.Fatalf("create scheduler run fixture: %v", err)
	}
	policyRunsBefore := countJSON(t, filepath.Join(root, "runtime", "worker-lifecycle-policy-runs"))
	supervisorsBefore := countJSON(t, filepath.Join(root, "runtime", "worker-lifecycle-maintenance-supervisor-runs"))

	audit, err := PlanWorkerLifecycleAudit(root, now.Add(12*time.Minute), DefaultWorkerLeaseStaleAfter)
	if err != nil {
		t.Fatalf("plan worker lifecycle audit: %v", err)
	}

	if audit.Summary.LifecyclePolicies != 1 ||
		audit.Summary.LifecyclePolicySelections != 1 ||
		audit.Summary.LifecyclePolicyRuns != 1 ||
		audit.Summary.LifecyclePolicySchedulerRuns != 1 {
		t.Fatalf("expected policy/scheduler counts in audit summary, got %#v", audit.Summary)
	}
	if !workerLifecycleAuditHasItem(audit.Items, "worker-lifecycle-policy", policy.Path) {
		t.Fatalf("expected audit to include policy item for %s, got %#v", policy.Path, audit.Items)
	}
	if !workerLifecycleAuditHasItem(audit.Items, "worker-lifecycle-policy-selection", selectionPath) {
		t.Fatalf("expected audit to include policy selection item for %s, got %#v", selectionPath, audit.Items)
	}
	if !workerLifecycleAuditHasItem(audit.Items, "worker-lifecycle-policy-run", policyRun.Path) {
		t.Fatalf("expected audit to include policy-run item for %s, got %#v", policyRun.Path, audit.Items)
	}
	if !workerLifecycleAuditHasItem(audit.Items, "worker-lifecycle-policy-scheduler-run", schedulerRun.Path) {
		t.Fatalf("expected audit to include scheduler-run item for %s, got %#v", schedulerRun.Path, audit.Items)
	}
	if got := countJSON(t, filepath.Join(root, "runtime", "worker-lifecycle-policy-runs")); got != policyRunsBefore {
		t.Fatalf("audit planning must not create policy-run artifacts: before=%d after=%d", policyRunsBefore, got)
	}
	if got := countJSON(t, filepath.Join(root, "runtime", "worker-lifecycle-maintenance-supervisor-runs")); got != supervisorsBefore {
		t.Fatalf("audit planning must not create maintenance supervisors: before=%d after=%d", supervisorsBefore, got)
	}
}

func TestPlanWorkerLifecycleAuditCarriesMaintenanceAggregateFields(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Date(2026, 5, 31, 16, 20, 0, 0, time.UTC)
	marker := filepath.Join(t.TempDir(), "audit-maintenance-retry-dispatch-ran")
	_, retryDispatch, _ := createRuntimeWorkerDispatchRetryDispatchFixture(t, root, now, RuntimeWorkerToolPlanPayload{
		Action:     "discover_assets",
		Runner:     "command",
		Parser:     "web_api_parser",
		RunCommand: []string{"sh", "-c", "touch " + marker},
	})
	maintenance, err := CreateWorkerLifecycleMaintenance(root, WorkerLifecycleMaintenanceOptions{
		Execute:    true,
		Now:        now.Add(4 * time.Minute),
		StaleAfter: DefaultWorkerLeaseStaleAfter,
	})
	if err != nil {
		t.Fatalf("create lifecycle maintenance fixture: %v", err)
	}
	if maintenance.Summary.ActionsTotal != 1 || maintenance.Summary.SkippedUnsafeActions != 1 || maintenance.Summary.RetryDispatchExecuteActions != 1 {
		t.Fatalf("expected retry-dispatch aggregate maintenance fixture, got %#v", maintenance.Summary)
	}
	claimsBefore := countJSON(t, filepath.Join(root, "runtime", "worker-queue-claims"))

	audit, err := PlanWorkerLifecycleAudit(root, now.Add(5*time.Minute), DefaultWorkerLeaseStaleAfter)
	if err != nil {
		t.Fatalf("plan worker lifecycle audit: %v", err)
	}

	item, ok := workerLifecycleAuditFindItem(audit.Items, "worker-lifecycle-maintenance", maintenance.Path)
	if !ok {
		t.Fatalf("expected audit to include maintenance item for %s, got %#v", maintenance.Path, audit.Items)
	}
	if item.ActionsTotal != 1 || item.SkippedUnsafeActions != 1 || item.RetryDispatchExecuteActions != 1 {
		t.Fatalf("expected maintenance aggregate fields on audit item, got %#v", item)
	}
	if item.LinkedPath != maintenance.ActionPlanPath {
		t.Fatalf("expected maintenance item to link action plan %s, got %#v", maintenance.ActionPlanPath, item)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("audit planning must not execute retry tool-plan command for %s, marker stat err=%v", retryDispatch.Path, err)
	}
	if got := countJSON(t, filepath.Join(root, "runtime", "worker-queue-claims")); got != claimsBefore {
		t.Fatalf("audit planning must not create retry queue claims: before=%d after=%d", claimsBefore, got)
	}
	if executions, err := ListRuntimeWorkerDispatchRetryDispatchExecutions(root); err != nil {
		t.Fatalf("list retry-dispatch executions after audit: %v", err)
	} else if len(executions) != 0 {
		t.Fatalf("audit planning must not execute retry dispatch artifacts, got %#v", executions)
	}
}

func TestPlanWorkerLifecycleAuditCarriesActionPlanAggregateFields(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Date(2026, 5, 31, 16, 45, 0, 0, time.UTC)
	marker := filepath.Join(t.TempDir(), "audit-action-plan-retry-dispatch-ran")
	_, retryDispatch, _ := createRuntimeWorkerDispatchRetryDispatchFixture(t, root, now, RuntimeWorkerToolPlanPayload{
		Action:     "discover_assets",
		Runner:     "command",
		Parser:     "web_api_parser",
		RunCommand: []string{"sh", "-c", "touch " + marker},
	})
	plan, err := CreateWorkerLifecycleActionPlan(root, "", now.Add(4*time.Minute), DefaultWorkerLeaseStaleAfter)
	if err != nil {
		t.Fatalf("create lifecycle action plan fixture: %v", err)
	}
	if plan.Summary.Total == 0 || plan.Summary.ExecuteDispatchRetryDispatch != 1 {
		t.Fatalf("expected retry-dispatch action-plan fixture, got %#v", plan.Summary)
	}
	claimsBefore := countJSON(t, filepath.Join(root, "runtime", "worker-queue-claims"))

	audit, err := PlanWorkerLifecycleAudit(root, now.Add(5*time.Minute), DefaultWorkerLeaseStaleAfter)
	if err != nil {
		t.Fatalf("plan worker lifecycle audit: %v", err)
	}

	item, ok := workerLifecycleAuditFindItem(audit.Items, "worker-lifecycle-action-plan", plan.Path)
	if !ok {
		t.Fatalf("expected audit to include action-plan item for %s, got %#v", plan.Path, audit.Items)
	}
	if item.ActionsTotal != plan.Summary.Total ||
		item.AckActions != plan.Summary.AckPendingTransport ||
		item.RunActions != plan.Summary.RunAcknowledgedTransport ||
		item.DaemonControlActions != plan.Summary.ApplyDaemonControl ||
		item.RetryDispatchExecuteActions != plan.Summary.ExecuteDispatchRetryDispatch ||
		item.ReclaimActions != plan.Summary.ReclaimStaleLease ||
		item.ReconciliationActions != plan.Summary.ReconcileResultLease {
		t.Fatalf("expected action-plan aggregate fields on audit item, got item=%#v summary=%#v", item, plan.Summary)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("audit planning must not execute retry tool-plan command for %s, marker stat err=%v", retryDispatch.Path, err)
	}
	if got := countJSON(t, filepath.Join(root, "runtime", "worker-queue-claims")); got != claimsBefore {
		t.Fatalf("audit planning must not create retry queue claims: before=%d after=%d", claimsBefore, got)
	}
	if executions, err := ListRuntimeWorkerDispatchRetryDispatchExecutions(root); err != nil {
		t.Fatalf("list retry-dispatch executions after audit: %v", err)
	} else if len(executions) != 0 {
		t.Fatalf("audit planning must not execute retry dispatch artifacts, got %#v", executions)
	}
}

func TestPlanWorkerLifecycleAuditIncludesActionPlanReviewSelectionMetadata(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Date(2026, 6, 2, 10, 0, 0, 0, time.UTC)
	dispatch := createFileOutboxRuntimeWorkerQueueDispatch(t, root, now)
	plan, err := CreateWorkerLifecycleActionPlan(root, "", now.Add(time.Minute), DefaultWorkerLeaseStaleAfter)
	if err != nil {
		t.Fatalf("create lifecycle action plan fixture: %v", err)
	}
	reviewedAt := now.Add(2 * time.Minute)
	if _, err := AnnotateWorkerLifecycleActionPlan(root, filepath.Base(plan.Path), WorkerLifecycleActionPlanAnnotationOptions{
		Decision:   "accepted",
		Reviewer:   "alice",
		ReviewedAt: reviewedAt,
		Reason:     "ready for current lifecycle focus",
	}); err != nil {
		t.Fatalf("annotate lifecycle action plan fixture: %v", err)
	}
	selectedAt := now.Add(3 * time.Minute)
	selection, err := SelectWorkerLifecycleActionPlan(root, filepath.Base(plan.Path), WorkerLifecycleActionPlanSelectionOptions{
		SelectedBy: "bob",
		SelectedAt: selectedAt,
		Reason:     "current reviewed lifecycle action plan",
	})
	if err != nil {
		t.Fatalf("select lifecycle action plan fixture: %v", err)
	}
	auditsBefore := countJSON(t, filepath.Join(root, "runtime", "worker-lifecycle-audits"))
	claimsBefore := countJSON(t, filepath.Join(root, "runtime", "worker-queue-claims"))

	audit, err := PlanWorkerLifecycleAudit(root, now.Add(4*time.Minute), DefaultWorkerLeaseStaleAfter)
	if err != nil {
		t.Fatalf("plan worker lifecycle audit: %v", err)
	}

	if audit.Summary.ActionPlans != 1 || audit.Summary.ActionPlanAnnotations != 1 || audit.Summary.ActionPlanSelections != 1 {
		t.Fatalf("expected action-plan review/selection summary, got %#v", audit.Summary)
	}
	planItem, ok := workerLifecycleAuditFindItem(audit.Items, "worker-lifecycle-action-plan", plan.Path)
	if !ok {
		t.Fatalf("expected audit to include action-plan item for %s, got %#v", plan.Path, audit.Items)
	}
	if planItem.Decision != "accepted" || planItem.Reviewer != "alice" || planItem.ReviewedAt != reviewedAt.Format(time.RFC3339) || planItem.Reason != "ready for current lifecycle focus" {
		t.Fatalf("expected action-plan item to carry review metadata, got %#v", planItem)
	}
	selectionItem, ok := workerLifecycleAuditFindItem(audit.Items, "worker-lifecycle-action-plan-selection", selection.Path)
	if !ok {
		t.Fatalf("expected audit to include action-plan selection item for %s, got %#v", selection.Path, audit.Items)
	}
	if selectionItem.Status != "current" || selectionItem.Mode != "metadata-only" || selectionItem.LinkedPath != plan.Path {
		t.Fatalf("expected current selection links/state, got %#v", selectionItem)
	}
	if selectionItem.SelectedBy != "bob" || selectionItem.SelectedAt != selectedAt.Format(time.RFC3339) || selectionItem.Reason != "current reviewed lifecycle action plan" {
		t.Fatalf("expected selection operator metadata, got %#v", selectionItem)
	}
	if selectionItem.ActionsTotal != plan.Summary.Total || selectionItem.AckActions != plan.Summary.AckPendingTransport {
		t.Fatalf("expected selection item to carry action summary, got item=%#v plan=%#v", selectionItem, plan.Summary)
	}
	if got := countJSON(t, filepath.Join(root, "runtime", "worker-lifecycle-audits")); got != auditsBefore {
		t.Fatalf("audit planning must not create audit artifacts: before=%d after=%d", auditsBefore, got)
	}
	if got := countJSON(t, filepath.Join(root, "runtime", "worker-queue-claims")); got != claimsBefore {
		t.Fatalf("audit planning must not create queue claims: before=%d after=%d", claimsBefore, got)
	}
	requests, err := ListRuntimeWorkerTransportDispatchRequests(root, dispatch.WorkerID)
	if err != nil {
		t.Fatalf("list transport requests after audit planning: %v", err)
	}
	if len(requests) != 1 || requests[0].Status != "pending" {
		t.Fatalf("audit planning must not acknowledge or run transport requests, got %#v", requests)
	}

	clearedAt := now.Add(5 * time.Minute)
	cleared, err := ClearWorkerLifecycleActionPlanSelection(root, WorkerLifecycleActionPlanSelectionClearOptions{
		ClearedBy: "carol",
		ClearedAt: clearedAt,
		Reason:    "close current lifecycle focus",
	})
	if err != nil {
		t.Fatalf("clear lifecycle action plan selection fixture: %v", err)
	}
	clearedAudit, err := PlanWorkerLifecycleAudit(root, now.Add(6*time.Minute), DefaultWorkerLeaseStaleAfter)
	if err != nil {
		t.Fatalf("plan worker lifecycle audit after clear: %v", err)
	}
	if clearedAudit.Summary.ActionPlanSelections != 1 {
		t.Fatalf("expected cleared action-plan selection marker in audit summary, got %#v", clearedAudit.Summary)
	}
	clearedItem, ok := workerLifecycleAuditFindItem(clearedAudit.Items, "worker-lifecycle-action-plan-selection", cleared.Path)
	if !ok {
		t.Fatalf("expected audit to include cleared action-plan selection item for %s, got %#v", cleared.Path, clearedAudit.Items)
	}
	if clearedItem.Status != "cleared" || clearedItem.Mode != "metadata-only" || clearedItem.LinkedPath != plan.Path {
		t.Fatalf("expected cleared selection links/state, got %#v", clearedItem)
	}
	if clearedItem.ClearedBy != "carol" || clearedItem.ClearedAt != clearedAt.Format(time.RFC3339) || clearedItem.Reason != "close current lifecycle focus" {
		t.Fatalf("expected cleared selection operator metadata, got %#v", clearedItem)
	}
	if got := countJSON(t, filepath.Join(root, "runtime", "worker-lifecycle-audits")); got != auditsBefore {
		t.Fatalf("audit planning after clear must not create audit artifacts: before=%d after=%d", auditsBefore, got)
	}
}

func TestPlanWorkerLifecycleAuditCarriesMaintenanceSupervisorAggregateFields(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Date(2026, 5, 31, 15, 45, 0, 0, time.UTC)
	marker := filepath.Join(t.TempDir(), "audit-supervisor-retry-dispatch-ran")
	_, retryDispatch, _ := createRuntimeWorkerDispatchRetryDispatchFixture(t, root, now, RuntimeWorkerToolPlanPayload{
		Action:     "discover_assets",
		Runner:     "command",
		Parser:     "web_api_parser",
		RunCommand: []string{"sh", "-c", "touch " + marker},
	})
	supervisor, err := CreateWorkerLifecycleMaintenanceSupervisor(root, WorkerLifecycleMaintenanceSupervisorOptions{
		Execute:    true,
		Now:        now.Add(4 * time.Minute),
		StaleAfter: DefaultWorkerLeaseStaleAfter,
		MaxRuns:    2,
	})
	if err != nil {
		t.Fatalf("create maintenance supervisor fixture: %v", err)
	}
	if supervisor.Summary.ActionsTotal != 2 || supervisor.Summary.SkippedUnsafeActions != 2 || supervisor.Summary.RetryDispatchExecuteActions != 2 {
		t.Fatalf("expected retry-dispatch aggregate supervisor fixture, got %#v", supervisor.Summary)
	}
	claimsBefore := countJSON(t, filepath.Join(root, "runtime", "worker-queue-claims"))

	audit, err := PlanWorkerLifecycleAudit(root, now.Add(5*time.Minute), DefaultWorkerLeaseStaleAfter)
	if err != nil {
		t.Fatalf("plan worker lifecycle audit: %v", err)
	}

	item, ok := workerLifecycleAuditFindItem(audit.Items, "worker-lifecycle-maintenance-supervisor", supervisor.Path)
	if !ok {
		t.Fatalf("expected audit to include maintenance supervisor item for %s, got %#v", supervisor.Path, audit.Items)
	}
	if item.ActionsTotal != 2 || item.SkippedUnsafeActions != 2 || item.RetryDispatchExecuteActions != 2 {
		t.Fatalf("expected supervisor aggregate fields on audit item, got %#v", item)
	}
	if item.LinkedPath == "" {
		t.Fatalf("expected supervisor item to keep linked maintenance path, got %#v", item)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("audit planning must not execute retry tool-plan command for %s, marker stat err=%v", retryDispatch.Path, err)
	}
	if got := countJSON(t, filepath.Join(root, "runtime", "worker-queue-claims")); got != claimsBefore {
		t.Fatalf("audit planning must not create retry queue claims: before=%d after=%d", claimsBefore, got)
	}
	if executions, err := ListRuntimeWorkerDispatchRetryDispatchExecutions(root); err != nil {
		t.Fatalf("list retry-dispatch executions after audit: %v", err)
	} else if len(executions) != 0 {
		t.Fatalf("audit planning must not execute retry dispatch artifacts, got %#v", executions)
	}
}

func TestPlanWorkerLifecycleAuditIncludesWorkerOrchestrationSupervisorRuns(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Date(2026, 5, 31, 11, 50, 0, 0, time.UTC)
	supervisor, err := CreateRuntimeWorkerOrchestrationSupervisor(root, RuntimeWorkerOrchestrationSupervisorOptions{
		Now:           now,
		RunIDPrefix:   "run-audit-orchestration-supervisor",
		TransportMode: "metadata-only",
		MaxRuns:       2,
	})
	if err != nil {
		t.Fatalf("create worker orchestration supervisor fixture: %v", err)
	}

	audit, err := PlanWorkerLifecycleAudit(root, now.Add(time.Minute), DefaultWorkerLeaseStaleAfter)
	if err != nil {
		t.Fatalf("plan worker lifecycle audit: %v", err)
	}
	if audit.Summary.WorkerOrchestrationSupervisors != 1 {
		t.Fatalf("expected one worker orchestration supervisor in audit summary, got %#v", audit.Summary)
	}
	if !workerLifecycleAuditHasItem(audit.Items, "worker-orchestration-supervisor", supervisor.Path) {
		t.Fatalf("expected audit to include worker orchestration supervisor item for %s, got %#v", supervisor.Path, audit.Items)
	}
	if countJSON(t, filepath.Join(root, "runtime", "worker-orchestrations")) != 0 {
		t.Fatalf("dry-run orchestration supervisor fixture must not create child orchestrations")
	}
}

func TestPlanWorkerLifecycleAuditCarriesWorkerOrchestrationSupervisorAggregateFields(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Date(2026, 5, 31, 16, 25, 0, 0, time.UTC)
	registerWorkerLifecycleAuditOrchestrationWorkers(t, root, now)
	supervisor, err := CreateRuntimeWorkerOrchestrationSupervisor(root, RuntimeWorkerOrchestrationSupervisorOptions{
		Execute:                     true,
		Now:                         now.Add(time.Minute),
		RunIDPrefix:                 "run-audit-orchestration-supervisor-aggregate",
		TransportMode:               "metadata-only",
		MaxCandidates:               1,
		MaxRuns:                     2,
		LeaseDuration:               10 * time.Minute,
		IncludeLifecycleMaintenance: true,
	})
	if err != nil {
		t.Fatalf("create worker orchestration supervisor fixture: %v", err)
	}
	if supervisor.Summary.Runs != 2 || supervisor.Summary.OrchestrationArtifactsCreated != 2 || supervisor.Summary.LifecycleMaintenanceArtifactsCreated != 2 || supervisor.Summary.LifecycleAuditArtifactsCreated != 2 {
		t.Fatalf("expected populated orchestration supervisor fixture, got %#v", supervisor.Summary)
	}
	orchestrationsBefore := countJSON(t, filepath.Join(root, "runtime", "worker-orchestrations"))
	claimsBefore := countJSON(t, filepath.Join(root, "runtime", "worker-queue-claims"))
	outboxBefore := countJSON(t, filepath.Join(root, "runtime", "worker-transport-outbox"))

	audit, err := PlanWorkerLifecycleAudit(root, now.Add(3*time.Minute), DefaultWorkerLeaseStaleAfter)
	if err != nil {
		t.Fatalf("plan worker lifecycle audit: %v", err)
	}

	item, ok := workerLifecycleAuditFindItem(audit.Items, "worker-orchestration-supervisor", supervisor.Path)
	if !ok {
		t.Fatalf("expected audit to include worker orchestration supervisor item for %s, got %#v", supervisor.Path, audit.Items)
	}
	if item.OrchestrationRuns != supervisor.Summary.Runs ||
		item.MaxRuns != supervisor.Summary.MaxRuns ||
		item.OrchestrationsCreated != supervisor.Summary.OrchestrationArtifactsCreated ||
		item.LifecycleMaintenanceCreated != supervisor.Summary.LifecycleMaintenanceArtifactsCreated ||
		item.LifecycleAuditCreated != supervisor.Summary.LifecycleAuditArtifactsCreated ||
		item.ClaimedCount != supervisor.Summary.ClaimedCount ||
		item.DispatchedCount != supervisor.Summary.DispatchedCount ||
		item.SkippedCount != supervisor.Summary.SkippedCount ||
		item.FailedCount != supervisor.Summary.FailedCount {
		t.Fatalf("expected orchestration supervisor aggregate fields on audit item, got item=%#v summary=%#v", item, supervisor.Summary)
	}
	if item.LinkedPath == "" {
		t.Fatalf("expected orchestration supervisor item to keep linked orchestration path, got %#v", item)
	}
	if got := countJSON(t, filepath.Join(root, "runtime", "worker-orchestrations")); got != orchestrationsBefore {
		t.Fatalf("audit planning must not create orchestration artifacts: before=%d after=%d", orchestrationsBefore, got)
	}
	if got := countJSON(t, filepath.Join(root, "runtime", "worker-queue-claims")); got != claimsBefore {
		t.Fatalf("audit planning must not create queue claims: before=%d after=%d", claimsBefore, got)
	}
	if got := countJSON(t, filepath.Join(root, "runtime", "worker-transport-outbox")); got != outboxBefore {
		t.Fatalf("audit planning must not create worker outbox requests: before=%d after=%d", outboxBefore, got)
	}
}

func TestPlanWorkerLifecycleAuditCarriesWorkerOrchestrationAggregateFields(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Date(2026, 5, 31, 16, 45, 0, 0, time.UTC)
	registerWorkerLifecycleAuditOrchestrationWorkers(t, root, now)
	orchestration, err := CreateRuntimeWorkerOrchestration(root, RuntimeWorkerOrchestrationOptions{
		Execute:                     true,
		Now:                         now.Add(time.Minute),
		RunID:                       "run-audit-orchestration-aggregate",
		TransportMode:               "metadata-only",
		MaxCandidates:               1,
		LeaseDuration:               10 * time.Minute,
		IncludeLifecycleMaintenance: true,
	})
	if err != nil {
		t.Fatalf("create worker orchestration fixture: %v", err)
	}
	if orchestration.PlacementReady == 0 || orchestration.ClaimedCount != 1 || orchestration.DispatchedCount != 1 || orchestration.LifecycleMaintenancePath == "" || orchestration.LifecycleAuditPath == "" {
		t.Fatalf("expected populated worker orchestration fixture, got %#v", orchestration)
	}
	orchestrationsBefore := countJSON(t, filepath.Join(root, "runtime", "worker-orchestrations"))
	claimsBefore := countJSON(t, filepath.Join(root, "runtime", "worker-queue-claims"))
	outboxBefore := countJSON(t, filepath.Join(root, "runtime", "worker-transport-outbox"))

	audit, err := PlanWorkerLifecycleAudit(root, now.Add(3*time.Minute), DefaultWorkerLeaseStaleAfter)
	if err != nil {
		t.Fatalf("plan worker lifecycle audit: %v", err)
	}

	item, ok := workerLifecycleAuditFindItem(audit.Items, "worker-orchestration", orchestration.Path)
	if !ok {
		t.Fatalf("expected audit to include worker orchestration item for %s, got %#v", orchestration.Path, audit.Items)
	}
	if item.PlacementReady != orchestration.PlacementReady ||
		item.PlacementSkipped != orchestration.PlacementSkipped ||
		item.ClaimedCount != orchestration.ClaimedCount ||
		item.DispatchReady != orchestration.DispatchReady ||
		item.DispatchSkipped != orchestration.DispatchSkipped ||
		item.DispatchedCount != orchestration.DispatchedCount ||
		item.SkippedCount != orchestration.ClaimSkipped+orchestration.DispatchSkipped ||
		item.FailedCount != orchestration.ClaimFailed+orchestration.DispatchFailed ||
		item.LifecycleMaintenanceCreated != 1 ||
		item.LifecycleAuditCreated != 1 {
		t.Fatalf("expected worker orchestration aggregate fields on audit item, got item=%#v orchestration=%#v", item, orchestration)
	}
	if got := countJSON(t, filepath.Join(root, "runtime", "worker-orchestrations")); got != orchestrationsBefore {
		t.Fatalf("audit planning must not create orchestration artifacts: before=%d after=%d", orchestrationsBefore, got)
	}
	if got := countJSON(t, filepath.Join(root, "runtime", "worker-queue-claims")); got != claimsBefore {
		t.Fatalf("audit planning must not create queue claims: before=%d after=%d", claimsBefore, got)
	}
	if got := countJSON(t, filepath.Join(root, "runtime", "worker-transport-outbox")); got != outboxBefore {
		t.Fatalf("audit planning must not create worker outbox requests: before=%d after=%d", outboxBefore, got)
	}
}

func TestPlanWorkerLifecycleAuditIncludesWorkerDispatchRetryPlans(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Date(2026, 5, 31, 13, 45, 0, 0, time.UTC)
	marker := filepath.Join(t.TempDir(), "audit-retry-plan-ran")
	dispatch := createFileOutboxRuntimeWorkerQueueDispatchWithToolPlan(t, root, now, RuntimeWorkerToolPlanPayload{
		Action:     "discover_assets",
		Runner:     "command",
		Parser:     "web_api_parser",
		RunCommand: []string{"sh", "-c", "touch " + marker},
	})
	if _, err := SubmitRuntimeWorkerQueueDispatchResult(root, filepath.Base(dispatch.Path), RuntimeWorkerQueueDispatchResult{
		Status: "failed",
		Error:  "exit status 23",
	}, RuntimeWorkerQueueDispatchResultOptions{Now: now.Add(time.Minute)}); err != nil {
		t.Fatalf("submit failed worker dispatch result: %v", err)
	}
	retryPlan, err := CreateRuntimeWorkerDispatchRetryPlan(root, now.Add(2*time.Minute))
	if err != nil {
		t.Fatalf("create worker dispatch retry plan: %v", err)
	}

	audit, err := PlanWorkerLifecycleAudit(root, now.Add(3*time.Minute), DefaultWorkerLeaseStaleAfter)
	if err != nil {
		t.Fatalf("plan worker lifecycle audit: %v", err)
	}

	if audit.Summary.DispatchRetryPlans != 1 {
		t.Fatalf("expected one dispatch retry plan in audit summary, got %#v", audit.Summary)
	}
	if !workerLifecycleAuditHasItem(audit.Items, "worker-dispatch-retry-plan", retryPlan.Path) {
		t.Fatalf("expected audit to include dispatch retry plan item for %s, got %#v", retryPlan.Path, audit.Items)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("audit planning must not execute retry tool-plan command, marker stat err=%v", err)
	}
	plans, err := ListRuntimeWorkerDispatchRetryPlans(root)
	if err != nil {
		t.Fatalf("list worker dispatch retry plans after audit: %v", err)
	}
	if len(plans) != 1 || plans[0].Path != retryPlan.Path || plans[0].Summary.Ready != 1 {
		t.Fatalf("audit planning must not mutate retry plans, got %#v", plans)
	}
}

func TestPlanWorkerLifecycleAuditIncludesWorkerDispatchRetryDispatches(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Date(2026, 5, 31, 14, 20, 0, 0, time.UTC)
	dispatch := createFileOutboxRuntimeWorkerQueueDispatch(t, root, now)
	if _, err := SubmitRuntimeWorkerQueueDispatchResult(root, filepath.Base(dispatch.Path), RuntimeWorkerQueueDispatchResult{
		Status: "failed",
		Error:  "exit status 23",
	}, RuntimeWorkerQueueDispatchResultOptions{Now: now.Add(time.Minute)}); err != nil {
		t.Fatalf("submit failed worker dispatch result: %v", err)
	}
	retryPlan, err := CreateRuntimeWorkerDispatchRetryPlan(root, now.Add(2*time.Minute))
	if err != nil {
		t.Fatalf("create worker dispatch retry plan: %v", err)
	}
	retryDispatch, err := CreateRuntimeWorkerDispatchRetryDispatch(root, filepath.Base(retryPlan.Path), now.Add(3*time.Minute))
	if err != nil {
		t.Fatalf("create worker dispatch retry dispatch: %v", err)
	}

	audit, err := PlanWorkerLifecycleAudit(root, now.Add(4*time.Minute), DefaultWorkerLeaseStaleAfter)
	if err != nil {
		t.Fatalf("plan worker lifecycle audit: %v", err)
	}

	if audit.Summary.DispatchRetryDispatches != 1 {
		t.Fatalf("expected one dispatch retry dispatch in audit summary, got %#v", audit.Summary)
	}
	if !workerLifecycleAuditHasItem(audit.Items, "worker-dispatch-retry-dispatch", retryDispatch.Path) {
		t.Fatalf("expected audit to include dispatch retry dispatch item for %s, got %#v", retryDispatch.Path, audit.Items)
	}
	dispatches, err := ListRuntimeWorkerDispatchRetryDispatches(root)
	if err != nil {
		t.Fatalf("list worker dispatch retry dispatches after audit: %v", err)
	}
	if len(dispatches) != 1 || dispatches[0].Path != retryDispatch.Path || dispatches[0].RetryPlanPath != retryPlan.Path {
		t.Fatalf("audit planning must not mutate retry dispatches, got %#v", dispatches)
	}
}

func TestPlanWorkerLifecycleAuditIncludesWorkerDispatchRetryDispatchExecutions(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Date(2026, 5, 31, 14, 55, 0, 0, time.UTC)
	_, retryDispatch, _ := createRuntimeWorkerDispatchRetryDispatchFixture(t, root, now)
	execution, err := ExecuteRuntimeWorkerDispatchRetryDispatch(root, filepath.Base(retryDispatch.Path), RuntimeWorkerDispatchRetryDispatchExecutionOptions{
		Execute: true,
		Now:     now.Add(4 * time.Minute),
	})
	if err != nil {
		t.Fatalf("execute worker dispatch retry dispatch: %v", err)
	}
	claimsBefore := countJSON(t, filepath.Join(root, "runtime", "worker-queue-claims"))

	audit, err := PlanWorkerLifecycleAudit(root, now.Add(5*time.Minute), DefaultWorkerLeaseStaleAfter)
	if err != nil {
		t.Fatalf("plan worker lifecycle audit: %v", err)
	}

	if audit.Summary.DispatchRetryDispatchExecutions != 1 {
		t.Fatalf("expected one dispatch retry dispatch execution in audit summary, got %#v", audit.Summary)
	}
	if !workerLifecycleAuditHasItem(audit.Items, "worker-dispatch-retry-dispatch-execution", execution.Path) {
		t.Fatalf("expected audit to include dispatch retry dispatch execution item for %s, got %#v", execution.Path, audit.Items)
	}
	if got := countJSON(t, filepath.Join(root, "runtime", "worker-queue-claims")); got != claimsBefore {
		t.Fatalf("audit planning must not create additional queue claims: before=%d after=%d", claimsBefore, got)
	}
}

func TestPlanWorkerLifecycleAuditCarriesWorkerDispatchRetryAggregateFields(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Date(2026, 5, 31, 17, 5, 0, 0, time.UTC)
	failedDispatch := createFileOutboxRuntimeWorkerQueueDispatch(t, root, now)
	if _, err := SubmitRuntimeWorkerQueueDispatchResult(root, filepath.Base(failedDispatch.Path), RuntimeWorkerQueueDispatchResult{
		Status: "failed",
		Error:  "exit status 23",
	}, RuntimeWorkerQueueDispatchResultOptions{Now: now.Add(time.Minute)}); err != nil {
		t.Fatalf("submit failed worker dispatch result: %v", err)
	}
	completedDispatch := createFileOutboxRuntimeWorkerQueueDispatchForExecution(t, root, now.Add(2*time.Minute), "run-retry-audit-completed", "tool-retry-audit-completed")
	if _, err := SubmitRuntimeWorkerQueueDispatchResult(root, filepath.Base(completedDispatch.Path), RuntimeWorkerQueueDispatchResult{
		Status: "completed",
		Output: "already completed",
	}, RuntimeWorkerQueueDispatchResultOptions{Now: now.Add(3 * time.Minute)}); err != nil {
		t.Fatalf("submit completed worker dispatch result: %v", err)
	}
	retryPlan, err := CreateRuntimeWorkerDispatchRetryPlan(root, now.Add(4*time.Minute))
	if err != nil {
		t.Fatalf("create worker dispatch retry plan: %v", err)
	}
	retryDispatch, err := CreateRuntimeWorkerDispatchRetryDispatch(root, filepath.Base(retryPlan.Path), now.Add(5*time.Minute))
	if err != nil {
		t.Fatalf("create worker dispatch retry dispatch: %v", err)
	}
	retryExecution, err := ExecuteRuntimeWorkerDispatchRetryDispatch(root, filepath.Base(retryDispatch.Path), RuntimeWorkerDispatchRetryDispatchExecutionOptions{
		Execute: true,
		Now:     now.Add(6 * time.Minute),
	})
	if err != nil {
		t.Fatalf("execute worker dispatch retry dispatch: %v", err)
	}
	claimsBefore := countJSON(t, filepath.Join(root, "runtime", "worker-queue-claims"))

	audit, err := PlanWorkerLifecycleAudit(root, now.Add(7*time.Minute), DefaultWorkerLeaseStaleAfter)
	if err != nil {
		t.Fatalf("plan worker lifecycle audit: %v", err)
	}

	planItem, ok := workerLifecycleAuditFindItem(audit.Items, "worker-dispatch-retry-plan", retryPlan.Path)
	if !ok {
		t.Fatalf("expected audit to include worker dispatch retry plan item for %s, got %#v", retryPlan.Path, audit.Items)
	}
	if planItem.TotalCount != retryPlan.Summary.Total || planItem.ReadyCount != retryPlan.Summary.Ready || planItem.SkippedCount != retryPlan.Summary.Skipped {
		t.Fatalf("expected retry plan aggregate fields to match summary %#v, got %#v", retryPlan.Summary, planItem)
	}

	dispatchItem, ok := workerLifecycleAuditFindItem(audit.Items, "worker-dispatch-retry-dispatch", retryDispatch.Path)
	if !ok {
		t.Fatalf("expected audit to include worker dispatch retry dispatch item for %s, got %#v", retryDispatch.Path, audit.Items)
	}
	if dispatchItem.TotalCount != retryDispatch.Summary.Total || dispatchItem.ReadyCount != retryDispatch.Summary.Ready || dispatchItem.SkippedCount != retryDispatch.Summary.Skipped {
		t.Fatalf("expected retry dispatch aggregate fields to match summary %#v, got %#v", retryDispatch.Summary, dispatchItem)
	}

	executionItem, ok := workerLifecycleAuditFindItem(audit.Items, "worker-dispatch-retry-dispatch-execution", retryExecution.Path)
	if !ok {
		t.Fatalf("expected audit to include worker dispatch retry dispatch execution item for %s, got %#v", retryExecution.Path, audit.Items)
	}
	if executionItem.TotalCount != retryExecution.Summary.Total ||
		executionItem.ReadyCount != retryExecution.Summary.Ready ||
		executionItem.ClaimedCount != retryExecution.Summary.Claimed ||
		executionItem.SkippedCount != retryExecution.Summary.Skipped ||
		executionItem.FailedCount != retryExecution.Summary.Failed {
		t.Fatalf("expected retry dispatch execution aggregate fields to match summary %#v, got %#v", retryExecution.Summary, executionItem)
	}
	if got := countJSON(t, filepath.Join(root, "runtime", "worker-queue-claims")); got != claimsBefore {
		t.Fatalf("audit planning must not create additional queue claims: before=%d after=%d", claimsBefore, got)
	}
}

func TestPlanWorkerLifecycleAuditCarriesLeaseRecoveryAggregateFields(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Date(2026, 5, 31, 17, 40, 0, 0, time.UTC)
	staleAfter := 5 * time.Minute
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	if _, err := RegisterRuntimeWorker(root, RuntimeWorkerArtifact{
		WorkerID:     "remote-web-1",
		Mode:         "remote",
		Status:       "available",
		Endpoint:     "file://remote-web-1/inbox",
		Capabilities: []string{"web.discovery"},
	}, RuntimeWorkerRegistrationOptions{Now: now}); err != nil {
		t.Fatalf("register runtime worker: %v", err)
	}
	oldHeartbeat := now.Add(-staleAfter - time.Minute).Format(time.RFC3339Nano)
	freshHeartbeat := now.Format(time.RFC3339Nano)
	for _, lease := range []WorkerLeaseArtifact{
		{
			Version:     1,
			RunID:       "run-reclaim-ready",
			ExecutionID: "tool-reclaim-ready",
			Tool:        "web-api-pack",
			Capability:  "web.discovery",
			WorkerID:    "remote-web-1",
			Status:      "active",
			AcquiredAt:  oldHeartbeat,
			HeartbeatAt: oldHeartbeat,
		},
		{
			Version:     1,
			RunID:       "run-reclaim-skip",
			ExecutionID: "tool-reclaim-skip",
			Tool:        "web-api-pack",
			Capability:  "web.discovery",
			WorkerID:    "remote-web-1",
			Status:      "active",
			AcquiredAt:  freshHeartbeat,
			HeartbeatAt: freshHeartbeat,
		},
	} {
		if err := writeWorkerLeaseArtifact(root, lease); err != nil {
			t.Fatalf("write reclaim lease fixture: %v", err)
		}
	}
	reclaimReview, err := CreateWorkerLeaseReclaimDispatch(root, now, staleAfter)
	if err != nil {
		t.Fatalf("create reclaim review: %v", err)
	}
	if len(reclaimReview.Ready) != 1 || len(reclaimReview.Skipped) != 1 {
		t.Fatalf("expected reclaim review aggregate fixture, got %#v", reclaimReview)
	}
	reclaimExecution, err := ExecuteWorkerLeaseReclaimDispatch(root, filepath.Base(reclaimReview.Path), WorkerLeaseReclaimExecutionOptions{
		Execute:    true,
		Now:        now.Add(time.Minute),
		StaleAfter: staleAfter,
	})
	if err != nil {
		t.Fatalf("execute reclaim review: %v", err)
	}
	if len(reclaimExecution.Results) != 1 || len(reclaimExecution.Skipped) != 1 {
		t.Fatalf("expected reclaim execution aggregate fixture, got %#v", reclaimExecution)
	}

	readyDispatch := createFileOutboxRuntimeWorkerQueueDispatchForExecution(t, root, now.Add(2*time.Minute), "run-reconcile-ready", "tool-reconcile-ready")
	if _, err := SubmitRuntimeWorkerQueueDispatchResult(root, filepath.Base(readyDispatch.Path), RuntimeWorkerQueueDispatchResult{
		Status: "completed",
		Output: "ready result",
	}, RuntimeWorkerQueueDispatchResultOptions{Now: now.Add(3 * time.Minute)}); err != nil {
		t.Fatalf("submit ready reconciliation result: %v", err)
	}
	if err := writeWorkerLeaseArtifact(root, WorkerLeaseArtifact{
		Version:     1,
		RunID:       readyDispatch.RunID,
		ExecutionID: readyDispatch.ExecutionID,
		Tool:        readyDispatch.Tool,
		Capability:  readyDispatch.Capability,
		WorkerID:    readyDispatch.WorkerID,
		Status:      "active",
		AcquiredAt:  now.Add(2 * time.Minute).Format(time.RFC3339Nano),
		HeartbeatAt: now.Add(2 * time.Minute).Format(time.RFC3339Nano),
	}); err != nil {
		t.Fatalf("write active reconciliation lease fixture: %v", err)
	}
	skippedDispatch := createFileOutboxRuntimeWorkerQueueDispatchForExecution(t, root, now.Add(4*time.Minute), "run-reconcile-skip", "tool-reconcile-skip")
	if _, err := SubmitRuntimeWorkerQueueDispatchResult(root, filepath.Base(skippedDispatch.Path), RuntimeWorkerQueueDispatchResult{
		Status: "completed",
		Output: "already aligned",
	}, RuntimeWorkerQueueDispatchResultOptions{Now: now.Add(5 * time.Minute)}); err != nil {
		t.Fatalf("submit skipped reconciliation result: %v", err)
	}
	if err := writeWorkerLeaseArtifact(root, WorkerLeaseArtifact{
		Version:           1,
		RunID:             skippedDispatch.RunID,
		ExecutionID:       skippedDispatch.ExecutionID,
		ResultExecutionID: "aligned-result",
		Tool:              skippedDispatch.Tool,
		Capability:        skippedDispatch.Capability,
		WorkerID:          skippedDispatch.WorkerID,
		Status:            "completed",
		AcquiredAt:        now.Add(4 * time.Minute).Format(time.RFC3339Nano),
		HeartbeatAt:       now.Add(5 * time.Minute).Format(time.RFC3339Nano),
		ReleasedAt:        now.Add(5 * time.Minute).Format(time.RFC3339Nano),
	}); err != nil {
		t.Fatalf("write aligned reconciliation lease fixture: %v", err)
	}
	reconciliationReview, err := CreateWorkerLeaseResultReconciliationDispatch(root, now.Add(6*time.Minute))
	if err != nil {
		t.Fatalf("create reconciliation review: %v", err)
	}
	if len(reconciliationReview.Ready) != 1 || len(reconciliationReview.Skipped) != 1 {
		t.Fatalf("expected reconciliation review aggregate fixture, got %#v", reconciliationReview)
	}
	reconciliationExecution, err := ExecuteWorkerLeaseResultReconciliationDispatch(root, filepath.Base(reconciliationReview.Path), WorkerLeaseResultReconciliationExecutionOptions{
		Execute: true,
		Now:     now.Add(7 * time.Minute),
	})
	if err != nil {
		t.Fatalf("execute reconciliation review: %v", err)
	}
	if len(reconciliationExecution.Results) != 1 || len(reconciliationExecution.Skipped) != 1 {
		t.Fatalf("expected reconciliation execution aggregate fixture, got %#v", reconciliationExecution)
	}
	reclaimExecutionsBefore := countJSON(t, filepath.Join(root, "runtime", "worker-lease-reclaim-executions"))
	reconciliationExecutionsBefore := countJSON(t, filepath.Join(root, "runtime", "worker-lease-result-reconciliation-executions"))

	audit, err := PlanWorkerLifecycleAudit(root, now.Add(8*time.Minute), staleAfter)
	if err != nil {
		t.Fatalf("plan worker lifecycle audit: %v", err)
	}

	reclaimReviewItem, ok := workerLifecycleAuditFindItem(audit.Items, "worker-lease-reclaim-review", reclaimReview.Path)
	if !ok {
		t.Fatalf("expected audit to include reclaim review item for %s, got %#v", reclaimReview.Path, audit.Items)
	}
	if reclaimReviewItem.TotalCount != 2 || reclaimReviewItem.ReadyCount != 1 || reclaimReviewItem.SkippedCount != 1 {
		t.Fatalf("expected reclaim review aggregate fields, got %#v", reclaimReviewItem)
	}
	reclaimExecutionItem, ok := workerLifecycleAuditFindItem(audit.Items, "worker-lease-reclaim-execution", reclaimExecution.Path)
	if !ok {
		t.Fatalf("expected audit to include reclaim execution item for %s, got %#v", reclaimExecution.Path, audit.Items)
	}
	if reclaimExecutionItem.TotalCount != 2 || reclaimExecutionItem.ResultCount != 1 || reclaimExecutionItem.SkippedCount != 1 {
		t.Fatalf("expected reclaim execution aggregate fields, got %#v", reclaimExecutionItem)
	}
	reconciliationReviewItem, ok := workerLifecycleAuditFindItem(audit.Items, "worker-lease-result-reconciliation-review", reconciliationReview.Path)
	if !ok {
		t.Fatalf("expected audit to include reconciliation review item for %s, got %#v", reconciliationReview.Path, audit.Items)
	}
	if reconciliationReviewItem.TotalCount != 2 || reconciliationReviewItem.ReadyCount != 1 || reconciliationReviewItem.SkippedCount != 1 {
		t.Fatalf("expected reconciliation review aggregate fields, got %#v", reconciliationReviewItem)
	}
	reconciliationExecutionItem, ok := workerLifecycleAuditFindItem(audit.Items, "worker-lease-result-reconciliation-execution", reconciliationExecution.Path)
	if !ok {
		t.Fatalf("expected audit to include reconciliation execution item for %s, got %#v", reconciliationExecution.Path, audit.Items)
	}
	if reconciliationExecutionItem.TotalCount != 2 || reconciliationExecutionItem.ResultCount != 1 || reconciliationExecutionItem.SkippedCount != 1 {
		t.Fatalf("expected reconciliation execution aggregate fields, got %#v", reconciliationExecutionItem)
	}
	if got := countJSON(t, filepath.Join(root, "runtime", "worker-lease-reclaim-executions")); got != reclaimExecutionsBefore {
		t.Fatalf("audit planning must not create reclaim executions: before=%d after=%d", reclaimExecutionsBefore, got)
	}
	if got := countJSON(t, filepath.Join(root, "runtime", "worker-lease-result-reconciliation-executions")); got != reconciliationExecutionsBefore {
		t.Fatalf("audit planning must not create reconciliation executions: before=%d after=%d", reconciliationExecutionsBefore, got)
	}
}

func workerLifecycleAuditHasItem(items []WorkerLifecycleAuditItem, kind string, path string) bool {
	_, ok := workerLifecycleAuditFindItem(items, kind, path)
	return ok
}

func workerLifecycleAuditFindItem(items []WorkerLifecycleAuditItem, kind string, path string) (WorkerLifecycleAuditItem, bool) {
	for _, item := range items {
		if item.Kind == kind && item.ArtifactPath == path {
			return item, true
		}
	}
	return WorkerLifecycleAuditItem{}, false
}

func registerWorkerLifecycleAuditOrchestrationWorkers(t *testing.T, root string, now time.Time) {
	t.Helper()
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	for _, worker := range []RuntimeWorkerArtifact{
		{
			WorkerID:     "remote-web-1",
			Mode:         "remote",
			Status:       "available",
			Endpoint:     "https://web-worker.example.test/dispatch",
			Capabilities: []string{"web.discovery"},
		},
		{
			WorkerID:     "remote-infra-1",
			Mode:         "remote",
			Status:       "available",
			Endpoint:     "https://infra-worker.example.test/dispatch",
			Capabilities: []string{"infra.host_discovery"},
		},
	} {
		if _, err := RegisterRuntimeWorker(root, worker, RuntimeWorkerRegistrationOptions{Now: now}); err != nil {
			t.Fatalf("register worker %s: %v", worker.WorkerID, err)
		}
	}
}
