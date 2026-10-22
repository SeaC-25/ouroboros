package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCreateToolchainInstallOrchestrationExecutesWithoutCompatibilityGates(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	skipIfWindows(t)
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}

	marker := filepath.Join(t.TempDir(), "install-ran")
	writeInstallRequestFixture(t, root, ToolchainInstallRequest{
		Status:     "denied",
		Pack:       "web-api-pack",
		Dependency: "missing-httpx",
		Command:    []string{"sh", "-c", "printf executed > " + marker},
	})
	if err := os.Remove(filepath.Join(root, "scope.yaml")); err != nil {
		t.Fatalf("remove compatibility scope: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "authorization.yaml"), []byte("version: [\n"), 0o644); err != nil {
		t.Fatalf("write invalid compatibility authorization: %v", err)
	}

	orchestration, err := CreateToolchainInstallOrchestration(root, ToolchainInstallOrchestrationOptions{})
	if err != nil {
		t.Fatalf("create production install orchestration: %v", err)
	}
	if orchestration.Status != "completed" || orchestration.Mode != "execute" || orchestration.ExecutionResults != 1 {
		t.Fatalf("expected real install execution, got %#v", orchestration)
	}
	data, err := os.ReadFile(marker)
	if err != nil {
		t.Fatalf("read install marker: %v", err)
	}
	if strings.TrimSpace(string(data)) != "executed" {
		t.Fatalf("unexpected install marker: %q", data)
	}
}
