package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type ProviderGenerationRequest struct {
	SystemPrompt string
	UserPrompt   string
	MaxTokens    int
}

type ProviderGenerationResult struct {
	ProviderName string
	Model        string
	Text         string
	InputTokens  int
	OutputTokens int
	TotalTokens  int
	Latency      time.Duration
}

func GenerateProviderMessage(ctx context.Context, root string, request ProviderGenerationRequest) (ProviderGenerationResult, error) {
	if strings.TrimSpace(request.UserPrompt) == "" {
		return ProviderGenerationResult{}, fmt.Errorf("provider generation prompt is required")
	}
	if request.MaxTokens < 0 {
		return ProviderGenerationResult{}, fmt.Errorf("provider generation max_tokens must be non-negative")
	}
	config, err := LoadProviderConfig(filepath.Join(root, "providers.yaml"))
	if err != nil {
		return ProviderGenerationResult{}, fmt.Errorf("load provider config: %w", err)
	}
	if err := ValidateProviderConfig(config); err != nil {
		return ProviderGenerationResult{}, fmt.Errorf("invalid provider config: %w", err)
	}
	provider, ok := EnabledProvider(config)
	if !ok {
		return ProviderGenerationResult{}, fmt.Errorf("provider config has no enabled provider")
	}
	switch strings.TrimSpace(provider.Type) {
	case "offline":
		return ProviderGenerationResult{}, fmt.Errorf("provider %s type offline does not support generation", provider.Name)
	case providerTypeOpenAICompatible:
		return generateOpenAICompatibleMessage(ctx, provider, request)
	default:
		return ProviderGenerationResult{}, fmt.Errorf("provider adapter is not available for type %s", provider.Type)
	}
}

func generateOpenAICompatibleMessage(ctx context.Context, provider Provider, request ProviderGenerationRequest) (ProviderGenerationResult, error) {
	if err := ctx.Err(); err != nil {
		return ProviderGenerationResult{}, err
	}
	baseURL, err := parseOpenAICompatibleProviderBaseURL(provider.BaseURL)
	if err != nil {
		return ProviderGenerationResult{}, fmt.Errorf("provider %s base_url is invalid: %w", provider.Name, err)
	}
	body := openAICompatibleChatCompletionRequest{
		Model: strings.TrimSpace(provider.Model),
		Messages: []openAICompatibleChatMessage{
			{Role: "user", Content: strings.TrimSpace(request.UserPrompt)},
		},
	}
	if system := strings.TrimSpace(request.SystemPrompt); system != "" {
		body.Messages = append([]openAICompatibleChatMessage{{Role: "system", Content: system}}, body.Messages...)
	}
	if request.MaxTokens > 0 {
		body.MaxTokens = request.MaxTokens
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return ProviderGenerationResult{}, fmt.Errorf("provider %s generation request encode: %w", provider.Name, err)
	}
	endpoint := strings.TrimRight(baseURL.String(), "/") + "/chat/completions"
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return ProviderGenerationResult{}, fmt.Errorf("provider %s generation request: %w", provider.Name, err)
	}
	httpRequest.Header.Set("Content-Type", "application/json")
	if err := applyOpenAICompatibleBearer(httpRequest, provider); err != nil {
		return ProviderGenerationResult{}, err
	}
	client := &http.Client{Timeout: openAICompatibleProviderTimeout(provider)}
	started := time.Now()
	response, err := client.Do(httpRequest)
	latency := time.Since(started)
	if err != nil {
		return ProviderGenerationResult{}, fmt.Errorf("provider %s generation failed: %w", provider.Name, err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return ProviderGenerationResult{}, fmt.Errorf("provider %s generation failed with status %d", provider.Name, response.StatusCode)
	}
	var decoded openAICompatibleChatCompletionResponse
	if err := json.NewDecoder(response.Body).Decode(&decoded); err != nil {
		return ProviderGenerationResult{}, fmt.Errorf("provider %s generation response decode: %w", provider.Name, err)
	}
	text := firstOpenAICompatibleChoiceText(decoded)
	if strings.TrimSpace(text) == "" {
		return ProviderGenerationResult{}, fmt.Errorf("provider %s generation response had no assistant content", provider.Name)
	}
	totalTokens := decoded.Usage.TotalTokens
	if totalTokens == 0 {
		totalTokens = decoded.Usage.PromptTokens + decoded.Usage.CompletionTokens
	}
	return ProviderGenerationResult{
		ProviderName: strings.TrimSpace(provider.Name),
		Model:        strings.TrimSpace(provider.Model),
		Text:         text,
		InputTokens:  decoded.Usage.PromptTokens,
		OutputTokens: decoded.Usage.CompletionTokens,
		TotalTokens:  totalTokens,
		Latency:      latency,
	}, nil
}

type openAICompatibleChatCompletionRequest struct {
	Model     string                        `json:"model"`
	Messages  []openAICompatibleChatMessage `json:"messages"`
	MaxTokens int                           `json:"max_tokens,omitempty"`
}

type openAICompatibleChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openAICompatibleChatCompletionResponse struct {
	Choices []struct {
		Message openAICompatibleChatMessage `json:"message"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

func firstOpenAICompatibleChoiceText(response openAICompatibleChatCompletionResponse) string {
	for _, choice := range response.Choices {
		if strings.TrimSpace(choice.Message.Content) != "" {
			return choice.Message.Content
		}
	}
	return ""
}

func applyOpenAICompatibleBearer(request *http.Request, provider Provider) error {
	if envName := strings.TrimSpace(provider.APIKeyEnv); envName != "" {
		apiKey, ok := os.LookupEnv(envName)
		if !ok || strings.TrimSpace(apiKey) == "" {
			return fmt.Errorf("provider %s api key env %s is required", provider.Name, envName)
		}
		request.Header.Set("Authorization", "Bearer "+apiKey)
	}
	return nil
}

func openAICompatibleProviderTimeout(provider Provider) time.Duration {
	if provider.TimeoutMS > 0 {
		return time.Duration(provider.TimeoutMS) * time.Millisecond
	}
	return 5 * time.Second
}

func openAICompatibleModelEndpoint(baseURL *url.URL, model string) string {
	return strings.TrimRight(baseURL.String(), "/") + "/models/" + url.PathEscape(strings.TrimSpace(model))
}
