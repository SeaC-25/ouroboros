package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCreateRuntimeWorkerOrchestrationSupervisorDryRunWritesAuditWithoutChildArtifacts(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	now := time.Date(2026, 5, 31, 11, 30, 0, 0, time.UTC)
	if _, err := RegisterRuntimeWorker(root, RuntimeWorkerArtifact{
		WorkerID:     "remote-web-1",
		Mode:         "remote",
		Status:       "available",
		Endpoint:     "https://worker.example.test/dispatch",
		Capabilities: []string{"web.discovery"},
	}, RuntimeWorkerRegistrationOptions{Now: now}); err != nil {
		t.Fatalf("register worker: %v", err)
	}

	supervisor, err := CreateRuntimeWorkerOrchestrationSupervisor(root, RuntimeWorkerOrchestrationSupervisorOptions{
		Now:           now.Add(time.Minute),
		RunIDPrefix:   "run-worker-orchestration-supervisor-dry",
		TransportMode: "metadata-only",
		MaxCandidates: 1,
		MaxRuns:       2,
	})
	if err != nil {
		t.Fatalf("create dry-run worker orchestration supervisor: %v", err)
	}
	if supervisor.Status != "dry-run" || supervisor.Mode != "metadata-only" || supervisor.TransportMode != "metadata-only" {
		t.Fatalf("unexpected dry-run supervisor metadata: %#v", supervisor)
	}
	if supervisor.Summary.Runs != 0 || supervisor.Summary.MaxRuns != 2 || supervisor.Summary.OrchestrationArtifactsCreated != 0 {
		t.Fatalf("unexpected dry-run supervisor summary: %#v", supervisor.Summary)
	}
	if len(supervisor.OrchestrationPaths) != 0 {
		t.Fatalf("dry-run supervisor must not link child orchestrations, got %#v", supervisor)
	}
	if !strings.Contains(supervisor.Path, filepath.Join("runtime", "worker-orchestration-supervisor-runs")) {
		t.Fatalf("expected supervisor artifact path, got %s", supervisor.Path)
	}
	if countJSON(t, filepath.Join(root, "runtime", "worker-orchestrations")) != 0 {
		t.Fatalf("dry-run supervisor must not create worker orchestration artifacts")
	}
	if countJSON(t, filepath.Join(root, "runtime", "worker-placement-plans")) != 0 {
		t.Fatalf("dry-run supervisor must not create placement plans")
	}
	if countJSON(t, filepath.Join(root, "runtime", "worker-queue-dispatch-executions")) != 0 {
		t.Fatalf("dry-run supervisor must not dispatch queue claims")
	}
}

func TestCreateRuntimeWorkerOrchestrationSupervisorExecuteRunsBoundedOrchestrations(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	now := time.Date(2026, 5, 31, 11, 40, 0, 0, time.UTC)
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

	supervisor, err := CreateRuntimeWorkerOrchestrationSupervisor(root, RuntimeWorkerOrchestrationSupervisorOptions{
		Execute:                     true,
		Now:                         now.Add(time.Minute),
		RunIDPrefix:                 "run-worker-orchestration-supervisor-exec",
		TransportMode:               "metadata-only",
		MaxCandidates:               1,
		MaxRuns:                     2,
		LeaseDuration:               10 * time.Minute,
		IncludeLifecycleMaintenance: true,
	})
	if err != nil {
		t.Fatalf("execute worker orchestration supervisor: %v", err)
	}
	if supervisor.Status != "completed" || supervisor.Mode != "metadata-only" || supervisor.TransportMode != "metadata-only" {
		t.Fatalf("unexpected executed supervisor metadata: %#v", supervisor)
	}
	if supervisor.Summary.Runs != 2 || supervisor.Summary.MaxRuns != 2 || supervisor.Summary.OrchestrationArtifactsCreated != 2 {
		t.Fatalf("unexpected executed supervisor summary: %#v", supervisor.Summary)
	}
	if supervisor.Summary.ClaimedCount != 2 || supervisor.Summary.DispatchedCount != 2 || supervisor.Summary.LifecycleMaintenanceArtifactsCreated != 2 || supervisor.Summary.LifecycleAuditArtifactsCreated != 2 {
		t.Fatalf("expected bounded orchestration/lifecycle counts, got %#v", supervisor.Summary)
	}
	if len(supervisor.OrchestrationPaths) != 2 {
		t.Fatalf("expected two linked orchestration artifacts, got %#v", supervisor.OrchestrationPaths)
	}
	if countJSON(t, filepath.Join(root, "runtime", "worker-orchestrations")) != 2 {
		t.Fatalf("execute supervisor should create two worker orchestration artifacts")
	}
	if countJSON(t, filepath.Join(root, "runtime", "worker-placement-plans")) != 2 {
		t.Fatalf("execute supervisor should create two placement plans")
	}
	if countJSON(t, filepath.Join(root, "runtime", "worker-queue-dispatch-executions")) != 2 {
		t.Fatalf("execute supervisor should dispatch two metadata-only queue claims")
	}
	if countJSON(t, filepath.Join(root, "runtime", "worker-transport-outbox")) != 0 {
		t.Fatalf("metadata-only orchestration supervisor must not create worker outbox requests")
	}
	if countJSON(t, filepath.Join(root, "runtime", "worker-lease-reclaim-executions")) != 0 {
		t.Fatalf("orchestration supervisor must not execute lease reclaim")
	}
	if countJSON(t, filepath.Join(root, "runtime", "worker-lease-result-reconciliation-executions")) != 0 {
		t.Fatalf("orchestration supervisor must not execute reconciliation release")
	}
	read, err := ReadRuntimeWorkerOrchestrationSupervisor(root, filepath.Base(supervisor.Path))
	if err != nil {
		t.Fatalf("read worker orchestration supervisor: %v", err)
	}
	if read.Path != supervisor.Path || read.Summary.Runs != 2 {
		t.Fatalf("unexpected persisted supervisor: %#v", read)
	}
	runs, err := ListRuntimeWorkerOrchestrationSupervisors(root)
	if err != nil {
		t.Fatalf("list worker orchestration supervisors: %v", err)
	}
	if len(runs) != 1 || runs[0].Path != supervisor.Path {
		t.Fatalf("expected one persisted supervisor run, got %#v", runs)
	}
}

func TestCreateRuntimeWorkerOrchestrationSupervisorLifecycleMaintenanceSkipsRetryDispatchByDefault(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Date(2026, 6, 1, 16, 0, 0, 0, time.UTC)
	marker := filepath.Join(t.TempDir(), "orchestration-supervisor-default-retry-ran")
	createRuntimeWorkerDispatchRetryDispatchFixture(t, root, now, RuntimeWorkerToolPlanPayload{
		Action:     "discover_assets",
		Runner:     "command",
		Parser:     "web_api_parser",
		RunCommand: []string{"sh", "-c", "touch " + marker},
	})
	claimsBefore := countJSON(t, filepath.Join(root, "runtime", "worker-queue-claims"))

	supervisor, err := CreateRuntimeWorkerOrchestrationSupervisor(root, RuntimeWorkerOrchestrationSupervisorOptions{
		Execute:                     true,
		Now:                         now.Add(5 * time.Minute),
		RunIDPrefix:                 "run-worker-orchestration-supervisor-lifecycle-retry-default",
		TransportMode:               "metadata-only",
		MaxCandidates:               1,
		MaxRuns:                     1,
		LeaseDuration:               10 * time.Minute,
		IncludeLifecycleMaintenance: true,
	})
	if err != nil {
		t.Fatalf("execute worker orchestration supervisor with default lifecycle retry-dispatch handling: %v", err)
	}
	if supervisor.Summary.RetryDispatchExecutionsCreated != 0 || supervisor.Summary.RetryDispatchClaimsCreated != 0 || len(supervisor.RetryDispatchExecutionPaths) != 0 {
		t.Fatalf("default orchestration supervisor must not execute lifecycle retry dispatches, got %#v", supervisor)
	}
	if got := countJSON(t, filepath.Join(root, "runtime", "worker-queue-claims")); got != claimsBefore+1 {
		t.Fatalf("expected only one child orchestration placement claim, before=%d after=%d", claimsBefore, got)
	}
	if executions, err := ListRuntimeWorkerDispatchRetryDispatchExecutions(root); err != nil {
		t.Fatalf("list retry-dispatch executions after supervisor: %v", err)
	} else if len(executions) != 0 {
		t.Fatalf("default supervisor must not execute retry dispatch artifacts, got %#v", executions)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("default supervisor must not execute retry tool-plan command, marker stat err=%v", err)
	}
}

func TestCreateRuntimeWorkerOrchestrationSupervisorExecutesLifecycleRetryDispatchesWithExplicitOption(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Date(2026, 6, 1, 16, 5, 0, 0, time.UTC)
	marker := filepath.Join(t.TempDir(), "orchestration-supervisor-explicit-retry-ran")
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

	supervisor, err := CreateRuntimeWorkerOrchestrationSupervisor(root, RuntimeWorkerOrchestrationSupervisorOptions{
		Execute:                         true,
		Now:                             now.Add(5 * time.Minute),
		RunIDPrefix:                     "run-worker-orchestration-supervisor-lifecycle-retry-explicit",
		TransportMode:                   "metadata-only",
		MaxCandidates:                   1,
		MaxRuns:                         1,
		LeaseDuration:                   10 * time.Minute,
		IncludeLifecycleMaintenance:     true,
		ExecuteLifecycleRetryDispatches: true,
	})
	if err != nil {
		t.Fatalf("execute worker orchestration supervisor with explicit lifecycle retry-dispatch handling: %v", err)
	}
	if supervisor.Summary.RetryDispatchExecutionsCreated != 1 || supervisor.Summary.RetryDispatchClaimsCreated != 1 {
		t.Fatalf("expected supervisor retry-dispatch aggregate counts, got %#v", supervisor.Summary)
	}
	if len(supervisor.RetryDispatchExecutionPaths) != 1 {
		t.Fatalf("expected supervisor to link one retry-dispatch execution path, got %#v", supervisor.RetryDispatchExecutionPaths)
	}
	executions, err := ListRuntimeWorkerDispatchRetryDispatchExecutions(root)
	if err != nil {
		t.Fatalf("list retry-dispatch executions after supervisor: %v", err)
	}
	if len(executions) != 1 || executions[0].RetryDispatchPath != retryDispatch.Path || executions[0].Summary.Claimed != 1 || executions[0].Summary.Dispatched != 0 {
		t.Fatalf("expected one metadata-only retry-dispatch execution without claim dispatch, got %#v", executions)
	}
	if supervisor.RetryDispatchExecutionPaths[0] != executions[0].Path {
		t.Fatalf("expected supervisor to link execution %s, got %#v", executions[0].Path, supervisor.RetryDispatchExecutionPaths)
	}
	if got := countJSON(t, filepath.Join(root, "runtime", "worker-queue-claims")); got != claimsBefore+2 {
		t.Fatalf("expected child orchestration placement claim plus retry queue claim: before=%d after=%d", claimsBefore, got)
	}
	requestsAfter, err := ListRuntimeWorkerTransportDispatchRequests(root, "")
	if err != nil {
		t.Fatalf("list transport requests after supervisor: %v", err)
	}
	if len(requestsAfter) != len(requestsBefore) {
		t.Fatalf("explicit supervisor lifecycle retry dispatch must not create file-outbox requests: before=%d after=%d", len(requestsBefore), len(requestsAfter))
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("explicit supervisor lifecycle retry dispatch must not execute retry tool-plan command, marker stat err=%v", err)
	}
}

func TestCreateRuntimeWorkerOrchestrationSupervisorDispatchesLifecycleRetryClaimsWithExplicitHandoff(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Date(2026, 6, 2, 12, 30, 0, 0, time.UTC)
	marker := filepath.Join(t.TempDir(), "orchestration-supervisor-retry-handoff-ran")
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

	supervisor, err := CreateRuntimeWorkerOrchestrationSupervisor(root, RuntimeWorkerOrchestrationSupervisorOptions{
		Execute:                         true,
		Now:                             now.Add(5 * time.Minute),
		RunIDPrefix:                     "run-worker-orchestration-supervisor-lifecycle-retry-handoff",
		TransportMode:                   "metadata-only",
		MaxCandidates:                   1,
		MaxRuns:                         1,
		LeaseDuration:                   10 * time.Minute,
		IncludeLifecycleMaintenance:     true,
		ExecuteLifecycleRetryDispatches: true,
		DispatchLifecycleRetryClaims:    true,
		LifecycleRetryTransportMode:     "file-outbox",
	})
	if err != nil {
		t.Fatalf("execute worker orchestration supervisor with explicit lifecycle retry handoff: %v", err)
	}

	if !supervisor.DispatchLifecycleRetryClaims || supervisor.LifecycleRetryTransportMode != "file-outbox" {
		t.Fatalf("expected supervisor to persist lifecycle retry handoff intent, got %#v", supervisor)
	}
	if supervisor.Summary.RetryDispatchExecutionsCreated != 1 ||
		supervisor.Summary.RetryDispatchClaimsCreated != 1 ||
		supervisor.Summary.RetryDispatchClaimsDispatched != 1 ||
		supervisor.Summary.RetryDispatchTransportRequestsCreated != 1 {
		t.Fatalf("expected supervisor lifecycle retry handoff counts, got %#v", supervisor.Summary)
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
		t.Fatalf("supervisor retry handoff must not create worker result artifacts: before=%d after=%d", resultsBefore, got)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("supervisor retry handoff must not execute retry tool-plan command, marker stat err=%v", err)
	}
}

func TestReadRuntimeWorkerOrchestrationSupervisorRejectsPathOutsideDir(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if _, err := ReadRuntimeWorkerOrchestrationSupervisor(root, filepath.Join("..", "supervisor.json")); err == nil || !strings.Contains(err.Error(), "worker orchestration supervisor path must stay under") {
		t.Fatalf("expected supervisor path confinement error, got %v", err)
	}
}
