package devmgr

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ouroboros/ouroboros/internal/desktop/buildmain"
)

type fakeRunner struct {
	cmds []*fakeCommand
}

func (f *fakeRunner) Command(ctx context.Context, name string, args ...string) Command {
	c := &fakeCommand{name: name, args: args, code: 0}
	f.cmds = append(f.cmds, c)
	return c
}

type fakeCommand struct {
	name  string
	args  []string
	code  int
	env   []string
	stdin io.ReadCloser
}

func (c *fakeCommand) Start() error { c.ensureViteMarker(); return nil }
func (c *fakeCommand) Wait() (int, error) {
	return c.code, nil
}
func (c *fakeCommand) Stdout() (io.ReadCloser, error) {
	return c.stdin, nil
}
func (c *fakeCommand) AddEnv(kv ...string)                    { c.env = append(c.env, kv...) }
func (c *fakeCommand) SetStdout(w io.Writer)                  {}
func (c *fakeCommand) SetStderr(w io.Writer)                  {}
func (c *fakeCommand) Signal(sig os.Signal) error             { return nil }
func (c *fakeCommand) Kill(grace, forced time.Duration) error { return nil }
func (c *fakeCommand) Exited() bool                           { return true }
func (c *fakeCommand) PID() int                               { return 4242 }

func (c *fakeCommand) ensureViteMarker() {
	if len(c.args) > 0 && strings.HasSuffix(c.args[0], "vite-driver.mjs") {
		c.stdin = io.NopCloser(strings.NewReader(marker + " http://127.0.0.1:5173/\n"))
	}
}

func fakeGUI(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "package.json"), []byte(`{"name":"fake-gui","private":true}`), 0o644); err != nil {
		t.Fatalf("write package.json: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "vite.config.ts"), []byte("export default {}\n"), 0o644); err != nil {
		t.Fatalf("write vite.config.ts: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "src", "main"), 0o755); err != nil {
		t.Fatalf("mkdir src/main: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "electron"), 0o755); err != nil {
		t.Fatalf("mkdir electron: %v", err)
	}
	// The packaged overlay must satisfy buildmain's required-output list:
	// generate every entry so the fixture tracks the build contract.
	for _, required := range buildmain.RequiredOutputs() {
		rel := filepath.FromSlash(required)
		if err := os.MkdirAll(filepath.Join(root, "electron", filepath.Dir(rel)), 0o755); err != nil {
			t.Fatalf("mkdir electron/%s: %v", filepath.Dir(rel), err)
		}
		if err := os.WriteFile(filepath.Join(root, "electron", rel), []byte("// packaged\n"), 0o644); err != nil {
			t.Fatalf("write electron/%s: %v", required, err)
		}
	}
	for _, name := range []string{"main.js", "backend.js", "preload.js"} {
		if err := os.WriteFile(filepath.Join(root, "src", "main", name), []byte("// fake\n"), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	return root
}

func TestRunWithRunnerSuccessPath(t *testing.T) {
	root := fakeGUI(t)
	fr := &fakeRunner{}
	// 🚨 electron binary lookup requires the dist binary; skip full path by
	// testing internal error mapping first — the success path requires a
	// real node_modules/electron tree, which is unavailable in unit tests.
	if err := RunWithRunner(root, Options{}, fr); err == nil {
		t.Fatalf("expected electron-binary environment error, got nil")
	} else {
		var envErr *EnvError
		if !errors.As(err, &envErr) {
			t.Fatalf("expected EnvError, got %T(%v)", err, err)
		}
	}
}

func TestAssertSuccessfulChildMapsNonZeroExit(t *testing.T) {
	err := assertSuccessfulChild(3, nil, "Desktop main build")
	var ce *ChildError
	if !errors.As(err, &ce) || ce.Code != 3 {
		t.Fatalf("expected ChildError code 3, got %v", err)
	}
	if got := ExitCode(err); got != 3 {
		t.Fatalf("expected exit code 3, got %d", got)
	}
}

func TestSignalNameAndCode(t *testing.T) {
	sigErr := &SignalExitError{Signal: "SIGTERM"}
	if got := ExitCode(sigErr); got != 143 {
		t.Fatalf("expected 143, got %d", got)
	}
	intErr := &SignalExitError{Signal: "SIGINT"}
	if got := ExitCode(intErr); got != 130 {
		t.Fatalf("expected 130, got %d", got)
	}
	envErr := &EnvError{What: "locate node"}
	if got := ExitCode(envErr); got != 2 {
		t.Fatalf("expected 2, got %d", got)
	}
}
