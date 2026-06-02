package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCreateWorkerLifecycleMaintenanceDryRunDoesNotCreateChildArtifactsOrMutate(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Date(2026, 5, 31, 9, 30, 0, 0, time.UTC)
	fixture := createWorkerLifecycleMaintenanceFixture(t, root, now)

	maintenance, err := CreateWorkerLifecycleMaintenance(root, WorkerLifecycleMaintenanceOptions{
		Now:        now.Add(5 * time.Minute),
		StaleAfter: 5 * time.Minute,
	})
	if err != nil {
		t.Fatalf("create dry-run worker lifecycle maintenance: %v", err)
	}
	if maintenance.Status != "dry-run" || maintenance.Mode != "metadata-only" {
		t.Fatalf("unexpected dry-run maintenance metadata: %#v", maintenance)
	}
	if maintenance.Summary.ActionsTotal != 5 || maintenance.Summary.AckActions != 1 || maintenance.Summary.RunActions != 1 || maintenance.Summary.DaemonControlActions != 1 || maintenance.Summary.ReclaimActions != 1 || maintenance.Summary.ReconciliationActions != 1 {
		t.Fatalf("unexpected dry-run maintenance summary: %#v", maintenance.Summary)
	}
	if maintenance.ReviewPath != "" || maintenance.ActionPlanPath != "" || maintenance.ReclaimDispatchPath != "" || maintenance.ReconciliationDispatchPath != "" {
		t.Fatalf("dry-run maintenance must not create child artifacts, got %#v", maintenance)
	}
	if !strings.Contains(maintenance.Path, filepath.Join("runtime", "worker-lifecycle-maintenance")) {
		t.Fatalf("expected maintenance artifact path, got %s", maintenance.Path)
	}
	if countJSON(t, filepath.Join(root, "runtime", "worker-lifecycle-reviews")) != 0 {
		t.Fatalf("dry-run maintenance must not create lifecycle review artifacts")
	}
	if countJSON(t, filepath.Join(root, "runtime", "worker-lifecycle-action-plans")) != 0 {
		t.Fatalf("dry-run maintenance must not create action-plan artifacts")
	}
	if countJSON(t, filepath.Join(root, "runtime", "worker-lease-reclaims")) != 0 {
		t.Fatalf("dry-run maintenance must not create reclaim review artifacts")
	}
	if countJSON(t, filepath.Join(root, "runtime", "worker-lease-result-reconciliations")) != 0 {
		t.Fatalf("dry-run maintenance must not create reconciliation review artifacts")
	}
	assertWorkerLifecycleMaintenanceFixtureUnmutated(t, root, fixture)
}

func TestCreateWorkerLifecycleMaintenanceExecuteWritesOnlyMetadataReviewArtifacts(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Date(2026, 5, 31, 9, 45, 0, 0, time.UTC)
	fixture := createWorkerLifecycleMaintenanceFixture(t, root, now)

	maintenance, err := CreateWorkerLifecycleMaintenance(root, WorkerLifecycleMaintenanceOptions{
		Execute:    true,
		Now:        now.Add(5 * time.Minute),
		StaleAfter: 5 * time.Minute,
	})
	if err != nil {
		t.Fatalf("execute worker lifecycle maintenance: %v", err)
	}
	if maintenance.Status != "completed" || maintenance.Mode != "metadata-only" {
		t.Fatalf("unexpected executed maintenance metadata: %#v", maintenance)
	}
	if maintenance.ReviewPath == "" || maintenance.ActionPlanPath == "" || maintenance.ReclaimDispatchPath == "" || maintenance.ReconciliationDispatchPath == "" {
		t.Fatalf("execute maintenance should link metadata-only child artifacts, got %#v", maintenance)
	}
	if maintenance.Summary.ReviewArtifactsCreated != 1 || maintenance.Summary.ActionPlanArtifactsCreated != 1 || maintenance.Summary.ReclaimDispatchArtifactsCreated != 1 || maintenance.Summary.ReconciliationDispatchArtifactsCreated != 1 {
		t.Fatalf("unexpected created artifact counts: %#v", maintenance.Summary)
	}
	if maintenance.Summary.SkippedUnsafeActions != 3 {
		t.Fatalf("expected ack/run/daemon-control actions to be skipped, got %#v", maintenance.Summary)
	}
	if len(maintenance.SkippedActions) != 3 {
		t.Fatalf("expected unsafe action skip details, got %#v", maintenance.SkippedActions)
	}
	if countJSON(t, filepath.Join(root, "runtime", "worker-lifecycle-reviews")) != 1 {
		t.Fatalf("execute maintenance should create one lifecycle review")
	}
	if countJSON(t, filepath.Join(root, "runtime", "worker-lifecycle-action-plans")) != 1 {
		t.Fatalf("execute maintenance should create one action plan")
	}
	if countJSON(t, filepath.Join(root, "runtime", "worker-lease-reclaims")) != 1 {
		t.Fatalf("execute maintenance should create one reclaim review")
	}
	if countJSON(t, filepath.Join(root, "runtime", "worker-lease-result-reconciliations")) != 1 {
		t.Fatalf("execute maintenance should create one reconciliation review")
	}
	if countJSON(t, filepath.Join(root, "runtime", "worker-lease-reclaim-executions")) != 0 {
		t.Fatalf("maintenance must not execute reclaim")
	}
	if countJSON(t, filepath.Join(root, "runtime", "worker-lease-result-reconciliation-executions")) != 0 {
		t.Fatalf("maintenance must not execute reconciliation release")
	}
	assertWorkerLifecycleMaintenanceFixtureUnmutated(t, root, fixture)
}

func TestCreateWorkerLifecycleMaintenanceExecutesLeaseRecoveryOnlyWithExplicitOption(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Date(2026, 6, 2, 0, 20, 0, 0, time.UTC)
	fixture := createWorkerLifecycleMaintenanceFixture(t, root, now)
	requestsBefore, err := ListRuntimeWorkerTransportDispatchRequests(root, "")
	if err != nil {
		t.Fatalf("list transport requests before maintenance: %v", err)
	}

	maintenance, err := CreateWorkerLifecycleMaintenance(root, WorkerLifecycleMaintenanceOptions{
		Execute:              true,
		ExecuteLeaseRecovery: true,
		Now:                  now.Add(5 * time.Minute),
		StaleAfter:           5 * time.Minute,
	})
	if err != nil {
		t.Fatalf("execute worker lifecycle maintenance with lease recovery: %v", err)
	}

	if maintenance.Summary.ReclaimExecutionsCreated != 1 || maintenance.Summary.ReclaimResultsCreated != 1 {
		t.Fatalf("expected one reclaim execution/result, got %#v", maintenance.Summary)
	}
	if maintenance.Summary.ReconciliationExecutionsCreated != 1 || maintenance.Summary.ReconciliationResultsCreated != 1 {
		t.Fatalf("expected one reconciliation execution/result, got %#v", maintenance.Summary)
	}
	if maintenance.ReclaimExecutionPath == "" || maintenance.ReconciliationExecutionPath == "" {
		t.Fatalf("expected linked lease recovery execution paths, got %#v", maintenance)
	}
	if countJSON(t, filepath.Join(root, "runtime", "worker-lease-reclaim-executions")) != 1 {
		t.Fatalf("explicit lease recovery should create one reclaim execution")
	}
	if countJSON(t, filepath.Join(root, "runtime", "worker-lease-result-reconciliation-executions")) != 1 {
		t.Fatalf("explicit lease recovery should create one reconciliation execution")
	}
	leases, err := ListWorkerLeases(root)
	if err != nil {
		t.Fatalf("list worker leases after explicit recovery: %v", err)
	}
	statuses := map[string]string{}
	for _, lease := range leases {
		statuses[lease.ExecutionID] = lease.Status
	}
	if statuses["tool-maintenance-stale"] != "reclaimed" || statuses[fixture.ReconcileID] != "completed" {
		t.Fatalf("expected stale lease reclaimed and result lease released, got %#v", statuses)
	}
	requestsAfter, err := ListRuntimeWorkerTransportDispatchRequests(root, "")
	if err != nil {
		t.Fatalf("list transport requests after maintenance: %v", err)
	}
	if len(requestsAfter) != len(requestsBefore) {
		t.Fatalf("explicit lease recovery must not create transport requests: before=%d after=%d", len(requestsBefore), len(requestsAfter))
	}
	controls, err := ListRuntimeWorkerTransportDaemonControlRequests(root, fixture.DaemonWorkerID)
	if err != nil {
		t.Fatalf("list daemon controls after explicit recovery: %v", err)
	}
	if len(controls) != 1 || controls[0].Status != "pending" || controls[0].AppliedAt != "" {
		t.Fatalf("explicit lease recovery must not apply daemon controls, got %#v", controls)
	}
}

func TestCreateWorkerLifecycleMaintenanceSkipsWorkerDispatchRetryDispatchExecution(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Date(2026, 5, 31, 15, 50, 0, 0, time.UTC)
	marker := filepath.Join(t.TempDir(), "maintenance-retry-dispatch-ran")
	_, retryDispatch, _ := createRuntimeWorkerDispatchRetryDispatchFixture(t, root, now, RuntimeWorkerToolPlanPayload{
		Action:     "discover_assets",
		Runner:     "command",
		Parser:     "web_api_parser",
		RunCommand: []string{"sh", "-c", "touch " + marker},
	})
	claimsBefore := countJSON(t, filepath.Join(root, "runtime", "worker-queue-claims"))

	maintenance, err := CreateWorkerLifecycleMaintenance(root, WorkerLifecycleMaintenanceOptions{
		Execute: true,
		Now:     now.Add(5 * time.Minute),
	})
	if err != nil {
		t.Fatalf("execute worker lifecycle maintenance with retry-dispatch action: %v", err)
	}

	if maintenance.Summary.ActionsTotal != 1 || maintenance.Summary.RetryDispatchExecuteActions != 1 || maintenance.Summary.SkippedUnsafeActions != 1 {
		t.Fatalf("expected retry-dispatch execute action to be counted and skipped, got %#v", maintenance.Summary)
	}
	if len(maintenance.SkippedActions) != 1 {
		t.Fatalf("expected one skipped retry-dispatch action, got %#v", maintenance.SkippedActions)
	}
	skipped := maintenance.SkippedActions[0]
	if skipped.Type != "execute-dispatch-retry-dispatch" || skipped.Gate != "worker-dispatch-retry-dispatch-execute" || skipped.Reason != "requires-explicit-operator-gate" {
		t.Fatalf("unexpected retry-dispatch skipped action: %#v", skipped)
	}
	if skipped.DispatchPath != retryDispatch.Path {
		t.Fatalf("expected skipped retry-dispatch path %s, got %#v", retryDispatch.Path, skipped)
	}
	if got := countJSON(t, filepath.Join(root, "runtime", "worker-queue-claims")); got != claimsBefore {
		t.Fatalf("maintenance must not create retry queue claims: before=%d after=%d", claimsBefore, got)
	}
	if executions, err := ListRuntimeWorkerDispatchRetryDispatchExecutions(root); err != nil {
		t.Fatalf("list retry-dispatch executions after maintenance: %v", err)
	} else if len(executions) != 0 {
		t.Fatalf("maintenance must not execute retry dispatch artifacts, got %#v", executions)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("maintenance must not execute retry tool-plan command, marker stat err=%v", err)
	}
}

func TestCreateWorkerLifecycleMaintenanceExecutesRetryDispatchesOnlyWithExplicitOption(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Date(2026, 6, 1, 15, 5, 0, 0, time.UTC)
	marker := filepath.Join(t.TempDir(), "maintenance-retry-dispatch-explicit-ran")
	_, retryDispatch, _ := createRuntimeWorkerDispatchRetryDispatchFixture(t, root, now, RuntimeWorkerToolPlanPayload{
		Action:     "discover_assets",
		Runner:     "command",
		Parser:     "web_api_parser",
		RunCommand: []string{"sh", "-c", "touch " + marker},
	})
	claimsBefore := countJSON(t, filepath.Join(root, "runtime", "worker-queue-claims"))
	requestsBefore, err := ListRuntimeWorkerTransportDispatchRequests(root, "")
	if err != nil {
		t.Fatalf("list transport requests before maintenance: %v", err)
	}

	maintenance, err := CreateWorkerLifecycleMaintenance(root, WorkerLifecycleMaintenanceOptions{
		Execute:                true,
		ExecuteRetryDispatches: true,
		Now:                    now.Add(5 * time.Minute),
	})
	if err != nil {
		t.Fatalf("execute worker lifecycle maintenance with explicit retry-dispatch execution: %v", err)
	}

	if maintenance.Summary.ActionsTotal != 1 || maintenance.Summary.RetryDispatchExecuteActions != 1 {
		t.Fatalf("expected one retry-dispatch action, got %#v", maintenance.Summary)
	}
	if maintenance.Summary.SkippedUnsafeActions != 0 || len(maintenance.SkippedActions) != 0 {
		t.Fatalf("explicit retry-dispatch execution should not be recorded as skipped, got summary=%#v skipped=%#v", maintenance.Summary, maintenance.SkippedActions)
	}
	if maintenance.Summary.RetryDispatchExecutionsCreated != 1 || maintenance.Summary.RetryDispatchClaimsCreated != 1 {
		t.Fatalf("expected retry-dispatch execution and claim counts, got %#v", maintenance.Summary)
	}
	if len(maintenance.RetryDispatchExecutionPaths) != 1 {
		t.Fatalf("expected linked retry-dispatch execution path, got %#v", maintenance)
	}
	executions, err := ListRuntimeWorkerDispatchRetryDispatchExecutions(root)
	if err != nil {
		t.Fatalf("list retry-dispatch executions after maintenance: %v", err)
	}
	if len(executions) != 1 || executions[0].RetryDispatchPath != retryDispatch.Path || executions[0].Summary.Claimed != 1 || executions[0].Summary.Dispatched != 0 {
		t.Fatalf("expected one metadata-only retry-dispatch execution that created one claim and did not dispatch, got %#v", executions)
	}
	if maintenance.RetryDispatchExecutionPaths[0] != executions[0].Path {
		t.Fatalf("expected maintenance to link execution %s, got %#v", executions[0].Path, maintenance.RetryDispatchExecutionPaths)
	}
	if got := countJSON(t, filepath.Join(root, "runtime", "worker-queue-claims")); got != claimsBefore+1 {
		t.Fatalf("expected exactly one retry queue claim: before=%d after=%d", claimsBefore, got)
	}
	requestsAfter, err := ListRuntimeWorkerTransportDispatchRequests(root, "")
	if err != nil {
		t.Fatalf("list transport requests after maintenance: %v", err)
	}
	if len(requestsAfter) != len(requestsBefore) {
		t.Fatalf("explicit retry-dispatch execution must not dispatch retry claims or create file-outbox requests: before=%d after=%d", len(requestsBefore), len(requestsAfter))
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("explicit retry-dispatch maintenance must not execute retry tool-plan command, marker stat err=%v", err)
	}
}

func TestCreateWorkerLifecycleMaintenanceDispatchesRetryClaimsOnlyWithExplicitHandoff(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Date(2026, 6, 2, 11, 0, 0, 0, time.UTC)
	marker := filepath.Join(t.TempDir(), "maintenance-retry-handoff-ran")
	_, retryDispatch, _ := createRuntimeWorkerDispatchRetryDispatchFixture(t, root, now, RuntimeWorkerToolPlanPayload{
		Action:     "discover_assets",
		Runner:     "command",
		Parser:     "web_api_parser",
		RunCommand: []string{"sh", "-c", "touch " + marker},
	})
	claimsBefore := countJSON(t, filepath.Join(root, "runtime", "worker-queue-claims"))
	queueDispatchesBefore, err := ListRuntimeWorkerQueueDispatchExecutions(root)
	if err != nil {
		t.Fatalf("list queue dispatches before maintenance: %v", err)
	}
	requestsBefore, err := ListRuntimeWorkerTransportDispatchRequests(root, "")
	if err != nil {
		t.Fatalf("list transport requests before maintenance: %v", err)
	}
	resultsBefore := countJSON(t, filepath.Join(root, "runtime", "worker-queue-dispatch-results"))

	maintenance, err := CreateWorkerLifecycleMaintenance(root, WorkerLifecycleMaintenanceOptions{
		Execute:                    true,
		ExecuteRetryDispatches:     true,
		DispatchRetryClaims:        true,
		RetryDispatchTransportMode: "file-outbox",
		Now:                        now.Add(5 * time.Minute),
	})
	if err != nil {
		t.Fatalf("execute worker lifecycle maintenance with explicit retry claim handoff: %v", err)
	}

	if !maintenance.DispatchRetryClaims || maintenance.RetryDispatchTransportMode != "file-outbox" {
		t.Fatalf("expected maintenance to persist retry handoff intent, got %#v", maintenance)
	}
	if maintenance.Summary.RetryDispatchExecutionsCreated != 1 ||
		maintenance.Summary.RetryDispatchClaimsCreated != 1 ||
		maintenance.Summary.RetryDispatchClaimsDispatched != 1 ||
		maintenance.Summary.RetryDispatchTransportRequestsCreated != 1 {
		t.Fatalf("expected retry-dispatch execution/claim/handoff counts, got %#v", maintenance.Summary)
	}
	executions, err := ListRuntimeWorkerDispatchRetryDispatchExecutions(root)
	if err != nil {
		t.Fatalf("list retry-dispatch executions after maintenance: %v", err)
	}
	if len(executions) != 1 || executions[0].RetryDispatchPath != retryDispatch.Path || executions[0].Summary.Claimed != 1 || executions[0].Summary.Dispatched != 1 {
		t.Fatalf("expected one retry-dispatch execution that dispatched one retry claim, got %#v", executions)
	}
	if len(executions[0].Results) != 1 ||
		executions[0].Results[0].Status != "dispatched" ||
		executions[0].Results[0].RetryTransportMode != "file-outbox" ||
		executions[0].Results[0].RetryQueueDispatchPath == "" ||
		executions[0].Results[0].RetryTransportRequestPath == "" {
		t.Fatalf("expected dispatched retry handoff result, got %#v", executions[0].Results)
	}
	queueDispatchesAfter, err := ListRuntimeWorkerQueueDispatchExecutions(root)
	if err != nil {
		t.Fatalf("list queue dispatches after maintenance: %v", err)
	}
	if len(queueDispatchesAfter) != len(queueDispatchesBefore)+1 {
		t.Fatalf("expected one retry queue dispatch execution: before=%d after=%d", len(queueDispatchesBefore), len(queueDispatchesAfter))
	}
	requestsAfter, err := ListRuntimeWorkerTransportDispatchRequests(root, "")
	if err != nil {
		t.Fatalf("list transport requests after maintenance: %v", err)
	}
	if len(requestsAfter) != len(requestsBefore)+1 {
		t.Fatalf("expected one pending retry file-outbox request: before=%d after=%d", len(requestsBefore), len(requestsAfter))
	}
	var retryRequest RuntimeWorkerTransportDispatchRequest
	for _, request := range requestsAfter {
		if request.Path == executions[0].Results[0].RetryTransportRequestPath {
			retryRequest = request
			break
		}
	}
	if retryRequest.Path == "" || retryRequest.Status != "pending" || retryRequest.AckedAt != "" || retryRequest.ResultPath != "" {
		t.Fatalf("expected pending unacknowledged retry handoff request, got %#v", retryRequest)
	}
	if got := countJSON(t, filepath.Join(root, "runtime", "worker-queue-claims")); got != claimsBefore+1 {
		t.Fatalf("expected exactly one retry queue claim: before=%d after=%d", claimsBefore, got)
	}
	if got := countJSON(t, filepath.Join(root, "runtime", "worker-queue-dispatch-results")); got != resultsBefore {
		t.Fatalf("retry handoff must not create worker result artifacts: before=%d after=%d", resultsBefore, got)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("retry handoff maintenance must not execute retry tool-plan command, marker stat err=%v", err)
	}
}

func TestCreateWorkerLifecycleMaintenanceRejectsRetryHandoffBeforeWritingArtifacts(t *testing.T) {
	now := time.Date(2026, 6, 2, 11, 15, 0, 0, time.UTC)
	tests := []struct {
		name    string
		options WorkerLifecycleMaintenanceOptions
		want    string
	}{
		{
			name: "without execute",
			options: WorkerLifecycleMaintenanceOptions{
				ExecuteRetryDispatches:     true,
				DispatchRetryClaims:        true,
				RetryDispatchTransportMode: "file-outbox",
			},
			want: "requires explicit execute",
		},
		{
			name: "without retry execution gate",
			options: WorkerLifecycleMaintenanceOptions{
				Execute:                    true,
				DispatchRetryClaims:        true,
				RetryDispatchTransportMode: "file-outbox",
			},
			want: "requires retry-dispatch execution",
		},
		{
			name: "http-json",
			options: WorkerLifecycleMaintenanceOptions{
				Execute:                    true,
				ExecuteRetryDispatches:     true,
				DispatchRetryClaims:        true,
				RetryDispatchTransportMode: "http-json",
			},
			want: "supports metadata-only or file-outbox transport",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "workspace")
			_, _, _ = createRuntimeWorkerDispatchRetryDispatchFixture(t, root, now)
			tc.options.Now = now.Add(5 * time.Minute)
			_, err := CreateWorkerLifecycleMaintenance(root, tc.options)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected retry handoff validation error %q, got %v", tc.want, err)
			}
			if got := countJSON(t, filepath.Join(root, "runtime", "worker-lifecycle-maintenance")); got != 0 {
				t.Fatalf("invalid retry handoff must not write maintenance artifacts, got %d", got)
			}
			if got := countJSON(t, filepath.Join(root, "runtime", "worker-lifecycle-reviews")); got != 0 {
				t.Fatalf("invalid retry handoff must not write review artifacts, got %d", got)
			}
			if got := countJSON(t, filepath.Join(root, "runtime", "worker-lifecycle-action-plans")); got != 0 {
				t.Fatalf("invalid retry handoff must not write action-plan artifacts, got %d", got)
			}
			if got := countJSON(t, filepath.Join(root, "runtime", "worker-dispatch-retry-dispatch-executions")); got != 0 {
				t.Fatalf("invalid retry handoff must not write retry execution artifacts, got %d", got)
			}
			if got := countJSON(t, filepath.Join(root, "runtime", "worker-transport-outbox")); got != 0 {
				t.Fatalf("invalid retry handoff must not write outbox artifacts, got %d", got)
			}
		})
	}
}

func TestReadWorkerLifecycleMaintenanceRejectsPathOutsideDir(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if _, err := ReadWorkerLifecycleMaintenance(root, filepath.Join("..", "maintenance.json")); err == nil || !strings.Contains(err.Error(), "worker lifecycle maintenance path must stay under") {
		t.Fatalf("expected maintenance path confinement error, got %v", err)
	}
}

type workerLifecycleMaintenanceFixture struct {
	PendingExecutionID string
	AckedExecutionID   string
	ReconcileID        string
	DaemonWorkerID     string
}

func createWorkerLifecycleMaintenanceFixture(t *testing.T, root string, now time.Time) workerLifecycleMaintenanceFixture {
	t.Helper()
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
	staleHeartbeat := now.Add(-10 * time.Minute).Format(time.RFC3339Nano)
	if err := writeWorkerLeaseArtifact(root, WorkerLeaseArtifact{
		Version:     1,
		RunID:       "run-maintenance-stale",
		ExecutionID: "tool-maintenance-stale",
		Tool:        "web-api-pack",
		Capability:  "web.discovery",
		WorkerID:    "remote-maintenance",
		Status:      "active",
		AcquiredAt:  staleHeartbeat,
		HeartbeatAt: staleHeartbeat,
	}); err != nil {
		t.Fatalf("write stale lease fixture: %v", err)
	}
	pendingDispatch := createFileOutboxRuntimeWorkerQueueDispatchForExecution(t, root, now, "run-maintenance-pending", "tool-maintenance-pending")
	ackedDispatch := createFileOutboxRuntimeWorkerQueueDispatchForExecution(t, root, now.Add(time.Minute), "run-maintenance-acked", "tool-maintenance-acked")
	if _, err := AcknowledgeRuntimeWorkerTransportDispatchRequest(root, ackedDispatch.WorkerID, filepath.Base(ackedDispatch.TransportRequestPath), RuntimeWorkerTransportAckOptions{
		Ack: true,
		Now: now.Add(2 * time.Minute),
	}); err != nil {
		t.Fatalf("ack worker transport request fixture: %v", err)
	}
	reconcileDispatch := createFileOutboxRuntimeWorkerQueueDispatchForExecution(t, root, now.Add(3*time.Minute), "run-maintenance-reconcile", "tool-maintenance-reconcile")
	if _, err := SubmitRuntimeWorkerQueueDispatchResult(root, filepath.Base(reconcileDispatch.Path), RuntimeWorkerQueueDispatchResult{
		Status: "completed",
		Output: "remote result complete",
	}, RuntimeWorkerQueueDispatchResultOptions{Now: now.Add(4 * time.Minute)}); err != nil {
		t.Fatalf("submit dispatch result fixture: %v", err)
	}
	if err := writeWorkerLeaseArtifact(root, WorkerLeaseArtifact{
		Version:     1,
		RunID:       reconcileDispatch.RunID,
		ExecutionID: reconcileDispatch.ExecutionID,
		Tool:        reconcileDispatch.Tool,
		Capability:  reconcileDispatch.Capability,
		WorkerID:    reconcileDispatch.WorkerID,
		Status:      "active",
		AcquiredAt:  now.Format(time.RFC3339Nano),
		HeartbeatAt: now.Format(time.RFC3339Nano),
	}); err != nil {
		t.Fatalf("write reconciliation lease fixture: %v", err)
	}
	if _, err := CreateRuntimeWorkerTransportDaemonControlRequest(root, "remote-maintenance-daemon", RuntimeWorkerTransportDaemonControlRequest{
		Action:      "restart",
		Reason:      "maintenance policy",
		RequestedBy: "operator",
	}, RuntimeWorkerTransportDaemonControlRequestOptions{Now: now}); err != nil {
		t.Fatalf("create daemon control fixture: %v", err)
	}
	return workerLifecycleMaintenanceFixture{
		PendingExecutionID: pendingDispatch.ExecutionID,
		AckedExecutionID:   ackedDispatch.ExecutionID,
		ReconcileID:        reconcileDispatch.ExecutionID,
		DaemonWorkerID:     "remote-maintenance-daemon",
	}
}

func assertWorkerLifecycleMaintenanceFixtureUnmutated(t *testing.T, root string, fixture workerLifecycleMaintenanceFixture) {
	t.Helper()
	requests, err := ListRuntimeWorkerTransportDispatchRequests(root, "")
	if err != nil {
		t.Fatalf("list transport requests: %v", err)
	}
	statuses := map[string]string{}
	for _, request := range requests {
		statuses[request.ExecutionID] = request.Status
	}
	if statuses[fixture.PendingExecutionID] != "pending" || statuses[fixture.AckedExecutionID] != "acknowledged" || statuses[fixture.ReconcileID] != "completed" {
		t.Fatalf("maintenance must not mutate transport requests, got %#v", statuses)
	}
	leases, err := ListWorkerLeases(root)
	if err != nil {
		t.Fatalf("list worker leases: %v", err)
	}
	for _, lease := range leases {
		if lease.Status != "active" {
			t.Fatalf("maintenance must not mutate leases, got %#v", leases)
		}
	}
	controls, err := ListRuntimeWorkerTransportDaemonControlRequests(root, fixture.DaemonWorkerID)
	if err != nil {
		t.Fatalf("list daemon controls: %v", err)
	}
	if len(controls) != 1 || controls[0].Status != "pending" || controls[0].AppliedAt != "" {
		t.Fatalf("maintenance must not apply daemon controls, got %#v", controls)
	}
}
