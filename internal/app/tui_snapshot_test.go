package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ouroboros/ouroboros/internal/compiler"
	"github.com/ouroboros/ouroboros/internal/eventlog"
	"github.com/ouroboros/ouroboros/internal/tui"
)

func TestTUIRuntimeActionRequesterWritesPendingInstallRequest(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	missing := "missing-httpx"
	marker := filepath.Join(t.TempDir(), "installer-ran")
	manifest := `name: web-api-pack
version: "0.1.0"
capabilities:
  - web.discovery
runner: native
parser: web_api_parser
healthcheck: built_in
dependencies:
  - name: ` + missing + `
    kind: binary
    install_hint: install bundled httpx pack
    install_command:
      - sh
      - -c
      - touch ` + marker + `
`
	if err := os.WriteFile(filepath.Join(root, "toolchain", "web-api-pack", "manifest.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	tabs := tui.DefaultRuntimeTabs().
		Select(tui.RuntimeTabToolchainHealth).
		WithRows(tui.RuntimeTabToolchainHealth, []string{
			"toolchain: missing",
			"install " + missing + ": missing required=true pack=web-api-pack command=sh -c touch " + marker,
		}).
		SelectRow(1)
	model := tui.NewModel().
		WithGraphStack(tui.GraphStack{RuntimeTabs: tabs}).
		WithRuntimeActionRequester(tuiRuntimeActionRequester(root))
	resized, cmd := model.Update(tea.WindowSizeMsg{Width: 260, Height: 30})
	if cmd != nil {
		t.Fatalf("expected resize update to be synchronous")
	}
	model = resized.(tui.Model)

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	if cmd == nil {
		t.Fatalf("expected runtime action request command")
	}
	updated, cmd = updated.Update(cmd())
	if cmd != nil {
		t.Fatalf("expected runtime action request result to be synchronous")
	}
	view := updated.View()
	if !strings.Contains(view, "request status: pending") {
		t.Fatalf("expected pending request status in TUI view:\n%s", view)
	}

	files, err := filepath.Glob(filepath.Join(root, "toolchain", "install-requests", "*.json"))
	if err != nil {
		t.Fatalf("glob install requests: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("expected one install request artifact, got %#v", files)
	}
	data, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatalf("read install request: %v", err)
	}
	var request ToolchainInstallRequest
	if err := json.Unmarshal(data, &request); err != nil {
		t.Fatalf("decode install request: %v", err)
	}
	if request.Status != "pending" || request.Pack != "web-api-pack" || request.Dependency != missing {
		t.Fatalf("unexpected install request: %#v", request)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("runtime action requester must not execute install command, marker stat err=%v", err)
	}
}

func TestNewTUIModelForWorkspaceShowsPendingInstallRequests(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	missing := "missing-httpx"
	manifest := `name: web-api-pack
version: "0.1.0"
capabilities:
  - web.discovery
runner: native
parser: web_api_parser
healthcheck: built_in
dependencies:
  - name: ` + missing + `
    kind: binary
    install_command:
      - ouro-toolchain
      - install
      - httpx
`
	if err := os.WriteFile(filepath.Join(root, "toolchain", "web-api-pack", "manifest.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	_, err := CreateToolchainInstallRequest(root, "web-api-pack", missing)
	if err != nil {
		t.Fatalf("create install request: %v", err)
	}

	model := NewTUIModelForWorkspace(root).SelectRuntimeTab(tui.RuntimeTabToolchainHealth)
	resized, cmd := model.Update(tea.WindowSizeMsg{Width: 260, Height: 30})
	if cmd != nil {
		t.Fatalf("expected resize update to be synchronous")
	}
	view := resized.View()
	for _, want := range []string{
		"install requests: total=1 pending=1 approved=0 denied=0",
		"requests:p1/a0/d0",
		"request web-api-pack/" + missing + ": pending",
		"request path:",
		"request command: ouro-toolchain install httpx",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected TUI pending request view to contain %q:\n%s", want, view)
		}
	}
}

func TestNewTUIModelForWorkspaceShowsProviderUsageSummaryReadOnly(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	recordProviderUsageSummaryFixture(t, root)

	model := NewTUIModelForWorkspace(root).
		SelectRuntimeTab(tui.RuntimeTabModelRuntime).
		SelectRuntimeRowMatching(tui.RuntimeTabModelRuntime, "provider usage summary:", "")
	resized, cmd := model.Update(tea.WindowSizeMsg{Width: 2200, Height: 40})
	if cmd != nil {
		t.Fatalf("expected resize update to be synchronous")
	}
	view := resized.View()
	for _, want := range []string{
		"provider usage summary: mode=metadata-only records=2",
		"providers=local-openai|second-openai",
		"models=gpt-a|gpt-b",
		"input_tokens=1200",
		"output_tokens=800",
		"total_tokens=2000",
		"estimated_cost_usd=0.007500",
		"estimated_cost_records=1",
		"budget_enforced_records=1",
		"budget_within_limit_records=1",
		"first_created_at=2026-06-04T12:30:00Z",
		"last_created_at=2026-06-04T13:00:00Z",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected TUI provider usage summary view to contain %q:\n%s", want, view)
		}
	}
	for _, leaked := range []string{"do not persist generated text", "Authorization", "Bearer", "secret-value"} {
		if strings.Contains(view, leaked) {
			t.Fatalf("provider usage TUI summary leaked %q:\n%s", leaked, view)
		}
	}

	detailed := resized.(tui.Model).ShowRuntimeActionDetails()
	detailView := detailed.View()
	for _, want := range []string{
		"[Node Inspector]",
		"runtime action: provider-usage-summary",
		"execution mode: metadata-only",
		"source: " + filepath.Join(root, "runtime", "model-usage"),
		"ready: records=2 providers=2 models=2",
		"results: input_tokens=1200 output_tokens=800 total_tokens=2000 estimated_cost_usd=0.007500 estimated_cost_records=1",
		"output: provider usage summary is read-only metadata from manual provider previews",
		"mode: read-only",
	} {
		if !strings.Contains(detailView, want) {
			t.Fatalf("expected TUI provider usage details to contain %q:\n%s", want, detailView)
		}
	}
}

func TestNewTUIModelForWorkspaceProviderUsageSummaryDoesNotMutateReplayEvents(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	recordProviderUsageSummaryFixture(t, root)
	store, err := eventlog.OpenSQLite(filepath.Join(root, "replay", "events.db"))
	if err != nil {
		t.Fatalf("open replay store: %v", err)
	}
	defer store.Close()
	const runID = "run-provider-usage-summary-tui"
	if _, err := store.Append(ctx, eventlog.Event{
		RunID:   runID,
		Type:    eventlog.EventReplay,
		Payload: json.RawMessage(`{"status":"seed"}`),
	}); err != nil {
		t.Fatalf("seed replay event: %v", err)
	}
	eventsBefore, err := store.Replay(ctx, runID)
	if err != nil {
		t.Fatalf("replay before: %v", err)
	}
	recordsBefore, err := ListProviderUsageRecords(root)
	if err != nil {
		t.Fatalf("list usage records before: %v", err)
	}

	model := NewTUIModelForWorkspace(root).SelectRuntimeTab(tui.RuntimeTabModelRuntime)
	resized, cmd := model.Update(tea.WindowSizeMsg{Width: 520, Height: 40})
	if cmd != nil {
		t.Fatalf("expected resize update to be synchronous")
	}
	if view := resized.View(); !strings.Contains(view, "provider usage summary: mode=metadata-only records=2") {
		t.Fatalf("expected provider usage summary row in TUI view:\n%s", view)
	}

	eventsAfter, err := store.Replay(ctx, runID)
	if err != nil {
		t.Fatalf("replay after: %v", err)
	}
	recordsAfter, err := ListProviderUsageRecords(root)
	if err != nil {
		t.Fatalf("list usage records after: %v", err)
	}
	if len(eventsAfter) != len(eventsBefore) {
		t.Fatalf("provider usage TUI summary must not mutate replay events: before=%d after=%d", len(eventsBefore), len(eventsAfter))
	}
	if len(recordsAfter) != len(recordsBefore) {
		t.Fatalf("provider usage TUI summary must not create usage records: before=%d after=%d", len(recordsBefore), len(recordsAfter))
	}
}

func TestNewTUIModelForWorkspaceProviderUsageSummaryDetailsHandleSpacePathsAndNames(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace with space")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	if _, err := RecordProviderGenerationUsage(root, ProviderGenerationResult{
		ProviderName: "local openai",
		Model:        "gpt a",
		InputTokens:  10,
		OutputTokens: 20,
		TotalTokens:  30,
	}, ProviderUsageRecordOptions{Now: time.Date(2026, 6, 4, 14, 0, 0, 0, time.UTC)}); err != nil {
		t.Fatalf("record first usage: %v", err)
	}
	if _, err := RecordProviderGenerationUsage(root, ProviderGenerationResult{
		ProviderName: "pipe|provider",
		Model:        "gpt|b",
		InputTokens:  1,
		OutputTokens: 2,
		TotalTokens:  3,
	}, ProviderUsageRecordOptions{Now: time.Date(2026, 6, 4, 15, 0, 0, 0, time.UTC)}); err != nil {
		t.Fatalf("record second usage: %v", err)
	}

	model := NewTUIModelForWorkspace(root).
		SelectRuntimeTab(tui.RuntimeTabModelRuntime).
		SelectRuntimeRowMatching(tui.RuntimeTabModelRuntime, "provider usage summary:", "")
	resized, cmd := model.Update(tea.WindowSizeMsg{Width: 2200, Height: 40})
	if cmd != nil {
		t.Fatalf("expected resize update to be synchronous")
	}
	detailView := resized.(tui.Model).ShowRuntimeActionDetails().View()
	for _, want := range []string{
		"source: " + filepath.Join(root, "runtime", "model-usage"),
		"ready: records=2 providers=2 models=2",
		"results: input_tokens=11 output_tokens=22 total_tokens=33",
	} {
		if !strings.Contains(detailView, want) {
			t.Fatalf("expected provider usage summary details to contain %q:\n%s", want, detailView)
		}
	}
}

func recordProviderUsageSummaryFixture(t *testing.T, root string) {
	t.Helper()
	maxCost := 0.01
	if _, err := RecordProviderGenerationUsage(root, ProviderGenerationResult{
		ProviderName:           "local-openai",
		Model:                  "gpt-a",
		Text:                   "do not persist generated text Authorization Bearer secret-value",
		InputTokens:            1000,
		OutputTokens:           500,
		TotalTokens:            1500,
		EstimatedCostUSD:       0.0075,
		EstimatedCostAvailable: true,
	}, ProviderUsageRecordOptions{
		Now:                 time.Date(2026, 6, 4, 12, 30, 0, 0, time.UTC),
		MaxEstimatedCostUSD: &maxCost,
	}); err != nil {
		t.Fatalf("record first usage: %v", err)
	}
	if _, err := RecordProviderGenerationUsage(root, ProviderGenerationResult{
		ProviderName: "second-openai",
		Model:        "gpt-b",
		InputTokens:  200,
		OutputTokens: 300,
		TotalTokens:  500,
	}, ProviderUsageRecordOptions{Now: time.Date(2026, 6, 4, 13, 0, 0, 0, time.UTC)}); err != nil {
		t.Fatalf("record second usage: %v", err)
	}
}

func TestNewTUIModelForWorkspaceShowsCompilePreviewRowsMetadataOnly(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	marker := filepath.Join(t.TempDir(), "compile-preview-ran")
	manifest := `name: web-api-pack
version: "0.1.0"
capabilities:
  - web.discovery
runner: command
run_command:
  - sh
  - -c
  - touch ` + marker + `
parser: web_api_parser
healthcheck: built_in
`
	if err := os.WriteFile(filepath.Join(root, "toolchain", "web-api-pack", "manifest.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	model := NewTUIModelForWorkspace(root).SelectRuntimeTab(tui.RuntimeTabToolchainHealth)
	resized, cmd := model.Update(tea.WindowSizeMsg{Width: 520, Height: 40})
	if cmd != nil {
		t.Fatalf("expected resize update to be synchronous")
	}
	view := resized.View()
	for _, want := range []string{
		"compile preview: action=discover_assets mode=metadata-only nodes=7 warnings=0",
		"compile candidate: discover_assets:01 manifest=web-api-pack tool=pack adapter=none operation=none capability=web.discovery",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected TUI compile preview view to contain %q:\n%s", want, view)
		}
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("compile preview TUI rows must not execute run_command, marker stat err=%v", err)
	}
}

func TestNewTUIModelForWorkspaceShowsCompilePreviewSignalsDetails(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	marker := filepath.Join(t.TempDir(), "compile-preview-signals-ran")
	manifest := `name: web-api-pack
version: "0.1.0"
capabilities:
  - web.discovery
runner: command
run_command:
  - sh
  - -c
  - touch ` + marker + `
parser: web_api_parser
healthcheck: built_in
`
	if err := os.WriteFile(filepath.Join(root, "toolchain", "web-api-pack", "manifest.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	signals := `{
  "schema_version": 1,
  "budget": {"max_plan_nodes": 1},
  "domain_priorities": [{"domain": "web_api", "weight": 2.5}],
  "frontier_scores": [{
    "manifest_name": "web-api-pack",
    "capability": "web.discovery",
    "path": {
      "id": "frontier-web",
      "target": "https://app.example.test",
      "hypothesis": "reviewed auth surface",
      "mission_value": 0.9,
      "unlock_potential": 0.8,
      "evidence_confidence": 0.7,
      "novelty": 0.6
    }
  }],
  "historical_stats": [{"manifest_name": "web-api-pack", "capability": "web.discovery", "successes": 3, "failures": 1}]
}`
	if err := os.WriteFile(filepath.Join(root, "toolchain", "compile-preview-signals.json"), []byte(signals), 0o644); err != nil {
		t.Fatalf("write compile preview signals: %v", err)
	}

	model := NewTUIModelForWorkspace(root).SelectRuntimeTab(tui.RuntimeTabToolchainHealth)
	resized, cmd := model.Update(tea.WindowSizeMsg{Width: 720, Height: 42})
	if cmd != nil {
		t.Fatalf("expected resize update to be synchronous")
	}
	view := resized.View()
	for _, want := range []string{
		"compile-preview signals: present",
		"max_plan_nodes=1 domain_priorities=1 frontier_scores=1 historical_stats=1",
		"compile-preview signal domain: domain=web_api weight=2.5",
		"compile-preview signal frontier: manifest=web-api-pack capability=web.discovery path=frontier-web target=https://app.example.test",
		"compile-preview signal history: manifest=web-api-pack capability=web.discovery successes=3 failures=1",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected TUI compile-preview signal view to contain %q:\n%s", want, view)
		}
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("compile-preview signal TUI rows must not execute run_command, marker stat err=%v", err)
	}

	model = selectRuntimeRowContaining(t, model, "> compile-preview signals: present")
	model = model.ShowRuntimeActionDetails()
	resized, cmd = model.Update(tea.WindowSizeMsg{Width: 720, Height: 42})
	if cmd != nil {
		t.Fatalf("expected runtime action details resize to be synchronous")
	}
	view = resized.View()
	for _, want := range []string{
		"[Node Inspector]",
		"runtime action: compile-preview-signals",
		"artifact path: " + filepath.Join(root, "toolchain", "compile-preview-signals.json"),
		"execution mode: metadata-only",
		"max plan nodes: 1",
		"domain priorities: 1",
		"frontier scores: 1",
		"historical stats: 1",
		"mode: metadata-only",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected compile-preview signal details %q:\n%s", want, view)
		}
	}
}

func TestNewTUIModelForWorkspaceShowsReplayWorkbenchSessionArtifacts(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	primary, err := RunWorkspace(context.Background(), root)
	if err != nil {
		t.Fatalf("run primary workspace: %v", err)
	}
	comparison, err := RunWorkspace(context.Background(), root)
	if err != nil {
		t.Fatalf("run comparison workspace: %v", err)
	}
	session, err := CreateReplayWorkbenchSession(context.Background(), root, ReplayWorkbenchSessionOptions{
		PrimaryRunID:  primary.RunID,
		CompareRunIDs: []string{comparison.RunID},
		SelectedRunID: comparison.RunID,
		Filters: map[string]string{
			"lane": "toolchain",
			"q":    "needle",
			"edge": "edge-1",
		},
		CreatedBy: "operator",
		Reason:    "saved comparison focus",
	})
	if err != nil {
		t.Fatalf("create replay workbench session: %v", err)
	}
	session, err = AnnotateReplayWorkbenchSession(root, filepath.Base(session.Path), ReplayWorkbenchSessionAnnotationOptions{
		Decision:   "accepted",
		Reason:     "authorized comparison baseline",
		Reviewer:   "operator",
		ReviewedAt: time.Date(2026, 6, 1, 2, 35, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("annotate replay workbench session: %v", err)
	}
	if session.Path == "" {
		t.Fatalf("expected replay workbench session artifact path")
	}
	before, err := ReplayWorkspace(context.Background(), root, primary.RunID)
	if err != nil {
		t.Fatalf("replay before TUI session rows: %v", err)
	}

	model := NewTUIModelForWorkspace(root).SelectRuntimeTab(tui.RuntimeTabTimelineLanes)
	resized, cmd := model.Update(tea.WindowSizeMsg{Width: 760, Height: 44})
	if cmd != nil {
		t.Fatalf("expected resize update to be synchronous")
	}
	view := resized.View()
	for _, want := range []string{
		"replay workbench sessions: total=1 saved=1",
		"replay workbench session artifact: status=saved mode=metadata-only primary=" + primary.RunID + " selected=" + comparison.RunID,
		"runs=2 filters=3 endpoints=9 created_by=operator",
		"decision=accepted reviewer=operator",
		"replay workbench session reason: saved comparison focus",
		"replay workbench session filter: edge=edge-1",
		"replay workbench session endpoint: html=/?edge=edge-1&lane=toolchain&q=needle&run=" + comparison.RunID,
		"replay workbench session endpoint: provider_usage_summary=/api/provider-usage-summary",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected TUI replay workbench session row %q:\n%s", want, view)
		}
	}

	model = selectRuntimeRowContaining(t, model, "> replay workbench session artifact:")
	model = model.ShowRuntimeActionDetails()
	resized, cmd = model.Update(tea.WindowSizeMsg{Width: 760, Height: 44})
	if cmd != nil {
		t.Fatalf("expected runtime action details resize to be synchronous")
	}
	view = resized.View()
	for _, want := range []string{
		"[Node Inspector]",
		"runtime action: replay-workbench-session-artifact",
		"artifact path: ",
		"workbench-sessions/replay-workbench-session",
		"state: saved",
		"execution mode: metadata-only",
		"run: " + primary.RunID,
		"selected run: " + comparison.RunID,
		"run ids: 2",
		"filters: 3",
		"endpoints: 9",
		"created by: operator",
		"reason: saved comparison focus",
		"decision: accepted",
		"reviewer: operator",
		"reviewed at: 2026-06-01T02:35:00Z",
		"review reason: authorized comparison baseline",
		"mode: metadata-only",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected replay workbench session details %q:\n%s", want, view)
		}
	}

	after, err := ReplayWorkspace(context.Background(), root, primary.RunID)
	if err != nil {
		t.Fatalf("replay after TUI session rows: %v", err)
	}
	if after.EventCount != before.EventCount {
		t.Fatalf("TUI replay workbench session rows must not mutate replay events, before=%d after=%d", before.EventCount, after.EventCount)
	}
}

func TestNewTUIModelForWorkspaceShowsReplayWorkbenchCollaborationArtifacts(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	run, err := RunWorkspace(context.Background(), root)
	if err != nil {
		t.Fatalf("run workspace: %v", err)
	}
	session, err := CreateReplayWorkbenchSession(context.Background(), root, ReplayWorkbenchSessionOptions{
		PrimaryRunID: run.RunID,
		Filters: map[string]string{
			"lane": "timeline",
			"q":    "collab",
		},
		CreatedBy: "operator",
		Reason:    "saved collaboration baseline",
	})
	if err != nil {
		t.Fatalf("create replay workbench session: %v", err)
	}
	collaboration, err := CreateReplayWorkbenchCollaboration(context.Background(), root, ReplayWorkbenchCollaborationOptions{
		SessionPath:  filepath.Base(session.Path),
		Participants: []string{"alice", "bob"},
		CreatedBy:    "lead",
		CreatedAt:    time.Date(2026, 6, 1, 3, 45, 0, 0, time.UTC),
		Reason:       "pair review",
	})
	if err != nil {
		t.Fatalf("create replay workbench collaboration: %v", err)
	}
	if collaboration.Path == "" {
		t.Fatalf("expected replay workbench collaboration artifact path")
	}
	if _, err := AnnotateReplayWorkbenchCollaboration(root, filepath.Base(collaboration.Path), ReplayWorkbenchCollaborationAnnotationOptions{
		Decision:   "accepted",
		Reason:     "authorized pair review",
		Reviewer:   "alice",
		ReviewedAt: time.Date(2026, 6, 1, 5, 10, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("annotate replay workbench collaboration: %v", err)
	}
	before, err := ReplayWorkspace(context.Background(), root, run.RunID)
	if err != nil {
		t.Fatalf("replay before TUI collaboration rows: %v", err)
	}

	model := NewTUIModelForWorkspace(root).SelectRuntimeTab(tui.RuntimeTabTimelineLanes)
	resized, cmd := model.Update(tea.WindowSizeMsg{Width: 760, Height: 44})
	if cmd != nil {
		t.Fatalf("expected resize update to be synchronous")
	}
	view := resized.View()
	for _, want := range []string{
		"replay workbench collaborations: total=1 saved=1 dry-run=0 participants=2",
		"replay workbench collaboration artifact: status=saved mode=metadata-only primary=" + run.RunID,
		"primary=" + run.RunID + " selected=" + run.RunID,
		"runs=1 filters=2 endpoints=10 participants=2 created_by=lead",
		"replay workbench collaboration annotation: decision=accepted reviewer=alice reviewed_at=2026-06-01T05:10:00Z reason=authorized pair review",
		"replay workbench collaboration review: decision=accepted reviewer=alice reviewed_at=2026-06-01T05:10:00Z reason=authorized pair review",
		"replay workbench collaboration participant: alice",
		"replay workbench collaboration participant: bob",
		"replay workbench collaboration endpoint: collaborations=/api/collaborations",
		"replay workbench collaboration reason: pair review",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected TUI replay workbench collaboration row %q:\n%s", want, view)
		}
	}

	model = selectRuntimeRowContaining(t, model, "> replay workbench collaboration artifact:")
	model = model.ShowRuntimeActionDetails()
	resized, cmd = model.Update(tea.WindowSizeMsg{Width: 760, Height: 44})
	if cmd != nil {
		t.Fatalf("expected runtime action details resize to be synchronous")
	}
	view = resized.View()
	for _, want := range []string{
		"[Node Inspector]",
		"runtime action: replay-workbench-collaboration-artifact",
		"artifact path: ",
		"workbench-collaborations/replay-workbench-collaboration",
		"session path: ",
		"workbench-sessions/replay-workbench-session",
		"state: saved",
		"execution mode: metadata-only",
		"run: " + run.RunID,
		"selected run: " + run.RunID,
		"run ids: 1",
		"filters: 2",
		"endpoints: 10",
		"participants: 2",
		"created by: lead",
		"reason: pair review",
		"decision: accepted",
		"reviewer: alice",
		"reviewed at: 2026-06-01T05:10:00Z",
		"review reason: authorized pair review",
		"mode: metadata-only",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected replay workbench collaboration details %q:\n%s", want, view)
		}
	}

	after, err := ReplayWorkspace(context.Background(), root, run.RunID)
	if err != nil {
		t.Fatalf("replay after TUI collaboration rows: %v", err)
	}
	if after.EventCount != before.EventCount {
		t.Fatalf("TUI replay workbench collaboration rows must not mutate replay events, before=%d after=%d", before.EventCount, after.EventCount)
	}
}

func TestNewTUIModelForWorkspaceShowsReplayWorkbenchCollaborationPresence(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	run, err := RunWorkspace(context.Background(), root)
	if err != nil {
		t.Fatalf("run workspace: %v", err)
	}
	session, err := CreateReplayWorkbenchSession(context.Background(), root, ReplayWorkbenchSessionOptions{
		PrimaryRunID: run.RunID,
		CreatedBy:    "operator",
		Reason:       "saved collaboration baseline",
	})
	if err != nil {
		t.Fatalf("create replay workbench session: %v", err)
	}
	collaboration, err := CreateReplayWorkbenchCollaboration(context.Background(), root, ReplayWorkbenchCollaborationOptions{
		SessionPath:  filepath.Base(session.Path),
		Participants: []string{"alice", "bob"},
		CreatedBy:    "lead",
		CreatedAt:    time.Date(2026, 6, 1, 3, 45, 0, 0, time.UTC),
		Reason:       "pair review",
	})
	if err != nil {
		t.Fatalf("create replay workbench collaboration: %v", err)
	}
	presence, err := UpdateReplayWorkbenchCollaborationPresence(context.Background(), root, ReplayWorkbenchCollaborationPresenceOptions{
		CollaborationPath: filepath.Base(collaboration.Path),
		Participant:       "alice",
		Status:            "online",
		SeenBy:            "lead",
		SeenAt:            time.Date(2026, 6, 1, 5, 30, 0, 0, time.UTC),
		Reason:            "review active",
	})
	if err != nil {
		t.Fatalf("update replay workbench collaboration presence: %v", err)
	}
	if presence.Path == "" {
		t.Fatalf("expected replay workbench collaboration presence artifact path")
	}
	before, err := ReplayWorkspace(context.Background(), root, run.RunID)
	if err != nil {
		t.Fatalf("replay before TUI collaboration presence rows: %v", err)
	}

	model := NewTUIModelForWorkspace(root).SelectRuntimeTab(tui.RuntimeTabTimelineLanes)
	resized, cmd := model.Update(tea.WindowSizeMsg{Width: 760, Height: 44})
	if cmd != nil {
		t.Fatalf("expected resize update to be synchronous")
	}
	view := resized.View()
	for _, want := range []string{
		"replay workbench collaboration presence: total=1 online=1 away=0 offline=0",
		"replay workbench collaboration presence artifact: status=online mode=metadata-only primary=" + run.RunID,
		"participant=alice seen_by=lead seen_at=2026-06-01T05:30:00Z",
		"replay workbench collaboration presence participant: alice status=online seen_by=lead seen_at=2026-06-01T05:30:00Z reason=review active",
		"replay workbench collaboration presence reason: review active",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected TUI replay workbench collaboration presence row %q:\n%s", want, view)
		}
	}

	model = selectRuntimeRowContaining(t, model, "> replay workbench collaboration presence artifact:")
	model = model.ShowRuntimeActionDetails()
	resized, cmd = model.Update(tea.WindowSizeMsg{Width: 760, Height: 44})
	if cmd != nil {
		t.Fatalf("expected runtime action details resize to be synchronous")
	}
	view = resized.View()
	for _, want := range []string{
		"[Node Inspector]",
		"runtime action: replay-workbench-collaboration-presence",
		"artifact path: ",
		"workbench-collaboration-presence",
		"collaboration path: ",
		"workbench-collaborations",
		"session path: ",
		"workbench-sessions",
		"state: online",
		"execution mode: metadata-only",
		"run: " + run.RunID,
		"selected run: " + run.RunID,
		"participant: alice",
		"seen by: lead",
		"seen at: 2026-06-01T05:30:00Z",
		"reason: review active",
		"mode: metadata-only",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected replay workbench collaboration presence details %q:\n%s", want, view)
		}
	}

	after, err := ReplayWorkspace(context.Background(), root, run.RunID)
	if err != nil {
		t.Fatalf("replay after TUI collaboration presence rows: %v", err)
	}
	if after.EventCount != before.EventCount {
		t.Fatalf("TUI replay workbench collaboration presence rows must not mutate replay events, before=%d after=%d", before.EventCount, after.EventCount)
	}
}

func TestNewTUIModelForWorkspaceShowsReplayWorkbenchCollaborationNotes(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	run, err := RunWorkspace(context.Background(), root)
	if err != nil {
		t.Fatalf("run workspace: %v", err)
	}
	session, err := CreateReplayWorkbenchSession(context.Background(), root, ReplayWorkbenchSessionOptions{
		PrimaryRunID: run.RunID,
		Filters:      map[string]string{"node": "fact-target"},
		Reason:       "saved collaboration baseline",
	})
	if err != nil {
		t.Fatalf("create replay workbench session: %v", err)
	}
	collaboration, err := CreateReplayWorkbenchCollaboration(context.Background(), root, ReplayWorkbenchCollaborationOptions{
		SessionPath:  filepath.Base(session.Path),
		Participants: []string{"alice", "bob"},
		CreatedBy:    "lead",
		CreatedAt:    time.Date(2026, 6, 1, 8, 10, 0, 0, time.UTC),
		Reason:       "note pair review",
	})
	if err != nil {
		t.Fatalf("create replay workbench collaboration: %v", err)
	}
	note, err := CreateReplayWorkbenchCollaborationNote(context.Background(), root, ReplayWorkbenchCollaborationNoteOptions{
		CollaborationPath: filepath.Base(collaboration.Path),
		Author:            "alice",
		Body:              "Confirm graph node before accepting.",
		TargetKind:        "graph-node",
		TargetID:          "fact-target",
		CreatedAt:         time.Date(2026, 6, 1, 8, 20, 0, 0, time.UTC),
		Reason:            "collaborative evidence review",
	})
	if err != nil {
		t.Fatalf("create replay workbench collaboration note: %v", err)
	}
	note, err = AnnotateReplayWorkbenchCollaborationNote(root, filepath.Base(note.Path), ReplayWorkbenchCollaborationNoteAnnotationOptions{
		Decision:   "accepted",
		Reviewer:   "alice",
		ReviewedAt: time.Date(2026, 6, 1, 9, 30, 0, 0, time.UTC),
		Reason:     "validated evidence note",
	})
	if err != nil {
		t.Fatalf("annotate replay workbench collaboration note: %v", err)
	}
	if _, err := SelectReplayWorkbenchCollaborationNote(root, filepath.Base(note.Path), ReplayWorkbenchCollaborationNoteSelectionOptions{
		SelectedBy: "alice",
		SelectedAt: time.Date(2026, 6, 1, 9, 50, 0, 0, time.UTC),
		Reason:     "current note focus",
	}); err != nil {
		t.Fatalf("select replay workbench collaboration note: %v", err)
	}
	before, err := ReplayWorkspace(context.Background(), root, run.RunID)
	if err != nil {
		t.Fatalf("replay before TUI collaboration note rows: %v", err)
	}

	model := NewTUIModelForWorkspace(root).SelectRuntimeTab(tui.RuntimeTabTimelineLanes)
	resized, cmd := model.Update(tea.WindowSizeMsg{Width: 760, Height: 44})
	if cmd != nil {
		t.Fatalf("expected resize update to be synchronous")
	}
	view := resized.View()
	for _, want := range []string{
		"replay workbench collaboration notes: total=1 saved=1 dry-run=0 authors=1",
		"replay workbench collaboration note selection: status=current mode=metadata-only",
		"replay workbench collaboration note selection actor: selected_by=alice selected_at=2026-06-01T09:50:00Z",
		"replay workbench collaboration note current marker: current=true note=",
		"replay workbench collaboration note artifact: status=saved mode=metadata-only primary=" + run.RunID,
		"current=true",
		"author=alice target=graph-node:fact-target created_at=2026-06-01T08:20:00Z",
		"replay workbench collaboration note review: decision=accepted reviewer=alice reviewed_at=2026-06-01T09:30:00Z reason=validated evidence note",
		"reason=collaborative evidence review body=Confirm graph node before accepting.",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected TUI replay workbench collaboration note row %q:\n%s", want, view)
		}
	}

	model = selectRuntimeRowContaining(t, model, "> replay workbench collaboration note selection:")
	model = model.ShowRuntimeActionDetails()
	resized, cmd = model.Update(tea.WindowSizeMsg{Width: 760, Height: 44})
	if cmd != nil {
		t.Fatalf("expected runtime action selection details resize to be synchronous")
	}
	view = resized.View()
	for _, want := range []string{
		"[Node Inspector]",
		"runtime action: replay-workbench-collaboration-note-selection",
		"artifact path: ",
		"workbench-collaboration-notes",
		"collaboration path: ",
		"workbench-collaborations",
		"session path: ",
		"workbench-sessions",
		"state: current",
		"author: alice",
		"target kind: graph-node",
		"target id: fact-target",
		"selected by: alice",
		"selected at: 2026-06-01T09:50:00Z",
		"reason: current note focus",
		"mode: metadata-only",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected replay workbench collaboration note selection details %q:\n%s", want, view)
		}
	}

	model = model.SelectRuntimeTab(tui.RuntimeTabTimelineLanes)
	model = selectRuntimeRowContaining(t, model, "> replay workbench collaboration note artifact:")
	model = model.ShowRuntimeActionDetails()
	resized, cmd = model.Update(tea.WindowSizeMsg{Width: 760, Height: 44})
	if cmd != nil {
		t.Fatalf("expected runtime action details resize to be synchronous")
	}
	view = resized.View()
	for _, want := range []string{
		"[Node Inspector]",
		"runtime action: replay-workbench-collaboration-note",
		"artifact path: ",
		"workbench-collaboration-notes",
		"collaboration path: ",
		"workbench-collaborations",
		"session path: ",
		"workbench-sessions",
		"state: saved",
		"execution mode: metadata-only",
		"run: " + run.RunID,
		"selected run: " + run.RunID,
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
			t.Fatalf("expected replay workbench collaboration note details %q:\n%s", want, view)
		}
	}

	after, err := ReplayWorkspace(context.Background(), root, run.RunID)
	if err != nil {
		t.Fatalf("replay after TUI collaboration note rows: %v", err)
	}
	if after.EventCount != before.EventCount {
		t.Fatalf("TUI replay workbench collaboration note rows must not mutate replay events, before=%d after=%d note=%s", before.EventCount, after.EventCount, note.Path)
	}
}

func TestNewTUIModelForWorkspaceSelectsReplayWorkbenchCollaborationNoteAfterDoubleConfirm(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	run, err := RunWorkspace(context.Background(), root)
	if err != nil {
		t.Fatalf("run workspace: %v", err)
	}
	session, err := CreateReplayWorkbenchSession(context.Background(), root, ReplayWorkbenchSessionOptions{
		PrimaryRunID: run.RunID,
		Reason:       "saved note baseline",
	})
	if err != nil {
		t.Fatalf("create replay workbench session: %v", err)
	}
	collaboration, err := CreateReplayWorkbenchCollaboration(context.Background(), root, ReplayWorkbenchCollaborationOptions{
		SessionPath:  filepath.Base(session.Path),
		Participants: []string{"alice", "bob"},
		Reason:       "candidate pair review",
	})
	if err != nil {
		t.Fatalf("create replay workbench collaboration: %v", err)
	}
	note, err := CreateReplayWorkbenchCollaborationNote(context.Background(), root, ReplayWorkbenchCollaborationNoteOptions{
		CollaborationPath: filepath.Base(collaboration.Path),
		Author:            "alice",
		Body:              "Confirm graph node before accepting.",
		TargetKind:        "graph-node",
		TargetID:          "fact-target",
		Reason:            "candidate note review",
	})
	if err != nil {
		t.Fatalf("create replay workbench collaboration note: %v", err)
	}

	model := NewTUIModelForWorkspace(root).SelectRuntimeTab(tui.RuntimeTabTimelineLanes)
	model = selectRuntimeRowContaining(t, model, "> replay workbench collaboration note artifact:")
	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if cmd != nil {
		t.Fatalf("first x should only arm replay workbench collaboration note selection")
	}
	next, ok := updated.(tui.Model)
	if !ok {
		t.Fatalf("expected tui.Model, got %T", updated)
	}
	resized, resizeCmd := next.Update(tea.WindowSizeMsg{Width: 860, Height: 48})
	if resizeCmd != nil {
		t.Fatalf("expected confirm resize to be synchronous")
	}
	next, ok = resized.(tui.Model)
	if !ok {
		t.Fatalf("expected tui.Model after confirm resize, got %T", resized)
	}
	confirmView := next.View()
	if !strings.Contains(confirmView, "execution status: confirm-required") || !strings.Contains(confirmView, "action: replay workbench collaboration note selected") {
		t.Fatalf("expected confirm-required replay workbench collaboration note action:\n%s", confirmView)
	}
	updated, cmd = next.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if cmd == nil {
		t.Fatalf("second x should execute replay workbench collaboration note selection")
	}
	msg := cmd()
	updated, cmd = updated.Update(msg)
	if cmd != nil {
		t.Fatalf("expected replay workbench collaboration note selection result to be synchronous")
	}
	selection, found, err := ReadReplayWorkbenchCollaborationNoteSelection(root)
	if err != nil {
		t.Fatalf("read replay workbench collaboration note selection: %v", err)
	}
	if !found || selection.Status != "current" || selection.NotePath != note.Path || selection.SelectedBy != "tui" || selection.Reason != "selected from TUI" {
		t.Fatalf("expected TUI-selected replay workbench collaboration note, found=%v selection=%#v", found, selection)
	}
	if _, ok := updated.(tui.Model); !ok {
		t.Fatalf("expected tui.Model after result, got %T", updated)
	}
	refreshedModel := NewTUIModelForWorkspace(root).SelectRuntimeTab(tui.RuntimeTabTimelineLanes)
	resized, resizeCmd = refreshedModel.Update(tea.WindowSizeMsg{Width: 860, Height: 48})
	if resizeCmd != nil {
		t.Fatalf("expected result resize to be synchronous")
	}
	resultView := resized.View()
	if !strings.Contains(resultView, "replay workbench collaboration note selection: status=current") {
		t.Fatalf("expected selected current replay workbench collaboration note row:\n%s", resultView)
	}
}

func TestNewTUIModelForWorkspaceClearsReplayWorkbenchCollaborationNoteSelectionAfterDoubleConfirm(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	run, err := RunWorkspace(context.Background(), root)
	if err != nil {
		t.Fatalf("run workspace: %v", err)
	}
	session, err := CreateReplayWorkbenchSession(context.Background(), root, ReplayWorkbenchSessionOptions{
		PrimaryRunID: run.RunID,
		Reason:       "saved note baseline",
	})
	if err != nil {
		t.Fatalf("create replay workbench session: %v", err)
	}
	collaboration, err := CreateReplayWorkbenchCollaboration(context.Background(), root, ReplayWorkbenchCollaborationOptions{
		SessionPath:  filepath.Base(session.Path),
		Participants: []string{"alice", "bob"},
		Reason:       "candidate pair review",
	})
	if err != nil {
		t.Fatalf("create replay workbench collaboration: %v", err)
	}
	note, err := CreateReplayWorkbenchCollaborationNote(context.Background(), root, ReplayWorkbenchCollaborationNoteOptions{
		CollaborationPath: filepath.Base(collaboration.Path),
		Author:            "alice",
		Body:              "Confirm graph node before accepting.",
		Reason:            "candidate note review",
	})
	if err != nil {
		t.Fatalf("create replay workbench collaboration note: %v", err)
	}
	if _, err := SelectReplayWorkbenchCollaborationNote(root, filepath.Base(note.Path), ReplayWorkbenchCollaborationNoteSelectionOptions{
		SelectedBy: "alice",
		Reason:     "current note focus",
	}); err != nil {
		t.Fatalf("select replay workbench collaboration note: %v", err)
	}

	model := NewTUIModelForWorkspace(root).SelectRuntimeTab(tui.RuntimeTabTimelineLanes)
	model = selectRuntimeRowContaining(t, model, "> replay workbench collaboration note selection:")
	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if cmd != nil {
		t.Fatalf("first x should only arm replay workbench collaboration note clear")
	}
	next, ok := updated.(tui.Model)
	if !ok {
		t.Fatalf("expected tui.Model, got %T", updated)
	}
	resized, resizeCmd := next.Update(tea.WindowSizeMsg{Width: 860, Height: 48})
	if resizeCmd != nil {
		t.Fatalf("expected confirm resize to be synchronous")
	}
	next, ok = resized.(tui.Model)
	if !ok {
		t.Fatalf("expected tui.Model after confirm resize, got %T", resized)
	}
	confirmView := next.View()
	if !strings.Contains(confirmView, "execution status: confirm-required") || !strings.Contains(confirmView, "action: replay workbench collaboration note selection selected") {
		t.Fatalf("expected confirm-required replay workbench collaboration note selection action:\n%s", confirmView)
	}
	updated, cmd = next.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if cmd == nil {
		t.Fatalf("second x should execute replay workbench collaboration note selection clear")
	}
	msg := cmd()
	updated, cmd = updated.Update(msg)
	if cmd != nil {
		t.Fatalf("expected replay workbench collaboration note clear result to be synchronous")
	}
	selection, found, err := ReadReplayWorkbenchCollaborationNoteSelection(root)
	if err != nil {
		t.Fatalf("read replay workbench collaboration note selection: %v", err)
	}
	if !found || selection.Status != "cleared" || selection.NotePath != note.Path || selection.ClearedBy != "tui" || selection.Reason != "cleared from TUI" {
		t.Fatalf("expected TUI-cleared replay workbench collaboration note selection, found=%v selection=%#v", found, selection)
	}
	if _, ok := updated.(tui.Model); !ok {
		t.Fatalf("expected tui.Model after clear result, got %T", updated)
	}
	refreshedModel := NewTUIModelForWorkspace(root).SelectRuntimeTab(tui.RuntimeTabTimelineLanes)
	resized, resizeCmd = refreshedModel.Update(tea.WindowSizeMsg{Width: 860, Height: 48})
	if resizeCmd != nil {
		t.Fatalf("expected clear result resize to be synchronous")
	}
	resultView := resized.View()
	if !strings.Contains(resultView, "replay workbench collaboration note selection: status=cleared") || strings.Contains(resultView, "replay workbench collaboration note current marker: current=true") {
		t.Fatalf("expected cleared replay workbench collaboration note selection without stale current row:\n%s", resultView)
	}
}

func TestNewTUIModelForWorkspaceShowsReplayWorkbenchCollaborationSelection(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	run, err := RunWorkspace(context.Background(), root)
	if err != nil {
		t.Fatalf("run workspace: %v", err)
	}
	session, err := CreateReplayWorkbenchSession(context.Background(), root, ReplayWorkbenchSessionOptions{
		PrimaryRunID: run.RunID,
		Filters:      map[string]string{"lane": "timeline"},
		Reason:       "saved collaboration baseline",
	})
	if err != nil {
		t.Fatalf("create replay workbench session: %v", err)
	}
	if _, err := CreateReplayWorkbenchCollaboration(context.Background(), root, ReplayWorkbenchCollaborationOptions{
		SessionPath:  filepath.Base(session.Path),
		Participants: []string{"alice"},
		CreatedAt:    time.Date(2026, 6, 1, 3, 50, 0, 0, time.UTC),
		Reason:       "baseline pair review",
	}); err != nil {
		t.Fatalf("create baseline replay workbench collaboration: %v", err)
	}
	candidate, err := CreateReplayWorkbenchCollaboration(context.Background(), root, ReplayWorkbenchCollaborationOptions{
		SessionPath:  filepath.Base(session.Path),
		Participants: []string{"alice", "bob"},
		CreatedBy:    "lead",
		CreatedAt:    time.Date(2026, 6, 1, 3, 51, 0, 0, time.UTC),
		Reason:       "current pair review",
	})
	if err != nil {
		t.Fatalf("create candidate replay workbench collaboration: %v", err)
	}
	if _, err := SelectReplayWorkbenchCollaboration(root, filepath.Base(candidate.Path), ReplayWorkbenchCollaborationSelectionOptions{
		SelectedBy: "alice",
		SelectedAt: time.Date(2026, 6, 1, 3, 52, 0, 0, time.UTC),
		Reason:     "current collaboration focus",
	}); err != nil {
		t.Fatalf("select replay workbench collaboration: %v", err)
	}

	model := NewTUIModelForWorkspace(root).SelectRuntimeTab(tui.RuntimeTabTimelineLanes)
	resized, cmd := model.Update(tea.WindowSizeMsg{Width: 860, Height: 48})
	if cmd != nil {
		t.Fatalf("expected resize update to be synchronous")
	}
	view := resized.View()
	for _, want := range []string{
		"replay workbench collaboration selection: status=current mode=metadata-only",
		"replay workbench collaboration selection actor: selected_by=alice selected_at=2026-06-01T03:52:00Z",
		"replay workbench collaboration selection reason: current collaboration focus",
		"replay workbench collaboration superseded:",
		"replay workbench collaboration artifact: status=saved mode=metadata-only primary=" + run.RunID,
		"current=true",
		"current=false",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected replay workbench collaboration selection row %q:\n%s", want, view)
		}
	}

	model = selectRuntimeRowContaining(t, model, "> replay workbench collaboration selection:")
	model = model.ShowRuntimeActionDetails()
	resized, cmd = model.Update(tea.WindowSizeMsg{Width: 860, Height: 48})
	if cmd != nil {
		t.Fatalf("expected runtime action details resize to be synchronous")
	}
	view = resized.View()
	for _, want := range []string{
		"[Node Inspector]",
		"runtime action: replay-workbench-collaboration-selection",
		"artifact path: " + candidate.Path,
		"session path: " + session.Path,
		"state: current",
		"participants: 2",
		"selected by: alice",
		"selected at: 2026-06-01T03:52:00Z",
		"reason: current collaboration focus",
		"mode: metadata-only",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected replay workbench collaboration selection details %q:\n%s", want, view)
		}
	}
}

func TestNewTUIModelForWorkspaceSelectsReplayWorkbenchCollaborationAfterDoubleConfirm(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	run, err := RunWorkspace(context.Background(), root)
	if err != nil {
		t.Fatalf("run workspace: %v", err)
	}
	session, err := CreateReplayWorkbenchSession(context.Background(), root, ReplayWorkbenchSessionOptions{
		PrimaryRunID: run.RunID,
		Reason:       "saved collaboration baseline",
	})
	if err != nil {
		t.Fatalf("create replay workbench session: %v", err)
	}
	collaboration, err := CreateReplayWorkbenchCollaboration(context.Background(), root, ReplayWorkbenchCollaborationOptions{
		SessionPath:  filepath.Base(session.Path),
		Participants: []string{"alice", "bob"},
		Reason:       "candidate pair review",
	})
	if err != nil {
		t.Fatalf("create replay workbench collaboration: %v", err)
	}

	model := NewTUIModelForWorkspace(root).SelectRuntimeTab(tui.RuntimeTabTimelineLanes)
	model = selectRuntimeRowContaining(t, model, "> replay workbench collaboration artifact:")
	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if cmd != nil {
		t.Fatalf("first x should only arm replay workbench collaboration selection")
	}
	next, ok := updated.(tui.Model)
	if !ok {
		t.Fatalf("expected tui.Model, got %T", updated)
	}
	resized, resizeCmd := next.Update(tea.WindowSizeMsg{Width: 860, Height: 48})
	if resizeCmd != nil {
		t.Fatalf("expected confirm resize to be synchronous")
	}
	next, ok = resized.(tui.Model)
	if !ok {
		t.Fatalf("expected tui.Model after confirm resize, got %T", resized)
	}
	confirmView := next.View()
	if !strings.Contains(confirmView, "execution status: confirm-required") || !strings.Contains(confirmView, "action: replay workbench collaboration artifact selected") {
		t.Fatalf("expected confirm-required replay workbench collaboration action:\n%s", confirmView)
	}
	updated, cmd = next.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if cmd == nil {
		t.Fatalf("second x should execute replay workbench collaboration selection")
	}
	msg := cmd()
	updated, cmd = updated.Update(msg)
	if cmd != nil {
		t.Fatalf("expected replay workbench collaboration selection result to be synchronous")
	}
	selection, found, err := ReadReplayWorkbenchCollaborationSelection(root)
	if err != nil {
		t.Fatalf("read replay workbench collaboration selection: %v", err)
	}
	if !found || selection.Status != "current" || selection.CollaborationPath != collaboration.Path || selection.SelectedBy != "tui" || selection.Reason != "selected from TUI" {
		t.Fatalf("expected TUI-selected replay workbench collaboration, found=%v selection=%#v", found, selection)
	}
	if _, ok := updated.(tui.Model); !ok {
		t.Fatalf("expected tui.Model after result, got %T", updated)
	}
	refreshedModel := NewTUIModelForWorkspace(root).SelectRuntimeTab(tui.RuntimeTabTimelineLanes)
	resized, resizeCmd = refreshedModel.Update(tea.WindowSizeMsg{Width: 860, Height: 48})
	if resizeCmd != nil {
		t.Fatalf("expected result resize to be synchronous")
	}
	if _, ok := resized.(tui.Model); !ok {
		t.Fatalf("expected tui.Model after result resize, got %T", resized)
	}
	resultView := resized.View()
	if !strings.Contains(resultView, "replay workbench collaboration selection: status=current") {
		t.Fatalf("expected selected current replay workbench collaboration row:\n%s", resultView)
	}
}

func TestNewTUIModelForWorkspaceClearsReplayWorkbenchCollaborationSelectionAfterDoubleConfirm(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	run, err := RunWorkspace(context.Background(), root)
	if err != nil {
		t.Fatalf("run workspace: %v", err)
	}
	session, err := CreateReplayWorkbenchSession(context.Background(), root, ReplayWorkbenchSessionOptions{
		PrimaryRunID: run.RunID,
		Reason:       "saved collaboration baseline",
	})
	if err != nil {
		t.Fatalf("create replay workbench session: %v", err)
	}
	collaboration, err := CreateReplayWorkbenchCollaboration(context.Background(), root, ReplayWorkbenchCollaborationOptions{
		SessionPath:  filepath.Base(session.Path),
		Participants: []string{"alice", "bob"},
		Reason:       "candidate pair review",
	})
	if err != nil {
		t.Fatalf("create replay workbench collaboration: %v", err)
	}
	if _, err := SelectReplayWorkbenchCollaboration(root, filepath.Base(collaboration.Path), ReplayWorkbenchCollaborationSelectionOptions{
		SelectedBy: "alice",
		Reason:     "current collaboration focus",
	}); err != nil {
		t.Fatalf("select replay workbench collaboration: %v", err)
	}

	model := NewTUIModelForWorkspace(root).SelectRuntimeTab(tui.RuntimeTabTimelineLanes)
	model = selectRuntimeRowContaining(t, model, "> replay workbench collaboration selection:")
	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if cmd != nil {
		t.Fatalf("first x should only arm replay workbench collaboration clear")
	}
	next, ok := updated.(tui.Model)
	if !ok {
		t.Fatalf("expected tui.Model, got %T", updated)
	}
	resized, resizeCmd := next.Update(tea.WindowSizeMsg{Width: 860, Height: 48})
	if resizeCmd != nil {
		t.Fatalf("expected confirm resize to be synchronous")
	}
	next, ok = resized.(tui.Model)
	if !ok {
		t.Fatalf("expected tui.Model after confirm resize, got %T", resized)
	}
	confirmView := next.View()
	if !strings.Contains(confirmView, "execution status: confirm-required") || !strings.Contains(confirmView, "action: replay workbench collaboration selection selected") {
		t.Fatalf("expected confirm-required replay workbench collaboration selection action:\n%s", confirmView)
	}
	updated, cmd = next.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if cmd == nil {
		t.Fatalf("second x should execute replay workbench collaboration selection clear")
	}
	msg := cmd()
	updated, cmd = updated.Update(msg)
	if cmd != nil {
		t.Fatalf("expected replay workbench collaboration clear result to be synchronous")
	}
	selection, found, err := ReadReplayWorkbenchCollaborationSelection(root)
	if err != nil {
		t.Fatalf("read replay workbench collaboration selection: %v", err)
	}
	if !found || selection.Status != "cleared" || selection.CollaborationPath != collaboration.Path || selection.ClearedBy != "tui" || selection.Reason != "cleared from TUI" {
		t.Fatalf("expected TUI-cleared replay workbench collaboration selection, found=%v selection=%#v", found, selection)
	}
	if _, ok := updated.(tui.Model); !ok {
		t.Fatalf("expected tui.Model after clear result, got %T", updated)
	}
	refreshedModel := NewTUIModelForWorkspace(root).SelectRuntimeTab(tui.RuntimeTabTimelineLanes)
	resized, resizeCmd = refreshedModel.Update(tea.WindowSizeMsg{Width: 860, Height: 48})
	if resizeCmd != nil {
		t.Fatalf("expected result resize to be synchronous")
	}
	if _, ok := resized.(tui.Model); !ok {
		t.Fatalf("expected tui.Model after clear result resize, got %T", resized)
	}
	resultView := resized.View()
	if !strings.Contains(resultView, "replay workbench collaboration selection: status=cleared") || strings.Contains(resultView, "replay workbench collaboration current marker: current=true") {
		t.Fatalf("expected cleared replay workbench collaboration selection without stale current row:\n%s", resultView)
	}
}

func TestNewTUIModelForWorkspaceShowsReplayWorkbenchSessionSelection(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	run, err := RunWorkspace(context.Background(), root)
	if err != nil {
		t.Fatalf("run workspace: %v", err)
	}
	if _, err := CreateReplayWorkbenchSession(context.Background(), root, ReplayWorkbenchSessionOptions{
		PrimaryRunID: run.RunID,
		CreatedAt:    time.Date(2026, 6, 1, 2, 40, 0, 0, time.UTC),
		Reason:       "baseline saved view",
	}); err != nil {
		t.Fatalf("create baseline replay workbench session: %v", err)
	}
	candidate, err := CreateReplayWorkbenchSession(context.Background(), root, ReplayWorkbenchSessionOptions{
		PrimaryRunID: run.RunID,
		Filters:      map[string]string{"lane": "toolchain"},
		CreatedAt:    time.Date(2026, 6, 1, 2, 41, 0, 0, time.UTC),
		Reason:       "candidate saved view",
	})
	if err != nil {
		t.Fatalf("create candidate replay workbench session: %v", err)
	}
	if _, err := SelectReplayWorkbenchSession(root, filepath.Base(candidate.Path), ReplayWorkbenchSessionSelectionOptions{
		SelectedBy: "alice",
		SelectedAt: time.Date(2026, 6, 1, 2, 42, 0, 0, time.UTC),
		Reason:     "current investigation focus",
	}); err != nil {
		t.Fatalf("select replay workbench session: %v", err)
	}

	model := NewTUIModelForWorkspace(root).SelectRuntimeTab(tui.RuntimeTabTimelineLanes)
	resized, cmd := model.Update(tea.WindowSizeMsg{Width: 820, Height: 58})
	if cmd != nil {
		t.Fatalf("expected resize update to be synchronous")
	}
	view := resized.View()
	for _, want := range []string{
		"replay workbench session selection: status=current mode=metadata-only",
		"replay workbench session selection actor: selected_by=alice selected_at=2026-06-01T02:42:00Z",
		"replay workbench session selection reason: current investigation focus",
		"replay workbench session superseded:",
		"replay workbench session reason: baseline saved view",
		"replay workbench session reason: candidate saved view",
		"replay workbench session review: decision=unreviewed",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected replay workbench session selection row %q:\n%s", want, view)
		}
	}

	model = selectRuntimeRowContaining(t, model, "> replay workbench session selection:")
	model = model.ShowRuntimeActionDetails()
	resized, cmd = model.Update(tea.WindowSizeMsg{Width: 820, Height: 58})
	if cmd != nil {
		t.Fatalf("expected runtime action details resize to be synchronous")
	}
	view = resized.View()
	for _, want := range []string{
		"[Node Inspector]",
		"runtime action: replay-workbench-session-selection",
		"artifact path: " + candidate.Path,
		"state: current",
		"selected by: alice",
		"selected at: 2026-06-01T02:42:00Z",
		"reason: current investigation focus",
		"mode: metadata-only",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected replay workbench session selection details %q:\n%s", want, view)
		}
	}
}

func TestNewTUIModelForWorkspaceSelectsReplayWorkbenchSessionAfterDoubleConfirm(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	run, err := RunWorkspace(context.Background(), root)
	if err != nil {
		t.Fatalf("run workspace: %v", err)
	}
	session, err := CreateReplayWorkbenchSession(context.Background(), root, ReplayWorkbenchSessionOptions{
		PrimaryRunID: run.RunID,
		Reason:       "candidate saved view",
	})
	if err != nil {
		t.Fatalf("create replay workbench session: %v", err)
	}

	model := NewTUIModelForWorkspace(root).SelectRuntimeTab(tui.RuntimeTabTimelineLanes)
	model = selectRuntimeRowContaining(t, model, "> replay workbench session artifact:")
	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if cmd != nil {
		t.Fatalf("first x should only arm replay workbench session selection")
	}
	next, ok := updated.(tui.Model)
	if !ok {
		t.Fatalf("expected tui.Model, got %T", updated)
	}
	resized, resizeCmd := next.Update(tea.WindowSizeMsg{Width: 820, Height: 46})
	if resizeCmd != nil {
		t.Fatalf("expected confirm resize to be synchronous")
	}
	next, ok = resized.(tui.Model)
	if !ok {
		t.Fatalf("expected tui.Model after confirm resize, got %T", resized)
	}
	confirmView := next.View()
	if !strings.Contains(confirmView, "execution status: confirm-required") || !strings.Contains(confirmView, "action: replay workbench session artifact selected") {
		t.Fatalf("expected confirm-required replay workbench session action:\n%s", confirmView)
	}
	updated, cmd = next.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if cmd == nil {
		t.Fatalf("second x should execute replay workbench session selection")
	}
	msg := cmd()
	updated, cmd = updated.Update(msg)
	if cmd != nil {
		t.Fatalf("expected replay workbench session selection result to be synchronous")
	}
	selection, found, err := ReadReplayWorkbenchSessionSelection(root)
	if err != nil {
		t.Fatalf("read replay workbench session selection: %v", err)
	}
	if !found || selection.Status != "current" || selection.SessionPath != session.Path || selection.SelectedBy != "tui" || selection.Reason != "selected from TUI" {
		t.Fatalf("expected TUI-selected replay workbench session, found=%v selection=%#v", found, selection)
	}
	next, ok = updated.(tui.Model)
	if !ok {
		t.Fatalf("expected tui.Model after result, got %T", updated)
	}
	resized, resizeCmd = next.Update(tea.WindowSizeMsg{Width: 820, Height: 46})
	if resizeCmd != nil {
		t.Fatalf("expected result resize to be synchronous")
	}
	next, ok = resized.(tui.Model)
	if !ok {
		t.Fatalf("expected tui.Model after result resize, got %T", resized)
	}
	resultView := next.View()
	if !strings.Contains(resultView, "> replay workbench session selection: status=current") {
		t.Fatalf("expected selected current replay workbench session row:\n%s", resultView)
	}
}

func TestNewTUIModelForWorkspaceClearsReplayWorkbenchSessionSelectionAfterDoubleConfirm(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	run, err := RunWorkspace(context.Background(), root)
	if err != nil {
		t.Fatalf("run workspace: %v", err)
	}
	session, err := CreateReplayWorkbenchSession(context.Background(), root, ReplayWorkbenchSessionOptions{
		PrimaryRunID: run.RunID,
		Reason:       "candidate saved view",
	})
	if err != nil {
		t.Fatalf("create replay workbench session: %v", err)
	}
	if _, err := SelectReplayWorkbenchSession(root, filepath.Base(session.Path), ReplayWorkbenchSessionSelectionOptions{
		SelectedBy: "alice",
		Reason:     "current investigation focus",
	}); err != nil {
		t.Fatalf("select replay workbench session: %v", err)
	}

	model := NewTUIModelForWorkspace(root).SelectRuntimeTab(tui.RuntimeTabTimelineLanes)
	model = selectRuntimeRowContaining(t, model, "> replay workbench session selection:")
	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if cmd != nil {
		t.Fatalf("first x should only arm replay workbench session clear")
	}
	next, ok := updated.(tui.Model)
	if !ok {
		t.Fatalf("expected tui.Model, got %T", updated)
	}
	resized, resizeCmd := next.Update(tea.WindowSizeMsg{Width: 820, Height: 46})
	if resizeCmd != nil {
		t.Fatalf("expected confirm resize to be synchronous")
	}
	next, ok = resized.(tui.Model)
	if !ok {
		t.Fatalf("expected tui.Model after confirm resize, got %T", resized)
	}
	confirmView := next.View()
	if !strings.Contains(confirmView, "execution status: confirm-required") || !strings.Contains(confirmView, "action: replay workbench session selection selected") {
		t.Fatalf("expected confirm-required replay workbench session selection action:\n%s", confirmView)
	}
	updated, cmd = next.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if cmd == nil {
		t.Fatalf("second x should execute replay workbench session selection clear")
	}
	msg := cmd()
	updated, cmd = updated.Update(msg)
	if cmd != nil {
		t.Fatalf("expected replay workbench session clear result to be synchronous")
	}
	selection, found, err := ReadReplayWorkbenchSessionSelection(root)
	if err != nil {
		t.Fatalf("read replay workbench session selection: %v", err)
	}
	if !found || selection.Status != "cleared" || selection.SessionPath != session.Path || selection.ClearedBy != "tui" || selection.Reason != "cleared from TUI" {
		t.Fatalf("expected TUI-cleared replay workbench session selection, found=%v selection=%#v", found, selection)
	}
	next, ok = updated.(tui.Model)
	if !ok {
		t.Fatalf("expected tui.Model after clear result, got %T", updated)
	}
	resized, resizeCmd = next.Update(tea.WindowSizeMsg{Width: 820, Height: 46})
	if resizeCmd != nil {
		t.Fatalf("expected result resize to be synchronous")
	}
	next, ok = resized.(tui.Model)
	if !ok {
		t.Fatalf("expected tui.Model after result resize, got %T", resized)
	}
	resultView := next.View()
	if !strings.Contains(resultView, "> replay workbench session selection: status=cleared") {
		t.Fatalf("expected selected cleared replay workbench session row:\n%s", resultView)
	}
}

func TestNewTUIModelForWorkspaceShowsCompilePreviewReviewArtifacts(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	marker := filepath.Join(t.TempDir(), "compile-preview-review-ran")
	manifest := `name: web-api-pack
version: "0.1.0"
capabilities:
  - web.discovery
runner: command
run_command:
  - sh
  - -c
  - touch ` + marker + `
parser: web_api_parser
healthcheck: built_in
`
	if err := os.WriteFile(filepath.Join(root, "toolchain", "web-api-pack", "manifest.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	review, err := CreateToolCompilePreviewReview(root, ToolCompilePreviewOptions{
		Budget: compiler.CompileBudget{MaxPlanNodes: 1},
	})
	if err != nil {
		t.Fatalf("create compile preview review: %v", err)
	}
	if review.Path == "" {
		t.Fatalf("expected compile preview review artifact path")
	}
	review, err = AnnotateToolCompilePreviewReview(root, filepath.Base(review.Path), ToolCompilePreviewReviewAnnotationOptions{
		Decision:   "accepted",
		Reason:     "authorized preview",
		Reviewer:   "operator",
		ReviewedAt: time.Date(2026, 5, 31, 15, 4, 5, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("annotate compile preview review: %v", err)
	}

	model := NewTUIModelForWorkspace(root).SelectRuntimeTab(tui.RuntimeTabToolchainHealth)
	resized, cmd := model.Update(tea.WindowSizeMsg{Width: 720, Height: 44})
	if cmd != nil {
		t.Fatalf("expected resize update to be synchronous")
	}
	view := resized.View()
	for _, want := range []string{
		"compile-preview reviews: total=1",
		"compile-preview review artifact: status=dry-run mode=metadata-only action=discover_assets nodes=1 warnings=1 current=false decision=accepted reviewer=operator",
		"reason=authorized preview",
		"compile-preview review node: discover_assets:01",
		"manifest=web-api-pack tool=pack adapter=none operation=none capability=web.discovery",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected TUI compile-preview review row %q:\n%s", want, view)
		}
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("compile-preview review TUI rows must not execute run_command, marker stat err=%v", err)
	}

	model = selectRuntimeRowContaining(t, model, "> compile-preview review artifact:")
	model = model.ShowRuntimeActionDetails()
	resized, cmd = model.Update(tea.WindowSizeMsg{Width: 720, Height: 44})
	if cmd != nil {
		t.Fatalf("expected runtime action details resize to be synchronous")
	}
	view = resized.View()
	for _, want := range []string{
		"[Node Inspector]",
		"runtime action: compile-preview-review-artifact",
		"artifact path: ",
		"compile-preview-reviews/compile-preview-review",
		"state: dry-run",
		"execution mode: metadata-only",
		"action type: discover_assets",
		"nodes: 1",
		"warnings: 1",
		"decision: accepted",
		"reviewer: operator",
		"reviewed at: 2026-05-31T15:04:05Z",
		"reason: authorized preview",
		"current: false",
		"mode: metadata-only",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected compile-preview review details %q:\n%s", want, view)
		}
	}
}

func TestNewTUIModelForWorkspaceShowsCompilePreviewReviewComparison(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	basePath := writeCompilePreviewReviewFixture(t, root, "base.json", ToolCompilePreviewReview{
		Status: "dry-run",
		Mode:   "metadata-only",
		Preview: ToolCompilePreview{
			Workspace: root,
			Action:    compiler.ActionDiscoverAssets,
			Mode:      "metadata-only",
			Nodes: []compiler.PlanNode{
				{ID: "discover_assets:01", ManifestName: "web-api-pack", Capability: "web.discovery", Runner: "command", Parser: "web_api_parser", SelectionScore: 1, SelectionSignals: []string{"history:successes=1 failures=0"}},
			},
			Warnings: []string{"base warning"},
		},
	})
	candidatePath := writeCompilePreviewReviewFixture(t, root, "candidate.json", ToolCompilePreviewReview{
		Status: "dry-run",
		Mode:   "metadata-only",
		Preview: ToolCompilePreview{
			Workspace: root,
			Action:    compiler.ActionDiscoverAssets,
			Mode:      "metadata-only",
			Nodes: []compiler.PlanNode{
				{ID: "discover_assets:01", ManifestName: "web-api-pack", Capability: "web.discovery", Runner: "command", Parser: "web_api_parser", SelectionScore: 4, SelectionSignals: []string{"frontier=3"}},
				{ID: "discover_assets:02", ManifestName: "ctf-pack", Capability: "ctf.challenge_discovery", Runner: "command", Parser: "ctf_parser", SelectionScore: 5, SelectionSignals: []string{"domain:ctf_lab=5"}},
			},
			Warnings: []string{"candidate warning"},
		},
	})

	model := NewTUIModelForWorkspace(root).SelectRuntimeTab(tui.RuntimeTabToolchainHealth)
	resized, cmd := model.Update(tea.WindowSizeMsg{Width: 720, Height: 44})
	if cmd != nil {
		t.Fatalf("expected resize update to be synchronous")
	}
	view := resized.View()
	for _, want := range []string{
		"compile-preview review comparison: mode=metadata-only",
		"node_delta=1 warning_delta=0 added=1 removed=0",
		"score_changed=1 signal_changed=1",
		"compile-preview review comparison node: change=added manifest=ctf-pack tool=pack adapter=none operation=none capability=ctf.challenge_discovery",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected TUI compile-preview review comparison row %q:\n%s", want, view)
		}
	}

	model = selectRuntimeRowContaining(t, model, "> compile-preview review comparison:")
	model = model.ShowRuntimeActionDetails()
	resized, cmd = model.Update(tea.WindowSizeMsg{Width: 720, Height: 44})
	if cmd != nil {
		t.Fatalf("expected comparison details resize to be synchronous")
	}
	view = resized.View()
	for _, want := range []string{
		"[Node Inspector]",
		"runtime action: compile-preview-review-comparison",
		"base path: " + basePath,
		"candidate path: " + candidatePath,
		"node delta: 1",
		"warning delta: 0",
		"added: 1",
		"removed: 0",
		"changed: 1",
		"mode: read-only",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected compile-preview review comparison details %q:\n%s", want, view)
		}
	}
}

func TestNewTUIModelForWorkspaceShowsCompilePreviewReviewSelection(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	basePath := writeCompilePreviewReviewFixture(t, root, "base.json", ToolCompilePreviewReview{
		Status: "dry-run",
		Mode:   "metadata-only",
		Preview: ToolCompilePreview{
			Workspace: root,
			Action:    compiler.ActionDiscoverAssets,
			Mode:      "metadata-only",
			Nodes: []compiler.PlanNode{
				{ID: "discover_assets:01", ManifestName: "web-api-pack", Capability: "web.discovery"},
			},
		},
	})
	candidatePath := writeCompilePreviewReviewFixture(t, root, "candidate.json", ToolCompilePreviewReview{
		Status: "dry-run",
		Mode:   "metadata-only",
		Preview: ToolCompilePreview{
			Workspace: root,
			Action:    compiler.ActionDiscoverAssets,
			Mode:      "metadata-only",
			Nodes: []compiler.PlanNode{
				{ID: "discover_assets:01", ManifestName: "infra-pack", Capability: "infra.host_discovery"},
			},
		},
	})
	if _, err := SelectToolCompilePreviewReview(root, filepath.Base(candidatePath), ToolCompilePreviewReviewSelectionOptions{
		SelectedBy: "alice",
		Reason:     "best reviewed candidate",
		SelectedAt: time.Date(2026, 6, 1, 0, 15, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("select compile preview review: %v", err)
	}

	model := NewTUIModelForWorkspace(root).SelectRuntimeTab(tui.RuntimeTabToolchainHealth)
	resized, cmd := model.Update(tea.WindowSizeMsg{Width: 720, Height: 44})
	if cmd != nil {
		t.Fatalf("expected resize update to be synchronous")
	}
	view := resized.View()
	for _, want := range []string{
		"compile-preview review selection: status=current mode=metadata-only action=discover_assets superseded=1 selected_by=alice",
		"reason=best reviewed candidate",
		"compile-preview review artifact: status=dry-run mode=metadata-only action=discover_assets nodes=1 warnings=0 current=false",
		"compile-preview review artifact: status=dry-run mode=metadata-only action=discover_assets nodes=1 warnings=0 current=true",
		"compile-preview review superseded: " + basePath,
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected TUI compile-preview review selection row %q:\n%s", want, view)
		}
	}

	model = selectRuntimeRowContaining(t, model, "> compile-preview review selection:")
	model = model.ShowRuntimeActionDetails()
	resized, cmd = model.Update(tea.WindowSizeMsg{Width: 720, Height: 44})
	if cmd != nil {
		t.Fatalf("expected selection details resize to be synchronous")
	}
	view = resized.View()
	for _, want := range []string{
		"[Node Inspector]",
		"runtime action: compile-preview-review-selection",
		"review path: " + candidatePath,
		"selected by: alice",
		"selected at: 2026-06-01T00:15:00Z",
		"reason: best reviewed candidate",
		"superseded: 1",
		"mode: metadata-only",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected compile-preview review selection details %q:\n%s", want, view)
		}
	}
}

func TestNewTUIModelForWorkspaceShowsClearedCompilePreviewReviewSelection(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	basePath := writeCompilePreviewReviewFixture(t, root, "base.json", ToolCompilePreviewReview{
		Status: "dry-run",
		Mode:   "metadata-only",
		Preview: ToolCompilePreview{
			Workspace: root,
			Action:    compiler.ActionDiscoverAssets,
			Mode:      "metadata-only",
			Nodes: []compiler.PlanNode{
				{ID: "discover_assets:01", ManifestName: "web-api-pack", Capability: "web.discovery"},
			},
		},
	})
	candidatePath := writeCompilePreviewReviewFixture(t, root, "candidate.json", ToolCompilePreviewReview{
		Status: "dry-run",
		Mode:   "metadata-only",
		Preview: ToolCompilePreview{
			Workspace: root,
			Action:    compiler.ActionDiscoverAssets,
			Mode:      "metadata-only",
			Nodes: []compiler.PlanNode{
				{ID: "discover_assets:01", ManifestName: "infra-pack", Capability: "infra.host_discovery"},
			},
		},
	})
	if _, err := SelectToolCompilePreviewReview(root, filepath.Base(candidatePath), ToolCompilePreviewReviewSelectionOptions{
		SelectedBy: "alice",
		SelectedAt: time.Date(2026, 6, 1, 0, 15, 0, 0, time.UTC),
		Reason:     "best reviewed candidate",
	}); err != nil {
		t.Fatalf("select compile preview review: %v", err)
	}
	if _, err := ClearToolCompilePreviewReviewSelection(root, ToolCompilePreviewReviewSelectionClearOptions{
		ClearedBy: "alice",
		ClearedAt: time.Date(2026, 6, 1, 0, 35, 0, 0, time.UTC),
		Reason:    "authorization changed",
	}); err != nil {
		t.Fatalf("clear compile preview review selection: %v", err)
	}

	model := NewTUIModelForWorkspace(root).SelectRuntimeTab(tui.RuntimeTabToolchainHealth)
	resized, cmd := model.Update(tea.WindowSizeMsg{Width: 720, Height: 44})
	if cmd != nil {
		t.Fatalf("expected resize update to be synchronous")
	}
	view := resized.View()
	for _, want := range []string{
		"compile-preview review selection: status=cleared mode=metadata-only action=discover_assets superseded=1 cleared_by=alice",
		"cleared_at=2026-06-01T00:35:00Z",
		"reason=authorization changed",
		"compile-preview review artifact: status=dry-run mode=metadata-only action=discover_assets nodes=1 warnings=0 current=false",
		"compile-preview review superseded: " + basePath,
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected cleared TUI compile-preview review selection row %q:\n%s", want, view)
		}
	}
	if strings.Contains(view, "current=true") {
		t.Fatalf("cleared selection must not mark any review current:\n%s", view)
	}

	model = selectRuntimeRowContaining(t, model, "> compile-preview review selection:")
	model = model.ShowRuntimeActionDetails()
	resized, cmd = model.Update(tea.WindowSizeMsg{Width: 720, Height: 44})
	if cmd != nil {
		t.Fatalf("expected selection details resize to be synchronous")
	}
	view = resized.View()
	for _, want := range []string{
		"[Node Inspector]",
		"runtime action: compile-preview-review-selection",
		"state: cleared",
		"review path: " + candidatePath,
		"cleared by: alice",
		"cleared at: 2026-06-01T00:35:00Z",
		"reason: authorization changed",
		"superseded: 1",
		"mode: read-only",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected cleared compile-preview review selection details %q:\n%s", want, view)
		}
	}
}

func TestNewTUIModelForWorkspaceClearsCompilePreviewReviewSelectionAfterDoubleConfirm(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	marker := filepath.Join(t.TempDir(), "compile-preview-review-selection-tui-clear-ran")
	manifest := `name: web-api-pack
version: "0.1.0"
capabilities:
  - web.discovery
runner: command
run_command:
  - sh
  - -c
  - touch ` + marker + `
parser: web_api_parser
healthcheck: built_in
`
	if err := os.WriteFile(filepath.Join(root, "toolchain", "web-api-pack", "manifest.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	candidatePath := writeCompilePreviewReviewFixture(t, root, "candidate.json", ToolCompilePreviewReview{
		Status: "dry-run",
		Mode:   "metadata-only",
		Preview: ToolCompilePreview{
			Workspace: root,
			Action:    compiler.ActionDiscoverAssets,
			Mode:      "metadata-only",
			Nodes: []compiler.PlanNode{
				{ID: "discover_assets:01", ManifestName: "web-api-pack", Capability: "web.discovery"},
			},
		},
	})
	if _, err := SelectToolCompilePreviewReview(root, filepath.Base(candidatePath), ToolCompilePreviewReviewSelectionOptions{
		SelectedBy: "alice",
		SelectedAt: time.Date(2026, 6, 1, 0, 15, 0, 0, time.UTC),
		Reason:     "best reviewed candidate",
	}); err != nil {
		t.Fatalf("select compile preview review: %v", err)
	}

	model := NewTUIModelForWorkspace(root).SelectRuntimeTab(tui.RuntimeTabToolchainHealth)
	model = selectRuntimeRowContaining(t, model, "> compile-preview review selection:")
	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if cmd != nil {
		t.Fatalf("first x should only arm selection clear confirmation")
	}
	armed, ok := updated.(tui.Model)
	if !ok {
		t.Fatalf("expected TUI model after first x, got %T", updated)
	}
	selection, found, err := ReadToolCompilePreviewReviewSelection(root)
	if err != nil {
		t.Fatalf("read selection after first x: %v", err)
	}
	if !found || selection.Status != "current" {
		t.Fatalf("first x must not clear selection, found=%v selection=%#v", found, selection)
	}
	resized, resizeCmd := armed.Update(tea.WindowSizeMsg{Width: 720, Height: 44})
	if resizeCmd != nil {
		t.Fatalf("expected resize after first x to be synchronous")
	}
	if !strings.Contains(resized.View(), "execution status: confirm-required") {
		t.Fatalf("expected confirmation status before clearing selection:\n%s", resized.View())
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("first x must not execute run_command, marker stat err=%v", err)
	}

	updated, cmd = armed.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if cmd == nil {
		t.Fatalf("second x should return selection clear command")
	}
	msg := cmd()
	updated, cmd = updated.Update(msg)
	if cmd != nil {
		t.Fatalf("expected selection clear result update to be synchronous")
	}
	completed, ok := updated.(tui.Model)
	if !ok {
		t.Fatalf("expected TUI model after second x, got %T", updated)
	}
	selection, found, err = ReadToolCompilePreviewReviewSelection(root)
	if err != nil {
		t.Fatalf("read selection after second x: %v", err)
	}
	if !found || selection.Status != "cleared" || selection.ReviewPath != candidatePath || selection.ClearedBy != "tui" || selection.Reason != "cleared from TUI" {
		t.Fatalf("expected cleared selection after second x, found=%v selection=%#v", found, selection)
	}
	resized, resizeCmd = completed.Update(tea.WindowSizeMsg{Width: 720, Height: 44})
	if resizeCmd != nil {
		t.Fatalf("expected resize after second x to be synchronous")
	}
	view := resized.View()
	for _, want := range []string{
		"execution status: completed",
		"action output: selection cleared",
		"compile-preview review selection: status=cleared",
		"cleared_by=tui",
		"reason=cleared from TUI",
		"current=false",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected selection clear execution view %q:\n%s", want, view)
		}
	}
	if strings.Contains(view, "current=true") {
		t.Fatalf("cleared TUI selection must not leave current=true visible:\n%s", view)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("clearing selection from TUI must not execute run_command, marker stat err=%v", err)
	}
}

func TestNewTUIModelForWorkspaceSelectsCompilePreviewReviewAfterDoubleConfirm(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	marker := filepath.Join(t.TempDir(), "compile-preview-review-select-tui-ran")
	manifest := `name: web-api-pack
version: "0.1.0"
capabilities:
  - web.discovery
runner: command
run_command:
  - sh
  - -c
  - touch ` + marker + `
parser: web_api_parser
healthcheck: built_in
`
	if err := os.WriteFile(filepath.Join(root, "toolchain", "web-api-pack", "manifest.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	basePath := writeCompilePreviewReviewFixture(t, root, "base.json", ToolCompilePreviewReview{
		Status: "dry-run",
		Mode:   "metadata-only",
		Preview: ToolCompilePreview{
			Workspace: root,
			Action:    compiler.ActionDiscoverAssets,
			Mode:      "metadata-only",
			Nodes: []compiler.PlanNode{
				{ID: "discover_assets:01", ManifestName: "infra-pack", Capability: "infra.host_discovery"},
			},
		},
	})
	candidatePath := writeCompilePreviewReviewFixture(t, root, "candidate.json", ToolCompilePreviewReview{
		Status: "dry-run",
		Mode:   "metadata-only",
		Preview: ToolCompilePreview{
			Workspace: root,
			Action:    compiler.ActionDiscoverAssets,
			Mode:      "metadata-only",
			Nodes: []compiler.PlanNode{
				{ID: "discover_assets:01", ManifestName: "web-api-pack", Capability: "web.discovery"},
			},
		},
	})
	if _, err := SelectToolCompilePreviewReview(root, filepath.Base(basePath), ToolCompilePreviewReviewSelectionOptions{
		SelectedBy: "alice",
		SelectedAt: time.Date(2026, 6, 1, 0, 15, 0, 0, time.UTC),
		Reason:     "previous candidate",
	}); err != nil {
		t.Fatalf("select base compile preview review: %v", err)
	}

	model := NewTUIModelForWorkspace(root).SelectRuntimeTab(tui.RuntimeTabToolchainHealth)
	model = selectRuntimeRowContaining(t, model, "> compile-preview review artifact: status=dry-run mode=metadata-only action=discover_assets nodes=1 warnings=0 current=false")
	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if cmd != nil {
		t.Fatalf("first x should only arm review selection confirmation")
	}
	armed, ok := updated.(tui.Model)
	if !ok {
		t.Fatalf("expected TUI model after first x, got %T", updated)
	}
	selection, found, err := ReadToolCompilePreviewReviewSelection(root)
	if err != nil {
		t.Fatalf("read selection after first x: %v", err)
	}
	if !found || selection.Status != "current" || selection.ReviewPath != basePath {
		t.Fatalf("first x must not change current selection, found=%v selection=%#v", found, selection)
	}
	resized, resizeCmd := armed.Update(tea.WindowSizeMsg{Width: 720, Height: 44})
	if resizeCmd != nil {
		t.Fatalf("expected resize after first x to be synchronous")
	}
	for _, want := range []string{
		"execution status: confirm-required",
		"execution confirm: press x again",
		"action mode: metadata-only",
	} {
		if !strings.Contains(resized.View(), want) {
			t.Fatalf("expected review selection confirmation %q:\n%s", want, resized.View())
		}
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("first x must not execute run_command, marker stat err=%v", err)
	}

	updated, cmd = armed.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if cmd == nil {
		t.Fatalf("second x should return review selection command")
	}
	msg := cmd()
	updated, cmd = updated.Update(msg)
	if cmd != nil {
		t.Fatalf("expected review selection result update to be synchronous")
	}
	completed, ok := updated.(tui.Model)
	if !ok {
		t.Fatalf("expected TUI model after second x, got %T", updated)
	}
	selection, found, err = ReadToolCompilePreviewReviewSelection(root)
	if err != nil {
		t.Fatalf("read selection after second x: %v", err)
	}
	if !found || selection.Status != "current" || selection.ReviewPath != candidatePath || selection.SelectedBy != "tui" || selection.Reason != "selected from TUI" {
		t.Fatalf("expected selected candidate after second x, found=%v selection=%#v", found, selection)
	}
	resized, resizeCmd = completed.Update(tea.WindowSizeMsg{Width: 720, Height: 44})
	if resizeCmd != nil {
		t.Fatalf("expected resize after second x to be synchronous")
	}
	view := resized.View()
	for _, want := range []string{
		"execution status: completed",
		"action output: review selected",
		"compile-preview review selection: status=current",
		"selected_by=tui",
		"reason=selected from TUI",
		"current=true",
		"current=false",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected review selection execution view %q:\n%s", want, view)
		}
	}
	if strings.Contains(view, "current=false") && strings.Contains(view, filepath.Base(candidatePath)+" current=false") {
		t.Fatalf("selected candidate must not remain current=false:\n%s", view)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("selecting review from TUI must not execute run_command, marker stat err=%v", err)
	}
}

func TestNewTUIModelForWorkspacePersistsCompilePreviewReviewAfterDoubleConfirm(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	marker := filepath.Join(t.TempDir(), "compile-preview-review-ran")
	manifest := `name: web-api-pack
version: "0.1.0"
capabilities:
  - web.discovery
runner: command
run_command:
  - sh
  - -c
  - touch ` + marker + `
parser: web_api_parser
healthcheck: built_in
`
	if err := os.WriteFile(filepath.Join(root, "toolchain", "web-api-pack", "manifest.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	model := NewTUIModelForWorkspace(root).SelectRuntimeTab(tui.RuntimeTabToolchainHealth)
	model = selectRuntimeRowContaining(t, model, "> compile preview:")
	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if cmd != nil {
		t.Fatalf("first x should only arm compile-preview review confirmation")
	}
	armed, ok := updated.(tui.Model)
	if !ok {
		t.Fatalf("expected TUI model after first x, got %T", updated)
	}
	resized, resizeCmd := armed.Update(tea.WindowSizeMsg{Width: 720, Height: 44})
	if resizeCmd != nil {
		t.Fatalf("expected resize after first x to be synchronous")
	}
	if !strings.Contains(resized.View(), "execution status: confirm-required") {
		t.Fatalf("expected confirmation status before creating review:\n%s", resized.View())
	}
	if reviews, err := filepath.Glob(filepath.Join(root, "toolchain", "compile-preview-reviews", "*.json")); err != nil || len(reviews) != 0 {
		t.Fatalf("first x must not create review artifacts, reviews=%#v err=%v", reviews, err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("first x must not execute run_command, marker stat err=%v", err)
	}

	updated, cmd = armed.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if cmd == nil {
		t.Fatalf("second x should return compile-preview review command")
	}
	msg := cmd()
	updated, cmd = updated.Update(msg)
	if cmd != nil {
		t.Fatalf("expected compile-preview review result update to be synchronous")
	}
	completed, ok := updated.(tui.Model)
	if !ok {
		t.Fatalf("expected TUI model after second x, got %T", updated)
	}
	reviews, err := filepath.Glob(filepath.Join(root, "toolchain", "compile-preview-reviews", "*.json"))
	if err != nil {
		t.Fatalf("glob compile preview review artifacts: %v", err)
	}
	if len(reviews) != 1 {
		t.Fatalf("expected one compile preview review artifact after second x, got %#v", reviews)
	}
	resized, resizeCmd = completed.Update(tea.WindowSizeMsg{Width: 720, Height: 44})
	if resizeCmd != nil {
		t.Fatalf("expected resize after second x to be synchronous")
	}
	view := resized.View()
	for _, want := range []string{
		"execution status: completed",
		"action output: review created",
		"compile-preview reviews: total=1",
		"compile-preview review artifact: status=dry-run mode=metadata-only action=discover_assets",
		"compile-preview review node: discover_assets:01",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected compile-preview review execution view %q:\n%s", want, view)
		}
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("creating compile-preview review from TUI must not execute run_command, marker stat err=%v", err)
	}
}

func TestNewTUIModelForWorkspaceShowsWorkerLeaseArtifacts(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	if _, err := RunWorkspaceWithOptions(context.Background(), root, RunOptions{ToolRunner: &recordingToolExecutionRunner{}}); err != nil {
		t.Fatalf("run workspace: %v", err)
	}

	model := NewTUIModelForWorkspace(root).SelectRuntimeTab(tui.RuntimeTabAgentRuntime)
	resized, cmd := model.Update(tea.WindowSizeMsg{Width: 260, Height: 90})
	if cmd != nil {
		t.Fatalf("expected resize update to be synchronous")
	}
	model = resized.(tui.Model)
	view := model.View()
	for _, want := range []string{
		"worker leases: total=8 active=0 stale=0 completed=8 failed=0",
		"worker lease web-api-pack web.discovery completed worker=local",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected worker lease row %q in TUI view:\n%s", want, view)
		}
	}
}

func TestNewTUIModelForWorkspaceShowsWorkerLeaseHeartbeatMetadata(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	heartbeat := time.Now().UTC().Format(time.RFC3339Nano)
	if err := writeWorkerLeaseArtifact(root, WorkerLeaseArtifact{
		Version:         1,
		RunID:           "run-heartbeat-tui",
		ExecutionID:     "tool-heartbeat-tui",
		Tool:            "web-api-pack",
		Capability:      "web.discovery",
		WorkerID:        "remote-web-1",
		Status:          "active",
		AcquiredAt:      heartbeat,
		HeartbeatAt:     heartbeat,
		HeartbeatBy:     "remote-web-1",
		HeartbeatReason: "still-running",
	}); err != nil {
		t.Fatalf("write heartbeat worker lease fixture: %v", err)
	}

	model := NewTUIModelForWorkspace(root).SelectRuntimeTab(tui.RuntimeTabAgentRuntime)
	resized, cmd := model.Update(tea.WindowSizeMsg{Width: 260, Height: 42})
	if cmd != nil {
		t.Fatalf("expected resize update to be synchronous")
	}
	model = resized.(tui.Model)
	view := model.View()
	for _, want := range []string{
		"worker leases: total=1 active=1 stale=0 completed=0 failed=0",
		"worker lease web-api-pack web.discovery active worker=remote-web-1",
		"worker lease heartbeat: run=run-heartbeat-tui execution=tool-heartbeat-tui",
		"by=remote-web-1",
		"reason=still-running",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected worker lease heartbeat row %q in TUI view:\n%s", want, view)
		}
	}
}

func TestNewTUIModelForWorkspaceShowsWorkerLeaseReleaseMetadata(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	releasedAt := time.Now().UTC().Format(time.RFC3339Nano)
	if err := writeWorkerLeaseArtifact(root, WorkerLeaseArtifact{
		Version:           1,
		RunID:             "run-release-tui",
		ExecutionID:       "tool-release-tui",
		ResultExecutionID: "result-release-tui",
		Tool:              "web-api-pack",
		Capability:        "web.discovery",
		WorkerID:          "remote-web-1",
		Status:            "completed",
		AcquiredAt:        releasedAt,
		HeartbeatAt:       releasedAt,
		ReleasedAt:        releasedAt,
		ReleasedBy:        "remote-web-1",
		ReleaseReason:     "finished",
	}); err != nil {
		t.Fatalf("write released worker lease fixture: %v", err)
	}

	model := NewTUIModelForWorkspace(root).SelectRuntimeTab(tui.RuntimeTabAgentRuntime)
	resized, cmd := model.Update(tea.WindowSizeMsg{Width: 260, Height: 42})
	if cmd != nil {
		t.Fatalf("expected resize update to be synchronous")
	}
	model = resized.(tui.Model)
	view := model.View()
	for _, want := range []string{
		"worker leases: total=1 active=0 stale=0 completed=1 failed=0",
		"worker lease web-api-pack web.discovery completed worker=remote-web-1",
		"worker lease release: run=run-release-tui execution=tool-release-tui",
		"worker lease release by=remote-web-1",
		"worker lease release reason=finished",
		"worker lease result=result-release-tui",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected worker lease release row %q in TUI view:\n%s", want, view)
		}
	}
}

func TestNewTUIModelForWorkspaceShowsWorkerLeaseResultReconciliation(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Date(2026, 5, 30, 19, 20, 0, 0, time.UTC)
	dispatch := createDispatchedRuntimeWorkerQueueClaim(t, root, now)
	if _, err := SubmitRuntimeWorkerQueueDispatchResult(root, filepath.Base(dispatch.Path), RuntimeWorkerQueueDispatchResult{
		Status: "completed",
		Output: "remote result complete",
	}, RuntimeWorkerQueueDispatchResultOptions{Now: now.Add(2 * time.Minute)}); err != nil {
		t.Fatalf("submit worker queue dispatch result: %v", err)
	}
	if err := writeWorkerLeaseArtifact(root, WorkerLeaseArtifact{
		Version:     1,
		RunID:       dispatch.RunID,
		ExecutionID: dispatch.ExecutionID,
		Tool:        dispatch.Tool,
		Capability:  dispatch.Capability,
		WorkerID:    dispatch.WorkerID,
		Status:      "active",
		AcquiredAt:  now.Format(time.RFC3339Nano),
		HeartbeatAt: now.Format(time.RFC3339Nano),
	}); err != nil {
		t.Fatalf("write active worker lease fixture: %v", err)
	}

	model := NewTUIModelForWorkspace(root).SelectRuntimeTab(tui.RuntimeTabAgentRuntime)
	resized, cmd := model.Update(tea.WindowSizeMsg{Width: 260, Height: 46})
	if cmd != nil {
		t.Fatalf("expected resize update to be synchronous")
	}
	model = resized.(tui.Model)
	view := model.View()
	for _, want := range []string{
		"worker lease result reconciliation: ready=1 skipped=0",
		"worker lease result reconcile run=" + dispatch.RunID,
		"execution=" + dispatch.ExecutionID,
		"result=completed",
		"lease=active",
		"recommended=completed",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected worker lease result reconciliation row %q in TUI view:\n%s", want, view)
		}
	}
}

func TestNewTUIModelForWorkspaceShowsWorkerLeaseResultReconciliationArtifacts(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Date(2026, 5, 30, 19, 25, 0, 0, time.UTC)
	dispatch := createDispatchedRuntimeWorkerQueueClaim(t, root, now)
	if _, err := SubmitRuntimeWorkerQueueDispatchResult(root, filepath.Base(dispatch.Path), RuntimeWorkerQueueDispatchResult{
		Status: "failed",
		Error:  "remote worker failed",
	}, RuntimeWorkerQueueDispatchResultOptions{Now: now.Add(2 * time.Minute)}); err != nil {
		t.Fatalf("submit worker queue dispatch result: %v", err)
	}
	if err := writeWorkerLeaseArtifact(root, WorkerLeaseArtifact{
		Version:     1,
		RunID:       dispatch.RunID,
		ExecutionID: dispatch.ExecutionID,
		Tool:        dispatch.Tool,
		Capability:  dispatch.Capability,
		WorkerID:    dispatch.WorkerID,
		Status:      "active",
		AcquiredAt:  now.Format(time.RFC3339Nano),
		HeartbeatAt: now.Format(time.RFC3339Nano),
	}); err != nil {
		t.Fatalf("write active worker lease fixture: %v", err)
	}
	review, err := CreateWorkerLeaseResultReconciliationDispatch(root, now.Add(3*time.Minute))
	if err != nil {
		t.Fatalf("create worker lease result reconciliation review: %v", err)
	}
	if review.Path == "" {
		t.Fatalf("expected reconciliation review path")
	}

	model := NewTUIModelForWorkspace(root).SelectRuntimeTab(tui.RuntimeTabAgentRuntime)
	resized, cmd := model.Update(tea.WindowSizeMsg{Width: 300, Height: 56})
	if cmd != nil {
		t.Fatalf("expected resize update to be synchronous")
	}
	model = resized.(tui.Model)
	view := model.View()
	for _, want := range []string{
		"worker lease result reconciliation artifacts: total=1",
		"worker lease result reconciliation artifact:",
		"status=dry-run",
		"mode=metadata-only",
		"ready=1",
		"skipped=0",
		"worker lease result reconciliation artifact path:",
		"lease result reconciliation artifact state: dry-run mode=metadata-only ready=1 skipped=0",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected worker lease result reconciliation artifact row %q in TUI view:\n%s", want, view)
		}
	}
}

func TestNewTUIModelForWorkspaceShowsWorkerLifecycleReviewRows(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Now().UTC()
	oldHeartbeat := now.Add(-defaultWorkerLeaseStaleAfter - time.Minute).Format(time.RFC3339Nano)
	if err := writeWorkerLeaseArtifact(root, WorkerLeaseArtifact{
		Version:     1,
		RunID:       "run-lifecycle-tui",
		ExecutionID: "tool-lifecycle-tui",
		Tool:        "web-api-pack",
		Capability:  "web.discovery",
		WorkerID:    "local",
		Status:      "active",
		AcquiredAt:  oldHeartbeat,
		HeartbeatAt: oldHeartbeat,
	}); err != nil {
		t.Fatalf("write worker lease fixture: %v", err)
	}
	createFileOutboxRuntimeWorkerQueueDispatch(t, root, now)
	review, err := CreateWorkerLifecycleReview(root, now, defaultWorkerLeaseStaleAfter)
	if err != nil {
		t.Fatalf("create worker lifecycle review: %v", err)
	}

	model := NewTUIModelForWorkspace(root).SelectRuntimeTab(tui.RuntimeTabAgentRuntime)
	resized, cmd := model.Update(tea.WindowSizeMsg{Width: 300, Height: 56})
	if cmd != nil {
		t.Fatalf("expected resize update to be synchronous")
	}
	model = resized.(tui.Model)
	view := model.View()
	for _, want := range []string{
		"worker lifecycle review: leases=1 active=1 stale=1 completed=0 failed=0 reclaimed=0",
		"reclaim_ready=1",
		"reconciliation_ready=0",
		"worker lifecycle queue: claims=1 dispatched=1 completed=0",
		"worker lifecycle queue closed: failed=0 canceled=0 expired=0",
		"worker lifecycle transport: requests=1 pending=1 ack=0 completed=0 failed=0",
		"worker lifecycle results: total=0 completed=0 failed=0",
		"mode=metadata-only",
		"worker lifecycle review artifacts: total=1",
		"worker lifecycle review artifact:",
		"status=dry-run",
		"worker lifecycle review artifact state: dry-run mode=metadata-only leases=1 stale=1",
		"worker lifecycle review artifact readiness: reclaim_ready=1 reconciliation_ready=0",
		"worker lifecycle artifact queue: claims=1 dispatched=1 completed=0",
		"worker lifecycle artifact transport: requests=1 pending=1 ack=0 results=0",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected worker lifecycle review row %q in TUI view:\n%s", want, view)
		}
	}
	if review.Path == "" {
		t.Fatalf("expected lifecycle review path")
	}
}

func TestNewTUIModelForWorkspaceShowsWorkerLifecycleActionPlanRows(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Now().UTC()
	oldHeartbeat := now.Add(-defaultWorkerLeaseStaleAfter - time.Minute).Format(time.RFC3339Nano)
	if err := writeWorkerLeaseArtifact(root, WorkerLeaseArtifact{
		Version:     1,
		RunID:       "run-lifecycle-action-tui",
		ExecutionID: "tool-lifecycle-action-tui",
		Tool:        "web-api-pack",
		Capability:  "web.discovery",
		WorkerID:    "local",
		Status:      "active",
		AcquiredAt:  oldHeartbeat,
		HeartbeatAt: oldHeartbeat,
	}); err != nil {
		t.Fatalf("write worker lease fixture: %v", err)
	}
	createFileOutboxRuntimeWorkerQueueDispatch(t, root, now)
	review, err := CreateWorkerLifecycleReview(root, now, defaultWorkerLeaseStaleAfter)
	if err != nil {
		t.Fatalf("create worker lifecycle review: %v", err)
	}
	actionPlan, err := CreateWorkerLifecycleActionPlan(root, filepath.Base(review.Path), now.Add(time.Minute), defaultWorkerLeaseStaleAfter)
	if err != nil {
		t.Fatalf("create worker lifecycle action plan: %v", err)
	}

	model := NewTUIModelForWorkspace(root).SelectRuntimeTab(tui.RuntimeTabAgentRuntime)
	resized, cmd := model.Update(tea.WindowSizeMsg{Width: 300, Height: 58})
	if cmd != nil {
		t.Fatalf("expected resize update to be synchronous")
	}
	model = resized.(tui.Model)
	view := model.View()
	for _, want := range []string{
		"worker lifecycle action plan artifacts: total=1",
		"worker lifecycle action plan artifact status=dry-run mode=metadata-only",
		"worker lifecycle action plan artifact:",
		"worker lifecycle action plan artifact state: dry-run mode=metadata-only actions=2",
		"worker lifecycle action plan artifact counts: ack=1 run=0 reclaim=1 reconcile=0",
		"worker lifecycle action plan artifact path:",
		"worker lifecycle action: type=ack-transport-request gate=worker-transport-ack",
		"worker lifecycle action: type=reclaim-stale-lease gate=worker-lease-reclaim",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected worker lifecycle action plan row %q in TUI view:\n%s", want, view)
		}
	}
	if actionPlan.Path == "" {
		t.Fatalf("expected action plan path")
	}
}

func TestNewTUIModelForWorkspaceShowsWorkerLifecycleActionPlanDaemonControlRows(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Now().UTC()
	control, err := CreateRuntimeWorkerTransportDaemonControlRequest(root, "remote-web-1", RuntimeWorkerTransportDaemonControlRequest{
		Action:      "restart",
		Reason:      "reload worker config",
		RequestedBy: "operator",
	}, RuntimeWorkerTransportDaemonControlRequestOptions{Now: now})
	if err != nil {
		t.Fatalf("create daemon control fixture: %v", err)
	}
	actionPlan, err := CreateWorkerLifecycleActionPlan(root, "", now.Add(time.Minute), defaultWorkerLeaseStaleAfter)
	if err != nil {
		t.Fatalf("create worker lifecycle action plan: %v", err)
	}

	model := NewTUIModelForWorkspace(root).SelectRuntimeTab(tui.RuntimeTabAgentRuntime)
	resized, cmd := model.Update(tea.WindowSizeMsg{Width: 300, Height: 58})
	if cmd != nil {
		t.Fatalf("expected resize update to be synchronous")
	}
	model = resized.(tui.Model)
	view := model.View()
	for _, want := range []string{
		"worker lifecycle action plan artifacts: total=1",
		"worker lifecycle action plan artifact state: dry-run mode=metadata-only actions=1",
		"worker lifecycle action plan daemon controls: ready=1",
		"worker lifecycle action: type=apply-daemon-control",
		"daemon control action: remote-web-1 restart ready enable-control",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected worker lifecycle daemon control action row %q in TUI view:\n%s", want, view)
		}
	}
	if actionPlan.Path == "" || control.Path == "" {
		t.Fatalf("expected action plan and control paths")
	}
}

func TestNewTUIModelForWorkspaceShowsWorkerLifecycleActionPlanRetryDispatchRows(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Now().UTC()
	_, retryDispatch, _ := createRuntimeWorkerDispatchRetryDispatchFixture(t, root, now)
	actionPlan, err := CreateWorkerLifecycleActionPlan(root, "", now.Add(4*time.Minute), defaultWorkerLeaseStaleAfter)
	if err != nil {
		t.Fatalf("create worker lifecycle action plan: %v", err)
	}

	model := NewTUIModelForWorkspace(root).SelectRuntimeTab(tui.RuntimeTabAgentRuntime)
	resized, cmd := model.Update(tea.WindowSizeMsg{Width: 300, Height: 58})
	if cmd != nil {
		t.Fatalf("expected resize update to be synchronous")
	}
	model = resized.(tui.Model)
	view := model.View()
	for _, want := range []string{
		"worker lifecycle action plan artifacts: total=1",
		"worker lifecycle action plan artifact state: dry-run mode=metadata-only actions=1",
		"worker lifecycle action plan retry dispatch executions: ready=1",
		"worker lifecycle action: type=execute-dispatch-retry-dispatch",
		"retry dispatch action: execute ready gate=worker-dispatch-retry-dispatch-execute",
		filepath.Base(retryDispatch.Path),
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected worker lifecycle retry-dispatch action row %q in TUI view:\n%s", want, view)
		}
	}
	if actionPlan.Path == "" {
		t.Fatalf("expected action plan path")
	}
}

func TestNewTUIModelForWorkspaceShowsWorkerLifecycleActionPlanReviewSelectionRows(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Now().UTC()
	createFileOutboxRuntimeWorkerQueueDispatch(t, root, now)
	actionPlan, err := CreateWorkerLifecycleActionPlan(root, "", now.Add(time.Minute), defaultWorkerLeaseStaleAfter)
	if err != nil {
		t.Fatalf("create worker lifecycle action plan: %v", err)
	}
	if _, err := AnnotateWorkerLifecycleActionPlan(root, filepath.Base(actionPlan.Path), WorkerLifecycleActionPlanAnnotationOptions{
		Decision:   "accepted",
		Reviewer:   "alice",
		ReviewedAt: time.Date(2026, 6, 2, 9, 12, 0, 0, time.UTC),
		Reason:     "reviewed authorized action plan",
	}); err != nil {
		t.Fatalf("annotate action plan: %v", err)
	}
	if _, err := SelectWorkerLifecycleActionPlan(root, filepath.Base(actionPlan.Path), WorkerLifecycleActionPlanSelectionOptions{
		SelectedBy: "bob",
		SelectedAt: time.Date(2026, 6, 2, 9, 13, 0, 0, time.UTC),
		Reason:     "current reviewed action plan",
	}); err != nil {
		t.Fatalf("select action plan: %v", err)
	}

	model := NewTUIModelForWorkspace(root).SelectRuntimeTab(tui.RuntimeTabAgentRuntime)
	resized, cmd := model.Update(tea.WindowSizeMsg{Width: 900, Height: 58})
	if cmd != nil {
		t.Fatalf("expected resize update to be synchronous")
	}
	model = resized.(tui.Model)
	view := model.View()
	for _, want := range []string{
		"worker lifecycle action plan artifacts: total=1 annotated=1 current=1",
		"worker lifecycle action plan selection: status=current mode=metadata-only current=true selected_by=bob selected_at=2026-06-02T09:13:00Z reason=current_reviewed_action_plan",
		"worker lifecycle action plan artifact:",
		"worker lifecycle action plan artifact review: current=true decision=accepted reviewer=alice reviewed_at=2026-06-02T09:12:00Z review_reason=reviewed_authorized_action_plan",
		"worker lifecycle action plan review: mode=metadata-only decision=accepted reviewer=alice reviewed_at=2026-06-02T09:12:00Z review_reason=reviewed_authorized_action_plan",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected worker lifecycle action-plan review/selection row %q in TUI view:\n%s", want, view)
		}
	}
	model = selectRuntimeRowContaining(t, model, "> worker lifecycle action plan selection:")
	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	if cmd != nil {
		t.Fatalf("expected action-plan selection detail update to be synchronous")
	}
	detailView := updated.View()
	for _, want := range []string{
		"runtime action: worker-lifecycle-action-plan-selection",
		"selection status: current",
		"state: current",
		"plan path: " + actionPlan.Path,
		"selection path: " + filepath.Join(root, "runtime", "worker-lifecycle-action-plan-selection.json"),
		"current: true",
		"selected by: bob",
		"reason: current_reviewed_action_plan",
	} {
		if !strings.Contains(detailView, want) {
			t.Fatalf("expected action-plan selection detail %q:\n%s", want, detailView)
		}
	}
}

func TestNewTUIModelForWorkspaceSelectsWorkerLifecycleActionPlanAfterDoubleConfirm(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Now().UTC()
	dispatch := createFileOutboxRuntimeWorkerQueueDispatch(t, root, now)
	actionPlan, err := CreateWorkerLifecycleActionPlan(root, "", now.Add(time.Minute), defaultWorkerLeaseStaleAfter)
	if err != nil {
		t.Fatalf("create worker lifecycle action plan: %v", err)
	}
	if _, err := AnnotateWorkerLifecycleActionPlan(root, filepath.Base(actionPlan.Path), WorkerLifecycleActionPlanAnnotationOptions{Decision: "accepted"}); err != nil {
		t.Fatalf("accept action plan: %v", err)
	}
	plansBefore := countJSON(t, filepath.Join(root, "runtime", "worker-lifecycle-action-plans"))
	maintenanceBefore := countJSON(t, filepath.Join(root, "runtime", "worker-lifecycle-maintenance"))
	sideEffectsBefore := snapshotWorkerLifecyclePolicySideEffects(t, root)

	model := NewTUIModelForWorkspace(root).SelectRuntimeTab(tui.RuntimeTabAgentRuntime)
	model = selectRuntimeRowContaining(t, model, "> worker lifecycle action plan artifact:")
	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if cmd != nil {
		t.Fatalf("first x should only arm action-plan selection")
	}
	armed, ok := updated.(tui.Model)
	if !ok {
		t.Fatalf("expected TUI model after first x, got %T", updated)
	}
	if _, found, err := ReadWorkerLifecycleActionPlanSelection(root); err != nil || found {
		t.Fatalf("first x must not write action-plan selection marker, found=%v err=%v", found, err)
	}

	updated, cmd = armed.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if cmd == nil {
		t.Fatalf("second x should select action plan")
	}
	msg := cmd()
	updated, cmd = updated.Update(msg)
	if cmd != nil {
		t.Fatalf("expected action-plan selection result to be synchronous")
	}
	selection, found, err := ReadWorkerLifecycleActionPlanSelection(root)
	if err != nil {
		t.Fatalf("read action-plan selection: %v", err)
	}
	if !found || selection.Status != "current" || selection.PlanPath != actionPlan.Path || selection.SelectedBy != "tui" || selection.Reason != "selected from TUI" {
		t.Fatalf("expected TUI-selected action plan, found=%v selection=%#v", found, selection)
	}
	if got := countJSON(t, filepath.Join(root, "runtime", "worker-lifecycle-action-plans")); got != plansBefore {
		t.Fatalf("TUI selection must not create action-plan artifacts, before=%d after=%d", plansBefore, got)
	}
	if got := countJSON(t, filepath.Join(root, "runtime", "worker-lifecycle-maintenance")); got != maintenanceBefore {
		t.Fatalf("TUI selection must not create lifecycle maintenance artifacts, before=%d after=%d", maintenanceBefore, got)
	}
	requests, err := ListRuntimeWorkerTransportDispatchRequests(root, dispatch.WorkerID)
	if err != nil {
		t.Fatalf("list transport requests after TUI select: %v", err)
	}
	if len(requests) != 1 || requests[0].Status != "pending" {
		t.Fatalf("TUI selection must not acknowledge or run transport requests: %#v", requests)
	}
	assertWorkerLifecyclePolicySideEffectsUnchanged(t, root, sideEffectsBefore)
	completed, ok := updated.(tui.Model)
	if !ok {
		t.Fatalf("expected TUI model after result, got %T", updated)
	}
	resized, resizeCmd := completed.Update(tea.WindowSizeMsg{Width: 900, Height: 58})
	if resizeCmd != nil {
		t.Fatalf("expected resize after result to be synchronous")
	}
	for _, want := range []string{
		"execution status: completed",
		"action output: worker lifecycle action plan selected",
		"worker lifecycle action plan selection: status=current",
		"selected_by=tui",
		"current=true",
	} {
		if !strings.Contains(resized.View(), want) {
			t.Fatalf("expected action-plan selection result view %q:\n%s", want, resized.View())
		}
	}
}

func TestNewTUIModelForWorkspaceRejectsUnselectableWorkerLifecycleActionPlanAfterDoubleConfirm(t *testing.T) {
	for _, tc := range []struct {
		name     string
		prepare  func(t *testing.T, plan WorkerLifecycleActionPlan)
		wantErr  string
		wantMode string
	}{
		{
			name:    "unreviewed",
			prepare: func(t *testing.T, plan WorkerLifecycleActionPlan) {},
			wantErr: "worker lifecycle action plan selection requires accepted annotation",
		},
		{
			name: "deferred",
			prepare: func(t *testing.T, plan WorkerLifecycleActionPlan) {
				t.Helper()
				if _, err := AnnotateWorkerLifecycleActionPlan(filepath.Dir(filepath.Dir(filepath.Dir(plan.Path))), filepath.Base(plan.Path), WorkerLifecycleActionPlanAnnotationOptions{Decision: "deferred"}); err != nil {
					t.Fatalf("defer action plan: %v", err)
				}
			},
			wantErr: "worker lifecycle action plan selection requires accepted annotation",
		},
		{
			name: "execute mode",
			prepare: func(t *testing.T, plan WorkerLifecycleActionPlan) {
				t.Helper()
				root := filepath.Dir(filepath.Dir(filepath.Dir(plan.Path)))
				accepted, err := AnnotateWorkerLifecycleActionPlan(root, filepath.Base(plan.Path), WorkerLifecycleActionPlanAnnotationOptions{Decision: "accepted"})
				if err != nil {
					t.Fatalf("accept action plan: %v", err)
				}
				accepted.Mode = "execute"
				data, err := json.MarshalIndent(accepted, "", "  ")
				if err != nil {
					t.Fatalf("encode tampered action plan: %v", err)
				}
				data = append(data, '\n')
				if err := os.WriteFile(accepted.Path, data, 0o644); err != nil {
					t.Fatalf("write tampered action plan: %v", err)
				}
			},
			wantErr:  "worker lifecycle action plan must be dry-run metadata-only before selection",
			wantMode: "mode=execute",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "workspace")
			now := time.Now().UTC()
			createFileOutboxRuntimeWorkerQueueDispatch(t, root, now)
			actionPlan, err := CreateWorkerLifecycleActionPlan(root, "", now.Add(time.Minute), defaultWorkerLeaseStaleAfter)
			if err != nil {
				t.Fatalf("create worker lifecycle action plan: %v", err)
			}
			tc.prepare(t, actionPlan)

			model := NewTUIModelForWorkspace(root).SelectRuntimeTab(tui.RuntimeTabAgentRuntime)
			resized, resizeCmd := model.Update(tea.WindowSizeMsg{Width: 900, Height: 58})
			if resizeCmd != nil {
				t.Fatalf("expected resize before failed action-plan selection to be synchronous")
			}
			model, ok := resized.(tui.Model)
			if !ok {
				t.Fatalf("expected resized TUI model, got %T", resized)
			}
			model = selectRuntimeRowContaining(t, model, "> worker lifecycle action plan artifact:")
			if tc.wantMode != "" && !strings.Contains(model.View(), tc.wantMode) {
				t.Fatalf("expected tampered action-plan row to expose %q:\n%s", tc.wantMode, model.View())
			}
			updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
			if cmd != nil {
				t.Fatalf("first x should only arm action-plan selection")
			}
			armed, ok := updated.(tui.Model)
			if !ok {
				t.Fatalf("expected TUI model after first x, got %T", updated)
			}
			updated, cmd = armed.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
			if cmd == nil {
				t.Fatalf("second x should call the action-plan selection gate")
			}
			msg := cmd()
			updated, cmd = updated.Update(msg)
			if cmd != nil {
				t.Fatalf("expected failed action-plan selection result to be synchronous")
			}
			if _, found, err := ReadWorkerLifecycleActionPlanSelection(root); err != nil || found {
				t.Fatalf("unselectable action plan must not write selection marker, found=%v err=%v", found, err)
			}
			failed, ok := updated.(tui.Model)
			if !ok {
				t.Fatalf("expected TUI model after failed result, got %T", updated)
			}
			resizedFailed, resizeCmd := failed.Update(tea.WindowSizeMsg{Width: 900, Height: 58})
			if resizeCmd != nil {
				t.Fatalf("expected resize after failed action-plan selection to be synchronous")
			}
			failed, ok = resizedFailed.(tui.Model)
			if !ok {
				t.Fatalf("expected resized failed TUI model, got %T", resizedFailed)
			}
			view := failed.View()
			if !strings.Contains(view, "execution status: failed") || !strings.Contains(view, tc.wantErr) {
				t.Fatalf("expected failed selection error %q:\n%s", tc.wantErr, view)
			}
		})
	}
}

func TestNewTUIModelForWorkspaceClearsWorkerLifecycleActionPlanSelectionAfterDoubleConfirm(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Now().UTC()
	createFileOutboxRuntimeWorkerQueueDispatch(t, root, now)
	actionPlan, err := CreateWorkerLifecycleActionPlan(root, "", now.Add(time.Minute), defaultWorkerLeaseStaleAfter)
	if err != nil {
		t.Fatalf("create worker lifecycle action plan: %v", err)
	}
	if _, err := AnnotateWorkerLifecycleActionPlan(root, filepath.Base(actionPlan.Path), WorkerLifecycleActionPlanAnnotationOptions{Decision: "accepted"}); err != nil {
		t.Fatalf("accept action plan: %v", err)
	}
	if _, err := SelectWorkerLifecycleActionPlan(root, filepath.Base(actionPlan.Path), WorkerLifecycleActionPlanSelectionOptions{
		SelectedBy: "alice",
		Reason:     "current action plan",
	}); err != nil {
		t.Fatalf("select action plan: %v", err)
	}
	plansBefore := countJSON(t, filepath.Join(root, "runtime", "worker-lifecycle-action-plans"))
	sideEffectsBefore := snapshotWorkerLifecyclePolicySideEffects(t, root)

	model := NewTUIModelForWorkspace(root).SelectRuntimeTab(tui.RuntimeTabAgentRuntime)
	model = selectRuntimeRowContaining(t, model, "> worker lifecycle action plan selection:")
	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if cmd != nil {
		t.Fatalf("first x should only arm action-plan selection clear")
	}
	armed, ok := updated.(tui.Model)
	if !ok {
		t.Fatalf("expected TUI model after first x, got %T", updated)
	}
	selection, found, err := ReadWorkerLifecycleActionPlanSelection(root)
	if err != nil {
		t.Fatalf("read selection after first x: %v", err)
	}
	if !found || selection.Status != "current" {
		t.Fatalf("first x must not clear action-plan selection, found=%v selection=%#v", found, selection)
	}

	updated, cmd = armed.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if cmd == nil {
		t.Fatalf("second x should clear action-plan selection")
	}
	msg := cmd()
	updated, cmd = updated.Update(msg)
	if cmd != nil {
		t.Fatalf("expected action-plan selection clear result to be synchronous")
	}
	selection, found, err = ReadWorkerLifecycleActionPlanSelection(root)
	if err != nil {
		t.Fatalf("read cleared action-plan selection: %v", err)
	}
	if !found || selection.Status != "cleared" || selection.PlanPath != actionPlan.Path || selection.ClearedBy != "tui" || selection.Reason != "cleared from TUI" {
		t.Fatalf("expected TUI-cleared action-plan selection, found=%v selection=%#v", found, selection)
	}
	if got := countJSON(t, filepath.Join(root, "runtime", "worker-lifecycle-action-plans")); got != plansBefore {
		t.Fatalf("TUI clear must not create action-plan artifacts, before=%d after=%d", plansBefore, got)
	}
	assertWorkerLifecyclePolicySideEffectsUnchanged(t, root, sideEffectsBefore)
	completed, ok := updated.(tui.Model)
	if !ok {
		t.Fatalf("expected TUI model after result, got %T", updated)
	}
	resized, resizeCmd := completed.Update(tea.WindowSizeMsg{Width: 900, Height: 58})
	if resizeCmd != nil {
		t.Fatalf("expected resize after clear result to be synchronous")
	}
	for _, want := range []string{
		"execution status: completed",
		"action output: worker lifecycle action plan selection cleared",
		"worker lifecycle action plan selection: status=cleared",
		"cleared_by=tui",
		"current=false",
	} {
		if !strings.Contains(resized.View(), want) {
			t.Fatalf("expected action-plan selection clear result view %q:\n%s", want, resized.View())
		}
	}
}

func TestNewTUIModelForWorkspaceShowsWorkerLifecyclePolicyRows(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Date(2026, 6, 1, 16, 55, 0, 0, time.UTC)
	createWorkerLifecycleMaintenanceFixture(t, root, now)
	policy, err := CreateWorkerLifecyclePolicy(root, WorkerLifecyclePolicyOptions{
		Now:                         now.Add(5 * time.Minute),
		StaleAfter:                  5 * time.Minute,
		WorkerStaleAfter:            2 * time.Minute,
		PollInterval:                250 * time.Millisecond,
		MaxSupervisorRuns:           3,
		TransportMode:               "metadata-only",
		IncludeLifecycleMaintenance: true,
		ExecuteLifecycleRetries:     true,
	})
	if err != nil {
		t.Fatalf("create worker lifecycle policy: %v", err)
	}

	model := NewTUIModelForWorkspace(root).SelectRuntimeTab(tui.RuntimeTabAgentRuntime)
	resized, cmd := model.Update(tea.WindowSizeMsg{Width: 360, Height: 60})
	if cmd != nil {
		t.Fatalf("expected resize update to be synchronous")
	}
	view := resized.View()
	for _, want := range []string{
		"worker lifecycle policy artifacts: total=1",
		"worker lifecycle policy artifact:",
		"worker lifecycle policy artifact state: dry-run mode=metadata-only transport=metadata-only max_runs=3",
		"worker lifecycle policy artifact cadence: stale_after=300 worker_stale_after=120 poll_interval_ms=250",
		"worker lifecycle policy artifact gates: lifecycle_maintenance=true lifecycle_retries=true",
		"worker lifecycle policy artifact summary: workers=",
		"stale_leases=",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected worker lifecycle policy row %q in TUI view:\n%s", want, view)
		}
	}
	if strings.Contains(view, "poll_interval=0") {
		t.Fatalf("worker lifecycle policy rows must not render subsecond poll intervals as zero seconds:\n%s", view)
	}
	if policy.Path == "" {
		t.Fatalf("expected policy path")
	}
}

func TestNewTUIModelForWorkspaceShowsWorkerLifecyclePolicyArtifactDetails(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Date(2026, 6, 1, 17, 0, 0, 0, time.UTC)
	createWorkerLifecycleMaintenanceFixture(t, root, now)
	policy, err := CreateWorkerLifecyclePolicy(root, WorkerLifecyclePolicyOptions{
		Now:                         now.Add(5 * time.Minute),
		StaleAfter:                  5 * time.Minute,
		WorkerStaleAfter:            2 * time.Minute,
		PollInterval:                250 * time.Millisecond,
		MaxSupervisorRuns:           3,
		TransportMode:               "metadata-only",
		IncludeLifecycleMaintenance: true,
		ExecuteLifecycleRetries:     true,
	})
	if err != nil {
		t.Fatalf("create worker lifecycle policy: %v", err)
	}

	model := NewTUIModelForWorkspace(root).SelectRuntimeTab(tui.RuntimeTabAgentRuntime)
	model = selectRuntimeRowContaining(t, model, "> worker lifecycle policy artifact:")
	model = model.ShowRuntimeActionDetails()
	resized, cmd := model.Update(tea.WindowSizeMsg{Width: 1200, Height: 80})
	if cmd != nil {
		t.Fatalf("expected resize update to be synchronous")
	}
	view := resized.View()
	for _, want := range []string{
		"[Node Inspector]",
		"runtime action: worker-lifecycle-policy-artifact",
		"artifact path: " + policy.Path,
		"state: dry-run",
		"execution mode: metadata-only",
		"transport: metadata-only",
		"ready: workers=",
		"stale_workers=",
		"stale_leases=",
		"results: stale_after=300 worker_stale_after=120 poll_interval_ms=250 max_runs=3 lifecycle_maintenance=true lifecycle_retries=true",
		"current: false",
		"mode: metadata-only",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected worker lifecycle policy detail %q:\n%s", want, view)
		}
	}
	if strings.Contains(view, "poll_interval=0") {
		t.Fatalf("worker lifecycle policy details must not render subsecond poll intervals as zero seconds:\n%s", view)
	}
}

func TestNewTUIModelForWorkspaceShowsWorkerLifecyclePolicySelectionDetails(t *testing.T) {
	root, base, candidate := createWorkerLifecyclePolicySelectionFixture(t)
	if _, err := SelectWorkerLifecyclePolicy(root, filepath.Base(candidate.Path), WorkerLifecyclePolicySelectionOptions{
		SelectedBy: "alice",
		SelectedAt: time.Date(2026, 6, 1, 17, 10, 0, 0, time.UTC),
		Reason:     "operator approved metadata policy",
	}); err != nil {
		t.Fatalf("select worker lifecycle policy: %v", err)
	}

	model := NewTUIModelForWorkspace(root).SelectRuntimeTab(tui.RuntimeTabAgentRuntime)
	resized, cmd := model.Update(tea.WindowSizeMsg{Width: 920, Height: 72})
	if cmd != nil {
		t.Fatalf("expected resize update to be synchronous")
	}
	view := resized.View()
	for _, want := range []string{
		"worker lifecycle policy selection: status=current mode=metadata-only",
		"policy=" + candidate.Path,
		"selection=" + filepath.Join(root, "runtime", "worker-lifecycle-policy-selection.json"),
		"worker lifecycle policy selection cadence: transport=metadata-only poll_interval_ms=250 max_runs=3",
		"worker lifecycle policy selection actor: selected_by=alice selected_at=2026-06-01T17:10:00Z reason=operator approved metadata policy",
		"worker lifecycle policy artifact: " + candidate.Path,
		"worker lifecycle policy artifact: " + base.Path,
		"worker lifecycle policy superseded: " + base.Path,
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected worker lifecycle policy selection row %q:\n%s", want, view)
		}
	}

	model = selectRuntimeRowContaining(t, model, "> worker lifecycle policy selection:")
	model = model.ShowRuntimeActionDetails()
	resized, cmd = model.Update(tea.WindowSizeMsg{Width: 920, Height: 72})
	if cmd != nil {
		t.Fatalf("expected selection details resize to be synchronous")
	}
	view = resized.View()
	for _, want := range []string{
		"[Node Inspector]",
		"runtime action: worker-lifecycle-policy-selection",
		"selection status: current",
		"policy path: " + candidate.Path,
		"selection path: " + filepath.Join(root, "runtime", "worker-lifecycle-policy-selection.json"),
		"transport: metadata-only",
		"results: poll_interval_ms=250 max_runs=3",
		"selected by: alice",
		"selected at: 2026-06-01T17:10:00Z",
		"reason: operator approved metadata policy",
		"superseded: 1",
		"mode: metadata-only",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected worker lifecycle policy selection detail %q:\n%s", want, view)
		}
	}
}

func TestNewTUIModelForWorkspaceShowsClearedWorkerLifecyclePolicySelectionDetails(t *testing.T) {
	root, base, candidate := createWorkerLifecyclePolicySelectionFixture(t)
	if _, err := SelectWorkerLifecyclePolicy(root, filepath.Base(candidate.Path), WorkerLifecyclePolicySelectionOptions{
		SelectedBy: "alice",
		SelectedAt: time.Date(2026, 6, 1, 17, 10, 0, 0, time.UTC),
		Reason:     "operator approved metadata policy",
	}); err != nil {
		t.Fatalf("select worker lifecycle policy: %v", err)
	}
	if _, err := ClearWorkerLifecyclePolicySelection(root, WorkerLifecyclePolicySelectionClearOptions{
		ClearedBy: "bob",
		ClearedAt: time.Date(2026, 6, 1, 17, 25, 0, 0, time.UTC),
		Reason:    "scope ended",
	}); err != nil {
		t.Fatalf("clear worker lifecycle policy selection: %v", err)
	}

	model := NewTUIModelForWorkspace(root).SelectRuntimeTab(tui.RuntimeTabAgentRuntime)
	resized, cmd := model.Update(tea.WindowSizeMsg{Width: 920, Height: 72})
	if cmd != nil {
		t.Fatalf("expected resize update to be synchronous")
	}
	view := resized.View()
	for _, want := range []string{
		"worker lifecycle policy selection: status=cleared mode=metadata-only",
		"policy=" + candidate.Path,
		"worker lifecycle policy selection cadence: transport=metadata-only poll_interval_ms=250 max_runs=3",
		"worker lifecycle policy selection clear: cleared_by=bob cleared_at=2026-06-01T17:25:00Z reason=scope ended",
		"worker lifecycle policy superseded: " + base.Path,
		"current=false",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected cleared worker lifecycle policy selection row %q:\n%s", want, view)
		}
	}
	if strings.Contains(view, "current=true") {
		t.Fatalf("cleared worker lifecycle policy selection must not mark any policy current:\n%s", view)
	}

	model = selectRuntimeRowContaining(t, model, "> worker lifecycle policy selection:")
	model = model.ShowRuntimeActionDetails()
	resized, cmd = model.Update(tea.WindowSizeMsg{Width: 920, Height: 72})
	if cmd != nil {
		t.Fatalf("expected cleared selection details resize to be synchronous")
	}
	view = resized.View()
	for _, want := range []string{
		"[Node Inspector]",
		"runtime action: worker-lifecycle-policy-selection",
		"selection status: cleared",
		"policy path: " + candidate.Path,
		"selection path: " + filepath.Join(root, "runtime", "worker-lifecycle-policy-selection.json"),
		"transport: metadata-only",
		"results: poll_interval_ms=250 max_runs=3",
		"cleared by: bob",
		"cleared at: 2026-06-01T17:25:00Z",
		"reason: scope ended",
		"superseded: 1",
		"mode: read-only",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected cleared worker lifecycle policy selection detail %q:\n%s", want, view)
		}
	}
}

func TestNewTUIModelForWorkspaceSelectsWorkerLifecyclePolicyAfterDoubleConfirm(t *testing.T) {
	root, base, candidate := createWorkerLifecyclePolicySelectionFixture(t)
	if _, err := SelectWorkerLifecyclePolicy(root, filepath.Base(base.Path), WorkerLifecyclePolicySelectionOptions{
		SelectedBy: "alice",
		SelectedAt: time.Date(2026, 6, 1, 17, 10, 0, 0, time.UTC),
		Reason:     "previous policy",
	}); err != nil {
		t.Fatalf("select base worker lifecycle policy: %v", err)
	}
	beforeSideEffects := snapshotWorkerLifecyclePolicySideEffects(t, root)

	model := NewTUIModelForWorkspace(root).SelectRuntimeTab(tui.RuntimeTabAgentRuntime)
	model = selectRuntimeRowContaining(t, model, "> worker lifecycle policy artifact: "+candidate.Path)
	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if cmd != nil {
		t.Fatalf("first x should only arm worker lifecycle policy selection confirmation")
	}
	armed, ok := updated.(tui.Model)
	if !ok {
		t.Fatalf("expected TUI model after first x, got %T", updated)
	}
	selection, found, err := ReadWorkerLifecyclePolicySelection(root)
	if err != nil {
		t.Fatalf("read policy selection after first x: %v", err)
	}
	if !found || selection.Status != "current" || selection.PolicyPath != base.Path {
		t.Fatalf("first x must not change current policy selection, found=%v selection=%#v", found, selection)
	}
	resized, resizeCmd := armed.Update(tea.WindowSizeMsg{Width: 920, Height: 72})
	if resizeCmd != nil {
		t.Fatalf("expected resize after first x to be synchronous")
	}
	for _, want := range []string{
		"execution status: confirm-required",
		"execution confirm: press x again",
		"action mode: metadata-only",
	} {
		if !strings.Contains(resized.View(), want) {
			t.Fatalf("expected worker lifecycle policy selection confirmation %q:\n%s", want, resized.View())
		}
	}

	updated, cmd = armed.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if cmd == nil {
		t.Fatalf("second x should return worker lifecycle policy selection command")
	}
	updated, cmd = updated.Update(cmd())
	if cmd != nil {
		t.Fatalf("expected worker lifecycle policy selection result update to be synchronous")
	}
	completed, ok := updated.(tui.Model)
	if !ok {
		t.Fatalf("expected TUI model after second x, got %T", updated)
	}
	selection, found, err = ReadWorkerLifecyclePolicySelection(root)
	if err != nil {
		t.Fatalf("read policy selection after second x: %v", err)
	}
	if !found || selection.Status != "current" || selection.PolicyPath != candidate.Path || selection.SelectedBy != "tui" || selection.Reason != "selected from TUI" {
		t.Fatalf("expected selected policy after second x, found=%v selection=%#v", found, selection)
	}
	assertWorkerLifecyclePolicySideEffectsUnchanged(t, root, beforeSideEffects)

	resized, resizeCmd = completed.Update(tea.WindowSizeMsg{Width: 920, Height: 72})
	if resizeCmd != nil {
		t.Fatalf("expected resize after second x to be synchronous")
	}
	view := resized.View()
	for _, want := range []string{
		"execution status: completed",
		"action output: policy selected",
		"selection status: current",
		"selected by: tui",
		"reason: selected from TUI",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected worker lifecycle policy selection execution view %q:\n%s", want, view)
		}
	}
}

func TestNewTUIModelForWorkspaceClearsWorkerLifecyclePolicySelectionAfterDoubleConfirm(t *testing.T) {
	root, _, candidate := createWorkerLifecyclePolicySelectionFixture(t)
	if _, err := SelectWorkerLifecyclePolicy(root, filepath.Base(candidate.Path), WorkerLifecyclePolicySelectionOptions{
		SelectedBy: "alice",
		SelectedAt: time.Date(2026, 6, 1, 17, 10, 0, 0, time.UTC),
		Reason:     "operator approved metadata policy",
	}); err != nil {
		t.Fatalf("select worker lifecycle policy: %v", err)
	}
	beforeSideEffects := snapshotWorkerLifecyclePolicySideEffects(t, root)

	model := NewTUIModelForWorkspace(root).SelectRuntimeTab(tui.RuntimeTabAgentRuntime)
	model = selectRuntimeRowContaining(t, model, "> worker lifecycle policy selection:")
	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if cmd != nil {
		t.Fatalf("first x should only arm worker lifecycle policy clear confirmation")
	}
	armed, ok := updated.(tui.Model)
	if !ok {
		t.Fatalf("expected TUI model after first x, got %T", updated)
	}
	selection, found, err := ReadWorkerLifecyclePolicySelection(root)
	if err != nil {
		t.Fatalf("read policy selection after first x: %v", err)
	}
	if !found || selection.Status != "current" || selection.PolicyPath != candidate.Path {
		t.Fatalf("first x must not clear current policy selection, found=%v selection=%#v", found, selection)
	}
	resized, resizeCmd := armed.Update(tea.WindowSizeMsg{Width: 920, Height: 72})
	if resizeCmd != nil {
		t.Fatalf("expected resize after first x to be synchronous")
	}
	if !strings.Contains(resized.View(), "execution status: confirm-required") {
		t.Fatalf("expected worker lifecycle policy clear confirmation:\n%s", resized.View())
	}

	updated, cmd = armed.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if cmd == nil {
		t.Fatalf("second x should return worker lifecycle policy clear command")
	}
	updated, cmd = updated.Update(cmd())
	if cmd != nil {
		t.Fatalf("expected worker lifecycle policy clear result update to be synchronous")
	}
	completed, ok := updated.(tui.Model)
	if !ok {
		t.Fatalf("expected TUI model after second x, got %T", updated)
	}
	selection, found, err = ReadWorkerLifecyclePolicySelection(root)
	if err != nil {
		t.Fatalf("read policy selection after second x: %v", err)
	}
	if !found || selection.Status != "cleared" || selection.PolicyPath != candidate.Path || selection.ClearedBy != "tui" || selection.Reason != "cleared from TUI" {
		t.Fatalf("expected cleared policy selection after second x, found=%v selection=%#v", found, selection)
	}
	assertWorkerLifecyclePolicySideEffectsUnchanged(t, root, beforeSideEffects)

	resized, resizeCmd = completed.Update(tea.WindowSizeMsg{Width: 920, Height: 72})
	if resizeCmd != nil {
		t.Fatalf("expected resize after second x to be synchronous")
	}
	view := resized.View()
	for _, want := range []string{
		"execution status: completed",
		"action output: policy selection cleared",
		"selection status: cleared",
		"cleared by: tui",
		"reason: cleared from TUI",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected worker lifecycle policy clear execution view %q:\n%s", want, view)
		}
	}
	if strings.Contains(view, "current=true") {
		t.Fatalf("cleared TUI policy selection must not leave current=true visible:\n%s", view)
	}
}

func TestNewTUIModelForWorkspaceShowsWorkerLifecyclePolicyArtifactCurrentMarkers(t *testing.T) {
	root, base, candidate := createWorkerLifecyclePolicySelectionFixture(t)
	if _, err := SelectWorkerLifecyclePolicy(root, filepath.Base(candidate.Path), WorkerLifecyclePolicySelectionOptions{
		SelectedBy: "alice",
		SelectedAt: time.Date(2026, 6, 1, 17, 10, 0, 0, time.UTC),
		Reason:     "operator approved metadata policy",
	}); err != nil {
		t.Fatalf("select worker lifecycle policy: %v", err)
	}

	model := NewTUIModelForWorkspace(root).SelectRuntimeTab(tui.RuntimeTabAgentRuntime)
	resized, cmd := model.Update(tea.WindowSizeMsg{Width: 920, Height: 72})
	if cmd != nil {
		t.Fatalf("expected resize update to be synchronous")
	}
	view := resized.View()
	for _, want := range []string{
		"worker lifecycle policy artifact current: current=false",
		"worker lifecycle policy artifact current: current=true",
		"worker lifecycle policy artifact state: dry-run mode=metadata-only transport=metadata-only max_runs=2",
		"worker lifecycle policy artifact state: dry-run mode=metadata-only transport=metadata-only max_runs=3",
		"worker lifecycle policy artifact cadence: stale_after=300 worker_stale_after=120 poll_interval_ms=500",
		"worker lifecycle policy artifact cadence: stale_after=300 worker_stale_after=120 poll_interval_ms=250",
		"worker lifecycle policy artifact path: " + filepath.Base(base.Path),
		"worker lifecycle policy artifact path: " + filepath.Base(candidate.Path),
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected worker lifecycle policy current marker %q:\n%s", want, view)
		}
	}
}

func TestNewTUIModelForWorkspaceShowsWorkerLifecyclePolicyRunDryRunRows(t *testing.T) {
	root, policy, _ := createWorkerLifecyclePolicySelectionFixture(t)
	run, err := CreateWorkerLifecyclePolicyRun(root, WorkerLifecyclePolicyRunOptions{
		Now:    time.Date(2026, 6, 1, 18, 5, 0, 0, time.UTC),
		Actor:  "operator",
		Reason: "persisted dry run",
	})
	if err != nil {
		t.Fatalf("create dry-run worker lifecycle policy run: %v", err)
	}
	policyRunsBefore := countJSON(t, filepath.Join(root, "runtime", "worker-lifecycle-policy-runs"))
	supervisorsBefore := countJSON(t, filepath.Join(root, "runtime", "worker-lifecycle-maintenance-supervisor-runs"))

	model := NewTUIModelForWorkspace(root).SelectRuntimeTab(tui.RuntimeTabAgentRuntime)
	resized, cmd := model.Update(tea.WindowSizeMsg{Width: 1400, Height: 90})
	if cmd != nil {
		t.Fatalf("expected resize update to be synchronous")
	}
	view := resized.View()
	selectionPath := filepath.Join(root, "runtime", "worker-lifecycle-policy-selection.json")
	projectedRows := strings.Join(workerLifecyclePolicyRunRuntimeRows([]WorkerLifecyclePolicyRun{run}), "\n")
	for _, want := range []string{
		"worker lifecycle policy runs: total=1",
		"worker lifecycle policy run artifact: " + run.Path + " status=dry-run mode=metadata-only policy=" + policy.Path + " selection=" + selectionPath + " max_runs=2 poll_interval_ms=500 transport=metadata-only include_lifecycle_maintenance=true execute_lifecycle_retries=true execute_lease_recovery=false supervisor_created=0 skip=none",
		"worker lifecycle policy run cadence: max_runs=2 poll_interval_ms=500 transport=metadata-only",
		"worker lifecycle policy run gates: include_lifecycle_maintenance=true execute_lifecycle_retries=true execute_lease_recovery=false supervisor_created=0",
		"worker lifecycle policy run refs: policy=" + filepath.Base(policy.Path) + " selection=worker-lifecycle-policy-selection.json supervisor=none",
		"worker lifecycle policy run path: " + filepath.Base(run.Path),
	} {
		if !strings.Contains(projectedRows, want) {
			t.Fatalf("expected projected worker lifecycle policy run row %q in rows:\n%s", want, projectedRows)
		}
	}
	for _, want := range []string{
		"worker lifecycle policy runs: total=1",
		"worker lifecycle policy run artifact:",
		"worker lifecycle policy run cadence: max_runs=2 poll_interval_ms=500 transport=metadata-only",
		"worker lifecycle policy run gates: include_lifecycle_maintenance=true execute_lifecycle_retries=true execute_lease_recovery=false supervisor_created=0",
		"worker lifecycle policy run refs: policy=" + filepath.Base(policy.Path) + " selection=worker-lifecycle-policy-selection.json supervisor=none",
		"worker lifecycle policy run path: " + filepath.Base(run.Path),
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected visible worker lifecycle policy run row %q in TUI view:\n%s", want, view)
		}
	}
	if strings.Contains(projectedRows, "worker lifecycle policy run skip:") || strings.Contains(view, "worker lifecycle policy run skip:") {
		t.Fatalf("dry-run policy run without skip reason must not render a skip detail row:\nrows:\n%s\nview:\n%s", projectedRows, view)
	}
	if countJSON(t, filepath.Join(root, "runtime", "worker-lifecycle-policy-runs")) != policyRunsBefore {
		t.Fatalf("TUI policy-run projection must not create policy-run artifacts")
	}
	if countJSON(t, filepath.Join(root, "runtime", "worker-lifecycle-maintenance-supervisor-runs")) != supervisorsBefore {
		t.Fatalf("TUI policy-run projection must not create maintenance supervisor artifacts")
	}
}

func TestNewTUIModelForWorkspaceShowsWorkerLifecyclePolicyRunSupervisorRefs(t *testing.T) {
	root, policy, _ := createWorkerLifecyclePolicySelectionFixture(t)
	run, err := CreateWorkerLifecyclePolicyRun(root, WorkerLifecyclePolicyRunOptions{
		Execute: true,
		Now:     time.Date(2026, 6, 1, 18, 15, 0, 0, time.UTC),
		Actor:   "operator",
		Reason:  "linked supervisor visibility",
	})
	if err != nil {
		t.Fatalf("create completed worker lifecycle policy run: %v", err)
	}
	if run.SupervisorPath == "" {
		t.Fatalf("expected completed policy run to link a supervisor: %#v", run)
	}
	policyRunsBefore := countJSON(t, filepath.Join(root, "runtime", "worker-lifecycle-policy-runs"))
	supervisorsBefore := countJSON(t, filepath.Join(root, "runtime", "worker-lifecycle-maintenance-supervisor-runs"))

	model := NewTUIModelForWorkspace(root).SelectRuntimeTab(tui.RuntimeTabAgentRuntime)
	resized, cmd := model.Update(tea.WindowSizeMsg{Width: 1400, Height: 90})
	if cmd != nil {
		t.Fatalf("expected resize update to be synchronous")
	}
	view := resized.View()
	selectionPath := filepath.Join(root, "runtime", "worker-lifecycle-policy-selection.json")
	projectedRows := strings.Join(workerLifecyclePolicyRunRuntimeRows([]WorkerLifecyclePolicyRun{run}), "\n")
	for _, want := range []string{
		"worker lifecycle policy runs: total=1",
		"worker lifecycle policy run artifact: " + run.Path + " status=completed mode=metadata-only policy=" + policy.Path + " selection=" + selectionPath + " max_runs=2 poll_interval_ms=500 transport=metadata-only include_lifecycle_maintenance=true execute_lifecycle_retries=true execute_lease_recovery=false supervisor_created=1 skip=none",
		"worker lifecycle policy run cadence: max_runs=2 poll_interval_ms=500 transport=metadata-only",
		"worker lifecycle policy run gates: include_lifecycle_maintenance=true execute_lifecycle_retries=true execute_lease_recovery=false supervisor_created=1",
		"worker lifecycle policy run refs: policy=" + filepath.Base(policy.Path) + " selection=worker-lifecycle-policy-selection.json supervisor=" + filepath.Base(run.SupervisorPath),
		"worker lifecycle policy run path: " + filepath.Base(run.Path),
	} {
		if !strings.Contains(projectedRows, want) {
			t.Fatalf("expected projected completed worker lifecycle policy run row %q in rows:\n%s", want, projectedRows)
		}
	}
	for _, want := range []string{
		"worker lifecycle policy runs: total=1",
		"worker lifecycle policy run artifact:",
		"worker lifecycle policy run cadence: max_runs=2 poll_interval_ms=500 transport=metadata-only",
		"worker lifecycle policy run gates: include_lifecycle_maintenance=true execute_lifecycle_retries=true execute_lease_recovery=false supervisor_created=1",
		"worker lifecycle policy run refs: policy=" + filepath.Base(policy.Path) + " selection=worker-lifecycle-policy-selection.json supervisor=" + filepath.Base(run.SupervisorPath),
		"worker lifecycle policy run path: " + filepath.Base(run.Path),
		"worker lifecycle maintenance supervisors: total=1",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected visible completed worker lifecycle policy run row %q in TUI view:\n%s", want, view)
		}
	}
	runIndex := strings.Index(view, "worker lifecycle policy run artifact:")
	supervisorIndex := strings.Index(view, "worker lifecycle maintenance supervisors: total=1")
	if runIndex < 0 || supervisorIndex < 0 || runIndex > supervisorIndex {
		t.Fatalf("expected policy-run rows before maintenance supervisor rows, run_index=%d supervisor_index=%d:\n%s", runIndex, supervisorIndex, view)
	}
	if strings.Contains(projectedRows, "worker lifecycle policy run skip:") || strings.Contains(view, "worker lifecycle policy run skip:") {
		t.Fatalf("completed policy run without skip reason must not render a skip detail row:\nrows:\n%s\nview:\n%s", projectedRows, view)
	}
	if countJSON(t, filepath.Join(root, "runtime", "worker-lifecycle-policy-runs")) != policyRunsBefore {
		t.Fatalf("TUI policy-run projection must not create policy-run artifacts")
	}
	if countJSON(t, filepath.Join(root, "runtime", "worker-lifecycle-maintenance-supervisor-runs")) != supervisorsBefore {
		t.Fatalf("TUI policy-run projection must not create maintenance supervisor artifacts")
	}
}

func TestNewTUIModelForWorkspaceShowsWorkerLifecyclePolicySchedulerRunRows(t *testing.T) {
	root, policy, _ := createWorkerLifecyclePolicySelectionFixture(t)
	run, err := CreateWorkerLifecyclePolicyRun(root, WorkerLifecyclePolicyRunOptions{
		Now:    time.Date(2026, 6, 1, 18, 35, 0, 0, time.UTC),
		Actor:  "operator",
		Reason: "scheduler target run",
	})
	if err != nil {
		t.Fatalf("create worker lifecycle policy run: %v", err)
	}
	selectionPath := filepath.Join(root, "runtime", "worker-lifecycle-policy-selection.json")
	schedulerDir := filepath.Join(root, "runtime", "worker-lifecycle-policy-scheduler-runs")
	if err := os.MkdirAll(schedulerDir, 0o755); err != nil {
		t.Fatalf("create scheduler dir: %v", err)
	}
	schedulerPath := filepath.Join(schedulerDir, "scheduler-001.json")
	scheduler := map[string]any{
		"version":                       1,
		"path":                          schedulerPath,
		"status":                        "failed",
		"mode":                          "metadata-only",
		"selection_path":                selectionPath,
		"policy_path":                   policy.Path,
		"policy_basename":               filepath.Base(policy.Path),
		"poll_interval_ms":              500,
		"max_runs_requested":            2,
		"policy_runs_created":           1,
		"policy_run_paths":              []string{run.Path},
		"supervisors_created":           1,
		"skip_reason":                   "policy run disabled by operator",
		"error":                         "scheduler failed to open policy",
		"transport":                     "metadata-only",
		"include_lifecycle_maintenance": true,
		"execute_lifecycle_retries":     true,
	}
	data, err := json.MarshalIndent(scheduler, "", "  ")
	if err != nil {
		t.Fatalf("encode scheduler artifact: %v", err)
	}
	if err := os.WriteFile(schedulerPath, data, 0o644); err != nil {
		t.Fatalf("write scheduler artifact: %v", err)
	}
	policyRunsBefore := countJSON(t, filepath.Join(root, "runtime", "worker-lifecycle-policy-runs"))
	schedulerRunsBefore := countJSON(t, schedulerDir)

	model := NewTUIModelForWorkspace(root).SelectRuntimeTab(tui.RuntimeTabAgentRuntime)
	resized, cmd := model.Update(tea.WindowSizeMsg{Width: 1500, Height: 96})
	if cmd != nil {
		t.Fatalf("expected resize update to be synchronous")
	}
	view := resized.View()
	listedSchedulerRuns, err := ListWorkerLifecyclePolicySchedulerRuns(root)
	if err != nil {
		t.Fatalf("list scheduler runs: %v", err)
	}
	projectedRows := strings.Join(workerLifecyclePolicySchedulerRunRuntimeRows(listedSchedulerRuns), "\n")
	for _, want := range []string{
		"worker lifecycle policy scheduler runs: total=1",
		"worker lifecycle policy scheduler run artifact: " + schedulerPath + " status=failed mode=metadata-only policy=" + policy.Path + " selection=" + selectionPath + " policy_basename=" + filepath.Base(policy.Path) + " selection_basename=worker-lifecycle-policy-selection.json max_runs=2 poll_interval_ms=500 transport=metadata-only include_lifecycle_maintenance=true execute_lifecycle_retries=true execute_lease_recovery=false policy_runs_created=1 supervisor_created=1 skip=policy run disabled by operator error=scheduler failed to open policy",
		"worker lifecycle policy scheduler run compact: status=failed mode=metadata-only max_runs=2 poll_interval_ms=500 policy=" + filepath.Base(policy.Path) + " selection=worker-lifecycle-policy-selection.json policy_runs_created=1 supervisor_created=1",
		"worker lifecycle policy scheduler run gates: include_lifecycle_maintenance=true execute_lifecycle_retries=true execute_lease_recovery=false transport=metadata-only",
		"worker lifecycle policy scheduler run skip: policy run disabled by operator",
		"worker lifecycle policy scheduler run error: scheduler failed to open policy",
		"worker lifecycle policy scheduler run path: scheduler-001.json",
	} {
		if !strings.Contains(projectedRows, want) {
			t.Fatalf("expected projected policy scheduler row %q in rows:\n%s", want, projectedRows)
		}
	}
	for _, want := range []string{
		"worker lifecycle policy runs: total=1",
		"worker lifecycle policy scheduler runs: total=1",
		"worker lifecycle policy scheduler run artifact:",
		"worker lifecycle policy scheduler run compact: status=failed mode=metadata-only max_runs=2 poll_interval_ms=500 policy=" + filepath.Base(policy.Path) + " selection=worker-lifecycle-policy-selection.json policy_runs_created=1 supervisor_created=1",
		"worker lifecycle policy scheduler run gates: include_lifecycle_maintenance=true execute_lifecycle_retries=true execute_lease_recovery=false transport=metadata-only",
		"worker lifecycle policy scheduler run skip: policy run disabled by operator",
		"worker lifecycle policy scheduler run error: scheduler failed to open policy",
		"worker lifecycle policy scheduler run path: scheduler-001.json",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected visible policy scheduler row %q in TUI view:\n%s", want, view)
		}
	}
	policyRunIndex := strings.Index(view, "worker lifecycle policy run artifact:")
	schedulerRunIndex := strings.Index(view, "worker lifecycle policy scheduler run artifact:")
	if policyRunIndex < 0 || schedulerRunIndex < 0 || policyRunIndex > schedulerRunIndex {
		t.Fatalf("expected scheduler-run rows near and after policy-run rows, policy_run_index=%d scheduler_run_index=%d:\n%s", policyRunIndex, schedulerRunIndex, view)
	}
	if countJSON(t, filepath.Join(root, "runtime", "worker-lifecycle-policy-runs")) != policyRunsBefore {
		t.Fatalf("TUI scheduler-run projection must not create policy-run artifacts")
	}
	if countJSON(t, schedulerDir) != schedulerRunsBefore {
		t.Fatalf("TUI scheduler-run projection must not create scheduler-run artifacts")
	}
}

func TestNewTUIModelForWorkspaceShowsWorkerDispatchRetrySchedulerRunRows(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	schedulerDir := filepath.Join(root, "runtime", "worker-dispatch-retry-scheduler-runs")
	if err := os.MkdirAll(schedulerDir, 0o755); err != nil {
		t.Fatalf("create dispatch retry scheduler dir: %v", err)
	}
	schedulerPath := filepath.Join(schedulerDir, "scheduler-001.json")
	scheduler := map[string]any{
		"version":                          1,
		"path":                             schedulerPath,
		"status":                           "completed",
		"mode":                             "metadata-only",
		"created_at":                       "2026-06-01T18:45:00Z",
		"completed_at":                     "2026-06-01T18:45:02Z",
		"actor":                            "scheduler-test",
		"reason":                           "operator audit",
		"max_runs_requested":               2,
		"max_runs":                         2,
		"dispatch_retry_claims":            true,
		"retry_transport_mode":             "file-outbox",
		"retry_plan_paths":                 []string{filepath.Join(root, "runtime", "worker-dispatch-retry-plans", "plan-001.json"), filepath.Join(root, "runtime", "worker-dispatch-retry-plans", "plan-002.json")},
		"retry_dispatch_paths":             []string{filepath.Join(root, "runtime", "worker-dispatch-retry-dispatches", "dispatch-001.json"), filepath.Join(root, "runtime", "worker-dispatch-retry-dispatches", "dispatch-002.json")},
		"retry_dispatch_execution_paths":   []string{filepath.Join(root, "runtime", "worker-dispatch-retry-dispatch-executions", "execution-001.json"), filepath.Join(root, "runtime", "worker-dispatch-retry-dispatch-executions", "execution-002.json")},
		"retry_claims_created":             3,
		"retry_claims_dispatched":          2,
		"retry_transport_requests_created": 2,
	}
	data, err := json.MarshalIndent(scheduler, "", "  ")
	if err != nil {
		t.Fatalf("encode dispatch retry scheduler artifact: %v", err)
	}
	if err := os.WriteFile(schedulerPath, data, 0o644); err != nil {
		t.Fatalf("write dispatch retry scheduler artifact: %v", err)
	}
	schedulerRunsBefore := countJSON(t, schedulerDir)
	retryPlansBefore := countJSON(t, filepath.Join(root, "runtime", "worker-dispatch-retry-plans"))
	retryDispatchesBefore := countJSON(t, filepath.Join(root, "runtime", "worker-dispatch-retry-dispatches"))
	retryExecutionsBefore := countJSON(t, filepath.Join(root, "runtime", "worker-dispatch-retry-dispatch-executions"))

	model := NewTUIModelForWorkspace(root).SelectRuntimeTab(tui.RuntimeTabAgentRuntime)
	resized, cmd := model.Update(tea.WindowSizeMsg{Width: 1500, Height: 96})
	if cmd != nil {
		t.Fatalf("expected resize update to be synchronous")
	}
	view := resized.View()
	listedSchedulerRuns, err := ListRuntimeWorkerDispatchRetrySchedulerRuns(root)
	if err != nil {
		t.Fatalf("list dispatch retry scheduler runs: %v", err)
	}
	projectedRows := strings.Join(runtimeWorkerDispatchRetrySchedulerRunRuntimeRows(listedSchedulerRuns), "\n")
	for _, want := range []string{
		"worker dispatch retry scheduler runs: total=1",
		"worker dispatch retry scheduler run artifact: " + schedulerPath + " status=completed mode=metadata-only max_runs=2 dispatch_retry_claims=true retry_transport=file-outbox retry_plans_created=2 retry_dispatches_created=2 retry_dispatch_executions_created=2",
		"worker dispatch retry scheduler run compact: status=completed mode=metadata-only max_runs=2 retry_plans_created=2 retry_dispatches_created=2 retry_dispatch_executions_created=2",
		"worker dispatch retry scheduler run handoff: dispatch_retry_claims=true retry_transport=file-outbox claims_created=3 claims_dispatched=2 transport_requests=2",
		"worker dispatch retry scheduler run path: scheduler-001.json",
	} {
		if !strings.Contains(projectedRows, want) {
			t.Fatalf("expected projected dispatch retry scheduler row %q in rows:\n%s", want, projectedRows)
		}
		if !strings.Contains(view, want) {
			t.Fatalf("expected visible dispatch retry scheduler row %q in TUI view:\n%s", want, view)
		}
	}
	if countJSON(t, schedulerDir) != schedulerRunsBefore {
		t.Fatalf("TUI dispatch retry scheduler projection must not create scheduler-run artifacts")
	}
	if countJSON(t, filepath.Join(root, "runtime", "worker-dispatch-retry-plans")) != retryPlansBefore {
		t.Fatalf("TUI dispatch retry scheduler projection must not create retry plan artifacts")
	}
	if countJSON(t, filepath.Join(root, "runtime", "worker-dispatch-retry-dispatches")) != retryDispatchesBefore {
		t.Fatalf("TUI dispatch retry scheduler projection must not create retry dispatch artifacts")
	}
	if countJSON(t, filepath.Join(root, "runtime", "worker-dispatch-retry-dispatch-executions")) != retryExecutionsBefore {
		t.Fatalf("TUI dispatch retry scheduler projection must not create retry execution artifacts")
	}
}

func TestNewTUIModelForWorkspaceShowsReportProjectionDispatchRows(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	run, err := RunWorkspaceWithOptions(context.Background(), root, RunOptions{DeferReportProjection: true})
	if err != nil {
		t.Fatalf("run workspace with deferred report projection: %v", err)
	}
	dispatch, err := CreateReportProjectionDispatch(context.Background(), root, run.RunID, ReportProjectionDispatchOptions{
		Now:    time.Date(2026, 6, 2, 4, 30, 0, 0, time.UTC),
		Actor:  "alice",
		Reason: "operator review",
	})
	if err != nil {
		t.Fatalf("create report projection dispatch: %v", err)
	}
	dispatchDir := filepath.Join(root, "reports", "report-projection-dispatches")
	dispatchesBefore := countJSON(t, dispatchDir)
	reportFilesBefore, err := filepath.Glob(filepath.Join(root, "reports", "*.md"))
	if err != nil {
		t.Fatalf("glob reports: %v", err)
	}

	model := NewTUIModelForWorkspace(root).SelectRuntimeTab(tui.RuntimeTabTimelineLanes)
	resized, cmd := model.Update(tea.WindowSizeMsg{Width: 1500, Height: 96})
	if cmd != nil {
		t.Fatalf("expected resize update to be synchronous")
	}
	view := resized.View()
	listedDispatches, err := ListReportProjectionDispatches(root)
	if err != nil {
		t.Fatalf("list report projection dispatches: %v", err)
	}
	projectedRows := strings.Join(reportProjectionDispatchRuntimeRows(listedDispatches), "\n")
	for _, want := range []string{
		"report projection dispatches: total=1",
		"report projection dispatch artifact: " + dispatch.Path + " status=dry-run mode=metadata-only run=" + run.RunID,
		"events=",
		"planned_report=" + dispatch.ReportPath,
		"actor=alice",
		"reason=operator_review",
		"report projection dispatch compact: status=dry-run mode=metadata-only run=" + run.RunID + " events=",
		"report projection dispatch path: " + filepath.Base(dispatch.Path),
	} {
		if !strings.Contains(projectedRows, want) {
			t.Fatalf("expected projected report dispatch row %q in rows:\n%s", want, projectedRows)
		}
		if !strings.Contains(view, want) {
			t.Fatalf("expected visible report dispatch row %q in TUI view:\n%s", want, view)
		}
	}
	if strings.Contains(projectedRows, " report=") {
		t.Fatalf("report projection dispatch TUI rows must label planned report paths explicitly:\n%s", projectedRows)
	}
	if countJSON(t, dispatchDir) != dispatchesBefore {
		t.Fatalf("TUI report dispatch projection must not create dispatch artifacts")
	}
	reportFilesAfter, err := filepath.Glob(filepath.Join(root, "reports", "*.md"))
	if err != nil {
		t.Fatalf("glob reports after TUI: %v", err)
	}
	if len(reportFilesAfter) != len(reportFilesBefore) {
		t.Fatalf("TUI report dispatch projection must not create markdown reports, before=%v after=%v", reportFilesBefore, reportFilesAfter)
	}
}

func TestNewTUIModelForWorkspaceExecutesReportProjectionDispatchAfterDoubleConfirm(t *testing.T) {
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
		Actor:  "alice",
		Reason: "operator review",
	})
	if err != nil {
		t.Fatalf("create report projection dispatch: %v", err)
	}
	dispatchDir := filepath.Join(root, "reports", "report-projection-dispatches")
	executionDir := filepath.Join(root, "reports", "report-projection-dispatch-executions")
	dispatchesBefore := countJSON(t, dispatchDir)
	executionsBefore := countJSON(t, executionDir)
	reportFilesBefore, err := filepath.Glob(filepath.Join(root, "reports", "*.md"))
	if err != nil {
		t.Fatalf("glob reports before TUI execution: %v", err)
	}
	queueClaimsBefore := countJSON(t, filepath.Join(root, "runtime", "worker-queue-claims"))
	transportOutboxBefore := countJSON(t, filepath.Join(root, "runtime", "worker-transport-outbox"))
	eventsBefore := reportProjectionDispatchTestEventCount(t, ctx, root, run.RunID)

	model := NewTUIModelForWorkspace(root).SelectRuntimeTab(tui.RuntimeTabTimelineLanes)
	model = selectRuntimeRowContaining(t, model, "> report projection dispatch artifact:")
	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if cmd != nil {
		t.Fatalf("first x should only arm report projection dispatch execution")
	}
	armed, ok := updated.(tui.Model)
	if !ok {
		t.Fatalf("expected TUI model after first x, got %T", updated)
	}
	if got := countJSON(t, executionDir); got != executionsBefore {
		t.Fatalf("first x must not create report projection execution audits, before=%d after=%d", executionsBefore, got)
	}
	reportFilesAfterFirst, err := filepath.Glob(filepath.Join(root, "reports", "*.md"))
	if err != nil {
		t.Fatalf("glob reports after first x: %v", err)
	}
	if len(reportFilesAfterFirst) != len(reportFilesBefore) {
		t.Fatalf("first x must not create markdown reports, before=%v after=%v", reportFilesBefore, reportFilesAfterFirst)
	}
	resized, resizeCmd := armed.Update(tea.WindowSizeMsg{Width: 900, Height: 46})
	if resizeCmd != nil {
		t.Fatalf("expected resize after first x to be synchronous")
	}
	if !strings.Contains(resized.View(), "execution status: confirm-required") || !strings.Contains(resized.View(), "action: report projection dispatch artifact selected") {
		t.Fatalf("expected report projection dispatch execution confirmation:\n%s", resized.View())
	}

	updated, cmd = armed.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if cmd == nil {
		t.Fatalf("second x should execute report projection dispatch")
	}
	msg := cmd()
	updated, cmd = updated.Update(msg)
	if cmd != nil {
		t.Fatalf("expected report projection dispatch execution result to be synchronous")
	}
	if got := countJSON(t, dispatchDir); got != dispatchesBefore {
		t.Fatalf("TUI execution must not create extra report projection dispatches, before=%d after=%d", dispatchesBefore, got)
	}
	if got := countJSON(t, executionDir); got != executionsBefore+1 {
		t.Fatalf("TUI execution must create exactly one report projection execution audit, before=%d after=%d", executionsBefore, got)
	}
	reportFilesAfter, err := filepath.Glob(filepath.Join(root, "reports", "*.md"))
	if err != nil {
		t.Fatalf("glob reports after TUI execution: %v", err)
	}
	if len(reportFilesAfter) != len(reportFilesBefore)+1 {
		t.Fatalf("TUI execution must create exactly one markdown report, before=%v after=%v", reportFilesBefore, reportFilesAfter)
	}
	replay, err := ReplayWorkspace(ctx, root, run.RunID)
	if err != nil {
		t.Fatalf("replay workspace after TUI execution: %v", err)
	}
	if replay.ReportPath != dispatch.ReportPath {
		t.Fatalf("expected report_ready event for dispatch report path %q, got %q", dispatch.ReportPath, replay.ReportPath)
	}
	eventsAfter := reportProjectionDispatchTestEventCount(t, ctx, root, run.RunID)
	if eventsAfter <= eventsBefore {
		t.Fatalf("TUI execution should append report projection replay events, before=%d after=%d", eventsBefore, eventsAfter)
	}
	if got := countJSON(t, filepath.Join(root, "runtime", "worker-queue-claims")); got != queueClaimsBefore {
		t.Fatalf("TUI report projection execution must not create worker queue claims, before=%d after=%d", queueClaimsBefore, got)
	}
	if got := countJSON(t, filepath.Join(root, "runtime", "worker-transport-outbox")); got != transportOutboxBefore {
		t.Fatalf("TUI report projection execution must not create worker transport requests, before=%d after=%d", transportOutboxBefore, got)
	}
	executions, err := ListReportProjectionDispatchExecutions(root)
	if err != nil {
		t.Fatalf("list report projection executions after TUI execution: %v", err)
	}
	if len(executions) != executionsBefore+1 {
		t.Fatalf("expected one listed TUI execution, before=%d executions=%#v", executionsBefore, executions)
	}
	execution := executions[len(executions)-1]
	if execution.Status != "completed" || execution.DispatchPath != dispatch.Path || execution.ReportPath != dispatch.ReportPath || !execution.Written || execution.Actor != "tui" || execution.Reason != "executed from TUI" {
		t.Fatalf("unexpected TUI report projection execution audit: %#v", execution)
	}
	completed, ok := updated.(tui.Model)
	if !ok {
		t.Fatalf("expected TUI model after result, got %T", updated)
	}
	resized, resizeCmd = completed.Update(tea.WindowSizeMsg{Width: 900, Height: 46})
	if resizeCmd != nil {
		t.Fatalf("expected resize after result to be synchronous")
	}
	for _, want := range []string{
		"execution status: completed",
		"action output: report projection dispatch executed",
		"report projection dispatch executions: total=1",
		"report projection dispatch execution artifact: " + execution.Path,
		"written=true actor=tui reason=executed_from_TUI",
	} {
		if !strings.Contains(resized.View(), want) {
			t.Fatalf("expected report projection dispatch execution result view %q:\n%s", want, resized.View())
		}
	}
}

func TestNewTUIModelForWorkspaceShowsReportProjectionExecutionRows(t *testing.T) {
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
		Actor:  "alice",
		Reason: "operator review",
	})
	if err != nil {
		t.Fatalf("create report projection dispatch: %v", err)
	}
	execution, err := ExecuteReportProjectionDispatch(ctx, root, filepath.Base(dispatch.Path), ReportProjectionDispatchExecutionOptions{
		Execute: true,
		Actor:   "bob",
		Reason:  "approved projection",
	})
	if err != nil {
		t.Fatalf("execute report projection dispatch: %v", err)
	}

	model := NewTUIModelForWorkspace(root).SelectRuntimeTab(tui.RuntimeTabTimelineLanes)
	resized, cmd := model.Update(tea.WindowSizeMsg{Width: 1500, Height: 96})
	if cmd != nil {
		t.Fatalf("expected resize update to be synchronous")
	}
	view := resized.View()
	listedExecutions, err := ListReportProjectionDispatchExecutions(root)
	if err != nil {
		t.Fatalf("list report projection executions: %v", err)
	}
	projectedRows := strings.Join(reportProjectionDispatchExecutionRuntimeRows(listedExecutions), "\n")
	for _, want := range []string{
		"report projection dispatch executions: total=1",
		"report projection dispatch execution artifact: " + execution.Path + " status=completed mode=execute dispatch=" + dispatch.Path + " run=" + run.RunID,
		"events=",
		"report=" + dispatch.ReportPath,
		"written=true",
		"actor=bob",
		"reason=approved_projection",
		"report projection dispatch execution compact: status=completed mode=execute run=" + run.RunID + " events=",
		"report projection dispatch execution path: " + filepath.Base(execution.Path),
	} {
		if !strings.Contains(projectedRows, want) {
			t.Fatalf("expected projected report execution row %q in rows:\n%s", want, projectedRows)
		}
	}
	for _, want := range []string{
		"report projection dispatch executions: total=1",
		"report projection dispatch execution compact: status=completed mode=execute run=" + run.RunID + " events=",
		"written=true actor=bob reason=approved_projection",
		"report projection dispatch execution path: " + filepath.Base(execution.Path),
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected visible report execution row %q in TUI view:\n%s", want, view)
		}
	}
}

func TestNewTUIModelForWorkspaceShowsReportProjectionExecutionReviewSelectionRows(t *testing.T) {
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
		t.Fatalf("create report projection dispatch: %v", err)
	}
	execution, err := ExecuteReportProjectionDispatch(ctx, root, filepath.Base(dispatch.Path), ReportProjectionDispatchExecutionOptions{
		Execute: true,
		Actor:   "bob",
		Reason:  "approved projection",
	})
	if err != nil {
		t.Fatalf("execute report projection dispatch: %v", err)
	}
	if _, err := AnnotateReportProjectionDispatchExecution(root, filepath.Base(execution.Path), ReportProjectionDispatchExecutionAnnotationOptions{
		Decision:   "accepted",
		Reviewer:   "alice",
		ReviewedAt: time.Date(2026, 6, 2, 7, 5, 0, 0, time.UTC),
		Reason:     "reviewed report",
	}); err != nil {
		t.Fatalf("annotate report projection execution: %v", err)
	}
	selection, err := SelectReportProjectionDispatchExecution(root, filepath.Base(execution.Path), ReportProjectionDispatchExecutionSelectionOptions{
		SelectedBy: "carol",
		SelectedAt: time.Date(2026, 6, 2, 7, 10, 0, 0, time.UTC),
		Reason:     "current reviewed report",
	})
	if err != nil {
		t.Fatalf("select report projection execution: %v", err)
	}

	model := NewTUIModelForWorkspace(root).SelectRuntimeTab(tui.RuntimeTabTimelineLanes)
	resized, cmd := model.Update(tea.WindowSizeMsg{Width: 1500, Height: 96})
	if cmd != nil {
		t.Fatalf("expected resize update to be synchronous")
	}
	view := resized.View()
	listedExecutions, err := ListReportProjectionDispatchExecutions(root)
	if err != nil {
		t.Fatalf("list report projection executions: %v", err)
	}
	projectedRows := strings.Join(reportProjectionDispatchExecutionRuntimeRowsWithSelection(listedExecutions, selection, true), "\n")
	for _, want := range []string{
		"report projection dispatch executions: total=1 annotated=1 current=1",
		"report projection execution selection: status=current mode=metadata-only current=true selected_by=carol",
		"execution=" + execution.Path,
		"selected_by=carol selected_at=2026-06-02T07:10:00Z reason=current_reviewed_report",
		"report projection dispatch execution artifact: " + execution.Path + " status=completed mode=execute dispatch=" + dispatch.Path + " run=" + run.RunID,
		"decision=accepted reviewer=alice reviewed_at=2026-06-02T07:05:00Z review_reason=reviewed_report current=true",
		"report projection execution review: mode=metadata-only decision=accepted reviewer=alice reviewed_at=2026-06-02T07:05:00Z review_reason=reviewed_report execution=" + execution.Path,
		"report projection dispatch execution annotation: decision=accepted reviewer=alice reviewed_at=2026-06-02T07:05:00Z reason=reviewed_report",
	} {
		if !strings.Contains(projectedRows, want) {
			t.Fatalf("expected projected report execution review/selection row %q in rows:\n%s", want, projectedRows)
		}
	}
	for _, want := range []string{
		"report projection dispatch executions: total=1 annotated=1 current=1",
		"report projection execution selection: status=current",
		"report projection dispatch execution artifact:",
		"decision=accepted reviewer=alice",
		"report projection execution review: mode=metadata-only decision=accepted",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected visible report execution review/selection row %q in TUI view:\n%s", want, view)
		}
	}

	model = selectRuntimeRowContaining(t, model, "> report projection execution selection:")
	model = model.ShowRuntimeActionDetails()
	resized, cmd = model.Update(tea.WindowSizeMsg{Width: 900, Height: 46})
	if cmd != nil {
		t.Fatalf("expected selection details resize to be synchronous")
	}
	view = resized.View()
	for _, want := range []string{
		"runtime action: report-projection-execution-selection",
		"artifact path: " + execution.Path,
		"dispatch path: " + dispatch.Path,
		"result path: " + execution.ReportPath,
		"state: current",
		"selected by: carol",
		"selected at: 2026-06-02T07:10:00Z",
		"reason: current_reviewed_report",
		"mode: metadata-only",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected report projection execution selection details %q:\n%s", want, view)
		}
	}
}

func TestNewTUIModelForWorkspaceSelectsReportProjectionExecutionAfterDoubleConfirm(t *testing.T) {
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
		t.Fatalf("create report projection dispatch: %v", err)
	}
	execution, err := ExecuteReportProjectionDispatch(ctx, root, filepath.Base(dispatch.Path), ReportProjectionDispatchExecutionOptions{Execute: true})
	if err != nil {
		t.Fatalf("execute report projection dispatch: %v", err)
	}
	if _, err := AnnotateReportProjectionDispatchExecution(root, filepath.Base(execution.Path), ReportProjectionDispatchExecutionAnnotationOptions{Decision: "accepted"}); err != nil {
		t.Fatalf("accept report projection execution: %v", err)
	}
	executionsBefore := countJSON(t, filepath.Join(root, "reports", "report-projection-dispatch-executions"))

	model := NewTUIModelForWorkspace(root).SelectRuntimeTab(tui.RuntimeTabTimelineLanes)
	model = selectRuntimeRowContaining(t, model, "> report projection dispatch execution artifact:")
	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if cmd != nil {
		t.Fatalf("first x should only arm report projection execution selection")
	}
	armed, ok := updated.(tui.Model)
	if !ok {
		t.Fatalf("expected TUI model after first x, got %T", updated)
	}
	if _, found, err := ReadReportProjectionDispatchExecutionSelection(root); err != nil || found {
		t.Fatalf("first x must not write selection marker, found=%v err=%v", found, err)
	}
	resized, resizeCmd := armed.Update(tea.WindowSizeMsg{Width: 900, Height: 46})
	if resizeCmd != nil {
		t.Fatalf("expected resize after first x to be synchronous")
	}
	if !strings.Contains(resized.View(), "execution status: confirm-required") || !strings.Contains(resized.View(), "action: report projection dispatch execution artifact selected") {
		t.Fatalf("expected report projection execution selection confirmation:\n%s", resized.View())
	}

	updated, cmd = armed.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if cmd == nil {
		t.Fatalf("second x should select report projection execution")
	}
	msg := cmd()
	updated, cmd = updated.Update(msg)
	if cmd != nil {
		t.Fatalf("expected report projection execution selection result to be synchronous")
	}
	selection, found, err := ReadReportProjectionDispatchExecutionSelection(root)
	if err != nil {
		t.Fatalf("read report projection execution selection: %v", err)
	}
	if !found || selection.Status != "current" || selection.ExecutionPath != execution.Path || selection.SelectedBy != "tui" || selection.Reason != "selected from TUI" {
		t.Fatalf("expected TUI-selected report projection execution, found=%v selection=%#v", found, selection)
	}
	if got := countJSON(t, filepath.Join(root, "reports", "report-projection-dispatch-executions")); got != executionsBefore {
		t.Fatalf("TUI selection must not create report projection execution audits, before=%d after=%d", executionsBefore, got)
	}
	if _, err := os.Stat(execution.ReportPath); err != nil {
		t.Fatalf("TUI selection must not remove or rewrite report: %v", err)
	}
	completed, ok := updated.(tui.Model)
	if !ok {
		t.Fatalf("expected TUI model after result, got %T", updated)
	}
	resized, resizeCmd = completed.Update(tea.WindowSizeMsg{Width: 900, Height: 46})
	if resizeCmd != nil {
		t.Fatalf("expected resize after result to be synchronous")
	}
	for _, want := range []string{
		"execution status: completed",
		"action output: report projection execution selected",
		"report projection execution selection: status=current",
		"selected_by=tui",
		"current=true",
	} {
		if !strings.Contains(resized.View(), want) {
			t.Fatalf("expected report projection execution selection result view %q:\n%s", want, resized.View())
		}
	}
}

func TestNewTUIModelForWorkspaceClearsReportProjectionExecutionSelectionAfterDoubleConfirm(t *testing.T) {
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
		t.Fatalf("create report projection dispatch: %v", err)
	}
	execution, err := ExecuteReportProjectionDispatch(ctx, root, filepath.Base(dispatch.Path), ReportProjectionDispatchExecutionOptions{Execute: true})
	if err != nil {
		t.Fatalf("execute report projection dispatch: %v", err)
	}
	if _, err := AnnotateReportProjectionDispatchExecution(root, filepath.Base(execution.Path), ReportProjectionDispatchExecutionAnnotationOptions{Decision: "accepted"}); err != nil {
		t.Fatalf("accept report projection execution: %v", err)
	}
	if _, err := SelectReportProjectionDispatchExecution(root, filepath.Base(execution.Path), ReportProjectionDispatchExecutionSelectionOptions{
		SelectedBy: "alice",
		Reason:     "current report",
	}); err != nil {
		t.Fatalf("select report projection execution: %v", err)
	}
	executionsBefore := countJSON(t, filepath.Join(root, "reports", "report-projection-dispatch-executions"))

	model := NewTUIModelForWorkspace(root).SelectRuntimeTab(tui.RuntimeTabTimelineLanes)
	model = selectRuntimeRowContaining(t, model, "> report projection execution selection:")
	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if cmd != nil {
		t.Fatalf("first x should only arm report projection execution selection clear")
	}
	armed, ok := updated.(tui.Model)
	if !ok {
		t.Fatalf("expected TUI model after first x, got %T", updated)
	}
	selection, found, err := ReadReportProjectionDispatchExecutionSelection(root)
	if err != nil {
		t.Fatalf("read selection after first x: %v", err)
	}
	if !found || selection.Status != "current" {
		t.Fatalf("first x must not clear report projection selection, found=%v selection=%#v", found, selection)
	}
	resized, resizeCmd := armed.Update(tea.WindowSizeMsg{Width: 900, Height: 46})
	if resizeCmd != nil {
		t.Fatalf("expected resize after first x to be synchronous")
	}
	if !strings.Contains(resized.View(), "execution status: confirm-required") {
		t.Fatalf("expected confirmation before report projection selection clear:\n%s", resized.View())
	}

	updated, cmd = armed.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if cmd == nil {
		t.Fatalf("second x should clear report projection execution selection")
	}
	msg := cmd()
	updated, cmd = updated.Update(msg)
	if cmd != nil {
		t.Fatalf("expected report projection selection clear result to be synchronous")
	}
	selection, found, err = ReadReportProjectionDispatchExecutionSelection(root)
	if err != nil {
		t.Fatalf("read selection after second x: %v", err)
	}
	if !found || selection.Status != "cleared" || selection.ExecutionPath != execution.Path || selection.ClearedBy != "tui" || selection.Reason != "cleared from TUI" {
		t.Fatalf("expected TUI-cleared report projection execution selection, found=%v selection=%#v", found, selection)
	}
	if got := countJSON(t, filepath.Join(root, "reports", "report-projection-dispatch-executions")); got != executionsBefore {
		t.Fatalf("TUI selection clear must not create report projection execution audits, before=%d after=%d", executionsBefore, got)
	}
	completed, ok := updated.(tui.Model)
	if !ok {
		t.Fatalf("expected TUI model after result, got %T", updated)
	}
	resized, resizeCmd = completed.Update(tea.WindowSizeMsg{Width: 900, Height: 46})
	if resizeCmd != nil {
		t.Fatalf("expected resize after result to be synchronous")
	}
	for _, want := range []string{
		"execution status: completed",
		"action output: report projection selection cleared",
		"report projection execution selection: status=cleared",
		"cleared_by=tui",
		"current=false",
	} {
		if !strings.Contains(resized.View(), want) {
			t.Fatalf("expected report projection execution selection clear view %q:\n%s", want, resized.View())
		}
	}
}

func createWorkerLifecyclePolicySelectionFixture(t *testing.T) (string, WorkerLifecyclePolicy, WorkerLifecyclePolicy) {
	t.Helper()
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Date(2026, 6, 1, 17, 0, 0, 0, time.UTC)
	createWorkerLifecycleMaintenanceFixture(t, root, now)
	base, err := CreateWorkerLifecyclePolicy(root, WorkerLifecyclePolicyOptions{
		Now:                         now.Add(5 * time.Minute),
		StaleAfter:                  5 * time.Minute,
		WorkerStaleAfter:            2 * time.Minute,
		PollInterval:                500 * time.Millisecond,
		MaxSupervisorRuns:           2,
		TransportMode:               "metadata-only",
		IncludeLifecycleMaintenance: true,
		ExecuteLifecycleRetries:     true,
	})
	if err != nil {
		t.Fatalf("create base worker lifecycle policy: %v", err)
	}
	candidate, err := CreateWorkerLifecyclePolicy(root, WorkerLifecyclePolicyOptions{
		Now:                         now.Add(6 * time.Minute),
		StaleAfter:                  5 * time.Minute,
		WorkerStaleAfter:            2 * time.Minute,
		PollInterval:                250 * time.Millisecond,
		MaxSupervisorRuns:           3,
		TransportMode:               "metadata-only",
		IncludeLifecycleMaintenance: true,
		ExecuteLifecycleRetries:     true,
	})
	if err != nil {
		t.Fatalf("create candidate worker lifecycle policy: %v", err)
	}
	if _, err := SelectWorkerLifecyclePolicy(root, filepath.Base(base.Path), WorkerLifecyclePolicySelectionOptions{
		SelectedBy: "fixture",
		SelectedAt: now.Add(7 * time.Minute),
		Reason:     "fixture baseline",
	}); err != nil {
		t.Fatalf("select base worker lifecycle policy fixture: %v", err)
	}
	return root, base, candidate
}

func TestNewTUIModelForWorkspaceShowsWorkerLifecycleMaintenanceRows(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Now().UTC()
	createWorkerLifecycleMaintenanceFixture(t, root, now)
	maintenance, err := CreateWorkerLifecycleMaintenance(root, WorkerLifecycleMaintenanceOptions{
		Execute:    true,
		Now:        now.Add(5 * time.Minute),
		StaleAfter: 5 * time.Minute,
	})
	if err != nil {
		t.Fatalf("create worker lifecycle maintenance: %v", err)
	}

	model := NewTUIModelForWorkspace(root).SelectRuntimeTab(tui.RuntimeTabAgentRuntime)
	resized, cmd := model.Update(tea.WindowSizeMsg{Width: 320, Height: 66})
	if cmd != nil {
		t.Fatalf("expected resize update to be synchronous")
	}
	model = resized.(tui.Model)
	view := model.View()
	for _, want := range []string{
		"worker lifecycle maintenance artifacts: total=1",
		"worker lifecycle maintenance artifact:",
		"worker lifecycle maintenance artifact state: completed mode=metadata-only actions=5",
		"worker lifecycle maintenance counts: ack=1 run=1 daemon=1 reclaim=1 reconcile=1 skipped=3",
		"worker lifecycle maintenance created: review=1 action=1 reclaim=1 reconcile=1",
		"lifecycle maintenance skip: ack-transport-request worker-transport-ack",
		"lifecycle maintenance skip: run-transport-request worker-transport-run-once",
		"lifecycle maintenance skip: apply-daemon-control worker-transport-run-daemon-enable-control",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected worker lifecycle maintenance row %q in TUI view:\n%s", want, view)
		}
	}
	if maintenance.Path == "" || maintenance.ReviewPath == "" || maintenance.ActionPlanPath == "" {
		t.Fatalf("expected maintenance and child paths")
	}
}

func TestNewTUIModelForWorkspaceShowsWorkerLifecycleMaintenanceRetryDispatchSkipRows(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Now().UTC()
	marker := filepath.Join(t.TempDir(), "maintenance-retry-dispatch-tui-ran")
	createRuntimeWorkerDispatchRetryDispatchFixture(t, root, now, RuntimeWorkerToolPlanPayload{
		Action:     "discover_assets",
		Runner:     "command",
		Parser:     "web_api_parser",
		RunCommand: []string{"sh", "-c", "touch " + marker},
	})
	maintenance, err := CreateWorkerLifecycleMaintenance(root, WorkerLifecycleMaintenanceOptions{
		Execute: true,
		Now:     now.Add(5 * time.Minute),
	})
	if err != nil {
		t.Fatalf("create worker lifecycle maintenance: %v", err)
	}

	model := NewTUIModelForWorkspace(root).SelectRuntimeTab(tui.RuntimeTabAgentRuntime)
	resized, cmd := model.Update(tea.WindowSizeMsg{Width: 320, Height: 66})
	if cmd != nil {
		t.Fatalf("expected resize update to be synchronous")
	}
	model = resized.(tui.Model)
	view := model.View()
	for _, want := range []string{
		"worker lifecycle maintenance artifact state: completed mode=metadata-only actions=1",
		"worker lifecycle maintenance retry dispatch executions: ready=1",
		"retry dispatch maintenance skip: gate=worker-dispatch-retry-dispatch-execute",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected retry-dispatch maintenance row %q in TUI view:\n%s", want, view)
		}
	}
	if maintenance.Summary.RetryDispatchExecuteActions != 1 {
		t.Fatalf("expected retry-dispatch execute count on maintenance, got %#v", maintenance.Summary)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("maintenance TUI fixture must not execute retry tool-plan command, marker stat err=%v", err)
	}
}

func TestNewTUIModelForWorkspaceShowsWorkerLifecycleMaintenanceRetryDispatchExecutionRows(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Now().UTC()
	marker := filepath.Join(t.TempDir(), "maintenance-retry-dispatch-tui-explicit-ran")
	createRuntimeWorkerDispatchRetryDispatchFixture(t, root, now, RuntimeWorkerToolPlanPayload{
		Action:     "discover_assets",
		Runner:     "command",
		Parser:     "web_api_parser",
		RunCommand: []string{"sh", "-c", "touch " + marker},
	})
	maintenance, err := CreateWorkerLifecycleMaintenance(root, WorkerLifecycleMaintenanceOptions{
		Execute:                true,
		ExecuteRetryDispatches: true,
		Now:                    now.Add(5 * time.Minute),
	})
	if err != nil {
		t.Fatalf("create worker lifecycle maintenance with explicit retry dispatch execution: %v", err)
	}

	model := NewTUIModelForWorkspace(root).SelectRuntimeTab(tui.RuntimeTabAgentRuntime)
	resized, cmd := model.Update(tea.WindowSizeMsg{Width: 320, Height: 68})
	if cmd != nil {
		t.Fatalf("expected resize update to be synchronous")
	}
	model = resized.(tui.Model)
	view := model.View()
	for _, want := range []string{
		"worker lifecycle maintenance artifact state: completed mode=metadata-only actions=1",
		"worker lifecycle maintenance retry dispatch executed: executions=1 claims=1",
		"worker dispatch retry execution artifact:",
		"worker dispatch retry execution result",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected retry-dispatch maintenance execution row %q in TUI view:\n%s", want, view)
		}
	}
	if maintenance.Summary.RetryDispatchExecutionsCreated != 1 || maintenance.Summary.RetryDispatchClaimsCreated != 1 {
		t.Fatalf("expected retry-dispatch execution counts on maintenance, got %#v", maintenance.Summary)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("maintenance TUI fixture must not execute retry tool-plan command, marker stat err=%v", err)
	}
}

func TestNewTUIModelForWorkspaceShowsWorkerLifecycleMaintenanceRetryHandoffRows(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Now().UTC()
	marker := filepath.Join(t.TempDir(), "maintenance-retry-handoff-tui-ran")
	createRuntimeWorkerDispatchRetryDispatchFixture(t, root, now, RuntimeWorkerToolPlanPayload{
		Action:     "discover_assets",
		Runner:     "command",
		Parser:     "web_api_parser",
		RunCommand: []string{"sh", "-c", "touch " + marker},
	})
	maintenance, err := CreateWorkerLifecycleMaintenance(root, WorkerLifecycleMaintenanceOptions{
		Execute:                    true,
		ExecuteRetryDispatches:     true,
		DispatchRetryClaims:        true,
		RetryDispatchTransportMode: "file-outbox",
		Now:                        now.Add(5 * time.Minute),
	})
	if err != nil {
		t.Fatalf("create worker lifecycle maintenance with retry handoff: %v", err)
	}
	projectedRows := strings.Join(workerLifecycleMaintenanceRuntimeRows([]WorkerLifecycleMaintenance{maintenance}), "\n")
	for _, want := range []string{
		"worker lifecycle maintenance retry handoff: dispatch_lifecycle_retry_claims=true lifecycle_retry_transport=file-outbox claims_dispatched=1 transport_requests=1",
	} {
		if !strings.Contains(projectedRows, want) {
			t.Fatalf("expected projected retry handoff maintenance row %q, got:\n%s", want, projectedRows)
		}
	}

	model := NewTUIModelForWorkspace(root).SelectRuntimeTab(tui.RuntimeTabAgentRuntime)
	resized, cmd := model.Update(tea.WindowSizeMsg{Width: 320, Height: 68})
	if cmd != nil {
		t.Fatalf("expected resize update to be synchronous")
	}
	model = resized.(tui.Model)
	view := model.View()
	for _, want := range []string{
		"worker lifecycle maintenance retry dispatch executed: executions=1 claims=1",
		"worker lifecycle maintenance retry handoff: dispatch_lifecycle_retry_claims=true",
		"worker dispatch retry execution result",
		"transport=file-outbox",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected retry handoff maintenance row %q in TUI view:\n%s", want, view)
		}
	}
	if maintenance.Summary.RetryDispatchClaimsDispatched != 1 || maintenance.Summary.RetryDispatchTransportRequestsCreated != 1 {
		t.Fatalf("expected retry handoff counts on maintenance, got %#v", maintenance.Summary)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("maintenance TUI handoff fixture must not execute retry tool-plan command, marker stat err=%v", err)
	}
}

func TestNewTUIModelForWorkspaceShowsWorkerLifecycleMaintenanceSupervisorRows(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Now().UTC()
	createWorkerLifecycleMaintenanceFixture(t, root, now)
	supervisor, err := CreateWorkerLifecycleMaintenanceSupervisor(root, WorkerLifecycleMaintenanceSupervisorOptions{
		Execute:    true,
		Now:        now.Add(5 * time.Minute),
		StaleAfter: 5 * time.Minute,
		MaxRuns:    2,
	})
	if err != nil {
		t.Fatalf("create worker lifecycle maintenance supervisor: %v", err)
	}

	model := NewTUIModelForWorkspace(root).SelectRuntimeTab(tui.RuntimeTabAgentRuntime)
	resized, cmd := model.Update(tea.WindowSizeMsg{Width: 320, Height: 72})
	if cmd != nil {
		t.Fatalf("expected resize update to be synchronous")
	}
	model = resized.(tui.Model)
	view := model.View()
	for _, want := range []string{
		"worker lifecycle maintenance supervisors: total=1 completed=1 dry-run=0 failed=0",
		"worker lifecycle maintenance supervisor artifact:",
		"worker lifecycle maintenance supervisor state: completed mode=metadata-only runs=2 max_runs=2",
		"worker lifecycle maintenance supervisor created: maintenance=2 audit=2",
		"worker lifecycle maintenance supervisor path:",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected worker lifecycle maintenance supervisor row %q in TUI view:\n%s", want, view)
		}
	}
	if supervisor.Path == "" || len(supervisor.MaintenancePaths) != 2 || len(supervisor.AuditPaths) != 2 {
		t.Fatalf("expected supervisor paths, got %#v", supervisor)
	}
}

func TestNewTUIModelForWorkspaceShowsWorkerLifecycleMaintenanceSupervisorRetryDispatchCounts(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Now().UTC()
	marker := filepath.Join(t.TempDir(), "maintenance-supervisor-retry-dispatch-tui-ran")
	createRuntimeWorkerDispatchRetryDispatchFixture(t, root, now, RuntimeWorkerToolPlanPayload{
		Action:     "discover_assets",
		Runner:     "command",
		Parser:     "web_api_parser",
		RunCommand: []string{"sh", "-c", "touch " + marker},
	})
	supervisor, err := CreateWorkerLifecycleMaintenanceSupervisor(root, WorkerLifecycleMaintenanceSupervisorOptions{
		Execute: true,
		Now:     now.Add(5 * time.Minute),
		MaxRuns: 2,
	})
	if err != nil {
		t.Fatalf("create worker lifecycle maintenance supervisor: %v", err)
	}

	model := NewTUIModelForWorkspace(root).SelectRuntimeTab(tui.RuntimeTabAgentRuntime)
	resized, cmd := model.Update(tea.WindowSizeMsg{Width: 320, Height: 72})
	if cmd != nil {
		t.Fatalf("expected resize update to be synchronous")
	}
	model = resized.(tui.Model)
	view := model.View()
	for _, want := range []string{
		"maintenance supervisor retry dispatch: execute=2 skipped=2",
		"worker lifecycle maintenance supervisor artifact:",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected supervisor retry-dispatch row %q in TUI view:\n%s", want, view)
		}
	}
	if supervisor.Summary.RetryDispatchExecuteActions != 2 {
		t.Fatalf("expected supervisor retry-dispatch count, got %#v", supervisor.Summary)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("supervisor TUI fixture must not execute retry tool-plan command, marker stat err=%v", err)
	}
}

func TestNewTUIModelForWorkspaceShowsWorkerLifecycleMaintenanceSupervisorRetryDispatchExecutionRows(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Now().UTC()
	marker := filepath.Join(t.TempDir(), "maintenance-supervisor-retry-dispatch-tui-explicit-ran")
	createRuntimeWorkerDispatchRetryDispatchFixture(t, root, now, RuntimeWorkerToolPlanPayload{
		Action:     "discover_assets",
		Runner:     "command",
		Parser:     "web_api_parser",
		RunCommand: []string{"sh", "-c", "touch " + marker},
	})
	supervisor, err := CreateWorkerLifecycleMaintenanceSupervisor(root, WorkerLifecycleMaintenanceSupervisorOptions{
		Execute:                true,
		ExecuteRetryDispatches: true,
		Now:                    now.Add(5 * time.Minute),
		MaxRuns:                2,
	})
	if err != nil {
		t.Fatalf("create worker lifecycle maintenance supervisor: %v", err)
	}

	model := NewTUIModelForWorkspace(root).SelectRuntimeTab(tui.RuntimeTabAgentRuntime)
	resized, cmd := model.Update(tea.WindowSizeMsg{Width: 320, Height: 72})
	if cmd != nil {
		t.Fatalf("expected resize update to be synchronous")
	}
	model = resized.(tui.Model)
	view := model.View()
	for _, want := range []string{
		"maintenance supervisor retry dispatch executed: executions=1 claims=1",
		"worker lifecycle maintenance supervisor retry-dispatch execution:",
		"worker dispatch retry execution artifact:",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected supervisor retry-dispatch execution row %q in TUI view:\n%s", want, view)
		}
	}
	if supervisor.Summary.RetryDispatchExecutionsCreated != 1 || supervisor.Summary.RetryDispatchClaimsCreated != 1 {
		t.Fatalf("expected supervisor retry-dispatch execution counts, got %#v", supervisor.Summary)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("supervisor TUI fixture must not execute retry tool-plan command, marker stat err=%v", err)
	}
}

func TestNewTUIModelForWorkspaceShowsWorkerLifecycleAuditRows(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Now().UTC()
	dispatch := createFileOutboxRuntimeWorkerQueueDispatch(t, root, now)
	if _, err := SubmitRuntimeWorkerQueueDispatchResult(root, filepath.Base(dispatch.Path), RuntimeWorkerQueueDispatchResult{
		Status: "failed",
		Error:  "remote result failed",
	}, RuntimeWorkerQueueDispatchResultOptions{Now: now.Add(time.Minute)}); err != nil {
		t.Fatalf("submit worker queue dispatch result: %v", err)
	}
	retryPlan, err := CreateRuntimeWorkerDispatchRetryPlan(root, now.Add(90*time.Second))
	if err != nil {
		t.Fatalf("create worker dispatch retry plan: %v", err)
	}
	retryDispatch, err := CreateRuntimeWorkerDispatchRetryDispatch(root, filepath.Base(retryPlan.Path), now.Add(100*time.Second))
	if err != nil {
		t.Fatalf("create worker dispatch retry dispatch: %v", err)
	}
	review, err := CreateWorkerLifecycleReview(root, now.Add(2*time.Minute), defaultWorkerLeaseStaleAfter)
	if err != nil {
		t.Fatalf("create worker lifecycle review: %v", err)
	}
	if _, err := CreateWorkerLifecycleActionPlan(root, filepath.Base(review.Path), now.Add(3*time.Minute), defaultWorkerLeaseStaleAfter); err != nil {
		t.Fatalf("create worker lifecycle action plan: %v", err)
	}
	maintenance, err := CreateWorkerLifecycleMaintenance(root, WorkerLifecycleMaintenanceOptions{
		Now:        now.Add(3*time.Minute + 30*time.Second),
		StaleAfter: defaultWorkerLeaseStaleAfter,
	})
	if err != nil {
		t.Fatalf("create worker lifecycle maintenance: %v", err)
	}
	supervisor, err := CreateWorkerLifecycleMaintenanceSupervisor(root, WorkerLifecycleMaintenanceSupervisorOptions{
		Now:        now.Add(3*time.Minute + 45*time.Second),
		StaleAfter: defaultWorkerLeaseStaleAfter,
		MaxRuns:    2,
	})
	if err != nil {
		t.Fatalf("create worker lifecycle maintenance supervisor: %v", err)
	}
	orchestrationSupervisor, err := CreateRuntimeWorkerOrchestrationSupervisor(root, RuntimeWorkerOrchestrationSupervisorOptions{
		Now:           now.Add(3*time.Minute + 50*time.Second),
		RunIDPrefix:   "run-audit-tui-orchestration-supervisor",
		TransportMode: "metadata-only",
		MaxRuns:       2,
	})
	if err != nil {
		t.Fatalf("create worker orchestration supervisor: %v", err)
	}
	policy, err := CreateWorkerLifecyclePolicy(root, WorkerLifecyclePolicyOptions{
		Now:                         now.Add(3*time.Minute + 55*time.Second),
		StaleAfter:                  defaultWorkerLeaseStaleAfter,
		WorkerStaleAfter:            DefaultRuntimeWorkerStaleAfter,
		PollInterval:                time.Second,
		MaxSupervisorRuns:           1,
		TransportMode:               "metadata-only",
		IncludeLifecycleMaintenance: true,
	})
	if err != nil {
		t.Fatalf("create worker lifecycle policy: %v", err)
	}
	if _, err := SelectWorkerLifecyclePolicy(root, filepath.Base(policy.Path), WorkerLifecyclePolicySelectionOptions{
		SelectedBy: "tui-audit",
		SelectedAt: now.Add(3*time.Minute + 56*time.Second),
		Reason:     "tui audit policy visibility",
	}); err != nil {
		t.Fatalf("select worker lifecycle policy: %v", err)
	}
	if _, err := CreateWorkerLifecyclePolicyRun(root, WorkerLifecyclePolicyRunOptions{
		Now:   now.Add(3*time.Minute + 57*time.Second),
		Actor: "tui-audit",
	}); err != nil {
		t.Fatalf("create worker lifecycle policy run: %v", err)
	}
	if _, err := CreateWorkerLifecyclePolicySchedulerRun(root, WorkerLifecyclePolicySchedulerRunOptions{
		Now:     now.Add(3*time.Minute + 58*time.Second),
		Actor:   "tui-audit",
		MaxRuns: 1,
	}); err != nil {
		t.Fatalf("create worker lifecycle policy scheduler run: %v", err)
	}
	audit, err := CreateWorkerLifecycleAudit(root, now.Add(4*time.Minute), defaultWorkerLeaseStaleAfter)
	if err != nil {
		t.Fatalf("create worker lifecycle audit: %v", err)
	}

	model := NewTUIModelForWorkspace(root).SelectRuntimeTab(tui.RuntimeTabAgentRuntime)
	resized, cmd := model.Update(tea.WindowSizeMsg{Width: 300, Height: 220})
	if cmd != nil {
		t.Fatalf("expected resize update to be synchronous")
	}
	model = resized.(tui.Model)
	view := model.View()
	for _, want := range []string{
		"worker lifecycle audit artifacts: total=1",
		"worker lifecycle audit artifact status=dry-run mode=metadata-only",
		"worker lifecycle audit artifact:",
		"worker lifecycle audit artifact state: dry-run mode=metadata-only workers=1",
		"worker lifecycle audit placement: plans=0 claims=0 dispatch_plans=0 batches=0",
		"worker lifecycle audit orchestration: orchestrations=0",
		"worker lifecycle audit queue: claims=1 dispatches=1",
		"worker lifecycle audit transport: requests=1 results=1",
		"worker lifecycle audit retry plans: total=1",
		"worker lifecycle audit retry dispatches: total=1",
		"worker lifecycle audit retry plan: total=1 ready=1",
		"worker lifecycle audit retry dispatch: total=1 ready=1",
		"worker lifecycle audit policies: total=1 selections=1 policy_runs=1 scheduler_runs=1",
		"worker lifecycle audit orchestration supervisors: total=1",
		"worker lifecycle audit maintenance supervisors: total=1",
		"worker lifecycle audit artifact path:",
		"retry_plans=1",
		"retry_dispatches=1",
		"lifecycle_policies=1",
		"policy_runs=1",
		"policy_scheduler_runs=1",
		"worker lifecycle audit item: kind=worker-dispatch-retry-plan",
		"worker lifecycle audit item: kind=worker-dispatch-retry-dispatch",
		"worker lifecycle audit item: kind=worker-lifecycle-policy",
		"worker lifecycle audit item: kind=worker-lifecycle-policy-selection",
		"worker lifecycle audit item: kind=worker-lifecycle-policy-run",
		"worker lifecycle audit item: kind=worker-lifecycle-policy-scheduler-run",
		"worker lifecycle audit item: kind=worker-lifecycle-maintenance",
		"worker lifecycle audit item: kind=worker-lifecycle-maintenance-supervisor",
		"worker lifecycle audit item: kind=worker-orchestration-supervisor",
		"worker lifecycle audit item: kind=queue-dispatch-result",
		"worker lifecycle audit item: kind=transport-request",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected worker lifecycle audit row %q in TUI view:\n%s", want, view)
		}
	}
	if audit.Path == "" {
		t.Fatalf("expected worker lifecycle audit path")
	}
	if maintenance.Path == "" {
		t.Fatalf("expected worker lifecycle maintenance path")
	}
	if retryPlan.Path == "" {
		t.Fatalf("expected worker dispatch retry plan path")
	}
	if retryDispatch.Path == "" {
		t.Fatalf("expected worker dispatch retry dispatch path")
	}
	if supervisor.Path == "" {
		t.Fatalf("expected worker lifecycle maintenance supervisor path")
	}
	if orchestrationSupervisor.Path == "" {
		t.Fatalf("expected worker orchestration supervisor path")
	}
}

func TestNewTUIModelForWorkspaceShowsWorkerLifecycleAuditWorkerDispatchRetryAggregateItemFields(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Date(2026, 5, 31, 17, 25, 0, 0, time.UTC)
	_, retryDispatch, retryPlan := createRuntimeWorkerDispatchRetryDispatchFixture(t, root, now)
	retryExecution, err := ExecuteRuntimeWorkerDispatchRetryDispatch(root, filepath.Base(retryDispatch.Path), RuntimeWorkerDispatchRetryDispatchExecutionOptions{
		Execute: true,
		Now:     now.Add(4 * time.Minute),
	})
	if err != nil {
		t.Fatalf("execute worker dispatch retry dispatch: %v", err)
	}
	if retryPlan.Summary.Ready != 1 || retryDispatch.Summary.Ready != 1 || retryExecution.Summary.Claimed != 1 {
		t.Fatalf("expected populated retry aggregate fixtures, got plan=%#v dispatch=%#v execution=%#v", retryPlan.Summary, retryDispatch.Summary, retryExecution.Summary)
	}
	if _, err := CreateWorkerLifecycleAudit(root, now.Add(5*time.Minute), defaultWorkerLeaseStaleAfter); err != nil {
		t.Fatalf("create worker lifecycle audit: %v", err)
	}

	model := NewTUIModelForWorkspace(root).SelectRuntimeTab(tui.RuntimeTabAgentRuntime)
	resized, cmd := model.Update(tea.WindowSizeMsg{Width: 320, Height: 180})
	if cmd != nil {
		t.Fatalf("expected resize update to be synchronous")
	}
	view := resized.(tui.Model).View()
	for _, want := range []string{
		"worker lifecycle audit item: kind=worker-dispatch-retry-plan",
		"worker lifecycle audit retry plan: total=1 ready=1",
		"worker lifecycle audit item: kind=worker-dispatch-retry-dispatch",
		"worker lifecycle audit retry dispatch: total=1 ready=1",
		"worker lifecycle audit item: kind=worker-dispatch-retry-dispatch-execution",
		"worker lifecycle audit retry dispatch execution: total=1 ready=1 claimed=1",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected worker dispatch retry audit aggregate row %q in TUI view:\n%s", want, view)
		}
	}
}

func TestNewTUIModelForWorkspaceShowsWorkerLifecycleAuditLeaseRecoveryAggregateItemFields(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Date(2026, 5, 31, 17, 50, 0, 0, time.UTC)
	staleAfter := 5 * time.Minute
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	if _, err := RegisterRuntimeWorker(root, RuntimeWorkerArtifact{
		WorkerID:     "remote-web-1",
		Mode:         "remote",
		Status:       "available",
		Endpoint:     "file://remote-web-1/inbox",
		Capabilities: []string{"web.discovery"},
	}, RuntimeWorkerRegistrationOptions{Now: now}); err != nil {
		t.Fatalf("register runtime worker: %v", err)
	}
	oldHeartbeat := now.Add(-staleAfter - time.Minute).Format(time.RFC3339Nano)
	freshHeartbeat := now.Format(time.RFC3339Nano)
	for _, lease := range []WorkerLeaseArtifact{
		{
			Version:     1,
			RunID:       "run-tui-reclaim-ready",
			ExecutionID: "tool-tui-reclaim-ready",
			Tool:        "web-api-pack",
			Capability:  "web.discovery",
			WorkerID:    "remote-web-1",
			Status:      "active",
			AcquiredAt:  oldHeartbeat,
			HeartbeatAt: oldHeartbeat,
		},
		{
			Version:     1,
			RunID:       "run-tui-reclaim-skip",
			ExecutionID: "tool-tui-reclaim-skip",
			Tool:        "web-api-pack",
			Capability:  "web.discovery",
			WorkerID:    "remote-web-1",
			Status:      "active",
			AcquiredAt:  freshHeartbeat,
			HeartbeatAt: freshHeartbeat,
		},
	} {
		if err := writeWorkerLeaseArtifact(root, lease); err != nil {
			t.Fatalf("write reclaim lease fixture: %v", err)
		}
	}
	reclaimReview, err := CreateWorkerLeaseReclaimDispatch(root, now, staleAfter)
	if err != nil {
		t.Fatalf("create reclaim review: %v", err)
	}
	if _, err := ExecuteWorkerLeaseReclaimDispatch(root, filepath.Base(reclaimReview.Path), WorkerLeaseReclaimExecutionOptions{
		Execute:    true,
		Now:        now.Add(time.Minute),
		StaleAfter: staleAfter,
	}); err != nil {
		t.Fatalf("execute reclaim review: %v", err)
	}
	readyDispatch := createFileOutboxRuntimeWorkerQueueDispatchForExecution(t, root, now.Add(2*time.Minute), "run-tui-reconcile-ready", "tool-tui-reconcile-ready")
	if _, err := SubmitRuntimeWorkerQueueDispatchResult(root, filepath.Base(readyDispatch.Path), RuntimeWorkerQueueDispatchResult{
		Status: "completed",
		Output: "ready result",
	}, RuntimeWorkerQueueDispatchResultOptions{Now: now.Add(3 * time.Minute)}); err != nil {
		t.Fatalf("submit ready reconciliation result: %v", err)
	}
	if err := writeWorkerLeaseArtifact(root, WorkerLeaseArtifact{
		Version:     1,
		RunID:       readyDispatch.RunID,
		ExecutionID: readyDispatch.ExecutionID,
		Tool:        readyDispatch.Tool,
		Capability:  readyDispatch.Capability,
		WorkerID:    readyDispatch.WorkerID,
		Status:      "active",
		AcquiredAt:  now.Add(2 * time.Minute).Format(time.RFC3339Nano),
		HeartbeatAt: now.Add(2 * time.Minute).Format(time.RFC3339Nano),
	}); err != nil {
		t.Fatalf("write active reconciliation lease fixture: %v", err)
	}
	skippedDispatch := createFileOutboxRuntimeWorkerQueueDispatchForExecution(t, root, now.Add(4*time.Minute), "run-tui-reconcile-skip", "tool-tui-reconcile-skip")
	if _, err := SubmitRuntimeWorkerQueueDispatchResult(root, filepath.Base(skippedDispatch.Path), RuntimeWorkerQueueDispatchResult{
		Status: "completed",
		Output: "aligned result",
	}, RuntimeWorkerQueueDispatchResultOptions{Now: now.Add(5 * time.Minute)}); err != nil {
		t.Fatalf("submit skipped reconciliation result: %v", err)
	}
	if err := writeWorkerLeaseArtifact(root, WorkerLeaseArtifact{
		Version:           1,
		RunID:             skippedDispatch.RunID,
		ExecutionID:       skippedDispatch.ExecutionID,
		ResultExecutionID: "aligned-result",
		Tool:              skippedDispatch.Tool,
		Capability:        skippedDispatch.Capability,
		WorkerID:          skippedDispatch.WorkerID,
		Status:            "completed",
		AcquiredAt:        now.Add(4 * time.Minute).Format(time.RFC3339Nano),
		HeartbeatAt:       now.Add(5 * time.Minute).Format(time.RFC3339Nano),
		ReleasedAt:        now.Add(5 * time.Minute).Format(time.RFC3339Nano),
	}); err != nil {
		t.Fatalf("write aligned reconciliation lease fixture: %v", err)
	}
	reconciliationReview, err := CreateWorkerLeaseResultReconciliationDispatch(root, now.Add(6*time.Minute))
	if err != nil {
		t.Fatalf("create reconciliation review: %v", err)
	}
	if _, err := ExecuteWorkerLeaseResultReconciliationDispatch(root, filepath.Base(reconciliationReview.Path), WorkerLeaseResultReconciliationExecutionOptions{
		Execute: true,
		Now:     now.Add(7 * time.Minute),
	}); err != nil {
		t.Fatalf("execute reconciliation review: %v", err)
	}
	if _, err := CreateWorkerLifecycleAudit(root, now.Add(8*time.Minute), staleAfter); err != nil {
		t.Fatalf("create worker lifecycle audit: %v", err)
	}

	model := NewTUIModelForWorkspace(root).SelectRuntimeTab(tui.RuntimeTabAgentRuntime)
	resized, cmd := model.Update(tea.WindowSizeMsg{Width: 340, Height: 220})
	if cmd != nil {
		t.Fatalf("expected resize update to be synchronous")
	}
	view := resized.(tui.Model).View()
	for _, want := range []string{
		"worker lifecycle audit item: kind=worker-lease-reclaim-review",
		"worker lifecycle audit reclaim review: total=2 ready=1 skipped=1",
		"worker lifecycle audit item: kind=worker-lease-reclaim-execution",
		"worker lifecycle audit reclaim execution: total=2 results=1 skipped=1",
		"worker lifecycle audit item: kind=worker-lease-result-reconciliation-review",
		"worker lifecycle audit reconciliation review: total=2 ready=1 skipped=1",
		"worker lifecycle audit item: kind=worker-lease-result-reconciliation-execution",
		"worker lifecycle audit reconciliation execution: total=2 results=1 skipped=1",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected worker lease recovery audit aggregate row %q in TUI view:\n%s", want, view)
		}
	}
}

func TestNewTUIModelForWorkspaceShowsWorkerLifecycleAuditSupervisorAggregateItemFields(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Date(2026, 5, 31, 16, 5, 0, 0, time.UTC)
	marker := filepath.Join(t.TempDir(), "audit-supervisor-retry-dispatch-tui-ran")
	_, _, _ = createRuntimeWorkerDispatchRetryDispatchFixture(t, root, now, RuntimeWorkerToolPlanPayload{
		Action:     "discover_assets",
		Runner:     "command",
		Parser:     "web_api_parser",
		RunCommand: []string{"sh", "-c", "touch " + marker},
	})
	if _, err := CreateWorkerLifecycleMaintenanceSupervisor(root, WorkerLifecycleMaintenanceSupervisorOptions{
		Execute:    true,
		Now:        now.Add(4 * time.Minute),
		StaleAfter: defaultWorkerLeaseStaleAfter,
		MaxRuns:    2,
	}); err != nil {
		t.Fatalf("create worker lifecycle maintenance supervisor: %v", err)
	}
	if err := os.RemoveAll(filepath.Join(root, "runtime", "worker-lifecycle-audits")); err != nil {
		t.Fatalf("clear child lifecycle audit fixtures: %v", err)
	}
	if _, err := CreateWorkerLifecycleAudit(root, now.Add(5*time.Minute), defaultWorkerLeaseStaleAfter); err != nil {
		t.Fatalf("create worker lifecycle audit: %v", err)
	}

	model := NewTUIModelForWorkspace(root).SelectRuntimeTab(tui.RuntimeTabAgentRuntime)
	resized, cmd := model.Update(tea.WindowSizeMsg{Width: 300, Height: 180})
	if cmd != nil {
		t.Fatalf("expected resize update to be synchronous")
	}
	view := resized.(tui.Model).View()
	for _, want := range []string{
		"worker lifecycle audit item: kind=worker-lifecycle-maintenance-supervisor",
		"actions=2",
		"skipped_unsafe=2",
		"retry_dispatch_execute=2",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected worker lifecycle audit supervisor aggregate row %q in TUI view:\n%s", want, view)
		}
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("audit TUI fixture must not execute retry tool-plan command, marker stat err=%v", err)
	}
}

func TestNewTUIModelForWorkspaceShowsWorkerLifecycleAuditMaintenanceAggregateItemFields(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Date(2026, 5, 31, 16, 30, 0, 0, time.UTC)
	marker := filepath.Join(t.TempDir(), "audit-maintenance-retry-dispatch-tui-ran")
	_, _, _ = createRuntimeWorkerDispatchRetryDispatchFixture(t, root, now, RuntimeWorkerToolPlanPayload{
		Action:     "discover_assets",
		Runner:     "command",
		Parser:     "web_api_parser",
		RunCommand: []string{"sh", "-c", "touch " + marker},
	})
	if _, err := CreateWorkerLifecycleMaintenance(root, WorkerLifecycleMaintenanceOptions{
		Execute:    true,
		Now:        now.Add(4 * time.Minute),
		StaleAfter: defaultWorkerLeaseStaleAfter,
	}); err != nil {
		t.Fatalf("create worker lifecycle maintenance: %v", err)
	}
	if _, err := CreateWorkerLifecycleAudit(root, now.Add(5*time.Minute), defaultWorkerLeaseStaleAfter); err != nil {
		t.Fatalf("create worker lifecycle audit: %v", err)
	}

	model := NewTUIModelForWorkspace(root).SelectRuntimeTab(tui.RuntimeTabAgentRuntime)
	resized, cmd := model.Update(tea.WindowSizeMsg{Width: 300, Height: 180})
	if cmd != nil {
		t.Fatalf("expected resize update to be synchronous")
	}
	view := resized.(tui.Model).View()
	for _, want := range []string{
		"worker lifecycle audit item: kind=worker-lifecycle-maintenance",
		"worker lifecycle audit maintenance actions=1",
		"worker lifecycle audit maintenance skipped_unsafe=1",
		"worker lifecycle audit maintenance retry_dispatch_execute=1",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected worker lifecycle audit maintenance aggregate row %q in TUI view:\n%s", want, view)
		}
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("audit TUI fixture must not execute retry tool-plan command, marker stat err=%v", err)
	}
}

func TestNewTUIModelForWorkspaceShowsWorkerLifecycleAuditActionPlanAggregateItemFields(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Date(2026, 5, 31, 16, 55, 0, 0, time.UTC)
	marker := filepath.Join(t.TempDir(), "audit-action-plan-retry-dispatch-tui-ran")
	_, _, _ = createRuntimeWorkerDispatchRetryDispatchFixture(t, root, now, RuntimeWorkerToolPlanPayload{
		Action:     "discover_assets",
		Runner:     "command",
		Parser:     "web_api_parser",
		RunCommand: []string{"sh", "-c", "touch " + marker},
	})
	actionPlan, err := CreateWorkerLifecycleActionPlan(root, "", now.Add(4*time.Minute), defaultWorkerLeaseStaleAfter)
	if err != nil {
		t.Fatalf("create worker lifecycle action plan: %v", err)
	}
	if actionPlan.Summary.Total == 0 || actionPlan.Summary.ExecuteDispatchRetryDispatch != 1 {
		t.Fatalf("expected retry-dispatch action-plan fixture, got %#v", actionPlan.Summary)
	}
	reviewedAt := now.Add(5 * time.Minute)
	if _, err := AnnotateWorkerLifecycleActionPlan(root, filepath.Base(actionPlan.Path), WorkerLifecycleActionPlanAnnotationOptions{
		Decision:   "accepted",
		Reviewer:   "tui-auditor",
		ReviewedAt: reviewedAt,
		Reason:     "authorized_retry_dispatch_review",
	}); err != nil {
		t.Fatalf("annotate worker lifecycle action plan: %v", err)
	}
	selectedAt := now.Add(6 * time.Minute)
	if _, err := SelectWorkerLifecycleActionPlan(root, filepath.Base(actionPlan.Path), WorkerLifecycleActionPlanSelectionOptions{
		SelectedBy: "tui-operator",
		SelectedAt: selectedAt,
		Reason:     "current_reviewed_action_plan",
	}); err != nil {
		t.Fatalf("select worker lifecycle action plan: %v", err)
	}
	if _, err := CreateWorkerLifecycleAudit(root, now.Add(7*time.Minute), defaultWorkerLeaseStaleAfter); err != nil {
		t.Fatalf("create worker lifecycle audit: %v", err)
	}

	model := NewTUIModelForWorkspace(root).SelectRuntimeTab(tui.RuntimeTabAgentRuntime)
	resized, cmd := model.Update(tea.WindowSizeMsg{Width: 300, Height: 180})
	if cmd != nil {
		t.Fatalf("expected resize update to be synchronous")
	}
	view := resized.(tui.Model).View()
	for _, want := range []string{
		"worker lifecycle audit artifact",
		"action_plan_annotations=1",
		"action_plan_selections=1",
		"worker lifecycle audit item: kind=worker-lifecycle-action-plan",
		"decision=accepted",
		"reviewer=tui-auditor",
		"reviewed_at=" + reviewedAt.Format(time.RFC3339),
		"reason=authorized_retry_dispatch_review",
		"worker lifecycle audit item: kind=worker-lifecycle-action-plan-selection",
		"audit action-plan marker selected_by=tui-operator",
		"audit action-plan marker linked=worker-lifecycle-action-plan",
		"selected_at=" + selectedAt.Format(time.RFC3339),
		"worker lifecycle audit action plan actions=",
		"worker lifecycle audit action plan review: decision=accepted reviewer=tui-auditor",
		"audit action-plan review reviewed_at=" + reviewedAt.Format(time.RFC3339),
		"worker lifecycle audit action plan retry_dispatch_execute=1",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected worker lifecycle audit action-plan aggregate row %q in TUI view:\n%s", want, view)
		}
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("audit TUI fixture must not execute retry tool-plan command, marker stat err=%v", err)
	}
}

func TestNewTUIModelForWorkspaceShowsWorkerLifecycleAuditWorkerOrchestrationSupervisorAggregateItemFields(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Date(2026, 5, 31, 17, 5, 0, 0, time.UTC)
	registerWorkerLifecycleAuditOrchestrationWorkers(t, root, now)
	supervisor, err := CreateRuntimeWorkerOrchestrationSupervisor(root, RuntimeWorkerOrchestrationSupervisorOptions{
		Execute:                     true,
		Now:                         now.Add(time.Minute),
		RunIDPrefix:                 "run-worker-orchestration-supervisor-audit-tui",
		TransportMode:               "metadata-only",
		MaxCandidates:               1,
		MaxRuns:                     2,
		LeaseDuration:               10 * time.Minute,
		IncludeLifecycleMaintenance: true,
	})
	if err != nil {
		t.Fatalf("create worker orchestration supervisor: %v", err)
	}
	if supervisor.Summary.Runs != 2 || supervisor.Summary.OrchestrationArtifactsCreated != 2 || supervisor.Summary.LifecycleMaintenanceArtifactsCreated != 2 || supervisor.Summary.LifecycleAuditArtifactsCreated != 2 {
		t.Fatalf("expected populated orchestration supervisor fixture, got %#v", supervisor.Summary)
	}
	if err := os.RemoveAll(filepath.Join(root, "runtime", "worker-lifecycle-audits")); err != nil {
		t.Fatalf("clear child lifecycle audit fixtures: %v", err)
	}
	if _, err := CreateWorkerLifecycleAudit(root, now.Add(3*time.Minute), defaultWorkerLeaseStaleAfter); err != nil {
		t.Fatalf("create worker lifecycle audit: %v", err)
	}

	model := NewTUIModelForWorkspace(root).SelectRuntimeTab(tui.RuntimeTabAgentRuntime)
	resized, cmd := model.Update(tea.WindowSizeMsg{Width: 320, Height: 220})
	if cmd != nil {
		t.Fatalf("expected resize update to be synchronous")
	}
	view := resized.(tui.Model).View()
	for _, want := range []string{
		"worker lifecycle audit item: kind=worker-orchestration-supervisor",
		"worker lifecycle audit orchestration supervisor runs=2 max_runs=2",
		"worker lifecycle audit orchestration supervisor created: orchestrations=2",
		"worker lifecycle audit orchestration supervisor lifecycle created: maintenance=2 audit=2",
		"worker lifecycle audit orchestration supervisor dispatch: claimed=2 dispatched=2",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected worker lifecycle audit orchestration supervisor aggregate row %q in TUI view:\n%s", want, view)
		}
	}
}

func TestNewTUIModelForWorkspaceShowsWorkerLifecycleAuditWorkerOrchestrationAggregateItemFields(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Date(2026, 5, 31, 17, 15, 0, 0, time.UTC)
	registerWorkerLifecycleAuditOrchestrationWorkers(t, root, now)
	orchestration, err := CreateRuntimeWorkerOrchestration(root, RuntimeWorkerOrchestrationOptions{
		Execute:                     true,
		Now:                         now.Add(time.Minute),
		RunID:                       "run-worker-orchestration-audit-tui",
		TransportMode:               "metadata-only",
		MaxCandidates:               1,
		LeaseDuration:               10 * time.Minute,
		IncludeLifecycleMaintenance: true,
	})
	if err != nil {
		t.Fatalf("create worker orchestration: %v", err)
	}
	if orchestration.PlacementReady == 0 || orchestration.ClaimedCount != 1 || orchestration.DispatchedCount != 1 || orchestration.LifecycleMaintenancePath == "" || orchestration.LifecycleAuditPath == "" {
		t.Fatalf("expected populated orchestration fixture, got %#v", orchestration)
	}
	if err := os.RemoveAll(filepath.Join(root, "runtime", "worker-lifecycle-audits")); err != nil {
		t.Fatalf("clear child lifecycle audit fixtures: %v", err)
	}
	if _, err := CreateWorkerLifecycleAudit(root, now.Add(3*time.Minute), defaultWorkerLeaseStaleAfter); err != nil {
		t.Fatalf("create worker lifecycle audit: %v", err)
	}

	model := NewTUIModelForWorkspace(root).SelectRuntimeTab(tui.RuntimeTabAgentRuntime)
	resized, cmd := model.Update(tea.WindowSizeMsg{Width: 320, Height: 220})
	if cmd != nil {
		t.Fatalf("expected resize update to be synchronous")
	}
	view := resized.(tui.Model).View()
	for _, want := range []string{
		"worker lifecycle audit item: kind=worker-orchestration",
		"worker lifecycle audit orchestration placement: ready=",
		"worker lifecycle audit orchestration dispatch ready=",
		"worker lifecycle audit orchestration lifecycle created: maintenance=1 audit=1",
		"worker lifecycle audit orchestration dispatch: claimed=1 dispatched=1",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected worker lifecycle audit orchestration aggregate row %q in TUI view:\n%s", want, view)
		}
	}
}

func TestNewTUIModelForWorkspaceShowsWorkerLeaseResultReconciliationExecutionArtifacts(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Date(2026, 5, 30, 19, 35, 0, 0, time.UTC)
	dispatch := createDispatchedRuntimeWorkerQueueClaim(t, root, now)
	if _, err := SubmitRuntimeWorkerQueueDispatchResult(root, filepath.Base(dispatch.Path), RuntimeWorkerQueueDispatchResult{
		Status: "completed",
		Output: "remote result complete",
	}, RuntimeWorkerQueueDispatchResultOptions{Now: now.Add(2 * time.Minute)}); err != nil {
		t.Fatalf("submit worker queue dispatch result: %v", err)
	}
	if err := writeWorkerLeaseArtifact(root, WorkerLeaseArtifact{
		Version:     1,
		RunID:       dispatch.RunID,
		ExecutionID: dispatch.ExecutionID,
		Tool:        dispatch.Tool,
		Capability:  dispatch.Capability,
		WorkerID:    dispatch.WorkerID,
		Status:      "active",
		AcquiredAt:  now.Format(time.RFC3339Nano),
		HeartbeatAt: now.Format(time.RFC3339Nano),
	}); err != nil {
		t.Fatalf("write active worker lease fixture: %v", err)
	}
	review, err := CreateWorkerLeaseResultReconciliationDispatch(root, now.Add(3*time.Minute))
	if err != nil {
		t.Fatalf("create worker lease result reconciliation review: %v", err)
	}
	if _, err := ExecuteWorkerLeaseResultReconciliationDispatch(root, review.Path, WorkerLeaseResultReconciliationExecutionOptions{
		Execute:    true,
		Now:        now.Add(4 * time.Minute),
		ReleasedBy: "operator",
		Reason:     "reviewed",
	}); err != nil {
		t.Fatalf("execute worker lease result reconciliation review: %v", err)
	}

	model := NewTUIModelForWorkspace(root).SelectRuntimeTab(tui.RuntimeTabAgentRuntime)
	resized, cmd := model.Update(tea.WindowSizeMsg{Width: 320, Height: 62})
	if cmd != nil {
		t.Fatalf("expected resize update to be synchronous")
	}
	model = resized.(tui.Model)
	view := model.View()
	for _, want := range []string{
		"worker lease result reconciliation execution artifacts: total=1",
		"worker lease result reconciliation execution artifact:",
		"status=completed",
		"mode=metadata-only",
		"results=1",
		"lease result reconciliation execution result run=" + dispatch.RunID,
		"execution=" + dispatch.ExecutionID,
		"result=completed",
		"lease=completed",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected worker lease result reconciliation execution row %q in TUI view:\n%s", want, view)
		}
	}
}

func TestNewTUIModelForWorkspaceExecutesWorkerLeaseResultReconciliationAfterDoubleConfirm(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Date(2026, 5, 30, 19, 45, 0, 0, time.UTC)
	dispatch, review := createWorkerLeaseResultReconciliationReviewFixture(t, root, now, "completed")

	model := NewTUIModelForWorkspace(root).SelectRuntimeTab(tui.RuntimeTabAgentRuntime)
	model = selectRuntimeRowContaining(t, model, "> worker lease result reconciliation artifact:")
	armed, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if cmd != nil {
		t.Fatalf("first x should only arm worker lease result reconciliation confirmation")
	}
	executions, err := ListWorkerLeaseResultReconciliationExecutions(root)
	if err != nil {
		t.Fatalf("list reconciliation executions after first x: %v", err)
	}
	if len(executions) != 0 {
		t.Fatalf("first x must not execute reconciliation release, got %#v", executions)
	}
	leases, err := ListWorkerLeases(root)
	if err != nil {
		t.Fatalf("list worker leases after first x: %v", err)
	}
	if len(leases) != 1 || leases[0].Status != "active" {
		t.Fatalf("first x must not release worker lease, got %#v", leases)
	}
	armedModel, ok := armed.(tui.Model)
	if !ok {
		t.Fatalf("expected TUI model, got %T", armed)
	}
	armedResized, cmd := armedModel.Update(tea.WindowSizeMsg{Width: 420, Height: 90})
	if cmd != nil {
		t.Fatalf("expected armed reconciliation resize to be synchronous")
	}
	armedModel = armedResized.(tui.Model)
	armedView := armedModel.View()
	for _, want := range []string{
		"action: worker lease result reconciliation artifact selected",
		"execution status: confirm-required",
		"execution confirm: press x again",
		"action mode: metadata-only",
	} {
		if !strings.Contains(armedView, want) {
			t.Fatalf("expected armed reconciliation view %q:\n%s", want, armedView)
		}
	}

	confirmed, cmd := armedModel.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if cmd == nil {
		t.Fatalf("second x should return reconciliation execution command")
	}
	confirmed, cmd = confirmed.Update(cmd())
	if cmd != nil {
		t.Fatalf("expected reconciliation execution result update to be synchronous")
	}
	executions, err = ListWorkerLeaseResultReconciliationExecutions(root)
	if err != nil {
		t.Fatalf("list reconciliation executions after second x: %v", err)
	}
	if len(executions) != 1 || executions[0].Status != "completed" || executions[0].DispatchPath != review.Path || len(executions[0].Results) != 1 {
		t.Fatalf("expected one completed reconciliation execution audit, got %#v", executions)
	}
	leases, err = ListWorkerLeases(root)
	if err != nil {
		t.Fatalf("list worker leases after second x: %v", err)
	}
	if len(leases) != 1 || leases[0].Status != "completed" || leases[0].ResultExecutionID == "" {
		t.Fatalf("expected worker lease released to dispatch result, got %#v", leases)
	}
	resized, cmd := confirmed.Update(tea.WindowSizeMsg{Width: 420, Height: 180})
	if cmd != nil {
		t.Fatalf("expected confirmed reconciliation resize to be synchronous")
	}
	view := resized.View()
	for _, want := range []string{
		"execution status: completed",
		"worker lease result reconciliation execution artifact:",
		"lease result reconciliation execution result run=" + dispatch.RunID,
		"lease=completed",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected confirmed reconciliation view %q:\n%s", want, view)
		}
	}
}

func TestNewTUIModelForWorkspaceShowsRuntimeWorkers(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	if _, err := RegisterRuntimeWorker(root, RuntimeWorkerArtifact{
		WorkerID:     "remote-web-1",
		Mode:         "remote",
		Status:       "available",
		Endpoint:     "tcp://127.0.0.1:9443",
		Capabilities: []string{"web.discovery", "web.enumeration"},
		Labels:       map[string]string{"region": "lab"},
	}, RuntimeWorkerRegistrationOptions{}); err != nil {
		t.Fatalf("register remote worker: %v", err)
	}
	if _, err := RegisterRuntimeWorker(root, RuntimeWorkerArtifact{
		WorkerID:     "local",
		Mode:         "local",
		Status:       "offline",
		Capabilities: []string{"ctf.fast_hypothesis"},
	}, RuntimeWorkerRegistrationOptions{}); err != nil {
		t.Fatalf("register local worker: %v", err)
	}

	model := NewTUIModelForWorkspace(root).SelectRuntimeTab(tui.RuntimeTabAgentRuntime)
	resized, cmd := model.Update(tea.WindowSizeMsg{Width: 260, Height: 72})
	if cmd != nil {
		t.Fatalf("expected resize update to be synchronous")
	}
	model = resized.(tui.Model)
	view := model.View()
	for _, want := range []string{
		"runtime workers: total=2 available=1 offline=1 local=1 remote=1",
		"runtime worker remote-web-1 remote available",
		"runtime worker caps: remote-web-1 web.discovery,web.enumeration",
		"runtime worker endpoint: remote-web-1 tcp://127.0.0.1:9443",
		"runtime worker local local offline capabilities=ctf.fast_hypothesis",
		"runtime worker label: remote-web-1 region=lab",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected runtime worker row %q in TUI view:\n%s", want, view)
		}
	}
}

func TestNewTUIModelForWorkspaceShowsStaleRuntimeWorkers(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	oldHeartbeat := time.Now().UTC().Add(-DefaultRuntimeWorkerStaleAfter - time.Minute)
	if _, err := RegisterRuntimeWorker(root, RuntimeWorkerArtifact{
		WorkerID:     "remote-web-1",
		Mode:         "remote",
		Status:       "available",
		Capabilities: []string{"web.discovery"},
	}, RuntimeWorkerRegistrationOptions{Now: oldHeartbeat}); err != nil {
		t.Fatalf("register stale runtime worker: %v", err)
	}

	model := NewTUIModelForWorkspace(root).SelectRuntimeTab(tui.RuntimeTabAgentRuntime)
	resized, cmd := model.Update(tea.WindowSizeMsg{Width: 260, Height: 42})
	if cmd != nil {
		t.Fatalf("expected resize update to be synchronous")
	}
	model = resized.(tui.Model)
	view := model.View()
	for _, want := range []string{
		"runtime workers: total=1 available=0 offline=0 local=0 remote=1 stale=1",
		"runtime worker remote-web-1 remote stale capabilities=web.discovery",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected stale runtime worker row %q in TUI view:\n%s", want, view)
		}
	}
}

func TestNewTUIModelForWorkspaceShowsRuntimeWorkerQueueClaims(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	now := time.Now().UTC()
	if _, err := RegisterRuntimeWorker(root, RuntimeWorkerArtifact{
		WorkerID:     "remote-web-1",
		Mode:         "remote",
		Status:       "available",
		Endpoint:     "tcp://127.0.0.1:9443",
		Capabilities: []string{"web.discovery"},
	}, RuntimeWorkerRegistrationOptions{Now: now}); err != nil {
		t.Fatalf("register runtime worker: %v", err)
	}
	if _, err := CreateRuntimeWorkerQueueClaim(root, RuntimeWorkerQueueClaim{
		WorkerID:    "remote-web-1",
		RunID:       "run-queue-1",
		ExecutionID: "tool-web",
		Tool:        "web-api-pack",
		Capability:  "web.discovery",
	}, RuntimeWorkerQueueClaimOptions{Now: now, LeaseDuration: 10 * time.Minute}); err != nil {
		t.Fatalf("create runtime worker queue claim: %v", err)
	}

	model := NewTUIModelForWorkspace(root).SelectRuntimeTab(tui.RuntimeTabAgentRuntime)
	resized, cmd := model.Update(tea.WindowSizeMsg{Width: 260, Height: 42})
	if cmd != nil {
		t.Fatalf("expected resize update to be synchronous")
	}
	model = resized.(tui.Model)
	view := model.View()
	for _, want := range []string{
		"worker queue claims: total=1 claimed=1",
		"queue claim: remote-web-1/tool-web web.discovery claimed metadata-only",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected queue claim row %q in TUI view:\n%s", want, view)
		}
	}
}

func TestNewTUIModelForWorkspaceCancelsRuntimeWorkerQueueClaimAfterDoubleConfirm(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	now := time.Now().UTC()
	if _, err := RegisterRuntimeWorker(root, RuntimeWorkerArtifact{
		WorkerID:     "remote-web-1",
		Mode:         "remote",
		Status:       "available",
		Endpoint:     "tcp://127.0.0.1:9443",
		Capabilities: []string{"web.discovery"},
	}, RuntimeWorkerRegistrationOptions{Now: now}); err != nil {
		t.Fatalf("register runtime worker: %v", err)
	}
	claim, err := CreateRuntimeWorkerQueueClaim(root, RuntimeWorkerQueueClaim{
		WorkerID:    "remote-web-1",
		RunID:       "run-cancel-tui",
		ExecutionID: "tool-web",
		Tool:        "web-api-pack",
		Capability:  "web.discovery",
	}, RuntimeWorkerQueueClaimOptions{Now: now, LeaseDuration: 10 * time.Minute})
	if err != nil {
		t.Fatalf("create runtime worker queue claim: %v", err)
	}

	model := NewTUIModelForWorkspace(root).SelectRuntimeTab(tui.RuntimeTabAgentRuntime)
	model = selectRuntimeRowContaining(t, model, "> worker queue claim artifact:")
	armed, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'C'}})
	if cmd != nil {
		t.Fatalf("first C should only arm queue claim cancel confirmation")
	}
	read, err := ReadRuntimeWorkerQueueClaim(root, claim.Path)
	if err != nil {
		t.Fatalf("read claim after first C: %v", err)
	}
	if read.Status != "claimed" || read.CanceledAt != "" {
		t.Fatalf("first C must not cancel claim, got %#v", read)
	}
	armedModel, ok := armed.(tui.Model)
	if !ok {
		t.Fatalf("expected TUI model, got %T", armed)
	}
	armedResized, cmd := armedModel.Update(tea.WindowSizeMsg{Width: 420, Height: 90})
	if cmd != nil {
		t.Fatalf("expected armed queue claim cancel resize to be synchronous")
	}
	armedModel = armedResized.(tui.Model)
	for _, want := range []string{
		"action: worker queue claim artifact selected",
		"control action: cancel",
		"execution status: confirm-required",
		"execution confirm: press C again",
	} {
		if !strings.Contains(armedModel.View(), want) {
			t.Fatalf("expected armed queue claim cancel view %q:\n%s", want, armedModel.View())
		}
	}

	confirmed, cmd := armedModel.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'C'}})
	if cmd == nil {
		t.Fatalf("second C should return queue claim cancel command")
	}
	confirmed, cmd = confirmed.Update(cmd())
	if cmd != nil {
		t.Fatalf("expected queue claim cancel result update to be synchronous")
	}
	read, err = ReadRuntimeWorkerQueueClaim(root, claim.Path)
	if err != nil {
		t.Fatalf("read claim after second C: %v", err)
	}
	if read.Status != "canceled" || read.CanceledAt == "" || read.CanceledBy != "tui" || !strings.Contains(read.CancelReason, "TUI cancel requested") {
		t.Fatalf("expected canceled queue claim from TUI, got %#v", read)
	}
	dispatches, err := ListRuntimeWorkerQueueDispatchExecutions(root)
	if err != nil {
		t.Fatalf("list dispatch executions: %v", err)
	}
	if len(dispatches) != 0 {
		t.Fatalf("queue claim cancel shortcut must not dispatch, got %#v", dispatches)
	}
	resized, cmd := confirmed.Update(tea.WindowSizeMsg{Width: 420, Height: 180})
	if cmd != nil {
		t.Fatalf("expected confirmed queue claim cancel resize to be synchronous")
	}
	view := resized.View()
	for _, want := range []string{
		"action output: queue claim canceled",
		"execution status: canceled",
		"worker queue claims: total=1 claimed=0 canceled=1",
		"queue claim: remote-web-1/tool-web web.discovery canceled metadata-only",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected confirmed queue claim cancel view %q:\n%s", want, view)
		}
	}
}

func TestNewTUIModelForWorkspaceExpiresRuntimeWorkerQueueClaimAfterDoubleConfirm(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	now := time.Now().UTC()
	if _, err := RegisterRuntimeWorker(root, RuntimeWorkerArtifact{
		WorkerID:     "remote-web-1",
		Mode:         "remote",
		Status:       "available",
		Endpoint:     "tcp://127.0.0.1:9443",
		Capabilities: []string{"web.discovery"},
	}, RuntimeWorkerRegistrationOptions{Now: now}); err != nil {
		t.Fatalf("register runtime worker: %v", err)
	}
	claim, err := CreateRuntimeWorkerQueueClaim(root, RuntimeWorkerQueueClaim{
		WorkerID:    "remote-web-1",
		RunID:       "run-expire-tui",
		ExecutionID: "tool-web",
		Tool:        "web-api-pack",
		Capability:  "web.discovery",
	}, RuntimeWorkerQueueClaimOptions{Now: now.Add(-2 * time.Minute), LeaseDuration: time.Minute})
	if err != nil {
		t.Fatalf("create runtime worker queue claim: %v", err)
	}

	model := NewTUIModelForWorkspace(root).SelectRuntimeTab(tui.RuntimeTabAgentRuntime)
	model = selectRuntimeRowContaining(t, model, "> worker queue claim artifact:")
	armed, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'E'}})
	if cmd != nil {
		t.Fatalf("first E should only arm queue claim expire confirmation")
	}
	read, err := ReadRuntimeWorkerQueueClaim(root, claim.Path)
	if err != nil {
		t.Fatalf("read claim after first E: %v", err)
	}
	if read.Status != "claimed" || read.ExpiredAt != "" {
		t.Fatalf("first E must not expire claim, got %#v", read)
	}
	armedModel, ok := armed.(tui.Model)
	if !ok {
		t.Fatalf("expected TUI model, got %T", armed)
	}
	armedResized, cmd := armedModel.Update(tea.WindowSizeMsg{Width: 420, Height: 90})
	if cmd != nil {
		t.Fatalf("expected armed queue claim expire resize to be synchronous")
	}
	armedModel = armedResized.(tui.Model)
	for _, want := range []string{
		"action: worker queue claim artifact selected",
		"control action: expire",
		"execution status: confirm-required",
		"execution confirm: press E again",
	} {
		if !strings.Contains(armedModel.View(), want) {
			t.Fatalf("expected armed queue claim expire view %q:\n%s", want, armedModel.View())
		}
	}

	confirmed, cmd := armedModel.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'E'}})
	if cmd == nil {
		t.Fatalf("second E should return queue claim expire command")
	}
	confirmed, cmd = confirmed.Update(cmd())
	if cmd != nil {
		t.Fatalf("expected queue claim expire result update to be synchronous")
	}
	read, err = ReadRuntimeWorkerQueueClaim(root, claim.Path)
	if err != nil {
		t.Fatalf("read claim after second E: %v", err)
	}
	if read.Status != "expired" || read.ExpiredAt == "" || read.ExpiredBy != "tui" || !strings.Contains(read.ExpireReason, "TUI expire requested") {
		t.Fatalf("expected expired queue claim from TUI, got %#v", read)
	}
	dispatches, err := ListRuntimeWorkerQueueDispatchExecutions(root)
	if err != nil {
		t.Fatalf("list dispatch executions: %v", err)
	}
	if len(dispatches) != 0 {
		t.Fatalf("queue claim expire shortcut must not dispatch, got %#v", dispatches)
	}
	resized, cmd := confirmed.Update(tea.WindowSizeMsg{Width: 420, Height: 90})
	if cmd != nil {
		t.Fatalf("expected confirmed queue claim expire resize to be synchronous")
	}
	view := resized.View()
	for _, want := range []string{
		"action output: queue claim expired",
		"execution status: expired",
		"worker queue claims: total=1 claimed=0 canceled=0 expired=1",
		"queue claim: remote-web-1/tool-web web.discovery expired metadata-only",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected confirmed queue claim expire view %q:\n%s", want, view)
		}
	}
}

func TestNewTUIModelForWorkspaceShowsRenewedRuntimeWorkerQueueClaim(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	now := time.Now().UTC()
	if _, err := RegisterRuntimeWorker(root, RuntimeWorkerArtifact{
		WorkerID:     "remote-web-1",
		Mode:         "remote",
		Status:       "available",
		Endpoint:     "tcp://127.0.0.1:9443",
		Capabilities: []string{"web.discovery"},
	}, RuntimeWorkerRegistrationOptions{Now: now}); err != nil {
		t.Fatalf("register runtime worker: %v", err)
	}
	claim, err := CreateRuntimeWorkerQueueClaim(root, RuntimeWorkerQueueClaim{
		WorkerID:    "remote-web-1",
		RunID:       "run-renew-tui",
		ExecutionID: "tool-web",
		Tool:        "web-api-pack",
		Capability:  "web.discovery",
	}, RuntimeWorkerQueueClaimOptions{Now: now.Add(-2 * time.Minute), LeaseDuration: 5 * time.Minute})
	if err != nil {
		t.Fatalf("create runtime worker queue claim: %v", err)
	}
	renewed, err := RenewRuntimeWorkerQueueClaim(root, claim.Path, RuntimeWorkerQueueClaimRenewOptions{
		Renew:         true,
		Now:           now,
		LeaseDuration: 10 * time.Minute,
		RenewedBy:     "operator",
		Reason:        "keep the active assignment alive",
	})
	if err != nil {
		t.Fatalf("renew runtime worker queue claim: %v", err)
	}

	model := NewTUIModelForWorkspace(root).SelectRuntimeTab(tui.RuntimeTabAgentRuntime)
	resized, cmd := model.Update(tea.WindowSizeMsg{Width: 260, Height: 42})
	if cmd != nil {
		t.Fatalf("expected resize update to be synchronous")
	}
	model = resized.(tui.Model)
	view := model.View()
	for _, want := range []string{
		"worker queue claims: total=1 claimed=1 canceled=0 expired=0",
		"worker queue claim renewed: at=",
		"by=operator",
		"reason=keep the active assignment alive",
		"expires=" + renewed.ExpiresAt,
		"queue claim: remote-web-1/tool-web web.discovery claimed metadata-only",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected renewed queue claim row %q in TUI view:\n%s", want, view)
		}
	}
}

func TestNewTUIModelForWorkspaceShowsRuntimeWorkerPlacementPlan(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	now := time.Now().UTC()
	if _, err := RegisterRuntimeWorker(root, RuntimeWorkerArtifact{
		WorkerID:     "remote-web-1",
		Mode:         "remote",
		Status:       "available",
		Capabilities: []string{"web.discovery"},
	}, RuntimeWorkerRegistrationOptions{Now: now}); err != nil {
		t.Fatalf("register runtime worker: %v", err)
	}
	if _, err := CreateRuntimeWorkerPlacementPlan(root, RuntimeWorkerPlacementPlanOptions{
		Now:   now,
		RunID: "run-placement-tui",
	}); err != nil {
		t.Fatalf("create placement plan: %v", err)
	}

	model := NewTUIModelForWorkspace(root).SelectRuntimeTab(tui.RuntimeTabAgentRuntime)
	resized, cmd := model.Update(tea.WindowSizeMsg{Width: 260, Height: 42})
	if cmd != nil {
		t.Fatalf("expected resize update to be synchronous")
	}
	model = resized.(tui.Model)
	view := model.View()
	if !strings.Contains(view, "worker placements: ready=2 skipped=6") {
		t.Fatalf("expected live placement plan summary, got:\n%s", view)
	}
	if !strings.Contains(view, "worker placement web-api-pack web.discovery: ready") || !strings.Contains(view, "placement: remote-web-1/") {
		t.Fatalf("expected ready worker placement row, got:\n%s", view)
	}
	artifactModel := model.SelectRuntimeRowMatching(tui.RuntimeTabAgentRuntime, "worker placement artifact:", "")
	artifactResized, cmd := artifactModel.Update(tea.WindowSizeMsg{Width: 260, Height: 90})
	if cmd != nil {
		t.Fatalf("expected artifact resize update to be synchronous")
	}
	artifactView := artifactResized.View()
	if !strings.Contains(artifactView, "worker placement artifacts: total=1") || !strings.Contains(artifactView, "worker placement artifact:") {
		t.Fatalf("expected persisted placement artifact rows, got:\n%s", artifactView)
	}
}

func TestNewTUIModelForWorkspaceClaimsRuntimeWorkerPlacementAfterDoubleConfirm(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	now := time.Now().UTC()
	if _, err := RegisterRuntimeWorker(root, RuntimeWorkerArtifact{
		WorkerID:     "remote-web-1",
		Mode:         "remote",
		Status:       "available",
		Capabilities: []string{"web.discovery"},
	}, RuntimeWorkerRegistrationOptions{Now: now}); err != nil {
		t.Fatalf("register runtime worker: %v", err)
	}
	if _, err := CreateRuntimeWorkerPlacementPlan(root, RuntimeWorkerPlacementPlanOptions{
		Now:   now,
		RunID: "run-placement-tui-claim",
	}); err != nil {
		t.Fatalf("create placement plan: %v", err)
	}

	model := NewTUIModelForWorkspace(root).SelectRuntimeTab(tui.RuntimeTabAgentRuntime)
	model = selectRuntimeRowContaining(t, model, "> worker placement artifact:")
	armed, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if cmd != nil {
		t.Fatalf("first x should only arm placement claim confirmation")
	}
	claims, err := ListRuntimeWorkerQueueClaims(root)
	if err != nil {
		t.Fatalf("list claims after first x: %v", err)
	}
	if len(claims) != 0 {
		t.Fatalf("first x must not create queue claims, got %#v", claims)
	}
	armedModel, ok := armed.(tui.Model)
	if !ok {
		t.Fatalf("expected TUI model, got %T", armed)
	}
	armedResized, cmd := armedModel.Update(tea.WindowSizeMsg{Width: 420, Height: 90})
	if cmd != nil {
		t.Fatalf("expected armed placement claim resize to be synchronous")
	}
	armedModel = armedResized.(tui.Model)
	armedView := armedModel.View()
	for _, want := range []string{
		"action: worker placement artifact selected",
		"execution status: confirm-required",
		"execution confirm: press x again",
	} {
		if !strings.Contains(armedView, want) {
			t.Fatalf("expected armed placement claim view %q:\n%s", want, armedView)
		}
	}

	confirmed, cmd := armedModel.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if cmd == nil {
		t.Fatalf("second x should return placement claim command")
	}
	confirmed, cmd = confirmed.Update(cmd())
	if cmd != nil {
		t.Fatalf("expected placement claim result update to be synchronous")
	}
	claims, err = ListRuntimeWorkerQueueClaims(root)
	if err != nil {
		t.Fatalf("list claims after second x: %v", err)
	}
	if len(claims) != 2 || claims[0].Status != "claimed" || claims[1].Status != "claimed" || claims[0].WorkerID != "remote-web-1" || claims[1].WorkerID != "remote-web-1" {
		t.Fatalf("expected two queue claims from tool-level placement, got %#v", claims)
	}
	dispatches, err := ListRuntimeWorkerQueueDispatchExecutions(root)
	if err != nil {
		t.Fatalf("list dispatches after placement claim: %v", err)
	}
	if len(dispatches) != 0 {
		t.Fatalf("TUI placement claim must not dispatch, got %#v", dispatches)
	}
	resized, cmd := confirmed.Update(tea.WindowSizeMsg{Width: 420, Height: 90})
	if cmd != nil {
		t.Fatalf("expected confirmed placement claim resize to be synchronous")
	}
	view := resized.View()
	for _, want := range []string{
		"action output: queue claims created",
		"execution status: claimed",
		"worker placement claim executions: total=2 claimed=2 skipped=0 failed=0",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected confirmed placement claim view %q:\n%s", want, view)
		}
	}
}

func TestNewTUIModelForWorkspaceShowsRuntimeWorkerPlacementDispatchPlan(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	now := time.Now().UTC()
	if _, err := RegisterRuntimeWorker(root, RuntimeWorkerArtifact{
		WorkerID:     "remote-web-1",
		Mode:         "remote",
		Status:       "available",
		Capabilities: []string{"web.discovery"},
	}, RuntimeWorkerRegistrationOptions{Now: now}); err != nil {
		t.Fatalf("register runtime worker: %v", err)
	}
	placement, err := CreateRuntimeWorkerPlacementPlan(root, RuntimeWorkerPlacementPlanOptions{
		Now:   now,
		RunID: "run-placement-dispatch-tui",
	})
	if err != nil {
		t.Fatalf("create placement plan: %v", err)
	}
	if _, err := ExecuteRuntimeWorkerPlacementClaim(root, placement.Path, RuntimeWorkerPlacementClaimExecutionOptions{
		Execute:              true,
		CandidateExecutionID: "tool-discover_assets-01",
		WorkerID:             "remote-web-1",
		Now:                  now.Add(time.Minute),
	}); err != nil {
		t.Fatalf("execute placement claim: %v", err)
	}
	if _, err := CreateRuntimeWorkerPlacementDispatchPlan(root, RuntimeWorkerPlacementDispatchPlanOptions{
		Now:           now.Add(2 * time.Minute),
		TransportMode: "file-outbox",
	}); err != nil {
		t.Fatalf("create placement dispatch plan: %v", err)
	}

	model := NewTUIModelForWorkspace(root).SelectRuntimeTab(tui.RuntimeTabAgentRuntime)
	resized, cmd := model.Update(tea.WindowSizeMsg{Width: 520, Height: 180})
	if cmd != nil {
		t.Fatalf("expected TUI resize to be synchronous")
	}
	view := resized.View()
	for _, want := range []string{
		"worker placement dispatches: ready=1 skipped=0 mode=metadata-only transport=file-outbox",
		"worker placement dispatch artifact:",
		"placement dispatch: remote-web-1/tool-discover_assets-01 web.discovery ready file-outbox",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected placement dispatch plan view %q:\n%s", want, view)
		}
	}
	dispatches, err := ListRuntimeWorkerQueueDispatchExecutions(root)
	if err != nil {
		t.Fatalf("list queue dispatches: %v", err)
	}
	if len(dispatches) != 0 {
		t.Fatalf("placement dispatch plan TUI rows must not dispatch, got %#v", dispatches)
	}
}

func TestNewTUIModelForWorkspaceShowsRuntimeWorkerOrchestrationArtifactDetails(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	now := time.Now().UTC()
	if _, err := RegisterRuntimeWorker(root, RuntimeWorkerArtifact{
		WorkerID:     "remote-web-1",
		Mode:         "remote",
		Status:       "available",
		Capabilities: []string{"web.discovery"},
	}, RuntimeWorkerRegistrationOptions{Now: now}); err != nil {
		t.Fatalf("register runtime worker: %v", err)
	}
	if _, err := CreateRuntimeWorkerOrchestration(root, RuntimeWorkerOrchestrationOptions{
		Now:   now,
		RunID: "run-worker-orchestration-tui",
	}); err != nil {
		t.Fatalf("create worker orchestration: %v", err)
	}

	model := NewTUIModelForWorkspace(root).SelectRuntimeTab(tui.RuntimeTabAgentRuntime)
	resized, cmd := model.Update(tea.WindowSizeMsg{Width: 420, Height: 90})
	if cmd != nil {
		t.Fatalf("expected TUI resize to be synchronous")
	}
	view := resized.View()
	for _, want := range []string{
		"worker orchestrations: total=1",
		"worker orchestration artifact:",
		"placement_ready=2",
		"placement_skipped=6",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected worker orchestration view %q:\n%s", want, view)
		}
	}
	claims, err := ListRuntimeWorkerQueueClaims(root)
	if err != nil {
		t.Fatalf("list queue claims: %v", err)
	}
	if len(claims) != 0 {
		t.Fatalf("TUI orchestration rows must not create queue claims, got %#v", claims)
	}

	model = selectRuntimeRowContaining(t, model, "> worker orchestration artifact:")
	detailed, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	if cmd != nil {
		t.Fatalf("expected worker orchestration detail update to be synchronous")
	}
	detailedModel, ok := detailed.(tui.Model)
	if !ok {
		t.Fatalf("expected TUI model, got %T", detailed)
	}
	resizedDetail, cmd := detailedModel.Update(tea.WindowSizeMsg{Width: 420, Height: 120})
	if cmd != nil {
		t.Fatalf("expected worker orchestration detail resize to be synchronous")
	}
	detailView := resizedDetail.View()
	for _, want := range []string{
		"runtime action: worker-orchestration-artifact",
		"artifact path:",
		"state: dry-run",
		"execution mode: metadata-only",
		"ready: 2",
		"skipped: 6",
		"mode: read-only",
	} {
		if !strings.Contains(detailView, want) {
			t.Fatalf("expected worker orchestration detail %q:\n%s", want, detailView)
		}
	}
}

func TestNewTUIModelForWorkspaceShowsRuntimeWorkerOrchestrationLifecycleRetryDispatchRows(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Date(2026, 6, 1, 15, 40, 0, 0, time.UTC)
	marker := filepath.Join(t.TempDir(), "orchestration-lifecycle-retry-tui-ran")
	createRuntimeWorkerDispatchRetryDispatchFixture(t, root, now, RuntimeWorkerToolPlanPayload{
		Action:     "discover_assets",
		Runner:     "command",
		Parser:     "web_api_parser",
		RunCommand: []string{"sh", "-c", "touch " + marker},
	})
	orchestration, err := CreateRuntimeWorkerOrchestration(root, RuntimeWorkerOrchestrationOptions{
		Execute:                         true,
		Now:                             now.Add(5 * time.Minute),
		RunID:                           "run-worker-orchestration-lifecycle-retry-tui",
		TransportMode:                   "metadata-only",
		MaxCandidates:                   1,
		LeaseDuration:                   10 * time.Minute,
		IncludeLifecycleMaintenance:     true,
		ExecuteLifecycleRetryDispatches: true,
	})
	if err != nil {
		t.Fatalf("create worker orchestration lifecycle retry dispatch fixture: %v", err)
	}
	if orchestration.RetryDispatchExecutionsCreated != 1 || orchestration.RetryDispatchClaimsCreated != 1 || len(orchestration.RetryDispatchExecutionPaths) != 1 {
		t.Fatalf("expected orchestration retry-dispatch aggregate fixture, got %#v", orchestration)
	}

	model := NewTUIModelForWorkspace(root).SelectRuntimeTab(tui.RuntimeTabAgentRuntime)
	resized, cmd := model.Update(tea.WindowSizeMsg{Width: 420, Height: 90})
	if cmd != nil {
		t.Fatalf("expected TUI resize to be synchronous")
	}
	view := resized.View()
	for _, want := range []string{
		"worker orchestration retry dispatch: executions_created=1 claims_created=1",
		"worker orchestration retry dispatch execution:",
		"worker dispatch retry execution artifact:",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected orchestration retry-dispatch TUI row %q:\n%s", want, view)
		}
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("orchestration lifecycle retry TUI rows must not execute retry tool-plan command, marker stat err=%v", err)
	}
}

func TestNewTUIModelForWorkspaceShowsRuntimeWorkerOrchestrationSupervisorRows(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	now := time.Now().UTC()
	if _, err := RegisterRuntimeWorker(root, RuntimeWorkerArtifact{
		WorkerID:     "remote-web-1",
		Mode:         "remote",
		Status:       "available",
		Endpoint:     "https://worker.example.test/dispatch",
		Capabilities: []string{"web.discovery"},
	}, RuntimeWorkerRegistrationOptions{Now: now}); err != nil {
		t.Fatalf("register runtime worker: %v", err)
	}
	supervisor, err := CreateRuntimeWorkerOrchestrationSupervisor(root, RuntimeWorkerOrchestrationSupervisorOptions{
		Execute:                     true,
		Now:                         now.Add(time.Minute),
		RunIDPrefix:                 "run-worker-orchestration-supervisor-tui",
		TransportMode:               "metadata-only",
		MaxCandidates:               1,
		MaxRuns:                     2,
		LeaseDuration:               10 * time.Minute,
		IncludeLifecycleMaintenance: true,
	})
	if err != nil {
		t.Fatalf("create worker orchestration supervisor: %v", err)
	}

	model := NewTUIModelForWorkspace(root).SelectRuntimeTab(tui.RuntimeTabAgentRuntime)
	resized, cmd := model.Update(tea.WindowSizeMsg{Width: 420, Height: 120})
	if cmd != nil {
		t.Fatalf("expected TUI resize to be synchronous")
	}
	view := resized.View()
	for _, want := range []string{
		"worker orchestration supervisors: total=1 completed=1 dry-run=0 failed=0",
		"worker orchestration supervisor artifact:",
		"worker orchestration supervisor state: completed mode=metadata-only runs=2 max_runs=2 transport=metadata-only",
		"worker orchestration supervisor created: orchestrations=2 lifecycle_maintenance=2 lifecycle_audit=2",
		"worker orchestration supervisor dispatch: claimed=2 dispatched=2 skipped=0 failed=0",
		"worker orchestration supervisor path:",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected worker orchestration supervisor view %q:\n%s", want, view)
		}
	}
	if supervisor.Path == "" || len(supervisor.OrchestrationPaths) != 2 {
		t.Fatalf("expected supervisor paths, got %#v", supervisor)
	}

	model = selectRuntimeRowContaining(t, model, "> worker orchestration supervisor artifact:")
	detailed, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	if cmd != nil {
		t.Fatalf("expected worker orchestration supervisor detail update to be synchronous")
	}
	detailedModel, ok := detailed.(tui.Model)
	if !ok {
		t.Fatalf("expected TUI model, got %T", detailed)
	}
	resizedDetail, cmd := detailedModel.Update(tea.WindowSizeMsg{Width: 420, Height: 120})
	if cmd != nil {
		t.Fatalf("expected worker orchestration supervisor detail resize to be synchronous")
	}
	detailView := resizedDetail.View()
	for _, want := range []string{
		"runtime action: worker-orchestration-supervisor-artifact",
		"artifact path:",
		"state: completed",
		"execution mode: metadata-only",
		"transport: metadata-only",
		"ready: 2",
		"results: max_runs=2 orchestrations_created=2 lifecycle_maintenance=2 lifecycle_audit=2 claimed=2 dispatched=2",
		"skipped: 0",
		"failed: 0",
		"mode: read-only",
	} {
		if !strings.Contains(detailView, want) {
			t.Fatalf("expected worker orchestration supervisor detail %q:\n%s", want, detailView)
		}
	}
}

func TestNewTUIModelForWorkspaceShowsRuntimeWorkerOrchestrationSupervisorLifecycleRetryDispatchRows(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Date(2026, 6, 1, 16, 10, 0, 0, time.UTC)
	marker := filepath.Join(t.TempDir(), "orchestration-supervisor-lifecycle-retry-tui-ran")
	createRuntimeWorkerDispatchRetryDispatchFixture(t, root, now, RuntimeWorkerToolPlanPayload{
		Action:     "discover_assets",
		Runner:     "command",
		Parser:     "web_api_parser",
		RunCommand: []string{"sh", "-c", "touch " + marker},
	})
	supervisor, err := CreateRuntimeWorkerOrchestrationSupervisor(root, RuntimeWorkerOrchestrationSupervisorOptions{
		Execute:                         true,
		Now:                             now.Add(5 * time.Minute),
		RunIDPrefix:                     "run-worker-orchestration-supervisor-lifecycle-retry-tui",
		TransportMode:                   "metadata-only",
		MaxCandidates:                   1,
		MaxRuns:                         1,
		LeaseDuration:                   10 * time.Minute,
		IncludeLifecycleMaintenance:     true,
		ExecuteLifecycleRetryDispatches: true,
	})
	if err != nil {
		t.Fatalf("create worker orchestration supervisor lifecycle retry dispatch fixture: %v", err)
	}
	if supervisor.Summary.RetryDispatchExecutionsCreated != 1 || supervisor.Summary.RetryDispatchClaimsCreated != 1 || len(supervisor.RetryDispatchExecutionPaths) != 1 {
		t.Fatalf("expected supervisor retry-dispatch aggregate fixture, got %#v", supervisor)
	}

	model := NewTUIModelForWorkspace(root).SelectRuntimeTab(tui.RuntimeTabAgentRuntime)
	resized, cmd := model.Update(tea.WindowSizeMsg{Width: 420, Height: 120})
	if cmd != nil {
		t.Fatalf("expected TUI resize to be synchronous")
	}
	view := resized.View()
	for _, want := range []string{
		"worker orchestration supervisor retry dispatch: executions_created=1 claims_created=1",
		"worker orchestration supervisor retry dispatch execution:",
		"worker dispatch retry execution artifact:",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected supervisor retry-dispatch TUI row %q:\n%s", want, view)
		}
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("supervisor lifecycle retry TUI rows must not execute retry tool-plan command, marker stat err=%v", err)
	}
}

func TestNewTUIModelForWorkspaceDispatchesRuntimeWorkerPlacementAfterDoubleConfirm(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	now := time.Now().UTC()
	if _, err := RegisterRuntimeWorker(root, RuntimeWorkerArtifact{
		WorkerID:     "remote-web-1",
		Mode:         "remote",
		Status:       "available",
		Capabilities: []string{"web.discovery"},
	}, RuntimeWorkerRegistrationOptions{Now: now}); err != nil {
		t.Fatalf("register runtime worker: %v", err)
	}
	placement, err := CreateRuntimeWorkerPlacementPlan(root, RuntimeWorkerPlacementPlanOptions{
		Now:   now,
		RunID: "run-placement-dispatch-exec-tui",
	})
	if err != nil {
		t.Fatalf("create placement plan: %v", err)
	}
	if _, err := ExecuteRuntimeWorkerPlacementClaim(root, placement.Path, RuntimeWorkerPlacementClaimExecutionOptions{
		Execute:              true,
		CandidateExecutionID: "tool-discover_assets-01",
		WorkerID:             "remote-web-1",
		Now:                  now.Add(time.Minute),
	}); err != nil {
		t.Fatalf("execute placement claim: %v", err)
	}
	if _, err := CreateRuntimeWorkerPlacementDispatchPlan(root, RuntimeWorkerPlacementDispatchPlanOptions{
		Now:           now.Add(2 * time.Minute),
		TransportMode: "metadata-only",
	}); err != nil {
		t.Fatalf("create placement dispatch plan: %v", err)
	}

	model := NewTUIModelForWorkspace(root).SelectRuntimeTab(tui.RuntimeTabAgentRuntime)
	model = selectRuntimeRowContaining(t, model, "> worker placement dispatch artifact:")
	armed, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if cmd != nil {
		t.Fatalf("first x should only arm placement dispatch confirmation")
	}
	dispatches, err := ListRuntimeWorkerQueueDispatchExecutions(root)
	if err != nil {
		t.Fatalf("list dispatches after first x: %v", err)
	}
	if len(dispatches) != 0 {
		t.Fatalf("first x must not dispatch, got %#v", dispatches)
	}
	armedModel, ok := armed.(tui.Model)
	if !ok {
		t.Fatalf("expected TUI model, got %T", armed)
	}
	armedResized, cmd := armedModel.Update(tea.WindowSizeMsg{Width: 420, Height: 90})
	if cmd != nil {
		t.Fatalf("expected armed placement dispatch resize to be synchronous")
	}
	armedModel = armedResized.(tui.Model)
	armedView := armedModel.View()
	for _, want := range []string{
		"action: worker placement dispatch artifact selected",
		"execution status: confirm-required",
		"execution confirm: press x again",
	} {
		if !strings.Contains(armedView, want) {
			t.Fatalf("expected armed placement dispatch view %q:\n%s", want, armedView)
		}
	}

	confirmed, cmd := armedModel.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if cmd == nil {
		t.Fatalf("second x should return placement dispatch command")
	}
	confirmed, cmd = confirmed.Update(cmd())
	if cmd != nil {
		t.Fatalf("expected placement dispatch result update to be synchronous")
	}
	dispatches, err = ListRuntimeWorkerQueueDispatchExecutions(root)
	if err != nil {
		t.Fatalf("list dispatches after second x: %v", err)
	}
	if len(dispatches) != 1 || dispatches[0].Status != "dispatched" || dispatches[0].TransportMode != "metadata-only" {
		t.Fatalf("expected one metadata-only queue dispatch, got %#v", dispatches)
	}
	executions, err := ListRuntimeWorkerPlacementDispatchExecutions(root)
	if err != nil {
		t.Fatalf("list placement dispatch executions: %v", err)
	}
	if len(executions) != 1 || executions[0].QueueDispatchExecutionPath != dispatches[0].Path {
		t.Fatalf("expected placement dispatch execution audit, got %#v", executions)
	}
	resized, cmd := confirmed.Update(tea.WindowSizeMsg{Width: 420, Height: 90})
	if cmd != nil {
		t.Fatalf("expected confirmed placement dispatch resize to be synchronous")
	}
	view := resized.View()
	for _, want := range []string{
		"action output: queue dispatch created",
		"execution status: dispatched",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected confirmed placement dispatch view %q:\n%s", want, view)
		}
	}
}

func TestNewTUIModelForWorkspaceDispatchesRuntimeWorkerPlacementBatchAfterDoubleConfirm(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	plan := createPlacementDispatchBatchPlanFixtureAt(t, root, "metadata-only", time.Now().UTC().Add(-2*time.Minute))
	if plan.ReadyCount != 8 {
		t.Fatalf("expected eight ready placement dispatch candidates, got %#v", plan)
	}

	model := NewTUIModelForWorkspace(root).
		SelectRuntimeTab(tui.RuntimeTabAgentRuntime).
		SelectRuntimeRowMatching(tui.RuntimeTabAgentRuntime, "worker placement dispatch artifact:", plan.Path)
	armed, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if cmd != nil {
		t.Fatalf("first x should only arm placement dispatch batch confirmation")
	}
	dispatches, err := ListRuntimeWorkerQueueDispatchExecutions(root)
	if err != nil {
		t.Fatalf("list dispatches after first x: %v", err)
	}
	if len(dispatches) != 0 {
		t.Fatalf("first x must not dispatch batch, got %#v", dispatches)
	}
	armedModel, ok := armed.(tui.Model)
	if !ok {
		t.Fatalf("expected TUI model, got %T", armed)
	}
	confirmed, cmd := armedModel.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if cmd == nil {
		t.Fatalf("second x should return placement dispatch batch command")
	}
	confirmed, cmd = confirmed.Update(cmd())
	if cmd != nil {
		t.Fatalf("expected placement dispatch batch result update to be synchronous")
	}
	dispatches, err = ListRuntimeWorkerQueueDispatchExecutions(root)
	if err != nil {
		t.Fatalf("list dispatches after second x: %v", err)
	}
	if len(dispatches) != 8 {
		t.Fatalf("expected eight metadata-only queue dispatches, got %#v", dispatches)
	}
	batches, err := ListRuntimeWorkerPlacementDispatchBatches(root)
	if err != nil {
		t.Fatalf("list placement dispatch batches: %v", err)
	}
	if len(batches) != 1 || batches[0].DispatchedCount != 8 {
		t.Fatalf("expected one placement dispatch batch audit, got %#v", batches)
	}
	resized, cmd := confirmed.Update(tea.WindowSizeMsg{Width: 420, Height: 90})
	if cmd != nil {
		t.Fatalf("expected confirmed placement dispatch batch resize to be synchronous")
	}
	view := resized.View()
	for _, want := range []string{
		"action output: queue dispatch batch created",
		"execution status: completed",
		"worker placement dispatch batch artifact:",
		"worker placement dispatch execution artifact:",
		"worker queue dispatch execution artifact:",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected confirmed placement dispatch batch view %q:\n%s", want, view)
		}
	}
}

func TestNewTUIModelForWorkspaceShowsRuntimeWorkerQueueDispatchExecutions(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	now := time.Now().UTC()
	if _, err := RegisterRuntimeWorker(root, RuntimeWorkerArtifact{
		WorkerID:     "remote-web-1",
		Mode:         "remote",
		Status:       "available",
		Capabilities: []string{"web.discovery"},
	}, RuntimeWorkerRegistrationOptions{Now: now}); err != nil {
		t.Fatalf("register runtime worker: %v", err)
	}
	claim, err := CreateRuntimeWorkerQueueClaim(root, RuntimeWorkerQueueClaim{
		WorkerID:    "remote-web-1",
		RunID:       "run-dispatch-1",
		ExecutionID: "tool-web",
		Tool:        "web-api-pack",
		Capability:  "web.discovery",
	}, RuntimeWorkerQueueClaimOptions{Now: now, LeaseDuration: 10 * time.Minute})
	if err != nil {
		t.Fatalf("create runtime worker queue claim: %v", err)
	}
	if _, err := ExecuteRuntimeWorkerQueueClaimDispatch(root, claim.Path, RuntimeWorkerQueueDispatchExecutionOptions{Execute: true, Now: now.Add(time.Minute)}); err != nil {
		t.Fatalf("execute runtime worker queue claim dispatch: %v", err)
	}

	model := NewTUIModelForWorkspace(root).SelectRuntimeTab(tui.RuntimeTabAgentRuntime)
	resized, cmd := model.Update(tea.WindowSizeMsg{Width: 260, Height: 42})
	if cmd != nil {
		t.Fatalf("expected resize update to be synchronous")
	}
	model = resized.(tui.Model)
	view := model.View()
	for _, want := range []string{
		"worker queue dispatch executions: total=1 dispatched=1 skipped=0",
		"queue dispatch: remote-web-1/tool-web web.discovery dispatched metadata-only",
		"worker queue dispatch execution artifact:",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected queue dispatch execution row %q in TUI view:\n%s", want, view)
		}
	}
}

func TestNewTUIModelForWorkspaceShowsRuntimeWorkerFileOutboxTransportRequest(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	now := time.Now().UTC()
	if _, err := RegisterRuntimeWorker(root, RuntimeWorkerArtifact{
		WorkerID:     "remote-web-1",
		Mode:         "remote",
		Status:       "available",
		Endpoint:     "file://remote-web-1/inbox",
		Capabilities: []string{"web.discovery"},
	}, RuntimeWorkerRegistrationOptions{Now: now}); err != nil {
		t.Fatalf("register runtime worker: %v", err)
	}
	claim, err := CreateRuntimeWorkerQueueClaim(root, RuntimeWorkerQueueClaim{
		WorkerID:    "remote-web-1",
		RunID:       "run-dispatch-file",
		ExecutionID: "tool-web",
		Tool:        "web-api-pack",
		Capability:  "web.discovery",
	}, RuntimeWorkerQueueClaimOptions{Now: now, LeaseDuration: 10 * time.Minute})
	if err != nil {
		t.Fatalf("create runtime worker queue claim: %v", err)
	}
	if _, err := ExecuteRuntimeWorkerQueueClaimDispatch(root, claim.Path, RuntimeWorkerQueueDispatchExecutionOptions{
		Execute:       true,
		Now:           now.Add(time.Minute),
		TransportMode: "file-outbox",
	}); err != nil {
		t.Fatalf("execute runtime worker queue claim dispatch: %v", err)
	}

	model := NewTUIModelForWorkspace(root).SelectRuntimeTab(tui.RuntimeTabAgentRuntime)
	resized, cmd := model.Update(tea.WindowSizeMsg{Width: 260, Height: 42})
	if cmd != nil {
		t.Fatalf("expected resize update to be synchronous")
	}
	model = resized.(tui.Model)
	view := model.View()
	for _, want := range []string{
		"queue dispatch: remote-web-1/tool-web web.discovery dispatched file-outbox",
		"worker queue dispatch transport request:",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected file-outbox dispatch row %q in TUI view:\n%s", want, view)
		}
	}
}

func TestNewTUIModelForWorkspaceShowsRuntimeWorkerHTTPJSONDispatchMetadata(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte("accepted"))
	}))
	defer server.Close()
	now := time.Now().UTC()
	if _, err := RegisterRuntimeWorker(root, RuntimeWorkerArtifact{
		WorkerID:     "remote-web-1",
		Mode:         "remote",
		Status:       "available",
		Endpoint:     server.URL,
		Capabilities: []string{"web.discovery"},
	}, RuntimeWorkerRegistrationOptions{Now: now}); err != nil {
		t.Fatalf("register runtime worker: %v", err)
	}
	claim, err := CreateRuntimeWorkerQueueClaim(root, RuntimeWorkerQueueClaim{
		WorkerID:    "remote-web-1",
		RunID:       "run-dispatch-http",
		ExecutionID: "tool-web",
		Tool:        "web-api-pack",
		Capability:  "web.discovery",
	}, RuntimeWorkerQueueClaimOptions{Now: now, LeaseDuration: 10 * time.Minute})
	if err != nil {
		t.Fatalf("create runtime worker queue claim: %v", err)
	}
	if _, err := ExecuteRuntimeWorkerQueueClaimDispatch(root, claim.Path, RuntimeWorkerQueueDispatchExecutionOptions{
		Execute:                true,
		Now:                    now.Add(time.Minute),
		TransportMode:          "http-json",
		ResultCallbackEndpoint: "http://cb/result",
	}); err != nil {
		t.Fatalf("execute runtime worker queue claim dispatch: %v", err)
	}

	model := NewTUIModelForWorkspace(root).SelectRuntimeTab(tui.RuntimeTabAgentRuntime)
	resized, cmd := model.Update(tea.WindowSizeMsg{Width: 260, Height: 50})
	if cmd != nil {
		t.Fatalf("expected resize update to be synchronous")
	}
	model = resized.(tui.Model)
	view := model.View()
	for _, want := range []string{
		"queue dispatch: remote-web-1/tool-web web.discovery dispatched http-json",
		"worker queue dispatch http endpoint: " + server.URL,
		"worker queue dispatch http response: status=202 body=accepted",
		"worker queue dispatch result callback: http://cb/result",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected http-json dispatch row %q in TUI view:\n%s", want, view)
		}
	}
}

func TestNewTUIModelForWorkspaceShowsRuntimeWorkerTransportOutboxRequests(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Now().UTC()
	dispatch := createFileOutboxRuntimeWorkerQueueDispatch(t, root, now)
	if _, err := AcknowledgeRuntimeWorkerTransportDispatchRequest(root, "remote-web-1", filepath.Base(dispatch.TransportRequestPath), RuntimeWorkerTransportAckOptions{
		Ack: true,
		Now: now.Add(2 * time.Minute),
	}); err != nil {
		t.Fatalf("ack transport request: %v", err)
	}

	model := NewTUIModelForWorkspace(root).SelectRuntimeTab(tui.RuntimeTabAgentRuntime)
	resized, cmd := model.Update(tea.WindowSizeMsg{Width: 260, Height: 42})
	if cmd != nil {
		t.Fatalf("expected resize update to be synchronous")
	}
	model = resized.(tui.Model)
	view := model.View()
	for _, want := range []string{
		"worker transport requests: total=1 pending=0 ack=1 completed=0 failed=0",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected worker transport request row %q in TUI view:\n%s", want, view)
		}
	}
}

func TestNewTUIModelForWorkspaceAcknowledgesWorkerTransportRequestAfterDoubleConfirm(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Now().UTC()
	dispatch := createFileOutboxRuntimeWorkerQueueDispatch(t, root, now)

	model := NewTUIModelForWorkspace(root).SelectRuntimeTab(tui.RuntimeTabAgentRuntime)
	model = selectRuntimeRowContaining(t, model, "> worker transport request:")
	armed, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if cmd != nil {
		t.Fatalf("first x should only arm worker transport request ack confirmation")
	}
	request, err := ReadRuntimeWorkerTransportDispatchRequest(root, "remote-web-1", filepath.Base(dispatch.TransportRequestPath))
	if err != nil {
		t.Fatalf("read request after first x: %v", err)
	}
	if request.Status != "pending" {
		t.Fatalf("first x must not ack request, got %#v", request)
	}
	armedModel, ok := armed.(tui.Model)
	if !ok {
		t.Fatalf("expected TUI model, got %T", armed)
	}
	armedResized, cmd := armedModel.Update(tea.WindowSizeMsg{Width: 420, Height: 90})
	if cmd != nil {
		t.Fatalf("expected armed request resize to be synchronous")
	}
	armedModel = armedResized.(tui.Model)
	for _, want := range []string{
		"action: worker transport request selected",
		"execution status: confirm-required",
		"execution confirm: press x again",
	} {
		if !strings.Contains(armedModel.View(), want) {
			t.Fatalf("expected armed worker transport request view %q:\n%s", want, armedModel.View())
		}
	}

	confirmed, cmd := armedModel.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if cmd == nil {
		t.Fatalf("second x should return worker transport request ack command")
	}
	confirmed, cmd = confirmed.Update(cmd())
	if cmd != nil {
		t.Fatalf("expected worker transport request ack result update to be synchronous")
	}
	request, err = ReadRuntimeWorkerTransportDispatchRequest(root, "remote-web-1", filepath.Base(dispatch.TransportRequestPath))
	if err != nil {
		t.Fatalf("read request after second x: %v", err)
	}
	if request.Status != "acknowledged" || request.AckedAt == "" || request.AckedBy != "remote-web-1" {
		t.Fatalf("expected acknowledged transport request, got %#v", request)
	}
	results, err := ListRuntimeWorkerQueueDispatchResults(root)
	if err != nil {
		t.Fatalf("list queue dispatch results: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("ack shortcut must not execute request or submit results, got %#v", results)
	}
	resized, cmd := confirmed.Update(tea.WindowSizeMsg{Width: 420, Height: 90})
	if cmd != nil {
		t.Fatalf("expected confirmed request resize to be synchronous")
	}
	view := resized.View()
	for _, want := range []string{
		"action output: transport request acknowledged",
		"execution status: acknowledged",
		"worker transport request:",
		"transport request: remote-web-1/tool-web acknowledged file-outbox",
		"worker transport request ack: worker=remote-web-1",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected confirmed worker transport request view %q:\n%s", want, view)
		}
	}
}

func TestNewTUIModelForWorkspaceRunsAcknowledgedWorkerTransportRequestAfterDoubleConfirm(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Now().UTC()
	dispatch := createFileOutboxRuntimeWorkerQueueDispatchWithToolPlan(t, root, now, RuntimeWorkerToolPlanPayload{
		Action:     "discover_assets",
		Runner:     "command",
		RunCommand: []string{"sh", "-c", "printf 'tui-run=%s' \"$OURO_EXECUTION_ID\""},
	})
	if _, err := AcknowledgeRuntimeWorkerTransportDispatchRequest(root, "remote-web-1", filepath.Base(dispatch.TransportRequestPath), RuntimeWorkerTransportAckOptions{
		Ack: true,
		Now: now.Add(2 * time.Minute),
	}); err != nil {
		t.Fatalf("ack transport request: %v", err)
	}

	model := NewTUIModelForWorkspace(root).SelectRuntimeTab(tui.RuntimeTabAgentRuntime)
	model = selectRuntimeRowContaining(t, model, "> worker transport request:")
	armed, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if cmd != nil {
		t.Fatalf("first x should only arm worker transport request run confirmation")
	}
	results, err := ListRuntimeWorkerQueueDispatchResults(root)
	if err != nil {
		t.Fatalf("list queue dispatch results after first x: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("first x must not execute acknowledged request, got %#v", results)
	}
	armedModel := armed.(tui.Model)
	armedResized, cmd := armedModel.Update(tea.WindowSizeMsg{Width: 420, Height: 90})
	if cmd != nil {
		t.Fatalf("expected armed request resize to be synchronous")
	}
	armedModel = armedResized.(tui.Model)
	for _, want := range []string{
		"action: worker transport request selected",
		"execution status: confirm-required",
		"execution confirm: press x again",
	} {
		if !strings.Contains(armedModel.View(), want) {
			t.Fatalf("expected armed worker transport request run view %q:\n%s", want, armedModel.View())
		}
	}

	confirmed, cmd := armedModel.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if cmd == nil {
		t.Fatalf("second x should return worker transport request run command")
	}
	confirmed, cmd = confirmed.Update(cmd())
	if cmd != nil {
		t.Fatalf("expected worker transport request run result update to be synchronous")
	}
	request, err := ReadRuntimeWorkerTransportDispatchRequest(root, "remote-web-1", filepath.Base(dispatch.TransportRequestPath))
	if err != nil {
		t.Fatalf("read request after second x: %v", err)
	}
	if request.Status != "completed" || request.ResultPath == "" {
		t.Fatalf("expected completed transport request with result path, got %#v", request)
	}
	results, err = ListRuntimeWorkerQueueDispatchResults(root)
	if err != nil {
		t.Fatalf("list queue dispatch results after run: %v", err)
	}
	if len(results) != 1 || results[0].Status != "completed" || !strings.Contains(results[0].Output, "tui-run=tool-web") {
		t.Fatalf("expected one completed queue dispatch result with command output, got %#v", results)
	}

	resized, cmd := confirmed.Update(tea.WindowSizeMsg{Width: 420, Height: 90})
	if cmd != nil {
		t.Fatalf("expected confirmed run resize to be synchronous")
	}
	view := resized.View()
	for _, want := range []string{
		"action output: tui-run=tool-web",
		"execution status: completed",
		"worker transport request:",
		"transport request: remote-web-1/tool-web completed file-outbox",
		"worker transport request result:",
		"worker queue dispatch result artifact:",
		"queue result: remote-web-1/tool-web completed",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected confirmed worker transport request run view %q:\n%s", want, view)
		}
	}
}

func TestNewTUIModelForWorkspaceShowsRuntimeWorkerTransportDaemonRuns(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Now().UTC()
	dispatch := createFileOutboxRuntimeWorkerQueueDispatchWithToolPlan(t, root, now, RuntimeWorkerToolPlanPayload{
		Action:     "discover_assets",
		Runner:     "command",
		RunCommand: []string{"sh", "-c", "printf 'daemon=%s' \"$OURO_EXECUTION_ID\""},
	})
	if _, err := AcknowledgeRuntimeWorkerTransportDispatchRequest(root, "remote-web-1", filepath.Base(dispatch.TransportRequestPath), RuntimeWorkerTransportAckOptions{
		Ack: true,
		Now: now.Add(time.Minute),
	}); err != nil {
		t.Fatalf("ack transport request: %v", err)
	}
	if _, err := ExecuteRuntimeWorkerTransportDispatchDaemon(context.Background(), root, "remote-web-1", RuntimeWorkerTransportRunDaemonOptions{
		Execute:           true,
		Now:               now.Add(2 * time.Minute),
		MaxRequests:       1,
		MaxIdleScans:      1,
		PollInterval:      time.Millisecond,
		UseRequestCommand: true,
	}); err != nil {
		t.Fatalf("execute transport daemon: %v", err)
	}

	model := NewTUIModelForWorkspace(root).SelectRuntimeTab(tui.RuntimeTabAgentRuntime)
	resized, cmd := model.Update(tea.WindowSizeMsg{Width: 260, Height: 52})
	if cmd != nil {
		t.Fatalf("expected resize update to be synchronous")
	}
	model = resized.(tui.Model)
	view := model.View()
	for _, want := range []string{
		"worker transport daemons: total=1 completed=1 failed=0 idle=0",
		"transport daemon: remote-web-1 completed processed=1 completed=1 failed=0",
		"worker transport daemon artifact:",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected transport daemon row %q in TUI view:\n%s", want, view)
		}
	}
}

func TestNewTUIModelForWorkspaceShowsRuntimeWorkerTransportDaemonControlRequests(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Now().UTC()
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	if _, err := CreateRuntimeWorkerTransportDaemonControlRequest(root, "remote-web-1", RuntimeWorkerTransportDaemonControlRequest{
		Action:      "stop",
		Reason:      "operator requested maintenance",
		RequestedBy: "alice",
	}, RuntimeWorkerTransportDaemonControlRequestOptions{Now: now}); err != nil {
		t.Fatalf("create daemon control request: %v", err)
	}

	model := NewTUIModelForWorkspace(root).SelectRuntimeTab(tui.RuntimeTabAgentRuntime)
	resized, cmd := model.Update(tea.WindowSizeMsg{Width: 260, Height: 52})
	if cmd != nil {
		t.Fatalf("expected resize update to be synchronous")
	}
	model = resized.(tui.Model)
	view := model.View()
	for _, want := range []string{
		"worker transport daemon controls: total=1 pending=1 stop=1 restart=0",
		"transport daemon control: remote-web-1 stop pending",
		"worker transport daemon control artifact:",
		"daemon control reason: operator requested maintenance",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected transport daemon control row %q in TUI view:\n%s", want, view)
		}
	}
}

func TestNewTUIModelForWorkspaceShowsRuntimeWorkerTransportDaemonControlRequestDetails(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Now().UTC()
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	control, err := CreateRuntimeWorkerTransportDaemonControlRequest(root, "remote-web-1", RuntimeWorkerTransportDaemonControlRequest{
		Action:      "restart",
		Reason:      "operator requested restart",
		RequestedBy: "alice",
	}, RuntimeWorkerTransportDaemonControlRequestOptions{Now: now})
	if err != nil {
		t.Fatalf("create daemon control request: %v", err)
	}

	model := NewTUIModelForWorkspace(root).SelectRuntimeTab(tui.RuntimeTabAgentRuntime)
	model = selectRuntimeRowContaining(t, model, "> worker transport daemon control artifact:")
	model = model.ShowRuntimeActionDetails()
	resized, cmd := model.Update(tea.WindowSizeMsg{Width: 1200, Height: 34})
	if cmd != nil {
		t.Fatalf("expected resize update to be synchronous")
	}
	view := resized.View()
	for _, want := range []string{
		"[Node Inspector]",
		"runtime action: worker-transport-daemon-control-artifact",
		"artifact path: " + control.Path,
		"name: remote-web-1",
		"state: pending",
		"execution mode: metadata-only",
		"control action: restart",
		"mode: read-only",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected daemon control detail %q:\n%s", want, view)
		}
	}
}

func TestNewTUIModelForWorkspaceShowsRuntimeWorkerTransportDaemonSupervisorRuns(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Now().UTC()
	dispatch := createFileOutboxRuntimeWorkerQueueDispatchWithToolPlan(t, root, now, RuntimeWorkerToolPlanPayload{
		Action:     "discover_assets",
		Runner:     "command",
		RunCommand: []string{"sh", "-c", "printf 'supervisor=%s' \"$OURO_EXECUTION_ID\""},
	})
	if _, err := AcknowledgeRuntimeWorkerTransportDispatchRequest(root, "remote-web-1", filepath.Base(dispatch.TransportRequestPath), RuntimeWorkerTransportAckOptions{
		Ack: true,
		Now: now.Add(time.Minute),
	}); err != nil {
		t.Fatalf("ack transport request: %v", err)
	}
	if _, err := CreateRuntimeWorkerTransportDaemonControlRequest(root, "remote-web-1", RuntimeWorkerTransportDaemonControlRequest{
		Action:      "restart",
		Reason:      "reload worker environment",
		RequestedBy: "alice",
	}, RuntimeWorkerTransportDaemonControlRequestOptions{Now: now.Add(2 * time.Minute)}); err != nil {
		t.Fatalf("create daemon restart control request: %v", err)
	}
	if _, err := ExecuteRuntimeWorkerTransportDaemonSupervisor(context.Background(), root, "remote-web-1", RuntimeWorkerTransportDaemonSupervisorOptions{
		Execute:               true,
		Now:                   now.Add(3 * time.Minute),
		MaxRuns:               2,
		MaxRequests:           1,
		MaxIdleScans:          1,
		PollInterval:          time.Millisecond,
		UseRequestCommand:     true,
		EnableControlRequests: true,
	}); err != nil {
		t.Fatalf("execute transport daemon supervisor: %v", err)
	}

	model := NewTUIModelForWorkspace(root).SelectRuntimeTab(tui.RuntimeTabAgentRuntime)
	resized, cmd := model.Update(tea.WindowSizeMsg{Width: 420, Height: 90})
	if cmd != nil {
		t.Fatalf("expected resize update to be synchronous")
	}
	model = resized.(tui.Model)
	view := model.View()
	for _, want := range []string{
		"worker transport daemon supervisors: total=1 completed=1 stopped=0 restart-limit=0 failed=0",
		"transport daemon supervisor: remote-web-1 completed runs=2 restarts=1 processed=1",
		"worker transport daemon supervisor artifact:",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected daemon supervisor row %q in TUI view:\n%s", want, view)
		}
	}
}

func TestNewTUIModelForWorkspaceRequestsRuntimeWorkerTransportDaemonStopAfterDoubleConfirm(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Now().UTC()
	dispatch := createFileOutboxRuntimeWorkerQueueDispatchWithToolPlan(t, root, now, RuntimeWorkerToolPlanPayload{
		Action:     "discover_assets",
		Runner:     "command",
		RunCommand: []string{"sh", "-c", "printf 'daemon=%s' \"$OURO_EXECUTION_ID\""},
	})
	if _, err := AcknowledgeRuntimeWorkerTransportDispatchRequest(root, "remote-web-1", filepath.Base(dispatch.TransportRequestPath), RuntimeWorkerTransportAckOptions{
		Ack: true,
		Now: now.Add(time.Minute),
	}); err != nil {
		t.Fatalf("ack transport request: %v", err)
	}
	daemon, err := ExecuteRuntimeWorkerTransportDispatchDaemon(context.Background(), root, "remote-web-1", RuntimeWorkerTransportRunDaemonOptions{
		Execute:           true,
		Now:               now.Add(2 * time.Minute),
		MaxRequests:       1,
		MaxIdleScans:      1,
		PollInterval:      time.Millisecond,
		UseRequestCommand: true,
	})
	if err != nil {
		t.Fatalf("execute transport daemon: %v", err)
	}

	model := NewTUIModelForWorkspace(root).SelectRuntimeTab(tui.RuntimeTabAgentRuntime)
	model = selectRuntimeRowContaining(t, model, "> worker transport daemon artifact:")
	armed, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if cmd != nil {
		t.Fatalf("first x should only arm daemon control confirmation")
	}
	controls, err := ListRuntimeWorkerTransportDaemonControlRequests(root, "remote-web-1")
	if err != nil {
		t.Fatalf("list daemon controls after first x: %v", err)
	}
	if len(controls) != 0 {
		t.Fatalf("first x must not create daemon control request, got %#v", controls)
	}
	armedModel, ok := armed.(tui.Model)
	if !ok {
		t.Fatalf("expected TUI model, got %T", armed)
	}
	armedResized, cmd := armedModel.Update(tea.WindowSizeMsg{Width: 420, Height: 90})
	if cmd != nil {
		t.Fatalf("expected armed daemon control resize to be synchronous")
	}
	armedModel, ok = armedResized.(tui.Model)
	if !ok {
		t.Fatalf("expected TUI model after resize, got %T", armedResized)
	}
	armedView := armedModel.View()
	for _, want := range []string{
		"action: worker transport daemon artifact selected",
		"execution status: confirm-required",
		"execution confirm: press x again",
	} {
		if !strings.Contains(armedView, want) {
			t.Fatalf("expected armed daemon control view %q:\n%s", want, armedView)
		}
	}

	confirmed, cmd := armedModel.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if cmd == nil {
		t.Fatalf("second x should return daemon control request command")
	}
	confirmed, cmd = confirmed.Update(cmd())
	if cmd != nil {
		t.Fatalf("expected daemon control result update to be synchronous")
	}
	controls, err = ListRuntimeWorkerTransportDaemonControlRequests(root, "remote-web-1")
	if err != nil {
		t.Fatalf("list daemon controls after second x: %v", err)
	}
	if len(controls) != 1 || controls[0].Status != "pending" || controls[0].Action != "stop" || controls[0].AppliedAt != "" {
		t.Fatalf("expected one pending daemon stop control request, got %#v", controls)
	}
	if controls[0].RequestedBy != "tui" || !strings.Contains(controls[0].Reason, "TUI") {
		t.Fatalf("expected TUI audit metadata on daemon stop request, got %#v", controls[0])
	}
	resized, cmd := confirmed.Update(tea.WindowSizeMsg{Width: 300, Height: 70})
	if cmd != nil {
		t.Fatalf("expected resize update to be synchronous")
	}
	view := resized.View()
	for _, want := range []string{
		"action output: stop requested",
		"execution status: pending",
		"execution path:",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected confirmed daemon control view %q:\n%s", want, view)
		}
	}
	readDaemon, err := ReadRuntimeWorkerTransportDaemonRun(root, "remote-web-1", filepath.Base(daemon.Path))
	if err != nil {
		t.Fatalf("read daemon run after TUI stop request: %v", err)
	}
	if readDaemon.Status != "completed" || readDaemon.ControlAction != "" {
		t.Fatalf("TUI stop request must not mutate existing daemon run artifact, got %#v", readDaemon)
	}
}

func TestNewTUIModelForWorkspaceRequestsRuntimeWorkerTransportDaemonRestartAfterDoubleConfirm(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Now().UTC()
	dispatch := createFileOutboxRuntimeWorkerQueueDispatchWithToolPlan(t, root, now, RuntimeWorkerToolPlanPayload{
		Action:     "discover_assets",
		Runner:     "command",
		RunCommand: []string{"sh", "-c", "printf 'daemon=%s' \"$OURO_EXECUTION_ID\""},
	})
	if _, err := AcknowledgeRuntimeWorkerTransportDispatchRequest(root, "remote-web-1", filepath.Base(dispatch.TransportRequestPath), RuntimeWorkerTransportAckOptions{
		Ack: true,
		Now: now.Add(time.Minute),
	}); err != nil {
		t.Fatalf("ack transport request: %v", err)
	}
	daemon, err := ExecuteRuntimeWorkerTransportDispatchDaemon(context.Background(), root, "remote-web-1", RuntimeWorkerTransportRunDaemonOptions{
		Execute:           true,
		Now:               now.Add(2 * time.Minute),
		MaxRequests:       1,
		MaxIdleScans:      1,
		PollInterval:      time.Millisecond,
		UseRequestCommand: true,
	})
	if err != nil {
		t.Fatalf("execute transport daemon: %v", err)
	}

	model := NewTUIModelForWorkspace(root).SelectRuntimeTab(tui.RuntimeTabAgentRuntime)
	model = selectRuntimeRowContaining(t, model, "> worker transport daemon artifact:")
	armed, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'R'}})
	if cmd != nil {
		t.Fatalf("first R should only arm daemon restart confirmation")
	}
	controls, err := ListRuntimeWorkerTransportDaemonControlRequests(root, "remote-web-1")
	if err != nil {
		t.Fatalf("list daemon controls after first R: %v", err)
	}
	if len(controls) != 0 {
		t.Fatalf("first R must not create daemon restart control request, got %#v", controls)
	}
	armedModel, ok := armed.(tui.Model)
	if !ok {
		t.Fatalf("expected TUI model, got %T", armed)
	}
	armedResized, cmd := armedModel.Update(tea.WindowSizeMsg{Width: 420, Height: 90})
	if cmd != nil {
		t.Fatalf("expected armed daemon restart resize to be synchronous")
	}
	armedModel, ok = armedResized.(tui.Model)
	if !ok {
		t.Fatalf("expected TUI model after resize, got %T", armedResized)
	}
	armedView := armedModel.View()
	for _, want := range []string{
		"action: worker transport daemon artifact selected",
		"control action: restart",
		"execution status: confirm-required",
		"execution confirm: press R again",
	} {
		if !strings.Contains(armedView, want) {
			t.Fatalf("expected armed daemon restart view %q:\n%s", want, armedView)
		}
	}

	confirmed, cmd := armedModel.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'R'}})
	if cmd == nil {
		t.Fatalf("second R should return daemon restart control request command")
	}
	confirmed, cmd = confirmed.Update(cmd())
	if cmd != nil {
		t.Fatalf("expected daemon restart result update to be synchronous")
	}
	controls, err = ListRuntimeWorkerTransportDaemonControlRequests(root, "remote-web-1")
	if err != nil {
		t.Fatalf("list daemon controls after second R: %v", err)
	}
	if len(controls) != 1 || controls[0].Status != "pending" || controls[0].Action != "restart" || controls[0].AppliedAt != "" {
		t.Fatalf("expected one pending daemon restart control request, got %#v", controls)
	}
	if controls[0].RequestedBy != "tui" || !strings.Contains(controls[0].Reason, "TUI") {
		t.Fatalf("expected TUI audit metadata on daemon restart request, got %#v", controls[0])
	}
	resized, cmd := confirmed.Update(tea.WindowSizeMsg{Width: 300, Height: 70})
	if cmd != nil {
		t.Fatalf("expected resize update to be synchronous")
	}
	view := resized.View()
	for _, want := range []string{
		"control action: restart",
		"action output: restart requested",
		"execution status: pending",
		"execution path:",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected confirmed daemon restart view %q:\n%s", want, view)
		}
	}
	readDaemon, err := ReadRuntimeWorkerTransportDaemonRun(root, "remote-web-1", filepath.Base(daemon.Path))
	if err != nil {
		t.Fatalf("read daemon run after TUI restart request: %v", err)
	}
	if readDaemon.Status != "completed" || readDaemon.ControlAction != "" {
		t.Fatalf("TUI restart request must not mutate existing daemon run artifact, got %#v", readDaemon)
	}
}

func TestNewTUIModelForWorkspaceShowsRuntimeWorkerQueueDispatchResults(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Now().UTC()
	dispatch := createDispatchedRuntimeWorkerQueueClaim(t, root, now)
	if _, err := SubmitRuntimeWorkerQueueDispatchResult(root, dispatch.Path, RuntimeWorkerQueueDispatchResult{
		Status: "completed",
		Output: "remote worker completed discovery",
	}, RuntimeWorkerQueueDispatchResultOptions{Now: now.Add(2 * time.Minute)}); err != nil {
		t.Fatalf("submit runtime worker queue dispatch result: %v", err)
	}

	model := NewTUIModelForWorkspace(root).SelectRuntimeTab(tui.RuntimeTabAgentRuntime)
	resized, cmd := model.Update(tea.WindowSizeMsg{Width: 260, Height: 42})
	if cmd != nil {
		t.Fatalf("expected resize update to be synchronous")
	}
	model = resized.(tui.Model)
	view := model.View()
	for _, want := range []string{
		"worker queue dispatch results: total=1 completed=1 failed=0",
		"queue result: remote-web-1/tool-web completed",
		"worker queue dispatch result artifact:",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected queue dispatch result row %q in TUI view:\n%s", want, view)
		}
	}
}

func TestNewTUIModelForWorkspaceShowsWorkerDispatchRetryPlanArtifacts(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Now().UTC()
	marker := filepath.Join(t.TempDir(), "worker-dispatch-retry-tui-ran")
	dispatch := createFileOutboxRuntimeWorkerQueueDispatchWithToolPlan(t, root, now, RuntimeWorkerToolPlanPayload{
		Action:     "discover_assets",
		Runner:     "command",
		Parser:     "web_api_parser",
		RunCommand: []string{"sh", "-c", "touch " + marker},
	})
	if _, err := SubmitRuntimeWorkerQueueDispatchResult(root, filepath.Base(dispatch.Path), RuntimeWorkerQueueDispatchResult{
		Status: "failed",
		Output: "partial worker output",
		Error:  "exit status 23",
	}, RuntimeWorkerQueueDispatchResultOptions{Now: now.Add(time.Minute)}); err != nil {
		t.Fatalf("submit failed runtime worker queue dispatch result: %v", err)
	}
	plan, err := CreateRuntimeWorkerDispatchRetryPlan(root, now.Add(2*time.Minute))
	if err != nil {
		t.Fatalf("create worker dispatch retry plan: %v", err)
	}

	model := NewTUIModelForWorkspace(root).SelectRuntimeTab(tui.RuntimeTabAgentRuntime)
	resized, cmd := model.Update(tea.WindowSizeMsg{Width: 360, Height: 60})
	if cmd != nil {
		t.Fatalf("expected resize update to be synchronous")
	}
	view := resized.View()
	for _, want := range []string{
		"worker dispatch retry plans: total=1 ready=1 skipped=0",
		"worker dispatch retry plan artifact:",
		"dispatch retry candidate: remote-web-1/tool-web ready metadata-only",
		"error=exit status 23",
		filepath.Base(plan.Path),
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected worker dispatch retry plan row %q:\n%s", want, view)
		}
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("TUI worker dispatch retry plan must not execute retry command, marker stat err=%v", err)
	}
}

func TestNewTUIModelForWorkspaceShowsWorkerDispatchRetryDispatchArtifacts(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Now().UTC()
	marker := filepath.Join(t.TempDir(), "worker-dispatch-retry-dispatch-tui-ran")
	dispatch := createFileOutboxRuntimeWorkerQueueDispatchWithToolPlan(t, root, now, RuntimeWorkerToolPlanPayload{
		Action:     "discover_assets",
		Runner:     "command",
		Parser:     "web_api_parser",
		RunCommand: []string{"sh", "-c", "touch " + marker},
	})
	if _, err := SubmitRuntimeWorkerQueueDispatchResult(root, filepath.Base(dispatch.Path), RuntimeWorkerQueueDispatchResult{
		Status: "failed",
		Error:  "exit status 23",
	}, RuntimeWorkerQueueDispatchResultOptions{Now: now.Add(time.Minute)}); err != nil {
		t.Fatalf("submit failed runtime worker queue dispatch result: %v", err)
	}
	plan, err := CreateRuntimeWorkerDispatchRetryPlan(root, now.Add(2*time.Minute))
	if err != nil {
		t.Fatalf("create worker dispatch retry plan: %v", err)
	}
	retryDispatch, err := CreateRuntimeWorkerDispatchRetryDispatch(root, filepath.Base(plan.Path), now.Add(3*time.Minute))
	if err != nil {
		t.Fatalf("create worker dispatch retry dispatch: %v", err)
	}

	model := NewTUIModelForWorkspace(root).SelectRuntimeTab(tui.RuntimeTabAgentRuntime)
	resized, cmd := model.Update(tea.WindowSizeMsg{Width: 360, Height: 70})
	if cmd != nil {
		t.Fatalf("expected resize update to be synchronous")
	}
	view := resized.View()
	for _, want := range []string{
		"worker dispatch retry dispatches: total=1 ready=1 skipped=0",
		"worker dispatch retry dispatch artifact:",
		"dispatch retry dispatch:",
		"retry_plan=" + filepath.Base(plan.Path),
		filepath.Base(retryDispatch.Path),
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected worker dispatch retry dispatch row %q:\n%s", want, view)
		}
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("TUI worker dispatch retry dispatch must not execute retry command, marker stat err=%v", err)
	}
}

func TestNewTUIModelForWorkspaceShowsWorkerDispatchRetryDispatchExecutionArtifacts(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Now().UTC()
	marker := filepath.Join(t.TempDir(), "worker-dispatch-retry-dispatch-execution-tui-ran")
	dispatch := createFileOutboxRuntimeWorkerQueueDispatchWithToolPlan(t, root, now, RuntimeWorkerToolPlanPayload{
		Action:     "discover_assets",
		Runner:     "command",
		Parser:     "web_api_parser",
		RunCommand: []string{"sh", "-c", "touch " + marker},
	})
	if _, err := SubmitRuntimeWorkerQueueDispatchResult(root, filepath.Base(dispatch.Path), RuntimeWorkerQueueDispatchResult{
		Status: "failed",
		Error:  "exit status 23",
	}, RuntimeWorkerQueueDispatchResultOptions{Now: now.Add(time.Minute)}); err != nil {
		t.Fatalf("submit failed runtime worker queue dispatch result: %v", err)
	}
	plan, err := CreateRuntimeWorkerDispatchRetryPlan(root, now.Add(2*time.Minute))
	if err != nil {
		t.Fatalf("create worker dispatch retry plan: %v", err)
	}
	retryDispatch, err := CreateRuntimeWorkerDispatchRetryDispatch(root, filepath.Base(plan.Path), now.Add(3*time.Minute))
	if err != nil {
		t.Fatalf("create worker dispatch retry dispatch: %v", err)
	}
	execution, err := ExecuteRuntimeWorkerDispatchRetryDispatch(root, filepath.Base(retryDispatch.Path), RuntimeWorkerDispatchRetryDispatchExecutionOptions{
		Execute: true,
		Now:     now.Add(4 * time.Minute),
	})
	if err != nil {
		t.Fatalf("execute worker dispatch retry dispatch: %v", err)
	}

	model := NewTUIModelForWorkspace(root).SelectRuntimeTab(tui.RuntimeTabAgentRuntime)
	resized, cmd := model.Update(tea.WindowSizeMsg{Width: 360, Height: 80})
	if cmd != nil {
		t.Fatalf("expected resize update to be synchronous")
	}
	view := resized.View()
	for _, want := range []string{
		"worker dispatch retry executions: total=1 claimed=1 dispatched=0 skipped=0 failed=0",
		"worker dispatch retry execution artifact:",
		"dispatch retry execution:",
		"worker dispatch retry execution result:",
		"dispatch retry execution retry_claim=",
		filepath.Base(execution.Path),
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected worker dispatch retry execution row %q:\n%s", want, view)
		}
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("TUI worker dispatch retry execution must not execute retry command, marker stat err=%v", err)
	}
}

func TestNewTUIModelForWorkspaceShowsWorkerDispatchRetryDispatchExecutionHandoff(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Now().UTC()
	marker := filepath.Join(t.TempDir(), "worker-dispatch-retry-dispatch-handoff-tui-ran")
	dispatch := createFileOutboxRuntimeWorkerQueueDispatchWithToolPlan(t, root, now, RuntimeWorkerToolPlanPayload{
		Action:     "discover_assets",
		Runner:     "command",
		Parser:     "web_api_parser",
		RunCommand: []string{"sh", "-c", "touch " + marker},
	})
	if _, err := SubmitRuntimeWorkerQueueDispatchResult(root, filepath.Base(dispatch.Path), RuntimeWorkerQueueDispatchResult{
		Status: "failed",
		Error:  "exit status 23",
	}, RuntimeWorkerQueueDispatchResultOptions{Now: now.Add(time.Minute)}); err != nil {
		t.Fatalf("submit failed runtime worker queue dispatch result: %v", err)
	}
	plan, err := CreateRuntimeWorkerDispatchRetryPlan(root, now.Add(2*time.Minute))
	if err != nil {
		t.Fatalf("create worker dispatch retry plan: %v", err)
	}
	retryDispatch, err := CreateRuntimeWorkerDispatchRetryDispatch(root, filepath.Base(plan.Path), now.Add(3*time.Minute))
	if err != nil {
		t.Fatalf("create worker dispatch retry dispatch: %v", err)
	}
	if _, err := ExecuteRuntimeWorkerDispatchRetryDispatch(root, filepath.Base(retryDispatch.Path), RuntimeWorkerDispatchRetryDispatchExecutionOptions{
		Execute:        true,
		Now:            now.Add(4 * time.Minute),
		DispatchClaims: true,
		TransportMode:  "file-outbox",
	}); err != nil {
		t.Fatalf("execute worker dispatch retry dispatch handoff: %v", err)
	}

	model := NewTUIModelForWorkspace(root).SelectRuntimeTab(tui.RuntimeTabAgentRuntime)
	resized, cmd := model.Update(tea.WindowSizeMsg{Width: 380, Height: 90})
	if cmd != nil {
		t.Fatalf("expected resize update to be synchronous")
	}
	view := resized.View()
	for _, want := range []string{
		"worker dispatch retry executions: total=1 claimed=1 dispatched=1 skipped=0 failed=0",
		"worker dispatch retry execution artifact:",
		"dispatched=1",
		"worker dispatch retry execution result:",
		"dispatch retry handoff status=dispatched transport=file-outbox",
		"dispatch retry handoff queue_dispatch=worker-queue-dispatch-execution",
		"dispatch retry handoff request=worker-transport-dispatch-request",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected worker dispatch retry handoff row %q:\n%s", want, view)
		}
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("TUI worker dispatch retry handoff must not execute retry command, marker stat err=%v", err)
	}
}

func TestNewTUIModelForWorkspaceShowsStaleWorkerLeaseArtifacts(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	oldHeartbeat := time.Now().UTC().Add(-defaultWorkerLeaseStaleAfter - time.Minute).Format(time.RFC3339Nano)
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

	model := NewTUIModelForWorkspace(root).SelectRuntimeTab(tui.RuntimeTabAgentRuntime)
	resized, cmd := model.Update(tea.WindowSizeMsg{Width: 260, Height: 42})
	if cmd != nil {
		t.Fatalf("expected resize update to be synchronous")
	}
	model = resized.(tui.Model)
	view := model.View()
	for _, want := range []string{
		"worker leases: total=1 active=0 stale=1 completed=0 failed=0",
		"worker lease web-api-pack web.discovery stale worker=local",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected stale worker lease row %q in TUI view:\n%s", want, view)
		}
	}
}

func TestNewTUIModelForWorkspaceShowsWorkerLeaseReclaimPlan(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	oldHeartbeat := time.Now().UTC().Add(-defaultWorkerLeaseStaleAfter - time.Minute).Format(time.RFC3339Nano)
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

	model := NewTUIModelForWorkspace(root).SelectRuntimeTab(tui.RuntimeTabAgentRuntime)
	resized, cmd := model.Update(tea.WindowSizeMsg{Width: 260, Height: 42})
	if cmd != nil {
		t.Fatalf("expected resize update to be synchronous")
	}
	model = resized.(tui.Model)
	view := model.View()
	for _, want := range []string{
		"worker lease reclaims: ready=1 skipped=0",
		"worker lease reclaim web-api-pack web.discovery: ready mode=metadata-only",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected worker lease reclaim row %q in TUI view:\n%s", want, view)
		}
	}
}

func TestNewTUIModelForWorkspaceShowsWorkerLeaseReclaimDispatchArtifacts(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	now := time.Now().UTC()
	oldHeartbeat := now.Add(-defaultWorkerLeaseStaleAfter - time.Minute).Format(time.RFC3339Nano)
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
	dispatch, err := CreateWorkerLeaseReclaimDispatch(root, now, defaultWorkerLeaseStaleAfter)
	if err != nil {
		t.Fatalf("create worker lease reclaim dispatch: %v", err)
	}

	model := NewTUIModelForWorkspace(root).SelectRuntimeTab(tui.RuntimeTabAgentRuntime)
	resized, cmd := model.Update(tea.WindowSizeMsg{Width: 260, Height: 42})
	if cmd != nil {
		t.Fatalf("expected resize update to be synchronous")
	}
	model = resized.(tui.Model)
	view := model.View()
	for _, want := range []string{
		"worker lease reclaim artifacts: total=1",
		"worker lease reclaim artifact:",
		"reclaim artifact: dry-run metadata-only ready=1 skipped=0",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected worker lease reclaim artifact row %q in TUI view:\n%s", want, view)
		}
	}
	if dispatch.Path == "" {
		t.Fatalf("expected reclaim dispatch path")
	}
}

func TestNewTUIModelForWorkspaceShowsWorkerLeaseReclaimExecutionArtifacts(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	now := time.Now().UTC()
	oldHeartbeat := now.Add(-defaultWorkerLeaseStaleAfter - time.Minute).Format(time.RFC3339Nano)
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
	dispatch, err := CreateWorkerLeaseReclaimDispatch(root, now, defaultWorkerLeaseStaleAfter)
	if err != nil {
		t.Fatalf("create worker lease reclaim dispatch: %v", err)
	}
	if _, err := ExecuteWorkerLeaseReclaimDispatch(root, dispatch.Path, WorkerLeaseReclaimExecutionOptions{
		Execute:    true,
		Now:        now,
		StaleAfter: defaultWorkerLeaseStaleAfter,
	}); err != nil {
		t.Fatalf("execute worker lease reclaim dispatch: %v", err)
	}

	model := NewTUIModelForWorkspace(root).SelectRuntimeTab(tui.RuntimeTabAgentRuntime)
	resized, cmd := model.Update(tea.WindowSizeMsg{Width: 260, Height: 42})
	if cmd != nil {
		t.Fatalf("expected resize update to be synchronous")
	}
	model = resized.(tui.Model)
	view := model.View()
	for _, want := range []string{
		"worker lease web-api-pack web.discovery reclaimed worker=local",
		"worker lease reclaim execution artifacts: total=1",
		"reclaim execution: completed execute results=1 skipped=0",
		"reclaim result: run-stale/tool-stale reclaimed worker=local",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected worker lease reclaim execution row %q in TUI view:\n%s", want, view)
		}
	}
}

func TestNewTUIModelForWorkspaceShowsSafetyBypassWarnings(t *testing.T) {
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
	authorization := `version: 1
safety_bypasses:
  - control: scope.target_policy
    mode: disabled
    authorized_by: alice
    reason: false positive target boundary
    ticket: ENG-42
    expires: 2999-06-30
`
	if err := os.WriteFile(filepath.Join(root, "authorization.yaml"), []byte(authorization), 0o644); err != nil {
		t.Fatalf("write authorization: %v", err)
	}

	model := NewTUIModelForWorkspace(root).SelectRuntimeTab(tui.RuntimeTabToolchainHealth)
	resized, cmd := model.Update(tea.WindowSizeMsg{Width: 720, Height: 30})
	if cmd != nil {
		t.Fatalf("expected resize update to be synchronous")
	}
	view := resized.View()
	for _, want := range []string{
		"workspace safety: warnings=2",
		"workspace safety strict gate: ready",
		"safety warning: scope.target_policy is disabled",
		"safety warning: scope.target_policy disabled authorized by alice; reason: false positive target boundary; ticket: ENG-42; expires: 2999-06-30",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected TUI safety bypass warning %q:\n%s", want, view)
		}
	}
}

func TestNewTUIModelForWorkspaceShowsStrictSafetyGateBlocked(t *testing.T) {
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

	model := NewTUIModelForWorkspace(root).SelectRuntimeTab(tui.RuntimeTabToolchainHealth)
	resized, cmd := model.Update(tea.WindowSizeMsg{Width: 720, Height: 30})
	if cmd != nil {
		t.Fatalf("expected resize update to be synchronous")
	}
	view := resized.View()
	for _, want := range []string{
		"workspace safety: warnings=4",
		"workspace safety strict gate: blocked errors=2",
		"safety strict error: missing required safety bypass authorization for scope.target_policy disabled",
		"safety strict error: missing required safety bypass authorization for execution.isolation disabled",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected TUI strict safety gate row %q:\n%s", want, view)
		}
	}
}

func TestNewTUIModelForWorkspaceShowsExpiredSafetyAuthorization(t *testing.T) {
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
	authorization := `version: 1
safety_bypasses:
  - control: scope.target_policy
    mode: disabled
    authorized_by: alice
    reason: authorized target-boundary false positive
    ticket: ENG-999
    expires: "2000-01-01"
`
	if err := os.WriteFile(filepath.Join(root, "authorization.yaml"), []byte(authorization), 0o644); err != nil {
		t.Fatalf("write authorization: %v", err)
	}

	model := NewTUIModelForWorkspace(root).SelectRuntimeTab(tui.RuntimeTabToolchainHealth)
	resized, cmd := model.Update(tea.WindowSizeMsg{Width: 720, Height: 30})
	if cmd != nil {
		t.Fatalf("expected resize update to be synchronous")
	}
	view := resized.View()
	for _, want := range []string{
		"workspace safety: warnings=2",
		"workspace safety strict gate: blocked errors=1",
		"safety warning: expired safety bypass authorization for scope.target_policy disabled",
		"safety strict error: expired required safety bypass authorization for scope.target_policy disabled",
		"expired on 2000-01-01",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected expired safety authorization row %q:\n%s", want, view)
		}
	}
}

func TestNewTUIModelForWorkspaceClearsSafetyRowsWhenBoundariesRestored(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	disabledScope := `version: 1
scope:
  target_policy: disabled
  targets: []
  exclusions: []
execution:
  isolation: disabled
`
	if err := os.WriteFile(filepath.Join(root, "scope.yaml"), []byte(disabledScope), 0o644); err != nil {
		t.Fatalf("write disabled scope: %v", err)
	}
	disabledModel := NewTUIModelForWorkspace(root).SelectRuntimeTab(tui.RuntimeTabToolchainHealth)
	disabledResized, cmd := disabledModel.Update(tea.WindowSizeMsg{Width: 720, Height: 30})
	if cmd != nil {
		t.Fatalf("expected disabled resize update to be synchronous")
	}
	disabledView := disabledResized.View()
	if !strings.Contains(disabledView, "workspace safety strict gate: blocked") {
		t.Fatalf("expected disabled scope to show safety rows before restore:\n%s", disabledView)
	}

	restoredScope := `version: 1
scope:
  target_policy: enforce
  targets: []
  exclusions: []
execution:
  isolation: process
`
	if err := os.WriteFile(filepath.Join(root, "scope.yaml"), []byte(restoredScope), 0o644); err != nil {
		t.Fatalf("write restored scope: %v", err)
	}
	restoredModel := NewTUIModelForWorkspace(root).SelectRuntimeTab(tui.RuntimeTabToolchainHealth)
	resized, cmd := restoredModel.Update(tea.WindowSizeMsg{Width: 720, Height: 30})
	if cmd != nil {
		t.Fatalf("expected resize update to be synchronous")
	}
	view := resized.View()
	for _, unwanted := range []string{
		"workspace safety:",
		"workspace safety strict gate:",
		"safety warning:",
		"safety strict error:",
		"safety error:",
	} {
		if strings.Contains(view, unwanted) {
			t.Fatalf("restored safety boundaries should not show %q rows:\n%s", unwanted, view)
		}
	}
}

func TestNewTUIModelForWorkspaceShowsStrictSafetyGateDetails(t *testing.T) {
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

	model := NewTUIModelForWorkspace(root).SelectRuntimeTab(tui.RuntimeTabToolchainHealth)
	model = selectRuntimeRowContaining(t, model, "> workspace safety strict gate: blocked errors=2")
	model = model.ShowRuntimeActionDetails()
	resized, cmd := model.Update(tea.WindowSizeMsg{Width: 720, Height: 30})
	if cmd != nil {
		t.Fatalf("expected resize update to be synchronous")
	}
	view := resized.View()
	for _, want := range []string{
		"[Node Inspector]",
		"runtime action: workspace-safety-strict-gate",
		"state: blocked",
		"execution mode: strict",
		"failed: 2",
		"reason: missing required safety bypass authorization",
		"mode: read-only",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected strict safety gate details %q:\n%s", want, view)
		}
	}
}

func TestNewTUIModelForWorkspaceShowsSafetyWarningDetails(t *testing.T) {
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
	authorization := `version: 1
safety_bypasses:
  - control: scope.target_policy
    mode: disabled
    authorized_by: alice
    reason: false positive target boundary
    ticket: ENG-42
    expires: 2999-06-30
`
	if err := os.WriteFile(filepath.Join(root, "authorization.yaml"), []byte(authorization), 0o644); err != nil {
		t.Fatalf("write authorization: %v", err)
	}

	model := NewTUIModelForWorkspace(root).SelectRuntimeTab(tui.RuntimeTabToolchainHealth)
	model = selectRuntimeRowContaining(t, model, "> safety warning: scope.target_policy disabled authorized by alice")
	model = model.ShowRuntimeActionDetails()
	resized, cmd := model.Update(tea.WindowSizeMsg{Width: 720, Height: 30})
	if cmd != nil {
		t.Fatalf("expected resize update to be synchronous")
	}
	view := resized.View()
	for _, want := range []string{
		"[Node Inspector]",
		"runtime action: workspace-safety-warning",
		"state: warning",
		"reason: scope.target_policy disabled authorized by alice; reason: false positive target boundary; ticket: ENG-42; expires: 2999-06-30",
		"mode: read-only",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected safety warning details %q:\n%s", want, view)
		}
	}
}

func TestNewTUIModelForWorkspaceShowsSafetyErrorDetails(t *testing.T) {
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
	authorization := `version: 1
safety_bypasses:
  - control: unsupported.control
    mode: disabled
    authorized_by: alice
    reason: invalid control fixture
`
	if err := os.WriteFile(filepath.Join(root, "authorization.yaml"), []byte(authorization), 0o644); err != nil {
		t.Fatalf("write invalid authorization: %v", err)
	}

	model := NewTUIModelForWorkspace(root).SelectRuntimeTab(tui.RuntimeTabToolchainHealth)
	model = selectRuntimeRowContaining(t, model, "> safety error: invalid authorization.yaml:")
	model = model.ShowRuntimeActionDetails()
	resized, cmd := model.Update(tea.WindowSizeMsg{Width: 720, Height: 30})
	if cmd != nil {
		t.Fatalf("expected resize update to be synchronous")
	}
	view := resized.View()
	for _, want := range []string{
		"[Node Inspector]",
		"runtime action: workspace-safety-error",
		"state: error",
		"reason: invalid authorization.yaml:",
		"mode: read-only",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected safety error details %q:\n%s", want, view)
		}
	}
}

func TestNewTUIModelForWorkspaceShowsInstallRequestStatusCounts(t *testing.T) {
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
  - name: missing-httpx
    kind: binary
    install_command: [ouro-toolchain, install, httpx]
  - name: missing-nuclei
    kind: binary
    install_command: [ouro-toolchain, install, nuclei]
  - name: missing-naabu
    kind: binary
    install_command: [ouro-toolchain, install, naabu]
`
	if err := os.WriteFile(filepath.Join(root, "toolchain", "web-api-pack", "manifest.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	approved, err := CreateToolchainInstallRequest(root, "web-api-pack", "missing-httpx")
	if err != nil {
		t.Fatalf("create approved request: %v", err)
	}
	denied, err := CreateToolchainInstallRequest(root, "web-api-pack", "missing-nuclei")
	if err != nil {
		t.Fatalf("create denied request: %v", err)
	}
	if _, err := CreateToolchainInstallRequest(root, "web-api-pack", "missing-naabu"); err != nil {
		t.Fatalf("create pending request: %v", err)
	}
	if _, err := UpdateToolchainInstallRequestStatus(root, approved.Path, "approved"); err != nil {
		t.Fatalf("approve request: %v", err)
	}
	if _, err := UpdateToolchainInstallRequestStatus(root, denied.Path, "denied"); err != nil {
		t.Fatalf("deny request: %v", err)
	}

	model := NewTUIModelForWorkspace(root).SelectRuntimeTab(tui.RuntimeTabToolchainHealth)
	resized, cmd := model.Update(tea.WindowSizeMsg{Width: 260, Height: 30})
	if cmd != nil {
		t.Fatalf("expected resize update to be synchronous")
	}
	view := resized.View()
	for _, want := range []string{
		"install requests: total=3 pending=1 approved=1 denied=1",
		"requests:p1/a1/d1",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected TUI install request counts %q:\n%s", want, view)
		}
	}
}

func TestNewTUIModelForWorkspaceShowsInstallDispatchPlan(t *testing.T) {
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
  - name: missing-httpx
    kind: binary
    install_command: [ouro-toolchain, install, httpx]
  - name: missing-nuclei
    kind: binary
    install_command: [ouro-toolchain, install, nuclei]
  - name: missing-naabu
    kind: binary
    install_command: [ouro-toolchain, install, naabu]
`
	if err := os.WriteFile(filepath.Join(root, "toolchain", "web-api-pack", "manifest.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	approved, err := CreateToolchainInstallRequest(root, "web-api-pack", "missing-httpx")
	if err != nil {
		t.Fatalf("create approved request: %v", err)
	}
	denied, err := CreateToolchainInstallRequest(root, "web-api-pack", "missing-nuclei")
	if err != nil {
		t.Fatalf("create denied request: %v", err)
	}
	if _, err := CreateToolchainInstallRequest(root, "web-api-pack", "missing-naabu"); err != nil {
		t.Fatalf("create pending request: %v", err)
	}
	if _, err := UpdateToolchainInstallRequestStatus(root, approved.Path, "approved"); err != nil {
		t.Fatalf("approve request: %v", err)
	}
	if _, err := UpdateToolchainInstallRequestStatus(root, denied.Path, "denied"); err != nil {
		t.Fatalf("deny request: %v", err)
	}

	model := NewTUIModelForWorkspace(root).SelectRuntimeTab(tui.RuntimeTabToolchainHealth)
	resized, cmd := model.Update(tea.WindowSizeMsg{Width: 260, Height: 30})
	if cmd != nil {
		t.Fatalf("expected resize update to be synchronous")
	}
	view := resized.View()
	for _, want := range []string{
		"install dispatch: ready=1 skipped=2",
		"dispatch web-api-pack/missing-httpx: ready mode=metadata-only",
		"dispatch command: ouro-toolchain install httpx",
		"dispatch skip web-api-pack/missing-naabu: not-approved: pending",
		"dispatch skip web-api-pack/missing-nuclei: not-approved: denied",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected TUI dispatch plan %q:\n%s", want, view)
		}
	}
}

func TestNewTUIModelForWorkspaceShowsInstallDispatchDependencies(t *testing.T) {
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
  - name: a-httpx
    kind: binary
    depends_on:
      - z-runtime
    install_command: [ouro-toolchain, install, httpx]
  - name: z-runtime
    kind: binary
    install_command: [ouro-toolchain, install, runtime]
`
	if err := os.WriteFile(filepath.Join(root, "toolchain", "web-api-pack", "manifest.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	dependent, err := CreateToolchainInstallRequest(root, "web-api-pack", "a-httpx")
	if err != nil {
		t.Fatalf("create dependent request: %v", err)
	}
	prerequisite, err := CreateToolchainInstallRequest(root, "web-api-pack", "z-runtime")
	if err != nil {
		t.Fatalf("create prerequisite request: %v", err)
	}
	if _, err := UpdateToolchainInstallRequestStatus(root, dependent.Path, "approved"); err != nil {
		t.Fatalf("approve dependent request: %v", err)
	}
	if _, err := UpdateToolchainInstallRequestStatus(root, prerequisite.Path, "approved"); err != nil {
		t.Fatalf("approve prerequisite request: %v", err)
	}

	model := NewTUIModelForWorkspace(root).SelectRuntimeTab(tui.RuntimeTabToolchainHealth)
	resized, cmd := model.Update(tea.WindowSizeMsg{Width: 260, Height: 30})
	if cmd != nil {
		t.Fatalf("expected resize update to be synchronous")
	}
	view := resized.View()
	runtimeIndex := strings.Index(view, "dispatch web-api-pack/z-runtime")
	httpxIndex := strings.Index(view, "dispatch web-api-pack/a-httpx")
	if runtimeIndex < 0 || httpxIndex < 0 || runtimeIndex > httpxIndex {
		t.Fatalf("expected prerequisite dispatch before dependent:\n%s", view)
	}
	if !strings.Contains(view, "dispatch depends_on web-api-pack/a-httpx: z-runtime") {
		t.Fatalf("expected TUI dispatch dependency metadata:\n%s", view)
	}
}

func TestNewTUIModelForWorkspaceShowsInstallDispatchDependencySkip(t *testing.T) {
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
  - name: a-httpx
    kind: binary
    depends_on:
      - z-runtime
    install_command: [ouro-toolchain, install, httpx]
  - name: z-runtime
    kind: binary
    install_command: [ouro-toolchain, install, runtime]
`
	if err := os.WriteFile(filepath.Join(root, "toolchain", "web-api-pack", "manifest.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	dependent, err := CreateToolchainInstallRequest(root, "web-api-pack", "a-httpx")
	if err != nil {
		t.Fatalf("create dependent request: %v", err)
	}
	if _, err := CreateToolchainInstallRequest(root, "web-api-pack", "z-runtime"); err != nil {
		t.Fatalf("create prerequisite request: %v", err)
	}
	if _, err := UpdateToolchainInstallRequestStatus(root, dependent.Path, "approved"); err != nil {
		t.Fatalf("approve dependent request: %v", err)
	}

	model := NewTUIModelForWorkspace(root).SelectRuntimeTab(tui.RuntimeTabToolchainHealth)
	resized, cmd := model.Update(tea.WindowSizeMsg{Width: 260, Height: 30})
	if cmd != nil {
		t.Fatalf("expected resize update to be synchronous")
	}
	view := resized.View()
	for _, want := range []string{
		"install dispatch: ready=0 skipped=2",
		"dispatch skip depends_on web-api-pack/a-httpx: z-runtime",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected TUI dependency skip %q:\n%s", want, view)
		}
	}
}

func TestNewTUIModelForWorkspaceShowsInstallDispatchArtifacts(t *testing.T) {
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
  - name: missing-httpx
    kind: binary
    install_command: [ouro-toolchain, install, httpx]
  - name: missing-nuclei
    kind: binary
    install_command: [ouro-toolchain, install, nuclei]
`
	if err := os.WriteFile(filepath.Join(root, "toolchain", "web-api-pack", "manifest.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	approved, err := CreateToolchainInstallRequest(root, "web-api-pack", "missing-httpx")
	if err != nil {
		t.Fatalf("create approved request: %v", err)
	}
	if _, err := CreateToolchainInstallRequest(root, "web-api-pack", "missing-nuclei"); err != nil {
		t.Fatalf("create pending request: %v", err)
	}
	if _, err := UpdateToolchainInstallRequestStatus(root, approved.Path, "approved"); err != nil {
		t.Fatalf("approve request: %v", err)
	}
	if _, err := CreateToolchainInstallDispatch(root); err != nil {
		t.Fatalf("create dispatch artifact: %v", err)
	}

	model := NewTUIModelForWorkspace(root).SelectRuntimeTab(tui.RuntimeTabToolchainHealth)
	resized, cmd := model.Update(tea.WindowSizeMsg{Width: 260, Height: 30})
	if cmd != nil {
		t.Fatalf("expected resize update to be synchronous")
	}
	view := resized.View()
	for _, want := range []string{
		"install dispatch artifacts: total=1",
		"dispatch artifact:",
		"dispatch artifact state: dry-run mode=metadata-only ready=1 skipped=1",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected TUI dispatch artifact row %q:\n%s", want, view)
		}
	}
}

func TestNewTUIModelForWorkspaceShowsInstallOrchestrationArtifacts(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	marker := filepath.Join(t.TempDir(), "orchestration-tui-ran")
	writeInstallRequestFixture(t, root, ToolchainInstallRequest{
		Status:     "approved",
		Pack:       "web-api-pack",
		Dependency: "missing-httpx",
		Command:    []string{"sh", "-c", "touch " + marker},
	})
	if _, err := CreateToolchainInstallOrchestration(root, ToolchainInstallOrchestrationOptions{}); err != nil {
		t.Fatalf("create orchestration artifact: %v", err)
	}

	model := NewTUIModelForWorkspace(root).SelectRuntimeTab(tui.RuntimeTabToolchainHealth)
	resized, cmd := model.Update(tea.WindowSizeMsg{Width: 260, Height: 36})
	if cmd != nil {
		t.Fatalf("expected resize update to be synchronous")
	}
	view := resized.View()
	for _, want := range []string{
		"install orchestration artifacts: total=1",
		"install orchestration artifact:",
		"orchestration summary: dry-run metadata-only d_ready=1 r_ready=0",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected TUI orchestration artifact row %q:\n%s", want, view)
		}
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("TUI orchestration visibility must not execute installer command, marker stat err=%v", err)
	}
}

func TestNewTUIModelForWorkspaceShowsInstallOrchestrationArtifactDetails(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	orchestration, err := CreateToolchainInstallOrchestration(root, ToolchainInstallOrchestrationOptions{})
	if err != nil {
		t.Fatalf("create orchestration artifact: %v", err)
	}

	model := NewTUIModelForWorkspace(root).SelectRuntimeTab(tui.RuntimeTabToolchainHealth)
	model = selectRuntimeRowContaining(t, model, "> install orchestration artifact:")
	model = model.ShowRuntimeActionDetails()
	resized, cmd := model.Update(tea.WindowSizeMsg{Width: 1200, Height: 32})
	if cmd != nil {
		t.Fatalf("expected resize update to be synchronous")
	}
	view := resized.View()
	for _, want := range []string{
		"[Node Inspector]",
		"runtime action: install-orchestration-artifact",
		"artifact path: " + orchestration.Path,
		"state: dry-run",
		"execution mode: metadata-only",
		"ready: 0",
		"mode: read-only",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected orchestration detail %q:\n%s", want, view)
		}
	}
}

func TestNewTUIModelForWorkspaceExecutesDispatchAfterDoubleConfirm(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	marker := filepath.Join(t.TempDir(), "tui-executed")
	manifest := `name: web-api-pack
version: "0.1.0"
capabilities:
  - web.discovery
runner: native
parser: web_api_parser
healthcheck: built_in
dependencies:
  - name: missing-httpx
    kind: binary
    install_command:
      - sh
      - -c
      - echo tui-confirmed > ` + marker + `; echo tui-confirmed
`
	if err := os.WriteFile(filepath.Join(root, "toolchain", "web-api-pack", "manifest.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	request, err := CreateToolchainInstallRequest(root, "web-api-pack", "missing-httpx")
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	if _, err := UpdateToolchainInstallRequestStatus(root, request.Path, "approved"); err != nil {
		t.Fatalf("approve request: %v", err)
	}
	if _, err := CreateToolchainInstallDispatch(root); err != nil {
		t.Fatalf("create dispatch artifact: %v", err)
	}

	model := NewTUIModelForWorkspace(root).SelectRuntimeTab(tui.RuntimeTabToolchainHealth)
	for i := 0; i < 7; i++ {
		model = model.MoveRuntimeSelection(1)
	}
	armed, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if cmd != nil {
		t.Fatalf("first x should only arm execution confirmation")
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("first confirmation step must not execute installer, marker stat err=%v", err)
	}
	armedModel, ok := armed.(tui.Model)
	if !ok {
		t.Fatalf("expected TUI model, got %T", armed)
	}
	confirmed, cmd := armedModel.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if cmd == nil {
		t.Fatalf("second x should return execution command")
	}
	msg := cmd()
	confirmed, cmd = confirmed.Update(msg)
	if cmd != nil {
		t.Fatalf("expected execution result update to be synchronous")
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("expected double-confirmed TUI execution to run installer: %v", err)
	}
	executions, err := ListToolchainInstallDispatchExecutions(root)
	if err != nil {
		t.Fatalf("list executions: %v", err)
	}
	if len(executions) != 1 || executions[0].Status != "completed" {
		t.Fatalf("expected completed execution artifact, got %#v", executions)
	}
	resized, cmd := confirmed.Update(tea.WindowSizeMsg{Width: 280, Height: 60})
	if cmd != nil {
		t.Fatalf("expected resize update to be synchronous")
	}
	view := resized.View()
	for _, want := range []string{
		"execution status: completed",
		"execution path:",
		"tui-confirmed",
		"install execution artifacts: total=1",
		"execution result web-api-pack/missing-httpx: completed",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected TUI execution result %q:\n%s", want, view)
		}
	}
}

func TestNewTUIModelForWorkspaceShowsFailedDispatchExecutionAfterDoubleConfirm(t *testing.T) {
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
  - name: missing-httpx
    kind: binary
    install_command:
      - sh
      - -c
      - echo tui-failed-output; exit 7
`
	if err := os.WriteFile(filepath.Join(root, "toolchain", "web-api-pack", "manifest.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	request, err := CreateToolchainInstallRequest(root, "web-api-pack", "missing-httpx")
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	if _, err := UpdateToolchainInstallRequestStatus(root, request.Path, "approved"); err != nil {
		t.Fatalf("approve request: %v", err)
	}
	if _, err := CreateToolchainInstallDispatch(root); err != nil {
		t.Fatalf("create dispatch artifact: %v", err)
	}

	model := NewTUIModelForWorkspace(root).SelectRuntimeTab(tui.RuntimeTabToolchainHealth)
	for i := 0; i < 7; i++ {
		model = model.MoveRuntimeSelection(1)
	}
	armed, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if cmd != nil {
		t.Fatalf("first x should only arm execution confirmation")
	}
	armedModel, ok := armed.(tui.Model)
	if !ok {
		t.Fatalf("expected TUI model, got %T", armed)
	}
	failed, cmd := armedModel.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if cmd == nil {
		t.Fatalf("second x should return execution command")
	}
	failed, cmd = failed.Update(cmd())
	if cmd != nil {
		t.Fatalf("expected execution result update to be synchronous")
	}
	executions, err := ListToolchainInstallDispatchExecutions(root)
	if err != nil {
		t.Fatalf("list executions: %v", err)
	}
	if len(executions) != 1 || executions[0].Status != "failed" || len(executions[0].Results) != 1 || executions[0].Results[0].Status != "failed" {
		t.Fatalf("expected failed execution artifact, got %#v", executions)
	}
	resized, cmd := failed.Update(tea.WindowSizeMsg{Width: 280, Height: 60})
	if cmd != nil {
		t.Fatalf("expected resize update to be synchronous")
	}
	view := resized.View()
	for _, want := range []string{
		"execution status: failed",
		"execution path:",
		"execution error:",
		"tui-failed-output",
		"install execution artifacts: total=1",
		"execution result web-api-pack/missing-httpx: failed",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected failed TUI execution result %q:\n%s", want, view)
		}
	}
}

func TestNewTUIModelForWorkspaceExecutesRetryDispatchAfterDoubleConfirm(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	marker := filepath.Join(t.TempDir(), "tui-retry-executed")
	request := writeInstallRequestFixture(t, root, ToolchainInstallRequest{
		Status:     "approved",
		Pack:       "web-api-pack",
		Dependency: "missing-httpx",
		Command:    []string{"sh", "-c", "true"},
	})
	writeInstallExecutionFixture(t, root, "failed-execution.json", ToolchainInstallDispatchExecution{
		Status:       "failed",
		Mode:         "execute",
		DispatchPath: filepath.Join(root, "toolchain", "install-dispatches", "dispatch.json"),
		Results: []ToolchainInstallDispatchExecutionResult{{
			RequestPath: request.Path,
			Pack:        "web-api-pack",
			Dependency:  "missing-httpx",
			Command:     []string{"sh", "-c", "echo retry-confirmed > " + marker + "; echo retry-confirmed"},
			Status:      "failed",
			Error:       "exit status 7",
		}},
	})
	if _, err := CreateToolchainInstallRetryDispatch(root); err != nil {
		t.Fatalf("create retry dispatch artifact: %v", err)
	}

	model := NewTUIModelForWorkspace(root).SelectRuntimeTab(tui.RuntimeTabToolchainHealth)
	for i := 0; i < 13; i++ {
		model = model.MoveRuntimeSelection(1)
	}
	armed, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if cmd != nil {
		t.Fatalf("first x should only arm retry execution confirmation")
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("first retry confirmation step must not execute retry command, marker stat err=%v", err)
	}
	armedModel, ok := armed.(tui.Model)
	if !ok {
		t.Fatalf("expected TUI model, got %T", armed)
	}
	confirmed, cmd := armedModel.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if cmd == nil {
		t.Fatalf("second x should return retry execution command")
	}
	msg := cmd()
	confirmed, cmd = confirmed.Update(msg)
	if cmd != nil {
		t.Fatalf("expected retry execution result update to be synchronous")
	}
	if data, err := os.ReadFile(marker); err != nil || strings.TrimSpace(string(data)) != "retry-confirmed" {
		t.Fatalf("expected double-confirmed TUI retry execution to run command, data=%q err=%v", data, err)
	}
	executions, err := ListToolchainInstallRetryDispatchExecutions(root)
	if err != nil {
		t.Fatalf("list retry executions: %v", err)
	}
	if len(executions) != 1 || executions[0].Status != "completed" {
		t.Fatalf("expected completed retry execution artifact, got %#v", executions)
	}
	resized, cmd := confirmed.Update(tea.WindowSizeMsg{Width: 420, Height: 70})
	if cmd != nil {
		t.Fatalf("expected resize update to be synchronous")
	}
	view := resized.View()
	for _, want := range []string{
		"execution status: completed",
		"execution path:",
		"retry-confirmed",
		"install retry execution artifacts: total=1",
		"retry execution result web-api-pack/missing-httpx: completed",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected TUI retry execution result %q:\n%s", want, view)
		}
	}
}

func TestNewTUIModelForWorkspaceShowsFailedRetryDispatchExecutionAfterDoubleConfirm(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	marker := filepath.Join(t.TempDir(), "tui-retry-failed")
	request := writeInstallRequestFixture(t, root, ToolchainInstallRequest{
		Status:     "approved",
		Pack:       "web-api-pack",
		Dependency: "missing-httpx",
		Command:    []string{"sh", "-c", "true"},
	})
	sourceExecution := writeInstallExecutionFixture(t, root, "failed-execution.json", ToolchainInstallDispatchExecution{
		Status:       "failed",
		Mode:         "execute",
		DispatchPath: filepath.Join(root, "toolchain", "install-dispatches", "dispatch.json"),
		Results: []ToolchainInstallDispatchExecutionResult{{
			RequestPath: request.Path,
			Pack:        "web-api-pack",
			Dependency:  "missing-httpx",
			Command:     []string{"sh", "-c", "echo retry-failed-marker > " + marker + "; echo retry-failed-output; exit 9"},
			Status:      "failed",
			Error:       "exit status 7",
		}},
	})
	if _, err := CreateToolchainInstallRetryDispatch(root); err != nil {
		t.Fatalf("create retry dispatch artifact: %v", err)
	}

	model := NewTUIModelForWorkspace(root).SelectRuntimeTab(tui.RuntimeTabToolchainHealth)
	for i := 0; i < 13; i++ {
		model = model.MoveRuntimeSelection(1)
	}
	armed, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if cmd != nil {
		t.Fatalf("first x should only arm retry execution confirmation")
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("first retry confirmation step must not execute retry command, marker stat err=%v", err)
	}
	armedModel, ok := armed.(tui.Model)
	if !ok {
		t.Fatalf("expected TUI model, got %T", armed)
	}
	failed, cmd := armedModel.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if cmd == nil {
		t.Fatalf("second x should return retry execution command")
	}
	failed, cmd = failed.Update(cmd())
	if cmd != nil {
		t.Fatalf("expected failed retry execution result update to be synchronous")
	}
	if data, err := os.ReadFile(marker); err != nil || strings.TrimSpace(string(data)) != "retry-failed-marker" {
		t.Fatalf("expected failed retry command to run once, data=%q err=%v", data, err)
	}
	executions, err := ListToolchainInstallRetryDispatchExecutions(root)
	if err != nil {
		t.Fatalf("list retry executions: %v", err)
	}
	if len(executions) != 1 || executions[0].Status != "failed" || len(executions[0].Results) != 1 || executions[0].Results[0].Status != "failed" {
		t.Fatalf("expected failed retry execution artifact, got %#v", executions)
	}
	if !strings.Contains(executions[0].Results[0].Output, "retry-failed-output") || !strings.Contains(executions[0].Results[0].Error, "exit status 9") {
		t.Fatalf("expected failed retry output and error metadata, got %#v", executions[0].Results[0])
	}
	resized, cmd := failed.Update(tea.WindowSizeMsg{Width: 520, Height: 72})
	if cmd != nil {
		t.Fatalf("expected resize update to be synchronous")
	}
	view := resized.View()
	for _, want := range []string{
		"execution status: failed",
		"execution path:",
		"execution error: execute retry install command for web-api-pack/missing-httpx",
		"retry-failed-output",
		"action mode: metadata-only",
		"install retry execution artifacts: total=1",
		"retry execution result web-api-pack/missing-httpx: failed",
		"error=exit status 9",
		"source_execution=" + filepath.Base(sourceExecution.Path),
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected failed TUI retry execution result %q:\n%s", want, view)
		}
	}
	failedModel, ok := failed.(tui.Model)
	if !ok {
		t.Fatalf("expected TUI model after failed retry execution, got %T", failed)
	}
	detailed := failedModel.ShowRuntimeActionDetails()
	resizedDetail, cmd := detailed.Update(tea.WindowSizeMsg{Width: 520, Height: 72})
	if cmd != nil {
		t.Fatalf("expected failed retry execution detail resize to be synchronous")
	}
	detailView := resizedDetail.View()
	for _, want := range []string{
		"runtime action: install-retry-execution-artifact",
		"state: failed",
		"retry dispatch path:",
		"mode: read-only",
	} {
		if !strings.Contains(detailView, want) {
			t.Fatalf("expected failed retry execution detail %q:\n%s", want, detailView)
		}
	}
}

func TestNewTUIModelForWorkspaceShowsInstallRetryPlan(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	marker := filepath.Join(t.TempDir(), "retry-ran")
	execution := writeInstallExecutionFixture(t, root, "failed-execution.json", ToolchainInstallDispatchExecution{
		Status:       "failed",
		Mode:         "execute",
		DispatchPath: filepath.Join(root, "toolchain", "install-dispatches", "dispatch.json"),
		Results: []ToolchainInstallDispatchExecutionResult{
			{
				RequestPath: filepath.Join(root, "toolchain", "install-requests", "httpx.json"),
				Pack:        "web-api-pack",
				Dependency:  "missing-httpx",
				Command:     []string{"sh", "-c", "touch " + marker},
				Status:      "failed",
				Error:       "exit status 7",
			},
			{
				RequestPath: filepath.Join(root, "toolchain", "install-requests", "nuclei.json"),
				Pack:        "web-api-pack",
				Dependency:  "missing-nuclei",
				Command:     []string{"ouro-toolchain", "install", "nuclei"},
				Status:      "completed",
			},
		},
	})

	model := NewTUIModelForWorkspace(root).SelectRuntimeTab(tui.RuntimeTabToolchainHealth)
	resized, cmd := model.Update(tea.WindowSizeMsg{Width: 1600, Height: 40})
	if cmd != nil {
		t.Fatalf("expected resize update to be synchronous")
	}
	view := resized.View()
	for _, want := range []string{
		"install retry plan: total=2 ready=1 skipped=1",
		"retry install web-api-pack/missing-httpx: ready",
		"artifact=" + execution.Path,
		"error=exit status 7",
		"retry skip web-api-pack/missing-nuclei: not-failed: completed",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected TUI retry plan row %q:\n%s", want, view)
		}
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("TUI retry plan must not execute retry command, marker stat err=%v", err)
	}
}

func TestNewTUIModelForWorkspaceShowsInstallRetryDispatchArtifacts(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	marker := filepath.Join(t.TempDir(), "retry-dispatch-tui-ran")
	writeInstallExecutionFixture(t, root, "failed-execution.json", ToolchainInstallDispatchExecution{
		Status:       "failed",
		Mode:         "execute",
		DispatchPath: filepath.Join(root, "toolchain", "install-dispatches", "dispatch.json"),
		Results: []ToolchainInstallDispatchExecutionResult{{
			RequestPath: filepath.Join(root, "toolchain", "install-requests", "httpx.json"),
			Pack:        "web-api-pack",
			Dependency:  "missing-httpx",
			Command:     []string{"sh", "-c", "touch " + marker},
			Status:      "failed",
			Error:       "exit status 7",
		}},
	})
	if _, err := CreateToolchainInstallRetryDispatch(root); err != nil {
		t.Fatalf("create retry dispatch: %v", err)
	}

	model := NewTUIModelForWorkspace(root).SelectRuntimeTab(tui.RuntimeTabToolchainHealth)
	resized, cmd := model.Update(tea.WindowSizeMsg{Width: 420, Height: 40})
	if cmd != nil {
		t.Fatalf("expected resize update to be synchronous")
	}
	view := resized.View()
	for _, want := range []string{
		"install retry dispatch artifacts: total=1",
		"retry dispatch artifact:",
		"retry dispatch artifact state: dry-run mode=metadata-only ready=1 skipped=0",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected TUI retry dispatch artifact row %q:\n%s", want, view)
		}
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("TUI retry dispatch artifact visibility must not execute retry command, marker stat err=%v", err)
	}
}

func TestNewTUIModelForWorkspaceShowsInstallRetryDispatchArtifactDetails(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	marker := filepath.Join(t.TempDir(), "retry-dispatch-detail-ran")
	writeInstallExecutionFixture(t, root, "failed-execution.json", ToolchainInstallDispatchExecution{
		Status:       "failed",
		Mode:         "execute",
		DispatchPath: filepath.Join(root, "toolchain", "install-dispatches", "dispatch.json"),
		Results: []ToolchainInstallDispatchExecutionResult{{
			RequestPath: filepath.Join(root, "toolchain", "install-requests", "httpx.json"),
			Pack:        "web-api-pack",
			Dependency:  "missing-httpx",
			Command:     []string{"sh", "-c", "touch " + marker},
			Status:      "failed",
			Error:       "exit status 7",
		}},
	})
	dispatch, err := CreateToolchainInstallRetryDispatch(root)
	if err != nil {
		t.Fatalf("create retry dispatch: %v", err)
	}

	model := NewTUIModelForWorkspace(root).SelectRuntimeTab(tui.RuntimeTabToolchainHealth)
	for i := 0; i < 8; i++ {
		model = model.MoveRuntimeSelection(1)
	}
	model = model.ShowRuntimeActionDetails()
	resized, cmd := model.Update(tea.WindowSizeMsg{Width: 1200, Height: 32})
	if cmd != nil {
		t.Fatalf("expected resize update to be synchronous")
	}
	view := resized.View()
	for _, want := range []string{
		"[Node Inspector]",
		"runtime action: install-retry-dispatch-artifact",
		"artifact path: " + dispatch.Path,
		"state: dry-run",
		"execution mode: metadata-only",
		"ready: 1",
		"skipped: 0",
		"mode: metadata-only",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected retry dispatch detail %q:\n%s", want, view)
		}
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("TUI retry dispatch artifact details must not execute retry command, marker stat err=%v", err)
	}
}

func TestNewTUIModelForWorkspaceShowsInstallRetryExecutionArtifacts(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	marker := filepath.Join(t.TempDir(), "retry-execution-tui-ran")
	execution := writeInstallRetryExecutionFixture(t, root, "retry-execution.json", ToolchainInstallRetryDispatchExecution{
		Status:            "completed",
		Mode:              "execute",
		RetryDispatchPath: filepath.Join(root, "toolchain", "install-retry-dispatches", "retry-dispatch.json"),
		Results: []ToolchainInstallRetryDispatchResult{{
			ExecutionPath: filepath.Join(root, "toolchain", "install-executions", "failed-execution.json"),
			DispatchPath:  filepath.Join(root, "toolchain", "install-dispatches", "dispatch.json"),
			RequestPath:   filepath.Join(root, "toolchain", "install-requests", "httpx.json"),
			Pack:          "web-api-pack",
			Dependency:    "missing-httpx",
			Command:       []string{"sh", "-c", "touch " + marker},
			LastStatus:    "failed",
			LastError:     "exit status 7",
			Status:        "completed",
			Output:        "retried",
		}},
	})

	model := NewTUIModelForWorkspace(root).SelectRuntimeTab(tui.RuntimeTabToolchainHealth)
	resized, cmd := model.Update(tea.WindowSizeMsg{Width: 720, Height: 40})
	if cmd != nil {
		t.Fatalf("expected resize update to be synchronous")
	}
	view := resized.View()
	for _, want := range []string{
		"install retry execution artifacts: total=1",
		"retry execution artifact: " + execution.Path,
		"retry execution artifact state: completed mode=execute results=1 skipped=0",
		"retry execution result web-api-pack/missing-httpx: completed",
		"output=retried",
		"source_execution=" + filepath.Base(execution.Results[0].ExecutionPath),
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected TUI retry execution artifact row %q:\n%s", want, view)
		}
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("TUI retry execution artifact visibility must not execute retry command, marker stat err=%v", err)
	}
}

func TestNewTUIModelForWorkspaceShowsInstallRetryExecutionResultDetails(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	execution := writeInstallRetryExecutionFixture(t, root, "retry-execution.json", ToolchainInstallRetryDispatchExecution{
		Status:            "failed",
		Mode:              "execute",
		RetryDispatchPath: filepath.Join(root, "toolchain", "install-retry-dispatches", "retry-dispatch.json"),
		Results: []ToolchainInstallRetryDispatchResult{{
			ExecutionPath: filepath.Join(root, "toolchain", "install-executions", "failed-execution.json"),
			DispatchPath:  filepath.Join(root, "toolchain", "install-dispatches", "dispatch.json"),
			RequestPath:   filepath.Join(root, "toolchain", "install-requests", "httpx.json"),
			Pack:          "web-api-pack",
			Dependency:    "missing-httpx",
			Command:       []string{"sh", "-c", "echo retried"},
			LastStatus:    "failed",
			LastError:     "exit status 7",
			Status:        "failed",
			Output:        "retry failed",
			Error:         "exit status 9",
		}},
	})

	model := NewTUIModelForWorkspace(root).SelectRuntimeTab(tui.RuntimeTabToolchainHealth)
	for i := 0; i < 4; i++ {
		model = model.MoveRuntimeSelection(1)
	}
	model = model.ShowRuntimeActionDetails()
	resized, cmd := model.Update(tea.WindowSizeMsg{Width: 1400, Height: 32})
	if cmd != nil {
		t.Fatalf("expected resize update to be synchronous")
	}
	view := resized.View()
	for _, want := range []string{
		"[Node Inspector]",
		"runtime action: install-retry-execution-result",
		"pack: web-api-pack",
		"name: missing-httpx",
		"state: failed",
		"command: sh -c echo retried",
		"output: retry failed",
		"execution error: exit status 9",
		"artifact path: " + execution.Path,
		"retry dispatch path: " + execution.RetryDispatchPath,
		"source execution path: " + execution.Results[0].ExecutionPath,
		"dispatch path: " + execution.Results[0].DispatchPath,
		"mode: read-only",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected retry execution detail %q:\n%s", want, view)
		}
	}
}

func TestNewTUIModelForWorkspaceShowsInstallExecutionArtifacts(t *testing.T) {
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
  - name: missing-httpx
    kind: binary
    install_command:
      - sh
      - -c
      - echo tui-executed
`
	if err := os.WriteFile(filepath.Join(root, "toolchain", "web-api-pack", "manifest.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	request, err := CreateToolchainInstallRequest(root, "web-api-pack", "missing-httpx")
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	if _, err := UpdateToolchainInstallRequestStatus(root, request.Path, "approved"); err != nil {
		t.Fatalf("approve request: %v", err)
	}
	dispatch, err := CreateToolchainInstallDispatch(root)
	if err != nil {
		t.Fatalf("create dispatch: %v", err)
	}
	if _, err := ExecuteToolchainInstallDispatch(root, dispatch.Path, ToolchainInstallDispatchExecutionOptions{Execute: true}); err != nil {
		t.Fatalf("execute dispatch: %v", err)
	}

	model := NewTUIModelForWorkspace(root).SelectRuntimeTab(tui.RuntimeTabToolchainHealth)
	resized, cmd := model.Update(tea.WindowSizeMsg{Width: 260, Height: 30})
	if cmd != nil {
		t.Fatalf("expected resize update to be synchronous")
	}
	view := resized.View()
	for _, want := range []string{
		"install execution artifacts: total=1",
		"execution artifact:",
		"execution artifact state: completed mode=execute results=1 skipped=0",
		"execution result web-api-pack/missing-httpx: completed",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected TUI execution artifact row %q:\n%s", want, view)
		}
	}
}

func TestNewTUIModelForWorkspaceShowsInstallExecutionResultDetails(t *testing.T) {
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
  - name: missing-httpx
    kind: binary
    install_command:
      - sh
      - -c
      - echo tui-detail-output
`
	if err := os.WriteFile(filepath.Join(root, "toolchain", "web-api-pack", "manifest.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	request, err := CreateToolchainInstallRequest(root, "web-api-pack", "missing-httpx")
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	if _, err := UpdateToolchainInstallRequestStatus(root, request.Path, "approved"); err != nil {
		t.Fatalf("approve request: %v", err)
	}
	dispatch, err := CreateToolchainInstallDispatch(root)
	if err != nil {
		t.Fatalf("create dispatch: %v", err)
	}
	if _, err := ExecuteToolchainInstallDispatch(root, dispatch.Path, ToolchainInstallDispatchExecutionOptions{Execute: true}); err != nil {
		t.Fatalf("execute dispatch: %v", err)
	}

	model := NewTUIModelForWorkspace(root).SelectRuntimeTab(tui.RuntimeTabToolchainHealth)
	for i := 0; i < 9; i++ {
		model = model.MoveRuntimeSelection(1)
	}
	model = model.ShowRuntimeActionDetails()
	resized, cmd := model.Update(tea.WindowSizeMsg{Width: 280, Height: 32})
	if cmd != nil {
		t.Fatalf("expected resize update to be synchronous")
	}
	view := resized.View()
	for _, want := range []string{
		"[Node Inspector]",
		"runtime action: install-execution-result",
		"pack: web-api-pack",
		"name: missing-httpx",
		"state: completed",
		"command: sh -c echo tui-detail-output",
		"output: tui-detail-output",
		"mode: read-only",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected execution detail %q:\n%s", want, view)
		}
	}
}

func selectRuntimeRowContaining(t *testing.T, model tui.Model, marker string) tui.Model {
	t.Helper()
	for i := 0; i < 1000; i++ {
		resized, cmd := model.Update(tea.WindowSizeMsg{Width: 1600, Height: 90})
		if cmd != nil {
			t.Fatalf("expected row selection resize to be synchronous")
		}
		resizedModel, ok := resized.(tui.Model)
		if !ok {
			t.Fatalf("expected TUI model, got %T", resized)
		}
		if strings.Contains(resizedModel.View(), marker) {
			return model
		}
		model = model.MoveRuntimeSelection(1)
	}
	resized, _ := model.Update(tea.WindowSizeMsg{Width: 1600, Height: 90})
	t.Fatalf("could not find selected runtime row marker %q:\n%s", marker, resized.View())
	return model
}
