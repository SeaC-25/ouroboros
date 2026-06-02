package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCreateRuntimeWorkerOrchestrationWritesDryRunAuditWithoutMutation(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	now := time.Date(2026, 5, 31, 4, 20, 0, 0, time.UTC)
	if _, err := RegisterRuntimeWorker(root, RuntimeWorkerArtifact{
		WorkerID:     "remote-web-1",
		Mode:         "remote",
		Status:       "available",
		Endpoint:     "https://worker.example.test/dispatch",
		Capabilities: []string{"web.discovery"},
	}, RuntimeWorkerRegistrationOptions{Now: now}); err != nil {
		t.Fatalf("register worker: %v", err)
	}

	orchestration, err := CreateRuntimeWorkerOrchestration(root, RuntimeWorkerOrchestrationOptions{
		Now:   now,
		RunID: "run-worker-orchestration-dry",
	})
	if err != nil {
		t.Fatalf("create worker orchestration: %v", err)
	}
	if orchestration.Status != "dry-run" || orchestration.Mode != "metadata-only" || orchestration.TransportMode != "metadata-only" {
		t.Fatalf("unexpected dry-run orchestration summary: %#v", orchestration)
	}
	if orchestration.PlacementReady != 2 || orchestration.PlacementSkipped != 6 {
		t.Fatalf("expected placement dry-run counts, got %#v", orchestration)
	}
	if orchestration.PlacementPlanPath != "" || orchestration.PlacementDispatchPlanPath != "" || orchestration.PlacementDispatchBatchPath != "" {
		t.Fatalf("dry-run orchestration must not create child artifacts, got %#v", orchestration)
	}
	if !strings.Contains(orchestration.Path, filepath.Join("runtime", "worker-orchestrations")) {
		t.Fatalf("expected worker orchestration artifact path, got %s", orchestration.Path)
	}
	if countJSON(t, filepath.Join(root, "runtime", "worker-placement-plans")) != 0 {
		t.Fatalf("dry-run orchestration must not create placement plan artifacts")
	}
	if countJSON(t, filepath.Join(root, "runtime", "worker-queue-claims")) != 0 {
		t.Fatalf("dry-run orchestration must not create queue claims")
	}
	if countJSON(t, filepath.Join(root, "runtime", "worker-placement-dispatch-plans")) != 0 {
		t.Fatalf("dry-run orchestration must not create placement dispatch plans")
	}
	if countJSON(t, filepath.Join(root, "runtime", "worker-placement-dispatch-batches")) != 0 {
		t.Fatalf("dry-run orchestration must not create placement dispatch batches")
	}
	if countJSON(t, filepath.Join(root, "runtime", "worker-transport-outbox", "remote-web-1")) != 0 {
		t.Fatalf("dry-run orchestration must not write worker outbox requests")
	}
}

func TestCreateRuntimeWorkerOrchestrationExecuteComposesPlacementClaimAndDispatchBatch(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	now := time.Date(2026, 5, 31, 4, 30, 0, 0, time.UTC)
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
		{
			WorkerID:     "remote-ctf-1",
			Mode:         "remote",
			Status:       "available",
			Endpoint:     "https://ctf-worker.example.test/dispatch",
			Capabilities: []string{"ctf.challenge_classification"},
		},
	} {
		if _, err := RegisterRuntimeWorker(root, worker, RuntimeWorkerRegistrationOptions{Now: now}); err != nil {
			t.Fatalf("register worker %s: %v", worker.WorkerID, err)
		}
	}

	orchestration, err := CreateRuntimeWorkerOrchestration(root, RuntimeWorkerOrchestrationOptions{
		Execute:       true,
		Now:           now.Add(time.Minute),
		RunID:         "run-worker-orchestration-exec",
		TransportMode: "metadata-only",
		MaxCandidates: 2,
		LeaseDuration: 10 * time.Minute,
	})
	if err != nil {
		t.Fatalf("execute worker orchestration: %v", err)
	}
	if orchestration.Status != "completed" || orchestration.Mode != "execute" || orchestration.TransportMode != "metadata-only" {
		t.Fatalf("unexpected executed orchestration summary: %#v", orchestration)
	}
	if orchestration.PlacementReady != 8 || orchestration.PlacementSkipped != 0 || orchestration.ClaimedCount != 2 {
		t.Fatalf("expected bounded placement claim counts, got %#v", orchestration)
	}
	if orchestration.PlacementPlanPath == "" || orchestration.PlacementDispatchPlanPath == "" || orchestration.PlacementDispatchBatchPath == "" {
		t.Fatalf("execute orchestration should link child artifacts, got %#v", orchestration)
	}
	if orchestration.DispatchReady != 2 || orchestration.DispatchedCount != 2 || orchestration.DispatchSkipped != 0 || orchestration.DispatchFailed != 0 {
		t.Fatalf("expected two dispatched candidates, got %#v", orchestration)
	}
	if countJSON(t, filepath.Join(root, "runtime", "worker-placement-plans")) != 1 {
		t.Fatalf("execute orchestration should create one placement plan")
	}
	if countJSON(t, filepath.Join(root, "runtime", "worker-placement-claim-executions")) != 2 {
		t.Fatalf("execute orchestration should create two placement claim executions")
	}
	if countJSON(t, filepath.Join(root, "runtime", "worker-placement-dispatch-plans")) != 1 {
		t.Fatalf("execute orchestration should create one placement dispatch plan")
	}
	if countJSON(t, filepath.Join(root, "runtime", "worker-placement-dispatch-batches")) != 1 {
		t.Fatalf("execute orchestration should create one placement dispatch batch")
	}
	if countJSON(t, filepath.Join(root, "runtime", "worker-queue-dispatch-executions")) != 2 {
		t.Fatalf("execute orchestration should dispatch two queue claims")
	}
	if countJSON(t, filepath.Join(root, "runtime", "worker-transport-outbox")) != 0 {
		t.Fatalf("metadata-only dispatch orchestration must not create worker outbox requests")
	}
}

func TestCreateRuntimeWorkerOrchestrationExecuteCanAttachLifecycleMaintenanceAudit(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	now := time.Date(2026, 5, 31, 10, 5, 0, 0, time.UTC)
	if _, err := RegisterRuntimeWorker(root, RuntimeWorkerArtifact{
		WorkerID:     "remote-web-1",
		Mode:         "remote",
		Status:       "available",
		Endpoint:     "https://web-worker.example.test/dispatch",
		Capabilities: []string{"web.discovery"},
	}, RuntimeWorkerRegistrationOptions{Now: now}); err != nil {
		t.Fatalf("register worker: %v", err)
	}

	orchestration, err := CreateRuntimeWorkerOrchestration(root, RuntimeWorkerOrchestrationOptions{
		Execute:                     true,
		Now:                         now.Add(time.Minute),
		RunID:                       "run-worker-orchestration-lifecycle",
		TransportMode:               "metadata-only",
		MaxCandidates:               1,
		LeaseDuration:               10 * time.Minute,
		IncludeLifecycleMaintenance: true,
	})
	if err != nil {
		t.Fatalf("execute worker orchestration with lifecycle maintenance: %v", err)
	}
	if orchestration.Status != "completed" || orchestration.LifecycleMaintenancePath == "" || orchestration.LifecycleAuditPath == "" {
		t.Fatalf("expected lifecycle maintenance/audit links on completed orchestration, got %#v", orchestration)
	}
	if countJSON(t, filepath.Join(root, "runtime", "worker-lifecycle-maintenance")) != 1 {
		t.Fatalf("expected one lifecycle maintenance artifact")
	}
	if countJSON(t, filepath.Join(root, "runtime", "worker-lifecycle-audits")) != 1 {
		t.Fatalf("expected one lifecycle audit artifact")
	}
	if countJSON(t, filepath.Join(root, "runtime", "worker-lease-reclaim-executions")) != 0 {
		t.Fatalf("lifecycle maintenance attachment must not execute lease reclaim")
	}
	if countJSON(t, filepath.Join(root, "runtime", "worker-lease-result-reconciliation-executions")) != 0 {
		t.Fatalf("lifecycle maintenance attachment must not execute reconciliation release")
	}
	read, err := ReadRuntimeWorkerOrchestration(root, filepath.Base(orchestration.Path))
	if err != nil {
		t.Fatalf("read worker orchestration: %v", err)
	}
	if read.LifecycleMaintenancePath != orchestration.LifecycleMaintenancePath || read.LifecycleAuditPath != orchestration.LifecycleAuditPath {
		t.Fatalf("expected persisted lifecycle links, got %#v", read)
	}
	audit, err := ReadWorkerLifecycleAudit(root, filepath.Base(orchestration.LifecycleAuditPath))
	if err != nil {
		t.Fatalf("read lifecycle audit: %v", err)
	}
	if audit.Summary.WorkerOrchestrations != 1 || audit.Summary.LifecycleMaintenances != 1 {
		t.Fatalf("expected lifecycle audit to include orchestration and maintenance, got %#v", audit.Summary)
	}
}

func TestCreateRuntimeWorkerOrchestrationLifecycleMaintenanceSkipsRetryDispatchByDefault(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Date(2026, 6, 1, 15, 30, 0, 0, time.UTC)
	marker := filepath.Join(t.TempDir(), "orchestration-default-retry-ran")
	createRuntimeWorkerDispatchRetryDispatchFixture(t, root, now, RuntimeWorkerToolPlanPayload{
		Action:     "discover_assets",
		Runner:     "command",
		Parser:     "web_api_parser",
		RunCommand: []string{"sh", "-c", "touch " + marker},
	})
	claimsBefore := countJSON(t, filepath.Join(root, "runtime", "worker-queue-claims"))

	orchestration, err := CreateRuntimeWorkerOrchestration(root, RuntimeWorkerOrchestrationOptions{
		Execute:                     true,
		Now:                         now.Add(5 * time.Minute),
		RunID:                       "run-worker-orchestration-lifecycle-retry-default",
		TransportMode:               "metadata-only",
		MaxCandidates:               1,
		LeaseDuration:               10 * time.Minute,
		IncludeLifecycleMaintenance: true,
	})
	if err != nil {
		t.Fatalf("execute worker orchestration with default lifecycle retry-dispatch handling: %v", err)
	}
	if orchestration.RetryDispatchExecutionsCreated != 0 || orchestration.RetryDispatchClaimsCreated != 0 || len(orchestration.RetryDispatchExecutionPaths) != 0 {
		t.Fatalf("default orchestration lifecycle maintenance must not execute retry dispatches, got %#v", orchestration)
	}
	if got := countJSON(t, filepath.Join(root, "runtime", "worker-queue-claims")); got != claimsBefore+1 {
		t.Fatalf("expected only the orchestration placement claim, before=%d after=%d", claimsBefore, got)
	}
	if executions, err := ListRuntimeWorkerDispatchRetryDispatchExecutions(root); err != nil {
		t.Fatalf("list retry-dispatch executions after orchestration: %v", err)
	} else if len(executions) != 0 {
		t.Fatalf("default orchestration must not execute retry dispatch artifacts, got %#v", executions)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("default orchestration must not execute retry tool-plan command, marker stat err=%v", err)
	}
}

func TestCreateRuntimeWorkerOrchestrationLifecycleMaintenanceExecutesRetryDispatchesWithExplicitOption(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Date(2026, 6, 1, 15, 35, 0, 0, time.UTC)
	marker := filepath.Join(t.TempDir(), "orchestration-explicit-retry-ran")
	_, retryDispatch, _ := createRuntimeWorkerDispatchRetryDispatchFixture(t, root, now, RuntimeWorkerToolPlanPayload{
		Action:     "discover_assets",
		Runner:     "command",
		Parser:     "web_api_parser",
		RunCommand: []string{"sh", "-c", "touch " + marker},
	})
	claimsBefore := countJSON(t, filepath.Join(root, "runtime", "worker-queue-claims"))
	requestsBefore, err := ListRuntimeWorkerTransportDispatchRequests(root, "")
	if err != nil {
		t.Fatalf("list transport requests before orchestration: %v", err)
	}

	orchestration, err := CreateRuntimeWorkerOrchestration(root, RuntimeWorkerOrchestrationOptions{
		Execute:                         true,
		Now:                             now.Add(5 * time.Minute),
		RunID:                           "run-worker-orchestration-lifecycle-retry-explicit",
		TransportMode:                   "metadata-only",
		MaxCandidates:                   1,
		LeaseDuration:                   10 * time.Minute,
		IncludeLifecycleMaintenance:     true,
		ExecuteLifecycleRetryDispatches: true,
	})
	if err != nil {
		t.Fatalf("execute worker orchestration with explicit lifecycle retry-dispatch handling: %v", err)
	}
	if orchestration.RetryDispatchExecutionsCreated != 1 || orchestration.RetryDispatchClaimsCreated != 1 {
		t.Fatalf("expected orchestration to aggregate lifecycle retry execution counts, got %#v", orchestration)
	}
	if len(orchestration.RetryDispatchExecutionPaths) != 1 {
		t.Fatalf("expected orchestration to link one retry-dispatch execution path, got %#v", orchestration)
	}
	executions, err := ListRuntimeWorkerDispatchRetryDispatchExecutions(root)
	if err != nil {
		t.Fatalf("list retry-dispatch executions after orchestration: %v", err)
	}
	if len(executions) != 1 || executions[0].RetryDispatchPath != retryDispatch.Path || executions[0].Summary.Claimed != 1 || executions[0].Summary.Dispatched != 0 {
		t.Fatalf("expected one metadata-only retry-dispatch execution without claim dispatch, got %#v", executions)
	}
	if orchestration.RetryDispatchExecutionPaths[0] != executions[0].Path {
		t.Fatalf("expected orchestration to link execution %s, got %#v", executions[0].Path, orchestration.RetryDispatchExecutionPaths)
	}
	if got := countJSON(t, filepath.Join(root, "runtime", "worker-queue-claims")); got != claimsBefore+2 {
		t.Fatalf("expected orchestration placement claim plus retry queue claim: before=%d after=%d", claimsBefore, got)
	}
	requestsAfter, err := ListRuntimeWorkerTransportDispatchRequests(root, "")
	if err != nil {
		t.Fatalf("list transport requests after orchestration: %v", err)
	}
	if len(requestsAfter) != len(requestsBefore) {
		t.Fatalf("explicit lifecycle retry dispatch must not create file-outbox requests: before=%d after=%d", len(requestsBefore), len(requestsAfter))
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("explicit lifecycle retry dispatch must not execute retry tool-plan command, marker stat err=%v", err)
	}
}

func TestCreateRuntimeWorkerOrchestrationDispatchesLifecycleRetryClaimsWithExplicitHandoff(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Date(2026, 6, 2, 12, 20, 0, 0, time.UTC)
	marker := filepath.Join(t.TempDir(), "orchestration-retry-handoff-ran")
	_, retryDispatch, _ := createRuntimeWorkerDispatchRetryDispatchFixture(t, root, now, RuntimeWorkerToolPlanPayload{
		Action:     "discover_assets",
		Runner:     "command",
		Parser:     "web_api_parser",
		RunCommand: []string{"sh", "-c", "touch " + marker},
	})
	claimsBefore := countJSON(t, filepath.Join(root, "runtime", "worker-queue-claims"))
	requestsBefore, err := ListRuntimeWorkerTransportDispatchRequests(root, "")
	if err != nil {
		t.Fatalf("list transport requests before orchestration: %v", err)
	}
	resultsBefore := countJSON(t, filepath.Join(root, "runtime", "worker-queue-dispatch-results"))

	orchestration, err := CreateRuntimeWorkerOrchestration(root, RuntimeWorkerOrchestrationOptions{
		Execute:                         true,
		Now:                             now.Add(5 * time.Minute),
		RunID:                           "run-worker-orchestration-lifecycle-retry-handoff",
		TransportMode:                   "metadata-only",
		MaxCandidates:                   1,
		LeaseDuration:                   10 * time.Minute,
		IncludeLifecycleMaintenance:     true,
		ExecuteLifecycleRetryDispatches: true,
		DispatchLifecycleRetryClaims:    true,
		LifecycleRetryTransportMode:     "file-outbox",
	})
	if err != nil {
		t.Fatalf("execute worker orchestration with explicit lifecycle retry handoff: %v", err)
	}

	if !orchestration.DispatchLifecycleRetryClaims || orchestration.LifecycleRetryTransportMode != "file-outbox" {
		t.Fatalf("expected orchestration to persist lifecycle retry handoff intent, got %#v", orchestration)
	}
	if orchestration.RetryDispatchExecutionsCreated != 1 ||
		orchestration.RetryDispatchClaimsCreated != 1 ||
		orchestration.RetryDispatchClaimsDispatched != 1 ||
		orchestration.RetryDispatchTransportRequestsCreated != 1 {
		t.Fatalf("expected orchestration lifecycle retry handoff counts, got %#v", orchestration)
	}
	executions, err := ListRuntimeWorkerDispatchRetryDispatchExecutions(root)
	if err != nil {
		t.Fatalf("list retry-dispatch executions after orchestration: %v", err)
	}
	if len(executions) != 1 || executions[0].RetryDispatchPath != retryDispatch.Path || executions[0].Summary.Claimed != 1 || executions[0].Summary.Dispatched != 1 {
		t.Fatalf("expected one dispatched retry-dispatch execution, got %#v", executions)
	}
	if got := countJSON(t, filepath.Join(root, "runtime", "worker-queue-claims")); got != claimsBefore+2 {
		t.Fatalf("expected placement claim plus retry queue claim: before=%d after=%d", claimsBefore, got)
	}
	requestsAfter, err := ListRuntimeWorkerTransportDispatchRequests(root, "")
	if err != nil {
		t.Fatalf("list transport requests after orchestration: %v", err)
	}
	if len(requestsAfter) != len(requestsBefore)+1 {
		t.Fatalf("expected one pending retry file-outbox request: before=%d after=%d", len(requestsBefore), len(requestsAfter))
	}
	if got := countJSON(t, filepath.Join(root, "runtime", "worker-queue-dispatch-results")); got != resultsBefore {
		t.Fatalf("orchestration retry handoff must not create worker result artifacts: before=%d after=%d", resultsBefore, got)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("orchestration retry handoff must not execute retry tool-plan command, marker stat err=%v", err)
	}
}

func TestReadRuntimeWorkerOrchestrationRejectsPathOutsideDir(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}

	if _, err := ReadRuntimeWorkerOrchestration(root, filepath.Join("..", "orchestration.json")); err == nil || !strings.Contains(err.Error(), "worker orchestration path must stay under") {
		t.Fatalf("expected path confinement error, got %v", err)
	}
}

func countJSON(t *testing.T, dir string) int {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		t.Fatalf("glob json under %s: %v", dir, err)
	}
	return len(paths)
}
