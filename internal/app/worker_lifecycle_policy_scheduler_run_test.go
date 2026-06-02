package app

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

func TestCreateWorkerLifecyclePolicySchedulerRunDryRunWritesOnlySchedulerArtifact(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Date(2026, 6, 1, 21, 0, 0, 0, time.UTC)
	createWorkerLifecycleMaintenanceFixture(t, root, now)
	marker := filepath.Join(t.TempDir(), "scheduler-dry-run-tool-side-effect")
	createFileOutboxRuntimeWorkerQueueDispatchWithToolPlan(t, root, now.Add(30*time.Second), RuntimeWorkerToolPlanPayload{
		Action:     "discover_assets",
		Runner:     "command",
		Parser:     "web_api_parser",
		RunCommand: []string{"sh", "-c", "touch " + marker},
	})
	policy := createSelectedWorkerLifecyclePolicyForScheduler(t, root, now, 750*time.Millisecond, true)
	sideEffectsBefore := snapshotWorkerLifecyclePolicySideEffects(t, root)
	filesBefore := snapshotWorkerLifecyclePolicySelectionBoundaryFiles(t, root)

	run, err := CreateWorkerLifecyclePolicySchedulerRun(root, WorkerLifecyclePolicySchedulerRunOptions{
		Now:     now.Add(10 * time.Minute),
		Actor:   "scheduler",
		Reason:  "review bounded policy schedule",
		MaxRuns: 2,
	})
	if err != nil {
		t.Fatalf("create dry-run worker lifecycle policy scheduler run: %v", err)
	}
	if run.Status != "dry-run" || run.Mode != "metadata-only" {
		t.Fatalf("unexpected scheduler run metadata: %#v", run)
	}
	if run.PolicyPath != policy.Path ||
		run.SelectionPath != filepath.Join(root, "runtime", "worker-lifecycle-policy-selection.json") ||
		run.PolicyBasename != filepath.Base(policy.Path) ||
		run.PollIntervalMilliseconds != 750 ||
		run.MaxRunsRequested != 2 ||
		run.PolicyMaxSupervisorRuns != 2 ||
		run.PolicyRunsCreated != 0 ||
		run.SupervisorsCreated != 0 ||
		run.Actor != "scheduler" ||
		run.CreatedAt != "2026-06-01T21:10:00Z" ||
		run.Reason != "review bounded policy schedule" {
		t.Fatalf("unexpected scheduler run fields: %#v", run)
	}
	if len(run.PolicyRunPaths) != 0 || run.SkipReason != "" {
		t.Fatalf("dry-run scheduler must not link policy-run children, got %#v", run)
	}
	if !strings.Contains(run.Path, filepath.Join("runtime", "worker-lifecycle-policy-scheduler-runs")) {
		t.Fatalf("expected scheduler run artifact path, got %s", run.Path)
	}
	assertOnlyWorkerLifecyclePolicySchedulerRunFileChanged(t, root, filesBefore, run.Path)
	assertWorkerLifecyclePolicySideEffectsUnchanged(t, root, sideEffectsBefore)
	if countJSON(t, filepath.Join(root, "runtime", "worker-lifecycle-policy-runs")) != 0 {
		t.Fatalf("dry-run scheduler must not create policy-run artifacts")
	}
	if countJSON(t, filepath.Join(root, "runtime", "worker-lifecycle-maintenance-supervisor-runs")) != 0 {
		t.Fatalf("dry-run scheduler must not create maintenance supervisor artifacts")
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("dry-run scheduler must not execute stored tool command, marker stat err=%v", err)
	}

	read, err := ReadWorkerLifecyclePolicySchedulerRun(root, filepath.Base(run.Path))
	if err != nil {
		t.Fatalf("read worker lifecycle policy scheduler run: %v", err)
	}
	if read.Path != run.Path || read.PolicyPath != policy.Path {
		t.Fatalf("unexpected read scheduler run: %#v", read)
	}
	runs, err := ListWorkerLifecyclePolicySchedulerRuns(root)
	if err != nil {
		t.Fatalf("list worker lifecycle policy scheduler runs: %v", err)
	}
	if len(runs) != 1 || runs[0].Path != run.Path {
		t.Fatalf("expected one listed scheduler run, got %#v", runs)
	}
	if _, err := ReadWorkerLifecyclePolicySchedulerRun(root, filepath.Join(t.TempDir(), "outside.json")); err == nil || !strings.Contains(err.Error(), "worker lifecycle policy scheduler run path must stay under") {
		t.Fatalf("expected scheduler run path confinement error, got %v", err)
	}
}

func TestCreateWorkerLifecyclePolicySchedulerRunExecuteCreatesBoundedPolicyRuns(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Date(2026, 6, 1, 21, 30, 0, 0, time.UTC)
	createWorkerLifecycleMaintenanceFixture(t, root, now)
	marker := filepath.Join(t.TempDir(), "scheduler-execute-tool-side-effect")
	createFileOutboxRuntimeWorkerQueueDispatchWithToolPlan(t, root, now.Add(30*time.Second), RuntimeWorkerToolPlanPayload{
		Action:     "discover_assets",
		Runner:     "command",
		Parser:     "web_api_parser",
		RunCommand: []string{"sh", "-c", "touch " + marker},
	})
	createSelectedWorkerLifecyclePolicyForScheduler(t, root, now, time.Second, true)

	run, err := CreateWorkerLifecyclePolicySchedulerRun(root, WorkerLifecyclePolicySchedulerRunOptions{
		Execute: true,
		Now:     now.Add(10 * time.Minute),
		Actor:   "scheduler",
		Reason:  "execute bounded policy schedule",
		MaxRuns: 2,
	})
	if err != nil {
		t.Fatalf("execute worker lifecycle policy scheduler run: %v", err)
	}
	if run.Status != "completed" || run.Mode != "metadata-only" {
		t.Fatalf("expected completed scheduler run, got %#v", run)
	}
	if run.PolicyRunsCreated != 2 || len(run.PolicyRunPaths) != 2 || run.SupervisorsCreated != 2 || !run.SupervisorCreated {
		t.Fatalf("expected two child policy runs and supervisors, got %#v", run)
	}
	if run.CompletedAt != "2026-06-01T21:40:02Z" {
		t.Fatalf("expected deterministic completed_at, got %#v", run)
	}
	if countJSON(t, filepath.Join(root, "runtime", "worker-lifecycle-policy-scheduler-runs")) != 1 {
		t.Fatalf("expected one scheduler run artifact")
	}
	if countJSON(t, filepath.Join(root, "runtime", "worker-lifecycle-policy-runs")) != 2 {
		t.Fatalf("expected two policy-run artifacts")
	}
	if countJSON(t, filepath.Join(root, "runtime", "worker-lifecycle-maintenance-supervisor-runs")) != 2 {
		t.Fatalf("expected each policy run to create one bounded supervisor")
	}
	for _, policyRunPath := range run.PolicyRunPaths {
		policyRun, err := ReadWorkerLifecyclePolicyRun(root, filepath.Base(policyRunPath))
		if err != nil {
			t.Fatalf("read child policy run: %v", err)
		}
		if policyRun.Status != "completed" || policyRun.SupervisorPath == "" || policyRun.MaxRuns != 2 {
			t.Fatalf("unexpected child policy run: %#v", policyRun)
		}
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("scheduler must not execute stored tool command, marker stat err=%v", err)
	}
}

func TestCreateWorkerLifecyclePolicySchedulerRunExecuteHonorsZeroPollIntervalAsClosedSwitch(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Date(2026, 6, 1, 22, 0, 0, 0, time.UTC)
	createWorkerLifecycleMaintenanceFixture(t, root, now)
	createSelectedWorkerLifecyclePolicyForScheduler(t, root, now, 0, true)
	sideEffectsBefore := snapshotWorkerLifecyclePolicySideEffects(t, root)

	run, err := CreateWorkerLifecyclePolicySchedulerRun(root, WorkerLifecyclePolicySchedulerRunOptions{
		Execute: true,
		Now:     now.Add(10 * time.Minute),
		Actor:   "scheduler",
		Reason:  "closed schedule",
		MaxRuns: 3,
	})
	if err != nil {
		t.Fatalf("execute closed worker lifecycle policy scheduler run: %v", err)
	}
	if run.Status != "completed" || run.SkipReason != "lifecycle-policy-scheduler-disabled" {
		t.Fatalf("expected closed scheduler skip, got %#v", run)
	}
	if run.PolicyRunsCreated != 0 || len(run.PolicyRunPaths) != 0 || run.SupervisorsCreated != 0 || run.SupervisorCreated {
		t.Fatalf("closed scheduler must not create child policy runs, got %#v", run)
	}
	if countJSON(t, filepath.Join(root, "runtime", "worker-lifecycle-policy-runs")) != 0 {
		t.Fatalf("closed scheduler must not create policy-run artifacts")
	}
	if countJSON(t, filepath.Join(root, "runtime", "worker-lifecycle-maintenance-supervisor-runs")) != 0 {
		t.Fatalf("closed scheduler must not create maintenance supervisor artifacts")
	}
	assertWorkerLifecyclePolicySideEffectsUnchanged(t, root, sideEffectsBefore)
}

func TestCreateWorkerLifecyclePolicySchedulerRunFailsClosedBeforeWrites(t *testing.T) {
	now := time.Date(2026, 6, 1, 22, 30, 0, 0, time.UTC)
	t.Run("execute max runs validates before selection lookup", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "workspace")
		if err := InitWorkspace(root); err != nil {
			t.Fatalf("init workspace: %v", err)
		}
		_, err := CreateWorkerLifecyclePolicySchedulerRun(root, WorkerLifecyclePolicySchedulerRunOptions{
			Execute: true,
			Now:     now,
			MaxRuns: 0,
		})
		if err == nil || !strings.Contains(err.Error(), "worker lifecycle policy scheduler run max runs must be positive") {
			t.Fatalf("expected max runs error before selection lookup, got %v", err)
		}
		if countJSON(t, filepath.Join(root, "runtime", "worker-lifecycle-policy-scheduler-runs")) != 0 {
			t.Fatalf("invalid max runs must not write scheduler artifacts")
		}
	})
	t.Run("dry run negative max runs validates before selection lookup", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "workspace")
		if err := InitWorkspace(root); err != nil {
			t.Fatalf("init workspace: %v", err)
		}
		_, err := CreateWorkerLifecyclePolicySchedulerRun(root, WorkerLifecyclePolicySchedulerRunOptions{
			Now:     now,
			MaxRuns: -1,
		})
		if err == nil || !strings.Contains(err.Error(), "worker lifecycle policy scheduler run max runs cannot be negative") {
			t.Fatalf("expected negative max runs error before selection lookup, got %v", err)
		}
		if countJSON(t, filepath.Join(root, "runtime", "worker-lifecycle-policy-scheduler-runs")) != 0 {
			t.Fatalf("negative max runs must not write scheduler artifacts")
		}
	})
	t.Run("missing selection", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "workspace")
		if err := InitWorkspace(root); err != nil {
			t.Fatalf("init workspace: %v", err)
		}
		_, err := CreateWorkerLifecyclePolicySchedulerRun(root, WorkerLifecyclePolicySchedulerRunOptions{
			Execute: true,
			Now:     now,
			MaxRuns: 1,
		})
		if err == nil || !strings.Contains(err.Error(), "current worker lifecycle policy selection is required") {
			t.Fatalf("expected current selection error, got %v", err)
		}
		if countJSON(t, filepath.Join(root, "runtime", "worker-lifecycle-policy-scheduler-runs")) != 0 {
			t.Fatalf("missing selection must not write scheduler artifacts")
		}
	})
	t.Run("cleared selection", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "workspace")
		if err := InitWorkspace(root); err != nil {
			t.Fatalf("init workspace: %v", err)
		}
		if _, err := ClearWorkerLifecyclePolicySelection(root, WorkerLifecyclePolicySelectionClearOptions{
			ClearedBy: "alice",
			ClearedAt: now,
			Reason:    "disable scheduler",
		}); err != nil {
			t.Fatalf("clear worker lifecycle policy selection: %v", err)
		}
		_, err := CreateWorkerLifecyclePolicySchedulerRun(root, WorkerLifecyclePolicySchedulerRunOptions{
			Execute: true,
			Now:     now,
			MaxRuns: 1,
		})
		if err == nil || !strings.Contains(err.Error(), "current worker lifecycle policy selection is required") {
			t.Fatalf("expected current selection error, got %v", err)
		}
		if countJSON(t, filepath.Join(root, "runtime", "worker-lifecycle-policy-scheduler-runs")) != 0 {
			t.Fatalf("cleared selection must not write scheduler artifacts")
		}
	})
	t.Run("empty current selection", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "workspace")
		if err := writeWorkerLifecyclePolicySelection(WorkerLifecyclePolicySelection{
			Version: 1,
			Path:    workerLifecyclePolicySelectionPath(root),
			Status:  "current",
			Mode:    "metadata-only",
		}); err != nil {
			t.Fatalf("write empty current selection: %v", err)
		}
		_, err := CreateWorkerLifecyclePolicySchedulerRun(root, WorkerLifecyclePolicySchedulerRunOptions{
			Execute: true,
			Now:     now,
			MaxRuns: 1,
		})
		if err == nil || !strings.Contains(err.Error(), "current worker lifecycle policy selection requires policy path") {
			t.Fatalf("expected empty current selection error, got %v", err)
		}
		if countJSON(t, filepath.Join(root, "runtime", "worker-lifecycle-policy-scheduler-runs")) != 0 {
			t.Fatalf("empty current selection must not write scheduler artifacts")
		}
	})
	t.Run("execute max runs", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "workspace")
		createWorkerLifecycleMaintenanceFixture(t, root, now)
		createSelectedWorkerLifecyclePolicyForScheduler(t, root, now, time.Second, true)
		_, err := CreateWorkerLifecyclePolicySchedulerRun(root, WorkerLifecyclePolicySchedulerRunOptions{
			Execute: true,
			Now:     now,
			MaxRuns: 0,
		})
		if err == nil || !strings.Contains(err.Error(), "worker lifecycle policy scheduler run max runs must be positive") {
			t.Fatalf("expected max runs error, got %v", err)
		}
		if countJSON(t, filepath.Join(root, "runtime", "worker-lifecycle-policy-scheduler-runs")) != 0 {
			t.Fatalf("invalid max runs must not write scheduler artifacts")
		}
	})
	t.Run("corrupt selected policy", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "workspace")
		createWorkerLifecycleMaintenanceFixture(t, root, now)
		policy := createSelectedWorkerLifecyclePolicyForScheduler(t, root, now, time.Second, true)
		policy.PollIntervalMilliseconds = -1
		if err := writeWorkerLifecyclePolicy(root, &policy); err != nil {
			t.Fatalf("corrupt selected policy: %v", err)
		}
		_, err := CreateWorkerLifecyclePolicySchedulerRun(root, WorkerLifecyclePolicySchedulerRunOptions{
			Execute: true,
			Now:     now,
			MaxRuns: 1,
		})
		if err == nil || !strings.Contains(err.Error(), "poll interval cannot be negative") {
			t.Fatalf("expected invalid selected policy error, got %v", err)
		}
		if countJSON(t, filepath.Join(root, "runtime", "worker-lifecycle-policy-scheduler-runs")) != 0 {
			t.Fatalf("invalid selected policy must not write scheduler artifacts")
		}
	})
	t.Run("invalid selected policy bounds", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "workspace")
		createWorkerLifecycleMaintenanceFixture(t, root, now)
		policy := createSelectedWorkerLifecyclePolicyForScheduler(t, root, now, time.Second, true)
		policy.MaxSupervisorRuns = 0
		if err := writeWorkerLifecyclePolicy(root, &policy); err != nil {
			t.Fatalf("corrupt selected policy: %v", err)
		}
		_, err := CreateWorkerLifecyclePolicySchedulerRun(root, WorkerLifecyclePolicySchedulerRunOptions{
			Execute: true,
			Now:     now,
			MaxRuns: 1,
		})
		if err == nil || !strings.Contains(err.Error(), "max supervisor runs must be positive") {
			t.Fatalf("expected invalid selected policy bounds error, got %v", err)
		}
		if countJSON(t, filepath.Join(root, "runtime", "worker-lifecycle-policy-scheduler-runs")) != 0 {
			t.Fatalf("invalid selected policy bounds must not write scheduler artifacts")
		}
	})
}

func createSelectedWorkerLifecyclePolicyForScheduler(t *testing.T, root string, now time.Time, pollInterval time.Duration, includeMaintenance bool) WorkerLifecyclePolicy {
	t.Helper()
	policy, err := CreateWorkerLifecyclePolicy(root, WorkerLifecyclePolicyOptions{
		Now:                         now.Add(5 * time.Minute),
		StaleAfter:                  5 * time.Minute,
		WorkerStaleAfter:            2 * time.Minute,
		PollInterval:                pollInterval,
		MaxSupervisorRuns:           2,
		TransportMode:               "metadata-only",
		IncludeLifecycleMaintenance: includeMaintenance,
	})
	if err != nil {
		t.Fatalf("create worker lifecycle policy: %v", err)
	}
	if _, err := SelectWorkerLifecyclePolicy(root, filepath.Base(policy.Path), WorkerLifecyclePolicySelectionOptions{
		SelectedBy: "alice",
		SelectedAt: now.Add(6 * time.Minute),
		Reason:     "selected scheduler policy",
	}); err != nil {
		t.Fatalf("select worker lifecycle policy: %v", err)
	}
	return policy
}

func assertOnlyWorkerLifecyclePolicySchedulerRunFileChanged(t *testing.T, root string, before map[string]string, runPath string) {
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
		t.Fatalf("resolve scheduler run artifact relative path: %v", err)
	}
	want := filepath.ToSlash(rel)
	if len(changed) != 1 || changed[0] != want {
		t.Fatalf("scheduler run must only change %s, got changed files %#v", want, changed)
	}
}
