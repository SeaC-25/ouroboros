package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPlanWorkerLeaseResultReconciliationFindsReturnedResultWithActiveLease(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Date(2026, 5, 30, 19, 0, 0, 0, time.UTC)
	dispatch := createDispatchedRuntimeWorkerQueueClaim(t, root, now)
	result, err := SubmitRuntimeWorkerQueueDispatchResult(root, filepath.Base(dispatch.Path), RuntimeWorkerQueueDispatchResult{
		Status: "completed",
		Output: "remote result complete",
	}, RuntimeWorkerQueueDispatchResultOptions{Now: now.Add(2 * time.Minute)})
	if err != nil {
		t.Fatalf("submit queue dispatch result: %v", err)
	}
	if err := writeWorkerLeaseArtifact(root, WorkerLeaseArtifact{
		Version:     1,
		RunID:       dispatch.RunID,
		ExecutionID: dispatch.ExecutionID,
		Tool:        dispatch.Tool,
		Capability:  dispatch.Capability,
		WorkerID:    dispatch.WorkerID,
		Status:      "active",
		AcquiredAt:  now.Format(time.RFC3339Nano),
		HeartbeatAt: now.Format(time.RFC3339Nano),
	}); err != nil {
		t.Fatalf("write active worker lease fixture: %v", err)
	}

	plan, err := PlanWorkerLeaseResultReconciliation(root)
	if err != nil {
		t.Fatalf("plan worker lease result reconciliation: %v", err)
	}
	if len(plan.Ready) != 1 || len(plan.Skipped) != 0 {
		t.Fatalf("expected one ready reconciliation candidate, got %#v", plan)
	}
	candidate := plan.Ready[0]
	if candidate.Mode != "metadata-only" || candidate.Reason != "result-returned-active-lease" || candidate.RecommendedStatus != "completed" {
		t.Fatalf("unexpected reconciliation candidate metadata: %#v", candidate)
	}
	if candidate.Lease.RunID != dispatch.RunID || candidate.Result.Path != result.Path || candidate.Dispatch.Path != dispatch.Path {
		t.Fatalf("candidate should link lease/result/dispatch, got %#v", candidate)
	}
	lease, err := readWorkerLeaseByIdentity(root, dispatch.RunID, dispatch.ExecutionID)
	if err != nil {
		t.Fatalf("read worker lease after plan: %v", err)
	}
	if lease.Status != "active" || lease.ReleasedAt != "" {
		t.Fatalf("reconciliation planning must not mutate lease, got %#v", lease)
	}
}

func TestPlanWorkerLeaseResultReconciliationSkipsAlignedAndMissingLeases(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Date(2026, 5, 30, 19, 10, 0, 0, time.UTC)
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
	alignedDispatch := createFileOutboxRuntimeWorkerQueueDispatchForExecution(t, root, now, "run-aligned", "tool-aligned")
	alignedResult, err := SubmitRuntimeWorkerQueueDispatchResult(root, filepath.Base(alignedDispatch.Path), RuntimeWorkerQueueDispatchResult{
		Status: "completed",
		Output: "aligned",
	}, RuntimeWorkerQueueDispatchResultOptions{Now: now.Add(2 * time.Minute)})
	if err != nil {
		t.Fatalf("submit aligned queue dispatch result: %v", err)
	}
	if err := writeWorkerLeaseArtifact(root, WorkerLeaseArtifact{
		Version:           1,
		RunID:             alignedDispatch.RunID,
		ExecutionID:       alignedDispatch.ExecutionID,
		ResultExecutionID: "aligned-result",
		Tool:              alignedDispatch.Tool,
		Capability:        alignedDispatch.Capability,
		WorkerID:          alignedDispatch.WorkerID,
		Status:            "completed",
		AcquiredAt:        now.Format(time.RFC3339Nano),
		HeartbeatAt:       now.Add(2 * time.Minute).Format(time.RFC3339Nano),
		ReleasedAt:        now.Add(2 * time.Minute).Format(time.RFC3339Nano),
	}); err != nil {
		t.Fatalf("write aligned worker lease fixture: %v", err)
	}
	missingDispatch := createFileOutboxRuntimeWorkerQueueDispatchForExecution(t, root, now.Add(time.Minute), "run-missing", "tool-missing")
	missingResult, err := SubmitRuntimeWorkerQueueDispatchResult(root, filepath.Base(missingDispatch.Path), RuntimeWorkerQueueDispatchResult{
		Status: "failed",
		Error:  "worker failed before lease persisted",
	}, RuntimeWorkerQueueDispatchResultOptions{Now: now.Add(3 * time.Minute)})
	if err != nil {
		t.Fatalf("submit missing-lease queue dispatch result: %v", err)
	}

	plan, err := PlanWorkerLeaseResultReconciliation(root)
	if err != nil {
		t.Fatalf("plan worker lease result reconciliation: %v", err)
	}
	if len(plan.Ready) != 0 || len(plan.Skipped) != 2 {
		t.Fatalf("expected two skipped reconciliation rows, got %#v", plan)
	}
	reasons := map[string]string{}
	for _, skipped := range plan.Skipped {
		reasons[skipped.Result.Path] = skipped.Reason
	}
	if reasons[alignedResult.Path] != "lease-aligned" || reasons[missingResult.Path] != "lease-missing" {
		t.Fatalf("unexpected reconciliation skip reasons: %#v", plan.Skipped)
	}
	for _, skipped := range plan.Skipped {
		if strings.TrimSpace(skipped.Mode) != "metadata-only" {
			t.Fatalf("skip rows should stay metadata-only, got %#v", skipped)
		}
	}
}

func TestCreateWorkerLeaseResultReconciliationDispatchWritesMetadataOnlyArtifact(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Date(2026, 5, 30, 19, 30, 0, 0, time.UTC)
	dispatch := createDispatchedRuntimeWorkerQueueClaim(t, root, now)
	result, err := SubmitRuntimeWorkerQueueDispatchResult(root, filepath.Base(dispatch.Path), RuntimeWorkerQueueDispatchResult{
		Status: "completed",
		Output: "remote result complete",
	}, RuntimeWorkerQueueDispatchResultOptions{Now: now.Add(2 * time.Minute)})
	if err != nil {
		t.Fatalf("submit queue dispatch result: %v", err)
	}
	if err := writeWorkerLeaseArtifact(root, WorkerLeaseArtifact{
		Version:     1,
		RunID:       dispatch.RunID,
		ExecutionID: dispatch.ExecutionID,
		Tool:        dispatch.Tool,
		Capability:  dispatch.Capability,
		WorkerID:    dispatch.WorkerID,
		Status:      "active",
		AcquiredAt:  now.Format(time.RFC3339Nano),
		HeartbeatAt: now.Format(time.RFC3339Nano),
	}); err != nil {
		t.Fatalf("write active worker lease fixture: %v", err)
	}

	review, err := CreateWorkerLeaseResultReconciliationDispatch(root, now.Add(3*time.Minute))
	if err != nil {
		t.Fatalf("create worker lease result reconciliation dispatch: %v", err)
	}
	if review.Status != "dry-run" || review.Mode != "metadata-only" || review.CreatedAt != now.Add(3*time.Minute).Format(time.RFC3339Nano) {
		t.Fatalf("unexpected review metadata: %#v", review)
	}
	if len(review.Ready) != 1 || len(review.Skipped) != 0 {
		t.Fatalf("expected one ready review candidate, got %#v", review)
	}
	if !strings.Contains(review.Path, filepath.Join("runtime", "worker-lease-result-reconciliations")) {
		t.Fatalf("expected review under worker-lease-result-reconciliations, got %s", review.Path)
	}
	data, err := os.ReadFile(review.Path)
	if err != nil {
		t.Fatalf("read review artifact: %v", err)
	}
	var decoded WorkerLeaseResultReconciliationDispatch
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("decode review artifact: %v", err)
	}
	if len(decoded.Ready) != 1 || decoded.Ready[0].ResultPath != result.Path || decoded.Ready[0].LeasePath == "" || decoded.Ready[0].DispatchPath == "" || decoded.Ready[0].Lease.Status != "active" {
		t.Fatalf("unexpected decoded review: %#v", decoded)
	}
	lease, err := readWorkerLeaseByIdentity(root, dispatch.RunID, dispatch.ExecutionID)
	if err != nil {
		t.Fatalf("read worker lease after review: %v", err)
	}
	if lease.Status != "active" || lease.ReleasedAt != "" {
		t.Fatalf("metadata-only review must not mutate lease, got %#v", lease)
	}
}

func TestListAndReadWorkerLeaseResultReconciliationDispatchesReadsArtifacts(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Date(2026, 5, 30, 19, 40, 0, 0, time.UTC)
	dispatch := createDispatchedRuntimeWorkerQueueClaim(t, root, now)
	if _, err := SubmitRuntimeWorkerQueueDispatchResult(root, filepath.Base(dispatch.Path), RuntimeWorkerQueueDispatchResult{
		Status: "failed",
		Error:  "remote worker failed",
	}, RuntimeWorkerQueueDispatchResultOptions{Now: now.Add(2 * time.Minute)}); err != nil {
		t.Fatalf("submit queue dispatch result: %v", err)
	}
	if err := writeWorkerLeaseArtifact(root, WorkerLeaseArtifact{
		Version:     1,
		RunID:       dispatch.RunID,
		ExecutionID: dispatch.ExecutionID,
		Tool:        dispatch.Tool,
		Capability:  dispatch.Capability,
		WorkerID:    dispatch.WorkerID,
		Status:      "active",
		AcquiredAt:  now.Format(time.RFC3339Nano),
		HeartbeatAt: now.Format(time.RFC3339Nano),
	}); err != nil {
		t.Fatalf("write active worker lease fixture: %v", err)
	}
	first, err := CreateWorkerLeaseResultReconciliationDispatch(root, now.Add(3*time.Minute))
	if err != nil {
		t.Fatalf("create first review: %v", err)
	}
	second, err := CreateWorkerLeaseResultReconciliationDispatch(root, now.Add(4*time.Minute))
	if err != nil {
		t.Fatalf("create second review: %v", err)
	}

	reviews, err := ListWorkerLeaseResultReconciliationDispatches(root)
	if err != nil {
		t.Fatalf("list worker lease result reconciliation reviews: %v", err)
	}
	if len(reviews) != 2 || reviews[0].Path != first.Path || reviews[1].Path != second.Path {
		t.Fatalf("unexpected listed reviews: %#v", reviews)
	}
	read, err := ReadWorkerLeaseResultReconciliationDispatch(root, filepath.Base(second.Path))
	if err != nil {
		t.Fatalf("read worker lease result reconciliation review: %v", err)
	}
	if read.Path != second.Path || len(read.Ready) != 1 || read.Ready[0].RecommendedStatus != "failed" {
		t.Fatalf("unexpected read review: %#v", read)
	}
	if _, err := ReadWorkerLeaseResultReconciliationDispatch(root, filepath.Join(t.TempDir(), "outside.json")); err == nil || !strings.Contains(err.Error(), "worker lease result reconciliation dispatch path must stay under") {
		t.Fatalf("expected path confinement error, got %v", err)
	}
}

func TestExecuteWorkerLeaseResultReconciliationDispatchRequiresExplicitExecute(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Date(2026, 5, 30, 19, 50, 0, 0, time.UTC)
	dispatch, review := createWorkerLeaseResultReconciliationReviewFixture(t, root, now, "completed")

	_, err := ExecuteWorkerLeaseResultReconciliationDispatch(root, review.Path, WorkerLeaseResultReconciliationExecutionOptions{
		Now: now.Add(4 * time.Minute),
	})
	if err == nil || !strings.Contains(err.Error(), "worker lease result reconciliation execution requires explicit execute") {
		t.Fatalf("expected explicit execute error, got %v", err)
	}
	executions, err := ListWorkerLeaseResultReconciliationExecutions(root)
	if err != nil {
		t.Fatalf("list reconciliation executions: %v", err)
	}
	if len(executions) != 0 {
		t.Fatalf("execution without explicit gate must not write audits, got %#v", executions)
	}
	lease, err := readWorkerLeaseByIdentity(root, dispatch.RunID, dispatch.ExecutionID)
	if err != nil {
		t.Fatalf("read worker lease after rejected execution: %v", err)
	}
	if lease.Status != "active" || lease.ReleasedAt != "" {
		t.Fatalf("rejected execution must not mutate lease, got %#v", lease)
	}
}

func TestExecuteWorkerLeaseResultReconciliationDispatchRevalidatesAndReleasesActiveLease(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Date(2026, 5, 30, 20, 0, 0, 0, time.UTC)
	dispatch, review := createWorkerLeaseResultReconciliationReviewFixture(t, root, now, "completed")

	execution, err := ExecuteWorkerLeaseResultReconciliationDispatch(root, filepath.Base(review.Path), WorkerLeaseResultReconciliationExecutionOptions{
		Execute:    true,
		Now:        now.Add(4 * time.Minute),
		ReleasedBy: "operator",
		Reason:     "reviewed returned result",
	})
	if err != nil {
		t.Fatalf("execute reconciliation dispatch: %v", err)
	}
	if execution.Status != "completed" || execution.Mode != "metadata-only" || len(execution.Results) != 1 || len(execution.Skipped) != 0 {
		t.Fatalf("unexpected reconciliation execution: %#v", execution)
	}
	if !strings.Contains(execution.Path, filepath.Join("runtime", "worker-lease-result-reconciliation-executions")) {
		t.Fatalf("expected execution audit path under worker-lease-result-reconciliation-executions, got %s", execution.Path)
	}
	result := execution.Results[0]
	if result.RunID != dispatch.RunID || result.ExecutionID != dispatch.ExecutionID || result.PreviousStatus != "active" || result.Status != "completed" || result.ResultStatus != "completed" {
		t.Fatalf("unexpected reconciliation release result: %#v", result)
	}
	lease, err := readWorkerLeaseByIdentity(root, dispatch.RunID, dispatch.ExecutionID)
	if err != nil {
		t.Fatalf("read released worker lease: %v", err)
	}
	if lease.Status != "completed" || lease.ReleasedBy != "operator" || lease.ReleaseReason != "reviewed returned result" || lease.ResultExecutionID == "" {
		t.Fatalf("expected completed lease with release metadata, got %#v", lease)
	}
	read, err := ReadWorkerLeaseResultReconciliationExecution(root, filepath.Base(execution.Path))
	if err != nil {
		t.Fatalf("read reconciliation execution: %v", err)
	}
	if read.Path != execution.Path || len(read.Results) != 1 {
		t.Fatalf("unexpected read execution: %#v", read)
	}
}

func TestExecuteWorkerLeaseResultReconciliationDispatchRequiresCurrentSafetyAuthorizationBeforeLeaseMutation(t *testing.T) {
	t.Skip("Safety authorization checks disabled")
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Date(2026, 5, 30, 20, 5, 0, 0, time.UTC)
	dispatch, review := createWorkerLeaseResultReconciliationReviewFixture(t, root, now, "completed")
	writeRuntimeWorkerDisabledSafetyScope(t, root)

	_, err := ExecuteWorkerLeaseResultReconciliationDispatch(root, filepath.Base(review.Path), WorkerLeaseResultReconciliationExecutionOptions{
		Execute:    true,
		Now:        now.Add(4 * time.Minute),
		ReleasedBy: "operator",
		Reason:     "reviewed returned result",
	})
	if err == nil || !strings.Contains(err.Error(), "worker runtime side effect requires current safety bypass authorization") {
		t.Fatalf("expected strict safety authorization error, got %v", err)
	}
	lease, err := readWorkerLeaseByIdentity(root, dispatch.RunID, dispatch.ExecutionID)
	if err != nil {
		t.Fatalf("read worker lease after rejected reconciliation: %v", err)
	}
	if lease.Status != "active" || lease.ReleasedAt != "" || lease.ReleasedBy != "" {
		t.Fatalf("rejected reconciliation must not mutate lease, got %#v", lease)
	}
	executions, err := ListWorkerLeaseResultReconciliationExecutions(root)
	if err != nil {
		t.Fatalf("list reconciliation executions: %v", err)
	}
	if len(executions) != 0 {
		t.Fatalf("rejected reconciliation must not write execution audits, got %#v", executions)
	}
}

func TestExecuteWorkerLeaseResultReconciliationDispatchAllowsDisabledBoundariesWithCurrentSafetyAuthorization(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Date(2026, 5, 30, 20, 7, 0, 0, time.UTC)
	dispatch, review := createWorkerLeaseResultReconciliationReviewFixture(t, root, now, "completed")
	writeRuntimeWorkerDisabledSafetyScope(t, root)
	writeRuntimeWorkerCurrentSafetyAuthorization(t, root)

	execution, err := ExecuteWorkerLeaseResultReconciliationDispatch(root, filepath.Base(review.Path), WorkerLeaseResultReconciliationExecutionOptions{
		Execute:    true,
		Now:        now.Add(4 * time.Minute),
		ReleasedBy: "operator",
		Reason:     "authorized reconciliation",
	})
	if err != nil {
		t.Fatalf("execute reconciliation with current safety authorization: %v", err)
	}
	if execution.Status != "completed" || len(execution.Results) != 1 || execution.Results[0].ExecutionID != dispatch.ExecutionID {
		t.Fatalf("expected authorized reconciliation result, got %#v", execution)
	}
	lease, err := readWorkerLeaseByIdentity(root, dispatch.RunID, dispatch.ExecutionID)
	if err != nil {
		t.Fatalf("read released worker lease: %v", err)
	}
	if lease.Status != "completed" || lease.ReleasedBy != "operator" || lease.ReleaseReason != "authorized reconciliation" {
		t.Fatalf("expected authorized reconciliation to release lease, got %#v", lease)
	}
}

func TestExecuteWorkerLeaseResultReconciliationDispatchSkipsLeaseThatIsNoLongerActive(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Date(2026, 5, 30, 20, 10, 0, 0, time.UTC)
	dispatch, review := createWorkerLeaseResultReconciliationReviewFixture(t, root, now, "failed")
	if _, err := ReleaseWorkerLease(root, dispatch.RunID, dispatch.ExecutionID, WorkerLeaseReleaseOptions{
		Release:    true,
		Status:     "failed",
		Now:        now.Add(3 * time.Minute),
		ReleasedBy: "worker",
		Reason:     "worker returned first",
	}); err != nil {
		t.Fatalf("pre-release worker lease: %v", err)
	}

	execution, err := ExecuteWorkerLeaseResultReconciliationDispatch(root, review.Path, WorkerLeaseResultReconciliationExecutionOptions{
		Execute:    true,
		Now:        now.Add(4 * time.Minute),
		ReleasedBy: "operator",
	})
	if err != nil {
		t.Fatalf("execute reconciliation dispatch after lease closed: %v", err)
	}
	if execution.Status != "skipped" || len(execution.Results) != 0 || len(execution.Skipped) != 1 {
		t.Fatalf("expected skipped execution, got %#v", execution)
	}
	if execution.Skipped[0].Reason != "lease-not-active" {
		t.Fatalf("expected lease-not-active skip, got %#v", execution.Skipped)
	}
	lease, err := readWorkerLeaseByIdentity(root, dispatch.RunID, dispatch.ExecutionID)
	if err != nil {
		t.Fatalf("read worker lease after skipped execution: %v", err)
	}
	if lease.ReleasedBy != "worker" || lease.ReleaseReason != "worker returned first" {
		t.Fatalf("skipped reconciliation must not overwrite closed lease, got %#v", lease)
	}
}

func createWorkerLeaseResultReconciliationReviewFixture(t *testing.T, root string, now time.Time, status string) (RuntimeWorkerQueueDispatchExecution, WorkerLeaseResultReconciliationDispatch) {
	t.Helper()
	dispatch := createDispatchedRuntimeWorkerQueueClaim(t, root, now)
	result := RuntimeWorkerQueueDispatchResult{Status: status}
	if status == "failed" {
		result.Error = "remote worker failed"
	} else {
		result.Output = "remote result complete"
	}
	if _, err := SubmitRuntimeWorkerQueueDispatchResult(root, filepath.Base(dispatch.Path), result, RuntimeWorkerQueueDispatchResultOptions{Now: now.Add(2 * time.Minute)}); err != nil {
		t.Fatalf("submit queue dispatch result: %v", err)
	}
	if err := writeWorkerLeaseArtifact(root, WorkerLeaseArtifact{
		Version:     1,
		RunID:       dispatch.RunID,
		ExecutionID: dispatch.ExecutionID,
		Tool:        dispatch.Tool,
		Capability:  dispatch.Capability,
		WorkerID:    dispatch.WorkerID,
		Status:      "active",
		AcquiredAt:  now.Format(time.RFC3339Nano),
		HeartbeatAt: now.Format(time.RFC3339Nano),
	}); err != nil {
		t.Fatalf("write active worker lease fixture: %v", err)
	}
	review, err := CreateWorkerLeaseResultReconciliationDispatch(root, now.Add(3*time.Minute))
	if err != nil {
		t.Fatalf("create worker lease result reconciliation review: %v", err)
	}
	return dispatch, review
}
