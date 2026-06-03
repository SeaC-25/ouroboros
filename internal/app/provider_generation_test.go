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

func TestGenerateProviderMessageRejectsOfflineProvider(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
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
