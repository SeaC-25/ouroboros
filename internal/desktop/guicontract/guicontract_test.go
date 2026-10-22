package guicontract

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// fake gui tree: the orchestrator must create the driver inside guiRoot and
// invoke node with it. A fake node binary (a shell script on unix; batch on
// windows) would be cumbersome, so we assert on the pieces that don't spawn
// node: root detection, error mapping, and EnvError when node is missing.

func TestExitCodeMapping(t *testing.T) {
	if got := ExitCode(nil); got != 0 {
		t.Fatalf("nil → %d", got)
	}
	if got := ExitCode(&ContractError{ExitCode: 1}); got != 1 {
		t.Fatalf("contract → %d", got)
	}
	if got := ExitCode(&EnvError{Err: errors.New("boom")}); got != 2 {
		t.Fatalf("env → %d", got)
	}
}

func TestFindGUIRoot(t *testing.T) {
	tmp := t.TempDir()
	gui := filepath.Join(tmp, "gui")
	if err := os.MkdirAll(filepath.Join(gui, "src", "renderer"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(gui, "package.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	cwd, _ := os.Getwd()
	defer func() { _ = os.Chdir(cwd) }()
	_ = os.Chdir(tmp)

	found, err := findGUIRoot()
	if err != nil {
		t.Fatalf("findGUIRoot: %v", err)
	}
	if found != gui {
		t.Fatalf("got %s want %s", found, gui)
	}
}

func TestRunWithoutNodeReturnsEnvError(t *testing.T) {
	tmp := t.TempDir()
	gui := filepath.Join(tmp, "gui")
	if err := os.MkdirAll(filepath.Join(gui, "src", "renderer"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(gui, "package.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Point Node at something that cannot possibly exist.
	err := Run(context.Background(), Options{GUIRoot: gui, Node: filepath.Join(tmp, "no-such-node")})
	if err == nil {
		t.Fatal("expected error")
	}
	var envErr *EnvError
	if !errors.As(err, &envErr) {
		t.Fatalf("expected EnvError, got %T(%v)", err, err)
	}
	if ExitCode(err) != 2 {
		t.Fatalf("expected exit code 2, got %d", ExitCode(err))
	}
}
