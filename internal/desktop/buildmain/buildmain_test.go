package buildmain

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// seedGUI creates a minimal guiRoot with src/main and electron trees that
// satisfy the required output list.
func seedGUI(t *testing.T) (guiRoot string, stale string) {
	t.Helper()
	guiRoot = t.TempDir()
	srcMain := map[string]string{
		"main.js":          "src-main",
		"backend.js":       "src-backend",
		"desktop-state.js": "src-state",
		"preload.js":       "src-preload",
		"workspace.js":     "src-workspace",
	}
	packaged := map[string]string{
		"main.js":                   "packaged-main",
		"backend.js":                "packaged-backend",
		"memory-profile.js":         "packaged-memory",
		"assets/ouroboros-icon.png": "icon",
		"backend-stop.test.mjs":     "test", // carried along like cpSync does
	}
	for rel, body := range srcMain {
		write(t, filepath.Join(guiRoot, "src", "main", filepath.FromSlash(rel)), body)
	}
	for rel, body := range packaged {
		write(t, filepath.Join(guiRoot, "electron", filepath.FromSlash(rel)), body)
	}
	stale = filepath.Join(guiRoot, "dist", "main", "stale.js")
	write(t, stale, "stale")
	return guiRoot, stale
}

func write(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}

func TestBuildAssemblesBundle(t *testing.T) {
	guiRoot, stale := seedGUI(t)
	var out bytes.Buffer
	if err := BuildWithEnv(guiRoot, Options{}, func(string) string { return "" }, &out); err != nil {
		t.Fatalf("Build: %v", err)
	}
	dest := filepath.Join(guiRoot, "dist", "main")

	// Renames happened with original content preserved.
	if got := read(t, filepath.Join(dest, "application-main.js")); got != "src-main" {
		t.Fatalf("application-main.js = %q", got)
	}
	if got := read(t, filepath.Join(dest, "base-backend.js")); got != "src-backend" {
		t.Fatalf("base-backend.js = %q", got)
	}
	// Packaged overlay shadows main.js/backend.js.
	if got := read(t, filepath.Join(dest, "main.js")); got != "packaged-main" {
		t.Fatalf("main.js = %q", got)
	}
	if got := read(t, filepath.Join(dest, "backend.js")); got != "packaged-backend" {
		t.Fatalf("backend.js = %q", got)
	}
	// Stale output directory was removed first (rmSync recursive force).
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Fatalf("stale file still present: %v", err)
	}
	// Required assets present.
	for _, rel := range requiredOutputs {
		if _, err := os.Stat(filepath.Join(dest, filepath.FromSlash(rel))); err != nil {
			t.Fatalf("missing %s: %v", rel, err)
		}
	}
	wantLine := "desktop main ready: " + dest + "\n"
	if out.String() != wantLine {
		t.Fatalf("stdout = %q, want %q", out.String(), wantLine)
	}
}

func TestBuildHonoursEnvOutput(t *testing.T) {
	guiRoot, _ := seedGUI(t)
	custom := filepath.Join(t.TempDir(), "custom-out")
	var out bytes.Buffer
	err := BuildWithEnv(guiRoot, Options{}, func(k string) string {
		if k == OutputEnvVar {
			return custom
		}
		return ""
	}, &out)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if _, err := os.Stat(filepath.Join(custom, "application-main.js")); err != nil {
		t.Fatalf("custom output missing files: %v", err)
	}
}

func TestBuildMissingSourceMainFailsWithPath(t *testing.T) {
	guiRoot, _ := seedGUI(t)
	if err := os.Remove(filepath.Join(guiRoot, "src", "main", "main.js")); err != nil {
		t.Fatal(err)
	}
	err := BuildWithEnv(guiRoot, Options{}, func(string) string { return "" }, nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "desktop main source missing: "+filepath.Join(guiRoot, "dist", "main", "main.js")) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestBuildMissingRequiredOutputFails(t *testing.T) {
	guiRoot, _ := seedGUI(t)
	if err := os.Remove(filepath.Join(guiRoot, "electron", "memory-profile.js")); err != nil {
		t.Fatal(err)
	}
	err := BuildWithEnv(guiRoot, Options{}, func(string) string { return "" }, nil)
	if err == nil || !strings.Contains(err.Error(), "desktop main build missing memory-profile.js") {
		t.Fatalf("unexpected error: %v", err)
	}
}
