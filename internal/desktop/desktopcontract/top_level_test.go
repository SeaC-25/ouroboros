package desktopcontract

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fixture builds a minimal gui/repo tree so checkTopLevel can be exercised
// without the real gui sources.
func fixture(t *testing.T) (gui, repo string) {
	t.Helper()
	repo = t.TempDir()
	gui = filepath.Join(repo, "gui")
	for _, dir := range []string{
		filepath.Join(gui, "src", "main"),
		filepath.Join(gui, "src", "app"),
		filepath.Join(gui, "electron", "assets"),
		filepath.Join(gui, "scripts"),
		filepath.Join(repo, "scripts"),
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	write := func(rel, content string) {
		path := filepath.Join(gui, rel)
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, rel := range []string{
		"src/main/main.js", "src/main/preload.js", "src/main/backend.js",
		"src/main/workspace.js", "src/main/desktop-state.js", "electron/main.js",
		"electron/memory-profile.js", "electron/backend.js", "electron/assets/ouroboros-icon.png",
		"scripts/build-desktop-main.mjs", "scripts/package-desktop.mjs",
		"scripts/run-desktop-runtime-smoke.mjs",
	} {
		write(rel, "")
	}
	if err := os.WriteFile(filepath.Join(gui, "package.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	// preload must satisfy the CommonJS requirement to keep the fixture focused
	write("src/main/preload.js", "const { contextBridge } = require('electron')")
	return gui, repo
}

func runCheck(t *testing.T, gui, repo string) (int, string) {
	t.Helper()
	var buf bytes.Buffer
	code := Check(Options{GUIRoot: gui, RepoRoot: repo, Stderr: &buf})
	return code, buf.String()
}

func TestCheckTopLevelReportsMissingDesktopFile(t *testing.T) {
	gui, repo := fixture(t)
	if err := os.Remove(filepath.Join(gui, "electron", "main.js")); err != nil {
		t.Fatal(err)
	}
	code, out := runCheck(t, gui, repo)
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(out, "missing desktop file: electron/main.js") {
		t.Fatalf("missing-file violation absent, got:\n%s", out)
	}
}

func TestCheckTopLevelFlagsForbiddenNodeOptions(t *testing.T) {
	gui, repo := fixture(t)
	write := func(rel, content string) {
		if err := os.WriteFile(filepath.Join(gui, rel), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("electron/main.js", "NODE_OPTIONS=--max-old-space-size=4096")
	code, out := runCheck(t, gui, repo)
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(out, "must not use Electron-restricted NODE_OPTIONS") {
		t.Fatalf("forbidden NODE_OPTIONS violation absent, got:\n%s", out)
	}
}

func TestCheckTopLevelIgnoresUnrelatedForbiddenNeedles(t *testing.T) {
	gui, repo := fixture(t)
	if err := os.WriteFile(filepath.Join(gui, "electron", "main.js"), []byte("const relaunch = () => {}"), 0o644); err != nil {
		t.Fatal(err)
	}
	code, out := runCheck(t, gui, repo)
	// Everything else in the empty fixture fails, but the NODE_OPTIONS rule
	// must not fire on unrelated content.
	if strings.Contains(out, "NODE_OPTIONS") {
		t.Fatalf("NODE_OPTIONS rule fired without the marker, got:\n%s", out)
	}
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
}

func TestPropagatesEnvErrorFromMissingNode(t *testing.T) {
	// A driver invoked with a missing node binary must report an environment
	// error (exit code 2) rather than a contract violation.
	var buf bytes.Buffer
	r := &runner{
		opts:   Options{Node: filepath.Join(t.TempDir(), "no-such-node"), Stderr: &buf},
		read:   func(string) string { return "" },
		readRP: func(string) string { return "" },
	}
	r.runNodeContract(dynamicMemoryContractDriver, "")
	if !r.envErr {
		t.Fatalf("expected envErr, output:\n%s", buf.String())
	}
	if r.failed {
		t.Fatalf("spawn failure must not be reported as a contract violation")
	}
}
