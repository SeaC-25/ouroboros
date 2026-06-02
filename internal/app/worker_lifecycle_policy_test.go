package app

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

func TestCreateWorkerLifecyclePolicyWritesMetadataOnlyStrategyWithoutChildArtifacts(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Date(2026, 6, 1, 16, 30, 0, 0, time.UTC)
	fixture := createWorkerLifecycleMaintenanceFixture(t, root, now)
	marker := filepath.Join(t.TempDir(), "worker-lifecycle-policy-tool-side-effect")
	createFileOutboxRuntimeWorkerQueueDispatchWithToolPlan(t, root, now.Add(30*time.Second), RuntimeWorkerToolPlanPayload{
		Action:     "discover_assets",
		Runner:     "command",
		Parser:     "web_api_parser",
		RunCommand: []string{"sh", "-c", "touch " + marker},
	})
	sideEffectsBefore := snapshotWorkerLifecyclePolicySideEffects(t, root)

	policy, err := CreateWorkerLifecyclePolicy(root, WorkerLifecyclePolicyOptions{
		Now:                         now.Add(5 * time.Minute),
		StaleAfter:                  5 * time.Minute,
		WorkerStaleAfter:            2 * time.Minute,
		PollInterval:                30 * time.Second,
		MaxSupervisorRuns:           3,
		TransportMode:               "metadata-only",
		IncludeLifecycleMaintenance: true,
		ExecuteLifecycleRetries:     true,
		ExecuteLeaseRecovery:        true,
	})
	if err != nil {
		t.Fatalf("create worker lifecycle policy: %v", err)
	}
	if policy.Status != "dry-run" || policy.Mode != "metadata-only" {
		t.Fatalf("unexpected policy metadata: %#v", policy)
	}
	if policy.MaxSupervisorRuns != 3 || policy.StaleAfterSeconds != 300 || policy.WorkerStaleAfterSeconds != 120 || policy.PollIntervalSeconds != 30 {
		t.Fatalf("unexpected policy bounds: %#v", policy)
	}
	if policy.TransportMode != "metadata-only" || !policy.IncludeLifecycleMaintenance || !policy.ExecuteLifecycleRetries || !policy.ExecuteLeaseRecovery {
		t.Fatalf("unexpected policy execution defaults: %#v", policy)
	}
	if policy.Summary.WorkerRegistrations == 0 || policy.Summary.StaleActiveLeases == 0 {
		t.Fatalf("expected policy to snapshot lifecycle audit summary, got %#v", policy.Summary)
	}
	if policy.Summary.StaleWorkerRegistrations != 1 {
		t.Fatalf("expected policy summary to use worker stale bound, got %#v", policy.Summary)
	}
	if !strings.Contains(policy.Path, filepath.Join("runtime", "worker-lifecycle-policies")) {
		t.Fatalf("expected policy artifact path, got %s", policy.Path)
	}
	if countJSON(t, filepath.Join(root, "runtime", "worker-lifecycle-maintenance-supervisor-runs")) != 0 {
		t.Fatalf("policy must not create maintenance supervisor runs")
	}
	if countJSON(t, filepath.Join(root, "runtime", "worker-orchestration-supervisor-runs")) != 0 {
		t.Fatalf("policy must not create orchestration supervisor runs")
	}
	if countJSON(t, filepath.Join(root, "runtime", "worker-lifecycle-maintenance")) != 0 {
		t.Fatalf("policy must not create maintenance artifacts")
	}
	assertWorkerLifecyclePolicySideEffectsUnchanged(t, root, sideEffectsBefore)
	assertWorkerLifecycleMaintenanceFixtureUnmutated(t, root, fixture)
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("policy must not execute stored tool command, marker stat err=%v", err)
	}

	read, err := ReadWorkerLifecyclePolicy(root, filepath.Base(policy.Path))
	if err != nil {
		t.Fatalf("read worker lifecycle policy: %v", err)
	}
	if read.Path != policy.Path || read.Summary.StaleActiveLeases != policy.Summary.StaleActiveLeases {
		t.Fatalf("unexpected persisted policy: %#v", read)
	}
	policies, err := ListWorkerLifecyclePolicies(root)
	if err != nil {
		t.Fatalf("list worker lifecycle policies: %v", err)
	}
	if len(policies) != 1 || policies[0].Path != policy.Path {
		t.Fatalf("expected one persisted policy, got %#v", policies)
	}
}

func TestCreateWorkerLifecyclePolicyRejectsLeaseRecoveryWithoutLifecycleMaintenance(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Date(2026, 6, 2, 0, 40, 0, 0, time.UTC)
	createWorkerLifecycleMaintenanceFixture(t, root, now)

	_, err := CreateWorkerLifecyclePolicy(root, WorkerLifecyclePolicyOptions{
		Now:                  now.Add(5 * time.Minute),
		StaleAfter:           5 * time.Minute,
		WorkerStaleAfter:     2 * time.Minute,
		PollInterval:         30 * time.Second,
		MaxSupervisorRuns:    1,
		TransportMode:        "metadata-only",
		ExecuteLeaseRecovery: true,
	})
	if err == nil || !strings.Contains(err.Error(), "worker lifecycle policy lease recovery execution requires lifecycle maintenance") {
		t.Fatalf("expected lease recovery lifecycle-maintenance gate error, got %v", err)
	}
	if countJSON(t, filepath.Join(root, "runtime", "worker-lifecycle-policies")) != 0 {
		t.Fatalf("invalid lease recovery policy must not write policy artifacts")
	}
}

func TestSelectWorkerLifecyclePolicyWritesCloseableMetadataOnlyMarker(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Date(2026, 6, 1, 18, 0, 0, 0, time.UTC)
	createWorkerLifecycleMaintenanceFixture(t, root, now)
	first, err := CreateWorkerLifecyclePolicy(root, WorkerLifecyclePolicyOptions{
		Now:               now.Add(5 * time.Minute),
		StaleAfter:        5 * time.Minute,
		WorkerStaleAfter:  2 * time.Minute,
		PollInterval:      30 * time.Second,
		MaxSupervisorRuns: 2,
		TransportMode:     "metadata-only",
	})
	if err != nil {
		t.Fatalf("create first worker lifecycle policy: %v", err)
	}
	second, err := CreateWorkerLifecyclePolicy(root, WorkerLifecyclePolicyOptions{
		Now:                         now.Add(6 * time.Minute),
		StaleAfter:                  10 * time.Minute,
		WorkerStaleAfter:            3 * time.Minute,
		PollInterval:                45 * time.Second,
		MaxSupervisorRuns:           4,
		TransportMode:               "file-outbox",
		IncludeLifecycleMaintenance: true,
	})
	if err != nil {
		t.Fatalf("create second worker lifecycle policy: %v", err)
	}
	sideEffectsBefore := snapshotWorkerLifecyclePolicySideEffects(t, root)
	filesBeforeFirstSelection := snapshotWorkerLifecyclePolicySelectionBoundaryFiles(t, root)

	firstSelection, err := SelectWorkerLifecyclePolicy(root, filepath.Base(first.Path), WorkerLifecyclePolicySelectionOptions{
		SelectedBy: "alice",
		SelectedAt: now.Add(7 * time.Minute),
		Reason:     "baseline lifecycle policy",
	})
	if err != nil {
		t.Fatalf("select first worker lifecycle policy: %v", err)
	}
	if firstSelection.Path != filepath.Join(root, "runtime", "worker-lifecycle-policy-selection.json") {
		t.Fatalf("unexpected selection marker path: %#v", firstSelection)
	}
	if firstSelection.Status != "current" || firstSelection.Mode != "metadata-only" ||
		firstSelection.PolicyPath != first.Path ||
		firstSelection.PolicyBasename != filepath.Base(first.Path) ||
		firstSelection.StaleAfterSeconds != first.StaleAfterSeconds ||
		firstSelection.WorkerStaleAfterSeconds != first.WorkerStaleAfterSeconds ||
		firstSelection.PollIntervalMilliseconds != first.PollIntervalMilliseconds ||
		firstSelection.MaxSupervisorRuns != first.MaxSupervisorRuns ||
		firstSelection.TransportMode != first.TransportMode ||
		firstSelection.Summary.WorkerRegistrations != first.Summary.WorkerRegistrations ||
		firstSelection.SelectedBy != "alice" ||
		firstSelection.SelectedAt != "2026-06-01T18:07:00Z" ||
		firstSelection.Reason != "baseline lifecycle policy" ||
		len(firstSelection.SupersededPolicyPaths) != 0 {
		t.Fatalf("unexpected first policy selection: %#v", firstSelection)
	}
	assertOnlyWorkerLifecyclePolicySelectionMarkerChanged(t, root, filesBeforeFirstSelection)
	filesBeforeSecondSelection := snapshotWorkerLifecyclePolicySelectionBoundaryFiles(t, root)

	secondSelection, err := SelectWorkerLifecyclePolicy(root, filepath.Base(second.Path), WorkerLifecyclePolicySelectionOptions{
		SelectedBy: "bob",
		SelectedAt: now.Add(8 * time.Minute),
		Reason:     "prefer file outbox policy",
	})
	if err != nil {
		t.Fatalf("select second worker lifecycle policy: %v", err)
	}
	if secondSelection.Status != "current" || secondSelection.Mode != "metadata-only" ||
		secondSelection.PolicyPath != second.Path ||
		secondSelection.PolicyBasename != filepath.Base(second.Path) ||
		secondSelection.StaleAfterSeconds != second.StaleAfterSeconds ||
		secondSelection.WorkerStaleAfterSeconds != second.WorkerStaleAfterSeconds ||
		secondSelection.PollIntervalSeconds != second.PollIntervalSeconds ||
		secondSelection.PollIntervalMilliseconds != second.PollIntervalMilliseconds ||
		secondSelection.MaxSupervisorRuns != second.MaxSupervisorRuns ||
		secondSelection.TransportMode != "file-outbox" ||
		!secondSelection.IncludeLifecycleMaintenance ||
		secondSelection.ExecuteLifecycleRetries ||
		secondSelection.Summary.TransportRequests != second.Summary.TransportRequests ||
		secondSelection.SelectedBy != "bob" ||
		secondSelection.SelectedAt != "2026-06-01T18:08:00Z" ||
		secondSelection.Reason != "prefer file outbox policy" {
		t.Fatalf("unexpected second policy selection: %#v", secondSelection)
	}
	if len(secondSelection.SupersededPolicyPaths) != 1 || secondSelection.SupersededPolicyPaths[0] != first.Path {
		t.Fatalf("expected previous current policy to be superseded, got %#v", secondSelection.SupersededPolicyPaths)
	}
	read, found, err := ReadWorkerLifecyclePolicySelection(root)
	if err != nil {
		t.Fatalf("read worker lifecycle policy selection: %v", err)
	}
	if !found || read.PolicyPath != second.Path || read.Status != "current" || read.SelectedBy != "bob" {
		t.Fatalf("unexpected persisted worker lifecycle policy selection found=%t selection=%#v", found, read)
	}
	if countJSON(t, filepath.Join(root, "runtime", "worker-lifecycle-policies")) != 2 {
		t.Fatalf("selection must not create or remove policy artifacts")
	}
	assertOnlyWorkerLifecyclePolicySelectionMarkerChanged(t, root, filesBeforeSecondSelection)
	assertWorkerLifecyclePolicySideEffectsUnchanged(t, root, sideEffectsBefore)
}

func TestClearWorkerLifecyclePolicySelectionWritesClearedMarkerAndPreservesPolicyMetadata(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Date(2026, 6, 1, 18, 30, 0, 0, time.UTC)
	createWorkerLifecycleMaintenanceFixture(t, root, now)
	policy, err := CreateWorkerLifecyclePolicy(root, WorkerLifecyclePolicyOptions{
		Now:                         now.Add(5 * time.Minute),
		StaleAfter:                  5 * time.Minute,
		WorkerStaleAfter:            2 * time.Minute,
		PollInterval:                30 * time.Second,
		MaxSupervisorRuns:           3,
		TransportMode:               "http-json",
		IncludeLifecycleMaintenance: true,
		ExecuteLifecycleRetries:     true,
	})
	if err != nil {
		t.Fatalf("create worker lifecycle policy: %v", err)
	}
	selected, err := SelectWorkerLifecyclePolicy(root, filepath.Base(policy.Path), WorkerLifecyclePolicySelectionOptions{
		SelectedBy: "alice",
		SelectedAt: now.Add(6 * time.Minute),
		Reason:     "current guard",
	})
	if err != nil {
		t.Fatalf("select worker lifecycle policy: %v", err)
	}
	sideEffectsBefore := snapshotWorkerLifecyclePolicySideEffects(t, root)
	filesBeforeClear := snapshotWorkerLifecyclePolicySelectionBoundaryFiles(t, root)

	cleared, err := ClearWorkerLifecyclePolicySelection(root, WorkerLifecyclePolicySelectionClearOptions{
		ClearedBy: "carol",
		ClearedAt: now.Add(7 * time.Minute),
		Reason:    "authorization changed",
	})
	if err != nil {
		t.Fatalf("clear worker lifecycle policy selection: %v", err)
	}
	if cleared.Status != "cleared" || cleared.Mode != "metadata-only" ||
		cleared.Path != filepath.Join(root, "runtime", "worker-lifecycle-policy-selection.json") ||
		cleared.PolicyPath != selected.PolicyPath ||
		cleared.PolicyBasename != selected.PolicyBasename ||
		cleared.Summary.WorkerLeases != selected.Summary.WorkerLeases ||
		cleared.TransportMode != selected.TransportMode ||
		cleared.SelectedBy != "alice" ||
		cleared.SelectedAt != "2026-06-01T18:36:00Z" ||
		cleared.ClearedBy != "carol" ||
		cleared.ClearedAt != "2026-06-01T18:37:00Z" ||
		cleared.Reason != "authorization changed" {
		t.Fatalf("unexpected cleared worker lifecycle policy selection: %#v", cleared)
	}
	read, found, err := ReadWorkerLifecyclePolicySelection(root)
	if err != nil {
		t.Fatalf("read cleared worker lifecycle policy selection: %v", err)
	}
	if !found || read.Status != "cleared" || read.PolicyPath != policy.Path || read.ClearedBy != "carol" {
		t.Fatalf("unexpected persisted cleared worker lifecycle policy selection found=%t selection=%#v", found, read)
	}
	if countJSON(t, filepath.Join(root, "runtime", "worker-lifecycle-policies")) != 1 {
		t.Fatalf("clear selection must not create or remove policy artifacts")
	}
	assertOnlyWorkerLifecyclePolicySelectionMarkerChanged(t, root, filesBeforeClear)
	assertWorkerLifecyclePolicySideEffectsUnchanged(t, root, sideEffectsBefore)
}

func TestClearWorkerLifecyclePolicySelectionWithoutCurrentWritesExplicitClearedMarker(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	filesBeforeClear := snapshotWorkerLifecyclePolicySelectionBoundaryFiles(t, root)
	cleared, err := ClearWorkerLifecyclePolicySelection(root, WorkerLifecyclePolicySelectionClearOptions{
		ClearedBy: "alice",
		ClearedAt: time.Date(2026, 6, 1, 18, 45, 0, 0, time.UTC),
		Reason:    "disable policy runner",
	})
	if err != nil {
		t.Fatalf("clear missing worker lifecycle policy selection: %v", err)
	}
	if cleared.Status != "cleared" || cleared.Mode != "metadata-only" ||
		cleared.PolicyPath != "" ||
		cleared.ClearedBy != "alice" ||
		cleared.ClearedAt != "2026-06-01T18:45:00Z" ||
		cleared.Reason != "disable policy runner" {
		t.Fatalf("unexpected explicit cleared marker: %#v", cleared)
	}
	read, found, err := ReadWorkerLifecyclePolicySelection(root)
	if err != nil {
		t.Fatalf("read explicit cleared worker lifecycle policy selection: %v", err)
	}
	if !found || read.Status != "cleared" || read.PolicyPath != "" {
		t.Fatalf("expected explicit cleared marker, found=%t selection=%#v", found, read)
	}
	assertOnlyWorkerLifecyclePolicySelectionMarkerChanged(t, root, filesBeforeClear)
}

func TestSelectWorkerLifecyclePolicyRejectsOutsidePolicyPath(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	_, err := SelectWorkerLifecyclePolicy(root, filepath.Join(t.TempDir(), "outside.json"), WorkerLifecyclePolicySelectionOptions{
		SelectedBy: "alice",
	})
	if err == nil || !strings.Contains(err.Error(), "worker lifecycle policy path must stay under") {
		t.Fatalf("expected path confinement error, got %v", err)
	}
	if _, found, readErr := ReadWorkerLifecyclePolicySelection(root); readErr != nil || found {
		t.Fatalf("outside path selection must not write marker, found=%t err=%v", found, readErr)
	}
}

func TestPreviewWorkerLifecyclePolicyDoesNotWriteArtifacts(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Date(2026, 6, 1, 16, 45, 0, 0, time.UTC)
	createWorkerLifecycleMaintenanceFixture(t, root, now)

	policy, err := PreviewWorkerLifecyclePolicy(root, WorkerLifecyclePolicyOptions{
		Now:               now.Add(5 * time.Minute),
		StaleAfter:        5 * time.Minute,
		WorkerStaleAfter:  DefaultRuntimeWorkerStaleAfter,
		MaxSupervisorRuns: 2,
	})
	if err != nil {
		t.Fatalf("preview worker lifecycle policy: %v", err)
	}
	if policy.Path != "" {
		t.Fatalf("preview policy must not have artifact path, got %#v", policy)
	}
	if policy.Status != "dry-run" || policy.Mode != "metadata-only" || policy.MaxSupervisorRuns != 2 {
		t.Fatalf("unexpected preview policy: %#v", policy)
	}
	if countJSON(t, filepath.Join(root, "runtime", "worker-lifecycle-policies")) != 0 {
		t.Fatalf("preview policy must not write policy artifacts")
	}
}

func TestCreateWorkerLifecyclePolicyRejectsInvalidBoundsBeforeWriting(t *testing.T) {
	base := WorkerLifecyclePolicyOptions{
		Now:               time.Date(2026, 6, 1, 17, 5, 0, 0, time.UTC),
		StaleAfter:        5 * time.Minute,
		WorkerStaleAfter:  2 * time.Minute,
		PollInterval:      30 * time.Second,
		MaxSupervisorRuns: 2,
		TransportMode:     "metadata-only",
	}
	tests := []struct {
		name    string
		options WorkerLifecyclePolicyOptions
		want    string
	}{
		{
			name: "stale after",
			options: WorkerLifecyclePolicyOptions{
				Now:               base.Now,
				StaleAfter:        0,
				WorkerStaleAfter:  base.WorkerStaleAfter,
				PollInterval:      base.PollInterval,
				MaxSupervisorRuns: base.MaxSupervisorRuns,
				TransportMode:     base.TransportMode,
			},
			want: "stale after must be positive",
		},
		{
			name: "worker stale after",
			options: WorkerLifecyclePolicyOptions{
				Now:               base.Now,
				StaleAfter:        base.StaleAfter,
				WorkerStaleAfter:  -time.Second,
				PollInterval:      base.PollInterval,
				MaxSupervisorRuns: base.MaxSupervisorRuns,
				TransportMode:     base.TransportMode,
			},
			want: "worker stale after must be positive",
		},
		{
			name: "max supervisor runs",
			options: WorkerLifecyclePolicyOptions{
				Now:               base.Now,
				StaleAfter:        base.StaleAfter,
				WorkerStaleAfter:  base.WorkerStaleAfter,
				PollInterval:      base.PollInterval,
				MaxSupervisorRuns: 0,
				TransportMode:     base.TransportMode,
			},
			want: "max supervisor runs must be positive",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "workspace")
			_, err := CreateWorkerLifecyclePolicy(root, tc.options)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected invalid bound error %q, got %v", tc.want, err)
			}
			if countJSON(t, filepath.Join(root, "runtime", "worker-lifecycle-policies")) != 0 {
				t.Fatalf("invalid policy must not write policy artifacts")
			}
		})
	}
}

func TestCreateWorkerLifecyclePolicyRejectsUnsupportedTransportBeforeWriting(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	_, err := CreateWorkerLifecyclePolicy(root, WorkerLifecyclePolicyOptions{
		Now:               time.Date(2026, 6, 1, 17, 10, 0, 0, time.UTC),
		StaleAfter:        5 * time.Minute,
		WorkerStaleAfter:  2 * time.Minute,
		PollInterval:      30 * time.Second,
		MaxSupervisorRuns: 2,
		TransportMode:     "smtp",
	})
	if err == nil || !strings.Contains(err.Error(), "unsupported worker queue dispatch transport") {
		t.Fatalf("expected unsupported transport error, got %v", err)
	}
	if countJSON(t, filepath.Join(root, "runtime", "worker-lifecycle-policies")) != 0 {
		t.Fatalf("invalid transport policy must not write policy artifacts")
	}
}

func TestPreviewWorkerLifecyclePolicyRejectsRetryGateWithoutLifecycleMaintenance(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	_, err := PreviewWorkerLifecyclePolicy(root, WorkerLifecyclePolicyOptions{
		StaleAfter:              5 * time.Minute,
		WorkerStaleAfter:        2 * time.Minute,
		MaxSupervisorRuns:       1,
		ExecuteLifecycleRetries: true,
	})
	if err == nil || !strings.Contains(err.Error(), "requires lifecycle maintenance") {
		t.Fatalf("expected lifecycle retry policy to require lifecycle maintenance, got %v", err)
	}
	if countJSON(t, filepath.Join(root, "runtime", "worker-lifecycle-policies")) != 0 {
		t.Fatalf("invalid preview policy must not write policy artifacts")
	}
}

func TestCreateWorkerLifecyclePolicyRejectsRetryHandoffWithoutRequiredLifecycleGates(t *testing.T) {
	now := time.Date(2026, 6, 2, 11, 45, 0, 0, time.UTC)
	tests := []struct {
		name    string
		options WorkerLifecyclePolicyOptions
		want    string
	}{
		{
			name: "without lifecycle maintenance",
			options: WorkerLifecyclePolicyOptions{
				StaleAfter:                   5 * time.Minute,
				WorkerStaleAfter:             2 * time.Minute,
				MaxSupervisorRuns:            1,
				ExecuteLifecycleRetries:      true,
				DispatchLifecycleRetryClaims: true,
				LifecycleRetryTransportMode:  "file-outbox",
			},
			want: "requires lifecycle maintenance",
		},
		{
			name: "without lifecycle retry execution",
			options: WorkerLifecyclePolicyOptions{
				StaleAfter:                   5 * time.Minute,
				WorkerStaleAfter:             2 * time.Minute,
				MaxSupervisorRuns:            1,
				IncludeLifecycleMaintenance:  true,
				DispatchLifecycleRetryClaims: true,
				LifecycleRetryTransportMode:  "file-outbox",
			},
			want: "requires lifecycle retry execution",
		},
		{
			name: "http-json",
			options: WorkerLifecyclePolicyOptions{
				StaleAfter:                   5 * time.Minute,
				WorkerStaleAfter:             2 * time.Minute,
				MaxSupervisorRuns:            1,
				IncludeLifecycleMaintenance:  true,
				ExecuteLifecycleRetries:      true,
				DispatchLifecycleRetryClaims: true,
				LifecycleRetryTransportMode:  "http-json",
			},
			want: "supports metadata-only or file-outbox transport",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "workspace")
			tc.options.Now = now
			_, err := CreateWorkerLifecyclePolicy(root, tc.options)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected lifecycle retry handoff validation error %q, got %v", tc.want, err)
			}
			if countJSON(t, filepath.Join(root, "runtime", "worker-lifecycle-policies")) != 0 {
				t.Fatalf("invalid retry handoff policy must not write policy artifacts")
			}
		})
	}
}

func snapshotWorkerLifecyclePolicySideEffects(t *testing.T, root string) map[string]int {
	t.Helper()
	paths := []string{
		"runtime/worker-lifecycle-maintenance-supervisor-runs",
		"runtime/worker-orchestration-supervisor-runs",
		"runtime/worker-lifecycle-maintenance",
		"runtime/worker-lifecycle-audits",
		"runtime/worker-lifecycle-reviews",
		"runtime/worker-lifecycle-action-plans",
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
		"runtime/worker-dispatch-retry-plans",
		"runtime/worker-dispatch-retry-dispatches",
		"runtime/worker-dispatch-retry-dispatch-executions",
		"runtime/worker-placement-plans",
		"runtime/worker-placement-claim-executions",
		"runtime/worker-placement-dispatch-plans",
		"runtime/worker-placement-dispatch-executions",
		"runtime/worker-placement-dispatch-batches",
		"runtime/worker-orchestrations",
	}
	counts := make(map[string]int, len(paths))
	for _, rel := range paths {
		counts[rel] = countJSONRecursive(t, filepath.Join(root, rel))
	}
	return counts
}

func assertWorkerLifecyclePolicySideEffectsUnchanged(t *testing.T, root string, before map[string]int) {
	t.Helper()
	after := snapshotWorkerLifecyclePolicySideEffects(t, root)
	for path, beforeCount := range before {
		if after[path] != beforeCount {
			t.Fatalf("policy must not change %s artifacts: before=%d after=%d", path, beforeCount, after[path])
		}
	}
}

func snapshotWorkerLifecyclePolicySelectionBoundaryFiles(t *testing.T, root string) map[string]string {
	t.Helper()
	files := map[string]string{}
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if info == nil || info.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		files[filepath.ToSlash(rel)] = string(data)
		return nil
	})
	if err != nil {
		t.Fatalf("snapshot worker lifecycle policy selection boundary files: %v", err)
	}
	return files
}

func assertOnlyWorkerLifecyclePolicySelectionMarkerChanged(t *testing.T, root string, before map[string]string) {
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
	want := filepath.ToSlash(filepath.Join("runtime", "worker-lifecycle-policy-selection.json"))
	if len(changed) != 1 || changed[0] != want {
		t.Fatalf("policy selection must only change %s, got changed files %#v", want, changed)
	}
}

func countJSONRecursive(t *testing.T, dir string) int {
	t.Helper()
	total := 0
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if info != nil && !info.IsDir() && filepath.Ext(path) == ".json" {
			total++
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk json under %s: %v", dir, err)
	}
	return total
}
