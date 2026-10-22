package productcontract

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestRunRealRepo runs the embedded driver against this repository.
// It is the smoke check for the port; skipped when node is absent.
func TestRunRealRepo(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node not on PATH")
	}
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	guiRoot := filepath.Join(cwd, "..", "..", "..", "gui")
	repoRoot := filepath.Dir(guiRoot)
	if err := Run(context.Background(), Options{GUIRoot: guiRoot, RepoRoot: repoRoot, Stdout: os.Stdout, Stderr: os.Stderr}); err != nil {
		t.Fatalf("product contract failed: %v (exit code %d)", err, ExitCode(err))
	}
}
