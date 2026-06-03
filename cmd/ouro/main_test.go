package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ouroboros/ouroboros/internal/app"
	"github.com/ouroboros/ouroboros/internal/blackboard"
	"github.com/ouroboros/ouroboros/internal/compiler"
	"github.com/ouroboros/ouroboros/internal/eventlog"
	"github.com/ouroboros/ouroboros/internal/evidence"
	"github.com/ouroboros/ouroboros/internal/tui"
)

func readFirstSSEEvent(t *testing.T, reader io.Reader) (string, string) {
	t.Helper()

	scanner := bufio.NewScanner(reader)
	var eventName string
	var payload string
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" && payload != "" {
			return eventName, payload
		}
		if strings.HasPrefix(line, "event: ") {
			eventName = strings.TrimSpace(strings.TrimPrefix(line, "event: "))
			continue
		}
		if strings.HasPrefix(line, "data: ") {
			payload = strings.TrimSpace(strings.TrimPrefix(line, "data: "))
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("read SSE event: %v", err)
	}
	t.Fatalf("expected first SSE event, got event=%q payload=%q", eventName, payload)
	return "", ""
}

func TestRootCommandIncludesProductionCommands(t *testing.T) {
	cmd := newRootCommand(new(bytes.Buffer), new(bytes.Buffer))
	names := map[string]bool{}
	for _, child := range cmd.Commands() {
		names[child.Name()] = true
	}

	for _, name := range []string{"init", "run", "report-project", "report-project-dispatch", "report-project-dispatches", "execute-report-project-dispatch", "report-project-dispatch-executions", "report-project-dispatch-execution-annotate", "report-project-dispatch-execution-select", "report-project-dispatch-execution-selection", "report-project-dispatch-execution-selection-clear", "replay", "replay-workbench", "replay-workbench-session", "replay-workbench-session-annotate", "replay-workbench-session-select", "replay-workbench-session-selection", "replay-workbench-session-selection-clear", "replay-workbench-sessions", "replay-workbench-collaboration", "replay-workbench-collaboration-annotate", "replay-workbench-collaboration-select", "replay-workbench-collaboration-selection", "replay-workbench-collaboration-selection-clear", "replay-workbench-collaboration-presence", "replay-workbench-collaboration-presences", "replay-workbench-collaboration-note", "replay-workbench-collaboration-note-annotate", "replay-workbench-collaboration-note-select", "replay-workbench-collaboration-note-selection", "replay-workbench-collaboration-note-selection-clear", "replay-workbench-collaboration-notes", "replay-workbench-collaborations", "toolchain", "runtime", "provider", "doctor"} {
		if !names[name] {
			t.Fatalf("expected root command to include %q", name)
		}
	}
	var runtimeCmdFound bool
	var runtimeNames map[string]bool
	for _, child := range cmd.Commands() {
		if child.Name() != "runtime" {
			continue
		}
		runtimeCmdFound = true
		runtimeNames = map[string]bool{}
		for _, runtimeChild := range child.Commands() {
			runtimeNames[runtimeChild.Name()] = true
		}
	}
	if !runtimeCmdFound {
		t.Fatalf("expected root command to include runtime command")
	}
	for _, name := range []string{"worker-lifecycle-policy-run", "worker-lifecycle-policy-runs", "worker-lifecycle-policy-scheduler-run", "worker-lifecycle-policy-scheduler-runs", "worker-dispatch-retry-scheduler-run", "worker-dispatch-retry-scheduler-runs", "worker-lifecycle-policy-select", "worker-lifecycle-policy-selection", "worker-lifecycle-policy-selection-clear"} {
		if !runtimeNames[name] {
			t.Fatalf("expected runtime command to include %q", name)
		}
	}
}

func TestProviderGenerateCommandUsesOpenAICompatibleProvider(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := app.InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	var captured struct {
		Model     string `json:"model"`
		MaxTokens int    `json:"max_tokens"`
		Messages  []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("expected chat completions path, got %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer secret-value" {
			t.Fatalf("expected bearer header, got %q", got)
		}
		if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
			t.Fatalf("decode generation body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"choices": [{"message": {"role": "assistant", "content": "approved plan"}}],
			"usage": {"prompt_tokens": 11, "completion_tokens": 5, "total_tokens": 16}
		}`))
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
    enabled: true
`, server.URL)
	if err := os.WriteFile(filepath.Join(root, "providers.yaml"), []byte(providers), 0o644); err != nil {
		t.Fatalf("write providers: %v", err)
	}

	var out bytes.Buffer
	cmd := newRootCommand(&out, new(bytes.Buffer))
	cmd.SetArgs([]string{"provider", "generate", root, "--system", "You are Ouroboros.", "--prompt", "Summarize scope.", "--max-tokens", "32"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute provider generate: %v output=%q", err, out.String())
	}
	for _, want := range []string{
		"provider-generate: status=completed",
		"provider=local-openai",
		"model=gpt-test",
		"input_tokens=11",
		"output_tokens=5",
		"total_tokens=16",
		"text=approved plan",
	} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("expected provider generate output to contain %q, got %q", want, out.String())
		}
	}
	if captured.Model != "gpt-test" || captured.MaxTokens != 32 {
		t.Fatalf("unexpected request payload: %#v", captured)
	}
	if len(captured.Messages) != 2 || captured.Messages[0].Role != "system" || captured.Messages[1].Role != "user" {
		t.Fatalf("unexpected request messages: %#v", captured.Messages)
	}
}

func TestProviderGenerateCommandPrintsMetadataAndEstimatedCost(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := app.InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"choices": [{"message": {"role": "assistant", "content": "approved plan"}}],
			"usage": {"prompt_tokens": 1000, "completion_tokens": 500, "total_tokens": 1500}
		}`))
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
    context_window_tokens: 128000
    input_price_per_million_tokens: 2.50
    output_price_per_million_tokens: 10.00
    capabilities:
      - tool_planning
      - json_mode
    enabled: true
`, server.URL)
	if err := os.WriteFile(filepath.Join(root, "providers.yaml"), []byte(providers), 0o644); err != nil {
		t.Fatalf("write providers: %v", err)
	}

	var out bytes.Buffer
	cmd := newRootCommand(&out, new(bytes.Buffer))
	cmd.SetArgs([]string{"provider", "generate", root, "--prompt", "Summarize scope."})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute provider generate: %v output=%q", err, out.String())
	}
	for _, want := range []string{
		"context_window_tokens=128000",
		"estimated_cost_usd=0.007500",
		"capabilities=json_mode|tool_planning",
	} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("expected provider generate output to contain %q, got %q", want, out.String())
		}
	}
}

func TestProviderGenerateCommandRequiresPrompt(t *testing.T) {
	var out bytes.Buffer
	cmd := newRootCommand(&out, new(bytes.Buffer))
	cmd.SetArgs([]string{"provider", "generate", t.TempDir(), "--prompt", "   "})

	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "prompt is required") {
		t.Fatalf("expected prompt-required error, got %v output=%q", err, out.String())
	}
	if out.Len() != 0 {
		t.Fatalf("expected no provider output, got %q", out.String())
	}
}

func TestProviderGenerateCommandRejectsNegativeMaxTokens(t *testing.T) {
	var out bytes.Buffer
	cmd := newRootCommand(&out, new(bytes.Buffer))
	cmd.SetArgs([]string{"provider", "generate", t.TempDir(), "--prompt", "hello", "--max-tokens", "-1"})

	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "max_tokens") {
		t.Fatalf("expected max_tokens error, got %v output=%q", err, out.String())
	}
	if out.Len() != 0 {
		t.Fatalf("expected no provider output, got %q", out.String())
	}
}

func TestInitCommandCreatesWorkspace(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	var out bytes.Buffer
	cmd := newRootCommand(&out, new(bytes.Buffer))
	cmd.SetArgs([]string{"init", root})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute init: %v", err)
	}
	if !strings.Contains(out.String(), "initialized") {
		t.Fatalf("expected init output, got %q", out.String())
	}
}

func TestDoctorCommandChecksWorkspace(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := app.InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}

	var out bytes.Buffer
	cmd := newRootCommand(&out, new(bytes.Buffer))
	cmd.SetArgs([]string{"doctor", root})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute doctor: %v", err)
	}
	if !strings.Contains(out.String(), "workspace: ready") {
		t.Fatalf("expected ready doctor output, got %q", out.String())
	}
}

func TestDoctorCommandPrintsSafetyWarnings(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := app.InitWorkspace(root); err != nil {
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

	var out bytes.Buffer
	cmd := newRootCommand(&out, new(bytes.Buffer))
	cmd.SetArgs([]string{"doctor", root})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute doctor: %v", err)
	}
	output := out.String()
	for _, want := range []string{
		"workspace: ready",
		"workspace: warning: scope.target_policy is disabled",
		"workspace: warning: execution.isolation is disabled",
		"workspace: warning: missing safety bypass authorization for scope.target_policy disabled",
		"workspace: warning: missing safety bypass authorization for execution.isolation disabled",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected %q in doctor output, got %q", want, output)
		}
	}
}

func TestDoctorCommandCanRequireSafetyBypassAuthorization(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := app.InitWorkspace(root); err != nil {
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

	var out bytes.Buffer
	cmd := newRootCommand(&out, new(bytes.Buffer))
	cmd.SetArgs([]string{"doctor", root, "--require-safety-authorization"})

	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "workspace is not ready") {
		t.Fatalf("expected strict doctor command to fail workspace readiness, got %v", err)
	}
	output := out.String()
	for _, want := range []string{
		"workspace: missing required safety bypass authorization for scope.target_policy disabled",
		"workspace: missing required safety bypass authorization for execution.isolation disabled",
		"workspace: warning: scope.target_policy is disabled",
		"workspace: warning: execution.isolation is disabled",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected %q in strict doctor output, got %q", want, output)
		}
	}
}

func TestDoctorCommandCanOutputJSONSafetyReport(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := app.InitWorkspace(root); err != nil {
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

	var out bytes.Buffer
	cmd := newRootCommand(&out, new(bytes.Buffer))
	cmd.SetArgs([]string{"doctor", root, "--format", "json"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute doctor json: %v", err)
	}
	var payload struct {
		SchemaVersion              int      `json:"schema_version"`
		Workspace                  string   `json:"workspace"`
		Ready                      bool     `json:"ready"`
		Errors                     []string `json:"errors"`
		Warnings                   []string `json:"warnings"`
		RequireSafetyAuthorization bool     `json:"require_safety_authorization"`
		StrictGate                 struct {
			Mode       string `json:"mode"`
			Status     string `json:"status"`
			ErrorCount int    `json:"error_count"`
		} `json:"strict_gate"`
		SafetyBypasses []struct {
			Control              string `json:"control"`
			Mode                 string `json:"mode"`
			Authorized           bool   `json:"authorized"`
			MissingAuthorization bool   `json:"missing_authorization"`
		} `json:"safety_bypasses"`
	}
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("decode doctor json: %v output=%q", err, out.String())
	}
	if payload.Workspace != root || !payload.Ready || payload.RequireSafetyAuthorization || len(payload.Errors) != 0 {
		t.Fatalf("unexpected doctor json payload: %#v", payload)
	}
	if payload.SchemaVersion != 1 || payload.StrictGate.Mode != "warning" || payload.StrictGate.Status != "ready" || payload.StrictGate.ErrorCount != 0 {
		t.Fatalf("unexpected doctor json schema/strict gate: %#v", payload)
	}
	if len(payload.SafetyBypasses) != 2 {
		t.Fatalf("expected two structured safety bypasses in doctor json: %#v", payload)
	}
	for _, bypass := range payload.SafetyBypasses {
		if bypass.Mode != "disabled" || bypass.Authorized || !bypass.MissingAuthorization {
			t.Fatalf("unexpected missing-authorization safety bypass in doctor json: %#v", bypass)
		}
	}
	warnings := strings.Join(payload.Warnings, "\n")
	for _, want := range []string{
		"scope.target_policy is disabled",
		"execution.isolation is disabled",
		"missing safety bypass authorization for scope.target_policy disabled",
		"missing safety bypass authorization for execution.isolation disabled",
	} {
		if !strings.Contains(warnings, want) {
			t.Fatalf("expected warning %q in doctor json payload: %#v", want, payload)
		}
	}
	if strings.Contains(out.String(), "workspace: ready") {
		t.Fatalf("expected json format not to include text output, got %q", out.String())
	}
}

func TestDoctorCommandJSONCanRequireSafetyAuthorization(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := app.InitWorkspace(root); err != nil {
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

	var out bytes.Buffer
	cmd := newRootCommand(&out, new(bytes.Buffer))
	cmd.SetArgs([]string{"doctor", root, "--require-safety-authorization", "--format", "json"})

	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "workspace is not ready") {
		t.Fatalf("expected strict doctor json command to fail workspace readiness, got %v", err)
	}
	var payload struct {
		SchemaVersion              int      `json:"schema_version"`
		Workspace                  string   `json:"workspace"`
		Ready                      bool     `json:"ready"`
		Errors                     []string `json:"errors"`
		Warnings                   []string `json:"warnings"`
		RequireSafetyAuthorization bool     `json:"require_safety_authorization"`
		StrictGate                 struct {
			Mode       string `json:"mode"`
			Status     string `json:"status"`
			ErrorCount int    `json:"error_count"`
		} `json:"strict_gate"`
		SafetyBypasses []struct {
			Control              string `json:"control"`
			Mode                 string `json:"mode"`
			Authorized           bool   `json:"authorized"`
			MissingAuthorization bool   `json:"missing_authorization"`
		} `json:"safety_bypasses"`
	}
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("decode strict doctor json: %v output=%q", err, out.String())
	}
	if payload.Workspace != root || payload.Ready || !payload.RequireSafetyAuthorization {
		t.Fatalf("unexpected strict doctor json payload: %#v", payload)
	}
	if payload.SchemaVersion != 1 || payload.StrictGate.Mode != "strict" || payload.StrictGate.Status != "blocked" || payload.StrictGate.ErrorCount != 2 {
		t.Fatalf("unexpected strict doctor json schema/strict gate: %#v", payload)
	}
	if len(payload.SafetyBypasses) != 2 {
		t.Fatalf("expected structured strict safety bypasses in doctor json: %#v", payload)
	}
	for _, bypass := range payload.SafetyBypasses {
		if bypass.Mode != "disabled" || bypass.Authorized || !bypass.MissingAuthorization {
			t.Fatalf("unexpected strict safety bypass metadata in doctor json: %#v", bypass)
		}
	}
	errors := strings.Join(payload.Errors, "\n")
	for _, want := range []string{
		"missing required safety bypass authorization for scope.target_policy disabled",
		"missing required safety bypass authorization for execution.isolation disabled",
	} {
		if !strings.Contains(errors, want) {
			t.Fatalf("expected error %q in strict doctor json payload: %#v", want, payload)
		}
	}
	if len(payload.Warnings) == 0 || !strings.Contains(strings.Join(payload.Warnings, "\n"), "scope.target_policy is disabled") {
		t.Fatalf("expected strict doctor json to retain safety warnings: %#v", payload)
	}
}

func TestDoctorCommandJSONReportsExpiredSafetyAuthorization(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := app.InitWorkspace(root); err != nil {
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
    reason: authorized false positive target boundary
    expires: "2000-01-01"
`
	if err := os.WriteFile(filepath.Join(root, "authorization.yaml"), []byte(authorization), 0o644); err != nil {
		t.Fatalf("write authorization: %v", err)
	}

	var out bytes.Buffer
	cmd := newRootCommand(&out, new(bytes.Buffer))
	cmd.SetArgs([]string{"doctor", root, "--require-safety-authorization", "--format", "json"})

	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "workspace is not ready") {
		t.Fatalf("expected expired strict doctor json command to fail workspace readiness, got %v", err)
	}
	var payload struct {
		Ready      bool     `json:"ready"`
		Errors     []string `json:"errors"`
		Warnings   []string `json:"warnings"`
		StrictGate struct {
			Mode       string `json:"mode"`
			Status     string `json:"status"`
			ErrorCount int    `json:"error_count"`
		} `json:"strict_gate"`
		SafetyBypasses []struct {
			Control              string `json:"control"`
			Authorized           bool   `json:"authorized"`
			AuthorizedBy         string `json:"authorized_by"`
			Expires              string `json:"expires"`
			MissingAuthorization bool   `json:"missing_authorization"`
			ExpiredAuthorization bool   `json:"expired_authorization"`
		} `json:"safety_bypasses"`
	}
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("decode expired strict doctor json: %v output=%q", err, out.String())
	}
	if payload.Ready || payload.StrictGate.Mode != "strict" || payload.StrictGate.Status != "blocked" || payload.StrictGate.ErrorCount != 1 {
		t.Fatalf("unexpected expired strict doctor gate payload: %#v", payload)
	}
	if len(payload.SafetyBypasses) != 1 {
		t.Fatalf("expected one expired safety bypass, got %#v", payload.SafetyBypasses)
	}
	bypass := payload.SafetyBypasses[0]
	if bypass.Control != "scope.target_policy" || bypass.Authorized || bypass.MissingAuthorization || !bypass.ExpiredAuthorization || bypass.AuthorizedBy != "alice" || bypass.Expires != "2000-01-01" {
		t.Fatalf("unexpected expired safety bypass metadata: %#v", bypass)
	}
	if !strings.Contains(strings.Join(payload.Errors, "\n"), "expired required safety bypass authorization for scope.target_policy disabled") {
		t.Fatalf("expected expired strict error, got %#v", payload.Errors)
	}
	if !strings.Contains(strings.Join(payload.Warnings, "\n"), "expired safety bypass authorization for scope.target_policy disabled") {
		t.Fatalf("expected expired warning, got %#v", payload.Warnings)
	}
}

func TestDoctorCommandRequireSafetyAuthorizationAcceptsMetadata(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := app.InitWorkspace(root); err != nil {
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
	authorization := `version: 1
safety_bypasses:
  - control: scope.target_policy
    mode: disabled
    authorized_by: alice
    reason: authorized target-boundary false positive
  - control: execution.isolation
    mode: disabled
    authorized_by: bob
    reason: authorized legacy tool compatibility
`
	if err := os.WriteFile(filepath.Join(root, "authorization.yaml"), []byte(authorization), 0o644); err != nil {
		t.Fatalf("write authorization: %v", err)
	}

	var out bytes.Buffer
	cmd := newRootCommand(&out, new(bytes.Buffer))
	cmd.SetArgs([]string{"doctor", root, "--require-safety-authorization"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute strict doctor with authorization metadata: %v", err)
	}
	output := out.String()
	for _, want := range []string{
		"workspace: ready",
		"workspace: warning: scope.target_policy disabled authorized by alice",
		"workspace: warning: execution.isolation disabled authorized by bob",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected %q in strict authorized doctor output, got %q", want, output)
		}
	}
	if strings.Contains(output, "missing required safety bypass authorization") {
		t.Fatalf("expected strict authorized doctor output not to contain missing authorization errors, got %q", output)
	}
}

func TestRunCommandExecutesWorkspaceRun(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := app.InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}

	var out bytes.Buffer
	cmd := newRootCommand(&out, new(bytes.Buffer))
	cmd.SetArgs([]string{"run", root})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute run: %v", err)
	}
	if !strings.Contains(out.String(), "run:") || !strings.Contains(out.String(), "report:") {
		t.Fatalf("expected run output, got %q", out.String())
	}
}

func TestRunCommandCanDeferReportProjection(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := app.InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}

	var out bytes.Buffer
	cmd := newRootCommand(&out, new(bytes.Buffer))
	cmd.SetArgs([]string{"run", root, "--defer-report-projection"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute run with deferred report projection: %v", err)
	}
	if !strings.Contains(out.String(), "report: not-generated") {
		t.Fatalf("expected deferred run output to show report not-generated, got %q", out.String())
	}
	runID := parseOutputValue(t, out.String(), "run:")

	var replayOut bytes.Buffer
	replayCmd := newRootCommand(&replayOut, new(bytes.Buffer))
	replayCmd.SetArgs([]string{"replay", root, runID})
	if err := replayCmd.Execute(); err != nil {
		t.Fatalf("replay deferred run: %v", err)
	}
	if !strings.Contains(replayOut.String(), "report: not-generated") {
		t.Fatalf("expected replay to show no generated report, got %q", replayOut.String())
	}
}

func TestRunCommandCanSelectFingerprintSurfaceActionForArtifactScope(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := app.InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	artifactPath := filepath.Join(root, "artifacts", "samples", "app.apk")
	if err := os.MkdirAll(filepath.Dir(artifactPath), 0o755); err != nil {
		t.Fatalf("create artifact dir: %v", err)
	}
	if err := os.WriteFile(artifactPath, []byte("apk fixture"), 0o644); err != nil {
		t.Fatalf("write artifact: %v", err)
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
	marker := filepath.Join(t.TempDir(), "cli-action-mobsfscan-ran")
	writeManagedBundleFixtureForCLITest(t, root, "mobsfscan", "#!/bin/sh\nprintf 'cli-mobsfscan args=%s target=%s\\n' \"$*\" \"$OURO_TARGET\"\ntouch "+marker+"\n")
	if _, err := app.ExecuteManagedToolBundleInstall(root, "ctf-pack", "mobsfscan", app.ManagedToolBundleInstallOptions{Execute: true}); err != nil {
		t.Fatalf("install managed mobsfscan bundle: %v", err)
	}

	var out bytes.Buffer
	cmd := newRootCommand(&out, new(bytes.Buffer))
	cmd.SetArgs([]string{"run", root, "--action", "fingerprint_surface", "--defer-report-projection"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute run with fingerprint_surface action: %v", err)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("expected run --action fingerprint_surface to execute mobsfscan payload: %v", err)
	}
	if !strings.Contains(out.String(), "report: not-generated") {
		t.Fatalf("expected deferred action run output to show report not-generated, got %q", out.String())
	}
}

func TestRunCommandRejectsUnknownAction(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := app.InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}

	var out bytes.Buffer
	cmd := newRootCommand(&out, new(bytes.Buffer))
	cmd.SetArgs([]string{"run", root, "--action", "definitely_unknown"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "unknown semantic action") {
		t.Fatalf("expected unknown action error, got err=%v output=%q", err, out.String())
	}
}

func TestReportProjectCommandWritesDeferredRunReport(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := app.InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}

	var runOut bytes.Buffer
	runCmd := newRootCommand(&runOut, new(bytes.Buffer))
	runCmd.SetArgs([]string{"run", root, "--defer-report-projection"})
	if err := runCmd.Execute(); err != nil {
		t.Fatalf("execute deferred run: %v", err)
	}
	runID := parseOutputValue(t, runOut.String(), "run:")

	var reportOut bytes.Buffer
	reportCmd := newRootCommand(&reportOut, new(bytes.Buffer))
	reportCmd.SetArgs([]string{"report-project", root, runID, "--write"})
	if err := reportCmd.Execute(); err != nil {
		t.Fatalf("execute report projection: %v", err)
	}
	for _, want := range []string{"report-project:", "status=completed", "mode=write", "report:"} {
		if !strings.Contains(reportOut.String(), want) {
			t.Fatalf("expected report projection output to contain %q, got %q", want, reportOut.String())
		}
	}
	reportPath := parseOutputValue(t, reportOut.String(), "report:")
	if _, err := os.Stat(reportPath); err != nil {
		t.Fatalf("expected report file: %v", err)
	}
}

func TestReportProjectCommandDryRunDoesNotWriteDeferredRunReport(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := app.InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}

	var runOut bytes.Buffer
	runCmd := newRootCommand(&runOut, new(bytes.Buffer))
	runCmd.SetArgs([]string{"run", root, "--defer-report-projection"})
	if err := runCmd.Execute(); err != nil {
		t.Fatalf("execute deferred run: %v", err)
	}
	runID := parseOutputValue(t, runOut.String(), "run:")

	var reportOut bytes.Buffer
	reportCmd := newRootCommand(&reportOut, new(bytes.Buffer))
	reportCmd.SetArgs([]string{"report-project", root, runID})
	if err := reportCmd.Execute(); err != nil {
		t.Fatalf("dry-run report projection: %v", err)
	}
	for _, want := range []string{"report-project:", "status=dry-run", "mode=metadata-only", "written=false", "planned_report:"} {
		if !strings.Contains(reportOut.String(), want) {
			t.Fatalf("expected dry-run report projection output to contain %q, got %q", want, reportOut.String())
		}
	}
	if strings.Contains(reportOut.String(), "\nreport:") {
		t.Fatalf("dry-run report projection must label the report path as planned, got %q", reportOut.String())
	}
	reportPath := parseOutputValue(t, reportOut.String(), "planned_report:")
	if reportPath == "" || reportPath == "not-generated" {
		t.Fatalf("expected dry-run output to preview a report path, got %q", reportOut.String())
	}
	if _, err := os.Stat(reportPath); !os.IsNotExist(err) {
		t.Fatalf("dry-run report projection must not write report, stat err=%v", err)
	}
	replay, err := app.ReplayWorkspace(context.Background(), root, runID)
	if err != nil {
		t.Fatalf("replay dry-run report projection: %v", err)
	}
	if replay.ReportPath != "" {
		t.Fatalf("dry-run report projection must not emit report_ready, got %q", replay.ReportPath)
	}
}

func TestReportProjectCommandRejectsUnsafeRunIDBeforeWriting(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := app.InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	store, err := eventlog.OpenSQLite(filepath.Join(root, "replay", "events.db"))
	if err != nil {
		t.Fatalf("open event log: %v", err)
	}
	unsafeRunID := "../escape"
	payload, err := json.Marshal(map[string]any{
		"attempt_id": "stage-workspace-bootstrap-completed",
		"kind":       "stage",
		"stage_id":   "workspace-bootstrap",
		"status":     "completed",
	})
	if err != nil {
		t.Fatalf("marshal unsafe run event: %v", err)
	}
	if _, err := store.Append(context.Background(), eventlog.Event{
		RunID:   unsafeRunID,
		Type:    eventlog.EventAttempt,
		Payload: payload,
	}); err != nil {
		t.Fatalf("append unsafe run event: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close event log: %v", err)
	}

	var stderr bytes.Buffer
	cmd := newRootCommand(new(bytes.Buffer), &stderr)
	cmd.SetArgs([]string{"report-project", root, unsafeRunID, "--write"})
	err = cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "report projection run id must be a safe basename") {
		t.Fatalf("expected unsafe run id error, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "escape.md")); !os.IsNotExist(err) {
		t.Fatalf("unsafe report projection must not write outside reports, stat err=%v", err)
	}
}

func TestReportProjectDispatchCommandWritesMetadataOnlyArtifact(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := app.InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}

	var runOut bytes.Buffer
	runCmd := newRootCommand(&runOut, new(bytes.Buffer))
	runCmd.SetArgs([]string{"run", root, "--defer-report-projection"})
	if err := runCmd.Execute(); err != nil {
		t.Fatalf("execute deferred run: %v", err)
	}
	runID := parseOutputValue(t, runOut.String(), "run:")

	var dispatchOut bytes.Buffer
	dispatchCmd := newRootCommand(&dispatchOut, new(bytes.Buffer))
	dispatchCmd.SetArgs([]string{"report-project-dispatch", root, runID, "--actor", "alice", "--reason", "operator review"})
	if err := dispatchCmd.Execute(); err != nil {
		t.Fatalf("create report projection dispatch: %v", err)
	}
	for _, want := range []string{
		"report-project-dispatch:",
		"status=dry-run",
		"mode=metadata-only",
		"actor=alice",
		"events:",
		"planned_report:",
		"artifact:",
	} {
		if !strings.Contains(dispatchOut.String(), want) {
			t.Fatalf("expected report dispatch output to contain %q, got %q", want, dispatchOut.String())
		}
	}
	if strings.Contains(dispatchOut.String(), "\nreport:") {
		t.Fatalf("report dispatch dry-run must label the report path as planned, got %q", dispatchOut.String())
	}
	artifactPath := parseOutputValue(t, dispatchOut.String(), "artifact:")
	if _, err := os.Stat(artifactPath); err != nil {
		t.Fatalf("expected report dispatch artifact: %v", err)
	}
	reportPath := parseOutputValue(t, dispatchOut.String(), "planned_report:")
	if _, err := os.Stat(reportPath); !os.IsNotExist(err) {
		t.Fatalf("report dispatch must not write markdown, stat err=%v", err)
	}
	replay, err := app.ReplayWorkspace(context.Background(), root, runID)
	if err != nil {
		t.Fatalf("replay dry-run report dispatch: %v", err)
	}
	if replay.ReportPath != "" {
		t.Fatalf("report dispatch must not emit report_ready, got %q", replay.ReportPath)
	}
}

func TestReportProjectDispatchesCommandListsAndReadsArtifacts(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := app.InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	run, err := app.RunWorkspaceWithOptions(context.Background(), root, app.RunOptions{DeferReportProjection: true})
	if err != nil {
		t.Fatalf("run workspace with deferred report projection: %v", err)
	}
	dispatch, err := app.CreateReportProjectionDispatch(context.Background(), root, run.RunID, app.ReportProjectionDispatchOptions{
		Actor:  "alice",
		Reason: "operator review",
	})
	if err != nil {
		t.Fatalf("create report dispatch: %v", err)
	}

	var listOut bytes.Buffer
	listCmd := newRootCommand(&listOut, new(bytes.Buffer))
	listCmd.SetArgs([]string{"report-project-dispatches", root})
	if err := listCmd.Execute(); err != nil {
		t.Fatalf("list report dispatch artifacts: %v", err)
	}
	if !strings.Contains(listOut.String(), "report-project-dispatches: total=1") ||
		!strings.Contains(listOut.String(), "report-project-dispatch-artifact: "+dispatch.Path) {
		t.Fatalf("expected report dispatch listing, got %q", listOut.String())
	}

	var readOut bytes.Buffer
	readCmd := newRootCommand(&readOut, new(bytes.Buffer))
	readCmd.SetArgs([]string{"report-project-dispatches", root, filepath.Base(dispatch.Path)})
	if err := readCmd.Execute(); err != nil {
		t.Fatalf("read report dispatch artifact: %v", err)
	}
	for _, want := range []string{
		"report-project-dispatch-artifact: " + dispatch.Path,
		"status=dry-run",
		"mode=metadata-only",
		"run=" + run.RunID,
		"events=",
		"planned_report=" + dispatch.ReportPath,
		"actor=alice",
		"reason=operator_review",
	} {
		if !strings.Contains(readOut.String(), want) {
			t.Fatalf("expected report dispatch read output to contain %q, got %q", want, readOut.String())
		}
	}
	if strings.Contains(readOut.String(), " report=") {
		t.Fatalf("report dispatch artifact dry-run must label the report path as planned, got %q", readOut.String())
	}
}

func TestExecuteReportProjectDispatchCommandRequiresExecuteBeforeWriting(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := app.InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	run, err := app.RunWorkspaceWithOptions(context.Background(), root, app.RunOptions{DeferReportProjection: true})
	if err != nil {
		t.Fatalf("run workspace with deferred report projection: %v", err)
	}
	dispatch, err := app.CreateReportProjectionDispatch(context.Background(), root, run.RunID, app.ReportProjectionDispatchOptions{})
	if err != nil {
		t.Fatalf("create report dispatch: %v", err)
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	cmd := newRootCommand(&out, &errOut)
	cmd.SetArgs([]string{"execute-report-project-dispatch", root, dispatch.Path})
	err = cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "report projection dispatch execution requires explicit execute") {
		t.Fatalf("expected explicit execute error, got %v stdout=%q stderr=%q", err, out.String(), errOut.String())
	}
	executionArtifacts, globErr := filepath.Glob(filepath.Join(root, "reports", "report-projection-dispatch-executions", "*.json"))
	if globErr != nil {
		t.Fatalf("glob report projection execution artifacts: %v", globErr)
	}
	if len(executionArtifacts) != 0 {
		t.Fatalf("missing --execute must not write execution artifacts, got %#v", executionArtifacts)
	}
	if _, err := os.Stat(dispatch.ReportPath); !os.IsNotExist(err) {
		t.Fatalf("missing --execute must not write markdown, stat err=%v", err)
	}
	replay, err := app.ReplayWorkspace(context.Background(), root, run.RunID)
	if err != nil {
		t.Fatalf("replay after rejected execution: %v", err)
	}
	if replay.ReportPath != "" {
		t.Fatalf("missing --execute must not emit report_ready, got %q", replay.ReportPath)
	}
}

func TestExecuteReportProjectDispatchCommandRejectsTamperedDispatchBeforeWriting(t *testing.T) {
	for _, tc := range []struct {
		name      string
		tamper    func(root string, dispatch *app.ReportProjectionDispatch) []string
		wantError string
	}{
		{
			name: "status",
			tamper: func(root string, dispatch *app.ReportProjectionDispatch) []string {
				dispatch.Status = "completed"
				return []string{dispatch.ReportPath}
			},
			wantError: "report projection dispatch must be dry-run metadata-only before execution",
		},
		{
			name: "mode",
			tamper: func(root string, dispatch *app.ReportProjectionDispatch) []string {
				dispatch.Mode = "execute"
				return []string{dispatch.ReportPath}
			},
			wantError: "report projection dispatch must be dry-run metadata-only before execution",
		},
		{
			name: "report-path",
			tamper: func(root string, dispatch *app.ReportProjectionDispatch) []string {
				tamperedReportPath := filepath.Join(root, "reports", "tampered-report.md")
				originalReportPath := dispatch.ReportPath
				dispatch.ReportPath = tamperedReportPath
				return []string{originalReportPath, tamperedReportPath}
			},
			wantError: "report projection dispatch report path mismatch",
		},
		{
			name: "event-count",
			tamper: func(root string, dispatch *app.ReportProjectionDispatch) []string {
				dispatch.EventCount++
				return []string{dispatch.ReportPath}
			},
			wantError: "report projection dispatch event count mismatch",
		},
		{
			name: "replay-digest",
			tamper: func(root string, dispatch *app.ReportProjectionDispatch) []string {
				dispatch.ReplayDigest = "tampered"
				return []string{dispatch.ReportPath}
			},
			wantError: "report projection dispatch replay digest mismatch",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "workspace")
			if err := app.InitWorkspace(root); err != nil {
				t.Fatalf("init workspace: %v", err)
			}
			run, err := app.RunWorkspaceWithOptions(context.Background(), root, app.RunOptions{DeferReportProjection: true})
			if err != nil {
				t.Fatalf("run workspace with deferred report projection: %v", err)
			}
			dispatch, err := app.CreateReportProjectionDispatch(context.Background(), root, run.RunID, app.ReportProjectionDispatchOptions{})
			if err != nil {
				t.Fatalf("create report dispatch: %v", err)
			}
			reportPaths := tc.tamper(root, &dispatch)
			data, err := json.MarshalIndent(dispatch, "", "  ")
			if err != nil {
				t.Fatalf("marshal tampered dispatch: %v", err)
			}
			data = append(data, '\n')
			if err := os.WriteFile(dispatch.Path, data, 0o644); err != nil {
				t.Fatalf("write tampered dispatch: %v", err)
			}

			var out bytes.Buffer
			var errOut bytes.Buffer
			cmd := newRootCommand(&out, &errOut)
			cmd.SetArgs([]string{"execute-report-project-dispatch", root, filepath.Base(dispatch.Path), "--execute"})
			err = cmd.Execute()
			if err == nil || !strings.Contains(err.Error(), tc.wantError) {
				t.Fatalf("expected tampered dispatch error %q, got %v stdout=%q stderr=%q", tc.wantError, err, out.String(), errOut.String())
			}
			if got := countCLIJSON(t, filepath.Join(root, "reports", "report-projection-dispatch-executions")); got != 0 {
				t.Fatalf("tampered dispatch must not write execution artifacts, got %d", got)
			}
			for _, reportPath := range reportPaths {
				if _, err := os.Stat(reportPath); !os.IsNotExist(err) {
					t.Fatalf("tampered dispatch must not write markdown %s, stat err=%v", reportPath, err)
				}
			}
			replay, err := app.ReplayWorkspace(context.Background(), root, run.RunID)
			if err != nil {
				t.Fatalf("replay after rejected tampered dispatch execution: %v", err)
			}
			if replay.ReportPath != "" {
				t.Fatalf("tampered dispatch must not emit report_ready, got %q", replay.ReportPath)
			}
		})
	}
}

func TestExecuteReportProjectDispatchCommandRejectsAlreadyProjectedDispatchWithoutWritingSecondAudit(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := app.InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	run, err := app.RunWorkspaceWithOptions(context.Background(), root, app.RunOptions{DeferReportProjection: true})
	if err != nil {
		t.Fatalf("run workspace with deferred report projection: %v", err)
	}
	dispatch, err := app.CreateReportProjectionDispatch(context.Background(), root, run.RunID, app.ReportProjectionDispatchOptions{})
	if err != nil {
		t.Fatalf("create report dispatch: %v", err)
	}

	var firstOut bytes.Buffer
	firstCmd := newRootCommand(&firstOut, new(bytes.Buffer))
	firstCmd.SetArgs([]string{"execute-report-project-dispatch", root, filepath.Base(dispatch.Path), "--execute"})
	if err := firstCmd.Execute(); err != nil {
		t.Fatalf("execute report dispatch first time: %v output=%q", err, firstOut.String())
	}
	if got := countCLIJSON(t, filepath.Join(root, "reports", "report-projection-dispatch-executions")); got != 1 {
		t.Fatalf("expected one execution artifact after first execution, got %d", got)
	}
	store, err := eventlog.OpenSQLite(filepath.Join(root, "replay", "events.db"))
	if err != nil {
		t.Fatalf("open event log after first execution: %v", err)
	}
	eventsBefore, err := store.Replay(context.Background(), run.RunID)
	if err != nil {
		t.Fatalf("replay before second execution: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close event log before second execution: %v", err)
	}

	var secondOut bytes.Buffer
	var secondErr bytes.Buffer
	secondCmd := newRootCommand(&secondOut, &secondErr)
	secondCmd.SetArgs([]string{"execute-report-project-dispatch", root, filepath.Base(dispatch.Path), "--execute"})
	err = secondCmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "already has a report projection") {
		t.Fatalf("expected already projected error, got %v stdout=%q stderr=%q", err, secondOut.String(), secondErr.String())
	}
	if got := countCLIJSON(t, filepath.Join(root, "reports", "report-projection-dispatch-executions")); got != 1 {
		t.Fatalf("already projected dispatch must not write a second execution artifact, got %d", got)
	}
	store, err = eventlog.OpenSQLite(filepath.Join(root, "replay", "events.db"))
	if err != nil {
		t.Fatalf("reopen event log after second execution: %v", err)
	}
	eventsAfter, err := store.Replay(context.Background(), run.RunID)
	if err != nil {
		t.Fatalf("replay after second execution: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close event log after second execution: %v", err)
	}
	if len(eventsAfter) != len(eventsBefore) {
		t.Fatalf("already projected dispatch must not append replay events, before=%d after=%d", len(eventsBefore), len(eventsAfter))
	}
}

func TestExecuteReportProjectDispatchCommandWritesReportAndListsExecutions(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := app.InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	run, err := app.RunWorkspaceWithOptions(context.Background(), root, app.RunOptions{DeferReportProjection: true})
	if err != nil {
		t.Fatalf("run workspace with deferred report projection: %v", err)
	}
	dispatch, err := app.CreateReportProjectionDispatch(context.Background(), root, run.RunID, app.ReportProjectionDispatchOptions{
		Actor:  "alice",
		Reason: "operator review",
	})
	if err != nil {
		t.Fatalf("create report dispatch: %v", err)
	}

	var executeOut bytes.Buffer
	executeCmd := newRootCommand(&executeOut, new(bytes.Buffer))
	executeCmd.SetArgs([]string{"execute-report-project-dispatch", root, filepath.Base(dispatch.Path), "--execute", "--actor", "bob", "--reason", "approved projection"})
	if err := executeCmd.Execute(); err != nil {
		t.Fatalf("execute report dispatch: %v output=%q", err, executeOut.String())
	}
	for _, want := range []string{
		"report-project-dispatch-execution:",
		"status=completed",
		"mode=execute",
		"dispatch=" + dispatch.Path,
		"run=" + run.RunID,
		"written=true",
		"report: " + dispatch.ReportPath,
		"actor=bob",
		"reason=approved_projection",
		"artifact:",
	} {
		if !strings.Contains(executeOut.String(), want) {
			t.Fatalf("expected execute output to contain %q, got %q", want, executeOut.String())
		}
	}
	executionPath := parseOutputValue(t, executeOut.String(), "artifact:")
	if _, err := os.Stat(executionPath); err != nil {
		t.Fatalf("expected report execution artifact: %v", err)
	}
	if _, err := os.Stat(dispatch.ReportPath); err != nil {
		t.Fatalf("expected markdown report: %v", err)
	}
	executionArtifactStat, err := os.Stat(executionPath)
	if err != nil {
		t.Fatalf("stat report execution artifact before list/read: %v", err)
	}
	executionArtifactData, err := os.ReadFile(executionPath)
	if err != nil {
		t.Fatalf("read report execution artifact before list/read: %v", err)
	}

	executionArtifactCount := countCLIJSON(t, filepath.Join(root, "reports", "report-projection-dispatch-executions"))
	var listOut bytes.Buffer
	listCmd := newRootCommand(&listOut, new(bytes.Buffer))
	listCmd.SetArgs([]string{"report-project-dispatch-executions", root})
	if err := listCmd.Execute(); err != nil {
		t.Fatalf("list report executions: %v", err)
	}
	if !strings.Contains(listOut.String(), "report-project-dispatch-executions: total=1") ||
		!strings.Contains(listOut.String(), "report-project-dispatch-execution-artifact: "+executionPath) ||
		!strings.Contains(listOut.String(), "report="+dispatch.ReportPath) {
		t.Fatalf("expected report execution listing, got %q", listOut.String())
	}
	if got := countCLIJSON(t, filepath.Join(root, "reports", "report-projection-dispatch-executions")); got != executionArtifactCount {
		t.Fatalf("list report executions must stay read-only, before=%d after=%d", executionArtifactCount, got)
	}
	assertFileUnchanged(t, executionPath, executionArtifactStat, executionArtifactData, "list report executions")

	var readOut bytes.Buffer
	readCmd := newRootCommand(&readOut, new(bytes.Buffer))
	readCmd.SetArgs([]string{"report-project-dispatch-executions", root, filepath.Base(executionPath)})
	if err := readCmd.Execute(); err != nil {
		t.Fatalf("read report execution: %v", err)
	}
	for _, want := range []string{
		"report-project-dispatch-execution-artifact: " + executionPath,
		"status=completed",
		"mode=execute",
		"dispatch=" + dispatch.Path,
		"run=" + run.RunID,
		"report=" + dispatch.ReportPath,
		"written=true",
	} {
		if !strings.Contains(readOut.String(), want) {
			t.Fatalf("expected report execution read output to contain %q, got %q", want, readOut.String())
		}
	}
	if got := countCLIJSON(t, filepath.Join(root, "reports", "report-projection-dispatch-executions")); got != executionArtifactCount {
		t.Fatalf("read report execution must stay read-only, before=%d after=%d", executionArtifactCount, got)
	}
	assertFileUnchanged(t, executionPath, executionArtifactStat, executionArtifactData, "read report execution")
}

func TestReportProjectDispatchExecutionAnnotateCommandWritesMetadataOnlyDecision(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	execution := createReportProjectionDispatchExecutionForCLI(t, root)

	var out bytes.Buffer
	cmd := newRootCommand(&out, new(bytes.Buffer))
	cmd.SetArgs([]string{
		"report-project-dispatch-execution-annotate", root, filepath.Base(execution.Path),
		"--decision", "accepted",
		"--reviewer", "alice",
		"--reviewed-at", "2026-06-01T10:15:00Z",
		"--reason", "verified-report-projection",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute report-project-dispatch-execution-annotate: %v", err)
	}
	for _, want := range []string{
		"report-project-dispatch-execution: " + execution.Path + " status=completed mode=execute",
		"written=true",
		"report-project-dispatch-execution-annotation: decision=accepted reviewer=alice reviewed_at=2026-06-01T10:15:00Z reason=verified-report-projection",
	} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("expected report execution annotation output to contain %q, got %q", want, out.String())
		}
	}

	executionArtifactStat, err := os.Stat(execution.Path)
	if err != nil {
		t.Fatalf("stat annotated report execution artifact before list/read: %v", err)
	}
	executionArtifactData, err := os.ReadFile(execution.Path)
	if err != nil {
		t.Fatalf("read annotated report execution artifact before list/read: %v", err)
	}
	executionArtifactCount := countCLIJSON(t, filepath.Join(root, "reports", "report-projection-dispatch-executions"))

	var listOut bytes.Buffer
	listCmd := newRootCommand(&listOut, new(bytes.Buffer))
	listCmd.SetArgs([]string{"report-project-dispatch-executions", root})
	if err := listCmd.Execute(); err != nil {
		t.Fatalf("list annotated report executions: %v", err)
	}
	if !strings.Contains(listOut.String(), "report-project-dispatch-execution-annotation: decision=accepted reviewer=alice reviewed_at=2026-06-01T10:15:00Z reason=verified-report-projection") {
		t.Fatalf("expected annotated report execution listing, got %q", listOut.String())
	}
	if got := countCLIJSON(t, filepath.Join(root, "reports", "report-projection-dispatch-executions")); got != executionArtifactCount {
		t.Fatalf("list annotated report executions must stay read-only, before=%d after=%d", executionArtifactCount, got)
	}
	assertFileUnchanged(t, execution.Path, executionArtifactStat, executionArtifactData, "list annotated report executions")

	var readOut bytes.Buffer
	readCmd := newRootCommand(&readOut, new(bytes.Buffer))
	readCmd.SetArgs([]string{"report-project-dispatch-executions", root, filepath.Base(execution.Path)})
	if err := readCmd.Execute(); err != nil {
		t.Fatalf("read annotated report execution: %v", err)
	}
	if !strings.Contains(readOut.String(), "report-project-dispatch-execution-annotation: decision=accepted reviewer=alice reviewed_at=2026-06-01T10:15:00Z reason=verified-report-projection") {
		t.Fatalf("expected annotated report execution read output, got %q", readOut.String())
	}
	if got := countCLIJSON(t, filepath.Join(root, "reports", "report-projection-dispatch-executions")); got != executionArtifactCount {
		t.Fatalf("read annotated report execution must stay read-only, before=%d after=%d", executionArtifactCount, got)
	}
	assertFileUnchanged(t, execution.Path, executionArtifactStat, executionArtifactData, "read annotated report execution")
}

func TestReportProjectDispatchExecutionSelectionCommandsSelectReadAndClearMetadataOnly(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	execution := createReportProjectionDispatchExecutionForCLI(t, root)
	executionArtifactStat, err := os.Stat(execution.Path)
	if err != nil {
		t.Fatalf("stat report execution artifact before selection commands: %v", err)
	}
	executionArtifactData, err := os.ReadFile(execution.Path)
	if err != nil {
		t.Fatalf("read report execution artifact before selection commands: %v", err)
	}

	var selectOut bytes.Buffer
	selectCmd := newRootCommand(&selectOut, new(bytes.Buffer))
	selectCmd.SetArgs([]string{
		"report-project-dispatch-execution-select", root, filepath.Base(execution.Path),
		"--selected-by", "alice",
		"--selected-at", "2026-06-01T10:20:00Z",
		"--reason", "current-report-projection",
	})
	if err := selectCmd.Execute(); err != nil {
		t.Fatalf("execute report-project-dispatch-execution-select: %v", err)
	}
	for _, want := range []string{
		"report-project-dispatch-execution-selection: status=current mode=metadata-only execution=" + execution.Path,
		"dispatch=" + execution.DispatchPath + " run=" + execution.RunID + " report=" + execution.ReportPath + " written=true",
		"selected_by=alice selected_at=2026-06-01T10:20:00Z reason=current-report-projection",
	} {
		if !strings.Contains(selectOut.String(), want) {
			t.Fatalf("expected report execution selection output to contain %q, got %q", want, selectOut.String())
		}
	}
	assertFileUnchanged(t, execution.Path, executionArtifactStat, executionArtifactData, "select report execution")

	var readOut bytes.Buffer
	readCmd := newRootCommand(&readOut, new(bytes.Buffer))
	readCmd.SetArgs([]string{"report-project-dispatch-execution-selection", root})
	if err := readCmd.Execute(); err != nil {
		t.Fatalf("execute report-project-dispatch-execution-selection: %v", err)
	}
	if !strings.Contains(readOut.String(), "report-project-dispatch-execution-selection: status=current mode=metadata-only execution="+execution.Path) {
		t.Fatalf("expected current report execution selection read output, got %q", readOut.String())
	}
	assertFileUnchanged(t, execution.Path, executionArtifactStat, executionArtifactData, "read report execution selection")

	var clearOut bytes.Buffer
	clearCmd := newRootCommand(&clearOut, new(bytes.Buffer))
	clearCmd.SetArgs([]string{
		"report-project-dispatch-execution-selection-clear", root,
		"--cleared-by", "bob",
		"--cleared-at", "2026-06-01T10:25:00Z",
		"--reason", "retire-current-report",
	})
	if err := clearCmd.Execute(); err != nil {
		t.Fatalf("execute report-project-dispatch-execution-selection-clear: %v", err)
	}
	for _, want := range []string{
		"report-project-dispatch-execution-selection: status=cleared mode=metadata-only execution=" + execution.Path,
		"cleared_by=bob cleared_at=2026-06-01T10:25:00Z reason=retire-current-report",
	} {
		if !strings.Contains(clearOut.String(), want) {
			t.Fatalf("expected cleared report execution selection output to contain %q, got %q", want, clearOut.String())
		}
	}
	assertFileUnchanged(t, execution.Path, executionArtifactStat, executionArtifactData, "clear report execution selection")
}

func TestReportProjectDispatchExecutionSelectionRejectsFailedOrUnwrittenExecutions(t *testing.T) {
	for _, tc := range []struct {
		name     string
		mutate   func(app.ReportProjectionDispatchExecution) app.ReportProjectionDispatchExecution
		wantName string
	}{
		{
			name: "failed",
			mutate: func(execution app.ReportProjectionDispatchExecution) app.ReportProjectionDispatchExecution {
				execution.Status = "failed"
				execution.Written = false
				execution.Error = "report projection failed"
				return execution
			},
			wantName: "failed-report-projection-dispatch-execution.json",
		},
		{
			name: "unwritten",
			mutate: func(execution app.ReportProjectionDispatchExecution) app.ReportProjectionDispatchExecution {
				execution.Status = "completed"
				execution.ReportPath = filepath.Join(filepath.Dir(execution.ReportPath), "planned-but-not-written.md")
				execution.Written = false
				return execution
			},
			wantName: "unwritten-report-projection-dispatch-execution.json",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "workspace")
			execution := createReportProjectionDispatchExecutionForCLI(t, root)
			candidate := tc.mutate(execution)
			candidatePath := writeReportProjectionDispatchExecutionFixture(t, root, tc.wantName, candidate)

			var out bytes.Buffer
			cmd := newRootCommand(&out, new(bytes.Buffer))
			cmd.SetArgs([]string{
				"report-project-dispatch-execution-select", root, filepath.Base(candidatePath),
				"--selected-by", "alice",
				"--reason", "invalid-current-report",
			})
			err := cmd.Execute()
			if err == nil || !strings.Contains(err.Error(), "report projection dispatch execution selection requires completed written execution") {
				t.Fatalf("expected failed/unwritten selection rejection, got %v output=%q", err, out.String())
			}

			var selectionOut bytes.Buffer
			selectionCmd := newRootCommand(&selectionOut, new(bytes.Buffer))
			selectionCmd.SetArgs([]string{"report-project-dispatch-execution-selection", root})
			if err := selectionCmd.Execute(); err != nil {
				t.Fatalf("read report execution selection after rejected select: %v", err)
			}
			if strings.TrimSpace(selectionOut.String()) != "report-project-dispatch-execution-selection: none" {
				t.Fatalf("rejected selection must not write current marker, got %q", selectionOut.String())
			}
		})
	}
}

func TestReportProjectDispatchExecutionSelectCommandRequiresAcceptedAnnotationBeforeMarker(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := app.InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	run, err := app.RunWorkspaceWithOptions(context.Background(), root, app.RunOptions{DeferReportProjection: true})
	if err != nil {
		t.Fatalf("run workspace with deferred report projection: %v", err)
	}
	dispatch, err := app.CreateReportProjectionDispatch(context.Background(), root, run.RunID, app.ReportProjectionDispatchOptions{})
	if err != nil {
		t.Fatalf("create report projection dispatch: %v", err)
	}
	execution, err := app.ExecuteReportProjectionDispatch(context.Background(), root, filepath.Base(dispatch.Path), app.ReportProjectionDispatchExecutionOptions{Execute: true})
	if err != nil {
		t.Fatalf("execute report projection dispatch: %v", err)
	}

	var out bytes.Buffer
	cmd := newRootCommand(&out, new(bytes.Buffer))
	cmd.SetArgs([]string{"report-project-dispatch-execution-select", root, filepath.Base(execution.Path), "--selected-by", "alice"})
	err = cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "report projection dispatch execution selection requires accepted annotation") {
		t.Fatalf("expected accepted annotation selection error, got %v output=%q", err, out.String())
	}
	var selectionOut bytes.Buffer
	selectionCmd := newRootCommand(&selectionOut, new(bytes.Buffer))
	selectionCmd.SetArgs([]string{"report-project-dispatch-execution-selection", root})
	if err := selectionCmd.Execute(); err != nil {
		t.Fatalf("read report execution selection after rejected select: %v", err)
	}
	if strings.TrimSpace(selectionOut.String()) != "report-project-dispatch-execution-selection: none" {
		t.Fatalf("unreviewed selection must not write current marker, got %q", selectionOut.String())
	}
}

func TestReportProjectDispatchExecutionAnnotateCommandRejectsFalsePositiveDecision(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	execution := createReportProjectionDispatchExecutionForCLI(t, root)
	before, err := os.ReadFile(execution.Path)
	if err != nil {
		t.Fatalf("read execution before invalid annotation: %v", err)
	}

	var out bytes.Buffer
	cmd := newRootCommand(&out, new(bytes.Buffer))
	cmd.SetArgs([]string{
		"report-project-dispatch-execution-annotate", root, filepath.Base(execution.Path),
		"--decision", "false-positive",
		"--reviewer", "alice",
	})
	err = cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "unsupported report projection dispatch execution annotation decision") {
		t.Fatalf("expected false-positive decision rejection, got %v output=%q", err, out.String())
	}
	after, err := os.ReadFile(execution.Path)
	if err != nil {
		t.Fatalf("read execution after invalid annotation: %v", err)
	}
	if string(after) != string(before) {
		t.Fatalf("false-positive annotation decision must not rewrite execution artifact")
	}
}

func TestRunCommandCanPrintTUISnapshot(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := app.InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}

	var out bytes.Buffer
	cmd := newRootCommand(&out, new(bytes.Buffer))
	cmd.SetArgs([]string{"run", root, "--tui"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute run with tui snapshot: %v", err)
	}
	for _, want := range []string{"Workspace", "Mission Conversation", "Attack Graph", "Runtime Tabs", "model:offline-model"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("expected TUI run output to contain %q, got %q", want, out.String())
		}
	}
}

func TestRunCommandCanRequireSafetyAuthorization(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := app.InitWorkspace(root); err != nil {
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

	var out bytes.Buffer
	cmd := newRootCommand(&out, new(bytes.Buffer))
	cmd.SetArgs([]string{"run", root, "--require-safety-authorization"})

	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "missing required safety bypass authorization for scope.target_policy disabled") {
		t.Fatalf("expected run strict safety authorization error, got %v output=%q", err, out.String())
	}
	if strings.Contains(out.String(), "run:") {
		t.Fatalf("strict safety authorization failure must not print a successful run summary, got %q", out.String())
	}
}

func TestRunCommandInteractiveStreamsSnapshotsToTUI(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := app.InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}

	session := &fakeInteractiveSession{}
	restore := replaceInteractiveSession(t, session)
	defer restore()

	var out bytes.Buffer
	cmd := newRootCommand(&out, new(bytes.Buffer))
	cmd.SetArgs([]string{"run", root, "--interactive"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute interactive run: %v", err)
	}
	for _, want := range []string{"run:", "report:"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("expected run summary to contain %q, got %q", want, out.String())
		}
	}
	if session.runCount != 1 {
		t.Fatalf("expected interactive session to run once, got %d", session.runCount)
	}
	snapshots := session.replaySnapshots()
	if len(snapshots) == 0 {
		t.Fatalf("expected interactive run to stream replay snapshots, got %#v", session.sent)
	}
	finalSnapshot := snapshots[len(snapshots)-1]
	if finalSnapshot.Status.Model != "offline-model" || finalSnapshot.Status.ToolHealth != "ready" {
		t.Fatalf("expected final streamed snapshot to contain runtime status, got %#v", finalSnapshot.Status)
	}
	finalView := tui.NewModel().WithReplaySnapshot(finalSnapshot).View()
	if !strings.Contains(finalView, "model:offline-model") {
		t.Fatalf("expected final streamed snapshot to render TUI status, got:\n%s", finalView)
	}
}

func TestRunCommandInteractiveCancelControlCancelsRunContext(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := app.InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	scope := `version: 1
scope:
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
  - sleep
  - "10"
parser: web_api_parser
healthcheck: built_in
`
	if err := os.WriteFile(filepath.Join(root, "toolchain", "web-api-pack", "manifest.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	session := &fakeInteractiveSession{sentCh: make(chan tea.Msg, 16)}
	session.runFunc = func(s *fakeInteractiveSession) error {
		select {
		case <-s.sentCh:
		case <-time.After(2 * time.Second):
			t.Fatalf("expected interactive run to stream a snapshot before cancellation")
		}
		armed, cmd := s.initialModel.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
		if cmd != nil {
			t.Fatalf("first cancel keypress should only arm confirmation")
		}
		confirmed, cmd := armed.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
		if cmd == nil {
			t.Fatalf("second cancel keypress should request run cancellation")
		}
		confirmed, _ = confirmed.Update(cmd())
		confirmedModel, ok := confirmed.(tui.Model)
		if !ok {
			t.Fatalf("expected TUI model after cancel result, got %T", confirmed)
		}
		selected := confirmedModel.SelectRuntimeTab(tui.RuntimeTabAgentRuntime)
		resized, _ := selected.Update(tea.WindowSizeMsg{Width: 160, Height: 40})
		s.initialModel = resized
		return nil
	}
	restore := replaceInteractiveSession(t, session)
	defer restore()

	var out bytes.Buffer
	cmd := newRootCommand(&out, new(bytes.Buffer))
	cmd.SetArgs([]string{"run", root, "--interactive"})

	err := cmd.Execute()
	if err == nil {
		t.Fatalf("expected canceled interactive run to return an error")
	}
	if !strings.Contains(err.Error(), "context canceled") {
		t.Fatalf("expected context cancellation error, got %v", err)
	}
	if session.runCount != 1 {
		t.Fatalf("expected interactive session to run once, got %d", session.runCount)
	}
	if session.quitCount == 0 {
		t.Fatalf("expected failed canceled run to quit the interactive session")
	}
	if !strings.Contains(session.initialModel.View(), "run control: cancel requested") {
		t.Fatalf("expected cancel feedback in TUI model, got:\n%s", session.initialModel.View())
	}
}

func TestRunCommandInteractivePauseResumeControlUpdatesTUI(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := app.InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}

	session := &fakeInteractiveSession{}
	session.runFunc = func(s *fakeInteractiveSession) error {
		pausing, cmd := s.initialModel.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
		if cmd == nil {
			t.Fatalf("pause key should request runtime control")
		}
		paused, _ := pausing.Update(cmd())
		resuming, cmd := paused.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
		if cmd == nil {
			t.Fatalf("second pause key should request resume")
		}
		resumed, _ := resuming.Update(cmd())
		resumedModel, ok := resumed.(tui.Model)
		if !ok {
			t.Fatalf("expected TUI model after resume result, got %T", resumed)
		}
		selected := resumedModel.SelectRuntimeTab(tui.RuntimeTabAgentRuntime)
		resized, _ := selected.Update(tea.WindowSizeMsg{Width: 180, Height: 44})
		s.initialModel = resized
		return nil
	}
	restore := replaceInteractiveSession(t, session)
	defer restore()

	var out bytes.Buffer
	cmd := newRootCommand(&out, new(bytes.Buffer))
	cmd.SetArgs([]string{"run", root, "--interactive"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute interactive run with pause/resume controls: %v", err)
	}
	view := session.initialModel.View()
	for _, want := range []string{
		"run control: pause paused",
		"run control: resume resumed",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected pause/resume feedback %q in TUI model, got:\n%s", want, view)
		}
	}
}

func TestReplayCommandReplaysWorkspaceRun(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := app.InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}

	var runOut bytes.Buffer
	runCmd := newRootCommand(&runOut, new(bytes.Buffer))
	runCmd.SetArgs([]string{"run", root})
	if err := runCmd.Execute(); err != nil {
		t.Fatalf("execute run: %v", err)
	}
	runID := parseOutputValue(t, runOut.String(), "run:")

	var replayOut bytes.Buffer
	replayCmd := newRootCommand(&replayOut, new(bytes.Buffer))
	replayCmd.SetArgs([]string{"replay", root, runID})
	if err := replayCmd.Execute(); err != nil {
		t.Fatalf("execute replay: %v", err)
	}
	for _, want := range []string{"replay:", "events:", "model: offline-model", "lane: mission", "report:"} {
		if !strings.Contains(replayOut.String(), want) {
			t.Fatalf("expected replay output to contain %q, got %q", want, replayOut.String())
		}
	}
}

func TestReplayCommandCanExportJSON(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := app.InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}

	var runOut bytes.Buffer
	runCmd := newRootCommand(&runOut, new(bytes.Buffer))
	runCmd.SetArgs([]string{"run", root})
	if err := runCmd.Execute(); err != nil {
		t.Fatalf("execute run: %v", err)
	}
	runID := parseOutputValue(t, runOut.String(), "run:")

	var replayOut bytes.Buffer
	replayCmd := newRootCommand(&replayOut, new(bytes.Buffer))
	replayCmd.SetArgs([]string{"replay", root, runID, "--format", "json"})
	if err := replayCmd.Execute(); err != nil {
		t.Fatalf("execute replay json export: %v", err)
	}
	var exported struct {
		RunID      string         `json:"run_id"`
		EventCount int            `json:"event_count"`
		LaneCounts map[string]int `json:"lane_counts"`
		Status     struct {
			Model      string `json:"model"`
			ToolHealth string `json:"tool_health"`
		} `json:"status"`
		ReportPath string `json:"report_path"`
		Timeline   []struct {
			EventID int64          `json:"event_id"`
			Lane    string         `json:"lane"`
			Type    string         `json:"type"`
			Payload map[string]any `json:"payload"`
		} `json:"timeline"`
	}
	if err := json.Unmarshal(replayOut.Bytes(), &exported); err != nil {
		t.Fatalf("decode replay json export: %v\n%s", err, replayOut.String())
	}
	if exported.RunID != runID || exported.EventCount == 0 || len(exported.Timeline) != exported.EventCount {
		t.Fatalf("unexpected replay json export counts: %#v", exported)
	}
	if exported.LaneCounts["mission"] == 0 || exported.Status.Model != "offline-model" || exported.ReportPath == "" {
		t.Fatalf("expected replay json export to include lane counts, status, and report path: %#v", exported)
	}
	if exported.Timeline[0].EventID == 0 || exported.Timeline[0].Lane == "" || exported.Timeline[0].Type == "" || exported.Timeline[0].Payload == nil {
		t.Fatalf("expected replay json timeline event metadata, got %#v", exported.Timeline[0])
	}
}

func TestReplayCommandCanExportNDJSON(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := app.InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}

	var runOut bytes.Buffer
	runCmd := newRootCommand(&runOut, new(bytes.Buffer))
	runCmd.SetArgs([]string{"run", root})
	if err := runCmd.Execute(); err != nil {
		t.Fatalf("execute run: %v", err)
	}
	runID := parseOutputValue(t, runOut.String(), "run:")

	var replayOut bytes.Buffer
	replayCmd := newRootCommand(&replayOut, new(bytes.Buffer))
	replayCmd.SetArgs([]string{"replay", root, runID, "--format", "ndjson"})
	if err := replayCmd.Execute(); err != nil {
		t.Fatalf("execute replay ndjson export: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(replayOut.String()), "\n")
	if len(lines) == 0 {
		t.Fatalf("expected replay ndjson export lines")
	}
	for i, line := range lines {
		var exported struct {
			EventID int64          `json:"event_id"`
			RunID   string         `json:"run_id"`
			Lane    string         `json:"lane"`
			Type    string         `json:"type"`
			Payload map[string]any `json:"payload"`
		}
		if err := json.Unmarshal([]byte(line), &exported); err != nil {
			t.Fatalf("decode replay ndjson line %d: %v\n%s", i, err, line)
		}
		if exported.EventID == 0 || exported.RunID != runID || exported.Lane == "" || exported.Type == "" || exported.Payload == nil {
			t.Fatalf("unexpected replay ndjson line %d: %#v", i, exported)
		}
	}
}

func TestReplayCommandCanExportHTMLTimeline(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := app.InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}

	var runOut bytes.Buffer
	runCmd := newRootCommand(&runOut, new(bytes.Buffer))
	runCmd.SetArgs([]string{"run", root})
	if err := runCmd.Execute(); err != nil {
		t.Fatalf("execute run: %v", err)
	}
	runID := parseOutputValue(t, runOut.String(), "run:")

	var replayOut bytes.Buffer
	replayCmd := newRootCommand(&replayOut, new(bytes.Buffer))
	replayCmd.SetArgs([]string{"replay", root, runID, "--format", "html"})
	if err := replayCmd.Execute(); err != nil {
		t.Fatalf("execute replay html export: %v", err)
	}
	output := replayOut.String()
	for _, want := range []string{
		"<!doctype html>",
		"<canvas id=\"ouro-replay-timeline\"",
		"window.OURO_REPLAY = {",
		"drawReplayTimeline",
		"timeline event",
		"\"run_id\":\"" + runID + "\"",
		"\"lane_counts\"",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected replay html export to contain %q, got:\n%s", want, output)
		}
	}
}

func TestReplayCommandRejectsHTMLFormatWithTUIFlags(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := app.InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}

	var runOut bytes.Buffer
	runCmd := newRootCommand(&runOut, new(bytes.Buffer))
	runCmd.SetArgs([]string{"run", root})
	if err := runCmd.Execute(); err != nil {
		t.Fatalf("execute run: %v", err)
	}
	runID := parseOutputValue(t, runOut.String(), "run:")

	var replayOut bytes.Buffer
	replayCmd := newRootCommand(&replayOut, new(bytes.Buffer))
	replayCmd.SetArgs([]string{"replay", root, runID, "--format", "html", "--tui"})
	err := replayCmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "replay --format html cannot be combined with --tui or --interactive") {
		t.Fatalf("expected html/tui combination error, got %v output=%q", err, replayOut.String())
	}
}

func TestReplayWorkbenchCommandRejectsNegativeMaxRequestsBeforeReplay(t *testing.T) {
	root := filepath.Join(t.TempDir(), "missing-workspace")

	var out bytes.Buffer
	var stderr bytes.Buffer
	cmd := newRootCommand(&out, &stderr)
	cmd.SetArgs([]string{
		"replay-workbench", root, "run-missing",
		"--listen", "not-a-tcp-address",
		"--max-requests", "-1",
	})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "--max-requests cannot be negative") {
		t.Fatalf("expected max-requests validation error before replay/listen, got %v stdout=%q stderr=%q", err, out.String(), stderr.String())
	}
	if strings.Contains(out.String(), "replay-workbench: listen=") {
		t.Fatalf("negative max-requests must not start listener, got stdout=%q", out.String())
	}
}

func TestReplayWorkbenchCommandServesHTMLAndReplayJSON(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := app.InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	run, err := app.RunWorkspace(context.Background(), root)
	if err != nil {
		t.Fatalf("run workspace: %v", err)
	}
	before, err := app.ReplayWorkspace(context.Background(), root, run.RunID)
	if err != nil {
		t.Fatalf("replay before workbench: %v", err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("allocate listen address: %v", err)
	}
	addr := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatalf("close temporary listener: %v", err)
	}

	var out bytes.Buffer
	cmd := newRootCommand(&out, new(bytes.Buffer))
	cmd.SetArgs([]string{
		"replay-workbench", root, run.RunID,
		"--listen", addr,
		"--max-requests", "2",
	})
	done := make(chan error, 1)
	go func() {
		done <- cmd.Execute()
	}()

	client := http.Client{Timeout: 100 * time.Millisecond}
	var replayPayload replayExport
	var lastErr error
	for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		response, err := client.Get("http://" + addr + "/api/replay")
		if err != nil {
			lastErr = err
			continue
		}
		if response.StatusCode != http.StatusOK {
			_ = response.Body.Close()
			lastErr = fmt.Errorf("unexpected replay API status %d", response.StatusCode)
			continue
		}
		if err := json.NewDecoder(response.Body).Decode(&replayPayload); err != nil {
			_ = response.Body.Close()
			lastErr = err
			continue
		}
		_ = response.Body.Close()
		lastErr = nil
		break
	}
	if lastErr != nil {
		t.Fatalf("get replay workbench API: %v", lastErr)
	}
	if replayPayload.RunID != run.RunID || replayPayload.EventCount != run.EventCount {
		t.Fatalf("unexpected replay workbench payload: %#v", replayPayload)
	}

	response, err := client.Get("http://" + addr + "/")
	if err != nil {
		t.Fatalf("get replay workbench html: %v", err)
	}
	htmlData := new(bytes.Buffer)
	if _, err := htmlData.ReadFrom(response.Body); err != nil {
		_ = response.Body.Close()
		t.Fatalf("read replay workbench html: %v", err)
	}
	_ = response.Body.Close()
	htmlBody := htmlData.String()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("expected replay workbench html 200, got %d body=%q", response.StatusCode, htmlBody)
	}
	for _, want := range []string{
		"<canvas id=\"ouro-replay-timeline\"",
		"window.OURO_REPLAY",
		"timeline event",
		run.RunID,
	} {
		if !strings.Contains(htmlBody, want) {
			t.Fatalf("expected replay workbench html to contain %q, got %q", want, htmlBody)
		}
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("serve replay workbench: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("replay workbench did not exit after max requests")
	}
	after, err := app.ReplayWorkspace(context.Background(), root, run.RunID)
	if err != nil {
		t.Fatalf("replay after workbench: %v", err)
	}
	if after.EventCount != before.EventCount {
		t.Fatalf("replay workbench must not mutate replay events, before=%d after=%d", before.EventCount, after.EventCount)
	}
	for _, want := range []string{
		"replay-workbench: listen=http://" + addr,
		"replay-workbench: accepted=2",
	} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("expected replay workbench output to contain %q, got %q", want, out.String())
		}
	}
}

func TestReplayWorkbenchCommandServesMultiRunComparisonData(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := app.InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	primary, err := app.RunWorkspace(context.Background(), root)
	if err != nil {
		t.Fatalf("run primary workspace: %v", err)
	}
	compare, err := app.RunWorkspace(context.Background(), root)
	if err != nil {
		t.Fatalf("run comparison workspace: %v", err)
	}
	beforePrimary, err := app.ReplayWorkspace(context.Background(), root, primary.RunID)
	if err != nil {
		t.Fatalf("replay primary before workbench: %v", err)
	}
	beforeCompare, err := app.ReplayWorkspace(context.Background(), root, compare.RunID)
	if err != nil {
		t.Fatalf("replay comparison before workbench: %v", err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("allocate listen address: %v", err)
	}
	addr := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatalf("close temporary listener: %v", err)
	}

	var out bytes.Buffer
	cmd := newRootCommand(&out, new(bytes.Buffer))
	cmd.SetArgs([]string{
		"replay-workbench", root, primary.RunID,
		"--compare-run", compare.RunID,
		"--listen", addr,
		"--max-requests", "3",
	})
	done := make(chan error, 1)
	go func() {
		done <- cmd.Execute()
	}()

	client := http.Client{Timeout: 100 * time.Millisecond}
	defer drainReplayWorkbench(client, addr, "/api/replay", "/", "/index.html")
	var replays []replayExport
	var lastErr error
	for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		select {
		case err := <-done:
			t.Fatalf("replay workbench exited before serving comparison data: %v output=%q", err, out.String())
		default:
		}
		response, err := client.Get("http://" + addr + "/api/replays")
		if err != nil {
			lastErr = err
			continue
		}
		if response.StatusCode != http.StatusOK {
			_ = response.Body.Close()
			lastErr = fmt.Errorf("unexpected comparison API status %d", response.StatusCode)
			continue
		}
		if err := json.NewDecoder(response.Body).Decode(&replays); err != nil {
			_ = response.Body.Close()
			lastErr = err
			continue
		}
		_ = response.Body.Close()
		lastErr = nil
		break
	}
	if lastErr != nil {
		t.Fatalf("get replay workbench comparison API: %v", lastErr)
	}
	if len(replays) != 2 || replays[0].RunID != primary.RunID || replays[1].RunID != compare.RunID {
		t.Fatalf("unexpected comparison replay payload: %#v", replays)
	}

	response, err := client.Get("http://" + addr + "/api/replay")
	if err != nil {
		t.Fatalf("get primary replay API: %v", err)
	}
	var primaryPayload replayExport
	if err := json.NewDecoder(response.Body).Decode(&primaryPayload); err != nil {
		_ = response.Body.Close()
		t.Fatalf("decode primary replay API: %v", err)
	}
	_ = response.Body.Close()
	if primaryPayload.RunID != primary.RunID {
		t.Fatalf("expected /api/replay to keep primary run %s, got %#v", primary.RunID, primaryPayload)
	}

	response, err = client.Get("http://" + addr + "/")
	if err != nil {
		t.Fatalf("get comparison workbench html: %v", err)
	}
	htmlData := new(bytes.Buffer)
	if _, err := htmlData.ReadFrom(response.Body); err != nil {
		_ = response.Body.Close()
		t.Fatalf("read comparison workbench html: %v", err)
	}
	_ = response.Body.Close()
	htmlBody := htmlData.String()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("expected comparison workbench html 200, got %d body=%q", response.StatusCode, htmlBody)
	}
	for _, want := range []string{
		"window.OURO_REPLAYS",
		"id=\"ouro-replay-run-select\"",
		primary.RunID,
		compare.RunID,
	} {
		if !strings.Contains(htmlBody, want) {
			t.Fatalf("expected comparison workbench html to contain %q, got %q", want, htmlBody)
		}
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("serve replay workbench comparison: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("replay workbench comparison did not exit after max requests")
	}
	afterPrimary, err := app.ReplayWorkspace(context.Background(), root, primary.RunID)
	if err != nil {
		t.Fatalf("replay primary after workbench: %v", err)
	}
	afterCompare, err := app.ReplayWorkspace(context.Background(), root, compare.RunID)
	if err != nil {
		t.Fatalf("replay comparison after workbench: %v", err)
	}
	if afterPrimary.EventCount != beforePrimary.EventCount || afterCompare.EventCount != beforeCompare.EventCount {
		t.Fatalf("replay workbench must not mutate replay events, primary before/after=%d/%d compare before/after=%d/%d",
			beforePrimary.EventCount, afterPrimary.EventCount, beforeCompare.EventCount, afterCompare.EventCount)
	}
	if !strings.Contains(out.String(), "replay-workbench: accepted=3") {
		t.Fatalf("expected replay workbench accepted count, got %q", out.String())
	}
}

func TestReplayWorkbenchCommandLoadsSavedSessionDefaults(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := app.InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	primary, err := app.RunWorkspace(context.Background(), root)
	if err != nil {
		t.Fatalf("run primary workspace: %v", err)
	}
	comparison, err := app.RunWorkspace(context.Background(), root)
	if err != nil {
		t.Fatalf("run comparison workspace: %v", err)
	}
	session, err := app.CreateReplayWorkbenchSession(context.Background(), root, app.ReplayWorkbenchSessionOptions{
		PrimaryRunID:  primary.RunID,
		CompareRunIDs: []string{comparison.RunID},
		SelectedRunID: comparison.RunID,
		Filters: map[string]string{
			"lane": "toolchain",
			"q":    "needle",
			"edge": "edge-1",
		},
		CreatedBy: "operator",
		Reason:    "restore comparison focus",
	})
	if err != nil {
		t.Fatalf("create replay workbench session: %v", err)
	}
	selection, err := app.SelectReplayWorkbenchSession(root, filepath.Base(session.Path), app.ReplayWorkbenchSessionSelectionOptions{
		SelectedBy: "alice",
		SelectedAt: time.Date(2026, 6, 1, 3, 10, 0, 0, time.UTC),
		Reason:     "current hosted workbench focus",
	})
	if err != nil {
		t.Fatalf("select replay workbench session: %v", err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("allocate listen address: %v", err)
	}
	addr := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatalf("close temporary listener: %v", err)
	}

	var out bytes.Buffer
	cmd := newRootCommand(&out, new(bytes.Buffer))
	cmd.SetArgs([]string{
		"replay-workbench", root, primary.RunID,
		"--session", filepath.Base(session.Path),
		"--listen", addr,
		"--max-requests", "4",
	})
	done := make(chan error, 1)
	go func() {
		done <- cmd.Execute()
	}()

	client := http.Client{Timeout: 100 * time.Millisecond}
	var loadedSession struct {
		SelectedRunID string            `json:"selected_run_id"`
		RunIDs        []string          `json:"run_ids"`
		Filters       map[string]string `json:"filters"`
		Endpoints     map[string]string `json:"endpoints"`
	}
	var lastErr error
	for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		response, err := client.Get("http://" + addr + "/api/session")
		if err != nil {
			lastErr = err
			continue
		}
		if response.StatusCode != http.StatusOK {
			_ = response.Body.Close()
			lastErr = fmt.Errorf("unexpected session API status %d", response.StatusCode)
			continue
		}
		if err := json.NewDecoder(response.Body).Decode(&loadedSession); err != nil {
			_ = response.Body.Close()
			lastErr = err
			continue
		}
		_ = response.Body.Close()
		lastErr = nil
		break
	}
	if lastErr != nil {
		t.Fatalf("get saved replay workbench session API: %v", lastErr)
	}
	if loadedSession.SelectedRunID != comparison.RunID ||
		len(loadedSession.RunIDs) != 2 ||
		loadedSession.Filters["lane"] != "toolchain" ||
		loadedSession.Filters["q"] != "needle" ||
		loadedSession.Filters["edge"] != "edge-1" ||
		loadedSession.Endpoints["html"] != "/?edge=edge-1&lane=toolchain&q=needle&run="+comparison.RunID {
		t.Fatalf("unexpected loaded session state: %#v", loadedSession)
	}

	sessionsResponse, err := client.Get("http://" + addr + "/api/sessions")
	if err != nil {
		t.Fatalf("get saved replay workbench sessions API: %v", err)
	}
	var savedSessions []app.ReplayWorkbenchSessionArtifact
	if err := json.NewDecoder(sessionsResponse.Body).Decode(&savedSessions); err != nil {
		_ = sessionsResponse.Body.Close()
		t.Fatalf("decode saved replay workbench sessions API: %v", err)
	}
	_ = sessionsResponse.Body.Close()
	if sessionsResponse.StatusCode != http.StatusOK {
		t.Fatalf("expected saved sessions API 200, got %d", sessionsResponse.StatusCode)
	}
	if len(savedSessions) != 1 ||
		savedSessions[0].Path != session.Path ||
		savedSessions[0].Reason != "restore comparison focus" ||
		savedSessions[0].Filters["edge"] != "edge-1" {
		t.Fatalf("unexpected saved sessions API payload: %#v", savedSessions)
	}

	selectionResponse, err := client.Get("http://" + addr + "/api/session-selection")
	if err != nil {
		t.Fatalf("get saved replay workbench session selection API: %v", err)
	}
	var loadedSelection app.ReplayWorkbenchSessionSelection
	if err := json.NewDecoder(selectionResponse.Body).Decode(&loadedSelection); err != nil {
		_ = selectionResponse.Body.Close()
		t.Fatalf("decode replay workbench session selection API: %v", err)
	}
	_ = selectionResponse.Body.Close()
	if selectionResponse.StatusCode != http.StatusOK {
		t.Fatalf("expected session selection API 200, got %d", selectionResponse.StatusCode)
	}
	if loadedSelection.SessionPath != selection.SessionPath ||
		loadedSelection.Status != "current" ||
		loadedSelection.SelectedBy != "alice" ||
		loadedSelection.Reason != "current hosted workbench focus" {
		t.Fatalf("unexpected loaded session selection API payload: %#v", loadedSelection)
	}

	response, err := client.Get("http://" + addr + "/")
	if err != nil {
		t.Fatalf("get saved-session workbench html: %v", err)
	}
	htmlData := new(bytes.Buffer)
	if _, err := htmlData.ReadFrom(response.Body); err != nil {
		_ = response.Body.Close()
		t.Fatalf("read saved-session workbench html: %v", err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("expected saved-session workbench html 200, got %d body=%q", response.StatusCode, htmlData.String())
	}
	for _, want := range []string{
		`"selected_run_id":"` + comparison.RunID + `"`,
		"window.OURO_REPLAY_SESSION_SELECTION",
		"current hosted workbench focus",
		`"edge":"edge-1"`,
		comparison.RunID,
	} {
		if !strings.Contains(htmlData.String(), want) {
			t.Fatalf("expected saved-session html to contain %q, got %q", want, htmlData.String())
		}
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("serve replay workbench with saved session: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("replay workbench with saved session did not exit after max requests")
	}
	if !strings.Contains(out.String(), "replay-workbench: session="+session.Path) || !strings.Contains(out.String(), "replay-workbench: accepted=4") {
		t.Fatalf("expected replay workbench saved-session output, got %q", out.String())
	}
}

func TestReplayWorkbenchCommandLoadsCurrentSavedSessionSelection(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := app.InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	primary, err := app.RunWorkspace(context.Background(), root)
	if err != nil {
		t.Fatalf("run primary workspace: %v", err)
	}
	comparison, err := app.RunWorkspace(context.Background(), root)
	if err != nil {
		t.Fatalf("run comparison workspace: %v", err)
	}
	session, err := app.CreateReplayWorkbenchSession(context.Background(), root, app.ReplayWorkbenchSessionOptions{
		PrimaryRunID:  primary.RunID,
		CompareRunIDs: []string{comparison.RunID},
		SelectedRunID: comparison.RunID,
		Filters: map[string]string{
			"lane": "toolchain",
			"q":    "current",
		},
		CreatedBy: "operator",
		Reason:    "current session restore",
	})
	if err != nil {
		t.Fatalf("create replay workbench session: %v", err)
	}
	if _, err := app.SelectReplayWorkbenchSession(root, filepath.Base(session.Path), app.ReplayWorkbenchSessionSelectionOptions{
		SelectedBy: "alice",
		SelectedAt: time.Date(2026, 6, 1, 3, 22, 0, 0, time.UTC),
		Reason:     "operator current session",
	}); err != nil {
		t.Fatalf("select replay workbench session: %v", err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("allocate listen address: %v", err)
	}
	addr := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatalf("close temporary listener: %v", err)
	}

	var out bytes.Buffer
	cmd := newRootCommand(&out, new(bytes.Buffer))
	cmd.SetArgs([]string{
		"replay-workbench", root, primary.RunID,
		"--current-session",
		"--listen", addr,
		"--max-requests", "2",
	})
	done := make(chan error, 1)
	go func() {
		done <- cmd.Execute()
	}()

	client := http.Client{Timeout: 100 * time.Millisecond}
	var loadedSession struct {
		SelectedRunID string            `json:"selected_run_id"`
		RunIDs        []string          `json:"run_ids"`
		Filters       map[string]string `json:"filters"`
	}
	var lastErr error
	for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		response, err := client.Get("http://" + addr + "/api/session")
		if err != nil {
			lastErr = err
			continue
		}
		if response.StatusCode != http.StatusOK {
			_ = response.Body.Close()
			lastErr = fmt.Errorf("unexpected session API status %d", response.StatusCode)
			continue
		}
		if err := json.NewDecoder(response.Body).Decode(&loadedSession); err != nil {
			_ = response.Body.Close()
			lastErr = err
			continue
		}
		_ = response.Body.Close()
		lastErr = nil
		break
	}
	if lastErr != nil {
		t.Fatalf("get current saved replay workbench session API: %v", lastErr)
	}
	if loadedSession.SelectedRunID != comparison.RunID ||
		len(loadedSession.RunIDs) != 2 ||
		loadedSession.Filters["lane"] != "toolchain" ||
		loadedSession.Filters["q"] != "current" {
		t.Fatalf("unexpected current saved session state: %#v", loadedSession)
	}

	selectionResponse, err := client.Get("http://" + addr + "/api/session-selection")
	if err != nil {
		t.Fatalf("get current session selection API: %v", err)
	}
	var selection app.ReplayWorkbenchSessionSelection
	if err := json.NewDecoder(selectionResponse.Body).Decode(&selection); err != nil {
		_ = selectionResponse.Body.Close()
		t.Fatalf("decode current session selection API: %v", err)
	}
	_ = selectionResponse.Body.Close()
	if selection.SessionPath != session.Path || selection.Status != "current" {
		t.Fatalf("unexpected current session selection: %#v", selection)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("serve replay workbench with current session: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("replay workbench with current session did not exit after max requests")
	}
	if !strings.Contains(out.String(), "replay-workbench: session="+session.Path) || !strings.Contains(out.String(), "replay-workbench: accepted=2") {
		t.Fatalf("expected replay workbench current-session output, got %q", out.String())
	}

	var conflict bytes.Buffer
	conflictCmd := newRootCommand(&conflict, new(bytes.Buffer))
	conflictCmd.SetArgs([]string{
		"replay-workbench", root, primary.RunID,
		"--session", filepath.Base(session.Path),
		"--current-session",
		"--max-requests", "1",
	})
	if err := conflictCmd.Execute(); err == nil || !strings.Contains(err.Error(), "--session and --current-session cannot be combined") {
		t.Fatalf("expected current-session/session conflict error, got %v output=%q", err, conflict.String())
	}
}

func TestReplayWorkbenchCommandLoadsCurrentCollaborationSelectionDefaults(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := app.InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	primary, err := app.RunWorkspace(context.Background(), root)
	if err != nil {
		t.Fatalf("run primary workspace: %v", err)
	}
	comparison, err := app.RunWorkspace(context.Background(), root)
	if err != nil {
		t.Fatalf("run comparison workspace: %v", err)
	}
	session, err := app.CreateReplayWorkbenchSession(context.Background(), root, app.ReplayWorkbenchSessionOptions{
		PrimaryRunID:  primary.RunID,
		CompareRunIDs: []string{comparison.RunID},
		SelectedRunID: comparison.RunID,
		Filters: map[string]string{
			"lane": "toolchain",
			"q":    "collaboration",
			"node": "node-1",
		},
		CreatedBy: "operator",
		Reason:    "current collaboration restore",
	})
	if err != nil {
		t.Fatalf("create replay workbench session: %v", err)
	}
	collaboration, err := app.CreateReplayWorkbenchCollaboration(context.Background(), root, app.ReplayWorkbenchCollaborationOptions{
		SessionPath:  filepath.Base(session.Path),
		Participants: []string{"alice", "bob"},
		CreatedBy:    "lead",
		CreatedAt:    time.Date(2026, 6, 1, 5, 30, 0, 0, time.UTC),
		Reason:       "restore current collaboration",
	})
	if err != nil {
		t.Fatalf("create replay workbench collaboration: %v", err)
	}
	if _, err := app.SelectReplayWorkbenchCollaboration(root, filepath.Base(collaboration.Path), app.ReplayWorkbenchCollaborationSelectionOptions{
		SelectedBy: "lead",
		SelectedAt: time.Date(2026, 6, 1, 5, 31, 0, 0, time.UTC),
		Reason:     "operator current collaboration",
	}); err != nil {
		t.Fatalf("select replay workbench collaboration: %v", err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("allocate listen address: %v", err)
	}
	addr := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatalf("close temporary listener: %v", err)
	}

	var out bytes.Buffer
	cmd := newRootCommand(&out, new(bytes.Buffer))
	cmd.SetArgs([]string{
		"replay-workbench", root, primary.RunID,
		"--current-collaboration",
		"--listen", addr,
		"--max-requests", "2",
	})
	done := make(chan error, 1)
	go func() {
		done <- cmd.Execute()
	}()

	client := http.Client{Timeout: 100 * time.Millisecond}
	var loadedSession struct {
		SelectedRunID string            `json:"selected_run_id"`
		RunIDs        []string          `json:"run_ids"`
		Filters       map[string]string `json:"filters"`
	}
	var lastErr error
	for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		response, err := client.Get("http://" + addr + "/api/session")
		if err != nil {
			lastErr = err
			continue
		}
		if response.StatusCode != http.StatusOK {
			_ = response.Body.Close()
			lastErr = fmt.Errorf("unexpected session API status %d", response.StatusCode)
			continue
		}
		if err := json.NewDecoder(response.Body).Decode(&loadedSession); err != nil {
			_ = response.Body.Close()
			lastErr = err
			continue
		}
		_ = response.Body.Close()
		lastErr = nil
		break
	}
	if lastErr != nil {
		t.Fatalf("get current collaboration replay workbench session API: %v", lastErr)
	}
	if loadedSession.SelectedRunID != comparison.RunID ||
		len(loadedSession.RunIDs) != 2 ||
		loadedSession.Filters["lane"] != "toolchain" ||
		loadedSession.Filters["q"] != "collaboration" ||
		loadedSession.Filters["node"] != "node-1" {
		t.Fatalf("unexpected current collaboration session state: %#v", loadedSession)
	}

	selectionResponse, err := client.Get("http://" + addr + "/api/collaboration-selection")
	if err != nil {
		t.Fatalf("get current collaboration selection API: %v", err)
	}
	var selection app.ReplayWorkbenchCollaborationSelection
	if err := json.NewDecoder(selectionResponse.Body).Decode(&selection); err != nil {
		_ = selectionResponse.Body.Close()
		t.Fatalf("decode current collaboration selection API: %v", err)
	}
	_ = selectionResponse.Body.Close()
	if selection.CollaborationPath != collaboration.Path || selection.Status != "current" {
		t.Fatalf("unexpected current collaboration selection: %#v", selection)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("serve replay workbench with current collaboration: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("replay workbench with current collaboration did not exit after max requests")
	}
	if !strings.Contains(out.String(), "replay-workbench: session="+session.Path) || !strings.Contains(out.String(), "replay-workbench: accepted=2") {
		t.Fatalf("expected replay workbench current-collaboration output, got %q", out.String())
	}

	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{
			name: "session conflict",
			args: []string{
				"replay-workbench", root, primary.RunID,
				"--session", filepath.Base(session.Path),
				"--current-collaboration",
				"--max-requests", "1",
			},
			want: "--session and --current-collaboration cannot be combined",
		},
		{
			name: "current session conflict",
			args: []string{
				"replay-workbench", root, primary.RunID,
				"--current-session",
				"--current-collaboration",
				"--max-requests", "1",
			},
			want: "--current-session and --current-collaboration cannot be combined",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var conflict bytes.Buffer
			conflictCmd := newRootCommand(&conflict, new(bytes.Buffer))
			conflictCmd.SetArgs(tc.args)
			if err := conflictCmd.Execute(); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected %q error, got %v output=%q", tc.want, err, conflict.String())
			}
		})
	}

	if _, err := app.ClearReplayWorkbenchCollaborationSelection(root, app.ReplayWorkbenchCollaborationSelectionClearOptions{
		ClearedBy: "lead",
		Reason:    "close current collaboration",
	}); err != nil {
		t.Fatalf("clear replay workbench collaboration selection: %v", err)
	}
	var cleared bytes.Buffer
	clearedCmd := newRootCommand(&cleared, new(bytes.Buffer))
	clearedCmd.SetArgs([]string{
		"replay-workbench", root, primary.RunID,
		"--current-collaboration",
		"--max-requests", "1",
	})
	if err := clearedCmd.Execute(); err == nil || !strings.Contains(err.Error(), "current replay workbench collaboration selection is not current") {
		t.Fatalf("expected cleared current-collaboration error, got %v output=%q", err, cleared.String())
	}

	missingRoot := filepath.Join(t.TempDir(), "missing-workspace")
	if err := app.InitWorkspace(missingRoot); err != nil {
		t.Fatalf("init missing selection workspace: %v", err)
	}
	missingRun, err := app.RunWorkspace(context.Background(), missingRoot)
	if err != nil {
		t.Fatalf("run missing selection workspace: %v", err)
	}
	var missing bytes.Buffer
	missingCmd := newRootCommand(&missing, new(bytes.Buffer))
	missingCmd.SetArgs([]string{
		"replay-workbench", missingRoot, missingRun.RunID,
		"--current-collaboration",
		"--max-requests", "1",
	})
	if err := missingCmd.Execute(); err == nil || !strings.Contains(err.Error(), "current replay workbench collaboration selection was not found") {
		t.Fatalf("expected missing current-collaboration error, got %v output=%q", err, missing.String())
	}
}

func TestReplayWorkbenchCommandLoadsCurrentCollaborationNoteSelectionDefaultsBasic(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := app.InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	primary, err := app.RunWorkspace(context.Background(), root)
	if err != nil {
		t.Fatalf("run primary workspace: %v", err)
	}
	comparison, err := app.RunWorkspace(context.Background(), root)
	if err != nil {
		t.Fatalf("run comparison workspace: %v", err)
	}
	session, err := app.CreateReplayWorkbenchSession(context.Background(), root, app.ReplayWorkbenchSessionOptions{
		PrimaryRunID:  primary.RunID,
		CompareRunIDs: []string{comparison.RunID},
		SelectedRunID: comparison.RunID,
		Filters: map[string]string{
			"lane": "toolchain",
			"q":    "note",
			"edge": "edge-1",
		},
		CreatedBy: "operator",
		Reason:    "current collaboration note restore",
	})
	if err != nil {
		t.Fatalf("create replay workbench session: %v", err)
	}
	collaboration, err := app.CreateReplayWorkbenchCollaboration(context.Background(), root, app.ReplayWorkbenchCollaborationOptions{
		SessionPath:  filepath.Base(session.Path),
		Participants: []string{"alice", "bob"},
		CreatedBy:    "lead",
		CreatedAt:    time.Date(2026, 6, 1, 11, 35, 0, 0, time.UTC),
		Reason:       "restore current collaboration note",
	})
	if err != nil {
		t.Fatalf("create replay workbench collaboration: %v", err)
	}
	note, err := app.CreateReplayWorkbenchCollaborationNote(context.Background(), root, app.ReplayWorkbenchCollaborationNoteOptions{
		CollaborationPath: filepath.Base(collaboration.Path),
		Author:            "alice",
		Body:              "review this replay focus",
		TargetKind:        "edge",
		TargetID:          "edge-1",
		CreatedAt:         time.Date(2026, 6, 1, 11, 36, 0, 0, time.UTC),
		Reason:            "operator note focus",
	})
	if err != nil {
		t.Fatalf("create replay workbench collaboration note: %v", err)
	}
	selected, err := app.SelectReplayWorkbenchCollaborationNote(root, filepath.Base(note.Path), app.ReplayWorkbenchCollaborationNoteSelectionOptions{
		SelectedBy: "lead",
		SelectedAt: time.Date(2026, 6, 1, 11, 37, 0, 0, time.UTC),
		Reason:     "restore note focus",
	})
	if err != nil {
		t.Fatalf("select replay workbench collaboration note: %v", err)
	}
	before, err := app.ReplayWorkspace(context.Background(), root, primary.RunID)
	if err != nil {
		t.Fatalf("replay workspace before current note restore: %v", err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("allocate listen address: %v", err)
	}
	addr := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatalf("close temporary listener: %v", err)
	}

	var out bytes.Buffer
	cmd := newRootCommand(&out, new(bytes.Buffer))
	cmd.SetArgs([]string{
		"replay-workbench", root, primary.RunID,
		"--current-collaboration-note",
		"--listen", addr,
		"--max-requests", "2",
	})
	done := make(chan error, 1)
	go func() {
		done <- cmd.Execute()
	}()

	client := http.Client{Timeout: 100 * time.Millisecond}
	var loadedSession struct {
		SelectedRunID string            `json:"selected_run_id"`
		RunIDs        []string          `json:"run_ids"`
		Filters       map[string]string `json:"filters"`
	}
	var lastErr error
	for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		response, err := client.Get("http://" + addr + "/api/session")
		if err != nil {
			lastErr = err
			continue
		}
		if response.StatusCode != http.StatusOK {
			_ = response.Body.Close()
			lastErr = fmt.Errorf("unexpected session API status %d", response.StatusCode)
			continue
		}
		if err := json.NewDecoder(response.Body).Decode(&loadedSession); err != nil {
			_ = response.Body.Close()
			lastErr = err
			continue
		}
		_ = response.Body.Close()
		lastErr = nil
		break
	}
	if lastErr != nil {
		t.Fatalf("get current collaboration note replay workbench session API: %v", lastErr)
	}
	if loadedSession.SelectedRunID != comparison.RunID ||
		len(loadedSession.RunIDs) != 2 ||
		loadedSession.Filters["lane"] != "toolchain" ||
		loadedSession.Filters["q"] != "note" ||
		loadedSession.Filters["edge"] != "edge-1" {
		t.Fatalf("unexpected current collaboration note session state: %#v", loadedSession)
	}

	selectionResponse, err := client.Get("http://" + addr + "/api/collaboration-note-selection")
	if err != nil {
		t.Fatalf("get current collaboration note selection API: %v", err)
	}
	var selection app.ReplayWorkbenchCollaborationNoteSelection
	if err := json.NewDecoder(selectionResponse.Body).Decode(&selection); err != nil {
		_ = selectionResponse.Body.Close()
		t.Fatalf("decode current collaboration note selection API: %v", err)
	}
	_ = selectionResponse.Body.Close()
	if selection.NotePath != note.Path || selection.CollaborationPath != collaboration.Path || selection.SessionPath != session.Path || selection.Status != "current" {
		t.Fatalf("unexpected current collaboration note selection: %#v", selection)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("serve replay workbench with current collaboration note: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("replay workbench with current collaboration note did not exit after max requests")
	}
	if !strings.Contains(out.String(), "replay-workbench: session="+session.Path) || !strings.Contains(out.String(), "replay-workbench: accepted=2") {
		t.Fatalf("expected replay workbench current-collaboration-note output, got %q", out.String())
	}
	afterSelection, found, err := app.ReadReplayWorkbenchCollaborationNoteSelection(root)
	if err != nil {
		t.Fatalf("read note selection after current collaboration note restore: %v", err)
	}
	if !found || afterSelection.NotePath != selected.NotePath || afterSelection.SelectedAt != selected.SelectedAt {
		t.Fatalf("current collaboration note restore mutated selection marker: before=%#v after=%#v found=%v", selected, afterSelection, found)
	}
	after, err := app.ReplayWorkspace(context.Background(), root, primary.RunID)
	if err != nil {
		t.Fatalf("replay workspace after current note restore: %v", err)
	}
	if after.EventCount != before.EventCount {
		t.Fatalf("current collaboration note restore must not mutate replay events, before=%d after=%d", before.EventCount, after.EventCount)
	}

	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{
			name: "session conflict",
			args: []string{
				"replay-workbench", root, primary.RunID,
				"--session", filepath.Base(session.Path),
				"--current-collaboration-note",
				"--max-requests", "1",
			},
			want: "--session and --current-collaboration-note cannot be combined",
		},
		{
			name: "current session conflict",
			args: []string{
				"replay-workbench", root, primary.RunID,
				"--current-session",
				"--current-collaboration-note",
				"--max-requests", "1",
			},
			want: "--current-session and --current-collaboration-note cannot be combined",
		},
		{
			name: "current collaboration conflict",
			args: []string{
				"replay-workbench", root, primary.RunID,
				"--current-collaboration",
				"--current-collaboration-note",
				"--max-requests", "1",
			},
			want: "--current-collaboration and --current-collaboration-note cannot be combined",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var conflict bytes.Buffer
			conflictCmd := newRootCommand(&conflict, new(bytes.Buffer))
			conflictCmd.SetArgs(tc.args)
			if err := conflictCmd.Execute(); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected %q error, got %v output=%q", tc.want, err, conflict.String())
			}
		})
	}

	if _, err := app.ClearReplayWorkbenchCollaborationNoteSelection(root, app.ReplayWorkbenchCollaborationNoteSelectionClearOptions{
		ClearedBy: "lead",
		Reason:    "close current collaboration note",
	}); err != nil {
		t.Fatalf("clear replay workbench collaboration note selection: %v", err)
	}
	var cleared bytes.Buffer
	clearedCmd := newRootCommand(&cleared, new(bytes.Buffer))
	clearedCmd.SetArgs([]string{
		"replay-workbench", root, primary.RunID,
		"--current-collaboration-note",
		"--max-requests", "1",
	})
	if err := clearedCmd.Execute(); err == nil || !strings.Contains(err.Error(), "current replay workbench collaboration note selection is not current") {
		t.Fatalf("expected cleared current-collaboration-note error, got %v output=%q", err, cleared.String())
	}

	missingRoot := filepath.Join(t.TempDir(), "missing-workspace")
	if err := app.InitWorkspace(missingRoot); err != nil {
		t.Fatalf("init missing note selection workspace: %v", err)
	}
	missingRun, err := app.RunWorkspace(context.Background(), missingRoot)
	if err != nil {
		t.Fatalf("run missing note selection workspace: %v", err)
	}
	var missing bytes.Buffer
	missingCmd := newRootCommand(&missing, new(bytes.Buffer))
	missingCmd.SetArgs([]string{
		"replay-workbench", missingRoot, missingRun.RunID,
		"--current-collaboration-note",
		"--max-requests", "1",
	})
	if err := missingCmd.Execute(); err == nil || !strings.Contains(err.Error(), "current replay workbench collaboration note selection was not found") {
		t.Fatalf("expected missing current-collaboration-note error, got %v output=%q", err, missing.String())
	}
}

func TestReplayWorkbenchCommandLoadsCurrentCollaborationNoteSelectionDefaults(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := app.InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	primary, err := app.RunWorkspace(context.Background(), root)
	if err != nil {
		t.Fatalf("run primary workspace: %v", err)
	}
	comparison, err := app.RunWorkspace(context.Background(), root)
	if err != nil {
		t.Fatalf("run comparison workspace: %v", err)
	}
	session, err := app.CreateReplayWorkbenchSession(context.Background(), root, app.ReplayWorkbenchSessionOptions{
		PrimaryRunID:  primary.RunID,
		CompareRunIDs: []string{comparison.RunID},
		SelectedRunID: comparison.RunID,
		Filters: map[string]string{
			"lane": "toolchain",
			"q":    "note-restore",
			"edge": "edge-1",
		},
		CreatedBy: "operator",
		Reason:    "current collaboration note restore",
	})
	if err != nil {
		t.Fatalf("create replay workbench session: %v", err)
	}
	collaboration, err := app.CreateReplayWorkbenchCollaboration(context.Background(), root, app.ReplayWorkbenchCollaborationOptions{
		SessionPath:  filepath.Base(session.Path),
		Participants: []string{"alice", "bob"},
		CreatedBy:    "lead",
		CreatedAt:    time.Date(2026, 6, 1, 11, 40, 0, 0, time.UTC),
		Reason:       "restore current collaboration note",
	})
	if err != nil {
		t.Fatalf("create replay workbench collaboration: %v", err)
	}
	note, err := app.CreateReplayWorkbenchCollaborationNote(context.Background(), root, app.ReplayWorkbenchCollaborationNoteOptions{
		CollaborationPath: filepath.Base(collaboration.Path),
		Author:            "alice",
		Body:              "restore this collaboration note focus",
		TargetKind:        "edge",
		TargetID:          "edge-1",
		CreatedAt:         time.Date(2026, 6, 1, 11, 41, 0, 0, time.UTC),
		Reason:            "current note restore",
	})
	if err != nil {
		t.Fatalf("create replay workbench collaboration note: %v", err)
	}
	selected, err := app.SelectReplayWorkbenchCollaborationNote(root, filepath.Base(note.Path), app.ReplayWorkbenchCollaborationNoteSelectionOptions{
		SelectedBy: "lead",
		SelectedAt: time.Date(2026, 6, 1, 11, 42, 0, 0, time.UTC),
		Reason:     "operator current collaboration note",
	})
	if err != nil {
		t.Fatalf("select replay workbench collaboration note: %v", err)
	}
	before, err := app.ReplayWorkspace(context.Background(), root, primary.RunID)
	if err != nil {
		t.Fatalf("replay primary before current collaboration note restore: %v", err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("allocate listen address: %v", err)
	}
	addr := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatalf("close temporary listener: %v", err)
	}

	var out bytes.Buffer
	cmd := newRootCommand(&out, new(bytes.Buffer))
	cmd.SetArgs([]string{
		"replay-workbench", root, primary.RunID,
		"--current-collaboration-note",
		"--listen", addr,
		"--max-requests", "2",
	})
	done := make(chan error, 1)
	go func() {
		done <- cmd.Execute()
	}()

	client := http.Client{Timeout: 100 * time.Millisecond}
	var loadedSession struct {
		SelectedRunID string            `json:"selected_run_id"`
		RunIDs        []string          `json:"run_ids"`
		Filters       map[string]string `json:"filters"`
	}
	var lastErr error
	for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		response, err := client.Get("http://" + addr + "/api/session")
		if err != nil {
			lastErr = err
			continue
		}
		if response.StatusCode != http.StatusOK {
			_ = response.Body.Close()
			lastErr = fmt.Errorf("unexpected session API status %d", response.StatusCode)
			continue
		}
		if err := json.NewDecoder(response.Body).Decode(&loadedSession); err != nil {
			_ = response.Body.Close()
			lastErr = err
			continue
		}
		_ = response.Body.Close()
		lastErr = nil
		break
	}
	if lastErr != nil {
		t.Fatalf("get current collaboration note replay workbench session API: %v", lastErr)
	}
	if loadedSession.SelectedRunID != comparison.RunID ||
		len(loadedSession.RunIDs) != 2 ||
		loadedSession.Filters["lane"] != "toolchain" ||
		loadedSession.Filters["q"] != "note-restore" ||
		loadedSession.Filters["edge"] != "edge-1" {
		t.Fatalf("unexpected current collaboration note session state: %#v", loadedSession)
	}

	selectionResponse, err := client.Get("http://" + addr + "/api/collaboration-note-selection")
	if err != nil {
		t.Fatalf("get current collaboration note selection API: %v", err)
	}
	var selection app.ReplayWorkbenchCollaborationNoteSelection
	if err := json.NewDecoder(selectionResponse.Body).Decode(&selection); err != nil {
		_ = selectionResponse.Body.Close()
		t.Fatalf("decode current collaboration note selection API: %v", err)
	}
	_ = selectionResponse.Body.Close()
	if selection.NotePath != note.Path ||
		selection.CollaborationPath != collaboration.Path ||
		selection.SessionPath != session.Path ||
		selection.Status != "current" {
		t.Fatalf("unexpected current collaboration note selection: %#v", selection)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("serve replay workbench with current collaboration note: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("replay workbench with current collaboration note did not exit after max requests")
	}
	if !strings.Contains(out.String(), "replay-workbench: session="+session.Path) || !strings.Contains(out.String(), "replay-workbench: accepted=2") {
		t.Fatalf("expected replay workbench current-collaboration-note output, got %q", out.String())
	}
	afterSelection, found, err := app.ReadReplayWorkbenchCollaborationNoteSelection(root)
	if err != nil {
		t.Fatalf("read collaboration note selection after restore: %v", err)
	}
	if !found || afterSelection.NotePath != selected.NotePath || afterSelection.SelectedAt != selected.SelectedAt || afterSelection.Status != selected.Status {
		t.Fatalf("current collaboration note restore mutated selection marker, before=%#v after=%#v found=%v", selected, afterSelection, found)
	}
	after, err := app.ReplayWorkspace(context.Background(), root, primary.RunID)
	if err != nil {
		t.Fatalf("replay primary after current collaboration note restore: %v", err)
	}
	if after.EventCount != before.EventCount {
		t.Fatalf("current collaboration note restore must not mutate replay events, before=%d after=%d", before.EventCount, after.EventCount)
	}

	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{
			name: "session conflict",
			args: []string{
				"replay-workbench", root, primary.RunID,
				"--session", filepath.Base(session.Path),
				"--current-collaboration-note",
				"--max-requests", "1",
			},
			want: "--session and --current-collaboration-note cannot be combined",
		},
		{
			name: "current session conflict",
			args: []string{
				"replay-workbench", root, primary.RunID,
				"--current-session",
				"--current-collaboration-note",
				"--max-requests", "1",
			},
			want: "--current-session and --current-collaboration-note cannot be combined",
		},
		{
			name: "current collaboration conflict",
			args: []string{
				"replay-workbench", root, primary.RunID,
				"--current-collaboration",
				"--current-collaboration-note",
				"--max-requests", "1",
			},
			want: "--current-collaboration and --current-collaboration-note cannot be combined",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var conflict bytes.Buffer
			conflictCmd := newRootCommand(&conflict, new(bytes.Buffer))
			conflictCmd.SetArgs(tc.args)
			if err := conflictCmd.Execute(); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected %q error, got %v output=%q", tc.want, err, conflict.String())
			}
		})
	}

	if _, err := app.ClearReplayWorkbenchCollaborationNoteSelection(root, app.ReplayWorkbenchCollaborationNoteSelectionClearOptions{
		ClearedBy: "lead",
		Reason:    "close current collaboration note",
	}); err != nil {
		t.Fatalf("clear replay workbench collaboration note selection: %v", err)
	}
	var cleared bytes.Buffer
	clearedCmd := newRootCommand(&cleared, new(bytes.Buffer))
	clearedCmd.SetArgs([]string{
		"replay-workbench", root, primary.RunID,
		"--current-collaboration-note",
		"--max-requests", "1",
	})
	if err := clearedCmd.Execute(); err == nil || !strings.Contains(err.Error(), "current replay workbench collaboration note selection is not current") {
		t.Fatalf("expected cleared current-collaboration-note error, got %v output=%q", err, cleared.String())
	}

	missingRoot := filepath.Join(t.TempDir(), "missing-workspace")
	if err := app.InitWorkspace(missingRoot); err != nil {
		t.Fatalf("init missing note selection workspace: %v", err)
	}
	missingRun, err := app.RunWorkspace(context.Background(), missingRoot)
	if err != nil {
		t.Fatalf("run missing note selection workspace: %v", err)
	}
	var missing bytes.Buffer
	missingCmd := newRootCommand(&missing, new(bytes.Buffer))
	missingCmd.SetArgs([]string{
		"replay-workbench", missingRoot, missingRun.RunID,
		"--current-collaboration-note",
		"--max-requests", "1",
	})
	if err := missingCmd.Execute(); err == nil || !strings.Contains(err.Error(), "current replay workbench collaboration note selection was not found") {
		t.Fatalf("expected missing current-collaboration-note error, got %v output=%q", err, missing.String())
	}
}

func TestReplayWorkbenchHandlerFiltersReplayAPIsAndHTML(t *testing.T) {
	now := time.Unix(0, 0).UTC()
	primary := app.ReplayResult{
		RunID:      "run-filter-primary",
		EventCount: 2,
		LaneCounts: map[eventlog.ReplayLane]int{
			eventlog.ReplayLaneMission:   1,
			eventlog.ReplayLaneToolchain: 1,
		},
		Timeline: []eventlog.ReplayTimelineEvent{
			{
				EventID:   1,
				RunID:     "run-filter-primary",
				Lane:      eventlog.ReplayLaneMission,
				Type:      eventlog.EventAttempt,
				CreatedAt: now,
				Payload:   json.RawMessage(`{"stage":"workspace-bootstrap","message":"ordinary event"}`),
			},
			{
				EventID:   2,
				RunID:     "run-filter-primary",
				Lane:      eventlog.ReplayLaneToolchain,
				Type:      eventlog.EventTool,
				CreatedAt: now.Add(time.Second),
				Payload:   json.RawMessage(`{"message":"needle parser failure"}`),
			},
		},
	}
	comparison := app.ReplayResult{
		RunID:      "run-filter-comparison",
		EventCount: 1,
		LaneCounts: map[eventlog.ReplayLane]int{
			eventlog.ReplayLaneMission: 1,
		},
		Timeline: []eventlog.ReplayTimelineEvent{
			{
				EventID:   3,
				RunID:     "run-filter-comparison",
				Lane:      eventlog.ReplayLaneMission,
				Type:      eventlog.EventAttempt,
				CreatedAt: now.Add(2 * time.Second),
				Payload:   json.RawMessage(`{"message":"needle in a different lane"}`),
			},
		},
	}
	handler := newReplayWorkbenchHandler(primary, comparison)

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/replays?lane=toolchain&q=needle", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected filtered replay list status 200, got %d body=%q", recorder.Code, recorder.Body.String())
	}
	var replays []replayExport
	if err := json.NewDecoder(recorder.Body).Decode(&replays); err != nil {
		t.Fatalf("decode filtered replay list: %v", err)
	}
	if len(replays) != 2 {
		t.Fatalf("expected both replay exports after filtering, got %#v", replays)
	}
	if replays[0].RunID != primary.RunID || replays[0].EventCount != 1 || len(replays[0].Timeline) != 1 || replays[0].Timeline[0].EventID != 2 || replays[0].LaneCounts["toolchain"] != 1 {
		t.Fatalf("expected primary replay to include only matching toolchain event, got %#v", replays[0])
	}
	if replays[1].RunID != comparison.RunID || replays[1].EventCount != 0 || len(replays[1].Timeline) != 0 {
		t.Fatalf("expected comparison replay to be filtered to zero events, got %#v", replays[1])
	}

	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/replay?lane=toolchain&q=needle", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected filtered primary replay status 200, got %d body=%q", recorder.Code, recorder.Body.String())
	}
	var primaryPayload replayExport
	if err := json.NewDecoder(recorder.Body).Decode(&primaryPayload); err != nil {
		t.Fatalf("decode filtered primary replay: %v", err)
	}
	if primaryPayload.RunID != primary.RunID || primaryPayload.EventCount != 1 || len(primaryPayload.Timeline) != 1 || primaryPayload.Timeline[0].EventID != 2 {
		t.Fatalf("expected /api/replay filters to keep primary matching event, got %#v", primaryPayload)
	}

	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/timeline.ndjson?lane=toolchain&q=needle", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected filtered timeline status 200, got %d body=%q", recorder.Code, recorder.Body.String())
	}
	lines := strings.Split(strings.TrimSpace(recorder.Body.String()), "\n")
	if len(lines) != 1 || !strings.Contains(lines[0], `"event_id":2`) || strings.Contains(lines[0], `"event_id":1`) {
		t.Fatalf("expected filtered NDJSON to include only matching event 2, got %q", recorder.Body.String())
	}

	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected filtered workbench html status 200, got %d body=%q", recorder.Code, recorder.Body.String())
	}
	for _, want := range []string{
		"id=\"ouro-replay-search\"",
		"id=\"ouro-replay-lane-filter\"",
		"filterReplayTimeline",
	} {
		if !strings.Contains(recorder.Body.String(), want) {
			t.Fatalf("expected workbench html to contain %q, got %q", want, recorder.Body.String())
		}
	}
}

func TestReplayWorkbenchHandlerServesShareableSessionState(t *testing.T) {
	primary := app.ReplayResult{RunID: "run-share-primary"}
	comparison := app.ReplayResult{RunID: "run-share-comparison"}
	handler := newReplayWorkbenchHandler(primary, comparison)

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/session?run=run-share-comparison&lane=toolchain&q=needle", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected session API status 200, got %d body=%q", recorder.Code, recorder.Body.String())
	}
	var session struct {
		PrimaryRunID  string            `json:"primary_run_id"`
		SelectedRunID string            `json:"selected_run_id"`
		RunIDs        []string          `json:"run_ids"`
		Filters       map[string]string `json:"filters"`
		Endpoints     map[string]string `json:"endpoints"`
	}
	if err := json.NewDecoder(recorder.Body).Decode(&session); err != nil {
		t.Fatalf("decode session API: %v", err)
	}
	if session.PrimaryRunID != primary.RunID || session.SelectedRunID != comparison.RunID {
		t.Fatalf("unexpected session run selection: %#v", session)
	}
	if len(session.RunIDs) != 2 || session.RunIDs[0] != primary.RunID || session.RunIDs[1] != comparison.RunID {
		t.Fatalf("unexpected session run ids: %#v", session.RunIDs)
	}
	if session.Filters["lane"] != "toolchain" || session.Filters["q"] != "needle" {
		t.Fatalf("unexpected session filters: %#v", session.Filters)
	}
	if session.Endpoints["replays"] != "/api/replays?lane=toolchain&q=needle" || session.Endpoints["timeline_ndjson"] != "/api/timeline.ndjson?lane=toolchain&q=needle" {
		t.Fatalf("unexpected session endpoints: %#v", session.Endpoints)
	}

	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/?run=run-share-comparison&lane=toolchain&q=needle", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected shareable workbench html status 200, got %d body=%q", recorder.Code, recorder.Body.String())
	}
	for _, want := range []string{
		"window.OURO_REPLAY_SESSION",
		"updateReplayWorkbenchURL",
		`"selected_run_id":"run-share-comparison"`,
		`"lane":"toolchain"`,
		`"q":"needle"`,
	} {
		if !strings.Contains(recorder.Body.String(), want) {
			t.Fatalf("expected shareable workbench html to contain %q, got %q", want, recorder.Body.String())
		}
	}
}

func TestReplayWorkbenchHandlerServesSavedSessionsReadOnly(t *testing.T) {
	primary := app.ReplayResult{RunID: "run-session-primary"}
	comparison := app.ReplayResult{RunID: "run-session-comparison"}
	sessions := []app.ReplayWorkbenchSessionArtifact{
		{
			Path:          "/workspace/replay/workbench-sessions/session-one.json",
			SchemaVersion: 1,
			Status:        "saved",
			Mode:          "metadata-only",
			PrimaryRunID:  primary.RunID,
			SelectedRunID: comparison.RunID,
			RunIDs:        []string{primary.RunID, comparison.RunID},
			Filters: map[string]string{
				"lane": "toolchain",
				"q":    "needle",
				"edge": "edge-1",
			},
			Endpoints: map[string]string{
				"html": "/?edge=edge-1&lane=toolchain&q=needle&run=" + comparison.RunID,
			},
			CreatedBy: "operator",
			CreatedAt: "2026-06-01T02:10:00Z",
			Reason:    "saved comparison focus",
		},
	}
	selection := app.ReplayWorkbenchSessionSelection{
		Path:          "/workspace/replay/workbench-session-selection.json",
		Status:        "current",
		Mode:          "metadata-only",
		SessionPath:   sessions[0].Path,
		PrimaryRunID:  primary.RunID,
		SelectedRunID: comparison.RunID,
		RunIDs:        2,
		Filters:       3,
		Endpoints:     8,
		SelectedBy:    "alice",
		SelectedAt:    "2026-06-01T03:10:00Z",
		Reason:        "current hosted focus",
	}
	collaborations := []app.ReplayWorkbenchCollaborationArtifact{
		{
			Path:          "/workspace/replay/workbench-collaborations/collab-one.json",
			SchemaVersion: 1,
			Status:        "saved",
			Mode:          "metadata-only",
			SessionPath:   sessions[0].Path,
			PrimaryRunID:  primary.RunID,
			SelectedRunID: comparison.RunID,
			RunIDs:        2,
			Filters:       3,
			Participants:  []string{"alice", "bob"},
			Endpoints: map[string]string{
				"session":        "/api/session?edge=edge-1&lane=toolchain&q=needle",
				"collaborations": "/api/collaborations",
			},
			CreatedBy: "lead",
			CreatedAt: "2026-06-01T03:30:00Z",
			Reason:    "hosted pair review",
		},
	}
	collaborationSelection := app.ReplayWorkbenchCollaborationSelection{
		Path:              "/workspace/replay/workbench-collaboration-selection.json",
		Status:            "current",
		Mode:              "metadata-only",
		CollaborationPath: collaborations[0].Path,
		SessionPath:       sessions[0].Path,
		PrimaryRunID:      primary.RunID,
		SelectedRunID:     comparison.RunID,
		RunIDs:            2,
		Filters:           3,
		Endpoints:         9,
		Participants:      2,
		SelectedBy:        "lead",
		SelectedAt:        "2026-06-01T04:10:00Z",
		Reason:            "current hosted collaboration",
	}
	collaborationPresence := []app.ReplayWorkbenchCollaborationPresence{
		{
			Path:              "/workspace/replay/workbench-collaboration-presence/collab-one-alice.json",
			SchemaVersion:     1,
			Status:            "online",
			Mode:              "metadata-only",
			CollaborationPath: collaborations[0].Path,
			SessionPath:       sessions[0].Path,
			PrimaryRunID:      primary.RunID,
			SelectedRunID:     comparison.RunID,
			Participant:       "alice",
			SeenBy:            "lead",
			SeenAt:            "2026-06-01T05:40:00Z",
			Reason:            "watching current review",
		},
	}
	collaborationNotes := []app.ReplayWorkbenchCollaborationNote{
		{
			Path:              "/workspace/replay/workbench-collaboration-notes/note-one.json",
			SchemaVersion:     1,
			Status:            "saved",
			Mode:              "metadata-only",
			CollaborationPath: collaborations[0].Path,
			SessionPath:       sessions[0].Path,
			PrimaryRunID:      primary.RunID,
			SelectedRunID:     comparison.RunID,
			Author:            "alice",
			Body:              "Confirm graph node before accepting.",
			TargetKind:        "graph-node",
			TargetID:          "fact-target",
			CreatedAt:         "2026-06-01T08:20:00Z",
			Reason:            "collaborative evidence review",
		},
	}
	collaborationNoteSelection := app.ReplayWorkbenchCollaborationNoteSelection{
		Path:              "/workspace/replay/workbench-collaboration-note-selection.json",
		Status:            "current",
		Mode:              "metadata-only",
		NotePath:          collaborationNotes[0].Path,
		CollaborationPath: collaborations[0].Path,
		SessionPath:       sessions[0].Path,
		PrimaryRunID:      primary.RunID,
		SelectedRunID:     comparison.RunID,
		Author:            "alice",
		TargetKind:        "graph-node",
		TargetID:          "fact-target",
		SelectedBy:        "lead",
		SelectedAt:        "2026-06-01T09:50:00Z",
		Reason:            "current hosted note",
	}
	handler := newReplayWorkbenchHandlerWithOptions(replayWorkbenchHandlerOptions{
		SavedSessions:              sessions,
		SessionSelection:           &selection,
		Collaborations:             collaborations,
		CollaborationSelection:     &collaborationSelection,
		CollaborationPresence:      collaborationPresence,
		CollaborationNotes:         collaborationNotes,
		CollaborationNoteSelection: &collaborationNoteSelection,
	}, primary, comparison)

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/sessions", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected sessions API status 200, got %d body=%q", recorder.Code, recorder.Body.String())
	}
	var decoded []app.ReplayWorkbenchSessionArtifact
	if err := json.NewDecoder(recorder.Body).Decode(&decoded); err != nil {
		t.Fatalf("decode sessions API: %v", err)
	}
	if len(decoded) != 1 || decoded[0].Path != sessions[0].Path || decoded[0].Filters["edge"] != "edge-1" {
		t.Fatalf("unexpected sessions payload: %#v", decoded)
	}

	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/session-selection", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected session selection API status 200, got %d body=%q", recorder.Code, recorder.Body.String())
	}
	var decodedSelection app.ReplayWorkbenchSessionSelection
	if err := json.NewDecoder(recorder.Body).Decode(&decodedSelection); err != nil {
		t.Fatalf("decode session selection API: %v", err)
	}
	if decodedSelection.SessionPath != sessions[0].Path || decodedSelection.Status != "current" || decodedSelection.SelectedBy != "alice" {
		t.Fatalf("unexpected session selection payload: %#v", decodedSelection)
	}

	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/collaborations", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected collaborations API status 200, got %d body=%q", recorder.Code, recorder.Body.String())
	}
	var decodedCollaborations []app.ReplayWorkbenchCollaborationArtifact
	if err := json.NewDecoder(recorder.Body).Decode(&decodedCollaborations); err != nil {
		t.Fatalf("decode collaborations API: %v", err)
	}
	if len(decodedCollaborations) != 1 ||
		decodedCollaborations[0].Path != collaborations[0].Path ||
		len(decodedCollaborations[0].Participants) != 2 ||
		decodedCollaborations[0].Reason != "hosted pair review" {
		t.Fatalf("unexpected collaborations payload: %#v", decodedCollaborations)
	}

	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/collaboration-selection", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected collaboration selection API status 200, got %d body=%q", recorder.Code, recorder.Body.String())
	}
	var decodedCollaborationSelection app.ReplayWorkbenchCollaborationSelection
	if err := json.NewDecoder(recorder.Body).Decode(&decodedCollaborationSelection); err != nil {
		t.Fatalf("decode collaboration selection API: %v", err)
	}
	if decodedCollaborationSelection.CollaborationPath != collaborations[0].Path ||
		decodedCollaborationSelection.Status != "current" ||
		decodedCollaborationSelection.SelectedBy != "lead" {
		t.Fatalf("unexpected collaboration selection payload: %#v", decodedCollaborationSelection)
	}

	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/collaboration-presence", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected collaboration presence API status 200, got %d body=%q", recorder.Code, recorder.Body.String())
	}
	var decodedPresence []app.ReplayWorkbenchCollaborationPresence
	if err := json.NewDecoder(recorder.Body).Decode(&decodedPresence); err != nil {
		t.Fatalf("decode collaboration presence API: %v", err)
	}
	if len(decodedPresence) != 1 ||
		decodedPresence[0].CollaborationPath != collaborations[0].Path ||
		decodedPresence[0].Participant != "alice" ||
		decodedPresence[0].Status != "online" {
		t.Fatalf("unexpected collaboration presence payload: %#v", decodedPresence)
	}

	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/collaboration-notes", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected collaboration notes API status 200, got %d body=%q", recorder.Code, recorder.Body.String())
	}
	var decodedNotes []app.ReplayWorkbenchCollaborationNote
	if err := json.NewDecoder(recorder.Body).Decode(&decodedNotes); err != nil {
		t.Fatalf("decode collaboration notes API: %v", err)
	}
	if len(decodedNotes) != 1 ||
		decodedNotes[0].CollaborationPath != collaborations[0].Path ||
		decodedNotes[0].Author != "alice" ||
		decodedNotes[0].TargetID != "fact-target" {
		t.Fatalf("unexpected collaboration notes payload: %#v", decodedNotes)
	}

	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/collaboration-note-selection", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected collaboration note selection API status 200, got %d body=%q", recorder.Code, recorder.Body.String())
	}
	var decodedNoteSelection app.ReplayWorkbenchCollaborationNoteSelection
	if err := json.NewDecoder(recorder.Body).Decode(&decodedNoteSelection); err != nil {
		t.Fatalf("decode collaboration note selection API: %v", err)
	}
	if decodedNoteSelection.NotePath != collaborationNotes[0].Path ||
		decodedNoteSelection.Status != "current" ||
		decodedNoteSelection.SelectedBy != "lead" {
		t.Fatalf("unexpected collaboration note selection payload: %#v", decodedNoteSelection)
	}

	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected saved-session HTML status 200, got %d body=%q", recorder.Code, recorder.Body.String())
	}
	for _, want := range []string{
		"window.OURO_REPLAY_SESSIONS",
		"window.OURO_REPLAY_SESSION_SELECTION",
		"window.OURO_REPLAY_COLLABORATIONS",
		"window.OURO_REPLAY_COLLABORATION_SELECTION",
		"window.OURO_REPLAY_COLLABORATION_PRESENCE",
		"window.OURO_REPLAY_COLLABORATION_NOTES",
		"window.OURO_REPLAY_COLLABORATION_NOTE_SELECTION",
		"window.OURO_REPLAY_COLLABORATION_NOTE_WRITES_ENABLED = false",
		"window.OURO_REPLAY_COLLABORATION_PRESENCE_WRITES_ENABLED = false",
		"id=\"ouro-replay-session-select\"",
		"id=\"ouro-replay-session-selection-summary\"",
		"id=\"ouro-replay-collaboration-summary\"",
		"id=\"ouro-replay-collaboration-selection-summary\"",
		"id=\"ouro-replay-collaboration-presence-summary\"",
		"id=\"ouro-replay-collaboration-notes-summary\"",
		"id=\"ouro-replay-collaboration-note-selection-summary\"",
		"applyReplayWorkbenchSession",
		"renderReplayWorkbenchSessionSelection",
		"renderReplayWorkbenchCollaborations",
		"renderReplayWorkbenchCollaborationSelection",
		"renderReplayWorkbenchCollaborationPresence",
		"renderReplayWorkbenchCollaborationNotes",
		"renderReplayWorkbenchCollaborationNoteSelection",
		"current hosted focus",
		"current hosted collaboration",
		"current hosted note",
		"watching current review",
		"Confirm graph node before accepting.",
		"saved comparison focus",
		"hosted pair review",
		"edge-1",
	} {
		if !strings.Contains(recorder.Body.String(), want) {
			t.Fatalf("expected saved-session HTML to contain %q, got %q", want, recorder.Body.String())
		}
	}
	for _, forbidden := range []string{
		"id=\"ouro-replay-collaboration-presence-form\"",
		"id=\"ouro-replay-collaboration-note-form\"",
		"method: \"POST\"",
	} {
		if strings.Contains(recorder.Body.String(), forbidden) {
			t.Fatalf("expected default saved-session HTML to omit %q, got %q", forbidden, recorder.Body.String())
		}
	}

	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/sessions", nil))
	if recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected saved sessions endpoint to remain read-only, got %d body=%q", recorder.Code, recorder.Body.String())
	}
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/session-selection", nil))
	if recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected session selection endpoint to remain read-only, got %d body=%q", recorder.Code, recorder.Body.String())
	}
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/collaborations", nil))
	if recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected collaborations endpoint to remain read-only, got %d body=%q", recorder.Code, recorder.Body.String())
	}
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/collaboration-selection", nil))
	if recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected collaboration selection endpoint to remain read-only, got %d body=%q", recorder.Code, recorder.Body.String())
	}
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/collaboration-presence", nil))
	if recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected collaboration presence endpoint to remain read-only, got %d body=%q", recorder.Code, recorder.Body.String())
	}
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/collaboration-notes", nil))
	if recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected collaboration notes endpoint to remain read-only, got %d body=%q", recorder.Code, recorder.Body.String())
	}
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/collaboration-note-selection", nil))
	if recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected collaboration note selection endpoint to remain read-only, got %d body=%q", recorder.Code, recorder.Body.String())
	}
}

func TestReplayWorkbenchHandlerServesReportSelectionReadOnly(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	execution := createReportProjectionDispatchExecutionForCLI(t, root)
	selection, err := app.SelectReportProjectionDispatchExecution(root, filepath.Base(execution.Path), app.ReportProjectionDispatchExecutionSelectionOptions{
		SelectedBy: "report-lead",
		SelectedAt: time.Date(2026, 6, 1, 11, 0, 0, 0, time.UTC),
		Reason:     "current hosted report",
	})
	if err != nil {
		t.Fatalf("select report projection dispatch execution: %v", err)
	}
	reportExecutions, err := app.ListReportProjectionDispatchExecutions(root)
	if err != nil {
		t.Fatalf("list report projection dispatch executions: %v", err)
	}
	handler := newReplayWorkbenchHandlerWithOptions(replayWorkbenchHandlerOptions{
		WorkspaceRoot:              root,
		ReportProjectionExecutions: reportExecutions,
		ReportProjectionSelection:  &selection,
	}, app.ReplayResult{
		RunID:      selection.RunID,
		ReportPath: selection.ReportPath,
	})

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/reports", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected reports API status 200, got %d body=%q", recorder.Code, recorder.Body.String())
	}
	var decodedReports []app.ReportProjectionDispatchExecution
	if err := json.NewDecoder(recorder.Body).Decode(&decodedReports); err != nil {
		t.Fatalf("decode reports API: %v", err)
	}
	if len(decodedReports) != 1 ||
		decodedReports[0].Path != execution.Path ||
		decodedReports[0].Annotation == nil ||
		decodedReports[0].Annotation.Decision != "accepted" {
		t.Fatalf("unexpected reports payload: %#v", decodedReports)
	}

	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/report-selection", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected report selection API status 200, got %d body=%q", recorder.Code, recorder.Body.String())
	}
	var decoded app.ReportProjectionDispatchExecutionSelection
	if err := json.NewDecoder(recorder.Body).Decode(&decoded); err != nil {
		t.Fatalf("decode report selection API: %v", err)
	}
	if decoded.Status != "current" ||
		decoded.ExecutionPath != execution.Path ||
		decoded.SelectedBy != "report-lead" ||
		decoded.Reason != "current hosted report" {
		t.Fatalf("unexpected report selection payload: %#v", decoded)
	}

	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected workbench HTML status 200, got %d body=%q", recorder.Code, recorder.Body.String())
	}
	for _, want := range []string{
		"window.OURO_REPLAY_REPORTS",
		"window.OURO_REPLAY_REPORT_SELECTION",
		"id=\"ouro-replay-reports-summary\"",
		"id=\"ouro-replay-report-selection-summary\"",
		"renderReplayWorkbenchReports",
		"renderReplayWorkbenchReportSelection",
		"current hosted report",
		"report-lead",
		filepath.Base(execution.ReportPath),
	} {
		if !strings.Contains(recorder.Body.String(), want) {
			t.Fatalf("expected workbench HTML to contain %q, got %q", want, recorder.Body.String())
		}
	}
	if strings.Contains(recorder.Body.String(), "fetch(\"/api/report-selection\", {") {
		t.Fatalf("expected report selection HTML to omit write controls, got %q", recorder.Body.String())
	}

	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/reports", nil))
	if recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected reports endpoint to remain read-only, got %d body=%q", recorder.Code, recorder.Body.String())
	}
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/report-selection", nil))
	if recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected report selection endpoint to remain read-only, got %d body=%q", recorder.Code, recorder.Body.String())
	}
}

func TestReplayWorkbenchHandlerWritesReportSelectionOnlyWithExplicitGate(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	execution := createReportProjectionDispatchExecutionForCLI(t, root)
	before, err := app.ReplayWorkspace(context.Background(), root, execution.RunID)
	if err != nil {
		t.Fatalf("replay before browser report selection write: %v", err)
	}
	body := fmt.Sprintf(`{"action":"select","execution_path":%q,"actor":"browser","at":"2026-06-01T11:05:00Z","reason":"browser selected report"}`, filepath.Base(execution.Path))

	disabled := newReplayWorkbenchHandlerWithOptions(replayWorkbenchHandlerOptions{
		WorkspaceRoot:              root,
		ReportProjectionExecutions: []app.ReportProjectionDispatchExecution{execution},
	}, before)
	recorder := httptest.NewRecorder()
	disabled.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/report-selection", strings.NewReader(body)))
	if recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected disabled report selection POST to be rejected, got %d body=%q", recorder.Code, recorder.Body.String())
	}
	if _, found, err := app.ReadReportProjectionDispatchExecutionSelection(root); err != nil {
		t.Fatalf("read report selection after rejected write: %v", err)
	} else if found {
		t.Fatalf("disabled browser report selection write must not create a selection marker")
	}

	enabled := newReplayWorkbenchHandlerWithOptions(replayWorkbenchHandlerOptions{
		WorkspaceRoot:                root,
		ReportProjectionExecutions:   []app.ReportProjectionDispatchExecution{execution},
		AllowReportSelectionWrites:   true,
		AllowSessionSelectionWrites:  false,
		AllowCollaborationNoteWrites: false,
	}, before)
	recorder = httptest.NewRecorder()
	enabled.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/report-selection", strings.NewReader(body)))
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected enabled report selection POST status 200, got %d body=%q", recorder.Code, recorder.Body.String())
	}
	var selected app.ReportProjectionDispatchExecutionSelection
	if err := json.NewDecoder(recorder.Body).Decode(&selected); err != nil {
		t.Fatalf("decode selected report marker: %v", err)
	}
	if selected.Status != "current" ||
		selected.ExecutionPath != execution.Path ||
		selected.SelectedBy != "browser" ||
		selected.Reason != "browser selected report" {
		t.Fatalf("unexpected selected report marker: %#v", selected)
	}

	recorder = httptest.NewRecorder()
	enabled.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/report-selection", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected report selection GET status 200, got %d body=%q", recorder.Code, recorder.Body.String())
	}
	var listed app.ReportProjectionDispatchExecutionSelection
	if err := json.NewDecoder(recorder.Body).Decode(&listed); err != nil {
		t.Fatalf("decode listed report selection: %v", err)
	}
	if listed.Status != "current" || listed.ExecutionPath != execution.Path {
		t.Fatalf("expected GET to return selected report marker, got %#v", listed)
	}

	clearBody := `{"action":"clear","actor":"browser","at":"2026-06-01T11:06:00Z","reason":"browser cleared report"}`
	recorder = httptest.NewRecorder()
	enabled.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/report-selection", strings.NewReader(clearBody)))
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected enabled report selection clear status 200, got %d body=%q", recorder.Code, recorder.Body.String())
	}
	var cleared app.ReportProjectionDispatchExecutionSelection
	if err := json.NewDecoder(recorder.Body).Decode(&cleared); err != nil {
		t.Fatalf("decode cleared report marker: %v", err)
	}
	if cleared.Status != "cleared" || cleared.ExecutionPath != execution.Path || cleared.ClearedBy != "browser" || cleared.Reason != "browser cleared report" {
		t.Fatalf("unexpected cleared report marker: %#v", cleared)
	}

	after, err := app.ReplayWorkspace(context.Background(), root, execution.RunID)
	if err != nil {
		t.Fatalf("replay after browser report selection write: %v", err)
	}
	if after.EventCount != before.EventCount {
		t.Fatalf("browser report selection write must not mutate replay events, before=%d after=%d", before.EventCount, after.EventCount)
	}
}

func TestReplayWorkbenchHandlerRendersReportSelectionWriteControlOnlyWithExplicitGate(t *testing.T) {
	execution := app.ReportProjectionDispatchExecution{
		Path:         "/workspace/reports/report-projection-dispatch-executions/execution-one.json",
		Status:       "completed",
		Mode:         "execute",
		RunID:        "run-report-primary",
		ReportPath:   "/workspace/reports/run-report-primary.md",
		Written:      true,
		DispatchPath: "/workspace/reports/report-projection-dispatches/dispatch-one.json",
		Annotation: &app.ReportProjectionDispatchExecutionAnnotation{
			Decision: "accepted",
		},
	}
	primary := app.ReplayResult{RunID: "run-report-primary"}
	disabled := newReplayWorkbenchHandlerWithOptions(replayWorkbenchHandlerOptions{
		ReportProjectionExecutions: []app.ReportProjectionDispatchExecution{execution},
	}, primary)
	recorder := httptest.NewRecorder()
	disabled.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected disabled HTML status 200, got %d body=%q", recorder.Code, recorder.Body.String())
	}
	disabledBody := recorder.Body.String()
	if strings.Contains(disabledBody, "ouro-replay-report-selection-form") ||
		strings.Contains(disabledBody, "fetch(\"/api/report-selection\", {") {
		t.Fatalf("expected disabled report-selection HTML to omit write controls, got %q", disabledBody)
	}

	enabled := newReplayWorkbenchHandlerWithOptions(replayWorkbenchHandlerOptions{
		ReportProjectionExecutions: []app.ReportProjectionDispatchExecution{execution},
		AllowReportSelectionWrites: true,
	}, primary)
	recorder = httptest.NewRecorder()
	enabled.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected enabled HTML status 200, got %d body=%q", recorder.Code, recorder.Body.String())
	}
	enabledBody := recorder.Body.String()
	for _, want := range []string{
		"window.OURO_REPLAY_REPORT_SELECTION_WRITES_ENABLED = true",
		"id=\"ouro-replay-report-selection-form\"",
		"id=\"ouro-replay-report-selection-execution\"",
		"id=\"ouro-replay-report-selection-actor\"",
		"id=\"ouro-replay-report-selection-reason\"",
		"id=\"ouro-replay-report-selection-select\"",
		"id=\"ouro-replay-report-selection-clear\"",
		"id=\"ouro-replay-report-selection-write-status\"",
		"installReplayWorkbenchReportSelectionWriter",
		"fetch(\"/api/report-selection\", {",
		"renderReplayWorkbenchReportSelection();",
	} {
		if !strings.Contains(enabledBody, want) {
			t.Fatalf("expected enabled report-selection HTML to contain %q, got %q", want, enabledBody)
		}
	}
}

func TestReplayWorkbenchHandlerRejectsPostOnAllReadOnlyRoutesByDefault(t *testing.T) {
	handler := newReplayWorkbenchHandler(app.ReplayResult{RunID: "run-read-only"})
	for _, path := range []string{
		"/",
		"/index.html",
		"/api/session",
		"/api/sessions",
		"/api/session-selection",
		"/api/replay",
		"/api/replays",
		"/api/graph",
		"/api/graphs",
		"/api/graph.ndjson",
		"/api/timeline.ndjson",
		"/api/collaborations",
		"/api/collaboration-selection",
		"/api/collaboration-presence",
		"/api/collaboration-notes",
		"/api/collaboration-note-selection",
		"/api/collaboration-events",
		"/api/reports",
		"/api/report-selection",
	} {
		t.Run(path, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{}`)))
			if recorder.Code != http.StatusMethodNotAllowed {
				t.Fatalf("expected POST %s to be rejected by default, got %d body=%q", path, recorder.Code, recorder.Body.String())
			}
		})
	}
}

func TestReplayWorkbenchHandlerEnabledWriteGateRejectsOtherPostRoutes(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := app.InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	run, err := app.RunWorkspace(context.Background(), root)
	if err != nil {
		t.Fatalf("run workspace: %v", err)
	}
	session, err := app.CreateReplayWorkbenchSession(context.Background(), root, app.ReplayWorkbenchSessionOptions{PrimaryRunID: run.RunID})
	if err != nil {
		t.Fatalf("create replay workbench session: %v", err)
	}
	collaboration, err := app.CreateReplayWorkbenchCollaboration(context.Background(), root, app.ReplayWorkbenchCollaborationOptions{
		SessionPath:  filepath.Base(session.Path),
		Participants: []string{"alice"},
	})
	if err != nil {
		t.Fatalf("create replay workbench collaboration: %v", err)
	}
	note, err := app.CreateReplayWorkbenchCollaborationNote(context.Background(), root, app.ReplayWorkbenchCollaborationNoteOptions{
		CollaborationPath: filepath.Base(collaboration.Path),
		Author:            "alice",
		Body:              "note",
	})
	if err != nil {
		t.Fatalf("create replay workbench collaboration note: %v", err)
	}
	replay, err := app.ReplayWorkspace(context.Background(), root, run.RunID)
	if err != nil {
		t.Fatalf("replay workspace: %v", err)
	}

	cases := []struct {
		name    string
		options replayWorkbenchHandlerOptions
		route   string
		body    string
	}{
		{
			name: "session selection gate does not enable collaboration notes",
			options: replayWorkbenchHandlerOptions{
				WorkspaceRoot:               root,
				SavedSessions:               []app.ReplayWorkbenchSessionArtifact{session},
				Collaborations:              []app.ReplayWorkbenchCollaborationArtifact{collaboration},
				AllowSessionSelectionWrites: true,
			},
			route: "/api/collaboration-notes",
			body:  fmt.Sprintf(`{"collaboration_path":%q,"author":"alice","body":"blocked"}`, filepath.Base(collaboration.Path)),
		},
		{
			name: "presence gate does not enable note selection",
			options: replayWorkbenchHandlerOptions{
				WorkspaceRoot:                    root,
				Collaborations:                   []app.ReplayWorkbenchCollaborationArtifact{collaboration},
				CollaborationNotes:               []app.ReplayWorkbenchCollaborationNote{note},
				AllowCollaborationPresenceWrites: true,
			},
			route: "/api/collaboration-note-selection",
			body:  fmt.Sprintf(`{"action":"select","note_path":%q}`, filepath.Base(note.Path)),
		},
		{
			name: "note gate does not enable session selection",
			options: replayWorkbenchHandlerOptions{
				WorkspaceRoot:                    root,
				SavedSessions:                    []app.ReplayWorkbenchSessionArtifact{session},
				Collaborations:                   []app.ReplayWorkbenchCollaborationArtifact{collaboration},
				AllowCollaborationNoteWrites:     true,
				AllowSessionSelectionWrites:      false,
				EnableCollaborationStream:        true,
				CollaborationNotesPollIntervalMS: 0,
			},
			route: "/api/session-selection",
			body:  fmt.Sprintf(`{"action":"select","session_path":%q}`, filepath.Base(session.Path)),
		},
		{
			name: "session selection gate does not enable report selection",
			options: replayWorkbenchHandlerOptions{
				WorkspaceRoot:               root,
				AllowSessionSelectionWrites: true,
			},
			route: "/api/report-selection",
			body:  `{"action":"clear"}`,
		},
		{
			name: "session selection gate does not enable report list writes",
			options: replayWorkbenchHandlerOptions{
				WorkspaceRoot:               root,
				AllowSessionSelectionWrites: true,
			},
			route: "/api/reports",
			body:  `{}`,
		},
		{
			name: "collaboration selection gate does not enable report selection",
			options: replayWorkbenchHandlerOptions{
				WorkspaceRoot:                     root,
				AllowCollaborationSelectionWrites: true,
			},
			route: "/api/report-selection",
			body:  `{"action":"clear"}`,
		},
		{
			name: "collaboration selection gate does not enable report list writes",
			options: replayWorkbenchHandlerOptions{
				WorkspaceRoot:                     root,
				AllowCollaborationSelectionWrites: true,
			},
			route: "/api/reports",
			body:  `{}`,
		},
		{
			name: "presence gate does not enable report selection",
			options: replayWorkbenchHandlerOptions{
				WorkspaceRoot:                    root,
				AllowCollaborationPresenceWrites: true,
			},
			route: "/api/report-selection",
			body:  `{"action":"clear"}`,
		},
		{
			name: "presence gate does not enable report list writes",
			options: replayWorkbenchHandlerOptions{
				WorkspaceRoot:                    root,
				AllowCollaborationPresenceWrites: true,
			},
			route: "/api/reports",
			body:  `{}`,
		},
		{
			name: "note gate does not enable report selection",
			options: replayWorkbenchHandlerOptions{
				WorkspaceRoot:                root,
				AllowCollaborationNoteWrites: true,
			},
			route: "/api/report-selection",
			body:  `{"action":"clear"}`,
		},
		{
			name: "note gate does not enable report list writes",
			options: replayWorkbenchHandlerOptions{
				WorkspaceRoot:                root,
				AllowCollaborationNoteWrites: true,
			},
			route: "/api/reports",
			body:  `{}`,
		},
		{
			name: "note selection gate does not enable report selection",
			options: replayWorkbenchHandlerOptions{
				WorkspaceRoot:                         root,
				AllowCollaborationNoteSelectionWrites: true,
			},
			route: "/api/report-selection",
			body:  `{"action":"clear"}`,
		},
		{
			name: "note selection gate does not enable report list writes",
			options: replayWorkbenchHandlerOptions{
				WorkspaceRoot:                         root,
				AllowCollaborationNoteSelectionWrites: true,
			},
			route: "/api/reports",
			body:  `{}`,
		},
		{
			name: "report selection gate does not enable session selection",
			options: replayWorkbenchHandlerOptions{
				WorkspaceRoot:                root,
				AllowReportSelectionWrites:   true,
				AllowSessionSelectionWrites:  false,
				AllowCollaborationNoteWrites: false,
			},
			route: "/api/session-selection",
			body:  fmt.Sprintf(`{"action":"select","session_path":%q}`, filepath.Base(session.Path)),
		},
		{
			name: "report selection gate does not enable report list writes",
			options: replayWorkbenchHandlerOptions{
				WorkspaceRoot:              root,
				AllowReportSelectionWrites: true,
			},
			route: "/api/reports",
			body:  `{}`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			handler := newReplayWorkbenchHandlerWithOptions(tc.options, replay)
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, tc.route, strings.NewReader(tc.body)))
			if recorder.Code != http.StatusMethodNotAllowed {
				t.Fatalf("expected unrelated POST route %s to remain disabled, got %d body=%q", tc.route, recorder.Code, recorder.Body.String())
			}
		})
	}
}

func TestReplayWorkbenchHandlerRejectsSymlinkedBrowserWriteTargets(t *testing.T) {
	t.Run("session selection marker", func(t *testing.T) {
		root, replay, session, _, _ := replayWorkbenchBrowserWriteSafetyFixture(t)
		if _, err := app.SelectReplayWorkbenchSession(root, filepath.Base(session.Path), app.ReplayWorkbenchSessionSelectionOptions{SelectedBy: "alice"}); err != nil {
			t.Fatalf("select session before symlink: %v", err)
		}
		outside := replayWorkbenchSymlinkCopyForHandlerTest(t, filepath.Join(root, "replay", "workbench-session-selection.json"), "outside-session-selection.json")
		before := replayWorkbenchReadFileForHandlerTest(t, outside)
		handler := newReplayWorkbenchHandlerWithOptions(replayWorkbenchHandlerOptions{
			WorkspaceRoot:               root,
			SavedSessions:               []app.ReplayWorkbenchSessionArtifact{session},
			AllowSessionSelectionWrites: true,
		}, replay)

		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/session-selection", strings.NewReader(`{"action":"clear","actor":"browser"}`)))
		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("expected symlinked session marker POST to fail 400, got %d body=%q", recorder.Code, recorder.Body.String())
		}
		if after := replayWorkbenchReadFileForHandlerTest(t, outside); string(after) != string(before) {
			t.Fatalf("symlinked session marker POST must not rewrite outside file")
		}
	})

	t.Run("collaboration selection marker", func(t *testing.T) {
		root, replay, _, collaboration, _ := replayWorkbenchBrowserWriteSafetyFixture(t)
		if _, err := app.SelectReplayWorkbenchCollaboration(root, filepath.Base(collaboration.Path), app.ReplayWorkbenchCollaborationSelectionOptions{SelectedBy: "alice"}); err != nil {
			t.Fatalf("select collaboration before symlink: %v", err)
		}
		outside := replayWorkbenchSymlinkCopyForHandlerTest(t, filepath.Join(root, "replay", "workbench-collaboration-selection.json"), "outside-collaboration-selection.json")
		before := replayWorkbenchReadFileForHandlerTest(t, outside)
		handler := newReplayWorkbenchHandlerWithOptions(replayWorkbenchHandlerOptions{
			WorkspaceRoot:                     root,
			Collaborations:                    []app.ReplayWorkbenchCollaborationArtifact{collaboration},
			AllowCollaborationSelectionWrites: true,
		}, replay)

		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/collaboration-selection", strings.NewReader(`{"action":"clear","actor":"browser"}`)))
		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("expected symlinked collaboration marker POST to fail 400, got %d body=%q", recorder.Code, recorder.Body.String())
		}
		if after := replayWorkbenchReadFileForHandlerTest(t, outside); string(after) != string(before) {
			t.Fatalf("symlinked collaboration marker POST must not rewrite outside file")
		}
	})

	t.Run("note selection marker", func(t *testing.T) {
		root, replay, _, _, note := replayWorkbenchBrowserWriteSafetyFixture(t)
		if _, err := app.SelectReplayWorkbenchCollaborationNote(root, filepath.Base(note.Path), app.ReplayWorkbenchCollaborationNoteSelectionOptions{SelectedBy: "alice"}); err != nil {
			t.Fatalf("select note before symlink: %v", err)
		}
		outside := replayWorkbenchSymlinkCopyForHandlerTest(t, filepath.Join(root, "replay", "workbench-collaboration-note-selection.json"), "outside-note-selection.json")
		before := replayWorkbenchReadFileForHandlerTest(t, outside)
		handler := newReplayWorkbenchHandlerWithOptions(replayWorkbenchHandlerOptions{
			WorkspaceRoot:                         root,
			CollaborationNotes:                    []app.ReplayWorkbenchCollaborationNote{note},
			AllowCollaborationNoteSelectionWrites: true,
		}, replay)

		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/collaboration-note-selection", strings.NewReader(`{"action":"clear","actor":"browser"}`)))
		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("expected symlinked note marker POST to fail 400, got %d body=%q", recorder.Code, recorder.Body.String())
		}
		if after := replayWorkbenchReadFileForHandlerTest(t, outside); string(after) != string(before) {
			t.Fatalf("symlinked note marker POST must not rewrite outside file")
		}
	})

	t.Run("report selection marker", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "workspace")
		execution := createReportProjectionDispatchExecutionForCLI(t, root)
		replay, err := app.ReplayWorkspace(context.Background(), root, execution.RunID)
		if err != nil {
			t.Fatalf("replay report selection safety fixture: %v", err)
		}
		if _, err := app.SelectReportProjectionDispatchExecution(root, filepath.Base(execution.Path), app.ReportProjectionDispatchExecutionSelectionOptions{SelectedBy: "alice"}); err != nil {
			t.Fatalf("select report execution before symlink: %v", err)
		}
		outside := replayWorkbenchSymlinkCopyForHandlerTest(t, filepath.Join(root, "reports", "report-projection-selection.json"), "outside-report-selection.json")
		before := replayWorkbenchReadFileForHandlerTest(t, outside)
		handler := newReplayWorkbenchHandlerWithOptions(replayWorkbenchHandlerOptions{
			WorkspaceRoot:               root,
			ReportProjectionExecutions:  []app.ReportProjectionDispatchExecution{execution},
			AllowReportSelectionWrites:  true,
			AllowSessionSelectionWrites: false,
		}, replay)

		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/report-selection", strings.NewReader(`{"action":"clear","actor":"browser"}`)))
		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("expected symlinked report selection marker POST to fail 400, got %d body=%q", recorder.Code, recorder.Body.String())
		}
		if after := replayWorkbenchReadFileForHandlerTest(t, outside); string(after) != string(before) {
			t.Fatalf("symlinked report selection marker POST must not rewrite outside file")
		}
	})

	t.Run("presence artifact", func(t *testing.T) {
		root, replay, _, collaboration, _ := replayWorkbenchBrowserWriteSafetyFixture(t)
		presence, err := app.UpdateReplayWorkbenchCollaborationPresence(context.Background(), root, app.ReplayWorkbenchCollaborationPresenceOptions{
			CollaborationPath: filepath.Base(collaboration.Path),
			Participant:       "alice",
			Status:            "online",
			SeenBy:            "alice",
			SeenAt:            time.Date(2026, 6, 2, 9, 0, 0, 0, time.UTC),
		})
		if err != nil {
			t.Fatalf("create presence before symlink: %v", err)
		}
		outside := replayWorkbenchSymlinkCopyForHandlerTest(t, presence.Path, "outside-presence.json")
		before := replayWorkbenchReadFileForHandlerTest(t, outside)
		handler := newReplayWorkbenchHandlerWithOptions(replayWorkbenchHandlerOptions{
			WorkspaceRoot:                    root,
			Collaborations:                   []app.ReplayWorkbenchCollaborationArtifact{collaboration},
			AllowCollaborationPresenceWrites: true,
		}, replay)
		body := fmt.Sprintf(`{"collaboration_path":%q,"participant":"alice","status":"away","seen_by":"browser","seen_at":"2026-06-02T09:01:00Z"}`, filepath.Base(collaboration.Path))

		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/collaboration-presence", strings.NewReader(body)))
		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("expected symlinked presence POST to fail 400, got %d body=%q", recorder.Code, recorder.Body.String())
		}
		if after := replayWorkbenchReadFileForHandlerTest(t, outside); string(after) != string(before) {
			t.Fatalf("symlinked presence POST must not rewrite outside file")
		}
	})

	t.Run("note artifact", func(t *testing.T) {
		root, replay, _, collaboration, note := replayWorkbenchBrowserWriteSafetyFixture(t)
		outside := replayWorkbenchSymlinkCopyForHandlerTest(t, note.Path, "outside-note.json")
		before := replayWorkbenchReadFileForHandlerTest(t, outside)
		handler := newReplayWorkbenchHandlerWithOptions(replayWorkbenchHandlerOptions{
			WorkspaceRoot:                root,
			Collaborations:               []app.ReplayWorkbenchCollaborationArtifact{collaboration},
			AllowCollaborationNoteWrites: true,
		}, replay)
		body := fmt.Sprintf(`{"collaboration_path":%q,"author":"alice","body":"handler safety note","target_kind":"graph-node","target_id":"fact-target","created_at":"2026-06-02T09:02:00Z"}`, filepath.Base(collaboration.Path))

		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/collaboration-notes", strings.NewReader(body)))
		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("expected symlinked note POST to fail 400, got %d body=%q", recorder.Code, recorder.Body.String())
		}
		if after := replayWorkbenchReadFileForHandlerTest(t, outside); string(after) != string(before) {
			t.Fatalf("symlinked note POST must not rewrite outside file")
		}
	})
}

func replayWorkbenchBrowserWriteSafetyFixture(t *testing.T) (string, app.ReplayResult, app.ReplayWorkbenchSessionArtifact, app.ReplayWorkbenchCollaborationArtifact, app.ReplayWorkbenchCollaborationNote) {
	t.Helper()
	root := filepath.Join(t.TempDir(), "workspace")
	if err := app.InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	run, err := app.RunWorkspace(context.Background(), root)
	if err != nil {
		t.Fatalf("run workspace: %v", err)
	}
	session, err := app.CreateReplayWorkbenchSession(context.Background(), root, app.ReplayWorkbenchSessionOptions{
		PrimaryRunID: run.RunID,
		CreatedAt:    time.Date(2026, 6, 2, 9, 0, 0, 0, time.UTC),
		Reason:       "handler safety fixture",
	})
	if err != nil {
		t.Fatalf("create session fixture: %v", err)
	}
	collaboration, err := app.CreateReplayWorkbenchCollaboration(context.Background(), root, app.ReplayWorkbenchCollaborationOptions{
		SessionPath:  filepath.Base(session.Path),
		Participants: []string{"alice", "bob"},
		CreatedBy:    "lead",
		CreatedAt:    time.Date(2026, 6, 2, 9, 1, 0, 0, time.UTC),
		Reason:       "handler safety collaboration",
	})
	if err != nil {
		t.Fatalf("create collaboration fixture: %v", err)
	}
	note, err := app.CreateReplayWorkbenchCollaborationNote(context.Background(), root, app.ReplayWorkbenchCollaborationNoteOptions{
		CollaborationPath: filepath.Base(collaboration.Path),
		Author:            "alice",
		Body:              "handler safety note",
		TargetKind:        "graph-node",
		TargetID:          "fact-target",
		CreatedAt:         time.Date(2026, 6, 2, 9, 2, 0, 0, time.UTC),
		Reason:            "handler safety note",
	})
	if err != nil {
		t.Fatalf("create note fixture: %v", err)
	}
	replay, err := app.ReplayWorkspace(context.Background(), root, run.RunID)
	if err != nil {
		t.Fatalf("replay fixture: %v", err)
	}
	return root, replay, session, collaboration, note
}

func replayWorkbenchSymlinkCopyForHandlerTest(t *testing.T, originalPath string, outsideName string) string {
	t.Helper()
	data := replayWorkbenchReadFileForHandlerTest(t, originalPath)
	outside := filepath.Join(t.TempDir(), outsideName)
	if err := os.WriteFile(outside, data, 0o644); err != nil {
		t.Fatalf("write outside symlink target: %v", err)
	}
	if err := os.Remove(originalPath); err != nil {
		t.Fatalf("remove original before symlink: %v", err)
	}
	if err := os.Symlink(outside, originalPath); err != nil {
		t.Fatalf("symlink original to outside target: %v", err)
	}
	return outside
}

func replayWorkbenchReadFileForHandlerTest(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return data
}

func TestReplayWorkbenchHandlerWritesSessionSelectionOnlyWithExplicitGate(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := app.InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	primary, err := app.RunWorkspace(context.Background(), root)
	if err != nil {
		t.Fatalf("run primary workspace: %v", err)
	}
	comparison, err := app.RunWorkspace(context.Background(), root)
	if err != nil {
		t.Fatalf("run comparison workspace: %v", err)
	}
	session, err := app.CreateReplayWorkbenchSession(context.Background(), root, app.ReplayWorkbenchSessionOptions{
		PrimaryRunID:  primary.RunID,
		CompareRunIDs: []string{comparison.RunID},
		SelectedRunID: comparison.RunID,
		Filters: map[string]string{
			"lane": "toolchain",
			"q":    "browser-session",
		},
		CreatedBy: "operator",
		Reason:    "browser session selection source",
	})
	if err != nil {
		t.Fatalf("create replay workbench session: %v", err)
	}
	before, err := app.ReplayWorkspace(context.Background(), root, primary.RunID)
	if err != nil {
		t.Fatalf("replay before browser session selection write: %v", err)
	}
	body := `{"action":"select","session_path":"` + filepath.Base(session.Path) + `","actor":"browser","at":"2026-06-01T12:05:00Z","reason":"browser selected session"}`
	disabled := newReplayWorkbenchHandlerWithOptions(replayWorkbenchHandlerOptions{
		WorkspaceRoot: root,
		SavedSessions: []app.ReplayWorkbenchSessionArtifact{session},
	}, before)
	recorder := httptest.NewRecorder()
	disabled.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/session-selection", strings.NewReader(body)))
	if recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected disabled session selection POST to be rejected, got %d body=%q", recorder.Code, recorder.Body.String())
	}
	if _, found, err := app.ReadReplayWorkbenchSessionSelection(root); err != nil {
		t.Fatalf("read disabled session selection marker: %v", err)
	} else if found {
		t.Fatalf("disabled browser session selection write created a marker")
	}

	enabled := newReplayWorkbenchHandlerWithOptions(replayWorkbenchHandlerOptions{
		WorkspaceRoot:                    root,
		SavedSessions:                    []app.ReplayWorkbenchSessionArtifact{session},
		AllowSessionSelectionWrites:      true,
		CollaborationSelection:           nil,
		CollaborationNoteSelection:       nil,
		CollaborationPresence:            nil,
		CollaborationNotes:               nil,
		AllowCollaborationNoteWrites:     false,
		EnableCollaborationStream:        true,
		CollaborationNotesPollIntervalMS: 0,
	}, before)
	recorder = httptest.NewRecorder()
	enabled.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/session-selection", strings.NewReader(body)))
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected enabled session selection POST status 200, got %d body=%q", recorder.Code, recorder.Body.String())
	}
	var selected app.ReplayWorkbenchSessionSelection
	if err := json.NewDecoder(recorder.Body).Decode(&selected); err != nil {
		t.Fatalf("decode selected session marker: %v", err)
	}
	if selected.Status != "current" ||
		selected.SessionPath != session.Path ||
		selected.SelectedBy != "browser" ||
		selected.Reason != "browser selected session" {
		t.Fatalf("unexpected selected session marker: %#v", selected)
	}
	recorder = httptest.NewRecorder()
	enabled.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/session-selection", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected session selection GET status 200, got %d body=%q", recorder.Code, recorder.Body.String())
	}
	var listed app.ReplayWorkbenchSessionSelection
	if err := json.NewDecoder(recorder.Body).Decode(&listed); err != nil {
		t.Fatalf("decode listed session marker: %v", err)
	}
	if listed.SessionPath != session.Path || listed.Status != "current" {
		t.Fatalf("unexpected listed session marker: %#v", listed)
	}
	clearBody := `{"action":"clear","actor":"browser","at":"2026-06-01T12:06:00Z","reason":"browser cleared session"}`
	recorder = httptest.NewRecorder()
	enabled.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/session-selection", strings.NewReader(clearBody)))
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected enabled session selection clear status 200, got %d body=%q", recorder.Code, recorder.Body.String())
	}
	var cleared app.ReplayWorkbenchSessionSelection
	if err := json.NewDecoder(recorder.Body).Decode(&cleared); err != nil {
		t.Fatalf("decode cleared session marker: %v", err)
	}
	if cleared.Status != "cleared" || cleared.SessionPath != session.Path || cleared.ClearedBy != "browser" {
		t.Fatalf("unexpected cleared session marker: %#v", cleared)
	}
	after, err := app.ReplayWorkspace(context.Background(), root, primary.RunID)
	if err != nil {
		t.Fatalf("replay after browser session selection write: %v", err)
	}
	if after.EventCount != before.EventCount {
		t.Fatalf("browser session selection write must not mutate replay events, before=%d after=%d", before.EventCount, after.EventCount)
	}
}

func TestReplayWorkbenchHandlerRendersSessionSelectionWriteControlOnlyWithExplicitGate(t *testing.T) {
	session := app.ReplayWorkbenchSessionArtifact{
		Path:          "/workspace/replay/workbench-sessions/session-one.json",
		SchemaVersion: 1,
		Status:        "saved",
		Mode:          "metadata-only",
		PrimaryRunID:  "run-session-primary",
		SelectedRunID: "run-session-primary",
		RunIDs:        []string{"run-session-primary"},
		CreatedBy:     "operator",
		CreatedAt:     "2026-06-01T12:10:00Z",
		Reason:        "render session selection write",
	}
	primary := app.ReplayResult{RunID: "run-session-primary"}
	disabled := newReplayWorkbenchHandlerWithOptions(replayWorkbenchHandlerOptions{
		SavedSessions: []app.ReplayWorkbenchSessionArtifact{session},
	}, primary)
	recorder := httptest.NewRecorder()
	disabled.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected disabled HTML status 200, got %d body=%q", recorder.Code, recorder.Body.String())
	}
	disabledBody := recorder.Body.String()
	if strings.Contains(disabledBody, "ouro-replay-session-selection-form") ||
		strings.Contains(disabledBody, "fetch(\"/api/session-selection\", {") {
		t.Fatalf("expected disabled session-selection HTML to omit write controls, got %q", disabledBody)
	}

	enabled := newReplayWorkbenchHandlerWithOptions(replayWorkbenchHandlerOptions{
		SavedSessions:               []app.ReplayWorkbenchSessionArtifact{session},
		AllowSessionSelectionWrites: true,
	}, primary)
	recorder = httptest.NewRecorder()
	enabled.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected enabled HTML status 200, got %d body=%q", recorder.Code, recorder.Body.String())
	}
	enabledBody := recorder.Body.String()
	for _, want := range []string{
		"window.OURO_REPLAY_SESSION_SELECTION_WRITES_ENABLED = true",
		"id=\"ouro-replay-session-selection-form\"",
		"id=\"ouro-replay-session-selection-session\"",
		"id=\"ouro-replay-session-selection-actor\"",
		"id=\"ouro-replay-session-selection-reason\"",
		"id=\"ouro-replay-session-selection-select\"",
		"id=\"ouro-replay-session-selection-clear\"",
		"id=\"ouro-replay-session-selection-write-status\"",
		"installReplayWorkbenchSessionSelectionWriter",
		"fetch(\"/api/session-selection\", {",
		"renderReplayWorkbenchSessionSelection();",
	} {
		if !strings.Contains(enabledBody, want) {
			t.Fatalf("expected enabled session-selection HTML to contain %q, got %q", want, enabledBody)
		}
	}
}

func TestReplayWorkbenchHandlerRendersSessionSelectionPollingOnlyWithExplicitInterval(t *testing.T) {
	primary := app.ReplayResult{RunID: "run-session-primary"}
	selection := app.ReplayWorkbenchSessionSelection{
		Path:          "/workspace/replay/workbench-session-selection.json",
		Status:        "current",
		Mode:          "metadata-only",
		SessionPath:   "/workspace/replay/workbench-sessions/session-one.json",
		PrimaryRunID:  primary.RunID,
		SelectedRunID: primary.RunID,
		RunIDs:        1,
		Filters:       1,
		Endpoints:     2,
		SelectedBy:    "operator",
		SelectedAt:    "2026-06-01T12:25:00Z",
		Reason:        "session selection polling baseline",
	}

	disabled := newReplayWorkbenchHandlerWithOptions(replayWorkbenchHandlerOptions{
		SessionSelection: &selection,
	}, primary)
	recorder := httptest.NewRecorder()
	disabled.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected disabled session selection polling HTML status 200, got %d body=%q", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "window.OURO_REPLAY_SESSION_SELECTION_POLL_INTERVAL_MS = 0") {
		t.Fatalf("expected disabled session selection polling interval in HTML, got %q", recorder.Body.String())
	}
	if strings.Contains(recorder.Body.String(), "setInterval(refreshReplayWorkbenchSessionSelection") ||
		strings.Contains(recorder.Body.String(), "installReplayWorkbenchSessionSelectionPoller") {
		t.Fatalf("expected default session selection polling HTML to avoid background polling, got %q", recorder.Body.String())
	}

	enabled := newReplayWorkbenchHandlerWithOptions(replayWorkbenchHandlerOptions{
		SessionSelection:               &selection,
		SessionSelectionPollIntervalMS: 1500,
	}, primary)
	recorder = httptest.NewRecorder()
	enabled.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected enabled session selection polling HTML status 200, got %d body=%q", recorder.Code, recorder.Body.String())
	}
	for _, want := range []string{
		"window.OURO_REPLAY_SESSION_SELECTION_POLL_INTERVAL_MS = 1500",
		"const sessionSelectionPollIntervalMS = 1500",
		"installReplayWorkbenchSessionSelectionPoller",
		"refreshReplayWorkbenchSessionSelection",
		"setInterval(refreshReplayWorkbenchSessionSelection, sessionSelectionPollIntervalMS)",
		"fetch(\"/api/session-selection\")",
		"renderReplayWorkbenchSessionSelection();",
	} {
		if !strings.Contains(recorder.Body.String(), want) {
			t.Fatalf("expected enabled session selection polling HTML to contain %q, got %q", want, recorder.Body.String())
		}
	}
	if strings.Contains(recorder.Body.String(), "method: \"POST\"") {
		t.Fatalf("session selection polling alone must not expose a POST path, got %q", recorder.Body.String())
	}
}

func TestReplayWorkbenchHandlerWritesCollaborationPresenceOnlyWithExplicitGate(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := app.InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	run, err := app.RunWorkspace(context.Background(), root)
	if err != nil {
		t.Fatalf("run workspace: %v", err)
	}
	session, err := app.CreateReplayWorkbenchSession(context.Background(), root, app.ReplayWorkbenchSessionOptions{
		PrimaryRunID: run.RunID,
		CreatedBy:    "operator",
		Reason:       "presence write gate baseline",
	})
	if err != nil {
		t.Fatalf("create replay workbench session: %v", err)
	}
	collaboration, err := app.CreateReplayWorkbenchCollaboration(context.Background(), root, app.ReplayWorkbenchCollaborationOptions{
		SessionPath:  filepath.Base(session.Path),
		Participants: []string{"alice", "bob"},
		CreatedBy:    "lead",
		Reason:       "browser presence gate",
	})
	if err != nil {
		t.Fatalf("create replay workbench collaboration: %v", err)
	}
	before, err := app.ReplayWorkspace(context.Background(), root, run.RunID)
	if err != nil {
		t.Fatalf("replay before collaboration presence browser write: %v", err)
	}

	disabled := newReplayWorkbenchHandlerWithOptions(replayWorkbenchHandlerOptions{
		WorkspaceRoot: root,
	}, before)
	disabledRecorder := httptest.NewRecorder()
	disabled.ServeHTTP(disabledRecorder, httptest.NewRequest(http.MethodPost, "/api/collaboration-presence", strings.NewReader(`{"participant":"alice"}`)))
	if disabledRecorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected collaboration presence browser writes to be disabled by default, got %d body=%q", disabledRecorder.Code, disabledRecorder.Body.String())
	}

	handler := newReplayWorkbenchHandlerWithOptions(replayWorkbenchHandlerOptions{
		WorkspaceRoot:                    root,
		Collaborations:                   []app.ReplayWorkbenchCollaborationArtifact{collaboration},
		AllowCollaborationPresenceWrites: true,
	}, before)
	body := fmt.Sprintf(`{"collaboration_path":%q,"participant":"alice","status":"online","seen_by":"browser","seen_at":"2026-06-01T06:10:00Z","reason":"browser heartbeat"}`, filepath.Base(collaboration.Path))
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/collaboration-presence", strings.NewReader(body)))
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected collaboration presence browser write status 200, got %d body=%q", recorder.Code, recorder.Body.String())
	}
	var written app.ReplayWorkbenchCollaborationPresence
	if err := json.NewDecoder(recorder.Body).Decode(&written); err != nil {
		t.Fatalf("decode written collaboration presence: %v", err)
	}
	if written.Status != "online" ||
		written.Mode != "metadata-only" ||
		written.Participant != "alice" ||
		written.SeenBy != "browser" ||
		written.CollaborationPath != collaboration.Path ||
		written.SessionPath != session.Path {
		t.Fatalf("unexpected written collaboration presence: %#v", written)
	}
	if written.Path == "" {
		t.Fatalf("expected written collaboration presence path")
	}

	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/collaboration-presence", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected collaboration presence GET after write status 200, got %d body=%q", recorder.Code, recorder.Body.String())
	}
	var listed []app.ReplayWorkbenchCollaborationPresence
	if err := json.NewDecoder(recorder.Body).Decode(&listed); err != nil {
		t.Fatalf("decode listed collaboration presence: %v", err)
	}
	if len(listed) != 1 || listed[0].Path != written.Path || listed[0].Reason != "browser heartbeat" {
		t.Fatalf("unexpected listed collaboration presence after write: %#v", listed)
	}

	after, err := app.ReplayWorkspace(context.Background(), root, run.RunID)
	if err != nil {
		t.Fatalf("replay after collaboration presence browser write: %v", err)
	}
	if after.EventCount != before.EventCount {
		t.Fatalf("browser collaboration presence write must not mutate replay events, before=%d after=%d", before.EventCount, after.EventCount)
	}
}

func TestReplayWorkbenchHandlerWritesCollaborationSelectionOnlyWithExplicitGate(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := app.InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	run, err := app.RunWorkspace(context.Background(), root)
	if err != nil {
		t.Fatalf("run workspace: %v", err)
	}
	session, err := app.CreateReplayWorkbenchSession(context.Background(), root, app.ReplayWorkbenchSessionOptions{
		PrimaryRunID: run.RunID,
		CreatedBy:    "operator",
		Reason:       "selection browser baseline",
	})
	if err != nil {
		t.Fatalf("create replay workbench session: %v", err)
	}
	collaboration, err := app.CreateReplayWorkbenchCollaboration(context.Background(), root, app.ReplayWorkbenchCollaborationOptions{
		SessionPath:  filepath.Base(session.Path),
		Participants: []string{"alice", "bob"},
		CreatedBy:    "lead",
		Reason:       "browser collaboration focus",
	})
	if err != nil {
		t.Fatalf("create replay workbench collaboration: %v", err)
	}
	before, err := app.ReplayWorkspace(context.Background(), root, run.RunID)
	if err != nil {
		t.Fatalf("replay before collaboration selection browser write: %v", err)
	}
	body := fmt.Sprintf(`{"action":"select","collaboration_path":%q,"actor":"browser","at":"2026-06-01T06:50:00Z","reason":"browser selected collaboration"}`, filepath.Base(collaboration.Path))

	disabled := newReplayWorkbenchHandlerWithOptions(replayWorkbenchHandlerOptions{
		WorkspaceRoot:  root,
		Collaborations: []app.ReplayWorkbenchCollaborationArtifact{collaboration},
	}, before)
	recorder := httptest.NewRecorder()
	disabled.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/collaboration-selection", strings.NewReader(body)))
	if recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected disabled collaboration selection write to be rejected, got %d body=%q", recorder.Code, recorder.Body.String())
	}
	if _, found, err := app.ReadReplayWorkbenchCollaborationSelection(root); err != nil {
		t.Fatalf("read collaboration selection after rejected write: %v", err)
	} else if found {
		t.Fatalf("disabled browser selection write must not create a selection marker")
	}

	enabled := newReplayWorkbenchHandlerWithOptions(replayWorkbenchHandlerOptions{
		WorkspaceRoot:                     root,
		Collaborations:                    []app.ReplayWorkbenchCollaborationArtifact{collaboration},
		AllowCollaborationSelectionWrites: true,
	}, before)
	recorder = httptest.NewRecorder()
	enabled.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/collaboration-selection", strings.NewReader(body)))
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected enabled collaboration selection write status 200, got %d body=%q", recorder.Code, recorder.Body.String())
	}
	var selected app.ReplayWorkbenchCollaborationSelection
	if err := json.NewDecoder(recorder.Body).Decode(&selected); err != nil {
		t.Fatalf("decode selected collaboration marker: %v", err)
	}
	if selected.Status != "current" || selected.CollaborationPath != collaboration.Path || selected.SelectedBy != "browser" || selected.Reason != "browser selected collaboration" {
		t.Fatalf("unexpected selected collaboration marker: %#v", selected)
	}

	recorder = httptest.NewRecorder()
	enabled.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/collaboration-selection", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected collaboration selection GET status 200, got %d body=%q", recorder.Code, recorder.Body.String())
	}
	var listed app.ReplayWorkbenchCollaborationSelection
	if err := json.NewDecoder(recorder.Body).Decode(&listed); err != nil {
		t.Fatalf("decode listed collaboration selection: %v", err)
	}
	if listed.Status != "current" || listed.Path != selected.Path {
		t.Fatalf("expected GET to return selected marker, got %#v", listed)
	}

	clearBody := `{"action":"clear","actor":"browser","at":"2026-06-01T06:51:00Z","reason":"browser cleared collaboration"}`
	recorder = httptest.NewRecorder()
	enabled.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/collaboration-selection", strings.NewReader(clearBody)))
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected enabled collaboration selection clear status 200, got %d body=%q", recorder.Code, recorder.Body.String())
	}
	var cleared app.ReplayWorkbenchCollaborationSelection
	if err := json.NewDecoder(recorder.Body).Decode(&cleared); err != nil {
		t.Fatalf("decode cleared collaboration marker: %v", err)
	}
	if cleared.Status != "cleared" || cleared.ClearedBy != "browser" || cleared.Reason != "browser cleared collaboration" {
		t.Fatalf("unexpected cleared collaboration marker: %#v", cleared)
	}

	after, err := app.ReplayWorkspace(context.Background(), root, run.RunID)
	if err != nil {
		t.Fatalf("replay after collaboration selection browser write: %v", err)
	}
	if after.EventCount != before.EventCount {
		t.Fatalf("browser collaboration selection write must not mutate replay events, before=%d after=%d", before.EventCount, after.EventCount)
	}
}

func TestReplayWorkbenchHandlerWritesCollaborationNoteSelectionOnlyWithExplicitGate(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := app.InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	run, err := app.RunWorkspace(context.Background(), root)
	if err != nil {
		t.Fatalf("run workspace: %v", err)
	}
	session, err := app.CreateReplayWorkbenchSession(context.Background(), root, app.ReplayWorkbenchSessionOptions{
		PrimaryRunID: run.RunID,
		CreatedBy:    "operator",
		Reason:       "note selection browser baseline",
	})
	if err != nil {
		t.Fatalf("create replay workbench session: %v", err)
	}
	collaboration, err := app.CreateReplayWorkbenchCollaboration(context.Background(), root, app.ReplayWorkbenchCollaborationOptions{
		SessionPath:  filepath.Base(session.Path),
		Participants: []string{"alice", "bob"},
		CreatedBy:    "lead",
		Reason:       "browser note selection collaboration",
	})
	if err != nil {
		t.Fatalf("create replay workbench collaboration: %v", err)
	}
	note, err := app.CreateReplayWorkbenchCollaborationNote(context.Background(), root, app.ReplayWorkbenchCollaborationNoteOptions{
		CollaborationPath: filepath.Base(collaboration.Path),
		Author:            "alice",
		Body:              "Select this review note.",
		TargetKind:        "graph-node",
		TargetID:          "fact-target",
		Reason:            "browser note selection candidate",
	})
	if err != nil {
		t.Fatalf("create replay workbench collaboration note: %v", err)
	}
	before, err := app.ReplayWorkspace(context.Background(), root, run.RunID)
	if err != nil {
		t.Fatalf("replay before collaboration note selection browser write: %v", err)
	}
	body := fmt.Sprintf(`{"action":"select","note_path":%q,"actor":"browser","at":"2026-06-01T10:55:00Z","reason":"browser selected note focus"}`, filepath.Base(note.Path))

	disabled := newReplayWorkbenchHandlerWithOptions(replayWorkbenchHandlerOptions{
		WorkspaceRoot:      root,
		CollaborationNotes: []app.ReplayWorkbenchCollaborationNote{note},
		CollaborationSelection: &app.ReplayWorkbenchCollaborationSelection{
			Status:            "current",
			Mode:              "metadata-only",
			CollaborationPath: collaboration.Path,
			SessionPath:       session.Path,
		},
	}, before)
	recorder := httptest.NewRecorder()
	disabled.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/collaboration-note-selection", strings.NewReader(body)))
	if recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected disabled collaboration note selection write to be rejected, got %d body=%q", recorder.Code, recorder.Body.String())
	}
	if _, found, err := app.ReadReplayWorkbenchCollaborationNoteSelection(root); err != nil {
		t.Fatalf("read note selection after rejected write: %v", err)
	} else if found {
		t.Fatalf("disabled browser note selection write must not create a selection marker")
	}

	enabled := newReplayWorkbenchHandlerWithOptions(replayWorkbenchHandlerOptions{
		WorkspaceRoot:                         root,
		CollaborationNotes:                    []app.ReplayWorkbenchCollaborationNote{note},
		AllowCollaborationNoteSelectionWrites: true,
		EnableCollaborationStream:             true,
		AllowCollaborationSelectionWrites:     false,
		AllowCollaborationNoteWrites:          false,
		AllowCollaborationPresenceWrites:      false,
		CollaborationSelectionPollIntervalMS:  0,
		CollaborationPresencePollIntervalMS:   0,
		CollaborationNotesPollIntervalMS:      0,
		CollaborationStreamRefreshIntervalMS:  0,
	}, before)
	recorder = httptest.NewRecorder()
	enabled.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/collaboration-note-selection", strings.NewReader(body)))
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected enabled collaboration note selection write status 200, got %d body=%q", recorder.Code, recorder.Body.String())
	}
	var selected app.ReplayWorkbenchCollaborationNoteSelection
	if err := json.NewDecoder(recorder.Body).Decode(&selected); err != nil {
		t.Fatalf("decode selected collaboration note marker: %v", err)
	}
	if selected.Status != "current" ||
		selected.Mode != "metadata-only" ||
		selected.NotePath != note.Path ||
		selected.CollaborationPath != collaboration.Path ||
		selected.SessionPath != session.Path ||
		selected.SelectedBy != "browser" ||
		selected.Reason != "browser selected note focus" {
		t.Fatalf("unexpected selected collaboration note marker: %#v", selected)
	}

	recorder = httptest.NewRecorder()
	enabled.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/collaboration-note-selection", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected collaboration note selection GET status 200, got %d body=%q", recorder.Code, recorder.Body.String())
	}
	var listed app.ReplayWorkbenchCollaborationNoteSelection
	if err := json.NewDecoder(recorder.Body).Decode(&listed); err != nil {
		t.Fatalf("decode listed collaboration note selection: %v", err)
	}
	if listed.Status != "current" || listed.Path != selected.Path || listed.NotePath != note.Path {
		t.Fatalf("expected GET to return selected note marker, got %#v", listed)
	}

	clearBody := `{"action":"clear","actor":"browser","at":"2026-06-01T10:56:00Z","reason":"browser cleared note focus"}`
	recorder = httptest.NewRecorder()
	enabled.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/collaboration-note-selection", strings.NewReader(clearBody)))
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected enabled collaboration note selection clear status 200, got %d body=%q", recorder.Code, recorder.Body.String())
	}
	var cleared app.ReplayWorkbenchCollaborationNoteSelection
	if err := json.NewDecoder(recorder.Body).Decode(&cleared); err != nil {
		t.Fatalf("decode cleared collaboration note marker: %v", err)
	}
	if cleared.Status != "cleared" || cleared.ClearedBy != "browser" || cleared.Reason != "browser cleared note focus" {
		t.Fatalf("unexpected cleared collaboration note marker: %#v", cleared)
	}

	after, err := app.ReplayWorkspace(context.Background(), root, run.RunID)
	if err != nil {
		t.Fatalf("replay after collaboration note selection browser write: %v", err)
	}
	if after.EventCount != before.EventCount {
		t.Fatalf("browser collaboration note selection write must not mutate replay events, before=%d after=%d", before.EventCount, after.EventCount)
	}
}

func TestReplayWorkbenchHandlerWritesCollaborationNotesOnlyWithExplicitGate(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := app.InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	run, err := app.RunWorkspace(context.Background(), root)
	if err != nil {
		t.Fatalf("run workspace: %v", err)
	}
	session, err := app.CreateReplayWorkbenchSession(context.Background(), root, app.ReplayWorkbenchSessionOptions{
		PrimaryRunID: run.RunID,
		CreatedBy:    "operator",
		Reason:       "note browser baseline",
	})
	if err != nil {
		t.Fatalf("create replay workbench session: %v", err)
	}
	collaboration, err := app.CreateReplayWorkbenchCollaboration(context.Background(), root, app.ReplayWorkbenchCollaborationOptions{
		SessionPath:  filepath.Base(session.Path),
		Participants: []string{"alice", "bob"},
		CreatedBy:    "lead",
		Reason:       "browser note collaboration",
	})
	if err != nil {
		t.Fatalf("create replay workbench collaboration: %v", err)
	}
	before, err := app.ReplayWorkspace(context.Background(), root, run.RunID)
	if err != nil {
		t.Fatalf("replay before collaboration note browser write: %v", err)
	}
	body := fmt.Sprintf(`{"collaboration_path":%q,"author":"browser","body":"Review graph node before accepting.","target_kind":"graph-node","target_id":"fact-target","created_at":"2026-06-01T08:45:00Z","reason":"browser evidence note"}`, filepath.Base(collaboration.Path))

	disabled := newReplayWorkbenchHandlerWithOptions(replayWorkbenchHandlerOptions{
		WorkspaceRoot:  root,
		Collaborations: []app.ReplayWorkbenchCollaborationArtifact{collaboration},
	}, before)
	recorder := httptest.NewRecorder()
	disabled.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/collaboration-notes", strings.NewReader(body)))
	if recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected disabled collaboration note write to be rejected, got %d body=%q", recorder.Code, recorder.Body.String())
	}
	if notes, err := app.ListReplayWorkbenchCollaborationNotes(root); err != nil {
		t.Fatalf("list notes after rejected browser write: %v", err)
	} else if len(notes) != 0 {
		t.Fatalf("disabled browser note write must not create note artifacts: %#v", notes)
	}

	enabled := newReplayWorkbenchHandlerWithOptions(replayWorkbenchHandlerOptions{
		WorkspaceRoot:                    root,
		Collaborations:                   []app.ReplayWorkbenchCollaborationArtifact{collaboration},
		AllowCollaborationNoteWrites:     true,
		EnableCollaborationStream:        true,
		CollaborationNotes:               nil,
		CollaborationPresence:            nil,
		CollaborationSelection:           nil,
		AllowCollaborationPresenceWrites: false,
	}, before)
	recorder = httptest.NewRecorder()
	enabled.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/collaboration-notes", strings.NewReader(body)))
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected enabled collaboration note write status 200, got %d body=%q", recorder.Code, recorder.Body.String())
	}
	var written app.ReplayWorkbenchCollaborationNote
	if err := json.NewDecoder(recorder.Body).Decode(&written); err != nil {
		t.Fatalf("decode written collaboration note: %v", err)
	}
	if written.Status != "saved" ||
		written.Mode != "metadata-only" ||
		written.Author != "browser" ||
		written.Body != "Review graph node before accepting." ||
		written.TargetKind != "graph-node" ||
		written.TargetID != "fact-target" ||
		written.CollaborationPath != collaboration.Path ||
		written.SessionPath != session.Path {
		t.Fatalf("unexpected written collaboration note: %#v", written)
	}
	if written.Path == "" {
		t.Fatalf("expected written collaboration note path")
	}

	recorder = httptest.NewRecorder()
	enabled.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/collaboration-notes", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected collaboration notes GET after write status 200, got %d body=%q", recorder.Code, recorder.Body.String())
	}
	var listed []app.ReplayWorkbenchCollaborationNote
	if err := json.NewDecoder(recorder.Body).Decode(&listed); err != nil {
		t.Fatalf("decode listed collaboration notes: %v", err)
	}
	if len(listed) != 1 || listed[0].Path != written.Path || listed[0].Reason != "browser evidence note" {
		t.Fatalf("unexpected listed collaboration notes after write: %#v", listed)
	}

	after, err := app.ReplayWorkspace(context.Background(), root, run.RunID)
	if err != nil {
		t.Fatalf("replay after collaboration note browser write: %v", err)
	}
	if after.EventCount != before.EventCount {
		t.Fatalf("browser collaboration note write must not mutate replay events, before=%d after=%d", before.EventCount, after.EventCount)
	}
}

func TestReplayWorkbenchHandlerRendersCollaborationPresenceWriteControlOnlyWithExplicitGate(t *testing.T) {
	primary := app.ReplayResult{RunID: "run-primary"}
	collaboration := app.ReplayWorkbenchCollaborationArtifact{
		Path:          "/workspace/replay/workbench-collaborations/collab-one.json",
		Status:        "dry-run",
		Mode:          "metadata-only",
		SessionPath:   "/workspace/replay/workbench-sessions/session-one.json",
		PrimaryRunID:  primary.RunID,
		SelectedRunID: primary.RunID,
		Participants:  []string{"alice", "bob"},
		CreatedBy:     "lead",
		CreatedAt:     "2026-06-01T06:30:00Z",
		Reason:        "browser presence form",
	}

	disabled := newReplayWorkbenchHandlerWithOptions(replayWorkbenchHandlerOptions{
		Collaborations: []app.ReplayWorkbenchCollaborationArtifact{collaboration},
	}, primary)
	recorder := httptest.NewRecorder()
	disabled.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected disabled presence form HTML status 200, got %d body=%q", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "window.OURO_REPLAY_COLLABORATION_PRESENCE_WRITES_ENABLED = false") {
		t.Fatalf("expected disabled presence writes flag in HTML, got %q", recorder.Body.String())
	}
	if strings.Contains(recorder.Body.String(), "ouro-replay-collaboration-presence-form") ||
		strings.Contains(recorder.Body.String(), "method: \"POST\"") {
		t.Fatalf("expected disabled presence write HTML to stay read-only, got %q", recorder.Body.String())
	}

	enabled := newReplayWorkbenchHandlerWithOptions(replayWorkbenchHandlerOptions{
		Collaborations:                   []app.ReplayWorkbenchCollaborationArtifact{collaboration},
		AllowCollaborationPresenceWrites: true,
	}, primary)
	recorder = httptest.NewRecorder()
	enabled.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected enabled presence form HTML status 200, got %d body=%q", recorder.Code, recorder.Body.String())
	}
	for _, want := range []string{
		"window.OURO_REPLAY_COLLABORATION_PRESENCE_WRITES_ENABLED = true",
		"id=\"ouro-replay-collaboration-presence-form\"",
		"id=\"ouro-replay-presence-collaboration\"",
		"id=\"ouro-replay-presence-participant\"",
		"id=\"ouro-replay-presence-status\"",
		"id=\"ouro-replay-presence-seen-by\"",
		"id=\"ouro-replay-presence-reason\"",
		"id=\"ouro-replay-presence-write-status\"",
		"fetch(\"/api/collaboration-presence\"",
		"method: \"POST\"",
		"browser presence form",
	} {
		if !strings.Contains(recorder.Body.String(), want) {
			t.Fatalf("expected enabled presence form HTML to contain %q, got %q", want, recorder.Body.String())
		}
	}
}

func TestReplayWorkbenchHandlerRendersCollaborationSelectionWriteControlOnlyWithExplicitGate(t *testing.T) {
	primary := app.ReplayResult{RunID: "run-primary"}
	collaboration := app.ReplayWorkbenchCollaborationArtifact{
		Path:          "/workspace/replay/workbench-collaborations/collab-one.json",
		Status:        "dry-run",
		Mode:          "metadata-only",
		SessionPath:   "/workspace/replay/workbench-sessions/session-one.json",
		PrimaryRunID:  primary.RunID,
		SelectedRunID: primary.RunID,
		Participants:  []string{"alice", "bob"},
		CreatedBy:     "lead",
		CreatedAt:     "2026-06-01T06:50:00Z",
		Reason:        "browser selection form",
	}

	disabled := newReplayWorkbenchHandlerWithOptions(replayWorkbenchHandlerOptions{
		Collaborations: []app.ReplayWorkbenchCollaborationArtifact{collaboration},
	}, primary)
	recorder := httptest.NewRecorder()
	disabled.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected disabled selection form HTML status 200, got %d body=%q", recorder.Code, recorder.Body.String())
	}
	disabledBody := recorder.Body.String()
	if !strings.Contains(disabledBody, "window.OURO_REPLAY_COLLABORATION_SELECTION_WRITES_ENABLED = false") {
		t.Fatalf("expected disabled collaboration selection writes flag in HTML, got %q", disabledBody)
	}
	if strings.Contains(disabledBody, "ouro-replay-collaboration-selection-form") ||
		strings.Contains(disabledBody, "/api/collaboration-selection\", {\n          method: \"POST\"") {
		t.Fatalf("expected disabled selection write HTML to stay read-only, got %q", disabledBody)
	}
	if strings.Count(disabledBody, "let sessionSelection = window.OURO_REPLAY_SESSION_SELECTION || null;") != 1 {
		t.Fatalf("expected replay workbench HTML to declare sessionSelection once, got %q", disabledBody)
	}

	enabled := newReplayWorkbenchHandlerWithOptions(replayWorkbenchHandlerOptions{
		Collaborations:                    []app.ReplayWorkbenchCollaborationArtifact{collaboration},
		AllowCollaborationSelectionWrites: true,
	}, primary)
	recorder = httptest.NewRecorder()
	enabled.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected enabled selection form HTML status 200, got %d body=%q", recorder.Code, recorder.Body.String())
	}
	enabledBody := recorder.Body.String()
	for _, want := range []string{
		"window.OURO_REPLAY_COLLABORATION_SELECTION_WRITES_ENABLED = true",
		"id=\"ouro-replay-collaboration-selection-form\"",
		"id=\"ouro-replay-collaboration-selection-collaboration\"",
		"id=\"ouro-replay-collaboration-selection-actor\"",
		"id=\"ouro-replay-collaboration-selection-reason\"",
		"id=\"ouro-replay-collaboration-selection-select\"",
		"id=\"ouro-replay-collaboration-selection-clear\"",
		"id=\"ouro-replay-collaboration-selection-write-status\"",
		"fetch(\"/api/collaboration-selection\", {",
		"method: \"POST\"",
		"renderReplayWorkbenchCollaborationSelection();",
		"browser selection form",
	} {
		if !strings.Contains(enabledBody, want) {
			t.Fatalf("expected enabled selection form HTML to contain %q, got %q", want, enabledBody)
		}
	}
	if strings.Count(enabledBody, "let sessionSelection = window.OURO_REPLAY_SESSION_SELECTION || null;") != 1 {
		t.Fatalf("expected enabled replay workbench HTML to declare sessionSelection once, got %q", enabledBody)
	}
}

func TestReplayWorkbenchHandlerRendersCollaborationNoteWriteControlOnlyWithExplicitGate(t *testing.T) {
	primary := app.ReplayResult{RunID: "run-primary"}
	collaboration := app.ReplayWorkbenchCollaborationArtifact{
		Path:          "/workspace/replay/workbench-collaborations/collab-one.json",
		Status:        "dry-run",
		Mode:          "metadata-only",
		SessionPath:   "/workspace/replay/workbench-sessions/session-one.json",
		PrimaryRunID:  primary.RunID,
		SelectedRunID: primary.RunID,
		Participants:  []string{"alice", "bob"},
		CreatedBy:     "lead",
		CreatedAt:     "2026-06-01T08:50:00Z",
		Reason:        "browser note form",
	}

	disabled := newReplayWorkbenchHandlerWithOptions(replayWorkbenchHandlerOptions{
		Collaborations: []app.ReplayWorkbenchCollaborationArtifact{collaboration},
	}, primary)
	recorder := httptest.NewRecorder()
	disabled.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected disabled note form HTML status 200, got %d body=%q", recorder.Code, recorder.Body.String())
	}
	disabledBody := recorder.Body.String()
	if !strings.Contains(disabledBody, "window.OURO_REPLAY_COLLABORATION_NOTE_WRITES_ENABLED = false") {
		t.Fatalf("expected disabled collaboration note writes flag in HTML, got %q", disabledBody)
	}
	if strings.Contains(disabledBody, "ouro-replay-collaboration-note-form") ||
		strings.Contains(disabledBody, "/api/collaboration-notes\", {\n          method: \"POST\"") {
		t.Fatalf("expected disabled note write HTML to stay read-only, got %q", disabledBody)
	}

	enabled := newReplayWorkbenchHandlerWithOptions(replayWorkbenchHandlerOptions{
		Collaborations:               []app.ReplayWorkbenchCollaborationArtifact{collaboration},
		AllowCollaborationNoteWrites: true,
	}, primary)
	recorder = httptest.NewRecorder()
	enabled.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected enabled note form HTML status 200, got %d body=%q", recorder.Code, recorder.Body.String())
	}
	enabledBody := recorder.Body.String()
	for _, want := range []string{
		"window.OURO_REPLAY_COLLABORATION_NOTE_WRITES_ENABLED = true",
		"id=\"ouro-replay-collaboration-note-form\"",
		"id=\"ouro-replay-note-collaboration\"",
		"id=\"ouro-replay-note-author\"",
		"id=\"ouro-replay-note-target-kind\"",
		"id=\"ouro-replay-note-target-id\"",
		"id=\"ouro-replay-note-body\"",
		"id=\"ouro-replay-note-reason\"",
		"id=\"ouro-replay-note-write-status\"",
		"fetch(\"/api/collaboration-notes\", {",
		"method: \"POST\"",
		"renderReplayWorkbenchCollaborationNotes();",
		"browser note form",
	} {
		if !strings.Contains(enabledBody, want) {
			t.Fatalf("expected enabled note form HTML to contain %q, got %q", want, enabledBody)
		}
	}
}

func TestReplayWorkbenchHandlerRendersCollaborationNoteSelectionWriteControlOnlyWithExplicitGate(t *testing.T) {
	primary := app.ReplayResult{RunID: "run-primary"}
	note := app.ReplayWorkbenchCollaborationNote{
		Path:              "/workspace/replay/workbench-collaboration-notes/note-one.json",
		Status:            "saved",
		Mode:              "metadata-only",
		CollaborationPath: "/workspace/replay/workbench-collaborations/collab-one.json",
		SessionPath:       "/workspace/replay/workbench-sessions/session-one.json",
		PrimaryRunID:      primary.RunID,
		SelectedRunID:     primary.RunID,
		Author:            "alice",
		Body:              "Review the current graph node.",
		TargetKind:        "graph-node",
		TargetID:          "fact-target",
		CreatedAt:         "2026-06-01T10:55:00Z",
		Reason:            "browser note selection form",
	}

	disabled := newReplayWorkbenchHandlerWithOptions(replayWorkbenchHandlerOptions{
		CollaborationNotes: []app.ReplayWorkbenchCollaborationNote{note},
	}, primary)
	recorder := httptest.NewRecorder()
	disabled.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected disabled note selection form HTML status 200, got %d body=%q", recorder.Code, recorder.Body.String())
	}
	disabledBody := recorder.Body.String()
	if !strings.Contains(disabledBody, "window.OURO_REPLAY_COLLABORATION_NOTE_SELECTION_WRITES_ENABLED = false") {
		t.Fatalf("expected disabled collaboration note selection writes flag in HTML, got %q", disabledBody)
	}
	if strings.Contains(disabledBody, "ouro-replay-collaboration-note-selection-form") ||
		strings.Contains(disabledBody, "/api/collaboration-note-selection\", {\n          method: \"POST\"") {
		t.Fatalf("expected disabled note selection write HTML to stay read-only, got %q", disabledBody)
	}

	enabled := newReplayWorkbenchHandlerWithOptions(replayWorkbenchHandlerOptions{
		CollaborationNotes:                    []app.ReplayWorkbenchCollaborationNote{note},
		AllowCollaborationNoteSelectionWrites: true,
	}, primary)
	recorder = httptest.NewRecorder()
	enabled.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected enabled note selection form HTML status 200, got %d body=%q", recorder.Code, recorder.Body.String())
	}
	enabledBody := recorder.Body.String()
	for _, want := range []string{
		"window.OURO_REPLAY_COLLABORATION_NOTE_SELECTION_WRITES_ENABLED = true",
		"id=\"ouro-replay-collaboration-note-selection-form\"",
		"id=\"ouro-replay-note-selection-action\"",
		"id=\"ouro-replay-note-selection-note\"",
		"id=\"ouro-replay-note-selection-actor\"",
		"id=\"ouro-replay-note-selection-reason\"",
		"id=\"ouro-replay-note-selection-write-status\"",
		"fetch(\"/api/collaboration-note-selection\", {",
		"method: \"POST\"",
		"renderReplayWorkbenchCollaborationNoteSelection();",
		"browser note selection form",
	} {
		if !strings.Contains(enabledBody, want) {
			t.Fatalf("expected enabled note selection form HTML to contain %q, got %q", want, enabledBody)
		}
	}
}

func TestReplayWorkbenchHandlerRendersCollaborationNoteSelectionPollingOnlyWithExplicitInterval(t *testing.T) {
	primary := app.ReplayResult{RunID: "run-primary"}
	selection := app.ReplayWorkbenchCollaborationNoteSelection{
		Path:              "/workspace/replay/workbench-collaboration-note-selection.json",
		Status:            "current",
		Mode:              "metadata-only",
		NotePath:          "/workspace/replay/workbench-collaboration-notes/note-one.json",
		CollaborationPath: "/workspace/replay/workbench-collaborations/collab-one.json",
		SessionPath:       "/workspace/replay/workbench-sessions/session-one.json",
		PrimaryRunID:      primary.RunID,
		SelectedRunID:     primary.RunID,
		Author:            "alice",
		TargetKind:        "graph-node",
		TargetID:          "fact-target",
		SelectedBy:        "lead",
		SelectedAt:        "2026-06-01T11:20:00Z",
		Reason:            "note selection polling baseline",
	}

	disabled := newReplayWorkbenchHandlerWithOptions(replayWorkbenchHandlerOptions{
		CollaborationNoteSelection: &selection,
	}, primary)
	recorder := httptest.NewRecorder()
	disabled.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected disabled note selection polling HTML status 200, got %d body=%q", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "window.OURO_REPLAY_COLLABORATION_NOTE_SELECTION_POLL_INTERVAL_MS = 0") {
		t.Fatalf("expected disabled note selection polling interval in HTML, got %q", recorder.Body.String())
	}
	if strings.Contains(recorder.Body.String(), "setInterval(refreshReplayWorkbenchCollaborationNoteSelection") ||
		strings.Contains(recorder.Body.String(), "installReplayWorkbenchCollaborationNoteSelectionPoller") {
		t.Fatalf("expected default note selection polling HTML to avoid background polling, got %q", recorder.Body.String())
	}

	enabled := newReplayWorkbenchHandlerWithOptions(replayWorkbenchHandlerOptions{
		CollaborationNoteSelection:               &selection,
		CollaborationNoteSelectionPollIntervalMS: 1500,
	}, primary)
	recorder = httptest.NewRecorder()
	enabled.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected enabled note selection polling HTML status 200, got %d body=%q", recorder.Code, recorder.Body.String())
	}
	for _, want := range []string{
		"window.OURO_REPLAY_COLLABORATION_NOTE_SELECTION_POLL_INTERVAL_MS = 1500",
		"const collaborationNoteSelectionPollIntervalMS = 1500",
		"installReplayWorkbenchCollaborationNoteSelectionPoller",
		"refreshReplayWorkbenchCollaborationNoteSelection",
		"setInterval(refreshReplayWorkbenchCollaborationNoteSelection, collaborationNoteSelectionPollIntervalMS)",
		"fetch(\"/api/collaboration-note-selection\")",
		"renderReplayWorkbenchCollaborationNoteSelection();",
	} {
		if !strings.Contains(recorder.Body.String(), want) {
			t.Fatalf("expected enabled note selection polling HTML to contain %q, got %q", want, recorder.Body.String())
		}
	}
	if strings.Contains(recorder.Body.String(), "method: \"POST\"") {
		t.Fatalf("note selection polling alone must not expose a POST path, got %q", recorder.Body.String())
	}
}

func TestReplayWorkbenchHandlerRendersCollaborationSelectionPollingOnlyWithExplicitInterval(t *testing.T) {
	primary := app.ReplayResult{RunID: "run-primary"}
	selection := app.ReplayWorkbenchCollaborationSelection{
		Path:              "/workspace/replay/workbench-collaboration-selection.json",
		Status:            "current",
		Mode:              "metadata-only",
		CollaborationPath: "/workspace/replay/workbench-collaborations/collab-one.json",
		SessionPath:       "/workspace/replay/workbench-sessions/session-one.json",
		PrimaryRunID:      primary.RunID,
		SelectedRunID:     primary.RunID,
		Participants:      2,
		SelectedBy:        "lead",
		SelectedAt:        "2026-06-01T07:15:00Z",
		Reason:            "selection polling baseline",
	}

	disabled := newReplayWorkbenchHandlerWithOptions(replayWorkbenchHandlerOptions{
		CollaborationSelection: &selection,
	}, primary)
	recorder := httptest.NewRecorder()
	disabled.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected disabled selection polling HTML status 200, got %d body=%q", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "window.OURO_REPLAY_COLLABORATION_SELECTION_POLL_INTERVAL_MS = 0") {
		t.Fatalf("expected disabled selection polling interval in HTML, got %q", recorder.Body.String())
	}
	if strings.Contains(recorder.Body.String(), "setInterval(refreshReplayWorkbenchCollaborationSelection") ||
		strings.Contains(recorder.Body.String(), "installReplayWorkbenchCollaborationSelectionPoller") {
		t.Fatalf("expected default selection polling HTML to avoid background polling, got %q", recorder.Body.String())
	}

	enabled := newReplayWorkbenchHandlerWithOptions(replayWorkbenchHandlerOptions{
		CollaborationSelection:               &selection,
		CollaborationSelectionPollIntervalMS: 1500,
	}, primary)
	recorder = httptest.NewRecorder()
	enabled.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected enabled selection polling HTML status 200, got %d body=%q", recorder.Code, recorder.Body.String())
	}
	for _, want := range []string{
		"window.OURO_REPLAY_COLLABORATION_SELECTION_POLL_INTERVAL_MS = 1500",
		"const collaborationSelectionPollIntervalMS = 1500",
		"installReplayWorkbenchCollaborationSelectionPoller",
		"refreshReplayWorkbenchCollaborationSelection",
		"setInterval(refreshReplayWorkbenchCollaborationSelection, collaborationSelectionPollIntervalMS)",
		"fetch(\"/api/collaboration-selection\")",
		"renderReplayWorkbenchCollaborationSelection();",
	} {
		if !strings.Contains(recorder.Body.String(), want) {
			t.Fatalf("expected enabled selection polling HTML to contain %q, got %q", want, recorder.Body.String())
		}
	}
	if strings.Contains(recorder.Body.String(), "method: \"POST\"") {
		t.Fatalf("selection polling alone must not expose a POST path, got %q", recorder.Body.String())
	}
}

func TestReplayWorkbenchHandlerServesCollaborationEventsOnlyWithExplicitGate(t *testing.T) {
	primary := app.ReplayResult{RunID: "run-primary"}
	collaboration := app.ReplayWorkbenchCollaborationArtifact{
		Path:          "/workspace/replay/workbench-collaborations/collab-one.json",
		Status:        "saved",
		Mode:          "metadata-only",
		SessionPath:   "/workspace/replay/workbench-sessions/session-one.json",
		PrimaryRunID:  primary.RunID,
		SelectedRunID: primary.RunID,
		Participants:  []string{"alice", "bob"},
		CreatedBy:     "lead",
		CreatedAt:     "2026-06-01T07:30:00Z",
		Reason:        "stream baseline",
	}
	selection := app.ReplayWorkbenchCollaborationSelection{
		Path:              "/workspace/replay/workbench-collaboration-selection.json",
		Status:            "current",
		Mode:              "metadata-only",
		CollaborationPath: collaboration.Path,
		SessionPath:       collaboration.SessionPath,
		PrimaryRunID:      primary.RunID,
		SelectedRunID:     primary.RunID,
		Participants:      2,
		SelectedBy:        "lead",
		SelectedAt:        "2026-06-01T07:31:00Z",
		Reason:            "stream current focus",
	}
	presence := app.ReplayWorkbenchCollaborationPresence{
		Path:              "/workspace/replay/workbench-collaboration-presence/alice.json",
		Status:            "online",
		Mode:              "metadata-only",
		CollaborationPath: collaboration.Path,
		SessionPath:       collaboration.SessionPath,
		PrimaryRunID:      primary.RunID,
		SelectedRunID:     primary.RunID,
		Participant:       "alice",
		SeenBy:            "lead",
		SeenAt:            "2026-06-01T07:32:00Z",
		Reason:            "streaming",
	}

	disabled := newReplayWorkbenchHandlerWithOptions(replayWorkbenchHandlerOptions{
		Collaborations:         []app.ReplayWorkbenchCollaborationArtifact{collaboration},
		CollaborationSelection: &selection,
		CollaborationPresence:  []app.ReplayWorkbenchCollaborationPresence{presence},
	}, primary)
	recorder := httptest.NewRecorder()
	disabled.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/collaboration-events", nil))
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("expected disabled collaboration events endpoint to be absent, got %d body=%q", recorder.Code, recorder.Body.String())
	}

	enabled := newReplayWorkbenchHandlerWithOptions(replayWorkbenchHandlerOptions{
		Collaborations:            []app.ReplayWorkbenchCollaborationArtifact{collaboration},
		CollaborationSelection:    &selection,
		CollaborationPresence:     []app.ReplayWorkbenchCollaborationPresence{presence},
		EnableCollaborationStream: true,
	}, primary)
	server := httptest.NewServer(enabled)
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/api/collaboration-events", nil)
	if err != nil {
		t.Fatalf("create collaboration events request: %v", err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("get collaboration events stream: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("expected collaboration events status 200, got %d", response.StatusCode)
	}
	if got := response.Header.Get("Content-Type"); !strings.Contains(got, "text/event-stream") {
		t.Fatalf("expected text/event-stream content type, got %q", got)
	}
	eventName, payload := readFirstSSEEvent(t, response.Body)
	if eventName != "collaboration-snapshot" {
		t.Fatalf("expected collaboration-snapshot event, got %q payload=%q", eventName, payload)
	}
	var snapshot replayWorkbenchCollaborationEventSnapshot
	if err := json.Unmarshal([]byte(payload), &snapshot); err != nil {
		t.Fatalf("decode collaboration snapshot: %v payload=%q", err, payload)
	}
	if len(snapshot.Collaborations) != 1 || snapshot.Collaborations[0].Path != collaboration.Path {
		t.Fatalf("unexpected stream collaborations: %#v", snapshot.Collaborations)
	}
	if snapshot.CollaborationSelection == nil ||
		snapshot.CollaborationSelection.CollaborationPath != collaboration.Path ||
		snapshot.CollaborationSelection.SelectedBy != "lead" {
		t.Fatalf("unexpected stream selection: %#v", snapshot.CollaborationSelection)
	}
	if len(snapshot.CollaborationPresence) != 1 ||
		snapshot.CollaborationPresence[0].Participant != "alice" ||
		snapshot.CollaborationPresence[0].Status != "online" {
		t.Fatalf("unexpected stream presence: %#v", snapshot.CollaborationPresence)
	}
}

func TestReplayWorkbenchHandlerStreamsCollaborationEventsAfterGatedPresenceWrites(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := app.InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	run, err := app.RunWorkspace(context.Background(), root)
	if err != nil {
		t.Fatalf("run workspace: %v", err)
	}
	replay, err := app.ReplayWorkspace(context.Background(), root, run.RunID)
	if err != nil {
		t.Fatalf("replay workspace: %v", err)
	}
	session, err := app.CreateReplayWorkbenchSession(context.Background(), root, app.ReplayWorkbenchSessionOptions{
		PrimaryRunID:  run.RunID,
		SelectedRunID: run.RunID,
		CreatedBy:     "lead",
		Reason:        "stream write session",
	})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	collaboration, err := app.CreateReplayWorkbenchCollaboration(context.Background(), root, app.ReplayWorkbenchCollaborationOptions{
		SessionPath:  filepath.Base(session.Path),
		Participants: []string{"alice"},
		CreatedBy:    "lead",
		Reason:       "stream write collaboration",
	})
	if err != nil {
		t.Fatalf("create collaboration: %v", err)
	}
	handler := newReplayWorkbenchHandlerWithOptions(replayWorkbenchHandlerOptions{
		WorkspaceRoot:                    root,
		Collaborations:                   []app.ReplayWorkbenchCollaborationArtifact{collaboration},
		AllowCollaborationPresenceWrites: true,
		EnableCollaborationStream:        true,
	}, replay)
	server := httptest.NewServer(handler)
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/api/collaboration-events", nil)
	if err != nil {
		t.Fatalf("create stream request: %v", err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("open collaboration stream: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("expected stream status 200, got %d", response.StatusCode)
	}
	if eventName, _ := readFirstSSEEvent(t, response.Body); eventName != "collaboration-snapshot" {
		t.Fatalf("expected initial collaboration snapshot event, got %q", eventName)
	}

	writeBody := strings.NewReader(fmt.Sprintf(`{
		"collaboration_path": %q,
		"participant": "alice",
		"status": "away",
		"seen_by": "lead",
		"seen_at": "2026-06-01T07:40:00Z",
		"reason": "streamed presence write"
	}`, filepath.Base(collaboration.Path)))
	writeResponse, err := http.Post(server.URL+"/api/collaboration-presence", "application/json", writeBody)
	if err != nil {
		t.Fatalf("write collaboration presence: %v", err)
	}
	_ = writeResponse.Body.Close()
	if writeResponse.StatusCode != http.StatusOK {
		t.Fatalf("expected presence write status 200, got %d", writeResponse.StatusCode)
	}

	eventName, payload := readFirstSSEEvent(t, response.Body)
	if eventName != "collaboration-snapshot" {
		t.Fatalf("expected collaboration snapshot after write, got %q payload=%q", eventName, payload)
	}
	var snapshot replayWorkbenchCollaborationEventSnapshot
	if err := json.Unmarshal([]byte(payload), &snapshot); err != nil {
		t.Fatalf("decode streamed snapshot after write: %v payload=%q", err, payload)
	}
	if len(snapshot.CollaborationPresence) != 1 ||
		snapshot.CollaborationPresence[0].Participant != "alice" ||
		snapshot.CollaborationPresence[0].Status != "away" ||
		snapshot.CollaborationPresence[0].Reason != "streamed presence write" {
		t.Fatalf("unexpected streamed presence after write: %#v", snapshot.CollaborationPresence)
	}
}

func TestReplayWorkbenchHandlerStreamsWorkspaceRefreshWithExplicitInterval(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := app.InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	run, err := app.RunWorkspace(context.Background(), root)
	if err != nil {
		t.Fatalf("run workspace: %v", err)
	}
	replay, err := app.ReplayWorkspace(context.Background(), root, run.RunID)
	if err != nil {
		t.Fatalf("replay workspace: %v", err)
	}
	session, err := app.CreateReplayWorkbenchSession(context.Background(), root, app.ReplayWorkbenchSessionOptions{
		PrimaryRunID:  run.RunID,
		SelectedRunID: run.RunID,
		CreatedBy:     "lead",
		Reason:        "stream refresh session",
	})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	collaboration, err := app.CreateReplayWorkbenchCollaboration(context.Background(), root, app.ReplayWorkbenchCollaborationOptions{
		SessionPath:  filepath.Base(session.Path),
		Participants: []string{"alice"},
		CreatedBy:    "lead",
		Reason:       "stream refresh collaboration",
	})
	if err != nil {
		t.Fatalf("create collaboration: %v", err)
	}
	handler := newReplayWorkbenchHandlerWithOptions(replayWorkbenchHandlerOptions{
		WorkspaceRoot:                        root,
		Collaborations:                       []app.ReplayWorkbenchCollaborationArtifact{collaboration},
		EnableCollaborationStream:            true,
		CollaborationStreamRefreshIntervalMS: 25,
	}, replay)
	server := httptest.NewServer(handler)
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/api/collaboration-events", nil)
	if err != nil {
		t.Fatalf("create stream request: %v", err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("open collaboration stream: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("expected stream status 200, got %d", response.StatusCode)
	}
	if eventName, payload := readFirstSSEEvent(t, response.Body); eventName != "collaboration-snapshot" || !strings.Contains(payload, "stream refresh collaboration") {
		t.Fatalf("expected initial collaboration snapshot, got event=%q payload=%q", eventName, payload)
	}

	if _, err := app.UpdateReplayWorkbenchCollaborationPresence(context.Background(), root, app.ReplayWorkbenchCollaborationPresenceOptions{
		CollaborationPath: filepath.Base(collaboration.Path),
		Participant:       "alice",
		Status:            "away",
		SeenBy:            "external-cli",
		SeenAt:            time.Date(2026, 6, 1, 7, 50, 0, 0, time.UTC),
		Reason:            "external workspace refresh",
	}); err != nil {
		t.Fatalf("write external presence: %v", err)
	}

	var snapshot replayWorkbenchCollaborationEventSnapshot
	foundRefresh := false
	for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline); {
		eventName, payload := readFirstSSEEvent(t, response.Body)
		if eventName != "collaboration-snapshot" {
			t.Fatalf("expected refreshed collaboration snapshot, got %q payload=%q", eventName, payload)
		}
		snapshot = replayWorkbenchCollaborationEventSnapshot{}
		if err := json.Unmarshal([]byte(payload), &snapshot); err != nil {
			t.Fatalf("decode refreshed stream snapshot: %v payload=%q", err, payload)
		}
		if len(snapshot.CollaborationPresence) == 1 &&
			snapshot.CollaborationPresence[0].Participant == "alice" &&
			snapshot.CollaborationPresence[0].Status == "away" &&
			snapshot.CollaborationPresence[0].SeenBy == "external-cli" &&
			snapshot.CollaborationPresence[0].Reason == "external workspace refresh" {
			foundRefresh = true
			break
		}
	}
	if !foundRefresh {
		t.Fatalf("expected refreshed stream presence from workspace, got last snapshot: %#v", snapshot.CollaborationPresence)
	}
}

func TestReplayWorkbenchHandlerRendersCollaborationStreamOnlyWithExplicitGate(t *testing.T) {
	primary := app.ReplayResult{RunID: "run-primary"}
	disabled := newReplayWorkbenchHandlerWithOptions(replayWorkbenchHandlerOptions{}, primary)
	recorder := httptest.NewRecorder()
	disabled.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected disabled collaboration stream HTML status 200, got %d body=%q", recorder.Code, recorder.Body.String())
	}
	disabledBody := recorder.Body.String()
	if !strings.Contains(disabledBody, "window.OURO_REPLAY_COLLABORATION_STREAM_ENABLED = false") {
		t.Fatalf("expected disabled collaboration stream flag in HTML, got %q", disabledBody)
	}
	if strings.Contains(disabledBody, "new EventSource(\"/api/collaboration-events\")") ||
		strings.Contains(disabledBody, "installReplayWorkbenchCollaborationStream") {
		t.Fatalf("expected default collaboration stream HTML to avoid EventSource, got %q", disabledBody)
	}

	enabled := newReplayWorkbenchHandlerWithOptions(replayWorkbenchHandlerOptions{
		EnableCollaborationStream: true,
	}, primary)
	recorder = httptest.NewRecorder()
	enabled.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected enabled collaboration stream HTML status 200, got %d body=%q", recorder.Code, recorder.Body.String())
	}
	enabledBody := recorder.Body.String()
	for _, want := range []string{
		"window.OURO_REPLAY_COLLABORATION_STREAM_ENABLED = true",
		"installReplayWorkbenchCollaborationStream",
		"new EventSource(\"/api/collaboration-events\")",
		"addEventListener(\"collaboration-snapshot\"",
		"renderReplayWorkbenchCollaborations();",
		"renderReplayWorkbenchCollaborationSelection();",
		"renderReplayWorkbenchCollaborationPresence();",
	} {
		if !strings.Contains(enabledBody, want) {
			t.Fatalf("expected enabled collaboration stream HTML to contain %q, got %q", want, enabledBody)
		}
	}
	if strings.Contains(enabledBody, "method: \"POST\"") {
		t.Fatalf("collaboration stream alone must not expose a POST path, got %q", enabledBody)
	}
}

func TestReplayWorkbenchHandlerRendersCollaborationPresencePollingOnlyWithExplicitInterval(t *testing.T) {
	primary := app.ReplayResult{RunID: "run-primary"}
	presence := app.ReplayWorkbenchCollaborationPresence{
		Path:              "/workspace/replay/workbench-collaboration-presence/alice.json",
		Status:            "online",
		Mode:              "metadata-only",
		CollaborationPath: "/workspace/replay/workbench-collaborations/collab-one.json",
		SessionPath:       "/workspace/replay/workbench-sessions/session-one.json",
		PrimaryRunID:      primary.RunID,
		SelectedRunID:     primary.RunID,
		Participant:       "alice",
		SeenBy:            "lead",
		SeenAt:            "2026-06-01T06:40:00Z",
		Reason:            "polling baseline",
	}

	disabled := newReplayWorkbenchHandlerWithOptions(replayWorkbenchHandlerOptions{
		CollaborationPresence: []app.ReplayWorkbenchCollaborationPresence{presence},
	}, primary)
	recorder := httptest.NewRecorder()
	disabled.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected disabled presence polling HTML status 200, got %d body=%q", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "window.OURO_REPLAY_COLLABORATION_PRESENCE_POLL_INTERVAL_MS = 0") {
		t.Fatalf("expected disabled presence polling interval in HTML, got %q", recorder.Body.String())
	}
	if strings.Contains(recorder.Body.String(), "setInterval(refreshReplayWorkbenchCollaborationPresence") ||
		strings.Contains(recorder.Body.String(), "installReplayWorkbenchCollaborationPresencePoller") {
		t.Fatalf("expected default presence polling HTML to avoid background polling, got %q", recorder.Body.String())
	}

	enabled := newReplayWorkbenchHandlerWithOptions(replayWorkbenchHandlerOptions{
		CollaborationPresence:               []app.ReplayWorkbenchCollaborationPresence{presence},
		CollaborationPresencePollIntervalMS: 1500,
	}, primary)
	recorder = httptest.NewRecorder()
	enabled.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected enabled presence polling HTML status 200, got %d body=%q", recorder.Code, recorder.Body.String())
	}
	for _, want := range []string{
		"window.OURO_REPLAY_COLLABORATION_PRESENCE_POLL_INTERVAL_MS = 1500",
		"const collaborationPresencePollIntervalMS = 1500",
		"installReplayWorkbenchCollaborationPresencePoller",
		"refreshReplayWorkbenchCollaborationPresence",
		"setInterval(refreshReplayWorkbenchCollaborationPresence, collaborationPresencePollIntervalMS)",
		"fetch(\"/api/collaboration-presence\")",
	} {
		if !strings.Contains(recorder.Body.String(), want) {
			t.Fatalf("expected enabled presence polling HTML to contain %q, got %q", want, recorder.Body.String())
		}
	}
	if strings.Contains(recorder.Body.String(), "method: \"POST\"") {
		t.Fatalf("presence polling alone must not expose a POST path, got %q", recorder.Body.String())
	}
}

func TestReplayWorkbenchHandlerRendersCollaborationNotesPollingOnlyWithExplicitInterval(t *testing.T) {
	primary := app.ReplayResult{RunID: "run-primary"}
	note := app.ReplayWorkbenchCollaborationNote{
		Path:              "/workspace/replay/workbench-collaboration-notes/note-one.json",
		Status:            "saved",
		Mode:              "metadata-only",
		CollaborationPath: "/workspace/replay/workbench-collaborations/collab-one.json",
		SessionPath:       "/workspace/replay/workbench-sessions/session-one.json",
		PrimaryRunID:      primary.RunID,
		SelectedRunID:     primary.RunID,
		Author:            "alice",
		Body:              "Check this graph edge.",
		TargetKind:        "graph-edge",
		TargetID:          "edge-one",
		CreatedAt:         "2026-06-01T09:15:00Z",
		Reason:            "notes polling baseline",
	}

	disabled := newReplayWorkbenchHandlerWithOptions(replayWorkbenchHandlerOptions{
		CollaborationNotes: []app.ReplayWorkbenchCollaborationNote{note},
	}, primary)
	recorder := httptest.NewRecorder()
	disabled.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected disabled notes polling HTML status 200, got %d body=%q", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "window.OURO_REPLAY_COLLABORATION_NOTES_POLL_INTERVAL_MS = 0") {
		t.Fatalf("expected disabled notes polling interval in HTML, got %q", recorder.Body.String())
	}
	if strings.Contains(recorder.Body.String(), "setInterval(refreshReplayWorkbenchCollaborationNotes") ||
		strings.Contains(recorder.Body.String(), "installReplayWorkbenchCollaborationNotesPoller") {
		t.Fatalf("expected default notes polling HTML to avoid background polling, got %q", recorder.Body.String())
	}

	enabled := newReplayWorkbenchHandlerWithOptions(replayWorkbenchHandlerOptions{
		CollaborationNotes:               []app.ReplayWorkbenchCollaborationNote{note},
		CollaborationNotesPollIntervalMS: 1500,
	}, primary)
	recorder = httptest.NewRecorder()
	enabled.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected enabled notes polling HTML status 200, got %d body=%q", recorder.Code, recorder.Body.String())
	}
	for _, want := range []string{
		"window.OURO_REPLAY_COLLABORATION_NOTES_POLL_INTERVAL_MS = 1500",
		"const collaborationNotesPollIntervalMS = 1500",
		"installReplayWorkbenchCollaborationNotesPoller",
		"refreshReplayWorkbenchCollaborationNotes",
		"setInterval(refreshReplayWorkbenchCollaborationNotes, collaborationNotesPollIntervalMS)",
		"fetch(\"/api/collaboration-notes\")",
		"renderReplayWorkbenchCollaborationNotes();",
	} {
		if !strings.Contains(recorder.Body.String(), want) {
			t.Fatalf("expected enabled notes polling HTML to contain %q, got %q", want, recorder.Body.String())
		}
	}
	if strings.Contains(recorder.Body.String(), "method: \"POST\"") {
		t.Fatalf("notes polling alone must not expose a POST path, got %q", recorder.Body.String())
	}
}

func TestReplayWorkbenchCommandEnablesSessionSelectionWrites(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := app.InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	primary, err := app.RunWorkspace(context.Background(), root)
	if err != nil {
		t.Fatalf("run primary workspace: %v", err)
	}
	comparison, err := app.RunWorkspace(context.Background(), root)
	if err != nil {
		t.Fatalf("run comparison workspace: %v", err)
	}
	session, err := app.CreateReplayWorkbenchSession(context.Background(), root, app.ReplayWorkbenchSessionOptions{
		PrimaryRunID:  primary.RunID,
		CompareRunIDs: []string{comparison.RunID},
		SelectedRunID: comparison.RunID,
		Filters: map[string]string{
			"q": "browser-command-session",
		},
		CreatedBy: "operator",
		Reason:    "browser command session selection",
	})
	if err != nil {
		t.Fatalf("create replay workbench session: %v", err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("allocate listen address: %v", err)
	}
	addr := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatalf("close temporary listener: %v", err)
	}

	var out bytes.Buffer
	cmd := newRootCommand(&out, new(bytes.Buffer))
	cmd.SetArgs([]string{
		"replay-workbench", root, primary.RunID,
		"--listen", addr,
		"--max-requests", "2",
		"--enable-session-selection-writes",
	})
	done := make(chan error, 1)
	go func() {
		done <- cmd.Execute()
	}()

	client := http.Client{Timeout: 250 * time.Millisecond}
	body := fmt.Sprintf(`{"action":"select","session_path":%q,"actor":"browser","at":"2026-06-01T12:20:00Z","reason":"browser command selected session"}`, filepath.Base(session.Path))
	var selected app.ReplayWorkbenchSessionSelection
	var lastErr error
	for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		response, err := client.Post("http://"+addr+"/api/session-selection", "application/json", strings.NewReader(body))
		if err != nil {
			lastErr = err
			continue
		}
		if response.StatusCode != http.StatusOK {
			payload, _ := io.ReadAll(response.Body)
			_ = response.Body.Close()
			lastErr = fmt.Errorf("unexpected session selection POST status %d body=%q", response.StatusCode, string(payload))
			continue
		}
		if err := json.NewDecoder(response.Body).Decode(&selected); err != nil {
			_ = response.Body.Close()
			lastErr = err
			continue
		}
		_ = response.Body.Close()
		lastErr = nil
		break
	}
	if lastErr != nil {
		t.Fatalf("post browser session selection through command gate: %v", lastErr)
	}
	if selected.Status != "current" || selected.SessionPath != session.Path || selected.SelectedBy != "browser" {
		t.Fatalf("unexpected selected session marker: %#v", selected)
	}
	response, err := client.Get("http://" + addr + "/api/session-selection")
	if err != nil {
		t.Fatalf("get session selection after command write: %v", err)
	}
	var listed app.ReplayWorkbenchSessionSelection
	if err := json.NewDecoder(response.Body).Decode(&listed); err != nil {
		_ = response.Body.Close()
		t.Fatalf("decode listed session selection after command write: %v", err)
	}
	_ = response.Body.Close()
	if listed.SessionPath != session.Path || listed.Status != "current" {
		t.Fatalf("unexpected listed session marker: %#v", listed)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("serve replay workbench with session selection writes: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("replay workbench with session selection writes did not exit after max requests")
	}
	if _, found, err := app.ReadReplayWorkbenchSessionSelection(root); err != nil {
		t.Fatalf("read written session selection marker: %v", err)
	} else if !found {
		t.Fatalf("expected browser command session selection marker to be written")
	}
}

func TestReplayWorkbenchCommandEnablesReportSelectionWrites(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	execution := createReportProjectionDispatchExecutionForCLI(t, root)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("allocate listen address: %v", err)
	}
	addr := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatalf("close temporary listener: %v", err)
	}

	var out bytes.Buffer
	cmd := newRootCommand(&out, new(bytes.Buffer))
	cmd.SetArgs([]string{
		"replay-workbench", root, execution.RunID,
		"--listen", addr,
		"--max-requests", "2",
		"--enable-report-selection-writes",
	})
	done := make(chan error, 1)
	go func() {
		done <- cmd.Execute()
	}()

	client := http.Client{Timeout: 250 * time.Millisecond}
	body := fmt.Sprintf(`{"action":"select","execution_path":%q,"actor":"browser","at":"2026-06-01T11:20:00Z","reason":"browser command selected report"}`, filepath.Base(execution.Path))
	var selected app.ReportProjectionDispatchExecutionSelection
	var lastErr error
	for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		response, err := client.Post("http://"+addr+"/api/report-selection", "application/json", strings.NewReader(body))
		if err != nil {
			lastErr = err
			continue
		}
		if response.StatusCode != http.StatusOK {
			payload, _ := io.ReadAll(response.Body)
			_ = response.Body.Close()
			lastErr = fmt.Errorf("unexpected report selection POST status %d body=%q", response.StatusCode, string(payload))
			continue
		}
		if err := json.NewDecoder(response.Body).Decode(&selected); err != nil {
			_ = response.Body.Close()
			lastErr = err
			continue
		}
		_ = response.Body.Close()
		lastErr = nil
		break
	}
	if lastErr != nil {
		t.Fatalf("post browser report selection through command gate: %v", lastErr)
	}
	if selected.Status != "current" || selected.ExecutionPath != execution.Path || selected.SelectedBy != "browser" {
		t.Fatalf("unexpected selected report marker: %#v", selected)
	}
	response, err := client.Get("http://" + addr + "/api/report-selection")
	if err != nil {
		t.Fatalf("get report selection after command write: %v", err)
	}
	var listed app.ReportProjectionDispatchExecutionSelection
	if err := json.NewDecoder(response.Body).Decode(&listed); err != nil {
		_ = response.Body.Close()
		t.Fatalf("decode listed report selection after command write: %v", err)
	}
	_ = response.Body.Close()
	if listed.ExecutionPath != execution.Path || listed.Status != "current" {
		t.Fatalf("unexpected listed report marker: %#v", listed)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("serve replay workbench with report selection writes: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("replay workbench with report selection writes did not exit after max requests")
	}
	if _, found, err := app.ReadReportProjectionDispatchExecutionSelection(root); err != nil {
		t.Fatalf("read written report selection marker: %v", err)
	} else if !found {
		t.Fatalf("expected browser command report selection marker to be written")
	}
}

func TestReplayWorkbenchCommandEnablesSessionSelectionPolling(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := app.InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	run, err := app.RunWorkspace(context.Background(), root)
	if err != nil {
		t.Fatalf("run workspace: %v", err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("allocate listen address: %v", err)
	}
	addr := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatalf("close temporary listener: %v", err)
	}

	var out bytes.Buffer
	cmd := newRootCommand(&out, new(bytes.Buffer))
	cmd.SetArgs([]string{
		"replay-workbench", root, run.RunID,
		"--listen", addr,
		"--max-requests", "1",
		"--session-selection-poll-interval-ms", "1500",
	})
	done := make(chan error, 1)
	go func() {
		done <- cmd.Execute()
	}()

	client := http.Client{Timeout: 250 * time.Millisecond}
	var htmlBody string
	var lastErr error
	for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		response, err := client.Get("http://" + addr + "/")
		if err != nil {
			lastErr = err
			continue
		}
		var body bytes.Buffer
		if _, err := body.ReadFrom(response.Body); err != nil {
			lastErr = err
			_ = response.Body.Close()
			continue
		}
		_ = response.Body.Close()
		if response.StatusCode != http.StatusOK {
			lastErr = fmt.Errorf("unexpected replay workbench status %d body=%q", response.StatusCode, body.String())
			continue
		}
		htmlBody = body.String()
		lastErr = nil
		break
	}
	if lastErr != nil {
		t.Fatalf("get replay workbench HTML with session selection polling: %v", lastErr)
	}
	for _, want := range []string{
		"window.OURO_REPLAY_SESSION_SELECTION_POLL_INTERVAL_MS = 1500",
		"setInterval(refreshReplayWorkbenchSessionSelection, sessionSelectionPollIntervalMS)",
	} {
		if !strings.Contains(htmlBody, want) {
			t.Fatalf("expected replay workbench HTML to contain %q, got %q", want, htmlBody)
		}
	}
	if strings.Contains(htmlBody, "method: \"POST\"") {
		t.Fatalf("session selection polling CLI flag must not expose POST without write flag, got %q", htmlBody)
	}

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("serve replay workbench with session selection polling: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for replay workbench session selection polling server to stop")
	}
}

func TestReplayWorkbenchCommandRejectsNegativeSessionSelectionPolling(t *testing.T) {
	var out bytes.Buffer
	errOut := new(bytes.Buffer)
	cmd := newRootCommand(&out, errOut)
	cmd.SetArgs([]string{
		"replay-workbench", t.TempDir(), "run-negative-session-selection-polling",
		"--session-selection-poll-interval-ms", "-1",
	})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "--session-selection-poll-interval-ms cannot be negative") {
		t.Fatalf("expected negative session selection polling interval rejection, got err=%v stdout=%q stderr=%q", err, out.String(), errOut.String())
	}
}

func TestReplayWorkbenchCommandEnablesCollaborationPresenceWrites(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := app.InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	run, err := app.RunWorkspace(context.Background(), root)
	if err != nil {
		t.Fatalf("run workspace: %v", err)
	}
	session, err := app.CreateReplayWorkbenchSession(context.Background(), root, app.ReplayWorkbenchSessionOptions{
		PrimaryRunID: run.RunID,
		CreatedBy:    "operator",
		Reason:       "presence command baseline",
	})
	if err != nil {
		t.Fatalf("create replay workbench session: %v", err)
	}
	collaboration, err := app.CreateReplayWorkbenchCollaboration(context.Background(), root, app.ReplayWorkbenchCollaborationOptions{
		SessionPath:  filepath.Base(session.Path),
		Participants: []string{"alice", "bob"},
		CreatedBy:    "lead",
		Reason:       "browser command gate",
	})
	if err != nil {
		t.Fatalf("create replay workbench collaboration: %v", err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("allocate listen address: %v", err)
	}
	addr := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatalf("close temporary listener: %v", err)
	}

	var out bytes.Buffer
	cmd := newRootCommand(&out, new(bytes.Buffer))
	cmd.SetArgs([]string{
		"replay-workbench", root, run.RunID,
		"--listen", addr,
		"--max-requests", "2",
		"--enable-collaboration-presence-writes",
	})
	done := make(chan error, 1)
	go func() {
		done <- cmd.Execute()
	}()

	client := http.Client{Timeout: 250 * time.Millisecond}
	body := fmt.Sprintf(`{"collaboration_path":%q,"participant":"alice","status":"away","seen_by":"browser","seen_at":"2026-06-01T06:20:00Z","reason":"browser command heartbeat"}`, filepath.Base(collaboration.Path))
	var written app.ReplayWorkbenchCollaborationPresence
	var lastErr error
	for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		response, err := client.Post("http://"+addr+"/api/collaboration-presence", "application/json", strings.NewReader(body))
		if err != nil {
			lastErr = err
			continue
		}
		if response.StatusCode != http.StatusOK {
			lastErr = fmt.Errorf("unexpected presence write status %d", response.StatusCode)
			_ = response.Body.Close()
			continue
		}
		if err := json.NewDecoder(response.Body).Decode(&written); err != nil {
			lastErr = err
			_ = response.Body.Close()
			continue
		}
		_ = response.Body.Close()
		lastErr = nil
		break
	}
	if lastErr != nil {
		t.Fatalf("post collaboration presence through replay workbench command: %v", lastErr)
	}
	if written.Status != "away" || written.Participant != "alice" || written.CollaborationPath != collaboration.Path {
		t.Fatalf("unexpected command-written collaboration presence: %#v", written)
	}

	response, err := client.Get("http://" + addr + "/api/collaboration-presence")
	if err != nil {
		t.Fatalf("get collaboration presence through replay workbench command: %v", err)
	}
	var listed []app.ReplayWorkbenchCollaborationPresence
	if err := json.NewDecoder(response.Body).Decode(&listed); err != nil {
		_ = response.Body.Close()
		t.Fatalf("decode collaboration presence command listing: %v", err)
	}
	_ = response.Body.Close()
	if len(listed) != 1 || listed[0].Path != written.Path || listed[0].Reason != "browser command heartbeat" {
		t.Fatalf("unexpected collaboration presence command listing: %#v", listed)
	}

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("serve replay workbench with collaboration presence writes: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("replay workbench with collaboration presence writes did not exit after max requests")
	}
	if !strings.Contains(out.String(), "replay-workbench: accepted=2") {
		t.Fatalf("expected replay workbench collaboration presence write output, got %q", out.String())
	}
}

func TestReplayWorkbenchCommandEnablesCollaborationSelectionWrites(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := app.InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	run, err := app.RunWorkspace(context.Background(), root)
	if err != nil {
		t.Fatalf("run workspace: %v", err)
	}
	session, err := app.CreateReplayWorkbenchSession(context.Background(), root, app.ReplayWorkbenchSessionOptions{
		PrimaryRunID: run.RunID,
		CreatedBy:    "operator",
		Reason:       "selection command baseline",
	})
	if err != nil {
		t.Fatalf("create replay workbench session: %v", err)
	}
	collaboration, err := app.CreateReplayWorkbenchCollaboration(context.Background(), root, app.ReplayWorkbenchCollaborationOptions{
		SessionPath:  filepath.Base(session.Path),
		Participants: []string{"alice", "bob"},
		CreatedBy:    "lead",
		Reason:       "browser selection command gate",
	})
	if err != nil {
		t.Fatalf("create replay workbench collaboration: %v", err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("allocate listen address: %v", err)
	}
	addr := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatalf("close temporary listener: %v", err)
	}

	var out bytes.Buffer
	cmd := newRootCommand(&out, new(bytes.Buffer))
	cmd.SetArgs([]string{
		"replay-workbench", root, run.RunID,
		"--listen", addr,
		"--max-requests", "2",
		"--enable-collaboration-selection-writes",
	})
	done := make(chan error, 1)
	go func() {
		done <- cmd.Execute()
	}()

	client := http.Client{Timeout: 250 * time.Millisecond}
	body := fmt.Sprintf(`{"action":"select","collaboration_path":%q,"actor":"browser","at":"2026-06-01T06:55:00Z","reason":"browser selected current collaboration"}`, filepath.Base(collaboration.Path))
	var selected app.ReplayWorkbenchCollaborationSelection
	var lastErr error
	for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		response, err := client.Post("http://"+addr+"/api/collaboration-selection", "application/json", strings.NewReader(body))
		if err != nil {
			lastErr = err
			continue
		}
		if response.StatusCode != http.StatusOK {
			lastErr = fmt.Errorf("unexpected collaboration selection write status %d", response.StatusCode)
			_ = response.Body.Close()
			continue
		}
		if err := json.NewDecoder(response.Body).Decode(&selected); err != nil {
			lastErr = err
			_ = response.Body.Close()
			continue
		}
		_ = response.Body.Close()
		lastErr = nil
		break
	}
	if lastErr != nil {
		t.Fatalf("post collaboration selection through replay workbench command: %v", lastErr)
	}
	if selected.Status != "current" || selected.CollaborationPath != collaboration.Path || selected.SelectedBy != "browser" {
		t.Fatalf("unexpected command-written collaboration selection: %#v", selected)
	}

	response, err := client.Get("http://" + addr + "/api/collaboration-selection")
	if err != nil {
		t.Fatalf("get collaboration selection through replay workbench command: %v", err)
	}
	var listed app.ReplayWorkbenchCollaborationSelection
	if err := json.NewDecoder(response.Body).Decode(&listed); err != nil {
		_ = response.Body.Close()
		t.Fatalf("decode collaboration selection command listing: %v", err)
	}
	_ = response.Body.Close()
	if listed.Status != "current" || listed.Path != selected.Path || listed.Reason != "browser selected current collaboration" {
		t.Fatalf("unexpected collaboration selection command listing: %#v", listed)
	}

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("serve replay workbench with collaboration selection writes: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("replay workbench with collaboration selection writes did not exit after max requests")
	}
	if !strings.Contains(out.String(), "replay-workbench: accepted=2") {
		t.Fatalf("expected replay workbench collaboration selection write output, got %q", out.String())
	}
}

func TestReplayWorkbenchCommandEnablesCollaborationNoteWrites(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := app.InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	run, err := app.RunWorkspace(context.Background(), root)
	if err != nil {
		t.Fatalf("run workspace: %v", err)
	}
	session, err := app.CreateReplayWorkbenchSession(context.Background(), root, app.ReplayWorkbenchSessionOptions{
		PrimaryRunID: run.RunID,
		CreatedBy:    "operator",
		Reason:       "note command gate baseline",
	})
	if err != nil {
		t.Fatalf("create replay workbench session: %v", err)
	}
	collaboration, err := app.CreateReplayWorkbenchCollaboration(context.Background(), root, app.ReplayWorkbenchCollaborationOptions{
		SessionPath:  filepath.Base(session.Path),
		Participants: []string{"alice", "bob"},
		CreatedBy:    "lead",
		Reason:       "browser note command gate",
	})
	if err != nil {
		t.Fatalf("create replay workbench collaboration: %v", err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("allocate listen address: %v", err)
	}
	addr := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatalf("close temporary listener: %v", err)
	}

	var out bytes.Buffer
	cmd := newRootCommand(&out, new(bytes.Buffer))
	cmd.SetArgs([]string{
		"replay-workbench", root, run.RunID,
		"--listen", addr,
		"--max-requests", "2",
		"--enable-collaboration-note-writes",
	})
	done := make(chan error, 1)
	go func() {
		done <- cmd.Execute()
	}()

	client := http.Client{Timeout: 250 * time.Millisecond}
	body := fmt.Sprintf(`{"collaboration_path":%q,"author":"browser","body":"Capture browser review note.","target_kind":"timeline-event","target_id":"event-1","created_at":"2026-06-01T08:55:00Z","reason":"browser command note"}`, filepath.Base(collaboration.Path))
	var written app.ReplayWorkbenchCollaborationNote
	var lastErr error
	for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		response, err := client.Post("http://"+addr+"/api/collaboration-notes", "application/json", strings.NewReader(body))
		if err != nil {
			lastErr = err
			continue
		}
		if response.StatusCode != http.StatusOK {
			lastErr = fmt.Errorf("unexpected collaboration note write status %d", response.StatusCode)
			_ = response.Body.Close()
			continue
		}
		if err := json.NewDecoder(response.Body).Decode(&written); err != nil {
			lastErr = err
			_ = response.Body.Close()
			continue
		}
		_ = response.Body.Close()
		lastErr = nil
		break
	}
	if lastErr != nil {
		t.Fatalf("post collaboration note through replay workbench command: %v", lastErr)
	}
	if written.Status != "saved" || written.Author != "browser" || written.Body != "Capture browser review note." || written.CollaborationPath != collaboration.Path {
		t.Fatalf("unexpected command-written collaboration note: %#v", written)
	}

	response, err := client.Get("http://" + addr + "/api/collaboration-notes")
	if err != nil {
		t.Fatalf("get collaboration notes through replay workbench command: %v", err)
	}
	var listed []app.ReplayWorkbenchCollaborationNote
	if err := json.NewDecoder(response.Body).Decode(&listed); err != nil {
		_ = response.Body.Close()
		t.Fatalf("decode collaboration note command listing: %v", err)
	}
	_ = response.Body.Close()
	if len(listed) != 1 || listed[0].Path != written.Path || listed[0].Reason != "browser command note" {
		t.Fatalf("unexpected collaboration note command listing: %#v", listed)
	}

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("serve replay workbench with collaboration note writes: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("replay workbench with collaboration note writes did not exit after max requests")
	}
	if !strings.Contains(out.String(), "replay-workbench: accepted=2") {
		t.Fatalf("expected replay workbench collaboration note write output, got %q", out.String())
	}
}

func TestReplayWorkbenchCommandEnablesCollaborationNoteSelectionWrites(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := app.InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	run, err := app.RunWorkspace(context.Background(), root)
	if err != nil {
		t.Fatalf("run workspace: %v", err)
	}
	session, err := app.CreateReplayWorkbenchSession(context.Background(), root, app.ReplayWorkbenchSessionOptions{
		PrimaryRunID: run.RunID,
		CreatedBy:    "operator",
		Reason:       "note selection command baseline",
	})
	if err != nil {
		t.Fatalf("create replay workbench session: %v", err)
	}
	collaboration, err := app.CreateReplayWorkbenchCollaboration(context.Background(), root, app.ReplayWorkbenchCollaborationOptions{
		SessionPath:  filepath.Base(session.Path),
		Participants: []string{"alice", "bob"},
		CreatedBy:    "lead",
		Reason:       "browser note selection command gate",
	})
	if err != nil {
		t.Fatalf("create replay workbench collaboration: %v", err)
	}
	note, err := app.CreateReplayWorkbenchCollaborationNote(context.Background(), root, app.ReplayWorkbenchCollaborationNoteOptions{
		CollaborationPath: filepath.Base(collaboration.Path),
		Author:            "alice",
		Body:              "Command should select this note.",
		TargetKind:        "timeline-event",
		TargetID:          "event-1",
		Reason:            "browser command note selection candidate",
	})
	if err != nil {
		t.Fatalf("create replay workbench collaboration note: %v", err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("allocate listen address: %v", err)
	}
	addr := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatalf("close temporary listener: %v", err)
	}

	var out bytes.Buffer
	cmd := newRootCommand(&out, new(bytes.Buffer))
	cmd.SetArgs([]string{
		"replay-workbench", root, run.RunID,
		"--listen", addr,
		"--max-requests", "2",
		"--enable-collaboration-note-selection-writes",
	})
	done := make(chan error, 1)
	go func() {
		done <- cmd.Execute()
	}()

	client := http.Client{Timeout: 250 * time.Millisecond}
	body := fmt.Sprintf(`{"action":"select","note_path":%q,"actor":"browser","at":"2026-06-01T10:58:00Z","reason":"browser command selected note"}`, filepath.Base(note.Path))
	var selected app.ReplayWorkbenchCollaborationNoteSelection
	var lastErr error
	for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		response, err := client.Post("http://"+addr+"/api/collaboration-note-selection", "application/json", strings.NewReader(body))
		if err != nil {
			lastErr = err
			continue
		}
		if response.StatusCode != http.StatusOK {
			lastErr = fmt.Errorf("unexpected collaboration note selection write status %d", response.StatusCode)
			_ = response.Body.Close()
			continue
		}
		if err := json.NewDecoder(response.Body).Decode(&selected); err != nil {
			lastErr = err
			_ = response.Body.Close()
			continue
		}
		_ = response.Body.Close()
		lastErr = nil
		break
	}
	if lastErr != nil {
		t.Fatalf("post collaboration note selection through replay workbench command: %v", lastErr)
	}
	if selected.Status != "current" || selected.NotePath != note.Path || selected.SelectedBy != "browser" {
		t.Fatalf("unexpected command-written collaboration note selection: %#v", selected)
	}

	response, err := client.Get("http://" + addr + "/api/collaboration-note-selection")
	if err != nil {
		t.Fatalf("get collaboration note selection through replay workbench command: %v", err)
	}
	var listed app.ReplayWorkbenchCollaborationNoteSelection
	if err := json.NewDecoder(response.Body).Decode(&listed); err != nil {
		_ = response.Body.Close()
		t.Fatalf("decode collaboration note selection command listing: %v", err)
	}
	_ = response.Body.Close()
	if listed.Status != "current" || listed.Path != selected.Path || listed.Reason != "browser command selected note" {
		t.Fatalf("unexpected collaboration note selection command listing: %#v", listed)
	}

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("serve replay workbench with collaboration note selection writes: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("replay workbench with collaboration note selection writes did not exit after max requests")
	}
	if !strings.Contains(out.String(), "replay-workbench: accepted=2") {
		t.Fatalf("expected replay workbench collaboration note selection write output, got %q", out.String())
	}
}

func TestReplayWorkbenchCommandLoadsCurrentCollaborationSelection(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := app.InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	run, err := app.RunWorkspace(context.Background(), root)
	if err != nil {
		t.Fatalf("run workspace: %v", err)
	}
	session, err := app.CreateReplayWorkbenchSession(context.Background(), root, app.ReplayWorkbenchSessionOptions{
		PrimaryRunID: run.RunID,
		Filters:      map[string]string{"lane": "timeline"},
		CreatedBy:    "operator",
		Reason:       "collaboration baseline",
	})
	if err != nil {
		t.Fatalf("create replay workbench session: %v", err)
	}
	collaboration, err := app.CreateReplayWorkbenchCollaboration(context.Background(), root, app.ReplayWorkbenchCollaborationOptions{
		SessionPath:  filepath.Base(session.Path),
		Participants: []string{"alice", "bob"},
		CreatedBy:    "lead",
		CreatedAt:    time.Date(2026, 6, 1, 4, 20, 0, 0, time.UTC),
		Reason:       "hosted collaboration focus",
	})
	if err != nil {
		t.Fatalf("create replay workbench collaboration: %v", err)
	}
	if _, err := app.SelectReplayWorkbenchCollaboration(root, filepath.Base(collaboration.Path), app.ReplayWorkbenchCollaborationSelectionOptions{
		SelectedBy: "lead",
		SelectedAt: time.Date(2026, 6, 1, 4, 21, 0, 0, time.UTC),
		Reason:     "current collaboration marker",
	}); err != nil {
		t.Fatalf("select replay workbench collaboration: %v", err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("allocate listen address: %v", err)
	}
	addr := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatalf("close temporary listener: %v", err)
	}

	var out bytes.Buffer
	cmd := newRootCommand(&out, new(bytes.Buffer))
	cmd.SetArgs([]string{
		"replay-workbench", root, run.RunID,
		"--listen", addr,
		"--max-requests", "1",
	})
	done := make(chan error, 1)
	go func() {
		done <- cmd.Execute()
	}()

	client := http.Client{Timeout: 100 * time.Millisecond}
	var selection app.ReplayWorkbenchCollaborationSelection
	var lastErr error
	for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		response, err := client.Get("http://" + addr + "/api/collaboration-selection")
		if err != nil {
			lastErr = err
			continue
		}
		if response.StatusCode != http.StatusOK {
			_ = response.Body.Close()
			lastErr = fmt.Errorf("unexpected collaboration selection API status %d", response.StatusCode)
			continue
		}
		if err := json.NewDecoder(response.Body).Decode(&selection); err != nil {
			_ = response.Body.Close()
			lastErr = err
			continue
		}
		_ = response.Body.Close()
		lastErr = nil
		break
	}
	if lastErr != nil {
		t.Fatalf("get current collaboration selection API: %v", lastErr)
	}
	if selection.CollaborationPath != collaboration.Path ||
		selection.Status != "current" ||
		selection.SelectedBy != "lead" ||
		selection.Reason != "current collaboration marker" {
		t.Fatalf("unexpected current collaboration selection: %#v", selection)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("serve replay workbench with collaboration selection: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("replay workbench with collaboration selection did not exit after max requests")
	}
	if !strings.Contains(out.String(), "replay-workbench: accepted=1") {
		t.Fatalf("expected replay workbench collaboration selection output, got %q", out.String())
	}
}

func TestReplayWorkbenchSessionCommandWritesAndListsMetadataOnlyArtifacts(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := app.InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	primary, err := app.RunWorkspace(context.Background(), root)
	if err != nil {
		t.Fatalf("run primary workspace: %v", err)
	}
	comparison, err := app.RunWorkspace(context.Background(), root)
	if err != nil {
		t.Fatalf("run comparison workspace: %v", err)
	}
	before, err := app.ReplayWorkspace(context.Background(), root, primary.RunID)
	if err != nil {
		t.Fatalf("replay before session command: %v", err)
	}

	var dryRunOut bytes.Buffer
	dryRunCmd := newRootCommand(&dryRunOut, new(bytes.Buffer))
	dryRunCmd.SetArgs([]string{
		"replay-workbench-session", root, primary.RunID,
		"--compare-run", comparison.RunID,
		"--selected-run", comparison.RunID,
		"--lane", "toolchain",
		"--q", "needle",
		"--node", "fact-target",
	})
	if err := dryRunCmd.Execute(); err != nil {
		t.Fatalf("execute replay-workbench-session dry run: %v", err)
	}
	if !strings.Contains(dryRunOut.String(), "replay-workbench-session: status=dry-run mode=metadata-only primary="+primary.RunID+" selected="+comparison.RunID) ||
		!strings.Contains(dryRunOut.String(), "endpoint: graph=/api/graph?lane=toolchain&node=fact-target&q=needle") {
		t.Fatalf("expected dry-run session output, got %q", dryRunOut.String())
	}
	if sessions, err := app.ListReplayWorkbenchSessions(root); err != nil {
		t.Fatalf("list sessions after dry-run command: %v", err)
	} else if len(sessions) != 0 {
		t.Fatalf("dry-run command must not write session artifacts, got %#v", sessions)
	}

	var writeOut bytes.Buffer
	writeCmd := newRootCommand(&writeOut, new(bytes.Buffer))
	writeCmd.SetArgs([]string{
		"replay-workbench-session", root, primary.RunID,
		"--compare-run", comparison.RunID,
		"--selected-run", comparison.RunID,
		"--lane", "toolchain",
		"--q", "needle",
		"--edge", "edge-1",
		"--created-by", "operator",
		"--reason", "save comparison focus",
		"--write",
	})
	if err := writeCmd.Execute(); err != nil {
		t.Fatalf("execute replay-workbench-session write: %v", err)
	}
	if !strings.Contains(writeOut.String(), "replay-workbench-session: status=saved mode=metadata-only primary="+primary.RunID+" selected="+comparison.RunID) ||
		!strings.Contains(writeOut.String(), "artifact=") ||
		!strings.Contains(writeOut.String(), "created_by=operator") ||
		!strings.Contains(writeOut.String(), "reason=save comparison focus") {
		t.Fatalf("expected saved session output, got %q", writeOut.String())
	}
	sessions, err := app.ListReplayWorkbenchSessions(root)
	if err != nil {
		t.Fatalf("list sessions after write command: %v", err)
	}
	if len(sessions) != 1 || sessions[0].SelectedRunID != comparison.RunID || sessions[0].Filters["edge"] != "edge-1" {
		t.Fatalf("unexpected persisted sessions: %#v", sessions)
	}

	var listOut bytes.Buffer
	listCmd := newRootCommand(&listOut, new(bytes.Buffer))
	listCmd.SetArgs([]string{"replay-workbench-sessions", root})
	if err := listCmd.Execute(); err != nil {
		t.Fatalf("execute replay-workbench-sessions list: %v", err)
	}
	if !strings.Contains(listOut.String(), "replay-workbench-sessions: total=1") ||
		!strings.Contains(listOut.String(), "replay-workbench-session: status=saved mode=metadata-only") {
		t.Fatalf("expected session listing output, got %q", listOut.String())
	}

	var inspectOut bytes.Buffer
	inspectCmd := newRootCommand(&inspectOut, new(bytes.Buffer))
	inspectCmd.SetArgs([]string{"replay-workbench-sessions", root, filepath.Base(sessions[0].Path)})
	if err := inspectCmd.Execute(); err != nil {
		t.Fatalf("execute replay-workbench-sessions inspect: %v", err)
	}
	for _, want := range []string{
		"replay-workbench-session: status=saved mode=metadata-only",
		"filter: edge=edge-1",
		"endpoint: html=/?edge=edge-1&lane=toolchain&q=needle&run=" + comparison.RunID,
	} {
		if !strings.Contains(inspectOut.String(), want) {
			t.Fatalf("expected inspect output to contain %q, got %q", want, inspectOut.String())
		}
	}

	after, err := app.ReplayWorkspace(context.Background(), root, primary.RunID)
	if err != nil {
		t.Fatalf("replay after session command: %v", err)
	}
	if after.EventCount != before.EventCount {
		t.Fatalf("saved workbench session must not mutate replay events, before=%d after=%d", before.EventCount, after.EventCount)
	}
}

func TestReplayWorkbenchCollaborationCommandWritesAndListsMetadataOnlyArtifacts(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := app.InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	run, err := app.RunWorkspace(context.Background(), root)
	if err != nil {
		t.Fatalf("run workspace: %v", err)
	}
	session, err := app.CreateReplayWorkbenchSession(context.Background(), root, app.ReplayWorkbenchSessionOptions{
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
	before, err := app.ReplayWorkspace(context.Background(), root, run.RunID)
	if err != nil {
		t.Fatalf("replay before collaboration command: %v", err)
	}

	var dryRunOut bytes.Buffer
	dryRunCmd := newRootCommand(&dryRunOut, new(bytes.Buffer))
	dryRunCmd.SetArgs([]string{
		"replay-workbench-collaboration", root, filepath.Base(session.Path),
		"--participant", "alice",
		"--participant", "bob",
	})
	if err := dryRunCmd.Execute(); err != nil {
		t.Fatalf("execute replay-workbench-collaboration dry run: %v", err)
	}
	if !strings.Contains(dryRunOut.String(), "replay-workbench-collaboration: status=dry-run mode=metadata-only session="+session.Path) ||
		!strings.Contains(dryRunOut.String(), "participant: alice") ||
		!strings.Contains(dryRunOut.String(), "endpoint: collaborations=/api/collaborations") {
		t.Fatalf("expected dry-run collaboration output, got %q", dryRunOut.String())
	}
	if collaborations, err := app.ListReplayWorkbenchCollaborations(root); err != nil {
		t.Fatalf("list collaborations after dry-run command: %v", err)
	} else if len(collaborations) != 0 {
		t.Fatalf("dry-run command must not write collaboration artifacts, got %#v", collaborations)
	}

	var writeOut bytes.Buffer
	writeCmd := newRootCommand(&writeOut, new(bytes.Buffer))
	writeCmd.SetArgs([]string{
		"replay-workbench-collaboration", root, filepath.Base(session.Path),
		"--participant", "alice",
		"--participant", "bob",
		"--created-by", "lead",
		"--created-at", "2026-06-01T03:45:00Z",
		"--reason", "pair review",
		"--write",
	})
	if err := writeCmd.Execute(); err != nil {
		t.Fatalf("execute replay-workbench-collaboration write: %v", err)
	}
	if !strings.Contains(writeOut.String(), "replay-workbench-collaboration: status=saved mode=metadata-only session="+session.Path) ||
		!strings.Contains(writeOut.String(), "participants=2") ||
		!strings.Contains(writeOut.String(), "created_by=lead") ||
		!strings.Contains(writeOut.String(), "reason=pair review") {
		t.Fatalf("expected saved collaboration output, got %q", writeOut.String())
	}
	collaborations, err := app.ListReplayWorkbenchCollaborations(root)
	if err != nil {
		t.Fatalf("list collaborations after write command: %v", err)
	}
	if len(collaborations) != 1 || collaborations[0].SessionPath != session.Path || len(collaborations[0].Participants) != 2 {
		t.Fatalf("unexpected persisted collaborations: %#v", collaborations)
	}

	var listOut bytes.Buffer
	listCmd := newRootCommand(&listOut, new(bytes.Buffer))
	listCmd.SetArgs([]string{"replay-workbench-collaborations", root})
	if err := listCmd.Execute(); err != nil {
		t.Fatalf("execute replay-workbench-collaborations list: %v", err)
	}
	if !strings.Contains(listOut.String(), "replay-workbench-collaborations: total=1") ||
		!strings.Contains(listOut.String(), "replay-workbench-collaboration: status=saved mode=metadata-only") {
		t.Fatalf("expected collaboration listing output, got %q", listOut.String())
	}

	var inspectOut bytes.Buffer
	inspectCmd := newRootCommand(&inspectOut, new(bytes.Buffer))
	inspectCmd.SetArgs([]string{"replay-workbench-collaborations", root, filepath.Base(collaborations[0].Path)})
	if err := inspectCmd.Execute(); err != nil {
		t.Fatalf("execute replay-workbench-collaborations inspect: %v", err)
	}
	for _, want := range []string{
		"replay-workbench-collaboration: status=saved mode=metadata-only",
		"participant: alice",
		"participant: bob",
		"endpoint: session=/api/session?lane=timeline&q=collab",
	} {
		if !strings.Contains(inspectOut.String(), want) {
			t.Fatalf("expected inspect output to contain %q, got %q", want, inspectOut.String())
		}
	}

	after, err := app.ReplayWorkspace(context.Background(), root, run.RunID)
	if err != nil {
		t.Fatalf("replay after collaboration command: %v", err)
	}
	if after.EventCount != before.EventCount {
		t.Fatalf("saved workbench collaboration must not mutate replay events, before=%d after=%d", before.EventCount, after.EventCount)
	}
}

func TestReplayWorkbenchCommandEnablesCollaborationPresencePolling(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := app.InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	run, err := app.RunWorkspace(context.Background(), root)
	if err != nil {
		t.Fatalf("run workspace: %v", err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("allocate listen address: %v", err)
	}
	addr := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatalf("close temporary listener: %v", err)
	}

	var out bytes.Buffer
	cmd := newRootCommand(&out, new(bytes.Buffer))
	cmd.SetArgs([]string{
		"replay-workbench", root, run.RunID,
		"--listen", addr,
		"--max-requests", "1",
		"--collaboration-presence-poll-interval-ms", "1500",
	})
	done := make(chan error, 1)
	go func() {
		done <- cmd.Execute()
	}()

	client := http.Client{Timeout: 250 * time.Millisecond}
	var htmlBody string
	var lastErr error
	for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		response, err := client.Get("http://" + addr + "/")
		if err != nil {
			lastErr = err
			continue
		}
		var body bytes.Buffer
		if _, err := body.ReadFrom(response.Body); err != nil {
			lastErr = err
			_ = response.Body.Close()
			continue
		}
		_ = response.Body.Close()
		if response.StatusCode != http.StatusOK {
			lastErr = fmt.Errorf("unexpected replay workbench status %d body=%q", response.StatusCode, body.String())
			continue
		}
		htmlBody = body.String()
		lastErr = nil
		break
	}
	if lastErr != nil {
		t.Fatalf("get replay workbench HTML with presence polling: %v", lastErr)
	}
	for _, want := range []string{
		"window.OURO_REPLAY_COLLABORATION_PRESENCE_POLL_INTERVAL_MS = 1500",
		"setInterval(refreshReplayWorkbenchCollaborationPresence, collaborationPresencePollIntervalMS)",
	} {
		if !strings.Contains(htmlBody, want) {
			t.Fatalf("expected replay workbench HTML to contain %q, got %q", want, htmlBody)
		}
	}
	if strings.Contains(htmlBody, "method: \"POST\"") {
		t.Fatalf("presence polling CLI flag must not expose POST without write flag, got %q", htmlBody)
	}

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("serve replay workbench with collaboration presence polling: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for replay workbench polling server to stop")
	}
}

func TestReplayWorkbenchCommandEnablesCollaborationSelectionPolling(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := app.InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	run, err := app.RunWorkspace(context.Background(), root)
	if err != nil {
		t.Fatalf("run workspace: %v", err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("allocate listen address: %v", err)
	}
	addr := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatalf("close temporary listener: %v", err)
	}

	var out bytes.Buffer
	cmd := newRootCommand(&out, new(bytes.Buffer))
	cmd.SetArgs([]string{
		"replay-workbench", root, run.RunID,
		"--listen", addr,
		"--max-requests", "1",
		"--collaboration-selection-poll-interval-ms", "1500",
	})
	done := make(chan error, 1)
	go func() {
		done <- cmd.Execute()
	}()

	client := http.Client{Timeout: 250 * time.Millisecond}
	var htmlBody string
	var lastErr error
	for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		response, err := client.Get("http://" + addr + "/")
		if err != nil {
			lastErr = err
			continue
		}
		var body bytes.Buffer
		if _, err := body.ReadFrom(response.Body); err != nil {
			lastErr = err
			_ = response.Body.Close()
			continue
		}
		_ = response.Body.Close()
		if response.StatusCode != http.StatusOK {
			lastErr = fmt.Errorf("unexpected replay workbench status %d body=%q", response.StatusCode, body.String())
			continue
		}
		htmlBody = body.String()
		lastErr = nil
		break
	}
	if lastErr != nil {
		t.Fatalf("get replay workbench HTML with selection polling: %v", lastErr)
	}
	for _, want := range []string{
		"window.OURO_REPLAY_COLLABORATION_SELECTION_POLL_INTERVAL_MS = 1500",
		"setInterval(refreshReplayWorkbenchCollaborationSelection, collaborationSelectionPollIntervalMS)",
	} {
		if !strings.Contains(htmlBody, want) {
			t.Fatalf("expected replay workbench HTML to contain %q, got %q", want, htmlBody)
		}
	}
	if strings.Contains(htmlBody, "method: \"POST\"") {
		t.Fatalf("selection polling CLI flag must not expose POST without write flag, got %q", htmlBody)
	}

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("serve replay workbench with collaboration selection polling: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for replay workbench selection polling server to stop")
	}
}

func TestReplayWorkbenchCommandEnablesCollaborationNotesPolling(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := app.InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	run, err := app.RunWorkspace(context.Background(), root)
	if err != nil {
		t.Fatalf("run workspace: %v", err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("allocate listen address: %v", err)
	}
	addr := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatalf("close temporary listener: %v", err)
	}

	var out bytes.Buffer
	cmd := newRootCommand(&out, new(bytes.Buffer))
	cmd.SetArgs([]string{
		"replay-workbench", root, run.RunID,
		"--listen", addr,
		"--max-requests", "1",
		"--collaboration-notes-poll-interval-ms", "1500",
	})
	done := make(chan error, 1)
	go func() {
		done <- cmd.Execute()
	}()

	client := http.Client{Timeout: 250 * time.Millisecond}
	var htmlBody string
	var lastErr error
	for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		response, err := client.Get("http://" + addr + "/")
		if err != nil {
			lastErr = err
			continue
		}
		var body bytes.Buffer
		if _, err := body.ReadFrom(response.Body); err != nil {
			lastErr = err
			_ = response.Body.Close()
			continue
		}
		_ = response.Body.Close()
		if response.StatusCode != http.StatusOK {
			lastErr = fmt.Errorf("unexpected replay workbench status %d body=%q", response.StatusCode, body.String())
			continue
		}
		htmlBody = body.String()
		lastErr = nil
		break
	}
	if lastErr != nil {
		t.Fatalf("get replay workbench HTML with notes polling: %v", lastErr)
	}
	for _, want := range []string{
		"window.OURO_REPLAY_COLLABORATION_NOTES_POLL_INTERVAL_MS = 1500",
		"setInterval(refreshReplayWorkbenchCollaborationNotes, collaborationNotesPollIntervalMS)",
	} {
		if !strings.Contains(htmlBody, want) {
			t.Fatalf("expected replay workbench HTML to contain %q, got %q", want, htmlBody)
		}
	}
	if strings.Contains(htmlBody, "method: \"POST\"") {
		t.Fatalf("notes polling CLI flag must not expose POST without write flag, got %q", htmlBody)
	}

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("serve replay workbench with collaboration notes polling: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for replay workbench notes polling server to stop")
	}
}

func TestReplayWorkbenchCommandEnablesCollaborationNoteSelectionPolling(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := app.InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	run, err := app.RunWorkspace(context.Background(), root)
	if err != nil {
		t.Fatalf("run workspace: %v", err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("allocate listen address: %v", err)
	}
	addr := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatalf("close temporary listener: %v", err)
	}

	var out bytes.Buffer
	cmd := newRootCommand(&out, new(bytes.Buffer))
	cmd.SetArgs([]string{
		"replay-workbench", root, run.RunID,
		"--listen", addr,
		"--max-requests", "1",
		"--collaboration-note-selection-poll-interval-ms", "1500",
	})
	done := make(chan error, 1)
	go func() {
		done <- cmd.Execute()
	}()

	client := http.Client{Timeout: 250 * time.Millisecond}
	var htmlBody string
	var lastErr error
	for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		response, err := client.Get("http://" + addr + "/")
		if err != nil {
			lastErr = err
			continue
		}
		var body bytes.Buffer
		if _, err := body.ReadFrom(response.Body); err != nil {
			lastErr = err
			_ = response.Body.Close()
			continue
		}
		_ = response.Body.Close()
		if response.StatusCode != http.StatusOK {
			lastErr = fmt.Errorf("unexpected replay workbench status %d body=%q", response.StatusCode, body.String())
			continue
		}
		htmlBody = body.String()
		lastErr = nil
		break
	}
	if lastErr != nil {
		t.Fatalf("get replay workbench HTML with note selection polling: %v", lastErr)
	}
	for _, want := range []string{
		"window.OURO_REPLAY_COLLABORATION_NOTE_SELECTION_POLL_INTERVAL_MS = 1500",
		"setInterval(refreshReplayWorkbenchCollaborationNoteSelection, collaborationNoteSelectionPollIntervalMS)",
	} {
		if !strings.Contains(htmlBody, want) {
			t.Fatalf("expected replay workbench HTML to contain %q, got %q", want, htmlBody)
		}
	}
	if strings.Contains(htmlBody, "method: \"POST\"") {
		t.Fatalf("note selection polling CLI flag must not expose POST without write flag, got %q", htmlBody)
	}

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("serve replay workbench with collaboration note selection polling: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for replay workbench note selection polling server to stop")
	}
}

func TestReplayWorkbenchCommandEnablesCollaborationStream(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := app.InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	run, err := app.RunWorkspace(context.Background(), root)
	if err != nil {
		t.Fatalf("run workspace: %v", err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("allocate listen address: %v", err)
	}
	addr := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatalf("close temporary listener: %v", err)
	}

	var out bytes.Buffer
	cmd := newRootCommand(&out, new(bytes.Buffer))
	cmd.SetArgs([]string{
		"replay-workbench", root, run.RunID,
		"--listen", addr,
		"--max-requests", "1",
		"--enable-collaboration-stream",
	})
	done := make(chan error, 1)
	go func() {
		done <- cmd.Execute()
	}()

	client := http.Client{Timeout: 250 * time.Millisecond}
	var htmlBody string
	var lastErr error
	for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		response, err := client.Get("http://" + addr + "/")
		if err != nil {
			lastErr = err
			continue
		}
		var body bytes.Buffer
		if _, err := body.ReadFrom(response.Body); err != nil {
			lastErr = err
			_ = response.Body.Close()
			continue
		}
		_ = response.Body.Close()
		if response.StatusCode != http.StatusOK {
			lastErr = fmt.Errorf("unexpected replay workbench status %d body=%q", response.StatusCode, body.String())
			continue
		}
		htmlBody = body.String()
		lastErr = nil
		break
	}
	if lastErr != nil {
		t.Fatalf("get replay workbench HTML with collaboration stream: %v", lastErr)
	}
	for _, want := range []string{
		"window.OURO_REPLAY_COLLABORATION_STREAM_ENABLED = true",
		"new EventSource(\"/api/collaboration-events\")",
		"installReplayWorkbenchCollaborationStream",
	} {
		if !strings.Contains(htmlBody, want) {
			t.Fatalf("expected replay workbench HTML to contain %q, got %q", want, htmlBody)
		}
	}
	if strings.Contains(htmlBody, "method: \"POST\"") {
		t.Fatalf("collaboration stream CLI flag must not expose POST without write flags, got %q", htmlBody)
	}

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("serve replay workbench with collaboration stream: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for replay workbench stream server to stop")
	}
}

func TestReplayWorkbenchCommandEnablesCollaborationStreamRefresh(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := app.InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	run, err := app.RunWorkspace(context.Background(), root)
	if err != nil {
		t.Fatalf("run workspace: %v", err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("allocate listen address: %v", err)
	}
	addr := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatalf("close temporary listener: %v", err)
	}

	var out bytes.Buffer
	cmd := newRootCommand(&out, new(bytes.Buffer))
	cmd.SetArgs([]string{
		"replay-workbench", root, run.RunID,
		"--listen", addr,
		"--max-requests", "1",
		"--enable-collaboration-stream",
		"--collaboration-stream-refresh-interval-ms", "25",
	})
	done := make(chan error, 1)
	go func() {
		done <- cmd.Execute()
	}()

	client := http.Client{Timeout: 250 * time.Millisecond}
	var htmlBody string
	var lastErr error
	for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		response, err := client.Get("http://" + addr + "/")
		if err != nil {
			lastErr = err
			continue
		}
		var body bytes.Buffer
		if _, err := body.ReadFrom(response.Body); err != nil {
			lastErr = err
			_ = response.Body.Close()
			continue
		}
		_ = response.Body.Close()
		if response.StatusCode != http.StatusOK {
			lastErr = fmt.Errorf("unexpected replay workbench status %d body=%q", response.StatusCode, body.String())
			continue
		}
		htmlBody = body.String()
		lastErr = nil
		break
	}
	if lastErr != nil {
		t.Fatalf("get replay workbench HTML with collaboration stream refresh: %v", lastErr)
	}
	for _, want := range []string{
		"window.OURO_REPLAY_COLLABORATION_STREAM_ENABLED = true",
		"new EventSource(\"/api/collaboration-events\")",
	} {
		if !strings.Contains(htmlBody, want) {
			t.Fatalf("expected replay workbench HTML to contain %q, got %q", want, htmlBody)
		}
	}
	if strings.Contains(htmlBody, "method: \"POST\"") {
		t.Fatalf("collaboration stream refresh flag must not expose POST without write flags, got %q", htmlBody)
	}

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("serve replay workbench with collaboration stream refresh: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for replay workbench stream refresh server to stop")
	}
}

func TestReplayWorkbenchCollaborationPresenceCommandWritesAndListsMetadataOnlyArtifacts(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := app.InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	run, err := app.RunWorkspace(context.Background(), root)
	if err != nil {
		t.Fatalf("run workspace: %v", err)
	}
	session, err := app.CreateReplayWorkbenchSession(context.Background(), root, app.ReplayWorkbenchSessionOptions{
		PrimaryRunID: run.RunID,
		Reason:       "presence baseline",
	})
	if err != nil {
		t.Fatalf("create replay workbench session: %v", err)
	}
	collaboration, err := app.CreateReplayWorkbenchCollaboration(context.Background(), root, app.ReplayWorkbenchCollaborationOptions{
		SessionPath:  filepath.Base(session.Path),
		Participants: []string{"alice", "bob"},
		CreatedBy:    "lead",
		Reason:       "presence review",
	})
	if err != nil {
		t.Fatalf("create replay workbench collaboration: %v", err)
	}
	before, err := app.ReplayWorkspace(context.Background(), root, run.RunID)
	if err != nil {
		t.Fatalf("replay before collaboration presence command: %v", err)
	}

	var dryRunOut bytes.Buffer
	dryRunCmd := newRootCommand(&dryRunOut, new(bytes.Buffer))
	dryRunCmd.SetArgs([]string{
		"replay-workbench-collaboration-presence", root, filepath.Base(collaboration.Path),
		"--participant", "alice",
		"--status", "online",
		"--seen-by", "lead",
		"--seen-at", "2026-06-01T05:40:00Z",
		"--reason", "joined review",
	})
	if err := dryRunCmd.Execute(); err != nil {
		t.Fatalf("execute replay-workbench-collaboration-presence dry run: %v", err)
	}
	if !strings.Contains(dryRunOut.String(), "replay-workbench-collaboration-presence: status=online mode=metadata-only collaboration="+collaboration.Path) ||
		!strings.Contains(dryRunOut.String(), "participant=alice") ||
		!strings.Contains(dryRunOut.String(), "seen_by=lead") {
		t.Fatalf("expected dry-run collaboration presence output, got %q", dryRunOut.String())
	}
	if presence, err := app.ListReplayWorkbenchCollaborationPresence(root); err != nil {
		t.Fatalf("list presence after dry-run command: %v", err)
	} else if len(presence) != 0 {
		t.Fatalf("dry-run command must not write collaboration presence artifacts, got %#v", presence)
	}

	var writeOut bytes.Buffer
	writeCmd := newRootCommand(&writeOut, new(bytes.Buffer))
	writeCmd.SetArgs([]string{
		"replay-workbench-collaboration-presence", root, filepath.Base(collaboration.Path),
		"--participant", "alice",
		"--status", "away",
		"--seen-by", "alice",
		"--seen-at", "2026-06-01T05:41:00Z",
		"--reason", "review still active",
		"--write",
	})
	if err := writeCmd.Execute(); err != nil {
		t.Fatalf("execute replay-workbench-collaboration-presence write: %v", err)
	}
	if !strings.Contains(writeOut.String(), "replay-workbench-collaboration-presence: status=away mode=metadata-only collaboration="+collaboration.Path) ||
		!strings.Contains(writeOut.String(), "participant=alice") ||
		!strings.Contains(writeOut.String(), "artifact=") ||
		!strings.Contains(writeOut.String(), "reason=review still active") {
		t.Fatalf("expected saved collaboration presence output, got %q", writeOut.String())
	}
	presence, err := app.ListReplayWorkbenchCollaborationPresence(root)
	if err != nil {
		t.Fatalf("list collaboration presence after write command: %v", err)
	}
	if len(presence) != 1 || presence[0].CollaborationPath != collaboration.Path || presence[0].Participant != "alice" || presence[0].Status != "away" {
		t.Fatalf("unexpected persisted collaboration presence: %#v", presence)
	}

	var listOut bytes.Buffer
	listCmd := newRootCommand(&listOut, new(bytes.Buffer))
	listCmd.SetArgs([]string{"replay-workbench-collaboration-presences", root})
	if err := listCmd.Execute(); err != nil {
		t.Fatalf("execute replay-workbench-collaboration-presences list: %v", err)
	}
	if !strings.Contains(listOut.String(), "replay-workbench-collaboration-presences: total=1") ||
		!strings.Contains(listOut.String(), "replay-workbench-collaboration-presence: status=away mode=metadata-only") {
		t.Fatalf("expected collaboration presence listing output, got %q", listOut.String())
	}

	var inspectOut bytes.Buffer
	inspectCmd := newRootCommand(&inspectOut, new(bytes.Buffer))
	inspectCmd.SetArgs([]string{"replay-workbench-collaboration-presences", root, filepath.Base(presence[0].Path)})
	if err := inspectCmd.Execute(); err != nil {
		t.Fatalf("execute replay-workbench-collaboration-presences inspect: %v", err)
	}
	for _, want := range []string{
		"replay-workbench-collaboration-presence: status=away mode=metadata-only",
		"collaboration=" + collaboration.Path,
		"participant=alice",
		"seen_by=alice",
		"reason=review still active",
	} {
		if !strings.Contains(inspectOut.String(), want) {
			t.Fatalf("expected inspect output to contain %q, got %q", want, inspectOut.String())
		}
	}

	after, err := app.ReplayWorkspace(context.Background(), root, run.RunID)
	if err != nil {
		t.Fatalf("replay after collaboration presence command: %v", err)
	}
	if after.EventCount != before.EventCount {
		t.Fatalf("saved workbench collaboration presence must not mutate replay events, before=%d after=%d", before.EventCount, after.EventCount)
	}
}

func TestReplayWorkbenchCollaborationNoteCommandWritesAndListsMetadataOnlyArtifacts(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := app.InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	run, err := app.RunWorkspace(context.Background(), root)
	if err != nil {
		t.Fatalf("run workspace: %v", err)
	}
	session, err := app.CreateReplayWorkbenchSession(context.Background(), root, app.ReplayWorkbenchSessionOptions{
		PrimaryRunID: run.RunID,
		Filters:      map[string]string{"node": "fact-target"},
		Reason:       "note command baseline",
	})
	if err != nil {
		t.Fatalf("create replay workbench session: %v", err)
	}
	collaboration, err := app.CreateReplayWorkbenchCollaboration(context.Background(), root, app.ReplayWorkbenchCollaborationOptions{
		SessionPath:  filepath.Base(session.Path),
		Participants: []string{"alice", "bob"},
		CreatedBy:    "lead",
		Reason:       "note command review",
	})
	if err != nil {
		t.Fatalf("create replay workbench collaboration: %v", err)
	}
	before, err := app.ReplayWorkspace(context.Background(), root, run.RunID)
	if err != nil {
		t.Fatalf("replay before collaboration note command: %v", err)
	}

	var dryRunOut bytes.Buffer
	dryRunCmd := newRootCommand(&dryRunOut, new(bytes.Buffer))
	dryRunCmd.SetArgs([]string{
		"replay-workbench-collaboration-note", root, filepath.Base(collaboration.Path),
		"--author", "alice",
		"--body", "Confirm graph node before accepting.",
		"--target-kind", "graph-node",
		"--target-id", "fact-target",
		"--created-at", "2026-06-01T08:20:00Z",
		"--reason", "collaborative evidence review",
	})
	if err := dryRunCmd.Execute(); err != nil {
		t.Fatalf("execute replay-workbench-collaboration-note dry run: %v", err)
	}
	if !strings.Contains(dryRunOut.String(), "replay-workbench-collaboration-note: status=dry-run mode=metadata-only collaboration="+collaboration.Path) ||
		!strings.Contains(dryRunOut.String(), "author=alice") ||
		!strings.Contains(dryRunOut.String(), "target=graph-node:fact-target") {
		t.Fatalf("expected dry-run collaboration note output, got %q", dryRunOut.String())
	}
	if notes, err := app.ListReplayWorkbenchCollaborationNotes(root); err != nil {
		t.Fatalf("list notes after dry-run command: %v", err)
	} else if len(notes) != 0 {
		t.Fatalf("dry-run command must not write collaboration note artifacts, got %#v", notes)
	}

	var writeOut bytes.Buffer
	writeCmd := newRootCommand(&writeOut, new(bytes.Buffer))
	writeCmd.SetArgs([]string{
		"replay-workbench-collaboration-note", root, filepath.Base(collaboration.Path),
		"--author", "alice",
		"--body", "Confirm graph node before accepting.",
		"--target-kind", "graph-node",
		"--target-id", "fact-target",
		"--created-at", "2026-06-01T08:20:00Z",
		"--reason", "collaborative evidence review",
		"--write",
	})
	if err := writeCmd.Execute(); err != nil {
		t.Fatalf("execute replay-workbench-collaboration-note write: %v", err)
	}
	if !strings.Contains(writeOut.String(), "replay-workbench-collaboration-note: status=saved mode=metadata-only collaboration="+collaboration.Path) ||
		!strings.Contains(writeOut.String(), "author=alice") ||
		!strings.Contains(writeOut.String(), "artifact=") ||
		!strings.Contains(writeOut.String(), "body=Confirm graph node before accepting.") {
		t.Fatalf("expected saved collaboration note output, got %q", writeOut.String())
	}
	notes, err := app.ListReplayWorkbenchCollaborationNotes(root)
	if err != nil {
		t.Fatalf("list collaboration notes after write command: %v", err)
	}
	if len(notes) != 1 || notes[0].CollaborationPath != collaboration.Path || notes[0].Author != "alice" || notes[0].TargetID != "fact-target" {
		t.Fatalf("unexpected persisted collaboration notes: %#v", notes)
	}

	var listOut bytes.Buffer
	listCmd := newRootCommand(&listOut, new(bytes.Buffer))
	listCmd.SetArgs([]string{"replay-workbench-collaboration-notes", root})
	if err := listCmd.Execute(); err != nil {
		t.Fatalf("execute replay-workbench-collaboration-notes list: %v", err)
	}
	if !strings.Contains(listOut.String(), "replay-workbench-collaboration-notes: total=1") ||
		!strings.Contains(listOut.String(), "replay-workbench-collaboration-note: status=saved mode=metadata-only") {
		t.Fatalf("expected collaboration note listing output, got %q", listOut.String())
	}

	var inspectOut bytes.Buffer
	inspectCmd := newRootCommand(&inspectOut, new(bytes.Buffer))
	inspectCmd.SetArgs([]string{"replay-workbench-collaboration-notes", root, filepath.Base(notes[0].Path)})
	if err := inspectCmd.Execute(); err != nil {
		t.Fatalf("execute replay-workbench-collaboration-notes inspect: %v", err)
	}
	for _, want := range []string{
		"replay-workbench-collaboration-note: status=saved mode=metadata-only",
		"collaboration=" + collaboration.Path,
		"author=alice",
		"target=graph-node:fact-target",
		"body=Confirm graph node before accepting.",
	} {
		if !strings.Contains(inspectOut.String(), want) {
			t.Fatalf("expected inspect output to contain %q, got %q", want, inspectOut.String())
		}
	}

	after, err := app.ReplayWorkspace(context.Background(), root, run.RunID)
	if err != nil {
		t.Fatalf("replay after collaboration note command: %v", err)
	}
	if after.EventCount != before.EventCount {
		t.Fatalf("saved workbench collaboration note must not mutate replay events, before=%d after=%d", before.EventCount, after.EventCount)
	}
}

func TestReplayWorkbenchCollaborationNoteAnnotateCommandWritesMetadataOnlyDecision(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := app.InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	run, err := app.RunWorkspace(context.Background(), root)
	if err != nil {
		t.Fatalf("run workspace: %v", err)
	}
	session, err := app.CreateReplayWorkbenchSession(context.Background(), root, app.ReplayWorkbenchSessionOptions{
		PrimaryRunID: run.RunID,
		Reason:       "saved note baseline",
	})
	if err != nil {
		t.Fatalf("create replay workbench session: %v", err)
	}
	collaboration, err := app.CreateReplayWorkbenchCollaboration(context.Background(), root, app.ReplayWorkbenchCollaborationOptions{
		SessionPath:  filepath.Base(session.Path),
		Participants: []string{"alice", "bob"},
		CreatedBy:    "lead",
		Reason:       "pair review",
	})
	if err != nil {
		t.Fatalf("create replay workbench collaboration: %v", err)
	}
	note, err := app.CreateReplayWorkbenchCollaborationNote(context.Background(), root, app.ReplayWorkbenchCollaborationNoteOptions{
		CollaborationPath: filepath.Base(collaboration.Path),
		Author:            "alice",
		Body:              "Confirm graph node before accepting.",
		TargetKind:        "graph-node",
		TargetID:          "fact-target",
		Reason:            "collaborative evidence review",
	})
	if err != nil {
		t.Fatalf("create replay workbench collaboration note: %v", err)
	}

	var out bytes.Buffer
	cmd := newRootCommand(&out, new(bytes.Buffer))
	cmd.SetArgs([]string{
		"replay-workbench-collaboration-note-annotate", root, filepath.Base(note.Path),
		"--decision", "deferred",
		"--reason", "needs graph confirmation",
		"--reviewer", "alice",
		"--reviewed-at", "2026-06-01T09:30:00Z",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute replay-workbench-collaboration-note-annotate: %v", err)
	}
	for _, want := range []string{
		"replay-workbench-collaboration-note: status=saved mode=metadata-only collaboration=" + collaboration.Path,
		"replay-collaboration-note-annotation: decision=deferred reviewer=alice reviewed_at=2026-06-01T09:30:00Z reason=needs graph confirmation",
	} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("expected collaboration note annotation output to contain %q, got %q", want, out.String())
		}
	}

	var listOut bytes.Buffer
	listCmd := newRootCommand(&listOut, new(bytes.Buffer))
	listCmd.SetArgs([]string{"replay-workbench-collaboration-notes", root})
	if err := listCmd.Execute(); err != nil {
		t.Fatalf("execute replay-workbench-collaboration-notes list: %v", err)
	}
	if !strings.Contains(listOut.String(), "replay-collaboration-note-annotation: decision=deferred reviewer=alice reviewed_at=2026-06-01T09:30:00Z reason=needs graph confirmation") {
		t.Fatalf("expected replay-workbench-collaboration-notes list to show annotation, got %q", listOut.String())
	}
}

func TestReplayWorkbenchCollaborationNoteSelectionCommandsSelectReadAndClearMetadataOnly(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := app.InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	run, err := app.RunWorkspace(context.Background(), root)
	if err != nil {
		t.Fatalf("run workspace: %v", err)
	}
	session, err := app.CreateReplayWorkbenchSession(context.Background(), root, app.ReplayWorkbenchSessionOptions{
		PrimaryRunID: run.RunID,
		Reason:       "saved note baseline",
	})
	if err != nil {
		t.Fatalf("create replay workbench session: %v", err)
	}
	collaboration, err := app.CreateReplayWorkbenchCollaboration(context.Background(), root, app.ReplayWorkbenchCollaborationOptions{
		SessionPath:  filepath.Base(session.Path),
		Participants: []string{"alice", "bob"},
		Reason:       "pair review",
	})
	if err != nil {
		t.Fatalf("create replay workbench collaboration: %v", err)
	}
	base, err := app.CreateReplayWorkbenchCollaborationNote(context.Background(), root, app.ReplayWorkbenchCollaborationNoteOptions{
		CollaborationPath: filepath.Base(collaboration.Path),
		Author:            "alice",
		Body:              "Baseline note",
		TargetKind:        "graph-node",
		TargetID:          "fact-base",
		CreatedAt:         time.Date(2026, 6, 1, 9, 40, 0, 0, time.UTC),
		Reason:            "baseline evidence note",
	})
	if err != nil {
		t.Fatalf("create baseline replay workbench collaboration note: %v", err)
	}
	candidate, err := app.CreateReplayWorkbenchCollaborationNote(context.Background(), root, app.ReplayWorkbenchCollaborationNoteOptions{
		CollaborationPath: filepath.Base(collaboration.Path),
		Author:            "bob",
		Body:              "Current note",
		TargetKind:        "graph-edge",
		TargetID:          "edge-1",
		CreatedAt:         time.Date(2026, 6, 1, 9, 41, 0, 0, time.UTC),
		Reason:            "current evidence note",
	})
	if err != nil {
		t.Fatalf("create candidate replay workbench collaboration note: %v", err)
	}

	var selectOut bytes.Buffer
	selectCmd := newRootCommand(&selectOut, new(bytes.Buffer))
	selectCmd.SetArgs([]string{
		"replay-workbench-collaboration-note-select", root, filepath.Base(candidate.Path),
		"--selected-by", "alice",
		"--selected-at", "2026-06-01T09:42:00Z",
		"--reason", "current note focus",
	})
	if err := selectCmd.Execute(); err != nil {
		t.Fatalf("execute replay-workbench-collaboration-note-select: %v", err)
	}
	for _, want := range []string{
		"replay-workbench-collaboration-note-selection: status=current mode=metadata-only note=" + candidate.Path,
		"collaboration=" + collaboration.Path + " session=" + session.Path + " primary=" + run.RunID + " selected=" + run.RunID + " author=bob target=graph-edge:edge-1 superseded=1 selected_by=alice selected_at=2026-06-01T09:42:00Z reason=current note focus",
		"replay-collaboration-note-superseded: " + base.Path,
	} {
		if !strings.Contains(selectOut.String(), want) {
			t.Fatalf("expected collaboration note selection output to contain %q, got %q", want, selectOut.String())
		}
	}

	var readOut bytes.Buffer
	readCmd := newRootCommand(&readOut, new(bytes.Buffer))
	readCmd.SetArgs([]string{"replay-workbench-collaboration-note-selection", root})
	if err := readCmd.Execute(); err != nil {
		t.Fatalf("execute replay-workbench-collaboration-note-selection: %v", err)
	}
	if !strings.Contains(readOut.String(), "replay-workbench-collaboration-note-selection: status=current mode=metadata-only note="+candidate.Path) {
		t.Fatalf("expected collaboration note selection read output, got %q", readOut.String())
	}

	var clearOut bytes.Buffer
	clearCmd := newRootCommand(&clearOut, new(bytes.Buffer))
	clearCmd.SetArgs([]string{
		"replay-workbench-collaboration-note-selection-clear", root,
		"--cleared-by", "bob",
		"--cleared-at", "2026-06-01T09:45:00Z",
		"--reason", "note focus closed",
	})
	if err := clearCmd.Execute(); err != nil {
		t.Fatalf("execute replay-workbench-collaboration-note-selection-clear: %v", err)
	}
	if !strings.Contains(clearOut.String(), "replay-workbench-collaboration-note-selection: status=cleared mode=metadata-only note="+candidate.Path) ||
		!strings.Contains(clearOut.String(), "cleared_by=bob cleared_at=2026-06-01T09:45:00Z reason=note focus closed") {
		t.Fatalf("expected cleared collaboration note selection output, got %q", clearOut.String())
	}
}

func TestReplayWorkbenchSessionAnnotateCommandWritesMetadataOnlyDecision(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := app.InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	run, err := app.RunWorkspace(context.Background(), root)
	if err != nil {
		t.Fatalf("run workspace: %v", err)
	}
	session, err := app.CreateReplayWorkbenchSession(context.Background(), root, app.ReplayWorkbenchSessionOptions{
		PrimaryRunID: run.RunID,
		Reason:       "saved replay focus",
	})
	if err != nil {
		t.Fatalf("create replay workbench session: %v", err)
	}

	var out bytes.Buffer
	cmd := newRootCommand(&out, new(bytes.Buffer))
	cmd.SetArgs([]string{
		"replay-workbench-session-annotate", root, filepath.Base(session.Path),
		"--decision", "deferred",
		"--reason", "needs more evidence",
		"--reviewer", "alice",
		"--reviewed-at", "2026-06-01T02:35:00Z",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute replay-workbench-session-annotate: %v", err)
	}
	for _, want := range []string{
		"replay-workbench-session: status=saved mode=metadata-only primary=" + run.RunID,
		"replay-session-annotation: decision=deferred reviewer=alice reviewed_at=2026-06-01T02:35:00Z reason=needs more evidence",
	} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("expected annotation output to contain %q, got %q", want, out.String())
		}
	}

	var listOut bytes.Buffer
	listCmd := newRootCommand(&listOut, new(bytes.Buffer))
	listCmd.SetArgs([]string{"replay-workbench-sessions", root})
	if err := listCmd.Execute(); err != nil {
		t.Fatalf("execute replay-workbench-sessions list: %v", err)
	}
	if !strings.Contains(listOut.String(), "replay-session-annotation: decision=deferred reviewer=alice reviewed_at=2026-06-01T02:35:00Z reason=needs more evidence") {
		t.Fatalf("expected replay-workbench-sessions list to show annotation, got %q", listOut.String())
	}
}

func TestReplayWorkbenchCollaborationAnnotateCommandWritesMetadataOnlyDecision(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := app.InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	run, err := app.RunWorkspace(context.Background(), root)
	if err != nil {
		t.Fatalf("run workspace: %v", err)
	}
	session, err := app.CreateReplayWorkbenchSession(context.Background(), root, app.ReplayWorkbenchSessionOptions{
		PrimaryRunID: run.RunID,
		Reason:       "saved collaboration baseline",
	})
	if err != nil {
		t.Fatalf("create replay workbench session: %v", err)
	}
	collaboration, err := app.CreateReplayWorkbenchCollaboration(context.Background(), root, app.ReplayWorkbenchCollaborationOptions{
		SessionPath:  filepath.Base(session.Path),
		Participants: []string{"alice", "bob"},
		CreatedBy:    "lead",
		Reason:       "pair review",
	})
	if err != nil {
		t.Fatalf("create replay workbench collaboration: %v", err)
	}

	var out bytes.Buffer
	cmd := newRootCommand(&out, new(bytes.Buffer))
	cmd.SetArgs([]string{
		"replay-workbench-collaboration-annotate", root, filepath.Base(collaboration.Path),
		"--decision", "accepted",
		"--reason", "authorized pair review",
		"--reviewer", "alice",
		"--reviewed-at", "2026-06-01T05:10:00Z",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute replay-workbench-collaboration-annotate: %v", err)
	}
	for _, want := range []string{
		"replay-workbench-collaboration: status=saved mode=metadata-only session=" + session.Path,
		"replay-collaboration-annotation: decision=accepted reviewer=alice reviewed_at=2026-06-01T05:10:00Z reason=authorized pair review",
	} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("expected collaboration annotation output to contain %q, got %q", want, out.String())
		}
	}

	var listOut bytes.Buffer
	listCmd := newRootCommand(&listOut, new(bytes.Buffer))
	listCmd.SetArgs([]string{"replay-workbench-collaborations", root})
	if err := listCmd.Execute(); err != nil {
		t.Fatalf("execute replay-workbench-collaborations list: %v", err)
	}
	if !strings.Contains(listOut.String(), "replay-collaboration-annotation: decision=accepted reviewer=alice reviewed_at=2026-06-01T05:10:00Z reason=authorized pair review") {
		t.Fatalf("expected replay-workbench-collaborations list to show annotation, got %q", listOut.String())
	}
}

func TestReplayWorkbenchCollaborationSelectionCommandsSelectReadAndClearMetadataOnly(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := app.InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	run, err := app.RunWorkspace(context.Background(), root)
	if err != nil {
		t.Fatalf("run workspace: %v", err)
	}
	session, err := app.CreateReplayWorkbenchSession(context.Background(), root, app.ReplayWorkbenchSessionOptions{
		PrimaryRunID: run.RunID,
		Filters:      map[string]string{"lane": "timeline"},
		Reason:       "saved collaboration baseline",
	})
	if err != nil {
		t.Fatalf("create replay workbench session: %v", err)
	}
	base, err := app.CreateReplayWorkbenchCollaboration(context.Background(), root, app.ReplayWorkbenchCollaborationOptions{
		SessionPath:  filepath.Base(session.Path),
		Participants: []string{"alice"},
		CreatedAt:    time.Date(2026, 6, 1, 3, 50, 0, 0, time.UTC),
		Reason:       "baseline pair review",
	})
	if err != nil {
		t.Fatalf("create baseline collaboration: %v", err)
	}
	candidate, err := app.CreateReplayWorkbenchCollaboration(context.Background(), root, app.ReplayWorkbenchCollaborationOptions{
		SessionPath:  filepath.Base(session.Path),
		Participants: []string{"alice", "bob"},
		CreatedBy:    "lead",
		CreatedAt:    time.Date(2026, 6, 1, 3, 51, 0, 0, time.UTC),
		Reason:       "current pair review",
	})
	if err != nil {
		t.Fatalf("create candidate collaboration: %v", err)
	}

	var selectOut bytes.Buffer
	selectCmd := newRootCommand(&selectOut, new(bytes.Buffer))
	selectCmd.SetArgs([]string{
		"replay-workbench-collaboration-select", root, filepath.Base(candidate.Path),
		"--selected-by", "alice",
		"--selected-at", "2026-06-01T03:52:00Z",
		"--reason", "current collaboration focus",
	})
	if err := selectCmd.Execute(); err != nil {
		t.Fatalf("execute replay-workbench-collaboration-select: %v", err)
	}
	for _, want := range []string{
		"replay-workbench-collaboration-selection: status=current mode=metadata-only collaboration=" + candidate.Path,
		"session=" + session.Path + " primary=" + run.RunID + " selected=" + run.RunID + " runs=1 filters=1 endpoints=10 participants=2 superseded=1 selected_by=alice selected_at=2026-06-01T03:52:00Z reason=current collaboration focus",
		"replay-collaboration-superseded: " + base.Path,
	} {
		if !strings.Contains(selectOut.String(), want) {
			t.Fatalf("expected collaboration selection output to contain %q, got %q", want, selectOut.String())
		}
	}

	var readOut bytes.Buffer
	readCmd := newRootCommand(&readOut, new(bytes.Buffer))
	readCmd.SetArgs([]string{"replay-workbench-collaboration-selection", root})
	if err := readCmd.Execute(); err != nil {
		t.Fatalf("execute replay-workbench-collaboration-selection: %v", err)
	}
	if !strings.Contains(readOut.String(), "replay-workbench-collaboration-selection: status=current mode=metadata-only collaboration="+candidate.Path) {
		t.Fatalf("expected collaboration selection read output, got %q", readOut.String())
	}

	var clearOut bytes.Buffer
	clearCmd := newRootCommand(&clearOut, new(bytes.Buffer))
	clearCmd.SetArgs([]string{
		"replay-workbench-collaboration-selection-clear", root,
		"--cleared-by", "bob",
		"--cleared-at", "2026-06-01T03:55:00Z",
		"--reason", "collaboration archived",
	})
	if err := clearCmd.Execute(); err != nil {
		t.Fatalf("execute replay-workbench-collaboration-selection-clear: %v", err)
	}
	if !strings.Contains(clearOut.String(), "replay-workbench-collaboration-selection: status=cleared mode=metadata-only collaboration="+candidate.Path) ||
		!strings.Contains(clearOut.String(), "cleared_by=bob cleared_at=2026-06-01T03:55:00Z reason=collaboration archived") {
		t.Fatalf("expected cleared collaboration selection output, got %q", clearOut.String())
	}
}

func TestReplayWorkbenchSessionSelectionCommandsSelectReadAndClearMetadataOnly(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := app.InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	run, err := app.RunWorkspace(context.Background(), root)
	if err != nil {
		t.Fatalf("run workspace: %v", err)
	}
	base, err := app.CreateReplayWorkbenchSession(context.Background(), root, app.ReplayWorkbenchSessionOptions{
		PrimaryRunID: run.RunID,
		CreatedAt:    time.Date(2026, 6, 1, 2, 40, 0, 0, time.UTC),
		Reason:       "baseline saved view",
	})
	if err != nil {
		t.Fatalf("create baseline session: %v", err)
	}
	candidate, err := app.CreateReplayWorkbenchSession(context.Background(), root, app.ReplayWorkbenchSessionOptions{
		PrimaryRunID: run.RunID,
		Filters:      map[string]string{"lane": "toolchain"},
		CreatedAt:    time.Date(2026, 6, 1, 2, 41, 0, 0, time.UTC),
		Reason:       "candidate saved view",
	})
	if err != nil {
		t.Fatalf("create candidate session: %v", err)
	}

	var selectOut bytes.Buffer
	selectCmd := newRootCommand(&selectOut, new(bytes.Buffer))
	selectCmd.SetArgs([]string{
		"replay-workbench-session-select", root, filepath.Base(candidate.Path),
		"--selected-by", "alice",
		"--selected-at", "2026-06-01T02:42:00Z",
		"--reason", "current investigation focus",
	})
	if err := selectCmd.Execute(); err != nil {
		t.Fatalf("execute replay-workbench-session-select: %v", err)
	}
	for _, want := range []string{
		"replay-workbench-session-selection: status=current mode=metadata-only session=" + candidate.Path,
		"primary=" + run.RunID + " selected=" + run.RunID + " runs=1 filters=1 endpoints=8 superseded=1 selected_by=alice selected_at=2026-06-01T02:42:00Z reason=current investigation focus",
		"replay-session-superseded: " + base.Path,
	} {
		if !strings.Contains(selectOut.String(), want) {
			t.Fatalf("expected selection output to contain %q, got %q", want, selectOut.String())
		}
	}

	var readOut bytes.Buffer
	readCmd := newRootCommand(&readOut, new(bytes.Buffer))
	readCmd.SetArgs([]string{"replay-workbench-session-selection", root})
	if err := readCmd.Execute(); err != nil {
		t.Fatalf("execute replay-workbench-session-selection: %v", err)
	}
	if !strings.Contains(readOut.String(), "replay-workbench-session-selection: status=current mode=metadata-only session="+candidate.Path) {
		t.Fatalf("expected selection read output, got %q", readOut.String())
	}

	var clearOut bytes.Buffer
	clearCmd := newRootCommand(&clearOut, new(bytes.Buffer))
	clearCmd.SetArgs([]string{
		"replay-workbench-session-selection-clear", root,
		"--cleared-by", "bob",
		"--cleared-at", "2026-06-01T02:45:00Z",
		"--reason", "investigation archived",
	})
	if err := clearCmd.Execute(); err != nil {
		t.Fatalf("execute replay-workbench-session-selection-clear: %v", err)
	}
	if !strings.Contains(clearOut.String(), "replay-workbench-session-selection: status=cleared mode=metadata-only session="+candidate.Path) ||
		!strings.Contains(clearOut.String(), "cleared_by=bob cleared_at=2026-06-01T02:45:00Z reason=investigation archived") {
		t.Fatalf("expected cleared selection output, got %q", clearOut.String())
	}
}

func TestReplayWorkbenchHandlerServesReadOnlyEvidenceGraph(t *testing.T) {
	primary := app.ReplayResult{
		RunID: "run-graph-primary",
		Evidence: evidence.Project(blackboard.ProjectState{
			Facts: []blackboard.Fact{
				{ID: "fact-target", Kind: "target", Statement: "https://example.test is in scope"},
				{ID: "fact-surface", Kind: "surface", Statement: "admin route discovered"},
			},
			Intents: []blackboard.Intent{
				{
					ID:          "intent-admin-check",
					Kind:        "explore",
					Goal:        "test admin route",
					SourceFacts: []string{"fact-target", "fact-surface"},
				},
			},
		}),
	}
	comparison := app.ReplayResult{
		RunID: "run-graph-comparison",
		Evidence: evidence.Project(blackboard.ProjectState{
			Facts: []blackboard.Fact{
				{ID: "fact-api", Kind: "surface", Statement: "api route discovered"},
			},
		}),
	}
	handler := newReplayWorkbenchHandler(primary, comparison)

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/graph", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected graph API status 200, got %d body=%q", recorder.Code, recorder.Body.String())
	}
	graphBody := append([]byte(nil), recorder.Body.Bytes()...)
	var graph replayGraphExport
	if err := json.NewDecoder(bytes.NewReader(graphBody)).Decode(&graph); err != nil {
		t.Fatalf("decode graph API: %v", err)
	}
	if graph.RunID != primary.RunID || graph.NodeCount != 3 || graph.EdgeCount != 2 {
		t.Fatalf("unexpected graph summary: %#v", graph)
	}
	if graph.NodeTypeCounts["Target"] != 1 || graph.NodeTypeCounts["Surface"] != 1 || graph.NodeTypeCounts["Hypothesis"] != 1 || graph.EdgeTypeCounts["requires"] != 2 {
		t.Fatalf("unexpected graph type counts: nodes=%#v edges=%#v", graph.NodeTypeCounts, graph.EdgeTypeCounts)
	}
	assertReplayGraphNode(t, graph.Nodes, "fact-target", "Target", "https://example.test is in scope")
	assertReplayGraphNode(t, graph.Nodes, "intent-admin-check", "Hypothesis", "test admin route")
	assertReplayGraphEdge(t, graph.Edges, "intent-admin-check", "fact-target", "requires")
	edgeID := replayGraphEdgeIDFromJSON(t, graphBody, "intent-admin-check", "fact-target", "requires")

	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/graph?node=fact-target", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected focused graph API status 200, got %d body=%q", recorder.Code, recorder.Body.String())
	}
	focusedBody := append([]byte(nil), recorder.Body.Bytes()...)
	var focusedGraph replayGraphExport
	if err := json.NewDecoder(bytes.NewReader(focusedBody)).Decode(&focusedGraph); err != nil {
		t.Fatalf("decode focused graph API: %v", err)
	}
	if focusedGraph.RunID != primary.RunID || focusedGraph.NodeCount != 2 || focusedGraph.EdgeCount != 1 {
		t.Fatalf("unexpected focused graph summary: %#v", focusedGraph)
	}
	nodeFocus := replayGraphFocusFromJSON(t, focusedBody)
	if nodeFocus.Kind != "node" || nodeFocus.ID != "fact-target" || nodeFocus.NodeID != "fact-target" || nodeFocus.NodeCount != 2 || nodeFocus.EdgeCount != 1 {
		t.Fatalf("unexpected node graph focus metadata: %#v", nodeFocus)
	}
	if focusedGraph.NodeTypeCounts["Target"] != 1 || focusedGraph.NodeTypeCounts["Hypothesis"] != 1 || focusedGraph.NodeTypeCounts["Surface"] != 0 || focusedGraph.EdgeTypeCounts["requires"] != 1 {
		t.Fatalf("unexpected focused graph type counts: nodes=%#v edges=%#v", focusedGraph.NodeTypeCounts, focusedGraph.EdgeTypeCounts)
	}
	assertReplayGraphNode(t, focusedGraph.Nodes, "fact-target", "Target", "https://example.test is in scope")
	assertReplayGraphNode(t, focusedGraph.Nodes, "intent-admin-check", "Hypothesis", "test admin route")
	assertReplayGraphEdge(t, focusedGraph.Edges, "intent-admin-check", "fact-target", "requires")
	if replayGraphHasNode(focusedGraph.Nodes, "fact-surface") {
		t.Fatalf("focused graph should not include unrelated one-hop nodes: %#v", focusedGraph.Nodes)
	}

	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/graph?edge="+url.QueryEscape(edgeID), nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected edge-focused graph API status 200, got %d body=%q", recorder.Code, recorder.Body.String())
	}
	edgeFocusedBody := append([]byte(nil), recorder.Body.Bytes()...)
	var edgeFocusedGraph replayGraphExport
	if err := json.NewDecoder(bytes.NewReader(edgeFocusedBody)).Decode(&edgeFocusedGraph); err != nil {
		t.Fatalf("decode edge-focused graph API: %v", err)
	}
	if edgeFocusedGraph.RunID != primary.RunID || edgeFocusedGraph.NodeCount != 2 || edgeFocusedGraph.EdgeCount != 1 {
		t.Fatalf("unexpected edge-focused graph summary: %#v", edgeFocusedGraph)
	}
	edgeFocus := replayGraphFocusFromJSON(t, edgeFocusedBody)
	if edgeFocus.Kind != "edge" || edgeFocus.ID != edgeID || edgeFocus.EdgeID != edgeID || edgeFocus.From != "intent-admin-check" || edgeFocus.To != "fact-target" || edgeFocus.Type != "requires" || edgeFocus.NodeCount != 2 || edgeFocus.EdgeCount != 1 {
		t.Fatalf("unexpected edge graph focus metadata: %#v", edgeFocus)
	}
	assertReplayGraphNode(t, edgeFocusedGraph.Nodes, "fact-target", "Target", "https://example.test is in scope")
	assertReplayGraphNode(t, edgeFocusedGraph.Nodes, "intent-admin-check", "Hypothesis", "test admin route")
	assertReplayGraphEdge(t, edgeFocusedGraph.Edges, "intent-admin-check", "fact-target", "requires")
	if edgeFocusedGraph.EdgeTypeCounts["requires"] != 1 || edgeFocusedGraph.NodeTypeCounts["Target"] != 1 || edgeFocusedGraph.NodeTypeCounts["Hypothesis"] != 1 {
		t.Fatalf("unexpected edge-focused graph type counts: nodes=%#v edges=%#v", edgeFocusedGraph.NodeTypeCounts, edgeFocusedGraph.EdgeTypeCounts)
	}

	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/graph.ndjson?edge="+url.QueryEscape(edgeID), nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected graph ndjson API status 200, got %d body=%q", recorder.Code, recorder.Body.String())
	}
	if got := recorder.Header().Get("Content-Type"); !strings.Contains(got, "application/x-ndjson") {
		t.Fatalf("expected graph ndjson content type, got %q", got)
	}
	graphLines := strings.Split(strings.TrimSpace(recorder.Body.String()), "\n")
	if len(graphLines) != 4 {
		t.Fatalf("expected graph ndjson summary plus two nodes and one edge, got %d lines: %q", len(graphLines), recorder.Body.String())
	}
	var summaryLine struct {
		Kind      string                  `json:"kind"`
		RunID     string                  `json:"run_id"`
		NodeCount int                     `json:"node_count"`
		EdgeCount int                     `json:"edge_count"`
		Focus     replayGraphFocusPayload `json:"focus"`
	}
	if err := json.Unmarshal([]byte(graphLines[0]), &summaryLine); err != nil {
		t.Fatalf("decode graph ndjson summary: %v line=%q", err, graphLines[0])
	}
	if summaryLine.Kind != "summary" || summaryLine.RunID != primary.RunID || summaryLine.NodeCount != 2 || summaryLine.EdgeCount != 1 {
		t.Fatalf("unexpected graph ndjson summary line: %#v", summaryLine)
	}
	if summaryLine.Focus.Kind != "edge" || summaryLine.Focus.ID != edgeID {
		t.Fatalf("unexpected graph ndjson focus metadata: %#v", summaryLine.Focus)
	}
	if !strings.Contains(recorder.Body.String(), `"kind":"edge"`) || !strings.Contains(recorder.Body.String(), `"id":"`+edgeID+`"`) {
		t.Fatalf("expected graph ndjson edge record with stable id, got %q", recorder.Body.String())
	}

	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/graphs?q=api", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected graph list API status 200, got %d body=%q", recorder.Code, recorder.Body.String())
	}
	var graphs []replayGraphExport
	if err := json.NewDecoder(recorder.Body).Decode(&graphs); err != nil {
		t.Fatalf("decode graph list API: %v", err)
	}
	if len(graphs) != 2 {
		t.Fatalf("expected two graph exports after filtering, got %#v", graphs)
	}
	if graphs[0].RunID != primary.RunID || graphs[0].NodeCount != 0 || graphs[0].EdgeCount != 0 {
		t.Fatalf("expected primary graph to filter to zero api nodes, got %#v", graphs[0])
	}
	if graphs[1].RunID != comparison.RunID || graphs[1].NodeCount != 1 || graphs[1].Nodes[0].ID != "fact-api" {
		t.Fatalf("expected comparison graph to keep api node, got %#v", graphs[1])
	}

	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/session?run=run-graph-comparison&q=api", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected session API status 200, got %d body=%q", recorder.Code, recorder.Body.String())
	}
	var session struct {
		Endpoints map[string]string `json:"endpoints"`
	}
	if err := json.NewDecoder(recorder.Body).Decode(&session); err != nil {
		t.Fatalf("decode graph session API: %v", err)
	}
	if session.Endpoints["graph"] != "/api/graph?q=api" || session.Endpoints["graphs"] != "/api/graphs?q=api" {
		t.Fatalf("unexpected graph session endpoints: %#v", session.Endpoints)
	}

	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/session?run=run-graph-primary&node=fact-target", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected focused session API status 200, got %d body=%q", recorder.Code, recorder.Body.String())
	}
	var focusedSession struct {
		Filters   map[string]string `json:"filters"`
		Endpoints map[string]string `json:"endpoints"`
	}
	if err := json.NewDecoder(recorder.Body).Decode(&focusedSession); err != nil {
		t.Fatalf("decode focused graph session API: %v", err)
	}
	if focusedSession.Filters["node"] != "fact-target" || focusedSession.Endpoints["graph"] != "/api/graph?node=fact-target" {
		t.Fatalf("unexpected focused graph session: %#v", focusedSession)
	}

	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/session?run=run-graph-primary&edge="+url.QueryEscape(edgeID), nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected edge-focused session API status 200, got %d body=%q", recorder.Code, recorder.Body.String())
	}
	var edgeFocusedSession struct {
		Filters   map[string]string `json:"filters"`
		Endpoints map[string]string `json:"endpoints"`
	}
	if err := json.NewDecoder(recorder.Body).Decode(&edgeFocusedSession); err != nil {
		t.Fatalf("decode edge-focused graph session API: %v", err)
	}
	expectedGraphEndpoint := "/api/graph?edge=" + url.QueryEscape(edgeID)
	expectedGraphNDJSONEndpoint := "/api/graph.ndjson?edge=" + url.QueryEscape(edgeID)
	if edgeFocusedSession.Filters["edge"] != edgeID || edgeFocusedSession.Endpoints["graph"] != expectedGraphEndpoint || edgeFocusedSession.Endpoints["graph_ndjson"] != expectedGraphNDJSONEndpoint {
		t.Fatalf("unexpected edge-focused graph session: filters=%#v endpoints=%#v", edgeFocusedSession.Filters, edgeFocusedSession.Endpoints)
	}

	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/?node=fact-target&edge="+url.QueryEscape(edgeID), nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected graph workbench html status 200, got %d body=%q", recorder.Code, recorder.Body.String())
	}
	for _, want := range []string{
		"window.OURO_REPLAY_GRAPHS",
		"id=\"ouro-evidence-graph\"",
		"id=\"ouro-evidence-summary\"",
		"id=\"ouro-evidence-detail\"",
		"id=\"ouro-graph-node-filter\"",
		"id=\"ouro-graph-edge-filter\"",
		"filterReplayGraphByNode",
		"filterReplayGraphByEdge",
		"renderReplayGraphDetail",
		"renderReplayGraphSummary",
		"renderReplayGraph",
		"Evidence Graph",
		"graph detail",
		`"node":"fact-target"`,
		`"edge":"` + edgeID + `"`,
	} {
		if !strings.Contains(recorder.Body.String(), want) {
			t.Fatalf("expected graph workbench html to contain %q, got %q", want, recorder.Body.String())
		}
	}
}

func assertReplayGraphNode(t *testing.T, nodes []replayGraphNode, id string, nodeType string, label string) {
	t.Helper()
	for _, node := range nodes {
		if node.ID == id {
			if node.Type != nodeType || node.Label != label {
				t.Fatalf("unexpected graph node %s: %#v", id, node)
			}
			return
		}
	}
	t.Fatalf("expected graph node %s in %#v", id, nodes)
}

func replayGraphHasNode(nodes []replayGraphNode, id string) bool {
	for _, node := range nodes {
		if node.ID == id {
			return true
		}
	}
	return false
}

type replayGraphFocusPayload struct {
	Kind      string `json:"kind"`
	ID        string `json:"id"`
	NodeID    string `json:"node_id"`
	EdgeID    string `json:"edge_id"`
	From      string `json:"from"`
	To        string `json:"to"`
	Type      string `json:"type"`
	NodeCount int    `json:"node_count"`
	EdgeCount int    `json:"edge_count"`
}

func replayGraphFocusFromJSON(t *testing.T, data []byte) replayGraphFocusPayload {
	t.Helper()
	var payload struct {
		Focus replayGraphFocusPayload `json:"focus"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("decode graph focus metadata: %v", err)
	}
	return payload.Focus
}

func replayGraphEdgeIDFromJSON(t *testing.T, data []byte, from string, to string, edgeType string) string {
	t.Helper()
	var payload struct {
		Edges []struct {
			ID   string `json:"id"`
			From string `json:"from"`
			To   string `json:"to"`
			Type string `json:"type"`
		} `json:"edges"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("decode graph edge ids: %v", err)
	}
	for _, edge := range payload.Edges {
		if edge.From == from && edge.To == to && edge.Type == edgeType {
			if edge.ID == "" {
				t.Fatalf("expected graph edge %s -> %s (%s) to expose stable id in %q", from, to, edgeType, string(data))
			}
			return edge.ID
		}
	}
	t.Fatalf("expected graph edge %s -> %s (%s) in %q", from, to, edgeType, string(data))
	return ""
}

func assertReplayGraphEdge(t *testing.T, edges []replayGraphEdge, from string, to string, edgeType string) {
	t.Helper()
	for _, edge := range edges {
		if edge.From == from && edge.To == to && edge.Type == edgeType {
			return
		}
	}
	t.Fatalf("expected graph edge %s -> %s (%s) in %#v", from, to, edgeType, edges)
}

func drainReplayWorkbench(client http.Client, addr string, paths ...string) {
	for _, path := range paths {
		response, err := client.Get("http://" + addr + path)
		if err == nil && response.Body != nil {
			_ = response.Body.Close()
		}
	}
}

func TestReplayWorkbenchHandlerRejectsMutationMethods(t *testing.T) {
	handler := newReplayWorkbenchHandler(app.ReplayResult{RunID: "run-readonly"})

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/", strings.NewReader("{}")))
	if recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected POST rejected, got status=%d body=%q", recorder.Code, recorder.Body.String())
	}
	if strings.Contains(recorder.Body.String(), "window.OURO_REPLAY") {
		t.Fatalf("mutation response must not return workbench html, got %q", recorder.Body.String())
	}
}

func TestReplayCommandCanLaunchInteractiveTUI(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := app.InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}

	var runOut bytes.Buffer
	runCmd := newRootCommand(&runOut, new(bytes.Buffer))
	runCmd.SetArgs([]string{"run", root})
	if err := runCmd.Execute(); err != nil {
		t.Fatalf("execute run: %v", err)
	}
	runID := parseOutputValue(t, runOut.String(), "run:")

	session := &fakeInteractiveSession{}
	restore := replaceInteractiveSession(t, session)
	defer restore()

	var replayOut bytes.Buffer
	replayCmd := newRootCommand(&replayOut, new(bytes.Buffer))
	replayCmd.SetArgs([]string{"replay", root, runID, "--interactive"})
	if err := replayCmd.Execute(); err != nil {
		t.Fatalf("execute interactive replay: %v", err)
	}
	for _, want := range []string{"replay:", "report:"} {
		if !strings.Contains(replayOut.String(), want) {
			t.Fatalf("expected replay summary to contain %q, got %q", want, replayOut.String())
		}
	}
	for _, want := range []string{"Workspace", "Mission Conversation", "Attack Graph", "Runtime Tabs", "model:offline-model"} {
		if !strings.Contains(session.initialView, want) {
			t.Fatalf("expected interactive TUI view to contain %q, got:\n%s", want, session.initialView)
		}
	}
	if session.initialCursor == 0 || session.initialCursor != session.initialCursorTotal {
		t.Fatalf("expected replay interactive model to start at final cursor, got %d/%d", session.initialCursor, session.initialCursorTotal)
	}
}

func TestReplayCommandCanPrintTUISnapshot(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := app.InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}

	var runOut bytes.Buffer
	runCmd := newRootCommand(&runOut, new(bytes.Buffer))
	runCmd.SetArgs([]string{"run", root})
	if err := runCmd.Execute(); err != nil {
		t.Fatalf("execute run: %v", err)
	}
	runID := parseOutputValue(t, runOut.String(), "run:")

	var replayOut bytes.Buffer
	replayCmd := newRootCommand(&replayOut, new(bytes.Buffer))
	replayCmd.SetArgs([]string{"replay", root, runID, "--tui"})
	if err := replayCmd.Execute(); err != nil {
		t.Fatalf("execute replay with tui snapshot: %v", err)
	}
	for _, want := range []string{"Workspace", "Mission Conversation", "Attack Graph", "Runtime Tabs", "model:offline-model"} {
		if !strings.Contains(replayOut.String(), want) {
			t.Fatalf("expected TUI replay output to contain %q, got %q", want, replayOut.String())
		}
	}
}

func TestReplayCommandShowsFailedRunWithoutGeneratedReport(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := app.InitWorkspace(root); err != nil {
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
	_, err := app.RunWorkspaceWithOptions(context.Background(), root, app.RunOptions{
		OnEvent: func(event eventlog.Event) error {
			observedEvents = append(observedEvents, event)
			return nil
		},
	})
	if err == nil {
		t.Fatalf("expected failed run")
	}
	if len(observedEvents) == 0 {
		t.Fatalf("expected failed run to persist events")
	}

	var replayOut bytes.Buffer
	replayCmd := newRootCommand(&replayOut, new(bytes.Buffer))
	replayCmd.SetArgs([]string{"replay", root, observedEvents[0].RunID})
	if err := replayCmd.Execute(); err != nil {
		t.Fatalf("execute replay: %v", err)
	}
	for _, want := range []string{"tools: failed", "report: not-generated"} {
		if !strings.Contains(replayOut.String(), want) {
			t.Fatalf("expected failed replay output to contain %q, got %q", want, replayOut.String())
		}
	}
}

func TestToolchainCommandsInspectWorkspaceManifests(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := app.InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}

	var listOut bytes.Buffer
	listCmd := newRootCommand(&listOut, new(bytes.Buffer))
	listCmd.SetArgs([]string{"toolchain", "list", root})
	if err := listCmd.Execute(); err != nil {
		t.Fatalf("execute toolchain list: %v", err)
	}
	for _, want := range []string{"web-api-pack", "infra-pack", "ctf-pack"} {
		if !strings.Contains(listOut.String(), want) {
			t.Fatalf("expected list output to contain %q, got %q", want, listOut.String())
		}
	}

	var healthOut bytes.Buffer
	healthCmd := newRootCommand(&healthOut, new(bytes.Buffer))
	healthCmd.SetArgs([]string{"toolchain", "health", root})
	if err := healthCmd.Execute(); err != nil {
		t.Fatalf("execute toolchain health: %v", err)
	}
	if !strings.Contains(healthOut.String(), "ready") || !strings.Contains(healthOut.String(), "web-api-pack") {
		t.Fatalf("expected health output, got %q", healthOut.String())
	}
}

func TestToolchainAuthoredToolCommandsValidateWriteListAndRead(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := app.InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	specPath := filepath.Join(t.TempDir(), "authored-tool.yaml")
	if err := os.WriteFile(specPath, []byte(validAuthoredToolSpecYAMLForCLITest()), 0o644); err != nil {
		t.Fatalf("write authored spec: %v", err)
	}

	var validateOut bytes.Buffer
	validateCmd := newRootCommand(&validateOut, new(bytes.Buffer))
	validateCmd.SetArgs([]string{"toolchain", "authored-tool-validate", specPath})
	if err := validateCmd.Execute(); err != nil {
		t.Fatalf("validate authored tool: %v", err)
	}
	for _, want := range []string{"authored-tool: status=valid name=web-login-state-enumerator", "adapter=managed_binary", "operation=http_probe"} {
		if !strings.Contains(validateOut.String(), want) {
			t.Fatalf("expected validate output to contain %q, got %q", want, validateOut.String())
		}
	}

	var writeOut bytes.Buffer
	writeCmd := newRootCommand(&writeOut, new(bytes.Buffer))
	writeCmd.SetArgs([]string{"toolchain", "authored-tool-write", root, specPath})
	if err := writeCmd.Execute(); err != nil {
		t.Fatalf("write authored tool: %v", err)
	}
	for _, want := range []string{"authored-tool: status=written name=web-login-state-enumerator", "adapter=managed_binary", "operation=http_probe"} {
		if !strings.Contains(writeOut.String(), want) {
			t.Fatalf("expected write output to contain %q, got %q", want, writeOut.String())
		}
	}

	var listOut bytes.Buffer
	listCmd := newRootCommand(&listOut, new(bytes.Buffer))
	listCmd.SetArgs([]string{"toolchain", "authored-tools", root})
	if err := listCmd.Execute(); err != nil {
		t.Fatalf("list authored tools: %v", err)
	}
	for _, want := range []string{"authored-tool: name=web-login-state-enumerator", "adapter=managed_binary", "operation=http_probe"} {
		if !strings.Contains(listOut.String(), want) {
			t.Fatalf("expected list output to contain %q, got %q", want, listOut.String())
		}
	}

	var readOut bytes.Buffer
	readCmd := newRootCommand(&readOut, new(bytes.Buffer))
	readCmd.SetArgs([]string{"toolchain", "authored-tools", root, "web-login-state-enumerator"})
	if err := readCmd.Execute(); err != nil {
		t.Fatalf("read authored tool: %v", err)
	}
	for _, want := range []string{"authored-tool: name=web-login-state-enumerator", "parser=web_login_state_parser", "adapter=managed_binary", "operation=http_probe", "manifest="} {
		if !strings.Contains(readOut.String(), want) {
			t.Fatalf("expected read output to contain %q, got %q", want, readOut.String())
		}
	}
}

func TestToolchainAuthoredToolValidateRejectsRawScript(t *testing.T) {
	specPath := filepath.Join(t.TempDir(), "bad-authored-tool.yaml")
	if err := os.WriteFile(specPath, []byte(validAuthoredToolSpecYAMLForCLITest()+"raw_script: nmap -sV $TARGET\n"), 0o644); err != nil {
		t.Fatalf("write authored spec: %v", err)
	}

	var out bytes.Buffer
	cmd := newRootCommand(&out, new(bytes.Buffer))
	cmd.SetArgs([]string{"toolchain", "authored-tool-validate", specPath})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "raw_script") {
		t.Fatalf("expected raw script validation error, got %v output=%q", err, out.String())
	}
}

func TestToolchainAuthoredToolValidateRejectsUnknownFieldAndDependencyCommand(t *testing.T) {
	unknownPath := filepath.Join(t.TempDir(), "unknown-authored-tool.yaml")
	if err := os.WriteFile(unknownPath, []byte(validAuthoredToolSpecYAMLForCLITest()+"scriptlet: nuclei -u $TARGET\n"), 0o644); err != nil {
		t.Fatalf("write unknown authored spec: %v", err)
	}
	var unknownOut bytes.Buffer
	unknownCmd := newRootCommand(&unknownOut, new(bytes.Buffer))
	unknownCmd.SetArgs([]string{"toolchain", "authored-tool-validate", unknownPath})
	err := unknownCmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "scriptlet") {
		t.Fatalf("expected unknown field validation error, got %v output=%q", err, unknownOut.String())
	}

	commandPath := filepath.Join(t.TempDir(), "command-authored-tool.yaml")
	commandSpec := strings.Replace(validAuthoredToolSpecYAMLForCLITest(), "install_hint: install managed httpx\n", "install_hint: install managed httpx\n    install_command: [ouro-toolchain, install, httpx]\n", 1)
	if err := os.WriteFile(commandPath, []byte(commandSpec), 0o644); err != nil {
		t.Fatalf("write command authored spec: %v", err)
	}
	var commandOut bytes.Buffer
	commandCmd := newRootCommand(&commandOut, new(bytes.Buffer))
	commandCmd.SetArgs([]string{"toolchain", "authored-tool-validate", commandPath})
	err = commandCmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "install_command") {
		t.Fatalf("expected dependency command validation error, got %v output=%q", err, commandOut.String())
	}
}

func TestToolchainAuthoredToolRegistryCommandPrintsContracts(t *testing.T) {
	var out bytes.Buffer
	cmd := newRootCommand(&out, new(bytes.Buffer))
	cmd.SetArgs([]string{"toolchain", "authored-tool-registry"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute authored tool registry command: %v", err)
	}
	output := out.String()
	for _, want := range []string{
		"adapter: id=managed_binary",
		"operation: adapter=managed_binary id=http_probe",
		"operation: adapter=managed_binary id=template_scan",
		"operation: adapter=managed_binary id=apk_decompile",
		"parser: id=httpx_jsonl",
		"parser: id=nmap_xml",
		"parser: id=semgrep_sarif",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected registry output to contain %q, got %q", want, output)
		}
	}
}

func TestToolchainManagedBundleCommandsListPlanAndInstall(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := app.InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	marker := filepath.Join(t.TempDir(), "managed-payload-ran")
	payload := writeManagedBundleFixtureForCLITest(t, root, "httpx", "#!/bin/sh\ntouch "+marker+"\n")

	var listOut bytes.Buffer
	listCmd := newRootCommand(&listOut, new(bytes.Buffer))
	listCmd.SetArgs([]string{"toolchain", "managed-bundles", root})
	if err := listCmd.Execute(); err != nil {
		t.Fatalf("execute managed-bundles: %v", err)
	}
	for _, want := range []string{"managed-bundle: name=httpx", "kind=binary", "payload=" + payload, "install=" + filepath.Join(root, "toolchain", "bin", "httpx")} {
		if !strings.Contains(listOut.String(), want) {
			t.Fatalf("expected managed-bundles output to contain %q, got %q", want, listOut.String())
		}
	}

	var planOut bytes.Buffer
	planCmd := newRootCommand(&planOut, new(bytes.Buffer))
	planCmd.SetArgs([]string{"toolchain", "managed-install-plan", root, "web-api-pack", "httpx"})
	if err := planCmd.Execute(); err != nil {
		t.Fatalf("execute managed-install-plan: %v", err)
	}
	for _, want := range []string{"managed-install-plan: status=ready", "pack=web-api-pack", "dependency=httpx", "action=link", "payload=" + payload} {
		if !strings.Contains(planOut.String(), want) {
			t.Fatalf("expected managed-install-plan output to contain %q, got %q", want, planOut.String())
		}
	}
	if _, err := os.Lstat(filepath.Join(root, "toolchain", "bin", "httpx")); !os.IsNotExist(err) {
		t.Fatalf("managed-install-plan must not create link, stat err=%v", err)
	}

	var dryRunOut bytes.Buffer
	dryRunCmd := newRootCommand(&dryRunOut, new(bytes.Buffer))
	dryRunCmd.SetArgs([]string{"toolchain", "managed-install", root, "web-api-pack", "httpx"})
	err := dryRunCmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "--execute") {
		t.Fatalf("expected managed-install to require --execute, got %v output=%q", err, dryRunOut.String())
	}

	var installOut bytes.Buffer
	installCmd := newRootCommand(&installOut, new(bytes.Buffer))
	installCmd.SetArgs([]string{"toolchain", "managed-install", root, "web-api-pack", "httpx", "--execute"})
	if err := installCmd.Execute(); err != nil {
		t.Fatalf("execute managed-install: %v", err)
	}
	for _, want := range []string{"managed-install: status=installed", "pack=web-api-pack", "dependency=httpx", "install=" + filepath.Join(root, "toolchain", "bin", "httpx")} {
		if !strings.Contains(installOut.String(), want) {
			t.Fatalf("expected managed-install output to contain %q, got %q", want, installOut.String())
		}
	}
	if _, err := os.Lstat(filepath.Join(root, "toolchain", "bin", "httpx")); err != nil {
		t.Fatalf("expected managed install link: %v", err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("managed-install must link payload without running it, marker stat err=%v", err)
	}
}

func TestToolchainReleaseManagedInstallPlanAndInstallCommands(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := app.InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	releaseRoot := filepath.Join(t.TempDir(), "release")
	marker := filepath.Join(t.TempDir(), "release-managed-payload-ran")
	releasePayload := writeReleaseManagedBundleFixtureForCLITest(t, releaseRoot, "httpx", "#!/bin/sh\ntouch "+marker+"\n")
	workspacePayload := filepath.Join(root, "toolchain", "managed-bundles", "httpx", "payload", "httpx")
	installPath := filepath.Join(root, "toolchain", "bin", "httpx")

	var planOut bytes.Buffer
	planCmd := newRootCommand(&planOut, new(bytes.Buffer))
	planCmd.SetArgs([]string{"toolchain", "release-managed-install-plan", root, releaseRoot, "web-api-pack", "httpx"})
	if err := planCmd.Execute(); err != nil {
		t.Fatalf("execute release-managed-install-plan: %v", err)
	}
	for _, want := range []string{"release-managed-install-plan: status=ready", "action=copy-and-link", "pack=web-api-pack", "dependency=httpx", "release_payload=" + releasePayload, "workspace_payload=" + workspacePayload} {
		if !strings.Contains(planOut.String(), want) {
			t.Fatalf("expected release-managed-install-plan output to contain %q, got %q", want, planOut.String())
		}
	}
	if _, err := os.Lstat(workspacePayload); !os.IsNotExist(err) {
		t.Fatalf("release-managed-install-plan must not copy payload, stat err=%v", err)
	}

	var dryRunOut bytes.Buffer
	dryRunCmd := newRootCommand(&dryRunOut, new(bytes.Buffer))
	dryRunCmd.SetArgs([]string{"toolchain", "release-managed-install", root, releaseRoot, "web-api-pack", "httpx"})
	err := dryRunCmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "--execute") {
		t.Fatalf("expected release-managed-install to require --execute, got %v output=%q", err, dryRunOut.String())
	}
	if _, err := os.Lstat(workspacePayload); !os.IsNotExist(err) {
		t.Fatalf("release-managed-install without execute must not copy payload, stat err=%v", err)
	}

	var installOut bytes.Buffer
	installCmd := newRootCommand(&installOut, new(bytes.Buffer))
	installCmd.SetArgs([]string{"toolchain", "release-managed-install", root, releaseRoot, "web-api-pack", "httpx", "--execute"})
	if err := installCmd.Execute(); err != nil {
		t.Fatalf("execute release-managed-install: %v", err)
	}
	for _, want := range []string{"release-managed-install: status=installed", "mode=workspace-copy-link", "pack=web-api-pack", "dependency=httpx", "workspace_payload=" + workspacePayload, "install=" + installPath} {
		if !strings.Contains(installOut.String(), want) {
			t.Fatalf("expected release-managed-install output to contain %q, got %q", want, installOut.String())
		}
	}
	if _, err := os.Stat(workspacePayload); err != nil {
		t.Fatalf("expected workspace payload copy: %v", err)
	}
	if _, err := os.Lstat(installPath); err != nil {
		t.Fatalf("expected release managed install link: %v", err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("release-managed-install must not execute payload, marker stat err=%v", err)
	}
}

func writeManagedBundleFixtureForCLITest(t *testing.T, root string, name string, payloadBody string) string {
	t.Helper()
	bundleDir := filepath.Join(root, "toolchain", "managed-bundles", name)
	payloadDir := filepath.Join(bundleDir, "payload")
	if err := os.MkdirAll(payloadDir, 0o755); err != nil {
		t.Fatalf("create bundle payload dir: %v", err)
	}
	payloadPath := filepath.Join(payloadDir, name)
	if err := os.WriteFile(payloadPath, []byte(payloadBody), 0o755); err != nil {
		t.Fatalf("write bundle payload: %v", err)
	}
	digest := sha256.Sum256([]byte(payloadBody))
	manifest := `name: ` + name + `
kind: binary
version: "1.3.0"
source: projectdiscovery/` + name + `
payload: payload/` + name + `
sha256: ` + hex.EncodeToString(digest[:]) + `
`
	if err := os.WriteFile(filepath.Join(bundleDir, "bundle.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write bundle manifest: %v", err)
	}
	return payloadPath
}

func writeReleaseManagedBundleFixtureForCLITest(t *testing.T, releaseRoot string, name string, payloadBody string) string {
	t.Helper()
	bundleDir := filepath.Join(releaseRoot, "toolchains", "managed-bundles", name)
	payloadDir := filepath.Join(bundleDir, "payload")
	if err := os.MkdirAll(payloadDir, 0o755); err != nil {
		t.Fatalf("create release bundle payload dir: %v", err)
	}
	payloadPath := filepath.Join(payloadDir, name)
	if err := os.WriteFile(payloadPath, []byte(payloadBody), 0o755); err != nil {
		t.Fatalf("write release bundle payload: %v", err)
	}
	digest := sha256.Sum256([]byte(payloadBody))
	manifest := `name: ` + name + `
kind: binary
version: "1.3.0"
source: projectdiscovery/` + name + `
payload: payload/` + name + `
sha256: ` + hex.EncodeToString(digest[:]) + `
`
	if err := os.WriteFile(filepath.Join(bundleDir, "bundle.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write release bundle manifest: %v", err)
	}
	return payloadPath
}

func validAuthoredToolSpecYAMLForCLITest() string {
	return `name: web-login-state-enumerator
version: "0.1.0"
capabilities:
  - web.auth_state_tracking
  - evidence.collection
parser: web_login_state_parser
adapter:
  id: managed_binary
  operation: http_probe
inputs:
  - name: target_url
    type: url
    required: true
expected_evidence:
  - type: http_observation
    fields:
      - status_code
      - body_hash
safety:
  target_policy: enforce
  isolation: process
dependencies:
  - name: httpx
    kind: binary
    optional: true
    source: projectdiscovery/httpx
    install_hint: install managed httpx
`
}

func TestToolchainCompilePreviewCommandPrintsScoringMetadataOnly(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := app.InitWorkspace(root); err != nil {
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

	var out bytes.Buffer
	cmd := newRootCommand(&out, new(bytes.Buffer))
	cmd.SetArgs([]string{
		"toolchain", "compile-preview", root,
		"--domain-priority", "infra_internal=5",
		"--domain-priority", "ctf_lab=1",
		"--max-plan-nodes", "2",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute toolchain compile-preview: %v", err)
	}
	output := out.String()
	for _, want := range []string{
		"compile-preview: workspace=" + root + " action=discover_assets mode=metadata-only nodes=2 warnings=1",
		"compile-node: id=discover_assets:01 manifest=infra-pack tool=amass adapter=managed_binary operation=subdomain_discovery capability=infra.host_discovery",
		"score=5",
		"signals=domain:infra_internal=5",
		"compile-node: id=discover_assets:02 manifest=infra-pack tool=dnsrecon adapter=managed_binary operation=dns_probe capability=infra.host_discovery",
		"compile-warning: budget max_plan_nodes=2 truncated 7 candidates",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected compile-preview output to contain %q, got %q", want, output)
		}
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("compile-preview command must not execute run_command, marker stat err=%v", err)
	}
}

func TestToolchainCompilePreviewCommandUsesPersistedWorkspaceSignals(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := app.InitWorkspace(root); err != nil {
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
	signals := `{
  "schema_version": 1,
  "budget": {"max_plan_nodes": 1},
  "domain_priorities": [
    {"domain": "infra_internal", "weight": 3}
  ],
  "frontier_scores": [
    {
      "manifest_name": "infra-pack",
      "capability": "infra.host_discovery",
      "path": {"mission_value": 2, "unlock_potential": 1}
    }
  ],
  "historical_stats": [
    {"manifest_name": "infra-pack", "capability": "infra.host_discovery", "successes": 2},
    {"manifest_name": "web-api-pack", "capability": "web.discovery", "failures": 4}
  ]
}`
	if err := os.WriteFile(filepath.Join(root, "toolchain", "compile-preview-signals.json"), []byte(signals), 0o644); err != nil {
		t.Fatalf("write compile preview signals: %v", err)
	}

	var out bytes.Buffer
	cmd := newRootCommand(&out, new(bytes.Buffer))
	cmd.SetArgs([]string{"toolchain", "compile-preview", root})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute toolchain compile-preview: %v", err)
	}
	output := out.String()
	for _, want := range []string{
		"compile-preview: workspace=" + root + " action=discover_assets mode=metadata-only nodes=1 warnings=1",
		"compile-node: id=discover_assets:01 manifest=infra-pack tool=amass adapter=managed_binary operation=subdomain_discovery capability=infra.host_discovery",
		"score=8",
		"signals=domain:infra_internal=3,frontier=3,history:successes=2 failures=0",
		"compile-warning: budget max_plan_nodes=1 truncated 7 candidates",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected compile-preview output to contain %q, got %q", want, output)
		}
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("compile-preview command must not execute run_command, marker stat err=%v", err)
	}
}

func TestToolchainCompilePreviewReviewCommandWritesAndListsMetadataOnlyArtifacts(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := app.InitWorkspace(root); err != nil {
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

	var out bytes.Buffer
	cmd := newRootCommand(&out, new(bytes.Buffer))
	cmd.SetArgs([]string{
		"toolchain", "compile-preview-review", root,
		"--domain-priority", "infra_internal=4",
		"--max-plan-nodes", "1",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute compile-preview-review: %v", err)
	}
	output := out.String()
	for _, want := range []string{
		"compile-preview-review: ",
		"status=dry-run mode=metadata-only action=discover_assets nodes=1 warnings=1",
		"compile-review-node: id=discover_assets:01 manifest=infra-pack tool=amass adapter=managed_binary operation=subdomain_discovery capability=infra.host_discovery",
		"signals=domain:infra_internal=4",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected compile-preview-review output to contain %q, got %q", want, output)
		}
	}
	reviews, err := filepath.Glob(filepath.Join(root, "toolchain", "compile-preview-reviews", "*.json"))
	if err != nil {
		t.Fatalf("glob compile preview reviews: %v", err)
	}
	if len(reviews) != 1 {
		t.Fatalf("expected one compile preview review artifact, got %#v", reviews)
	}

	var listOut bytes.Buffer
	listCmd := newRootCommand(&listOut, new(bytes.Buffer))
	listCmd.SetArgs([]string{"toolchain", "compile-preview-reviews", root})
	if err := listCmd.Execute(); err != nil {
		t.Fatalf("execute compile-preview-reviews list: %v", err)
	}
	for _, want := range []string{
		"compile-preview-reviews: total=1",
		"compile-preview-review: " + reviews[0] + " status=dry-run mode=metadata-only action=discover_assets nodes=1 warnings=1",
	} {
		if !strings.Contains(listOut.String(), want) {
			t.Fatalf("expected compile-preview-reviews output to contain %q, got %q", want, listOut.String())
		}
	}

	var inspectOut bytes.Buffer
	inspectCmd := newRootCommand(&inspectOut, new(bytes.Buffer))
	inspectCmd.SetArgs([]string{"toolchain", "compile-preview-reviews", root, filepath.Base(reviews[0])})
	if err := inspectCmd.Execute(); err != nil {
		t.Fatalf("execute compile-preview-reviews inspect: %v", err)
	}
	if !strings.Contains(inspectOut.String(), "compile-review-node: id=discover_assets:01 manifest=infra-pack tool=amass adapter=managed_binary operation=subdomain_discovery capability=infra.host_discovery") {
		t.Fatalf("expected inspected review nodes, got %q", inspectOut.String())
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("compile-preview review commands must not execute run_command, marker stat err=%v", err)
	}
}

func TestToolchainCompilePreviewReviewAnnotateCommandWritesMetadataOnlyDecision(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := app.InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	marker := filepath.Join(t.TempDir(), "compile-preview-review-annotate-ran")
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
	review, err := app.CreateToolCompilePreviewReview(root, app.ToolCompilePreviewOptions{
		Budget: compiler.CompileBudget{MaxPlanNodes: 1},
	})
	if err != nil {
		t.Fatalf("create compile preview review: %v", err)
	}

	var out bytes.Buffer
	cmd := newRootCommand(&out, new(bytes.Buffer))
	cmd.SetArgs([]string{
		"toolchain", "compile-preview-review-annotate", root, filepath.Base(review.Path),
		"--decision", "deferred",
		"--reason", "authorized boundary false positive",
		"--reviewer", "alice",
		"--reviewed-at", "2026-05-31T15:04:05Z",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute compile-preview-review-annotate: %v", err)
	}
	for _, want := range []string{
		"compile-preview-review: " + review.Path + " status=dry-run mode=metadata-only action=discover_assets nodes=1 warnings=1",
		"compile-review-annotation: decision=deferred reviewer=alice reviewed_at=2026-05-31T15:04:05Z reason=authorized boundary false positive",
	} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("expected annotation output to contain %q, got %q", want, out.String())
		}
	}

	var listOut bytes.Buffer
	listCmd := newRootCommand(&listOut, new(bytes.Buffer))
	listCmd.SetArgs([]string{"toolchain", "compile-preview-reviews", root})
	if err := listCmd.Execute(); err != nil {
		t.Fatalf("execute compile-preview-reviews list: %v", err)
	}
	if !strings.Contains(listOut.String(), "compile-review-annotation: decision=deferred reviewer=alice reviewed_at=2026-05-31T15:04:05Z reason=authorized boundary false positive") {
		t.Fatalf("expected compile-preview-reviews list to show annotation, got %q", listOut.String())
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("compile-preview review annotation commands must not execute run_command, marker stat err=%v", err)
	}
}

func TestToolchainCompilePreviewReviewCompareCommandPrintsMetadataOnlyDiffs(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := app.InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	marker := filepath.Join(t.TempDir(), "compile-preview-review-compare-ran")
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
	basePath := writeCompilePreviewReviewCommandFixture(t, root, "base.json", app.ToolCompilePreviewReview{
		Status: "dry-run",
		Mode:   "metadata-only",
		Preview: app.ToolCompilePreview{
			Workspace: root,
			Action:    compiler.ActionDiscoverAssets,
			Mode:      "metadata-only",
			Nodes: []compiler.PlanNode{
				{ID: "discover_assets:01", ManifestName: "web-api-pack", Capability: "web.discovery", Runner: "command", Parser: "web_api_parser", SelectionScore: 1, SelectionSignals: []string{"history:successes=1 failures=0"}},
			},
			Warnings: []string{"base warning"},
		},
	})
	candidatePath := writeCompilePreviewReviewCommandFixture(t, root, "candidate.json", app.ToolCompilePreviewReview{
		Status: "dry-run",
		Mode:   "metadata-only",
		Preview: app.ToolCompilePreview{
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

	var out bytes.Buffer
	cmd := newRootCommand(&out, new(bytes.Buffer))
	cmd.SetArgs([]string{"toolchain", "compile-preview-review-compare", root, filepath.Base(basePath), filepath.Base(candidatePath)})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute compile-preview-review-compare: %v", err)
	}
	for _, want := range []string{
		"compile-preview-review-compare: mode=metadata-only base=" + basePath + " candidate=" + candidatePath,
		"node_delta=1 warning_delta=0 added=1 removed=0",
		"score_changed=1 signal_changed=1",
		"compile-review-compare-node: change=score,signals manifest=web-api-pack capability=web.discovery",
		"compile-review-compare-node: change=added manifest=ctf-pack capability=ctf.challenge_discovery",
		"compile-review-compare-warning: change=removed warning=base warning",
		"compile-review-compare-warning: change=added warning=candidate warning",
	} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("expected compare output to contain %q, got %q", want, out.String())
		}
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("compile-preview review compare command must not execute run_command, marker stat err=%v", err)
	}
}

func TestToolchainCompilePreviewReviewSelectCommandWritesAndReadsCurrentSelection(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := app.InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	basePath := writeCompilePreviewReviewCommandFixture(t, root, "base.json", app.ToolCompilePreviewReview{
		Status: "dry-run",
		Mode:   "metadata-only",
		Preview: app.ToolCompilePreview{
			Workspace: root,
			Action:    compiler.ActionDiscoverAssets,
			Mode:      "metadata-only",
			Nodes: []compiler.PlanNode{
				{ID: "discover_assets:01", ManifestName: "web-api-pack", Capability: "web.discovery"},
			},
		},
	})
	candidatePath := writeCompilePreviewReviewCommandFixture(t, root, "candidate.json", app.ToolCompilePreviewReview{
		Status: "dry-run",
		Mode:   "metadata-only",
		Preview: app.ToolCompilePreview{
			Workspace: root,
			Action:    compiler.ActionDiscoverAssets,
			Mode:      "metadata-only",
			Nodes: []compiler.PlanNode{
				{ID: "discover_assets:01", ManifestName: "infra-pack", Capability: "infra.host_discovery"},
			},
		},
	})

	var out bytes.Buffer
	cmd := newRootCommand(&out, new(bytes.Buffer))
	cmd.SetArgs([]string{
		"toolchain", "compile-preview-review-select", root, filepath.Base(candidatePath),
		"--selected-by", "alice",
		"--selected-at", "2026-06-01T00:15:00Z",
		"--reason", "best reviewed candidate",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute compile-preview-review-select: %v", err)
	}
	for _, want := range []string{
		"compile-preview-review-selection: status=current mode=metadata-only review=" + candidatePath,
		"superseded=1 selected_by=alice selected_at=2026-06-01T00:15:00Z reason=best reviewed candidate",
		"compile-review-superseded: " + basePath,
	} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("expected selection output to contain %q, got %q", want, out.String())
		}
	}

	var readOut bytes.Buffer
	readCmd := newRootCommand(&readOut, new(bytes.Buffer))
	readCmd.SetArgs([]string{"toolchain", "compile-preview-review-selection", root})
	if err := readCmd.Execute(); err != nil {
		t.Fatalf("execute compile-preview-review-selection: %v", err)
	}
	if !strings.Contains(readOut.String(), "compile-preview-review-selection: status=current mode=metadata-only review="+candidatePath) {
		t.Fatalf("expected read selection output, got %q", readOut.String())
	}

	var clearOut bytes.Buffer
	clearCmd := newRootCommand(&clearOut, new(bytes.Buffer))
	clearCmd.SetArgs([]string{
		"toolchain", "compile-preview-review-selection-clear", root,
		"--cleared-by", "alice",
		"--cleared-at", "2026-06-01T00:35:00Z",
		"--reason", "authorization changed",
	})
	if err := clearCmd.Execute(); err != nil {
		t.Fatalf("execute compile-preview-review-selection-clear: %v", err)
	}
	for _, want := range []string{
		"compile-preview-review-selection: status=cleared mode=metadata-only review=" + candidatePath,
		"cleared_by=alice cleared_at=2026-06-01T00:35:00Z reason=authorization changed",
	} {
		if !strings.Contains(clearOut.String(), want) {
			t.Fatalf("expected cleared selection output to contain %q, got %q", want, clearOut.String())
		}
	}

	var readClearedOut bytes.Buffer
	readClearedCmd := newRootCommand(&readClearedOut, new(bytes.Buffer))
	readClearedCmd.SetArgs([]string{"toolchain", "compile-preview-review-selection", root})
	if err := readClearedCmd.Execute(); err != nil {
		t.Fatalf("execute compile-preview-review-selection after clear: %v", err)
	}
	if !strings.Contains(readClearedOut.String(), "compile-preview-review-selection: status=cleared mode=metadata-only review="+candidatePath) {
		t.Fatalf("expected cleared selection read output, got %q", readClearedOut.String())
	}
}

func writeCompilePreviewReviewCommandFixture(t *testing.T, root, name string, review app.ToolCompilePreviewReview) string {
	t.Helper()
	dir := filepath.Join(root, "toolchain", "compile-preview-reviews")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("create compile preview review fixture dir: %v", err)
	}
	path := filepath.Join(dir, name)
	data, err := json.MarshalIndent(review, "", "  ")
	if err != nil {
		t.Fatalf("marshal compile preview review fixture: %v", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write compile preview review fixture: %v", err)
	}
	return path
}

func TestToolchainCompilePreviewSignalsCommandWritesMetadataOnlySignals(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := app.InitWorkspace(root); err != nil {
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

	var out bytes.Buffer
	cmd := newRootCommand(&out, new(bytes.Buffer))
	cmd.SetArgs([]string{
		"toolchain", "compile-preview-signals", root,
		"--write",
		"--max-plan-nodes", "1",
		"--domain-priority", "infra_internal=3",
		"--frontier-score", "infra-pack:infra.host_discovery=mission_value=2,unlock_potential=1",
		"--history", "infra-pack:infra.host_discovery=successes=2,failures=0",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute toolchain compile-preview-signals: %v", err)
	}
	output := out.String()
	wantSummary := "compile-preview-signals: path=" + filepath.Join(root, "toolchain", "compile-preview-signals.json") + " status=written mode=metadata-only max_plan_nodes=1 domain_priorities=1 frontier_scores=1 historical_stats=1"
	if !strings.Contains(output, wantSummary) {
		t.Fatalf("expected compile-preview-signals output %q, got %q", wantSummary, output)
	}

	var previewOut bytes.Buffer
	previewCmd := newRootCommand(&previewOut, new(bytes.Buffer))
	previewCmd.SetArgs([]string{"toolchain", "compile-preview", root})
	if err := previewCmd.Execute(); err != nil {
		t.Fatalf("execute toolchain compile-preview: %v", err)
	}
	previewOutput := previewOut.String()
	for _, want := range []string{
		"compile-preview: workspace=" + root + " action=discover_assets mode=metadata-only nodes=1 warnings=1",
		"compile-node: id=discover_assets:01 manifest=infra-pack tool=amass adapter=managed_binary operation=subdomain_discovery capability=infra.host_discovery",
		"score=8",
		"signals=domain:infra_internal=3,frontier=3,history:successes=2 failures=0",
	} {
		if !strings.Contains(previewOutput, want) {
			t.Fatalf("expected compile-preview output to contain %q, got %q", want, previewOutput)
		}
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("compile-preview signals command must not execute run_command, marker stat err=%v", err)
	}
}

func TestToolchainCompilePreviewSignalsFromReplayCommandDerivesHistoryMetadataOnly(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := app.InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	marker := filepath.Join(t.TempDir(), "compile-preview-derived-ran")
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
	replayDir := filepath.Join(root, "replay")
	if err := os.MkdirAll(replayDir, 0o755); err != nil {
		t.Fatalf("create replay dir: %v", err)
	}
	store, err := eventlog.OpenSQLite(filepath.Join(replayDir, "events.db"))
	if err != nil {
		t.Fatalf("open event log: %v", err)
	}
	defer store.Close()
	runID := "run-derived-signals"
	for _, payload := range []map[string]any{
		{"tool": "infra-pack", "execution_id": "tool-infra", "execution_status": "completed", "capability": "infra.host_discovery"},
		{"tool": "web-api-pack", "execution_id": "tool-web", "execution_status": "failed", "capability": "web.discovery"},
		{"tool": "ctf-pack", "execution_id": "tool-ctf", "execution_status": "skipped", "capability": "ctf.challenge_classification"},
	} {
		data, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("encode event payload: %v", err)
		}
		if _, err := store.Append(context.Background(), eventlog.Event{RunID: runID, Type: eventlog.EventTool, Payload: data}); err != nil {
			t.Fatalf("append replay event: %v", err)
		}
	}

	var dryOut bytes.Buffer
	dryCmd := newRootCommand(&dryOut, new(bytes.Buffer))
	dryCmd.SetArgs([]string{"toolchain", "compile-preview-signals-from-replay", root, runID})
	if err := dryCmd.Execute(); err != nil {
		t.Fatalf("execute dry-run compile-preview-signals-from-replay: %v", err)
	}
	dryOutput := dryOut.String()
	for _, want := range []string{
		"compile-preview-signals-from-replay: workspace=" + root + " run=run-derived-signals status=dry-run mode=metadata-only",
		"historical_stats=2 successes=1 failures=1 ignored=1",
		"signal-history: manifest=infra-pack capability=infra.host_discovery successes=1 failures=0",
		"signal-history: manifest=web-api-pack capability=web.discovery successes=0 failures=1",
	} {
		if !strings.Contains(dryOutput, want) {
			t.Fatalf("expected dry-run output to contain %q, got %q", want, dryOutput)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "toolchain", "compile-preview-signals.json")); !os.IsNotExist(err) {
		t.Fatalf("dry-run derive command must not write signals, stat err=%v", err)
	}

	var writeOut bytes.Buffer
	writeCmd := newRootCommand(&writeOut, new(bytes.Buffer))
	writeCmd.SetArgs([]string{"toolchain", "compile-preview-signals-from-replay", root, runID, "--write", "--max-plan-nodes", "1"})
	if err := writeCmd.Execute(); err != nil {
		t.Fatalf("execute write compile-preview-signals-from-replay: %v", err)
	}
	if !strings.Contains(writeOut.String(), "status=written") || !strings.Contains(writeOut.String(), "max_plan_nodes=1") {
		t.Fatalf("expected write output to show written budgeted signals, got %q", writeOut.String())
	}
	var previewOut bytes.Buffer
	previewCmd := newRootCommand(&previewOut, new(bytes.Buffer))
	previewCmd.SetArgs([]string{"toolchain", "compile-preview", root})
	if err := previewCmd.Execute(); err != nil {
		t.Fatalf("execute compile preview: %v", err)
	}
	previewOutput := previewOut.String()
	for _, want := range []string{
		"compile-preview: workspace=" + root + " action=discover_assets mode=metadata-only nodes=1 warnings=1",
		"compile-node: id=discover_assets:01 manifest=infra-pack tool=amass adapter=managed_binary operation=subdomain_discovery capability=infra.host_discovery",
		"signals=history:successes=1 failures=0",
	} {
		if !strings.Contains(previewOutput, want) {
			t.Fatalf("expected preview output to contain %q, got %q", want, previewOutput)
		}
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("derived signal workflow must not execute run_command, marker stat err=%v", err)
	}
}

func TestToolchainCompilePreviewSignalsFromReplayCommandDerivesFrontierScores(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := app.InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	replayDir := filepath.Join(root, "replay")
	if err := os.MkdirAll(replayDir, 0o755); err != nil {
		t.Fatalf("create replay dir: %v", err)
	}
	store, err := eventlog.OpenSQLite(filepath.Join(replayDir, "events.db"))
	if err != nil {
		t.Fatalf("open event log: %v", err)
	}
	defer store.Close()
	runID := "run-frontier-signals"
	payload := map[string]any{
		"intent_id":           "intent-infra-frontier",
		"kind":                "hypothesis",
		"goal":                "prioritize host discovery",
		"target":              "10.0.0.0/24",
		"rationale":           "host inventory unlocks internal paths",
		"action":              "discover_assets",
		"mission_value":       2,
		"unlock_potential":    1,
		"evidence_confidence": 0.8,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("encode intent payload: %v", err)
	}
	if _, err := store.Append(context.Background(), eventlog.Event{RunID: runID, Type: eventlog.EventIntent, Payload: data}); err != nil {
		t.Fatalf("append intent event: %v", err)
	}

	var out bytes.Buffer
	cmd := newRootCommand(&out, new(bytes.Buffer))
	cmd.SetArgs([]string{"toolchain", "compile-preview-signals-from-replay", root, runID, "--write", "--max-plan-nodes", "1"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute compile-preview-signals-from-replay: %v", err)
	}
	output := out.String()
	for _, want := range []string{
		"compile-preview-signals-from-replay: workspace=" + root + " run=run-frontier-signals status=written mode=metadata-only",
		"frontier_scores=1",
		"signal-frontier: manifest=infra-pack capability=infra.host_discovery id=intent-infra-frontier target=10.0.0.0/24 score=3.8",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected frontier derivation output to contain %q, got %q", want, output)
		}
	}

	var previewOut bytes.Buffer
	previewCmd := newRootCommand(&previewOut, new(bytes.Buffer))
	previewCmd.SetArgs([]string{"toolchain", "compile-preview", root})
	if err := previewCmd.Execute(); err != nil {
		t.Fatalf("execute compile preview: %v", err)
	}
	previewOutput := previewOut.String()
	for _, want := range []string{
		"compile-preview: workspace=" + root + " action=discover_assets mode=metadata-only nodes=1 warnings=1",
		"compile-node: id=discover_assets:01 manifest=infra-pack tool=amass adapter=managed_binary operation=subdomain_discovery capability=infra.host_discovery",
		"signals=frontier=3.8",
	} {
		if !strings.Contains(previewOutput, want) {
			t.Fatalf("expected preview output to contain %q, got %q", want, previewOutput)
		}
	}
}

func TestToolchainCompilePreviewSignalsFromReplayCommandDerivesFrontierScoresFromCandidatePathFacts(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := app.InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	marker := filepath.Join(t.TempDir(), "candidate-path-frontier-ran")
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
	replayDir := filepath.Join(root, "replay")
	if err := os.MkdirAll(replayDir, 0o755); err != nil {
		t.Fatalf("create replay dir: %v", err)
	}
	store, err := eventlog.OpenSQLite(filepath.Join(replayDir, "events.db"))
	if err != nil {
		t.Fatalf("open event log: %v", err)
	}
	defer store.Close()
	runID := "run-candidate-path-frontier"
	payload := map[string]any{
		"fact_id":             "fact-web-frontier",
		"kind":                "candidate_path",
		"statement":           "prioritize web discovery from reviewed evidence",
		"target":              "https://app.example",
		"expected_unlock":     "route inventory unlocks auth testing",
		"action":              "discover_assets",
		"domain":              "web_api",
		"mission_value":       2,
		"unlock_potential":    1,
		"evidence_confidence": 0.8,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("encode candidate-path payload: %v", err)
	}
	if _, err := store.Append(context.Background(), eventlog.Event{RunID: runID, Type: eventlog.EventFact, Payload: data}); err != nil {
		t.Fatalf("append candidate-path fact event: %v", err)
	}

	var out bytes.Buffer
	cmd := newRootCommand(&out, new(bytes.Buffer))
	cmd.SetArgs([]string{"toolchain", "compile-preview-signals-from-replay", root, runID, "--write", "--max-plan-nodes", "1"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute candidate-path compile-preview-signals-from-replay: %v", err)
	}
	output := out.String()
	for _, want := range []string{
		"compile-preview-signals-from-replay: workspace=" + root + " run=run-candidate-path-frontier status=written mode=metadata-only",
		"frontier_scores=1",
		"signal-frontier: manifest=web-api-pack capability=web.discovery id=fact-web-frontier target=https://app.example score=3.8",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected candidate-path frontier output to contain %q, got %q", want, output)
		}
	}

	var previewOut bytes.Buffer
	previewCmd := newRootCommand(&previewOut, new(bytes.Buffer))
	previewCmd.SetArgs([]string{"toolchain", "compile-preview", root})
	if err := previewCmd.Execute(); err != nil {
		t.Fatalf("execute compile preview: %v", err)
	}
	previewOutput := previewOut.String()
	for _, want := range []string{
		"compile-preview: workspace=" + root + " action=discover_assets mode=metadata-only nodes=1 warnings=1",
		"compile-node: id=discover_assets:01 manifest=web-api-pack tool= adapter= operation= capability=web.discovery",
		"signals=frontier=3.8",
	} {
		if !strings.Contains(previewOutput, want) {
			t.Fatalf("expected candidate-path preview output to contain %q, got %q", want, previewOutput)
		}
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("candidate-path derived signal workflow must not execute run_command, marker stat err=%v", err)
	}
}

func TestToolchainHealthCommandShowsExecProbeOutput(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := app.InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	manifest := `name: web-api-pack
version: "0.1.0"
capabilities:
  - web.discovery
runner: native
parser: web_api_parser
healthcheck: exec
health_command:
  - printf
  - "cli-probe-5.0.0\n"
`
	if err := os.WriteFile(filepath.Join(root, "toolchain", "web-api-pack", "manifest.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	var healthOut bytes.Buffer
	healthCmd := newRootCommand(&healthOut, new(bytes.Buffer))
	healthCmd.SetArgs([]string{"toolchain", "health", root})
	if err := healthCmd.Execute(); err != nil {
		t.Fatalf("execute toolchain health: %v", err)
	}
	for _, want := range []string{"web-api-pack", "state=ready", "installed=cli-probe-5.0.0"} {
		if !strings.Contains(healthOut.String(), want) {
			t.Fatalf("expected health output to contain %q, got %q", want, healthOut.String())
		}
	}
}

func TestToolchainHealthCommandShowsDependencyHealth(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := app.InitWorkspace(root); err != nil {
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
  - name: httpx
    kind: binary
    health_command:
      - printf
      - "httpx-1.3.0\n"
`
	if err := os.WriteFile(filepath.Join(root, "toolchain", "web-api-pack", "manifest.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	var healthOut bytes.Buffer
	healthCmd := newRootCommand(&healthOut, new(bytes.Buffer))
	healthCmd.SetArgs([]string{"toolchain", "health", root})
	if err := healthCmd.Execute(); err != nil {
		t.Fatalf("execute toolchain health: %v", err)
	}
	for _, want := range []string{"dependency: web-api-pack/httpx", "state=ready", "installed=httpx-1.3.0"} {
		if !strings.Contains(healthOut.String(), want) {
			t.Fatalf("expected dependency health output to contain %q, got %q", want, healthOut.String())
		}
	}
}

func TestToolchainHealthCommandShowsDependencyInstallPlan(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := app.InitWorkspace(root); err != nil {
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

	var healthOut bytes.Buffer
	healthCmd := newRootCommand(&healthOut, new(bytes.Buffer))
	healthCmd.SetArgs([]string{"toolchain", "health", root})
	if err := healthCmd.Execute(); err != nil {
		t.Fatalf("execute toolchain health: %v", err)
	}
	for _, want := range []string{
		"install: web-api-pack/",
		"state=missing",
		"required=true",
		"command=[\"ouro-toolchain\",\"install\",\"httpx\"]",
		"hint=install bundled httpx pack",
	} {
		if !strings.Contains(healthOut.String(), want) {
			t.Fatalf("expected dependency install plan output to contain %q, got %q", want, healthOut.String())
		}
	}
}

func TestToolchainInstallPlanCommandPrintsMetadataOnlySteps(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := app.InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	marker := filepath.Join(t.TempDir(), "installer-ran")
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
      - sh
      - -c
      - touch ` + marker + `
`
	if err := os.WriteFile(filepath.Join(root, "toolchain", "web-api-pack", "manifest.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	var out bytes.Buffer
	cmd := newRootCommand(&out, new(bytes.Buffer))
	cmd.SetArgs([]string{"toolchain", "install-plan", root})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute toolchain install-plan: %v", err)
	}
	for _, want := range []string{
		"install-plan: web-api-pack/",
		"kind=binary",
		"state=missing",
		"required=true",
		"command=[\"sh\",\"-c\",\"touch " + marker + "\"]",
		"hint=install bundled httpx pack",
	} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("expected install-plan output to contain %q, got %q", want, out.String())
		}
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("install-plan command must not execute install command, marker stat err=%v", err)
	}
}

func TestToolchainInstallPlanCommandPrintsDependencyOrdering(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := app.InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	marker := filepath.Join(t.TempDir(), "installer-ran")
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
    install_command:
      - sh
      - -c
      - touch ` + marker + `
  - name: z-runtime
    kind: binary
    install_command:
      - sh
      - -c
      - touch ` + marker + `
`
	if err := os.WriteFile(filepath.Join(root, "toolchain", "web-api-pack", "manifest.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	var out bytes.Buffer
	cmd := newRootCommand(&out, new(bytes.Buffer))
	cmd.SetArgs([]string{"toolchain", "install-plan", root})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute toolchain install-plan: %v", err)
	}
	output := out.String()
	runtimeIndex := strings.Index(output, "install-plan: web-api-pack/z-runtime")
	httpxIndex := strings.Index(output, "install-plan: web-api-pack/a-httpx")
	if runtimeIndex < 0 || httpxIndex < 0 || runtimeIndex > httpxIndex {
		t.Fatalf("expected prerequisite before dependent, got %q", output)
	}
	if !strings.Contains(output, "depends_on=z-runtime") {
		t.Fatalf("expected depends_on metadata in install-plan output, got %q", output)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("install-plan command must not execute install command, marker stat err=%v", err)
	}
}

func TestToolchainInstallRequestCommandWritesPendingRequest(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := app.InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	missing := filepath.Join(t.TempDir(), "missing-httpx")
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

	var out bytes.Buffer
	cmd := newRootCommand(&out, new(bytes.Buffer))
	cmd.SetArgs([]string{"toolchain", "install-request", root, "web-api-pack", missing})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute toolchain install-request: %v", err)
	}
	requestPath := parseOutputValue(t, out.String(), "install-request:")
	if requestPath == "" {
		t.Fatalf("expected install request path, got %q", out.String())
	}
	for _, want := range []string{
		"status: pending",
		"command=[\"sh\",\"-c\",\"touch " + marker + "\"]",
	} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("expected install-request output to contain %q, got %q", want, out.String())
		}
	}
	if _, err := os.Stat(requestPath); err != nil {
		t.Fatalf("expected request file to exist: %v", err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("install-request command must not execute install command, marker stat err=%v", err)
	}
}

func TestToolchainInstallRequestsCommandListsPendingRequests(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := app.InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	missing := filepath.Join(t.TempDir(), "missing-httpx")
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
	request, err := app.CreateToolchainInstallRequest(root, "web-api-pack", missing)
	if err != nil {
		t.Fatalf("create install request: %v", err)
	}

	var out bytes.Buffer
	cmd := newRootCommand(&out, new(bytes.Buffer))
	cmd.SetArgs([]string{"toolchain", "install-requests", root})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute toolchain install-requests: %v", err)
	}
	for _, want := range []string{
		"install-request: " + request.Path,
		"status=pending",
		"pack=web-api-pack",
		"dependency=" + missing,
		"command=[\"sh\",\"-c\",\"touch " + marker + "\"]",
	} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("expected install-requests output to contain %q, got %q", want, out.String())
		}
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("install-requests command must not execute install command, marker stat err=%v", err)
	}
}

func TestToolchainInstallRequestApproveAndDenyCommandsUpdateMetadataOnly(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := app.InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	marker := filepath.Join(t.TempDir(), "installer-ran")
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
      - touch ` + marker + `
  - name: missing-nuclei
    kind: binary
    install_command:
      - sh
      - -c
      - touch ` + marker + `
`
	if err := os.WriteFile(filepath.Join(root, "toolchain", "web-api-pack", "manifest.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	approveRequest, err := app.CreateToolchainInstallRequest(root, "web-api-pack", "missing-httpx")
	if err != nil {
		t.Fatalf("create approve request: %v", err)
	}
	denyRequest, err := app.CreateToolchainInstallRequest(root, "web-api-pack", "missing-nuclei")
	if err != nil {
		t.Fatalf("create deny request: %v", err)
	}

	var approveOut bytes.Buffer
	approveCmd := newRootCommand(&approveOut, new(bytes.Buffer))
	approveCmd.SetArgs([]string{"toolchain", "approve-install-request", root, approveRequest.Path})
	if err := approveCmd.Execute(); err != nil {
		t.Fatalf("execute approve-install-request: %v", err)
	}
	var denyOut bytes.Buffer
	denyCmd := newRootCommand(&denyOut, new(bytes.Buffer))
	denyCmd.SetArgs([]string{"toolchain", "deny-install-request", root, filepath.Base(denyRequest.Path)})
	if err := denyCmd.Execute(); err != nil {
		t.Fatalf("execute deny-install-request: %v", err)
	}
	for _, check := range []struct {
		output string
		wants  []string
	}{
		{output: approveOut.String(), wants: []string{"install-request: " + approveRequest.Path, "status=approved"}},
		{output: denyOut.String(), wants: []string{"install-request: " + denyRequest.Path, "status=denied"}},
	} {
		for _, text := range check.wants {
			if !strings.Contains(check.output, text) {
				t.Fatalf("expected output to contain %q, got %q", text, check.output)
			}
		}
	}
	requests, err := app.ListToolchainInstallRequests(root)
	if err != nil {
		t.Fatalf("list requests: %v", err)
	}
	statuses := map[string]string{}
	for _, request := range requests {
		statuses[request.Dependency] = request.Status
	}
	if statuses["missing-httpx"] != "approved" || statuses["missing-nuclei"] != "denied" {
		t.Fatalf("expected CLI to persist statuses, got %#v", statuses)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("approve/deny commands must not execute install command, marker stat err=%v", err)
	}
}

func TestToolchainInstallRequestsCommandPrintsStatusCounts(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := app.InitWorkspace(root); err != nil {
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
	approved, err := app.CreateToolchainInstallRequest(root, "web-api-pack", "missing-httpx")
	if err != nil {
		t.Fatalf("create approved request: %v", err)
	}
	denied, err := app.CreateToolchainInstallRequest(root, "web-api-pack", "missing-nuclei")
	if err != nil {
		t.Fatalf("create denied request: %v", err)
	}
	if _, err := app.CreateToolchainInstallRequest(root, "web-api-pack", "missing-naabu"); err != nil {
		t.Fatalf("create pending request: %v", err)
	}
	if _, err := app.UpdateToolchainInstallRequestStatus(root, approved.Path, "approved"); err != nil {
		t.Fatalf("approve request: %v", err)
	}
	if _, err := app.UpdateToolchainInstallRequestStatus(root, denied.Path, "denied"); err != nil {
		t.Fatalf("deny request: %v", err)
	}

	var out bytes.Buffer
	cmd := newRootCommand(&out, new(bytes.Buffer))
	cmd.SetArgs([]string{"toolchain", "install-requests", root})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute install-requests: %v", err)
	}
	if !strings.Contains(out.String(), "install-requests: total=3 pending=1 approved=1 denied=1") {
		t.Fatalf("expected install request status counts, got %q", out.String())
	}
}

func TestToolchainInstallDispatchPlanCommandRequiresApprovedRequests(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := app.InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	marker := filepath.Join(t.TempDir(), "installer-ran")
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
      - touch ` + marker + `
  - name: missing-nuclei
    kind: binary
    install_command:
      - sh
      - -c
      - touch ` + marker + `
  - name: missing-naabu
    kind: binary
    install_command:
      - sh
      - -c
      - touch ` + marker + `
`
	if err := os.WriteFile(filepath.Join(root, "toolchain", "web-api-pack", "manifest.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	approved, err := app.CreateToolchainInstallRequest(root, "web-api-pack", "missing-httpx")
	if err != nil {
		t.Fatalf("create approved request: %v", err)
	}
	denied, err := app.CreateToolchainInstallRequest(root, "web-api-pack", "missing-nuclei")
	if err != nil {
		t.Fatalf("create denied request: %v", err)
	}
	if _, err := app.CreateToolchainInstallRequest(root, "web-api-pack", "missing-naabu"); err != nil {
		t.Fatalf("create pending request: %v", err)
	}
	if _, err := app.UpdateToolchainInstallRequestStatus(root, approved.Path, "approved"); err != nil {
		t.Fatalf("approve request: %v", err)
	}
	if _, err := app.UpdateToolchainInstallRequestStatus(root, denied.Path, "denied"); err != nil {
		t.Fatalf("deny request: %v", err)
	}

	var out bytes.Buffer
	cmd := newRootCommand(&out, new(bytes.Buffer))
	cmd.SetArgs([]string{"toolchain", "install-dispatch-plan", root})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute install-dispatch-plan: %v", err)
	}
	output := out.String()
	for _, want := range []string{
		"install-dispatch-plan: total=3 ready=1 skipped=2 pending=1 approved=1 denied=1",
		"dispatch-install: " + approved.Path,
		"status=approved",
		"dependency=missing-httpx",
		"mode=metadata-only",
		"skip-install-request:",
		"dependency=missing-naabu",
		"reason=not-approved: pending",
		"dependency=missing-nuclei",
		"reason=not-approved: denied",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected dispatch-plan output to contain %q, got %q", want, output)
		}
	}
	if strings.Contains(output, "dispatch-install: "+denied.Path) {
		t.Fatalf("denied request must not be dispatch-ready, got %q", output)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("install-dispatch-plan command must not execute install command, marker stat err=%v", err)
	}
}

func TestToolchainInstallDispatchPlanCommandPrintsDependencyReadiness(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := app.InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	marker := filepath.Join(t.TempDir(), "installer-ran")
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
    install_command:
      - sh
      - -c
      - touch ` + marker + `
  - name: z-runtime
    kind: binary
    install_command:
      - sh
      - -c
      - touch ` + marker + `
`
	if err := os.WriteFile(filepath.Join(root, "toolchain", "web-api-pack", "manifest.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	dependent, err := app.CreateToolchainInstallRequest(root, "web-api-pack", "a-httpx")
	if err != nil {
		t.Fatalf("create dependent request: %v", err)
	}
	prerequisite, err := app.CreateToolchainInstallRequest(root, "web-api-pack", "z-runtime")
	if err != nil {
		t.Fatalf("create prerequisite request: %v", err)
	}
	if _, err := app.UpdateToolchainInstallRequestStatus(root, dependent.Path, "approved"); err != nil {
		t.Fatalf("approve dependent request: %v", err)
	}
	if _, err := app.UpdateToolchainInstallRequestStatus(root, prerequisite.Path, "approved"); err != nil {
		t.Fatalf("approve prerequisite request: %v", err)
	}

	var out bytes.Buffer
	cmd := newRootCommand(&out, new(bytes.Buffer))
	cmd.SetArgs([]string{"toolchain", "install-dispatch-plan", root})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute install-dispatch-plan: %v", err)
	}
	output := out.String()
	runtimeIndex := strings.Index(output, "dependency=z-runtime")
	httpxIndex := strings.Index(output, "dependency=a-httpx")
	if runtimeIndex < 0 || httpxIndex < 0 || runtimeIndex > httpxIndex {
		t.Fatalf("expected prerequisite dispatch before dependent, got %q", output)
	}
	if !strings.Contains(output, "depends_on=z-runtime") {
		t.Fatalf("expected dispatch-plan output to include depends_on metadata, got %q", output)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("install-dispatch-plan command must not execute install command, marker stat err=%v", err)
	}
}

func TestToolchainInstallDispatchPlanCommandSkipsUnapprovedDependency(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := app.InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	marker := filepath.Join(t.TempDir(), "installer-ran")
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
    install_command:
      - sh
      - -c
      - touch ` + marker + `
  - name: z-runtime
    kind: binary
    install_command:
      - sh
      - -c
      - touch ` + marker + `
`
	if err := os.WriteFile(filepath.Join(root, "toolchain", "web-api-pack", "manifest.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	dependent, err := app.CreateToolchainInstallRequest(root, "web-api-pack", "a-httpx")
	if err != nil {
		t.Fatalf("create dependent request: %v", err)
	}
	if _, err := app.CreateToolchainInstallRequest(root, "web-api-pack", "z-runtime"); err != nil {
		t.Fatalf("create prerequisite request: %v", err)
	}
	if _, err := app.UpdateToolchainInstallRequestStatus(root, dependent.Path, "approved"); err != nil {
		t.Fatalf("approve dependent request: %v", err)
	}

	var out bytes.Buffer
	cmd := newRootCommand(&out, new(bytes.Buffer))
	cmd.SetArgs([]string{"toolchain", "install-dispatch-plan", root})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute install-dispatch-plan: %v", err)
	}
	output := out.String()
	if !strings.Contains(output, "ready=0 skipped=2") {
		t.Fatalf("expected no ready requests while prerequisite is pending, got %q", output)
	}
	if !strings.Contains(output, "dependency=a-httpx") || !strings.Contains(output, "reason=dependency-not-approved: z-runtime status=pending") {
		t.Fatalf("expected dependent skip reason, got %q", output)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("install-dispatch-plan command must not execute install command, marker stat err=%v", err)
	}
}

func TestToolchainInstallDispatchCommandWritesDryRunArtifact(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := app.InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	marker := filepath.Join(t.TempDir(), "installer-ran")
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
      - touch ` + marker + `
  - name: missing-nuclei
    kind: binary
    install_command:
      - sh
      - -c
      - touch ` + marker + `
`
	if err := os.WriteFile(filepath.Join(root, "toolchain", "web-api-pack", "manifest.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	approved, err := app.CreateToolchainInstallRequest(root, "web-api-pack", "missing-httpx")
	if err != nil {
		t.Fatalf("create approved request: %v", err)
	}
	if _, err := app.CreateToolchainInstallRequest(root, "web-api-pack", "missing-nuclei"); err != nil {
		t.Fatalf("create pending request: %v", err)
	}
	if _, err := app.UpdateToolchainInstallRequestStatus(root, approved.Path, "approved"); err != nil {
		t.Fatalf("approve request: %v", err)
	}

	var out bytes.Buffer
	cmd := newRootCommand(&out, new(bytes.Buffer))
	cmd.SetArgs([]string{"toolchain", "install-dispatch", root})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute install-dispatch: %v", err)
	}
	output := out.String()
	for _, want := range []string{
		"install-dispatch:",
		"status=dry-run",
		"mode=metadata-only",
		"ready=1",
		"skipped=1",
		"dispatch-install: " + approved.Path,
		"skip-install-request:",
		"reason=not-approved: pending",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected install-dispatch output to contain %q, got %q", want, output)
		}
	}
	paths, err := filepath.Glob(filepath.Join(root, "toolchain", "install-dispatches", "*.json"))
	if err != nil {
		t.Fatalf("glob dispatch artifacts: %v", err)
	}
	if len(paths) != 1 {
		t.Fatalf("expected one dispatch artifact, got %#v", paths)
	}
	data, err := os.ReadFile(paths[0])
	if err != nil {
		t.Fatalf("read dispatch artifact: %v", err)
	}
	var decoded app.ToolchainInstallDispatch
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("decode dispatch artifact: %v", err)
	}
	if decoded.Status != "dry-run" || decoded.Mode != "metadata-only" || len(decoded.Ready) != 1 || len(decoded.Skipped) != 1 {
		t.Fatalf("unexpected dispatch artifact: %#v", decoded)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("install-dispatch command must not execute install command, marker stat err=%v", err)
	}
}

func TestToolchainInstallDispatchesCommandListsDryRunArtifacts(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := app.InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	marker := filepath.Join(t.TempDir(), "installer-ran")
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
      - touch ` + marker + `
  - name: missing-nuclei
    kind: binary
    install_command:
      - sh
      - -c
      - touch ` + marker + `
`
	if err := os.WriteFile(filepath.Join(root, "toolchain", "web-api-pack", "manifest.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	approved, err := app.CreateToolchainInstallRequest(root, "web-api-pack", "missing-httpx")
	if err != nil {
		t.Fatalf("create approved request: %v", err)
	}
	if _, err := app.CreateToolchainInstallRequest(root, "web-api-pack", "missing-nuclei"); err != nil {
		t.Fatalf("create pending request: %v", err)
	}
	if _, err := app.UpdateToolchainInstallRequestStatus(root, approved.Path, "approved"); err != nil {
		t.Fatalf("approve request: %v", err)
	}
	dispatch, err := app.CreateToolchainInstallDispatch(root)
	if err != nil {
		t.Fatalf("create dispatch: %v", err)
	}

	var out bytes.Buffer
	cmd := newRootCommand(&out, new(bytes.Buffer))
	cmd.SetArgs([]string{"toolchain", "install-dispatches", root})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute install-dispatches: %v", err)
	}
	output := out.String()
	for _, want := range []string{
		"install-dispatches: total=1",
		"install-dispatch: " + dispatch.Path,
		"status=dry-run",
		"mode=metadata-only",
		"ready=1",
		"skipped=1",
		"dispatch-install: " + approved.Path,
		"skip-install-request:",
		"reason=not-approved: pending",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected install-dispatches output to contain %q, got %q", want, output)
		}
	}

	var inspectOut bytes.Buffer
	inspectCmd := newRootCommand(&inspectOut, new(bytes.Buffer))
	inspectCmd.SetArgs([]string{"toolchain", "install-dispatches", root, filepath.Base(dispatch.Path)})
	if err := inspectCmd.Execute(); err != nil {
		t.Fatalf("execute install-dispatches inspect: %v", err)
	}
	if !strings.Contains(inspectOut.String(), "install-dispatch: "+dispatch.Path) || !strings.Contains(inspectOut.String(), "mode=metadata-only") {
		t.Fatalf("expected inspected dispatch output, got %q", inspectOut.String())
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("install-dispatches command must not execute install command, marker stat err=%v", err)
	}
}

func TestToolchainExecuteInstallDispatchCommandRequiresExecuteFlag(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := app.InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	marker := filepath.Join(t.TempDir(), "installer-ran")
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
      - touch ` + marker + `
`
	if err := os.WriteFile(filepath.Join(root, "toolchain", "web-api-pack", "manifest.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	request, err := app.CreateToolchainInstallRequest(root, "web-api-pack", "missing-httpx")
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	if _, err := app.UpdateToolchainInstallRequestStatus(root, request.Path, "approved"); err != nil {
		t.Fatalf("approve request: %v", err)
	}
	dispatch, err := app.CreateToolchainInstallDispatch(root)
	if err != nil {
		t.Fatalf("create dispatch: %v", err)
	}

	var out bytes.Buffer
	cmd := newRootCommand(&out, new(bytes.Buffer))
	cmd.SetArgs([]string{"toolchain", "execute-install-dispatch", root, filepath.Base(dispatch.Path)})
	err = cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "requires explicit execute") {
		t.Fatalf("expected explicit execute error, got %v output=%q", err, out.String())
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("execute-install-dispatch without --execute must not run command, marker stat err=%v", err)
	}
}

func TestToolchainExecuteInstallDispatchCommandRunsWithExecuteFlag(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := app.InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	approvedMarker := filepath.Join(t.TempDir(), "approved-ran")
	pendingMarker := filepath.Join(t.TempDir(), "pending-ran")
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
      - echo approved > ` + approvedMarker + `; echo approved
  - name: missing-nuclei
    kind: binary
    install_command:
      - sh
      - -c
      - touch ` + pendingMarker + `
`
	if err := os.WriteFile(filepath.Join(root, "toolchain", "web-api-pack", "manifest.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	approved, err := app.CreateToolchainInstallRequest(root, "web-api-pack", "missing-httpx")
	if err != nil {
		t.Fatalf("create approved request: %v", err)
	}
	if _, err := app.CreateToolchainInstallRequest(root, "web-api-pack", "missing-nuclei"); err != nil {
		t.Fatalf("create pending request: %v", err)
	}
	if _, err := app.UpdateToolchainInstallRequestStatus(root, approved.Path, "approved"); err != nil {
		t.Fatalf("approve request: %v", err)
	}
	dispatch, err := app.CreateToolchainInstallDispatch(root)
	if err != nil {
		t.Fatalf("create dispatch: %v", err)
	}

	var out bytes.Buffer
	cmd := newRootCommand(&out, new(bytes.Buffer))
	cmd.SetArgs([]string{"toolchain", "execute-install-dispatch", root, filepath.Base(dispatch.Path), "--execute"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute install dispatch: %v", err)
	}
	output := out.String()
	for _, want := range []string{
		"install-dispatch-execution:",
		"status=completed",
		"mode=execute",
		"results=1",
		"install-execute: pack=web-api-pack dependency=missing-httpx status=completed",
		"skip-install-request:",
		"reason=not-approved: pending",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected execute output to contain %q, got %q", want, output)
		}
	}
	if data, err := os.ReadFile(approvedMarker); err != nil || strings.TrimSpace(string(data)) != "approved" {
		t.Fatalf("expected approved command to run, data=%q err=%v", data, err)
	}
	if _, err := os.Stat(pendingMarker); !os.IsNotExist(err) {
		t.Fatalf("pending request command must not execute, marker stat err=%v", err)
	}
}

func TestToolchainExecuteInstallDispatchCommandPrintsSafetyWarnings(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := app.InitWorkspace(root); err != nil {
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
    authorized_by: operator
    reason: approved install execution safety bypass
`
	if err := os.WriteFile(filepath.Join(root, "authorization.yaml"), []byte(authorization), 0o644); err != nil {
		t.Fatalf("write authorization: %v", err)
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
      - true
`
	if err := os.WriteFile(filepath.Join(root, "toolchain", "web-api-pack", "manifest.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	request, err := app.CreateToolchainInstallRequest(root, "web-api-pack", "missing-httpx")
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	if _, err := app.UpdateToolchainInstallRequestStatus(root, request.Path, "approved"); err != nil {
		t.Fatalf("approve request: %v", err)
	}
	dispatch, err := app.CreateToolchainInstallDispatch(root)
	if err != nil {
		t.Fatalf("create dispatch: %v", err)
	}

	var out bytes.Buffer
	cmd := newRootCommand(&out, new(bytes.Buffer))
	cmd.SetArgs([]string{"toolchain", "execute-install-dispatch", root, filepath.Base(dispatch.Path), "--execute"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute install dispatch: %v", err)
	}
	output := out.String()
	for _, want := range []string{
		"warning: scope.target_policy is disabled",
		"warning: scope.target_policy disabled authorized by operator",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected warning %q in output, got %q", want, output)
		}
	}
}

func TestToolchainInstallExecutionsCommandListsAuditArtifacts(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := app.InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	marker := filepath.Join(t.TempDir(), "installer-ran")
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
      - echo cli-listed > ` + marker + `; echo cli-listed
`
	if err := os.WriteFile(filepath.Join(root, "toolchain", "web-api-pack", "manifest.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	request, err := app.CreateToolchainInstallRequest(root, "web-api-pack", "missing-httpx")
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	if _, err := app.UpdateToolchainInstallRequestStatus(root, request.Path, "approved"); err != nil {
		t.Fatalf("approve request: %v", err)
	}
	dispatch, err := app.CreateToolchainInstallDispatch(root)
	if err != nil {
		t.Fatalf("create dispatch: %v", err)
	}
	execution, err := app.ExecuteToolchainInstallDispatch(root, dispatch.Path, app.ToolchainInstallDispatchExecutionOptions{Execute: true})
	if err != nil {
		t.Fatalf("execute dispatch: %v", err)
	}

	var out bytes.Buffer
	cmd := newRootCommand(&out, new(bytes.Buffer))
	cmd.SetArgs([]string{"toolchain", "install-executions", root})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute install-executions: %v", err)
	}
	output := out.String()
	for _, want := range []string{
		"install-executions: total=1",
		"install-execution: " + execution.Path,
		"status=completed",
		"mode=execute",
		"results=1",
		"install-execute: pack=web-api-pack dependency=missing-httpx status=completed",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected install-executions output to contain %q, got %q", want, output)
		}
	}

	var inspectOut bytes.Buffer
	inspectCmd := newRootCommand(&inspectOut, new(bytes.Buffer))
	inspectCmd.SetArgs([]string{"toolchain", "install-executions", root, filepath.Base(execution.Path)})
	if err := inspectCmd.Execute(); err != nil {
		t.Fatalf("execute install-executions inspect: %v", err)
	}
	if !strings.Contains(inspectOut.String(), "install-execution: "+execution.Path) || !strings.Contains(inspectOut.String(), "mode=execute") {
		t.Fatalf("expected inspected execution output, got %q", inspectOut.String())
	}
}

func TestToolchainInstallRetryPlanCommandPrintsMetadataOnlyFailedResults(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := app.InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	marker := filepath.Join(t.TempDir(), "retry-ran")
	execution := app.ToolchainInstallDispatchExecution{
		Status:       "failed",
		Mode:         "execute",
		DispatchPath: filepath.Join(root, "toolchain", "install-dispatches", "dispatch.json"),
		Results: []app.ToolchainInstallDispatchExecutionResult{
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
	}
	executionPath := writeCLIInstallExecutionFixture(t, root, "failed-execution.json", execution)

	var out bytes.Buffer
	cmd := newRootCommand(&out, new(bytes.Buffer))
	cmd.SetArgs([]string{"toolchain", "install-retry-plan", root})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute install-retry-plan: %v", err)
	}
	output := out.String()
	for _, want := range []string{
		"install-retry-plan: total=2 ready=1 skipped=1",
		"retry-install: " + executionPath,
		"pack=web-api-pack dependency=missing-httpx",
		"status=failed",
		"mode=metadata-only",
		"error=exit status 7",
		"skip-install-result: " + executionPath,
		"reason=not-failed: completed",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected install-retry-plan output to contain %q, got %q", want, output)
		}
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("install-retry-plan must not execute retry command, marker stat err=%v", err)
	}
}

func TestToolchainInstallRetryDispatchCommandWritesDryRunArtifact(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := app.InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	marker := filepath.Join(t.TempDir(), "retry-dispatch-ran")
	execution := app.ToolchainInstallDispatchExecution{
		Status:       "failed",
		Mode:         "execute",
		DispatchPath: filepath.Join(root, "toolchain", "install-dispatches", "dispatch.json"),
		Results: []app.ToolchainInstallDispatchExecutionResult{
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
	}
	writeCLIInstallExecutionFixture(t, root, "failed-execution.json", execution)

	var out bytes.Buffer
	cmd := newRootCommand(&out, new(bytes.Buffer))
	cmd.SetArgs([]string{"toolchain", "install-retry-dispatch", root})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute install-retry-dispatch: %v", err)
	}
	output := out.String()
	for _, want := range []string{
		"install-retry-dispatch:",
		"status=dry-run",
		"mode=metadata-only",
		"ready=1",
		"skipped=1",
		"retry-install:",
		"pack=web-api-pack dependency=missing-httpx",
		"skip-install-result:",
		"reason=not-failed: completed",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected install-retry-dispatch output to contain %q, got %q", want, output)
		}
	}
	paths, err := filepath.Glob(filepath.Join(root, "toolchain", "install-retry-dispatches", "*.json"))
	if err != nil {
		t.Fatalf("glob retry dispatch artifacts: %v", err)
	}
	if len(paths) != 1 {
		t.Fatalf("expected one retry dispatch artifact, got %#v", paths)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("install-retry-dispatch must not execute retry command, marker stat err=%v", err)
	}
}

func TestToolchainInstallRetryDispatchesCommandListsDryRunArtifacts(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := app.InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	marker := filepath.Join(t.TempDir(), "retry-dispatch-list-ran")
	execution := app.ToolchainInstallDispatchExecution{
		Status:       "failed",
		Mode:         "execute",
		DispatchPath: filepath.Join(root, "toolchain", "install-dispatches", "dispatch.json"),
		Results: []app.ToolchainInstallDispatchExecutionResult{{
			RequestPath: filepath.Join(root, "toolchain", "install-requests", "httpx.json"),
			Pack:        "web-api-pack",
			Dependency:  "missing-httpx",
			Command:     []string{"sh", "-c", "touch " + marker},
			Status:      "failed",
			Error:       "exit status 7",
		}},
	}
	writeCLIInstallExecutionFixture(t, root, "failed-execution.json", execution)
	dispatch, err := app.CreateToolchainInstallRetryDispatch(root)
	if err != nil {
		t.Fatalf("create retry dispatch: %v", err)
	}

	var out bytes.Buffer
	cmd := newRootCommand(&out, new(bytes.Buffer))
	cmd.SetArgs([]string{"toolchain", "install-retry-dispatches", root})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute install-retry-dispatches: %v", err)
	}
	output := out.String()
	for _, want := range []string{
		"install-retry-dispatches: total=1",
		"install-retry-dispatch: " + dispatch.Path,
		"status=dry-run",
		"mode=metadata-only",
		"ready=1",
		"retry-install:",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected install-retry-dispatches output to contain %q, got %q", want, output)
		}
	}

	var inspectOut bytes.Buffer
	inspectCmd := newRootCommand(&inspectOut, new(bytes.Buffer))
	inspectCmd.SetArgs([]string{"toolchain", "install-retry-dispatches", root, filepath.Base(dispatch.Path)})
	if err := inspectCmd.Execute(); err != nil {
		t.Fatalf("execute install-retry-dispatches inspect: %v", err)
	}
	if !strings.Contains(inspectOut.String(), "install-retry-dispatch: "+dispatch.Path) || !strings.Contains(inspectOut.String(), "mode=metadata-only") {
		t.Fatalf("expected inspected retry dispatch output, got %q", inspectOut.String())
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("install-retry-dispatches must not execute retry command, marker stat err=%v", err)
	}
}

func TestToolchainExecuteInstallRetryDispatchCommandRequiresExecuteFlag(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := app.InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	request := writeCLIInstallRequestFixture(t, root, app.ToolchainInstallRequest{
		Status:     "approved",
		Pack:       "web-api-pack",
		Dependency: "missing-httpx",
		Command:    []string{"sh", "-c", "true"},
	})
	marker := filepath.Join(t.TempDir(), "retry-ran")
	writeCLIInstallExecutionFixture(t, root, "failed-execution.json", app.ToolchainInstallDispatchExecution{
		Status:       "failed",
		Mode:         "execute",
		DispatchPath: filepath.Join(root, "toolchain", "install-dispatches", "dispatch.json"),
		Results: []app.ToolchainInstallDispatchExecutionResult{{
			RequestPath: request.Path,
			Pack:        "web-api-pack",
			Dependency:  "missing-httpx",
			Command:     []string{"sh", "-c", "touch " + marker},
			Status:      "failed",
			Error:       "exit status 7",
		}},
	})
	dispatch, err := app.CreateToolchainInstallRetryDispatch(root)
	if err != nil {
		t.Fatalf("create retry dispatch: %v", err)
	}

	var out bytes.Buffer
	cmd := newRootCommand(&out, new(bytes.Buffer))
	cmd.SetArgs([]string{"toolchain", "execute-install-retry-dispatch", root, filepath.Base(dispatch.Path)})
	err = cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "requires explicit execute") {
		t.Fatalf("expected explicit execute error, got %v output=%q", err, out.String())
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("execute-install-retry-dispatch without --execute must not run command, marker stat err=%v", err)
	}
}

func TestToolchainExecuteInstallRetryDispatchCommandRunsWithExecuteFlag(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := app.InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	request := writeCLIInstallRequestFixture(t, root, app.ToolchainInstallRequest{
		Status:     "approved",
		Pack:       "web-api-pack",
		Dependency: "missing-httpx",
		Command:    []string{"sh", "-c", "true"},
	})
	marker := filepath.Join(t.TempDir(), "retry-ran")
	writeCLIInstallExecutionFixture(t, root, "failed-execution.json", app.ToolchainInstallDispatchExecution{
		Status:       "failed",
		Mode:         "execute",
		DispatchPath: filepath.Join(root, "toolchain", "install-dispatches", "dispatch.json"),
		Results: []app.ToolchainInstallDispatchExecutionResult{{
			RequestPath: request.Path,
			Pack:        "web-api-pack",
			Dependency:  "missing-httpx",
			Command:     []string{"sh", "-c", "echo retried > " + marker + "; echo retried"},
			Status:      "failed",
			Error:       "exit status 7",
		}},
	})
	dispatch, err := app.CreateToolchainInstallRetryDispatch(root)
	if err != nil {
		t.Fatalf("create retry dispatch: %v", err)
	}

	var out bytes.Buffer
	cmd := newRootCommand(&out, new(bytes.Buffer))
	cmd.SetArgs([]string{"toolchain", "execute-install-retry-dispatch", root, filepath.Base(dispatch.Path), "--execute"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute retry dispatch: %v", err)
	}
	output := out.String()
	for _, want := range []string{
		"install-retry-dispatch-execution:",
		"status=completed",
		"mode=execute",
		"results=1",
		"retry-install-execute: pack=web-api-pack dependency=missing-httpx status=completed",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected retry execute output to contain %q, got %q", want, output)
		}
	}
	if data, err := os.ReadFile(marker); err != nil || strings.TrimSpace(string(data)) != "retried" {
		t.Fatalf("expected retry command to run, data=%q err=%v", data, err)
	}
}

func TestToolchainInstallRetryExecutionsCommandListsAuditArtifacts(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := app.InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	marker := filepath.Join(t.TempDir(), "retry-execution-list-ran")
	execution := app.ToolchainInstallRetryDispatchExecution{
		Status:            "completed",
		Mode:              "execute",
		RetryDispatchPath: filepath.Join(root, "toolchain", "install-retry-dispatches", "retry-dispatch.json"),
		Results: []app.ToolchainInstallRetryDispatchResult{{
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
	}
	executionPath := writeCLIInstallRetryExecutionFixture(t, root, "retry-execution.json", execution)

	var out bytes.Buffer
	cmd := newRootCommand(&out, new(bytes.Buffer))
	cmd.SetArgs([]string{"toolchain", "install-retry-executions", root})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute install-retry-executions: %v", err)
	}
	output := out.String()
	for _, want := range []string{
		"install-retry-executions: total=1",
		"install-retry-execution: " + executionPath,
		"status=completed",
		"mode=execute",
		"retry_dispatch=" + execution.RetryDispatchPath,
		"results=1",
		"retry-install-execute: pack=web-api-pack dependency=missing-httpx status=completed",
		"source_execution=" + execution.Results[0].ExecutionPath,
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected install-retry-executions output to contain %q, got %q", want, output)
		}
	}

	var inspectOut bytes.Buffer
	inspectCmd := newRootCommand(&inspectOut, new(bytes.Buffer))
	inspectCmd.SetArgs([]string{"toolchain", "install-retry-executions", root, filepath.Base(executionPath)})
	if err := inspectCmd.Execute(); err != nil {
		t.Fatalf("execute install-retry-executions inspect: %v", err)
	}
	if !strings.Contains(inspectOut.String(), "install-retry-execution: "+executionPath) || !strings.Contains(inspectOut.String(), "output=retried") {
		t.Fatalf("expected inspected retry execution output, got %q", inspectOut.String())
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("install-retry-executions must not execute retry command, marker stat err=%v", err)
	}
}

func TestToolchainInstallOrchestrateCommandWritesMetadataOnlyArtifact(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := app.InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	marker := filepath.Join(t.TempDir(), "orchestrate-cli-ran")
	writeCLIInstallRequestFixture(t, root, app.ToolchainInstallRequest{
		Status:     "approved",
		Pack:       "web-api-pack",
		Dependency: "missing-httpx",
		Command:    []string{"sh", "-c", "touch " + marker},
	})

	var out bytes.Buffer
	cmd := newRootCommand(&out, new(bytes.Buffer))
	cmd.SetArgs([]string{"toolchain", "install-orchestrate", root})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute install-orchestrate: %v", err)
	}
	output := out.String()
	for _, want := range []string{
		"install-orchestration:",
		"status=dry-run",
		"mode=metadata-only",
		"dispatch_ready=1",
		"retry_ready=0",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected install-orchestrate output to contain %q, got %q", want, output)
		}
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("metadata-only install-orchestrate must not execute command, marker stat err=%v", err)
	}
	paths, err := filepath.Glob(filepath.Join(root, "toolchain", "install-orchestrations", "*.json"))
	if err != nil {
		t.Fatalf("glob orchestrations: %v", err)
	}
	if len(paths) != 1 {
		t.Fatalf("expected one orchestration artifact, got %#v", paths)
	}
}

func TestToolchainInstallOrchestrateCommandExecuteRunsApprovedDispatch(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := app.InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	marker := filepath.Join(t.TempDir(), "orchestrate-cli-execute-ran")
	writeCLIInstallRequestFixture(t, root, app.ToolchainInstallRequest{
		Status:     "approved",
		Pack:       "web-api-pack",
		Dependency: "missing-httpx",
		Command:    []string{"sh", "-c", "echo cli-orchestrated > " + marker + "; echo cli-orchestrated"},
	})

	var out bytes.Buffer
	cmd := newRootCommand(&out, new(bytes.Buffer))
	cmd.SetArgs([]string{"toolchain", "install-orchestrate", root, "--execute"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute install-orchestrate --execute: %v", err)
	}
	output := out.String()
	for _, want := range []string{
		"install-orchestration:",
		"status=completed",
		"mode=execute",
		"dispatch_ready=1",
		"results=1",
		"install-dispatch=",
		"install-execution=",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected install-orchestrate --execute output to contain %q, got %q", want, output)
		}
	}
	if data, err := os.ReadFile(marker); err != nil || strings.TrimSpace(string(data)) != "cli-orchestrated" {
		t.Fatalf("expected execute orchestration command to run, data=%q err=%v", data, err)
	}
}

func TestToolchainInstallOrchestrationsCommandListsAndInspectsArtifacts(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := app.InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	writeCLIInstallRequestFixture(t, root, app.ToolchainInstallRequest{
		Status:     "approved",
		Pack:       "web-api-pack",
		Dependency: "missing-httpx",
		Command:    []string{"sh", "-c", "true"},
	})
	orchestration, err := app.CreateToolchainInstallOrchestration(root, app.ToolchainInstallOrchestrationOptions{})
	if err != nil {
		t.Fatalf("create orchestration: %v", err)
	}

	var out bytes.Buffer
	cmd := newRootCommand(&out, new(bytes.Buffer))
	cmd.SetArgs([]string{"toolchain", "install-orchestrations", root})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute install-orchestrations: %v", err)
	}
	output := out.String()
	for _, want := range []string{
		"install-orchestrations: total=1",
		"install-orchestration: " + orchestration.Path,
		"status=dry-run",
		"mode=metadata-only",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected install-orchestrations output to contain %q, got %q", want, output)
		}
	}

	var inspectOut bytes.Buffer
	inspectCmd := newRootCommand(&inspectOut, new(bytes.Buffer))
	inspectCmd.SetArgs([]string{"toolchain", "install-orchestrations", root, filepath.Base(orchestration.Path)})
	if err := inspectCmd.Execute(); err != nil {
		t.Fatalf("execute install-orchestrations inspect: %v", err)
	}
	if !strings.Contains(inspectOut.String(), "install-orchestration: "+orchestration.Path) || !strings.Contains(inspectOut.String(), "dispatch_ready=1") {
		t.Fatalf("expected inspected orchestration output, got %q", inspectOut.String())
	}
}

func TestRuntimeWorkerLeaseReclaimCommandWritesDryRunArtifact(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := app.InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	leasePath := writeCLIWorkerLeaseFixture(t, root, app.WorkerLeaseArtifact{
		Version:     1,
		RunID:       "run-stale",
		ExecutionID: "tool-stale",
		Tool:        "web-api-pack",
		Capability:  "web.discovery",
		WorkerID:    "local",
		Status:      "active",
		AcquiredAt:  time.Now().UTC().Add(-20 * time.Minute).Format(time.RFC3339Nano),
		HeartbeatAt: time.Now().UTC().Add(-20 * time.Minute).Format(time.RFC3339Nano),
	})

	var out bytes.Buffer
	cmd := newRootCommand(&out, new(bytes.Buffer))
	cmd.SetArgs([]string{"runtime", "worker-lease-reclaim", root})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute worker-lease-reclaim: %v", err)
	}
	output := out.String()
	for _, want := range []string{
		"worker-lease-reclaim:",
		"status=dry-run",
		"mode=metadata-only",
		"ready=1",
		"reclaim-worker-lease: run=run-stale execution=tool-stale",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected worker-lease-reclaim output to contain %q, got %q", want, output)
		}
	}
	paths, err := filepath.Glob(filepath.Join(root, "runtime", "worker-lease-reclaims", "*.json"))
	if err != nil {
		t.Fatalf("glob reclaim dispatch artifacts: %v", err)
	}
	if len(paths) != 1 {
		t.Fatalf("expected one reclaim dispatch artifact, got %#v", paths)
	}
	data, err := os.ReadFile(paths[0])
	if err != nil {
		t.Fatalf("read reclaim dispatch artifact: %v", err)
	}
	var dispatch app.WorkerLeaseReclaimDispatch
	if err := json.Unmarshal(data, &dispatch); err != nil {
		t.Fatalf("decode reclaim dispatch artifact: %v", err)
	}
	if dispatch.Status != "dry-run" || dispatch.Mode != "metadata-only" || len(dispatch.Ready) != 1 {
		t.Fatalf("unexpected reclaim dispatch artifact: %#v", dispatch)
	}
	leaseData, err := os.ReadFile(leasePath)
	if err != nil {
		t.Fatalf("read original worker lease: %v", err)
	}
	var lease app.WorkerLeaseArtifact
	if err := json.Unmarshal(leaseData, &lease); err != nil {
		t.Fatalf("decode original worker lease: %v", err)
	}
	if lease.Status != "active" || lease.ReleasedAt != "" {
		t.Fatalf("metadata-only reclaim dispatch must not mutate lease, got %#v", lease)
	}
}

func TestRuntimeWorkerLeaseReclaimsCommandListsDryRunArtifacts(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := app.InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	writeCLIWorkerLeaseFixture(t, root, app.WorkerLeaseArtifact{
		Version:     1,
		RunID:       "run-stale",
		ExecutionID: "tool-stale",
		Tool:        "web-api-pack",
		Capability:  "web.discovery",
		WorkerID:    "local",
		Status:      "active",
		AcquiredAt:  time.Now().UTC().Add(-20 * time.Minute).Format(time.RFC3339Nano),
		HeartbeatAt: time.Now().UTC().Add(-20 * time.Minute).Format(time.RFC3339Nano),
	})
	dispatch, err := app.CreateWorkerLeaseReclaimDispatch(root, time.Now().UTC(), 15*time.Minute)
	if err != nil {
		t.Fatalf("create reclaim dispatch: %v", err)
	}

	var out bytes.Buffer
	cmd := newRootCommand(&out, new(bytes.Buffer))
	cmd.SetArgs([]string{"runtime", "worker-lease-reclaims", root})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute worker-lease-reclaims: %v", err)
	}
	output := out.String()
	for _, want := range []string{
		"worker-lease-reclaims: total=1",
		"worker-lease-reclaim: " + dispatch.Path,
		"status=dry-run",
		"ready=1",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected worker-lease-reclaims output to contain %q, got %q", want, output)
		}
	}

	var inspectOut bytes.Buffer
	inspectCmd := newRootCommand(&inspectOut, new(bytes.Buffer))
	inspectCmd.SetArgs([]string{"runtime", "worker-lease-reclaims", root, filepath.Base(dispatch.Path)})
	if err := inspectCmd.Execute(); err != nil {
		t.Fatalf("execute worker-lease-reclaims inspect: %v", err)
	}
	if !strings.Contains(inspectOut.String(), "worker-lease-reclaim: "+dispatch.Path) || !strings.Contains(inspectOut.String(), "reclaim-worker-lease: run=run-stale execution=tool-stale") {
		t.Fatalf("expected inspected reclaim dispatch output, got %q", inspectOut.String())
	}
}

func TestRuntimeWorkerLeaseHeartbeatCommandRefreshesActiveLease(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := app.InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	now := time.Now().UTC()
	oldHeartbeat := now.Add(-app.DefaultWorkerLeaseStaleAfter - time.Minute).Format(time.RFC3339Nano)
	writeCLIWorkerLeaseFixture(t, root, app.WorkerLeaseArtifact{
		Version:     1,
		RunID:       "run-heartbeat-cli",
		ExecutionID: "tool-heartbeat-cli",
		Tool:        "web-api-pack",
		Capability:  "web.discovery",
		WorkerID:    "remote-web-1",
		Status:      "active",
		AcquiredAt:  oldHeartbeat,
		HeartbeatAt: oldHeartbeat,
	})

	var rejectedOut bytes.Buffer
	rejectedCmd := newRootCommand(&rejectedOut, new(bytes.Buffer))
	rejectedCmd.SetArgs([]string{"runtime", "worker-lease-heartbeat", root, "run-heartbeat-cli", "tool-heartbeat-cli"})
	if err := rejectedCmd.Execute(); err == nil || !strings.Contains(err.Error(), "requires explicit heartbeat") {
		t.Fatalf("expected missing heartbeat flag rejection, output=%q err=%v", rejectedOut.String(), err)
	}

	var out bytes.Buffer
	cmd := newRootCommand(&out, new(bytes.Buffer))
	cmd.SetArgs([]string{"runtime", "worker-lease-heartbeat", root, "run-heartbeat-cli", "tool-heartbeat-cli", "--heartbeat", "--heartbeat-by", "remote-web-1", "--reason", "still-running"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute worker lease heartbeat: %v", err)
	}
	output := out.String()
	for _, want := range []string{
		"worker-lease-heartbeat:",
		"run=run-heartbeat-cli",
		"execution=tool-heartbeat-cli",
		"status=active",
		"heartbeat_by=remote-web-1",
		"heartbeat_reason=still-running",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected worker lease heartbeat output to contain %q, got %q", want, output)
		}
	}
	leases, err := app.ListWorkerLeasesWithStaleness(root, time.Now().UTC(), app.DefaultWorkerLeaseStaleAfter)
	if err != nil {
		t.Fatalf("list worker leases with staleness: %v", err)
	}
	if len(leases) != 1 || leases[0].Stale || leases[0].HeartbeatBy != "remote-web-1" {
		t.Fatalf("expected refreshed non-stale lease, got %#v", leases)
	}
}

func TestRuntimeWorkerLeaseReleaseCommandCompletesActiveLease(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := app.InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	now := time.Now().UTC()
	oldHeartbeat := now.Add(-time.Minute).Format(time.RFC3339Nano)
	writeCLIWorkerLeaseFixture(t, root, app.WorkerLeaseArtifact{
		Version:     1,
		RunID:       "run-release-cli",
		ExecutionID: "tool-release-cli",
		Tool:        "web-api-pack",
		Capability:  "web.discovery",
		WorkerID:    "remote-web-1",
		Status:      "active",
		AcquiredAt:  oldHeartbeat,
		HeartbeatAt: oldHeartbeat,
	})

	var rejectedOut bytes.Buffer
	rejectedCmd := newRootCommand(&rejectedOut, new(bytes.Buffer))
	rejectedCmd.SetArgs([]string{"runtime", "worker-lease-release", root, "run-release-cli", "tool-release-cli", "--status", "completed"})
	if err := rejectedCmd.Execute(); err == nil || !strings.Contains(err.Error(), "requires explicit release") {
		t.Fatalf("expected missing release flag rejection, output=%q err=%v", rejectedOut.String(), err)
	}

	var out bytes.Buffer
	cmd := newRootCommand(&out, new(bytes.Buffer))
	cmd.SetArgs([]string{"runtime", "worker-lease-release", root, "run-release-cli", "tool-release-cli", "--release", "--status", "completed", "--released-by", "remote-web-1", "--reason", "finished", "--result-execution-id", "result-web-1", "--attempt-dir", "/workspace/attempts/run-release-cli/tool-release-cli"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute worker lease release: %v", err)
	}
	output := out.String()
	for _, want := range []string{
		"worker-lease-release:",
		"run=run-release-cli",
		"execution=tool-release-cli",
		"status=completed",
		"released_by=remote-web-1",
		"release_reason=finished",
		"result_execution=result-web-1",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected worker lease release output to contain %q, got %q", want, output)
		}
	}
	leases, err := app.ListWorkerLeasesWithStaleness(root, time.Now().UTC().Add(app.DefaultWorkerLeaseStaleAfter), app.DefaultWorkerLeaseStaleAfter)
	if err != nil {
		t.Fatalf("list worker leases with staleness: %v", err)
	}
	if len(leases) != 1 || leases[0].Status != "completed" || leases[0].Stale || leases[0].ReleasedBy != "remote-web-1" {
		t.Fatalf("expected completed non-stale lease, got %#v", leases)
	}
}

func TestRuntimeWorkerLeaseResultReconcileCommandShowsReturnedResultWithActiveLease(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	dispatch := createCLIFileOutboxWorkerQueueDispatch(t, root)
	result, err := app.SubmitRuntimeWorkerQueueDispatchResult(root, filepath.Base(dispatch.Path), app.RuntimeWorkerQueueDispatchResult{
		Status: "completed",
		Output: "remote result complete",
	}, app.RuntimeWorkerQueueDispatchResultOptions{Now: time.Now().UTC()})
	if err != nil {
		t.Fatalf("submit worker queue dispatch result: %v", err)
	}
	writeCLIWorkerLeaseFixture(t, root, app.WorkerLeaseArtifact{
		Version:     1,
		RunID:       dispatch.RunID,
		ExecutionID: dispatch.ExecutionID,
		Tool:        dispatch.Tool,
		Capability:  dispatch.Capability,
		WorkerID:    dispatch.WorkerID,
		Status:      "active",
		AcquiredAt:  time.Now().UTC().Add(-time.Minute).Format(time.RFC3339Nano),
		HeartbeatAt: time.Now().UTC().Add(-time.Minute).Format(time.RFC3339Nano),
	})

	var out bytes.Buffer
	cmd := newRootCommand(&out, new(bytes.Buffer))
	cmd.SetArgs([]string{"runtime", "worker-lease-result-reconcile", root})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute worker lease result reconcile: %v", err)
	}
	output := out.String()
	for _, want := range []string{
		"worker-lease-result-reconcile: ready=1 skipped=0 mode=metadata-only",
		"lease-result-reconcile: run=" + dispatch.RunID,
		"execution=" + dispatch.ExecutionID,
		"result=completed",
		"lease=active",
		"recommended=completed",
		"reason=result-returned-active-lease",
		"result_path=" + result.Path,
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected worker lease result reconcile output to contain %q, got %q", want, output)
		}
	}
}

func TestRuntimeWorkerLeaseResultReconcileCommandWritesDryRunArtifact(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	dispatch := createCLIFileOutboxWorkerQueueDispatch(t, root)
	if _, err := app.SubmitRuntimeWorkerQueueDispatchResult(root, filepath.Base(dispatch.Path), app.RuntimeWorkerQueueDispatchResult{
		Status: "failed",
		Error:  "remote worker failed",
	}, app.RuntimeWorkerQueueDispatchResultOptions{Now: time.Now().UTC()}); err != nil {
		t.Fatalf("submit worker queue dispatch result: %v", err)
	}
	writeCLIWorkerLeaseFixture(t, root, app.WorkerLeaseArtifact{
		Version:     1,
		RunID:       dispatch.RunID,
		ExecutionID: dispatch.ExecutionID,
		Tool:        dispatch.Tool,
		Capability:  dispatch.Capability,
		WorkerID:    dispatch.WorkerID,
		Status:      "active",
		AcquiredAt:  time.Now().UTC().Add(-time.Minute).Format(time.RFC3339Nano),
		HeartbeatAt: time.Now().UTC().Add(-time.Minute).Format(time.RFC3339Nano),
	})

	var out bytes.Buffer
	cmd := newRootCommand(&out, new(bytes.Buffer))
	cmd.SetArgs([]string{"runtime", "worker-lease-result-reconcile", root, "--write"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute worker lease result reconcile --write: %v", err)
	}
	output := out.String()
	for _, want := range []string{
		"worker-lease-result-reconciliation:",
		"status=dry-run",
		"mode=metadata-only",
		"ready=1",
		"skipped=0",
		"lease-result-reconciliation: run=" + dispatch.RunID,
		"recommended=failed",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected worker lease result reconciliation artifact output to contain %q, got %q", want, output)
		}
	}
	reviews, err := app.ListWorkerLeaseResultReconciliationDispatches(root)
	if err != nil {
		t.Fatalf("list worker lease result reconciliation reviews: %v", err)
	}
	if len(reviews) != 1 || reviews[0].Status != "dry-run" || reviews[0].Mode != "metadata-only" {
		t.Fatalf("expected one dry-run metadata-only review, got %#v", reviews)
	}
	lease, err := app.ListWorkerLeases(root)
	if err != nil {
		t.Fatalf("list worker leases: %v", err)
	}
	if len(lease) != 1 || lease[0].Status != "active" || lease[0].ReleasedAt != "" {
		t.Fatalf("metadata-only reconcile artifact must not mutate lease, got %#v", lease)
	}
}

func TestRuntimeWorkerLeaseResultReconciliationsCommandListsDryRunArtifacts(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	dispatch := createCLIFileOutboxWorkerQueueDispatch(t, root)
	if _, err := app.SubmitRuntimeWorkerQueueDispatchResult(root, filepath.Base(dispatch.Path), app.RuntimeWorkerQueueDispatchResult{
		Status: "completed",
		Output: "remote result complete",
	}, app.RuntimeWorkerQueueDispatchResultOptions{Now: time.Now().UTC()}); err != nil {
		t.Fatalf("submit worker queue dispatch result: %v", err)
	}
	writeCLIWorkerLeaseFixture(t, root, app.WorkerLeaseArtifact{
		Version:     1,
		RunID:       dispatch.RunID,
		ExecutionID: dispatch.ExecutionID,
		Tool:        dispatch.Tool,
		Capability:  dispatch.Capability,
		WorkerID:    dispatch.WorkerID,
		Status:      "active",
		AcquiredAt:  time.Now().UTC().Add(-time.Minute).Format(time.RFC3339Nano),
		HeartbeatAt: time.Now().UTC().Add(-time.Minute).Format(time.RFC3339Nano),
	})
	review, err := app.CreateWorkerLeaseResultReconciliationDispatch(root, time.Now().UTC())
	if err != nil {
		t.Fatalf("create worker lease result reconciliation review: %v", err)
	}

	var out bytes.Buffer
	cmd := newRootCommand(&out, new(bytes.Buffer))
	cmd.SetArgs([]string{"runtime", "worker-lease-result-reconciliations", root})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("list worker lease result reconciliations: %v", err)
	}
	output := out.String()
	if !strings.Contains(output, "worker-lease-result-reconciliations: total=1") || !strings.Contains(output, "worker-lease-result-reconciliation: "+review.Path) {
		t.Fatalf("expected worker lease result reconciliation listing, got %q", output)
	}

	var inspectOut bytes.Buffer
	inspectCmd := newRootCommand(&inspectOut, new(bytes.Buffer))
	inspectCmd.SetArgs([]string{"runtime", "worker-lease-result-reconciliations", root, filepath.Base(review.Path)})
	if err := inspectCmd.Execute(); err != nil {
		t.Fatalf("inspect worker lease result reconciliation: %v", err)
	}
	if !strings.Contains(inspectOut.String(), "worker-lease-result-reconciliation: "+review.Path) || !strings.Contains(inspectOut.String(), "lease-result-reconciliation: run="+dispatch.RunID) {
		t.Fatalf("expected inspected worker lease result reconciliation, got %q", inspectOut.String())
	}
}

func TestRuntimeWorkerLifecycleReviewCommandPrintsMetadataOnlySummary(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Now().UTC()
	writeCLIWorkerLeaseFixture(t, root, app.WorkerLeaseArtifact{
		Version:     1,
		RunID:       "run-stale-cli",
		ExecutionID: "tool-stale-cli",
		Tool:        "web-api-pack",
		Capability:  "web.discovery",
		WorkerID:    "local",
		Status:      "active",
		AcquiredAt:  now.Add(-30 * time.Minute).Format(time.RFC3339Nano),
		HeartbeatAt: now.Add(-30 * time.Minute).Format(time.RFC3339Nano),
	})
	writeCLIWorkerLeaseFixture(t, root, app.WorkerLeaseArtifact{
		Version:     1,
		RunID:       "run-done-cli",
		ExecutionID: "tool-done-cli",
		Tool:        "infra-pack",
		Capability:  "infra.host_discovery",
		WorkerID:    "local",
		Status:      "completed",
		AcquiredAt:  now.Add(-time.Minute).Format(time.RFC3339Nano),
		HeartbeatAt: now.Add(-time.Minute).Format(time.RFC3339Nano),
		ReleasedAt:  now.Add(-time.Minute).Format(time.RFC3339Nano),
	})
	createCLIFileOutboxWorkerQueueDispatch(t, root)

	var out bytes.Buffer
	cmd := newRootCommand(&out, new(bytes.Buffer))
	cmd.SetArgs([]string{"runtime", "worker-lifecycle-review", root})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute worker lifecycle review: %v", err)
	}
	output := out.String()
	for _, want := range []string{
		"worker-lifecycle-review: leases=2 active=1 stale=1 completed=1 failed=0 reclaimed=0",
		"reclaim_ready=1",
		"reclaim_skipped=1",
		"reconciliation_ready=0",
		"reconciliation_skipped=0",
		"queue_claims=1",
		"queue_dispatched=1",
		"transport_requests=1",
		"transport_pending=1",
		"dispatch_results=0",
		"mode=metadata-only",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected worker lifecycle review output to contain %q, got %q", want, output)
		}
	}
}

func TestRuntimeWorkerLifecycleReviewCommandWritesAndListsArtifact(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Now().UTC()
	writeCLIWorkerLeaseFixture(t, root, app.WorkerLeaseArtifact{
		Version:     1,
		RunID:       "run-review-cli",
		ExecutionID: "tool-review-cli",
		Tool:        "web-api-pack",
		Capability:  "web.discovery",
		WorkerID:    "local",
		Status:      "active",
		AcquiredAt:  now.Add(-30 * time.Minute).Format(time.RFC3339Nano),
		HeartbeatAt: now.Add(-30 * time.Minute).Format(time.RFC3339Nano),
	})

	var out bytes.Buffer
	cmd := newRootCommand(&out, new(bytes.Buffer))
	cmd.SetArgs([]string{"runtime", "worker-lifecycle-review", root, "--write"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute worker lifecycle review --write: %v", err)
	}
	output := out.String()
	for _, want := range []string{
		"worker-lifecycle-review-artifact:",
		"status=dry-run",
		"mode=metadata-only",
		"leases=1",
		"stale=1",
		"reclaim_ready=1",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected worker lifecycle review artifact output to contain %q, got %q", want, output)
		}
	}
	reviews, err := app.ListWorkerLifecycleReviews(root)
	if err != nil {
		t.Fatalf("list worker lifecycle reviews: %v", err)
	}
	if len(reviews) != 1 || reviews[0].Status != "dry-run" || reviews[0].Mode != "metadata-only" {
		t.Fatalf("expected one dry-run metadata-only lifecycle review, got %#v", reviews)
	}

	var listOut bytes.Buffer
	listCmd := newRootCommand(&listOut, new(bytes.Buffer))
	listCmd.SetArgs([]string{"runtime", "worker-lifecycle-reviews", root})
	if err := listCmd.Execute(); err != nil {
		t.Fatalf("list worker lifecycle reviews: %v", err)
	}
	if !strings.Contains(listOut.String(), "worker-lifecycle-reviews: total=1") || !strings.Contains(listOut.String(), "worker-lifecycle-review-artifact: "+reviews[0].Path) {
		t.Fatalf("expected worker lifecycle review listing, got %q", listOut.String())
	}

	var inspectOut bytes.Buffer
	inspectCmd := newRootCommand(&inspectOut, new(bytes.Buffer))
	inspectCmd.SetArgs([]string{"runtime", "worker-lifecycle-reviews", root, filepath.Base(reviews[0].Path)})
	if err := inspectCmd.Execute(); err != nil {
		t.Fatalf("inspect worker lifecycle review: %v", err)
	}
	if !strings.Contains(inspectOut.String(), "worker-lifecycle-review-artifact: "+reviews[0].Path) || !strings.Contains(inspectOut.String(), "reclaim_ready=1") {
		t.Fatalf("expected inspected worker lifecycle review, got %q", inspectOut.String())
	}
}

func TestRuntimeWorkerLifecycleActionPlanCommandWritesAndListsArtifact(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Now().UTC()
	writeCLIWorkerLeaseFixture(t, root, app.WorkerLeaseArtifact{
		Version:     1,
		RunID:       "run-action-cli",
		ExecutionID: "tool-action-cli",
		Tool:        "web-api-pack",
		Capability:  "web.discovery",
		WorkerID:    "local",
		Status:      "active",
		AcquiredAt:  now.Add(-30 * time.Minute).Format(time.RFC3339Nano),
		HeartbeatAt: now.Add(-30 * time.Minute).Format(time.RFC3339Nano),
	})
	createCLIFileOutboxWorkerQueueDispatch(t, root)
	review, err := app.CreateWorkerLifecycleReview(root, now, app.DefaultWorkerLeaseStaleAfter)
	if err != nil {
		t.Fatalf("create worker lifecycle review fixture: %v", err)
	}

	var out bytes.Buffer
	cmd := newRootCommand(&out, new(bytes.Buffer))
	cmd.SetArgs([]string{"runtime", "worker-lifecycle-action-plan", root, filepath.Base(review.Path), "--write"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute worker lifecycle action plan --write: %v", err)
	}
	output := out.String()
	for _, want := range []string{
		"worker-lifecycle-action-plan-artifact:",
		"status=dry-run",
		"mode=metadata-only",
		"actions=2",
		"ack_pending=1",
		"reclaim=1",
		"review=" + review.Path,
		"worker-lifecycle-action: type=ack-transport-request gate=worker-transport-ack",
		"worker-lifecycle-action: type=reclaim-stale-lease gate=worker-lease-reclaim",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected worker lifecycle action plan output to contain %q, got %q", want, output)
		}
	}
	actionPlans, err := app.ListWorkerLifecycleActionPlans(root)
	if err != nil {
		t.Fatalf("list worker lifecycle action plans: %v", err)
	}
	if len(actionPlans) != 1 || actionPlans[0].Status != "dry-run" || actionPlans[0].Mode != "metadata-only" {
		t.Fatalf("expected one dry-run metadata-only action plan, got %#v", actionPlans)
	}

	var listOut bytes.Buffer
	listCmd := newRootCommand(&listOut, new(bytes.Buffer))
	listCmd.SetArgs([]string{"runtime", "worker-lifecycle-action-plans", root})
	if err := listCmd.Execute(); err != nil {
		t.Fatalf("list worker lifecycle action plans: %v", err)
	}
	if !strings.Contains(listOut.String(), "worker-lifecycle-action-plans: total=1") || !strings.Contains(listOut.String(), "worker-lifecycle-action-plan-artifact: "+actionPlans[0].Path) {
		t.Fatalf("expected worker lifecycle action plan listing, got %q", listOut.String())
	}

	var inspectOut bytes.Buffer
	inspectCmd := newRootCommand(&inspectOut, new(bytes.Buffer))
	inspectCmd.SetArgs([]string{"runtime", "worker-lifecycle-action-plans", root, filepath.Base(actionPlans[0].Path)})
	if err := inspectCmd.Execute(); err != nil {
		t.Fatalf("inspect worker lifecycle action plan: %v", err)
	}
	if !strings.Contains(inspectOut.String(), "worker-lifecycle-action-plan-artifact: "+actionPlans[0].Path) || !strings.Contains(inspectOut.String(), "ack_pending=1") {
		t.Fatalf("expected inspected worker lifecycle action plan, got %q", inspectOut.String())
	}
}

func TestRuntimeWorkerLifecycleActionPlanReviewSelectionCommands(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Now().UTC()
	selectionPath := filepath.Join(root, "runtime", "worker-lifecycle-action-plan-selection.json")
	createCLIFileOutboxWorkerQueueDispatch(t, root)
	plan, err := app.CreateWorkerLifecycleActionPlan(root, "", now, app.DefaultWorkerLeaseStaleAfter)
	if err != nil {
		t.Fatalf("create worker lifecycle action plan fixture: %v", err)
	}

	var annotateOut bytes.Buffer
	annotateCmd := newRootCommand(&annotateOut, new(bytes.Buffer))
	annotateCmd.SetArgs([]string{
		"runtime", "worker-lifecycle-action-plan-annotate", root, filepath.Base(plan.Path),
		"--decision", "accepted",
		"--reviewer", "alice",
		"--reviewed-at", "2026-06-02T09:12:00Z",
		"--reason", "accepted authorized lifecycle action plan",
	})
	if err := annotateCmd.Execute(); err != nil {
		t.Fatalf("annotate worker lifecycle action plan: %v", err)
	}
	if !strings.Contains(annotateOut.String(), "worker-lifecycle-action-plan-annotation: decision=accepted reviewer=alice reviewed_at=2026-06-02T09:12:00Z reason=accepted_authorized_lifecycle_action_plan") {
		t.Fatalf("expected annotation output, got %q", annotateOut.String())
	}

	var selectOut bytes.Buffer
	selectCmd := newRootCommand(&selectOut, new(bytes.Buffer))
	selectCmd.SetArgs([]string{
		"runtime", "worker-lifecycle-action-plan-select", root, filepath.Base(plan.Path),
		"--selected-by", "bob",
		"--selected-at", "2026-06-02T09:13:00Z",
		"--reason", "make current reviewed action plan",
	})
	if err := selectCmd.Execute(); err != nil {
		t.Fatalf("select worker lifecycle action plan: %v", err)
	}
	if !strings.Contains(selectOut.String(), "worker-lifecycle-action-plan-selection: status=current mode=metadata-only") ||
		!strings.Contains(selectOut.String(), "current=true") ||
		!strings.Contains(selectOut.String(), "selection_path="+selectionPath) ||
		!strings.Contains(selectOut.String(), "plan_path="+plan.Path) ||
		!strings.Contains(selectOut.String(), "selection="+selectionPath) ||
		!strings.Contains(selectOut.String(), "plan="+plan.Path) ||
		!strings.Contains(selectOut.String(), "selected_by=bob selected_at=2026-06-02T09:13:00Z") ||
		!strings.Contains(selectOut.String(), "reason=make_current_reviewed_action_plan") {
		t.Fatalf("expected current action-plan selection output, got %q", selectOut.String())
	}

	var readOut bytes.Buffer
	readCmd := newRootCommand(&readOut, new(bytes.Buffer))
	readCmd.SetArgs([]string{"runtime", "worker-lifecycle-action-plan-selection", root})
	if err := readCmd.Execute(); err != nil {
		t.Fatalf("read worker lifecycle action plan selection: %v", err)
	}
	if !strings.Contains(readOut.String(), "worker-lifecycle-action-plan-selection: status=current") || !strings.Contains(readOut.String(), "plan="+plan.Path) {
		t.Fatalf("expected persisted action-plan selection output, got %q", readOut.String())
	}
	if !strings.Contains(readOut.String(), "current=true") ||
		!strings.Contains(readOut.String(), "selection_path="+selectionPath) ||
		!strings.Contains(readOut.String(), "plan_path="+plan.Path) ||
		!strings.Contains(readOut.String(), "selection="+selectionPath) {
		t.Fatalf("expected persisted action-plan selection output to expose current marker state/path, got %q", readOut.String())
	}

	var clearOut bytes.Buffer
	clearCmd := newRootCommand(&clearOut, new(bytes.Buffer))
	clearCmd.SetArgs([]string{
		"runtime", "worker-lifecycle-action-plan-selection-clear", root,
		"--cleared-by", "carol",
		"--cleared-at", "2026-06-02T09:14:00Z",
		"--reason", "close false-positive action-plan focus",
	})
	if err := clearCmd.Execute(); err != nil {
		t.Fatalf("clear worker lifecycle action plan selection: %v", err)
	}
	if !strings.Contains(clearOut.String(), "worker-lifecycle-action-plan-selection: status=cleared mode=metadata-only") ||
		!strings.Contains(clearOut.String(), "current=false") ||
		!strings.Contains(clearOut.String(), "selection_path="+selectionPath) ||
		!strings.Contains(clearOut.String(), "plan_path="+plan.Path) ||
		!strings.Contains(clearOut.String(), "selection="+selectionPath) ||
		!strings.Contains(clearOut.String(), "cleared_by=carol cleared_at=2026-06-02T09:14:00Z") ||
		!strings.Contains(clearOut.String(), "reason=close_false-positive_action-plan_focus") {
		t.Fatalf("expected cleared action-plan selection output, got %q", clearOut.String())
	}
}

func TestRuntimeWorkerLifecycleActionPlanSelectRequiresAcceptedAnnotation(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	createCLIFileOutboxWorkerQueueDispatch(t, root)
	plan, err := app.CreateWorkerLifecycleActionPlan(root, "", time.Now().UTC(), app.DefaultWorkerLeaseStaleAfter)
	if err != nil {
		t.Fatalf("create worker lifecycle action plan fixture: %v", err)
	}

	var out bytes.Buffer
	cmd := newRootCommand(&out, new(bytes.Buffer))
	cmd.SetArgs([]string{"runtime", "worker-lifecycle-action-plan-select", root, filepath.Base(plan.Path)})
	err = cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "worker lifecycle action plan selection requires accepted annotation") {
		t.Fatalf("expected accepted annotation selection error, got %v output=%q", err, out.String())
	}
	if _, found, readErr := app.ReadWorkerLifecycleActionPlanSelection(root); readErr != nil || found {
		t.Fatalf("rejected CLI selection must not write marker: found=%v err=%v", found, readErr)
	}
}

func TestRuntimeWorkerLifecycleActionPlanCommandShowsPendingDaemonControlRecommendation(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Now().UTC()
	if _, err := app.CreateRuntimeWorkerTransportDaemonControlRequest(root, "remote-web-1", app.RuntimeWorkerTransportDaemonControlRequest{
		Action:      "restart",
		Reason:      "reload worker config",
		RequestedBy: "operator",
	}, app.RuntimeWorkerTransportDaemonControlRequestOptions{Now: now}); err != nil {
		t.Fatalf("create daemon control fixture: %v", err)
	}

	var out bytes.Buffer
	cmd := newRootCommand(&out, new(bytes.Buffer))
	cmd.SetArgs([]string{"runtime", "worker-lifecycle-action-plan", root})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute worker lifecycle action plan: %v", err)
	}
	output := out.String()
	for _, want := range []string{
		"worker-lifecycle-action-plan:",
		"actions=1",
		"daemon_control=1",
		"worker-lifecycle-action: type=apply-daemon-control gate=worker-transport-run-daemon-enable-control",
		"worker=remote-web-1",
		"reason=pending-daemon-control",
		"recommended=restart",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected daemon control action-plan output to contain %q, got %q", want, output)
		}
	}
	actionPlans, err := app.ListWorkerLifecycleActionPlans(root)
	if err != nil {
		t.Fatalf("list worker lifecycle action plans: %v", err)
	}
	if len(actionPlans) != 0 {
		t.Fatalf("non-write action-plan command must not persist artifacts, got %#v", actionPlans)
	}
}

func TestRuntimeWorkerLifecycleActionPlanCommandShowsWorkerDispatchRetryDispatchRecommendation(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Now().UTC()
	failedDispatch := createCLIFileOutboxWorkerQueueDispatchWithToolPlan(t, root, app.RuntimeWorkerToolPlanPayload{
		Action:     "discover_assets",
		Runner:     "command",
		Parser:     "web_api_parser",
		RunCommand: []string{"sh", "-c", "echo retry"},
	})
	if _, err := app.SubmitRuntimeWorkerQueueDispatchResult(root, filepath.Base(failedDispatch.Path), app.RuntimeWorkerQueueDispatchResult{
		Status: "failed",
		Error:  "remote failed",
	}, app.RuntimeWorkerQueueDispatchResultOptions{Now: now}); err != nil {
		t.Fatalf("submit failed worker queue dispatch result: %v", err)
	}
	retryPlan, err := app.CreateRuntimeWorkerDispatchRetryPlan(root, now.Add(time.Minute))
	if err != nil {
		t.Fatalf("create worker dispatch retry plan fixture: %v", err)
	}
	retryDispatch, err := app.CreateRuntimeWorkerDispatchRetryDispatch(root, filepath.Base(retryPlan.Path), now.Add(2*time.Minute))
	if err != nil {
		t.Fatalf("create worker dispatch retry dispatch fixture: %v", err)
	}
	claimsBefore, err := app.ListRuntimeWorkerQueueClaims(root)
	if err != nil {
		t.Fatalf("list queue claims before action plan command: %v", err)
	}

	var out bytes.Buffer
	cmd := newRootCommand(&out, new(bytes.Buffer))
	cmd.SetArgs([]string{"runtime", "worker-lifecycle-action-plan", root})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute worker lifecycle action plan: %v", err)
	}
	output := out.String()
	for _, want := range []string{
		"worker-lifecycle-action-plan:",
		"actions=1",
		"retry_dispatch_execute=1",
		"worker-lifecycle-action: type=execute-dispatch-retry-dispatch gate=worker-dispatch-retry-dispatch-execute",
		"reason=ready-worker-dispatch-retry-dispatch",
		"dispatch=" + retryDispatch.Path,
		"recommended=execute",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected retry-dispatch action-plan output to contain %q, got %q", want, output)
		}
	}
	claimsAfter, err := app.ListRuntimeWorkerQueueClaims(root)
	if err != nil {
		t.Fatalf("list queue claims after action plan command: %v", err)
	}
	if len(claimsAfter) != len(claimsBefore) {
		t.Fatalf("action-plan command must not create retry queue claims: before=%d after=%d", len(claimsBefore), len(claimsAfter))
	}
	executions, err := app.ListRuntimeWorkerDispatchRetryDispatchExecutions(root)
	if err != nil {
		t.Fatalf("list retry-dispatch executions after action plan command: %v", err)
	}
	if len(executions) != 0 {
		t.Fatalf("action-plan command must not execute retry-dispatch artifacts, got %#v", executions)
	}
}

func TestRuntimeWorkerLifecycleAuditCommandWritesAndListsArtifact(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Now().UTC()
	dispatch := createCLIFileOutboxWorkerQueueDispatch(t, root)
	if _, err := app.SubmitRuntimeWorkerQueueDispatchResult(root, filepath.Base(dispatch.Path), app.RuntimeWorkerQueueDispatchResult{
		Status: "failed",
		Error:  "remote result failed",
	}, app.RuntimeWorkerQueueDispatchResultOptions{Now: now}); err != nil {
		t.Fatalf("submit worker queue dispatch result: %v", err)
	}
	retryPlan, err := app.CreateRuntimeWorkerDispatchRetryPlan(root, now.Add(30*time.Second))
	if err != nil {
		t.Fatalf("create worker dispatch retry plan fixture: %v", err)
	}
	retryDispatch, err := app.CreateRuntimeWorkerDispatchRetryDispatch(root, filepath.Base(retryPlan.Path), now.Add(45*time.Second))
	if err != nil {
		t.Fatalf("create worker dispatch retry dispatch fixture: %v", err)
	}
	retryExecution, err := app.ExecuteRuntimeWorkerDispatchRetryDispatch(root, filepath.Base(retryDispatch.Path), app.RuntimeWorkerDispatchRetryDispatchExecutionOptions{
		Execute: true,
		Now:     now.Add(50 * time.Second),
	})
	if err != nil {
		t.Fatalf("execute worker dispatch retry dispatch fixture: %v", err)
	}
	writeCLIWorkerLeaseFixture(t, root, app.WorkerLeaseArtifact{
		Version:     1,
		RunID:       "run-audit-cli",
		ExecutionID: "tool-audit-cli",
		Tool:        "web-api-pack",
		Capability:  "web.discovery",
		WorkerID:    "remote-web-1",
		Status:      "active",
		AcquiredAt:  now.Add(-30 * time.Minute).Format(time.RFC3339Nano),
		HeartbeatAt: now.Add(-30 * time.Minute).Format(time.RFC3339Nano),
	})
	review, err := app.CreateWorkerLifecycleReview(root, now, app.DefaultWorkerLeaseStaleAfter)
	if err != nil {
		t.Fatalf("create worker lifecycle review fixture: %v", err)
	}
	if _, err := app.CreateWorkerLifecycleActionPlan(root, filepath.Base(review.Path), now.Add(time.Minute), app.DefaultWorkerLeaseStaleAfter); err != nil {
		t.Fatalf("create worker lifecycle action plan fixture: %v", err)
	}
	if _, err := app.CreateWorkerLifecycleMaintenance(root, app.WorkerLifecycleMaintenanceOptions{
		Now:        now.Add(2 * time.Minute),
		StaleAfter: app.DefaultWorkerLeaseStaleAfter,
	}); err != nil {
		t.Fatalf("create worker lifecycle maintenance fixture: %v", err)
	}
	if _, err := app.CreateWorkerLifecycleMaintenanceSupervisor(root, app.WorkerLifecycleMaintenanceSupervisorOptions{
		Now:        now.Add(3 * time.Minute),
		StaleAfter: app.DefaultWorkerLeaseStaleAfter,
		MaxRuns:    2,
	}); err != nil {
		t.Fatalf("create worker lifecycle maintenance supervisor fixture: %v", err)
	}
	if _, err := app.CreateRuntimeWorkerOrchestrationSupervisor(root, app.RuntimeWorkerOrchestrationSupervisorOptions{
		Now:           now.Add(4 * time.Minute),
		RunIDPrefix:   "run-audit-cli-orchestration-supervisor",
		TransportMode: "metadata-only",
		MaxRuns:       2,
	}); err != nil {
		t.Fatalf("create worker orchestration supervisor fixture: %v", err)
	}
	policy, err := app.CreateWorkerLifecyclePolicy(root, app.WorkerLifecyclePolicyOptions{
		Now:                         now.Add(5 * time.Minute),
		StaleAfter:                  app.DefaultWorkerLeaseStaleAfter,
		WorkerStaleAfter:            app.DefaultRuntimeWorkerStaleAfter,
		PollInterval:                time.Second,
		MaxSupervisorRuns:           1,
		TransportMode:               "metadata-only",
		IncludeLifecycleMaintenance: true,
	})
	if err != nil {
		t.Fatalf("create worker lifecycle policy fixture: %v", err)
	}
	if _, err := app.SelectWorkerLifecyclePolicy(root, filepath.Base(policy.Path), app.WorkerLifecyclePolicySelectionOptions{
		SelectedBy: "audit-cli",
		SelectedAt: now.Add(6 * time.Minute),
		Reason:     "audit CLI policy visibility",
	}); err != nil {
		t.Fatalf("select worker lifecycle policy fixture: %v", err)
	}
	policyRun, err := app.CreateWorkerLifecyclePolicyRun(root, app.WorkerLifecyclePolicyRunOptions{
		Now:   now.Add(7 * time.Minute),
		Actor: "audit-cli",
	})
	if err != nil {
		t.Fatalf("create worker lifecycle policy run fixture: %v", err)
	}
	schedulerRun, err := app.CreateWorkerLifecyclePolicySchedulerRun(root, app.WorkerLifecyclePolicySchedulerRunOptions{
		Now:     now.Add(8 * time.Minute),
		Actor:   "audit-cli",
		MaxRuns: 1,
	})
	if err != nil {
		t.Fatalf("create worker lifecycle policy scheduler run fixture: %v", err)
	}

	var out bytes.Buffer
	cmd := newRootCommand(&out, new(bytes.Buffer))
	cmd.SetArgs([]string{"runtime", "worker-lifecycle-audit", root, "--write"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute worker lifecycle audit --write: %v", err)
	}
	output := out.String()
	for _, want := range []string{
		"worker-lifecycle-audit-artifact:",
		"status=dry-run",
		"mode=metadata-only",
		"workers=1",
		"queue_claims=2",
		"dispatches=1",
		"transport_requests=1",
		"results=1",
		"retry_plans=1",
		"retry_dispatches=1",
		"retry_dispatch_executions=1",
		"leases=1",
		"lifecycle_reviews=1",
		"action_plans=1",
		"lifecycle_policies=1",
		"policy_selections=1",
		"policy_runs=1",
		"policy_scheduler_runs=1",
		"maintenances=1",
		"maintenance_supervisors=1",
		"orchestration_supervisors=1",
		"actions=",
		"worker-lifecycle-audit-item: kind=transport-request",
		"worker-lifecycle-audit-item: kind=worker-dispatch-retry-plan",
		"worker-lifecycle-audit-item: kind=worker-dispatch-retry-dispatch",
		"worker-lifecycle-audit-item: kind=worker-dispatch-retry-dispatch-execution",
		"total=1",
		"ready=1",
		"claimed=1",
		"worker-lifecycle-audit-item: kind=worker-lifecycle-policy",
		"worker-lifecycle-audit-item: kind=worker-lifecycle-policy-selection",
		"worker-lifecycle-audit-item: kind=worker-lifecycle-policy-run",
		"worker-lifecycle-audit-item: kind=worker-lifecycle-policy-scheduler-run",
		"worker-lifecycle-audit-item: kind=worker-lifecycle-maintenance",
		"worker-lifecycle-audit-item: kind=worker-lifecycle-maintenance-supervisor",
		"worker-lifecycle-audit-item: kind=worker-orchestration-supervisor",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected worker lifecycle audit output to contain %q, got %q", want, output)
		}
	}
	audits, err := app.ListWorkerLifecycleAudits(root)
	if err != nil {
		t.Fatalf("list worker lifecycle audits: %v", err)
	}
	if len(audits) != 1 || audits[0].Status != "dry-run" || audits[0].Mode != "metadata-only" {
		t.Fatalf("expected one dry-run metadata-only audit, got %#v", audits)
	}
	if audits[0].Summary.DispatchRetryPlans != 1 || !workerLifecycleAuditOutputHasPath(audits[0], retryPlan.Path) {
		t.Fatalf("expected lifecycle audit to include dispatch retry plan %s, got %#v", retryPlan.Path, audits[0])
	}
	if audits[0].Summary.DispatchRetryDispatches != 1 || !workerLifecycleAuditOutputHasPath(audits[0], retryDispatch.Path) {
		t.Fatalf("expected lifecycle audit to include dispatch retry dispatch %s, got %#v", retryDispatch.Path, audits[0])
	}
	if audits[0].Summary.DispatchRetryDispatchExecutions != 1 || !workerLifecycleAuditOutputHasPath(audits[0], retryExecution.Path) {
		t.Fatalf("expected lifecycle audit to include dispatch retry dispatch execution %s, got %#v", retryExecution.Path, audits[0])
	}
	if audits[0].Summary.LifecyclePolicies != 1 || !workerLifecycleAuditOutputHasPath(audits[0], policy.Path) {
		t.Fatalf("expected lifecycle audit to include policy %s, got %#v", policy.Path, audits[0])
	}
	if audits[0].Summary.LifecyclePolicyRuns != 1 || !workerLifecycleAuditOutputHasPath(audits[0], policyRun.Path) {
		t.Fatalf("expected lifecycle audit to include policy run %s, got %#v", policyRun.Path, audits[0])
	}
	if audits[0].Summary.LifecyclePolicySchedulerRuns != 1 || !workerLifecycleAuditOutputHasPath(audits[0], schedulerRun.Path) {
		t.Fatalf("expected lifecycle audit to include policy scheduler run %s, got %#v", schedulerRun.Path, audits[0])
	}

	var listOut bytes.Buffer
	listCmd := newRootCommand(&listOut, new(bytes.Buffer))
	listCmd.SetArgs([]string{"runtime", "worker-lifecycle-audits", root})
	if err := listCmd.Execute(); err != nil {
		t.Fatalf("list worker lifecycle audits: %v", err)
	}
	if !strings.Contains(listOut.String(), "worker-lifecycle-audits: total=1") || !strings.Contains(listOut.String(), "worker-lifecycle-audit-artifact: "+audits[0].Path) {
		t.Fatalf("expected worker lifecycle audit listing, got %q", listOut.String())
	}

	var inspectOut bytes.Buffer
	inspectCmd := newRootCommand(&inspectOut, new(bytes.Buffer))
	inspectCmd.SetArgs([]string{"runtime", "worker-lifecycle-audits", root, filepath.Base(audits[0].Path)})
	if err := inspectCmd.Execute(); err != nil {
		t.Fatalf("inspect worker lifecycle audit: %v", err)
	}
	if !strings.Contains(inspectOut.String(), "worker-lifecycle-audit-artifact: "+audits[0].Path) || !strings.Contains(inspectOut.String(), "transport_requests=1") || !strings.Contains(inspectOut.String(), "retry_plans=1") || !strings.Contains(inspectOut.String(), "retry_dispatches=1") || !strings.Contains(inspectOut.String(), "lifecycle_policies=1") || !strings.Contains(inspectOut.String(), "policy_runs=1") || !strings.Contains(inspectOut.String(), "policy_scheduler_runs=1") || !strings.Contains(inspectOut.String(), "maintenances=1") || !strings.Contains(inspectOut.String(), "maintenance_supervisors=1") || !strings.Contains(inspectOut.String(), "orchestration_supervisors=1") {
		t.Fatalf("expected inspected worker lifecycle audit, got %q", inspectOut.String())
	}
}

func workerLifecycleAuditOutputHasPath(audit app.WorkerLifecycleAudit, path string) bool {
	for _, item := range audit.Items {
		if item.ArtifactPath == path {
			return true
		}
	}
	return false
}

func TestRuntimeWorkerLifecyclePolicyCommandPreviewsWithoutWriting(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	createCLIFileOutboxWorkerQueueDispatch(t, root)

	var out bytes.Buffer
	cmd := newRootCommand(&out, new(bytes.Buffer))
	cmd.SetArgs([]string{
		"runtime", "worker-lifecycle-policy", root,
		"--stale-after-seconds", "300",
		"--worker-stale-after-seconds", "120",
		"--poll-interval-ms", "0",
		"--max-runs", "2",
		"--transport", "http-json",
		"--include-lifecycle-maintenance",
		"--execute-lifecycle-retry-dispatches",
		"--execute-lifecycle-lease-recovery",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("preview worker lifecycle policy: %v", err)
	}
	output := out.String()
	for _, want := range []string{
		"worker-lifecycle-policy:",
		"status=dry-run",
		"mode=metadata-only",
		"stale_after_seconds=300",
		"worker_stale_after_seconds=120",
		"poll_interval_ms=0",
		"max_runs=2",
		"transport=http-json",
		"include_lifecycle_maintenance=true",
		"execute_lifecycle_retries=true",
		"execute_lease_recovery=true",
		"workers=1",
		"transport_requests=1",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected worker lifecycle policy preview output to contain %q, got %q", want, output)
		}
	}
	policies, err := app.ListWorkerLifecyclePolicies(root)
	if err != nil {
		t.Fatalf("list worker lifecycle policies: %v", err)
	}
	if len(policies) != 0 {
		t.Fatalf("preview policy command must not write artifacts, got %#v", policies)
	}
}

func TestRuntimeWorkerLifecyclePolicyCommandRejectsLeaseRecoveryWithoutLifecycleMaintenance(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := app.InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	t.Setenv("OURO_MISSING_WORKER_RECEIVER_TOKEN", "")

	var out bytes.Buffer
	var stderr bytes.Buffer
	cmd := newRootCommand(&out, &stderr)
	cmd.SetArgs([]string{
		"runtime", "worker-lifecycle-policy", root,
		"--stale-after-seconds", "300",
		"--worker-stale-after-seconds", "120",
		"--max-runs", "1",
		"--execute-lifecycle-lease-recovery",
	})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "worker lifecycle policy lease recovery execution requires lifecycle maintenance") {
		t.Fatalf("expected lease recovery lifecycle-maintenance gate error, got %v stdout=%q stderr=%q", err, out.String(), stderr.String())
	}
	if policies, listErr := app.ListWorkerLifecyclePolicies(root); listErr != nil {
		t.Fatalf("list worker lifecycle policies: %v", listErr)
	} else if len(policies) != 0 {
		t.Fatalf("invalid lease recovery policy must not write policy artifacts, got %#v", policies)
	}
}

func TestRuntimeWorkerLifecyclePolicyCommandWritesAndListsArtifactWithoutChildren(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	createCLIFileOutboxWorkerQueueDispatch(t, root)
	maintenancesBefore, err := app.ListWorkerLifecycleMaintenances(root)
	if err != nil {
		t.Fatalf("list worker lifecycle maintenances before policy: %v", err)
	}
	maintenanceSupervisorsBefore, err := app.ListWorkerLifecycleMaintenanceSupervisors(root)
	if err != nil {
		t.Fatalf("list worker lifecycle maintenance supervisors before policy: %v", err)
	}
	orchestrationSupervisorsBefore, err := app.ListRuntimeWorkerOrchestrationSupervisors(root)
	if err != nil {
		t.Fatalf("list worker orchestration supervisors before policy: %v", err)
	}
	claimsBefore, err := app.ListRuntimeWorkerQueueClaims(root)
	if err != nil {
		t.Fatalf("list worker queue claims before policy: %v", err)
	}
	requestsBefore, err := app.ListRuntimeWorkerTransportDispatchRequests(root, "")
	if err != nil {
		t.Fatalf("list worker transport requests before policy: %v", err)
	}

	var out bytes.Buffer
	cmd := newRootCommand(&out, new(bytes.Buffer))
	cmd.SetArgs([]string{
		"runtime", "worker-lifecycle-policy", root,
		"--write",
		"--stale-after-seconds", "300",
		"--worker-stale-after-seconds", "120",
		"--poll-interval-ms", "250",
		"--max-runs", "3",
		"--transport", "file-outbox",
		"--include-lifecycle-maintenance",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("write worker lifecycle policy: %v", err)
	}
	output := out.String()
	for _, want := range []string{
		"worker-lifecycle-policy-artifact:",
		"status=dry-run",
		"mode=metadata-only",
		"stale_after_seconds=300",
		"worker_stale_after_seconds=120",
		"poll_interval_ms=250",
		"max_runs=3",
		"transport=file-outbox",
		"include_lifecycle_maintenance=true",
		"execute_lifecycle_retries=false",
		"execute_lease_recovery=false",
		"workers=1",
		"transport_requests=1",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected worker lifecycle policy artifact output to contain %q, got %q", want, output)
		}
	}
	policies, err := app.ListWorkerLifecyclePolicies(root)
	if err != nil {
		t.Fatalf("list worker lifecycle policies: %v", err)
	}
	if len(policies) != 1 || policies[0].Status != "dry-run" || policies[0].Mode != "metadata-only" || policies[0].TransportMode != "file-outbox" {
		t.Fatalf("expected one dry-run metadata-only worker lifecycle policy, got %#v", policies)
	}

	var listOut bytes.Buffer
	listCmd := newRootCommand(&listOut, new(bytes.Buffer))
	listCmd.SetArgs([]string{"runtime", "worker-lifecycle-policies", root})
	if err := listCmd.Execute(); err != nil {
		t.Fatalf("list worker lifecycle policies: %v", err)
	}
	if !strings.Contains(listOut.String(), "worker-lifecycle-policies: total=1") || !strings.Contains(listOut.String(), "worker-lifecycle-policy-artifact: "+policies[0].Path) {
		t.Fatalf("expected worker lifecycle policy listing, got %q", listOut.String())
	}

	var inspectOut bytes.Buffer
	inspectCmd := newRootCommand(&inspectOut, new(bytes.Buffer))
	inspectCmd.SetArgs([]string{"runtime", "worker-lifecycle-policies", root, filepath.Base(policies[0].Path)})
	if err := inspectCmd.Execute(); err != nil {
		t.Fatalf("inspect worker lifecycle policy: %v", err)
	}
	if !strings.Contains(inspectOut.String(), "worker-lifecycle-policy-artifact: "+policies[0].Path) || !strings.Contains(inspectOut.String(), "transport=file-outbox") {
		t.Fatalf("expected inspected worker lifecycle policy, got %q", inspectOut.String())
	}

	maintenancesAfter, err := app.ListWorkerLifecycleMaintenances(root)
	if err != nil {
		t.Fatalf("list worker lifecycle maintenances after policy: %v", err)
	}
	maintenanceSupervisorsAfter, err := app.ListWorkerLifecycleMaintenanceSupervisors(root)
	if err != nil {
		t.Fatalf("list worker lifecycle maintenance supervisors after policy: %v", err)
	}
	orchestrationSupervisorsAfter, err := app.ListRuntimeWorkerOrchestrationSupervisors(root)
	if err != nil {
		t.Fatalf("list worker orchestration supervisors after policy: %v", err)
	}
	claimsAfter, err := app.ListRuntimeWorkerQueueClaims(root)
	if err != nil {
		t.Fatalf("list worker queue claims after policy: %v", err)
	}
	requestsAfter, err := app.ListRuntimeWorkerTransportDispatchRequests(root, "")
	if err != nil {
		t.Fatalf("list worker transport requests after policy: %v", err)
	}
	if len(maintenancesAfter) != len(maintenancesBefore) ||
		len(maintenanceSupervisorsAfter) != len(maintenanceSupervisorsBefore) ||
		len(orchestrationSupervisorsAfter) != len(orchestrationSupervisorsBefore) ||
		len(claimsAfter) != len(claimsBefore) ||
		len(requestsAfter) != len(requestsBefore) {
		t.Fatalf("policy write must not create child execution artifacts: maintenance %d/%d maintenance_supervisor %d/%d orchestration_supervisor %d/%d claims %d/%d requests %d/%d",
			len(maintenancesBefore), len(maintenancesAfter),
			len(maintenanceSupervisorsBefore), len(maintenanceSupervisorsAfter),
			len(orchestrationSupervisorsBefore), len(orchestrationSupervisorsAfter),
			len(claimsBefore), len(claimsAfter),
			len(requestsBefore), len(requestsAfter),
		)
	}
}

func TestRuntimeWorkerLifecyclePolicySelectionCommandsSelectReadAndClearMetadataOnly(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	createCLIFileOutboxWorkerQueueDispatch(t, root)
	now := time.Date(2026, 6, 1, 20, 0, 0, 0, time.UTC)
	base, err := app.CreateWorkerLifecyclePolicy(root, app.WorkerLifecyclePolicyOptions{
		Now:               now,
		StaleAfter:        5 * time.Minute,
		WorkerStaleAfter:  2 * time.Minute,
		PollInterval:      30 * time.Second,
		MaxSupervisorRuns: 2,
		TransportMode:     "metadata-only",
	})
	if err != nil {
		t.Fatalf("create base worker lifecycle policy: %v", err)
	}
	candidate, err := app.CreateWorkerLifecyclePolicy(root, app.WorkerLifecyclePolicyOptions{
		Now:               now.Add(time.Minute),
		StaleAfter:        10 * time.Minute,
		WorkerStaleAfter:  4 * time.Minute,
		PollInterval:      250 * time.Millisecond,
		MaxSupervisorRuns: 3,
		TransportMode:     "file-outbox",
	})
	if err != nil {
		t.Fatalf("create candidate worker lifecycle policy: %v", err)
	}
	var baseOut bytes.Buffer
	baseCmd := newRootCommand(&baseOut, new(bytes.Buffer))
	baseCmd.SetArgs([]string{
		"runtime", "worker-lifecycle-policy-select", root, filepath.Base(base.Path),
		"--selected-by", "alice",
		"--selected-at", "2026-06-01T20:05:00Z",
		"--reason", "initial policy",
	})
	if err := baseCmd.Execute(); err != nil {
		t.Fatalf("execute base worker-lifecycle-policy-select: %v", err)
	}

	var out bytes.Buffer
	cmd := newRootCommand(&out, new(bytes.Buffer))
	cmd.SetArgs([]string{
		"runtime", "worker-lifecycle-policy-select", root, filepath.Base(candidate.Path),
		"--selected-by", "bob",
		"--selected-at", "2026-06-01T20:06:00Z",
		"--reason", "prefer file outbox",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute worker-lifecycle-policy-select: %v", err)
	}
	for _, want := range []string{
		"worker-lifecycle-policy-selection: status=current mode=metadata-only policy=" + candidate.Path,
		"stale_after_seconds=600 worker_stale_after_seconds=240 poll_interval_ms=250 max_runs=3 transport=file-outbox",
		"include_lifecycle_maintenance=false execute_lifecycle_retries=false",
		"workers=1",
		"superseded=1 selected_by=bob selected_at=2026-06-01T20:06:00Z reason=prefer file outbox",
		"worker-lifecycle-policy-superseded: " + base.Path,
	} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("expected selection output to contain %q, got %q", want, out.String())
		}
	}

	var readOut bytes.Buffer
	readCmd := newRootCommand(&readOut, new(bytes.Buffer))
	readCmd.SetArgs([]string{"runtime", "worker-lifecycle-policy-selection", root})
	if err := readCmd.Execute(); err != nil {
		t.Fatalf("execute worker-lifecycle-policy-selection: %v", err)
	}
	if !strings.Contains(readOut.String(), "worker-lifecycle-policy-selection: status=current mode=metadata-only policy="+candidate.Path) {
		t.Fatalf("expected current selection read output, got %q", readOut.String())
	}

	var clearOut bytes.Buffer
	clearCmd := newRootCommand(&clearOut, new(bytes.Buffer))
	clearCmd.SetArgs([]string{
		"runtime", "worker-lifecycle-policy-selection-clear", root,
		"--cleared-by", "carol",
		"--cleared-at", "2026-06-01T20:07:00Z",
		"--reason", "authorization changed",
	})
	if err := clearCmd.Execute(); err != nil {
		t.Fatalf("execute worker-lifecycle-policy-selection-clear: %v", err)
	}
	for _, want := range []string{
		"worker-lifecycle-policy-selection: status=cleared mode=metadata-only policy=" + candidate.Path,
		"superseded=1 cleared_by=carol cleared_at=2026-06-01T20:07:00Z reason=authorization changed",
	} {
		if !strings.Contains(clearOut.String(), want) {
			t.Fatalf("expected cleared selection output to contain %q, got %q", want, clearOut.String())
		}
	}

	var readClearedOut bytes.Buffer
	readClearedCmd := newRootCommand(&readClearedOut, new(bytes.Buffer))
	readClearedCmd.SetArgs([]string{"runtime", "worker-lifecycle-policy-selection", root})
	if err := readClearedCmd.Execute(); err != nil {
		t.Fatalf("execute worker-lifecycle-policy-selection after clear: %v", err)
	}
	if !strings.Contains(readClearedOut.String(), "worker-lifecycle-policy-selection: status=cleared mode=metadata-only policy="+candidate.Path) {
		t.Fatalf("expected cleared selection read output, got %q", readClearedOut.String())
	}
	selection, found, err := app.ReadWorkerLifecyclePolicySelection(root)
	if err != nil {
		t.Fatalf("read worker lifecycle policy selection marker: %v", err)
	}
	if !found || selection.Status != "cleared" || selection.PolicyPath != candidate.Path || selection.ClearedBy != "carol" {
		t.Fatalf("unexpected persisted worker lifecycle policy selection found=%t selection=%#v", found, selection)
	}
}

func TestRuntimeWorkerLifecyclePolicyRunCommandWritesDryRunArtifactFromCurrentSelection(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	policy, selection := createCLIWorkerLifecyclePolicyRunSelection(t, root, app.WorkerLifecyclePolicyOptions{
		Now:                         time.Now().UTC(),
		StaleAfter:                  app.DefaultWorkerLeaseStaleAfter,
		WorkerStaleAfter:            app.DefaultRuntimeWorkerStaleAfter,
		PollInterval:                250 * time.Millisecond,
		MaxSupervisorRuns:           2,
		TransportMode:               "file-outbox",
		IncludeLifecycleMaintenance: true,
	})

	var out bytes.Buffer
	cmd := newRootCommand(&out, new(bytes.Buffer))
	cmd.SetArgs([]string{"runtime", "worker-lifecycle-policy-run", root})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute worker lifecycle policy dry-run: %v", err)
	}
	output := out.String()
	for _, want := range []string{
		"worker-lifecycle-policy-run-artifact:",
		"status=dry-run",
		"mode=metadata-only",
		"policy=" + policy.Path,
		"selection=" + selection.Path,
		"max_runs=2",
		"poll_interval_ms=250",
		"transport=file-outbox",
		"include_lifecycle_maintenance=true",
		"execute_lifecycle_retries=false",
		"supervisor_created=0",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected worker lifecycle policy dry-run output to contain %q, got %q", want, output)
		}
	}
	if strings.Contains(output, "worker-lifecycle-policy-run-supervisor:") {
		t.Fatalf("dry-run policy runner must not print supervisor paths, got %q", output)
	}
	runs := listCLIWorkerLifecyclePolicyRunArtifacts(t, root)
	if len(runs) != 1 {
		t.Fatalf("expected one policy-run artifact, got %v", runs)
	}
	supervisors, err := app.ListWorkerLifecycleMaintenanceSupervisors(root)
	if err != nil {
		t.Fatalf("list maintenance supervisors: %v", err)
	}
	if len(supervisors) != 0 {
		t.Fatalf("dry-run policy runner must not create supervisors, got %#v", supervisors)
	}
}

func TestRuntimeWorkerLifecyclePolicyRunCommandExecuteCreatesBoundedSupervisor(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	policy, selection := createCLIWorkerLifecyclePolicyRunSelection(t, root, app.WorkerLifecyclePolicyOptions{
		Now:                         time.Now().UTC(),
		StaleAfter:                  app.DefaultWorkerLeaseStaleAfter,
		WorkerStaleAfter:            app.DefaultRuntimeWorkerStaleAfter,
		PollInterval:                75 * time.Millisecond,
		MaxSupervisorRuns:           2,
		TransportMode:               "metadata-only",
		IncludeLifecycleMaintenance: true,
		ExecuteLifecycleRetries:     false,
	})

	var out bytes.Buffer
	cmd := newRootCommand(&out, new(bytes.Buffer))
	cmd.SetArgs([]string{"runtime", "worker-lifecycle-policy-run", root, "--execute"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute worker lifecycle policy runner: %v", err)
	}
	output := out.String()
	for _, want := range []string{
		"worker-lifecycle-policy-run-artifact:",
		"status=completed",
		"mode=metadata-only",
		"policy=" + policy.Path,
		"selection=" + selection.Path,
		"max_runs=2",
		"poll_interval_ms=75",
		"transport=metadata-only",
		"include_lifecycle_maintenance=true",
		"execute_lifecycle_retries=false",
		"supervisor_created=1",
		"worker-lifecycle-policy-run-supervisor:",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected worker lifecycle policy execution output to contain %q, got %q", want, output)
		}
	}
	runs := listCLIWorkerLifecyclePolicyRunArtifacts(t, root)
	if len(runs) != 1 {
		t.Fatalf("expected one policy-run artifact, got %v", runs)
	}
	supervisors, err := app.ListWorkerLifecycleMaintenanceSupervisors(root)
	if err != nil {
		t.Fatalf("list maintenance supervisors: %v", err)
	}
	if len(supervisors) != 1 || supervisors[0].Summary.MaxRuns != 2 || supervisors[0].Summary.Runs != 2 {
		t.Fatalf("expected one bounded supervisor using policy max runs, got %#v", supervisors)
	}
	if !strings.Contains(output, "worker-lifecycle-policy-run-supervisor: "+supervisors[0].Path) {
		t.Fatalf("expected linked supervisor path %q in output %q", supervisors[0].Path, output)
	}
}

func TestRuntimeWorkerLifecyclePolicyRunCommandRejectsMissingOrClearedSelectionBeforeWriting(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")

	var missingOut bytes.Buffer
	missingCmd := newRootCommand(&missingOut, new(bytes.Buffer))
	missingCmd.SetArgs([]string{"runtime", "worker-lifecycle-policy-run", root})
	err := missingCmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "worker lifecycle policy selection marker is required") {
		t.Fatalf("expected missing selection error, got err=%v output=%q", err, missingOut.String())
	}
	if runs := listCLIWorkerLifecyclePolicyRunArtifacts(t, root); len(runs) != 0 {
		t.Fatalf("missing selection must not write policy-run artifacts, got %v", runs)
	}

	createCLIWorkerLifecyclePolicyRunSelection(t, root, app.WorkerLifecyclePolicyOptions{
		Now:                         time.Now().UTC(),
		StaleAfter:                  app.DefaultWorkerLeaseStaleAfter,
		WorkerStaleAfter:            app.DefaultRuntimeWorkerStaleAfter,
		MaxSupervisorRuns:           1,
		TransportMode:               "metadata-only",
		IncludeLifecycleMaintenance: true,
	})
	if _, err := app.ClearWorkerLifecyclePolicySelection(root, app.WorkerLifecyclePolicySelectionClearOptions{
		ClearedBy: "test",
		Reason:    "disable policy runner",
	}); err != nil {
		t.Fatalf("clear worker lifecycle policy selection: %v", err)
	}

	var clearedOut bytes.Buffer
	clearedCmd := newRootCommand(&clearedOut, new(bytes.Buffer))
	clearedCmd.SetArgs([]string{"runtime", "worker-lifecycle-policy-run", root, "--execute"})
	err = clearedCmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "worker lifecycle policy selection must be current") {
		t.Fatalf("expected cleared selection error, got err=%v output=%q", err, clearedOut.String())
	}
	if runs := listCLIWorkerLifecyclePolicyRunArtifacts(t, root); len(runs) != 0 {
		t.Fatalf("cleared selection must not write policy-run artifacts, got %v", runs)
	}
	supervisors, listErr := app.ListWorkerLifecycleMaintenanceSupervisors(root)
	if listErr != nil {
		t.Fatalf("list maintenance supervisors: %v", listErr)
	}
	if len(supervisors) != 0 {
		t.Fatalf("cleared selection must not create supervisors, got %#v", supervisors)
	}
}

func TestRuntimeWorkerLifecyclePolicyRunsCommandListsAndReadsArtifacts(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	policy, _ := createCLIWorkerLifecyclePolicyRunSelection(t, root, app.WorkerLifecyclePolicyOptions{
		Now:                         time.Now().UTC(),
		StaleAfter:                  app.DefaultWorkerLeaseStaleAfter,
		WorkerStaleAfter:            app.DefaultRuntimeWorkerStaleAfter,
		PollInterval:                125 * time.Millisecond,
		MaxSupervisorRuns:           1,
		TransportMode:               "file-outbox",
		IncludeLifecycleMaintenance: true,
	})

	var runOut bytes.Buffer
	runCmd := newRootCommand(&runOut, new(bytes.Buffer))
	runCmd.SetArgs([]string{"runtime", "worker-lifecycle-policy-run", root})
	if err := runCmd.Execute(); err != nil {
		t.Fatalf("execute worker lifecycle policy runner dry-run: %v", err)
	}
	runs := listCLIWorkerLifecyclePolicyRunArtifacts(t, root)
	if len(runs) != 1 {
		t.Fatalf("expected one policy-run artifact, got %v", runs)
	}

	var listOut bytes.Buffer
	listCmd := newRootCommand(&listOut, new(bytes.Buffer))
	listCmd.SetArgs([]string{"runtime", "worker-lifecycle-policy-runs", root})
	if err := listCmd.Execute(); err != nil {
		t.Fatalf("list worker lifecycle policy runs: %v", err)
	}
	if !strings.Contains(listOut.String(), "worker-lifecycle-policy-runs: total=1") ||
		!strings.Contains(listOut.String(), "worker-lifecycle-policy-run-artifact: "+runs[0]) {
		t.Fatalf("expected policy-run listing, got %q", listOut.String())
	}

	var readOut bytes.Buffer
	readCmd := newRootCommand(&readOut, new(bytes.Buffer))
	readCmd.SetArgs([]string{"runtime", "worker-lifecycle-policy-runs", root, filepath.Base(runs[0])})
	if err := readCmd.Execute(); err != nil {
		t.Fatalf("read worker lifecycle policy run: %v", err)
	}
	for _, want := range []string{
		"worker-lifecycle-policy-run-artifact: " + runs[0],
		"policy=" + policy.Path,
		"poll_interval_ms=125",
		"supervisor_created=0",
	} {
		if !strings.Contains(readOut.String(), want) {
			t.Fatalf("expected policy-run read output to contain %q, got %q", want, readOut.String())
		}
	}
}

func TestRuntimeWorkerLifecyclePolicySchedulerRunCommandWritesDryRunArtifactFromCurrentSelection(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	policy, selection := createCLIWorkerLifecyclePolicyRunSelection(t, root, app.WorkerLifecyclePolicyOptions{
		Now:                         time.Now().UTC(),
		StaleAfter:                  app.DefaultWorkerLeaseStaleAfter,
		WorkerStaleAfter:            app.DefaultRuntimeWorkerStaleAfter,
		PollInterval:                250 * time.Millisecond,
		MaxSupervisorRuns:           2,
		TransportMode:               "file-outbox",
		IncludeLifecycleMaintenance: true,
	})

	var out bytes.Buffer
	cmd := newRootCommand(&out, new(bytes.Buffer))
	cmd.SetArgs([]string{"runtime", "worker-lifecycle-policy-scheduler-run", root, "--actor", "scheduler-test", "--reason", "operator audit"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute worker lifecycle policy scheduler dry-run: %v", err)
	}
	output := out.String()
	for _, want := range []string{
		"worker-lifecycle-policy-scheduler-run-artifact:",
		"status=dry-run",
		"mode=metadata-only",
		"policy_basename=" + filepath.Base(policy.Path),
		"policy_path=" + policy.Path,
		"selection_path=" + selection.Path,
		"poll_interval_ms=250",
		"max_runs=0",
		"policy_runs_created=0",
		"supervisor_created=0",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected worker lifecycle policy scheduler dry-run output to contain %q, got %q", want, output)
		}
	}
	for _, notWant := range []string{"resident", "endpoint", "target contact"} {
		if strings.Contains(output, notWant) {
			t.Fatalf("scheduler dry-run output must preserve safe boundary and not contain %q, got %q", notWant, output)
		}
	}
	if runs := listCLIWorkerLifecyclePolicyRunArtifacts(t, root); len(runs) != 0 {
		t.Fatalf("dry-run scheduler must not create policy-run artifacts, got %v", runs)
	}
	schedulerRuns := listCLIWorkerLifecyclePolicySchedulerRunArtifacts(t, root)
	if len(schedulerRuns) != 1 {
		t.Fatalf("expected one scheduler-run artifact, got %v", schedulerRuns)
	}
	supervisors, err := app.ListWorkerLifecycleMaintenanceSupervisors(root)
	if err != nil {
		t.Fatalf("list maintenance supervisors: %v", err)
	}
	if len(supervisors) != 0 {
		t.Fatalf("dry-run scheduler must not create supervisors, got %#v", supervisors)
	}
}

func TestRuntimeWorkerLifecyclePolicySchedulerRunCommandRejectsExecuteWithoutPositiveMaxRunsBeforeWriting(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
	}{
		{
			name: "missing max-runs",
			args: []string{"runtime", "worker-lifecycle-policy-scheduler-run", filepath.Join(t.TempDir(), "workspace"), "--execute"},
		},
		{
			name: "zero max-runs",
			args: []string{"runtime", "worker-lifecycle-policy-scheduler-run", filepath.Join(t.TempDir(), "workspace"), "--execute", "--max-runs", "0"},
		},
		{
			name: "negative max-runs",
			args: []string{"runtime", "worker-lifecycle-policy-scheduler-run", filepath.Join(t.TempDir(), "workspace"), "--execute", "--max-runs", "-1"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			cmd := newRootCommand(&out, new(bytes.Buffer))
			cmd.SetArgs(tc.args)
			err := cmd.Execute()
			if err == nil || !strings.Contains(err.Error(), "--execute requires --max-runs > 0") {
				t.Fatalf("expected execute max-runs validation error, got err=%v output=%q", err, out.String())
			}
			root := tc.args[2]
			if runs := listCLIWorkerLifecyclePolicySchedulerRunArtifacts(t, root); len(runs) != 0 {
				t.Fatalf("invalid execute scheduler command must not write scheduler-run artifacts, got %v", runs)
			}
			if policyRuns := listCLIWorkerLifecyclePolicyRunArtifacts(t, root); len(policyRuns) != 0 {
				t.Fatalf("invalid execute scheduler command must not write policy-run artifacts, got %v", policyRuns)
			}
			if strings.Contains(out.String(), "worker-lifecycle-policy-scheduler-run") {
				t.Fatalf("invalid execute scheduler command must not print scheduler output, got %q", out.String())
			}
		})
	}
}

func TestRuntimeWorkerLifecyclePolicySchedulerRunsCommandListsAndReadsArtifacts(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	policy, selection := createCLIWorkerLifecyclePolicyRunSelection(t, root, app.WorkerLifecyclePolicyOptions{
		Now:                         time.Now().UTC(),
		StaleAfter:                  app.DefaultWorkerLeaseStaleAfter,
		WorkerStaleAfter:            app.DefaultRuntimeWorkerStaleAfter,
		PollInterval:                125 * time.Millisecond,
		MaxSupervisorRuns:           1,
		TransportMode:               "metadata-only",
		IncludeLifecycleMaintenance: true,
	})

	var runOut bytes.Buffer
	runCmd := newRootCommand(&runOut, new(bytes.Buffer))
	runCmd.SetArgs([]string{"runtime", "worker-lifecycle-policy-scheduler-run", root})
	if err := runCmd.Execute(); err != nil {
		t.Fatalf("execute worker lifecycle policy scheduler dry-run: %v", err)
	}
	runs := listCLIWorkerLifecyclePolicySchedulerRunArtifacts(t, root)
	if len(runs) != 1 {
		t.Fatalf("expected one scheduler-run artifact, got %v", runs)
	}

	var listOut bytes.Buffer
	listCmd := newRootCommand(&listOut, new(bytes.Buffer))
	listCmd.SetArgs([]string{"runtime", "worker-lifecycle-policy-scheduler-runs", root})
	if err := listCmd.Execute(); err != nil {
		t.Fatalf("list worker lifecycle policy scheduler runs: %v", err)
	}
	if !strings.Contains(listOut.String(), "worker-lifecycle-policy-scheduler-runs: total=1") ||
		!strings.Contains(listOut.String(), "worker-lifecycle-policy-scheduler-run-artifact: "+runs[0]) {
		t.Fatalf("expected scheduler-run listing, got %q", listOut.String())
	}

	var readOut bytes.Buffer
	readCmd := newRootCommand(&readOut, new(bytes.Buffer))
	readCmd.SetArgs([]string{"runtime", "worker-lifecycle-policy-scheduler-runs", root, filepath.Base(runs[0])})
	if err := readCmd.Execute(); err != nil {
		t.Fatalf("read worker lifecycle policy scheduler run: %v", err)
	}
	for _, want := range []string{
		"worker-lifecycle-policy-scheduler-run-artifact: " + runs[0],
		"policy_basename=" + filepath.Base(policy.Path),
		"policy_path=" + policy.Path,
		"selection_path=" + selection.Path,
		"poll_interval_ms=125",
		"policy_runs_created=0",
		"supervisor_created=0",
	} {
		if !strings.Contains(readOut.String(), want) {
			t.Fatalf("expected scheduler-run read output to contain %q, got %q", want, readOut.String())
		}
	}
}

func TestRuntimeWorkerDispatchRetrySchedulerRunCommandWritesDryRunAuditOnly(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := app.InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	retryPlanDir := filepath.Join(root, "runtime", "worker-dispatch-retry-plans")
	retryDispatchDir := filepath.Join(root, "runtime", "worker-dispatch-retry-dispatches")
	retryExecutionDir := filepath.Join(root, "runtime", "worker-dispatch-retry-dispatch-executions")
	plansBefore := countCLIJSON(t, retryPlanDir)
	dispatchesBefore := countCLIJSON(t, retryDispatchDir)
	executionsBefore := countCLIJSON(t, retryExecutionDir)

	var out bytes.Buffer
	cmd := newRootCommand(&out, new(bytes.Buffer))
	cmd.SetArgs([]string{"runtime", "worker-dispatch-retry-scheduler-run", root, "--actor", "scheduler-test", "--reason", "operator audit"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute worker dispatch retry scheduler dry-run: %v", err)
	}
	output := out.String()
	for _, want := range []string{
		"worker-dispatch-retry-scheduler-run-artifact:",
		"status=dry-run",
		"mode=metadata-only",
		"max_runs=0",
		"dispatch_retry_claims=false",
		"retry_transport=metadata-only",
		"retry_plans_created=0",
		"retry_dispatches_created=0",
		"retry_dispatch_executions_created=0",
		"retry_dispatch_claims_dispatched=0",
		"retry_dispatch_transport_requests_created=0",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected worker dispatch retry scheduler dry-run output to contain %q, got %q", want, output)
		}
	}
	for _, notWant := range []string{"endpoint", "callback", "daemon", "run-loop", "token-env", "target"} {
		if strings.Contains(output, notWant) {
			t.Fatalf("scheduler dry-run output must preserve non-resident boundary and not contain %q, got %q", notWant, output)
		}
	}
	if runs := listCLIWorkerDispatchRetrySchedulerRunArtifacts(t, root); len(runs) != 1 {
		t.Fatalf("expected one dispatch retry scheduler-run artifact, got %v", runs)
	}
	if countCLIJSON(t, retryPlanDir) != plansBefore {
		t.Fatalf("dry-run scheduler must not create retry plan artifacts")
	}
	if countCLIJSON(t, retryDispatchDir) != dispatchesBefore {
		t.Fatalf("dry-run scheduler must not create retry dispatch artifacts")
	}
	if countCLIJSON(t, retryExecutionDir) != executionsBefore {
		t.Fatalf("dry-run scheduler must not create retry dispatch execution artifacts")
	}
}

func TestRuntimeWorkerDispatchRetrySchedulerRunCommandRejectsExecuteWithoutPositiveMaxRunsBeforeWriting(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
	}{
		{
			name: "missing max-runs",
			args: []string{"runtime", "worker-dispatch-retry-scheduler-run", filepath.Join(t.TempDir(), "workspace"), "--execute"},
		},
		{
			name: "zero max-runs",
			args: []string{"runtime", "worker-dispatch-retry-scheduler-run", filepath.Join(t.TempDir(), "workspace"), "--execute", "--max-runs", "0"},
		},
		{
			name: "negative max-runs",
			args: []string{"runtime", "worker-dispatch-retry-scheduler-run", filepath.Join(t.TempDir(), "workspace"), "--execute", "--max-runs", "-1"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			cmd := newRootCommand(&out, new(bytes.Buffer))
			cmd.SetArgs(tc.args)
			err := cmd.Execute()
			if err == nil || !strings.Contains(err.Error(), "--execute requires --max-runs > 0") {
				t.Fatalf("expected execute max-runs validation error, got err=%v output=%q", err, out.String())
			}
			root := tc.args[2]
			if runs := listCLIWorkerDispatchRetrySchedulerRunArtifacts(t, root); len(runs) != 0 {
				t.Fatalf("invalid execute scheduler command must not write scheduler-run artifacts, got %v", runs)
			}
			if strings.Contains(out.String(), "worker-dispatch-retry-scheduler-run") {
				t.Fatalf("invalid execute scheduler command must not print scheduler output, got %q", out.String())
			}
		})
	}
}

func TestRuntimeWorkerDispatchRetrySchedulerRunCommandRejectsUnsafeHandoffFlagsBeforeWriting(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{
			name: "http-json retry transport",
			args: []string{"runtime", "worker-dispatch-retry-scheduler-run", filepath.Join(t.TempDir(), "workspace"), "--retry-transport", "http-json"},
			want: "--retry-transport supports metadata-only or file-outbox",
		},
		{
			name: "handoff without execute",
			args: []string{"runtime", "worker-dispatch-retry-scheduler-run", filepath.Join(t.TempDir(), "workspace"), "--dispatch-retry-claims"},
			want: "--dispatch-retry-claims requires --execute",
		},
		{
			name: "non-positive lease",
			args: []string{"runtime", "worker-dispatch-retry-scheduler-run", filepath.Join(t.TempDir(), "workspace"), "--lease-seconds", "0"},
			want: "--lease-seconds must be positive",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			cmd := newRootCommand(&out, new(bytes.Buffer))
			cmd.SetArgs(tc.args)
			err := cmd.Execute()
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected error containing %q, got err=%v output=%q", tc.want, err, out.String())
			}
			root := tc.args[2]
			if runs := listCLIWorkerDispatchRetrySchedulerRunArtifacts(t, root); len(runs) != 0 {
				t.Fatalf("invalid scheduler handoff command must not write scheduler-run artifacts, got %v", runs)
			}
			if strings.Contains(out.String(), "worker-dispatch-retry-scheduler-run") {
				t.Fatalf("invalid scheduler handoff command must not print scheduler output, got %q", out.String())
			}
		})
	}
}

func TestRuntimeWorkerDispatchRetrySchedulerCommandExposesOnlyBoundedNonResidentFlags(t *testing.T) {
	cmd := newRootCommand(new(bytes.Buffer), new(bytes.Buffer))
	runtimeCmd, _, err := cmd.Find([]string{"runtime", "worker-dispatch-retry-scheduler-run"})
	if err != nil {
		t.Fatalf("find dispatch retry scheduler command: %v", err)
	}
	for _, name := range []string{"execute", "max-runs", "actor", "reason", "lease-seconds", "dispatch-retry-claims", "retry-transport"} {
		if runtimeCmd.Flags().Lookup(name) == nil {
			t.Fatalf("expected dispatch retry scheduler flag %q", name)
		}
	}
	for _, forbidden := range []string{"endpoint", "target", "ack", "result", "listen", "run-loop", "daemon", "token-env", "callback", "result-callback", "transport-token-env"} {
		if runtimeCmd.Flags().Lookup(forbidden) != nil {
			t.Fatalf("dispatch retry scheduler command must not expose forbidden flag %q", forbidden)
		}
	}
}

func TestRuntimeWorkerDispatchRetrySchedulerRunsCommandListsAndReadsArtifacts(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := app.InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}

	var runOut bytes.Buffer
	runCmd := newRootCommand(&runOut, new(bytes.Buffer))
	runCmd.SetArgs([]string{"runtime", "worker-dispatch-retry-scheduler-run", root})
	if err := runCmd.Execute(); err != nil {
		t.Fatalf("execute worker dispatch retry scheduler dry-run: %v", err)
	}
	runs := listCLIWorkerDispatchRetrySchedulerRunArtifacts(t, root)
	if len(runs) != 1 {
		t.Fatalf("expected one dispatch retry scheduler-run artifact, got %v", runs)
	}

	var listOut bytes.Buffer
	listCmd := newRootCommand(&listOut, new(bytes.Buffer))
	listCmd.SetArgs([]string{"runtime", "worker-dispatch-retry-scheduler-runs", root})
	if err := listCmd.Execute(); err != nil {
		t.Fatalf("list worker dispatch retry scheduler runs: %v", err)
	}
	if !strings.Contains(listOut.String(), "worker-dispatch-retry-scheduler-runs: total=1") ||
		!strings.Contains(listOut.String(), "worker-dispatch-retry-scheduler-run-artifact: "+runs[0]) {
		t.Fatalf("expected dispatch retry scheduler-run listing, got %q", listOut.String())
	}

	var readOut bytes.Buffer
	readCmd := newRootCommand(&readOut, new(bytes.Buffer))
	readCmd.SetArgs([]string{"runtime", "worker-dispatch-retry-scheduler-runs", root, filepath.Base(runs[0])})
	if err := readCmd.Execute(); err != nil {
		t.Fatalf("read worker dispatch retry scheduler run: %v", err)
	}
	for _, want := range []string{
		"worker-dispatch-retry-scheduler-run-artifact: " + runs[0],
		"status=dry-run",
		"mode=metadata-only",
		"max_runs=0",
		"dispatch_retry_claims=false",
		"retry_transport=metadata-only",
	} {
		if !strings.Contains(readOut.String(), want) {
			t.Fatalf("expected dispatch retry scheduler-run read output to contain %q, got %q", want, readOut.String())
		}
	}
}

func TestRuntimeWorkerLifecyclePolicyCommandRejectsInvalidBoundsAndTransport(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{
			name: "max-runs",
			args: []string{"runtime", "worker-lifecycle-policy", "workspace", "--max-runs", "0"},
			want: "--max-runs must be positive",
		},
		{
			name: "stale-after-seconds",
			args: []string{"runtime", "worker-lifecycle-policy", "workspace", "--stale-after-seconds", "0"},
			want: "--stale-after-seconds must be positive",
		},
		{
			name: "worker-stale-after-seconds",
			args: []string{"runtime", "worker-lifecycle-policy", "workspace", "--worker-stale-after-seconds", "0"},
			want: "--worker-stale-after-seconds must be positive",
		},
		{
			name: "poll-interval-ms",
			args: []string{"runtime", "worker-lifecycle-policy", "workspace", "--poll-interval-ms", "-1"},
			want: "--poll-interval-ms cannot be negative",
		},
		{
			name: "transport",
			args: []string{"runtime", "worker-lifecycle-policy", "workspace", "--transport", "smtp"},
			want: "unsupported worker queue dispatch transport: smtp",
		},
		{
			name: "retry-without-maintenance",
			args: []string{"runtime", "worker-lifecycle-policy", "workspace", "--execute-lifecycle-retry-dispatches"},
			want: "requires lifecycle maintenance",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			cmd := newRootCommand(&out, new(bytes.Buffer))
			cmd.SetArgs(tc.args)
			err := cmd.Execute()
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected error containing %q, got err=%v output=%q", tc.want, err, out.String())
			}
			if strings.Contains(out.String(), "worker-lifecycle-policy") {
				t.Fatalf("invalid policy command must not print policy output, got %q", out.String())
			}
		})
	}
}

func createCLIWorkerLifecyclePolicyRunSelection(t *testing.T, root string, options app.WorkerLifecyclePolicyOptions) (app.WorkerLifecyclePolicy, app.WorkerLifecyclePolicySelection) {
	t.Helper()

	policy, err := app.CreateWorkerLifecyclePolicy(root, options)
	if err != nil {
		t.Fatalf("create worker lifecycle policy: %v", err)
	}
	selection, err := app.SelectWorkerLifecyclePolicy(root, filepath.Base(policy.Path), app.WorkerLifecyclePolicySelectionOptions{
		SelectedBy: "test",
		Reason:     "policy runner",
	})
	if err != nil {
		t.Fatalf("select worker lifecycle policy: %v", err)
	}
	return policy, selection
}

func listCLIWorkerLifecyclePolicyRunArtifacts(t *testing.T, root string) []string {
	t.Helper()

	paths, err := filepath.Glob(filepath.Join(root, "runtime", "worker-lifecycle-policy-runs", "*.json"))
	if err != nil {
		t.Fatalf("glob worker lifecycle policy runs: %v", err)
	}
	return paths
}

func listCLIWorkerLifecyclePolicySchedulerRunArtifacts(t *testing.T, root string) []string {
	t.Helper()

	paths, err := filepath.Glob(filepath.Join(root, "runtime", "worker-lifecycle-policy-scheduler-runs", "*.json"))
	if err != nil {
		t.Fatalf("glob worker lifecycle policy scheduler runs: %v", err)
	}
	return paths
}

func listCLIWorkerDispatchRetrySchedulerRunArtifacts(t *testing.T, root string) []string {
	t.Helper()

	paths, err := filepath.Glob(filepath.Join(root, "runtime", "worker-dispatch-retry-scheduler-runs", "*.json"))
	if err != nil {
		t.Fatalf("glob worker dispatch retry scheduler runs: %v", err)
	}
	return paths
}

func createReportProjectionDispatchExecutionForCLI(t *testing.T, root string) app.ReportProjectionDispatchExecution {
	t.Helper()

	if err := app.InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	run, err := app.RunWorkspaceWithOptions(context.Background(), root, app.RunOptions{DeferReportProjection: true})
	if err != nil {
		t.Fatalf("run workspace with deferred report projection: %v", err)
	}
	dispatch, err := app.CreateReportProjectionDispatch(context.Background(), root, run.RunID, app.ReportProjectionDispatchOptions{
		Now:    time.Date(2026, 6, 1, 10, 0, 0, 0, time.UTC),
		Actor:  "alice",
		Reason: "operator-review",
	})
	if err != nil {
		t.Fatalf("create report projection dispatch: %v", err)
	}
	execution, err := app.ExecuteReportProjectionDispatch(context.Background(), root, filepath.Base(dispatch.Path), app.ReportProjectionDispatchExecutionOptions{
		Execute: true,
		Now:     time.Date(2026, 6, 1, 10, 5, 0, 0, time.UTC),
		Actor:   "bob",
		Reason:  "approved-projection",
	})
	if err != nil {
		t.Fatalf("execute report projection dispatch: %v", err)
	}
	if execution.Status != "completed" || !execution.Written {
		t.Fatalf("expected completed written report execution fixture, got %#v", execution)
	}
	annotated, err := app.AnnotateReportProjectionDispatchExecution(root, filepath.Base(execution.Path), app.ReportProjectionDispatchExecutionAnnotationOptions{
		Decision:   "accepted",
		Reviewer:   "fixture",
		ReviewedAt: time.Date(2026, 6, 1, 10, 6, 0, 0, time.UTC),
		Reason:     "accepted-for-cli-fixture",
	})
	if err != nil {
		t.Fatalf("accept report projection dispatch execution fixture: %v", err)
	}
	return annotated
}

func writeReportProjectionDispatchExecutionFixture(t *testing.T, root string, name string, execution app.ReportProjectionDispatchExecution) string {
	t.Helper()

	dir := filepath.Join(root, "reports", "report-projection-dispatch-executions")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("create report projection dispatch execution fixture dir: %v", err)
	}
	path := filepath.Join(dir, name)
	execution.Path = path
	data, err := json.MarshalIndent(execution, "", "  ")
	if err != nil {
		t.Fatalf("marshal report projection dispatch execution fixture: %v", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write report projection dispatch execution fixture: %v", err)
	}
	return path
}

func countCLIJSON(t *testing.T, dir string) int {
	t.Helper()

	paths, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		t.Fatalf("glob json under %s: %v", dir, err)
	}
	return len(paths)
}

func assertFileUnchanged(t *testing.T, path string, before os.FileInfo, beforeData []byte, operation string) {
	t.Helper()

	after, err := os.Stat(path)
	if err != nil {
		t.Fatalf("%s must leave %s readable: %v", operation, path, err)
	}
	if after.Size() != before.Size() || !after.ModTime().Equal(before.ModTime()) {
		t.Fatalf("%s must not mutate %s, before size=%d mod=%s after size=%d mod=%s", operation, path, before.Size(), before.ModTime().Format(time.RFC3339Nano), after.Size(), after.ModTime().Format(time.RFC3339Nano))
	}
	afterData, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s after %s: %v", path, operation, err)
	}
	if !bytes.Equal(afterData, beforeData) {
		t.Fatalf("%s must not change %s content", operation, path)
	}
}

func TestRuntimeWorkerLifecycleMaintenanceCommandExecutesMetadataOnlyRecoveryReviews(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Now().UTC()
	writeCLIWorkerLeaseFixture(t, root, app.WorkerLeaseArtifact{
		Version:     1,
		RunID:       "run-maintenance-cli",
		ExecutionID: "tool-maintenance-cli",
		Tool:        "web-api-pack",
		Capability:  "web.discovery",
		WorkerID:    "local",
		Status:      "active",
		AcquiredAt:  now.Add(-30 * time.Minute).Format(time.RFC3339Nano),
		HeartbeatAt: now.Add(-30 * time.Minute).Format(time.RFC3339Nano),
	})
	createCLIFileOutboxWorkerQueueDispatch(t, root)

	var out bytes.Buffer
	cmd := newRootCommand(&out, new(bytes.Buffer))
	cmd.SetArgs([]string{"runtime", "worker-lifecycle-maintenance", root, "--execute"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute worker lifecycle maintenance: %v", err)
	}
	output := out.String()
	for _, want := range []string{
		"worker-lifecycle-maintenance-artifact:",
		"status=completed",
		"mode=metadata-only",
		"actions=2",
		"ack_actions=1",
		"reclaim_actions=1",
		"skipped_unsafe=1",
		"review_created=1",
		"action_plan_created=1",
		"reclaim_created=1",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected worker lifecycle maintenance output to contain %q, got %q", want, output)
		}
	}
	maintenances, err := app.ListWorkerLifecycleMaintenances(root)
	if err != nil {
		t.Fatalf("list worker lifecycle maintenances: %v", err)
	}
	if len(maintenances) != 1 || maintenances[0].Status != "completed" || maintenances[0].Summary.ReclaimDispatchArtifactsCreated != 1 {
		t.Fatalf("expected one completed metadata-only maintenance, got %#v", maintenances)
	}

	var listOut bytes.Buffer
	listCmd := newRootCommand(&listOut, new(bytes.Buffer))
	listCmd.SetArgs([]string{"runtime", "worker-lifecycle-maintenances", root})
	if err := listCmd.Execute(); err != nil {
		t.Fatalf("list worker lifecycle maintenances: %v", err)
	}
	if !strings.Contains(listOut.String(), "worker-lifecycle-maintenances: total=1") || !strings.Contains(listOut.String(), "worker-lifecycle-maintenance-artifact: "+maintenances[0].Path) {
		t.Fatalf("expected worker lifecycle maintenance listing, got %q", listOut.String())
	}

	var inspectOut bytes.Buffer
	inspectCmd := newRootCommand(&inspectOut, new(bytes.Buffer))
	inspectCmd.SetArgs([]string{"runtime", "worker-lifecycle-maintenances", root, filepath.Base(maintenances[0].Path)})
	if err := inspectCmd.Execute(); err != nil {
		t.Fatalf("inspect worker lifecycle maintenance: %v", err)
	}
	if !strings.Contains(inspectOut.String(), "worker-lifecycle-maintenance-artifact: "+maintenances[0].Path) || !strings.Contains(inspectOut.String(), "reclaim_created=1") {
		t.Fatalf("expected inspected worker lifecycle maintenance, got %q", inspectOut.String())
	}
}

func TestRuntimeWorkerLifecycleMaintenanceCommandSkipsRetryDispatchExecution(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	marker := filepath.Join(t.TempDir(), "maintenance-retry-dispatch-cli-ran")
	failedDispatch := createCLIFileOutboxWorkerQueueDispatchWithToolPlan(t, root, app.RuntimeWorkerToolPlanPayload{
		Action:     "discover_assets",
		Runner:     "command",
		Parser:     "web_api_parser",
		RunCommand: []string{"sh", "-c", "touch " + marker},
	})
	if _, err := app.SubmitRuntimeWorkerQueueDispatchResult(root, filepath.Base(failedDispatch.Path), app.RuntimeWorkerQueueDispatchResult{
		Status: "failed",
		Error:  "exit status 23",
	}, app.RuntimeWorkerQueueDispatchResultOptions{Now: time.Now().UTC()}); err != nil {
		t.Fatalf("submit failed worker queue dispatch result: %v", err)
	}
	plan, err := app.CreateRuntimeWorkerDispatchRetryPlan(root, time.Now().UTC())
	if err != nil {
		t.Fatalf("create worker dispatch retry plan: %v", err)
	}
	retryDispatch, err := app.CreateRuntimeWorkerDispatchRetryDispatch(root, filepath.Base(plan.Path), time.Now().UTC())
	if err != nil {
		t.Fatalf("create worker dispatch retry dispatch: %v", err)
	}
	claimsBefore, err := app.ListRuntimeWorkerQueueClaims(root)
	if err != nil {
		t.Fatalf("list queue claims before maintenance: %v", err)
	}

	var out bytes.Buffer
	cmd := newRootCommand(&out, new(bytes.Buffer))
	cmd.SetArgs([]string{"runtime", "worker-lifecycle-maintenance", root, "--execute"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute worker lifecycle maintenance: %v", err)
	}
	output := out.String()
	for _, want := range []string{
		"worker-lifecycle-maintenance-artifact:",
		"actions=1",
		"retry_dispatch_execute=1",
		"skipped_unsafe=1",
		"worker-lifecycle-maintenance-skip: type=execute-dispatch-retry-dispatch gate=worker-dispatch-retry-dispatch-execute",
		"dispatch=" + retryDispatch.Path,
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected retry-dispatch maintenance output to contain %q, got %q", want, output)
		}
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("maintenance must not execute retry tool-plan command, marker stat err=%v", err)
	}
	claimsAfter, err := app.ListRuntimeWorkerQueueClaims(root)
	if err != nil {
		t.Fatalf("list queue claims after maintenance: %v", err)
	}
	if len(claimsAfter) != len(claimsBefore) {
		t.Fatalf("maintenance must not create retry queue claims: before=%d after=%d", len(claimsBefore), len(claimsAfter))
	}
	if executions, err := app.ListRuntimeWorkerDispatchRetryDispatchExecutions(root); err != nil {
		t.Fatalf("list retry-dispatch executions after maintenance: %v", err)
	} else if len(executions) != 0 {
		t.Fatalf("maintenance must not execute retry dispatch artifacts, got %#v", executions)
	}
}

func TestRuntimeWorkerLifecycleMaintenanceCommandExecutesRetryDispatchesWithExplicitFlag(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	marker := filepath.Join(t.TempDir(), "maintenance-retry-dispatch-cli-explicit-ran")
	failedDispatch := createCLIFileOutboxWorkerQueueDispatchWithToolPlan(t, root, app.RuntimeWorkerToolPlanPayload{
		Action:     "discover_assets",
		Runner:     "command",
		Parser:     "web_api_parser",
		RunCommand: []string{"sh", "-c", "touch " + marker},
	})
	if _, err := app.SubmitRuntimeWorkerQueueDispatchResult(root, filepath.Base(failedDispatch.Path), app.RuntimeWorkerQueueDispatchResult{
		Status: "failed",
		Error:  "exit status 23",
	}, app.RuntimeWorkerQueueDispatchResultOptions{Now: time.Now().UTC()}); err != nil {
		t.Fatalf("submit failed worker queue dispatch result: %v", err)
	}
	plan, err := app.CreateRuntimeWorkerDispatchRetryPlan(root, time.Now().UTC())
	if err != nil {
		t.Fatalf("create worker dispatch retry plan: %v", err)
	}
	if _, err := app.CreateRuntimeWorkerDispatchRetryDispatch(root, filepath.Base(plan.Path), time.Now().UTC()); err != nil {
		t.Fatalf("create worker dispatch retry dispatch: %v", err)
	}
	claimsBefore, err := app.ListRuntimeWorkerQueueClaims(root)
	if err != nil {
		t.Fatalf("list queue claims before maintenance: %v", err)
	}

	var out bytes.Buffer
	cmd := newRootCommand(&out, new(bytes.Buffer))
	cmd.SetArgs([]string{"runtime", "worker-lifecycle-maintenance", root, "--execute", "--execute-retry-dispatches"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute worker lifecycle maintenance with explicit retry-dispatch flag: %v", err)
	}
	output := out.String()
	for _, want := range []string{
		"worker-lifecycle-maintenance-artifact:",
		"retry_dispatch_execute=1",
		"skipped_unsafe=0",
		"retry_dispatch_executions_created=1",
		"retry_dispatch_claims_created=1",
		"worker-lifecycle-maintenance-retry-dispatch-execution:",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected explicit retry-dispatch maintenance output to contain %q, got %q", want, output)
		}
	}
	claimsAfter, err := app.ListRuntimeWorkerQueueClaims(root)
	if err != nil {
		t.Fatalf("list queue claims after maintenance: %v", err)
	}
	if len(claimsAfter) != len(claimsBefore)+1 {
		t.Fatalf("expected one retry queue claim to be created: before=%d after=%d", len(claimsBefore), len(claimsAfter))
	}
	executions, err := app.ListRuntimeWorkerDispatchRetryDispatchExecutions(root)
	if err != nil {
		t.Fatalf("list retry-dispatch executions after maintenance: %v", err)
	}
	if len(executions) != 1 || executions[0].Summary.Claimed != 1 || executions[0].Summary.Dispatched != 0 {
		t.Fatalf("expected one metadata-only retry-dispatch execution with one claim and no dispatch, got %#v", executions)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("maintenance retry-dispatch execution must not execute retry tool-plan command, marker stat err=%v", err)
	}
}

func TestRuntimeWorkerLifecycleMaintenanceCommandDispatchesRetryClaimsWithExplicitHandoff(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	marker := filepath.Join(t.TempDir(), "maintenance-retry-handoff-cli-ran")
	failedDispatch := createCLIFileOutboxWorkerQueueDispatchWithToolPlan(t, root, app.RuntimeWorkerToolPlanPayload{
		Action:     "discover_assets",
		Runner:     "command",
		Parser:     "web_api_parser",
		RunCommand: []string{"sh", "-c", "touch " + marker},
	})
	if _, err := app.SubmitRuntimeWorkerQueueDispatchResult(root, filepath.Base(failedDispatch.Path), app.RuntimeWorkerQueueDispatchResult{
		Status: "failed",
		Error:  "exit status 23",
	}, app.RuntimeWorkerQueueDispatchResultOptions{Now: time.Now().UTC()}); err != nil {
		t.Fatalf("submit failed worker queue dispatch result: %v", err)
	}
	plan, err := app.CreateRuntimeWorkerDispatchRetryPlan(root, time.Now().UTC())
	if err != nil {
		t.Fatalf("create worker dispatch retry plan: %v", err)
	}
	if _, err := app.CreateRuntimeWorkerDispatchRetryDispatch(root, filepath.Base(plan.Path), time.Now().UTC()); err != nil {
		t.Fatalf("create worker dispatch retry dispatch: %v", err)
	}
	requestsBefore, err := app.ListRuntimeWorkerTransportDispatchRequests(root, "")
	if err != nil {
		t.Fatalf("list transport requests before maintenance: %v", err)
	}
	resultsBefore := countCLIJSON(t, filepath.Join(root, "runtime", "worker-queue-dispatch-results"))

	var out bytes.Buffer
	cmd := newRootCommand(&out, new(bytes.Buffer))
	cmd.SetArgs([]string{
		"runtime", "worker-lifecycle-maintenance", root,
		"--execute",
		"--execute-retry-dispatches",
		"--dispatch-lifecycle-retry-claims",
		"--lifecycle-retry-transport", "file-outbox",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute worker lifecycle maintenance with retry handoff: %v", err)
	}
	output := out.String()
	for _, want := range []string{
		"worker-lifecycle-maintenance-artifact:",
		"retry_dispatch_claims_created=1",
		"retry_dispatch_claims_dispatched=1",
		"retry_dispatch_transport_requests_created=1",
		"dispatch_lifecycle_retry_claims=true",
		"lifecycle_retry_transport=file-outbox",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected retry handoff maintenance output to contain %q, got %q", want, output)
		}
	}
	executions, err := app.ListRuntimeWorkerDispatchRetryDispatchExecutions(root)
	if err != nil {
		t.Fatalf("list retry-dispatch executions after maintenance: %v", err)
	}
	if len(executions) != 1 || executions[0].Summary.Claimed != 1 || executions[0].Summary.Dispatched != 1 {
		t.Fatalf("expected one retry-dispatch execution with one dispatched claim, got %#v", executions)
	}
	requestsAfter, err := app.ListRuntimeWorkerTransportDispatchRequests(root, "")
	if err != nil {
		t.Fatalf("list transport requests after maintenance: %v", err)
	}
	if len(requestsAfter) != len(requestsBefore)+1 {
		t.Fatalf("expected one pending retry file-outbox request: before=%d after=%d", len(requestsBefore), len(requestsAfter))
	}
	if got := countCLIJSON(t, filepath.Join(root, "runtime", "worker-queue-dispatch-results")); got != resultsBefore {
		t.Fatalf("retry handoff maintenance must not create worker results: before=%d after=%d", resultsBefore, got)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("retry handoff maintenance must not execute retry tool-plan command, marker stat err=%v", err)
	}
}

func TestRuntimeWorkerLifecycleMaintenanceCommandRejectsRetryHandoffWithoutRequiredGates(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "without execute",
			args: []string{"--execute-retry-dispatches", "--dispatch-lifecycle-retry-claims"},
			want: "--dispatch-lifecycle-retry-claims requires --execute",
		},
		{
			name: "without retry execution",
			args: []string{"--execute", "--dispatch-lifecycle-retry-claims"},
			want: "--dispatch-lifecycle-retry-claims requires --execute-retry-dispatches",
		},
		{
			name: "http-json transport",
			args: []string{"--execute", "--execute-retry-dispatches", "--dispatch-lifecycle-retry-claims", "--lifecycle-retry-transport", "http-json"},
			want: "--lifecycle-retry-transport supports metadata-only or file-outbox",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "workspace")
			if err := app.InitWorkspace(root); err != nil {
				t.Fatalf("init workspace: %v", err)
			}
			var out bytes.Buffer
			var stderr bytes.Buffer
			cmd := newRootCommand(&out, &stderr)
			args := append([]string{"runtime", "worker-lifecycle-maintenance", root}, tc.args...)
			cmd.SetArgs(args)
			err := cmd.Execute()
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected retry handoff gate error %q, got %v stdout=%q stderr=%q", tc.want, err, out.String(), stderr.String())
			}
			if maintenances, listErr := app.ListWorkerLifecycleMaintenances(root); listErr != nil {
				t.Fatalf("list worker lifecycle maintenances: %v", listErr)
			} else if len(maintenances) != 0 {
				t.Fatalf("invalid retry handoff must not write maintenance artifacts, got %#v", maintenances)
			}
		})
	}
}

func TestRuntimeWorkerLifecycleMaintenanceCommandRejectsRetryDispatchFlagWithoutExecute(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := app.InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}

	var out bytes.Buffer
	var stderr bytes.Buffer
	cmd := newRootCommand(&out, &stderr)
	cmd.SetArgs([]string{"runtime", "worker-lifecycle-maintenance", root, "--execute-retry-dispatches"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "worker lifecycle maintenance retry-dispatch execution requires explicit execute") {
		t.Fatalf("expected retry-dispatch execute gate error, got %v stdout=%q stderr=%q", err, out.String(), stderr.String())
	}
	if maintenances, listErr := app.ListWorkerLifecycleMaintenances(root); listErr != nil {
		t.Fatalf("list worker lifecycle maintenances: %v", listErr)
	} else if len(maintenances) != 0 {
		t.Fatalf("retry-dispatch flag without execute must not write maintenance artifacts, got %#v", maintenances)
	}
}

func TestRuntimeWorkerLifecycleMaintenanceCommandExecutesLeaseRecoveryWithExplicitFlag(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Now().UTC()
	writeCLIWorkerLeaseFixture(t, root, app.WorkerLeaseArtifact{
		Version:     1,
		RunID:       "run-maintenance-lease-recovery-stale",
		ExecutionID: "tool-maintenance-lease-recovery-stale",
		Tool:        "web-api-pack",
		Capability:  "web.discovery",
		WorkerID:    "local",
		Status:      "active",
		AcquiredAt:  now.Add(-30 * time.Minute).Format(time.RFC3339Nano),
		HeartbeatAt: now.Add(-30 * time.Minute).Format(time.RFC3339Nano),
	})
	dispatch := createCLIFileOutboxWorkerQueueDispatch(t, root)
	if _, err := app.SubmitRuntimeWorkerQueueDispatchResult(root, filepath.Base(dispatch.Path), app.RuntimeWorkerQueueDispatchResult{
		Status: "completed",
		Output: "lease recovery result",
	}, app.RuntimeWorkerQueueDispatchResultOptions{Now: now.Add(2 * time.Minute)}); err != nil {
		t.Fatalf("submit worker queue dispatch result: %v", err)
	}
	writeCLIWorkerLeaseFixture(t, root, app.WorkerLeaseArtifact{
		Version:     1,
		RunID:       dispatch.RunID,
		ExecutionID: dispatch.ExecutionID,
		Tool:        dispatch.Tool,
		Capability:  dispatch.Capability,
		WorkerID:    dispatch.WorkerID,
		Status:      "active",
		AcquiredAt:  now.Format(time.RFC3339Nano),
		HeartbeatAt: now.Format(time.RFC3339Nano),
	})
	requestsBefore, err := app.ListRuntimeWorkerTransportDispatchRequests(root, "")
	if err != nil {
		t.Fatalf("list transport requests before maintenance: %v", err)
	}

	var out bytes.Buffer
	cmd := newRootCommand(&out, new(bytes.Buffer))
	cmd.SetArgs([]string{"runtime", "worker-lifecycle-maintenance", root, "--execute", "--execute-lease-recovery"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute worker lifecycle maintenance with lease recovery: %v", err)
	}
	output := out.String()
	for _, want := range []string{
		"worker-lifecycle-maintenance-artifact:",
		"reclaim_executions_created=1",
		"reclaim_results_created=1",
		"reconciliation_executions_created=1",
		"reconciliation_results_created=1",
		"worker-lifecycle-maintenance-reclaim-execution:",
		"worker-lifecycle-maintenance-reconciliation-execution:",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected explicit lease recovery maintenance output to contain %q, got %q", want, output)
		}
	}
	requestsAfter, err := app.ListRuntimeWorkerTransportDispatchRequests(root, "")
	if err != nil {
		t.Fatalf("list transport requests after maintenance: %v", err)
	}
	if len(requestsAfter) != len(requestsBefore) {
		t.Fatalf("lease recovery maintenance must not create transport requests: before=%d after=%d", len(requestsBefore), len(requestsAfter))
	}
}

func TestRuntimeWorkerLifecycleMaintenanceCommandRejectsLeaseRecoveryFlagWithoutExecute(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := app.InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}

	var out bytes.Buffer
	var stderr bytes.Buffer
	cmd := newRootCommand(&out, &stderr)
	cmd.SetArgs([]string{"runtime", "worker-lifecycle-maintenance", root, "--execute-lease-recovery"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "worker lifecycle maintenance lease recovery execution requires explicit execute") {
		t.Fatalf("expected lease recovery execute gate error, got %v stdout=%q stderr=%q", err, out.String(), stderr.String())
	}
	if maintenances, listErr := app.ListWorkerLifecycleMaintenances(root); listErr != nil {
		t.Fatalf("list worker lifecycle maintenances: %v", listErr)
	} else if len(maintenances) != 0 {
		t.Fatalf("lease recovery flag without execute must not write maintenance artifacts, got %#v", maintenances)
	}
}

func TestRuntimeWorkerLifecycleMaintenanceSupervisorCommandRunsBoundedMetadataOnlyMaintenanceAndAudit(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Now().UTC()
	writeCLIWorkerLeaseFixture(t, root, app.WorkerLeaseArtifact{
		Version:     1,
		RunID:       "run-maintenance-supervisor-cli",
		ExecutionID: "tool-maintenance-supervisor-cli",
		Tool:        "web-api-pack",
		Capability:  "web.discovery",
		WorkerID:    "local",
		Status:      "active",
		AcquiredAt:  now.Add(-30 * time.Minute).Format(time.RFC3339Nano),
		HeartbeatAt: now.Add(-30 * time.Minute).Format(time.RFC3339Nano),
	})
	createCLIFileOutboxWorkerQueueDispatch(t, root)

	var out bytes.Buffer
	cmd := newRootCommand(&out, new(bytes.Buffer))
	cmd.SetArgs([]string{"runtime", "worker-lifecycle-maintenance-supervisor", root, "--execute", "--max-runs", "2"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute worker lifecycle maintenance supervisor: %v", err)
	}
	output := out.String()
	for _, want := range []string{
		"worker-lifecycle-maintenance-supervisor-artifact:",
		"status=completed",
		"mode=metadata-only",
		"runs=2",
		"max_runs=2",
		"maintenance_created=2",
		"audit_created=2",
		"worker-lifecycle-maintenance-supervisor-maintenance:",
		"worker-lifecycle-maintenance-supervisor-audit:",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected worker lifecycle maintenance supervisor output to contain %q, got %q", want, output)
		}
	}
	supervisors, err := app.ListWorkerLifecycleMaintenanceSupervisors(root)
	if err != nil {
		t.Fatalf("list worker lifecycle maintenance supervisors: %v", err)
	}
	if len(supervisors) != 1 || supervisors[0].Summary.Runs != 2 {
		t.Fatalf("expected one completed supervisor run, got %#v", supervisors)
	}
	maintenances, err := app.ListWorkerLifecycleMaintenances(root)
	if err != nil {
		t.Fatalf("list worker lifecycle maintenances: %v", err)
	}
	audits, err := app.ListWorkerLifecycleAudits(root)
	if err != nil {
		t.Fatalf("list worker lifecycle audits: %v", err)
	}
	if len(maintenances) != 2 || len(audits) != 2 {
		t.Fatalf("expected two maintenance and audit artifacts, got maintenances=%d audits=%d", len(maintenances), len(audits))
	}

	var listOut bytes.Buffer
	listCmd := newRootCommand(&listOut, new(bytes.Buffer))
	listCmd.SetArgs([]string{"runtime", "worker-lifecycle-maintenance-supervisor-runs", root})
	if err := listCmd.Execute(); err != nil {
		t.Fatalf("list worker lifecycle maintenance supervisor runs: %v", err)
	}
	if !strings.Contains(listOut.String(), "worker-lifecycle-maintenance-supervisor-runs: total=1") || !strings.Contains(listOut.String(), "worker-lifecycle-maintenance-supervisor-artifact: "+supervisors[0].Path) {
		t.Fatalf("expected worker lifecycle maintenance supervisor listing, got %q", listOut.String())
	}

	var inspectOut bytes.Buffer
	inspectCmd := newRootCommand(&inspectOut, new(bytes.Buffer))
	inspectCmd.SetArgs([]string{"runtime", "worker-lifecycle-maintenance-supervisor-runs", root, filepath.Base(supervisors[0].Path)})
	if err := inspectCmd.Execute(); err != nil {
		t.Fatalf("inspect worker lifecycle maintenance supervisor: %v", err)
	}
	if !strings.Contains(inspectOut.String(), "worker-lifecycle-maintenance-supervisor-artifact: "+supervisors[0].Path) || !strings.Contains(inspectOut.String(), "audit_created=2") {
		t.Fatalf("expected inspected worker lifecycle maintenance supervisor, got %q", inspectOut.String())
	}
}

func TestRuntimeWorkerLifecycleMaintenanceSupervisorCommandRejectsInvalidBounds(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")

	var out bytes.Buffer
	var stderr bytes.Buffer
	cmd := newRootCommand(&out, &stderr)
	cmd.SetArgs([]string{"runtime", "worker-lifecycle-maintenance-supervisor", root, "--max-runs", "0"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "--max-runs must be positive") {
		t.Fatalf("expected max-runs validation error, got %v stdout=%q stderr=%q", err, out.String(), stderr.String())
	}
	supervisors, listErr := app.ListWorkerLifecycleMaintenanceSupervisors(root)
	if listErr != nil {
		t.Fatalf("list worker lifecycle maintenance supervisors: %v", listErr)
	}
	if len(supervisors) != 0 {
		t.Fatalf("invalid supervisor bounds must not write artifacts, got %#v", supervisors)
	}
}

func TestRuntimeWorkerLifecycleMaintenanceSupervisorCommandAggregatesRetryDispatchSkips(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	marker := filepath.Join(t.TempDir(), "maintenance-supervisor-retry-dispatch-cli-ran")
	failedDispatch := createCLIFileOutboxWorkerQueueDispatchWithToolPlan(t, root, app.RuntimeWorkerToolPlanPayload{
		Action:     "discover_assets",
		Runner:     "command",
		Parser:     "web_api_parser",
		RunCommand: []string{"sh", "-c", "touch " + marker},
	})
	if _, err := app.SubmitRuntimeWorkerQueueDispatchResult(root, filepath.Base(failedDispatch.Path), app.RuntimeWorkerQueueDispatchResult{
		Status: "failed",
		Error:  "exit status 23",
	}, app.RuntimeWorkerQueueDispatchResultOptions{Now: time.Now().UTC()}); err != nil {
		t.Fatalf("submit failed worker queue dispatch result: %v", err)
	}
	plan, err := app.CreateRuntimeWorkerDispatchRetryPlan(root, time.Now().UTC())
	if err != nil {
		t.Fatalf("create worker dispatch retry plan: %v", err)
	}
	if _, err := app.CreateRuntimeWorkerDispatchRetryDispatch(root, filepath.Base(plan.Path), time.Now().UTC()); err != nil {
		t.Fatalf("create worker dispatch retry dispatch: %v", err)
	}
	claimsBefore, err := app.ListRuntimeWorkerQueueClaims(root)
	if err != nil {
		t.Fatalf("list queue claims before supervisor: %v", err)
	}

	var out bytes.Buffer
	cmd := newRootCommand(&out, new(bytes.Buffer))
	cmd.SetArgs([]string{"runtime", "worker-lifecycle-maintenance-supervisor", root, "--execute", "--max-runs", "2"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute worker lifecycle maintenance supervisor: %v", err)
	}
	output := out.String()
	for _, want := range []string{
		"worker-lifecycle-maintenance-supervisor-artifact:",
		"runs=2",
		"actions=2",
		"retry_dispatch_execute=2",
		"skipped_unsafe=2",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected retry-dispatch supervisor output to contain %q, got %q", want, output)
		}
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("supervisor must not execute retry tool-plan command, marker stat err=%v", err)
	}
	claimsAfter, err := app.ListRuntimeWorkerQueueClaims(root)
	if err != nil {
		t.Fatalf("list queue claims after supervisor: %v", err)
	}
	if len(claimsAfter) != len(claimsBefore) {
		t.Fatalf("supervisor must not create retry queue claims: before=%d after=%d", len(claimsBefore), len(claimsAfter))
	}
	if executions, err := app.ListRuntimeWorkerDispatchRetryDispatchExecutions(root); err != nil {
		t.Fatalf("list retry-dispatch executions after supervisor: %v", err)
	} else if len(executions) != 0 {
		t.Fatalf("supervisor must not execute retry dispatch artifacts, got %#v", executions)
	}
}

func TestRuntimeWorkerLifecycleMaintenanceSupervisorCommandExecutesRetryDispatchesWithExplicitFlag(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	marker := filepath.Join(t.TempDir(), "maintenance-supervisor-retry-dispatch-cli-explicit-ran")
	failedDispatch := createCLIFileOutboxWorkerQueueDispatchWithToolPlan(t, root, app.RuntimeWorkerToolPlanPayload{
		Action:     "discover_assets",
		Runner:     "command",
		Parser:     "web_api_parser",
		RunCommand: []string{"sh", "-c", "touch " + marker},
	})
	if _, err := app.SubmitRuntimeWorkerQueueDispatchResult(root, filepath.Base(failedDispatch.Path), app.RuntimeWorkerQueueDispatchResult{
		Status: "failed",
		Error:  "exit status 23",
	}, app.RuntimeWorkerQueueDispatchResultOptions{Now: time.Now().UTC()}); err != nil {
		t.Fatalf("submit failed worker queue dispatch result: %v", err)
	}
	plan, err := app.CreateRuntimeWorkerDispatchRetryPlan(root, time.Now().UTC())
	if err != nil {
		t.Fatalf("create worker dispatch retry plan: %v", err)
	}
	if _, err := app.CreateRuntimeWorkerDispatchRetryDispatch(root, filepath.Base(plan.Path), time.Now().UTC()); err != nil {
		t.Fatalf("create worker dispatch retry dispatch: %v", err)
	}
	claimsBefore, err := app.ListRuntimeWorkerQueueClaims(root)
	if err != nil {
		t.Fatalf("list queue claims before supervisor: %v", err)
	}
	requestsBefore, err := app.ListRuntimeWorkerTransportDispatchRequests(root, "")
	if err != nil {
		t.Fatalf("list transport requests before supervisor: %v", err)
	}

	var out bytes.Buffer
	cmd := newRootCommand(&out, new(bytes.Buffer))
	cmd.SetArgs([]string{"runtime", "worker-lifecycle-maintenance-supervisor", root, "--execute", "--max-runs", "2", "--execute-retry-dispatches"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute worker lifecycle maintenance supervisor retry-dispatches: %v", err)
	}
	output := out.String()
	for _, want := range []string{
		"worker-lifecycle-maintenance-supervisor-artifact:",
		"runs=2",
		"retry_dispatch_execute=1",
		"skipped_unsafe=0",
		"retry_dispatch_executions_created=1",
		"retry_dispatch_claims_created=1",
		"worker-lifecycle-maintenance-supervisor-retry-dispatch-execution:",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected retry-dispatch supervisor output to contain %q, got %q", want, output)
		}
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("supervisor must not execute retry tool-plan command, marker stat err=%v", err)
	}
	claimsAfter, err := app.ListRuntimeWorkerQueueClaims(root)
	if err != nil {
		t.Fatalf("list queue claims after supervisor: %v", err)
	}
	if len(claimsAfter) != len(claimsBefore)+1 {
		t.Fatalf("expected one retry queue claim: before=%d after=%d", len(claimsBefore), len(claimsAfter))
	}
	requestsAfter, err := app.ListRuntimeWorkerTransportDispatchRequests(root, "")
	if err != nil {
		t.Fatalf("list transport requests after supervisor: %v", err)
	}
	if len(requestsAfter) != len(requestsBefore) {
		t.Fatalf("supervisor retry-dispatch execution must not create file-outbox requests: before=%d after=%d", len(requestsBefore), len(requestsAfter))
	}
	executions, err := app.ListRuntimeWorkerDispatchRetryDispatchExecutions(root)
	if err != nil {
		t.Fatalf("list retry-dispatch executions after supervisor: %v", err)
	}
	if len(executions) != 1 || executions[0].Summary.Claimed != 1 || executions[0].Summary.Dispatched != 0 {
		t.Fatalf("expected one metadata-only retry-dispatch execution, got %#v", executions)
	}
}

func TestRuntimeWorkerLifecycleMaintenanceSupervisorCommandRejectsRetryDispatchFlagWithoutExecute(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := app.InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}

	var out bytes.Buffer
	var stderr bytes.Buffer
	cmd := newRootCommand(&out, &stderr)
	cmd.SetArgs([]string{"runtime", "worker-lifecycle-maintenance-supervisor", root, "--max-runs", "1", "--execute-retry-dispatches"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "worker lifecycle maintenance supervisor retry-dispatch execution requires explicit execute") {
		t.Fatalf("expected supervisor retry-dispatch execute gate error, got %v stdout=%q stderr=%q", err, out.String(), stderr.String())
	}
	if supervisors, listErr := app.ListWorkerLifecycleMaintenanceSupervisors(root); listErr != nil {
		t.Fatalf("list worker lifecycle maintenance supervisors: %v", listErr)
	} else if len(supervisors) != 0 {
		t.Fatalf("retry-dispatch flag without execute must not write supervisor artifacts, got %#v", supervisors)
	}
}

func TestRuntimeWorkerLifecycleMaintenanceSupervisorCommandRejectsLeaseRecoveryFlagWithoutExecute(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := app.InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}

	var out bytes.Buffer
	var stderr bytes.Buffer
	cmd := newRootCommand(&out, &stderr)
	cmd.SetArgs([]string{"runtime", "worker-lifecycle-maintenance-supervisor", root, "--max-runs", "1", "--execute-lease-recovery"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "worker lifecycle maintenance supervisor lease recovery execution requires explicit execute") {
		t.Fatalf("expected supervisor lease recovery execute gate error, got %v stdout=%q stderr=%q", err, out.String(), stderr.String())
	}
	if supervisors, listErr := app.ListWorkerLifecycleMaintenanceSupervisors(root); listErr != nil {
		t.Fatalf("list worker lifecycle maintenance supervisors: %v", listErr)
	} else if len(supervisors) != 0 {
		t.Fatalf("lease recovery flag without execute must not write supervisor artifacts, got %#v", supervisors)
	}
}

func TestRuntimeWorkerLifecycleAuditCommandShowsMaintenanceSupervisorAggregateFields(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	marker := filepath.Join(t.TempDir(), "audit-supervisor-retry-dispatch-cli-ran")
	failedDispatch := createCLIFileOutboxWorkerQueueDispatchWithToolPlan(t, root, app.RuntimeWorkerToolPlanPayload{
		Action:     "discover_assets",
		Runner:     "command",
		Parser:     "web_api_parser",
		RunCommand: []string{"sh", "-c", "touch " + marker},
	})
	now := time.Now().UTC()
	if _, err := app.SubmitRuntimeWorkerQueueDispatchResult(root, filepath.Base(failedDispatch.Path), app.RuntimeWorkerQueueDispatchResult{
		Status: "failed",
		Error:  "exit status 23",
	}, app.RuntimeWorkerQueueDispatchResultOptions{Now: now}); err != nil {
		t.Fatalf("submit failed worker queue dispatch result: %v", err)
	}
	plan, err := app.CreateRuntimeWorkerDispatchRetryPlan(root, now.Add(time.Minute))
	if err != nil {
		t.Fatalf("create worker dispatch retry plan: %v", err)
	}
	if _, err := app.CreateRuntimeWorkerDispatchRetryDispatch(root, filepath.Base(plan.Path), now.Add(2*time.Minute)); err != nil {
		t.Fatalf("create worker dispatch retry dispatch: %v", err)
	}
	if _, err := app.CreateWorkerLifecycleMaintenanceSupervisor(root, app.WorkerLifecycleMaintenanceSupervisorOptions{
		Execute:    true,
		Now:        now.Add(3 * time.Minute),
		StaleAfter: app.DefaultWorkerLeaseStaleAfter,
		MaxRuns:    2,
	}); err != nil {
		t.Fatalf("create worker lifecycle maintenance supervisor fixture: %v", err)
	}
	claimsBefore, err := app.ListRuntimeWorkerQueueClaims(root)
	if err != nil {
		t.Fatalf("list queue claims before audit: %v", err)
	}

	var out bytes.Buffer
	cmd := newRootCommand(&out, new(bytes.Buffer))
	cmd.SetArgs([]string{"runtime", "worker-lifecycle-audit", root})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute worker lifecycle audit: %v", err)
	}
	output := out.String()
	for _, want := range []string{
		"worker-lifecycle-audit-item: kind=worker-lifecycle-maintenance-supervisor",
		"actions=2",
		"skipped_unsafe=2",
		"retry_dispatch_execute=2",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected lifecycle audit output to contain %q, got %q", want, output)
		}
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("audit must not execute retry tool-plan command, marker stat err=%v", err)
	}
	claimsAfter, err := app.ListRuntimeWorkerQueueClaims(root)
	if err != nil {
		t.Fatalf("list queue claims after audit: %v", err)
	}
	if len(claimsAfter) != len(claimsBefore) {
		t.Fatalf("audit must not create retry queue claims: before=%d after=%d", len(claimsBefore), len(claimsAfter))
	}
	if executions, err := app.ListRuntimeWorkerDispatchRetryDispatchExecutions(root); err != nil {
		t.Fatalf("list retry-dispatch executions after audit: %v", err)
	} else if len(executions) != 0 {
		t.Fatalf("audit must not execute retry dispatch artifacts, got %#v", executions)
	}
}

func TestRuntimeWorkerLifecycleAuditCommandShowsMaintenanceAggregateFields(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	marker := filepath.Join(t.TempDir(), "audit-maintenance-retry-dispatch-cli-ran")
	failedDispatch := createCLIFileOutboxWorkerQueueDispatchWithToolPlan(t, root, app.RuntimeWorkerToolPlanPayload{
		Action:     "discover_assets",
		Runner:     "command",
		Parser:     "web_api_parser",
		RunCommand: []string{"sh", "-c", "touch " + marker},
	})
	now := time.Now().UTC()
	if _, err := app.SubmitRuntimeWorkerQueueDispatchResult(root, filepath.Base(failedDispatch.Path), app.RuntimeWorkerQueueDispatchResult{
		Status: "failed",
		Error:  "exit status 23",
	}, app.RuntimeWorkerQueueDispatchResultOptions{Now: now}); err != nil {
		t.Fatalf("submit failed worker queue dispatch result: %v", err)
	}
	plan, err := app.CreateRuntimeWorkerDispatchRetryPlan(root, now.Add(time.Minute))
	if err != nil {
		t.Fatalf("create worker dispatch retry plan: %v", err)
	}
	if _, err := app.CreateRuntimeWorkerDispatchRetryDispatch(root, filepath.Base(plan.Path), now.Add(2*time.Minute)); err != nil {
		t.Fatalf("create worker dispatch retry dispatch: %v", err)
	}
	if _, err := app.CreateWorkerLifecycleMaintenance(root, app.WorkerLifecycleMaintenanceOptions{
		Execute:    true,
		Now:        now.Add(3 * time.Minute),
		StaleAfter: app.DefaultWorkerLeaseStaleAfter,
	}); err != nil {
		t.Fatalf("create worker lifecycle maintenance fixture: %v", err)
	}
	claimsBefore, err := app.ListRuntimeWorkerQueueClaims(root)
	if err != nil {
		t.Fatalf("list queue claims before audit: %v", err)
	}

	var out bytes.Buffer
	cmd := newRootCommand(&out, new(bytes.Buffer))
	cmd.SetArgs([]string{"runtime", "worker-lifecycle-audit", root})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute worker lifecycle audit: %v", err)
	}
	output := out.String()
	for _, want := range []string{
		"worker-lifecycle-audit-item: kind=worker-lifecycle-maintenance",
		"actions=1",
		"skipped_unsafe=1",
		"retry_dispatch_execute=1",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected lifecycle audit output to contain %q, got %q", want, output)
		}
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("audit must not execute retry tool-plan command, marker stat err=%v", err)
	}
	claimsAfter, err := app.ListRuntimeWorkerQueueClaims(root)
	if err != nil {
		t.Fatalf("list queue claims after audit: %v", err)
	}
	if len(claimsAfter) != len(claimsBefore) {
		t.Fatalf("audit must not create retry queue claims: before=%d after=%d", len(claimsBefore), len(claimsAfter))
	}
	if executions, err := app.ListRuntimeWorkerDispatchRetryDispatchExecutions(root); err != nil {
		t.Fatalf("list retry-dispatch executions after audit: %v", err)
	} else if len(executions) != 0 {
		t.Fatalf("audit must not execute retry dispatch artifacts, got %#v", executions)
	}
}

func TestRuntimeWorkerLifecycleAuditCommandShowsActionPlanAggregateFields(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	marker := filepath.Join(t.TempDir(), "audit-action-plan-retry-dispatch-cli-ran")
	failedDispatch := createCLIFileOutboxWorkerQueueDispatchWithToolPlan(t, root, app.RuntimeWorkerToolPlanPayload{
		Action:     "discover_assets",
		Runner:     "command",
		Parser:     "web_api_parser",
		RunCommand: []string{"sh", "-c", "touch " + marker},
	})
	now := time.Now().UTC()
	if _, err := app.SubmitRuntimeWorkerQueueDispatchResult(root, filepath.Base(failedDispatch.Path), app.RuntimeWorkerQueueDispatchResult{
		Status: "failed",
		Error:  "exit status 23",
	}, app.RuntimeWorkerQueueDispatchResultOptions{Now: now}); err != nil {
		t.Fatalf("submit failed worker queue dispatch result: %v", err)
	}
	plan, err := app.CreateRuntimeWorkerDispatchRetryPlan(root, now.Add(time.Minute))
	if err != nil {
		t.Fatalf("create worker dispatch retry plan: %v", err)
	}
	if _, err := app.CreateRuntimeWorkerDispatchRetryDispatch(root, filepath.Base(plan.Path), now.Add(2*time.Minute)); err != nil {
		t.Fatalf("create worker dispatch retry dispatch: %v", err)
	}
	actionPlan, err := app.CreateWorkerLifecycleActionPlan(root, "", now.Add(3*time.Minute), app.DefaultWorkerLeaseStaleAfter)
	if err != nil {
		t.Fatalf("create worker lifecycle action plan fixture: %v", err)
	}
	if actionPlan.Summary.Total == 0 || actionPlan.Summary.ExecuteDispatchRetryDispatch != 1 {
		t.Fatalf("expected retry-dispatch action-plan fixture, got %#v", actionPlan.Summary)
	}
	reviewedAt := now.Add(4 * time.Minute)
	if _, err := app.AnnotateWorkerLifecycleActionPlan(root, filepath.Base(actionPlan.Path), app.WorkerLifecycleActionPlanAnnotationOptions{
		Decision:   "accepted",
		Reviewer:   "cli-auditor",
		ReviewedAt: reviewedAt,
		Reason:     "authorized_retry_dispatch_review",
	}); err != nil {
		t.Fatalf("annotate worker lifecycle action plan fixture: %v", err)
	}
	selectedAt := now.Add(5 * time.Minute)
	selection, err := app.SelectWorkerLifecycleActionPlan(root, filepath.Base(actionPlan.Path), app.WorkerLifecycleActionPlanSelectionOptions{
		SelectedBy: "cli-operator",
		SelectedAt: selectedAt,
		Reason:     "current_reviewed_action_plan",
	})
	if err != nil {
		t.Fatalf("select worker lifecycle action plan fixture: %v", err)
	}
	claimsBefore, err := app.ListRuntimeWorkerQueueClaims(root)
	if err != nil {
		t.Fatalf("list queue claims before audit: %v", err)
	}

	var out bytes.Buffer
	cmd := newRootCommand(&out, new(bytes.Buffer))
	cmd.SetArgs([]string{"runtime", "worker-lifecycle-audit", root})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute worker lifecycle audit: %v", err)
	}
	output := out.String()
	for _, want := range []string{
		"action_plan_annotations=1",
		"action_plan_selections=1",
		"worker-lifecycle-audit-item: kind=worker-lifecycle-action-plan",
		"decision=accepted",
		"reviewer=cli-auditor",
		"reviewed_at=" + reviewedAt.Format(time.RFC3339),
		"reason=authorized_retry_dispatch_review",
		"worker-lifecycle-audit-item: kind=worker-lifecycle-action-plan-selection",
		"path=" + selection.Path,
		"linked=" + actionPlan.Path,
		"selected_by=cli-operator",
		"selected_at=" + selectedAt.Format(time.RFC3339),
		fmt.Sprintf("actions=%d", actionPlan.Summary.Total),
		"retry_dispatch_execute=1",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected lifecycle audit output to contain %q, got %q", want, output)
		}
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("audit must not execute retry tool-plan command, marker stat err=%v", err)
	}
	claimsAfter, err := app.ListRuntimeWorkerQueueClaims(root)
	if err != nil {
		t.Fatalf("list queue claims after audit: %v", err)
	}
	if len(claimsAfter) != len(claimsBefore) {
		t.Fatalf("audit must not create retry queue claims: before=%d after=%d", len(claimsBefore), len(claimsAfter))
	}
	if executions, err := app.ListRuntimeWorkerDispatchRetryDispatchExecutions(root); err != nil {
		t.Fatalf("list retry-dispatch executions after audit: %v", err)
	} else if len(executions) != 0 {
		t.Fatalf("audit must not execute retry dispatch artifacts, got %#v", executions)
	}
}

func TestRuntimeWorkerLifecycleAuditCommandShowsWorkerOrchestrationSupervisorAggregateFields(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	registerCLIWorkerOrchestrationWorkers(t, root)
	now := time.Now().UTC()
	supervisor, err := app.CreateRuntimeWorkerOrchestrationSupervisor(root, app.RuntimeWorkerOrchestrationSupervisorOptions{
		Execute:                     true,
		Now:                         now,
		RunIDPrefix:                 "run-worker-orchestration-supervisor-audit-cli",
		TransportMode:               "metadata-only",
		MaxCandidates:               1,
		MaxRuns:                     2,
		LeaseDuration:               10 * time.Minute,
		IncludeLifecycleMaintenance: true,
	})
	if err != nil {
		t.Fatalf("create worker orchestration supervisor fixture: %v", err)
	}
	orchestrationsBefore, err := app.ListRuntimeWorkerOrchestrations(root)
	if err != nil {
		t.Fatalf("list worker orchestrations before audit: %v", err)
	}

	var out bytes.Buffer
	cmd := newRootCommand(&out, new(bytes.Buffer))
	cmd.SetArgs([]string{"runtime", "worker-lifecycle-audit", root})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute worker lifecycle audit: %v", err)
	}
	output := out.String()
	for _, want := range []string{
		"worker-lifecycle-audit-item: kind=worker-orchestration-supervisor",
		fmt.Sprintf("runs=%d", supervisor.Summary.Runs),
		fmt.Sprintf("max_runs=%d", supervisor.Summary.MaxRuns),
		fmt.Sprintf("orchestrations_created=%d", supervisor.Summary.OrchestrationArtifactsCreated),
		fmt.Sprintf("lifecycle_maintenance=%d", supervisor.Summary.LifecycleMaintenanceArtifactsCreated),
		fmt.Sprintf("lifecycle_audit=%d", supervisor.Summary.LifecycleAuditArtifactsCreated),
		fmt.Sprintf("claimed=%d", supervisor.Summary.ClaimedCount),
		fmt.Sprintf("dispatched=%d", supervisor.Summary.DispatchedCount),
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected lifecycle audit output to contain %q, got %q", want, output)
		}
	}
	orchestrationsAfter, err := app.ListRuntimeWorkerOrchestrations(root)
	if err != nil {
		t.Fatalf("list worker orchestrations after audit: %v", err)
	}
	if len(orchestrationsAfter) != len(orchestrationsBefore) {
		t.Fatalf("audit must not create worker orchestrations: before=%d after=%d", len(orchestrationsBefore), len(orchestrationsAfter))
	}
	if outbox, err := app.ListRuntimeWorkerTransportDispatchRequests(root, ""); err != nil {
		t.Fatalf("list worker outbox requests after audit: %v", err)
	} else if len(outbox) != 0 {
		t.Fatalf("metadata-only audit must not create worker outbox requests, got %#v", outbox)
	}
}

func TestRuntimeWorkerLifecycleAuditCommandShowsWorkerOrchestrationAggregateFields(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	registerCLIWorkerOrchestrationWorkers(t, root)
	now := time.Now().UTC()
	orchestration, err := app.CreateRuntimeWorkerOrchestration(root, app.RuntimeWorkerOrchestrationOptions{
		Execute:                     true,
		Now:                         now,
		RunID:                       "run-worker-orchestration-audit-cli",
		TransportMode:               "metadata-only",
		MaxCandidates:               1,
		LeaseDuration:               10 * time.Minute,
		IncludeLifecycleMaintenance: true,
	})
	if err != nil {
		t.Fatalf("create worker orchestration fixture: %v", err)
	}
	orchestrationsBefore, err := app.ListRuntimeWorkerOrchestrations(root)
	if err != nil {
		t.Fatalf("list worker orchestrations before audit: %v", err)
	}

	var out bytes.Buffer
	cmd := newRootCommand(&out, new(bytes.Buffer))
	cmd.SetArgs([]string{"runtime", "worker-lifecycle-audit", root})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute worker lifecycle audit: %v", err)
	}
	output := out.String()
	for _, want := range []string{
		"worker-lifecycle-audit-item: kind=worker-orchestration",
		fmt.Sprintf("placement_ready=%d", orchestration.PlacementReady),
		fmt.Sprintf("claimed=%d", orchestration.ClaimedCount),
		fmt.Sprintf("dispatch_ready=%d", orchestration.DispatchReady),
		fmt.Sprintf("dispatched=%d", orchestration.DispatchedCount),
		"lifecycle_maintenance=1",
		"lifecycle_audit=1",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected lifecycle audit output to contain %q, got %q", want, output)
		}
	}
	if orchestration.PlacementSkipped > 0 && !strings.Contains(output, fmt.Sprintf("placement_skipped=%d", orchestration.PlacementSkipped)) {
		t.Fatalf("expected lifecycle audit output to contain placement_skipped=%d, got %q", orchestration.PlacementSkipped, output)
	}
	if orchestration.DispatchSkipped > 0 && !strings.Contains(output, fmt.Sprintf("dispatch_skipped=%d", orchestration.DispatchSkipped)) {
		t.Fatalf("expected lifecycle audit output to contain dispatch_skipped=%d, got %q", orchestration.DispatchSkipped, output)
	}
	orchestrationsAfter, err := app.ListRuntimeWorkerOrchestrations(root)
	if err != nil {
		t.Fatalf("list worker orchestrations after audit: %v", err)
	}
	if len(orchestrationsAfter) != len(orchestrationsBefore) {
		t.Fatalf("audit must not create worker orchestrations: before=%d after=%d", len(orchestrationsBefore), len(orchestrationsAfter))
	}
	if outbox, err := app.ListRuntimeWorkerTransportDispatchRequests(root, ""); err != nil {
		t.Fatalf("list worker outbox requests after audit: %v", err)
	} else if len(outbox) != 0 {
		t.Fatalf("metadata-only audit must not create worker outbox requests, got %#v", outbox)
	}
}

func TestRuntimeWorkerLifecycleAuditCommandShowsLeaseRecoveryAggregateFields(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Date(2026, 5, 31, 17, 45, 0, 0, time.UTC)
	staleAfter := 5 * time.Minute
	if err := app.InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	if _, err := app.RegisterRuntimeWorker(root, app.RuntimeWorkerArtifact{
		WorkerID:     "remote-web-1",
		Mode:         "remote",
		Status:       "available",
		Endpoint:     "file://remote-web-1/inbox",
		Capabilities: []string{"web.discovery"},
	}, app.RuntimeWorkerRegistrationOptions{Now: now}); err != nil {
		t.Fatalf("register runtime worker: %v", err)
	}
	oldHeartbeat := now.Add(-staleAfter - time.Minute).Format(time.RFC3339Nano)
	freshHeartbeat := now.Format(time.RFC3339Nano)
	writeCLIWorkerLeaseFixture(t, root, app.WorkerLeaseArtifact{
		Version:     1,
		RunID:       "run-cli-reclaim-ready",
		ExecutionID: "tool-cli-reclaim-ready",
		Tool:        "web-api-pack",
		Capability:  "web.discovery",
		WorkerID:    "remote-web-1",
		Status:      "active",
		AcquiredAt:  oldHeartbeat,
		HeartbeatAt: oldHeartbeat,
	})
	writeCLIWorkerLeaseFixture(t, root, app.WorkerLeaseArtifact{
		Version:     1,
		RunID:       "run-cli-reclaim-skip",
		ExecutionID: "tool-cli-reclaim-skip",
		Tool:        "web-api-pack",
		Capability:  "web.discovery",
		WorkerID:    "remote-web-1",
		Status:      "active",
		AcquiredAt:  freshHeartbeat,
		HeartbeatAt: freshHeartbeat,
	})
	reclaimReview, err := app.CreateWorkerLeaseReclaimDispatch(root, now, staleAfter)
	if err != nil {
		t.Fatalf("create reclaim review: %v", err)
	}
	if _, err := app.ExecuteWorkerLeaseReclaimDispatch(root, filepath.Base(reclaimReview.Path), app.WorkerLeaseReclaimExecutionOptions{
		Execute:    true,
		Now:        now.Add(time.Minute),
		StaleAfter: staleAfter,
	}); err != nil {
		t.Fatalf("execute reclaim review: %v", err)
	}
	readyDispatch := createCLIWorkerQueueDispatchForExecution(t, root, now.Add(2*time.Minute), "run-cli-reconcile-ready", "tool-cli-reconcile-ready")
	if _, err := app.SubmitRuntimeWorkerQueueDispatchResult(root, filepath.Base(readyDispatch.Path), app.RuntimeWorkerQueueDispatchResult{
		Status: "completed",
		Output: "ready result",
	}, app.RuntimeWorkerQueueDispatchResultOptions{Now: now.Add(3 * time.Minute)}); err != nil {
		t.Fatalf("submit ready reconciliation result: %v", err)
	}
	writeCLIWorkerLeaseFixture(t, root, app.WorkerLeaseArtifact{
		Version:     1,
		RunID:       readyDispatch.RunID,
		ExecutionID: readyDispatch.ExecutionID,
		Tool:        readyDispatch.Tool,
		Capability:  readyDispatch.Capability,
		WorkerID:    readyDispatch.WorkerID,
		Status:      "active",
		AcquiredAt:  now.Add(2 * time.Minute).Format(time.RFC3339Nano),
		HeartbeatAt: now.Add(2 * time.Minute).Format(time.RFC3339Nano),
	})
	skippedDispatch := createCLIWorkerQueueDispatchForExecution(t, root, now.Add(4*time.Minute), "run-cli-reconcile-skip", "tool-cli-reconcile-skip")
	if _, err := app.SubmitRuntimeWorkerQueueDispatchResult(root, filepath.Base(skippedDispatch.Path), app.RuntimeWorkerQueueDispatchResult{
		Status: "completed",
		Output: "aligned result",
	}, app.RuntimeWorkerQueueDispatchResultOptions{Now: now.Add(5 * time.Minute)}); err != nil {
		t.Fatalf("submit skipped reconciliation result: %v", err)
	}
	writeCLIWorkerLeaseFixture(t, root, app.WorkerLeaseArtifact{
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
	})
	reconciliationReview, err := app.CreateWorkerLeaseResultReconciliationDispatch(root, now.Add(6*time.Minute))
	if err != nil {
		t.Fatalf("create reconciliation review: %v", err)
	}
	if _, err := app.ExecuteWorkerLeaseResultReconciliationDispatch(root, filepath.Base(reconciliationReview.Path), app.WorkerLeaseResultReconciliationExecutionOptions{
		Execute: true,
		Now:     now.Add(7 * time.Minute),
	}); err != nil {
		t.Fatalf("execute reconciliation review: %v", err)
	}

	var out bytes.Buffer
	cmd := newRootCommand(&out, new(bytes.Buffer))
	cmd.SetArgs([]string{"runtime", "worker-lifecycle-audit", root})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute worker lifecycle audit: %v", err)
	}
	output := out.String()
	for _, want := range []string{
		"worker-lifecycle-audit-item: kind=worker-lease-reclaim-review",
		"worker-lifecycle-audit-item: kind=worker-lease-reclaim-execution",
		"worker-lifecycle-audit-item: kind=worker-lease-result-reconciliation-review",
		"worker-lifecycle-audit-item: kind=worker-lease-result-reconciliation-execution",
		"total=2 ready=1 skipped=1",
		"total=2 results=1 skipped=1",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected worker lifecycle audit output to contain %q, got %q", want, output)
		}
	}
}

func TestRuntimeWorkerLifecycleMaintenanceSupervisorCommandWithoutExecuteCreatesNoChildren(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Now().UTC()
	writeCLIWorkerLeaseFixture(t, root, app.WorkerLeaseArtifact{
		Version:     1,
		RunID:       "run-maintenance-supervisor-dry-cli",
		ExecutionID: "tool-maintenance-supervisor-dry-cli",
		Tool:        "web-api-pack",
		Capability:  "web.discovery",
		WorkerID:    "local",
		Status:      "active",
		AcquiredAt:  now.Add(-30 * time.Minute).Format(time.RFC3339Nano),
		HeartbeatAt: now.Add(-30 * time.Minute).Format(time.RFC3339Nano),
	})

	var out bytes.Buffer
	cmd := newRootCommand(&out, new(bytes.Buffer))
	cmd.SetArgs([]string{"runtime", "worker-lifecycle-maintenance-supervisor", root, "--max-runs", "2"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("dry-run worker lifecycle maintenance supervisor: %v", err)
	}
	output := out.String()
	for _, want := range []string{
		"worker-lifecycle-maintenance-supervisor-artifact:",
		"status=dry-run",
		"runs=0",
		"max_runs=2",
		"maintenance_created=0",
		"audit_created=0",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected dry-run supervisor output to contain %q, got %q", want, output)
		}
	}
	if maintenances, err := app.ListWorkerLifecycleMaintenances(root); err != nil {
		t.Fatalf("list worker lifecycle maintenances: %v", err)
	} else if len(maintenances) != 0 {
		t.Fatalf("dry-run supervisor must not create maintenance artifacts, got %#v", maintenances)
	}
	if audits, err := app.ListWorkerLifecycleAudits(root); err != nil {
		t.Fatalf("list worker lifecycle audits: %v", err)
	} else if len(audits) != 0 {
		t.Fatalf("dry-run supervisor must not create audit artifacts, got %#v", audits)
	}
}

func TestRuntimeExecuteWorkerLeaseResultReconciliationCommandRequiresExecuteFlag(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	dispatch := createCLIFileOutboxWorkerQueueDispatch(t, root)
	if _, err := app.SubmitRuntimeWorkerQueueDispatchResult(root, filepath.Base(dispatch.Path), app.RuntimeWorkerQueueDispatchResult{
		Status: "completed",
		Output: "remote result complete",
	}, app.RuntimeWorkerQueueDispatchResultOptions{Now: time.Now().UTC()}); err != nil {
		t.Fatalf("submit worker queue dispatch result: %v", err)
	}
	writeCLIWorkerLeaseFixture(t, root, app.WorkerLeaseArtifact{
		Version:     1,
		RunID:       dispatch.RunID,
		ExecutionID: dispatch.ExecutionID,
		Tool:        dispatch.Tool,
		Capability:  dispatch.Capability,
		WorkerID:    dispatch.WorkerID,
		Status:      "active",
		AcquiredAt:  time.Now().UTC().Add(-time.Minute).Format(time.RFC3339Nano),
		HeartbeatAt: time.Now().UTC().Add(-time.Minute).Format(time.RFC3339Nano),
	})
	review, err := app.CreateWorkerLeaseResultReconciliationDispatch(root, time.Now().UTC())
	if err != nil {
		t.Fatalf("create reconciliation review: %v", err)
	}

	var out bytes.Buffer
	cmd := newRootCommand(&out, new(bytes.Buffer))
	cmd.SetArgs([]string{"runtime", "execute-worker-lease-result-reconciliation", root, filepath.Base(review.Path)})
	err = cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "requires explicit execute") {
		t.Fatalf("expected explicit execute error, got %v output %q", err, out.String())
	}
	executions, err := app.ListWorkerLeaseResultReconciliationExecutions(root)
	if err != nil {
		t.Fatalf("list reconciliation executions: %v", err)
	}
	if len(executions) != 0 {
		t.Fatalf("expected no reconciliation executions without --execute, got %#v", executions)
	}
}

func TestRuntimeExecuteWorkerLeaseResultReconciliationCommandRunsWithExecuteFlag(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	dispatch := createCLIFileOutboxWorkerQueueDispatch(t, root)
	if _, err := app.SubmitRuntimeWorkerQueueDispatchResult(root, filepath.Base(dispatch.Path), app.RuntimeWorkerQueueDispatchResult{
		Status: "completed",
		Output: "remote result complete",
	}, app.RuntimeWorkerQueueDispatchResultOptions{Now: time.Now().UTC()}); err != nil {
		t.Fatalf("submit worker queue dispatch result: %v", err)
	}
	writeCLIWorkerLeaseFixture(t, root, app.WorkerLeaseArtifact{
		Version:     1,
		RunID:       dispatch.RunID,
		ExecutionID: dispatch.ExecutionID,
		Tool:        dispatch.Tool,
		Capability:  dispatch.Capability,
		WorkerID:    dispatch.WorkerID,
		Status:      "active",
		AcquiredAt:  time.Now().UTC().Add(-time.Minute).Format(time.RFC3339Nano),
		HeartbeatAt: time.Now().UTC().Add(-time.Minute).Format(time.RFC3339Nano),
	})
	review, err := app.CreateWorkerLeaseResultReconciliationDispatch(root, time.Now().UTC())
	if err != nil {
		t.Fatalf("create reconciliation review: %v", err)
	}

	var out bytes.Buffer
	cmd := newRootCommand(&out, new(bytes.Buffer))
	cmd.SetArgs([]string{"runtime", "execute-worker-lease-result-reconciliation", root, filepath.Base(review.Path), "--execute", "--released-by", "operator", "--reason", "reviewed"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute worker lease result reconciliation: %v", err)
	}
	output := out.String()
	for _, want := range []string{
		"worker-lease-result-reconciliation-execution:",
		"status=completed",
		"mode=metadata-only",
		"results=1",
		"release-worker-lease-result-reconciliation: run=" + dispatch.RunID,
		"status=completed",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected reconciliation execution output to contain %q, got %q", want, output)
		}
	}
	leases, err := app.ListWorkerLeases(root)
	if err != nil {
		t.Fatalf("list worker leases: %v", err)
	}
	if len(leases) != 1 || leases[0].Status != "completed" || leases[0].ReleasedBy != "operator" {
		t.Fatalf("expected completed released lease, got %#v", leases)
	}
	executions, err := app.ListWorkerLeaseResultReconciliationExecutions(root)
	if err != nil {
		t.Fatalf("list reconciliation executions: %v", err)
	}
	if len(executions) != 1 {
		t.Fatalf("expected one reconciliation execution, got %#v", executions)
	}

	var listOut bytes.Buffer
	listCmd := newRootCommand(&listOut, new(bytes.Buffer))
	listCmd.SetArgs([]string{"runtime", "worker-lease-result-reconciliation-executions", root})
	if err := listCmd.Execute(); err != nil {
		t.Fatalf("list worker lease result reconciliation executions: %v", err)
	}
	if !strings.Contains(listOut.String(), "worker-lease-result-reconciliation-executions: total=1") || !strings.Contains(listOut.String(), "release-worker-lease-result-reconciliation: run="+dispatch.RunID) {
		t.Fatalf("expected reconciliation execution listing, got %q", listOut.String())
	}
}

func TestRuntimeWorkerRegisterCommandWritesAndListsWorkers(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := app.InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}

	var out bytes.Buffer
	cmd := newRootCommand(&out, new(bytes.Buffer))
	cmd.SetArgs([]string{
		"runtime", "worker-register", root, "remote-web-1",
		"--mode", "remote",
		"--status", "available",
		"--endpoint", "tcp://127.0.0.1:9443",
		"--capability", "web.discovery",
		"--capability", "web.enumeration",
		"--label", "region=lab",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("register runtime worker: %v", err)
	}
	output := out.String()
	for _, want := range []string{
		"runtime-worker:",
		"worker=remote-web-1",
		"mode=remote",
		"status=available",
		"capabilities=2",
		"endpoint=tcp://127.0.0.1:9443",
		"runtime-worker-capability: worker=remote-web-1 capability=web.discovery",
		"runtime-worker-label: worker=remote-web-1 region=lab",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected worker-register output to contain %q, got %q", want, output)
		}
	}

	var listOut bytes.Buffer
	listCmd := newRootCommand(&listOut, new(bytes.Buffer))
	listCmd.SetArgs([]string{"runtime", "workers", root})
	if err := listCmd.Execute(); err != nil {
		t.Fatalf("list runtime workers: %v", err)
	}
	if !strings.Contains(listOut.String(), "runtime-workers: total=1") || !strings.Contains(listOut.String(), "worker=remote-web-1") {
		t.Fatalf("expected runtime workers listing, got %q", listOut.String())
	}

	var inspectOut bytes.Buffer
	inspectCmd := newRootCommand(&inspectOut, new(bytes.Buffer))
	inspectCmd.SetArgs([]string{"runtime", "workers", root, "remote-web-1"})
	if err := inspectCmd.Execute(); err != nil {
		t.Fatalf("inspect runtime worker: %v", err)
	}
	if !strings.Contains(inspectOut.String(), "runtime-worker:") || !strings.Contains(inspectOut.String(), "runtime-worker-capability: worker=remote-web-1 capability=web.enumeration") {
		t.Fatalf("expected inspected runtime worker output, got %q", inspectOut.String())
	}
}

func TestRuntimeWorkerHeartbeatCommandUpdatesWorker(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := app.InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	registeredAt := time.Now().UTC().Add(-20 * time.Minute)
	registered, err := app.RegisterRuntimeWorker(root, app.RuntimeWorkerArtifact{
		WorkerID:     "remote-web-1",
		Mode:         "remote",
		Status:       "available",
		Capabilities: []string{"web.discovery"},
	}, app.RuntimeWorkerRegistrationOptions{Now: registeredAt})
	if err != nil {
		t.Fatalf("register runtime worker: %v", err)
	}

	var out bytes.Buffer
	cmd := newRootCommand(&out, new(bytes.Buffer))
	cmd.SetArgs([]string{"runtime", "worker-heartbeat", root, "remote-web-1", "--status", "busy"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("heartbeat runtime worker: %v", err)
	}
	output := out.String()
	for _, want := range []string{
		"runtime-worker:",
		"worker=remote-web-1",
		"status=busy",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected worker-heartbeat output to contain %q, got %q", want, output)
		}
	}
	updated, err := app.ReadRuntimeWorker(root, filepath.Base(registered.Path))
	if err != nil {
		t.Fatalf("read updated runtime worker: %v", err)
	}
	if updated.Status != "busy" || updated.HeartbeatAt == registered.HeartbeatAt {
		t.Fatalf("expected heartbeat command to update status and heartbeat, got before=%#v after=%#v", registered, updated)
	}
}

func TestRuntimeWorkerPlacementPlanCommandWritesAndListsPlan(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := app.InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	if _, err := app.RegisterRuntimeWorker(root, app.RuntimeWorkerArtifact{
		WorkerID:     "remote-web-1",
		Mode:         "remote",
		Status:       "available",
		Capabilities: []string{"web.discovery"},
	}, app.RuntimeWorkerRegistrationOptions{Now: time.Now().UTC()}); err != nil {
		t.Fatalf("register runtime worker: %v", err)
	}

	var out bytes.Buffer
	cmd := newRootCommand(&out, new(bytes.Buffer))
	cmd.SetArgs([]string{"runtime", "worker-placement-plan", root, "--run-id", "run-placement-cli"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("create worker placement plan: %v", err)
	}
	output := out.String()
	for _, want := range []string{
		"worker-placement-plan:",
		"status=dry-run",
		"mode=metadata-only",
		"ready=2",
		"skipped=6",
		"worker-placement: status=ready",
		"worker=remote-web-1",
		"capability=web.discovery",
		"execution=tool-discover_assets-01",
		"parser=httpx_jsonl",
		"execution=tool-discover_assets-02",
		"parser=subfinder_jsonl",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected worker-placement-plan output to contain %q, got %q", want, output)
		}
	}
	plans, err := app.ListRuntimeWorkerPlacementPlans(root)
	if err != nil {
		t.Fatalf("list placement plans: %v", err)
	}
	if len(plans) != 1 {
		t.Fatalf("expected one placement plan artifact, got %#v", plans)
	}

	var listOut bytes.Buffer
	listCmd := newRootCommand(&listOut, new(bytes.Buffer))
	listCmd.SetArgs([]string{"runtime", "worker-placement-plans", root})
	if err := listCmd.Execute(); err != nil {
		t.Fatalf("list worker placement plans: %v", err)
	}
	if !strings.Contains(listOut.String(), "worker-placement-plans: total=1") || !strings.Contains(listOut.String(), "worker=remote-web-1") {
		t.Fatalf("expected placement plan listing, got %q", listOut.String())
	}

	var inspectOut bytes.Buffer
	inspectCmd := newRootCommand(&inspectOut, new(bytes.Buffer))
	inspectCmd.SetArgs([]string{"runtime", "worker-placement-plans", root, filepath.Base(plans[0].Path)})
	if err := inspectCmd.Execute(); err != nil {
		t.Fatalf("inspect worker placement plan: %v", err)
	}
	if !strings.Contains(inspectOut.String(), "worker-placement-plan: "+plans[0].Path) || !strings.Contains(inspectOut.String(), "worker-placement: status=ready") {
		t.Fatalf("expected inspected placement plan output, got %q", inspectOut.String())
	}
}

func TestRuntimeExecuteWorkerPlacementPlanCommandCreatesQueueClaim(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := app.InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	if _, err := app.RegisterRuntimeWorker(root, app.RuntimeWorkerArtifact{
		WorkerID:     "remote-web-1",
		Mode:         "remote",
		Status:       "available",
		Capabilities: []string{"web.discovery"},
	}, app.RuntimeWorkerRegistrationOptions{Now: time.Now().UTC()}); err != nil {
		t.Fatalf("register runtime worker: %v", err)
	}
	plan, err := app.CreateRuntimeWorkerPlacementPlan(root, app.RuntimeWorkerPlacementPlanOptions{
		Now:   time.Now().UTC(),
		RunID: "run-placement-cli-claim",
	})
	if err != nil {
		t.Fatalf("create placement plan: %v", err)
	}

	var rejectedOut bytes.Buffer
	rejectedCmd := newRootCommand(&rejectedOut, new(bytes.Buffer))
	rejectedCmd.SetArgs([]string{"runtime", "execute-worker-placement-plan", root, filepath.Base(plan.Path), "--execution-id", "tool-discover_assets-01", "--worker", "remote-web-1"})
	if err := rejectedCmd.Execute(); err == nil || !strings.Contains(err.Error(), "requires explicit execute") {
		t.Fatalf("expected missing execute rejection, got output=%q err=%v", rejectedOut.String(), err)
	}

	var out bytes.Buffer
	cmd := newRootCommand(&out, new(bytes.Buffer))
	cmd.SetArgs([]string{"runtime", "execute-worker-placement-plan", root, filepath.Base(plan.Path), "--execute", "--execution-id", "tool-discover_assets-01", "--worker", "remote-web-1"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute worker placement plan: %v", err)
	}
	output := out.String()
	for _, want := range []string{
		"worker-placement-claim-execution:",
		"status=claimed",
		"mode=metadata-only",
		"worker=remote-web-1",
		"claim=",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected placement claim execution output to contain %q, got %q", want, output)
		}
	}
	claims, err := app.ListRuntimeWorkerQueueClaims(root)
	if err != nil {
		t.Fatalf("list queue claims: %v", err)
	}
	if len(claims) != 1 || claims[0].WorkerID != "remote-web-1" || claims[0].Status != "claimed" {
		t.Fatalf("expected one queue claim from placement, got %#v", claims)
	}

	var listOut bytes.Buffer
	listCmd := newRootCommand(&listOut, new(bytes.Buffer))
	listCmd.SetArgs([]string{"runtime", "worker-placement-claim-executions", root})
	if err := listCmd.Execute(); err != nil {
		t.Fatalf("list worker placement claim executions: %v", err)
	}
	if !strings.Contains(listOut.String(), "worker-placement-claim-executions: total=1") || !strings.Contains(listOut.String(), "status=claimed") {
		t.Fatalf("expected placement claim execution listing, got %q", listOut.String())
	}
}

func TestRuntimeExecuteWorkerPlacementPlanCommandRejectsInvalidLeaseBeforeClaimWrite(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := app.InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	if _, err := app.RegisterRuntimeWorker(root, app.RuntimeWorkerArtifact{
		WorkerID:     "remote-web-1",
		Mode:         "remote",
		Status:       "available",
		Capabilities: []string{"web.discovery"},
	}, app.RuntimeWorkerRegistrationOptions{Now: time.Now().UTC()}); err != nil {
		t.Fatalf("register runtime worker: %v", err)
	}
	plan, err := app.CreateRuntimeWorkerPlacementPlan(root, app.RuntimeWorkerPlacementPlanOptions{
		Now:   time.Now().UTC(),
		RunID: "run-placement-invalid-lease-cli",
	})
	if err != nil {
		t.Fatalf("create placement plan: %v", err)
	}

	var out bytes.Buffer
	var stderr bytes.Buffer
	cmd := newRootCommand(&out, &stderr)
	cmd.SetArgs([]string{
		"runtime", "execute-worker-placement-plan", root, filepath.Base(plan.Path),
		"--execute",
		"--execution-id", "tool-discover_assets-01",
		"--worker", "remote-web-1",
		"--lease-seconds", "-1",
	})
	err = cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "--lease-seconds must be positive") {
		t.Fatalf("expected lease validation error before claim write, got %v stdout=%q stderr=%q", err, out.String(), stderr.String())
	}
	claims, listErr := app.ListRuntimeWorkerQueueClaims(root)
	if listErr != nil {
		t.Fatalf("list queue claims: %v", listErr)
	}
	if len(claims) != 0 {
		t.Fatalf("invalid placement claim lease must not write queue claims, got %#v", claims)
	}
	executions, listErr := app.ListRuntimeWorkerPlacementClaimExecutions(root)
	if listErr != nil {
		t.Fatalf("list placement claim executions: %v", listErr)
	}
	if len(executions) != 0 {
		t.Fatalf("invalid placement claim lease must not write execution artifacts, got %#v", executions)
	}
}

func TestRuntimeWorkerPlacementDispatchPlanCommandWritesAndListsPlan(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := app.InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	if _, err := app.RegisterRuntimeWorker(root, app.RuntimeWorkerArtifact{
		WorkerID:     "remote-web-1",
		Mode:         "remote",
		Status:       "available",
		Capabilities: []string{"web.discovery"},
	}, app.RuntimeWorkerRegistrationOptions{Now: time.Now().UTC()}); err != nil {
		t.Fatalf("register runtime worker: %v", err)
	}
	placement, err := app.CreateRuntimeWorkerPlacementPlan(root, app.RuntimeWorkerPlacementPlanOptions{
		Now:   time.Now().UTC(),
		RunID: "run-placement-dispatch-cli",
	})
	if err != nil {
		t.Fatalf("create placement plan: %v", err)
	}
	if _, err := app.ExecuteRuntimeWorkerPlacementClaim(root, placement.Path, app.RuntimeWorkerPlacementClaimExecutionOptions{
		Execute:              true,
		CandidateExecutionID: "tool-discover_assets-01",
		WorkerID:             "remote-web-1",
		Now:                  time.Now().UTC(),
	}); err != nil {
		t.Fatalf("execute placement claim: %v", err)
	}

	var out bytes.Buffer
	cmd := newRootCommand(&out, new(bytes.Buffer))
	cmd.SetArgs([]string{"runtime", "worker-placement-dispatch-plan", root, "--transport", "file-outbox"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("create worker placement dispatch plan: %v", err)
	}
	output := out.String()
	for _, want := range []string{
		"worker-placement-dispatch-plan:",
		"mode=metadata-only",
		"transport=file-outbox",
		"ready=1",
		"worker-placement-dispatch: status=ready",
		"claim=",
		"worker=remote-web-1",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected placement dispatch plan output to contain %q, got %q", want, output)
		}
	}
	dispatches, err := app.ListRuntimeWorkerQueueDispatchExecutions(root)
	if err != nil {
		t.Fatalf("list queue dispatches: %v", err)
	}
	if len(dispatches) != 0 {
		t.Fatalf("placement dispatch plan command must not dispatch, got %#v", dispatches)
	}

	plans, err := app.ListRuntimeWorkerPlacementDispatchPlans(root)
	if err != nil {
		t.Fatalf("list placement dispatch plans: %v", err)
	}
	if len(plans) != 1 {
		t.Fatalf("expected one placement dispatch plan artifact, got %#v", plans)
	}

	var listOut bytes.Buffer
	listCmd := newRootCommand(&listOut, new(bytes.Buffer))
	listCmd.SetArgs([]string{"runtime", "worker-placement-dispatch-plans", root})
	if err := listCmd.Execute(); err != nil {
		t.Fatalf("list worker placement dispatch plans: %v", err)
	}
	if !strings.Contains(listOut.String(), "worker-placement-dispatch-plans: total=1") || !strings.Contains(listOut.String(), "status=ready") {
		t.Fatalf("expected placement dispatch plan listing, got %q", listOut.String())
	}

	var inspectOut bytes.Buffer
	inspectCmd := newRootCommand(&inspectOut, new(bytes.Buffer))
	inspectCmd.SetArgs([]string{"runtime", "worker-placement-dispatch-plans", root, filepath.Base(plans[0].Path)})
	if err := inspectCmd.Execute(); err != nil {
		t.Fatalf("inspect worker placement dispatch plan: %v", err)
	}
	if !strings.Contains(inspectOut.String(), "worker-placement-dispatch-plan: "+plans[0].Path) || !strings.Contains(inspectOut.String(), "worker-placement-dispatch: status=ready") {
		t.Fatalf("expected inspected placement dispatch plan output, got %q", inspectOut.String())
	}
}

func TestRuntimeExecuteWorkerPlacementDispatchPlanCommandRunsWithExecuteFlag(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := app.InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	if _, err := app.RegisterRuntimeWorker(root, app.RuntimeWorkerArtifact{
		WorkerID:     "remote-web-1",
		Mode:         "remote",
		Status:       "available",
		Capabilities: []string{"web.discovery"},
	}, app.RuntimeWorkerRegistrationOptions{Now: time.Now().UTC()}); err != nil {
		t.Fatalf("register runtime worker: %v", err)
	}
	placement, err := app.CreateRuntimeWorkerPlacementPlan(root, app.RuntimeWorkerPlacementPlanOptions{
		Now:   time.Now().UTC(),
		RunID: "run-placement-dispatch-exec-cli",
	})
	if err != nil {
		t.Fatalf("create placement plan: %v", err)
	}
	if _, err := app.ExecuteRuntimeWorkerPlacementClaim(root, placement.Path, app.RuntimeWorkerPlacementClaimExecutionOptions{
		Execute:              true,
		CandidateExecutionID: "tool-discover_assets-01",
		WorkerID:             "remote-web-1",
		Now:                  time.Now().UTC(),
	}); err != nil {
		t.Fatalf("execute placement claim: %v", err)
	}
	plan, err := app.CreateRuntimeWorkerPlacementDispatchPlan(root, app.RuntimeWorkerPlacementDispatchPlanOptions{
		Now:           time.Now().UTC(),
		TransportMode: "metadata-only",
	})
	if err != nil {
		t.Fatalf("create placement dispatch plan: %v", err)
	}
	claim := plan.Candidates[0].ClaimPath

	var rejectedOut bytes.Buffer
	rejectedCmd := newRootCommand(&rejectedOut, new(bytes.Buffer))
	rejectedCmd.SetArgs([]string{"runtime", "execute-worker-placement-dispatch-plan", root, filepath.Base(plan.Path), "--claim", filepath.Base(claim)})
	if err := rejectedCmd.Execute(); err == nil || !strings.Contains(err.Error(), "requires explicit execute") {
		t.Fatalf("expected missing execute rejection, got output=%q err=%v", rejectedOut.String(), err)
	}

	var out bytes.Buffer
	cmd := newRootCommand(&out, new(bytes.Buffer))
	cmd.SetArgs([]string{"runtime", "execute-worker-placement-dispatch-plan", root, filepath.Base(plan.Path), "--execute", "--claim", filepath.Base(claim)})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute worker placement dispatch plan: %v", err)
	}
	output := out.String()
	for _, want := range []string{
		"worker-placement-dispatch-execution:",
		"status=dispatched",
		"mode=metadata-only",
		"transport=metadata-only",
		"queue_dispatch=",
		"claim=",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected placement dispatch execution output to contain %q, got %q", want, output)
		}
	}
	dispatches, err := app.ListRuntimeWorkerQueueDispatchExecutions(root)
	if err != nil {
		t.Fatalf("list queue dispatch executions: %v", err)
	}
	if len(dispatches) != 1 || dispatches[0].ClaimPath != claim {
		t.Fatalf("expected one queue dispatch execution, got %#v", dispatches)
	}

	var listOut bytes.Buffer
	listCmd := newRootCommand(&listOut, new(bytes.Buffer))
	listCmd.SetArgs([]string{"runtime", "worker-placement-dispatch-executions", root})
	if err := listCmd.Execute(); err != nil {
		t.Fatalf("list worker placement dispatch executions: %v", err)
	}
	if !strings.Contains(listOut.String(), "worker-placement-dispatch-executions: total=1") || !strings.Contains(listOut.String(), "status=dispatched") {
		t.Fatalf("expected placement dispatch execution listing, got %q", listOut.String())
	}
}

func TestRuntimeExecuteWorkerPlacementDispatchPlanCommandRunsAllReadyCandidates(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	plan := createPlacementDispatchBatchPlanForCLI(t, root, "metadata-only")

	var rejectedOut bytes.Buffer
	rejectedCmd := newRootCommand(&rejectedOut, new(bytes.Buffer))
	rejectedCmd.SetArgs([]string{"runtime", "execute-worker-placement-dispatch-plan", root, filepath.Base(plan.Path), "--all"})
	if err := rejectedCmd.Execute(); err == nil || !strings.Contains(err.Error(), "requires explicit execute") {
		t.Fatalf("expected missing execute rejection, got output=%q err=%v", rejectedOut.String(), err)
	}

	var out bytes.Buffer
	cmd := newRootCommand(&out, new(bytes.Buffer))
	cmd.SetArgs([]string{"runtime", "execute-worker-placement-dispatch-plan", root, filepath.Base(plan.Path), "--execute", "--all"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute all worker placement dispatches: %v", err)
	}
	output := out.String()
	for _, want := range []string{
		"worker-placement-dispatch-batch:",
		"status=completed",
		"mode=metadata-only",
		"transport=metadata-only",
		"total=8",
		"dispatched=8",
		"worker-placement-dispatch-execution:",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected placement dispatch batch output to contain %q, got %q", want, output)
		}
	}
	dispatches, err := app.ListRuntimeWorkerQueueDispatchExecutions(root)
	if err != nil {
		t.Fatalf("list queue dispatch executions: %v", err)
	}
	if len(dispatches) != 8 {
		t.Fatalf("expected eight queue dispatch executions, got %#v", dispatches)
	}

	var listOut bytes.Buffer
	listCmd := newRootCommand(&listOut, new(bytes.Buffer))
	listCmd.SetArgs([]string{"runtime", "worker-placement-dispatch-batches", root})
	if err := listCmd.Execute(); err != nil {
		t.Fatalf("list worker placement dispatch batches: %v", err)
	}
	if !strings.Contains(listOut.String(), "worker-placement-dispatch-batches: total=1") || !strings.Contains(listOut.String(), "dispatched=8") {
		t.Fatalf("expected placement dispatch batch listing, got %q", listOut.String())
	}
}

func TestRuntimeExecuteWorkerPlacementDispatchPlanCommandRejectsNegativeMaxCandidatesBeforeBatch(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	plan := createPlacementDispatchBatchPlanForCLI(t, root, "metadata-only")

	var out bytes.Buffer
	var stderr bytes.Buffer
	cmd := newRootCommand(&out, &stderr)
	cmd.SetArgs([]string{
		"runtime", "execute-worker-placement-dispatch-plan", root, filepath.Base(plan.Path),
		"--execute",
		"--all",
		"--max-candidates", "-1",
	})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "--max-candidates cannot be negative") {
		t.Fatalf("expected max-candidates validation error before batch dispatch, got %v stdout=%q stderr=%q", err, out.String(), stderr.String())
	}
	batches, listErr := app.ListRuntimeWorkerPlacementDispatchBatches(root)
	if listErr != nil {
		t.Fatalf("list placement dispatch batches: %v", listErr)
	}
	if len(batches) != 0 {
		t.Fatalf("negative max-candidates must not write dispatch batches, got %#v", batches)
	}
	dispatches, listErr := app.ListRuntimeWorkerQueueDispatchExecutions(root)
	if listErr != nil {
		t.Fatalf("list queue dispatch executions: %v", listErr)
	}
	if len(dispatches) != 0 {
		t.Fatalf("negative max-candidates must not dispatch queue claims, got %#v", dispatches)
	}
}

func TestRuntimeExecuteWorkerCommandsRequireExecuteBeforeTransportTokenLookup(t *testing.T) {
	missingTokenEnv := "OURO_TEST_MISSING_RUNTIME_TRANSPORT_TOKEN"
	t.Setenv(missingTokenEnv, "")
	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{
			name:    "direct queue dispatch",
			args:    []string{"runtime", "execute-worker-queue-claim", filepath.Join(t.TempDir(), "workspace"), "claim-does-not-matter", "--transport", "http-json", "--transport-token-env", missingTokenEnv},
			wantErr: "worker queue claim dispatch requires explicit execute",
		},
		{
			name:    "placement dispatch candidate",
			args:    []string{"runtime", "execute-worker-placement-dispatch-plan", filepath.Join(t.TempDir(), "workspace"), "plan-does-not-matter", "--transport-token-env", missingTokenEnv},
			wantErr: "worker placement dispatch requires explicit execute",
		},
		{
			name:    "placement dispatch batch",
			args:    []string{"runtime", "execute-worker-placement-dispatch-plan", filepath.Join(t.TempDir(), "workspace"), "plan-does-not-matter", "--all", "--transport-token-env", missingTokenEnv},
			wantErr: "worker placement dispatch batch requires explicit execute",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			var stderr bytes.Buffer
			cmd := newRootCommand(&out, &stderr)
			cmd.SetArgs(tt.args)
			err := cmd.Execute()
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("expected explicit execute error before token lookup, got %v stdout=%q stderr=%q", err, out.String(), stderr.String())
			}
			if strings.Contains(err.Error(), "transport token env") {
				t.Fatalf("missing execute must be reported before token lookup, got %v", err)
			}
		})
	}
}

func TestRuntimeExecuteWorkerPlacementDispatchPlanSkipsTransportTokenForNonHTTPPlans(t *testing.T) {
	missingTokenEnv := "OURO_TEST_MISSING_PLACEMENT_DISPATCH_TOKEN"
	t.Setenv(missingTokenEnv, "")
	for _, transportMode := range []string{"metadata-only", "file-outbox"} {
		t.Run(transportMode, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "workspace")
			plan := createPlacementDispatchBatchPlanForCLI(t, root, transportMode)

			var out bytes.Buffer
			var stderr bytes.Buffer
			cmd := newRootCommand(&out, &stderr)
			cmd.SetArgs([]string{
				"runtime", "execute-worker-placement-dispatch-plan", root, filepath.Base(plan.Path),
				"--execute",
				"--all",
				"--transport-token-env", missingTokenEnv,
			})
			if err := cmd.Execute(); err != nil {
				t.Fatalf("non-http placement dispatch should not require transport token env: %v stdout=%q stderr=%q", err, out.String(), stderr.String())
			}
			for _, want := range []string{
				"worker-placement-dispatch-batch:",
				"status=completed",
				"transport=" + transportMode,
				"dispatched=8",
			} {
				if !strings.Contains(out.String(), want) {
					t.Fatalf("expected placement dispatch output to contain %q, got %q", want, out.String())
				}
			}
			dispatches, err := app.ListRuntimeWorkerQueueDispatchExecutions(root)
			if err != nil {
				t.Fatalf("list queue dispatch executions: %v", err)
			}
			if len(dispatches) != 8 {
				t.Fatalf("expected eight non-http queue dispatch executions, got %#v", dispatches)
			}
		})
	}
}

func TestRuntimeWorkerOrchestrateCommandWritesMetadataOnlyArtifact(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := app.InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	if _, err := app.RegisterRuntimeWorker(root, app.RuntimeWorkerArtifact{
		WorkerID:     "remote-web-1",
		Mode:         "remote",
		Status:       "available",
		Capabilities: []string{"web.discovery"},
	}, app.RuntimeWorkerRegistrationOptions{Now: time.Now().UTC()}); err != nil {
		t.Fatalf("register runtime worker: %v", err)
	}

	var out bytes.Buffer
	cmd := newRootCommand(&out, new(bytes.Buffer))
	cmd.SetArgs([]string{"runtime", "worker-orchestrate", root, "--run-id", "run-worker-orchestrate-cli"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute worker-orchestrate: %v", err)
	}
	output := out.String()
	for _, want := range []string{
		"worker-orchestration:",
		"status=dry-run",
		"mode=metadata-only",
		"transport=metadata-only",
		"placement_ready=2",
		"placement_skipped=6",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected worker-orchestrate output to contain %q, got %q", want, output)
		}
	}
	orchestrations, err := app.ListRuntimeWorkerOrchestrations(root)
	if err != nil {
		t.Fatalf("list worker orchestrations: %v", err)
	}
	if len(orchestrations) != 1 {
		t.Fatalf("expected one worker orchestration artifact, got %#v", orchestrations)
	}
	placementPlans, err := app.ListRuntimeWorkerPlacementPlans(root)
	if err != nil {
		t.Fatalf("list placement plans: %v", err)
	}
	if len(placementPlans) != 0 {
		t.Fatalf("dry-run worker orchestration must not create placement plans, got %#v", placementPlans)
	}
	claims, err := app.ListRuntimeWorkerQueueClaims(root)
	if err != nil {
		t.Fatalf("list queue claims: %v", err)
	}
	if len(claims) != 0 {
		t.Fatalf("dry-run worker orchestration must not create queue claims, got %#v", claims)
	}
}

func TestRuntimeWorkerOrchestrationDryRunCommandsDoNotRequireTransportTokenEnv(t *testing.T) {
	missingTokenEnv := "OURO_TEST_MISSING_RUNTIME_ORCHESTRATION_TOKEN"
	t.Setenv(missingTokenEnv, "")
	tests := []struct {
		name string
		args func(root string) []string
	}{
		{
			name: "orchestration dry-run",
			args: func(root string) []string {
				return []string{"runtime", "worker-orchestrate", root, "--run-id", "run-token-dry-cli", "--transport", "http-json", "--transport-token-env", missingTokenEnv}
			},
		},
		{
			name: "orchestration supervisor dry-run",
			args: func(root string) []string {
				return []string{"runtime", "worker-orchestration-supervisor", root, "--run-id-prefix", "run-token-supervisor-dry-cli", "--transport", "http-json", "--transport-token-env", missingTokenEnv}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "workspace")
			registerCLIWorkerOrchestrationWorkers(t, root)

			var out bytes.Buffer
			var stderr bytes.Buffer
			cmd := newRootCommand(&out, &stderr)
			cmd.SetArgs(tt.args(root))
			if err := cmd.Execute(); err != nil {
				t.Fatalf("dry-run orchestration command should not require transport token env: %v stdout=%q stderr=%q", err, out.String(), stderr.String())
			}
			for _, want := range []string{"status=dry-run", "mode=metadata-only", "transport=http-json"} {
				if !strings.Contains(out.String(), want) {
					t.Fatalf("expected dry-run output to contain %q, got %q", want, out.String())
				}
			}
			dispatches, err := app.ListRuntimeWorkerQueueDispatchExecutions(root)
			if err != nil {
				t.Fatalf("list queue dispatch executions: %v", err)
			}
			if len(dispatches) != 0 {
				t.Fatalf("dry-run orchestration must not dispatch queue claims, got %#v", dispatches)
			}
		})
	}
}

func TestRuntimeWorkerOrchestrateCommandExecuteComposesWorkerGates(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	registerCLIWorkerOrchestrationWorkers(t, root)

	var out bytes.Buffer
	cmd := newRootCommand(&out, new(bytes.Buffer))
	cmd.SetArgs([]string{"runtime", "worker-orchestrate", root, "--execute", "--run-id", "run-worker-orchestrate-exec-cli", "--max-candidates", "2"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute worker-orchestrate --execute: %v", err)
	}
	output := out.String()
	for _, want := range []string{
		"worker-orchestration:",
		"status=completed",
		"mode=execute",
		"transport=metadata-only",
		"placement_ready=8",
		"claimed=2",
		"dispatch_ready=2",
		"dispatched=2",
		"placement-plan=",
		"placement-dispatch-plan=",
		"placement-dispatch-batch=",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected worker-orchestrate --execute output to contain %q, got %q", want, output)
		}
	}
	dispatches, err := app.ListRuntimeWorkerQueueDispatchExecutions(root)
	if err != nil {
		t.Fatalf("list queue dispatch executions: %v", err)
	}
	if len(dispatches) != 2 {
		t.Fatalf("expected two queue dispatch executions, got %#v", dispatches)
	}
}

func TestRuntimeWorkerOrchestrateCommandExecuteCanAttachLifecycleMaintenanceAudit(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	registerCLIWorkerOrchestrationWorkers(t, root)

	var out bytes.Buffer
	cmd := newRootCommand(&out, new(bytes.Buffer))
	cmd.SetArgs([]string{
		"runtime", "worker-orchestrate", root,
		"--execute",
		"--run-id", "run-worker-orchestrate-lifecycle-cli",
		"--max-candidates", "1",
		"--include-lifecycle-maintenance",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute worker-orchestrate lifecycle maintenance: %v", err)
	}
	output := out.String()
	for _, want := range []string{
		"worker-orchestration:",
		"status=completed",
		"lifecycle-maintenance=",
		"lifecycle-audit=",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected worker-orchestrate lifecycle output to contain %q, got %q", want, output)
		}
	}
	orchestrations, err := app.ListRuntimeWorkerOrchestrations(root)
	if err != nil {
		t.Fatalf("list worker orchestrations: %v", err)
	}
	if len(orchestrations) != 1 || orchestrations[0].LifecycleMaintenancePath == "" || orchestrations[0].LifecycleAuditPath == "" {
		t.Fatalf("expected persisted lifecycle links, got %#v", orchestrations)
	}
	maintenances, err := app.ListWorkerLifecycleMaintenances(root)
	if err != nil {
		t.Fatalf("list worker lifecycle maintenances: %v", err)
	}
	audits, err := app.ListWorkerLifecycleAudits(root)
	if err != nil {
		t.Fatalf("list worker lifecycle audits: %v", err)
	}
	if len(maintenances) != 1 || len(audits) != 1 {
		t.Fatalf("expected one maintenance and one audit artifact, got maintenances=%#v audits=%#v", maintenances, audits)
	}
}

func TestRuntimeWorkerOrchestrateCommandExecutesLifecycleRetryDispatchesWithExplicitFlag(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	marker := filepath.Join(t.TempDir(), "orchestration-retry-dispatch-cli-explicit-ran")
	failedDispatch := createCLIFileOutboxWorkerQueueDispatchWithToolPlan(t, root, app.RuntimeWorkerToolPlanPayload{
		Action:     "discover_assets",
		Runner:     "command",
		Parser:     "web_api_parser",
		RunCommand: []string{"sh", "-c", "touch " + marker},
	})
	if _, err := app.SubmitRuntimeWorkerQueueDispatchResult(root, filepath.Base(failedDispatch.Path), app.RuntimeWorkerQueueDispatchResult{
		Status: "failed",
		Error:  "exit status 23",
	}, app.RuntimeWorkerQueueDispatchResultOptions{Now: time.Now().UTC()}); err != nil {
		t.Fatalf("submit failed worker queue dispatch result: %v", err)
	}
	plan, err := app.CreateRuntimeWorkerDispatchRetryPlan(root, time.Now().UTC())
	if err != nil {
		t.Fatalf("create worker dispatch retry plan: %v", err)
	}
	if _, err := app.CreateRuntimeWorkerDispatchRetryDispatch(root, filepath.Base(plan.Path), time.Now().UTC()); err != nil {
		t.Fatalf("create worker dispatch retry dispatch: %v", err)
	}
	claimsBefore, err := app.ListRuntimeWorkerQueueClaims(root)
	if err != nil {
		t.Fatalf("list queue claims before orchestration: %v", err)
	}
	requestsBefore, err := app.ListRuntimeWorkerTransportDispatchRequests(root, "")
	if err != nil {
		t.Fatalf("list transport requests before orchestration: %v", err)
	}

	var out bytes.Buffer
	cmd := newRootCommand(&out, new(bytes.Buffer))
	cmd.SetArgs([]string{
		"runtime", "worker-orchestrate", root,
		"--execute",
		"--run-id", "run-worker-orchestrate-lifecycle-retry-cli",
		"--max-candidates", "1",
		"--include-lifecycle-maintenance",
		"--execute-lifecycle-retry-dispatches",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute worker-orchestrate lifecycle retry dispatches: %v", err)
	}
	output := out.String()
	for _, want := range []string{
		"worker-orchestration:",
		"status=completed",
		"lifecycle-maintenance=",
		"lifecycle-audit=",
		"retry_dispatch_executions_created=1",
		"retry_dispatch_claims_created=1",
		"worker-orchestration-retry-dispatch-execution:",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected worker-orchestrate lifecycle retry output to contain %q, got %q", want, output)
		}
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("orchestration lifecycle retry dispatch must not execute retry tool-plan command, marker stat err=%v", err)
	}
	claimsAfter, err := app.ListRuntimeWorkerQueueClaims(root)
	if err != nil {
		t.Fatalf("list queue claims after orchestration: %v", err)
	}
	if len(claimsAfter) != len(claimsBefore)+2 {
		t.Fatalf("expected placement claim plus retry queue claim: before=%d after=%d", len(claimsBefore), len(claimsAfter))
	}
	requestsAfter, err := app.ListRuntimeWorkerTransportDispatchRequests(root, "")
	if err != nil {
		t.Fatalf("list transport requests after orchestration: %v", err)
	}
	if len(requestsAfter) != len(requestsBefore) {
		t.Fatalf("orchestration lifecycle retry dispatch must not create file-outbox requests: before=%d after=%d", len(requestsBefore), len(requestsAfter))
	}
	executions, err := app.ListRuntimeWorkerDispatchRetryDispatchExecutions(root)
	if err != nil {
		t.Fatalf("list worker dispatch retry executions: %v", err)
	}
	if len(executions) != 1 || executions[0].Summary.Claimed != 1 || executions[0].Summary.Dispatched != 0 {
		t.Fatalf("expected one metadata-only retry dispatch execution, got %#v", executions)
	}
}

func TestRuntimeWorkerOrchestrateCommandRejectsLifecycleRetryDispatchFlagWithoutRequiredGates(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{
			name: "without execute",
			args: []string{"--include-lifecycle-maintenance", "--execute-lifecycle-retry-dispatches"},
			want: "--execute-lifecycle-retry-dispatches requires --execute",
		},
		{
			name: "without lifecycle maintenance",
			args: []string{"--execute", "--execute-lifecycle-retry-dispatches"},
			want: "--execute-lifecycle-retry-dispatches requires --include-lifecycle-maintenance",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "workspace")
			registerCLIWorkerOrchestrationWorkers(t, root)

			var out bytes.Buffer
			var errOut bytes.Buffer
			cmd := newRootCommand(&out, &errOut)
			args := append([]string{"runtime", "worker-orchestrate", root}, tc.args...)
			cmd.SetArgs(args)
			err := cmd.Execute()
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected %q error, got err=%v stdout=%q stderr=%q", tc.want, err, out.String(), errOut.String())
			}
			orchestrations, listErr := app.ListRuntimeWorkerOrchestrations(root)
			if listErr != nil {
				t.Fatalf("list worker orchestrations after rejected command: %v", listErr)
			}
			if len(orchestrations) != 0 {
				t.Fatalf("rejected lifecycle retry flag must fail before orchestration writes, got %#v", orchestrations)
			}
		})
	}
}

func TestRuntimeWorkerOrchestrationSupervisorCommandExecutesLifecycleRetryDispatchesWithExplicitFlag(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	marker := filepath.Join(t.TempDir(), "orchestration-supervisor-retry-dispatch-cli-explicit-ran")
	failedDispatch := createCLIFileOutboxWorkerQueueDispatchWithToolPlan(t, root, app.RuntimeWorkerToolPlanPayload{
		Action:     "discover_assets",
		Runner:     "command",
		Parser:     "web_api_parser",
		RunCommand: []string{"sh", "-c", "touch " + marker},
	})
	if _, err := app.SubmitRuntimeWorkerQueueDispatchResult(root, filepath.Base(failedDispatch.Path), app.RuntimeWorkerQueueDispatchResult{
		Status: "failed",
		Error:  "exit status 23",
	}, app.RuntimeWorkerQueueDispatchResultOptions{Now: time.Now().UTC()}); err != nil {
		t.Fatalf("submit failed worker queue dispatch result: %v", err)
	}
	plan, err := app.CreateRuntimeWorkerDispatchRetryPlan(root, time.Now().UTC())
	if err != nil {
		t.Fatalf("create worker dispatch retry plan: %v", err)
	}
	if _, err := app.CreateRuntimeWorkerDispatchRetryDispatch(root, filepath.Base(plan.Path), time.Now().UTC()); err != nil {
		t.Fatalf("create worker dispatch retry dispatch: %v", err)
	}
	claimsBefore, err := app.ListRuntimeWorkerQueueClaims(root)
	if err != nil {
		t.Fatalf("list queue claims before supervisor: %v", err)
	}
	requestsBefore, err := app.ListRuntimeWorkerTransportDispatchRequests(root, "")
	if err != nil {
		t.Fatalf("list transport requests before supervisor: %v", err)
	}

	var out bytes.Buffer
	cmd := newRootCommand(&out, new(bytes.Buffer))
	cmd.SetArgs([]string{
		"runtime", "worker-orchestration-supervisor", root,
		"--execute",
		"--run-id-prefix", "run-worker-orchestration-supervisor-lifecycle-retry-cli",
		"--max-runs", "1",
		"--max-candidates", "1",
		"--include-lifecycle-maintenance",
		"--execute-lifecycle-retry-dispatches",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute worker-orchestration-supervisor lifecycle retry dispatches: %v", err)
	}
	output := out.String()
	for _, want := range []string{
		"worker-orchestration-supervisor-artifact:",
		"status=completed",
		"lifecycle_maintenance=1",
		"lifecycle_audit=1",
		"retry_dispatch_executions_created=1",
		"retry_dispatch_claims_created=1",
		"worker-orchestration-supervisor-retry-dispatch-execution:",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected worker-orchestration-supervisor lifecycle retry output to contain %q, got %q", want, output)
		}
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("supervisor lifecycle retry dispatch must not execute retry tool-plan command, marker stat err=%v", err)
	}
	claimsAfter, err := app.ListRuntimeWorkerQueueClaims(root)
	if err != nil {
		t.Fatalf("list queue claims after supervisor: %v", err)
	}
	if len(claimsAfter) != len(claimsBefore)+2 {
		t.Fatalf("expected child orchestration placement claim plus retry queue claim: before=%d after=%d", len(claimsBefore), len(claimsAfter))
	}
	requestsAfter, err := app.ListRuntimeWorkerTransportDispatchRequests(root, "")
	if err != nil {
		t.Fatalf("list transport requests after supervisor: %v", err)
	}
	if len(requestsAfter) != len(requestsBefore) {
		t.Fatalf("supervisor lifecycle retry dispatch must not create file-outbox requests: before=%d after=%d", len(requestsBefore), len(requestsAfter))
	}
	executions, err := app.ListRuntimeWorkerDispatchRetryDispatchExecutions(root)
	if err != nil {
		t.Fatalf("list worker dispatch retry executions: %v", err)
	}
	if len(executions) != 1 || executions[0].Summary.Claimed != 1 || executions[0].Summary.Dispatched != 0 {
		t.Fatalf("expected one metadata-only retry dispatch execution, got %#v", executions)
	}
}

func TestRuntimeWorkerOrchestrationSupervisorCommandRejectsLifecycleRetryDispatchFlagWithoutRequiredGates(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{
			name: "without execute",
			args: []string{"--max-runs", "1", "--include-lifecycle-maintenance", "--execute-lifecycle-retry-dispatches"},
			want: "--execute-lifecycle-retry-dispatches requires --execute",
		},
		{
			name: "without lifecycle maintenance",
			args: []string{"--execute", "--max-runs", "1", "--execute-lifecycle-retry-dispatches"},
			want: "--execute-lifecycle-retry-dispatches requires --include-lifecycle-maintenance",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "workspace")
			registerCLIWorkerOrchestrationWorkers(t, root)

			var out bytes.Buffer
			var errOut bytes.Buffer
			cmd := newRootCommand(&out, &errOut)
			args := append([]string{"runtime", "worker-orchestration-supervisor", root}, tc.args...)
			cmd.SetArgs(args)
			err := cmd.Execute()
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected %q error, got err=%v stdout=%q stderr=%q", tc.want, err, out.String(), errOut.String())
			}
			supervisors, listErr := app.ListRuntimeWorkerOrchestrationSupervisors(root)
			if listErr != nil {
				t.Fatalf("list worker orchestration supervisors after rejected command: %v", listErr)
			}
			if len(supervisors) != 0 {
				t.Fatalf("rejected lifecycle retry flag must fail before supervisor writes, got %#v", supervisors)
			}
		})
	}
}

func TestRuntimeWorkerOrchestrationsCommandListsAndInspectsArtifacts(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := app.InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	if _, err := app.RegisterRuntimeWorker(root, app.RuntimeWorkerArtifact{
		WorkerID:     "remote-web-1",
		Mode:         "remote",
		Status:       "available",
		Capabilities: []string{"web.discovery"},
	}, app.RuntimeWorkerRegistrationOptions{Now: time.Now().UTC()}); err != nil {
		t.Fatalf("register runtime worker: %v", err)
	}
	orchestration, err := app.CreateRuntimeWorkerOrchestration(root, app.RuntimeWorkerOrchestrationOptions{
		Now:   time.Now().UTC(),
		RunID: "run-worker-orchestrations-list-cli",
	})
	if err != nil {
		t.Fatalf("create worker orchestration: %v", err)
	}

	var out bytes.Buffer
	cmd := newRootCommand(&out, new(bytes.Buffer))
	cmd.SetArgs([]string{"runtime", "worker-orchestrations", root})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute worker-orchestrations: %v", err)
	}
	output := out.String()
	for _, want := range []string{
		"worker-orchestrations: total=1",
		"worker-orchestration: " + orchestration.Path,
		"status=dry-run",
		"mode=metadata-only",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected worker-orchestrations output to contain %q, got %q", want, output)
		}
	}

	var inspectOut bytes.Buffer
	inspectCmd := newRootCommand(&inspectOut, new(bytes.Buffer))
	inspectCmd.SetArgs([]string{"runtime", "worker-orchestrations", root, filepath.Base(orchestration.Path)})
	if err := inspectCmd.Execute(); err != nil {
		t.Fatalf("execute worker-orchestrations inspect: %v", err)
	}
	if !strings.Contains(inspectOut.String(), "worker-orchestration: "+orchestration.Path) || !strings.Contains(inspectOut.String(), "placement_ready=2") || !strings.Contains(inspectOut.String(), "placement_skipped=6") {
		t.Fatalf("expected inspected worker orchestration output, got %q", inspectOut.String())
	}
}

func TestRuntimeWorkerOrchestrationSupervisorCommandRunsBoundedWorkerOrchestrations(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	registerCLIWorkerOrchestrationWorkers(t, root)

	var dryOut bytes.Buffer
	dryCmd := newRootCommand(&dryOut, new(bytes.Buffer))
	dryCmd.SetArgs([]string{
		"runtime", "worker-orchestration-supervisor", root,
		"--run-id-prefix", "run-worker-orchestration-supervisor-dry-cli",
		"--max-runs", "2",
		"--max-candidates", "1",
	})
	if err := dryCmd.Execute(); err != nil {
		t.Fatalf("execute dry-run worker-orchestration-supervisor: %v", err)
	}
	for _, want := range []string{
		"worker-orchestration-supervisor-artifact:",
		"status=dry-run",
		"mode=metadata-only",
		"transport=metadata-only",
		"runs=0",
		"max_runs=2",
		"orchestrations_created=0",
	} {
		if !strings.Contains(dryOut.String(), want) {
			t.Fatalf("expected dry-run supervisor output to contain %q, got %q", want, dryOut.String())
		}
	}
	if orchestrations, err := app.ListRuntimeWorkerOrchestrations(root); err != nil {
		t.Fatalf("list worker orchestrations: %v", err)
	} else if len(orchestrations) != 0 {
		t.Fatalf("dry-run supervisor must not create child orchestrations, got %#v", orchestrations)
	}

	var out bytes.Buffer
	cmd := newRootCommand(&out, new(bytes.Buffer))
	cmd.SetArgs([]string{
		"runtime", "worker-orchestration-supervisor", root,
		"--execute",
		"--run-id-prefix", "run-worker-orchestration-supervisor-exec-cli",
		"--max-runs", "2",
		"--max-candidates", "1",
		"--include-lifecycle-maintenance",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute worker-orchestration-supervisor --execute: %v", err)
	}
	output := out.String()
	for _, want := range []string{
		"worker-orchestration-supervisor-artifact:",
		"status=completed",
		"mode=metadata-only",
		"transport=metadata-only",
		"runs=2",
		"max_runs=2",
		"orchestrations_created=2",
		"claimed=2",
		"dispatched=2",
		"lifecycle_maintenance=2",
		"lifecycle_audit=2",
		"worker-orchestration-supervisor-orchestration:",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected worker-orchestration-supervisor output to contain %q, got %q", want, output)
		}
	}
	supervisors, err := app.ListRuntimeWorkerOrchestrationSupervisors(root)
	if err != nil {
		t.Fatalf("list worker orchestration supervisors: %v", err)
	}
	if len(supervisors) != 2 {
		t.Fatalf("expected dry-run and executed supervisor artifacts, got %#v", supervisors)
	}
	orchestrations, err := app.ListRuntimeWorkerOrchestrations(root)
	if err != nil {
		t.Fatalf("list worker orchestrations: %v", err)
	}
	if len(orchestrations) != 2 {
		t.Fatalf("expected two worker orchestration artifacts, got %#v", orchestrations)
	}

	var listOut bytes.Buffer
	listCmd := newRootCommand(&listOut, new(bytes.Buffer))
	listCmd.SetArgs([]string{"runtime", "worker-orchestration-supervisor-runs", root})
	if err := listCmd.Execute(); err != nil {
		t.Fatalf("list worker orchestration supervisor runs: %v", err)
	}
	if !strings.Contains(listOut.String(), "worker-orchestration-supervisor-runs: total=2") || !strings.Contains(listOut.String(), "orchestrations_created=2") {
		t.Fatalf("expected worker orchestration supervisor listing, got %q", listOut.String())
	}

	var inspectOut bytes.Buffer
	inspectCmd := newRootCommand(&inspectOut, new(bytes.Buffer))
	inspectCmd.SetArgs([]string{"runtime", "worker-orchestration-supervisor-runs", root, filepath.Base(supervisors[1].Path)})
	if err := inspectCmd.Execute(); err != nil {
		t.Fatalf("inspect worker orchestration supervisor run: %v", err)
	}
	if !strings.Contains(inspectOut.String(), "worker-orchestration-supervisor-artifact: "+supervisors[1].Path) || !strings.Contains(inspectOut.String(), "runs=2") {
		t.Fatalf("expected inspected worker orchestration supervisor output, got %q", inspectOut.String())
	}
}

func TestRuntimeWorkerOrchestrationSupervisorCommandRejectsInvalidBounds(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")

	var out bytes.Buffer
	var stderr bytes.Buffer
	cmd := newRootCommand(&out, &stderr)
	cmd.SetArgs([]string{"runtime", "worker-orchestration-supervisor", root, "--max-runs", "0"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "--max-runs must be positive") {
		t.Fatalf("expected max-runs validation error, got %v stdout=%q stderr=%q", err, out.String(), stderr.String())
	}
	supervisors, listErr := app.ListRuntimeWorkerOrchestrationSupervisors(root)
	if listErr != nil {
		t.Fatalf("list worker orchestration supervisors: %v", listErr)
	}
	if len(supervisors) != 0 {
		t.Fatalf("invalid supervisor bounds must not write artifacts, got %#v", supervisors)
	}
}

func TestRuntimeWorkerOrchestrationCommandsRejectInvalidCandidateAndLeaseBoundsBeforeArtifacts(t *testing.T) {
	tests := []struct {
		name    string
		args    func(root string) []string
		wantErr string
	}{
		{
			name: "orchestrate negative max candidates",
			args: func(root string) []string {
				return []string{"runtime", "worker-orchestrate", root, "--max-candidates", "-1"}
			},
			wantErr: "--max-candidates cannot be negative",
		},
		{
			name: "orchestrate negative lease",
			args: func(root string) []string {
				return []string{"runtime", "worker-orchestrate", root, "--lease-seconds", "-1"}
			},
			wantErr: "--lease-seconds must be positive",
		},
		{
			name: "supervisor negative max candidates",
			args: func(root string) []string {
				return []string{"runtime", "worker-orchestration-supervisor", root, "--max-runs", "1", "--max-candidates", "-1"}
			},
			wantErr: "--max-candidates cannot be negative",
		},
		{
			name: "supervisor negative lease",
			args: func(root string) []string {
				return []string{"runtime", "worker-orchestration-supervisor", root, "--max-runs", "1", "--lease-seconds", "-1"}
			},
			wantErr: "--lease-seconds must be positive",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "workspace")
			registerCLIWorkerOrchestrationWorkers(t, root)

			var out bytes.Buffer
			var stderr bytes.Buffer
			cmd := newRootCommand(&out, &stderr)
			cmd.SetArgs(tt.args(root))
			err := cmd.Execute()
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("expected %q validation error before artifact writes, got %v stdout=%q stderr=%q", tt.wantErr, err, out.String(), stderr.String())
			}
			orchestrations, listErr := app.ListRuntimeWorkerOrchestrations(root)
			if listErr != nil {
				t.Fatalf("list worker orchestrations: %v", listErr)
			}
			if len(orchestrations) != 0 {
				t.Fatalf("invalid orchestration bounds must not write orchestrations, got %#v", orchestrations)
			}
			supervisors, listErr := app.ListRuntimeWorkerOrchestrationSupervisors(root)
			if listErr != nil {
				t.Fatalf("list worker orchestration supervisors: %v", listErr)
			}
			if len(supervisors) != 0 {
				t.Fatalf("invalid orchestration bounds must not write supervisors, got %#v", supervisors)
			}
		})
	}
}

func TestRuntimeWorkerQueueClaimCommandWritesAndListsClaims(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := app.InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	if _, err := app.RegisterRuntimeWorker(root, app.RuntimeWorkerArtifact{
		WorkerID:     "remote-web-1",
		Mode:         "remote",
		Status:       "available",
		Capabilities: []string{"web.discovery"},
	}, app.RuntimeWorkerRegistrationOptions{Now: time.Now().UTC()}); err != nil {
		t.Fatalf("register runtime worker: %v", err)
	}

	var out bytes.Buffer
	cmd := newRootCommand(&out, new(bytes.Buffer))
	cmd.SetArgs([]string{
		"runtime", "worker-queue-claim", root, "remote-web-1",
		"--run-id", "run-queue-1",
		"--execution-id", "tool-web",
		"--tool", "web-api-pack",
		"--capability", "web.discovery",
		"--lease-seconds", "600",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("create worker queue claim: %v", err)
	}
	output := out.String()
	for _, want := range []string{
		"worker-queue-claim:",
		"worker=remote-web-1",
		"status=claimed",
		"mode=metadata-only",
		"run=run-queue-1",
		"execution=tool-web",
		"capability=web.discovery",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected worker-queue-claim output to contain %q, got %q", want, output)
		}
	}

	claims, err := app.ListRuntimeWorkerQueueClaims(root)
	if err != nil {
		t.Fatalf("list queue claims: %v", err)
	}
	if len(claims) != 1 {
		t.Fatalf("expected one queue claim artifact, got %#v", claims)
	}

	var listOut bytes.Buffer
	listCmd := newRootCommand(&listOut, new(bytes.Buffer))
	listCmd.SetArgs([]string{"runtime", "worker-queue-claims", root})
	if err := listCmd.Execute(); err != nil {
		t.Fatalf("list worker queue claims: %v", err)
	}
	if !strings.Contains(listOut.String(), "worker-queue-claims: total=1") || !strings.Contains(listOut.String(), "worker=remote-web-1") {
		t.Fatalf("expected queue claim listing, got %q", listOut.String())
	}

	var inspectOut bytes.Buffer
	inspectCmd := newRootCommand(&inspectOut, new(bytes.Buffer))
	inspectCmd.SetArgs([]string{"runtime", "worker-queue-claims", root, filepath.Base(claims[0].Path)})
	if err := inspectCmd.Execute(); err != nil {
		t.Fatalf("inspect worker queue claim: %v", err)
	}
	if !strings.Contains(inspectOut.String(), "worker-queue-claim:") || !strings.Contains(inspectOut.String(), "worker=remote-web-1") {
		t.Fatalf("expected inspected queue claim output, got %q", inspectOut.String())
	}
}

func TestRuntimeWorkerQueueClaimCommandPersistsToolPlanPayload(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := app.InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	if _, err := app.RegisterRuntimeWorker(root, app.RuntimeWorkerArtifact{
		WorkerID:     "remote-web-1",
		Mode:         "remote",
		Status:       "available",
		Capabilities: []string{"web.discovery"},
	}, app.RuntimeWorkerRegistrationOptions{Now: time.Now().UTC()}); err != nil {
		t.Fatalf("register runtime worker: %v", err)
	}

	var out bytes.Buffer
	cmd := newRootCommand(&out, new(bytes.Buffer))
	cmd.SetArgs([]string{
		"runtime", "worker-queue-claim", root, "remote-web-1",
		"--run-id", "run-queue-plan",
		"--execution-id", "tool-web",
		"--tool", "web-api-pack",
		"--capability", "web.discovery",
		"--action", "discover_assets",
		"--runner", "command",
		"--parser", "nuclei-json",
		"--run-timeout-ms", "2500",
		"--run-command", "nuclei",
		"--run-command", "-jsonl",
		"--input-node", "target:https://example.test",
		"--expected-evidence", "surface",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("create worker queue claim with tool plan: %v", err)
	}
	for _, want := range []string{
		"action=discover_assets",
		"runner=command",
		"parser=nuclei-json",
		"run_command=nuclei -jsonl",
	} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("expected tool-plan claim output to contain %q, got %q", want, out.String())
		}
	}
	claims, err := app.ListRuntimeWorkerQueueClaims(root)
	if err != nil {
		t.Fatalf("list queue claims: %v", err)
	}
	if len(claims) != 1 || claims[0].ToolPlan.Action != "discover_assets" || len(claims[0].ToolPlan.RunCommand) != 2 {
		t.Fatalf("expected persisted tool-plan payload on claim, got %#v", claims)
	}
}

func TestRuntimeWorkerQueueClaimCommandRejectsInvalidBoundsBeforeClaimWrite(t *testing.T) {
	tests := []struct {
		name    string
		extra   []string
		wantErr string
	}{
		{
			name:    "negative lease",
			extra:   []string{"--lease-seconds", "-1"},
			wantErr: "--lease-seconds must be positive",
		},
		{
			name:    "zero lease",
			extra:   []string{"--lease-seconds", "0"},
			wantErr: "--lease-seconds must be positive",
		},
		{
			name:    "negative run timeout",
			extra:   []string{"--run-timeout-ms", "-1"},
			wantErr: "--run-timeout-ms cannot be negative",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "workspace")
			if err := app.InitWorkspace(root); err != nil {
				t.Fatalf("init workspace: %v", err)
			}
			if _, err := app.RegisterRuntimeWorker(root, app.RuntimeWorkerArtifact{
				WorkerID:     "remote-web-1",
				Mode:         "remote",
				Status:       "available",
				Capabilities: []string{"web.discovery"},
			}, app.RuntimeWorkerRegistrationOptions{Now: time.Now().UTC()}); err != nil {
				t.Fatalf("register runtime worker: %v", err)
			}

			args := []string{
				"runtime", "worker-queue-claim", root, "remote-web-1",
				"--run-id", "run-invalid-claim",
				"--execution-id", "tool-invalid-claim",
				"--tool", "web-api-pack",
				"--capability", "web.discovery",
			}
			args = append(args, tt.extra...)
			var out bytes.Buffer
			var stderr bytes.Buffer
			cmd := newRootCommand(&out, &stderr)
			cmd.SetArgs(args)
			err := cmd.Execute()
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("expected %q validation error before claim write, got %v stdout=%q stderr=%q", tt.wantErr, err, out.String(), stderr.String())
			}
			claims, listErr := app.ListRuntimeWorkerQueueClaims(root)
			if listErr != nil {
				t.Fatalf("list queue claims: %v", listErr)
			}
			if len(claims) != 0 {
				t.Fatalf("invalid queue claim bounds must not write claims, got %#v", claims)
			}
		})
	}
}

func TestRuntimeExecuteWorkerQueueClaimCommandRequiresExecuteFlag(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := app.InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	if _, err := app.RegisterRuntimeWorker(root, app.RuntimeWorkerArtifact{
		WorkerID:     "remote-web-1",
		Mode:         "remote",
		Status:       "available",
		Capabilities: []string{"web.discovery"},
	}, app.RuntimeWorkerRegistrationOptions{Now: time.Now().UTC()}); err != nil {
		t.Fatalf("register runtime worker: %v", err)
	}
	claim, err := app.CreateRuntimeWorkerQueueClaim(root, app.RuntimeWorkerQueueClaim{
		WorkerID:    "remote-web-1",
		RunID:       "run-dispatch-1",
		ExecutionID: "tool-web",
		Tool:        "web-api-pack",
		Capability:  "web.discovery",
	}, app.RuntimeWorkerQueueClaimOptions{Now: time.Now().UTC(), LeaseDuration: 10 * time.Minute})
	if err != nil {
		t.Fatalf("create worker queue claim: %v", err)
	}

	var out bytes.Buffer
	cmd := newRootCommand(&out, new(bytes.Buffer))
	cmd.SetArgs([]string{"runtime", "execute-worker-queue-claim", root, filepath.Base(claim.Path)})
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "requires explicit execute") {
		t.Fatalf("expected explicit execute error, got %v output=%q", err, out.String())
	}
	executions, err := app.ListRuntimeWorkerQueueDispatchExecutions(root)
	if err != nil {
		t.Fatalf("list worker queue dispatch executions: %v", err)
	}
	if len(executions) != 0 {
		t.Fatalf("command without execute flag must not write audit artifacts, got %#v", executions)
	}
}

func TestRuntimeWorkerQueueClaimCancelCommandRequiresCancelFlag(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := app.InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	if _, err := app.RegisterRuntimeWorker(root, app.RuntimeWorkerArtifact{
		WorkerID:     "remote-web-1",
		Mode:         "remote",
		Status:       "available",
		Capabilities: []string{"web.discovery"},
	}, app.RuntimeWorkerRegistrationOptions{Now: time.Now().UTC()}); err != nil {
		t.Fatalf("register runtime worker: %v", err)
	}
	claim, err := app.CreateRuntimeWorkerQueueClaim(root, app.RuntimeWorkerQueueClaim{
		WorkerID:    "remote-web-1",
		RunID:       "run-cancel-cli-1",
		ExecutionID: "tool-web",
		Tool:        "web-api-pack",
		Capability:  "web.discovery",
	}, app.RuntimeWorkerQueueClaimOptions{Now: time.Now().UTC(), LeaseDuration: 10 * time.Minute})
	if err != nil {
		t.Fatalf("create worker queue claim: %v", err)
	}

	var out bytes.Buffer
	cmd := newRootCommand(&out, new(bytes.Buffer))
	cmd.SetArgs([]string{"runtime", "worker-queue-claim-cancel", root, filepath.Base(claim.Path)})
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "requires explicit cancel") {
		t.Fatalf("expected explicit cancel error, got %v output=%q", err, out.String())
	}
	read, err := app.ReadRuntimeWorkerQueueClaim(root, claim.Path)
	if err != nil {
		t.Fatalf("read queue claim: %v", err)
	}
	if read.Status != "claimed" || read.CanceledAt != "" {
		t.Fatalf("command without cancel flag must not mutate claim, got %#v", read)
	}
}

func TestRuntimeWorkerQueueClaimCancelCommandCancelsClaim(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := app.InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	if _, err := app.RegisterRuntimeWorker(root, app.RuntimeWorkerArtifact{
		WorkerID:     "remote-web-1",
		Mode:         "remote",
		Status:       "available",
		Capabilities: []string{"web.discovery"},
	}, app.RuntimeWorkerRegistrationOptions{Now: time.Now().UTC()}); err != nil {
		t.Fatalf("register runtime worker: %v", err)
	}
	claim, err := app.CreateRuntimeWorkerQueueClaim(root, app.RuntimeWorkerQueueClaim{
		WorkerID:    "remote-web-1",
		RunID:       "run-cancel-cli-2",
		ExecutionID: "tool-web",
		Tool:        "web-api-pack",
		Capability:  "web.discovery",
	}, app.RuntimeWorkerQueueClaimOptions{Now: time.Now().UTC(), LeaseDuration: 10 * time.Minute})
	if err != nil {
		t.Fatalf("create worker queue claim: %v", err)
	}

	var out bytes.Buffer
	cmd := newRootCommand(&out, new(bytes.Buffer))
	cmd.SetArgs([]string{
		"runtime", "worker-queue-claim-cancel", root, filepath.Base(claim.Path),
		"--cancel",
		"--canceled-by", "operator",
		"--reason", "release stale assignment",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("cancel worker queue claim: %v", err)
	}
	output := out.String()
	for _, want := range []string{
		"worker-queue-claim:",
		"status=canceled",
		"canceled_by=operator",
		"cancel_reason=release stale assignment",
		"worker=remote-web-1",
		"execution=tool-web",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected cancel output to contain %q, got %q", want, output)
		}
	}
	read, err := app.ReadRuntimeWorkerQueueClaim(root, claim.Path)
	if err != nil {
		t.Fatalf("read queue claim: %v", err)
	}
	if read.Status != "canceled" || read.CanceledBy != "operator" || read.CancelReason != "release stale assignment" {
		t.Fatalf("expected canceled claim artifact, got %#v", read)
	}
	executions, err := app.ListRuntimeWorkerQueueDispatchExecutions(root)
	if err != nil {
		t.Fatalf("list worker queue dispatch executions: %v", err)
	}
	if len(executions) != 0 {
		t.Fatalf("cancel command must not write dispatch executions, got %#v", executions)
	}
}

func TestRuntimeWorkerQueueClaimExpireCommandRequiresExpireFlag(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := app.InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	now := time.Now().UTC()
	if _, err := app.RegisterRuntimeWorker(root, app.RuntimeWorkerArtifact{
		WorkerID:     "remote-web-1",
		Mode:         "remote",
		Status:       "available",
		Capabilities: []string{"web.discovery"},
	}, app.RuntimeWorkerRegistrationOptions{Now: now}); err != nil {
		t.Fatalf("register runtime worker: %v", err)
	}
	claim, err := app.CreateRuntimeWorkerQueueClaim(root, app.RuntimeWorkerQueueClaim{
		WorkerID:    "remote-web-1",
		RunID:       "run-expire-cli-1",
		ExecutionID: "tool-web",
		Tool:        "web-api-pack",
		Capability:  "web.discovery",
	}, app.RuntimeWorkerQueueClaimOptions{Now: now.Add(-2 * time.Minute), LeaseDuration: time.Minute})
	if err != nil {
		t.Fatalf("create worker queue claim: %v", err)
	}

	var out bytes.Buffer
	cmd := newRootCommand(&out, new(bytes.Buffer))
	cmd.SetArgs([]string{"runtime", "worker-queue-claim-expire", root, filepath.Base(claim.Path)})
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "requires explicit expire") {
		t.Fatalf("expected explicit expire error, got %v output=%q", err, out.String())
	}
	read, err := app.ReadRuntimeWorkerQueueClaim(root, claim.Path)
	if err != nil {
		t.Fatalf("read queue claim: %v", err)
	}
	if read.Status != "claimed" || read.ExpiredAt != "" {
		t.Fatalf("command without expire flag must not mutate claim, got %#v", read)
	}
}

func TestRuntimeWorkerQueueClaimExpireCommandExpiresClaim(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := app.InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	now := time.Now().UTC()
	if _, err := app.RegisterRuntimeWorker(root, app.RuntimeWorkerArtifact{
		WorkerID:     "remote-web-1",
		Mode:         "remote",
		Status:       "available",
		Capabilities: []string{"web.discovery"},
	}, app.RuntimeWorkerRegistrationOptions{Now: now}); err != nil {
		t.Fatalf("register runtime worker: %v", err)
	}
	claim, err := app.CreateRuntimeWorkerQueueClaim(root, app.RuntimeWorkerQueueClaim{
		WorkerID:    "remote-web-1",
		RunID:       "run-expire-cli-2",
		ExecutionID: "tool-web",
		Tool:        "web-api-pack",
		Capability:  "web.discovery",
	}, app.RuntimeWorkerQueueClaimOptions{Now: now.Add(-2 * time.Minute), LeaseDuration: time.Minute})
	if err != nil {
		t.Fatalf("create worker queue claim: %v", err)
	}

	var out bytes.Buffer
	cmd := newRootCommand(&out, new(bytes.Buffer))
	cmd.SetArgs([]string{
		"runtime", "worker-queue-claim-expire", root, filepath.Base(claim.Path),
		"--expire",
		"--expired-by", "operator",
		"--reason", "lease elapsed before dispatch",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("expire worker queue claim: %v", err)
	}
	output := out.String()
	for _, want := range []string{
		"worker-queue-claim:",
		"status=expired",
		"expired_by=operator",
		"expire_reason=lease elapsed before dispatch",
		"worker=remote-web-1",
		"execution=tool-web",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected expire output to contain %q, got %q", want, output)
		}
	}
	read, err := app.ReadRuntimeWorkerQueueClaim(root, claim.Path)
	if err != nil {
		t.Fatalf("read queue claim: %v", err)
	}
	if read.Status != "expired" || read.ExpiredBy != "operator" || read.ExpireReason != "lease elapsed before dispatch" {
		t.Fatalf("expected expired claim artifact, got %#v", read)
	}
	executions, err := app.ListRuntimeWorkerQueueDispatchExecutions(root)
	if err != nil {
		t.Fatalf("list worker queue dispatch executions: %v", err)
	}
	if len(executions) != 0 {
		t.Fatalf("expire command must not write dispatch executions, got %#v", executions)
	}
}

func TestRuntimeWorkerQueueClaimRenewCommandRequiresRenewFlag(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := app.InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	now := time.Now().UTC()
	if _, err := app.RegisterRuntimeWorker(root, app.RuntimeWorkerArtifact{
		WorkerID:     "remote-web-1",
		Mode:         "remote",
		Status:       "available",
		Capabilities: []string{"web.discovery"},
	}, app.RuntimeWorkerRegistrationOptions{Now: now}); err != nil {
		t.Fatalf("register runtime worker: %v", err)
	}
	claim, err := app.CreateRuntimeWorkerQueueClaim(root, app.RuntimeWorkerQueueClaim{
		WorkerID:    "remote-web-1",
		RunID:       "run-renew-cli-1",
		ExecutionID: "tool-web",
		Tool:        "web-api-pack",
		Capability:  "web.discovery",
	}, app.RuntimeWorkerQueueClaimOptions{Now: now, LeaseDuration: 5 * time.Minute})
	if err != nil {
		t.Fatalf("create worker queue claim: %v", err)
	}

	var out bytes.Buffer
	cmd := newRootCommand(&out, new(bytes.Buffer))
	cmd.SetArgs([]string{"runtime", "worker-queue-claim-renew", root, filepath.Base(claim.Path)})
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "requires explicit renew") {
		t.Fatalf("expected explicit renew error, got %v output=%q", err, out.String())
	}
	read, err := app.ReadRuntimeWorkerQueueClaim(root, claim.Path)
	if err != nil {
		t.Fatalf("read queue claim: %v", err)
	}
	if read.Status != "claimed" || read.RenewedAt != "" {
		t.Fatalf("command without renew flag must not mutate claim, got %#v", read)
	}
}

func TestRuntimeWorkerQueueClaimRenewCommandRenewsClaim(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := app.InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	now := time.Now().UTC()
	if _, err := app.RegisterRuntimeWorker(root, app.RuntimeWorkerArtifact{
		WorkerID:     "remote-web-1",
		Mode:         "remote",
		Status:       "available",
		Capabilities: []string{"web.discovery"},
	}, app.RuntimeWorkerRegistrationOptions{Now: now}); err != nil {
		t.Fatalf("register runtime worker: %v", err)
	}
	claim, err := app.CreateRuntimeWorkerQueueClaim(root, app.RuntimeWorkerQueueClaim{
		WorkerID:    "remote-web-1",
		RunID:       "run-renew-cli-2",
		ExecutionID: "tool-web",
		Tool:        "web-api-pack",
		Capability:  "web.discovery",
	}, app.RuntimeWorkerQueueClaimOptions{Now: now, LeaseDuration: 5 * time.Minute})
	if err != nil {
		t.Fatalf("create worker queue claim: %v", err)
	}

	var out bytes.Buffer
	cmd := newRootCommand(&out, new(bytes.Buffer))
	cmd.SetArgs([]string{
		"runtime", "worker-queue-claim-renew", root, filepath.Base(claim.Path),
		"--renew",
		"--lease-seconds", "900",
		"--renewed-by", "operator",
		"--reason", "extend active assignment",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("renew worker queue claim: %v", err)
	}
	output := out.String()
	for _, want := range []string{
		"worker-queue-claim:",
		"status=claimed",
		"renewed_at=",
		"renewed_by=operator",
		"renew_reason=extend active assignment",
		"lease_seconds=900",
		"worker=remote-web-1",
		"execution=tool-web",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected renew output to contain %q, got %q", want, output)
		}
	}
	read, err := app.ReadRuntimeWorkerQueueClaim(root, claim.Path)
	if err != nil {
		t.Fatalf("read queue claim: %v", err)
	}
	if read.Status != "claimed" || read.RenewedBy != "operator" || read.RenewReason != "extend active assignment" || read.LeaseSeconds != 900 {
		t.Fatalf("expected renewed claim artifact, got %#v", read)
	}
	if read.RenewedAt == "" {
		t.Fatalf("expected renewed_at metadata on claim, got %#v", read)
	}
	executions, err := app.ListRuntimeWorkerQueueDispatchExecutions(root)
	if err != nil {
		t.Fatalf("list worker queue dispatch executions: %v", err)
	}
	if len(executions) != 0 {
		t.Fatalf("renew command must not write dispatch executions, got %#v", executions)
	}
}

func TestRuntimeExecuteWorkerQueueClaimCommandRunsWithExecuteFlag(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := app.InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	if _, err := app.RegisterRuntimeWorker(root, app.RuntimeWorkerArtifact{
		WorkerID:     "remote-web-1",
		Mode:         "remote",
		Status:       "available",
		Endpoint:     "tcp://127.0.0.1:9443",
		Capabilities: []string{"web.discovery"},
	}, app.RuntimeWorkerRegistrationOptions{Now: time.Now().UTC()}); err != nil {
		t.Fatalf("register runtime worker: %v", err)
	}
	claim, err := app.CreateRuntimeWorkerQueueClaim(root, app.RuntimeWorkerQueueClaim{
		WorkerID:    "remote-web-1",
		RunID:       "run-dispatch-1",
		ExecutionID: "tool-web",
		Tool:        "web-api-pack",
		Capability:  "web.discovery",
	}, app.RuntimeWorkerQueueClaimOptions{Now: time.Now().UTC(), LeaseDuration: 10 * time.Minute})
	if err != nil {
		t.Fatalf("create worker queue claim: %v", err)
	}

	var out bytes.Buffer
	cmd := newRootCommand(&out, new(bytes.Buffer))
	cmd.SetArgs([]string{"runtime", "execute-worker-queue-claim", root, filepath.Base(claim.Path), "--execute"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute worker queue claim: %v", err)
	}
	output := out.String()
	for _, want := range []string{
		"worker-queue-dispatch-execution:",
		"status=dispatched",
		"mode=execute",
		"transport=metadata-only",
		"worker=remote-web-1",
		"execution=tool-web",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected worker queue dispatch output to contain %q, got %q", want, output)
		}
	}

	executions, err := app.ListRuntimeWorkerQueueDispatchExecutions(root)
	if err != nil {
		t.Fatalf("list worker queue dispatch executions: %v", err)
	}
	if len(executions) != 1 || executions[0].Status != "dispatched" {
		t.Fatalf("expected one dispatch execution audit, got %#v", executions)
	}
	var listOut bytes.Buffer
	listCmd := newRootCommand(&listOut, new(bytes.Buffer))
	listCmd.SetArgs([]string{"runtime", "worker-queue-dispatch-executions", root})
	if err := listCmd.Execute(); err != nil {
		t.Fatalf("list worker queue dispatch executions: %v", err)
	}
	if !strings.Contains(listOut.String(), "worker-queue-dispatch-executions: total=1") || !strings.Contains(listOut.String(), "worker=remote-web-1") {
		t.Fatalf("expected queue dispatch execution listing, got %q", listOut.String())
	}
}

func TestRuntimeExecuteWorkerQueueClaimCommandWritesFileOutboxRequest(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := app.InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	if _, err := app.RegisterRuntimeWorker(root, app.RuntimeWorkerArtifact{
		WorkerID:     "remote-web-1",
		Mode:         "remote",
		Status:       "available",
		Endpoint:     "file://remote-web-1/inbox",
		Capabilities: []string{"web.discovery"},
	}, app.RuntimeWorkerRegistrationOptions{Now: time.Now().UTC()}); err != nil {
		t.Fatalf("register runtime worker: %v", err)
	}
	claim, err := app.CreateRuntimeWorkerQueueClaim(root, app.RuntimeWorkerQueueClaim{
		WorkerID:    "remote-web-1",
		RunID:       "run-dispatch-file",
		ExecutionID: "tool-web",
		Tool:        "web-api-pack",
		Capability:  "web.discovery",
	}, app.RuntimeWorkerQueueClaimOptions{Now: time.Now().UTC(), LeaseDuration: 10 * time.Minute})
	if err != nil {
		t.Fatalf("create worker queue claim: %v", err)
	}

	var out bytes.Buffer
	cmd := newRootCommand(&out, new(bytes.Buffer))
	cmd.SetArgs([]string{"runtime", "execute-worker-queue-claim", root, filepath.Base(claim.Path), "--execute", "--transport", "file-outbox"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute worker queue claim with file outbox: %v", err)
	}
	output := out.String()
	for _, want := range []string{
		"transport=file-outbox",
		"transport_request=",
		"worker=remote-web-1",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected file-outbox output to contain %q, got %q", want, output)
		}
	}
	executions, err := app.ListRuntimeWorkerQueueDispatchExecutions(root)
	if err != nil {
		t.Fatalf("list queue dispatch executions: %v", err)
	}
	if len(executions) != 1 || executions[0].TransportRequestPath == "" {
		t.Fatalf("expected execution with transport request path, got %#v", executions)
	}
	if _, err := os.Stat(executions[0].TransportRequestPath); err != nil {
		t.Fatalf("expected transport request artifact: %v", err)
	}
}

func TestRuntimeExecuteWorkerQueueClaimCommandDispatchesHTTPJSON(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := app.InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	received := make(chan app.RuntimeWorkerTransportDispatchRequest, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		var request app.RuntimeWorkerTransportDispatchRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode http-json request: %v", err)
			return
		}
		received <- request
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte("accepted"))
	}))
	defer server.Close()
	if _, err := app.RegisterRuntimeWorker(root, app.RuntimeWorkerArtifact{
		WorkerID:     "remote-web-1",
		Mode:         "remote",
		Status:       "available",
		Endpoint:     server.URL,
		Capabilities: []string{"web.discovery"},
	}, app.RuntimeWorkerRegistrationOptions{Now: time.Now().UTC()}); err != nil {
		t.Fatalf("register runtime worker: %v", err)
	}
	claim, err := app.CreateRuntimeWorkerQueueClaim(root, app.RuntimeWorkerQueueClaim{
		WorkerID:    "remote-web-1",
		RunID:       "run-dispatch-http",
		ExecutionID: "tool-web",
		Tool:        "web-api-pack",
		Capability:  "web.discovery",
		ToolPlan: app.RuntimeWorkerToolPlanPayload{
			Action:     "discover_assets",
			Runner:     "command",
			RunCommand: []string{"nuclei", "-u", "{{target}}"},
		},
	}, app.RuntimeWorkerQueueClaimOptions{Now: time.Now().UTC(), LeaseDuration: 10 * time.Minute})
	if err != nil {
		t.Fatalf("create worker queue claim: %v", err)
	}

	var out bytes.Buffer
	cmd := newRootCommand(&out, new(bytes.Buffer))
	callbackEndpoint := "https://controller.example/ouro/runtime/result"
	cmd.SetArgs([]string{
		"runtime", "execute-worker-queue-claim", root, filepath.Base(claim.Path),
		"--execute",
		"--transport", "http-json",
		"--result-callback", callbackEndpoint,
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute worker queue claim with http-json: %v", err)
	}
	output := out.String()
	for _, want := range []string{
		"transport=http-json",
		"transport_endpoint=" + server.URL,
		"transport_http_status=202",
		"transport_response=accepted",
		"result_callback=" + callbackEndpoint,
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected http-json output to contain %q, got %q", want, output)
		}
	}
	select {
	case request := <-received:
		if request.TransportMode != "http-json" || request.WorkerID != "remote-web-1" || request.ToolPlan.Action != "discover_assets" || request.ResultCallbackEndpoint != callbackEndpoint {
			t.Fatalf("unexpected http-json request body: %#v", request)
		}
	case <-time.After(time.Second):
		t.Fatalf("expected http-json request body")
	}
}

func TestRuntimeExecuteWorkerQueueClaimCommandReadsHTTPJSONTokenFromEnv(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := app.InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	receivedAuth := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuth <- r.Header.Get("Authorization")
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte("accepted"))
	}))
	defer server.Close()
	if _, err := app.RegisterRuntimeWorker(root, app.RuntimeWorkerArtifact{
		WorkerID:     "remote-web-1",
		Mode:         "remote",
		Status:       "available",
		Endpoint:     server.URL,
		Capabilities: []string{"web.discovery"},
	}, app.RuntimeWorkerRegistrationOptions{Now: time.Now().UTC()}); err != nil {
		t.Fatalf("register runtime worker: %v", err)
	}
	claim, err := app.CreateRuntimeWorkerQueueClaim(root, app.RuntimeWorkerQueueClaim{
		WorkerID:    "remote-web-1",
		RunID:       "run-dispatch-http-auth",
		ExecutionID: "tool-web",
		Tool:        "web-api-pack",
		Capability:  "web.discovery",
	}, app.RuntimeWorkerQueueClaimOptions{Now: time.Now().UTC(), LeaseDuration: 10 * time.Minute})
	if err != nil {
		t.Fatalf("create worker queue claim: %v", err)
	}
	t.Setenv("OURO_WORKER_TOKEN", "secret-worker-token")

	var out bytes.Buffer
	cmd := newRootCommand(&out, new(bytes.Buffer))
	cmd.SetArgs([]string{
		"runtime", "execute-worker-queue-claim", root, filepath.Base(claim.Path),
		"--execute",
		"--transport", "http-json",
		"--transport-token-env", "OURO_WORKER_TOKEN",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute worker queue claim with env token: %v", err)
	}
	output := out.String()
	if !strings.Contains(output, "transport_auth=bearer") {
		t.Fatalf("expected bearer auth marker in output, got %q", output)
	}
	if strings.Contains(output, "secret-worker-token") {
		t.Fatalf("output must not contain worker auth token, got %q", output)
	}
	select {
	case auth := <-receivedAuth:
		if auth != "Bearer secret-worker-token" {
			t.Fatalf("expected bearer authorization header, got %q", auth)
		}
	case <-time.After(time.Second):
		t.Fatalf("expected http-json request")
	}
}

func TestRuntimeWorkerTransportOutboxCommandListsAndAcknowledgesRequests(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	dispatch := createCLIFileOutboxWorkerQueueDispatch(t, root)

	var listOut bytes.Buffer
	listCmd := newRootCommand(&listOut, new(bytes.Buffer))
	listCmd.SetArgs([]string{"runtime", "worker-transport-outbox", root, "remote-web-1"})
	if err := listCmd.Execute(); err != nil {
		t.Fatalf("list worker transport outbox: %v", err)
	}
	for _, want := range []string{
		"worker-transport-requests: total=1 pending=1 acknowledged=0 completed=0 failed=0",
		"worker-transport-request:",
		"status=pending",
		"transport=file-outbox",
		"worker=remote-web-1",
		"execution=tool-web",
	} {
		if !strings.Contains(listOut.String(), want) {
			t.Fatalf("expected outbox listing to contain %q, got %q", want, listOut.String())
		}
	}

	var rejectedOut bytes.Buffer
	rejectedCmd := newRootCommand(&rejectedOut, new(bytes.Buffer))
	rejectedCmd.SetArgs([]string{"runtime", "worker-transport-ack", root, "remote-web-1", filepath.Base(dispatch.TransportRequestPath)})
	if err := rejectedCmd.Execute(); err == nil || !strings.Contains(err.Error(), "requires explicit ack") {
		t.Fatalf("expected explicit ack error, got %v output=%q", err, rejectedOut.String())
	}

	var ackOut bytes.Buffer
	ackCmd := newRootCommand(&ackOut, new(bytes.Buffer))
	ackCmd.SetArgs([]string{"runtime", "worker-transport-ack", root, "remote-web-1", filepath.Base(dispatch.TransportRequestPath), "--ack"})
	if err := ackCmd.Execute(); err != nil {
		t.Fatalf("ack worker transport request: %v", err)
	}
	for _, want := range []string{
		"worker-transport-request:",
		"status=acknowledged",
		"acked_by=remote-web-1",
	} {
		if !strings.Contains(ackOut.String(), want) {
			t.Fatalf("expected ack output to contain %q, got %q", want, ackOut.String())
		}
	}

	var inspectOut bytes.Buffer
	inspectCmd := newRootCommand(&inspectOut, new(bytes.Buffer))
	inspectCmd.SetArgs([]string{"runtime", "worker-transport-outbox", root, "remote-web-1", filepath.Base(dispatch.TransportRequestPath)})
	if err := inspectCmd.Execute(); err != nil {
		t.Fatalf("inspect worker transport request: %v", err)
	}
	if !strings.Contains(inspectOut.String(), "status=acknowledged") || !strings.Contains(inspectOut.String(), "dispatch="+dispatch.Path) {
		t.Fatalf("expected acknowledged request inspection, got %q", inspectOut.String())
	}
}

func TestRuntimeWorkerTransportServeHTTPCommandRequiresExecuteFlag(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := app.InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}

	var out bytes.Buffer
	cmd := newRootCommand(&out, new(bytes.Buffer))
	cmd.SetArgs([]string{"runtime", "worker-transport-serve-http", root, "remote-web-1", "--listen", "127.0.0.1:0"})
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "requires explicit execute") {
		t.Fatalf("expected explicit execute error, got %v output=%q", err, out.String())
	}
}

func TestRuntimeWorkerTransportServeHTTPCommandRejectsNegativeMaxRequestsBeforeListen(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := app.InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}

	var out bytes.Buffer
	var stderr bytes.Buffer
	cmd := newRootCommand(&out, &stderr)
	cmd.SetArgs([]string{
		"runtime", "worker-transport-serve-http", root, "remote-web-1",
		"--execute",
		"--listen", "not-a-tcp-address",
		"--max-requests", "-1",
		"--auth-token-env", "OURO_MISSING_WORKER_RECEIVER_TOKEN",
	})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "--max-requests cannot be negative") {
		t.Fatalf("expected max-requests validation error before listen, got %v stdout=%q stderr=%q", err, out.String(), stderr.String())
	}
	if strings.Contains(out.String(), "worker-transport-serve-http:") {
		t.Fatalf("negative max-requests must not start receiver, got stdout=%q", out.String())
	}
}

func TestRuntimeWorkerTransportServeHTTPCommandPersistsOneRequest(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := app.InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("allocate listen address: %v", err)
	}
	addr := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatalf("close temporary listener: %v", err)
	}

	var out bytes.Buffer
	cmd := newRootCommand(&out, new(bytes.Buffer))
	cmd.SetArgs([]string{
		"runtime", "worker-transport-serve-http", root, "remote-web-1",
		"--execute",
		"--listen", addr,
		"--max-requests", "1",
	})
	done := make(chan error, 1)
	go func() {
		done <- cmd.Execute()
	}()

	body := app.RuntimeWorkerTransportDispatchRequest{
		Version:               1,
		Status:                "pending",
		TransportMode:         "http-json",
		DispatchExecutionPath: filepath.Join(root, "runtime", "worker-queue-dispatch-executions", "dispatch.json"),
		ClaimPath:             filepath.Join(root, "runtime", "worker-queue-claims", "claim.json"),
		WorkerID:              "remote-web-1",
		RunID:                 "run-http",
		ExecutionID:           "tool-web",
		Tool:                  "web-api-pack",
		Capability:            "web.discovery",
		ToolPlan: app.RuntimeWorkerToolPlanPayload{
			Action:     "discover_assets",
			Runner:     "command",
			RunCommand: []string{"nuclei", "-u", "{{target}}"},
		},
		CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}
	data, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	client := http.Client{Timeout: 100 * time.Millisecond}
	var lastErr error
	for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		response, err := client.Post("http://"+addr+"/dispatch", "application/json", bytes.NewReader(data))
		if err != nil {
			lastErr = err
			continue
		}
		_ = response.Body.Close()
		if response.StatusCode == http.StatusAccepted {
			lastErr = nil
			break
		}
		lastErr = fmt.Errorf("unexpected status %d", response.StatusCode)
	}
	if lastErr != nil {
		t.Fatalf("post worker transport request: %v", lastErr)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("serve http worker transport: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("serve http command did not exit after max request")
	}
	requests, err := app.ListRuntimeWorkerTransportDispatchRequests(root, "remote-web-1")
	if err != nil {
		t.Fatalf("list worker transport requests: %v", err)
	}
	if len(requests) != 1 || requests[0].Status != "pending" || requests[0].TransportMode != "file-outbox" {
		t.Fatalf("expected one persisted pending request, got %#v", requests)
	}
	for _, want := range []string{
		"worker-transport-serve-http: worker=remote-web-1 listen=" + addr,
		"worker-transport-serve-http: accepted=1",
	} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("expected serve output to contain %q, got %q", want, out.String())
		}
	}
}

func TestRuntimeWorkerTransportServeHTTPCommandRequiresCurrentSafetyAuthorizationBeforePersistingRequest(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := app.InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	writeRuntimeWorkerHTTPDisabledSafetyScope(t, root)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("allocate listen address: %v", err)
	}
	addr := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatalf("close temporary listener: %v", err)
	}

	var out bytes.Buffer
	cmd := newRootCommand(&out, new(bytes.Buffer))
	cmd.SetArgs([]string{
		"runtime", "worker-transport-serve-http", root, "remote-web-1",
		"--execute",
		"--listen", addr,
		"--max-requests", "1",
	})
	done := make(chan error, 1)
	go func() {
		done <- cmd.Execute()
	}()

	body := app.RuntimeWorkerTransportDispatchRequest{
		Version:               1,
		Status:                "pending",
		TransportMode:         "http-json",
		DispatchExecutionPath: filepath.Join(root, "runtime", "worker-queue-dispatch-executions", "dispatch.json"),
		ClaimPath:             filepath.Join(root, "runtime", "worker-queue-claims", "claim.json"),
		WorkerID:              "remote-web-1",
		RunID:                 "run-http",
		ExecutionID:           "tool-web",
		Tool:                  "web-api-pack",
		Capability:            "web.discovery",
		CreatedAt:             time.Now().UTC().Format(time.RFC3339Nano),
	}
	data, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	client := http.Client{Timeout: 100 * time.Millisecond}
	var blockedBody string
	var lastErr error
	for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		response, err := client.Post("http://"+addr+"/dispatch", "application/json", bytes.NewReader(data))
		if err != nil {
			lastErr = err
			continue
		}
		blockedData, _ := io.ReadAll(response.Body)
		_ = response.Body.Close()
		blockedBody = string(blockedData)
		if response.StatusCode == http.StatusBadRequest && strings.Contains(blockedBody, "worker runtime side effect requires current safety bypass authorization") {
			lastErr = nil
			break
		}
		if response.StatusCode >= 200 && response.StatusCode <= 299 {
			t.Fatalf("blocked worker transport request must not be accepted before safety authorization, got status=%d body=%s", response.StatusCode, blockedBody)
		}
		lastErr = fmt.Errorf("unexpected status %d body=%s", response.StatusCode, blockedBody)
	}
	if lastErr != nil {
		t.Fatalf("post blocked worker transport request: %v", lastErr)
	}
	requests, err := app.ListRuntimeWorkerTransportDispatchRequests(root, "remote-web-1")
	if err != nil {
		t.Fatalf("list worker transport requests after blocked receive: %v", err)
	}
	if len(requests) != 0 {
		t.Fatalf("blocked http receive must not persist requests, got %#v", requests)
	}

	writeRuntimeWorkerHTTPCurrentSafetyAuthorization(t, root)
	var accepted bool
	for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		response, err := client.Post("http://"+addr+"/dispatch", "application/json", bytes.NewReader(data))
		if err != nil {
			lastErr = err
			continue
		}
		_ = response.Body.Close()
		if response.StatusCode == http.StatusAccepted {
			accepted = true
			lastErr = nil
			break
		}
		lastErr = fmt.Errorf("unexpected status %d after authorization", response.StatusCode)
	}
	if !accepted {
		t.Fatalf("post authorized worker transport request: %v", lastErr)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("serve http worker transport: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("serve http command did not exit after authorized max request")
	}
	requests, err = app.ListRuntimeWorkerTransportDispatchRequests(root, "remote-web-1")
	if err != nil {
		t.Fatalf("list worker transport requests after authorized receive: %v", err)
	}
	if len(requests) != 1 || requests[0].Status != "pending" || requests[0].TransportMode != "file-outbox" {
		t.Fatalf("expected one authorized pending request, got %#v", requests)
	}
	if !strings.Contains(out.String(), "worker-transport-serve-http: accepted=1") {
		t.Fatalf("expected command to count only authorized accepted request, got output=%q blocked=%q", out.String(), blockedBody)
	}
}

func writeRuntimeWorkerHTTPDisabledSafetyScope(t *testing.T, root string) {
	t.Helper()
	scope := `version: 1
scope:
  target_policy: disabled
  targets: []
  exclusions: []
execution:
  isolation: process
`
	if err := os.WriteFile(filepath.Join(root, "scope.yaml"), []byte(scope), 0o644); err != nil {
		t.Fatalf("write disabled safety scope: %v", err)
	}
}

func writeRuntimeWorkerHTTPCurrentSafetyAuthorization(t *testing.T, root string) {
	t.Helper()
	authorization := `version: 1
safety_bypasses:
  - control: scope.target_policy
    mode: disabled
    authorized_by: alice
    reason: authorized runtime worker target-boundary bypass
    ticket: ENG-4242
    expires: 2999-06-30
`
	if err := os.WriteFile(filepath.Join(root, "authorization.yaml"), []byte(authorization), 0o644); err != nil {
		t.Fatalf("write current safety authorization: %v", err)
	}
}

func TestRuntimeWorkerTransportServeHTTPCommandReadsAuthTokenFromEnv(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := app.InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("allocate listen address: %v", err)
	}
	addr := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatalf("close temporary listener: %v", err)
	}
	t.Setenv("OURO_WORKER_TOKEN", "secret-worker-token")

	var out bytes.Buffer
	cmd := newRootCommand(&out, new(bytes.Buffer))
	cmd.SetArgs([]string{
		"runtime", "worker-transport-serve-http", root, "remote-web-1",
		"--execute",
		"--listen", addr,
		"--max-requests", "1",
		"--auth-token-env", "OURO_WORKER_TOKEN",
	})
	done := make(chan error, 1)
	go func() {
		done <- cmd.Execute()
	}()

	body := app.RuntimeWorkerTransportDispatchRequest{
		Version:               1,
		Status:                "pending",
		TransportMode:         "http-json",
		DispatchExecutionPath: filepath.Join(root, "runtime", "worker-queue-dispatch-executions", "dispatch.json"),
		ClaimPath:             filepath.Join(root, "runtime", "worker-queue-claims", "claim.json"),
		WorkerID:              "remote-web-1",
		RunID:                 "run-http",
		ExecutionID:           "tool-web",
		Tool:                  "web-api-pack",
		Capability:            "web.discovery",
		CreatedAt:             time.Now().UTC().Format(time.RFC3339Nano),
	}
	data, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	client := http.Client{Timeout: 100 * time.Millisecond}
	var unauthorizedSeen bool
	var lastErr error
	for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		response, err := client.Post("http://"+addr+"/dispatch", "application/json", bytes.NewReader(data))
		if err != nil {
			lastErr = err
			continue
		}
		_ = response.Body.Close()
		if response.StatusCode == http.StatusUnauthorized {
			unauthorizedSeen = true
			lastErr = nil
			break
		}
		lastErr = fmt.Errorf("unexpected unauthenticated status %d", response.StatusCode)
	}
	if lastErr != nil {
		t.Fatalf("post unauthenticated worker transport request: %v", lastErr)
	}
	if !unauthorizedSeen {
		t.Fatalf("expected unauthenticated request to be rejected")
	}

	authorized, err := http.NewRequest(http.MethodPost, "http://"+addr+"/dispatch", bytes.NewReader(data))
	if err != nil {
		t.Fatalf("create authorized request: %v", err)
	}
	authorized.Header.Set("Content-Type", "application/json")
	authorized.Header.Set("Authorization", "Bearer secret-worker-token")
	response, err := client.Do(authorized)
	if err != nil {
		t.Fatalf("post authorized worker transport request: %v", err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusAccepted {
		t.Fatalf("expected authorized request accepted, got %d", response.StatusCode)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("serve http worker transport: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("serve http command did not exit after authorized max request")
	}
	requests, err := app.ListRuntimeWorkerTransportDispatchRequests(root, "remote-web-1")
	if err != nil {
		t.Fatalf("list worker transport requests: %v", err)
	}
	if len(requests) != 1 || requests[0].Status != "pending" {
		t.Fatalf("expected only authorized request persisted, got %#v", requests)
	}
	if strings.Contains(out.String(), "secret-worker-token") {
		t.Fatalf("serve output must not contain worker auth token, got %q", out.String())
	}
}

func TestRuntimeWorkerTransportRunOnceCommandExecutesAcknowledgedRequest(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	dispatch := createCLIFileOutboxWorkerQueueDispatch(t, root)
	if _, err := app.AcknowledgeRuntimeWorkerTransportDispatchRequest(root, "remote-web-1", filepath.Base(dispatch.TransportRequestPath), app.RuntimeWorkerTransportAckOptions{
		Ack: true,
		Now: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("ack transport request: %v", err)
	}

	var rejectedOut bytes.Buffer
	rejectedCmd := newRootCommand(&rejectedOut, new(bytes.Buffer))
	rejectedCmd.SetArgs([]string{
		"runtime", "worker-transport-run-once", root, "remote-web-1", filepath.Base(dispatch.TransportRequestPath),
		"--", "sh", "-c", "printf should-not-run",
	})
	if err := rejectedCmd.Execute(); err == nil || !strings.Contains(err.Error(), "requires explicit execute") {
		t.Fatalf("expected explicit execute error, got %v output=%q", err, rejectedOut.String())
	}

	var out bytes.Buffer
	cmd := newRootCommand(&out, new(bytes.Buffer))
	cmd.SetArgs([]string{
		"runtime", "worker-transport-run-once", root, "remote-web-1", filepath.Base(dispatch.TransportRequestPath),
		"--execute",
		"--", "sh", "-c", "printf 'worker=%s execution=%s' \"$OURO_WORKER_ID\" \"$OURO_EXECUTION_ID\"",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("run worker transport request: %v", err)
	}
	output := out.String()
	for _, want := range []string{
		"worker-queue-dispatch-result:",
		"status=completed",
		"worker=remote-web-1",
		"execution=tool-web",
		"output=worker=remote-web-1 execution=tool-web",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected run-once output to contain %q, got %q", want, output)
		}
	}
	request, err := app.ReadRuntimeWorkerTransportDispatchRequest(root, "remote-web-1", filepath.Base(dispatch.TransportRequestPath))
	if err != nil {
		t.Fatalf("read transport request: %v", err)
	}
	if request.Status != "completed" || request.ResultPath == "" {
		t.Fatalf("expected request closed by run-once command, got %#v", request)
	}
}

func TestRuntimeWorkerTransportRunOnceCommandUsesRequestToolPlanCommand(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	dispatch := createCLIFileOutboxWorkerQueueDispatchWithToolPlan(t, root, app.RuntimeWorkerToolPlanPayload{
		Action:     "discover_assets",
		Runner:     "command",
		RunCommand: []string{"sh", "-c", "printf 'plan=%s execution=%s' \"$OURO_ACTION\" \"$OURO_EXECUTION_ID\""},
	})
	if _, err := app.AcknowledgeRuntimeWorkerTransportDispatchRequest(root, "remote-web-1", filepath.Base(dispatch.TransportRequestPath), app.RuntimeWorkerTransportAckOptions{
		Ack: true,
		Now: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("ack transport request: %v", err)
	}

	var out bytes.Buffer
	cmd := newRootCommand(&out, new(bytes.Buffer))
	cmd.SetArgs([]string{
		"runtime", "worker-transport-run-once", root, "remote-web-1", filepath.Base(dispatch.TransportRequestPath),
		"--execute",
		"--use-request-command",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("run worker transport request from tool plan: %v", err)
	}
	output := out.String()
	for _, want := range []string{
		"worker-queue-dispatch-result:",
		"status=completed",
		"output=plan=discover_assets execution=tool-web",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected request-command output to contain %q, got %q", want, output)
		}
	}
}

func TestRuntimeWorkerTransportRunOnceCommandCanHeartbeatActiveLease(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	dispatch := createCLIFileOutboxWorkerQueueDispatchWithToolPlan(t, root, app.RuntimeWorkerToolPlanPayload{
		Action:     "discover_assets",
		Runner:     "command",
		RunCommand: []string{"sh", "-c", "printf 'heartbeat-cli=%s' \"$OURO_EXECUTION_ID\""},
	})
	if _, err := app.AcknowledgeRuntimeWorkerTransportDispatchRequest(root, "remote-web-1", filepath.Base(dispatch.TransportRequestPath), app.RuntimeWorkerTransportAckOptions{
		Ack: true,
		Now: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("ack transport request: %v", err)
	}
	oldHeartbeat := time.Now().UTC().Add(-app.DefaultWorkerLeaseStaleAfter - time.Minute).Format(time.RFC3339Nano)
	writeCLIWorkerLeaseFixture(t, root, app.WorkerLeaseArtifact{
		Version:     1,
		RunID:       dispatch.RunID,
		ExecutionID: dispatch.ExecutionID,
		Tool:        dispatch.Tool,
		Capability:  dispatch.Capability,
		WorkerID:    "remote-web-1",
		Status:      "active",
		AcquiredAt:  oldHeartbeat,
		HeartbeatAt: oldHeartbeat,
	})

	var out bytes.Buffer
	cmd := newRootCommand(&out, new(bytes.Buffer))
	cmd.SetArgs([]string{
		"runtime", "worker-transport-run-once", root, "remote-web-1", filepath.Base(dispatch.TransportRequestPath),
		"--execute",
		"--use-request-command",
		"--heartbeat-lease",
		"--heartbeat-by", "remote-web-1",
		"--heartbeat-reason", "worker transport run-once",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("run worker transport request from tool plan with heartbeat: %v", err)
	}
	if !strings.Contains(out.String(), "output=heartbeat-cli=tool-web") {
		t.Fatalf("expected request-command output, got %q", out.String())
	}
	leases, err := app.ListWorkerLeasesWithStaleness(root, time.Now().UTC(), app.DefaultWorkerLeaseStaleAfter)
	if err != nil {
		t.Fatalf("list worker leases with staleness: %v", err)
	}
	if len(leases) != 1 || leases[0].Status != "active" || leases[0].Stale || leases[0].HeartbeatAt == oldHeartbeat || leases[0].HeartbeatBy != "remote-web-1" || leases[0].HeartbeatReason != "worker transport run-once" {
		t.Fatalf("expected CLI run-once to refresh active worker lease heartbeat only, got %#v", leases)
	}
}

func TestRuntimeWorkerTransportRunLoopCommandProcessesBoundedAcknowledgedRequests(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	dispatch := createCLIFileOutboxWorkerQueueDispatch(t, root)
	if _, err := app.AcknowledgeRuntimeWorkerTransportDispatchRequest(root, "remote-web-1", filepath.Base(dispatch.TransportRequestPath), app.RuntimeWorkerTransportAckOptions{
		Ack: true,
		Now: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("ack transport request: %v", err)
	}

	var out bytes.Buffer
	cmd := newRootCommand(&out, new(bytes.Buffer))
	cmd.SetArgs([]string{
		"runtime", "worker-transport-run-loop", root, "remote-web-1",
		"--execute",
		"--max-requests", "1",
		"--", "sh", "-c", "printf 'loop=%s' \"$OURO_EXECUTION_ID\"",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("run worker transport loop: %v", err)
	}
	output := out.String()
	for _, want := range []string{
		"worker-transport-run-loop: worker=remote-web-1 status=completed processed=1 completed=1 failed=0 skipped=0",
		"worker-queue-dispatch-result:",
		"output=loop=tool-web",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected run-loop output to contain %q, got %q", want, output)
		}
	}
}

func TestRuntimeWorkerTransportRunLoopCommandRejectsInvalidBounds(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")

	var out bytes.Buffer
	var stderr bytes.Buffer
	cmd := newRootCommand(&out, &stderr)
	cmd.SetArgs([]string{
		"runtime", "worker-transport-run-loop", root, "remote-web-1",
		"--execute",
		"--use-request-command",
		"--max-requests", "0",
	})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "--max-requests must be positive") {
		t.Fatalf("expected max-requests validation error, got %v stdout=%q stderr=%q", err, out.String(), stderr.String())
	}
}

func TestRuntimeWorkerTransportRunLoopCommandUsesRequestToolPlanCommand(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	dispatch := createCLIFileOutboxWorkerQueueDispatchWithToolPlan(t, root, app.RuntimeWorkerToolPlanPayload{
		Action:     "discover_assets",
		Runner:     "command",
		RunCommand: []string{"sh", "-c", "printf 'loop-plan=%s' \"$OURO_EXECUTION_ID\""},
	})
	if _, err := app.AcknowledgeRuntimeWorkerTransportDispatchRequest(root, "remote-web-1", filepath.Base(dispatch.TransportRequestPath), app.RuntimeWorkerTransportAckOptions{
		Ack: true,
		Now: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("ack transport request: %v", err)
	}

	var out bytes.Buffer
	cmd := newRootCommand(&out, new(bytes.Buffer))
	cmd.SetArgs([]string{
		"runtime", "worker-transport-run-loop", root, "remote-web-1",
		"--execute",
		"--use-request-command",
		"--max-requests", "1",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("run worker transport loop from tool plan: %v", err)
	}
	output := out.String()
	for _, want := range []string{
		"worker-transport-run-loop: worker=remote-web-1 status=completed processed=1 completed=1 failed=0 skipped=0",
		"output=loop-plan=tool-web",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected request-command loop output to contain %q, got %q", want, output)
		}
	}
}

func TestRuntimeWorkerTransportRunDaemonCommandUsesRequestToolPlanCommand(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	dispatch := createCLIFileOutboxWorkerQueueDispatchWithToolPlan(t, root, app.RuntimeWorkerToolPlanPayload{
		Action:     "discover_assets",
		Runner:     "command",
		RunCommand: []string{"sh", "-c", "printf 'daemon-plan=%s' \"$OURO_EXECUTION_ID\""},
	})
	if _, err := app.AcknowledgeRuntimeWorkerTransportDispatchRequest(root, "remote-web-1", filepath.Base(dispatch.TransportRequestPath), app.RuntimeWorkerTransportAckOptions{
		Ack: true,
		Now: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("ack transport request: %v", err)
	}

	var out bytes.Buffer
	cmd := newRootCommand(&out, new(bytes.Buffer))
	cmd.SetArgs([]string{
		"runtime", "worker-transport-run-daemon", root, "remote-web-1",
		"--execute",
		"--use-request-command",
		"--max-requests", "1",
		"--idle-rounds", "2",
		"--poll-interval-ms", "1",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("run worker transport daemon from tool plan: %v", err)
	}
	output := out.String()
	for _, want := range []string{
		"worker-transport-run-daemon: worker=remote-web-1 status=completed processed=1 completed=1 failed=0 skipped=0",
		"scans=",
		"idle_rounds=",
		"artifact=",
		"output=daemon-plan=tool-web",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected request-command daemon output to contain %q, got %q", want, output)
		}
	}

	var listOut bytes.Buffer
	listCmd := newRootCommand(&listOut, new(bytes.Buffer))
	listCmd.SetArgs([]string{"runtime", "worker-transport-daemon-runs", root, "remote-web-1"})
	if err := listCmd.Execute(); err != nil {
		t.Fatalf("list worker transport daemon runs: %v", err)
	}
	for _, want := range []string{
		"worker-transport-daemon-runs: total=1",
		"worker-transport-daemon-run:",
		"status=completed",
		"processed=1",
		"source=request-command",
	} {
		if !strings.Contains(listOut.String(), want) {
			t.Fatalf("expected daemon run listing to contain %q, got %q", want, listOut.String())
		}
	}
}

func TestRuntimeWorkerTransportRunDaemonCommandRejectsInvalidBounds(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	cases := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "max requests",
			args: []string{"runtime", "worker-transport-run-daemon", root, "remote-web-1", "--execute", "--use-request-command", "--max-requests", "0"},
			want: "--max-requests must be positive",
		},
		{
			name: "idle rounds",
			args: []string{"runtime", "worker-transport-run-daemon", root, "remote-web-1", "--execute", "--use-request-command", "--idle-rounds", "0"},
			want: "--idle-rounds must be positive",
		},
		{
			name: "poll interval",
			args: []string{"runtime", "worker-transport-run-daemon", root, "remote-web-1", "--execute", "--use-request-command", "--poll-interval-ms", "0"},
			want: "--poll-interval-ms must be positive",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			var stderr bytes.Buffer
			cmd := newRootCommand(&out, &stderr)
			cmd.SetArgs(tc.args)
			err := cmd.Execute()
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected validation error %q, got %v stdout=%q stderr=%q", tc.want, err, out.String(), stderr.String())
			}
			runs, listErr := app.ListRuntimeWorkerTransportDaemonRuns(root, "remote-web-1")
			if listErr != nil {
				t.Fatalf("list worker transport daemon runs: %v", listErr)
			}
			if len(runs) != 0 {
				t.Fatalf("invalid daemon bounds must not write artifacts, got %#v", runs)
			}
		})
	}
}

func TestRuntimeWorkerTransportDaemonControlCommandWritesAndListsRequests(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := app.InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}

	var out bytes.Buffer
	cmd := newRootCommand(&out, new(bytes.Buffer))
	cmd.SetArgs([]string{
		"runtime", "worker-transport-daemon-control", root, "remote-web-1",
		"--action", "stop",
		"--reason", "operator requested maintenance",
		"--requested-by", "alice",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("write worker transport daemon control request: %v", err)
	}
	output := out.String()
	for _, want := range []string{
		"worker-transport-daemon-control:",
		"worker=remote-web-1",
		"status=pending",
		"mode=metadata-only",
		"action=stop",
		"reason=operator requested maintenance",
		"requested_by=alice",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected daemon control output to contain %q, got %q", want, output)
		}
	}

	var listOut bytes.Buffer
	listCmd := newRootCommand(&listOut, new(bytes.Buffer))
	listCmd.SetArgs([]string{"runtime", "worker-transport-daemon-controls", root, "remote-web-1"})
	if err := listCmd.Execute(); err != nil {
		t.Fatalf("list worker transport daemon control requests: %v", err)
	}
	for _, want := range []string{
		"worker-transport-daemon-controls: total=1",
		"worker-transport-daemon-control:",
		"action=stop",
		"status=pending",
	} {
		if !strings.Contains(listOut.String(), want) {
			t.Fatalf("expected daemon control listing to contain %q, got %q", want, listOut.String())
		}
	}
}

func TestRuntimeWorkerTransportDaemonControlCommandRequiresCurrentSafetyAuthorizationBeforeWriting(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := app.InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	writeRuntimeWorkerHTTPDisabledSafetyScope(t, root)

	var out bytes.Buffer
	var stderr bytes.Buffer
	cmd := newRootCommand(&out, &stderr)
	cmd.SetArgs([]string{
		"runtime", "worker-transport-daemon-control", root, "remote-web-1",
		"--action", "stop",
		"--reason", "operator requested maintenance",
		"--requested-by", "alice",
	})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "worker runtime side effect requires current safety bypass authorization") {
		t.Fatalf("expected strict safety authorization error, got %v stdout=%q stderr=%q", err, out.String(), stderr.String())
	}
	if strings.Contains(out.String(), "worker-transport-daemon-control:") {
		t.Fatalf("blocked daemon control creation must not print created artifact, got stdout=%q", out.String())
	}
	controls, err := app.ListRuntimeWorkerTransportDaemonControlRequests(root, "remote-web-1")
	if err != nil {
		t.Fatalf("list daemon controls after blocked CLI creation: %v", err)
	}
	if len(controls) != 0 {
		t.Fatalf("blocked daemon control creation must not write artifacts, got %#v", controls)
	}
}

func TestRuntimeWorkerTransportDaemonControlCommandAllowsDisabledBoundariesWithCurrentSafetyAuthorization(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := app.InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	writeRuntimeWorkerHTTPDisabledSafetyScope(t, root)
	writeRuntimeWorkerHTTPCurrentSafetyAuthorization(t, root)

	var out bytes.Buffer
	cmd := newRootCommand(&out, new(bytes.Buffer))
	cmd.SetArgs([]string{
		"runtime", "worker-transport-daemon-control", root, "remote-web-1",
		"--action", "restart",
		"--reason", "authorized restart",
		"--requested-by", "alice",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("write authorized worker transport daemon control request: %v", err)
	}
	output := out.String()
	for _, want := range []string{
		"worker-transport-daemon-control:",
		"worker=remote-web-1",
		"status=pending",
		"mode=metadata-only",
		"action=restart",
		"reason=authorized restart",
		"requested_by=alice",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected authorized daemon control output to contain %q, got %q", want, output)
		}
	}
	controls, err := app.ListRuntimeWorkerTransportDaemonControlRequests(root, "remote-web-1")
	if err != nil {
		t.Fatalf("list daemon controls after authorized CLI creation: %v", err)
	}
	if len(controls) != 1 || controls[0].Action != "restart" || controls[0].Status != "pending" {
		t.Fatalf("expected one authorized pending daemon control, got %#v", controls)
	}
}

func TestRuntimeWorkerTransportRunDaemonCommandStopsWhenControlEnabled(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := app.InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	control, err := app.CreateRuntimeWorkerTransportDaemonControlRequest(root, "remote-web-1", app.RuntimeWorkerTransportDaemonControlRequest{
		Action:      "stop",
		Reason:      "operator requested maintenance",
		RequestedBy: "alice",
	}, app.RuntimeWorkerTransportDaemonControlRequestOptions{Now: time.Now().UTC()})
	if err != nil {
		t.Fatalf("create daemon control request: %v", err)
	}

	var out bytes.Buffer
	cmd := newRootCommand(&out, new(bytes.Buffer))
	cmd.SetArgs([]string{
		"runtime", "worker-transport-run-daemon", root, "remote-web-1",
		"--execute",
		"--use-request-command",
		"--enable-control",
		"--max-requests", "1",
		"--idle-rounds", "20",
		"--poll-interval-ms", "1",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("run worker transport daemon with control enabled: %v", err)
	}
	output := out.String()
	for _, want := range []string{
		"worker-transport-run-daemon: worker=remote-web-1 status=stopped processed=0 completed=0 failed=0 skipped=0",
		"control=stop",
		"control_request=" + control.Path,
		"stop_reason=operator requested maintenance",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected controlled daemon output to contain %q, got %q", want, output)
		}
	}
	read, err := app.ReadRuntimeWorkerTransportDaemonControlRequest(root, "remote-web-1", filepath.Base(control.Path))
	if err != nil {
		t.Fatalf("read daemon control request: %v", err)
	}
	if read.Status != "applied" || read.AppliedDaemonRunPath == "" {
		t.Fatalf("expected CLI-controlled daemon to mark control applied, got %#v", read)
	}
}

func TestRuntimeWorkerTransportRunSupervisorCommandRequiresExecuteFlag(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := app.InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}

	var out bytes.Buffer
	var stderr bytes.Buffer
	cmd := newRootCommand(&out, &stderr)
	cmd.SetArgs([]string{
		"runtime", "worker-transport-run-supervisor", root, "remote-web-1",
		"--use-request-command",
		"--enable-control",
		"--max-runs", "1",
	})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "worker transport daemon supervisor requires explicit execute") {
		t.Fatalf("expected execute flag error, got %v stdout=%q stderr=%q", err, out.String(), stderr.String())
	}
	runs, listErr := app.ListRuntimeWorkerTransportDaemonSupervisorRuns(root, "remote-web-1")
	if listErr != nil {
		t.Fatalf("list supervisor runs: %v", listErr)
	}
	if len(runs) != 0 {
		t.Fatalf("missing --execute must not write supervisor audit, got %#v", runs)
	}
}

func TestRuntimeWorkerTransportRunSupervisorCommandRejectsInvalidBounds(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	cases := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "max runs",
			args: []string{"runtime", "worker-transport-run-supervisor", root, "remote-web-1", "--execute", "--use-request-command", "--max-runs", "0"},
			want: "--max-runs must be positive",
		},
		{
			name: "max requests",
			args: []string{"runtime", "worker-transport-run-supervisor", root, "remote-web-1", "--execute", "--use-request-command", "--max-requests", "0"},
			want: "--max-requests must be positive",
		},
		{
			name: "idle rounds",
			args: []string{"runtime", "worker-transport-run-supervisor", root, "remote-web-1", "--execute", "--use-request-command", "--idle-rounds", "0"},
			want: "--idle-rounds must be positive",
		},
		{
			name: "poll interval",
			args: []string{"runtime", "worker-transport-run-supervisor", root, "remote-web-1", "--execute", "--use-request-command", "--poll-interval-ms", "0"},
			want: "--poll-interval-ms must be positive",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			var stderr bytes.Buffer
			cmd := newRootCommand(&out, &stderr)
			cmd.SetArgs(tc.args)
			err := cmd.Execute()
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected validation error %q, got %v stdout=%q stderr=%q", tc.want, err, out.String(), stderr.String())
			}
			runs, listErr := app.ListRuntimeWorkerTransportDaemonSupervisorRuns(root, "remote-web-1")
			if listErr != nil {
				t.Fatalf("list worker transport supervisor runs: %v", listErr)
			}
			if len(runs) != 0 {
				t.Fatalf("invalid supervisor bounds must not write artifacts, got %#v", runs)
			}
		})
	}
}

func TestRuntimeWorkerTransportRunSupervisorCommandRestartsAndListsRuns(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	dispatch := createCLIFileOutboxWorkerQueueDispatchWithToolPlan(t, root, app.RuntimeWorkerToolPlanPayload{
		Action:     "discover_assets",
		Runner:     "command",
		RunCommand: []string{"sh", "-c", "printf 'supervisor-plan=%s' \"$OURO_EXECUTION_ID\""},
	})
	if _, err := app.AcknowledgeRuntimeWorkerTransportDispatchRequest(root, "remote-web-1", filepath.Base(dispatch.TransportRequestPath), app.RuntimeWorkerTransportAckOptions{
		Ack: true,
		Now: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("ack transport request: %v", err)
	}
	control, err := app.CreateRuntimeWorkerTransportDaemonControlRequest(root, "remote-web-1", app.RuntimeWorkerTransportDaemonControlRequest{
		Action:      "restart",
		Reason:      "reload worker environment",
		RequestedBy: "alice",
	}, app.RuntimeWorkerTransportDaemonControlRequestOptions{Now: time.Now().UTC()})
	if err != nil {
		t.Fatalf("create daemon restart control request: %v", err)
	}

	var out bytes.Buffer
	cmd := newRootCommand(&out, new(bytes.Buffer))
	cmd.SetArgs([]string{
		"runtime", "worker-transport-run-supervisor", root, "remote-web-1",
		"--execute",
		"--use-request-command",
		"--enable-control",
		"--max-runs", "2",
		"--max-requests", "1",
		"--idle-rounds", "1",
		"--poll-interval-ms", "1",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("run worker transport supervisor: %v", err)
	}
	output := out.String()
	for _, want := range []string{
		"worker-transport-run-supervisor: worker=remote-web-1 status=completed runs=2 restarts=1 processed=1 completed=1 failed=0 skipped=0",
		"worker-transport-run-supervisor-daemon:",
		"status=restart-requested",
		"control=restart",
		"control_request=" + control.Path,
		"status=completed",
		"output=supervisor-plan=tool-web",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected supervisor output to contain %q, got %q", want, output)
		}
	}
	readControl, err := app.ReadRuntimeWorkerTransportDaemonControlRequest(root, "remote-web-1", filepath.Base(control.Path))
	if err != nil {
		t.Fatalf("read daemon restart control: %v", err)
	}
	if readControl.Status != "applied" || readControl.AppliedDaemonRunPath == "" {
		t.Fatalf("expected supervisor to apply restart control, got %#v", readControl)
	}

	var listOut bytes.Buffer
	listCmd := newRootCommand(&listOut, new(bytes.Buffer))
	listCmd.SetArgs([]string{"runtime", "worker-transport-supervisor-runs", root, "remote-web-1"})
	if err := listCmd.Execute(); err != nil {
		t.Fatalf("list worker transport supervisor runs: %v", err)
	}
	for _, want := range []string{
		"worker-transport-supervisor-runs: total=1",
		"worker-transport-supervisor-run:",
		"status=completed",
		"runs=2",
		"restarts=1",
		"processed=1",
	} {
		if !strings.Contains(listOut.String(), want) {
			t.Fatalf("expected supervisor run listing to contain %q, got %q", want, listOut.String())
		}
	}
}

func TestRuntimeWorkerQueueDispatchResultCommandWritesAndListsResults(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := app.InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	if _, err := app.RegisterRuntimeWorker(root, app.RuntimeWorkerArtifact{
		WorkerID:     "remote-web-1",
		Mode:         "remote",
		Status:       "available",
		Capabilities: []string{"web.discovery"},
	}, app.RuntimeWorkerRegistrationOptions{Now: time.Now().UTC()}); err != nil {
		t.Fatalf("register runtime worker: %v", err)
	}
	claim, err := app.CreateRuntimeWorkerQueueClaim(root, app.RuntimeWorkerQueueClaim{
		WorkerID:    "remote-web-1",
		RunID:       "run-dispatch-1",
		ExecutionID: "tool-web",
		Tool:        "web-api-pack",
		Capability:  "web.discovery",
	}, app.RuntimeWorkerQueueClaimOptions{Now: time.Now().UTC(), LeaseDuration: 10 * time.Minute})
	if err != nil {
		t.Fatalf("create worker queue claim: %v", err)
	}
	dispatch, err := app.ExecuteRuntimeWorkerQueueClaimDispatch(root, claim.Path, app.RuntimeWorkerQueueDispatchExecutionOptions{Execute: true, Now: time.Now().UTC()})
	if err != nil {
		t.Fatalf("execute worker queue claim: %v", err)
	}

	var out bytes.Buffer
	cmd := newRootCommand(&out, new(bytes.Buffer))
	cmd.SetArgs([]string{
		"runtime", "worker-queue-dispatch-result", root, filepath.Base(dispatch.Path),
		"--status", "completed",
		"--output", "remote worker completed discovery",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("submit worker queue dispatch result: %v", err)
	}
	output := out.String()
	for _, want := range []string{
		"worker-queue-dispatch-result:",
		"status=completed",
		"worker=remote-web-1",
		"execution=tool-web",
		"output=remote worker completed discovery",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected dispatch result output to contain %q, got %q", want, output)
		}
	}

	results, err := app.ListRuntimeWorkerQueueDispatchResults(root)
	if err != nil {
		t.Fatalf("list worker queue dispatch results: %v", err)
	}
	if len(results) != 1 || results[0].Status != "completed" {
		t.Fatalf("expected one dispatch result audit, got %#v", results)
	}
	var listOut bytes.Buffer
	listCmd := newRootCommand(&listOut, new(bytes.Buffer))
	listCmd.SetArgs([]string{"runtime", "worker-queue-dispatch-results", root})
	if err := listCmd.Execute(); err != nil {
		t.Fatalf("list worker queue dispatch results: %v", err)
	}
	if !strings.Contains(listOut.String(), "worker-queue-dispatch-results: total=1") || !strings.Contains(listOut.String(), "worker=remote-web-1") {
		t.Fatalf("expected queue dispatch result listing, got %q", listOut.String())
	}
}

func TestRuntimeWorkerDispatchRetryPlanCommandPrintsAndWritesMetadataOnlyFailedResults(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	marker := filepath.Join(t.TempDir(), "worker-dispatch-retry-ran")
	failedDispatch := createCLIFileOutboxWorkerQueueDispatchWithToolPlan(t, root, app.RuntimeWorkerToolPlanPayload{
		Action:     "discover_assets",
		Runner:     "command",
		Parser:     "web_api_parser",
		RunCommand: []string{"sh", "-c", "touch " + marker},
	})
	if _, err := app.SubmitRuntimeWorkerQueueDispatchResult(root, filepath.Base(failedDispatch.Path), app.RuntimeWorkerQueueDispatchResult{
		Status: "failed",
		Output: "partial worker output",
		Error:  "exit status 23",
	}, app.RuntimeWorkerQueueDispatchResultOptions{Now: time.Now().UTC()}); err != nil {
		t.Fatalf("submit failed worker queue dispatch result: %v", err)
	}
	completedClaim, err := app.CreateRuntimeWorkerQueueClaim(root, app.RuntimeWorkerQueueClaim{
		WorkerID:    "remote-web-1",
		RunID:       "run-dispatch-cli",
		ExecutionID: "tool-web-completed",
		Tool:        "web-api-pack",
		Capability:  "web.discovery",
	}, app.RuntimeWorkerQueueClaimOptions{Now: time.Now().UTC(), LeaseDuration: 10 * time.Minute})
	if err != nil {
		t.Fatalf("create completed worker queue claim: %v", err)
	}
	completedDispatch, err := app.ExecuteRuntimeWorkerQueueClaimDispatch(root, completedClaim.Path, app.RuntimeWorkerQueueDispatchExecutionOptions{
		Execute:       true,
		TransportMode: "file-outbox",
		Now:           time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("execute completed worker queue claim dispatch: %v", err)
	}
	if _, err := app.SubmitRuntimeWorkerQueueDispatchResult(root, filepath.Base(completedDispatch.Path), app.RuntimeWorkerQueueDispatchResult{
		Status: "completed",
		Output: "done",
	}, app.RuntimeWorkerQueueDispatchResultOptions{Now: time.Now().UTC()}); err != nil {
		t.Fatalf("submit completed worker queue dispatch result: %v", err)
	}

	var out bytes.Buffer
	cmd := newRootCommand(&out, new(bytes.Buffer))
	cmd.SetArgs([]string{"runtime", "worker-dispatch-retry-plan", root})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("print worker dispatch retry plan: %v", err)
	}
	output := out.String()
	for _, want := range []string{
		"worker-dispatch-retry-plan: total=2 ready=1 skipped=1",
		"retry-worker-dispatch:",
		"gate=worker-dispatch-retry-review",
		"mode=metadata-only",
		"worker=remote-web-1",
		"execution=tool-web",
		"error=exit status 23",
		"skip-worker-dispatch-result:",
		"reason=result-not-failed",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected worker dispatch retry plan output to contain %q, got %q", want, output)
		}
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("worker dispatch retry planning must not execute retry command, marker stat err=%v", err)
	}
	if entries, err := os.ReadDir(filepath.Join(root, "runtime", "worker-dispatch-retry-plans")); err == nil && len(entries) != 0 {
		t.Fatalf("retry plan without --write must not create artifacts, got %#v", entries)
	}

	var writeOut bytes.Buffer
	writeCmd := newRootCommand(&writeOut, new(bytes.Buffer))
	writeCmd.SetArgs([]string{"runtime", "worker-dispatch-retry-plan", root, "--write"})
	if err := writeCmd.Execute(); err != nil {
		t.Fatalf("write worker dispatch retry plan: %v", err)
	}
	if !strings.Contains(writeOut.String(), "worker-dispatch-retry-plan-artifact:") || !strings.Contains(writeOut.String(), "status=dry-run") || !strings.Contains(writeOut.String(), "mode=metadata-only") {
		t.Fatalf("expected written retry plan artifact output, got %q", writeOut.String())
	}
	plans, err := app.ListRuntimeWorkerDispatchRetryPlans(root)
	if err != nil {
		t.Fatalf("list worker dispatch retry plans: %v", err)
	}
	if len(plans) != 1 || plans[0].Summary.Ready != 1 || plans[0].Summary.Skipped != 1 {
		t.Fatalf("unexpected worker dispatch retry plan artifacts: %#v", plans)
	}
	var listOut bytes.Buffer
	listCmd := newRootCommand(&listOut, new(bytes.Buffer))
	listCmd.SetArgs([]string{"runtime", "worker-dispatch-retry-plans", root})
	if err := listCmd.Execute(); err != nil {
		t.Fatalf("list worker dispatch retry plans: %v", err)
	}
	if !strings.Contains(listOut.String(), "worker-dispatch-retry-plans: total=1") || !strings.Contains(listOut.String(), filepath.Base(plans[0].Path)) {
		t.Fatalf("expected retry plan listing, got %q", listOut.String())
	}
	var inspectOut bytes.Buffer
	inspectCmd := newRootCommand(&inspectOut, new(bytes.Buffer))
	inspectCmd.SetArgs([]string{"runtime", "worker-dispatch-retry-plans", root, filepath.Base(plans[0].Path)})
	if err := inspectCmd.Execute(); err != nil {
		t.Fatalf("inspect worker dispatch retry plan: %v", err)
	}
	if !strings.Contains(inspectOut.String(), "worker-dispatch-retry-plan-artifact:") || !strings.Contains(inspectOut.String(), "ready=1 skipped=1") {
		t.Fatalf("expected retry plan inspect output, got %q", inspectOut.String())
	}
}

func TestRuntimeWorkerDispatchRetryDispatchCommandWritesAndListsMetadataOnlyArtifact(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	marker := filepath.Join(t.TempDir(), "worker-dispatch-retry-dispatch-ran")
	failedDispatch := createCLIFileOutboxWorkerQueueDispatchWithToolPlan(t, root, app.RuntimeWorkerToolPlanPayload{
		Action:     "discover_assets",
		Runner:     "command",
		Parser:     "web_api_parser",
		RunCommand: []string{"sh", "-c", "touch " + marker},
	})
	if _, err := app.SubmitRuntimeWorkerQueueDispatchResult(root, filepath.Base(failedDispatch.Path), app.RuntimeWorkerQueueDispatchResult{
		Status: "failed",
		Error:  "exit status 23",
	}, app.RuntimeWorkerQueueDispatchResultOptions{Now: time.Now().UTC()}); err != nil {
		t.Fatalf("submit failed worker queue dispatch result: %v", err)
	}
	plan, err := app.CreateRuntimeWorkerDispatchRetryPlan(root, time.Now().UTC())
	if err != nil {
		t.Fatalf("create worker dispatch retry plan: %v", err)
	}

	var out bytes.Buffer
	cmd := newRootCommand(&out, new(bytes.Buffer))
	cmd.SetArgs([]string{"runtime", "worker-dispatch-retry-dispatch", root, filepath.Base(plan.Path)})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("write worker dispatch retry dispatch: %v", err)
	}
	output := out.String()
	for _, want := range []string{
		"worker-dispatch-retry-dispatch:",
		"status=dry-run",
		"mode=metadata-only",
		"retry_plan=" + plan.Path,
		"ready=1",
		"skipped=0",
		"retry-worker-dispatch:",
		"gate=worker-dispatch-retry-review",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected worker dispatch retry dispatch output to contain %q, got %q", want, output)
		}
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("worker dispatch retry dispatch must not execute retry command, marker stat err=%v", err)
	}
	dispatches, err := app.ListRuntimeWorkerDispatchRetryDispatches(root)
	if err != nil {
		t.Fatalf("list worker dispatch retry dispatches: %v", err)
	}
	if len(dispatches) != 1 || dispatches[0].Summary.Ready != 1 || dispatches[0].RetryPlanPath != plan.Path {
		t.Fatalf("unexpected worker dispatch retry dispatch artifacts: %#v", dispatches)
	}

	var listOut bytes.Buffer
	listCmd := newRootCommand(&listOut, new(bytes.Buffer))
	listCmd.SetArgs([]string{"runtime", "worker-dispatch-retry-dispatches", root})
	if err := listCmd.Execute(); err != nil {
		t.Fatalf("list worker dispatch retry dispatches: %v", err)
	}
	if !strings.Contains(listOut.String(), "worker-dispatch-retry-dispatches: total=1") || !strings.Contains(listOut.String(), filepath.Base(dispatches[0].Path)) {
		t.Fatalf("expected retry dispatch listing, got %q", listOut.String())
	}

	var inspectOut bytes.Buffer
	inspectCmd := newRootCommand(&inspectOut, new(bytes.Buffer))
	inspectCmd.SetArgs([]string{"runtime", "worker-dispatch-retry-dispatches", root, filepath.Base(dispatches[0].Path)})
	if err := inspectCmd.Execute(); err != nil {
		t.Fatalf("inspect worker dispatch retry dispatch: %v", err)
	}
	if !strings.Contains(inspectOut.String(), "worker-dispatch-retry-dispatch: "+dispatches[0].Path) || !strings.Contains(inspectOut.String(), "retry_plan="+plan.Path) {
		t.Fatalf("expected retry dispatch inspect output, got %q", inspectOut.String())
	}
}

func TestRuntimeExecuteWorkerDispatchRetryDispatchCommandCreatesMetadataOnlyQueueClaim(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	marker := filepath.Join(t.TempDir(), "worker-dispatch-retry-dispatch-execute-ran")
	failedDispatch := createCLIFileOutboxWorkerQueueDispatchWithToolPlan(t, root, app.RuntimeWorkerToolPlanPayload{
		Action:     "discover_assets",
		Runner:     "command",
		Parser:     "web_api_parser",
		RunCommand: []string{"sh", "-c", "touch " + marker},
	})
	if _, err := app.SubmitRuntimeWorkerQueueDispatchResult(root, filepath.Base(failedDispatch.Path), app.RuntimeWorkerQueueDispatchResult{
		Status: "failed",
		Error:  "exit status 23",
	}, app.RuntimeWorkerQueueDispatchResultOptions{Now: time.Now().UTC()}); err != nil {
		t.Fatalf("submit failed worker queue dispatch result: %v", err)
	}
	plan, err := app.CreateRuntimeWorkerDispatchRetryPlan(root, time.Now().UTC())
	if err != nil {
		t.Fatalf("create worker dispatch retry plan: %v", err)
	}
	dispatch, err := app.CreateRuntimeWorkerDispatchRetryDispatch(root, filepath.Base(plan.Path), time.Now().UTC())
	if err != nil {
		t.Fatalf("create worker dispatch retry dispatch: %v", err)
	}
	claimsBefore, err := app.ListRuntimeWorkerQueueClaims(root)
	if err != nil {
		t.Fatalf("list queue claims before retry dispatch execution: %v", err)
	}
	dispatchesBefore, err := app.ListRuntimeWorkerQueueDispatchExecutions(root)
	if err != nil {
		t.Fatalf("list queue dispatches before retry dispatch execution: %v", err)
	}

	var missingFlagOut bytes.Buffer
	missingFlagCmd := newRootCommand(&missingFlagOut, new(bytes.Buffer))
	missingFlagCmd.SetArgs([]string{"runtime", "execute-worker-dispatch-retry-dispatch", root, filepath.Base(dispatch.Path)})
	if err := missingFlagCmd.Execute(); err == nil || !strings.Contains(err.Error(), "requires explicit execute") {
		t.Fatalf("expected explicit execute error, got %v output=%q", err, missingFlagOut.String())
	}

	var out bytes.Buffer
	cmd := newRootCommand(&out, new(bytes.Buffer))
	cmd.SetArgs([]string{"runtime", "execute-worker-dispatch-retry-dispatch", root, filepath.Base(dispatch.Path), "--execute", "--lease-seconds", "900"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute worker dispatch retry dispatch: %v", err)
	}
	output := out.String()
	for _, want := range []string{
		"worker-dispatch-retry-dispatch-execution:",
		"status=completed",
		"mode=execute",
		"retry_dispatch=" + dispatch.Path,
		"claimed=1",
		"retry-worker-queue-claim:",
		"retry_claim=",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected worker dispatch retry execution output to contain %q, got %q", want, output)
		}
	}
	claimsAfter, err := app.ListRuntimeWorkerQueueClaims(root)
	if err != nil {
		t.Fatalf("list queue claims after retry dispatch execution: %v", err)
	}
	if got := len(claimsAfter); got != len(claimsBefore)+1 {
		t.Fatalf("expected one retry queue claim: before=%d after=%d", len(claimsBefore), got)
	}
	dispatchesAfter, err := app.ListRuntimeWorkerQueueDispatchExecutions(root)
	if err != nil {
		t.Fatalf("list queue dispatches after retry dispatch execution: %v", err)
	}
	if got := len(dispatchesAfter); got != len(dispatchesBefore) {
		t.Fatalf("retry dispatch execution must not create queue dispatches: before=%d after=%d", len(dispatchesBefore), got)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("retry dispatch execution must not execute retry command, marker stat err=%v", err)
	}
	executions, err := app.ListRuntimeWorkerDispatchRetryDispatchExecutions(root)
	if err != nil {
		t.Fatalf("list worker dispatch retry dispatch executions: %v", err)
	}
	if len(executions) != 1 || executions[0].Summary.Claimed != 1 {
		t.Fatalf("unexpected worker dispatch retry executions: %#v", executions)
	}

	var listOut bytes.Buffer
	listCmd := newRootCommand(&listOut, new(bytes.Buffer))
	listCmd.SetArgs([]string{"runtime", "worker-dispatch-retry-dispatch-executions", root})
	if err := listCmd.Execute(); err != nil {
		t.Fatalf("list worker dispatch retry dispatch executions: %v", err)
	}
	if !strings.Contains(listOut.String(), "worker-dispatch-retry-dispatch-executions: total=1") || !strings.Contains(listOut.String(), filepath.Base(executions[0].Path)) {
		t.Fatalf("expected retry dispatch execution listing, got %q", listOut.String())
	}
}

func TestRuntimeExecuteWorkerDispatchRetryDispatchCommandRejectsInvalidLeaseBeforeRetryClaim(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	failedDispatch := createCLIFileOutboxWorkerQueueDispatchWithToolPlan(t, root, app.RuntimeWorkerToolPlanPayload{
		Action:     "discover_assets",
		Runner:     "command",
		Parser:     "web_api_parser",
		RunCommand: []string{"sh", "-c", "exit 23"},
	})
	if _, err := app.SubmitRuntimeWorkerQueueDispatchResult(root, filepath.Base(failedDispatch.Path), app.RuntimeWorkerQueueDispatchResult{
		Status: "failed",
		Error:  "exit status 23",
	}, app.RuntimeWorkerQueueDispatchResultOptions{Now: time.Now().UTC()}); err != nil {
		t.Fatalf("submit failed worker queue dispatch result: %v", err)
	}
	plan, err := app.CreateRuntimeWorkerDispatchRetryPlan(root, time.Now().UTC())
	if err != nil {
		t.Fatalf("create worker dispatch retry plan: %v", err)
	}
	dispatch, err := app.CreateRuntimeWorkerDispatchRetryDispatch(root, filepath.Base(plan.Path), time.Now().UTC())
	if err != nil {
		t.Fatalf("create worker dispatch retry dispatch: %v", err)
	}
	claimsBefore, err := app.ListRuntimeWorkerQueueClaims(root)
	if err != nil {
		t.Fatalf("list queue claims before retry dispatch execution: %v", err)
	}

	var out bytes.Buffer
	var stderr bytes.Buffer
	cmd := newRootCommand(&out, &stderr)
	cmd.SetArgs([]string{
		"runtime", "execute-worker-dispatch-retry-dispatch", root, filepath.Base(dispatch.Path),
		"--execute",
		"--lease-seconds", "-1",
	})
	err = cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "--lease-seconds must be positive") {
		t.Fatalf("expected lease validation error before retry claim write, got %v stdout=%q stderr=%q", err, out.String(), stderr.String())
	}
	claimsAfter, err := app.ListRuntimeWorkerQueueClaims(root)
	if err != nil {
		t.Fatalf("list queue claims after retry dispatch execution: %v", err)
	}
	if len(claimsAfter) != len(claimsBefore) {
		t.Fatalf("invalid retry lease must not create retry queue claims: before=%d after=%d", len(claimsBefore), len(claimsAfter))
	}
	executions, err := app.ListRuntimeWorkerDispatchRetryDispatchExecutions(root)
	if err != nil {
		t.Fatalf("list worker dispatch retry dispatch executions: %v", err)
	}
	if len(executions) != 0 {
		t.Fatalf("invalid retry lease must not write retry dispatch executions, got %#v", executions)
	}
}

func TestRuntimeExecuteWorkerDispatchRetryDispatchCommandDispatchesRetryClaimsToFileOutbox(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	marker := filepath.Join(t.TempDir(), "worker-dispatch-retry-dispatch-handoff-ran")
	failedDispatch := createCLIFileOutboxWorkerQueueDispatchWithToolPlan(t, root, app.RuntimeWorkerToolPlanPayload{
		Action:     "discover_assets",
		Runner:     "command",
		Parser:     "web_api_parser",
		RunCommand: []string{"sh", "-c", "touch " + marker},
	})
	if _, err := app.SubmitRuntimeWorkerQueueDispatchResult(root, filepath.Base(failedDispatch.Path), app.RuntimeWorkerQueueDispatchResult{
		Status: "failed",
		Error:  "exit status 23",
	}, app.RuntimeWorkerQueueDispatchResultOptions{Now: time.Now().UTC()}); err != nil {
		t.Fatalf("submit failed worker queue dispatch result: %v", err)
	}
	plan, err := app.CreateRuntimeWorkerDispatchRetryPlan(root, time.Now().UTC())
	if err != nil {
		t.Fatalf("create worker dispatch retry plan: %v", err)
	}
	dispatch, err := app.CreateRuntimeWorkerDispatchRetryDispatch(root, filepath.Base(plan.Path), time.Now().UTC())
	if err != nil {
		t.Fatalf("create worker dispatch retry dispatch: %v", err)
	}
	dispatchesBefore, err := app.ListRuntimeWorkerQueueDispatchExecutions(root)
	if err != nil {
		t.Fatalf("list queue dispatches before retry handoff: %v", err)
	}
	requestsBefore, err := app.ListRuntimeWorkerTransportDispatchRequests(root, "remote-web-1")
	if err != nil {
		t.Fatalf("list transport requests before retry handoff: %v", err)
	}

	var out bytes.Buffer
	cmd := newRootCommand(&out, new(bytes.Buffer))
	cmd.SetArgs([]string{"runtime", "execute-worker-dispatch-retry-dispatch", root, filepath.Base(dispatch.Path), "--execute", "--dispatch-claims", "--transport", "file-outbox"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute worker dispatch retry dispatch handoff: %v", err)
	}
	output := out.String()
	for _, want := range []string{
		"worker-dispatch-retry-dispatch-execution:",
		"claimed=1",
		"dispatched=1",
		"retry-worker-queue-claim:",
		"status=dispatched",
		"retry_queue_dispatch=",
		"retry_transport_request=",
		"transport=file-outbox",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected worker dispatch retry handoff output to contain %q, got %q", want, output)
		}
	}
	dispatchesAfter, err := app.ListRuntimeWorkerQueueDispatchExecutions(root)
	if err != nil {
		t.Fatalf("list queue dispatches after retry handoff: %v", err)
	}
	if got := len(dispatchesAfter); got != len(dispatchesBefore)+1 {
		t.Fatalf("expected retry handoff to create one queue dispatch: before=%d after=%d", len(dispatchesBefore), got)
	}
	requestsAfter, err := app.ListRuntimeWorkerTransportDispatchRequests(root, "remote-web-1")
	if err != nil {
		t.Fatalf("list transport requests after retry handoff: %v", err)
	}
	if got := len(requestsAfter); got != len(requestsBefore)+1 {
		t.Fatalf("expected retry handoff to create one file-outbox request: before=%d after=%d", len(requestsBefore), got)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("retry handoff must not execute retry command, marker stat err=%v", err)
	}
	executions, err := app.ListRuntimeWorkerDispatchRetryDispatchExecutions(root)
	if err != nil {
		t.Fatalf("list worker dispatch retry dispatch executions: %v", err)
	}
	if len(executions) != 1 || executions[0].Summary.Dispatched != 1 || executions[0].Results[0].RetryQueueDispatchPath == "" || executions[0].Results[0].RetryTransportRequestPath == "" {
		t.Fatalf("unexpected persisted retry handoff execution: %#v", executions)
	}
}

func TestRuntimeWorkerDispatchRetrySchedulerRunCommandWritesDryRunOnly(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	marker := filepath.Join(t.TempDir(), "worker-dispatch-retry-scheduler-cli-ran")
	failedDispatch := createCLIFileOutboxWorkerQueueDispatchWithToolPlan(t, root, app.RuntimeWorkerToolPlanPayload{
		Action:     "discover_assets",
		Runner:     "command",
		Parser:     "web_api_parser",
		RunCommand: []string{"sh", "-c", "touch " + marker},
	})
	if _, err := app.SubmitRuntimeWorkerQueueDispatchResult(root, filepath.Base(failedDispatch.Path), app.RuntimeWorkerQueueDispatchResult{
		Status: "failed",
		Error:  "exit status 23",
	}, app.RuntimeWorkerQueueDispatchResultOptions{Now: time.Now().UTC()}); err != nil {
		t.Fatalf("submit failed worker queue dispatch result: %v", err)
	}

	var out bytes.Buffer
	cmd := newRootCommand(&out, new(bytes.Buffer))
	cmd.SetArgs([]string{"runtime", "worker-dispatch-retry-scheduler-run", root, "--actor", "scheduler-test", "--reason", "operator audit", "--max-runs", "2"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute worker dispatch retry scheduler dry-run: %v", err)
	}
	output := out.String()
	for _, want := range []string{
		"worker-dispatch-retry-scheduler-run-artifact:",
		"status=dry-run",
		"mode=metadata-only",
		"max_runs=2",
		"runs_created=0",
		"retry_plans_created=0",
		"retry_claims_created=0",
		"dispatch_retry_claims=false",
		"transport=metadata-only",
		"current_ready=1",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected scheduler dry-run output to contain %q, got %q", want, output)
		}
	}
	if got, err := app.ListRuntimeWorkerDispatchRetrySchedulerRuns(root); err != nil || len(got) != 1 {
		t.Fatalf("expected one scheduler run artifact, got %#v err=%v", got, err)
	}
	if got, err := app.ListRuntimeWorkerDispatchRetryPlans(root); err != nil || len(got) != 0 {
		t.Fatalf("dry-run scheduler must not create retry plans, got %#v err=%v", got, err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("scheduler dry-run must not execute retry command, marker stat err=%v", err)
	}
}

func TestRuntimeWorkerDispatchRetrySchedulerRunCommandExecutesBoundedRetryOnce(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	marker := filepath.Join(t.TempDir(), "worker-dispatch-retry-scheduler-cli-execute-ran")
	failedDispatch := createCLIFileOutboxWorkerQueueDispatchWithToolPlan(t, root, app.RuntimeWorkerToolPlanPayload{
		Action:     "discover_assets",
		Runner:     "command",
		Parser:     "web_api_parser",
		RunCommand: []string{"sh", "-c", "touch " + marker},
	})
	if _, err := app.SubmitRuntimeWorkerQueueDispatchResult(root, filepath.Base(failedDispatch.Path), app.RuntimeWorkerQueueDispatchResult{
		Status: "failed",
		Error:  "exit status 23",
	}, app.RuntimeWorkerQueueDispatchResultOptions{Now: time.Now().UTC()}); err != nil {
		t.Fatalf("submit failed worker queue dispatch result: %v", err)
	}

	var out bytes.Buffer
	cmd := newRootCommand(&out, new(bytes.Buffer))
	cmd.SetArgs([]string{"runtime", "worker-dispatch-retry-scheduler-run", root, "--execute", "--max-runs", "2", "--lease-seconds", "1200"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute worker dispatch retry scheduler run: %v", err)
	}
	output := out.String()
	for _, want := range []string{
		"worker-dispatch-retry-scheduler-run-artifact:",
		"status=completed",
		"mode=execute",
		"max_runs=2",
		"runs_created=1",
		"retry_plans_created=1",
		"retry_dispatches_created=1",
		"retry_dispatch_executions_created=1",
		"retry_claims_created=1",
		"retry_claims_dispatched=0",
		"retry_transport_requests_created=0",
		"dispatch_retry_claims=false",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected scheduler execute output to contain %q, got %q", want, output)
		}
	}
	if got, err := app.ListRuntimeWorkerDispatchRetryPlans(root); err != nil || len(got) != 1 {
		t.Fatalf("expected one retry plan, got %#v err=%v", got, err)
	}
	executions, err := app.ListRuntimeWorkerDispatchRetryDispatchExecutions(root)
	if err != nil {
		t.Fatalf("list retry dispatch executions: %v", err)
	}
	if len(executions) != 1 || executions[0].Summary.Claimed != 1 {
		t.Fatalf("expected one retry execution with one claim, got %#v", executions)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("scheduler execute must not execute retry command, marker stat err=%v", err)
	}

	var listOut bytes.Buffer
	listCmd := newRootCommand(&listOut, new(bytes.Buffer))
	listCmd.SetArgs([]string{"runtime", "worker-dispatch-retry-scheduler-runs", root})
	if err := listCmd.Execute(); err != nil {
		t.Fatalf("list worker dispatch retry scheduler runs: %v", err)
	}
	if !strings.Contains(listOut.String(), "worker-dispatch-retry-scheduler-runs: total=1") {
		t.Fatalf("expected scheduler listing, got %q", listOut.String())
	}
}

func TestRuntimeWorkerDispatchRetrySchedulerRunCommandDispatchesRetryClaimsToFileOutbox(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	marker := filepath.Join(t.TempDir(), "worker-dispatch-retry-scheduler-cli-handoff-ran")
	failedDispatch := createCLIFileOutboxWorkerQueueDispatchWithToolPlan(t, root, app.RuntimeWorkerToolPlanPayload{
		Action:     "discover_assets",
		Runner:     "command",
		Parser:     "web_api_parser",
		RunCommand: []string{"sh", "-c", "touch " + marker},
	})
	if _, err := app.SubmitRuntimeWorkerQueueDispatchResult(root, filepath.Base(failedDispatch.Path), app.RuntimeWorkerQueueDispatchResult{
		Status: "failed",
		Error:  "exit status 23",
	}, app.RuntimeWorkerQueueDispatchResultOptions{Now: time.Now().UTC()}); err != nil {
		t.Fatalf("submit failed worker queue dispatch result: %v", err)
	}
	dispatchesBefore, err := app.ListRuntimeWorkerQueueDispatchExecutions(root)
	if err != nil {
		t.Fatalf("list queue dispatches before scheduler handoff: %v", err)
	}
	requestsBefore, err := app.ListRuntimeWorkerTransportDispatchRequests(root, "remote-web-1")
	if err != nil {
		t.Fatalf("list transport requests before scheduler handoff: %v", err)
	}

	var out bytes.Buffer
	cmd := newRootCommand(&out, new(bytes.Buffer))
	cmd.SetArgs([]string{
		"runtime", "worker-dispatch-retry-scheduler-run", root,
		"--execute",
		"--max-runs", "1",
		"--dispatch-retry-claims",
		"--retry-transport", "file-outbox",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute worker dispatch retry scheduler handoff: %v", err)
	}
	output := out.String()
	for _, want := range []string{
		"worker-dispatch-retry-scheduler-run-artifact:",
		"status=completed",
		"mode=execute",
		"dispatch_retry_claims=true",
		"retry_transport=file-outbox",
		"retry_claims_created=1",
		"retry_claims_dispatched=1",
		"retry_transport_requests_created=1",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected scheduler handoff output to contain %q, got %q", want, output)
		}
	}
	runs, err := app.ListRuntimeWorkerDispatchRetrySchedulerRuns(root)
	if err != nil {
		t.Fatalf("list scheduler runs: %v", err)
	}
	if len(runs) != 1 || runs[0].RetryClaimsDispatched != 1 || runs[0].RetryTransportRequestsCreated != 1 {
		t.Fatalf("unexpected scheduler handoff run: %#v", runs)
	}
	executions, err := app.ListRuntimeWorkerDispatchRetryDispatchExecutions(root)
	if err != nil {
		t.Fatalf("list retry dispatch executions: %v", err)
	}
	if len(executions) != 1 || executions[0].Summary.Dispatched != 1 {
		t.Fatalf("expected one retry dispatch execution with one handoff, got %#v", executions)
	}
	dispatchesAfter, err := app.ListRuntimeWorkerQueueDispatchExecutions(root)
	if err != nil {
		t.Fatalf("list queue dispatches after scheduler handoff: %v", err)
	}
	if got := len(dispatchesAfter); got != len(dispatchesBefore)+1 {
		t.Fatalf("expected scheduler handoff to create one queue dispatch: before=%d after=%d", len(dispatchesBefore), got)
	}
	requestsAfter, err := app.ListRuntimeWorkerTransportDispatchRequests(root, "remote-web-1")
	if err != nil {
		t.Fatalf("list transport requests after scheduler handoff: %v", err)
	}
	if got := len(requestsAfter); got != len(requestsBefore)+1 {
		t.Fatalf("expected scheduler handoff to create one file-outbox request: before=%d after=%d", len(requestsBefore), got)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("scheduler handoff must not execute retry command, marker stat err=%v", err)
	}
}

func TestRuntimeWorkerDispatchRetrySchedulerRunCommandRejectsInvalidExecuteAndHandoffBeforeWriting(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{
			name: "execute without positive max runs",
			args: []string{"runtime", "worker-dispatch-retry-scheduler-run", filepath.Join(t.TempDir(), "workspace"), "--execute"},
			want: "--execute requires --max-runs > 0",
		},
		{
			name: "handoff without execute",
			args: []string{"runtime", "worker-dispatch-retry-scheduler-run", filepath.Join(t.TempDir(), "workspace"), "--dispatch-retry-claims"},
			want: "retry claim handoff requires execute",
		},
		{
			name: "http-json rejected",
			args: []string{"runtime", "worker-dispatch-retry-scheduler-run", filepath.Join(t.TempDir(), "workspace"), "--execute", "--max-runs", "1", "--dispatch-retry-claims", "--retry-transport", "http-json"},
			want: "worker dispatch retry claim handoff supports metadata-only or file-outbox transport",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			cmd := newRootCommand(&out, new(bytes.Buffer))
			cmd.SetArgs(tc.args)
			err := cmd.Execute()
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected validation error %q, got err=%v output=%q", tc.want, err, out.String())
			}
			root := tc.args[2]
			if runs, listErr := app.ListRuntimeWorkerDispatchRetrySchedulerRuns(root); listErr != nil || len(runs) != 0 {
				t.Fatalf("invalid scheduler command must not write scheduler runs, got %#v err=%v", runs, listErr)
			}
			if plans, listErr := app.ListRuntimeWorkerDispatchRetryPlans(root); listErr != nil || len(plans) != 0 {
				t.Fatalf("invalid scheduler command must not write retry plans, got %#v err=%v", plans, listErr)
			}
		})
	}
}

func TestRuntimeWorkerQueueDispatchResultServeHTTPCommandAcceptsAuthorizedCallback(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	dispatch := createCLIFileOutboxWorkerQueueDispatch(t, root)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("allocate listen address: %v", err)
	}
	addr := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatalf("close temporary listener: %v", err)
	}
	t.Setenv("OURO_RESULT_TOKEN", "secret-result-token")

	var out bytes.Buffer
	cmd := newRootCommand(&out, new(bytes.Buffer))
	cmd.SetArgs([]string{
		"runtime", "worker-queue-dispatch-result-serve-http", root,
		"--execute",
		"--listen", addr,
		"--max-results", "1",
		"--auth-token-env", "OURO_RESULT_TOKEN",
	})
	done := make(chan error, 1)
	go func() {
		done <- cmd.Execute()
	}()

	payload := app.RuntimeWorkerQueueDispatchResult{
		DispatchExecutionPath: filepath.Base(dispatch.Path),
		Status:                "completed",
		Output:                "remote callback complete",
	}
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal callback payload: %v", err)
	}
	client := http.Client{Timeout: 100 * time.Millisecond}
	var unauthorizedSeen bool
	var lastErr error
	for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		response, err := client.Post("http://"+addr+"/result", "application/json", bytes.NewReader(data))
		if err != nil {
			lastErr = err
			continue
		}
		_ = response.Body.Close()
		if response.StatusCode == http.StatusUnauthorized {
			unauthorizedSeen = true
			lastErr = nil
			break
		}
		lastErr = fmt.Errorf("unexpected unauthenticated status %d", response.StatusCode)
	}
	if lastErr != nil {
		t.Fatalf("post unauthenticated callback: %v", lastErr)
	}
	if !unauthorizedSeen {
		t.Fatalf("expected unauthenticated callback to be rejected")
	}

	request, err := http.NewRequest(http.MethodPost, "http://"+addr+"/result", bytes.NewReader(data))
	if err != nil {
		t.Fatalf("create authorized callback: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer secret-result-token")
	response, err := client.Do(request)
	if err != nil {
		t.Fatalf("post authorized callback: %v", err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusAccepted {
		t.Fatalf("expected authorized callback accepted, got %d", response.StatusCode)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("serve http result callback: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("result callback server did not exit after max result")
	}
	results, err := app.ListRuntimeWorkerQueueDispatchResults(root)
	if err != nil {
		t.Fatalf("list callback results: %v", err)
	}
	if len(results) != 1 || results[0].Status != "completed" || results[0].Output != "remote callback complete" {
		t.Fatalf("expected one completed callback result, got %#v", results)
	}
	if strings.Contains(out.String(), "secret-result-token") {
		t.Fatalf("serve output must not contain result callback token, got %q", out.String())
	}
	for _, want := range []string{
		"worker-queue-dispatch-result-serve-http: listen=" + addr,
		"worker-queue-dispatch-result-serve-http: accepted=1",
	} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("expected result callback serve output to contain %q, got %q", want, out.String())
		}
	}
}

func TestRuntimeWorkerQueueDispatchResultServeHTTPCommandRejectsNegativeMaxResultsBeforeListen(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := app.InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}

	var out bytes.Buffer
	var stderr bytes.Buffer
	cmd := newRootCommand(&out, &stderr)
	cmd.SetArgs([]string{
		"runtime", "worker-queue-dispatch-result-serve-http", root,
		"--execute",
		"--listen", "not-a-tcp-address",
		"--max-results", "-1",
	})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "--max-results cannot be negative") {
		t.Fatalf("expected max-results validation error before listen, got %v stdout=%q stderr=%q", err, out.String(), stderr.String())
	}
	if strings.Contains(out.String(), "worker-queue-dispatch-result-serve-http:") {
		t.Fatalf("negative max-results must not start receiver, got stdout=%q", out.String())
	}
	results, listErr := app.ListRuntimeWorkerQueueDispatchResults(root)
	if listErr != nil {
		t.Fatalf("list worker dispatch results: %v", listErr)
	}
	if len(results) != 0 {
		t.Fatalf("negative max-results must not write result artifacts, got %#v", results)
	}
}

func createCLIFileOutboxWorkerQueueDispatch(t *testing.T, root string) app.RuntimeWorkerQueueDispatchExecution {
	return createCLIFileOutboxWorkerQueueDispatchWithToolPlan(t, root, app.RuntimeWorkerToolPlanPayload{})
}

func createCLIFileOutboxWorkerQueueDispatchWithToolPlan(t *testing.T, root string, plan app.RuntimeWorkerToolPlanPayload) app.RuntimeWorkerQueueDispatchExecution {
	t.Helper()
	if err := app.InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	now := time.Now().UTC()
	if _, err := app.RegisterRuntimeWorker(root, app.RuntimeWorkerArtifact{
		WorkerID:     "remote-web-1",
		Mode:         "remote",
		Status:       "available",
		Endpoint:     "file://remote-web-1/inbox",
		Capabilities: []string{"web.discovery"},
	}, app.RuntimeWorkerRegistrationOptions{Now: now}); err != nil {
		t.Fatalf("register runtime worker: %v", err)
	}
	claim, err := app.CreateRuntimeWorkerQueueClaim(root, app.RuntimeWorkerQueueClaim{
		WorkerID:    "remote-web-1",
		RunID:       "run-dispatch-file",
		ExecutionID: "tool-web",
		Tool:        "web-api-pack",
		Capability:  "web.discovery",
		ToolPlan:    plan,
	}, app.RuntimeWorkerQueueClaimOptions{Now: now, LeaseDuration: 10 * time.Minute})
	if err != nil {
		t.Fatalf("create worker queue claim: %v", err)
	}
	dispatch, err := app.ExecuteRuntimeWorkerQueueClaimDispatch(root, claim.Path, app.RuntimeWorkerQueueDispatchExecutionOptions{
		Execute:       true,
		Now:           now.Add(time.Minute),
		TransportMode: "file-outbox",
	})
	if err != nil {
		t.Fatalf("execute file-outbox dispatch: %v", err)
	}
	return dispatch
}

func createCLIWorkerQueueDispatchForExecution(t *testing.T, root string, now time.Time, runID string, executionID string) app.RuntimeWorkerQueueDispatchExecution {
	t.Helper()
	claim, err := app.CreateRuntimeWorkerQueueClaim(root, app.RuntimeWorkerQueueClaim{
		WorkerID:    "remote-web-1",
		RunID:       runID,
		ExecutionID: executionID,
		Tool:        "web-api-pack",
		Capability:  "web.discovery",
	}, app.RuntimeWorkerQueueClaimOptions{Now: now, LeaseDuration: 10 * time.Minute})
	if err != nil {
		t.Fatalf("create worker queue claim: %v", err)
	}
	dispatch, err := app.ExecuteRuntimeWorkerQueueClaimDispatch(root, claim.Path, app.RuntimeWorkerQueueDispatchExecutionOptions{
		Execute:       true,
		Now:           now.Add(time.Minute),
		TransportMode: "file-outbox",
	})
	if err != nil {
		t.Fatalf("execute file-outbox dispatch: %v", err)
	}
	return dispatch
}

func TestRuntimeExecuteWorkerLeaseReclaimCommandRequiresExecuteFlag(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := app.InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	leasePath := writeCLIWorkerLeaseFixture(t, root, app.WorkerLeaseArtifact{
		Version:     1,
		RunID:       "run-stale",
		ExecutionID: "tool-stale",
		Tool:        "web-api-pack",
		Capability:  "web.discovery",
		WorkerID:    "local",
		Status:      "active",
		AcquiredAt:  time.Now().UTC().Add(-20 * time.Minute).Format(time.RFC3339Nano),
		HeartbeatAt: time.Now().UTC().Add(-20 * time.Minute).Format(time.RFC3339Nano),
	})
	dispatch, err := app.CreateWorkerLeaseReclaimDispatch(root, time.Now().UTC(), app.DefaultWorkerLeaseStaleAfter)
	if err != nil {
		t.Fatalf("create reclaim dispatch: %v", err)
	}

	var out bytes.Buffer
	cmd := newRootCommand(&out, new(bytes.Buffer))
	cmd.SetArgs([]string{"runtime", "execute-worker-lease-reclaim", root, filepath.Base(dispatch.Path)})
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "requires explicit execute") {
		t.Fatalf("expected explicit execute error, got err=%v output=%q", err, out.String())
	}
	leaseData, err := os.ReadFile(leasePath)
	if err != nil {
		t.Fatalf("read original worker lease: %v", err)
	}
	var lease app.WorkerLeaseArtifact
	if err := json.Unmarshal(leaseData, &lease); err != nil {
		t.Fatalf("decode original worker lease: %v", err)
	}
	if lease.Status != "active" || lease.ReleasedAt != "" {
		t.Fatalf("missing --execute must not mutate lease, got %#v", lease)
	}
	paths, err := filepath.Glob(filepath.Join(root, "runtime", "worker-lease-reclaim-executions", "*.json"))
	if err != nil {
		t.Fatalf("glob reclaim execution artifacts: %v", err)
	}
	if len(paths) != 0 {
		t.Fatalf("missing --execute must not write execution artifact, got %#v", paths)
	}
}

func TestRuntimeExecuteWorkerLeaseReclaimCommandRunsWithExecuteFlag(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := app.InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	writeCLIWorkerLeaseFixture(t, root, app.WorkerLeaseArtifact{
		Version:     1,
		RunID:       "run-stale",
		ExecutionID: "tool-stale",
		Tool:        "web-api-pack",
		Capability:  "web.discovery",
		WorkerID:    "local",
		Status:      "active",
		AcquiredAt:  time.Now().UTC().Add(-20 * time.Minute).Format(time.RFC3339Nano),
		HeartbeatAt: time.Now().UTC().Add(-20 * time.Minute).Format(time.RFC3339Nano),
	})
	dispatch, err := app.CreateWorkerLeaseReclaimDispatch(root, time.Now().UTC(), app.DefaultWorkerLeaseStaleAfter)
	if err != nil {
		t.Fatalf("create reclaim dispatch: %v", err)
	}

	var out bytes.Buffer
	cmd := newRootCommand(&out, new(bytes.Buffer))
	cmd.SetArgs([]string{"runtime", "execute-worker-lease-reclaim", root, filepath.Base(dispatch.Path), "--execute"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute worker lease reclaim: %v", err)
	}
	output := out.String()
	for _, want := range []string{
		"worker-lease-reclaim-execution:",
		"status=completed",
		"mode=execute",
		"dispatch=" + dispatch.Path,
		"reclaim-worker-lease-execute: run=run-stale execution=tool-stale status=reclaimed",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected execute-worker-lease-reclaim output to contain %q, got %q", want, output)
		}
	}
	executions, err := app.ListWorkerLeaseReclaimExecutions(root)
	if err != nil {
		t.Fatalf("list reclaim executions: %v", err)
	}
	if len(executions) != 1 || len(executions[0].Results) != 1 || executions[0].Results[0].Status != "reclaimed" {
		t.Fatalf("expected one reclaimed execution artifact, got %#v", executions)
	}

	var listOut bytes.Buffer
	listCmd := newRootCommand(&listOut, new(bytes.Buffer))
	listCmd.SetArgs([]string{"runtime", "worker-lease-reclaim-executions", root})
	if err := listCmd.Execute(); err != nil {
		t.Fatalf("list worker lease reclaim executions: %v", err)
	}
	if !strings.Contains(listOut.String(), "worker-lease-reclaim-executions: total=1") || !strings.Contains(listOut.String(), "reclaim-worker-lease-execute: run=run-stale execution=tool-stale status=reclaimed") {
		t.Fatalf("expected listed reclaim execution output, got %q", listOut.String())
	}
}

func writeCLIInstallRequestFixture(t *testing.T, root string, request app.ToolchainInstallRequest) app.ToolchainInstallRequest {
	t.Helper()
	dir := filepath.Join(root, "toolchain", "install-requests")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("create install request fixture dir: %v", err)
	}
	request.Path = filepath.Join(dir, request.Pack+"-"+request.Dependency+".json")
	data, err := json.MarshalIndent(request, "", "  ")
	if err != nil {
		t.Fatalf("encode install request fixture: %v", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(request.Path, data, 0o644); err != nil {
		t.Fatalf("write install request fixture: %v", err)
	}
	return request
}

func writeCLIInstallExecutionFixture(t *testing.T, root string, name string, execution app.ToolchainInstallDispatchExecution) string {
	t.Helper()
	dir := filepath.Join(root, "toolchain", "install-executions")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("create execution fixture dir: %v", err)
	}
	path := filepath.Join(dir, name)
	execution.Path = path
	data, err := json.MarshalIndent(execution, "", "  ")
	if err != nil {
		t.Fatalf("encode execution fixture: %v", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write execution fixture: %v", err)
	}
	return path
}

func writeCLIWorkerLeaseFixture(t *testing.T, root string, lease app.WorkerLeaseArtifact) string {
	t.Helper()
	dir := filepath.Join(root, "runtime", "worker-leases", lease.RunID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("create worker lease fixture dir: %v", err)
	}
	path := filepath.Join(dir, lease.ExecutionID+".json")
	data, err := json.MarshalIndent(lease, "", "  ")
	if err != nil {
		t.Fatalf("encode worker lease fixture: %v", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write worker lease fixture: %v", err)
	}
	return path
}

func writeCLIInstallRetryExecutionFixture(t *testing.T, root string, name string, execution app.ToolchainInstallRetryDispatchExecution) string {
	t.Helper()
	dir := filepath.Join(root, "toolchain", "install-retry-executions")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("create retry execution fixture dir: %v", err)
	}
	path := filepath.Join(dir, name)
	execution.Path = path
	data, err := json.MarshalIndent(execution, "", "  ")
	if err != nil {
		t.Fatalf("encode retry execution fixture: %v", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write retry execution fixture: %v", err)
	}
	return path
}

func TestToolchainHealthCommandShowsDependencyProvenance(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := app.InitWorkspace(root); err != nil {
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
  - name: ouro-httpx-worker
    kind: container_image
    version: "1.0.0"
    latest_version: "1.1.0"
    source: ghcr.io/ouroboros/httpx-worker
    image_digest: sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
`
	if err := os.WriteFile(filepath.Join(root, "toolchain", "web-api-pack", "manifest.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	var healthOut bytes.Buffer
	healthCmd := newRootCommand(&healthOut, new(bytes.Buffer))
	healthCmd.SetArgs([]string{"toolchain", "health", root})
	if err := healthCmd.Execute(); err != nil {
		t.Fatalf("execute toolchain health: %v", err)
	}
	for _, want := range []string{
		"dependency: web-api-pack/ouro-httpx-worker",
		"state=update_available",
		"latest=1.1.0",
		"source=ghcr.io/ouroboros/httpx-worker",
		"digest=sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
	} {
		if !strings.Contains(healthOut.String(), want) {
			t.Fatalf("expected dependency provenance output to contain %q, got %q", want, healthOut.String())
		}
	}
}

func TestToolchainHealthCommandShowsDependencyRemoteMetadata(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := app.InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	pinned := "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	latest := "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	updateDir := filepath.Join(root, "toolchain", "web-api-pack", "updates")
	if err := os.MkdirAll(updateDir, 0o755); err != nil {
		t.Fatalf("create updates dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(updateDir, "httpx-worker.json"), []byte(`{
  "latest_version": "1.2.0",
  "image_digest": "`+latest+`",
  "source": "ghcr.io/ouroboros/httpx-worker",
  "message": "remote metadata fixture"
}`), 0o644); err != nil {
		t.Fatalf("write metadata: %v", err)
	}
	manifest := `name: web-api-pack
version: "0.1.0"
capabilities:
  - web.discovery
runner: native
parser: web_api_parser
healthcheck: built_in
dependencies:
  - name: ouro-httpx-worker
    kind: container_image
    version: "1.0.0"
    source: ghcr.io/ouroboros/httpx-worker
    image_digest: ` + pinned + `
    metadata_source: updates/httpx-worker.json
`
	if err := os.WriteFile(filepath.Join(root, "toolchain", "web-api-pack", "manifest.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	var healthOut bytes.Buffer
	healthCmd := newRootCommand(&healthOut, new(bytes.Buffer))
	healthCmd.SetArgs([]string{"toolchain", "health", root})
	if err := healthCmd.Execute(); err != nil {
		t.Fatalf("execute toolchain health: %v", err)
	}
	for _, want := range []string{
		"dependency: web-api-pack/ouro-httpx-worker",
		"state=update_available",
		"latest=1.2.0",
		"latest_digest=" + latest,
		"metadata_source=updates/httpx-worker.json",
		"remote metadata fixture",
	} {
		if !strings.Contains(healthOut.String(), want) {
			t.Fatalf("expected dependency remote metadata output to contain %q, got %q", want, healthOut.String())
		}
	}
}

func TestToolchainHealthCommandShowsContainerImageDigestMismatch(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := app.InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	expected := "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	actual := "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	probe := filepath.Join(t.TempDir(), "image-inspect")
	if err := os.WriteFile(probe, []byte("#!/bin/sh\nprintf 'ghcr.io/ouroboros/httpx-worker@"+actual+"\\n'\n"), 0o755); err != nil {
		t.Fatalf("write image probe: %v", err)
	}
	manifest := `name: web-api-pack
version: "0.1.0"
capabilities:
  - web.discovery
runner: native
parser: web_api_parser
healthcheck: built_in
dependencies:
  - name: ouro-httpx-worker
    kind: container_image
    version: "1.0.0"
    source: ghcr.io/ouroboros/httpx-worker
    image_digest: ` + expected + `
    health_command:
      - ` + probe + `
      - image
      - inspect
      - ghcr.io/ouroboros/httpx-worker
`
	if err := os.WriteFile(filepath.Join(root, "toolchain", "web-api-pack", "manifest.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	var healthOut bytes.Buffer
	healthCmd := newRootCommand(&healthOut, new(bytes.Buffer))
	healthCmd.SetArgs([]string{"toolchain", "health", root})
	if err := healthCmd.Execute(); err != nil {
		t.Fatalf("execute toolchain health: %v", err)
	}
	for _, want := range []string{
		"dependency: web-api-pack/ouro-httpx-worker",
		"state=failed",
		"digest mismatch",
		expected,
		actual,
	} {
		if !strings.Contains(healthOut.String(), want) {
			t.Fatalf("expected dependency digest mismatch output to contain %q, got %q", want, healthOut.String())
		}
	}
}

func parseOutputValue(t *testing.T, output string, key string) string {
	t.Helper()
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, key) {
			return strings.TrimSpace(strings.TrimPrefix(line, key))
		}
	}
	t.Fatalf("missing %s in output %q", key, output)
	return ""
}

func createPlacementDispatchBatchPlanForCLI(t *testing.T, root string, transportMode string) app.RuntimeWorkerPlacementDispatchPlan {
	t.Helper()
	if err := app.InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	now := time.Now().UTC()
	workers := []app.RuntimeWorkerArtifact{
		{
			WorkerID:     "remote-web-1",
			Mode:         "remote",
			Status:       "available",
			Endpoint:     "https://web-worker.example.test/dispatch",
			Capabilities: []string{"web.discovery"},
		},
		{
			WorkerID:     "remote-infra-1",
			Mode:         "remote",
			Status:       "available",
			Endpoint:     "https://infra-worker.example.test/dispatch",
			Capabilities: []string{"infra.host_discovery"},
		},
		{
			WorkerID:     "remote-ctf-1",
			Mode:         "remote",
			Status:       "available",
			Endpoint:     "https://ctf-worker.example.test/dispatch",
			Capabilities: []string{"ctf.challenge_classification"},
		},
	}
	for _, worker := range workers {
		if _, err := app.RegisterRuntimeWorker(root, worker, app.RuntimeWorkerRegistrationOptions{Now: now}); err != nil {
			t.Fatalf("register worker %s: %v", worker.WorkerID, err)
		}
	}
	placement, err := app.CreateRuntimeWorkerPlacementPlan(root, app.RuntimeWorkerPlacementPlanOptions{
		Now:   now,
		RunID: "run-placement-dispatch-batch-cli",
	})
	if err != nil {
		t.Fatalf("create placement plan: %v", err)
	}
	for _, candidate := range placement.Candidates {
		if candidate.Status != "ready" {
			continue
		}
		if _, err := app.ExecuteRuntimeWorkerPlacementClaim(root, placement.Path, app.RuntimeWorkerPlacementClaimExecutionOptions{
			Execute:              true,
			CandidateExecutionID: candidate.ExecutionID,
			WorkerID:             candidate.WorkerID,
			Now:                  now.Add(time.Minute),
		}); err != nil {
			t.Fatalf("execute placement claim for %s/%s: %v", candidate.WorkerID, candidate.ExecutionID, err)
		}
	}
	plan, err := app.CreateRuntimeWorkerPlacementDispatchPlan(root, app.RuntimeWorkerPlacementDispatchPlanOptions{
		Now:           now.Add(2 * time.Minute),
		TransportMode: transportMode,
	})
	if err != nil {
		t.Fatalf("create placement dispatch plan: %v", err)
	}
	if plan.ReadyCount != 8 {
		t.Fatalf("expected eight ready placement dispatch candidates, got %#v", plan)
	}
	return plan
}

func registerCLIWorkerOrchestrationWorkers(t *testing.T, root string) {
	t.Helper()
	if err := app.InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	now := time.Now().UTC()
	for _, worker := range []app.RuntimeWorkerArtifact{
		{
			WorkerID:     "remote-web-1",
			Mode:         "remote",
			Status:       "available",
			Endpoint:     "https://web-worker.example.test/dispatch",
			Capabilities: []string{"web.discovery"},
		},
		{
			WorkerID:     "remote-infra-1",
			Mode:         "remote",
			Status:       "available",
			Endpoint:     "https://infra-worker.example.test/dispatch",
			Capabilities: []string{"infra.host_discovery"},
		},
		{
			WorkerID:     "remote-ctf-1",
			Mode:         "remote",
			Status:       "available",
			Endpoint:     "https://ctf-worker.example.test/dispatch",
			Capabilities: []string{"ctf.challenge_classification"},
		},
	} {
		if _, err := app.RegisterRuntimeWorker(root, worker, app.RuntimeWorkerRegistrationOptions{Now: now}); err != nil {
			t.Fatalf("register worker %s: %v", worker.WorkerID, err)
		}
	}
}

type fakeInteractiveSession struct {
	initialView        string
	initialCursor      int
	initialCursorTotal int
	initialModel       interactiveModel
	sent               []tea.Msg
	sentCh             chan tea.Msg
	runCount           int
	quitCount          int
	runFunc            func(*fakeInteractiveSession) error
}

func (s *fakeInteractiveSession) Run() error {
	s.runCount++
	if s.runFunc != nil {
		return s.runFunc(s)
	}
	return nil
}

func (s *fakeInteractiveSession) Send(msg tea.Msg) {
	s.sent = append(s.sent, msg)
	if s.sentCh != nil {
		select {
		case s.sentCh <- msg:
		default:
		}
	}
}

func (s *fakeInteractiveSession) Quit() {
	s.quitCount++
}

func (s *fakeInteractiveSession) replaySnapshots() []eventlog.ReplaySnapshot {
	snapshots := []eventlog.ReplaySnapshot{}
	for _, msg := range s.sent {
		typed, ok := msg.(tui.ReplaySnapshotMsg)
		if !ok {
			continue
		}
		snapshots = append(snapshots, typed.Snapshot)
	}
	return snapshots
}

func replaceInteractiveSession(t *testing.T, session *fakeInteractiveSession) func() {
	t.Helper()
	previous := newInteractiveSession
	newInteractiveSession = func(model interactiveModel) interactiveSession {
		session.initialModel = model
		session.initialView = model.View()
		if withCursor, ok := model.(interface{ ReplayCursor() (int, int) }); ok {
			session.initialCursor, session.initialCursorTotal = withCursor.ReplayCursor()
		}
		return session
	}
	return func() {
		newInteractiveSession = previous
	}
}
