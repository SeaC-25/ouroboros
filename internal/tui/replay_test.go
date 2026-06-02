package tui

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ouroboros/ouroboros/internal/eventlog"
)

func TestStatusFromReplaySnapshotFormatsRuntimeStatus(t *testing.T) {
	status := StatusFromReplay(eventlog.ReplaySnapshot{
		Status: eventlog.ReplayStatus{
			Model:          "gpt-x",
			ContextPercent: 73,
			AgentCount:     2,
			BreadthPercent: 40,
			DepthChains:    3,
			ToolHealth:     "ready",
			Cost:           1.25,
		},
	})

	if status.Model != "gpt-x" || status.ContextPercent != 73 || status.Agents != 2 {
		t.Fatalf("unexpected status fields: %#v", status)
	}
	if status.BreadthPercent != 40 || status.DepthChains != 3 || status.Tools != "ready" || status.Cost != "$1.25" {
		t.Fatalf("unexpected status display fields: %#v", status)
	}
}

func TestModelWithReplaySnapshotUpdatesGraphAndRuntimeTabs(t *testing.T) {
	snapshot, err := eventlog.BuildReplaySnapshot(replayProjectionFixtureEvents())
	if err != nil {
		t.Fatalf("build replay snapshot: %v", err)
	}

	model := NewModel().WithReplaySnapshot(snapshot)

	if model.status.Model != "gpt-x" || model.status.Agents != 1 || model.status.Tools != "ready" {
		t.Fatalf("status was not projected from replay: %#v", model.status)
	}
	if len(model.graphStack.Attack.Nodes) == 0 || len(model.graphStack.Attack.Edges) == 0 {
		t.Fatalf("attack graph was not projected from replay: %#v", model.graphStack.Attack)
	}
	if len(model.graphStack.Agents.Agents) != 1 || model.graphStack.Agents.Agents[0].AgentID != "agent-web" {
		t.Fatalf("agent graph was not projected from replay: %#v", model.graphStack.Agents)
	}

	modelRows := model.graphStack.RuntimeTabs.Select(RuntimeTabModelRuntime).ActiveRows()
	if !containsRow(modelRows, "model: gpt-x") || !containsRow(modelRows, "tokens: 1000 in / 250 out") {
		t.Fatalf("model runtime rows not projected: %#v", modelRows)
	}
	timelineRows := model.graphStack.RuntimeTabs.Select(RuntimeTabTimelineLanes).ActiveRows()
	if !containsRow(timelineRows, "mission: 1") || !containsRow(timelineRows, "blackboard: 3") {
		t.Fatalf("timeline rows not projected: %#v", timelineRows)
	}
}

func TestModelWithReplaySnapshotShowsToolchainDependencyRows(t *testing.T) {
	digest := "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	latestDigest := "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	snapshot, err := eventlog.BuildReplaySnapshot([]eventlog.Event{
		{
			ID:    1,
			RunID: "run-toolchain-deps",
			Type:  eventlog.EventTool,
			Payload: mustReplayProjectionJSON(map[string]any{
				"tool":    "web-api-pack",
				"health":  "failed",
				"message": "dependency ouro-httpx-worker failed: digest mismatch",
				"dependencies": []map[string]any{
					{
						"name":                "ouro-httpx-worker",
						"kind":                "container_image",
						"state":               "failed",
						"required":            true,
						"source":              "ghcr.io/ouroboros/httpx-worker",
						"image_digest":        digest,
						"latest_image_digest": latestDigest,
						"metadata_source":     "updates/httpx-worker.json",
						"message":             "digest mismatch",
					},
				},
				"install_plan": []map[string]any{
					{
						"name":     "ouro-httpx-worker",
						"kind":     "container_image",
						"state":    "failed",
						"required": true,
						"command":  []string{"ouro-toolchain", "install", "ouro-httpx-worker"},
						"hint":     "install pinned worker image",
						"source":   "ghcr.io/ouroboros/httpx-worker",
						"digest":   digest,
					},
				},
			}),
			CreatedAt: time.Date(2026, 5, 29, 12, 0, 0, 0, time.UTC),
		},
	})
	if err != nil {
		t.Fatalf("build replay snapshot: %v", err)
	}

	model := NewModel().WithReplaySnapshot(snapshot)
	rows := model.graphStack.RuntimeTabs.Select(RuntimeTabToolchainHealth).ActiveRows()

	for _, want := range []string{
		"toolchain: failed",
		"web-api-pack: failed",
		"dep ouro-httpx-worker: failed required=true",
		"source=ghcr.io/ouroboros/httpx-worker",
		"digest=" + digest,
		"latest_digest=" + latestDigest,
		"metadata=updates/httpx-worker.json",
		"digest mismatch",
		"install ouro-httpx-worker: failed required=true",
		"command=ouro-toolchain install ouro-httpx-worker",
		"hint=install pinned worker image",
	} {
		if !containsRow(rows, want) {
			t.Fatalf("expected toolchain dependency row %q, got %#v", want, rows)
		}
	}
}

func TestModelWithReplaySnapshotShowsActiveToolWorkers(t *testing.T) {
	snapshot, err := eventlog.BuildReplaySnapshot([]eventlog.Event{
		{
			ID:    1,
			RunID: "run-workers",
			Type:  eventlog.EventTool,
			Payload: mustReplayProjectionJSON(map[string]any{
				"tool":              "web-api-pack",
				"execution_id":      "worker-web",
				"worker_status":     "started",
				"worker_capability": "web.discovery",
			}),
			CreatedAt: time.Date(2026, 5, 29, 12, 0, 0, 0, time.UTC),
		},
		{
			ID:    2,
			RunID: "run-workers",
			Type:  eventlog.EventTool,
			Payload: mustReplayProjectionJSON(map[string]any{
				"tool":             "infra-pack",
				"execution_id":     "worker-infra",
				"execution_status": "completed",
				"capability":       "infra.discovery",
			}),
			CreatedAt: time.Date(2026, 5, 29, 12, 0, 1, 0, time.UTC),
		},
		{
			ID:    3,
			RunID: "run-workers",
			Type:  eventlog.EventTool,
			Payload: mustReplayProjectionJSON(map[string]any{
				"tool":              "ctf-pack",
				"execution_id":      "worker-ctf",
				"worker_status":     "queued",
				"worker_capability": "ctf.fast_hypothesis",
			}),
			CreatedAt: time.Date(2026, 5, 29, 12, 0, 2, 0, time.UTC),
		},
		{
			ID:    4,
			RunID: "run-workers",
			Type:  eventlog.EventTool,
			Payload: mustReplayProjectionJSON(map[string]any{
				"tool":             "web-api-pack",
				"execution_id":     "worker-web-skipped",
				"execution_status": "skipped",
				"capability":       "web.discovery",
			}),
			CreatedAt: time.Date(2026, 5, 29, 12, 0, 3, 0, time.UTC),
		},
		{
			ID:    5,
			RunID: "run-workers",
			Type:  eventlog.EventTool,
			Payload: mustReplayProjectionJSON(map[string]any{
				"tool":              "infra-pack",
				"execution_id":      "worker-waiting",
				"worker_status":     "waiting",
				"worker_capability": "infra.host_discovery",
				"wait_reason":       "cpu_units",
			}),
			CreatedAt: time.Date(2026, 5, 29, 12, 0, 4, 0, time.UTC),
		},
	})
	if err != nil {
		t.Fatalf("build replay snapshot: %v", err)
	}

	model := NewModel().WithReplaySnapshot(snapshot)
	rows := model.graphStack.RuntimeTabs.Select(RuntimeTabAgentRuntime).ActiveRows()

	for _, want := range []string{
		"tool workers: queued=1 waiting=1 active=1 completed=1 failed=0 skipped=1",
		"worker worker-ctf ctf-pack ctf.fast_hypothesis queued",
		"worker worker-waiting infra-pack infra.host_discovery waiting reason=cpu_units",
		"worker worker-web web-api-pack web.discovery started",
	} {
		if !containsRow(rows, want) {
			t.Fatalf("expected active worker row %q, got %#v", want, rows)
		}
	}
}

func TestModelWithReplaySnapshotShowsSchedulerAdmissions(t *testing.T) {
	snapshot, err := eventlog.BuildReplaySnapshot([]eventlog.Event{
		{
			ID:    1,
			RunID: "run-admission",
			Type:  eventlog.EventTool,
			Payload: mustReplayProjectionJSON(map[string]any{
				"kind":                 "scheduler_admission",
				"admission_id":         "admission-web",
				"tool":                 "web-api-pack",
				"admission_action":     "discover_assets",
				"admission_capability": "web.discovery",
				"admission_decision":   "skipped",
				"execution_policy":     "disabled",
				"policy_reason":        "noisy active scan deferred",
				"isolation":            "process",
				"container_overrides":  false,
			}),
			CreatedAt: time.Date(2026, 5, 29, 12, 5, 0, 0, time.UTC),
		},
		{
			ID:    2,
			RunID: "run-admission",
			Type:  eventlog.EventTool,
			Payload: mustReplayProjectionJSON(map[string]any{
				"kind":                        "scheduler_admission",
				"admission_id":                "admission-infra",
				"tool":                        "infra-pack",
				"admission_action":            "discover_assets",
				"admission_capability":        "infra.host_discovery",
				"admission_decision":          "admitted",
				"execution_policy":            "enabled",
				"policy_reason":               "authorized infra discovery",
				"isolation":                   "container",
				"container_overrides":         true,
				"pack_max_concurrency":        1,
				"resource_cpu_units":          2,
				"resource_memory_mb":          1024,
				"resource_limit_cpu_units":    2,
				"resource_limit_memory_mb":    2048,
				"worker_id":                   "infra-local",
				"worker_pool_max_concurrency": 1,
			}),
			CreatedAt: time.Date(2026, 5, 29, 12, 5, 1, 0, time.UTC),
		},
	})
	if err != nil {
		t.Fatalf("build replay snapshot: %v", err)
	}

	model := NewModel().WithReplaySnapshot(snapshot)
	rows := model.graphStack.RuntimeTabs.Select(RuntimeTabAgentRuntime).ActiveRows()

	for _, want := range []string{
		"tool admissions: admitted=1 skipped=1",
		"admission infra-pack infra.host_discovery admitted policy=enabled worker=infra-local worker_max_concurrency=1 max_concurrency=1 cpu=2/2 memory_mb=1024/2048 container_override=true reason=authorized infra discovery",
		"admission web-api-pack web.discovery skipped policy=disabled reason=noisy active scan deferred",
	} {
		if !containsRow(rows, want) {
			t.Fatalf("expected scheduler admission row %q, got %#v", want, rows)
		}
	}
}

func TestModelUpdateAppliesReplaySnapshotMessage(t *testing.T) {
	snapshot, err := eventlog.BuildReplaySnapshot(replayProjectionFixtureEvents())
	if err != nil {
		t.Fatalf("build replay snapshot: %v", err)
	}

	updated, cmd := NewModel().Update(ReplaySnapshotMsg{Snapshot: snapshot})
	if cmd != nil {
		t.Fatalf("expected replay snapshot update to be synchronous")
	}
	model, ok := updated.(Model)
	if !ok {
		t.Fatalf("expected Model after update, got %T", updated)
	}
	if model.status.Model != "gpt-x" || model.status.Tools != "ready" || model.status.Agents != 1 {
		t.Fatalf("snapshot message was not applied to status: %#v", model.status)
	}
	if !strings.Contains(model.View(), "model:gpt-x") {
		t.Fatalf("expected updated view to include replay status, got:\n%s", model.View())
	}
}

func TestModelReplayTimelineStepsThroughEvents(t *testing.T) {
	events := replayProjectionFixtureEvents()

	updated, cmd := NewModel().Update(ReplayTimelineMsg{Events: events})
	if cmd != nil {
		t.Fatalf("expected replay timeline load to be synchronous")
	}
	model, ok := updated.(Model)
	if !ok {
		t.Fatalf("expected Model after timeline load, got %T", updated)
	}
	if model.replayCursor != len(events) || model.status.Model != "gpt-x" {
		t.Fatalf("expected replay timeline to start at final event, cursor=%d status=%#v", model.replayCursor, model.status)
	}
	if model.graphStack.RuntimeTabs.Active != RuntimeTabTimelineLanes {
		t.Fatalf("expected replay timeline to focus Timeline Lanes tab, got %s", model.graphStack.RuntimeTabs.Active)
	}
	rows := model.graphStack.RuntimeTabs.ActiveRows()
	for _, want := range []string{"cursor: 6/6", "event: #6 agent lane:agent", "keys: left/right"} {
		if !containsRow(rows, want) {
			t.Fatalf("expected final timeline row %q, got %#v", want, rows)
		}
	}

	updated, cmd = model.Update(tea.KeyMsg{Type: tea.KeyLeft})
	if cmd != nil {
		t.Fatalf("expected replay step left to be synchronous")
	}
	model = updated.(Model)
	if model.replayCursor != len(events)-1 {
		t.Fatalf("expected cursor to move left, got %d", model.replayCursor)
	}
	if model.status.Model != "not-set" || model.status.Tools != "ready" {
		t.Fatalf("expected pre-agent replay state after step left, got %#v", model.status)
	}
	rows = model.graphStack.RuntimeTabs.ActiveRows()
	for _, want := range []string{"cursor: 5/6", "event: #5 tool lane:toolchain", "keys: left/right"} {
		if !containsRow(rows, want) {
			t.Fatalf("expected stepped timeline row %q, got %#v", want, rows)
		}
	}

	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRight})
	model = updated.(Model)
	if model.replayCursor != len(events) || model.status.Model != "gpt-x" {
		t.Fatalf("expected cursor to return to final state, cursor=%d status=%#v", model.replayCursor, model.status)
	}
}

func TestModelReplayTimelineShowsLaneBarsAndEventWindow(t *testing.T) {
	events := replayProjectionFixtureEvents()

	updated, _ := NewModel().Update(ReplayTimelineMsg{Events: events})
	model := updated.(Model)
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyLeft})
	model = updated.(Model)

	rows := model.graphStack.RuntimeTabs.ActiveRows()
	for _, want := range []string{
		"mission: 1 [#-----]",
		"agent: 0 [------]",
		"blackboard: 3 [###---]",
		"toolchain: 1 [#-----]",
		"prev: #4 hint lane:blackboard",
		"current: #5 tool lane:toolchain",
		"next: #6 agent lane:agent",
	} {
		if !containsRow(rows, want) {
			t.Fatalf("expected replay timeline controller row %q, got %#v", want, rows)
		}
	}
}

func TestModelReplayTimelineJumpsToStartAndEnd(t *testing.T) {
	events := replayProjectionFixtureEvents()

	updated, _ := NewModel().Update(ReplayTimelineMsg{Events: events})
	model := updated.(Model)

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyHome})
	if cmd != nil {
		t.Fatalf("expected home replay jump to be synchronous")
	}
	model = updated.(Model)
	if model.replayCursor != 0 {
		t.Fatalf("expected home to jump to replay start, got cursor=%d", model.replayCursor)
	}
	rows := model.graphStack.RuntimeTabs.ActiveRows()
	for _, want := range []string{"cursor: 0/6", "event: start", "next: #1 goal lane:mission", "keys: left/right home/end"} {
		if !containsRow(rows, want) {
			t.Fatalf("expected replay start row %q, got %#v", want, rows)
		}
	}

	updated, cmd = model.Update(tea.KeyMsg{Type: tea.KeyEnd})
	if cmd != nil {
		t.Fatalf("expected end replay jump to be synchronous")
	}
	model = updated.(Model)
	if model.replayCursor != len(events) || model.status.Model != "gpt-x" {
		t.Fatalf("expected end to jump to replay finish, cursor=%d status=%#v", model.replayCursor, model.status)
	}
	rows = model.graphStack.RuntimeTabs.ActiveRows()
	for _, want := range []string{"cursor: 6/6", "current: #6 agent lane:agent", "next: end", "keys: left/right home/end"} {
		if !containsRow(rows, want) {
			t.Fatalf("expected replay end row %q, got %#v", want, rows)
		}
	}
}

func replayProjectionFixtureEvents() []eventlog.Event {
	base := time.Date(2026, 5, 29, 10, 0, 0, 0, time.UTC)
	return []eventlog.Event{
		{ID: 1, RunID: "run-replay", Type: eventlog.EventGoal, Payload: mustReplayProjectionJSON(map[string]any{"goal": "verify admin route", "context_percent": 73}), CreatedAt: base},
		{ID: 2, RunID: "run-replay", Type: eventlog.EventFact, Payload: mustReplayProjectionJSON(map[string]any{"fact_id": "fact-target", "kind": "target", "statement": "https://app.example", "confidence": 0.9, "source": "scope"}), CreatedAt: base.Add(time.Second)},
		{ID: 3, RunID: "run-replay", Type: eventlog.EventIntent, Payload: mustReplayProjectionJSON(map[string]any{"intent_id": "intent-auth", "kind": "auth-check", "goal": "test admin authorization", "source_facts": []string{"fact-target"}, "state": "Open"}), CreatedAt: base.Add(2 * time.Second)},
		{ID: 4, RunID: "run-replay", Type: eventlog.EventHint, Payload: mustReplayProjectionJSON(map[string]any{"hint_id": "hint-1", "kind": "operator", "message": "prioritize admin routes", "source": "operator"}), CreatedAt: base.Add(3 * time.Second)},
		{ID: 5, RunID: "run-replay", Type: eventlog.EventTool, Payload: mustReplayProjectionJSON(map[string]any{"tool": "httpx", "health": "ready"}), CreatedAt: base.Add(4 * time.Second)},
		{ID: 6, RunID: "run-replay", Type: eventlog.EventAgent, Payload: mustReplayProjectionJSON(map[string]any{"agent_id": "agent-web", "role": "web-api", "state": "Complete", "progress_percent": 100, "context_percent": 73, "model_usage": map[string]any{"provider": "openai", "model": "gpt-x", "input_tokens": 1000, "output_tokens": 250, "cost": 0.42}}), CreatedAt: base.Add(5 * time.Second)},
	}
}

func mustReplayProjectionJSON(value any) json.RawMessage {
	data, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return data
}

func containsRow(rows []string, want string) bool {
	for _, row := range rows {
		if strings.Contains(row, want) {
			return true
		}
	}
	return false
}
