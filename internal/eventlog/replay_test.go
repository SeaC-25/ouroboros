package eventlog

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/ouroboros/ouroboros/internal/blackboard"
	"github.com/ouroboros/ouroboros/internal/evidence"
)

func TestBuildReplaySnapshotReconstructsStateFromFixtureEvents(t *testing.T) {
	events := replayFixtureEvents()

	snapshot, err := BuildReplaySnapshot(events)
	if err != nil {
		t.Fatalf("build replay snapshot: %v", err)
	}

	if snapshot.RunID != "run-replay" {
		t.Fatalf("unexpected run id: %s", snapshot.RunID)
	}
	if len(snapshot.Blackboard.Facts) != 1 {
		t.Fatalf("expected one fact, got %#v", snapshot.Blackboard.Facts)
	}
	fact := snapshot.Blackboard.Facts[0]
	if fact.ID != "fact-target" || fact.Kind != "target" || fact.Statement != "https://app.example" {
		t.Fatalf("unexpected replayed fact: %#v", fact)
	}

	if len(snapshot.Blackboard.Intents) != 1 {
		t.Fatalf("expected one intent, got %#v", snapshot.Blackboard.Intents)
	}
	intent := snapshot.Blackboard.Intents[0]
	if intent.ID != "intent-auth" || intent.State != blackboard.IntentOpen || len(intent.SourceFacts) != 1 || intent.SourceFacts[0] != "fact-target" {
		t.Fatalf("unexpected replayed intent: %#v", intent)
	}

	if len(snapshot.Blackboard.Hints) != 1 || snapshot.Blackboard.Hints[0].Message != "prioritize admin routes" {
		t.Fatalf("unexpected replayed hints: %#v", snapshot.Blackboard.Hints)
	}

	if node, ok := snapshot.Evidence.Node("fact-target"); !ok || node.Type != evidence.NodeTarget {
		t.Fatalf("expected fact target evidence node, got %#v ok=%v", node, ok)
	}
	requires := snapshot.Evidence.EdgesOfType(evidence.EdgeRequires)
	if len(requires) != 1 || requires[0].From != "intent-auth" || requires[0].To != "fact-target" {
		t.Fatalf("expected intent requires fact edge, got %#v", requires)
	}

	if len(snapshot.Timeline) != len(events) {
		t.Fatalf("expected timeline for every event, got %#v", snapshot.Timeline)
	}
	if snapshot.Timeline[0].EventID != 1 || snapshot.Timeline[0].Lane != ReplayLaneMission {
		t.Fatalf("timeline was not chronologically sorted into mission lane: %#v", snapshot.Timeline)
	}
	if snapshot.Timeline[2].Lane != ReplayLaneBlackboard || snapshot.Timeline[4].Lane != ReplayLaneToolchain {
		t.Fatalf("unexpected timeline lanes: %#v", snapshot.Timeline)
	}

	if snapshot.ModelUsage.InputTokens != 1000 || snapshot.ModelUsage.OutputTokens != 250 || snapshot.ModelUsage.Cost != 0.42 {
		t.Fatalf("unexpected model usage aggregate: %#v", snapshot.ModelUsage)
	}
	if snapshot.Status.Model != "gpt-x" || snapshot.Status.ContextPercent != 73 || snapshot.Status.AgentCount != 1 || snapshot.Status.ToolHealth != "ready" || snapshot.Status.Cost != 0.42 {
		t.Fatalf("unexpected replay status: %#v", snapshot.Status)
	}
	if len(snapshot.Agents) != 1 || snapshot.Agents[0].ID != "agent-web" || snapshot.Agents[0].State != "Complete" {
		t.Fatalf("unexpected replay agents: %#v", snapshot.Agents)
	}
}

func TestBuildReplaySnapshotProjectsModelUsagePreviews(t *testing.T) {
	base := time.Date(2026, 6, 4, 15, 25, 0, 0, time.UTC)
	events := []Event{
		{
			ID:    1,
			RunID: "run-model-preview",
			Type:  EventVerification,
			Payload: mustFixtureJSON(map[string]any{
				"kind":           "model_usage_preview",
				"source":         "closed-loop-planner-preview",
				"agent_id":       "closed-loop-planner",
				"role":           "planner",
				"execution_mode": "metadata-only",
				"budget_status":  "not_enforced",
				"ledger_write":   false,
				"model_usage": map[string]any{
					"provider":      "local-openai",
					"model":         "gpt-preview",
					"input_tokens":  12,
					"output_tokens": 3,
					"cost":          0.0042,
				},
			}),
			CreatedAt: base,
		},
	}

	snapshot, err := BuildReplaySnapshot(events)
	if err != nil {
		t.Fatalf("build replay snapshot: %v", err)
	}

	if snapshot.ModelUsage.InputTokens != 12 || snapshot.ModelUsage.OutputTokens != 3 || snapshot.ModelUsage.Cost != 0.0042 {
		t.Fatalf("expected preview usage to aggregate into model usage, got %#v", snapshot.ModelUsage)
	}
	if snapshot.Status.Model != "gpt-preview" || snapshot.Status.Cost != 0.0042 {
		t.Fatalf("expected preview usage to update replay status, got %#v", snapshot.Status)
	}
	if len(snapshot.ModelUsagePreviews) != 1 {
		t.Fatalf("expected one model usage preview, got %#v", snapshot.ModelUsagePreviews)
	}
	preview := snapshot.ModelUsagePreviews[0]
	if preview.Source != "closed-loop-planner-preview" || preview.AgentID != "closed-loop-planner" || preview.Role != "planner" {
		t.Fatalf("unexpected preview identity: %#v", preview)
	}
	if preview.Provider != "local-openai" || preview.Model != "gpt-preview" || preview.InputTokens != 12 || preview.OutputTokens != 3 {
		t.Fatalf("unexpected preview usage fields: %#v", preview)
	}
	if preview.ExecutionMode != "metadata-only" || preview.BudgetStatus != "not_enforced" || preview.LedgerWrite {
		t.Fatalf("unexpected preview gate fields: %#v", preview)
	}
}

func TestBuildReplaySnapshotProjectsModelGenerationAsActualUsage(t *testing.T) {
	base := time.Date(2026, 6, 4, 15, 40, 0, 0, time.UTC)
	events := []Event{
		{
			ID:    1,
			RunID: "run-model-generation",
			Type:  EventVerification,
			Payload: mustFixtureJSON(map[string]any{
				"kind":                     "model_generation",
				"source":                   "closed-loop-planner-generation",
				"agent_id":                 "closed-loop-planner",
				"role":                     "planner",
				"status":                   "completed",
				"execution_mode":           "provider-generation",
				"budget_status":            "within_limit",
				"ledger_write":             true,
				"assistant_text_persisted": false,
				"streaming":                true,
				"stream_completed":         true,
				"stream_chunk_count":       3,
				"stream_digest":            "sha256:stream-generation",
				"stream_preview":           "Provider generation streamed the first attack hypothesis.",
				"text_digest":              "sha256:planner-generation",
				"text_preview":             "Provider generation selected the first attack hypothesis.",
				"model_usage": map[string]any{
					"run_id":        "run-model-generation",
					"agent_id":      "closed-loop-planner",
					"role":          "planner",
					"provider":      "openai",
					"model":         "gpt-provider",
					"input_tokens":  321,
					"output_tokens": 45,
					"cost":          0.019,
					"latency_ms":    1450,
				},
			}),
			CreatedAt: base,
		},
	}

	snapshot, err := BuildReplaySnapshot(events)
	if err != nil {
		t.Fatalf("build replay snapshot without full assistant text: %v", err)
	}

	if snapshot.ModelUsage.InputTokens != 321 || snapshot.ModelUsage.OutputTokens != 45 || snapshot.ModelUsage.Cost != 0.019 || snapshot.ModelUsage.Latency != 1450*time.Millisecond {
		t.Fatalf("expected generation usage to aggregate into actual model usage, got %#v", snapshot.ModelUsage)
	}
	if snapshot.Status.Model != "gpt-provider" || snapshot.Status.Cost != 0.019 {
		t.Fatalf("expected generation usage to update replay status, got %#v", snapshot.Status)
	}
	if len(snapshot.ModelUsagePreviews) != 1 {
		t.Fatalf("expected one model generation runtime preview, got %#v", snapshot.ModelUsagePreviews)
	}
	generation := snapshot.ModelUsagePreviews[0]
	if generation.Source != "closed-loop-planner-generation" || generation.AgentID != "closed-loop-planner" || generation.Role != "planner" {
		t.Fatalf("unexpected generation identity: %#v", generation)
	}
	if generation.Provider != "openai" || generation.Model != "gpt-provider" || generation.InputTokens != 321 || generation.OutputTokens != 45 {
		t.Fatalf("unexpected generation usage fields: %#v", generation)
	}
	if generation.ExecutionMode != "provider-generation" || generation.BudgetStatus != "within_limit" || !generation.LedgerWrite {
		t.Fatalf("unexpected generation gate fields: %#v", generation)
	}
	if !generation.Streaming || !generation.StreamCompleted || generation.StreamChunkCount != 3 || generation.StreamDigest != "sha256:stream-generation" {
		t.Fatalf("unexpected generation stream fields: %#v", generation)
	}
	if generation.StreamPreview != "Provider generation streamed the first attack hypothesis." {
		t.Fatalf("unexpected generation stream preview: %#v", generation)
	}
}

func TestBuildReplaySnapshotProjectsModelToolPlanPreview(t *testing.T) {
	base := time.Date(2026, 6, 5, 1, 15, 0, 0, time.UTC)
	events := []Event{
		{
			ID:    1,
			RunID: "run-tool-plan-preview",
			Type:  EventVerification,
			Payload: mustFixtureJSON(map[string]any{
				"kind":                           "model_tool_plan_preview",
				"source":                         "closed-loop-planner-generation",
				"agent_id":                       "closed-loop-planner",
				"role":                           "planner",
				"status":                         "completed",
				"execution_mode":                 "metadata-only",
				"provider":                       "local-openai",
				"model":                          "gpt-plan",
				"action":                         "enumerate_paths",
				"semantic_action":                "enumerate_paths",
				"valid":                          true,
				"validity":                       "valid",
				"marker_preview":                 "ouro_action: enumerate_paths",
				"model_output_digest":            "sha256:model-output",
				"model_generation_artifact_path": "/tmp/ouro/runtime/model-generations/generation.json",
				"compile_preview_review_path":    "/tmp/ouro/toolchain/compile-preview-reviews/review.json",
				"compile_preview_nodes":          3,
				"compile_preview_warnings":       1,
				"artifact_path":                  "/tmp/ouro/runtime/model-tool-plan-previews/preview.json",
				"artifact_digest":                "sha256:artifact",
				"assistant_text_persisted":       false,
				"executed_tools":                 0,
				"worker_dispatch":                false,
				"target_contact":                 false,
			}),
			CreatedAt: base,
		},
	}

	snapshot, err := BuildReplaySnapshot(events)
	if err != nil {
		t.Fatalf("build replay snapshot: %v", err)
	}
	if snapshot.ModelUsage.InputTokens != 0 || snapshot.ModelUsage.OutputTokens != 0 || snapshot.Status.Cost != 0 {
		t.Fatalf("tool-plan preview must not count as model token usage, got usage=%#v status=%#v", snapshot.ModelUsage, snapshot.Status)
	}
	if len(snapshot.ModelUsagePreviews) != 1 {
		t.Fatalf("expected one model tool-plan preview, got %#v", snapshot.ModelUsagePreviews)
	}
	preview := snapshot.ModelUsagePreviews[0]
	if preview.Kind != "model_tool_plan_preview" || preview.Source != "closed-loop-planner-generation" || preview.AgentID != "closed-loop-planner" || preview.Role != "planner" {
		t.Fatalf("unexpected tool-plan preview identity: %#v", preview)
	}
	if preview.Provider != "local-openai" || preview.Model != "gpt-plan" || preview.SemanticAction != "enumerate_paths" || preview.Validity != "valid" {
		t.Fatalf("unexpected tool-plan preview route/action fields: %#v", preview)
	}
	if preview.ArtifactPath != "/tmp/ouro/runtime/model-tool-plan-previews/preview.json" || preview.ArtifactDigest != "sha256:artifact" || preview.CompilePreviewReviewPath == "" {
		t.Fatalf("unexpected tool-plan preview artifact links: %#v", preview)
	}
	if preview.AssistantTextPersisted || preview.ExecutedTools != 0 || preview.WorkerDispatch || preview.TargetContact {
		t.Fatalf("tool-plan preview must remain read-only metadata, got %#v", preview)
	}
	if preview.TextPreview != "ouro_action: enumerate_paths" || preview.TextDigest != "sha256:model-output" {
		t.Fatalf("expected marker preview and model output digest, got %#v", preview)
	}
}

func TestBuildReplaySnapshotRejectsMixedRunIDs(t *testing.T) {
	events := replayFixtureEvents()
	events[1].RunID = "other-run"

	_, err := BuildReplaySnapshot(events)
	if err == nil {
		t.Fatalf("expected mixed run ids to fail")
	}
}

func TestBuildReplaySnapshotMarksToolchainFailedForFailedExecution(t *testing.T) {
	base := time.Date(2026, 5, 29, 11, 0, 0, 0, time.UTC)
	events := []Event{
		{
			ID:        1,
			RunID:     "run-failed-tool",
			Type:      EventTool,
			Payload:   mustFixtureJSON(map[string]any{"tool": "web-api-pack", "health": "ready"}),
			CreatedAt: base,
		},
		{
			ID:        2,
			RunID:     "run-failed-tool",
			Type:      EventTool,
			Payload:   mustFixtureJSON(map[string]any{"tool": "web-api-pack", "execution_id": "command-discover_assets-01", "execution_status": "failed", "transcript": "command timed out after 1ms"}),
			CreatedAt: base.Add(time.Second),
		},
		{
			ID:        3,
			RunID:     "run-failed-tool",
			Type:      EventTool,
			Payload:   mustFixtureJSON(map[string]any{"tool": "infra-pack", "execution_id": "offline-discover_assets-02", "execution_status": "completed"}),
			CreatedAt: base.Add(2 * time.Second),
		},
	}

	snapshot, err := BuildReplaySnapshot(events)
	if err != nil {
		t.Fatalf("build replay snapshot: %v", err)
	}
	if snapshot.Status.ToolHealth != "failed" {
		t.Fatalf("expected failed tool execution to mark tool health failed, got %#v", snapshot.Status)
	}
}

func TestBuildReplaySnapshotProjectsToolExecutions(t *testing.T) {
	base := time.Date(2026, 5, 29, 11, 15, 0, 0, time.UTC)
	events := []Event{
		{
			ID:    1,
			RunID: "run-workers",
			Type:  EventTool,
			Payload: mustFixtureJSON(map[string]any{
				"tool":              "web-api-pack",
				"execution_id":      "worker-web",
				"worker_status":     "started",
				"worker_capability": "web.discovery",
			}),
			CreatedAt: base,
		},
		{
			ID:    2,
			RunID: "run-workers",
			Type:  EventTool,
			Payload: mustFixtureJSON(map[string]any{
				"tool":              "infra-pack",
				"execution_id":      "worker-infra",
				"worker_status":     "waiting",
				"worker_capability": "infra.discovery",
				"wait_reason":       "cpu_units",
			}),
			CreatedAt: base.Add(time.Second),
		},
		{
			ID:    3,
			RunID: "run-workers",
			Type:  EventTool,
			Payload: mustFixtureJSON(map[string]any{
				"tool":             "worker-infra",
				"execution_id":     "worker-infra",
				"execution_status": "completed",
				"capability":       "infra.discovery",
			}),
			CreatedAt: base.Add(2 * time.Second),
		},
	}

	snapshot, err := BuildReplaySnapshot(events)
	if err != nil {
		t.Fatalf("build replay snapshot: %v", err)
	}
	if len(snapshot.ToolExecutions) != 2 {
		t.Fatalf("expected two projected tool executions, got %#v", snapshot.ToolExecutions)
	}
	if snapshot.ToolExecutions[0].ExecutionID != "worker-infra" || snapshot.ToolExecutions[0].Status != "completed" {
		t.Fatalf("expected completed infra worker first by id, got %#v", snapshot.ToolExecutions)
	}
	if snapshot.ToolExecutions[0].WaitReason != "cpu_units" {
		t.Fatalf("expected completed infra worker to preserve wait reason, got %#v", snapshot.ToolExecutions[0])
	}
	if snapshot.ToolExecutions[1].ExecutionID != "worker-web" || snapshot.ToolExecutions[1].Status != "started" {
		t.Fatalf("expected active web worker, got %#v", snapshot.ToolExecutions)
	}
}

func TestBuildReplaySnapshotProjectsSchedulerAdmissions(t *testing.T) {
	base := time.Date(2026, 5, 29, 11, 20, 0, 0, time.UTC)
	events := []Event{
		{
			ID:    1,
			RunID: "run-admission",
			Type:  EventTool,
			Payload: mustFixtureJSON(map[string]any{
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
			CreatedAt: base,
		},
		{
			ID:    2,
			RunID: "run-admission",
			Type:  EventTool,
			Payload: mustFixtureJSON(map[string]any{
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
			CreatedAt: base.Add(time.Second),
		},
	}

	snapshot, err := BuildReplaySnapshot(events)
	if err != nil {
		t.Fatalf("build replay snapshot: %v", err)
	}
	if len(snapshot.ToolAdmissions) != 2 {
		t.Fatalf("expected two scheduler admissions, got %#v", snapshot.ToolAdmissions)
	}
	if snapshot.ToolAdmissions[0].AdmissionID != "admission-infra" || snapshot.ToolAdmissions[0].Decision != "admitted" || !snapshot.ToolAdmissions[0].ContainerOverrides {
		t.Fatalf("expected admitted infra admission with container override, got %#v", snapshot.ToolAdmissions)
	}
	if snapshot.ToolAdmissions[0].PackMaxConcurrency != 1 {
		t.Fatalf("expected admitted infra admission to project pack max concurrency, got %#v", snapshot.ToolAdmissions[0])
	}
	if snapshot.ToolAdmissions[0].ResourceCPUUnits != 2 || snapshot.ToolAdmissions[0].ResourceMemoryMB != 1024 || snapshot.ToolAdmissions[0].ResourceLimitCPUUnits != 2 || snapshot.ToolAdmissions[0].ResourceLimitMemoryMB != 2048 {
		t.Fatalf("expected admitted infra admission to project resource request/limit, got %#v", snapshot.ToolAdmissions[0])
	}
	if snapshot.ToolAdmissions[0].WorkerID != "infra-local" {
		t.Fatalf("expected admitted infra admission to project worker placement, got %#v", snapshot.ToolAdmissions[0])
	}
	if snapshot.ToolAdmissions[0].WorkerPoolMaxConcurrency != 1 {
		t.Fatalf("expected admitted infra admission to project worker pool max concurrency, got %#v", snapshot.ToolAdmissions[0])
	}
	if snapshot.ToolAdmissions[1].AdmissionID != "admission-web" || snapshot.ToolAdmissions[1].Decision != "skipped" || snapshot.ToolAdmissions[1].ExecutionPolicy != "disabled" {
		t.Fatalf("expected skipped web admission, got %#v", snapshot.ToolAdmissions)
	}
	if snapshot.ToolAdmissions[1].PolicyReason != "noisy active scan deferred" {
		t.Fatalf("expected admission reason, got %#v", snapshot.ToolAdmissions[1])
	}
}

func TestBuildReplaySnapshotProjectsToolchainDependencyHealth(t *testing.T) {
	digest := "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	base := time.Date(2026, 5, 29, 11, 30, 0, 0, time.UTC)
	events := []Event{
		{
			ID:    1,
			RunID: "run-toolchain-deps",
			Type:  EventTool,
			Payload: mustFixtureJSON(map[string]any{
				"tool":    "web-api-pack",
				"health":  "failed",
				"message": "dependency ouro-httpx-worker failed: digest mismatch",
				"dependencies": []map[string]any{
					{
						"name":              "ouro-httpx-worker",
						"kind":              "container_image",
						"state":             "failed",
						"required":          true,
						"installed_version": "ghcr.io/ouroboros/httpx-worker@sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
						"source":            "ghcr.io/ouroboros/httpx-worker",
						"image_digest":      digest,
						"message":           "container image digest mismatch",
					},
				},
			}),
			CreatedAt: base,
		},
	}

	snapshot, err := BuildReplaySnapshot(events)
	if err != nil {
		t.Fatalf("build replay snapshot: %v", err)
	}
	if snapshot.Status.ToolHealth != "failed" {
		t.Fatalf("expected dependency failure to mark toolchain failed, got %#v", snapshot.Status)
	}
	if len(snapshot.Toolchain) != 1 {
		t.Fatalf("expected one toolchain status, got %#v", snapshot.Toolchain)
	}
	status := snapshot.Toolchain[0]
	if status.Name != "web-api-pack" || status.Health != "failed" {
		t.Fatalf("unexpected toolchain status: %#v", status)
	}
	if len(status.Dependencies) != 1 {
		t.Fatalf("expected dependency status, got %#v", status.Dependencies)
	}
	dependency := status.Dependencies[0]
	if dependency.Name != "ouro-httpx-worker" || dependency.State != "failed" || !dependency.Required {
		t.Fatalf("unexpected dependency status: %#v", dependency)
	}
	if dependency.Source != "ghcr.io/ouroboros/httpx-worker" || dependency.ImageDigest != digest {
		t.Fatalf("expected dependency provenance, got %#v", dependency)
	}
}

func replayFixtureEvents() []Event {
	base := time.Date(2026, 5, 29, 10, 0, 0, 0, time.UTC)
	return []Event{
		{
			ID:        3,
			RunID:     "run-replay",
			Type:      EventIntent,
			Payload:   mustFixtureJSON(map[string]any{"intent_id": "intent-auth", "kind": "auth-check", "goal": "test admin authorization", "source_facts": []string{"fact-target"}, "state": "Open"}),
			CreatedAt: base.Add(2 * time.Second),
		},
		{
			ID:        1,
			RunID:     "run-replay",
			Type:      EventGoal,
			Payload:   mustFixtureJSON(map[string]any{"goal": "verify admin route", "context_percent": 73}),
			CreatedAt: base,
		},
		{
			ID:        6,
			RunID:     "run-replay",
			Type:      EventAgent,
			Payload:   mustFixtureJSON(map[string]any{"agent_id": "agent-web", "role": "web-api", "state": "Complete", "progress_percent": 100, "context_percent": 73, "model_usage": map[string]any{"provider": "openai", "model": "gpt-x", "input_tokens": 1000, "output_tokens": 250, "cache_read_tokens": 25, "cache_write_tokens": 10, "cost": 0.42, "latency_ms": 1500}}),
			CreatedAt: base.Add(5 * time.Second),
		},
		{
			ID:        2,
			RunID:     "run-replay",
			Type:      EventFact,
			Payload:   mustFixtureJSON(map[string]any{"fact_id": "fact-target", "kind": "target", "statement": "https://app.example", "confidence": 0.9, "source": "scope"}),
			CreatedAt: base.Add(time.Second),
		},
		{
			ID:        5,
			RunID:     "run-replay",
			Type:      EventTool,
			Payload:   mustFixtureJSON(map[string]any{"tool": "httpx", "health": "ready"}),
			CreatedAt: base.Add(4 * time.Second),
		},
		{
			ID:        4,
			RunID:     "run-replay",
			Type:      EventHint,
			Payload:   mustFixtureJSON(map[string]any{"hint_id": "hint-1", "kind": "operator", "message": "prioritize admin routes", "source": "operator"}),
			CreatedAt: base.Add(3 * time.Second),
		},
	}
}

func mustFixtureJSON(value any) json.RawMessage {
	data, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return data
}
