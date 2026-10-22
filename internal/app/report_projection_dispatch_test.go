package app

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/ouroboros/ouroboros/internal/eventlog"
)

func TestCreateReportProjectionDispatchWritesMetadataOnlyArtifact(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	run, err := RunWorkspaceWithOptions(ctx, root, RunOptions{DeferReportProjection: true})
	if err != nil {
		t.Fatalf("run workspace with deferred report projection: %v", err)
	}
	store, err := eventlog.OpenSQLite(filepath.Join(root, "replay", "events.db"))
	if err != nil {
		t.Fatalf("open event log: %v", err)
	}
	eventsBefore, err := store.Replay(ctx, run.RunID)
	if err != nil {
		t.Fatalf("replay before dispatch: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close event log: %v", err)
	}

	dispatch, err := CreateReportProjectionDispatch(ctx, root, run.RunID, ReportProjectionDispatchOptions{
		Now:    time.Date(2026, 6, 2, 4, 20, 0, 0, time.UTC),
		Actor:  "alice",
		Reason: "operator review",
	})
	if err != nil {
		t.Fatalf("create report projection dispatch: %v", err)
	}
	if dispatch.Version != 1 || dispatch.Status != "dry-run" || dispatch.Mode != "metadata-only" {
		t.Fatalf("unexpected dispatch metadata: %#v", dispatch)
	}
	if dispatch.RunID != run.RunID || dispatch.Actor != "alice" || dispatch.Reason != "operator review" {
		t.Fatalf("unexpected dispatch identity: %#v", dispatch)
	}
	if dispatch.CreatedAt != "2026-06-02T04:20:00Z" {
		t.Fatalf("unexpected dispatch timestamp: %#v", dispatch)
	}
	if dispatch.EventCount != len(eventsBefore) || dispatch.ReportPath == "" || dispatch.Path == "" {
		t.Fatalf("unexpected dispatch projection metadata: %#v", dispatch)
	}
	if !strings.HasPrefix(dispatch.Path, filepath.Join(root, "reports", "report-projection-dispatches")+string(os.PathSeparator)) {
		t.Fatalf("expected dispatch under report projection dispatch directory, got %s", dispatch.Path)
	}
	if _, err := os.Stat(dispatch.ReportPath); !os.IsNotExist(err) {
		t.Fatalf("report projection dispatch must not write markdown, stat err=%v", err)
	}

	data, err := os.ReadFile(dispatch.Path)
	if err != nil {
		t.Fatalf("read dispatch artifact: %v", err)
	}
	var saved ReportProjectionDispatch
	if err := json.Unmarshal(data, &saved); err != nil {
		t.Fatalf("decode dispatch artifact: %v", err)
	}
	if saved.Path != dispatch.Path || saved.ReportPath != dispatch.ReportPath || saved.EventCount != dispatch.EventCount {
		t.Fatalf("saved dispatch mismatch: saved=%#v dispatch=%#v", saved, dispatch)
	}
	if saved.ReplayDigest == "" || saved.ReplayDigest != dispatch.ReplayDigest {
		t.Fatalf("saved dispatch must preserve replay digest: saved=%#v dispatch=%#v", saved, dispatch)
	}

	store, err = eventlog.OpenSQLite(filepath.Join(root, "replay", "events.db"))
	if err != nil {
		t.Fatalf("reopen event log: %v", err)
	}
	eventsAfter, err := store.Replay(ctx, run.RunID)
	if err != nil {
		t.Fatalf("replay after dispatch: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close event log after dispatch: %v", err)
	}
	if len(eventsAfter) != len(eventsBefore) {
		t.Fatalf("report projection dispatch must not append replay events, before=%d after=%d", len(eventsBefore), len(eventsAfter))
	}
	replay, err := ReplayWorkspace(ctx, root, run.RunID)
	if err != nil {
		t.Fatalf("replay dispatch run: %v", err)
	}
	if replay.ReportPath != "" {
		t.Fatalf("report projection dispatch must not emit report_ready, got %q", replay.ReportPath)
	}
}

func TestListAndReadReportProjectionDispatchesReadsArtifacts(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	run, err := RunWorkspaceWithOptions(ctx, root, RunOptions{DeferReportProjection: true})
	if err != nil {
		t.Fatalf("run workspace with deferred report projection: %v", err)
	}
	dispatch, err := CreateReportProjectionDispatch(ctx, root, run.RunID, ReportProjectionDispatchOptions{
		Now:   time.Date(2026, 6, 2, 4, 21, 0, 0, time.UTC),
		Actor: "bob",
	})
	if err != nil {
		t.Fatalf("create dispatch: %v", err)
	}

	dispatches, err := ListReportProjectionDispatches(root)
	if err != nil {
		t.Fatalf("list report projection dispatches: %v", err)
	}
	if len(dispatches) != 1 || dispatches[0].Path != dispatch.Path {
		t.Fatalf("unexpected dispatch listing: %#v", dispatches)
	}
	read, err := ReadReportProjectionDispatch(root, filepath.Base(dispatch.Path))
	if err != nil {
		t.Fatalf("read dispatch by basename: %v", err)
	}
	if read.Path != dispatch.Path || read.RunID != run.RunID || read.Actor != "bob" {
		t.Fatalf("unexpected read dispatch: %#v", read)
	}
	if _, err := ReadReportProjectionDispatch(root, filepath.Join("..", "escape.json")); err == nil || !strings.Contains(err.Error(), "report projection dispatch path must stay under") {
		t.Fatalf("expected path confinement error, got %v", err)
	}
	if _, err := ReadReportProjectionDispatch(root, "dispatch.txt"); err == nil || !strings.Contains(err.Error(), "report projection dispatch path must be a json file") {
		t.Fatalf("expected json extension error, got %v", err)
	}
}

func TestCreateReportProjectionDispatchDoesNotRunContainerRuntimeDoctor(t *testing.T) {
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
printf 'container runtime should not be inspected during report dispatch creation\n' >&2
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

	if _, err := CreateReportProjectionDispatch(ctx, root, run.RunID, ReportProjectionDispatchOptions{}); err != nil {
		t.Fatalf("create dispatch should not run container doctor checks: %v", err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("report projection dispatch must not inspect container runtime, marker stat err=%v", err)
	}
}

func TestCreateReportProjectionDispatchRejectsAlreadyProjectedRunBeforeWriting(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	run, err := RunWorkspace(ctx, root)
	if err != nil {
		t.Fatalf("run workspace: %v", err)
	}

	_, err = CreateReportProjectionDispatch(ctx, root, run.RunID, ReportProjectionDispatchOptions{})
	if err == nil || !strings.Contains(err.Error(), "already has a report projection") {
		t.Fatalf("expected already projected error, got %v", err)
	}
	if got := countJSON(t, filepath.Join(root, "reports", "report-projection-dispatches")); got != 0 {
		t.Fatalf("already projected run must not create dispatch artifact, got %d", got)
	}
}

func TestExecuteReportProjectionDispatchRequiresExplicitExecute(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	run, err := RunWorkspaceWithOptions(ctx, root, RunOptions{DeferReportProjection: true})
	if err != nil {
		t.Fatalf("run workspace with deferred report projection: %v", err)
	}
	dispatch, err := CreateReportProjectionDispatch(ctx, root, run.RunID, ReportProjectionDispatchOptions{})
	if err != nil {
		t.Fatalf("create dispatch: %v", err)
	}

	_, err = ExecuteReportProjectionDispatch(ctx, root, dispatch.Path, ReportProjectionDispatchExecutionOptions{})
	if err == nil || !strings.Contains(err.Error(), "report projection dispatch execution requires explicit execute") {
		t.Fatalf("expected explicit execute error, got %v", err)
	}
	if got := countJSON(t, filepath.Join(root, "reports", "report-projection-dispatch-executions")); got != 0 {
		t.Fatalf("missing execute gate must not write execution artifacts, got %d", got)
	}
	if _, err := os.Stat(dispatch.ReportPath); !os.IsNotExist(err) {
		t.Fatalf("missing execute gate must not write markdown, stat err=%v", err)
	}
	replay, err := ReplayWorkspace(ctx, root, run.RunID)
	if err != nil {
		t.Fatalf("replay after rejected dispatch execution: %v", err)
	}
	if replay.ReportPath != "" {
		t.Fatalf("missing execute gate must not emit report_ready, got %q", replay.ReportPath)
	}
}

func TestExecuteReportProjectionDispatchWritesReportAndAuditArtifact(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	run, err := RunWorkspaceWithOptions(ctx, root, RunOptions{DeferReportProjection: true})
	if err != nil {
		t.Fatalf("run workspace with deferred report projection: %v", err)
	}
	dispatch, err := CreateReportProjectionDispatch(ctx, root, run.RunID, ReportProjectionDispatchOptions{
		Now:    time.Date(2026, 6, 2, 5, 30, 0, 0, time.UTC),
		Actor:  "alice",
		Reason: "ready to project",
	})
	if err != nil {
		t.Fatalf("create dispatch: %v", err)
	}

	execution, err := ExecuteReportProjectionDispatch(ctx, root, filepath.Base(dispatch.Path), ReportProjectionDispatchExecutionOptions{
		Execute: true,
		Now:     time.Date(2026, 6, 2, 5, 31, 0, 0, time.UTC),
		Actor:   "bob",
		Reason:  "operator approved projection",
	})
	if err != nil {
		t.Fatalf("execute report projection dispatch: %v", err)
	}
	if execution.Version != 1 || execution.Status != "completed" || execution.Mode != "execute" || !execution.Written {
		t.Fatalf("unexpected execution metadata: %#v", execution)
	}
	if execution.DispatchPath != dispatch.Path || execution.RunID != run.RunID || execution.ReportPath != dispatch.ReportPath {
		t.Fatalf("unexpected execution links: execution=%#v dispatch=%#v", execution, dispatch)
	}
	if execution.Actor != "bob" || execution.Reason != "operator approved projection" || execution.ExecutedAt != "2026-06-02T05:31:00Z" {
		t.Fatalf("unexpected execution operator metadata: %#v", execution)
	}
	if execution.EventCount <= dispatch.EventCount {
		t.Fatalf("expected execution event count to include report projection events, dispatch=%d execution=%d", dispatch.EventCount, execution.EventCount)
	}
	if !strings.HasPrefix(execution.Path, filepath.Join(root, "reports", "report-projection-dispatch-executions")+string(os.PathSeparator)) {
		t.Fatalf("expected execution under report projection executions directory, got %s", execution.Path)
	}
	if _, err := os.Stat(execution.ReportPath); err != nil {
		t.Fatalf("expected markdown report to be written: %v", err)
	}
	reportBytes, err := os.ReadFile(execution.ReportPath)
	if err != nil {
		t.Fatalf("read markdown report: %v", err)
	}
	reportDigest := sha256.Sum256(reportBytes)
	reportSHA256 := hex.EncodeToString(reportDigest[:])
	artifactBytes, err := os.ReadFile(execution.Path)
	if err != nil {
		t.Fatalf("read report projection execution artifact: %v", err)
	}
	artifactText := string(artifactBytes)
	if !strings.Contains(artifactText, `"report_sha256":"`+reportSHA256+`"`) &&
		!strings.Contains(artifactText, `"report_sha256": "`+reportSHA256+`"`) {
		t.Fatalf("expected execution artifact to record report_sha256 %s", reportSHA256)
	}
	replay, err := ReplayWorkspace(ctx, root, run.RunID)
	if err != nil {
		t.Fatalf("replay after report dispatch execution: %v", err)
	}
	if replay.ReportPath != dispatch.ReportPath {
		t.Fatalf("expected replay report path %q, got %q", dispatch.ReportPath, replay.ReportPath)
	}

	executions, err := ListReportProjectionDispatchExecutions(root)
	if err != nil {
		t.Fatalf("list report projection dispatch executions: %v", err)
	}
	if len(executions) != 1 || executions[0].Path != execution.Path {
		t.Fatalf("unexpected execution listing: %#v", executions)
	}
	read, err := ReadReportProjectionDispatchExecution(root, filepath.Base(execution.Path))
	if err != nil {
		t.Fatalf("read report projection dispatch execution: %v", err)
	}
	if read.Path != execution.Path || read.DispatchPath != dispatch.Path || !read.Written {
		t.Fatalf("unexpected read execution: %#v", read)
	}
}

func TestExecuteReportProjectionDispatchDoesNotRunContainerRuntimeDoctor(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	run, err := RunWorkspaceWithOptions(ctx, root, RunOptions{DeferReportProjection: true})
	if err != nil {
		t.Fatalf("run workspace with deferred report projection: %v", err)
	}
	dispatch, err := CreateReportProjectionDispatch(ctx, root, run.RunID, ReportProjectionDispatchOptions{})
	if err != nil {
		t.Fatalf("create dispatch: %v", err)
	}
	engineDir := t.TempDir()
	engine := filepath.Join(engineDir, "fake-container-engine")
	marker := filepath.Join(engineDir, "container-runtime-was-called")
	engineScript := `#!/bin/sh
touch "` + marker + `"
printf 'container runtime should not be inspected during report dispatch execution\n' >&2
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

	if _, err := ExecuteReportProjectionDispatch(ctx, root, dispatch.Path, ReportProjectionDispatchExecutionOptions{Execute: true}); err != nil {
		t.Fatalf("execute dispatch should not run container doctor checks: %v", err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("report projection dispatch execution must not inspect container runtime, marker stat err=%v", err)
	}
}

func TestExecuteReportProjectionDispatchRejectsSymlinkedDispatchFileBeforeWriting(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	run, err := RunWorkspaceWithOptions(ctx, root, RunOptions{DeferReportProjection: true})
	if err != nil {
		t.Fatalf("run workspace with deferred report projection: %v", err)
	}
	dispatch, err := CreateReportProjectionDispatch(ctx, root, run.RunID, ReportProjectionDispatchOptions{})
	if err != nil {
		t.Fatalf("create dispatch: %v", err)
	}
	linkPath := filepath.Join(root, "reports", "report-projection-dispatches", "linked-dispatch.json")
	if err := os.Symlink(dispatch.Path, linkPath); err != nil {
		t.Fatalf("create dispatch symlink: %v", err)
	}

	_, err = ExecuteReportProjectionDispatch(ctx, root, filepath.Base(linkPath), ReportProjectionDispatchExecutionOptions{Execute: true})
	if err == nil || !strings.Contains(err.Error(), "report projection dispatch file must not be a symlink") {
		t.Fatalf("expected symlink dispatch rejection, got %v", err)
	}
	if got := countJSON(t, filepath.Join(root, "reports", "report-projection-dispatch-executions")); got != 0 {
		t.Fatalf("symlinked dispatch must not write execution artifacts, got %d", got)
	}
	if _, err := os.Stat(dispatch.ReportPath); !os.IsNotExist(err) {
		t.Fatalf("symlinked dispatch must not write markdown, stat err=%v", err)
	}
}

func TestExecuteReportProjectionDispatchRejectsSymlinkedDispatchDirectoryBeforeReading(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	run, err := RunWorkspaceWithOptions(ctx, root, RunOptions{DeferReportProjection: true})
	if err != nil {
		t.Fatalf("run workspace with deferred report projection: %v", err)
	}
	dispatch, err := CreateReportProjectionDispatch(ctx, root, run.RunID, ReportProjectionDispatchOptions{})
	if err != nil {
		t.Fatalf("create dispatch: %v", err)
	}
	externalDir := filepath.Join(t.TempDir(), "outside-dispatches")
	if err := os.MkdirAll(externalDir, 0o755); err != nil {
		t.Fatalf("create external dispatch dir: %v", err)
	}
	data, err := os.ReadFile(dispatch.Path)
	if err != nil {
		t.Fatalf("read original dispatch: %v", err)
	}
	if err := os.WriteFile(filepath.Join(externalDir, filepath.Base(dispatch.Path)), data, 0o644); err != nil {
		t.Fatalf("write external dispatch: %v", err)
	}
	dispatchDir := filepath.Join(root, "reports", "report-projection-dispatches")
	if err := os.RemoveAll(dispatchDir); err != nil {
		t.Fatalf("remove dispatch dir: %v", err)
	}
	if err := os.Symlink(externalDir, dispatchDir); err != nil {
		t.Fatalf("symlink dispatch dir: %v", err)
	}

	_, err = ExecuteReportProjectionDispatch(ctx, root, filepath.Base(dispatch.Path), ReportProjectionDispatchExecutionOptions{Execute: true})
	if err == nil || !strings.Contains(err.Error(), "report projection dispatch directory must not be a symlink") {
		t.Fatalf("expected symlinked dispatch directory rejection, got %v", err)
	}
	if got := countJSON(t, filepath.Join(root, "reports", "report-projection-dispatch-executions")); got != 0 {
		t.Fatalf("symlinked dispatch directory must not write execution artifacts, got %d", got)
	}
	if _, err := os.Stat(dispatch.ReportPath); !os.IsNotExist(err) {
		t.Fatalf("symlinked dispatch directory must not write markdown, stat err=%v", err)
	}
}

func TestExecuteReportProjectionDispatchRejectsNestedSymlinkDispatchParentBeforeReading(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	run, err := RunWorkspaceWithOptions(ctx, root, RunOptions{DeferReportProjection: true})
	if err != nil {
		t.Fatalf("run workspace with deferred report projection: %v", err)
	}
	dispatch, err := CreateReportProjectionDispatch(ctx, root, run.RunID, ReportProjectionDispatchOptions{})
	if err != nil {
		t.Fatalf("create dispatch: %v", err)
	}
	externalDir := filepath.Join(t.TempDir(), "outside-nested-dispatches")
	if err := os.MkdirAll(externalDir, 0o755); err != nil {
		t.Fatalf("create external nested dispatch dir: %v", err)
	}
	data, err := os.ReadFile(dispatch.Path)
	if err != nil {
		t.Fatalf("read original dispatch: %v", err)
	}
	if err := os.WriteFile(filepath.Join(externalDir, filepath.Base(dispatch.Path)), data, 0o644); err != nil {
		t.Fatalf("write external nested dispatch: %v", err)
	}
	linkParent := filepath.Join(root, "reports", "report-projection-dispatches", "linked-parent")
	if err := os.Symlink(externalDir, linkParent); err != nil {
		t.Fatalf("symlink nested dispatch parent: %v", err)
	}

	_, err = ExecuteReportProjectionDispatch(ctx, root, filepath.Join("linked-parent", filepath.Base(dispatch.Path)), ReportProjectionDispatchExecutionOptions{Execute: true})
	if err == nil || !strings.Contains(err.Error(), "report projection dispatch path must be a direct json file") {
		t.Fatalf("expected nested dispatch path rejection, got %v", err)
	}
	if got := countJSON(t, filepath.Join(root, "reports", "report-projection-dispatch-executions")); got != 0 {
		t.Fatalf("nested symlink dispatch parent must not write execution artifacts, got %d", got)
	}
	if _, err := os.Stat(dispatch.ReportPath); !os.IsNotExist(err) {
		t.Fatalf("nested symlink dispatch parent must not write markdown, stat err=%v", err)
	}
}

func TestExecuteReportProjectionDispatchRejectsTamperedDispatchMetadataBeforeWriting(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name      string
		mutate    func(*ReportProjectionDispatch)
		wantError string
	}{
		{
			name: "version",
			mutate: func(dispatch *ReportProjectionDispatch) {
				dispatch.Version = 2
			},
			wantError: "report projection dispatch version is unsupported",
		},
		{
			name: "status",
			mutate: func(dispatch *ReportProjectionDispatch) {
				dispatch.Status = "completed"
			},
			wantError: "report projection dispatch must be dry-run metadata-only before execution",
		},
		{
			name: "mode",
			mutate: func(dispatch *ReportProjectionDispatch) {
				dispatch.Mode = "write"
			},
			wantError: "report projection dispatch must be dry-run metadata-only before execution",
		},
		{
			name: "report-path",
			mutate: func(dispatch *ReportProjectionDispatch) {
				dispatch.ReportPath = filepath.Join(filepath.Dir(dispatch.ReportPath), "tampered.md")
			},
			wantError: "report projection dispatch report path mismatch",
		},
		{
			name: "event-count",
			mutate: func(dispatch *ReportProjectionDispatch) {
				dispatch.EventCount++
			},
			wantError: "report projection dispatch event count mismatch",
		},
		{
			name: "replay-digest",
			mutate: func(dispatch *ReportProjectionDispatch) {
				dispatch.ReplayDigest = "tampered"
			},
			wantError: "report projection dispatch replay digest mismatch",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "workspace")
			if err := InitWorkspace(root); err != nil {
				t.Fatalf("init workspace: %v", err)
			}
			run, err := RunWorkspaceWithOptions(ctx, root, RunOptions{DeferReportProjection: true})
			if err != nil {
				t.Fatalf("run workspace with deferred report projection: %v", err)
			}
			dispatch, err := CreateReportProjectionDispatch(ctx, root, run.RunID, ReportProjectionDispatchOptions{})
			if err != nil {
				t.Fatalf("create dispatch: %v", err)
			}
			tampered := dispatch
			tc.mutate(&tampered)
			data, err := json.MarshalIndent(tampered, "", "  ")
			if err != nil {
				t.Fatalf("encode tampered dispatch: %v", err)
			}
			if err := os.WriteFile(dispatch.Path, append(data, '\n'), 0o644); err != nil {
				t.Fatalf("write tampered dispatch: %v", err)
			}

			_, err = ExecuteReportProjectionDispatch(ctx, root, filepath.Base(dispatch.Path), ReportProjectionDispatchExecutionOptions{Execute: true})
			if err == nil || !strings.Contains(err.Error(), tc.wantError) {
				t.Fatalf("expected %q error, got %v", tc.wantError, err)
			}
			if got := countJSON(t, filepath.Join(root, "reports", "report-projection-dispatch-executions")); got != 0 {
				t.Fatalf("tampered dispatch must not write execution artifacts, got %d", got)
			}
			if _, err := os.Stat(dispatch.ReportPath); !os.IsNotExist(err) {
				t.Fatalf("tampered dispatch must not write markdown, stat err=%v", err)
			}
			replay, err := ReplayWorkspace(ctx, root, run.RunID)
			if err != nil {
				t.Fatalf("replay after tampered dispatch execution: %v", err)
			}
			if replay.ReportPath != "" {
				t.Fatalf("tampered dispatch must not emit report_ready, got %q", replay.ReportPath)
			}
		})
	}
}

func TestExecuteReportProjectionDispatchRejectsSameCountReplayMutationBeforeWriting(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	run, err := RunWorkspaceWithOptions(ctx, root, RunOptions{DeferReportProjection: true})
	if err != nil {
		t.Fatalf("run workspace with deferred report projection: %v", err)
	}
	dispatch, err := CreateReportProjectionDispatch(ctx, root, run.RunID, ReportProjectionDispatchOptions{})
	if err != nil {
		t.Fatalf("create dispatch: %v", err)
	}
	store, err := eventlog.OpenSQLite(filepath.Join(root, "replay", "events.db"))
	if err != nil {
		t.Fatalf("open event log before replay mutation: %v", err)
	}
	events, err := store.Replay(ctx, run.RunID)
	if err != nil {
		t.Fatalf("replay before mutation: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close event log before mutation: %v", err)
	}
	if len(events) == 0 {
		t.Fatalf("expected replay events")
	}
	var payload map[string]any
	if err := json.Unmarshal(events[0].Payload, &payload); err != nil {
		t.Fatalf("decode first event payload: %v", err)
	}
	payload["tampered"] = true
	tamperedPayload, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("encode tampered event payload: %v", err)
	}
	db, err := sql.Open("sqlite", filepath.Join(root, "replay", "events.db"))
	if err != nil {
		t.Fatalf("open sqlite for replay mutation: %v", err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE events SET payload = ? WHERE id = ?`, tamperedPayload, events[0].ID); err != nil {
		_ = db.Close()
		t.Fatalf("mutate replay event payload: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close sqlite after replay mutation: %v", err)
	}

	_, err = ExecuteReportProjectionDispatch(ctx, root, dispatch.Path, ReportProjectionDispatchExecutionOptions{Execute: true})
	if err == nil || !strings.Contains(err.Error(), "report projection dispatch replay digest mismatch") {
		t.Fatalf("expected replay digest mismatch, got %v", err)
	}
	if got := countJSON(t, filepath.Join(root, "reports", "report-projection-dispatch-executions")); got != 0 {
		t.Fatalf("same-count replay mutation must not write execution artifacts, got %d", got)
	}
	if _, err := os.Stat(dispatch.ReportPath); !os.IsNotExist(err) {
		t.Fatalf("same-count replay mutation must not write markdown, stat err=%v", err)
	}
}

func TestExecuteReportProjectionDispatchRejectsAlreadyProjectedDispatchWithoutWritingSecondAudit(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	run, err := RunWorkspaceWithOptions(ctx, root, RunOptions{DeferReportProjection: true})
	if err != nil {
		t.Fatalf("run workspace with deferred report projection: %v", err)
	}
	dispatch, err := CreateReportProjectionDispatch(ctx, root, run.RunID, ReportProjectionDispatchOptions{})
	if err != nil {
		t.Fatalf("create dispatch: %v", err)
	}
	if _, err := ExecuteReportProjectionDispatch(ctx, root, dispatch.Path, ReportProjectionDispatchExecutionOptions{Execute: true}); err != nil {
		t.Fatalf("execute dispatch first time: %v", err)
	}
	store, err := eventlog.OpenSQLite(filepath.Join(root, "replay", "events.db"))
	if err != nil {
		t.Fatalf("open event log after first execution: %v", err)
	}
	eventsBefore, err := store.Replay(ctx, run.RunID)
	if err != nil {
		t.Fatalf("replay events before second execution: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close event log before second execution: %v", err)
	}

	_, err = ExecuteReportProjectionDispatch(ctx, root, dispatch.Path, ReportProjectionDispatchExecutionOptions{Execute: true})
	if err == nil || !strings.Contains(err.Error(), "already has a report projection") {
		t.Fatalf("expected already projected error, got %v", err)
	}
	if got := countJSON(t, filepath.Join(root, "reports", "report-projection-dispatch-executions")); got != 1 {
		t.Fatalf("already projected dispatch must not write a second execution artifact, got %d", got)
	}
	store, err = eventlog.OpenSQLite(filepath.Join(root, "replay", "events.db"))
	if err != nil {
		t.Fatalf("reopen event log after second execution: %v", err)
	}
	eventsAfter, err := store.Replay(ctx, run.RunID)
	if err != nil {
		t.Fatalf("replay events after second execution: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close event log after second execution: %v", err)
	}
	if len(eventsAfter) != len(eventsBefore) {
		t.Fatalf("already projected dispatch must not append replay events, before=%d after=%d", len(eventsBefore), len(eventsAfter))
	}
}

func TestExecuteReportProjectionDispatchFailureAuditRefreshesReplayState(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	run, err := RunWorkspaceWithOptions(ctx, root, RunOptions{DeferReportProjection: true})
	if err != nil {
		t.Fatalf("run workspace with deferred report projection: %v", err)
	}
	dispatch, err := CreateReportProjectionDispatch(ctx, root, run.RunID, ReportProjectionDispatchOptions{})
	if err != nil {
		t.Fatalf("create dispatch: %v", err)
	}
	if err := os.MkdirAll(dispatch.ReportPath, 0o755); err != nil {
		t.Fatalf("create blocking report directory: %v", err)
	}

	execution, err := ExecuteReportProjectionDispatch(ctx, root, dispatch.Path, ReportProjectionDispatchExecutionOptions{Execute: true})
	if err == nil || !strings.Contains(err.Error(), "write report") {
		t.Fatalf("expected report write error, got %v", err)
	}
	if execution.Status != "failed" || execution.Written {
		t.Fatalf("unexpected failed execution metadata: %#v", execution)
	}
	if execution.EventCount <= dispatch.EventCount {
		t.Fatalf("failed execution audit must refresh replay event count after started event, dispatch=%d execution=%d", dispatch.EventCount, execution.EventCount)
	}
	if !strings.Contains(execution.Error, "write report") {
		t.Fatalf("expected failed execution error metadata, got %#v", execution)
	}
	executions, err := ListReportProjectionDispatchExecutions(root)
	if err != nil {
		t.Fatalf("list report projection dispatch executions: %v", err)
	}
	if len(executions) != 1 || executions[0].Path != execution.Path || executions[0].EventCount != execution.EventCount {
		t.Fatalf("unexpected failed execution audit listing: %#v execution=%#v", executions, execution)
	}
	store, err := eventlog.OpenSQLite(filepath.Join(root, "replay", "events.db"))
	if err != nil {
		t.Fatalf("open event log after failed execution: %v", err)
	}
	events, err := store.Replay(ctx, run.RunID)
	if err != nil {
		t.Fatalf("replay events after failed execution: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close event log after failed execution: %v", err)
	}
	if len(events) != execution.EventCount {
		t.Fatalf("failed execution event count must match replay, events=%d execution=%d", len(events), execution.EventCount)
	}
	replay, err := ReplayWorkspace(ctx, root, run.RunID)
	if err != nil {
		t.Fatalf("replay after failed dispatch execution: %v", err)
	}
	if replay.ReportPath != "" {
		t.Fatalf("failed dispatch execution must not emit report_ready, got %q", replay.ReportPath)
	}
}

func TestAnnotateReportProjectionDispatchExecutionWritesMetadataOnlyDecision(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	run, err := RunWorkspaceWithOptions(ctx, root, RunOptions{DeferReportProjection: true})
	if err != nil {
		t.Fatalf("run workspace with deferred report projection: %v", err)
	}
	dispatch, err := CreateReportProjectionDispatch(ctx, root, run.RunID, ReportProjectionDispatchOptions{})
	if err != nil {
		t.Fatalf("create dispatch: %v", err)
	}
	execution, err := ExecuteReportProjectionDispatch(ctx, root, dispatch.Path, ReportProjectionDispatchExecutionOptions{Execute: true})
	if err != nil {
		t.Fatalf("execute dispatch: %v", err)
	}
	reportBefore, err := os.ReadFile(execution.ReportPath)
	if err != nil {
		t.Fatalf("read report before annotation: %v", err)
	}
	eventsBefore := reportProjectionDispatchTestEventCount(t, ctx, root, run.RunID)

	annotated, err := AnnotateReportProjectionDispatchExecution(root, filepath.Base(execution.Path), ReportProjectionDispatchExecutionAnnotationOptions{
		Decision:   "accepted",
		Reviewer:   "alice",
		Reason:     "reviewed false positive boundary",
		ReviewedAt: time.Date(2026, 6, 2, 6, 45, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("annotate execution: %v", err)
	}
	if annotated.Annotation == nil {
		t.Fatalf("expected annotation on execution: %#v", annotated)
	}
	if annotated.Annotation.Decision != "accepted" || annotated.Annotation.Reviewer != "alice" || annotated.Annotation.Reason != "reviewed false positive boundary" || annotated.Annotation.ReviewedAt != "2026-06-02T06:45:00Z" {
		t.Fatalf("unexpected annotation metadata: %#v", annotated.Annotation)
	}
	if annotated.Path != execution.Path || annotated.ReportPath != execution.ReportPath || !annotated.Written {
		t.Fatalf("annotation must preserve execution links/status: annotated=%#v execution=%#v", annotated, execution)
	}
	read, err := ReadReportProjectionDispatchExecution(root, filepath.Base(execution.Path))
	if err != nil {
		t.Fatalf("read annotated execution: %v", err)
	}
	if read.Annotation == nil || read.Annotation.Decision != "accepted" {
		t.Fatalf("annotation was not persisted: %#v", read)
	}
	reportAfter, err := os.ReadFile(execution.ReportPath)
	if err != nil {
		t.Fatalf("read report after annotation: %v", err)
	}
	if string(reportAfter) != string(reportBefore) {
		t.Fatalf("annotation must not rewrite markdown report")
	}
	if eventsAfter := reportProjectionDispatchTestEventCount(t, ctx, root, run.RunID); eventsAfter != eventsBefore {
		t.Fatalf("annotation must not append replay events, before=%d after=%d", eventsBefore, eventsAfter)
	}
}

func TestAnnotateReportProjectionDispatchExecutionRejectsInvalidDecisionBeforeWriting(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	run, err := RunWorkspaceWithOptions(ctx, root, RunOptions{DeferReportProjection: true})
	if err != nil {
		t.Fatalf("run workspace with deferred report projection: %v", err)
	}
	dispatch, err := CreateReportProjectionDispatch(ctx, root, run.RunID, ReportProjectionDispatchOptions{})
	if err != nil {
		t.Fatalf("create dispatch: %v", err)
	}
	execution, err := ExecuteReportProjectionDispatch(ctx, root, dispatch.Path, ReportProjectionDispatchExecutionOptions{Execute: true})
	if err != nil {
		t.Fatalf("execute dispatch: %v", err)
	}
	before, err := os.ReadFile(execution.Path)
	if err != nil {
		t.Fatalf("read execution before invalid annotation: %v", err)
	}

	_, err = AnnotateReportProjectionDispatchExecution(root, execution.Path, ReportProjectionDispatchExecutionAnnotationOptions{Decision: "unknown"})
	if err == nil || !strings.Contains(err.Error(), "unsupported report projection dispatch execution annotation decision") {
		t.Fatalf("expected invalid decision error, got %v", err)
	}
	after, err := os.ReadFile(execution.Path)
	if err != nil {
		t.Fatalf("read execution after invalid annotation: %v", err)
	}
	if string(after) != string(before) {
		t.Fatalf("invalid annotation must not rewrite execution artifact")
	}
}

func TestSelectReportProjectionDispatchExecutionWritesCurrentMetadataOnlySelection(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	run, err := RunWorkspaceWithOptions(ctx, root, RunOptions{DeferReportProjection: true})
	if err != nil {
		t.Fatalf("run workspace with deferred report projection: %v", err)
	}
	dispatch, err := CreateReportProjectionDispatch(ctx, root, run.RunID, ReportProjectionDispatchOptions{})
	if err != nil {
		t.Fatalf("create dispatch: %v", err)
	}
	execution, err := ExecuteReportProjectionDispatch(ctx, root, dispatch.Path, ReportProjectionDispatchExecutionOptions{Execute: true})
	if err != nil {
		t.Fatalf("execute dispatch: %v", err)
	}
	acceptReportProjectionDispatchExecutionForTest(t, root, execution.Path)

	selection, err := SelectReportProjectionDispatchExecution(root, filepath.Base(execution.Path), ReportProjectionDispatchExecutionSelectionOptions{
		SelectedBy: "alice",
		SelectedAt: time.Date(2026, 6, 2, 6, 50, 0, 0, time.UTC),
		Reason:     "make current reviewed report",
	})
	if err != nil {
		t.Fatalf("select report projection execution: %v", err)
	}
	if selection.Status != "current" || selection.Mode != "metadata-only" {
		t.Fatalf("unexpected selection state: %#v", selection)
	}
	if selection.ExecutionPath != execution.Path || selection.DispatchPath != dispatch.Path || selection.RunID != run.RunID || selection.ReportPath != execution.ReportPath {
		t.Fatalf("unexpected selection links: selection=%#v execution=%#v dispatch=%#v", selection, execution, dispatch)
	}
	if selection.EventCount != execution.EventCount || !selection.Written {
		t.Fatalf("selection must preserve completed written execution metadata: %#v", selection)
	}
	if selection.SelectedBy != "alice" || selection.SelectedAt != "2026-06-02T06:50:00Z" || selection.Reason != "make current reviewed report" {
		t.Fatalf("unexpected selection operator metadata: %#v", selection)
	}
	if selection.Path != filepath.Join(root, "reports", "report-projection-selection.json") {
		t.Fatalf("unexpected selection path: %s", selection.Path)
	}
	read, found, err := ReadReportProjectionDispatchExecutionSelection(root)
	if err != nil {
		t.Fatalf("read selection: %v", err)
	}
	if !found || read.Path != selection.Path || read.ExecutionPath != execution.Path || read.Status != "current" {
		t.Fatalf("unexpected persisted selection: found=%v selection=%#v", found, read)
	}
}

func TestSelectReportProjectionDispatchExecutionRejectsFailedOrUnwrittenExecutionBeforeMarker(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name   string
		mutate func(ReportProjectionDispatchExecution) ReportProjectionDispatchExecution
	}{
		{
			name: "failed",
			mutate: func(execution ReportProjectionDispatchExecution) ReportProjectionDispatchExecution {
				execution.Status = "failed"
				execution.Written = false
				return execution
			},
		},
		{
			name: "unwritten",
			mutate: func(execution ReportProjectionDispatchExecution) ReportProjectionDispatchExecution {
				execution.Status = "completed"
				execution.Written = false
				return execution
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "workspace")
			if err := InitWorkspace(root); err != nil {
				t.Fatalf("init workspace: %v", err)
			}
			run, err := RunWorkspaceWithOptions(ctx, root, RunOptions{DeferReportProjection: true})
			if err != nil {
				t.Fatalf("run workspace with deferred report projection: %v", err)
			}
			dispatch, err := CreateReportProjectionDispatch(ctx, root, run.RunID, ReportProjectionDispatchOptions{})
			if err != nil {
				t.Fatalf("create dispatch: %v", err)
			}
			execution, err := ExecuteReportProjectionDispatch(ctx, root, dispatch.Path, ReportProjectionDispatchExecutionOptions{Execute: true})
			if err != nil {
				t.Fatalf("execute dispatch: %v", err)
			}
			mutated := tc.mutate(execution)
			data, err := json.MarshalIndent(mutated, "", "  ")
			if err != nil {
				t.Fatalf("encode mutated execution: %v", err)
			}
			if err := os.WriteFile(execution.Path, append(data, '\n'), 0o644); err != nil {
				t.Fatalf("write mutated execution: %v", err)
			}

			_, err = SelectReportProjectionDispatchExecution(root, filepath.Base(execution.Path), ReportProjectionDispatchExecutionSelectionOptions{})
			if err == nil || !strings.Contains(err.Error(), "report projection dispatch execution must be completed and written before selection") {
				t.Fatalf("expected completed/written selection error, got %v", err)
			}
			if _, err := os.Stat(filepath.Join(root, "reports", "report-projection-selection.json")); !os.IsNotExist(err) {
				t.Fatalf("rejected selection must not write marker, stat err=%v", err)
			}
		})
	}
}

func TestSelectReportProjectionDispatchExecutionRequiresAcceptedAnnotationBeforeMarker(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	run, err := RunWorkspaceWithOptions(ctx, root, RunOptions{DeferReportProjection: true})
	if err != nil {
		t.Fatalf("run workspace with deferred report projection: %v", err)
	}
	dispatch, err := CreateReportProjectionDispatch(ctx, root, run.RunID, ReportProjectionDispatchOptions{})
	if err != nil {
		t.Fatalf("create dispatch: %v", err)
	}
	execution, err := ExecuteReportProjectionDispatch(ctx, root, dispatch.Path, ReportProjectionDispatchExecutionOptions{Execute: true})
	if err != nil {
		t.Fatalf("execute dispatch: %v", err)
	}

	_, err = SelectReportProjectionDispatchExecution(root, filepath.Base(execution.Path), ReportProjectionDispatchExecutionSelectionOptions{})
	if err == nil || !strings.Contains(err.Error(), "report projection dispatch execution selection requires accepted annotation") {
		t.Fatalf("expected accepted annotation selection error, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "reports", "report-projection-selection.json")); !os.IsNotExist(err) {
		t.Fatalf("unreviewed execution must not write selection marker, stat err=%v", err)
	}
}

func TestSelectReportProjectionDispatchExecutionRejectsDeferredOrRejectedAnnotationBeforeMarker(t *testing.T) {
	ctx := context.Background()
	for _, decision := range []string{"deferred", "rejected"} {
		t.Run(decision, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "workspace")
			if err := InitWorkspace(root); err != nil {
				t.Fatalf("init workspace: %v", err)
			}
			run, err := RunWorkspaceWithOptions(ctx, root, RunOptions{DeferReportProjection: true})
			if err != nil {
				t.Fatalf("run workspace with deferred report projection: %v", err)
			}
			dispatch, err := CreateReportProjectionDispatch(ctx, root, run.RunID, ReportProjectionDispatchOptions{})
			if err != nil {
				t.Fatalf("create dispatch: %v", err)
			}
			execution, err := ExecuteReportProjectionDispatch(ctx, root, dispatch.Path, ReportProjectionDispatchExecutionOptions{Execute: true})
			if err != nil {
				t.Fatalf("execute dispatch: %v", err)
			}
			if _, err := AnnotateReportProjectionDispatchExecution(root, filepath.Base(execution.Path), ReportProjectionDispatchExecutionAnnotationOptions{Decision: decision}); err != nil {
				t.Fatalf("annotate execution as %s: %v", decision, err)
			}

			_, err = SelectReportProjectionDispatchExecution(root, filepath.Base(execution.Path), ReportProjectionDispatchExecutionSelectionOptions{})
			if err == nil || !strings.Contains(err.Error(), "report projection dispatch execution selection requires accepted annotation") {
				t.Fatalf("expected accepted annotation selection error, got %v", err)
			}
			if _, err := os.Stat(filepath.Join(root, "reports", "report-projection-selection.json")); !os.IsNotExist(err) {
				t.Fatalf("%s execution must not write selection marker, stat err=%v", decision, err)
			}
		})
	}
}

func TestAnnotateReportProjectionDispatchExecutionRejectsFalsePositiveDecision(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	run, err := RunWorkspaceWithOptions(ctx, root, RunOptions{DeferReportProjection: true})
	if err != nil {
		t.Fatalf("run workspace with deferred report projection: %v", err)
	}
	dispatch, err := CreateReportProjectionDispatch(ctx, root, run.RunID, ReportProjectionDispatchOptions{})
	if err != nil {
		t.Fatalf("create dispatch: %v", err)
	}
	execution, err := ExecuteReportProjectionDispatch(ctx, root, dispatch.Path, ReportProjectionDispatchExecutionOptions{Execute: true})
	if err != nil {
		t.Fatalf("execute dispatch: %v", err)
	}
	before, err := os.ReadFile(execution.Path)
	if err != nil {
		t.Fatalf("read execution before invalid annotation: %v", err)
	}

	_, err = AnnotateReportProjectionDispatchExecution(root, filepath.Base(execution.Path), ReportProjectionDispatchExecutionAnnotationOptions{Decision: "false-positive"})
	if err == nil || !strings.Contains(err.Error(), "unsupported report projection dispatch execution annotation decision") {
		t.Fatalf("expected false-positive decision rejection, got %v", err)
	}
	after, err := os.ReadFile(execution.Path)
	if err != nil {
		t.Fatalf("read execution after invalid annotation: %v", err)
	}
	if string(after) != string(before) {
		t.Fatalf("false-positive annotation decision must not rewrite execution artifact")
	}
}

func TestClearReportProjectionDispatchExecutionSelectionWritesClearedMetadataOnlyMarker(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	run, err := RunWorkspaceWithOptions(ctx, root, RunOptions{DeferReportProjection: true})
	if err != nil {
		t.Fatalf("run workspace with deferred report projection: %v", err)
	}
	dispatch, err := CreateReportProjectionDispatch(ctx, root, run.RunID, ReportProjectionDispatchOptions{})
	if err != nil {
		t.Fatalf("create dispatch: %v", err)
	}
	execution, err := ExecuteReportProjectionDispatch(ctx, root, dispatch.Path, ReportProjectionDispatchExecutionOptions{Execute: true})
	if err != nil {
		t.Fatalf("execute dispatch: %v", err)
	}
	acceptReportProjectionDispatchExecutionForTest(t, root, execution.Path)
	selected, err := SelectReportProjectionDispatchExecution(root, filepath.Base(execution.Path), ReportProjectionDispatchExecutionSelectionOptions{
		SelectedBy: "alice",
		SelectedAt: time.Date(2026, 6, 2, 6, 55, 0, 0, time.UTC),
		Reason:     "current report",
	})
	if err != nil {
		t.Fatalf("select execution: %v", err)
	}

	cleared, err := ClearReportProjectionDispatchExecutionSelection(root, ReportProjectionDispatchExecutionSelectionClearOptions{
		ClearedBy: "bob",
		ClearedAt: time.Date(2026, 6, 2, 6, 56, 0, 0, time.UTC),
		Reason:    "close false-positive target boundary",
	})
	if err != nil {
		t.Fatalf("clear selection: %v", err)
	}
	if cleared.Path != selected.Path || cleared.Status != "cleared" || cleared.Mode != "metadata-only" {
		t.Fatalf("unexpected cleared marker state: %#v", cleared)
	}
	if cleared.ExecutionPath != execution.Path || cleared.DispatchPath != dispatch.Path || cleared.ReportPath != execution.ReportPath {
		t.Fatalf("cleared marker must preserve selected report context: %#v", cleared)
	}
	if cleared.ClearedBy != "bob" || cleared.ClearedAt != "2026-06-02T06:56:00Z" || cleared.Reason != "close false-positive target boundary" {
		t.Fatalf("unexpected clear operator metadata: %#v", cleared)
	}
	read, found, err := ReadReportProjectionDispatchExecutionSelection(root)
	if err != nil {
		t.Fatalf("read cleared selection: %v", err)
	}
	if !found || read.Status != "cleared" || read.ClearedBy != "bob" || read.ExecutionPath != execution.Path {
		t.Fatalf("unexpected persisted cleared marker: found=%v selection=%#v", found, read)
	}
}

func TestSelectReportProjectionDispatchExecutionRejectsSymlinkedSelectionMarkerBeforeWriting(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	run, err := RunWorkspaceWithOptions(ctx, root, RunOptions{DeferReportProjection: true})
	if err != nil {
		t.Fatalf("run workspace with deferred report projection: %v", err)
	}
	dispatch, err := CreateReportProjectionDispatch(ctx, root, run.RunID, ReportProjectionDispatchOptions{})
	if err != nil {
		t.Fatalf("create dispatch: %v", err)
	}
	execution, err := ExecuteReportProjectionDispatch(ctx, root, dispatch.Path, ReportProjectionDispatchExecutionOptions{Execute: true})
	if err != nil {
		t.Fatalf("execute dispatch: %v", err)
	}
	acceptReportProjectionDispatchExecutionForTest(t, root, execution.Path)
	outside := filepath.Join(t.TempDir(), "outside-selection.json")
	if err := os.WriteFile(outside, []byte("outside\n"), 0o644); err != nil {
		t.Fatalf("write outside selection target: %v", err)
	}
	selectionPath := filepath.Join(root, "reports", "report-projection-selection.json")
	if err := os.Symlink(outside, selectionPath); err != nil {
		t.Fatalf("symlink selection marker: %v", err)
	}

	_, err = SelectReportProjectionDispatchExecution(root, filepath.Base(execution.Path), ReportProjectionDispatchExecutionSelectionOptions{})
	if err == nil || !strings.Contains(err.Error(), "report projection selection file must not be a symlink") {
		t.Fatalf("expected symlinked selection marker rejection, got %v", err)
	}
	outsideData, err := os.ReadFile(outside)
	if err != nil {
		t.Fatalf("read outside selection target: %v", err)
	}
	if string(outsideData) != "outside\n" {
		t.Fatalf("symlinked selection marker must not rewrite outside file, got %q", string(outsideData))
	}
}

func TestAnnotateReportProjectionDispatchExecutionRejectsOutsidePathBeforeWriting(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "workspace")
	run, _, execution := createReportProjectionDispatchExecutionForTest(t, ctx, root)
	before, err := os.ReadFile(execution.Path)
	if err != nil {
		t.Fatalf("read execution before outside annotation: %v", err)
	}
	eventsBefore := reportProjectionDispatchTestEventCount(t, ctx, root, run.RunID)
	outside := filepath.Join(t.TempDir(), "outside-execution.json")
	if err := os.WriteFile(outside, []byte("outside\n"), 0o644); err != nil {
		t.Fatalf("write outside execution target: %v", err)
	}

	_, err = AnnotateReportProjectionDispatchExecution(root, outside, ReportProjectionDispatchExecutionAnnotationOptions{Decision: "accepted"})
	if err == nil || !strings.Contains(err.Error(), "report projection dispatch execution path must stay under") {
		t.Fatalf("expected outside execution path rejection, got %v", err)
	}
	after, err := os.ReadFile(execution.Path)
	if err != nil {
		t.Fatalf("read execution after outside annotation: %v", err)
	}
	if string(after) != string(before) {
		t.Fatalf("outside annotation must not rewrite execution artifact")
	}
	outsideData, err := os.ReadFile(outside)
	if err != nil {
		t.Fatalf("read outside execution target: %v", err)
	}
	if string(outsideData) != "outside\n" {
		t.Fatalf("outside annotation must not rewrite outside file, got %q", string(outsideData))
	}
	if eventsAfter := reportProjectionDispatchTestEventCount(t, ctx, root, run.RunID); eventsAfter != eventsBefore {
		t.Fatalf("outside annotation must not append replay events, before=%d after=%d", eventsBefore, eventsAfter)
	}
}

func TestAnnotateReportProjectionDispatchExecutionRejectsSymlinkedExecutionFileBeforeWriting(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "workspace")
	run, _, execution := createReportProjectionDispatchExecutionForTest(t, ctx, root)
	eventsBefore := reportProjectionDispatchTestEventCount(t, ctx, root, run.RunID)
	outside := filepath.Join(t.TempDir(), "outside-execution.json")
	if err := os.WriteFile(outside, []byte("outside\n"), 0o644); err != nil {
		t.Fatalf("write outside execution target: %v", err)
	}
	if err := os.Remove(execution.Path); err != nil {
		t.Fatalf("remove execution before symlink: %v", err)
	}
	if err := os.Symlink(outside, execution.Path); err != nil {
		t.Fatalf("symlink execution file: %v", err)
	}

	_, err := AnnotateReportProjectionDispatchExecution(root, filepath.Base(execution.Path), ReportProjectionDispatchExecutionAnnotationOptions{Decision: "accepted"})
	if err == nil || !strings.Contains(err.Error(), "report projection dispatch execution file must not be a symlink") {
		t.Fatalf("expected symlinked execution annotation rejection, got %v", err)
	}
	outsideData, err := os.ReadFile(outside)
	if err != nil {
		t.Fatalf("read outside execution target: %v", err)
	}
	if string(outsideData) != "outside\n" {
		t.Fatalf("symlinked annotation must not rewrite outside file, got %q", string(outsideData))
	}
	if _, err := os.Stat(filepath.Join(root, "reports", "report-projection-selection.json")); !os.IsNotExist(err) {
		t.Fatalf("symlinked annotation must not write selection marker, stat err=%v", err)
	}
	if eventsAfter := reportProjectionDispatchTestEventCount(t, ctx, root, run.RunID); eventsAfter != eventsBefore {
		t.Fatalf("symlinked annotation must not append replay events, before=%d after=%d", eventsBefore, eventsAfter)
	}
}

func TestSelectReportProjectionDispatchExecutionRejectsOutsideAndSymlinkedExecutionBeforeMarker(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name  string
		setup func(t *testing.T, root string, execution ReportProjectionDispatchExecution) string
		want  string
	}{
		{
			name: "outside",
			setup: func(t *testing.T, root string, execution ReportProjectionDispatchExecution) string {
				t.Helper()
				outside := filepath.Join(t.TempDir(), "outside-execution.json")
				data, err := os.ReadFile(execution.Path)
				if err != nil {
					t.Fatalf("read execution for outside copy: %v", err)
				}
				if err := os.WriteFile(outside, data, 0o644); err != nil {
					t.Fatalf("write outside execution copy: %v", err)
				}
				return outside
			},
			want: "report projection dispatch execution path must stay under",
		},
		{
			name: "symlink",
			setup: func(t *testing.T, root string, execution ReportProjectionDispatchExecution) string {
				t.Helper()
				outside := filepath.Join(t.TempDir(), "outside-execution.json")
				if err := os.WriteFile(outside, []byte("outside\n"), 0o644); err != nil {
					t.Fatalf("write outside execution target: %v", err)
				}
				if err := os.Remove(execution.Path); err != nil {
					t.Fatalf("remove execution before symlink: %v", err)
				}
				if err := os.Symlink(outside, execution.Path); err != nil {
					t.Fatalf("symlink execution file: %v", err)
				}
				return filepath.Base(execution.Path)
			},
			want: "report projection dispatch execution file must not be a symlink",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "workspace")
			run, _, execution := createReportProjectionDispatchExecutionForTest(t, ctx, root)
			acceptReportProjectionDispatchExecutionForTest(t, root, execution.Path)
			eventsBefore := reportProjectionDispatchTestEventCount(t, ctx, root, run.RunID)
			path := tc.setup(t, root, execution)

			_, err := SelectReportProjectionDispatchExecution(root, path, ReportProjectionDispatchExecutionSelectionOptions{})
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected %q rejection, got %v", tc.want, err)
			}
			if _, err := os.Stat(filepath.Join(root, "reports", "report-projection-selection.json")); !os.IsNotExist(err) {
				t.Fatalf("rejected selection must not write marker, stat err=%v", err)
			}
			if eventsAfter := reportProjectionDispatchTestEventCount(t, ctx, root, run.RunID); eventsAfter != eventsBefore {
				t.Fatalf("rejected selection must not append replay events, before=%d after=%d", eventsBefore, eventsAfter)
			}
		})
	}
}

func TestClearReportProjectionDispatchExecutionSelectionRejectsSymlinkedMarkerBeforeWriting(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	if _, err := RunWorkspaceWithOptions(ctx, root, RunOptions{DeferReportProjection: true}); err != nil {
		t.Fatalf("run workspace with deferred report projection: %v", err)
	}
	outside := filepath.Join(t.TempDir(), "outside-selection.json")
	if err := os.WriteFile(outside, []byte("outside\n"), 0o644); err != nil {
		t.Fatalf("write outside selection target: %v", err)
	}
	selectionPath := filepath.Join(root, "reports", "report-projection-selection.json")
	if err := os.Symlink(outside, selectionPath); err != nil {
		t.Fatalf("symlink selection marker: %v", err)
	}

	_, err := ClearReportProjectionDispatchExecutionSelection(root, ReportProjectionDispatchExecutionSelectionClearOptions{
		ClearedBy: "alice",
		Reason:    "clear symlink check",
	})
	if err == nil || !strings.Contains(err.Error(), "report projection selection file must not be a symlink") {
		t.Fatalf("expected symlinked selection clear rejection, got %v", err)
	}
	outsideData, err := os.ReadFile(outside)
	if err != nil {
		t.Fatalf("read outside selection target: %v", err)
	}
	if string(outsideData) != "outside\n" {
		t.Fatalf("symlinked clear must not rewrite outside file, got %q", string(outsideData))
	}
}

func reportProjectionDispatchTestEventCount(t *testing.T, ctx context.Context, root string, runID string) int {
	t.Helper()
	store, err := eventlog.OpenSQLite(filepath.Join(root, "replay", "events.db"))
	if err != nil {
		t.Fatalf("open event log: %v", err)
	}
	events, err := store.Replay(ctx, runID)
	if closeErr := store.Close(); closeErr != nil && err == nil {
		err = closeErr
	}
	if err != nil {
		t.Fatalf("read replay events: %v", err)
	}
	return len(events)
}

func createReportProjectionDispatchExecutionForTest(t *testing.T, ctx context.Context, root string) (RunResult, ReportProjectionDispatch, ReportProjectionDispatchExecution) {
	t.Helper()
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	run, err := RunWorkspaceWithOptions(ctx, root, RunOptions{DeferReportProjection: true})
	if err != nil {
		t.Fatalf("run workspace with deferred report projection: %v", err)
	}
	dispatch, err := CreateReportProjectionDispatch(ctx, root, run.RunID, ReportProjectionDispatchOptions{})
	if err != nil {
		t.Fatalf("create dispatch: %v", err)
	}
	execution, err := ExecuteReportProjectionDispatch(ctx, root, dispatch.Path, ReportProjectionDispatchExecutionOptions{Execute: true})
	if err != nil {
		t.Fatalf("execute dispatch: %v", err)
	}
	return run, dispatch, execution
}

func acceptReportProjectionDispatchExecutionForTest(t *testing.T, root string, executionPath string) ReportProjectionDispatchExecution {
	t.Helper()
	execution, err := AnnotateReportProjectionDispatchExecution(root, filepath.Base(executionPath), ReportProjectionDispatchExecutionAnnotationOptions{
		Decision:   "accepted",
		Reviewer:   "test",
		ReviewedAt: time.Date(2026, 6, 2, 6, 59, 0, 0, time.UTC),
		Reason:     "accepted for current marker",
	})
	if err != nil {
		t.Fatalf("accept report projection dispatch execution: %v", err)
	}
	return execution
}
