package app

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestConfigureOpenAICompatibleProviderWritesMetadataOnlyProviderConfig(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	t.Setenv("OURO_TEST_OPENAI_KEY", "real-secret-value")

	result, err := ConfigureOpenAICompatibleProvider(root, OpenAICompatibleProviderConfiguration{
		Name:                " local-openai ",
		BaseURL:             " https://api.example.test/v1 ",
		Model:               " gpt-test ",
		APIKeyEnv:           " OURO_TEST_OPENAI_KEY ",
		Enabled:             true,
		TimeoutMS:           30000,
		ContextWindowTokens: 128000,
		Capabilities:        []string{"chat", "tool_planning", "chat"},
		RoutingStrategy:     "enabled_order_failover",
	})
	if err != nil {
		t.Fatalf("configure provider: %v", err)
	}

	providersPath := filepath.Join(root, "providers.yaml")
	if result.Path != providersPath || result.APIKeyValueStored {
		t.Fatalf("unexpected configure result: %#v", result)
	}
	if result.Provider.Name != "local-openai" || result.Provider.Type != "openai_compatible" || !result.Provider.Enabled {
		t.Fatalf("unexpected configured provider result: %#v", result.Provider)
	}

	data, err := os.ReadFile(providersPath)
	if err != nil {
		t.Fatalf("read providers: %v", err)
	}
	if strings.Contains(string(data), "real-secret-value") {
		t.Fatalf("providers.yaml must not store api key value: %s", string(data))
	}

	config, err := LoadProviderConfig(providersPath)
	if err != nil {
		t.Fatalf("load providers: %v", err)
	}
	if err := ValidateProviderConfig(config); err != nil {
		t.Fatalf("validate providers: %v", err)
	}
	if config.Routing.Strategy != "enabled_order_failover" {
		t.Fatalf("unexpected routing strategy: %#v", config.Routing)
	}
	if len(config.Providers) != 2 {
		t.Fatalf("expected configured provider plus default offline provider, got %#v", config.Providers)
	}
	provider := config.Providers[0]
	if provider.Name != "local-openai" || provider.Type != "openai_compatible" || provider.Model != "gpt-test" || provider.BaseURL != "https://api.example.test/v1" || provider.APIKeyEnv != "OURO_TEST_OPENAI_KEY" || !provider.Enabled {
		t.Fatalf("unexpected configured provider: %#v", provider)
	}
	if provider.TimeoutMS != 30000 || provider.ContextWindowTokens != 128000 {
		t.Fatalf("unexpected provider runtime metadata: %#v", provider)
	}
	if !reflect.DeepEqual(provider.Capabilities, []string{"chat", "tool_planning"}) {
		t.Fatalf("unexpected provider capabilities: %#v", provider.Capabilities)
	}
	if config.Providers[1].Name != "offline" || config.Providers[1].Enabled {
		t.Fatalf("default offline provider should be disabled once a real provider is enabled, got %#v", config.Providers[1])
	}
}

func TestConfigureOpenAICompatibleProviderRejectsRawAPIKeyValue(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}

	_, err := ConfigureOpenAICompatibleProvider(root, OpenAICompatibleProviderConfiguration{
		Name:      "local-openai",
		BaseURL:   "https://api.example.test/v1",
		Model:     "gpt-test",
		APIKeyEnv: "sk-live-secret-value",
		Enabled:   true,
	})
	if err == nil || !strings.Contains(err.Error(), "api_key_env must be an environment variable name") {
		t.Fatalf("expected api_key_env name validation error, got %v", err)
	}

	data, readErr := os.ReadFile(filepath.Join(root, "providers.yaml"))
	if readErr != nil {
		t.Fatalf("read providers: %v", readErr)
	}
	if strings.Contains(string(data), "sk-live-secret-value") {
		t.Fatalf("raw api key must not be written to providers.yaml: %s", string(data))
	}
}
