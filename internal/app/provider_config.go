package app

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type OpenAICompatibleProviderConfiguration struct {
	Name                string
	BaseURL             string
	BaseURLEnv          string
	Model               string
	APIKeyEnv           string
	Enabled             bool
	TimeoutMS           int
	ContextWindowTokens int
	Capabilities        []string
	RoutingStrategy     string
}

type ProviderConfigurationResult struct {
	Path              string
	Provider          Provider
	RoutingStrategy   string
	APIKeyValueStored bool
}

func ConfigureOpenAICompatibleProvider(root string, options OpenAICompatibleProviderConfiguration) (ProviderConfigurationResult, error) {
	if strings.TrimSpace(root) == "" {
		return ProviderConfigurationResult{}, fmt.Errorf("workspace path is required")
	}
	providersPath := filepath.Join(root, "providers.yaml")
	config, err := LoadProviderConfig(providersPath)
	if err != nil {
		return ProviderConfigurationResult{}, fmt.Errorf("load provider config: %w", err)
	}

	provider, err := openAICompatibleProviderFromConfiguration(options)
	if err != nil {
		return ProviderConfigurationResult{}, err
	}
	if routingStrategy := strings.TrimSpace(options.RoutingStrategy); routingStrategy != "" {
		config.Routing.Strategy = routingStrategy
	}
	config.Providers = upsertConfiguredProvider(config.Providers, provider)
	if err := ValidateProviderConfig(config); err != nil {
		return ProviderConfigurationResult{}, fmt.Errorf("invalid provider config: %w", err)
	}
	resolvedRoutingStrategy, err := providerRoutingStrategy(config)
	if err != nil {
		return ProviderConfigurationResult{}, err
	}
	if err := writeProviderConfig(providersPath, config); err != nil {
		return ProviderConfigurationResult{}, err
	}
	return ProviderConfigurationResult{
		Path:              providersPath,
		Provider:          provider,
		RoutingStrategy:   resolvedRoutingStrategy,
		APIKeyValueStored: false,
	}, nil
}

func openAICompatibleProviderFromConfiguration(options OpenAICompatibleProviderConfiguration) (Provider, error) {
	name := strings.TrimSpace(options.Name)
	if name == "" {
		return Provider{}, fmt.Errorf("provider name is required")
	}
	model := strings.TrimSpace(options.Model)
	if model == "" {
		return Provider{}, fmt.Errorf("provider model is required")
	}
	baseURL := ""
	if trimmedBaseURL := strings.TrimSpace(options.BaseURL); trimmedBaseURL != "" {
		parsed, err := parseOpenAICompatibleProviderBaseURL(trimmedBaseURL)
		if err != nil {
			return Provider{}, fmt.Errorf("provider %s base_url is invalid: %w", name, err)
		}
		baseURL = parsed.String()
	}
	baseURLEnv := strings.TrimSpace(options.BaseURLEnv)
	if baseURL == "" && baseURLEnv == "" {
		return Provider{}, fmt.Errorf("provider %s base_url is invalid: %w", name, errors.New("base_url is required"))
	}
	if baseURLEnv != "" && !validProviderAPIKeyEnvName(baseURLEnv) {
		return Provider{}, fmt.Errorf("provider %s base_url_env must be an environment variable name", name)
	}
	apiKeyEnv := strings.TrimSpace(options.APIKeyEnv)
	if !validProviderAPIKeyEnvName(apiKeyEnv) {
		return Provider{}, fmt.Errorf("provider %s api_key_env must be an environment variable name", name)
	}
	if options.TimeoutMS < 0 {
		return Provider{}, fmt.Errorf("provider %s timeout_ms must be non-negative", name)
	}
	if options.ContextWindowTokens < 0 {
		return Provider{}, fmt.Errorf("provider %s context_window_tokens must be non-negative", name)
	}
	capabilities := NormalizeProviderCapabilities(options.Capabilities)
	if len(capabilities) == 0 {
		capabilities = []string{"chat"}
	}
	return Provider{
		Name:                name,
		Type:                providerTypeOpenAICompatible,
		Model:               model,
		BaseURL:             baseURL,
		BaseURLEnv:          baseURLEnv,
		APIKeyEnv:           apiKeyEnv,
		TimeoutMS:           options.TimeoutMS,
		ContextWindowTokens: options.ContextWindowTokens,
		Capabilities:        capabilities,
		Enabled:             options.Enabled,
	}, nil
}

func upsertConfiguredProvider(existing []Provider, provider Provider) []Provider {
	providers := make([]Provider, 0, len(existing)+1)
	replaced := false
	if provider.Enabled {
		providers = append(providers, provider)
	}
	for _, current := range existing {
		if strings.TrimSpace(current.Name) == provider.Name {
			replaced = true
			if !provider.Enabled {
				providers = append(providers, provider)
			}
			continue
		}
		if provider.Enabled && isDefaultOfflineProvider(current) {
			current.Enabled = false
		}
		providers = append(providers, current)
	}
	if !replaced && !provider.Enabled {
		providers = append(providers, provider)
	}
	return providers
}

func isDefaultOfflineProvider(provider Provider) bool {
	return strings.TrimSpace(provider.Name) == "offline" &&
		strings.TrimSpace(provider.Type) == "offline" &&
		strings.TrimSpace(provider.Model) == "offline-model"
}

func validProviderAPIKeyEnvName(value string) bool {
	if value == "" {
		return false
	}
	for i, r := range value {
		if i == 0 {
			if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || r == '_' {
				continue
			}
			return false
		}
		if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' {
			continue
		}
		return false
	}
	return true
}

func writeProviderConfig(path string, config ProviderConfig) error {
	if info, err := os.Lstat(path); err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("providers.yaml must not be a symlink")
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat provider config: %w", err)
	}
	data, err := yaml.Marshal(config)
	if err != nil {
		return fmt.Errorf("encode provider config: %w", err)
	}
	dir := filepath.Dir(path)
	temp, err := os.CreateTemp(dir, ".providers-*.yaml")
	if err != nil {
		return fmt.Errorf("create provider config temp file: %w", err)
	}
	tempName := temp.Name()
	defer func() {
		_ = os.Remove(tempName)
	}()
	if _, err := temp.Write(data); err != nil {
		_ = temp.Close()
		return fmt.Errorf("write provider config temp file: %w", err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("close provider config temp file: %w", err)
	}
	if err := os.Rename(tempName, path); err != nil {
		return fmt.Errorf("write provider config: %w", err)
	}
	return nil
}
