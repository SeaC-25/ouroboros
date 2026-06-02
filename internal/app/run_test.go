package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ouroboros/ouroboros/internal/compiler"
	"github.com/ouroboros/ouroboros/internal/eventlog"
	"github.com/ouroboros/ouroboros/internal/toolchain"
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
	if len(runner.requests) != 3 {
		t.Fatalf("expected one discover-assets execution per toolchain manifest, got %#v", runner.requests)
	}
	for _, request := range runner.requests {
		if request.RunID != result.RunID || request.WorkspaceRoot != root {
			t.Fatalf("expected request to carry run/workspace context, got %#v", request)
		}
		if request.Node.Action != compiler.ActionDiscoverAssets {
			t.Fatalf("expected discover-assets plan node, got %#v", request.Node)
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
	executions := 0
	queued := 0
	started := 0
	completed := 0
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
	}
	if queued != len(runner.requests) || started != len(runner.requests) || completed != len(runner.requests) {
		t.Fatalf("expected queued/started/completed lifecycle per request, queued=%d started=%d completed=%d executions=%d requests=%d", queued, started, completed, executions, len(runner.requests))
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
	if len(runner.requests) != 3 {
		t.Fatalf("expected one request per toolchain manifest, got %#v", runner.requests)
	}
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
	runner := &streamingCompletionToolExecutionRunner{
		slowCapability: "infra.host_discovery",
		slowDelay:      100 * time.Millisecond,
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
	if len(runner.requests) != 2 {
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
	var skipped struct {
		Tool            string `json:"tool"`
		Capability      string `json:"capability"`
		ExecutionStatus string `json:"execution_status"`
		ExecutionPolicy string `json:"execution_policy"`
		PolicyReason    string `json:"policy_reason"`
		Transcript      string `json:"transcript"`
	}
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
			Transcript      string `json:"transcript"`
		}
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			t.Fatalf("decode tool payload: %v", err)
		}
		if payload.Tool == "web-api-pack" && payload.Capability == "web.discovery" {
			if payload.WorkerStatus != "" {
				t.Fatalf("disabled pack must not emit worker lifecycle events, got %#v", payload)
			}
			skipped.Tool = payload.Tool
			skipped.Capability = payload.Capability
			skipped.ExecutionStatus = payload.ExecutionStatus
			skipped.ExecutionPolicy = payload.ExecutionPolicy
			skipped.PolicyReason = payload.PolicyReason
			skipped.Transcript = payload.Transcript
		}
	}
	if skipped.ExecutionStatus != "skipped" || skipped.ExecutionPolicy != "disabled" {
		t.Fatalf("expected skipped policy event for disabled pack, got %#v", skipped)
	}
	if !strings.Contains(skipped.Transcript, "noisy active scan deferred") || skipped.PolicyReason != "noisy active scan deferred" {
		t.Fatalf("expected policy reason in skipped event, got %#v", skipped)
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
	}
	admissions := map[string]admissionPayload{}
	admissionIndex := map[string]int{}
	webSkippedIndex := -1
	infraQueuedIndex := -1
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
		}
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			t.Fatalf("decode tool payload: %v", err)
		}
		key := payload.Tool + "/" + payload.AdmissionCapability
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
			}
			admissionIndex[key] = index
		}
		if payload.Tool == "web-api-pack" && payload.Capability == "web.discovery" && payload.ExecutionStatus == "skipped" {
			webSkippedIndex = index
		}
		if payload.Tool == "infra-pack" && payload.WorkerCapability == "infra.host_discovery" && payload.WorkerStatus == "queued" {
			infraQueuedIndex = index
		}
	}

	if len(admissions) != 3 {
		t.Fatalf("expected one scheduler admission event per compiled plan node, got %#v", admissions)
	}
	webAdmission := admissions["web-api-pack/web.discovery"]
	if webAdmission.AdmissionDecision != "skipped" || webAdmission.ExecutionPolicy != "disabled" {
		t.Fatalf("expected disabled web pack admission decision, got %#v", webAdmission)
	}
	if webAdmission.PolicyReason != "noisy active scan deferred" || webAdmission.Action != string(compiler.ActionDiscoverAssets) || webAdmission.Isolation != "process" {
		t.Fatalf("expected disabled admission metadata, got %#v", webAdmission)
	}
	if webSkippedIndex == -1 || admissionIndex["web-api-pack/web.discovery"] >= webSkippedIndex {
		t.Fatalf("expected web admission before skipped execution event, admission=%d skipped=%d", admissionIndex["web-api-pack/web.discovery"], webSkippedIndex)
	}
	infraAdmission := admissions["infra-pack/infra.host_discovery"]
	if infraAdmission.AdmissionDecision != "admitted" || infraAdmission.ExecutionPolicy != "enabled" {
		t.Fatalf("expected enabled infra pack admission decision, got %#v", infraAdmission)
	}
	if infraAdmission.PolicyReason != "authorized infra discovery" {
		t.Fatalf("expected infra policy reason in admission, got %#v", infraAdmission)
	}
	if infraQueuedIndex == -1 || admissionIndex["infra-pack/infra.host_discovery"] >= infraQueuedIndex {
		t.Fatalf("expected infra admission before queued worker event, admission=%d queued=%d", admissionIndex["infra-pack/infra.host_discovery"], infraQueuedIndex)
	}
	ctfAdmission := admissions["ctf-pack/ctf.challenge_classification"]
	if ctfAdmission.AdmissionDecision != "admitted" || ctfAdmission.ExecutionPolicy != "default" {
		t.Fatalf("expected default ctf admission decision, got %#v", ctfAdmission)
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

func TestRunWorkspaceRejectsInvalidScope(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	scope := `version: 1
scope:
  targets:
    - kind: artifact
      value: sample.bin
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
	requests []ToolExecutionRequest
}

func (r *recordingToolExecutionRunner) Execute(ctx context.Context, request ToolExecutionRequest) (ToolExecutionResult, error) {
	if err := ctx.Err(); err != nil {
		return ToolExecutionResult{}, err
	}
	r.requests = append(r.requests, request)
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
	slowDone       bool
}

func (r *streamingCompletionToolExecutionRunner) Execute(ctx context.Context, request ToolExecutionRequest) (ToolExecutionResult, error) {
	if err := ctx.Err(); err != nil {
		return ToolExecutionResult{}, err
	}
	if request.Node.Capability == r.slowCapability {
		select {
		case <-ctx.Done():
			return ToolExecutionResult{}, ctx.Err()
		case <-time.After(r.slowDelay):
		}
		r.mu.Lock()
		r.slowDone = true
		r.mu.Unlock()
	}
	return ToolExecutionResult{
		ExecutionID: "stream-" + request.Node.Capability,
		Status:      "completed",
		Transcript:  "streamed execution for " + request.Node.ManifestName,
	}, nil
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

func firstNonEmptyTestValue(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
