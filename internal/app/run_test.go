package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ouroboros/ouroboros/internal/compiler"
	"github.com/ouroboros/ouroboros/internal/eventlog"
	"github.com/ouroboros/ouroboros/internal/evidence"
	"github.com/ouroboros/ouroboros/internal/sisyphus"
	"github.com/ouroboros/ouroboros/internal/toolchain"
	"github.com/ouroboros/ouroboros/internal/tui"
)

func TestInitWorkspaceCreatesProviderConfigAndToolchainManifests(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")

	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}

	if _, err := os.Stat(filepath.Join(root, "providers.yaml")); err != nil {
		t.Fatalf("expected providers.yaml: %v", err)
	}

	manifests, err := toolchain.LoadManifests(filepath.Join(root, "toolchain"))
	if err != nil {
		t.Fatalf("load workspace manifests: %v", err)
	}
	if len(manifests) != 3 {
		t.Fatalf("expected three workspace manifests, got %d", len(manifests))
	}
	byName := map[string]toolchain.Manifest{}
	for _, manifest := range manifests {
		byName[manifest.Name] = manifest
	}
	assertWorkspaceToolDependencies(t, byName["web-api-pack"], []string{"nuclei", "nuclei-templates", "httpx", "subfinder", "ffuf", "katana"})
	assertWorkspaceToolDependencies(t, byName["infra-pack"], []string{"nmap", "naabu", "dnsx"})
}

func TestInitWorkspaceToolchainManifestsMatchBundledManifests(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}

	bundled, err := toolchain.LoadManifests(filepath.Join("..", "..", "toolchains"))
	if err != nil {
		t.Fatalf("load bundled manifests: %v", err)
	}
	workspace, err := toolchain.LoadManifests(filepath.Join(root, "toolchain"))
	if err != nil {
		t.Fatalf("load workspace manifests: %v", err)
	}

	bundledByName := manifestsByNameWithoutPath(bundled)
	workspaceByName := manifestsByNameWithoutPath(workspace)
	if !reflect.DeepEqual(bundledByName, workspaceByName) {
		t.Fatalf("expected workspace manifests to match bundled manifests\nbundled=%#v\nworkspace=%#v", bundledByName, workspaceByName)
	}
}

func TestDoctorWorkspaceRejectsMissingProviderConfigAndToolchainManifest(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	if err := os.Remove(filepath.Join(root, "providers.yaml")); err != nil {
		t.Fatalf("remove providers: %v", err)
	}
	if err := os.Remove(filepath.Join(root, "toolchain", "ctf-pack", "manifest.yaml")); err != nil {
		t.Fatalf("remove ctf manifest: %v", err)
	}

	report, err := DoctorWorkspace(root)
	if err != nil {
		t.Fatalf("doctor workspace: %v", err)
	}
	if report.Ready {
		t.Fatalf("expected workspace to be unready")
	}
	joined := strings.Join(report.Errors, "\n")
	for _, want := range []string{"providers.yaml", "ctf-pack"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("expected %q in doctor errors, got %q", want, joined)
		}
	}
}

func manifestsByNameWithoutPath(manifests []toolchain.Manifest) map[string]toolchain.Manifest {
	byName := map[string]toolchain.Manifest{}
	for _, manifest := range manifests {
		manifest.Path = ""
		byName[manifest.Name] = manifest
	}
	return byName
}

func assertWorkspaceToolDependencies(t *testing.T, manifest toolchain.Manifest, names []string) {
	t.Helper()
	if manifest.Name == "" {
		t.Fatalf("expected workspace manifest to be present")
	}
	byName := map[string]toolchain.Dependency{}
	for _, dependency := range manifest.Dependencies {
		byName[dependency.Name] = dependency
	}
	for _, name := range names {
		dependency, ok := byName[name]
		if !ok {
			t.Fatalf("expected %s dependency in %s workspace manifest, got %#v", name, manifest.Name, manifest.Dependencies)
		}
		if dependency.InstallHint == "" || len(dependency.InstallCommand) == 0 {
			t.Fatalf("expected install metadata for %s, got %#v", name, dependency)
		}
	}
}

func TestRunWorkspaceRecordsDurableEventsReplayAndReport(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}

	result, err := RunWorkspace(ctx, root)
	if err != nil {
		t.Fatalf("run workspace: %v", err)
	}
	if result.RunID == "" || result.EventCount == 0 {
		t.Fatalf("unexpected run result: %#v", result)
	}
	if result.ReportPath == "" {
		t.Fatalf("expected report path in result: %#v", result)
	}
	if !strings.Contains(result.StatusLine, "model:offline-model") {
		t.Fatalf("expected TUI status line in result, got %#v", result)
	}
	for _, want := range []string{"Workspace", "Mission Conversation", "Attack Graph", "Runtime Tabs", "model:offline-model"} {
		if !strings.Contains(result.TUIView, want) {
			t.Fatalf("expected TUI view to contain %q, got:\n%s", want, result.TUIView)
		}
	}
	reportData, err := os.ReadFile(result.ReportPath)
	if err != nil {
		t.Fatalf("read report: %v", err)
	}
	if !strings.Contains(string(reportData), "# Ouroboros Report") {
		t.Fatalf("expected markdown report, got:\n%s", string(reportData))
	}
	for _, want := range []string{"artifact://replay/", "configure a real provider adapter before live target execution"} {
		if !strings.Contains(string(reportData), want) {
			t.Fatalf("expected report to contain %q, got:\n%s", want, string(reportData))
		}
	}
	plannerSuggestion := "closed-loop planner preview selected next task"
	if !strings.Contains(string(reportData), plannerSuggestion) {
		t.Fatalf("expected report to contain planner suggestion %q, got:\n%s", plannerSuggestion, string(reportData))
	}

	store, err := eventlog.OpenSQLite(filepath.Join(root, "replay", "events.db"))
	if err != nil {
		t.Fatalf("open event log: %v", err)
	}
	defer store.Close()

	events, err := store.Replay(ctx, result.RunID)
	if err != nil {
		t.Fatalf("replay events: %v", err)
	}
	if len(events) != result.EventCount {
		t.Fatalf("expected %d events, got %d", result.EventCount, len(events))
	}
	snapshot, err := eventlog.BuildReplaySnapshot(events)
	if err != nil {
		t.Fatalf("build replay snapshot: %v", err)
	}
	if snapshot.Status.Model != "offline-model" || snapshot.Status.ToolHealth != "ready" || len(snapshot.Timeline) == 0 {
		t.Fatalf("unexpected replay snapshot: %#v", snapshot)
	}
	if !snapshotHasFact(snapshot, "a-closed-loop-preview", "suggestion", "closed-loop-planner", plannerSuggestion) {
		t.Fatalf("expected closed-loop planner suggestion fact in replay snapshot, facts=%#v", snapshot.Blackboard.Facts)
	}
	if !snapshotHasHint(snapshot, "hint-closed-loop-planner-preview", "planning", "closed-loop-planner", plannerSuggestion) {
		t.Fatalf("expected closed-loop planner hint in replay snapshot, hints=%#v", snapshot.Blackboard.Hints)
	}
	if !snapshotHasAgent(snapshot, "closed-loop-planner", "planner", "Complete") {
		t.Fatalf("expected closed-loop planner agent in replay snapshot, agents=%#v", snapshot.Agents)
	}
	assertPlannerPreviewBeforeReportProjection(t, events)
	if !strings.Contains(result.TUIView, "a-closed-loop-preview") {
		t.Fatalf("expected TUI view to expose planner preview, got:\n%s", result.TUIView)
	}
}

func TestRunWorkspaceRecordsPlannerSelectedCompilePreviewReview(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}

	result, err := RunWorkspaceWithOptions(ctx, root, RunOptions{Action: compiler.ActionEnumeratePaths})
	if err != nil {
		t.Fatalf("run workspace: %v", err)
	}

	reviews, err := ListToolCompilePreviewReviews(root)
	if err != nil {
		t.Fatalf("list compile preview reviews: %v", err)
	}
	if len(reviews) != 1 {
		t.Fatalf("expected one planner-selected compile preview review, got %#v", reviews)
	}
	review := reviews[0]
	if review.Status != "dry-run" || review.Mode != "metadata-only" {
		t.Fatalf("unexpected review state: %#v", review)
	}
	if review.Preview.Action != compiler.ActionEnumeratePaths {
		t.Fatalf("expected planner-selected action %s, got %s", compiler.ActionEnumeratePaths, review.Preview.Action)
	}
	if len(review.Preview.Nodes) == 0 {
		t.Fatalf("expected planner-selected compile preview nodes, got %#v", review.Preview)
	}

	selection, found, err := ReadToolCompilePreviewReviewSelection(root)
	if err != nil {
		t.Fatalf("read compile preview selection: %v", err)
	}
	if !found {
		t.Fatalf("expected planner-selected compile preview selection")
	}
	if selection.Status != "current" || selection.Mode != "metadata-only" || selection.SelectedBy != closedLoopPlannerSource {
		t.Fatalf("unexpected compile preview selection: %#v", selection)
	}
	if selection.Action != compiler.ActionEnumeratePaths || selection.ReviewPath != review.Path || selection.ReviewNodes != len(review.Preview.Nodes) {
		t.Fatalf("selection does not reference planner review: selection=%#v review=%#v", selection, review)
	}

	reportData, err := os.ReadFile(result.ReportPath)
	if err != nil {
		t.Fatalf("read report: %v", err)
	}
	for _, want := range []string{
		"planner-selected compile preview action enumerate_paths",
		"metadata-only compile preview review",
	} {
		if !strings.Contains(string(reportData), want) {
			t.Fatalf("expected report to contain %q, got:\n%s", want, string(reportData))
		}
	}
	model := NewTUIModelForWorkspace(root).SelectRuntimeTab(tui.RuntimeTabToolchainHealth)
	resized, cmd := model.Update(tea.WindowSizeMsg{Width: 720, Height: 44})
	if cmd != nil {
		t.Fatalf("expected TUI resize update to be synchronous")
	}
	workspaceTUIView := resized.View()
	for _, want := range []string{
		"compile-preview review selection:",
		"selected_by=closed-loop-planner",
		"action=enumerate_paths",
		"compile-preview review node:",
	} {
		if !strings.Contains(workspaceTUIView, want) {
			t.Fatalf("expected TUI view to contain %q, got:\n%s", want, workspaceTUIView)
		}
	}

	store, err := eventlog.OpenSQLite(filepath.Join(root, "replay", "events.db"))
	if err != nil {
		t.Fatalf("open event log: %v", err)
	}
	defer store.Close()
	events, err := store.Replay(ctx, result.RunID)
	if err != nil {
		t.Fatalf("replay events: %v", err)
	}
	attempt := plannerReviewAttemptPayload(t, events)
	if attempt["selected_action"] != string(compiler.ActionEnumeratePaths) {
		t.Fatalf("expected planner review selected_action=%s, got %#v", compiler.ActionEnumeratePaths, attempt)
	}
	if attempt["compile_preview_mode"] != "metadata-only" || attempt["compile_preview_review_path"] != review.Path || attempt["compile_preview_selection_path"] != selection.Path {
		t.Fatalf("expected planner review compile preview metadata, got %#v", attempt)
	}
	if nodes, ok := attempt["compile_preview_nodes"].(float64); !ok || int(nodes) != len(review.Preview.Nodes) {
		t.Fatalf("expected planner review node count %d, got %#v", len(review.Preview.Nodes), attempt)
	}
	assertPlannerPreviewBeforeReportProjection(t, events)
}

func TestRunWorkspaceCanDeferReportProjection(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}

	result, err := RunWorkspaceWithOptions(ctx, root, RunOptions{DeferReportProjection: true})
	if err != nil {
		t.Fatalf("run workspace with deferred report projection: %v", err)
	}
	if result.ReportPath != "" {
		t.Fatalf("deferred report projection should not return a report path, got %#v", result)
	}
	reportFiles, err := filepath.Glob(filepath.Join(root, "reports", "*.md"))
	if err != nil {
		t.Fatalf("glob reports: %v", err)
	}
	if len(reportFiles) != 0 {
		t.Fatalf("deferred report projection should not write reports, got %#v", reportFiles)
	}

	store, err := eventlog.OpenSQLite(filepath.Join(root, "replay", "events.db"))
	if err != nil {
		t.Fatalf("open event log: %v", err)
	}
	defer store.Close()
	events, err := store.Replay(ctx, result.RunID)
	if err != nil {
		t.Fatalf("replay events: %v", err)
	}
	if replayReportPath(events) != "" {
		t.Fatalf("deferred report projection should not emit report_ready, got %q", replayReportPath(events))
	}
	lastStage := lastStageAttempt(t, events)
	if lastStage.StageID != "report-projection" || lastStage.Status != "skipped" || lastStage.Reason != "deferred" {
		t.Fatalf("expected deferred report projection stage, got %#v", lastStage)
	}
}

func TestRunWorkspacePersistsWorkspaceSafetyAuditEvent(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	scope := `version: 1
scope:
  target_policy: disabled
  targets: []
  exclusions: []
execution:
  isolation: disabled
`
	if err := os.WriteFile(filepath.Join(root, "scope.yaml"), []byte(scope), 0o644); err != nil {
		t.Fatalf("write scope: %v", err)
	}

	result, err := RunWorkspaceWithOptions(ctx, root, RunOptions{DeferReportProjection: true})
	if err != nil {
		t.Fatalf("run workspace with safety bypass audit: %v", err)
	}

	store, err := eventlog.OpenSQLite(filepath.Join(root, "replay", "events.db"))
	if err != nil {
		t.Fatalf("open event log: %v", err)
	}
	defer store.Close()
	events, err := store.Replay(ctx, result.RunID)
	if err != nil {
		t.Fatalf("replay events: %v", err)
	}
	if len(events) == 0 {
		t.Fatalf("expected run events")
	}
	if events[0].Type != eventlog.EventVerification {
		t.Fatalf("expected workspace safety audit to be the first event, got %#v", events[0])
	}
	payload := decodeWorkspaceSafetyPayloadForTest(t, events[0])
	if payload.Kind != "workspace_safety" || payload.Status != "warning" || payload.StrictGate.Mode != "audit" || payload.StrictGate.Status != "warning" {
		t.Fatalf("unexpected workspace safety audit payload: %#v", payload)
	}
	if payload.TargetPolicy != "disabled" || payload.Isolation != "disabled" || payload.RequireSafetyAuthorization {
		t.Fatalf("unexpected workspace safety boundary fields: %#v", payload)
	}
	if len(payload.SafetyBypasses) != 2 {
		t.Fatalf("expected two safety bypass entries, got %#v", payload.SafetyBypasses)
	}
	for _, bypass := range payload.SafetyBypasses {
		if bypass.Authorized || !bypass.MissingAuthorization || bypass.ExpiredAuthorization {
			t.Fatalf("expected missing authorization metadata in audit bypass, got %#v", bypass)
		}
	}
	if payload.WarningCount == 0 || payload.ErrorCount != 0 {
		t.Fatalf("expected warning-only safety audit, got %#v", payload)
	}
}

func TestRunWorkspaceRequireSafetyAuthorizationBlocksMissingSafetyBypassAndPersistsAudit(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	scope := `version: 1
scope:
  target_policy: disabled
  targets: []
  exclusions: []
execution:
  isolation: process
`
	if err := os.WriteFile(filepath.Join(root, "scope.yaml"), []byte(scope), 0o644); err != nil {
		t.Fatalf("write scope: %v", err)
	}
	runner := &recordingToolExecutionRunner{}
	observed := []eventlog.Event{}

	_, err := RunWorkspaceWithOptions(ctx, root, RunOptions{
		RequireSafetyAuthorization: true,
		ToolRunner:                 runner,
		OnEvent: func(event eventlog.Event) error {
			observed = append(observed, event)
			return nil
		},
	})
	if err == nil || !strings.Contains(err.Error(), "workspace is not ready") || !strings.Contains(err.Error(), "missing required safety bypass authorization for scope.target_policy disabled") {
		t.Fatalf("expected strict safety authorization run failure, got %v", err)
	}
	if len(runner.requests) != 0 {
		t.Fatalf("strict safety gate must block before tool execution, got %#v", runner.requests)
	}
	if len(observed) != 1 {
		t.Fatalf("expected one persisted blocked workspace safety event, got %#v", observed)
	}
	payload := decodeWorkspaceSafetyPayloadForTest(t, observed[0])
	if observed[0].Type != eventlog.EventVerification || payload.Kind != "workspace_safety" || payload.Status != "blocked" {
		t.Fatalf("expected blocked workspace safety event, event=%#v payload=%#v", observed[0], payload)
	}
	if !payload.RequireSafetyAuthorization || payload.StrictGate.Mode != "strict" || payload.StrictGate.Status != "blocked" || payload.StrictGate.ErrorCount != 1 {
		t.Fatalf("unexpected strict safety gate payload: %#v", payload)
	}
	if len(payload.SafetyBypasses) != 1 || !payload.SafetyBypasses[0].MissingAuthorization {
		t.Fatalf("expected missing target-policy authorization in blocked payload: %#v", payload.SafetyBypasses)
	}
	if reports, err := filepath.Glob(filepath.Join(root, "reports", "*.md")); err != nil || len(reports) != 0 {
		t.Fatalf("strict safety gate must not write reports, reports=%#v err=%v", reports, err)
	}
	leases, listErr := ListWorkerLeases(root)
	if listErr != nil {
		t.Fatalf("list worker leases after strict safety gate: %v", listErr)
	}
	if len(leases) != 0 {
		t.Fatalf("strict safety gate must not write worker leases, got %#v", leases)
	}
}

func TestProjectRunReportWritesReportForDeferredRun(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	run, err := RunWorkspaceWithOptions(ctx, root, RunOptions{DeferReportProjection: true})
	if err != nil {
		t.Fatalf("run workspace with deferred report projection: %v", err)
	}

	dryRun, err := ProjectRunReport(ctx, root, run.RunID, ReportProjectionOptions{})
	if err != nil {
		t.Fatalf("dry-run project report: %v", err)
	}
	if dryRun.Status != "dry-run" || dryRun.Mode != "metadata-only" || dryRun.ReportPath == "" {
		t.Fatalf("unexpected dry-run report projection result: %#v", dryRun)
	}
	if _, err := os.Stat(dryRun.ReportPath); !os.IsNotExist(err) {
		t.Fatalf("dry-run report projection should not write report, stat err=%v", err)
	}

	written, err := ProjectRunReport(ctx, root, run.RunID, ReportProjectionOptions{Write: true})
	if err != nil {
		t.Fatalf("write project report: %v", err)
	}
	if written.Status != "completed" || written.Mode != "write" || !written.Written {
		t.Fatalf("unexpected written report projection result: %#v", written)
	}
	reportData, err := os.ReadFile(written.ReportPath)
	if err != nil {
		t.Fatalf("read projected report: %v", err)
	}
	if !strings.Contains(string(reportData), "# Ouroboros Report") {
		t.Fatalf("expected markdown report, got:\n%s", string(reportData))
	}

	replay, err := ReplayWorkspace(ctx, root, run.RunID)
	if err != nil {
		t.Fatalf("replay projected run: %v", err)
	}
	if replay.ReportPath != written.ReportPath {
		t.Fatalf("expected replay report path %q, got %q", written.ReportPath, replay.ReportPath)
	}
	store, err := eventlog.OpenSQLite(filepath.Join(root, "replay", "events.db"))
	if err != nil {
		t.Fatalf("open event log: %v", err)
	}
	defer store.Close()
	events, err := store.Replay(ctx, run.RunID)
	if err != nil {
		t.Fatalf("replay events: %v", err)
	}
	lastStage := lastStageAttempt(t, events)
	if lastStage.StageID != "report-projection" || lastStage.Status != "completed" {
		t.Fatalf("expected completed report projection stage, got %#v", lastStage)
	}
}

func TestProjectRunReportRejectsUnsafeRunIDBeforeWriting(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	store, err := eventlog.OpenSQLite(filepath.Join(root, "replay", "events.db"))
	if err != nil {
		t.Fatalf("open event log: %v", err)
	}
	unsafeRunID := "../escape"
	if _, err := store.Append(ctx, eventlog.Event{
		RunID: unsafeRunID,
		Type:  eventlog.EventAttempt,
		Payload: mustJSON(map[string]any{
			"attempt_id": "stage-workspace-bootstrap-completed",
			"kind":       "stage",
			"stage_id":   "workspace-bootstrap",
			"status":     "completed",
		}),
	}); err != nil {
		t.Fatalf("append unsafe run event: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close event log: %v", err)
	}

	_, err = ProjectRunReport(ctx, root, unsafeRunID, ReportProjectionOptions{Write: true})
	if err == nil || !strings.Contains(err.Error(), "report projection run id must be a safe basename") {
		t.Fatalf("expected unsafe run id error, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "escape.md")); !os.IsNotExist(err) {
		t.Fatalf("unsafe report projection must not write outside reports, stat err=%v", err)
	}
}

func TestProjectRunReportDoesNotEmitReportReadyWhenWriteFails(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	run, err := RunWorkspaceWithOptions(ctx, root, RunOptions{DeferReportProjection: true})
	if err != nil {
		t.Fatalf("run workspace with deferred report projection: %v", err)
	}
	blockingDir := filepath.Join(root, "reports", run.RunID+".md")
	if err := os.MkdirAll(blockingDir, 0o755); err != nil {
		t.Fatalf("create blocking report directory: %v", err)
	}

	_, err = ProjectRunReport(ctx, root, run.RunID, ReportProjectionOptions{Write: true})
	if err == nil || !strings.Contains(err.Error(), "write report") {
		t.Fatalf("expected report write error, got %v", err)
	}
	replay, err := ReplayWorkspace(ctx, root, run.RunID)
	if err != nil {
		t.Fatalf("replay after failed report projection: %v", err)
	}
	if replay.ReportPath != "" {
		t.Fatalf("failed report write must not leave report_ready, got %q", replay.ReportPath)
	}
}

func TestProjectRunReportDoesNotRunContainerRuntimeDoctor(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	run, err := RunWorkspaceWithOptions(ctx, root, RunOptions{DeferReportProjection: true})
	if err != nil {
		t.Fatalf("run workspace with deferred report projection: %v", err)
	}
	engineDir := t.TempDir()
	engine := filepath.Join(engineDir, "fake-container-engine")
	marker := filepath.Join(engineDir, "container-runtime-was-called")
	engineScript := `#!/bin/sh
touch "` + marker + `"
printf 'container runtime should not be inspected during report projection\n' >&2
exit 1
`
	if err := os.WriteFile(engine, []byte(engineScript), 0o755); err != nil {
		t.Fatalf("write fake engine: %v", err)
	}
	scope := `version: 1
scope:
  target_policy: enforce
  targets: []
  exclusions: []
execution:
  isolation: container
  container:
    engine: ` + engine + `
    image: ouro-worker:test
`
	if err := os.WriteFile(filepath.Join(root, "scope.yaml"), []byte(scope), 0o644); err != nil {
		t.Fatalf("write container scope: %v", err)
	}

	_, err = ProjectRunReport(ctx, root, run.RunID, ReportProjectionOptions{Write: true})
	if err != nil {
		t.Fatalf("project report should not run container doctor checks: %v", err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("report projection must not inspect container runtime, marker stat err=%v", err)
	}
}

func TestProjectRunReportRejectsSymlinkedReportsDirectory(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	run, err := RunWorkspaceWithOptions(ctx, root, RunOptions{DeferReportProjection: true})
	if err != nil {
		t.Fatalf("run workspace with deferred report projection: %v", err)
	}
	outsideReports := filepath.Join(t.TempDir(), "outside-reports")
	if err := os.MkdirAll(outsideReports, 0o755); err != nil {
		t.Fatalf("create outside reports dir: %v", err)
	}
	reportsDir := filepath.Join(root, "reports")
	if err := os.Remove(reportsDir); err != nil {
		t.Fatalf("remove workspace reports dir: %v", err)
	}
	if err := os.Symlink(outsideReports, reportsDir); err != nil {
		t.Fatalf("symlink reports dir: %v", err)
	}

	_, err = ProjectRunReport(ctx, root, run.RunID, ReportProjectionOptions{Write: true})
	if err == nil || !strings.Contains(err.Error(), "reports directory must not be a symlink") {
		t.Fatalf("expected symlinked reports directory error, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(outsideReports, run.RunID+".md")); !os.IsNotExist(err) {
		t.Fatalf("report projection must not write through symlinked reports dir, stat err=%v", err)
	}
}

func TestProjectRunReportRejectsSymlinkedReportFile(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	run, err := RunWorkspaceWithOptions(ctx, root, RunOptions{DeferReportProjection: true})
	if err != nil {
		t.Fatalf("run workspace with deferred report projection: %v", err)
	}
	outsideReport := filepath.Join(t.TempDir(), "escaped-report.md")
	reportPath := filepath.Join(root, "reports", run.RunID+".md")
	if err := os.Symlink(outsideReport, reportPath); err != nil {
		t.Fatalf("symlink report file: %v", err)
	}

	_, err = ProjectRunReport(ctx, root, run.RunID, ReportProjectionOptions{Write: true})
	if err == nil || !strings.Contains(err.Error(), "report file must not be a symlink") {
		t.Fatalf("expected symlinked report file error, got %v", err)
	}
	if _, err := os.Stat(outsideReport); !os.IsNotExist(err) {
		t.Fatalf("report projection must not write through symlinked report file, stat err=%v", err)
	}
	replay, err := ReplayWorkspace(ctx, root, run.RunID)
	if err != nil {
		t.Fatalf("replay after symlinked report projection: %v", err)
	}
	if replay.ReportPath != "" {
		t.Fatalf("failed symlinked report write must not leave report_ready, got %q", replay.ReportPath)
	}
}

func TestRunWorkspaceRecordsToolchainDependencyHealth(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	manifest := `name: web-api-pack
version: "0.1.0"
capabilities:
  - web.discovery
runner: native
parser: web_api_parser
healthcheck: built_in
dependencies:
  - name: ` + filepath.Join(t.TempDir(), "missing-httpx") + `
    kind: binary
    install_hint: install bundled httpx pack
    install_command:
      - ouro-toolchain
      - install
      - httpx
`
	if err := os.WriteFile(filepath.Join(root, "toolchain", "web-api-pack", "manifest.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	result, err := RunWorkspace(ctx, root)
	if err != nil {
		t.Fatalf("run workspace: %v", err)
	}
	store, err := eventlog.OpenSQLite(filepath.Join(root, "replay", "events.db"))
	if err != nil {
		t.Fatalf("open event log: %v", err)
	}
	defer store.Close()
	events, err := store.Replay(ctx, result.RunID)
	if err != nil {
		t.Fatalf("replay events: %v", err)
	}
	snapshot, err := eventlog.BuildReplaySnapshot(events)
	if err != nil {
		t.Fatalf("build replay snapshot: %v", err)
	}
	for _, status := range snapshot.Toolchain {
		if status.Name != "web-api-pack" {
			continue
		}
		if status.Health != "missing" || len(status.Dependencies) != 1 {
			t.Fatalf("expected missing dependency status, got %#v", status)
		}
		if status.Dependencies[0].State != "missing" || !status.Dependencies[0].Required {
			t.Fatalf("expected required missing dependency, got %#v", status.Dependencies[0])
		}
		if len(status.InstallPlan) != 1 || status.InstallPlan[0].Name == "" {
			t.Fatalf("expected replayed install plan, got %#v", status.InstallPlan)
		}
		if strings.Join(status.InstallPlan[0].Command, " ") != "ouro-toolchain install httpx" {
			t.Fatalf("expected install command in replay snapshot, got %#v", status.InstallPlan[0])
		}
		return
	}
	t.Fatalf("expected web-api-pack toolchain dependency status in %#v", snapshot.Toolchain)
}

func TestRunWorkspaceUsesDomainPacksForScopeTargets(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	scope := `version: 1
scope:
  targets:
    - kind: url
      value: https://app.example
    - kind: cidr
      value: 10.0.0.0/24
    - kind: challenge
      value: pwnable-login
  exclusions: []
`
	if err := os.WriteFile(filepath.Join(root, "scope.yaml"), []byte(scope), 0o644); err != nil {
		t.Fatalf("write scope: %v", err)
	}

	result, err := RunWorkspace(ctx, root)
	if err != nil {
		t.Fatalf("run workspace: %v", err)
	}

	store, err := eventlog.OpenSQLite(filepath.Join(root, "replay", "events.db"))
	if err != nil {
		t.Fatalf("open event log: %v", err)
	}
	defer store.Close()

	events, err := store.Replay(ctx, result.RunID)
	if err != nil {
		t.Fatalf("replay events: %v", err)
	}
	intentIDs := map[string]bool{}
	for _, event := range events {
		if event.Type != eventlog.EventIntent {
			continue
		}
		var payload struct {
			IntentID string `json:"intent_id"`
		}
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			t.Fatalf("decode intent payload: %v", err)
		}
		intentIDs[payload.IntentID] = true
	}
	for _, want := range []string{"web-discover-surface", "infra-discover-hosts", "ctf-classify-challenge"} {
		if !intentIDs[want] {
			t.Fatalf("expected domain pack hypothesis %s in intents %#v", want, intentIDs)
		}
	}
}

type stageAttemptPayload struct {
	Kind    string `json:"kind"`
	StageID string `json:"stage_id"`
	Status  string `json:"status"`
	Reason  string `json:"reason"`
}

type workspaceSafetyPayloadForTest struct {
	Kind                       string                  `json:"kind"`
	Status                     string                  `json:"status"`
	TargetPolicy               string                  `json:"target_policy"`
	Isolation                  string                  `json:"isolation"`
	RequireSafetyAuthorization bool                    `json:"require_safety_authorization"`
	WarningCount               int                     `json:"warning_count"`
	ErrorCount                 int                     `json:"error_count"`
	Warnings                   []string                `json:"warnings"`
	Errors                     []string                `json:"errors"`
	SafetyBypasses             []DoctorSafetyBypass    `json:"safety_bypasses"`
	StrictGate                 workspaceSafetyGateTest `json:"strict_gate"`
}

type workspaceSafetyGateTest struct {
	Mode       string `json:"mode"`
	Status     string `json:"status"`
	ErrorCount int    `json:"error_count"`
}

func decodeWorkspaceSafetyPayloadForTest(t *testing.T, event eventlog.Event) workspaceSafetyPayloadForTest {
	t.Helper()
	var payload workspaceSafetyPayloadForTest
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		t.Fatalf("decode workspace safety payload: %v json=%s", err, string(event.Payload))
	}
	return payload
}

func lastStageAttempt(t *testing.T, events []eventlog.Event) stageAttemptPayload {
	t.Helper()
	var last stageAttemptPayload
	for _, event := range events {
		if event.Type != eventlog.EventAttempt {
			continue
		}
		var payload stageAttemptPayload
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			t.Fatalf("decode stage attempt payload: %v", err)
		}
		if payload.Kind == "stage" {
			last = payload
		}
	}
	if last.StageID == "" {
		t.Fatalf("expected at least one stage attempt in %#v", events)
	}
	return last
}

func snapshotHasFact(snapshot eventlog.ReplaySnapshot, id string, kind string, source string, statementSubstring string) bool {
	for _, fact := range snapshot.Blackboard.Facts {
		if fact.ID == id && fact.Kind == kind && fact.Source == source && strings.Contains(fact.Statement, statementSubstring) {
			return true
		}
	}
	return false
}

func snapshotHasHint(snapshot eventlog.ReplaySnapshot, id string, kind string, source string, messageSubstring string) bool {
	for _, hint := range snapshot.Blackboard.Hints {
		if hint.ID == id && hint.Kind == kind && hint.Source == source && strings.Contains(hint.Message, messageSubstring) {
			return true
		}
	}
	return false
}

func snapshotHasAgent(snapshot eventlog.ReplaySnapshot, id string, role string, state string) bool {
	for _, agent := range snapshot.Agents {
		if agent.ID == id && agent.Role == role && agent.State == state {
			return true
		}
	}
	return false
}

func plannerReviewAttemptPayload(t *testing.T, events []eventlog.Event) map[string]any {
	t.Helper()
	for _, event := range events {
		if event.Type != eventlog.EventAttempt {
			continue
		}
		var payload map[string]any
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			t.Fatalf("decode attempt payload: %v", err)
		}
		if payload["kind"] == "planner_review" {
			return payload
		}
	}
	t.Fatalf("expected planner_review attempt in %#v", events)
	return nil
}

func TestClosedLoopPlannerSelectedActionUsesLatestMatchingIntentPayload(t *testing.T) {
	action, found, err := closedLoopPlannerSelectedAction(sisyphusTask("intent-repeat"), []eventlog.Event{
		{
			Type: eventlog.EventIntent,
			Payload: mustJSON(map[string]any{
				"intent_id": "intent-repeat",
				"kind":      "run",
				"goal":      "older payload without action",
				"state":     "Pending",
			}),
		},
		{
			Type: eventlog.EventIntent,
			Payload: mustJSON(map[string]any{
				"intent_id": "intent-repeat",
				"kind":      "run",
				"goal":      "newer payload with action",
				"state":     "Open",
				"action":    string(compiler.ActionEnumeratePaths),
			}),
		},
	})
	if err != nil {
		t.Fatalf("selected action: %v", err)
	}
	if !found || action != compiler.ActionEnumeratePaths {
		t.Fatalf("expected latest matching action %s, found=%v action=%s", compiler.ActionEnumeratePaths, found, action)
	}
}

func sisyphusTask(intentID string) sisyphus.Task {
	return sisyphus.Task{Kind: sisyphus.TaskExplore, IntentID: intentID}
}

func assertPlannerPreviewBeforeReportProjection(t *testing.T, events []eventlog.Event) {
	t.Helper()
	plannerCompletedIndex := -1
	reportStartedIndex := -1
	for i, event := range events {
		if event.Type != eventlog.EventAttempt {
			continue
		}
		var payload stageAttemptPayload
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			t.Fatalf("decode attempt payload: %v", err)
		}
		if payload.Kind != "stage" {
			continue
		}
		if payload.StageID == "closed-loop-planner-preview" && payload.Status == "completed" {
			plannerCompletedIndex = i
		}
		if payload.StageID == "report-projection" && payload.Status == "started" {
			reportStartedIndex = i
		}
	}
	if plannerCompletedIndex < 0 {
		t.Fatalf("expected completed closed-loop planner preview stage in events")
	}
	if reportStartedIndex < 0 {
		t.Fatalf("expected started report projection stage in events")
	}
	if plannerCompletedIndex > reportStartedIndex {
		t.Fatalf("planner preview must complete before report projection starts, planner=%d report=%d", plannerCompletedIndex, reportStartedIndex)
	}
}

func TestRunWorkspaceRecordsObservableStageAttemptEvents(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}

	result, err := RunWorkspace(ctx, root)
	if err != nil {
		t.Fatalf("run workspace: %v", err)
	}

	store, err := eventlog.OpenSQLite(filepath.Join(root, "replay", "events.db"))
	if err != nil {
		t.Fatalf("open event log: %v", err)
	}
	defer store.Close()

	events, err := store.Replay(ctx, result.RunID)
	if err != nil {
		t.Fatalf("replay events: %v", err)
	}
	stages := []string{}
	for _, event := range events {
		if event.Type != eventlog.EventAttempt {
			continue
		}
		var payload struct {
			Kind    string `json:"kind"`
			StageID string `json:"stage_id"`
			Status  string `json:"status"`
		}
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			t.Fatalf("decode stage attempt payload: %v", err)
		}
		if payload.Kind == "stage" {
			stages = append(stages, payload.StageID+":"+payload.Status)
		}
	}

	want := []string{
		"workspace-bootstrap:started",
		"workspace-bootstrap:completed",
		"toolchain-health:started",
		"toolchain-health:completed",
		"hypothesis-seeding:started",
		"hypothesis-seeding:completed",
		"tool-execution:started",
		"tool-execution:completed",
		"closed-loop-planner-preview:started",
		"closed-loop-planner-preview:completed",
		"report-projection:started",
		"report-projection:completed",
	}
	if strings.Join(stages, "\n") != strings.Join(want, "\n") {
		t.Fatalf("unexpected stage sequence:\n got: %#v\nwant: %#v", stages, want)
	}
}

func TestRunWorkspaceExecutesToolPlanThroughRunner(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	runner := &recordingToolExecutionRunner{}

	result, err := RunWorkspaceWithOptions(ctx, root, RunOptions{ToolRunner: runner})
	if err != nil {
		t.Fatalf("run workspace with tool runner: %v", err)
	}
	assertBuiltInDiscoverToolRequests(t, runner.requests, result.RunID, root)

	store, err := eventlog.OpenSQLite(filepath.Join(root, "replay", "events.db"))
	if err != nil {
		t.Fatalf("open event log: %v", err)
	}
	defer store.Close()
	events, err := store.Replay(ctx, result.RunID)
	if err != nil {
		t.Fatalf("replay events: %v", err)
	}
	executions := 0
	queued := 0
	started := 0
	completed := 0
	lifecycleByParser := map[string]map[string]int{}
	for _, event := range events {
		if event.Type != eventlog.EventTool {
			continue
		}
		var payload struct {
			ExecutionID      string `json:"execution_id"`
			ExecutionStatus  string `json:"execution_status"`
			WorkerStatus     string `json:"worker_status"`
			Capability       string `json:"capability"`
			WorkerCapability string `json:"worker_capability"`
			Action           string `json:"action"`
			Parser           string `json:"parser"`
		}
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			t.Fatalf("decode tool payload: %v", err)
		}
		if payload.ExecutionID == "" {
			continue
		}
		executions++
		switch {
		case payload.WorkerStatus == "queued":
			queued++
		case payload.WorkerStatus == "started":
			started++
		case payload.ExecutionStatus == "completed":
			completed++
		default:
			t.Fatalf("unexpected tool execution payload: %#v", payload)
		}
		capability := firstNonEmptyTestValue(payload.Capability, payload.WorkerCapability)
		if capability == "" || payload.Action != string(compiler.ActionDiscoverAssets) {
			t.Fatalf("unexpected tool execution payload: %#v", payload)
		}
		want := expectedBuiltInDiscoverNodeByParser(t, payload.Parser)
		if payload.Parser == "" || capability != want.Capability {
			t.Fatalf("unexpected tool execution parser/capability payload: %#v", payload)
		}
		if lifecycleByParser[payload.Parser] == nil {
			lifecycleByParser[payload.Parser] = map[string]int{}
		}
		switch {
		case payload.WorkerStatus != "":
			lifecycleByParser[payload.Parser][payload.WorkerStatus]++
		case payload.ExecutionStatus != "":
			lifecycleByParser[payload.Parser][payload.ExecutionStatus]++
		}
	}
	if queued != len(runner.requests) || started != len(runner.requests) || completed != len(runner.requests) {
		t.Fatalf("expected queued/started/completed lifecycle per request, queued=%d started=%d completed=%d executions=%d requests=%d", queued, started, completed, executions, len(runner.requests))
	}
	for _, want := range expectedBuiltInDiscoverPlanNodes() {
		lifecycle := lifecycleByParser[want.ParserID]
		if lifecycle["queued"] != 1 || lifecycle["started"] != 1 || lifecycle["completed"] != 1 {
			t.Fatalf("expected one queued/started/completed lifecycle for %s, got %#v", want.ParserID, lifecycle)
		}
	}
}

func TestRunWorkspaceExecutesToolPlanWithConfiguredConcurrency(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	scope := `version: 1
scope:
  target_policy: enforce
  targets: []
  exclusions: []
execution:
  isolation: process
  tool_concurrency: 3
`
	if err := os.WriteFile(filepath.Join(root, "scope.yaml"), []byte(scope), 0o644); err != nil {
		t.Fatalf("write scope: %v", err)
	}
	runner := &concurrentToolExecutionRunner{delay: 30 * time.Millisecond}

	result, err := RunWorkspaceWithOptions(ctx, root, RunOptions{ToolRunner: runner})
	if err != nil {
		t.Fatalf("run workspace with concurrent tool runner: %v", err)
	}
	if result.RunID == "" {
		t.Fatalf("expected run id")
	}
	if runner.maxActive < 2 {
		t.Fatalf("expected concurrent tool execution, max active=%d requests=%d", runner.maxActive, len(runner.requests))
	}
	assertBuiltInDiscoverToolRequests(t, runner.requests, result.RunID, root)
}

func TestRunWorkspaceHonorsPackPolicyMaxConcurrency(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	scope := `version: 1
scope:
  target_policy: enforce
  targets: []
  exclusions: []
execution:
  isolation: process
  tool_concurrency: 3
  pack_policies:
    - pack: web-api-pack
      mode: enabled
      reason: serialize heavier web probes
      max_concurrency: 1
`
	if err := os.WriteFile(filepath.Join(root, "scope.yaml"), []byte(scope), 0o644); err != nil {
		t.Fatalf("write scope: %v", err)
	}
	manifest := `name: web-api-pack
version: "0.1.0"
capabilities:
  - web.discovery
  - infra.host_discovery
runner: native
parser: web_api_parser
healthcheck: built_in
`
	if err := os.WriteFile(filepath.Join(root, "toolchain", "web-api-pack", "manifest.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	runner := &packConcurrencyProbeRunner{delay: 40 * time.Millisecond}

	result, err := RunWorkspaceWithOptions(ctx, root, RunOptions{ToolRunner: runner})
	if err != nil {
		t.Fatalf("run workspace with pack max concurrency: %v", err)
	}
	if runner.maxActiveByPack["web-api-pack"] != 1 {
		t.Fatalf("expected web-api-pack max concurrency 1, got max=%d requests=%#v", runner.maxActiveByPack["web-api-pack"], runner.requests)
	}
	if runner.maxActive < 2 {
		t.Fatalf("expected global tool_concurrency to still admit other packs concurrently, max active=%d", runner.maxActive)
	}

	store, err := eventlog.OpenSQLite(filepath.Join(root, "replay", "events.db"))
	if err != nil {
		t.Fatalf("open event log: %v", err)
	}
	defer store.Close()
	events, err := store.Replay(ctx, result.RunID)
	if err != nil {
		t.Fatalf("replay events: %v", err)
	}
	foundAdmission := false
	for _, event := range events {
		if event.Type != eventlog.EventTool {
			continue
		}
		var payload struct {
			Kind                string `json:"kind"`
			Tool                string `json:"tool"`
			AdmissionCapability string `json:"admission_capability"`
			AdmissionDecision   string `json:"admission_decision"`
			ExecutionPolicy     string `json:"execution_policy"`
			PackMaxConcurrency  int    `json:"pack_max_concurrency"`
		}
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			t.Fatalf("decode tool payload: %v", err)
		}
		if payload.Kind != "scheduler_admission" || payload.Tool != "web-api-pack" || payload.AdmissionCapability != "web.discovery" {
			continue
		}
		foundAdmission = true
		if payload.AdmissionDecision != "admitted" || payload.ExecutionPolicy != "enabled" || payload.PackMaxConcurrency != 1 {
			t.Fatalf("expected admission to carry pack max concurrency, got %#v", payload)
		}
	}
	if !foundAdmission {
		t.Fatalf("expected web-api-pack scheduler admission event with max concurrency")
	}
}

func TestRunWorkspaceHonorsExecutionResourceLimits(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	scope := `version: 1
scope:
  target_policy: enforce
  targets: []
  exclusions: []
execution:
  isolation: process
  tool_concurrency: 3
  resource_limits:
    cpu_units: 2
    memory_mb: 1024
  pack_policies:
    - pack: web-api-pack
      mode: enabled
      cpu_units: 2
      memory_mb: 1024
    - pack: infra-pack
      mode: enabled
      cpu_units: 1
      memory_mb: 512
    - pack: ctf-pack
      mode: enabled
      cpu_units: 1
      memory_mb: 512
`
	if err := os.WriteFile(filepath.Join(root, "scope.yaml"), []byte(scope), 0o644); err != nil {
		t.Fatalf("write scope: %v", err)
	}
	runner := &resourceLeaseProbeRunner{
		delay:        40 * time.Millisecond,
		cpuByPack:    map[string]int{"web-api-pack": 2, "infra-pack": 1, "ctf-pack": 1},
		memoryByPack: map[string]int{"web-api-pack": 1024, "infra-pack": 512, "ctf-pack": 512},
	}

	result, err := RunWorkspaceWithOptions(ctx, root, RunOptions{ToolRunner: runner})
	if err != nil {
		t.Fatalf("run workspace with resource limits: %v", err)
	}
	if runner.maxActiveCPU > 2 || runner.maxActiveMemory > 1024 {
		t.Fatalf("expected resource leases to cap active cpu/memory, max cpu=%d max memory=%d requests=%#v", runner.maxActiveCPU, runner.maxActiveMemory, runner.requests)
	}
	if runner.maxActive < 2 {
		t.Fatalf("expected resource lease scheduler to still allow compatible workers concurrently, max active=%d", runner.maxActive)
	}

	store, err := eventlog.OpenSQLite(filepath.Join(root, "replay", "events.db"))
	if err != nil {
		t.Fatalf("open event log: %v", err)
	}
	defer store.Close()
	events, err := store.Replay(ctx, result.RunID)
	if err != nil {
		t.Fatalf("replay events: %v", err)
	}
	foundAdmission := false
	for _, event := range events {
		if event.Type != eventlog.EventTool {
			continue
		}
		var payload struct {
			Kind                string `json:"kind"`
			Tool                string `json:"tool"`
			AdmissionCapability string `json:"admission_capability"`
			ResourceCPUUnits    int    `json:"resource_cpu_units"`
			ResourceMemoryMB    int    `json:"resource_memory_mb"`
			LimitCPUUnits       int    `json:"resource_limit_cpu_units"`
			LimitMemoryMB       int    `json:"resource_limit_memory_mb"`
		}
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			t.Fatalf("decode tool payload: %v", err)
		}
		if payload.Kind != "scheduler_admission" || payload.Tool != "web-api-pack" || payload.AdmissionCapability != "web.discovery" {
			continue
		}
		foundAdmission = true
		if payload.ResourceCPUUnits != 2 || payload.ResourceMemoryMB != 1024 || payload.LimitCPUUnits != 2 || payload.LimitMemoryMB != 1024 {
			t.Fatalf("expected admission to carry resource request/limit, got %#v", payload)
		}
	}
	if !foundAdmission {
		t.Fatalf("expected web-api-pack scheduler admission event with resource limits")
	}
}

func TestRunWorkspaceDoesNotOccupyGlobalWorkerSlotWhileWaitingForResources(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	scope := `version: 1
scope:
  target_policy: enforce
  targets: []
  exclusions: []
execution:
  isolation: process
  tool_concurrency: 2
  resource_limits:
    cpu_units: 2
  pack_policies:
    - pack: web-api-pack
      mode: enabled
      cpu_units: 2
    - pack: infra-pack
      mode: enabled
      cpu_units: 2
`
	if err := os.WriteFile(filepath.Join(root, "scope.yaml"), []byte(scope), 0o644); err != nil {
		t.Fatalf("write scope: %v", err)
	}
	runner := &resourceWaitDoesNotHoldWorkerSlotRunner{
		heavyPacks:     map[string]bool{"web-api-pack": true, "infra-pack": true},
		compatiblePack: "ctf-pack",
		heavyDelay:     80 * time.Millisecond,
	}

	if _, err := RunWorkspaceWithOptions(ctx, root, RunOptions{ToolRunner: runner}); err != nil {
		t.Fatalf("run workspace with resource wait scheduling: %v", err)
	}
	if !runner.CompatibleStartedBeforeAnyHeavyCompleted() {
		t.Fatalf("expected compatible ctf worker to start while infra waited for CPU lease; requests=%#v", runner.RequestedPacks())
	}
}

func TestRunWorkspaceRecordsSchedulerWaitReason(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	scope := `version: 1
scope:
  target_policy: enforce
  targets: []
  exclusions: []
execution:
  isolation: process
  tool_concurrency: 3
  resource_limits:
    cpu_units: 2
  pack_policies:
    - pack: web-api-pack
      mode: enabled
      cpu_units: 2
    - pack: infra-pack
      mode: enabled
      cpu_units: 2
`
	if err := os.WriteFile(filepath.Join(root, "scope.yaml"), []byte(scope), 0o644); err != nil {
		t.Fatalf("write scope: %v", err)
	}
	runner := &resourceWaitDoesNotHoldWorkerSlotRunner{
		heavyPacks:     map[string]bool{"web-api-pack": true, "infra-pack": true},
		compatiblePack: "ctf-pack",
		heavyDelay:     80 * time.Millisecond,
	}

	result, err := RunWorkspaceWithOptions(ctx, root, RunOptions{ToolRunner: runner})
	if err != nil {
		t.Fatalf("run workspace with resource wait reason: %v", err)
	}
	store, err := eventlog.OpenSQLite(filepath.Join(root, "replay", "events.db"))
	if err != nil {
		t.Fatalf("open event log: %v", err)
	}
	defer store.Close()
	events, err := store.Replay(ctx, result.RunID)
	if err != nil {
		t.Fatalf("replay events: %v", err)
	}
	foundWaiting := false
	for _, event := range events {
		if event.Type != eventlog.EventTool {
			continue
		}
		var payload struct {
			WorkerStatus string `json:"worker_status"`
			WaitReason   string `json:"wait_reason"`
			Capability   string `json:"worker_capability"`
		}
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			t.Fatalf("decode tool payload: %v", err)
		}
		if payload.WorkerStatus == "waiting" && payload.WaitReason == "cpu_units" && payload.Capability != "" {
			foundWaiting = true
			break
		}
	}
	if !foundWaiting {
		t.Fatalf("expected durable waiting worker event with cpu_units reason")
	}
}

func TestRunWorkspacePauseControllerDelaysWorkerStartUntilResume(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	scope := `version: 1
scope:
  target_policy: enforce
  targets: []
  exclusions: []
execution:
  isolation: process
  tool_concurrency: 2
`
	if err := os.WriteFile(filepath.Join(root, "scope.yaml"), []byte(scope), 0o644); err != nil {
		t.Fatalf("write scope: %v", err)
	}
	controller := NewRunPauseController()
	controller.Pause()
	runner := &pauseResumeProbeRunner{started: make(chan struct{}, 1)}
	events := make(chan eventlog.Event, 64)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := RunWorkspaceWithOptions(ctx, root, RunOptions{
			ToolRunner:      runner,
			PauseController: controller,
			OnEvent: func(event eventlog.Event) error {
				select {
				case events <- event:
				default:
				}
				return nil
			},
		})
		done <- err
	}()

	waitForToolWaitingReason(t, events, "run_paused")
	if count := runner.StartCount(); count != 0 {
		t.Fatalf("paused run should not start workers before resume, started=%d", count)
	}
	controller.Resume()
	select {
	case <-runner.started:
	case <-time.After(time.Second):
		t.Fatalf("expected worker to start after resume")
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("run workspace after resume: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatalf("expected run to complete after resume")
	}
}

func TestToolAdmissionLimiterDoesNotReserveResourcesWhileWaitingForGlobalSlot(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	running := ToolExecutionRequest{
		Node: compiler.PlanNode{
			ID:           "running",
			ManifestName: "ctf-pack",
		},
	}
	heavy := ToolExecutionRequest{
		Node: compiler.PlanNode{
			ID:           "heavy",
			ManifestName: "web-api-pack",
		},
		ResourceCPUUnits:      2,
		ResourceLimitCPUUnits: 2,
	}
	limiter := newToolAdmissionLimiter([]ToolExecutionRequest{running, heavy}, 1)

	releaseRunning, err := limiter.acquire(ctx, running, nil)
	if err != nil {
		t.Fatalf("acquire running request: %v", err)
	}

	heavyAcquired := make(chan func(), 1)
	go func() {
		release, err := limiter.acquire(ctx, heavy, nil)
		if err != nil {
			return
		}
		heavyAcquired <- release
	}()

	select {
	case release := <-heavyAcquired:
		release()
		t.Fatalf("heavy request acquired admission while global slot was full")
	case <-time.After(20 * time.Millisecond):
	}

	limiter.mu.Lock()
	activeCPU := limiter.activeCPU
	activeGlobal := limiter.activeGlobal
	limiter.mu.Unlock()
	if activeCPU != 0 || activeGlobal != 1 {
		t.Fatalf("waiting heavy request must not reserve resources, activeCPU=%d activeGlobal=%d", activeCPU, activeGlobal)
	}

	releaseRunning()
	select {
	case releaseHeavy := <-heavyAcquired:
		releaseHeavy()
	case <-ctx.Done():
		t.Fatalf("heavy request did not acquire admission after global slot was released")
	}
}

func TestRunWorkspaceStreamsToolCompletionBeforeAllWorkersFinish(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	scope := `version: 1
scope:
  target_policy: enforce
  targets: []
  exclusions: []
execution:
  isolation: process
  tool_concurrency: 3
`
	if err := os.WriteFile(filepath.Join(root, "scope.yaml"), []byte(scope), 0o644); err != nil {
		t.Fatalf("write scope: %v", err)
	}
	releaseSlow := make(chan struct{})
	var releaseSlowOnce sync.Once
	runner := &streamingCompletionToolExecutionRunner{
		slowCapability: "infra.host_discovery",
		slowRelease:    releaseSlow,
	}

	var observedMu sync.Mutex
	observedCompletedBeforeSlowDone := false
	_, err := RunWorkspaceWithOptions(ctx, root, RunOptions{
		ToolRunner: runner,
		OnSnapshot: func(snapshot eventlog.ReplaySnapshot) error {
			for _, execution := range snapshot.ToolExecutions {
				if execution.Capability != "web.discovery" || execution.Status != "completed" {
					continue
				}
				if !runner.SlowDone() {
					observedMu.Lock()
					observedCompletedBeforeSlowDone = true
					observedMu.Unlock()
					releaseSlowOnce.Do(func() {
						close(releaseSlow)
					})
				}
			}
			return nil
		},
	})
	if err != nil {
		t.Fatalf("run workspace with streaming completion runner: %v", err)
	}
	observedMu.Lock()
	defer observedMu.Unlock()
	if !observedCompletedBeforeSlowDone {
		t.Fatalf("expected fast tool completion snapshot before slow worker finished")
	}
}

func TestRunWorkspaceSkipsDisabledPackExecutionPolicy(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	scope := `version: 1
scope:
  target_policy: enforce
  targets: []
  exclusions: []
execution:
  isolation: process
  pack_policies:
    - pack: web-api-pack
      mode: disabled
      reason: noisy active scan deferred
`
	if err := os.WriteFile(filepath.Join(root, "scope.yaml"), []byte(scope), 0o644); err != nil {
		t.Fatalf("write scope: %v", err)
	}
	runner := &recordingToolExecutionRunner{}

	result, err := RunWorkspaceWithOptions(ctx, root, RunOptions{ToolRunner: runner})
	if err != nil {
		t.Fatalf("run workspace with pack execution policy: %v", err)
	}
	if result.ReportPath == "" {
		t.Fatalf("expected report after skipping disabled pack")
	}
	if len(runner.requests) != 6 {
		t.Fatalf("expected disabled web-api-pack to be skipped before runner execution, got %#v", runner.requests)
	}
	for _, request := range runner.requests {
		if request.Node.ManifestName == "web-api-pack" {
			t.Fatalf("disabled web-api-pack must not execute, got %#v", runner.requests)
		}
	}

	store, err := eventlog.OpenSQLite(filepath.Join(root, "replay", "events.db"))
	if err != nil {
		t.Fatalf("open event log: %v", err)
	}
	defer store.Close()
	events, err := store.Replay(ctx, result.RunID)
	if err != nil {
		t.Fatalf("replay events: %v", err)
	}
	skippedByParser := map[string]struct {
		Tool            string `json:"tool"`
		Capability      string `json:"capability"`
		ExecutionStatus string `json:"execution_status"`
		ExecutionPolicy string `json:"execution_policy"`
		PolicyReason    string `json:"policy_reason"`
		Transcript      string `json:"transcript"`
	}{}
	for _, event := range events {
		if event.Type != eventlog.EventTool {
			continue
		}
		var payload struct {
			Tool            string `json:"tool"`
			Capability      string `json:"capability"`
			WorkerStatus    string `json:"worker_status"`
			ExecutionStatus string `json:"execution_status"`
			ExecutionPolicy string `json:"execution_policy"`
			PolicyReason    string `json:"policy_reason"`
			Parser          string `json:"parser"`
			Transcript      string `json:"transcript"`
		}
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			t.Fatalf("decode tool payload: %v", err)
		}
		if payload.Tool == "web-api-pack" && payload.Capability == "web.discovery" {
			if payload.WorkerStatus != "" {
				t.Fatalf("disabled pack must not emit worker lifecycle events, got %#v", payload)
			}
			skippedByParser[payload.Parser] = struct {
				Tool            string `json:"tool"`
				Capability      string `json:"capability"`
				ExecutionStatus string `json:"execution_status"`
				ExecutionPolicy string `json:"execution_policy"`
				PolicyReason    string `json:"policy_reason"`
				Transcript      string `json:"transcript"`
			}{
				Tool:            payload.Tool,
				Capability:      payload.Capability,
				ExecutionStatus: payload.ExecutionStatus,
				ExecutionPolicy: payload.ExecutionPolicy,
				PolicyReason:    payload.PolicyReason,
				Transcript:      payload.Transcript,
			}
		}
	}
	for _, parser := range []string{"httpx_jsonl", "subfinder_jsonl"} {
		skipped, ok := skippedByParser[parser]
		if !ok || skipped.ExecutionStatus != "skipped" || skipped.ExecutionPolicy != "disabled" {
			t.Fatalf("expected skipped policy event for disabled web parser %s, got %#v", parser, skippedByParser)
		}
		if !strings.Contains(skipped.Transcript, "noisy active scan deferred") || skipped.PolicyReason != "noisy active scan deferred" {
			t.Fatalf("expected policy reason in skipped event for %s, got %#v", parser, skipped)
		}
	}
}

func TestRunWorkspaceRecordsSchedulerAdmissionDecisions(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	scope := `version: 1
scope:
  target_policy: enforce
  targets: []
  exclusions: []
execution:
  isolation: process
  pack_policies:
    - pack: web-api-pack
      mode: disabled
      reason: noisy active scan deferred
    - pack: infra-pack
      mode: enabled
      reason: authorized infra discovery
`
	if err := os.WriteFile(filepath.Join(root, "scope.yaml"), []byte(scope), 0o644); err != nil {
		t.Fatalf("write scope: %v", err)
	}

	result, err := RunWorkspaceWithOptions(ctx, root, RunOptions{ToolRunner: &recordingToolExecutionRunner{}})
	if err != nil {
		t.Fatalf("run workspace with scheduler admission policies: %v", err)
	}
	store, err := eventlog.OpenSQLite(filepath.Join(root, "replay", "events.db"))
	if err != nil {
		t.Fatalf("open event log: %v", err)
	}
	defer store.Close()
	events, err := store.Replay(ctx, result.RunID)
	if err != nil {
		t.Fatalf("replay events: %v", err)
	}

	type admissionPayload struct {
		Kind              string `json:"kind"`
		Tool              string `json:"tool"`
		AdmissionDecision string `json:"admission_decision"`
		ExecutionPolicy   string `json:"execution_policy"`
		PolicyReason      string `json:"policy_reason"`
		Action            string `json:"admission_action"`
		Capability        string `json:"admission_capability"`
		Isolation         string `json:"isolation"`
		Parser            string `json:"parser"`
	}
	admissions := map[string]admissionPayload{}
	admissionIndex := map[string]int{}
	webSkippedIndex := map[string]int{}
	infraQueuedIndex := map[string]int{}
	for index, event := range events {
		if event.Type != eventlog.EventTool {
			continue
		}
		var payload struct {
			Kind                string `json:"kind"`
			Tool                string `json:"tool"`
			AdmissionDecision   string `json:"admission_decision"`
			ExecutionPolicy     string `json:"execution_policy"`
			PolicyReason        string `json:"policy_reason"`
			AdmissionAction     string `json:"admission_action"`
			AdmissionCapability string `json:"admission_capability"`
			Capability          string `json:"capability"`
			Isolation           string `json:"isolation"`
			WorkerStatus        string `json:"worker_status"`
			WorkerCapability    string `json:"worker_capability"`
			ExecutionStatus     string `json:"execution_status"`
			Parser              string `json:"parser"`
		}
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			t.Fatalf("decode tool payload: %v", err)
		}
		key := expectedBuiltInDiscoverPlanKey(payload.Tool, payload.Parser)
		if payload.Kind == "scheduler_admission" {
			admissions[key] = admissionPayload{
				Kind:              payload.Kind,
				Tool:              payload.Tool,
				AdmissionDecision: payload.AdmissionDecision,
				ExecutionPolicy:   payload.ExecutionPolicy,
				PolicyReason:      payload.PolicyReason,
				Action:            payload.AdmissionAction,
				Capability:        payload.AdmissionCapability,
				Isolation:         payload.Isolation,
				Parser:            payload.Parser,
			}
			admissionIndex[key] = index
		}
		if payload.Tool == "web-api-pack" && payload.Capability == "web.discovery" && payload.ExecutionStatus == "skipped" {
			webSkippedIndex[payload.Parser] = index
		}
		if payload.Tool == "infra-pack" && payload.WorkerCapability == "infra.host_discovery" && payload.WorkerStatus == "queued" {
			infraQueuedIndex[payload.Parser] = index
		}
	}

	if len(admissions) != len(expectedBuiltInDiscoverPlanNodes()) {
		t.Fatalf("expected one scheduler admission event per compiled plan node, got %#v", admissions)
	}
	for _, want := range expectedBuiltInDiscoverPlanNodes() {
		key := expectedBuiltInDiscoverPlanKey(want.ManifestName, want.ParserID)
		admission, ok := admissions[key]
		if !ok {
			t.Fatalf("missing scheduler admission for %s, got %#v", key, admissions)
		}
		if admission.Action != string(compiler.ActionDiscoverAssets) || admission.Capability != want.Capability || admission.Parser != want.ParserID || admission.Isolation != "process" {
			t.Fatalf("unexpected admission metadata for %s: %#v", key, admission)
		}
		switch want.ManifestName {
		case "web-api-pack":
			if admission.AdmissionDecision != "skipped" || admission.ExecutionPolicy != "disabled" || admission.PolicyReason != "noisy active scan deferred" {
				t.Fatalf("expected disabled web pack admission decision for %s, got %#v", key, admission)
			}
			skippedIndex, ok := webSkippedIndex[want.ParserID]
			if !ok || admissionIndex[key] >= skippedIndex {
				t.Fatalf("expected web admission before skipped execution event for %s, admission=%d skipped=%d", key, admissionIndex[key], skippedIndex)
			}
		case "infra-pack":
			if admission.AdmissionDecision != "admitted" || admission.ExecutionPolicy != "enabled" || admission.PolicyReason != "authorized infra discovery" {
				t.Fatalf("expected enabled infra pack admission decision for %s, got %#v", key, admission)
			}
			queuedIndex, ok := infraQueuedIndex[want.ParserID]
			if !ok || admissionIndex[key] >= queuedIndex {
				t.Fatalf("expected infra admission before queued worker event for %s, admission=%d queued=%d", key, admissionIndex[key], queuedIndex)
			}
		case "ctf-pack":
			if admission.AdmissionDecision != "admitted" || admission.ExecutionPolicy != "default" {
				t.Fatalf("expected default ctf admission decision for %s, got %#v", key, admission)
			}
		}
	}
}

func TestRunWorkspaceRecordsWorkerPoolPlacement(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	scope := `version: 1
scope:
  target_policy: enforce
  targets: []
  exclusions: []
execution:
  isolation: process
  worker_pools:
    - id: web-local
      mode: local
      packs:
        - web-api-pack
    - id: infra-local
      mode: local
      packs:
        - infra-pack
    - id: ctf-local
      mode: local
      packs:
        - ctf-pack
`
	if err := os.WriteFile(filepath.Join(root, "scope.yaml"), []byte(scope), 0o644); err != nil {
		t.Fatalf("write scope: %v", err)
	}
	runner := &recordingToolExecutionRunner{}

	result, err := RunWorkspaceWithOptions(ctx, root, RunOptions{ToolRunner: runner})
	if err != nil {
		t.Fatalf("run workspace with worker placement: %v", err)
	}
	store, err := eventlog.OpenSQLite(filepath.Join(root, "replay", "events.db"))
	if err != nil {
		t.Fatalf("open event log: %v", err)
	}
	defer store.Close()
	events, err := store.Replay(ctx, result.RunID)
	if err != nil {
		t.Fatalf("replay events: %v", err)
	}

	admissions := map[string]string{}
	for _, event := range events {
		if event.Type != eventlog.EventTool {
			continue
		}
		var payload struct {
			Kind       string `json:"kind"`
			Capability string `json:"admission_capability"`
			WorkerID   string `json:"worker_id"`
		}
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			t.Fatalf("decode tool payload: %v", err)
		}
		if payload.Kind == "scheduler_admission" {
			admissions[payload.Capability] = payload.WorkerID
		}
	}
	expected := map[string]string{
		"web.discovery":                "web-local",
		"infra.host_discovery":         "infra-local",
		"ctf.challenge_classification": "ctf-local",
	}
	for capability, workerID := range expected {
		if admissions[capability] != workerID {
			t.Fatalf("expected admission %s to be placed on %s, got %#v", capability, workerID, admissions)
		}
	}
	for _, request := range runner.requests {
		if request.WorkerID == "" {
			t.Fatalf("expected runner request to carry worker placement, got %#v", request)
		}
	}
}

func TestRunWorkspaceWorkerPoolMaxConcurrencyLimitsSharedWorker(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	scope := `version: 1
scope:
  target_policy: enforce
  targets: []
  exclusions: []
execution:
  isolation: process
  tool_concurrency: 3
  worker_pools:
    - id: shared-local
      mode: local
      max_concurrency: 1
      packs:
        - web-api-pack
        - infra-pack
`
	if err := os.WriteFile(filepath.Join(root, "scope.yaml"), []byte(scope), 0o644); err != nil {
		t.Fatalf("write scope: %v", err)
	}
	runner := &workerPoolConcurrencyProbeRunner{
		activeByWorker:    map[string]int{},
		maxActiveByWorker: map[string]int{},
		delay:             75 * time.Millisecond,
	}

	result, err := RunWorkspaceWithOptions(ctx, root, RunOptions{ToolRunner: runner})
	if err != nil {
		t.Fatalf("run workspace with worker pool limit: %v", err)
	}
	if runner.MaxActiveForWorker("shared-local") > 1 {
		t.Fatalf("expected shared worker pool concurrency to be capped at 1, got %d", runner.MaxActiveForWorker("shared-local"))
	}
	if runner.MaxActive() < 2 {
		t.Fatalf("expected an unpooled local worker to run alongside the capped shared pool, max active=%d", runner.MaxActive())
	}

	store, err := eventlog.OpenSQLite(filepath.Join(root, "replay", "events.db"))
	if err != nil {
		t.Fatalf("open event log: %v", err)
	}
	defer store.Close()
	events, err := store.Replay(ctx, result.RunID)
	if err != nil {
		t.Fatalf("replay events: %v", err)
	}
	sawAdmissionLimit := false
	sawWorkerPoolWait := false
	for _, event := range events {
		if event.Type != eventlog.EventTool {
			continue
		}
		var payload struct {
			Kind                     string `json:"kind"`
			WorkerID                 string `json:"worker_id"`
			WorkerStatus             string `json:"worker_status"`
			WaitReason               string `json:"wait_reason"`
			WorkerPoolMaxConcurrency int    `json:"worker_pool_max_concurrency"`
		}
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			t.Fatalf("decode tool payload: %v", err)
		}
		if payload.Kind == "scheduler_admission" && payload.WorkerID == "shared-local" && payload.WorkerPoolMaxConcurrency == 1 {
			sawAdmissionLimit = true
		}
		if payload.WorkerID == "shared-local" && payload.WorkerStatus == "waiting" && payload.WaitReason == "worker_pool_max_concurrency" {
			sawWorkerPoolWait = true
		}
	}
	if !sawAdmissionLimit {
		t.Fatalf("expected scheduler admission to record shared worker pool max concurrency")
	}
	if !sawWorkerPoolWait {
		t.Fatalf("expected blocked shared worker to persist worker_pool_max_concurrency wait reason")
	}
}

func TestRunWorkspaceWritesWorkerLeaseArtifacts(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	scope := `version: 1
scope:
  target_policy: enforce
  targets: []
  exclusions: []
execution:
  isolation: process
  tool_concurrency: 3
  worker_pools:
    - id: shared-local
      mode: local
      max_concurrency: 2
      packs:
        - web-api-pack
        - infra-pack
`
	if err := os.WriteFile(filepath.Join(root, "scope.yaml"), []byte(scope), 0o644); err != nil {
		t.Fatalf("write scope: %v", err)
	}
	runner := &recordingToolExecutionRunner{}

	result, err := RunWorkspaceWithOptions(ctx, root, RunOptions{ToolRunner: runner})
	if err != nil {
		t.Fatalf("run workspace with worker leases: %v", err)
	}
	leases, err := ListWorkerLeases(root)
	if err != nil {
		t.Fatalf("list worker leases: %v", err)
	}
	if len(leases) != len(runner.requests) {
		t.Fatalf("expected one worker lease per started request, leases=%d requests=%d leases=%#v", len(leases), len(runner.requests), leases)
	}
	for _, lease := range leases {
		if lease.RunID != result.RunID {
			t.Fatalf("expected lease run id %s, got %#v", result.RunID, lease)
		}
		if lease.ExecutionID == "" || lease.Tool == "" || lease.Capability == "" || lease.WorkerID == "" {
			t.Fatalf("expected lease identity fields, got %#v", lease)
		}
		if lease.Status != "completed" {
			t.Fatalf("expected completed worker lease, got %#v", lease)
		}
		if lease.AcquiredAt == "" || lease.ReleasedAt == "" {
			t.Fatalf("expected lease timestamps, got %#v", lease)
		}
		if !strings.Contains(lease.Path, filepath.Join("runtime", "worker-leases", result.RunID)) {
			t.Fatalf("expected lease path inside runtime worker lease directory, got %#v", lease)
		}
	}
}

func TestRunWorkspaceRequireSafetyAuthorizationRechecksBeforeWorkerLeaseAcquire(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	writeRuntimeWorkerDisabledSafetyScope(t, root)
	writeRuntimeWorkerCurrentSafetyAuthorization(t, root)
	runner := &recordingToolExecutionRunner{}
	authorizationRemoved := false

	_, err := RunWorkspaceWithOptions(ctx, root, RunOptions{
		RequireSafetyAuthorization: true,
		ToolRunner:                 runner,
		OnEvent: func(event eventlog.Event) error {
			if authorizationRemoved || event.Type != eventlog.EventVerification {
				return nil
			}
			payload := decodeWorkspaceSafetyPayloadForTest(t, event)
			if payload.Kind != "workspace_safety" {
				return nil
			}
			authorizationRemoved = true
			return os.WriteFile(filepath.Join(root, "authorization.yaml"), []byte("version: 1\nsafety_bypasses: []\n"), 0o644)
		},
	})
	if err == nil || !strings.Contains(err.Error(), "worker runtime side effect requires current safety bypass authorization") {
		t.Fatalf("expected strict safety authorization recheck before lease acquire, got %v", err)
	}
	if len(runner.requests) != 0 {
		t.Fatalf("lease acquire safety failure must block before runner execution, got %#v", runner.requests)
	}
	leases, listErr := ListWorkerLeases(root)
	if listErr != nil {
		t.Fatalf("list worker leases after rejected acquire: %v", listErr)
	}
	if len(leases) != 0 {
		t.Fatalf("rejected acquire must not write worker leases, got %#v", leases)
	}
}

func TestRunWorkspaceRequireSafetyAuthorizationRechecksBeforeWorkerLeaseComplete(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	writeRuntimeWorkerDisabledSafetyScope(t, root)
	writeRuntimeWorkerCurrentSafetyAuthorization(t, root)
	runner := &authorizationRemovingToolExecutionRunner{root: root}

	_, err := RunWorkspaceWithOptions(ctx, root, RunOptions{
		RequireSafetyAuthorization: true,
		ToolRunner:                 runner,
	})
	if err == nil || !strings.Contains(err.Error(), "worker runtime side effect requires current safety bypass authorization") {
		t.Fatalf("expected strict safety authorization recheck before lease complete, got %v", err)
	}
	if len(runner.requests) == 0 {
		t.Fatalf("expected runner to execute before lease completion recheck")
	}
	leases, listErr := ListWorkerLeases(root)
	if listErr != nil {
		t.Fatalf("list worker leases after rejected complete: %v", listErr)
	}
	if len(leases) == 0 {
		t.Fatalf("expected active worker lease from successful acquire")
	}
	for _, lease := range leases {
		if lease.Status != "active" || lease.ReleasedAt != "" {
			t.Fatalf("rejected complete must leave lease active, got %#v", lease)
		}
	}
}

func TestRunWorkspaceDefaultSafetyAuditKeepsWorkerLeaseWritesWarningOnly(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	writeRuntimeWorkerDisabledSafetyScope(t, root)
	runner := &recordingToolExecutionRunner{}

	result, err := RunWorkspaceWithOptions(ctx, root, RunOptions{
		ToolRunner:            runner,
		DeferReportProjection: true,
	})
	if err != nil {
		t.Fatalf("default run should remain warning-only for disabled safety boundary: %v", err)
	}
	if len(runner.requests) == 0 {
		t.Fatalf("expected default warning-only run to execute tool requests")
	}
	leases, listErr := ListWorkerLeases(root)
	if listErr != nil {
		t.Fatalf("list worker leases: %v", listErr)
	}
	if len(leases) != len(runner.requests) {
		t.Fatalf("expected default run lease writes, leases=%d requests=%d leases=%#v", len(leases), len(runner.requests), leases)
	}
	for _, lease := range leases {
		if lease.RunID != result.RunID || lease.Status != "completed" || lease.ReleasedAt == "" {
			t.Fatalf("expected completed warning-only lease for run %s, got %#v", result.RunID, lease)
		}
	}
}

func TestRunWorkspaceRequireSafetyAuthorizationAllowsWorkerLeaseWritesWithCurrentAuthorization(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	writeRuntimeWorkerDisabledSafetyScope(t, root)
	writeRuntimeWorkerCurrentSafetyAuthorization(t, root)
	runner := &recordingToolExecutionRunner{}

	result, err := RunWorkspaceWithOptions(ctx, root, RunOptions{
		RequireSafetyAuthorization: true,
		ToolRunner:                 runner,
		DeferReportProjection:      true,
	})
	if err != nil {
		t.Fatalf("strict run with current authorization should write worker leases: %v", err)
	}
	if len(runner.requests) == 0 {
		t.Fatalf("expected strict authorized run to execute tool requests")
	}
	leases, listErr := ListWorkerLeases(root)
	if listErr != nil {
		t.Fatalf("list worker leases: %v", listErr)
	}
	if len(leases) != len(runner.requests) {
		t.Fatalf("expected one worker lease per strict authorized request, leases=%d requests=%d leases=%#v", len(leases), len(runner.requests), leases)
	}
	for _, lease := range leases {
		if lease.RunID != result.RunID || lease.Status != "completed" || lease.ReleasedAt == "" {
			t.Fatalf("expected completed strict authorized lease for run %s, got %#v", result.RunID, lease)
		}
	}
}

func TestRunWorkspaceMarksWorkerLeaseFailedOnRunnerError(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	runner := &failingWorkerLeaseProbeRunner{}

	_, err := RunWorkspaceWithOptions(ctx, root, RunOptions{ToolRunner: runner})
	if err == nil {
		t.Fatalf("expected run to fail")
	}
	leases, listErr := ListWorkerLeases(root)
	if listErr != nil {
		t.Fatalf("list worker leases: %v", listErr)
	}
	if len(leases) == 0 {
		t.Fatalf("expected a failed worker lease artifact")
	}
	failed := leases[0]
	if failed.Status != "failed" || !strings.Contains(failed.Error, "lease probe failed") {
		t.Fatalf("expected failed worker lease with runner error, got %#v", failed)
	}
	if failed.ReleasedAt == "" {
		t.Fatalf("expected failed lease release timestamp, got %#v", failed)
	}
}

func TestAcquireWorkerLeaseRejectsActiveCollision(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	request := workerLeaseTestRequest()

	if err := acquireWorkerLease(root, "run-collision", request, false); err != nil {
		t.Fatalf("acquire worker lease: %v", err)
	}
	err := acquireWorkerLease(root, "run-collision", request, false)
	if err == nil || !strings.Contains(err.Error(), "active worker lease already exists") {
		t.Fatalf("expected active lease collision error, got %v", err)
	}
	leases, listErr := ListWorkerLeases(root)
	if listErr != nil {
		t.Fatalf("list worker leases: %v", listErr)
	}
	if len(leases) != 1 {
		t.Fatalf("expected one lease after rejected collision, got %#v", leases)
	}
	if leases[0].Status != "active" || leases[0].ReleasedAt != "" {
		t.Fatalf("expected original active lease to remain unchanged, got %#v", leases[0])
	}
}

func TestListWorkerLeasesWithStalenessMarksOldActiveLease(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)
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

	leases, err := ListWorkerLeasesWithStaleness(root, now, staleAfter)
	if err != nil {
		t.Fatalf("list worker leases with staleness: %v", err)
	}
	if len(leases) != 1 || !leases[0].Stale {
		t.Fatalf("expected active lease to be marked stale, got %#v", leases)
	}
}

func TestUpdateWorkerLeaseHeartbeatRequiresExplicitHeartbeat(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Date(2026, 5, 30, 12, 10, 0, 0, time.UTC)
	oldHeartbeat := now.Add(-10 * time.Minute).Format(time.RFC3339Nano)
	if err := writeWorkerLeaseArtifact(root, WorkerLeaseArtifact{
		Version:     1,
		RunID:       "run-heartbeat-required",
		ExecutionID: "tool-heartbeat",
		Tool:        "web-api-pack",
		Capability:  "web.discovery",
		WorkerID:    "remote-web-1",
		Status:      "active",
		AcquiredAt:  oldHeartbeat,
		HeartbeatAt: oldHeartbeat,
	}); err != nil {
		t.Fatalf("write worker lease fixture: %v", err)
	}

	if _, err := UpdateWorkerLeaseHeartbeat(root, "run-heartbeat-required", "tool-heartbeat", WorkerLeaseHeartbeatOptions{
		Now: now,
	}); err == nil || !strings.Contains(err.Error(), "worker lease heartbeat requires explicit heartbeat") {
		t.Fatalf("expected explicit heartbeat error, got %v", err)
	}
	lease, err := readWorkerLeaseByIdentity(root, "run-heartbeat-required", "tool-heartbeat")
	if err != nil {
		t.Fatalf("read worker lease: %v", err)
	}
	if lease.HeartbeatAt != oldHeartbeat || lease.HeartbeatBy != "" {
		t.Fatalf("missing heartbeat flag must not mutate lease, got %#v", lease)
	}
}

func TestUpdateWorkerLeaseHeartbeatRefreshesActiveLeaseAndPreventsReclaim(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Date(2026, 5, 30, 12, 20, 0, 0, time.UTC)
	staleAfter := 5 * time.Minute
	oldHeartbeat := now.Add(-staleAfter - time.Minute).Format(time.RFC3339Nano)
	if err := writeWorkerLeaseArtifact(root, WorkerLeaseArtifact{
		Version:     1,
		RunID:       "run-heartbeat",
		ExecutionID: "tool-heartbeat",
		Tool:        "web-api-pack",
		Capability:  "web.discovery",
		WorkerID:    "remote-web-1",
		Status:      "active",
		AcquiredAt:  oldHeartbeat,
		HeartbeatAt: oldHeartbeat,
	}); err != nil {
		t.Fatalf("write worker lease fixture: %v", err)
	}

	lease, err := UpdateWorkerLeaseHeartbeat(root, "run-heartbeat", "tool-heartbeat", WorkerLeaseHeartbeatOptions{
		Heartbeat:   true,
		Now:         now,
		HeartbeatBy: "remote-web-1",
		Reason:      "still-running",
	})
	if err != nil {
		t.Fatalf("update worker lease heartbeat: %v", err)
	}
	if lease.Status != "active" || lease.HeartbeatAt != now.Format(time.RFC3339Nano) || lease.HeartbeatBy != "remote-web-1" || lease.HeartbeatReason != "still-running" {
		t.Fatalf("unexpected refreshed worker lease: %#v", lease)
	}
	if lease.ReleasedAt != "" {
		t.Fatalf("heartbeat must not release active lease, got %#v", lease)
	}
	plan, err := PlanWorkerLeaseReclaims(root, now.Add(time.Minute), staleAfter)
	if err != nil {
		t.Fatalf("plan worker lease reclaims: %v", err)
	}
	if len(plan.Ready) != 0 || len(plan.Skipped) != 1 || plan.Skipped[0].Reason != "active-not-stale" {
		t.Fatalf("refreshed lease should not be ready for reclaim, got %#v", plan)
	}
}

func TestUpdateWorkerLeaseHeartbeatRequiresCurrentSafetyAuthorizationForDisabledBoundaries(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Date(2026, 5, 30, 12, 22, 0, 0, time.UTC)
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	oldHeartbeat := now.Add(-10 * time.Minute).Format(time.RFC3339Nano)
	if err := writeWorkerLeaseArtifact(root, WorkerLeaseArtifact{
		Version:     1,
		RunID:       "run-heartbeat-safety",
		ExecutionID: "tool-heartbeat-safety",
		Tool:        "web-api-pack",
		Capability:  "web.discovery",
		WorkerID:    "remote-web-1",
		Status:      "active",
		AcquiredAt:  oldHeartbeat,
		HeartbeatAt: oldHeartbeat,
	}); err != nil {
		t.Fatalf("write worker lease fixture: %v", err)
	}
	writeRuntimeWorkerDisabledSafetyScope(t, root)

	_, err := UpdateWorkerLeaseHeartbeat(root, "run-heartbeat-safety", "tool-heartbeat-safety", WorkerLeaseHeartbeatOptions{
		Heartbeat:   true,
		Now:         now,
		HeartbeatBy: "remote-web-1",
		Reason:      "still-running",
	})
	if err == nil || !strings.Contains(err.Error(), "worker runtime side effect requires current safety bypass authorization") {
		t.Fatalf("expected strict safety authorization error, got %v", err)
	}
	lease, err := readWorkerLeaseByIdentity(root, "run-heartbeat-safety", "tool-heartbeat-safety")
	if err != nil {
		t.Fatalf("read worker lease after rejected heartbeat: %v", err)
	}
	if lease.HeartbeatAt != oldHeartbeat || lease.HeartbeatBy != "" || lease.HeartbeatReason != "" {
		t.Fatalf("rejected heartbeat must not mutate lease, got %#v", lease)
	}
}

func TestUpdateWorkerLeaseHeartbeatRejectsNonActiveLease(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Date(2026, 5, 30, 12, 25, 0, 0, time.UTC)
	if err := writeWorkerLeaseArtifact(root, WorkerLeaseArtifact{
		Version:     1,
		RunID:       "run-completed",
		ExecutionID: "tool-completed",
		Tool:        "web-api-pack",
		Capability:  "web.discovery",
		WorkerID:    "remote-web-1",
		Status:      "completed",
		AcquiredAt:  now.Add(-time.Minute).Format(time.RFC3339Nano),
		HeartbeatAt: now.Add(-time.Minute).Format(time.RFC3339Nano),
		ReleasedAt:  now.Add(-time.Minute).Format(time.RFC3339Nano),
	}); err != nil {
		t.Fatalf("write worker lease fixture: %v", err)
	}

	if _, err := UpdateWorkerLeaseHeartbeat(root, "run-completed", "tool-completed", WorkerLeaseHeartbeatOptions{
		Heartbeat:   true,
		Now:         now,
		HeartbeatBy: "remote-web-1",
	}); err == nil || !strings.Contains(err.Error(), "worker lease heartbeat requires active lease") {
		t.Fatalf("expected non-active heartbeat rejection, got %v", err)
	}
}

func TestReleaseWorkerLeaseRequiresExplicitRelease(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Date(2026, 5, 30, 12, 26, 0, 0, time.UTC)
	acquiredAt := now.Add(-time.Minute).Format(time.RFC3339Nano)
	if err := writeWorkerLeaseArtifact(root, WorkerLeaseArtifact{
		Version:     1,
		RunID:       "run-release-required",
		ExecutionID: "tool-release",
		Tool:        "web-api-pack",
		Capability:  "web.discovery",
		WorkerID:    "remote-web-1",
		Status:      "active",
		AcquiredAt:  acquiredAt,
		HeartbeatAt: acquiredAt,
	}); err != nil {
		t.Fatalf("write worker lease fixture: %v", err)
	}

	if _, err := ReleaseWorkerLease(root, "run-release-required", "tool-release", WorkerLeaseReleaseOptions{
		Status: "completed",
		Now:    now,
	}); err == nil || !strings.Contains(err.Error(), "worker lease release requires explicit release") {
		t.Fatalf("expected explicit release error, got %v", err)
	}
	lease, err := readWorkerLeaseByIdentity(root, "run-release-required", "tool-release")
	if err != nil {
		t.Fatalf("read worker lease: %v", err)
	}
	if lease.Status != "active" || lease.ReleasedAt != "" || lease.ReleasedBy != "" {
		t.Fatalf("missing release flag must not mutate lease, got %#v", lease)
	}
}

func TestReleaseWorkerLeaseCompletesActiveLeaseAndPreventsReclaim(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Date(2026, 5, 30, 12, 27, 0, 0, time.UTC)
	staleAfter := 5 * time.Minute
	oldHeartbeat := now.Add(-staleAfter - time.Minute).Format(time.RFC3339Nano)
	if err := writeWorkerLeaseArtifact(root, WorkerLeaseArtifact{
		Version:     1,
		RunID:       "run-release",
		ExecutionID: "tool-release",
		Tool:        "web-api-pack",
		Capability:  "web.discovery",
		WorkerID:    "remote-web-1",
		Status:      "active",
		AcquiredAt:  oldHeartbeat,
		HeartbeatAt: oldHeartbeat,
	}); err != nil {
		t.Fatalf("write worker lease fixture: %v", err)
	}

	lease, err := ReleaseWorkerLease(root, "run-release", "tool-release", WorkerLeaseReleaseOptions{
		Release:           true,
		Status:            "completed",
		Now:               now,
		ReleasedBy:        "remote-web-1",
		Reason:            "finished",
		ResultExecutionID: "result-web-1",
		AttemptDir:        "/workspace/attempts/run-release/tool-release",
	})
	if err != nil {
		t.Fatalf("release worker lease: %v", err)
	}
	if lease.Status != "completed" || lease.ReleasedAt != now.Format(time.RFC3339Nano) || lease.ReleasedBy != "remote-web-1" || lease.ReleaseReason != "finished" || lease.ResultExecutionID != "result-web-1" {
		t.Fatalf("unexpected released worker lease: %#v", lease)
	}
	if lease.HeartbeatAt != now.Format(time.RFC3339Nano) || lease.AttemptDir != "/workspace/attempts/run-release/tool-release" {
		t.Fatalf("release should refresh heartbeat and preserve result metadata, got %#v", lease)
	}
	plan, err := PlanWorkerLeaseReclaims(root, now.Add(10*time.Minute), staleAfter)
	if err != nil {
		t.Fatalf("plan worker lease reclaims: %v", err)
	}
	if len(plan.Ready) != 0 || len(plan.Skipped) != 1 || plan.Skipped[0].Reason != "not-active" {
		t.Fatalf("released lease should not be ready for reclaim, got %#v", plan)
	}
}

func TestReleaseWorkerLeaseRequiresCurrentSafetyAuthorizationForDisabledBoundaries(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Date(2026, 5, 30, 12, 27, 30, 0, time.UTC)
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	oldHeartbeat := now.Add(-10 * time.Minute).Format(time.RFC3339Nano)
	if err := writeWorkerLeaseArtifact(root, WorkerLeaseArtifact{
		Version:     1,
		RunID:       "run-release-safety",
		ExecutionID: "tool-release-safety",
		Tool:        "web-api-pack",
		Capability:  "web.discovery",
		WorkerID:    "remote-web-1",
		Status:      "active",
		AcquiredAt:  oldHeartbeat,
		HeartbeatAt: oldHeartbeat,
	}); err != nil {
		t.Fatalf("write worker lease fixture: %v", err)
	}
	writeRuntimeWorkerDisabledSafetyScope(t, root)

	_, err := ReleaseWorkerLease(root, "run-release-safety", "tool-release-safety", WorkerLeaseReleaseOptions{
		Release:    true,
		Status:     "completed",
		Now:        now,
		ReleasedBy: "remote-web-1",
		Reason:     "finished",
	})
	if err == nil || !strings.Contains(err.Error(), "worker runtime side effect requires current safety bypass authorization") {
		t.Fatalf("expected strict safety authorization error, got %v", err)
	}
	lease, err := readWorkerLeaseByIdentity(root, "run-release-safety", "tool-release-safety")
	if err != nil {
		t.Fatalf("read worker lease after rejected release: %v", err)
	}
	if lease.Status != "active" || lease.ReleasedAt != "" || lease.ReleasedBy != "" || lease.ReleaseReason != "" {
		t.Fatalf("rejected release must not mutate lease, got %#v", lease)
	}
}

func TestReleaseWorkerLeaseRejectsNonActiveLeaseAndUnsupportedStatus(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Date(2026, 5, 30, 12, 28, 0, 0, time.UTC)
	if err := writeWorkerLeaseArtifact(root, WorkerLeaseArtifact{
		Version:     1,
		RunID:       "run-release-completed",
		ExecutionID: "tool-release-completed",
		Tool:        "web-api-pack",
		Capability:  "web.discovery",
		WorkerID:    "remote-web-1",
		Status:      "completed",
		AcquiredAt:  now.Add(-time.Minute).Format(time.RFC3339Nano),
		HeartbeatAt: now.Add(-time.Minute).Format(time.RFC3339Nano),
		ReleasedAt:  now.Add(-time.Minute).Format(time.RFC3339Nano),
	}); err != nil {
		t.Fatalf("write completed worker lease fixture: %v", err)
	}
	if _, err := ReleaseWorkerLease(root, "run-release-completed", "tool-release-completed", WorkerLeaseReleaseOptions{
		Release: true,
		Status:  "completed",
		Now:     now,
	}); err == nil || !strings.Contains(err.Error(), "worker lease release requires active lease") {
		t.Fatalf("expected non-active release rejection, got %v", err)
	}

	if err := writeWorkerLeaseArtifact(root, WorkerLeaseArtifact{
		Version:     1,
		RunID:       "run-release-invalid",
		ExecutionID: "tool-release-invalid",
		Tool:        "web-api-pack",
		Capability:  "web.discovery",
		WorkerID:    "remote-web-1",
		Status:      "active",
		AcquiredAt:  now.Add(-time.Minute).Format(time.RFC3339Nano),
		HeartbeatAt: now.Add(-time.Minute).Format(time.RFC3339Nano),
	}); err != nil {
		t.Fatalf("write active worker lease fixture: %v", err)
	}
	if _, err := ReleaseWorkerLease(root, "run-release-invalid", "tool-release-invalid", WorkerLeaseReleaseOptions{
		Release: true,
		Status:  "reclaimed",
		Now:     now,
	}); err == nil || !strings.Contains(err.Error(), "worker lease release status must be completed or failed") {
		t.Fatalf("expected invalid release status rejection, got %v", err)
	}
}

func TestPlanWorkerLeaseReclaimsRequiresStaleActiveLeases(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Date(2026, 5, 30, 12, 30, 0, 0, time.UTC)
	staleAfter := 5 * time.Minute
	oldHeartbeat := now.Add(-staleAfter - time.Minute).Format(time.RFC3339Nano)
	freshHeartbeat := now.Add(-time.Minute).Format(time.RFC3339Nano)
	fixtures := []WorkerLeaseArtifact{
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
			Capability:  "ctf.challenge_classification",
			WorkerID:    "local",
			Status:      "completed",
			AcquiredAt:  oldHeartbeat,
			HeartbeatAt: oldHeartbeat,
			ReleasedAt:  oldHeartbeat,
		},
	}
	for _, fixture := range fixtures {
		if err := writeWorkerLeaseArtifact(root, fixture); err != nil {
			t.Fatalf("write worker lease fixture: %v", err)
		}
	}

	plan, err := PlanWorkerLeaseReclaims(root, now, staleAfter)
	if err != nil {
		t.Fatalf("plan worker lease reclaims: %v", err)
	}
	if len(plan.Ready) != 1 || plan.Ready[0].Lease.ExecutionID != "tool-stale" || plan.Ready[0].Mode != "metadata-only" {
		t.Fatalf("expected only stale active lease ready for metadata-only reclaim, got %#v", plan)
	}
	if len(plan.Skipped) != 2 {
		t.Fatalf("expected fresh active and completed leases to be skipped, got %#v", plan)
	}
	reasons := map[string]string{}
	for _, skipped := range plan.Skipped {
		reasons[skipped.Lease.ExecutionID] = skipped.Reason
	}
	if reasons["tool-fresh"] != "active-not-stale" || reasons["tool-done"] != "not-active" {
		t.Fatalf("unexpected skip reasons: %#v", reasons)
	}
}

func TestCreateWorkerLeaseReclaimDispatchWritesMetadataOnlyArtifact(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Date(2026, 5, 30, 13, 0, 0, 0, time.UTC)
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
	} {
		if err := writeWorkerLeaseArtifact(root, fixture); err != nil {
			t.Fatalf("write worker lease fixture: %v", err)
		}
	}

	dispatch, err := CreateWorkerLeaseReclaimDispatch(root, now, staleAfter)
	if err != nil {
		t.Fatalf("create worker lease reclaim dispatch: %v", err)
	}
	if dispatch.Status != "dry-run" || dispatch.Mode != "metadata-only" {
		t.Fatalf("expected dry-run metadata-only dispatch, got %#v", dispatch)
	}
	if len(dispatch.Ready) != 1 || dispatch.Ready[0].Lease.ExecutionID != "tool-stale" {
		t.Fatalf("expected only stale lease ready, got %#v", dispatch)
	}
	if len(dispatch.Skipped) != 1 || dispatch.Skipped[0].Reason != "active-not-stale" {
		t.Fatalf("expected fresh lease skipped, got %#v", dispatch)
	}
	if !strings.Contains(dispatch.Path, filepath.Join("runtime", "worker-lease-reclaims")) {
		t.Fatalf("expected dispatch under runtime worker-lease-reclaims, got %s", dispatch.Path)
	}
	data, err := os.ReadFile(dispatch.Path)
	if err != nil {
		t.Fatalf("read reclaim dispatch artifact: %v", err)
	}
	var decoded WorkerLeaseReclaimDispatch
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("decode reclaim dispatch artifact: %v", err)
	}
	if decoded.Status != "dry-run" || len(decoded.Ready) != 1 || decoded.Ready[0].Lease.ExecutionID != "tool-stale" {
		t.Fatalf("unexpected decoded reclaim dispatch: %#v", decoded)
	}
	leases, err := ListWorkerLeases(root)
	if err != nil {
		t.Fatalf("list worker leases after dispatch: %v", err)
	}
	for _, lease := range leases {
		if lease.ExecutionID == "tool-stale" && (lease.Status != "active" || lease.ReleasedAt != "") {
			t.Fatalf("metadata-only dispatch must not mutate stale lease, got %#v", lease)
		}
	}
}

func TestListAndReadWorkerLeaseReclaimDispatchesReadsArtifacts(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Date(2026, 5, 30, 13, 30, 0, 0, time.UTC)
	staleAfter := 5 * time.Minute
	oldHeartbeat := now.Add(-staleAfter - time.Minute).Format(time.RFC3339Nano)
	if err := writeWorkerLeaseArtifact(root, WorkerLeaseArtifact{
		Version:     1,
		RunID:       "run-stale-a",
		ExecutionID: "tool-stale-a",
		Tool:        "web-api-pack",
		Capability:  "web.discovery",
		WorkerID:    "local",
		Status:      "active",
		AcquiredAt:  oldHeartbeat,
		HeartbeatAt: oldHeartbeat,
	}); err != nil {
		t.Fatalf("write first worker lease fixture: %v", err)
	}
	first, err := CreateWorkerLeaseReclaimDispatch(root, now, staleAfter)
	if err != nil {
		t.Fatalf("create first reclaim dispatch: %v", err)
	}
	if err := writeWorkerLeaseArtifact(root, WorkerLeaseArtifact{
		Version:     1,
		RunID:       "run-stale-b",
		ExecutionID: "tool-stale-b",
		Tool:        "infra-pack",
		Capability:  "infra.host_discovery",
		WorkerID:    "remote-a",
		Status:      "active",
		AcquiredAt:  oldHeartbeat,
		HeartbeatAt: oldHeartbeat,
	}); err != nil {
		t.Fatalf("write second worker lease fixture: %v", err)
	}
	second, err := CreateWorkerLeaseReclaimDispatch(root, now, staleAfter)
	if err != nil {
		t.Fatalf("create second reclaim dispatch: %v", err)
	}

	dispatches, err := ListWorkerLeaseReclaimDispatches(root)
	if err != nil {
		t.Fatalf("list worker lease reclaim dispatches: %v", err)
	}
	if len(dispatches) != 2 {
		t.Fatalf("expected two reclaim dispatches, got %#v", dispatches)
	}
	byPath := map[string]WorkerLeaseReclaimDispatch{}
	for _, dispatch := range dispatches {
		byPath[dispatch.Path] = dispatch
	}
	if byPath[first.Path].Status != "dry-run" || len(byPath[second.Path].Ready) != 2 {
		t.Fatalf("unexpected listed reclaim dispatches: %#v", byPath)
	}
	read, err := ReadWorkerLeaseReclaimDispatch(root, filepath.Base(second.Path))
	if err != nil {
		t.Fatalf("read worker lease reclaim dispatch: %v", err)
	}
	if read.Path != second.Path || len(read.Ready) != 2 {
		t.Fatalf("unexpected read reclaim dispatch: %#v", read)
	}
	if _, err := ReadWorkerLeaseReclaimDispatch(root, filepath.Join(t.TempDir(), "outside.json")); err == nil {
		t.Fatalf("expected reclaim dispatch path confinement error")
	}
}

func TestExecuteWorkerLeaseReclaimDispatchRequiresExplicitExecute(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Date(2026, 5, 30, 14, 0, 0, 0, time.UTC)
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
		t.Fatalf("write worker lease fixture: %v", err)
	}
	dispatch, err := CreateWorkerLeaseReclaimDispatch(root, now, staleAfter)
	if err != nil {
		t.Fatalf("create reclaim dispatch: %v", err)
	}

	_, err = ExecuteWorkerLeaseReclaimDispatch(root, dispatch.Path, WorkerLeaseReclaimExecutionOptions{
		Now:        now,
		StaleAfter: staleAfter,
	})
	if err == nil || !strings.Contains(err.Error(), "requires explicit execute") {
		t.Fatalf("expected explicit execute error, got %v", err)
	}
	leases, err := ListWorkerLeases(root)
	if err != nil {
		t.Fatalf("list worker leases: %v", err)
	}
	if len(leases) != 1 || leases[0].Status != "active" || leases[0].ReleasedAt != "" {
		t.Fatalf("execution without execute flag must not mutate lease, got %#v", leases)
	}
	executions, err := ListWorkerLeaseReclaimExecutions(root)
	if err != nil {
		t.Fatalf("list reclaim executions: %v", err)
	}
	if len(executions) != 0 {
		t.Fatalf("execution without execute flag must not write audit artifact, got %#v", executions)
	}
}

func TestExecuteWorkerLeaseReclaimDispatchRevalidatesAndReclaimsStaleLease(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Date(2026, 5, 30, 14, 30, 0, 0, time.UTC)
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
		t.Fatalf("write worker lease fixture: %v", err)
	}
	dispatch, err := CreateWorkerLeaseReclaimDispatch(root, now, staleAfter)
	if err != nil {
		t.Fatalf("create reclaim dispatch: %v", err)
	}

	execution, err := ExecuteWorkerLeaseReclaimDispatch(root, filepath.Base(dispatch.Path), WorkerLeaseReclaimExecutionOptions{
		Execute:    true,
		Now:        now,
		StaleAfter: staleAfter,
	})
	if err != nil {
		t.Fatalf("execute reclaim dispatch: %v", err)
	}
	if execution.Status != "completed" || execution.Mode != "execute" || execution.DispatchPath != dispatch.Path {
		t.Fatalf("unexpected reclaim execution: %#v", execution)
	}
	if len(execution.Results) != 1 || execution.Results[0].Status != "reclaimed" || execution.Results[0].ExecutionID != "tool-stale" {
		t.Fatalf("expected reclaimed result, got %#v", execution.Results)
	}
	if !strings.Contains(execution.Path, filepath.Join("runtime", "worker-lease-reclaim-executions")) {
		t.Fatalf("expected execution audit path under worker-lease-reclaim-executions, got %s", execution.Path)
	}
	leases, err := ListWorkerLeases(root)
	if err != nil {
		t.Fatalf("list worker leases: %v", err)
	}
	if len(leases) != 1 || leases[0].Status != "reclaimed" || leases[0].ReleasedAt == "" {
		t.Fatalf("expected lease status reclaimed with release timestamp, got %#v", leases)
	}
	decoded, err := ReadWorkerLeaseReclaimExecution(root, filepath.Base(execution.Path))
	if err != nil {
		t.Fatalf("read reclaim execution: %v", err)
	}
	if decoded.Path != execution.Path || len(decoded.Results) != 1 || decoded.Results[0].Status != "reclaimed" {
		t.Fatalf("unexpected decoded reclaim execution: %#v", decoded)
	}
}

func TestExecuteWorkerLeaseReclaimDispatchRequiresCurrentSafetyAuthorizationBeforeLeaseMutation(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Date(2026, 5, 30, 14, 35, 0, 0, time.UTC)
	staleAfter := 5 * time.Minute
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	oldHeartbeat := now.Add(-staleAfter - time.Minute).Format(time.RFC3339Nano)
	if err := writeWorkerLeaseArtifact(root, WorkerLeaseArtifact{
		Version:     1,
		RunID:       "run-reclaim-safety",
		ExecutionID: "tool-reclaim-safety",
		Tool:        "web-api-pack",
		Capability:  "web.discovery",
		WorkerID:    "local",
		Status:      "active",
		AcquiredAt:  oldHeartbeat,
		HeartbeatAt: oldHeartbeat,
	}); err != nil {
		t.Fatalf("write worker lease fixture: %v", err)
	}
	dispatch, err := CreateWorkerLeaseReclaimDispatch(root, now, staleAfter)
	if err != nil {
		t.Fatalf("create reclaim dispatch: %v", err)
	}
	writeRuntimeWorkerDisabledSafetyScope(t, root)

	_, err = ExecuteWorkerLeaseReclaimDispatch(root, filepath.Base(dispatch.Path), WorkerLeaseReclaimExecutionOptions{
		Execute:    true,
		Now:        now,
		StaleAfter: staleAfter,
	})
	if err == nil || !strings.Contains(err.Error(), "worker runtime side effect requires current safety bypass authorization") {
		t.Fatalf("expected strict safety authorization error, got %v", err)
	}
	lease, err := readWorkerLeaseByIdentity(root, "run-reclaim-safety", "tool-reclaim-safety")
	if err != nil {
		t.Fatalf("read worker lease after rejected reclaim: %v", err)
	}
	if lease.Status != "active" || lease.ReleasedAt != "" {
		t.Fatalf("rejected reclaim must not mutate lease, got %#v", lease)
	}
	executions, err := ListWorkerLeaseReclaimExecutions(root)
	if err != nil {
		t.Fatalf("list reclaim executions: %v", err)
	}
	if len(executions) != 0 {
		t.Fatalf("rejected reclaim must not write execution audits, got %#v", executions)
	}
}

func TestWorkerLeaseMutationGatesAllowDisabledBoundariesWithCurrentSafetyAuthorization(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Date(2026, 5, 30, 14, 45, 0, 0, time.UTC)
	staleAfter := 5 * time.Minute
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	writeRuntimeWorkerDisabledSafetyScope(t, root)
	writeRuntimeWorkerCurrentSafetyAuthorization(t, root)
	oldHeartbeat := now.Add(-staleAfter - time.Minute).Format(time.RFC3339Nano)
	for _, lease := range []WorkerLeaseArtifact{
		{
			Version:     1,
			RunID:       "run-heartbeat-authorized",
			ExecutionID: "tool-heartbeat-authorized",
			Tool:        "web-api-pack",
			Capability:  "web.discovery",
			WorkerID:    "remote-web-1",
			Status:      "active",
			AcquiredAt:  oldHeartbeat,
			HeartbeatAt: oldHeartbeat,
		},
		{
			Version:     1,
			RunID:       "run-release-authorized",
			ExecutionID: "tool-release-authorized",
			Tool:        "web-api-pack",
			Capability:  "web.discovery",
			WorkerID:    "remote-web-1",
			Status:      "active",
			AcquiredAt:  oldHeartbeat,
			HeartbeatAt: oldHeartbeat,
		},
		{
			Version:     1,
			RunID:       "run-reclaim-authorized",
			ExecutionID: "tool-reclaim-authorized",
			Tool:        "web-api-pack",
			Capability:  "web.discovery",
			WorkerID:    "local",
			Status:      "active",
			AcquiredAt:  oldHeartbeat,
			HeartbeatAt: oldHeartbeat,
		},
	} {
		if err := writeWorkerLeaseArtifact(root, lease); err != nil {
			t.Fatalf("write worker lease fixture: %v", err)
		}
	}

	heartbeat, err := UpdateWorkerLeaseHeartbeat(root, "run-heartbeat-authorized", "tool-heartbeat-authorized", WorkerLeaseHeartbeatOptions{
		Heartbeat:   true,
		Now:         now,
		HeartbeatBy: "remote-web-1",
		Reason:      "authorized heartbeat",
	})
	if err != nil {
		t.Fatalf("heartbeat with current safety authorization: %v", err)
	}
	if heartbeat.HeartbeatAt != now.Format(time.RFC3339Nano) || heartbeat.HeartbeatBy != "remote-web-1" {
		t.Fatalf("expected authorized heartbeat mutation, got %#v", heartbeat)
	}
	released, err := ReleaseWorkerLease(root, "run-release-authorized", "tool-release-authorized", WorkerLeaseReleaseOptions{
		Release:    true,
		Status:     "completed",
		Now:        now.Add(time.Minute),
		ReleasedBy: "remote-web-1",
		Reason:     "authorized release",
	})
	if err != nil {
		t.Fatalf("release with current safety authorization: %v", err)
	}
	if released.Status != "completed" || released.ReleasedBy != "remote-web-1" || released.ReleasedAt == "" {
		t.Fatalf("expected authorized release mutation, got %#v", released)
	}
	dispatch, err := CreateWorkerLeaseReclaimDispatch(root, now.Add(2*time.Minute), staleAfter)
	if err != nil {
		t.Fatalf("create reclaim dispatch: %v", err)
	}
	reclaim, err := ExecuteWorkerLeaseReclaimDispatch(root, filepath.Base(dispatch.Path), WorkerLeaseReclaimExecutionOptions{
		Execute:    true,
		Now:        now.Add(2 * time.Minute),
		StaleAfter: staleAfter,
	})
	if err != nil {
		t.Fatalf("reclaim with current safety authorization: %v", err)
	}
	if len(reclaim.Results) != 1 || reclaim.Results[0].ExecutionID != "tool-reclaim-authorized" || reclaim.Results[0].Status != "reclaimed" {
		t.Fatalf("expected authorized reclaim result, got %#v", reclaim)
	}
}

func TestExecuteWorkerLeaseReclaimDispatchSkipsLeaseThatIsNoLongerStale(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Date(2026, 5, 30, 15, 0, 0, 0, time.UTC)
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
	dispatch, err := CreateWorkerLeaseReclaimDispatch(root, now, staleAfter)
	if err != nil {
		t.Fatalf("create reclaim dispatch: %v", err)
	}
	freshHeartbeat := now.Add(-time.Minute).Format(time.RFC3339Nano)
	if err := writeWorkerLeaseArtifact(root, WorkerLeaseArtifact{
		Version:     1,
		RunID:       "run-stale",
		ExecutionID: "tool-stale",
		Tool:        "web-api-pack",
		Capability:  "web.discovery",
		WorkerID:    "local",
		Status:      "active",
		AcquiredAt:  oldHeartbeat,
		HeartbeatAt: freshHeartbeat,
	}); err != nil {
		t.Fatalf("refresh worker lease fixture: %v", err)
	}

	execution, err := ExecuteWorkerLeaseReclaimDispatch(root, dispatch.Path, WorkerLeaseReclaimExecutionOptions{
		Execute:    true,
		Now:        now,
		StaleAfter: staleAfter,
	})
	if err != nil {
		t.Fatalf("execute reclaim dispatch with refreshed lease: %v", err)
	}
	if len(execution.Results) != 0 || len(execution.Skipped) != 1 || execution.Skipped[0].Reason != "active-not-stale" {
		t.Fatalf("expected refreshed lease to be skipped, got %#v", execution)
	}
	leases, err := ListWorkerLeases(root)
	if err != nil {
		t.Fatalf("list worker leases: %v", err)
	}
	if len(leases) != 1 || leases[0].Status != "active" || leases[0].ReleasedAt != "" {
		t.Fatalf("skipped reclaim must not mutate refreshed lease, got %#v", leases)
	}
}

func workerLeaseTestRequest() ToolExecutionRequest {
	return ToolExecutionRequest{
		Node: compiler.PlanNode{
			ID:           "web:discover",
			ManifestName: "web-api-pack",
			Action:       compiler.ActionDiscoverAssets,
			Capability:   "web.discovery",
			Runner:       "native",
			Parser:       "web_api_parser",
		},
		WorkerID: "shared-local",
	}
}

func TestValidateScopeConfigRejectsNegativeWorkerPoolMaxConcurrency(t *testing.T) {
	config := ScopeConfig{Version: 1}
	config.Execution.WorkerPools = []WorkerPoolPolicy{{
		ID:             "shared-local",
		Mode:           "local",
		MaxConcurrency: -1,
		Packs:          []string{"web-api-pack"},
	}}

	err := ValidateScopeConfig(config)
	if err == nil || !strings.Contains(err.Error(), "worker pool shared-local max_concurrency must be non-negative") {
		t.Fatalf("expected negative worker pool max_concurrency validation error, got %v", err)
	}
}

func TestValidateScopeConfigAcceptsDomainAndHostTargets(t *testing.T) {
	config := ScopeConfig{Version: 1}
	config.Scope.Targets = []ScopeTarget{
		{Kind: "domain", Value: "example.com"},
		{Kind: "host", Value: "api.example.com"},
	}

	if err := ValidateScopeConfig(config); err != nil {
		t.Fatalf("expected domain and host targets to be valid, got %v", err)
	}
}

func TestRunWorkspaceExecutesManifestRunCommandInAttemptSandbox(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	scope := `version: 1
scope:
  targets:
    - kind: url
      value: https://app.example
  exclusions: []
`
	if err := os.WriteFile(filepath.Join(root, "scope.yaml"), []byte(scope), 0o644); err != nil {
		t.Fatalf("write scope: %v", err)
	}
	manifest := `name: web-api-pack
version: "0.1.0"
capabilities:
  - web.discovery
runner: command
run_command:
  - printf
  - "command-runner-output\n"
parser: web_api_parser
healthcheck: built_in
`
	if err := os.WriteFile(filepath.Join(root, "toolchain", "web-api-pack", "manifest.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	result, err := RunWorkspace(ctx, root)
	if err != nil {
		t.Fatalf("run workspace: %v", err)
	}
	store, err := eventlog.OpenSQLite(filepath.Join(root, "replay", "events.db"))
	if err != nil {
		t.Fatalf("open event log: %v", err)
	}
	defer store.Close()
	events, err := store.Replay(ctx, result.RunID)
	if err != nil {
		t.Fatalf("replay events: %v", err)
	}

	for _, event := range events {
		if event.Type != eventlog.EventTool {
			continue
		}
		var payload struct {
			Action          string `json:"action"`
			Capability      string `json:"capability"`
			ExecutionStatus string `json:"execution_status"`
			Transcript      string `json:"transcript"`
			AttemptDir      string `json:"attempt_dir"`
		}
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			t.Fatalf("decode tool payload: %v", err)
		}
		if payload.Action != string(compiler.ActionDiscoverAssets) || payload.Capability != "web.discovery" {
			continue
		}
		if payload.ExecutionStatus != "completed" {
			t.Fatalf("expected command execution to complete, got %#v", payload)
		}
		if !strings.Contains(payload.Transcript, "command-runner-output") {
			t.Fatalf("expected command output in transcript, got %#v", payload)
		}
		if !strings.HasPrefix(payload.AttemptDir, filepath.Join(root, "attempts", result.RunID)) {
			t.Fatalf("expected attempt dir inside workspace attempts, got %#v", payload)
		}
		return
	}
	t.Fatalf("expected command-backed web.discovery execution event")
}

func TestRunWorkspaceExecutesNativeManagedBundleFromWorkspaceToolchainBin(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	scope := `version: 1
scope:
  target_policy: enforce
  targets:
    - kind: url
      value: https://app.example
  exclusions: []
execution:
  isolation: process
`
	if err := os.WriteFile(filepath.Join(root, "scope.yaml"), []byte(scope), 0o644); err != nil {
		t.Fatalf("write scope: %v", err)
	}
	writeNativeManagedWebAPIPackManifestForRunTest(t, root)
	marker := filepath.Join(t.TempDir(), "managed-native-ran")
	writeManagedBundleFixture(t, root, "httpx", "binary", "1.3.0", "#!/usr/bin/env sh\nprintf 'managed-native-output target=%s bin=%s rules=%s tool=%s workspace=%s\\n' \"$1\" \"$OURO_TOOLCHAIN_BIN\" \"$OURO_TOOLCHAIN_RULES\" \"$OURO_TOOL\" \"$OURO_WORKSPACE\"\ntouch "+marker+"\n")
	if _, err := ExecuteManagedToolBundleInstall(root, "web-api-pack", "httpx", ManagedToolBundleInstallOptions{Execute: true}); err != nil {
		t.Fatalf("install managed httpx bundle: %v", err)
	}

	result, err := RunWorkspace(ctx, root)
	if err != nil {
		t.Fatalf("run workspace: %v", err)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("expected native managed payload to run: %v", err)
	}
	store, err := eventlog.OpenSQLite(filepath.Join(root, "replay", "events.db"))
	if err != nil {
		t.Fatalf("open event log: %v", err)
	}
	defer store.Close()
	events, err := store.Replay(ctx, result.RunID)
	if err != nil {
		t.Fatalf("replay events: %v", err)
	}

	for _, event := range events {
		if event.Type != eventlog.EventTool {
			continue
		}
		var payload struct {
			Tool            string `json:"tool"`
			Capability      string `json:"capability"`
			Runner          string `json:"runner"`
			ExecutionStatus string `json:"execution_status"`
			Transcript      string `json:"transcript"`
			AttemptDir      string `json:"attempt_dir"`
		}
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			t.Fatalf("decode tool payload: %v", err)
		}
		if payload.Tool != "web-api-pack" || payload.Capability != "web.discovery" {
			continue
		}
		if payload.Runner != "native" || payload.ExecutionStatus != "completed" {
			t.Fatalf("expected native managed execution to complete, got %#v", payload)
		}
		for _, want := range []string{
			"managed-native-output",
			"target=https://app.example",
			"bin=" + filepath.Join(root, "toolchain", "bin"),
			"rules=" + filepath.Join(root, "toolchain", "rules"),
			"tool=web-api-pack",
			"workspace=" + root,
		} {
			if !strings.Contains(payload.Transcript, want) {
				t.Fatalf("expected native managed transcript to contain %q, got %#v", want, payload)
			}
		}
		if strings.Contains(payload.Transcript, "offline tool execution boundary") {
			t.Fatalf("native managed execution must not use offline runner, got %#v", payload)
		}
		if !strings.HasPrefix(payload.AttemptDir, filepath.Join(root, "attempts", result.RunID)) {
			t.Fatalf("expected attempt dir inside workspace attempts, got %#v", payload)
		}
		return
	}
	t.Fatalf("expected native managed web.discovery execution event, got %#v", events)
}

func TestRunWorkspaceExecutesToolLevelCatalogNodeThroughRegisteredArgvBuilder(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	scope := `version: 1
scope:
  target_policy: enforce
  targets:
    - kind: url
      value: https://app.example
  exclusions: []
execution:
  isolation: process
`
	if err := os.WriteFile(filepath.Join(root, "scope.yaml"), []byte(scope), 0o644); err != nil {
		t.Fatalf("write scope: %v", err)
	}
	marker := filepath.Join(t.TempDir(), "tool-level-catalog-httpx-ran")
	writeManagedBundleFixture(t, root, "httpx", "binary", "1.3.0", "#!/usr/bin/env sh\nprintf 'tool-level-httpx-output args=%s target=%s tool_id=%s adapter=%s operation=%s parser=%s dep=%s target_kind=%s\\n' \"$*\" \"$OURO_TARGET\" \"$OURO_TOOL_ID\" \"$OURO_ADAPTER_ID\" \"$OURO_OPERATION\" \"$OURO_PARSER_ID\" \"$OURO_DEPENDENCY_NAME\" \"$OURO_TARGET_KIND\"\ntouch "+marker+"\n")
	if _, err := ExecuteManagedToolBundleInstall(root, "web-api-pack", "httpx", ManagedToolBundleInstallOptions{Execute: true}); err != nil {
		t.Fatalf("install managed httpx bundle: %v", err)
	}

	result, err := RunWorkspace(ctx, root)
	if err != nil {
		t.Fatalf("run workspace: %v", err)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("expected tool-level catalog payload to run through registered argv builder: %v", err)
	}
	store, err := eventlog.OpenSQLite(filepath.Join(root, "replay", "events.db"))
	if err != nil {
		t.Fatalf("open event log: %v", err)
	}
	defer store.Close()
	events, err := store.Replay(ctx, result.RunID)
	if err != nil {
		t.Fatalf("replay events: %v", err)
	}
	for _, event := range events {
		if event.Type != eventlog.EventTool {
			continue
		}
		var payload struct {
			Tool            string `json:"tool"`
			Parser          string `json:"parser"`
			ExecutionStatus string `json:"execution_status"`
			Transcript      string `json:"transcript"`
		}
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			t.Fatalf("decode tool payload: %v", err)
		}
		if payload.Tool != "web-api-pack" || payload.Parser != "httpx_jsonl" || payload.ExecutionStatus == "" {
			continue
		}
		if payload.ExecutionStatus != "completed" {
			t.Fatalf("expected httpx tool-level node to complete, got %#v", payload)
		}
		for _, want := range []string{
			"tool-level-httpx-output",
			"args=-json -u https://app.example",
			"target=https://app.example",
			"tool_id=httpx",
			"adapter=managed_binary",
			"operation=http_probe",
			"parser=httpx_jsonl",
			"dep=httpx",
			"target_kind=url",
		} {
			if !strings.Contains(payload.Transcript, want) {
				t.Fatalf("expected httpx tool-level transcript to contain %q, got %#v", want, payload)
			}
		}
		if strings.Contains(payload.Transcript, "offline tool execution boundary") {
			t.Fatalf("httpx tool-level execution must not use offline runner, got %#v", payload)
		}
		return
	}
	t.Fatalf("expected httpx tool-level execution event, got %#v", events)
}

func TestRunWorkspaceToolLevelManagedBinaryRejectsExcludedScopeBeforeExecution(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	scope := `version: 1
scope:
  target_policy: enforce
  targets:
    - kind: url
      value: https://app.example
  exclusions:
    - https://app.example
execution:
  isolation: process
`
	if err := os.WriteFile(filepath.Join(root, "scope.yaml"), []byte(scope), 0o644); err != nil {
		t.Fatalf("write scope: %v", err)
	}
	marker := filepath.Join(t.TempDir(), "excluded-tool-level-httpx-ran")
	writeManagedBundleFixture(t, root, "httpx", "binary", "1.3.0", "#!/usr/bin/env sh\nprintf 'should-not-run-tool-level-httpx\\n'\ntouch "+marker+"\n")
	if _, err := ExecuteManagedToolBundleInstall(root, "web-api-pack", "httpx", ManagedToolBundleInstallOptions{Execute: true}); err != nil {
		t.Fatalf("install managed httpx bundle: %v", err)
	}
	observed := []eventlog.Event{}

	_, err := RunWorkspaceWithOptions(ctx, root, RunOptions{
		OnEvent: func(event eventlog.Event) error {
			observed = append(observed, event)
			return nil
		},
	})
	if err == nil || !strings.Contains(err.Error(), "excluded") {
		t.Fatalf("expected excluded tool-level managed execution failure, got %v", err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("excluded tool-level payload must not run, marker stat err=%v", err)
	}
	if len(observed) == 0 {
		t.Fatalf("expected persisted events before excluded tool-level managed failure")
	}
	store, err := eventlog.OpenSQLite(filepath.Join(root, "replay", "events.db"))
	if err != nil {
		t.Fatalf("open event log: %v", err)
	}
	defer store.Close()
	events, err := store.Replay(ctx, observed[0].RunID)
	if err != nil {
		t.Fatalf("replay events: %v", err)
	}
	for _, event := range events {
		if event.Type != eventlog.EventTool {
			continue
		}
		var payload struct {
			Tool            string `json:"tool"`
			Parser          string `json:"parser"`
			ExecutionStatus string `json:"execution_status"`
			Transcript      string `json:"transcript"`
		}
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			t.Fatalf("decode tool payload: %v", err)
		}
		if payload.Tool == "web-api-pack" && payload.Parser == "httpx_jsonl" && payload.ExecutionStatus == "failed" {
			if !strings.Contains(payload.Transcript, "excluded") {
				t.Fatalf("expected exclusion transcript, got %#v", payload)
			}
			if strings.Contains(payload.Transcript, "should-not-run-tool-level-httpx") {
				t.Fatalf("excluded tool-level command must not execute, got %#v", payload)
			}
			return
		}
	}
	t.Fatalf("expected durable failed tool-level managed tool event for excluded target, got %#v", events)
}

func TestRunWorkspaceExecutesToolLevelNmapThroughRegisteredArgvBuilder(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	scope := `version: 1
scope:
  target_policy: enforce
  targets:
    - kind: cidr
      value: 10.0.0.0/24
  exclusions: []
execution:
  isolation: process
`
	if err := os.WriteFile(filepath.Join(root, "scope.yaml"), []byte(scope), 0o644); err != nil {
		t.Fatalf("write scope: %v", err)
	}
	marker := filepath.Join(t.TempDir(), "tool-level-catalog-nmap-ran")
	writeManagedBundleFixture(t, root, "nmap", "binary", "7.95", "#!/usr/bin/env sh\nprintf 'tool-level-nmap-output args=%s target=%s tool_id=%s operation=%s parser=%s dep=%s target_kind=%s scope_target_kind=%s\\n' \"$*\" \"$OURO_TARGET\" \"$OURO_TOOL_ID\" \"$OURO_OPERATION\" \"$OURO_PARSER_ID\" \"$OURO_DEPENDENCY_NAME\" \"$OURO_TARGET_KIND\" \"$OURO_SCOPE_TARGET_KIND\"\ntouch "+marker+"\n")
	if _, err := ExecuteManagedToolBundleInstall(root, "infra-pack", "nmap", ManagedToolBundleInstallOptions{Execute: true}); err != nil {
		t.Fatalf("install managed nmap bundle: %v", err)
	}

	result, err := RunWorkspace(ctx, root)
	if err != nil {
		t.Fatalf("run workspace: %v", err)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("expected nmap tool-level payload to run through registered argv builder: %v", err)
	}
	store, err := eventlog.OpenSQLite(filepath.Join(root, "replay", "events.db"))
	if err != nil {
		t.Fatalf("open event log: %v", err)
	}
	defer store.Close()
	events, err := store.Replay(ctx, result.RunID)
	if err != nil {
		t.Fatalf("replay events: %v", err)
	}
	for _, event := range events {
		if event.Type != eventlog.EventTool {
			continue
		}
		var payload struct {
			Tool            string `json:"tool"`
			Parser          string `json:"parser"`
			ExecutionStatus string `json:"execution_status"`
			Transcript      string `json:"transcript"`
		}
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			t.Fatalf("decode tool payload: %v", err)
		}
		if payload.Tool != "infra-pack" || payload.Parser != "nmap_xml" || payload.ExecutionStatus == "" {
			continue
		}
		if payload.ExecutionStatus != "completed" {
			t.Fatalf("expected nmap tool-level node to complete, got %#v", payload)
		}
		for _, want := range []string{
			"tool-level-nmap-output",
			"args=-oX - 10.0.0.0/24",
			"target=10.0.0.0/24",
			"tool_id=nmap",
			"operation=service_fingerprint",
			"parser=nmap_xml",
			"dep=nmap",
			"target_kind=host",
			"scope_target_kind=cidr",
		} {
			if !strings.Contains(payload.Transcript, want) {
				t.Fatalf("expected nmap tool-level transcript to contain %q, got %#v", want, payload)
			}
		}
		if strings.Contains(payload.Transcript, "offline tool execution boundary") {
			t.Fatalf("nmap tool-level execution must not use offline runner, got %#v", payload)
		}
		return
	}
	t.Fatalf("expected nmap tool-level execution event, got %#v", events)
}

func TestNativeManagedRunCommandBuildsNucleiTemplateScanArgvWithRules(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	payloadPath := writeManagedBundleFixture(t, root, "nuclei", "binary", "3.3.0", "#!/usr/bin/env sh\nprintf 'nuclei-fixture\\n'\n")
	writeManagedRuleBundleFixtureForRunTest(t, root, "nuclei-templates", "2026.06.03", map[string]string{
		"http/ouro-test.yaml": "id: ouro-test\ninfo:\n  name: Ouro Test\n  severity: info\n",
	})
	if _, err := ExecuteManagedToolBundleInstall(root, "web-api-pack", "nuclei", ManagedToolBundleInstallOptions{Execute: true}); err != nil {
		t.Fatalf("install managed nuclei bundle: %v", err)
	}
	if _, err := ExecuteManagedToolBundleInstall(root, "web-api-pack", "nuclei-templates", ManagedToolBundleInstallOptions{Execute: true}); err != nil {
		t.Fatalf("install managed nuclei-templates bundle: %v", err)
	}
	request := ToolExecutionRequest{
		WorkspaceRoot: root,
		Node: compiler.PlanNode{
			ID:             "nuclei-template-scan:01",
			Capability:     "web.candidate_generation",
			ManifestName:   "web-api-pack",
			ToolID:         "nuclei",
			AdapterID:      "managed_binary",
			Operation:      "template_scan",
			ParserID:       "nuclei_jsonl",
			TargetKind:     "url",
			DependencyName: "nuclei",
			DependencyKind: "binary",
			Runner:         "native",
			Dependencies: []toolchain.Dependency{
				{Name: "nuclei", Kind: "binary"},
				{Name: "nuclei-templates", Kind: "rule_bundle"},
			},
		},
		ScopeTargets:  []ScopeTarget{{Kind: "url", Value: "https://app.example"}},
		IsolationMode: "process",
	}

	command, ok, err := nativeManagedRunCommand(request)
	if err != nil || !ok {
		t.Fatalf("expected nuclei template-scan builder command, command=%#v ok=%v err=%v", command, ok, err)
	}
	wantRules := filepath.Join(root, "toolchain", "rules", "nuclei-templates")
	want := []string{payloadPath, "-jsonl", "-silent", "-no-color", "-disable-update-check", "-no-stdin", "-ni", "-t", wantRules, "-u", "https://app.example"}
	if !reflect.DeepEqual(command, want) {
		t.Fatalf("unexpected nuclei argv\nwant=%#v\n got=%#v", want, command)
	}
}

func TestNativeManagedRunCommandKeepsNucleiOfflineWithoutRuleBundle(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	writeManagedBundleFixture(t, root, "nuclei", "binary", "3.3.0", "#!/usr/bin/env sh\nprintf 'nuclei-without-rules\\n'\n")
	if _, err := ExecuteManagedToolBundleInstall(root, "web-api-pack", "nuclei", ManagedToolBundleInstallOptions{Execute: true}); err != nil {
		t.Fatalf("install managed nuclei bundle: %v", err)
	}
	request := ToolExecutionRequest{
		WorkspaceRoot: root,
		Node: compiler.PlanNode{
			ID:             "nuclei-missing-rules:01",
			Capability:     "web.candidate_generation",
			ManifestName:   "web-api-pack",
			ToolID:         "nuclei",
			AdapterID:      "managed_binary",
			Operation:      "template_scan",
			ParserID:       "nuclei_jsonl",
			TargetKind:     "url",
			DependencyName: "nuclei",
			DependencyKind: "binary",
			Runner:         "native",
			Dependencies: []toolchain.Dependency{
				{Name: "nuclei", Kind: "binary"},
				{Name: "nuclei-templates", Kind: "rule_bundle"},
			},
		},
		ScopeTargets:  []ScopeTarget{{Kind: "url", Value: "https://app.example"}},
		IsolationMode: "process",
	}

	command, ok, err := nativeManagedRunCommand(request)
	if err != nil || ok || len(command) > 0 {
		t.Fatalf("nuclei must stay offline without installed templates, command=%#v ok=%v err=%v", command, ok, err)
	}
}

func TestNativeManagedRunCommandRejectsEscapingNucleiRuleBundleSymlink(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	writeManagedBundleFixture(t, root, "nuclei", "binary", "3.3.0", "#!/usr/bin/env sh\nprintf 'nuclei-escaping-rules\\n'\n")
	if _, err := ExecuteManagedToolBundleInstall(root, "web-api-pack", "nuclei", ManagedToolBundleInstallOptions{Execute: true}); err != nil {
		t.Fatalf("install managed nuclei bundle: %v", err)
	}
	outsideRules := filepath.Join(t.TempDir(), "outside-rules")
	if err := os.MkdirAll(outsideRules, 0o755); err != nil {
		t.Fatalf("create outside rules dir: %v", err)
	}
	rulesLink := filepath.Join(root, "toolchain", "rules", "nuclei-templates")
	if err := os.MkdirAll(filepath.Dir(rulesLink), 0o755); err != nil {
		t.Fatalf("create rules dir: %v", err)
	}
	if err := os.Symlink(outsideRules, rulesLink); err != nil {
		t.Fatalf("write escaping rules symlink: %v", err)
	}
	request := ToolExecutionRequest{
		WorkspaceRoot: root,
		Node: compiler.PlanNode{
			ID:             "nuclei-escaping-rules:01",
			Capability:     "web.candidate_generation",
			ManifestName:   "web-api-pack",
			ToolID:         "nuclei",
			AdapterID:      "managed_binary",
			Operation:      "template_scan",
			ParserID:       "nuclei_jsonl",
			TargetKind:     "url",
			DependencyName: "nuclei",
			DependencyKind: "binary",
			Runner:         "native",
			Dependencies: []toolchain.Dependency{
				{Name: "nuclei", Kind: "binary"},
				{Name: "nuclei-templates", Kind: "rule_bundle"},
			},
		},
		ScopeTargets:  []ScopeTarget{{Kind: "url", Value: "https://app.example"}},
		IsolationMode: "process",
	}

	command, ok, err := nativeManagedRunCommand(request)
	if err == nil || !strings.Contains(err.Error(), "rule bundle target must stay under") || !ok || len(command) > 0 {
		t.Fatalf("expected escaping rule bundle symlink rejection, command=%#v ok=%v err=%v", command, ok, err)
	}
}

func TestNativeManagedRunCommandKeepsNucleiOfflineWithoutRuleBundleDependencyContract(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	writeManagedBundleFixture(t, root, "nuclei", "binary", "3.3.0", "#!/usr/bin/env sh\nprintf 'nuclei-with-rules-without-contract\\n'\n")
	writeManagedRuleBundleFixtureForRunTest(t, root, "nuclei-templates", "2026.06.03", map[string]string{
		"http/ouro-test.yaml": "id: ouro-test\ninfo:\n  name: Ouro Test\n  severity: info\n",
	})
	if _, err := ExecuteManagedToolBundleInstall(root, "web-api-pack", "nuclei", ManagedToolBundleInstallOptions{Execute: true}); err != nil {
		t.Fatalf("install managed nuclei bundle: %v", err)
	}
	if _, err := ExecuteManagedToolBundleInstall(root, "web-api-pack", "nuclei-templates", ManagedToolBundleInstallOptions{Execute: true}); err != nil {
		t.Fatalf("install managed nuclei-templates bundle: %v", err)
	}
	request := ToolExecutionRequest{
		WorkspaceRoot: root,
		Node: compiler.PlanNode{
			ID:             "nuclei-missing-rule-contract:01",
			Capability:     "web.candidate_generation",
			ManifestName:   "web-api-pack",
			ToolID:         "nuclei",
			AdapterID:      "managed_binary",
			Operation:      "template_scan",
			ParserID:       "nuclei_jsonl",
			TargetKind:     "url",
			DependencyName: "nuclei",
			DependencyKind: "binary",
			Runner:         "native",
			Dependencies:   []toolchain.Dependency{{Name: "nuclei", Kind: "binary"}},
		},
		ScopeTargets:  []ScopeTarget{{Kind: "url", Value: "https://app.example"}},
		IsolationMode: "process",
	}

	command, ok, err := nativeManagedRunCommand(request)
	if err != nil || ok || len(command) > 0 {
		t.Fatalf("nuclei must stay offline without declared templates dependency, command=%#v ok=%v err=%v", command, ok, err)
	}
}

func TestNativeManagedRunCommandKeepsNucleiOfflineForNonURLScope(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	writeManagedBundleFixture(t, root, "nuclei", "binary", "3.3.0", "#!/usr/bin/env sh\nprintf 'nuclei-non-url\\n'\n")
	writeManagedRuleBundleFixtureForRunTest(t, root, "nuclei-templates", "2026.06.03", map[string]string{
		"http/ouro-test.yaml": "id: ouro-test\ninfo:\n  name: Ouro Test\n  severity: info\n",
	})
	if _, err := ExecuteManagedToolBundleInstall(root, "web-api-pack", "nuclei", ManagedToolBundleInstallOptions{Execute: true}); err != nil {
		t.Fatalf("install managed nuclei bundle: %v", err)
	}
	if _, err := ExecuteManagedToolBundleInstall(root, "web-api-pack", "nuclei-templates", ManagedToolBundleInstallOptions{Execute: true}); err != nil {
		t.Fatalf("install managed nuclei-templates bundle: %v", err)
	}
	request := ToolExecutionRequest{
		WorkspaceRoot: root,
		Node: compiler.PlanNode{
			ID:             "nuclei-non-url-scope:01",
			Capability:     "web.candidate_generation",
			ManifestName:   "web-api-pack",
			ToolID:         "nuclei",
			AdapterID:      "managed_binary",
			Operation:      "template_scan",
			ParserID:       "nuclei_jsonl",
			TargetKind:     "url",
			DependencyName: "nuclei",
			DependencyKind: "binary",
			Runner:         "native",
			Dependencies: []toolchain.Dependency{
				{Name: "nuclei", Kind: "binary"},
				{Name: "nuclei-templates", Kind: "rule_bundle"},
			},
		},
		ScopeTargets:  []ScopeTarget{{Kind: "cidr", Value: "10.0.0.0/24"}},
		IsolationMode: "process",
	}

	command, ok, err := nativeManagedRunCommand(request)
	if err != nil || ok || len(command) > 0 {
		t.Fatalf("nuclei must stay offline for non-URL scope, command=%#v ok=%v err=%v", command, ok, err)
	}
}

func TestNativeManagedRunCommandBuildsMobsfscanStaticCodeScanArgvForArtifactScope(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	artifactPath := filepath.Join(root, "artifacts", "samples", "app.apk")
	if err := os.MkdirAll(filepath.Dir(artifactPath), 0o755); err != nil {
		t.Fatalf("create artifact dir: %v", err)
	}
	if err := os.WriteFile(artifactPath, []byte("apk fixture"), 0o644); err != nil {
		t.Fatalf("write artifact fixture: %v", err)
	}
	payloadPath := writeManagedBundleFixture(t, root, "mobsfscan", "binary", "0.4.0", "#!/usr/bin/env sh\nprintf 'mobsfscan-fixture\\n'\n")
	if _, err := ExecuteManagedToolBundleInstall(root, "ctf-pack", "mobsfscan", ManagedToolBundleInstallOptions{Execute: true}); err != nil {
		t.Fatalf("install managed mobsfscan bundle: %v", err)
	}
	request := ToolExecutionRequest{
		WorkspaceRoot: root,
		Node: compiler.PlanNode{
			ID:             "mobsfscan-static-code-scan:01",
			Capability:     "ctf.artifact_analysis",
			ManifestName:   "ctf-pack",
			ToolID:         "mobsfscan",
			AdapterID:      "managed_binary",
			Operation:      "static_code_scan",
			ParserID:       "mobsfscan_json",
			TargetKind:     "artifact",
			DependencyName: "mobsfscan",
			DependencyKind: "binary",
			Runner:         "native",
		},
		ScopeTargets:  []ScopeTarget{{Kind: "artifact", Value: "samples/app.apk"}},
		IsolationMode: "process",
	}

	command, ok, err := nativeManagedRunCommand(request)
	if err != nil || !ok {
		t.Fatalf("expected mobsfscan static-code-scan builder command, command=%#v ok=%v err=%v", command, ok, err)
	}
	want := []string{payloadPath, "--json", "--no-fail", artifactPath}
	if !reflect.DeepEqual(command, want) {
		t.Fatalf("unexpected mobsfscan argv\nwant=%#v\n got=%#v", want, command)
	}
}

func TestNativeManagedRunCommandKeepsMobsfscanOfflineWithoutArtifactScope(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	writeManagedBundleFixture(t, root, "mobsfscan", "binary", "0.4.0", "#!/usr/bin/env sh\nprintf 'mobsfscan-non-artifact\\n'\n")
	if _, err := ExecuteManagedToolBundleInstall(root, "ctf-pack", "mobsfscan", ManagedToolBundleInstallOptions{Execute: true}); err != nil {
		t.Fatalf("install managed mobsfscan bundle: %v", err)
	}
	for _, scopeTarget := range []ScopeTarget{
		{Kind: "url", Value: "https://app.example"},
		{Kind: "challenge", Value: "mobile-reverse"},
	} {
		t.Run(scopeTarget.Kind, func(t *testing.T) {
			request := ToolExecutionRequest{
				WorkspaceRoot: root,
				Node: compiler.PlanNode{
					ID:             "mobsfscan-non-artifact-scope:01",
					Capability:     "ctf.artifact_analysis",
					ManifestName:   "ctf-pack",
					ToolID:         "mobsfscan",
					AdapterID:      "managed_binary",
					Operation:      "static_code_scan",
					ParserID:       "mobsfscan_json",
					TargetKind:     "artifact",
					DependencyName: "mobsfscan",
					DependencyKind: "binary",
					Runner:         "native",
				},
				ScopeTargets:  []ScopeTarget{scopeTarget},
				IsolationMode: "process",
			}

			command, ok, err := nativeManagedRunCommand(request)
			if err != nil || ok || len(command) > 0 {
				t.Fatalf("mobsfscan must stay offline for non-artifact scope %#v, command=%#v ok=%v err=%v", scopeTarget, command, ok, err)
			}
		})
	}
}

func TestNativeManagedRunCommandRejectsEscapingArtifactSymlink(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	outsideArtifact := filepath.Join(t.TempDir(), "outside.apk")
	if err := os.WriteFile(outsideArtifact, []byte("outside"), 0o644); err != nil {
		t.Fatalf("write outside artifact: %v", err)
	}
	artifactLink := filepath.Join(root, "artifacts", "samples", "app.apk")
	if err := os.MkdirAll(filepath.Dir(artifactLink), 0o755); err != nil {
		t.Fatalf("create artifact dir: %v", err)
	}
	if err := os.Symlink(outsideArtifact, artifactLink); err != nil {
		t.Fatalf("write artifact symlink: %v", err)
	}
	writeManagedBundleFixture(t, root, "mobsfscan", "binary", "0.4.0", "#!/usr/bin/env sh\nprintf 'should-not-run-escaping-artifact\\n'\n")
	if _, err := ExecuteManagedToolBundleInstall(root, "ctf-pack", "mobsfscan", ManagedToolBundleInstallOptions{Execute: true}); err != nil {
		t.Fatalf("install managed mobsfscan bundle: %v", err)
	}
	request := ToolExecutionRequest{
		WorkspaceRoot: root,
		Node: compiler.PlanNode{
			ID:             "mobsfscan-escaping-artifact:01",
			Capability:     "ctf.artifact_analysis",
			ManifestName:   "ctf-pack",
			ToolID:         "mobsfscan",
			AdapterID:      "managed_binary",
			Operation:      "static_code_scan",
			ParserID:       "mobsfscan_json",
			TargetKind:     "artifact",
			DependencyName: "mobsfscan",
			DependencyKind: "binary",
			Runner:         "native",
		},
		ScopeTargets:  []ScopeTarget{{Kind: "artifact", Value: "samples/app.apk"}},
		IsolationMode: "process",
	}

	command, ok, err := nativeManagedRunCommand(request)
	if err == nil || !strings.Contains(err.Error(), "artifact target must stay under") || !ok || len(command) > 0 {
		t.Fatalf("expected escaping artifact symlink rejection, command=%#v ok=%v err=%v", command, ok, err)
	}
}

func TestNativeManagedRunCommandRejectsSymlinkedArtifactRoot(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	outsideRoot := filepath.Join(t.TempDir(), "outside-artifacts")
	outsideArtifact := filepath.Join(outsideRoot, "samples", "app.apk")
	if err := os.MkdirAll(filepath.Dir(outsideArtifact), 0o755); err != nil {
		t.Fatalf("create outside artifact dir: %v", err)
	}
	if err := os.WriteFile(outsideArtifact, []byte("outside artifact"), 0o644); err != nil {
		t.Fatalf("write outside artifact: %v", err)
	}
	artifactRoot := filepath.Join(root, "artifacts")
	if err := os.RemoveAll(artifactRoot); err != nil {
		t.Fatalf("remove artifact root: %v", err)
	}
	if err := os.Symlink(outsideRoot, artifactRoot); err != nil {
		t.Fatalf("symlink artifact root: %v", err)
	}
	writeManagedBundleFixture(t, root, "mobsfscan", "binary", "0.4.0", "#!/usr/bin/env sh\nprintf 'should-not-run-symlinked-root\\n'\n")
	if _, err := ExecuteManagedToolBundleInstall(root, "ctf-pack", "mobsfscan", ManagedToolBundleInstallOptions{Execute: true}); err != nil {
		t.Fatalf("install managed mobsfscan bundle: %v", err)
	}
	request := ToolExecutionRequest{
		WorkspaceRoot: root,
		Node: compiler.PlanNode{
			ID:             "mobsfscan-symlinked-artifact-root:01",
			Capability:     "ctf.artifact_analysis",
			ManifestName:   "ctf-pack",
			ToolID:         "mobsfscan",
			AdapterID:      "managed_binary",
			Operation:      "static_code_scan",
			ParserID:       "mobsfscan_json",
			TargetKind:     "artifact",
			DependencyName: "mobsfscan",
			DependencyKind: "binary",
			Runner:         "native",
		},
		ScopeTargets:  []ScopeTarget{{Kind: "artifact", Value: "samples/app.apk"}},
		IsolationMode: "process",
	}

	command, ok, err := nativeManagedRunCommand(request)
	if err == nil || !strings.Contains(err.Error(), "workspace artifact root must not be a symlink") || !ok || len(command) > 0 {
		t.Fatalf("expected symlinked artifact root rejection, command=%#v ok=%v err=%v", command, ok, err)
	}
}

func TestValidateArtifactRunCommandArgumentsRejectsAbsolutePathContainingEquals(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	artifactPath := filepath.Join(root, "artifacts", "sample.apk")
	if err := os.WriteFile(artifactPath, []byte("scoped artifact"), 0o644); err != nil {
		t.Fatalf("write scoped artifact: %v", err)
	}
	outsideArtifact := filepath.Join(t.TempDir(), "outside=sample.apk")
	if err := os.WriteFile(outsideArtifact, []byte("outside artifact"), 0o644); err != nil {
		t.Fatalf("write outside artifact: %v", err)
	}
	target := ScopeTarget{Kind: "artifact", Value: "sample.apk"}
	request := ToolExecutionRequest{
		WorkspaceRoot: root,
		Node: compiler.PlanNode{
			ID:         "artifact-argument-with-equals:01",
			Capability: "ctf.artifact_analysis",
			TargetKind: "artifact",
		},
		ScopeTargets: []ScopeTarget{target},
	}

	err := validateArtifactRunCommandArguments([]string{"mobsfscan", outsideArtifact}, request, target)
	if err == nil || !strings.Contains(err.Error(), "workspace artifact target must stay under") {
		t.Fatalf("expected absolute artifact path with equals to be rejected, err=%v", err)
	}
}

func TestRunWorkspaceExecutesToolLevelMobsfscanThroughExplicitArtifactScope(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	artifactPath := filepath.Join(root, "artifacts", "samples", "app.apk")
	if err := os.MkdirAll(filepath.Dir(artifactPath), 0o755); err != nil {
		t.Fatalf("create artifact dir: %v", err)
	}
	if err := os.WriteFile(artifactPath, []byte("apk fixture"), 0o644); err != nil {
		t.Fatalf("write artifact fixture: %v", err)
	}
	scope := `version: 1
scope:
  target_policy: enforce
  targets:
    - kind: artifact
      value: samples/app.apk
  exclusions: []
execution:
  isolation: process
`
	if err := os.WriteFile(filepath.Join(root, "scope.yaml"), []byte(scope), 0o644); err != nil {
		t.Fatalf("write scope: %v", err)
	}
	marker := filepath.Join(t.TempDir(), "tool-level-catalog-mobsfscan-ran")
	writeManagedBundleFixture(t, root, "mobsfscan", "binary", "0.4.0", "#!/usr/bin/env sh\nprintf 'tool-level-mobsfscan-output args=%s target=%s tool_id=%s operation=%s parser=%s dep=%s target_kind=%s scope_target_kind=%s\\n' \"$*\" \"$OURO_TARGET\" \"$OURO_TOOL_ID\" \"$OURO_OPERATION\" \"$OURO_PARSER_ID\" \"$OURO_DEPENDENCY_NAME\" \"$OURO_TARGET_KIND\" \"$OURO_SCOPE_TARGET_KIND\"\ntouch "+marker+"\n")
	if _, err := ExecuteManagedToolBundleInstall(root, "ctf-pack", "mobsfscan", ManagedToolBundleInstallOptions{Execute: true}); err != nil {
		t.Fatalf("install managed mobsfscan bundle: %v", err)
	}

	result, err := RunWorkspaceWithOptions(ctx, root, RunOptions{Action: compiler.ActionFingerprintSurface})
	if err != nil {
		t.Fatalf("run workspace: %v", err)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("expected mobsfscan artifact payload to run through registered argv builder: %v", err)
	}
	store, err := eventlog.OpenSQLite(filepath.Join(root, "replay", "events.db"))
	if err != nil {
		t.Fatalf("open event log: %v", err)
	}
	defer store.Close()
	events, err := store.Replay(ctx, result.RunID)
	if err != nil {
		t.Fatalf("replay events: %v", err)
	}
	for _, event := range events {
		if event.Type != eventlog.EventTool {
			continue
		}
		var payload struct {
			Tool            string `json:"tool"`
			Parser          string `json:"parser"`
			ExecutionStatus string `json:"execution_status"`
			Transcript      string `json:"transcript"`
		}
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			t.Fatalf("decode tool payload: %v", err)
		}
		if payload.Tool != "ctf-pack" || payload.Parser != "mobsfscan_json" || payload.ExecutionStatus == "" {
			continue
		}
		if payload.ExecutionStatus != "completed" {
			t.Fatalf("expected mobsfscan tool-level node to complete, got %#v", payload)
		}
		for _, want := range []string{
			"tool-level-mobsfscan-output",
			"args=--json --no-fail " + artifactPath,
			"target=" + artifactPath,
			"tool_id=mobsfscan",
			"operation=static_code_scan",
			"parser=mobsfscan_json",
			"dep=mobsfscan",
			"target_kind=artifact",
			"scope_target_kind=artifact",
		} {
			if !strings.Contains(payload.Transcript, want) {
				t.Fatalf("expected mobsfscan tool-level transcript to contain %q, got %#v", want, payload)
			}
		}
		if strings.Contains(payload.Transcript, "offline tool execution boundary") {
			t.Fatalf("mobsfscan tool-level execution must not use offline runner, got %#v", payload)
		}
		return
	}
	t.Fatalf("expected mobsfscan tool-level execution event, got %#v", events)
}

func TestRunWorkspacePromotesMobsfscanJSONToEvidenceFinding(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	artifactPath := filepath.Join(root, "artifacts", "samples", "App.java")
	if err := os.MkdirAll(filepath.Dir(artifactPath), 0o755); err != nil {
		t.Fatalf("create artifact dir: %v", err)
	}
	if err := os.WriteFile(artifactPath, []byte("class App { void log() {} }\n"), 0o644); err != nil {
		t.Fatalf("write artifact fixture: %v", err)
	}
	scope := `version: 1
scope:
  target_policy: enforce
  targets:
    - kind: artifact
      value: samples/App.java
  exclusions: []
execution:
  isolation: process
  tool_concurrency: 1
`
	if err := os.WriteFile(filepath.Join(root, "scope.yaml"), []byte(scope), 0o644); err != nil {
		t.Fatalf("write artifact scope: %v", err)
	}
	writeManagedBundleFixture(t, root, "mobsfscan", "binary", "0.4.0", `#!/usr/bin/env sh
cat <<'JSON'
{
  "results": {
    "android_logging": {
      "metadata": {
        "description": "Application logs sensitive data",
        "severity": "WARNING",
        "cwe": "CWE-532",
        "owasp-mobile": "M1",
        "masvs": "MSTG-STORAGE-3"
      },
      "files": [
        {
          "file_path": "samples/App.java",
          "match_lines": [19, 19],
          "match_string": "Log.d(\"token\", token);"
        }
      ]
    }
  }
}
JSON
`)
	if _, err := ExecuteManagedToolBundleInstall(root, "ctf-pack", "mobsfscan", ManagedToolBundleInstallOptions{Execute: true}); err != nil {
		t.Fatalf("install managed mobsfscan bundle: %v", err)
	}

	result, err := RunWorkspaceWithOptions(ctx, root, RunOptions{
		Action:                compiler.ActionFingerprintSurface,
		DeferReportProjection: true,
	})
	if err != nil {
		t.Fatalf("run workspace with mobsfscan parser: %v", err)
	}
	store, err := eventlog.OpenSQLite(filepath.Join(root, "replay", "events.db"))
	if err != nil {
		t.Fatalf("open event log: %v", err)
	}
	defer store.Close()
	events, err := store.Replay(ctx, result.RunID)
	if err != nil {
		t.Fatalf("replay events: %v", err)
	}
	snapshot, err := eventlog.BuildReplaySnapshot(events)
	if err != nil {
		t.Fatalf("build replay snapshot: %v", err)
	}

	var findingID string
	for _, fact := range snapshot.Blackboard.Facts {
		if fact.Source != "parser.mobsfscan_json" {
			continue
		}
		findingID = fact.ID
		if fact.Kind != "finding" {
			t.Fatalf("expected mobsfscan parser fact kind finding, got %#v", fact)
		}
		for _, want := range []string{"mobsfscan", "android_logging", "WARNING", "samples/App.java:19", "CWE-532"} {
			if !strings.Contains(fact.Statement, want) {
				t.Fatalf("expected mobsfscan finding to contain %q, got %#v", want, fact)
			}
		}
		if fact.Confidence < 0.7 {
			t.Fatalf("expected warning finding confidence to be promoted above transcript level, got %#v", fact)
		}
		break
	}
	if findingID == "" {
		t.Fatalf("expected mobsfscan parser finding fact in replay snapshot, facts=%#v", snapshot.Blackboard.Facts)
	}
	node, ok := snapshot.Evidence.Node(findingID)
	if !ok {
		t.Fatalf("expected parser fact to project into evidence graph")
	}
	if node.Type != evidence.NodeFinding {
		t.Fatalf("expected evidence finding node, got %#v", node)
	}

	foundFindingFactEvent := false
	for _, event := range events {
		if event.Type != eventlog.EventFact {
			continue
		}
		var payload struct {
			FactID    string `json:"fact_id"`
			Source    string `json:"source"`
			RuleID    string `json:"rule_id"`
			Severity  string `json:"severity"`
			FilePath  string `json:"file_path"`
			LineStart int    `json:"line_start"`
			ParserID  string `json:"parser_id"`
		}
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			t.Fatalf("decode fact event: %v", err)
		}
		if payload.FactID != findingID || payload.Source != "parser.mobsfscan_json" {
			continue
		}
		foundFindingFactEvent = true
		if payload.RuleID != "android_logging" || payload.Severity != "WARNING" || payload.FilePath != "samples/App.java" || payload.LineStart != 19 || payload.ParserID != "mobsfscan_json" {
			t.Fatalf("expected structured mobsfscan fact metadata, got %#v", payload)
		}
	}
	if !foundFindingFactEvent {
		t.Fatalf("expected raw mobsfscan fact event for %s", findingID)
	}

	foundParserEvent := false
	for _, event := range events {
		if event.Type != eventlog.EventParser {
			continue
		}
		var payload struct {
			Parser string `json:"parser"`
			Status string `json:"status"`
			ToolID string `json:"tool_id"`
		}
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			t.Fatalf("decode parser event: %v", err)
		}
		if payload.Parser == "mobsfscan_json" && payload.Status == "parsed" && payload.ToolID == "mobsfscan" {
			foundParserEvent = true
		}
	}
	if !foundParserEvent {
		t.Fatalf("expected parsed mobsfscan parser event, events=%#v", events)
	}
}

func TestRunWorkspaceRecordsMobsfscanParseFailureWithoutFinding(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	artifactPath := filepath.Join(root, "artifacts", "samples", "Bad.java")
	if err := os.MkdirAll(filepath.Dir(artifactPath), 0o755); err != nil {
		t.Fatalf("create artifact dir: %v", err)
	}
	if err := os.WriteFile(artifactPath, []byte("class Bad {}\n"), 0o644); err != nil {
		t.Fatalf("write artifact fixture: %v", err)
	}
	scope := `version: 1
scope:
  target_policy: enforce
  targets:
    - kind: artifact
      value: samples/Bad.java
  exclusions: []
execution:
  isolation: process
  tool_concurrency: 1
`
	if err := os.WriteFile(filepath.Join(root, "scope.yaml"), []byte(scope), 0o644); err != nil {
		t.Fatalf("write artifact scope: %v", err)
	}
	writeManagedBundleFixture(t, root, "mobsfscan", "binary", "0.4.0", "#!/usr/bin/env sh\nprintf 'not-json\\n'\n")
	if _, err := ExecuteManagedToolBundleInstall(root, "ctf-pack", "mobsfscan", ManagedToolBundleInstallOptions{Execute: true}); err != nil {
		t.Fatalf("install managed mobsfscan bundle: %v", err)
	}

	result, err := RunWorkspaceWithOptions(ctx, root, RunOptions{
		Action:                compiler.ActionFingerprintSurface,
		DeferReportProjection: true,
	})
	if err != nil {
		t.Fatalf("parse failure should not fail completed mobsfscan run: %v", err)
	}
	store, err := eventlog.OpenSQLite(filepath.Join(root, "replay", "events.db"))
	if err != nil {
		t.Fatalf("open event log: %v", err)
	}
	defer store.Close()
	events, err := store.Replay(ctx, result.RunID)
	if err != nil {
		t.Fatalf("replay events: %v", err)
	}

	foundParseFailure := false
	for _, event := range events {
		switch event.Type {
		case eventlog.EventParser:
			var payload struct {
				Parser       string `json:"parser"`
				Status       string `json:"status"`
				ToolID       string `json:"tool_id"`
				FindingCount int    `json:"finding_count"`
			}
			if err := json.Unmarshal(event.Payload, &payload); err != nil {
				t.Fatalf("decode parser event: %v", err)
			}
			if payload.Parser == "mobsfscan_json" && payload.Status == "parse_failed" && payload.ToolID == "mobsfscan" && payload.FindingCount == 0 {
				foundParseFailure = true
			}
		case eventlog.EventFact:
			var payload struct {
				Source string `json:"source"`
			}
			if err := json.Unmarshal(event.Payload, &payload); err != nil {
				t.Fatalf("decode fact event: %v", err)
			}
			if payload.Source == "parser.mobsfscan_json" {
				t.Fatalf("parse failure must not emit mobsfscan parser fact: %#v", payload)
			}
		}
	}
	if !foundParseFailure {
		t.Fatalf("expected run-level mobsfscan parse_failed event, events=%#v", events)
	}
}

func TestRunWorkspacePromotesNucleiJSONLToEvidenceFinding(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	scope := `version: 1
scope:
  target_policy: enforce
  targets:
    - kind: url
      value: https://app.example
  exclusions: []
execution:
  isolation: process
  tool_concurrency: 1
`
	if err := os.WriteFile(filepath.Join(root, "scope.yaml"), []byte(scope), 0o644); err != nil {
		t.Fatalf("write URL scope: %v", err)
	}
	writeManagedBundleFixture(t, root, "nuclei", "binary", "3.3.0", `#!/usr/bin/env sh
cat <<'JSONL'
{"template-id":"exposure/admin-panel","info":{"name":"Exposed Admin Panel","description":"Administrative panel exposed","severity":"high","classification":{"cwe-id":["CWE-200"],"cve-id":["CVE-2024-1234"]}},"type":"http","matched-at":"https://app.example/admin","matcher-name":"word","curl-command":"curl https://app.example/admin -H 'Authorization: Bearer secret-token'","extracted-results":["secret-token"]}
JSONL
`)
	writeManagedRuleBundleFixtureForRunTest(t, root, "nuclei-templates", "2026.06.03", map[string]string{
		"http/exposure.yaml": "id: exposure-admin-panel\ninfo:\n  name: Exposed Admin Panel\n  severity: high\n",
	})
	if _, err := ExecuteManagedToolBundleInstall(root, "web-api-pack", "nuclei", ManagedToolBundleInstallOptions{Execute: true}); err != nil {
		t.Fatalf("install managed nuclei bundle: %v", err)
	}
	if _, err := ExecuteManagedToolBundleInstall(root, "web-api-pack", "nuclei-templates", ManagedToolBundleInstallOptions{Execute: true}); err != nil {
		t.Fatalf("install managed nuclei-templates bundle: %v", err)
	}

	result, err := RunWorkspaceWithOptions(ctx, root, RunOptions{
		Action:                compiler.ActionGenerateCandidates,
		DeferReportProjection: true,
	})
	if err != nil {
		t.Fatalf("run workspace with nuclei parser: %v", err)
	}
	store, err := eventlog.OpenSQLite(filepath.Join(root, "replay", "events.db"))
	if err != nil {
		t.Fatalf("open event log: %v", err)
	}
	defer store.Close()
	events, err := store.Replay(ctx, result.RunID)
	if err != nil {
		t.Fatalf("replay events: %v", err)
	}
	snapshot, err := eventlog.BuildReplaySnapshot(events)
	if err != nil {
		t.Fatalf("build replay snapshot: %v", err)
	}

	var findingID string
	for _, fact := range snapshot.Blackboard.Facts {
		if fact.Source != "parser.nuclei_jsonl" {
			continue
		}
		findingID = fact.ID
		if fact.Kind != "finding" {
			t.Fatalf("expected nuclei parser fact kind finding, got %#v", fact)
		}
		for _, want := range []string{"nuclei", "exposure/admin-panel", "HIGH", "https://app.example/admin", "CWE-200", "CVE-2024-1234"} {
			if !strings.Contains(fact.Statement, want) {
				t.Fatalf("expected nuclei finding to contain %q, got %#v", want, fact)
			}
		}
		if strings.Contains(fact.Statement, "secret-token") || strings.Contains(fact.Statement, "curl ") {
			t.Fatalf("nuclei finding statement leaked redacted command/extracted data: %#v", fact)
		}
		if fact.Confidence < 0.85 {
			t.Fatalf("expected high nuclei finding confidence, got %#v", fact)
		}
		break
	}
	if findingID == "" {
		t.Fatalf("expected nuclei parser finding fact in replay snapshot, facts=%#v", snapshot.Blackboard.Facts)
	}
	node, ok := snapshot.Evidence.Node(findingID)
	if !ok {
		t.Fatalf("expected nuclei parser fact to project into evidence graph")
	}
	if node.Type != evidence.NodeFinding {
		t.Fatalf("expected evidence finding node, got %#v", node)
	}

	foundFactEvent := false
	foundParserEvent := false
	for _, event := range events {
		switch event.Type {
		case eventlog.EventFact:
			var payload struct {
				FactID     string `json:"fact_id"`
				Source     string `json:"source"`
				TemplateID string `json:"template_id"`
				Severity   string `json:"severity"`
				MatchedURL string `json:"matched_url"`
				TargetURL  string `json:"target_url"`
				CWE        string `json:"cwe"`
				CVE        string `json:"cve"`
				ParserID   string `json:"parser_id"`
			}
			if err := json.Unmarshal(event.Payload, &payload); err != nil {
				t.Fatalf("decode fact event: %v", err)
			}
			if payload.FactID != findingID || payload.Source != "parser.nuclei_jsonl" {
				continue
			}
			foundFactEvent = true
			if payload.TemplateID != "exposure/admin-panel" || payload.Severity != "HIGH" || payload.MatchedURL != "https://app.example/admin" || payload.TargetURL != "https://app.example" || payload.CWE != "CWE-200" || payload.CVE != "CVE-2024-1234" || payload.ParserID != "nuclei_jsonl" {
				t.Fatalf("expected structured nuclei fact metadata, got %#v", payload)
			}
		case eventlog.EventParser:
			var payload struct {
				Parser string `json:"parser"`
				Status string `json:"status"`
				ToolID string `json:"tool_id"`
			}
			if err := json.Unmarshal(event.Payload, &payload); err != nil {
				t.Fatalf("decode parser event: %v", err)
			}
			if payload.Parser == "nuclei_jsonl" && payload.Status == "parsed" && payload.ToolID == "nuclei" {
				foundParserEvent = true
			}
		}
	}
	if !foundFactEvent {
		t.Fatalf("expected raw nuclei fact event for %s", findingID)
	}
	if !foundParserEvent {
		t.Fatalf("expected parsed nuclei parser event, events=%#v", events)
	}
}

func TestRunWorkspaceRecordsNucleiParseFailureWithoutFinding(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	scope := `version: 1
scope:
  target_policy: enforce
  targets:
    - kind: url
      value: https://app.example
  exclusions: []
execution:
  isolation: process
  tool_concurrency: 1
`
	if err := os.WriteFile(filepath.Join(root, "scope.yaml"), []byte(scope), 0o644); err != nil {
		t.Fatalf("write URL scope: %v", err)
	}
	writeManagedBundleFixture(t, root, "nuclei", "binary", "3.3.0", "#!/usr/bin/env sh\nprintf 'not-jsonl\\n'\n")
	writeManagedRuleBundleFixtureForRunTest(t, root, "nuclei-templates", "2026.06.03", map[string]string{
		"http/exposure.yaml": "id: exposure-admin-panel\ninfo:\n  name: Exposed Admin Panel\n  severity: high\n",
	})
	if _, err := ExecuteManagedToolBundleInstall(root, "web-api-pack", "nuclei", ManagedToolBundleInstallOptions{Execute: true}); err != nil {
		t.Fatalf("install managed nuclei bundle: %v", err)
	}
	if _, err := ExecuteManagedToolBundleInstall(root, "web-api-pack", "nuclei-templates", ManagedToolBundleInstallOptions{Execute: true}); err != nil {
		t.Fatalf("install managed nuclei-templates bundle: %v", err)
	}

	result, err := RunWorkspaceWithOptions(ctx, root, RunOptions{
		Action:                compiler.ActionGenerateCandidates,
		DeferReportProjection: true,
	})
	if err != nil {
		t.Fatalf("parse failure should not fail completed nuclei run: %v", err)
	}
	store, err := eventlog.OpenSQLite(filepath.Join(root, "replay", "events.db"))
	if err != nil {
		t.Fatalf("open event log: %v", err)
	}
	defer store.Close()
	events, err := store.Replay(ctx, result.RunID)
	if err != nil {
		t.Fatalf("replay events: %v", err)
	}

	foundParseFailure := false
	for _, event := range events {
		switch event.Type {
		case eventlog.EventParser:
			var payload struct {
				Parser       string `json:"parser"`
				Status       string `json:"status"`
				ToolID       string `json:"tool_id"`
				FindingCount int    `json:"finding_count"`
			}
			if err := json.Unmarshal(event.Payload, &payload); err != nil {
				t.Fatalf("decode parser event: %v", err)
			}
			if payload.Parser == "nuclei_jsonl" && payload.Status == "parse_failed" && payload.ToolID == "nuclei" && payload.FindingCount == 0 {
				foundParseFailure = true
			}
		case eventlog.EventFact:
			var payload struct {
				Source string `json:"source"`
			}
			if err := json.Unmarshal(event.Payload, &payload); err != nil {
				t.Fatalf("decode fact event: %v", err)
			}
			if payload.Source == "parser.nuclei_jsonl" {
				t.Fatalf("parse failure must not emit nuclei parser fact: %#v", payload)
			}
		}
	}
	if !foundParseFailure {
		t.Fatalf("expected run-level nuclei parse_failed event, events=%#v", events)
	}
}

func TestRunWorkspacePromotesHttpxJSONLToEvidenceSurface(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	scope := `version: 1
scope:
  target_policy: enforce
  targets:
    - kind: url
      value: https://app.example
  exclusions: []
execution:
  isolation: process
  tool_concurrency: 1
`
	if err := os.WriteFile(filepath.Join(root, "scope.yaml"), []byte(scope), 0o644); err != nil {
		t.Fatalf("write URL scope: %v", err)
	}
	writeManagedBundleFixture(t, root, "httpx", "binary", "1.6.0", `#!/usr/bin/env sh
cat <<'JSONL'
{"url":"https://app.example/login","input":"https://app.example","status_code":200,"content_type":"text/html; charset=utf-8","content_length":6726,"title":"secret-token admin login","webserver":"nginx secret-token","header":"Authorization: Bearer secret-token","raw":"secret-token","response":"secret-token"}
JSONL
`)
	if _, err := ExecuteManagedToolBundleInstall(root, "web-api-pack", "httpx", ManagedToolBundleInstallOptions{Execute: true}); err != nil {
		t.Fatalf("install managed httpx bundle: %v", err)
	}

	result, err := RunWorkspaceWithOptions(ctx, root, RunOptions{DeferReportProjection: true})
	if err != nil {
		t.Fatalf("run workspace with httpx parser: %v", err)
	}
	store, err := eventlog.OpenSQLite(filepath.Join(root, "replay", "events.db"))
	if err != nil {
		t.Fatalf("open event log: %v", err)
	}
	defer store.Close()
	events, err := store.Replay(ctx, result.RunID)
	if err != nil {
		t.Fatalf("replay events: %v", err)
	}
	snapshot, err := eventlog.BuildReplaySnapshot(events)
	if err != nil {
		t.Fatalf("build replay snapshot: %v", err)
	}

	var surfaceID string
	for _, fact := range snapshot.Blackboard.Facts {
		if fact.Source != "parser.httpx_jsonl" {
			continue
		}
		surfaceID = fact.ID
		if fact.Kind != "surface" {
			t.Fatalf("expected httpx parser fact kind surface, got %#v", fact)
		}
		for _, want := range []string{"httpx", "https://app.example/login", "status=200", "content_type=text/html"} {
			if !strings.Contains(fact.Statement, want) {
				t.Fatalf("expected httpx surface to contain %q, got %#v", want, fact)
			}
		}
		if strings.Contains(fact.Statement, "secret-token") || strings.Contains(fact.Statement, "Authorization") {
			t.Fatalf("httpx surface statement leaked redacted output: %#v", fact)
		}
		if fact.Confidence < 0.6 {
			t.Fatalf("expected completed httpx surface confidence, got %#v", fact)
		}
		break
	}
	if surfaceID == "" {
		t.Fatalf("expected httpx parser surface fact in replay snapshot, facts=%#v", snapshot.Blackboard.Facts)
	}
	node, ok := snapshot.Evidence.Node(surfaceID)
	if !ok {
		t.Fatalf("expected httpx parser fact to project into evidence graph")
	}
	if node.Type != evidence.NodeSurface {
		t.Fatalf("expected evidence surface node, got %#v", node)
	}

	foundFactEvent := false
	foundParserEvent := false
	for _, event := range events {
		switch event.Type {
		case eventlog.EventFact:
			var payload struct {
				FactID        string `json:"fact_id"`
				Source        string `json:"source"`
				URL           string `json:"url"`
				TargetURL     string `json:"target_url"`
				StatusCode    int    `json:"status_code"`
				ContentType   string `json:"content_type"`
				ContentLength int64  `json:"content_length"`
				ParserID      string `json:"parser_id"`
			}
			if err := json.Unmarshal(event.Payload, &payload); err != nil {
				t.Fatalf("decode fact event: %v", err)
			}
			if payload.FactID != surfaceID || payload.Source != "parser.httpx_jsonl" {
				continue
			}
			foundFactEvent = true
			if payload.URL != "https://app.example/login" || payload.TargetURL != "https://app.example" || payload.StatusCode != 200 || payload.ContentType != "text/html" || payload.ContentLength != 6726 || payload.ParserID != "httpx_jsonl" {
				t.Fatalf("expected structured httpx fact metadata, got %#v", payload)
			}
			if strings.Contains(string(event.Payload), "secret-token") || strings.Contains(string(event.Payload), "Authorization") {
				t.Fatalf("httpx fact payload leaked redacted output: %s", string(event.Payload))
			}
		case eventlog.EventParser:
			var payload struct {
				Parser string `json:"parser"`
				Status string `json:"status"`
				ToolID string `json:"tool_id"`
			}
			if err := json.Unmarshal(event.Payload, &payload); err != nil {
				t.Fatalf("decode parser event: %v", err)
			}
			if payload.Parser == "httpx_jsonl" && payload.Status == "parsed" && payload.ToolID == "httpx" {
				foundParserEvent = true
			}
		}
	}
	if !foundFactEvent {
		t.Fatalf("expected raw httpx fact event for %s", surfaceID)
	}
	if !foundParserEvent {
		t.Fatalf("expected parsed httpx parser event, events=%#v", events)
	}
}

func TestRunWorkspaceRecordsHttpxParseFailureWithoutSurface(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	scope := `version: 1
scope:
  target_policy: enforce
  targets:
    - kind: url
      value: https://app.example
  exclusions: []
execution:
  isolation: process
  tool_concurrency: 1
`
	if err := os.WriteFile(filepath.Join(root, "scope.yaml"), []byte(scope), 0o644); err != nil {
		t.Fatalf("write URL scope: %v", err)
	}
	writeManagedBundleFixture(t, root, "httpx", "binary", "1.6.0", "#!/usr/bin/env sh\nprintf 'not-jsonl\\n'\n")
	if _, err := ExecuteManagedToolBundleInstall(root, "web-api-pack", "httpx", ManagedToolBundleInstallOptions{Execute: true}); err != nil {
		t.Fatalf("install managed httpx bundle: %v", err)
	}

	result, err := RunWorkspaceWithOptions(ctx, root, RunOptions{DeferReportProjection: true})
	if err != nil {
		t.Fatalf("parse failure should not fail completed httpx run: %v", err)
	}
	store, err := eventlog.OpenSQLite(filepath.Join(root, "replay", "events.db"))
	if err != nil {
		t.Fatalf("open event log: %v", err)
	}
	defer store.Close()
	events, err := store.Replay(ctx, result.RunID)
	if err != nil {
		t.Fatalf("replay events: %v", err)
	}

	foundParseFailure := false
	for _, event := range events {
		switch event.Type {
		case eventlog.EventParser:
			var payload struct {
				Parser       string `json:"parser"`
				Status       string `json:"status"`
				ToolID       string `json:"tool_id"`
				FindingCount int    `json:"finding_count"`
			}
			if err := json.Unmarshal(event.Payload, &payload); err != nil {
				t.Fatalf("decode parser event: %v", err)
			}
			if payload.Parser == "httpx_jsonl" && payload.Status == "parse_failed" && payload.ToolID == "httpx" && payload.FindingCount == 0 {
				foundParseFailure = true
			}
		case eventlog.EventFact:
			var payload struct {
				Source string `json:"source"`
			}
			if err := json.Unmarshal(event.Payload, &payload); err != nil {
				t.Fatalf("decode fact event: %v", err)
			}
			if payload.Source == "parser.httpx_jsonl" {
				t.Fatalf("parse failure must not emit httpx parser fact: %#v", payload)
			}
		}
	}
	if !foundParseFailure {
		t.Fatalf("expected run-level httpx parse_failed event, events=%#v", events)
	}
}

func TestRunWorkspacePromotesNmapXMLToEvidenceSurfaceFromCIDR(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	scope := `version: 1
scope:
  target_policy: enforce
  targets:
    - kind: cidr
      value: 10.0.0.0/24
  exclusions: []
execution:
  isolation: process
  tool_concurrency: 1
`
	if err := os.WriteFile(filepath.Join(root, "scope.yaml"), []byte(scope), 0o644); err != nil {
		t.Fatalf("write CIDR scope: %v", err)
	}
	writeManagedBundleFixture(t, root, "nmap", "binary", "7.95", `#!/usr/bin/env sh
cat <<'XML'
<?xml version="1.0"?>
<nmaprun scanner="nmap" args="nmap -oX - 10.0.0.0/24 --script secret-token" version="7.95">
  <host>
    <address addr="10.0.0.8" addrtype="ipv4"/>
    <hostnames><hostname name="ptr.evil.example" type="PTR"/></hostnames>
    <ports>
      <port protocol="tcp" portid="443">
        <state state="open" reason="syn-ack" reason_ttl="0"/>
        <service name="https" tunnel="ssl" conf="10" product="nginx" version="1.27" extrainfo="secret-token"/>
      </port>
      <port protocol="tcp" portid="80"><state state="closed"/><service name="http"/></port>
    </ports>
  </host>
  <host>
    <address addr="10.0.1.8" addrtype="ipv4"/>
    <ports>
      <port protocol="tcp" portid="22"><state state="open"/><service name="ssh" product="OpenSSH"/></port>
    </ports>
  </host>
</nmaprun>
XML
`)
	if _, err := ExecuteManagedToolBundleInstall(root, "infra-pack", "nmap", ManagedToolBundleInstallOptions{Execute: true}); err != nil {
		t.Fatalf("install managed nmap bundle: %v", err)
	}

	result, err := RunWorkspaceWithOptions(ctx, root, RunOptions{DeferReportProjection: true})
	if err != nil {
		t.Fatalf("run workspace with nmap parser: %v", err)
	}
	store, err := eventlog.OpenSQLite(filepath.Join(root, "replay", "events.db"))
	if err != nil {
		t.Fatalf("open event log: %v", err)
	}
	defer store.Close()
	events, err := store.Replay(ctx, result.RunID)
	if err != nil {
		t.Fatalf("replay events: %v", err)
	}
	snapshot, err := eventlog.BuildReplaySnapshot(events)
	if err != nil {
		t.Fatalf("build replay snapshot: %v", err)
	}

	var surfaceID string
	for _, fact := range snapshot.Blackboard.Facts {
		if fact.Source != "parser.nmap_xml" {
			continue
		}
		surfaceID = fact.ID
		if fact.Kind != "surface" {
			t.Fatalf("expected nmap parser fact kind surface, got %#v", fact)
		}
		if fact.Statement != "nmap | 10.0.0.8 | tcp/443 | open | service=https" {
			t.Fatalf("unexpected nmap surface statement: %#v", fact)
		}
		if strings.Contains(fact.Statement, "10.0.1.8") || strings.Contains(fact.Statement, "ptr.evil.example") || strings.Contains(fact.Statement, "secret-token") || strings.Contains(fact.Statement, "nginx") {
			t.Fatalf("nmap surface statement leaked untrusted XML: %#v", fact)
		}
		if fact.Confidence < 0.7 {
			t.Fatalf("expected nmap surface confidence, got %#v", fact)
		}
		break
	}
	if surfaceID == "" {
		t.Fatalf("expected nmap parser surface fact in replay snapshot, facts=%#v", snapshot.Blackboard.Facts)
	}
	node, ok := snapshot.Evidence.Node(surfaceID)
	if !ok {
		t.Fatalf("expected nmap parser fact to project into evidence graph")
	}
	if node.Type != evidence.NodeSurface {
		t.Fatalf("expected evidence surface node, got %#v", node)
	}

	foundFactEvent := false
	foundParserEvent := false
	for _, event := range events {
		switch event.Type {
		case eventlog.EventFact:
			var payload struct {
				FactID      string `json:"fact_id"`
				Source      string `json:"source"`
				ScopeKind   string `json:"scope_kind"`
				ScopeTarget string `json:"scope_target"`
				Host        string `json:"host"`
				Address     string `json:"address"`
				Protocol    string `json:"protocol"`
				Port        int    `json:"port"`
				ServiceName string `json:"service_name"`
				ParserID    string `json:"parser_id"`
			}
			if err := json.Unmarshal(event.Payload, &payload); err != nil {
				t.Fatalf("decode fact event: %v", err)
			}
			if payload.FactID != surfaceID || payload.Source != "parser.nmap_xml" {
				continue
			}
			foundFactEvent = true
			if payload.ScopeKind != "cidr" || payload.ScopeTarget != "10.0.0.0/24" || payload.Host != "10.0.0.8" || payload.Address != "10.0.0.8" || payload.Protocol != "tcp" || payload.Port != 443 || payload.ServiceName != "https" || payload.ParserID != "nmap_xml" {
				t.Fatalf("expected structured nmap CIDR fact metadata, got %#v", payload)
			}
			for _, leaked := range []string{"10.0.1.8", "ptr.evil.example", "secret-token", "nginx", "OpenSSH", "reason_ttl"} {
				if strings.Contains(string(event.Payload), leaked) {
					t.Fatalf("nmap fact payload leaked untrusted XML %q: %s", leaked, string(event.Payload))
				}
			}
		case eventlog.EventParser:
			var payload struct {
				Parser string `json:"parser"`
				Status string `json:"status"`
				ToolID string `json:"tool_id"`
			}
			if err := json.Unmarshal(event.Payload, &payload); err != nil {
				t.Fatalf("decode parser event: %v", err)
			}
			if payload.Parser == "nmap_xml" && payload.Status == "parsed" && payload.ToolID == "nmap" {
				foundParserEvent = true
			}
		}
	}
	if !foundFactEvent {
		t.Fatalf("expected raw nmap fact event for %s", surfaceID)
	}
	if !foundParserEvent {
		t.Fatalf("expected parsed nmap parser event, events=%#v", events)
	}
}

func TestRunWorkspacePromotesNmapXMLToEvidenceSurfaceFromHost(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	scope := `version: 1
scope:
  target_policy: enforce
  targets:
    - kind: host
      value: api.example.com
  exclusions: []
execution:
  isolation: process
  tool_concurrency: 1
`
	if err := os.WriteFile(filepath.Join(root, "scope.yaml"), []byte(scope), 0o644); err != nil {
		t.Fatalf("write host scope: %v", err)
	}
	writeManagedBundleFixture(t, root, "nmap", "binary", "7.95", `#!/usr/bin/env sh
cat <<'XML'
<?xml version="1.0"?>
<nmaprun>
  <host>
    <address addr="203.0.113.10" addrtype="ipv4"/>
    <hostnames><hostname name="ptr.evil.example" type="PTR"/></hostnames>
    <ports>
      <port protocol="tcp" portid="22">
        <state state="open"/>
        <service name="ssh" conf="10" product="OpenSSH" version="9.6"/>
      </port>
    </ports>
  </host>
</nmaprun>
XML
`)
	if _, err := ExecuteManagedToolBundleInstall(root, "infra-pack", "nmap", ManagedToolBundleInstallOptions{Execute: true}); err != nil {
		t.Fatalf("install managed nmap bundle: %v", err)
	}

	result, err := RunWorkspaceWithOptions(ctx, root, RunOptions{DeferReportProjection: true})
	if err != nil {
		t.Fatalf("run workspace with host-scoped nmap parser: %v", err)
	}
	store, err := eventlog.OpenSQLite(filepath.Join(root, "replay", "events.db"))
	if err != nil {
		t.Fatalf("open event log: %v", err)
	}
	defer store.Close()
	events, err := store.Replay(ctx, result.RunID)
	if err != nil {
		t.Fatalf("replay events: %v", err)
	}
	snapshot, err := eventlog.BuildReplaySnapshot(events)
	if err != nil {
		t.Fatalf("build replay snapshot: %v", err)
	}

	var surfaceID string
	for _, fact := range snapshot.Blackboard.Facts {
		if fact.Source != "parser.nmap_xml" {
			continue
		}
		surfaceID = fact.ID
		if fact.Kind != "surface" || fact.Statement != "nmap | api.example.com | tcp/22 | open | service=ssh" {
			t.Fatalf("unexpected host-scoped nmap surface fact: %#v", fact)
		}
		if strings.Contains(fact.Statement, "ptr.evil.example") || strings.Contains(fact.Statement, "OpenSSH") || strings.Contains(fact.Statement, "9.6") {
			t.Fatalf("host-scoped nmap statement leaked untrusted XML: %#v", fact)
		}
		break
	}
	if surfaceID == "" {
		t.Fatalf("expected host-scoped nmap parser surface fact in replay snapshot, facts=%#v", snapshot.Blackboard.Facts)
	}
	node, ok := snapshot.Evidence.Node(surfaceID)
	if !ok || node.Type != evidence.NodeSurface {
		t.Fatalf("expected host-scoped nmap surface evidence node, ok=%v node=%#v", ok, node)
	}

	for _, event := range events {
		if event.Type != eventlog.EventFact {
			continue
		}
		var payload struct {
			FactID      string `json:"fact_id"`
			Source      string `json:"source"`
			ScopeKind   string `json:"scope_kind"`
			ScopeTarget string `json:"scope_target"`
			Host        string `json:"host"`
			Address     string `json:"address"`
			Protocol    string `json:"protocol"`
			Port        int    `json:"port"`
			ServiceName string `json:"service_name"`
			ParserID    string `json:"parser_id"`
		}
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			t.Fatalf("decode fact event: %v", err)
		}
		if payload.FactID != surfaceID || payload.Source != "parser.nmap_xml" {
			continue
		}
		if payload.ScopeKind != "host" || payload.ScopeTarget != "api.example.com" || payload.Host != "api.example.com" || payload.Address != "203.0.113.10" || payload.Protocol != "tcp" || payload.Port != 22 || payload.ServiceName != "ssh" || payload.ParserID != "nmap_xml" {
			t.Fatalf("expected structured host-scoped nmap fact metadata, got %#v", payload)
		}
		return
	}
	t.Fatalf("expected raw host-scoped nmap fact event for %s", surfaceID)
}

func TestRunWorkspaceRecordsNmapParseFailureWithoutSurface(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	scope := `version: 1
scope:
  target_policy: enforce
  targets:
    - kind: cidr
      value: 10.0.0.0/24
  exclusions: []
execution:
  isolation: process
  tool_concurrency: 1
`
	if err := os.WriteFile(filepath.Join(root, "scope.yaml"), []byte(scope), 0o644); err != nil {
		t.Fatalf("write CIDR scope: %v", err)
	}
	writeManagedBundleFixture(t, root, "nmap", "binary", "7.95", "#!/usr/bin/env sh\nprintf 'not-xml\\n'\n")
	if _, err := ExecuteManagedToolBundleInstall(root, "infra-pack", "nmap", ManagedToolBundleInstallOptions{Execute: true}); err != nil {
		t.Fatalf("install managed nmap bundle: %v", err)
	}

	result, err := RunWorkspaceWithOptions(ctx, root, RunOptions{DeferReportProjection: true})
	if err != nil {
		t.Fatalf("parse failure should not fail completed nmap run: %v", err)
	}
	store, err := eventlog.OpenSQLite(filepath.Join(root, "replay", "events.db"))
	if err != nil {
		t.Fatalf("open event log: %v", err)
	}
	defer store.Close()
	events, err := store.Replay(ctx, result.RunID)
	if err != nil {
		t.Fatalf("replay events: %v", err)
	}

	foundParseFailure := false
	for _, event := range events {
		switch event.Type {
		case eventlog.EventParser:
			var payload struct {
				Parser       string `json:"parser"`
				Status       string `json:"status"`
				ToolID       string `json:"tool_id"`
				FindingCount int    `json:"finding_count"`
			}
			if err := json.Unmarshal(event.Payload, &payload); err != nil {
				t.Fatalf("decode parser event: %v", err)
			}
			if payload.Parser == "nmap_xml" && payload.Status == "parse_failed" && payload.ToolID == "nmap" && payload.FindingCount == 0 {
				foundParseFailure = true
			}
		case eventlog.EventFact:
			var payload struct {
				Source string `json:"source"`
			}
			if err := json.Unmarshal(event.Payload, &payload); err != nil {
				t.Fatalf("decode fact event: %v", err)
			}
			if payload.Source == "parser.nmap_xml" {
				t.Fatalf("parse failure must not emit nmap parser fact: %#v", payload)
			}
		}
	}
	if !foundParseFailure {
		t.Fatalf("expected run-level nmap parse_failed event, events=%#v", events)
	}
}

func TestRunWorkspacePromotesSubfinderJSONLToEvidenceSurface(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	scope := `version: 1
scope:
  target_policy: enforce
  targets:
    - kind: domain
      value: example.com
  exclusions: []
execution:
  isolation: process
  tool_concurrency: 1
`
	if err := os.WriteFile(filepath.Join(root, "scope.yaml"), []byte(scope), 0o644); err != nil {
		t.Fatalf("write domain scope: %v", err)
	}
	writeManagedBundleFixture(t, root, "subfinder", "binary", "2.7.0", `#!/usr/bin/env sh
cat <<'JSONL'
{"host":"api.example.com","source":"secret-token","ip":"192.0.2.10","resolver":"8.8.8.8"}
{"host":"evil.example.net","source":"secret-token"}
JSONL
`)
	if _, err := ExecuteManagedToolBundleInstall(root, "web-api-pack", "subfinder", ManagedToolBundleInstallOptions{Execute: true}); err != nil {
		t.Fatalf("install managed subfinder bundle: %v", err)
	}

	result, err := RunWorkspaceWithOptions(ctx, root, RunOptions{DeferReportProjection: true})
	if err != nil {
		t.Fatalf("run workspace with subfinder parser: %v", err)
	}
	store, err := eventlog.OpenSQLite(filepath.Join(root, "replay", "events.db"))
	if err != nil {
		t.Fatalf("open event log: %v", err)
	}
	defer store.Close()
	events, err := store.Replay(ctx, result.RunID)
	if err != nil {
		t.Fatalf("replay events: %v", err)
	}
	snapshot, err := eventlog.BuildReplaySnapshot(events)
	if err != nil {
		t.Fatalf("build replay snapshot: %v", err)
	}

	var surfaceID string
	for _, fact := range snapshot.Blackboard.Facts {
		if fact.Source != "parser.subfinder_jsonl" {
			continue
		}
		surfaceID = fact.ID
		if fact.Kind != "surface" {
			t.Fatalf("expected subfinder parser fact kind surface, got %#v", fact)
		}
		for _, want := range []string{"subfinder", "api.example.com", "domain=example.com"} {
			if !strings.Contains(fact.Statement, want) {
				t.Fatalf("expected subfinder surface to contain %q, got %#v", want, fact)
			}
		}
		for _, leaked := range []string{"secret-token", "192.0.2.10", "8.8.8.8", "evil.example.net"} {
			if strings.Contains(fact.Statement, leaked) {
				t.Fatalf("subfinder surface statement leaked untrusted output %q: %#v", leaked, fact)
			}
		}
		if fact.Confidence < 0.6 {
			t.Fatalf("expected completed subfinder surface confidence, got %#v", fact)
		}
		break
	}
	if surfaceID == "" {
		t.Fatalf("expected subfinder parser surface fact in replay snapshot, facts=%#v", snapshot.Blackboard.Facts)
	}
	node, ok := snapshot.Evidence.Node(surfaceID)
	if !ok {
		t.Fatalf("expected subfinder parser fact to project into evidence graph")
	}
	if node.Type != evidence.NodeSurface {
		t.Fatalf("expected evidence surface node, got %#v", node)
	}

	foundFactEvent := false
	foundParserEvent := false
	for _, event := range events {
		switch event.Type {
		case eventlog.EventFact:
			var payload struct {
				FactID    string `json:"fact_id"`
				Source    string `json:"source"`
				Domain    string `json:"domain"`
				Subdomain string `json:"subdomain"`
				ParserID  string `json:"parser_id"`
			}
			if err := json.Unmarshal(event.Payload, &payload); err != nil {
				t.Fatalf("decode fact event: %v", err)
			}
			if payload.FactID != surfaceID || payload.Source != "parser.subfinder_jsonl" {
				continue
			}
			foundFactEvent = true
			if payload.Domain != "example.com" || payload.Subdomain != "api.example.com" || payload.ParserID != "subfinder_jsonl" {
				t.Fatalf("expected structured subfinder fact metadata, got %#v", payload)
			}
			for _, leaked := range []string{"secret-token", "192.0.2.10", "8.8.8.8", "evil.example.net"} {
				if strings.Contains(string(event.Payload), leaked) {
					t.Fatalf("subfinder fact payload leaked untrusted output %q: %s", leaked, string(event.Payload))
				}
			}
		case eventlog.EventParser:
			var payload struct {
				Parser string `json:"parser"`
				Status string `json:"status"`
				ToolID string `json:"tool_id"`
			}
			if err := json.Unmarshal(event.Payload, &payload); err != nil {
				t.Fatalf("decode parser event: %v", err)
			}
			if payload.Parser == "subfinder_jsonl" && payload.Status == "parsed" && payload.ToolID == "subfinder" {
				foundParserEvent = true
			}
		}
	}
	if !foundFactEvent {
		t.Fatalf("expected raw subfinder fact event for %s", surfaceID)
	}
	if !foundParserEvent {
		t.Fatalf("expected parsed subfinder parser event, events=%#v", events)
	}
}

func TestRunWorkspaceRecordsSubfinderParseFailureWithoutSurface(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	scope := `version: 1
scope:
  target_policy: enforce
  targets:
    - kind: domain
      value: example.com
  exclusions: []
execution:
  isolation: process
  tool_concurrency: 1
`
	if err := os.WriteFile(filepath.Join(root, "scope.yaml"), []byte(scope), 0o644); err != nil {
		t.Fatalf("write domain scope: %v", err)
	}
	writeManagedBundleFixture(t, root, "subfinder", "binary", "2.7.0", "#!/usr/bin/env sh\nprintf 'not-jsonl\\n'\n")
	if _, err := ExecuteManagedToolBundleInstall(root, "web-api-pack", "subfinder", ManagedToolBundleInstallOptions{Execute: true}); err != nil {
		t.Fatalf("install managed subfinder bundle: %v", err)
	}

	result, err := RunWorkspaceWithOptions(ctx, root, RunOptions{DeferReportProjection: true})
	if err != nil {
		t.Fatalf("parse failure should not fail completed subfinder run: %v", err)
	}
	store, err := eventlog.OpenSQLite(filepath.Join(root, "replay", "events.db"))
	if err != nil {
		t.Fatalf("open event log: %v", err)
	}
	defer store.Close()
	events, err := store.Replay(ctx, result.RunID)
	if err != nil {
		t.Fatalf("replay events: %v", err)
	}

	foundParseFailure := false
	for _, event := range events {
		switch event.Type {
		case eventlog.EventParser:
			var payload struct {
				Parser       string `json:"parser"`
				Status       string `json:"status"`
				ToolID       string `json:"tool_id"`
				FindingCount int    `json:"finding_count"`
			}
			if err := json.Unmarshal(event.Payload, &payload); err != nil {
				t.Fatalf("decode parser event: %v", err)
			}
			if payload.Parser == "subfinder_jsonl" && payload.Status == "parse_failed" && payload.ToolID == "subfinder" && payload.FindingCount == 0 {
				foundParseFailure = true
			}
		case eventlog.EventFact:
			var payload struct {
				Source string `json:"source"`
			}
			if err := json.Unmarshal(event.Payload, &payload); err != nil {
				t.Fatalf("decode fact event: %v", err)
			}
			if payload.Source == "parser.subfinder_jsonl" {
				t.Fatalf("parse failure must not emit subfinder parser fact: %#v", payload)
			}
		}
	}
	if !foundParseFailure {
		t.Fatalf("expected run-level subfinder parse_failed event, events=%#v", events)
	}
}

func TestRunWorkspaceRejectsExcludedArtifactBeforeMobsfscanExecution(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	artifactPath := filepath.Join(root, "artifacts", "samples", "app.apk")
	if err := os.MkdirAll(filepath.Dir(artifactPath), 0o755); err != nil {
		t.Fatalf("create artifact dir: %v", err)
	}
	if err := os.WriteFile(artifactPath, []byte("apk fixture"), 0o644); err != nil {
		t.Fatalf("write artifact fixture: %v", err)
	}
	scope := `version: 1
scope:
  target_policy: enforce
  targets:
    - kind: artifact
      value: samples/app.apk
  exclusions:
    - samples/app.apk
execution:
  isolation: process
`
	if err := os.WriteFile(filepath.Join(root, "scope.yaml"), []byte(scope), 0o644); err != nil {
		t.Fatalf("write scope: %v", err)
	}
	marker := filepath.Join(t.TempDir(), "excluded-artifact-mobsfscan-ran")
	writeManagedBundleFixture(t, root, "mobsfscan", "binary", "0.4.0", "#!/usr/bin/env sh\nprintf 'should-not-run-excluded-artifact-mobsfscan\\n'\ntouch "+marker+"\n")
	if _, err := ExecuteManagedToolBundleInstall(root, "ctf-pack", "mobsfscan", ManagedToolBundleInstallOptions{Execute: true}); err != nil {
		t.Fatalf("install managed mobsfscan bundle: %v", err)
	}

	_, err := RunWorkspaceWithOptions(ctx, root, RunOptions{Action: compiler.ActionFingerprintSurface})
	if err == nil || !strings.Contains(err.Error(), "excluded") {
		t.Fatalf("expected excluded artifact failure before mobsfscan execution, got %v", err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("excluded artifact mobsfscan payload must not run, marker stat err=%v", err)
	}
}

func TestRunWorkspaceExecutesToolLevelNaabuThroughRegisteredArgvBuilder(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	scope := `version: 1
scope:
  target_policy: enforce
  targets:
    - kind: cidr
      value: 10.0.0.0/24
  exclusions: []
execution:
  isolation: process
`
	if err := os.WriteFile(filepath.Join(root, "scope.yaml"), []byte(scope), 0o644); err != nil {
		t.Fatalf("write scope: %v", err)
	}
	marker := filepath.Join(t.TempDir(), "tool-level-catalog-naabu-ran")
	writeManagedBundleFixture(t, root, "naabu", "binary", "2.3.4", "#!/usr/bin/env sh\nprintf 'tool-level-naabu-output args=%s target=%s tool_id=%s operation=%s parser=%s dep=%s target_kind=%s scope_target_kind=%s\\n' \"$*\" \"$OURO_TARGET\" \"$OURO_TOOL_ID\" \"$OURO_OPERATION\" \"$OURO_PARSER_ID\" \"$OURO_DEPENDENCY_NAME\" \"$OURO_TARGET_KIND\" \"$OURO_SCOPE_TARGET_KIND\"\ntouch "+marker+"\n")
	if _, err := ExecuteManagedToolBundleInstall(root, "infra-pack", "naabu", ManagedToolBundleInstallOptions{Execute: true}); err != nil {
		t.Fatalf("install managed naabu bundle: %v", err)
	}

	result, err := RunWorkspace(ctx, root)
	if err != nil {
		t.Fatalf("run workspace: %v", err)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("expected naabu tool-level payload to run through registered argv builder: %v", err)
	}
	store, err := eventlog.OpenSQLite(filepath.Join(root, "replay", "events.db"))
	if err != nil {
		t.Fatalf("open event log: %v", err)
	}
	defer store.Close()
	events, err := store.Replay(ctx, result.RunID)
	if err != nil {
		t.Fatalf("replay events: %v", err)
	}
	for _, event := range events {
		if event.Type != eventlog.EventTool {
			continue
		}
		var payload struct {
			Tool            string `json:"tool"`
			Parser          string `json:"parser"`
			ExecutionStatus string `json:"execution_status"`
			Transcript      string `json:"transcript"`
		}
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			t.Fatalf("decode tool payload: %v", err)
		}
		if payload.Tool != "infra-pack" || payload.Parser != "naabu_jsonl" || payload.ExecutionStatus == "" {
			continue
		}
		if payload.ExecutionStatus != "completed" {
			t.Fatalf("expected naabu tool-level node to complete, got %#v", payload)
		}
		for _, want := range []string{
			"tool-level-naabu-output",
			"args=-json -host 10.0.0.0/24",
			"target=10.0.0.0/24",
			"tool_id=naabu",
			"operation=port_scan",
			"parser=naabu_jsonl",
			"dep=naabu",
			"target_kind=host",
			"scope_target_kind=cidr",
		} {
			if !strings.Contains(payload.Transcript, want) {
				t.Fatalf("expected naabu tool-level transcript to contain %q, got %#v", want, payload)
			}
		}
		if strings.Contains(payload.Transcript, "offline tool execution boundary") {
			t.Fatalf("naabu tool-level execution must not use offline runner, got %#v", payload)
		}
		return
	}
	t.Fatalf("expected naabu tool-level execution event, got %#v", events)
}

func TestRunWorkspacePromotesNaabuJSONLToEvidenceSurface(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	scope := `version: 1
scope:
  target_policy: enforce
  targets:
    - kind: cidr
      value: 10.0.0.0/24
  exclusions: []
execution:
  isolation: process
  tool_concurrency: 1
`
	if err := os.WriteFile(filepath.Join(root, "scope.yaml"), []byte(scope), 0o644); err != nil {
		t.Fatalf("write CIDR scope: %v", err)
	}
	writeManagedBundleFixture(t, root, "naabu", "binary", "2.3.4", `#!/usr/bin/env sh
cat <<'JSONL'
{"ip":"10.0.0.5","port":443,"host":"leak.example","timestamp":"secret-token","banner":"secret-token"}
JSONL
`)
	if _, err := ExecuteManagedToolBundleInstall(root, "infra-pack", "naabu", ManagedToolBundleInstallOptions{Execute: true}); err != nil {
		t.Fatalf("install managed naabu bundle: %v", err)
	}

	result, err := RunWorkspaceWithOptions(ctx, root, RunOptions{DeferReportProjection: true})
	if err != nil {
		t.Fatalf("run workspace with naabu parser: %v", err)
	}
	store, err := eventlog.OpenSQLite(filepath.Join(root, "replay", "events.db"))
	if err != nil {
		t.Fatalf("open event log: %v", err)
	}
	defer store.Close()
	events, err := store.Replay(ctx, result.RunID)
	if err != nil {
		t.Fatalf("replay events: %v", err)
	}
	snapshot, err := eventlog.BuildReplaySnapshot(events)
	if err != nil {
		t.Fatalf("build replay snapshot: %v", err)
	}

	var surfaceID string
	for _, fact := range snapshot.Blackboard.Facts {
		if fact.Source != "parser.naabu_jsonl" {
			continue
		}
		surfaceID = fact.ID
		if fact.Kind != "surface" {
			t.Fatalf("expected naabu parser fact kind surface, got %#v", fact)
		}
		for _, want := range []string{"naabu", "10.0.0.5:443", "open"} {
			if !strings.Contains(fact.Statement, want) {
				t.Fatalf("expected naabu surface to contain %q, got %#v", want, fact)
			}
		}
		if strings.Contains(fact.Statement, "secret-token") || strings.Contains(fact.Statement, "leak.example") {
			t.Fatalf("naabu surface statement leaked redacted output: %#v", fact)
		}
		break
	}
	if surfaceID == "" {
		t.Fatalf("expected naabu parser surface fact in replay snapshot, facts=%#v", snapshot.Blackboard.Facts)
	}
	node, ok := snapshot.Evidence.Node(surfaceID)
	if !ok {
		t.Fatalf("expected naabu parser fact to project into evidence graph")
	}
	if node.Type != evidence.NodeSurface {
		t.Fatalf("expected evidence surface node, got %#v", node)
	}

	foundFactEvent := false
	foundParserEvent := false
	for _, event := range events {
		switch event.Type {
		case eventlog.EventFact:
			var payload struct {
				FactID     string `json:"fact_id"`
				Source     string `json:"source"`
				IP         string `json:"ip"`
				Port       int    `json:"port"`
				Endpoint   string `json:"endpoint"`
				TargetCIDR string `json:"target_cidr"`
				ParserID   string `json:"parser_id"`
			}
			if err := json.Unmarshal(event.Payload, &payload); err != nil {
				t.Fatalf("decode fact event: %v", err)
			}
			if payload.FactID != surfaceID || payload.Source != "parser.naabu_jsonl" {
				continue
			}
			foundFactEvent = true
			if payload.IP != "10.0.0.5" || payload.Port != 443 || payload.Endpoint != "10.0.0.5:443" || payload.TargetCIDR != "10.0.0.0/24" || payload.ParserID != "naabu_jsonl" {
				t.Fatalf("expected structured naabu fact metadata, got %#v", payload)
			}
			if strings.Contains(string(event.Payload), "secret-token") || strings.Contains(string(event.Payload), "leak.example") {
				t.Fatalf("naabu fact payload leaked redacted output: %s", string(event.Payload))
			}
		case eventlog.EventParser:
			var payload struct {
				Parser string `json:"parser"`
				Status string `json:"status"`
				ToolID string `json:"tool_id"`
			}
			if err := json.Unmarshal(event.Payload, &payload); err != nil {
				t.Fatalf("decode parser event: %v", err)
			}
			if payload.Parser == "naabu_jsonl" && payload.Status == "parsed" && payload.ToolID == "naabu" {
				foundParserEvent = true
			}
		}
	}
	if !foundFactEvent {
		t.Fatalf("expected raw naabu fact event for %s", surfaceID)
	}
	if !foundParserEvent {
		t.Fatalf("expected parsed naabu parser event, events=%#v", events)
	}
}

func TestRunWorkspaceRecordsNaabuParseFailureWithoutSurface(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	scope := `version: 1
scope:
  target_policy: enforce
  targets:
    - kind: cidr
      value: 10.0.0.0/24
  exclusions: []
execution:
  isolation: process
  tool_concurrency: 1
`
	if err := os.WriteFile(filepath.Join(root, "scope.yaml"), []byte(scope), 0o644); err != nil {
		t.Fatalf("write CIDR scope: %v", err)
	}
	writeManagedBundleFixture(t, root, "naabu", "binary", "2.3.4", "#!/usr/bin/env sh\nprintf 'not-jsonl\\n'\n")
	if _, err := ExecuteManagedToolBundleInstall(root, "infra-pack", "naabu", ManagedToolBundleInstallOptions{Execute: true}); err != nil {
		t.Fatalf("install managed naabu bundle: %v", err)
	}

	result, err := RunWorkspaceWithOptions(ctx, root, RunOptions{DeferReportProjection: true})
	if err != nil {
		t.Fatalf("parse failure should not fail completed naabu run: %v", err)
	}
	store, err := eventlog.OpenSQLite(filepath.Join(root, "replay", "events.db"))
	if err != nil {
		t.Fatalf("open event log: %v", err)
	}
	defer store.Close()
	events, err := store.Replay(ctx, result.RunID)
	if err != nil {
		t.Fatalf("replay events: %v", err)
	}

	foundParseFailure := false
	for _, event := range events {
		switch event.Type {
		case eventlog.EventParser:
			var payload struct {
				Parser       string `json:"parser"`
				Status       string `json:"status"`
				ToolID       string `json:"tool_id"`
				FindingCount int    `json:"finding_count"`
			}
			if err := json.Unmarshal(event.Payload, &payload); err != nil {
				t.Fatalf("decode parser event: %v", err)
			}
			if payload.Parser == "naabu_jsonl" && payload.Status == "parse_failed" && payload.ToolID == "naabu" && payload.FindingCount == 0 {
				foundParseFailure = true
			}
		case eventlog.EventFact:
			var payload struct {
				Source string `json:"source"`
			}
			if err := json.Unmarshal(event.Payload, &payload); err != nil {
				t.Fatalf("decode fact event: %v", err)
			}
			if payload.Source == "parser.naabu_jsonl" {
				t.Fatalf("parse failure must not emit naabu parser fact: %#v", payload)
			}
		}
	}
	if !foundParseFailure {
		t.Fatalf("expected run-level naabu parse_failed event, events=%#v", events)
	}
}

func TestRunWorkspaceExecutesToolLevelDnsxThroughExplicitDomainScope(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	scope := `version: 1
scope:
  target_policy: enforce
  targets:
    - kind: domain
      value: example.com
  exclusions: []
execution:
  isolation: process
`
	if err := os.WriteFile(filepath.Join(root, "scope.yaml"), []byte(scope), 0o644); err != nil {
		t.Fatalf("write scope: %v", err)
	}
	marker := filepath.Join(t.TempDir(), "tool-level-catalog-dnsx-ran")
	writeManagedBundleFixture(t, root, "dnsx", "binary", "1.2.3", `#!/usr/bin/env sh
args="$*"
input=""
while [ "$#" -gt 0 ]; do
  if [ "$1" = "-l" ]; then
    shift
    input="$1"
  fi
  shift || true
done
printf 'tool-level-dnsx-output args=%s input=%s input_value=%s target=%s tool_id=%s operation=%s parser=%s dep=%s target_kind=%s scope_target_kind=%s\n' "$args" "$input" "$(cat "$input")" "$OURO_TARGET" "$OURO_TOOL_ID" "$OURO_OPERATION" "$OURO_PARSER_ID" "$OURO_DEPENDENCY_NAME" "$OURO_TARGET_KIND" "$OURO_SCOPE_TARGET_KIND"
touch `+marker+`
`)
	if _, err := ExecuteManagedToolBundleInstall(root, "infra-pack", "dnsx", ManagedToolBundleInstallOptions{Execute: true}); err != nil {
		t.Fatalf("install managed dnsx bundle: %v", err)
	}

	result, err := RunWorkspace(ctx, root)
	if err != nil {
		t.Fatalf("run workspace: %v", err)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("expected dnsx tool-level payload to run through explicit domain scope builder: %v", err)
	}
	store, err := eventlog.OpenSQLite(filepath.Join(root, "replay", "events.db"))
	if err != nil {
		t.Fatalf("open event log: %v", err)
	}
	defer store.Close()
	events, err := store.Replay(ctx, result.RunID)
	if err != nil {
		t.Fatalf("replay events: %v", err)
	}
	for _, event := range events {
		if event.Type != eventlog.EventTool {
			continue
		}
		var payload struct {
			Tool            string `json:"tool"`
			Parser          string `json:"parser"`
			ExecutionStatus string `json:"execution_status"`
			Transcript      string `json:"transcript"`
		}
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			t.Fatalf("decode tool payload: %v", err)
		}
		if payload.Tool != "infra-pack" || payload.Parser != "dnsx_jsonl" || payload.ExecutionStatus == "" {
			continue
		}
		if payload.ExecutionStatus != "completed" {
			t.Fatalf("expected dnsx tool-level node to complete, got %#v", payload)
		}
		for _, want := range []string{
			"tool-level-dnsx-output",
			"args=-j -silent -omit-raw -l ",
			"input_value=example.com",
			"target=example.com",
			"tool_id=dnsx",
			"operation=dns_probe",
			"parser=dnsx_jsonl",
			"dep=dnsx",
			"target_kind=domain",
			"scope_target_kind=domain",
		} {
			if !strings.Contains(payload.Transcript, want) {
				t.Fatalf("expected dnsx tool-level transcript to contain %q, got %#v", want, payload)
			}
		}
		if strings.Contains(payload.Transcript, "offline tool execution boundary") {
			t.Fatalf("dnsx tool-level execution must not use offline runner, got %#v", payload)
		}
		return
	}
	t.Fatalf("expected dnsx tool-level execution event, got %#v", events)
}

func TestRunWorkspacePromotesDnsxJSONLToEvidenceSurface(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	scope := `version: 1
scope:
  target_policy: enforce
  targets:
    - kind: domain
      value: example.com
  exclusions: []
execution:
  isolation: process
  tool_concurrency: 1
`
	if err := os.WriteFile(filepath.Join(root, "scope.yaml"), []byte(scope), 0o644); err != nil {
		t.Fatalf("write domain scope: %v", err)
	}
	writeManagedBundleFixture(t, root, "dnsx", "binary", "1.2.3", `#!/usr/bin/env sh
cat <<'JSONL'
{"host":"api.example.com","a":["192.0.2.10"],"cname":["alias.example.com"],"resolver":["secret-token"],"raw":"secret-token","all":["secret-token"],"cdn_name":"secret-token","asn":{"asn":"secret-token"}}
{"host":"api.evil.test","a":["198.51.100.9"],"raw":"secret-token"}
JSONL
`)
	if _, err := ExecuteManagedToolBundleInstall(root, "infra-pack", "dnsx", ManagedToolBundleInstallOptions{Execute: true}); err != nil {
		t.Fatalf("install managed dnsx bundle: %v", err)
	}

	result, err := RunWorkspaceWithOptions(ctx, root, RunOptions{DeferReportProjection: true})
	if err != nil {
		t.Fatalf("run workspace with dnsx parser: %v", err)
	}
	store, err := eventlog.OpenSQLite(filepath.Join(root, "replay", "events.db"))
	if err != nil {
		t.Fatalf("open event log: %v", err)
	}
	defer store.Close()
	events, err := store.Replay(ctx, result.RunID)
	if err != nil {
		t.Fatalf("replay events: %v", err)
	}
	snapshot, err := eventlog.BuildReplaySnapshot(events)
	if err != nil {
		t.Fatalf("build replay snapshot: %v", err)
	}

	dnsxFacts := 0
	var surfaceID string
	for _, fact := range snapshot.Blackboard.Facts {
		if fact.Source != "parser.dnsx_jsonl" {
			continue
		}
		dnsxFacts++
		if surfaceID == "" {
			surfaceID = fact.ID
		}
		if fact.Kind != "surface" {
			t.Fatalf("expected dnsx parser fact kind surface, got %#v", fact)
		}
		if !strings.Contains(fact.Statement, "dnsx") || !strings.Contains(fact.Statement, "api.example.com") {
			t.Fatalf("expected dnsx surface statement to name scoped host, got %#v", fact)
		}
		if strings.Contains(fact.Statement, "secret-token") || strings.Contains(fact.Statement, "evil.test") {
			t.Fatalf("dnsx surface statement leaked redacted output: %#v", fact)
		}
	}
	if dnsxFacts != 2 || surfaceID == "" {
		t.Fatalf("expected two dnsx parser surface facts in replay snapshot, facts=%#v", snapshot.Blackboard.Facts)
	}
	node, ok := snapshot.Evidence.Node(surfaceID)
	if !ok {
		t.Fatalf("expected dnsx parser fact to project into evidence graph")
	}
	if node.Type != evidence.NodeSurface {
		t.Fatalf("expected evidence surface node, got %#v", node)
	}

	foundFactEvent := false
	foundParserEvent := false
	for _, event := range events {
		switch event.Type {
		case eventlog.EventFact:
			var payload struct {
				Source     string `json:"source"`
				Host       string `json:"host"`
				RecordType string `json:"record_type"`
				Value      string `json:"value"`
				Domain     string `json:"domain"`
				ParserID   string `json:"parser_id"`
			}
			if err := json.Unmarshal(event.Payload, &payload); err != nil {
				t.Fatalf("decode fact event: %v", err)
			}
			if payload.Source != "parser.dnsx_jsonl" {
				continue
			}
			foundFactEvent = true
			if payload.Host != "api.example.com" || payload.Domain != "example.com" || payload.ParserID != "dnsx_jsonl" {
				t.Fatalf("expected structured dnsx fact metadata, got %#v", payload)
			}
			if payload.RecordType != "A" && payload.RecordType != "CNAME" {
				t.Fatalf("unexpected dnsx record type: %#v", payload)
			}
			if strings.Contains(string(event.Payload), "secret-token") || strings.Contains(string(event.Payload), "evil.test") {
				t.Fatalf("dnsx fact payload leaked redacted output: %s", string(event.Payload))
			}
		case eventlog.EventParser:
			var payload struct {
				Parser string `json:"parser"`
				Status string `json:"status"`
				ToolID string `json:"tool_id"`
			}
			if err := json.Unmarshal(event.Payload, &payload); err != nil {
				t.Fatalf("decode parser event: %v", err)
			}
			if payload.Parser == "dnsx_jsonl" && payload.Status == "parsed" && payload.ToolID == "dnsx" {
				foundParserEvent = true
			}
		}
	}
	if !foundFactEvent {
		t.Fatalf("expected raw dnsx fact event")
	}
	if !foundParserEvent {
		t.Fatalf("expected parsed dnsx parser event, events=%#v", events)
	}
}

func TestRunWorkspaceRecordsDnsxParseFailureWithoutSurface(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	scope := `version: 1
scope:
  target_policy: enforce
  targets:
    - kind: domain
      value: example.com
  exclusions: []
execution:
  isolation: process
  tool_concurrency: 1
`
	if err := os.WriteFile(filepath.Join(root, "scope.yaml"), []byte(scope), 0o644); err != nil {
		t.Fatalf("write domain scope: %v", err)
	}
	writeManagedBundleFixture(t, root, "dnsx", "binary", "1.2.3", "#!/usr/bin/env sh\nprintf 'not-jsonl\\n'\n")
	if _, err := ExecuteManagedToolBundleInstall(root, "infra-pack", "dnsx", ManagedToolBundleInstallOptions{Execute: true}); err != nil {
		t.Fatalf("install managed dnsx bundle: %v", err)
	}

	result, err := RunWorkspaceWithOptions(ctx, root, RunOptions{DeferReportProjection: true})
	if err != nil {
		t.Fatalf("parse failure should not fail completed dnsx run: %v", err)
	}
	store, err := eventlog.OpenSQLite(filepath.Join(root, "replay", "events.db"))
	if err != nil {
		t.Fatalf("open event log: %v", err)
	}
	defer store.Close()
	events, err := store.Replay(ctx, result.RunID)
	if err != nil {
		t.Fatalf("replay events: %v", err)
	}

	foundParseFailure := false
	for _, event := range events {
		switch event.Type {
		case eventlog.EventParser:
			var payload struct {
				Parser       string `json:"parser"`
				Status       string `json:"status"`
				ToolID       string `json:"tool_id"`
				FindingCount int    `json:"finding_count"`
			}
			if err := json.Unmarshal(event.Payload, &payload); err != nil {
				t.Fatalf("decode parser event: %v", err)
			}
			if payload.Parser == "dnsx_jsonl" && payload.Status == "parse_failed" && payload.ToolID == "dnsx" && payload.FindingCount == 0 {
				foundParseFailure = true
			}
		case eventlog.EventFact:
			var payload struct {
				Source string `json:"source"`
			}
			if err := json.Unmarshal(event.Payload, &payload); err != nil {
				t.Fatalf("decode fact event: %v", err)
			}
			if payload.Source == "parser.dnsx_jsonl" {
				t.Fatalf("parse failure must not emit dnsx parser fact: %#v", payload)
			}
		}
	}
	if !foundParseFailure {
		t.Fatalf("expected run-level dnsx parse_failed event, events=%#v", events)
	}
}

func TestRunWorkspacePromotesFfufJSONToEvidenceSurface(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	scope := `version: 1
scope:
  target_policy: enforce
  targets:
    - kind: url
      value: https://app.example
  exclusions: []
execution:
  isolation: process
  tool_concurrency: 1
`
	if err := os.WriteFile(filepath.Join(root, "scope.yaml"), []byte(scope), 0o644); err != nil {
		t.Fatalf("write URL scope: %v", err)
	}
	marker := filepath.Join(t.TempDir(), "tool-level-ffuf-ran")
	writeManagedBundleFixture(t, root, "ffuf", "binary", "2.1.0", `#!/usr/bin/env sh
cat <<'JSONL'
{"url":"https://app.example/admin?token=secret-token#frag","status":200,"length":6726,"words":120,"lines":32,"content-type":"text/html; charset=utf-8","input":{"FUZZ":"secret-token"},"redirectlocation":"https://evil.example/secret-token","resultfile":"/tmp/secret-token"}
JSONL
touch `+marker+`
`)
	if _, err := ExecuteManagedToolBundleInstall(root, "web-api-pack", "ffuf", ManagedToolBundleInstallOptions{Execute: true}); err != nil {
		t.Fatalf("install managed ffuf bundle: %v", err)
	}

	result, err := RunWorkspaceWithOptions(ctx, root, RunOptions{Action: compiler.ActionEnumeratePaths, DeferReportProjection: true})
	if err != nil {
		t.Fatalf("run workspace with ffuf parser: %v", err)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("expected ffuf payload to run through registered argv builder: %v", err)
	}
	store, err := eventlog.OpenSQLite(filepath.Join(root, "replay", "events.db"))
	if err != nil {
		t.Fatalf("open event log: %v", err)
	}
	defer store.Close()
	events, err := store.Replay(ctx, result.RunID)
	if err != nil {
		t.Fatalf("replay events: %v", err)
	}
	snapshot, err := eventlog.BuildReplaySnapshot(events)
	if err != nil {
		t.Fatalf("build replay snapshot: %v", err)
	}

	var surfaceID string
	for _, fact := range snapshot.Blackboard.Facts {
		if fact.Source != "parser.ffuf_json" {
			continue
		}
		surfaceID = fact.ID
		if fact.Kind != "surface" {
			t.Fatalf("expected ffuf parser fact kind surface, got %#v", fact)
		}
		for _, want := range []string{"ffuf", "https://app.example/admin", "status=200", "length=6726"} {
			if !strings.Contains(fact.Statement, want) {
				t.Fatalf("expected ffuf route surface to contain %q, got %#v", want, fact)
			}
		}
		if strings.Contains(fact.Statement, "secret-token") || strings.Contains(fact.Statement, "evil.example") {
			t.Fatalf("ffuf route statement leaked redacted output: %#v", fact)
		}
		if fact.Confidence < 0.6 {
			t.Fatalf("expected completed ffuf route confidence, got %#v", fact)
		}
		break
	}
	if surfaceID == "" {
		t.Fatalf("expected ffuf parser route fact in replay snapshot, facts=%#v", snapshot.Blackboard.Facts)
	}
	node, ok := snapshot.Evidence.Node(surfaceID)
	if !ok {
		t.Fatalf("expected ffuf parser fact to project into evidence graph")
	}
	if node.Type != evidence.NodeSurface {
		t.Fatalf("expected evidence surface node, got %#v", node)
	}

	foundFactEvent := false
	foundParserEvent := false
	for _, event := range events {
		switch event.Type {
		case eventlog.EventFact:
			var payload struct {
				FactID        string `json:"fact_id"`
				Source        string `json:"source"`
				URL           string `json:"url"`
				TargetURL     string `json:"target_url"`
				StatusCode    int    `json:"status_code"`
				ContentLength int64  `json:"content_length"`
				ContentWords  int64  `json:"content_words"`
				ContentLines  int64  `json:"content_lines"`
				ContentType   string `json:"content_type"`
				ParserID      string `json:"parser_id"`
			}
			if err := json.Unmarshal(event.Payload, &payload); err != nil {
				t.Fatalf("decode fact event: %v", err)
			}
			if payload.FactID != surfaceID || payload.Source != "parser.ffuf_json" {
				continue
			}
			foundFactEvent = true
			if payload.URL != "https://app.example/admin" || payload.TargetURL != "https://app.example" || payload.StatusCode != 200 || payload.ContentLength != 6726 || payload.ContentWords != 120 || payload.ContentLines != 32 || payload.ContentType != "text/html" || payload.ParserID != "ffuf_json" {
				t.Fatalf("expected structured ffuf route metadata, got %#v", payload)
			}
			if strings.Contains(string(event.Payload), "secret-token") || strings.Contains(string(event.Payload), "evil.example") || strings.Contains(string(event.Payload), "resultfile") {
				t.Fatalf("ffuf route fact payload leaked redacted output: %s", string(event.Payload))
			}
		case eventlog.EventParser:
			var payload struct {
				Parser string `json:"parser"`
				Status string `json:"status"`
				ToolID string `json:"tool_id"`
			}
			if err := json.Unmarshal(event.Payload, &payload); err != nil {
				t.Fatalf("decode parser event: %v", err)
			}
			if payload.Parser == "ffuf_json" && payload.Status == "parsed" && payload.ToolID == "ffuf" {
				foundParserEvent = true
			}
		}
	}
	if !foundFactEvent {
		t.Fatalf("expected raw ffuf fact event for %s", surfaceID)
	}
	if !foundParserEvent {
		t.Fatalf("expected parsed ffuf parser event, events=%#v", events)
	}
}

func TestRunWorkspaceRecordsFfufParseFailureWithoutSurface(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	scope := `version: 1
scope:
  target_policy: enforce
  targets:
    - kind: url
      value: https://app.example
  exclusions: []
execution:
  isolation: process
  tool_concurrency: 1
`
	if err := os.WriteFile(filepath.Join(root, "scope.yaml"), []byte(scope), 0o644); err != nil {
		t.Fatalf("write URL scope: %v", err)
	}
	writeManagedBundleFixture(t, root, "ffuf", "binary", "2.1.0", "#!/usr/bin/env sh\nprintf 'not-json\\n'\n")
	if _, err := ExecuteManagedToolBundleInstall(root, "web-api-pack", "ffuf", ManagedToolBundleInstallOptions{Execute: true}); err != nil {
		t.Fatalf("install managed ffuf bundle: %v", err)
	}

	result, err := RunWorkspaceWithOptions(ctx, root, RunOptions{Action: compiler.ActionEnumeratePaths, DeferReportProjection: true})
	if err != nil {
		t.Fatalf("parse failure should not fail completed ffuf run: %v", err)
	}
	store, err := eventlog.OpenSQLite(filepath.Join(root, "replay", "events.db"))
	if err != nil {
		t.Fatalf("open event log: %v", err)
	}
	defer store.Close()
	events, err := store.Replay(ctx, result.RunID)
	if err != nil {
		t.Fatalf("replay events: %v", err)
	}

	foundParseFailure := false
	for _, event := range events {
		switch event.Type {
		case eventlog.EventParser:
			var payload struct {
				Parser       string `json:"parser"`
				Status       string `json:"status"`
				ToolID       string `json:"tool_id"`
				FindingCount int    `json:"finding_count"`
			}
			if err := json.Unmarshal(event.Payload, &payload); err != nil {
				t.Fatalf("decode parser event: %v", err)
			}
			if payload.Parser == "ffuf_json" && payload.Status == "parse_failed" && payload.ToolID == "ffuf" && payload.FindingCount == 0 {
				foundParseFailure = true
			}
		case eventlog.EventFact:
			var payload struct {
				Source string `json:"source"`
			}
			if err := json.Unmarshal(event.Payload, &payload); err != nil {
				t.Fatalf("decode fact event: %v", err)
			}
			if payload.Source == "parser.ffuf_json" {
				t.Fatalf("parse failure must not emit ffuf parser fact: %#v", payload)
			}
		}
	}
	if !foundParseFailure {
		t.Fatalf("expected run-level ffuf parse_failed event, events=%#v", events)
	}
}

func TestRunWorkspacePromotesDalfoxJSONToEvidenceFinding(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	scope := `version: 1
scope:
  target_policy: enforce
  targets:
    - kind: url
      value: https://app.example/search?q=hello
  exclusions: []
execution:
  isolation: process
  tool_concurrency: 1
`
	if err := os.WriteFile(filepath.Join(root, "scope.yaml"), []byte(scope), 0o644); err != nil {
		t.Fatalf("write URL scope: %v", err)
	}
	marker := filepath.Join(t.TempDir(), "tool-level-dalfox-ran")
	writeManagedBundleFixture(t, root, "dalfox", "binary", "2.12.0", `#!/usr/bin/env sh
cat <<'JSON'
{"pocs":[{"type":"V","inject_type":"inHTML","poc_type":"verified","method":"GET","data":"https://app.example/search?q=%3Cscript%3Esecret-token%3C%2Fscript%3E#frag","param":"q","payload":"secret-token","evidence":"secret-token","cwe":"CWE-79","severity":"high","raw_request":"Authorization: Bearer secret-token","raw_response":"secret-token"}]}
JSON
touch `+marker+`
`)
	if _, err := ExecuteManagedToolBundleInstall(root, "web-api-pack", "dalfox", ManagedToolBundleInstallOptions{Execute: true}); err != nil {
		t.Fatalf("install managed dalfox bundle: %v", err)
	}

	result, err := RunWorkspaceWithOptions(ctx, root, RunOptions{
		Action:                compiler.ActionGenerateCandidates,
		DeferReportProjection: true,
	})
	if err != nil {
		t.Fatalf("run workspace with dalfox parser: %v", err)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("expected dalfox payload to run through registered argv builder: %v", err)
	}
	store, err := eventlog.OpenSQLite(filepath.Join(root, "replay", "events.db"))
	if err != nil {
		t.Fatalf("open event log: %v", err)
	}
	defer store.Close()
	events, err := store.Replay(ctx, result.RunID)
	if err != nil {
		t.Fatalf("replay events: %v", err)
	}
	snapshot, err := eventlog.BuildReplaySnapshot(events)
	if err != nil {
		t.Fatalf("build replay snapshot: %v", err)
	}

	var findingID string
	for _, fact := range snapshot.Blackboard.Facts {
		if fact.Source != "parser.dalfox_json" {
			continue
		}
		findingID = fact.ID
		if fact.Kind != "finding" {
			t.Fatalf("expected dalfox parser fact kind finding, got %#v", fact)
		}
		for _, want := range []string{"dalfox", "V", "HIGH", "https://app.example/search", "q", "CWE-79"} {
			if !strings.Contains(fact.Statement, want) {
				t.Fatalf("expected dalfox finding to contain %q, got %#v", want, fact)
			}
		}
		if strings.Contains(fact.Statement, "secret-token") || strings.Contains(fact.Statement, "?") || strings.Contains(fact.Statement, "#") {
			t.Fatalf("dalfox finding statement leaked redacted output: %#v", fact)
		}
		if fact.Confidence < 0.85 {
			t.Fatalf("expected high-confidence dalfox finding, got %#v", fact)
		}
		break
	}
	if findingID == "" {
		t.Fatalf("expected dalfox parser finding fact in replay snapshot, facts=%#v", snapshot.Blackboard.Facts)
	}
	node, ok := snapshot.Evidence.Node(findingID)
	if !ok {
		t.Fatalf("expected dalfox parser fact to project into evidence graph")
	}
	if node.Type != evidence.NodeFinding {
		t.Fatalf("expected evidence finding node, got %#v", node)
	}

	foundFactEvent := false
	foundParserEvent := false
	for _, event := range events {
		switch event.Type {
		case eventlog.EventFact:
			var payload struct {
				FactID     string `json:"fact_id"`
				Source     string `json:"source"`
				Type       string `json:"type"`
				InjectType string `json:"inject_type"`
				PoCType    string `json:"poc_type"`
				Method     string `json:"method"`
				Param      string `json:"param"`
				Severity   string `json:"severity"`
				CWE        string `json:"cwe"`
				MatchedURL string `json:"matched_url"`
				TargetURL  string `json:"target_url"`
				ParserID   string `json:"parser_id"`
			}
			if err := json.Unmarshal(event.Payload, &payload); err != nil {
				t.Fatalf("decode fact event: %v", err)
			}
			if payload.FactID != findingID || payload.Source != "parser.dalfox_json" {
				continue
			}
			foundFactEvent = true
			if payload.Type != "V" || payload.InjectType != "inHTML" || payload.PoCType != "verified" || payload.Method != "GET" || payload.Param != "q" || payload.Severity != "HIGH" || payload.CWE != "CWE-79" || payload.MatchedURL != "https://app.example/search" || payload.TargetURL != "https://app.example/search" || payload.ParserID != "dalfox_json" {
				t.Fatalf("expected structured dalfox finding metadata, got %#v", payload)
			}
			if strings.Contains(string(event.Payload), "secret-token") || strings.Contains(string(event.Payload), "Authorization") || strings.Contains(string(event.Payload), "raw_request") || strings.Contains(string(event.Payload), "raw_response") || strings.Contains(string(event.Payload), "payload") || strings.Contains(string(event.Payload), "evidence") {
				t.Fatalf("dalfox finding fact payload leaked redacted output: %s", string(event.Payload))
			}
		case eventlog.EventParser:
			var payload struct {
				Parser string `json:"parser"`
				Status string `json:"status"`
				ToolID string `json:"tool_id"`
			}
			if err := json.Unmarshal(event.Payload, &payload); err != nil {
				t.Fatalf("decode parser event: %v", err)
			}
			if payload.Parser == "dalfox_json" && payload.Status == "parsed" && payload.ToolID == "dalfox" {
				foundParserEvent = true
			}
		}
	}
	if !foundFactEvent {
		t.Fatalf("expected raw dalfox fact event for %s", findingID)
	}
	if !foundParserEvent {
		t.Fatalf("expected parsed dalfox parser event, events=%#v", events)
	}
}

func TestRunWorkspacePromotesArjunManagedOutputFileToEvidenceSurface(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	scope := `version: 1
scope:
  target_policy: enforce
  targets:
    - kind: url
      value: https://app.example/search?q=hello
  exclusions: []
execution:
  isolation: process
  tool_concurrency: 1
`
	if err := os.WriteFile(filepath.Join(root, "scope.yaml"), []byte(scope), 0o644); err != nil {
		t.Fatalf("write URL scope: %v", err)
	}
	marker := filepath.Join(t.TempDir(), "tool-level-arjun-ran")
	writeManagedBundleFixture(t, root, "arjun", "binary", "2.2.7", `#!/usr/bin/env sh
out=""
while [ "$#" -gt 0 ]; do
  if [ "$1" = "-oJ" ]; then
    shift
    out="$1"
  fi
  shift
done
mkdir -p "$(dirname "$out")"
cat > "$out" <<'JSON'
{
  "https://app.example/search?token=secret-token#frag": ["q", "page"],
  "https://evil.example/out?token=secret-token": ["evil"],
  "headers": {"Authorization": "Bearer secret-token"}
}
JSON
touch `+marker+`
`)
	if _, err := ExecuteManagedToolBundleInstall(root, "web-api-pack", "arjun", ManagedToolBundleInstallOptions{Execute: true}); err != nil {
		t.Fatalf("install managed arjun bundle: %v", err)
	}

	result, err := RunWorkspaceWithOptions(ctx, root, RunOptions{
		Action:                compiler.ActionEnumeratePaths,
		DeferReportProjection: true,
	})
	if err != nil {
		t.Fatalf("run workspace with arjun parser: %v", err)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("expected arjun payload to run through registered argv builder: %v", err)
	}
	store, err := eventlog.OpenSQLite(filepath.Join(root, "replay", "events.db"))
	if err != nil {
		t.Fatalf("open event log: %v", err)
	}
	defer store.Close()
	events, err := store.Replay(ctx, result.RunID)
	if err != nil {
		t.Fatalf("replay events: %v", err)
	}
	snapshot, err := eventlog.BuildReplaySnapshot(events)
	if err != nil {
		t.Fatalf("build replay snapshot: %v", err)
	}

	var surfaceID string
	for _, fact := range snapshot.Blackboard.Facts {
		if fact.Source != "parser.arjun_json" {
			continue
		}
		surfaceID = fact.ID
		if fact.Kind != "surface" {
			t.Fatalf("expected arjun parser fact kind surface, got %#v", fact)
		}
		for _, want := range []string{"arjun", "https://app.example/search", "parameter=q"} {
			if !strings.Contains(fact.Statement, want) {
				t.Fatalf("expected arjun surface to contain %q, got %#v", want, fact)
			}
		}
		for _, leaked := range []string{"secret-token", "evil.example", "Authorization", "token=", "#frag"} {
			if strings.Contains(fact.Statement, leaked) {
				t.Fatalf("arjun surface statement leaked untrusted output %q: %#v", leaked, fact)
			}
		}
		break
	}
	if surfaceID == "" {
		t.Fatalf("expected arjun parser surface fact in replay snapshot, facts=%#v", snapshot.Blackboard.Facts)
	}
	if _, ok := snapshot.Evidence.Node(surfaceID); !ok {
		t.Fatalf("expected arjun parser fact to project into evidence graph")
	}
}

func TestRunWorkspacePromotesSqlmapManagedOutputFileToEvidenceFinding(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	scope := `version: 1
scope:
  target_policy: enforce
  targets:
    - kind: url
      value: https://app.example/search?q=hello
  exclusions: []
execution:
  isolation: process
  tool_concurrency: 1
`
	if err := os.WriteFile(filepath.Join(root, "scope.yaml"), []byte(scope), 0o644); err != nil {
		t.Fatalf("write URL scope: %v", err)
	}
	marker := filepath.Join(t.TempDir(), "tool-level-sqlmap-ran")
	writeManagedBundleFixture(t, root, "sqlmap", "binary", "1.8.12", `#!/usr/bin/env sh
out=""
while [ "$#" -gt 0 ]; do
  if [ "$1" = "--output-dir" ]; then
    shift
    out="$1"
  fi
  shift
done
mkdir -p "$out"
cat > "$out/ouro-sqlmap.json" <<'JSON'
{
  "findings": [{
    "url": "https://app.example/search?q=secret-token#frag",
    "parameter": "q",
    "place": "GET",
    "technique": "boolean-based blind",
    "dbms": "PostgreSQL",
    "title": "SQL injection vulnerability",
    "vulnerable": true,
    "payload": "secret-token",
    "request": "Authorization: Bearer secret-token",
    "response": "secret-token",
    "dump": {"users": ["secret-token"]}
  }]
}
JSON
touch `+marker+`
`)
	if _, err := ExecuteManagedToolBundleInstall(root, "web-api-pack", "sqlmap", ManagedToolBundleInstallOptions{Execute: true}); err != nil {
		t.Fatalf("install managed sqlmap bundle: %v", err)
	}

	result, err := RunWorkspaceWithOptions(ctx, root, RunOptions{
		Action:                compiler.ActionGenerateCandidates,
		DeferReportProjection: true,
	})
	if err != nil {
		t.Fatalf("run workspace with sqlmap parser: %v", err)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("expected sqlmap payload to run through registered argv builder: %v", err)
	}
	store, err := eventlog.OpenSQLite(filepath.Join(root, "replay", "events.db"))
	if err != nil {
		t.Fatalf("open event log: %v", err)
	}
	defer store.Close()
	events, err := store.Replay(ctx, result.RunID)
	if err != nil {
		t.Fatalf("replay events: %v", err)
	}
	snapshot, err := eventlog.BuildReplaySnapshot(events)
	if err != nil {
		t.Fatalf("build replay snapshot: %v", err)
	}

	var findingID string
	for _, fact := range snapshot.Blackboard.Facts {
		if fact.Source != "parser.sqlmap_json" {
			continue
		}
		findingID = fact.ID
		if fact.Kind != "finding" {
			t.Fatalf("expected sqlmap parser fact kind finding, got %#v", fact)
		}
		for _, want := range []string{"sqlmap", "https://app.example/search", "parameter=q", "boolean-based blind", "PostgreSQL"} {
			if !strings.Contains(fact.Statement, want) {
				t.Fatalf("expected sqlmap finding to contain %q, got %#v", want, fact)
			}
		}
		for _, leaked := range []string{"secret-token", "Authorization", "payload", "request", "response", "dump", "?", "#frag"} {
			if strings.Contains(fact.Statement, leaked) {
				t.Fatalf("sqlmap finding statement leaked redacted output %q: %#v", leaked, fact)
			}
		}
		break
	}
	if findingID == "" {
		t.Fatalf("expected sqlmap parser finding fact in replay snapshot, facts=%#v", snapshot.Blackboard.Facts)
	}
	node, ok := snapshot.Evidence.Node(findingID)
	if !ok {
		t.Fatalf("expected sqlmap parser fact to project into evidence graph")
	}
	if node.Type != evidence.NodeFinding {
		t.Fatalf("expected evidence finding node, got %#v", node)
	}
}

func TestRunWorkspacePromotesSqlmapManagedLogToEvidenceFinding(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	scope := `version: 1
scope:
  target_policy: enforce
  targets:
    - kind: url
      value: https://app.example/search?q=hello
  exclusions: []
execution:
  isolation: process
  tool_concurrency: 1
`
	if err := os.WriteFile(filepath.Join(root, "scope.yaml"), []byte(scope), 0o644); err != nil {
		t.Fatalf("write URL scope: %v", err)
	}
	marker := filepath.Join(t.TempDir(), "tool-level-sqlmap-log-ran")
	writeManagedBundleFixture(t, root, "sqlmap", "binary", "1.8.12", `#!/usr/bin/env sh
out=""
while [ "$#" -gt 0 ]; do
  if [ "$1" = "--output-dir" ]; then
    shift
    out="$1"
  fi
  shift
done
mkdir -p "$out/app.example"
cat > "$out/app.example/log" <<'LOG'
---
Parameter: q (GET)
    Type: boolean-based blind
    Title: AND boolean-based blind - WHERE or HAVING clause
    Payload: q=secret-token AND 123=123

    Type: error-based
    Title: PostgreSQL AND error-based - WHERE or HAVING clause
    Payload: q=secret-token AND 1=CAST(secret-token AS INT)
---
[INFO] the back-end DBMS is PostgreSQL
Cookie: session=secret-token
Authorization: Bearer secret-token
LOG
touch `+marker+`
`)
	if _, err := ExecuteManagedToolBundleInstall(root, "web-api-pack", "sqlmap", ManagedToolBundleInstallOptions{Execute: true}); err != nil {
		t.Fatalf("install managed sqlmap bundle: %v", err)
	}

	result, err := RunWorkspaceWithOptions(ctx, root, RunOptions{
		Action:                compiler.ActionGenerateCandidates,
		DeferReportProjection: true,
	})
	if err != nil {
		t.Fatalf("run workspace with sqlmap log parser: %v", err)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("expected sqlmap payload to run through registered argv builder: %v", err)
	}
	store, err := eventlog.OpenSQLite(filepath.Join(root, "replay", "events.db"))
	if err != nil {
		t.Fatalf("open event log: %v", err)
	}
	defer store.Close()
	events, err := store.Replay(ctx, result.RunID)
	if err != nil {
		t.Fatalf("replay events: %v", err)
	}
	snapshot, err := eventlog.BuildReplaySnapshot(events)
	if err != nil {
		t.Fatalf("build replay snapshot: %v", err)
	}

	var findingID string
	for _, fact := range snapshot.Blackboard.Facts {
		if fact.Source != "parser.sqlmap_json" {
			continue
		}
		findingID = fact.ID
		if fact.Kind != "finding" {
			t.Fatalf("expected sqlmap log parser fact kind finding, got %#v", fact)
		}
		for _, want := range []string{"sqlmap", "https://app.example/search", "parameter=q", "boolean-based blind,error-based", "PostgreSQL"} {
			if !strings.Contains(fact.Statement, want) {
				t.Fatalf("expected sqlmap log finding to contain %q, got %#v", want, fact)
			}
		}
		for _, leaked := range []string{"secret-token", "Authorization", "Cookie", "Payload", "?", "#frag"} {
			if strings.Contains(fact.Statement, leaked) {
				t.Fatalf("sqlmap log finding statement leaked redacted output %q: %#v", leaked, fact)
			}
		}
		break
	}
	if findingID == "" {
		t.Fatalf("expected sqlmap log parser finding fact in replay snapshot, facts=%#v", snapshot.Blackboard.Facts)
	}
	node, ok := snapshot.Evidence.Node(findingID)
	if !ok {
		t.Fatalf("expected sqlmap log parser fact to project into evidence graph")
	}
	if node.Type != evidence.NodeFinding {
		t.Fatalf("expected evidence finding node, got %#v", node)
	}
}

func TestRunWorkspacePromotesURLListTextToEvidenceSurface(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	scope := `version: 1
scope:
  target_policy: enforce
  targets:
    - kind: domain
      value: example.com
  exclusions: []
execution:
  isolation: process
  tool_concurrency: 1
`
	if err := os.WriteFile(filepath.Join(root, "scope.yaml"), []byte(scope), 0o644); err != nil {
		t.Fatalf("write domain scope: %v", err)
	}
	writeManagedBundleFixture(t, root, "gau", "binary", "2.2.4", `#!/usr/bin/env sh
cat <<'URLS'
https://www.example.com/login?token=secret-token#frag
https://evil.example.net/admin
URLS
`)
	if _, err := ExecuteManagedToolBundleInstall(root, "web-api-pack", "gau", ManagedToolBundleInstallOptions{Execute: true}); err != nil {
		t.Fatalf("install managed gau bundle: %v", err)
	}

	result, err := RunWorkspaceWithOptions(ctx, root, RunOptions{Action: compiler.ActionEnumeratePaths, DeferReportProjection: true})
	if err != nil {
		t.Fatalf("run workspace with url list parser: %v", err)
	}
	store, err := eventlog.OpenSQLite(filepath.Join(root, "replay", "events.db"))
	if err != nil {
		t.Fatalf("open event log: %v", err)
	}
	defer store.Close()
	events, err := store.Replay(ctx, result.RunID)
	if err != nil {
		t.Fatalf("replay events: %v", err)
	}
	snapshot, err := eventlog.BuildReplaySnapshot(events)
	if err != nil {
		t.Fatalf("build replay snapshot: %v", err)
	}

	var surfaceID string
	for _, fact := range snapshot.Blackboard.Facts {
		if fact.Source != "parser.url_list_text" {
			continue
		}
		surfaceID = fact.ID
		if fact.Kind != "surface" {
			t.Fatalf("expected URL list parser fact kind surface, got %#v", fact)
		}
		for _, want := range []string{"archive", "https://www.example.com/login", "domain=example.com"} {
			if !strings.Contains(fact.Statement, want) {
				t.Fatalf("expected URL list surface to contain %q, got %#v", want, fact)
			}
		}
		for _, leaked := range []string{"secret-token", "evil.example.net"} {
			if strings.Contains(fact.Statement, leaked) {
				t.Fatalf("URL list surface statement leaked untrusted output %q: %#v", leaked, fact)
			}
		}
		break
	}
	if surfaceID == "" {
		t.Fatalf("expected URL list parser surface fact in replay snapshot, facts=%#v", snapshot.Blackboard.Facts)
	}
	if _, ok := snapshot.Evidence.Node(surfaceID); !ok {
		t.Fatalf("expected URL list parser fact to project into evidence graph")
	}
}

func TestNativeManagedRunCommandBuildsArjunParameterDiscoveryArgvForURLScope(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	payloadPath := writeManagedBundleFixture(t, root, "arjun", "binary", "2.2.7", "#!/usr/bin/env sh\nprintf 'arjun-url\\n'\n")
	if _, err := ExecuteManagedToolBundleInstall(root, "web-api-pack", "arjun", ManagedToolBundleInstallOptions{Execute: true}); err != nil {
		t.Fatalf("install managed arjun bundle: %v", err)
	}
	request := ToolExecutionRequest{
		WorkspaceRoot: root,
		Node: compiler.PlanNode{
			ID:             "arjun-url-scope:01",
			Capability:     "web.route_enumeration",
			ManifestName:   "web-api-pack",
			ToolID:         "arjun",
			AdapterID:      "managed_binary",
			Operation:      "parameter_discovery",
			ParserID:       "arjun_json",
			TargetKind:     "url",
			DependencyName: "arjun",
			DependencyKind: "binary",
			Runner:         "native",
		},
		ScopeTargets:      []ScopeTarget{{Kind: "url", Value: "https://app.example/search?q=hello"}},
		ScopeTargetPolicy: "enforce",
		IsolationMode:     "process",
	}

	command, target, ok, err := nativeManagedRunCommandWithTarget(request)
	if err != nil || !ok {
		t.Fatalf("expected arjun command for URL scope, command=%#v target=%#v ok=%v err=%v", command, target, ok, err)
	}
	if target.Kind != "url" || target.Value != "https://app.example/search?q=hello" {
		t.Fatalf("expected selected URL target, got %#v", target)
	}
	if len(command) != 8 {
		t.Fatalf("unexpected arjun argv length: %#v", command)
	}
	if command[0] != payloadPath || command[1] != "-u" || command[2] != "https://app.example/search?q=hello" || command[3] != "-oJ" || command[5] != "-q" || command[6] != "-t" || command[7] != "1" {
		t.Fatalf("unexpected arjun argv: %#v", command)
	}
	wantOutput := filepath.Join(root, "toolchain", "managed-outputs", "arjun", "arjun-url-scope-01.json")
	if command[4] != wantOutput {
		t.Fatalf("unexpected arjun managed output path: got %q want %q", command[4], wantOutput)
	}
}

func TestNativeManagedRunCommandBuildsSqlmapInjectionTestArgvForURLScope(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	payloadPath := writeManagedBundleFixture(t, root, "sqlmap", "binary", "1.8.12", "#!/usr/bin/env sh\nprintf 'sqlmap-url\\n'\n")
	if _, err := ExecuteManagedToolBundleInstall(root, "web-api-pack", "sqlmap", ManagedToolBundleInstallOptions{Execute: true}); err != nil {
		t.Fatalf("install managed sqlmap bundle: %v", err)
	}
	request := ToolExecutionRequest{
		WorkspaceRoot: root,
		Node: compiler.PlanNode{
			ID:             "sqlmap-url-scope:01",
			Capability:     "web.candidate_generation",
			ManifestName:   "web-api-pack",
			ToolID:         "sqlmap",
			AdapterID:      "managed_binary",
			Operation:      "injection_test",
			ParserID:       "sqlmap_json",
			TargetKind:     "url",
			DependencyName: "sqlmap",
			DependencyKind: "binary",
			Runner:         "native",
		},
		ScopeTargets:      []ScopeTarget{{Kind: "url", Value: "https://app.example/search?q=hello"}},
		ScopeTargetPolicy: "enforce",
		IsolationMode:     "process",
	}

	command, target, ok, err := nativeManagedRunCommandWithTarget(request)
	if err != nil || !ok {
		t.Fatalf("expected sqlmap command for URL scope, command=%#v target=%#v ok=%v err=%v", command, target, ok, err)
	}
	if target.Kind != "url" || target.Value != "https://app.example/search?q=hello" {
		t.Fatalf("expected selected URL target, got %#v", target)
	}
	if len(command) != 11 {
		t.Fatalf("unexpected sqlmap argv length: %#v", command)
	}
	if command[0] != payloadPath || command[1] != "-u" || command[2] != "https://app.example/search?q=hello" || command[3] != "--batch" || command[4] != "--level=1" || command[5] != "--risk=1" || command[6] != "--technique=BEU" || command[7] != "--disable-coloring" || command[8] != "--output-dir" {
		t.Fatalf("unexpected sqlmap argv: %#v", command)
	}
	wantOutputDir := filepath.Join(root, "toolchain", "managed-outputs", "sqlmap", "sqlmap-url-scope-01")
	if command[9] != wantOutputDir || command[10] != "--flush-session" {
		t.Fatalf("unexpected sqlmap managed output/control args: %#v", command)
	}
	for _, forbidden := range []string{"--dump", "--dbs", "--tables", "--columns", "--schema", "--forms", "--crawl", "--os-shell", "--file-read", "--sql-shell", "--threads=2", "--risk=2", "--level=2"} {
		for _, arg := range command {
			if arg == forbidden {
				t.Fatalf("sqlmap argv must not include unsafe option %q: %#v", forbidden, command)
			}
		}
	}
}

func TestNativeManagedRunCommandKeepsSqlmapOfflineForNonURLScope(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	writeManagedBundleFixture(t, root, "sqlmap", "binary", "1.8.12", "#!/usr/bin/env sh\nprintf 'sqlmap-non-url\\n'\n")
	if _, err := ExecuteManagedToolBundleInstall(root, "web-api-pack", "sqlmap", ManagedToolBundleInstallOptions{Execute: true}); err != nil {
		t.Fatalf("install managed sqlmap bundle: %v", err)
	}
	for _, scopeTarget := range []ScopeTarget{
		{Kind: "domain", Value: "example.com"},
		{Kind: "host", Value: "www.example.com"},
		{Kind: "cidr", Value: "10.0.0.0/24"},
	} {
		t.Run(scopeTarget.Kind, func(t *testing.T) {
			request := ToolExecutionRequest{
				WorkspaceRoot: root,
				Node: compiler.PlanNode{
					ID:             "sqlmap-non-url:01",
					Capability:     "web.candidate_generation",
					ManifestName:   "web-api-pack",
					ToolID:         "sqlmap",
					AdapterID:      "managed_binary",
					Operation:      "injection_test",
					ParserID:       "sqlmap_json",
					TargetKind:     "url",
					DependencyName: "sqlmap",
					DependencyKind: "binary",
					Runner:         "native",
				},
				ScopeTargets:      []ScopeTarget{scopeTarget},
				ScopeTargetPolicy: "enforce",
				IsolationMode:     "process",
			}

			command, ok, err := nativeManagedRunCommand(request)
			if err != nil || ok || len(command) > 0 {
				t.Fatalf("sqlmap must stay offline for non-URL scope %#v, command=%#v ok=%v err=%v", scopeTarget, command, ok, err)
			}
		})
	}
}

func TestNativeManagedRunCommandRejectsExcludedSqlmapURLBeforeOutputDir(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	writeManagedBundleFixture(t, root, "sqlmap", "binary", "1.8.12", "#!/usr/bin/env sh\nprintf 'sqlmap-excluded\\n'")
	if _, err := ExecuteManagedToolBundleInstall(root, "web-api-pack", "sqlmap", ManagedToolBundleInstallOptions{Execute: true}); err != nil {
		t.Fatalf("install managed sqlmap bundle: %v", err)
	}
	request := ToolExecutionRequest{
		WorkspaceRoot: root,
		Node: compiler.PlanNode{
			ID:             "sqlmap-excluded-url:01",
			Capability:     "web.candidate_generation",
			ManifestName:   "web-api-pack",
			ToolID:         "sqlmap",
			AdapterID:      "managed_binary",
			Operation:      "injection_test",
			ParserID:       "sqlmap_json",
			TargetKind:     "url",
			DependencyName: "sqlmap",
			DependencyKind: "binary",
			Runner:         "native",
		},
		ScopeTargets:      []ScopeTarget{{Kind: "url", Value: "https://app.example/search?q=hello"}},
		ScopeExclusions:   []string{"https://app.example/search"},
		ScopeTargetPolicy: "enforce",
		IsolationMode:     "process",
	}

	command, ok, err := nativeManagedRunCommand(request)
	if err == nil || !strings.Contains(err.Error(), "excluded") || !ok || len(command) > 0 {
		t.Fatalf("expected excluded sqlmap URL to fail before argv, command=%#v ok=%v err=%v", command, ok, err)
	}
	outputDir := filepath.Join(root, "toolchain", "managed-outputs", "sqlmap")
	if _, statErr := os.Stat(outputDir); !os.IsNotExist(statErr) {
		t.Fatalf("excluded sqlmap URL should fail before creating output dir, stat err=%v", statErr)
	}
}

func TestManagedSqlmapOutputTranscriptRejectsSymlinkedLog(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	request := ToolExecutionRequest{
		WorkspaceRoot: root,
		Node: compiler.PlanNode{
			ID:             "sqlmap-symlink-log:01",
			Capability:     "web.candidate_generation",
			ManifestName:   "web-api-pack",
			ToolID:         "sqlmap",
			AdapterID:      "managed_binary",
			Operation:      "injection_test",
			ParserID:       "sqlmap_json",
			TargetKind:     "url",
			DependencyName: "sqlmap",
			DependencyKind: "binary",
			Runner:         "native",
		},
		ScopeTargets:      []ScopeTarget{{Kind: "url", Value: "https://app.example/search?q=hello"}},
		ScopeTargetPolicy: "enforce",
		IsolationMode:     "process",
	}
	outputDir, err := managedOutputDirForRequest(request, "sqlmap")
	if err != nil {
		t.Fatalf("create managed sqlmap output dir: %v", err)
	}
	targetDir := filepath.Join(outputDir, "app.example")
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		t.Fatalf("create sqlmap target dir: %v", err)
	}
	outside := filepath.Join(t.TempDir(), "outside-log")
	if err := os.WriteFile(outside, []byte("Parameter: q (GET)\n"), 0o644); err != nil {
		t.Fatalf("write outside log: %v", err)
	}
	if err := os.Symlink(outside, filepath.Join(targetDir, "log")); err != nil {
		t.Fatalf("symlink log: %v", err)
	}

	transcript, ok, err := managedOutputTranscriptForRequest(request)
	if err == nil || !strings.Contains(err.Error(), "symlink") || !ok || transcript != "" {
		t.Fatalf("expected symlinked sqlmap log to fail closed, transcript=%q ok=%v err=%v", transcript, ok, err)
	}
}

func TestNativeManagedRunCommandBuildsDalfoxXSSScanArgvForURLScope(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	payloadPath := writeManagedBundleFixture(t, root, "dalfox", "binary", "2.12.0", "#!/usr/bin/env sh\nprintf 'dalfox-url\\n'\n")
	if _, err := ExecuteManagedToolBundleInstall(root, "web-api-pack", "dalfox", ManagedToolBundleInstallOptions{Execute: true}); err != nil {
		t.Fatalf("install managed dalfox bundle: %v", err)
	}
	request := ToolExecutionRequest{
		WorkspaceRoot: root,
		Node: compiler.PlanNode{
			ID:             "dalfox-url-scope:01",
			Capability:     "web.candidate_generation",
			ManifestName:   "web-api-pack",
			ToolID:         "dalfox",
			AdapterID:      "managed_binary",
			Operation:      "xss_scan",
			ParserID:       "dalfox_json",
			TargetKind:     "url",
			DependencyName: "dalfox",
			DependencyKind: "binary",
			Runner:         "native",
		},
		ScopeTargets:      []ScopeTarget{{Kind: "url", Value: "https://app.example/search?q=hello"}},
		ScopeTargetPolicy: "enforce",
		IsolationMode:     "process",
	}

	command, target, ok, err := nativeManagedRunCommandWithTarget(request)
	if err != nil || !ok {
		t.Fatalf("expected dalfox command for URL scope, command=%#v target=%#v ok=%v err=%v", command, target, ok, err)
	}
	if target.Kind != "url" || target.Value != "https://app.example/search?q=hello" {
		t.Fatalf("expected selected URL target, got %#v", target)
	}
	want := []string{payloadPath, "url", "https://app.example/search?q=hello", "--format", "json", "--silence", "--no-color", "--no-spinner"}
	if strings.Join(command, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("unexpected dalfox argv: got %#v want %#v", command, want)
	}
}

func TestNativeManagedRunCommandBuildsArchiveDiscoveryArgvForDomainScope(t *testing.T) {
	for _, tt := range []struct {
		toolID  string
		version string
		want    []string
	}{
		{toolID: "gau", version: "2.2.4", want: []string{"example.com", "--subs", "--providers", "wayback,otx,commoncrawl"}},
		{toolID: "waybackurls", version: "0.1.0", want: []string{"example.com"}},
	} {
		t.Run(tt.toolID, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "workspace")
			if err := InitWorkspace(root); err != nil {
				t.Fatalf("init workspace: %v", err)
			}
			payloadPath := writeManagedBundleFixture(t, root, tt.toolID, "binary", tt.version, "#!/usr/bin/env sh\nprintf 'archive-url-list\\n'\n")
			if _, err := ExecuteManagedToolBundleInstall(root, "web-api-pack", tt.toolID, ManagedToolBundleInstallOptions{Execute: true}); err != nil {
				t.Fatalf("install managed %s bundle: %v", tt.toolID, err)
			}
			request := ToolExecutionRequest{
				WorkspaceRoot: root,
				Node: compiler.PlanNode{
					ID:             tt.toolID + "-archive-domain:01",
					Capability:     "web.route_enumeration",
					ManifestName:   "web-api-pack",
					ToolID:         tt.toolID,
					AdapterID:      "managed_binary",
					Operation:      "archive_discovery",
					ParserID:       "url_list_text",
					TargetKind:     "domain",
					DependencyName: tt.toolID,
					DependencyKind: "binary",
					Runner:         "native",
				},
				ScopeTargets:      []ScopeTarget{{Kind: "domain", Value: "example.com"}},
				ScopeTargetPolicy: "enforce",
				IsolationMode:     "process",
			}

			command, target, ok, err := nativeManagedRunCommandWithTarget(request)
			if err != nil || !ok {
				t.Fatalf("expected %s archive discovery command for domain scope, command=%#v target=%#v ok=%v err=%v", tt.toolID, command, target, ok, err)
			}
			if target.Kind != "domain" || target.Value != "example.com" {
				t.Fatalf("expected selected domain target, got %#v", target)
			}
			want := append([]string{payloadPath}, tt.want...)
			if strings.Join(command, "\x00") != strings.Join(want, "\x00") {
				t.Fatalf("unexpected %s argv: got %#v want %#v", tt.toolID, command, want)
			}
		})
	}
}

func TestNativeManagedRunCommandKeepsArchiveDiscoveryOfflineForNonDomainScope(t *testing.T) {
	for _, tt := range []struct {
		toolID  string
		version string
	}{
		{toolID: "gau", version: "2.2.4"},
		{toolID: "waybackurls", version: "0.1.0"},
	} {
		t.Run(tt.toolID, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "workspace")
			if err := InitWorkspace(root); err != nil {
				t.Fatalf("init workspace: %v", err)
			}
			writeManagedBundleFixture(t, root, tt.toolID, "binary", tt.version, "#!/usr/bin/env sh\nprintf 'archive-non-domain\\n'\n")
			if _, err := ExecuteManagedToolBundleInstall(root, "web-api-pack", tt.toolID, ManagedToolBundleInstallOptions{Execute: true}); err != nil {
				t.Fatalf("install managed %s bundle: %v", tt.toolID, err)
			}
			for _, scopeTarget := range []ScopeTarget{
				{Kind: "url", Value: "https://example.com"},
				{Kind: "host", Value: "www.example.com"},
				{Kind: "cidr", Value: "10.0.0.0/24"},
			} {
				t.Run(scopeTarget.Kind, func(t *testing.T) {
					request := ToolExecutionRequest{
						WorkspaceRoot: root,
						Node: compiler.PlanNode{
							ID:             tt.toolID + "-archive-non-domain:01",
							Capability:     "web.route_enumeration",
							ManifestName:   "web-api-pack",
							ToolID:         tt.toolID,
							AdapterID:      "managed_binary",
							Operation:      "archive_discovery",
							ParserID:       "url_list_text",
							TargetKind:     "domain",
							DependencyName: tt.toolID,
							DependencyKind: "binary",
							Runner:         "native",
						},
						ScopeTargets:      []ScopeTarget{scopeTarget},
						ScopeTargetPolicy: "enforce",
						IsolationMode:     "process",
					}

					command, ok, err := nativeManagedRunCommand(request)
					if err != nil || ok || len(command) > 0 {
						t.Fatalf("%s must stay offline for non-domain scope %#v, command=%#v ok=%v err=%v", tt.toolID, scopeTarget, command, ok, err)
					}
				})
			}
		})
	}
}

func TestNativeManagedRunCommandRejectsExcludedArchiveDiscoveryDomain(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	writeManagedBundleFixture(t, root, "gau", "binary", "2.2.4", "#!/usr/bin/env sh\nprintf 'archive-excluded\\n'")
	if _, err := ExecuteManagedToolBundleInstall(root, "web-api-pack", "gau", ManagedToolBundleInstallOptions{Execute: true}); err != nil {
		t.Fatalf("install managed gau bundle: %v", err)
	}
	request := ToolExecutionRequest{
		WorkspaceRoot: root,
		Node: compiler.PlanNode{
			ID:             "gau-excluded-domain:01",
			Capability:     "web.route_enumeration",
			ManifestName:   "web-api-pack",
			ToolID:         "gau",
			AdapterID:      "managed_binary",
			Operation:      "archive_discovery",
			ParserID:       "url_list_text",
			TargetKind:     "domain",
			DependencyName: "gau",
			DependencyKind: "binary",
			Runner:         "native",
		},
		ScopeTargets:      []ScopeTarget{{Kind: "domain", Value: "example.com"}},
		ScopeExclusions:   []string{"example.com"},
		ScopeTargetPolicy: "enforce",
		IsolationMode:     "process",
	}

	command, ok, err := nativeManagedRunCommand(request)
	if err == nil || !strings.Contains(err.Error(), "excluded") || !ok || len(command) > 0 {
		t.Fatalf("expected excluded archive discovery domain to fail before argv, command=%#v ok=%v err=%v", command, ok, err)
	}
}

func TestNativeManagedRunCommandKeepsArjunOfflineForNonURLScope(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	writeManagedBundleFixture(t, root, "arjun", "binary", "2.2.7", "#!/usr/bin/env sh\nprintf 'arjun-non-url\\n'\n")
	if _, err := ExecuteManagedToolBundleInstall(root, "web-api-pack", "arjun", ManagedToolBundleInstallOptions{Execute: true}); err != nil {
		t.Fatalf("install managed arjun bundle: %v", err)
	}
	for _, scopeTarget := range []ScopeTarget{
		{Kind: "domain", Value: "example.com"},
		{Kind: "host", Value: "app.example.com"},
		{Kind: "cidr", Value: "10.0.0.0/24"},
	} {
		t.Run(scopeTarget.Kind, func(t *testing.T) {
			request := ToolExecutionRequest{
				WorkspaceRoot: root,
				Node: compiler.PlanNode{
					ID:             "arjun-non-url-scope:01",
					Capability:     "web.route_enumeration",
					ManifestName:   "web-api-pack",
					ToolID:         "arjun",
					AdapterID:      "managed_binary",
					Operation:      "parameter_discovery",
					ParserID:       "arjun_json",
					TargetKind:     "url",
					DependencyName: "arjun",
					DependencyKind: "binary",
					Runner:         "native",
				},
				ScopeTargets:      []ScopeTarget{scopeTarget},
				ScopeTargetPolicy: "enforce",
				IsolationMode:     "process",
			}

			command, ok, err := nativeManagedRunCommand(request)
			if err != nil || ok || len(command) > 0 {
				t.Fatalf("arjun must stay offline for non-URL scope %#v, command=%#v ok=%v err=%v", scopeTarget, command, ok, err)
			}
		})
	}
}

func TestNativeManagedRunCommandRejectsExcludedArjunURL(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	writeManagedBundleFixture(t, root, "arjun", "binary", "2.2.7", "#!/usr/bin/env sh\nprintf 'arjun-excluded\\n'")
	if _, err := ExecuteManagedToolBundleInstall(root, "web-api-pack", "arjun", ManagedToolBundleInstallOptions{Execute: true}); err != nil {
		t.Fatalf("install managed arjun bundle: %v", err)
	}
	request := ToolExecutionRequest{
		WorkspaceRoot: root,
		Node: compiler.PlanNode{
			ID:             "arjun-excluded-url:01",
			Capability:     "web.route_enumeration",
			ManifestName:   "web-api-pack",
			ToolID:         "arjun",
			AdapterID:      "managed_binary",
			Operation:      "parameter_discovery",
			ParserID:       "arjun_json",
			TargetKind:     "url",
			DependencyName: "arjun",
			DependencyKind: "binary",
			Runner:         "native",
		},
		ScopeTargets:      []ScopeTarget{{Kind: "url", Value: "https://app.example/search?q=hello"}},
		ScopeExclusions:   []string{"https://app.example/search"},
		ScopeTargetPolicy: "enforce",
		IsolationMode:     "process",
	}

	command, ok, err := nativeManagedRunCommand(request)
	if err == nil || !strings.Contains(err.Error(), "excluded") || !ok || len(command) > 0 {
		t.Fatalf("expected excluded arjun URL to fail before argv, command=%#v ok=%v err=%v", command, ok, err)
	}
	outputDir := filepath.Join(root, "toolchain", "managed-outputs", "arjun")
	if _, statErr := os.Stat(outputDir); statErr == nil || !os.IsNotExist(statErr) {
		t.Fatalf("excluded arjun URL must not create managed output directory, statErr=%v", statErr)
	}
}

func TestNativeManagedRunCommandKeepsDalfoxOfflineForNonURLScope(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	writeManagedBundleFixture(t, root, "dalfox", "binary", "2.12.0", "#!/usr/bin/env sh\nprintf 'dalfox-non-url\\n'\n")
	if _, err := ExecuteManagedToolBundleInstall(root, "web-api-pack", "dalfox", ManagedToolBundleInstallOptions{Execute: true}); err != nil {
		t.Fatalf("install managed dalfox bundle: %v", err)
	}
	for _, scopeTarget := range []ScopeTarget{
		{Kind: "domain", Value: "example.com"},
		{Kind: "host", Value: "app.example.com"},
		{Kind: "cidr", Value: "10.0.0.0/24"},
	} {
		t.Run(scopeTarget.Kind, func(t *testing.T) {
			request := ToolExecutionRequest{
				WorkspaceRoot: root,
				Node: compiler.PlanNode{
					ID:             "dalfox-non-url-scope:01",
					Capability:     "web.candidate_generation",
					ManifestName:   "web-api-pack",
					ToolID:         "dalfox",
					AdapterID:      "managed_binary",
					Operation:      "xss_scan",
					ParserID:       "dalfox_json",
					TargetKind:     "url",
					DependencyName: "dalfox",
					DependencyKind: "binary",
					Runner:         "native",
				},
				ScopeTargets:      []ScopeTarget{scopeTarget},
				ScopeTargetPolicy: "enforce",
				IsolationMode:     "process",
			}

			command, ok, err := nativeManagedRunCommand(request)
			if err != nil || ok || len(command) > 0 {
				t.Fatalf("dalfox must stay offline for non-URL scope %#v, command=%#v ok=%v err=%v", scopeTarget, command, ok, err)
			}
		})
	}
}

func TestNativeManagedRunCommandRejectsExcludedDalfoxURL(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	writeManagedBundleFixture(t, root, "dalfox", "binary", "2.12.0", "#!/usr/bin/env sh\nprintf 'dalfox-excluded\\n'")
	if _, err := ExecuteManagedToolBundleInstall(root, "web-api-pack", "dalfox", ManagedToolBundleInstallOptions{Execute: true}); err != nil {
		t.Fatalf("install managed dalfox bundle: %v", err)
	}
	request := ToolExecutionRequest{
		WorkspaceRoot: root,
		Node: compiler.PlanNode{
			ID:             "dalfox-excluded-url:01",
			Capability:     "web.candidate_generation",
			ManifestName:   "web-api-pack",
			ToolID:         "dalfox",
			AdapterID:      "managed_binary",
			Operation:      "xss_scan",
			ParserID:       "dalfox_json",
			TargetKind:     "url",
			DependencyName: "dalfox",
			DependencyKind: "binary",
			Runner:         "native",
		},
		ScopeTargets:      []ScopeTarget{{Kind: "url", Value: "https://app.example/search?q=hello"}},
		ScopeExclusions:   []string{"https://app.example/search"},
		ScopeTargetPolicy: "enforce",
		IsolationMode:     "process",
	}

	command, ok, err := nativeManagedRunCommand(request)
	if err == nil || !strings.Contains(err.Error(), "excluded") || !ok || len(command) > 0 {
		t.Fatalf("expected excluded dalfox URL to fail before argv, command=%#v ok=%v err=%v", command, ok, err)
	}
}

func TestNativeManagedRunCommandBuildsFfufPathFuzzArgvForURLScope(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	payloadPath := writeManagedBundleFixture(t, root, "ffuf", "binary", "2.1.0", "#!/usr/bin/env sh\nprintf 'ffuf-url\\n'\n")
	if _, err := ExecuteManagedToolBundleInstall(root, "web-api-pack", "ffuf", ManagedToolBundleInstallOptions{Execute: true}); err != nil {
		t.Fatalf("install managed ffuf bundle: %v", err)
	}
	request := ToolExecutionRequest{
		WorkspaceRoot: root,
		Node: compiler.PlanNode{
			ID:             "ffuf-url-scope:01",
			Capability:     "web.route_enumeration",
			ManifestName:   "web-api-pack",
			ToolID:         "ffuf",
			AdapterID:      "managed_binary",
			Operation:      "path_fuzz",
			ParserID:       "ffuf_json",
			TargetKind:     "url",
			DependencyName: "ffuf",
			DependencyKind: "binary",
			Runner:         "native",
		},
		ScopeTargets:      []ScopeTarget{{Kind: "url", Value: "https://app.example/app"}},
		ScopeTargetPolicy: "enforce",
		IsolationMode:     "process",
	}

	command, target, ok, err := nativeManagedRunCommandWithTarget(request)
	if err != nil || !ok {
		t.Fatalf("expected ffuf command for URL scope, command=%#v target=%#v ok=%v err=%v", command, target, ok, err)
	}
	if target.Kind != "url" || target.Value != "https://app.example/app" {
		t.Fatalf("expected selected URL target, got %#v", target)
	}
	if len(command) != 7 {
		t.Fatalf("unexpected ffuf argv length: %#v", command)
	}
	inputPath := command[2]
	wantPrefix := filepath.Join(root, "toolchain", "managed-inputs", "ffuf") + string(os.PathSeparator)
	if !strings.HasPrefix(inputPath, wantPrefix) {
		t.Fatalf("expected ffuf wordlist under managed inputs, got %q prefix %q", inputPath, wantPrefix)
	}
	wordlist, err := os.ReadFile(inputPath)
	if err != nil {
		t.Fatalf("read ffuf managed wordlist: %v", err)
	}
	wordlistText := string(wordlist)
	for _, want := range []string{"admin\n", "login\n", "api\n"} {
		if !strings.Contains(wordlistText, want) {
			t.Fatalf("expected managed ffuf wordlist to contain %q, got %q", want, wordlistText)
		}
	}
	want := []string{payloadPath, "-w", inputPath, "-u", "https://app.example/app/FUZZ", "-json", "-s"}
	if strings.Join(command, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("unexpected ffuf argv: got %#v want %#v", command, want)
	}
}

func TestNativeManagedRunCommandKeepsFfufOfflineForNonURLScope(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	writeManagedBundleFixture(t, root, "ffuf", "binary", "2.1.0", "#!/usr/bin/env sh\nprintf 'ffuf-non-url\\n'\n")
	if _, err := ExecuteManagedToolBundleInstall(root, "web-api-pack", "ffuf", ManagedToolBundleInstallOptions{Execute: true}); err != nil {
		t.Fatalf("install managed ffuf bundle: %v", err)
	}
	for _, scopeTarget := range []ScopeTarget{
		{Kind: "domain", Value: "example.com"},
		{Kind: "host", Value: "app.example.com"},
		{Kind: "cidr", Value: "10.0.0.0/24"},
	} {
		t.Run(scopeTarget.Kind, func(t *testing.T) {
			request := ToolExecutionRequest{
				WorkspaceRoot: root,
				Node: compiler.PlanNode{
					ID:             "ffuf-non-url-scope:01",
					Capability:     "web.route_enumeration",
					ManifestName:   "web-api-pack",
					ToolID:         "ffuf",
					AdapterID:      "managed_binary",
					Operation:      "path_fuzz",
					ParserID:       "ffuf_json",
					TargetKind:     "url",
					DependencyName: "ffuf",
					DependencyKind: "binary",
					Runner:         "native",
				},
				ScopeTargets:      []ScopeTarget{scopeTarget},
				ScopeTargetPolicy: "enforce",
				IsolationMode:     "process",
			}

			command, ok, err := nativeManagedRunCommand(request)
			if err != nil || ok || len(command) > 0 {
				t.Fatalf("ffuf must stay offline for non-URL scope %#v, command=%#v ok=%v err=%v", scopeTarget, command, ok, err)
			}
		})
	}
}

func TestNativeManagedRunCommandRejectsExcludedFfufURLBeforeWritingWordlist(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	writeManagedBundleFixture(t, root, "ffuf", "binary", "2.1.0", "#!/usr/bin/env sh\nprintf 'ffuf-excluded\\n'")
	if _, err := ExecuteManagedToolBundleInstall(root, "web-api-pack", "ffuf", ManagedToolBundleInstallOptions{Execute: true}); err != nil {
		t.Fatalf("install managed ffuf bundle: %v", err)
	}
	request := ToolExecutionRequest{
		WorkspaceRoot: root,
		Node: compiler.PlanNode{
			ID:             "ffuf-excluded-url:01",
			Capability:     "web.route_enumeration",
			ManifestName:   "web-api-pack",
			ToolID:         "ffuf",
			AdapterID:      "managed_binary",
			Operation:      "path_fuzz",
			ParserID:       "ffuf_json",
			TargetKind:     "url",
			DependencyName: "ffuf",
			DependencyKind: "binary",
			Runner:         "native",
		},
		ScopeTargets:      []ScopeTarget{{Kind: "url", Value: "https://app.example/app"}},
		ScopeExclusions:   []string{"https://app.example/app"},
		ScopeTargetPolicy: "enforce",
		IsolationMode:     "process",
	}

	command, ok, err := nativeManagedRunCommand(request)
	if err == nil || !strings.Contains(err.Error(), "excluded") || !ok || len(command) > 0 {
		t.Fatalf("expected excluded ffuf URL to fail before argv, command=%#v ok=%v err=%v", command, ok, err)
	}
	assertNoWorkspaceFileNamed(t, root, "ffuf-wordlist.txt")
}

func TestNativeManagedRunCommandRejectsExcludedFfufGeneratedURLBeforeWritingWordlist(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	writeManagedBundleFixture(t, root, "ffuf", "binary", "2.1.0", "#!/usr/bin/env sh\nprintf 'ffuf-excluded-generated\\n'")
	if _, err := ExecuteManagedToolBundleInstall(root, "web-api-pack", "ffuf", ManagedToolBundleInstallOptions{Execute: true}); err != nil {
		t.Fatalf("install managed ffuf bundle: %v", err)
	}
	request := ToolExecutionRequest{
		WorkspaceRoot: root,
		Node: compiler.PlanNode{
			ID:             "ffuf-excluded-generated-url:01",
			Capability:     "web.route_enumeration",
			ManifestName:   "web-api-pack",
			ToolID:         "ffuf",
			AdapterID:      "managed_binary",
			Operation:      "path_fuzz",
			ParserID:       "ffuf_json",
			TargetKind:     "url",
			DependencyName: "ffuf",
			DependencyKind: "binary",
			Runner:         "native",
		},
		ScopeTargets:      []ScopeTarget{{Kind: "url", Value: "https://app.example"}},
		ScopeExclusions:   []string{"https://app.example/admin"},
		ScopeTargetPolicy: "enforce",
		IsolationMode:     "process",
	}

	command, ok, err := nativeManagedRunCommand(request)
	if err == nil || !strings.Contains(err.Error(), "excluded") || !ok || len(command) > 0 {
		t.Fatalf("expected generated excluded ffuf URL to fail before argv, command=%#v ok=%v err=%v", command, ok, err)
	}
	assertNoWorkspaceFileNamed(t, root, "ffuf-wordlist.txt")
}

func TestRunWorkspacePromotesKatanaJSONLToEvidenceSurface(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	scope := `version: 1
scope:
  target_policy: enforce
  targets:
    - kind: url
      value: https://app.example
  exclusions: []
execution:
  isolation: process
  tool_concurrency: 1
`
	if err := os.WriteFile(filepath.Join(root, "scope.yaml"), []byte(scope), 0o644); err != nil {
		t.Fatalf("write URL scope: %v", err)
	}
	marker := filepath.Join(t.TempDir(), "tool-level-katana-ran")
	writeManagedBundleFixture(t, root, "katana", "binary", "1.6.1", `#!/usr/bin/env sh
cat <<'JSONL'
{"url":"https://app.example/login?token=secret-token#frag","method":"GET","status_code":200,"content_length":6726,"depth":1,"raw":"secret-token","response":"secret-token","headers":{"Authorization":"Bearer secret-token"}}
JSONL
touch `+marker+`
`)
	if _, err := ExecuteManagedToolBundleInstall(root, "web-api-pack", "katana", ManagedToolBundleInstallOptions{Execute: true}); err != nil {
		t.Fatalf("install managed katana bundle: %v", err)
	}

	result, err := RunWorkspaceWithOptions(ctx, root, RunOptions{Action: compiler.ActionEnumeratePaths, DeferReportProjection: true})
	if err != nil {
		t.Fatalf("run workspace with katana parser: %v", err)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("expected katana payload to run through registered argv builder: %v", err)
	}
	store, err := eventlog.OpenSQLite(filepath.Join(root, "replay", "events.db"))
	if err != nil {
		t.Fatalf("open event log: %v", err)
	}
	defer store.Close()
	events, err := store.Replay(ctx, result.RunID)
	if err != nil {
		t.Fatalf("replay events: %v", err)
	}
	snapshot, err := eventlog.BuildReplaySnapshot(events)
	if err != nil {
		t.Fatalf("build replay snapshot: %v", err)
	}

	var surfaceID string
	for _, fact := range snapshot.Blackboard.Facts {
		if fact.Source != "parser.katana_jsonl" {
			continue
		}
		surfaceID = fact.ID
		if fact.Kind != "surface" {
			t.Fatalf("expected katana parser fact kind surface, got %#v", fact)
		}
		for _, want := range []string{"katana", "https://app.example/login", "method=GET", "status=200"} {
			if !strings.Contains(fact.Statement, want) {
				t.Fatalf("expected katana route surface to contain %q, got %#v", want, fact)
			}
		}
		if strings.Contains(fact.Statement, "secret-token") || strings.Contains(fact.Statement, "Authorization") {
			t.Fatalf("katana route statement leaked redacted output: %#v", fact)
		}
		if fact.Confidence < 0.6 {
			t.Fatalf("expected completed katana route confidence, got %#v", fact)
		}
		break
	}
	if surfaceID == "" {
		t.Fatalf("expected katana parser route fact in replay snapshot, facts=%#v", snapshot.Blackboard.Facts)
	}
	node, ok := snapshot.Evidence.Node(surfaceID)
	if !ok {
		t.Fatalf("expected katana parser fact to project into evidence graph")
	}
	if node.Type != evidence.NodeSurface {
		t.Fatalf("expected evidence surface node, got %#v", node)
	}

	foundFactEvent := false
	foundParserEvent := false
	for _, event := range events {
		switch event.Type {
		case eventlog.EventFact:
			var payload struct {
				FactID        string `json:"fact_id"`
				Source        string `json:"source"`
				URL           string `json:"url"`
				TargetURL     string `json:"target_url"`
				Method        string `json:"method"`
				StatusCode    int    `json:"status_code"`
				ContentLength int64  `json:"content_length"`
				Depth         int    `json:"depth"`
				ParserID      string `json:"parser_id"`
			}
			if err := json.Unmarshal(event.Payload, &payload); err != nil {
				t.Fatalf("decode fact event: %v", err)
			}
			if payload.FactID != surfaceID || payload.Source != "parser.katana_jsonl" {
				continue
			}
			foundFactEvent = true
			if payload.URL != "https://app.example/login" || payload.TargetURL != "https://app.example" || payload.Method != "GET" || payload.StatusCode != 200 || payload.ContentLength != 6726 || payload.Depth != 1 || payload.ParserID != "katana_jsonl" {
				t.Fatalf("expected structured katana route metadata, got %#v", payload)
			}
			if strings.Contains(string(event.Payload), "secret-token") || strings.Contains(string(event.Payload), "Authorization") {
				t.Fatalf("katana route fact payload leaked redacted output: %s", string(event.Payload))
			}
		case eventlog.EventParser:
			var payload struct {
				Parser string `json:"parser"`
				Status string `json:"status"`
				ToolID string `json:"tool_id"`
			}
			if err := json.Unmarshal(event.Payload, &payload); err != nil {
				t.Fatalf("decode parser event: %v", err)
			}
			if payload.Parser == "katana_jsonl" && payload.Status == "parsed" && payload.ToolID == "katana" {
				foundParserEvent = true
			}
		}
	}
	if !foundFactEvent {
		t.Fatalf("expected raw katana fact event for %s", surfaceID)
	}
	if !foundParserEvent {
		t.Fatalf("expected parsed katana parser event, events=%#v", events)
	}
}

func TestRunWorkspaceRecordsKatanaParseFailureWithoutSurface(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	scope := `version: 1
scope:
  target_policy: enforce
  targets:
    - kind: url
      value: https://app.example
  exclusions: []
execution:
  isolation: process
  tool_concurrency: 1
`
	if err := os.WriteFile(filepath.Join(root, "scope.yaml"), []byte(scope), 0o644); err != nil {
		t.Fatalf("write URL scope: %v", err)
	}
	writeManagedBundleFixture(t, root, "katana", "binary", "1.6.1", "#!/usr/bin/env sh\nprintf 'not-jsonl\\n'\n")
	if _, err := ExecuteManagedToolBundleInstall(root, "web-api-pack", "katana", ManagedToolBundleInstallOptions{Execute: true}); err != nil {
		t.Fatalf("install managed katana bundle: %v", err)
	}

	result, err := RunWorkspaceWithOptions(ctx, root, RunOptions{Action: compiler.ActionEnumeratePaths, DeferReportProjection: true})
	if err != nil {
		t.Fatalf("parse failure should not fail completed katana run: %v", err)
	}
	store, err := eventlog.OpenSQLite(filepath.Join(root, "replay", "events.db"))
	if err != nil {
		t.Fatalf("open event log: %v", err)
	}
	defer store.Close()
	events, err := store.Replay(ctx, result.RunID)
	if err != nil {
		t.Fatalf("replay events: %v", err)
	}

	foundParseFailure := false
	for _, event := range events {
		switch event.Type {
		case eventlog.EventParser:
			var payload struct {
				Parser       string `json:"parser"`
				Status       string `json:"status"`
				ToolID       string `json:"tool_id"`
				FindingCount int    `json:"finding_count"`
			}
			if err := json.Unmarshal(event.Payload, &payload); err != nil {
				t.Fatalf("decode parser event: %v", err)
			}
			if payload.Parser == "katana_jsonl" && payload.Status == "parse_failed" && payload.ToolID == "katana" && payload.FindingCount == 0 {
				foundParseFailure = true
			}
		case eventlog.EventFact:
			var payload struct {
				Source string `json:"source"`
			}
			if err := json.Unmarshal(event.Payload, &payload); err != nil {
				t.Fatalf("decode fact event: %v", err)
			}
			if payload.Source == "parser.katana_jsonl" {
				t.Fatalf("parse failure must not emit katana parser fact: %#v", payload)
			}
		}
	}
	if !foundParseFailure {
		t.Fatalf("expected run-level katana parse_failed event, events=%#v", events)
	}
}

func TestNativeManagedRunCommandBuildsKatanaCrawlArgvForURLScope(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	payloadPath := writeManagedBundleFixture(t, root, "katana", "binary", "1.6.1", "#!/usr/bin/env sh\nprintf 'katana-url\\n'\n")
	if _, err := ExecuteManagedToolBundleInstall(root, "web-api-pack", "katana", ManagedToolBundleInstallOptions{Execute: true}); err != nil {
		t.Fatalf("install managed katana bundle: %v", err)
	}
	request := ToolExecutionRequest{
		WorkspaceRoot: root,
		Node: compiler.PlanNode{
			ID:             "katana-url-scope:01",
			Capability:     "web.route_enumeration",
			ManifestName:   "web-api-pack",
			ToolID:         "katana",
			AdapterID:      "managed_binary",
			Operation:      "crawl",
			ParserID:       "katana_jsonl",
			TargetKind:     "url",
			DependencyName: "katana",
			DependencyKind: "binary",
			Runner:         "native",
		},
		ScopeTargets:      []ScopeTarget{{Kind: "url", Value: "https://app.example"}},
		ScopeTargetPolicy: "enforce",
		IsolationMode:     "process",
	}

	command, target, ok, err := nativeManagedRunCommandWithTarget(request)
	if err != nil || !ok {
		t.Fatalf("expected katana command for URL scope, command=%#v target=%#v ok=%v err=%v", command, target, ok, err)
	}
	if target.Kind != "url" || target.Value != "https://app.example" {
		t.Fatalf("expected selected URL target, got %#v", target)
	}
	want := []string{payloadPath, "-u", "https://app.example", "-j", "-silent", "-or", "-ob", "-duc"}
	if strings.Join(command, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("unexpected katana argv: got %#v want %#v", command, want)
	}
}

func TestNativeManagedRunCommandKeepsKatanaOfflineForNonURLScope(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	writeManagedBundleFixture(t, root, "katana", "binary", "1.6.1", "#!/usr/bin/env sh\nprintf 'katana-non-url\\n'\n")
	if _, err := ExecuteManagedToolBundleInstall(root, "web-api-pack", "katana", ManagedToolBundleInstallOptions{Execute: true}); err != nil {
		t.Fatalf("install managed katana bundle: %v", err)
	}
	for _, scopeTarget := range []ScopeTarget{
		{Kind: "domain", Value: "example.com"},
		{Kind: "host", Value: "app.example.com"},
		{Kind: "cidr", Value: "10.0.0.0/24"},
	} {
		t.Run(scopeTarget.Kind, func(t *testing.T) {
			request := ToolExecutionRequest{
				WorkspaceRoot: root,
				Node: compiler.PlanNode{
					ID:             "katana-non-url-scope:01",
					Capability:     "web.route_enumeration",
					ManifestName:   "web-api-pack",
					ToolID:         "katana",
					AdapterID:      "managed_binary",
					Operation:      "crawl",
					ParserID:       "katana_jsonl",
					TargetKind:     "url",
					DependencyName: "katana",
					DependencyKind: "binary",
					Runner:         "native",
				},
				ScopeTargets:      []ScopeTarget{scopeTarget},
				ScopeTargetPolicy: "enforce",
				IsolationMode:     "process",
			}

			command, ok, err := nativeManagedRunCommand(request)
			if err != nil || ok || len(command) > 0 {
				t.Fatalf("katana must stay offline for non-URL scope %#v, command=%#v ok=%v err=%v", scopeTarget, command, ok, err)
			}
		})
	}
}

func TestRunWorkspaceRejectsExcludedDomainBeforeDnsxExecution(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	scope := `version: 1
scope:
  target_policy: enforce
  targets:
    - kind: domain
      value: example.com
  exclusions:
    - example.com
execution:
  isolation: process
`
	if err := os.WriteFile(filepath.Join(root, "scope.yaml"), []byte(scope), 0o644); err != nil {
		t.Fatalf("write scope: %v", err)
	}
	marker := filepath.Join(t.TempDir(), "excluded-domain-dnsx-ran")
	writeManagedBundleFixture(t, root, "dnsx", "binary", "1.2.3", "#!/usr/bin/env sh\nprintf 'should-not-run-excluded-domain-dnsx\\n'\ntouch "+marker+"\n")
	if _, err := ExecuteManagedToolBundleInstall(root, "infra-pack", "dnsx", ManagedToolBundleInstallOptions{Execute: true}); err != nil {
		t.Fatalf("install managed dnsx bundle: %v", err)
	}

	_, err := RunWorkspace(ctx, root)
	if err == nil || !strings.Contains(err.Error(), "excluded") {
		t.Fatalf("expected excluded domain failure before dnsx execution, got %v", err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("excluded domain dnsx payload must not run, marker stat err=%v", err)
	}
	assertNoWorkspaceFileNamed(t, root, "dnsx-targets.txt")
}

func TestNativeManagedRunCommandBuildsDnsxDNSProbeArgvForDomainScope(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	payloadPath := writeManagedBundleFixture(t, root, "dnsx", "binary", "1.2.3", "#!/usr/bin/env sh\nprintf 'dnsx-domain\\n'\n")
	if _, err := ExecuteManagedToolBundleInstall(root, "infra-pack", "dnsx", ManagedToolBundleInstallOptions{Execute: true}); err != nil {
		t.Fatalf("install managed dnsx bundle: %v", err)
	}
	request := ToolExecutionRequest{
		WorkspaceRoot: root,
		Node: compiler.PlanNode{
			ID:             "dnsx-domain-scope:01",
			Capability:     "infra.host_discovery",
			ManifestName:   "infra-pack",
			ToolID:         "dnsx",
			AdapterID:      "managed_binary",
			Operation:      "dns_probe",
			ParserID:       "dnsx_jsonl",
			TargetKind:     "domain",
			DependencyName: "dnsx",
			DependencyKind: "binary",
			Runner:         "native",
		},
		ScopeTargets:      []ScopeTarget{{Kind: "domain", Value: "example.com"}},
		ScopeTargetPolicy: "enforce",
		IsolationMode:     "process",
	}

	command, target, ok, err := nativeManagedRunCommandWithTarget(request)
	if err != nil || !ok {
		t.Fatalf("expected dnsx command for domain scope, command=%#v target=%#v ok=%v err=%v", command, target, ok, err)
	}
	if target.Kind != "domain" || target.Value != "example.com" {
		t.Fatalf("expected selected domain target, got %#v", target)
	}
	if len(command) != 6 || command[0] != payloadPath || command[1] != "-j" || command[2] != "-silent" || command[3] != "-omit-raw" || command[4] != "-l" {
		t.Fatalf("unexpected dnsx argv: %#v", command)
	}
	inputPath := command[5]
	if !strings.HasPrefix(inputPath, root+string(os.PathSeparator)) {
		t.Fatalf("dnsx input file must stay under workspace root, got %q", inputPath)
	}
	content, err := os.ReadFile(inputPath)
	if err != nil {
		t.Fatalf("read dnsx input file: %v", err)
	}
	if string(content) != "example.com\n" {
		t.Fatalf("expected dnsx input file to contain scoped domain, got %q", string(content))
	}
}

func TestNativeManagedRunCommandKeepsDnsxOfflineForNonDomainScope(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	writeManagedBundleFixture(t, root, "dnsx", "binary", "1.2.3", "#!/usr/bin/env sh\nprintf 'dnsx-non-domain\\n'\n")
	if _, err := ExecuteManagedToolBundleInstall(root, "infra-pack", "dnsx", ManagedToolBundleInstallOptions{Execute: true}); err != nil {
		t.Fatalf("install managed dnsx bundle: %v", err)
	}
	for _, scopeTarget := range []ScopeTarget{
		{Kind: "url", Value: "https://app.example.com"},
		{Kind: "host", Value: "api.example.com"},
		{Kind: "cidr", Value: "10.0.0.0/24"},
	} {
		t.Run(scopeTarget.Kind, func(t *testing.T) {
			request := ToolExecutionRequest{
				WorkspaceRoot: root,
				Node: compiler.PlanNode{
					ID:             "dnsx-non-domain-scope:01",
					Capability:     "infra.host_discovery",
					ManifestName:   "infra-pack",
					ToolID:         "dnsx",
					AdapterID:      "managed_binary",
					Operation:      "dns_probe",
					ParserID:       "dnsx_jsonl",
					TargetKind:     "domain",
					DependencyName: "dnsx",
					DependencyKind: "binary",
					Runner:         "native",
				},
				ScopeTargets:      []ScopeTarget{scopeTarget},
				ScopeTargetPolicy: "enforce",
				IsolationMode:     "process",
			}

			command, ok, err := nativeManagedRunCommand(request)
			if err != nil || ok || len(command) > 0 {
				t.Fatalf("dnsx must stay offline for non-domain scope %#v, command=%#v ok=%v err=%v", scopeTarget, command, ok, err)
			}
			assertNoWorkspaceFileNamed(t, root, "dnsx-targets.txt")
		})
	}
}

func TestNativeManagedRunCommandRejectsExcludedDnsxDomainBeforeWritingInput(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	writeManagedBundleFixture(t, root, "dnsx", "binary", "1.2.3", "#!/usr/bin/env sh\nprintf 'dnsx-excluded\\n'\n")
	if _, err := ExecuteManagedToolBundleInstall(root, "infra-pack", "dnsx", ManagedToolBundleInstallOptions{Execute: true}); err != nil {
		t.Fatalf("install managed dnsx bundle: %v", err)
	}
	request := ToolExecutionRequest{
		WorkspaceRoot: root,
		Node: compiler.PlanNode{
			ID:             "dnsx-excluded-domain:01",
			Capability:     "infra.host_discovery",
			ManifestName:   "infra-pack",
			ToolID:         "dnsx",
			AdapterID:      "managed_binary",
			Operation:      "dns_probe",
			ParserID:       "dnsx_jsonl",
			TargetKind:     "domain",
			DependencyName: "dnsx",
			DependencyKind: "binary",
			Runner:         "native",
		},
		ScopeTargets:      []ScopeTarget{{Kind: "domain", Value: "example.com"}},
		ScopeExclusions:   []string{"example.com"},
		ScopeTargetPolicy: "enforce",
		IsolationMode:     "process",
	}

	command, ok, err := nativeManagedRunCommand(request)
	if err == nil || !strings.Contains(err.Error(), "excluded") || !ok || len(command) > 0 {
		t.Fatalf("expected excluded dnsx domain to fail before argv, command=%#v ok=%v err=%v", command, ok, err)
	}
	assertNoWorkspaceFileNamed(t, root, "dnsx-targets.txt")
}

func assertNoWorkspaceFileNamed(t *testing.T, root string, base string) {
	t.Helper()
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() && entry.Name() == base {
			t.Fatalf("did not expect workspace file %s at %s", base, path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk workspace: %v", err)
	}
}

func TestRunWorkspaceRejectsExcludedCIDRBeforeNaabuExecutionWhenHostScopePresent(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	scope := `version: 1
scope:
  target_policy: enforce
  targets:
    - kind: host
      value: api.example.com
    - kind: cidr
      value: 10.0.0.0/24
  exclusions:
    - 10.0.0.0/24
execution:
  isolation: process
`
	if err := os.WriteFile(filepath.Join(root, "scope.yaml"), []byte(scope), 0o644); err != nil {
		t.Fatalf("write scope: %v", err)
	}
	marker := filepath.Join(t.TempDir(), "excluded-cidr-naabu-ran")
	writeManagedBundleFixture(t, root, "naabu", "binary", "2.3.4", "#!/usr/bin/env sh\nprintf 'should-not-run-excluded-cidr-naabu\\n'\ntouch "+marker+"\n")
	if _, err := ExecuteManagedToolBundleInstall(root, "infra-pack", "naabu", ManagedToolBundleInstallOptions{Execute: true}); err != nil {
		t.Fatalf("install managed naabu bundle: %v", err)
	}

	_, err := RunWorkspace(ctx, root)
	if err == nil || !strings.Contains(err.Error(), "excluded") {
		t.Fatalf("expected excluded CIDR failure before naabu execution, got %v", err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("excluded CIDR naabu payload must not run, marker stat err=%v", err)
	}
}

func TestNativeManagedRunCommandKeepsNaabuOfflineForNonCIDRScope(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	writeManagedBundleFixture(t, root, "naabu", "binary", "2.3.4", "#!/usr/bin/env sh\nprintf 'naabu-non-cidr\\n'\n")
	if _, err := ExecuteManagedToolBundleInstall(root, "infra-pack", "naabu", ManagedToolBundleInstallOptions{Execute: true}); err != nil {
		t.Fatalf("install managed naabu bundle: %v", err)
	}
	for _, scopeTarget := range []ScopeTarget{
		{Kind: "url", Value: "https://app.example"},
		{Kind: "host", Value: "api.example.com"},
	} {
		t.Run(scopeTarget.Kind, func(t *testing.T) {
			request := ToolExecutionRequest{
				WorkspaceRoot: root,
				Node: compiler.PlanNode{
					ID:             "naabu-non-cidr-scope:01",
					Capability:     "infra.host_discovery",
					ManifestName:   "infra-pack",
					ToolID:         "naabu",
					AdapterID:      "managed_binary",
					Operation:      "port_scan",
					ParserID:       "naabu_jsonl",
					TargetKind:     "host",
					DependencyName: "naabu",
					DependencyKind: "binary",
					Runner:         "native",
				},
				ScopeTargets:  []ScopeTarget{scopeTarget},
				IsolationMode: "process",
			}

			command, ok, err := nativeManagedRunCommand(request)
			if err != nil || ok || len(command) > 0 {
				t.Fatalf("naabu must stay offline for non-CIDR scope %#v, command=%#v ok=%v err=%v", scopeTarget, command, ok, err)
			}
		})
	}
}

func TestNativeManagedRunCommandUsesCIDRForNaabuWhenHostAndCIDRScopeExist(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	payloadPath := writeManagedBundleFixture(t, root, "naabu", "binary", "2.3.4", "#!/usr/bin/env sh\nprintf 'naabu-cidr-with-host-present\\n'\n")
	if _, err := ExecuteManagedToolBundleInstall(root, "infra-pack", "naabu", ManagedToolBundleInstallOptions{Execute: true}); err != nil {
		t.Fatalf("install managed naabu bundle: %v", err)
	}
	request := ToolExecutionRequest{
		WorkspaceRoot: root,
		Node: compiler.PlanNode{
			ID:             "naabu-host-cidr-scope:01",
			Capability:     "infra.host_discovery",
			ManifestName:   "infra-pack",
			ToolID:         "naabu",
			AdapterID:      "managed_binary",
			Operation:      "port_scan",
			ParserID:       "naabu_jsonl",
			TargetKind:     "host",
			DependencyName: "naabu",
			DependencyKind: "binary",
			Runner:         "native",
		},
		ScopeTargets: []ScopeTarget{
			{Kind: "host", Value: "api.example.com"},
			{Kind: "cidr", Value: "10.0.0.0/24"},
		},
		IsolationMode: "process",
	}

	command, ok, err := nativeManagedRunCommand(request)
	if err != nil || !ok {
		t.Fatalf("expected naabu to use CIDR scope even when host scope is present, command=%#v ok=%v err=%v", command, ok, err)
	}
	want := []string{payloadPath, "-json", "-host", "10.0.0.0/24"}
	if !reflect.DeepEqual(command, want) {
		t.Fatalf("unexpected naabu argv\nwant=%#v\n got=%#v", want, command)
	}
}

func TestNativeManagedRunCommandKeepsNmapOfflineForUnsupportedTargetKind(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	writeManagedBundleFixture(t, root, "nmap", "binary", "7.95", "#!/usr/bin/env sh\nprintf 'nmap-unsupported-target\\n'\n")
	if _, err := ExecuteManagedToolBundleInstall(root, "infra-pack", "nmap", ManagedToolBundleInstallOptions{Execute: true}); err != nil {
		t.Fatalf("install managed nmap bundle: %v", err)
	}
	request := ToolExecutionRequest{
		WorkspaceRoot: root,
		Node: compiler.PlanNode{
			ID:             "nmap-url-target-kind:01",
			Capability:     "infra.host_discovery",
			ManifestName:   "infra-pack",
			ToolID:         "nmap",
			AdapterID:      "managed_binary",
			Operation:      "service_fingerprint",
			ParserID:       "nmap_xml",
			TargetKind:     "url",
			DependencyName: "nmap",
			DependencyKind: "binary",
			Runner:         "native",
		},
		ScopeTargets:  []ScopeTarget{{Kind: "url", Value: "https://app.example"}},
		IsolationMode: "process",
	}

	command, ok, err := nativeManagedRunCommand(request)
	if err != nil || ok || len(command) > 0 {
		t.Fatalf("nmap must stay offline for unsupported target kind, command=%#v ok=%v err=%v", command, ok, err)
	}
}

func TestNativeManagedRunCommandKeepsSubfinderOfflineForURLScope(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	writeManagedBundleFixture(t, root, "subfinder", "binary", "2.7.0", "#!/usr/bin/env sh\nprintf 'should-not-run-subfinder-from-url-scope\\n'\n")
	if _, err := ExecuteManagedToolBundleInstall(root, "web-api-pack", "subfinder", ManagedToolBundleInstallOptions{Execute: true}); err != nil {
		t.Fatalf("install managed subfinder bundle: %v", err)
	}
	request := ToolExecutionRequest{
		WorkspaceRoot: root,
		Node: compiler.PlanNode{
			ID:             "subfinder-url-scope:01",
			Capability:     "web.discovery",
			ManifestName:   "web-api-pack",
			ToolID:         "subfinder",
			AdapterID:      "managed_binary",
			Operation:      "subdomain_discovery",
			ParserID:       "subfinder_jsonl",
			TargetKind:     "domain",
			DependencyName: "subfinder",
			DependencyKind: "binary",
			Runner:         "native",
		},
		ScopeTargets:  []ScopeTarget{{Kind: "url", Value: "https://app.example"}},
		IsolationMode: "process",
	}

	command, ok, err := nativeManagedRunCommand(request)
	if err != nil || ok || len(command) > 0 {
		t.Fatalf("subfinder must stay offline until domain scope is explicit, command=%#v ok=%v err=%v", command, ok, err)
	}
}

func TestRunWorkspaceExecutesToolLevelSubfinderThroughExplicitDomainScope(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	scope := `version: 1
scope:
  target_policy: enforce
  targets:
    - kind: domain
      value: example.com
  exclusions: []
execution:
  isolation: process
`
	if err := os.WriteFile(filepath.Join(root, "scope.yaml"), []byte(scope), 0o644); err != nil {
		t.Fatalf("write scope: %v", err)
	}
	marker := filepath.Join(t.TempDir(), "tool-level-catalog-subfinder-ran")
	writeManagedBundleFixture(t, root, "subfinder", "binary", "2.7.0", "#!/usr/bin/env sh\nprintf 'tool-level-subfinder-output args=%s target=%s tool_id=%s operation=%s parser=%s dep=%s target_kind=%s scope_target_kind=%s\\n' \"$*\" \"$OURO_TARGET\" \"$OURO_TOOL_ID\" \"$OURO_OPERATION\" \"$OURO_PARSER_ID\" \"$OURO_DEPENDENCY_NAME\" \"$OURO_TARGET_KIND\" \"$OURO_SCOPE_TARGET_KIND\"\ntouch "+marker+"\n")
	if _, err := ExecuteManagedToolBundleInstall(root, "web-api-pack", "subfinder", ManagedToolBundleInstallOptions{Execute: true}); err != nil {
		t.Fatalf("install managed subfinder bundle: %v", err)
	}

	result, err := RunWorkspace(ctx, root)
	if err != nil {
		t.Fatalf("run workspace: %v", err)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("expected subfinder tool-level payload to run through explicit domain scope builder: %v", err)
	}
	store, err := eventlog.OpenSQLite(filepath.Join(root, "replay", "events.db"))
	if err != nil {
		t.Fatalf("open event log: %v", err)
	}
	defer store.Close()
	events, err := store.Replay(ctx, result.RunID)
	if err != nil {
		t.Fatalf("replay events: %v", err)
	}
	for _, event := range events {
		if event.Type != eventlog.EventTool {
			continue
		}
		var payload struct {
			Tool            string `json:"tool"`
			Parser          string `json:"parser"`
			ExecutionStatus string `json:"execution_status"`
			Transcript      string `json:"transcript"`
		}
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			t.Fatalf("decode tool payload: %v", err)
		}
		if payload.Tool != "web-api-pack" || payload.Parser != "subfinder_jsonl" || payload.ExecutionStatus == "" {
			continue
		}
		if payload.ExecutionStatus != "completed" {
			t.Fatalf("expected subfinder tool-level node to complete, got %#v", payload)
		}
		for _, want := range []string{
			"tool-level-subfinder-output",
			"args=-json -d example.com",
			"target=example.com",
			"tool_id=subfinder",
			"operation=subdomain_discovery",
			"parser=subfinder_jsonl",
			"dep=subfinder",
			"target_kind=domain",
			"scope_target_kind=domain",
		} {
			if !strings.Contains(payload.Transcript, want) {
				t.Fatalf("expected subfinder tool-level transcript to contain %q, got %#v", want, payload)
			}
		}
		if strings.Contains(payload.Transcript, "offline tool execution boundary") {
			t.Fatalf("subfinder tool-level execution must not use offline runner, got %#v", payload)
		}
		return
	}
	t.Fatalf("expected subfinder tool-level execution event, got %#v", events)
}

func TestRunWorkspaceRejectsExcludedDomainBeforeSubfinderExecution(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	scope := `version: 1
scope:
  target_policy: enforce
  targets:
    - kind: domain
      value: example.com
  exclusions:
    - example.com
execution:
  isolation: process
`
	if err := os.WriteFile(filepath.Join(root, "scope.yaml"), []byte(scope), 0o644); err != nil {
		t.Fatalf("write scope: %v", err)
	}
	marker := filepath.Join(t.TempDir(), "excluded-domain-subfinder-ran")
	writeManagedBundleFixture(t, root, "subfinder", "binary", "2.7.0", "#!/usr/bin/env sh\nprintf 'should-not-run-excluded-domain-subfinder\\n'\ntouch "+marker+"\n")
	if _, err := ExecuteManagedToolBundleInstall(root, "web-api-pack", "subfinder", ManagedToolBundleInstallOptions{Execute: true}); err != nil {
		t.Fatalf("install managed subfinder bundle: %v", err)
	}

	_, err := RunWorkspace(ctx, root)
	if err == nil || !strings.Contains(err.Error(), "excluded") {
		t.Fatalf("expected excluded domain failure, got %v", err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("excluded domain subfinder payload must not run, marker stat err=%v", err)
	}
}

func TestRunWorkspaceExecutesToolLevelNmapThroughExplicitHostScope(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	scope := `version: 1
scope:
  target_policy: enforce
  targets:
    - kind: host
      value: api.example.com
  exclusions: []
execution:
  isolation: process
`
	if err := os.WriteFile(filepath.Join(root, "scope.yaml"), []byte(scope), 0o644); err != nil {
		t.Fatalf("write scope: %v", err)
	}
	marker := filepath.Join(t.TempDir(), "tool-level-catalog-nmap-host-ran")
	writeManagedBundleFixture(t, root, "nmap", "binary", "7.95", "#!/usr/bin/env sh\nprintf 'tool-level-nmap-host-output args=%s target=%s tool_id=%s operation=%s parser=%s dep=%s target_kind=%s scope_target_kind=%s\\n' \"$*\" \"$OURO_TARGET\" \"$OURO_TOOL_ID\" \"$OURO_OPERATION\" \"$OURO_PARSER_ID\" \"$OURO_DEPENDENCY_NAME\" \"$OURO_TARGET_KIND\" \"$OURO_SCOPE_TARGET_KIND\"\ntouch "+marker+"\n")
	if _, err := ExecuteManagedToolBundleInstall(root, "infra-pack", "nmap", ManagedToolBundleInstallOptions{Execute: true}); err != nil {
		t.Fatalf("install managed nmap bundle: %v", err)
	}

	result, err := RunWorkspace(ctx, root)
	if err != nil {
		t.Fatalf("run workspace: %v", err)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("expected nmap host-scope payload to run through registered argv builder: %v", err)
	}
	store, err := eventlog.OpenSQLite(filepath.Join(root, "replay", "events.db"))
	if err != nil {
		t.Fatalf("open event log: %v", err)
	}
	defer store.Close()
	events, err := store.Replay(ctx, result.RunID)
	if err != nil {
		t.Fatalf("replay events: %v", err)
	}
	for _, event := range events {
		if event.Type != eventlog.EventTool {
			continue
		}
		var payload struct {
			Tool            string `json:"tool"`
			Parser          string `json:"parser"`
			ExecutionStatus string `json:"execution_status"`
			Transcript      string `json:"transcript"`
		}
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			t.Fatalf("decode tool payload: %v", err)
		}
		if payload.Tool != "infra-pack" || payload.Parser != "nmap_xml" || payload.ExecutionStatus == "" {
			continue
		}
		if payload.ExecutionStatus != "completed" {
			t.Fatalf("expected nmap host-scope node to complete, got %#v", payload)
		}
		for _, want := range []string{
			"tool-level-nmap-host-output",
			"args=-oX - api.example.com",
			"target=api.example.com",
			"tool_id=nmap",
			"operation=service_fingerprint",
			"parser=nmap_xml",
			"dep=nmap",
			"target_kind=host",
			"scope_target_kind=host",
		} {
			if !strings.Contains(payload.Transcript, want) {
				t.Fatalf("expected nmap host-scope transcript to contain %q, got %#v", want, payload)
			}
		}
		if strings.Contains(payload.Transcript, "offline tool execution boundary") {
			t.Fatalf("nmap host-scope execution must not use offline runner, got %#v", payload)
		}
		return
	}
	t.Fatalf("expected nmap host-scope execution event, got %#v", events)
}

func TestRunWorkspacePromotesAmassManagedOutputFileToEvidenceSurface(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	scope := `version: 1
scope:
  target_policy: enforce
  targets:
    - kind: domain
      value: example.com
  exclusions: []
execution:
  isolation: process
  tool_concurrency: 1
`
	if err := os.WriteFile(filepath.Join(root, "scope.yaml"), []byte(scope), 0o644); err != nil {
		t.Fatalf("write domain scope: %v", err)
	}
	writeManagedBundleFixture(t, root, "amass", "binary", "4.2.0", `#!/usr/bin/env sh
out=""
while [ "$#" -gt 0 ]; do
  if [ "$1" = "-json" ]; then
    shift
    out="$1"
  fi
  shift
done
mkdir -p "$(dirname "$out")"
cat > "$out" <<'JSONL'
{"name":"api.example.com","source":"secret-token","addresses":[{"ip":"192.0.2.10"}]}
{"name":"evil.example.net","source":"secret-token"}
JSONL
printf 'amass wrote %s\n' "$out"
`)
	if _, err := ExecuteManagedToolBundleInstall(root, "infra-pack", "amass", ManagedToolBundleInstallOptions{Execute: true}); err != nil {
		t.Fatalf("install managed amass bundle: %v", err)
	}

	result, err := RunWorkspaceWithOptions(ctx, root, RunOptions{Action: compiler.ActionDiscoverAssets, DeferReportProjection: true})
	if err != nil {
		t.Fatalf("run workspace with amass parser: %v", err)
	}
	store, err := eventlog.OpenSQLite(filepath.Join(root, "replay", "events.db"))
	if err != nil {
		t.Fatalf("open event log: %v", err)
	}
	defer store.Close()
	events, err := store.Replay(ctx, result.RunID)
	if err != nil {
		t.Fatalf("replay events: %v", err)
	}
	snapshot, err := eventlog.BuildReplaySnapshot(events)
	if err != nil {
		t.Fatalf("build replay snapshot: %v", err)
	}
	var surfaceID string
	for _, fact := range snapshot.Blackboard.Facts {
		if fact.Source != "parser.amass_json" {
			continue
		}
		surfaceID = fact.ID
		if fact.Kind != "surface" || !strings.Contains(fact.Statement, "api.example.com") || strings.Contains(fact.Statement, "secret-token") || strings.Contains(fact.Statement, "evil.example.net") {
			t.Fatalf("unexpected amass fact: %#v", fact)
		}
	}
	if surfaceID == "" {
		t.Fatalf("expected amass parser surface fact in replay snapshot, facts=%#v", snapshot.Blackboard.Facts)
	}
	if _, ok := snapshot.Evidence.Node(surfaceID); !ok {
		t.Fatalf("expected amass parser fact to project into evidence graph")
	}
}

func TestRunWorkspacePromotesDnsreconManagedOutputFileToEvidenceSurface(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	scope := `version: 1
scope:
  target_policy: enforce
  targets:
    - kind: domain
      value: example.com
  exclusions: []
execution:
  isolation: process
  tool_concurrency: 1
`
	if err := os.WriteFile(filepath.Join(root, "scope.yaml"), []byte(scope), 0o644); err != nil {
		t.Fatalf("write domain scope: %v", err)
	}
	writeManagedBundleFixture(t, root, "dnsrecon", "binary", "1.2.0", `#!/usr/bin/env sh
out=""
while [ "$#" -gt 0 ]; do
  if [ "$1" = "-j" ]; then
    shift
    out="$1"
  fi
  shift
done
mkdir -p "$(dirname "$out")"
cat > "$out" <<'JSON'
[
  {"type":"A","name":"api.example.com","address":"192.0.2.10","raw":"secret-token"},
  {"type":"A","name":"evil.example.net","address":"198.51.100.10","raw":"secret-token"}
]
JSON
printf 'dnsrecon wrote %s\n' "$out"
`)
	if _, err := ExecuteManagedToolBundleInstall(root, "infra-pack", "dnsrecon", ManagedToolBundleInstallOptions{Execute: true}); err != nil {
		t.Fatalf("install managed dnsrecon bundle: %v", err)
	}

	result, err := RunWorkspaceWithOptions(ctx, root, RunOptions{Action: compiler.ActionDiscoverAssets, DeferReportProjection: true})
	if err != nil {
		t.Fatalf("run workspace with dnsrecon parser: %v", err)
	}
	store, err := eventlog.OpenSQLite(filepath.Join(root, "replay", "events.db"))
	if err != nil {
		t.Fatalf("open event log: %v", err)
	}
	defer store.Close()
	events, err := store.Replay(ctx, result.RunID)
	if err != nil {
		t.Fatalf("replay events: %v", err)
	}
	snapshot, err := eventlog.BuildReplaySnapshot(events)
	if err != nil {
		t.Fatalf("build replay snapshot: %v", err)
	}
	var surfaceID string
	for _, fact := range snapshot.Blackboard.Facts {
		if fact.Source != "parser.dnsrecon_json" {
			continue
		}
		surfaceID = fact.ID
		if fact.Kind != "surface" || !strings.Contains(fact.Statement, "api.example.com") || strings.Contains(fact.Statement, "secret-token") || strings.Contains(fact.Statement, "evil.example.net") {
			t.Fatalf("unexpected dnsrecon fact: %#v", fact)
		}
	}
	if surfaceID == "" {
		t.Fatalf("expected dnsrecon parser surface fact in replay snapshot, facts=%#v", snapshot.Blackboard.Facts)
	}
	if _, ok := snapshot.Evidence.Node(surfaceID); !ok {
		t.Fatalf("expected dnsrecon parser fact to project into evidence graph")
	}
}

func TestNativeManagedRunCommandBuildsDomainFileOutputArgv(t *testing.T) {
	tests := []struct {
		toolID    string
		operation string
		parserID  string
		wantArgs  []string
	}{
		{
			toolID:    "amass",
			operation: "subdomain_discovery",
			parserID:  "amass_json",
			wantArgs:  []string{"enum", "-passive", "-nocolor", "-d", "example.com", "-json"},
		},
		{
			toolID:    "dnsrecon",
			operation: "dns_probe",
			parserID:  "dnsrecon_json",
			wantArgs:  []string{"-d", "example.com", "-t", "std", "-j"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.toolID, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "workspace")
			if err := InitWorkspace(root); err != nil {
				t.Fatalf("init workspace: %v", err)
			}
			payloadPath := writeManagedBundleFixture(t, root, tt.toolID, "binary", "1.0.0", "#!/usr/bin/env sh\nprintf 'file-output-tool\\n'\n")
			if _, err := ExecuteManagedToolBundleInstall(root, "infra-pack", tt.toolID, ManagedToolBundleInstallOptions{Execute: true}); err != nil {
				t.Fatalf("install managed %s bundle: %v", tt.toolID, err)
			}
			request := ToolExecutionRequest{
				WorkspaceRoot: root,
				Node: compiler.PlanNode{
					ID:             tt.toolID + "-domain-output:01",
					Capability:     "infra.host_discovery",
					ManifestName:   "infra-pack",
					ToolID:         tt.toolID,
					AdapterID:      "managed_binary",
					Operation:      tt.operation,
					ParserID:       tt.parserID,
					TargetKind:     "domain",
					DependencyName: tt.toolID,
					DependencyKind: "binary",
					Runner:         "native",
				},
				ScopeTargets:      []ScopeTarget{{Kind: "domain", Value: "example.com"}},
				ScopeTargetPolicy: "enforce",
				IsolationMode:     "process",
			}

			command, target, ok, err := nativeManagedRunCommandWithTarget(request)
			if err != nil || !ok {
				t.Fatalf("expected %s command for domain scope, command=%#v target=%#v ok=%v err=%v", tt.toolID, command, target, ok, err)
			}
			if target.Kind != "domain" || target.Value != "example.com" {
				t.Fatalf("expected selected domain target, got %#v", target)
			}
			if command[0] != payloadPath {
				t.Fatalf("expected resolved workspace-managed binary, got %#v want %s", command, payloadPath)
			}
			for _, want := range tt.wantArgs {
				if !containsTrimmedValue(command[1:], want) {
					t.Fatalf("expected %s argv to contain %q, got %#v", tt.toolID, want, command)
				}
			}
			outputPath := command[len(command)-1]
			if !strings.HasPrefix(outputPath, filepath.Join(root, "toolchain", "managed-outputs", tt.toolID)+string(os.PathSeparator)) || !strings.HasSuffix(outputPath, ".json") {
				t.Fatalf("expected workspace-confined managed output path, got %#v", command)
			}
		})
	}
}

func TestNativeManagedRunCommandKeepsDomainToolsOfflineForURLScope(t *testing.T) {
	tests := []struct {
		name       string
		pack       string
		capability string
		toolID     string
		operation  string
		parserID   string
	}{
		{
			name:       "amass",
			pack:       "infra-pack",
			capability: "infra.host_discovery",
			toolID:     "amass",
			operation:  "subdomain_discovery",
			parserID:   "amass_json",
		},
		{
			name:       "dnsrecon",
			pack:       "infra-pack",
			capability: "infra.host_discovery",
			toolID:     "dnsrecon",
			operation:  "dns_probe",
			parserID:   "dnsrecon_json",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "workspace")
			if err := InitWorkspace(root); err != nil {
				t.Fatalf("init workspace: %v", err)
			}
			writeManagedBundleFixture(t, root, tt.toolID, "binary", "1.0.0", "#!/usr/bin/env sh\nprintf 'should-not-run-domain-tool-from-url-scope\\n'\n")
			if _, err := ExecuteManagedToolBundleInstall(root, tt.pack, tt.toolID, ManagedToolBundleInstallOptions{Execute: true}); err != nil {
				t.Fatalf("install managed %s bundle: %v", tt.toolID, err)
			}
			request := ToolExecutionRequest{
				WorkspaceRoot: root,
				Node: compiler.PlanNode{
					ID:             tt.toolID + "-url-scope:01",
					Capability:     tt.capability,
					ManifestName:   tt.pack,
					ToolID:         tt.toolID,
					AdapterID:      "managed_binary",
					Operation:      tt.operation,
					ParserID:       tt.parserID,
					TargetKind:     "domain",
					DependencyName: tt.toolID,
					DependencyKind: "binary",
					Runner:         "native",
				},
				ScopeTargets:  []ScopeTarget{{Kind: "url", Value: "https://app.example"}},
				IsolationMode: "process",
			}

			command, ok, err := nativeManagedRunCommand(request)
			if err != nil || ok || len(command) > 0 {
				t.Fatalf("%s must stay offline until domain scope is explicit, command=%#v ok=%v err=%v", tt.toolID, command, ok, err)
			}
		})
	}
}

func TestNativeManagedRunCommandKeepsUnknownToolLevelOperationOffline(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	writeManagedBundleFixture(t, root, "httpx", "binary", "1.3.0", "#!/usr/bin/env sh\nprintf 'should-not-run-unknown-operation\\n'\n")
	if _, err := ExecuteManagedToolBundleInstall(root, "web-api-pack", "httpx", ManagedToolBundleInstallOptions{Execute: true}); err != nil {
		t.Fatalf("install managed httpx bundle: %v", err)
	}
	request := ToolExecutionRequest{
		WorkspaceRoot: root,
		Node: compiler.PlanNode{
			ID:             "unknown-tool-level:01",
			Capability:     "web.discovery",
			ManifestName:   "web-api-pack",
			ToolID:         "httpx",
			AdapterID:      "managed_binary",
			Operation:      "unknown_probe",
			ParserID:       "generic_jsonl",
			TargetKind:     "url",
			DependencyName: "httpx",
			DependencyKind: "binary",
			Runner:         "native",
		},
		ScopeTargets:  []ScopeTarget{{Kind: "url", Value: "https://app.example"}},
		IsolationMode: "process",
	}

	command, ok, err := nativeManagedRunCommand(request)
	if err != nil || ok || len(command) > 0 {
		t.Fatalf("unknown tool-level operation must stay offline, command=%#v ok=%v err=%v", command, ok, err)
	}
}

func TestNativeManagedRunCommandRequiresWorkspaceManagedBinaryForToolLevelBuilder(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	request := ToolExecutionRequest{
		WorkspaceRoot: root,
		Node: compiler.PlanNode{
			ID:             "missing-managed-binary:01",
			Capability:     "web.discovery",
			ManifestName:   "web-api-pack",
			ToolID:         "httpx",
			AdapterID:      "managed_binary",
			Operation:      "http_probe",
			ParserID:       "httpx_jsonl",
			TargetKind:     "url",
			DependencyName: "httpx",
			DependencyKind: "binary",
			Runner:         "native",
		},
		ScopeTargets:  []ScopeTarget{{Kind: "url", Value: "https://app.example"}},
		IsolationMode: "process",
	}

	command, ok, err := nativeManagedRunCommand(request)
	if err != nil || ok || len(command) > 0 {
		t.Fatalf("tool-level builder must not fall through to ambient PATH, command=%#v ok=%v err=%v", command, ok, err)
	}
}

func TestNativeManagedRunCommandUsesResolvedWorkspaceManagedBinaryForToolLevelBuilder(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	payloadPath := writeManagedBundleFixture(t, root, "httpx", "binary", "1.3.0", "#!/usr/bin/env sh\nprintf 'resolved-workspace-managed-binary\\n'\n")
	if _, err := ExecuteManagedToolBundleInstall(root, "web-api-pack", "httpx", ManagedToolBundleInstallOptions{Execute: true}); err != nil {
		t.Fatalf("install managed httpx bundle: %v", err)
	}
	request := ToolExecutionRequest{
		WorkspaceRoot: root,
		Node: compiler.PlanNode{
			ID:             "resolved-managed-binary:01",
			Capability:     "web.discovery",
			ManifestName:   "web-api-pack",
			ToolID:         "httpx",
			AdapterID:      "managed_binary",
			Operation:      "http_probe",
			ParserID:       "httpx_jsonl",
			TargetKind:     "url",
			DependencyName: "httpx",
			DependencyKind: "binary",
			Runner:         "native",
		},
		ScopeTargets:  []ScopeTarget{{Kind: "url", Value: "https://app.example"}},
		IsolationMode: "process",
	}

	command, ok, err := nativeManagedRunCommand(request)
	if err != nil || !ok {
		t.Fatalf("expected tool-level builder command, command=%#v ok=%v err=%v", command, ok, err)
	}
	if len(command) == 0 || command[0] != payloadPath {
		t.Fatalf("expected resolved workspace-managed binary path %q, got %#v", payloadPath, command)
	}
}

func TestNativeManagedRunCommandBuildsToolLevelBuilderForContainerIsolation(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	request := ToolExecutionRequest{
		WorkspaceRoot: root,
		Node: compiler.PlanNode{
			ID:             "container-tool-level:01",
			Capability:     "web.discovery",
			ManifestName:   "web-api-pack",
			ToolID:         "httpx",
			AdapterID:      "managed_binary",
			Operation:      "http_probe",
			ParserID:       "httpx_jsonl",
			TargetKind:     "url",
			DependencyName: "httpx",
			DependencyKind: "binary",
			Runner:         "native",
		},
		ScopeTargets:  []ScopeTarget{{Kind: "url", Value: "https://app.example"}},
		IsolationMode: "container",
	}

	command, ok, err := nativeManagedRunCommand(request)
	if err != nil || !ok {
		t.Fatalf("expected container-mode tool-level builder command, command=%#v ok=%v err=%v", command, ok, err)
	}
	want := []string{"httpx", "-json", "-u", "https://app.example"}
	if strings.Join(command, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("unexpected container-mode managed argv: got %#v want %#v", command, want)
	}
}

func TestNativeManagedRunCommandRequiresBinaryDependencyForToolLevelBuilder(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	writeManagedBundleFixture(t, root, "httpx", "binary", "1.3.0", "#!/usr/bin/env sh\nprintf 'should-not-run-without-dependency-contract\\n'\n")
	if _, err := ExecuteManagedToolBundleInstall(root, "web-api-pack", "httpx", ManagedToolBundleInstallOptions{Execute: true}); err != nil {
		t.Fatalf("install managed httpx bundle: %v", err)
	}
	request := ToolExecutionRequest{
		WorkspaceRoot: root,
		Node: compiler.PlanNode{
			ID:           "missing-dependency-contract:01",
			Capability:   "web.discovery",
			ManifestName: "web-api-pack",
			ToolID:       "httpx",
			AdapterID:    "managed_binary",
			Operation:    "http_probe",
			ParserID:     "httpx_jsonl",
			TargetKind:   "url",
			Runner:       "native",
		},
		ScopeTargets:  []ScopeTarget{{Kind: "url", Value: "https://app.example"}},
		IsolationMode: "process",
	}

	command, ok, err := nativeManagedRunCommand(request)
	if err != nil || ok || len(command) > 0 {
		t.Fatalf("tool-level builder must require a binary dependency contract, command=%#v ok=%v err=%v", command, ok, err)
	}
}

func TestRunWorkspaceNativeManagedBundleRejectsExcludedScopeBeforeExecution(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	scope := `version: 1
scope:
  target_policy: enforce
  targets:
    - kind: url
      value: https://app.example
  exclusions:
    - https://app.example
execution:
  isolation: process
`
	if err := os.WriteFile(filepath.Join(root, "scope.yaml"), []byte(scope), 0o644); err != nil {
		t.Fatalf("write scope: %v", err)
	}
	writeNativeManagedWebAPIPackManifestForRunTest(t, root)
	marker := filepath.Join(t.TempDir(), "excluded-native-ran")
	writeManagedBundleFixture(t, root, "httpx", "binary", "1.3.0", "#!/usr/bin/env sh\nprintf 'should-not-run\\n'\ntouch "+marker+"\n")
	if _, err := ExecuteManagedToolBundleInstall(root, "web-api-pack", "httpx", ManagedToolBundleInstallOptions{Execute: true}); err != nil {
		t.Fatalf("install managed httpx bundle: %v", err)
	}
	observed := []eventlog.Event{}

	_, err := RunWorkspaceWithOptions(ctx, root, RunOptions{
		OnEvent: func(event eventlog.Event) error {
			observed = append(observed, event)
			return nil
		},
	})
	if err == nil || !strings.Contains(err.Error(), "excluded") {
		t.Fatalf("expected excluded native managed execution failure, got %v", err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("excluded native managed payload must not run, marker stat err=%v", err)
	}
	if len(observed) == 0 {
		t.Fatalf("expected persisted events before excluded native managed failure")
	}
	store, err := eventlog.OpenSQLite(filepath.Join(root, "replay", "events.db"))
	if err != nil {
		t.Fatalf("open event log: %v", err)
	}
	defer store.Close()
	events, err := store.Replay(ctx, observed[0].RunID)
	if err != nil {
		t.Fatalf("replay events: %v", err)
	}
	for _, event := range events {
		if event.Type == eventlog.EventAttempt {
			var payload struct {
				Kind    string `json:"kind"`
				StageID string `json:"stage_id"`
			}
			if err := json.Unmarshal(event.Payload, &payload); err != nil {
				t.Fatalf("decode stage payload: %v", err)
			}
			if payload.Kind == "stage" && payload.StageID == "report-projection" {
				t.Fatalf("report projection must not start after excluded native managed execution")
			}
		}
		if event.Type != eventlog.EventTool {
			continue
		}
		var payload struct {
			Tool            string `json:"tool"`
			Capability      string `json:"capability"`
			ExecutionStatus string `json:"execution_status"`
			Transcript      string `json:"transcript"`
		}
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			t.Fatalf("decode tool payload: %v", err)
		}
		if payload.Tool == "web-api-pack" && payload.Capability == "web.discovery" && payload.ExecutionStatus == "failed" {
			if !strings.Contains(payload.Transcript, "excluded") {
				t.Fatalf("expected exclusion transcript, got %#v", payload)
			}
			if strings.Contains(payload.Transcript, "should-not-run") {
				t.Fatalf("excluded native managed command must not execute, got %#v", payload)
			}
			return
		}
	}
	t.Fatalf("expected durable failed native managed tool event for excluded target, got %#v", events)
}

func writeNativeManagedWebAPIPackManifestForRunTest(t *testing.T, root string) {
	t.Helper()
	manifest := `name: web-api-pack
version: "0.1.0"
capabilities:
  - web.discovery
runner: native
parser: web_api_parser
healthcheck: built_in
dependencies:
  - name: httpx
    kind: binary
    optional: true
    source: projectdiscovery/httpx
    health_command: [httpx, -version]
    install_hint: install managed httpx bundle
    install_command: [ouro-toolchain, install, httpx]
`
	if err := os.WriteFile(filepath.Join(root, "toolchain", "web-api-pack", "manifest.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write native managed web-api manifest: %v", err)
	}
}

func TestRunWorkspaceExpandsRunCommandTargetFromScope(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	scope := `version: 1
scope:
  targets:
    - kind: url
      value: https://app.example
  exclusions: []
`
	if err := os.WriteFile(filepath.Join(root, "scope.yaml"), []byte(scope), 0o644); err != nil {
		t.Fatalf("write scope: %v", err)
	}
	manifest := `name: web-api-pack
version: "0.1.0"
capabilities:
  - web.discovery
runner: command
run_command:
  - printf
  - "target={{target}} kind={{target_kind}}\n"
parser: web_api_parser
healthcheck: built_in
`
	if err := os.WriteFile(filepath.Join(root, "toolchain", "web-api-pack", "manifest.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	result, err := RunWorkspace(ctx, root)
	if err != nil {
		t.Fatalf("run workspace: %v", err)
	}
	store, err := eventlog.OpenSQLite(filepath.Join(root, "replay", "events.db"))
	if err != nil {
		t.Fatalf("open event log: %v", err)
	}
	defer store.Close()
	events, err := store.Replay(ctx, result.RunID)
	if err != nil {
		t.Fatalf("replay events: %v", err)
	}
	for _, event := range events {
		if event.Type != eventlog.EventTool {
			continue
		}
		var payload struct {
			Capability string `json:"capability"`
			Transcript string `json:"transcript"`
		}
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			t.Fatalf("decode tool payload: %v", err)
		}
		if payload.Capability != "web.discovery" {
			continue
		}
		if !strings.Contains(payload.Transcript, "target=https://app.example kind=url") {
			t.Fatalf("expected scoped target expansion, got %#v", payload)
		}
		return
	}
	t.Fatalf("expected web.discovery execution event")
}

func TestRunWorkspaceRejectsTargetTemplateWithoutMatchingScope(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	manifest := `name: web-api-pack
version: "0.1.0"
capabilities:
  - web.discovery
runner: command
run_command:
  - printf
  - "target={{target}}\n"
parser: web_api_parser
healthcheck: built_in
`
	if err := os.WriteFile(filepath.Join(root, "toolchain", "web-api-pack", "manifest.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	_, err := RunWorkspace(ctx, root)
	if err == nil {
		t.Fatalf("expected target template without matching scope to fail")
	}
	if !strings.Contains(err.Error(), "target template") {
		t.Fatalf("expected target template error, got %v", err)
	}
}

func TestRunWorkspaceRejectsCommandRunnerWithoutMatchingScopeTarget(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	manifest := `name: web-api-pack
version: "0.1.0"
capabilities:
  - web.discovery
runner: command
run_command:
  - printf
  - "unscoped command should not run\n"
parser: web_api_parser
healthcheck: built_in
`
	if err := os.WriteFile(filepath.Join(root, "toolchain", "web-api-pack", "manifest.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	observedEvents := []eventlog.Event{}
	_, err := RunWorkspaceWithOptions(ctx, root, RunOptions{
		OnEvent: func(event eventlog.Event) error {
			observedEvents = append(observedEvents, event)
			return nil
		},
	})
	if err == nil {
		t.Fatalf("expected command runner without matching scope target to fail")
	}
	if !strings.Contains(err.Error(), "scope target") {
		t.Fatalf("expected scope target error, got %v", err)
	}

	for _, event := range observedEvents {
		if event.Type != eventlog.EventTool {
			continue
		}
		var payload struct {
			Capability      string `json:"capability"`
			ExecutionStatus string `json:"execution_status"`
			Transcript      string `json:"transcript"`
		}
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			t.Fatalf("decode tool payload: %v", err)
		}
		if payload.Capability == "web.discovery" && payload.ExecutionStatus == "failed" && strings.Contains(payload.Transcript, "scope target") {
			return
		}
	}
	t.Fatalf("expected durable failed tool event for unscoped command, got %#v", observedEvents)
}

func TestRunWorkspaceRejectsCommandRunnerForExcludedScopeTarget(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	scope := `version: 1
scope:
  targets:
    - kind: url
      value: https://app.example
  exclusions:
    - " https://app.example "
`
	if err := os.WriteFile(filepath.Join(root, "scope.yaml"), []byte(scope), 0o644); err != nil {
		t.Fatalf("write scope: %v", err)
	}
	manifest := `name: web-api-pack
version: "0.1.0"
capabilities:
  - web.discovery
runner: command
run_command:
  - printf
  - "should-not-run\n"
parser: web_api_parser
healthcheck: built_in
`
	if err := os.WriteFile(filepath.Join(root, "toolchain", "web-api-pack", "manifest.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	observedEvents := []eventlog.Event{}
	observedSnapshots := []eventlog.ReplaySnapshot{}
	_, err := RunWorkspaceWithOptions(ctx, root, RunOptions{
		OnEvent: func(event eventlog.Event) error {
			observedEvents = append(observedEvents, event)
			return nil
		},
		OnSnapshot: func(snapshot eventlog.ReplaySnapshot) error {
			observedSnapshots = append(observedSnapshots, snapshot)
			return nil
		},
	})
	if err == nil {
		t.Fatalf("expected excluded scope target to fail")
	}
	if !strings.Contains(err.Error(), "excluded") {
		t.Fatalf("expected excluded target error, got %v", err)
	}
	if len(observedEvents) == 0 {
		t.Fatalf("expected persisted events before exclusion failure")
	}
	if len(observedSnapshots) == 0 || observedSnapshots[len(observedSnapshots)-1].Status.ToolHealth != "failed" {
		t.Fatalf("expected failed final snapshot, got %#v", observedSnapshots)
	}

	store, err := eventlog.OpenSQLite(filepath.Join(root, "replay", "events.db"))
	if err != nil {
		t.Fatalf("open event log: %v", err)
	}
	defer store.Close()
	events, err := store.Replay(ctx, observedEvents[0].RunID)
	if err != nil {
		t.Fatalf("replay events: %v", err)
	}
	for _, event := range events {
		if event.Type == eventlog.EventAttempt {
			var payload struct {
				Kind    string `json:"kind"`
				StageID string `json:"stage_id"`
			}
			if err := json.Unmarshal(event.Payload, &payload); err != nil {
				t.Fatalf("decode stage payload: %v", err)
			}
			if payload.Kind == "stage" && payload.StageID == "report-projection" {
				t.Fatalf("report projection must not start after excluded tool execution")
			}
		}
		if event.Type != eventlog.EventTool {
			continue
		}
		var payload struct {
			Capability      string `json:"capability"`
			ExecutionStatus string `json:"execution_status"`
			Transcript      string `json:"transcript"`
		}
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			t.Fatalf("decode tool payload: %v", err)
		}
		if payload.Capability == "web.discovery" && payload.ExecutionStatus == "failed" {
			if !strings.Contains(payload.Transcript, "excluded") {
				t.Fatalf("expected exclusion transcript, got %#v", payload)
			}
			if strings.Contains(payload.Transcript, "should-not-run") {
				t.Fatalf("excluded command must not execute, got %#v", payload)
			}
			return
		}
	}
	t.Fatalf("expected durable failed tool event for excluded target, got %#v", events)
}

func TestScopeTargetExcludedNormalizesURLAndCIDRExclusions(t *testing.T) {
	tests := []struct {
		name       string
		target     ScopeTarget
		exclusions []string
		want       bool
	}{
		{
			name:       "url subtree with trailing slash",
			target:     ScopeTarget{Kind: "url", Value: "https://app.example/admin/users/"},
			exclusions: []string{"https://app.example/admin"},
			want:       true,
		},
		{
			name:       "url default port",
			target:     ScopeTarget{Kind: "url", Value: "https://app.example:443/admin"},
			exclusions: []string{"https://app.example/admin/"},
			want:       true,
		},
		{
			name:       "url sibling path not excluded",
			target:     ScopeTarget{Kind: "url", Value: "https://app.example/app"},
			exclusions: []string{"https://app.example/admin"},
			want:       false,
		},
		{
			name:       "cidr host inside excluded prefix",
			target:     ScopeTarget{Kind: "cidr", Value: "10.0.1.5"},
			exclusions: []string{"10.0.0.0/16"},
			want:       true,
		},
		{
			name:       "cidr prefix inside excluded prefix",
			target:     ScopeTarget{Kind: "cidr", Value: "10.0.1.0/24"},
			exclusions: []string{"10.0.0.0/16"},
			want:       true,
		},
		{
			name:       "cidr outside excluded prefix",
			target:     ScopeTarget{Kind: "cidr", Value: "10.1.0.0/16"},
			exclusions: []string{"10.0.0.0/16"},
			want:       false,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := scopeTargetExcluded(test.target, test.exclusions); got != test.want {
				t.Fatalf("scopeTargetExcluded(%#v, %#v)=%v want %v", test.target, test.exclusions, got, test.want)
			}
		})
	}
}

func TestScopeTargetExcludedSupportsWildcardExclusions(t *testing.T) {
	tests := []struct {
		name       string
		target     ScopeTarget
		exclusions []string
		want       bool
	}{
		{
			name:       "url wildcard host and path excludes matching subtree",
			target:     ScopeTarget{Kind: "url", Value: "https://api.example.com/admin/users"},
			exclusions: []string{"https://*.example.com/admin/*"},
			want:       true,
		},
		{
			name:       "url wildcard host keeps default port normalization",
			target:     ScopeTarget{Kind: "url", Value: "https://api.example.com:443/admin"},
			exclusions: []string{"https://*.example.com/admin"},
			want:       true,
		},
		{
			name:       "url wildcard path does not match sibling prefix",
			target:     ScopeTarget{Kind: "url", Value: "https://api.example.com/administrator"},
			exclusions: []string{"https://*.example.com/admin/*"},
			want:       false,
		},
		{
			name:       "url wildcard host does not match sibling domain",
			target:     ScopeTarget{Kind: "url", Value: "https://evil-example.com/admin/users"},
			exclusions: []string{"https://*.example.com/admin/*"},
			want:       false,
		},
		{
			name:       "cidr wildcard matches explicit target string shape",
			target:     ScopeTarget{Kind: "cidr", Value: "10.0.12.0/24"},
			exclusions: []string{"10.0.*.0/24"},
			want:       true,
		},
		{
			name:       "cidr wildcard does not infer other address ranges",
			target:     ScopeTarget{Kind: "cidr", Value: "10.1.12.0/24"},
			exclusions: []string{"10.0.*.0/24"},
			want:       false,
		},
		{
			name:       "generic challenge wildcard matches explicit challenge value",
			target:     ScopeTarget{Kind: "challenge", Value: "pwnable-login"},
			exclusions: []string{"pwnable-*"},
			want:       true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := scopeTargetExcluded(test.target, test.exclusions); got != test.want {
				t.Fatalf("scopeTargetExcluded(%#v, %#v)=%v want %v", test.target, test.exclusions, got, test.want)
			}
		})
	}
}

func TestRunWorkspaceCanDisableCommandTargetPolicyForAuthorizedScope(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	scope := `version: 1
scope:
  target_policy: disabled
  targets:
    - kind: url
      value: https://app.example
  exclusions:
    - https://app.example
`
	if err := os.WriteFile(filepath.Join(root, "scope.yaml"), []byte(scope), 0o644); err != nil {
		t.Fatalf("write scope: %v", err)
	}
	manifest := `name: web-api-pack
version: "0.1.0"
capabilities:
  - web.discovery
runner: command
run_command:
  - printf
  - "target-policy-disabled\n"
parser: web_api_parser
healthcheck: built_in
`
	if err := os.WriteFile(filepath.Join(root, "toolchain", "web-api-pack", "manifest.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	result, err := RunWorkspace(ctx, root)
	if err != nil {
		t.Fatalf("run workspace with disabled target policy: %v", err)
	}
	store, err := eventlog.OpenSQLite(filepath.Join(root, "replay", "events.db"))
	if err != nil {
		t.Fatalf("open event log: %v", err)
	}
	defer store.Close()
	events, err := store.Replay(ctx, result.RunID)
	if err != nil {
		t.Fatalf("replay events: %v", err)
	}
	for _, event := range events {
		if event.Type != eventlog.EventTool {
			continue
		}
		var payload struct {
			Capability      string `json:"capability"`
			ExecutionStatus string `json:"execution_status"`
			Transcript      string `json:"transcript"`
		}
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			t.Fatalf("decode tool payload: %v", err)
		}
		if payload.Capability != "web.discovery" {
			continue
		}
		if payload.ExecutionStatus != "completed" || !strings.Contains(payload.Transcript, "target-policy-disabled") {
			t.Fatalf("expected disabled target policy to allow command execution, got %#v", payload)
		}
		return
	}
	t.Fatalf("expected web.discovery command event")
}

func TestRunWorkspaceRejectsOutOfScopeRunCommandURLArgument(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	scope := `version: 1
scope:
  target_policy: enforce
  targets:
    - kind: url
      value: https://app.example
  exclusions: []
execution:
  isolation: process
`
	if err := os.WriteFile(filepath.Join(root, "scope.yaml"), []byte(scope), 0o644); err != nil {
		t.Fatalf("write scope: %v", err)
	}
	manifest := `name: web-api-pack
version: "0.1.0"
capabilities:
  - web.discovery
runner: command
run_command:
  - printf
  - "probe https://other.example\n"
parser: web_api_parser
healthcheck: built_in
`
	if err := os.WriteFile(filepath.Join(root, "toolchain", "web-api-pack", "manifest.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	observedEvents := []eventlog.Event{}
	_, err := RunWorkspaceWithOptions(ctx, root, RunOptions{
		OnEvent: func(event eventlog.Event) error {
			observedEvents = append(observedEvents, event)
			return nil
		},
	})
	if err == nil {
		t.Fatalf("expected out-of-scope command URL argument to fail")
	}
	if !strings.Contains(err.Error(), "out-of-scope URL argument") {
		t.Fatalf("expected out-of-scope URL argument error, got %v", err)
	}
	for _, event := range observedEvents {
		if event.Type == eventlog.EventAttempt {
			var payload struct {
				Kind    string `json:"kind"`
				StageID string `json:"stage_id"`
			}
			if err := json.Unmarshal(event.Payload, &payload); err != nil {
				t.Fatalf("decode stage payload: %v", err)
			}
			if payload.Kind == "stage" && payload.StageID == "report-projection" {
				t.Fatalf("report projection must not start after out-of-scope command argument")
			}
		}
		if event.Type != eventlog.EventTool {
			continue
		}
		var payload struct {
			Capability      string `json:"capability"`
			ExecutionStatus string `json:"execution_status"`
			Transcript      string `json:"transcript"`
		}
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			t.Fatalf("decode tool payload: %v", err)
		}
		if payload.Capability == "web.discovery" && payload.ExecutionStatus == "failed" {
			if !strings.Contains(payload.Transcript, "out-of-scope URL argument") {
				t.Fatalf("expected target-policy transcript, got %#v", payload)
			}
			if strings.Contains(payload.Transcript, "probe ") {
				t.Fatalf("out-of-scope command must not execute, got %#v", payload)
			}
			return
		}
	}
	t.Fatalf("expected durable failed tool event for out-of-scope command argument, got %#v", observedEvents)
}

func TestRunWorkspaceRejectsEmbeddedOutOfScopeRunCommandURLArgument(t *testing.T) {
	for _, tc := range []struct {
		name     string
		argument string
	}{
		{name: "key value", argument: "--url=https://other.example"},
		{name: "comma list", argument: "targets=https://app.example,https://other.example"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			root := filepath.Join(t.TempDir(), "workspace")
			if err := InitWorkspace(root); err != nil {
				t.Fatalf("init workspace: %v", err)
			}
			scope := `version: 1
scope:
  target_policy: enforce
  targets:
    - kind: url
      value: https://app.example
  exclusions: []
execution:
  isolation: process
`
			if err := os.WriteFile(filepath.Join(root, "scope.yaml"), []byte(scope), 0o644); err != nil {
				t.Fatalf("write scope: %v", err)
			}
			manifest := fmt.Sprintf(`name: web-api-pack
version: "0.1.0"
capabilities:
  - web.discovery
runner: command
run_command:
  - printf
  - "%s\n"
parser: web_api_parser
healthcheck: built_in
`, tc.argument)
			if err := os.WriteFile(filepath.Join(root, "toolchain", "web-api-pack", "manifest.yaml"), []byte(manifest), 0o644); err != nil {
				t.Fatalf("write manifest: %v", err)
			}

			_, err := RunWorkspace(ctx, root)
			if err == nil {
				t.Fatalf("expected embedded out-of-scope command URL argument to fail")
			}
			if !strings.Contains(err.Error(), "out-of-scope URL argument") {
				t.Fatalf("expected out-of-scope URL argument error, got %v", err)
			}
		})
	}
}

func TestRunWorkspaceCanDisableRunCommandURLArgumentPolicy(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	scope := `version: 1
scope:
  target_policy: disabled
  targets:
    - kind: url
      value: https://app.example
  exclusions: []
execution:
  isolation: process
`
	if err := os.WriteFile(filepath.Join(root, "scope.yaml"), []byte(scope), 0o644); err != nil {
		t.Fatalf("write scope: %v", err)
	}
	manifest := `name: web-api-pack
version: "0.1.0"
capabilities:
  - web.discovery
runner: command
run_command:
  - printf
  - "probe https://other.example\n"
parser: web_api_parser
healthcheck: built_in
`
	if err := os.WriteFile(filepath.Join(root, "toolchain", "web-api-pack", "manifest.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	result, err := RunWorkspace(ctx, root)
	if err != nil {
		t.Fatalf("run workspace: %v", err)
	}
	store, err := eventlog.OpenSQLite(filepath.Join(root, "replay", "events.db"))
	if err != nil {
		t.Fatalf("open event log: %v", err)
	}
	defer store.Close()
	events, err := store.Replay(ctx, result.RunID)
	if err != nil {
		t.Fatalf("replay events: %v", err)
	}
	for _, event := range events {
		if event.Type != eventlog.EventTool {
			continue
		}
		var payload struct {
			Capability      string `json:"capability"`
			ExecutionStatus string `json:"execution_status"`
			Transcript      string `json:"transcript"`
		}
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			t.Fatalf("decode tool payload: %v", err)
		}
		if payload.Capability != "web.discovery" {
			continue
		}
		if payload.ExecutionStatus != "completed" || !strings.Contains(payload.Transcript, "probe https://other.example") {
			t.Fatalf("expected disabled target policy to allow URL argument, got %#v", payload)
		}
		return
	}
	t.Fatalf("expected web.discovery command event")
}

func TestRunWorkspaceCanDisableEmbeddedRunCommandURLArgumentPolicy(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	scope := `version: 1
scope:
  target_policy: disabled
  targets:
    - kind: url
      value: https://app.example
  exclusions: []
execution:
  isolation: process
`
	if err := os.WriteFile(filepath.Join(root, "scope.yaml"), []byte(scope), 0o644); err != nil {
		t.Fatalf("write scope: %v", err)
	}
	manifest := `name: web-api-pack
version: "0.1.0"
capabilities:
  - web.discovery
runner: command
run_command:
  - printf
  - "targets=https://app.example,https://other.example\n"
parser: web_api_parser
healthcheck: built_in
`
	if err := os.WriteFile(filepath.Join(root, "toolchain", "web-api-pack", "manifest.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	result, err := RunWorkspace(ctx, root)
	if err != nil {
		t.Fatalf("run workspace: %v", err)
	}
	store, err := eventlog.OpenSQLite(filepath.Join(root, "replay", "events.db"))
	if err != nil {
		t.Fatalf("open event log: %v", err)
	}
	defer store.Close()
	events, err := store.Replay(ctx, result.RunID)
	if err != nil {
		t.Fatalf("replay events: %v", err)
	}
	for _, event := range events {
		if event.Type != eventlog.EventTool {
			continue
		}
		var payload struct {
			Capability      string `json:"capability"`
			ExecutionStatus string `json:"execution_status"`
			Transcript      string `json:"transcript"`
		}
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			t.Fatalf("decode tool payload: %v", err)
		}
		if payload.Capability != "web.discovery" {
			continue
		}
		if payload.ExecutionStatus != "completed" || !strings.Contains(payload.Transcript, "targets=https://app.example,https://other.example") {
			t.Fatalf("expected disabled target policy to allow embedded URL arguments, got %#v", payload)
		}
		return
	}
	t.Fatalf("expected web.discovery command event")
}

func TestRunWorkspaceRejectsOutOfScopeRunCommandCIDRArgument(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	scope := `version: 1
scope:
  target_policy: enforce
  targets:
    - kind: cidr
      value: 10.0.0.0/24
  exclusions: []
execution:
  isolation: process
`
	if err := os.WriteFile(filepath.Join(root, "scope.yaml"), []byte(scope), 0o644); err != nil {
		t.Fatalf("write scope: %v", err)
	}
	manifest := `name: infra-pack
version: "0.1.0"
capabilities:
  - infra.host_discovery
runner: command
run_command:
  - printf
  - "scan 10.0.1.5\n"
parser: infra_parser
healthcheck: built_in
`
	if err := os.WriteFile(filepath.Join(root, "toolchain", "infra-pack", "manifest.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	observedEvents := []eventlog.Event{}
	_, err := RunWorkspaceWithOptions(ctx, root, RunOptions{
		OnEvent: func(event eventlog.Event) error {
			observedEvents = append(observedEvents, event)
			return nil
		},
	})
	if err == nil {
		t.Fatalf("expected out-of-scope command CIDR argument to fail")
	}
	if !strings.Contains(err.Error(), "out-of-scope CIDR argument") {
		t.Fatalf("expected out-of-scope CIDR argument error, got %v", err)
	}
	for _, event := range observedEvents {
		if event.Type == eventlog.EventAttempt {
			var payload struct {
				Kind    string `json:"kind"`
				StageID string `json:"stage_id"`
			}
			if err := json.Unmarshal(event.Payload, &payload); err != nil {
				t.Fatalf("decode stage payload: %v", err)
			}
			if payload.Kind == "stage" && payload.StageID == "report-projection" {
				t.Fatalf("report projection must not start after out-of-scope command argument")
			}
		}
		if event.Type != eventlog.EventTool {
			continue
		}
		var payload struct {
			Capability      string `json:"capability"`
			ExecutionStatus string `json:"execution_status"`
			Transcript      string `json:"transcript"`
		}
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			t.Fatalf("decode tool payload: %v", err)
		}
		if payload.Capability == "infra.host_discovery" && payload.ExecutionStatus == "failed" {
			if !strings.Contains(payload.Transcript, "out-of-scope CIDR argument") {
				t.Fatalf("expected CIDR policy transcript, got %#v", payload)
			}
			if strings.Contains(payload.Transcript, "scan ") {
				t.Fatalf("out-of-scope command must not execute, got %#v", payload)
			}
			return
		}
	}
	t.Fatalf("expected durable failed tool event for out-of-scope CIDR argument, got %#v", observedEvents)
}

func TestRunWorkspaceRejectsEmbeddedOutOfScopeRunCommandCIDRArgument(t *testing.T) {
	for _, tc := range []struct {
		name     string
		argument string
	}{
		{name: "key value", argument: "--host=10.0.1.5"},
		{name: "comma list", argument: "targets=10.0.0.10,10.0.1.5"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			root := filepath.Join(t.TempDir(), "workspace")
			if err := InitWorkspace(root); err != nil {
				t.Fatalf("init workspace: %v", err)
			}
			scope := `version: 1
scope:
  target_policy: enforce
  targets:
    - kind: cidr
      value: 10.0.0.0/24
  exclusions: []
execution:
  isolation: process
`
			if err := os.WriteFile(filepath.Join(root, "scope.yaml"), []byte(scope), 0o644); err != nil {
				t.Fatalf("write scope: %v", err)
			}
			manifest := fmt.Sprintf(`name: infra-pack
version: "0.1.0"
capabilities:
  - infra.host_discovery
runner: command
run_command:
  - printf
  - "%s\n"
parser: infra_parser
healthcheck: built_in
`, tc.argument)
			if err := os.WriteFile(filepath.Join(root, "toolchain", "infra-pack", "manifest.yaml"), []byte(manifest), 0o644); err != nil {
				t.Fatalf("write manifest: %v", err)
			}

			_, err := RunWorkspace(ctx, root)
			if err == nil {
				t.Fatalf("expected embedded out-of-scope command CIDR argument to fail")
			}
			if !strings.Contains(err.Error(), "out-of-scope CIDR argument") {
				t.Fatalf("expected out-of-scope CIDR argument error, got %v", err)
			}
		})
	}
}

func TestRunWorkspaceCanDisableRunCommandCIDRArgumentPolicy(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	scope := `version: 1
scope:
  target_policy: disabled
  targets:
    - kind: cidr
      value: 10.0.0.0/24
  exclusions: []
execution:
  isolation: process
`
	if err := os.WriteFile(filepath.Join(root, "scope.yaml"), []byte(scope), 0o644); err != nil {
		t.Fatalf("write scope: %v", err)
	}
	manifest := `name: infra-pack
version: "0.1.0"
capabilities:
  - infra.host_discovery
runner: command
run_command:
  - printf
  - "scan 10.0.1.5\n"
parser: infra_parser
healthcheck: built_in
`
	if err := os.WriteFile(filepath.Join(root, "toolchain", "infra-pack", "manifest.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	result, err := RunWorkspace(ctx, root)
	if err != nil {
		t.Fatalf("run workspace: %v", err)
	}
	store, err := eventlog.OpenSQLite(filepath.Join(root, "replay", "events.db"))
	if err != nil {
		t.Fatalf("open event log: %v", err)
	}
	defer store.Close()
	events, err := store.Replay(ctx, result.RunID)
	if err != nil {
		t.Fatalf("replay events: %v", err)
	}
	for _, event := range events {
		if event.Type != eventlog.EventTool {
			continue
		}
		var payload struct {
			Capability      string `json:"capability"`
			ExecutionStatus string `json:"execution_status"`
			Transcript      string `json:"transcript"`
		}
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			t.Fatalf("decode tool payload: %v", err)
		}
		if payload.Capability != "infra.host_discovery" {
			continue
		}
		if payload.ExecutionStatus != "completed" || !strings.Contains(payload.Transcript, "scan 10.0.1.5") {
			t.Fatalf("expected disabled target policy to allow CIDR argument, got %#v", payload)
		}
		return
	}
	t.Fatalf("expected infra.host_discovery command event")
}

func TestRunWorkspaceRejectsOutOfScopeRunCommandChallengeArgument(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	scope := `version: 1
scope:
  target_policy: enforce
  targets:
    - kind: challenge
      value: pwnable-login
  exclusions: []
execution:
  isolation: process
`
	if err := os.WriteFile(filepath.Join(root, "scope.yaml"), []byte(scope), 0o644); err != nil {
		t.Fatalf("write scope: %v", err)
	}
	manifest := `name: ctf-pack
version: "0.1.0"
capabilities:
  - ctf.challenge_classification
runner: command
run_command:
  - printf
  - "solve challenge:other-login\n"
parser: ctf_parser
healthcheck: built_in
`
	if err := os.WriteFile(filepath.Join(root, "toolchain", "ctf-pack", "manifest.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	observedEvents := []eventlog.Event{}
	_, err := RunWorkspaceWithOptions(ctx, root, RunOptions{
		OnEvent: func(event eventlog.Event) error {
			observedEvents = append(observedEvents, event)
			return nil
		},
	})
	if err == nil {
		t.Fatalf("expected out-of-scope challenge argument to fail")
	}
	if !strings.Contains(err.Error(), "out-of-scope challenge argument") {
		t.Fatalf("expected out-of-scope challenge argument error, got %v", err)
	}
	for _, event := range observedEvents {
		if event.Type == eventlog.EventAttempt {
			var payload struct {
				Kind    string `json:"kind"`
				StageID string `json:"stage_id"`
			}
			if err := json.Unmarshal(event.Payload, &payload); err != nil {
				t.Fatalf("decode stage payload: %v", err)
			}
			if payload.Kind == "stage" && payload.StageID == "report-projection" {
				t.Fatalf("report projection must not start after out-of-scope challenge argument")
			}
		}
		if event.Type != eventlog.EventTool {
			continue
		}
		var payload struct {
			Capability      string `json:"capability"`
			ExecutionStatus string `json:"execution_status"`
			Transcript      string `json:"transcript"`
		}
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			t.Fatalf("decode tool payload: %v", err)
		}
		if payload.Capability == "ctf.challenge_classification" && payload.ExecutionStatus == "failed" {
			if !strings.Contains(payload.Transcript, "out-of-scope challenge argument") {
				t.Fatalf("expected challenge policy transcript, got %#v", payload)
			}
			if strings.Contains(payload.Transcript, "solve ") {
				t.Fatalf("out-of-scope command must not execute, got %#v", payload)
			}
			return
		}
	}
	t.Fatalf("expected durable failed tool event for out-of-scope challenge argument, got %#v", observedEvents)
}

func TestRunWorkspaceRejectsEmbeddedOutOfScopeRunCommandChallengeArgument(t *testing.T) {
	for _, tc := range []struct {
		name     string
		argument string
	}{
		{name: "challenge prefix", argument: "--challenge=challenge:other-login"},
		{name: "ctf URL", argument: "target=ctf://other-login"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			root := filepath.Join(t.TempDir(), "workspace")
			if err := InitWorkspace(root); err != nil {
				t.Fatalf("init workspace: %v", err)
			}
			scope := `version: 1
scope:
  target_policy: enforce
  targets:
    - kind: challenge
      value: pwnable-login
  exclusions: []
execution:
  isolation: process
`
			if err := os.WriteFile(filepath.Join(root, "scope.yaml"), []byte(scope), 0o644); err != nil {
				t.Fatalf("write scope: %v", err)
			}
			manifest := fmt.Sprintf(`name: ctf-pack
version: "0.1.0"
capabilities:
  - ctf.challenge_classification
runner: command
run_command:
  - printf
  - "%s\n"
parser: ctf_parser
healthcheck: built_in
`, tc.argument)
			if err := os.WriteFile(filepath.Join(root, "toolchain", "ctf-pack", "manifest.yaml"), []byte(manifest), 0o644); err != nil {
				t.Fatalf("write manifest: %v", err)
			}

			_, err := RunWorkspace(ctx, root)
			if err == nil {
				t.Fatalf("expected embedded out-of-scope challenge argument to fail")
			}
			if !strings.Contains(err.Error(), "out-of-scope challenge argument") {
				t.Fatalf("expected out-of-scope challenge argument error, got %v", err)
			}
		})
	}
}

func TestRunWorkspaceCanDisableRunCommandChallengeArgumentPolicy(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	scope := `version: 1
scope:
  target_policy: disabled
  targets:
    - kind: challenge
      value: pwnable-login
  exclusions: []
execution:
  isolation: process
`
	if err := os.WriteFile(filepath.Join(root, "scope.yaml"), []byte(scope), 0o644); err != nil {
		t.Fatalf("write scope: %v", err)
	}
	manifest := `name: ctf-pack
version: "0.1.0"
capabilities:
  - ctf.challenge_classification
runner: command
run_command:
  - printf
  - "solve challenge:other-login\n"
parser: ctf_parser
healthcheck: built_in
`
	if err := os.WriteFile(filepath.Join(root, "toolchain", "ctf-pack", "manifest.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	result, err := RunWorkspace(ctx, root)
	if err != nil {
		t.Fatalf("run workspace: %v", err)
	}
	store, err := eventlog.OpenSQLite(filepath.Join(root, "replay", "events.db"))
	if err != nil {
		t.Fatalf("open event log: %v", err)
	}
	defer store.Close()
	events, err := store.Replay(ctx, result.RunID)
	if err != nil {
		t.Fatalf("replay events: %v", err)
	}
	for _, event := range events {
		if event.Type != eventlog.EventTool {
			continue
		}
		var payload struct {
			Capability      string `json:"capability"`
			ExecutionStatus string `json:"execution_status"`
			Transcript      string `json:"transcript"`
		}
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			t.Fatalf("decode tool payload: %v", err)
		}
		if payload.Capability != "ctf.challenge_classification" {
			continue
		}
		if payload.ExecutionStatus != "completed" || !strings.Contains(payload.Transcript, "solve challenge:other-login") {
			t.Fatalf("expected disabled target policy to allow challenge argument, got %#v", payload)
		}
		return
	}
	t.Fatalf("expected ctf.challenge_classification command event")
}

func TestRunWorkspaceCommandRunnerUsesScopedWorkerEnvironment(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	scope := `version: 1
scope:
  targets:
    - kind: url
      value: https://app.example
  exclusions: []
`
	if err := os.WriteFile(filepath.Join(root, "scope.yaml"), []byte(scope), 0o644); err != nil {
		t.Fatalf("write scope: %v", err)
	}
	manifest := `name: web-api-pack
version: "0.1.0"
capabilities:
  - web.discovery
runner: command
run_command:
  - env
parser: web_api_parser
healthcheck: built_in
`
	if err := os.WriteFile(filepath.Join(root, "toolchain", "web-api-pack", "manifest.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	result, err := RunWorkspace(ctx, root)
	if err != nil {
		t.Fatalf("run workspace: %v", err)
	}
	store, err := eventlog.OpenSQLite(filepath.Join(root, "replay", "events.db"))
	if err != nil {
		t.Fatalf("open event log: %v", err)
	}
	defer store.Close()
	events, err := store.Replay(ctx, result.RunID)
	if err != nil {
		t.Fatalf("replay events: %v", err)
	}
	for _, event := range events {
		if event.Type != eventlog.EventTool {
			continue
		}
		var payload struct {
			Capability string `json:"capability"`
			Transcript string `json:"transcript"`
			AttemptDir string `json:"attempt_dir"`
		}
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			t.Fatalf("decode tool payload: %v", err)
		}
		if payload.Capability != "web.discovery" {
			continue
		}
		for _, want := range []string{
			"OURO_WORKSPACE=" + root,
			"OURO_RUN_ID=" + result.RunID,
			"OURO_ATTEMPT_DIR=" + payload.AttemptDir,
			"OURO_TARGET=https://app.example",
			"OURO_TARGET_KIND=url",
		} {
			if !strings.Contains(payload.Transcript, want) {
				t.Fatalf("expected worker env to contain %q, got:\n%s", want, payload.Transcript)
			}
		}
		if strings.Contains(payload.Transcript, "HOME=") {
			t.Fatalf("worker env must not inherit HOME, got:\n%s", payload.Transcript)
		}
		return
	}
	t.Fatalf("expected web.discovery execution event")
}

func TestRunWorkspaceCommandRunnerCanDisableIsolationForLegacyTools(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	t.Setenv("OURO_LEGACY_ENV", "legacy-visible")
	scope := `version: 1
scope:
  target_policy: enforce
  targets:
    - kind: url
      value: https://app.example
  exclusions: []
execution:
  isolation: disabled
`
	if err := os.WriteFile(filepath.Join(root, "scope.yaml"), []byte(scope), 0o644); err != nil {
		t.Fatalf("write scope: %v", err)
	}
	manifest := `name: web-api-pack
version: "0.1.0"
capabilities:
  - web.discovery
runner: command
run_command:
  - env
parser: web_api_parser
healthcheck: built_in
`
	if err := os.WriteFile(filepath.Join(root, "toolchain", "web-api-pack", "manifest.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	result, err := RunWorkspace(ctx, root)
	if err != nil {
		t.Fatalf("run workspace: %v", err)
	}
	store, err := eventlog.OpenSQLite(filepath.Join(root, "replay", "events.db"))
	if err != nil {
		t.Fatalf("open event log: %v", err)
	}
	defer store.Close()
	events, err := store.Replay(ctx, result.RunID)
	if err != nil {
		t.Fatalf("replay events: %v", err)
	}
	for _, event := range events {
		if event.Type != eventlog.EventTool {
			continue
		}
		var payload struct {
			Capability      string `json:"capability"`
			ExecutionStatus string `json:"execution_status"`
			Isolation       string `json:"isolation"`
			Transcript      string `json:"transcript"`
		}
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			t.Fatalf("decode tool payload: %v", err)
		}
		if payload.Capability != "web.discovery" {
			continue
		}
		if payload.ExecutionStatus != "completed" || payload.Isolation != "disabled" {
			t.Fatalf("expected disabled isolation command event, got %#v", payload)
		}
		if !strings.Contains(payload.Transcript, "OURO_LEGACY_ENV=legacy-visible") {
			t.Fatalf("disabled isolation should inherit host environment, got:\n%s", payload.Transcript)
		}
		return
	}
	t.Fatalf("expected web.discovery execution event")
}

func TestRunWorkspaceCommandRunnerUsesContainerIsolation(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	engine := filepath.Join(t.TempDir(), "fake-container-engine")
	engineScript := `#!/bin/sh
printf 'engine-cwd=%s\n' "$PWD"
printf 'engine-args=%s\n' "$*"
`
	if err := os.WriteFile(engine, []byte(engineScript), 0o755); err != nil {
		t.Fatalf("write fake engine: %v", err)
	}
	scope := `version: 1
scope:
  target_policy: enforce
  targets:
    - kind: url
      value: https://app.example
  exclusions: []
execution:
  isolation: container
  container:
    engine: ` + engine + `
    image: ouro-worker:test
`
	if err := os.WriteFile(filepath.Join(root, "scope.yaml"), []byte(scope), 0o644); err != nil {
		t.Fatalf("write scope: %v", err)
	}
	manifest := `name: web-api-pack
version: "0.1.0"
capabilities:
  - web.discovery
runner: command
run_command:
  - missing-inner-binary
  - --target
  - "{{target}}"
parser: web_api_parser
healthcheck: built_in
`
	if err := os.WriteFile(filepath.Join(root, "toolchain", "web-api-pack", "manifest.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	result, err := RunWorkspace(ctx, root)
	if err != nil {
		t.Fatalf("run workspace with container isolation: %v", err)
	}
	store, err := eventlog.OpenSQLite(filepath.Join(root, "replay", "events.db"))
	if err != nil {
		t.Fatalf("open event log: %v", err)
	}
	defer store.Close()
	events, err := store.Replay(ctx, result.RunID)
	if err != nil {
		t.Fatalf("replay events: %v", err)
	}
	for _, event := range events {
		if event.Type != eventlog.EventTool {
			continue
		}
		var payload struct {
			Capability      string `json:"capability"`
			ExecutionStatus string `json:"execution_status"`
			Isolation       string `json:"isolation"`
			ContainerEngine string `json:"container_engine"`
			ContainerImage  string `json:"container_image"`
			Transcript      string `json:"transcript"`
			AttemptDir      string `json:"attempt_dir"`
		}
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			t.Fatalf("decode tool payload: %v", err)
		}
		if payload.Capability != "web.discovery" {
			continue
		}
		if payload.ExecutionStatus != "completed" || payload.Isolation != "container" {
			t.Fatalf("expected container isolation command event, got %#v", payload)
		}
		if payload.ContainerEngine != engine || payload.ContainerImage != "ouro-worker:test" {
			t.Fatalf("expected container engine/image audit fields, got %#v", payload)
		}
		for _, want := range []string{
			"engine-args=run --rm",
			"--volume " + root + ":/workspace",
			"--workdir /workspace/attempts/" + result.RunID,
			"--env OURO_WORKSPACE=/workspace",
			"--env OURO_ATTEMPT_DIR=/workspace/attempts/" + result.RunID,
			"--env OURO_TARGET=https://app.example",
			"--env OURO_TARGET_KIND=url",
			"ouro-worker:test missing-inner-binary --target https://app.example",
		} {
			if !strings.Contains(payload.Transcript, want) {
				t.Fatalf("expected container engine transcript to contain %q, got:\n%s", want, payload.Transcript)
			}
		}
		if !strings.HasPrefix(payload.AttemptDir, filepath.Join(root, "attempts", result.RunID)) {
			t.Fatalf("expected host attempt dir inside workspace attempts, got %s", payload.AttemptDir)
		}
		return
	}
	t.Fatalf("expected web.discovery execution event")
}

func TestRunWorkspaceExecutesManagedBinaryBuilderThroughContainerIsolation(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	engine := filepath.Join(t.TempDir(), "fake-container-engine")
	engineScript := `#!/bin/sh
printf 'engine-cwd=%s\n' "$PWD"
printf 'engine-args=%s\n' "$*"
`
	if err := os.WriteFile(engine, []byte(engineScript), 0o755); err != nil {
		t.Fatalf("write fake engine: %v", err)
	}
	scope := `version: 1
scope:
  target_policy: enforce
  targets:
    - kind: url
      value: https://app.example
  exclusions: []
execution:
  isolation: container
  container:
    engine: ` + engine + `
    image: ouro-worker:test
`
	if err := os.WriteFile(filepath.Join(root, "scope.yaml"), []byte(scope), 0o644); err != nil {
		t.Fatalf("write scope: %v", err)
	}

	result, err := RunWorkspace(ctx, root)
	if err != nil {
		t.Fatalf("run workspace with container managed binary builder: %v", err)
	}
	store, err := eventlog.OpenSQLite(filepath.Join(root, "replay", "events.db"))
	if err != nil {
		t.Fatalf("open event log: %v", err)
	}
	defer store.Close()
	events, err := store.Replay(ctx, result.RunID)
	if err != nil {
		t.Fatalf("replay events: %v", err)
	}
	for _, event := range events {
		if event.Type != eventlog.EventTool {
			continue
		}
		var payload struct {
			ExecutionStatus string `json:"execution_status"`
			Isolation       string `json:"isolation"`
			ContainerEngine string `json:"container_engine"`
			ContainerImage  string `json:"container_image"`
			Transcript      string `json:"transcript"`
		}
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			t.Fatalf("decode tool payload: %v", err)
		}
		if !strings.Contains(payload.Transcript, "ouro-worker:test httpx -json -u https://app.example") {
			continue
		}
		if payload.ExecutionStatus != "completed" || payload.Isolation != "container" {
			t.Fatalf("expected completed container managed execution, got %#v", payload)
		}
		if payload.ContainerEngine != engine || payload.ContainerImage != "ouro-worker:test" {
			t.Fatalf("expected container audit fields, got %#v", payload)
		}
		for _, want := range []string{
			"engine-args=run --rm",
			"--volume " + root + ":/workspace",
			"--env OURO_TOOL_ID=httpx",
			"--env OURO_OPERATION=http_probe",
			"ouro-worker:test httpx -json -u https://app.example",
		} {
			if !strings.Contains(payload.Transcript, want) {
				t.Fatalf("expected container managed execution transcript to contain %q, got:\n%s", want, payload.Transcript)
			}
		}
		return
	}
	t.Fatalf("expected httpx managed binary container execution event")
}

func TestRunWorkspaceCommandRunnerAppliesContainerSandboxProfile(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	engine := filepath.Join(t.TempDir(), "fake-container-engine")
	engineScript := `#!/bin/sh
printf 'engine-args=%s\n' "$*"
`
	if err := os.WriteFile(engine, []byte(engineScript), 0o755); err != nil {
		t.Fatalf("write fake engine: %v", err)
	}
	scope := `version: 1
scope:
  target_policy: enforce
  targets:
    - kind: url
      value: https://app.example
  exclusions: []
execution:
  isolation: container
  container:
    engine: ` + engine + `
    image: ouro-worker:test
    network: none
    user: "1000:1000"
    read_only_rootfs: true
    no_new_privileges: true
    cap_drop:
      - ALL
      - NET_RAW
    memory: 512m
    cpus: "1.5"
    pids_limit: 128
    seccomp: default
    apparmor: ouro-default
    tmpfs:
      - /tmp:rw,nosuid,nodev,noexec,size=64m
`
	if err := os.WriteFile(filepath.Join(root, "scope.yaml"), []byte(scope), 0o644); err != nil {
		t.Fatalf("write scope: %v", err)
	}
	manifest := `name: web-api-pack
version: "0.1.0"
capabilities:
  - web.discovery
runner: command
run_command:
  - missing-inner-binary
  - --target
  - "{{target}}"
parser: web_api_parser
healthcheck: built_in
`
	if err := os.WriteFile(filepath.Join(root, "toolchain", "web-api-pack", "manifest.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	result, err := RunWorkspace(ctx, root)
	if err != nil {
		t.Fatalf("run workspace with container sandbox profile: %v", err)
	}
	store, err := eventlog.OpenSQLite(filepath.Join(root, "replay", "events.db"))
	if err != nil {
		t.Fatalf("open event log: %v", err)
	}
	defer store.Close()
	events, err := store.Replay(ctx, result.RunID)
	if err != nil {
		t.Fatalf("replay events: %v", err)
	}
	for _, event := range events {
		if event.Type != eventlog.EventTool {
			continue
		}
		var payload struct {
			Capability               string   `json:"capability"`
			ContainerNetwork         string   `json:"container_network"`
			ContainerReadOnlyRootFS  bool     `json:"container_read_only_rootfs"`
			ContainerNoNewPrivileges bool     `json:"container_no_new_privileges"`
			ContainerCapDrop         []string `json:"container_cap_drop"`
			Transcript               string   `json:"transcript"`
		}
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			t.Fatalf("decode tool payload: %v", err)
		}
		if payload.Capability != "web.discovery" {
			continue
		}
		if payload.ContainerNetwork != "none" || !payload.ContainerReadOnlyRootFS || !payload.ContainerNoNewPrivileges {
			t.Fatalf("expected container sandbox profile audit fields, got %#v", payload)
		}
		if strings.Join(payload.ContainerCapDrop, ",") != "ALL,NET_RAW" {
			t.Fatalf("expected container cap-drop audit fields, got %#v", payload.ContainerCapDrop)
		}
		for _, want := range []string{
			"--network none",
			"--user 1000:1000",
			"--read-only",
			"--security-opt no-new-privileges",
			"--cap-drop ALL",
			"--cap-drop NET_RAW",
			"--memory 512m",
			"--cpus 1.5",
			"--pids-limit 128",
			"--security-opt seccomp=default",
			"--security-opt apparmor=ouro-default",
			"--tmpfs /tmp:rw,nosuid,nodev,noexec,size=64m",
			"ouro-worker:test missing-inner-binary --target https://app.example",
		} {
			if !strings.Contains(payload.Transcript, want) {
				t.Fatalf("expected container sandbox args to contain %q, got:\n%s", want, payload.Transcript)
			}
		}
		return
	}
	t.Fatalf("expected web.discovery execution event")
}

func TestRunWorkspacePackPolicyOverridesContainerProfile(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	engine := filepath.Join(t.TempDir(), "fake-container-engine")
	engineScript := `#!/bin/sh
printf 'engine-args=%s\n' "$*"
`
	if err := os.WriteFile(engine, []byte(engineScript), 0o755); err != nil {
		t.Fatalf("write fake engine: %v", err)
	}
	scope := `version: 1
scope:
  target_policy: enforce
  targets:
    - kind: url
      value: https://app.example
  exclusions: []
execution:
  isolation: container
  container:
    engine: ` + engine + `
    image: default-worker:test
    network: bridge
    memory: 1g
  pack_policies:
    - pack: web-api-pack
      mode: enabled
      container:
        image: web-worker:test
        network: none
        memory: 256m
        cpus: "0.5"
`
	if err := os.WriteFile(filepath.Join(root, "scope.yaml"), []byte(scope), 0o644); err != nil {
		t.Fatalf("write scope: %v", err)
	}
	manifest := `name: web-api-pack
version: "0.1.0"
capabilities:
  - web.discovery
runner: command
run_command:
  - probe-web
  - "{{target}}"
parser: web_api_parser
healthcheck: built_in
`
	if err := os.WriteFile(filepath.Join(root, "toolchain", "web-api-pack", "manifest.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	result, err := RunWorkspace(ctx, root)
	if err != nil {
		t.Fatalf("run workspace with pack container policy: %v", err)
	}
	store, err := eventlog.OpenSQLite(filepath.Join(root, "replay", "events.db"))
	if err != nil {
		t.Fatalf("open event log: %v", err)
	}
	defer store.Close()
	events, err := store.Replay(ctx, result.RunID)
	if err != nil {
		t.Fatalf("replay events: %v", err)
	}
	for _, event := range events {
		if event.Type != eventlog.EventTool {
			continue
		}
		var payload struct {
			Capability       string `json:"capability"`
			ExecutionStatus  string `json:"execution_status"`
			Isolation        string `json:"isolation"`
			ContainerEngine  string `json:"container_engine"`
			ContainerImage   string `json:"container_image"`
			ContainerNetwork string `json:"container_network"`
			ContainerMemory  string `json:"container_memory"`
			ContainerCPUs    string `json:"container_cpus"`
			Transcript       string `json:"transcript"`
		}
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			t.Fatalf("decode tool payload: %v", err)
		}
		if payload.Capability != "web.discovery" {
			continue
		}
		if payload.ExecutionStatus != "completed" || payload.Isolation != "container" {
			t.Fatalf("expected completed container execution, got %#v", payload)
		}
		if payload.ContainerEngine != engine || payload.ContainerImage != "web-worker:test" || payload.ContainerNetwork != "none" {
			t.Fatalf("expected pack container policy to override image/network while inheriting engine, got %#v", payload)
		}
		if payload.ContainerMemory != "256m" || payload.ContainerCPUs != "0.5" {
			t.Fatalf("expected pack container resource overrides, got %#v", payload)
		}
		for _, want := range []string{
			"--network none",
			"--memory 256m",
			"--cpus 0.5",
			"web-worker:test probe-web https://app.example",
		} {
			if !strings.Contains(payload.Transcript, want) {
				t.Fatalf("expected container transcript to contain %q, got:\n%s", want, payload.Transcript)
			}
		}
		if strings.Contains(payload.Transcript, "default-worker:test") || strings.Contains(payload.Transcript, "--memory 1g") {
			t.Fatalf("pack container override must not use inherited overridden values, got:\n%s", payload.Transcript)
		}
		return
	}
	t.Fatalf("expected web.discovery execution event")
}

func TestRunWorkspacePersistsFailedToolEventWhenCommandTimesOut(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	scope := `version: 1
scope:
  targets:
    - kind: url
      value: https://app.example
  exclusions: []
`
	if err := os.WriteFile(filepath.Join(root, "scope.yaml"), []byte(scope), 0o644); err != nil {
		t.Fatalf("write scope: %v", err)
	}
	manifest := `name: web-api-pack
version: "0.1.0"
capabilities:
  - web.discovery
runner: command
run_command:
  - sleep
  - "1"
run_timeout_ms: 1
parser: web_api_parser
healthcheck: built_in
`
	if err := os.WriteFile(filepath.Join(root, "toolchain", "web-api-pack", "manifest.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	observedEvents := []eventlog.Event{}
	observedSnapshots := []eventlog.ReplaySnapshot{}
	_, err := RunWorkspaceWithOptions(ctx, root, RunOptions{
		OnEvent: func(event eventlog.Event) error {
			observedEvents = append(observedEvents, event)
			return nil
		},
		OnSnapshot: func(snapshot eventlog.ReplaySnapshot) error {
			observedSnapshots = append(observedSnapshots, snapshot)
			return nil
		},
	})
	if err == nil {
		t.Fatalf("expected timed out command to fail")
	}
	if !strings.Contains(err.Error(), "timed out") && !strings.Contains(err.Error(), "deadline exceeded") {
		t.Fatalf("expected timeout error, got %v", err)
	}
	if len(observedEvents) == 0 {
		t.Fatalf("expected persisted events before timeout")
	}
	if len(observedSnapshots) == 0 {
		t.Fatalf("expected persisted snapshots before timeout")
	}
	if observedSnapshots[len(observedSnapshots)-1].Status.ToolHealth != "failed" {
		t.Fatalf("expected final failed snapshot tool health, got %#v", observedSnapshots[len(observedSnapshots)-1].Status)
	}

	store, err := eventlog.OpenSQLite(filepath.Join(root, "replay", "events.db"))
	if err != nil {
		t.Fatalf("open event log: %v", err)
	}
	defer store.Close()
	events, err := store.Replay(ctx, observedEvents[0].RunID)
	if err != nil {
		t.Fatalf("replay events: %v", err)
	}

	var failed struct {
		Action          string `json:"action"`
		Capability      string `json:"capability"`
		ExecutionID     string `json:"execution_id"`
		ExecutionStatus string `json:"execution_status"`
		Transcript      string `json:"transcript"`
		AttemptDir      string `json:"attempt_dir"`
	}
	for _, event := range events {
		if event.Type == eventlog.EventAttempt {
			var payload struct {
				Kind    string `json:"kind"`
				StageID string `json:"stage_id"`
			}
			if err := json.Unmarshal(event.Payload, &payload); err != nil {
				t.Fatalf("decode stage payload: %v", err)
			}
			if payload.Kind == "stage" && payload.StageID == "report-projection" {
				t.Fatalf("report projection must not start after failed tool execution")
			}
		}
		if event.Type != eventlog.EventTool {
			continue
		}
		var payload struct {
			Action          string `json:"action"`
			Capability      string `json:"capability"`
			ExecutionID     string `json:"execution_id"`
			ExecutionStatus string `json:"execution_status"`
			Transcript      string `json:"transcript"`
			AttemptDir      string `json:"attempt_dir"`
		}
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			t.Fatalf("decode tool payload: %v", err)
		}
		if payload.Action == string(compiler.ActionDiscoverAssets) && payload.Capability == "web.discovery" {
			failed = payload
		}
	}
	if failed.ExecutionStatus != "failed" {
		t.Fatalf("expected failed command event, got %#v", failed)
	}
	if failed.ExecutionID == "" || failed.AttemptDir == "" {
		t.Fatalf("expected execution id and attempt dir in failed event, got %#v", failed)
	}
	if !strings.HasPrefix(failed.AttemptDir, filepath.Join(root, "attempts", observedEvents[0].RunID)) {
		t.Fatalf("expected attempt dir inside workspace attempts, got %#v", failed)
	}
	if !strings.Contains(failed.Transcript, "timed out") && !strings.Contains(failed.Transcript, "deadline exceeded") {
		t.Fatalf("expected timeout transcript, got %#v", failed)
	}
}

func TestRunWorkspaceStreamsPersistedEventsAndSnapshots(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}

	observedEvents := []eventlog.Event{}
	observedSnapshots := []eventlog.ReplaySnapshot{}
	result, err := RunWorkspaceWithOptions(ctx, root, RunOptions{
		OnEvent: func(event eventlog.Event) error {
			observedEvents = append(observedEvents, event)
			return nil
		},
		OnSnapshot: func(snapshot eventlog.ReplaySnapshot) error {
			observedSnapshots = append(observedSnapshots, snapshot)
			return nil
		},
	})
	if err != nil {
		t.Fatalf("run workspace with observers: %v", err)
	}
	if len(observedEvents) != result.EventCount {
		t.Fatalf("expected %d observed events, got %d", result.EventCount, len(observedEvents))
	}
	if len(observedSnapshots) != result.EventCount {
		t.Fatalf("expected %d observed snapshots, got %d", result.EventCount, len(observedSnapshots))
	}
	if observedEvents[0].ID == 0 || observedEvents[0].CreatedAt.IsZero() || observedEvents[0].Type != eventlog.EventVerification {
		t.Fatalf("expected first observed event to be persisted workspace safety audit, got %#v", observedEvents[0])
	}
	firstSafety := decodeWorkspaceSafetyPayloadForTest(t, observedEvents[0])
	if firstSafety.Kind != "workspace_safety" || firstSafety.Status != "ready" || firstSafety.StrictGate.Mode != "audit" {
		t.Fatalf("expected first observed event to audit workspace safety, got %#v", firstSafety)
	}
	if len(observedEvents) < 2 || observedEvents[1].Type != eventlog.EventAttempt {
		t.Fatalf("expected second observed event to be persisted stage attempt, got %#v", observedEvents)
	}
	var firstStage struct {
		Kind    string `json:"kind"`
		StageID string `json:"stage_id"`
		Status  string `json:"status"`
	}
	if err := json.Unmarshal(observedEvents[1].Payload, &firstStage); err != nil {
		t.Fatalf("decode first stage payload: %v", err)
	}
	if firstStage.Kind != "stage" || firstStage.StageID != "workspace-bootstrap" || firstStage.Status != "started" {
		t.Fatalf("expected second observed event to start workspace bootstrap, got %#v", firstStage)
	}
	lastEvent := observedEvents[len(observedEvents)-1]
	if lastEvent.Type != eventlog.EventAttempt || lastEvent.RunID != result.RunID {
		t.Fatalf("expected final observed event to be report stage completion for result run, got %#v", lastEvent)
	}
	var lastStage struct {
		Kind    string `json:"kind"`
		StageID string `json:"stage_id"`
		Status  string `json:"status"`
	}
	if err := json.Unmarshal(lastEvent.Payload, &lastStage); err != nil {
		t.Fatalf("decode last stage payload: %v", err)
	}
	if lastStage.Kind != "stage" || lastStage.StageID != "report-projection" || lastStage.Status != "completed" {
		t.Fatalf("expected final observed event to complete report projection, got %#v", lastStage)
	}
	for i := 1; i < len(observedEvents); i++ {
		if observedEvents[i].ID <= observedEvents[i-1].ID {
			t.Fatalf("expected observed event ids to increase, got %#v", observedEvents)
		}
	}
	lastSnapshot := observedSnapshots[len(observedSnapshots)-1]
	if lastSnapshot.RunID != result.RunID || lastSnapshot.Status.Model != "offline-model" || lastSnapshot.Status.ToolHealth != "ready" {
		t.Fatalf("expected final snapshot to match run result, got %#v", lastSnapshot.Status)
	}
}

func TestRunWorkspaceRejectsInvalidWorkspace(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	if err := os.Remove(filepath.Join(root, "providers.yaml")); err != nil {
		t.Fatalf("remove providers: %v", err)
	}

	_, err := RunWorkspace(context.Background(), root)
	if err == nil {
		t.Fatalf("expected invalid workspace to fail")
	}
	if !strings.Contains(err.Error(), "providers.yaml") {
		t.Fatalf("expected providers.yaml error, got %v", err)
	}
}

func TestRunWorkspaceRejectsUnsupportedProviderAdapter(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	providers := `version: 1
providers:
  - name: remote
    type: remote
    model: external-model
    enabled: true
`
	if err := os.WriteFile(filepath.Join(root, "providers.yaml"), []byte(providers), 0o644); err != nil {
		t.Fatalf("write providers: %v", err)
	}

	_, err := RunWorkspace(context.Background(), root)
	if err == nil {
		t.Fatalf("expected unsupported provider adapter to fail")
	}
	if !strings.Contains(err.Error(), "provider adapter") {
		t.Fatalf("expected provider adapter error, got %v", err)
	}
}

func TestRunWorkspacePreflightsOpenAICompatibleProvider(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		if r.Method != http.MethodGet {
			t.Fatalf("expected GET preflight, got %s", r.Method)
		}
		if r.URL.Path != "/v1/models/gpt-test" {
			t.Fatalf("expected escaped model endpoint, got %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer secret-value" {
			t.Fatalf("expected bearer header, got %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"gpt-test"}`))
	}))
	defer server.Close()
	t.Setenv("OURO_TEST_PROVIDER_KEY", "secret-value")
	providers := fmt.Sprintf(`version: 1
providers:
  - name: local-openai
    type: openai_compatible
    model: gpt-test
    base_url: %s/v1
    api_key_env: OURO_TEST_PROVIDER_KEY
    timeout_ms: 5000
    enabled: true
`, server.URL)
	if err := os.WriteFile(filepath.Join(root, "providers.yaml"), []byte(providers), 0o644); err != nil {
		t.Fatalf("write providers: %v", err)
	}

	result, err := RunWorkspaceWithOptions(context.Background(), root, RunOptions{DeferReportProjection: true})
	if err != nil {
		t.Fatalf("run workspace: %v", err)
	}
	if !called {
		t.Fatalf("expected provider preflight request")
	}
	if !strings.Contains(result.StatusLine, "model:gpt-test") {
		t.Fatalf("expected configured model in status line, got %q", result.StatusLine)
	}
}

func TestRunWorkspaceRejectsOpenAICompatibleProviderMissingAPIKeyEnv(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	providers := fmt.Sprintf(`version: 1
providers:
  - name: local-openai
    type: openai_compatible
    model: gpt-test
    base_url: %s
    api_key_env: OURO_MISSING_PROVIDER_KEY
    enabled: true
`, server.URL)
	if err := os.WriteFile(filepath.Join(root, "providers.yaml"), []byte(providers), 0o644); err != nil {
		t.Fatalf("write providers: %v", err)
	}

	_, err := RunWorkspace(context.Background(), root)
	if err == nil {
		t.Fatalf("expected missing api key env to fail")
	}
	if called {
		t.Fatalf("expected missing api key env to fail before HTTP preflight")
	}
	if !strings.Contains(err.Error(), "OURO_MISSING_PROVIDER_KEY") || !strings.Contains(err.Error(), "local-openai") {
		t.Fatalf("expected provider/env error, got %v", err)
	}
}

func TestRunWorkspaceRejectsOpenAICompatibleProviderNon2xxPreflight(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "secret-value should not leak", http.StatusServiceUnavailable)
	}))
	defer server.Close()
	t.Setenv("OURO_TEST_PROVIDER_KEY", "secret-value")
	providers := fmt.Sprintf(`version: 1
providers:
  - name: local-openai
    type: openai_compatible
    model: gpt-test
    base_url: %s
    api_key_env: OURO_TEST_PROVIDER_KEY
    enabled: true
`, server.URL)
	if err := os.WriteFile(filepath.Join(root, "providers.yaml"), []byte(providers), 0o644); err != nil {
		t.Fatalf("write providers: %v", err)
	}

	_, err := RunWorkspace(context.Background(), root)
	if err == nil {
		t.Fatalf("expected non-2xx provider preflight to fail")
	}
	if !strings.Contains(err.Error(), "local-openai") || !strings.Contains(err.Error(), "status 503") {
		t.Fatalf("expected provider/status error, got %v", err)
	}
	for _, secret := range []string{"secret-value", "should not leak"} {
		if strings.Contains(err.Error(), secret) {
			t.Fatalf("provider error leaked response or secret material: %v", err)
		}
	}
}

func TestRunWorkspaceRejectsOpenAICompatibleProviderInvalidBaseURL(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	providers := `version: 1
providers:
  - name: local-openai
    type: openai_compatible
    model: gpt-test
    base_url: /not-absolute
    enabled: true
`
	if err := os.WriteFile(filepath.Join(root, "providers.yaml"), []byte(providers), 0o644); err != nil {
		t.Fatalf("write providers: %v", err)
	}

	_, err := RunWorkspace(context.Background(), root)
	if err == nil {
		t.Fatalf("expected invalid base_url to fail")
	}
	if !strings.Contains(err.Error(), "base_url") || !strings.Contains(err.Error(), "local-openai") {
		t.Fatalf("expected provider base_url error, got %v", err)
	}
}

func TestRunWorkspaceRejectsOpenAICompatibleProviderBaseURLUserinfo(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	providers := `version: 1
providers:
  - name: local-openai
    type: openai_compatible
    model: gpt-test
    base_url: http://user:pass@127.0.0.1:1/v1
    enabled: true
`
	if err := os.WriteFile(filepath.Join(root, "providers.yaml"), []byte(providers), 0o644); err != nil {
		t.Fatalf("write providers: %v", err)
	}

	_, err := RunWorkspace(context.Background(), root)
	if err == nil {
		t.Fatalf("expected userinfo base_url to fail")
	}
	if !strings.Contains(err.Error(), "base_url") || !strings.Contains(err.Error(), "userinfo") || !strings.Contains(err.Error(), "local-openai") {
		t.Fatalf("expected provider base_url userinfo error, got %v", err)
	}
	for _, secret := range []string{"user:pass", "pass@127"} {
		if strings.Contains(err.Error(), secret) {
			t.Fatalf("provider base_url error leaked userinfo: %v", err)
		}
	}
}

func TestRunWorkspaceRejectsOpenAICompatibleProviderNegativeCostMetadata(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	providers := fmt.Sprintf(`version: 1
providers:
  - name: local-openai
    type: openai_compatible
    model: gpt-test
    base_url: %s
    context_window_tokens: -1
    input_price_per_million_tokens: -0.01
    output_price_per_million_tokens: -0.01
    enabled: true
`, server.URL)
	if err := os.WriteFile(filepath.Join(root, "providers.yaml"), []byte(providers), 0o644); err != nil {
		t.Fatalf("write providers: %v", err)
	}

	_, err := RunWorkspace(context.Background(), root)
	if err == nil {
		t.Fatalf("expected negative provider cost metadata to fail")
	}
	if called {
		t.Fatalf("expected invalid provider metadata to fail before HTTP")
	}
	if !strings.Contains(err.Error(), "context_window_tokens") {
		t.Fatalf("expected context window validation error, got %v", err)
	}
}

func TestRunWorkspaceRejectsInvalidScope(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	scope := `version: 1
scope:
  targets:
    - kind: unsupported
      value: sample
  exclusions: []
`
	if err := os.WriteFile(filepath.Join(root, "scope.yaml"), []byte(scope), 0o644); err != nil {
		t.Fatalf("write scope: %v", err)
	}

	_, err := RunWorkspace(context.Background(), root)
	if err == nil {
		t.Fatalf("expected invalid scope to fail")
	}
	if !strings.Contains(err.Error(), "scope.yaml") {
		t.Fatalf("expected scope.yaml error, got %v", err)
	}
}

func TestValidateScopeConfigAcceptsArtifactTargets(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	scope := `version: 1
scope:
  target_policy: enforce
  targets:
    - kind: artifact
      value: samples/app.apk
  exclusions: []
execution:
  isolation: process
`
	scopePath := filepath.Join(root, "scope.yaml")
	if err := os.WriteFile(scopePath, []byte(scope), 0o644); err != nil {
		t.Fatalf("write scope: %v", err)
	}
	config, err := LoadScopeConfig(scopePath)
	if err != nil {
		t.Fatalf("load scope: %v", err)
	}

	if err := ValidateScopeConfig(config); err != nil {
		t.Fatalf("expected artifact scope target to validate: %v", err)
	}
}

func TestRunWorkspaceRejectsInvalidExecutionIsolationMode(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	scope := `version: 1
scope:
  targets: []
  exclusions: []
execution:
  isolation: virtual-machine
`
	if err := os.WriteFile(filepath.Join(root, "scope.yaml"), []byte(scope), 0o644); err != nil {
		t.Fatalf("write scope: %v", err)
	}

	_, err := RunWorkspace(context.Background(), root)
	if err == nil {
		t.Fatalf("expected invalid execution isolation to fail")
	}
	if !strings.Contains(err.Error(), "isolation") {
		t.Fatalf("expected isolation error, got %v", err)
	}
}

func TestRunWorkspaceRejectsNegativeToolConcurrency(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	scope := `version: 1
scope:
  targets: []
  exclusions: []
execution:
  tool_concurrency: -1
`
	if err := os.WriteFile(filepath.Join(root, "scope.yaml"), []byte(scope), 0o644); err != nil {
		t.Fatalf("write scope: %v", err)
	}

	_, err := RunWorkspace(ctx, root)
	if err == nil {
		t.Fatalf("expected negative tool concurrency to fail")
	}
	if !strings.Contains(err.Error(), "tool_concurrency") {
		t.Fatalf("expected tool_concurrency error, got %v", err)
	}
}

func TestRunWorkspaceRejectsInvalidPackExecutionPolicy(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	scope := `version: 1
scope:
  target_policy: enforce
  targets: []
  exclusions: []
execution:
  isolation: process
  pack_policies:
    - pack: web-api-pack
      mode: throttle
`
	if err := os.WriteFile(filepath.Join(root, "scope.yaml"), []byte(scope), 0o644); err != nil {
		t.Fatalf("write scope: %v", err)
	}

	_, err := RunWorkspace(context.Background(), root)
	if err == nil {
		t.Fatalf("expected invalid pack execution policy to fail")
	}
	if !strings.Contains(err.Error(), "pack policy") {
		t.Fatalf("expected pack policy error, got %v", err)
	}
}

func TestRunWorkspaceRejectsInvalidPackExecutionMaxConcurrency(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	scope := `version: 1
scope:
  target_policy: enforce
  targets: []
  exclusions: []
execution:
  isolation: process
  pack_policies:
    - pack: web-api-pack
      mode: enabled
      max_concurrency: -1
`
	if err := os.WriteFile(filepath.Join(root, "scope.yaml"), []byte(scope), 0o644); err != nil {
		t.Fatalf("write scope: %v", err)
	}

	_, err := RunWorkspace(context.Background(), root)
	if err == nil {
		t.Fatalf("expected invalid pack max_concurrency to fail")
	}
	if !strings.Contains(err.Error(), "max_concurrency") {
		t.Fatalf("expected max_concurrency error, got %v", err)
	}
}

func TestRunWorkspaceRejectsInvalidExecutionResourceLimits(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	scope := `version: 1
scope:
  target_policy: enforce
  targets: []
  exclusions: []
execution:
  isolation: process
  resource_limits:
    cpu_units: 1
    memory_mb: 512
  pack_policies:
    - pack: web-api-pack
      mode: enabled
      cpu_units: 2
      memory_mb: 256
`
	if err := os.WriteFile(filepath.Join(root, "scope.yaml"), []byte(scope), 0o644); err != nil {
		t.Fatalf("write scope: %v", err)
	}

	_, err := RunWorkspace(context.Background(), root)
	if err == nil {
		t.Fatalf("expected pack resource request above global limit to fail")
	}
	if !strings.Contains(err.Error(), "cpu_units") {
		t.Fatalf("expected cpu_units error, got %v", err)
	}
}

func TestRunWorkspaceRejectsContainerIsolationWithoutImage(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	scope := `version: 1
scope:
  targets: []
  exclusions: []
execution:
  isolation: container
  container:
    engine: docker
`
	if err := os.WriteFile(filepath.Join(root, "scope.yaml"), []byte(scope), 0o644); err != nil {
		t.Fatalf("write scope: %v", err)
	}

	_, err := RunWorkspace(context.Background(), root)
	if err == nil {
		t.Fatalf("expected missing container image to fail")
	}
	if !strings.Contains(err.Error(), "container image") {
		t.Fatalf("expected container image error, got %v", err)
	}
}

func TestRunWorkspaceRejectsInvalidContainerNetwork(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	scope := `version: 1
scope:
  targets: []
  exclusions: []
execution:
  isolation: container
  container:
    image: ouro-worker:test
    network: internet
`
	if err := os.WriteFile(filepath.Join(root, "scope.yaml"), []byte(scope), 0o644); err != nil {
		t.Fatalf("write scope: %v", err)
	}

	_, err := RunWorkspace(context.Background(), root)
	if err == nil {
		t.Fatalf("expected invalid container network to fail")
	}
	if !strings.Contains(err.Error(), "container network") {
		t.Fatalf("expected container network error, got %v", err)
	}
}

func TestRunWorkspaceRejectsMissingStorageDirectory(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	if err := os.Remove(filepath.Join(root, "replay")); err != nil {
		t.Fatalf("remove replay: %v", err)
	}

	_, err := RunWorkspace(context.Background(), root)
	if err == nil {
		t.Fatalf("expected missing storage directory to fail")
	}
	if !strings.Contains(err.Error(), "replay") {
		t.Fatalf("expected replay storage error, got %v", err)
	}
}

type recordingToolExecutionRunner struct {
	mu       sync.Mutex
	requests []ToolExecutionRequest
}

func (r *recordingToolExecutionRunner) Execute(ctx context.Context, request ToolExecutionRequest) (ToolExecutionResult, error) {
	if err := ctx.Err(); err != nil {
		return ToolExecutionResult{}, err
	}
	r.mu.Lock()
	r.requests = append(r.requests, request)
	r.mu.Unlock()
	return ToolExecutionResult{
		ExecutionID: "recorded-" + request.Node.ID,
		Status:      "completed",
		Transcript:  "recorded execution for " + request.Node.ManifestName,
	}, nil
}

type authorizationRemovingToolExecutionRunner struct {
	root     string
	requests []ToolExecutionRequest
}

func (r *authorizationRemovingToolExecutionRunner) Execute(ctx context.Context, request ToolExecutionRequest) (ToolExecutionResult, error) {
	if err := ctx.Err(); err != nil {
		return ToolExecutionResult{}, err
	}
	r.requests = append(r.requests, request)
	if err := os.WriteFile(filepath.Join(r.root, "authorization.yaml"), []byte("version: 1\nsafety_bypasses: []\n"), 0o644); err != nil {
		return ToolExecutionResult{}, err
	}
	return ToolExecutionResult{
		ExecutionID: "authorization-removed-" + request.Node.ID,
		Status:      "completed",
		Transcript:  "authorization removed before lease completion",
	}, nil
}

type concurrentToolExecutionRunner struct {
	mu        sync.Mutex
	requests  []ToolExecutionRequest
	active    int
	maxActive int
	delay     time.Duration
}

func (r *concurrentToolExecutionRunner) Execute(ctx context.Context, request ToolExecutionRequest) (ToolExecutionResult, error) {
	if err := ctx.Err(); err != nil {
		return ToolExecutionResult{}, err
	}
	r.mu.Lock()
	r.requests = append(r.requests, request)
	r.active++
	if r.active > r.maxActive {
		r.maxActive = r.active
	}
	r.mu.Unlock()

	select {
	case <-ctx.Done():
		r.mu.Lock()
		r.active--
		r.mu.Unlock()
		return ToolExecutionResult{}, ctx.Err()
	case <-time.After(r.delay):
	}

	r.mu.Lock()
	r.active--
	r.mu.Unlock()
	return ToolExecutionResult{
		ExecutionID: "concurrent-" + request.Node.ID,
		Status:      "completed",
		Transcript:  "concurrent execution for " + request.Node.ManifestName,
	}, nil
}

type packConcurrencyProbeRunner struct {
	mu              sync.Mutex
	requests        []ToolExecutionRequest
	active          int
	maxActive       int
	activeByPack    map[string]int
	maxActiveByPack map[string]int
	delay           time.Duration
}

func (r *packConcurrencyProbeRunner) Execute(ctx context.Context, request ToolExecutionRequest) (ToolExecutionResult, error) {
	if err := ctx.Err(); err != nil {
		return ToolExecutionResult{}, err
	}
	r.mu.Lock()
	if r.activeByPack == nil {
		r.activeByPack = map[string]int{}
	}
	if r.maxActiveByPack == nil {
		r.maxActiveByPack = map[string]int{}
	}
	r.requests = append(r.requests, request)
	r.active++
	if r.active > r.maxActive {
		r.maxActive = r.active
	}
	pack := request.Node.ManifestName
	r.activeByPack[pack]++
	if r.activeByPack[pack] > r.maxActiveByPack[pack] {
		r.maxActiveByPack[pack] = r.activeByPack[pack]
	}
	r.mu.Unlock()

	select {
	case <-ctx.Done():
		r.finish(request.Node.ManifestName)
		return ToolExecutionResult{}, ctx.Err()
	case <-time.After(r.delay):
	}

	r.finish(request.Node.ManifestName)
	return ToolExecutionResult{
		ExecutionID: "pack-concurrency-" + request.Node.ID,
		Status:      "completed",
		Transcript:  "pack concurrency probe for " + request.Node.ManifestName,
	}, nil
}

func (r *packConcurrencyProbeRunner) finish(pack string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.active--
	r.activeByPack[pack]--
}

type resourceLeaseProbeRunner struct {
	mu              sync.Mutex
	requests        []ToolExecutionRequest
	active          int
	maxActive       int
	activeCPU       int
	maxActiveCPU    int
	activeMemory    int
	maxActiveMemory int
	cpuByPack       map[string]int
	memoryByPack    map[string]int
	delay           time.Duration
}

func (r *resourceLeaseProbeRunner) Execute(ctx context.Context, request ToolExecutionRequest) (ToolExecutionResult, error) {
	if err := ctx.Err(); err != nil {
		return ToolExecutionResult{}, err
	}
	cpu := r.cpuByPack[request.Node.ManifestName]
	memory := r.memoryByPack[request.Node.ManifestName]
	r.mu.Lock()
	r.requests = append(r.requests, request)
	r.active++
	if r.active > r.maxActive {
		r.maxActive = r.active
	}
	r.activeCPU += cpu
	if r.activeCPU > r.maxActiveCPU {
		r.maxActiveCPU = r.activeCPU
	}
	r.activeMemory += memory
	if r.activeMemory > r.maxActiveMemory {
		r.maxActiveMemory = r.activeMemory
	}
	r.mu.Unlock()

	select {
	case <-ctx.Done():
		r.finish(cpu, memory)
		return ToolExecutionResult{}, ctx.Err()
	case <-time.After(r.delay):
	}

	r.finish(cpu, memory)
	return ToolExecutionResult{
		ExecutionID: "resource-lease-" + request.Node.ID,
		Status:      "completed",
		Transcript:  "resource lease probe for " + request.Node.ManifestName,
	}, nil
}

func (r *resourceLeaseProbeRunner) finish(cpu int, memory int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.active--
	r.activeCPU -= cpu
	r.activeMemory -= memory
}

type workerPoolConcurrencyProbeRunner struct {
	mu                sync.Mutex
	active            int
	maxActive         int
	activeByWorker    map[string]int
	maxActiveByWorker map[string]int
	delay             time.Duration
}

func (r *workerPoolConcurrencyProbeRunner) Execute(ctx context.Context, request ToolExecutionRequest) (ToolExecutionResult, error) {
	if err := ctx.Err(); err != nil {
		return ToolExecutionResult{}, err
	}
	workerID := request.WorkerID
	r.mu.Lock()
	r.active++
	if r.active > r.maxActive {
		r.maxActive = r.active
	}
	r.activeByWorker[workerID]++
	if r.activeByWorker[workerID] > r.maxActiveByWorker[workerID] {
		r.maxActiveByWorker[workerID] = r.activeByWorker[workerID]
	}
	r.mu.Unlock()

	select {
	case <-ctx.Done():
		r.finish(workerID)
		return ToolExecutionResult{}, ctx.Err()
	case <-time.After(r.delay):
	}

	r.finish(workerID)
	return ToolExecutionResult{
		ExecutionID: "worker-pool-" + request.Node.ID,
		Status:      "completed",
		Transcript:  "worker pool probe for " + request.Node.ManifestName,
	}, nil
}

func (r *workerPoolConcurrencyProbeRunner) finish(workerID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.active--
	r.activeByWorker[workerID]--
}

func (r *workerPoolConcurrencyProbeRunner) MaxActiveForWorker(workerID string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.maxActiveByWorker[workerID]
}

func (r *workerPoolConcurrencyProbeRunner) MaxActive() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.maxActive
}

type failingWorkerLeaseProbeRunner struct{}

func (failingWorkerLeaseProbeRunner) Execute(ctx context.Context, request ToolExecutionRequest) (ToolExecutionResult, error) {
	if err := ctx.Err(); err != nil {
		return ToolExecutionResult{}, err
	}
	return ToolExecutionResult{
		ExecutionID: "failed-lease-" + request.Node.ID,
		Status:      "failed",
		Transcript:  "lease probe failed",
	}, fmt.Errorf("lease probe failed")
}

type resourceWaitDoesNotHoldWorkerSlotRunner struct {
	mu                                       sync.Mutex
	requestedPacks                           []string
	heavyPacks                               map[string]bool
	compatiblePack                           string
	heavyDelay                               time.Duration
	completedHeavyWorkers                    int
	compatibleStartedBeforeAnyHeavyCompleted bool
}

func (r *resourceWaitDoesNotHoldWorkerSlotRunner) Execute(ctx context.Context, request ToolExecutionRequest) (ToolExecutionResult, error) {
	if err := ctx.Err(); err != nil {
		return ToolExecutionResult{}, err
	}
	pack := request.Node.ManifestName
	r.mu.Lock()
	r.requestedPacks = append(r.requestedPacks, pack)
	if pack == r.compatiblePack && r.completedHeavyWorkers == 0 {
		r.compatibleStartedBeforeAnyHeavyCompleted = true
	}
	r.mu.Unlock()

	if r.heavyPacks[pack] {
		select {
		case <-ctx.Done():
			return ToolExecutionResult{}, ctx.Err()
		case <-time.After(r.heavyDelay):
		}
		r.mu.Lock()
		r.completedHeavyWorkers++
		r.mu.Unlock()
	}

	return ToolExecutionResult{
		ExecutionID: "resource-wait-slot-" + request.Node.ID,
		Status:      "completed",
		Transcript:  "resource wait slot probe for " + pack,
	}, nil
}

func (r *resourceWaitDoesNotHoldWorkerSlotRunner) CompatibleStartedBeforeAnyHeavyCompleted() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.compatibleStartedBeforeAnyHeavyCompleted
}

func (r *resourceWaitDoesNotHoldWorkerSlotRunner) RequestedPacks() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.requestedPacks...)
}

type streamingCompletionToolExecutionRunner struct {
	mu             sync.Mutex
	slowCapability string
	slowDelay      time.Duration
	slowRelease    <-chan struct{}
	slowStarted    bool
	slowDone       bool
}

func (r *streamingCompletionToolExecutionRunner) Execute(ctx context.Context, request ToolExecutionRequest) (ToolExecutionResult, error) {
	if err := ctx.Err(); err != nil {
		return ToolExecutionResult{}, err
	}
	if request.Node.Capability == r.slowCapability && r.claimSlowWorker() {
		if r.slowRelease != nil {
			select {
			case <-ctx.Done():
				return ToolExecutionResult{}, ctx.Err()
			case <-r.slowRelease:
			}
		} else {
			select {
			case <-ctx.Done():
				return ToolExecutionResult{}, ctx.Err()
			case <-time.After(r.slowDelay):
			}
		}
		r.markSlowDone()
	}
	return ToolExecutionResult{
		ExecutionID: "stream-" + request.Node.Capability,
		Status:      "completed",
		Transcript:  "streamed execution for " + request.Node.ManifestName,
	}, nil
}

func (r *streamingCompletionToolExecutionRunner) claimSlowWorker() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.slowStarted {
		return false
	}
	r.slowStarted = true
	return true
}

func (r *streamingCompletionToolExecutionRunner) markSlowDone() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.slowDone = true
}

func (r *streamingCompletionToolExecutionRunner) SlowDone() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.slowDone
}

type pauseResumeProbeRunner struct {
	mu      sync.Mutex
	started chan struct{}
	count   int
}

func (r *pauseResumeProbeRunner) Execute(ctx context.Context, request ToolExecutionRequest) (ToolExecutionResult, error) {
	if err := ctx.Err(); err != nil {
		return ToolExecutionResult{}, err
	}
	r.mu.Lock()
	r.count++
	if r.count == 1 {
		select {
		case r.started <- struct{}{}:
		default:
		}
	}
	r.mu.Unlock()
	return ToolExecutionResult{
		ExecutionID: "pause-resume-" + request.Node.ID,
		Status:      "completed",
		Transcript:  "pause resume probe for " + request.Node.ManifestName,
	}, nil
}

func (r *pauseResumeProbeRunner) StartCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.count
}

func waitForToolWaitingReason(t *testing.T, events <-chan eventlog.Event, reason string) {
	t.Helper()
	timeout := time.After(time.Second)
	for {
		select {
		case event := <-events:
			if event.Type != eventlog.EventTool {
				continue
			}
			var payload struct {
				WorkerStatus string `json:"worker_status"`
				WaitReason   string `json:"wait_reason"`
			}
			if err := json.Unmarshal(event.Payload, &payload); err != nil {
				t.Fatalf("decode tool payload: %v", err)
			}
			if payload.WorkerStatus == "waiting" && payload.WaitReason == reason {
				return
			}
		case <-timeout:
			t.Fatalf("timed out waiting for worker wait_reason=%s", reason)
		}
	}
}

type expectedBuiltInDiscoverPlanNode struct {
	ManifestName   string
	Capability     string
	ToolID         string
	AdapterID      string
	Operation      string
	ParserID       string
	TargetKind     string
	DependencyName string
	DependencyKind string
}

func expectedBuiltInDiscoverPlanNodes() []expectedBuiltInDiscoverPlanNode {
	return []expectedBuiltInDiscoverPlanNode{
		{
			ManifestName:   "web-api-pack",
			Capability:     "web.discovery",
			ToolID:         "httpx",
			AdapterID:      "managed_binary",
			Operation:      "http_probe",
			ParserID:       "httpx_jsonl",
			TargetKind:     "url",
			DependencyName: "httpx",
			DependencyKind: "binary",
		},
		{
			ManifestName:   "web-api-pack",
			Capability:     "web.discovery",
			ToolID:         "subfinder",
			AdapterID:      "managed_binary",
			Operation:      "subdomain_discovery",
			ParserID:       "subfinder_jsonl",
			TargetKind:     "domain",
			DependencyName: "subfinder",
			DependencyKind: "binary",
		},
		{
			ManifestName:   "infra-pack",
			Capability:     "infra.host_discovery",
			ToolID:         "amass",
			AdapterID:      "managed_binary",
			Operation:      "subdomain_discovery",
			ParserID:       "amass_json",
			TargetKind:     "domain",
			DependencyName: "amass",
			DependencyKind: "binary",
		},
		{
			ManifestName:   "infra-pack",
			Capability:     "infra.host_discovery",
			ToolID:         "dnsrecon",
			AdapterID:      "managed_binary",
			Operation:      "dns_probe",
			ParserID:       "dnsrecon_json",
			TargetKind:     "domain",
			DependencyName: "dnsrecon",
			DependencyKind: "binary",
		},
		{
			ManifestName:   "infra-pack",
			Capability:     "infra.host_discovery",
			ToolID:         "dnsx",
			AdapterID:      "managed_binary",
			Operation:      "dns_probe",
			ParserID:       "dnsx_jsonl",
			TargetKind:     "domain",
			DependencyName: "dnsx",
			DependencyKind: "binary",
		},
		{
			ManifestName:   "infra-pack",
			Capability:     "infra.host_discovery",
			ToolID:         "naabu",
			AdapterID:      "managed_binary",
			Operation:      "port_scan",
			ParserID:       "naabu_jsonl",
			TargetKind:     "host",
			DependencyName: "naabu",
			DependencyKind: "binary",
		},
		{
			ManifestName:   "infra-pack",
			Capability:     "infra.host_discovery",
			ToolID:         "nmap",
			AdapterID:      "managed_binary",
			Operation:      "service_fingerprint",
			ParserID:       "nmap_xml",
			TargetKind:     "host",
			DependencyName: "nmap",
			DependencyKind: "binary",
		},
		{
			ManifestName: "ctf-pack",
			Capability:   "ctf.challenge_classification",
			ParserID:     "ctf_parser",
		},
	}
}

func expectedBuiltInDiscoverPlanKey(manifestName, parser string) string {
	return manifestName + "/" + parser
}

func expectedBuiltInDiscoverNodeByParser(t *testing.T, parser string) expectedBuiltInDiscoverPlanNode {
	t.Helper()
	for _, want := range expectedBuiltInDiscoverPlanNodes() {
		if want.ParserID == parser {
			return want
		}
	}
	t.Fatalf("unexpected built-in discover parser %q", parser)
	return expectedBuiltInDiscoverPlanNode{}
}

func assertBuiltInDiscoverToolRequests(t *testing.T, requests []ToolExecutionRequest, runID string, root string) {
	t.Helper()
	expected := expectedBuiltInDiscoverPlanNodes()
	if len(requests) != len(expected) {
		t.Fatalf("expected one request per built-in discover tool node, got %#v", requests)
	}

	seen := map[string]bool{}
	for _, request := range requests {
		if request.RunID != runID || request.WorkspaceRoot != root {
			t.Fatalf("expected request to carry run/workspace context, got %#v", request)
		}
		if request.Node.ID == "" || request.Node.Action != compiler.ActionDiscoverAssets {
			t.Fatalf("expected discover-assets plan node, got %#v", request.Node)
		}
		want := expectedBuiltInDiscoverNodeByParser(t, request.Node.ParserID)
		key := expectedBuiltInDiscoverPlanKey(request.Node.ManifestName, request.Node.ParserID)
		if seen[key] {
			t.Fatalf("duplicate built-in discover request for %s: %#v", key, request.Node)
		}
		seen[key] = true

		if request.Node.ManifestName != want.ManifestName ||
			request.Node.Capability != want.Capability ||
			request.Node.ToolID != want.ToolID ||
			request.Node.AdapterID != want.AdapterID ||
			request.Node.Operation != want.Operation ||
			request.Node.ParserID != want.ParserID ||
			request.Node.Parser != want.ParserID ||
			request.Node.TargetKind != want.TargetKind ||
			request.Node.DependencyName != want.DependencyName ||
			request.Node.DependencyKind != want.DependencyKind ||
			request.Node.Runner != "native" {
			t.Fatalf("unexpected built-in discover request for %s: got %#v want %#v", key, request.Node, want)
		}
		if want.DependencyName == "" {
			continue
		}
		if len(request.Node.Dependencies) != 1 ||
			request.Node.Dependencies[0].Name != want.DependencyName ||
			request.Node.Dependencies[0].Kind != want.DependencyKind {
			t.Fatalf("expected request dependency %s/%s, got %#v", want.DependencyName, want.DependencyKind, request.Node.Dependencies)
		}
	}
	for _, want := range expected {
		key := expectedBuiltInDiscoverPlanKey(want.ManifestName, want.ParserID)
		if !seen[key] {
			t.Fatalf("missing built-in discover request for %s, got %#v", key, requests)
		}
	}
}

func firstNonEmptyTestValue(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
