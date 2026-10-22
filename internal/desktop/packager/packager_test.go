package packager

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeRunner records invocations and optionally fabricates the backend binary.
type fakeRunner struct {
	calls    []runnerCall
	binary   string // if set, emulate build.sh by writing this file (BIN target)
	code     int
	startErr error
}

type runnerCall struct {
	dir  string
	env  []string
	name string
	args []string
}

func (f *fakeRunner) Run(_ context.Context, dir string, env []string, name string, args ...string) (int, error) {
	f.calls = append(f.calls, runnerCall{dir, env, name, args})
	if f.startErr != nil {
		return ExitFailure, f.startErr
	}
	if f.code != 0 {
		return f.code, nil
	}
	for _, e := range env {
		if strings.HasPrefix(e, "BIN=") && f.binary != "" {
			if err := os.WriteFile(strings.TrimPrefix(e, "BIN="), []byte("binary"), 0o755); err != nil {
				return ExitFailure, err
			}
		}
	}
	return ExitOK, nil
}

func testOptions(t *testing.T) Options {
	t.Helper()
	gui := t.TempDir()
	return Options{
		GUIroot:  gui,
		RepoRoot: t.TempDir(),
		Platform: "linux",
		Arch:     "amd64",
		Env:      []string{"PATH="},
	}
}

func TestStepsBackendOnly(t *testing.T) {
	o := testOptions(t)
	steps := Steps(o)
	if len(steps) != 2 {
		t.Fatalf("backend-only mode must produce 2 steps, got %d: %v", len(steps), steps)
	}
	if !strings.HasPrefix(steps[0], "mkdir -p ") {
		t.Fatalf("step 0: %q", steps[0])
	}
	if !strings.HasPrefix(steps[1], "BIN=") || !strings.Contains(steps[1], "build.sh") {
		t.Fatalf("step 1: %q", steps[1])
	}
}

func TestStepsDirModeOrder(t *testing.T) {
	o := testOptions(t)
	o.DirMode = true
	steps := Steps(o)
	if len(steps) != 12 {
		t.Fatalf("dir mode must produce 12 steps, got %d: %v", len(steps), steps)
	}
	if !strings.Contains(steps[0], "run build") {
		t.Fatalf("step 0 should be the frontend build: %q", steps[0])
	}
	if !strings.HasPrefix(steps[3], "rm -rf ") {
		t.Fatalf("step 3: %q", steps[3])
	}
	if !strings.HasPrefix(steps[len(steps)-2], "write ") || !strings.Contains(steps[len(steps)-2], ".desktop") {
		t.Fatalf("step 10 should write the desktop entry: %q", steps[10])
	}
}

func TestRunDryRunTouchesNothing(t *testing.T) {
	o := testOptions(t)
	o.DryRun = true
	o.DirMode = true
	var out bytes.Buffer
	o.Stdout = &out
	code := Run(context.Background(), o)
	if code != ExitOK {
		t.Fatalf("dry-run exit code = %d", code)
	}
	if lines := strings.Count(strings.TrimSpace(out.String()), "\n") + 1; lines != 12 {
		t.Fatalf("expected 12 dry-run lines, got %d:\n%s", lines, out.String())
	}
	entries, err := os.ReadDir(o.GUIroot)
	if err != nil || len(entries) != 0 {
		t.Fatalf("dry-run must not write anything: %v %v", entries, err)
	}
}

func TestRunBackendMode(t *testing.T) {
	o := testOptions(t)
	r := &fakeRunner{binary: "yes"}
	o.Runner = r
	var out bytes.Buffer
	o.Stdout = &out
	code := Run(context.Background(), o)
	if code != ExitOK {
		t.Fatalf("exit code = %d", code)
	}
	if !strings.Contains(out.String(), "desktop backend ready:") {
		t.Fatalf("missing ready line: %q", out.String())
	}
	if len(r.calls) != 1 {
		t.Fatalf("expected exactly one build invocation, got %d", len(r.calls))
	}
	l := o.layout()
	info, err := os.Stat(l.ResourceBinary)
	if err != nil || info.Size() == 0 {
		t.Fatalf("backend binary missing: %v", err)
	}
}

func TestRunDirModeAssemblesPackage(t *testing.T) {
	o := testOptions(t)
	// Fake Electron runtime with an electron binary to rename.
	electron := filepath.Join(o.GUIroot, "node_modules", "electron", "dist")
	mustWrite(t, filepath.Join(electron, "electron"), "elf")
	// Fake frontend build outputs.
	mustWrite(t, filepath.Join(o.GUIroot, "dist", "renderer", "index.html"), "<html>")
	mustWrite(t, filepath.Join(o.GUIroot, "dist", "main", "main.js"), "js")

	o.DirMode = true
	o.Runner = &fakeRunner{binary: "yes"}
	var out bytes.Buffer
	o.Stdout = &out
	if code := Run(context.Background(), o); code != ExitOK {
		t.Fatalf("exit code = %d", code)
	}
	l := o.layout()
	for _, want := range []string{
		filepath.Join(l.DesktopDir, "Ouroboros"),
		filepath.Join(l.PackagedAppDir, "dist", "renderer", "index.html"),
		filepath.Join(l.PackagedAppDir, "dist", "main", "main.js"),
		filepath.Join(l.PackagedAppDir, "package.json"),
		l.PackagedBackend,
		l.LinuxEntryPath,
		l.LinuxInstaller,
	} {
		info, err := os.Stat(want)
		if err != nil || info.Size() == 0 {
			t.Errorf("packaged artifact missing: %s (%v)", want, err)
		}
	}
	if !strings.Contains(out.String(), "package:desktop:dir: "+l.DesktopDir) {
		t.Fatalf("missing final line: %q", out.String())
	}
	entry, err := os.ReadFile(l.LinuxEntryPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(entry), "Exec=/bin/sh -c ") || !strings.Contains(string(entry), "MimeType=x-scheme-handler/ouroboros;") {
		t.Errorf("desktop entry content wrong:\n%s", entry)
	}

	// A stale staged package must be replaced wholesale (rm -rf semantics).
	mustWrite(t, filepath.Join(l.PackagedAppDir, "package.json"), "stale-marker")
	if code := Run(context.Background(), o); code != ExitOK {
		t.Fatalf("second run exit code = %d", code)
	}
	if _, err := os.Stat(l.LinuxEntryPath); err != nil {
		t.Fatalf("entry must be rewritten on re-run: %v", err)
	}
}

func TestRunBackendBuildFailure(t *testing.T) {
	o := testOptions(t)
	o.Runner = &fakeRunner{code: 3}
	o.Stdout = &bytes.Buffer{}
	o.Stderr = &bytes.Buffer{}
	if code := Run(context.Background(), o); code != 3 {
		t.Fatalf("build failure must propagate exit code 3, got %d", code)
	}
}

func TestRunMissingBinaryIsContractFailure(t *testing.T) {
	o := testOptions(t)
	o.Runner = &fakeRunner{} // succeeds without producing the binary
	o.Stdout = &bytes.Buffer{}
	o.Stderr = &bytes.Buffer{}
	if code := Run(context.Background(), o); code != ExitFailure {
		t.Fatalf("missing binary must be exit 1, got %d", code)
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
