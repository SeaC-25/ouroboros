package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestWorkspaceRunIsHermeticWhenProviderUnreachable proves the suite no longer
// depends on an external provider endpoint: with preflight disabled (the suite
// default) a run against a workspace whose provider host cannot resolve still
// completes, while the same run with preflight on fails fast. This is the
// regression guard for the "third-party DNS outage turns unrelated tests red"
// failure mode.
func TestWorkspaceRunIsHermeticWhenProviderUnreachable(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	config := `version: 1
providers:
  - name: unreachable-openai
    type: openai_compatible
    model: gpt-unreachable
    base_url: https://ouroboros-provider-does-not-exist.invalid/v1
    timeout_ms: 1000
    enabled: true
`
	if err := os.WriteFile(filepath.Join(root, "providers.yaml"), []byte(config), 0o644); err != nil {
		t.Fatalf("write providers: %v", err)
	}

	t.Setenv(ProviderPreflightEnvVar, "off")
	if _, err := RunWorkspaceWithOptions(context.Background(), root, RunOptions{DeferReportProjection: true}); err != nil {
		t.Fatalf("run with preflight off must not depend on the provider endpoint: %v", err)
	}

	t.Setenv(ProviderPreflightEnvVar, "on")
	_, err := RunWorkspaceWithOptions(context.Background(), root, RunOptions{DeferReportProjection: true})
	if err == nil {
		t.Fatal("run with preflight on must fail when the provider host cannot be resolved")
	}
	if !strings.Contains(err.Error(), "preflight failed") {
		t.Fatalf("expected a preflight failure, got %v", err)
	}
}

// TestProviderPreflightDefaultIsOn pins the production default: operators get
// the fail-fast network check unless they explicitly disable it.
func TestProviderPreflightDefaultIsOn(t *testing.T) {
	t.Setenv(ProviderPreflightEnvVar, "")
	if providerPreflightDisabled() {
		t.Fatal("unset provider preflight env must keep the network check enabled")
	}
}
