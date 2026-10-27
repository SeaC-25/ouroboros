package app

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/ouroboros/ouroboros/internal/modelruntime"
)

type ProviderGenerationRequest struct {
	SystemPrompt         string
	UserPrompt           string
	Messages             []ProviderGenerationMessage
	MaxTokens            int
	RequiredCapabilities []string
	Stream               bool
	TimeoutMS            int
}

type ProviderGenerationMessage struct {
	Role    string
	Content string
}

type ProviderGenerationResult struct {
	ProviderName           string
	Model                  string
	Text                   string
	InputTokens            int
	OutputTokens           int
	TotalTokens            int
	Latency                time.Duration
	ContextWindowTokens    int
	Capabilities           []string
	EstimatedCostUSD       float64
	EstimatedCostAvailable bool
	RoutingStrategy        string
	ProviderAttemptCount   int
	ProviderFailover       bool
	SelectedProviderName   string
	FailedProviderNames    []string
	RequiredCapabilities   []string
	CapabilityMatch        bool
	SkippedProviders       []ProviderSkippedProvider
	Streaming              bool
	StreamChunkCount       int
	StreamCompleted        bool
	StreamDigest           string
	StreamPreview          string
}

type ProviderSkippedProvider struct {
	Name                string   `json:"name"`
	Reason              string   `json:"reason"`
	MissingCapabilities []string `json:"missing_capabilities,omitempty"`
}

func GenerateProviderMessage(ctx context.Context, root string, request ProviderGenerationRequest) (ProviderGenerationResult, error) {
	if err := validateProviderGenerationRequestMessages(request); err != nil {
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
	providers := EnabledProviders(config)
	if len(providers) == 0 {
		return ProviderGenerationResult{}, fmt.Errorf("provider config has no enabled provider")
	}
	strategy, err := providerRoutingStrategy(config)
	if err != nil {
		return ProviderGenerationResult{}, err
	}
	requiredCapabilities := NormalizeProviderCapabilities(request.RequiredCapabilities)
	if strategy != providerRoutingEnabledOrderFailover {
		provider, skippedProviders, ok := firstProviderMatchingRequiredCapabilities(providers, requiredCapabilities)
		if !ok {
			return ProviderGenerationResult{}, noProviderMatchesRequiredCapabilitiesError("provider generation", strategy, requiredCapabilities, skippedProviders)
		}
		result, err := generateProviderMessageForProvider(ctx, provider, request)
		if err != nil {
			return ProviderGenerationResult{}, err
		}
		return providerGenerationResultWithRouting(result, strategy, 1, nil, requiredCapabilities, skippedProviders), nil
	}
	failedProviderNames := []string{}
	skippedProviders := []ProviderSkippedProvider{}
	failures := []string{}
	attemptCount := 0
	for _, provider := range providers {
		if skipped, ok := skippedProviderForMissingRequiredCapabilities(provider, requiredCapabilities); ok {
			skippedProviders = append(skippedProviders, skipped)
			continue
		}
		attemptCount++
		result, err := generateProviderMessageForProvider(ctx, provider, request)
		if err == nil {
			return providerGenerationResultWithRouting(result, strategy, attemptCount, failedProviderNames, requiredCapabilities, skippedProviders), nil
		}
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return ProviderGenerationResult{}, err
		}
		failedProviderNames = append(failedProviderNames, strings.TrimSpace(provider.Name))
		failures = append(failures, providerAttemptFailureSummary(provider.Name, err))
	}
	if attemptCount == 0 {
		return ProviderGenerationResult{}, noProviderMatchesRequiredCapabilitiesError("provider generation", strategy, requiredCapabilities, skippedProviders)
	}
	return ProviderGenerationResult{}, fmt.Errorf("provider generation failed after %s attempts: %s", strategy, strings.Join(failures, "; "))
}

func validateProviderGenerationRequestMessages(request ProviderGenerationRequest) error {
	messages := providerGenerationMessages(request)
	if len(messages) == 0 {
		return fmt.Errorf("missing prompt")
	}
	if messages[len(messages)-1].Role != "user" {
		return fmt.Errorf("missing current user prompt")
	}
	return nil
}

func providerGenerationMessages(request ProviderGenerationRequest) []ProviderGenerationMessage {
	if len(request.Messages) > 0 {
		return cleanProviderGenerationMessages(request.Messages)
	}
	prompt := strings.TrimSpace(request.UserPrompt)
	if prompt == "" {
		return nil
	}
	return []ProviderGenerationMessage{{Role: "user", Content: prompt}}
}

func cleanProviderGenerationMessages(messages []ProviderGenerationMessage) []ProviderGenerationMessage {
	cleaned := []ProviderGenerationMessage{}
	for _, message := range messages {
		role := strings.ToLower(strings.TrimSpace(message.Role))
		content := strings.TrimSpace(message.Content)
		if content == "" {
			continue
		}
		if role != "user" && role != "assistant" {
			continue
		}
		cleaned = append(cleaned, ProviderGenerationMessage{Role: role, Content: content})
	}
	return cleaned
}

func generateProviderMessageForProvider(ctx context.Context, provider Provider, request ProviderGenerationRequest) (ProviderGenerationResult, error) {
	switch strings.TrimSpace(provider.Type) {
	case "offline":
		return ProviderGenerationResult{}, fmt.Errorf("provider %s type offline does not support generation", provider.Name)
	case providerTypeOpenAICompatible:
		return generateOpenAICompatibleMessage(ctx, provider, request)
	default:
		return ProviderGenerationResult{}, fmt.Errorf("provider adapter is not available for type %s", provider.Type)
	}
}

func providerGenerationResultWithRouting(result ProviderGenerationResult, strategy string, attemptCount int, failedProviderNames []string, requiredCapabilities []string, skippedProviders []ProviderSkippedProvider) ProviderGenerationResult {
	result.RoutingStrategy = strategy
	result.ProviderAttemptCount = attemptCount
	result.ProviderFailover = len(failedProviderNames) > 0
	result.SelectedProviderName = strings.TrimSpace(result.ProviderName)
	result.FailedProviderNames = append([]string{}, failedProviderNames...)
	result.RequiredCapabilities = append([]string{}, requiredCapabilities...)
	result.CapabilityMatch = len(requiredCapabilities) > 0
	result.SkippedProviders = appendProviderSkippedProviders(nil, skippedProviders...)
	return result
}

func providerAttemptFailureSummary(providerName string, err error) string {
	name := strings.TrimSpace(providerName)
	if name == "" {
		name = "unknown"
	}
	reason := "failed"
	if err != nil {
		message := err.Error()
		switch {
		case strings.Contains(message, "status "):
			reason = "status_" + providerStatusCodeFromError(message)
		case strings.Contains(message, "api key env"):
			reason = "api_key_env_required"
		case strings.Contains(message, "does not support generation"):
			reason = "generation_unsupported"
		case strings.Contains(message, "adapter is not available"):
			reason = "adapter_unavailable"
		case strings.Contains(message, "base_url is invalid"):
			reason = "invalid_base_url"
		case strings.Contains(message, "response decode"):
			reason = "response_decode_failed"
		case strings.Contains(message, "no assistant content"):
			reason = "empty_assistant_content"
		case strings.Contains(message, "request encode"):
			reason = "request_encode_failed"
		case strings.Contains(message, "request"):
			reason = "request_failed"
		}
	}
	return name + "=" + reason
}

func providerStatusCodeFromError(message string) string {
	fields := strings.Fields(message)
	for i, field := range fields {
		if field == "status" && i+1 < len(fields) {
			return strings.Trim(fields[i+1], ".,;:")
		}
	}
	return "unknown"
}

func generateOpenAICompatibleMessage(ctx context.Context, provider Provider, request ProviderGenerationRequest) (ProviderGenerationResult, error) {
	if err := ctx.Err(); err != nil {
		return ProviderGenerationResult{}, err
	}
	baseURL, err := parseOpenAICompatibleProviderBaseURL(providerEffectiveBaseURL(provider))
	if err != nil {
		return ProviderGenerationResult{}, fmt.Errorf("provider %s base_url is invalid: %w", provider.Name, err)
	}
	messages := []openAICompatibleChatMessage{}
	for _, message := range providerGenerationMessages(request) {
		messages = append(messages, openAICompatibleChatMessage{
			Role:    message.Role,
			Content: message.Content,
		})
	}
	body := openAICompatibleChatCompletionRequest{
		Model:    strings.TrimSpace(provider.Model),
		Messages: messages,
		Stream:   request.Stream,
	}
	if request.Stream {
		body.StreamOptions = &openAICompatibleStreamOptions{IncludeUsage: true}
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
	if request.TimeoutMS > 0 {
		client.Timeout = time.Duration(request.TimeoutMS) * time.Millisecond
	}
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
	if request.Stream {
		return decodeOpenAICompatibleStreamingMessage(response.Body, provider, latency)
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
	inputPrice, outputPrice, costAvailable := providerGenerationPricing(provider)
	estimatedCost := 0.0
	if costAvailable {
		estimatedCost = modelruntime.EstimateTokenCost(decoded.Usage.PromptTokens, decoded.Usage.CompletionTokens, inputPrice, outputPrice)
	}
	return ProviderGenerationResult{
		ProviderName:           strings.TrimSpace(provider.Name),
		Model:                  strings.TrimSpace(provider.Model),
		Text:                   text,
		InputTokens:            decoded.Usage.PromptTokens,
		OutputTokens:           decoded.Usage.CompletionTokens,
		TotalTokens:            totalTokens,
		Latency:                latency,
		ContextWindowTokens:    provider.ContextWindowTokens,
		Capabilities:           normalizeProviderCapabilities(provider.Capabilities),
		EstimatedCostUSD:       estimatedCost,
		EstimatedCostAvailable: costAvailable,
	}, nil
}

func decodeOpenAICompatibleStreamingMessage(body io.Reader, provider Provider, latency time.Duration) (ProviderGenerationResult, error) {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	var builder strings.Builder
	var usage *openAICompatibleTokenUsage
	chunkCount := 0
	completed := false
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, ":") {
			continue
		}
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" {
			continue
		}
		if data == "[DONE]" {
			completed = true
			continue
		}
		var chunk openAICompatibleChatCompletionStreamChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			return ProviderGenerationResult{}, fmt.Errorf("provider %s generation stream response decode: %w", provider.Name, err)
		}
		if chunk.Usage != nil {
			usage = chunk.Usage
		}
		for _, choice := range chunk.Choices {
			if choice.Delta.Content == "" {
				continue
			}
			builder.WriteString(choice.Delta.Content)
			chunkCount++
		}
	}
	if err := scanner.Err(); err != nil {
		return ProviderGenerationResult{}, fmt.Errorf("provider %s generation stream response read: %w", provider.Name, err)
	}
	text := builder.String()
	if strings.TrimSpace(text) == "" {
		return ProviderGenerationResult{}, fmt.Errorf("provider %s generation response had no assistant content", provider.Name)
	}
	tokenUsage := openAICompatibleTokenUsage{}
	usageAvailable := false
	if usage != nil {
		tokenUsage = *usage
		usageAvailable = true
	}
	totalTokens := tokenUsage.TotalTokens
	if totalTokens == 0 {
		totalTokens = tokenUsage.PromptTokens + tokenUsage.CompletionTokens
	}
	inputPrice, outputPrice, pricingAvailable := providerGenerationPricing(provider)
	estimatedCost := 0.0
	costAvailable := pricingAvailable && usageAvailable
	if costAvailable {
		estimatedCost = modelruntime.EstimateTokenCost(tokenUsage.PromptTokens, tokenUsage.CompletionTokens, inputPrice, outputPrice)
	}
	return ProviderGenerationResult{
		ProviderName:           strings.TrimSpace(provider.Name),
		Model:                  strings.TrimSpace(provider.Model),
		Text:                   text,
		InputTokens:            tokenUsage.PromptTokens,
		OutputTokens:           tokenUsage.CompletionTokens,
		TotalTokens:            totalTokens,
		Latency:                latency,
		ContextWindowTokens:    provider.ContextWindowTokens,
		Capabilities:           normalizeProviderCapabilities(provider.Capabilities),
		EstimatedCostUSD:       estimatedCost,
		EstimatedCostAvailable: costAvailable,
		Streaming:              true,
		StreamChunkCount:       chunkCount,
		StreamCompleted:        completed,
		StreamDigest:           providerGenerationTextDigest(text),
		StreamPreview:          providerGenerationTextPreview(text),
	}, nil
}

func providerGenerationPricing(provider Provider) (float64, float64, bool) {
	inputPrice := 0.0
	outputPrice := 0.0
	available := false
	if provider.InputPricePerMillionTokens != nil {
		inputPrice = *provider.InputPricePerMillionTokens
		available = true
	}
	if provider.OutputPricePerMillionTokens != nil {
		outputPrice = *provider.OutputPricePerMillionTokens
		available = true
	}
	return inputPrice, outputPrice, available
}

func NormalizeProviderCapabilities(capabilities []string) []string {
	seen := map[string]bool{}
	normalized := make([]string, 0, len(capabilities))
	for _, capability := range capabilities {
		capability = strings.TrimSpace(capability)
		if capability == "" || seen[capability] {
			continue
		}
		seen[capability] = true
		normalized = append(normalized, capability)
	}
	sort.Strings(normalized)
	return normalized
}

func normalizeProviderCapabilities(capabilities []string) []string {
	return NormalizeProviderCapabilities(capabilities)
}

func firstProviderMatchingRequiredCapabilities(providers []Provider, requiredCapabilities []string) (Provider, []ProviderSkippedProvider, bool) {
	skippedProviders := []ProviderSkippedProvider{}
	for _, provider := range providers {
		if skipped, ok := skippedProviderForMissingRequiredCapabilities(provider, requiredCapabilities); ok {
			skippedProviders = append(skippedProviders, skipped)
			continue
		}
		return provider, skippedProviders, true
	}
	return Provider{}, skippedProviders, false
}

func skippedProviderForMissingRequiredCapabilities(provider Provider, requiredCapabilities []string) (ProviderSkippedProvider, bool) {
	if len(requiredCapabilities) == 0 {
		return ProviderSkippedProvider{}, false
	}
	missing := missingProviderCapabilities(provider, requiredCapabilities)
	if len(missing) == 0 {
		return ProviderSkippedProvider{}, false
	}
	return ProviderSkippedProvider{
		Name:                strings.TrimSpace(provider.Name),
		Reason:              "missing_required_capabilities",
		MissingCapabilities: missing,
	}, true
}

func missingProviderCapabilities(provider Provider, requiredCapabilities []string) []string {
	if len(requiredCapabilities) == 0 {
		return nil
	}
	available := map[string]bool{}
	for _, capability := range NormalizeProviderCapabilities(provider.Capabilities) {
		available[capability] = true
	}
	missing := []string{}
	for _, capability := range requiredCapabilities {
		if !available[capability] {
			missing = append(missing, capability)
		}
	}
	return missing
}

func noProviderMatchesRequiredCapabilitiesError(operation string, strategy string, requiredCapabilities []string, skippedProviders []ProviderSkippedProvider) error {
	return fmt.Errorf("%s failed after %s capability routing: no enabled provider satisfies required_capabilities=%s skipped_providers=%s", operation, strategy, providerCapabilityList(requiredCapabilities), providerSkippedProvidersSummary(skippedProviders))
}

func providerCapabilityList(capabilities []string) string {
	if len(capabilities) == 0 {
		return "none"
	}
	return strings.Join(NormalizeProviderCapabilities(capabilities), "|")
}

func providerSkippedProvidersSummary(skippedProviders []ProviderSkippedProvider) string {
	if len(skippedProviders) == 0 {
		return "none"
	}
	parts := make([]string, 0, len(skippedProviders))
	for _, skipped := range skippedProviders {
		name := strings.TrimSpace(skipped.Name)
		if name == "" {
			name = "unknown"
		}
		reason := strings.TrimSpace(skipped.Reason)
		if reason == "" {
			reason = "skipped"
		}
		parts = append(parts, name+"="+reason)
	}
	sort.Strings(parts)
	return strings.Join(parts, "|")
}

func appendProviderSkippedProviders(dst []ProviderSkippedProvider, skippedProviders ...ProviderSkippedProvider) []ProviderSkippedProvider {
	for _, skipped := range skippedProviders {
		cleaned := ProviderSkippedProvider{
			Name:                strings.TrimSpace(skipped.Name),
			Reason:              strings.TrimSpace(skipped.Reason),
			MissingCapabilities: NormalizeProviderCapabilities(skipped.MissingCapabilities),
		}
		if cleaned.Name == "" || cleaned.Reason == "" {
			continue
		}
		dst = append(dst, cleaned)
	}
	return dst
}

type openAICompatibleChatCompletionRequest struct {
	Model         string                         `json:"model"`
	Messages      []openAICompatibleChatMessage  `json:"messages"`
	MaxTokens     int                            `json:"max_tokens,omitempty"`
	Stream        bool                           `json:"stream,omitempty"`
	StreamOptions *openAICompatibleStreamOptions `json:"stream_options,omitempty"`
}

type openAICompatibleStreamOptions struct {
	IncludeUsage bool `json:"include_usage"`
}

type openAICompatibleChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openAICompatibleTokenUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type openAICompatibleChatCompletionMessage struct {
	Role             string `json:"role"`
	Content          string `json:"content"`
	ReasoningContent string `json:"reasoning_content"`
}

type openAICompatibleChatCompletionResponse struct {
	Choices []struct {
		Message openAICompatibleChatCompletionMessage `json:"message"`
	} `json:"choices"`
	Usage openAICompatibleTokenUsage `json:"usage"`
}

type openAICompatibleChatCompletionStreamChunk struct {
	Choices []struct {
		Delta openAICompatibleChatMessage `json:"delta"`
	} `json:"choices"`
	Usage *openAICompatibleTokenUsage `json:"usage"`
}

func firstOpenAICompatibleChoiceText(response openAICompatibleChatCompletionResponse) string {
	for _, choice := range response.Choices {
		if strings.TrimSpace(choice.Message.Content) != "" {
			return choice.Message.Content
		}
		if strings.TrimSpace(choice.Message.ReasoningContent) != "" {
			return choice.Message.ReasoningContent
		}
	}
	return ""
}

// effectiveProviderBaseURL resolves the provider's base URL: base_url_env
// takes precedence (read at call time — AGENTS.md env-first rule), falling
// back to the yaml base_url value.
func effectiveProviderBaseURL(provider Provider) (string, error) {
	if envName := strings.TrimSpace(provider.BaseURLEnv); envName != "" {
		if value := strings.TrimSpace(os.Getenv(envName)); value != "" {
			return value, nil
		}
	}
	if value := strings.TrimSpace(provider.BaseURL); value != "" {
		return value, nil
	}
	return "", fmt.Errorf("provider %s base_url is required (neither base_url_env %q nor base_url is set)", provider.Name, strings.TrimSpace(provider.BaseURLEnv))
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
