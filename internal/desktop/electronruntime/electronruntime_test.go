package electronruntime

import (
	"bytes"
	"context"
	_ "embed"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// moduleDir is the package directory captured at process start; tests that
// chdir still build the fakenode helper from the real source tree.
var moduleDir = func() string {
	dir, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	return dir
}()

// buildFakeNode compiles the testdata fakenode helper and returns its path.
func buildFakeNode(t *testing.T) string {
	t.Helper()
	out := filepath.Join(t.TempDir(), "fakenode.exe")
	source := filepath.Join(moduleDir, "testdata", "fakenode")
	build := exec.Command("go", "build", "-o", out, source)
	build.Dir = moduleDir
	if combined, err := build.CombinedOutput(); err != nil {
		t.Fatalf("building fakenode: %v\n%s", err, combined)
	}
	return out
}

// stubGUI creates a directory shaped like the gui tree for orchestration tests.
func stubGUI(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	gui := filepath.Join(dir, "gui")
	if err := os.MkdirAll(filepath.Join(gui, "src", "renderer"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(gui, "package.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	return gui
}

func TestRunSuccessStreamsOKLine(t *testing.T) {
	gui := stubGUI(t)
	var stdout, stderr bytes.Buffer
	t.Setenv("FAKENODE_MUSTCONTAIN", "checkPackagedBootstrapRelaunch")
	err := Run(context.Background(), Options{
		GUIRoot: gui,
		Node:    buildFakeNode(t),
		Stdout:  &stdout,
		Stderr:  &stderr,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := strings.TrimSpace(stdout.String()); got != "electron runtime contract: ok" {
		t.Fatalf("stdout = %q, want %q", got, "electron runtime contract: ok")
	}
	// The temporary driver directory under the gui root must be cleaned up.
	entries, readErr := os.ReadDir(gui)
	if readErr != nil {
		t.Fatal(readErr)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".electron-runtime-check-") {
			t.Fatalf("temporary driver directory %q was not removed", entry.Name())
		}
	}
}

func TestRunContractFailureExitCodeOne(t *testing.T) {
	gui := stubGUI(t)
	t.Setenv("FAKENODE_EXIT", "1")
	t.Setenv("FAKENODE_STDERR", "AssertionError")
	var stdout, stderr bytes.Buffer
	err := Run(context.Background(), Options{
		GUIRoot: gui,
		Node:    buildFakeNode(t),
		Stdout:  &stdout,
		Stderr:  &stderr,
	})
	var contractErr *ContractError
	if !errors.As(err, &contractErr) {
		t.Fatalf("error = %v (%T), want *ContractError", err, err)
	}
	if got := ExitCode(err); got != 1 {
		t.Fatalf("ExitCode = %d, want 1", got)
	}
	if !strings.Contains(stderr.String(), "AssertionError") {
		t.Fatalf("stderr %q lost driver failure output", stderr.String())
	}
}

func TestRunMissingNodeIsEnvError(t *testing.T) {
	gui := stubGUI(t)
	err := Run(context.Background(), Options{
		GUIRoot: gui,
		Node:    filepath.Join(t.TempDir(), "does-not-exist-node.exe"),
		Stdout:  &bytes.Buffer{},
		Stderr:  &bytes.Buffer{},
	})
	var envErr *EnvError
	if !errors.As(err, &envErr) {
		t.Fatalf("error = %v (%T), want *EnvError", err, err)
	}
	if got := ExitCode(err); got != 2 {
		t.Fatalf("ExitCode = %d, want 2", got)
	}
}

func TestRunGUIRootDetectionAndUnresolved(t *testing.T) {
	// Detection finds the gui tree by walking up from a nested cwd.
	gui := stubGUI(t)
	nested := filepath.Join(gui, "src", "deep")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(nested); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(previous) })

	if err := Run(context.Background(), Options{
		Node:   buildFakeNode(t),
		Stdout: &bytes.Buffer{},
		Stderr: &bytes.Buffer{},
	}); err != nil {
		t.Fatalf("Run with detected GUIRoot: %v", err)
	}
}

func TestRunGUIRootNotFoundIsEnvError(t *testing.T) {
	empty := t.TempDir()
	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(empty); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(previous) })

	runErr := Run(context.Background(), Options{Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}})
	var envErr *EnvError
	if !errors.As(runErr, &envErr) {
		t.Fatalf("error = %v (%T), want *EnvError", runErr, runErr)
	}
	if !strings.Contains(envErr.Path, "package.json") {
		t.Fatalf("EnvError.Path = %q, want it to name the missing anchor", envErr.Path)
	}
	if got := ExitCode(runErr); got != 2 {
		t.Fatalf("ExitCode = %d, want 2", got)
	}
}

func TestExitCodeNilIsZero(t *testing.T) {
	if got := ExitCode(nil); got != 0 {
		t.Fatalf("ExitCode(nil) = %d, want 0", got)
	}
}

// TestEmbeddedDriverUsesGoDesktopToolchain guards the post-cutover contract:
// the driver must obtain the packaged main bundle from the Go toolchain
// (cmd/desktop build-main) and must not depend on the removed
// scripts/build-desktop-main.mjs.
func TestEmbeddedDriverUsesGoDesktopToolchain(t *testing.T) {
	source := string(driverSource)
	if !strings.Contains(source, "'./cmd/desktop', 'build-main'") {
		t.Fatal("embedded driver must invoke the Go desktop toolchain build-main command")
	}
	if strings.Contains(source, "build-desktop-main.mjs") {
		t.Fatal("embedded driver must not reference the removed build-desktop-main.mjs")
	}
	if !strings.Contains(source, "OURO_DESKTOP_MAIN_OUTPUT") {
		t.Fatal("embedded driver must keep the OURO_DESKTOP_MAIN_OUTPUT override")
	}
}

//go:embed testdata/backend-stop.test.mjs
var backendStopDriver []byte

// TestPackagedBackendStopContract runs the relocated backend-stop contract
// (原 gui/electron/backend-stop.test.mjs) with node --test. The driver must be
// materialised directly inside gui/electron because it imports
// '../src/main/backend.js' relative to its own location.
func TestPackagedBackendStopContract(t *testing.T) {
	guiRoot, err := findGUIRoot()
	if err != nil {
		t.Skipf("gui root not available: %v", err)
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not available")
	}
	driverPath := filepath.Join(guiRoot, "electron", "backend-stop-contract.test.mjs")
	if err := os.WriteFile(driverPath, backendStopDriver, 0o644); err != nil {
		t.Fatalf("materialising driver: %v", err)
	}
	defer func() { _ = os.Remove(driverPath) }()

	cmd := exec.Command(node, "--test", driverPath)
	cmd.Dir = guiRoot
	cmd.Env = os.Environ()
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("backend-stop contract failed: %v\n%s", err, out)
	}
}
