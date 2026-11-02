package app

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
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
	// CognitiveTier selects the model class for this request (Budget 门
	// L1/L2 routing): 0 = default flagship, 1 = tactical small model when
	// OURO_TACTIC_MODEL is set, 2 = forced flagship (gates/pivots).
	CognitiveTier int `json:"cognitive_tier"`
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
		result, err := generateProviderMessageWithTransientRetry(ctx, provider, request)
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
		result, err := generateProviderMessageWithTransientRetry(ctx, provider, request)
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

// generateProviderMessageWithTransientRetry retries network-class failures
// (TLS handshake timeouts, connection resets, proxy dials) a couple of times
// with backoff before surfacing them: long-running loops (chat turns,
// sub-loops) must survive provider hiccups instead of wedging on a single
// flaky dial. Non-network errors return immediately.
func generateProviderMessageWithTransientRetry(ctx context.Context, provider Provider, request ProviderGenerationRequest) (ProviderGenerationResult, error) {
	result, err := generateProviderMessageForProvider(ctx, provider, request)
	if err == nil {
		return result, nil
	}
	for _, delay := range []time.Duration{3 * time.Second, 9 * time.Second} {
		if !isTransientProviderNetworkError(err) {
			return result, err
		}
		select {
		case <-ctx.Done():
			return result, ctx.Err()
		case <-time.After(delay):
		}
		result, err = generateProviderMessageForProvider(ctx, provider, request)
		if err == nil {
			return result, nil
		}
	}
	return result, err
}

// isTransientProviderNetworkError matches dial/TLS/reset/EOF shapes.
func isTransientProviderNetworkError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	for _, marker := range []string{
		"tls handshake timeout", "connection refused", "connection reset",
		"i/o timeout", "proxyconnect", "dial tcp",
		"connection timed out", "unexpected eof",
	} {
		if strings.Contains(message, marker) {
			return true
		}
	}
	return false
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

// selectedModelForTier resolves the model name for a cognitive tier:
// tier=1 with OURO_TACTIC_MODEL set routes to the tactical small model;
// every other tier (and unset env) uses the provider's configured model.
func selectedModelForTier(provider Provider, tier int) string {
	if tier == 1 {
		if tacticModel := strings.TrimSpace(os.Getenv("OURO_TACTIC_MODEL")); tacticModel != "" {
			return tacticModel
		}
	}
	return provider.Model
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
	// Cognitive-tier routing (Budget 门 L1/L2): tier=1 with OURO_TACTIC_MODEL
	// set sends this request with the tactical small model instead of the
	// provider's flagship; tier=2/0 and unset env always use provider.Model.
	// The routed copy is scoped to this call so result.Model / request body
	// carry the same name the API actually receives.
	provider.Model = selectedModelForTier(provider, request.CognitiveTier)
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

// dnsCache caches resolved IPs for provider hosts. The host machine's
// Docker DNS (192.168.65.7) intermittently dies after Docker Desktop
// restarts; with a warm cache the dial bypasses DNS entirely and the
// campaign survives the outage.
var dnsCache sync.Map // host -> []string

func cachedLookupHost(host string) ([]string, error) {
	if ips, ok := dnsCache.Load(host); ok {
		return ips.([]string), nil
	}
	ips, err := net.DefaultResolver.LookupHost(context.Background(), host)
	if err != nil || len(ips) == 0 {
		return ips, err
	}
	dnsCache.Store(host, ips)
	return ips, nil
}

// openAICompatibleTransport is the shared keep-alive transport for
// provider calls: pooled connections avoid the TLS handshake on every
// turn (this network drops handshakes intermittently), and a short
// handshake deadline fails fast into the caller's bounded retry.
var openAICompatibleTransport = &http.Transport{
	MaxIdleConns:        16,
	MaxIdleConnsPerHost: 8,
	IdleConnTimeout:     5 * time.Minute,
	TLSHandshakeTimeout: 15 * time.Second,
	DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
		// DNS-outage resilience: resolve via cache first; fall back to the
		// original resolver only on a miss.
		host, port, err := net.SplitHostPort(addr)
		if err != nil {
			return (&net.Dialer{Timeout: 30 * time.Second}).DialContext(ctx, network, addr)
		}
		if ips, lerr := cachedLookupHost(host); lerr == nil && len(ips) > 0 {
			var lastErr error
			for _, ip := range ips {
				conn, derr := (&net.Dialer{Timeout: 30 * time.Second}).DialContext(ctx, network, net.JoinHostPort(ip, port))
				if derr == nil {
					return conn, nil
				}
				lastErr = derr
			}
			// Cache invalidation: every cached IP refused/timed out — the
			// provider likely rotated its endpoint. Evict and re-resolve
			// once (fresh DNS), so a stale IP never poisons the campaign.
			dnsCache.Delete(host)
			if fresh, ferr := net.DefaultResolver.LookupHost(context.Background(), host); ferr == nil && len(fresh) > 0 {
				dnsCache.Store(host, fresh)
				for _, ip := range fresh {
					conn, derr := (&net.Dialer{Timeout: 30 * time.Second}).DialContext(ctx, network, net.JoinHostPort(ip, port))
					if derr == nil {
						return conn, nil
					}
				}
			}
			return nil, lastErr
		}
		return (&net.Dialer{Timeout: 30 * time.Second}).DialContext(ctx, network, addr)
	},
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
	client := &http.Client{Timeout: openAICompatibleProviderTimeout(provider), Transport: openAICompatibleTransport}
	if request.TimeoutMS > 0 {
		client.Timeout = time.Duration(request.TimeoutMS) * time.Millisecond
	}
	started := time.Now()
	response, err := client.Do(httpRequest)
	if err != nil && isTransientHTTPError(err) {
		// TLS handshake drops are intermittent on this network: one
		// fast retry on the pooled connection before failing the turn.
		time.Sleep(2 * time.Second)
		response, err = client.Do(httpRequest)
	}
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
	// ThinkingMode disables GLM-class in-content reasoning for bounded
	// intent calls (the guillotine path); omitted otherwise.
	Thinking *openAIThinkingControl `json:"thinking,omitempty"`
}

type openAICompatibleStreamOptions struct {
	IncludeUsage bool `json:"include_usage"`
}

type openAICompatibleChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
	// ReasoningContent is the GLM-class thinking channel: streamed deltas
	// carry it separately; some modes emit the entire answer there.
	ReasoningContent string `json:"reasoning_content,omitempty"`
}

type openAIThinkingControl struct {
	Type string `json:"type"`
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

// StreamWithRootJSONGuillotine streams a chat completion but physically
// cancels the connection the moment the root JSON object closes — the
// "token guillotine" (v28.3). Non-separated-reasoning models will happily
// write thousands of characters after the intent JSON; with the guillotine
// the parent decision costs only the tokens up to the closing brace.
//
// The accumulated text is returned regardless of how the stream ended
// (natural [DONE] or guillotine) — the caller parses the JSON prefix.
func StreamWithRootJSONGuillotine(ctx context.Context, root string, request ProviderGenerationRequest) (ProviderGenerationResult, error) {
	// Reuse the streaming request path with a forced Stream flag.
	request.Stream = true
	request.MaxTokens = 8192 // GLM-class thinking runs in-content; the brace cancel stops essays, this cap only bounds total cost

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
	selected := providers[0]

	baseURLValue, err := effectiveProviderBaseURL(selected)
	if err != nil {
		return ProviderGenerationResult{}, err
	}
	baseURL, err := parseOpenAICompatibleProviderBaseURL(baseURLValue)
	if err != nil {
		return ProviderGenerationResult{}, fmt.Errorf("provider %s base_url is invalid: %w", selected.Name, err)
	}

	chatMessages := make([]openAICompatibleChatMessage, 0, 4)
	if system := strings.TrimSpace(request.SystemPrompt); system != "" {
		chatMessages = append(chatMessages, openAICompatibleChatMessage{Role: "system", Content: system})
	}
	chatMessages = append(chatMessages, openAICompatibleChatMessage{Role: "user", Content: request.UserPrompt})
	body := openAICompatibleChatCompletionRequest{
		Model:         selectedModelForTier(selected, request.CognitiveTier),
		Messages:      chatMessages,
		Stream:        true,
		StreamOptions: &openAICompatibleStreamOptions{IncludeUsage: true},
		MaxTokens:     8192,
		// Intent envelopes must not burn the cap on in-content reasoning:
		// GLM-class models honor this toggle; others ignore the field.
		Thinking: &openAIThinkingControl{Type: "disabled"},
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return ProviderGenerationResult{}, fmt.Errorf("provider %s guillotine request encode: %w", selected.Name, err)
	}
	endpoint := strings.TrimRight(baseURL.String(), "/") + "/chat/completions"
	guillotineCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	httpRequest, err := http.NewRequestWithContext(guillotineCtx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return ProviderGenerationResult{}, fmt.Errorf("provider %s guillotine request: %w", selected.Name, err)
	}
	httpRequest.Header.Set("Content-Type", "application/json")
	if err := applyOpenAICompatibleBearer(httpRequest, selected); err != nil {
		return ProviderGenerationResult{}, err
	}
	client := &http.Client{Timeout: 90 * time.Second}
	started := time.Now()
	response, err := client.Do(httpRequest)
	if err != nil {
		return ProviderGenerationResult{}, fmt.Errorf("provider %s guillotine transport: %w", selected.Name, err)
	}
	defer func() { _ = response.Body.Close() }()

	// Incremental SSE scan with root-JSON brace tracking.
	scanner := bufio.NewScanner(response.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	var text strings.Builder
	depth := 0
	seenRoot := false
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" || data == "[DONE]" {
			continue
		}
		var chunk openAICompatibleChatCompletionStreamChunk
		if json.Unmarshal([]byte(data), &chunk) != nil {
			continue
		}
		for _, choice := range chunk.Choices {
			// GLM-class models stream reasoning in reasoning_content and
			// the answer in content; some modes emit the ENTIRE answer as
			// reasoning. Accumulate both channels so the envelope JSON is
			// captured whichever lane it arrives in (brace tracking runs
			// over the same combined text).
			delta := choice.Delta.Content
			if delta == "" {
				delta = choice.Delta.ReasoningContent
			}
			if delta == "" {
				continue
			}
			text.WriteString(delta)
			// Track brace depth outside strings.
			for _, r := range delta {
				switch r {
				case '{':
					if !inJSONString(text.String()) {
						depth++
						seenRoot = true
					}
				case '}':
					if !inJSONString(text.String()) {
						depth--
						if seenRoot && depth == 0 {
							// Root object closed: GUILLOTINE.
							cancel()
							return ProviderGenerationResult{
								Text:         text.String(),
								Model:        selectedModelForTier(selected, request.CognitiveTier),
								Latency:      time.Since(started),
								ProviderName: selected.Name,
							}, nil
						}
					}
				}
			}
		}
	}
	return ProviderGenerationResult{
		Text:         text.String(),
		Model:        selectedModelForTier(selected, request.CognitiveTier),
		Latency:      time.Since(started),
		ProviderName: selected.Name,
	}, scanner.Err()
}

// inJSONString reports whether the LAST character of the accumulated text
// sits inside a JSON string literal (cheap heuristic: count unescaped
// quotes; odd count = inside a string). Used by the brace tracker so braces
// inside string values don't fool the depth counter.
func inJSONString(accumulated string) bool {
	quotes := 0
	escaped := false
	for _, r := range accumulated {
		if escaped {
			escaped = false
			continue
		}
		if r == '\\' {
			escaped = true
			continue
		}
		if r == '"' {
			quotes++
		}
	}
	return quotes%2 == 1
}

// isTransientHTTPError reports transport-level failures worth one retry.
func isTransientHTTPError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "TLS handshake timeout") ||
		strings.Contains(msg, "connection reset") ||
		strings.Contains(msg, "EOF") ||
		strings.Contains(msg, "broken pipe")
}
