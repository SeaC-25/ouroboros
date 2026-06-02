package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPlanWorkerLifecycleReviewSummarizesLeasesAndReadiness(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Date(2026, 5, 31, 8, 0, 0, 0, time.UTC)
	staleAfter := 5 * time.Minute
	oldHeartbeat := now.Add(-staleAfter - time.Minute).Format(time.RFC3339Nano)
	freshHeartbeat := now.Add(-time.Minute).Format(time.RFC3339Nano)
	for _, fixture := range []WorkerLeaseArtifact{
		{
			Version:     1,
			RunID:       "run-stale",
			ExecutionID: "tool-stale",
			Tool:        "web-api-pack",
			Capability:  "web.discovery",
			WorkerID:    "local",
			Status:      "active",
			AcquiredAt:  oldHeartbeat,
			HeartbeatAt: oldHeartbeat,
		},
		{
			Version:     1,
			RunID:       "run-fresh",
			ExecutionID: "tool-fresh",
			Tool:        "infra-pack",
			Capability:  "infra.host_discovery",
			WorkerID:    "local",
			Status:      "active",
			AcquiredAt:  freshHeartbeat,
			HeartbeatAt: freshHeartbeat,
		},
		{
			Version:     1,
			RunID:       "run-done",
			ExecutionID: "tool-done",
			Tool:        "ctf-pack",
			Capability:  "ctf.fast_hypothesis",
			WorkerID:    "local",
			Status:      "completed",
			AcquiredAt:  oldHeartbeat,
			HeartbeatAt: oldHeartbeat,
			ReleasedAt:  oldHeartbeat,
		},
	} {
		if err := writeWorkerLeaseArtifact(root, fixture); err != nil {
			t.Fatalf("write worker lease fixture: %v", err)
		}
	}
	dispatch, _ := createWorkerLeaseResultReconciliationReviewFixture(t, root, now, "completed")

	plan, err := PlanWorkerLifecycleReview(root, now, staleAfter)
	if err != nil {
		t.Fatalf("plan worker lifecycle review: %v", err)
	}
	if plan.Mode != "metadata-only" || plan.StaleAfterSeconds != int64(staleAfter/time.Second) {
		t.Fatalf("unexpected review metadata: %#v", plan)
	}
	if plan.LeaseSummary.Total != 4 || plan.LeaseSummary.Active != 3 || plan.LeaseSummary.StaleActive != 1 || plan.LeaseSummary.Completed != 1 {
		t.Fatalf("unexpected lease summary: %#v", plan.LeaseSummary)
	}
	if len(plan.Reclaim.Ready) != 1 || plan.Reclaim.Ready[0].Lease.ExecutionID != "tool-stale" {
		t.Fatalf("expected stale lease ready for reclaim review, got %#v", plan.Reclaim)
	}
	if len(plan.Reconciliation.Ready) != 1 || plan.Reconciliation.Ready[0].Result.RunID != dispatch.RunID {
		t.Fatalf("expected returned result ready for reconciliation review, got %#v", plan.Reconciliation)
	}
	if dispatches, err := ListWorkerLeaseReclaimDispatches(root); err != nil || len(dispatches) != 0 {
		t.Fatalf("planning must not write reclaim artifacts, dispatches=%#v err=%v", dispatches, err)
	}
	if reviews, err := ListWorkerLifecycleReviews(root); err != nil || len(reviews) != 0 {
		t.Fatalf("planning must not write lifecycle review artifacts, reviews=%#v err=%v", reviews, err)
	}
}

func TestPlanWorkerLifecycleReviewSummarizesQueueTransportAndResults(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Date(2026, 5, 31, 9, 0, 0, 0, time.UTC)
	staleAfter := 5 * time.Minute

	pendingDispatch := createFileOutboxRuntimeWorkerQueueDispatch(t, root, now)
	ackedDispatch := createFileOutboxRuntimeWorkerQueueDispatchForExecution(t, root, now.Add(time.Minute), "run-lifecycle-acked", "tool-acked")
	completedDispatch := createFileOutboxRuntimeWorkerQueueDispatchForExecution(t, root, now.Add(2*time.Minute), "run-lifecycle-completed", "tool-completed")
	if _, err := AcknowledgeRuntimeWorkerTransportDispatchRequest(root, "remote-web-1", filepath.Base(ackedDispatch.TransportRequestPath), RuntimeWorkerTransportAckOptions{
		Ack: true,
		Now: now.Add(3 * time.Minute),
	}); err != nil {
		t.Fatalf("ack lifecycle transport request: %v", err)
	}
	if _, err := SubmitRuntimeWorkerQueueDispatchResult(root, filepath.Base(completedDispatch.Path), RuntimeWorkerQueueDispatchResult{
		Status: "completed",
		Output: "done",
	}, RuntimeWorkerQueueDispatchResultOptions{Now: now.Add(4 * time.Minute)}); err != nil {
		t.Fatalf("submit lifecycle dispatch result: %v", err)
	}

	plan, err := PlanWorkerLifecycleReview(root, now.Add(5*time.Minute), staleAfter)
	if err != nil {
		t.Fatalf("plan worker lifecycle review: %v", err)
	}
	if plan.QueueSummary.Total != 3 || plan.QueueSummary.Dispatched != 2 || plan.QueueSummary.Completed != 1 {
		t.Fatalf("unexpected queue summary: %#v", plan.QueueSummary)
	}
	if plan.TransportSummary.RequestsTotal != 3 || plan.TransportSummary.Pending != 1 || plan.TransportSummary.Acknowledged != 1 || plan.TransportSummary.Completed != 1 {
		t.Fatalf("unexpected transport request summary: %#v", plan.TransportSummary)
	}
	if plan.TransportSummary.DispatchResultsTotal != 1 || plan.TransportSummary.DispatchResultsCompleted != 1 || plan.TransportSummary.DispatchResultsFailed != 0 {
		t.Fatalf("unexpected dispatch result summary: %#v", plan.TransportSummary)
	}

	requests, err := ListRuntimeWorkerTransportDispatchRequests(root, "remote-web-1")
	if err != nil {
		t.Fatalf("list transport requests after lifecycle review: %v", err)
	}
	statuses := map[string]string{}
	for _, request := range requests {
		statuses[request.ExecutionID] = request.Status
	}
	if statuses[pendingDispatch.ExecutionID] != "pending" || statuses[ackedDispatch.ExecutionID] != "acknowledged" || statuses[completedDispatch.ExecutionID] != "completed" {
		t.Fatalf("metadata-only lifecycle review must not mutate transport requests, got %#v", statuses)
	}
}

func TestCreateWorkerLifecycleReviewWritesMetadataOnlyArtifact(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Date(2026, 5, 31, 8, 30, 0, 0, time.UTC)
	staleAfter := 5 * time.Minute
	oldHeartbeat := now.Add(-staleAfter - time.Minute).Format(time.RFC3339Nano)
	if err := writeWorkerLeaseArtifact(root, WorkerLeaseArtifact{
		Version:     1,
		RunID:       "run-stale",
		ExecutionID: "tool-stale",
		Tool:        "web-api-pack",
		Capability:  "web.discovery",
		WorkerID:    "local",
		Status:      "active",
		AcquiredAt:  oldHeartbeat,
		HeartbeatAt: oldHeartbeat,
	}); err != nil {
		t.Fatalf("write stale worker lease fixture: %v", err)
	}

	review, err := CreateWorkerLifecycleReview(root, now, staleAfter)
	if err != nil {
		t.Fatalf("create worker lifecycle review: %v", err)
	}
	if review.Status != "dry-run" || review.Mode != "metadata-only" {
		t.Fatalf("expected dry-run metadata-only review, got %#v", review)
	}
	if len(review.Reclaim.Ready) != 1 || review.LeaseSummary.StaleActive != 1 {
		t.Fatalf("expected stale reclaim readiness in review, got %#v", review)
	}
	if !strings.Contains(review.Path, filepath.Join("runtime", "worker-lifecycle-reviews")) {
		t.Fatalf("expected review under worker-lifecycle-reviews, got %s", review.Path)
	}
	data, err := os.ReadFile(review.Path)
	if err != nil {
		t.Fatalf("read worker lifecycle review artifact: %v", err)
	}
	var decoded WorkerLifecycleReview
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("decode worker lifecycle review artifact: %v", err)
	}
	if decoded.Status != "dry-run" || decoded.Mode != "metadata-only" || decoded.LeaseSummary.StaleActive != 1 {
		t.Fatalf("unexpected decoded lifecycle review: %#v", decoded)
	}
	if decoded.QueueSummary.Total != 0 || decoded.TransportSummary.RequestsTotal != 0 || decoded.TransportSummary.DispatchResultsTotal != 0 {
		t.Fatalf("expected empty queue/transport summary in decoded lifecycle review, got queue=%#v transport=%#v", decoded.QueueSummary, decoded.TransportSummary)
	}
	leases, err := ListWorkerLeases(root)
	if err != nil {
		t.Fatalf("list worker leases after lifecycle review: %v", err)
	}
	if len(leases) != 1 || leases[0].Status != "active" || leases[0].ReleasedAt != "" {
		t.Fatalf("metadata-only lifecycle review must not mutate leases, got %#v", leases)
	}
	if reclaims, err := ListWorkerLeaseReclaimDispatches(root); err != nil || len(reclaims) != 0 {
		t.Fatalf("lifecycle review must not write reclaim dispatches, reclaims=%#v err=%v", reclaims, err)
	}
}

func TestListAndReadWorkerLifecycleReviewsReadsArtifacts(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Date(2026, 5, 31, 8, 45, 0, 0, time.UTC)
	staleAfter := 5 * time.Minute
	if err := writeWorkerLeaseArtifact(root, WorkerLeaseArtifact{
		Version:     1,
		RunID:       "run-a",
		ExecutionID: "tool-a",
		Tool:        "web-api-pack",
		Capability:  "web.discovery",
		WorkerID:    "local",
		Status:      "completed",
		AcquiredAt:  now.Format(time.RFC3339Nano),
		HeartbeatAt: now.Format(time.RFC3339Nano),
		ReleasedAt:  now.Format(time.RFC3339Nano),
	}); err != nil {
		t.Fatalf("write worker lease fixture: %v", err)
	}
	first, err := CreateWorkerLifecycleReview(root, now, staleAfter)
	if err != nil {
		t.Fatalf("create first lifecycle review: %v", err)
	}
	second, err := CreateWorkerLifecycleReview(root, now.Add(time.Minute), staleAfter)
	if err != nil {
		t.Fatalf("create second lifecycle review: %v", err)
	}

	reviews, err := ListWorkerLifecycleReviews(root)
	if err != nil {
		t.Fatalf("list worker lifecycle reviews: %v", err)
	}
	if len(reviews) != 2 || reviews[0].Path != first.Path || reviews[1].Path != second.Path {
		t.Fatalf("unexpected listed lifecycle reviews: %#v", reviews)
	}
	read, err := ReadWorkerLifecycleReview(root, filepath.Base(second.Path))
	if err != nil {
		t.Fatalf("read worker lifecycle review: %v", err)
	}
	if read.Path != second.Path || read.LeaseSummary.Completed != 1 {
		t.Fatalf("unexpected read lifecycle review: %#v", read)
	}
	if _, err := ReadWorkerLifecycleReview(root, filepath.Join(t.TempDir(), "outside.json")); err == nil || !strings.Contains(err.Error(), "worker lifecycle review path must stay under") {
		t.Fatalf("expected lifecycle review path confinement error, got %v", err)
	}
}
