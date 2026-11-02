package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ouroboros/ouroboros/internal/eventlog"
)

// writeEgressScope writes a scope.yaml with the given policy, targets and
// infrastructure entries into the test workspace.
func writeEgressScope(t *testing.T, workspace string, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(workspace, "scope.yaml"), []byte(content), 0o644); err != nil {
		t.Fatalf("write scope.yaml: %v", err)
	}
}

// egressTestServer builds a Server wired to a fresh workspace with an event
// store, mirroring the existing api test harness shape.
func egressTestServer(t *testing.T) (*Server, string) {
	t.Helper()
	workspace := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspace, "replay"), 0o755); err != nil {
		t.Fatalf("mkdir replay: %v", err)
	}
	store, err := eventlog.OpenSQLite(filepath.Join(workspace, "replay/events.db"))
	if err != nil {
		t.Fatalf("open event store: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	return &Server{workspace: workspace, eventStore: store}, workspace
}

const egressEnforceScope = `version: 1
scope:
  target_policy: enforce
  targets:
    - kind: url
      value: http://target.local
    - kind: url
      value: http://mirror.target.local
  exclusions:
    - http://mirror.target.local/
  infrastructure:
    - kind: url
      value: http://vpn.infra.local:8443
    - kind: url
      value: http://platform.api.local
execution:
  isolation: process
`

const egressDisabledScope = `version: 1
scope:
  target_policy: disabled
  targets: []
  exclusions: []
  infrastructure:
    - kind: url
      value: http://vpn.infra.local:8443
    - kind: url
      value: http://platform.api.local
execution:
  isolation: process
`

// TestCheckExecutionEgressPolicyMatrix covers acceptance ①-⑤: the disabled
// policy still rejects infrastructure URLs but passes ordinary ones, the
// enforce policy passes in-scope hosts and rejects out-of-scope ones, and
// URL-less local commands always pass.
func TestCheckExecutionEgressPolicyMatrix(t *testing.T) {
	cases := []struct {
		name     string
		scope    string
		command  string
		wantErr  bool
		wantFrag string
	}{
		{name: "disabled rejects infra URL", scope: egressDisabledScope, command: "curl http://vpn.infra.local:8443/health", wantErr: true, wantFrag: "基础设施端点"},
		{name: "disabled passes ordinary URL", scope: egressDisabledScope, command: "curl http://anything.example.com/", wantErr: false},
		{name: "disabled passes local command", scope: egressDisabledScope, command: "ls -la ./replay", wantErr: false},
		{name: "enforce passes in-scope host", scope: egressEnforceScope, command: "curl -s http://target.local/api/v1/probe -o /dev/null", wantErr: false},
		{name: "enforce rejects out-of-scope host", scope: egressEnforceScope, command: "curl http://off-scope.example.net/x", wantErr: true, wantFrag: "域外端点"},
		{name: "enforce rejects excluded host", scope: egressEnforceScope, command: "curl http://mirror.target.local/data", wantErr: true, wantFrag: "排除项端点"},
		{name: "enforce passes local command", scope: egressEnforceScope, command: "grep -r TODO .", wantErr: false},
		{name: "enforce rejects second URL out of scope", scope: egressEnforceScope, command: "curl http://target.local/ && curl http://sneaky.example.org/y", wantErr: true, wantFrag: "sneaky.example.org"},
		{name: "enforce rejects https sibling of scoped http", scope: egressEnforceScope, command: "curl https://target.local/z", wantErr: true, wantFrag: "域外端点"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server, workspace := egressTestServer(t)
			writeEgressScope(t, workspace, tc.scope)
			err := server.checkExecutionEgress(workspace, tc.command)
			if tc.wantErr && err == nil {
				t.Fatalf("expected rejection for command %q, got nil", tc.command)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("expected pass for command %q, got %v", tc.command, err)
			}
			if tc.wantFrag != "" && (err == nil || !strings.Contains(err.Error(), tc.wantFrag)) {
				t.Fatalf("rejection for %q missing fragment %q: %v", tc.command, tc.wantFrag, err)
			}
		})
	}
}

// TestCheckExecutionEgressEnvBypass covers acceptance ⑥: OURO_EGRESS_GATE=0
// disables every gate decision, including the infrastructure rejection.
func TestCheckExecutionEgressEnvBypass(t *testing.T) {
	server, workspace := egressTestServer(t)
	writeEgressScope(t, workspace, egressDisabledScope)

	t.Setenv(egressGateEnv, "0")
	if err := server.checkExecutionEgress(workspace, "curl http://vpn.infra.local:8443/health"); err != nil {
		t.Fatalf("expected env bypass to allow infra command, got %v", err)
	}
	if err := server.checkExecutionEgress(workspace, "curl http://anything.example.com/"); err != nil {
		t.Fatalf("expected env bypass to allow ordinary command, got %v", err)
	}

	t.Setenv(egressGateEnv, "")
	writeEgressScope(t, workspace, egressEnforceScope)
	if err := server.checkExecutionEgress(workspace, "curl http://off-scope.example.net/x"); err == nil {
		t.Fatalf("expected gate re-armed after env cleared, got nil error")
	}
}

// TestCheckExecutionEgressFailClosed covers the unreadable-scope case: with
// egress present and no scope.yaml the gate fails closed with a readable
// error, while a local command still passes.
func TestCheckExecutionEgressFailClosed(t *testing.T) {
	server, workspace := egressTestServer(t)
	if err := server.checkExecutionEgress(workspace, "curl http://target.local/x"); err == nil {
		t.Fatalf("expected fail-closed rejection without scope.yaml, got nil")
	}
	if err := server.checkExecutionEgress(workspace, "ls -la"); err != nil {
		t.Fatalf("expected local command to pass without scope.yaml, got %v", err)
	}
}

// TestStartExecutionEgressRejectsBeforeSideEffects covers acceptance ⑦: a
// rejected command must not spawn a process nor create any execution record
// in the workspace executions/ directory, and the rejection must be visible
// through the same /api/executions entry.
func TestStartExecutionEgressRejectsBeforeSideEffects(t *testing.T) {
	skipIfWindows(t)
	server, workspace := egressTestServer(t)
	writeEgressScope(t, workspace, egressEnforceScope)

	createRequest := httptest.NewRequest(http.MethodPost, "/api/executions", strings.NewReader(`{"command":"curl http://off-scope.example.net/x"}`))
	createRecorder := httptest.NewRecorder()
	server.handleExecutions(createRecorder, createRequest)
	if createRecorder.Code != http.StatusBadRequest {
		t.Fatalf("expected egress rejection to surface as 400, status=%d body=%s", createRecorder.Code, createRecorder.Body.String())
	}
	if !strings.Contains(createRecorder.Body.String(), "执行出站门拒绝") || !strings.Contains(createRecorder.Body.String(), "off-scope.example.net") {
		t.Fatalf("rejection body must name the gate and the refused host, got %s", createRecorder.Body.String())
	}

	entries, err := os.ReadDir(filepath.Join(workspace, "executions"))
	if err == nil && len(entries) != 0 {
		t.Fatalf("rejected execution must leave no record, found %d entries: %v", len(entries), entries)
	}

	// The gate is re-checked through StartExecution directly as well: no
	// process launch, no metadata write.
	execution, err := server.StartExecution(ExecutionCreateRequestDTO{Command: "curl", Args: []string{"http://off-scope.example.net/x"}})
	if err == nil {
		t.Fatalf("expected direct StartExecution rejection, got execution %#v", execution)
	}
	entries, err = os.ReadDir(filepath.Join(workspace, "executions"))
	if err == nil && len(entries) != 0 {
		t.Fatalf("rejected direct start must leave no record, found %d entries: %v", len(entries), entries)
	}

	// A scoped command still flows through the same entry end-to-end.
	okRequest := httptest.NewRequest(http.MethodPost, "/api/executions", strings.NewReader(`{"command":"printf 'egress-ok\\n'"}`))
	okRecorder := httptest.NewRecorder()
	server.handleExecutions(okRecorder, okRequest)
	if okRecorder.Code != http.StatusAccepted {
		t.Fatalf("expected in-policy execution accepted, status=%d body=%s", okRecorder.Code, okRecorder.Body.String())
	}
	var created map[string]any
	if err := json.Unmarshal(okRecorder.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	executionID, _ := created["id"].(string)
	if executionID == "" {
		t.Fatalf("accepted execution missing id: %#v", created)
	}
	_ = waitExecutionStatus(t, server, executionID, "completed")
}
