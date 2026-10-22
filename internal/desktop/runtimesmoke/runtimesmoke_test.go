package runtimesmoke

import (
	"bytes"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

type fakeOptions struct {
	files   map[string]bool
	env     map[string]string
	result  RunResult
	runArgs []string
	runEnv  []string
	runDir  string
}

func (f *fakeOptions) options(mode string, stderr *bytes.Buffer) Options {
	return Options{
		RepoRoot: "C:/repo",
		Mode:     mode,
		Env:      []string{"KEEP=1"},
		Getenv:   func(k string) string { return f.env[k] },
		FileExists: func(p string) bool {
			if f.files == nil {
				return true
			}
			return f.files[p]
		},
		Run: func(cmd *exec.Cmd) RunResult {
			f.runArgs = cmd.Args
			f.runDir = cmd.Dir
			f.runEnv = cmd.Env
			return f.result
		},
		CleanupWindows: func(_, _ string) {},
		Stderr:         stderr,
	}
}

func envHas(env []string, kv string) bool {
	for _, e := range env {
		if e == kv {
			return true
		}
	}
	return false
}

func TestMissingSmokeScript(t *testing.T) {
	f := &fakeOptions{files: map[string]bool{}}
	var stderr bytes.Buffer
	if code := Execute(f.options("", &stderr)); code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	out := stderr.String()
	wantPath := filepath.Join("C:/repo", "scripts", "smoke_desktop_package_runtime.sh")
	if !strings.Contains(out, "desktop runtime smoke script missing: "+wantPath) {
		t.Fatalf("stderr = %q", out)
	}
}

func TestMissingWebSocketModule(t *testing.T) {
	f := &fakeOptions{files: map[string]bool{filepath.Join("C:/repo", "scripts", "smoke_desktop_package_runtime.sh"): true}}
	var stderr bytes.Buffer
	if code := Execute(f.options("", &stderr)); code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	wantPath := filepath.Join("C:/repo", "gui", "node_modules", "ws", "index.js")
	if !strings.Contains(stderr.String(), "desktop runtime smoke WebSocket module missing: "+wantPath) {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestRunEnvAndArgs(t *testing.T) {
	f := &fakeOptions{}
	var stderr bytes.Buffer
	if code := Execute(f.options("quick", &stderr)); code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if f.runDir != "C:/repo" {
		t.Fatalf("cwd = %q", f.runDir)
	}
	if len(f.runArgs) != 2 || f.runArgs[1] != filepath.Join("C:/repo", "scripts", "smoke_desktop_package_runtime.sh") {
		t.Fatalf("args = %v", f.runArgs)
	}
	if !envHas(f.runEnv, "KEEP=1") {
		t.Fatalf("inherited env lost: %v", f.runEnv)
	}
	if !envHas(f.runEnv, envWSModule+"="+filepath.Join("C:/repo", "gui", "node_modules", "ws", "index.js")) {
		t.Fatalf("ws module env missing: %v", f.runEnv)
	}
	if !envHas(f.runEnv, envSmokeMode+"=quick") {
		t.Fatalf("mode env missing: %v", f.runEnv)
	}
}

func TestEmptyModeOmitsModeEnv(t *testing.T) {
	f := &fakeOptions{}
	var stderr bytes.Buffer
	if code := Execute(f.options("  ", &stderr)); code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	for _, e := range f.runEnv {
		if strings.HasPrefix(e, envSmokeMode+"=") {
			t.Fatalf("unexpected mode env: %v", f.runEnv)
		}
	}
}

func TestChildExitCodeForwarded(t *testing.T) {
	f := &fakeOptions{result: RunResult{ExitCode: 42}}
	var stderr bytes.Buffer
	if code := Execute(f.options("", &stderr)); code != 42 {
		t.Fatalf("exit code = %d, want 42", code)
	}
}

func TestSignalStopsRun(t *testing.T) {
	f := &fakeOptions{result: RunResult{Signaled: true}}
	var stderr bytes.Buffer
	if code := Execute(f.options("", &stderr)); code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "desktop runtime smoke stopped by signal") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestStartFailure(t *testing.T) {
	f := &fakeOptions{result: RunResult{StartErr: errors.New("executable file not found")}}
	var stderr bytes.Buffer
	if code := Execute(f.options("", &stderr)); code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "desktop runtime smoke could not start") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestIsDigits(t *testing.T) {
	for _, ok := range []string{"19224", "0"} {
		if !isDigits(ok) {
			t.Fatalf("isDigits(%q) = false", ok)
		}
	}
	for _, bad := range []string{"", "12a", " 80"} {
		if isDigits(bad) {
			t.Fatalf("isDigits(%q) = true", bad)
		}
	}
}

func TestCleanupParamResolution(t *testing.T) {
	// Exercise cleanupSmokeProcesses indirectly through fake cleanup capture on
	// platforms where it is a no-op is impossible; validate env resolution
	// helpers instead.
	if got := resolvePort(map[string]string{}); got != defaultDebugPort {
		t.Fatalf("default port = %q", got)
	}
	if got := resolvePort(map[string]string{envDebugPort: "  18080 "}); got != "18080" {
		t.Fatalf("env port = %q", got)
	}
	if got := resolvePackageRoot(map[string]string{}); got != defaultPackageDirName {
		t.Fatalf("default root = %q", got)
	}
	if got := resolvePackageRoot(map[string]string{envPackageDir: "gui/dist/desktop/Custom-win32-arm64"}); got != "Custom-win32-arm64" {
		t.Fatalf("base root = %q", got)
	}
}

func TestCobraCommandModeValidation(t *testing.T) {
	cmd := NewCommand()
	if err := cmd.Args(cmd, []string{"bogus"}); err == nil {
		t.Fatal("expected invalid mode error")
	}
	for _, mode := range ValidModes {
		if err := cmd.Args(cmd, []string{mode}); err != nil {
			t.Fatalf("mode %s rejected: %v", mode, err)
		}
	}
	if err := cmd.Args(cmd, []string{"quick", "extra"}); err == nil {
		t.Fatal("expected arity error")
	}
}

func TestExitCodeError(t *testing.T) {
	var err error = exitCodeError{code: 7}
	ec, ok := err.(ExitCodeError)
	if !ok || ec.ExitCode() != 7 {
		t.Fatalf("ExitCodeError mismatch: %v", err)
	}
	if !strings.Contains(fmt.Sprint(err), "7") {
		t.Fatalf("message = %q", err)
	}
}
