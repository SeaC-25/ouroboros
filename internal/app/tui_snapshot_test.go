package app

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ouroboros/ouroboros/internal/compiler"
	"github.com/ouroboros/ouroboros/internal/contracts"
	"github.com/ouroboros/ouroboros/internal/eventlog"
	"github.com/ouroboros/ouroboros/internal/toolchain"
	"github.com/ouroboros/ouroboros/internal/tui"
)

func openAIDialogInputIfNeeded(t *testing.T, model tui.Model) tui.Model {
	t.Helper()
	if strings.Contains(model.View(), "You > |") {
		return model
	}
	open, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}})
	return open.(tui.Model)
}

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

func TestNewTUIModelForWorkspaceAIDialogUsesConfiguredProvider(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	var captured struct {
		Model    string `json:"model"`
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("expected chat completions POST, got %s %s", r.Method, r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
			t.Fatalf("decode provider request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"choices": [{"message": {"role": "assistant", "content": "Run scoped discovery first."}}],
			"usage": {"prompt_tokens": 13, "completion_tokens": 5, "total_tokens": 18}
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
    timeout_ms: 5000
    enabled: true
`, server.URL)
	if err := os.WriteFile(filepath.Join(root, "providers.yaml"), []byte(providers), 0o644); err != nil {
		t.Fatalf("write providers: %v", err)
	}

	model := NewTUIModelForWorkspace(root)
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 180, Height: 36})
	model = updated.(tui.Model)
	model = openAIDialogInputIfNeeded(t, model)
	for _, ch := range "what should I do next?" {
		typed, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}})
		model = typed.(tui.Model)
	}
	submitting, submitCmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if submitCmd == nil {
		t.Fatalf("expected AI dialog enter to submit to provider")
	}
	completed, _ := submitting.Update(submitCmd())
	model = completed.(tui.Model)

	if captured.Model != "gpt-test" || len(captured.Messages) != 2 || captured.Messages[0].Role != "system" || !strings.Contains(captured.Messages[0].Content, "authorized security work") || captured.Messages[1].Role != "user" || captured.Messages[1].Content != "what should I do next?" {
		t.Fatalf("unexpected provider request: %#v", captured)
	}
	view := model.View()
	for _, want := range []string{
		"You > what should I do next?",
		"AI > Run scoped discovery first.",
		"provider=local-openai model=gpt-test tokens=18",
		"stored=false",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected AI dialog provider response view to contain %q:\n%s", want, view)
		}
	}
}

func TestNewTUIModelForWorkspaceAIDialogSendsCurrentSessionHistory(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	type capturedRequest struct {
		Model    string `json:"model"`
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
	}
	requests := []capturedRequest{}
	const firstPrompt = "USER_HISTORY_SECRET_PROMPT_1 summarize scope"
	const secondPrompt = "USER_HISTORY_SECRET_PROMPT_2 refine next gate"
	const firstAssistant = "ASSISTANT_HISTORY_SECRET_RESPONSE_1 use scoped discovery"
	const secondAssistant = "ASSISTANT_HISTORY_SECRET_RESPONSE_2 inspect the review gate"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("expected chat completions POST, got %s %s", r.Method, r.URL.Path)
		}
		var captured capturedRequest
		if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
			t.Fatalf("decode provider request: %v", err)
		}
		requests = append(requests, captured)
		assistantText := firstAssistant
		if len(requests) == 2 {
			assistantText = secondAssistant
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{
			"choices": [{"message": {"role": "assistant", "content": %q}}],
			"usage": {"prompt_tokens": 31, "completion_tokens": 7, "total_tokens": 38}
		}`, assistantText)
	}))
	defer server.Close()
	t.Setenv("OURO_TEST_PROVIDER_KEY", "secret-value")
	providers := fmt.Sprintf(`version: 1
providers:
  - name: local-openai
    type: openai_compatible
    model: gpt-history
    base_url: %s/v1
    api_key_env: OURO_TEST_PROVIDER_KEY
    timeout_ms: 5000
    enabled: true
`, server.URL)
	if err := os.WriteFile(filepath.Join(root, "providers.yaml"), []byte(providers), 0o644); err != nil {
		t.Fatalf("write providers: %v", err)
	}

	model := NewTUIModelForWorkspace(root)
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 2200, Height: 48})
	model = updated.(tui.Model)
	for _, prompt := range []string{firstPrompt, secondPrompt} {
		model = openAIDialogInputIfNeeded(t, model)
		for _, ch := range prompt {
			typed, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}})
			model = typed.(tui.Model)
		}
		submitting, submitCmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
		if submitCmd == nil {
			t.Fatalf("expected AI dialog enter to submit prompt %q", prompt)
		}
		completed, cmd := submitting.Update(submitCmd())
		if cmd != nil {
			t.Fatalf("expected AI dialog result update to be synchronous")
		}
		model = completed.(tui.Model)
	}

	if len(requests) != 2 {
		t.Fatalf("expected two provider requests, got %#v", requests)
	}
	firstMessages := requests[0].Messages
	if len(firstMessages) != 2 || firstMessages[0].Role != "system" || !strings.Contains(firstMessages[0].Content, "authorized security work") || firstMessages[1].Role != "user" || firstMessages[1].Content != firstPrompt {
		t.Fatalf("unexpected first provider request messages: %#v", firstMessages)
	}
	secondMessages := requests[1].Messages
	if len(secondMessages) != 4 {
		t.Fatalf("expected second provider request to include system plus prior user/assistant and current user, got %#v", secondMessages)
	}
	wantSecond := []struct {
		role    string
		content string
	}{
		{"system", ""},
		{"user", firstPrompt},
		{"assistant", firstAssistant},
		{"user", secondPrompt},
	}
	for i, want := range wantSecond {
		if secondMessages[i].Role != want.role {
			t.Fatalf("unexpected second provider message %d role: got %#v want %q", i, secondMessages[i], want.role)
		}
		if want.content != "" && secondMessages[i].Content != want.content {
			t.Fatalf("unexpected second provider message %d content: got %#v want %q", i, secondMessages[i], want.content)
		}
	}
	if strings.Contains(secondMessages[0].Content, firstPrompt) || strings.Contains(secondMessages[0].Content, firstAssistant) {
		t.Fatalf("system prompt should not absorb chat history: %#v", secondMessages[0])
	}
	workspaceData := strings.Builder{}
	if err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		workspaceData.Write(data)
		return nil
	}); err != nil {
		t.Fatalf("walk workspace files: %v", err)
	}
	for _, forbidden := range []string{firstPrompt, secondPrompt, firstAssistant, secondAssistant, "secret-value"} {
		if strings.Contains(workspaceData.String(), forbidden) {
			t.Fatalf("AI dialog in-session history leaked %q into workspace files", forbidden)
		}
	}
}

func TestNewTUIModelForWorkspaceAIDialogSendsReadOnlyWorkspaceContext(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	mission := `version: 1
mission:
  name: context secret mission
  goal: "review https://context.secret.example/app?token=context-token"
`
	if err := os.WriteFile(filepath.Join(root, "mission.yaml"), []byte(mission), 0o644); err != nil {
		t.Fatalf("write mission: %v", err)
	}
	scope := `version: 1
scope:
  target_policy: enforce
  targets:
    - kind: url
      value: https://context.secret.example/app?token=context-token
    - kind: domain
      value: context.secret.example
  exclusions:
    - https://context.secret.example/private
execution:
  isolation: process
`
	if err := os.WriteFile(filepath.Join(root, "scope.yaml"), []byte(scope), 0o644); err != nil {
		t.Fatalf("write scope: %v", err)
	}
	var captured struct {
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("expected chat completions POST, got %s %s", r.Method, r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
			t.Fatalf("decode provider request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"choices": [{"message": {"role": "assistant", "content": "Use the next safe gate."}}],
			"usage": {"prompt_tokens": 19, "completion_tokens": 6, "total_tokens": 25}
		}`))
	}))
	defer server.Close()
	t.Setenv("OURO_TEST_PROVIDER_KEY", "secret-value")
	providers := fmt.Sprintf(`version: 1
providers:
  - name: local-openai
    type: openai_compatible
    model: gpt-context
    base_url: %s/v1
    api_key_env: OURO_TEST_PROVIDER_KEY
    timeout_ms: 5000
    enabled: true
`, server.URL)
	if err := os.WriteFile(filepath.Join(root, "providers.yaml"), []byte(providers), 0o644); err != nil {
		t.Fatalf("write providers: %v", err)
	}

	model := NewTUIModelForWorkspace(root)
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 2200, Height: 48})
	model = updated.(tui.Model)
	model = openAIDialogInputIfNeeded(t, model)
	const prompt = "CONTEXT_PROMPT_SHOULD_NOT_BE_IN_SYSTEM"
	for _, ch := range prompt {
		typed, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}})
		model = typed.(tui.Model)
	}
	submitting, submitCmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if submitCmd == nil {
		t.Fatalf("expected AI dialog enter to submit to provider")
	}
	completed, cmd := submitting.Update(submitCmd())
	if cmd != nil {
		t.Fatalf("expected AI dialog result update to be synchronous")
	}
	_ = completed

	if len(captured.Messages) == 0 || captured.Messages[0].Role != "system" {
		t.Fatalf("expected first provider message to be system context, got %#v", captured.Messages)
	}
	system := captured.Messages[0].Content
	for _, want := range []string{
		"authorized security work",
		"Workspace context:",
		"workflow.mode=read-only",
		"workflow.status=ready",
		"workflow.stage=model-tool-plan",
		"workflow.next_gate=run-provider-tool-plan-preview",
		"workflow.reason=no-current-model-tool-plan",
		"workflow.safety_errors=0",
		"overview.status=available",
		"overview.workflow_status=ready",
		"overview.stage=model-tool-plan",
		"overview.next_gate=run-provider-tool-plan-preview",
		"overview.mission_configured=true",
		"overview.target_policy=enforce",
		"overview.isolation=process",
		"overview.targets=2",
		"overview.safety_ready=true",
		"overview.strict_ready=true",
		"overview.providers_status=ready",
		"overview.providers=1",
		"overview.provider_enabled=1",
		"overview.chat=false",
		"overview.tool_planning=false",
		"overview.capabilities_status=ready",
		"overview.evidence_runs=0",
		"overview.findings=0",
		"overview.reports_current=false",
		"overview.workers_registered=0",
		"overview.worker_queue_claims=0",
		"semantic_actions.status=available",
		"semantic_actions.count=7",
		"semantic_actions.allowed=collect_evidence,discover_assets,enumerate_paths,fingerprint_surface,generate_candidates,produce_report,verify_candidate",
		"semantic_actions.marker_format=semantic_action:<action>",
		"semantic_actions.mode=preview_only",
		"semantic_actions.acceptance=operator_review_required",
	} {
		if !strings.Contains(system, want) {
			t.Fatalf("expected system prompt workspace context to contain %q:\n%s", want, system)
		}
	}
	for _, forbidden := range []string{root, prompt, "secret-value", server.URL, "context secret mission", "review https://", "context.secret.example", "context-token", "gpt-context", "OURO_TEST_PROVIDER_KEY", "configure-openai-compatible", "subfinder", "httpx", "nuclei", "RunCommand", "--output", "toolchain/", "Authorization", "Bearer"} {
		if strings.Contains(system, forbidden) {
			t.Fatalf("system prompt workspace context leaked %q:\n%s", forbidden, system)
		}
	}
}

func TestNewTUIModelForWorkspaceAIDialogStatusCommandDoesNotRequireProvider(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}

	model := NewTUIModelForWorkspace(root)
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 2200, Height: 48})
	model = updated.(tui.Model)
	model = openAIDialogInputIfNeeded(t, model)
	for _, ch := range "/status" {
		typed, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}})
		model = typed.(tui.Model)
	}
	submitting, submitCmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if submitCmd == nil {
		t.Fatalf("expected AI dialog /status to submit")
	}
	completed, cmd := submitting.Update(submitCmd())
	if cmd != nil {
		t.Fatalf("expected AI dialog /status update to be synchronous")
	}
	view := completed.View()
	for _, want := range []string{
		"You > /status",
		"AI > workflow status: status=ready stage=model-tool-plan next_gate=run-provider-tool-plan-preview reason=no-current-model-tool-plan safety_errors=0 stored=false",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected /status response to contain %q:\n%s", want, view)
		}
	}
	for _, forbidden := range []string{
		"AI > error:",
		"provider=local-openai",
	} {
		if strings.Contains(view, forbidden) {
			t.Fatalf("/status should not require provider setup or provider metadata %q:\n%s", forbidden, view)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "replay", "events.db")); err == nil {
		t.Fatalf("/status must not create replay events.db")
	} else if !os.IsNotExist(err) {
		t.Fatalf("stat replay events.db: %v", err)
	}
	for _, pattern := range []string{
		"runtime/model-usage/*.json",
		"runtime/model-generations/*.json",
		"runtime/model-tool-plan-previews/*.json",
		"toolchain/compile-preview-reviews/*.json",
		"toolchain/compile-preview-review-execution-dispatches/*.json",
		"runtime/worker-queue-claims/*.json",
	} {
		if got := workflowStatusGlobCount(t, root, pattern); got != 0 {
			t.Fatalf("/status must not create artifacts matching %s, got %d", pattern, got)
		}
	}
}

func TestNewTUIModelForWorkspaceAIDialogNextCommandDoesNotRequireProvider(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	t.Setenv("OURO_TEST_PROVIDER_KEY", "secret-value")

	model := NewTUIModelForWorkspace(root)
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 2200, Height: 48})
	model = updated.(tui.Model)
	model = openAIDialogInputIfNeeded(t, model)
	for _, ch := range "/next" {
		typed, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}})
		model = typed.(tui.Model)
	}
	submitting, submitCmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if submitCmd == nil {
		t.Fatalf("expected AI dialog /next to submit")
	}
	completed, cmd := submitting.Update(submitCmd())
	if cmd != nil {
		t.Fatalf("expected AI dialog /next update to be synchronous")
	}
	view := completed.View()
	for _, want := range []string{
		"You > /next",
		"AI > next gate:",
		"status=ready",
		"stage=model-tool-plan",
		"next_gate=run-provider-tool-plan-preview",
		"reason=no-current-model-tool-plan",
		"action=none",
		"safety_errors=0",
		"command_available=true",
		"stored=false",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected /next response to contain %q:\n%s", want, view)
		}
	}
	nextLineStart := strings.Index(view, "AI > next gate:")
	if nextLineStart < 0 {
		t.Fatalf("expected /next response line in view:\n%s", view)
	}
	nextLine := view[nextLineStart:]
	if nextLineEnd := strings.Index(nextLine, "\n"); nextLineEnd >= 0 {
		nextLine = nextLine[:nextLineEnd]
	}
	for _, forbidden := range []string{
		"AI > error:",
		root,
		"configure-openai-compatible",
		"provider.example",
		"secret-value",
	} {
		if strings.Contains(nextLine, forbidden) {
			t.Fatalf("/next should not require provider setup, leak paths, or leak secrets %q:\n%s", forbidden, view)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "replay", "events.db")); err == nil {
		t.Fatalf("/next must not create replay events.db")
	} else if !os.IsNotExist(err) {
		t.Fatalf("stat replay events.db: %v", err)
	}
	for _, pattern := range []string{
		"runtime/model-usage/*.json",
		"runtime/model-generations/*.json",
		"runtime/model-tool-plan-previews/*.json",
		"toolchain/compile-preview-reviews/*.json",
		"toolchain/compile-preview-review-execution-dispatches/*.json",
		"runtime/worker-queue-claims/*.json",
	} {
		if got := workflowStatusGlobCount(t, root, pattern); got != 0 {
			t.Fatalf("/next must not create artifacts matching %s, got %d", pattern, got)
		}
	}
}

func TestNewTUIModelForWorkspaceAIDialogCommandCommandDoesNotRequireProvider(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	t.Setenv("OURO_TEST_PROVIDER_KEY", "secret-value")

	model := NewTUIModelForWorkspace(root)
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 2200, Height: 48})
	model = updated.(tui.Model)
	model = openAIDialogInputIfNeeded(t, model)
	for _, ch := range "/command" {
		typed, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}})
		model = typed.(tui.Model)
	}
	submitting, submitCmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if submitCmd == nil {
		t.Fatalf("expected AI dialog /command to submit")
	}
	completed, cmd := submitting.Update(submitCmd())
	if cmd != nil {
		t.Fatalf("expected AI dialog /command update to be synchronous")
	}
	view := completed.View()
	for _, want := range []string{
		"You > /command",
		"AI > next command:",
		"status=ready",
		"stage=model-tool-plan",
		"next_gate=run-provider-tool-plan-preview",
		"reason=no-current-model-tool-plan",
		"command=ouro run <workspace> --provider-generate --provider-generate-tool-plan-preview --provider-generate-prompt <prompt>",
		"stored=false",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected /command response to contain %q:\n%s", want, view)
		}
	}
	commandLineStart := strings.Index(view, "AI > next command:")
	if commandLineStart < 0 {
		t.Fatalf("expected /command response line in view:\n%s", view)
	}
	commandLine := view[commandLineStart:]
	if commandLineEnd := strings.Index(commandLine, "\n"); commandLineEnd >= 0 {
		commandLine = commandLine[:commandLineEnd]
	}
	for _, forbidden := range []string{
		"AI > error:",
		root,
		"configure-openai-compatible",
		"provider.example",
		"secret-value",
	} {
		if strings.Contains(commandLine, forbidden) {
			t.Fatalf("/command should not require provider setup, leak paths, or leak secrets %q:\n%s", forbidden, view)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "replay", "events.db")); err == nil {
		t.Fatalf("/command must not create replay events.db")
	} else if !os.IsNotExist(err) {
		t.Fatalf("stat replay events.db: %v", err)
	}
	for _, pattern := range []string{
		"runtime/model-usage/*.json",
		"runtime/model-generations/*.json",
		"runtime/model-tool-plan-previews/*.json",
		"toolchain/compile-preview-reviews/*.json",
		"toolchain/compile-preview-review-execution-dispatches/*.json",
		"runtime/worker-queue-claims/*.json",
	} {
		if got := workflowStatusGlobCount(t, root, pattern); got != 0 {
			t.Fatalf("/command must not create artifacts matching %s, got %d", pattern, got)
		}
	}
}

func TestNewTUIModelForWorkspaceAIDialogArtifactsCommandDoesNotRequireProvider(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	t.Setenv("OURO_TEST_PROVIDER_KEY", "secret-value")

	model := NewTUIModelForWorkspace(root)
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 2200, Height: 48})
	model = updated.(tui.Model)
	model = openAIDialogInputIfNeeded(t, model)
	for _, ch := range "/artifacts" {
		typed, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}})
		model = typed.(tui.Model)
	}
	submitting, submitCmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if submitCmd == nil {
		t.Fatalf("expected AI dialog /artifacts to submit")
	}
	completed, cmd := submitting.Update(submitCmd())
	if cmd != nil {
		t.Fatalf("expected AI dialog /artifacts update to be synchronous")
	}
	view := completed.View()
	for _, want := range []string{
		"You > /artifacts",
		"AI > workflow artifacts:",
		"status=ready",
		"stage=model-tool-plan",
		"previews=0",
		"acceptances=0",
		"compile_dispatches=0",
		"compile_executions=0",
		"report_dispatches=0",
		"report_executions=0",
		"safety_errors=0",
		"stored=false",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected /artifacts response to contain %q:\n%s", want, view)
		}
	}
	artifactsLineStart := strings.Index(view, "AI > workflow artifacts:")
	if artifactsLineStart < 0 {
		t.Fatalf("expected /artifacts response line in view:\n%s", view)
	}
	artifactsLine := view[artifactsLineStart:]
	if artifactsLineEnd := strings.Index(artifactsLine, "\n"); artifactsLineEnd >= 0 {
		artifactsLine = artifactsLine[:artifactsLineEnd]
	}
	for _, forbidden := range []string{
		"AI > error:",
		root,
		"configure-openai-compatible",
		"provider.example",
		"secret-value",
	} {
		if strings.Contains(artifactsLine, forbidden) {
			t.Fatalf("/artifacts should not require provider setup, leak paths, or leak secrets %q:\n%s", forbidden, view)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "replay", "events.db")); err == nil {
		t.Fatalf("/artifacts must not create replay events.db")
	} else if !os.IsNotExist(err) {
		t.Fatalf("stat replay events.db: %v", err)
	}
	for _, pattern := range []string{
		"runtime/model-usage/*.json",
		"runtime/model-generations/*.json",
		"runtime/model-tool-plan-previews/*.json",
		"toolchain/compile-preview-reviews/*.json",
		"toolchain/compile-preview-review-execution-dispatches/*.json",
		"runtime/worker-queue-claims/*.json",
	} {
		if got := workflowStatusGlobCount(t, root, pattern); got != 0 {
			t.Fatalf("/artifacts must not create artifacts matching %s, got %d", pattern, got)
		}
	}
}

func TestNewTUIModelForWorkspaceAIDialogPreviewsCommandDoesNotRequireProvider(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	t.Setenv("OURO_TEST_PROVIDER_KEY", "secret-value")
	review, err := CreateToolCompilePreviewReview(root, ToolCompilePreviewOptions{Action: compiler.ActionEnumeratePaths})
	if err != nil {
		t.Fatalf("create compile-preview review: %v", err)
	}
	preview, err := RecordModelToolPlanPreviewArtifact(root, "run-ai-preview-summary", ProviderGenerationResult{
		ProviderName: "local-openai",
		Model:        "gpt-plan",
		Text:         "semantic_action: enumerate_paths\nSECRET_ASSISTANT_TEXT_SHOULD_NOT_LEAK",
	}, "", ModelToolPlanPreviewExtraction{
		Found:             true,
		Action:            compiler.ActionEnumeratePaths,
		Valid:             true,
		Reason:            "accepted",
		MarkerPreview:     "semantic_action: enumerate_paths",
		ModelOutputDigest: "sha256:model-output",
	}, review, ModelToolPlanPreviewArtifactOptions{
		Source:  "tui-ai-dialog",
		AgentID: "tui-ai-dialog",
		Role:    "operator-assistant",
	})
	if err != nil {
		t.Fatalf("record model tool-plan preview: %v", err)
	}
	acceptance, err := AcceptModelToolPlanPreviewArtifact(root, filepath.Base(preview.Path), ModelToolPlanPreviewAcceptanceOptions{AcceptedBy: "operator"})
	if err != nil {
		t.Fatalf("accept model tool-plan preview: %v", err)
	}
	before := map[string]int{
		"runtime/model-tool-plan-previews/*.json":                      workflowStatusGlobCount(t, root, "runtime/model-tool-plan-previews/*.json"),
		"runtime/model-tool-plan-preview-acceptances/*.json":           workflowStatusGlobCount(t, root, "runtime/model-tool-plan-preview-acceptances/*.json"),
		"toolchain/compile-preview-reviews/*.json":                     workflowStatusGlobCount(t, root, "toolchain/compile-preview-reviews/*.json"),
		"toolchain/compile-preview-review-execution-dispatches/*.json": workflowStatusGlobCount(t, root, "toolchain/compile-preview-review-execution-dispatches/*.json"),
		"runtime/model-usage/*.json":                                   workflowStatusGlobCount(t, root, "runtime/model-usage/*.json"),
		"runtime/model-generations/*.json":                             workflowStatusGlobCount(t, root, "runtime/model-generations/*.json"),
		"runtime/worker-queue-claims/*.json":                           workflowStatusGlobCount(t, root, "runtime/worker-queue-claims/*.json"),
	}

	model := NewTUIModelForWorkspace(root)
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 2200, Height: 48})
	model = updated.(tui.Model)
	model = openAIDialogInputIfNeeded(t, model)
	for _, ch := range "/previews" {
		typed, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}})
		model = typed.(tui.Model)
	}
	submitting, submitCmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if submitCmd == nil {
		t.Fatalf("expected AI dialog /previews to submit")
	}
	completed, cmd := submitting.Update(submitCmd())
	if cmd != nil {
		t.Fatalf("expected AI dialog /previews update to be synchronous")
	}
	view := completed.View()
	for _, want := range []string{
		"You > /previews",
		"AI > model previews: previews=1",
		"valid=1",
		"completed=1",
		"acceptances=1",
		"pending_acceptance=0",
		"latest_action=enumerate_paths",
		"latest_status=completed",
		"latest_valid=true",
		"review_available=true",
		"stored=false",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected /previews response to contain %q:\n%s", want, view)
		}
	}
	previewsLineStart := strings.Index(view, "AI > model previews:")
	if previewsLineStart < 0 {
		t.Fatalf("expected /previews response line in view:\n%s", view)
	}
	previewsLine := view[previewsLineStart:]
	if previewsLineEnd := strings.Index(previewsLine, "\n"); previewsLineEnd >= 0 {
		previewsLine = previewsLine[:previewsLineEnd]
	}
	for _, forbidden := range []string{
		"AI > error:",
		root,
		preview.Path,
		acceptance.Path,
		review.Path,
		"configure-openai-compatible",
		"provider.example",
		"secret-value",
		"SECRET_ASSISTANT_TEXT_SHOULD_NOT_LEAK",
		"local-openai",
		"gpt-plan",
		"Authorization",
		"Bearer",
	} {
		if strings.Contains(previewsLine, forbidden) {
			t.Fatalf("/previews should not require provider setup, leak paths, provider metadata, or secrets %q:\n%s", forbidden, view)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "replay", "events.db")); err == nil {
		t.Fatalf("/previews must not create replay events.db")
	} else if !os.IsNotExist(err) {
		t.Fatalf("stat replay events.db: %v", err)
	}
	for pattern, want := range before {
		if got := workflowStatusGlobCount(t, root, pattern); got != want {
			t.Fatalf("/previews must not create artifacts matching %s, before=%d after=%d", pattern, want, got)
		}
	}
}

func TestNewTUIModelForWorkspaceAIDialogReviewGatesCommandDoesNotRequireProvider(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	t.Setenv("OURO_TEST_PROVIDER_KEY", "secret-value")
	review, err := CreateToolCompilePreviewReview(root, ToolCompilePreviewOptions{Action: compiler.ActionEnumeratePaths})
	if err != nil {
		t.Fatalf("create compile-preview review: %v", err)
	}
	preview, err := RecordModelToolPlanPreviewArtifact(root, "run-ai-gate-summary", ProviderGenerationResult{
		ProviderName: "local-openai",
		Model:        "gpt-gates",
		Text:         "semantic_action: enumerate_paths\nSECRET_GATE_TEXT_SHOULD_NOT_LEAK",
	}, "", ModelToolPlanPreviewExtraction{
		Found:             true,
		Action:            compiler.ActionEnumeratePaths,
		Valid:             true,
		Reason:            "accepted",
		MarkerPreview:     "semantic_action: enumerate_paths",
		ModelOutputDigest: "sha256:gate-output",
	}, review, ModelToolPlanPreviewArtifactOptions{
		Source:  "tui-ai-dialog",
		AgentID: "tui-ai-dialog",
		Role:    "operator-assistant",
	})
	if err != nil {
		t.Fatalf("record model tool-plan preview: %v", err)
	}
	acceptance, err := AcceptModelToolPlanPreviewArtifact(root, filepath.Base(preview.Path), ModelToolPlanPreviewAcceptanceOptions{AcceptedBy: "operator"})
	if err != nil {
		t.Fatalf("accept model tool-plan preview: %v", err)
	}
	before := map[string]int{
		"runtime/model-tool-plan-previews/*.json":                      workflowStatusGlobCount(t, root, "runtime/model-tool-plan-previews/*.json"),
		"runtime/model-tool-plan-preview-acceptances/*.json":           workflowStatusGlobCount(t, root, "runtime/model-tool-plan-preview-acceptances/*.json"),
		"toolchain/compile-preview-reviews/*.json":                     workflowStatusGlobCount(t, root, "toolchain/compile-preview-reviews/*.json"),
		"toolchain/compile-preview-review-execution-dispatches/*.json": workflowStatusGlobCount(t, root, "toolchain/compile-preview-review-execution-dispatches/*.json"),
		"toolchain/compile-preview-review-executions/*.json":           workflowStatusGlobCount(t, root, "toolchain/compile-preview-review-executions/*.json"),
		"reports/report-projection-dispatches/*.json":                  workflowStatusGlobCount(t, root, "reports/report-projection-dispatches/*.json"),
		"reports/report-projection-dispatch-executions/*.json":         workflowStatusGlobCount(t, root, "reports/report-projection-dispatch-executions/*.json"),
		"runtime/model-usage/*.json":                                   workflowStatusGlobCount(t, root, "runtime/model-usage/*.json"),
		"runtime/model-generations/*.json":                             workflowStatusGlobCount(t, root, "runtime/model-generations/*.json"),
		"runtime/worker-queue-claims/*.json":                           workflowStatusGlobCount(t, root, "runtime/worker-queue-claims/*.json"),
	}

	model := NewTUIModelForWorkspace(root)
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 2200, Height: 48})
	model = updated.(tui.Model)
	model = openAIDialogInputIfNeeded(t, model)
	for _, ch := range "/gates" {
		typed, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}})
		model = typed.(tui.Model)
	}
	submitting, submitCmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if submitCmd == nil {
		t.Fatalf("expected AI dialog /gates to submit")
	}
	completed, cmd := submitting.Update(submitCmd())
	if cmd != nil {
		t.Fatalf("expected AI dialog /gates update to be synchronous")
	}
	view := completed.View()
	for _, want := range []string{
		"You > /gates",
		"AI > review gates: status=ready",
		"stage=model-tool-plan",
		"next_gate=create-compile-preview-review-execution-dispatch",
		"reason=current-review-awaits-dispatch",
		"action=enumerate_paths",
		"previews=1",
		"acceptances=1",
		"compile_dispatches=0",
		"compile_executions=0",
		"report_dispatches=0",
		"report_executions=0",
		"report_selected=false",
		"safety_errors=0",
		"command_available=true",
		"stored=false",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected /gates response to contain %q:\n%s", want, view)
		}
	}
	gatesLineStart := strings.Index(view, "AI > review gates:")
	if gatesLineStart < 0 {
		t.Fatalf("expected /gates response line in view:\n%s", view)
	}
	gatesLine := view[gatesLineStart:]
	if gatesLineEnd := strings.Index(gatesLine, "\n"); gatesLineEnd >= 0 {
		gatesLine = gatesLine[:gatesLineEnd]
	}
	for _, forbidden := range []string{
		"AI > error:",
		root,
		preview.Path,
		acceptance.Path,
		review.Path,
		"configure-openai-compatible",
		"provider.example",
		"secret-value",
		"SECRET_GATE_TEXT_SHOULD_NOT_LEAK",
		"local-openai",
		"gpt-gates",
		"Authorization",
		"Bearer",
	} {
		if strings.Contains(gatesLine, forbidden) {
			t.Fatalf("/gates should not require provider setup, leak paths, provider metadata, or secrets %q:\n%s", forbidden, view)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "replay", "events.db")); err == nil {
		t.Fatalf("/gates must not create replay events.db")
	} else if !os.IsNotExist(err) {
		t.Fatalf("stat replay events.db: %v", err)
	}
	for pattern, want := range before {
		if got := workflowStatusGlobCount(t, root, pattern); got != want {
			t.Fatalf("/gates must not create artifacts matching %s, before=%d after=%d", pattern, want, got)
		}
	}
}

func TestNewTUIModelForWorkspaceAIDialogRunsCommandDoesNotRequireProvider(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	ctx := context.Background()
	store, err := eventlog.OpenSQLite(filepath.Join(root, "replay", "events.db"))
	if err != nil {
		t.Fatalf("open replay store: %v", err)
	}
	if _, err := store.Append(ctx, eventlog.Event{RunID: "run-a", Type: eventlog.EventGoal, Payload: json.RawMessage(`{"goal":"a"}`)}); err != nil {
		t.Fatalf("append run-a: %v", err)
	}
	if _, err := store.Append(ctx, eventlog.Event{RunID: "run-b", Type: eventlog.EventGoal, Payload: json.RawMessage(`{"goal":"b"}`)}); err != nil {
		t.Fatalf("append run-b: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close replay store: %v", err)
	}
	t.Setenv("OURO_TEST_PROVIDER_KEY", "secret-value")

	readOnlyBefore, err := eventlog.OpenSQLiteReadOnly(filepath.Join(root, "replay", "events.db"))
	if err != nil {
		t.Fatalf("open replay store read-only before: %v", err)
	}
	runIDsBefore, err := readOnlyBefore.ListRunIDs(ctx)
	if err != nil {
		t.Fatalf("list run ids before: %v", err)
	}
	if err := readOnlyBefore.Close(); err != nil {
		t.Fatalf("close read-only before: %v", err)
	}

	model := NewTUIModelForWorkspace(root)
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 2200, Height: 48})
	model = updated.(tui.Model)
	model = openAIDialogInputIfNeeded(t, model)
	for _, ch := range "/runs" {
		typed, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}})
		model = typed.(tui.Model)
	}
	submitting, submitCmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if submitCmd == nil {
		t.Fatalf("expected AI dialog /runs to submit")
	}
	completed, cmd := submitting.Update(submitCmd())
	if cmd != nil {
		t.Fatalf("expected AI dialog /runs update to be synchronous")
	}
	view := completed.View()
	for _, want := range []string{
		"You > /runs",
		"AI > replay runs:",
		"runs=2",
		"latest=run-b",
		"stored=false",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected /runs response to contain %q:\n%s", want, view)
		}
	}
	runsLineStart := strings.Index(view, "AI > replay runs:")
	if runsLineStart < 0 {
		t.Fatalf("expected /runs response line in view:\n%s", view)
	}
	runsLine := view[runsLineStart:]
	if runsLineEnd := strings.Index(runsLine, "\n"); runsLineEnd >= 0 {
		runsLine = runsLine[:runsLineEnd]
	}
	for _, forbidden := range []string{
		"AI > error:",
		root,
		"configure-openai-compatible",
		"provider.example",
		"secret-value",
	} {
		if strings.Contains(runsLine, forbidden) {
			t.Fatalf("/runs should not require provider setup, leak paths, or leak secrets %q:\n%s", forbidden, view)
		}
	}

	readOnlyAfter, err := eventlog.OpenSQLiteReadOnly(filepath.Join(root, "replay", "events.db"))
	if err != nil {
		t.Fatalf("open replay store read-only after: %v", err)
	}
	runIDsAfter, err := readOnlyAfter.ListRunIDs(ctx)
	if err != nil {
		t.Fatalf("list run ids after: %v", err)
	}
	if err := readOnlyAfter.Close(); err != nil {
		t.Fatalf("close read-only after: %v", err)
	}
	if strings.Join(runIDsAfter, ",") != strings.Join(runIDsBefore, ",") {
		t.Fatalf("/runs must not mutate replay run ids: before=%v after=%v", runIDsBefore, runIDsAfter)
	}
	for _, pattern := range []string{
		"runtime/model-usage/*.json",
		"runtime/model-generations/*.json",
		"runtime/model-tool-plan-previews/*.json",
		"toolchain/compile-preview-reviews/*.json",
		"toolchain/compile-preview-review-execution-dispatches/*.json",
		"runtime/worker-queue-claims/*.json",
	} {
		if got := workflowStatusGlobCount(t, root, pattern); got != 0 {
			t.Fatalf("/runs must not create artifacts matching %s, got %d", pattern, got)
		}
	}
}

func TestNewTUIModelForWorkspaceAIDialogEvidenceCommandDoesNotRequireProvider(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	ctx := context.Background()
	store, err := eventlog.OpenSQLite(filepath.Join(root, "replay", "events.db"))
	if err != nil {
		t.Fatalf("open replay store: %v", err)
	}
	runID := "run-evidence-secret"
	for _, event := range []eventlog.Event{
		{RunID: runID, Type: eventlog.EventGoal, Payload: json.RawMessage(`{"goal":"assess https://target.secret.example/admin"}`)},
		{RunID: runID, Type: eventlog.EventFact, Payload: json.RawMessage(`{"fact_id":"fact-target-secret","kind":"target","statement":"https://target.secret.example/admin","confidence":1,"source":"scope"}`)},
		{RunID: runID, Type: eventlog.EventFact, Payload: json.RawMessage(`{"fact_id":"fact-finding-secret","kind":"finding","statement":"SQL injection at https://target.secret.example/login?token=secret-token","confidence":0.95,"source":"parser.sqlmap_json"}`)},
		{RunID: runID, Type: eventlog.EventFact, Payload: json.RawMessage(`{"fact_id":"fact-surface-secret","kind":"surface","statement":"api.secret.internal:443","confidence":0.8,"source":"parser.httpx_jsonl"}`)},
		{RunID: runID, Type: eventlog.EventFact, Payload: json.RawMessage(`{"fact_id":"fact-artifact-secret","kind":"artifact","statement":"runtime/attempts/secret-output.json","confidence":1,"source":"artifact"}`)},
		{RunID: runID, Type: eventlog.EventIntent, Payload: json.RawMessage(`{"intent_id":"intent-secret","kind":"auth-check","goal":"prove secret admin path","source_facts":["fact-target-secret"],"result_fact":"fact-finding-secret","state":"Open"}`)},
		{RunID: runID, Type: eventlog.EventHint, Payload: json.RawMessage(`{"hint_id":"hint-secret","kind":"operator","message":"prioritize secret admin path","source":"operator"}`)},
	} {
		if _, err := store.Append(ctx, event); err != nil {
			t.Fatalf("append evidence event %s: %v", event.Type, err)
		}
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close replay store: %v", err)
	}
	t.Setenv("OURO_TEST_PROVIDER_KEY", "secret-value")

	readOnlyBefore, err := eventlog.OpenSQLiteReadOnly(filepath.Join(root, "replay", "events.db"))
	if err != nil {
		t.Fatalf("open replay store read-only before: %v", err)
	}
	runIDsBefore, err := readOnlyBefore.ListRunIDs(ctx)
	if err != nil {
		t.Fatalf("list run ids before: %v", err)
	}
	eventsBefore, err := readOnlyBefore.Replay(ctx, runID)
	if err != nil {
		t.Fatalf("replay events before: %v", err)
	}
	if err := readOnlyBefore.Close(); err != nil {
		t.Fatalf("close read-only before: %v", err)
	}

	before := map[string]int{}
	for _, pattern := range []string{
		"runtime/model-usage/*.json",
		"runtime/model-generations/*.json",
		"runtime/model-tool-plan-previews/*.json",
		"toolchain/compile-preview-reviews/*.json",
		"toolchain/compile-preview-review-execution-dispatches/*.json",
		"runtime/worker-queue-claims/*.json",
		"reports/report-projection-dispatches/*.json",
		"reports/report-projection-dispatch-executions/*.json",
		"reports/*.md",
		"reports/current-report.json",
	} {
		before[pattern] = workflowStatusGlobCount(t, root, pattern)
	}

	model := NewTUIModelForWorkspace(root)
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 2200, Height: 48})
	model = updated.(tui.Model)
	model = openAIDialogInputIfNeeded(t, model)
	for _, ch := range "/evidence" {
		typed, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}})
		model = typed.(tui.Model)
	}
	submitting, submitCmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if submitCmd == nil {
		t.Fatalf("expected AI dialog /evidence to submit")
	}
	completed, cmd := submitting.Update(submitCmd())
	if cmd != nil {
		t.Fatalf("expected AI dialog /evidence update to be synchronous")
	}
	view := completed.View()
	for _, want := range []string{
		"You > /evidence",
		"AI > evidence:",
		"runs=1",
		"latest_available=true",
		"events=7",
		"facts=4",
		"intents=1",
		"hints=1",
		"nodes=5",
		"edges=2",
		"findings=1",
		"surfaces=1",
		"artifacts=1",
		"verified_findings=1",
		"paths=1",
		"evidence_refs=1",
		"exhausted=0",
		"suggestions=0",
		"stored=false",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected /evidence response to contain %q:\n%s", want, view)
		}
	}
	evidenceLineStart := strings.Index(view, "AI > evidence:")
	if evidenceLineStart < 0 {
		t.Fatalf("expected /evidence response line in view:\n%s", view)
	}
	evidenceLine := view[evidenceLineStart:]
	if evidenceLineEnd := strings.Index(evidenceLine, "\n"); evidenceLineEnd >= 0 {
		evidenceLine = evidenceLine[:evidenceLineEnd]
	}
	for _, forbidden := range []string{
		"AI > error:",
		root,
		runID,
		"fact-target-secret",
		"fact-finding-secret",
		"fact-surface-secret",
		"fact-artifact-secret",
		"intent-secret",
		"hint-secret",
		"https://target.secret.example",
		"secret-token",
		"api.secret.internal",
		"runtime/attempts/secret-output.json",
		"prioritize secret admin path",
		"configure-openai-compatible",
		"secret-value",
		"Authorization",
		"Bearer",
	} {
		if strings.Contains(evidenceLine, forbidden) {
			t.Fatalf("/evidence should not require provider setup, leak paths, or leak evidence secrets %q:\n%s", forbidden, view)
		}
	}

	readOnlyAfter, err := eventlog.OpenSQLiteReadOnly(filepath.Join(root, "replay", "events.db"))
	if err != nil {
		t.Fatalf("open replay store read-only after: %v", err)
	}
	runIDsAfter, err := readOnlyAfter.ListRunIDs(ctx)
	if err != nil {
		t.Fatalf("list run ids after: %v", err)
	}
	eventsAfter, err := readOnlyAfter.Replay(ctx, runID)
	if err != nil {
		t.Fatalf("replay events after: %v", err)
	}
	if err := readOnlyAfter.Close(); err != nil {
		t.Fatalf("close read-only after: %v", err)
	}
	if strings.Join(runIDsBefore, "\n") != strings.Join(runIDsAfter, "\n") {
		t.Fatalf("/evidence must not mutate replay run ids: before=%v after=%v", runIDsBefore, runIDsAfter)
	}
	if len(eventsAfter) != len(eventsBefore) {
		t.Fatalf("/evidence must not mutate replay events: before=%d after=%d", len(eventsBefore), len(eventsAfter))
	}
	for pattern, want := range before {
		if got := workflowStatusGlobCount(t, root, pattern); got != want {
			t.Fatalf("/evidence must not create artifacts matching %s, before=%d after=%d", pattern, want, got)
		}
	}
}

func TestNewTUIModelForWorkspaceAIDialogMissionCommandDoesNotRequireProvider(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	mission := `version: 1
mission:
  name: secret mission name
  goal: "find auth bypass in https://secret.example/app?token=secret-token"
`
	if err := os.WriteFile(filepath.Join(root, "mission.yaml"), []byte(mission), 0o644); err != nil {
		t.Fatalf("write mission: %v", err)
	}
	scope := `version: 1
scope:
  target_policy: enforce
  targets:
    - kind: url
      value: https://secret.example/app?token=secret-token
    - kind: domain
      value: corp.secret.example
    - kind: host
      value: admin.secret.internal
    - kind: cidr
      value: 10.10.0.0/16
    - kind: artifact
      value: secrets/mobile.apk
    - kind: challenge
      value: ctf-secret-01
  exclusions:
    - https://secret.example/private
    - corp.secret.example
execution:
  isolation: process
`
	if err := os.WriteFile(filepath.Join(root, "scope.yaml"), []byte(scope), 0o644); err != nil {
		t.Fatalf("write scope: %v", err)
	}
	t.Setenv("OURO_TEST_PROVIDER_KEY", "secret-value")

	model := NewTUIModelForWorkspace(root)
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 2200, Height: 48})
	model = updated.(tui.Model)
	model = openAIDialogInputIfNeeded(t, model)
	for _, ch := range "/mission" {
		typed, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}})
		model = typed.(tui.Model)
	}
	submitting, submitCmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if submitCmd == nil {
		t.Fatalf("expected AI dialog /mission to submit")
	}
	completed, cmd := submitting.Update(submitCmd())
	if cmd != nil {
		t.Fatalf("expected AI dialog /mission update to be synchronous")
	}
	view := completed.View()
	for _, want := range []string{
		"You > /mission",
		"AI > mission:",
		"configured=true",
		"goal_set=true",
		"target_policy=enforce",
		"isolation=process",
		"targets=6",
		"urls=1",
		"domains=1",
		"hosts=1",
		"cidrs=1",
		"artifacts=1",
		"challenges=1",
		"exclusions=2",
		"stored=false",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected /mission response to contain %q:\n%s", want, view)
		}
	}
	missionLineStart := strings.Index(view, "AI > mission:")
	if missionLineStart < 0 {
		t.Fatalf("expected /mission response line in view:\n%s", view)
	}
	missionLine := view[missionLineStart:]
	if missionLineEnd := strings.Index(missionLine, "\n"); missionLineEnd >= 0 {
		missionLine = missionLine[:missionLineEnd]
	}
	for _, forbidden := range []string{
		"AI > error:",
		root,
		"secret mission name",
		"find auth bypass",
		"secret.example",
		"secret-token",
		"admin.secret.internal",
		"10.10.0.0/16",
		"secrets/mobile.apk",
		"ctf-secret-01",
		"configure-openai-compatible",
		"secret-value",
		"Authorization",
		"Bearer",
	} {
		if strings.Contains(missionLine, forbidden) {
			t.Fatalf("/mission should not require provider setup, leak paths, or leak mission/scope secrets %q:\n%s", forbidden, view)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "replay", "events.db")); err == nil {
		t.Fatalf("/mission must not create replay events.db")
	} else if !os.IsNotExist(err) {
		t.Fatalf("stat replay events.db: %v", err)
	}
	for _, pattern := range []string{
		"runtime/model-usage/*.json",
		"runtime/model-generations/*.json",
		"runtime/model-tool-plan-previews/*.json",
		"toolchain/compile-preview-reviews/*.json",
		"toolchain/compile-preview-review-execution-dispatches/*.json",
		"runtime/worker-queue-claims/*.json",
		"reports/report-projection-dispatches/*.json",
		"reports/report-projection-dispatch-executions/*.json",
		"reports/*.md",
		"reports/current-report.json",
	} {
		if got := workflowStatusGlobCount(t, root, pattern); got != 0 {
			t.Fatalf("/mission must not create artifacts matching %s, got %d", pattern, got)
		}
	}
}

func TestNewTUIModelForWorkspaceAIDialogReportsCommandDoesNotRequireProvider(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	ctx := context.Background()
	run, err := RunWorkspaceWithOptions(ctx, root, RunOptions{DeferReportProjection: true})
	if err != nil {
		t.Fatalf("run workspace with deferred report projection: %v", err)
	}
	dispatch, err := CreateReportProjectionDispatch(ctx, root, run.RunID, ReportProjectionDispatchOptions{
		Actor:  "test",
		Reason: "test report dispatch",
	})
	if err != nil {
		t.Fatalf("create report projection dispatch: %v", err)
	}
	execution, err := ExecuteReportProjectionDispatch(ctx, root, dispatch.Path, ReportProjectionDispatchExecutionOptions{
		Execute: true,
		Actor:   "test",
		Reason:  "test report execution",
	})
	if err != nil {
		t.Fatalf("execute report projection dispatch: %v", err)
	}
	if _, err := AnnotateReportProjectionDispatchExecution(root, execution.Path, ReportProjectionDispatchExecutionAnnotationOptions{
		Decision: "accepted",
		Reviewer: "test",
		Reason:   "test report review",
	}); err != nil {
		t.Fatalf("annotate report projection execution: %v", err)
	}
	selection, err := SelectReportProjectionDispatchExecution(root, filepath.Base(execution.Path), ReportProjectionDispatchExecutionSelectionOptions{
		SelectedBy: "test",
		Reason:     "test current report",
	})
	if err != nil {
		t.Fatalf("select report projection execution: %v", err)
	}
	t.Setenv("OURO_TEST_PROVIDER_KEY", "secret-value")
	dispatchesBefore := workflowStatusGlobCount(t, root, "reports/report-projection-dispatches/*.json")
	executionsBefore := workflowStatusGlobCount(t, root, "reports/report-projection-dispatch-executions/*.json")
	reportsBefore := workflowStatusGlobCount(t, root, "reports/*.md")
	artifactPatterns := []string{
		"runtime/model-usage/*.json",
		"runtime/model-generations/*.json",
		"runtime/model-tool-plan-previews/*.json",
		"toolchain/compile-preview-reviews/*.json",
		"toolchain/compile-preview-review-execution-dispatches/*.json",
		"runtime/worker-queue-claims/*.json",
	}
	artifactCountsBefore := make(map[string]int, len(artifactPatterns))
	for _, pattern := range artifactPatterns {
		artifactCountsBefore[pattern] = workflowStatusGlobCount(t, root, pattern)
	}

	model := NewTUIModelForWorkspace(root)
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 2200, Height: 48})
	model = updated.(tui.Model)
	model = openAIDialogInputIfNeeded(t, model)
	for _, ch := range "/reports" {
		typed, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}})
		model = typed.(tui.Model)
	}
	submitting, submitCmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if submitCmd == nil {
		t.Fatalf("expected AI dialog /reports to submit")
	}
	completed, cmd := submitting.Update(submitCmd())
	if cmd != nil {
		t.Fatalf("expected AI dialog /reports update to be synchronous")
	}
	view := completed.View()
	for _, want := range []string{
		"You > /reports",
		"AI > reports:",
		"dispatches=1",
		"executions=1",
		"current=true",
		"run=" + run.RunID,
		fmt.Sprintf("events=%d", selection.EventCount),
		"written=true",
		"stored=false",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected /reports response to contain %q:\n%s", want, view)
		}
	}
	reportsLineStart := strings.Index(view, "AI > reports:")
	if reportsLineStart < 0 {
		t.Fatalf("expected /reports response line in view:\n%s", view)
	}
	reportsLine := view[reportsLineStart:]
	if reportsLineEnd := strings.Index(reportsLine, "\n"); reportsLineEnd >= 0 {
		reportsLine = reportsLine[:reportsLineEnd]
	}
	for _, forbidden := range []string{
		"AI > error:",
		root,
		"reports/",
		"report-projection",
		"configure-openai-compatible",
		"provider.example",
		"secret-value",
	} {
		if strings.Contains(reportsLine, forbidden) {
			t.Fatalf("/reports should not require provider setup, leak paths, or leak secrets %q:\n%s", forbidden, view)
		}
	}
	if got := workflowStatusGlobCount(t, root, "reports/report-projection-dispatches/*.json"); got != dispatchesBefore {
		t.Fatalf("/reports must not create report projection dispatches: before=%d after=%d", dispatchesBefore, got)
	}
	if got := workflowStatusGlobCount(t, root, "reports/report-projection-dispatch-executions/*.json"); got != executionsBefore {
		t.Fatalf("/reports must not create report projection executions: before=%d after=%d", executionsBefore, got)
	}
	if got := workflowStatusGlobCount(t, root, "reports/*.md"); got != reportsBefore {
		t.Fatalf("/reports must not create markdown reports: before=%d after=%d", reportsBefore, got)
	}
	for _, pattern := range artifactPatterns {
		if got := workflowStatusGlobCount(t, root, pattern); got != artifactCountsBefore[pattern] {
			t.Fatalf("/reports must not create artifacts matching %s, before=%d after=%d", pattern, artifactCountsBefore[pattern], got)
		}
	}
}

func TestNewTUIModelForWorkspaceAIDialogToolchainCommandDoesNotRequireProvider(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	check, err := CheckToolchainContracts(root)
	if err != nil {
		t.Fatalf("check toolchain contracts: %v", err)
	}
	t.Setenv("OURO_TEST_PROVIDER_KEY", "secret-value")

	model := NewTUIModelForWorkspace(root)
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 2200, Height: 48})
	model = updated.(tui.Model)
	model = openAIDialogInputIfNeeded(t, model)
	for _, ch := range "/toolchain" {
		typed, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}})
		model = typed.(tui.Model)
	}
	submitting, submitCmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if submitCmd == nil {
		t.Fatalf("expected AI dialog /toolchain to submit")
	}
	completed, cmd := submitting.Update(submitCmd())
	if cmd != nil {
		t.Fatalf("expected AI dialog /toolchain update to be synchronous")
	}
	view := completed.View()
	for _, want := range []string{
		"You > /toolchain",
		"AI > toolchain:",
		"status=" + check.Status,
		fmt.Sprintf("manifests=%d", check.ManifestCount),
		fmt.Sprintf("tools=%d", check.ToolCount),
		fmt.Sprintf("domain_packs=%d", check.DomainPackCount),
		fmt.Sprintf("patterns=%d", check.PatternCount),
		fmt.Sprintf("semantic_actions=%d", check.SemanticActionCount),
		fmt.Sprintf("errors=%d", len(check.Errors)),
		"stored=false",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected /toolchain response to contain %q:\n%s", want, view)
		}
	}
	toolchainLineStart := strings.Index(view, "AI > toolchain:")
	if toolchainLineStart < 0 {
		t.Fatalf("expected /toolchain response line in view:\n%s", view)
	}
	toolchainLine := view[toolchainLineStart:]
	if toolchainLineEnd := strings.Index(toolchainLine, "\n"); toolchainLineEnd >= 0 {
		toolchainLine = toolchainLine[:toolchainLineEnd]
	}
	for _, forbidden := range []string{
		"AI > error:",
		root,
		"toolchains/",
		"configs/",
		"configure-openai-compatible",
		"provider.example",
		"secret-value",
	} {
		if strings.Contains(toolchainLine, forbidden) {
			t.Fatalf("/toolchain should not require provider setup, leak paths, or leak secrets %q:\n%s", forbidden, view)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "replay", "events.db")); err == nil {
		t.Fatalf("/toolchain must not create replay events.db")
	} else if !os.IsNotExist(err) {
		t.Fatalf("stat replay events.db: %v", err)
	}
	for _, pattern := range []string{
		"runtime/model-usage/*.json",
		"runtime/model-generations/*.json",
		"runtime/model-tool-plan-previews/*.json",
		"toolchain/compile-preview-reviews/*.json",
		"toolchain/compile-preview-review-execution-dispatches/*.json",
		"runtime/worker-queue-claims/*.json",
	} {
		if got := workflowStatusGlobCount(t, root, pattern); got != 0 {
			t.Fatalf("/toolchain must not create artifacts matching %s, got %d", pattern, got)
		}
	}
}

func TestNewTUIModelForWorkspaceAIDialogCapabilitiesCommandDoesNotRequireProvider(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	check, err := CheckToolchainContracts(root)
	if err != nil {
		t.Fatalf("check toolchain contracts: %v", err)
	}
	packs := contracts.BuiltInDomainPacks()
	domains := map[string]bool{}
	packCapabilities := 0
	parsers := 0
	strategies := 0
	for _, pack := range packs {
		domains[string(pack.Domain())] = true
		packCapabilities += len(pack.Capabilities())
		parsers += len(pack.Parsers())
		strategies += len(pack.StrategyPatterns())
	}
	semanticCapabilities := 0
	for _, action := range compiler.DeclaredSemanticActions() {
		capabilities, ok := compiler.CapabilitiesForAction(action)
		if !ok {
			t.Fatalf("semantic action has no capabilities: %s", action)
		}
		semanticCapabilities += len(capabilities)
	}
	manifests, err := toolchain.LoadManifests(filepath.Join(root, "toolchain"))
	if err != nil {
		t.Fatalf("load manifests: %v", err)
	}
	manifestCapabilities := 0
	manifestDependencies := 0
	for _, manifest := range manifests {
		manifestCapabilities += len(manifest.Capabilities)
		manifestDependencies += len(manifest.Dependencies)
	}
	tools := toolchain.BuiltInAppSecTools()
	toolCapabilities := 0
	toolCategories := map[string]bool{}
	targetKinds := map[string]bool{}
	for _, builtInTool := range tools {
		toolCapabilities += len(builtInTool.Capabilities)
		if strings.TrimSpace(builtInTool.Category) != "" {
			toolCategories[builtInTool.Category] = true
		}
		if strings.TrimSpace(builtInTool.TargetKind) != "" {
			targetKinds[builtInTool.TargetKind] = true
		}
	}
	bundles, err := ListManagedToolBundles(root)
	if err != nil {
		t.Fatalf("list managed bundles: %v", err)
	}
	managedInstalled := 0
	managedBlocked := 0
	for _, bundle := range bundles {
		switch strings.ToLower(strings.TrimSpace(bundle.Status)) {
		case "installed":
			managedInstalled++
		case "blocked":
			managedBlocked++
		}
	}
	t.Setenv("OURO_TEST_PROVIDER_KEY", "secret-value")

	model := NewTUIModelForWorkspace(root)
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 2200, Height: 48})
	model = updated.(tui.Model)
	model = openAIDialogInputIfNeeded(t, model)
	for _, ch := range "/capabilities" {
		typed, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}})
		model = typed.(tui.Model)
	}
	submitting, submitCmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if submitCmd == nil {
		t.Fatalf("expected AI dialog /capabilities to submit")
	}
	completed, cmd := submitting.Update(submitCmd())
	if cmd != nil {
		t.Fatalf("expected AI dialog /capabilities update to be synchronous")
	}
	view := completed.View()
	for _, want := range []string{
		"You > /capabilities",
		"AI > capabilities:",
		"status=" + check.Status,
		fmt.Sprintf("domains=%d", len(domains)),
		fmt.Sprintf("domain_packs=%d", check.DomainPackCount),
		fmt.Sprintf("pack_capabilities=%d", packCapabilities),
		fmt.Sprintf("parsers=%d", parsers),
		fmt.Sprintf("strategies=%d", strategies),
		fmt.Sprintf("semantic_actions=%d", check.SemanticActionCount),
		fmt.Sprintf("semantic_capabilities=%d", semanticCapabilities),
		fmt.Sprintf("manifests=%d", check.ManifestCount),
		fmt.Sprintf("manifest_capabilities=%d", manifestCapabilities),
		fmt.Sprintf("manifest_dependencies=%d", manifestDependencies),
		fmt.Sprintf("tools=%d", check.ToolCount),
		fmt.Sprintf("tool_capabilities=%d", toolCapabilities),
		fmt.Sprintf("tool_categories=%d", len(toolCategories)),
		fmt.Sprintf("target_kinds=%d", len(targetKinds)),
		fmt.Sprintf("managed_bundles=%d", len(bundles)),
		fmt.Sprintf("managed_installed=%d", managedInstalled),
		fmt.Sprintf("managed_blocked=%d", managedBlocked),
		fmt.Sprintf("patterns=%d", check.PatternCount),
		fmt.Sprintf("errors=%d", len(check.Errors)),
		"stored=false",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected /capabilities response to contain %q:\n%s", want, view)
		}
	}
	capabilitiesLineStart := strings.Index(view, "AI > capabilities:")
	if capabilitiesLineStart < 0 {
		t.Fatalf("expected /capabilities response line in view:\n%s", view)
	}
	capabilitiesLine := view[capabilitiesLineStart:]
	if capabilitiesLineEnd := strings.Index(capabilitiesLine, "\n"); capabilitiesLineEnd >= 0 {
		capabilitiesLine = capabilitiesLine[:capabilitiesLineEnd]
	}
	for _, forbidden := range []string{
		"AI > error:",
		root,
		"toolchains/",
		"configs/",
		"run_command",
		"health_command",
		"install_command",
		"configure-openai-compatible",
		"provider.example",
		"secret-value",
		"Authorization",
		"Bearer",
	} {
		if strings.Contains(capabilitiesLine, forbidden) {
			t.Fatalf("/capabilities should not require provider setup, leak paths, commands, or secrets %q:\n%s", forbidden, view)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "replay", "events.db")); err == nil {
		t.Fatalf("/capabilities must not create replay events.db")
	} else if !os.IsNotExist(err) {
		t.Fatalf("stat replay events.db: %v", err)
	}
	for _, pattern := range []string{
		"runtime/model-usage/*.json",
		"runtime/model-generations/*.json",
		"runtime/model-tool-plan-previews/*.json",
		"toolchain/compile-preview-reviews/*.json",
		"toolchain/compile-preview-review-execution-dispatches/*.json",
		"runtime/worker-queue-claims/*.json",
		"reports/report-projection-dispatches/*.json",
		"reports/report-projection-dispatch-executions/*.json",
		"reports/*.md",
	} {
		if got := workflowStatusGlobCount(t, root, pattern); got != 0 {
			t.Fatalf("/capabilities must not create artifacts matching %s, got %d", pattern, got)
		}
	}
}

func TestNewTUIModelForWorkspaceAIDialogOverviewCommandDoesNotRequireProvider(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	mission := `version: 1
mission:
  name: overview secret mission
  goal: "review https://overview.secret.example/app?token=secret-token"
`
	if err := os.WriteFile(filepath.Join(root, "mission.yaml"), []byte(mission), 0o644); err != nil {
		t.Fatalf("write mission: %v", err)
	}
	scope := `version: 1
scope:
  target_policy: enforce
  targets:
    - kind: url
      value: https://overview.secret.example/app?token=secret-token
    - kind: domain
      value: overview.secret.example
  exclusions:
    - https://overview.secret.example/private
execution:
  isolation: process
`
	if err := os.WriteFile(filepath.Join(root, "scope.yaml"), []byte(scope), 0o644); err != nil {
		t.Fatalf("write scope: %v", err)
	}
	if _, err := ConfigureOpenAICompatibleProvider(root, OpenAICompatibleProviderConfiguration{
		Name:         "local-overview",
		BaseURL:      "https://provider.example/v1",
		Model:        "gpt-overview",
		APIKeyEnv:    "OURO_TEST_PROVIDER_KEY",
		Capabilities: []string{"chat", "tool_planning"},
		Enabled:      true,
	}); err != nil {
		t.Fatalf("configure provider: %v", err)
	}
	t.Setenv("OURO_TEST_PROVIDER_KEY", "secret-value")
	workflow, err := SummarizeWorkspaceWorkflow(root)
	if err != nil {
		t.Fatalf("summarize workflow: %v", err)
	}
	missionSummary, err := tuiAIDialogSummarizeMissionScope(root)
	if err != nil {
		t.Fatalf("summarize mission: %v", err)
	}
	safetySummary, err := tuiAIDialogSafetySummary(root)
	if err != nil {
		t.Fatalf("summarize safety: %v", err)
	}
	providerSummary := tuiAIDialogSummarizeProviders(root)
	capabilitySummary := tuiAIDialogCapabilitiesSummary(root)
	evidenceSummary, err := tuiAIDialogSummarizeEvidence(root)
	if err != nil {
		t.Fatalf("summarize evidence: %v", err)
	}
	reportSummary, err := tuiAIDialogSummarizeReports(root)
	if err != nil {
		t.Fatalf("summarize reports: %v", err)
	}
	workerSummary, err := tuiAIDialogSummarizeWorkers(root)
	if err != nil {
		t.Fatalf("summarize workers: %v", err)
	}

	model := NewTUIModelForWorkspace(root)
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 2200, Height: 48})
	model = updated.(tui.Model)
	model = openAIDialogInputIfNeeded(t, model)
	for _, ch := range "/overview" {
		typed, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}})
		model = typed.(tui.Model)
	}
	submitting, submitCmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if submitCmd == nil {
		t.Fatalf("expected AI dialog /overview to submit")
	}
	completed, cmd := submitting.Update(submitCmd())
	if cmd != nil {
		t.Fatalf("expected AI dialog /overview update to be synchronous")
	}
	view := completed.View()
	for _, want := range []string{
		"You > /overview",
		"AI > overview:",
		"workflow_status=" + workflowStatusRowValue(workflow.Status, "unknown"),
		"stage=" + workflowStatusRowValue(workflow.Stage, "unknown"),
		"next_gate=" + workflowStatusRowValue(workflow.NextGate, "none"),
		fmt.Sprintf("mission_configured=%t", missionSummary.Configured),
		"target_policy=enforce",
		"isolation=process",
		"targets=2",
		fmt.Sprintf("safety_ready=%t", safetySummary.Ready),
		fmt.Sprintf("strict_ready=%t", safetySummary.StrictReady),
		"providers_status=" + workflowStatusRowValue(providerSummary.Status, "unknown"),
		fmt.Sprintf("providers=%d", providerSummary.ProviderCount),
		fmt.Sprintf("provider_enabled=%d", providerSummary.EnabledCount),
		fmt.Sprintf("chat=%t", providerSummary.Chat),
		fmt.Sprintf("tool_planning=%t", providerSummary.ToolPlanning),
		"capabilities_status=" + workflowStatusRowValue(capabilitySummary.Status, "blocked"),
		fmt.Sprintf("domain_packs=%d", capabilitySummary.DomainPacks),
		fmt.Sprintf("tools=%d", capabilitySummary.Tools),
		fmt.Sprintf("evidence_runs=%d", evidenceSummary.Runs),
		fmt.Sprintf("findings=%d", evidenceSummary.Findings),
		fmt.Sprintf("reports_current=%t", reportSummary.Current),
		fmt.Sprintf("workers_registered=%d", workerSummary.Registered),
		fmt.Sprintf("worker_queue_claims=%d", workerSummary.QueueClaims),
		"stored=false",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected /overview response to contain %q:\n%s", want, view)
		}
	}
	overviewLineStart := strings.Index(view, "AI > overview:")
	if overviewLineStart < 0 {
		t.Fatalf("expected /overview response line in view:\n%s", view)
	}
	overviewLine := view[overviewLineStart:]
	if overviewLineEnd := strings.Index(overviewLine, "\n"); overviewLineEnd >= 0 {
		overviewLine = overviewLine[:overviewLineEnd]
	}
	for _, forbidden := range []string{
		"AI > error:",
		root,
		"overview secret mission",
		"review https://",
		"overview.secret.example",
		"secret-token",
		"provider.example",
		"OURO_TEST_PROVIDER_KEY",
		"gpt-overview",
		"configure-openai-compatible",
		"secret-value",
		"Authorization",
		"Bearer",
		"toolchains/",
		"configs/",
		"run_command",
		"health_command",
		"install_command",
	} {
		if strings.Contains(overviewLine, forbidden) {
			t.Fatalf("/overview should not require provider setup, leak paths, commands, targets, provider metadata, or secrets %q:\n%s", forbidden, view)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "replay", "events.db")); err == nil {
		t.Fatalf("/overview must not create replay events.db")
	} else if !os.IsNotExist(err) {
		t.Fatalf("stat replay events.db: %v", err)
	}
	for _, pattern := range []string{
		"runtime/model-usage/*.json",
		"runtime/model-generations/*.json",
		"runtime/model-tool-plan-previews/*.json",
		"toolchain/compile-preview-reviews/*.json",
		"toolchain/compile-preview-review-execution-dispatches/*.json",
		"runtime/worker-queue-claims/*.json",
		"reports/report-projection-dispatches/*.json",
		"reports/report-projection-dispatch-executions/*.json",
		"reports/*.md",
		"reports/current-report.json",
	} {
		if got := workflowStatusGlobCount(t, root, pattern); got != 0 {
			t.Fatalf("/overview must not create artifacts matching %s, got %d", pattern, got)
		}
	}
}

func TestNewTUIModelForWorkspaceAIDialogSafetyCommandDoesNotRequireProvider(t *testing.T) {
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
		t.Fatalf("write disabled safety scope: %v", err)
	}
	t.Setenv("OURO_TEST_PROVIDER_KEY", "secret-value")

	model := NewTUIModelForWorkspace(root)
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 2200, Height: 48})
	model = updated.(tui.Model)
	model = openAIDialogInputIfNeeded(t, model)
	for _, ch := range "/safety" {
		typed, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}})
		model = typed.(tui.Model)
	}
	submitting, submitCmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if submitCmd == nil {
		t.Fatalf("expected AI dialog /safety to submit")
	}
	completed, cmd := submitting.Update(submitCmd())
	if cmd != nil {
		t.Fatalf("expected AI dialog /safety update to be synchronous")
	}
	view := completed.View()
	for _, want := range []string{
		"You > /safety",
		"AI > safety:",
		"ready=true",
		"strict_ready=false",
		"warnings=4",
		"errors=0",
		"strict_errors=2",
		"bypasses=2",
		"authorized=0",
		"missing=2",
		"expired=0",
		"stored=false",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected /safety response to contain %q:\n%s", want, view)
		}
	}
	safetyLineStart := strings.Index(view, "AI > safety:")
	if safetyLineStart < 0 {
		t.Fatalf("expected /safety response line in view:\n%s", view)
	}
	safetyLine := view[safetyLineStart:]
	if safetyLineEnd := strings.Index(safetyLine, "\n"); safetyLineEnd >= 0 {
		safetyLine = safetyLine[:safetyLineEnd]
	}
	for _, forbidden := range []string{
		"AI > error:",
		root,
		"scope.yaml",
		"authorization.yaml",
		"scope.target_policy",
		"execution.isolation",
		"configure-openai-compatible",
		"provider.example",
		"secret-value",
	} {
		if strings.Contains(safetyLine, forbidden) {
			t.Fatalf("/safety should not require provider setup, leak paths, or leak secrets %q:\n%s", forbidden, view)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "replay", "events.db")); err == nil {
		t.Fatalf("/safety must not create replay events.db")
	} else if !os.IsNotExist(err) {
		t.Fatalf("stat replay events.db: %v", err)
	}
	for _, pattern := range []string{
		"runtime/model-usage/*.json",
		"runtime/model-generations/*.json",
		"runtime/model-tool-plan-previews/*.json",
		"toolchain/compile-preview-reviews/*.json",
		"toolchain/compile-preview-review-execution-dispatches/*.json",
		"runtime/worker-queue-claims/*.json",
	} {
		if got := workflowStatusGlobCount(t, root, pattern); got != 0 {
			t.Fatalf("/safety must not create artifacts matching %s, got %d", pattern, got)
		}
	}
}

func TestNewTUIModelForWorkspaceAIDialogProvidersCommandDoesNotRequireProvider(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	skipIfWindows(t)
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	if _, err := ConfigureOpenAICompatibleProvider(root, OpenAICompatibleProviderConfiguration{
		Name:         "primary",
		BaseURL:      "https://provider.example/v1",
		Model:        "gpt-test",
		APIKeyEnv:    "OURO_TEST_PROVIDER_KEY",
		Enabled:      true,
		Capabilities: []string{"tool_planning", "chat"},
	}); err != nil {
		t.Fatalf("configure provider: %v", err)
	}
	t.Setenv("OURO_TEST_PROVIDER_KEY", "secret-value")

	model := NewTUIModelForWorkspace(root)
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 2200, Height: 48})
	model = updated.(tui.Model)
	model = openAIDialogInputIfNeeded(t, model)
	for _, ch := range "/providers" {
		typed, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}})
		model = typed.(tui.Model)
	}
	submitting, submitCmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if submitCmd == nil {
		t.Fatalf("expected AI dialog /providers to submit")
	}
	completed, cmd := submitting.Update(submitCmd())
	if cmd != nil {
		t.Fatalf("expected AI dialog /providers update to be synchronous")
	}
	view := completed.View()
	for _, want := range []string{
		"You > /providers",
		"AI > providers:",
		"status=ready",
		"reason=openai-compatible-ready",
		"providers=2",
		"enabled=1",
		"routing=first_enabled",
		"first_type=openai_compatible",
		"first_model=gpt-test",
		"chat=true",
		"tool_planning=true",
		"api_key_env_configured=true",
		"api_key_env_set=true",
		"api_key_value_stored=false",
		"stored=false",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected /providers response to contain %q:\n%s", want, view)
		}
	}
	providersLineStart := strings.Index(view, "AI > providers:")
	if providersLineStart < 0 {
		t.Fatalf("expected /providers response line in view:\n%s", view)
	}
	providersLine := view[providersLineStart:]
	if providersLineEnd := strings.Index(providersLine, "\n"); providersLineEnd >= 0 {
		providersLine = providersLine[:providersLineEnd]
	}
	for _, forbidden := range []string{
		"AI > error:",
		root,
		"https://provider.example",
		"OURO_TEST_PROVIDER_KEY",
		"secret-value",
		"configure-openai-compatible",
		"Authorization",
		"Bearer",
	} {
		if strings.Contains(providersLine, forbidden) {
			t.Fatalf("/providers should not require provider setup, leak paths, or leak secrets %q:\n%s", forbidden, view)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "replay", "events.db")); err == nil {
		t.Fatalf("/providers must not create replay events.db")
	} else if !os.IsNotExist(err) {
		t.Fatalf("stat replay events.db: %v", err)
	}
	for _, pattern := range []string{
		"runtime/model-usage/*.json",
		"runtime/model-generations/*.json",
		"runtime/model-tool-plan-previews/*.json",
		"toolchain/compile-preview-reviews/*.json",
		"toolchain/compile-preview-review-execution-dispatches/*.json",
		"runtime/worker-queue-claims/*.json",
	} {
		if got := workflowStatusGlobCount(t, root, pattern); got != 0 {
			t.Fatalf("/providers must not create artifacts matching %s, got %d", pattern, got)
		}
	}
}

func TestNewTUIModelForWorkspaceAIDialogWorkersCommandDoesNotRequireProvider(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	now := time.Now().UTC()
	if _, err := RegisterRuntimeWorker(root, RuntimeWorkerArtifact{
		WorkerID:     "remote-secret-worker",
		Mode:         "remote",
		Status:       "available",
		Endpoint:     "https://worker.example/private?token=worker-token",
		Capabilities: []string{"web.discovery"},
		Labels: map[string]string{
			"api_key": "secret-label-value",
			"region":  "lab",
		},
	}, RuntimeWorkerRegistrationOptions{Now: now}); err != nil {
		t.Fatalf("register runtime worker: %v", err)
	}
	if _, err := RegisterRuntimeWorker(root, RuntimeWorkerArtifact{
		WorkerID:     "stale-secret-worker",
		Mode:         "remote",
		Status:       "busy",
		Endpoint:     "tcp://127.0.0.1:9443",
		Capabilities: []string{"infra.enumeration"},
		Labels:       map[string]string{"token": "stale-secret-label"},
	}, RuntimeWorkerRegistrationOptions{Now: now.Add(-DefaultRuntimeWorkerStaleAfter - time.Minute)}); err != nil {
		t.Fatalf("register stale runtime worker: %v", err)
	}
	if _, err := CreateRuntimeWorkerQueueClaim(root, RuntimeWorkerQueueClaim{
		WorkerID:    "remote-secret-worker",
		RunID:       "run-secret",
		ExecutionID: "tool-secret",
		Tool:        "web-api-pack",
		Capability:  "web.discovery",
	}, RuntimeWorkerQueueClaimOptions{Now: now, LeaseDuration: 10 * time.Minute}); err != nil {
		t.Fatalf("create runtime worker queue claim: %v", err)
	}
	if err := writeWorkerLeaseArtifact(root, WorkerLeaseArtifact{
		Version:     1,
		RunID:       "run-secret",
		ExecutionID: "tool-secret",
		Tool:        "web-api-pack",
		Action:      "discover_assets",
		Capability:  "web.discovery",
		WorkerID:    "remote-secret-worker",
		Status:      "active",
		AcquiredAt:  now.Format(time.RFC3339Nano),
		HeartbeatAt: now.Format(time.RFC3339Nano),
		AttemptDir:  filepath.Join(root, "runtime", "attempts", "secret-attempt"),
	}); err != nil {
		t.Fatalf("write worker lease: %v", err)
	}
	t.Setenv("OURO_TEST_PROVIDER_KEY", "secret-value")

	before := map[string]int{}
	for _, pattern := range []string{
		"runtime/worker-queue-claims/*.json",
		"runtime/worker-queue-dispatch-executions/*.json",
		"runtime/worker-transport-outbox/*/*.json",
		"runtime/worker-queue-dispatch-results/*.json",
		"runtime/model-usage/*.json",
		"runtime/model-generations/*.json",
		"runtime/model-tool-plan-previews/*.json",
		"toolchain/compile-preview-reviews/*.json",
		"toolchain/compile-preview-review-execution-dispatches/*.json",
	} {
		before[pattern] = workflowStatusGlobCount(t, root, pattern)
	}

	model := NewTUIModelForWorkspace(root)
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 2200, Height: 48})
	model = updated.(tui.Model)
	model = openAIDialogInputIfNeeded(t, model)
	for _, ch := range "/workers" {
		typed, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}})
		model = typed.(tui.Model)
	}
	submitting, submitCmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if submitCmd == nil {
		t.Fatalf("expected AI dialog /workers to submit")
	}
	completed, cmd := submitting.Update(submitCmd())
	if cmd != nil {
		t.Fatalf("expected AI dialog /workers update to be synchronous")
	}
	view := completed.View()
	for _, want := range []string{
		"You > /workers",
		"AI > workers:",
		"registered=2",
		"available=1",
		"busy=0",
		"draining=0",
		"offline=0",
		"stale=1",
		"local=0",
		"remote=2",
		"queue_claims=1",
		"queue_claimed=1",
		"queue_dispatched=0",
		"queue_completed=0",
		"queue_failed=0",
		"dispatch_executions=0",
		"transport_requests=0",
		"dispatch_results=0",
		"leases=1",
		"active_leases=1",
		"stale_leases=0",
		"completed_leases=0",
		"failed_leases=0",
		"reclaimed_leases=0",
		"stored=false",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected /workers response to contain %q:\n%s", want, view)
		}
	}
	workersLineStart := strings.Index(view, "AI > workers:")
	if workersLineStart < 0 {
		t.Fatalf("expected /workers response line in view:\n%s", view)
	}
	workersLine := view[workersLineStart:]
	if workersLineEnd := strings.Index(workersLine, "\n"); workersLineEnd >= 0 {
		workersLine = workersLine[:workersLineEnd]
	}
	for _, forbidden := range []string{
		"AI > error:",
		root,
		"remote-secret-worker",
		"stale-secret-worker",
		"https://worker.example",
		"tcp://127.0.0.1:9443",
		"worker-token",
		"secret-label-value",
		"stale-secret-label",
		"run-secret",
		"tool-secret",
		"secret-attempt",
		"secret-value",
		"configure-openai-compatible",
		"Authorization",
		"Bearer",
	} {
		if strings.Contains(workersLine, forbidden) {
			t.Fatalf("/workers should not require provider setup, leak paths, or leak runtime secrets %q:\n%s", forbidden, view)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "replay", "events.db")); err == nil {
		t.Fatalf("/workers must not create replay events.db")
	} else if !os.IsNotExist(err) {
		t.Fatalf("stat replay events.db: %v", err)
	}
	for pattern, want := range before {
		if got := workflowStatusGlobCount(t, root, pattern); got != want {
			t.Fatalf("/workers must not create artifacts matching %s, before=%d after=%d", pattern, want, got)
		}
	}
}

func TestNewTUIModelForWorkspaceAIDialogContextCommandDoesNotRequireProvider(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	t.Setenv("OURO_TEST_PROVIDER_KEY", "secret-value")

	model := NewTUIModelForWorkspace(root)
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 2200, Height: 48})
	model = updated.(tui.Model)
	model = openAIDialogInputIfNeeded(t, model)
	for _, ch := range "/context" {
		typed, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}})
		model = typed.(tui.Model)
	}
	submitting, submitCmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if submitCmd == nil {
		t.Fatalf("expected AI dialog /context to submit")
	}
	completed, cmd := submitting.Update(submitCmd())
	if cmd != nil {
		t.Fatalf("expected AI dialog /context update to be synchronous")
	}
	view := completed.View()
	for _, want := range []string{
		"You > /context",
		"AI > workspace context:",
		"workspace.context_status=available",
		"workspace.context_mode=read-only",
		"workflow.status=ready",
		"workflow.stage=model-tool-plan",
		"workflow.next_gate=run-provider-tool-plan-preview",
		"workflow.reason=no-current-model-tool-plan",
		"workflow.safety_errors=0",
		"semantic_actions.status=available",
		"semantic_actions.marker_format=semantic_action:<action>",
		"semantic_actions.acceptance=operator_review_required",
		"stored=false",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected /context response to contain %q:\n%s", want, view)
		}
	}
	contextLineStart := strings.Index(view, "AI > workspace context:")
	if contextLineStart < 0 {
		t.Fatalf("expected /context response line in view:\n%s", view)
	}
	contextLine := view[contextLineStart:]
	if contextLineEnd := strings.Index(contextLine, "\n"); contextLineEnd >= 0 {
		contextLine = contextLine[:contextLineEnd]
	}
	for _, forbidden := range []string{
		"AI > error:",
		root,
		"configure-openai-compatible",
		"provider.example",
		"secret-value",
	} {
		if strings.Contains(contextLine, forbidden) {
			t.Fatalf("/context should not require provider setup, leak paths, or leak secrets %q:\n%s", forbidden, view)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "replay", "events.db")); err == nil {
		t.Fatalf("/context must not create replay events.db")
	} else if !os.IsNotExist(err) {
		t.Fatalf("stat replay events.db: %v", err)
	}
	for _, pattern := range []string{
		"runtime/model-usage/*.json",
		"runtime/model-generations/*.json",
		"runtime/model-tool-plan-previews/*.json",
		"toolchain/compile-preview-reviews/*.json",
		"toolchain/compile-preview-review-execution-dispatches/*.json",
		"runtime/worker-queue-claims/*.json",
	} {
		if got := workflowStatusGlobCount(t, root, pattern); got != 0 {
			t.Fatalf("/context must not create artifacts matching %s, got %d", pattern, got)
		}
	}
}

func TestNewTUIModelForWorkspaceAIDialogActionsCommandDoesNotRequireProvider(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	t.Setenv("OURO_TEST_PROVIDER_KEY", "secret-value")

	model := NewTUIModelForWorkspace(root)
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 2200, Height: 48})
	model = updated.(tui.Model)
	model = openAIDialogInputIfNeeded(t, model)
	for _, ch := range "/actions" {
		typed, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}})
		model = typed.(tui.Model)
	}
	submitting, submitCmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if submitCmd == nil {
		t.Fatalf("expected AI dialog /actions to submit")
	}
	completed, cmd := submitting.Update(submitCmd())
	if cmd != nil {
		t.Fatalf("expected AI dialog /actions update to be synchronous")
	}
	view := completed.View()
	for _, want := range []string{
		"You > /actions",
		"AI > semantic actions:",
		"status=available",
		"count=7",
		"allowed=collect_evidence,discover_assets,enumerate_paths,fingerprint_surface,generate_candidates,produce_report,verify_candidate",
		"marker_format=semantic_action:<action>",
		"mode=preview_only",
		"acceptance=operator_review_required",
		"stored=false",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected /actions response to contain %q:\n%s", want, view)
		}
	}
	actionsLineStart := strings.Index(view, "AI > semantic actions:")
	if actionsLineStart < 0 {
		t.Fatalf("expected /actions response line in view:\n%s", view)
	}
	actionsLine := view[actionsLineStart:]
	if actionsLineEnd := strings.Index(actionsLine, "\n"); actionsLineEnd >= 0 {
		actionsLine = actionsLine[:actionsLineEnd]
	}
	for _, forbidden := range []string{
		"AI > error:",
		root,
		"configure-openai-compatible",
		"provider.example",
		"secret-value",
		"subfinder",
		"httpx",
		"nuclei",
		"RunCommand",
		"--output",
		"toolchain/",
		"Authorization",
		"Bearer",
	} {
		if strings.Contains(actionsLine, forbidden) {
			t.Fatalf("/actions should not require provider setup, leak paths, tools, commands, or secrets %q:\n%s", forbidden, view)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "replay", "events.db")); err == nil {
		t.Fatalf("/actions must not create replay events.db")
	} else if !os.IsNotExist(err) {
		t.Fatalf("stat replay events.db: %v", err)
	}
	for _, pattern := range []string{
		"runtime/model-usage/*.json",
		"runtime/model-generations/*.json",
		"runtime/model-tool-plan-previews/*.json",
		"toolchain/compile-preview-reviews/*.json",
		"toolchain/compile-preview-review-execution-dispatches/*.json",
		"runtime/worker-queue-claims/*.json",
	} {
		if got := workflowStatusGlobCount(t, root, pattern); got != 0 {
			t.Fatalf("/actions must not create artifacts matching %s, got %d", pattern, got)
		}
	}
}

func TestNewTUIModelForWorkspaceAIDialogUsageCommandDoesNotRequireProvider(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	t.Setenv("OURO_TEST_PROVIDER_KEY", "secret-value")
	recordProviderUsageSummaryFixture(t, root)
	usageBefore := workflowStatusGlobCount(t, root, "runtime/model-usage/*.json")

	model := NewTUIModelForWorkspace(root)
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 2200, Height: 48})
	model = updated.(tui.Model)
	model = openAIDialogInputIfNeeded(t, model)
	for _, ch := range "/usage" {
		typed, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}})
		model = typed.(tui.Model)
	}
	submitting, submitCmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if submitCmd == nil {
		t.Fatalf("expected AI dialog /usage to submit")
	}
	completed, cmd := submitting.Update(submitCmd())
	if cmd != nil {
		t.Fatalf("expected AI dialog /usage update to be synchronous")
	}
	view := completed.View()
	for _, want := range []string{
		"You > /usage",
		"AI > provider usage: records=2",
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
		"stored=false",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected /usage response to contain %q:\n%s", want, view)
		}
	}
	usageLineStart := strings.Index(view, "AI > provider usage:")
	if usageLineStart < 0 {
		t.Fatalf("expected /usage response line in view:\n%s", view)
	}
	usageLine := view[usageLineStart:]
	if usageLineEnd := strings.Index(usageLine, "\n"); usageLineEnd >= 0 {
		usageLine = usageLine[:usageLineEnd]
	}
	for _, forbidden := range []string{
		"AI > error:",
		root,
		"configure-openai-compatible",
		"provider.example",
		"secret-value",
		"do not persist generated text",
		"Authorization",
		"Bearer",
	} {
		if strings.Contains(usageLine, forbidden) {
			t.Fatalf("/usage should not require provider setup, leak paths, or leak secrets %q:\n%s", forbidden, view)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "replay", "events.db")); err == nil {
		t.Fatalf("/usage must not create replay events.db")
	} else if !os.IsNotExist(err) {
		t.Fatalf("stat replay events.db: %v", err)
	}
	if usageAfter := workflowStatusGlobCount(t, root, "runtime/model-usage/*.json"); usageAfter != usageBefore {
		t.Fatalf("/usage must not create provider usage records: before=%d after=%d", usageBefore, usageAfter)
	}
	for _, pattern := range []string{
		"runtime/model-generations/*.json",
		"runtime/model-tool-plan-previews/*.json",
		"toolchain/compile-preview-reviews/*.json",
		"toolchain/compile-preview-review-execution-dispatches/*.json",
		"runtime/worker-queue-claims/*.json",
	} {
		if got := workflowStatusGlobCount(t, root, pattern); got != 0 {
			t.Fatalf("/usage must not create artifacts matching %s, got %d", pattern, got)
		}
	}
}

func TestNewTUIModelForWorkspaceAIDialogHelpCommandDoesNotRequireProvider(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}

	model := NewTUIModelForWorkspace(root)
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 2200, Height: 48})
	model = updated.(tui.Model)
	model = openAIDialogInputIfNeeded(t, model)
	for _, ch := range "/help" {
		typed, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}})
		model = typed.(tui.Model)
	}
	submitting, submitCmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if submitCmd == nil {
		t.Fatalf("expected AI dialog /help to submit")
	}
	completed, cmd := submitting.Update(submitCmd())
	if cmd != nil {
		t.Fatalf("expected AI dialog /help update to be synchronous")
	}
	view := completed.View()
	for _, want := range []string{
		"You > /help",
		"AI > local commands: /quickstart /use /runbook /keys /shortcuts /controls /doctor /readiness /health /completion /progress /percent /acceptance /completion-status /final /final-evidence /final-review /audit /audit-completion /production-acceptance /status /workflow /workflow-status /overview /context /actions /usage /providers /workers /evidence /mission /next /command /artifacts /gates /previews /runs /reports /toolchain /capabilities /safety /setup /clear /help; normal prompts call configured provider; stored=false",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected /help response to contain %q:\n%s", want, view)
		}
	}
	for _, forbidden := range []string{
		"AI > error:",
		"provider=local-openai",
	} {
		if strings.Contains(view, forbidden) {
			t.Fatalf("/help should not require provider setup or provider metadata %q:\n%s", forbidden, view)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "replay", "events.db")); err == nil {
		t.Fatalf("/help must not create replay events.db")
	} else if !os.IsNotExist(err) {
		t.Fatalf("stat replay events.db: %v", err)
	}
	for _, pattern := range []string{
		"runtime/model-usage/*.json",
		"runtime/model-generations/*.json",
		"runtime/model-tool-plan-previews/*.json",
		"toolchain/compile-preview-reviews/*.json",
		"toolchain/compile-preview-review-execution-dispatches/*.json",
		"runtime/worker-queue-claims/*.json",
	} {
		if got := workflowStatusGlobCount(t, root, pattern); got != 0 {
			t.Fatalf("/help must not create artifacts matching %s, got %d", pattern, got)
		}
	}
}

func TestNewTUIModelForWorkspaceAIDialogQuickstartCommandDoesNotRequireProvider(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	t.Setenv("OURO_TEST_PROVIDER_KEY", "secret-value")

	model := NewTUIModelForWorkspace(root)
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 2200, Height: 48})
	model = updated.(tui.Model)
	model = openAIDialogInputIfNeeded(t, model)
	for _, ch := range "/quickstart" {
		typed, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}})
		model = typed.(tui.Model)
	}
	submitting, submitCmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if submitCmd == nil {
		t.Fatalf("expected AI dialog /quickstart to submit")
	}
	completed, cmd := submitting.Update(submitCmd())
	if cmd != nil {
		t.Fatalf("expected AI dialog /quickstart update to be synchronous")
	}
	view := completed.View()
	for _, want := range []string{
		"You > /quickstart",
		"AI > quickstart:",
		"init=./bin/ouro init <workspace>",
		"chat=./bin/ouro chat <workspace>",
		"provider=ouro provider configure-openai-compatible <workspace> --name local-openai --base-url <base-url> --model <model> --api-key-env <ENV_VAR> --capability chat --capability tool_planning --enable",
		"smoke=make smoke-ide",
		"commands=/help /overview /setup /status /next /command /evidence /mission /capabilities /workers",
		"api_key_value_stored=false",
		"stored=false",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected /quickstart response to contain %q:\n%s", want, view)
		}
	}
	quickstartLine := ""
	for _, line := range strings.Split(view, "\n") {
		if strings.Contains(line, "AI > quickstart:") {
			quickstartLine = line
			break
		}
	}
	if quickstartLine == "" {
		t.Fatalf("expected /quickstart response line:\n%s", view)
	}
	for _, forbidden := range []string{
		root,
		"secret-value",
		"http://127.0.0.1",
	} {
		if strings.Contains(quickstartLine, forbidden) {
			t.Fatalf("/quickstart response should not leak paths or secrets %q:\n%s", forbidden, quickstartLine)
		}
	}
	if strings.Contains(view, "AI > error:") {
		t.Fatalf("/quickstart should not require provider setup:\n%s", view)
	}
	if _, err := os.Stat(filepath.Join(root, "replay", "events.db")); err == nil {
		t.Fatalf("/quickstart must not create replay events.db")
	} else if !os.IsNotExist(err) {
		t.Fatalf("stat replay events.db: %v", err)
	}
	for _, pattern := range []string{
		"runtime/model-usage/*.json",
		"runtime/model-generations/*.json",
		"runtime/model-tool-plan-previews/*.json",
		"toolchain/compile-preview-reviews/*.json",
		"toolchain/compile-preview-review-execution-dispatches/*.json",
		"runtime/worker-queue-claims/*.json",
	} {
		if got := workflowStatusGlobCount(t, root, pattern); got != 0 {
			t.Fatalf("/quickstart must not create artifacts matching %s, got %d", pattern, got)
		}
	}
}

func TestNewTUIModelForWorkspaceAIDialogKeysCommandDoesNotRequireProvider(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	t.Setenv("OURO_TEST_PROVIDER_KEY", "secret-value")

	model := NewTUIModelForWorkspace(root)
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 2200, Height: 48})
	model = updated.(tui.Model)
	model = openAIDialogInputIfNeeded(t, model)
	for _, ch := range "/keys" {
		typed, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}})
		model = typed.(tui.Model)
	}
	submitting, submitCmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if submitCmd == nil {
		t.Fatalf("expected AI dialog /keys to submit")
	}
	completed, cmd := submitting.Update(submitCmd())
	if cmd != nil {
		t.Fatalf("expected AI dialog /keys update to be synchronous")
	}
	view := completed.View()
	for _, want := range []string{
		"You > /keys",
		"AI > keys:",
		"chat_focus=i",
		"submit=enter",
		"cancel_draft=esc",
		"quit=q|ctrl+c",
		"runtime_select=up|down",
		"runtime_details=d",
		"runtime_execute=x_then_x",
		"runtime_request=r",
		"replay_step=left|right",
		"run_control=c_cancel|p_pause",
		"stored=false",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected /keys response to contain %q:\n%s", want, view)
		}
	}
	keysLine := ""
	for _, line := range strings.Split(view, "\n") {
		if strings.Contains(line, "AI > keys:") {
			keysLine = line
			break
		}
	}
	if keysLine == "" {
		t.Fatalf("expected /keys response line:\n%s", view)
	}
	for _, forbidden := range []string{
		root,
		"secret-value",
		"configure-openai-compatible",
	} {
		if strings.Contains(keysLine, forbidden) {
			t.Fatalf("/keys response should not leak paths or secrets %q:\n%s", forbidden, keysLine)
		}
	}
	if strings.Contains(view, "AI > error:") {
		t.Fatalf("/keys should not require provider setup:\n%s", view)
	}
	if _, err := os.Stat(filepath.Join(root, "replay", "events.db")); err == nil {
		t.Fatalf("/keys must not create replay events.db")
	} else if !os.IsNotExist(err) {
		t.Fatalf("stat replay events.db: %v", err)
	}
	for _, pattern := range []string{
		"runtime/model-usage/*.json",
		"runtime/model-generations/*.json",
		"runtime/model-tool-plan-previews/*.json",
		"toolchain/compile-preview-reviews/*.json",
		"toolchain/compile-preview-review-execution-dispatches/*.json",
		"runtime/worker-queue-claims/*.json",
	} {
		if got := workflowStatusGlobCount(t, root, pattern); got != 0 {
			t.Fatalf("/keys must not create artifacts matching %s, got %d", pattern, got)
		}
	}
}

func TestNewTUIModelForWorkspaceAIDialogDoctorCommandDoesNotRequireProvider(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	skipIfWindows(t)
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	t.Setenv("OURO_TEST_PROVIDER_KEY", "secret-value")

	model := NewTUIModelForWorkspace(root)
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 2200, Height: 48})
	model = updated.(tui.Model)
	model = openAIDialogInputIfNeeded(t, model)
	for _, ch := range "/doctor" {
		typed, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}})
		model = typed.(tui.Model)
	}
	submitting, submitCmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if submitCmd == nil {
		t.Fatalf("expected AI dialog /doctor to submit")
	}
	completed, cmd := submitting.Update(submitCmd())
	if cmd != nil {
		t.Fatalf("expected AI dialog /doctor update to be synchronous")
	}
	view := completed.View()
	for _, want := range []string{
		"You > /doctor",
		"AI > doctor:",
		"workspace_ready=true",
		"strict_ready=true",
		"warnings=0",
		"errors=0",
		"strict_errors=0",
		"safety_bypasses=0",
		"providers_status=needs-provider",
		"provider_enabled=1",
		"chat=false",
		"tool_planning=false",
		"workflow_status=ready",
		"toolchain_status=ready",
		"stored=false",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected /doctor response to contain %q:\n%s", want, view)
		}
	}
	doctorLine := ""
	for _, line := range strings.Split(view, "\n") {
		if strings.Contains(line, "AI > doctor:") {
			doctorLine = line
			break
		}
	}
	if doctorLine == "" {
		t.Fatalf("expected /doctor response line:\n%s", view)
	}
	for _, forbidden := range []string{
		root,
		"secret-value",
		"configure-openai-compatible",
		"http://127.0.0.1",
		"OURO_TEST_PROVIDER_KEY",
	} {
		if strings.Contains(doctorLine, forbidden) {
			t.Fatalf("/doctor response should not leak paths or secrets %q:\n%s", forbidden, doctorLine)
		}
	}
	if strings.Contains(view, "AI > error:") {
		t.Fatalf("/doctor should not require provider setup:\n%s", view)
	}
	if _, err := os.Stat(filepath.Join(root, "replay", "events.db")); err == nil {
		t.Fatalf("/doctor must not create replay events.db")
	} else if !os.IsNotExist(err) {
		t.Fatalf("stat replay events.db: %v", err)
	}
	for _, pattern := range []string{
		"runtime/model-usage/*.json",
		"runtime/model-generations/*.json",
		"runtime/model-tool-plan-previews/*.json",
		"toolchain/compile-preview-reviews/*.json",
		"toolchain/compile-preview-review-execution-dispatches/*.json",
		"runtime/worker-queue-claims/*.json",
		"runtime/worker-transport-requests/*.json",
	} {
		if got := workflowStatusGlobCount(t, root, pattern); got != 0 {
			t.Fatalf("/doctor must not create artifacts matching %s, got %d", pattern, got)
		}
	}
}

func TestNewTUIModelForWorkspaceAIDialogCompletionCommandDoesNotRequireProvider(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	t.Setenv("OURO_TEST_PROVIDER_KEY", "secret-value")

	model := NewTUIModelForWorkspace(root)
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 2200, Height: 48})
	model = updated.(tui.Model)
	model = openAIDialogInputIfNeeded(t, model)
	for _, ch := range "/completion" {
		typed, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}})
		model = typed.(tui.Model)
	}
	submitting, submitCmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if submitCmd == nil {
		t.Fatalf("expected AI dialog /completion to submit")
	}
	completed, cmd := submitting.Update(submitCmd())
	if cmd != nil {
		t.Fatalf("expected AI dialog /completion update to be synchronous")
	}
	view := completed.View()
	for _, want := range []string{
		"You > /completion",
		"AI > completion:",
		"local_ide_usable=true",
		"direct_chat=true",
		"completion_estimate=99.99",
		"claim_100_ready=false",
		"final_review_status=external_evidence_required",
		"production_acceptance_required=true",
		"completion_audit_required=true",
		"attestation_required=true",
		"manifest_review_required=true",
		"operator_acceptance_required=true",
		"stored=false",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected /completion response to contain %q:\n%s", want, view)
		}
	}
	completionLine := ""
	for _, line := range strings.Split(view, "\n") {
		if strings.Contains(line, "AI > completion:") {
			completionLine = line
			break
		}
	}
	if completionLine == "" {
		t.Fatalf("expected /completion response line:\n%s", view)
	}
	for _, forbidden := range []string{
		root,
		"secret-value",
		"configure-openai-compatible",
		"OURO_TEST_PROVIDER_KEY",
		"completion-audit.txt",
		"summary.txt",
	} {
		if strings.Contains(completionLine, forbidden) {
			t.Fatalf("/completion response should not leak paths, commands, or secrets %q:\n%s", forbidden, completionLine)
		}
	}
	if strings.Contains(view, "AI > error:") {
		t.Fatalf("/completion should not require provider setup:\n%s", view)
	}
	if _, err := os.Stat(filepath.Join(root, "replay", "events.db")); err == nil {
		t.Fatalf("/completion must not create replay events.db")
	} else if !os.IsNotExist(err) {
		t.Fatalf("stat replay events.db: %v", err)
	}
	for _, pattern := range []string{
		"runtime/model-usage/*.json",
		"runtime/model-generations/*.json",
		"runtime/model-tool-plan-previews/*.json",
		"toolchain/compile-preview-reviews/*.json",
		"toolchain/compile-preview-review-execution-dispatches/*.json",
		"runtime/worker-queue-claims/*.json",
		"runtime/worker-transport-requests/*.json",
	} {
		if got := workflowStatusGlobCount(t, root, pattern); got != 0 {
			t.Fatalf("/completion must not create artifacts matching %s, got %d", pattern, got)
		}
	}
}

func TestNewTUIModelForWorkspaceAIDialogFinalEvidenceCommandDoesNotRequireProvider(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	t.Setenv("OURO_TEST_PROVIDER_KEY", "secret-value")

	model := NewTUIModelForWorkspace(root)
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 2200, Height: 48})
	model = updated.(tui.Model)
	model = openAIDialogInputIfNeeded(t, model)
	for _, ch := range "/final" {
		typed, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}})
		model = typed.(tui.Model)
	}
	submitting, submitCmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if submitCmd == nil {
		t.Fatalf("expected AI dialog /final to submit")
	}
	completed, cmd := submitting.Update(submitCmd())
	if cmd != nil {
		t.Fatalf("expected AI dialog /final update to be synchronous")
	}
	view := completed.View()
	for _, want := range []string{
		"You > /final",
		"AI > final evidence:",
		"production_acceptance=make_smoke-production-acceptance",
		"inventory=<payload-inventory>",
		"policy=<deployment-policy>",
		"attestation=<completion-attestation>",
		"completion_audit=make_audit-completion",
		"review=manifest_and_digest_sidecar",
		"claim_100_after=operator_acceptance",
		"stored=false",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected /final response to contain %q:\n%s", want, view)
		}
	}
	finalLine := ""
	for _, line := range strings.Split(view, "\n") {
		if strings.Contains(line, "AI > final evidence:") {
			finalLine = line
			break
		}
	}
	if finalLine == "" {
		t.Fatalf("expected /final response line:\n%s", view)
	}
	for _, forbidden := range []string{
		root,
		"secret-value",
		"configure-openai-compatible",
		"OURO_TEST_PROVIDER_KEY",
		"completion-audit.txt",
		"summary.txt",
	} {
		if strings.Contains(finalLine, forbidden) {
			t.Fatalf("/final response should not leak paths, provider setup, output filenames, or secrets %q:\n%s", forbidden, finalLine)
		}
	}
	if strings.Contains(view, "AI > error:") {
		t.Fatalf("/final should not require provider setup:\n%s", view)
	}
	if _, err := os.Stat(filepath.Join(root, "replay", "events.db")); err == nil {
		t.Fatalf("/final must not create replay events.db")
	} else if !os.IsNotExist(err) {
		t.Fatalf("stat replay events.db: %v", err)
	}
	for _, pattern := range []string{
		"runtime/model-usage/*.json",
		"runtime/model-generations/*.json",
		"runtime/model-tool-plan-previews/*.json",
		"toolchain/compile-preview-reviews/*.json",
		"toolchain/compile-preview-review-execution-dispatches/*.json",
		"runtime/worker-queue-claims/*.json",
		"runtime/worker-transport-requests/*.json",
	} {
		if got := workflowStatusGlobCount(t, root, pattern); got != 0 {
			t.Fatalf("/final must not create artifacts matching %s, got %d", pattern, got)
		}
	}
}

func TestNewTUIModelForWorkspaceAIDialogSetupCommandDoesNotRequireProvider(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	skipIfWindows(t)
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	t.Setenv("OURO_TEST_PROVIDER_KEY", "secret-value")

	model := NewTUIModelForWorkspace(root)
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 2200, Height: 48})
	model = updated.(tui.Model)
	model = openAIDialogInputIfNeeded(t, model)
	for _, ch := range "/setup" {
		typed, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}})
		model = typed.(tui.Model)
	}
	submitting, submitCmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if submitCmd == nil {
		t.Fatalf("expected AI dialog /setup to submit")
	}
	completed, cmd := submitting.Update(submitCmd())
	if cmd != nil {
		t.Fatalf("expected AI dialog /setup update to be synchronous")
	}
	view := completed.View()
	for _, want := range []string{
		"You > /setup",
		"AI > AI setup: status=needs-provider reason=enabled-provider-offline provider=offline model=offline-model",
		"configure: ouro provider configure-openai-compatible " + root,
		"--api-key-env <ENV_VAR>",
		"--capability chat --capability tool_planning --enable",
		"api_key_value_stored=false stored=false",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected /setup response to contain %q:\n%s", want, view)
		}
	}
	for _, forbidden := range []string{
		"AI > error:",
		"provider=local-openai",
		"secret-value",
	} {
		if strings.Contains(view, forbidden) {
			t.Fatalf("/setup should not require provider setup, call a provider, or leak secrets %q:\n%s", forbidden, view)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "replay", "events.db")); err == nil {
		t.Fatalf("/setup must not create replay events.db")
	} else if !os.IsNotExist(err) {
		t.Fatalf("stat replay events.db: %v", err)
	}
	for _, pattern := range []string{
		"runtime/model-usage/*.json",
		"runtime/model-generations/*.json",
		"runtime/model-tool-plan-previews/*.json",
		"toolchain/compile-preview-reviews/*.json",
		"toolchain/compile-preview-review-execution-dispatches/*.json",
		"runtime/worker-queue-claims/*.json",
	} {
		if got := workflowStatusGlobCount(t, root, pattern); got != 0 {
			t.Fatalf("/setup must not create artifacts matching %s, got %d", pattern, got)
		}
	}
}

func TestNewTUIModelForWorkspaceAIDialogLocalCommandsDoNotEnterProviderHistory(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	type capturedRequest struct {
		Model    string `json:"model"`
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
	}
	requests := []capturedRequest{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("expected chat completions POST, got %s %s", r.Method, r.URL.Path)
		}
		var captured capturedRequest
		if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
			t.Fatalf("decode provider request: %v", err)
		}
		requests = append(requests, captured)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"choices": [{"message": {"role": "assistant", "content": "Provider history is clean."}}],
			"usage": {"prompt_tokens": 17, "completion_tokens": 4, "total_tokens": 21}
		}`))
	}))
	defer server.Close()
	t.Setenv("OURO_TEST_PROVIDER_KEY", "secret-value")

	model := NewTUIModelForWorkspace(root)
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 2200, Height: 48})
	model = updated.(tui.Model)
	model = openAIDialogInputIfNeeded(t, model)
	for _, ch := range "/setup" {
		typed, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}})
		model = typed.(tui.Model)
	}
	submitting, submitCmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if submitCmd == nil {
		t.Fatalf("expected AI dialog /setup to submit")
	}
	completed, cmd := submitting.Update(submitCmd())
	if cmd != nil {
		t.Fatalf("expected AI dialog /setup update to be synchronous")
	}
	model = completed.(tui.Model)
	if len(requests) != 0 {
		t.Fatalf("/setup must not call provider, got %#v", requests)
	}

	model = openAIDialogInputIfNeeded(t, model)
	for _, ch := range "/quickstart" {
		typed, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}})
		model = typed.(tui.Model)
	}
	submitting, submitCmd = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if submitCmd == nil {
		t.Fatalf("expected AI dialog /quickstart to submit")
	}
	completed, cmd = submitting.Update(submitCmd())
	if cmd != nil {
		t.Fatalf("expected AI dialog /quickstart update to be synchronous")
	}
	model = completed.(tui.Model)
	if len(requests) != 0 {
		t.Fatalf("/quickstart must not call provider, got %#v", requests)
	}

	model = openAIDialogInputIfNeeded(t, model)
	for _, ch := range "/keys" {
		typed, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}})
		model = typed.(tui.Model)
	}
	submitting, submitCmd = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if submitCmd == nil {
		t.Fatalf("expected AI dialog /keys to submit")
	}
	completed, cmd = submitting.Update(submitCmd())
	if cmd != nil {
		t.Fatalf("expected AI dialog /keys update to be synchronous")
	}
	model = completed.(tui.Model)
	if len(requests) != 0 {
		t.Fatalf("/keys must not call provider, got %#v", requests)
	}

	model = openAIDialogInputIfNeeded(t, model)
	for _, ch := range "/doctor" {
		typed, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}})
		model = typed.(tui.Model)
	}
	submitting, submitCmd = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if submitCmd == nil {
		t.Fatalf("expected AI dialog /doctor to submit")
	}
	completed, cmd = submitting.Update(submitCmd())
	if cmd != nil {
		t.Fatalf("expected AI dialog /doctor update to be synchronous")
	}
	model = completed.(tui.Model)
	if len(requests) != 0 {
		t.Fatalf("/doctor must not call provider, got %#v", requests)
	}

	model = openAIDialogInputIfNeeded(t, model)
	for _, ch := range "/completion" {
		typed, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}})
		model = typed.(tui.Model)
	}
	submitting, submitCmd = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if submitCmd == nil {
		t.Fatalf("expected AI dialog /completion to submit")
	}
	completed, cmd = submitting.Update(submitCmd())
	if cmd != nil {
		t.Fatalf("expected AI dialog /completion update to be synchronous")
	}
	model = completed.(tui.Model)
	if len(requests) != 0 {
		t.Fatalf("/completion must not call provider, got %#v", requests)
	}

	model = openAIDialogInputIfNeeded(t, model)
	for _, ch := range "/final" {
		typed, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}})
		model = typed.(tui.Model)
	}
	submitting, submitCmd = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if submitCmd == nil {
		t.Fatalf("expected AI dialog /final to submit")
	}
	completed, cmd = submitting.Update(submitCmd())
	if cmd != nil {
		t.Fatalf("expected AI dialog /final update to be synchronous")
	}
	model = completed.(tui.Model)
	if len(requests) != 0 {
		t.Fatalf("/final must not call provider, got %#v", requests)
	}

	model = openAIDialogInputIfNeeded(t, model)
	for _, ch := range "/workers" {
		typed, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}})
		model = typed.(tui.Model)
	}
	submitting, submitCmd = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if submitCmd == nil {
		t.Fatalf("expected AI dialog /workers to submit")
	}
	completed, cmd = submitting.Update(submitCmd())
	if cmd != nil {
		t.Fatalf("expected AI dialog /workers update to be synchronous")
	}
	model = completed.(tui.Model)
	if len(requests) != 0 {
		t.Fatalf("/workers must not call provider, got %#v", requests)
	}

	model = openAIDialogInputIfNeeded(t, model)
	for _, ch := range "/evidence" {
		typed, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}})
		model = typed.(tui.Model)
	}
	submitting, submitCmd = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if submitCmd == nil {
		t.Fatalf("expected AI dialog /evidence to submit")
	}
	completed, cmd = submitting.Update(submitCmd())
	if cmd != nil {
		t.Fatalf("expected AI dialog /evidence update to be synchronous")
	}
	model = completed.(tui.Model)
	if len(requests) != 0 {
		t.Fatalf("/evidence must not call provider, got %#v", requests)
	}

	model = openAIDialogInputIfNeeded(t, model)
	for _, ch := range "/mission" {
		typed, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}})
		model = typed.(tui.Model)
	}
	submitting, submitCmd = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if submitCmd == nil {
		t.Fatalf("expected AI dialog /mission to submit")
	}
	completed, cmd = submitting.Update(submitCmd())
	if cmd != nil {
		t.Fatalf("expected AI dialog /mission update to be synchronous")
	}
	model = completed.(tui.Model)
	if len(requests) != 0 {
		t.Fatalf("/mission must not call provider, got %#v", requests)
	}

	model = openAIDialogInputIfNeeded(t, model)
	for _, ch := range "/capabilities" {
		typed, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}})
		model = typed.(tui.Model)
	}
	submitting, submitCmd = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if submitCmd == nil {
		t.Fatalf("expected AI dialog /capabilities to submit")
	}
	completed, cmd = submitting.Update(submitCmd())
	if cmd != nil {
		t.Fatalf("expected AI dialog /capabilities update to be synchronous")
	}
	model = completed.(tui.Model)
	if len(requests) != 0 {
		t.Fatalf("/capabilities must not call provider, got %#v", requests)
	}

	model = openAIDialogInputIfNeeded(t, model)
	for _, ch := range "/overview" {
		typed, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}})
		model = typed.(tui.Model)
	}
	submitting, submitCmd = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if submitCmd == nil {
		t.Fatalf("expected AI dialog /overview to submit")
	}
	completed, cmd = submitting.Update(submitCmd())
	if cmd != nil {
		t.Fatalf("expected AI dialog /overview update to be synchronous")
	}
	model = completed.(tui.Model)
	if len(requests) != 0 {
		t.Fatalf("/overview must not call provider, got %#v", requests)
	}

	model = openAIDialogInputIfNeeded(t, model)
	for _, ch := range "/actions" {
		typed, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}})
		model = typed.(tui.Model)
	}
	submitting, submitCmd = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if submitCmd == nil {
		t.Fatalf("expected AI dialog /actions to submit")
	}
	completed, cmd = submitting.Update(submitCmd())
	if cmd != nil {
		t.Fatalf("expected AI dialog /actions update to be synchronous")
	}
	model = completed.(tui.Model)
	if len(requests) != 0 {
		t.Fatalf("/actions must not call provider, got %#v", requests)
	}

	model = openAIDialogInputIfNeeded(t, model)
	for _, ch := range "/previews" {
		typed, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}})
		model = typed.(tui.Model)
	}
	submitting, submitCmd = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if submitCmd == nil {
		t.Fatalf("expected AI dialog /previews to submit")
	}
	completed, cmd = submitting.Update(submitCmd())
	if cmd != nil {
		t.Fatalf("expected AI dialog /previews update to be synchronous")
	}
	model = completed.(tui.Model)
	if len(requests) != 0 {
		t.Fatalf("/previews must not call provider, got %#v", requests)
	}

	model = openAIDialogInputIfNeeded(t, model)
	for _, ch := range "/gates" {
		typed, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}})
		model = typed.(tui.Model)
	}
	submitting, submitCmd = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if submitCmd == nil {
		t.Fatalf("expected AI dialog /gates to submit")
	}
	completed, cmd = submitting.Update(submitCmd())
	if cmd != nil {
		t.Fatalf("expected AI dialog /gates update to be synchronous")
	}
	model = completed.(tui.Model)
	if len(requests) != 0 {
		t.Fatalf("/gates must not call provider, got %#v", requests)
	}

	providers := fmt.Sprintf(`version: 1
providers:
  - name: local-openai
    type: openai_compatible
    model: gpt-clean-history
    base_url: %s/v1
    api_key_env: OURO_TEST_PROVIDER_KEY
    timeout_ms: 5000
    enabled: true
`, server.URL)
	if err := os.WriteFile(filepath.Join(root, "providers.yaml"), []byte(providers), 0o644); err != nil {
		t.Fatalf("write providers: %v", err)
	}

	model = openAIDialogInputIfNeeded(t, model)
	const prompt = "continue with provider context"
	for _, ch := range prompt {
		typed, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}})
		model = typed.(tui.Model)
	}
	submitting, submitCmd = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if submitCmd == nil {
		t.Fatalf("expected AI dialog normal prompt to submit")
	}
	completed, cmd = submitting.Update(submitCmd())
	if cmd != nil {
		t.Fatalf("expected AI dialog provider update to be synchronous")
	}
	model = completed.(tui.Model)

	if len(requests) != 1 {
		t.Fatalf("expected one provider request after normal prompt, got %#v", requests)
	}
	messages := requests[0].Messages
	if len(messages) != 2 || messages[0].Role != "system" || messages[1].Role != "user" || messages[1].Content != prompt {
		t.Fatalf("expected provider request to contain only system plus current user prompt, got %#v", messages)
	}
	for _, message := range messages {
		commonForbidden := []string{"/setup", "/quickstart", "/keys", "/doctor", "/completion", "/final", "/workers", "/evidence", "/mission", "/capabilities", "/overview", "/actions", "/previews", "/gates", "AI setup:", "quickstart:", "keys:", "doctor:", "completion:", "final evidence:", "chat_focus=", "runtime_execute=", "quit=", "workspace_ready=", "strict_errors=", "toolchain_status=", "completion_estimate=", "claim_100_ready=", "external_evidence_required", "production_acceptance_required=", "completion_audit_required=", "production_acceptance=make_smoke-production-acceptance", "completion_audit=make_audit-completion", "claim_100_after=", "init=./bin/ouro", "chat=./bin/ouro", "smoke=make smoke-ide", "workers:", "evidence:", "mission:", "capabilities:", "overview:", "semantic actions:", "model previews:", "review gates:", root, "configure-openai-compatible", "<workspace>", "<base-url>", "<ENV_VAR>", "secret-value"}
		roleForbidden := commonForbidden
		if message.Role != "system" {
			roleForbidden = append(roleForbidden, "workflow_status=", "providers_status=", "capabilities_status=", "registered=", "facts=", "target_policy=", "targets=", "pack_capabilities=", "semantic_capabilities=")
		}
		for _, forbidden := range roleForbidden {
			if strings.Contains(message.Content, forbidden) {
				t.Fatalf("provider history leaked local command content %q in %#v", forbidden, messages)
			}
		}
	}
	if !strings.Contains(model.View(), "AI > Provider history is clean.") {
		t.Fatalf("expected normal provider response in view:\n%s", model.View())
	}
}

func TestNewTUIModelForWorkspaceAIDialogClearCommandClearsSessionHistory(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	type capturedRequest struct {
		Model    string `json:"model"`
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
	}
	requests := []capturedRequest{}
	const firstPrompt = "CLEAR_HISTORY_SECRET_PROMPT use old context"
	const firstAssistant = "CLEAR_HISTORY_SECRET_ASSISTANT old answer"
	const secondPrompt = "start fresh after clear"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("expected chat completions POST, got %s %s", r.Method, r.URL.Path)
		}
		var captured capturedRequest
		if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
			t.Fatalf("decode provider request: %v", err)
		}
		requests = append(requests, captured)
		assistantText := firstAssistant
		if len(requests) > 1 {
			assistantText = "fresh answer"
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{
			"choices": [{"message": {"role": "assistant", "content": %q}}],
			"usage": {"prompt_tokens": 19, "completion_tokens": 5, "total_tokens": 24}
		}`, assistantText)
	}))
	defer server.Close()
	t.Setenv("OURO_TEST_PROVIDER_KEY", "secret-value")
	providers := fmt.Sprintf(`version: 1
providers:
  - name: local-openai
    type: openai_compatible
    model: gpt-clear-history
    base_url: %s/v1
    api_key_env: OURO_TEST_PROVIDER_KEY
    timeout_ms: 5000
    enabled: true
`, server.URL)
	if err := os.WriteFile(filepath.Join(root, "providers.yaml"), []byte(providers), 0o644); err != nil {
		t.Fatalf("write providers: %v", err)
	}

	model := NewTUIModelForWorkspace(root)
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 2200, Height: 48})
	model = updated.(tui.Model)
	submitPrompt := func(prompt string) {
		t.Helper()
		model = openAIDialogInputIfNeeded(t, model)
		for _, ch := range prompt {
			typed, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}})
			model = typed.(tui.Model)
		}
		submitting, submitCmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
		if submitCmd == nil {
			t.Fatalf("expected AI dialog prompt %q to submit", prompt)
		}
		completed, cmd := submitting.Update(submitCmd())
		if cmd != nil {
			t.Fatalf("expected AI dialog prompt %q update to be synchronous", prompt)
		}
		model = completed.(tui.Model)
	}

	submitPrompt(firstPrompt)
	if len(requests) != 1 {
		t.Fatalf("expected first provider request, got %#v", requests)
	}
	submitPrompt("/clear")
	if len(requests) != 1 {
		t.Fatalf("/clear must not call provider, got %#v", requests)
	}
	clearView := model.View()
	if !strings.Contains(clearView, "AI > dialog cleared; stored=false") {
		t.Fatalf("expected /clear confirmation in view:\n%s", clearView)
	}
	for _, forbidden := range []string{firstPrompt, firstAssistant, "You > /clear"} {
		if strings.Contains(clearView, forbidden) {
			t.Fatalf("/clear should remove prior transcript content %q:\n%s", forbidden, clearView)
		}
	}

	submitPrompt(secondPrompt)
	if len(requests) != 2 {
		t.Fatalf("expected second provider request after /clear, got %#v", requests)
	}
	secondMessages := requests[1].Messages
	if len(secondMessages) != 2 || secondMessages[0].Role != "system" || secondMessages[1].Role != "user" || secondMessages[1].Content != secondPrompt {
		t.Fatalf("expected second provider request to contain only system plus fresh user prompt, got %#v", secondMessages)
	}
	for _, message := range secondMessages {
		for _, forbidden := range []string{firstPrompt, firstAssistant, "/clear", "dialog cleared", "secret-value"} {
			if strings.Contains(message.Content, forbidden) {
				t.Fatalf("provider request after /clear leaked %q in %#v", forbidden, secondMessages)
			}
		}
	}
}

func TestNewTUIModelForWorkspaceAIDialogShowsProviderSetupGuidance(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	skipIfWindows(t)
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	t.Setenv("OURO_TEST_PROVIDER_KEY", "secret-value")

	model := NewTUIModelForWorkspace(root).StartAIDialogInput().SelectRuntimeTab(tui.RuntimeTabModelRuntime)
	resized, cmd := model.Update(tea.WindowSizeMsg{Width: 2200, Height: 48})
	if cmd != nil {
		t.Fatalf("expected resize update to be synchronous")
	}
	initialView := resized.View()
	for _, want := range []string{
		"AI setup: status=needs-provider",
		"reason=enabled-provider-offline",
		"api_key_value_stored=false",
		"ouro provider configure-openai-compatible " + root,
		"--capability chat --capability tool_planning --enable",
	} {
		if !strings.Contains(initialView, want) {
			t.Fatalf("expected fresh chat TUI setup guidance to contain %q:\n%s", want, initialView)
		}
	}
	if strings.Contains(initialView, "secret-value") {
		t.Fatalf("AI setup guidance leaked API key value:\n%s", initialView)
	}

	model = resized.(tui.Model)
	for _, ch := range "help me start" {
		typed, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}})
		model = typed.(tui.Model)
	}
	submitting, submitCmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if submitCmd == nil {
		t.Fatalf("expected AI dialog enter to submit to provider")
	}
	completed, cmd := submitting.Update(submitCmd())
	if cmd != nil {
		t.Fatalf("expected AI dialog failure update to be synchronous")
	}
	errorView := completed.View()
	for _, want := range []string{
		"AI > error:",
		"AI setup: configure provider with:",
		"ouro provider configure-openai-compatible " + root,
		"--api-key-env <ENV_VAR>",
	} {
		if !strings.Contains(errorView, want) {
			t.Fatalf("expected failed fresh chat to contain setup guidance %q:\n%s", want, errorView)
		}
	}
	if strings.Contains(errorView, "secret-value") {
		t.Fatalf("AI dialog setup error leaked API key value:\n%s", errorView)
	}
}

func TestNewTUIModelForWorkspaceAIDialogCreatesModelToolPlanPreview(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	assistantSecret := "ASSISTANT_TOOL_PLAN_SECRET_SHOULD_NOT_PERSIST"
	assistantText := "Use the existing review gate.\nsemantic_action: enumerate_paths\n" + strings.Repeat("padding ", 32) + assistantSecret
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("expected chat completions POST, got %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{
			"choices": [{"message": {"role": "assistant", "content": %q}}],
			"usage": {"prompt_tokens": 21, "completion_tokens": 8, "total_tokens": 29}
		}`, assistantText)
	}))
	defer server.Close()
	t.Setenv("OURO_TEST_PROVIDER_KEY", "secret-value")
	providers := fmt.Sprintf(`version: 1
providers:
  - name: local-openai
    type: openai_compatible
    model: gpt-plan
    base_url: %s/v1
    api_key_env: OURO_TEST_PROVIDER_KEY
    timeout_ms: 5000
    enabled: true
`, server.URL)
	if err := os.WriteFile(filepath.Join(root, "providers.yaml"), []byte(providers), 0o644); err != nil {
		t.Fatalf("write providers: %v", err)
	}

	model := NewTUIModelForWorkspace(root)
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 2200, Height: 48})
	model = updated.(tui.Model)
	model = openAIDialogInputIfNeeded(t, model)
	const prompt = "PROMPT_SHOULD_NOT_PERSIST create the next safe plan"
	for _, ch := range prompt {
		typed, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}})
		model = typed.(tui.Model)
	}
	submitting, submitCmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if submitCmd == nil {
		t.Fatalf("expected AI dialog enter to submit to provider")
	}
	completed, cmd := submitting.Update(submitCmd())
	if cmd != nil {
		t.Fatalf("expected AI dialog result update to be synchronous")
	}
	model = completed.(tui.Model).SelectRuntimeTab(tui.RuntimeTabModelRuntime)

	artifacts, err := ListModelToolPlanPreviewArtifacts(root)
	if err != nil {
		t.Fatalf("list model tool-plan preview artifacts: %v", err)
	}
	if len(artifacts) != 1 {
		t.Fatalf("expected one model tool-plan preview artifact, got %#v", artifacts)
	}
	artifact := artifacts[0]
	if artifact.Source != "tui-ai-dialog" || artifact.AgentID != "tui-ai-dialog" || artifact.Role != "operator-assistant" {
		t.Fatalf("expected TUI AI dialog source metadata, got %#v", artifact)
	}
	if !artifact.Valid || artifact.Action != compiler.ActionEnumeratePaths || artifact.ExecutionMode != "metadata-only" || artifact.AssistantTextPersisted || artifact.ExecutedTools != 0 || artifact.WorkerDispatch || artifact.TargetContact {
		t.Fatalf("unexpected model tool-plan preview artifact safety metadata: %#v", artifact)
	}
	if artifact.ReviewPath == "" || artifact.ReviewNodes == 0 {
		t.Fatalf("expected linked compile-preview review metadata, got %#v", artifact)
	}
	reviews, err := ListToolCompilePreviewReviews(root)
	if err != nil {
		t.Fatalf("list compile-preview reviews: %v", err)
	}
	if len(reviews) != 1 || reviews[0].Path != artifact.ReviewPath || reviews[0].Preview.Action != compiler.ActionEnumeratePaths || reviews[0].Mode != "metadata-only" {
		t.Fatalf("expected one linked metadata-only compile-preview review, reviews=%#v artifact=%#v", reviews, artifact)
	}
	if dispatches, err := ListToolCompilePreviewReviewExecutionDispatches(root); err != nil || len(dispatches) != 0 {
		t.Fatalf("AI dialog tool-plan preview must not create execution dispatches, dispatches=%#v err=%v", dispatches, err)
	}
	if executions, err := ListToolCompilePreviewReviewExecutions(root); err != nil || len(executions) != 0 {
		t.Fatalf("AI dialog tool-plan preview must not execute review nodes, executions=%#v err=%v", executions, err)
	}
	artifactData, err := os.ReadFile(artifact.Path)
	if err != nil {
		t.Fatalf("read model tool-plan preview artifact: %v", err)
	}
	previewRows := strings.Join(modelToolPlanPreviewArtifactRuntimeRows([]ModelToolPlanPreviewArtifact{artifact}), "\n")
	for _, forbidden := range []string{assistantSecret, prompt, "secret-value"} {
		if strings.Contains(string(artifactData), forbidden) {
			t.Fatalf("model tool-plan preview artifact leaked %q: %s", forbidden, string(artifactData))
		}
		if strings.Contains(previewRows, forbidden) {
			t.Fatalf("model tool-plan preview runtime row leaked %q: %s", forbidden, previewRows)
		}
	}

	resized, resizeCmd := model.Update(tea.WindowSizeMsg{Width: 2200, Height: 48})
	if resizeCmd != nil {
		t.Fatalf("expected resize after AI dialog preview to be synchronous")
	}
	view := resized.View()
	for _, want := range []string{
		"model tool plan preview: source=tui-ai-dialog",
		"action=enumerate_paths",
		"validity=valid",
		"mode=metadata-only",
		"preview=semantic_action: enumerate_paths",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected AI dialog-created preview view %q:\n%s", want, view)
		}
	}
	selected := selectRuntimeRowContaining(t, model, "> model tool plan preview: source=tui-ai-dialog")
	detail := selected.ShowRuntimeActionDetails().View()
	for _, want := range []string{
		"runtime action: model-tool-plan-preview",
		"action: enumerate_paths",
		"state: valid",
		"mode: metadata-only",
		"output: preview=semantic_action: enumerate_paths",
	} {
		if !strings.Contains(detail, want) {
			t.Fatalf("expected AI dialog-created preview details %q:\n%s", want, detail)
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

func TestNewTUIModelForWorkspaceShowsModelGenerationReviewArtifacts(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	skipIfWindows(t)
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	secret := "DO_NOT_RENDER_FULL_GENERATION"
	artifact, err := RecordModelGenerationArtifact(root, "run-tui-generation", ProviderGenerationResult{
		ProviderName:           "local-openai",
		Model:                  "gpt-review",
		Text:                   "bounded review preview " + strings.Repeat("padding ", 24) + secret,
		InputTokens:            64,
		OutputTokens:           16,
		TotalTokens:            80,
		ContextWindowTokens:    8192,
		Capabilities:           []string{"planning"},
		EstimatedCostUSD:       0.0004,
		EstimatedCostAvailable: true,
		RoutingStrategy:        providerRoutingEnabledOrderFailover,
		ProviderAttemptCount:   2,
		ProviderFailover:       true,
		SelectedProviderName:   "local-openai",
		FailedProviderNames:    []string{"primary-openai"},
		RequiredCapabilities:   []string{"json_mode", "tool_planning"},
		CapabilityMatch:        true,
		SkippedProviders: []ProviderSkippedProvider{{
			Name:                "model-without-json",
			Reason:              "missing_required_capabilities",
			MissingCapabilities: []string{"json_mode"},
		}},
		Streaming:        true,
		StreamChunkCount: 3,
		StreamCompleted:  true,
		StreamDigest:     "stream-digest",
		StreamPreview:    "bounded review preview",
	}, ProviderGenerationBudgetDecision{
		Enforced:            true,
		Status:              "within_limit",
		MaxEstimatedCostUSD: 0.01,
	}, true, ModelGenerationArtifactOptions{Now: time.Date(2026, 6, 4, 15, 30, 0, 0, time.UTC)})
	if err != nil {
		t.Fatalf("record model generation artifact: %v", err)
	}

	model := NewTUIModelForWorkspace(root).
		SelectRuntimeTab(tui.RuntimeTabModelRuntime).
		SelectRuntimeRowMatching(tui.RuntimeTabModelRuntime, "model generation artifact:", "")
	resized, cmd := model.Update(tea.WindowSizeMsg{Width: 2200, Height: 40})
	if cmd != nil {
		t.Fatalf("expected resize update to be synchronous")
	}
	view := resized.View()
	for _, want := range []string{
		"model generation artifact: mode=metadata-only status=completed run=run-tui-generation",
		"source=closed-loop-planner-generation",
		"provider=local-openai",
		"model=gpt-review",
		"tokens=64/16/80",
		"cost=$0.000400",
		"budget=within_limit",
		"ledger_write=true",
		"assistant_text_persisted=false",
		"routing=enabled_order_failover",
		"attempts=2",
		"failover=true",
		"selected=local-openai",
		"failed=primary-openai",
		"required_capabilities=json_mode|tool_planning",
		"capability_match=true",
		"skipped_providers=model-without-json=missing_required_capabilities",
		"streaming=true",
		"stream_completed=true",
		"stream_chunk_count=3",
		"stream_digest=stream-digest",
		"stream_preview=bounded review preview",
		"path=" + artifact.Path,
		"digest=",
		"preview=",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected model generation artifact row to contain %q:\n%s", want, view)
		}
	}
	rows := strings.Join(modelGenerationArtifactRuntimeRows([]ModelGenerationArtifact{artifact}), "\n")
	for _, want := range []string{
		"stream_preview_b64=" + base64.RawURLEncoding.EncodeToString([]byte("bounded review preview")),
		"preview_b64=" + base64.RawURLEncoding.EncodeToString([]byte(artifact.TextPreview)),
	} {
		if !strings.Contains(rows, want) {
			t.Fatalf("expected model generation runtime row to contain %q:\n%s", want, rows)
		}
	}
	if strings.Contains(view, secret) {
		t.Fatalf("model generation artifact TUI row leaked full generation secret:\n%s", view)
	}
}

func TestNewTUIModelForWorkspaceShowsModelToolPlanPreviewArtifacts(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	review, err := CreateToolCompilePreviewReview(root, ToolCompilePreviewOptions{Action: compiler.ActionEnumeratePaths})
	if err != nil {
		t.Fatalf("create compile preview review: %v", err)
	}
	extraction := ExtractModelToolPlanPreview("ouro_action: enumerate_paths\n" + strings.Repeat("padding ", 32) + "DO_NOT_PRINT_MODEL_TOOL_PLAN_SECRET")
	artifact, err := RecordModelToolPlanPreviewArtifact(root, "run-tui-tool-plan-preview", ProviderGenerationResult{
		ProviderName: "local-openai",
		Model:        "gpt-plan",
	}, "", extraction, review, ModelToolPlanPreviewArtifactOptions{Now: time.Date(2026, 6, 4, 16, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatalf("record model tool-plan preview artifact: %v", err)
	}

	model := NewTUIModelForWorkspace(root).
		SelectRuntimeTab(tui.RuntimeTabModelRuntime).
		SelectRuntimeRowMatching(tui.RuntimeTabModelRuntime, "model tool plan preview:", "")
	resized, cmd := model.Update(tea.WindowSizeMsg{Width: 2200, Height: 40})
	if cmd != nil {
		t.Fatalf("expected resize update to be synchronous")
	}
	view := resized.View()
	for _, want := range []string{
		"model tool plan preview: source=closed-loop-planner-generation agent=closed-loop-planner role=planner",
		"provider=local-openai",
		"model=gpt-plan",
		"action=enumerate_paths",
		"validity=valid",
		"mode=metadata-only",
		"path=" + artifact.Path,
		"digest=" + artifact.ArtifactDigest,
		"preview=ouro_action: enumerate_paths",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected model tool-plan preview row to contain %q:\n%s", want, view)
		}
	}
	rows := strings.Join(modelToolPlanPreviewArtifactRuntimeRows([]ModelToolPlanPreviewArtifact{artifact}), "\n")
	if want := "preview_b64=" + base64.RawURLEncoding.EncodeToString([]byte(artifact.MarkerPreview)); !strings.Contains(rows, want) {
		t.Fatalf("expected model tool-plan preview runtime row to contain %q:\n%s", want, rows)
	}
	if strings.Contains(view, "DO_NOT_PRINT_MODEL_TOOL_PLAN_SECRET") {
		t.Fatalf("model tool-plan preview TUI row leaked full model output:\n%s", view)
	}

	detailed := resized.(tui.Model).ShowRuntimeActionDetails()
	detailView := detailed.View()
	for _, want := range []string{
		"runtime action: model-tool-plan-preview",
		"action type: enumerate_paths",
		"state: valid",
		"execution mode: metadata-only",
		"artifact path: " + artifact.Path,
		"digest: " + artifact.ArtifactDigest,
		"output: preview=ouro_action: enumerate_paths",
		"mode: metadata-only",
	} {
		if !strings.Contains(detailView, want) {
			t.Fatalf("expected model tool-plan preview details to contain %q:\n%s", want, detailView)
		}
	}
}

func TestModelToolPlanPreviewProjectionIncludesEncodedReferenceFields(t *testing.T) {
	previewPath := "/tmp/ouro/model preview\tstatus=failed mode=execute"
	reviewPath := "/tmp/ouro/compile review\tmode=execute"
	selectionPath := "/tmp/ouro/review selection\taccepted_by=evil"
	acceptancePath := "/tmp/ouro/model acceptance\tstatus=failed"
	acceptedBy := "tui operator\tstatus=failed"
	reason := "accepted by operator status=failed mode=execute preview=/tmp/ouro/evil-preview.json"
	artifact := ModelToolPlanPreviewArtifact{
		Path:                   previewPath,
		Source:                 closedLoopPlannerGenerationStageID,
		AgentID:                "closed-loop-planner",
		Role:                   "planner",
		ProviderName:           "local-openai",
		Model:                  "gpt-plan",
		Action:                 compiler.ActionEnumeratePaths,
		Valid:                  true,
		ExecutionMode:          "metadata-only",
		ArtifactDigest:         "sha256:preview123",
		MarkerPreview:          "ouro_action: enumerate_paths",
		AssistantTextPersisted: false,
	}
	acceptance := ModelToolPlanPreviewAcceptance{
		Path:                     acceptancePath,
		Status:                   "accepted",
		ExecutionMode:            "metadata-only",
		AcceptedBy:               acceptedBy,
		AcceptedAt:               "2026-06-05T03:00:00Z",
		Reason:                   reason,
		Action:                   compiler.ActionEnumeratePaths,
		ReviewNodes:              1,
		ReviewWarnings:           0,
		PreviewArtifactPath:      previewPath,
		ReviewPath:               reviewPath,
		SelectionPath:            selectionPath,
		AssistantTextPersisted:   false,
		ExecutedTools:            0,
		WorkerDispatch:           false,
		TargetContact:            false,
		ExecutionDispatchCreated: false,
		AcceptanceDigest:         "sha256:accept123",
	}

	previewRow := requireSingleRuntimeRowWithPrefix(t, modelToolPlanPreviewArtifactRuntimeRows([]ModelToolPlanPreviewArtifact{artifact}), "model tool plan preview:")
	for _, want := range []string{
		"path=" + compactRuntimeRowValue(previewPath) + " path_b64=",
		"path_b64=" + base64.RawURLEncoding.EncodeToString([]byte(previewPath)),
		"preview_b64=" + base64.RawURLEncoding.EncodeToString([]byte(artifact.MarkerPreview)),
	} {
		if !strings.Contains(previewRow, want) {
			t.Fatalf("expected model tool-plan preview row %q in row:\n%s", want, previewRow)
		}
	}

	acceptanceRow := requireSingleRuntimeRowWithPrefix(t, modelToolPlanPreviewAcceptanceRuntimeRows([]ModelToolPlanPreviewAcceptance{acceptance}), "model tool plan preview acceptance artifact:")
	for _, want := range []string{
		"preview=" + compactRuntimeRowValue(previewPath) + " preview_b64=",
		"review=" + compactRuntimeRowValue(reviewPath) + " review_b64=",
		"selection=" + compactRuntimeRowValue(selectionPath) + " selection_b64=",
		"path=" + compactRuntimeRowValue(acceptancePath) + " path_b64=",
		"accepted_by=" + compactRuntimeRowValue(acceptedBy) + " accepted_by_b64=",
		"preview_b64=" + base64.RawURLEncoding.EncodeToString([]byte(previewPath)),
		"review_b64=" + base64.RawURLEncoding.EncodeToString([]byte(reviewPath)),
		"selection_b64=" + base64.RawURLEncoding.EncodeToString([]byte(selectionPath)),
		"path_b64=" + base64.RawURLEncoding.EncodeToString([]byte(acceptancePath)),
		"accepted_by_b64=" + base64.RawURLEncoding.EncodeToString([]byte(acceptedBy)),
		"reason_b64=" + base64.RawURLEncoding.EncodeToString([]byte(reason)),
	} {
		if !strings.Contains(acceptanceRow, want) {
			t.Fatalf("expected model tool-plan preview acceptance row %q in row:\n%s", want, acceptanceRow)
		}
	}
}

func TestNewTUIModelForWorkspaceShowsWorkflowStatusNextGate(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	review, err := CreateToolCompilePreviewReview(root, ToolCompilePreviewOptions{Action: compiler.ActionEnumeratePaths})
	if err != nil {
		t.Fatalf("create compile preview review: %v", err)
	}
	extraction := ExtractModelToolPlanPreview("ouro_action: enumerate_paths\n" + strings.Repeat("padding ", 16))
	preview, err := RecordModelToolPlanPreviewArtifact(root, "run-workflow-tui", ProviderGenerationResult{
		ProviderName: "local-openai",
		Model:        "gpt-plan",
	}, "", extraction, review, ModelToolPlanPreviewArtifactOptions{Now: time.Date(2026, 6, 5, 3, 20, 0, 0, time.UTC)})
	if err != nil {
		t.Fatalf("record model tool-plan preview artifact: %v", err)
	}

	model := NewTUIModelForWorkspace(root).
		SelectRuntimeTab(tui.RuntimeTabModelRuntime).
		SelectRuntimeRowMatching(tui.RuntimeTabModelRuntime, "workflow status:", "")
	resized, cmd := model.Update(tea.WindowSizeMsg{Width: 2200, Height: 40})
	if cmd != nil {
		t.Fatalf("expected resize update to be synchronous")
	}
	view := resized.View()
	for _, want := range []string{
		"workflow status: mode=read-only status=ready stage=model-tool-plan next_gate=accept-model-tool-plan-preview reason=valid-preview-awaits-acceptance",
		"action=enumerate_paths",
		"preview=" + preview.Path,
		"review=" + review.Path,
		"command=ouro",
		"tool-plan-preview-accept",
		"--accept",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected workflow status row to contain %q:\n%s", want, view)
		}
	}
	detailed := resized.(tui.Model).ShowRuntimeActionDetails()
	detailView := detailed.View()
	for _, want := range []string{
		"runtime action: workflow-status",
		"state: ready",
		"execution mode: read-only",
		"action type: enumerate_paths",
		"artifact path: " + preview.Path,
		"review path: " + review.Path,
		"request path: accept-model-tool-plan-preview",
		"output: command=ouro",
	} {
		if !strings.Contains(detailView, want) {
			t.Fatalf("expected workflow status details to contain %q:\n%s", want, detailView)
		}
	}
}

func TestNewTUIModelForWorkspaceShowsWorkflowStatusSafetyAuthorizationGate(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	review, err := CreateToolCompilePreviewReview(root, ToolCompilePreviewOptions{Action: compiler.ActionEnumeratePaths})
	if err != nil {
		t.Fatalf("create compile preview review: %v", err)
	}
	extraction := ExtractModelToolPlanPreview("ouro_action: enumerate_paths\n" + strings.Repeat("padding ", 16))
	preview, err := RecordModelToolPlanPreviewArtifact(root, "run-workflow-tui-safety", ProviderGenerationResult{
		ProviderName: "local-openai",
		Model:        "gpt-plan",
	}, "", extraction, review, ModelToolPlanPreviewArtifactOptions{Now: time.Date(2026, 6, 5, 4, 20, 0, 0, time.UTC)})
	if err != nil {
		t.Fatalf("record model tool-plan preview artifact: %v", err)
	}
	if _, err := AcceptModelToolPlanPreviewArtifact(root, filepath.Base(preview.Path), ModelToolPlanPreviewAcceptanceOptions{AcceptedBy: "operator"}); err != nil {
		t.Fatalf("accept preview: %v", err)
	}
	dispatch, err := CreateToolCompilePreviewReviewExecutionDispatch(root, review.Path, ToolCompilePreviewReviewExecutionDispatchOptions{Actor: "operator"})
	if err != nil {
		t.Fatalf("create compile dispatch: %v", err)
	}
	writeWorkflowStatusDisabledScope(t, root)

	model := NewTUIModelForWorkspace(root).
		SelectRuntimeTab(tui.RuntimeTabModelRuntime).
		SelectRuntimeRowMatching(tui.RuntimeTabModelRuntime, "workflow status:", "")
	resized, cmd := model.Update(tea.WindowSizeMsg{Width: 2200, Height: 40})
	if cmd != nil {
		t.Fatalf("expected resize update to be synchronous")
	}
	view := resized.View()
	for _, want := range []string{
		"workflow status: mode=read-only status=blocked stage=model-tool-plan next_gate=safety-authorization reason=compile-execution-requires-current-safety-authorization",
		"safety_errors=1",
		"action=enumerate_paths",
		"command=ouro",
		"doctor",
		"--require-safety-authorization",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected workflow safety status row to contain %q:\n%s", want, view)
		}
	}
	detailed := resized.(tui.Model).ShowRuntimeActionDetails()
	detailView := detailed.View()
	for _, want := range []string{
		"runtime action: workflow-status",
		"state: blocked",
		"execution mode: read-only",
		"request path: safety-authorization",
		"review path: " + review.Path,
		"dispatch path: " + dispatch.Path,
		"reason: compile-execution-requires-current-safety-authorization",
		"output: command=ouro",
	} {
		if !strings.Contains(detailView, want) {
			t.Fatalf("expected workflow safety details to contain %q:\n%s", want, detailView)
		}
	}
}

func TestNewTUIModelForWorkspaceCreatesCompilePreviewReviewExecutionDispatchFromWorkflowStatusAfterDoubleConfirm(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace with spaces")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	marker := filepath.Join(t.TempDir(), "workflow-status-compile-dispatch-ran")
	reviewPath := writeCompilePreviewReviewFixture(t, root, "workflow-status-current-review.json", ToolCompilePreviewReview{
		Status: "dry-run",
		Mode:   "metadata-only",
		Preview: ToolCompilePreview{
			Workspace: root,
			Action:    compiler.ActionDiscoverAssets,
			Mode:      "metadata-only",
			Nodes: []compiler.PlanNode{
				{
					ID:           "discover_assets:workflow-status",
					Action:       compiler.ActionDiscoverAssets,
					ManifestName: "web-api-pack",
					ToolID:       "workflow-status-safe-touch",
					AdapterID:    "command",
					Operation:    "touch_marker",
					Capability:   "web.discovery",
					TargetKind:   "url",
					Runner:       "command",
					RunCommand:   []string{"sh", "-c", "touch " + marker},
					Parser:       "web_api_parser",
				},
			},
		},
	})
	if _, err := SelectToolCompilePreviewReview(root, filepath.Base(reviewPath), ToolCompilePreviewReviewSelectionOptions{
		SelectedBy: "closed-loop-planner",
		Reason:     "workflow status current review",
	}); err != nil {
		t.Fatalf("select compile preview review: %v", err)
	}

	model := NewTUIModelForWorkspace(root).
		SelectRuntimeTab(tui.RuntimeTabModelRuntime)
	model = selectRuntimeRowContaining(t, model, "> workflow status: mode=read-only status=ready stage=model-tool-plan next_gate=create-compile-preview-review-execution-dispatch")
	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if cmd != nil {
		t.Fatalf("first x should only arm workflow-status compile dispatch creation confirmation")
	}
	armed, ok := updated.(tui.Model)
	if !ok {
		t.Fatalf("expected TUI model after first x, got %T", updated)
	}
	if dispatches, err := ListToolCompilePreviewReviewExecutionDispatches(root); err != nil || len(dispatches) != 0 {
		t.Fatalf("first x must not create compile dispatches, dispatches=%#v err=%v", dispatches, err)
	}
	if executions, err := ListToolCompilePreviewReviewExecutions(root); err != nil || len(executions) != 0 {
		t.Fatalf("first x must not execute compile review nodes, executions=%#v err=%v", executions, err)
	}
	if got := workflowStatusGlobCount(t, root, "reports/report-projection-dispatches/*.json"); got != 0 {
		t.Fatalf("first x must not create report dispatches, got=%d", got)
	}
	if countJSON(t, filepath.Join(root, "runtime", "worker-queue-claims")) != 0 {
		t.Fatalf("first x must not create worker queue claims")
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("first x must not execute review command, marker stat err=%v", err)
	}
	resized, resizeCmd := armed.Update(tea.WindowSizeMsg{Width: 1200, Height: 44})
	if resizeCmd != nil {
		t.Fatalf("expected resize after first x to be synchronous")
	}
	for _, want := range []string{
		"execution status: confirm-required",
		"execution confirm: press x again",
		"action mode: metadata-only",
		"request path: create-compile-preview-review-execution-dispatch",
	} {
		if !strings.Contains(resized.View(), want) {
			t.Fatalf("expected workflow-status compile dispatch confirmation %q:\n%s", want, resized.View())
		}
	}

	updated, cmd = armed.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if cmd == nil {
		t.Fatalf("second x should return workflow-status compile dispatch creation command")
	}
	msg := cmd()
	updated, cmd = updated.Update(msg)
	if cmd != nil {
		t.Fatalf("expected workflow-status compile dispatch result update to be synchronous")
	}
	completed, ok := updated.(tui.Model)
	if !ok {
		t.Fatalf("expected TUI model after second x, got %T", updated)
	}
	dispatches, err := ListToolCompilePreviewReviewExecutionDispatches(root)
	if err != nil {
		t.Fatalf("list compile preview review execution dispatches: %v", err)
	}
	if len(dispatches) != 1 || dispatches[0].ReviewPath != reviewPath || dispatches[0].Actor != "tui" || dispatches[0].Reason != "created from TUI workflow status" {
		t.Fatalf("expected one TUI workflow-status dispatch linked to current review, got %#v review=%s", dispatches, reviewPath)
	}
	dispatchRows := strings.Join(compilePreviewReviewExecutionDispatchRuntimeRows(dispatches), "\n")
	for _, want := range []string{
		"compile-preview review execution dispatch artifact:",
		"reason=created from TUI workflow status",
		"reason_b64=Y3JlYXRlZCBmcm9tIFRVSSB3b3JrZmxvdyBzdGF0dXM",
	} {
		if !strings.Contains(dispatchRows, want) {
			t.Fatalf("expected workflow-status compile dispatch runtime rows %q:\n%s", want, dispatchRows)
		}
	}
	if executions, err := ListToolCompilePreviewReviewExecutions(root); err != nil || len(executions) != 0 {
		t.Fatalf("second x must not execute compile review nodes, executions=%#v err=%v", executions, err)
	}
	if got := workflowStatusGlobCount(t, root, "reports/report-projection-dispatches/*.json"); got != 0 {
		t.Fatalf("second x must not create report dispatches, got=%d", got)
	}
	if countJSON(t, filepath.Join(root, "runtime", "worker-queue-claims")) != 0 {
		t.Fatalf("second x must not create worker queue claims")
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("creating dispatch from workflow-status must not execute review command, marker stat err=%v", err)
	}
	resized, resizeCmd = completed.Update(tea.WindowSizeMsg{Width: 1200, Height: 44})
	if resizeCmd != nil {
		t.Fatalf("expected resize after second x to be synchronous")
	}
	view := resized.View()
	for _, want := range []string{
		"execution status: completed",
		"action output: workflow status compile dispatch created",
		"compile-preview review execution dispatch artifact:",
		"status=dry-run",
		"mode=metadata-only",
		"actor=tui",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected workflow-status compile dispatch view %q:\n%s", want, view)
		}
	}
}

func TestNewTUIModelForWorkspaceExecutesCompilePreviewReviewExecutionDispatchFromWorkflowStatusAfterDoubleConfirm(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace with spaces")
	skipIfWindows(t)
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	marker := filepath.Join(t.TempDir(), "workflow-status-compile-dispatch-execution-ran")
	scope := `version: 1
scope:
  target_policy: enforce
  targets:
    - kind: url
      value: https://example.test
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
  - sh
  - -c
  - true
parser: web_api_parser
healthcheck: built_in
`
	if err := os.WriteFile(filepath.Join(root, "toolchain", "web-api-pack", "manifest.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	reviewPath := writeCompilePreviewReviewFixture(t, root, "workflow-status-executable-review.json", ToolCompilePreviewReview{
		Status: "dry-run",
		Mode:   "metadata-only",
		Preview: ToolCompilePreview{
			Workspace: root,
			Action:    compiler.ActionDiscoverAssets,
			Mode:      "metadata-only",
			Nodes: []compiler.PlanNode{
				{
					ID:           "discover_assets:workflow-status-execute",
					Action:       compiler.ActionDiscoverAssets,
					ManifestName: "web-api-pack",
					ToolID:       "workflow-status-safe-touch",
					AdapterID:    "command",
					Operation:    "touch_marker",
					Capability:   "web.discovery",
					TargetKind:   "url",
					Runner:       "command",
					RunCommand:   []string{"sh", "-c", "printf workflow-status-executed > " + marker + "; echo workflow-status-executed"},
					Parser:       "web_api_parser",
				},
			},
		},
	})
	if _, err := SelectToolCompilePreviewReview(root, filepath.Base(reviewPath), ToolCompilePreviewReviewSelectionOptions{
		SelectedBy: "closed-loop-planner",
		Reason:     "workflow status selected review",
	}); err != nil {
		t.Fatalf("select compile preview review: %v", err)
	}
	dispatch, err := CreateToolCompilePreviewReviewExecutionDispatch(root, filepath.Base(reviewPath), ToolCompilePreviewReviewExecutionDispatchOptions{
		Actor:  "operator",
		Reason: "ready for workflow status execution",
	})
	if err != nil {
		t.Fatalf("create compile preview review execution dispatch: %v", err)
	}
	status, err := SummarizeWorkspaceWorkflow(root)
	if err != nil {
		t.Fatalf("summarize workflow before TUI execution: %v", err)
	}
	if status.NextGate != "execute-compile-preview-review-execution-dispatch" || status.DispatchPath != dispatch.Path || status.ReviewPath != reviewPath {
		t.Fatalf("expected compile dispatch execution workflow gate, got %#v", status)
	}

	dispatchDir := filepath.Join(root, "toolchain", "compile-preview-review-execution-dispatches")
	executionDir := filepath.Join(root, "toolchain", "compile-preview-review-executions")
	dispatchesBefore := countJSON(t, dispatchDir)
	executionsBefore := countJSON(t, executionDir)
	reportDispatchesBefore := countJSON(t, filepath.Join(root, "reports", "report-projection-dispatches"))
	reportExecutionsBefore := countJSON(t, filepath.Join(root, "reports", "report-projection-dispatch-executions"))
	reportFilesBefore, err := filepath.Glob(filepath.Join(root, "reports", "*.md"))
	if err != nil {
		t.Fatalf("glob reports before workflow status compile execution: %v", err)
	}
	queueClaimsBefore := countJSON(t, filepath.Join(root, "runtime", "worker-queue-claims"))
	transportOutboxBefore := countJSON(t, filepath.Join(root, "runtime", "worker-transport-outbox"))

	model := NewTUIModelForWorkspace(root).SelectRuntimeTab(tui.RuntimeTabModelRuntime)
	model = selectRuntimeRowContaining(t, model, "> workflow status: mode=read-only status=ready stage=model-tool-plan next_gate=execute-compile-preview-review-execution-dispatch")
	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if cmd != nil {
		t.Fatalf("first x should only arm workflow-status compile dispatch execution confirmation")
	}
	if got := countJSON(t, executionDir); got != executionsBefore {
		t.Fatalf("first x must not create compile execution audits, before=%d after=%d", executionsBefore, got)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("first x must not execute review command, marker stat err=%v", err)
	}
	armed, ok := updated.(tui.Model)
	if !ok {
		t.Fatalf("expected TUI model after first x, got %T", updated)
	}
	resized, resizeCmd := armed.Update(tea.WindowSizeMsg{Width: 1200, Height: 44})
	if resizeCmd != nil {
		t.Fatalf("expected resize after first x to be synchronous")
	}
	for _, want := range []string{
		"action: workflow status:",
		"request path: execute-compile-preview-review-execution-dispatch",
		"execution status: confirm-required",
		"execution confirm: press x again",
		"action mode: metadata-only",
	} {
		if !strings.Contains(resized.View(), want) {
			t.Fatalf("expected workflow-status compile dispatch execution confirmation %q:\n%s", want, resized.View())
		}
	}

	updated, cmd = armed.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if cmd == nil {
		t.Fatalf("second x should return workflow-status compile dispatch execution command")
	}
	msg := cmd()
	updated, cmd = updated.Update(msg)
	if cmd != nil {
		t.Fatalf("expected workflow-status compile dispatch execution result update to be synchronous")
	}
	if data, err := os.ReadFile(marker); err != nil || strings.TrimSpace(string(data)) != "workflow-status-executed" {
		t.Fatalf("expected double-confirmed workflow-status execution to run frozen node, data=%q err=%v", data, err)
	}
	if got := countJSON(t, dispatchDir); got != dispatchesBefore {
		t.Fatalf("TUI workflow status must not create extra compile dispatches, before=%d after=%d", dispatchesBefore, got)
	}
	if got := countJSON(t, executionDir); got != executionsBefore+1 {
		t.Fatalf("TUI workflow status must create exactly one compile execution audit, before=%d after=%d", executionsBefore, got)
	}
	if got := countJSON(t, filepath.Join(root, "reports", "report-projection-dispatches")); got != reportDispatchesBefore {
		t.Fatalf("TUI workflow status compile execution must not create report dispatches, before=%d after=%d", reportDispatchesBefore, got)
	}
	if got := countJSON(t, filepath.Join(root, "reports", "report-projection-dispatch-executions")); got != reportExecutionsBefore {
		t.Fatalf("TUI workflow status compile execution must not create report executions, before=%d after=%d", reportExecutionsBefore, got)
	}
	reportFilesAfter, err := filepath.Glob(filepath.Join(root, "reports", "*.md"))
	if err != nil {
		t.Fatalf("glob reports after workflow status compile execution: %v", err)
	}
	if len(reportFilesAfter) != len(reportFilesBefore) {
		t.Fatalf("TUI workflow status compile execution must not create markdown reports, before=%v after=%v", reportFilesBefore, reportFilesAfter)
	}
	if got := countJSON(t, filepath.Join(root, "runtime", "worker-queue-claims")); got != queueClaimsBefore {
		t.Fatalf("TUI workflow status compile execution must not create worker queue claims, before=%d after=%d", queueClaimsBefore, got)
	}
	if got := countJSON(t, filepath.Join(root, "runtime", "worker-transport-outbox")); got != transportOutboxBefore {
		t.Fatalf("TUI workflow status compile execution must not create worker transport requests, before=%d after=%d", transportOutboxBefore, got)
	}
	executions, err := ListToolCompilePreviewReviewExecutions(root)
	if err != nil {
		t.Fatalf("list compile preview review executions after workflow status execution: %v", err)
	}
	if len(executions) != executionsBefore+1 {
		t.Fatalf("expected one listed workflow-status compile execution, before=%d executions=%#v", executionsBefore, executions)
	}
	execution := executions[len(executions)-1]
	if execution.Status != "completed" || execution.DispatchPath != dispatch.Path || execution.ReviewPath != reviewPath || execution.Actor != "tui" || execution.Reason != "executed from TUI workflow status" {
		t.Fatalf("unexpected TUI workflow compile execution audit: %#v", execution)
	}
	completed, ok := updated.(tui.Model)
	if !ok {
		t.Fatalf("expected TUI model after result, got %T", updated)
	}
	resized, resizeCmd = completed.Update(tea.WindowSizeMsg{Width: 1200, Height: 44})
	if resizeCmd != nil {
		t.Fatalf("expected resize after workflow status compile execution to be synchronous")
	}
	view := resized.View()
	for _, want := range []string{
		"execution status: completed",
		"action output: workflow status compile dispatch executed",
		"compile-preview review executions: total=1",
		"compile-preview review execution artifact: status=completed mode=execute action=discover_assets",
		"actor=tui",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected workflow-status compile execution view %q:\n%s", want, view)
		}
	}
}

func TestTUIRuntimeActionExecutorRejectsWorkflowStatusCompileDispatchWithoutReview(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}

	executor := tuiRuntimeActionExecutor(root)
	result, err := executor(tui.RuntimeAction{
		Kind:        "workflow-status",
		State:       "ready",
		Source:      "model-tool-plan",
		RequestPath: "create-compile-preview-review-execution-dispatch",
		ReviewPath:  "none",
	})
	if err == nil {
		t.Fatalf("expected workflow status compile dispatch with review=none to fail, result=%#v", result)
	}
	if !strings.Contains(err.Error(), "missing review") {
		t.Fatalf("expected missing review error, got %v", err)
	}
	if dispatches, listErr := ListToolCompilePreviewReviewExecutionDispatches(root); listErr != nil || len(dispatches) != 0 {
		t.Fatalf("invalid workflow status compile dispatch must not write dispatch artifacts, dispatches=%#v err=%v", dispatches, listErr)
	}
}

func TestNewTUIModelForWorkspaceAcceptsModelToolPlanPreviewAfterDoubleConfirm(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	review, err := CreateToolCompilePreviewReview(root, ToolCompilePreviewOptions{Action: compiler.ActionEnumeratePaths})
	if err != nil {
		t.Fatalf("create compile preview review: %v", err)
	}
	extraction := ExtractModelToolPlanPreview("ouro_action: enumerate_paths\n" + strings.Repeat("padding ", 32) + "DO_NOT_PRINT_MODEL_TOOL_PLAN_ACCEPT_SECRET")
	artifact, err := RecordModelToolPlanPreviewArtifact(root, "run-tui-tool-plan-accept", ProviderGenerationResult{
		ProviderName: "local-openai",
		Model:        "gpt-plan",
	}, "", extraction, review, ModelToolPlanPreviewArtifactOptions{Now: time.Date(2026, 6, 5, 3, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatalf("record model tool-plan preview artifact: %v", err)
	}

	model := NewTUIModelForWorkspace(root).SelectRuntimeTab(tui.RuntimeTabModelRuntime)
	model = selectRuntimeRowContaining(t, model, "> model tool plan preview: source=closed-loop-planner-generation")
	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if cmd != nil {
		t.Fatalf("first x should only arm model tool-plan preview acceptance confirmation")
	}
	armed, ok := updated.(tui.Model)
	if !ok {
		t.Fatalf("expected TUI model after first x, got %T", updated)
	}
	if acceptances, err := ListModelToolPlanPreviewAcceptances(root); err != nil || len(acceptances) != 0 {
		t.Fatalf("first x must not write acceptance artifacts, acceptances=%#v err=%v", acceptances, err)
	}
	if selection, found, err := ReadToolCompilePreviewReviewSelection(root); err != nil || found {
		t.Fatalf("first x must not select compile preview review, found=%v selection=%#v err=%v", found, selection, err)
	}
	if dispatches, err := ListToolCompilePreviewReviewExecutionDispatches(root); err != nil || len(dispatches) != 0 {
		t.Fatalf("first x must not create execution dispatches, dispatches=%#v err=%v", dispatches, err)
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
			t.Fatalf("expected model tool-plan preview confirmation %q:\n%s", want, resized.View())
		}
	}

	updated, cmd = armed.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if cmd == nil {
		t.Fatalf("second x should return model tool-plan preview acceptance command")
	}
	msg := cmd()
	updated, cmd = updated.Update(msg)
	if cmd != nil {
		t.Fatalf("expected model tool-plan preview acceptance result update to be synchronous")
	}
	completed, ok := updated.(tui.Model)
	if !ok {
		t.Fatalf("expected TUI model after second x, got %T", updated)
	}
	selection, found, err := ReadToolCompilePreviewReviewSelection(root)
	if err != nil {
		t.Fatalf("read selection after accept: %v", err)
	}
	if !found || selection.Status != "current" || selection.ReviewPath != review.Path || selection.SelectedBy != "tui" || selection.Reason != "accepted model tool-plan preview: accepted from TUI" {
		t.Fatalf("expected TUI acceptance to select review, found=%v selection=%#v", found, selection)
	}
	acceptances, err := ListModelToolPlanPreviewAcceptances(root)
	if err != nil {
		t.Fatalf("list acceptances after TUI accept: %v", err)
	}
	if len(acceptances) != 1 || acceptances[0].PreviewArtifactPath != artifact.Path || acceptances[0].ReviewPath != review.Path {
		t.Fatalf("expected one linked acceptance artifact, got %#v", acceptances)
	}
	if dispatches, err := ListToolCompilePreviewReviewExecutionDispatches(root); err != nil || len(dispatches) != 0 {
		t.Fatalf("second x must not create execution dispatches, dispatches=%#v err=%v", dispatches, err)
	}
	if executions, err := ListToolCompilePreviewReviewExecutions(root); err != nil || len(executions) != 0 {
		t.Fatalf("second x must not execute review nodes, executions=%#v err=%v", executions, err)
	}
	resized, resizeCmd = completed.Update(tea.WindowSizeMsg{Width: 2200, Height: 44})
	if resizeCmd != nil {
		t.Fatalf("expected resize after second x to be synchronous")
	}
	view := resized.View()
	for _, want := range []string{
		"execution status: completed",
		"action output: model tool-plan preview accepted",
		"model tool plan preview acceptance artifact:",
		"status=accepted",
		"accepted_by=tui",
		"reason=accepted from TUI",
		"preview=" + artifact.Path,
		"review=",
		"execution_dispatch_created=false",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected TUI acceptance view %q:\n%s", want, view)
		}
	}
	if strings.Contains(view, "DO_NOT_PRINT_MODEL_TOOL_PLAN_ACCEPT_SECRET") {
		t.Fatalf("model tool-plan preview acceptance view leaked full model output:\n%s", view)
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

func TestNewTUIModelForWorkspaceShowsToolchainContractRowsMetadataOnly(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	engineDir := t.TempDir()
	engine := filepath.Join(engineDir, "fake-container-engine")
	marker := filepath.Join(engineDir, "container-runtime-was-called")
	engineScript := `#!/bin/sh
touch "` + marker + `"
printf 'container runtime should not be inspected during TUI contract projection\n' >&2
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

	model := NewTUIModelForWorkspace(root).SelectRuntimeTab(tui.RuntimeTabToolchainHealth)
	resized, cmd := model.Update(tea.WindowSizeMsg{Width: 720, Height: 42})
	if cmd != nil {
		t.Fatalf("expected resize update to be synchronous")
	}
	view := resized.View()
	for _, want := range []string{
		"toolchain contract: status=ready mode=metadata-only",
		"manifests=3",
		"domain_packs=3",
		"patterns=3",
		"semantic_actions=7",
		"errors=0",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected TUI contract view to contain %q:\n%s", want, view)
		}
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("TUI contract projection must not inspect container runtime, marker stat err=%v", err)
	}

	model = resized.(tui.Model).SelectRuntimeRowMatching(tui.RuntimeTabToolchainHealth, "toolchain contract:", "")
	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	if cmd != nil {
		t.Fatalf("expected contract detail update to be synchronous")
	}
	detailView := updated.(tui.Model).View()
	for _, want := range []string{
		"runtime action: toolchain-contract",
		"state: ready",
		"execution mode: metadata-only",
		"output: contract check is read-only metadata",
		"mode: read-only",
	} {
		if !strings.Contains(detailView, want) {
			t.Fatalf("expected TUI contract details to contain %q:\n%s", want, detailView)
		}
	}
}

func TestNewTUIModelForWorkspaceShowsManagedBundleRowsReadOnly(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	payload := writeManagedBundleFixture(t, root, "httpx", "binary", "1.3.0", "#!/bin/sh\nexit 44\n")
	appendManagedBundleProvenanceForTest(t,
		filepath.Join(root, "toolchain", "managed-bundles", "httpx", "bundle.yaml"),
		"https://github.com/projectdiscovery/httpx",
		"v1.3.0",
		"2026-06-05T00:00:00Z",
		"operator",
	)
	bundle, err := ReadManagedToolBundle(root, "httpx")
	if err != nil {
		t.Fatalf("read bundle before update metadata append: %v", err)
	}
	appendManagedBundleUpdateForTest(t,
		filepath.Join(root, "toolchain", "managed-bundles", "httpx", "bundle.yaml"),
		"1.3.0",
		bundle.SHA256,
		"2026-06-05T00:00:00Z",
		"current",
	)

	model := NewTUIModelForWorkspace(root).SelectRuntimeTab(tui.RuntimeTabToolchainHealth)
	resized, cmd := model.Update(tea.WindowSizeMsg{Width: 720, Height: 42})
	if cmd != nil {
		t.Fatalf("expected resize update to be synchronous")
	}
	view := resized.View()
	for _, want := range []string{
		"managed bundles: total=1",
		"managed bundle: name=httpx status=available mode=metadata-only kind=binary version=1.3.0",
		"source=projectdiscovery/httpx",
		"update_status=current",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected managed bundle TUI row %q:\n%s", want, view)
		}
	}

	model = resized.(tui.Model).SelectRuntimeRowMatching(tui.RuntimeTabToolchainHealth, "managed bundle:", "name=httpx")
	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	if cmd != nil {
		t.Fatalf("expected managed bundle detail update to be synchronous")
	}
	detailView := updated.(tui.Model).View()
	for _, want := range []string{
		"runtime action: managed-bundle",
		"name: httpx",
		"state: available",
		"execution mode: metadata-only",
		"digest: " + bundle.SHA256,
		"artifact path: " + filepath.Join(root, "toolchain", "managed-bundles", "httpx", "bundle.yaml"),
		"result path: " + payload,
		"target: " + filepath.Join(root, "toolchain", "bin", "httpx"),
		"output: provenance_status=recorded provenance_source_url=https://github.com/projectdiscovery/httpx provenance_upstream_version=v1.3.0 provenance_retrieved_at=2026-06-05T00:00:00Z provenance_verified_by=operator",
		"action hint: update_status=current update_latest_version=1.3.0 update_latest_sha256=" + bundle.SHA256 + " update_checked_at=2026-06-05T00:00:00Z update_message=current",
		"mode: read-only",
	} {
		if !strings.Contains(detailView, want) {
			t.Fatalf("expected managed bundle details %q:\n%s", want, detailView)
		}
	}
}

func TestManagedBundleProjectionIncludesEncodedSourceText(t *testing.T) {
	source := "projectdiscovery httpx source_b64=fake status=blocked mode=execute name=evil-source sha256=evil"
	bundle := ManagedToolBundle{
		Name:               "httpx",
		Status:             "available",
		Kind:               "binary",
		Version:            "1.3.0",
		Source:             source,
		UpdateStatus:       "current",
		ProvenanceStatus:   "recorded",
		SHA256:             "abc123",
		PayloadPath:        "/tmp/ouro/workspace/toolchain/managed-bundles/httpx/payload/httpx",
		InstallPath:        "/tmp/ouro/workspace/toolchain/bin/httpx",
		ManifestPath:       "/tmp/ouro/workspace/toolchain/managed-bundles/httpx/bundle.yaml",
		UpdateMessage:      "current",
		UpdateLatestSHA256: "abc123",
	}

	rows := managedBundleRuntimeRows([]ManagedToolBundle{bundle})
	row := requireSingleRuntimeRowWithPrefix(t, rows, "managed bundle: name=httpx")
	want := "source_b64=" + base64.RawURLEncoding.EncodeToString([]byte(source))
	if !strings.Contains(row, want) {
		t.Fatalf("expected managed bundle row %q in row:\n%s", want, row)
	}
}

func TestManagedBundleProjectionIncludesEncodedReferenceFields(t *testing.T) {
	payloadPath := "/tmp/ouro/workspace/toolchain/managed bundles/httpx/payload status=blocked/httpx"
	installPath := "/tmp/ouro/workspace/toolchain/bin/install mode=execute/httpx"
	manifestPath := "/tmp/ouro/workspace/toolchain/managed bundles/httpx/manifest payload=/tmp/evil/bundle.yaml"
	bundle := ManagedToolBundle{
		Name:             "httpx",
		Status:           "available",
		Kind:             "binary",
		Version:          "1.3.0",
		Source:           "projectdiscovery/httpx",
		UpdateStatus:     "current",
		ProvenanceStatus: "recorded",
		SHA256:           "abc123",
		PayloadPath:      payloadPath,
		InstallPath:      installPath,
		ManifestPath:     manifestPath,
	}

	rows := managedBundleRuntimeRows([]ManagedToolBundle{bundle})
	row := requireSingleRuntimeRowWithPrefix(t, rows, "managed bundle: name=httpx")
	for _, want := range []string{
		"payload=" + compactRuntimeRowValue(payloadPath) + " payload_b64=",
		"install=" + compactRuntimeRowValue(installPath) + " install_b64=",
		"manifest=" + compactRuntimeRowValue(manifestPath) + " manifest_b64=",
		"payload_b64=" + base64.RawURLEncoding.EncodeToString([]byte(payloadPath)),
		"install_b64=" + base64.RawURLEncoding.EncodeToString([]byte(installPath)),
		"manifest_b64=" + base64.RawURLEncoding.EncodeToString([]byte(manifestPath)),
	} {
		if !strings.Contains(row, want) {
			t.Fatalf("expected managed bundle row %q in row:\n%s", want, row)
		}
	}
}

func TestManagedBundleProjectionIncludesSignatureMetadata(t *testing.T) {
	bundle := ManagedToolBundle{
		Name:               "httpx",
		Status:             "available",
		Kind:               "binary",
		Version:            "1.3.0",
		Source:             "projectdiscovery/httpx",
		UpdateStatus:       "current",
		ProvenanceStatus:   "recorded",
		SignatureStatus:    "verified",
		SignatureAlgorithm: "ed25519",
		SignatureKeyID:     "local-test-key",
		SignatureSignedAt:  "2026-06-07T00:00:00Z",
		SignatureSignedBy:  "operator",
		SHA256:             "abc123",
		PayloadPath:        "/tmp/ouro/workspace/toolchain/managed-bundles/httpx/payload/httpx",
		InstallPath:        "/tmp/ouro/workspace/toolchain/bin/httpx",
		ManifestPath:       "/tmp/ouro/workspace/toolchain/managed-bundles/httpx/bundle.yaml",
	}

	rows := managedBundleRuntimeRows([]ManagedToolBundle{bundle})
	row := requireSingleRuntimeRowWithPrefix(t, rows, "managed bundle: name=httpx")
	for _, want := range []string{
		"signature_status=verified",
		"signature_algorithm=ed25519",
		"signature_key_id=local-test-key",
		"signature_signed_at=2026-06-07T00:00:00Z",
		"signature_signed_by=operator",
	} {
		if !strings.Contains(row, want) {
			t.Fatalf("expected managed bundle row %q in row:\n%s", want, row)
		}
	}
}

func TestManagedBundleProjectionIncludesCorpusMetadata(t *testing.T) {
	sha := strings.Repeat("d", 64)
	bundle := ManagedToolBundle{
		Name:                  "nuclei-templates",
		Status:                "available",
		Kind:                  "rule_bundle",
		Version:               "2026.06.05",
		Source:                "projectdiscovery/nuclei-templates",
		UpdateStatus:          "current",
		ProvenanceStatus:      "recorded",
		CorpusStatus:          "recorded",
		CorpusName:            "nuclei-templates",
		CorpusSource:          "https://github.com/projectdiscovery/nuclei-templates",
		CorpusRetrievedAt:     "2026-06-05T00:00:00Z",
		CorpusFileCount:       1,
		CorpusSHA256:          sha,
		CorpusDigestStatus:    "matched",
		CorpusDigestAlgorithm: "sha256",
		CorpusDigestScope:     "directory_canonical_records",
		SHA256:                sha,
		PayloadPath:           "/tmp/ouro/workspace/toolchain/managed-bundles/nuclei-templates/payload",
		InstallPath:           "/tmp/ouro/workspace/toolchain/rules/nuclei-templates",
		ManifestPath:          "/tmp/ouro/workspace/toolchain/managed-bundles/nuclei-templates/bundle.yaml",
	}

	rows := managedBundleRuntimeRows([]ManagedToolBundle{bundle})
	row := requireSingleRuntimeRowWithPrefix(t, rows, "managed bundle: name=nuclei-templates")
	for _, want := range []string{
		"corpus_status=recorded",
		"corpus_name=nuclei-templates",
		"corpus_source=https://github.com/projectdiscovery/nuclei-templates",
		"corpus_retrieved_at=2026-06-05T00:00:00Z",
		"corpus_file_count=1",
		"corpus_sha256=" + sha,
		"corpus_digest_status=matched",
		"corpus_digest_algorithm=sha256",
		"corpus_digest_scope=directory_canonical_records",
	} {
		if !strings.Contains(row, want) {
			t.Fatalf("expected managed bundle corpus row %q in row:\n%s", want, row)
		}
	}
}

func TestNewTUIModelForWorkspaceShowsBlockedManagedBundleUpdateRowsReadOnly(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	marker := filepath.Join(t.TempDir(), "managed-payload-ran")
	writeManagedBundleFixture(t, root, "httpx", "binary", "1.3.0", "#!/bin/sh\ntouch "+marker+"\n")
	appendManagedBundleUpdateForTest(
		t,
		filepath.Join(root, "toolchain", "managed-bundles", "httpx", "bundle.yaml"),
		"1.3.1",
		"not-a-sha",
		"2026-06-05T00:00:00Z",
		"update_available",
	)

	model := NewTUIModelForWorkspace(root).SelectRuntimeTab(tui.RuntimeTabToolchainHealth)
	resized, cmd := model.Update(tea.WindowSizeMsg{Width: 720, Height: 42})
	if cmd != nil {
		t.Fatalf("expected resize update to be synchronous")
	}
	view := resized.View()
	for _, want := range []string{
		"managed bundles: total=1",
		"managed bundle: name=httpx status=blocked",
		"update_status=blocked",
		"error=managed_bundle_update_latest_sha256",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected blocked managed bundle TUI row %q:\n%s", want, view)
		}
	}
	if strings.Contains(view, "managed bundle error:") {
		t.Fatalf("expected blocked row instead of generic managed bundle error:\n%s", view)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("TUI blocked update projection must not execute payload, marker stat err=%v", err)
	}

	model = resized.(tui.Model).SelectRuntimeRowMatching(tui.RuntimeTabToolchainHealth, "managed bundle:", "name=httpx")
	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	if cmd != nil {
		t.Fatalf("expected managed bundle detail update to be synchronous")
	}
	detailView := updated.(tui.Model).View()
	for _, want := range []string{
		"runtime action: managed-bundle",
		"state: blocked",
		"action hint: update_status=blocked",
		"failed: error=managed_bundle_update_latest_sha256",
		"mode: read-only",
	} {
		if !strings.Contains(detailView, want) {
			t.Fatalf("expected blocked managed bundle details %q:\n%s", want, detailView)
		}
	}
}

func TestNewTUIModelForWorkspaceShowsReleaseManagedStatusRowsFromSourceReadOnly(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	releaseRoot := filepath.Join(t.TempDir(), "release")
	marker := filepath.Join(t.TempDir(), "release-status-tui-payload-ran")
	writeReleaseManagedBinaryBundleFixture(t, releaseRoot, "httpx", "1.3.0", "#!/bin/sh\ntouch "+marker+"\n")
	appendManagedBundleProvenanceForTest(t,
		filepath.Join(releaseRoot, "toolchains", "managed-bundles", "httpx", "bundle.yaml"),
		"https://github.com/projectdiscovery/httpx",
		"v1.3.0",
		"2026-06-05T00:00:00Z",
		"operator",
	)
	releaseBundle, err := ReadReleaseManagedToolBundle(releaseRoot, "httpx")
	if err != nil {
		t.Fatalf("read release bundle before update metadata append: %v", err)
	}
	appendManagedBundleUpdateForTest(t,
		filepath.Join(releaseRoot, "toolchains", "managed-bundles", "httpx", "bundle.yaml"),
		"1.4.0",
		releaseBundle.SHA256,
		"2026-06-05T00:00:00Z",
		"update_available",
	)
	source, err := SetReleaseManagedToolBundleStatusSource(root, releaseRoot, ReleaseManagedToolBundleStatusSourceOptions{
		Set:    true,
		SetBy:  "operator",
		Reason: "show release in TUI",
	})
	if err != nil {
		t.Fatalf("set release managed status source: %v", err)
	}

	model := NewTUIModelForWorkspace(root).SelectRuntimeTab(tui.RuntimeTabToolchainHealth)
	resized, cmd := model.Update(tea.WindowSizeMsg{Width: 900, Height: 44})
	if cmd != nil {
		t.Fatalf("expected resize update to be synchronous")
	}
	view := resized.View()
	for _, want := range []string{
		"release managed status source: status=current mode=metadata-only",
		"release managed statuses: total=1 available=1 blocked=0 not_imported=1 imported=0 linked=0 conflict=0",
		"release managed status: name=httpx status=available mode=read-only kind=binary version=1.3.0",
		"update_status=update_available",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected release managed status TUI row %q:\n%s", want, view)
		}
	}
	if _, err := os.Lstat(filepath.Join(root, "toolchain", "managed-bundles", "httpx")); !os.IsNotExist(err) {
		t.Fatalf("TUI release status projection must not copy workspace bundle, stat err=%v", err)
	}
	if _, err := os.Lstat(filepath.Join(root, "toolchain", "bin", "httpx")); !os.IsNotExist(err) {
		t.Fatalf("TUI release status projection must not create install link, stat err=%v", err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("TUI release status projection must not execute payload, marker stat err=%v", err)
	}

	model = resized.(tui.Model).SelectRuntimeRowMatching(tui.RuntimeTabToolchainHealth, "release managed status source:", "status=current")
	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	if cmd != nil {
		t.Fatalf("expected release managed status source detail update to be synchronous")
	}
	detailView := updated.(tui.Model).View()
	for _, want := range []string{
		"runtime action: release-managed-status-source",
		"state: current",
		"execution mode: metadata-only",
		"output: release managed status source is metadata-only",
		"artifact path: " + source.Path,
		"result path: " + releaseRoot,
		"mode: read-only",
	} {
		if !strings.Contains(detailView, want) {
			t.Fatalf("expected release managed status source details %q:\n%s", want, detailView)
		}
	}

	model = resized.(tui.Model).SelectRuntimeRowMatching(tui.RuntimeTabToolchainHealth, "release managed status:", "name=httpx")
	updated, cmd = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	if cmd != nil {
		t.Fatalf("expected release managed status detail update to be synchronous")
	}
	detailView = updated.(tui.Model).View()
	for _, want := range []string{
		"runtime action: release-managed-status",
		"name: httpx",
		"state: available",
		"execution mode: read-only",
		"output: digest_status=verified import_status=not_imported link_status=missing",
		"reason: provenance_status=recorded provenance_source_url=https://github.com/projectdiscovery/httpx provenance_upstream_version=v1.3.0 provenance_retrieved_at=2026-06-05T00:00:00Z provenance_verified_by=operator",
		"action hint: update_status=update_available update_latest_version=1.4.0 update_latest_sha256=" + releaseBundle.SHA256 + " update_checked_at=2026-06-05T00:00:00Z update_message=update_available",
		"artifact path: " + filepath.Join(releaseRoot, "toolchains", "managed-bundles", "httpx", "bundle.yaml"),
		"target: " + filepath.Join(root, "toolchain", "bin", "httpx"),
		"mode: read-only",
	} {
		if !strings.Contains(detailView, want) {
			t.Fatalf("expected release managed status details %q:\n%s", want, detailView)
		}
	}
}

func TestNewTUIModelForWorkspaceAppliesReleaseManagedStatusSourcePinPolicy(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	releaseRoot := filepath.Join(t.TempDir(), "release")
	marker := filepath.Join(t.TempDir(), "release-status-pin-tui-payload-ran")
	writeReleaseManagedBinaryBundleFixture(t, releaseRoot, "httpx", "1.3.0", "#!/bin/sh\ntouch "+marker+"\n")
	wrongDigest := sha256.Sum256([]byte("different release payload"))
	wrongSHA := hex.EncodeToString(wrongDigest[:])
	source, err := SetReleaseManagedToolBundleStatusSource(root, releaseRoot, ReleaseManagedToolBundleStatusSourceOptions{
		Set:                    true,
		SetBy:                  "operator",
		Reason:                 "pin release in TUI",
		ExpectedSHA256Bundle:   "httpx",
		ExpectedSHA256:         wrongSHA,
		ExpectedSHA256Provided: true,
	})
	if err != nil {
		t.Fatalf("set release managed status source with mismatched expected pin: %v", err)
	}

	model := NewTUIModelForWorkspace(root).SelectRuntimeTab(tui.RuntimeTabToolchainHealth)
	resized, cmd := model.Update(tea.WindowSizeMsg{Width: 900, Height: 44})
	if cmd != nil {
		t.Fatalf("expected resize update to be synchronous")
	}
	view := resized.View()
	for _, want := range []string{
		"release managed status source: status=current mode=metadata-only",
		"expected_sha256_bundle=httpx",
		"expected_sha256=" + wrongSHA,
		"expected_sha256_status=mismatch",
		"release managed statuses: total=1 available=0 blocked=1 not_imported=0 imported=0 linked=0 conflict=0",
		"release managed status: name=httpx status=blocked mode=read-only kind=binary version=1.3.0",
		"digest_status=verified",
		"import_status=blocked",
		"expected_sha256_status=mismatch",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected pinned release managed status TUI row %q:\n%s", want, view)
		}
	}
	if _, err := os.Lstat(filepath.Join(root, "toolchain", "managed-bundles", "httpx")); !os.IsNotExist(err) {
		t.Fatalf("TUI pinned release status projection must not copy workspace bundle, stat err=%v", err)
	}
	if _, err := os.Lstat(filepath.Join(root, "toolchain", "bin", "httpx")); !os.IsNotExist(err) {
		t.Fatalf("TUI pinned release status projection must not create install link, stat err=%v", err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("TUI pinned release status projection must not execute payload, marker stat err=%v", err)
	}

	model = resized.(tui.Model).SelectRuntimeRowMatching(tui.RuntimeTabToolchainHealth, "release managed status source:", "status=current")
	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	if cmd != nil {
		t.Fatalf("expected release managed status source detail update to be synchronous")
	}
	detailView := updated.(tui.Model).View()
	for _, want := range []string{
		"runtime action: release-managed-status-source",
		"state: current",
		"artifact path: " + source.Path,
		"hint: expected_sha256_bundle=httpx expected_sha256_status=mismatch expected_sha256=" + wrongSHA,
		"mode: read-only",
	} {
		if !strings.Contains(detailView, want) {
			t.Fatalf("expected pinned release managed status source details %q:\n%s", want, detailView)
		}
	}
}

func TestNewTUIModelForWorkspaceShowsManagedUpdateRequestRowsReadOnly(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	skipIfWindows(t)
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	releaseRoot := filepath.Join(t.TempDir(), "release root with spaces")
	marker := filepath.Join(t.TempDir(), "managed-update-request-tui-payload-ran")
	writeReleaseManagedBinaryBundleFixture(t, releaseRoot, "httpx", "1.3.0", "#!/bin/sh\ntouch "+marker+"\n")
	releaseBundle, err := ReadReleaseManagedToolBundle(releaseRoot, "httpx")
	if err != nil {
		t.Fatalf("read release bundle before update append: %v", err)
	}
	latestSHA := strings.Repeat("e", 64)
	appendManagedBundleUpdateForTest(t,
		filepath.Join(releaseRoot, "toolchains", "managed-bundles", "httpx", "bundle.yaml"),
		"1.4.0",
		latestSHA,
		"2026-06-07T00:00:00Z",
		"update_available_from_TUI",
	)
	request, err := CreateManagedToolBundleUpdateRequest(root, releaseRoot, "httpx", ManagedToolBundleUpdateRequestOptions{
		Request:     true,
		RequestedBy: "operator",
		Reason:      "review update in TUI",
	})
	if err != nil {
		t.Fatalf("create managed update request: %v", err)
	}

	model := NewTUIModelForWorkspace(root).SelectRuntimeTab(tui.RuntimeTabToolchainHealth)
	resized, cmd := model.Update(tea.WindowSizeMsg{Width: 900, Height: 44})
	if cmd != nil {
		t.Fatalf("expected resize update to be synchronous")
	}
	view := resized.View()
	for _, want := range []string{
		"managed update requests: total=1 pending=1",
		"managed update request artifact: status=pending mode=metadata-only name=httpx kind=binary current_version=1.3.0 latest_version=1.4.0 update_status=update_available",
		"latest_sha256=" + latestSHA,
		"requested_by=operator",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected managed update request TUI row %q:\n%s", want, view)
		}
	}
	if _, err := os.Lstat(filepath.Join(root, "toolchain", "managed-bundles", "httpx")); !os.IsNotExist(err) {
		t.Fatalf("TUI update request projection must not copy workspace bundle, stat err=%v", err)
	}
	if _, err := os.Lstat(filepath.Join(root, "toolchain", "bin", "httpx")); !os.IsNotExist(err) {
		t.Fatalf("TUI update request projection must not create install link, stat err=%v", err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("TUI update request projection must not execute payload, marker stat err=%v", err)
	}

	model = resized.(tui.Model).SelectRuntimeRowMatching(tui.RuntimeTabToolchainHealth, "managed update request artifact:", "name=httpx")
	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	if cmd != nil {
		t.Fatalf("expected managed update request detail update to be synchronous")
	}
	detailView := updated.(tui.Model).View()
	for _, want := range []string{
		"runtime action: managed-update-request",
		"name: httpx",
		"state: pending",
		"execution mode: metadata-only",
		"output: update_status=update_available current_version=1.3.0 latest_version=1.4.0 latest_sha256=" + latestSHA,
		"reason: review update in TUI",
		"artifact path: " + request.Path,
		"result path: " + filepath.Join(releaseRoot, "toolchains", "managed-bundles", "httpx", "payload", "httpx"),
		"target: " + filepath.Join(releaseRoot, "toolchains", "managed-bundles", "httpx"),
		"request path: " + filepath.Join(releaseRoot, "toolchains", "managed-bundles", "httpx", "bundle.yaml"),
		"hint: requested_by=operator current_sha256=" + releaseBundle.SHA256 + " checked_at=2026-06-07T00:00:00Z message=update_available_from_TUI",
		"mode: read-only",
	} {
		if !strings.Contains(detailView, want) {
			t.Fatalf("expected managed update request details %q:\n%s", want, detailView)
		}
	}
}

func TestManagedUpdateRequestProjectionIncludesEncodedReferenceFields(t *testing.T) {
	request := ManagedToolBundleUpdateRequest{
		Status:              "pending",
		Mode:                "metadata-only",
		Name:                "httpx",
		Kind:                "binary",
		CurrentVersion:      "1.3.0",
		CurrentSHA256:       "abc",
		LatestVersion:       "1.4.0",
		LatestSHA256:        strings.Repeat("f", 64),
		UpdateStatus:        "update_available",
		UpdateCheckedAt:     "2026-06-07T00:00:00Z",
		UpdateMessage:       "message with status=blocked artifact=/tmp/evil",
		ReleaseRoot:         "/tmp/ouro release root status=blocked",
		ReleaseBundleDir:    "/tmp/ouro release/toolchains/managed bundles/httpx release_manifest=/tmp/evil",
		ReleaseManifestPath: "/tmp/ouro release/toolchains/managed bundles/httpx/bundle latest_sha256=evil.yaml",
		ReleasePayloadPath:  "/tmp/ouro release/toolchains/managed bundles/httpx/payload/httpx mode=execute",
		Path:                "/tmp/ouro workspace/toolchain/managed update requests/httpx status=approved.json",
		RequestedBy:         "operator name=evil",
		RequestedAt:         "2026-06-07T00:01:00Z",
		Reason:              "review reason status=approved path=/tmp/evil",
	}

	row := requireSingleRuntimeRowWithPrefix(t, managedUpdateRequestRuntimeRows([]ManagedToolBundleUpdateRequest{request}), "managed update request artifact:")
	for _, want := range []string{
		"artifact_b64=" + base64.RawURLEncoding.EncodeToString([]byte(request.Path)),
		"release_root_b64=" + base64.RawURLEncoding.EncodeToString([]byte(request.ReleaseRoot)),
		"release_bundle_b64=" + base64.RawURLEncoding.EncodeToString([]byte(request.ReleaseBundleDir)),
		"release_manifest_b64=" + base64.RawURLEncoding.EncodeToString([]byte(request.ReleaseManifestPath)),
		"release_payload_b64=" + base64.RawURLEncoding.EncodeToString([]byte(request.ReleasePayloadPath)),
		"requested_by_b64=" + base64.RawURLEncoding.EncodeToString([]byte(request.RequestedBy)),
		"update_message_b64=" + base64.RawURLEncoding.EncodeToString([]byte(request.UpdateMessage)),
		"reason_b64=" + base64.RawURLEncoding.EncodeToString([]byte(request.Reason)),
	} {
		if !strings.Contains(row, want) {
			t.Fatalf("expected managed update request row %q in row:\n%s", want, row)
		}
	}
}

func TestNewTUIModelForWorkspaceShowsManagedUpdateDownloadPlanRowsReadOnly(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	releaseRoot := filepath.Join(t.TempDir(), "release root with spaces")
	marker := filepath.Join(t.TempDir(), "managed-update-download-plan-tui-payload-ran")
	writeReleaseManagedBinaryBundleFixture(t, releaseRoot, "httpx", "1.3.0", "#!/bin/sh\ntouch "+marker+"\n")
	latestSHA := strings.Repeat("a", 64)
	appendManagedBundleUpdateForTest(t,
		filepath.Join(releaseRoot, "toolchains", "managed-bundles", "httpx", "bundle.yaml"),
		"1.4.0",
		latestSHA,
		"2026-06-07T00:00:00Z",
		"download_plan_from_TUI",
	)
	request, err := CreateManagedToolBundleUpdateRequest(root, releaseRoot, "httpx", ManagedToolBundleUpdateRequestOptions{
		Request:     true,
		RequestedBy: "operator",
		Reason:      "prepare update in TUI",
	})
	if err != nil {
		t.Fatalf("create managed update request: %v", err)
	}
	sourceURL := "https://downloads.example.com/httpx/v1.4.0/httpx-linux-amd64.tar.gz"
	plan, err := CreateManagedToolBundleUpdateDownloadPlan(root, request.Path, ManagedToolBundleUpdateDownloadPlanOptions{
		Plan:           true,
		SourceURL:      sourceURL,
		ExpectedSHA256: latestSHA,
		PlannedBy:      "operator",
		Reason:         "download source reviewed in TUI",
	})
	if err != nil {
		t.Fatalf("create managed update download plan: %v", err)
	}

	model := NewTUIModelForWorkspace(root).SelectRuntimeTab(tui.RuntimeTabToolchainHealth)
	resized, cmd := model.Update(tea.WindowSizeMsg{Width: 980, Height: 46})
	if cmd != nil {
		t.Fatalf("expected resize update to be synchronous")
	}
	view := resized.View()
	for _, want := range []string{
		"managed update download plans: total=1 pending=1",
		"managed update download plan artifact: status=pending mode=metadata-only name=httpx kind=binary current_version=1.3.0",
		"latest_version=1.4.0",
		"update_status=update_available",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected managed update download plan TUI row %q:\n%s", want, view)
		}
	}
	if _, err := os.Lstat(filepath.Join(root, "toolchain", "managed-bundles", "httpx")); !os.IsNotExist(err) {
		t.Fatalf("TUI download plan projection must not copy workspace bundle, stat err=%v", err)
	}
	if _, err := os.Lstat(filepath.Join(root, "toolchain", "bin", "httpx")); !os.IsNotExist(err) {
		t.Fatalf("TUI download plan projection must not create install link, stat err=%v", err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("TUI download plan projection must not execute payload, marker stat err=%v", err)
	}

	model = resized.(tui.Model).SelectRuntimeRowMatching(tui.RuntimeTabToolchainHealth, "managed update download plan artifact:", "name=httpx")
	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	if cmd != nil {
		t.Fatalf("expected managed update download plan detail update to be synchronous")
	}
	detailView := updated.(tui.Model).View()
	for _, want := range []string{
		"runtime action: managed-update-download-plan",
		"name: httpx",
		"state: pending",
		"execution mode: metadata-only",
		"source: " + sourceURL,
		"digest: " + latestSHA,
		"reason: download source reviewed in TUI",
		"artifact path: " + plan.Path,
		"request path: " + request.Path,
		"target: " + filepath.Join(releaseRoot, "toolchains", "managed-bundles", "httpx"),
		"result path: " + filepath.Join(releaseRoot, "toolchains", "managed-bundles", "httpx", "payload", "httpx"),
		"output: update_status=update_available latest_version=1.4.0 expected_sha256_status=matched",
		"hint: planned_by=operator latest_sha256=" + latestSHA + " checked_at=2026-06-07T00:00:00Z message=download_plan_from_TUI",
		"mode: read-only",
	} {
		if !strings.Contains(detailView, want) {
			t.Fatalf("expected managed update download plan details %q:\n%s", want, detailView)
		}
	}
}

func TestManagedUpdateDownloadPlanProjectionIncludesEncodedReferenceFields(t *testing.T) {
	plan := ManagedToolBundleUpdateDownloadPlan{
		Status:                      "pending",
		Mode:                        "metadata-only",
		Name:                        "httpx",
		Kind:                        "binary",
		CurrentVersion:              "1.3.0",
		CurrentSHA256:               "abc",
		LatestVersion:               "1.4.0",
		LatestSHA256:                strings.Repeat("b", 64),
		UpdateStatus:                "update_available",
		UpdateCheckedAt:             "2026-06-07T00:00:00Z",
		UpdateMessage:               "message with status=blocked artifact=/tmp/evil",
		ExpectedSHA256:              strings.Repeat("b", 64),
		ExpectedSHA256Status:        "matched",
		ArchiveFormat:               "tar.gz",
		ArchiveExpectedSHA256:       strings.Repeat("a", 64),
		ArchiveExpectedSHA256Status: "matched",
		SourceURL:                   "https://downloads.example.com/httpx/status=blocked/httpx.tar.gz",
		RequestPath:                 "/tmp/ouro workspace/toolchain/managed update requests/httpx status=approved.json",
		ReleaseRoot:                 "/tmp/ouro release root status=blocked",
		ReleaseBundleDir:            "/tmp/ouro release/toolchains/managed bundles/httpx release_manifest=/tmp/evil",
		ReleaseManifestPath:         "/tmp/ouro release/toolchains/managed bundles/httpx/bundle latest_sha256=evil.yaml",
		ReleasePayloadPath:          "/tmp/ouro release/toolchains/managed bundles/httpx/payload/httpx mode=execute",
		Path:                        "/tmp/ouro workspace/toolchain/managed update download plans/httpx status=approved.json",
		PlannedBy:                   "operator name=evil",
		PlannedAt:                   "2026-06-07T00:01:00Z",
		Reason:                      "review reason status=approved path=/tmp/evil",
	}

	row := requireSingleRuntimeRowWithPrefix(t, managedUpdateDownloadPlanRuntimeRows([]ManagedToolBundleUpdateDownloadPlan{plan}), "managed update download plan artifact:")
	for _, want := range []string{
		"artifact_b64=" + base64.RawURLEncoding.EncodeToString([]byte(plan.Path)),
		"request_b64=" + base64.RawURLEncoding.EncodeToString([]byte(plan.RequestPath)),
		"source_url_b64=" + base64.RawURLEncoding.EncodeToString([]byte(plan.SourceURL)),
		"release_root_b64=" + base64.RawURLEncoding.EncodeToString([]byte(plan.ReleaseRoot)),
		"release_bundle_b64=" + base64.RawURLEncoding.EncodeToString([]byte(plan.ReleaseBundleDir)),
		"release_manifest_b64=" + base64.RawURLEncoding.EncodeToString([]byte(plan.ReleaseManifestPath)),
		"release_payload_b64=" + base64.RawURLEncoding.EncodeToString([]byte(plan.ReleasePayloadPath)),
		"archive_format=tar.gz",
		"archive_expected_sha256=" + plan.ArchiveExpectedSHA256,
		"archive_expected_sha256_status=matched",
		"planned_by_b64=" + base64.RawURLEncoding.EncodeToString([]byte(plan.PlannedBy)),
		"update_message_b64=" + base64.RawURLEncoding.EncodeToString([]byte(plan.UpdateMessage)),
		"reason_b64=" + base64.RawURLEncoding.EncodeToString([]byte(plan.Reason)),
	} {
		if !strings.Contains(row, want) {
			t.Fatalf("expected managed update download plan row %q in row:\n%s", want, row)
		}
	}
}

func TestNewTUIModelForWorkspaceShowsManagedUpdateDownloadExecutionRowsReadOnly(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	releaseRoot := filepath.Join(t.TempDir(), "release root with spaces")
	writeReleaseManagedBinaryBundleFixture(t, releaseRoot, "httpx", "1.3.0", "#!/bin/sh\nexit 0\n")
	payload := []byte("verified TUI download payload")
	digest := sha256.Sum256(payload)
	latestSHA := hex.EncodeToString(digest[:])
	appendManagedBundleUpdateForTest(t,
		filepath.Join(releaseRoot, "toolchains", "managed-bundles", "httpx", "bundle.yaml"),
		"1.4.0",
		latestSHA,
		"2026-06-07T00:00:00Z",
		"download_execution_from_TUI",
	)
	request, err := CreateManagedToolBundleUpdateRequest(root, releaseRoot, "httpx", ManagedToolBundleUpdateRequestOptions{Request: true})
	if err != nil {
		t.Fatalf("create managed update request: %v", err)
	}
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(payload)
	}))
	t.Cleanup(server.Close)
	plan, err := CreateManagedToolBundleUpdateDownloadPlan(root, request.Path, ManagedToolBundleUpdateDownloadPlanOptions{
		Plan:           true,
		SourceURL:      server.URL,
		ExpectedSHA256: latestSHA,
		PlannedBy:      "operator",
		Reason:         "download source reviewed in TUI",
	})
	if err != nil {
		t.Fatalf("create managed update download plan: %v", err)
	}
	execution, err := ExecuteManagedToolBundleUpdateDownloadPlan(root, plan.Path, ManagedToolBundleUpdateDownloadExecutionOptions{
		Execute:    true,
		HTTPClient: server.Client(),
		Actor:      "operator",
		Reason:     "download execution reviewed in TUI",
	})
	if err != nil {
		t.Fatalf("execute managed update download plan: %v", err)
	}

	model := NewTUIModelForWorkspace(root).SelectRuntimeTab(tui.RuntimeTabToolchainHealth)
	resized, cmd := model.Update(tea.WindowSizeMsg{Width: 980, Height: 46})
	if cmd != nil {
		t.Fatalf("expected resize update to be synchronous")
	}
	view := resized.View()
	for _, want := range []string{
		"managed update download executions: total=1 completed=1 failed=0",
		"managed update download execution artifact: status=completed mode=execute action=download name=httpx kind=binary",
		"digest_status=matched",
		"bytes_written=29",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected managed update download execution TUI row %q:\n%s", want, view)
		}
	}

	model = resized.(tui.Model).SelectRuntimeRowMatching(tui.RuntimeTabToolchainHealth, "managed update download execution artifact:", "name=httpx")
	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	if cmd != nil {
		t.Fatalf("expected managed update download execution detail update to be synchronous")
	}
	detailView := updated.(tui.Model).View()
	for _, want := range []string{
		"runtime action: managed-update-download-execution",
		"name: httpx",
		"state: completed",
		"execution mode: execute",
		"source: " + server.URL,
		"digest: " + latestSHA,
		"reason: download execution reviewed in TUI",
		"artifact path: " + execution.Path,
		"dispatch path: " + plan.Path,
		"request path: " + request.Path,
		"result path: " + execution.PayloadPath,
		"target: " + filepath.Join(releaseRoot, "toolchains", "managed-bundles", "httpx"),
		"output: digest_status=matched digest_algorithm=sha256 digest_scope=file expected_sha256=" + latestSHA + " bytes_written=29 http_status=200",
		"hint: actor=operator latest_version=1.4.0 checked_at=2026-06-07T00:00:00Z message=download_execution_from_TUI",
		"mode: read-only",
	} {
		if !strings.Contains(detailView, want) {
			t.Fatalf("expected managed update download execution details %q:\n%s", want, detailView)
		}
	}
}

func TestManagedUpdateDownloadExecutionProjectionIncludesEncodedReferenceFields(t *testing.T) {
	execution := ManagedToolBundleUpdateDownloadExecution{
		Status:                 "failed",
		Mode:                   "execute",
		Action:                 "download",
		Name:                   "httpx",
		Kind:                   "binary",
		CurrentVersion:         "1.3.0",
		CurrentSHA256:          "abc",
		LatestVersion:          "1.4.0",
		LatestSHA256:           strings.Repeat("c", 64),
		UpdateStatus:           "update_available",
		UpdateCheckedAt:        "2026-06-07T00:00:00Z",
		UpdateMessage:          "message with status=blocked artifact=/tmp/evil",
		SourceURL:              "https://downloads.example.com/httpx/status=blocked/httpx.tar.gz",
		ExpectedSHA256:         strings.Repeat("b", 64),
		ActualSHA256:           strings.Repeat("d", 64),
		DigestStatus:           "mismatch",
		DigestAlgorithm:        "sha256",
		DigestScope:            "directory_canonical_records",
		ArchiveFormat:          "tar.gz",
		ArchivePath:            "/tmp/ouro workspace/toolchain/managed update downloads/httpx archive status=ready.tar.gz",
		ArchiveExpectedSHA256:  strings.Repeat("e", 64),
		ArchiveActualSHA256:    strings.Repeat("e", 64),
		ArchiveDigestStatus:    "matched",
		ArchiveDigestAlgorithm: "sha256",
		ArchiveDigestScope:     "file",
		HTTPStatus:             200,
		ContentLength:          42,
		BytesWritten:           42,
		RedirectPolicy:         "rejected",
		RedirectLocation:       "https://redirect.example.com/downloads/httpx.tar.gz",
		RedirectLocationStatus: "redacted",
		PlanPath:               "/tmp/ouro workspace/toolchain/managed update download plans/httpx status=approved.json",
		RequestPath:            "/tmp/ouro workspace/toolchain/managed update requests/httpx status=approved.json",
		ReleaseRoot:            "/tmp/ouro release root status=blocked",
		ReleaseBundleDir:       "/tmp/ouro release/toolchains/managed bundles/httpx release_manifest=/tmp/evil",
		ReleaseManifestPath:    "/tmp/ouro release/toolchains/managed bundles/httpx/bundle latest_sha256=evil.yaml",
		ReleasePayloadPath:     "/tmp/ouro release/toolchains/managed bundles/httpx/payload/httpx mode=execute",
		PayloadPath:            "/tmp/ouro workspace/toolchain/managed update downloads/httpx payload status=ready",
		Path:                   "/tmp/ouro workspace/toolchain/managed update download executions/httpx status=completed.json",
		Actor:                  "operator name=evil",
		ExecutedAt:             "2026-06-07T00:01:00Z",
		Reason:                 "execution reason status=approved path=/tmp/evil",
		Error:                  "download error payload=/tmp/evil status=completed",
	}

	row := requireSingleRuntimeRowWithPrefix(t, managedUpdateDownloadExecutionRuntimeRows([]ManagedToolBundleUpdateDownloadExecution{execution}), "managed update download execution artifact:")
	for _, want := range []string{
		"artifact_b64=" + base64.RawURLEncoding.EncodeToString([]byte(execution.Path)),
		"plan_b64=" + base64.RawURLEncoding.EncodeToString([]byte(execution.PlanPath)),
		"request_b64=" + base64.RawURLEncoding.EncodeToString([]byte(execution.RequestPath)),
		"source_url_b64=" + base64.RawURLEncoding.EncodeToString([]byte(execution.SourceURL)),
		"release_root_b64=" + base64.RawURLEncoding.EncodeToString([]byte(execution.ReleaseRoot)),
		"release_bundle_b64=" + base64.RawURLEncoding.EncodeToString([]byte(execution.ReleaseBundleDir)),
		"release_manifest_b64=" + base64.RawURLEncoding.EncodeToString([]byte(execution.ReleaseManifestPath)),
		"release_payload_b64=" + base64.RawURLEncoding.EncodeToString([]byte(execution.ReleasePayloadPath)),
		"archive_b64=" + base64.RawURLEncoding.EncodeToString([]byte(execution.ArchivePath)),
		"payload_b64=" + base64.RawURLEncoding.EncodeToString([]byte(execution.PayloadPath)),
		"archive_format=tar.gz",
		"archive_expected_sha256=" + execution.ArchiveExpectedSHA256,
		"archive_actual_sha256=" + execution.ArchiveActualSHA256,
		"archive_digest_status=matched",
		"archive_digest_algorithm=sha256",
		"archive_digest_scope=file",
		"digest_algorithm=sha256",
		"digest_scope=directory_canonical_records",
		"redirect_policy=rejected",
		"redirect_location_b64=" + base64.RawURLEncoding.EncodeToString([]byte(execution.RedirectLocation)),
		"redirect_location_status=redacted",
		"actor_b64=" + base64.RawURLEncoding.EncodeToString([]byte(execution.Actor)),
		"update_message_b64=" + base64.RawURLEncoding.EncodeToString([]byte(execution.UpdateMessage)),
		"reason_b64=" + base64.RawURLEncoding.EncodeToString([]byte(execution.Reason)),
		"error_b64=" + base64.RawURLEncoding.EncodeToString([]byte(execution.Error)),
	} {
		if !strings.Contains(row, want) {
			t.Fatalf("expected managed update download execution row %q in row:\n%s", want, row)
		}
	}
}

func TestNewTUIModelForWorkspaceShowsManagedUpdateApplyExecutionRowsReadOnly(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	skipIfWindows(t)
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	releaseRoot := filepath.Join(t.TempDir(), "release root with spaces")
	writeReleaseManagedBinaryBundleFixture(t, releaseRoot, "httpx", "1.3.0", "#!/bin/sh\nexit 0\n")
	writeManagedBundleFixture(t, root, "httpx", "binary", "1.3.0", "#!/bin/sh\nexit 0\n")
	payload := []byte("verified TUI apply payload")
	digest := sha256.Sum256(payload)
	latestSHA := hex.EncodeToString(digest[:])
	appendManagedBundleUpdateForTest(t,
		filepath.Join(releaseRoot, "toolchains", "managed-bundles", "httpx", "bundle.yaml"),
		"1.4.0",
		latestSHA,
		"2026-06-07T00:00:00Z",
		"apply_execution_from_TUI",
	)
	request, err := CreateManagedToolBundleUpdateRequest(root, releaseRoot, "httpx", ManagedToolBundleUpdateRequestOptions{Request: true})
	if err != nil {
		t.Fatalf("create managed update request: %v", err)
	}
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(payload)
	}))
	t.Cleanup(server.Close)
	plan, err := CreateManagedToolBundleUpdateDownloadPlan(root, request.Path, ManagedToolBundleUpdateDownloadPlanOptions{
		Plan:           true,
		SourceURL:      server.URL,
		ExpectedSHA256: latestSHA,
		PlannedBy:      "operator",
		Reason:         "download source reviewed in TUI",
	})
	if err != nil {
		t.Fatalf("create managed update download plan: %v", err)
	}
	download, err := ExecuteManagedToolBundleUpdateDownloadPlan(root, plan.Path, ManagedToolBundleUpdateDownloadExecutionOptions{
		Execute:    true,
		HTTPClient: server.Client(),
		Actor:      "operator",
		Reason:     "download execution reviewed in TUI",
	})
	if err != nil {
		t.Fatalf("execute managed update download plan: %v", err)
	}
	apply, err := ApplyManagedToolBundleUpdateDownloadExecution(root, download.Path, ManagedToolBundleUpdateApplyOptions{
		Apply:  true,
		Actor:  "operator",
		Reason: "apply execution reviewed in TUI",
	})
	if err != nil {
		t.Fatalf("apply managed update download execution: %v", err)
	}

	model := NewTUIModelForWorkspace(root).SelectRuntimeTab(tui.RuntimeTabToolchainHealth)
	resized, cmd := model.Update(tea.WindowSizeMsg{Width: 980, Height: 46})
	if cmd != nil {
		t.Fatalf("expected resize update to be synchronous")
	}
	view := resized.(tui.Model).View()
	for _, want := range []string{
		"managed update apply executions: total=1 completed=1 failed=0",
		"managed update apply execution artifact: status=completed mode=apply action=workspace-copy name=httpx kind=binary",
		"expected_sha256=" + latestSHA,
		"digest_status=matched",
		"digest_algorithm=sha256",
		"digest_scope=file",
		"managed update apply execution digest: name=httpx digest_status=matched digest_algorithm=sha256 digest_scope=file expected_sha256=" + latestSHA,
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected managed update apply execution TUI row %q:\n%s", want, view)
		}
	}

	model = resized.(tui.Model).SelectRuntimeRowMatching(tui.RuntimeTabToolchainHealth, "managed update apply execution artifact:", "name=httpx")
	detailed, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	if cmd != nil {
		t.Fatalf("expected managed update apply execution detail update to be synchronous")
	}
	detailView := detailed.(tui.Model).View()
	for _, want := range []string{
		"runtime action: managed-update-apply-execution",
		"state: completed",
		"execution mode: apply",
		"name: httpx",
		"source: " + server.URL,
		"target: " + apply.WorkspacePayloadPath,
		"artifact path: " + apply.Path,
		"dispatch path: " + plan.Path,
		"request path: " + request.Path,
		"source execution path: " + download.Path,
		"result path: " + download.PayloadPath,
		"reason: apply execution reviewed in TUI",
		"mode: read-only",
	} {
		if !strings.Contains(detailView, want) {
			t.Fatalf("expected managed update apply execution details %q:\n%s", want, detailView)
		}
	}
}

func TestManagedUpdateApplyExecutionProjectionIncludesEncodedReferenceFields(t *testing.T) {
	apply := ManagedToolBundleUpdateApplyExecution{
		Status:                "completed",
		Mode:                  "apply",
		Action:                "workspace-copy",
		Name:                  "httpx",
		Kind:                  "binary",
		Source:                "projectdiscovery/httpx",
		CurrentVersion:        "1.3.0",
		CurrentSHA256:         "abc",
		LatestVersion:         "1.4.0",
		LatestSHA256:          strings.Repeat("d", 64),
		UpdateStatus:          "update_available",
		UpdateCheckedAt:       "2026-06-07T00:00:00Z",
		UpdateMessage:         "apply status=blocked",
		SourceURL:             "https://downloads.example.com/httpx/status=blocked.tar.gz",
		ExpectedSHA256:        strings.Repeat("d", 64),
		ActualSHA256:          strings.Repeat("d", 64),
		DigestStatus:          "matched",
		DigestAlgorithm:       "sha256",
		DigestScope:           "file",
		DownloadExecutionPath: "/tmp/ouro workspace/toolchain/managed update download executions/httpx status=completed.json",
		PlanPath:              "/tmp/ouro workspace/toolchain/managed update download plans/httpx status=pending.json",
		RequestPath:           "/tmp/ouro workspace/toolchain/managed update requests/httpx status=pending.json",
		ReleaseRoot:           "/tmp/ouro release root with spaces",
		ReleaseBundleDir:      "/tmp/ouro release/toolchains/managed bundles/httpx",
		ReleaseManifestPath:   "/tmp/ouro release/toolchains/managed bundles/httpx/bundle.yaml",
		ReleasePayloadPath:    "/tmp/ouro release/toolchains/managed bundles/httpx/payload/httpx",
		StagedPayloadPath:     "/tmp/ouro workspace/toolchain/managed update downloads/httpx/payload",
		WorkspaceBundleDir:    "/tmp/ouro workspace/toolchain/managed bundles/httpx",
		WorkspaceManifestPath: "/tmp/ouro workspace/toolchain/managed bundles/httpx/bundle.yaml",
		WorkspacePayloadPath:  "/tmp/ouro workspace/toolchain/managed bundles/httpx/payload/httpx",
		InstallPath:           "/tmp/ouro workspace/toolchain/bin/httpx",
		Path:                  "/tmp/ouro workspace/toolchain/managed update apply executions/httpx status=completed.json",
		Actor:                 "operator name=evil",
		AppliedAt:             "2026-06-07T00:02:00Z",
		Reason:                "apply reason status=blocked path=/tmp/evil",
	}
	row := requireSingleRuntimeRowWithPrefix(t, managedUpdateApplyExecutionRuntimeRows([]ManagedToolBundleUpdateApplyExecution{apply}), "managed update apply execution artifact:")
	for _, want := range []string{
		"artifact_b64=" + runtimeRowValueB64(apply.Path),
		"download_execution_b64=" + runtimeRowValueB64(apply.DownloadExecutionPath),
		"plan_b64=" + runtimeRowValueB64(apply.PlanPath),
		"request_b64=" + runtimeRowValueB64(apply.RequestPath),
		"source_url_b64=" + runtimeRowValueB64(apply.SourceURL),
		"staged_payload_b64=" + runtimeRowValueB64(apply.StagedPayloadPath),
		"workspace_bundle_b64=" + runtimeRowValueB64(apply.WorkspaceBundleDir),
		"workspace_payload_b64=" + runtimeRowValueB64(apply.WorkspacePayloadPath),
		"reason_b64=" + runtimeRowValueB64(apply.Reason),
	} {
		if !strings.Contains(row, want) {
			t.Fatalf("expected managed update apply execution row %q in row:\n%s", want, row)
		}
	}
}

func TestReleaseManagedStatusProjectionIncludesEncodedSourceErrorText(t *testing.T) {
	sourceText := "projectdiscovery httpx status=available mode=execute name=evil-source"
	errorText := "release payload blocked install=/tmp/ouro/evil-bin release_manifest=/tmp/ouro/evil-manifest.yaml workspace_manifest=/tmp/ouro/evil-workspace.yaml digest_status=blocked"
	status := ReleaseManagedToolBundleStatus{
		Name:                  "httpx",
		Status:                "blocked",
		Mode:                  "read-only",
		Kind:                  "binary",
		Version:               "1.3.0",
		Source:                sourceText,
		SHA256:                "abc",
		DigestStatus:          "verified",
		DigestAlgorithm:       "sha256",
		DigestScope:           "file",
		ImportStatus:          "not_imported",
		LinkStatus:            "missing",
		ReleaseBundleDir:      "/tmp/ouro/share/toolchains/managed-bundles/httpx",
		ReleasePayloadPath:    "/tmp/ouro/share/toolchains/managed-bundles/httpx/payload/httpx",
		ReleaseManifestPath:   "/tmp/ouro/share/toolchains/managed-bundles/httpx/bundle.yaml",
		WorkspaceBundleDir:    "/tmp/ouro/workspace/toolchain/managed-bundles/httpx",
		WorkspacePayloadPath:  "/tmp/ouro/workspace/toolchain/managed-bundles/httpx/payload/httpx",
		WorkspaceManifestPath: "/tmp/ouro/workspace/toolchain/managed-bundles/httpx/bundle.yaml",
		InstallPath:           "/tmp/ouro/workspace/toolchain/bin/httpx",
		Error:                 errorText,
	}

	rows := releaseManagedStatusRuntimeRows([]ReleaseManagedToolBundleStatus{status})
	row := requireSingleRuntimeRowWithPrefix(t, rows, "release managed status: name=httpx")
	for _, want := range []string{
		"source_b64=" + base64.RawURLEncoding.EncodeToString([]byte(sourceText)),
		"error_b64=" + base64.RawURLEncoding.EncodeToString([]byte(errorText)),
	} {
		if !strings.Contains(row, want) {
			t.Fatalf("expected release managed status row %q in row:\n%s", want, row)
		}
	}
}

func TestReleaseManagedStatusProjectionIncludesEncodedReferenceFields(t *testing.T) {
	releaseBundle := "/tmp/ouro/share/toolchains/managed bundles/httpx release_payload=/tmp/evil"
	releasePayload := "/tmp/ouro/share/toolchains/managed bundles/httpx/payload status=blocked/httpx"
	releaseManifest := "/tmp/ouro/share/toolchains/managed bundles/httpx/manifest workspace_manifest=/tmp/evil/bundle.yaml"
	workspaceBundle := "/tmp/ouro/workspace/toolchain/managed bundles/httpx workspace_payload=/tmp/evil"
	workspacePayload := "/tmp/ouro/workspace/toolchain/managed bundles/httpx/payload link_status=linked/httpx"
	workspaceManifest := "/tmp/ouro/workspace/toolchain/managed bundles/httpx/manifest install=/tmp/evil/bundle.yaml"
	installPath := "/tmp/ouro/workspace/toolchain/bin/install mode=execute/httpx"
	status := ReleaseManagedToolBundleStatus{
		Name:                  "httpx",
		Status:                "available",
		Mode:                  "read-only",
		Kind:                  "binary",
		Version:               "1.3.0",
		Source:                "projectdiscovery/httpx",
		SHA256:                "abc",
		DigestStatus:          "verified",
		DigestAlgorithm:       "sha256",
		DigestScope:           "file",
		ImportStatus:          "not_imported",
		LinkStatus:            "missing",
		ReleaseBundleDir:      releaseBundle,
		ReleasePayloadPath:    releasePayload,
		ReleaseManifestPath:   releaseManifest,
		WorkspaceBundleDir:    workspaceBundle,
		WorkspacePayloadPath:  workspacePayload,
		WorkspaceManifestPath: workspaceManifest,
		InstallPath:           installPath,
	}

	rows := releaseManagedStatusRuntimeRows([]ReleaseManagedToolBundleStatus{status})
	row := requireSingleRuntimeRowWithPrefix(t, rows, "release managed status: name=httpx")
	for _, item := range []struct {
		key   string
		value string
	}{
		{"release_bundle", releaseBundle},
		{"release_payload", releasePayload},
		{"release_manifest", releaseManifest},
		{"workspace_bundle", workspaceBundle},
		{"workspace_payload", workspacePayload},
		{"workspace_manifest", workspaceManifest},
		{"install", installPath},
	} {
		for _, want := range []string{
			item.key + "=" + compactRuntimeRowValue(item.value) + " " + item.key + "_b64=",
			item.key + "_b64=" + base64.RawURLEncoding.EncodeToString([]byte(item.value)),
		} {
			if !strings.Contains(row, want) {
				t.Fatalf("expected release managed status row %q in row:\n%s", want, row)
			}
		}
	}
}

func TestReleaseManagedStatusProjectionIncludesCorpusMetadata(t *testing.T) {
	sha := strings.Repeat("e", 64)
	status := ReleaseManagedToolBundleStatus{
		Name:                  "nuclei-templates",
		Status:                "available",
		Mode:                  "read-only",
		Kind:                  "rule_bundle",
		Version:               "2026.06.05",
		Source:                "projectdiscovery/nuclei-templates",
		SHA256:                sha,
		ProvenanceStatus:      "recorded",
		CorpusStatus:          "recorded",
		CorpusName:            "nuclei-templates",
		CorpusSource:          "https://github.com/projectdiscovery/nuclei-templates",
		CorpusRetrievedAt:     "2026-06-05T00:00:00Z",
		CorpusFileCount:       1,
		CorpusSHA256:          sha,
		CorpusDigestStatus:    "matched",
		CorpusDigestAlgorithm: "sha256",
		CorpusDigestScope:     "directory_canonical_records",
		DigestStatus:          "verified",
		DigestAlgorithm:       "sha256",
		DigestScope:           "directory_canonical_records",
		ImportStatus:          "not_imported",
		LinkStatus:            "missing",
		ReleaseBundleDir:      "/tmp/ouro/share/toolchains/managed-bundles/nuclei-templates",
		ReleaseManifestPath:   "/tmp/ouro/share/toolchains/managed-bundles/nuclei-templates/bundle.yaml",
		ReleasePayloadPath:    "/tmp/ouro/share/toolchains/managed-bundles/nuclei-templates/payload",
		WorkspaceBundleDir:    "/tmp/ouro/workspace/toolchain/managed-bundles/nuclei-templates",
		WorkspaceManifestPath: "/tmp/ouro/workspace/toolchain/managed-bundles/nuclei-templates/bundle.yaml",
		WorkspacePayloadPath:  "/tmp/ouro/workspace/toolchain/managed-bundles/nuclei-templates/payload",
		InstallPath:           "/tmp/ouro/workspace/toolchain/rules/nuclei-templates",
	}

	rows := releaseManagedStatusRuntimeRows([]ReleaseManagedToolBundleStatus{status})
	row := requireSingleRuntimeRowWithPrefix(t, rows, "release managed status: name=nuclei-templates")
	for _, want := range []string{
		"corpus_status=recorded",
		"corpus_name=nuclei-templates",
		"corpus_source=https://github.com/projectdiscovery/nuclei-templates",
		"corpus_retrieved_at=2026-06-05T00:00:00Z",
		"corpus_file_count=1",
		"corpus_sha256=" + sha,
		"corpus_digest_status=matched",
		"corpus_digest_algorithm=sha256",
		"corpus_digest_scope=directory_canonical_records",
	} {
		if !strings.Contains(row, want) {
			t.Fatalf("expected release managed corpus row %q in row:\n%s", want, row)
		}
	}
}

func TestReleaseManagedStatusSourceProjectionIncludesEncodedReasonText(t *testing.T) {
	reason := "operator selected release reason_b64=fake status=evil mode=execute marker=/tmp/ouro/evil-marker.json"
	source := ReleaseManagedToolBundleStatusSource{
		Status:      "current",
		Mode:        "metadata-only",
		Path:        "/tmp/ouro/toolchain/release-managed-status-source.json",
		ReleaseRoot: "/tmp/ouro/share",
		Total:       1,
		Available:   1,
		Blocked:     0,
		NotImported: 1,
		Imported:    0,
		Linked:      0,
		Conflict:    0,
		Reason:      reason,
	}

	rows := releaseManagedStatusSourceRuntimeRows(source)
	row := requireSingleRuntimeRowWithPrefix(t, rows, "release managed status source:")
	for _, want := range []string{
		"reason=" + compactRuntimeRowValue(reason),
		"reason_b64=" + base64.RawURLEncoding.EncodeToString([]byte(reason)),
	} {
		if !strings.Contains(row, want) {
			t.Fatalf("expected release managed status source row %q in row:\n%s", want, row)
		}
	}
}

func TestReleaseManagedStatusSourceProjectionIncludesEncodedReferenceFields(t *testing.T) {
	releaseRoot := "/tmp/ouro/share release_root status=evil"
	marker := "/tmp/ouro/workspace/toolchain/release status source marker mode=execute.json"
	source := ReleaseManagedToolBundleStatusSource{
		Status:      "current",
		Mode:        "metadata-only",
		Path:        marker,
		ReleaseRoot: releaseRoot,
		Total:       1,
		Available:   1,
		Blocked:     0,
		NotImported: 1,
		Imported:    0,
		Linked:      0,
		Conflict:    0,
		Reason:      "current release",
	}

	rows := releaseManagedStatusSourceRuntimeRows(source)
	row := requireSingleRuntimeRowWithPrefix(t, rows, "release managed status source:")
	for _, want := range []string{
		"release_root=" + compactRuntimeRowValue(releaseRoot) + " release_root_b64=",
		"marker=" + compactRuntimeRowValue(marker) + " marker_b64=",
		"release_root_b64=" + base64.RawURLEncoding.EncodeToString([]byte(releaseRoot)),
		"marker_b64=" + base64.RawURLEncoding.EncodeToString([]byte(marker)),
	} {
		if !strings.Contains(row, want) {
			t.Fatalf("expected release managed status source row %q in row:\n%s", want, row)
		}
	}
}

func TestNewTUIModelForWorkspaceShowsBlockedToolchainContractRows(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	manifestPath := filepath.Join(root, "toolchain", "web-api-pack", "manifest.yaml")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	withoutAuthState := strings.ReplaceAll(string(data), "  - web.auth_state_tracking\n", "")
	if err := os.WriteFile(manifestPath, []byte(withoutAuthState), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	model := NewTUIModelForWorkspace(root).SelectRuntimeTab(tui.RuntimeTabToolchainHealth)
	resized, cmd := model.Update(tea.WindowSizeMsg{Width: 820, Height: 48})
	if cmd != nil {
		t.Fatalf("expected resize update to be synchronous")
	}
	view := resized.View()
	for _, want := range []string{
		"toolchain contract: status=blocked mode=metadata-only",
		"errors=1",
		"toolchain contract error: domain capability web.auth_state_tracking is not provided by workspace manifests or built-in tools",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected blocked contract view to contain %q:\n%s", want, view)
		}
	}

	model = resized.(tui.Model).SelectRuntimeRowMatching(tui.RuntimeTabToolchainHealth, "toolchain contract error:", "web.auth_state_tracking")
	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	if cmd != nil {
		t.Fatalf("expected contract error detail update to be synchronous")
	}
	detailView := updated.(tui.Model).View()
	for _, want := range []string{
		"runtime action: toolchain-contract-error",
		"state: blocked",
		"reason: domain capability web.auth_state_tracking is not provided by workspace manifests or built-in tools",
		"mode: read-only",
	} {
		if !strings.Contains(detailView, want) {
			t.Fatalf("expected blocked contract details to contain %q:\n%s", want, detailView)
		}
	}
}

func TestToolchainContractRuntimeRowsCompactManyErrors(t *testing.T) {
	check := ToolchainContractCheck{
		Status:              "blocked",
		ManifestCount:       3,
		ToolCount:           26,
		DomainPackCount:     3,
		PatternCount:        3,
		SemanticActionCount: 7,
		Errors: []string{
			"error 1",
			"error 2",
			"error 3",
			"error 4",
			"error 5",
			"error 6",
			"error 7",
			"error 8",
		},
	}

	rows := toolchainContractRuntimeRows(check)
	joined := strings.Join(rows, "\n")
	for _, want := range []string{
		"toolchain contract: status=blocked mode=metadata-only manifests=3 tools=26 domain_packs=3 patterns=3 semantic_actions=7 errors=8",
		"toolchain contract error: error 1",
		"toolchain contract error: error 3",
		"toolchain contract error: additional_errors_omitted=5 run=ouro_toolchain_contract_check",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("expected compact contract rows to contain %q:\n%s", want, joined)
		}
	}
	if strings.Contains(joined, "error 4") || strings.Contains(joined, "error 8") {
		t.Fatalf("expected contract rows to omit overflow validation errors:\n%s", joined)
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
	skipIfWindows(t)
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
	sessionReason := "saved comparison focus current=false path=/tmp/ouro/evil-session.json"
	sessionReviewReason := "authorized comparison baseline current=false path=/tmp/ouro/evil-session-review.json"
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
		Reason:    sessionReason,
	})
	if err != nil {
		t.Fatalf("create replay workbench session: %v", err)
	}
	session, err = AnnotateReplayWorkbenchSession(root, filepath.Base(session.Path), ReplayWorkbenchSessionAnnotationOptions{
		Decision:   "accepted",
		Reason:     sessionReviewReason,
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
	rows := strings.Join(replayWorkbenchSessionRuntimeRows([]ReplayWorkbenchSessionArtifact{session}), "\n")
	for _, want := range []string{
		"reason_b64=" + base64.RawURLEncoding.EncodeToString([]byte(sessionReason)),
		"review_reason_b64=" + base64.RawURLEncoding.EncodeToString([]byte(sessionReviewReason)),
	} {
		if !strings.Contains(rows, want) {
			t.Fatalf("expected replay workbench session runtime row to contain %q:\n%s", want, rows)
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
	skipIfWindows(t)
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
	collaborationReason := "pair review path=/tmp/ouro/evil-collaboration.json session_path=/tmp/ouro/evil-collaboration-session.json"
	collaborationReviewReason := "authorized pair review current=false path=/tmp/ouro/evil-collaboration-review.json"
	collaboration, err := CreateReplayWorkbenchCollaboration(context.Background(), root, ReplayWorkbenchCollaborationOptions{
		SessionPath:  filepath.Base(session.Path),
		Participants: []string{"alice", "bob"},
		CreatedBy:    "lead",
		CreatedAt:    time.Date(2026, 6, 1, 3, 45, 0, 0, time.UTC),
		Reason:       collaborationReason,
	})
	if err != nil {
		t.Fatalf("create replay workbench collaboration: %v", err)
	}
	if collaboration.Path == "" {
		t.Fatalf("expected replay workbench collaboration artifact path")
	}
	collaboration, err = AnnotateReplayWorkbenchCollaboration(root, filepath.Base(collaboration.Path), ReplayWorkbenchCollaborationAnnotationOptions{
		Decision:   "accepted",
		Reason:     collaborationReviewReason,
		Reviewer:   "alice",
		ReviewedAt: time.Date(2026, 6, 1, 5, 10, 0, 0, time.UTC),
	})
	if err != nil {
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
	rows := strings.Join(replayWorkbenchCollaborationRuntimeRows([]ReplayWorkbenchCollaborationArtifact{collaboration}), "\n")
	for _, want := range []string{
		"reason_b64=" + base64.RawURLEncoding.EncodeToString([]byte(collaborationReason)),
		"review_reason_b64=" + base64.RawURLEncoding.EncodeToString([]byte(collaborationReviewReason)),
	} {
		if !strings.Contains(rows, want) {
			t.Fatalf("expected replay workbench collaboration runtime row to contain %q:\n%s", want, rows)
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
	presenceReason := "review active path=/tmp/ouro/evil-presence.json collaboration=/tmp/ouro/evil-presence-collaboration.json session_path=/tmp/ouro/evil-presence-session.json"
	presence, err := UpdateReplayWorkbenchCollaborationPresence(context.Background(), root, ReplayWorkbenchCollaborationPresenceOptions{
		CollaborationPath: filepath.Base(collaboration.Path),
		Participant:       "alice",
		Status:            "online",
		SeenBy:            "lead",
		SeenAt:            time.Date(2026, 6, 1, 5, 30, 0, 0, time.UTC),
		Reason:            presenceReason,
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
	rows := strings.Join(replayWorkbenchCollaborationPresenceRuntimeRows([]ReplayWorkbenchCollaborationPresence{presence}), "\n")
	if want := "reason_b64=" + base64.RawURLEncoding.EncodeToString([]byte(presenceReason)); !strings.Contains(rows, want) {
		t.Fatalf("expected replay workbench collaboration presence runtime row to contain %q:\n%s", want, rows)
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
	noteReason := "collaborative evidence review current=false path=/tmp/ouro/evil-note.json"
	noteBody := "Confirm graph node before accepting. collaboration=/tmp/ouro/evil-note-collaboration.json session_path=/tmp/ouro/evil-note-session.json"
	noteReviewReason := "validated evidence note current=false path=/tmp/ouro/evil-note-review.json"
	note, err := CreateReplayWorkbenchCollaborationNote(context.Background(), root, ReplayWorkbenchCollaborationNoteOptions{
		CollaborationPath: filepath.Base(collaboration.Path),
		Author:            "alice",
		Body:              noteBody,
		TargetKind:        "graph-node",
		TargetID:          "fact-target",
		CreatedAt:         time.Date(2026, 6, 1, 8, 20, 0, 0, time.UTC),
		Reason:            noteReason,
	})
	if err != nil {
		t.Fatalf("create replay workbench collaboration note: %v", err)
	}
	note, err = AnnotateReplayWorkbenchCollaborationNote(root, filepath.Base(note.Path), ReplayWorkbenchCollaborationNoteAnnotationOptions{
		Decision:   "accepted",
		Reviewer:   "alice",
		ReviewedAt: time.Date(2026, 6, 1, 9, 30, 0, 0, time.UTC),
		Reason:     noteReviewReason,
	})
	if err != nil {
		t.Fatalf("annotate replay workbench collaboration note: %v", err)
	}
	noteSelectionReason := "current note focus note=/tmp/ouro/evil-note-selection.json"
	if _, err := SelectReplayWorkbenchCollaborationNote(root, filepath.Base(note.Path), ReplayWorkbenchCollaborationNoteSelectionOptions{
		SelectedBy: "alice",
		SelectedAt: time.Date(2026, 6, 1, 9, 50, 0, 0, time.UTC),
		Reason:     noteSelectionReason,
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
		"replay workbench collaboration note body: Confirm graph node before accepting.",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected TUI replay workbench collaboration note row %q:\n%s", want, view)
		}
	}
	rows := strings.Join(replayWorkbenchCollaborationNoteRuntimeRows([]ReplayWorkbenchCollaborationNote{note}), "\n")
	for _, want := range []string{
		"reason_b64=" + base64.RawURLEncoding.EncodeToString([]byte(noteReason)),
		"body_b64=" + base64.RawURLEncoding.EncodeToString([]byte(noteBody)),
		"review_reason_b64=" + base64.RawURLEncoding.EncodeToString([]byte(noteReviewReason)),
	} {
		if !strings.Contains(rows, want) {
			t.Fatalf("expected replay workbench collaboration note runtime row to contain %q:\n%s", want, rows)
		}
	}
	noteSelection, noteSelectionFound, err := ReadReplayWorkbenchCollaborationNoteSelection(root)
	if err != nil || !noteSelectionFound {
		t.Fatalf("read replay workbench collaboration note selection: found=%v err=%v", noteSelectionFound, err)
	}
	selectionRows := strings.Join(replayWorkbenchCollaborationNoteRuntimeRowsWithSelection([]ReplayWorkbenchCollaborationNote{note}, noteSelection, true), "\n")
	if want := "reason_b64=" + base64.RawURLEncoding.EncodeToString([]byte(noteSelectionReason)); !strings.Contains(selectionRows, want) {
		t.Fatalf("expected replay workbench collaboration note selection row to contain %q:\n%s", want, selectionRows)
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
	skipIfWindows(t)
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
	collaborationSelectionReason := "current collaboration focus collaboration=/tmp/ouro/evil-collaboration-selection.json session=/tmp/ouro/evil-collaboration-session.json"
	if _, err := SelectReplayWorkbenchCollaboration(root, filepath.Base(candidate.Path), ReplayWorkbenchCollaborationSelectionOptions{
		SelectedBy: "alice",
		SelectedAt: time.Date(2026, 6, 1, 3, 52, 0, 0, time.UTC),
		Reason:     collaborationSelectionReason,
	}); err != nil {
		t.Fatalf("select replay workbench collaboration: %v", err)
	}
	selection, selectionFound, err := ReadReplayWorkbenchCollaborationSelection(root)
	if err != nil || !selectionFound {
		t.Fatalf("read replay workbench collaboration selection: found=%v err=%v", selectionFound, err)
	}
	selectionRows := strings.Join(replayWorkbenchCollaborationRuntimeRowsWithSelection([]ReplayWorkbenchCollaborationArtifact{candidate}, selection, true), "\n")
	if want := "reason_b64=" + base64.RawURLEncoding.EncodeToString([]byte(collaborationSelectionReason)); !strings.Contains(selectionRows, want) {
		t.Fatalf("expected replay workbench collaboration selection row to contain %q:\n%s", want, selectionRows)
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
	skipIfWindows(t)
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
	sessionSelectionReason := "current investigation focus session=/tmp/ouro/evil-session-selection.json primary=run-evil"
	if _, err := SelectReplayWorkbenchSession(root, filepath.Base(candidate.Path), ReplayWorkbenchSessionSelectionOptions{
		SelectedBy: "alice",
		SelectedAt: time.Date(2026, 6, 1, 2, 42, 0, 0, time.UTC),
		Reason:     sessionSelectionReason,
	}); err != nil {
		t.Fatalf("select replay workbench session: %v", err)
	}
	selection, selectionFound, err := ReadReplayWorkbenchSessionSelection(root)
	if err != nil || !selectionFound {
		t.Fatalf("read replay workbench session selection: found=%v err=%v", selectionFound, err)
	}
	selectionRows := strings.Join(replayWorkbenchSessionRuntimeRowsWithSelection([]ReplayWorkbenchSessionArtifact{candidate}, selection, true), "\n")
	if want := "reason_b64=" + base64.RawURLEncoding.EncodeToString([]byte(sessionSelectionReason)); !strings.Contains(selectionRows, want) {
		t.Fatalf("expected replay workbench session selection row to contain %q:\n%s", want, selectionRows)
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
	skipIfWindows(t)
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
	rows := strings.Join(compilePreviewReviewRuntimeRows([]ToolCompilePreviewReview{review}), "\n")
	if want := "reason_b64=" + base64.RawURLEncoding.EncodeToString([]byte("authorized preview")); !strings.Contains(rows, want) {
		t.Fatalf("expected compile-preview review runtime row to contain %q:\n%s", want, rows)
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

func TestCompilePreviewReviewProjectionIncludesEncodedReferenceFields(t *testing.T) {
	reviewPath := "/tmp/ouro/compile preview reviews/review path=evil.json"
	reviewer := "operator reviewer_b64=fake reviewed_at=2030-01-01T00:00:00Z path=/tmp/ouro/evil-review.json"
	reviewedAt := "2026-05-31T15:04:05Z"
	reason := "authorized preview path=/tmp/ouro/other-review.json"
	review := ToolCompilePreviewReview{
		Status: "dry-run",
		Mode:   "metadata-only",
		Path:   reviewPath,
		Preview: ToolCompilePreview{
			Action: compiler.ActionDiscoverAssets,
		},
		Annotation: &ToolCompilePreviewReviewAnnotation{
			Decision:   "accepted",
			Reason:     reason,
			Reviewer:   reviewer,
			ReviewedAt: reviewedAt,
		},
	}

	rows := strings.Join(compilePreviewReviewRuntimeRows([]ToolCompilePreviewReview{review}), "\n")
	for _, want := range []string{
		"reviewer=" + compactRuntimeRowValue(reviewer) + " reviewer_b64=",
		"reviewer_b64=" + base64.RawURLEncoding.EncodeToString([]byte(reviewer)),
		"reason=" + compactRuntimeRowValue(reason) + " reason_b64=",
		"reason_b64=" + base64.RawURLEncoding.EncodeToString([]byte(reason)),
		"reviewed_at=" + compactRuntimeRowValue(reviewedAt) + " reviewed_at_b64=",
		"reviewed_at_b64=" + base64.RawURLEncoding.EncodeToString([]byte(reviewedAt)),
		"path=" + compactRuntimeRowValue(reviewPath) + " path_b64=",
		"path_b64=" + base64.RawURLEncoding.EncodeToString([]byte(reviewPath)),
	} {
		if !strings.Contains(rows, want) {
			t.Fatalf("expected compile-preview review row to contain %q:\n%s", want, rows)
		}
	}
}

func TestCompilePreviewReviewSelectionProjectionIncludesEncodedReferenceFields(t *testing.T) {
	reviewPath := "/tmp/ouro/compile preview reviews/selected review=evil.json"
	selectedBy := "operator selected_by_b64=fake review=/tmp/ouro/evil-review.json"
	selectedAt := "2026-06-01T00:15:00Z"
	reason := "best reviewed candidate review=/tmp/ouro/other-review.json selected_at=2030-01-01T00:00:00Z"
	selection := ToolCompilePreviewReviewSelection{
		Status:     "current",
		Mode:       "metadata-only",
		Action:     compiler.ActionDiscoverAssets,
		ReviewPath: reviewPath,
		SelectedBy: selectedBy,
		SelectedAt: selectedAt,
		Reason:     reason,
		SupersededPaths: []string{
			"/tmp/ouro/old-review.json",
		},
	}

	rows := strings.Join(compilePreviewReviewRuntimeRowsWithSelection(nil, selection, true), "\n")
	for _, want := range []string{
		"selected_by=" + compactRuntimeRowValue(selectedBy) + " selected_by_b64=",
		"selected_by_b64=" + base64.RawURLEncoding.EncodeToString([]byte(selectedBy)),
		"selected_at=" + compactRuntimeRowValue(selectedAt) + " selected_at_b64=",
		"selected_at_b64=" + base64.RawURLEncoding.EncodeToString([]byte(selectedAt)),
		"reason=" + compactRuntimeRowValue(reason) + " reason_b64=",
		"reason_b64=" + base64.RawURLEncoding.EncodeToString([]byte(reason)),
		"review=" + compactRuntimeRowValue(reviewPath) + " review_b64=",
		"review_b64=" + base64.RawURLEncoding.EncodeToString([]byte(reviewPath)),
	} {
		if !strings.Contains(rows, want) {
			t.Fatalf("expected compile-preview review selection row to contain %q:\n%s", want, rows)
		}
	}
}

func TestCompilePreviewReviewClearedSelectionProjectionIncludesEncodedReferenceFields(t *testing.T) {
	reviewPath := "/tmp/ouro/compile preview reviews/cleared review=evil.json"
	clearedBy := "operator cleared_by_b64=fake review=/tmp/ouro/evil-review.json"
	clearedAt := "2026-06-01T00:35:00Z"
	reason := "authorization changed review=/tmp/ouro/other-review.json cleared_at=2030-01-01T00:00:00Z"
	selection := ToolCompilePreviewReviewSelection{
		Status:     "cleared",
		Mode:       "metadata-only",
		Action:     compiler.ActionDiscoverAssets,
		ReviewPath: reviewPath,
		ClearedBy:  clearedBy,
		ClearedAt:  clearedAt,
		Reason:     reason,
		SupersededPaths: []string{
			"/tmp/ouro/old-review.json",
		},
	}

	rows := strings.Join(compilePreviewReviewRuntimeRowsWithSelection(nil, selection, true), "\n")
	for _, want := range []string{
		"cleared_by=" + compactRuntimeRowValue(clearedBy) + " cleared_by_b64=",
		"cleared_by_b64=" + base64.RawURLEncoding.EncodeToString([]byte(clearedBy)),
		"cleared_at=" + compactRuntimeRowValue(clearedAt) + " cleared_at_b64=",
		"cleared_at_b64=" + base64.RawURLEncoding.EncodeToString([]byte(clearedAt)),
		"reason=" + compactRuntimeRowValue(reason) + " reason_b64=",
		"reason_b64=" + base64.RawURLEncoding.EncodeToString([]byte(reason)),
		"review=" + compactRuntimeRowValue(reviewPath) + " review_b64=",
		"review_b64=" + base64.RawURLEncoding.EncodeToString([]byte(reviewPath)),
	} {
		if !strings.Contains(rows, want) {
			t.Fatalf("expected cleared compile-preview review selection row to contain %q:\n%s", want, rows)
		}
	}
}

func TestCompilePreviewReviewComparisonProjectionIncludesEncodedReferenceFields(t *testing.T) {
	basePath := "/tmp/ouro/compile preview reviews/base candidate=evil.json"
	candidatePath := "/tmp/ouro/compile preview reviews/candidate base=evil.json"
	comparison := ToolCompilePreviewReviewComparison{
		Mode: "metadata-only",
		Base: ToolCompilePreviewReviewComparisonSide{
			Path:   basePath,
			Action: compiler.ActionDiscoverAssets,
		},
		Candidate: ToolCompilePreviewReviewComparisonSide{
			Path:   candidatePath,
			Action: compiler.ActionDiscoverAssets,
		},
		Summary: ToolCompilePreviewReviewComparisonStats{
			NodeDelta: 1,
		},
	}

	rows := strings.Join(compilePreviewReviewComparisonRuntimeRows(comparison), "\n")
	for _, want := range []string{
		"base=" + compactRuntimeRowValue(basePath) + " base_b64=",
		"base_b64=" + base64.RawURLEncoding.EncodeToString([]byte(basePath)),
		"candidate=" + compactRuntimeRowValue(candidatePath) + " candidate_b64=",
		"candidate_b64=" + base64.RawURLEncoding.EncodeToString([]byte(candidatePath)),
	} {
		if !strings.Contains(rows, want) {
			t.Fatalf("expected compile-preview review comparison row to contain %q:\n%s", want, rows)
		}
	}
}

func TestNewTUIModelForWorkspaceShowsCompilePreviewReviewExecutionRowsReadOnly(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	review, err := CreateToolCompilePreviewReview(root, ToolCompilePreviewOptions{
		Action: compiler.ActionDiscoverAssets,
		Budget: compiler.CompileBudget{MaxPlanNodes: 1},
	})
	if err != nil {
		t.Fatalf("create compile preview review: %v", err)
	}
	if _, err := SelectToolCompilePreviewReview(root, filepath.Base(review.Path), ToolCompilePreviewReviewSelectionOptions{SelectedBy: "closed-loop-planner"}); err != nil {
		t.Fatalf("select compile preview review: %v", err)
	}
	executedAt := time.Date(2026, 6, 4, 11, 30, 0, 0, time.UTC)
	execution, err := ExecuteToolCompilePreviewReview(context.Background(), root, filepath.Base(review.Path), ToolCompilePreviewReviewExecutionOptions{
		Execute:    true,
		Actor:      "operator",
		Reason:     "reviewed execution gate",
		ExecutedAt: executedAt,
		ToolRunner: &recordingToolExecutionRunner{},
	})
	if err != nil {
		t.Fatalf("execute compile preview review: %v", err)
	}

	model := NewTUIModelForWorkspace(root).SelectRuntimeTab(tui.RuntimeTabToolchainHealth)
	resized, cmd := model.Update(tea.WindowSizeMsg{Width: 720, Height: 44})
	if cmd != nil {
		t.Fatalf("expected resize update to be synchronous")
	}
	view := resized.View()
	for _, want := range []string{
		"compile-preview review executions: total=1",
		"compile-preview review execution artifact: status=completed mode=execute action=discover_assets",
		"nodes=1",
		"warnings=1",
		"actor=operator",
		"executed_at=2026-06-04T11:30:00Z",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected TUI compile-preview review execution row %q:\n%s", want, view)
		}
	}

	model = selectRuntimeRowContaining(t, model, "> compile-preview review execution artifact:")
	model = model.ShowRuntimeActionDetails()
	resized, cmd = model.Update(tea.WindowSizeMsg{Width: 720, Height: 44})
	if cmd != nil {
		t.Fatalf("expected runtime action details resize to be synchronous")
	}
	view = resized.View()
	for _, want := range []string{
		"[Node Inspector]",
		"runtime action: compile-preview-review-execution-artifact",
		"artifact path: ",
		"compile-preview-review-executions",
		"review path: ",
		"compile-preview-reviews",
		"run: " + execution.RunID,
		"action type: discover_assets",
		"nodes: 1",
		"warnings: 1",
		"results: events=",
		"created by: operator",
		"created at: 2026-06-04T11:30:00Z",
		"reason: reviewed execution gate",
		"mode: read-only",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected compile-preview review execution details %q:\n%s", want, view)
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

func TestCompilePreviewReviewExecutionProjectionIncludesEncodedReasonErrorText(t *testing.T) {
	reason := "operator reviewed reason_b64=fake run=evil dispatch=/tmp/ouro/evil-dispatch.json review=/tmp/ouro/evil-review.json path=/tmp/ouro/evil-execution.json"
	executionError := "runner failed status=failed mode=metadata-only action=evil nodes=99 warnings=99"
	execution := ToolCompilePreviewReviewExecution{
		Path:           "/tmp/ouro/toolchain/compile-preview-review-executions/execution.json",
		Status:         "failed",
		Mode:           "execute",
		RunID:          "compile-preview-review-execution-1",
		DispatchPath:   "/tmp/ouro/toolchain/compile-preview-review-execution-dispatches/dispatch.json",
		ReviewPath:     "/tmp/ouro/toolchain/compile-preview-reviews/review.json",
		SelectionPath:  "/tmp/ouro/toolchain/compile-preview-review-selection.json",
		Action:         compiler.ActionDiscoverAssets,
		ReviewNodes:    1,
		ReviewWarnings: 1,
		EventCount:     12,
		ExecutedAt:     "2026-06-04T11:30:00Z",
		Actor:          "operator",
		Reason:         reason,
		Error:          executionError,
	}

	rows := compilePreviewReviewExecutionRuntimeRows([]ToolCompilePreviewReviewExecution{execution})
	row := requireSingleRuntimeRowWithPrefix(t, rows, "compile-preview review execution artifact:")
	for _, want := range []string{
		"reason_b64=" + base64.RawURLEncoding.EncodeToString([]byte(reason)),
		"error_b64=" + base64.RawURLEncoding.EncodeToString([]byte(executionError)),
	} {
		if !strings.Contains(row, want) {
			t.Fatalf("expected compile-preview review execution row %q in row:\n%s", want, row)
		}
	}
}

func TestCompilePreviewReviewExecutionDispatchProjectionIncludesEncodedReasonText(t *testing.T) {
	reason := "dispatch approved reason_b64=fake status=evil mode=execute action=evil nodes=99 warnings=99 actor=mallory review=/tmp/ouro/evil-review.json selection=/tmp/ouro/evil-selection.json path=/tmp/ouro/evil-dispatch.json"
	dispatch := ToolCompilePreviewReviewExecutionDispatch{
		Path:           "/tmp/ouro/toolchain/compile-preview-review-execution-dispatches/dispatch.json",
		Status:         "dry-run",
		Mode:           "metadata-only",
		ReviewPath:     "/tmp/ouro/toolchain/compile-preview-reviews/review.json",
		SelectionPath:  "/tmp/ouro/toolchain/compile-preview-review-selection.json",
		Action:         compiler.ActionDiscoverAssets,
		ReviewNodes:    1,
		ReviewWarnings: 1,
		CreatedAt:      "2026-06-04T12:15:00Z",
		Actor:          "operator",
		Reason:         reason,
	}

	rows := compilePreviewReviewExecutionDispatchRuntimeRows([]ToolCompilePreviewReviewExecutionDispatch{dispatch})
	row := requireSingleRuntimeRowWithPrefix(t, rows, "compile-preview review execution dispatch artifact:")
	if want := "reason_b64=" + base64.RawURLEncoding.EncodeToString([]byte(reason)); !strings.Contains(row, want) {
		t.Fatalf("expected compile-preview review execution dispatch row %q in row:\n%s", want, row)
	}
}

func TestNewTUIModelForWorkspaceCreatesCompilePreviewReviewExecutionDispatchFromCurrentReviewAfterDoubleConfirm(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	marker := filepath.Join(t.TempDir(), "compile-preview-review-dispatch-tui-ran")
	reviewPath := writeCompilePreviewReviewFixture(t, root, "current-review.json", ToolCompilePreviewReview{
		Status: "dry-run",
		Mode:   "metadata-only",
		Preview: ToolCompilePreview{
			Workspace: root,
			Action:    compiler.ActionDiscoverAssets,
			Mode:      "metadata-only",
			Nodes: []compiler.PlanNode{
				{
					ID:           "discover_assets:01",
					Action:       compiler.ActionDiscoverAssets,
					ManifestName: "web-api-pack",
					ToolID:       "tui-safe-touch",
					AdapterID:    "command",
					Operation:    "touch_marker",
					Capability:   "web.discovery",
					TargetKind:   "url",
					Runner:       "command",
					RunCommand:   []string{"sh", "-c", "touch " + marker},
					Parser:       "web_api_parser",
				},
			},
		},
	})
	if _, err := SelectToolCompilePreviewReview(root, filepath.Base(reviewPath), ToolCompilePreviewReviewSelectionOptions{
		SelectedBy: "operator",
		Reason:     "ready for TUI dispatch",
	}); err != nil {
		t.Fatalf("select compile preview review: %v", err)
	}

	model := NewTUIModelForWorkspace(root).SelectRuntimeTab(tui.RuntimeTabToolchainHealth)
	model = selectRuntimeRowContaining(t, model, "> compile-preview review artifact: status=dry-run mode=metadata-only action=discover_assets nodes=1 warnings=0 current=true")
	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if cmd != nil {
		t.Fatalf("first x should only arm execution dispatch creation confirmation")
	}
	armed, ok := updated.(tui.Model)
	if !ok {
		t.Fatalf("expected TUI model after first x, got %T", updated)
	}
	if dispatches, err := ListToolCompilePreviewReviewExecutionDispatches(root); err != nil || len(dispatches) != 0 {
		t.Fatalf("first x must not create dispatch artifacts, dispatches=%#v err=%v", dispatches, err)
	}
	if executions, err := ListToolCompilePreviewReviewExecutions(root); err != nil || len(executions) != 0 {
		t.Fatalf("first x must not create execution artifacts, executions=%#v err=%v", executions, err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("first x must not execute review node, marker stat err=%v", err)
	}
	resized, resizeCmd := armed.Update(tea.WindowSizeMsg{Width: 720, Height: 44})
	if resizeCmd != nil {
		t.Fatalf("expected resize after first x to be synchronous")
	}
	if !strings.Contains(resized.View(), "execution status: confirm-required") {
		t.Fatalf("expected confirmation status before creating dispatch:\n%s", resized.View())
	}

	updated, cmd = armed.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if cmd == nil {
		t.Fatalf("second x should return execution dispatch creation command")
	}
	msg := cmd()
	updated, cmd = updated.Update(msg)
	if cmd != nil {
		t.Fatalf("expected dispatch creation result update to be synchronous")
	}
	completed, ok := updated.(tui.Model)
	if !ok {
		t.Fatalf("expected TUI model after second x, got %T", updated)
	}
	dispatches, err := ListToolCompilePreviewReviewExecutionDispatches(root)
	if err != nil {
		t.Fatalf("list compile preview review execution dispatches: %v", err)
	}
	if len(dispatches) != 1 || dispatches[0].ReviewPath != reviewPath || dispatches[0].Actor != "tui" || dispatches[0].Reason != "dispatched from TUI" {
		t.Fatalf("expected one TUI-created dispatch linked to current review, got %#v review=%s", dispatches, reviewPath)
	}
	executions, err := ListToolCompilePreviewReviewExecutions(root)
	if err != nil {
		t.Fatalf("list compile preview review executions: %v", err)
	}
	if len(executions) != 0 {
		t.Fatalf("dispatch creation must not execute review nodes, executions=%#v", executions)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("creating dispatch from TUI must not execute review node, marker stat err=%v", err)
	}
	resized, resizeCmd = completed.Update(tea.WindowSizeMsg{Width: 720, Height: 44})
	if resizeCmd != nil {
		t.Fatalf("expected resize after second x to be synchronous")
	}
	view := resized.View()
	for _, want := range []string{
		"execution status: completed",
		"action output: execution dispatch created",
		"compile-preview review execution dispatches: total=1",
		"> compile-preview review execution dispatch artifact: status=dry-run mode=metadata-only action=discover_assets",
		"actor=tui",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected TUI dispatch creation view %q:\n%s", want, view)
		}
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
	skipIfWindows(t)
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
		"worker lifecycle action plan selection: status=current mode=metadata-only current=true selected_by=bob",
		"selected_at=2026-06-02T09:13:00Z",
		"reason=current_reviewed_action_plan",
		"worker lifecycle action plan artifact:",
		"worker lifecycle action plan artifact review: current=true decision=accepted reviewer=alice",
		"reviewed_at=2026-06-02T09:12:00Z review_reason=reviewed_authorized_action_plan",
		"worker lifecycle action plan review: mode=metadata-only decision=accepted reviewer=alice",
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
		"reason: current reviewed action plan",
	} {
		if !strings.Contains(detailView, want) {
			t.Fatalf("expected action-plan selection detail %q:\n%s", want, detailView)
		}
	}
}

func TestWorkerLifecycleActionPlanProjectionIncludesEncodedReviewText(t *testing.T) {
	selectionReason := "current reviewed plan=/tmp/ouro/evil-plan.json selection=/tmp/ouro/evil-selection.json review=/tmp/ouro/evil-review.json current=false actions=99"
	reviewReason := "reviewed authorized current=false review=/tmp/ouro/evil-review.json decision=rejected reviewer=mallory reviewed_at=2030-01-01T00:00:00Z"
	summary := WorkerLifecycleActionSummary{
		Total:                        2,
		AckPendingTransport:          1,
		ExecuteDispatchRetryDispatch: 1,
		ReconcileResultLease:         1,
	}
	plan := WorkerLifecycleActionPlan{
		Path:       "/tmp/ouro/action-plan.json",
		Status:     "dry-run",
		Mode:       "metadata-only",
		ReviewPath: "/tmp/ouro/review.json",
		Summary:    summary,
		Annotation: &WorkerLifecycleActionPlanAnnotation{
			Decision:   "accepted",
			Reviewer:   "alice",
			ReviewedAt: "2026-06-02T09:12:00Z",
			Reason:     reviewReason,
		},
	}
	selection := WorkerLifecycleActionPlanSelection{
		Path:       "/tmp/ouro/worker-lifecycle-action-plan-selection.json",
		Status:     "current",
		Mode:       "metadata-only",
		PlanPath:   plan.Path,
		ReviewPath: plan.ReviewPath,
		Summary:    summary,
		SelectedBy: "bob",
		SelectedAt: "2026-06-02T09:13:00Z",
		Reason:     selectionReason,
	}

	rows := strings.Join(workerLifecycleActionPlanArtifactRuntimeRowsWithSelection([]WorkerLifecycleActionPlan{plan}, selection, true), "\n")
	for _, want := range []string{
		"reason_b64=" + base64.RawURLEncoding.EncodeToString([]byte(selectionReason)),
		"review_reason_b64=" + base64.RawURLEncoding.EncodeToString([]byte(reviewReason)),
		"selected_by_b64=" + base64.RawURLEncoding.EncodeToString([]byte(selection.SelectedBy)),
		"selected_at_b64=" + base64.RawURLEncoding.EncodeToString([]byte(selection.SelectedAt)),
		"plan_b64=" + base64.RawURLEncoding.EncodeToString([]byte(selection.PlanPath)),
		"selection_b64=" + base64.RawURLEncoding.EncodeToString([]byte(selection.Path)),
		"review_b64=" + base64.RawURLEncoding.EncodeToString([]byte(selection.ReviewPath)),
		"artifact_b64=" + base64.RawURLEncoding.EncodeToString([]byte(plan.Path)),
		"reviewer_b64=" + base64.RawURLEncoding.EncodeToString([]byte(plan.Annotation.Reviewer)),
		"reviewed_at_b64=" + base64.RawURLEncoding.EncodeToString([]byte(plan.Annotation.ReviewedAt)),
	} {
		if !strings.Contains(rows, want) {
			t.Fatalf("expected projected worker lifecycle action-plan row %q in rows:\n%s", want, rows)
		}
	}
}

func TestWorkerLifecycleActionProjectionIncludesEncodedReasonText(t *testing.T) {
	reason := "pending file outbox request_path=/tmp/ouro/evil-request.json dispatch=/tmp/ouro/evil-dispatch.json recommended=skip"
	plan := WorkerLifecycleActionPlan{
		Path:   "/tmp/ouro/action-plan.json",
		Status: "dry-run",
		Mode:   "metadata-only",
		Actions: []WorkerLifecycleAction{{
			Type:         "ack-transport-request",
			Gate:         "worker-transport-ack",
			Status:       "ready",
			Mode:         "metadata-only",
			WorkerID:     "remote-web-1",
			RunID:        "run-action",
			ExecutionID:  "tool-action",
			Reason:       reason,
			RequestPath:  "/tmp/ouro/request.json",
			DispatchPath: "/tmp/ouro/dispatch.json",
		}},
	}

	rows := workerLifecycleActionPlanArtifactRuntimeRows([]WorkerLifecycleActionPlan{plan})
	row := requireSingleRuntimeRowWithPrefix(t, rows, "worker lifecycle action:")
	if want := "reason_b64=" + base64.RawURLEncoding.EncodeToString([]byte(reason)); !strings.Contains(row, want) {
		t.Fatalf("expected projected worker lifecycle action row %q in row:\n%s", want, row)
	}
}

func TestWorkerLifecycleActionProjectionIncludesEncodedIdentityText(t *testing.T) {
	workerID := "remote web status=failed request_path=/tmp/ouro/evil-request.json"
	runID := "run action execution=evil-execution dispatch=/tmp/ouro/evil-dispatch.json"
	executionID := "tool action status=failed recommended=skip"
	plan := WorkerLifecycleActionPlan{
		Path:   "/tmp/ouro/action-plan.json",
		Status: "dry-run",
		Mode:   "metadata-only",
		Actions: []WorkerLifecycleAction{{
			Type:         "ack-transport-request",
			Gate:         "worker-transport-ack",
			Status:       "ready",
			Mode:         "metadata-only",
			WorkerID:     workerID,
			RunID:        runID,
			ExecutionID:  executionID,
			Reason:       "pending-file-outbox-request",
			RequestPath:  "/tmp/ouro/request.json",
			DispatchPath: "/tmp/ouro/dispatch.json",
		}},
	}

	rows := workerLifecycleActionPlanArtifactRuntimeRows([]WorkerLifecycleActionPlan{plan})
	row := requireSingleRuntimeRowWithPrefix(t, rows, "worker lifecycle action:")
	for _, want := range []string{
		"worker_b64=" + base64.RawURLEncoding.EncodeToString([]byte(workerID)),
		"run_b64=" + base64.RawURLEncoding.EncodeToString([]byte(runID)),
		"execution_b64=" + base64.RawURLEncoding.EncodeToString([]byte(executionID)),
	} {
		if !strings.Contains(row, want) {
			t.Fatalf("expected projected worker lifecycle action row %q in row:\n%s", want, row)
		}
	}
}

func TestWorkerLifecycleAuditItemProjectionIncludesEncodedReasonText(t *testing.T) {
	reason := "audit review actions=99 skipped_unsafe=99 failed=99"
	audit := WorkerLifecycleAudit{
		Path:   "/tmp/ouro/audit.json",
		Status: "dry-run",
		Mode:   "metadata-only",
		Items: []WorkerLifecycleAuditItem{{
			Kind:                 "worker-lifecycle-maintenance-supervisor",
			Status:               "completed",
			Mode:                 "metadata-only",
			ArtifactPath:         "/tmp/ouro/supervisor.json",
			LinkedPath:           "/tmp/ouro/maintenance.json",
			Reason:               reason,
			ActionsTotal:         2,
			SkippedUnsafeActions: 1,
		}},
	}

	rows := workerLifecycleAuditArtifactRuntimeRows([]WorkerLifecycleAudit{audit})
	row := requireSingleRuntimeRowWithPrefix(t, rows, "worker lifecycle audit item:")
	if want := "reason_b64=" + base64.RawURLEncoding.EncodeToString([]byte(reason)); !strings.Contains(row, want) {
		t.Fatalf("expected projected worker lifecycle audit item row %q in row:\n%s", want, row)
	}
}

func TestWorkerDispatchRetryExecutionResultProjectionIncludesEncodedReasonText(t *testing.T) {
	reason := "retry queue claim dispatched transport=http-json worker=evil status=failed"
	execution := RuntimeWorkerDispatchRetryDispatchExecution{
		Path:   "/tmp/ouro/retry-execution.json",
		Status: "completed",
		Mode:   "execute",
		Results: []RuntimeWorkerDispatchRetryDispatchExecutionResult{{
			Status:                 "dispatched",
			Reason:                 reason,
			SourceResultPath:       "/tmp/ouro/source-result.json",
			SourceDispatchPath:     "/tmp/ouro/source-dispatch.json",
			SourceClaimPath:        "/tmp/ouro/source-claim.json",
			RetryClaimPath:         "/tmp/ouro/retry-claim.json",
			RetryQueueDispatchPath: "/tmp/ouro/retry-queue-dispatch.json",
			WorkerID:               "remote-web-1",
			RunID:                  "run-one",
			ExecutionID:            "tool-one",
			RetryTransportMode:     "file-outbox",
		}},
	}

	rows := runtimeWorkerDispatchRetryDispatchExecutionRows([]RuntimeWorkerDispatchRetryDispatchExecution{execution})
	row := requireSingleRuntimeRowWithPrefix(t, rows, "worker dispatch retry execution result:")
	if want := "reason_b64=" + base64.RawURLEncoding.EncodeToString([]byte(reason)); !strings.Contains(row, want) {
		t.Fatalf("expected projected worker dispatch retry execution result row %q in row:\n%s", want, row)
	}
}

func TestWorkerDispatchRetryExecutionProjectionIncludesEncodedReferenceFields(t *testing.T) {
	artifactPath := "/tmp/ouro/retry  execution\tstatus=failed mode=metadata-only retry_dispatch=/tmp/ouro/evil-dispatch.json"
	retryDispatchPath := "/tmp/ouro/retry  dispatch\tclaimed=99 failed=99"
	sourceResultPath := "/tmp/ouro/source  result\tstatus=failed"
	sourceDispatchPath := "/tmp/ouro/source  dispatch\tworker=evil"
	sourceClaimPath := "/tmp/ouro/source  claim\ttransport=http-json"
	retryClaimPath := "/tmp/ouro/retry  claim\tstatus=failed"
	queueDispatchPath := "/tmp/ouro/retry  queue-dispatch\tworker=evil"
	requestPath := "/tmp/ouro/retry  request\ttransport=http-json"
	workerID := "remote  web\tstatus=failed transport=http-json"
	runID := "run  one\texecution=evil"
	executionID := "tool  one\tstatus=failed"
	reason := "retry queue claim dispatched reason_b64=fake transport=http-json worker=evil status=failed"
	execution := RuntimeWorkerDispatchRetryDispatchExecution{
		Path:              artifactPath,
		Status:            "completed",
		Mode:              "execute",
		RetryDispatchPath: retryDispatchPath,
		Summary: RuntimeWorkerDispatchRetryDispatchExecutionSummary{
			Claimed:    1,
			Dispatched: 1,
		},
		Results: []RuntimeWorkerDispatchRetryDispatchExecutionResult{{
			Status:                    "dispatched",
			Reason:                    reason,
			SourceResultPath:          sourceResultPath,
			SourceDispatchPath:        sourceDispatchPath,
			SourceClaimPath:           sourceClaimPath,
			RetryClaimPath:            retryClaimPath,
			RetryQueueDispatchPath:    queueDispatchPath,
			RetryTransportRequestPath: requestPath,
			WorkerID:                  workerID,
			RunID:                     runID,
			ExecutionID:               executionID,
			RetryTransportMode:        "file-outbox",
		}},
	}

	rows := runtimeWorkerDispatchRetryDispatchExecutionRows([]RuntimeWorkerDispatchRetryDispatchExecution{execution})
	artifactRow := requireSingleRuntimeRowWithPrefix(t, rows, "worker dispatch retry execution artifact:")
	for _, want := range []string{
		"worker dispatch retry execution artifact: " + compactRuntimeRowValue(artifactPath) + " artifact_b64=",
		" retry_dispatch=" + compactRuntimeRowValue(retryDispatchPath) + " retry_dispatch_b64=",
		"artifact_b64=" + base64.RawURLEncoding.EncodeToString([]byte(artifactPath)),
		"retry_dispatch_b64=" + base64.RawURLEncoding.EncodeToString([]byte(retryDispatchPath)),
	} {
		if !strings.Contains(artifactRow, want) {
			t.Fatalf("expected projected worker dispatch retry execution artifact row %q in row:\n%s", want, artifactRow)
		}
	}
	resultRow := requireSingleRuntimeRowWithPrefix(t, rows, "worker dispatch retry execution result:")
	for _, want := range []string{
		"source_result=" + compactRuntimeRowValue(sourceResultPath) + " source_result_b64=",
		"source_dispatch=" + compactRuntimeRowValue(sourceDispatchPath) + " source_dispatch_b64=",
		"source_claim=" + compactRuntimeRowValue(sourceClaimPath) + " source_claim_b64=",
		"retry_claim=" + compactRuntimeRowValue(retryClaimPath) + " retry_claim_b64=",
		"queue_dispatch=" + compactRuntimeRowValue(queueDispatchPath) + " queue_dispatch_b64=",
		"request_path=" + compactRuntimeRowValue(requestPath) + " request_path_b64=",
		"worker=" + compactRuntimeRowValue(workerID) + " worker_b64=",
		"run=" + compactRuntimeRowValue(runID) + " run_b64=",
		"execution=" + compactRuntimeRowValue(executionID) + " execution_b64=",
		"source_result_b64=" + base64.RawURLEncoding.EncodeToString([]byte(sourceResultPath)),
		"source_dispatch_b64=" + base64.RawURLEncoding.EncodeToString([]byte(sourceDispatchPath)),
		"source_claim_b64=" + base64.RawURLEncoding.EncodeToString([]byte(sourceClaimPath)),
		"retry_claim_b64=" + base64.RawURLEncoding.EncodeToString([]byte(retryClaimPath)),
		"queue_dispatch_b64=" + base64.RawURLEncoding.EncodeToString([]byte(queueDispatchPath)),
		"request_path_b64=" + base64.RawURLEncoding.EncodeToString([]byte(requestPath)),
		"worker_b64=" + base64.RawURLEncoding.EncodeToString([]byte(workerID)),
		"run_b64=" + base64.RawURLEncoding.EncodeToString([]byte(runID)),
		"execution_b64=" + base64.RawURLEncoding.EncodeToString([]byte(executionID)),
		"reason_b64=" + base64.RawURLEncoding.EncodeToString([]byte(reason)),
	} {
		if !strings.Contains(resultRow, want) {
			t.Fatalf("expected projected worker dispatch retry execution result row %q in row:\n%s", want, resultRow)
		}
	}
}

func TestToolchainInstallExecutionResultProjectionIncludesEncodedOutputText(t *testing.T) {
	output := "install output artifact=/tmp/ouro/evil-execution.json dispatch=/tmp/ouro/evil-dispatch.json command=evil"
	execution := ToolchainInstallDispatchExecution{
		Path:         "/tmp/ouro/execution.json",
		Status:       "completed",
		Mode:         "execute",
		DispatchPath: "/tmp/ouro/dispatch.json",
		Results: []ToolchainInstallDispatchExecutionResult{{
			Pack:       "web-api-pack",
			Dependency: "missing-httpx",
			Command:    []string{"sh", "-c", "echo output=evil artifact=/tmp/ouro/evil-execution.json"},
			Status:     "completed",
			Output:     output,
		}},
	}

	rows := installExecutionArtifactRuntimeRows([]ToolchainInstallDispatchExecution{execution})
	row := requireSingleRuntimeRowWithPrefix(t, rows, "execution result web-api-pack/missing-httpx:")
	for _, want := range []string{
		"output_b64=" + base64.RawURLEncoding.EncodeToString([]byte(output)),
		"command_b64=" + base64.RawURLEncoding.EncodeToString([]byte(strings.Join(execution.Results[0].Command, " "))),
	} {
		if !strings.Contains(row, want) {
			t.Fatalf("expected projected install execution result row %q in row:\n%s", want, row)
		}
	}
}

func TestToolchainInstallExecutionProjectionIncludesEncodedReferenceFields(t *testing.T) {
	executionPath := "/tmp/ouro/toolchain/install executions/execution status=failed.json"
	dispatchPath := "/tmp/ouro/toolchain/install dispatches/dispatch mode=execute.json"
	execution := ToolchainInstallDispatchExecution{
		Path:         executionPath,
		Status:       "completed",
		Mode:         "execute",
		DispatchPath: dispatchPath,
		Results: []ToolchainInstallDispatchExecutionResult{{
			Pack:       "web-api-pack",
			Dependency: "missing-httpx",
			Command:    []string{"sh", "-c", "echo artifact=/tmp/ouro/evil-execution.json dispatch=/tmp/ouro/evil-dispatch.json"},
			Status:     "completed",
			Output:     "install output artifact=/tmp/ouro/evil-execution.json dispatch=/tmp/ouro/evil-dispatch.json",
		}},
	}

	rows := installExecutionArtifactRuntimeRows([]ToolchainInstallDispatchExecution{execution})
	artifactRow := requireSingleRuntimeRowWithPrefix(t, rows, "execution artifact:")
	for _, want := range []string{
		"execution artifact: " + compactRuntimeRowValue(executionPath) + " artifact_b64=",
		" dispatch=" + compactRuntimeRowValue(dispatchPath) + " dispatch_b64=",
		"artifact_b64=" + base64.RawURLEncoding.EncodeToString([]byte(executionPath)),
		"dispatch_b64=" + base64.RawURLEncoding.EncodeToString([]byte(dispatchPath)),
	} {
		if !strings.Contains(artifactRow, want) {
			t.Fatalf("expected projected install execution artifact row %q in row:\n%s", want, artifactRow)
		}
	}
	resultRow := requireSingleRuntimeRowWithPrefix(t, rows, "execution result web-api-pack/missing-httpx:")
	for _, want := range []string{
		"artifact=" + compactRuntimeRowValue(executionPath) + " artifact_b64=",
		"dispatch=" + compactRuntimeRowValue(dispatchPath) + " dispatch_b64=",
		"artifact_b64=" + base64.RawURLEncoding.EncodeToString([]byte(executionPath)),
		"dispatch_b64=" + base64.RawURLEncoding.EncodeToString([]byte(dispatchPath)),
	} {
		if !strings.Contains(resultRow, want) {
			t.Fatalf("expected projected install execution result row %q in row:\n%s", want, resultRow)
		}
	}
}

func TestToolchainInstallDispatchProjectionIncludesEncodedReferenceFields(t *testing.T) {
	dispatchPath := "/tmp/ouro/toolchain/install dispatches/dispatch status=failed.json"
	rows := installDispatchArtifactRuntimeRows([]ToolchainInstallDispatch{{
		Path:   dispatchPath,
		Status: "dry-run",
		Mode:   "metadata-only",
		Ready: []ToolchainInstallRequest{{
			Pack:       "web-api-pack",
			Dependency: "missing-httpx",
		}},
	}})
	row := requireSingleRuntimeRowWithPrefix(t, rows, "dispatch artifact:")
	for _, want := range []string{
		"dispatch artifact: " + compactRuntimeRowValue(dispatchPath) + " artifact_b64=",
		"artifact_b64=" + base64.RawURLEncoding.EncodeToString([]byte(dispatchPath)),
	} {
		if !strings.Contains(row, want) {
			t.Fatalf("expected projected install dispatch row %q in row:\n%s", want, row)
		}
	}
}

func TestToolchainInstallOrchestrationProjectionIncludesEncodedErrorText(t *testing.T) {
	errorText := "orchestration failed status=evil mode=execute dispatch=/tmp/ouro/evil-dispatch.json"
	orchestration := ToolchainInstallOrchestration{
		Path:              "/tmp/ouro/orchestration.json",
		Status:            "failed",
		Mode:              "execute",
		DispatchReady:     1,
		DispatchSkipped:   2,
		ExecutionResults:  3,
		DispatchPath:      "/tmp/ouro/dispatch.json",
		ExecutionPath:     "/tmp/ouro/execution.json",
		RetryDispatchPath: "/tmp/ouro/retry-dispatch.json",
		Error:             errorText,
	}

	rows := installOrchestrationArtifactRuntimeRows([]ToolchainInstallOrchestration{orchestration})
	row := requireSingleRuntimeRowWithPrefix(t, rows, "install orchestration artifact:")
	for _, want := range []string{
		"error=" + compactRuntimeRowValue(errorText),
		"error_b64=" + base64.RawURLEncoding.EncodeToString([]byte(errorText)),
	} {
		if !strings.Contains(row, want) {
			t.Fatalf("expected projected install orchestration row %q in row:\n%s", want, row)
		}
	}
}

func TestToolchainInstallOrchestrationProjectionIncludesEncodedReferenceFields(t *testing.T) {
	artifactPath := "/tmp/ouro/toolchain/install orchestrations/orchestration status=failed.json"
	dispatchPath := "/tmp/ouro/toolchain/install dispatches/dispatch mode=execute.json"
	executionPath := "/tmp/ouro/toolchain/install executions/execution retry_dispatch=/tmp/ouro/evil-retry.json"
	retryDispatchPath := "/tmp/ouro/toolchain/install retry dispatches/retry mode=execute.json"
	errorText := "orchestration failed status=evil mode=execute dispatch=/tmp/ouro/evil-dispatch.json"
	orchestration := ToolchainInstallOrchestration{
		Path:              artifactPath,
		Status:            "failed",
		Mode:              "execute",
		DispatchReady:     1,
		DispatchSkipped:   2,
		ExecutionResults:  3,
		DispatchPath:      dispatchPath,
		ExecutionPath:     executionPath,
		RetryDispatchPath: retryDispatchPath,
		Error:             errorText,
	}

	rows := installOrchestrationArtifactRuntimeRows([]ToolchainInstallOrchestration{orchestration})
	row := requireSingleRuntimeRowWithPrefix(t, rows, "install orchestration artifact:")
	for _, want := range []string{
		"install orchestration artifact: " + compactRuntimeRowValue(artifactPath) + " artifact_b64=",
		"dispatch=" + compactRuntimeRowValue(dispatchPath) + " dispatch_b64=",
		"execution=" + compactRuntimeRowValue(executionPath) + " execution_b64=",
		"retry_dispatch=" + compactRuntimeRowValue(retryDispatchPath) + " retry_dispatch_b64=",
		"artifact_b64=" + base64.RawURLEncoding.EncodeToString([]byte(artifactPath)),
		"dispatch_b64=" + base64.RawURLEncoding.EncodeToString([]byte(dispatchPath)),
		"execution_b64=" + base64.RawURLEncoding.EncodeToString([]byte(executionPath)),
		"retry_dispatch_b64=" + base64.RawURLEncoding.EncodeToString([]byte(retryDispatchPath)),
	} {
		if !strings.Contains(row, want) {
			t.Fatalf("expected projected install orchestration row %q in row:\n%s", want, row)
		}
	}
}

func TestToolchainRetryExecutionResultProjectionIncludesEncodedOutputErrorText(t *testing.T) {
	output := "retry output artifact=/tmp/ouro/evil-retry-execution.json dispatch=/tmp/ouro/evil-dispatch.json command=evil"
	errorText := "retry failed retry_dispatch=/tmp/ouro/evil-retry-dispatch.json source_execution=/tmp/ouro/evil-source.json"
	execution := ToolchainInstallRetryDispatchExecution{
		Path:              "/tmp/ouro/retry-execution.json",
		Status:            "failed",
		Mode:              "execute",
		RetryDispatchPath: "/tmp/ouro/retry-dispatch.json",
		Results: []ToolchainInstallRetryDispatchResult{{
			ExecutionPath: "/tmp/ouro/source-execution.json",
			DispatchPath:  "/tmp/ouro/install-dispatch.json",
			Pack:          "web-api-pack",
			Dependency:    "missing-httpx",
			Command:       []string{"sh", "-c", "echo error=evil retry_dispatch=/tmp/ouro/evil-retry.json"},
			Status:        "failed",
			Output:        output,
			Error:         errorText,
		}},
	}

	rows := installRetryExecutionArtifactRuntimeRows([]ToolchainInstallRetryDispatchExecution{execution})
	row := requireSingleRuntimeRowWithPrefix(t, rows, "retry execution result web-api-pack/missing-httpx:")
	for _, want := range []string{
		"output_b64=" + base64.RawURLEncoding.EncodeToString([]byte(output)),
		"error_b64=" + base64.RawURLEncoding.EncodeToString([]byte(errorText)),
		"command_b64=" + base64.RawURLEncoding.EncodeToString([]byte(strings.Join(execution.Results[0].Command, " "))),
	} {
		if !strings.Contains(row, want) {
			t.Fatalf("expected projected retry execution result row %q in row:\n%s", want, row)
		}
	}
	refsRow := requireSingleRuntimeRowWithPrefix(t, rows, "retry execution result refs web-api-pack/missing-httpx:")
	if want := "command_b64=" + base64.RawURLEncoding.EncodeToString([]byte(strings.Join(execution.Results[0].Command, " "))); !strings.Contains(refsRow, want) {
		t.Fatalf("expected projected retry execution refs row %q in row:\n%s", want, refsRow)
	}
}

func TestToolchainInstallRetryProjectionIncludesEncodedReferenceFields(t *testing.T) {
	retryDispatchPath := "/tmp/ouro/toolchain/install retry dispatches/retry dispatch status=failed.json"
	retryExecutionPath := "/tmp/ouro/toolchain/install retry executions/retry execution mode=execute.json"
	sourceExecutionPath := "/tmp/ouro/toolchain/install executions/source execution status=failed.json"
	dispatchPath := "/tmp/ouro/toolchain/install dispatches/dispatch mode=execute.json"
	retryRows := installRetryDispatchArtifactRuntimeRows([]ToolchainInstallRetryDispatch{{
		Path:   retryDispatchPath,
		Status: "dry-run",
		Mode:   "metadata-only",
		Ready: []ToolchainInstallRetryCandidate{{
			Pack:       "web-api-pack",
			Dependency: "missing-httpx",
		}},
	}})
	retryDispatchRow := requireSingleRuntimeRowWithPrefix(t, retryRows, "retry dispatch artifact:")
	for _, want := range []string{
		"retry dispatch artifact: " + compactRuntimeRowValue(retryDispatchPath) + " artifact_b64=",
		"artifact_b64=" + base64.RawURLEncoding.EncodeToString([]byte(retryDispatchPath)),
	} {
		if !strings.Contains(retryDispatchRow, want) {
			t.Fatalf("expected projected retry dispatch row %q in row:\n%s", want, retryDispatchRow)
		}
	}

	execution := ToolchainInstallRetryDispatchExecution{
		Path:              retryExecutionPath,
		Status:            "failed",
		Mode:              "execute",
		RetryDispatchPath: retryDispatchPath,
		Results: []ToolchainInstallRetryDispatchResult{{
			ExecutionPath: sourceExecutionPath,
			DispatchPath:  dispatchPath,
			Pack:          "web-api-pack",
			Dependency:    "missing-httpx",
			Command:       []string{"sh", "-c", "echo retry_dispatch=/tmp/ouro/evil-retry.json source_execution=/tmp/ouro/evil-source.json"},
			Status:        "failed",
			Output:        "retry output",
			Error:         "retry failed",
		}},
	}
	rows := installRetryExecutionArtifactRuntimeRows([]ToolchainInstallRetryDispatchExecution{execution})
	artifactRow := requireSingleRuntimeRowWithPrefix(t, rows, "retry execution artifact:")
	for _, want := range []string{
		"retry execution artifact: " + compactRuntimeRowValue(retryExecutionPath) + " artifact_b64=",
		"retry_dispatch=" + compactRuntimeRowValue(retryDispatchPath) + " retry_dispatch_b64=",
		"artifact_b64=" + base64.RawURLEncoding.EncodeToString([]byte(retryExecutionPath)),
		"retry_dispatch_b64=" + base64.RawURLEncoding.EncodeToString([]byte(retryDispatchPath)),
	} {
		if !strings.Contains(artifactRow, want) {
			t.Fatalf("expected projected retry execution artifact row %q in row:\n%s", want, artifactRow)
		}
	}
	resultRow := requireSingleRuntimeRowWithPrefix(t, rows, "retry execution result web-api-pack/missing-httpx:")
	refsRow := requireSingleRuntimeRowWithPrefix(t, rows, "retry execution result refs web-api-pack/missing-httpx:")
	summaryRow := requireSingleRuntimeRowWithPrefix(t, rows, "retry execution result refs summary web-api-pack/missing-httpx:")
	for _, row := range []string{resultRow, refsRow} {
		for _, want := range []string{
			"artifact=" + compactRuntimeRowValue(retryExecutionPath) + " artifact_b64=",
			"retry_dispatch=" + compactRuntimeRowValue(retryDispatchPath) + " retry_dispatch_b64=",
			"source_execution=" + compactRuntimeRowValue(sourceExecutionPath) + " source_execution_b64=",
			"dispatch=" + compactRuntimeRowValue(dispatchPath) + " dispatch_b64=",
			"artifact_b64=" + base64.RawURLEncoding.EncodeToString([]byte(retryExecutionPath)),
			"retry_dispatch_b64=" + base64.RawURLEncoding.EncodeToString([]byte(retryDispatchPath)),
			"source_execution_b64=" + base64.RawURLEncoding.EncodeToString([]byte(sourceExecutionPath)),
			"dispatch_b64=" + base64.RawURLEncoding.EncodeToString([]byte(dispatchPath)),
		} {
			if !strings.Contains(row, want) {
				t.Fatalf("expected projected retry execution row %q in row:\n%s", want, row)
			}
		}
	}
	for _, want := range []string{
		"artifact=" + compactRuntimeRowValue(filepath.Base(retryExecutionPath)) + " artifact_b64=",
		"retry_dispatch=" + compactRuntimeRowValue(filepath.Base(retryDispatchPath)) + " retry_dispatch_b64=",
		"source_execution=" + compactRuntimeRowValue(filepath.Base(sourceExecutionPath)) + " source_execution_b64=",
		"dispatch=" + compactRuntimeRowValue(filepath.Base(dispatchPath)) + " dispatch_b64=",
		"artifact_b64=" + base64.RawURLEncoding.EncodeToString([]byte(filepath.Base(retryExecutionPath))),
		"retry_dispatch_b64=" + base64.RawURLEncoding.EncodeToString([]byte(filepath.Base(retryDispatchPath))),
		"source_execution_b64=" + base64.RawURLEncoding.EncodeToString([]byte(filepath.Base(sourceExecutionPath))),
		"dispatch_b64=" + base64.RawURLEncoding.EncodeToString([]byte(filepath.Base(dispatchPath))),
	} {
		if !strings.Contains(summaryRow, want) {
			t.Fatalf("expected projected retry execution summary row %q in row:\n%s", want, summaryRow)
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
	skipIfWindows(t)
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

func TestWorkerLifecyclePolicySelectionProjectionIncludesEncodedReasonText(t *testing.T) {
	reason := "operator approved policy=/tmp/ouro/evil-policy.json selection=/tmp/ouro/evil-selection.json selected_by=mallory cleared_at=2030-01-01T00:00:00Z superseded=99"
	policy := WorkerLifecyclePolicy{
		Path:                     "/tmp/ouro/runtime/worker-lifecycle-policies/policy-b.json",
		Status:                   "dry-run",
		Mode:                     "metadata-only",
		TransportMode:            "metadata-only",
		PollIntervalMilliseconds: 250,
		MaxSupervisorRuns:        3,
	}
	selection := WorkerLifecyclePolicySelection{
		Path:                         "/tmp/ouro/runtime/worker-lifecycle-policy-selection.json",
		Status:                       "current",
		Mode:                         "metadata-only",
		PolicyPath:                   policy.Path,
		TransportMode:                "metadata-only",
		PollIntervalMilliseconds:     250,
		MaxSupervisorRuns:            3,
		StaleAfterSeconds:            300,
		WorkerStaleAfterSeconds:      120,
		IncludeLifecycleMaintenance:  true,
		ExecuteLifecycleRetries:      true,
		DispatchLifecycleRetryClaims: true,
		LifecycleRetryTransportMode:  "file-outbox",
		ExecuteLeaseRecovery:         true,
		SupersededPolicyPaths:        []string{"/tmp/ouro/runtime/worker-lifecycle-policies/policy-a.json"},
		SelectedBy:                   "alice",
		SelectedAt:                   "2026-06-01T17:10:00Z",
		Reason:                       reason,
	}

	rows := strings.Join(workerLifecyclePolicyArtifactRuntimeRowsWithSelection([]WorkerLifecyclePolicy{policy}, selection, true), "\n")
	for _, want := range []string{
		"reason_b64=" + base64.RawURLEncoding.EncodeToString([]byte(reason)),
		"policy_b64=" + base64.RawURLEncoding.EncodeToString([]byte(selection.PolicyPath)),
		"selection_b64=" + base64.RawURLEncoding.EncodeToString([]byte(selection.Path)),
		"selected_by_b64=" + base64.RawURLEncoding.EncodeToString([]byte(selection.SelectedBy)),
		"selected_at_b64=" + base64.RawURLEncoding.EncodeToString([]byte(selection.SelectedAt)),
		"artifact_b64=" + base64.RawURLEncoding.EncodeToString([]byte(policy.Path)),
	} {
		if !strings.Contains(rows, want) {
			t.Fatalf("expected projected worker lifecycle policy selection row %q in rows:\n%s", want, rows)
		}
	}
}

func TestNewTUIModelForWorkspaceShowsClearedWorkerLifecyclePolicySelectionDetails(t *testing.T) {
	root, base, candidate := createWorkerLifecyclePolicySelectionFixture(t)
	skipIfWindows(t)
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
		"worker lifecycle policy run artifact: " + run.Path + " artifact_b64=" + base64.RawURLEncoding.EncodeToString([]byte(run.Path)),
		"status=dry-run mode=metadata-only policy=" + policy.Path + " policy_b64=" + base64.RawURLEncoding.EncodeToString([]byte(policy.Path)),
		"selection=" + selectionPath + " selection_b64=" + base64.RawURLEncoding.EncodeToString([]byte(selectionPath)),
		"max_runs=2 poll_interval_ms=500 transport=metadata-only include_lifecycle_maintenance=true execute_lifecycle_retries=true execute_lease_recovery=false supervisor_created=0 skip=none",
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
		"worker lifecycle policy run artifact: " + run.Path + " artifact_b64=" + base64.RawURLEncoding.EncodeToString([]byte(run.Path)),
		"status=completed mode=metadata-only policy=" + policy.Path + " policy_b64=" + base64.RawURLEncoding.EncodeToString([]byte(policy.Path)),
		"selection=" + selectionPath + " selection_b64=" + base64.RawURLEncoding.EncodeToString([]byte(selectionPath)),
		"max_runs=2 poll_interval_ms=500 transport=metadata-only include_lifecycle_maintenance=true execute_lifecycle_retries=true execute_lease_recovery=false supervisor_created=1 skip=none",
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

func TestWorkerLifecyclePolicyRunProjectionIncludesEncodedSkipText(t *testing.T) {
	skipReason := "policy run disabled policy=/tmp/ouro/evil-policy.json selection=/tmp/ouro/evil-selection.json max_runs=99"
	run := WorkerLifecyclePolicyRun{
		Path:                         "/tmp/ouro/runtime/worker-lifecycle-policy-runs/run-001.json",
		Status:                       "skipped",
		Mode:                         "metadata-only",
		PolicyPath:                   "/tmp/ouro/runtime/worker-lifecycle-policies/policy-b.json",
		SelectionPath:                "/tmp/ouro/runtime/worker-lifecycle-policy-selection.json",
		MaxRuns:                      3,
		PollIntervalMilliseconds:     250,
		TransportMode:                "metadata-only",
		IncludeLifecycleMaintenance:  true,
		ExecuteLifecycleRetries:      false,
		DispatchLifecycleRetryClaims: true,
		LifecycleRetryTransportMode:  "file-outbox",
		ExecuteLeaseRecovery:         true,
		SkipReason:                   skipReason,
	}

	rows := workerLifecyclePolicyRunRuntimeRows([]WorkerLifecyclePolicyRun{run})
	artifactRow := requireSingleRuntimeRowWithPrefix(t, rows, "worker lifecycle policy run artifact:")
	for _, want := range []string{
		"artifact_b64=" + base64.RawURLEncoding.EncodeToString([]byte(run.Path)),
		"policy_b64=" + base64.RawURLEncoding.EncodeToString([]byte(run.PolicyPath)),
		"selection_b64=" + base64.RawURLEncoding.EncodeToString([]byte(run.SelectionPath)),
		"skip_b64=" + base64.RawURLEncoding.EncodeToString([]byte(skipReason)),
	} {
		if !strings.Contains(artifactRow, want) {
			t.Fatalf("expected projected worker lifecycle policy run artifact row %q in row:\n%s", want, artifactRow)
		}
	}
	skipRow := requireSingleRuntimeRowWithPrefix(t, rows, "worker lifecycle policy run skip:")
	if !strings.Contains(skipRow, compactRuntimeRowValue(skipReason)) {
		t.Fatalf("expected display-only worker lifecycle policy run skip text in row:\n%s", skipRow)
	}
	requireNoRuntimeSidecarFields(t, skipRow)
}

func TestWorkerLifecyclePolicySchedulerRunProjectionIncludesEncodedSkipErrorText(t *testing.T) {
	skipReason := "scheduler disabled policy=/tmp/ouro/evil-policy.json selection=/tmp/ouro/evil-selection.json error=skip-embedded"
	errorText := "failed to open policy policy=/tmp/ouro/evil-policy.json selection=/tmp/ouro/evil-selection.json policy_runs_created=99"
	run := WorkerLifecyclePolicySchedulerRun{
		Path:                        "/tmp/ouro/runtime/worker-lifecycle-policy-scheduler-runs/scheduler-001.json",
		Status:                      "failed",
		Mode:                        "metadata-only",
		PolicyPath:                  "/tmp/ouro/runtime/worker-lifecycle-policies/policy-b.json",
		SelectionPath:               "/tmp/ouro/runtime/worker-lifecycle-policy-selection.json",
		PolicyBasename:              "policy-b.json",
		MaxRunsRequested:            3,
		PollIntervalMilliseconds:    250,
		TransportMode:               "metadata-only",
		IncludeLifecycleMaintenance: true,
		ExecuteLifecycleRetries:     false,
		ExecuteLeaseRecovery:        true,
		PolicyRunsCreated:           2,
		SupervisorsCreated:          1,
		SkipReason:                  skipReason,
		Error:                       errorText,
	}

	rows := workerLifecyclePolicySchedulerRunRuntimeRows([]WorkerLifecyclePolicySchedulerRun{run})
	artifactRow := requireSingleRuntimeRowWithPrefix(t, rows, "worker lifecycle policy scheduler run artifact:")
	for _, want := range []string{
		"artifact_b64=" + base64.RawURLEncoding.EncodeToString([]byte(run.Path)),
		"policy_b64=" + base64.RawURLEncoding.EncodeToString([]byte(run.PolicyPath)),
		"selection_b64=" + base64.RawURLEncoding.EncodeToString([]byte(run.SelectionPath)),
		"skip_b64=" + base64.RawURLEncoding.EncodeToString([]byte(skipReason)),
		"error_b64=" + base64.RawURLEncoding.EncodeToString([]byte(errorText)),
	} {
		if !strings.Contains(artifactRow, want) {
			t.Fatalf("expected projected worker lifecycle policy scheduler artifact row %q in row:\n%s", want, artifactRow)
		}
	}
	skipRow := requireSingleRuntimeRowWithPrefix(t, rows, "worker lifecycle policy scheduler run skip:")
	if !strings.Contains(skipRow, compactRuntimeRowValue(skipReason)) {
		t.Fatalf("expected display-only worker lifecycle policy scheduler skip text in row:\n%s", skipRow)
	}
	requireNoRuntimeSidecarFields(t, skipRow)
	errorRow := requireSingleRuntimeRowWithPrefix(t, rows, "worker lifecycle policy scheduler run error:")
	if !strings.Contains(errorRow, compactRuntimeRowValue(errorText)) {
		t.Fatalf("expected display-only worker lifecycle policy scheduler error text in row:\n%s", errorRow)
	}
	requireNoRuntimeSidecarFields(t, errorRow)
}

func TestWorkerDispatchRetrySchedulerRunProjectionIncludesEncodedSkipErrorText(t *testing.T) {
	skipReason := "retry scheduler disabled error=skip-embedded retry_transport=http-json retry_plans_created=99"
	errorText := "failed to create retry dispatch retry_transport=http-json retry_dispatch_claims_created=99"
	run := RuntimeWorkerDispatchRetrySchedulerRun{
		Path:                           "/tmp/ouro/runtime/worker-dispatch-retry-scheduler-runs/scheduler-001.json",
		Status:                         "failed",
		Mode:                           "metadata-only",
		MaxRunsRequested:               2,
		DispatchRetryClaims:            true,
		RetryTransportMode:             "file-outbox",
		RetryPlansCreated:              2,
		RetryDispatchesCreated:         2,
		RetryDispatchExecutionsCreated: 2,
		RetryClaimsCreated:             3,
		RetryClaimsDispatched:          2,
		RetryTransportRequestsCreated:  2,
		SkipReason:                     skipReason,
		Error:                          errorText,
	}

	rows := runtimeWorkerDispatchRetrySchedulerRunRuntimeRows([]RuntimeWorkerDispatchRetrySchedulerRun{run})
	artifactRow := requireSingleRuntimeRowWithPrefix(t, rows, "worker dispatch retry scheduler run artifact:")
	for _, want := range []string{
		"skip_b64=" + base64.RawURLEncoding.EncodeToString([]byte(skipReason)),
		"error_b64=" + base64.RawURLEncoding.EncodeToString([]byte(errorText)),
	} {
		if !strings.Contains(artifactRow, want) {
			t.Fatalf("expected projected worker dispatch retry scheduler artifact row %q in row:\n%s", want, artifactRow)
		}
	}
	skipRow := requireSingleRuntimeRowWithPrefix(t, rows, "worker dispatch retry scheduler run skip:")
	if !strings.Contains(skipRow, compactRuntimeRowValue(skipReason)) {
		t.Fatalf("expected display-only worker dispatch retry scheduler skip text in row:\n%s", skipRow)
	}
	requireNoRuntimeSidecarFields(t, skipRow)
	errorRow := requireSingleRuntimeRowWithPrefix(t, rows, "worker dispatch retry scheduler run error:")
	if !strings.Contains(errorRow, compactRuntimeRowValue(errorText)) {
		t.Fatalf("expected display-only worker dispatch retry scheduler error text in row:\n%s", errorRow)
	}
	requireNoRuntimeSidecarFields(t, errorRow)
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
		"worker lifecycle policy scheduler run artifact: " + schedulerPath + " artifact_b64=" + base64.RawURLEncoding.EncodeToString([]byte(schedulerPath)),
		"status=failed mode=metadata-only policy=" + policy.Path + " policy_b64=" + base64.RawURLEncoding.EncodeToString([]byte(policy.Path)),
		"selection=" + selectionPath + " selection_b64=" + base64.RawURLEncoding.EncodeToString([]byte(selectionPath)),
		"policy_basename=" + filepath.Base(policy.Path) + " policy_basename_b64=" + base64.RawURLEncoding.EncodeToString([]byte(filepath.Base(policy.Path))),
		"selection_basename=worker-lifecycle-policy-selection.json selection_basename_b64=" + base64.RawURLEncoding.EncodeToString([]byte("worker-lifecycle-policy-selection.json")),
		"max_runs=2 poll_interval_ms=500 transport=metadata-only include_lifecycle_maintenance=true execute_lifecycle_retries=true execute_lease_recovery=false policy_runs_created=1 supervisor_created=1 skip=policy run disabled by operator skip_b64=" + base64.RawURLEncoding.EncodeToString([]byte("policy run disabled by operator")) + " error=scheduler failed to open policy error_b64=" + base64.RawURLEncoding.EncodeToString([]byte("scheduler failed to open policy")),
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

func TestNewTUIModelForWorkspaceShowsCompilePreviewReviewExecutionDispatchRowsWithExecutableDetails(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	review, err := CreateToolCompilePreviewReview(root, ToolCompilePreviewOptions{
		Action: compiler.ActionDiscoverAssets,
		Budget: compiler.CompileBudget{MaxPlanNodes: 1},
	})
	if err != nil {
		t.Fatalf("create compile preview review: %v", err)
	}
	if _, err := SelectToolCompilePreviewReview(root, filepath.Base(review.Path), ToolCompilePreviewReviewSelectionOptions{SelectedBy: "closed-loop-planner"}); err != nil {
		t.Fatalf("select compile preview review: %v", err)
	}
	createdAt := time.Date(2026, 6, 4, 12, 15, 0, 0, time.UTC)
	dispatch, err := CreateToolCompilePreviewReviewExecutionDispatch(root, filepath.Base(review.Path), ToolCompilePreviewReviewExecutionDispatchOptions{
		Actor:     "operator",
		Reason:    "ready for execution gate",
		CreatedAt: createdAt,
	})
	if err != nil {
		t.Fatalf("create compile preview review execution dispatch: %v", err)
	}
	if strings.TrimSpace(dispatch.Path) == "" {
		t.Fatalf("expected dispatch artifact path")
	}

	model := NewTUIModelForWorkspace(root).SelectRuntimeTab(tui.RuntimeTabToolchainHealth)
	resized, cmd := model.Update(tea.WindowSizeMsg{Width: 720, Height: 44})
	if cmd != nil {
		t.Fatalf("expected resize update to be synchronous")
	}
	view := resized.View()
	for _, want := range []string{
		"compile-preview review execution dispatches: total=1",
		"compile-preview review execution dispatch artifact: status=dry-run mode=metadata-only action=discover_assets",
		"nodes=1",
		"warnings=1",
		"actor=operator",
		"created_at=2026-06-04T12:15:00Z",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected TUI compile-preview review execution dispatch row %q:\n%s", want, view)
		}
	}

	model = selectRuntimeRowContaining(t, model, "> compile-preview review execution dispatch artifact:")
	model = model.ShowRuntimeActionDetails()
	resized, cmd = model.Update(tea.WindowSizeMsg{Width: 720, Height: 44})
	if cmd != nil {
		t.Fatalf("expected runtime action details resize to be synchronous")
	}
	view = resized.View()
	for _, want := range []string{
		"[Node Inspector]",
		"runtime action: compile-preview-review-execution-dispatch-artifact",
		"artifact path: ",
		"compile-preview-review-execution-dispatches",
		"review path: ",
		"compile-preview-reviews",
		"request path: ",
		"compile-preview-review-selection.json",
		"action type: discover_assets",
		"nodes: 1",
		"warnings: 1",
		"created by: operator",
		"created at: 2026-06-04T12:15:00Z",
		"reason: ready for execution gate",
		"mode: metadata-only",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected compile-preview review execution dispatch details %q:\n%s", want, view)
		}
	}
}

func TestNewTUIModelForWorkspaceExecutesCompilePreviewReviewExecutionDispatchAfterDoubleConfirm(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	skipIfWindows(t)
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	marker := filepath.Join(t.TempDir(), "compile-preview-dispatch-tui-ran")
	scope := `version: 1
scope:
  target_policy: enforce
  targets:
    - kind: url
      value: https://example.test
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
  - sh
  - -c
  - true
parser: web_api_parser
healthcheck: built_in
`
	if err := os.WriteFile(filepath.Join(root, "toolchain", "web-api-pack", "manifest.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	reviewPath := writeCompilePreviewReviewFixture(t, root, "tui-executable-review.json", ToolCompilePreviewReview{
		Status: "dry-run",
		Mode:   "metadata-only",
		Preview: ToolCompilePreview{
			Workspace: root,
			Action:    compiler.ActionDiscoverAssets,
			Mode:      "metadata-only",
			Nodes: []compiler.PlanNode{
				{
					ID:           "discover_assets:01",
					Action:       compiler.ActionDiscoverAssets,
					ManifestName: "web-api-pack",
					ToolID:       "tui-safe-touch",
					AdapterID:    "command",
					Operation:    "touch_marker",
					Capability:   "web.discovery",
					TargetKind:   "url",
					Runner:       "command",
					RunCommand:   []string{"sh", "-c", "printf tui-executed > " + marker + "; echo tui-executed"},
					Parser:       "web_api_parser",
				},
			},
		},
	})
	if _, err := SelectToolCompilePreviewReview(root, filepath.Base(reviewPath), ToolCompilePreviewReviewSelectionOptions{
		SelectedBy: "closed-loop-planner",
		Reason:     "planner-selected review",
	}); err != nil {
		t.Fatalf("select compile preview review: %v", err)
	}
	dispatch, err := CreateToolCompilePreviewReviewExecutionDispatch(root, filepath.Base(reviewPath), ToolCompilePreviewReviewExecutionDispatchOptions{
		Actor:  "operator",
		Reason: "ready for TUI execution",
	})
	if err != nil {
		t.Fatalf("create compile preview review execution dispatch: %v", err)
	}

	model := NewTUIModelForWorkspace(root).SelectRuntimeTab(tui.RuntimeTabToolchainHealth)
	model = selectRuntimeRowContaining(t, model, "> compile-preview review execution dispatch artifact:")
	armed, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if cmd != nil {
		t.Fatalf("first x should only arm execution confirmation")
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("first confirmation step must not execute review nodes, marker stat err=%v", err)
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
	if data, err := os.ReadFile(marker); err != nil || strings.TrimSpace(string(data)) != "tui-executed" {
		t.Fatalf("expected double-confirmed TUI execution to run frozen node, data=%q err=%v", data, err)
	}
	executions, err := ListToolCompilePreviewReviewExecutions(root)
	if err != nil {
		t.Fatalf("list compile preview review executions: %v", err)
	}
	if len(executions) != 1 || executions[0].DispatchPath != dispatch.Path {
		t.Fatalf("expected one execution linked to dispatch, got %#v dispatch=%s", executions, dispatch.Path)
	}
	if executions[0].Actor != "tui" || executions[0].Reason != "executed from TUI" {
		t.Fatalf("expected TUI execution metadata, got %#v", executions[0])
	}
	confirmedModel, ok := confirmed.(tui.Model)
	if !ok {
		t.Fatalf("expected TUI model after execution, got %T", confirmed)
	}
	resized, cmd := confirmedModel.Update(tea.WindowSizeMsg{Width: 720, Height: 44})
	if cmd != nil {
		t.Fatalf("expected resize update to be synchronous")
	}
	view := resized.View()
	for _, want := range []string{
		"compile-preview review execution artifact: status=completed mode=execute action=discover_assets",
		"actor=tui",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected TUI execution row %q:\n%s", want, view)
		}
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
	dispatchReason := "operator review planned_report=/tmp/ouro/evil.md run=evil"
	dispatch, err := CreateReportProjectionDispatch(context.Background(), root, run.RunID, ReportProjectionDispatchOptions{
		Now:    time.Date(2026, 6, 2, 4, 30, 0, 0, time.UTC),
		Actor:  "alice",
		Reason: dispatchReason,
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
	projected := reportProjectionDispatchRuntimeRows(listedDispatches)
	projectedRows := strings.Join(projected, "\n")
	artifactRow := requireSingleRuntimeRowWithPrefix(t, projected, "report projection dispatch artifact: ")
	for _, want := range []string{
		"report projection dispatch artifact: " + dispatch.Path + " artifact_b64=",
		"artifact_b64=" + base64.RawURLEncoding.EncodeToString([]byte(dispatch.Path)),
		"status=dry-run",
		"mode=metadata-only",
		"run=" + run.RunID,
		"events=",
		"planned_report=" + dispatch.ReportPath + " planned_report_b64=",
		"planned_report_b64=" + base64.RawURLEncoding.EncodeToString([]byte(dispatch.ReportPath)),
		"actor=alice actor_b64=",
		"actor_b64=" + base64.RawURLEncoding.EncodeToString([]byte("alice")),
		"created_at=2026-06-02T04:30:00Z created_at_b64=",
		"created_at_b64=" + base64.RawURLEncoding.EncodeToString([]byte("2026-06-02T04:30:00Z")),
		"reason=operator_review_planned_report=/tmp/ouro/evil.md_run=evil",
		"reason_b64=" + base64.RawURLEncoding.EncodeToString([]byte(dispatchReason)),
	} {
		if !strings.Contains(artifactRow, want) {
			t.Fatalf("expected projected report dispatch artifact row %q in row:\n%s", want, artifactRow)
		}
	}
	for _, want := range []string{
		"report projection dispatches: total=1",
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

func TestReportProjectionDispatchProjectionIncludesEncodedReferenceFields(t *testing.T) {
	dispatchPath := "/tmp/ouro/reports/report projection dispatches/dispatch status=evil.json"
	reportPath := "/tmp/ouro/reports/final planned_report=evil.md"
	actor := "operator actor_b64=fake planned_report=/tmp/ouro/evil.md"
	createdAt := "2026-06-02T04:30:00Z"
	dispatch := ReportProjectionDispatch{
		Path:       dispatchPath,
		Status:     "dry-run",
		Mode:       "metadata-only",
		CreatedAt:  createdAt,
		Actor:      actor,
		Reason:     "operator review",
		RunID:      "run-123",
		ReportPath: reportPath,
		EventCount: 42,
	}

	rows := strings.Join(reportProjectionDispatchRuntimeRows([]ReportProjectionDispatch{dispatch}), "\n")
	for _, want := range []string{
		"report projection dispatch artifact: " + dispatchPath + " artifact_b64=",
		"artifact_b64=" + base64.RawURLEncoding.EncodeToString([]byte(dispatchPath)),
		"planned_report=" + reportProjectionDispatchRowValue(reportPath, "not-generated") + " planned_report_b64=",
		"planned_report_b64=" + base64.RawURLEncoding.EncodeToString([]byte(reportPath)),
		"actor=" + reportProjectionDispatchRowValue(actor, "operator") + " actor_b64=",
		"actor_b64=" + base64.RawURLEncoding.EncodeToString([]byte(actor)),
		"created_at=" + reportProjectionDispatchRowValue(createdAt, "unknown") + " created_at_b64=",
		"created_at_b64=" + base64.RawURLEncoding.EncodeToString([]byte(createdAt)),
	} {
		if !strings.Contains(rows, want) {
			t.Fatalf("expected report projection dispatch row to contain %q:\n%s", want, rows)
		}
	}
}

func TestNewTUIModelForWorkspaceExecutesReportProjectionDispatchAfterDoubleConfirm(t *testing.T) {
	ctx := context.Background()
	skipIfWindows(t)
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

func TestNewTUIModelForWorkspaceCreatesReportProjectionDispatchFromWorkflowStatusAfterDoubleConfirm(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "workspace")
	_, preview := createWorkflowStatusPreviewArtifact(t, root)
	if _, err := AcceptModelToolPlanPreviewArtifact(root, filepath.Base(preview.Path), ModelToolPlanPreviewAcceptanceOptions{AcceptedBy: "operator"}); err != nil {
		t.Fatalf("accept preview: %v", err)
	}
	compileDispatch, err := CreateToolCompilePreviewReviewExecutionDispatch(root, preview.ReviewPath, ToolCompilePreviewReviewExecutionDispatchOptions{
		Actor: "operator",
	})
	if err != nil {
		t.Fatalf("create compile dispatch: %v", err)
	}
	if _, err := ExecuteToolCompilePreviewReviewExecutionDispatch(ctx, root, filepath.Base(compileDispatch.Path), ToolCompilePreviewReviewExecutionOptions{
		Execute:    true,
		Actor:      "operator",
		ToolRunner: &recordingToolExecutionRunner{},
	}); err != nil {
		t.Fatalf("execute compile dispatch: %v", err)
	}
	run, err := RunWorkspaceWithOptions(ctx, root, RunOptions{DeferReportProjection: true})
	if err != nil {
		t.Fatalf("run workspace with deferred report projection: %v", err)
	}
	status, err := SummarizeWorkspaceWorkflow(root)
	if err != nil {
		t.Fatalf("summarize workflow: %v", err)
	}
	if status.NextGate != "create-report-projection-dispatch" || status.ReportRunID != run.RunID {
		t.Fatalf("expected report projection dispatch workflow gate, got %#v", status)
	}

	dispatchDir := filepath.Join(root, "reports", "report-projection-dispatches")
	executionDir := filepath.Join(root, "reports", "report-projection-dispatch-executions")
	dispatchesBefore := countJSON(t, dispatchDir)
	executionsBefore := countJSON(t, executionDir)
	reportFilesBefore, err := filepath.Glob(filepath.Join(root, "reports", "*.md"))
	if err != nil {
		t.Fatalf("glob reports before TUI dispatch creation: %v", err)
	}
	queueClaimsBefore := countJSON(t, filepath.Join(root, "runtime", "worker-queue-claims"))
	transportOutboxBefore := countJSON(t, filepath.Join(root, "runtime", "worker-transport-outbox"))
	eventsBefore := reportProjectionDispatchTestEventCount(t, ctx, root, run.RunID)

	model := NewTUIModelForWorkspace(root).SelectRuntimeTab(tui.RuntimeTabModelRuntime)
	model = selectRuntimeRowContaining(t, model, "> workflow status:")
	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if cmd != nil {
		t.Fatalf("first x should only arm workflow report dispatch creation")
	}
	if got := countJSON(t, dispatchDir); got != dispatchesBefore {
		t.Fatalf("first x must not create report projection dispatches, before=%d after=%d", dispatchesBefore, got)
	}
	armed, ok := updated.(tui.Model)
	if !ok {
		t.Fatalf("expected TUI model after first x, got %T", updated)
	}
	resized, resizeCmd := armed.Update(tea.WindowSizeMsg{Width: 1000, Height: 48})
	if resizeCmd != nil {
		t.Fatalf("expected resize after first x to be synchronous")
	}
	for _, want := range []string{
		"action: workflow status:",
		"execution status: confirm-required",
		"execution confirm: press x again",
		"action mode: metadata-only",
	} {
		if !strings.Contains(resized.View(), want) {
			t.Fatalf("expected workflow report dispatch creation confirmation %q:\n%s", want, resized.View())
		}
	}

	updated, cmd = armed.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if cmd == nil {
		t.Fatalf("second x should create report projection dispatch")
	}
	msg := cmd()
	updated, cmd = updated.Update(msg)
	if cmd != nil {
		t.Fatalf("expected workflow report dispatch creation result to be synchronous")
	}
	if got := countJSON(t, dispatchDir); got != dispatchesBefore+1 {
		t.Fatalf("TUI workflow status must create exactly one report projection dispatch, before=%d after=%d", dispatchesBefore, got)
	}
	if got := countJSON(t, executionDir); got != executionsBefore {
		t.Fatalf("TUI workflow status must not execute report projection dispatches, before=%d after=%d", executionsBefore, got)
	}
	reportFilesAfter, err := filepath.Glob(filepath.Join(root, "reports", "*.md"))
	if err != nil {
		t.Fatalf("glob reports after TUI dispatch creation: %v", err)
	}
	if len(reportFilesAfter) != len(reportFilesBefore) {
		t.Fatalf("TUI workflow status must not create markdown reports, before=%v after=%v", reportFilesBefore, reportFilesAfter)
	}
	if replay, err := ReplayWorkspace(ctx, root, run.RunID); err != nil {
		t.Fatalf("replay workspace after TUI dispatch creation: %v", err)
	} else if replay.ReportPath != "" {
		t.Fatalf("TUI workflow status must not append report_ready, got report path %q", replay.ReportPath)
	}
	if eventsAfter := reportProjectionDispatchTestEventCount(t, ctx, root, run.RunID); eventsAfter != eventsBefore {
		t.Fatalf("TUI workflow status must not append replay events, before=%d after=%d", eventsBefore, eventsAfter)
	}
	if got := countJSON(t, filepath.Join(root, "runtime", "worker-queue-claims")); got != queueClaimsBefore {
		t.Fatalf("TUI workflow status must not create worker queue claims, before=%d after=%d", queueClaimsBefore, got)
	}
	if got := countJSON(t, filepath.Join(root, "runtime", "worker-transport-outbox")); got != transportOutboxBefore {
		t.Fatalf("TUI workflow status must not create worker transport requests, before=%d after=%d", transportOutboxBefore, got)
	}
	dispatches, err := ListReportProjectionDispatches(root)
	if err != nil {
		t.Fatalf("list report projection dispatches after TUI workflow action: %v", err)
	}
	if len(dispatches) != dispatchesBefore+1 {
		t.Fatalf("expected one listed TUI report projection dispatch, before=%d dispatches=%#v", dispatchesBefore, dispatches)
	}
	dispatch := dispatches[len(dispatches)-1]
	if dispatch.Status != "dry-run" || dispatch.Mode != "metadata-only" || dispatch.RunID != run.RunID || dispatch.Actor != "tui" || dispatch.Reason != "created from TUI workflow status" {
		t.Fatalf("unexpected TUI workflow report projection dispatch audit: %#v", dispatch)
	}
	completed, ok := updated.(tui.Model)
	if !ok {
		t.Fatalf("expected TUI model after result, got %T", updated)
	}
	resized, resizeCmd = completed.Update(tea.WindowSizeMsg{Width: 1000, Height: 48})
	if resizeCmd != nil {
		t.Fatalf("expected resize after result to be synchronous")
	}
	for _, want := range []string{
		"execution status: completed",
		"action output: report projection dispatch created",
		"report projection dispatches: total=1",
		"report projection dispatch artifact: " + dispatch.Path,
		"status=dry-run mode=metadata-only run=" + run.RunID,
		"actor=tui reason=created_from_TUI_workflow_status",
	} {
		if !strings.Contains(resized.View(), want) {
			t.Fatalf("expected workflow report dispatch creation result view %q:\n%s", want, resized.View())
		}
	}
}

func TestNewTUIModelForWorkspaceExecutesReportProjectionDispatchFromWorkflowStatusAfterDoubleConfirm(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "workspace")
	_, preview := createWorkflowStatusPreviewArtifact(t, root)
	if _, err := AcceptModelToolPlanPreviewArtifact(root, filepath.Base(preview.Path), ModelToolPlanPreviewAcceptanceOptions{AcceptedBy: "operator"}); err != nil {
		t.Fatalf("accept preview: %v", err)
	}
	compileDispatch, err := CreateToolCompilePreviewReviewExecutionDispatch(root, preview.ReviewPath, ToolCompilePreviewReviewExecutionDispatchOptions{
		Actor: "operator",
	})
	if err != nil {
		t.Fatalf("create compile dispatch: %v", err)
	}
	if _, err := ExecuteToolCompilePreviewReviewExecutionDispatch(ctx, root, filepath.Base(compileDispatch.Path), ToolCompilePreviewReviewExecutionOptions{
		Execute:    true,
		Actor:      "operator",
		ToolRunner: &recordingToolExecutionRunner{},
	}); err != nil {
		t.Fatalf("execute compile dispatch: %v", err)
	}
	run, err := RunWorkspaceWithOptions(ctx, root, RunOptions{DeferReportProjection: true})
	if err != nil {
		t.Fatalf("run workspace with deferred report projection: %v", err)
	}
	reportDispatch, err := CreateReportProjectionDispatch(ctx, root, run.RunID, ReportProjectionDispatchOptions{
		Actor:  "operator",
		Reason: "ready for workflow TUI execution",
	})
	if err != nil {
		t.Fatalf("create report projection dispatch: %v", err)
	}
	status, err := SummarizeWorkspaceWorkflow(root)
	if err != nil {
		t.Fatalf("summarize workflow: %v", err)
	}
	if status.NextGate != "execute-report-projection-dispatch" || status.ReportDispatchPath != reportDispatch.Path || status.ReportRunID != run.RunID {
		t.Fatalf("expected report projection dispatch execution workflow gate, got %#v", status)
	}

	dispatchDir := filepath.Join(root, "reports", "report-projection-dispatches")
	executionDir := filepath.Join(root, "reports", "report-projection-dispatch-executions")
	dispatchesBefore := countJSON(t, dispatchDir)
	executionsBefore := countJSON(t, executionDir)
	reportFilesBefore, err := filepath.Glob(filepath.Join(root, "reports", "*.md"))
	if err != nil {
		t.Fatalf("glob reports before TUI workflow execution: %v", err)
	}
	queueClaimsBefore := countJSON(t, filepath.Join(root, "runtime", "worker-queue-claims"))
	transportOutboxBefore := countJSON(t, filepath.Join(root, "runtime", "worker-transport-outbox"))
	eventsBefore := reportProjectionDispatchTestEventCount(t, ctx, root, run.RunID)

	model := NewTUIModelForWorkspace(root).SelectRuntimeTab(tui.RuntimeTabModelRuntime)
	model = selectRuntimeRowContaining(t, model, "> workflow status:")
	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if cmd != nil {
		t.Fatalf("first x should only arm workflow report dispatch execution")
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
	armed, ok := updated.(tui.Model)
	if !ok {
		t.Fatalf("expected TUI model after first x, got %T", updated)
	}
	resized, resizeCmd := armed.Update(tea.WindowSizeMsg{Width: 1000, Height: 48})
	if resizeCmd != nil {
		t.Fatalf("expected resize after first x to be synchronous")
	}
	for _, want := range []string{
		"action: workflow status:",
		"execution status: confirm-required",
		"execution confirm: press x again",
		"action mode: metadata-only",
	} {
		if !strings.Contains(resized.View(), want) {
			t.Fatalf("expected workflow report dispatch execution confirmation %q:\n%s", want, resized.View())
		}
	}

	updated, cmd = armed.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if cmd == nil {
		t.Fatalf("second x should execute report projection dispatch from workflow status")
	}
	msg := cmd()
	updated, cmd = updated.Update(msg)
	if cmd != nil {
		t.Fatalf("expected workflow report dispatch execution result to be synchronous")
	}
	if got := countJSON(t, dispatchDir); got != dispatchesBefore {
		t.Fatalf("TUI workflow status must not create extra report projection dispatches, before=%d after=%d", dispatchesBefore, got)
	}
	if got := countJSON(t, executionDir); got != executionsBefore+1 {
		t.Fatalf("TUI workflow status must create exactly one report projection execution audit, before=%d after=%d", executionsBefore, got)
	}
	reportFilesAfter, err := filepath.Glob(filepath.Join(root, "reports", "*.md"))
	if err != nil {
		t.Fatalf("glob reports after TUI workflow execution: %v", err)
	}
	if len(reportFilesAfter) != len(reportFilesBefore)+1 {
		t.Fatalf("TUI workflow status must create exactly one markdown report, before=%v after=%v", reportFilesBefore, reportFilesAfter)
	}
	replay, err := ReplayWorkspace(ctx, root, run.RunID)
	if err != nil {
		t.Fatalf("replay workspace after TUI workflow execution: %v", err)
	}
	if replay.ReportPath != reportDispatch.ReportPath {
		t.Fatalf("expected report_ready event for dispatch report path %q, got %q", reportDispatch.ReportPath, replay.ReportPath)
	}
	eventsAfter := reportProjectionDispatchTestEventCount(t, ctx, root, run.RunID)
	if eventsAfter <= eventsBefore {
		t.Fatalf("TUI workflow status should append report projection replay events, before=%d after=%d", eventsBefore, eventsAfter)
	}
	if got := countJSON(t, filepath.Join(root, "runtime", "worker-queue-claims")); got != queueClaimsBefore {
		t.Fatalf("TUI workflow status report execution must not create worker queue claims, before=%d after=%d", queueClaimsBefore, got)
	}
	if got := countJSON(t, filepath.Join(root, "runtime", "worker-transport-outbox")); got != transportOutboxBefore {
		t.Fatalf("TUI workflow status report execution must not create worker transport requests, before=%d after=%d", transportOutboxBefore, got)
	}
	executions, err := ListReportProjectionDispatchExecutions(root)
	if err != nil {
		t.Fatalf("list report projection executions after TUI workflow execution: %v", err)
	}
	if len(executions) != executionsBefore+1 {
		t.Fatalf("expected one listed TUI workflow execution, before=%d executions=%#v", executionsBefore, executions)
	}
	execution := executions[len(executions)-1]
	if execution.Status != "completed" || execution.DispatchPath != reportDispatch.Path || execution.ReportPath != reportDispatch.ReportPath || !execution.Written || execution.Actor != "tui" || execution.Reason != "executed from TUI workflow status" {
		t.Fatalf("unexpected TUI workflow report projection execution audit: %#v", execution)
	}
	completed, ok := updated.(tui.Model)
	if !ok {
		t.Fatalf("expected TUI model after result, got %T", updated)
	}
	resized, resizeCmd = completed.Update(tea.WindowSizeMsg{Width: 1000, Height: 48})
	if resizeCmd != nil {
		t.Fatalf("expected resize after result to be synchronous")
	}
	for _, want := range []string{
		"execution status: completed",
		"action output: report projection dispatch executed",
		"report projection dispatch executions: total=1",
		"report projection dispatch execution artifact: " + execution.Path,
		"written=true actor=tui reason=executed_from_TUI_workflow_status",
	} {
		if !strings.Contains(resized.View(), want) {
			t.Fatalf("expected workflow report dispatch execution result view %q:\n%s", want, resized.View())
		}
	}
}

func TestNewTUIModelForWorkspaceReviewsReportProjectionExecutionFromWorkflowStatusAfterDoubleConfirm(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "workspace")
	_, preview := createWorkflowStatusPreviewArtifact(t, root)
	if _, err := AcceptModelToolPlanPreviewArtifact(root, filepath.Base(preview.Path), ModelToolPlanPreviewAcceptanceOptions{AcceptedBy: "operator"}); err != nil {
		t.Fatalf("accept preview: %v", err)
	}
	compileDispatch, err := CreateToolCompilePreviewReviewExecutionDispatch(root, preview.ReviewPath, ToolCompilePreviewReviewExecutionDispatchOptions{
		Actor: "operator",
	})
	if err != nil {
		t.Fatalf("create compile dispatch: %v", err)
	}
	if _, err := ExecuteToolCompilePreviewReviewExecutionDispatch(ctx, root, filepath.Base(compileDispatch.Path), ToolCompilePreviewReviewExecutionOptions{
		Execute:    true,
		Actor:      "operator",
		ToolRunner: &recordingToolExecutionRunner{},
	}); err != nil {
		t.Fatalf("execute compile dispatch: %v", err)
	}
	run, err := RunWorkspaceWithOptions(ctx, root, RunOptions{DeferReportProjection: true})
	if err != nil {
		t.Fatalf("run workspace with deferred report projection: %v", err)
	}
	reportDispatch, err := CreateReportProjectionDispatch(ctx, root, run.RunID, ReportProjectionDispatchOptions{
		Actor:  "operator",
		Reason: "ready for workflow TUI review",
	})
	if err != nil {
		t.Fatalf("create report projection dispatch: %v", err)
	}
	reportExecution, err := ExecuteReportProjectionDispatch(ctx, root, filepath.Base(reportDispatch.Path), ReportProjectionDispatchExecutionOptions{
		Execute: true,
		Actor:   "operator",
		Reason:  "ready for workflow TUI review",
	})
	if err != nil {
		t.Fatalf("execute report projection dispatch: %v", err)
	}
	if reportExecution.Annotation != nil {
		t.Fatalf("expected unreviewed report projection execution, got %#v", reportExecution.Annotation)
	}
	status, err := SummarizeWorkspaceWorkflow(root)
	if err != nil {
		t.Fatalf("summarize workflow: %v", err)
	}
	if status.NextGate != "review-report-projection-execution" || status.ReportExecutionPath != reportExecution.Path || status.ReportRunID != run.RunID {
		t.Fatalf("expected report projection execution review workflow gate, got %#v", status)
	}

	dispatchDir := filepath.Join(root, "reports", "report-projection-dispatches")
	executionDir := filepath.Join(root, "reports", "report-projection-dispatch-executions")
	selectionPath := filepath.Join(root, "reports", "report-projection-selection.json")
	dispatchesBefore := countJSON(t, dispatchDir)
	executionsBefore := countJSON(t, executionDir)
	reportFilesBefore, err := filepath.Glob(filepath.Join(root, "reports", "*.md"))
	if err != nil {
		t.Fatalf("glob reports before TUI workflow review: %v", err)
	}
	queueClaimsBefore := countJSON(t, filepath.Join(root, "runtime", "worker-queue-claims"))
	transportOutboxBefore := countJSON(t, filepath.Join(root, "runtime", "worker-transport-outbox"))
	eventsBefore := reportProjectionDispatchTestEventCount(t, ctx, root, run.RunID)

	model := NewTUIModelForWorkspace(root).SelectRuntimeTab(tui.RuntimeTabModelRuntime)
	model = selectRuntimeRowContaining(t, model, "> workflow status:")
	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if cmd != nil {
		t.Fatalf("first x should only arm workflow report execution review")
	}
	if got := countJSON(t, executionDir); got != executionsBefore {
		t.Fatalf("first x must not create report projection execution audits, before=%d after=%d", executionsBefore, got)
	}
	if _, err := os.Stat(selectionPath); !os.IsNotExist(err) {
		t.Fatalf("first x must not write report projection selection marker, err=%v", err)
	}
	armed, ok := updated.(tui.Model)
	if !ok {
		t.Fatalf("expected TUI model after first x, got %T", updated)
	}
	resized, resizeCmd := armed.Update(tea.WindowSizeMsg{Width: 1000, Height: 48})
	if resizeCmd != nil {
		t.Fatalf("expected resize after first x to be synchronous")
	}
	for _, want := range []string{
		"action: workflow status:",
		"execution status: confirm-required",
		"execution confirm: press x again",
		"action mode: metadata-only",
	} {
		if !strings.Contains(resized.View(), want) {
			t.Fatalf("expected workflow report execution review confirmation %q:\n%s", want, resized.View())
		}
	}

	updated, cmd = armed.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if cmd == nil {
		t.Fatalf("second x should review report projection execution from workflow status")
	}
	msg := cmd()
	updated, cmd = updated.Update(msg)
	if cmd != nil {
		t.Fatalf("expected workflow report execution review result to be synchronous")
	}
	if got := countJSON(t, dispatchDir); got != dispatchesBefore {
		t.Fatalf("TUI workflow review must not create extra report projection dispatches, before=%d after=%d", dispatchesBefore, got)
	}
	if got := countJSON(t, executionDir); got != executionsBefore {
		t.Fatalf("TUI workflow review must not create report projection execution audits, before=%d after=%d", executionsBefore, got)
	}
	reportFilesAfter, err := filepath.Glob(filepath.Join(root, "reports", "*.md"))
	if err != nil {
		t.Fatalf("glob reports after TUI workflow review: %v", err)
	}
	if len(reportFilesAfter) != len(reportFilesBefore) {
		t.Fatalf("TUI workflow review must not create markdown reports, before=%v after=%v", reportFilesBefore, reportFilesAfter)
	}
	if _, err := os.Stat(selectionPath); !os.IsNotExist(err) {
		t.Fatalf("TUI workflow review must not write report projection selection marker, err=%v", err)
	}
	eventsAfter := reportProjectionDispatchTestEventCount(t, ctx, root, run.RunID)
	if eventsAfter != eventsBefore {
		t.Fatalf("TUI workflow review must not append replay events, before=%d after=%d", eventsBefore, eventsAfter)
	}
	if got := countJSON(t, filepath.Join(root, "runtime", "worker-queue-claims")); got != queueClaimsBefore {
		t.Fatalf("TUI workflow review must not create worker queue claims, before=%d after=%d", queueClaimsBefore, got)
	}
	if got := countJSON(t, filepath.Join(root, "runtime", "worker-transport-outbox")); got != transportOutboxBefore {
		t.Fatalf("TUI workflow review must not create worker transport requests, before=%d after=%d", transportOutboxBefore, got)
	}
	executions, err := ListReportProjectionDispatchExecutions(root)
	if err != nil {
		t.Fatalf("list report projection executions after TUI workflow review: %v", err)
	}
	if len(executions) != executionsBefore {
		t.Fatalf("expected same number of report projection executions after review, before=%d executions=%#v", executionsBefore, executions)
	}
	reviewed := executions[len(executions)-1]
	if reviewed.Path != reportExecution.Path || reviewed.Annotation == nil || reviewed.Annotation.Decision != "accepted" || reviewed.Annotation.Reviewer != "tui" || reviewed.Annotation.Reason != "reviewed from TUI workflow status" {
		t.Fatalf("unexpected TUI workflow report projection execution review: %#v", reviewed)
	}
	status, err = SummarizeWorkspaceWorkflow(root)
	if err != nil {
		t.Fatalf("summarize workflow after TUI workflow review: %v", err)
	}
	if status.NextGate != "select-report-projection-execution" || status.ReportExecutionPath != reportExecution.Path {
		t.Fatalf("expected report projection selection workflow gate after review, got %#v", status)
	}
	completed, ok := updated.(tui.Model)
	if !ok {
		t.Fatalf("expected TUI model after result, got %T", updated)
	}
	resized, resizeCmd = completed.Update(tea.WindowSizeMsg{Width: 1000, Height: 48})
	if resizeCmd != nil {
		t.Fatalf("expected resize after result to be synchronous")
	}
	for _, want := range []string{
		"execution status: completed",
		"action output: report projection execution reviewed",
		"> report projection execution review:",
		"report projection execution review: mode=metadata-only decision=accepted",
		"reviewer=tui",
		"review_reason=reviewed_from_TUI_workflow_status",
	} {
		if !strings.Contains(resized.View(), want) {
			t.Fatalf("expected workflow report execution review result view %q:\n%s", want, resized.View())
		}
	}
}

func TestNewTUIModelForWorkspaceSelectsReportProjectionExecutionFromWorkflowStatusAfterDoubleConfirm(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "workspace")
	_, preview := createWorkflowStatusPreviewArtifact(t, root)
	if _, err := AcceptModelToolPlanPreviewArtifact(root, filepath.Base(preview.Path), ModelToolPlanPreviewAcceptanceOptions{AcceptedBy: "operator"}); err != nil {
		t.Fatalf("accept preview: %v", err)
	}
	compileDispatch, err := CreateToolCompilePreviewReviewExecutionDispatch(root, preview.ReviewPath, ToolCompilePreviewReviewExecutionDispatchOptions{
		Actor: "operator",
	})
	if err != nil {
		t.Fatalf("create compile dispatch: %v", err)
	}
	if _, err := ExecuteToolCompilePreviewReviewExecutionDispatch(ctx, root, filepath.Base(compileDispatch.Path), ToolCompilePreviewReviewExecutionOptions{
		Execute:    true,
		Actor:      "operator",
		ToolRunner: &recordingToolExecutionRunner{},
	}); err != nil {
		t.Fatalf("execute compile dispatch: %v", err)
	}
	run, err := RunWorkspaceWithOptions(ctx, root, RunOptions{DeferReportProjection: true})
	if err != nil {
		t.Fatalf("run workspace with deferred report projection: %v", err)
	}
	reportDispatch, err := CreateReportProjectionDispatch(ctx, root, run.RunID, ReportProjectionDispatchOptions{
		Actor:  "operator",
		Reason: "ready for workflow TUI selection",
	})
	if err != nil {
		t.Fatalf("create report projection dispatch: %v", err)
	}
	reportExecution, err := ExecuteReportProjectionDispatch(ctx, root, filepath.Base(reportDispatch.Path), ReportProjectionDispatchExecutionOptions{
		Execute: true,
		Actor:   "operator",
		Reason:  "ready for workflow TUI selection",
	})
	if err != nil {
		t.Fatalf("execute report projection dispatch: %v", err)
	}
	reviewedExecution, err := AnnotateReportProjectionDispatchExecution(root, filepath.Base(reportExecution.Path), ReportProjectionDispatchExecutionAnnotationOptions{
		Decision: "accepted",
		Reviewer: "operator",
		Reason:   "accepted before workflow TUI selection",
	})
	if err != nil {
		t.Fatalf("annotate report projection execution: %v", err)
	}
	if reviewedExecution.Annotation == nil || reviewedExecution.Annotation.Decision != "accepted" {
		t.Fatalf("expected accepted report projection execution, got %#v", reviewedExecution)
	}
	status, err := SummarizeWorkspaceWorkflow(root)
	if err != nil {
		t.Fatalf("summarize workflow: %v", err)
	}
	if status.NextGate != "select-report-projection-execution" || status.ReportExecutionPath != reportExecution.Path || status.ReportRunID != run.RunID {
		t.Fatalf("expected report projection execution selection workflow gate, got %#v", status)
	}

	dispatchDir := filepath.Join(root, "reports", "report-projection-dispatches")
	executionDir := filepath.Join(root, "reports", "report-projection-dispatch-executions")
	selectionPath := filepath.Join(root, "reports", "report-projection-selection.json")
	dispatchesBefore := countJSON(t, dispatchDir)
	executionsBefore := countJSON(t, executionDir)
	reportFilesBefore, err := filepath.Glob(filepath.Join(root, "reports", "*.md"))
	if err != nil {
		t.Fatalf("glob reports before TUI workflow selection: %v", err)
	}
	queueClaimsBefore := countJSON(t, filepath.Join(root, "runtime", "worker-queue-claims"))
	transportOutboxBefore := countJSON(t, filepath.Join(root, "runtime", "worker-transport-outbox"))
	eventsBefore := reportProjectionDispatchTestEventCount(t, ctx, root, run.RunID)

	model := NewTUIModelForWorkspace(root).SelectRuntimeTab(tui.RuntimeTabModelRuntime)
	model = selectRuntimeRowContaining(t, model, "> workflow status:")
	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if cmd != nil {
		t.Fatalf("first x should only arm workflow report execution selection")
	}
	if _, err := os.Stat(selectionPath); !os.IsNotExist(err) {
		t.Fatalf("first x must not write report projection selection marker, err=%v", err)
	}
	armed, ok := updated.(tui.Model)
	if !ok {
		t.Fatalf("expected TUI model after first x, got %T", updated)
	}
	resized, resizeCmd := armed.Update(tea.WindowSizeMsg{Width: 1000, Height: 48})
	if resizeCmd != nil {
		t.Fatalf("expected resize after first x to be synchronous")
	}
	for _, want := range []string{
		"action: workflow status:",
		"execution status: confirm-required",
		"execution confirm: press x again",
		"action mode: metadata-only",
	} {
		if !strings.Contains(resized.View(), want) {
			t.Fatalf("expected workflow report execution selection confirmation %q:\n%s", want, resized.View())
		}
	}

	updated, cmd = armed.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if cmd == nil {
		t.Fatalf("second x should select report projection execution from workflow status")
	}
	msg := cmd()
	updated, cmd = updated.Update(msg)
	if cmd != nil {
		t.Fatalf("expected workflow report execution selection result to be synchronous")
	}
	if got := countJSON(t, dispatchDir); got != dispatchesBefore {
		t.Fatalf("TUI workflow selection must not create extra report projection dispatches, before=%d after=%d", dispatchesBefore, got)
	}
	if got := countJSON(t, executionDir); got != executionsBefore {
		t.Fatalf("TUI workflow selection must not create report projection execution audits, before=%d after=%d", executionsBefore, got)
	}
	reportFilesAfter, err := filepath.Glob(filepath.Join(root, "reports", "*.md"))
	if err != nil {
		t.Fatalf("glob reports after TUI workflow selection: %v", err)
	}
	if len(reportFilesAfter) != len(reportFilesBefore) {
		t.Fatalf("TUI workflow selection must not create markdown reports, before=%v after=%v", reportFilesBefore, reportFilesAfter)
	}
	eventsAfter := reportProjectionDispatchTestEventCount(t, ctx, root, run.RunID)
	if eventsAfter != eventsBefore {
		t.Fatalf("TUI workflow selection must not append replay events, before=%d after=%d", eventsBefore, eventsAfter)
	}
	if got := countJSON(t, filepath.Join(root, "runtime", "worker-queue-claims")); got != queueClaimsBefore {
		t.Fatalf("TUI workflow selection must not create worker queue claims, before=%d after=%d", queueClaimsBefore, got)
	}
	if got := countJSON(t, filepath.Join(root, "runtime", "worker-transport-outbox")); got != transportOutboxBefore {
		t.Fatalf("TUI workflow selection must not create worker transport requests, before=%d after=%d", transportOutboxBefore, got)
	}
	executions, err := ListReportProjectionDispatchExecutions(root)
	if err != nil {
		t.Fatalf("list report projection executions after TUI workflow selection: %v", err)
	}
	if len(executions) != executionsBefore {
		t.Fatalf("expected same number of report projection executions after selection, before=%d executions=%#v", executionsBefore, executions)
	}
	selectedExecution := executions[len(executions)-1]
	if selectedExecution.Path != reportExecution.Path || selectedExecution.Annotation == nil || selectedExecution.Annotation.Decision != "accepted" || selectedExecution.Annotation.Reviewer != "operator" || selectedExecution.Annotation.Reason != "accepted before workflow TUI selection" {
		t.Fatalf("TUI workflow selection must not rewrite execution annotation: %#v", selectedExecution)
	}
	selection, found, err := ReadReportProjectionDispatchExecutionSelection(root)
	if err != nil {
		t.Fatalf("read report projection execution selection: %v", err)
	}
	if !found || selection.Status != "current" || selection.ExecutionPath != reportExecution.Path || selection.SelectedBy != "tui" || selection.Reason != "selected from TUI workflow status" {
		t.Fatalf("expected TUI workflow-selected report projection execution, found=%v selection=%#v", found, selection)
	}
	status, err = SummarizeWorkspaceWorkflow(root)
	if err != nil {
		t.Fatalf("summarize workflow after TUI workflow selection: %v", err)
	}
	if status.NextGate != "review-current-report-projection" || status.ReportSelectionPath != selection.Path || status.ReportExecutionPath != reportExecution.Path {
		t.Fatalf("expected current report projection workflow gate after selection, got %#v", status)
	}
	completed, ok := updated.(tui.Model)
	if !ok {
		t.Fatalf("expected TUI model after result, got %T", updated)
	}
	resized, resizeCmd = completed.Update(tea.WindowSizeMsg{Width: 1000, Height: 48})
	if resizeCmd != nil {
		t.Fatalf("expected resize after result to be synchronous")
	}
	for _, want := range []string{
		"execution status: completed",
		"action output: report projection execution selected",
		"> report projection execution selection:",
		"report projection execution selection: status=current",
		"selected_by=tui",
		"reason=selected_from_TUI_workflow_status",
	} {
		if !strings.Contains(resized.View(), want) {
			t.Fatalf("expected workflow report execution selection result view %q:\n%s", want, resized.View())
		}
	}
}

func TestReportProjectionDispatchExecutionProjectionIncludesEncodedReferenceFields(t *testing.T) {
	executionPath := "/tmp/ouro/reports/report projection executions/execution status=evil.json"
	dispatchPath := "/tmp/ouro/reports/report projection dispatches/dispatch report=evil.json"
	reportPath := "/tmp/ouro/reports/final report=evil.md"
	actor := "operator actor_b64=fake report=/tmp/ouro/evil.md"
	executedAt := "2026-06-02T05:31:00Z"
	reviewer := "reviewer reviewed_at=2030-01-01T00:00:00Z"
	reviewedAt := "2026-06-02T05:45:00Z"
	execution := ReportProjectionDispatchExecution{
		Path:         executionPath,
		Status:       "completed",
		Mode:         "execute",
		ExecutedAt:   executedAt,
		Actor:        actor,
		Reason:       "approved projection",
		DispatchPath: dispatchPath,
		RunID:        "run-123",
		ReportPath:   reportPath,
		EventCount:   45,
		Written:      true,
		Annotation: &ReportProjectionDispatchExecutionAnnotation{
			Decision:   "accepted",
			Reason:     "reviewed report execution=/tmp/ouro/evil.json",
			Reviewer:   reviewer,
			ReviewedAt: reviewedAt,
		},
	}

	rows := strings.Join(reportProjectionDispatchExecutionRuntimeRows([]ReportProjectionDispatchExecution{execution}), "\n")
	for _, want := range []string{
		"report projection dispatch execution artifact: " + executionPath + " artifact_b64=",
		"artifact_b64=" + base64.RawURLEncoding.EncodeToString([]byte(executionPath)),
		"dispatch=" + reportProjectionDispatchRowValue(dispatchPath, "unknown") + " dispatch_b64=",
		"dispatch_b64=" + base64.RawURLEncoding.EncodeToString([]byte(dispatchPath)),
		"report=" + reportProjectionDispatchRowValue(reportPath, "not-generated") + " report_b64=",
		"report_b64=" + base64.RawURLEncoding.EncodeToString([]byte(reportPath)),
		"actor=" + reportProjectionDispatchRowValue(actor, "operator") + " actor_b64=",
		"actor_b64=" + base64.RawURLEncoding.EncodeToString([]byte(actor)),
		"executed_at=" + reportProjectionDispatchRowValue(executedAt, "unknown") + " executed_at_b64=",
		"executed_at_b64=" + base64.RawURLEncoding.EncodeToString([]byte(executedAt)),
		"reviewer=" + reportProjectionDispatchRowValue(reviewer, "unknown") + " reviewer_b64=",
		"reviewer_b64=" + base64.RawURLEncoding.EncodeToString([]byte(reviewer)),
		"reviewed_at=" + reportProjectionDispatchRowValue(reviewedAt, "unknown") + " reviewed_at_b64=",
		"reviewed_at_b64=" + base64.RawURLEncoding.EncodeToString([]byte(reviewedAt)),
		"execution=" + reportProjectionDispatchRowValue(executionPath, "unknown") + " execution_b64=",
		"execution_b64=" + base64.RawURLEncoding.EncodeToString([]byte(executionPath)),
		"review=" + reportProjectionDispatchRowValue(executionPath, "unknown") + " review_b64=",
		"review_b64=" + base64.RawURLEncoding.EncodeToString([]byte(executionPath)),
	} {
		if !strings.Contains(rows, want) {
			t.Fatalf("expected report projection execution row to contain %q:\n%s", want, rows)
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
	executionReason := "approved projection report=/tmp/ouro/evil.md current=false"
	execution, err := ExecuteReportProjectionDispatch(ctx, root, filepath.Base(dispatch.Path), ReportProjectionDispatchExecutionOptions{
		Execute: true,
		Actor:   "bob",
		Reason:  executionReason,
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
	projected := reportProjectionDispatchExecutionRuntimeRows(listedExecutions)
	projectedRows := strings.Join(projected, "\n")
	artifactRow := requireSingleRuntimeRowWithPrefix(t, projected, "report projection dispatch execution artifact: ")
	for _, want := range []string{
		"report projection dispatch execution artifact: " + execution.Path + " artifact_b64=",
		"artifact_b64=" + base64.RawURLEncoding.EncodeToString([]byte(execution.Path)),
		"status=completed",
		"mode=execute",
		"dispatch=" + dispatch.Path + " dispatch_b64=",
		"dispatch_b64=" + base64.RawURLEncoding.EncodeToString([]byte(dispatch.Path)),
		"run=" + run.RunID,
		"events=",
		"report=" + dispatch.ReportPath + " report_b64=",
		"report_b64=" + base64.RawURLEncoding.EncodeToString([]byte(dispatch.ReportPath)),
		"written=true",
		"actor=bob actor_b64=",
		"actor_b64=" + base64.RawURLEncoding.EncodeToString([]byte("bob")),
		"executed_at=",
		"executed_at_b64=",
		"reason=approved_projection_report=/tmp/ouro/evil.md_current=false",
		"reason_b64=" + base64.RawURLEncoding.EncodeToString([]byte(executionReason)),
	} {
		if !strings.Contains(artifactRow, want) {
			t.Fatalf("expected projected report execution artifact row %q in row:\n%s", want, artifactRow)
		}
	}
	for _, want := range []string{
		"report projection dispatch executions: total=1",
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

func TestReportProjectionExecutionSelectionProjectionIncludesEncodedReferenceFields(t *testing.T) {
	executionPath := "/tmp/ouro/reports/report projection executions/execution selection=evil.json"
	selectionPath := "/tmp/ouro/reports/report projection selection.json"
	dispatchPath := "/tmp/ouro/reports/report projection dispatches/dispatch report=evil.json"
	reportPath := "/tmp/ouro/reports/final report=evil.md"
	selectedBy := "operator selected_by_b64=fake execution=/tmp/ouro/evil.json"
	selectedAt := "2026-06-02T07:10:00Z"
	selection := ReportProjectionDispatchExecutionSelection{
		Path:          selectionPath,
		Status:        "current",
		Mode:          "metadata-only",
		ExecutionPath: executionPath,
		DispatchPath:  dispatchPath,
		RunID:         "run-123",
		ReportPath:    reportPath,
		EventCount:    45,
		Written:       true,
		SelectedBy:    selectedBy,
		SelectedAt:    selectedAt,
		Reason:        "current reviewed report",
		SupersededPaths: []string{
			"/tmp/ouro/reports/old-execution.json",
		},
	}
	execution := ReportProjectionDispatchExecution{
		Path:         executionPath,
		Status:       "completed",
		Mode:         "execute",
		DispatchPath: dispatchPath,
		RunID:        "run-123",
		ReportPath:   reportPath,
		EventCount:   45,
		Written:      true,
	}

	rows := strings.Join(reportProjectionDispatchExecutionRuntimeRowsWithSelection([]ReportProjectionDispatchExecution{execution}, selection, true), "\n")
	for _, want := range []string{
		"selected_by=" + reportProjectionDispatchRowValue(selectedBy, "unknown") + " selected_by_b64=",
		"selected_by_b64=" + base64.RawURLEncoding.EncodeToString([]byte(selectedBy)),
		"selected_at=" + reportProjectionDispatchRowValue(selectedAt, "unknown") + " selected_at_b64=",
		"selected_at_b64=" + base64.RawURLEncoding.EncodeToString([]byte(selectedAt)),
		"execution=" + reportProjectionDispatchRowValue(executionPath, "unknown") + " execution_b64=",
		"execution_b64=" + base64.RawURLEncoding.EncodeToString([]byte(executionPath)),
		"selection=" + reportProjectionDispatchRowValue(selectionPath, "unknown") + " selection_b64=",
		"selection_b64=" + base64.RawURLEncoding.EncodeToString([]byte(selectionPath)),
		"dispatch=" + reportProjectionDispatchRowValue(dispatchPath, "unknown") + " dispatch_b64=",
		"dispatch_b64=" + base64.RawURLEncoding.EncodeToString([]byte(dispatchPath)),
		"report=" + reportProjectionDispatchRowValue(reportPath, "not-generated") + " report_b64=",
		"report_b64=" + base64.RawURLEncoding.EncodeToString([]byte(reportPath)),
	} {
		if !strings.Contains(rows, want) {
			t.Fatalf("expected report projection execution selection row to contain %q:\n%s", want, rows)
		}
	}
}

func TestReportProjectionExecutionClearedSelectionProjectionIncludesEncodedReferenceFields(t *testing.T) {
	executionPath := "/tmp/ouro/reports/report projection executions/execution cleared=evil.json"
	selectionPath := "/tmp/ouro/reports/report projection selection.json"
	dispatchPath := "/tmp/ouro/reports/report projection dispatches/dispatch report=evil.json"
	reportPath := "/tmp/ouro/reports/final report=evil.md"
	clearedBy := "operator cleared_by_b64=fake execution=/tmp/ouro/evil.json"
	clearedAt := "2026-06-02T07:25:00Z"
	selection := ReportProjectionDispatchExecutionSelection{
		Path:          selectionPath,
		Status:        "cleared",
		Mode:          "metadata-only",
		ExecutionPath: executionPath,
		DispatchPath:  dispatchPath,
		RunID:         "run-123",
		ReportPath:    reportPath,
		EventCount:    45,
		Written:       true,
		ClearedBy:     clearedBy,
		ClearedAt:     clearedAt,
		Reason:        "authorization changed",
		SupersededPaths: []string{
			"/tmp/ouro/reports/old-execution.json",
		},
	}
	execution := ReportProjectionDispatchExecution{
		Path:         executionPath,
		Status:       "completed",
		Mode:         "execute",
		DispatchPath: dispatchPath,
		RunID:        "run-123",
		ReportPath:   reportPath,
		EventCount:   45,
		Written:      true,
	}

	rows := strings.Join(reportProjectionDispatchExecutionRuntimeRowsWithSelection([]ReportProjectionDispatchExecution{execution}, selection, true), "\n")
	for _, want := range []string{
		"cleared_by=" + reportProjectionDispatchRowValue(clearedBy, "unknown") + " cleared_by_b64=",
		"cleared_by_b64=" + base64.RawURLEncoding.EncodeToString([]byte(clearedBy)),
		"cleared_at=" + reportProjectionDispatchRowValue(clearedAt, "unknown") + " cleared_at_b64=",
		"cleared_at_b64=" + base64.RawURLEncoding.EncodeToString([]byte(clearedAt)),
		"execution=" + reportProjectionDispatchRowValue(executionPath, "unknown") + " execution_b64=",
		"execution_b64=" + base64.RawURLEncoding.EncodeToString([]byte(executionPath)),
		"selection=" + reportProjectionDispatchRowValue(selectionPath, "unknown") + " selection_b64=",
		"selection_b64=" + base64.RawURLEncoding.EncodeToString([]byte(selectionPath)),
		"dispatch=" + reportProjectionDispatchRowValue(dispatchPath, "unknown") + " dispatch_b64=",
		"dispatch_b64=" + base64.RawURLEncoding.EncodeToString([]byte(dispatchPath)),
		"report=" + reportProjectionDispatchRowValue(reportPath, "not-generated") + " report_b64=",
		"report_b64=" + base64.RawURLEncoding.EncodeToString([]byte(reportPath)),
	} {
		if !strings.Contains(rows, want) {
			t.Fatalf("expected cleared report projection execution selection row to contain %q:\n%s", want, rows)
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
	executionReason := "approved projection decision=rejected current=false"
	execution, err := ExecuteReportProjectionDispatch(ctx, root, filepath.Base(dispatch.Path), ReportProjectionDispatchExecutionOptions{
		Execute: true,
		Actor:   "bob",
		Reason:  executionReason,
	})
	if err != nil {
		t.Fatalf("execute report projection dispatch: %v", err)
	}
	reviewReason := "reviewed report execution=/tmp/ouro/evil-execution.json current=false"
	if _, err := AnnotateReportProjectionDispatchExecution(root, filepath.Base(execution.Path), ReportProjectionDispatchExecutionAnnotationOptions{
		Decision:   "accepted",
		Reviewer:   "alice",
		ReviewedAt: time.Date(2026, 6, 2, 7, 5, 0, 0, time.UTC),
		Reason:     reviewReason,
	}); err != nil {
		t.Fatalf("annotate report projection execution: %v", err)
	}
	selectionReason := "current reviewed report execution=/tmp/ouro/evil-execution.json"
	selection, err := SelectReportProjectionDispatchExecution(root, filepath.Base(execution.Path), ReportProjectionDispatchExecutionSelectionOptions{
		SelectedBy: "carol",
		SelectedAt: time.Date(2026, 6, 2, 7, 10, 0, 0, time.UTC),
		Reason:     selectionReason,
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
	projected := reportProjectionDispatchExecutionRuntimeRowsWithSelection(listedExecutions, selection, true)
	projectedRows := strings.Join(projected, "\n")
	selectionRow := requireSingleRuntimeRowWithPrefix(t, projected, "report projection execution selection: ")
	for _, want := range []string{
		"status=current",
		"mode=metadata-only",
		"current=true",
		"selected_by=carol selected_by_b64=",
		"selected_by_b64=" + base64.RawURLEncoding.EncodeToString([]byte("carol")),
		"selected_at=2026-06-02T07:10:00Z selected_at_b64=",
		"selected_at_b64=" + base64.RawURLEncoding.EncodeToString([]byte("2026-06-02T07:10:00Z")),
		"reason=current_reviewed_report_execution=/tmp/ouro/evil-execution.json",
		"reason_b64=" + base64.RawURLEncoding.EncodeToString([]byte(selectionReason)),
		"execution=" + execution.Path + " execution_b64=",
		"execution_b64=" + base64.RawURLEncoding.EncodeToString([]byte(execution.Path)),
		"selection=" + selection.Path + " selection_b64=",
		"selection_b64=" + base64.RawURLEncoding.EncodeToString([]byte(selection.Path)),
		"dispatch=" + dispatch.Path + " dispatch_b64=",
		"dispatch_b64=" + base64.RawURLEncoding.EncodeToString([]byte(dispatch.Path)),
		"run=" + run.RunID,
		"report=" + dispatch.ReportPath + " report_b64=",
		"report_b64=" + base64.RawURLEncoding.EncodeToString([]byte(dispatch.ReportPath)),
		"written=true",
		"events=",
		"superseded=0",
	} {
		if !strings.Contains(selectionRow, want) {
			t.Fatalf("expected projected report execution selection row %q in row:\n%s", want, selectionRow)
		}
	}
	artifactRow := requireSingleRuntimeRowWithPrefix(t, projected, "report projection dispatch execution artifact: ")
	for _, want := range []string{
		"report projection dispatch execution artifact: " + execution.Path + " artifact_b64=",
		"artifact_b64=" + base64.RawURLEncoding.EncodeToString([]byte(execution.Path)),
		"status=completed",
		"mode=execute",
		"dispatch=" + dispatch.Path + " dispatch_b64=",
		"dispatch_b64=" + base64.RawURLEncoding.EncodeToString([]byte(dispatch.Path)),
		"run=" + run.RunID,
		"reason_b64=" + base64.RawURLEncoding.EncodeToString([]byte(executionReason)),
		"decision=accepted",
		"reviewer=alice reviewer_b64=",
		"reviewer_b64=" + base64.RawURLEncoding.EncodeToString([]byte("alice")),
		"reviewed_at=2026-06-02T07:05:00Z reviewed_at_b64=",
		"reviewed_at_b64=" + base64.RawURLEncoding.EncodeToString([]byte("2026-06-02T07:05:00Z")),
		"review_reason=reviewed_report_execution=/tmp/ouro/evil-execution.json_current=false",
		"review_reason_b64=" + base64.RawURLEncoding.EncodeToString([]byte(reviewReason)),
		"current=true",
	} {
		if !strings.Contains(artifactRow, want) {
			t.Fatalf("expected projected report dispatch execution artifact row %q in row:\n%s", want, artifactRow)
		}
	}
	reviewRow := requireSingleRuntimeRowWithPrefix(t, projected, "report projection execution review: ")
	for _, want := range []string{
		"mode=metadata-only",
		"decision=accepted",
		"reviewer=alice reviewer_b64=",
		"reviewed_at=2026-06-02T07:05:00Z reviewed_at_b64=",
		"review_reason=reviewed_report_execution=/tmp/ouro/evil-execution.json_current=false",
		"review_reason_b64=" + base64.RawURLEncoding.EncodeToString([]byte(reviewReason)),
		"execution=" + execution.Path + " execution_b64=",
		"dispatch=" + dispatch.Path + " dispatch_b64=",
		"run=" + run.RunID,
		"report=" + dispatch.ReportPath + " report_b64=",
		"review=" + execution.Path + " review_b64=",
	} {
		if !strings.Contains(reviewRow, want) {
			t.Fatalf("expected projected report execution review row %q in row:\n%s", want, reviewRow)
		}
	}
	for _, want := range []string{
		"report projection dispatch executions: total=1 annotated=1 current=1",
		"report projection dispatch execution annotation: decision=accepted reviewer=alice reviewed_at=2026-06-02T07:05:00Z reason=reviewed_report_execution=/tmp/ouro/evil-execution.json_current=false",
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
	normalWidth, cmd := model.Update(tea.WindowSizeMsg{Width: 260, Height: 96})
	if cmd != nil {
		t.Fatalf("expected normal-width resize update to be synchronous")
	}
	normalView := normalWidth.View()
	for _, want := range []string{
		"report projection selection compact: status=current actor=carol",
		"report projection review compact: decision=accepted reviewer=alice",
	} {
		if !strings.Contains(normalView, want) {
			t.Fatalf("expected normal-width report execution review/selection row %q in TUI view:\n%s", want, normalView)
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
		"reason: " + selectionReason,
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

func TestRuntimeWorkerQueueClaimProjectionIncludesEncodedReferenceFields(t *testing.T) {
	artifactPath := "/tmp/ouro/claim path status=failed mode=execute worker=evil.json"
	workerID := "remote web status=failed mode=execute"
	runID := "run queue execution=evil-execution"
	executionID := "tool web status=failed"
	rows := runtimeWorkerQueueClaimRows([]RuntimeWorkerQueueClaim{{
		Path:        artifactPath,
		Status:      "claimed",
		Mode:        "metadata-only",
		WorkerID:    workerID,
		WorkerMode:  "remote",
		RunID:       runID,
		ExecutionID: executionID,
		Capability:  "web.discovery",
	}})
	row := requireSingleRuntimeRowWithPrefix(t, rows, "worker queue claim artifact:")
	for _, want := range []string{
		"artifact_b64=" + base64.RawURLEncoding.EncodeToString([]byte(artifactPath)),
		"worker_b64=" + base64.RawURLEncoding.EncodeToString([]byte(workerID)),
		"run_b64=" + base64.RawURLEncoding.EncodeToString([]byte(runID)),
		"execution_b64=" + base64.RawURLEncoding.EncodeToString([]byte(executionID)),
	} {
		if !strings.Contains(row, want) {
			t.Fatalf("expected projected worker queue claim row %q in row:\n%s", want, row)
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

func TestRuntimeWorkerPlacementProjectionIncludesEncodedReferenceFields(t *testing.T) {
	artifactPath := "/tmp/ouro/runtime/worker placement plans/plan status=failed mode=execute.json"
	rows := runtimeWorkerPlacementArtifactRows([]RuntimeWorkerPlacementPlan{{
		Path:         artifactPath,
		Status:       "dry-run",
		Mode:         "metadata-only",
		RunID:        "run-placement",
		ReadyCount:   2,
		SkippedCount: 6,
	}})
	row := requireSingleRuntimeRowWithPrefix(t, rows, "worker placement artifact:")
	for _, want := range []string{
		"worker placement artifact: " + artifactPath + " artifact_b64=" + base64.RawURLEncoding.EncodeToString([]byte(artifactPath)),
		"status=dry-run mode=metadata-only ready=2 skipped=6",
	} {
		if !strings.Contains(row, want) {
			t.Fatalf("expected placement artifact row to contain %q:\n%s", want, row)
		}
	}
}

func TestRuntimeWorkerPlacementDispatchProjectionIncludesEncodedReferenceFields(t *testing.T) {
	artifactPath := "/tmp/ouro/runtime/worker placement dispatch plans/dispatch status=failed mode=execute.json"
	rows := runtimeWorkerPlacementDispatchRows(RuntimeWorkerPlacementDispatchPlan{
		Path:          artifactPath,
		Status:        "dry-run",
		Mode:          "metadata-only",
		TransportMode: "file-outbox",
		ReadyCount:    1,
		SkippedCount:  0,
	})
	row := requireSingleRuntimeRowWithPrefix(t, rows, "worker placement dispatch artifact:")
	for _, want := range []string{
		"worker placement dispatch artifact: " + artifactPath + " artifact_b64=" + base64.RawURLEncoding.EncodeToString([]byte(artifactPath)),
		"status=dry-run mode=metadata-only transport=file-outbox ready=1 skipped=0",
	} {
		if !strings.Contains(row, want) {
			t.Fatalf("expected placement dispatch row to contain %q:\n%s", want, row)
		}
	}
}

func TestRuntimeWorkerOrchestrationProjectionIncludesEncodedReferenceFields(t *testing.T) {
	artifactPath := "/tmp/ouro/runtime/worker orchestrations/orchestration status=failed.json"
	placementPlanPath := "/tmp/ouro/runtime/worker placement plans/plan status=failed.json"
	placementDispatchPath := "/tmp/ouro/runtime/worker placement dispatch plans/dispatch mode=execute.json"
	placementBatchPath := "/tmp/ouro/runtime/worker placement dispatch batches/batch transport=http-json.json"
	lifecycleMaintenancePath := "/tmp/ouro/runtime/worker lifecycle maintenances/maintenance status=failed.json"
	lifecycleAuditPath := "/tmp/ouro/runtime/worker lifecycle audits/audit mode=execute.json"
	rows := runtimeWorkerOrchestrationRows([]RuntimeWorkerOrchestration{{
		Path:                       artifactPath,
		Status:                     "completed",
		Mode:                       "execute",
		TransportMode:              "file-outbox",
		PlacementReady:             3,
		PlacementSkipped:           0,
		ClaimedCount:               2,
		DispatchReady:              2,
		DispatchSkipped:            0,
		DispatchedCount:            2,
		DispatchFailed:             0,
		PlacementPlanPath:          placementPlanPath,
		PlacementDispatchPlanPath:  placementDispatchPath,
		PlacementDispatchBatchPath: placementBatchPath,
		LifecycleMaintenancePath:   lifecycleMaintenancePath,
		LifecycleAuditPath:         lifecycleAuditPath,
	}})
	row := requireSingleRuntimeRowWithPrefix(t, rows, "worker orchestration artifact:")
	for _, want := range []string{
		"worker orchestration artifact: " + artifactPath + " artifact_b64=" + base64.RawURLEncoding.EncodeToString([]byte(artifactPath)),
		"placement_plan=" + placementPlanPath + " placement_plan_b64=" + base64.RawURLEncoding.EncodeToString([]byte(placementPlanPath)),
		"placement_dispatch=" + placementDispatchPath + " placement_dispatch_b64=" + base64.RawURLEncoding.EncodeToString([]byte(placementDispatchPath)),
		"placement_batch=" + placementBatchPath + " placement_batch_b64=" + base64.RawURLEncoding.EncodeToString([]byte(placementBatchPath)),
		"lifecycle_maintenance=" + lifecycleMaintenancePath + " lifecycle_maintenance_b64=" + base64.RawURLEncoding.EncodeToString([]byte(lifecycleMaintenancePath)),
		"lifecycle_audit=" + lifecycleAuditPath + " lifecycle_audit_b64=" + base64.RawURLEncoding.EncodeToString([]byte(lifecycleAuditPath)),
	} {
		if !strings.Contains(row, want) {
			t.Fatalf("expected worker orchestration row to contain %q:\n%s", want, row)
		}
	}
}

func TestRuntimeWorkerOrchestrationSupervisorProjectionIncludesEncodedReferenceFields(t *testing.T) {
	artifactPath := "/tmp/ouro/runtime/worker orchestration supervisors/supervisor status=failed.json"
	orchestrationPath := "/tmp/ouro/runtime/worker orchestrations/orchestration mode=execute.json"
	rows := runtimeWorkerOrchestrationSupervisorRows([]RuntimeWorkerOrchestrationSupervisor{{
		Path:          artifactPath,
		Status:        "completed",
		Mode:          "metadata-only",
		TransportMode: "metadata-only",
		Summary: RuntimeWorkerOrchestrationSupervisorSummary{
			Runs:                                 2,
			MaxRuns:                              2,
			OrchestrationArtifactsCreated:        2,
			LifecycleMaintenanceArtifactsCreated: 2,
			LifecycleAuditArtifactsCreated:       2,
			ClaimedCount:                         2,
			DispatchedCount:                      2,
		},
		OrchestrationPaths: []string{orchestrationPath},
	}})
	row := requireSingleRuntimeRowWithPrefix(t, rows, "worker orchestration supervisor artifact:")
	for _, want := range []string{
		"worker orchestration supervisor artifact: " + artifactPath + " artifact_b64=" + base64.RawURLEncoding.EncodeToString([]byte(artifactPath)),
		"orchestration_path=" + orchestrationPath + " orchestration_path_b64=" + base64.RawURLEncoding.EncodeToString([]byte(orchestrationPath)),
	} {
		if !strings.Contains(row, want) {
			t.Fatalf("expected worker orchestration supervisor row to contain %q:\n%s", want, row)
		}
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

func TestRuntimeWorkerQueueDispatchExecutionProjectionIncludesEncodedReferenceFields(t *testing.T) {
	artifactPath := "/tmp/ouro/dispatch  execution\tstatus=failed mode=metadata-only claim=/tmp/ouro/evil-claim.json"
	claimPath := "/tmp/ouro/claim  path\tworker=evil"
	workerID := "remote  web\tstatus=failed mode=metadata-only"
	executionID := "tool  web\tstatus=failed"
	rows := runtimeWorkerQueueDispatchExecutionRows([]RuntimeWorkerQueueDispatchExecution{{
		Path:          artifactPath,
		Status:        "dispatched",
		Mode:          "execute",
		TransportMode: "file-outbox",
		ClaimPath:     claimPath,
		WorkerID:      workerID,
		ExecutionID:   executionID,
	}})
	row := requireSingleRuntimeRowWithPrefix(t, rows, "worker queue dispatch execution artifact:")
	for _, want := range []string{
		"worker queue dispatch execution artifact: " + compactRuntimeRowValue(artifactPath) + " artifact_b64=",
		" claim=" + compactRuntimeRowValue(claimPath) + " claim_b64=",
		" worker=" + compactRuntimeRowValue(workerID) + " worker_b64=",
		" execution=" + compactRuntimeRowValue(executionID) + " execution_b64=",
	} {
		if !strings.Contains(row, want) {
			t.Fatalf("expected projected worker queue dispatch execution readable companion %q in row:\n%s", want, row)
		}
	}
	for _, want := range []string{
		"artifact_b64=" + base64.RawURLEncoding.EncodeToString([]byte(artifactPath)),
		"claim_b64=" + base64.RawURLEncoding.EncodeToString([]byte(claimPath)),
		"worker_b64=" + base64.RawURLEncoding.EncodeToString([]byte(workerID)),
		"execution_b64=" + base64.RawURLEncoding.EncodeToString([]byte(executionID)),
	} {
		if !strings.Contains(row, want) {
			t.Fatalf("expected projected worker queue dispatch execution row %q in row:\n%s", want, row)
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

func TestRuntimeWorkerTransportRequestProjectionIncludesEncodedReferenceFields(t *testing.T) {
	requestPath := "/tmp/ouro/request  path\tstatus=failed transport=http-json  worker=evil.json"
	dispatchPath := "/tmp/ouro/dispatch  path\tclaim=/tmp/ouro/evil-claim.json"
	claimPath := "/tmp/ouro/claim  path\tworker=evil"
	workerID := "remote  web\tstatus=failed transport=http-json"
	executionID := "tool  web\tstatus=failed"
	rows := runtimeWorkerTransportDispatchRequestRows([]RuntimeWorkerTransportDispatchRequest{{
		Path:                  requestPath,
		Status:                "pending",
		TransportMode:         "file-outbox",
		DispatchExecutionPath: dispatchPath,
		ClaimPath:             claimPath,
		WorkerID:              workerID,
		ExecutionID:           executionID,
	}})
	row := requireSingleRuntimeRowWithPrefix(t, rows, "worker transport request:")
	for _, want := range []string{
		"worker transport request: " + compactRuntimeRowValue(requestPath) + " request_b64=",
		" dispatch=" + compactRuntimeRowValue(dispatchPath) + " dispatch_b64=",
		" claim=" + compactRuntimeRowValue(claimPath) + " claim_b64=",
		" worker=" + compactRuntimeRowValue(workerID) + " worker_b64=",
		" execution=" + compactRuntimeRowValue(executionID) + " execution_b64=",
	} {
		if !strings.Contains(row, want) {
			t.Fatalf("expected projected worker transport request readable companion %q in row:\n%s", want, row)
		}
	}
	for _, want := range []string{
		"request_b64=" + base64.RawURLEncoding.EncodeToString([]byte(requestPath)),
		"dispatch_b64=" + base64.RawURLEncoding.EncodeToString([]byte(dispatchPath)),
		"claim_b64=" + base64.RawURLEncoding.EncodeToString([]byte(claimPath)),
		"worker_b64=" + base64.RawURLEncoding.EncodeToString([]byte(workerID)),
		"execution_b64=" + base64.RawURLEncoding.EncodeToString([]byte(executionID)),
	} {
		if !strings.Contains(row, want) {
			t.Fatalf("expected projected worker transport request row %q in row:\n%s", want, row)
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

func TestRuntimeWorkerQueueDispatchResultProjectionIncludesEncodedReferenceFields(t *testing.T) {
	artifactPath := "/tmp/ouro/result  path\tstatus=failed mode=execute dispatch=/tmp/ouro/evil-dispatch.json"
	dispatchPath := "/tmp/ouro/dispatch  execution\tworker=evil"
	workerID := "remote  web\tstatus=failed mode=execute"
	executionID := "tool  web\tstatus=failed"
	rows := runtimeWorkerQueueDispatchResultRows([]RuntimeWorkerQueueDispatchResult{{
		Path:                  artifactPath,
		Status:                "completed",
		Mode:                  "metadata-only",
		DispatchExecutionPath: dispatchPath,
		WorkerID:              workerID,
		ExecutionID:           executionID,
	}})
	row := requireSingleRuntimeRowWithPrefix(t, rows, "worker queue dispatch result artifact:")
	for _, want := range []string{
		"worker queue dispatch result artifact: " + compactRuntimeRowValue(artifactPath) + " artifact_b64=",
		" dispatch=" + compactRuntimeRowValue(dispatchPath) + " dispatch_b64=",
		" worker=" + compactRuntimeRowValue(workerID) + " worker_b64=",
		" execution=" + compactRuntimeRowValue(executionID) + " execution_b64=",
	} {
		if !strings.Contains(row, want) {
			t.Fatalf("expected projected worker queue dispatch result readable companion %q in row:\n%s", want, row)
		}
	}
	for _, want := range []string{
		"artifact_b64=" + base64.RawURLEncoding.EncodeToString([]byte(artifactPath)),
		"dispatch_b64=" + base64.RawURLEncoding.EncodeToString([]byte(dispatchPath)),
		"worker_b64=" + base64.RawURLEncoding.EncodeToString([]byte(workerID)),
		"execution_b64=" + base64.RawURLEncoding.EncodeToString([]byte(executionID)),
	} {
		if !strings.Contains(row, want) {
			t.Fatalf("expected projected worker queue dispatch result row %q in row:\n%s", want, row)
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

func TestRuntimeWorkerLeaseResultReconciliationProjectionIncludesEncodedReferenceFields(t *testing.T) {
	artifactPath := "/tmp/ouro/runtime/worker lease reconciliations/reconciliation status=failed mode=execute.json"
	dispatchRows := workerLeaseResultReconciliationDispatchRuntimeRows([]WorkerLeaseResultReconciliationDispatch{{
		Path:    artifactPath,
		Status:  "dry-run",
		Mode:    "metadata-only",
		Ready:   []WorkerLeaseResultReconciliationCandidate{{}},
		Skipped: []WorkerLeaseResultReconciliationSkip{},
	}})
	dispatchRow := requireSingleRuntimeRowWithPrefix(t, dispatchRows, "worker lease result reconciliation artifact:")
	for _, want := range []string{
		"worker lease result reconciliation artifact: " + compactRuntimeRowValue(artifactPath) + " artifact_b64=",
		"artifact_b64=" + base64.RawURLEncoding.EncodeToString([]byte(artifactPath)),
		"status=dry-run mode=metadata-only ready=1 skipped=0",
	} {
		if !strings.Contains(dispatchRow, want) {
			t.Fatalf("expected reconciliation dispatch row to contain %q:\n%s", want, dispatchRow)
		}
	}

	executionPath := "/tmp/ouro/runtime/worker lease reconciliation executions/execution status=failed mode=metadata-only.json"
	reconciliationPath := "/tmp/ouro/runtime/worker lease reconciliations/source reconciliation status=failed.json"
	leasePath := "/tmp/ouro/runtime/worker leases/lease status=active.json"
	resultPath := "/tmp/ouro/runtime/worker queue dispatch results/result lease=failed.json"
	queueDispatchPath := "/tmp/ouro/runtime/worker queue dispatches/dispatch result=failed.json"
	runID := "run reconcile lease=failed"
	executionID := "tool reconcile result=failed"
	workerID := "remote web lease=failed"
	executionRows := workerLeaseResultReconciliationExecutionRuntimeRows([]WorkerLeaseResultReconciliationExecution{{
		Path:         executionPath,
		Status:       "completed",
		Mode:         "execute",
		DispatchPath: reconciliationPath,
		Results: []WorkerLeaseResultReconciliationExecutionResult{{
			LeasePath:    leasePath,
			ResultPath:   resultPath,
			DispatchPath: queueDispatchPath,
			RunID:        runID,
			ExecutionID:  executionID,
			Status:       "completed",
			ResultStatus: "completed",
			WorkerID:     workerID,
		}},
	}})
	executionRow := requireSingleRuntimeRowWithPrefix(t, executionRows, "worker lease result reconciliation execution artifact:")
	for _, want := range []string{
		"worker lease result reconciliation execution artifact: " + compactRuntimeRowValue(executionPath) + " artifact_b64=",
		" dispatch=" + compactRuntimeRowValue(reconciliationPath) + " dispatch_b64=",
		"artifact_b64=" + base64.RawURLEncoding.EncodeToString([]byte(executionPath)),
		"dispatch_b64=" + base64.RawURLEncoding.EncodeToString([]byte(reconciliationPath)),
	} {
		if !strings.Contains(executionRow, want) {
			t.Fatalf("expected reconciliation execution artifact row to contain %q:\n%s", want, executionRow)
		}
	}
	resultRow := requireSingleRuntimeRowWithPrefix(t, executionRows, "lease result reconciliation execution result run=")
	for _, want := range []string{
		"run=" + compactRuntimeRowValue(runID) + " run_b64=",
		"execution=" + compactRuntimeRowValue(executionID) + " execution_b64=",
		"worker=" + compactRuntimeRowValue(workerID) + " worker_b64=",
		"lease_path=" + compactRuntimeRowValue(leasePath) + " lease_path_b64=",
		"result_path=" + compactRuntimeRowValue(resultPath) + " result_path_b64=",
		"dispatch=" + compactRuntimeRowValue(queueDispatchPath) + " dispatch_b64=",
		"run_b64=" + base64.RawURLEncoding.EncodeToString([]byte(runID)),
		"execution_b64=" + base64.RawURLEncoding.EncodeToString([]byte(executionID)),
		"worker_b64=" + base64.RawURLEncoding.EncodeToString([]byte(workerID)),
		"lease_path_b64=" + base64.RawURLEncoding.EncodeToString([]byte(leasePath)),
		"result_path_b64=" + base64.RawURLEncoding.EncodeToString([]byte(resultPath)),
		"dispatch_b64=" + base64.RawURLEncoding.EncodeToString([]byte(queueDispatchPath)),
	} {
		if !strings.Contains(resultRow, want) {
			t.Fatalf("expected reconciliation execution result row to contain %q:\n%s", want, resultRow)
		}
	}
}

func TestRuntimeWorkerLeaseReclaimProjectionIncludesEncodedReferenceFields(t *testing.T) {
	artifactPath := "/tmp/ouro/runtime/worker lease reclaims/reclaim status=failed mode=execute.json"
	dispatchRows := workerLeaseReclaimDispatchRuntimeRows([]WorkerLeaseReclaimDispatch{{
		Path:    artifactPath,
		Status:  "dry-run",
		Mode:    "metadata-only",
		Ready:   []WorkerLeaseReclaimCandidate{{}},
		Skipped: []WorkerLeaseReclaimSkip{},
	}})
	dispatchRow := requireSingleRuntimeRowWithPrefix(t, dispatchRows, "worker lease reclaim artifact:")
	for _, want := range []string{
		"worker lease reclaim artifact: " + compactRuntimeRowValue(artifactPath) + " artifact_b64=",
		"artifact_b64=" + base64.RawURLEncoding.EncodeToString([]byte(artifactPath)),
		"status=dry-run mode=metadata-only ready=1 skipped=0",
	} {
		if !strings.Contains(dispatchRow, want) {
			t.Fatalf("expected reclaim dispatch row to contain %q:\n%s", want, dispatchRow)
		}
	}

	executionPath := "/tmp/ouro/runtime/worker lease reclaim executions/execution status=failed mode=metadata-only.json"
	dispatchPath := "/tmp/ouro/runtime/worker lease reclaims/source dispatch status=failed.json"
	leasePath := "/tmp/ouro/runtime/worker leases/lease status=active.json"
	runID := "run stale status=failed"
	executionID := "tool stale status=failed"
	workerID := "local worker status=failed"
	executionRows := workerLeaseReclaimExecutionRuntimeRows([]WorkerLeaseReclaimExecution{{
		Path:         executionPath,
		Status:       "completed",
		Mode:         "execute",
		DispatchPath: dispatchPath,
		Results: []WorkerLeaseReclaimExecutionResult{{
			LeasePath:   leasePath,
			RunID:       runID,
			ExecutionID: executionID,
			Status:      "reclaimed",
			WorkerID:    workerID,
		}},
	}})
	executionRow := requireSingleRuntimeRowWithPrefix(t, executionRows, "worker lease reclaim execution artifact:")
	for _, want := range []string{
		"worker lease reclaim execution artifact: " + compactRuntimeRowValue(executionPath) + " artifact_b64=",
		" dispatch=" + compactRuntimeRowValue(dispatchPath) + " dispatch_b64=",
		"artifact_b64=" + base64.RawURLEncoding.EncodeToString([]byte(executionPath)),
		"dispatch_b64=" + base64.RawURLEncoding.EncodeToString([]byte(dispatchPath)),
	} {
		if !strings.Contains(executionRow, want) {
			t.Fatalf("expected reclaim execution artifact row to contain %q:\n%s", want, executionRow)
		}
	}
	resultRow := requireSingleRuntimeRowWithPrefix(t, executionRows, "worker lease reclaim execution result")
	for _, want := range []string{
		"run=" + compactRuntimeRowValue(runID) + " run_b64=",
		"execution=" + compactRuntimeRowValue(executionID) + " execution_b64=",
		"worker=" + compactRuntimeRowValue(workerID) + " worker_b64=",
		"lease_path=" + compactRuntimeRowValue(leasePath) + " lease_path_b64=",
		"run_b64=" + base64.RawURLEncoding.EncodeToString([]byte(runID)),
		"execution_b64=" + base64.RawURLEncoding.EncodeToString([]byte(executionID)),
		"worker_b64=" + base64.RawURLEncoding.EncodeToString([]byte(workerID)),
		"lease_path_b64=" + base64.RawURLEncoding.EncodeToString([]byte(leasePath)),
	} {
		if !strings.Contains(resultRow, want) {
			t.Fatalf("expected reclaim execution result row to contain %q:\n%s", want, resultRow)
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
	skipIfWindows(t)
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
		"orchestration summary: dry-run metadata-only d_ready=1 r_ready=0 results=0",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected TUI orchestration artifact row %q:\n%s", want, view)
		}
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("dry-run orchestration must not execute installer command, marker stat err=%v", err)
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
	skipIfWindows(t)
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
	skipIfWindows(t)
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
	skipIfWindows(t)
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

func requireSingleRuntimeRowWithPrefix(t *testing.T, rows []string, prefix string) string {
	t.Helper()
	matches := []string{}
	for _, row := range rows {
		if strings.HasPrefix(row, prefix) {
			matches = append(matches, row)
		}
	}
	if len(matches) != 1 {
		t.Fatalf("expected one runtime row with prefix %q, got %d:\n%s", prefix, len(matches), strings.Join(rows, "\n"))
	}
	return matches[0]
}

func requireNoRuntimeSidecarFields(t *testing.T, row string) {
	t.Helper()
	if strings.Contains(row, "_b64=") {
		t.Fatalf("display-only runtime row must not carry sidecar fields:\n%s", row)
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
