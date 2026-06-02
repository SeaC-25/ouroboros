package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPlanWorkerLifecycleActionPlanRecommendsExplicitGatesWithoutMutating(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Date(2026, 5, 31, 10, 0, 0, 0, time.UTC)
	staleAfter := 5 * time.Minute
	oldHeartbeat := now.Add(-staleAfter - time.Minute).Format(time.RFC3339Nano)

	if err := writeWorkerLeaseArtifact(root, WorkerLeaseArtifact{
		Version:     1,
		RunID:       "run-stale-action",
		ExecutionID: "tool-stale-action",
		Tool:        "web-api-pack",
		Capability:  "web.discovery",
		WorkerID:    "local",
		Status:      "active",
		AcquiredAt:  oldHeartbeat,
		HeartbeatAt: oldHeartbeat,
	}); err != nil {
		t.Fatalf("write stale worker lease fixture: %v", err)
	}
	pendingDispatch := createFileOutboxRuntimeWorkerQueueDispatch(t, root, now)
	ackedDispatch := createFileOutboxRuntimeWorkerQueueDispatchForExecution(t, root, now.Add(time.Minute), "run-action-acked", "tool-action-acked")
	if _, err := AcknowledgeRuntimeWorkerTransportDispatchRequest(root, "remote-web-1", filepath.Base(ackedDispatch.TransportRequestPath), RuntimeWorkerTransportAckOptions{
		Ack: true,
		Now: now.Add(2 * time.Minute),
	}); err != nil {
		t.Fatalf("ack worker transport request fixture: %v", err)
	}
	reconcileDispatch := createFileOutboxRuntimeWorkerQueueDispatchForExecution(t, root, now.Add(3*time.Minute), "run-action-reconcile", "tool-action-reconcile")
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
		t.Fatalf("write reconciliation worker lease fixture: %v", err)
	}
	review, err := CreateWorkerLifecycleReview(root, now.Add(4*time.Minute), staleAfter)
	if err != nil {
		t.Fatalf("create worker lifecycle review fixture: %v", err)
	}

	plan, err := PlanWorkerLifecycleActionPlan(root, filepath.Base(review.Path), now.Add(4*time.Minute), staleAfter)
	if err != nil {
		t.Fatalf("plan worker lifecycle actions: %v", err)
	}
	if plan.Status != "dry-run" || plan.Mode != "metadata-only" || plan.ReviewPath != review.Path {
		t.Fatalf("unexpected action plan metadata: %#v", plan)
	}
	if plan.Summary.Total != 4 || plan.Summary.AckPendingTransport != 1 || plan.Summary.RunAcknowledgedTransport != 1 || plan.Summary.ReclaimStaleLease != 1 || plan.Summary.ReconcileResultLease != 1 {
		t.Fatalf("unexpected action summary: %#v", plan.Summary)
	}
	actions := workerLifecycleActionsByType(plan.Actions)
	for actionType, gate := range map[string]string{
		"ack-transport-request":  "worker-transport-ack",
		"run-transport-request":  "worker-transport-run-once",
		"reclaim-stale-lease":    "worker-lease-reclaim",
		"reconcile-result-lease": "worker-lease-result-reconcile",
	} {
		action := actions[actionType]
		if action.Type != actionType || action.Gate != gate || action.Mode != "metadata-only" || action.Status != "ready" {
			t.Fatalf("unexpected action for %s: %#v", actionType, action)
		}
	}
	if actions["ack-transport-request"].RequestPath != pendingDispatch.TransportRequestPath {
		t.Fatalf("expected pending request path in ack action, got %#v", actions["ack-transport-request"])
	}
	if actions["run-transport-request"].RequestPath != ackedDispatch.TransportRequestPath {
		t.Fatalf("expected acknowledged request path in run action, got %#v", actions["run-transport-request"])
	}
	if actions["reclaim-stale-lease"].LeasePath == "" || actions["reclaim-stale-lease"].ExecutionID != "tool-stale-action" {
		t.Fatalf("expected stale lease path in reclaim action, got %#v", actions["reclaim-stale-lease"])
	}
	if actions["reconcile-result-lease"].ResultPath == "" || actions["reconcile-result-lease"].LeasePath == "" || actions["reconcile-result-lease"].RecommendedStatus != "completed" {
		t.Fatalf("expected result/lease paths in reconcile action, got %#v", actions["reconcile-result-lease"])
	}

	requests, err := ListRuntimeWorkerTransportDispatchRequests(root, "remote-web-1")
	if err != nil {
		t.Fatalf("list transport requests after action planning: %v", err)
	}
	statuses := map[string]string{}
	for _, request := range requests {
		statuses[request.ExecutionID] = request.Status
	}
	if statuses[pendingDispatch.ExecutionID] != "pending" || statuses["tool-action-acked"] != "acknowledged" || statuses["tool-action-reconcile"] != "completed" {
		t.Fatalf("action planning must not mutate transport requests, got %#v", statuses)
	}
	leases, err := ListWorkerLeases(root)
	if err != nil {
		t.Fatalf("list leases after action planning: %v", err)
	}
	for _, lease := range leases {
		if lease.Status != "active" {
			t.Fatalf("action planning must not mutate active leases, got %#v", leases)
		}
	}
	if actionPlans, err := ListWorkerLifecycleActionPlans(root); err != nil || len(actionPlans) != 0 {
		t.Fatalf("planning must not write action-plan artifacts, plans=%#v err=%v", actionPlans, err)
	}
}

func TestPlanWorkerLifecycleActionPlanRecommendsPendingDaemonControlWithoutMutating(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Date(2026, 5, 31, 11, 0, 0, 0, time.UTC)
	stopControl, err := CreateRuntimeWorkerTransportDaemonControlRequest(root, "remote-web-1", RuntimeWorkerTransportDaemonControlRequest{
		Action:      "stop",
		Reason:      "maintenance window",
		RequestedBy: "operator",
	}, RuntimeWorkerTransportDaemonControlRequestOptions{Now: now})
	if err != nil {
		t.Fatalf("create stop daemon control fixture: %v", err)
	}
	restartControl, err := CreateRuntimeWorkerTransportDaemonControlRequest(root, "remote-web-2", RuntimeWorkerTransportDaemonControlRequest{
		Action:      "restart",
		Reason:      "reload worker config",
		RequestedBy: "operator",
	}, RuntimeWorkerTransportDaemonControlRequestOptions{Now: now.Add(time.Minute)})
	if err != nil {
		t.Fatalf("create restart daemon control fixture: %v", err)
	}

	plan, err := PlanWorkerLifecycleActionPlan(root, "", now.Add(2*time.Minute), DefaultWorkerLeaseStaleAfter)
	if err != nil {
		t.Fatalf("plan worker lifecycle daemon control action: %v", err)
	}
	if plan.Status != "dry-run" || plan.Mode != "metadata-only" {
		t.Fatalf("unexpected action plan metadata: %#v", plan)
	}
	if plan.Summary.Total != 2 || plan.Summary.ApplyDaemonControl != 2 {
		t.Fatalf("unexpected daemon control action summary: %#v", plan.Summary)
	}
	action := plan.Actions[0]
	if action.Type != "apply-daemon-control" || action.Gate != "worker-transport-run-daemon-enable-control" || action.Status != "ready" || action.Mode != "metadata-only" {
		t.Fatalf("unexpected daemon control action: %#v", action)
	}
	if action.Reason != "pending-daemon-control" || action.WorkerID != "remote-web-1" || action.RequestPath != stopControl.Path || action.RecommendedStatus != "stop" {
		t.Fatalf("expected pending stop daemon control recommendation, got %#v", action)
	}
	if plan.Actions[1].WorkerID != "remote-web-2" || plan.Actions[1].RequestPath != restartControl.Path || plan.Actions[1].RecommendedStatus != "restart" {
		t.Fatalf("expected pending restart daemon control recommendation, got %#v", plan.Actions[1])
	}

	controls, err := ListRuntimeWorkerTransportDaemonControlRequests(root, "remote-web-1")
	if err != nil {
		t.Fatalf("list daemon controls after action planning: %v", err)
	}
	if len(controls) != 1 || controls[0].Status != "pending" || controls[0].AppliedAt != "" || controls[0].AppliedDaemonRunPath != "" {
		t.Fatalf("action planning must not apply daemon controls, got %#v", controls)
	}
	if actionPlans, err := ListWorkerLifecycleActionPlans(root); err != nil || len(actionPlans) != 0 {
		t.Fatalf("planning must not write action-plan artifacts, plans=%#v err=%v", actionPlans, err)
	}
}

func TestPlanWorkerLifecycleActionPlanRecommendsWorkerDispatchRetryDispatchExecutionWithoutMutating(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Date(2026, 5, 31, 15, 10, 0, 0, time.UTC)
	marker := filepath.Join(t.TempDir(), "retry-dispatch-action-ran")
	_, retryDispatch, _ := createRuntimeWorkerDispatchRetryDispatchFixture(t, root, now, RuntimeWorkerToolPlanPayload{
		Action:     "discover_assets",
		Runner:     "command",
		Parser:     "web_api_parser",
		RunCommand: []string{"sh", "-c", "touch " + marker},
	})
	claimsBefore := countJSON(t, filepath.Join(root, "runtime", "worker-queue-claims"))

	plan, err := PlanWorkerLifecycleActionPlan(root, "", now.Add(4*time.Minute), DefaultWorkerLeaseStaleAfter)
	if err != nil {
		t.Fatalf("plan worker lifecycle retry-dispatch action: %v", err)
	}

	if plan.Status != "dry-run" || plan.Mode != "metadata-only" {
		t.Fatalf("unexpected action plan metadata: %#v", plan)
	}
	if plan.Summary.Total != 1 || plan.Summary.ExecuteDispatchRetryDispatch != 1 {
		t.Fatalf("unexpected retry-dispatch action summary: %#v", plan.Summary)
	}
	action := plan.Actions[0]
	if action.Type != "execute-dispatch-retry-dispatch" || action.Gate != "worker-dispatch-retry-dispatch-execute" || action.Status != "ready" || action.Mode != "metadata-only" {
		t.Fatalf("unexpected retry-dispatch action: %#v", action)
	}
	if action.Reason != "ready-worker-dispatch-retry-dispatch" || action.DispatchPath != retryDispatch.Path || action.RecommendedStatus != "execute" {
		t.Fatalf("expected retry-dispatch execution recommendation, got %#v", action)
	}
	if got := countJSON(t, filepath.Join(root, "runtime", "worker-queue-claims")); got != claimsBefore {
		t.Fatalf("action planning must not create retry queue claims: before=%d after=%d", claimsBefore, got)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("action planning must not execute retry tool-plan command, marker stat err=%v", err)
	}
	executions, err := ListRuntimeWorkerDispatchRetryDispatchExecutions(root)
	if err != nil {
		t.Fatalf("list retry-dispatch executions after action planning: %v", err)
	}
	if len(executions) != 0 {
		t.Fatalf("action planning must not execute retry dispatch artifacts, got %#v", executions)
	}
}

func TestPlanWorkerLifecycleActionPlanSkipsExecutedWorkerDispatchRetryDispatch(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Date(2026, 5, 31, 15, 35, 0, 0, time.UTC)
	_, retryDispatch, _ := createRuntimeWorkerDispatchRetryDispatchFixture(t, root, now)
	if _, err := ExecuteRuntimeWorkerDispatchRetryDispatch(root, filepath.Base(retryDispatch.Path), RuntimeWorkerDispatchRetryDispatchExecutionOptions{
		Execute: true,
		Now:     now.Add(4 * time.Minute),
	}); err != nil {
		t.Fatalf("execute worker dispatch retry dispatch fixture: %v", err)
	}
	claimsBefore := countJSON(t, filepath.Join(root, "runtime", "worker-queue-claims"))

	plan, err := PlanWorkerLifecycleActionPlan(root, "", now.Add(5*time.Minute), DefaultWorkerLeaseStaleAfter)
	if err != nil {
		t.Fatalf("plan worker lifecycle action after retry execution: %v", err)
	}

	if plan.Summary.ExecuteDispatchRetryDispatch != 0 {
		t.Fatalf("executed retry-dispatch artifact must not be recommended again, got %#v", plan)
	}
	if got := countJSON(t, filepath.Join(root, "runtime", "worker-queue-claims")); got != claimsBefore {
		t.Fatalf("action planning must not create additional queue claims: before=%d after=%d", claimsBefore, got)
	}
}

func TestCreateWorkerLifecycleActionPlanWritesMetadataOnlyArtifact(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Date(2026, 5, 31, 10, 30, 0, 0, time.UTC)
	dispatch := createFileOutboxRuntimeWorkerQueueDispatch(t, root, now)
	review, err := CreateWorkerLifecycleReview(root, now.Add(time.Minute), DefaultWorkerLeaseStaleAfter)
	if err != nil {
		t.Fatalf("create worker lifecycle review fixture: %v", err)
	}

	plan, err := CreateWorkerLifecycleActionPlan(root, filepath.Base(review.Path), now.Add(2*time.Minute), DefaultWorkerLeaseStaleAfter)
	if err != nil {
		t.Fatalf("create worker lifecycle action plan: %v", err)
	}
	if plan.Path == "" || !strings.Contains(plan.Path, filepath.Join("runtime", "worker-lifecycle-action-plans")) {
		t.Fatalf("expected action plan under worker-lifecycle-action-plans, got %s", plan.Path)
	}
	if plan.Status != "dry-run" || plan.Mode != "metadata-only" || plan.Summary.AckPendingTransport != 1 {
		t.Fatalf("unexpected persisted action plan: %#v", plan)
	}
	data, err := os.ReadFile(plan.Path)
	if err != nil {
		t.Fatalf("read worker lifecycle action plan artifact: %v", err)
	}
	var decoded WorkerLifecycleActionPlan
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("decode worker lifecycle action plan artifact: %v", err)
	}
	if decoded.Status != "dry-run" || decoded.Mode != "metadata-only" || decoded.Summary.Total != 1 || len(decoded.Actions) != 1 {
		t.Fatalf("unexpected decoded action plan: %#v", decoded)
	}
	if decoded.Actions[0].RequestPath != dispatch.TransportRequestPath || decoded.Actions[0].Gate != "worker-transport-ack" {
		t.Fatalf("expected pending request ack action in decoded artifact, got %#v", decoded.Actions)
	}
	read, err := ReadWorkerLifecycleActionPlan(root, filepath.Base(plan.Path))
	if err != nil {
		t.Fatalf("read worker lifecycle action plan: %v", err)
	}
	if read.Path != plan.Path || read.Summary.Total != 1 {
		t.Fatalf("unexpected read worker lifecycle action plan: %#v", read)
	}
	plans, err := ListWorkerLifecycleActionPlans(root)
	if err != nil {
		t.Fatalf("list worker lifecycle action plans: %v", err)
	}
	if len(plans) != 1 || plans[0].Path != plan.Path {
		t.Fatalf("unexpected listed worker lifecycle action plans: %#v", plans)
	}
	if _, err := ReadWorkerLifecycleActionPlan(root, filepath.Join(t.TempDir(), "outside.json")); err == nil || !strings.Contains(err.Error(), "worker lifecycle action plan path must stay under") {
		t.Fatalf("expected action plan path confinement error, got %v", err)
	}
}

func TestAnnotateWorkerLifecycleActionPlanWritesMetadataOnlyDecision(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Date(2026, 6, 2, 9, 10, 0, 0, time.UTC)
	dispatch := createFileOutboxRuntimeWorkerQueueDispatch(t, root, now)
	plan, err := CreateWorkerLifecycleActionPlan(root, "", now.Add(time.Minute), DefaultWorkerLeaseStaleAfter)
	if err != nil {
		t.Fatalf("create action plan: %v", err)
	}

	annotated, err := AnnotateWorkerLifecycleActionPlan(root, filepath.Base(plan.Path), WorkerLifecycleActionPlanAnnotationOptions{
		Decision:   "accepted",
		Reviewer:   "alice",
		ReviewedAt: time.Date(2026, 6, 2, 9, 12, 0, 0, time.UTC),
		Reason:     "reviewed authorized maintenance action",
	})
	if err != nil {
		t.Fatalf("annotate action plan: %v", err)
	}
	if annotated.Path != plan.Path || annotated.Status != "dry-run" || annotated.Mode != "metadata-only" || annotated.Summary.Total != 1 {
		t.Fatalf("annotation must preserve action plan metadata: %#v", annotated)
	}
	if annotated.Annotation == nil {
		t.Fatalf("expected action plan annotation: %#v", annotated)
	}
	if annotated.Annotation.Decision != "accepted" || annotated.Annotation.Reviewer != "alice" || annotated.Annotation.ReviewedAt != "2026-06-02T09:12:00Z" || annotated.Annotation.Reason != "reviewed authorized maintenance action" {
		t.Fatalf("unexpected action plan annotation: %#v", annotated.Annotation)
	}
	read, err := ReadWorkerLifecycleActionPlan(root, filepath.Base(plan.Path))
	if err != nil {
		t.Fatalf("read annotated action plan: %v", err)
	}
	if read.Annotation == nil || read.Annotation.Decision != "accepted" {
		t.Fatalf("annotation was not persisted: %#v", read)
	}
	requests, err := ListRuntimeWorkerTransportDispatchRequests(root, dispatch.WorkerID)
	if err != nil {
		t.Fatalf("list transport requests after annotation: %v", err)
	}
	if len(requests) != 1 || requests[0].Status != "pending" {
		t.Fatalf("annotation must not acknowledge or run transport requests: %#v", requests)
	}
	if _, found, err := ReadWorkerLifecycleActionPlanSelection(root); err != nil || found {
		t.Fatalf("annotation must not write action-plan selection marker: found=%v err=%v", found, err)
	}
}

func TestAnnotateWorkerLifecycleActionPlanRejectsInvalidDecisionBeforeWriting(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Date(2026, 6, 2, 9, 20, 0, 0, time.UTC)
	createFileOutboxRuntimeWorkerQueueDispatch(t, root, now)
	plan, err := CreateWorkerLifecycleActionPlan(root, "", now.Add(time.Minute), DefaultWorkerLeaseStaleAfter)
	if err != nil {
		t.Fatalf("create action plan: %v", err)
	}
	before, err := os.ReadFile(plan.Path)
	if err != nil {
		t.Fatalf("read action plan before invalid annotation: %v", err)
	}
	for _, decision := range []string{"unknown", "false-positive"} {
		_, err = AnnotateWorkerLifecycleActionPlan(root, filepath.Base(plan.Path), WorkerLifecycleActionPlanAnnotationOptions{Decision: decision})
		if err == nil || !strings.Contains(err.Error(), "unsupported worker lifecycle action plan annotation decision") {
			t.Fatalf("expected invalid decision rejection for %q, got %v", decision, err)
		}
		after, err := os.ReadFile(plan.Path)
		if err != nil {
			t.Fatalf("read action plan after invalid annotation: %v", err)
		}
		if string(after) != string(before) {
			t.Fatalf("invalid annotation decision %q must not rewrite action plan", decision)
		}
	}
}

func TestSelectWorkerLifecycleActionPlanWritesCurrentMetadataOnlySelection(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Date(2026, 6, 2, 9, 30, 0, 0, time.UTC)
	dispatch := createFileOutboxRuntimeWorkerQueueDispatch(t, root, now)
	plan, err := CreateWorkerLifecycleActionPlan(root, "", now.Add(time.Minute), DefaultWorkerLeaseStaleAfter)
	if err != nil {
		t.Fatalf("create action plan: %v", err)
	}
	if _, err := AnnotateWorkerLifecycleActionPlan(root, filepath.Base(plan.Path), WorkerLifecycleActionPlanAnnotationOptions{Decision: "accepted"}); err != nil {
		t.Fatalf("accept action plan: %v", err)
	}

	selection, err := SelectWorkerLifecycleActionPlan(root, filepath.Base(plan.Path), WorkerLifecycleActionPlanSelectionOptions{
		SelectedBy: "alice",
		SelectedAt: time.Date(2026, 6, 2, 9, 31, 0, 0, time.UTC),
		Reason:     "current reviewed lifecycle action plan",
	})
	if err != nil {
		t.Fatalf("select action plan: %v", err)
	}
	if selection.Path != filepath.Join(root, "runtime", "worker-lifecycle-action-plan-selection.json") {
		t.Fatalf("unexpected selection marker path: %s", selection.Path)
	}
	if selection.Status != "current" || selection.Mode != "metadata-only" || selection.PlanPath != plan.Path || selection.PlanBasename != filepath.Base(plan.Path) {
		t.Fatalf("unexpected selection links/state: %#v", selection)
	}
	if selection.Summary.Total != 1 || selection.Summary.AckPendingTransport != 1 {
		t.Fatalf("selection must preserve action summary: %#v", selection.Summary)
	}
	if selection.SelectedBy != "alice" || selection.SelectedAt != "2026-06-02T09:31:00Z" || selection.Reason != "current reviewed lifecycle action plan" {
		t.Fatalf("unexpected selection operator metadata: %#v", selection)
	}
	read, found, err := ReadWorkerLifecycleActionPlanSelection(root)
	if err != nil {
		t.Fatalf("read action-plan selection: %v", err)
	}
	if !found || read.Status != "current" || read.PlanPath != plan.Path {
		t.Fatalf("unexpected persisted action-plan selection: found=%v selection=%#v", found, read)
	}
	requests, err := ListRuntimeWorkerTransportDispatchRequests(root, dispatch.WorkerID)
	if err != nil {
		t.Fatalf("list transport requests after select: %v", err)
	}
	if len(requests) != 1 || requests[0].Status != "pending" {
		t.Fatalf("selection must not acknowledge or run transport requests: %#v", requests)
	}
}

func TestSelectWorkerLifecycleActionPlanRequiresAcceptedAnnotationBeforeMarker(t *testing.T) {
	for _, decision := range []string{"", "deferred", "rejected"} {
		t.Run("decision="+decision, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "workspace")
			now := time.Date(2026, 6, 2, 9, 40, 0, 0, time.UTC)
			createFileOutboxRuntimeWorkerQueueDispatch(t, root, now)
			plan, err := CreateWorkerLifecycleActionPlan(root, "", now.Add(time.Minute), DefaultWorkerLeaseStaleAfter)
			if err != nil {
				t.Fatalf("create action plan: %v", err)
			}
			if decision != "" {
				if _, err := AnnotateWorkerLifecycleActionPlan(root, filepath.Base(plan.Path), WorkerLifecycleActionPlanAnnotationOptions{Decision: decision}); err != nil {
					t.Fatalf("annotate action plan as %s: %v", decision, err)
				}
			}

			_, err = SelectWorkerLifecycleActionPlan(root, filepath.Base(plan.Path), WorkerLifecycleActionPlanSelectionOptions{})
			if err == nil || !strings.Contains(err.Error(), "worker lifecycle action plan selection requires accepted annotation") {
				t.Fatalf("expected accepted annotation selection error, got %v", err)
			}
			if _, err := os.Stat(filepath.Join(root, "runtime", "worker-lifecycle-action-plan-selection.json")); !os.IsNotExist(err) {
				t.Fatalf("unaccepted action plan must not write selection marker, stat err=%v", err)
			}
		})
	}
}

func TestSelectWorkerLifecycleActionPlanRequiresDryRunMetadataOnlyVersionOne(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*WorkerLifecycleActionPlan)
		want   string
	}{
		{
			name: "unsupported version",
			mutate: func(plan *WorkerLifecycleActionPlan) {
				plan.Version = 2
			},
			want: "worker lifecycle action plan version is unsupported",
		},
		{
			name: "non dry-run status",
			mutate: func(plan *WorkerLifecycleActionPlan) {
				plan.Status = "completed"
			},
			want: "worker lifecycle action plan must be dry-run metadata-only before selection",
		},
		{
			name: "non metadata-only mode",
			mutate: func(plan *WorkerLifecycleActionPlan) {
				plan.Mode = "execute"
			},
			want: "worker lifecycle action plan must be dry-run metadata-only before selection",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "workspace")
			now := time.Date(2026, 6, 2, 9, 45, 0, 0, time.UTC)
			createFileOutboxRuntimeWorkerQueueDispatch(t, root, now)
			plan, err := CreateWorkerLifecycleActionPlan(root, "", now.Add(time.Minute), DefaultWorkerLeaseStaleAfter)
			if err != nil {
				t.Fatalf("create action plan: %v", err)
			}
			accepted, err := AnnotateWorkerLifecycleActionPlan(root, filepath.Base(plan.Path), WorkerLifecycleActionPlanAnnotationOptions{Decision: "accepted"})
			if err != nil {
				t.Fatalf("accept action plan: %v", err)
			}
			tc.mutate(&accepted)
			data, err := json.MarshalIndent(accepted, "", "  ")
			if err != nil {
				t.Fatalf("encode tampered action plan: %v", err)
			}
			data = append(data, '\n')
			if err := os.WriteFile(accepted.Path, data, 0o644); err != nil {
				t.Fatalf("write tampered action plan: %v", err)
			}

			_, err = SelectWorkerLifecycleActionPlan(root, filepath.Base(accepted.Path), WorkerLifecycleActionPlanSelectionOptions{})
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected selection gate error %q, got %v", tc.want, err)
			}
			if _, err := os.Stat(filepath.Join(root, "runtime", "worker-lifecycle-action-plan-selection.json")); !os.IsNotExist(err) {
				t.Fatalf("tampered action plan must not write selection marker, stat err=%v", err)
			}
		})
	}
}

func TestClearWorkerLifecycleActionPlanSelectionWritesClearedMetadataOnlyMarker(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Date(2026, 6, 2, 9, 50, 0, 0, time.UTC)
	createFileOutboxRuntimeWorkerQueueDispatch(t, root, now)
	plan, err := CreateWorkerLifecycleActionPlan(root, "", now.Add(time.Minute), DefaultWorkerLeaseStaleAfter)
	if err != nil {
		t.Fatalf("create action plan: %v", err)
	}
	if _, err := AnnotateWorkerLifecycleActionPlan(root, filepath.Base(plan.Path), WorkerLifecycleActionPlanAnnotationOptions{Decision: "accepted"}); err != nil {
		t.Fatalf("accept action plan: %v", err)
	}
	selected, err := SelectWorkerLifecycleActionPlan(root, filepath.Base(plan.Path), WorkerLifecycleActionPlanSelectionOptions{
		SelectedBy: "alice",
		SelectedAt: time.Date(2026, 6, 2, 9, 51, 0, 0, time.UTC),
		Reason:     "current action plan",
	})
	if err != nil {
		t.Fatalf("select action plan: %v", err)
	}

	cleared, err := ClearWorkerLifecycleActionPlanSelection(root, WorkerLifecycleActionPlanSelectionClearOptions{
		ClearedBy: "bob",
		ClearedAt: time.Date(2026, 6, 2, 9, 52, 0, 0, time.UTC),
		Reason:    "close false-positive lifecycle target boundary",
	})
	if err != nil {
		t.Fatalf("clear action-plan selection: %v", err)
	}
	if cleared.Path != selected.Path || cleared.Status != "cleared" || cleared.Mode != "metadata-only" || cleared.PlanPath != plan.Path {
		t.Fatalf("unexpected cleared action-plan marker: %#v", cleared)
	}
	if cleared.ClearedBy != "bob" || cleared.ClearedAt != "2026-06-02T09:52:00Z" || cleared.Reason != "close false-positive lifecycle target boundary" {
		t.Fatalf("unexpected clear operator metadata: %#v", cleared)
	}
	read, found, err := ReadWorkerLifecycleActionPlanSelection(root)
	if err != nil {
		t.Fatalf("read cleared action-plan selection: %v", err)
	}
	if !found || read.Status != "cleared" || read.PlanPath != plan.Path || read.ClearedBy != "bob" {
		t.Fatalf("unexpected persisted cleared marker: found=%v selection=%#v", found, read)
	}
}

func TestWorkerLifecycleActionPlanReviewSelectionRejectsUnsafePathsBeforeWriting(t *testing.T) {
	for _, tc := range []struct {
		name string
		run  func(t *testing.T, root string, plan WorkerLifecycleActionPlan, outside string) error
		want string
	}{
		{
			name: "annotate symlinked plan file",
			run: func(t *testing.T, root string, plan WorkerLifecycleActionPlan, outside string) error {
				t.Helper()
				if err := os.Remove(plan.Path); err != nil {
					t.Fatalf("remove plan before symlink: %v", err)
				}
				if err := os.Symlink(outside, plan.Path); err != nil {
					t.Fatalf("symlink plan: %v", err)
				}
				_, err := AnnotateWorkerLifecycleActionPlan(root, filepath.Base(plan.Path), WorkerLifecycleActionPlanAnnotationOptions{Decision: "accepted"})
				return err
			},
			want: "worker lifecycle action plan file must not be a symlink",
		},
		{
			name: "select symlinked plan file",
			run: func(t *testing.T, root string, plan WorkerLifecycleActionPlan, outside string) error {
				t.Helper()
				if _, err := AnnotateWorkerLifecycleActionPlan(root, filepath.Base(plan.Path), WorkerLifecycleActionPlanAnnotationOptions{Decision: "accepted"}); err != nil {
					t.Fatalf("accept action plan: %v", err)
				}
				if err := os.Remove(plan.Path); err != nil {
					t.Fatalf("remove plan before symlink: %v", err)
				}
				if err := os.Symlink(outside, plan.Path); err != nil {
					t.Fatalf("symlink plan: %v", err)
				}
				_, err := SelectWorkerLifecycleActionPlan(root, filepath.Base(plan.Path), WorkerLifecycleActionPlanSelectionOptions{})
				return err
			},
			want: "worker lifecycle action plan file must not be a symlink",
		},
		{
			name: "select symlinked marker",
			run: func(t *testing.T, root string, plan WorkerLifecycleActionPlan, outside string) error {
				t.Helper()
				if _, err := AnnotateWorkerLifecycleActionPlan(root, filepath.Base(plan.Path), WorkerLifecycleActionPlanAnnotationOptions{Decision: "accepted"}); err != nil {
					t.Fatalf("accept action plan: %v", err)
				}
				if err := os.Symlink(outside, filepath.Join(root, "runtime", "worker-lifecycle-action-plan-selection.json")); err != nil {
					t.Fatalf("symlink selection marker: %v", err)
				}
				_, err := SelectWorkerLifecycleActionPlan(root, filepath.Base(plan.Path), WorkerLifecycleActionPlanSelectionOptions{})
				return err
			},
			want: "worker lifecycle action plan selection file must not be a symlink",
		},
		{
			name: "clear symlinked marker",
			run: func(t *testing.T, root string, plan WorkerLifecycleActionPlan, outside string) error {
				t.Helper()
				if err := os.Symlink(outside, filepath.Join(root, "runtime", "worker-lifecycle-action-plan-selection.json")); err != nil {
					t.Fatalf("symlink selection marker: %v", err)
				}
				_, err := ClearWorkerLifecycleActionPlanSelection(root, WorkerLifecycleActionPlanSelectionClearOptions{})
				return err
			},
			want: "worker lifecycle action plan selection file must not be a symlink",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "workspace")
			now := time.Date(2026, 6, 2, 10, 0, 0, 0, time.UTC)
			createFileOutboxRuntimeWorkerQueueDispatch(t, root, now)
			plan, err := CreateWorkerLifecycleActionPlan(root, "", now.Add(time.Minute), DefaultWorkerLeaseStaleAfter)
			if err != nil {
				t.Fatalf("create action plan: %v", err)
			}
			outside := filepath.Join(t.TempDir(), "outside-action-plan.json")
			if err := os.WriteFile(outside, []byte("outside\n"), 0o644); err != nil {
				t.Fatalf("write outside target: %v", err)
			}

			err = tc.run(t, root, plan, outside)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected %q rejection, got %v", tc.want, err)
			}
			outsideData, err := os.ReadFile(outside)
			if err != nil {
				t.Fatalf("read outside target: %v", err)
			}
			if string(outsideData) != "outside\n" {
				t.Fatalf("unsafe action-plan path must not rewrite outside file, got %q", string(outsideData))
			}
		})
	}
}

func workerLifecycleActionsByType(actions []WorkerLifecycleAction) map[string]WorkerLifecycleAction {
	byType := map[string]WorkerLifecycleAction{}
	for _, action := range actions {
		byType[action.Type] = action
	}
	return byType
}
