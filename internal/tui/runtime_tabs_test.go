package tui

import (
	"encoding/base64"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ouroboros/ouroboros/internal/evidence"
)

func TestRuntimeTabsRenderDefaultNodeInspector(t *testing.T) {
	tabs := DefaultRuntimeTabs()

	if tabs.Active != RuntimeTabNodeInspector {
		t.Fatalf("expected node inspector active by default, got %s", tabs.Active)
	}
	view := tabs.Render(72, 8)
	for _, text := range []string{
		"[Node Inspector]",
		"Model Runtime",
		"Agent Runtime",
		"Toolchain Health",
		"Coverage Matrix",
		"Frontier Queue",
		"Timeline Lanes",
		"selected: none",
	} {
		if !strings.Contains(view, text) {
			t.Fatalf("runtime tabs missing %q:\n%s", text, view)
		}
	}
}

func TestRuntimeTabsSwitchActiveTab(t *testing.T) {
	tabs := DefaultRuntimeTabs().Next()
	if tabs.Active != RuntimeTabModelRuntime {
		t.Fatalf("expected model runtime after next, got %s", tabs.Active)
	}
	if !strings.Contains(tabs.Render(56, 6), "[Model Runtime]") {
		t.Fatalf("expected active model runtime tab:\n%s", tabs.Render(56, 6))
	}

	tabs = tabs.Select(RuntimeTabFrontierQueue)
	if tabs.Active != RuntimeTabFrontierQueue {
		t.Fatalf("expected frontier queue tab, got %s", tabs.Active)
	}
	view := tabs.Render(56, 6)
	if !strings.Contains(view, "[Frontier Queue]") || !strings.Contains(view, "frontier: empty") {
		t.Fatalf("expected frontier queue content:\n%s", view)
	}
}

func TestRuntimeTabsSelectActiveRows(t *testing.T) {
	tabs := DefaultRuntimeTabs().
		Select(RuntimeTabToolchainHealth).
		WithRows(RuntimeTabToolchainHealth, []string{
			"toolchain: missing",
			"dep httpx: missing required=true",
			"install httpx: missing required=true command=ouro-toolchain install httpx",
		}).
		SelectRow(2)

	row, ok := tabs.ActiveSelectedRow()
	if !ok || !strings.Contains(row, "install httpx") {
		t.Fatalf("expected install row to be selected, ok=%v row=%q", ok, row)
	}
	view := tabs.Render(80, 8)
	if !strings.Contains(view, "> install httpx: missing") {
		t.Fatalf("expected selected row marker in runtime tabs:\n%s", view)
	}
}

func TestRuntimeTabsRenderKeepsSelectedRowVisible(t *testing.T) {
	rows := []string{
		"toolchain: unknown",
		"contract summary",
		"contract error 1",
		"contract error 2",
		"contract error 3",
		"compile-preview reviews: total=1",
		"compile-preview review artifact: candidate.json",
		"compile-preview review selection: status=current current=true review=candidate.json",
	}
	tabs := DefaultRuntimeTabs().
		Select(RuntimeTabToolchainHealth).
		WithRows(RuntimeTabToolchainHealth, rows).
		SelectRow(7)

	view := tabs.Render(96, 7)
	if !strings.Contains(view, "> compile-preview review selection: status=current current=true") {
		t.Fatalf("expected selected row to stay visible in short runtime tab:\n%s", view)
	}
	if strings.Contains(view, "toolchain: unknown") {
		t.Fatalf("expected short runtime tab to scroll past unselected top rows:\n%s", view)
	}
}

func TestModelUpdateMovesRuntimeTabSelection(t *testing.T) {
	tabs := DefaultRuntimeTabs().
		Select(RuntimeTabToolchainHealth).
		WithRows(RuntimeTabToolchainHealth, []string{
			"toolchain: missing",
			"install httpx: missing required=true command=ouro-toolchain install httpx",
		})
	model := NewModel().WithGraphStack(GraphStack{RuntimeTabs: tabs})

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyDown})
	if cmd != nil {
		t.Fatalf("expected row selection update to be synchronous")
	}
	next, ok := updated.(Model)
	if !ok {
		t.Fatalf("expected Model, got %T", updated)
	}
	row, ok := next.graphStack.RuntimeTabs.ActiveSelectedRow()
	if !ok || !strings.Contains(row, "install httpx") {
		t.Fatalf("expected down key to select remediation row, ok=%v row=%q", ok, row)
	}
}

func TestModelUpdateRunCancelRequiresSecondConfirm(t *testing.T) {
	called := false
	model := NewModel().WithRuntimeControlRequester(func(action RuntimeControlAction) (RuntimeControlResult, error) {
		called = true
		if action.Kind != "cancel-run" {
			t.Fatalf("unexpected runtime control action: %#v", action)
		}
		return RuntimeControlResult{Status: "requested", Message: "run context canceled"}, nil
	})

	armed, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	if cmd != nil {
		t.Fatalf("first cancel keypress should only arm confirmation")
	}
	if called {
		t.Fatalf("runtime control requester should not run before confirmation")
	}
	armedModel, ok := armed.(Model)
	if !ok {
		t.Fatalf("expected Model, got %T", armed)
	}
	armedRows := armedModel.graphStack.RuntimeTabs.Select(RuntimeTabAgentRuntime).ActiveRows()
	if !containsRow(armedRows, "run control: cancel confirm-required") {
		t.Fatalf("expected cancel confirmation row, got %#v", armedRows)
	}

	requesting, cmd := armedModel.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	if cmd == nil {
		t.Fatalf("second cancel keypress should request cancellation")
	}
	if called {
		t.Fatalf("runtime control requester should run only when command executes")
	}
	msg := cmd()
	if !called {
		t.Fatalf("expected runtime control requester to run")
	}
	updated, cmd := requesting.Update(msg)
	if cmd != nil {
		t.Fatalf("runtime control result should be synchronous")
	}
	next, ok := updated.(Model)
	if !ok {
		t.Fatalf("expected Model, got %T", updated)
	}
	rows := next.graphStack.RuntimeTabs.Select(RuntimeTabAgentRuntime).ActiveRows()
	for _, want := range []string{
		"run control: cancel requesting",
		"run control: cancel requested run context canceled",
	} {
		if !containsRow(rows, want) {
			t.Fatalf("expected runtime control row %q, got %#v", want, rows)
		}
	}
}

func TestModelUpdateRunPauseResumeUsesRuntimeControlRequester(t *testing.T) {
	actions := []string{}
	model := NewModel().WithRuntimeControlRequester(func(action RuntimeControlAction) (RuntimeControlResult, error) {
		actions = append(actions, action.Kind)
		switch action.Kind {
		case "pause-run":
			return RuntimeControlResult{Status: "paused", Message: "worker admission paused"}, nil
		case "resume-run":
			return RuntimeControlResult{Status: "resumed", Message: "worker admission resumed"}, nil
		default:
			t.Fatalf("unexpected runtime control action: %#v", action)
			return RuntimeControlResult{}, nil
		}
	})

	pausing, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
	if cmd == nil {
		t.Fatalf("pause key should request runtime control")
	}
	paused, cmd := pausing.Update(cmd())
	if cmd != nil {
		t.Fatalf("pause result should be synchronous")
	}
	pausedModel, ok := paused.(Model)
	if !ok {
		t.Fatalf("expected Model, got %T", paused)
	}
	rows := pausedModel.graphStack.RuntimeTabs.Select(RuntimeTabAgentRuntime).ActiveRows()
	if !containsRow(rows, "run control: pause paused worker admission paused") {
		t.Fatalf("expected pause feedback row, got %#v", rows)
	}

	resuming, cmd := pausedModel.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
	if cmd == nil {
		t.Fatalf("second pause key should request resume")
	}
	resumed, _ := resuming.Update(cmd())
	resumedModel, ok := resumed.(Model)
	if !ok {
		t.Fatalf("expected Model, got %T", resumed)
	}
	rows = resumedModel.graphStack.RuntimeTabs.Select(RuntimeTabAgentRuntime).ActiveRows()
	if !containsRow(rows, "run control: resume resumed worker admission resumed") {
		t.Fatalf("expected resume feedback row, got %#v", rows)
	}
	if strings.Join(actions, ",") != "pause-run,resume-run" {
		t.Fatalf("unexpected runtime control actions: %#v", actions)
	}
}

func TestRuntimeTabsActivateSelectedRemediationRow(t *testing.T) {
	tabs := DefaultRuntimeTabs().
		Select(RuntimeTabToolchainHealth).
		WithRows(RuntimeTabToolchainHealth, []string{
			"toolchain: missing",
			"install httpx: missing required=true command=ouro-toolchain install httpx depends_on=z-runtime",
		}).
		SelectRow(1).
		ActivateSelectedRow()

	action, ok := tabs.ActiveAction()
	if !ok || !strings.Contains(action, "remediation selected install httpx") {
		t.Fatalf("expected remediation action, ok=%v action=%q", ok, action)
	}
	details, ok := tabs.ActiveRuntimeAction()
	if !ok {
		t.Fatalf("expected active runtime action details")
	}
	if details.Kind != "install" || details.Name != "httpx" || details.Command != "ouro-toolchain install httpx" || details.DependsOn != "z-runtime" {
		t.Fatalf("unexpected runtime action details: %#v", details)
	}
	view := tabs.Render(96, 10)
	for _, want := range []string{
		"action: remediation selected install httpx",
		"action command: ouro-toolchain install httpx",
		"action depends_on: z-runtime",
		"action mode: metadata-only",
		"> install httpx: missing",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("runtime tabs missing %q:\n%s", want, view)
		}
	}
}

func TestRuntimeTabsWorkerLifecyclePolicyArtifactDetailsAreReadOnly(t *testing.T) {
	tabs := DefaultRuntimeTabs().
		Select(RuntimeTabAgentRuntime).
		WithRows(RuntimeTabAgentRuntime, []string{
			"agents: 0",
			"worker lifecycle policy artifact: /tmp/ouro/policy.json status=dry-run mode=metadata-only transport=metadata-only stale_after=300 worker_stale_after=120 poll_interval_ms=250 max_runs=3 lifecycle_maintenance=true lifecycle_retries=true workers=2 stale_workers=1 stale_leases=1",
		}).
		SelectRow(1)
	model := NewModel().WithGraphStack(GraphStack{RuntimeTabs: tabs})

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	if cmd != nil {
		t.Fatalf("expected policy detail update to be synchronous")
	}
	detailed, ok := updated.(Model)
	if !ok {
		t.Fatalf("expected Model, got %T", updated)
	}
	action := detailed.graphStack.RuntimeAction
	if action.Kind != "worker-lifecycle-policy-artifact" || action.ArtifactPath != "/tmp/ouro/policy.json" {
		t.Fatalf("expected policy artifact runtime action, got %#v", action)
	}
	if action.State != "dry-run" || action.ExecutionMode != "metadata-only" || action.TransportMode != "metadata-only" {
		t.Fatalf("unexpected policy action metadata: %#v", action)
	}
	view := strings.Join(action.RenderRows(), "\n")
	for _, want := range []string{
		"action: worker lifecycle policy artifact selected /tmp/ouro/policy.json",
		"artifact path: /tmp/ouro/policy.json",
		"transport: metadata-only",
		"ready: workers=2 stale_workers=1 stale_leases=1",
		"results: stale_after=300 worker_stale_after=120 poll_interval_ms=250 max_runs=3 lifecycle_maintenance=true lifecycle_retries=true",
		"action mode: read-only",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("policy runtime tabs missing %q:\n%s", want, view)
		}
	}

	inspector := detailed.graphStack.Inspector().Rows
	for _, want := range []string{
		"runtime action: worker-lifecycle-policy-artifact",
		"artifact path: /tmp/ouro/policy.json",
		"state: dry-run",
		"execution mode: metadata-only",
		"transport: metadata-only",
		"ready: workers=2 stale_workers=1 stale_leases=1",
		"results: stale_after=300 worker_stale_after=120 poll_interval_ms=250 max_runs=3 lifecycle_maintenance=true lifecycle_retries=true",
		"mode: read-only",
	} {
		if !containsRow(inspector, want) {
			t.Fatalf("policy inspector missing %q, got %#v", want, inspector)
		}
	}
}

func TestRuntimeTabsWorkerLifecyclePolicyRunArtifactDetailsAreReadOnly(t *testing.T) {
	tabs := DefaultRuntimeTabs().
		Select(RuntimeTabAgentRuntime).
		WithRows(RuntimeTabAgentRuntime, []string{
			"agents: 0",
			"worker lifecycle policy runs: total=1",
			"worker lifecycle policy run artifact: /tmp/ouro/runtime/worker-lifecycle-policy-runs/run-001.json status=skipped mode=metadata-only policy=/tmp/ouro/runtime/worker-lifecycle-policies/policy-b.json selection=/tmp/ouro/runtime/worker-lifecycle-policy-selection.json max_runs=3 poll_interval_ms=250 transport=metadata-only include_lifecycle_maintenance=true execute_lifecycle_retries=false supervisor_created=0 skip=policy run disabled by operator",
		}).
		SelectRow(2)
	model := NewModel().WithGraphStack(GraphStack{RuntimeTabs: tabs})

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	if cmd != nil {
		t.Fatalf("expected policy run detail update to be synchronous")
	}
	detailed, ok := updated.(Model)
	if !ok {
		t.Fatalf("expected Model, got %T", updated)
	}
	action := detailed.graphStack.RuntimeAction
	if action.Kind != "worker-lifecycle-policy-run-artifact" || action.ArtifactPath != "/tmp/ouro/runtime/worker-lifecycle-policy-runs/run-001.json" {
		t.Fatalf("expected policy run artifact runtime action, got %#v", action)
	}
	if action.State != "skipped" || action.ExecutionMode != "metadata-only" || action.TransportMode != "metadata-only" {
		t.Fatalf("unexpected policy run action metadata: %#v", action)
	}
	if action.Source != "/tmp/ouro/runtime/worker-lifecycle-policies/policy-b.json" || action.RequestPath != "/tmp/ouro/runtime/worker-lifecycle-policy-selection.json" {
		t.Fatalf("unexpected policy run references: %#v", action)
	}
	view := strings.Join(action.RenderRows(), "\n")
	for _, want := range []string{
		"action: worker lifecycle policy run artifact selected /tmp/ouro/runtime/worker-lifecycle-policy-runs/run-001.json",
		"artifact path: /tmp/ouro/runtime/worker-lifecycle-policy-runs/run-001.json",
		"source: /tmp/ouro/runtime/worker-lifecycle-policies/policy-b.json",
		"transport: metadata-only",
		"results: max_runs=3 poll_interval_ms=250 include_lifecycle_maintenance=true execute_lifecycle_retries=false supervisor_created=0",
		"skipped: policy run disabled by operator",
		"request path: /tmp/ouro/runtime/worker-lifecycle-policy-selection.json",
		"action mode: read-only",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("policy run runtime tabs missing %q:\n%s", want, view)
		}
	}

	inspector := detailed.graphStack.Inspector().Rows
	for _, want := range []string{
		"runtime action: worker-lifecycle-policy-run-artifact",
		"artifact path: /tmp/ouro/runtime/worker-lifecycle-policy-runs/run-001.json",
		"state: skipped",
		"execution mode: metadata-only",
		"source: /tmp/ouro/runtime/worker-lifecycle-policies/policy-b.json",
		"request path: /tmp/ouro/runtime/worker-lifecycle-policy-selection.json",
		"transport: metadata-only",
		"results: max_runs=3 poll_interval_ms=250 include_lifecycle_maintenance=true execute_lifecycle_retries=false supervisor_created=0",
		"skipped: policy run disabled by operator",
		"mode: read-only",
	} {
		if !containsRow(inspector, want) {
			t.Fatalf("policy run inspector missing %q, got %#v", want, inspector)
		}
	}
}

func TestRuntimeTabsPolicySchedulerRunArtifactDetailsAreReadOnly(t *testing.T) {
	tabs := DefaultRuntimeTabs().
		Select(RuntimeTabAgentRuntime).
		WithRows(RuntimeTabAgentRuntime, []string{
			"agents: 0",
			"worker lifecycle policy scheduler run artifact: /tmp/ouro/runtime/worker-lifecycle-policy-scheduler-runs/scheduler-001.json status=failed mode=metadata-only policy=/tmp/ouro/runtime/worker-lifecycle-policies/policy-b.json selection=/tmp/ouro/runtime/worker-lifecycle-policy-selection.json policy_basename=policy-b.json selection_basename=worker-lifecycle-policy-selection.json max_runs=3 poll_interval_ms=250 transport=metadata-only include_lifecycle_maintenance=true execute_lifecycle_retries=false policy_runs_created=2 supervisor_created=1 skip=scheduler disabled by operator error=failed to open policy artifact",
		}).
		SelectRow(1)
	model := NewModel().WithGraphStack(GraphStack{RuntimeTabs: tabs})

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	if cmd != nil {
		t.Fatalf("expected policy scheduler detail update to be synchronous")
	}
	detailed, ok := updated.(Model)
	if !ok {
		t.Fatalf("expected Model, got %T", updated)
	}
	action := detailed.graphStack.RuntimeAction
	if action.Kind != "worker-lifecycle-policy-scheduler-run-artifact" || action.ArtifactPath != "/tmp/ouro/runtime/worker-lifecycle-policy-scheduler-runs/scheduler-001.json" {
		t.Fatalf("expected policy scheduler run artifact runtime action, got %#v", action)
	}
	if action.State != "failed" || action.ExecutionMode != "metadata-only" || action.TransportMode != "metadata-only" {
		t.Fatalf("unexpected policy scheduler action metadata: %#v", action)
	}
	if action.Source != "/tmp/ouro/runtime/worker-lifecycle-policies/policy-b.json" || action.RequestPath != "/tmp/ouro/runtime/worker-lifecycle-policy-selection.json" || action.Name != "policy-b.json" {
		t.Fatalf("unexpected policy scheduler references: %#v", action)
	}
	view := strings.Join(action.RenderRows(), "\n")
	for _, want := range []string{
		"action: worker lifecycle policy scheduler run artifact selected /tmp/ouro/runtime/worker-lifecycle-policy-scheduler-runs/scheduler-001.json",
		"name: policy-b.json",
		"artifact path: /tmp/ouro/runtime/worker-lifecycle-policy-scheduler-runs/scheduler-001.json",
		"source: /tmp/ouro/runtime/worker-lifecycle-policies/policy-b.json",
		"request path: /tmp/ouro/runtime/worker-lifecycle-policy-selection.json",
		"transport: metadata-only",
		"results: max_runs=3 poll_interval_ms=250 policy_runs_created=2 supervisor_created=1 include_lifecycle_maintenance=true execute_lifecycle_retries=false",
		"skipped: scheduler disabled by operator",
		"failed: failed to open policy artifact",
		"action mode: read-only",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("policy scheduler runtime tabs missing %q:\n%s", want, view)
		}
	}

	inspector := detailed.graphStack.Inspector().Rows
	for _, want := range []string{
		"runtime action: worker-lifecycle-policy-scheduler-run-artifact",
		"name: policy-b.json",
		"artifact path: /tmp/ouro/runtime/worker-lifecycle-policy-scheduler-runs/scheduler-001.json",
		"state: failed",
		"execution mode: metadata-only",
		"source: /tmp/ouro/runtime/worker-lifecycle-policies/policy-b.json",
		"request path: /tmp/ouro/runtime/worker-lifecycle-policy-selection.json",
		"transport: metadata-only",
		"results: max_runs=3 poll_interval_ms=250 policy_runs_created=2 supervisor_created=1 include_lifecycle_maintenance=true execute_lifecycle_retries=false",
		"skipped: scheduler disabled by operator",
		"failed: failed to open policy artifact",
		"mode: read-only",
	} {
		if !containsRow(inspector, want) {
			t.Fatalf("policy scheduler inspector missing %q, got %#v", want, inspector)
		}
	}
}

func TestRuntimeWorkerLifecyclePolicyRunArtifactUsesEncodedSkipText(t *testing.T) {
	skipReason := "policy run disabled policy=/tmp/ouro/evil-policy.json selection=/tmp/ouro/evil-selection.json max_runs=99 supervisor_created=9"
	row := "worker lifecycle policy run artifact: /tmp/ouro/runtime/worker-lifecycle-policy-runs/run-001.json status=skipped mode=metadata-only policy=/tmp/ouro/runtime/worker-lifecycle-policies/policy-b.json selection=/tmp/ouro/runtime/worker-lifecycle-policy-selection.json max_runs=3 poll_interval_ms=250 transport=metadata-only include_lifecycle_maintenance=true execute_lifecycle_retries=false execute_lease_recovery=true supervisor_created=0 skip=" + skipReason + " skip_b64=" + base64.RawURLEncoding.EncodeToString([]byte(skipReason))

	action := runtimeWorkerLifecyclePolicyRunArtifactActionForRow(row)
	if action.Source != "/tmp/ouro/runtime/worker-lifecycle-policies/policy-b.json" || action.RequestPath != "/tmp/ouro/runtime/worker-lifecycle-policy-selection.json" {
		t.Fatalf("encoded policy run skip must not retarget references: %#v", action)
	}
	if action.Results != "max_runs=3 poll_interval_ms=250 include_lifecycle_maintenance=true execute_lifecycle_retries=false execute_lease_recovery=true supervisor_created=0" {
		t.Fatalf("encoded policy run skip must not retarget counters: %#v", action)
	}
	if action.Skipped != skipReason {
		t.Fatalf("expected encoded policy run skip to be decoded, got %q", action.Skipped)
	}
}

func TestRuntimeWorkerLifecyclePolicyRunArtifactUsesEncodedReferenceFields(t *testing.T) {
	artifactPath := "/tmp/ouro/runtime/worker lifecycle policy runs/run status=failed.json"
	policyPath := "/tmp/ouro/runtime/worker lifecycle policies/policy status=failed.json"
	selectionPath := "/tmp/ouro/runtime/worker lifecycle policy selection mode=execute.json"
	skipReason := "policy run disabled policy=/tmp/ouro/evil-policy.json selection=/tmp/ouro/evil-selection.json"
	row := "worker lifecycle policy run artifact: " + artifactPath +
		" artifact_b64=" + base64.RawURLEncoding.EncodeToString([]byte(artifactPath)) +
		" status=skipped mode=metadata-only policy=" + policyPath +
		" policy_b64=" + base64.RawURLEncoding.EncodeToString([]byte(policyPath)) +
		" selection=" + selectionPath +
		" selection_b64=" + base64.RawURLEncoding.EncodeToString([]byte(selectionPath)) +
		" max_runs=3 poll_interval_ms=250 transport=metadata-only include_lifecycle_maintenance=true execute_lifecycle_retries=false execute_lease_recovery=true supervisor_created=0 skip=" + skipReason +
		" skip_b64=" + base64.RawURLEncoding.EncodeToString([]byte(skipReason))

	action := runtimeWorkerLifecyclePolicyRunArtifactActionForRow(row)
	if action.ArtifactPath != artifactPath || action.Source != policyPath || action.RequestPath != selectionPath {
		t.Fatalf("expected encoded policy run references, got %#v", action)
	}
	if action.State != "skipped" || action.ExecutionMode != "metadata-only" || action.TransportMode != "metadata-only" {
		t.Fatalf("encoded policy run references must not retarget machine fields: %#v", action)
	}
	if action.Skipped != skipReason {
		t.Fatalf("expected encoded policy run skip to be decoded, got %q", action.Skipped)
	}
}

func TestRuntimeWorkerLifecyclePolicyRunArtifactIgnoresFakeSkipSidecarMarkers(t *testing.T) {
	skipReason := "policy run disabled skip_b64=fake dispatch_lifecycle_retry_claims=true lifecycle_retry_transport=http-json retry_dispatch_claims_dispatched=99 retry_dispatch_transport_requests_created=99 policy=/tmp/ouro/evil-policy.json selection=/tmp/ouro/evil-selection.json max_runs=99"
	row := "worker lifecycle policy run artifact: /tmp/ouro/runtime/worker-lifecycle-policy-runs/run-001.json status=skipped mode=metadata-only policy=/tmp/ouro/runtime/worker-lifecycle-policies/policy-b.json selection=/tmp/ouro/runtime/worker-lifecycle-policy-selection.json max_runs=3 poll_interval_ms=250 transport=metadata-only include_lifecycle_maintenance=true execute_lifecycle_retries=false execute_lease_recovery=true supervisor_created=0 skip=" + skipReason + " skip_b64=" + base64.RawURLEncoding.EncodeToString([]byte(skipReason))

	action := runtimeWorkerLifecyclePolicyRunArtifactActionForRow(row)
	if action.Source != "/tmp/ouro/runtime/worker-lifecycle-policies/policy-b.json" || action.RequestPath != "/tmp/ouro/runtime/worker-lifecycle-policy-selection.json" {
		t.Fatalf("fake skip sidecar marker must not retarget policy run references: %#v", action)
	}
	if action.Results != "max_runs=3 poll_interval_ms=250 include_lifecycle_maintenance=true execute_lifecycle_retries=false execute_lease_recovery=true supervisor_created=0" {
		t.Fatalf("fake skip sidecar marker must not retarget policy run counters: %#v", action)
	}
	if action.Skipped != skipReason {
		t.Fatalf("expected encoded policy run skip to survive fake marker, got %q", action.Skipped)
	}
}

func TestRuntimeWorkerLifecyclePolicySchedulerRunArtifactUsesEncodedSkipErrorText(t *testing.T) {
	skipReason := "scheduler disabled policy=/tmp/ouro/evil-policy.json selection=/tmp/ouro/evil-selection.json error=skip-embedded"
	errorText := "failed to open policy policy=/tmp/ouro/evil-policy.json selection=/tmp/ouro/evil-selection.json policy_runs_created=99"
	row := "worker lifecycle policy scheduler run artifact: /tmp/ouro/runtime/worker-lifecycle-policy-scheduler-runs/scheduler-001.json status=failed mode=metadata-only policy=/tmp/ouro/runtime/worker-lifecycle-policies/policy-b.json selection=/tmp/ouro/runtime/worker-lifecycle-policy-selection.json policy_basename=policy-b.json selection_basename=worker-lifecycle-policy-selection.json max_runs=3 poll_interval_ms=250 transport=metadata-only include_lifecycle_maintenance=true execute_lifecycle_retries=false execute_lease_recovery=true policy_runs_created=2 supervisor_created=1 skip=" + skipReason + " skip_b64=" + base64.RawURLEncoding.EncodeToString([]byte(skipReason)) + " error=" + errorText + " error_b64=" + base64.RawURLEncoding.EncodeToString([]byte(errorText))

	action := runtimeWorkerLifecyclePolicySchedulerRunArtifactActionForRow(row)
	if action.Source != "/tmp/ouro/runtime/worker-lifecycle-policies/policy-b.json" || action.RequestPath != "/tmp/ouro/runtime/worker-lifecycle-policy-selection.json" || action.Name != "policy-b.json" {
		t.Fatalf("encoded policy scheduler skip/error must not retarget references: %#v", action)
	}
	if action.Results != "max_runs=3 poll_interval_ms=250 policy_runs_created=2 supervisor_created=1 include_lifecycle_maintenance=true execute_lifecycle_retries=false execute_lease_recovery=true" {
		t.Fatalf("encoded policy scheduler skip/error must not retarget counters: %#v", action)
	}
	if action.Skipped != skipReason || action.Failed != errorText {
		t.Fatalf("expected encoded policy scheduler skip/error to be decoded, got skipped=%q failed=%q", action.Skipped, action.Failed)
	}
}

func TestRuntimeWorkerLifecyclePolicySchedulerRunArtifactUsesEncodedReferenceFields(t *testing.T) {
	artifactPath := "/tmp/ouro/runtime/worker lifecycle policy scheduler runs/scheduler status=failed.json"
	policyPath := "/tmp/ouro/runtime/worker lifecycle policies/policy status=failed.json"
	selectionPath := "/tmp/ouro/runtime/worker lifecycle policy selection mode=execute.json"
	skipReason := "scheduler disabled policy=/tmp/ouro/evil-policy.json selection=/tmp/ouro/evil-selection.json"
	errorText := "failed to open policy policy=/tmp/ouro/error-policy.json selection=/tmp/ouro/error-selection.json"
	row := "worker lifecycle policy scheduler run artifact: " + artifactPath +
		" artifact_b64=" + base64.RawURLEncoding.EncodeToString([]byte(artifactPath)) +
		" status=failed mode=metadata-only policy=" + policyPath +
		" policy_b64=" + base64.RawURLEncoding.EncodeToString([]byte(policyPath)) +
		" selection=" + selectionPath +
		" selection_b64=" + base64.RawURLEncoding.EncodeToString([]byte(selectionPath)) +
		" policy_basename=policy-b.json selection_basename=worker-lifecycle-policy-selection.json max_runs=3 poll_interval_ms=250 transport=metadata-only include_lifecycle_maintenance=true execute_lifecycle_retries=false execute_lease_recovery=true policy_runs_created=2 supervisor_created=1 skip=" + skipReason +
		" skip_b64=" + base64.RawURLEncoding.EncodeToString([]byte(skipReason)) +
		" error=" + errorText +
		" error_b64=" + base64.RawURLEncoding.EncodeToString([]byte(errorText))

	action := runtimeWorkerLifecyclePolicySchedulerRunArtifactActionForRow(row)
	if action.ArtifactPath != artifactPath || action.Source != policyPath || action.RequestPath != selectionPath {
		t.Fatalf("expected encoded policy scheduler references, got %#v", action)
	}
	if action.State != "failed" || action.ExecutionMode != "metadata-only" || action.Name != "policy-b.json" {
		t.Fatalf("encoded policy scheduler references must not retarget machine fields: %#v", action)
	}
	if action.Skipped != skipReason || action.Failed != errorText {
		t.Fatalf("expected encoded policy scheduler skip/error to be decoded, got skipped=%q failed=%q", action.Skipped, action.Failed)
	}
}

func TestRuntimeWorkerLifecyclePolicySchedulerRunArtifactIgnoresFakeSkipErrorSidecarMarkers(t *testing.T) {
	skipReason := "scheduler disabled skip_b64=fake dispatch_lifecycle_retry_claims=true lifecycle_retry_transport=http-json error=skip-embedded error_b64=fake policy=/tmp/ouro/evil-policy.json selection=/tmp/ouro/evil-selection.json policy_runs_created=99 supervisor_created=99"
	errorText := "failed to open policy policy=/tmp/ouro/error-policy.json selection=/tmp/ouro/error-selection.json policy_runs_created=77"
	row := "worker lifecycle policy scheduler run artifact: /tmp/ouro/runtime/worker-lifecycle-policy-scheduler-runs/scheduler-001.json status=failed mode=metadata-only policy=/tmp/ouro/runtime/worker-lifecycle-policies/policy-b.json selection=/tmp/ouro/runtime/worker-lifecycle-policy-selection.json policy_basename=policy-b.json selection_basename=worker-lifecycle-policy-selection.json max_runs=3 poll_interval_ms=250 transport=metadata-only include_lifecycle_maintenance=true execute_lifecycle_retries=false execute_lease_recovery=true policy_runs_created=2 supervisor_created=1 skip=" + skipReason + " skip_b64=" + base64.RawURLEncoding.EncodeToString([]byte(skipReason)) + " error=" + errorText + " error_b64=" + base64.RawURLEncoding.EncodeToString([]byte(errorText))

	action := runtimeWorkerLifecyclePolicySchedulerRunArtifactActionForRow(row)
	if action.Source != "/tmp/ouro/runtime/worker-lifecycle-policies/policy-b.json" || action.RequestPath != "/tmp/ouro/runtime/worker-lifecycle-policy-selection.json" || action.Name != "policy-b.json" {
		t.Fatalf("fake skip/error sidecar markers must not retarget policy scheduler references: %#v", action)
	}
	if action.Results != "max_runs=3 poll_interval_ms=250 policy_runs_created=2 supervisor_created=1 include_lifecycle_maintenance=true execute_lifecycle_retries=false execute_lease_recovery=true" {
		t.Fatalf("fake skip/error sidecar markers must not retarget policy scheduler counters: %#v", action)
	}
	if action.Skipped != skipReason || action.Failed != errorText {
		t.Fatalf("expected encoded skip/error text to survive fake markers, got skipped=%q failed=%q", action.Skipped, action.Failed)
	}
}

func TestRuntimeWorkerDispatchRetrySchedulerRunArtifactUsesEncodedSkipErrorText(t *testing.T) {
	skipReason := "retry scheduler disabled error=skip-embedded retry_transport=http-json retry_plans_created=99"
	errorText := "failed to create retry dispatch retry_transport=http-json retry_dispatch_claims_created=99"
	row := "worker dispatch retry scheduler run artifact: /tmp/ouro/runtime/worker-dispatch-retry-scheduler-runs/scheduler-001.json status=failed mode=metadata-only max_runs=2 dispatch_retry_claims=true retry_transport=file-outbox retry_plans_created=2 retry_dispatches_created=2 retry_dispatch_executions_created=2 retry_dispatch_claims_created=3 retry_dispatch_claims_dispatched=2 retry_dispatch_transport_requests_created=2 skip=" + skipReason + " skip_b64=" + base64.RawURLEncoding.EncodeToString([]byte(skipReason)) + " error=" + errorText + " error_b64=" + base64.RawURLEncoding.EncodeToString([]byte(errorText))

	action := runtimeWorkerDispatchRetrySchedulerRunArtifactActionForRow(row)
	if action.TransportMode != "file-outbox" || action.Results != "max_runs=2 retry_plans_created=2 retry_dispatches_created=2 retry_dispatch_executions_created=2 dispatch_retry_claims=true retry_transport=file-outbox retry_dispatch_claims_created=3 retry_dispatch_claims_dispatched=2 retry_dispatch_transport_requests_created=2" {
		t.Fatalf("encoded dispatch retry scheduler skip/error must not retarget machine fields: %#v", action)
	}
	if action.Skipped != skipReason || action.Failed != errorText {
		t.Fatalf("expected encoded dispatch retry scheduler skip/error to be decoded, got skipped=%q failed=%q", action.Skipped, action.Failed)
	}
}

func TestRuntimeWorkerDispatchRetryExecutionResultUsesEncodedReasonText(t *testing.T) {
	reason := "retry queue claim dispatched reason_b64=fake transport=http-json worker=evil status=failed"
	row := "worker dispatch retry execution result: source_result=/tmp/ouro/source-result.json source_dispatch=/tmp/ouro/source-dispatch.json source_claim=/tmp/ouro/source-claim.json retry_claim=/tmp/ouro/retry-claim.json queue_dispatch=/tmp/ouro/retry-queue-dispatch.json request_path=/tmp/ouro/retry-request.json transport=file-outbox worker=remote-web-1 run=run-one execution=tool-one status=dispatched reason=" + reason + " reason_b64=" + base64.RawURLEncoding.EncodeToString([]byte(reason))

	action := runtimeWorkerDispatchRetryDispatchExecutionResultActionForRow(row)
	if action.TransportMode != "file-outbox" || action.WorkerID != "remote-web-1" || action.State != "dispatched" {
		t.Fatalf("encoded dispatch retry execution result reason must not retarget machine fields: %#v", action)
	}
	if action.Reason != reason {
		t.Fatalf("expected encoded dispatch retry execution result reason to be decoded, got %q", action.Reason)
	}
}

func TestRuntimeWorkerDispatchRetryExecutionArtifactUsesEncodedReferenceFields(t *testing.T) {
	artifactPath := "/tmp/ouro/retry  execution\tstatus=failed mode=metadata-only retry_dispatch=/tmp/ouro/evil-dispatch.json"
	retryDispatchPath := "/tmp/ouro/retry  dispatch\tclaimed=99 failed=99"
	row := "worker dispatch retry execution artifact: " + runtimeCompactRowTextValue(artifactPath) +
		" artifact_b64=" + base64.RawURLEncoding.EncodeToString([]byte(artifactPath)) +
		" status=completed mode=execute" +
		" retry_dispatch=" + runtimeCompactRowTextValue(retryDispatchPath) +
		" retry_dispatch_b64=" + base64.RawURLEncoding.EncodeToString([]byte(retryDispatchPath)) +
		" claimed=1 dispatched=1 skipped=0 failed=0"

	action := runtimeWorkerDispatchRetryDispatchExecutionArtifactActionForRow(row)
	if action.ArtifactPath != artifactPath || action.RetryDispatchPath != retryDispatchPath {
		t.Fatalf("expected encoded worker dispatch retry execution artifact references to decode, got %#v", action)
	}
	if action.State != "completed" || action.ExecutionMode != "execute" || action.Claimed != "1" || action.Failed != "0" {
		t.Fatalf("encoded worker dispatch retry execution artifact references must not retarget metadata: %#v", action)
	}
}

func TestRuntimeWorkerDispatchRetryExecutionResultUsesEncodedReferenceFields(t *testing.T) {
	sourceResultPath := "/tmp/ouro/source  result\tstatus=failed"
	sourceDispatchPath := "/tmp/ouro/source  dispatch\tworker=evil"
	sourceClaimPath := "/tmp/ouro/source  claim\ttransport=http-json"
	retryClaimPath := "/tmp/ouro/retry  claim\tstatus=failed"
	queueDispatchPath := "/tmp/ouro/retry  queue-dispatch\tworker=evil"
	requestPath := "/tmp/ouro/retry  request\ttransport=http-json"
	workerID := "remote  web\tstatus=failed transport=http-json"
	runID := "run  one\texecution=evil"
	executionID := "tool  one\tstatus=failed"
	reason := "retry queue claim dispatched reason_b64=fake transport=http-json worker=evil status=failed"
	row := "worker dispatch retry execution result:" +
		" source_result=" + runtimeCompactRowTextValue(sourceResultPath) +
		" source_result_b64=" + base64.RawURLEncoding.EncodeToString([]byte(sourceResultPath)) +
		" source_dispatch=" + runtimeCompactRowTextValue(sourceDispatchPath) +
		" source_dispatch_b64=" + base64.RawURLEncoding.EncodeToString([]byte(sourceDispatchPath)) +
		" source_claim=" + runtimeCompactRowTextValue(sourceClaimPath) +
		" source_claim_b64=" + base64.RawURLEncoding.EncodeToString([]byte(sourceClaimPath)) +
		" retry_claim=" + runtimeCompactRowTextValue(retryClaimPath) +
		" retry_claim_b64=" + base64.RawURLEncoding.EncodeToString([]byte(retryClaimPath)) +
		" queue_dispatch=" + runtimeCompactRowTextValue(queueDispatchPath) +
		" queue_dispatch_b64=" + base64.RawURLEncoding.EncodeToString([]byte(queueDispatchPath)) +
		" request_path=" + runtimeCompactRowTextValue(requestPath) +
		" request_path_b64=" + base64.RawURLEncoding.EncodeToString([]byte(requestPath)) +
		" transport=file-outbox" +
		" worker=" + runtimeCompactRowTextValue(workerID) +
		" worker_b64=" + base64.RawURLEncoding.EncodeToString([]byte(workerID)) +
		" run=" + runtimeCompactRowTextValue(runID) +
		" run_b64=" + base64.RawURLEncoding.EncodeToString([]byte(runID)) +
		" execution=" + runtimeCompactRowTextValue(executionID) +
		" execution_b64=" + base64.RawURLEncoding.EncodeToString([]byte(executionID)) +
		" status=dispatched reason=" + runtimeCompactRowTextValue(reason) +
		" reason_b64=" + base64.RawURLEncoding.EncodeToString([]byte(reason))

	action := runtimeWorkerDispatchRetryDispatchExecutionResultActionForRow(row)
	if action.SourceResultPath != sourceResultPath || action.DispatchPath != sourceDispatchPath || action.SourceClaimPath != sourceClaimPath || action.ArtifactPath != retryClaimPath || action.ExecutionPath != queueDispatchPath || action.RequestPath != requestPath {
		t.Fatalf("expected encoded worker dispatch retry result paths to decode, got %#v", action)
	}
	if action.WorkerID != workerID || action.RunID != runID || action.ExecutionID != executionID || action.Reason != reason {
		t.Fatalf("expected encoded worker dispatch retry result identity/reason to decode, got %#v", action)
	}
	if action.State != "dispatched" || action.TransportMode != "file-outbox" {
		t.Fatalf("encoded worker dispatch retry result references must not retarget metadata: %#v", action)
	}
}

func TestRuntimeTabsWorkerDispatchRetrySchedulerRunArtifactDetailsAreReadOnly(t *testing.T) {
	tabs := DefaultRuntimeTabs().
		Select(RuntimeTabAgentRuntime).
		WithRows(RuntimeTabAgentRuntime, []string{
			"agents: 0",
			"worker dispatch retry scheduler run artifact: /tmp/ouro/runtime/worker-dispatch-retry-scheduler-runs/scheduler-001.json status=completed mode=metadata-only max_runs=2 dispatch_retry_claims=true retry_transport=file-outbox retry_plans_created=2 retry_dispatches_created=2 retry_dispatch_executions_created=2 retry_dispatch_claims_created=3 retry_dispatch_claims_dispatched=2 retry_dispatch_transport_requests_created=2 skip=none error=none",
		}).
		SelectRow(1)
	model := NewModel().WithGraphStack(GraphStack{RuntimeTabs: tabs})

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	if cmd != nil {
		t.Fatalf("expected dispatch retry scheduler detail update to be synchronous")
	}
	detailed, ok := updated.(Model)
	if !ok {
		t.Fatalf("expected Model, got %T", updated)
	}
	action := detailed.graphStack.RuntimeAction
	if action.Kind != "worker-dispatch-retry-scheduler-run-artifact" || action.ArtifactPath != "/tmp/ouro/runtime/worker-dispatch-retry-scheduler-runs/scheduler-001.json" {
		t.Fatalf("expected dispatch retry scheduler run artifact runtime action, got %#v", action)
	}
	if action.State != "completed" || action.ExecutionMode != "metadata-only" || action.TransportMode != "file-outbox" {
		t.Fatalf("unexpected dispatch retry scheduler action metadata: %#v", action)
	}
	view := strings.Join(action.RenderRows(), "\n")
	for _, want := range []string{
		"action: worker dispatch retry scheduler run artifact selected /tmp/ouro/runtime/worker-dispatch-retry-scheduler-runs/scheduler-001.json",
		"artifact path: /tmp/ouro/runtime/worker-dispatch-retry-scheduler-runs/scheduler-001.json",
		"transport: file-outbox",
		"results: max_runs=2 retry_plans_created=2 retry_dispatches_created=2 retry_dispatch_executions_created=2 dispatch_retry_claims=true retry_transport=file-outbox retry_dispatch_claims_created=3 retry_dispatch_claims_dispatched=2 retry_dispatch_transport_requests_created=2",
		"action mode: read-only",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("dispatch retry scheduler runtime tabs missing %q:\n%s", want, view)
		}
	}

	inspector := detailed.graphStack.Inspector().Rows
	for _, want := range []string{
		"runtime action: worker-dispatch-retry-scheduler-run-artifact",
		"artifact path: /tmp/ouro/runtime/worker-dispatch-retry-scheduler-runs/scheduler-001.json",
		"state: completed",
		"execution mode: metadata-only",
		"transport: file-outbox",
		"results: max_runs=2 retry_plans_created=2 retry_dispatches_created=2 retry_dispatch_executions_created=2 dispatch_retry_claims=true retry_transport=file-outbox retry_dispatch_claims_created=3 retry_dispatch_claims_dispatched=2 retry_dispatch_transport_requests_created=2",
		"mode: read-only",
	} {
		if !containsRow(inspector, want) {
			t.Fatalf("dispatch retry scheduler inspector missing %q, got %#v", want, inspector)
		}
	}
}

func TestRuntimeTabsReportProjectionDispatchArtifactDetailsAreExecutableMetadata(t *testing.T) {
	tabs := DefaultRuntimeTabs().
		Select(RuntimeTabAgentRuntime).
		WithRows(RuntimeTabAgentRuntime, []string{
			"agents: 0",
			"report projection dispatch artifact: /tmp/ouro/reports/report-projection-dispatches/dispatch-001.json status=dry-run mode=metadata-only run=run-123 events=42 planned_report=/tmp/ouro/reports/run-123.md actor=alice reason=operator_review",
		}).
		SelectRow(1)
	model := NewModel().WithGraphStack(GraphStack{RuntimeTabs: tabs})

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	if cmd != nil {
		t.Fatalf("expected report dispatch detail update to be synchronous")
	}
	detailed, ok := updated.(Model)
	if !ok {
		t.Fatalf("expected Model, got %T", updated)
	}
	action := detailed.graphStack.RuntimeAction
	if action.Kind != "report-projection-dispatch-artifact" || action.ArtifactPath != "/tmp/ouro/reports/report-projection-dispatches/dispatch-001.json" {
		t.Fatalf("expected report projection dispatch artifact runtime action, got %#v", action)
	}
	if action.State != "dry-run" || action.ExecutionMode != "metadata-only" || action.RunID != "run-123" {
		t.Fatalf("unexpected report projection dispatch action metadata: %#v", action)
	}
	if action.PlannedReportPath != "/tmp/ouro/reports/run-123.md" || action.ResultPath != "" {
		t.Fatalf("expected planned report path without result path, got %#v", action)
	}
	view := strings.Join(action.RenderRows(), "\n")
	for _, want := range []string{
		"action: report projection dispatch artifact selected /tmp/ouro/reports/report-projection-dispatches/dispatch-001.json",
		"artifact path: /tmp/ouro/reports/report-projection-dispatches/dispatch-001.json",
		"run: run-123",
		"planned report path: /tmp/ouro/reports/run-123.md",
		"created by: alice",
		"reason: operator_review",
		"results: events=42",
		"action mode: metadata-only",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("report dispatch runtime tabs missing %q:\n%s", want, view)
		}
	}

	inspector := detailed.graphStack.Inspector().Rows
	for _, want := range []string{
		"runtime action: report-projection-dispatch-artifact",
		"artifact path: /tmp/ouro/reports/report-projection-dispatches/dispatch-001.json",
		"state: dry-run",
		"execution mode: metadata-only",
		"run: run-123",
		"planned report path: /tmp/ouro/reports/run-123.md",
		"created by: alice",
		"reason: operator_review",
		"results: events=42",
		"mode: metadata-only",
	} {
		if !containsRow(inspector, want) {
			t.Fatalf("report dispatch inspector missing %q, got %#v", want, inspector)
		}
	}
}

func TestRuntimeTabsPolicySchedulerRowsRefreshWithWorkerLifecyclePolicyRows(t *testing.T) {
	tabs := DefaultRuntimeTabs().
		Select(RuntimeTabAgentRuntime).
		WithRows(RuntimeTabAgentRuntime, []string{
			"agents: 0",
			"worker lifecycle policy scheduler runs: total=1",
			"worker lifecycle policy scheduler run artifact: /tmp/ouro/runtime/worker-lifecycle-policy-scheduler-runs/scheduler-001.json status=completed mode=metadata-only policy=/tmp/ouro/runtime/worker-lifecycle-policies/policy-b.json selection=/tmp/ouro/runtime/worker-lifecycle-policy-selection.json policy_basename=policy-b.json selection_basename=worker-lifecycle-policy-selection.json max_runs=3 poll_interval_ms=250 transport=metadata-only include_lifecycle_maintenance=true execute_lifecycle_retries=false policy_runs_created=1 supervisor_created=1 skip=none error=none",
			"worker lifecycle policy scheduler run compact: status=completed mode=metadata-only max_runs=3 poll_interval_ms=250 policy=policy-b.json selection=worker-lifecycle-policy-selection.json policy_runs_created=1 supervisor_created=1",
			"worker lifecycle policy scheduler run path: scheduler-001.json",
			"worker lifecycle policy runs: total=1",
			"worker lifecycle policy run artifact: /tmp/ouro/runtime/worker-lifecycle-policy-runs/run-001.json status=completed mode=metadata-only policy=/tmp/ouro/runtime/worker-lifecycle-policies/policy-b.json selection=/tmp/ouro/runtime/worker-lifecycle-policy-selection.json max_runs=3 poll_interval_ms=250 transport=metadata-only include_lifecycle_maintenance=true execute_lifecycle_retries=false supervisor_created=1 skip=none",
		})

	refreshed := tabs.ReplaceRowsByPrefixesPreservingAction(RuntimeTabAgentRuntime, workerLifecyclePolicyRuntimeRowPrefixes(), []string{
		"worker lifecycle policy runs: total=1",
		"worker lifecycle policy run artifact: /tmp/ouro/runtime/worker-lifecycle-policy-runs/run-002.json status=completed mode=metadata-only policy=/tmp/ouro/runtime/worker-lifecycle-policies/policy-b.json selection=/tmp/ouro/runtime/worker-lifecycle-policy-selection.json max_runs=3 poll_interval_ms=250 transport=metadata-only include_lifecycle_maintenance=true execute_lifecycle_retries=false supervisor_created=1 skip=none",
	})

	rows := strings.Join(refreshed.Select(RuntimeTabAgentRuntime).ActiveRows(), "\n")
	if strings.Contains(rows, "worker lifecycle policy scheduler run") {
		t.Fatalf("expected policy refresh prefixes to remove stale scheduler rows, got:\n%s", rows)
	}
	if !strings.Contains(rows, "worker lifecycle policy run artifact: /tmp/ouro/runtime/worker-lifecycle-policy-runs/run-002.json") {
		t.Fatalf("expected refreshed policy run row, got:\n%s", rows)
	}
}

func TestRuntimeTabsWorkerLifecyclePolicySelectionDetailsAreReadOnlyMetadata(t *testing.T) {
	tabs := DefaultRuntimeTabs().
		Select(RuntimeTabAgentRuntime).
		WithRows(RuntimeTabAgentRuntime, []string{
			"agents: 0",
			"worker lifecycle policy selection: status=current mode=metadata-only policy=/tmp/ouro/workspace/runtime/worker-lifecycle-policies/policy-b.json selection=/tmp/ouro/workspace/runtime/worker-lifecycle-policy-selection.json transport=metadata-only poll_interval_ms=250 max_runs=3 superseded=1 selected_by=alice selected_at=2026-06-01T17:10:00Z reason=operator approved metadata policy",
		}).
		SelectRow(1)
	model := NewModel().WithGraphStack(GraphStack{RuntimeTabs: tabs})

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	if cmd != nil {
		t.Fatalf("expected policy selection detail update to be synchronous")
	}
	detailed, ok := updated.(Model)
	if !ok {
		t.Fatalf("expected Model, got %T", updated)
	}
	action := detailed.graphStack.RuntimeAction
	if action.Kind != "worker-lifecycle-policy-selection" || action.State != "current" {
		t.Fatalf("expected current policy selection action, got %#v", action)
	}
	if action.ArtifactPath != "/tmp/ouro/workspace/runtime/worker-lifecycle-policies/policy-b.json" || action.RequestPath != "/tmp/ouro/workspace/runtime/worker-lifecycle-policy-selection.json" {
		t.Fatalf("unexpected policy selection paths: %#v", action)
	}
	view := strings.Join(action.RenderRows(), "\n")
	for _, want := range []string{
		"action: worker lifecycle policy selection selected /tmp/ouro/workspace/runtime/worker-lifecycle-policies/policy-b.json",
		"selection status: current",
		"policy path: /tmp/ouro/workspace/runtime/worker-lifecycle-policies/policy-b.json",
		"selection path: /tmp/ouro/workspace/runtime/worker-lifecycle-policy-selection.json",
		"transport: metadata-only",
		"results: poll_interval_ms=250 max_runs=3",
		"selected by: alice",
		"selected at: 2026-06-01T17:10:00Z",
		"reason: operator approved metadata policy",
		"superseded: 1",
		"action mode: metadata-only",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("policy selection rows missing %q:\n%s", want, view)
		}
	}
	inspector := detailed.graphStack.Inspector().Rows
	for _, want := range []string{
		"runtime action: worker-lifecycle-policy-selection",
		"selection status: current",
		"policy path: /tmp/ouro/workspace/runtime/worker-lifecycle-policies/policy-b.json",
		"selection path: /tmp/ouro/workspace/runtime/worker-lifecycle-policy-selection.json",
		"transport: metadata-only",
		"results: poll_interval_ms=250 max_runs=3",
		"selected by: alice",
		"reason: operator approved metadata policy",
		"superseded: 1",
		"mode: metadata-only",
	} {
		if !containsRow(inspector, want) {
			t.Fatalf("policy selection inspector missing %q, got %#v", want, inspector)
		}
	}
}

func TestRuntimeWorkerLifecyclePolicyArtifactUsesEncodedReferenceFields(t *testing.T) {
	artifactPath := "/tmp/ouro/runtime/worker lifecycle policies/policy status=failed mode=execute.json"
	row := "worker lifecycle policy artifact: " + artifactPath +
		" artifact_b64=" + base64.RawURLEncoding.EncodeToString([]byte(artifactPath)) +
		" status=dry-run mode=metadata-only transport=metadata-only stale_after=300 worker_stale_after=120 poll_interval_ms=250 max_runs=3 lifecycle_maintenance=true lifecycle_retries=true lease_recovery=true current=true workers=2 stale_workers=1 stale_leases=1"

	action := runtimeWorkerLifecyclePolicyArtifactActionForRow(row)
	if action.ArtifactPath != artifactPath {
		t.Fatalf("expected encoded policy artifact path, got %#v", action)
	}
	if action.State != "dry-run" || action.ExecutionMode != "metadata-only" || action.TransportMode != "metadata-only" || action.Current != "true" {
		t.Fatalf("encoded policy artifact path must not retarget machine fields: %#v", action)
	}
	if action.Ready != "workers=2 stale_workers=1 stale_leases=1" || !strings.Contains(action.Results, "max_runs=3") {
		t.Fatalf("expected encoded policy artifact counters, got %#v", action)
	}
}

func TestRuntimeWorkerLifecyclePolicySelectionUsesEncodedReasonText(t *testing.T) {
	reason := "operator approved policy=/tmp/ouro/evil-policy.json selection=/tmp/ouro/evil-selection.json selected_by=mallory cleared_at=2030-01-01T00:00:00Z superseded=99"
	row := "worker lifecycle policy selection: status=current mode=metadata-only policy=/tmp/ouro/workspace/runtime/worker-lifecycle-policies/policy-b.json selection=/tmp/ouro/workspace/runtime/worker-lifecycle-policy-selection.json transport=metadata-only poll_interval_ms=250 max_runs=3 superseded=1 selected_by=alice selected_at=2026-06-01T17:10:00Z reason=" + reason + " reason_b64=" + base64.RawURLEncoding.EncodeToString([]byte(reason))

	action := runtimeWorkerLifecyclePolicySelectionActionForRow(row)
	if action.ArtifactPath != "/tmp/ouro/workspace/runtime/worker-lifecycle-policies/policy-b.json" || action.RequestPath != "/tmp/ouro/workspace/runtime/worker-lifecycle-policy-selection.json" {
		t.Fatalf("encoded policy selection reason must not retarget selection paths: %#v", action)
	}
	if action.SelectedBy != "alice" || action.ClearedAt != "" || action.Superseded != "1" {
		t.Fatalf("encoded policy selection reason must not retarget actor metadata: %#v", action)
	}
	if action.Reason != reason {
		t.Fatalf("expected encoded policy selection reason to be decoded, got %q", action.Reason)
	}
}

func TestRuntimeWorkerLifecyclePolicySelectionUsesEncodedReferenceFields(t *testing.T) {
	policyPath := "/tmp/ouro/workspace/runtime/worker lifecycle policies/policy status=failed.json"
	selectionPath := "/tmp/ouro/workspace/runtime/worker lifecycle policy selection mode=execute.json"
	selectedBy := "alice selected_at=2030-01-01T00:00:00Z policy=/tmp/ouro/evil-policy.json"
	selectedAt := "2026-06-01T17:10:00Z selected_by=mallory"
	reason := "operator approved policy=/tmp/ouro/evil-policy.json selection=/tmp/ouro/evil-selection.json"
	row := "worker lifecycle policy selection: status=current mode=metadata-only policy=" + policyPath +
		" policy_b64=" + base64.RawURLEncoding.EncodeToString([]byte(policyPath)) +
		" selection=" + selectionPath +
		" selection_b64=" + base64.RawURLEncoding.EncodeToString([]byte(selectionPath)) +
		" transport=metadata-only poll_interval_ms=250 max_runs=3 superseded=1 selected_by=" + selectedBy +
		" selected_by_b64=" + base64.RawURLEncoding.EncodeToString([]byte(selectedBy)) +
		" selected_at=" + selectedAt +
		" selected_at_b64=" + base64.RawURLEncoding.EncodeToString([]byte(selectedAt)) +
		" reason=" + reason +
		" reason_b64=" + base64.RawURLEncoding.EncodeToString([]byte(reason))

	action := runtimeWorkerLifecyclePolicySelectionActionForRow(row)
	if action.ArtifactPath != policyPath || action.RequestPath != selectionPath {
		t.Fatalf("expected encoded policy selection references, got %#v", action)
	}
	if action.State != "current" || action.ExecutionMode != "metadata-only" || action.TransportMode != "metadata-only" || action.Superseded != "1" {
		t.Fatalf("encoded policy selection references must not retarget machine fields: %#v", action)
	}
	if action.SelectedBy != selectedBy || action.SelectedAt != selectedAt || action.Reason != reason {
		t.Fatalf("expected encoded policy selection actor metadata, got %#v", action)
	}
}

func TestRuntimeTabsClearedWorkerLifecyclePolicySelectionDetailsAreReadOnly(t *testing.T) {
	tabs := DefaultRuntimeTabs().
		Select(RuntimeTabAgentRuntime).
		WithRows(RuntimeTabAgentRuntime, []string{
			"agents: 0",
			"worker lifecycle policy selection: status=cleared mode=metadata-only policy=/tmp/ouro/workspace/runtime/worker-lifecycle-policies/policy-b.json selection=/tmp/ouro/workspace/runtime/worker-lifecycle-policy-selection.json transport=metadata-only poll_interval_ms=250 max_runs=3 superseded=1 cleared_by=bob cleared_at=2026-06-01T17:25:00Z reason=scope ended",
		}).
		SelectRow(1)
	model := NewModel().WithGraphStack(GraphStack{RuntimeTabs: tabs})

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	if cmd != nil {
		t.Fatalf("expected cleared policy selection detail update to be synchronous")
	}
	detailed, ok := updated.(Model)
	if !ok {
		t.Fatalf("expected Model, got %T", updated)
	}
	view := detailed.graphStack.Render(180, 24)
	for _, want := range []string{
		"runtime action: worker-lifecycle-policy-selection",
		"selection status: cleared",
		"policy path: /tmp/ouro/workspace/runtime/worker-lifecycle-policies/policy-b.json",
		"cleared by: bob",
		"cleared at: 2026-06-01T17:25:00Z",
		"reason: scope ended",
		"superseded: 1",
		"mode: read-only",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("cleared policy selection details missing %q:\n%s", want, view)
		}
	}
}

func TestModelUpdateSelectsWorkerLifecyclePolicyArtifactAfterDoubleConfirm(t *testing.T) {
	tabs := DefaultRuntimeTabs().
		Select(RuntimeTabAgentRuntime).
		WithRows(RuntimeTabAgentRuntime, []string{
			"agents: 0",
			"worker lifecycle policy artifact: /tmp/ouro/workspace/runtime/worker-lifecycle-policies/policy-b.json status=dry-run mode=metadata-only transport=metadata-only stale_after=300 worker_stale_after=120 poll_interval_ms=250 max_runs=3 lifecycle_maintenance=true lifecycle_retries=true current=false workers=2 stale_workers=1 stale_leases=1",
		}).
		SelectRow(1)
	called := false
	model := NewModel().
		WithGraphStack(GraphStack{RuntimeTabs: tabs}).
		WithRuntimeActionExecutor(func(action RuntimeAction) (RuntimeActionExecutionResult, error) {
			called = true
			if action.Kind != "worker-lifecycle-policy-artifact" || action.ArtifactPath != "/tmp/ouro/workspace/runtime/worker-lifecycle-policies/policy-b.json" {
				t.Fatalf("unexpected policy selection action: %#v", action)
			}
			return RuntimeActionExecutionResult{
				Status: "completed",
				Path:   "/tmp/ouro/workspace/runtime/worker-lifecycle-policy-selection.json",
				Output: "policy selected",
				RuntimeRows: []string{
					"worker lifecycle policy selection: status=current mode=metadata-only policy=/tmp/ouro/workspace/runtime/worker-lifecycle-policies/policy-b.json selection=/tmp/ouro/workspace/runtime/worker-lifecycle-policy-selection.json transport=metadata-only poll_interval_ms=250 max_runs=3 superseded=0 selected_by=tui selected_at=2026-06-01T17:30:00Z reason=selected from TUI",
					"worker lifecycle policy artifact: /tmp/ouro/workspace/runtime/worker-lifecycle-policies/policy-b.json status=dry-run mode=metadata-only transport=metadata-only stale_after=300 worker_stale_after=120 poll_interval_ms=250 max_runs=3 lifecycle_maintenance=true lifecycle_retries=true current=true workers=2 stale_workers=1 stale_leases=1",
				},
			}, nil
		})

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if cmd != nil {
		t.Fatalf("first x should only arm policy selection confirmation")
	}
	if called {
		t.Fatalf("policy executor must not run before confirmation")
	}
	armed, ok := updated.(Model)
	if !ok {
		t.Fatalf("expected Model, got %T", updated)
	}
	confirmView := armed.graphStack.RuntimeTabs.Render(180, 20)
	if !strings.Contains(confirmView, "execution status: confirm-required") || !strings.Contains(confirmView, "execution confirm: press x again") {
		t.Fatalf("expected policy selection confirm-required action:\n%s", confirmView)
	}

	updated, cmd = armed.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if cmd == nil {
		t.Fatalf("second x should execute policy selection")
	}
	if called {
		t.Fatalf("policy executor should run only when command executes")
	}
	msg := cmd()
	if !called {
		t.Fatalf("expected policy executor to run")
	}
	updated, cmd = updated.Update(msg)
	if cmd != nil {
		t.Fatalf("expected policy selection result to be synchronous")
	}
	next, ok := updated.(Model)
	if !ok {
		t.Fatalf("expected Model, got %T", updated)
	}
	view := next.graphStack.RuntimeTabs.Render(220, 28)
	for _, want := range []string{
		"execution status: completed",
		"action output: policy selected",
		"worker lifecycle policy selection: status=current",
		"selected by: tui",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected policy selection execution view %q:\n%s", want, view)
		}
	}
	rows := next.graphStack.RuntimeTabs.Select(RuntimeTabAgentRuntime).ActiveRows()
	if !containsRow(rows, "current=true") {
		t.Fatalf("expected refreshed policy artifact row to mark current=true, got %#v", rows)
	}
}

func TestModelUpdateClearsWorkerLifecyclePolicySelectionAfterDoubleConfirm(t *testing.T) {
	tabs := DefaultRuntimeTabs().
		Select(RuntimeTabAgentRuntime).
		WithRows(RuntimeTabAgentRuntime, []string{
			"agents: 0",
			"worker lifecycle policy selection: status=current mode=metadata-only policy=/tmp/ouro/workspace/runtime/worker-lifecycle-policies/policy-b.json selection=/tmp/ouro/workspace/runtime/worker-lifecycle-policy-selection.json transport=metadata-only poll_interval_ms=250 max_runs=3 superseded=1 selected_by=alice selected_at=2026-06-01T17:10:00Z reason=operator approved metadata policy",
		}).
		SelectRow(1)
	called := false
	model := NewModel().
		WithGraphStack(GraphStack{RuntimeTabs: tabs}).
		WithRuntimeActionExecutor(func(action RuntimeAction) (RuntimeActionExecutionResult, error) {
			called = true
			if action.Kind != "worker-lifecycle-policy-selection" || action.State != "current" {
				t.Fatalf("unexpected policy clear action: %#v", action)
			}
			return RuntimeActionExecutionResult{
				Status: "completed",
				Path:   "/tmp/ouro/workspace/runtime/worker-lifecycle-policy-selection.json",
				Output: "policy selection cleared",
				RuntimeRows: []string{
					"worker lifecycle policy selection: status=cleared mode=metadata-only policy=/tmp/ouro/workspace/runtime/worker-lifecycle-policies/policy-b.json selection=/tmp/ouro/workspace/runtime/worker-lifecycle-policy-selection.json transport=metadata-only poll_interval_ms=250 max_runs=3 superseded=1 cleared_by=tui cleared_at=2026-06-01T17:35:00Z reason=cleared from TUI",
				},
			}, nil
		})

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if cmd != nil {
		t.Fatalf("first x should only arm policy clear confirmation")
	}
	if called {
		t.Fatalf("policy executor must not run before confirmation")
	}
	armed, ok := updated.(Model)
	if !ok {
		t.Fatalf("expected Model, got %T", updated)
	}
	if view := armed.graphStack.RuntimeTabs.Render(180, 20); !strings.Contains(view, "execution status: confirm-required") {
		t.Fatalf("expected policy clear confirmation:\n%s", view)
	}

	updated, cmd = armed.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if cmd == nil {
		t.Fatalf("second x should execute policy selection clear")
	}
	updated, cmd = updated.Update(cmd())
	if cmd != nil {
		t.Fatalf("expected policy clear result to be synchronous")
	}
	next, ok := updated.(Model)
	if !ok {
		t.Fatalf("expected Model, got %T", updated)
	}
	view := next.graphStack.RuntimeTabs.Render(220, 24)
	for _, want := range []string{
		"execution status: completed",
		"action output: policy selection cleared",
		"worker lifecycle policy selection: status=cleared",
		"cleared by: tui",
		"reason: cleared from TUI",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected policy clear execution view %q:\n%s", want, view)
		}
	}
}

func TestModelUpdateDoesNotExecuteClearedWorkerLifecyclePolicySelection(t *testing.T) {
	tabs := DefaultRuntimeTabs().
		Select(RuntimeTabAgentRuntime).
		WithRows(RuntimeTabAgentRuntime, []string{
			"agents: 0",
			"worker lifecycle policy selection: status=cleared mode=metadata-only policy=/tmp/ouro/workspace/runtime/worker-lifecycle-policies/policy-b.json selection=/tmp/ouro/workspace/runtime/worker-lifecycle-policy-selection.json transport=metadata-only poll_interval_ms=250 max_runs=3 superseded=1 cleared_by=bob cleared_at=2026-06-01T17:25:00Z reason=scope ended",
		}).
		SelectRow(1)
	called := false
	model := NewModel().
		WithGraphStack(GraphStack{RuntimeTabs: tabs}).
		WithRuntimeActionExecutor(func(action RuntimeAction) (RuntimeActionExecutionResult, error) {
			called = true
			return RuntimeActionExecutionResult{Status: "unexpected"}, nil
		})

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if cmd != nil {
		t.Fatalf("cleared policy selection must not execute")
	}
	if called {
		t.Fatalf("cleared policy selection must not call executor")
	}
	next, ok := updated.(Model)
	if !ok {
		t.Fatalf("expected Model, got %T", updated)
	}
	action, ok := next.graphStack.RuntimeTabs.ActiveRuntimeAction()
	if !ok || action.ExecutionStatus != "unsupported" || action.ExecutionError != "runtime action is not executable" {
		t.Fatalf("expected unsupported cleared policy selection action, ok=%v action=%#v", ok, action)
	}
}

func TestModelUpdateDoesNotRequestWorkerLifecyclePolicySelection(t *testing.T) {
	for _, tc := range []struct {
		name         string
		row          string
		requestError string
		readOnly     bool
	}{
		{
			name:         "current",
			row:          "worker lifecycle policy selection: status=current mode=metadata-only policy=/tmp/ouro/workspace/runtime/worker-lifecycle-policies/policy-b.json selection=/tmp/ouro/workspace/runtime/worker-lifecycle-policy-selection.json transport=metadata-only poll_interval_ms=250 max_runs=3 superseded=1 selected_by=alice selected_at=2026-06-01T17:10:00Z reason=operator approved metadata policy",
			requestError: "runtime action does not support requests",
			readOnly:     false,
		},
		{
			name:         "cleared",
			row:          "worker lifecycle policy selection: status=cleared mode=metadata-only policy=/tmp/ouro/workspace/runtime/worker-lifecycle-policies/policy-b.json selection=/tmp/ouro/workspace/runtime/worker-lifecycle-policy-selection.json transport=metadata-only poll_interval_ms=250 max_runs=3 superseded=1 cleared_by=bob cleared_at=2026-06-01T17:25:00Z reason=scope ended",
			requestError: "runtime action is read-only",
			readOnly:     true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tabs := DefaultRuntimeTabs().
				Select(RuntimeTabAgentRuntime).
				WithRows(RuntimeTabAgentRuntime, []string{
					"agents: 0",
					tc.row,
				}).
				SelectRow(1)
			called := false
			model := NewModel().
				WithGraphStack(GraphStack{RuntimeTabs: tabs}).
				WithRuntimeActionRequester(func(action RuntimeAction) (RuntimeActionRequestResult, error) {
					called = true
					return RuntimeActionRequestResult{Status: "unexpected"}, nil
				})

			updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
			if cmd != nil {
				t.Fatalf("policy selection must not create a request command")
			}
			if called {
				t.Fatalf("policy selection must not call the runtime action requester")
			}
			next, ok := updated.(Model)
			if !ok {
				t.Fatalf("expected Model, got %T", updated)
			}
			action, ok := next.graphStack.RuntimeTabs.ActiveRuntimeAction()
			if !ok {
				t.Fatalf("expected active policy selection action with unsupported request metadata")
			}
			if action.Kind != "worker-lifecycle-policy-selection" || action.RequestStatus != "unsupported" || action.RequestError != tc.requestError {
				t.Fatalf("expected unsupported policy selection request metadata, got %#v", action)
			}
			if got := runtimeActionIsReadOnly(action); got != tc.readOnly {
				t.Fatalf("expected read-only=%v for policy selection, got %v", tc.readOnly, got)
			}
		})
	}
}

func TestModelUpdateDoesNotRequestWorkerLifecyclePolicyArtifact(t *testing.T) {
	tabs := DefaultRuntimeTabs().
		Select(RuntimeTabAgentRuntime).
		WithRows(RuntimeTabAgentRuntime, []string{
			"agents: 0",
			"worker lifecycle policy artifact: /tmp/ouro/policy.json status=dry-run mode=metadata-only transport=metadata-only stale_after=300 worker_stale_after=120 poll_interval_ms=250 max_runs=3 lifecycle_maintenance=true lifecycle_retries=true workers=2 stale_workers=1 stale_leases=1",
		}).
		SelectRow(1)
	called := false
	model := NewModel().
		WithGraphStack(GraphStack{RuntimeTabs: tabs}).
		WithRuntimeActionRequester(func(action RuntimeAction) (RuntimeActionRequestResult, error) {
			called = true
			return RuntimeActionRequestResult{Status: "unexpected"}, nil
		})

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	if cmd != nil {
		t.Fatalf("policy artifact must not create a request command")
	}
	if called {
		t.Fatalf("policy artifact must not call the runtime action requester")
	}
	next, ok := updated.(Model)
	if !ok {
		t.Fatalf("expected Model, got %T", updated)
	}
	action, ok := next.graphStack.RuntimeTabs.ActiveRuntimeAction()
	if !ok {
		t.Fatalf("expected active policy action with unsupported request metadata")
	}
	if action.Kind != "worker-lifecycle-policy-artifact" || action.RequestStatus != "unsupported" {
		t.Fatalf("expected unsupported policy request metadata, got %#v", action)
	}
}

func TestModelUpdateDoesNotExecuteWorkerLifecyclePolicyArtifact(t *testing.T) {
	tabs := DefaultRuntimeTabs().
		Select(RuntimeTabAgentRuntime).
		WithRows(RuntimeTabAgentRuntime, []string{
			"agents: 0",
			"worker lifecycle policy artifact: /tmp/ouro/policy.json status=dry-run mode=metadata-only transport=metadata-only stale_after=300 worker_stale_after=120 poll_interval_ms=250 max_runs=3 lifecycle_maintenance=true lifecycle_retries=true workers=2 stale_workers=1 stale_leases=1",
		}).
		SelectRow(1)
	called := false
	model := NewModel().
		WithGraphStack(GraphStack{RuntimeTabs: tabs}).
		WithRuntimeActionExecutor(func(action RuntimeAction) (RuntimeActionExecutionResult, error) {
			called = true
			return RuntimeActionExecutionResult{Status: "unexpected", Path: "/tmp/ouro/executed.json"}, nil
		})

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if cmd != nil {
		t.Fatalf("policy artifact must not arm or return an execution command")
	}
	if called {
		t.Fatalf("policy artifact must not call the runtime action executor")
	}
	next, ok := updated.(Model)
	if !ok {
		t.Fatalf("expected Model, got %T", updated)
	}
	action, ok := next.graphStack.RuntimeTabs.ActiveRuntimeAction()
	if !ok {
		t.Fatalf("expected active policy action with unsupported execution metadata")
	}
	if action.Kind != "worker-lifecycle-policy-artifact" || action.ExecutionStatus != "unsupported" || action.ExecutionError != "runtime action is not executable" {
		t.Fatalf("expected unsupported policy execution metadata, got %#v", action)
	}

	updated, cmd = next.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if cmd != nil {
		t.Fatalf("second x on policy artifact must not return an execution command")
	}
	if called {
		t.Fatalf("second x on policy artifact must not call the runtime action executor")
	}
}

func TestModelUpdateDoesNotExecuteWorkerLifecyclePolicyRunArtifact(t *testing.T) {
	tabs := DefaultRuntimeTabs().
		Select(RuntimeTabAgentRuntime).
		WithRows(RuntimeTabAgentRuntime, []string{
			"agents: 0",
			"worker lifecycle policy run artifact: /tmp/ouro/runtime/worker-lifecycle-policy-runs/run-001.json status=completed mode=metadata-only policy=/tmp/ouro/runtime/worker-lifecycle-policies/policy-b.json selection=/tmp/ouro/runtime/worker-lifecycle-policy-selection.json max_runs=3 poll_interval_ms=250 transport=metadata-only include_lifecycle_maintenance=true execute_lifecycle_retries=false supervisor_created=1 skip=none",
		}).
		SelectRow(1)
	called := false
	model := NewModel().
		WithGraphStack(GraphStack{RuntimeTabs: tabs}).
		WithRuntimeActionExecutor(func(action RuntimeAction) (RuntimeActionExecutionResult, error) {
			called = true
			return RuntimeActionExecutionResult{Status: "unexpected", Path: "/tmp/ouro/executed.json"}, nil
		})

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if cmd != nil {
		t.Fatalf("policy run artifact must not arm or return an execution command")
	}
	if called {
		t.Fatalf("policy run artifact must not call the runtime action executor")
	}
	next, ok := updated.(Model)
	if !ok {
		t.Fatalf("expected Model, got %T", updated)
	}
	action, ok := next.graphStack.RuntimeTabs.ActiveRuntimeAction()
	if !ok {
		t.Fatalf("expected active policy run action with unsupported execution metadata")
	}
	if action.Kind != "worker-lifecycle-policy-run-artifact" || action.ExecutionStatus != "unsupported" || action.ExecutionError != "runtime action is not executable" {
		t.Fatalf("expected unsupported policy run execution metadata, got %#v", action)
	}
	if !runtimeActionIsReadOnly(action) {
		t.Fatalf("expected policy run artifact to be classified read-only")
	}

	updated, cmd = next.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if cmd != nil {
		t.Fatalf("second x on policy run artifact must not return an execution command")
	}
	if called {
		t.Fatalf("second x on policy run artifact must not call the runtime action executor")
	}
}

func TestModelUpdateDoesNotRequestPolicySchedulerRunArtifact(t *testing.T) {
	tabs := DefaultRuntimeTabs().
		Select(RuntimeTabAgentRuntime).
		WithRows(RuntimeTabAgentRuntime, []string{
			"agents: 0",
			"worker lifecycle policy scheduler run artifact: /tmp/ouro/runtime/worker-lifecycle-policy-scheduler-runs/scheduler-001.json status=completed mode=metadata-only policy=/tmp/ouro/runtime/worker-lifecycle-policies/policy-b.json selection=/tmp/ouro/runtime/worker-lifecycle-policy-selection.json policy_basename=policy-b.json selection_basename=worker-lifecycle-policy-selection.json max_runs=3 poll_interval_ms=250 transport=metadata-only include_lifecycle_maintenance=true execute_lifecycle_retries=false policy_runs_created=1 supervisor_created=1 skip=none error=none",
		}).
		SelectRow(1)
	called := false
	model := NewModel().
		WithGraphStack(GraphStack{RuntimeTabs: tabs}).
		WithRuntimeActionRequester(func(action RuntimeAction) (RuntimeActionRequestResult, error) {
			called = true
			return RuntimeActionRequestResult{Status: "unexpected"}, nil
		})

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	if cmd != nil {
		t.Fatalf("policy scheduler run artifact must not create a request command")
	}
	if called {
		t.Fatalf("policy scheduler run artifact must not call the runtime action requester")
	}
	next, ok := updated.(Model)
	if !ok {
		t.Fatalf("expected Model, got %T", updated)
	}
	action, ok := next.graphStack.RuntimeTabs.ActiveRuntimeAction()
	if !ok {
		t.Fatalf("expected active policy scheduler action with unsupported request metadata")
	}
	if action.Kind != "worker-lifecycle-policy-scheduler-run-artifact" || action.RequestStatus != "unsupported" || action.RequestError != "runtime action is read-only" {
		t.Fatalf("expected unsupported policy scheduler request metadata, got %#v", action)
	}
}

func TestModelUpdateDoesNotRequestReadOnlyRuntimeActions(t *testing.T) {
	for _, tc := range []struct {
		name  string
		row   string
		kind  string
		state string
	}{
		{
			name: "compile preview review comparison",
			row:  "compile-preview review comparison: mode=metadata-only base=/tmp/ouro/base.json candidate=/tmp/ouro/candidate.json action=compare node_delta=0 warning_delta=0 added=0 removed=0 changed=0 reordered=0 score_changed=0 signal_changed=0",
			kind: "compile-preview-review-comparison",
		},
		{
			name: "compile preview review execution artifact",
			row:  "compile-preview review execution artifact: status=completed mode=execute action=discover_assets nodes=1 warnings=0 events=12 actor=operator executed_at=2026-06-04T11:30:00Z run=compile-preview-review-execution-1 review=/tmp/ouro/review.json selection=/tmp/ouro/selection.json path=/tmp/ouro/execution.json reason=reviewed execution gate",
			kind: "compile-preview-review-execution-artifact",
		},
		{
			name:  "cleared compile preview review selection",
			row:   "compile-preview review selection: status=cleared mode=metadata-only review=/tmp/ouro/candidate.json action=discover_assets superseded=1 cleared_by=alice cleared_at=2026-06-01T00:35:00Z reason=authorization changed",
			kind:  "compile-preview-review-selection",
			state: "cleared",
		},
		{
			name: "collaboration presence",
			row:  "replay workbench collaboration presence artifact: status=away mode=metadata-only primary=run-primary selected=run-comparison participant=alice seen_by=lead seen_at=2026-06-01T05:30:00Z reason=review active path=/tmp/ouro/replay/workbench-collaboration-presence/alice.json collaboration=/tmp/ouro/replay/workbench-collaborations/collab.json session_path=/tmp/ouro/replay/workbench-sessions/session.json",
			kind: "replay-workbench-collaboration-presence",
		},
		{
			name: "provider usage summary",
			row:  "provider usage summary: mode=metadata-only records=2 providers=local-openai|second-openai models=gpt-a|gpt-b input_tokens=1200 output_tokens=800 total_tokens=2000 estimated_cost_usd=0.007500 estimated_cost_records=1 budget_enforced_records=1 budget_within_limit_records=1 first_created_at=2026-06-04T12:30:00Z last_created_at=2026-06-04T13:00:00Z source=/tmp/ouro/runtime/model-usage",
			kind: "provider-usage-summary",
		},
		{
			name:  "workflow status",
			row:   "workflow status: mode=read-only status=ready stage=model-tool-plan next_gate=accept-model-tool-plan-preview reason=valid-preview-awaits-acceptance action=enumerate_paths preview=/tmp/ouro/runtime/model-tool-plan-previews/preview.json review=/tmp/ouro/toolchain/compile-preview-reviews/review.json selection=- dispatch=- execution=- command=ouro_provider_tool-plan-preview-accept",
			kind:  "workflow-status",
			state: "ready",
		},
		{
			name:  "workflow status safety blocked",
			row:   "workflow status: mode=read-only status=blocked stage=model-tool-plan next_gate=safety-authorization reason=compile-execution-requires-current-safety-authorization safety_errors=1 action=enumerate_paths preview=/tmp/ouro/runtime/model-tool-plan-previews/preview.json review=/tmp/ouro/toolchain/compile-preview-reviews/review.json selection=/tmp/ouro/toolchain/compile-preview-review-selection.json dispatch=/tmp/ouro/toolchain/compile-preview-review-execution-dispatches/dispatch.json execution=- command=ouro_doctor_/tmp/ouro_--require-safety-authorization",
			kind:  "workflow-status",
			state: "blocked",
		},
		{
			name:  "release managed status source",
			row:   "release managed status source: status=current mode=metadata-only release_root=/tmp/ouro/share total=1 available=1 blocked=0 not_imported=1 imported=0 linked=0 conflict=0 marker=/tmp/ouro/toolchain/release-managed-status-source.json reason=current_release",
			kind:  "release-managed-status-source",
			state: "current",
		},
		{
			name:  "release managed status",
			row:   "release managed status: name=httpx status=available mode=read-only kind=binary version=1.3.0 source=projectdiscovery/httpx sha256=abc digest_status=verified digest_algorithm=sha256 digest_scope=file import_status=not_imported link_status=missing release_bundle=/tmp/ouro/share/toolchains/managed-bundles/httpx release_payload=/tmp/ouro/share/toolchains/managed-bundles/httpx/payload/httpx release_manifest=/tmp/ouro/share/toolchains/managed-bundles/httpx/bundle.yaml workspace_bundle=/tmp/ouro/workspace/toolchain/managed-bundles/httpx workspace_payload=/tmp/ouro/workspace/toolchain/managed-bundles/httpx/payload/httpx workspace_manifest=/tmp/ouro/workspace/toolchain/managed-bundles/httpx/bundle.yaml install=/tmp/ouro/workspace/toolchain/bin/httpx error=",
			kind:  "release-managed-status",
			state: "available",
		},
		{
			name:  "run model generation",
			row:   "run model generation: source=closed-loop-planner-generation provider=local-openai model=gpt-4.1-mini tokens=120/80 cost=$0.000450 budget=within_limit ledger_write=true mode=provider-generation status=completed assistant_text_persisted=false digest=sha256:abc123 preview=Generated provider notes",
			kind:  "run-model-generation",
			state: "completed",
		},
		{
			name:  "model generation artifact",
			row:   "model generation artifact: mode=metadata-only status=completed run=run-review source=closed-loop-planner-generation provider=local-openai model=gpt-review tokens=42/11/53 cost=$0.000321 budget=within_limit ledger_write=true assistant_text_persisted=false path=/tmp/ouro/runtime/model-generations/review.json artifact=model-generation-review digest=sha256:def456 preview=Bounded provider notes",
			kind:  "model-generation-artifact",
			state: "completed",
		},
		{
			name: "toolchain contract summary",
			row:  "toolchain contract: status=ready mode=metadata-only manifests=3 tools=14 domain_packs=3 patterns=3 semantic_actions=7 errors=0",
			kind: "toolchain-contract",
		},
		{
			name:  "managed bundle",
			row:   "managed bundle: name=httpx status=available mode=metadata-only kind=binary version=1.3.0 source=projectdiscovery/httpx sha256=abc123 payload=/tmp/ouro/workspace/toolchain/managed-bundles/httpx/payload/httpx install=/tmp/ouro/workspace/toolchain/bin/httpx manifest=/tmp/ouro/workspace/toolchain/managed-bundles/httpx/bundle.yaml",
			kind:  "managed-bundle",
			state: "available",
		},
		{
			name:  "managed update request",
			row:   "managed update request artifact: status=pending mode=metadata-only name=httpx kind=binary current_version=1.3.0 current_sha256=abc latest_version=1.4.0 latest_sha256=def update_status=update_available update_checked_at=2026-06-07T00:00:00Z update_message=update release_root=/tmp/ouro/share release_bundle=/tmp/ouro/share/toolchains/managed-bundles/httpx release_manifest=/tmp/ouro/share/toolchains/managed-bundles/httpx/bundle.yaml release_payload=/tmp/ouro/share/toolchains/managed-bundles/httpx/payload/httpx artifact=/tmp/ouro/workspace/toolchain/managed-update-requests/httpx.json requested_by=operator requested_at=2026-06-07T00:01:00Z reason=review",
			kind:  "managed-update-request",
			state: "pending",
		},
		{
			name:  "managed update download plan",
			row:   "managed update download plan artifact: status=pending mode=metadata-only name=httpx kind=binary current_version=1.3.0 current_sha256=abc latest_version=1.4.0 latest_sha256=def update_status=update_available expected_sha256=def expected_sha256_status=matched source_url=https://downloads.example.com/httpx.tar.gz request=/tmp/ouro/workspace/toolchain/managed-update-requests/httpx.json release_root=/tmp/ouro/share release_bundle=/tmp/ouro/share/toolchains/managed-bundles/httpx release_manifest=/tmp/ouro/share/toolchains/managed-bundles/httpx/bundle.yaml release_payload=/tmp/ouro/share/toolchains/managed-bundles/httpx/payload/httpx artifact=/tmp/ouro/workspace/toolchain/managed-update-download-plans/httpx.json planned_by=operator planned_at=2026-06-07T00:01:00Z reason=review",
			kind:  "managed-update-download-plan",
			state: "pending",
		},
		{
			name:  "managed update download execution",
			row:   "managed update download execution artifact: status=completed mode=execute action=download name=httpx kind=binary current_version=1.3.0 current_sha256=abc latest_version=1.4.0 latest_sha256=def update_status=update_available expected_sha256=def actual_sha256=def digest_status=matched source_url=https://downloads.example.com/httpx.tar.gz plan=/tmp/ouro/workspace/toolchain/managed-update-download-plans/httpx.json request=/tmp/ouro/workspace/toolchain/managed-update-requests/httpx.json release_bundle=/tmp/ouro/share/toolchains/managed-bundles/httpx payload=/tmp/ouro/workspace/toolchain/managed-update-downloads/httpx/payload artifact=/tmp/ouro/workspace/toolchain/managed-update-download-executions/httpx.json actor=operator executed_at=2026-06-07T00:02:00Z reason=review",
			kind:  "managed-update-download-execution",
			state: "completed",
		},
		{
			name:  "managed update apply execution",
			row:   "managed update apply execution artifact: status=completed mode=apply action=workspace-copy name=httpx kind=binary current_version=1.3.0 current_sha256=abc latest_version=1.4.0 latest_sha256=def update_status=update_available expected_sha256=def actual_sha256=def digest_status=matched digest_algorithm=sha256 digest_scope=file source_url=https://downloads.example.com/httpx.tar.gz download_execution=/tmp/ouro/workspace/toolchain/managed-update-download-executions/httpx.json plan=/tmp/ouro/workspace/toolchain/managed-update-download-plans/httpx.json request=/tmp/ouro/workspace/toolchain/managed-update-requests/httpx.json release_bundle=/tmp/ouro/share/toolchains/managed-bundles/httpx staged_payload=/tmp/ouro/workspace/toolchain/managed-update-downloads/httpx/payload workspace_bundle=/tmp/ouro/workspace/toolchain/managed-bundles/httpx workspace_payload=/tmp/ouro/workspace/toolchain/managed-bundles/httpx/payload/httpx artifact=/tmp/ouro/workspace/toolchain/managed-update-apply-executions/httpx.json actor=operator applied_at=2026-06-07T00:02:00Z reason=review",
			kind:  "managed-update-apply-execution",
			state: "completed",
		},
		{
			name:  "toolchain contract error",
			row:   "toolchain contract error: domain capability web.auth_state_tracking is not provided by workspace manifests or built-in tools",
			kind:  "toolchain-contract-error",
			state: "blocked",
		},
		{
			name: "workspace safety strict gate",
			row:  "workspace safety strict gate: blocked errors=2",
			kind: "workspace-safety-strict-gate",
		},
		{
			name: "worker lifecycle audit artifact",
			row:  "worker lifecycle audit artifact: /tmp/ouro/audit.json status=dry-run mode=metadata-only workers=1 stale_workers=0 placements=2 claims=1 dispatches=1 requests=1 results=1 retry_plans=1 retry_dispatches=1 retry_dispatch_executions=1 leases=1 reclaims=0 reconciliations=0 lifecycle_reviews=1 lifecycle_policies=2 policy_selections=1 policy_runs=3 policy_scheduler_runs=4 action_plans=1 maintenances=1 maintenance_supervisors=1 orchestration_supervisors=1 actions=2 daemon_runs=0 supervisor_runs=0 controls=0",
			kind: "worker-lifecycle-audit-artifact",
		},
		{
			name: "worker lifecycle audit item",
			row:  "worker lifecycle audit item: kind=worker-lifecycle-maintenance-supervisor status=completed mode=metadata-only gate=none worker=none run=none execution=none path=/tmp/ouro/supervisor.json linked=/tmp/ouro/maintenance.json reason=none actions=2 skipped_unsafe=2 retry_dispatch_execute=2",
			kind: "worker-lifecycle-audit-item",
		},
		{
			name: "worker lifecycle action",
			row:  "worker lifecycle action: type=ack-transport-request gate=worker-transport-ack worker=remote-web-1 run=run-action execution=tool-action status=ready mode=metadata-only reason=pending-file-outbox-request request_path=/tmp/ouro/request.json lease_path=none result_path=none dispatch=/tmp/ouro/dispatch.json recommended=none",
			kind: "worker-lifecycle-action",
		},
		{
			name: "managed update download plan",
			row:  "managed update download plan artifact: status=pending mode=metadata-only name=httpx kind=binary current_version=1.3.0 current_sha256=abc latest_version=1.4.0 latest_sha256=def update_status=update_available expected_sha256=def expected_sha256_status=matched source_url=https://downloads.example.com/httpx.tar.gz request=/tmp/ouro/workspace/toolchain/managed-update-requests/httpx.json release_root=/tmp/ouro/share release_bundle=/tmp/ouro/share/toolchains/managed-bundles/httpx release_manifest=/tmp/ouro/share/toolchains/managed-bundles/httpx/bundle.yaml release_payload=/tmp/ouro/share/toolchains/managed-bundles/httpx/payload/httpx artifact=/tmp/ouro/workspace/toolchain/managed-update-download-plans/httpx.json planned_by=operator planned_at=2026-06-07T00:01:00Z reason=review",
			kind: "managed-update-download-plan",
		},
		{
			name: "worker dispatch retry scheduler run artifact",
			row:  "worker dispatch retry scheduler run artifact: /tmp/ouro/runtime/worker-dispatch-retry-scheduler-runs/scheduler-001.json status=completed mode=metadata-only max_runs=2 dispatch_retry_claims=true retry_transport=file-outbox retry_plans_created=2 retry_dispatches_created=2 retry_dispatch_executions_created=2 retry_dispatch_claims_created=3 retry_dispatch_claims_dispatched=2 retry_dispatch_transport_requests_created=2 skip=none error=none",
			kind: "worker-dispatch-retry-scheduler-run-artifact",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tabs := DefaultRuntimeTabs().
				Select(RuntimeTabAgentRuntime).
				WithRows(RuntimeTabAgentRuntime, []string{
					"agents: 0",
					tc.row,
				}).
				SelectRow(1)
			called := false
			model := NewModel().
				WithGraphStack(GraphStack{RuntimeTabs: tabs}).
				WithRuntimeActionRequester(func(action RuntimeAction) (RuntimeActionRequestResult, error) {
					called = true
					return RuntimeActionRequestResult{Status: "unexpected"}, nil
				})

			updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
			if cmd != nil {
				t.Fatalf("read-only runtime action %q must not create a request command", tc.name)
			}
			if called {
				t.Fatalf("read-only runtime action %q must not call requester", tc.name)
			}
			next, ok := updated.(Model)
			if !ok {
				t.Fatalf("expected Model, got %T", updated)
			}
			action, ok := next.graphStack.RuntimeTabs.ActiveRuntimeAction()
			if !ok {
				t.Fatalf("expected active runtime action")
			}
			if action.Kind != tc.kind || action.RequestStatus != "unsupported" || action.RequestError != "runtime action is read-only" {
				t.Fatalf("expected unsupported read-only request metadata, got %#v", action)
			}
			if tc.state != "" && action.State != tc.state {
				t.Fatalf("expected runtime action state %q, got %q", tc.state, action.State)
			}
			if !runtimeActionIsReadOnly(action) {
				t.Fatalf("expected action %s to be classified read-only", action.Kind)
			}
		})
	}
}

func TestRuntimeReleaseManagedStatusSourceUsesEncodedReasonText(t *testing.T) {
	reason := "operator selected release reason_b64=fake status=evil mode=execute release_root=/tmp/ouro/evil-release total=99 available=99 blocked=99 not_imported=99 imported=99 linked=99 conflict=99 marker=/tmp/ouro/evil-marker.json"
	row := "release managed status source: status=current mode=metadata-only release_root=/tmp/ouro/share total=1 available=1 blocked=0 not_imported=1 imported=0 linked=0 conflict=0 marker=/tmp/ouro/toolchain/release-managed-status-source.json reason=" + reason + " reason_b64=" + base64.RawURLEncoding.EncodeToString([]byte(reason))

	action := runtimeReleaseManagedStatusSourceActionForRow(row)
	if action.State != "current" || action.ExecutionMode != "metadata-only" {
		t.Fatalf("encoded status-source reason must not retarget state/mode: %#v", action)
	}
	if action.ResultPath != "/tmp/ouro/share" || action.ArtifactPath != "/tmp/ouro/toolchain/release-managed-status-source.json" {
		t.Fatalf("encoded status-source reason must not retarget paths: %#v", action)
	}
	if action.Ready != "total=1 available=1 not_imported=1 imported=0 linked=0" {
		t.Fatalf("encoded status-source reason must not retarget ready counts: %#v", action)
	}
	if action.Failed != "blocked=0 conflict=0" {
		t.Fatalf("encoded status-source reason must not retarget failed counts: %#v", action)
	}
	if action.Reason != reason {
		t.Fatalf("expected encoded status-source reason to be decoded, got %q", action.Reason)
	}
}

func TestRuntimeManagedBundleUsesEncodedSourceText(t *testing.T) {
	source := "projectdiscovery httpx source_b64=fake status=blocked mode=execute name=evil-source sha256=evil payload=/tmp/ouro/evil-payload install=/tmp/ouro/evil-install manifest=/tmp/ouro/evil-manifest.yaml"
	row := "managed bundle: name=httpx status=available mode=metadata-only kind=binary version=1.3.0 source=" + source + " source_b64=" + base64.RawURLEncoding.EncodeToString([]byte(source)) + " update_status=current error= provenance_status=recorded sha256=abc123 update_latest_version=1.3.0 update_latest_sha256=abc123 update_checked_at=2026-06-05T00:00:00Z update_message=current provenance_source_url=https://github.com/projectdiscovery/httpx provenance_upstream_version=v1.3.0 provenance_retrieved_at=2026-06-05T00:00:00Z provenance_verified_by=operator payload=/tmp/ouro/workspace/toolchain/managed-bundles/httpx/payload/httpx install=/tmp/ouro/workspace/toolchain/bin/httpx manifest=/tmp/ouro/workspace/toolchain/managed-bundles/httpx/bundle.yaml"

	action := runtimeManagedBundleActionForRow(row)
	if action.Name != "httpx" || action.State != "available" || action.ExecutionMode != "metadata-only" {
		t.Fatalf("encoded managed bundle source must not retarget identity metadata: %#v", action)
	}
	if action.Source != source {
		t.Fatalf("expected managed bundle source to decode full text, got %q", action.Source)
	}
	if action.Digest != "abc123" || action.ResultPath != "/tmp/ouro/workspace/toolchain/managed-bundles/httpx/payload/httpx" || action.Target != "/tmp/ouro/workspace/toolchain/bin/httpx" || action.ArtifactPath != "/tmp/ouro/workspace/toolchain/managed-bundles/httpx/bundle.yaml" {
		t.Fatalf("encoded managed bundle source must not retarget paths or digest: %#v", action)
	}
}

func TestRuntimeManagedBundleUsesEncodedReferenceFields(t *testing.T) {
	payload := "/tmp/ouro/workspace/toolchain/managed bundles/httpx/payload status=blocked/httpx"
	install := "/tmp/ouro/workspace/toolchain/bin/install mode=execute/httpx"
	manifest := "/tmp/ouro/workspace/toolchain/managed bundles/httpx/manifest payload=/tmp/evil/bundle.yaml"
	row := strings.Join([]string{
		"managed bundle: name=httpx status=available mode=metadata-only kind=binary version=1.3.0 source=projectdiscovery/httpx source_b64=" + base64.RawURLEncoding.EncodeToString([]byte("projectdiscovery/httpx")),
		"update_status=current error= provenance_status=recorded sha256=abc123 update_latest_version=1.3.0 update_latest_sha256=abc123 update_checked_at=2026-06-05T00:00:00Z update_message=current provenance_source_url=https://github.com/projectdiscovery/httpx provenance_upstream_version=v1.3.0 provenance_retrieved_at=2026-06-05T00:00:00Z provenance_verified_by=operator",
		encodedRuntimeRowField("payload", payload),
		encodedRuntimeRowField("install", install),
		encodedRuntimeRowField("manifest", manifest),
	}, " ")

	action := runtimeManagedBundleActionForRow(row)
	if action.Name != "httpx" || action.State != "available" || action.ExecutionMode != "metadata-only" {
		t.Fatalf("encoded managed bundle references must not retarget identity metadata: %#v", action)
	}
	if action.ResultPath != payload || action.Target != install || action.ArtifactPath != manifest {
		t.Fatalf("expected exact managed bundle reference paths, got %#v", action)
	}
	if action.Digest != "abc123" || action.Source != "projectdiscovery/httpx" {
		t.Fatalf("encoded managed bundle references changed source/digest metadata: %#v", action)
	}
}

func TestRuntimeManagedBundleShowsSignatureMetadata(t *testing.T) {
	row := strings.Join([]string{
		"managed bundle: name=httpx status=available mode=metadata-only kind=binary version=1.3.0 source=projectdiscovery/httpx source_b64=" + base64.RawURLEncoding.EncodeToString([]byte("projectdiscovery/httpx")),
		"update_status=current error= provenance_status=recorded signature_status=verified signature_algorithm=ed25519 signature_key_id=local-test-key signature_signed_at=2026-06-07T00:00:00Z signature_signed_by=operator sha256=abc123 update_latest_version=1.3.0 update_latest_sha256=abc123 update_checked_at=2026-06-05T00:00:00Z update_message=current provenance_source_url=https://github.com/projectdiscovery/httpx provenance_upstream_version=v1.3.0 provenance_retrieved_at=2026-06-05T00:00:00Z provenance_verified_by=operator",
		encodedRuntimeRowField("payload", "/tmp/ouro/workspace/toolchain/managed-bundles/httpx/payload/httpx"),
		encodedRuntimeRowField("install", "/tmp/ouro/workspace/toolchain/bin/httpx"),
		encodedRuntimeRowField("manifest", "/tmp/ouro/workspace/toolchain/managed-bundles/httpx/bundle.yaml"),
	}, " ")

	action := runtimeManagedBundleActionForRow(row)
	for _, want := range []string{
		"signature_status=verified",
		"signature_algorithm=ed25519",
		"signature_key_id=local-test-key",
		"signature_signed_at=2026-06-07T00:00:00Z",
		"signature_signed_by=operator",
	} {
		if !strings.Contains(action.Output, want) {
			t.Fatalf("expected managed bundle action output %q, got %#v", want, action)
		}
	}
}

func TestRuntimeManagedBundleShowsCorpusMetadata(t *testing.T) {
	row := strings.Join([]string{
		"managed bundle: name=nuclei-templates status=available mode=metadata-only kind=rule_bundle version=2026.06.05 source=projectdiscovery/nuclei-templates source_b64=" + base64.RawURLEncoding.EncodeToString([]byte("projectdiscovery/nuclei-templates")),
		"update_status=current error= provenance_status=recorded signature_status=missing sha256=" + strings.Repeat("a", 64),
		"corpus_status=recorded corpus_name=nuclei-templates corpus_source=https://github.com/projectdiscovery/nuclei-templates corpus_retrieved_at=2026-06-05T00:00:00Z corpus_file_count=1 corpus_sha256=" + strings.Repeat("a", 64) + " corpus_digest_status=matched corpus_digest_algorithm=sha256 corpus_digest_scope=directory_canonical_records",
		encodedRuntimeRowField("payload", "/tmp/ouro/workspace/toolchain/managed-bundles/nuclei-templates/payload"),
		encodedRuntimeRowField("install", "/tmp/ouro/workspace/toolchain/rules/nuclei-templates"),
		encodedRuntimeRowField("manifest", "/tmp/ouro/workspace/toolchain/managed-bundles/nuclei-templates/bundle.yaml"),
	}, " ")

	action := runtimeManagedBundleActionForRow(row)
	for _, want := range []string{
		"corpus_status=recorded",
		"corpus_name=nuclei-templates",
		"corpus_source=https://github.com/projectdiscovery/nuclei-templates",
		"corpus_file_count=1",
		"corpus_digest_status=matched",
		"corpus_digest_scope=directory_canonical_records",
	} {
		if !strings.Contains(action.Output, want) {
			t.Fatalf("expected managed bundle corpus output %q, got %#v", want, action)
		}
	}
}

func TestRuntimeDecodedRowFieldIgnoresMismatchedMapSidecarWhenReadableValueExists(t *testing.T) {
	row := "source=projectdiscovery/httpx source_b64=" + base64.RawURLEncoding.EncodeToString([]byte("evil source")) + " status=available"

	got := runtimeDecodedRowField(row, runtimeSimpleKeyValueFields(row), "source")
	if got != "projectdiscovery/httpx" {
		t.Fatalf("mismatched map sidecar must not override readable source, got %q", got)
	}
}

func TestRuntimeDecodedRowFieldWithFallbackRowIgnoresMismatchedMapSidecarWhenFallbackValueExists(t *testing.T) {
	row := "output=ok command_b64=" + base64.RawURLEncoding.EncodeToString([]byte("evil command"))
	fallbackRow := "command=sh -c echo ok"

	got := runtimeDecodedRowFieldWithFallbackRow(row, runtimeSimpleKeyValueFields(row), "command", fallbackRow)
	if got != "sh -c echo ok" {
		t.Fatalf("mismatched map sidecar must not override fallback command, got %q", got)
	}
}

func TestRuntimeDecodedRowFieldKeepsSidecarOnlyCompatibility(t *testing.T) {
	preview := "Generated provider notes path=/tmp/evil mode=process with spaces"
	row := "status=completed preview_b64=" + base64.RawURLEncoding.EncodeToString([]byte(preview))

	got := runtimeDecodedRowField(row, runtimeSimpleKeyValueFields(row), "preview")
	if got != preview {
		t.Fatalf("sidecar-only rows must still decode preview, got %q", got)
	}
}

func TestRuntimeDecodedRowFieldAcceptsUnderscoreCompactedReadableSidecar(t *testing.T) {
	reason := "current reviewed action plan"
	row := "reason=current_reviewed_action_plan reason_b64=" + base64.RawURLEncoding.EncodeToString([]byte(reason)) + " status=current"

	got := runtimeDecodedRowField(row, runtimeSimpleKeyValueFields(row), "reason")
	if got != reason {
		t.Fatalf("underscore-compacted readable sidecar must decode original reason, got %q", got)
	}
}

func TestRuntimeReleaseManagedStatusSourceLegacyReasonFallback(t *testing.T) {
	row := "release managed status source: status=current mode=metadata-only release_root=/tmp/ouro/share total=1 available=1 blocked=0 not_imported=1 imported=0 linked=0 conflict=0 marker=/tmp/ouro/toolchain/release-managed-status-source.json reason=current release root"

	action := runtimeReleaseManagedStatusSourceActionForRow(row)
	if action.ResultPath != "/tmp/ouro/share" || action.ArtifactPath != "/tmp/ouro/toolchain/release-managed-status-source.json" {
		t.Fatalf("legacy status-source paths changed: %#v", action)
	}
	if action.Reason != "current release root" {
		t.Fatalf("legacy status-source reason fallback changed: %#v", action)
	}
}

func TestRuntimeReleaseManagedStatusSourceUsesEncodedReferenceFields(t *testing.T) {
	releaseRoot := "/tmp/ouro/share release_root status=evil"
	marker := "/tmp/ouro/workspace/toolchain/release status source marker mode=execute.json"
	reason := "current release"
	row := strings.Join([]string{
		"release managed status source: status=current mode=metadata-only",
		encodedRuntimeRowField("release_root", releaseRoot),
		"total=1 available=1 blocked=0 not_imported=1 imported=0 linked=0 conflict=0",
		encodedRuntimeRowField("marker", marker),
		"reason=" + runtimeCompactRowTextValue(reason) + " reason_b64=" + base64.RawURLEncoding.EncodeToString([]byte(reason)),
	}, " ")

	action := runtimeReleaseManagedStatusSourceActionForRow(row)
	if action.State != "current" || action.ExecutionMode != "metadata-only" {
		t.Fatalf("encoded status-source references must not retarget state/mode: %#v", action)
	}
	if action.ResultPath != releaseRoot || action.ArtifactPath != marker {
		t.Fatalf("expected exact release managed status-source references, got %#v", action)
	}
	if action.Ready != "total=1 available=1 not_imported=1 imported=0 linked=0" || action.Failed != "blocked=0 conflict=0" {
		t.Fatalf("encoded status-source references must not retarget counts: %#v", action)
	}
	if action.Reason != reason {
		t.Fatalf("expected encoded status-source reason to decode, got %q", action.Reason)
	}
}

func TestRuntimeReleaseManagedStatusSourceShowsExpectedPinPolicy(t *testing.T) {
	expectedSHA := strings.Repeat("a", 64)
	row := strings.Join([]string{
		"release managed status source: status=current mode=metadata-only",
		encodedRuntimeRowField("release_root", "/tmp/ouro/share"),
		"total=1 available=0 blocked=1 not_imported=0 imported=0 linked=0 conflict=0",
		"expected_sha256_bundle=httpx",
		"expected_sha256=" + expectedSHA,
		"expected_sha256_status=mismatch",
		encodedRuntimeRowField("marker", "/tmp/ouro/workspace/toolchain/release-managed-status-source.json"),
		"reason=pin_release reason_b64=" + base64.RawURLEncoding.EncodeToString([]byte("pin release")),
	}, " ")

	action := runtimeReleaseManagedStatusSourceActionForRow(row)
	if action.Hint != "expected_sha256_bundle=httpx expected_sha256_status=mismatch expected_sha256="+expectedSHA {
		t.Fatalf("expected status-source expected pin hint, got %#v", action)
	}
	if action.Ready != "total=1 available=0 not_imported=0 imported=0 linked=0" || action.Failed != "blocked=1 conflict=0" {
		t.Fatalf("expected pin fields not to retarget counts, got %#v", action)
	}
}

func TestRuntimeReleaseManagedStatusUsesEncodedErrorText(t *testing.T) {
	sourceText := "projectdiscovery httpx status=available mode=execute name=evil-source"
	errorText := "blocked import install=/tmp/ouro/evil-bin release_manifest=/tmp/ouro/evil-manifest.yaml workspace_manifest=/tmp/ouro/evil-workspace.yaml digest_status=blocked import_status=linked link_status=linked error_b64=fake"
	row := "release managed status: name=httpx status=blocked mode=read-only kind=binary version=1.3.0 source=" + sourceText + " source_b64=" + base64.RawURLEncoding.EncodeToString([]byte(sourceText)) + " sha256=abc digest_status=verified digest_algorithm=sha256 digest_scope=file import_status=not_imported link_status=missing release_bundle=/tmp/ouro/share/toolchains/managed-bundles/httpx release_payload=/tmp/ouro/share/toolchains/managed-bundles/httpx/payload/httpx release_manifest=/tmp/ouro/share/toolchains/managed-bundles/httpx/bundle.yaml workspace_bundle=/tmp/ouro/workspace/toolchain/managed-bundles/httpx workspace_payload=/tmp/ouro/workspace/toolchain/managed-bundles/httpx/payload/httpx workspace_manifest=/tmp/ouro/workspace/toolchain/managed-bundles/httpx/bundle.yaml install=/tmp/ouro/workspace/toolchain/bin/httpx error=" + errorText + " error_b64=" + base64.RawURLEncoding.EncodeToString([]byte(errorText))

	action := runtimeReleaseManagedStatusActionForRow(row)
	if action.Name != "httpx" || action.State != "blocked" || action.ExecutionMode != "read-only" || action.Source != sourceText {
		t.Fatalf("encoded release managed status source must not retarget identity metadata: %#v", action)
	}
	if action.Target != "/tmp/ouro/workspace/toolchain/bin/httpx" || action.ArtifactPath != "/tmp/ouro/share/toolchains/managed-bundles/httpx/bundle.yaml" || action.RequestPath != "/tmp/ouro/workspace/toolchain/managed-bundles/httpx/bundle.yaml" {
		t.Fatalf("encoded release managed status error must not retarget paths: %#v", action)
	}
	if action.Ready != "digest_status=verified import_status=not_imported link_status=missing" {
		t.Fatalf("encoded release managed status error must not retarget status metadata: %#v", action)
	}
	if action.Failed != "error="+errorText {
		t.Fatalf("expected encoded release managed status error to be decoded, got %q", action.Failed)
	}
}

func TestRuntimeReleaseManagedStatusShowsExpectedPinPolicy(t *testing.T) {
	expectedSHA := strings.Repeat("b", 64)
	sourceText := "projectdiscovery/httpx"
	row := strings.Join([]string{
		"release managed status: name=httpx status=blocked mode=read-only kind=binary version=1.3.0 source=" + sourceText + " source_b64=" + base64.RawURLEncoding.EncodeToString([]byte(sourceText)),
		"update_status=current sha256=abc expected_sha256=" + expectedSHA + " expected_sha256_status=mismatch provenance_status=recorded signature_status=missing signature_algorithm=none signature_key_id=none signature_signed_at=none signature_signed_by=none update_latest_version=none update_latest_sha256=none update_checked_at=none update_message=none provenance_source_url=none provenance_upstream_version=none provenance_retrieved_at=none provenance_verified_by=none digest_status=verified digest_algorithm=sha256 digest_scope=file import_status=blocked link_status=blocked",
		encodedRuntimeRowField("release_bundle", "/tmp/ouro/share/toolchains/managed-bundles/httpx"),
		encodedRuntimeRowField("release_payload", "/tmp/ouro/share/toolchains/managed-bundles/httpx/payload/httpx"),
		encodedRuntimeRowField("release_manifest", "/tmp/ouro/share/toolchains/managed-bundles/httpx/bundle.yaml"),
		encodedRuntimeRowField("workspace_bundle", "/tmp/ouro/workspace/toolchain/managed-bundles/httpx"),
		encodedRuntimeRowField("workspace_payload", "/tmp/ouro/workspace/toolchain/managed-bundles/httpx/payload/httpx"),
		encodedRuntimeRowField("workspace_manifest", "/tmp/ouro/workspace/toolchain/managed-bundles/httpx/bundle.yaml"),
		encodedRuntimeRowField("install", "/tmp/ouro/workspace/toolchain/bin/httpx"),
		"error=expected_mismatch error_b64=" + base64.RawURLEncoding.EncodeToString([]byte("expected mismatch")),
	}, " ")

	action := runtimeReleaseManagedStatusActionForRow(row)
	for _, want := range []string{
		"expected_sha256_status=mismatch",
		"expected_sha256=" + expectedSHA,
	} {
		if !strings.Contains(action.Hint, want) {
			t.Fatalf("expected release managed status pin hint %q, got %#v", want, action)
		}
	}
	if action.Output != "digest_status=verified import_status=blocked link_status=blocked" {
		t.Fatalf("expected pin fields not to retarget output, got %#v", action)
	}
}

func TestRuntimeReleaseManagedStatusShowsCorpusMetadata(t *testing.T) {
	sourceText := "projectdiscovery/nuclei-templates"
	row := strings.Join([]string{
		"release managed status: name=nuclei-templates status=available mode=read-only kind=rule_bundle version=2026.06.05 source=" + sourceText + " source_b64=" + base64.RawURLEncoding.EncodeToString([]byte(sourceText)),
		"update_status=current sha256=" + strings.Repeat("c", 64) + " expected_sha256_status=none provenance_status=recorded signature_status=missing signature_algorithm=none signature_key_id=none signature_signed_at=none signature_signed_by=none update_latest_version=none update_latest_sha256=none update_checked_at=none update_message=none provenance_source_url=none provenance_upstream_version=none provenance_retrieved_at=none provenance_verified_by=none digest_status=verified digest_algorithm=sha256 digest_scope=directory_canonical_records import_status=not_imported link_status=missing",
		"corpus_status=recorded corpus_name=nuclei-templates corpus_source=https://github.com/projectdiscovery/nuclei-templates corpus_retrieved_at=2026-06-05T00:00:00Z corpus_file_count=1 corpus_sha256=" + strings.Repeat("c", 64) + " corpus_digest_status=matched corpus_digest_algorithm=sha256 corpus_digest_scope=directory_canonical_records",
		encodedRuntimeRowField("release_bundle", "/tmp/ouro/share/toolchains/managed-bundles/nuclei-templates"),
		encodedRuntimeRowField("release_payload", "/tmp/ouro/share/toolchains/managed-bundles/nuclei-templates/payload"),
		encodedRuntimeRowField("release_manifest", "/tmp/ouro/share/toolchains/managed-bundles/nuclei-templates/bundle.yaml"),
		encodedRuntimeRowField("workspace_bundle", "/tmp/ouro/workspace/toolchain/managed-bundles/nuclei-templates"),
		encodedRuntimeRowField("workspace_payload", "/tmp/ouro/workspace/toolchain/managed-bundles/nuclei-templates/payload"),
		encodedRuntimeRowField("workspace_manifest", "/tmp/ouro/workspace/toolchain/managed-bundles/nuclei-templates/bundle.yaml"),
		encodedRuntimeRowField("install", "/tmp/ouro/workspace/toolchain/rules/nuclei-templates"),
	}, " ")

	action := runtimeReleaseManagedStatusActionForRow(row)
	for _, want := range []string{
		"corpus_status=recorded",
		"corpus_name=nuclei-templates",
		"corpus_source=https://github.com/projectdiscovery/nuclei-templates",
		"corpus_file_count=1",
		"corpus_digest_status=matched",
		"corpus_digest_scope=directory_canonical_records",
	} {
		if !strings.Contains(action.Reason, want) {
			t.Fatalf("expected release managed corpus reason %q, got %#v", want, action)
		}
	}
}

func TestRuntimeManagedUpdateRequestUsesEncodedReferenceFields(t *testing.T) {
	artifact := "/tmp/ouro workspace/toolchain/managed update requests/httpx status=approved.json"
	releaseRoot := "/tmp/ouro release root status=blocked"
	releaseBundle := "/tmp/ouro release/toolchains/managed bundles/httpx release_manifest=/tmp/evil"
	releaseManifest := "/tmp/ouro release/toolchains/managed bundles/httpx/bundle latest_sha256=evil.yaml"
	releasePayload := "/tmp/ouro release/toolchains/managed bundles/httpx/payload/httpx mode=execute"
	requestedBy := "operator name=evil"
	updateMessage := "update available status=blocked artifact=/tmp/evil"
	reason := "review reason status=approved path=/tmp/evil"
	row := strings.Join([]string{
		"managed update request artifact: status=pending mode=metadata-only name=httpx kind=binary current_version=1.3.0 current_sha256=abc latest_version=1.4.0 latest_sha256=" + strings.Repeat("c", 64),
		"update_status=update_available update_checked_at=2026-06-07T00:00:00Z",
		encodedRuntimeRowField("artifact", artifact),
		encodedRuntimeRowField("release_root", releaseRoot),
		encodedRuntimeRowField("release_bundle", releaseBundle),
		encodedRuntimeRowField("release_manifest", releaseManifest),
		encodedRuntimeRowField("release_payload", releasePayload),
		"requested_by=" + requestedBy + " requested_by_b64=" + base64.RawURLEncoding.EncodeToString([]byte(requestedBy)),
		"requested_at=2026-06-07T00:01:00Z",
		"update_message=" + updateMessage + " update_message_b64=" + base64.RawURLEncoding.EncodeToString([]byte(updateMessage)),
		"reason=" + reason + " reason_b64=" + base64.RawURLEncoding.EncodeToString([]byte(reason)),
	}, " ")

	action := runtimeManagedUpdateRequestActionForRow(row)
	if action.Kind != "managed-update-request" || action.State != "pending" || action.ExecutionMode != "metadata-only" || action.Name != "httpx" {
		t.Fatalf("encoded update request references must not retarget identity metadata: %#v", action)
	}
	if action.ArtifactPath != artifact || action.Target != releaseBundle || action.RequestPath != releaseManifest || action.ResultPath != releasePayload {
		t.Fatalf("expected exact managed update request paths, got %#v", action)
	}
	if action.Source != releaseRoot || action.Reason != reason {
		t.Fatalf("expected exact update request source/reason text, got %#v", action)
	}
	for _, want := range []string{
		"update_status=update_available",
		"current_version=1.3.0",
		"latest_version=1.4.0",
		"latest_sha256=" + strings.Repeat("c", 64),
	} {
		if !strings.Contains(action.Output, want) {
			t.Fatalf("expected update request output %q, got %#v", want, action)
		}
	}
	for _, want := range []string{
		"requested_by=" + requestedBy,
		"current_sha256=abc",
		"checked_at=2026-06-07T00:00:00Z",
		"message=" + updateMessage,
	} {
		if !strings.Contains(action.Hint, want) {
			t.Fatalf("expected update request hint %q, got %#v", want, action)
		}
	}
	details := strings.Join(inspectorForRuntimeAction(action).Rows, "\n")
	for _, want := range []string{
		"runtime action: managed-update-request",
		"artifact path: " + artifact,
		"request path: " + releaseManifest,
		"target: " + releaseBundle,
		"result path: " + releasePayload,
		"source: " + releaseRoot,
		"reason: " + reason,
		"mode: read-only",
	} {
		if !strings.Contains(details, want) {
			t.Fatalf("expected update request details %q, got %q", want, details)
		}
	}
}

func TestRuntimeManagedUpdateDownloadPlanUsesEncodedReferenceFields(t *testing.T) {
	artifact := "/tmp/ouro workspace/toolchain/managed update download plans/httpx status=approved.json"
	request := "/tmp/ouro workspace/toolchain/managed update requests/httpx mode=execute.json"
	releaseRoot := "/tmp/ouro release root status=blocked"
	releaseBundle := "/tmp/ouro release/toolchains/managed bundles/httpx release_manifest=/tmp/evil"
	releaseManifest := "/tmp/ouro release/toolchains/managed bundles/httpx/bundle latest_sha256=evil.yaml"
	releasePayload := "/tmp/ouro release/toolchains/managed bundles/httpx/payload/httpx mode=execute"
	sourceURL := "https://downloads.example.com/httpx/status=blocked/httpx.tar.gz"
	plannedBy := "operator name=evil"
	updateMessage := "download available status=blocked artifact=/tmp/evil"
	reason := "download plan reason status=approved path=/tmp/evil"
	latestSHA := strings.Repeat("d", 64)
	archiveSHA := strings.Repeat("a", 64)
	row := strings.Join([]string{
		"managed update download plan artifact: status=pending mode=metadata-only name=httpx kind=binary current_version=1.3.0 current_sha256=abc latest_version=1.4.0 latest_sha256=" + latestSHA,
		"update_status=update_available update_checked_at=2026-06-07T00:00:00Z expected_sha256=" + latestSHA + " expected_sha256_status=matched archive_format=tar.gz archive_expected_sha256=" + archiveSHA + " archive_expected_sha256_status=matched",
		encodedRuntimeRowField("artifact", artifact),
		encodedRuntimeRowField("request", request),
		encodedRuntimeRowField("source_url", sourceURL),
		encodedRuntimeRowField("release_root", releaseRoot),
		encodedRuntimeRowField("release_bundle", releaseBundle),
		encodedRuntimeRowField("release_manifest", releaseManifest),
		encodedRuntimeRowField("release_payload", releasePayload),
		"planned_by=" + plannedBy + " planned_by_b64=" + base64.RawURLEncoding.EncodeToString([]byte(plannedBy)),
		"planned_at=2026-06-07T00:01:00Z",
		"update_message=" + updateMessage + " update_message_b64=" + base64.RawURLEncoding.EncodeToString([]byte(updateMessage)),
		"reason=" + reason + " reason_b64=" + base64.RawURLEncoding.EncodeToString([]byte(reason)),
	}, " ")

	action := runtimeManagedUpdateDownloadPlanActionForRow(row)
	if action.Kind != "managed-update-download-plan" || action.State != "pending" || action.ExecutionMode != "metadata-only" || action.Name != "httpx" {
		t.Fatalf("encoded download plan references must not retarget identity metadata: %#v", action)
	}
	if action.ArtifactPath != artifact || action.RequestPath != request || action.Target != releaseBundle || action.ResultPath != releasePayload {
		t.Fatalf("expected exact managed update download plan paths, got %#v", action)
	}
	if action.Source != sourceURL || action.Reason != reason || action.Digest != latestSHA {
		t.Fatalf("expected exact source/reason/digest, got %#v", action)
	}
	for _, want := range []string{
		"update_status=update_available",
		"latest_version=1.4.0",
		"expected_sha256_status=matched",
		"archive_format=tar.gz",
		"archive_expected_sha256_status=matched",
	} {
		if !strings.Contains(action.Output, want) {
			t.Fatalf("expected download plan output %q, got %#v", want, action)
		}
	}
	for _, want := range []string{
		"planned_by=" + plannedBy,
		"latest_sha256=" + latestSHA,
		"checked_at=2026-06-07T00:00:00Z",
		"message=" + updateMessage,
	} {
		if !strings.Contains(action.Hint, want) {
			t.Fatalf("expected download plan hint %q, got %#v", want, action)
		}
	}
	details := strings.Join(inspectorForRuntimeAction(action).Rows, "\n")
	for _, want := range []string{
		"runtime action: managed-update-download-plan",
		"artifact path: " + artifact,
		"request path: " + request,
		"target: " + releaseBundle,
		"result path: " + releasePayload,
		"source: " + sourceURL,
		"digest: " + latestSHA,
		"reason: " + reason,
		"mode: read-only",
	} {
		if !strings.Contains(details, want) {
			t.Fatalf("expected download plan details %q, got %q", want, details)
		}
	}
}

func TestRuntimeManagedUpdateDownloadExecutionUsesEncodedReferenceFields(t *testing.T) {
	artifact := "/tmp/ouro workspace/toolchain/managed update download executions/httpx status=completed.json"
	plan := "/tmp/ouro workspace/toolchain/managed update download plans/httpx mode=execute.json"
	request := "/tmp/ouro workspace/toolchain/managed update requests/httpx mode=execute.json"
	releaseRoot := "/tmp/ouro release root status=blocked"
	releaseBundle := "/tmp/ouro release/toolchains/managed bundles/httpx release_manifest=/tmp/evil"
	releaseManifest := "/tmp/ouro release/toolchains/managed bundles/httpx/bundle latest_sha256=evil.yaml"
	releasePayload := "/tmp/ouro release/toolchains/managed bundles/httpx/payload/httpx mode=execute"
	archive := "/tmp/ouro workspace/toolchain/managed update downloads/httpx archive status=ready.tar.gz"
	payload := "/tmp/ouro workspace/toolchain/managed update downloads/httpx payload status=ready"
	sourceURL := "https://downloads.example.com/httpx/status=blocked/httpx.tar.gz"
	redirectLocation := "https://redirect.example.com/downloads/httpx.tar.gz"
	actor := "operator name=evil"
	updateMessage := "download executed status=blocked artifact=/tmp/evil"
	reason := "download execution reason status=approved path=/tmp/evil"
	errorText := "download execution error payload=/tmp/evil status=completed"
	expectedSHA := strings.Repeat("d", 64)
	actualSHA := strings.Repeat("e", 64)
	archiveSHA := strings.Repeat("f", 64)
	row := strings.Join([]string{
		"managed update download execution artifact: status=failed mode=execute action=download name=httpx kind=binary current_version=1.3.0 current_sha256=abc latest_version=1.4.0 latest_sha256=" + expectedSHA,
		"update_status=update_available update_checked_at=2026-06-07T00:00:00Z expected_sha256=" + expectedSHA + " actual_sha256=" + actualSHA + " digest_status=mismatch digest_algorithm=sha256 digest_scope=directory_canonical_records archive_format=tar.gz archive_expected_sha256=" + archiveSHA + " archive_actual_sha256=" + archiveSHA + " archive_digest_status=matched archive_digest_algorithm=sha256 archive_digest_scope=file http_status=200 content_length=42 bytes_written=42",
		encodedRuntimeRowField("artifact", artifact),
		encodedRuntimeRowField("plan", plan),
		encodedRuntimeRowField("request", request),
		encodedRuntimeRowField("source_url", sourceURL),
		encodedRuntimeRowField("release_root", releaseRoot),
		encodedRuntimeRowField("release_bundle", releaseBundle),
		encodedRuntimeRowField("release_manifest", releaseManifest),
		encodedRuntimeRowField("release_payload", releasePayload),
		encodedRuntimeRowField("archive", archive),
		encodedRuntimeRowField("payload", payload),
		"redirect_policy=rejected",
		encodedRuntimeRowField("redirect_location", redirectLocation),
		"redirect_location_status=redacted",
		"actor=" + actor + " actor_b64=" + base64.RawURLEncoding.EncodeToString([]byte(actor)),
		"executed_at=2026-06-07T00:01:00Z",
		"update_message=" + updateMessage + " update_message_b64=" + base64.RawURLEncoding.EncodeToString([]byte(updateMessage)),
		"reason=" + reason + " reason_b64=" + base64.RawURLEncoding.EncodeToString([]byte(reason)),
		"error=" + errorText + " error_b64=" + base64.RawURLEncoding.EncodeToString([]byte(errorText)),
	}, " ")

	action := runtimeManagedUpdateDownloadExecutionActionForRow(row)
	if action.Kind != "managed-update-download-execution" || action.State != "failed" || action.ExecutionMode != "execute" || action.Name != "httpx" {
		t.Fatalf("encoded download execution references must not retarget identity metadata: %#v", action)
	}
	if action.ArtifactPath != artifact || action.DispatchPath != plan || action.RequestPath != request || action.Target != releaseBundle || action.ResultPath != payload {
		t.Fatalf("expected exact managed update download execution paths, got %#v", action)
	}
	if action.ArchivePath != archive {
		t.Fatalf("expected exact managed update archive path, got %#v", action)
	}
	if action.Source != sourceURL || action.Reason != reason || action.Digest != actualSHA || action.Failed != errorText {
		t.Fatalf("expected exact source/reason/digest/error, got %#v", action)
	}
	if !runtimeActionIsReadOnly(action) {
		t.Fatalf("managed update download execution action must be read-only: %#v", action)
	}
	for _, want := range []string{
		"digest_status=mismatch",
		"digest_algorithm=sha256",
		"digest_scope=directory_canonical_records",
		"expected_sha256=" + expectedSHA,
		"archive_format=tar.gz",
		"archive_digest_status=matched",
		"archive_digest_scope=file",
		"bytes_written=42",
		"http_status=200",
		"redirect_policy=rejected",
		"redirect_location=" + redirectLocation,
		"redirect_location_status=redacted",
	} {
		if !strings.Contains(action.Output, want) {
			t.Fatalf("expected download execution output %q, got %#v", want, action)
		}
	}
	for _, want := range []string{
		"actor=" + actor,
		"latest_version=1.4.0",
		"checked_at=2026-06-07T00:00:00Z",
		"message=" + updateMessage,
	} {
		if !strings.Contains(action.Hint, want) {
			t.Fatalf("expected download execution hint %q, got %#v", want, action)
		}
	}
	details := strings.Join(inspectorForRuntimeAction(action).Rows, "\n")
	for _, want := range []string{
		"runtime action: managed-update-download-execution",
		"artifact path: " + artifact,
		"dispatch path: " + plan,
		"request path: " + request,
		"archive path: " + archive,
		"target: " + releaseBundle,
		"result path: " + payload,
		"source: " + sourceURL,
		"digest: " + actualSHA,
		"reason: " + reason,
		"failed: " + errorText,
		"mode: read-only",
	} {
		if !strings.Contains(details, want) {
			t.Fatalf("expected download execution details %q, got %q", want, details)
		}
	}
}

func TestRuntimeManagedUpdateApplyExecutionUsesEncodedReferenceFields(t *testing.T) {
	artifact := "/tmp/ouro workspace/toolchain/managed update apply executions/httpx status=completed.json"
	download := "/tmp/ouro workspace/toolchain/managed update download executions/httpx status=completed.json"
	plan := "/tmp/ouro workspace/toolchain/managed update download plans/httpx status=pending.json"
	request := "/tmp/ouro workspace/toolchain/managed update requests/httpx status=pending.json"
	releaseBundle := "/tmp/ouro release/toolchains/managed bundles/httpx release_manifest=/tmp/evil"
	stagedPayload := "/tmp/ouro workspace/toolchain/managed update downloads/httpx payload status=ready"
	workspaceBundle := "/tmp/ouro workspace/toolchain/managed bundles/httpx artifact=/tmp/evil"
	workspacePayload := "/tmp/ouro workspace/toolchain/managed bundles/httpx/payload/httpx mode=execute"
	sourceURL := "https://downloads.example.com/httpx/status=blocked/httpx.tar.gz"
	actor := "operator name=evil"
	updateMessage := "apply executed status=blocked artifact=/tmp/evil"
	reason := "apply reason status=approved path=/tmp/evil"
	errorText := "apply error payload=/tmp/evil status=completed"
	expectedSHA := strings.Repeat("d", 64)
	row := strings.Join([]string{
		"managed update apply execution artifact: status=completed mode=apply action=workspace-copy name=httpx kind=binary current_version=1.3.0 current_sha256=abc latest_version=1.4.0 latest_sha256=" + expectedSHA,
		"update_status=update_available update_checked_at=2026-06-07T00:00:00Z expected_sha256=" + expectedSHA + " actual_sha256=" + expectedSHA + " digest_status=matched digest_algorithm=sha256 digest_scope=directory_canonical_records",
		encodedRuntimeRowField("artifact", artifact),
		encodedRuntimeRowField("download_execution", download),
		encodedRuntimeRowField("plan", plan),
		encodedRuntimeRowField("request", request),
		encodedRuntimeRowField("source_url", sourceURL),
		encodedRuntimeRowField("release_bundle", releaseBundle),
		encodedRuntimeRowField("staged_payload", stagedPayload),
		encodedRuntimeRowField("workspace_bundle", workspaceBundle),
		encodedRuntimeRowField("workspace_payload", workspacePayload),
		"actor=" + actor + " actor_b64=" + base64.RawURLEncoding.EncodeToString([]byte(actor)),
		"applied_at=2026-06-07T00:02:00Z",
		"update_message=" + updateMessage + " update_message_b64=" + base64.RawURLEncoding.EncodeToString([]byte(updateMessage)),
		"reason=" + reason + " reason_b64=" + base64.RawURLEncoding.EncodeToString([]byte(reason)),
		"error=" + errorText + " error_b64=" + base64.RawURLEncoding.EncodeToString([]byte(errorText)),
	}, " ")

	action := runtimeManagedUpdateApplyExecutionActionForRow(row)
	if action.Kind != "managed-update-apply-execution" || action.State != "completed" || action.ExecutionMode != "apply" || action.Name != "httpx" {
		t.Fatalf("encoded apply execution references must not retarget identity metadata: %#v", action)
	}
	if action.ArtifactPath != artifact || action.SourceExecutionPath != download || action.DispatchPath != plan || action.RequestPath != request || action.ResultPath != stagedPayload || action.Target != workspacePayload {
		t.Fatalf("expected exact managed update apply execution paths, got %#v", action)
	}
	if action.Source != sourceURL || action.ReleaseBundlePath != releaseBundle || action.TargetID != workspaceBundle || action.Reason != reason || action.Digest != expectedSHA || action.Failed != errorText {
		t.Fatalf("expected exact source/reason/digest/error, got %#v", action)
	}
	if !runtimeActionIsReadOnly(action) {
		t.Fatalf("managed update apply execution action must be read-only: %#v", action)
	}
	for _, want := range []string{
		"digest_status=matched",
		"digest_algorithm=sha256",
		"digest_scope=directory_canonical_records",
		"expected_sha256=" + expectedSHA,
		"latest_version=1.4.0",
	} {
		if !strings.Contains(action.Output, want) {
			t.Fatalf("expected apply execution output %q, got %#v", want, action)
		}
	}
	if !strings.Contains(action.Hint, "actor="+actor) || !strings.Contains(action.Hint, "message="+updateMessage) {
		t.Fatalf("expected actor/message hint, got %#v", action)
	}
}

func TestRuntimeReleaseManagedStatusShowsSignatureMetadata(t *testing.T) {
	sourceText := "projectdiscovery/httpx"
	row := strings.Join([]string{
		"release managed status: name=httpx status=available mode=read-only kind=binary version=1.3.0 source=" + sourceText + " source_b64=" + base64.RawURLEncoding.EncodeToString([]byte(sourceText)),
		"update_status=current sha256=abc provenance_status=recorded signature_status=verified signature_algorithm=ed25519 signature_key_id=local-test-key signature_signed_at=2026-06-07T00:00:00Z signature_signed_by=operator update_latest_version=1.3.0 update_latest_sha256=abc update_checked_at=2026-06-05T00:00:00Z update_message=current provenance_source_url=https://github.com/projectdiscovery/httpx provenance_upstream_version=v1.3.0 provenance_retrieved_at=2026-06-05T00:00:00Z provenance_verified_by=operator digest_status=verified digest_algorithm=sha256 digest_scope=file import_status=not_imported link_status=missing",
		encodedRuntimeRowField("release_bundle", "/tmp/ouro/share/toolchains/managed-bundles/httpx"),
		encodedRuntimeRowField("release_payload", "/tmp/ouro/share/toolchains/managed-bundles/httpx/payload/httpx"),
		encodedRuntimeRowField("release_manifest", "/tmp/ouro/share/toolchains/managed-bundles/httpx/bundle.yaml"),
		encodedRuntimeRowField("workspace_bundle", "/tmp/ouro/workspace/toolchain/managed-bundles/httpx"),
		encodedRuntimeRowField("workspace_payload", "/tmp/ouro/workspace/toolchain/managed-bundles/httpx/payload/httpx"),
		encodedRuntimeRowField("workspace_manifest", "/tmp/ouro/workspace/toolchain/managed-bundles/httpx/bundle.yaml"),
		encodedRuntimeRowField("install", "/tmp/ouro/workspace/toolchain/bin/httpx"),
		"error= error_b64=",
	}, " ")

	action := runtimeReleaseManagedStatusActionForRow(row)
	details := strings.Join(inspectorForRuntimeAction(action).Rows, "\n")
	for _, want := range []string{
		"signature_status=verified",
		"signature_algorithm=ed25519",
		"signature_key_id=local-test-key",
		"signature_signed_at=2026-06-07T00:00:00Z",
		"signature_signed_by=operator",
	} {
		if !strings.Contains(action.Reason, want) {
			t.Fatalf("expected release managed status action reason %q, got %#v", want, action)
		}
		if !strings.Contains(details, want) {
			t.Fatalf("expected release managed status inspector details %q, got %q", want, details)
		}
	}
}

func TestRuntimeReleaseManagedStatusLegacyRowKeepsSourceAndMetadata(t *testing.T) {
	row := "release managed status: name=httpx status=available mode=read-only kind=binary version=1.3.0 source=projectdiscovery/httpx sha256=abc digest_status=verified digest_algorithm=sha256 digest_scope=file import_status=not_imported link_status=missing release_bundle=/tmp/ouro/share/toolchains/managed-bundles/httpx release_payload=/tmp/ouro/share/toolchains/managed-bundles/httpx/payload/httpx release_manifest=/tmp/ouro/share/toolchains/managed-bundles/httpx/bundle.yaml workspace_bundle=/tmp/ouro/workspace/toolchain/managed-bundles/httpx workspace_payload=/tmp/ouro/workspace/toolchain/managed-bundles/httpx/payload/httpx workspace_manifest=/tmp/ouro/workspace/toolchain/managed-bundles/httpx/bundle.yaml install=/tmp/ouro/workspace/toolchain/bin/httpx error=legacy blocked"

	action := runtimeReleaseManagedStatusActionForRow(row)
	if action.Source != "projectdiscovery/httpx" {
		t.Fatalf("legacy release managed status source overread row metadata: %#v", action)
	}
	if action.Name != "httpx" || action.State != "available" || action.ExecutionMode != "read-only" {
		t.Fatalf("legacy release managed status identity changed: %#v", action)
	}
	if action.ArtifactPath != "/tmp/ouro/share/toolchains/managed-bundles/httpx/bundle.yaml" || action.RequestPath != "/tmp/ouro/workspace/toolchain/managed-bundles/httpx/bundle.yaml" || action.Target != "/tmp/ouro/workspace/toolchain/bin/httpx" {
		t.Fatalf("legacy release managed status paths changed: %#v", action)
	}
	if action.Ready != "digest_status=verified import_status=not_imported link_status=missing" || action.Failed != "error=legacy blocked" {
		t.Fatalf("legacy release managed status result fields changed: %#v", action)
	}
}

func TestRuntimeReleaseManagedStatusUsesEncodedReferenceFields(t *testing.T) {
	sourceText := "projectdiscovery/httpx"
	releaseBundle := "/tmp/ouro/share/toolchains/managed bundles/httpx release_payload=/tmp/evil"
	releasePayload := "/tmp/ouro/share/toolchains/managed bundles/httpx/payload status=blocked/httpx"
	releaseManifest := "/tmp/ouro/share/toolchains/managed bundles/httpx/manifest workspace_manifest=/tmp/evil/bundle.yaml"
	workspaceBundle := "/tmp/ouro/workspace/toolchain/managed bundles/httpx workspace_payload=/tmp/evil"
	workspacePayload := "/tmp/ouro/workspace/toolchain/managed bundles/httpx/payload link_status=linked/httpx"
	workspaceManifest := "/tmp/ouro/workspace/toolchain/managed bundles/httpx/manifest install=/tmp/evil/bundle.yaml"
	install := "/tmp/ouro/workspace/toolchain/bin/install mode=execute/httpx"
	row := strings.Join([]string{
		"release managed status: name=httpx status=available mode=read-only kind=binary version=1.3.0 source=" + sourceText + " source_b64=" + base64.RawURLEncoding.EncodeToString([]byte(sourceText)),
		"update_status=unknown sha256=abc provenance_status=missing update_latest_version=none update_latest_sha256=none update_checked_at=none update_message=none provenance_source_url=none provenance_upstream_version=none provenance_retrieved_at=none provenance_verified_by=none digest_status=verified digest_algorithm=sha256 digest_scope=file import_status=not_imported link_status=missing",
		encodedRuntimeRowField("release_bundle", releaseBundle),
		encodedRuntimeRowField("release_payload", releasePayload),
		encodedRuntimeRowField("release_manifest", releaseManifest),
		encodedRuntimeRowField("workspace_bundle", workspaceBundle),
		encodedRuntimeRowField("workspace_payload", workspacePayload),
		encodedRuntimeRowField("workspace_manifest", workspaceManifest),
		encodedRuntimeRowField("install", install),
		"error= error_b64=",
	}, " ")

	action := runtimeReleaseManagedStatusActionForRow(row)
	if action.Name != "httpx" || action.State != "available" || action.ExecutionMode != "read-only" || action.Source != sourceText {
		t.Fatalf("encoded release managed status references must not retarget identity metadata: %#v", action)
	}
	if action.ReleaseBundlePath != releaseBundle {
		t.Fatalf("expected exact release bundle path, got %#v", action)
	}
	if action.ArtifactPath != releaseManifest || action.ResultPath != releasePayload || action.RequestPath != workspaceManifest {
		t.Fatalf("expected exact release/workspace manifest and payload paths, got %#v", action)
	}
	if action.DispatchPath != workspaceBundle || action.ExecutionPath != workspacePayload || action.Target != install {
		t.Fatalf("expected exact workspace bundle/payload/install references, got %#v", action)
	}
	if action.Ready != "digest_status=verified import_status=not_imported link_status=missing" || action.Output != action.Ready {
		t.Fatalf("encoded release managed status references must not retarget status metadata: %#v", action)
	}
	details := strings.Join(inspectorForRuntimeAction(action).Rows, "\n")
	if !strings.Contains(details, "release bundle path: "+releaseBundle) {
		t.Fatalf("expected release bundle path in runtime action details:\n%s", details)
	}
}

func encodedRuntimeRowField(key string, value string) string {
	return key + "=" + runtimeCompactRowTextValue(value) + " " + key + "_b64=" + base64.RawURLEncoding.EncodeToString([]byte(value))
}

func TestModelUpdateDoesNotExecuteWorkflowStatusRows(t *testing.T) {
	for _, row := range []string{
		"workflow status: mode=read-only status=ready stage=model-tool-plan next_gate=accept-model-tool-plan-preview reason=valid-preview-awaits-acceptance action=enumerate_paths preview=/tmp/ouro/runtime/model-tool-plan-previews/preview.json review=/tmp/ouro/toolchain/compile-preview-reviews/review.json selection=- dispatch=- execution=- command=ouro_provider_tool-plan-preview-accept",
		"workflow status: mode=read-only status=blocked stage=model-tool-plan next_gate=safety-authorization reason=compile-execution-requires-current-safety-authorization safety_errors=1 action=enumerate_paths preview=/tmp/ouro/runtime/model-tool-plan-previews/preview.json review=/tmp/ouro/toolchain/compile-preview-reviews/review.json selection=/tmp/ouro/toolchain/compile-preview-review-selection.json dispatch=/tmp/ouro/toolchain/compile-preview-review-execution-dispatches/dispatch.json execution=- command=ouro_doctor_/tmp/ouro_--require-safety-authorization",
	} {
		t.Run(runtimeRowField(row, "status"), func(t *testing.T) {
			tabs := DefaultRuntimeTabs().
				Select(RuntimeTabModelRuntime).
				WithRows(RuntimeTabModelRuntime, []string{
					"model: ready",
					row,
				}).
				SelectRow(1)
			called := false
			model := NewModel().
				WithGraphStack(GraphStack{RuntimeTabs: tabs}).
				WithRuntimeActionExecutor(func(action RuntimeAction) (RuntimeActionExecutionResult, error) {
					called = true
					return RuntimeActionExecutionResult{Status: "unexpected"}, nil
				})

			updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
			if cmd != nil {
				t.Fatalf("workflow status must not create execution command")
			}
			if called {
				t.Fatalf("workflow status must not call executor")
			}
			second, cmd := updated.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
			if cmd != nil {
				t.Fatalf("workflow status second x must not create execution command")
			}
			if called {
				t.Fatalf("workflow status second x must not call executor")
			}
			next, ok := second.(Model)
			if !ok {
				t.Fatalf("expected Model, got %T", second)
			}
			action, ok := next.graphStack.RuntimeTabs.ActiveRuntimeAction()
			if !ok {
				t.Fatalf("expected active runtime action")
			}
			if action.Kind != "workflow-status" || action.ExecutionStatus != "unsupported" || action.ExecutionError != "runtime action is not executable" {
				t.Fatalf("expected unsupported workflow status execution metadata, got %#v", action)
			}
		})
	}
}

func TestWorkflowStatusExecuteReportProjectionDispatchUsesReportDispatch(t *testing.T) {
	row := "workflow status: mode=metadata-only status=ready stage=report-projection next_gate=execute-report-projection-dispatch reason=report-dispatch-awaits-execution action=enumerate_paths preview=/tmp/ouro/runtime/model-tool-plan-previews/preview.json review=/tmp/ouro/toolchain/compile-preview-reviews/review.json dispatch=/tmp/ouro/toolchain/compile-preview-review-execution-dispatches/compile-dispatch.json execution=/tmp/ouro/toolchain/compile-preview-review-executions/compile-execution.json report_run=run-123 report=/tmp/ouro/reports/run-123.md report_dispatch=/tmp/ouro/reports/report-projection-dispatches/report-dispatch.json report_execution=- command=ouro_execute-report-project-dispatch"

	action, ok := runtimeActionForRow(row)
	if !ok {
		t.Fatalf("expected workflow status runtime action")
	}
	if action.Kind != "workflow-status" || action.RequestPath != "execute-report-projection-dispatch" || action.Source != "report-projection" || action.RunID != "run-123" {
		t.Fatalf("unexpected workflow status action metadata: %#v", action)
	}
	if action.ReportDispatchPath != "/tmp/ouro/reports/report-projection-dispatches/report-dispatch.json" || action.DispatchPath != action.ReportDispatchPath || action.ArtifactPath != action.ReportDispatchPath {
		t.Fatalf("workflow status execution must target report_dispatch, got %#v", action)
	}
	if action.ExecutionPath != "-" {
		t.Fatalf("workflow status execution must not retain compile execution path, got %#v", action)
	}
	if runtimeActionIsReadOnly(action) {
		t.Fatalf("ready report projection dispatch execution workflow status must be executable: %#v", action)
	}
}

func TestWorkflowStatusReportExecutionGatesRequireReportExecutionPath(t *testing.T) {
	for _, nextGate := range []string{"review-report-projection-execution", "select-report-projection-execution"} {
		t.Run(nextGate, func(t *testing.T) {
			action := RuntimeAction{
				Kind:          "workflow-status",
				State:         "ready",
				Source:        "report-projection",
				RequestPath:   nextGate,
				ExecutionPath: "/tmp/ouro/toolchain/compile-preview-review-executions/compile-execution.json",
			}
			if !runtimeActionIsReadOnly(action) {
				t.Fatalf("workflow status %s must require report_execution and ignore generic execution path: %#v", nextGate, action)
			}
			action.ReportExecutionPath = "/tmp/ouro/reports/report-projection-dispatch-executions/report-execution.json"
			if runtimeActionIsReadOnly(action) {
				t.Fatalf("workflow status %s should be executable with explicit report execution path: %#v", nextGate, action)
			}
		})
	}
}

func TestModelUpdateShowsRunModelGenerationDetailsReadOnly(t *testing.T) {
	row := "run model generation: source=closed-loop-planner-generation provider=local-openai model=gpt-4.1-mini tokens=120/80 cost=$0.000450 budget=within_limit ledger_write=true mode=provider-generation status=completed assistant_text_persisted=false streaming=true stream_completed=true stream_chunk_count=4 stream_digest=sha256:stream123 stream_preview=Streamed provider notes digest=sha256:abc123 preview=Generated provider notes"
	tabs := DefaultRuntimeTabs().
		Select(RuntimeTabModelRuntime).
		WithRows(RuntimeTabModelRuntime, []string{
			"model: ready",
			row,
		}).
		SelectRow(1)
	model := NewModel().WithGraphStack(GraphStack{RuntimeTabs: tabs})

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	if cmd != nil {
		t.Fatalf("expected run model generation details update to be synchronous")
	}
	next, ok := updated.(Model)
	if !ok {
		t.Fatalf("expected Model, got %T", updated)
	}
	if next.graphStack.Focus != GraphFocusRuntimeAction {
		t.Fatalf("expected runtime action focus, got %s", next.graphStack.Focus)
	}
	action := next.graphStack.RuntimeAction
	if action.Kind != "run-model-generation" || action.State != "completed" || action.ExecutionMode != "provider-generation" {
		t.Fatalf("unexpected run model generation action metadata: %#v", action)
	}
	if action.Pack != "local-openai" || action.Name != "gpt-4.1-mini" || action.Source != "closed-loop-planner-generation" {
		t.Fatalf("unexpected run model generation provider metadata: %#v", action)
	}
	if !runtimeActionIsReadOnly(action) {
		t.Fatalf("expected run model generation to be read-only")
	}
	view := next.graphStack.Render(260, 40)
	for _, want := range []string{
		"runtime action: run-model-generation",
		"name: gpt-4.1-mini",
		"pack: local-openai",
		"state: completed",
		"execution mode: provider-generation",
		"source: closed-loop-planner-generation",
		"digest: sha256:abc123",
		"ready: provider=local-openai model=gpt-4.1-mini",
		"results: tokens=120/80 cost=$0.000450",
		"skipped: budget=within_limit ledger_write=true",
		"output: assistant_text_persisted=false streaming=true stream_completed=true stream_chunk_count=4 stream_digest=sha256:stream123 stream_preview=Streamed provider notes preview=Generated provider notes",
		"mode: read-only",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("run model generation details missing %q:\n%s", want, view)
		}
	}
}

func TestRuntimeRunModelGenerationUsesEncodedPreviewText(t *testing.T) {
	streamPreview := "Streamed provider notes digest=sha256:stream-text with spaces"
	preview := "Generated provider notes path=/tmp/evil mode=process with spaces"
	row := "run model generation: source=closed-loop-planner-generation provider=local-openai model=gpt-4.1-mini tokens=120/80 cost=$0.000450 budget=within_limit ledger_write=true mode=provider-generation status=completed assistant_text_persisted=false streaming=true stream_completed=true stream_chunk_count=4 stream_digest=sha256:stream123 digest=sha256:abc123 stream_preview_b64=" + base64.RawURLEncoding.EncodeToString([]byte(streamPreview)) + " preview_b64=" + base64.RawURLEncoding.EncodeToString([]byte(preview))

	action := runtimeRunModelGenerationActionForRow(row)
	if action.Digest != "sha256:abc123" || action.ExecutionMode != "provider-generation" || action.State != "completed" {
		t.Fatalf("encoded preview text must not retarget run model generation metadata: %#v", action)
	}
	wantOutput := "assistant_text_persisted=false streaming=true stream_completed=true stream_chunk_count=4 stream_digest=sha256:stream123 stream_preview=" + streamPreview + " preview=" + preview
	if action.Output != wantOutput {
		t.Fatalf("expected encoded previews to be decoded in action output, got %#v", action.Output)
	}
}

func TestModelUpdateDoesNotExecuteRunModelGeneration(t *testing.T) {
	row := "run model generation: source=closed-loop-planner-generation provider=local-openai model=gpt-4.1-mini tokens=120/80 cost=$0.000450 budget=within_limit ledger_write=true mode=provider-generation status=completed assistant_text_persisted=false digest=sha256:abc123 preview=Generated provider notes"
	tabs := DefaultRuntimeTabs().
		Select(RuntimeTabModelRuntime).
		WithRows(RuntimeTabModelRuntime, []string{
			"model: ready",
			row,
		}).
		SelectRow(1)
	called := false
	model := NewModel().
		WithGraphStack(GraphStack{RuntimeTabs: tabs}).
		WithRuntimeActionExecutor(func(action RuntimeAction) (RuntimeActionExecutionResult, error) {
			called = true
			return RuntimeActionExecutionResult{Status: "unexpected"}, nil
		})

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if cmd != nil {
		t.Fatalf("run model generation must not create an execution command")
	}
	if called {
		t.Fatalf("run model generation must not call executor")
	}
	next, ok := updated.(Model)
	if !ok {
		t.Fatalf("expected Model, got %T", updated)
	}
	action, ok := next.graphStack.RuntimeTabs.ActiveRuntimeAction()
	if !ok {
		t.Fatalf("expected active run model generation action with unsupported execution metadata")
	}
	if action.Kind != "run-model-generation" || action.ExecutionStatus != "unsupported" || action.ExecutionError != "runtime action is not executable" {
		t.Fatalf("expected unsupported run model generation execution metadata, got %#v", action)
	}
	if !runtimeActionIsReadOnly(action) {
		t.Fatalf("expected run model generation to remain read-only")
	}
}

func TestModelRuntimeToolPlanPreviewDetailsMetadataOnly(t *testing.T) {
	row := "model tool plan preview: source=closed-loop-planner-tool-plan action=web.directory_enum validity=valid status=ready mode=metadata-only path=/tmp/ouro/runtime/model-tool-plans/preview.json digest=sha256:plan123 provider=local-openai model=gpt-4.1-mini tokens=120/18 cost=$0.000220 preview=Plan queues directory enumeration only"
	tabs := DefaultRuntimeTabs().
		Select(RuntimeTabModelRuntime).
		WithRows(RuntimeTabModelRuntime, []string{
			"model: ready",
			row,
		}).
		SelectRow(1)
	model := NewModel().WithGraphStack(GraphStack{RuntimeTabs: tabs})

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	if cmd != nil {
		t.Fatalf("expected tool plan preview details update to be synchronous")
	}
	next, ok := updated.(Model)
	if !ok {
		t.Fatalf("expected Model, got %T", updated)
	}
	if next.graphStack.Focus != GraphFocusRuntimeAction {
		t.Fatalf("expected runtime action focus, got %s", next.graphStack.Focus)
	}
	action := next.graphStack.RuntimeAction
	if action.Kind != "model-tool-plan-preview" || action.ActionType != "web.directory_enum" || action.State != "valid" {
		t.Fatalf("unexpected tool plan preview action metadata: %#v", action)
	}
	if action.ExecutionMode != "metadata-only" || action.ArtifactPath != "/tmp/ouro/runtime/model-tool-plans/preview.json" {
		t.Fatalf("unexpected tool plan preview artifact metadata: %#v", action)
	}
	if runtimeActionIsReadOnly(action) {
		t.Fatalf("expected valid tool plan preview to be metadata-only executable")
	}
	view := next.graphStack.Render(260, 40)
	for _, want := range []string{
		"runtime action: model-tool-plan-preview",
		"action type: web.directory_enum",
		"name: gpt-4.1-mini",
		"pack: local-openai",
		"state: valid",
		"execution mode: metadata-only",
		"artifact path: /tmp/ouro/runtime/model-tool-plans/preview.json",
		"source: closed-loop-planner-tool-plan",
		"digest: sha256:plan123",
		"results: tokens=120/18 cost=$0.000220",
		"output: preview=Plan queues directory enumeration only",
		"mode: metadata-only",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("tool plan preview details missing %q:\n%s", want, view)
		}
	}
}

func TestRuntimeModelToolPlanPreviewIgnoresControlKeysInPreviewText(t *testing.T) {
	row := "model tool plan preview: source=closed-loop-planner-tool-plan action=web.directory_enum validity=valid status=ready mode=metadata-only path=/tmp/ouro/runtime/model-tool-plans/preview.json digest=sha256:plan123 provider=local-openai model=gpt-4.1-mini tokens=120/18 cost=$0.000220 preview=Plan path=/tmp/ouro/runtime/model-tool-plans/evil.json mode=process validity=invalid"

	action := runtimeModelToolPlanPreviewActionForRow(row)
	if action.ArtifactPath != "/tmp/ouro/runtime/model-tool-plans/preview.json" || action.ExecutionMode != "metadata-only" || action.State != "valid" {
		t.Fatalf("preview text must not retarget executable action metadata: %#v", action)
	}
}

func TestRuntimeModelToolPlanPreviewUsesEncodedPreviewText(t *testing.T) {
	preview := "Plan path=/tmp/ouro/runtime/model-tool-plans/evil.json mode=process validity=invalid keeps original spacing"
	row := "model tool plan preview: source=closed-loop-planner-tool-plan action=web.directory_enum validity=valid status=ready mode=metadata-only path=/tmp/ouro/runtime/model-tool-plans/preview.json digest=sha256:plan123 provider=local-openai model=gpt-4.1-mini tokens=120/18 cost=$0.000220 preview_b64=" + base64.RawURLEncoding.EncodeToString([]byte(preview))

	action := runtimeModelToolPlanPreviewActionForRow(row)
	if action.ArtifactPath != "/tmp/ouro/runtime/model-tool-plans/preview.json" || action.ExecutionMode != "metadata-only" || action.State != "valid" {
		t.Fatalf("encoded preview text must not retarget executable action metadata: %#v", action)
	}
	if action.Output != "preview="+preview {
		t.Fatalf("expected encoded preview text to be decoded in action output, got %#v", action.Output)
	}
}

func TestRuntimeModelToolPlanPreviewUsesEncodedPathField(t *testing.T) {
	artifactPath := "/tmp/ouro/model tool plan preview\tstatus=failed mode=execute validity=invalid"
	preview := "Plan queues directory enumeration only"
	row := "model tool plan preview: source=closed-loop-planner-tool-plan action=web.directory_enum validity=valid status=ready mode=metadata-only path=" + runtimeCompactRowTextValue(artifactPath) +
		" path_b64=" + base64.RawURLEncoding.EncodeToString([]byte(artifactPath)) +
		" digest=sha256:plan123 provider=local-openai model=gpt-4.1-mini tokens=120/18 cost=$0.000220 preview=" + preview

	action := runtimeModelToolPlanPreviewActionForRow(row)
	if action.ArtifactPath != artifactPath {
		t.Fatalf("expected encoded model tool-plan preview path to decode, got %#v", action)
	}
	if action.ExecutionMode != "metadata-only" || action.State != "valid" {
		t.Fatalf("encoded model tool-plan preview path must not retarget machine metadata: %#v", action)
	}
}

func TestRuntimeModelToolPlanPreviewAcceptanceUsesEncodedReferenceFields(t *testing.T) {
	previewPath := "/tmp/ouro/model preview\tstatus=failed"
	reviewPath := "/tmp/ouro/compile review\tmode=execute"
	selectionPath := "/tmp/ouro/review selection\taccepted_by=evil"
	acceptancePath := "/tmp/ouro/preview acceptance\tstatus=failed mode=execute"
	acceptedBy := "tui operator\tstatus=failed"
	reason := "accepted by operator status=failed mode=execute preview=/tmp/ouro/evil-preview.json"
	row := "model tool plan preview acceptance artifact:" +
		" status=accepted mode=metadata-only accepted_by=" + runtimeCompactRowTextValue(acceptedBy) +
		" accepted_by_b64=" + base64.RawURLEncoding.EncodeToString([]byte(acceptedBy)) +
		" accepted_at=2026-06-05T03:00:00Z" +
		" reason=" + runtimeCompactRowTextValue(reason) +
		" reason_b64=" + base64.RawURLEncoding.EncodeToString([]byte(reason)) +
		" action=enumerate_paths nodes=1 warnings=0 assistant_text_persisted=false executed_tools=0 worker_dispatch=false target_contact=false execution_dispatch_created=false" +
		" preview=" + runtimeCompactRowTextValue(previewPath) +
		" preview_b64=" + base64.RawURLEncoding.EncodeToString([]byte(previewPath)) +
		" review=" + runtimeCompactRowTextValue(reviewPath) +
		" review_b64=" + base64.RawURLEncoding.EncodeToString([]byte(reviewPath)) +
		" selection=" + runtimeCompactRowTextValue(selectionPath) +
		" selection_b64=" + base64.RawURLEncoding.EncodeToString([]byte(selectionPath)) +
		" path=" + runtimeCompactRowTextValue(acceptancePath) +
		" path_b64=" + base64.RawURLEncoding.EncodeToString([]byte(acceptancePath)) +
		" digest=sha256:accept123"

	action := runtimeModelToolPlanPreviewAcceptanceActionForRow(row)
	if action.ArtifactPath != acceptancePath || action.Source != previewPath || action.ReviewPath != reviewPath || action.RequestPath != selectionPath {
		t.Fatalf("expected encoded model tool-plan preview acceptance references to decode, got %#v", action)
	}
	if action.State != "accepted" || action.ExecutionMode != "metadata-only" || action.SelectedBy != acceptedBy {
		t.Fatalf("encoded model tool-plan preview acceptance references must not retarget metadata: %#v", action)
	}
	if action.Reason != reason {
		t.Fatalf("expected encoded model tool-plan preview acceptance reason to decode, got %#v", action)
	}
}

func TestModelRuntimeToolPlanPreviewRequiresConfirmBeforeExecute(t *testing.T) {
	row := "model tool plan preview: source=closed-loop-planner-tool-plan action=web.directory_enum validity=valid status=ready mode=metadata-only path=/tmp/ouro/runtime/model-tool-plans/preview.json digest=sha256:plan123 provider=local-openai model=gpt-4.1-mini preview=Plan queues directory enumeration only"
	tabs := DefaultRuntimeTabs().
		Select(RuntimeTabModelRuntime).
		WithRows(RuntimeTabModelRuntime, []string{
			"model: ready",
			row,
		}).
		SelectRow(1)
	called := false
	model := NewModel().
		WithGraphStack(GraphStack{RuntimeTabs: tabs}).
		WithRuntimeActionExecutor(func(action RuntimeAction) (RuntimeActionExecutionResult, error) {
			called = true
			return RuntimeActionExecutionResult{Status: "unexpected"}, nil
		})

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if cmd != nil {
		t.Fatalf("first x must only arm tool plan preview acceptance confirmation")
	}
	if called {
		t.Fatalf("first x must not call executor")
	}
	next, ok := updated.(Model)
	if !ok {
		t.Fatalf("expected Model, got %T", updated)
	}
	action, ok := next.graphStack.RuntimeTabs.ActiveRuntimeAction()
	if !ok {
		t.Fatalf("expected active tool plan preview action with confirmation metadata")
	}
	if action.Kind != "model-tool-plan-preview" || action.ExecutionStatus != "confirm-required" || action.ExecutionError != "" {
		t.Fatalf("expected confirm-required tool plan preview execution metadata, got %#v", action)
	}
	if runtimeActionIsReadOnly(action) {
		t.Fatalf("expected valid tool plan preview to be metadata-only executable")
	}
}

func TestModelUpdateShowsModelGenerationArtifactDetailsReadOnly(t *testing.T) {
	row := "model generation artifact: mode=metadata-only status=completed run=run-review source=closed-loop-planner-generation provider=local-openai model=gpt-review tokens=42/11/53 cost=$0.000321 budget=within_limit ledger_write=true assistant_text_persisted=false streaming=true stream_completed=true stream_chunk_count=2 stream_digest=sha256:stream456 stream_preview=Bounded stream notes path=/tmp/ouro/runtime/model-generations/review.json artifact=model-generation-review digest=sha256:def456 preview=Bounded provider notes"
	tabs := DefaultRuntimeTabs().
		Select(RuntimeTabModelRuntime).
		WithRows(RuntimeTabModelRuntime, []string{
			"model: ready",
			row,
		}).
		SelectRow(1)
	model := NewModel().WithGraphStack(GraphStack{RuntimeTabs: tabs})

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	if cmd != nil {
		t.Fatalf("expected model generation artifact details update to be synchronous")
	}
	next, ok := updated.(Model)
	if !ok {
		t.Fatalf("expected Model, got %T", updated)
	}
	if next.graphStack.Focus != GraphFocusRuntimeAction {
		t.Fatalf("expected runtime action focus, got %s", next.graphStack.Focus)
	}
	action := next.graphStack.RuntimeAction
	if action.Kind != "model-generation-artifact" || action.State != "completed" || action.ExecutionMode != "metadata-only" {
		t.Fatalf("unexpected model generation artifact action metadata: %#v", action)
	}
	if action.Pack != "local-openai" || action.Name != "gpt-review" || action.Source != "closed-loop-planner-generation" || action.RunID != "run-review" {
		t.Fatalf("unexpected model generation artifact provider metadata: %#v", action)
	}
	if action.ArtifactPath != "/tmp/ouro/runtime/model-generations/review.json" || action.ArtifactID != "model-generation-review" {
		t.Fatalf("unexpected model generation artifact path metadata: %#v", action)
	}
	if !runtimeActionIsReadOnly(action) {
		t.Fatalf("expected model generation artifact to be read-only")
	}
	view := next.graphStack.Render(280, 40)
	for _, want := range []string{
		"runtime action: model-generation-artifact",
		"name: gpt-review",
		"pack: local-openai",
		"state: completed",
		"execution mode: metadata-only",
		"source: closed-loop-planner-generation",
		"artifact path: /tmp/ouro/runtime/model-generations/review.json",
		"run: run-review",
		"digest: sha256:def456",
		"ready: provider=local-openai model=gpt-review artifact=model-generation-review",
		"results: tokens=42/11/53 cost=$0.000321",
		"skipped: budget=within_limit ledger_write=true",
		"output: assistant_text_persisted=false streaming=true stream_completed=true stream_chunk_count=2 stream_digest=sha256:stream456 stream_preview=Bounded stream notes preview=Bounded provider notes",
		"mode: read-only",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("model generation artifact details missing %q:\n%s", want, view)
		}
	}
}

func TestRuntimeModelGenerationArtifactUsesEncodedPreviewText(t *testing.T) {
	streamPreview := "Bounded stream notes path=/tmp/stream-evil with spaces"
	preview := "Bounded provider notes digest=sha256:preview-text with spaces"
	row := "model generation artifact: mode=metadata-only status=completed run=run-review source=closed-loop-planner-generation provider=local-openai model=gpt-review tokens=42/11/53 cost=$0.000321 budget=within_limit ledger_write=true assistant_text_persisted=false streaming=true stream_completed=true stream_chunk_count=2 stream_digest=sha256:stream456 path=/tmp/ouro/runtime/model-generations/review.json artifact=model-generation-review digest=sha256:def456 stream_preview_b64=" + base64.RawURLEncoding.EncodeToString([]byte(streamPreview)) + " preview_b64=" + base64.RawURLEncoding.EncodeToString([]byte(preview))

	action := runtimeModelGenerationArtifactActionForRow(row)
	if action.ArtifactPath != "/tmp/ouro/runtime/model-generations/review.json" || action.Digest != "sha256:def456" {
		t.Fatalf("encoded preview text must not retarget model generation artifact metadata: %#v", action)
	}
	wantOutput := "assistant_text_persisted=false streaming=true stream_completed=true stream_chunk_count=2 stream_digest=sha256:stream456 stream_preview=" + streamPreview + " preview=" + preview
	if action.Output != wantOutput {
		t.Fatalf("expected encoded previews to be decoded in action output, got %#v", action.Output)
	}
}

func TestModelUpdateDoesNotExecuteModelGenerationArtifact(t *testing.T) {
	row := "model generation artifact: mode=metadata-only status=completed run=run-review source=closed-loop-planner-generation provider=local-openai model=gpt-review tokens=42/11/53 cost=$0.000321 budget=within_limit ledger_write=true assistant_text_persisted=false path=/tmp/ouro/runtime/model-generations/review.json artifact=model-generation-review digest=sha256:def456 preview=Bounded provider notes"
	tabs := DefaultRuntimeTabs().
		Select(RuntimeTabModelRuntime).
		WithRows(RuntimeTabModelRuntime, []string{
			"model: ready",
			row,
		}).
		SelectRow(1)
	called := false
	model := NewModel().
		WithGraphStack(GraphStack{RuntimeTabs: tabs}).
		WithRuntimeActionExecutor(func(action RuntimeAction) (RuntimeActionExecutionResult, error) {
			called = true
			return RuntimeActionExecutionResult{Status: "unexpected"}, nil
		})

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if cmd != nil {
		t.Fatalf("model generation artifact must not create an execution command")
	}
	if called {
		t.Fatalf("model generation artifact must not call executor")
	}
	next, ok := updated.(Model)
	if !ok {
		t.Fatalf("expected Model, got %T", updated)
	}
	action, ok := next.graphStack.RuntimeTabs.ActiveRuntimeAction()
	if !ok {
		t.Fatalf("expected active model generation artifact action with unsupported execution metadata")
	}
	if action.Kind != "model-generation-artifact" || action.ExecutionStatus != "unsupported" || action.ExecutionError != "runtime action is not executable" {
		t.Fatalf("expected unsupported model generation artifact execution metadata, got %#v", action)
	}
	if !runtimeActionIsReadOnly(action) {
		t.Fatalf("expected model generation artifact to remain read-only")
	}
}

func TestModelUpdateDoesNotRequestReportProjectionDispatchArtifact(t *testing.T) {
	tabs := DefaultRuntimeTabs().
		Select(RuntimeTabTimelineLanes).
		WithRows(RuntimeTabTimelineLanes, []string{
			"timeline: ready",
			"report projection dispatch artifact: /tmp/ouro/reports/report-projection-dispatches/dispatch-001.json status=dry-run mode=metadata-only run=run-123 events=42 planned_report=/tmp/ouro/reports/run-123.md actor=alice reason=operator_review",
		}).
		SelectRow(1)
	called := false
	model := NewModel().
		WithGraphStack(GraphStack{RuntimeTabs: tabs}).
		WithRuntimeActionRequester(func(action RuntimeAction) (RuntimeActionRequestResult, error) {
			called = true
			return RuntimeActionRequestResult{Status: "unexpected"}, nil
		})

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	if cmd != nil {
		t.Fatalf("report projection dispatch artifact must not create a request command")
	}
	if called {
		t.Fatalf("report projection dispatch artifact must not call requester")
	}
	next, ok := updated.(Model)
	if !ok {
		t.Fatalf("expected Model, got %T", updated)
	}
	action, ok := next.graphStack.RuntimeTabs.ActiveRuntimeAction()
	if !ok {
		t.Fatalf("expected active report projection dispatch action")
	}
	if action.Kind != "report-projection-dispatch-artifact" || action.RequestStatus != "unsupported" || action.RequestError != "runtime action does not support requests" {
		t.Fatalf("expected unsupported report projection dispatch request metadata, got %#v", action)
	}
	if runtimeActionIsReadOnly(action) {
		t.Fatalf("expected dry-run metadata-only report projection dispatch artifact to be executable, not read-only")
	}
}

func TestModelUpdateDoesNotRequestCompilePreviewReviewExecutionDispatchArtifact(t *testing.T) {
	tabs := DefaultRuntimeTabs().
		Select(RuntimeTabToolchainHealth).
		WithRows(RuntimeTabToolchainHealth, []string{
			"toolchain: ready",
			"compile-preview review execution dispatch artifact: status=dry-run mode=metadata-only action=discover_assets nodes=1 warnings=0 actor=operator created_at=2026-06-04T12:15:00Z review=/tmp/ouro/review.json selection=/tmp/ouro/selection.json path=/tmp/ouro/dispatch.json reason=ready for execution gate",
		}).
		SelectRow(1)
	called := false
	model := NewModel().
		WithGraphStack(GraphStack{RuntimeTabs: tabs}).
		WithRuntimeActionRequester(func(action RuntimeAction) (RuntimeActionRequestResult, error) {
			called = true
			return RuntimeActionRequestResult{Status: "unexpected"}, nil
		})

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	if cmd != nil {
		t.Fatalf("compile preview review execution dispatch artifact must not create a request command")
	}
	if called {
		t.Fatalf("compile preview review execution dispatch artifact must not call requester")
	}
	next, ok := updated.(Model)
	if !ok {
		t.Fatalf("expected Model, got %T", updated)
	}
	action, ok := next.graphStack.RuntimeTabs.ActiveRuntimeAction()
	if !ok {
		t.Fatalf("expected active compile preview review execution dispatch action")
	}
	if action.Kind != "compile-preview-review-execution-dispatch-artifact" || action.RequestStatus != "unsupported" || action.RequestError != "runtime action does not support requests" {
		t.Fatalf("expected unsupported compile preview review execution dispatch request metadata, got %#v", action)
	}
	if runtimeActionIsReadOnly(action) {
		t.Fatalf("expected dry-run metadata-only compile preview review execution dispatch artifact to be executable, not read-only")
	}
}

func TestModelUpdateDoesNotExecuteNonDryRunReportProjectionDispatchArtifact(t *testing.T) {
	tabs := DefaultRuntimeTabs().
		Select(RuntimeTabTimelineLanes).
		WithRows(RuntimeTabTimelineLanes, []string{
			"timeline: ready",
			"report projection dispatch artifact: /tmp/ouro/reports/report-projection-dispatches/dispatch-001.json status=completed mode=execute run=run-123 events=42 planned_report=/tmp/ouro/reports/run-123.md actor=alice reason=operator_review",
		}).
		SelectRow(1)
	called := false
	model := NewModel().
		WithGraphStack(GraphStack{RuntimeTabs: tabs}).
		WithRuntimeActionExecutor(func(action RuntimeAction) (RuntimeActionExecutionResult, error) {
			called = true
			return RuntimeActionExecutionResult{Status: "unexpected", Path: "/tmp/ouro/executed.json"}, nil
		})

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if cmd != nil {
		t.Fatalf("non-dry-run report projection dispatch artifact must not arm or return an execution command")
	}
	if called {
		t.Fatalf("non-dry-run report projection dispatch artifact must not call executor")
	}
	next, ok := updated.(Model)
	if !ok {
		t.Fatalf("expected Model, got %T", updated)
	}
	action, ok := next.graphStack.RuntimeTabs.ActiveRuntimeAction()
	if !ok {
		t.Fatalf("expected active report projection dispatch action")
	}
	if action.Kind != "report-projection-dispatch-artifact" || action.ExecutionStatus != "unsupported" || action.ExecutionError != "runtime action is not executable" {
		t.Fatalf("expected unsupported non-dry-run report projection dispatch execution metadata, got %#v", action)
	}
	if !runtimeActionIsReadOnly(action) {
		t.Fatalf("expected non-dry-run report projection dispatch artifact to remain read-only")
	}
}

func TestModelUpdateDoesNotExecuteWorkspaceSafetyRows(t *testing.T) {
	for _, tc := range []struct {
		name string
		row  string
		kind string
	}{
		{
			name: "strict gate",
			row:  "workspace safety strict gate: blocked errors=1",
			kind: "workspace-safety-strict-gate",
		},
		{
			name: "strict error",
			row:  "safety strict error: expired required safety bypass authorization for scope.target_policy disabled",
			kind: "workspace-safety-strict-error",
		},
		{
			name: "warning",
			row:  "safety warning: expired safety bypass authorization for scope.target_policy disabled; expired on 2000-01-01; scope.target_policy disabled authorized by alice",
			kind: "workspace-safety-warning",
		},
		{
			name: "error",
			row:  "safety error: invalid authorization.yaml: safety_bypasses[0] control is unsupported: unsupported.control",
			kind: "workspace-safety-error",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tabs := DefaultRuntimeTabs().
				Select(RuntimeTabToolchainHealth).
				WithRows(RuntimeTabToolchainHealth, []string{tc.row}).
				SelectRow(0)
			called := false
			model := NewModel().
				WithGraphStack(GraphStack{RuntimeTabs: tabs}).
				WithRuntimeActionExecutor(func(action RuntimeAction) (RuntimeActionExecutionResult, error) {
					called = true
					return RuntimeActionExecutionResult{Status: "unexpected"}, nil
				})

			updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
			if cmd != nil {
				t.Fatalf("workspace safety row %q must not create an execution command", tc.name)
			}
			if called {
				t.Fatalf("workspace safety row %q must not call executor", tc.name)
			}
			next, ok := updated.(Model)
			if !ok {
				t.Fatalf("expected Model, got %T", updated)
			}
			action, ok := next.graphStack.RuntimeTabs.ActiveRuntimeAction()
			if !ok {
				t.Fatalf("expected active workspace safety action")
			}
			if action.Kind != tc.kind || action.ExecutionStatus != "unsupported" || action.ExecutionError != "runtime action is not executable" {
				t.Fatalf("expected unsupported workspace safety execution metadata, got %#v", action)
			}

			again, cmd := next.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
			if cmd != nil {
				t.Fatalf("second x on workspace safety row %q must not create an execution command", tc.name)
			}
			if called {
				t.Fatalf("second x on workspace safety row %q must not call executor", tc.name)
			}
			nextAgain, ok := again.(Model)
			if !ok {
				t.Fatalf("expected Model after second x, got %T", again)
			}
			action, ok = nextAgain.graphStack.RuntimeTabs.ActiveRuntimeAction()
			if !ok || action.ExecutionStatus != "unsupported" || action.ExecutionError != "runtime action is not executable" {
				t.Fatalf("expected unsupported workspace safety execution metadata after second x, ok=%v action=%#v", ok, action)
			}
		})
	}
}

func TestModelUpdateDoesNotExecutePolicySchedulerRunArtifact(t *testing.T) {
	tabs := DefaultRuntimeTabs().
		Select(RuntimeTabAgentRuntime).
		WithRows(RuntimeTabAgentRuntime, []string{
			"agents: 0",
			"worker lifecycle policy scheduler run artifact: /tmp/ouro/runtime/worker-lifecycle-policy-scheduler-runs/scheduler-001.json status=completed mode=metadata-only policy=/tmp/ouro/runtime/worker-lifecycle-policies/policy-b.json selection=/tmp/ouro/runtime/worker-lifecycle-policy-selection.json policy_basename=policy-b.json selection_basename=worker-lifecycle-policy-selection.json max_runs=3 poll_interval_ms=250 transport=metadata-only include_lifecycle_maintenance=true execute_lifecycle_retries=false policy_runs_created=1 supervisor_created=1 skip=none error=none",
		}).
		SelectRow(1)
	called := false
	model := NewModel().
		WithGraphStack(GraphStack{RuntimeTabs: tabs}).
		WithRuntimeActionExecutor(func(action RuntimeAction) (RuntimeActionExecutionResult, error) {
			called = true
			return RuntimeActionExecutionResult{Status: "unexpected", Path: "/tmp/ouro/executed.json"}, nil
		})

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if cmd != nil {
		t.Fatalf("policy scheduler run artifact must not arm or return an execution command")
	}
	if called {
		t.Fatalf("policy scheduler run artifact must not call the runtime action executor")
	}
	next, ok := updated.(Model)
	if !ok {
		t.Fatalf("expected Model, got %T", updated)
	}
	action, ok := next.graphStack.RuntimeTabs.ActiveRuntimeAction()
	if !ok {
		t.Fatalf("expected active policy scheduler action with unsupported execution metadata")
	}
	if action.Kind != "worker-lifecycle-policy-scheduler-run-artifact" || action.ExecutionStatus != "unsupported" || action.ExecutionError != "runtime action is not executable" {
		t.Fatalf("expected unsupported policy scheduler execution metadata, got %#v", action)
	}
	if !runtimeActionIsReadOnly(action) {
		t.Fatalf("expected policy scheduler run artifact to be classified read-only")
	}

	updated, cmd = next.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if cmd != nil {
		t.Fatalf("second x on policy scheduler run artifact must not return an execution command")
	}
	if called {
		t.Fatalf("second x on policy scheduler run artifact must not call the runtime action executor")
	}
}

func TestModelUpdateDoesNotExecuteWorkerDispatchRetrySchedulerRunArtifact(t *testing.T) {
	tabs := DefaultRuntimeTabs().
		Select(RuntimeTabAgentRuntime).
		WithRows(RuntimeTabAgentRuntime, []string{
			"agents: 0",
			"worker dispatch retry scheduler run artifact: /tmp/ouro/runtime/worker-dispatch-retry-scheduler-runs/scheduler-001.json status=completed mode=metadata-only max_runs=2 dispatch_retry_claims=true retry_transport=file-outbox retry_plans_created=2 retry_dispatches_created=2 retry_dispatch_executions_created=2 retry_dispatch_claims_created=3 retry_dispatch_claims_dispatched=2 retry_dispatch_transport_requests_created=2 skip=none error=none",
		}).
		SelectRow(1)
	called := false
	model := NewModel().
		WithGraphStack(GraphStack{RuntimeTabs: tabs}).
		WithRuntimeActionExecutor(func(action RuntimeAction) (RuntimeActionExecutionResult, error) {
			called = true
			return RuntimeActionExecutionResult{Status: "unexpected", Path: "/tmp/ouro/executed.json"}, nil
		})

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if cmd != nil {
		t.Fatalf("dispatch retry scheduler run artifact must not arm or return an execution command")
	}
	if called {
		t.Fatalf("dispatch retry scheduler run artifact must not call the runtime action executor")
	}
	next, ok := updated.(Model)
	if !ok {
		t.Fatalf("expected Model, got %T", updated)
	}
	action, ok := next.graphStack.RuntimeTabs.ActiveRuntimeAction()
	if !ok {
		t.Fatalf("expected active dispatch retry scheduler action with unsupported execution metadata")
	}
	if action.Kind != "worker-dispatch-retry-scheduler-run-artifact" || action.ExecutionStatus != "unsupported" || action.ExecutionError != "runtime action is not executable" {
		t.Fatalf("expected unsupported dispatch retry scheduler execution metadata, got %#v", action)
	}
	if !runtimeActionIsReadOnly(action) {
		t.Fatalf("expected dispatch retry scheduler run artifact to be classified read-only")
	}

	updated, cmd = next.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if cmd != nil {
		t.Fatalf("second x on dispatch retry scheduler run artifact must not return an execution command")
	}
	if called {
		t.Fatalf("second x on dispatch retry scheduler run artifact must not call the runtime action executor")
	}
}

func TestRuntimeTabsReportProjectionDispatchExecutionArtifactDetailsAreReadOnly(t *testing.T) {
	tabs := DefaultRuntimeTabs().
		Select(RuntimeTabTimelineLanes).
		WithRows(RuntimeTabTimelineLanes, []string{
			"timeline: 0",
			"report projection dispatch execution artifact: /tmp/ouro/reports/report-projection-dispatch-executions/execution-001.json status=completed mode=execute dispatch=/tmp/ouro/reports/report-projection-dispatches/dispatch-001.json run=run-123 events=45 report=/tmp/ouro/reports/run-123.md written=true actor=bob executed_at=2026-06-02T05:31:00Z reason=approved_projection",
		}).
		SelectRow(1)
	model := NewModel().WithGraphStack(GraphStack{RuntimeTabs: tabs})

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	if cmd != nil {
		t.Fatalf("expected report dispatch execution detail update to be synchronous")
	}
	detailed, ok := updated.(Model)
	if !ok {
		t.Fatalf("expected Model, got %T", updated)
	}
	action := detailed.graphStack.RuntimeAction
	if action.Kind != "report-projection-dispatch-execution-artifact" || action.ArtifactPath != "/tmp/ouro/reports/report-projection-dispatch-executions/execution-001.json" {
		t.Fatalf("expected report projection dispatch execution artifact runtime action, got %#v", action)
	}
	if action.State != "completed" || action.ExecutionMode != "execute" || action.RunID != "run-123" {
		t.Fatalf("unexpected report projection dispatch execution action metadata: %#v", action)
	}
	if action.DispatchPath != "/tmp/ouro/reports/report-projection-dispatches/dispatch-001.json" || action.ResultPath != "/tmp/ouro/reports/run-123.md" {
		t.Fatalf("expected dispatch and report paths, got %#v", action)
	}
	view := strings.Join(action.RenderRows(), "\n")
	for _, want := range []string{
		"action: report projection dispatch execution artifact selected /tmp/ouro/reports/report-projection-dispatch-executions/execution-001.json",
		"artifact path: /tmp/ouro/reports/report-projection-dispatch-executions/execution-001.json",
		"dispatch path: /tmp/ouro/reports/report-projection-dispatches/dispatch-001.json",
		"run: run-123",
		"result path: /tmp/ouro/reports/run-123.md",
		"created by: bob",
		"reason: approved_projection",
		"results: events=45 written=true executed_at=2026-06-02T05:31:00Z",
		"action mode: read-only",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("report dispatch execution runtime tabs missing %q:\n%s", want, view)
		}
	}

	inspector := detailed.graphStack.Inspector().Rows
	for _, want := range []string{
		"runtime action: report-projection-dispatch-execution-artifact",
		"artifact path: /tmp/ouro/reports/report-projection-dispatch-executions/execution-001.json",
		"state: completed",
		"execution mode: execute",
		"dispatch path: /tmp/ouro/reports/report-projection-dispatches/dispatch-001.json",
		"run: run-123",
		"result path: /tmp/ouro/reports/run-123.md",
		"created by: bob",
		"reason: approved_projection",
		"results: events=45 written=true executed_at=2026-06-02T05:31:00Z",
		"mode: read-only",
	} {
		if !containsRow(inspector, want) {
			t.Fatalf("report dispatch execution inspector missing %q, got %#v", want, inspector)
		}
	}
}

func TestRuntimeTabsReportProjectionExecutionReviewDetailsAreReadOnly(t *testing.T) {
	tabs := DefaultRuntimeTabs().
		Select(RuntimeTabTimelineLanes).
		WithRows(RuntimeTabTimelineLanes, []string{
			"timeline: 0",
			"report projection execution review: mode=metadata-only decision=accepted reviewer=alice reviewed_at=2026-06-02T05:45:00Z review_reason=authorized_report_publication execution=/tmp/ouro/reports/report-projection-dispatch-executions/execution-001.json dispatch=/tmp/ouro/reports/report-projection-dispatches/dispatch-001.json run=run-123 report=/tmp/ouro/reports/run-123.md review=/tmp/ouro/reports/report-projection-dispatch-executions/execution-001.json",
		}).
		SelectRow(1)
	model := NewModel().WithGraphStack(GraphStack{RuntimeTabs: tabs})

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	if cmd != nil {
		t.Fatalf("expected report projection execution review detail update to be synchronous")
	}
	detailed, ok := updated.(Model)
	if !ok {
		t.Fatalf("expected Model, got %T", updated)
	}
	action := detailed.graphStack.RuntimeAction
	if action.Kind != "report-projection-execution-review" || action.ReviewPath != "/tmp/ouro/reports/report-projection-dispatch-executions/execution-001.json" {
		t.Fatalf("expected report projection execution review runtime action, got %#v", action)
	}
	if action.Decision != "accepted" || action.Reviewer != "alice" || action.ReviewedAt != "2026-06-02T05:45:00Z" || action.ReviewReason != "authorized_report_publication" {
		t.Fatalf("unexpected report projection execution review metadata: %#v", action)
	}
	if action.ExecutionPath != "/tmp/ouro/reports/report-projection-dispatch-executions/execution-001.json" || action.DispatchPath != "/tmp/ouro/reports/report-projection-dispatches/dispatch-001.json" || action.ResultPath != "/tmp/ouro/reports/run-123.md" {
		t.Fatalf("unexpected report projection execution review references: %#v", action)
	}
	view := strings.Join(action.RenderRows(), "\n")
	for _, want := range []string{
		"action: report projection execution review selected /tmp/ouro/reports/report-projection-dispatch-executions/execution-001.json",
		"review path: /tmp/ouro/reports/report-projection-dispatch-executions/execution-001.json",
		"execution path: /tmp/ouro/reports/report-projection-dispatch-executions/execution-001.json",
		"dispatch path: /tmp/ouro/reports/report-projection-dispatches/dispatch-001.json",
		"run: run-123",
		"result path: /tmp/ouro/reports/run-123.md",
		"decision: accepted",
		"reviewer: alice",
		"reviewed at: 2026-06-02T05:45:00Z",
		"review reason: authorized_report_publication",
		"action mode: read-only",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("report projection execution review runtime rows missing %q:\n%s", want, view)
		}
	}

	inspector := detailed.graphStack.Inspector().Rows
	for _, want := range []string{
		"runtime action: report-projection-execution-review",
		"review path: /tmp/ouro/reports/report-projection-dispatch-executions/execution-001.json",
		"execution path: /tmp/ouro/reports/report-projection-dispatch-executions/execution-001.json",
		"dispatch path: /tmp/ouro/reports/report-projection-dispatches/dispatch-001.json",
		"run: run-123",
		"result path: /tmp/ouro/reports/run-123.md",
		"decision: accepted",
		"reviewer: alice",
		"reviewed at: 2026-06-02T05:45:00Z",
		"review reason: authorized_report_publication",
		"mode: read-only",
	} {
		if !containsRow(inspector, want) {
			t.Fatalf("report projection execution review inspector missing %q, got %#v", want, inspector)
		}
	}
}

func TestRuntimeTabsReportProjectionExecutionSelectionDetailsExposeCloseableMarker(t *testing.T) {
	tabs := DefaultRuntimeTabs().
		Select(RuntimeTabTimelineLanes).
		WithRows(RuntimeTabTimelineLanes, []string{
			"timeline: 0",
			"report projection execution selection: status=current mode=metadata-only execution=/tmp/ouro/reports/report-projection-dispatch-executions/execution-001.json selection=/tmp/ouro/reports/report-projection-selection.json dispatch=/tmp/ouro/reports/report-projection-dispatches/dispatch-001.json run=run-123 report=/tmp/ouro/reports/run-123.md written=true events=45 superseded=1 selected_by=alice selected_at=2026-06-02T05:50:00Z reason=current_published_report",
		}).
		SelectRow(1)
	model := NewModel().WithGraphStack(GraphStack{RuntimeTabs: tabs})

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	if cmd != nil {
		t.Fatalf("expected report projection execution selection detail update to be synchronous")
	}
	detailed, ok := updated.(Model)
	if !ok {
		t.Fatalf("expected Model, got %T", updated)
	}
	action := detailed.graphStack.RuntimeAction
	if action.Kind != "report-projection-execution-selection" || action.State != "current" {
		t.Fatalf("expected current report projection execution selection runtime action, got %#v", action)
	}
	if action.ArtifactPath != "/tmp/ouro/reports/report-projection-dispatch-executions/execution-001.json" || action.RequestPath != "/tmp/ouro/reports/report-projection-selection.json" {
		t.Fatalf("unexpected report projection execution selection paths: %#v", action)
	}
	view := strings.Join(action.RenderRows(), "\n")
	for _, want := range []string{
		"action: report projection execution selection selected /tmp/ouro/reports/report-projection-dispatch-executions/execution-001.json",
		"selection status: current",
		"artifact path: /tmp/ouro/reports/report-projection-dispatch-executions/execution-001.json",
		"selection path: /tmp/ouro/reports/report-projection-selection.json",
		"dispatch path: /tmp/ouro/reports/report-projection-dispatches/dispatch-001.json",
		"run: run-123",
		"result path: /tmp/ouro/reports/run-123.md",
		"selected by: alice",
		"selected at: 2026-06-02T05:50:00Z",
		"reason: current_published_report",
		"superseded: 1",
		"action mode: metadata-only",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("report projection execution selection runtime rows missing %q:\n%s", want, view)
		}
	}

	inspector := detailed.graphStack.Inspector().Rows
	for _, want := range []string{
		"runtime action: report-projection-execution-selection",
		"state: current",
		"artifact path: /tmp/ouro/reports/report-projection-dispatch-executions/execution-001.json",
		"selection path: /tmp/ouro/reports/report-projection-selection.json",
		"dispatch path: /tmp/ouro/reports/report-projection-dispatches/dispatch-001.json",
		"run: run-123",
		"result path: /tmp/ouro/reports/run-123.md",
		"selected by: alice",
		"selected at: 2026-06-02T05:50:00Z",
		"reason: current_published_report",
		"superseded: 1",
		"mode: metadata-only",
	} {
		if !containsRow(inspector, want) {
			t.Fatalf("report projection execution selection inspector missing %q, got %#v", want, inspector)
		}
	}
}

func TestRuntimeReportProjectionDispatchArtifactUsesEncodedReasonText(t *testing.T) {
	reason := "operator review planned_report=/tmp/ouro/reports/evil.md report=/tmp/ouro/reports/evil-result.md run=run-evil"
	row := "report projection dispatch artifact: /tmp/ouro/reports/report-projection-dispatches/dispatch-001.json status=dry-run mode=metadata-only reason=" + reason + " reason_b64=" + base64.RawURLEncoding.EncodeToString([]byte(reason)) + " run=run-123 events=42 planned_report=/tmp/ouro/reports/run-123.md actor=alice created_at=2026-06-02T04:30:00Z"

	action := runtimeReportProjectionDispatchArtifactActionForRow(row)
	if action.RunID != "run-123" || action.PlannedReportPath != "/tmp/ouro/reports/run-123.md" {
		t.Fatalf("encoded report dispatch reason must not retarget machine fields: %#v", action)
	}
	if action.Reason != reason {
		t.Fatalf("expected encoded report dispatch reason to be decoded, got %q", action.Reason)
	}
}

func TestRuntimeReportProjectionDispatchExecutionArtifactUsesEncodedReviewText(t *testing.T) {
	reason := "approved projection decision=rejected current=false reviewed_at=2030-01-01T00:00:00Z"
	reviewReason := "accepted after review current=false report=/tmp/ouro/reports/evil.md"
	row := "report projection dispatch execution artifact: /tmp/ouro/reports/report-projection-dispatch-executions/execution-001.json status=completed mode=execute dispatch=/tmp/ouro/reports/report-projection-dispatches/dispatch-001.json run=run-123 events=45 report=/tmp/ouro/reports/run-123.md written=true actor=bob executed_at=2026-06-02T05:31:00Z reason=" + reason + " reason_b64=" + base64.RawURLEncoding.EncodeToString([]byte(reason)) + " decision=accepted reviewer=alice reviewed_at=2026-06-02T05:45:00Z review_reason=" + reviewReason + " review_reason_b64=" + base64.RawURLEncoding.EncodeToString([]byte(reviewReason)) + " current=true"

	action := runtimeReportProjectionDispatchExecutionArtifactActionForRow(row)
	if action.Decision != "accepted" || action.Current != "true" || action.ReviewedAt != "2026-06-02T05:45:00Z" {
		t.Fatalf("encoded report dispatch execution text must not retarget review metadata: %#v", action)
	}
	if action.ResultPath != "/tmp/ouro/reports/run-123.md" {
		t.Fatalf("encoded report dispatch execution review text must not retarget report path: %#v", action)
	}
	if action.Reason != reason || action.ReviewReason != reviewReason {
		t.Fatalf("expected encoded report dispatch execution text to be decoded, got reason=%q review_reason=%q", action.Reason, action.ReviewReason)
	}
}

func TestRuntimeReportProjectionExecutionReviewUsesEncodedReasonText(t *testing.T) {
	reviewReason := "authorized report publication execution=/tmp/ouro/reports/report-projection-dispatch-executions/evil.json dispatch=/tmp/ouro/reports/report-projection-dispatches/evil.json report=/tmp/ouro/reports/evil.md"
	row := "report projection execution review: mode=metadata-only decision=accepted reviewer=alice reviewed_at=2026-06-02T05:45:00Z review_reason=" + reviewReason + " review_reason_b64=" + base64.RawURLEncoding.EncodeToString([]byte(reviewReason)) + " execution=/tmp/ouro/reports/report-projection-dispatch-executions/execution-001.json dispatch=/tmp/ouro/reports/report-projection-dispatches/dispatch-001.json run=run-123 report=/tmp/ouro/reports/run-123.md review=/tmp/ouro/reports/report-projection-dispatch-executions/execution-001.json"

	action := runtimeReportProjectionExecutionReviewActionForRow(row)
	if action.ExecutionPath != "/tmp/ouro/reports/report-projection-dispatch-executions/execution-001.json" || action.DispatchPath != "/tmp/ouro/reports/report-projection-dispatches/dispatch-001.json" || action.ResultPath != "/tmp/ouro/reports/run-123.md" {
		t.Fatalf("encoded report execution review reason must not retarget references: %#v", action)
	}
	if action.ReviewReason != reviewReason {
		t.Fatalf("expected encoded report execution review reason to be decoded, got %q", action.ReviewReason)
	}
}

func TestRuntimeReportProjectionExecutionSelectionUsesEncodedReasonText(t *testing.T) {
	reason := "current published report execution=/tmp/ouro/reports/report-projection-dispatch-executions/evil.json selection=/tmp/ouro/reports/evil-selection.json dispatch=/tmp/ouro/reports/report-projection-dispatches/evil.json report=/tmp/ouro/reports/evil.md"
	row := "report projection execution selection: status=current mode=metadata-only current=true selected_by=alice selected_at=2026-06-02T05:50:00Z reason=" + reason + " reason_b64=" + base64.RawURLEncoding.EncodeToString([]byte(reason)) + " execution=/tmp/ouro/reports/report-projection-dispatch-executions/execution-001.json selection=/tmp/ouro/reports/report-projection-selection.json dispatch=/tmp/ouro/reports/report-projection-dispatches/dispatch-001.json run=run-123 report=/tmp/ouro/reports/run-123.md written=true events=45 superseded=1"

	action := runtimeReportProjectionExecutionSelectionActionForRow(row)
	if action.ArtifactPath != "/tmp/ouro/reports/report-projection-dispatch-executions/execution-001.json" || action.RequestPath != "/tmp/ouro/reports/report-projection-selection.json" || action.DispatchPath != "/tmp/ouro/reports/report-projection-dispatches/dispatch-001.json" || action.ResultPath != "/tmp/ouro/reports/run-123.md" {
		t.Fatalf("encoded report execution selection reason must not retarget marker references: %#v", action)
	}
	if action.Reason != reason {
		t.Fatalf("expected encoded report execution selection reason to be decoded, got %q", action.Reason)
	}
}

func TestRuntimeReportProjectionDispatchArtifactUsesEncodedReferenceFields(t *testing.T) {
	artifactPath := "/tmp/ouro/reports/report projection dispatches/dispatch status=evil.json"
	plannedReportPath := "/tmp/ouro/reports/final planned_report=evil.md"
	actor := "operator actor_b64=fake planned_report=/tmp/ouro/evil.md"
	createdAt := "2026-06-02T04:30:00Z"
	reason := "operator review"
	row := "report projection dispatch artifact: " + artifactPath +
		" artifact_b64=" + base64.RawURLEncoding.EncodeToString([]byte(artifactPath)) +
		" status=dry-run mode=metadata-only reason=" + reason +
		" reason_b64=" + base64.RawURLEncoding.EncodeToString([]byte(reason)) +
		" run=run-123 events=42 planned_report=" + strings.Join(strings.Fields(plannedReportPath), "_") +
		" planned_report_b64=" + base64.RawURLEncoding.EncodeToString([]byte(plannedReportPath)) +
		" actor=" + strings.Join(strings.Fields(actor), "_") +
		" actor_b64=" + base64.RawURLEncoding.EncodeToString([]byte(actor)) +
		" created_at=" + createdAt +
		" created_at_b64=" + base64.RawURLEncoding.EncodeToString([]byte(createdAt))

	action := runtimeReportProjectionDispatchArtifactActionForRow(row)
	if action.ArtifactPath != artifactPath || action.PlannedReportPath != plannedReportPath || action.CreatedBy != actor || action.CreatedAt != createdAt {
		t.Fatalf("expected encoded dispatch references to decode, got %#v", action)
	}
	if action.State != "dry-run" || action.ExecutionMode != "metadata-only" || action.RunID != "run-123" || action.Results != "events=42" {
		t.Fatalf("encoded dispatch references must not retarget machine metadata: %#v", action)
	}
}

func TestRuntimeReportProjectionDispatchExecutionArtifactUsesEncodedReferenceFields(t *testing.T) {
	artifactPath := "/tmp/ouro/reports/report projection executions/execution status=evil.json"
	dispatchPath := "/tmp/ouro/reports/report projection dispatches/dispatch report=evil.json"
	reportPath := "/tmp/ouro/reports/final report=evil.md"
	actor := "operator actor_b64=fake report=/tmp/ouro/evil.md"
	executedAt := "2026-06-02T05:31:00Z"
	reviewer := "reviewer reviewed_at=2030-01-01T00:00:00Z"
	reviewedAt := "2026-06-02T05:45:00Z"
	reason := "approved projection"
	reviewReason := "accepted after review"
	row := "report projection dispatch execution artifact: " + artifactPath +
		" artifact_b64=" + base64.RawURLEncoding.EncodeToString([]byte(artifactPath)) +
		" status=completed mode=execute dispatch=" + strings.Join(strings.Fields(dispatchPath), "_") +
		" dispatch_b64=" + base64.RawURLEncoding.EncodeToString([]byte(dispatchPath)) +
		" run=run-123 events=45 report=" + strings.Join(strings.Fields(reportPath), "_") +
		" report_b64=" + base64.RawURLEncoding.EncodeToString([]byte(reportPath)) +
		" written=true actor=" + strings.Join(strings.Fields(actor), "_") +
		" actor_b64=" + base64.RawURLEncoding.EncodeToString([]byte(actor)) +
		" executed_at=" + executedAt +
		" executed_at_b64=" + base64.RawURLEncoding.EncodeToString([]byte(executedAt)) +
		" reason=" + reason +
		" reason_b64=" + base64.RawURLEncoding.EncodeToString([]byte(reason)) +
		" decision=accepted reviewer=" + strings.Join(strings.Fields(reviewer), "_") +
		" reviewer_b64=" + base64.RawURLEncoding.EncodeToString([]byte(reviewer)) +
		" reviewed_at=" + reviewedAt +
		" reviewed_at_b64=" + base64.RawURLEncoding.EncodeToString([]byte(reviewedAt)) +
		" review_reason=" + reviewReason +
		" review_reason_b64=" + base64.RawURLEncoding.EncodeToString([]byte(reviewReason)) +
		" current=true"

	action := runtimeReportProjectionDispatchExecutionArtifactActionForRow(row)
	if action.ArtifactPath != artifactPath || action.DispatchPath != dispatchPath || action.ResultPath != reportPath {
		t.Fatalf("expected encoded execution references to decode, got %#v", action)
	}
	if action.CreatedBy != actor || action.CreatedAt != executedAt || action.Reviewer != reviewer || action.ReviewedAt != reviewedAt {
		t.Fatalf("expected encoded execution identities to decode, got %#v", action)
	}
	if action.State != "completed" || action.ExecutionMode != "execute" || action.Current != "true" || action.Decision != "accepted" {
		t.Fatalf("encoded execution references must not retarget machine metadata: %#v", action)
	}
}

func TestRuntimeReportProjectionExecutionReviewUsesEncodedReferenceFields(t *testing.T) {
	executionPath := "/tmp/ouro/reports/report projection executions/execution dispatch=evil.json"
	dispatchPath := "/tmp/ouro/reports/report projection dispatches/dispatch report=evil.json"
	reportPath := "/tmp/ouro/reports/final report=evil.md"
	reviewer := "reviewer reviewed_at=2030-01-01T00:00:00Z"
	reviewedAt := "2026-06-02T05:45:00Z"
	reviewReason := "authorized report publication"
	row := "report projection execution review: mode=metadata-only decision=accepted reviewer=" + strings.Join(strings.Fields(reviewer), "_") +
		" reviewer_b64=" + base64.RawURLEncoding.EncodeToString([]byte(reviewer)) +
		" reviewed_at=" + reviewedAt +
		" reviewed_at_b64=" + base64.RawURLEncoding.EncodeToString([]byte(reviewedAt)) +
		" review_reason=" + reviewReason +
		" review_reason_b64=" + base64.RawURLEncoding.EncodeToString([]byte(reviewReason)) +
		" execution=" + strings.Join(strings.Fields(executionPath), "_") +
		" execution_b64=" + base64.RawURLEncoding.EncodeToString([]byte(executionPath)) +
		" dispatch=" + strings.Join(strings.Fields(dispatchPath), "_") +
		" dispatch_b64=" + base64.RawURLEncoding.EncodeToString([]byte(dispatchPath)) +
		" run=run-123 report=" + strings.Join(strings.Fields(reportPath), "_") +
		" report_b64=" + base64.RawURLEncoding.EncodeToString([]byte(reportPath)) +
		" review=" + strings.Join(strings.Fields(executionPath), "_") +
		" review_b64=" + base64.RawURLEncoding.EncodeToString([]byte(executionPath))

	action := runtimeReportProjectionExecutionReviewActionForRow(row)
	if action.ExecutionPath != executionPath || action.DispatchPath != dispatchPath || action.ResultPath != reportPath || action.ReviewPath != executionPath {
		t.Fatalf("expected encoded report execution review references to decode, got %#v", action)
	}
	if action.Reviewer != reviewer || action.ReviewedAt != reviewedAt || action.Decision != "accepted" {
		t.Fatalf("expected encoded report execution review identity to decode without retargeting metadata: %#v", action)
	}
}

func TestRuntimeReportProjectionExecutionSelectionUsesEncodedReferenceFields(t *testing.T) {
	executionPath := "/tmp/ouro/reports/report projection executions/execution selection=evil.json"
	selectionPath := "/tmp/ouro/reports/report projection selection.json"
	dispatchPath := "/tmp/ouro/reports/report projection dispatches/dispatch report=evil.json"
	reportPath := "/tmp/ouro/reports/final report=evil.md"
	selectedBy := "operator selected_by_b64=fake execution=/tmp/ouro/evil.json"
	selectedAt := "2026-06-02T07:10:00Z"
	reason := "current reviewed report"
	row := "report projection execution selection: status=current mode=metadata-only current=true selected_by=" + strings.Join(strings.Fields(selectedBy), "_") +
		" selected_by_b64=" + base64.RawURLEncoding.EncodeToString([]byte(selectedBy)) +
		" selected_at=" + selectedAt +
		" selected_at_b64=" + base64.RawURLEncoding.EncodeToString([]byte(selectedAt)) +
		" reason=" + reason +
		" reason_b64=" + base64.RawURLEncoding.EncodeToString([]byte(reason)) +
		" execution=" + strings.Join(strings.Fields(executionPath), "_") +
		" execution_b64=" + base64.RawURLEncoding.EncodeToString([]byte(executionPath)) +
		" selection=" + strings.Join(strings.Fields(selectionPath), "_") +
		" selection_b64=" + base64.RawURLEncoding.EncodeToString([]byte(selectionPath)) +
		" dispatch=" + strings.Join(strings.Fields(dispatchPath), "_") +
		" dispatch_b64=" + base64.RawURLEncoding.EncodeToString([]byte(dispatchPath)) +
		" run=run-123 report=" + strings.Join(strings.Fields(reportPath), "_") +
		" report_b64=" + base64.RawURLEncoding.EncodeToString([]byte(reportPath)) +
		" written=true events=45 superseded=1"

	action := runtimeReportProjectionExecutionSelectionActionForRow(row)
	if action.ArtifactPath != executionPath || action.RequestPath != selectionPath || action.DispatchPath != dispatchPath || action.ResultPath != reportPath {
		t.Fatalf("expected encoded report execution selection references to decode, got %#v", action)
	}
	if action.SelectedBy != selectedBy || action.SelectedAt != selectedAt || action.State != "current" || action.Current != "true" {
		t.Fatalf("expected encoded report execution selection identity without retargeting metadata: %#v", action)
	}
}

func TestRuntimeReportProjectionExecutionSelectionUsesEncodedClearedReferenceFields(t *testing.T) {
	executionPath := "/tmp/ouro/reports/report projection executions/execution cleared=evil.json"
	selectionPath := "/tmp/ouro/reports/report projection selection.json"
	dispatchPath := "/tmp/ouro/reports/report projection dispatches/dispatch report=evil.json"
	reportPath := "/tmp/ouro/reports/final report=evil.md"
	clearedBy := "operator cleared_by_b64=fake execution=/tmp/ouro/evil.json"
	clearedAt := "2026-06-02T07:25:00Z"
	reason := "authorization changed"
	row := "report projection execution selection: status=cleared mode=metadata-only current=false cleared_by=" + strings.Join(strings.Fields(clearedBy), "_") +
		" cleared_by_b64=" + base64.RawURLEncoding.EncodeToString([]byte(clearedBy)) +
		" cleared_at=" + clearedAt +
		" cleared_at_b64=" + base64.RawURLEncoding.EncodeToString([]byte(clearedAt)) +
		" reason=" + reason +
		" reason_b64=" + base64.RawURLEncoding.EncodeToString([]byte(reason)) +
		" execution=" + strings.Join(strings.Fields(executionPath), "_") +
		" execution_b64=" + base64.RawURLEncoding.EncodeToString([]byte(executionPath)) +
		" selection=" + strings.Join(strings.Fields(selectionPath), "_") +
		" selection_b64=" + base64.RawURLEncoding.EncodeToString([]byte(selectionPath)) +
		" dispatch=" + strings.Join(strings.Fields(dispatchPath), "_") +
		" dispatch_b64=" + base64.RawURLEncoding.EncodeToString([]byte(dispatchPath)) +
		" run=run-123 report=" + strings.Join(strings.Fields(reportPath), "_") +
		" report_b64=" + base64.RawURLEncoding.EncodeToString([]byte(reportPath)) +
		" written=true events=45 superseded=1"

	action := runtimeReportProjectionExecutionSelectionActionForRow(row)
	if action.ArtifactPath != executionPath || action.RequestPath != selectionPath || action.DispatchPath != dispatchPath || action.ResultPath != reportPath {
		t.Fatalf("expected encoded cleared report execution selection references to decode, got %#v", action)
	}
	if action.ClearedBy != clearedBy || action.ClearedAt != clearedAt || action.State != "cleared" || action.Current != "false" {
		t.Fatalf("expected encoded cleared report execution selection identity without retargeting metadata: %#v", action)
	}
}

func TestModelUpdateExecutesReportProjectionDispatchArtifactOnlyAfterSecondConfirm(t *testing.T) {
	tabs := DefaultRuntimeTabs().
		Select(RuntimeTabTimelineLanes).
		WithRows(RuntimeTabTimelineLanes, []string{
			"timeline: ready",
			"report projection dispatch artifact: /tmp/ouro/reports/report-projection-dispatches/dispatch-001.json status=dry-run mode=metadata-only run=run-123 events=42 planned_report=/tmp/ouro/reports/run-123.md actor=alice reason=operator_review",
		}).
		SelectRow(1)
	called := false
	model := NewModel().
		WithGraphStack(GraphStack{RuntimeTabs: tabs}).
		WithRuntimeActionExecutor(func(action RuntimeAction) (RuntimeActionExecutionResult, error) {
			called = true
			if action.Kind != "report-projection-dispatch-artifact" || action.ArtifactPath != "/tmp/ouro/reports/report-projection-dispatches/dispatch-001.json" {
				t.Fatalf("unexpected executed action: %#v", action)
			}
			return RuntimeActionExecutionResult{
				Status: "completed",
				Path:   "/tmp/ouro/reports/report-projection-dispatch-executions/execution-001.json",
				Output: "report projection dispatch executed",
				RuntimeRows: []string{
					"report projection dispatch executions: total=1",
					"report projection dispatch execution artifact: /tmp/ouro/reports/report-projection-dispatch-executions/execution-001.json status=completed mode=execute dispatch=/tmp/ouro/reports/report-projection-dispatches/dispatch-001.json run=run-123 events=45 report=/tmp/ouro/reports/run-123.md written=true actor=tui executed_at=2026-06-02T05:31:00Z reason=executed_from_TUI current=false",
					"report projection dispatch execution compact: status=completed mode=execute run=run-123 events=45 written=true actor=tui reason=executed_from_TUI current=false",
					"report projection dispatch execution path: execution-001.json",
				},
			}, nil
		})

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if cmd != nil {
		t.Fatalf("first x should only arm report projection dispatch execution confirmation")
	}
	if called {
		t.Fatalf("executor must not run on first x")
	}
	next, ok := updated.(Model)
	if !ok {
		t.Fatalf("expected Model, got %T", updated)
	}
	action, ok := next.graphStack.RuntimeTabs.ActiveRuntimeAction()
	if !ok || action.Kind != "report-projection-dispatch-artifact" || action.ExecutionStatus != "confirm-required" {
		t.Fatalf("expected confirm-required report dispatch action, ok=%v action=%#v", ok, action)
	}
	if runtimeActionIsReadOnly(action) {
		t.Fatalf("expected report projection dispatch artifact to be executable")
	}
	view := next.graphStack.RuntimeTabs.Render(150, 14)
	for _, want := range []string{
		"action: report projection dispatch artifact selected /tmp/ouro/reports/report-projection-dispatches/dispatch-001.json",
		"execution status: confirm-required",
		"execution confirm: press x again",
		"action mode: metadata-only",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("confirmation view missing %q:\n%s", want, view)
		}
	}

	updated, cmd = next.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if cmd == nil {
		t.Fatalf("second x should return report projection dispatch execution command")
	}
	if called {
		t.Fatalf("executor should run only when returned command is executed")
	}
	msg := cmd()
	if !called {
		t.Fatalf("expected executor to run when command executes")
	}
	updated, cmd = updated.Update(msg)
	if cmd != nil {
		t.Fatalf("expected report projection dispatch execution result update to be synchronous")
	}
	next, ok = updated.(Model)
	if !ok {
		t.Fatalf("expected Model, got %T", updated)
	}
	action, ok = next.graphStack.RuntimeTabs.ActiveRuntimeAction()
	if !ok || action.ExecutionStatus != "completed" || action.ExecutionPath != "/tmp/ouro/reports/report-projection-dispatch-executions/execution-001.json" || action.Output != "report projection dispatch executed" {
		t.Fatalf("expected completed report projection dispatch execution metadata, ok=%v action=%#v", ok, action)
	}
	selected, ok := next.graphStack.RuntimeTabs.ActiveSelectedRow()
	if !ok || !strings.HasPrefix(strings.TrimSpace(selected), "report projection dispatch execution artifact: /tmp/ouro/reports/report-projection-dispatch-executions/execution-001.json") {
		t.Fatalf("expected newly written report projection dispatch execution artifact to be selected, ok=%v row=%q", ok, selected)
	}
	rows := strings.Join(next.graphStack.RuntimeTabs.ActiveRows(), "\n")
	for _, want := range []string{
		"report projection dispatch executions: total=1",
		"report projection dispatch execution artifact: /tmp/ouro/reports/report-projection-dispatch-executions/execution-001.json",
		"report projection dispatch execution compact: status=completed",
		"report projection dispatch execution path: execution-001.json",
	} {
		if !strings.Contains(rows, want) {
			t.Fatalf("report projection dispatch execution rows missing %q:\n%s", want, rows)
		}
	}
}

func TestModelUpdateDoesNotExecuteReportProjectionDispatchExecutionArtifact(t *testing.T) {
	tabs := DefaultRuntimeTabs().
		Select(RuntimeTabTimelineLanes).
		WithRows(RuntimeTabTimelineLanes, []string{
			"timeline: 0",
			"report projection dispatch execution artifact: /tmp/ouro/reports/report-projection-dispatch-executions/execution-001.json status=completed mode=execute dispatch=/tmp/ouro/reports/report-projection-dispatches/dispatch-001.json run=run-123 events=45 report=/tmp/ouro/reports/run-123.md written=true actor=bob executed_at=2026-06-02T05:31:00Z reason=approved_projection",
		}).
		SelectRow(1)
	called := false
	model := NewModel().
		WithGraphStack(GraphStack{RuntimeTabs: tabs}).
		WithRuntimeActionExecutor(func(action RuntimeAction) (RuntimeActionExecutionResult, error) {
			called = true
			return RuntimeActionExecutionResult{Status: "unexpected", Path: "/tmp/ouro/executed.json"}, nil
		})

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if cmd != nil {
		t.Fatalf("report projection dispatch execution artifact must not arm or return an execution command")
	}
	if called {
		t.Fatalf("report projection dispatch execution artifact must not call the runtime action executor")
	}
	next, ok := updated.(Model)
	if !ok {
		t.Fatalf("expected Model, got %T", updated)
	}
	action, ok := next.graphStack.RuntimeTabs.ActiveRuntimeAction()
	if !ok {
		t.Fatalf("expected active report dispatch execution action with unsupported execution metadata")
	}
	if action.Kind != "report-projection-dispatch-execution-artifact" || action.ExecutionStatus != "unsupported" || action.ExecutionError != "runtime action is not executable" {
		t.Fatalf("expected unsupported report dispatch execution metadata, got %#v", action)
	}
	if !runtimeActionIsReadOnly(action) {
		t.Fatalf("expected report projection dispatch execution artifact to be classified read-only")
	}

	updated, cmd = next.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if cmd != nil {
		t.Fatalf("second x on report projection dispatch execution artifact must not return an execution command")
	}
	if called {
		t.Fatalf("second x on report projection dispatch execution artifact must not call the runtime action executor")
	}
}

func TestModelUpdateSelectsReportProjectionExecutionArtifactOnlyAfterSecondConfirm(t *testing.T) {
	tabs := DefaultRuntimeTabs().
		Select(RuntimeTabTimelineLanes).
		WithRows(RuntimeTabTimelineLanes, []string{
			"timeline: 0",
			"report projection execution selection: status=current mode=metadata-only execution=/tmp/ouro/reports/report-projection-dispatch-executions/base.json selection=/tmp/ouro/reports/report-projection-selection.json dispatch=/tmp/ouro/reports/report-projection-dispatches/base-dispatch.json run=run-123 report=/tmp/ouro/reports/run-123.md written=true events=45 superseded=1 selected_by=alice selected_at=2026-06-02T05:40:00Z reason=previous_report",
			"report projection dispatch execution artifact: /tmp/ouro/reports/report-projection-dispatch-executions/base.json status=completed mode=execute dispatch=/tmp/ouro/reports/report-projection-dispatches/base-dispatch.json run=run-123 events=45 report=/tmp/ouro/reports/run-123.md written=true current=true actor=alice executed_at=2026-06-02T05:30:00Z reason=previous_projection",
			"report projection dispatch execution artifact: /tmp/ouro/reports/report-projection-dispatch-executions/candidate.json status=completed mode=execute dispatch=/tmp/ouro/reports/report-projection-dispatches/candidate-dispatch.json run=run-123 events=45 report=/tmp/ouro/reports/run-123.md written=true current=false actor=bob executed_at=2026-06-02T05:31:00Z reason=approved_projection",
		}).
		SelectRow(3)
	called := false
	model := NewModel().
		WithGraphStack(GraphStack{RuntimeTabs: tabs}).
		WithRuntimeActionExecutor(func(action RuntimeAction) (RuntimeActionExecutionResult, error) {
			called = true
			if action.Kind != "report-projection-dispatch-execution-artifact" || action.ArtifactPath != "/tmp/ouro/reports/report-projection-dispatch-executions/candidate.json" {
				t.Fatalf("unexpected report projection execution selection action: %#v", action)
			}
			return RuntimeActionExecutionResult{
				Status: "completed",
				Path:   "/tmp/ouro/reports/report-projection-selection.json",
				Output: "report selected",
				RuntimeRows: []string{
					"report projection dispatch executions: total=2",
					"report projection execution selection: status=current mode=metadata-only execution=/tmp/ouro/reports/report-projection-dispatch-executions/candidate.json selection=/tmp/ouro/reports/report-projection-selection.json dispatch=/tmp/ouro/reports/report-projection-dispatches/candidate-dispatch.json run=run-123 report=/tmp/ouro/reports/run-123.md written=true events=45 superseded=1 selected_by=tui selected_at=2026-06-02T05:55:00Z reason=selected_from_TUI",
					"report projection execution superseded: /tmp/ouro/reports/report-projection-dispatch-executions/base.json",
					"report projection dispatch execution artifact: /tmp/ouro/reports/report-projection-dispatch-executions/base.json status=completed mode=execute dispatch=/tmp/ouro/reports/report-projection-dispatches/base-dispatch.json run=run-123 events=45 report=/tmp/ouro/reports/run-123.md written=true current=false actor=alice executed_at=2026-06-02T05:30:00Z reason=previous_projection",
					"report projection dispatch execution artifact: /tmp/ouro/reports/report-projection-dispatch-executions/candidate.json status=completed mode=execute dispatch=/tmp/ouro/reports/report-projection-dispatches/candidate-dispatch.json run=run-123 events=45 report=/tmp/ouro/reports/run-123.md written=true current=true actor=bob executed_at=2026-06-02T05:31:00Z reason=approved_projection",
				},
			}, nil
		})

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if cmd != nil {
		t.Fatalf("first x should only arm report projection execution selection confirmation")
	}
	if called {
		t.Fatalf("report projection execution selection executor must not run on first x")
	}
	next, ok := updated.(Model)
	if !ok {
		t.Fatalf("expected Model, got %T", updated)
	}
	action, ok := next.graphStack.RuntimeTabs.ActiveRuntimeAction()
	if !ok || action.ExecutionStatus != "confirm-required" {
		t.Fatalf("expected confirm-required report projection execution artifact action, ok=%v action=%#v", ok, action)
	}
	view := next.graphStack.RuntimeTabs.Render(180, 24)
	for _, want := range []string{
		"action: report projection dispatch execution artifact selected /tmp/ouro/reports/report-projection-dispatch-executions/candidate.json",
		"execution status: confirm-required",
		"execution confirm: press x again",
		"action mode: metadata-only",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("report projection execution selection confirmation view missing %q:\n%s", want, view)
		}
	}

	updated, cmd = next.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if cmd == nil {
		t.Fatalf("second x should return report projection execution selection command")
	}
	if called {
		t.Fatalf("report projection execution selection executor should run only when returned command is executed")
	}
	msg := cmd()
	if !called {
		t.Fatalf("expected report projection execution selection executor to run when command executes")
	}
	updated, cmd = updated.Update(msg)
	if cmd != nil {
		t.Fatalf("expected report projection execution selection result update to be synchronous")
	}
	next, ok = updated.(Model)
	if !ok {
		t.Fatalf("expected Model, got %T", updated)
	}
	action, ok = next.graphStack.RuntimeTabs.ActiveRuntimeAction()
	if !ok || action.ExecutionStatus != "completed" || action.ExecutionPath != "/tmp/ouro/reports/report-projection-selection.json" || action.Output != "report selected" {
		t.Fatalf("expected completed report projection execution selection metadata, ok=%v action=%#v", ok, action)
	}
	selected, ok := next.graphStack.RuntimeTabs.ActiveSelectedRow()
	if !ok || !strings.HasPrefix(strings.TrimSpace(selected), "report projection execution selection: status=current") || !strings.Contains(selected, "/tmp/ouro/reports/report-projection-dispatch-executions/candidate.json") {
		t.Fatalf("expected current report projection execution selection row to be selected, ok=%v row=%q", ok, selected)
	}
	rendered := next.graphStack.RuntimeTabs.Render(220, 24)
	if strings.Contains(rendered, "current=false actor=bob") {
		t.Fatalf("report projection execution selection refresh must not leave selected artifact current=false:\n%s", rendered)
	}
}

func TestModelUpdateClearsReportProjectionExecutionSelectionOnlyAfterSecondConfirm(t *testing.T) {
	tabs := DefaultRuntimeTabs().
		Select(RuntimeTabTimelineLanes).
		WithRows(RuntimeTabTimelineLanes, []string{
			"timeline: 0",
			"report projection execution selection: status=current mode=metadata-only execution=/tmp/ouro/reports/report-projection-dispatch-executions/current.json selection=/tmp/ouro/reports/report-projection-selection.json dispatch=/tmp/ouro/reports/report-projection-dispatches/dispatch-001.json run=run-123 report=/tmp/ouro/reports/run-123.md written=true events=45 superseded=1 selected_by=alice selected_at=2026-06-02T05:40:00Z reason=current_report",
			"report projection dispatch execution artifact: /tmp/ouro/reports/report-projection-dispatch-executions/current.json status=completed mode=execute dispatch=/tmp/ouro/reports/report-projection-dispatches/dispatch-001.json run=run-123 events=45 report=/tmp/ouro/reports/run-123.md written=true current=true actor=alice executed_at=2026-06-02T05:30:00Z reason=approved_projection",
		}).
		SelectRow(1)
	called := false
	model := NewModel().
		WithGraphStack(GraphStack{RuntimeTabs: tabs}).
		WithRuntimeActionExecutor(func(action RuntimeAction) (RuntimeActionExecutionResult, error) {
			called = true
			if action.Kind != "report-projection-execution-selection" || action.State != "current" || action.ArtifactPath != "/tmp/ouro/reports/report-projection-dispatch-executions/current.json" {
				t.Fatalf("unexpected report projection execution selection clear action: %#v", action)
			}
			return RuntimeActionExecutionResult{
				Status: "completed",
				Path:   "/tmp/ouro/reports/report-projection-selection.json",
				Output: "report selection cleared",
				RuntimeRows: []string{
					"report projection dispatch executions: total=1",
					"report projection execution selection: status=cleared mode=metadata-only execution=/tmp/ouro/reports/report-projection-dispatch-executions/current.json selection=/tmp/ouro/reports/report-projection-selection.json dispatch=/tmp/ouro/reports/report-projection-dispatches/dispatch-001.json run=run-123 report=/tmp/ouro/reports/run-123.md written=true events=45 superseded=1 cleared_by=tui cleared_at=2026-06-02T06:00:00Z reason=cleared_from_TUI",
					"report projection dispatch execution artifact: /tmp/ouro/reports/report-projection-dispatch-executions/current.json status=completed mode=execute dispatch=/tmp/ouro/reports/report-projection-dispatches/dispatch-001.json run=run-123 events=45 report=/tmp/ouro/reports/run-123.md written=true current=false actor=alice executed_at=2026-06-02T05:30:00Z reason=approved_projection",
				},
			}, nil
		})

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if cmd != nil {
		t.Fatalf("first x should only arm report projection execution selection clear confirmation")
	}
	if called {
		t.Fatalf("report projection execution selection clear executor must not run on first x")
	}
	next, ok := updated.(Model)
	if !ok {
		t.Fatalf("expected Model, got %T", updated)
	}
	action, ok := next.graphStack.RuntimeTabs.ActiveRuntimeAction()
	if !ok || action.ExecutionStatus != "confirm-required" {
		t.Fatalf("expected confirm-required report projection execution selection action, ok=%v action=%#v", ok, action)
	}
	view := next.graphStack.RuntimeTabs.Render(180, 24)
	for _, want := range []string{
		"action: report projection execution selection selected /tmp/ouro/reports/report-projection-dispatch-executions/current.json",
		"execution status: confirm-required",
		"execution confirm: press x again",
		"action mode: metadata-only",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("report projection execution selection clear confirmation view missing %q:\n%s", want, view)
		}
	}

	updated, cmd = next.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if cmd == nil {
		t.Fatalf("second x should return report projection execution selection clear command")
	}
	if called {
		t.Fatalf("report projection execution selection clear executor should run only when returned command is executed")
	}
	msg := cmd()
	if !called {
		t.Fatalf("expected report projection execution selection clear executor to run when command executes")
	}
	updated, cmd = updated.Update(msg)
	if cmd != nil {
		t.Fatalf("expected report projection execution selection clear result update to be synchronous")
	}
	next, ok = updated.(Model)
	if !ok {
		t.Fatalf("expected Model, got %T", updated)
	}
	action, ok = next.graphStack.RuntimeTabs.ActiveRuntimeAction()
	if !ok || action.ExecutionStatus != "completed" || action.ExecutionPath != "/tmp/ouro/reports/report-projection-selection.json" || action.Output != "report selection cleared" {
		t.Fatalf("expected completed report projection execution selection clear metadata, ok=%v action=%#v", ok, action)
	}
	selected, ok := next.graphStack.RuntimeTabs.ActiveSelectedRow()
	if !ok || !strings.HasPrefix(strings.TrimSpace(selected), "report projection execution selection: status=cleared") {
		t.Fatalf("expected cleared report projection execution selection row to be selected, ok=%v row=%q", ok, selected)
	}
	rendered := next.graphStack.RuntimeTabs.Render(220, 24)
	if strings.Contains(rendered, "current=true") {
		t.Fatalf("cleared report projection execution selection refresh must not leave current=true rows:\n%s", rendered)
	}
}

func TestModelUpdateRequestsRuntimeActionMetadataOnly(t *testing.T) {
	tabs := DefaultRuntimeTabs().
		Select(RuntimeTabToolchainHealth).
		WithRows(RuntimeTabToolchainHealth, []string{
			"toolchain: missing",
			"install httpx: missing required=true pack=web-api-pack command=ouro-toolchain install httpx",
		}).
		SelectRow(1)
	called := false
	model := NewModel().
		WithGraphStack(GraphStack{RuntimeTabs: tabs}).
		WithRuntimeActionRequester(func(action RuntimeAction) (RuntimeActionRequestResult, error) {
			called = true
			if action.Kind != "install" || action.Pack != "web-api-pack" || action.Name != "httpx" {
				t.Fatalf("unexpected requested action: %#v", action)
			}
			return RuntimeActionRequestResult{
				Status: "pending",
				Path:   "/tmp/ouro/request.json",
			}, nil
		})

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	if cmd == nil {
		t.Fatalf("expected runtime action request command")
	}
	if called {
		t.Fatalf("requester should not run before the returned command is executed")
	}
	msg := cmd()
	if !called {
		t.Fatalf("expected requester to run when command executes")
	}
	updated, cmd = updated.Update(msg)
	if cmd != nil {
		t.Fatalf("expected runtime action request result to be synchronous")
	}
	next, ok := updated.(Model)
	if !ok {
		t.Fatalf("expected Model, got %T", updated)
	}
	action, ok := next.graphStack.RuntimeTabs.ActiveRuntimeAction()
	if !ok {
		t.Fatalf("expected active runtime action")
	}
	if action.RequestStatus != "pending" || action.RequestPath != "/tmp/ouro/request.json" {
		t.Fatalf("expected pending request metadata, got %#v", action)
	}
	view := next.graphStack.RuntimeTabs.Render(120, 10)
	for _, want := range []string{
		"action: remediation selected install httpx",
		"request status: pending",
		"request path: /tmp/ouro/request.json",
		"action mode: metadata-only",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("runtime action request view missing %q:\n%s", want, view)
		}
	}
}

func TestModelUpdateExecutesDispatchArtifactOnlyAfterSecondConfirm(t *testing.T) {
	tabs := DefaultRuntimeTabs().
		Select(RuntimeTabToolchainHealth).
		WithRows(RuntimeTabToolchainHealth, []string{
			"toolchain: ready",
			"dispatch artifact: /tmp/ouro/dispatch.json status=dry-run mode=metadata-only ready=1 skipped=0",
		}).
		SelectRow(1)
	called := false
	model := NewModel().
		WithGraphStack(GraphStack{RuntimeTabs: tabs}).
		WithRuntimeActionExecutor(func(action RuntimeAction) (RuntimeActionExecutionResult, error) {
			called = true
			if action.Kind != "install-dispatch-artifact" || action.ArtifactPath != "/tmp/ouro/dispatch.json" {
				t.Fatalf("unexpected executed action: %#v", action)
			}
			return RuntimeActionExecutionResult{
				Status: "completed",
				Path:   "/tmp/ouro/execution.json",
				RuntimeRows: []string{
					"install execution artifacts: total=1",
					"execution artifact: /tmp/ouro/execution.json status=completed mode=execute dispatch=/tmp/ouro/dispatch.json results=1 skipped=0",
					"execution result web-api-pack/missing-httpx: completed",
				},
			}, nil
		})

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if cmd != nil {
		t.Fatalf("first x should only arm execution confirmation")
	}
	if called {
		t.Fatalf("executor must not run on first x")
	}
	next, ok := updated.(Model)
	if !ok {
		t.Fatalf("expected Model, got %T", updated)
	}
	action, ok := next.graphStack.RuntimeTabs.ActiveRuntimeAction()
	if !ok || action.ExecutionStatus != "confirm-required" {
		t.Fatalf("expected confirm-required active action, ok=%v action=%#v", ok, action)
	}
	view := next.graphStack.RuntimeTabs.Render(120, 12)
	for _, want := range []string{
		"action: install dispatch artifact selected /tmp/ouro/dispatch.json",
		"execution status: confirm-required",
		"execution confirm: press x again",
		"action mode: metadata-only",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("confirmation view missing %q:\n%s", want, view)
		}
	}

	updated, cmd = next.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if cmd == nil {
		t.Fatalf("second x should return execution command")
	}
	if called {
		t.Fatalf("executor should run only when returned command is executed")
	}
	msg := cmd()
	if !called {
		t.Fatalf("expected executor to run when command executes")
	}
	updated, cmd = updated.Update(msg)
	if cmd != nil {
		t.Fatalf("expected execution result update to be synchronous")
	}
	next, ok = updated.(Model)
	if !ok {
		t.Fatalf("expected Model, got %T", updated)
	}
	action, ok = next.graphStack.RuntimeTabs.ActiveRuntimeAction()
	if !ok || action.ExecutionStatus != "completed" || action.ExecutionPath != "/tmp/ouro/execution.json" {
		t.Fatalf("expected completed execution metadata, ok=%v action=%#v", ok, action)
	}
	selected, ok := next.graphStack.RuntimeTabs.ActiveSelectedRow()
	if !ok || !strings.HasPrefix(strings.TrimSpace(selected), "execution artifact: /tmp/ouro/execution.json") {
		t.Fatalf("expected newly written execution artifact to be selected, ok=%v row=%q", ok, selected)
	}
	updated, cmd = next.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	if cmd != nil {
		t.Fatalf("expected execution artifact detail update to be synchronous")
	}
	detailed, ok := updated.(Model)
	if !ok {
		t.Fatalf("expected Model, got %T", updated)
	}
	if detailed.graphStack.RuntimeAction.Kind != "install-execution-artifact" || detailed.graphStack.RuntimeAction.ArtifactPath != "/tmp/ouro/execution.json" {
		t.Fatalf("expected selected execution artifact details, got %#v", detailed.graphStack.RuntimeAction)
	}
	view = next.graphStack.RuntimeTabs.Render(140, 18)
	for _, want := range []string{
		"install execution artifacts: total=1",
		"execution artifact: /tmp/ouro/execution.json",
		"execution result web-api-pack/missing-httpx: completed",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected refreshed execution artifact rows %q:\n%s", want, view)
		}
	}
}

func TestModelUpdateExecutesCompilePreviewReviewExecutionDispatchArtifactOnlyAfterSecondConfirm(t *testing.T) {
	tabs := DefaultRuntimeTabs().
		Select(RuntimeTabToolchainHealth).
		WithRows(RuntimeTabToolchainHealth, []string{
			"toolchain: ready",
			"compile-preview review execution dispatch artifact: status=dry-run mode=metadata-only action=discover_assets nodes=1 warnings=0 actor=operator created_at=2026-06-04T12:15:00Z review=/tmp/ouro/review.json selection=/tmp/ouro/selection.json path=/tmp/ouro/dispatch.json reason=ready-for-tui",
		}).
		SelectRow(1)
	called := false
	model := NewModel().
		WithGraphStack(GraphStack{RuntimeTabs: tabs}).
		WithRuntimeActionExecutor(func(action RuntimeAction) (RuntimeActionExecutionResult, error) {
			called = true
			if action.Kind != "compile-preview-review-execution-dispatch-artifact" || action.ArtifactPath != "/tmp/ouro/dispatch.json" {
				t.Fatalf("unexpected executed action: %#v", action)
			}
			return RuntimeActionExecutionResult{
				Status: "completed",
				Path:   "/tmp/ouro/execution.json",
				RuntimeRows: []string{
					"compile-preview review executions: total=1",
					"compile-preview review execution artifact: status=completed mode=execute action=discover_assets nodes=1 warnings=0 events=12 actor=tui executed_at=2026-06-04T12:20:00Z run=compile-preview-review-execution-1 dispatch=/tmp/ouro/dispatch.json review=/tmp/ouro/review.json selection=/tmp/ouro/selection.json path=/tmp/ouro/execution.json reason=executed-from-tui",
				},
			}, nil
		})

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if cmd != nil {
		t.Fatalf("first x should only arm execution confirmation")
	}
	if called {
		t.Fatalf("executor must not run on first x")
	}
	next, ok := updated.(Model)
	if !ok {
		t.Fatalf("expected Model, got %T", updated)
	}
	action, ok := next.graphStack.RuntimeTabs.ActiveRuntimeAction()
	if !ok || action.ExecutionStatus != "confirm-required" {
		t.Fatalf("expected confirm-required active action, ok=%v action=%#v", ok, action)
	}
	view := next.graphStack.RuntimeTabs.Render(140, 18)
	for _, want := range []string{
		"action: compile preview review execution dispatch artifact selected /tmp/ouro/dispatch.json",
		"execution status: confirm-required",
		"execution confirm: press x again",
		"action mode: metadata-only",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("confirmation view missing %q:\n%s", want, view)
		}
	}

	updated, cmd = next.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if cmd == nil {
		t.Fatalf("second x should return execution command")
	}
	if called {
		t.Fatalf("executor should run only when returned command is executed")
	}
	msg := cmd()
	if !called {
		t.Fatalf("expected executor to run when command executes")
	}
	updated, cmd = updated.Update(msg)
	if cmd != nil {
		t.Fatalf("expected execution result update to be synchronous")
	}
	next, ok = updated.(Model)
	if !ok {
		t.Fatalf("expected Model, got %T", updated)
	}
	action, ok = next.graphStack.RuntimeTabs.ActiveRuntimeAction()
	if !ok || action.ExecutionStatus != "completed" || action.ExecutionPath != "/tmp/ouro/execution.json" {
		t.Fatalf("expected completed execution metadata, ok=%v action=%#v", ok, action)
	}
	selected, ok := next.graphStack.RuntimeTabs.ActiveSelectedRow()
	if !ok || !strings.HasPrefix(strings.TrimSpace(selected), "compile-preview review execution artifact:") || !strings.Contains(selected, "/tmp/ouro/execution.json") {
		t.Fatalf("expected newly written compile-preview execution artifact to be selected, ok=%v row=%q", ok, selected)
	}
}

func TestModelUpdatePersistsCompilePreviewReviewOnlyAfterSecondConfirm(t *testing.T) {
	tabs := DefaultRuntimeTabs().
		Select(RuntimeTabToolchainHealth).
		WithRows(RuntimeTabToolchainHealth, []string{
			"toolchain: ready",
			"compile preview: action=discover_assets mode=metadata-only nodes=1 warnings=1",
			"compile candidate: discover_assets:01 web-api-pack web.discovery runner=command parser=web_api_parser score=0 signals=- command=[\"sh\",\"-c\",\"touch /tmp/marker\"]",
		}).
		SelectRow(1)
	called := false
	model := NewModel().
		WithGraphStack(GraphStack{RuntimeTabs: tabs}).
		WithRuntimeActionExecutor(func(action RuntimeAction) (RuntimeActionExecutionResult, error) {
			called = true
			if action.Kind != "compile-preview" || action.ActionType != "discover_assets" || action.Nodes != "1" {
				t.Fatalf("unexpected executed action: %#v", action)
			}
			return RuntimeActionExecutionResult{
				Status: "completed",
				Path:   "/tmp/ouro/compile-preview-review.json",
				Output: "review created",
				RuntimeRows: []string{
					"compile-preview reviews: total=1",
					"compile-preview review artifact: status=dry-run mode=metadata-only action=discover_assets nodes=1 warnings=1 path=/tmp/ouro/compile-preview-review.json",
				},
			}, nil
		})

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if cmd != nil {
		t.Fatalf("first x should only arm compile-preview review confirmation")
	}
	if called {
		t.Fatalf("compile-preview executor must not run on first x")
	}
	next, ok := updated.(Model)
	if !ok {
		t.Fatalf("expected Model, got %T", updated)
	}
	action, ok := next.graphStack.RuntimeTabs.ActiveRuntimeAction()
	if !ok || action.ExecutionStatus != "confirm-required" {
		t.Fatalf("expected confirm-required compile-preview action, ok=%v action=%#v", ok, action)
	}
	view := next.graphStack.RuntimeTabs.Render(120, 12)
	for _, want := range []string{
		"action: compile preview selected discover_assets",
		"execution status: confirm-required",
		"execution confirm: press x again",
		"action mode: metadata-only",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("confirmation view missing %q:\n%s", want, view)
		}
	}

	updated, cmd = next.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if cmd == nil {
		t.Fatalf("second x should return compile-preview review command")
	}
	if called {
		t.Fatalf("compile-preview executor should run only when returned command is executed")
	}
	msg := cmd()
	if !called {
		t.Fatalf("expected compile-preview executor to run when command executes")
	}
	updated, cmd = updated.Update(msg)
	if cmd != nil {
		t.Fatalf("expected compile-preview review result update to be synchronous")
	}
	next, ok = updated.(Model)
	if !ok {
		t.Fatalf("expected Model, got %T", updated)
	}
	action, ok = next.graphStack.RuntimeTabs.ActiveRuntimeAction()
	if !ok || action.ExecutionStatus != "completed" || action.ExecutionPath != "/tmp/ouro/compile-preview-review.json" || action.Output != "review created" {
		t.Fatalf("expected completed compile-preview review metadata, ok=%v action=%#v", ok, action)
	}
	selected, ok := next.graphStack.RuntimeTabs.ActiveSelectedRow()
	if !ok || !strings.HasPrefix(strings.TrimSpace(selected), "compile-preview review artifact:") || !strings.Contains(selected, "/tmp/ouro/compile-preview-review.json") {
		t.Fatalf("expected newly written compile-preview review artifact to be selected, ok=%v row=%q", ok, selected)
	}
}

func TestModelUpdateClearsCompilePreviewReviewSelectionOnlyAfterSecondConfirm(t *testing.T) {
	tabs := DefaultRuntimeTabs().
		Select(RuntimeTabToolchainHealth).
		WithRows(RuntimeTabToolchainHealth, []string{
			"toolchain: ready",
			"compile-preview review selection: status=current mode=metadata-only review=/tmp/ouro/candidate.json action=discover_assets superseded=1 selected_by=alice selected_at=2026-06-01T00:15:00Z reason=best reviewed candidate",
			"compile-preview review artifact: status=dry-run mode=metadata-only action=discover_assets nodes=1 warnings=0 current=true path=/tmp/ouro/candidate.json",
		}).
		SelectRow(1)
	called := false
	model := NewModel().
		WithGraphStack(GraphStack{RuntimeTabs: tabs}).
		WithRuntimeActionExecutor(func(action RuntimeAction) (RuntimeActionExecutionResult, error) {
			called = true
			if action.Kind != "compile-preview-review-selection" || action.State != "current" || action.ReviewPath != "/tmp/ouro/candidate.json" {
				t.Fatalf("unexpected executed action: %#v", action)
			}
			return RuntimeActionExecutionResult{
				Status: "completed",
				Path:   "/tmp/ouro/compile-preview-review-selection.json",
				Output: "selection cleared",
				RuntimeRows: []string{
					"compile-preview reviews: total=1",
					"compile-preview review selection: status=cleared mode=metadata-only review=/tmp/ouro/candidate.json action=discover_assets superseded=1 cleared_by=tui cleared_at=2026-06-01T00:35:00Z reason=cleared from TUI",
					"compile-preview review artifact: status=dry-run mode=metadata-only action=discover_assets nodes=1 warnings=0 current=false path=/tmp/ouro/candidate.json",
				},
			}, nil
		})

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if cmd != nil {
		t.Fatalf("first x should only arm compile-preview review selection clear confirmation")
	}
	if called {
		t.Fatalf("selection clear executor must not run on first x")
	}
	next, ok := updated.(Model)
	if !ok {
		t.Fatalf("expected Model, got %T", updated)
	}
	action, ok := next.graphStack.RuntimeTabs.ActiveRuntimeAction()
	if !ok || action.ExecutionStatus != "confirm-required" {
		t.Fatalf("expected confirm-required selection action, ok=%v action=%#v", ok, action)
	}
	view := next.graphStack.RuntimeTabs.Render(140, 14)
	for _, want := range []string{
		"action: compile-preview review selection selected /tmp/ouro/candidate.json",
		"execution status: confirm-required",
		"execution confirm: press x again",
		"action mode: metadata-only",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("selection clear confirmation view missing %q:\n%s", want, view)
		}
	}

	updated, cmd = next.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if cmd == nil {
		t.Fatalf("second x should return selection clear command")
	}
	if called {
		t.Fatalf("selection clear executor should run only when returned command is executed")
	}
	msg := cmd()
	if !called {
		t.Fatalf("expected selection clear executor to run when command executes")
	}
	updated, cmd = updated.Update(msg)
	if cmd != nil {
		t.Fatalf("expected selection clear result update to be synchronous")
	}
	next, ok = updated.(Model)
	if !ok {
		t.Fatalf("expected Model, got %T", updated)
	}
	action, ok = next.graphStack.RuntimeTabs.ActiveRuntimeAction()
	if !ok || action.ExecutionStatus != "completed" || action.ExecutionPath != "/tmp/ouro/compile-preview-review-selection.json" || action.Output != "selection cleared" {
		t.Fatalf("expected completed selection clear metadata, ok=%v action=%#v", ok, action)
	}
	selected, ok := next.graphStack.RuntimeTabs.ActiveSelectedRow()
	if !ok || !strings.HasPrefix(strings.TrimSpace(selected), "compile-preview review selection: status=cleared") {
		t.Fatalf("expected cleared selection row to be selected, ok=%v row=%q", ok, selected)
	}
	rendered := next.graphStack.RuntimeTabs.Render(160, 18)
	if strings.Contains(rendered, "current=true") {
		t.Fatalf("cleared selection refresh must not leave stale current=true rows:\n%s", rendered)
	}
}

func TestModelUpdateSelectsCompilePreviewReviewArtifactOnlyAfterSecondConfirm(t *testing.T) {
	tabs := DefaultRuntimeTabs().
		Select(RuntimeTabToolchainHealth).
		WithRows(RuntimeTabToolchainHealth, []string{
			"toolchain: ready",
			"compile-preview review selection: status=current mode=metadata-only review=/tmp/ouro/base.json action=discover_assets superseded=1 selected_by=alice selected_at=2026-06-01T00:15:00Z reason=previous candidate",
			"compile-preview review artifact: status=dry-run mode=metadata-only action=discover_assets nodes=1 warnings=0 current=true path=/tmp/ouro/base.json",
			"compile-preview review artifact: status=dry-run mode=metadata-only action=discover_assets nodes=1 warnings=0 current=false path=/tmp/ouro/candidate.json",
		}).
		SelectRow(3)
	called := false
	model := NewModel().
		WithGraphStack(GraphStack{RuntimeTabs: tabs}).
		WithRuntimeActionExecutor(func(action RuntimeAction) (RuntimeActionExecutionResult, error) {
			called = true
			if action.Kind != "compile-preview-review-artifact" || action.ArtifactPath != "/tmp/ouro/candidate.json" {
				t.Fatalf("unexpected executed action: %#v", action)
			}
			return RuntimeActionExecutionResult{
				Status: "completed",
				Path:   "/tmp/ouro/compile-preview-review-selection.json",
				Output: "review selected",
				RuntimeRows: []string{
					"compile-preview reviews: total=2",
					"compile-preview review selection: status=current mode=metadata-only review=/tmp/ouro/candidate.json action=discover_assets superseded=1 selected_by=tui selected_at=2026-06-01T00:40:00Z reason=selected from TUI",
					"compile-preview review superseded: /tmp/ouro/base.json",
					"compile-preview review artifact: status=dry-run mode=metadata-only action=discover_assets nodes=1 warnings=0 current=false path=/tmp/ouro/base.json",
					"compile-preview review artifact: status=dry-run mode=metadata-only action=discover_assets nodes=1 warnings=0 current=true path=/tmp/ouro/candidate.json",
				},
			}, nil
		})

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if cmd != nil {
		t.Fatalf("first x should only arm compile-preview review selection confirmation")
	}
	if called {
		t.Fatalf("review selection executor must not run on first x")
	}
	next, ok := updated.(Model)
	if !ok {
		t.Fatalf("expected Model, got %T", updated)
	}
	action, ok := next.graphStack.RuntimeTabs.ActiveRuntimeAction()
	if !ok || action.ExecutionStatus != "confirm-required" {
		t.Fatalf("expected confirm-required review artifact action, ok=%v action=%#v", ok, action)
	}
	view := next.graphStack.RuntimeTabs.Render(150, 14)
	for _, want := range []string{
		"action: compile-preview review artifact selected /tmp/ouro/candidate.json",
		"execution status: confirm-required",
		"execution confirm: press x again",
		"action mode: metadata-only",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("review selection confirmation view missing %q:\n%s", want, view)
		}
	}

	updated, cmd = next.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if cmd == nil {
		t.Fatalf("second x should return review selection command")
	}
	if called {
		t.Fatalf("review selection executor should run only when returned command is executed")
	}
	msg := cmd()
	if !called {
		t.Fatalf("expected review selection executor to run when command executes")
	}
	updated, cmd = updated.Update(msg)
	if cmd != nil {
		t.Fatalf("expected review selection result update to be synchronous")
	}
	next, ok = updated.(Model)
	if !ok {
		t.Fatalf("expected Model, got %T", updated)
	}
	action, ok = next.graphStack.RuntimeTabs.ActiveRuntimeAction()
	if !ok || action.ExecutionStatus != "completed" || action.ExecutionPath != "/tmp/ouro/compile-preview-review-selection.json" || action.Output != "review selected" {
		t.Fatalf("expected completed review selection metadata, ok=%v action=%#v", ok, action)
	}
	selected, ok := next.graphStack.RuntimeTabs.ActiveSelectedRow()
	if !ok || !strings.HasPrefix(strings.TrimSpace(selected), "compile-preview review selection: status=current") || !strings.Contains(selected, "/tmp/ouro/candidate.json") {
		t.Fatalf("expected current selection row to be selected, ok=%v row=%q", ok, selected)
	}
	rendered := next.graphStack.RuntimeTabs.Render(180, 20)
	if strings.Contains(rendered, "current=false path=/tmp/ouro/candidate.json") {
		t.Fatalf("review selection refresh must not leave stale candidate current=false rows:\n%s", rendered)
	}
}

func TestModelUpdateCreatesCompilePreviewReviewExecutionDispatchFromCurrentReviewOnlyAfterSecondConfirm(t *testing.T) {
	tabs := DefaultRuntimeTabs().
		Select(RuntimeTabToolchainHealth).
		WithRows(RuntimeTabToolchainHealth, []string{
			"toolchain: ready",
			"compile-preview review selection: status=current mode=metadata-only review=/tmp/ouro/candidate.json action=discover_assets superseded=0 selected_by=alice selected_at=2026-06-04T12:00:00Z reason=ready for dispatch",
			"compile-preview review artifact: status=dry-run mode=metadata-only action=discover_assets nodes=1 warnings=0 current=true path=/tmp/ouro/candidate.json",
		}).
		SelectRow(2)
	called := false
	model := NewModel().
		WithGraphStack(GraphStack{RuntimeTabs: tabs}).
		WithRuntimeActionExecutor(func(action RuntimeAction) (RuntimeActionExecutionResult, error) {
			called = true
			if action.Kind != "compile-preview-review-artifact" || action.ArtifactPath != "/tmp/ouro/candidate.json" || action.Current != "true" {
				t.Fatalf("unexpected executed action: %#v", action)
			}
			return RuntimeActionExecutionResult{
				Status: "completed",
				Path:   "/tmp/ouro/dispatch.json",
				Output: "execution dispatch created",
				RuntimeRows: []string{
					"compile-preview review execution dispatches: total=1",
					"compile-preview review execution dispatch artifact: status=dry-run mode=metadata-only action=discover_assets nodes=1 warnings=0 actor=tui created_at=2026-06-04T12:05:00Z review=/tmp/ouro/candidate.json selection=/tmp/ouro/compile-preview-review-selection.json path=/tmp/ouro/dispatch.json reason=dispatched-from-tui",
				},
			}, nil
		})

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if cmd != nil {
		t.Fatalf("first x should only arm compile-preview review dispatch confirmation")
	}
	if called {
		t.Fatalf("dispatch executor must not run on first x")
	}
	next, ok := updated.(Model)
	if !ok {
		t.Fatalf("expected Model, got %T", updated)
	}
	action, ok := next.graphStack.RuntimeTabs.ActiveRuntimeAction()
	if !ok || action.ExecutionStatus != "confirm-required" {
		t.Fatalf("expected confirm-required current review artifact action, ok=%v action=%#v", ok, action)
	}
	view := next.graphStack.RuntimeTabs.Render(150, 16)
	for _, want := range []string{
		"action: compile-preview review artifact selected /tmp/ouro/candidate.json",
		"execution status: confirm-required",
		"execution confirm: press x again",
		"action mode: metadata-only",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("dispatch confirmation view missing %q:\n%s", want, view)
		}
	}

	updated, cmd = next.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if cmd == nil {
		t.Fatalf("second x should return compile-preview review dispatch command")
	}
	if called {
		t.Fatalf("dispatch executor should run only when returned command is executed")
	}
	msg := cmd()
	if !called {
		t.Fatalf("expected dispatch executor to run when command executes")
	}
	updated, cmd = updated.Update(msg)
	if cmd != nil {
		t.Fatalf("expected dispatch result update to be synchronous")
	}
	next, ok = updated.(Model)
	if !ok {
		t.Fatalf("expected Model, got %T", updated)
	}
	action, ok = next.graphStack.RuntimeTabs.ActiveRuntimeAction()
	if !ok || action.ExecutionStatus != "completed" || action.ExecutionPath != "/tmp/ouro/dispatch.json" || action.Output != "execution dispatch created" {
		t.Fatalf("expected completed dispatch metadata, ok=%v action=%#v", ok, action)
	}
	selected, ok := next.graphStack.RuntimeTabs.ActiveSelectedRow()
	if !ok || !strings.HasPrefix(strings.TrimSpace(selected), "compile-preview review execution dispatch artifact:") || !strings.Contains(selected, "/tmp/ouro/dispatch.json") {
		t.Fatalf("expected newly written compile-preview review dispatch artifact to be selected, ok=%v row=%q", ok, selected)
	}
}

func TestModelUpdateCreatesCompilePreviewReviewExecutionDispatchFromWorkflowStatusOnlyAfterSecondConfirm(t *testing.T) {
	row := "workflow status: mode=read-only status=ready stage=model-tool-plan next_gate=create-compile-preview-review-execution-dispatch reason=current-review-awaits-dispatch action=discover_assets preview=- acceptance=- review=/tmp/ouro/candidate.json selection=/tmp/ouro/compile-preview-review-selection.json dispatch=- execution=- report_dispatch=- report_execution=- command=ouro_toolchain_compile-preview-review-execution-dispatch"
	tabs := DefaultRuntimeTabs().
		Select(RuntimeTabModelRuntime).
		WithRows(RuntimeTabModelRuntime, []string{
			"model: ready",
			row,
		}).
		SelectRow(1)
	called := false
	model := NewModel().
		WithGraphStack(GraphStack{RuntimeTabs: tabs}).
		WithRuntimeActionExecutor(func(action RuntimeAction) (RuntimeActionExecutionResult, error) {
			called = true
			if action.Kind != "workflow-status" || action.Source != "model-tool-plan" || action.RequestPath != "create-compile-preview-review-execution-dispatch" || action.ReviewPath != "/tmp/ouro/candidate.json" {
				t.Fatalf("unexpected workflow status dispatch action: %#v", action)
			}
			return RuntimeActionExecutionResult{
				Status: "completed",
				Path:   "/tmp/ouro/dispatch.json",
				Output: "workflow status compile dispatch created",
				RuntimeRows: []string{
					"compile-preview review execution dispatches: total=1",
					"compile-preview review execution dispatch artifact: status=dry-run mode=metadata-only action=discover_assets nodes=1 warnings=0 actor=tui created_at=2026-06-04T12:05:00Z review=/tmp/ouro/candidate.json selection=/tmp/ouro/compile-preview-review-selection.json path=/tmp/ouro/dispatch.json reason=created_from_TUI_workflow_status",
				},
			}, nil
		})

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if cmd != nil {
		t.Fatalf("first x should only arm workflow-status compile dispatch confirmation")
	}
	if called {
		t.Fatalf("workflow-status dispatch executor must not run on first x")
	}
	next, ok := updated.(Model)
	if !ok {
		t.Fatalf("expected Model, got %T", updated)
	}
	action, ok := next.graphStack.RuntimeTabs.ActiveRuntimeAction()
	if !ok || action.ExecutionStatus != "confirm-required" {
		t.Fatalf("expected confirm-required workflow status action, ok=%v action=%#v", ok, action)
	}
	view := next.graphStack.RuntimeTabs.Render(180, 16)
	for _, want := range []string{
		"action: workflow status:",
		"request path: create-compile-preview-review-execution-dispatch",
		"execution status: confirm-required",
		"execution confirm: press x again",
		"action mode: metadata-only",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("workflow status dispatch confirmation view missing %q:\n%s", want, view)
		}
	}

	updated, cmd = next.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if cmd == nil {
		t.Fatalf("second x should return workflow-status dispatch command")
	}
	if called {
		t.Fatalf("workflow-status dispatch executor should run only when returned command is executed")
	}
	msg := cmd()
	if !called {
		t.Fatalf("expected workflow-status dispatch executor to run when command executes")
	}
	updated, cmd = updated.Update(msg)
	if cmd != nil {
		t.Fatalf("expected workflow-status dispatch result update to be synchronous")
	}
	next, ok = updated.(Model)
	if !ok {
		t.Fatalf("expected Model, got %T", updated)
	}
	action, ok = next.graphStack.RuntimeTabs.ActiveRuntimeAction()
	if !ok || action.ExecutionStatus != "completed" || action.ExecutionPath != "/tmp/ouro/dispatch.json" || action.Output != "workflow status compile dispatch created" {
		t.Fatalf("expected completed workflow-status dispatch metadata, ok=%v action=%#v", ok, action)
	}
	selected, ok := next.graphStack.RuntimeTabs.ActiveSelectedRow()
	if !ok || !strings.HasPrefix(strings.TrimSpace(selected), "compile-preview review execution dispatch artifact:") || !strings.Contains(selected, "/tmp/ouro/dispatch.json") {
		t.Fatalf("expected compile dispatch artifact row to be selected, ok=%v row=%q", ok, selected)
	}
}

func TestModelUpdateExecutesCompilePreviewReviewExecutionDispatchFromWorkflowStatusOnlyAfterSecondConfirm(t *testing.T) {
	row := "workflow status: mode=read-only status=ready stage=model-tool-plan next_gate=execute-compile-preview-review-execution-dispatch reason=dispatch-awaits-execution action=discover_assets preview=- acceptance=- review=/tmp/ouro/candidate.json selection=/tmp/ouro/compile-preview-review-selection.json dispatch=/tmp/ouro/dispatch.json execution=- report_dispatch=- report_execution=- command=ouro_toolchain_execute-compile-preview-review-execution-dispatch"
	tabs := DefaultRuntimeTabs().
		Select(RuntimeTabModelRuntime).
		WithRows(RuntimeTabModelRuntime, []string{
			"model: ready",
			row,
		}).
		SelectRow(1)
	called := false
	model := NewModel().
		WithGraphStack(GraphStack{RuntimeTabs: tabs}).
		WithRuntimeActionExecutor(func(action RuntimeAction) (RuntimeActionExecutionResult, error) {
			called = true
			if action.Kind != "workflow-status" || action.Source != "model-tool-plan" || action.RequestPath != "execute-compile-preview-review-execution-dispatch" || action.DispatchPath != "/tmp/ouro/dispatch.json" || action.ArtifactPath != "/tmp/ouro/dispatch.json" {
				t.Fatalf("unexpected workflow status compile dispatch execution action: %#v", action)
			}
			return RuntimeActionExecutionResult{
				Status: "completed",
				Path:   "/tmp/ouro/execution.json",
				Output: "workflow status compile dispatch executed",
				RuntimeRows: []string{
					"compile-preview review executions: total=1",
					"compile-preview review execution artifact: status=completed mode=execute action=discover_assets nodes=1 warnings=0 events=4 actor=tui executed_at=2026-06-04T12:30:00Z run=compile-run dispatch=/tmp/ouro/dispatch.json review=/tmp/ouro/candidate.json selection=/tmp/ouro/compile-preview-review-selection.json path=/tmp/ouro/execution.json reason=executed_from_TUI_workflow_status",
				},
			}, nil
		})

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if cmd != nil {
		t.Fatalf("first x should only arm workflow-status compile dispatch execution confirmation")
	}
	if called {
		t.Fatalf("workflow-status compile dispatch execution must not run on first x")
	}
	next, ok := updated.(Model)
	if !ok {
		t.Fatalf("expected Model, got %T", updated)
	}
	action, ok := next.graphStack.RuntimeTabs.ActiveRuntimeAction()
	if !ok || action.ExecutionStatus != "confirm-required" {
		t.Fatalf("expected confirm-required workflow status execution action, ok=%v action=%#v", ok, action)
	}
	view := next.graphStack.RuntimeTabs.Render(180, 16)
	for _, want := range []string{
		"action: workflow status:",
		"request path: execute-compile-preview-review-execution-dispatch",
		"execution status: confirm-required",
		"execution confirm: press x again",
		"action mode: metadata-only",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("workflow status compile execution confirmation view missing %q:\n%s", want, view)
		}
	}

	updated, cmd = next.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if cmd == nil {
		t.Fatalf("second x should return workflow-status compile dispatch execution command")
	}
	if called {
		t.Fatalf("workflow-status compile dispatch execution executor should run only when returned command is executed")
	}
	msg := cmd()
	if !called {
		t.Fatalf("expected workflow-status compile dispatch execution executor to run when command executes")
	}
	updated, cmd = updated.Update(msg)
	if cmd != nil {
		t.Fatalf("expected workflow-status compile dispatch execution result update to be synchronous")
	}
	next, ok = updated.(Model)
	if !ok {
		t.Fatalf("expected Model, got %T", updated)
	}
	action, ok = next.graphStack.RuntimeTabs.ActiveRuntimeAction()
	if !ok || action.ExecutionStatus != "completed" || action.ExecutionPath != "/tmp/ouro/execution.json" || action.Output != "workflow status compile dispatch executed" {
		t.Fatalf("expected completed workflow-status compile execution metadata, ok=%v action=%#v", ok, action)
	}
	if next.graphStack.RuntimeTabs.Active != RuntimeTabToolchainHealth {
		t.Fatalf("expected workflow-status compile execution result to select Toolchain Health, got %s", next.graphStack.RuntimeTabs.Active)
	}
	selected, ok := next.graphStack.RuntimeTabs.ActiveSelectedRow()
	if !ok || !strings.HasPrefix(strings.TrimSpace(selected), "compile-preview review execution artifact:") || !strings.Contains(selected, "/tmp/ouro/execution.json") {
		t.Fatalf("expected compile execution artifact row to be selected, ok=%v row=%q", ok, selected)
	}
}

func TestRuntimeWorkflowStatusActionUsesEncodedPathFields(t *testing.T) {
	previewPath := "/tmp/ouro space/runtime/model-tool-plan-previews/preview one.json"
	acceptancePath := "/tmp/ouro space/runtime/model-tool-plan-preview-acceptances/accept one.json"
	reviewPath := "/tmp/ouro space/toolchain/compile-preview-reviews/review one.json"
	selectionPath := "/tmp/ouro space/toolchain/compile-preview-review-selection.json"
	dispatchPath := "/tmp/ouro space/toolchain/compile-preview-review-execution-dispatches/dispatch one.json"
	executionPath := "/tmp/ouro space/toolchain/compile-preview-review-executions/execution one.json"
	reportDispatchPath := "/tmp/ouro space/reports/report-projection-dispatches/report dispatch.json"
	reportExecutionPath := "/tmp/ouro space/reports/report-projection-dispatch-executions/report execution.json"
	reportSelectionPath := "/tmp/ouro space/reports/report-projection-selection.json"
	reportPath := "/tmp/ouro space/reports/run one.md"
	row := strings.Join([]string{
		"workflow status: mode=read-only status=ready stage=report-projection next_gate=execute-report-projection-dispatch reason=report-dispatch-awaits-execution safety_errors=0 action=discover_assets command=ouro_execute-report-project-dispatch",
		"preview=" + strings.Join(strings.Fields(previewPath), "_"),
		"preview_b64=" + base64.RawURLEncoding.EncodeToString([]byte(previewPath)),
		"acceptance=" + strings.Join(strings.Fields(acceptancePath), "_"),
		"acceptance_b64=" + base64.RawURLEncoding.EncodeToString([]byte(acceptancePath)),
		"review=" + strings.Join(strings.Fields(reviewPath), "_"),
		"review_b64=" + base64.RawURLEncoding.EncodeToString([]byte(reviewPath)),
		"selection=" + strings.Join(strings.Fields(selectionPath), "_"),
		"selection_b64=" + base64.RawURLEncoding.EncodeToString([]byte(selectionPath)),
		"dispatch=" + strings.Join(strings.Fields(dispatchPath), "_"),
		"dispatch_b64=" + base64.RawURLEncoding.EncodeToString([]byte(dispatchPath)),
		"execution=" + strings.Join(strings.Fields(executionPath), "_"),
		"execution_b64=" + base64.RawURLEncoding.EncodeToString([]byte(executionPath)),
		"report_dispatch=" + strings.Join(strings.Fields(reportDispatchPath), "_"),
		"report_dispatch_b64=" + base64.RawURLEncoding.EncodeToString([]byte(reportDispatchPath)),
		"report_execution=" + strings.Join(strings.Fields(reportExecutionPath), "_"),
		"report_execution_b64=" + base64.RawURLEncoding.EncodeToString([]byte(reportExecutionPath)),
		"report_selection=" + strings.Join(strings.Fields(reportSelectionPath), "_"),
		"report_selection_b64=" + base64.RawURLEncoding.EncodeToString([]byte(reportSelectionPath)),
		"report_run=run-123",
		"report=" + strings.Join(strings.Fields(reportPath), "_"),
		"report_b64=" + base64.RawURLEncoding.EncodeToString([]byte(reportPath)),
	}, " ")

	action, ok := runtimeActionForRow(row)
	if !ok {
		t.Fatalf("expected workflow status action")
	}
	if action.ArtifactPath != reportDispatchPath || action.DispatchPath != reportDispatchPath || action.ReportDispatchPath != reportDispatchPath {
		t.Fatalf("expected report dispatch paths to decode, got %#v", action)
	}
	if action.ReviewPath != reviewPath || action.ExecutionPath != reportExecutionPath || action.ReportExecutionPath != reportExecutionPath {
		t.Fatalf("expected review/report execution paths to decode, got %#v", action)
	}
	if action.PlannedReportPath != reportPath || !strings.Contains(action.Output, "report_selection="+reportSelectionPath) {
		t.Fatalf("expected report path and selection output to decode, got %#v", action)
	}
}

func TestModelUpdateDoesNotExecuteUnsupportedCompileWorkflowStatusRows(t *testing.T) {
	rows := []string{
		"workflow status: mode=read-only status=ready stage=model-tool-plan next_gate=create-compile-preview-review-execution-dispatch reason=current-review-awaits-dispatch action=discover_assets preview=- acceptance=- review=- selection=/tmp/ouro/compile-preview-review-selection.json dispatch=- execution=- command=ouro_toolchain_compile-preview-review-execution-dispatch",
		"workflow status: mode=read-only status=blocked stage=model-tool-plan next_gate=create-compile-preview-review-execution-dispatch reason=current-review-awaits-dispatch action=discover_assets preview=- acceptance=- review=/tmp/ouro/candidate.json selection=/tmp/ouro/compile-preview-review-selection.json dispatch=- execution=- command=ouro_toolchain_compile-preview-review-execution-dispatch",
		"workflow status: mode=read-only status=ready stage=model-tool-plan next_gate=execute-compile-preview-review-execution-dispatch reason=dispatch-awaits-execution action=discover_assets preview=- acceptance=- review=/tmp/ouro/candidate.json selection=/tmp/ouro/compile-preview-review-selection.json dispatch=- execution=- command=ouro_toolchain_execute-compile-preview-review-execution-dispatch",
		"workflow status: mode=read-only status=ready stage=model-tool-plan next_gate=execute-compile-preview-review-execution-dispatch reason=dispatch-awaits-execution action=discover_assets preview=- acceptance=- review=/tmp/ouro/candidate.json selection=/tmp/ouro/compile-preview-review-selection.json dispatch=none execution=- command=ouro_toolchain_execute-compile-preview-review-execution-dispatch",
		"workflow status: mode=read-only status=ready stage=report-projection next_gate=execute-compile-preview-review-execution-dispatch reason=dispatch-awaits-execution action=discover_assets preview=- acceptance=- review=/tmp/ouro/candidate.json selection=/tmp/ouro/compile-preview-review-selection.json dispatch=/tmp/ouro/dispatch.json execution=- command=ouro_toolchain_execute-compile-preview-review-execution-dispatch",
	}
	for _, row := range rows {
		t.Run(runtimeRowField(row, "next_gate")+"_"+runtimeRowField(row, "status")+"_"+runtimeRowField(row, "stage")+"_"+runtimeRowField(row, "dispatch")+"_"+runtimeRowField(row, "review"), func(t *testing.T) {
			tabs := DefaultRuntimeTabs().
				Select(RuntimeTabModelRuntime).
				WithRows(RuntimeTabModelRuntime, []string{
					"model: ready",
					row,
				}).
				SelectRow(1)
			called := false
			model := NewModel().
				WithGraphStack(GraphStack{RuntimeTabs: tabs}).
				WithRuntimeActionExecutor(func(action RuntimeAction) (RuntimeActionExecutionResult, error) {
					called = true
					return RuntimeActionExecutionResult{Status: "unexpected"}, nil
				})

			updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
			if cmd != nil {
				t.Fatalf("unsupported workflow status must not create execution command")
			}
			if called {
				t.Fatalf("unsupported workflow status must not call executor")
			}
			next, ok := updated.(Model)
			if !ok {
				t.Fatalf("expected Model, got %T", updated)
			}
			action, ok := next.graphStack.RuntimeTabs.ActiveRuntimeAction()
			if !ok || action.Kind != "workflow-status" || action.ExecutionStatus != "unsupported" || action.ExecutionError != "runtime action is not executable" {
				t.Fatalf("expected unsupported workflow status execution metadata, ok=%v action=%#v", ok, action)
			}
		})
	}
}

func TestModelUpdateExecutesRetryDispatchArtifactOnlyAfterSecondConfirm(t *testing.T) {
	tabs := DefaultRuntimeTabs().
		Select(RuntimeTabToolchainHealth).
		WithRows(RuntimeTabToolchainHealth, []string{
			"toolchain: ready",
			"retry dispatch artifact: /tmp/ouro/retry-dispatch.json status=dry-run mode=metadata-only ready=1 skipped=0",
		}).
		SelectRow(1)
	called := false
	model := NewModel().
		WithGraphStack(GraphStack{RuntimeTabs: tabs}).
		WithRuntimeActionExecutor(func(action RuntimeAction) (RuntimeActionExecutionResult, error) {
			called = true
			if action.Kind != "install-retry-dispatch-artifact" || action.ArtifactPath != "/tmp/ouro/retry-dispatch.json" {
				t.Fatalf("unexpected executed action: %#v", action)
			}
			return RuntimeActionExecutionResult{
				Status: "completed",
				Path:   "/tmp/ouro/retry-execution.json",
				RuntimeRows: []string{
					"install retry execution artifacts: total=1",
					"retry execution artifact: /tmp/ouro/retry-execution.json status=completed mode=execute retry_dispatch=/tmp/ouro/retry-dispatch.json results=1 skipped=0",
					"retry execution result web-api-pack/missing-httpx: completed output=retried error= artifact=/tmp/ouro/retry-execution.json retry_dispatch=/tmp/ouro/retry-dispatch.json source_execution=/tmp/ouro/source-execution.json dispatch=/tmp/ouro/dispatch.json command=sh -c echo retried",
				},
			}, nil
		})

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if cmd != nil {
		t.Fatalf("first x should only arm retry execution confirmation")
	}
	if called {
		t.Fatalf("retry executor must not run on first x")
	}
	next, ok := updated.(Model)
	if !ok {
		t.Fatalf("expected Model, got %T", updated)
	}
	action, ok := next.graphStack.RuntimeTabs.ActiveRuntimeAction()
	if !ok || action.ExecutionStatus != "confirm-required" {
		t.Fatalf("expected confirm-required retry action, ok=%v action=%#v", ok, action)
	}

	updated, cmd = next.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if cmd == nil {
		t.Fatalf("second x should return retry execution command")
	}
	if called {
		t.Fatalf("retry executor should run only when returned command is executed")
	}
	msg := cmd()
	if !called {
		t.Fatalf("expected retry executor to run when command executes")
	}
	updated, cmd = updated.Update(msg)
	if cmd != nil {
		t.Fatalf("expected retry execution result update to be synchronous")
	}
	next, ok = updated.(Model)
	if !ok {
		t.Fatalf("expected Model, got %T", updated)
	}
	action, ok = next.graphStack.RuntimeTabs.ActiveRuntimeAction()
	if !ok || action.ExecutionStatus != "completed" || action.ExecutionPath != "/tmp/ouro/retry-execution.json" {
		t.Fatalf("expected completed retry execution metadata, ok=%v action=%#v", ok, action)
	}
	selected, ok := next.graphStack.RuntimeTabs.ActiveSelectedRow()
	if !ok || !strings.HasPrefix(strings.TrimSpace(selected), "retry execution artifact: /tmp/ouro/retry-execution.json") {
		t.Fatalf("expected newly written retry execution artifact to be selected, ok=%v row=%q", ok, selected)
	}
}

func TestModelUpdateExecutesWorkerLeaseReclaimArtifactOnlyAfterSecondConfirm(t *testing.T) {
	tabs := DefaultRuntimeTabs().
		Select(RuntimeTabAgentRuntime).
		WithRows(RuntimeTabAgentRuntime, []string{
			"agents: 0",
			"worker lease reclaim artifact: /tmp/ouro/reclaim.json status=dry-run mode=metadata-only ready=1 skipped=0",
		}).
		SelectRow(1)
	called := false
	model := NewModel().
		WithGraphStack(GraphStack{RuntimeTabs: tabs}).
		WithRuntimeActionExecutor(func(action RuntimeAction) (RuntimeActionExecutionResult, error) {
			called = true
			if action.Kind != "worker-lease-reclaim-artifact" || action.ArtifactPath != "/tmp/ouro/reclaim.json" {
				t.Fatalf("unexpected executed action: %#v", action)
			}
			return RuntimeActionExecutionResult{
				Status: "completed",
				Path:   "/tmp/ouro/reclaim-execution.json",
				RuntimeRows: []string{
					"worker lease reclaim execution artifacts: total=1",
					"worker lease reclaim execution artifact: /tmp/ouro/reclaim-execution.json status=completed mode=execute dispatch=/tmp/ouro/reclaim.json results=1 skipped=0",
					"worker lease reclaim execution result run=run-stale execution=tool-stale status=reclaimed worker=local",
				},
			}, nil
		})

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if cmd != nil {
		t.Fatalf("first x should only arm worker lease reclaim confirmation")
	}
	if called {
		t.Fatalf("worker lease reclaim executor must not run on first x")
	}
	next, ok := updated.(Model)
	if !ok {
		t.Fatalf("expected Model, got %T", updated)
	}
	action, ok := next.graphStack.RuntimeTabs.ActiveRuntimeAction()
	if !ok || action.ExecutionStatus != "confirm-required" {
		t.Fatalf("expected confirm-required worker lease reclaim action, ok=%v action=%#v", ok, action)
	}
	view := next.graphStack.RuntimeTabs.Render(140, 14)
	for _, want := range []string{
		"action: worker lease reclaim artifact selected /tmp/ouro/reclaim.json",
		"execution status: confirm-required",
		"execution confirm: press x again",
		"action mode: metadata-only",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("confirmation view missing %q:\n%s", want, view)
		}
	}

	updated, cmd = next.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if cmd == nil {
		t.Fatalf("second x should return worker lease reclaim execution command")
	}
	if called {
		t.Fatalf("worker lease reclaim executor should run only when returned command is executed")
	}
	msg := cmd()
	if !called {
		t.Fatalf("expected worker lease reclaim executor to run when command executes")
	}
	updated, cmd = updated.Update(msg)
	if cmd != nil {
		t.Fatalf("expected worker lease reclaim execution result update to be synchronous")
	}
	next, ok = updated.(Model)
	if !ok {
		t.Fatalf("expected Model, got %T", updated)
	}
	action, ok = next.graphStack.RuntimeTabs.ActiveRuntimeAction()
	if !ok || action.ExecutionStatus != "completed" || action.ExecutionPath != "/tmp/ouro/reclaim-execution.json" {
		t.Fatalf("expected completed worker lease reclaim execution metadata, ok=%v action=%#v", ok, action)
	}
	selected, ok := next.graphStack.RuntimeTabs.ActiveSelectedRow()
	if !ok || !strings.HasPrefix(strings.TrimSpace(selected), "worker lease reclaim execution artifact: /tmp/ouro/reclaim-execution.json") {
		t.Fatalf("expected newly written worker lease reclaim execution artifact to be selected, ok=%v row=%q", ok, selected)
	}
}

func TestRuntimeWorkerLeaseReclaimArtifactUsesEncodedReferenceFields(t *testing.T) {
	artifactPath := "/tmp/ouro/runtime/worker lease reclaims/reclaim status=failed mode=execute.json"
	row := "worker lease reclaim artifact: " + runtimeCompactRowTextValue(artifactPath) +
		" artifact_b64=" + base64.RawURLEncoding.EncodeToString([]byte(artifactPath)) +
		" status=dry-run mode=metadata-only ready=1 skipped=0"

	action := runtimeWorkerLeaseReclaimArtifactActionForRow(row)

	if action.ArtifactPath != artifactPath {
		t.Fatalf("expected encoded reclaim artifact path, got %#v", action)
	}
	if action.State != "dry-run" || action.ExecutionMode != "metadata-only" || action.Ready != "1" || action.Skipped != "0" {
		t.Fatalf("encoded reclaim artifact must not retarget machine fields: %#v", action)
	}
}

func TestRuntimeWorkerLeaseReclaimExecutionUsesEncodedReferenceFields(t *testing.T) {
	artifactPath := "/tmp/ouro/runtime/worker lease reclaim executions/reclaim execution status=failed mode=metadata-only.json"
	dispatchPath := "/tmp/ouro/runtime/worker lease reclaims/reclaim dispatch status=failed.json"
	artifactRow := "worker lease reclaim execution artifact: " + runtimeCompactRowTextValue(artifactPath) +
		" artifact_b64=" + base64.RawURLEncoding.EncodeToString([]byte(artifactPath)) +
		" status=completed mode=execute" +
		" dispatch=" + runtimeCompactRowTextValue(dispatchPath) +
		" dispatch_b64=" + base64.RawURLEncoding.EncodeToString([]byte(dispatchPath)) +
		" results=1 skipped=0"

	artifactAction := runtimeWorkerLeaseReclaimExecutionArtifactActionForRow(artifactRow)
	if artifactAction.ArtifactPath != artifactPath || artifactAction.DispatchPath != dispatchPath {
		t.Fatalf("expected encoded reclaim execution artifact references, got %#v", artifactAction)
	}
	if artifactAction.State != "completed" || artifactAction.ExecutionMode != "execute" || artifactAction.Results != "1" || artifactAction.Skipped != "0" {
		t.Fatalf("encoded reclaim execution artifact must not retarget machine fields: %#v", artifactAction)
	}

	runID := "run stale status=failed"
	executionID := "tool stale status=failed"
	workerID := "local worker status=failed"
	leasePath := "/tmp/ouro/runtime/worker leases/lease status=active.json"
	resultRow := "worker lease reclaim execution result run=" + runtimeCompactRowTextValue(runID) +
		" run_b64=" + base64.RawURLEncoding.EncodeToString([]byte(runID)) +
		" execution=" + runtimeCompactRowTextValue(executionID) +
		" execution_b64=" + base64.RawURLEncoding.EncodeToString([]byte(executionID)) +
		" status=reclaimed" +
		" worker=" + runtimeCompactRowTextValue(workerID) +
		" worker_b64=" + base64.RawURLEncoding.EncodeToString([]byte(workerID)) +
		" lease_path=" + runtimeCompactRowTextValue(leasePath) +
		" lease_path_b64=" + base64.RawURLEncoding.EncodeToString([]byte(leasePath))

	resultAction := runtimeWorkerLeaseReclaimExecutionResultActionForRow(resultRow)
	if resultAction.RunID != runID || resultAction.ExecutionID != executionID || resultAction.Name != runID+"/"+executionID || resultAction.WorkerID != workerID || resultAction.LeasePath != leasePath {
		t.Fatalf("expected encoded reclaim execution result references, got %#v", resultAction)
	}
	if resultAction.State != "reclaimed" {
		t.Fatalf("encoded reclaim execution result must not retarget state: %#v", resultAction)
	}
}

func TestModelUpdateExecutesWorkerLeaseResultReconciliationArtifactOnlyAfterSecondConfirm(t *testing.T) {
	tabs := DefaultRuntimeTabs().
		Select(RuntimeTabAgentRuntime).
		WithRows(RuntimeTabAgentRuntime, []string{
			"agents: 0",
			"worker lease result reconciliation artifact: /tmp/ouro/reconciliation.json status=dry-run mode=metadata-only ready=1 skipped=0",
		}).
		SelectRow(1)
	called := false
	model := NewModel().
		WithGraphStack(GraphStack{RuntimeTabs: tabs}).
		WithRuntimeActionExecutor(func(action RuntimeAction) (RuntimeActionExecutionResult, error) {
			called = true
			if action.Kind != "worker-lease-result-reconciliation-artifact" || action.ArtifactPath != "/tmp/ouro/reconciliation.json" {
				t.Fatalf("unexpected executed action: %#v", action)
			}
			return RuntimeActionExecutionResult{
				Status: "completed",
				Path:   "/tmp/ouro/reconciliation-execution.json",
				RuntimeRows: []string{
					"worker lease result reconciliation execution artifacts: total=1",
					"worker lease result reconciliation execution artifact: /tmp/ouro/reconciliation-execution.json status=completed mode=execute results=1 skipped=0",
					"lease result reconciliation execution result run=run-1 execution=tool-1 result=completed lease=completed worker=remote-web-1",
				},
			}, nil
		})

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if cmd != nil {
		t.Fatalf("first x should only arm worker lease result reconciliation confirmation")
	}
	if called {
		t.Fatalf("worker lease result reconciliation executor must not run on first x")
	}
	next, ok := updated.(Model)
	if !ok {
		t.Fatalf("expected Model, got %T", updated)
	}
	action, ok := next.graphStack.RuntimeTabs.ActiveRuntimeAction()
	if !ok || action.ExecutionStatus != "confirm-required" {
		t.Fatalf("expected confirm-required worker lease result reconciliation action, ok=%v action=%#v", ok, action)
	}
	view := next.graphStack.RuntimeTabs.Render(150, 14)
	for _, want := range []string{
		"action: worker lease result reconciliation artifact selected /tmp/ouro/reconciliation.json",
		"execution status: confirm-required",
		"execution confirm: press x again",
		"action mode: metadata-only",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("confirmation view missing %q:\n%s", want, view)
		}
	}

	updated, cmd = next.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if cmd == nil {
		t.Fatalf("second x should return worker lease result reconciliation execution command")
	}
	if called {
		t.Fatalf("worker lease result reconciliation executor should run only when returned command is executed")
	}
	msg := cmd()
	if !called {
		t.Fatalf("expected worker lease result reconciliation executor to run when command executes")
	}
	updated, cmd = updated.Update(msg)
	if cmd != nil {
		t.Fatalf("expected worker lease result reconciliation execution result update to be synchronous")
	}
	next, ok = updated.(Model)
	if !ok {
		t.Fatalf("expected Model, got %T", updated)
	}
	action, ok = next.graphStack.RuntimeTabs.ActiveRuntimeAction()
	if !ok || action.ExecutionStatus != "completed" || action.ExecutionPath != "/tmp/ouro/reconciliation-execution.json" {
		t.Fatalf("expected completed worker lease result reconciliation execution metadata, ok=%v action=%#v", ok, action)
	}
	selected, ok := next.graphStack.RuntimeTabs.ActiveSelectedRow()
	if !ok || !strings.HasPrefix(strings.TrimSpace(selected), "worker lease result reconciliation execution artifact: /tmp/ouro/reconciliation-execution.json") {
		t.Fatalf("expected newly written worker lease result reconciliation execution artifact to be selected, ok=%v row=%q", ok, selected)
	}
	updated, cmd = next.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	if cmd != nil {
		t.Fatalf("expected worker lease result reconciliation execution artifact detail update to be synchronous")
	}
	detailed, ok := updated.(Model)
	if !ok {
		t.Fatalf("expected Model, got %T", updated)
	}
	if detailed.graphStack.RuntimeAction.Kind != "worker-lease-result-reconciliation-execution-artifact" || detailed.graphStack.RuntimeAction.ArtifactPath != "/tmp/ouro/reconciliation-execution.json" {
		t.Fatalf("expected selected worker lease result reconciliation execution artifact details, got %#v", detailed.graphStack.RuntimeAction)
	}
}

func TestRuntimeWorkerLeaseResultReconciliationArtifactUsesEncodedReferenceFields(t *testing.T) {
	artifactPath := "/tmp/ouro/runtime/worker lease result reconciliations/reconcile status=failed mode=execute.json"
	row := "worker lease result reconciliation artifact: " + runtimeCompactRowTextValue(artifactPath) +
		" artifact_b64=" + base64.RawURLEncoding.EncodeToString([]byte(artifactPath)) +
		" status=dry-run mode=metadata-only ready=1 skipped=0"

	action := runtimeWorkerLeaseResultReconciliationArtifactActionForRow(row)

	if action.ArtifactPath != artifactPath {
		t.Fatalf("expected encoded reconciliation artifact path, got %#v", action)
	}
	if action.State != "dry-run" || action.ExecutionMode != "metadata-only" || action.Ready != "1" || action.Skipped != "0" {
		t.Fatalf("encoded reconciliation artifact must not retarget machine fields: %#v", action)
	}
}

func TestRuntimeWorkerLeaseResultReconciliationExecutionUsesEncodedReferenceFields(t *testing.T) {
	artifactPath := "/tmp/ouro/runtime/worker lease result reconciliation executions/reconcile execution status=failed.json"
	dispatchPath := "/tmp/ouro/runtime/worker lease result reconciliations/reconcile dispatch mode=metadata-only.json"
	artifactRow := "worker lease result reconciliation execution artifact: " + runtimeCompactRowTextValue(artifactPath) +
		" artifact_b64=" + base64.RawURLEncoding.EncodeToString([]byte(artifactPath)) +
		" status=completed mode=execute" +
		" dispatch=" + runtimeCompactRowTextValue(dispatchPath) +
		" dispatch_b64=" + base64.RawURLEncoding.EncodeToString([]byte(dispatchPath)) +
		" results=1 skipped=0"

	artifactAction := runtimeWorkerLeaseResultReconciliationExecutionArtifactActionForRow(artifactRow)
	if artifactAction.ArtifactPath != artifactPath || artifactAction.DispatchPath != dispatchPath {
		t.Fatalf("expected encoded reconciliation execution artifact references, got %#v", artifactAction)
	}
	if artifactAction.State != "completed" || artifactAction.ExecutionMode != "execute" || artifactAction.Results != "1" || artifactAction.Skipped != "0" {
		t.Fatalf("encoded reconciliation execution artifact must not retarget machine fields: %#v", artifactAction)
	}

	runID := "run reconcile status=failed"
	executionID := "tool reconcile status=failed"
	workerID := "remote web status=failed"
	leasePath := "/tmp/ouro/runtime/worker leases/lease status=active.json"
	resultPath := "/tmp/ouro/runtime/worker results/result lease=failed.json"
	queueDispatchPath := "/tmp/ouro/runtime/worker queue dispatches/dispatch worker=evil.json"
	resultRow := "lease result reconciliation execution result run=" + runtimeCompactRowTextValue(runID) +
		" run_b64=" + base64.RawURLEncoding.EncodeToString([]byte(runID)) +
		" execution=" + runtimeCompactRowTextValue(executionID) +
		" execution_b64=" + base64.RawURLEncoding.EncodeToString([]byte(executionID)) +
		" result=completed lease=completed" +
		" worker=" + runtimeCompactRowTextValue(workerID) +
		" worker_b64=" + base64.RawURLEncoding.EncodeToString([]byte(workerID)) +
		" lease_path=" + runtimeCompactRowTextValue(leasePath) +
		" lease_path_b64=" + base64.RawURLEncoding.EncodeToString([]byte(leasePath)) +
		" result_path=" + runtimeCompactRowTextValue(resultPath) +
		" result_path_b64=" + base64.RawURLEncoding.EncodeToString([]byte(resultPath)) +
		" dispatch=" + runtimeCompactRowTextValue(queueDispatchPath) +
		" dispatch_b64=" + base64.RawURLEncoding.EncodeToString([]byte(queueDispatchPath))

	resultAction := runtimeWorkerLeaseResultReconciliationExecutionResultActionForRow(resultRow)
	if resultAction.RunID != runID || resultAction.ExecutionID != executionID || resultAction.Name != runID+"/"+executionID || resultAction.WorkerID != workerID {
		t.Fatalf("expected encoded reconciliation execution result identities, got %#v", resultAction)
	}
	if resultAction.LeasePath != leasePath || resultAction.ResultPath != resultPath || resultAction.DispatchPath != queueDispatchPath {
		t.Fatalf("expected encoded reconciliation execution result paths, got %#v", resultAction)
	}
	if resultAction.State != "completed" || resultAction.Output != "completed" {
		t.Fatalf("encoded reconciliation execution result must not retarget statuses: %#v", resultAction)
	}
}

func TestModelUpdateExecutesWorkerQueueClaimArtifactOnlyAfterSecondConfirm(t *testing.T) {
	tabs := DefaultRuntimeTabs().
		Select(RuntimeTabAgentRuntime).
		WithRows(RuntimeTabAgentRuntime, []string{
			"agents: 0",
			"worker queue claim artifact: /tmp/ouro/claim.json status=claimed mode=metadata-only worker=remote-web-1 run=run-dispatch-1 execution=tool-web",
		}).
		SelectRow(1)
	called := false
	model := NewModel().
		WithGraphStack(GraphStack{RuntimeTabs: tabs}).
		WithRuntimeActionExecutor(func(action RuntimeAction) (RuntimeActionExecutionResult, error) {
			called = true
			if action.Kind != "worker-queue-claim-artifact" || action.ArtifactPath != "/tmp/ouro/claim.json" {
				t.Fatalf("unexpected executed action: %#v", action)
			}
			return RuntimeActionExecutionResult{
				Status: "dispatched",
				Path:   "/tmp/ouro/dispatch-execution.json",
				RuntimeRows: []string{
					"worker queue dispatch executions: total=1 dispatched=1 skipped=0",
					"worker queue dispatch execution artifact: /tmp/ouro/dispatch-execution.json status=dispatched mode=execute transport=metadata-only claim=/tmp/ouro/claim.json worker=remote-web-1 execution=tool-web",
					"queue dispatch: remote-web-1/tool-web web.discovery dispatched metadata-only",
				},
			}, nil
		})

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if cmd != nil {
		t.Fatalf("first x should only arm worker queue claim confirmation")
	}
	if called {
		t.Fatalf("worker queue claim executor must not run on first x")
	}
	next, ok := updated.(Model)
	if !ok {
		t.Fatalf("expected Model, got %T", updated)
	}
	action, ok := next.graphStack.RuntimeTabs.ActiveRuntimeAction()
	if !ok || action.ExecutionStatus != "confirm-required" {
		t.Fatalf("expected confirm-required worker queue claim action, ok=%v action=%#v", ok, action)
	}
	view := next.graphStack.RuntimeTabs.Render(140, 14)
	for _, want := range []string{
		"action: worker queue claim artifact selected /tmp/ouro/claim.json",
		"execution status: confirm-required",
		"execution confirm: press x again",
		"action mode: metadata-only",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("confirmation view missing %q:\n%s", want, view)
		}
	}

	updated, cmd = next.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if cmd == nil {
		t.Fatalf("second x should return worker queue claim execution command")
	}
	if called {
		t.Fatalf("worker queue claim executor should run only when returned command is executed")
	}
	msg := cmd()
	if !called {
		t.Fatalf("expected worker queue claim executor to run when command executes")
	}
	updated, cmd = updated.Update(msg)
	if cmd != nil {
		t.Fatalf("expected worker queue claim execution result update to be synchronous")
	}
	next, ok = updated.(Model)
	if !ok {
		t.Fatalf("expected Model, got %T", updated)
	}
	action, ok = next.graphStack.RuntimeTabs.ActiveRuntimeAction()
	if !ok || action.ExecutionStatus != "dispatched" || action.ExecutionPath != "/tmp/ouro/dispatch-execution.json" {
		t.Fatalf("expected dispatched worker queue claim execution metadata, ok=%v action=%#v", ok, action)
	}
	selected, ok := next.graphStack.RuntimeTabs.ActiveSelectedRow()
	if !ok || !strings.HasPrefix(strings.TrimSpace(selected), "worker queue dispatch execution artifact: /tmp/ouro/dispatch-execution.json") {
		t.Fatalf("expected newly written worker queue dispatch execution artifact to be selected, ok=%v row=%q", ok, selected)
	}
}

func TestRuntimeWorkerQueueClaimArtifactUsesEncodedReferenceFields(t *testing.T) {
	artifactPath := "/tmp/ouro/claim path status=failed mode=execute worker=evil.json"
	workerID := "remote web status=failed mode=execute"
	runID := "run queue execution=evil-execution"
	executionID := "tool web status=failed"
	row := "worker queue claim artifact: " + artifactPath +
		" artifact_b64=" + base64.RawURLEncoding.EncodeToString([]byte(artifactPath)) +
		" status=claimed mode=metadata-only" +
		" worker=" + workerID +
		" worker_b64=" + base64.RawURLEncoding.EncodeToString([]byte(workerID)) +
		" run=" + runID +
		" run_b64=" + base64.RawURLEncoding.EncodeToString([]byte(runID)) +
		" execution=" + executionID +
		" execution_b64=" + base64.RawURLEncoding.EncodeToString([]byte(executionID))

	action := runtimeWorkerQueueClaimArtifactActionForRow(row)
	if action.ArtifactPath != artifactPath || action.WorkerID != workerID || action.RunID != runID || action.ExecutionID != executionID || action.Name != executionID {
		t.Fatalf("expected encoded worker queue claim references to be decoded, got %#v", action)
	}
	if action.State != "claimed" || action.ExecutionMode != "metadata-only" {
		t.Fatalf("encoded worker queue claim references must not retarget action metadata: %#v", action)
	}
}

func TestRuntimeWorkerTransportRequestArtifactUsesEncodedReferenceFields(t *testing.T) {
	requestPath := "/tmp/ouro/request  path\tstatus=failed transport=http-json  worker=evil.json"
	dispatchPath := "/tmp/ouro/dispatch  path\tclaim=/tmp/ouro/evil-claim.json"
	claimPath := "/tmp/ouro/claim  path\tworker=evil"
	workerID := "remote  web\tstatus=failed transport=http-json"
	executionID := "tool  web\tstatus=failed"
	row := "worker transport request: " + runtimeCompactRowTextValue(requestPath) +
		" request_b64=" + base64.RawURLEncoding.EncodeToString([]byte(requestPath)) +
		" status=pending transport=file-outbox" +
		" dispatch=" + runtimeCompactRowTextValue(dispatchPath) +
		" dispatch_b64=" + base64.RawURLEncoding.EncodeToString([]byte(dispatchPath)) +
		" claim=" + runtimeCompactRowTextValue(claimPath) +
		" claim_b64=" + base64.RawURLEncoding.EncodeToString([]byte(claimPath)) +
		" worker=" + runtimeCompactRowTextValue(workerID) +
		" worker_b64=" + base64.RawURLEncoding.EncodeToString([]byte(workerID)) +
		" execution=" + runtimeCompactRowTextValue(executionID) +
		" execution_b64=" + base64.RawURLEncoding.EncodeToString([]byte(executionID))

	action := runtimeWorkerTransportRequestArtifactActionForRow(row)
	if action.ArtifactPath != requestPath || action.DispatchPath != dispatchPath || action.Source != claimPath || action.Name != workerID || action.ExecutionID != executionID {
		t.Fatalf("expected encoded worker transport request references to decode, got %#v", action)
	}
	if action.State != "pending" || action.ExecutionMode != "file-outbox" {
		t.Fatalf("encoded worker transport request references must not retarget action metadata: %#v", action)
	}
}

func TestRuntimeWorkerTransportRequestArtifactKeepsLegacyReferenceFields(t *testing.T) {
	row := "worker transport request: /tmp/ouro/request.json status=pending transport=file-outbox dispatch=/tmp/ouro/dispatch.json claim=/tmp/ouro/claim.json worker=remote-web-1 execution=tool-web"

	action := runtimeWorkerTransportRequestArtifactActionForRow(row)
	if action.ArtifactPath != "/tmp/ouro/request.json" || action.DispatchPath != "/tmp/ouro/dispatch.json" || action.Source != "/tmp/ouro/claim.json" || action.Name != "remote-web-1" || action.ExecutionID != "tool-web" {
		t.Fatalf("expected legacy worker transport request references to parse, got %#v", action)
	}
	if action.State != "pending" || action.ExecutionMode != "file-outbox" {
		t.Fatalf("expected legacy worker transport request metadata to parse, got %#v", action)
	}
}

func TestRuntimeWorkerQueueDispatchExecutionArtifactUsesEncodedReferenceFields(t *testing.T) {
	artifactPath := "/tmp/ouro/dispatch  execution\tstatus=failed mode=metadata-only claim=/tmp/ouro/evil-claim.json"
	claimPath := "/tmp/ouro/claim  path\tworker=evil"
	workerID := "remote  web\tstatus=failed mode=metadata-only"
	executionID := "tool  web\tstatus=failed"
	row := "worker queue dispatch execution artifact: " + runtimeCompactRowTextValue(artifactPath) +
		" artifact_b64=" + base64.RawURLEncoding.EncodeToString([]byte(artifactPath)) +
		" status=dispatched mode=execute transport=file-outbox" +
		" claim=" + runtimeCompactRowTextValue(claimPath) +
		" claim_b64=" + base64.RawURLEncoding.EncodeToString([]byte(claimPath)) +
		" worker=" + runtimeCompactRowTextValue(workerID) +
		" worker_b64=" + base64.RawURLEncoding.EncodeToString([]byte(workerID)) +
		" execution=" + runtimeCompactRowTextValue(executionID) +
		" execution_b64=" + base64.RawURLEncoding.EncodeToString([]byte(executionID))

	action := runtimeWorkerQueueDispatchExecutionArtifactActionForRow(row)
	if action.ArtifactPath != artifactPath || action.DispatchPath != claimPath || action.WorkerID != workerID || action.Name != executionID || action.ExecutionID != executionID {
		t.Fatalf("expected encoded worker queue dispatch execution references to decode, got %#v", action)
	}
	if action.State != "dispatched" || action.ExecutionMode != "execute" {
		t.Fatalf("encoded worker queue dispatch execution references must not retarget action metadata: %#v", action)
	}
}

func TestRuntimeWorkerQueueDispatchResultArtifactUsesEncodedReferenceFields(t *testing.T) {
	artifactPath := "/tmp/ouro/result  path\tstatus=failed mode=execute dispatch=/tmp/ouro/evil-dispatch.json"
	dispatchPath := "/tmp/ouro/dispatch  execution\tworker=evil"
	workerID := "remote  web\tstatus=failed mode=execute"
	executionID := "tool  web\tstatus=failed"
	row := "worker queue dispatch result artifact: " + runtimeCompactRowTextValue(artifactPath) +
		" artifact_b64=" + base64.RawURLEncoding.EncodeToString([]byte(artifactPath)) +
		" status=completed mode=metadata-only" +
		" dispatch=" + runtimeCompactRowTextValue(dispatchPath) +
		" dispatch_b64=" + base64.RawURLEncoding.EncodeToString([]byte(dispatchPath)) +
		" worker=" + runtimeCompactRowTextValue(workerID) +
		" worker_b64=" + base64.RawURLEncoding.EncodeToString([]byte(workerID)) +
		" execution=" + runtimeCompactRowTextValue(executionID) +
		" execution_b64=" + base64.RawURLEncoding.EncodeToString([]byte(executionID))

	action := runtimeWorkerQueueDispatchResultArtifactActionForRow(row)
	if action.ArtifactPath != artifactPath || action.DispatchPath != dispatchPath || action.Source != workerID || action.WorkerID != workerID || action.Name != executionID || action.ExecutionID != executionID {
		t.Fatalf("expected encoded worker queue dispatch result references to decode, got %#v", action)
	}
	if action.State != "completed" || action.ExecutionMode != "metadata-only" {
		t.Fatalf("encoded worker queue dispatch result references must not retarget action metadata: %#v", action)
	}
}

func TestModelUpdateCancelsWorkerQueueClaimArtifactOnlyAfterSecondConfirm(t *testing.T) {
	tabs := DefaultRuntimeTabs().
		Select(RuntimeTabAgentRuntime).
		WithRows(RuntimeTabAgentRuntime, []string{
			"agents: 0",
			"worker queue claim artifact: /tmp/ouro/claim.json status=claimed mode=metadata-only worker=remote-web-1 run=run-cancel-1 execution=tool-web",
		}).
		SelectRow(1)
	called := false
	model := NewModel().
		WithGraphStack(GraphStack{RuntimeTabs: tabs}).
		WithRuntimeActionExecutor(func(action RuntimeAction) (RuntimeActionExecutionResult, error) {
			called = true
			if action.Kind != "worker-queue-claim-artifact" || action.ArtifactPath != "/tmp/ouro/claim.json" || action.ControlAction != "cancel" {
				t.Fatalf("unexpected canceled action: %#v", action)
			}
			return RuntimeActionExecutionResult{
				Status: "canceled",
				Path:   "/tmp/ouro/claim.json",
				Output: "queue claim canceled",
				RuntimeRows: []string{
					"worker queue claims: total=1 claimed=0 canceled=1",
					"worker queue claim artifact: /tmp/ouro/claim.json status=canceled mode=metadata-only worker=remote-web-1 run=run-cancel-1 execution=tool-web",
				},
			}, nil
		})

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'C'}})
	if cmd != nil {
		t.Fatalf("first C should only arm worker queue claim cancel confirmation")
	}
	if called {
		t.Fatalf("worker queue claim cancel executor must not run on first C")
	}
	next, ok := updated.(Model)
	if !ok {
		t.Fatalf("expected Model, got %T", updated)
	}
	action, ok := next.graphStack.RuntimeTabs.ActiveRuntimeAction()
	if !ok || action.ExecutionStatus != "confirm-required" || action.ControlAction != "cancel" {
		t.Fatalf("expected confirm-required worker queue claim cancel action, ok=%v action=%#v", ok, action)
	}
	view := next.graphStack.RuntimeTabs.Render(140, 14)
	for _, want := range []string{
		"action: worker queue claim artifact selected /tmp/ouro/claim.json",
		"control action: cancel",
		"execution status: confirm-required",
		"execution confirm: press C again",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("cancel confirmation view missing %q:\n%s", want, view)
		}
	}

	updated, cmd = next.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'C'}})
	if cmd == nil {
		t.Fatalf("second C should return worker queue claim cancel command")
	}
	if called {
		t.Fatalf("worker queue claim cancel executor should run only when returned command is executed")
	}
	msg := cmd()
	if !called {
		t.Fatalf("expected worker queue claim cancel executor to run when command executes")
	}
	updated, cmd = updated.Update(msg)
	if cmd != nil {
		t.Fatalf("expected worker queue claim cancel result update to be synchronous")
	}
	next, ok = updated.(Model)
	if !ok {
		t.Fatalf("expected Model, got %T", updated)
	}
	action, ok = next.graphStack.RuntimeTabs.ActiveRuntimeAction()
	if !ok || action.ExecutionStatus != "canceled" || action.ExecutionPath != "/tmp/ouro/claim.json" || action.Output != "queue claim canceled" || action.ControlAction != "cancel" {
		t.Fatalf("expected canceled worker queue claim metadata, ok=%v action=%#v", ok, action)
	}
	selected, ok := next.graphStack.RuntimeTabs.ActiveSelectedRow()
	if !ok || !strings.HasPrefix(strings.TrimSpace(selected), "worker queue claim artifact: /tmp/ouro/claim.json status=canceled") {
		t.Fatalf("expected canceled worker queue claim artifact to be selected, ok=%v row=%q", ok, selected)
	}
}

func TestModelUpdateExpiresWorkerQueueClaimArtifactOnlyAfterSecondConfirm(t *testing.T) {
	tabs := DefaultRuntimeTabs().
		Select(RuntimeTabAgentRuntime).
		WithRows(RuntimeTabAgentRuntime, []string{
			"agents: 0",
			"worker queue claim artifact: /tmp/ouro/claim.json status=claimed mode=metadata-only worker=remote-web-1 run=run-expire-1 execution=tool-web",
		}).
		SelectRow(1)
	called := false
	model := NewModel().
		WithGraphStack(GraphStack{RuntimeTabs: tabs}).
		WithRuntimeActionExecutor(func(action RuntimeAction) (RuntimeActionExecutionResult, error) {
			called = true
			if action.Kind != "worker-queue-claim-artifact" || action.ArtifactPath != "/tmp/ouro/claim.json" || action.ControlAction != "expire" {
				t.Fatalf("unexpected expired action: %#v", action)
			}
			return RuntimeActionExecutionResult{
				Status: "expired",
				Path:   "/tmp/ouro/claim.json",
				Output: "queue claim expired",
				RuntimeRows: []string{
					"worker queue claims: total=1 claimed=0 expired=1",
					"worker queue claim artifact: /tmp/ouro/claim.json status=expired mode=metadata-only worker=remote-web-1 run=run-expire-1 execution=tool-web",
				},
			}, nil
		})

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'E'}})
	if cmd != nil {
		t.Fatalf("first E should only arm worker queue claim expire confirmation")
	}
	if called {
		t.Fatalf("worker queue claim expire executor must not run on first E")
	}
	next, ok := updated.(Model)
	if !ok {
		t.Fatalf("expected Model, got %T", updated)
	}
	action, ok := next.graphStack.RuntimeTabs.ActiveRuntimeAction()
	if !ok || action.ExecutionStatus != "confirm-required" || action.ControlAction != "expire" {
		t.Fatalf("expected confirm-required worker queue claim expire action, ok=%v action=%#v", ok, action)
	}
	view := next.graphStack.RuntimeTabs.Render(140, 14)
	for _, want := range []string{
		"action: worker queue claim artifact selected /tmp/ouro/claim.json",
		"control action: expire",
		"execution status: confirm-required",
		"execution confirm: press E again",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expire confirmation view missing %q:\n%s", want, view)
		}
	}

	updated, cmd = next.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'E'}})
	if cmd == nil {
		t.Fatalf("second E should return worker queue claim expire command")
	}
	if called {
		t.Fatalf("worker queue claim expire executor should run only when returned command is executed")
	}
	msg := cmd()
	if !called {
		t.Fatalf("expected worker queue claim expire executor to run when command executes")
	}
	updated, cmd = updated.Update(msg)
	if cmd != nil {
		t.Fatalf("expected worker queue claim expire result update to be synchronous")
	}
	next, ok = updated.(Model)
	if !ok {
		t.Fatalf("expected Model, got %T", updated)
	}
	action, ok = next.graphStack.RuntimeTabs.ActiveRuntimeAction()
	if !ok || action.ExecutionStatus != "expired" || action.ExecutionPath != "/tmp/ouro/claim.json" || action.Output != "queue claim expired" || action.ControlAction != "expire" {
		t.Fatalf("expected expired worker queue claim metadata, ok=%v action=%#v", ok, action)
	}
	selected, ok := next.graphStack.RuntimeTabs.ActiveSelectedRow()
	if !ok || !strings.HasPrefix(strings.TrimSpace(selected), "worker queue claim artifact: /tmp/ouro/claim.json status=expired") {
		t.Fatalf("expected expired worker queue claim artifact to be selected, ok=%v row=%q", ok, selected)
	}
}

func TestModelUpdateRenewsWorkerQueueClaimArtifactOnlyAfterSecondConfirm(t *testing.T) {
	tabs := DefaultRuntimeTabs().
		Select(RuntimeTabAgentRuntime).
		WithRows(RuntimeTabAgentRuntime, []string{
			"agents: 0",
			"worker queue claim artifact: /tmp/ouro/claim.json status=claimed mode=metadata-only worker=remote-web-1 run=run-renew-1 execution=tool-web",
		}).
		SelectRow(1)
	called := false
	model := NewModel().
		WithGraphStack(GraphStack{RuntimeTabs: tabs}).
		WithRuntimeActionExecutor(func(action RuntimeAction) (RuntimeActionExecutionResult, error) {
			called = true
			if action.Kind != "worker-queue-claim-artifact" || action.ArtifactPath != "/tmp/ouro/claim.json" || action.ControlAction != "renew" {
				t.Fatalf("unexpected renewed action: %#v", action)
			}
			return RuntimeActionExecutionResult{
				Status: "renewed",
				Path:   "/tmp/ouro/claim.json",
				Output: "queue claim renewed",
				RuntimeRows: []string{
					"worker queue claims: total=1 claimed=1 canceled=0 expired=0 dispatched=0 completed=0 failed=0",
					"worker queue claim renewed: at=2026-05-30T22:12:00Z by=tui reason=TUI renew requested for queue claim /tmp/ouro/claim.json lease_seconds=900 expires=2026-05-30T22:27:00Z",
					"worker queue claim artifact: /tmp/ouro/claim.json status=claimed mode=metadata-only worker=remote-web-1 run=run-renew-1 execution=tool-web",
				},
			}, nil
		})

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'L'}})
	if cmd != nil {
		t.Fatalf("first L should only arm worker queue claim renew confirmation")
	}
	if called {
		t.Fatalf("worker queue claim renew executor must not run on first L")
	}
	next, ok := updated.(Model)
	if !ok {
		t.Fatalf("expected Model, got %T", updated)
	}
	action, ok := next.graphStack.RuntimeTabs.ActiveRuntimeAction()
	if !ok || action.ExecutionStatus != "confirm-required" || action.ControlAction != "renew" {
		t.Fatalf("expected confirm-required worker queue claim renew action, ok=%v action=%#v", ok, action)
	}
	view := next.graphStack.RuntimeTabs.Render(140, 14)
	for _, want := range []string{
		"action: worker queue claim artifact selected /tmp/ouro/claim.json",
		"control action: renew",
		"execution status: confirm-required",
		"execution confirm: press L again",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("renew confirmation view missing %q:\n%s", want, view)
		}
	}

	updated, cmd = next.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'L'}})
	if cmd == nil {
		t.Fatalf("second L should return worker queue claim renew command")
	}
	if called {
		t.Fatalf("worker queue claim renew executor should run only when returned command is executed")
	}
	msg := cmd()
	if !called {
		t.Fatalf("expected worker queue claim renew executor to run when command executes")
	}
	updated, cmd = updated.Update(msg)
	if cmd != nil {
		t.Fatalf("expected worker queue claim renew result update to be synchronous")
	}
	next, ok = updated.(Model)
	if !ok {
		t.Fatalf("expected Model, got %T", updated)
	}
	action, ok = next.graphStack.RuntimeTabs.ActiveRuntimeAction()
	if !ok || action.ExecutionStatus != "renewed" || action.ExecutionPath != "/tmp/ouro/claim.json" || action.Output != "queue claim renewed" || action.ControlAction != "renew" {
		t.Fatalf("expected renewed worker queue claim metadata, ok=%v action=%#v", ok, action)
	}
	selected, ok := next.graphStack.RuntimeTabs.ActiveSelectedRow()
	if !ok || !strings.HasPrefix(strings.TrimSpace(selected), "worker queue claim artifact: /tmp/ouro/claim.json status=claimed") {
		t.Fatalf("expected renewed worker queue claim artifact to remain selected, ok=%v row=%q", ok, selected)
	}
}

func TestModelUpdateRequestsWorkerTransportDaemonStopOnlyAfterSecondConfirm(t *testing.T) {
	tabs := DefaultRuntimeTabs().
		Select(RuntimeTabAgentRuntime).
		WithRows(RuntimeTabAgentRuntime, []string{
			"agents: 0",
			"worker transport daemon artifact: /tmp/ouro/daemon-run.json status=completed worker=remote-web-1 source=request-command",
		}).
		SelectRow(1)
	called := false
	model := NewModel().
		WithGraphStack(GraphStack{RuntimeTabs: tabs}).
		WithRuntimeActionExecutor(func(action RuntimeAction) (RuntimeActionExecutionResult, error) {
			called = true
			if action.Kind != "worker-transport-daemon-artifact" || action.ArtifactPath != "/tmp/ouro/daemon-run.json" || action.Name != "remote-web-1" {
				t.Fatalf("unexpected daemon control action: %#v", action)
			}
			return RuntimeActionExecutionResult{
				Status: "pending",
				Path:   "/tmp/ouro/daemon-control.json",
				Output: "stop requested",
				RuntimeRows: []string{
					"worker transport daemon controls: total=1 pending=1 stop=1 restart=0 applied=0",
					"worker transport daemon control artifact: /tmp/ouro/daemon-control.json status=pending worker=remote-web-1 action=stop mode=metadata-only",
					"transport daemon control: remote-web-1 stop pending",
				},
			}, nil
		})

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if cmd != nil {
		t.Fatalf("first x should only arm daemon stop confirmation")
	}
	if called {
		t.Fatalf("daemon control executor must not run on first x")
	}
	next, ok := updated.(Model)
	if !ok {
		t.Fatalf("expected Model, got %T", updated)
	}
	action, ok := next.graphStack.RuntimeTabs.ActiveRuntimeAction()
	if !ok || action.ExecutionStatus != "confirm-required" {
		t.Fatalf("expected confirm-required daemon control action, ok=%v action=%#v", ok, action)
	}
	view := next.graphStack.RuntimeTabs.Render(150, 16)
	for _, want := range []string{
		"action: worker transport daemon artifact selected /tmp/ouro/daemon-run.json",
		"execution status: confirm-required",
		"execution confirm: press x again",
		"action mode: metadata-only",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("confirmation view missing %q:\n%s", want, view)
		}
	}

	updated, cmd = next.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if cmd == nil {
		t.Fatalf("second x should return daemon control command")
	}
	if called {
		t.Fatalf("daemon control executor should run only when returned command is executed")
	}
	updated, cmd = updated.Update(cmd())
	if cmd != nil {
		t.Fatalf("expected daemon control result update to be synchronous")
	}
	next, ok = updated.(Model)
	if !ok {
		t.Fatalf("expected Model, got %T", updated)
	}
	action, ok = next.graphStack.RuntimeTabs.ActiveRuntimeAction()
	if !ok || action.ExecutionStatus != "pending" || action.ExecutionPath != "/tmp/ouro/daemon-control.json" || action.Output != "stop requested" {
		t.Fatalf("expected pending daemon control metadata, ok=%v action=%#v", ok, action)
	}
	selected, ok := next.graphStack.RuntimeTabs.ActiveSelectedRow()
	if !ok || !strings.HasPrefix(strings.TrimSpace(selected), "worker transport daemon control artifact: /tmp/ouro/daemon-control.json") {
		t.Fatalf("expected newly written daemon control artifact to be selected, ok=%v row=%q", ok, selected)
	}
}

func TestModelUpdatePreservesFailedRetryDispatchExecutionMetadata(t *testing.T) {
	tabs := DefaultRuntimeTabs().
		Select(RuntimeTabToolchainHealth).
		WithRows(RuntimeTabToolchainHealth, []string{
			"toolchain: ready",
			"retry dispatch artifact: /tmp/ouro/retry-dispatch.json status=dry-run mode=metadata-only ready=1 skipped=0",
		}).
		SelectRow(1)
	model := NewModel().
		WithGraphStack(GraphStack{RuntimeTabs: tabs}).
		WithRuntimeActionExecutor(func(action RuntimeAction) (RuntimeActionExecutionResult, error) {
			if action.Kind != "install-retry-dispatch-artifact" || action.ArtifactPath != "/tmp/ouro/retry-dispatch.json" {
				t.Fatalf("unexpected executed retry action: %#v", action)
			}
			return RuntimeActionExecutionResult{
				Status: "failed",
				Path:   "/tmp/ouro/retry-execution-failed.json",
				Output: "partial retry output",
				RuntimeRows: []string{
					"install retry execution artifacts: total=1",
					"retry execution artifact: /tmp/ouro/retry-execution-failed.json status=failed mode=execute retry_dispatch=/tmp/ouro/retry-dispatch.json results=1 skipped=0",
					"retry execution result web-api-pack/missing-httpx: failed output=partial retry output error=exit status 9 artifact=/tmp/ouro/retry-execution-failed.json retry_dispatch=/tmp/ouro/retry-dispatch.json source_execution=/tmp/ouro/source-execution.json dispatch=/tmp/ouro/dispatch.json command=sh -c echo retry; exit 9",
				},
			}, assertErr("execute retry install command for web-api-pack/missing-httpx: exit status 9")
		})

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if cmd != nil {
		t.Fatalf("first x should only arm retry execution confirmation")
	}
	updated, cmd = updated.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if cmd == nil {
		t.Fatalf("second x should return retry execution command")
	}
	updated, cmd = updated.Update(cmd())
	if cmd != nil {
		t.Fatalf("expected failed retry execution result update to be synchronous")
	}
	next, ok := updated.(Model)
	if !ok {
		t.Fatalf("expected Model, got %T", updated)
	}
	action, ok := next.graphStack.RuntimeTabs.ActiveRuntimeAction()
	if !ok {
		t.Fatalf("expected active runtime action")
	}
	if action.ExecutionStatus != "failed" || action.ExecutionPath != "/tmp/ouro/retry-execution-failed.json" || action.Output != "partial retry output" || !strings.Contains(action.ExecutionError, "exit status 9") {
		t.Fatalf("expected failed retry execution metadata, got %#v", action)
	}
	selected, ok := next.graphStack.RuntimeTabs.ActiveSelectedRow()
	if !ok || !strings.HasPrefix(strings.TrimSpace(selected), "retry execution artifact: /tmp/ouro/retry-execution-failed.json") {
		t.Fatalf("expected failed retry execution artifact to be selected, ok=%v row=%q", ok, selected)
	}
	view := next.graphStack.RuntimeTabs.Render(170, 22)
	for _, want := range []string{
		"execution status: failed",
		"execution path: /tmp/ouro/retry-execution-failed.json",
		"execution error: execute retry install command for web-api-pack/missing-httpx: exit status 9",
		"action output: partial retry output",
		"action mode: metadata-only",
		"install retry execution artifacts: total=1",
		"retry execution artifact: /tmp/ouro/retry-execution-failed.json",
		"retry execution result web-api-pack/missing-httpx: failed output=partial retry output error=exit status 9",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected failed retry execution view %q:\n%s", want, view)
		}
	}

	updated, cmd = next.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	if cmd != nil {
		t.Fatalf("expected failed retry execution artifact detail update to be synchronous")
	}
	detailed, ok := updated.(Model)
	if !ok {
		t.Fatalf("expected Model, got %T", updated)
	}
	if detailed.graphStack.RuntimeAction.Kind != "install-retry-execution-artifact" || detailed.graphStack.RuntimeAction.ArtifactPath != "/tmp/ouro/retry-execution-failed.json" {
		t.Fatalf("expected selected failed retry execution artifact details, got %#v", detailed.graphStack.RuntimeAction)
	}
}

func TestModelUpdatePreservesFailedDispatchExecutionMetadata(t *testing.T) {
	tabs := DefaultRuntimeTabs().
		Select(RuntimeTabToolchainHealth).
		WithRows(RuntimeTabToolchainHealth, []string{
			"toolchain: ready",
			"dispatch artifact: /tmp/ouro/dispatch.json status=dry-run mode=metadata-only ready=1 skipped=0",
		}).
		SelectRow(1)
	model := NewModel().
		WithGraphStack(GraphStack{RuntimeTabs: tabs}).
		WithRuntimeActionExecutor(func(action RuntimeAction) (RuntimeActionExecutionResult, error) {
			return RuntimeActionExecutionResult{
				Status: "failed",
				Path:   "/tmp/ouro/execution-failed.json",
				Output: "installer failed",
				RuntimeRows: []string{
					"install execution artifacts: total=1",
					"execution result web-api-pack/missing-httpx: failed",
				},
			}, assertErr("exit status 7")
		})

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if cmd != nil {
		t.Fatalf("first x should only arm execution confirmation")
	}
	updated, cmd = updated.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if cmd == nil {
		t.Fatalf("second x should return execution command")
	}
	updated, cmd = updated.Update(cmd())
	if cmd != nil {
		t.Fatalf("expected failed execution result update to be synchronous")
	}
	next, ok := updated.(Model)
	if !ok {
		t.Fatalf("expected Model, got %T", updated)
	}
	action, ok := next.graphStack.RuntimeTabs.ActiveRuntimeAction()
	if !ok {
		t.Fatalf("expected active runtime action")
	}
	if action.ExecutionStatus != "failed" || action.ExecutionPath != "/tmp/ouro/execution-failed.json" || action.Output != "installer failed" || !strings.Contains(action.ExecutionError, "exit status 7") {
		t.Fatalf("expected failed execution metadata, got %#v", action)
	}
	view := next.graphStack.RuntimeTabs.Render(140, 18)
	for _, want := range []string{
		"execution status: failed",
		"execution path: /tmp/ouro/execution-failed.json",
		"execution error: exit status 7",
		"action output: installer failed",
		"install execution artifacts: total=1",
		"execution result web-api-pack/missing-httpx: failed",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected failed execution view %q:\n%s", want, view)
		}
	}
}

type assertErr string

func (e assertErr) Error() string {
	return string(e)
}

func TestModelUpdateActivatesRuntimeTabSelection(t *testing.T) {
	tabs := DefaultRuntimeTabs().
		Select(RuntimeTabToolchainHealth).
		WithRows(RuntimeTabToolchainHealth, []string{
			"toolchain: missing",
			"install httpx: missing required=true command=ouro-toolchain install httpx",
		}).
		SelectRow(1)
	model := NewModel().WithGraphStack(GraphStack{RuntimeTabs: tabs})

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil {
		t.Fatalf("expected remediation action update to be synchronous")
	}
	next, ok := updated.(Model)
	if !ok {
		t.Fatalf("expected Model, got %T", updated)
	}
	action, ok := next.graphStack.RuntimeTabs.ActiveAction()
	if !ok || !strings.Contains(action, "remediation selected install httpx") {
		t.Fatalf("expected enter key to activate remediation row, ok=%v action=%q", ok, action)
	}
}

func TestModelUpdateRequestsWorkerTransportDaemonRestartOnlyAfterSecondConfirm(t *testing.T) {
	tabs := DefaultRuntimeTabs().
		Select(RuntimeTabAgentRuntime).
		WithRows(RuntimeTabAgentRuntime, []string{
			"agents: 0",
			"worker transport daemon artifact: /tmp/ouro/daemon-run.json status=completed worker=remote-web-1 source=request-command",
		}).
		SelectRow(1)
	called := false
	model := NewModel().
		WithGraphStack(GraphStack{RuntimeTabs: tabs}).
		WithRuntimeActionExecutor(func(action RuntimeAction) (RuntimeActionExecutionResult, error) {
			called = true
			if action.Kind != "worker-transport-daemon-artifact" || action.ArtifactPath != "/tmp/ouro/daemon-run.json" || action.Name != "remote-web-1" || action.ControlAction != "restart" {
				t.Fatalf("unexpected daemon restart action: %#v", action)
			}
			return RuntimeActionExecutionResult{
				Status: "pending",
				Path:   "/tmp/ouro/daemon-control.json",
				Output: "restart requested",
				RuntimeRows: []string{
					"worker transport daemon controls: total=1 pending=1 stop=0 restart=1 applied=0",
					"worker transport daemon control artifact: /tmp/ouro/daemon-control.json status=pending worker=remote-web-1 action=restart mode=metadata-only",
					"transport daemon control: remote-web-1 restart pending",
				},
			}, nil
		})

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'R'}})
	if cmd != nil {
		t.Fatalf("first restart keypress should only arm daemon restart confirmation")
	}
	if called {
		t.Fatalf("daemon restart executor must not run on first R")
	}
	next, ok := updated.(Model)
	if !ok {
		t.Fatalf("expected Model, got %T", updated)
	}
	action, ok := next.graphStack.RuntimeTabs.ActiveRuntimeAction()
	if !ok || action.ExecutionStatus != "confirm-required" || action.ControlAction != "restart" {
		t.Fatalf("expected confirm-required daemon restart action, ok=%v action=%#v", ok, action)
	}
	view := next.graphStack.RuntimeTabs.Render(150, 16)
	for _, want := range []string{
		"action: worker transport daemon artifact selected /tmp/ouro/daemon-run.json",
		"control action: restart",
		"execution status: confirm-required",
		"execution confirm: press R again",
		"action mode: metadata-only",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("restart confirmation view missing %q:\n%s", want, view)
		}
	}

	updated, cmd = next.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'R'}})
	if cmd == nil {
		t.Fatalf("second R should return daemon restart control command")
	}
	if called {
		t.Fatalf("daemon restart executor should run only when returned command is executed")
	}
	updated, cmd = updated.Update(cmd())
	if cmd != nil {
		t.Fatalf("expected daemon restart result update to be synchronous")
	}
	next, ok = updated.(Model)
	if !ok {
		t.Fatalf("expected Model, got %T", updated)
	}
	action, ok = next.graphStack.RuntimeTabs.ActiveRuntimeAction()
	if !ok || action.ExecutionStatus != "pending" || action.ExecutionPath != "/tmp/ouro/daemon-control.json" || action.Output != "restart requested" || action.ControlAction != "restart" {
		t.Fatalf("expected pending daemon restart metadata, ok=%v action=%#v", ok, action)
	}
	selected, ok := next.graphStack.RuntimeTabs.ActiveSelectedRow()
	if !ok || !strings.HasPrefix(strings.TrimSpace(selected), "worker transport daemon control artifact: /tmp/ouro/daemon-control.json") {
		t.Fatalf("expected newly written daemon restart control artifact to be selected, ok=%v row=%q", ok, selected)
	}
}

func TestModelUpdateShowsSelectedRuntimeActionDetails(t *testing.T) {
	tabs := DefaultRuntimeTabs().
		Select(RuntimeTabToolchainHealth).
		WithRows(RuntimeTabToolchainHealth, []string{
			"toolchain: missing",
			"install httpx: missing required=true command=ouro-toolchain install httpx hint=install bundled httpx pack",
		}).
		SelectRow(1)
	model := NewModel().WithGraphStack(GraphStack{RuntimeTabs: tabs})

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	if cmd != nil {
		t.Fatalf("expected runtime action details update to be synchronous")
	}
	next, ok := updated.(Model)
	if !ok {
		t.Fatalf("expected Model, got %T", updated)
	}
	if next.graphStack.Focus != GraphFocusRuntimeAction {
		t.Fatalf("expected runtime action focus, got %s", next.graphStack.Focus)
	}
	view := next.graphStack.Render(96, 18)
	for _, want := range []string{
		"[Node Inspector]",
		"runtime action: install",
		"name: httpx",
		"command: ouro-toolchain install httpx",
		"hint: install bundled httpx pack",
		"mode: metadata-only",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("runtime action details missing %q:\n%s", want, view)
		}
	}
}

func TestModelUpdateShowsSelectedToolchainContractDetails(t *testing.T) {
	for _, tc := range []struct {
		name  string
		row   string
		wants []string
	}{
		{
			name: "summary",
			row:  "toolchain contract: status=ready mode=metadata-only manifests=3 tools=14 domain_packs=3 patterns=3 semantic_actions=7 errors=0",
			wants: []string{
				"runtime action: toolchain-contract",
				"state: ready",
				"execution mode: metadata-only",
				"output: contract check is read-only metadata",
				"ready: manifests=3 tools=14 domain_packs=3 patterns=3 semantic_actions=7",
				"failed: errors=0",
				"mode: read-only",
			},
		},
		{
			name: "error",
			row:  "toolchain contract error: domain capability web.auth_state_tracking is not provided by workspace manifests or built-in tools",
			wants: []string{
				"runtime action: toolchain-contract-error",
				"state: blocked",
				"execution mode: metadata-only",
				"reason: domain capability web.auth_state_tracking is not provided by workspace manifests or built-in tools",
				"output: contract validation error is read-only metadata",
				"mode: read-only",
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tabs := DefaultRuntimeTabs().
				Select(RuntimeTabToolchainHealth).
				WithRows(RuntimeTabToolchainHealth, []string{tc.row}).
				SelectRow(0)
			model := NewModel().WithGraphStack(GraphStack{RuntimeTabs: tabs})

			updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
			if cmd != nil {
				t.Fatalf("expected toolchain contract details update to be synchronous")
			}
			next, ok := updated.(Model)
			if !ok {
				t.Fatalf("expected Model, got %T", updated)
			}
			if next.graphStack.Focus != GraphFocusRuntimeAction {
				t.Fatalf("expected runtime action focus, got %s", next.graphStack.Focus)
			}
			view := next.graphStack.Render(180, 36)
			for _, want := range tc.wants {
				if !strings.Contains(view, want) {
					t.Fatalf("toolchain contract details missing %q:\n%s", want, view)
				}
			}
		})
	}
}

func TestModelUpdateShowsSelectedCompilePreviewSignalDetails(t *testing.T) {
	tabs := DefaultRuntimeTabs().
		Select(RuntimeTabToolchainHealth).
		WithRows(RuntimeTabToolchainHealth, []string{
			"toolchain: ready",
			"compile-preview signals: present path=/tmp/ouro/toolchain/compile-preview-signals.json mode=metadata-only max_plan_nodes=1 domain_priorities=1 frontier_scores=1 historical_stats=1",
			"compile-preview signal frontier: manifest=web-api-pack capability=web.discovery path=frontier-web target=https://app.example.test score=3.25",
			"compile-preview signal history: manifest=web-api-pack capability=web.discovery successes=3 failures=1",
		}).
		SelectRow(2)
	model := NewModel().WithGraphStack(GraphStack{RuntimeTabs: tabs})

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	if cmd != nil {
		t.Fatalf("expected compile-preview signal details update to be synchronous")
	}
	next, ok := updated.(Model)
	if !ok {
		t.Fatalf("expected Model, got %T", updated)
	}
	if next.graphStack.Focus != GraphFocusRuntimeAction {
		t.Fatalf("expected runtime action focus, got %s", next.graphStack.Focus)
	}
	view := next.graphStack.Render(140, 44)
	for _, want := range []string{
		"[Node Inspector]",
		"runtime action: compile-preview-signal-frontier",
		"pack: web-api-pack",
		"name: web.discovery",
		"source: frontier-web",
		"target: https://app.example.test",
		"score: 3.25",
		"mode: read-only",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("compile-preview signal details missing %q:\n%s", want, view)
		}
	}
}

func TestModelUpdateShowsSelectedCompilePreviewReviewAnnotationDetails(t *testing.T) {
	tabs := DefaultRuntimeTabs().
		Select(RuntimeTabToolchainHealth).
		WithRows(RuntimeTabToolchainHealth, []string{
			"toolchain: ready",
			"compile-preview review artifact: status=dry-run mode=metadata-only action=discover_assets nodes=1 warnings=0 decision=deferred reviewer=alice reviewed_at=2026-05-31T15:04:05Z reason=authorized boundary false positive path=/tmp/ouro/compile-preview-review.json",
		}).
		SelectRow(1)
	model := NewModel().WithGraphStack(GraphStack{RuntimeTabs: tabs})

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	if cmd != nil {
		t.Fatalf("expected compile-preview review details update to be synchronous")
	}
	next, ok := updated.(Model)
	if !ok {
		t.Fatalf("expected Model, got %T", updated)
	}
	view := next.graphStack.Render(140, 44)
	for _, want := range []string{
		"[Node Inspector]",
		"runtime action: compile-preview-review-artifact",
		"artifact path: /tmp/ouro/compile-preview-review.json",
		"state: dry-run",
		"execution mode: metadata-only",
		"action type: discover_assets",
		"nodes: 1",
		"warnings: 0",
		"decision: deferred",
		"reviewer: alice",
		"reviewed at: 2026-05-31T15:04:05Z",
		"reason: authorized boundary false positive",
		"mode: metadata-only",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("compile-preview review annotation details missing %q:\n%s", want, view)
		}
	}
}

func TestRuntimeCompilePreviewReviewArtifactIgnoresPathInReasonText(t *testing.T) {
	row := "compile-preview review artifact: status=dry-run mode=metadata-only action=discover_assets nodes=1 warnings=0 current=false decision=deferred reviewer=alice reason=operator note path=/tmp/ouro/other-review.json reviewed_at=2026-05-31T15:04:05Z path=/tmp/ouro/compile-preview-review.json"

	action := runtimeCompilePreviewReviewArtifactActionForRow(row)
	if action.ArtifactPath != "/tmp/ouro/compile-preview-review.json" {
		t.Fatalf("reason text must not retarget compile-preview review artifact path: %#v", action)
	}
}

func TestRuntimeCompilePreviewReviewArtifactUsesEncodedReasonText(t *testing.T) {
	reason := "operator note path=/tmp/ouro/other-review.json reviewed_at=2030-01-01T00:00:00Z with spaces"
	row := "compile-preview review artifact: status=dry-run mode=metadata-only action=discover_assets nodes=1 warnings=0 current=false decision=deferred reviewer=alice reason=" + reason + " reason_b64=" + base64.RawURLEncoding.EncodeToString([]byte(reason)) + " reviewed_at=2026-05-31T15:04:05Z path=/tmp/ouro/compile-preview-review.json"

	action := runtimeCompilePreviewReviewArtifactActionForRow(row)
	if action.ArtifactPath != "/tmp/ouro/compile-preview-review.json" {
		t.Fatalf("encoded reason text must not retarget compile-preview review artifact path: %#v", action)
	}
	if action.ReviewedAt != "2026-05-31T15:04:05Z" {
		t.Fatalf("encoded reason text must not retarget compile-preview review timestamp: %#v", action)
	}
	if action.Reason != reason {
		t.Fatalf("expected encoded reason to be decoded, got %#v", action.Reason)
	}
}

func TestRuntimeCompilePreviewReviewArtifactUsesEncodedReferenceFields(t *testing.T) {
	reviewer := "alice reviewer_b64=fake reviewed_at=2030-01-01T00:00:00Z path=/tmp/ouro/evil-review.json"
	reviewedAt := "2026-05-31T15:04:05Z"
	reason := "operator note path=/tmp/ouro/other-review.json reviewed_at=2031-01-01T00:00:00Z"
	artifactPath := "/tmp/ouro/compile preview reviews/review path=evil.json"
	row := "compile-preview review artifact: status=dry-run mode=metadata-only action=discover_assets nodes=1 warnings=0 current=false decision=deferred" +
		" reviewer=" + runtimeCompactRowTextValue(reviewer) +
		" reviewer_b64=" + base64.RawURLEncoding.EncodeToString([]byte(reviewer)) +
		" reason=" + runtimeCompactRowTextValue(reason) +
		" reason_b64=" + base64.RawURLEncoding.EncodeToString([]byte(reason)) +
		" reviewed_at=" + runtimeCompactRowTextValue(reviewedAt) +
		" reviewed_at_b64=" + base64.RawURLEncoding.EncodeToString([]byte(reviewedAt)) +
		" path=" + runtimeCompactRowTextValue(artifactPath) +
		" path_b64=" + base64.RawURLEncoding.EncodeToString([]byte(artifactPath))

	action := runtimeCompilePreviewReviewArtifactActionForRow(row)
	if action.State != "dry-run" || action.ExecutionMode != "metadata-only" || action.ActionType != "discover_assets" {
		t.Fatalf("encoded review references must not retarget identity metadata: %#v", action)
	}
	if action.Current != "false" || action.Decision != "deferred" {
		t.Fatalf("encoded review references must not retarget state fields: %#v", action)
	}
	if action.Reviewer != reviewer || action.ReviewedAt != reviewedAt || action.Reason != reason {
		t.Fatalf("expected encoded review text fields to decode, got %#v", action)
	}
	if action.ArtifactPath != artifactPath {
		t.Fatalf("expected encoded artifact path, got %#v", action)
	}
}

func TestRuntimeCompilePreviewReviewSelectionUsesEncodedReferenceFields(t *testing.T) {
	selectedBy := "alice selected_by_b64=fake review=/tmp/ouro/evil-review.json"
	selectedAt := "2026-06-01T00:15:00Z"
	reason := "best reviewed candidate review=/tmp/ouro/other-review.json selected_at=2030-01-01T00:00:00Z"
	reviewPath := "/tmp/ouro/compile preview reviews/candidate review=evil.json"
	row := "compile-preview review selection: status=current mode=metadata-only action=discover_assets superseded=1" +
		" selected_by=" + runtimeCompactRowTextValue(selectedBy) +
		" selected_by_b64=" + base64.RawURLEncoding.EncodeToString([]byte(selectedBy)) +
		" selected_at=" + runtimeCompactRowTextValue(selectedAt) +
		" selected_at_b64=" + base64.RawURLEncoding.EncodeToString([]byte(selectedAt)) +
		" reason=" + runtimeCompactRowTextValue(reason) +
		" reason_b64=" + base64.RawURLEncoding.EncodeToString([]byte(reason)) +
		" review=" + runtimeCompactRowTextValue(reviewPath) +
		" review_b64=" + base64.RawURLEncoding.EncodeToString([]byte(reviewPath))

	action := runtimeCompilePreviewReviewSelectionActionForRow(row)
	if action.State != "current" || action.ExecutionMode != "metadata-only" || action.ActionType != "discover_assets" || action.Superseded != "1" {
		t.Fatalf("encoded selection references must not retarget selection metadata: %#v", action)
	}
	if action.ReviewPath != reviewPath || action.SelectedBy != selectedBy || action.SelectedAt != selectedAt || action.Reason != reason {
		t.Fatalf("expected encoded selection fields to decode, got %#v", action)
	}
}

func TestRuntimeCompilePreviewReviewSelectionUsesEncodedClearedFields(t *testing.T) {
	clearedBy := "alice cleared_by_b64=fake review=/tmp/ouro/evil-review.json"
	clearedAt := "2026-06-01T00:35:00Z"
	reason := "authorization changed review=/tmp/ouro/other-review.json cleared_at=2030-01-01T00:00:00Z"
	reviewPath := "/tmp/ouro/compile preview reviews/candidate review=evil.json"
	row := "compile-preview review selection: status=cleared mode=metadata-only action=discover_assets superseded=1" +
		" cleared_by=" + runtimeCompactRowTextValue(clearedBy) +
		" cleared_by_b64=" + base64.RawURLEncoding.EncodeToString([]byte(clearedBy)) +
		" cleared_at=" + runtimeCompactRowTextValue(clearedAt) +
		" cleared_at_b64=" + base64.RawURLEncoding.EncodeToString([]byte(clearedAt)) +
		" reason=" + runtimeCompactRowTextValue(reason) +
		" reason_b64=" + base64.RawURLEncoding.EncodeToString([]byte(reason)) +
		" review=" + runtimeCompactRowTextValue(reviewPath) +
		" review_b64=" + base64.RawURLEncoding.EncodeToString([]byte(reviewPath))

	action := runtimeCompilePreviewReviewSelectionActionForRow(row)
	if action.State != "cleared" || action.ExecutionMode != "metadata-only" || action.ActionType != "discover_assets" || action.Superseded != "1" {
		t.Fatalf("encoded cleared selection references must not retarget selection metadata: %#v", action)
	}
	if action.ReviewPath != reviewPath || action.ClearedBy != clearedBy || action.ClearedAt != clearedAt || action.Reason != reason {
		t.Fatalf("expected encoded cleared selection fields to decode, got %#v", action)
	}
}

func TestRuntimeCompilePreviewReviewComparisonUsesEncodedReferenceFields(t *testing.T) {
	basePath := "/tmp/ouro/compile preview reviews/base candidate=evil.json"
	candidatePath := "/tmp/ouro/compile preview reviews/candidate base=evil.json"
	row := "compile-preview review comparison: mode=metadata-only action=discover_assets node_delta=1 warning_delta=0 added=1 removed=0 changed=1 reordered=1 score_changed=1 signal_changed=1" +
		" base=" + runtimeCompactRowTextValue(basePath) +
		" base_b64=" + base64.RawURLEncoding.EncodeToString([]byte(basePath)) +
		" candidate=" + runtimeCompactRowTextValue(candidatePath) +
		" candidate_b64=" + base64.RawURLEncoding.EncodeToString([]byte(candidatePath))

	action := runtimeCompilePreviewReviewComparisonActionForRow(row)
	if action.ExecutionMode != "metadata-only" || action.ActionType != "discover_assets" || action.NodeDelta != "1" || action.Added != "1" {
		t.Fatalf("encoded comparison references must not retarget comparison metadata: %#v", action)
	}
	if action.BasePath != basePath || action.CandidatePath != candidatePath {
		t.Fatalf("expected encoded comparison paths to decode, got %#v", action)
	}
}

func TestRuntimeCompilePreviewReviewExecutionDispatchArtifactUsesEncodedReasonText(t *testing.T) {
	reason := "dispatch approved reason_b64=fake status=evil mode=execute action=evil nodes=99 warnings=99 actor=mallory created_at=2030-01-01T00:00:00Z review=/tmp/ouro/evil-review.json selection=/tmp/ouro/evil-selection.json path=/tmp/ouro/evil-dispatch.json"
	row := "compile-preview review execution dispatch artifact: status=dry-run mode=metadata-only action=discover_assets nodes=1 warnings=0 actor=operator created_at=2026-06-04T12:15:00Z review=/tmp/ouro/review.json selection=/tmp/ouro/selection.json path=/tmp/ouro/dispatch.json reason=" + reason + " reason_b64=" + base64.RawURLEncoding.EncodeToString([]byte(reason))

	action := runtimeCompilePreviewReviewExecutionDispatchArtifactActionForRow(row)
	if action.State != "dry-run" || action.ExecutionMode != "metadata-only" || action.ActionType != "discover_assets" {
		t.Fatalf("encoded dispatch reason must not retarget dispatch identity: %#v", action)
	}
	if action.Nodes != "1" || action.Warnings != "0" {
		t.Fatalf("encoded dispatch reason must not retarget dispatch counts: %#v", action)
	}
	if action.CreatedBy != "operator" || action.CreatedAt != "2026-06-04T12:15:00Z" {
		t.Fatalf("encoded dispatch reason must not retarget actor/timestamp: %#v", action)
	}
	if action.ReviewPath != "/tmp/ouro/review.json" || action.RequestPath != "/tmp/ouro/selection.json" || action.ArtifactPath != "/tmp/ouro/dispatch.json" {
		t.Fatalf("encoded dispatch reason must not retarget artifact references: %#v", action)
	}
	if action.Reason != reason {
		t.Fatalf("expected encoded dispatch reason to be decoded, got %q", action.Reason)
	}
}

func TestRuntimeCompilePreviewReviewExecutionDispatchArtifactLegacyReasonFallback(t *testing.T) {
	row := "compile-preview review execution dispatch artifact: status=dry-run mode=metadata-only action=discover_assets nodes=1 warnings=0 actor=operator created_at=2026-06-04T12:15:00Z review=/tmp/ouro/review.json selection=/tmp/ouro/selection.json path=/tmp/ouro/dispatch.json reason=ready for execution gate"

	action := runtimeCompilePreviewReviewExecutionDispatchArtifactActionForRow(row)
	if action.ArtifactPath != "/tmp/ouro/dispatch.json" || action.ReviewPath != "/tmp/ouro/review.json" || action.RequestPath != "/tmp/ouro/selection.json" {
		t.Fatalf("legacy dispatch row references changed: %#v", action)
	}
	if action.Reason != "ready for execution gate" {
		t.Fatalf("legacy dispatch reason fallback changed: %#v", action)
	}
}

func TestRuntimeCompilePreviewReviewExecutionArtifactUsesEncodedReasonErrorText(t *testing.T) {
	reason := "operator reviewed reason_b64=fake run=evil dispatch=/tmp/ouro/evil-dispatch.json review=/tmp/ouro/evil-review.json selection=/tmp/ouro/evil-selection.json path=/tmp/ouro/evil-execution.json events=99 actor=mallory executed_at=2030-01-01T00:00:00Z error=evil"
	executionError := "runner failed status=failed mode=metadata-only action=evil nodes=99 warnings=99 run=evil path=/tmp/ouro/evil-execution.json error_b64=fake"
	row := "compile-preview review execution artifact: status=completed mode=execute action=discover_assets nodes=1 warnings=0 events=12 actor=operator executed_at=2026-06-04T11:30:00Z run=compile-preview-review-execution-1 dispatch=/tmp/ouro/dispatch.json review=/tmp/ouro/review.json selection=/tmp/ouro/selection.json path=/tmp/ouro/execution.json reason=" + reason + " reason_b64=" + base64.RawURLEncoding.EncodeToString([]byte(reason)) + " error=" + executionError + " error_b64=" + base64.RawURLEncoding.EncodeToString([]byte(executionError))

	action := runtimeCompilePreviewReviewExecutionArtifactActionForRow(row)
	if action.State != "completed" || action.ExecutionMode != "execute" || action.ActionType != "discover_assets" {
		t.Fatalf("encoded execution reason/error must not retarget execution identity: %#v", action)
	}
	if action.Nodes != "1" || action.Warnings != "0" || action.Results != "events=12" {
		t.Fatalf("encoded execution reason/error must not retarget execution counts: %#v", action)
	}
	if action.CreatedBy != "operator" || action.CreatedAt != "2026-06-04T11:30:00Z" || action.RunID != "compile-preview-review-execution-1" {
		t.Fatalf("encoded execution reason/error must not retarget actor/run metadata: %#v", action)
	}
	if action.DispatchPath != "/tmp/ouro/dispatch.json" || action.ReviewPath != "/tmp/ouro/review.json" || action.RequestPath != "/tmp/ouro/selection.json" || action.ArtifactPath != "/tmp/ouro/execution.json" {
		t.Fatalf("encoded execution reason/error must not retarget artifact references: %#v", action)
	}
	if action.Reason != reason || action.ExecutionError != executionError {
		t.Fatalf("expected encoded reason/error to be decoded, got reason=%q error=%q", action.Reason, action.ExecutionError)
	}
}

func TestRuntimeCompilePreviewReviewExecutionArtifactLegacyReasonErrorFallback(t *testing.T) {
	row := "compile-preview review execution artifact: status=failed mode=execute action=discover_assets nodes=1 warnings=0 events=12 actor=operator executed_at=2026-06-04T11:30:00Z run=compile-preview-review-execution-1 dispatch=/tmp/ouro/dispatch.json review=/tmp/ouro/review.json selection=/tmp/ouro/selection.json path=/tmp/ouro/execution.json reason=reviewed execution gate error=runner failed"

	action := runtimeCompilePreviewReviewExecutionArtifactActionForRow(row)
	if action.ArtifactPath != "/tmp/ouro/execution.json" || action.ReviewPath != "/tmp/ouro/review.json" || action.RequestPath != "/tmp/ouro/selection.json" || action.DispatchPath != "/tmp/ouro/dispatch.json" {
		t.Fatalf("legacy execution row references changed: %#v", action)
	}
	if action.Reason != "reviewed execution gate" || action.ExecutionError != "runner failed" {
		t.Fatalf("legacy execution reason/error fallback changed: %#v", action)
	}
}

func TestModelUpdateShowsSelectedCompilePreviewReviewComparisonDetails(t *testing.T) {
	tabs := DefaultRuntimeTabs().
		Select(RuntimeTabToolchainHealth).
		WithRows(RuntimeTabToolchainHealth, []string{
			"toolchain: ready",
			"compile-preview review comparison: mode=metadata-only base=/tmp/ouro/base.json candidate=/tmp/ouro/candidate.json action=discover_assets node_delta=1 warning_delta=0 added=1 removed=0 changed=1 reordered=1 score_changed=1 signal_changed=1",
		}).
		SelectRow(1)
	model := NewModel().WithGraphStack(GraphStack{RuntimeTabs: tabs})

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	if cmd != nil {
		t.Fatalf("expected compile-preview review comparison details update to be synchronous")
	}
	next, ok := updated.(Model)
	if !ok {
		t.Fatalf("expected Model, got %T", updated)
	}
	view := next.graphStack.Render(140, 44)
	for _, want := range []string{
		"[Node Inspector]",
		"runtime action: compile-preview-review-comparison",
		"base path: /tmp/ouro/base.json",
		"candidate path: /tmp/ouro/candidate.json",
		"action type: discover_assets",
		"node delta: 1",
		"warning delta: 0",
		"added: 1",
		"removed: 0",
		"changed: 1",
		"reordered: 1",
		"score changed: 1",
		"signal changed: 1",
		"mode: read-only",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("compile-preview review comparison details missing %q:\n%s", want, view)
		}
	}
}

func TestModelUpdateShowsSelectedCompilePreviewReviewExecutionDetails(t *testing.T) {
	tabs := DefaultRuntimeTabs().
		Select(RuntimeTabToolchainHealth).
		WithRows(RuntimeTabToolchainHealth, []string{
			"toolchain: ready",
			"compile-preview review execution artifact: status=completed mode=execute action=discover_assets nodes=1 warnings=0 events=12 actor=operator executed_at=2026-06-04T11:30:00Z run=compile-preview-review-execution-1 review=/tmp/ouro/review.json selection=/tmp/ouro/selection.json path=/tmp/ouro/execution.json reason=reviewed execution gate",
		}).
		SelectRow(1)
	model := NewModel().WithGraphStack(GraphStack{RuntimeTabs: tabs})

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	if cmd != nil {
		t.Fatalf("expected compile-preview review execution details update to be synchronous")
	}
	next, ok := updated.(Model)
	if !ok {
		t.Fatalf("expected Model, got %T", updated)
	}
	view := next.graphStack.Render(140, 44)
	for _, want := range []string{
		"[Node Inspector]",
		"runtime action: compile-preview-review-execution-artifact",
		"artifact path: /tmp/ouro/execution.json",
		"review path: /tmp/ouro/review.json",
		"request path: /tmp/ouro/selection.json",
		"run: compile-preview-review-execution-1",
		"action type: discover_assets",
		"nodes: 1",
		"warnings: 0",
		"results: events=12",
		"created by: operator",
		"created at: 2026-06-04T11:30:00Z",
		"reason: reviewed execution gate",
		"mode: read-only",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("compile-preview review execution details missing %q:\n%s", want, view)
		}
	}
}

func TestModelUpdateShowsSelectedCompilePreviewReviewSelectionDetails(t *testing.T) {
	tabs := DefaultRuntimeTabs().
		Select(RuntimeTabToolchainHealth).
		WithRows(RuntimeTabToolchainHealth, []string{
			"toolchain: ready",
			"compile-preview review selection: status=current mode=metadata-only review=/tmp/ouro/candidate.json action=discover_assets superseded=1 selected_by=alice selected_at=2026-06-01T00:15:00Z reason=best reviewed candidate",
		}).
		SelectRow(1)
	model := NewModel().WithGraphStack(GraphStack{RuntimeTabs: tabs})

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	if cmd != nil {
		t.Fatalf("expected compile-preview review selection details update to be synchronous")
	}
	next, ok := updated.(Model)
	if !ok {
		t.Fatalf("expected Model, got %T", updated)
	}
	view := next.graphStack.Render(140, 44)
	for _, want := range []string{
		"[Node Inspector]",
		"runtime action: compile-preview-review-selection",
		"review path: /tmp/ouro/candidate.json",
		"action type: discover_assets",
		"superseded: 1",
		"selected by: alice",
		"selected at: 2026-06-01T00:15:00Z",
		"reason: best reviewed candidate",
		"mode: metadata-only",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("compile-preview review selection details missing %q:\n%s", want, view)
		}
	}
}

func TestModelUpdateShowsClearedCompilePreviewReviewSelectionDetails(t *testing.T) {
	tabs := DefaultRuntimeTabs().
		Select(RuntimeTabToolchainHealth).
		WithRows(RuntimeTabToolchainHealth, []string{
			"toolchain: ready",
			"compile-preview review selection: status=cleared mode=metadata-only review=/tmp/ouro/candidate.json action=discover_assets superseded=1 cleared_by=alice cleared_at=2026-06-01T00:35:00Z reason=authorization changed",
		}).
		SelectRow(1)
	model := NewModel().WithGraphStack(GraphStack{RuntimeTabs: tabs})

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	if cmd != nil {
		t.Fatalf("expected compile-preview review cleared selection details update to be synchronous")
	}
	next, ok := updated.(Model)
	if !ok {
		t.Fatalf("expected Model, got %T", updated)
	}
	view := next.graphStack.Render(140, 44)
	for _, want := range []string{
		"[Node Inspector]",
		"runtime action: compile-preview-review-selection",
		"state: cleared",
		"review path: /tmp/ouro/candidate.json",
		"action type: discover_assets",
		"superseded: 1",
		"cleared by: alice",
		"cleared at: 2026-06-01T00:35:00Z",
		"reason: authorization changed",
		"mode: read-only",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("compile-preview review cleared selection details missing %q:\n%s", want, view)
		}
	}
}

func TestModelUpdateShowsSelectedReplayWorkbenchSessionDetails(t *testing.T) {
	tabs := DefaultRuntimeTabs().
		Select(RuntimeTabTimelineLanes).
		WithRows(RuntimeTabTimelineLanes, []string{
			"timeline lanes: ready",
			"replay workbench session artifact: status=saved mode=metadata-only primary=run-primary selected=run-comparison runs=2 filters=3 endpoints=8 created_by=operator created_at=2026-06-01T02:10:00Z reason=saved comparison focus decision=accepted reviewer=alice reviewed_at=2026-06-01T02:35:00Z review_reason=authorized comparison baseline path=/tmp/ouro/replay/workbench-sessions/session.json",
		}).
		SelectRow(1)
	model := NewModel().WithGraphStack(GraphStack{RuntimeTabs: tabs})

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	if cmd != nil {
		t.Fatalf("expected replay workbench session details update to be synchronous")
	}
	next, ok := updated.(Model)
	if !ok {
		t.Fatalf("expected Model, got %T", updated)
	}
	view := next.graphStack.Render(150, 44)
	for _, want := range []string{
		"[Node Inspector]",
		"runtime action: replay-workbench-session-artifact",
		"artifact path: /tmp/ouro/replay/workbench-sessions/session.json",
		"state: saved",
		"execution mode: metadata-only",
		"run: run-primary",
		"selected run: run-comparison",
		"run ids: 2",
		"filters: 3",
		"endpoints: 8",
		"created by: operator",
		"created at: 2026-06-01T02:10:00Z",
		"reason: saved comparison focus",
		"decision: accepted",
		"reviewer: alice",
		"reviewed at: 2026-06-01T02:35:00Z",
		"review reason: authorized comparison baseline",
		"mode: read-only",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("replay workbench session details missing %q:\n%s", want, view)
		}
	}
}

func TestRuntimeReplayWorkbenchSessionArtifactUsesEncodedReviewText(t *testing.T) {
	reason := "saved comparison focus current=false path=/tmp/ouro/replay/workbench-sessions/evil.json"
	reviewReason := "authorized comparison baseline reviewed_at=2030-01-01T00:00:00Z current=false"
	row := "replay workbench session artifact: status=saved mode=metadata-only primary=run-primary selected=run-comparison runs=2 filters=3 endpoints=8 created_by=operator created_at=2026-06-01T02:10:00Z reason=" + reason + " reason_b64=" + base64.RawURLEncoding.EncodeToString([]byte(reason)) + " decision=accepted reviewer=alice reviewed_at=2026-06-01T02:35:00Z review_reason=" + reviewReason + " review_reason_b64=" + base64.RawURLEncoding.EncodeToString([]byte(reviewReason)) + " current=true path=/tmp/ouro/replay/workbench-sessions/session.json"

	action := runtimeReplayWorkbenchSessionArtifactActionForRow(row)
	if action.ArtifactPath != "/tmp/ouro/replay/workbench-sessions/session.json" || action.Current != "true" {
		t.Fatalf("encoded session text must not retarget control metadata: %#v", action)
	}
	if action.Reason != reason || action.ReviewReason != reviewReason {
		t.Fatalf("expected encoded session text to be decoded, got reason=%q review_reason=%q", action.Reason, action.ReviewReason)
	}
}

func TestRuntimeReplayWorkbenchSessionArtifactUsesLegacyReasonBeforeEmptySidecar(t *testing.T) {
	row := "replay workbench session artifact: status=saved mode=metadata-only primary=run-primary selected=run-comparison runs=2 filters=3 endpoints=8 created_by=operator created_at=2026-06-01T02:10:00Z reason=none reason_b64= decision=accepted reviewer=alice reviewed_at=2026-06-01T02:35:00Z review_reason=none review_reason_b64=" + base64.RawURLEncoding.EncodeToString([]byte("none")) + " current=false path=/tmp/ouro/replay/workbench-sessions/session.json"

	action := runtimeReplayWorkbenchSessionArtifactActionForRow(row)
	if action.Reason != "none" {
		t.Fatalf("expected legacy reason fallback to stop before empty b64 sidecar, got %q", action.Reason)
	}
}

func TestModelUpdateShowsSelectedReplayWorkbenchCollaborationDetails(t *testing.T) {
	tabs := DefaultRuntimeTabs().
		Select(RuntimeTabTimelineLanes).
		WithRows(RuntimeTabTimelineLanes, []string{
			"timeline lanes: ready",
			"replay workbench collaboration artifact: status=saved mode=metadata-only primary=run-primary selected=run-comparison runs=2 filters=3 endpoints=10 participants=2 created_by=lead created_at=2026-06-01T03:45:00Z reason=pair review decision=accepted reviewer=alice reviewed_at=2026-06-01T05:10:00Z review_reason=authorized pair review path=/tmp/ouro/replay/workbench-collaborations/collab.json session_path=/tmp/ouro/replay/workbench-sessions/session.json",
		}).
		SelectRow(1)
	model := NewModel().WithGraphStack(GraphStack{RuntimeTabs: tabs})

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	if cmd != nil {
		t.Fatalf("expected replay workbench collaboration details update to be synchronous")
	}
	next, ok := updated.(Model)
	if !ok {
		t.Fatalf("expected Model, got %T", updated)
	}
	view := next.graphStack.Render(150, 44)
	for _, want := range []string{
		"[Node Inspector]",
		"runtime action: replay-workbench-collaboration-artifact",
		"artifact path: /tmp/ouro/replay/workbench-collaborations/collab.json",
		"session path: /tmp/ouro/replay/workbench-sessions/session.json",
		"state: saved",
		"execution mode: metadata-only",
		"run: run-primary",
		"selected run: run-comparison",
		"run ids: 2",
		"filters: 3",
		"endpoints: 10",
		"participants: 2",
		"created by: lead",
		"created at: 2026-06-01T03:45:00Z",
		"reason: pair review",
		"decision: accepted",
		"reviewer: alice",
		"reviewed at: 2026-06-01T05:10:00Z",
		"review reason: authorized pair review",
		"mode: read-only",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("replay workbench collaboration details missing %q:\n%s", want, view)
		}
	}
}

func TestRuntimeReplayWorkbenchCollaborationArtifactUsesEncodedReviewText(t *testing.T) {
	reason := "pair review path=/tmp/ouro/replay/workbench-collaborations/evil.json session_path=/tmp/ouro/replay/workbench-sessions/evil.json"
	reviewReason := "authorized pair review current=false reviewed_at=2030-01-01T00:00:00Z"
	row := "replay workbench collaboration artifact: status=saved mode=metadata-only primary=run-primary selected=run-comparison runs=2 filters=3 endpoints=10 participants=2 created_by=lead created_at=2026-06-01T03:45:00Z reason=" + reason + " reason_b64=" + base64.RawURLEncoding.EncodeToString([]byte(reason)) + " decision=accepted reviewer=alice reviewed_at=2026-06-01T05:10:00Z review_reason=" + reviewReason + " review_reason_b64=" + base64.RawURLEncoding.EncodeToString([]byte(reviewReason)) + " current=true path=/tmp/ouro/replay/workbench-collaborations/collab.json session_path=/tmp/ouro/replay/workbench-sessions/session.json"

	action := runtimeReplayWorkbenchCollaborationArtifactActionForRow(row)
	if action.ArtifactPath != "/tmp/ouro/replay/workbench-collaborations/collab.json" || action.SessionPath != "/tmp/ouro/replay/workbench-sessions/session.json" || action.Current != "true" {
		t.Fatalf("encoded collaboration text must not retarget control metadata: %#v", action)
	}
	if action.Reason != reason || action.ReviewReason != reviewReason {
		t.Fatalf("expected encoded collaboration text to be decoded, got reason=%q review_reason=%q", action.Reason, action.ReviewReason)
	}
}

func TestModelUpdateShowsSelectedReplayWorkbenchCollaborationPresenceDetails(t *testing.T) {
	tabs := DefaultRuntimeTabs().
		Select(RuntimeTabTimelineLanes).
		WithRows(RuntimeTabTimelineLanes, []string{
			"timeline lanes: ready",
			"replay workbench collaboration presence artifact: status=away mode=metadata-only primary=run-primary selected=run-comparison participant=alice seen_by=lead seen_at=2026-06-01T05:30:00Z reason=review active path=/tmp/ouro/replay/workbench-collaboration-presence/alice.json collaboration=/tmp/ouro/replay/workbench-collaborations/collab.json session_path=/tmp/ouro/replay/workbench-sessions/session.json",
		}).
		SelectRow(1)
	model := NewModel().WithGraphStack(GraphStack{RuntimeTabs: tabs})

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	if cmd != nil {
		t.Fatalf("expected replay workbench collaboration presence details update to be synchronous")
	}
	next, ok := updated.(Model)
	if !ok {
		t.Fatalf("expected Model, got %T", updated)
	}
	view := next.graphStack.Render(150, 44)
	for _, want := range []string{
		"[Node Inspector]",
		"runtime action: replay-workbench-collaboration-presence",
		"artifact path: /tmp/ouro/replay/workbench-collaboration-presence/alice.json",
		"collaboration path: /tmp/ouro/replay/workbench-collaborations/collab.json",
		"session path: /tmp/ouro/replay/workbench-sessions/session.json",
		"state: away",
		"execution mode: metadata-only",
		"run: run-primary",
		"selected run: run-comparison",
		"participant: alice",
		"seen by: lead",
		"seen at: 2026-06-01T05:30:00Z",
		"reason: review active",
		"mode: read-only",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("replay workbench collaboration presence details missing %q:\n%s", want, view)
		}
	}
}

func TestRuntimeReplayWorkbenchCollaborationPresenceUsesEncodedReasonText(t *testing.T) {
	reason := "review active path=/tmp/ouro/replay/workbench-collaboration-presence/evil.json collaboration=/tmp/ouro/replay/workbench-collaborations/evil.json session_path=/tmp/ouro/replay/workbench-sessions/evil.json"
	row := "replay workbench collaboration presence artifact: status=away mode=metadata-only primary=run-primary selected=run-comparison participant=alice seen_by=lead seen_at=2026-06-01T05:30:00Z reason=" + reason + " reason_b64=" + base64.RawURLEncoding.EncodeToString([]byte(reason)) + " path=/tmp/ouro/replay/workbench-collaboration-presence/alice.json collaboration=/tmp/ouro/replay/workbench-collaborations/collab.json session_path=/tmp/ouro/replay/workbench-sessions/session.json"

	action := runtimeReplayWorkbenchCollaborationPresenceActionForRow(row)
	if action.ArtifactPath != "/tmp/ouro/replay/workbench-collaboration-presence/alice.json" || action.CollaborationPath != "/tmp/ouro/replay/workbench-collaborations/collab.json" || action.SessionPath != "/tmp/ouro/replay/workbench-sessions/session.json" {
		t.Fatalf("encoded presence reason must not retarget control metadata: %#v", action)
	}
	if action.Reason != reason {
		t.Fatalf("expected encoded presence reason to be decoded, got %q", action.Reason)
	}
}

func TestRuntimeReplayWorkbenchCollaborationNoteArtifactUsesEncodedText(t *testing.T) {
	reason := "collaborative evidence review current=false path=/tmp/ouro/replay/workbench-collaboration-notes/evil.json"
	body := "Confirm graph node before accepting. collaboration=/tmp/ouro/replay/workbench-collaborations/evil.json session_path=/tmp/ouro/replay/workbench-sessions/evil.json"
	reviewReason := "validated evidence note reviewed_at=2030-01-01T00:00:00Z current=false"
	row := "replay workbench collaboration note artifact: status=saved mode=metadata-only primary=run-primary selected=run-comparison author=alice target=graph-node:fact-target target_kind=graph-node target_id=fact-target created_at=2026-06-01T08:20:00Z reason=" + reason + " reason_b64=" + base64.RawURLEncoding.EncodeToString([]byte(reason)) + " body=" + body + " body_b64=" + base64.RawURLEncoding.EncodeToString([]byte(body)) + " decision=accepted reviewer=alice reviewed_at=2026-06-01T09:30:00Z review_reason=" + reviewReason + " review_reason_b64=" + base64.RawURLEncoding.EncodeToString([]byte(reviewReason)) + " current=true path=/tmp/ouro/replay/workbench-collaboration-notes/note.json collaboration=/tmp/ouro/replay/workbench-collaborations/collab.json session_path=/tmp/ouro/replay/workbench-sessions/session.json"

	action := runtimeReplayWorkbenchCollaborationNoteActionForRow(row)
	if action.ArtifactPath != "/tmp/ouro/replay/workbench-collaboration-notes/note.json" || action.CollaborationPath != "/tmp/ouro/replay/workbench-collaborations/collab.json" || action.SessionPath != "/tmp/ouro/replay/workbench-sessions/session.json" || action.Current != "true" {
		t.Fatalf("encoded note text must not retarget control metadata: %#v", action)
	}
	if action.Reason != reason || action.Body != body || action.ReviewReason != reviewReason {
		t.Fatalf("expected encoded note text to be decoded, got reason=%q body=%q review_reason=%q", action.Reason, action.Body, action.ReviewReason)
	}
}

func TestModelUpdateShowsSelectedReplayWorkbenchCollaborationNoteDetails(t *testing.T) {
	tabs := DefaultRuntimeTabs().
		Select(RuntimeTabTimelineLanes).
		WithRows(RuntimeTabTimelineLanes, []string{
			"timeline lanes: ready",
			"replay workbench collaboration note artifact: status=saved mode=metadata-only primary=run-primary selected=run-comparison author=alice target=graph-node:fact-target target_kind=graph-node target_id=fact-target created_at=2026-06-01T08:20:00Z reason=collaborative evidence review body=Confirm graph node before accepting. decision=accepted reviewer=alice reviewed_at=2026-06-01T09:30:00Z review_reason=validated evidence note current=false path=/tmp/ouro/replay/workbench-collaboration-notes/note.json collaboration=/tmp/ouro/replay/workbench-collaborations/collab.json session_path=/tmp/ouro/replay/workbench-sessions/session.json",
		}).
		SelectRow(1)
	model := NewModel().WithGraphStack(GraphStack{RuntimeTabs: tabs})

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	if cmd != nil {
		t.Fatalf("expected replay workbench collaboration note details update to be synchronous")
	}
	next, ok := updated.(Model)
	if !ok {
		t.Fatalf("expected Model, got %T", updated)
	}
	view := next.graphStack.Render(150, 44)
	for _, want := range []string{
		"[Node Inspector]",
		"runtime action: replay-workbench-collaboration-note",
		"artifact path: /tmp/ouro/replay/workbench-collaboration-notes/note.json",
		"collaboration path: /tmp/ouro/replay/workbench-collaborations/collab.json",
		"session path: /tmp/ouro/replay/workbench-sessions/session.json",
		"state: saved",
		"execution mode: metadata-only",
		"run: run-primary",
		"selected run: run-comparison",
		"author: alice",
		"target kind: graph-node",
		"target id: fact-target",
		"body: Confirm graph node before accepting.",
		"decision: accepted",
		"reviewer: alice",
		"reviewed at: 2026-06-01T09:30:00Z",
		"review reason: validated evidence note",
		"reason: collaborative evidence review",
		"mode: metadata-only",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("replay workbench collaboration note details missing %q:\n%s", want, view)
		}
	}
}

func TestModelUpdateShowsSelectedReplayWorkbenchCollaborationNoteSelectionDetails(t *testing.T) {
	tabs := DefaultRuntimeTabs().
		Select(RuntimeTabTimelineLanes).
		WithRows(RuntimeTabTimelineLanes, []string{
			"timeline lanes: ready",
			"replay workbench collaboration note selection: status=current mode=metadata-only note=/tmp/ouro/replay/workbench-collaboration-notes/note.json collaboration=/tmp/ouro/replay/workbench-collaborations/collab.json session=/tmp/ouro/replay/workbench-sessions/session.json primary=run-primary selected=run-comparison author=alice target=graph-node:fact-target target_kind=graph-node target_id=fact-target superseded=1 selected_by=lead selected_at=2026-06-01T09:50:00Z reason=current hosted note",
		}).
		SelectRow(1)
	model := NewModel().WithGraphStack(GraphStack{RuntimeTabs: tabs})

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	if cmd != nil {
		t.Fatalf("expected replay workbench collaboration note selection details update to be synchronous")
	}
	next, ok := updated.(Model)
	if !ok {
		t.Fatalf("expected Model, got %T", updated)
	}
	view := next.graphStack.Render(150, 44)
	for _, want := range []string{
		"[Node Inspector]",
		"runtime action: replay-workbench-collaboration-note-selection",
		"artifact path: /tmp/ouro/replay/workbench-collaboration-notes/note.json",
		"collaboration path: /tmp/ouro/replay/workbench-collaborations/collab.json",
		"session path: /tmp/ouro/replay/workbench-sessions/session.json",
		"state: current",
		"execution mode: metadata-only",
		"run: run-primary",
		"selected run: run-comparison",
		"author: alice",
		"target kind: graph-node",
		"target id: fact-target",
		"selected by: lead",
		"selected at: 2026-06-01T09:50:00Z",
		"superseded: 1",
		"reason: current hosted note",
		"mode: metadata-only",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("replay workbench collaboration note selection details missing %q:\n%s", want, view)
		}
	}
}

func TestRuntimeReplayWorkbenchCollaborationNoteSelectionUsesEncodedReasonText(t *testing.T) {
	reason := "current hosted note note=/tmp/ouro/replay/workbench-collaboration-notes/evil.json collaboration=/tmp/ouro/replay/workbench-collaborations/evil.json session=/tmp/ouro/replay/workbench-sessions/evil.json"
	row := "replay workbench collaboration note selection: status=current mode=metadata-only note=/tmp/ouro/replay/workbench-collaboration-notes/note.json collaboration=/tmp/ouro/replay/workbench-collaborations/collab.json session=/tmp/ouro/replay/workbench-sessions/session.json primary=run-primary selected=run-comparison author=alice target=graph-node:fact-target target_kind=graph-node target_id=fact-target superseded=1 selected_by=lead selected_at=2026-06-01T09:50:00Z reason=" + reason + " reason_b64=" + base64.RawURLEncoding.EncodeToString([]byte(reason))

	action := runtimeReplayWorkbenchCollaborationNoteSelectionActionForRow(row)
	if action.ArtifactPath != "/tmp/ouro/replay/workbench-collaboration-notes/note.json" || action.CollaborationPath != "/tmp/ouro/replay/workbench-collaborations/collab.json" || action.SessionPath != "/tmp/ouro/replay/workbench-sessions/session.json" {
		t.Fatalf("encoded note selection reason must not retarget control metadata: %#v", action)
	}
	if action.Reason != reason {
		t.Fatalf("expected encoded note selection reason to be decoded, got %q", action.Reason)
	}
}

func TestRuntimeReplayWorkbenchCollaborationSelectionUsesEncodedReasonText(t *testing.T) {
	reason := "current collaboration focus collaboration=/tmp/ouro/replay/workbench-collaborations/evil.json session=/tmp/ouro/replay/workbench-sessions/evil.json"
	row := "replay workbench collaboration selection: status=current mode=metadata-only collaboration=/tmp/ouro/replay/workbench-collaborations/collab.json session=/tmp/ouro/replay/workbench-sessions/session.json primary=run-primary selected=run-comparison runs=2 filters=3 endpoints=10 participants=2 superseded=1 selected_by=alice selected_at=2026-06-01T03:52:00Z reason=" + reason + " reason_b64=" + base64.RawURLEncoding.EncodeToString([]byte(reason))

	action := runtimeReplayWorkbenchCollaborationSelectionActionForRow(row)
	if action.ArtifactPath != "/tmp/ouro/replay/workbench-collaborations/collab.json" || action.SessionPath != "/tmp/ouro/replay/workbench-sessions/session.json" {
		t.Fatalf("encoded collaboration selection reason must not retarget control metadata: %#v", action)
	}
	if action.Reason != reason {
		t.Fatalf("expected encoded collaboration selection reason to be decoded, got %q", action.Reason)
	}
}

func TestModelUpdateShowsSelectedReplayWorkbenchSessionSelectionDetails(t *testing.T) {
	tabs := DefaultRuntimeTabs().
		Select(RuntimeTabTimelineLanes).
		WithRows(RuntimeTabTimelineLanes, []string{
			"timeline lanes: ready",
			"replay workbench session selection: status=current mode=metadata-only session=/tmp/ouro/replay/workbench-sessions/current.json primary=run-primary selected=run-comparison runs=2 filters=3 endpoints=8 superseded=1 selected_by=alice selected_at=2026-06-01T02:42:00Z reason=current investigation focus",
		}).
		SelectRow(1)
	model := NewModel().WithGraphStack(GraphStack{RuntimeTabs: tabs})

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	if cmd != nil {
		t.Fatalf("expected replay workbench session selection details update to be synchronous")
	}
	next, ok := updated.(Model)
	if !ok {
		t.Fatalf("expected Model, got %T", updated)
	}
	view := next.graphStack.Render(150, 44)
	for _, want := range []string{
		"[Node Inspector]",
		"runtime action: replay-workbench-session-selection",
		"artifact path: /tmp/ouro/replay/workbench-sessions/current.json",
		"state: current",
		"execution mode: metadata-only",
		"run: run-primary",
		"selected run: run-comparison",
		"run ids: 2",
		"filters: 3",
		"endpoints: 8",
		"superseded: 1",
		"selected by: alice",
		"selected at: 2026-06-01T02:42:00Z",
		"reason: current investigation focus",
		"mode: metadata-only",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("replay workbench session selection details missing %q:\n%s", want, view)
		}
	}
}

func TestRuntimeReplayWorkbenchSessionSelectionUsesEncodedReasonText(t *testing.T) {
	reason := "current investigation focus session=/tmp/ouro/replay/workbench-sessions/evil.json primary=run-evil selected=run-evil"
	row := "replay workbench session selection: status=current mode=metadata-only selected_by=alice selected_at=2026-06-01T02:42:00Z reason=" + reason + " reason_b64=" + base64.RawURLEncoding.EncodeToString([]byte(reason)) + " primary=run-primary selected=run-comparison runs=2 filters=3 endpoints=8 superseded=1 session=/tmp/ouro/replay/workbench-sessions/current.json"

	action := runtimeReplayWorkbenchSessionSelectionActionForRow(row)
	if action.ArtifactPath != "/tmp/ouro/replay/workbench-sessions/current.json" || action.RunID != "run-primary" || action.SelectedRunID != "run-comparison" {
		t.Fatalf("encoded session selection reason must not retarget control metadata: %#v", action)
	}
	if action.Reason != reason {
		t.Fatalf("expected encoded session selection reason to be decoded, got %q", action.Reason)
	}
}

func TestModelUpdateClearsReplayWorkbenchSessionSelectionOnlyAfterSecondConfirm(t *testing.T) {
	tabs := DefaultRuntimeTabs().
		Select(RuntimeTabTimelineLanes).
		WithRows(RuntimeTabTimelineLanes, []string{
			"timeline lanes: ready",
			"replay workbench session selection: status=current mode=metadata-only session=/tmp/ouro/replay/workbench-sessions/current.json primary=run-primary selected=run-primary runs=1 filters=0 endpoints=8 superseded=1 selected_by=alice selected_at=2026-06-01T02:42:00Z reason=current investigation focus",
			"replay workbench session artifact: status=saved mode=metadata-only primary=run-primary selected=run-primary runs=1 filters=0 endpoints=8 created_by=operator created_at=2026-06-01T02:40:00Z reason=current decision=unreviewed reviewer=unknown reviewed_at=unknown review_reason=none current=true path=/tmp/ouro/replay/workbench-sessions/current.json",
		}).
		SelectRow(1)
	called := false
	model := NewModel().
		WithGraphStack(GraphStack{RuntimeTabs: tabs}).
		WithRuntimeActionExecutor(func(action RuntimeAction) (RuntimeActionExecutionResult, error) {
			called = true
			if action.Kind != "replay-workbench-session-selection" || action.State != "current" || action.ArtifactPath != "/tmp/ouro/replay/workbench-sessions/current.json" {
				t.Fatalf("unexpected executed action: %#v", action)
			}
			return RuntimeActionExecutionResult{
				Status: "completed",
				Path:   "/tmp/ouro/replay/workbench-session-selection.json",
				Output: "session selection cleared",
				RuntimeRows: []string{
					"replay workbench sessions: total=1 saved=1 dry-run=0 annotated=0",
					"replay workbench session selection: status=cleared mode=metadata-only session=/tmp/ouro/replay/workbench-sessions/current.json primary=run-primary selected=run-primary runs=1 filters=0 endpoints=8 superseded=1 cleared_by=tui cleared_at=2026-06-01T02:45:00Z reason=cleared from TUI",
					"replay workbench session artifact: status=saved mode=metadata-only primary=run-primary selected=run-primary runs=1 filters=0 endpoints=8 created_by=operator created_at=2026-06-01T02:40:00Z reason=current decision=unreviewed reviewer=unknown reviewed_at=unknown review_reason=none current=false path=/tmp/ouro/replay/workbench-sessions/current.json",
				},
			}, nil
		})

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if cmd != nil {
		t.Fatalf("first x should only arm replay workbench session selection clear confirmation")
	}
	if called {
		t.Fatalf("session selection clear executor must not run on first x")
	}
	next, ok := updated.(Model)
	if !ok {
		t.Fatalf("expected Model, got %T", updated)
	}
	action, ok := next.graphStack.RuntimeTabs.ActiveRuntimeAction()
	if !ok || action.ExecutionStatus != "confirm-required" {
		t.Fatalf("expected confirm-required session selection action, ok=%v action=%#v", ok, action)
	}
	view := next.graphStack.RuntimeTabs.Render(160, 16)
	for _, want := range []string{
		"action: replay workbench session selection selected /tmp/ouro/replay/workbench-sessions/current.json",
		"execution status: confirm-required",
		"execution confirm: press x again",
		"action mode: metadata-only",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("session selection clear confirmation view missing %q:\n%s", want, view)
		}
	}

	updated, cmd = next.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if cmd == nil {
		t.Fatalf("second x should return session selection clear command")
	}
	if called {
		t.Fatalf("session selection clear executor should run only when returned command is executed")
	}
	msg := cmd()
	if !called {
		t.Fatalf("expected session selection clear executor to run when command executes")
	}
	updated, cmd = updated.Update(msg)
	if cmd != nil {
		t.Fatalf("expected session selection clear result update to be synchronous")
	}
	next, ok = updated.(Model)
	if !ok {
		t.Fatalf("expected Model, got %T", updated)
	}
	action, ok = next.graphStack.RuntimeTabs.ActiveRuntimeAction()
	if !ok || action.ExecutionStatus != "completed" || action.ExecutionPath != "/tmp/ouro/replay/workbench-session-selection.json" || action.Output != "session selection cleared" {
		t.Fatalf("expected completed session selection clear metadata, ok=%v action=%#v", ok, action)
	}
	selected, ok := next.graphStack.RuntimeTabs.ActiveSelectedRow()
	if !ok || !strings.HasPrefix(strings.TrimSpace(selected), "replay workbench session selection: status=cleared") {
		t.Fatalf("expected cleared session selection row to be selected, ok=%v row=%q", ok, selected)
	}
	if strings.Contains(next.graphStack.RuntimeTabs.Render(180, 20), "current=true") {
		t.Fatalf("cleared session selection refresh must not leave stale current=true rows")
	}
}

func TestModelUpdateSelectsReplayWorkbenchSessionArtifactOnlyAfterSecondConfirm(t *testing.T) {
	tabs := DefaultRuntimeTabs().
		Select(RuntimeTabTimelineLanes).
		WithRows(RuntimeTabTimelineLanes, []string{
			"timeline lanes: ready",
			"replay workbench session artifact: status=saved mode=metadata-only primary=run-primary selected=run-primary runs=1 filters=0 endpoints=8 created_by=operator created_at=2026-06-01T02:40:00Z reason=baseline decision=unreviewed reviewer=unknown reviewed_at=unknown review_reason=none current=false path=/tmp/ouro/replay/workbench-sessions/candidate.json",
		}).
		SelectRow(1)
	called := false
	model := NewModel().
		WithGraphStack(GraphStack{RuntimeTabs: tabs}).
		WithRuntimeActionExecutor(func(action RuntimeAction) (RuntimeActionExecutionResult, error) {
			called = true
			if action.Kind != "replay-workbench-session-artifact" || action.ArtifactPath != "/tmp/ouro/replay/workbench-sessions/candidate.json" {
				t.Fatalf("unexpected executed action: %#v", action)
			}
			return RuntimeActionExecutionResult{
				Status: "completed",
				Path:   "/tmp/ouro/replay/workbench-session-selection.json",
				Output: "session selected",
				RuntimeRows: []string{
					"replay workbench sessions: total=1 saved=1 dry-run=0 annotated=0",
					"replay workbench session selection: status=current mode=metadata-only session=/tmp/ouro/replay/workbench-sessions/candidate.json primary=run-primary selected=run-primary runs=1 filters=0 endpoints=8 superseded=0 selected_by=tui selected_at=2026-06-01T02:42:00Z reason=selected from TUI",
					"replay workbench session artifact: status=saved mode=metadata-only primary=run-primary selected=run-primary runs=1 filters=0 endpoints=8 created_by=operator created_at=2026-06-01T02:40:00Z reason=baseline decision=unreviewed reviewer=unknown reviewed_at=unknown review_reason=none current=true path=/tmp/ouro/replay/workbench-sessions/candidate.json",
				},
			}, nil
		})

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if cmd != nil {
		t.Fatalf("first x should only arm replay workbench session selection confirmation")
	}
	if called {
		t.Fatalf("session selection executor must not run on first x")
	}
	next, ok := updated.(Model)
	if !ok {
		t.Fatalf("expected Model, got %T", updated)
	}
	action, ok := next.graphStack.RuntimeTabs.ActiveRuntimeAction()
	if !ok || action.ExecutionStatus != "confirm-required" {
		t.Fatalf("expected confirm-required session artifact action, ok=%v action=%#v", ok, action)
	}

	updated, cmd = next.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if cmd == nil {
		t.Fatalf("second x should return session selection command")
	}
	msg := cmd()
	if !called {
		t.Fatalf("expected session selection executor to run when command executes")
	}
	updated, cmd = updated.Update(msg)
	if cmd != nil {
		t.Fatalf("expected session selection result update to be synchronous")
	}
	next, ok = updated.(Model)
	if !ok {
		t.Fatalf("expected Model, got %T", updated)
	}
	action, ok = next.graphStack.RuntimeTabs.ActiveRuntimeAction()
	if !ok || action.ExecutionStatus != "completed" || action.ExecutionPath != "/tmp/ouro/replay/workbench-session-selection.json" || action.Output != "session selected" {
		t.Fatalf("expected completed session selection metadata, ok=%v action=%#v", ok, action)
	}
	selected, ok := next.graphStack.RuntimeTabs.ActiveSelectedRow()
	if !ok || !strings.HasPrefix(strings.TrimSpace(selected), "replay workbench session selection: status=current") || !strings.Contains(selected, "/tmp/ouro/replay/workbench-sessions/candidate.json") {
		t.Fatalf("expected current replay workbench session selection row to be selected, ok=%v row=%q", ok, selected)
	}
}

func TestModelUpdateShowsSelectedInstallExecutionArtifactDetails(t *testing.T) {
	tabs := DefaultRuntimeTabs().
		Select(RuntimeTabToolchainHealth).
		WithRows(RuntimeTabToolchainHealth, []string{
			"toolchain: ready",
			"execution artifact: /tmp/ouro/execution.json status=completed mode=execute dispatch=/tmp/ouro/dispatch.json results=1 skipped=0",
		}).
		SelectRow(1)
	model := NewModel().WithGraphStack(GraphStack{RuntimeTabs: tabs})

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	if cmd != nil {
		t.Fatalf("expected execution artifact details update to be synchronous")
	}
	next, ok := updated.(Model)
	if !ok {
		t.Fatalf("expected Model, got %T", updated)
	}
	if next.graphStack.Focus != GraphFocusRuntimeAction {
		t.Fatalf("expected runtime action focus, got %s", next.graphStack.Focus)
	}
	view := next.graphStack.Render(120, 24)
	for _, want := range []string{
		"[Node Inspector]",
		"runtime action: install-execution-artifact",
		"artifact path: /tmp/ouro/execution.json",
		"dispatch path: /tmp/ouro/dispatch.json",
		"state: completed",
		"execution mode: execute",
		"results: 1",
		"skipped: 0",
		"mode: read-only",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("execution artifact details missing %q:\n%s", want, view)
		}
	}
}

func TestModelUpdateShowsSelectedWorkerQueueDispatchResultArtifactDetails(t *testing.T) {
	tabs := DefaultRuntimeTabs().
		Select(RuntimeTabAgentRuntime).
		WithRows(RuntimeTabAgentRuntime, []string{
			"worker queue dispatch results: total=1 completed=1 failed=0",
			"worker queue dispatch result artifact: /tmp/ouro/result.json status=completed mode=metadata-only dispatch=/tmp/ouro/dispatch.json worker=remote-web-1 execution=tool-web",
		}).
		SelectRow(1)
	model := NewModel().WithGraphStack(GraphStack{RuntimeTabs: tabs})

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	if cmd != nil {
		t.Fatalf("expected worker queue dispatch result details update to be synchronous")
	}
	next, ok := updated.(Model)
	if !ok {
		t.Fatalf("expected Model, got %T", updated)
	}
	if next.graphStack.Focus != GraphFocusRuntimeAction {
		t.Fatalf("expected runtime action focus, got %s", next.graphStack.Focus)
	}
	view := next.graphStack.Render(128, 24)
	for _, want := range []string{
		"[Node Inspector]",
		"runtime action: worker-queue-dispatch-result-artifact",
		"artifact path: /tmp/ouro/result.json",
		"dispatch path: /tmp/ouro/dispatch.json",
		"source: remote-web-1",
		"name: tool-web",
		"state: completed",
		"execution mode: metadata-only",
		"mode: read-only",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("worker queue dispatch result details missing %q:\n%s", want, view)
		}
	}
}

func TestModelUpdateShowsSelectedWorkerDispatchRetryPlanArtifactDetails(t *testing.T) {
	tabs := DefaultRuntimeTabs().
		Select(RuntimeTabAgentRuntime).
		WithRows(RuntimeTabAgentRuntime, []string{
			"worker dispatch retry plans: total=1 ready=1 skipped=1",
			"worker dispatch retry plan artifact: /tmp/ouro/retry-plan.json status=dry-run mode=metadata-only ready=1 skipped=1",
		}).
		SelectRow(1)
	model := NewModel().WithGraphStack(GraphStack{RuntimeTabs: tabs})

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	if cmd != nil {
		t.Fatalf("expected worker dispatch retry plan details update to be synchronous")
	}
	next, ok := updated.(Model)
	if !ok {
		t.Fatalf("expected Model, got %T", updated)
	}
	if next.graphStack.Focus != GraphFocusRuntimeAction {
		t.Fatalf("expected runtime action focus, got %s", next.graphStack.Focus)
	}
	view := next.graphStack.Render(128, 24)
	for _, want := range []string{
		"[Node Inspector]",
		"runtime action: worker-dispatch-retry-plan-artifact",
		"artifact path: /tmp/ouro/retry-plan.json",
		"state: dry-run",
		"execution mode: metadata-only",
		"ready: 1",
		"skipped: 1",
		"mode: read-only",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("worker dispatch retry plan details missing %q:\n%s", want, view)
		}
	}
}

func TestModelUpdateShowsSelectedWorkerDispatchRetryDispatchArtifactDetails(t *testing.T) {
	tabs := DefaultRuntimeTabs().
		Select(RuntimeTabAgentRuntime).
		WithRows(RuntimeTabAgentRuntime, []string{
			"worker dispatch retry dispatches: total=1 ready=1 skipped=0",
			"worker dispatch retry dispatch artifact: /tmp/ouro/retry-dispatch.json status=dry-run mode=metadata-only retry_plan=/tmp/ouro/retry-plan.json ready=1 skipped=0",
		}).
		SelectRow(1)
	model := NewModel().WithGraphStack(GraphStack{RuntimeTabs: tabs})

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	if cmd != nil {
		t.Fatalf("expected worker dispatch retry dispatch details update to be synchronous")
	}
	next, ok := updated.(Model)
	if !ok {
		t.Fatalf("expected Model, got %T", updated)
	}
	if next.graphStack.Focus != GraphFocusRuntimeAction {
		t.Fatalf("expected runtime action focus, got %s", next.graphStack.Focus)
	}
	view := next.graphStack.Render(144, 36)
	for _, want := range []string{
		"[Node Inspector]",
		"runtime action: worker-dispatch-retry-dispatch-artifact",
		"artifact path: /tmp/ouro/retry-dispatch.json",
		"dispatch path: /tmp/ouro/retry-plan.json",
		"state: dry-run",
		"execution mode: metadata-only",
		"ready: 1",
		"skipped: 0",
		"mode: read-only",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("worker dispatch retry dispatch details missing %q:\n%s", want, view)
		}
	}
}

func TestModelUpdateShowsSelectedWorkerDispatchRetryDispatchExecutionDetails(t *testing.T) {
	tabs := DefaultRuntimeTabs().
		Select(RuntimeTabAgentRuntime).
		WithRows(RuntimeTabAgentRuntime, []string{
			"worker dispatch retry executions: total=1 claimed=1 dispatched=1 skipped=0 failed=0",
			"worker dispatch retry execution artifact: /tmp/ouro/retry-execution.json status=completed mode=execute retry_dispatch=/tmp/ouro/retry-dispatch.json claimed=1 dispatched=1 skipped=0 failed=0",
			"worker dispatch retry execution result: source_result=/tmp/ouro/source-result.json source_dispatch=/tmp/ouro/source-dispatch.json source_claim=/tmp/ouro/source-claim.json retry_claim=/tmp/ouro/retry-claim.json queue_dispatch=/tmp/ouro/retry-queue-dispatch.json request_path=/tmp/ouro/retry-request.json transport=file-outbox worker=remote-web-1 run=run-one execution=tool-one status=dispatched reason=retry-queue-claim-dispatched",
		}).
		SelectRow(1)
	model := NewModel().WithGraphStack(GraphStack{RuntimeTabs: tabs})

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	if cmd != nil {
		t.Fatalf("expected worker dispatch retry execution details update to be synchronous")
	}
	next, ok := updated.(Model)
	if !ok {
		t.Fatalf("expected Model, got %T", updated)
	}
	if next.graphStack.Focus != GraphFocusRuntimeAction {
		t.Fatalf("expected runtime action focus, got %s", next.graphStack.Focus)
	}
	view := next.graphStack.Render(144, 24)
	for _, want := range []string{
		"[Node Inspector]",
		"runtime action: worker-dispatch-retry-dispatch-execution-artifact",
		"artifact path: /tmp/ouro/retry-execution.json",
		"retry dispatch path: /tmp/ouro/retry-dispatch.json",
		"state: completed",
		"execution mode: execute",
		"claimed: 1",
		"skipped: 0",
		"failed: 0",
		"mode: read-only",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("worker dispatch retry execution details missing %q:\n%s", want, view)
		}
	}

	tabs = tabs.SelectRow(2)
	model = NewModel().WithGraphStack(GraphStack{RuntimeTabs: tabs})
	updated, cmd = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	if cmd != nil {
		t.Fatalf("expected worker dispatch retry execution result details update to be synchronous")
	}
	next = updated.(Model)
	view = next.graphStack.Render(144, 36)
	for _, want := range []string{
		"runtime action: worker-dispatch-retry-dispatch-execution-result",
		"artifact path: /tmp/ouro/retry-claim.json",
		"source result path: /tmp/ouro/source-result.json",
		"dispatch path: /tmp/ouro/source-dispatch.json",
		"transport: file-outbox",
		"execution path: /tmp/ouro/retry-queue-dispatch.json",
		"request path: /tmp/ouro/retry-request.json",
		"claim path: /tmp/ouro/source-claim.json",
		"worker: remote-web-1",
		"run: run-one",
		"execution: tool-one",
		"state: dispatched",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("worker dispatch retry execution result details missing %q:\n%s", want, view)
		}
	}
}

func TestModelUpdateShowsSelectedWorkerTransportDaemonControlArtifactDetails(t *testing.T) {
	tabs := DefaultRuntimeTabs().
		Select(RuntimeTabAgentRuntime).
		WithRows(RuntimeTabAgentRuntime, []string{
			"worker transport daemon controls: total=1 pending=1 stop=0 restart=1 applied=0",
			"worker transport daemon control artifact: /tmp/ouro/daemon-control.json status=pending worker=remote-web-1 action=restart mode=metadata-only",
		}).
		SelectRow(1)
	model := NewModel().WithGraphStack(GraphStack{RuntimeTabs: tabs})

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	if cmd != nil {
		t.Fatalf("expected worker transport daemon control details update to be synchronous")
	}
	next, ok := updated.(Model)
	if !ok {
		t.Fatalf("expected Model, got %T", updated)
	}
	if next.graphStack.Focus != GraphFocusRuntimeAction {
		t.Fatalf("expected runtime action focus, got %s", next.graphStack.Focus)
	}
	view := next.graphStack.Render(132, 24)
	for _, want := range []string{
		"[Node Inspector]",
		"runtime action: worker-transport-daemon-control-artifact",
		"artifact path: /tmp/ouro/daemon-control.json",
		"name: remote-web-1",
		"state: pending",
		"execution mode: metadata-only",
		"control action: restart",
		"mode: read-only",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("worker transport daemon control details missing %q:\n%s", want, view)
		}
	}
}

func TestModelUpdateShowsSelectedWorkerTransportDaemonSupervisorArtifactDetails(t *testing.T) {
	tabs := DefaultRuntimeTabs().
		Select(RuntimeTabAgentRuntime).
		WithRows(RuntimeTabAgentRuntime, []string{
			"worker transport daemon supervisors: total=1 completed=1 stopped=0 restart-limit=0 failed=0",
			"worker transport daemon supervisor artifact: /tmp/ouro/daemon-supervisor.json status=completed worker=remote-web-1 source=request-command mode=daemon-supervisor-run",
		}).
		SelectRow(1)
	model := NewModel().WithGraphStack(GraphStack{RuntimeTabs: tabs})

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	if cmd != nil {
		t.Fatalf("expected worker transport daemon supervisor details update to be synchronous")
	}
	next, ok := updated.(Model)
	if !ok {
		t.Fatalf("expected Model, got %T", updated)
	}
	if next.graphStack.Focus != GraphFocusRuntimeAction {
		t.Fatalf("expected runtime action focus, got %s", next.graphStack.Focus)
	}
	view := next.graphStack.Render(144, 24)
	for _, want := range []string{
		"[Node Inspector]",
		"runtime action: worker-transport-daemon-supervisor-artifact",
		"artifact path: /tmp/ouro/daemon-supervisor.json",
		"name: remote-web-1",
		"source: request-command",
		"state: completed",
		"execution mode: daemon-supervisor-run",
		"mode: read-only",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("worker transport daemon supervisor details missing %q:\n%s", want, view)
		}
	}
}

func TestModelUpdateShowsSelectedWorkerOrchestrationArtifactDetails(t *testing.T) {
	tabs := DefaultRuntimeTabs().
		Select(RuntimeTabAgentRuntime).
		WithRows(RuntimeTabAgentRuntime, []string{
			"worker orchestrations: total=1 completed=1 dry-run=0 skipped=0 failed=0",
			"worker orchestration artifact: /tmp/ouro/worker-orchestration.json status=completed mode=execute transport=metadata-only placement_ready=3 placement_skipped=0 claimed=2 dispatch_ready=2 dispatch_skipped=0 dispatched=2 failed=0 placement_plan=/tmp/ouro/placement.json placement_dispatch=/tmp/ouro/dispatch.json placement_batch=/tmp/ouro/batch.json lifecycle_maintenance=/tmp/ouro/maintenance.json lifecycle_audit=/tmp/ouro/audit.json",
		}).
		SelectRow(1)
	model := NewModel().WithGraphStack(GraphStack{RuntimeTabs: tabs})

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	if cmd != nil {
		t.Fatalf("expected worker orchestration details update to be synchronous")
	}
	next, ok := updated.(Model)
	if !ok {
		t.Fatalf("expected Model, got %T", updated)
	}
	if next.graphStack.Focus != GraphFocusRuntimeAction {
		t.Fatalf("expected runtime action focus, got %s", next.graphStack.Focus)
	}
	view := next.graphStack.Render(150, 40)
	for _, want := range []string{
		"[Node Inspector]",
		"runtime action: worker-orchestration-artifact",
		"artifact path: /tmp/ouro/worker-orchestration.json",
		"state: completed",
		"execution mode: execute",
		"transport: metadata-only",
		"ready: 3",
		"skipped: 0",
		"claimed: 2",
		"results: 2",
		"failed: 0",
		"placement plan path: /tmp/ouro/placement.json",
		"placement dispatch plan path: /tmp/ouro/dispatch.json",
		"placement dispatch batch path: /tmp/ouro/batch.json",
		"lifecycle maintenance path: /tmp/ouro/maintenance.json",
		"lifecycle audit path: /tmp/ouro/audit.json",
		"mode: read-only",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("worker orchestration details missing %q:\n%s", want, view)
		}
	}
}

func TestRuntimeWorkerPlacementArtifactUsesEncodedReferenceFields(t *testing.T) {
	artifactPath := "/tmp/ouro/runtime/worker placement plans/plan status=failed mode=execute.json"
	row := "worker placement artifact: " + artifactPath +
		" artifact_b64=" + base64.RawURLEncoding.EncodeToString([]byte(artifactPath)) +
		" status=dry-run mode=metadata-only ready=2 skipped=6 run=run-placement"

	action := runtimeWorkerPlacementArtifactActionForRow(row)

	if action.ArtifactPath != artifactPath {
		t.Fatalf("expected encoded placement artifact path, got %#v", action)
	}
	if action.State != "dry-run" || action.ExecutionMode != "metadata-only" || action.Ready != "2" || action.Skipped != "6" {
		t.Fatalf("encoded placement artifact must not retarget machine fields: %#v", action)
	}
}

func TestRuntimeWorkerPlacementDispatchArtifactUsesEncodedReferenceFields(t *testing.T) {
	artifactPath := "/tmp/ouro/runtime/worker placement dispatch plans/dispatch status=failed mode=execute.json"
	row := "worker placement dispatch artifact: " + artifactPath +
		" artifact_b64=" + base64.RawURLEncoding.EncodeToString([]byte(artifactPath)) +
		" status=dry-run mode=metadata-only transport=file-outbox ready=1 skipped=0"

	action := runtimeWorkerPlacementDispatchArtifactActionForRow(row)

	if action.ArtifactPath != artifactPath {
		t.Fatalf("expected encoded placement dispatch artifact path, got %#v", action)
	}
	if action.State != "dry-run" || action.ExecutionMode != "metadata-only" || action.Source != "file-outbox" || action.Ready != "1" || action.Skipped != "0" {
		t.Fatalf("encoded placement dispatch artifact must not retarget machine fields: %#v", action)
	}
}

func TestRuntimeWorkerOrchestrationArtifactUsesEncodedPlacementReferenceFields(t *testing.T) {
	artifactPath := "/tmp/ouro/runtime/worker orchestrations/orchestration status=failed.json"
	placementPlanPath := "/tmp/ouro/runtime/worker placement plans/plan status=failed.json"
	placementDispatchPath := "/tmp/ouro/runtime/worker placement dispatch plans/dispatch mode=execute.json"
	placementBatchPath := "/tmp/ouro/runtime/worker placement dispatch batches/batch transport=http-json.json"
	lifecycleMaintenancePath := "/tmp/ouro/runtime/worker lifecycle maintenances/maintenance status=failed.json"
	lifecycleAuditPath := "/tmp/ouro/runtime/worker lifecycle audits/audit mode=execute.json"
	row := "worker orchestration artifact: " + artifactPath +
		" artifact_b64=" + base64.RawURLEncoding.EncodeToString([]byte(artifactPath)) +
		" status=completed mode=execute transport=file-outbox placement_ready=3 placement_skipped=0 claimed=2 dispatch_ready=2 dispatch_skipped=0 dispatched=2 failed=0" +
		" placement_plan=" + placementPlanPath +
		" placement_plan_b64=" + base64.RawURLEncoding.EncodeToString([]byte(placementPlanPath)) +
		" placement_dispatch=" + placementDispatchPath +
		" placement_dispatch_b64=" + base64.RawURLEncoding.EncodeToString([]byte(placementDispatchPath)) +
		" placement_batch=" + placementBatchPath +
		" placement_batch_b64=" + base64.RawURLEncoding.EncodeToString([]byte(placementBatchPath)) +
		" lifecycle_maintenance=" + lifecycleMaintenancePath +
		" lifecycle_maintenance_b64=" + base64.RawURLEncoding.EncodeToString([]byte(lifecycleMaintenancePath)) +
		" lifecycle_audit=" + lifecycleAuditPath +
		" lifecycle_audit_b64=" + base64.RawURLEncoding.EncodeToString([]byte(lifecycleAuditPath))

	action := runtimeWorkerOrchestrationArtifactActionForRow(row)

	if action.ArtifactPath != artifactPath || action.PlacementPlanPath != placementPlanPath || action.PlacementDispatchPlanPath != placementDispatchPath || action.PlacementDispatchBatchPath != placementBatchPath {
		t.Fatalf("expected encoded orchestration placement references, got %#v", action)
	}
	if action.LifecycleMaintenancePath != lifecycleMaintenancePath || action.LifecycleAuditPath != lifecycleAuditPath {
		t.Fatalf("expected encoded orchestration lifecycle references, got %#v", action)
	}
	if action.State != "completed" || action.ExecutionMode != "execute" || action.TransportMode != "file-outbox" || action.Ready != "3" || action.Results != "2" {
		t.Fatalf("encoded orchestration references must not retarget machine fields: %#v", action)
	}
}

func TestRuntimeWorkerOrchestrationSupervisorArtifactUsesEncodedReferenceFields(t *testing.T) {
	artifactPath := "/tmp/ouro/runtime/worker orchestration supervisors/supervisor status=failed.json"
	orchestrationPath := "/tmp/ouro/runtime/worker orchestrations/orchestration mode=execute.json"
	row := "worker orchestration supervisor artifact: " + artifactPath +
		" artifact_b64=" + base64.RawURLEncoding.EncodeToString([]byte(artifactPath)) +
		" status=completed mode=metadata-only transport=metadata-only runs=2 max_runs=2 orchestrations_created=2 lifecycle_maintenance=2 lifecycle_audit=2 claimed=2 dispatched=2 skipped=0 failed=0" +
		" orchestration_path=" + orchestrationPath +
		" orchestration_path_b64=" + base64.RawURLEncoding.EncodeToString([]byte(orchestrationPath))

	action := runtimeWorkerOrchestrationSupervisorArtifactActionForRow(row)

	if action.ArtifactPath != artifactPath || action.DispatchPath != orchestrationPath {
		t.Fatalf("expected encoded orchestration supervisor references, got %#v", action)
	}
	if action.State != "completed" || action.ExecutionMode != "metadata-only" || action.Ready != "2" || action.Failed != "0" {
		t.Fatalf("encoded orchestration supervisor references must not retarget machine fields: %#v", action)
	}
}

func TestModelUpdateShowsSelectedWorkerOrchestrationSupervisorArtifactDetails(t *testing.T) {
	tabs := DefaultRuntimeTabs().
		Select(RuntimeTabAgentRuntime).
		WithRows(RuntimeTabAgentRuntime, []string{
			"worker orchestration supervisors: total=1 completed=1 dry-run=0 failed=0",
			"worker orchestration supervisor artifact: /tmp/ouro/orchestration-supervisor.json status=completed mode=metadata-only transport=metadata-only runs=2 max_runs=2 orchestrations_created=2 lifecycle_maintenance=2 lifecycle_audit=2 retry_dispatch_executions_created=1 retry_dispatch_claims_created=1 claimed=2 dispatched=2 skipped=0 failed=0 orchestration_path=/tmp/ouro/orchestration.json",
		}).
		SelectRow(1)
	model := NewModel().WithGraphStack(GraphStack{RuntimeTabs: tabs})

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	if cmd != nil {
		t.Fatalf("expected worker orchestration supervisor details update to be synchronous")
	}
	next, ok := updated.(Model)
	if !ok {
		t.Fatalf("expected Model, got %T", updated)
	}
	if next.graphStack.Focus != GraphFocusRuntimeAction {
		t.Fatalf("expected runtime action focus, got %s", next.graphStack.Focus)
	}
	view := next.graphStack.Render(160, 40)
	for _, want := range []string{
		"[Node Inspector]",
		"runtime action: worker-orchestration-supervisor-artifact",
		"artifact path: /tmp/ouro/orchestration-supervisor.json",
		"state: completed",
		"execution mode: metadata-only",
		"transport: metadata-only",
		"ready: 2",
		"results: max_runs=2 orchestrations_created=2 lifecycle_maintenance=2 lifecycle_audit=2",
		"retry_dispatch_executions_created=1",
		"retry_dispatch_claims_created=1",
		"skipped: 0",
		"failed: 0",
		"dispatch path: /tmp/ouro/orchestration.json",
		"mode: read-only",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("worker orchestration supervisor details missing %q:\n%s", want, view)
		}
	}
}

func TestModelUpdateShowsSelectedInstallExecutionResultDetails(t *testing.T) {
	tabs := DefaultRuntimeTabs().
		Select(RuntimeTabToolchainHealth).
		WithRows(RuntimeTabToolchainHealth, []string{
			"toolchain: ready",
			"execution result web-api-pack/missing-httpx: completed command=sh -c echo ok output=ok artifact=/tmp/ouro/execution.json dispatch=/tmp/ouro/dispatch.json",
		}).
		SelectRow(1)
	model := NewModel().WithGraphStack(GraphStack{RuntimeTabs: tabs})

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	if cmd != nil {
		t.Fatalf("expected execution result details update to be synchronous")
	}
	next, ok := updated.(Model)
	if !ok {
		t.Fatalf("expected Model, got %T", updated)
	}
	view := next.graphStack.Render(120, 24)
	for _, want := range []string{
		"runtime action: install-execution-result",
		"pack: web-api-pack",
		"name: missing-httpx",
		"state: completed",
		"command: sh -c echo ok",
		"output: ok",
		"artifact path: /tmp/ouro/execution.json",
		"dispatch path: /tmp/ouro/dispatch.json",
		"mode: read-only",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("execution result details missing %q:\n%s", want, view)
		}
	}
}

func TestRuntimeInstallExecutionResultUsesEncodedOutputText(t *testing.T) {
	output := "install output output_b64=fake artifact=/tmp/ouro/evil-execution.json dispatch=/tmp/ouro/evil-dispatch.json command=evil"
	row := "execution result web-api-pack/missing-httpx: completed artifact=/tmp/ouro/execution.json dispatch=/tmp/ouro/dispatch.json command=sh -c echo ok output=" + output + " output_b64=" + base64.RawURLEncoding.EncodeToString([]byte(output))

	action := runtimeExecutionResultActionForRow(row)
	if action.ArtifactPath != "/tmp/ouro/execution.json" || action.DispatchPath != "/tmp/ouro/dispatch.json" || action.Command != "sh -c echo ok" {
		t.Fatalf("encoded install execution output must not retarget machine fields: %#v", action)
	}
	if action.Output != output {
		t.Fatalf("expected encoded install execution output to be decoded, got %q", action.Output)
	}
}

func TestRuntimeInstallDispatchArtifactUsesEncodedReferenceFields(t *testing.T) {
	artifactPath := "/tmp/ouro/toolchain/install dispatches/dispatch status=failed.json"
	row := "dispatch artifact: " + runtimeCompactRowTextValue(artifactPath) +
		" artifact_b64=" + base64.RawURLEncoding.EncodeToString([]byte(artifactPath)) +
		" status=dry-run mode=metadata-only ready=1 skipped=0"

	action := runtimeDispatchArtifactActionForRow(row)
	if action.ArtifactPath != artifactPath {
		t.Fatalf("expected encoded install dispatch artifact path, got %#v", action)
	}
	if action.State != "dry-run" || action.ExecutionMode != "metadata-only" || action.Ready != "1" || action.Skipped != "0" {
		t.Fatalf("encoded install dispatch artifact path must not retarget metadata: %#v", action)
	}
}

func TestRuntimeInstallExecutionArtifactUsesEncodedReferenceFields(t *testing.T) {
	artifactPath := "/tmp/ouro/toolchain/install executions/execution status=failed.json"
	dispatchPath := "/tmp/ouro/toolchain/install dispatches/dispatch mode=execute.json"
	row := "execution artifact: " + runtimeCompactRowTextValue(artifactPath) +
		" artifact_b64=" + base64.RawURLEncoding.EncodeToString([]byte(artifactPath)) +
		" status=completed mode=execute dispatch=" + runtimeCompactRowTextValue(dispatchPath) +
		" dispatch_b64=" + base64.RawURLEncoding.EncodeToString([]byte(dispatchPath)) +
		" results=1 skipped=0"

	action := runtimeExecutionArtifactActionForRow(row)
	if action.ArtifactPath != artifactPath || action.DispatchPath != dispatchPath {
		t.Fatalf("expected encoded install execution artifact references, got %#v", action)
	}
	if action.State != "completed" || action.ExecutionMode != "execute" || action.Results != "1" || action.Skipped != "0" {
		t.Fatalf("encoded install execution artifact references must not retarget metadata: %#v", action)
	}
}

func TestRuntimeInstallExecutionResultUsesEncodedReferenceFields(t *testing.T) {
	artifactPath := "/tmp/ouro/toolchain/install executions/execution status=failed.json"
	dispatchPath := "/tmp/ouro/toolchain/install dispatches/dispatch mode=execute.json"
	command := "sh -c echo artifact=/tmp/ouro/evil-execution.json dispatch=/tmp/ouro/evil-dispatch.json"
	output := "install output artifact=/tmp/ouro/evil-execution.json dispatch=/tmp/ouro/evil-dispatch.json"
	row := "execution result web-api-pack/missing-httpx: completed output=" + runtimeCompactRowTextValue(output) +
		" output_b64=" + base64.RawURLEncoding.EncodeToString([]byte(output)) +
		" artifact=" + runtimeCompactRowTextValue(artifactPath) +
		" artifact_b64=" + base64.RawURLEncoding.EncodeToString([]byte(artifactPath)) +
		" dispatch=" + runtimeCompactRowTextValue(dispatchPath) +
		" dispatch_b64=" + base64.RawURLEncoding.EncodeToString([]byte(dispatchPath)) +
		" command=" + runtimeCompactRowTextValue(command) +
		" command_b64=" + base64.RawURLEncoding.EncodeToString([]byte(command))

	action := runtimeExecutionResultActionForRow(row)
	if action.ArtifactPath != artifactPath || action.DispatchPath != dispatchPath {
		t.Fatalf("expected encoded install execution result references, got %#v", action)
	}
	if action.State != "completed" || action.Output != output || action.Command != command {
		t.Fatalf("encoded install execution result references must not retarget text/metadata: %#v", action)
	}
}

func TestRuntimeInstallExecutionResultUsesEncodedCommandText(t *testing.T) {
	command := "sh -c echo output=evil artifact=/tmp/ouro/evil-execution.json dispatch=/tmp/ouro/evil-dispatch.json output_b64=" + base64.RawURLEncoding.EncodeToString([]byte("fake output"))
	output := "install output"
	row := "execution result web-api-pack/missing-httpx: completed output=" + output + " output_b64=" + base64.RawURLEncoding.EncodeToString([]byte(output)) + " artifact=/tmp/ouro/execution.json dispatch=/tmp/ouro/dispatch.json command=" + command + " command_b64=" + base64.RawURLEncoding.EncodeToString([]byte(command))

	action := runtimeExecutionResultActionForRow(row)
	if action.ArtifactPath != "/tmp/ouro/execution.json" || action.DispatchPath != "/tmp/ouro/dispatch.json" || action.Output != output {
		t.Fatalf("encoded install execution command must not retarget output or machine fields: %#v", action)
	}
	if action.Command != command {
		t.Fatalf("expected encoded install execution command to be decoded, got %q", action.Command)
	}
}

func TestModelUpdateShowsSelectedInstallExecutionResultDetailsDecodeEncodedOutput(t *testing.T) {
	output := "install output artifact=/tmp/ouro/evil-execution.json dispatch=/tmp/ouro/evil-dispatch.json command=evil"
	row := "execution result web-api-pack/missing-httpx: completed artifact=/tmp/ouro/execution.json dispatch=/tmp/ouro/dispatch.json command=sh -c echo ok output=" + output + " output_b64=" + base64.RawURLEncoding.EncodeToString([]byte(output))
	tabs := DefaultRuntimeTabs().
		Select(RuntimeTabToolchainHealth).
		WithRows(RuntimeTabToolchainHealth, []string{
			"toolchain: ready",
			row,
		}).
		SelectRow(1)
	model := NewModel().WithGraphStack(GraphStack{RuntimeTabs: tabs})

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	if cmd != nil {
		t.Fatalf("expected encoded execution result details update to be synchronous")
	}
	view := updated.(Model).graphStack.Render(160, 24)
	for _, want := range []string{
		"runtime action: install-execution-result",
		"output: " + output,
		"artifact path: /tmp/ouro/execution.json",
		"dispatch path: /tmp/ouro/dispatch.json",
		"command: sh -c echo ok",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("encoded execution result details missing %q:\n%s", want, view)
		}
	}
}

func TestModelUpdateShowsSelectedInstallRetryDispatchArtifactDetails(t *testing.T) {
	tabs := DefaultRuntimeTabs().
		Select(RuntimeTabToolchainHealth).
		WithRows(RuntimeTabToolchainHealth, []string{
			"toolchain: ready",
			"retry dispatch artifact: /tmp/ouro/retry-dispatch.json status=dry-run mode=metadata-only ready=1 skipped=0",
		}).
		SelectRow(1)
	model := NewModel().WithGraphStack(GraphStack{RuntimeTabs: tabs})

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	if cmd != nil {
		t.Fatalf("expected retry dispatch artifact details update to be synchronous")
	}
	next, ok := updated.(Model)
	if !ok {
		t.Fatalf("expected Model, got %T", updated)
	}
	if next.graphStack.Focus != GraphFocusRuntimeAction {
		t.Fatalf("expected runtime action focus, got %s", next.graphStack.Focus)
	}
	view := next.graphStack.Render(120, 24)
	for _, want := range []string{
		"[Node Inspector]",
		"runtime action: install-retry-dispatch-artifact",
		"artifact path: /tmp/ouro/retry-dispatch.json",
		"state: dry-run",
		"execution mode: metadata-only",
		"ready: 1",
		"skipped: 0",
		"mode: metadata-only",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("retry dispatch artifact details missing %q:\n%s", want, view)
		}
	}
}

func TestModelUpdateShowsSelectedWorkerLifecycleActionPlanArtifactDetails(t *testing.T) {
	tabs := DefaultRuntimeTabs().
		Select(RuntimeTabAgentRuntime).
		WithRows(RuntimeTabAgentRuntime, []string{
			"agents: 0",
			"worker lifecycle action plan artifact: /tmp/ouro/action-plan.json status=dry-run mode=metadata-only actions=2 ack=1 run=0 reclaim=1 reconcile=0 review=/tmp/ouro/review.json",
		}).
		SelectRow(1)
	model := NewModel().WithGraphStack(GraphStack{RuntimeTabs: tabs})

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	if cmd != nil {
		t.Fatalf("expected worker lifecycle action plan details update to be synchronous")
	}
	next, ok := updated.(Model)
	if !ok {
		t.Fatalf("expected Model, got %T", updated)
	}
	if next.graphStack.Focus != GraphFocusRuntimeAction {
		t.Fatalf("expected runtime action focus, got %s", next.graphStack.Focus)
	}
	view := next.graphStack.Render(150, 24)
	for _, want := range []string{
		"[Node Inspector]",
		"runtime action: worker-lifecycle-action-plan-artifact",
		"artifact path: /tmp/ouro/action-plan.json",
		"review path: /tmp/ouro/review.json",
		"state: dry-run",
		"execution mode: metadata-only",
		"ready: 2",
		"results: ack=1 run=0 reclaim=1 reconcile=0",
		"mode: read-only",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("worker lifecycle action plan artifact details missing %q:\n%s", want, view)
		}
	}
	if strings.Contains(view, "dispatch path: /tmp/ouro/review.json") {
		t.Fatalf("worker lifecycle action plan review path must not be labeled as dispatch path:\n%s", view)
	}
}

func TestRuntimeWorkerLifecycleActionPlanArtifactUsesEncodedReviewReasonText(t *testing.T) {
	reviewReason := "accepted after review current=false review=/tmp/ouro/evil-review.json decision=rejected reviewer=mallory reviewed_at=2030-01-01T00:00:00Z"
	row := "worker lifecycle action plan artifact: /tmp/ouro/action-plan.json status=dry-run mode=metadata-only actions=2 ack=1 run=0 reclaim=1 reconcile=0 daemon_control=0 retry_dispatch_execute=1 review=/tmp/ouro/review.json decision=accepted reviewer=alice reviewed_at=2026-06-02T09:12:00Z review_reason=" + reviewReason + " review_reason_b64=" + base64.RawURLEncoding.EncodeToString([]byte(reviewReason)) + " current=true"

	action := runtimeWorkerLifecycleActionPlanArtifactActionForRow(row)
	if action.ReviewPath != "/tmp/ouro/review.json" || action.Decision != "accepted" || action.Reviewer != "alice" || action.ReviewedAt != "2026-06-02T09:12:00Z" {
		t.Fatalf("encoded action-plan artifact review reason must not retarget review metadata: %#v", action)
	}
	if action.Current != "true" {
		t.Fatalf("encoded action-plan artifact review reason must not retarget current marker: %#v", action)
	}
	if action.ReviewReason != reviewReason {
		t.Fatalf("expected encoded action-plan artifact review reason to be decoded, got %q", action.ReviewReason)
	}
}

func TestRuntimeWorkerLifecycleActionPlanArtifactUsesEncodedReferenceFields(t *testing.T) {
	artifactPath := "/tmp/ouro/action plan status=failed mode=execute.json"
	reviewPath := "/tmp/ouro/review path decision=rejected.json"
	reviewer := "alice reviewed_at=2030-01-01T00:00:00Z current=false"
	reviewedAt := "2026-06-02T09:12:00Z reviewer=mallory"
	reviewReason := "accepted after review status=failed review=/tmp/ouro/evil-review.json"
	row := "worker lifecycle action plan artifact: " + artifactPath +
		" artifact_b64=" + base64.RawURLEncoding.EncodeToString([]byte(artifactPath)) +
		" status=dry-run mode=metadata-only actions=2 ack=1 run=0 reclaim=1 reconcile=0 daemon_control=0 retry_dispatch_execute=1 review=" + reviewPath +
		" review_b64=" + base64.RawURLEncoding.EncodeToString([]byte(reviewPath)) +
		" decision=accepted reviewer=" + reviewer +
		" reviewer_b64=" + base64.RawURLEncoding.EncodeToString([]byte(reviewer)) +
		" reviewed_at=" + reviewedAt +
		" reviewed_at_b64=" + base64.RawURLEncoding.EncodeToString([]byte(reviewedAt)) +
		" review_reason=" + reviewReason +
		" review_reason_b64=" + base64.RawURLEncoding.EncodeToString([]byte(reviewReason)) +
		" current=true"

	action := runtimeWorkerLifecycleActionPlanArtifactActionForRow(row)
	if action.ArtifactPath != artifactPath || action.ReviewPath != reviewPath {
		t.Fatalf("expected encoded action-plan artifact references, got %#v", action)
	}
	if action.State != "dry-run" || action.ExecutionMode != "metadata-only" || action.Decision != "accepted" || action.Current != "true" {
		t.Fatalf("encoded action-plan artifact text must not retarget machine fields: %#v", action)
	}
	if action.Reviewer != reviewer || action.ReviewedAt != reviewedAt || action.ReviewReason != reviewReason {
		t.Fatalf("expected encoded action-plan review metadata, got %#v", action)
	}
}

func TestModelUpdateShowsSelectedWorkerLifecycleMaintenanceArtifactDetails(t *testing.T) {
	row := "worker lifecycle maintenance artifact: /tmp/ouro/maintenance.json status=completed mode=metadata-only actions=6 ack=1 run=1 daemon_control=1 retry_dispatch_execute=1 reclaim=1 reconcile=1 skipped_unsafe=4 review_created=1 action_plan_created=1 reclaim_created=1 reconciliation_created=1 retry_dispatch_executions_created=1 retry_dispatch_claims_created=1 reclaim_executions_created=1 reclaim_results_created=1 reconciliation_executions_created=1 reconciliation_results_created=1 review_path=/tmp/ouro/review.json action_plan_path=/tmp/ouro/action-plan.json reclaim_path=/tmp/ouro/reclaim.json reconciliation_path=/tmp/ouro/reconcile.json"
	action, ok := runtimeActionForRow(row)
	if !ok {
		t.Fatalf("expected worker lifecycle maintenance row to parse")
	}
	for _, want := range []string{"reclaim_executions_created=1", "reconciliation_results_created=1"} {
		if !strings.Contains(action.Results, want) {
			t.Fatalf("maintenance action results missing %q: %#v", want, action)
		}
	}
	tabs := DefaultRuntimeTabs().
		Select(RuntimeTabAgentRuntime).
		WithRows(RuntimeTabAgentRuntime, []string{
			"agents: 0",
			row,
		}).
		SelectRow(1)
	model := NewModel().WithGraphStack(GraphStack{RuntimeTabs: tabs})

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	if cmd != nil {
		t.Fatalf("expected worker lifecycle maintenance details update to be synchronous")
	}
	next, ok := updated.(Model)
	if !ok {
		t.Fatalf("expected Model, got %T", updated)
	}
	if next.graphStack.Focus != GraphFocusRuntimeAction {
		t.Fatalf("expected runtime action focus, got %s", next.graphStack.Focus)
	}
	view := next.graphStack.Render(160, 40)
	for _, want := range []string{
		"[Node Inspector]",
		"runtime action: worker-lifecycle-maintenance-artifact",
		"artifact path: /tmp/ouro/maintenance.json",
		"dispatch path: /tmp/ouro/review.json",
		"request path: /tmp/ouro/action-plan.json",
		"lease path: /tmp/ouro/reclaim.json",
		"result path: /tmp/ouro/reconcile.json",
		"state: completed",
		"execution mode: metadata-only",
		"ready: 6",
		"skipped: 4",
		"results: ack=1 run=1 daemon_control=1 retry_dispatch_execute=1",
		"mode: read-only",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("worker lifecycle maintenance artifact details missing %q:\n%s", want, view)
		}
	}
}

func TestModelUpdateShowsWorkerLifecycleMaintenanceRetryHandoffDetails(t *testing.T) {
	row := "worker lifecycle maintenance artifact: /tmp/ouro/maintenance.json status=completed mode=metadata-only actions=1 retry_dispatch_execute=1 skipped_unsafe=0 retry_dispatch_executions_created=1 retry_dispatch_claims_created=1 retry_dispatch_claims_dispatched=1 retry_dispatch_transport_requests_created=1 dispatch_lifecycle_retry_claims=true lifecycle_retry_transport=file-outbox review_path=/tmp/ouro/review.json action_plan_path=/tmp/ouro/action-plan.json"
	action, ok := runtimeActionForRow(row)
	if !ok {
		t.Fatalf("expected worker lifecycle maintenance handoff row to parse")
	}
	for _, want := range []string{
		"retry_dispatch_claims_dispatched=1",
		"retry_dispatch_transport_requests_created=1",
		"dispatch_lifecycle_retry_claims=true",
		"lifecycle_retry_transport=file-outbox",
	} {
		if !strings.Contains(action.Results, want) {
			t.Fatalf("maintenance handoff action results missing %q: %#v", want, action)
		}
	}
}

func TestModelUpdateShowsSelectedWorkerLifecycleMaintenanceSupervisorArtifactDetails(t *testing.T) {
	row := "worker lifecycle maintenance supervisor artifact: /tmp/ouro/supervisor.json status=completed mode=metadata-only runs=2 max_runs=2 actions=2 skipped_unsafe=2 retry_dispatch_execute=2 maintenance_created=2 audit_created=2 retry_dispatch_executions_created=1 retry_dispatch_claims_created=1 reclaim_executions_created=1 reclaim_results_created=1 reconciliation_executions_created=1 reconciliation_results_created=1 maintenance_path=/tmp/ouro/maintenance.json audit_path=/tmp/ouro/audit.json"
	action, ok := runtimeActionForRow(row)
	if !ok {
		t.Fatalf("expected worker lifecycle maintenance supervisor row to parse")
	}
	for _, want := range []string{"reclaim_executions_created=1", "reconciliation_results_created=1"} {
		if !strings.Contains(action.Results, want) {
			t.Fatalf("maintenance supervisor action results missing %q: %#v", want, action)
		}
	}
	tabs := DefaultRuntimeTabs().
		Select(RuntimeTabAgentRuntime).
		WithRows(RuntimeTabAgentRuntime, []string{
			"agents: 0",
			row,
		}).
		SelectRow(1)
	model := NewModel().WithGraphStack(GraphStack{RuntimeTabs: tabs})

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	if cmd != nil {
		t.Fatalf("expected worker lifecycle maintenance supervisor details update to be synchronous")
	}
	next, ok := updated.(Model)
	if !ok {
		t.Fatalf("expected Model, got %T", updated)
	}
	if next.graphStack.Focus != GraphFocusRuntimeAction {
		t.Fatalf("expected runtime action focus, got %s", next.graphStack.Focus)
	}
	view := next.graphStack.Render(160, 40)
	for _, want := range []string{
		"[Node Inspector]",
		"runtime action: worker-lifecycle-maintenance-supervisor-artifact",
		"artifact path: /tmp/ouro/supervisor.json",
		"dispatch path: /tmp/ouro/maintenance.json",
		"result path: /tmp/ouro/audit.json",
		"state: completed",
		"execution mode: metadata-only",
		"ready: 2",
		"results: max_runs=2 actions=2 skipped_unsafe=2 retry_dispatch_execute=2 maintenance_created=2 audit_created=2",
		"mode: read-only",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("worker lifecycle maintenance supervisor artifact details missing %q:\n%s", want, view)
		}
	}
}

func TestModelUpdateShowsSelectedWorkerLifecycleAuditArtifactDetails(t *testing.T) {
	auditRow := "worker lifecycle audit artifact: /tmp/ouro/audit.json status=dry-run mode=metadata-only workers=1 stale_workers=0 placements=2 claims=1 dispatches=1 requests=1 results=1 retry_plans=1 retry_dispatches=1 retry_dispatch_executions=1 leases=1 reclaims=0 reconciliations=0 lifecycle_reviews=1 lifecycle_policies=2 policy_selections=1 policy_runs=3 policy_scheduler_runs=4 action_plans=1 maintenances=1 maintenance_supervisors=1 orchestration_supervisors=1 actions=2 daemon_runs=0 supervisor_runs=0 controls=0"
	tabs := DefaultRuntimeTabs().
		Select(RuntimeTabAgentRuntime).
		WithRows(RuntimeTabAgentRuntime, []string{
			"agents: 0",
			auditRow,
		}).
		SelectRow(1)
	model := NewModel().WithGraphStack(GraphStack{RuntimeTabs: tabs})

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	if cmd != nil {
		t.Fatalf("expected worker lifecycle audit details update to be synchronous")
	}
	next, ok := updated.(Model)
	if !ok {
		t.Fatalf("expected Model, got %T", updated)
	}
	if next.graphStack.Focus != GraphFocusRuntimeAction {
		t.Fatalf("expected runtime action focus, got %s", next.graphStack.Focus)
	}
	view := next.graphStack.Render(260, 28)
	for _, want := range []string{
		"[Node Inspector]",
		"runtime action: worker-lifecycle-audit-artifact",
		"artifact path: /tmp/ouro/audit.json",
		"state: dry-run",
		"execution mode: metadata-only",
		"ready: workers=1 stale_workers=0 placements=2",
		"results: claims=1 dispatches=1 requests=1 results=1 retry_plans=1 retry_dispatches=1 retry_dispatch_executions=1 leases=1",
		"skipped: reclaims=0 reconciliations=0",
		"claimed: lifecycle_policies=2 policy_selections=1 policy_runs=3 policy_scheduler_runs=4 lifecycle_reviews=1",
		"action_plans=1 maintenances=1 maintenance_supervisors=1",
		"orchestration_supervisors=1 actions=2",
		"failed: daemon_runs=0 supervisor_runs=0 controls=0",
		"mode: read-only",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("worker lifecycle audit artifact details missing %q:\n%s", want, view)
		}
	}
	for _, notWant := range []string{
		"results: claims=1 dispatches=1 requests=1 results=1 retry_plans=1 retry_dispatches=1 leases=1 reclaims=0",
		"skipped: reclaims=0 reconciliations=0 lifecycle_policies=2",
	} {
		if strings.Contains(view, notWant) {
			t.Fatalf("worker lifecycle audit artifact details should not overrun field boundary %q:\n%s", notWant, view)
		}
	}
	rest := strings.TrimSpace(strings.TrimPrefix(auditRow, "worker lifecycle audit artifact: /tmp/ouro/audit.json"))
	for key, want := range map[string]string{
		"lifecycle_reviews":     "1",
		"lifecycle_policies":    "2",
		"policy_selections":     "1",
		"policy_runs":           "3",
		"policy_scheduler_runs": "4",
		"action_plans":          "1",
	} {
		if got := runtimeRowField(rest, key); got != want {
			t.Fatalf("runtime row field %s = %q, want %q", key, got, want)
		}
	}
}

func TestModelUpdateShowsSelectedWorkerLifecycleAuditItemDetails(t *testing.T) {
	tabs := DefaultRuntimeTabs().
		Select(RuntimeTabAgentRuntime).
		WithRows(RuntimeTabAgentRuntime, []string{
			"agents: 0",
			"worker lifecycle audit item: kind=worker-lifecycle-maintenance-supervisor status=completed mode=metadata-only gate=none worker=none run=none execution=none path=/tmp/ouro/supervisor.json linked=/tmp/ouro/maintenance.json reason=none actions=2 skipped_unsafe=2 retry_dispatch_execute=2",
		}).
		SelectRow(1)
	model := NewModel().WithGraphStack(GraphStack{RuntimeTabs: tabs})

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	if cmd != nil {
		t.Fatalf("expected worker lifecycle audit item details update to be synchronous")
	}
	next, ok := updated.(Model)
	if !ok {
		t.Fatalf("expected Model, got %T", updated)
	}
	if next.graphStack.Focus != GraphFocusRuntimeAction {
		t.Fatalf("expected runtime action focus, got %s", next.graphStack.Focus)
	}
	view := next.graphStack.Render(170, 36)
	for _, want := range []string{
		"[Node Inspector]",
		"runtime action: worker-lifecycle-audit-item",
		"audit item kind: worker-lifecycle-maintenance-supervisor",
		"state: completed",
		"execution mode: metadata-only",
		"results: actions=2 retry_dispatch_execute=2",
		"skipped: skipped_unsafe=2",
		"artifact path: /tmp/ouro/supervisor.json",
		"dispatch path: /tmp/ouro/maintenance.json",
		"mode: read-only",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("worker lifecycle audit item details missing %q:\n%s", want, view)
		}
	}
}

func TestModelUpdateShowsSelectedWorkerLifecycleAuditActionPlanReviewDetails(t *testing.T) {
	tabs := DefaultRuntimeTabs().
		Select(RuntimeTabAgentRuntime).
		WithRows(RuntimeTabAgentRuntime, []string{
			"agents: 0",
			"worker lifecycle audit item: kind=worker-lifecycle-action-plan status=dry-run mode=metadata-only gate=none worker=none run=none execution=none path=/tmp/ouro/action-plan.json linked=none reason=authorized_retry_dispatch_review decision=accepted reviewer=alice reviewed_at=2026-06-02T10:02:00Z actions=1 ack_actions=1",
			"worker lifecycle audit item: kind=worker-lifecycle-action-plan-selection status=current mode=metadata-only gate=none worker=none run=none execution=none path=/tmp/ouro/worker-lifecycle-action-plan-selection.json linked=/tmp/ouro/action-plan.json reason=current_reviewed_action_plan selected_by=bob selected_at=2026-06-02T10:03:00Z actions=1 ack_actions=1",
		}).
		SelectRow(1)
	model := NewModel().WithGraphStack(GraphStack{RuntimeTabs: tabs})

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	if cmd != nil {
		t.Fatalf("expected worker lifecycle audit action-plan details update to be synchronous")
	}
	next, ok := updated.(Model)
	if !ok {
		t.Fatalf("expected Model, got %T", updated)
	}
	view := next.graphStack.Render(170, 36)
	for _, want := range []string{
		"audit item kind: worker-lifecycle-action-plan",
		"decision: accepted",
		"reviewer: alice",
		"reviewed at: 2026-06-02T10:02:00Z",
		"reason: authorized_retry_dispatch_review",
		"results: actions=1 ack_actions=1",
		"mode: read-only",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("worker lifecycle audit action-plan details missing %q:\n%s", want, view)
		}
	}

	selectionTabs := DefaultRuntimeTabs().
		Select(RuntimeTabAgentRuntime).
		WithRows(RuntimeTabAgentRuntime, []string{
			"agents: 0",
			"worker lifecycle audit item: kind=worker-lifecycle-action-plan status=dry-run mode=metadata-only gate=none worker=none run=none execution=none path=/tmp/ouro/action-plan.json linked=none reason=authorized_retry_dispatch_review decision=accepted reviewer=alice reviewed_at=2026-06-02T10:02:00Z actions=1 ack_actions=1",
			"worker lifecycle audit item: kind=worker-lifecycle-action-plan-selection status=current mode=metadata-only gate=none worker=none run=none execution=none path=/tmp/ouro/worker-lifecycle-action-plan-selection.json linked=/tmp/ouro/action-plan.json reason=current_reviewed_action_plan selected_by=bob selected_at=2026-06-02T10:03:00Z actions=1 ack_actions=1",
		}).
		SelectRow(2)
	selectionModel := NewModel().WithGraphStack(GraphStack{RuntimeTabs: selectionTabs})
	updated, cmd = selectionModel.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	if cmd != nil {
		t.Fatalf("expected worker lifecycle audit selection details update to be synchronous")
	}
	next, ok = updated.(Model)
	if !ok {
		t.Fatalf("expected Model, got %T", updated)
	}
	view = next.graphStack.Render(170, 28)
	for _, want := range []string{
		"audit item kind: worker-lifecycle-action-plan-selection",
		"state: current",
		"reason: current_reviewed_action_plan",
		"selected by: bob",
		"selected at: 2026-06-02T10:03:00Z",
		"dispatch path: /tmp/ouro/action-plan.json",
		"results: actions=1 ack_actions=1",
		"mode: read-only",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("worker lifecycle audit selection details missing %q:\n%s", want, view)
		}
	}
}

func TestModelUpdateShowsWorkerLifecycleActionPlanSelectionSemanticDetails(t *testing.T) {
	tabs := DefaultRuntimeTabs().
		Select(RuntimeTabAgentRuntime).
		WithRows(RuntimeTabAgentRuntime, []string{
			"agents: 0",
			"worker lifecycle action plan selection: status=current mode=metadata-only current=true selected_by=bob selected_at=2026-06-02T09:13:00Z reason=current_reviewed_action_plan plan=/tmp/ouro/action-plan.json selection=/tmp/ouro/worker-lifecycle-action-plan-selection.json review=/tmp/ouro/review.json actions=2 ack=1 run=0 daemon_control=0 retry_dispatch_execute=1 reclaim=0 reconcile=1 superseded=1",
		}).
		SelectRow(1)
	model := NewModel().WithGraphStack(GraphStack{RuntimeTabs: tabs})
	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	if cmd != nil {
		t.Fatalf("expected worker lifecycle action-plan selection detail update to be synchronous")
	}
	next, ok := updated.(Model)
	if !ok {
		t.Fatalf("expected Model, got %T", updated)
	}
	view := next.graphStack.Render(170, 60)
	for _, want := range []string{
		"runtime action: worker-lifecycle-action-plan-selection",
		"selection status: current",
		"plan path: /tmp/ouro/action-plan.json",
		"selection path: /tmp/ouro/worker-lifecycle-action-plan-selection.json",
		"review path: /tmp/ouro/review.json",
		"current: true",
		"selected by: bob",
		"selected at: 2026-06-02T09:13:00Z",
		"reason: current_reviewed_action_plan",
		"results: ack=1 run=0 daemon_control=0 retry_dispatch_execute=1 reclaim=0 reconcile=1",
		"mode: metadata-only",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("worker lifecycle action-plan selection semantic detail missing %q:\n%s", want, view)
		}
	}
}

func TestRuntimeWorkerLifecycleActionPlanSelectionUsesEncodedReasonText(t *testing.T) {
	reason := "current reviewed plan=/tmp/ouro/evil-plan.json selection=/tmp/ouro/evil-selection.json review=/tmp/ouro/evil-review.json current=false actions=99 ack=99"
	row := "worker lifecycle action plan selection: status=current mode=metadata-only current=true selected_by=bob selected_at=2026-06-02T09:13:00Z reason=" + reason + " reason_b64=" + base64.RawURLEncoding.EncodeToString([]byte(reason)) + " plan=/tmp/ouro/action-plan.json selection=/tmp/ouro/worker-lifecycle-action-plan-selection.json review=/tmp/ouro/review.json actions=2 ack=1 run=0 daemon_control=0 retry_dispatch_execute=1 reclaim=0 reconcile=1 superseded=1"

	action := runtimeWorkerLifecycleActionPlanSelectionActionForRow(row)
	if action.ArtifactPath != "/tmp/ouro/action-plan.json" || action.RequestPath != "/tmp/ouro/worker-lifecycle-action-plan-selection.json" || action.ReviewPath != "/tmp/ouro/review.json" {
		t.Fatalf("encoded action-plan selection reason must not retarget marker references: %#v", action)
	}
	if action.Current != "true" || action.Ready != "2" || action.Results != "ack=1 run=0 daemon_control=0 retry_dispatch_execute=1 reclaim=0 reconcile=1" {
		t.Fatalf("encoded action-plan selection reason must not retarget counters: %#v", action)
	}
	if action.Reason != reason {
		t.Fatalf("expected encoded action-plan selection reason to be decoded, got %q", action.Reason)
	}
}

func TestRuntimeWorkerLifecycleActionPlanSelectionIgnoresFakeReasonSidecarMarkers(t *testing.T) {
	reason := "current reviewed reason_b64=fake plan=/tmp/ouro/evil-plan.json selection=/tmp/ouro/evil-selection.json review=/tmp/ouro/evil-review.json current=false actions=99 ack=99"
	row := "worker lifecycle action plan selection: status=current mode=metadata-only current=true selected_by=bob selected_at=2026-06-02T09:13:00Z reason=" + reason + " reason_b64=" + base64.RawURLEncoding.EncodeToString([]byte(reason)) + " plan=/tmp/ouro/action-plan.json selection=/tmp/ouro/worker-lifecycle-action-plan-selection.json review=/tmp/ouro/review.json actions=2 ack=1 run=0 daemon_control=0 retry_dispatch_execute=1 reclaim=0 reconcile=1 superseded=1"

	action := runtimeWorkerLifecycleActionPlanSelectionActionForRow(row)
	if action.ArtifactPath != "/tmp/ouro/action-plan.json" || action.RequestPath != "/tmp/ouro/worker-lifecycle-action-plan-selection.json" || action.ReviewPath != "/tmp/ouro/review.json" {
		t.Fatalf("fake reason sidecar marker must not retarget action-plan selection references: %#v", action)
	}
	if action.Current != "true" || action.Ready != "2" || action.Results != "ack=1 run=0 daemon_control=0 retry_dispatch_execute=1 reclaim=0 reconcile=1" {
		t.Fatalf("fake reason sidecar marker must not retarget action-plan selection counters: %#v", action)
	}
	if action.Reason != reason {
		t.Fatalf("expected encoded action-plan selection reason to survive fake marker, got %q", action.Reason)
	}
}

func TestRuntimeWorkerLifecycleActionPlanSelectionUsesEncodedReferenceFields(t *testing.T) {
	planPath := "/tmp/ouro/action plan status=failed.json"
	selectionPath := "/tmp/ouro/selection path current=false.json"
	reviewPath := "/tmp/ouro/review path actions=99.json"
	selectedBy := "bob selected_at=2030-01-01T00:00:00Z review=/tmp/ouro/evil-review.json"
	selectedAt := "2026-06-02T09:13:00Z selected_by=mallory"
	reason := "current reviewed plan=/tmp/ouro/evil-plan.json selection=/tmp/ouro/evil-selection.json"
	row := "worker lifecycle action plan selection: status=current mode=metadata-only current=true selected_by=" + selectedBy +
		" selected_by_b64=" + base64.RawURLEncoding.EncodeToString([]byte(selectedBy)) +
		" selected_at=" + selectedAt +
		" selected_at_b64=" + base64.RawURLEncoding.EncodeToString([]byte(selectedAt)) +
		" reason=" + reason +
		" reason_b64=" + base64.RawURLEncoding.EncodeToString([]byte(reason)) +
		" plan=" + planPath +
		" plan_b64=" + base64.RawURLEncoding.EncodeToString([]byte(planPath)) +
		" selection=" + selectionPath +
		" selection_b64=" + base64.RawURLEncoding.EncodeToString([]byte(selectionPath)) +
		" review=" + reviewPath +
		" review_b64=" + base64.RawURLEncoding.EncodeToString([]byte(reviewPath)) +
		" actions=2 ack=1 run=0 daemon_control=0 retry_dispatch_execute=1 reclaim=0 reconcile=1 superseded=1"

	action := runtimeWorkerLifecycleActionPlanSelectionActionForRow(row)
	if action.ArtifactPath != planPath || action.RequestPath != selectionPath || action.ReviewPath != reviewPath {
		t.Fatalf("expected encoded action-plan selection references, got %#v", action)
	}
	if action.State != "current" || action.ExecutionMode != "metadata-only" || action.Current != "true" || action.Ready != "2" || action.Superseded != "1" {
		t.Fatalf("encoded action-plan selection text must not retarget machine fields: %#v", action)
	}
	if action.SelectedBy != selectedBy || action.SelectedAt != selectedAt || action.Reason != reason {
		t.Fatalf("expected encoded action-plan selection actor metadata, got %#v", action)
	}
}

func TestModelUpdateShowsClearedWorkerLifecycleActionPlanSelectionSemanticDetails(t *testing.T) {
	tabs := DefaultRuntimeTabs().
		Select(RuntimeTabAgentRuntime).
		WithRows(RuntimeTabAgentRuntime, []string{
			"agents: 0",
			"worker lifecycle action plan selection: status=cleared mode=metadata-only current=false cleared_by=tui cleared_at=2026-06-02T09:30:00Z reason=cleared_from_TUI plan=/tmp/ouro/action-plan.json selection=/tmp/ouro/worker-lifecycle-action-plan-selection.json review=/tmp/ouro/review.json actions=2 ack=1 run=0 daemon_control=0 retry_dispatch_execute=1 reclaim=0 reconcile=1 superseded=1",
		}).
		SelectRow(1)
	model := NewModel().WithGraphStack(GraphStack{RuntimeTabs: tabs})
	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	if cmd != nil {
		t.Fatalf("expected cleared worker lifecycle action-plan selection detail update to be synchronous")
	}
	next, ok := updated.(Model)
	if !ok {
		t.Fatalf("expected Model, got %T", updated)
	}
	view := next.graphStack.Render(170, 60)
	for _, want := range []string{
		"runtime action: worker-lifecycle-action-plan-selection",
		"selection status: cleared",
		"plan path: /tmp/ouro/action-plan.json",
		"selection path: /tmp/ouro/worker-lifecycle-action-plan-selection.json",
		"review path: /tmp/ouro/review.json",
		"current: false",
		"cleared by: tui",
		"cleared at: 2026-06-02T09:30:00Z",
		"reason: cleared_from_TUI",
		"mode: read-only",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("cleared worker lifecycle action-plan selection semantic detail missing %q:\n%s", want, view)
		}
	}
}

func TestModelUpdateShowsSelectedWorkerLifecycleAuditRetryItemDetails(t *testing.T) {
	tabs := DefaultRuntimeTabs().
		Select(RuntimeTabAgentRuntime).
		WithRows(RuntimeTabAgentRuntime, []string{
			"agents: 0",
			"worker lifecycle audit item: kind=worker-dispatch-retry-dispatch-execution status=completed mode=metadata-only gate=worker-dispatch-retry-dispatch-execute worker=none run=none execution=none path=/tmp/ouro/retry-execution.json linked=/tmp/ouro/retry-dispatch.json reason=none total=2 ready=1 claimed=1 skipped=1 failed=0",
		}).
		SelectRow(1)
	model := NewModel().WithGraphStack(GraphStack{RuntimeTabs: tabs})

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	if cmd != nil {
		t.Fatalf("expected worker lifecycle audit retry item details update to be synchronous")
	}
	next, ok := updated.(Model)
	if !ok {
		t.Fatalf("expected Model, got %T", updated)
	}
	if next.graphStack.Focus != GraphFocusRuntimeAction {
		t.Fatalf("expected runtime action focus, got %s", next.graphStack.Focus)
	}
	view := next.graphStack.Render(180, 28)
	for _, want := range []string{
		"[Node Inspector]",
		"runtime action: worker-lifecycle-audit-item",
		"audit item kind: worker-dispatch-retry-dispatch-execution",
		"state: completed",
		"execution mode: metadata-only",
		"gate: worker-dispatch-retry-dispatch-execute",
		"results: total=2 ready=1 claimed=1",
		"skipped: skipped=1",
		"failed: failed=0",
		"artifact path: /tmp/ouro/retry-execution.json",
		"dispatch path: /tmp/ouro/retry-dispatch.json",
		"mode: read-only",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("worker lifecycle audit retry item details missing %q:\n%s", want, view)
		}
	}
}

func TestModelUpdateShowsSelectedWorkerLifecycleAuditLeaseRecoveryItemDetails(t *testing.T) {
	tabs := DefaultRuntimeTabs().
		Select(RuntimeTabAgentRuntime).
		WithRows(RuntimeTabAgentRuntime, []string{
			"agents: 0",
			"worker lifecycle audit item: kind=worker-lease-result-reconciliation-execution status=completed mode=metadata-only gate=worker-lease-result-reconciliation-execute worker=none run=none execution=none path=/tmp/ouro/reconcile-execution.json linked=/tmp/ouro/reconcile-review.json reason=none total=2 results=1 skipped=1",
		}).
		SelectRow(1)
	model := NewModel().WithGraphStack(GraphStack{RuntimeTabs: tabs})

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	if cmd != nil {
		t.Fatalf("expected worker lifecycle audit lease recovery item details update to be synchronous")
	}
	next, ok := updated.(Model)
	if !ok {
		t.Fatalf("expected Model, got %T", updated)
	}
	if next.graphStack.Focus != GraphFocusRuntimeAction {
		t.Fatalf("expected runtime action focus, got %s", next.graphStack.Focus)
	}
	view := next.graphStack.Render(180, 28)
	for _, want := range []string{
		"[Node Inspector]",
		"runtime action: worker-lifecycle-audit-item",
		"audit item kind: worker-lease-result-reconciliation-execution",
		"state: completed",
		"execution mode: metadata-only",
		"gate: worker-lease-result-reconciliation-execute",
		"results: total=2 results=1",
		"skipped: skipped=1",
		"artifact path: /tmp/ouro/reconcile-execution.json",
		"dispatch path: /tmp/ouro/reconcile-review.json",
		"mode: read-only",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("worker lifecycle audit lease recovery item details missing %q:\n%s", want, view)
		}
	}
}

func TestModelUpdateShowsSelectedWorkerLifecycleAuditOrchestrationSupervisorItemDetails(t *testing.T) {
	tabs := DefaultRuntimeTabs().
		Select(RuntimeTabAgentRuntime).
		WithRows(RuntimeTabAgentRuntime, []string{
			"agents: 0",
			"worker lifecycle audit item: kind=worker-orchestration-supervisor status=completed mode=metadata-only gate=none worker=none run=run-supervisor execution=none path=/tmp/ouro/orchestration-supervisor.json linked=/tmp/ouro/orchestration.json reason=none runs=2 max_runs=2 orchestrations_created=2 lifecycle_maintenance=2 lifecycle_audit=2 claimed=2 dispatched=2 skipped=0 failed=0",
		}).
		SelectRow(1)
	model := NewModel().WithGraphStack(GraphStack{RuntimeTabs: tabs})

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	if cmd != nil {
		t.Fatalf("expected worker lifecycle audit orchestration supervisor item details update to be synchronous")
	}
	next, ok := updated.(Model)
	if !ok {
		t.Fatalf("expected Model, got %T", updated)
	}
	if next.graphStack.Focus != GraphFocusRuntimeAction {
		t.Fatalf("expected runtime action focus, got %s", next.graphStack.Focus)
	}
	view := next.graphStack.Render(190, 30)
	for _, want := range []string{
		"[Node Inspector]",
		"runtime action: worker-lifecycle-audit-item",
		"audit item kind: worker-orchestration-supervisor",
		"state: completed",
		"execution mode: metadata-only",
		"results: runs=2 max_runs=2 orchestrations_created=2 lifecycle_maintenance=2 lifecycle_audit=2 claimed=2 dispatched=2",
		"skipped: skipped=0",
		"failed: failed=0",
		"artifact path: /tmp/ouro/orchestration-supervisor.json",
		"dispatch path: /tmp/ouro/orchestration.json",
		"mode: read-only",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("worker lifecycle audit orchestration supervisor item details missing %q:\n%s", want, view)
		}
	}
}

func TestModelUpdateShowsSelectedWorkerLifecycleAuditOrchestrationItemDetails(t *testing.T) {
	tabs := DefaultRuntimeTabs().
		Select(RuntimeTabAgentRuntime).
		WithRows(RuntimeTabAgentRuntime, []string{
			"agents: 0",
			"worker lifecycle audit item: kind=worker-orchestration status=completed mode=execute gate=none worker=none run=run-orchestration execution=none path=/tmp/ouro/orchestration.json linked=/tmp/ouro/batch.json reason=none placement_ready=2 placement_skipped=1 dispatch_ready=1 dispatch_skipped=0 lifecycle_maintenance=1 lifecycle_audit=1 claimed=1 dispatched=1 skipped=1 failed=0",
		}).
		SelectRow(1)
	model := NewModel().WithGraphStack(GraphStack{RuntimeTabs: tabs})

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	if cmd != nil {
		t.Fatalf("expected worker lifecycle audit orchestration item details update to be synchronous")
	}
	next, ok := updated.(Model)
	if !ok {
		t.Fatalf("expected Model, got %T", updated)
	}
	if next.graphStack.Focus != GraphFocusRuntimeAction {
		t.Fatalf("expected runtime action focus, got %s", next.graphStack.Focus)
	}
	view := next.graphStack.Render(190, 30)
	for _, want := range []string{
		"[Node Inspector]",
		"runtime action: worker-lifecycle-audit-item",
		"audit item kind: worker-orchestration",
		"state: completed",
		"execution mode: execute",
		"results: placement_ready=2 placement_skipped=1 dispatch_ready=1 dispatch_skipped=0 lifecycle_maintenance=1 lifecycle_audit=1 claimed=1 dispatched=1",
		"skipped: skipped=1",
		"failed: failed=0",
		"artifact path: /tmp/ouro/orchestration.json",
		"dispatch path: /tmp/ouro/batch.json",
		"mode: read-only",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("worker lifecycle audit orchestration item details missing %q:\n%s", want, view)
		}
	}
}

func TestRuntimeWorkerLifecycleAuditItemUsesEncodedReasonText(t *testing.T) {
	reason := "maintenance supervisor reason_b64=fake actions=99 skipped_unsafe=99 failed=99 retry_dispatch_execute=99"
	row := "worker lifecycle audit item: kind=worker-lifecycle-maintenance-supervisor status=completed mode=metadata-only gate=none worker=none run=none execution=none path=/tmp/ouro/supervisor.json linked=/tmp/ouro/maintenance.json reason=" + reason + " reason_b64=" + base64.RawURLEncoding.EncodeToString([]byte(reason)) + " actions=2 skipped_unsafe=2 retry_dispatch_execute=2"

	action := runtimeWorkerLifecycleAuditItemActionForRow(row)
	if action.Results != "actions=2 retry_dispatch_execute=2" || action.Skipped != "skipped_unsafe=2" || action.Failed != "" {
		t.Fatalf("encoded worker lifecycle audit item reason must not retarget aggregate fields: %#v", action)
	}
	if action.Reason != reason {
		t.Fatalf("expected encoded worker lifecycle audit item reason to be decoded, got %q", action.Reason)
	}
}

func TestModelUpdateShowsSelectedWorkerLifecycleActionDetails(t *testing.T) {
	tabs := DefaultRuntimeTabs().
		Select(RuntimeTabAgentRuntime).
		WithRows(RuntimeTabAgentRuntime, []string{
			"agents: 0",
			"worker lifecycle action: type=ack-transport-request gate=worker-transport-ack worker=remote-web-1 run=run-action execution=tool-action status=ready mode=metadata-only reason=pending-file-outbox-request request_path=/tmp/ouro/request.json lease_path=none result_path=none dispatch=/tmp/ouro/dispatch.json recommended=none",
		}).
		SelectRow(1)
	model := NewModel().WithGraphStack(GraphStack{RuntimeTabs: tabs})

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	if cmd != nil {
		t.Fatalf("expected worker lifecycle action details update to be synchronous")
	}
	next, ok := updated.(Model)
	if !ok {
		t.Fatalf("expected Model, got %T", updated)
	}
	if next.graphStack.Focus != GraphFocusRuntimeAction {
		t.Fatalf("expected runtime action focus, got %s", next.graphStack.Focus)
	}
	view := next.graphStack.Render(160, 28)
	for _, want := range []string{
		"[Node Inspector]",
		"runtime action: worker-lifecycle-action",
		"action type: ack-transport-request",
		"action gate: worker-transport-ack",
		"worker: remote-web-1",
		"run: run-action",
		"execution: tool-action",
		"reason: pending-file-outbox-request",
		"request path: /tmp/ouro/request.json",
		"dispatch path: /tmp/ouro/dispatch.json",
		"mode: read-only",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("worker lifecycle action details missing %q:\n%s", want, view)
		}
	}
}

func TestRuntimeWorkerLifecycleActionUsesEncodedReasonText(t *testing.T) {
	reason := "pending file outbox reason_b64=fake request_path=/tmp/ouro/evil-request.json lease_path=/tmp/ouro/evil-lease.json result_path=/tmp/ouro/evil-result.json dispatch=/tmp/ouro/evil-dispatch.json recommended=skip"
	row := "worker lifecycle action: type=ack-transport-request gate=worker-transport-ack worker=remote-web-1 run=run-action execution=tool-action status=ready mode=metadata-only reason=" + reason + " reason_b64=" + base64.RawURLEncoding.EncodeToString([]byte(reason)) + " request_path=/tmp/ouro/request.json lease_path=none result_path=none dispatch=/tmp/ouro/dispatch.json recommended=none"

	action := runtimeWorkerLifecycleActionForRow(row)
	if action.RequestPath != "/tmp/ouro/request.json" || action.DispatchPath != "/tmp/ouro/dispatch.json" || action.RecommendedStatus != "" {
		t.Fatalf("encoded worker lifecycle action reason must not retarget action paths/status: %#v", action)
	}
	if action.Reason != reason {
		t.Fatalf("expected encoded worker lifecycle action reason to be decoded, got %q", action.Reason)
	}
}

func TestRuntimeWorkerLifecycleActionUsesEncodedIdentityText(t *testing.T) {
	workerID := "remote web status=failed mode=execute request_path=/tmp/ouro/evil-request.json"
	runID := "run action execution=evil-execution dispatch=/tmp/ouro/evil-dispatch.json"
	executionID := "tool action status=failed recommended=skip"
	row := "worker lifecycle action: type=ack-transport-request gate=worker-transport-ack" +
		" worker=" + workerID +
		" worker_b64=" + base64.RawURLEncoding.EncodeToString([]byte(workerID)) +
		" run=" + runID +
		" run_b64=" + base64.RawURLEncoding.EncodeToString([]byte(runID)) +
		" execution=" + executionID +
		" execution_b64=" + base64.RawURLEncoding.EncodeToString([]byte(executionID)) +
		" status=ready mode=metadata-only reason=pending-file-outbox-request reason_b64=" + base64.RawURLEncoding.EncodeToString([]byte("pending-file-outbox-request")) +
		" request_path=/tmp/ouro/request.json lease_path=none result_path=none dispatch=/tmp/ouro/dispatch.json recommended=none"

	action := runtimeWorkerLifecycleActionForRow(row)
	if action.WorkerID != workerID || action.RunID != runID || action.ExecutionID != executionID {
		t.Fatalf("expected encoded worker lifecycle identity fields to be decoded, got %#v", action)
	}
	if action.State != "ready" || action.ExecutionMode != "metadata-only" || action.RequestPath != "/tmp/ouro/request.json" || action.DispatchPath != "/tmp/ouro/dispatch.json" || action.RecommendedStatus != "" {
		t.Fatalf("encoded worker lifecycle identity fields must not retarget action metadata: %#v", action)
	}
}

func TestModelUpdateDoesNotExecuteWorkerLifecycleActionRows(t *testing.T) {
	tabs := DefaultRuntimeTabs().
		Select(RuntimeTabAgentRuntime).
		WithRows(RuntimeTabAgentRuntime, []string{
			"agents: 0",
			"worker lifecycle action: type=ack-transport-request gate=worker-transport-ack worker=remote-web-1 run=run-action execution=tool-action status=ready mode=metadata-only reason=pending-file-outbox-request request_path=/tmp/ouro/request.json lease_path=none result_path=none dispatch=/tmp/ouro/dispatch.json recommended=none",
		}).
		SelectRow(1)
	called := false
	model := NewModel().
		WithGraphStack(GraphStack{RuntimeTabs: tabs}).
		WithRuntimeActionExecutor(func(action RuntimeAction) (RuntimeActionExecutionResult, error) {
			called = true
			return RuntimeActionExecutionResult{Status: "unexpected", Path: "/tmp/ouro/executed.json"}, nil
		})

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if cmd != nil {
		t.Fatalf("worker lifecycle action row must not arm or return an execution command")
	}
	if called {
		t.Fatalf("worker lifecycle action row must not call the runtime action executor")
	}
	next, ok := updated.(Model)
	if !ok {
		t.Fatalf("expected Model, got %T", updated)
	}
	action, ok := next.graphStack.RuntimeTabs.ActiveRuntimeAction()
	if !ok {
		t.Fatalf("expected active worker lifecycle action with unsupported execution metadata")
	}
	if action.Kind != "worker-lifecycle-action" || action.ExecutionStatus != "unsupported" || action.ExecutionError != "runtime action is not executable" {
		t.Fatalf("expected unsupported worker lifecycle action execution metadata, got %#v", action)
	}
	if !runtimeActionIsReadOnly(action) {
		t.Fatalf("expected worker lifecycle action row to be classified read-only")
	}

	updated, cmd = next.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if cmd != nil {
		t.Fatalf("second x on worker lifecycle action row must not return an execution command")
	}
	if called {
		t.Fatalf("second x on worker lifecycle action row must not call the runtime action executor")
	}
}

func TestRuntimeInstallOrchestrationArtifactUsesEncodedErrorText(t *testing.T) {
	errorText := "orchestration failed status=evil mode=execute ready=99 skipped=99 results=99 dispatch=/tmp/evil execution=/tmp/evil retry_dispatch=/tmp/evil error_b64=fake"
	row := "install orchestration artifact: /tmp/ouro/orchestration.json status=failed mode=metadata-only ready=1 skipped=2 results=3 dispatch=/tmp/ouro/dispatch.json execution=/tmp/ouro/execution.json retry_dispatch=/tmp/ouro/retry-dispatch.json error=" + errorText + " error_b64=" + base64.RawURLEncoding.EncodeToString([]byte(errorText))

	action := runtimeInstallOrchestrationArtifactActionForRow(row)
	if action.State != "failed" || action.ExecutionMode != "metadata-only" {
		t.Fatalf("encoded orchestration error must not retarget state/mode: %#v", action)
	}
	if action.Ready != "1" || action.Skipped != "2" || action.Results != "3" {
		t.Fatalf("encoded orchestration error must not retarget counts: %#v", action)
	}
	if action.DispatchPath != "/tmp/ouro/dispatch.json" || action.ExecutionPath != "/tmp/ouro/execution.json" || action.RetryDispatchPath != "/tmp/ouro/retry-dispatch.json" {
		t.Fatalf("encoded orchestration error must not retarget artifact references: %#v", action)
	}
	if action.ExecutionError != errorText {
		t.Fatalf("expected encoded orchestration error to be decoded, got %q", action.ExecutionError)
	}
}

func TestRuntimeInstallOrchestrationArtifactUsesEncodedReferenceFields(t *testing.T) {
	artifactPath := "/tmp/ouro/toolchain/install orchestrations/orchestration status=failed.json"
	dispatchPath := "/tmp/ouro/toolchain/install dispatches/dispatch mode=execute.json"
	executionPath := "/tmp/ouro/toolchain/install executions/execution retry_dispatch=/tmp/ouro/evil-retry.json"
	retryDispatchPath := "/tmp/ouro/toolchain/install retry dispatches/retry mode=execute.json"
	errorText := "orchestration failed dispatch=/tmp/ouro/evil-dispatch.json execution=/tmp/ouro/evil-execution.json retry_dispatch=/tmp/ouro/evil-retry.json"
	row := "install orchestration artifact: " + runtimeCompactRowTextValue(artifactPath) +
		" artifact_b64=" + base64.RawURLEncoding.EncodeToString([]byte(artifactPath)) +
		" status=failed mode=metadata-only ready=1 skipped=2 results=3 dispatch=" + runtimeCompactRowTextValue(dispatchPath) +
		" dispatch_b64=" + base64.RawURLEncoding.EncodeToString([]byte(dispatchPath)) +
		" execution=" + runtimeCompactRowTextValue(executionPath) +
		" execution_b64=" + base64.RawURLEncoding.EncodeToString([]byte(executionPath)) +
		" retry_dispatch=" + runtimeCompactRowTextValue(retryDispatchPath) +
		" retry_dispatch_b64=" + base64.RawURLEncoding.EncodeToString([]byte(retryDispatchPath)) +
		" error=" + runtimeCompactRowTextValue(errorText) +
		" error_b64=" + base64.RawURLEncoding.EncodeToString([]byte(errorText))

	action := runtimeInstallOrchestrationArtifactActionForRow(row)
	if action.ArtifactPath != artifactPath || action.DispatchPath != dispatchPath || action.ExecutionPath != executionPath || action.RetryDispatchPath != retryDispatchPath {
		t.Fatalf("expected encoded install orchestration references, got %#v", action)
	}
	if action.State != "failed" || action.ExecutionMode != "metadata-only" || action.Ready != "1" || action.Skipped != "2" || action.Results != "3" {
		t.Fatalf("encoded install orchestration references must not retarget metadata: %#v", action)
	}
	if action.ExecutionError != errorText {
		t.Fatalf("expected encoded orchestration error to decode, got %q", action.ExecutionError)
	}
}

func TestRuntimeInstallOrchestrationArtifactLegacyErrorFallback(t *testing.T) {
	row := "install orchestration artifact: /tmp/ouro/orchestration.json status=failed mode=execute ready=1 skipped=2 results=3 dispatch=/tmp/ouro/dispatch.json execution=/tmp/ouro/execution.json retry_dispatch=/tmp/ouro/retry-dispatch.json error=legacy orchestration failed"

	action := runtimeInstallOrchestrationArtifactActionForRow(row)
	if action.State != "failed" || action.ExecutionMode != "execute" || action.Ready != "1" || action.Skipped != "2" || action.Results != "3" {
		t.Fatalf("legacy orchestration row metadata changed: %#v", action)
	}
	if action.DispatchPath != "/tmp/ouro/dispatch.json" || action.ExecutionPath != "/tmp/ouro/execution.json" || action.RetryDispatchPath != "/tmp/ouro/retry-dispatch.json" {
		t.Fatalf("legacy orchestration row references changed: %#v", action)
	}
	if action.ExecutionError != "legacy orchestration failed" {
		t.Fatalf("legacy orchestration error fallback changed: %#v", action)
	}
}

func TestRuntimeInstallRetryDispatchArtifactUsesEncodedReferenceFields(t *testing.T) {
	artifactPath := "/tmp/ouro/toolchain/install retry dispatches/retry status=failed.json"
	row := "retry dispatch artifact: " + runtimeCompactRowTextValue(artifactPath) +
		" artifact_b64=" + base64.RawURLEncoding.EncodeToString([]byte(artifactPath)) +
		" status=dry-run mode=metadata-only ready=1 skipped=0"

	action := runtimeRetryDispatchArtifactActionForRow(row)
	if action.ArtifactPath != artifactPath {
		t.Fatalf("expected encoded install retry dispatch artifact path, got %#v", action)
	}
	if action.State != "dry-run" || action.ExecutionMode != "metadata-only" || action.Ready != "1" || action.Skipped != "0" {
		t.Fatalf("encoded install retry dispatch artifact path must not retarget metadata: %#v", action)
	}
}

func TestModelUpdateShowsSelectedInstallRetryExecutionArtifactDetails(t *testing.T) {
	tabs := DefaultRuntimeTabs().
		Select(RuntimeTabToolchainHealth).
		WithRows(RuntimeTabToolchainHealth, []string{
			"toolchain: ready",
			"retry execution artifact: /tmp/ouro/retry-execution.json status=completed mode=execute retry_dispatch=/tmp/ouro/retry-dispatch.json results=1 skipped=0",
		}).
		SelectRow(1)
	model := NewModel().WithGraphStack(GraphStack{RuntimeTabs: tabs})

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	if cmd != nil {
		t.Fatalf("expected retry execution artifact details update to be synchronous")
	}
	next, ok := updated.(Model)
	if !ok {
		t.Fatalf("expected Model, got %T", updated)
	}
	if next.graphStack.Focus != GraphFocusRuntimeAction {
		t.Fatalf("expected runtime action focus, got %s", next.graphStack.Focus)
	}
	view := next.graphStack.Render(140, 24)
	for _, want := range []string{
		"[Node Inspector]",
		"runtime action: install-retry-execution-artifact",
		"artifact path: /tmp/ouro/retry-execution.json",
		"retry dispatch path: /tmp/ouro/retry-dispatch.json",
		"state: completed",
		"execution mode: execute",
		"results: 1",
		"skipped: 0",
		"mode: read-only",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("retry execution artifact details missing %q:\n%s", want, view)
		}
	}
}

func TestRuntimeInstallRetryExecutionArtifactUsesEncodedReferenceFields(t *testing.T) {
	artifactPath := "/tmp/ouro/toolchain/install retry executions/retry status=failed.json"
	retryDispatchPath := "/tmp/ouro/toolchain/install retry dispatches/retry mode=execute.json"
	row := "retry execution artifact: " + runtimeCompactRowTextValue(artifactPath) +
		" artifact_b64=" + base64.RawURLEncoding.EncodeToString([]byte(artifactPath)) +
		" status=completed mode=execute retry_dispatch=" + runtimeCompactRowTextValue(retryDispatchPath) +
		" retry_dispatch_b64=" + base64.RawURLEncoding.EncodeToString([]byte(retryDispatchPath)) +
		" results=1 skipped=0"

	action := runtimeRetryExecutionArtifactActionForRow(row)
	if action.ArtifactPath != artifactPath || action.RetryDispatchPath != retryDispatchPath {
		t.Fatalf("expected encoded retry execution artifact references, got %#v", action)
	}
	if action.State != "completed" || action.ExecutionMode != "execute" || action.Results != "1" || action.Skipped != "0" {
		t.Fatalf("encoded retry execution artifact references must not retarget metadata: %#v", action)
	}
}

func TestModelUpdateShowsSelectedInstallRetryExecutionResultDetails(t *testing.T) {
	tabs := DefaultRuntimeTabs().
		Select(RuntimeTabToolchainHealth).
		WithRows(RuntimeTabToolchainHealth, []string{
			"toolchain: ready",
			"retry execution result web-api-pack/missing-httpx: failed output=retry failed error=exit status 7 artifact=/tmp/ouro/retry-execution.json dispatch=/tmp/ouro/install-dispatch.json retry_dispatch=/tmp/ouro/retry-dispatch.json source_execution=/tmp/ouro/source-execution.json command=sh -c echo retry",
		}).
		SelectRow(1)
	model := NewModel().WithGraphStack(GraphStack{RuntimeTabs: tabs})

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	if cmd != nil {
		t.Fatalf("expected retry execution result details update to be synchronous")
	}
	next, ok := updated.(Model)
	if !ok {
		t.Fatalf("expected Model, got %T", updated)
	}
	view := next.graphStack.Render(150, 26)
	for _, want := range []string{
		"runtime action: install-retry-execution-result",
		"pack: web-api-pack",
		"name: missing-httpx",
		"state: failed",
		"output: retry failed",
		"execution error: exit status 7",
		"artifact path: /tmp/ouro/retry-execution.json",
		"dispatch path: /tmp/ouro/install-dispatch.json",
		"retry dispatch path: /tmp/ouro/retry-dispatch.json",
		"source execution path: /tmp/ouro/source-execution.json",
		"command: sh -c echo retry",
		"mode: read-only",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("retry execution result details missing %q:\n%s", want, view)
		}
	}
}

func TestRuntimeInstallRetryExecutionResultUsesEncodedOutputErrorText(t *testing.T) {
	output := "retry output output_b64=fake artifact=/tmp/ouro/evil-retry-execution.json dispatch=/tmp/ouro/evil-dispatch.json command=evil"
	errorText := "retry failed error_b64=fake retry_dispatch=/tmp/ouro/evil-retry-dispatch.json source_execution=/tmp/ouro/evil-source.json dispatch=/tmp/ouro/evil-dispatch.json command=evil"
	row := "retry execution result web-api-pack/missing-httpx: failed output=" + output + " output_b64=" + base64.RawURLEncoding.EncodeToString([]byte(output)) + " error=" + errorText + " error_b64=" + base64.RawURLEncoding.EncodeToString([]byte(errorText)) + " artifact=/tmp/ouro/retry-execution.json retry_dispatch=/tmp/ouro/retry-dispatch.json source_execution=/tmp/ouro/source-execution.json dispatch=/tmp/ouro/install-dispatch.json command=sh -c echo retry"

	action := runtimeRetryExecutionResultActionForRow(row)
	if action.ArtifactPath != "/tmp/ouro/retry-execution.json" || action.DispatchPath != "/tmp/ouro/install-dispatch.json" || action.RetryDispatchPath != "/tmp/ouro/retry-dispatch.json" || action.SourceExecutionPath != "/tmp/ouro/source-execution.json" || action.Command != "sh -c echo retry" {
		t.Fatalf("encoded retry execution output/error must not retarget machine fields: %#v", action)
	}
	if action.Output != output || action.ExecutionError != errorText {
		t.Fatalf("expected encoded retry execution output/error to be decoded, got output=%q error=%q", action.Output, action.ExecutionError)
	}
}

func TestRuntimeInstallRetryExecutionResultUsesEncodedReferenceFields(t *testing.T) {
	artifactPath := "/tmp/ouro/toolchain/install retry executions/retry status=failed.json"
	retryDispatchPath := "/tmp/ouro/toolchain/install retry dispatches/retry mode=execute.json"
	sourceExecutionPath := "/tmp/ouro/toolchain/install executions/source status=failed.json"
	dispatchPath := "/tmp/ouro/toolchain/install dispatches/dispatch mode=execute.json"
	command := "sh -c echo retry_dispatch=/tmp/ouro/evil-retry.json source_execution=/tmp/ouro/evil-source.json"
	output := "retry output artifact=/tmp/ouro/evil-retry-execution.json dispatch=/tmp/ouro/evil-dispatch.json"
	errorText := "retry failed retry_dispatch=/tmp/ouro/evil-retry-dispatch.json source_execution=/tmp/ouro/evil-source.json"
	row := "retry execution result web-api-pack/missing-httpx: failed output=" + runtimeCompactRowTextValue(output) +
		" output_b64=" + base64.RawURLEncoding.EncodeToString([]byte(output)) +
		" error=" + runtimeCompactRowTextValue(errorText) +
		" error_b64=" + base64.RawURLEncoding.EncodeToString([]byte(errorText)) +
		" artifact=" + runtimeCompactRowTextValue(artifactPath) +
		" artifact_b64=" + base64.RawURLEncoding.EncodeToString([]byte(artifactPath)) +
		" retry_dispatch=" + runtimeCompactRowTextValue(retryDispatchPath) +
		" retry_dispatch_b64=" + base64.RawURLEncoding.EncodeToString([]byte(retryDispatchPath)) +
		" source_execution=" + runtimeCompactRowTextValue(sourceExecutionPath) +
		" source_execution_b64=" + base64.RawURLEncoding.EncodeToString([]byte(sourceExecutionPath)) +
		" dispatch=" + runtimeCompactRowTextValue(dispatchPath) +
		" dispatch_b64=" + base64.RawURLEncoding.EncodeToString([]byte(dispatchPath)) +
		" command=" + runtimeCompactRowTextValue(command) +
		" command_b64=" + base64.RawURLEncoding.EncodeToString([]byte(command))

	action := runtimeRetryExecutionResultActionForRow(row)
	if action.ArtifactPath != artifactPath || action.RetryDispatchPath != retryDispatchPath || action.SourceExecutionPath != sourceExecutionPath || action.DispatchPath != dispatchPath {
		t.Fatalf("expected encoded retry execution result references, got %#v", action)
	}
	if action.State != "failed" || action.Output != output || action.ExecutionError != errorText || action.Command != command {
		t.Fatalf("encoded retry execution result references must not retarget text/metadata: %#v", action)
	}
}

func TestRuntimeRetryExecutionResultRefsUsesEncodedReferenceFields(t *testing.T) {
	artifactPath := "/tmp/ouro/toolchain/install retry executions/retry status=failed.json"
	retryDispatchPath := "/tmp/ouro/toolchain/install retry dispatches/retry mode=execute.json"
	sourceExecutionPath := "/tmp/ouro/toolchain/install executions/source status=failed.json"
	dispatchPath := "/tmp/ouro/toolchain/install dispatches/dispatch mode=execute.json"
	command := "sh -c echo retry_dispatch=/tmp/ouro/evil-retry.json source_execution=/tmp/ouro/evil-source.json"
	row := "retry execution result refs web-api-pack/missing-httpx:" +
		" source_execution=" + runtimeCompactRowTextValue(sourceExecutionPath) +
		" source_execution_b64=" + base64.RawURLEncoding.EncodeToString([]byte(sourceExecutionPath)) +
		" dispatch=" + runtimeCompactRowTextValue(dispatchPath) +
		" dispatch_b64=" + base64.RawURLEncoding.EncodeToString([]byte(dispatchPath)) +
		" artifact=" + runtimeCompactRowTextValue(artifactPath) +
		" artifact_b64=" + base64.RawURLEncoding.EncodeToString([]byte(artifactPath)) +
		" retry_dispatch=" + runtimeCompactRowTextValue(retryDispatchPath) +
		" retry_dispatch_b64=" + base64.RawURLEncoding.EncodeToString([]byte(retryDispatchPath)) +
		" command=" + runtimeCompactRowTextValue(command) +
		" command_b64=" + base64.RawURLEncoding.EncodeToString([]byte(command))

	action, ok := runtimeActionForRow(row)
	if !ok {
		t.Fatalf("expected retry execution refs action")
	}
	if action.ArtifactPath != artifactPath || action.RetryDispatchPath != retryDispatchPath || action.SourceExecutionPath != sourceExecutionPath || action.DispatchPath != dispatchPath {
		t.Fatalf("expected encoded retry execution refs references, got %#v", action)
	}
	if action.Kind != "install-retry-execution-result" || action.Pack != "web-api-pack" || action.Name != "missing-httpx" || action.Command != command {
		t.Fatalf("encoded retry execution refs must preserve identity and command: %#v", action)
	}
}

func TestRuntimeInstallRetryExecutionResultIgnoresLaterFakeOutputSidecar(t *testing.T) {
	output := "retry output"
	errorText := "retry failed output_b64=" + base64.RawURLEncoding.EncodeToString([]byte("fake output")) + " retry_dispatch=/tmp/ouro/evil-retry-dispatch.json source_execution=/tmp/ouro/evil-source.json dispatch=/tmp/ouro/evil-dispatch.json command=evil"
	row := "retry execution result web-api-pack/missing-httpx: failed output=" + output + " output_b64=" + base64.RawURLEncoding.EncodeToString([]byte(output)) + " error=" + errorText + " error_b64=" + base64.RawURLEncoding.EncodeToString([]byte(errorText)) + " artifact=/tmp/ouro/retry-execution.json retry_dispatch=/tmp/ouro/retry-dispatch.json source_execution=/tmp/ouro/source-execution.json dispatch=/tmp/ouro/install-dispatch.json command=sh -c echo retry command_b64=" + base64.RawURLEncoding.EncodeToString([]byte("sh -c echo retry"))

	action := runtimeRetryExecutionResultActionForRow(row)
	if action.Output != output || action.ExecutionError != errorText {
		t.Fatalf("later fake output sidecar must not replace decoded retry output/error: %#v", action)
	}
	if action.ArtifactPath != "/tmp/ouro/retry-execution.json" || action.DispatchPath != "/tmp/ouro/install-dispatch.json" || action.RetryDispatchPath != "/tmp/ouro/retry-dispatch.json" || action.SourceExecutionPath != "/tmp/ouro/source-execution.json" || action.Command != "sh -c echo retry" {
		t.Fatalf("later fake output sidecar must not retarget retry machine fields: %#v", action)
	}
}

func TestRuntimeInstallRetryExecutionResultUsesEncodedCommandText(t *testing.T) {
	output := "retry output"
	errorText := "retry failed"
	command := "sh -c echo error=evil retry_dispatch=/tmp/ouro/evil-retry-dispatch.json source_execution=/tmp/ouro/evil-source.json dispatch=/tmp/ouro/evil-dispatch.json command_b64=" + base64.RawURLEncoding.EncodeToString([]byte("fake command"))
	row := "retry execution result web-api-pack/missing-httpx: failed output=" + output + " output_b64=" + base64.RawURLEncoding.EncodeToString([]byte(output)) + " error=" + errorText + " error_b64=" + base64.RawURLEncoding.EncodeToString([]byte(errorText)) + " artifact=/tmp/ouro/retry-execution.json retry_dispatch=/tmp/ouro/retry-dispatch.json source_execution=/tmp/ouro/source-execution.json dispatch=/tmp/ouro/install-dispatch.json command=" + command + " command_b64=" + base64.RawURLEncoding.EncodeToString([]byte(command))

	action := runtimeRetryExecutionResultActionForRow(row)
	if action.ArtifactPath != "/tmp/ouro/retry-execution.json" || action.DispatchPath != "/tmp/ouro/install-dispatch.json" || action.RetryDispatchPath != "/tmp/ouro/retry-dispatch.json" || action.SourceExecutionPath != "/tmp/ouro/source-execution.json" {
		t.Fatalf("encoded retry execution command must not retarget machine fields: %#v", action)
	}
	if action.Output != output || action.ExecutionError != errorText || action.Command != command {
		t.Fatalf("expected encoded retry output/error/command, got output=%q error=%q command=%q", action.Output, action.ExecutionError, action.Command)
	}
}

func TestRuntimeRetryExecutionResultRefsSummaryUsesSummaryAction(t *testing.T) {
	row := "retry execution result refs summary web-api-pack/missing-httpx: source_execution=source-execution.json dispatch=dispatch.json artifact=retry-execution.json retry_dispatch=retry-dispatch.json"

	action, ok := runtimeActionForRow(row)
	if !ok {
		t.Fatalf("expected retry execution result refs summary action")
	}
	if action.Kind != "install-retry-execution-result-refs-summary" || action.Pack != "web-api-pack" || action.Name != "missing-httpx" || action.State != "summary" {
		t.Fatalf("expected retry refs summary identity, got %#v", action)
	}
	if action.SourceExecutionPath != "source-execution.json" || action.DispatchPath != "dispatch.json" || action.ArtifactPath != "retry-execution.json" || action.RetryDispatchPath != "retry-dispatch.json" {
		t.Fatalf("expected retry refs summary paths, got %#v", action)
	}
}

func TestRuntimeRetryExecutionResultRefsSummaryUsesEncodedReferenceFields(t *testing.T) {
	sourceExecution := "source execution status=failed.json"
	dispatch := "dispatch retry_dispatch=evil.json"
	artifact := "retry execution mode=execute.json"
	retryDispatch := "retry dispatch status=failed.json"
	row := "retry execution result refs summary web-api-pack/missing-httpx:" +
		" source_execution=" + runtimeCompactRowTextValue(sourceExecution) +
		" source_execution_b64=" + base64.RawURLEncoding.EncodeToString([]byte(sourceExecution)) +
		" dispatch=" + runtimeCompactRowTextValue(dispatch) +
		" dispatch_b64=" + base64.RawURLEncoding.EncodeToString([]byte(dispatch)) +
		" artifact=" + runtimeCompactRowTextValue(artifact) +
		" artifact_b64=" + base64.RawURLEncoding.EncodeToString([]byte(artifact)) +
		" retry_dispatch=" + runtimeCompactRowTextValue(retryDispatch) +
		" retry_dispatch_b64=" + base64.RawURLEncoding.EncodeToString([]byte(retryDispatch))

	action := runtimeRetryExecutionResultRefsSummaryActionForRow(row)
	if action.SourceExecutionPath != sourceExecution || action.DispatchPath != dispatch || action.ArtifactPath != artifact || action.RetryDispatchPath != retryDispatch {
		t.Fatalf("expected encoded retry refs summary references, got %#v", action)
	}
	if action.Kind != "install-retry-execution-result-refs-summary" || action.State != "summary" || action.Pack != "web-api-pack" || action.Name != "missing-httpx" {
		t.Fatalf("encoded retry refs summary must not retarget identity: %#v", action)
	}
}

func TestModelUpdateShowsSelectedInstallRetryExecutionResultDetailsDecodeEncodedOutputError(t *testing.T) {
	output := "retry output artifact=/tmp/ouro/evil-retry-execution.json dispatch=/tmp/ouro/evil-dispatch.json command=evil"
	errorText := "retry failed retry_dispatch=/tmp/ouro/evil-retry-dispatch.json source_execution=/tmp/ouro/evil-source.json dispatch=/tmp/ouro/evil-dispatch.json command=evil"
	row := "retry execution result web-api-pack/missing-httpx: failed output=" + output + " output_b64=" + base64.RawURLEncoding.EncodeToString([]byte(output)) + " error=" + errorText + " error_b64=" + base64.RawURLEncoding.EncodeToString([]byte(errorText)) + " artifact=/tmp/ouro/retry-execution.json retry_dispatch=/tmp/ouro/retry-dispatch.json source_execution=/tmp/ouro/source-execution.json dispatch=/tmp/ouro/install-dispatch.json command=sh -c echo retry"
	tabs := DefaultRuntimeTabs().
		Select(RuntimeTabToolchainHealth).
		WithRows(RuntimeTabToolchainHealth, []string{
			"toolchain: ready",
			row,
		}).
		SelectRow(1)
	model := NewModel().WithGraphStack(GraphStack{RuntimeTabs: tabs})

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	if cmd != nil {
		t.Fatalf("expected encoded retry execution result details update to be synchronous")
	}
	view := updated.(Model).graphStack.Render(180, 28)
	for _, want := range []string{
		"runtime action: install-retry-execution-result",
		"output: " + output,
		"error: " + errorText,
		"artifact path: /tmp/ouro/retry-execution.json",
		"dispatch path: /tmp/ouro/install-dispatch.json",
		"retry dispatch path: /tmp/ouro/retry-dispatch.json",
		"source execution path: /tmp/ouro/source-execution.json",
		"command: sh -c echo retry",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("encoded retry execution result details missing %q:\n%s", want, view)
		}
	}
}

func TestGraphStackUsesRuntimeTabsForSelectedNodeInspector(t *testing.T) {
	stack := GraphStack{
		Attack: AttackGraph{
			Nodes: []evidence.Node{
				{ID: "finding-1", Type: evidence.NodeFinding, Label: "admin exposure", SourceKind: "finding"},
			},
		},
		RuntimeTabs: DefaultRuntimeTabs(),
	}.SelectAttackNode("finding-1")

	view := stack.Render(64, 15)
	for _, text := range []string{
		"[Node Inspector]",
		"selected: finding-1",
		"type: Finding",
		"label: admin exposure",
	} {
		if !strings.Contains(view, text) {
			t.Fatalf("graph stack runtime tabs missing %q:\n%s", text, view)
		}
	}
}
