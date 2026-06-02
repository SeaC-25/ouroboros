package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCreateWorkerLifecycleMaintenanceSupervisorDryRunWritesAuditWithoutChildArtifacts(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Date(2026, 5, 31, 10, 10, 0, 0, time.UTC)
	fixture := createWorkerLifecycleMaintenanceFixture(t, root, now)

	supervisor, err := CreateWorkerLifecycleMaintenanceSupervisor(root, WorkerLifecycleMaintenanceSupervisorOptions{
		Now:        now.Add(5 * time.Minute),
		StaleAfter: 5 * time.Minute,
		MaxRuns:    2,
	})
	if err != nil {
		t.Fatalf("create dry-run worker lifecycle maintenance supervisor: %v", err)
	}
	if supervisor.Status != "dry-run" || supervisor.Mode != "metadata-only" {
		t.Fatalf("unexpected dry-run supervisor metadata: %#v", supervisor)
	}
	if supervisor.Summary.Runs != 0 || supervisor.Summary.MaxRuns != 2 || supervisor.Summary.MaintenanceArtifactsCreated != 0 || supervisor.Summary.AuditArtifactsCreated != 0 {
		t.Fatalf("unexpected dry-run supervisor summary: %#v", supervisor.Summary)
	}
	if len(supervisor.MaintenancePaths) != 0 || len(supervisor.AuditPaths) != 0 {
		t.Fatalf("dry-run supervisor must not link child artifacts, got %#v", supervisor)
	}
	if !strings.Contains(supervisor.Path, filepath.Join("runtime", "worker-lifecycle-maintenance-supervisor-runs")) {
		t.Fatalf("expected supervisor artifact path, got %s", supervisor.Path)
	}
	if countJSON(t, filepath.Join(root, "runtime", "worker-lifecycle-maintenance")) != 0 {
		t.Fatalf("dry-run supervisor must not create maintenance artifacts")
	}
	if countJSON(t, filepath.Join(root, "runtime", "worker-lifecycle-audits")) != 0 {
		t.Fatalf("dry-run supervisor must not create lifecycle audit artifacts")
	}
	if countJSON(t, filepath.Join(root, "runtime", "worker-lifecycle-reviews")) != 0 {
		t.Fatalf("dry-run supervisor must not create lifecycle review artifacts")
	}
	assertWorkerLifecycleMaintenanceFixtureUnmutated(t, root, fixture)
}

func TestCreateWorkerLifecycleMaintenanceSupervisorExecuteRunsBoundedMetadataOnlyMaintenanceAndAudit(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Date(2026, 5, 31, 10, 20, 0, 0, time.UTC)
	fixture := createWorkerLifecycleMaintenanceFixture(t, root, now)

	supervisor, err := CreateWorkerLifecycleMaintenanceSupervisor(root, WorkerLifecycleMaintenanceSupervisorOptions{
		Execute:    true,
		Now:        now.Add(5 * time.Minute),
		StaleAfter: 5 * time.Minute,
		MaxRuns:    2,
	})
	if err != nil {
		t.Fatalf("execute worker lifecycle maintenance supervisor: %v", err)
	}
	if supervisor.Status != "completed" || supervisor.Mode != "metadata-only" {
		t.Fatalf("unexpected executed supervisor metadata: %#v", supervisor)
	}
	if supervisor.Summary.Runs != 2 || supervisor.Summary.MaxRuns != 2 || supervisor.Summary.MaintenanceArtifactsCreated != 2 || supervisor.Summary.AuditArtifactsCreated != 2 {
		t.Fatalf("unexpected executed supervisor summary: %#v", supervisor.Summary)
	}
	if len(supervisor.MaintenancePaths) != 2 || len(supervisor.AuditPaths) != 2 {
		t.Fatalf("expected linked maintenance and audit artifacts, got %#v", supervisor)
	}
	if countJSON(t, filepath.Join(root, "runtime", "worker-lifecycle-maintenance")) != 2 {
		t.Fatalf("execute supervisor should create two maintenance artifacts")
	}
	if countJSON(t, filepath.Join(root, "runtime", "worker-lifecycle-audits")) != 2 {
		t.Fatalf("execute supervisor should create two lifecycle audit artifacts")
	}
	if countJSON(t, filepath.Join(root, "runtime", "worker-lease-reclaim-executions")) != 0 {
		t.Fatalf("supervisor must not execute reclaim")
	}
	if countJSON(t, filepath.Join(root, "runtime", "worker-lease-result-reconciliation-executions")) != 0 {
		t.Fatalf("supervisor must not execute reconciliation release")
	}
	read, err := ReadWorkerLifecycleMaintenanceSupervisor(root, filepath.Base(supervisor.Path))
	if err != nil {
		t.Fatalf("read worker lifecycle maintenance supervisor: %v", err)
	}
	if read.Path != supervisor.Path || read.Summary.Runs != 2 {
		t.Fatalf("unexpected persisted supervisor: %#v", read)
	}
	runs, err := ListWorkerLifecycleMaintenanceSupervisors(root)
	if err != nil {
		t.Fatalf("list worker lifecycle maintenance supervisors: %v", err)
	}
	if len(runs) != 1 || runs[0].Path != supervisor.Path {
		t.Fatalf("expected one persisted supervisor run, got %#v", runs)
	}
	assertWorkerLifecycleMaintenanceFixtureUnmutated(t, root, fixture)
}

func TestCreateWorkerLifecycleMaintenanceSupervisorExecutesLeaseRecoveryOnlyWithExplicitOption(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Date(2026, 6, 2, 0, 30, 0, 0, time.UTC)
	fixture := createWorkerLifecycleMaintenanceFixture(t, root, now)
	requestsBefore, err := ListRuntimeWorkerTransportDispatchRequests(root, "")
	if err != nil {
		t.Fatalf("list transport requests before supervisor: %v", err)
	}

	supervisor, err := CreateWorkerLifecycleMaintenanceSupervisor(root, WorkerLifecycleMaintenanceSupervisorOptions{
		Execute:              true,
		ExecuteLeaseRecovery: true,
		Now:                  now.Add(5 * time.Minute),
		StaleAfter:           5 * time.Minute,
		MaxRuns:              1,
	})
	if err != nil {
		t.Fatalf("execute worker lifecycle maintenance supervisor with lease recovery: %v", err)
	}

	if supervisor.Summary.Runs != 1 || supervisor.Summary.MaintenanceArtifactsCreated != 1 || supervisor.Summary.AuditArtifactsCreated != 1 {
		t.Fatalf("expected one bounded supervisor pass, got %#v", supervisor.Summary)
	}
	if supervisor.Summary.ReclaimExecutionsCreated != 1 || supervisor.Summary.ReclaimResultsCreated != 1 {
		t.Fatalf("expected one reclaim execution/result, got %#v", supervisor.Summary)
	}
	if supervisor.Summary.ReconciliationExecutionsCreated != 1 || supervisor.Summary.ReconciliationResultsCreated != 1 {
		t.Fatalf("expected one reconciliation execution/result, got %#v", supervisor.Summary)
	}
	if len(supervisor.ReclaimExecutionPaths) != 1 || len(supervisor.ReconciliationExecutionPaths) != 1 {
		t.Fatalf("expected linked lease recovery execution paths, got %#v", supervisor)
	}
	leases, err := ListWorkerLeases(root)
	if err != nil {
		t.Fatalf("list worker leases after explicit recovery supervisor: %v", err)
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
		t.Fatalf("list transport requests after supervisor: %v", err)
	}
	if len(requestsAfter) != len(requestsBefore) {
		t.Fatalf("explicit lease recovery supervisor must not create transport requests: before=%d after=%d", len(requestsBefore), len(requestsAfter))
	}
	controls, err := ListRuntimeWorkerTransportDaemonControlRequests(root, fixture.DaemonWorkerID)
	if err != nil {
		t.Fatalf("list daemon controls after explicit recovery supervisor: %v", err)
	}
	if len(controls) != 1 || controls[0].Status != "pending" || controls[0].AppliedAt != "" {
		t.Fatalf("explicit lease recovery supervisor must not apply daemon controls, got %#v", controls)
	}
}

func TestCreateWorkerLifecycleMaintenanceSupervisorAggregatesSkippedRetryDispatchExecution(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Date(2026, 5, 31, 15, 35, 0, 0, time.UTC)
	marker := filepath.Join(t.TempDir(), "maintenance-supervisor-retry-dispatch-ran")
	createRuntimeWorkerDispatchRetryDispatchFixture(t, root, now, RuntimeWorkerToolPlanPayload{
		Action:     "discover_assets",
		Runner:     "command",
		Parser:     "web_api_parser",
		RunCommand: []string{"sh", "-c", "touch " + marker},
	})
	claimsBefore := countJSON(t, filepath.Join(root, "runtime", "worker-queue-claims"))

	supervisor, err := CreateWorkerLifecycleMaintenanceSupervisor(root, WorkerLifecycleMaintenanceSupervisorOptions{
		Execute: true,
		Now:     now.Add(5 * time.Minute),
		MaxRuns: 2,
	})
	if err != nil {
		t.Fatalf("execute worker lifecycle maintenance supervisor with retry-dispatch action: %v", err)
	}

	if supervisor.Summary.Runs != 2 || supervisor.Summary.ActionsTotal != 2 || supervisor.Summary.RetryDispatchExecuteActions != 2 || supervisor.Summary.SkippedUnsafeActions != 2 {
		t.Fatalf("expected supervisor to aggregate retry-dispatch skipped actions, got %#v", supervisor.Summary)
	}
	if countJSON(t, filepath.Join(root, "runtime", "worker-lifecycle-maintenance")) != 2 {
		t.Fatalf("expected supervisor to create two maintenance artifacts")
	}
	if got := countJSON(t, filepath.Join(root, "runtime", "worker-queue-claims")); got != claimsBefore {
		t.Fatalf("supervisor must not create retry queue claims: before=%d after=%d", claimsBefore, got)
	}
	if executions, err := ListRuntimeWorkerDispatchRetryDispatchExecutions(root); err != nil {
		t.Fatalf("list retry-dispatch executions after supervisor: %v", err)
	} else if len(executions) != 0 {
		t.Fatalf("supervisor must not execute retry dispatch artifacts, got %#v", executions)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("supervisor must not execute retry tool-plan command, marker stat err=%v", err)
	}
}

func TestCreateWorkerLifecycleMaintenanceSupervisorExecutesRetryDispatchesOnlyWithExplicitOption(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Date(2026, 6, 1, 15, 20, 0, 0, time.UTC)
	marker := filepath.Join(t.TempDir(), "maintenance-supervisor-retry-dispatch-explicit-ran")
	_, retryDispatch, _ := createRuntimeWorkerDispatchRetryDispatchFixture(t, root, now, RuntimeWorkerToolPlanPayload{
		Action:     "discover_assets",
		Runner:     "command",
		Parser:     "web_api_parser",
		RunCommand: []string{"sh", "-c", "touch " + marker},
	})
	claimsBefore := countJSON(t, filepath.Join(root, "runtime", "worker-queue-claims"))
	requestsBefore, err := ListRuntimeWorkerTransportDispatchRequests(root, "")
	if err != nil {
		t.Fatalf("list transport requests before supervisor: %v", err)
	}

	supervisor, err := CreateWorkerLifecycleMaintenanceSupervisor(root, WorkerLifecycleMaintenanceSupervisorOptions{
		Execute:                true,
		ExecuteRetryDispatches: true,
		Now:                    now.Add(5 * time.Minute),
		MaxRuns:                2,
	})
	if err != nil {
		t.Fatalf("execute worker lifecycle maintenance supervisor with explicit retry-dispatch execution: %v", err)
	}

	if supervisor.Summary.Runs != 2 || supervisor.Summary.MaintenanceArtifactsCreated != 2 || supervisor.Summary.AuditArtifactsCreated != 2 {
		t.Fatalf("expected bounded supervisor child artifacts, got %#v", supervisor.Summary)
	}
	if supervisor.Summary.RetryDispatchExecuteActions != 1 || supervisor.Summary.SkippedUnsafeActions != 0 {
		t.Fatalf("expected first pass to execute retry-dispatch action without skipped-unsafe rows, got %#v", supervisor.Summary)
	}
	if supervisor.Summary.RetryDispatchExecutionsCreated != 1 || supervisor.Summary.RetryDispatchClaimsCreated != 1 {
		t.Fatalf("expected retry-dispatch execution and claim counts, got %#v", supervisor.Summary)
	}
	if len(supervisor.RetryDispatchExecutionPaths) != 1 {
		t.Fatalf("expected linked retry-dispatch execution path, got %#v", supervisor)
	}
	executions, err := ListRuntimeWorkerDispatchRetryDispatchExecutions(root)
	if err != nil {
		t.Fatalf("list retry-dispatch executions after supervisor: %v", err)
	}
	if len(executions) != 1 || executions[0].RetryDispatchPath != retryDispatch.Path || executions[0].Summary.Claimed != 1 || executions[0].Summary.Dispatched != 0 {
		t.Fatalf("expected one metadata-only retry-dispatch execution that created one claim and did not dispatch, got %#v", executions)
	}
	if supervisor.RetryDispatchExecutionPaths[0] != executions[0].Path {
		t.Fatalf("expected supervisor to link execution %s, got %#v", executions[0].Path, supervisor.RetryDispatchExecutionPaths)
	}
	if got := countJSON(t, filepath.Join(root, "runtime", "worker-queue-claims")); got != claimsBefore+1 {
		t.Fatalf("expected exactly one retry queue claim: before=%d after=%d", claimsBefore, got)
	}
	requestsAfter, err := ListRuntimeWorkerTransportDispatchRequests(root, "")
	if err != nil {
		t.Fatalf("list transport requests after supervisor: %v", err)
	}
	if len(requestsAfter) != len(requestsBefore) {
		t.Fatalf("explicit retry-dispatch supervisor must not dispatch retry claims or create file-outbox requests: before=%d after=%d", len(requestsBefore), len(requestsAfter))
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("explicit retry-dispatch supervisor must not execute retry tool-plan command, marker stat err=%v", err)
	}
}

func TestCreateWorkerLifecycleMaintenanceSupervisorDispatchesRetryClaimsWhenPropagated(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Date(2026, 6, 2, 11, 30, 0, 0, time.UTC)
	marker := filepath.Join(t.TempDir(), "maintenance-supervisor-retry-handoff-ran")
	_, retryDispatch, _ := createRuntimeWorkerDispatchRetryDispatchFixture(t, root, now, RuntimeWorkerToolPlanPayload{
		Action:     "discover_assets",
		Runner:     "command",
		Parser:     "web_api_parser",
		RunCommand: []string{"sh", "-c", "touch " + marker},
	})
	requestsBefore, err := ListRuntimeWorkerTransportDispatchRequests(root, "")
	if err != nil {
		t.Fatalf("list transport requests before supervisor: %v", err)
	}
	resultsBefore := countJSON(t, filepath.Join(root, "runtime", "worker-queue-dispatch-results"))

	supervisor, err := CreateWorkerLifecycleMaintenanceSupervisor(root, WorkerLifecycleMaintenanceSupervisorOptions{
		Execute:                    true,
		ExecuteRetryDispatches:     true,
		DispatchRetryClaims:        true,
		RetryDispatchTransportMode: "file-outbox",
		Now:                        now.Add(5 * time.Minute),
		MaxRuns:                    2,
	})
	if err != nil {
		t.Fatalf("execute worker lifecycle maintenance supervisor with explicit retry handoff: %v", err)
	}

	if !supervisor.DispatchRetryClaims || supervisor.RetryDispatchTransportMode != "file-outbox" {
		t.Fatalf("expected supervisor to persist retry handoff intent, got %#v", supervisor)
	}
	if supervisor.Summary.RetryDispatchExecutionsCreated != 1 ||
		supervisor.Summary.RetryDispatchClaimsCreated != 1 ||
		supervisor.Summary.RetryDispatchClaimsDispatched != 1 ||
		supervisor.Summary.RetryDispatchTransportRequestsCreated != 1 {
		t.Fatalf("expected supervisor retry handoff summary counts, got %#v", supervisor.Summary)
	}
	executions, err := ListRuntimeWorkerDispatchRetryDispatchExecutions(root)
	if err != nil {
		t.Fatalf("list retry-dispatch executions after supervisor: %v", err)
	}
	if len(executions) != 1 || executions[0].RetryDispatchPath != retryDispatch.Path || executions[0].Summary.Claimed != 1 || executions[0].Summary.Dispatched != 1 {
		t.Fatalf("expected one dispatched retry-dispatch execution, got %#v", executions)
	}
	requestsAfter, err := ListRuntimeWorkerTransportDispatchRequests(root, "")
	if err != nil {
		t.Fatalf("list transport requests after supervisor: %v", err)
	}
	if len(requestsAfter) != len(requestsBefore)+1 {
		t.Fatalf("expected one pending retry file-outbox request: before=%d after=%d", len(requestsBefore), len(requestsAfter))
	}
	if got := countJSON(t, filepath.Join(root, "runtime", "worker-queue-dispatch-results")); got != resultsBefore {
		t.Fatalf("retry handoff supervisor must not create worker result artifacts: before=%d after=%d", resultsBefore, got)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("retry handoff supervisor must not execute retry tool-plan command, marker stat err=%v", err)
	}
}

func TestReadWorkerLifecycleMaintenanceSupervisorRejectsPathOutsideDir(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if _, err := ReadWorkerLifecycleMaintenanceSupervisor(root, filepath.Join("..", "supervisor.json")); err == nil || !strings.Contains(err.Error(), "worker lifecycle maintenance supervisor path must stay under") {
		t.Fatalf("expected supervisor path confinement error, got %v", err)
	}
}
