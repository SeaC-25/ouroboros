package app

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPlanRuntimeWorkerPlacementsBuildsMetadataOnlyCandidates(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	now := time.Date(2026, 5, 30, 13, 0, 0, 0, time.UTC)
	old := now.Add(-DefaultRuntimeWorkerStaleAfter - time.Minute)
	if _, err := RegisterRuntimeWorker(root, RuntimeWorkerArtifact{
		WorkerID:     "remote-web-1",
		Mode:         "remote",
		Status:       "available",
		Endpoint:     "https://worker.example.test/dispatch",
		Capabilities: []string{"web.discovery"},
	}, RuntimeWorkerRegistrationOptions{Now: now}); err != nil {
		t.Fatalf("register web worker: %v", err)
	}
	if _, err := RegisterRuntimeWorker(root, RuntimeWorkerArtifact{
		WorkerID:     "remote-infra-stale",
		Mode:         "remote",
		Status:       "available",
		Capabilities: []string{"infra.host_discovery"},
	}, RuntimeWorkerRegistrationOptions{Now: old}); err != nil {
		t.Fatalf("register stale infra worker: %v", err)
	}
	if _, err := RegisterRuntimeWorker(root, RuntimeWorkerArtifact{
		WorkerID:     "remote-ctf-offline",
		Mode:         "remote",
		Status:       "offline",
		Capabilities: []string{"ctf.challenge_classification"},
	}, RuntimeWorkerRegistrationOptions{Now: now}); err != nil {
		t.Fatalf("register offline ctf worker: %v", err)
	}

	plan, err := PlanRuntimeWorkerPlacements(root, RuntimeWorkerPlacementPlanOptions{
		Now:   now,
		RunID: "run-placement-1",
	})
	if err != nil {
		t.Fatalf("plan runtime worker placements: %v", err)
	}

	if plan.Status != "dry-run" || plan.Mode != "metadata-only" || plan.ReadyCount != 2 || plan.SkippedCount != 6 {
		t.Fatalf("unexpected placement plan summary: %#v", plan)
	}
	if plan.Path != "" {
		t.Fatalf("planning should not persist an artifact path, got %s", plan.Path)
	}
	if len(plan.Candidates) != 8 {
		t.Fatalf("expected one candidate per compiled tool request, got %#v", plan.Candidates)
	}
	for _, toolID := range []string{"httpx", "subfinder"} {
		ready := placementCandidateByToolID(plan.Candidates, toolID)
		if ready.Status != "ready" || ready.WorkerID != "remote-web-1" || ready.WorkerMode != "remote" || ready.WorkerEndpoint == "" {
			t.Fatalf("expected ready web placement on remote worker for %s, got %#v", toolID, ready)
		}
		if ready.ToolPlan.Action != "discover_assets" || ready.ToolPlan.ManifestName != "web-api-pack" || ready.ToolPlan.ToolID != toolID {
			t.Fatalf("expected carried tool-plan metadata for %s, got %#v", toolID, ready.ToolPlan)
		}
	}
	stale := placementCandidateByToolID(plan.Candidates, "nmap")
	if stale.Status != "skipped" || stale.Reason != "worker-stale" || stale.WorkerID != "remote-infra-stale" {
		t.Fatalf("expected stale infra skip, got %#v", stale)
	}
	offline := placementCandidateByParser(plan.Candidates, "ctf_parser")
	if offline.Status != "skipped" || offline.Reason != "worker-offline" || offline.WorkerID != "remote-ctf-offline" {
		t.Fatalf("expected offline ctf skip, got %#v", offline)
	}

	claims, err := ListRuntimeWorkerQueueClaims(root)
	if err != nil {
		t.Fatalf("list queue claims: %v", err)
	}
	if len(claims) != 0 {
		t.Fatalf("placement planning must not create queue claims, got %#v", claims)
	}
}

func TestCreateRuntimeWorkerPlacementPlanWritesDryRunArtifact(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	now := time.Date(2026, 5, 30, 13, 30, 0, 0, time.UTC)
	if _, err := RegisterRuntimeWorker(root, RuntimeWorkerArtifact{
		WorkerID:     "remote-web-1",
		Mode:         "remote",
		Status:       "available",
		Capabilities: []string{"web.discovery"},
	}, RuntimeWorkerRegistrationOptions{Now: now}); err != nil {
		t.Fatalf("register worker: %v", err)
	}

	plan, err := CreateRuntimeWorkerPlacementPlan(root, RuntimeWorkerPlacementPlanOptions{
		Now:   now,
		RunID: "run-placement-2",
	})
	if err != nil {
		t.Fatalf("create placement plan: %v", err)
	}
	if plan.Path == "" || !strings.Contains(plan.Path, filepath.Join("runtime", "worker-placement-plans")) {
		t.Fatalf("expected placement plan artifact path, got %s", plan.Path)
	}
	if plan.Status != "dry-run" || plan.Mode != "metadata-only" {
		t.Fatalf("expected metadata-only dry-run artifact, got %#v", plan)
	}

	plans, err := ListRuntimeWorkerPlacementPlans(root)
	if err != nil {
		t.Fatalf("list placement plans: %v", err)
	}
	if len(plans) != 1 || plans[0].Path != plan.Path {
		t.Fatalf("expected listed placement plan, got %#v", plans)
	}
	read, err := ReadRuntimeWorkerPlacementPlan(root, filepath.Base(plan.Path))
	if err != nil {
		t.Fatalf("read placement plan: %v", err)
	}
	if read.PlanID != plan.PlanID || read.ReadyCount != plan.ReadyCount || read.SkippedCount != plan.SkippedCount {
		t.Fatalf("expected read placement plan, got %#v", read)
	}
}

func TestReadRuntimeWorkerPlacementPlanRejectsPathOutsidePlanDir(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}

	if _, err := ReadRuntimeWorkerPlacementPlan(root, filepath.Join("..", "plan.json")); err == nil || !strings.Contains(err.Error(), "worker placement plan path must stay under") {
		t.Fatalf("expected path confinement error, got %v", err)
	}
}

func TestExecuteRuntimeWorkerPlacementClaimRequiresExplicitExecute(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	now := time.Date(2026, 5, 30, 14, 0, 0, 0, time.UTC)
	if _, err := RegisterRuntimeWorker(root, RuntimeWorkerArtifact{
		WorkerID:     "remote-web-1",
		Mode:         "remote",
		Status:       "available",
		Capabilities: []string{"web.discovery"},
	}, RuntimeWorkerRegistrationOptions{Now: now}); err != nil {
		t.Fatalf("register worker: %v", err)
	}
	plan, err := CreateRuntimeWorkerPlacementPlan(root, RuntimeWorkerPlacementPlanOptions{
		Now:   now,
		RunID: "run-placement-claim-required",
	})
	if err != nil {
		t.Fatalf("create placement plan: %v", err)
	}

	if _, err := ExecuteRuntimeWorkerPlacementClaim(root, plan.Path, RuntimeWorkerPlacementClaimExecutionOptions{
		CandidateExecutionID: "tool-discover_assets-01",
		WorkerID:             "remote-web-1",
		Now:                  now.Add(time.Minute),
	}); err == nil || !strings.Contains(err.Error(), "worker placement claim requires explicit execute") {
		t.Fatalf("expected explicit execute error, got %v", err)
	}
	claims, err := ListRuntimeWorkerQueueClaims(root)
	if err != nil {
		t.Fatalf("list queue claims: %v", err)
	}
	if len(claims) != 0 {
		t.Fatalf("missing execute flag must not create queue claims, got %#v", claims)
	}
	executions, err := ListRuntimeWorkerPlacementClaimExecutions(root)
	if err != nil {
		t.Fatalf("list placement claim executions: %v", err)
	}
	if len(executions) != 0 {
		t.Fatalf("missing execute flag must not write execution artifacts, got %#v", executions)
	}
}

func TestExecuteRuntimeWorkerPlacementClaimCreatesQueueClaimFromReadyCandidate(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	now := time.Date(2026, 5, 30, 14, 15, 0, 0, time.UTC)
	if _, err := RegisterRuntimeWorker(root, RuntimeWorkerArtifact{
		WorkerID:     "remote-web-1",
		Mode:         "remote",
		Status:       "available",
		Endpoint:     "https://worker.example.test/dispatch",
		Capabilities: []string{"web.discovery"},
	}, RuntimeWorkerRegistrationOptions{Now: now}); err != nil {
		t.Fatalf("register worker: %v", err)
	}
	plan, err := CreateRuntimeWorkerPlacementPlan(root, RuntimeWorkerPlacementPlanOptions{
		Now:   now,
		RunID: "run-placement-claim",
	})
	if err != nil {
		t.Fatalf("create placement plan: %v", err)
	}

	execution, err := ExecuteRuntimeWorkerPlacementClaim(root, filepath.Base(plan.Path), RuntimeWorkerPlacementClaimExecutionOptions{
		Execute:              true,
		CandidateExecutionID: "tool-discover_assets-01",
		WorkerID:             "remote-web-1",
		Now:                  now.Add(time.Minute),
		LeaseDuration:        10 * time.Minute,
	})
	if err != nil {
		t.Fatalf("execute placement claim: %v", err)
	}
	if execution.Status != "claimed" || execution.Mode != "metadata-only" || execution.ClaimPath == "" {
		t.Fatalf("expected claimed placement execution, got %#v", execution)
	}
	if execution.PlacementPlanPath != plan.Path || execution.WorkerID != "remote-web-1" || execution.Capability != "web.discovery" {
		t.Fatalf("expected placement execution metadata, got %#v", execution)
	}
	claim, err := ReadRuntimeWorkerQueueClaim(root, filepath.Base(execution.ClaimPath))
	if err != nil {
		t.Fatalf("read created queue claim: %v", err)
	}
	if claim.Status != "claimed" || claim.Mode != "metadata-only" || claim.WorkerID != "remote-web-1" || claim.RunID != "run-placement-claim" {
		t.Fatalf("expected metadata-only queue claim from placement, got %#v", claim)
	}
	if claim.ExecutionID != "tool-discover_assets-01" || claim.Tool != "web-api-pack" || claim.Capability != "web.discovery" {
		t.Fatalf("expected claim to carry candidate tool identity, got %#v", claim)
	}
	if claim.ToolPlan.Action != "discover_assets" || claim.ToolPlan.ManifestName != "web-api-pack" {
		t.Fatalf("expected claim to carry tool plan, got %#v", claim.ToolPlan)
	}
	executions, err := ListRuntimeWorkerPlacementClaimExecutions(root)
	if err != nil {
		t.Fatalf("list placement claim executions: %v", err)
	}
	if len(executions) != 1 || executions[0].Path != execution.Path || executions[0].ClaimPath != claim.Path {
		t.Fatalf("expected listed placement claim execution, got %#v", executions)
	}
	dispatches, err := ListRuntimeWorkerQueueDispatchExecutions(root)
	if err != nil {
		t.Fatalf("list queue dispatches: %v", err)
	}
	if len(dispatches) != 0 {
		t.Fatalf("placement claim must not dispatch queue claims, got %#v", dispatches)
	}
}

func TestExecuteRuntimeWorkerPlacementClaimSkipsCandidateWhenWorkerBecomesOffline(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	now := time.Date(2026, 5, 30, 14, 30, 0, 0, time.UTC)
	if _, err := RegisterRuntimeWorker(root, RuntimeWorkerArtifact{
		WorkerID:     "remote-web-1",
		Mode:         "remote",
		Status:       "available",
		Capabilities: []string{"web.discovery"},
	}, RuntimeWorkerRegistrationOptions{Now: now}); err != nil {
		t.Fatalf("register worker: %v", err)
	}
	plan, err := CreateRuntimeWorkerPlacementPlan(root, RuntimeWorkerPlacementPlanOptions{
		Now:   now,
		RunID: "run-placement-stale",
	})
	if err != nil {
		t.Fatalf("create placement plan: %v", err)
	}
	if _, err := UpdateRuntimeWorkerHeartbeat(root, "remote-web-1", RuntimeWorkerHeartbeatOptions{
		Now:    now.Add(time.Minute),
		Status: "offline",
	}); err != nil {
		t.Fatalf("mark worker offline: %v", err)
	}

	execution, err := ExecuteRuntimeWorkerPlacementClaim(root, plan.Path, RuntimeWorkerPlacementClaimExecutionOptions{
		Execute:              true,
		CandidateExecutionID: "tool-discover_assets-01",
		WorkerID:             "remote-web-1",
		Now:                  now.Add(2 * time.Minute),
	})
	if err != nil {
		t.Fatalf("execute placement claim should write skipped audit, got %v", err)
	}
	if execution.Status != "skipped" || execution.SkipReason != "worker-offline" || execution.ClaimPath != "" {
		t.Fatalf("expected skipped placement claim execution, got %#v", execution)
	}
	claims, err := ListRuntimeWorkerQueueClaims(root)
	if err != nil {
		t.Fatalf("list queue claims: %v", err)
	}
	if len(claims) != 0 {
		t.Fatalf("skipped placement claim must not create queue claims, got %#v", claims)
	}
}

func TestPlanRuntimeWorkerPlacementDispatchesBuildsMetadataOnlyDispatchCandidates(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	now := time.Date(2026, 5, 30, 15, 0, 0, 0, time.UTC)
	if _, err := RegisterRuntimeWorker(root, RuntimeWorkerArtifact{
		WorkerID:     "remote-web-1",
		Mode:         "remote",
		Status:       "available",
		Endpoint:     "https://worker.example.test/dispatch",
		Capabilities: []string{"web.discovery"},
	}, RuntimeWorkerRegistrationOptions{Now: now}); err != nil {
		t.Fatalf("register worker: %v", err)
	}
	plan, err := CreateRuntimeWorkerPlacementPlan(root, RuntimeWorkerPlacementPlanOptions{
		Now:   now,
		RunID: "run-placement-dispatch",
	})
	if err != nil {
		t.Fatalf("create placement plan: %v", err)
	}
	claimExecution, err := ExecuteRuntimeWorkerPlacementClaim(root, plan.Path, RuntimeWorkerPlacementClaimExecutionOptions{
		Execute:              true,
		CandidateExecutionID: "tool-discover_assets-01",
		WorkerID:             "remote-web-1",
		Now:                  now.Add(time.Minute),
		LeaseDuration:        10 * time.Minute,
	})
	if err != nil {
		t.Fatalf("execute placement claim: %v", err)
	}

	dispatchPlan, err := PlanRuntimeWorkerPlacementDispatches(root, RuntimeWorkerPlacementDispatchPlanOptions{
		Now:           now.Add(2 * time.Minute),
		TransportMode: "file-outbox",
	})
	if err != nil {
		t.Fatalf("plan placement dispatches: %v", err)
	}
	if dispatchPlan.Status != "dry-run" || dispatchPlan.Mode != "metadata-only" || dispatchPlan.TransportMode != "file-outbox" || dispatchPlan.ReadyCount != 1 || dispatchPlan.SkippedCount != 0 {
		t.Fatalf("unexpected placement dispatch plan summary: %#v", dispatchPlan)
	}
	if dispatchPlan.Path != "" {
		t.Fatalf("planning should not persist an artifact path, got %s", dispatchPlan.Path)
	}
	if len(dispatchPlan.Candidates) != 1 {
		t.Fatalf("expected one placement dispatch candidate, got %#v", dispatchPlan.Candidates)
	}
	ready := dispatchPlan.Candidates[0]
	if ready.Status != "ready" || ready.ClaimPath != claimExecution.ClaimPath || ready.PlacementClaimExecutionPath != claimExecution.Path {
		t.Fatalf("expected ready placement-derived dispatch candidate, got %#v", ready)
	}
	if ready.WorkerID != "remote-web-1" || ready.ExecutionID != "tool-discover_assets-01" || ready.Capability != "web.discovery" {
		t.Fatalf("expected dispatch candidate identity, got %#v", ready)
	}
	if ready.ToolPlan.Action != "discover_assets" || ready.ToolPlan.ManifestName != "web-api-pack" {
		t.Fatalf("expected carried tool plan, got %#v", ready.ToolPlan)
	}
	claims, err := ListRuntimeWorkerQueueClaims(root)
	if err != nil {
		t.Fatalf("list queue claims: %v", err)
	}
	if len(claims) != 1 || claims[0].Status != "claimed" {
		t.Fatalf("placement dispatch planning must not mutate claims, got %#v", claims)
	}
	dispatches, err := ListRuntimeWorkerQueueDispatchExecutions(root)
	if err != nil {
		t.Fatalf("list queue dispatches: %v", err)
	}
	if len(dispatches) != 0 {
		t.Fatalf("placement dispatch planning must not dispatch claims, got %#v", dispatches)
	}
}

func TestCreateRuntimeWorkerPlacementDispatchPlanWritesDryRunArtifact(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	now := time.Date(2026, 5, 30, 15, 30, 0, 0, time.UTC)
	if _, err := RegisterRuntimeWorker(root, RuntimeWorkerArtifact{
		WorkerID:     "remote-web-1",
		Mode:         "remote",
		Status:       "available",
		Capabilities: []string{"web.discovery"},
	}, RuntimeWorkerRegistrationOptions{Now: now}); err != nil {
		t.Fatalf("register worker: %v", err)
	}
	placement, err := CreateRuntimeWorkerPlacementPlan(root, RuntimeWorkerPlacementPlanOptions{Now: now, RunID: "run-placement-dispatch-artifact"})
	if err != nil {
		t.Fatalf("create placement plan: %v", err)
	}
	if _, err := ExecuteRuntimeWorkerPlacementClaim(root, placement.Path, RuntimeWorkerPlacementClaimExecutionOptions{
		Execute:              true,
		CandidateExecutionID: "tool-discover_assets-01",
		WorkerID:             "remote-web-1",
		Now:                  now.Add(time.Minute),
	}); err != nil {
		t.Fatalf("execute placement claim: %v", err)
	}

	plan, err := CreateRuntimeWorkerPlacementDispatchPlan(root, RuntimeWorkerPlacementDispatchPlanOptions{
		Now:           now.Add(2 * time.Minute),
		TransportMode: "metadata-only",
	})
	if err != nil {
		t.Fatalf("create placement dispatch plan: %v", err)
	}
	if plan.Path == "" || !strings.Contains(plan.Path, filepath.Join("runtime", "worker-placement-dispatch-plans")) {
		t.Fatalf("expected placement dispatch plan artifact path, got %s", plan.Path)
	}
	if plan.Status != "dry-run" || plan.Mode != "metadata-only" || plan.ReadyCount != 1 {
		t.Fatalf("expected metadata-only placement dispatch artifact, got %#v", plan)
	}
	plans, err := ListRuntimeWorkerPlacementDispatchPlans(root)
	if err != nil {
		t.Fatalf("list placement dispatch plans: %v", err)
	}
	if len(plans) != 1 || plans[0].Path != plan.Path {
		t.Fatalf("expected listed placement dispatch plan, got %#v", plans)
	}
	read, err := ReadRuntimeWorkerPlacementDispatchPlan(root, filepath.Base(plan.Path))
	if err != nil {
		t.Fatalf("read placement dispatch plan: %v", err)
	}
	if read.PlanID != plan.PlanID || read.ReadyCount != 1 || read.Candidates[0].Status != "ready" {
		t.Fatalf("expected read placement dispatch plan, got %#v", read)
	}
}

func TestReadRuntimeWorkerPlacementDispatchPlanRejectsPathOutsidePlanDir(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}

	if _, err := ReadRuntimeWorkerPlacementDispatchPlan(root, filepath.Join("..", "plan.json")); err == nil || !strings.Contains(err.Error(), "worker placement dispatch plan path must stay under") {
		t.Fatalf("expected path confinement error, got %v", err)
	}
}

func TestExecuteRuntimeWorkerPlacementDispatchRequiresExplicitExecute(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	plan := createPlacementDispatchPlanFixture(t, root, "metadata-only")

	if _, err := ExecuteRuntimeWorkerPlacementDispatch(root, plan.Path, RuntimeWorkerPlacementDispatchExecutionOptions{
		CandidateClaimPath: plan.Candidates[0].ClaimPath,
	}); err == nil || !strings.Contains(err.Error(), "worker placement dispatch requires explicit execute") {
		t.Fatalf("expected explicit execute error, got %v", err)
	}
	dispatches, err := ListRuntimeWorkerQueueDispatchExecutions(root)
	if err != nil {
		t.Fatalf("list queue dispatches: %v", err)
	}
	if len(dispatches) != 0 {
		t.Fatalf("missing execute flag must not dispatch, got %#v", dispatches)
	}
	executions, err := ListRuntimeWorkerPlacementDispatchExecutions(root)
	if err != nil {
		t.Fatalf("list placement dispatch executions: %v", err)
	}
	if len(executions) != 0 {
		t.Fatalf("missing execute flag must not write placement dispatch executions, got %#v", executions)
	}
}

func TestExecuteRuntimeWorkerPlacementDispatchCreatesQueueDispatchFromReadyCandidate(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	plan := createPlacementDispatchPlanFixture(t, root, "file-outbox")
	claimPath := plan.Candidates[0].ClaimPath

	execution, err := ExecuteRuntimeWorkerPlacementDispatch(root, filepath.Base(plan.Path), RuntimeWorkerPlacementDispatchExecutionOptions{
		Execute:            true,
		CandidateClaimPath: filepath.Base(claimPath),
		Now:                time.Date(2026, 5, 30, 16, 10, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("execute placement dispatch: %v", err)
	}
	if execution.Status != "dispatched" || execution.Mode != "metadata-only" || execution.TransportMode != "file-outbox" || execution.QueueDispatchExecutionPath == "" {
		t.Fatalf("expected dispatched placement execution, got %#v", execution)
	}
	if execution.PlacementDispatchPlanPath != plan.Path || execution.ClaimPath != claimPath || execution.TransportRequestPath == "" {
		t.Fatalf("expected placement dispatch metadata, got %#v", execution)
	}
	claim, err := ReadRuntimeWorkerQueueClaim(root, filepath.Base(claimPath))
	if err != nil {
		t.Fatalf("read queue claim: %v", err)
	}
	if claim.Status != "dispatched" || claim.DispatchExecutionPath == "" {
		t.Fatalf("expected queue claim to be dispatched through existing gate, got %#v", claim)
	}
	dispatches, err := ListRuntimeWorkerQueueDispatchExecutions(root)
	if err != nil {
		t.Fatalf("list queue dispatches: %v", err)
	}
	if len(dispatches) != 1 || dispatches[0].Path != execution.QueueDispatchExecutionPath || dispatches[0].TransportMode != "file-outbox" {
		t.Fatalf("expected one queue dispatch execution, got %#v", dispatches)
	}
	executions, err := ListRuntimeWorkerPlacementDispatchExecutions(root)
	if err != nil {
		t.Fatalf("list placement dispatch executions: %v", err)
	}
	if len(executions) != 1 || executions[0].Path != execution.Path || executions[0].QueueDispatchExecutionPath != dispatches[0].Path {
		t.Fatalf("expected listed placement dispatch execution, got %#v", executions)
	}
}

func TestExecuteRuntimeWorkerPlacementDispatchSkipsWhenClaimAlreadyDispatched(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	plan := createPlacementDispatchPlanFixture(t, root, "metadata-only")
	claimPath := plan.Candidates[0].ClaimPath
	if _, err := ExecuteRuntimeWorkerQueueClaimDispatch(root, claimPath, RuntimeWorkerQueueDispatchExecutionOptions{
		Execute: true,
		Now:     time.Date(2026, 5, 30, 16, 3, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("dispatch queue claim before placement dispatch execution: %v", err)
	}

	execution, err := ExecuteRuntimeWorkerPlacementDispatch(root, plan.Path, RuntimeWorkerPlacementDispatchExecutionOptions{
		Execute:            true,
		CandidateClaimPath: claimPath,
		Now:                time.Date(2026, 5, 30, 16, 4, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("execute placement dispatch should write skipped audit, got %v", err)
	}
	if execution.Status != "skipped" || execution.SkipReason != "claim-not-claimed" || execution.QueueDispatchExecutionPath != "" {
		t.Fatalf("expected skipped placement dispatch execution, got %#v", execution)
	}
	dispatches, err := ListRuntimeWorkerQueueDispatchExecutions(root)
	if err != nil {
		t.Fatalf("list queue dispatches: %v", err)
	}
	if len(dispatches) != 1 {
		t.Fatalf("skipped placement dispatch must not create another queue dispatch, got %#v", dispatches)
	}
}

func TestExecuteRuntimeWorkerPlacementDispatchBatchRequiresExplicitExecute(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	plan := createPlacementDispatchBatchPlanFixture(t, root, "metadata-only")

	if _, err := ExecuteRuntimeWorkerPlacementDispatchBatch(root, plan.Path, RuntimeWorkerPlacementDispatchBatchOptions{}); err == nil || !strings.Contains(err.Error(), "worker placement dispatch batch requires explicit execute") {
		t.Fatalf("expected explicit execute error, got %v", err)
	}
	dispatches, err := ListRuntimeWorkerQueueDispatchExecutions(root)
	if err != nil {
		t.Fatalf("list queue dispatches: %v", err)
	}
	if len(dispatches) != 0 {
		t.Fatalf("missing execute flag must not dispatch, got %#v", dispatches)
	}
	batches, err := ListRuntimeWorkerPlacementDispatchBatches(root)
	if err != nil {
		t.Fatalf("list placement dispatch batches: %v", err)
	}
	if len(batches) != 0 {
		t.Fatalf("missing execute flag must not write batch audit, got %#v", batches)
	}
}

func TestExecuteRuntimeWorkerPlacementDispatchBatchDispatchesAllReadyCandidates(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	plan := createPlacementDispatchBatchPlanFixture(t, root, "metadata-only")

	batch, err := ExecuteRuntimeWorkerPlacementDispatchBatch(root, filepath.Base(plan.Path), RuntimeWorkerPlacementDispatchBatchOptions{
		Execute: true,
		Now:     time.Date(2026, 5, 30, 17, 10, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("execute placement dispatch batch: %v", err)
	}
	if batch.Status != "completed" || batch.Mode != "metadata-only" || batch.TransportMode != "metadata-only" {
		t.Fatalf("unexpected batch summary: %#v", batch)
	}
	if batch.DispatchedCount != 8 || batch.SkippedCount != 0 || batch.FailedCount != 0 || len(batch.Executions) != 8 {
		t.Fatalf("expected eight dispatched executions, got %#v", batch)
	}
	if batch.Path == "" || !strings.Contains(batch.Path, filepath.Join("runtime", "worker-placement-dispatch-batches")) {
		t.Fatalf("expected batch audit path, got %s", batch.Path)
	}
	dispatches, err := ListRuntimeWorkerQueueDispatchExecutions(root)
	if err != nil {
		t.Fatalf("list queue dispatches: %v", err)
	}
	if len(dispatches) != 8 {
		t.Fatalf("expected eight queue dispatch executions, got %#v", dispatches)
	}
	executions, err := ListRuntimeWorkerPlacementDispatchExecutions(root)
	if err != nil {
		t.Fatalf("list placement dispatch executions: %v", err)
	}
	if len(executions) != 8 {
		t.Fatalf("expected eight placement dispatch executions, got %#v", executions)
	}
	batches, err := ListRuntimeWorkerPlacementDispatchBatches(root)
	if err != nil {
		t.Fatalf("list placement dispatch batches: %v", err)
	}
	if len(batches) != 1 || batches[0].Path != batch.Path || batches[0].DispatchedCount != 8 {
		t.Fatalf("expected listed batch audit, got %#v", batches)
	}
	read, err := ReadRuntimeWorkerPlacementDispatchBatch(root, filepath.Base(batch.Path))
	if err != nil {
		t.Fatalf("read placement dispatch batch: %v", err)
	}
	if read.Path != batch.Path || len(read.Executions) != 8 {
		t.Fatalf("expected inspected batch audit, got %#v", read)
	}
}

func TestExecuteRuntimeWorkerPlacementDispatchBatchSkipsNoLongerReadyClaims(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	plan := createPlacementDispatchBatchPlanFixture(t, root, "metadata-only")
	alreadyDispatchedClaim := plan.Candidates[0].ClaimPath
	if _, err := ExecuteRuntimeWorkerQueueClaimDispatch(root, alreadyDispatchedClaim, RuntimeWorkerQueueDispatchExecutionOptions{
		Execute: true,
		Now:     time.Date(2026, 5, 30, 17, 3, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("pre-dispatch queue claim: %v", err)
	}

	batch, err := ExecuteRuntimeWorkerPlacementDispatchBatch(root, plan.Path, RuntimeWorkerPlacementDispatchBatchOptions{
		Execute: true,
		Now:     time.Date(2026, 5, 30, 17, 5, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("execute placement dispatch batch: %v", err)
	}
	if batch.Status != "completed" || batch.DispatchedCount != 7 || batch.SkippedCount != 1 || batch.FailedCount != 0 {
		t.Fatalf("expected seven dispatches and one skip, got %#v", batch)
	}
	sawSkip := false
	for _, execution := range batch.Executions {
		if execution.Status == "skipped" && execution.SkipReason == "claim-not-claimed" && execution.ClaimPath == alreadyDispatchedClaim {
			sawSkip = true
		}
	}
	if !sawSkip {
		t.Fatalf("expected skipped audit for already dispatched claim, got %#v", batch.Executions)
	}
	dispatches, err := ListRuntimeWorkerQueueDispatchExecutions(root)
	if err != nil {
		t.Fatalf("list queue dispatches: %v", err)
	}
	if len(dispatches) != 8 {
		t.Fatalf("batch should add only seven new queue dispatches, got %#v", dispatches)
	}
}

func createPlacementDispatchPlanFixture(t *testing.T, root string, transportMode string) RuntimeWorkerPlacementDispatchPlan {
	t.Helper()
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	now := time.Date(2026, 5, 30, 16, 0, 0, 0, time.UTC)
	if _, err := RegisterRuntimeWorker(root, RuntimeWorkerArtifact{
		WorkerID:     "remote-web-1",
		Mode:         "remote",
		Status:       "available",
		Endpoint:     "https://worker.example.test/dispatch",
		Capabilities: []string{"web.discovery"},
	}, RuntimeWorkerRegistrationOptions{Now: now}); err != nil {
		t.Fatalf("register worker: %v", err)
	}
	placement, err := CreateRuntimeWorkerPlacementPlan(root, RuntimeWorkerPlacementPlanOptions{Now: now, RunID: "run-placement-dispatch-exec"})
	if err != nil {
		t.Fatalf("create placement plan: %v", err)
	}
	if _, err := ExecuteRuntimeWorkerPlacementClaim(root, placement.Path, RuntimeWorkerPlacementClaimExecutionOptions{
		Execute:              true,
		CandidateExecutionID: "tool-discover_assets-01",
		WorkerID:             "remote-web-1",
		Now:                  now.Add(time.Minute),
		LeaseDuration:        10 * time.Minute,
	}); err != nil {
		t.Fatalf("execute placement claim: %v", err)
	}
	plan, err := CreateRuntimeWorkerPlacementDispatchPlan(root, RuntimeWorkerPlacementDispatchPlanOptions{
		Now:           now.Add(2 * time.Minute),
		TransportMode: transportMode,
	})
	if err != nil {
		t.Fatalf("create placement dispatch plan: %v", err)
	}
	if len(plan.Candidates) != 1 || plan.Candidates[0].Status != "ready" {
		t.Fatalf("expected ready placement dispatch plan fixture, got %#v", plan)
	}
	return plan
}

func createPlacementDispatchBatchPlanFixture(t *testing.T, root string, transportMode string) RuntimeWorkerPlacementDispatchPlan {
	t.Helper()
	return createPlacementDispatchBatchPlanFixtureAt(t, root, transportMode, time.Date(2026, 5, 30, 17, 0, 0, 0, time.UTC))
}

func createPlacementDispatchBatchPlanFixtureAt(t *testing.T, root string, transportMode string, now time.Time) RuntimeWorkerPlacementDispatchPlan {
	t.Helper()
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	workers := []RuntimeWorkerArtifact{
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
	}
	for _, worker := range workers {
		if _, err := RegisterRuntimeWorker(root, worker, RuntimeWorkerRegistrationOptions{Now: now}); err != nil {
			t.Fatalf("register worker %s: %v", worker.WorkerID, err)
		}
	}
	placement, err := CreateRuntimeWorkerPlacementPlan(root, RuntimeWorkerPlacementPlanOptions{Now: now, RunID: "run-placement-dispatch-batch"})
	if err != nil {
		t.Fatalf("create placement plan: %v", err)
	}
	if placement.ReadyCount != 8 {
		t.Fatalf("expected eight ready placement candidates, got %#v", placement)
	}
	for _, candidate := range placement.Candidates {
		if candidate.Status != "ready" {
			continue
		}
		if _, err := ExecuteRuntimeWorkerPlacementClaim(root, placement.Path, RuntimeWorkerPlacementClaimExecutionOptions{
			Execute:              true,
			CandidateExecutionID: candidate.ExecutionID,
			WorkerID:             candidate.WorkerID,
			Now:                  now.Add(time.Minute),
			LeaseDuration:        10 * time.Minute,
		}); err != nil {
			t.Fatalf("execute placement claim for %s/%s: %v", candidate.WorkerID, candidate.ExecutionID, err)
		}
	}
	plan, err := CreateRuntimeWorkerPlacementDispatchPlan(root, RuntimeWorkerPlacementDispatchPlanOptions{
		Now:           now.Add(2 * time.Minute),
		TransportMode: transportMode,
	})
	if err != nil {
		t.Fatalf("create placement dispatch plan: %v", err)
	}
	if plan.ReadyCount != 8 {
		t.Fatalf("expected eight ready placement dispatch candidates, got %#v", plan)
	}
	return plan
}

func placementCandidateByCapability(candidates []RuntimeWorkerPlacementCandidate, capability string) RuntimeWorkerPlacementCandidate {
	for _, candidate := range candidates {
		if candidate.Capability == capability {
			return candidate
		}
	}
	return RuntimeWorkerPlacementCandidate{}
}

func placementCandidateByToolID(candidates []RuntimeWorkerPlacementCandidate, toolID string) RuntimeWorkerPlacementCandidate {
	for _, candidate := range candidates {
		if candidate.ToolPlan.ToolID == toolID {
			return candidate
		}
	}
	return RuntimeWorkerPlacementCandidate{}
}

func placementCandidateByParser(candidates []RuntimeWorkerPlacementCandidate, parser string) RuntimeWorkerPlacementCandidate {
	for _, candidate := range candidates {
		if candidate.Parser == parser || candidate.ToolPlan.ParserID == parser {
			return candidate
		}
	}
	return RuntimeWorkerPlacementCandidate{}
}
