package tui

import (
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
