package toolchain

import (
	"os"
	"testing"
)

// TestNativeGoSelectionIsOptIn pins the determinism contract: without the
// selector every built-in tool keeps the managed_binary adapter, so runs do not
// silently change behaviour depending on which Go handlers are compiled in.
func TestNativeGoSelectionIsOptIn(t *testing.T) {
	previous, had := os.LookupEnv(NativeToolsEnvVar)
	defer func() {
		if had {
			_ = os.Setenv(NativeToolsEnvVar, previous)
			return
		}
		_ = os.Unsetenv(NativeToolsEnvVar)
	}()

	_ = os.Unsetenv(NativeToolsEnvVar)
	for _, tool := range BuiltInAppSecTools() {
		if tool.DependencyKind != "binary" {
			continue // rule bundles carry no adapter
		}
		if tool.AdapterID != "managed_binary" {
			t.Fatalf("tool %s defaulted to adapter %s, want managed_binary", tool.Name, tool.AdapterID)
		}
	}
	_ = os.Setenv(NativeToolsEnvVar, "httpx,gitleaks")
	adapters := map[string]string{}
	for _, tool := range BuiltInAppSecTools() {
		adapters[tool.Name] = tool.AdapterID
	}
	if adapters["httpx"] != "native_go" || adapters["gitleaks"] != "native_go" {
		t.Fatalf("explicitly selected tools were not switched: %#v", adapters)
	}
	if adapters["ffuf"] != "managed_binary" {
		t.Fatalf("unselected tool ffuf changed adapter: %#v", adapters)
	}

	_ = os.Setenv(NativeToolsEnvVar, "all")
	for _, tool := range BuiltInAppSecTools() {
		if !nativeGoToolNames[tool.Name] {
			continue
		}
		if tool.AdapterID != "native_go" {
			t.Fatalf("tool %s not switched under all: %s", tool.Name, tool.AdapterID)
		}
	}
}

// TestNativeGoToolsHaveNativeHandlers guards the list against drift: every tool
// advertised as native must be backed by an operation the native adapter
// exposes.
func TestNativeGoToolsHaveNativeHandlers(t *testing.T) {
	opportunities := map[string]bool{}
	for _, operation := range DefaultAuthoredToolRegistry().NativeGoOperations() {
		opportunities[operation] = true
	}
	if len(opportunities) == 0 {
		t.Fatal("native_go adapter exposes no operations")
	}

	_ = os.Setenv(NativeToolsEnvVar, "all")
	defer func() { _ = os.Unsetenv(NativeToolsEnvVar) }()
	for _, tool := range BuiltInAppSecTools() {
		if tool.AdapterID != "native_go" {
			continue
		}
		if !opportunities[tool.Operation] {
			t.Fatalf("tool %s is native but operation %s has no native handler", tool.Name, tool.Operation)
		}
	}
}
