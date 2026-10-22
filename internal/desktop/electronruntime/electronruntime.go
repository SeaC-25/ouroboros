// Package electronruntime is the Go port of gui/scripts/check-electron-runtime.mjs.
//
// The subject under test is the built Electron main-process JavaScript
// (memory-profile.js, backend.js, main.js produced by build-desktop-main.mjs),
// so the contract assertions must execute inside Node.js to remain
// behaviourally identical. This package therefore acts as the orchestrator only:
// it locates the gui tree, materialises the embedded driver script (a
// byte-identical copy of gui/scripts/check-electron-runtime.mjs) into a
// temporary directory directly under the gui root — required so that the
// driver's `new URL('..', import.meta.url)` still resolves to the gui root —
// runs it with node, streams output, and translates the outcome into the
// shared exit-code contract:
//
//	0 — all contract checks passed (driver exits 0, prints
//	    "electron runtime contract: ok")
//	1 — contract failure (driver exited non-zero, e.g. an assertion failed)
//	2 — environment / IO error (node missing, gui tree not found, the
//	    temporary driver file could not be written, ...)
//
// Every error carries the relevant file path.
package electronruntime

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
)

//go:embed driver.mjs
var driverSource []byte

// Options controls a single contract run.
type Options struct {
	// GUIRoot is the path to the gui/ directory. When empty it is detected by
	// walking upwards from the working directory looking for
	// gui/scripts/build-desktop-main.mjs.
	GUIRoot string
	// Node is the node executable; empty resolves "node" via PATH.
	Node string
	// Stdout/Stderr receive the driver output; nil defaults to os.Stdout/os.Stderr.
	Stdout io.Writer
	Stderr io.Writer
}

// ContractError is returned when the driver ran but reported a contract
// failure (exit code 1).
type ContractError struct {
	ExitCode int
	Output   string
}

func (e *ContractError) Error() string {
	return fmt.Sprintf("electron runtime contract failed (driver exit code %d)", e.ExitCode)
}

// EnvError is returned for environment or IO failures (exit code 2).
type EnvError struct {
	Path string
	Err  error
}

func (e *EnvError) Error() string {
	if e.Path != "" {
		return fmt.Sprintf("electron runtime environment error at %s: %v", e.Path, e.Err)
	}
	return fmt.Sprintf("electron runtime environment error: %v", e.Err)
}

func (e *EnvError) Unwrap() error { return e.Err }

// ExitCode maps a Run result to the shared exit-code contract.
func ExitCode(err error) int {
	if err == nil {
		return 0
	}
	var contractErr *ContractError
	if errors.As(err, &contractErr) {
		return 1
	}
	return 2
}

// Run executes the electron runtime contract checks.
func Run(ctx context.Context, opts Options) error {
	guiRoot := opts.GUIRoot
	if guiRoot == "" {
		found, err := findGUIRoot()
		if err != nil {
			return err
		}
		guiRoot = found
	}

	node := opts.Node
	if node == "" {
		resolved, err := exec.LookPath("node")
		if err != nil {
			return &EnvError{Path: "node", Err: errors.New("node executable not found on PATH")}
		}
		node = resolved
	}

	stdout := opts.Stdout
	if stdout == nil {
		stdout = os.Stdout
	}
	stderr := opts.Stderr
	if stderr == nil {
		stderr = os.Stderr
	}

	// The driver resolves the gui root as the parent of its own directory
	// (`new URL('..', import.meta.url)`), exactly like the original script lives
	// at <gui>/scripts/. It must therefore be materialised one level below the
	// gui root. The directory is removed on completion.
	driverDir, err := os.MkdirTemp(guiRoot, ".electron-runtime-check-")
	if err != nil {
		return &EnvError{Path: guiRoot, Err: fmt.Errorf("creating driver directory: %w", err)}
	}
	defer func() { _ = os.RemoveAll(driverDir) }()

	driverPath := filepath.Join(driverDir, "check-electron-runtime.mjs")
	if err := os.WriteFile(driverPath, driverSource, 0o644); err != nil {
		return &EnvError{Path: driverPath, Err: fmt.Errorf("writing driver: %w", err)}
	}

	cmd := exec.CommandContext(ctx, node, driverPath)
	cmd.Dir = guiRoot
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return &ContractError{ExitCode: exitErr.ExitCode()}
		}
		return &EnvError{Path: driverPath, Err: err}
	}
	return nil
}

// findGUIRoot walks upwards from the current working directory until it finds
// the gui tree, identified by its package.json plus the renderer entry
// directory (the desktop tooling itself now lives in Go under cmd/desktop).
func findGUIRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", &EnvError{Err: fmt.Errorf("resolving working directory: %w", err)}
	}
	marker := func(candidate string) bool {
		if _, statErr := os.Stat(filepath.Join(candidate, "package.json")); statErr != nil {
			return false
		}
		info, statErr := os.Stat(filepath.Join(candidate, "src", "renderer"))
		return statErr == nil && info.IsDir()
	}
	for {
		candidate := filepath.Join(dir, "gui")
		if marker(candidate) {
			return candidate, nil
		}
		if marker(dir) {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", &EnvError{Path: "gui/package.json", Err: errors.New("could not locate the gui tree from the working directory")}
		}
		dir = parent
	}
}
