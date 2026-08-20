package api

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ouroboros/ouroboros/internal/eventlog"
)

func TestProjectorDoesNotInventUncheckedBusinessFacts(t *testing.T) {
	t.Run("toolchain manifest is unchecked until a real health result exists", func(t *testing.T) {
		workspace := t.TempDir()
		if err := os.MkdirAll(filepath.Join(workspace, "toolchain/local-pack"), 0o755); err != nil {
			t.Fatalf("mkdir toolchain: %v", err)
		}
		mustWrite(t, filepath.Join(workspace, "toolchain/local-pack/manifest.yaml"), strings.Join([]string{
			"name: local-pack",
			"version: \"0.1.0\"",
			"capabilities:",
			"  - web.discovery",
			"runner: native",
			"parser: local_parser",
			"healthcheck: built_in",
			"run_command:",
			"  - local-probe",
			"dependencies:",
			"  - name: local-probe",
			"    kind: binary",
			"",
		}, "\n"))

		dto, err := ProjectToolchain(workspace)
		if err != nil {
			t.Fatalf("project toolchain: %v", err)
		}
		if dto.Status != "unknown" || dto.ToolsReady != 0 || len(dto.Packs) != 1 {
			t.Fatalf("manifest-only toolchain must be unknown with zero ready tools, got %#v", dto)
		}
		if dto.Packs[0].Status != "unknown" || dto.Packs[0].ToolsReady != 0 {
			t.Fatalf("manifest-only pack must be unknown with zero ready tools, got %#v", dto.Packs[0])
		}

		action := projectToolchainAction(ToolchainPackDTO{ID: "manual", Name: "Manual", RunCommand: []string{"probe"}})
		if action.Status != "unknown" {
			t.Fatalf("empty action status must stay unknown, got %#v", action)
		}
	})

	t.Run("ordinary findings files stay artifacts", func(t *testing.T) {
		ctx := context.Background()
		workspace := t.TempDir()
		if err := os.MkdirAll(filepath.Join(workspace, "findings"), 0o755); err != nil {
			t.Fatalf("mkdir findings: %v", err)
		}
		mustWrite(t, filepath.Join(workspace, "findings/note.md"), "# Investigation notes\n\nNo confirmed finding.\n")
		mustWrite(t, filepath.Join(workspace, "findings/raw.txt"), "plain operator notes\n")
		mustWrite(t, filepath.Join(workspace, "findings/structured.json"), `{"kind":"finding","title":"Confirmed finding","severity":"high","endpoint":"https://target.example/admin"}`)

		graph, err := ProjectEvidenceGraph(ctx, workspace, nil)
		if err != nil {
			t.Fatalf("project graph: %v", err)
		}
		if node, ok := graphNodeByPath(graph.Nodes, "findings/note.md"); !ok || node.Type != "Artifact" {
			t.Fatalf("ordinary markdown finding file must project as artifact, got ok=%v node=%#v", ok, node)
		}
		for _, node := range graph.Nodes {
			if node.Type == "Finding" && (node.Path == "findings/note.md" || node.Path == "findings/raw.txt") {
				t.Fatalf("ordinary file was projected as finding: %#v", node)
			}
		}

		findings, err := ProjectReportFindings(workspace, graph)
		if err != nil {
			t.Fatalf("project report findings: %v", err)
		}
		if containsReportFinding(findings, "Investigation notes", "info", "findings/note.md") {
			t.Fatalf("markdown title must not become a report finding: %#v", findings)
		}
		if !containsReportFinding(findings, "Confirmed finding", "high", "https://target.example/admin") {
			t.Fatalf("explicit structured finding was not preserved: %#v", findings)
		}
	})

	t.Run("eventlog entries without source ids do not become graph facts", func(t *testing.T) {
		nodes := map[string]GraphNodeDTO{}
		edges := map[string]GraphEdgeDTO{}
		events := []eventlog.Event{
			{ID: 11, RunID: "run-missing", Type: eventlog.EventFact, Payload: json.RawMessage(`{"kind":"finding","statement":"missing fact id"}`)},
			{ID: 12, RunID: "run-missing", Type: eventlog.EventIntent, Payload: json.RawMessage(`{"goal":"missing intent id","source_facts":["fact-real"]}`)},
			{ID: 13, RunID: "run-missing", Type: eventlog.EventTool, Payload: json.RawMessage(`{"tool":"missing execution id","input_node_ids":["fact-real"]}`)},
		}

		projectEventsIntoGraph(events, nodes, edges)
		if len(nodes) != 0 || len(edges) != 0 {
			t.Fatalf("missing source IDs must not create business graph nodes or edges, nodes=%#v edges=%#v", nodes, edges)
		}
		if got := replayEventNodeID(events[0], map[string]any{"kind": "finding", "statement": "missing fact id"}); got != "" {
			t.Fatalf("replay event node link must not synthesize event ID, got %q", got)
		}
	})

	t.Run("runs without explicit status or timestamp stay unknown and zero", func(t *testing.T) {
		events := []eventlog.Event{{ID: 21, RunID: "run-unknown", Type: eventlog.EventFact, Payload: json.RawMessage(`{"fact_id":"fact-real","kind":"target","statement":"target"}`)}}
		if got := runStatus(events); got != "unknown" {
			t.Fatalf("run without execution or worker status must be unknown, got %q", got)
		}
		if got := firstEventUnix(events); got != 0 {
			t.Fatalf("run without source timestamp must stay zero, got %d", got)
		}
	})
}

func TestProjectGUIBootstrapUsesWorkspaceArtifacts(t *testing.T) {
	ctx := context.Background()
	workspace := t.TempDir()
	for _, dir := range []string{"replay", "toolchain/web-api-pack", "reports", "evidence"} {
		if err := os.MkdirAll(filepath.Join(workspace, dir), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}
	mustWrite(t, filepath.Join(workspace, "providers.yaml"), "version: 1\nproviders:\n  - name: local\n    model: local-model\n    enabled: true\n")
	mustWrite(t, filepath.Join(workspace, "mission.yaml"), "version: 1\nmission:\n  name: real-mission\n  goal: 真实任务目标\n  command: printf 'real mission'\n")
	mustWrite(t, filepath.Join(workspace, "toolchain/web-api-pack/manifest.yaml"), strings.Join([]string{
		"name: web-api-pack",
		"version: \"0.1.0\"",
		"capabilities:",
		"  - web.discovery",
		"runner: native",
		"parser: web_api_parser",
		"healthcheck: built_in",
		"dependencies:",
		"  - name: httpx",
		"    kind: binary",
		"    optional: true",
		"  - name: nuclei",
		"    kind: binary",
		"    optional: true",
		"",
	}, "\n"))
	mustWrite(t, filepath.Join(workspace, "reports/report.md"), "# 真实报告\n\n内容来自测试工作区。\n")
	if err := os.MkdirAll(filepath.Join(workspace, "findings"), 0o755); err != nil {
		t.Fatalf("mkdir findings: %v", err)
	}
	mustWrite(t, filepath.Join(workspace, "findings/high.json"), `{"title":"真实高危发现","severity":"high","endpoint":"https://target.example/admin"}`)
	mustWrite(t, filepath.Join(workspace, "findings/cross.json"), `{"title":"真实关联发现","severity":"medium","endpoint":"https://target.example/login","node_id":"fact-real-target","execution_id":"exec-real","replay_event_id":"run-real-1","file_path":"evidence/custom.json"}`)
	mustWrite(t, filepath.Join(workspace, "evidence/custom.json"), "{}\n")

	store, err := eventlog.OpenSQLite(filepath.Join(workspace, "replay/events.db"))
	if err != nil {
		t.Fatalf("open event store: %v", err)
	}
	defer store.Close()
	if _, err := store.Append(ctx, eventlog.Event{RunID: "run-real", Type: eventlog.EventFact, Payload: json.RawMessage(`{"fact_id":"fact-real-target","kind":"target","statement":"真实目标节点","source":"test"}`)}); err != nil {
		t.Fatalf("append fact: %v", err)
	}
	if _, err := store.Append(ctx, eventlog.Event{RunID: "run-real", Type: eventlog.EventIntent, Payload: json.RawMessage(`{"intent_id":"intent-real","kind":"run","goal":"继续执行真实工作区","source_facts":["fact-real-target"]}`)}); err != nil {
		t.Fatalf("append intent: %v", err)
	}

	dto, err := ProjectGUIBootstrap(ctx, workspace, store)
	if err != nil {
		t.Fatalf("project bootstrap: %v", err)
	}
	if dto.Workspace.Path != workspace || dto.Workspace.Status != "ready" {
		t.Fatalf("unexpected workspace dto: %#v", dto.Workspace)
	}
	if dto.Mission.Name != "real-mission" || dto.Mission.Goal != "真实任务目标" || dto.Mission.Command == "" {
		t.Fatalf("expected mission from mission.yaml, got %#v", dto.Mission)
	}
	if dto.Provider.Model != "local-model" {
		t.Fatalf("expected provider model from providers.yaml, got %#v", dto.Provider)
	}
	if dto.Toolchain.Status != "unknown" || dto.Toolchain.ToolsReady != 0 || len(dto.Toolchain.Packs) != 1 {
		t.Fatalf("expected toolchain from manifest, got %#v", dto.Toolchain)
	}
	if dto.Toolchain.Packs[0].Status != "unknown" || dto.Toolchain.Packs[0].ToolsReady != 0 || dto.Toolchain.Packs[0].ManifestPath == "" {
		t.Fatalf("expected unchecked toolchain pack from manifest, got %#v", dto.Toolchain.Packs[0])
	}
	if len(dto.Runs) != 1 || dto.Runs[0].ID != "run-real" {
		t.Fatalf("expected run from eventlog, got %#v", dto.Runs)
	}
	if len(dto.Reports) != 1 || dto.Reports[0].Title != "真实报告" {
		t.Fatalf("expected report title from markdown, got %#v", dto.Reports)
	}
	if len(dto.ReplayEvents) < 2 || dto.ReplayEvents[0].RunID != "run-real" || dto.ReplayEvents[0].Detail == "" {
		t.Fatalf("expected replay events from eventlog, got %#v", dto.ReplayEvents)
	}
	if !containsReplayEventLink(dto.ReplayEvents, "fact-real-target", "") {
		t.Fatalf("expected replay event to link to graph node fact-real-target, got %#v", dto.ReplayEvents)
	}
	if !containsReportFinding(dto.ReportFindings, "真实高危发现", "high", "https://target.example/admin") {
		t.Fatalf("expected report finding from findings JSON, got %#v", dto.ReportFindings)
	}
	if !containsReportFindingLink(dto.ReportFindings, "真实高危发现", "findings/high.json", "findings-high-json") {
		t.Fatalf("expected report finding to link to source file and graph node, got %#v", dto.ReportFindings)
	}
	if !containsReportFindingRefs(dto.ReportFindings, "真实关联发现", "fact-real-target", "evidence/custom.json", "exec-real", "run-real-1") {
		t.Fatalf("expected report finding to preserve explicit cross references, got %#v", dto.ReportFindings)
	}
	if !containsWorkspaceFile(dto.Files, "providers.yaml") || !containsWorkspaceFile(dto.Files, "reports/report.md") {
		t.Fatalf("expected real workspace files in bootstrap, got %#v", dto.Files)
	}

	body, err := json.Marshal(dto)
	if err != nil {
		t.Fatalf("marshal dto: %v", err)
	}
	text := string(body)
	for _, want := range []string{"真实目标节点", "真实报告", "fact-real-target", "intent-real"} {
		if !strings.Contains(text, want) {
			t.Fatalf("projected dto missing %q: %s", want, text)
		}
	}
	for _, forbidden := range []string{"SQL Injection", "Port 80", "192.168.1.", "10.0.0.1"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("projected dto contains mock literal %q: %s", forbidden, text)
		}
	}
}

func containsWorkspaceFile(files []WorkspaceFileDTO, path string) bool {
	for _, file := range files {
		if file.Path == path {
			return true
		}
	}
	return false
}

func graphNodeByPath(nodes []GraphNodeDTO, path string) (GraphNodeDTO, bool) {
	for _, node := range nodes {
		if node.Path == path {
			return node, true
		}
	}
	return GraphNodeDTO{}, false
}

func containsReportFinding(findings []ReportFindingDTO, title string, severity string, endpoint string) bool {
	for _, finding := range findings {
		if finding.Title == title && finding.Severity == severity && finding.Endpoint == endpoint {
			return true
		}
	}
	return false
}

func containsReplayEventLink(events []ReplayEventDTO, nodeID string, executionID string) bool {
	for _, event := range events {
		if nodeID != "" && event.NodeID != nodeID {
			continue
		}
		if executionID != "" && event.ExecutionID != executionID {
			continue
		}
		return true
	}
	return false
}

func containsReportFindingLink(findings []ReportFindingDTO, title string, filePath string, nodeID string) bool {
	for _, finding := range findings {
		if finding.Title == title && finding.FilePath == filePath && finding.NodeID == nodeID {
			return true
		}
	}
	return false
}

func containsReportFindingRefs(findings []ReportFindingDTO, title string, nodeID string, filePath string, executionID string, replayEventID string) bool {
	for _, finding := range findings {
		if finding.Title == title && finding.NodeID == nodeID && finding.FilePath == filePath && finding.ExecutionID == executionID && finding.ReplayEventID == replayEventID {
			return true
		}
	}
	return false
}

func mustWrite(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
