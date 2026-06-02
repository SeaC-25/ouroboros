package app

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

func TestCreateWorkerLifecyclePolicyRunDryRunWritesOnlyRunArtifact(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Date(2026, 6, 1, 19, 0, 0, 0, time.UTC)
	createWorkerLifecycleMaintenanceFixture(t, root, now)
	marker := filepath.Join(t.TempDir(), "policy-run-dry-run-tool-side-effect")
	createFileOutboxRuntimeWorkerQueueDispatchWithToolPlan(t, root, now.Add(30*time.Second), RuntimeWorkerToolPlanPayload{
		Action:     "discover_assets",
		Runner:     "command",
		Parser:     "web_api_parser",
		RunCommand: []string{"sh", "-c", "touch " + marker},
	})
	policy, err := CreateWorkerLifecyclePolicy(root, WorkerLifecyclePolicyOptions{
		Now:                         now.Add(5 * time.Minute),
		StaleAfter:                  7 * time.Minute,
		WorkerStaleAfter:            3 * time.Minute,
		PollInterval:                2500 * time.Millisecond,
		MaxSupervisorRuns:           3,
		TransportMode:               "file-outbox",
		IncludeLifecycleMaintenance: true,
		ExecuteLifecycleRetries:     true,
	})
	if err != nil {
		t.Fatalf("create worker lifecycle policy: %v", err)
	}
	if _, err := SelectWorkerLifecyclePolicy(root, filepath.Base(policy.Path), WorkerLifecyclePolicySelectionOptions{
		SelectedBy: "alice",
		SelectedAt: now.Add(6 * time.Minute),
		Reason:     "current dry run policy",
	}); err != nil {
		t.Fatalf("select worker lifecycle policy: %v", err)
	}
	sideEffectsBefore := snapshotWorkerLifecyclePolicySideEffects(t, root)
	filesBefore := snapshotWorkerLifecyclePolicySelectionBoundaryFiles(t, root)

	run, err := CreateWorkerLifecyclePolicyRun(root, WorkerLifecyclePolicyRunOptions{
		Now:    now.Add(7 * time.Minute),
		Actor:  "runner",
		Reason: "operator dry run",
	})
	if err != nil {
		t.Fatalf("create dry-run worker lifecycle policy run: %v", err)
	}
	if run.Status != "dry-run" || run.Mode != "metadata-only" {
		t.Fatalf("unexpected dry-run policy run metadata: %#v", run)
	}
	if run.PolicyPath != policy.Path ||
		run.SelectionPath != filepath.Join(root, "runtime", "worker-lifecycle-policy-selection.json") ||
		run.PolicyBasename != filepath.Base(policy.Path) ||
		run.PollIntervalMilliseconds != 2500 ||
		run.MaxRuns != 3 ||
		run.TransportMode != "file-outbox" ||
		!run.IncludeLifecycleMaintenance ||
		!run.ExecuteLifecycleRetries ||
		run.Actor != "runner" ||
		run.CreatedAt != "2026-06-01T19:07:00Z" ||
		run.Reason != "operator dry run" {
		t.Fatalf("unexpected policy run fields: %#v", run)
	}
	if run.SupervisorPath != "" || run.SkipReason != "" || run.SupervisorSummary.Runs != 0 {
		t.Fatalf("dry-run policy run must not link supervisor state, got %#v", run)
	}
	if run.PolicySummary.WorkerRegistrations != policy.Summary.WorkerRegistrations {
		t.Fatalf("expected policy summary counts to be persisted, got %#v", run.PolicySummary)
	}
	if !strings.Contains(run.Path, filepath.Join("runtime", "worker-lifecycle-policy-runs")) {
		t.Fatalf("expected policy run artifact path, got %s", run.Path)
	}
	assertOnlyWorkerLifecyclePolicyRunFileChanged(t, root, filesBefore, run.Path)
	assertWorkerLifecyclePolicySideEffectsUnchanged(t, root, sideEffectsBefore)
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("dry-run policy runner must not execute stored tool command, marker stat err=%v", err)
	}

	read, err := ReadWorkerLifecyclePolicyRun(root, filepath.Base(run.Path))
	if err != nil {
		t.Fatalf("read worker lifecycle policy run: %v", err)
	}
	if read.Path != run.Path || read.PolicyPath != policy.Path || read.PolicySummary.WorkerRegistrations != run.PolicySummary.WorkerRegistrations {
		t.Fatalf("unexpected read policy run: %#v", read)
	}
	runs, err := ListWorkerLifecyclePolicyRuns(root)
	if err != nil {
		t.Fatalf("list worker lifecycle policy runs: %v", err)
	}
	if len(runs) != 1 || runs[0].Path != run.Path {
		t.Fatalf("expected one listed policy run, got %#v", runs)
	}
	if _, err := ReadWorkerLifecyclePolicyRun(root, filepath.Join(t.TempDir(), "outside.json")); err == nil || !strings.Contains(err.Error(), "worker lifecycle policy run path must stay under") {
		t.Fatalf("expected policy run path confinement error, got %v", err)
	}
}

func TestCreateWorkerLifecyclePolicyRunExecuteCreatesOneBoundedMaintenanceSupervisor(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Date(2026, 6, 1, 19, 30, 0, 0, time.UTC)
	createWorkerLifecycleMaintenanceFixture(t, root, now)
	marker := filepath.Join(t.TempDir(), "policy-run-execute-tool-side-effect")
	createFileOutboxRuntimeWorkerQueueDispatchWithToolPlan(t, root, now.Add(30*time.Second), RuntimeWorkerToolPlanPayload{
		Action:     "discover_assets",
		Runner:     "command",
		Parser:     "web_api_parser",
		RunCommand: []string{"sh", "-c", "touch " + marker},
	})
	policy, err := CreateWorkerLifecyclePolicy(root, WorkerLifecyclePolicyOptions{
		Now:                         now.Add(5 * time.Minute),
		StaleAfter:                  9 * time.Minute,
		WorkerStaleAfter:            3 * time.Minute,
		PollInterval:                1500 * time.Millisecond,
		MaxSupervisorRuns:           2,
		TransportMode:               "metadata-only",
		IncludeLifecycleMaintenance: true,
		ExecuteLifecycleRetries:     true,
	})
	if err != nil {
		t.Fatalf("create worker lifecycle policy: %v", err)
	}
	if _, err := SelectWorkerLifecyclePolicy(root, filepath.Base(policy.Path), WorkerLifecyclePolicySelectionOptions{
		SelectedBy: "alice",
		SelectedAt: now.Add(6 * time.Minute),
		Reason:     "current execute policy",
	}); err != nil {
		t.Fatalf("select worker lifecycle policy: %v", err)
	}
	transportOutboxBefore := countJSONRecursive(t, filepath.Join(root, "runtime", "worker-transport-outbox"))
	transportRunsBefore := countJSONRecursive(t, filepath.Join(root, "runtime", "worker-transport-daemon-runs"))
	transportSupervisorRunsBefore := countJSONRecursive(t, filepath.Join(root, "runtime", "worker-transport-daemon-supervisor-runs"))
	transportControlsBefore := countJSONRecursive(t, filepath.Join(root, "runtime", "worker-transport-daemon-controls"))
	queueDispatchExecutionsBefore := countJSON(t, filepath.Join(root, "runtime", "worker-queue-dispatch-executions"))
	queueDispatchResultsBefore := countJSON(t, filepath.Join(root, "runtime", "worker-queue-dispatch-results"))

	run, err := CreateWorkerLifecyclePolicyRun(root, WorkerLifecyclePolicyRunOptions{
		Execute: true,
		Now:     now.Add(7 * time.Minute),
		Actor:   "runner",
		Reason:  "execute selected policy",
	})
	if err != nil {
		t.Fatalf("execute worker lifecycle policy run: %v", err)
	}
	if run.Status != "completed" || run.Mode != "metadata-only" || run.SupervisorPath == "" {
		t.Fatalf("expected completed policy run with linked supervisor, got %#v", run)
	}
	if run.PolicyPath != policy.Path || run.MaxRuns != 2 || run.PollIntervalMilliseconds != 1500 || run.ExecuteLifecycleRetries != true {
		t.Fatalf("expected selected policy values to be persisted, got %#v", run)
	}
	supervisors, err := ListWorkerLifecycleMaintenanceSupervisors(root)
	if err != nil {
		t.Fatalf("list worker lifecycle maintenance supervisors: %v", err)
	}
	if len(supervisors) != 1 || supervisors[0].Path != run.SupervisorPath {
		t.Fatalf("expected exactly one maintenance supervisor linked by policy run, got run=%#v supervisors=%#v", run, supervisors)
	}
	supervisor := supervisors[0]
	if supervisor.Summary.MaxRuns != 2 || supervisor.Summary.Runs != 2 || supervisor.StaleAfterSeconds != 540 {
		t.Fatalf("expected supervisor to use selected policy bounds, got %#v", supervisor)
	}
	if run.SupervisorSummary.Runs != supervisor.Summary.Runs ||
		run.SupervisorSummary.MaxRuns != supervisor.Summary.MaxRuns ||
		run.SupervisorSummary.MaintenanceArtifactsCreated != supervisor.Summary.MaintenanceArtifactsCreated ||
		run.SupervisorSummary.AuditArtifactsCreated != supervisor.Summary.AuditArtifactsCreated {
		t.Fatalf("expected policy run to carry supervisor summary counts, got run=%#v supervisor=%#v", run.SupervisorSummary, supervisor.Summary)
	}
	if countJSON(t, filepath.Join(root, "runtime", "worker-lifecycle-maintenance-supervisor-runs")) != 1 {
		t.Fatalf("expected one supervisor artifact")
	}
	if countJSON(t, filepath.Join(root, "runtime", "worker-lifecycle-maintenance")) != 2 {
		t.Fatalf("expected supervisor to create two maintenance artifacts")
	}
	if countJSON(t, filepath.Join(root, "runtime", "worker-lifecycle-audits")) != 2 {
		t.Fatalf("expected supervisor to create two lifecycle audit artifacts")
	}
	if got := countJSONRecursive(t, filepath.Join(root, "runtime", "worker-transport-outbox")); got != transportOutboxBefore {
		t.Fatalf("policy runner must not create transport outbox artifacts: before=%d after=%d", transportOutboxBefore, got)
	}
	if got := countJSONRecursive(t, filepath.Join(root, "runtime", "worker-transport-daemon-runs")); got != transportRunsBefore {
		t.Fatalf("policy runner must not create transport daemon runs: before=%d after=%d", transportRunsBefore, got)
	}
	if got := countJSONRecursive(t, filepath.Join(root, "runtime", "worker-transport-daemon-supervisor-runs")); got != transportSupervisorRunsBefore {
		t.Fatalf("policy runner must not create transport daemon supervisor runs: before=%d after=%d", transportSupervisorRunsBefore, got)
	}
	if got := countJSONRecursive(t, filepath.Join(root, "runtime", "worker-transport-daemon-controls")); got != transportControlsBefore {
		t.Fatalf("policy runner must not create transport daemon controls: before=%d after=%d", transportControlsBefore, got)
	}
	if got := countJSON(t, filepath.Join(root, "runtime", "worker-queue-dispatch-executions")); got != queueDispatchExecutionsBefore {
		t.Fatalf("policy runner must not dispatch queue claims: before=%d after=%d", queueDispatchExecutionsBefore, got)
	}
	if got := countJSON(t, filepath.Join(root, "runtime", "worker-queue-dispatch-results")); got != queueDispatchResultsBefore {
		t.Fatalf("policy runner must not create worker result artifacts: before=%d after=%d", queueDispatchResultsBefore, got)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("policy runner must not execute stored tool command, marker stat err=%v", err)
	}
}

func TestCreateWorkerLifecyclePolicyRunPropagatesRetryHandoffToSupervisor(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Date(2026, 6, 2, 12, 0, 0, 0, time.UTC)
	marker := filepath.Join(t.TempDir(), "policy-run-retry-handoff-ran")
	_, retryDispatch, _ := createRuntimeWorkerDispatchRetryDispatchFixture(t, root, now, RuntimeWorkerToolPlanPayload{
		Action:     "discover_assets",
		Runner:     "command",
		Parser:     "web_api_parser",
		RunCommand: []string{"sh", "-c", "touch " + marker},
	})
	policy, err := CreateWorkerLifecyclePolicy(root, WorkerLifecyclePolicyOptions{
		Now:                          now.Add(5 * time.Minute),
		StaleAfter:                   9 * time.Minute,
		WorkerStaleAfter:             3 * time.Minute,
		PollInterval:                 1500 * time.Millisecond,
		MaxSupervisorRuns:            2,
		IncludeLifecycleMaintenance:  true,
		ExecuteLifecycleRetries:      true,
		DispatchLifecycleRetryClaims: true,
		LifecycleRetryTransportMode:  "file-outbox",
	})
	if err != nil {
		t.Fatalf("create worker lifecycle policy with retry handoff: %v", err)
	}
	if _, err := SelectWorkerLifecyclePolicy(root, filepath.Base(policy.Path), WorkerLifecyclePolicySelectionOptions{
		SelectedBy: "alice",
		SelectedAt: now.Add(6 * time.Minute),
		Reason:     "current retry handoff policy",
	}); err != nil {
		t.Fatalf("select worker lifecycle policy: %v", err)
	}
	requestsBefore, err := ListRuntimeWorkerTransportDispatchRequests(root, "")
	if err != nil {
		t.Fatalf("list transport requests before policy run: %v", err)
	}
	resultsBefore := countJSON(t, filepath.Join(root, "runtime", "worker-queue-dispatch-results"))

	run, err := CreateWorkerLifecyclePolicyRun(root, WorkerLifecyclePolicyRunOptions{
		Execute: true,
		Now:     now.Add(7 * time.Minute),
		Actor:   "runner",
		Reason:  "execute retry handoff policy",
	})
	if err != nil {
		t.Fatalf("execute worker lifecycle policy run with retry handoff: %v", err)
	}

	if !run.DispatchLifecycleRetryClaims || run.LifecycleRetryTransportMode != "file-outbox" {
		t.Fatalf("expected policy run to persist retry handoff intent, got %#v", run)
	}
	if run.SupervisorSummary.RetryDispatchExecutionsCreated != 1 ||
		run.SupervisorSummary.RetryDispatchClaimsCreated != 1 ||
		run.SupervisorSummary.RetryDispatchClaimsDispatched != 1 ||
		run.SupervisorSummary.RetryDispatchTransportRequestsCreated != 1 {
		t.Fatalf("expected policy-run supervisor summary to include retry handoff counts, got %#v", run.SupervisorSummary)
	}
	supervisors, err := ListWorkerLifecycleMaintenanceSupervisors(root)
	if err != nil {
		t.Fatalf("list worker lifecycle maintenance supervisors: %v", err)
	}
	if len(supervisors) != 1 || !supervisors[0].DispatchRetryClaims || supervisors[0].RetryDispatchTransportMode != "file-outbox" {
		t.Fatalf("expected one supervisor with retry handoff intent, got %#v", supervisors)
	}
	executions, err := ListRuntimeWorkerDispatchRetryDispatchExecutions(root)
	if err != nil {
		t.Fatalf("list retry-dispatch executions after policy run: %v", err)
	}
	if len(executions) != 1 || executions[0].RetryDispatchPath != retryDispatch.Path || executions[0].Summary.Dispatched != 1 {
		t.Fatalf("expected one dispatched retry-dispatch execution, got %#v", executions)
	}
	requestsAfter, err := ListRuntimeWorkerTransportDispatchRequests(root, "")
	if err != nil {
		t.Fatalf("list transport requests after policy run: %v", err)
	}
	if len(requestsAfter) != len(requestsBefore)+1 {
		t.Fatalf("expected one pending retry file-outbox request: before=%d after=%d", len(requestsBefore), len(requestsAfter))
	}
	if got := countJSON(t, filepath.Join(root, "runtime", "worker-queue-dispatch-results")); got != resultsBefore {
		t.Fatalf("policy-run retry handoff must not create worker result artifacts: before=%d after=%d", resultsBefore, got)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("policy-run retry handoff must not execute stored tool command, marker stat err=%v", err)
	}
}

func TestCreateWorkerLifecyclePolicyRunExecutesLeaseRecoveryOnlyWhenPolicyEnablesGate(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Date(2026, 6, 2, 0, 50, 0, 0, time.UTC)
	createWorkerLifecycleMaintenanceFixture(t, root, now)
	policy, err := CreateWorkerLifecyclePolicy(root, WorkerLifecyclePolicyOptions{
		Now:                         now.Add(5 * time.Minute),
		StaleAfter:                  5 * time.Minute,
		WorkerStaleAfter:            2 * time.Minute,
		PollInterval:                time.Second,
		MaxSupervisorRuns:           1,
		TransportMode:               "metadata-only",
		IncludeLifecycleMaintenance: true,
		ExecuteLeaseRecovery:        true,
	})
	if err != nil {
		t.Fatalf("create worker lifecycle policy with lease recovery: %v", err)
	}
	if _, err := SelectWorkerLifecyclePolicy(root, filepath.Base(policy.Path), WorkerLifecyclePolicySelectionOptions{
		SelectedBy: "alice",
		SelectedAt: now.Add(6 * time.Minute),
		Reason:     "current lease recovery policy",
	}); err != nil {
		t.Fatalf("select worker lifecycle policy: %v", err)
	}
	requestsBefore, err := ListRuntimeWorkerTransportDispatchRequests(root, "")
	if err != nil {
		t.Fatalf("list transport requests before policy run: %v", err)
	}

	run, err := CreateWorkerLifecyclePolicyRun(root, WorkerLifecyclePolicyRunOptions{
		Execute: true,
		Now:     now.Add(7 * time.Minute),
		Actor:   "runner",
		Reason:  "execute lease recovery policy",
	})
	if err != nil {
		t.Fatalf("execute worker lifecycle policy run with lease recovery: %v", err)
	}

	if !run.ExecuteLeaseRecovery {
		t.Fatalf("expected policy run to persist lease recovery intent, got %#v", run)
	}
	if run.SupervisorSummary.ReclaimExecutionsCreated != 1 || run.SupervisorSummary.ReconciliationExecutionsCreated != 1 {
		t.Fatalf("expected policy-run supervisor summary to include lease recovery execution counts, got %#v", run.SupervisorSummary)
	}
	if countJSON(t, filepath.Join(root, "runtime", "worker-lease-reclaim-executions")) != 1 {
		t.Fatalf("policy-run lease recovery should create one reclaim execution")
	}
	if countJSON(t, filepath.Join(root, "runtime", "worker-lease-result-reconciliation-executions")) != 1 {
		t.Fatalf("policy-run lease recovery should create one reconciliation execution")
	}
	requestsAfter, err := ListRuntimeWorkerTransportDispatchRequests(root, "")
	if err != nil {
		t.Fatalf("list transport requests after policy run: %v", err)
	}
	if len(requestsAfter) != len(requestsBefore) {
		t.Fatalf("policy-run lease recovery must not create transport requests: before=%d after=%d", len(requestsBefore), len(requestsAfter))
	}
}

func TestCreateWorkerLifecyclePolicyRunFailsClosedForMissingClearedOrEmptySelection(t *testing.T) {
	now := time.Date(2026, 6, 1, 20, 0, 0, 0, time.UTC)
	tests := []struct {
		name  string
		setup func(t *testing.T, root string)
		want  string
	}{
		{
			name: "missing selection",
			setup: func(t *testing.T, root string) {
				t.Helper()
				if err := InitWorkspace(root); err != nil {
					t.Fatalf("init workspace: %v", err)
				}
			},
			want: "current worker lifecycle policy selection is required",
		},
		{
			name: "cleared selection",
			setup: func(t *testing.T, root string) {
				t.Helper()
				if err := InitWorkspace(root); err != nil {
					t.Fatalf("init workspace: %v", err)
				}
				if _, err := ClearWorkerLifecyclePolicySelection(root, WorkerLifecyclePolicySelectionClearOptions{
					ClearedBy: "alice",
					ClearedAt: now,
					Reason:    "disabled",
				}); err != nil {
					t.Fatalf("clear policy selection: %v", err)
				}
			},
			want: "current worker lifecycle policy selection is required",
		},
		{
			name: "current selection without policy path",
			setup: func(t *testing.T, root string) {
				t.Helper()
				selection := WorkerLifecyclePolicySelection{
					Version: 1,
					Path:    workerLifecyclePolicySelectionPath(root),
					Status:  "current",
					Mode:    "metadata-only",
				}
				if err := writeWorkerLifecyclePolicySelection(selection); err != nil {
					t.Fatalf("write empty current selection: %v", err)
				}
			},
			want: "current worker lifecycle policy selection requires policy path",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "workspace")
			tc.setup(t, root)
			_, err := CreateWorkerLifecyclePolicyRun(root, WorkerLifecyclePolicyRunOptions{
				Now:    now.Add(time.Minute),
				Actor:  "runner",
				Reason: "must fail closed",
			})
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected fail-closed selection error %q, got %v", tc.want, err)
			}
			if countJSON(t, filepath.Join(root, "runtime", "worker-lifecycle-policy-runs")) != 0 {
				t.Fatalf("fail-closed policy runner must not write run artifacts")
			}
		})
	}
}

func TestCreateWorkerLifecyclePolicyRunFailsClosedForInvalidSelectedPolicyArtifact(t *testing.T) {
	now := time.Date(2026, 6, 1, 20, 15, 0, 0, time.UTC)
	tests := []struct {
		name   string
		mutate func(policy *WorkerLifecyclePolicy)
		want   string
	}{
		{
			name: "non-positive stale after",
			mutate: func(policy *WorkerLifecyclePolicy) {
				policy.StaleAfterSeconds = 0
			},
			want: "stale after must be positive",
		},
		{
			name: "negative poll interval",
			mutate: func(policy *WorkerLifecyclePolicy) {
				policy.PollIntervalSeconds = -1
				policy.PollIntervalMilliseconds = -1
			},
			want: "poll interval cannot be negative",
		},
		{
			name: "non-positive max supervisor runs",
			mutate: func(policy *WorkerLifecyclePolicy) {
				policy.MaxSupervisorRuns = 0
			},
			want: "max supervisor runs must be positive",
		},
		{
			name: "unsupported transport mode",
			mutate: func(policy *WorkerLifecyclePolicy) {
				policy.TransportMode = "smtp"
			},
			want: "unsupported worker queue dispatch transport",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "workspace")
			createWorkerLifecycleMaintenanceFixture(t, root, now)
			policy, err := CreateWorkerLifecyclePolicy(root, WorkerLifecyclePolicyOptions{
				Now:                         now.Add(5 * time.Minute),
				StaleAfter:                  6 * time.Minute,
				WorkerStaleAfter:            3 * time.Minute,
				PollInterval:                time.Second,
				MaxSupervisorRuns:           2,
				TransportMode:               "metadata-only",
				IncludeLifecycleMaintenance: true,
				ExecuteLifecycleRetries:     true,
			})
			if err != nil {
				t.Fatalf("create worker lifecycle policy: %v", err)
			}
			if _, err := SelectWorkerLifecyclePolicy(root, filepath.Base(policy.Path), WorkerLifecyclePolicySelectionOptions{
				SelectedBy: "alice",
				SelectedAt: now.Add(6 * time.Minute),
				Reason:     "current invalid-bound fixture",
			}); err != nil {
				t.Fatalf("select worker lifecycle policy: %v", err)
			}
			tc.mutate(&policy)
			if err := writeWorkerLifecyclePolicy(root, &policy); err != nil {
				t.Fatalf("corrupt selected worker lifecycle policy artifact: %v", err)
			}
			sideEffectsBefore := snapshotWorkerLifecyclePolicySideEffects(t, root)

			_, err = CreateWorkerLifecyclePolicyRun(root, WorkerLifecyclePolicyRunOptions{
				Execute: true,
				Now:     now.Add(7 * time.Minute),
				Actor:   "runner",
				Reason:  "must fail closed",
			})
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected invalid selected policy error %q, got %v", tc.want, err)
			}
			if countJSON(t, filepath.Join(root, "runtime", "worker-lifecycle-policy-runs")) != 0 {
				t.Fatalf("invalid selected policy must not write policy-run artifacts")
			}
			assertWorkerLifecyclePolicySideEffectsUnchanged(t, root, sideEffectsBefore)
		})
	}
}

func TestCreateWorkerLifecyclePolicyRunAllowsZeroPollIntervalAsDisabledScheduler(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Date(2026, 6, 1, 20, 20, 0, 0, time.UTC)
	createWorkerLifecycleMaintenanceFixture(t, root, now)
	policy, err := CreateWorkerLifecyclePolicy(root, WorkerLifecyclePolicyOptions{
		Now:                         now.Add(5 * time.Minute),
		StaleAfter:                  6 * time.Minute,
		WorkerStaleAfter:            3 * time.Minute,
		PollInterval:                0,
		MaxSupervisorRuns:           1,
		TransportMode:               "metadata-only",
		IncludeLifecycleMaintenance: true,
	})
	if err != nil {
		t.Fatalf("create zero-poll worker lifecycle policy: %v", err)
	}
	if _, err := SelectWorkerLifecyclePolicy(root, filepath.Base(policy.Path), WorkerLifecyclePolicySelectionOptions{
		SelectedBy: "alice",
		SelectedAt: now.Add(6 * time.Minute),
		Reason:     "polling disabled explicitly",
	}); err != nil {
		t.Fatalf("select zero-poll worker lifecycle policy: %v", err)
	}

	run, err := CreateWorkerLifecyclePolicyRun(root, WorkerLifecyclePolicyRunOptions{
		Now:    now.Add(7 * time.Minute),
		Actor:  "runner",
		Reason: "metadata-only zero poll",
	})
	if err != nil {
		t.Fatalf("zero poll interval should disable scheduling without rejecting policy run: %v", err)
	}
	if run.PollIntervalMilliseconds != 0 || run.Status != "dry-run" || run.Mode != "metadata-only" {
		t.Fatalf("unexpected zero-poll policy run: %#v", run)
	}
}

func TestCreateWorkerLifecyclePolicyRunExecuteSkipsWhenLifecycleMaintenanceDisabled(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Date(2026, 6, 1, 20, 30, 0, 0, time.UTC)
	createWorkerLifecycleMaintenanceFixture(t, root, now)
	policy, err := CreateWorkerLifecyclePolicy(root, WorkerLifecyclePolicyOptions{
		Now:                         now.Add(5 * time.Minute),
		StaleAfter:                  8 * time.Minute,
		WorkerStaleAfter:            3 * time.Minute,
		PollInterval:                time.Second,
		MaxSupervisorRuns:           4,
		TransportMode:               "metadata-only",
		IncludeLifecycleMaintenance: false,
	})
	if err != nil {
		t.Fatalf("create worker lifecycle policy: %v", err)
	}
	if _, err := SelectWorkerLifecyclePolicy(root, filepath.Base(policy.Path), WorkerLifecyclePolicySelectionOptions{
		SelectedBy: "alice",
		SelectedAt: now.Add(6 * time.Minute),
		Reason:     "disabled maintenance policy",
	}); err != nil {
		t.Fatalf("select worker lifecycle policy: %v", err)
	}
	sideEffectsBefore := snapshotWorkerLifecyclePolicySideEffects(t, root)

	run, err := CreateWorkerLifecyclePolicyRun(root, WorkerLifecyclePolicyRunOptions{
		Execute: true,
		Now:     now.Add(7 * time.Minute),
		Actor:   "runner",
		Reason:  "execute selected policy",
	})
	if err != nil {
		t.Fatalf("execute worker lifecycle policy run with maintenance disabled: %v", err)
	}
	if run.Status != "completed" || run.Mode != "metadata-only" || run.SkipReason != "lifecycle-maintenance-disabled" {
		t.Fatalf("expected completed skipped run, got %#v", run)
	}
	if run.SupervisorPath != "" || run.SupervisorSummary.Runs != 0 {
		t.Fatalf("maintenance-disabled policy run must not link supervisor state, got %#v", run)
	}
	if run.IncludeLifecycleMaintenance || run.MaxRuns != 4 || run.PolicyPath != policy.Path {
		t.Fatalf("expected selected policy values to be persisted, got %#v", run)
	}
	if countJSON(t, filepath.Join(root, "runtime", "worker-lifecycle-policy-runs")) != 1 {
		t.Fatalf("expected one policy run artifact")
	}
	if countJSON(t, filepath.Join(root, "runtime", "worker-lifecycle-maintenance-supervisor-runs")) != 0 {
		t.Fatalf("maintenance-disabled policy run must not create supervisor artifacts")
	}
	assertWorkerLifecyclePolicySideEffectsUnchanged(t, root, sideEffectsBefore)
}

func assertOnlyWorkerLifecyclePolicyRunFileChanged(t *testing.T, root string, before map[string]string, runPath string) {
	t.Helper()
	after := snapshotWorkerLifecyclePolicySelectionBoundaryFiles(t, root)
	changed := []string{}
	for path, beforeContents := range before {
		afterContents, ok := after[path]
		if !ok || afterContents != beforeContents {
			changed = append(changed, path)
		}
	}
	for path := range after {
		if _, ok := before[path]; !ok {
			changed = append(changed, path)
		}
	}
	sort.Strings(changed)
	rel, err := filepath.Rel(root, runPath)
	if err != nil {
		t.Fatalf("resolve run artifact relative path: %v", err)
	}
	want := filepath.ToSlash(rel)
	if len(changed) != 1 || changed[0] != want {
		t.Fatalf("policy runner must only change %s, got changed files %#v", want, changed)
	}
}
