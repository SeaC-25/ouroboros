package app

import (
	"os"
	"testing"
)

// TestMain keeps the suite hermetic: provider preflight is disabled by default
// so a DNS or upstream outage on a third-party provider endpoint cannot turn
// unrelated tests red. Tests that exercise the preflight itself re-enable it
// with t.Setenv(ProviderPreflightEnvVar, "on").
func TestMain(m *testing.M) {
	if _, set := os.LookupEnv(ProviderPreflightEnvVar); !set {
		_ = os.Setenv(ProviderPreflightEnvVar, "off")
	}
	os.Exit(m.Run())
}
