package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/ouroboros/ouroboros/internal/eventlog"
)

func TestServerGUIHandlersReturnProjectedWorkspaceData(t *testing.T) {
	ctx := context.Background()
	workspace := t.TempDir()
	for _, dir := range []string{"replay", "toolchain/infra-pack", "evidence"} {
		if err := os.MkdirAll(filepath.Join(workspace, dir), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}
	mustWrite(t, filepath.Join(workspace, "providers.yaml"), "version: 1\nproviders:\n  - name: local\n    model: local-model\n    enabled: true\n")
	mustWrite(t, filepath.Join(workspace, "toolchain/infra-pack/manifest.yaml"), strings.Join([]string{
		"name: infra-pack",
		"version: \"0.1.0\"",
		"capabilities:",
		"  - infra.host_discovery",
		"runner: native",
		"parser: infra_parser",
		"healthcheck: built_in",
		"dependencies:",
		"  - name: nmap",
		"    kind: binary",
		"    optional: true",
		"",
	}, "\n"))
	mustWrite(t, filepath.Join(workspace, "evidence/real.json"), "{}\n")

	store, err := eventlog.OpenSQLite(filepath.Join(workspace, "replay/events.db"))
	if err != nil {
		t.Fatalf("open event store: %v", err)
	}
	defer store.Close()
	if _, err := store.Append(ctx, eventlog.Event{RunID: "run-handler", Type: eventlog.EventFact, Payload: json.RawMessage(`{"fact_id":"fact-handler","kind":"finding","statement":"真实处理器发现","source":"test"}`)}); err != nil {
		t.Fatalf("append event: %v", err)
	}

	server := &Server{workspace: workspace, eventStore: store}
	responses := []string{
		recordHandler(t, server.handleGUIBootstrap),
		recordHandler(t, server.handleWorkspaceFiles),
		recordHandler(t, server.handleEvidenceGraph),
		recordHandler(t, server.handleToolchainHealth),
		recordHandler(t, server.handleRuns),
	}

	joined := strings.Join(responses, "\n")
	for _, want := range []string{"local-model", "真实处理器发现", "infra-pack", "run-handler", "providers.yaml", "evidence/real.json"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("handler responses missing %q:\n%s", want, joined)
		}
	}
	for _, forbidden := range []string{"SQL Injection", "Port 80", "192.168.1.", "10.0.0.1"} {
		if strings.Contains(joined, forbidden) {
			t.Fatalf("handler responses contain mock literal %q:\n%s", forbidden, joined)
		}
	}
}

func TestServerWorkspaceFileReadAndWrite(t *testing.T) {
	workspace := t.TempDir()
	mustWrite(t, filepath.Join(workspace, "notes.md"), "# 原始内容\n")
	mustWrite(t, filepath.Join(workspace, "other.txt"), "这里包含 关键字\n")
	server := &Server{workspace: workspace}

	readRequest := httptest.NewRequest(http.MethodGet, "/api/workspace/file?path=notes.md", nil)
	readRecorder := httptest.NewRecorder()
	server.handleWorkspaceFile(readRecorder, readRequest)
	if readRecorder.Code != http.StatusOK || !strings.Contains(readRecorder.Body.String(), "原始内容") {
		t.Fatalf("expected file content, status=%d body=%s", readRecorder.Code, readRecorder.Body.String())
	}

	writeRequest := httptest.NewRequest(http.MethodPut, "/api/workspace/file?path=notes.md", bytes.NewBufferString(`{"content":"# 保存后的内容\n"}`))
	writeRecorder := httptest.NewRecorder()
	server.handleWorkspaceFile(writeRecorder, writeRequest)
	if writeRecorder.Code != http.StatusOK || !strings.Contains(writeRecorder.Body.String(), "保存后的内容") {
		t.Fatalf("expected saved file content, status=%d body=%s", writeRecorder.Code, writeRecorder.Body.String())
	}
	data, err := os.ReadFile(filepath.Join(workspace, "notes.md"))
	if err != nil {
		t.Fatalf("read saved file: %v", err)
	}
	if string(data) != "# 保存后的内容\n" {
		t.Fatalf("unexpected saved content: %q", string(data))
	}

	outsideRequest := httptest.NewRequest(http.MethodGet, "/api/workspace/file?path=../outside.md", nil)
	outsideRecorder := httptest.NewRecorder()
	server.handleWorkspaceFile(outsideRecorder, outsideRequest)
	if outsideRecorder.Code != http.StatusBadRequest {
		t.Fatalf("expected relative path outside workspace to fail, got %d", outsideRecorder.Code)
	}

	searchRequest := httptest.NewRequest(http.MethodGet, "/api/workspace/search?q=关键字", nil)
	searchRecorder := httptest.NewRecorder()
	server.handleWorkspaceSearch(searchRecorder, searchRequest)
	if searchRecorder.Code != http.StatusOK || !strings.Contains(searchRecorder.Body.String(), "other.txt") || !strings.Contains(searchRecorder.Body.String(), "关键字") {
		t.Fatalf("expected search result, status=%d body=%s", searchRecorder.Code, searchRecorder.Body.String())
	}
}

func TestServerEvidenceGraphExportWritesWorkspaceFile(t *testing.T) {
	ctx := context.Background()
	workspace := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspace, "replay"), 0o755); err != nil {
		t.Fatalf("mkdir replay: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(workspace, "findings"), 0o755); err != nil {
		t.Fatalf("mkdir findings: %v", err)
	}
	mustWrite(t, filepath.Join(workspace, "findings/high.json"), `{"id":"finding-export","kind":"finding","statement":"导出图谱发现"}`+"\n")

	store, err := eventlog.OpenSQLite(filepath.Join(workspace, "replay/events.db"))
	if err != nil {
		t.Fatalf("open event store: %v", err)
	}
	defer store.Close()
	if _, err := store.Append(ctx, eventlog.Event{RunID: "run-export", Type: eventlog.EventFact, Payload: json.RawMessage(`{"fact_id":"fact-export","kind":"finding","statement":"事件图谱发现","source":"test"}`)}); err != nil {
		t.Fatalf("append event: %v", err)
	}

	server := &Server{workspace: workspace, eventStore: store}
	request := httptest.NewRequest(http.MethodPost, "/api/evidence/graph/export", nil)
	recorder := httptest.NewRecorder()
	server.handleEvidenceGraphExport(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected graph export success, status=%d body=%s", recorder.Code, recorder.Body.String())
	}

	var response GraphExportDTO
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode graph export response: %v", err)
	}
	if response.FilePath != "exports/attack-graph.json" {
		t.Fatalf("unexpected export path: %#v", response)
	}
	if response.NodeCount == 0 {
		t.Fatalf("expected exported graph to contain real nodes: %#v", response)
	}
	data, err := os.ReadFile(filepath.Join(workspace, response.FilePath))
	if err != nil {
		t.Fatalf("read exported graph: %v", err)
	}
	for _, want := range []string{"findings/high.json", "事件图谱发现", `"nodes"`, `"edges"`} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("exported graph missing %q:\n%s", want, string(data))
		}
	}
}

func TestServerReportExportWritesWorkspaceMarkdown(t *testing.T) {
	ctx := context.Background()
	workspace := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspace, "replay"), 0o755); err != nil {
		t.Fatalf("mkdir replay: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(workspace, "findings"), 0o755); err != nil {
		t.Fatalf("mkdir findings: %v", err)
	}
	mustWrite(t, filepath.Join(workspace, "workspace.yaml"), strings.Join([]string{
		"version: 1",
		"workspace:",
		"  name: 报告导出工作区",
		"  operator: 安全工程师",
		"",
	}, "\n"))
	mustWrite(t, filepath.Join(workspace, "findings/high.json"), strings.Join([]string{
		"{",
		`  "title": "报告导出发现",`,
		`  "severity": "high",`,
		`  "endpoint": "https://report-export.local/admin",`,
		`  "execution_id": "exec-report",`,
		`  "replay_event_id": "run-report-1"`,
		"}",
		"",
	}, "\n"))

	store, err := eventlog.OpenSQLite(filepath.Join(workspace, "replay/events.db"))
	if err != nil {
		t.Fatalf("open event store: %v", err)
	}
	defer store.Close()
	if _, err := store.Append(ctx, eventlog.Event{RunID: "run-report", Type: eventlog.EventTool, Payload: json.RawMessage(`{"execution_id":"exec-report","tool":"report-smoke","log_path":"executions/exec-report/output.log"}`)}); err != nil {
		t.Fatalf("append event: %v", err)
	}

	server := &Server{workspace: workspace, eventStore: store}
	request := httptest.NewRequest(http.MethodPost, "/api/reports/export", nil)
	recorder := httptest.NewRecorder()
	server.handleReportExport(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected report export success, status=%d body=%s", recorder.Code, recorder.Body.String())
	}

	var response ReportExportDTO
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode report export response: %v", err)
	}
	if response.FilePath != "reports/ouroboros-report.md" {
		t.Fatalf("unexpected report export path: %#v", response)
	}
	if response.FindingCount == 0 {
		t.Fatalf("expected exported report to include findings: %#v", response)
	}
	data, err := os.ReadFile(filepath.Join(workspace, response.FilePath))
	if err != nil {
		t.Fatalf("read exported report: %v", err)
	}
	for _, want := range []string{"# Ouroboros 渗透测试报告", "报告导出工作区", "报告导出发现", "https://report-export.local/admin", "exec-report", "run-report-1"} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("exported report missing %q:\n%s", want, string(data))
		}
	}
}

func TestServerWorkspaceSettingsReadSaveAndBootstrap(t *testing.T) {
	workspace := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspace, "replay"), 0o755); err != nil {
		t.Fatalf("mkdir replay: %v", err)
	}
	mustWrite(t, filepath.Join(workspace, "workspace.yaml"), strings.Join([]string{
		"version: 1",
		"workspace:",
		"  name: 初始工作区",
		"  description: 初始描述",
		"  operator: 初始操作者",
		"  tags:",
		"    - lab",
		"",
	}, "\n"))
	store, err := eventlog.OpenSQLite(filepath.Join(workspace, "replay/events.db"))
	if err != nil {
		t.Fatalf("open event store: %v", err)
	}
	defer store.Close()
	server := &Server{workspace: workspace, eventStore: store}

	readRecorder := httptest.NewRecorder()
	server.handleWorkspaceSettings(readRecorder, httptest.NewRequest(http.MethodGet, "/api/workspace/settings", nil))
	if readRecorder.Code != http.StatusOK || !strings.Contains(readRecorder.Body.String(), "初始工作区") || !strings.Contains(readRecorder.Body.String(), "lab") {
		t.Fatalf("expected current workspace settings, status=%d body=%s", readRecorder.Code, readRecorder.Body.String())
	}

	saveBody := `{"name":"真实桌面工作区","description":"用于真实 IDE smoke 的工作区","operator":"红队工程师","tags":["desktop","真实"]}`
	saveRecorder := httptest.NewRecorder()
	server.handleWorkspaceSettings(saveRecorder, httptest.NewRequest(http.MethodPut, "/api/workspace/settings", strings.NewReader(saveBody)))
	if saveRecorder.Code != http.StatusOK {
		t.Fatalf("expected saved workspace settings, status=%d body=%s", saveRecorder.Code, saveRecorder.Body.String())
	}
	var saved WorkspaceDTO
	if err := json.Unmarshal(saveRecorder.Body.Bytes(), &saved); err != nil {
		t.Fatalf("decode saved workspace settings: %v", err)
	}
	if saved.Name != "真实桌面工作区" || saved.Description != "用于真实 IDE smoke 的工作区" || saved.Operator != "红队工程师" || len(saved.Tags) != 2 {
		t.Fatalf("unexpected saved workspace settings: %#v", saved)
	}
	data, err := os.ReadFile(filepath.Join(workspace, "workspace.yaml"))
	if err != nil {
		t.Fatalf("read workspace.yaml: %v", err)
	}
	workspaceYAML := string(data)
	for _, want := range []string{"真实桌面工作区", "用于真实 IDE smoke 的工作区", "红队工程师", "desktop", "真实"} {
		if !strings.Contains(workspaceYAML, want) {
			t.Fatalf("workspace.yaml missing %q:\n%s", want, workspaceYAML)
		}
	}

	bootstrapRecorder := httptest.NewRecorder()
	server.handleGUIBootstrap(bootstrapRecorder, httptest.NewRequest(http.MethodGet, "/api/gui/bootstrap", nil))
	if bootstrapRecorder.Code != http.StatusOK {
		t.Fatalf("expected bootstrap after workspace save, status=%d body=%s", bootstrapRecorder.Code, bootstrapRecorder.Body.String())
	}
	var bootstrap GUIBootstrapDTO
	if err := json.Unmarshal(bootstrapRecorder.Body.Bytes(), &bootstrap); err != nil {
		t.Fatalf("decode bootstrap: %v", err)
	}
	if bootstrap.Workspace.Name != "真实桌面工作区" ||
		bootstrap.Workspace.Description != "用于真实 IDE smoke 的工作区" ||
		bootstrap.Workspace.Operator != "红队工程师" ||
		bootstrap.Workspace.ConfigPath != filepath.Join(workspace, "workspace.yaml") {
		t.Fatalf("bootstrap did not reflect saved workspace settings: %#v", bootstrap.Workspace)
	}
}

func TestServerProviderReadSaveAndBootstrap(t *testing.T) {
	workspace := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspace, "replay"), 0o755); err != nil {
		t.Fatalf("mkdir replay: %v", err)
	}
	mustWrite(t, filepath.Join(workspace, "providers.yaml"), strings.Join([]string{
		"version: 1",
		"providers:",
		"  - name: offline",
		"    type: offline",
		"    model: offline-model",
		"    enabled: true",
		"",
	}, "\n"))
	store, err := eventlog.OpenSQLite(filepath.Join(workspace, "replay/events.db"))
	if err != nil {
		t.Fatalf("open event store: %v", err)
	}
	defer store.Close()
	server := &Server{workspace: workspace, eventStore: store}

	readRecorder := httptest.NewRecorder()
	server.handleProvider(readRecorder, httptest.NewRequest(http.MethodGet, "/api/provider", nil))
	if readRecorder.Code != http.StatusOK || !strings.Contains(readRecorder.Body.String(), "offline-model") {
		t.Fatalf("expected current provider config, status=%d body=%s", readRecorder.Code, readRecorder.Body.String())
	}

	saveBody := strings.Join([]string{
		"{",
		`"name":"local-openai",`,
		`"baseUrl":"https://models.internal.example/v1",`,
		`"model":"qwen-coder-real",`,
		`"apiKeyEnv":"OURO_LOCAL_API_KEY",`,
		`"enabled":true,`,
		`"timeoutMs":45000,`,
		`"contextWindowTokens":65536,`,
		`"capabilities":["chat","tools"],`,
		`"routingStrategy":"enabled_order_failover"`,
		"}",
	}, "")
	saveRecorder := httptest.NewRecorder()
	server.handleProvider(saveRecorder, httptest.NewRequest(http.MethodPut, "/api/provider", strings.NewReader(saveBody)))
	if saveRecorder.Code != http.StatusOK {
		t.Fatalf("expected saved provider config, status=%d body=%s", saveRecorder.Code, saveRecorder.Body.String())
	}
	var saved ProviderConfigDTO
	if err := json.Unmarshal(saveRecorder.Body.Bytes(), &saved); err != nil {
		t.Fatalf("decode saved provider config: %v", err)
	}
	if saved.Path != filepath.Join(workspace, "providers.yaml") || saved.RoutingStrategy != "enabled_order_failover" {
		t.Fatalf("unexpected saved provider config metadata: %#v", saved)
	}
	if len(saved.Providers) == 0 || saved.Providers[0].Name != "local-openai" || saved.Providers[0].Model != "qwen-coder-real" {
		t.Fatalf("expected saved provider in response, got %#v", saved.Providers)
	}
	data, err := os.ReadFile(filepath.Join(workspace, "providers.yaml"))
	if err != nil {
		t.Fatalf("read providers.yaml: %v", err)
	}
	providerYAML := string(data)
	for _, want := range []string{"local-openai", "qwen-coder-real", "https://models.internal.example/v1", "OURO_LOCAL_API_KEY", "enabled_order_failover", "chat", "tools"} {
		if !strings.Contains(providerYAML, want) {
			t.Fatalf("providers.yaml missing %q:\n%s", want, providerYAML)
		}
	}
	if strings.Contains(providerYAML, "sk-") {
		t.Fatalf("providers.yaml should store the API key environment name, not a raw secret:\n%s", providerYAML)
	}

	bootstrapRecorder := httptest.NewRecorder()
	server.handleGUIBootstrap(bootstrapRecorder, httptest.NewRequest(http.MethodGet, "/api/gui/bootstrap", nil))
	if bootstrapRecorder.Code != http.StatusOK {
		t.Fatalf("expected bootstrap after provider save, status=%d body=%s", bootstrapRecorder.Code, bootstrapRecorder.Body.String())
	}
	var bootstrap GUIBootstrapDTO
	if err := json.Unmarshal(bootstrapRecorder.Body.Bytes(), &bootstrap); err != nil {
		t.Fatalf("decode bootstrap: %v", err)
	}
	if bootstrap.Provider.Name != "local-openai" ||
		bootstrap.Provider.Model != "qwen-coder-real" ||
		bootstrap.Provider.BaseURL != "https://models.internal.example/v1" ||
		bootstrap.Provider.APIKeyEnv != "OURO_LOCAL_API_KEY" ||
		bootstrap.Provider.RoutingStrategy != "enabled_order_failover" {
		t.Fatalf("bootstrap did not reflect saved provider: %#v", bootstrap.Provider)
	}
}

func TestServerToolchainReadSaveAndBootstrap(t *testing.T) {
	workspace := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspace, "replay"), 0o755); err != nil {
		t.Fatalf("mkdir replay: %v", err)
	}
	packDir := filepath.Join(workspace, "toolchain", "local-pack")
	if err := os.MkdirAll(packDir, 0o755); err != nil {
		t.Fatalf("mkdir toolchain pack: %v", err)
	}
	mustWrite(t, filepath.Join(packDir, "manifest.yaml"), strings.Join([]string{
		"name: local-pack",
		"version: \"0.1.0\"",
		"capabilities:",
		"  - local.command",
		"runner: native",
		"parser: local_parser",
		"healthcheck: built_in",
		"dependencies:",
		"  - name: sh",
		"    kind: binary",
		"    optional: true",
		"",
	}, "\n"))
	store, err := eventlog.OpenSQLite(filepath.Join(workspace, "replay/events.db"))
	if err != nil {
		t.Fatalf("open event store: %v", err)
	}
	defer store.Close()
	server := &Server{workspace: workspace, eventStore: store}

	readRecorder := httptest.NewRecorder()
	server.handleToolchain(readRecorder, httptest.NewRequest(http.MethodGet, "/api/toolchain", nil))
	if readRecorder.Code != http.StatusOK || !strings.Contains(readRecorder.Body.String(), "local.command") {
		t.Fatalf("expected current toolchain config, status=%d body=%s", readRecorder.Code, readRecorder.Body.String())
	}

	saveBody := strings.Join([]string{
		"{",
		`"name":"local-pack-renamed",`,
		`"version":"0.2.0",`,
		`"capabilities":["local.command","evidence.collection"],`,
		`"runner":"command",`,
		`"runCommand":["sh","-lc","printf toolchain-run"],`,
		`"runTimeoutMs":12000,`,
		`"parser":"jsonl_parser",`,
		`"healthcheck":"exec",`,
		`"healthCommand":["sh","-c","printf ok"]`,
		"}",
	}, "")
	saveRecorder := httptest.NewRecorder()
	server.handleToolchainPack(saveRecorder, httptest.NewRequest(http.MethodPut, "/api/toolchain/packs/local", strings.NewReader(saveBody)))
	if saveRecorder.Code != http.StatusOK {
		t.Fatalf("expected saved toolchain pack, status=%d body=%s", saveRecorder.Code, saveRecorder.Body.String())
	}
	var saved ToolchainPackDTO
	if err := json.Unmarshal(saveRecorder.Body.Bytes(), &saved); err != nil {
		t.Fatalf("decode saved toolchain pack: %v", err)
	}
	if saved.Name != "local-pack-renamed" || saved.Version != "0.2.0" || saved.Runner != "command" || saved.Parser != "jsonl_parser" {
		t.Fatalf("unexpected saved toolchain pack: %#v", saved)
	}
	data, err := os.ReadFile(filepath.Join(packDir, "manifest.yaml"))
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	manifestYAML := string(data)
	for _, want := range []string{"local-pack-renamed", "0.2.0", "evidence.collection", "runner: command", "jsonl_parser", "healthcheck: exec", "run_timeout_ms: 12000"} {
		if !strings.Contains(manifestYAML, want) {
			t.Fatalf("manifest missing %q:\n%s", want, manifestYAML)
		}
	}

	bootstrapRecorder := httptest.NewRecorder()
	server.handleGUIBootstrap(bootstrapRecorder, httptest.NewRequest(http.MethodGet, "/api/gui/bootstrap", nil))
	if bootstrapRecorder.Code != http.StatusOK {
		t.Fatalf("expected bootstrap after toolchain save, status=%d body=%s", bootstrapRecorder.Code, bootstrapRecorder.Body.String())
	}
	var bootstrap GUIBootstrapDTO
	if err := json.Unmarshal(bootstrapRecorder.Body.Bytes(), &bootstrap); err != nil {
		t.Fatalf("decode bootstrap: %v", err)
	}
	if len(bootstrap.Toolchain.Packs) != 1 ||
		bootstrap.Toolchain.Packs[0].Name != "local-pack-renamed" ||
		bootstrap.Toolchain.Packs[0].Version != "0.2.0" ||
		bootstrap.Toolchain.Packs[0].Runner != "command" ||
		bootstrap.Toolchain.Packs[0].RunTimeoutMS != 12000 {
		t.Fatalf("bootstrap did not reflect saved toolchain manifest: %#v", bootstrap.Toolchain.Packs)
	}
}

func TestServerToolchainActionRunUsesManifestCommand(t *testing.T) {
	skipIfWindows(t)
	workspace := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspace, "replay"), 0o755); err != nil {
		t.Fatalf("mkdir replay: %v", err)
	}
	packDir := filepath.Join(workspace, "toolchain", "local-pack")
	if err := os.MkdirAll(packDir, 0o755); err != nil {
		t.Fatalf("mkdir toolchain pack: %v", err)
	}
	mustWrite(t, filepath.Join(packDir, "manifest.yaml"), strings.Join([]string{
		"name: local-pack",
		"version: \"0.1.0\"",
		"capabilities:",
		"  - local.command",
		"  - evidence.collection",
		"runner: command",
		"run_command:",
		"  - sh",
		"  - -lc",
		"  - printf 'tool-action-log\\n'; printf '%s|%s\\n' \"$OURO_TOOL_ACTION_ID\" \"$OURO_TOOL_ARGUMENTS\" > \"$OURO_ARTIFACT_DIR/tool-action.txt\"",
		"run_timeout_ms: 9000",
		"parser: local_parser",
		"healthcheck: built_in",
		"dependencies:",
		"  - name: sh",
		"    kind: binary",
		"    optional: true",
		"",
	}, "\n"))
	store, err := eventlog.OpenSQLite(filepath.Join(workspace, "replay/events.db"))
	if err != nil {
		t.Fatalf("open event store: %v", err)
	}
	defer store.Close()
	server := &Server{workspace: workspace, eventStore: store}

	bootstrapRecorder := httptest.NewRecorder()
	server.handleGUIBootstrap(bootstrapRecorder, httptest.NewRequest(http.MethodGet, "/api/gui/bootstrap", nil))
	if bootstrapRecorder.Code != http.StatusOK {
		t.Fatalf("expected bootstrap with tool action, status=%d body=%s", bootstrapRecorder.Code, bootstrapRecorder.Body.String())
	}
	var bootstrap GUIBootstrapDTO
	if err := json.Unmarshal(bootstrapRecorder.Body.Bytes(), &bootstrap); err != nil {
		t.Fatalf("decode bootstrap: %v", err)
	}
	if len(bootstrap.ToolActions) != 1 {
		t.Fatalf("expected one manifest-backed tool action, got %#v", bootstrap.ToolActions)
	}
	action := bootstrap.ToolActions[0]
	if action.ID != "local" || action.PackID != "local" || action.PackName != "local-pack" || action.CommandText == "" || action.RunTimeoutMS != 9000 {
		t.Fatalf("unexpected action projection: %#v", action)
	}

	runRecorder := httptest.NewRecorder()
	server.handleToolchainActionByID(runRecorder, httptest.NewRequest(http.MethodPost, "/api/toolchain/actions/local/run", strings.NewReader(`{"arguments":"--target desktop.local"}`)))
	if runRecorder.Code != http.StatusAccepted {
		t.Fatalf("expected tool action run accepted, status=%d body=%s", runRecorder.Code, runRecorder.Body.String())
	}
	var started ToolchainActionRunResponseDTO
	if err := json.Unmarshal(runRecorder.Body.Bytes(), &started); err != nil {
		t.Fatalf("decode tool action response: %v", err)
	}
	if started.Action.ID != "local" || started.Execution.ID == "" || !strings.Contains(started.Execution.Command, "sh") {
		t.Fatalf("unexpected tool action response: %#v", started)
	}
	completed := waitExecutionStatus(t, server, started.Execution.ID, "completed")
	if completed["exitCode"] != float64(0) {
		t.Fatalf("expected zero exit code, got %#v", completed)
	}
	logs := recordExecutionSubresource(t, server, started.Execution.ID, "logs")
	if !strings.Contains(logs, "tool-action-log") {
		t.Fatalf("expected tool action log output, got %s", logs)
	}
	artifacts := recordExecutionSubresource(t, server, started.Execution.ID, "artifacts")
	if !strings.Contains(artifacts, "tool-action.txt") || !strings.Contains(artifacts, "--target desktop.local") {
		t.Fatalf("expected tool action artifact with arguments, got %s", artifacts)
	}
	events, err := store.Replay(context.Background(), started.Execution.ID)
	if err != nil {
		t.Fatalf("replay tool action execution: %v", err)
	}
	joinedPayloads := make([]string, 0, len(events))
	for _, event := range events {
		joinedPayloads = append(joinedPayloads, string(event.Payload))
	}
	for _, want := range []string{"tool_action_id", "local", "tool-action-log"} {
		if !strings.Contains(strings.Join(joinedPayloads, "\n"), want) {
			t.Fatalf("tool action eventlog missing %q:\n%s", want, strings.Join(joinedPayloads, "\n"))
		}
	}
}

func TestServerMissionReadSaveAndStartExecution(t *testing.T) {
	skipIfWindows(t)
	ctx := context.Background()
	workspace := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspace, "replay"), 0o755); err != nil {
		t.Fatalf("mkdir replay: %v", err)
	}
	mustWrite(t, filepath.Join(workspace, "mission.yaml"), strings.Join([]string{
		"version: 1",
		"mission:",
		"  name: old-mission",
		"  goal: old goal",
		"  owner: retained",
		"",
	}, "\n"))
	store, err := eventlog.OpenSQLite(filepath.Join(workspace, "replay/events.db"))
	if err != nil {
		t.Fatalf("open event store: %v", err)
	}
	defer store.Close()
	server := &Server{workspace: workspace, eventStore: store}

	readRecorder := httptest.NewRecorder()
	server.handleMission(readRecorder, httptest.NewRequest(http.MethodGet, "/api/mission", nil))
	if readRecorder.Code != http.StatusOK || !strings.Contains(readRecorder.Body.String(), "old goal") {
		t.Fatalf("expected current mission, status=%d body=%s", readRecorder.Code, readRecorder.Body.String())
	}

	saveBody := `{"name":"真实任务","goal":"完成真实工作区启动","command":"printf 'mission-run\\n'"}`
	saveRecorder := httptest.NewRecorder()
	server.handleMission(saveRecorder, httptest.NewRequest(http.MethodPut, "/api/mission", strings.NewReader(saveBody)))
	if saveRecorder.Code != http.StatusOK || !strings.Contains(saveRecorder.Body.String(), "真实任务") {
		t.Fatalf("expected saved mission, status=%d body=%s", saveRecorder.Code, saveRecorder.Body.String())
	}
	missionData, err := os.ReadFile(filepath.Join(workspace, "mission.yaml"))
	if err != nil {
		t.Fatalf("read mission yaml: %v", err)
	}
	for _, want := range []string{"真实任务", "完成真实工作区启动", "printf 'mission-run", "owner: retained"} {
		if !strings.Contains(string(missionData), want) {
			t.Fatalf("saved mission missing %q:\n%s", want, missionData)
		}
	}

	startBody := `{"goal":"通过 API 启动任务","command":"printf 'mission-start-log\\n'; printf 'mission-artifact\\n' > \"$OURO_ARTIFACT_DIR/mission.txt\""}`
	startRecorder := httptest.NewRecorder()
	server.handleMissionStart(startRecorder, httptest.NewRequest(http.MethodPost, "/api/mission/start", strings.NewReader(startBody)))
	if startRecorder.Code != http.StatusAccepted {
		t.Fatalf("expected mission start accepted, status=%d body=%s", startRecorder.Code, startRecorder.Body.String())
	}
	var started struct {
		Mission   MissionDTO   `json:"mission"`
		Execution ExecutionDTO `json:"execution"`
	}
	if err := json.Unmarshal(startRecorder.Body.Bytes(), &started); err != nil {
		t.Fatalf("decode mission start: %v", err)
	}
	if started.Mission.Goal != "通过 API 启动任务" || started.Execution.ID == "" {
		t.Fatalf("unexpected mission start response: %#v", started)
	}
	completed := waitExecutionStatus(t, server, started.Execution.ID, "completed")
	if completed["exitCode"] != float64(0) {
		t.Fatalf("expected zero exit code, got %#v", completed)
	}
	logs := recordExecutionSubresource(t, server, started.Execution.ID, "logs")
	if !strings.Contains(logs, "mission-start-log") {
		t.Fatalf("expected mission execution logs, got %s", logs)
	}
	artifacts := recordExecutionSubresource(t, server, started.Execution.ID, "artifacts")
	if !strings.Contains(artifacts, "mission.txt") || !strings.Contains(artifacts, "mission-artifact") {
		t.Fatalf("expected mission artifact, got %s", artifacts)
	}
	events, err := store.Replay(ctx, started.Execution.ID)
	if err != nil {
		t.Fatalf("replay mission execution: %v", err)
	}
	joinedEvents := make([]string, 0, len(events))
	for _, event := range events {
		joinedEvents = append(joinedEvents, string(event.Payload))
	}
	if !strings.Contains(strings.Join(joinedEvents, "\n"), "通过 API 启动任务") {
		t.Fatalf("mission goal was not written to eventlog: %#v", joinedEvents)
	}
}

func TestServerExecutionRunLogsArtifactsAndEvents(t *testing.T) {
	skipIfWindows(t)
	ctx := context.Background()
	workspace := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspace, "replay"), 0o755); err != nil {
		t.Fatalf("mkdir replay: %v", err)
	}
	store, err := eventlog.OpenSQLite(filepath.Join(workspace, "replay/events.db"))
	if err != nil {
		t.Fatalf("open event store: %v", err)
	}
	defer store.Close()
	server := &Server{workspace: workspace, eventStore: store}

	body := `{"command":"printf '真实执行日志\\n'; printf 'artifact-data\\n' > \"$OURO_ARTIFACT_DIR/result.txt\""}`
	createRequest := httptest.NewRequest(http.MethodPost, "/api/executions", strings.NewReader(body))
	createRecorder := httptest.NewRecorder()
	server.handleExecutions(createRecorder, createRequest)
	if createRecorder.Code != http.StatusAccepted {
		t.Fatalf("expected accepted execution creation, status=%d body=%s", createRecorder.Code, createRecorder.Body.String())
	}

	var created map[string]any
	if err := json.Unmarshal(createRecorder.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	executionID, _ := created["id"].(string)
	if strings.TrimSpace(executionID) == "" {
		t.Fatalf("execution response missing id: %s", createRecorder.Body.String())
	}
	if label, _ := created["label"].(string); strings.TrimSpace(label) == "" || label != created["command"] {
		t.Fatalf("execution response must include backend-authored label matching the display command, got %#v", created)
	}

	completed := waitExecutionStatus(t, server, executionID, "completed")
	if completed["exitCode"] != float64(0) {
		t.Fatalf("expected zero exit code, got %#v", completed)
	}

	logs := recordExecutionSubresource(t, server, executionID, "logs")
	if !strings.Contains(logs, "真实执行日志") {
		t.Fatalf("expected real command output in logs, got %s", logs)
	}

	artifacts := recordExecutionSubresource(t, server, executionID, "artifacts")
	if !strings.Contains(artifacts, "result.txt") || !strings.Contains(artifacts, "artifact-data") {
		t.Fatalf("expected real artifact listing, got %s", artifacts)
	}
	if !strings.Contains(artifacts, `"filePath":"executions/`+executionID+`/artifacts/result.txt"`) {
		t.Fatalf("expected workspace-relative artifact filePath, got %s", artifacts)
	}

	events, err := store.Replay(ctx, executionID)
	if err != nil {
		t.Fatalf("replay execution events: %v", err)
	}
	joinedPayloads := make([]string, 0, len(events))
	for _, event := range events {
		joinedPayloads = append(joinedPayloads, string(event.Payload))
	}
	joined := strings.Join(joinedPayloads, "\n")
	for _, want := range []string{"execution_status", "started", "completed", "真实执行日志"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("execution events missing %q:\n%s", want, joined)
		}
	}
}

func TestServerExecutionStopAndRetry(t *testing.T) {
	skipIfWindows(t)
	workspace := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspace, "replay"), 0o755); err != nil {
		t.Fatalf("mkdir replay: %v", err)
	}
	store, err := eventlog.OpenSQLite(filepath.Join(workspace, "replay/events.db"))
	if err != nil {
		t.Fatalf("open event store: %v", err)
	}
	defer store.Close()
	server := &Server{workspace: workspace, eventStore: store}

	createRequest := httptest.NewRequest(http.MethodPost, "/api/executions", strings.NewReader(`{"command":"sleep 10"}`))
	createRecorder := httptest.NewRecorder()
	server.handleExecutions(createRecorder, createRequest)
	if createRecorder.Code != http.StatusAccepted {
		t.Fatalf("expected accepted execution creation, status=%d body=%s", createRecorder.Code, createRecorder.Body.String())
	}
	var created map[string]any
	if err := json.Unmarshal(createRecorder.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	executionID, _ := created["id"].(string)
	if executionID == "" {
		t.Fatalf("execution response missing id: %s", createRecorder.Body.String())
	}

	stopRequest := httptest.NewRequest(http.MethodPost, "/api/executions/"+executionID+"/stop", nil)
	stopRecorder := httptest.NewRecorder()
	server.handleExecutionByID(stopRecorder, stopRequest)
	if stopRecorder.Code != http.StatusOK {
		t.Fatalf("expected stop ok, status=%d body=%s", stopRecorder.Code, stopRecorder.Body.String())
	}
	waitExecutionStatus(t, server, executionID, "stopped")

	retryRequest := httptest.NewRequest(http.MethodPost, "/api/executions/"+executionID+"/retry", nil)
	retryRecorder := httptest.NewRecorder()
	server.handleExecutionByID(retryRecorder, retryRequest)
	if retryRecorder.Code != http.StatusAccepted {
		t.Fatalf("expected retry accepted, status=%d body=%s", retryRecorder.Code, retryRecorder.Body.String())
	}
	var retried map[string]any
	if err := json.Unmarshal(retryRecorder.Body.Bytes(), &retried); err != nil {
		t.Fatalf("decode retry response: %v", err)
	}
	if retried["id"] == executionID || strings.TrimSpace(retried["retryOf"].(string)) != executionID {
		t.Fatalf("expected retry to create new execution linked to original, got %#v", retried)
	}
}

func TestServerStopCancelsActiveExecutions(t *testing.T) {
	workspace := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspace, "replay"), 0o755); err != nil {
		t.Fatalf("mkdir replay: %v", err)
	}
	store, err := eventlog.OpenSQLite(filepath.Join(workspace, "replay/events.db"))
	if err != nil {
		t.Fatalf("open event store: %v", err)
	}
	defer store.Close()
	server := &Server{workspace: workspace, eventStore: store}

	createRequest := httptest.NewRequest(http.MethodPost, "/api/executions", strings.NewReader(`{"command":"sleep 10"}`))
	createRecorder := httptest.NewRecorder()
	server.handleExecutions(createRecorder, createRequest)
	if createRecorder.Code != http.StatusAccepted {
		t.Fatalf("expected accepted execution creation, status=%d body=%s", createRecorder.Code, createRecorder.Body.String())
	}
	var created map[string]any
	if err := json.Unmarshal(createRecorder.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	executionID, _ := created["id"].(string)
	if executionID == "" {
		t.Fatalf("execution response missing id: %s", createRecorder.Body.String())
	}

	if err := server.Stop(context.Background()); err != nil {
		t.Fatalf("stop server: %v", err)
	}
	waitExecutionStatus(t, server, executionID, "stopped")
}

func TestServerExecutionBroadcastsWebSocketEvents(t *testing.T) {
	skipIfWindows(t)
	workspace := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspace, "replay"), 0o755); err != nil {
		t.Fatalf("mkdir replay: %v", err)
	}
	store, err := eventlog.OpenSQLite(filepath.Join(workspace, "replay/events.db"))
	if err != nil {
		t.Fatalf("open event store: %v", err)
	}
	defer store.Close()
	server := &Server{workspace: workspace, eventStore: store, clients: map[*websocket.Conn]bool{}}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/events/stream", server.handleWebSocket)
	mux.HandleFunc("/api/executions", server.handleExecutions)
	httpServer := httptest.NewServer(mux)
	defer httpServer.Close()

	streamURL := "ws" + strings.TrimPrefix(httpServer.URL, "http") + "/api/events/stream"
	conn, _, err := websocket.DefaultDialer.Dial(streamURL, nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	defer conn.Close()

	welcome := readWebSocketEvent(t, conn)
	if welcome["type"] != "connected" {
		t.Fatalf("expected connected websocket event, got %#v", welcome)
	}

	response, err := http.Post(httpServer.URL+"/api/executions", "application/json", strings.NewReader(`{"command":"printf '实时事件\\n'"}`))
	if err != nil {
		t.Fatalf("post execution: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusAccepted {
		t.Fatalf("expected accepted execution creation, got %d", response.StatusCode)
	}
	var created map[string]any
	if err := json.NewDecoder(response.Body).Decode(&created); err != nil {
		t.Fatalf("decode execution: %v", err)
	}
	executionID, _ := created["id"].(string)
	if executionID == "" {
		t.Fatalf("execution response missing id: %#v", created)
	}

	seenStarted := false
	seenCompleted := false
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) && (!seenStarted || !seenCompleted) {
		event := readWebSocketEvent(t, conn)
		payload, _ := event["payload"].(map[string]any)
		if payload["id"] != executionID {
			continue
		}
		switch event["type"] {
		case "execution:started":
			seenStarted = true
		case "execution:completed":
			seenCompleted = true
		}
	}
	if !seenStarted || !seenCompleted {
		t.Fatalf("expected execution websocket started/completed events, started=%v completed=%v", seenStarted, seenCompleted)
	}
}

func readWebSocketEvent(t *testing.T, conn *websocket.Conn) map[string]any {
	t.Helper()
	if err := conn.SetReadDeadline(time.Now().Add(1 * time.Second)); err != nil {
		t.Fatalf("set websocket deadline: %v", err)
	}
	_, data, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read websocket event: %v", err)
	}
	var event map[string]any
	if err := json.Unmarshal(data, &event); err != nil {
		t.Fatalf("decode websocket event: %v", err)
	}
	return event
}

func waitExecutionStatus(t *testing.T, server *Server, executionID string, want string) map[string]any {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	var last map[string]any
	for time.Now().Before(deadline) {
		request := httptest.NewRequest(http.MethodGet, "/api/executions/"+executionID, nil)
		recorder := httptest.NewRecorder()
		server.handleExecutionByID(recorder, request)
		if recorder.Code != http.StatusOK {
			t.Fatalf("execution status failed, status=%d body=%s", recorder.Code, recorder.Body.String())
		}
		last = map[string]any{}
		if err := json.Unmarshal(recorder.Body.Bytes(), &last); err != nil {
			t.Fatalf("decode execution status: %v", err)
		}
		if last["status"] == want {
			return last
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("execution %s did not reach %s, last=%#v", executionID, want, last)
	return nil
}

func recordExecutionSubresource(t *testing.T, server *Server, executionID string, resource string) string {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, "/api/executions/"+executionID+"/"+resource, nil)
	recorder := httptest.NewRecorder()
	server.handleExecutionByID(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("execution %s failed, status=%d body=%s", resource, recorder.Code, recorder.Body.String())
	}
	return recorder.Body.String()
}

func recordHandler(t *testing.T, handler http.HandlerFunc) string {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	recorder := httptest.NewRecorder()
	handler(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("handler returned status %d body %s", recorder.Code, recorder.Body.String())
	}
	return recorder.Body.String()
}
