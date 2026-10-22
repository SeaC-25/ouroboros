// Package guicontract drives gui/scripts/check-gui-contract.mjs (embedded as
// driver.mjs, byte-identical to the original script) from Go.
//
// Design note: the original script performs deep TypeScript-AST checks via the
// `typescript` package (findNamedDeclaration, JSX attribute inspection, if-branch
// extraction). A stdlib-only Go port could not reproduce those semantics
// faithfully, so the Go side owns orchestration only: resolve the gui root and
// node, materialise the embedded driver into a temporary directory directly
// under the gui root (so `new URL('..', import.meta.url)` and bare `typescript`
// imports resolve exactly as they do for gui/scripts/), stream its output, and
// map exit codes onto the shared contract: 0 = pass, 1 = contract violation,
// 2 = environment/IO failure.
package guicontract

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

// Options configures a single contract run.
type Options struct {
	// GUIRoot is the gui/ directory. Empty probes from the working directory.
	GUIRoot string
	// Node is the node executable; empty resolves "node" on PATH.
	Node string
	// Stdout/Stderr receive driver output; nil defaults to os.Stdout/os.Stderr.
	Stdout io.Writer
	Stderr io.Writer
}

// ContractError: the driver ran but reported a contract failure.
type ContractError struct{ ExitCode int }

func (e *ContractError) Error() string {
	return fmt.Sprintf("gui contract failed (driver exit code %d)", e.ExitCode)
}

// EnvError is an environment or IO failure (exit code 2).
type EnvError struct {
	Path string
	Err  error
}

func (e *EnvError) Error() string {
	if e.Path != "" {
		return fmt.Sprintf("gui contract environment error at %s: %v", e.Path, e.Err)
	}
	return fmt.Sprintf("gui contract environment error: %v", e.Err)
}

func (e *EnvError) Unwrap() error { return e.Err }

// ExitCode maps a Run result onto the shared exit-code contract.
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

// Run executes the GUI contract check.
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

	stdout, stderr := opts.Stdout, opts.Stderr
	if stdout == nil {
		stdout = os.Stdout
	}
	if stderr == nil {
		stderr = os.Stderr
	}

	// The driver computes the gui root as the parent of the directory that
	// contains the script file, so it must be materialised exactly one level
	// under the gui root. Bare `import ts from 'typescript'` resolves from
	// gui/node_modules against the driver location, matching the original.
	driverDir, err := os.MkdirTemp(guiRoot, ".gui-contract-")
	if err != nil {
		return &EnvError{Path: guiRoot, Err: fmt.Errorf("creating driver directory: %w", err)}
	}
	defer func() { _ = os.RemoveAll(driverDir) }()

	driverPath := filepath.Join(driverDir, "driver.mjs")
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
			if exitErr.ExitCode() == 1 {
				return &ContractError{ExitCode: 1}
			}
			return &EnvError{Err: fmt.Errorf("driver exited with code %d", exitErr.ExitCode())}
		}
		return &EnvError{Err: fmt.Errorf("running driver: %w", err)}
	}
	return nil
}

// findGUIRoot walks upwards from the working directory looking for the gui
// root, identified by its package.json plus the renderer entry directory.
func findGUIRoot() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", &EnvError{Err: err}
	}
	marker := func(dir string) bool {
		if _, err := os.Stat(filepath.Join(dir, "package.json")); err != nil {
			return false
		}
		st, err := os.Stat(filepath.Join(dir, "src", "renderer"))
		return err == nil && st.IsDir()
	}
	for dir := cwd; ; dir = filepath.Dir(dir) {
		if marker(dir) {
			return dir, nil
		}
		candidate := filepath.Join(dir, "gui")
		if marker(candidate) {
			return candidate, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
	}
	return "", &EnvError{Err: fmt.Errorf("locate gui root walking up from %s", cwd)}
}
