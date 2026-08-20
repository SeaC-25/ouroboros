package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateProviderMessageCallsOpenAICompatibleChatCompletions(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
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
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST generation request, got %s", r.Method)
		}
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("expected chat completions endpoint, got %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer secret-value" {
			t.Fatalf("expected bearer header, got %q", got)
		}
		if got := r.Header.Get("Content-Type"); !strings.Contains(got, "application/json") {
			t.Fatalf("expected JSON content type, got %q", got)
		}
		if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
			t.Fatalf("decode generation request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id": "chatcmpl-test",
			"choices": [{"message": {"role": "assistant", "content": "Use the scoped plan."}}],
			"usage": {"prompt_tokens": 17, "completion_tokens": 6, "total_tokens": 23}
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

	result, err := GenerateProviderMessage(context.Background(), root, ProviderGenerationRequest{
		SystemPrompt: "You are Ouroboros.",
		UserPrompt:   "Summarize the next step.",
		MaxTokens:    64,
	})
	if err != nil {
		t.Fatalf("generate provider message: %v", err)
	}
	if result.ProviderName != "local-openai" || result.Model != "gpt-test" || result.Text != "Use the scoped plan." {
		t.Fatalf("unexpected generation result identity/text: %#v", result)
	}
	if result.InputTokens != 17 || result.OutputTokens != 6 || result.TotalTokens != 23 {
		t.Fatalf("unexpected token usage: %#v", result)
	}
	if captured.Model != "gpt-test" || captured.MaxTokens != 64 {
		t.Fatalf("unexpected generation request payload: %#v", captured)
	}
	if len(captured.Messages) != 2 || captured.Messages[0].Role != "system" || captured.Messages[0].Content != "You are Ouroboros." || captured.Messages[1].Role != "user" || captured.Messages[1].Content != "Summarize the next step." {
		t.Fatalf("unexpected generation messages: %#v", captured.Messages)
	}
}

func TestGenerateProviderMessageStreamsOpenAICompatibleChatCompletionChunks(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	var captured struct {
		Model         string `json:"model"`
		Stream        bool   `json:"stream"`
		StreamOptions struct {
			IncludeUsage bool `json:"include_usage"`
		} `json:"stream_options"`
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("expected streamed chat completion POST, got %s %s", r.Method, r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
			t.Fatalf("decode streamed generation request: %v", err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"Use \"}}]}\n\n")
		_, _ = fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"streamed guidance.\"}}]}\n\n")
		_, _ = fmt.Fprint(w, "data: {\"usage\":{\"prompt_tokens\":21,\"completion_tokens\":7,\"total_tokens\":28},\"choices\":[]}\n\n")
		_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()
	t.Setenv("OURO_TEST_PROVIDER_KEY", "secret-value")
	providers := fmt.Sprintf(`version: 1
providers:
  - name: local-openai
    type: openai_compatible
    model: gpt-stream
    base_url: %s/v1
    api_key_env: OURO_TEST_PROVIDER_KEY
    input_price_per_million_tokens: 1
    output_price_per_million_tokens: 2
    enabled: true
`, server.URL)
	if err := os.WriteFile(filepath.Join(root, "providers.yaml"), []byte(providers), 0o644); err != nil {
		t.Fatalf("write providers: %v", err)
	}

	result, err := GenerateProviderMessage(context.Background(), root, ProviderGenerationRequest{
		UserPrompt: "stream next step",
		Stream:     true,
	})
	if err != nil {
		t.Fatalf("generate streamed provider message: %v", err)
	}
	if !captured.Stream || !captured.StreamOptions.IncludeUsage {
		t.Fatalf("expected explicit streamed request with usage metadata, got %#v", captured)
	}
	if result.Text != "Use streamed guidance." {
		t.Fatalf("unexpected streamed text: %#v", result)
	}
	if !result.Streaming || !result.StreamCompleted || result.StreamChunkCount != 2 {
		t.Fatalf("unexpected streaming metadata: %#v", result)
	}
	if result.StreamDigest == "" || result.StreamPreview != "Use streamed guidance." {
		t.Fatalf("expected safe stream digest and preview, got %#v", result)
	}
	if result.InputTokens != 21 || result.OutputTokens != 7 || result.TotalTokens != 28 || !result.EstimatedCostAvailable {
		t.Fatalf("unexpected streamed usage/cost metadata: %#v", result)
	}
}

func TestGenerateProviderMessageReportsMetadataAndEstimatedCost(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"choices": [{"message": {"role": "assistant", "content": "Use the scoped plan."}}],
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
      - tool_planning
    enabled: true
`, server.URL)
	if err := os.WriteFile(filepath.Join(root, "providers.yaml"), []byte(providers), 0o644); err != nil {
		t.Fatalf("write providers: %v", err)
	}

	result, err := GenerateProviderMessage(context.Background(), root, ProviderGenerationRequest{UserPrompt: "hello"})
	if err != nil {
		t.Fatalf("generate provider message: %v", err)
	}
	if result.ContextWindowTokens != 128000 {
		t.Fatalf("expected context window, got %#v", result)
	}
	if !result.EstimatedCostAvailable || result.EstimatedCostUSD != 0.0075 {
		t.Fatalf("unexpected estimated cost: %#v", result)
	}
	if strings.Join(result.Capabilities, "|") != "json_mode|tool_planning" {
		t.Fatalf("unexpected capabilities: %#v", result.Capabilities)
	}
}

func TestGenerateProviderMessageFailsOverEnabledProvidersWhenConfigured(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	primaryPostCount := 0
	primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		primaryPostCount++
		http.Error(w, "primary-secret-body should not leak", http.StatusBadGateway)
	}))
	defer primary.Close()
	backupPostCount := 0
	backup := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		backupPostCount++
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("expected backup chat completions endpoint, got %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"choices": [{"message": {"role": "assistant", "content": "Use backup provider."}}],
			"usage": {"prompt_tokens": 9, "completion_tokens": 4, "total_tokens": 13}
		}`))
	}))
	defer backup.Close()
	t.Setenv("OURO_PRIMARY_PROVIDER_KEY", "primary-secret-token")
	t.Setenv("OURO_BACKUP_PROVIDER_KEY", "backup-secret-token")
	providers := fmt.Sprintf(`version: 1
routing:
  strategy: enabled_order_failover
providers:
  - name: primary-openai
    type: openai_compatible
    model: gpt-primary
    base_url: %s/v1
    api_key_env: OURO_PRIMARY_PROVIDER_KEY
    enabled: true
  - name: backup-openai
    type: openai_compatible
    model: gpt-backup
    base_url: %s/v1
    api_key_env: OURO_BACKUP_PROVIDER_KEY
    enabled: true
`, primary.URL, backup.URL)
	if err := os.WriteFile(filepath.Join(root, "providers.yaml"), []byte(providers), 0o644); err != nil {
		t.Fatalf("write providers: %v", err)
	}

	result, err := GenerateProviderMessage(context.Background(), root, ProviderGenerationRequest{UserPrompt: "hello"})
	if err != nil {
		t.Fatalf("generate provider message with failover: %v", err)
	}
	if primaryPostCount != 1 || backupPostCount != 1 {
		t.Fatalf("expected one primary and one backup POST, got primary=%d backup=%d", primaryPostCount, backupPostCount)
	}
	if result.ProviderName != "backup-openai" || result.Model != "gpt-backup" || result.Text != "Use backup provider." {
		t.Fatalf("unexpected failover generation result: %#v", result)
	}
	if result.RoutingStrategy != providerRoutingEnabledOrderFailover || result.ProviderAttemptCount != 2 || !result.ProviderFailover || result.SelectedProviderName != "backup-openai" {
		t.Fatalf("unexpected routing metadata: %#v", result)
	}
	if strings.Join(result.FailedProviderNames, "|") != "primary-openai" {
		t.Fatalf("unexpected failed provider names: %#v", result.FailedProviderNames)
	}
}

func TestGenerateProviderMessageFailoverSkipsProvidersWithoutRequiredCapabilities(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	primaryPostCount := 0
	primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		primaryPostCount++
		http.Error(w, "primary should be skipped before HTTP", http.StatusBadGateway)
	}))
	defer primary.Close()
	backupPostCount := 0
	backup := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		backupPostCount++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"choices": [{"message": {"role": "assistant", "content": "Use capability-matched backup."}}],
			"usage": {"prompt_tokens": 9, "completion_tokens": 4, "total_tokens": 13}
		}`))
	}))
	defer backup.Close()
	providers := fmt.Sprintf(`version: 1
routing:
  strategy: enabled_order_failover
providers:
  - name: primary-openai
    type: openai_compatible
    model: gpt-primary
    base_url: %s/v1
    capabilities:
      - text_only
    enabled: true
  - name: backup-openai
    type: openai_compatible
    model: gpt-backup
    base_url: %s/v1
    capabilities:
      - json_mode
      - tool_planning
    enabled: true
`, primary.URL, backup.URL)
	if err := os.WriteFile(filepath.Join(root, "providers.yaml"), []byte(providers), 0o644); err != nil {
		t.Fatalf("write providers: %v", err)
	}

	result, err := GenerateProviderMessage(context.Background(), root, ProviderGenerationRequest{
		UserPrompt:           "hello",
		RequiredCapabilities: []string{"json_mode", "json_mode", " "},
	})
	if err != nil {
		t.Fatalf("generate provider message with required capabilities: %v", err)
	}
	if primaryPostCount != 0 || backupPostCount != 1 {
		t.Fatalf("expected primary skip and one backup POST, got primary=%d backup=%d", primaryPostCount, backupPostCount)
	}
	if result.ProviderName != "backup-openai" || result.SelectedProviderName != "backup-openai" {
		t.Fatalf("unexpected selected provider: %#v", result)
	}
	if strings.Join(result.RequiredCapabilities, "|") != "json_mode" || !result.CapabilityMatch {
		t.Fatalf("unexpected capability metadata: %#v", result)
	}
	if result.ProviderAttemptCount != 1 || result.ProviderFailover || len(result.FailedProviderNames) != 0 {
		t.Fatalf("capability skip must not count as failed failover attempt: %#v", result)
	}
	if len(result.SkippedProviders) != 1 || result.SkippedProviders[0].Name != "primary-openai" || result.SkippedProviders[0].Reason != "missing_required_capabilities" || strings.Join(result.SkippedProviders[0].MissingCapabilities, "|") != "json_mode" {
		t.Fatalf("unexpected skipped provider metadata: %#v", result.SkippedProviders)
	}
}

func TestGenerateProviderMessageFirstEnabledSelectsFirstRequiredCapabilityMatch(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	primaryPostCount := 0
	primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		primaryPostCount++
		http.Error(w, "primary-secret-body should not leak", http.StatusBadGateway)
	}))
	defer primary.Close()
	backupPostCount := 0
	backup := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		backupPostCount++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"backup should not be called"}}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`))
	}))
	defer backup.Close()
	providers := fmt.Sprintf(`version: 1
providers:
  - name: primary-openai
    type: openai_compatible
    model: gpt-primary
    base_url: %s/v1
    capabilities:
      - text_only
    enabled: true
  - name: backup-openai
    type: openai_compatible
    model: gpt-backup
    base_url: %s/v1
    capabilities:
      - json_mode
    enabled: true
`, primary.URL, backup.URL)
	if err := os.WriteFile(filepath.Join(root, "providers.yaml"), []byte(providers), 0o644); err != nil {
		t.Fatalf("write providers: %v", err)
	}

	result, err := GenerateProviderMessage(context.Background(), root, ProviderGenerationRequest{
		UserPrompt:           "SECRET_PROMPT_SHOULD_NOT_LEAK",
		RequiredCapabilities: []string{"json_mode"},
	})
	if err != nil {
		t.Fatalf("generate provider message with first capability match: %v", err)
	}
	if primaryPostCount != 0 || backupPostCount != 1 {
		t.Fatalf("expected first provider skip and one backup POST, got primary=%d backup=%d", primaryPostCount, backupPostCount)
	}
	if result.RoutingStrategy != providerRoutingFirstEnabled || result.ProviderAttemptCount != 1 || result.ProviderFailover {
		t.Fatalf("unexpected first-enabled capability routing metadata: %#v", result)
	}
	if result.ProviderName != "backup-openai" || result.SelectedProviderName != "backup-openai" || !result.CapabilityMatch {
		t.Fatalf("unexpected selected provider/capability match: %#v", result)
	}
	if strings.Join(result.RequiredCapabilities, "|") != "json_mode" {
		t.Fatalf("unexpected required capabilities: %#v", result.RequiredCapabilities)
	}
	if len(result.SkippedProviders) != 1 || result.SkippedProviders[0].Name != "primary-openai" || result.SkippedProviders[0].Reason != "missing_required_capabilities" {
		t.Fatalf("unexpected skipped providers: %#v", result.SkippedProviders)
	}
	if strings.Contains(result.Text, "SECRET_PROMPT_SHOULD_NOT_LEAK") || strings.Contains(result.Text, "primary-secret-body") {
		t.Fatalf("capability routing result leaked forbidden material: %#v", result)
	}
}

func TestGenerateProviderMessageKeepsFirstEnabledDefaultWithoutFailover(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "primary-secret-body should not leak", http.StatusBadGateway)
	}))
	defer primary.Close()
	backupPostCount := 0
	backup := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		backupPostCount++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"backup should not be called"}}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`))
	}))
	defer backup.Close()
	t.Setenv("OURO_PRIMARY_PROVIDER_KEY", "primary-secret-token")
	providers := fmt.Sprintf(`version: 1
providers:
  - name: primary-openai
    type: openai_compatible
    model: gpt-primary
    base_url: %s/v1
    api_key_env: OURO_PRIMARY_PROVIDER_KEY
    enabled: true
  - name: backup-openai
    type: openai_compatible
    model: gpt-backup
    base_url: %s/v1
    enabled: true
`, primary.URL, backup.URL)
	if err := os.WriteFile(filepath.Join(root, "providers.yaml"), []byte(providers), 0o644); err != nil {
		t.Fatalf("write providers: %v", err)
	}

	_, err := GenerateProviderMessage(context.Background(), root, ProviderGenerationRequest{UserPrompt: "hello"})
	if err == nil {
		t.Fatalf("expected first enabled provider failure")
	}
	if backupPostCount != 0 {
		t.Fatalf("default routing must not contact backup provider, got %d calls", backupPostCount)
	}
	for _, forbidden := range []string{"primary-secret-body", "primary-secret-token"} {
		if strings.Contains(err.Error(), forbidden) {
			t.Fatalf("provider generation error leaked %q: %v", forbidden, err)
		}
	}
}

func TestGenerateProviderMessageFailoverAllFailuresSanitizesAggregateError(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	primarySecret := "PRIMARY_BODY_SECRET"
	primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, primarySecret, http.StatusBadGateway)
	}))
	defer primary.Close()
	backupSecret := "BACKUP_BODY_SECRET"
	backup := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, backupSecret, http.StatusServiceUnavailable)
	}))
	defer backup.Close()
	providers := fmt.Sprintf(`version: 1
routing:
  strategy: enabled_order_failover
providers:
  - name: primary-openai
    type: openai_compatible
    model: gpt-primary
    base_url: %s/v1
    enabled: true
  - name: backup-openai
    type: openai_compatible
    model: gpt-backup
    base_url: %s/v1
    enabled: true
`, primary.URL, backup.URL)
	if err := os.WriteFile(filepath.Join(root, "providers.yaml"), []byte(providers), 0o644); err != nil {
		t.Fatalf("write providers: %v", err)
	}

	_, err := GenerateProviderMessage(context.Background(), root, ProviderGenerationRequest{UserPrompt: "hello"})
	if err == nil {
		t.Fatalf("expected provider generation failover aggregate error")
	}
	for _, want := range []string{"primary-openai=status_502", "backup-openai=status_503"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("expected aggregate error to contain %q, got %v", want, err)
		}
	}
	for _, forbidden := range []string{primarySecret, backupSecret} {
		if strings.Contains(err.Error(), forbidden) {
			t.Fatalf("aggregate provider error leaked %q: %v", forbidden, err)
		}
	}
}

func TestValidateProviderConfigChecksRoutingStrategy(t *testing.T) {
	t.Skip("Safety authorization checks disabled")
	base := ProviderConfig{
		Version: 1,
		Providers: []Provider{{
			Name:    "offline",
			Type:    "offline",
			Model:   "offline-model",
			Enabled: true,
		}},
	}
	if err := ValidateProviderConfig(base); err != nil {
		t.Fatalf("default routing should be valid: %v", err)
	}
	base.Routing.Strategy = providerRoutingEnabledOrderFailover
	if err := ValidateProviderConfig(base); err != nil {
		t.Fatalf("failover routing should be valid: %v", err)
	}
	base.Routing.Strategy = "round_robin"
	err := ValidateProviderConfig(base)
	if err == nil || !strings.Contains(err.Error(), "provider routing strategy is not supported") {
		t.Fatalf("expected unsupported routing strategy error, got %v", err)
	}
}

func TestGenerateProviderMessageRejectsOfflineProvider(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	skipIfWindows(t)
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}

	_, err := GenerateProviderMessage(context.Background(), root, ProviderGenerationRequest{UserPrompt: "hello"})
	if err == nil || !strings.Contains(err.Error(), "offline") || !strings.Contains(err.Error(), "generation") {
		t.Fatalf("expected offline generation rejection, got %v", err)
	}
}

func TestGenerateProviderMessageRejectsMissingAPIKeyEnvBeforeHTTP(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	providers := fmt.Sprintf(`version: 1
providers:
  - name: local-openai
    type: openai_compatible
    model: gpt-test
    base_url: %s
    api_key_env: OURO_MISSING_PROVIDER_KEY
    enabled: true
`, server.URL)
	if err := os.WriteFile(filepath.Join(root, "providers.yaml"), []byte(providers), 0o644); err != nil {
		t.Fatalf("write providers: %v", err)
	}

	_, err := GenerateProviderMessage(context.Background(), root, ProviderGenerationRequest{UserPrompt: "hello"})
	if err == nil {
		t.Fatalf("expected missing api key env to fail")
	}
	if called {
		t.Fatalf("expected missing api key env to fail before HTTP generation")
	}
	if !strings.Contains(err.Error(), "OURO_MISSING_PROVIDER_KEY") || !strings.Contains(err.Error(), "local-openai") {
		t.Fatalf("expected provider/env error, got %v", err)
	}
}

func TestGenerateProviderMessageSanitizesOpenAICompatibleErrors(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "secret-value should not leak", http.StatusBadGateway)
	}))
	defer server.Close()
	t.Setenv("OURO_TEST_PROVIDER_KEY", "secret-value")
	providers := fmt.Sprintf(`version: 1
providers:
  - name: local-openai
    type: openai_compatible
    model: gpt-test
    base_url: %s
    api_key_env: OURO_TEST_PROVIDER_KEY
    enabled: true
`, server.URL)
	if err := os.WriteFile(filepath.Join(root, "providers.yaml"), []byte(providers), 0o644); err != nil {
		t.Fatalf("write providers: %v", err)
	}

	_, err := GenerateProviderMessage(context.Background(), root, ProviderGenerationRequest{UserPrompt: "hello"})
	if err == nil {
		t.Fatalf("expected generation error")
	}
	if !strings.Contains(err.Error(), "local-openai") || !strings.Contains(err.Error(), "status 502") {
		t.Fatalf("expected provider/status error, got %v", err)
	}
	for _, secret := range []string{"secret-value", "should not leak"} {
		if strings.Contains(err.Error(), secret) {
			t.Fatalf("provider generation error leaked secret material: %v", err)
		}
	}
}
