package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCreateRuntimeWorkerDispatchRetrySchedulerRunDryRunWritesOnlySchedulerArtifact(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Date(2026, 6, 2, 10, 0, 0, 0, time.UTC)
	marker := filepath.Join(t.TempDir(), "worker-dispatch-retry-scheduler-dry-run-ran")
	createRuntimeWorkerDispatchRetrySchedulerFailedResultFixture(t, root, now, RuntimeWorkerToolPlanPayload{
		Action:     "discover_assets",
		Runner:     "command",
		Parser:     "web_api_parser",
		RunCommand: []string{"sh", "-c", "touch " + marker},
	})
	before := snapshotRuntimeWorkerDispatchRetrySchedulerSideEffects(t, root)

	run, err := CreateRuntimeWorkerDispatchRetrySchedulerRun(root, RuntimeWorkerDispatchRetrySchedulerRunOptions{
		Now:     now.Add(10 * time.Minute),
		Actor:   "scheduler",
		Reason:  "review retry schedule",
		MaxRuns: 2,
	})
	if err != nil {
		t.Fatalf("create dry-run worker dispatch retry scheduler run: %v", err)
	}
	if run.Status != "dry-run" || run.Mode != "metadata-only" {
		t.Fatalf("unexpected scheduler metadata: %#v", run)
	}
	if run.Actor != "scheduler" || run.Reason != "review retry schedule" || run.CreatedAt != "2026-06-02T10:10:00Z" {
		t.Fatalf("unexpected scheduler audit fields: %#v", run)
	}
	if run.MaxRunsRequested != 2 || run.MaxRuns != 2 || run.RunsCreated != 0 || run.RetryPlansCreated != 0 || run.RetryClaimsCreated != 0 {
		t.Fatalf("dry-run scheduler must not create child retry work: %#v", run)
	}
	if run.DispatchRetryClaims || run.RetryTransportMode != "metadata-only" {
		t.Fatalf("retry handoff must be default-off metadata-only intent, got %#v", run)
	}
	if run.CurrentSummary.Ready != 1 || run.CurrentSummary.Total != 1 {
		t.Fatalf("dry-run scheduler should summarize current retry candidates without writing them, got %#v", run.CurrentSummary)
	}
	if !strings.Contains(run.Path, filepath.Join("runtime", "worker-dispatch-retry-scheduler-runs")) {
		t.Fatalf("expected scheduler artifact under runtime dir, got %s", run.Path)
	}
	assertRuntimeWorkerDispatchRetrySchedulerOnlyChanged(t, root, before, run.Path)
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("dry-run scheduler must not execute stored tool command, marker stat err=%v", err)
	}

	read, err := ReadRuntimeWorkerDispatchRetrySchedulerRun(root, filepath.Base(run.Path))
	if err != nil {
		t.Fatalf("read worker dispatch retry scheduler run: %v", err)
	}
	if read.Path != run.Path || read.CurrentSummary.Ready != 1 {
		t.Fatalf("unexpected read scheduler run: %#v", read)
	}
	runs, err := ListRuntimeWorkerDispatchRetrySchedulerRuns(root)
	if err != nil {
		t.Fatalf("list worker dispatch retry scheduler runs: %v", err)
	}
	if len(runs) != 1 || runs[0].Path != run.Path {
		t.Fatalf("expected one listed scheduler run, got %#v", runs)
	}
	if _, err := ReadRuntimeWorkerDispatchRetrySchedulerRun(root, filepath.Join("..", "scheduler-run.json")); err == nil || !strings.Contains(err.Error(), "worker dispatch retry scheduler run path must stay under") {
		t.Fatalf("expected path confinement error, got %v", err)
	}
}

func TestCreateRuntimeWorkerDispatchRetrySchedulerRunExecuteCreatesBoundedRetryClaimsOnce(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Date(2026, 6, 2, 10, 30, 0, 0, time.UTC)
	marker := filepath.Join(t.TempDir(), "worker-dispatch-retry-scheduler-execute-ran")
	createRuntimeWorkerDispatchRetrySchedulerFailedResultFixture(t, root, now, RuntimeWorkerToolPlanPayload{
		Action:     "discover_assets",
		Runner:     "command",
		Parser:     "web_api_parser",
		RunCommand: []string{"sh", "-c", "touch " + marker},
	})

	run, err := CreateRuntimeWorkerDispatchRetrySchedulerRun(root, RuntimeWorkerDispatchRetrySchedulerRunOptions{
		Execute:       true,
		Now:           now.Add(10 * time.Minute),
		Actor:         "scheduler",
		Reason:        "execute retry schedule",
		MaxRuns:       2,
		LeaseDuration: 20 * time.Minute,
	})
	if err != nil {
		t.Fatalf("execute worker dispatch retry scheduler run: %v", err)
	}
	if run.Status != "completed" || run.Mode != "execute" {
		t.Fatalf("expected completed execute scheduler run, got %#v", run)
	}
	if run.MaxRunsRequested != 2 || run.RunsCreated != 1 || run.RetryPlansCreated != 1 || run.RetryDispatchesCreated != 1 || run.RetryDispatchExecutionsCreated != 1 {
		t.Fatalf("expected bounded single retry run despite max-runs=2, got %#v", run)
	}
	if run.RetryClaimsCreated != 1 || run.RetryClaimsDispatched != 0 || run.RetryTransportRequestsCreated != 0 {
		t.Fatalf("default execute should create only one metadata queue claim, got %#v", run)
	}
	if len(run.RetryPlanPaths) != 1 || len(run.RetryDispatchPaths) != 1 || len(run.RetryDispatchExecutionPaths) != 1 {
		t.Fatalf("scheduler should link one child plan/dispatch/execution, got %#v", run)
	}
	if countJSON(t, filepath.Join(root, "runtime", "worker-dispatch-retry-plans")) != 1 ||
		countJSON(t, filepath.Join(root, "runtime", "worker-dispatch-retry-dispatches")) != 1 ||
		countJSON(t, filepath.Join(root, "runtime", "worker-dispatch-retry-dispatch-executions")) != 1 {
		t.Fatalf("expected one retry plan, dispatch, and execution artifact")
	}
	if countJSON(t, filepath.Join(root, "runtime", "worker-queue-claims")) != 2 {
		t.Fatalf("expected original claim plus one retry queue claim")
	}
	if countJSON(t, filepath.Join(root, "runtime", "worker-queue-dispatch-executions")) != 1 {
		t.Fatalf("default retry scheduler must not dispatch retry queue claims")
	}
	if countJSON(t, filepath.Join(root, "runtime", "worker-transport-outbox", "remote-web-1")) != 1 {
		t.Fatalf("default retry scheduler must not create retry transport requests")
	}
	if countJSON(t, filepath.Join(root, "runtime", "worker-queue-dispatch-results")) != 1 {
		t.Fatalf("retry scheduler must not submit results")
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("retry scheduler must not execute stored tool command, marker stat err=%v", err)
	}

	executions, err := ListRuntimeWorkerDispatchRetryDispatchExecutions(root)
	if err != nil {
		t.Fatalf("list retry dispatch executions: %v", err)
	}
	if len(executions) != 1 || executions[0].Summary.Claimed != 1 {
		t.Fatalf("unexpected child retry execution: %#v", executions)
	}
}

func TestCreateRuntimeWorkerDispatchRetrySchedulerRunCanHandoffRetryClaimsToFileOutbox(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Date(2026, 6, 2, 11, 0, 0, 0, time.UTC)
	marker := filepath.Join(t.TempDir(), "worker-dispatch-retry-scheduler-handoff-ran")
	createRuntimeWorkerDispatchRetrySchedulerFailedResultFixture(t, root, now, RuntimeWorkerToolPlanPayload{
		Action:     "discover_assets",
		Runner:     "command",
		Parser:     "web_api_parser",
		RunCommand: []string{"sh", "-c", "touch " + marker},
	})

	run, err := CreateRuntimeWorkerDispatchRetrySchedulerRun(root, RuntimeWorkerDispatchRetrySchedulerRunOptions{
		Execute:             true,
		Now:                 now.Add(10 * time.Minute),
		MaxRuns:             1,
		DispatchRetryClaims: true,
		RetryTransportMode:  "file-outbox",
	})
	if err != nil {
		t.Fatalf("execute worker dispatch retry scheduler run with handoff: %v", err)
	}
	if run.Status != "completed" || !run.DispatchRetryClaims || run.RetryTransportMode != "file-outbox" {
		t.Fatalf("unexpected scheduler handoff metadata: %#v", run)
	}
	if run.RetryClaimsCreated != 1 || run.RetryClaimsDispatched != 1 || run.RetryTransportRequestsCreated != 1 {
		t.Fatalf("expected one retry claim handoff, got %#v", run)
	}
	if countJSON(t, filepath.Join(root, "runtime", "worker-queue-claims")) != 2 {
		t.Fatalf("expected original claim plus one retry claim")
	}
	if countJSON(t, filepath.Join(root, "runtime", "worker-queue-dispatch-executions")) != 2 {
		t.Fatalf("expected original dispatch plus one retry queue-dispatch audit")
	}
	if countJSON(t, filepath.Join(root, "runtime", "worker-transport-outbox", "remote-web-1")) != 2 {
		t.Fatalf("expected original request plus one pending retry outbox request")
	}
	if countJSON(t, filepath.Join(root, "runtime", "worker-queue-dispatch-results")) != 1 {
		t.Fatalf("handoff must not submit queue dispatch results")
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("retry scheduler handoff must not execute stored tool command, marker stat err=%v", err)
	}
}

func TestCreateRuntimeWorkerDispatchRetrySchedulerRunFailsClosedBeforeWrites(t *testing.T) {
	now := time.Date(2026, 6, 2, 11, 30, 0, 0, time.UTC)
	tests := []struct {
		name string
		opts RuntimeWorkerDispatchRetrySchedulerRunOptions
		want string
	}{
		{
			name: "execute without positive max runs",
			opts: RuntimeWorkerDispatchRetrySchedulerRunOptions{Execute: true, MaxRuns: 0, Now: now},
			want: "worker dispatch retry scheduler run max runs must be positive",
		},
		{
			name: "dry run negative max runs",
			opts: RuntimeWorkerDispatchRetrySchedulerRunOptions{MaxRuns: -1, Now: now},
			want: "worker dispatch retry scheduler run max runs cannot be negative",
		},
		{
			name: "handoff without execute",
			opts: RuntimeWorkerDispatchRetrySchedulerRunOptions{DispatchRetryClaims: true, MaxRuns: 1, Now: now},
			want: "worker dispatch retry scheduler run retry claim handoff requires execute",
		},
		{
			name: "http transport rejected",
			opts: RuntimeWorkerDispatchRetrySchedulerRunOptions{Execute: true, DispatchRetryClaims: true, RetryTransportMode: "http-json", MaxRuns: 1, Now: now},
			want: "worker dispatch retry claim handoff supports metadata-only or file-outbox transport",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "workspace")
			if err := InitWorkspace(root); err != nil {
				t.Fatalf("init workspace: %v", err)
			}
			_, err := CreateRuntimeWorkerDispatchRetrySchedulerRun(root, tc.opts)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected %q error, got %v", tc.want, err)
			}
			if countJSON(t, filepath.Join(root, "runtime", "worker-dispatch-retry-scheduler-runs")) != 0 ||
				countJSON(t, filepath.Join(root, "runtime", "worker-dispatch-retry-plans")) != 0 ||
				countJSON(t, filepath.Join(root, "runtime", "worker-dispatch-retry-dispatches")) != 0 ||
				countJSON(t, filepath.Join(root, "runtime", "worker-dispatch-retry-dispatch-executions")) != 0 ||
				countJSON(t, filepath.Join(root, "runtime", "worker-queue-claims")) != 0 {
				t.Fatalf("invalid scheduler options must fail before writing artifacts")
			}
		})
	}
}

func createRuntimeWorkerDispatchRetrySchedulerFailedResultFixture(t *testing.T, root string, now time.Time, plan RuntimeWorkerToolPlanPayload) RuntimeWorkerQueueDispatchExecution {
	t.Helper()
	dispatch := createFileOutboxRuntimeWorkerQueueDispatchWithToolPlan(t, root, now, plan)
	if _, err := SubmitRuntimeWorkerQueueDispatchResult(root, filepath.Base(dispatch.Path), RuntimeWorkerQueueDispatchResult{
		Status: "failed",
		Error:  "exit status 23",
	}, RuntimeWorkerQueueDispatchResultOptions{Now: now.Add(time.Minute)}); err != nil {
		t.Fatalf("submit failed dispatch result: %v", err)
	}
	return dispatch
}

func snapshotRuntimeWorkerDispatchRetrySchedulerSideEffects(t *testing.T, root string) map[string]int {
	t.Helper()
	paths := []string{
		"runtime/worker-dispatch-retry-scheduler-runs",
		"runtime/worker-dispatch-retry-plans",
		"runtime/worker-dispatch-retry-dispatches",
		"runtime/worker-dispatch-retry-dispatch-executions",
		"runtime/worker-queue-claims",
		"runtime/worker-queue-dispatch-executions",
		"runtime/worker-queue-dispatch-results",
		"runtime/worker-transport-outbox",
		"runtime/worker-transport-daemon-runs",
		"runtime/worker-transport-daemon-supervisor-runs",
		"runtime/worker-transport-daemon-controls",
		"runtime/worker-leases",
		"runtime/worker-lease-reclaims",
		"runtime/worker-lease-reclaim-executions",
		"runtime/worker-lease-result-reconciliations",
		"runtime/worker-lease-result-reconciliation-executions",
	}
	counts := make(map[string]int, len(paths))
	for _, rel := range paths {
		counts[rel] = countJSONRecursive(t, filepath.Join(root, rel))
	}
	return counts
}

func assertRuntimeWorkerDispatchRetrySchedulerOnlyChanged(t *testing.T, root string, before map[string]int, schedulerPath string) {
	t.Helper()
	after := snapshotRuntimeWorkerDispatchRetrySchedulerSideEffects(t, root)
	for path, beforeCount := range before {
		want := beforeCount
		if path == "runtime/worker-dispatch-retry-scheduler-runs" {
			want++
		}
		if after[path] != want {
			t.Fatalf("scheduler dry-run changed %s artifacts: before=%d want=%d after=%d scheduler=%s", path, beforeCount, want, after[path], schedulerPath)
		}
	}
}
