// Package productcontract drives the product contract check (the former
// gui/scripts/check-product-contract.mjs, embedded as driver.mjs) from Go.
//
// Design notes:
//   - The original script parses the theme stylesheet with postcss and runs
//     a cascade-aware red-black brand audit. Reimplementing that CSS
//     machinery in Go would need a new dependency, so the Go side owns
//     orchestration only: resolve the repo/gui roots and node, materialise
//     the embedded driver inside the gui root (so the bare `postcss`
//     import resolves from gui/node_modules exactly as it does for
//     gui/scripts/), stream its output, and map exit codes onto the shared
//     contract: 0 = pass, 1 = contract violation, 2 = environment/IO
//     failure.
//   - Unlike the original script, the driver does not hardcode the
//     2026-07-03 master-plan filename: docs/superpowers/plans/*.md is
//     globbed and the lexicographically latest (date-prefixed) document is
//     checked. A missing or empty plans directory aborts the driver with
//     exit code 2 before any contract assertion runs.
package productcontract

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

// Options configures a single product contract run.
type Options struct {
	// GUIRoot is the gui/ directory. Empty probes from the working
	// directory. It is used as the driver host so node can resolve the
	// gui/node_modules dependencies (postcss).
	GUIRoot string
	// RepoRoot is the repository root containing PRODUCT.md, DESIGN.md and
	// docs/superpowers/plans. Empty defaults to the parent of GUIRoot.
	RepoRoot string
	// Node is the node executable; empty resolves "node" on PATH.
	Node string
	// Stdout/Stderr receive driver output; nil defaults to os.Stdout/os.Stderr.
	Stdout io.Writer
	Stderr io.Writer
}

// ContractError: the driver ran but reported a contract failure.
type ContractError struct{ ExitCode int }

func (e *ContractError) Error() string {
	return fmt.Sprintf("product contract failed (driver exit code %d)", e.ExitCode)
}

// EnvError is an environment or IO failure (exit code 2): node missing,
// driver not writable, or — per the plans-glob change — no
// docs/superpowers/plans/*.md master plan in the checked tree.
type EnvError struct {
	Path string
	Err  error
}

func (e *EnvError) Error() string {
	if e.Path != "" {
		return fmt.Sprintf("product contract environment error at %s: %v", e.Path, e.Err)
	}
	return fmt.Sprintf("product contract environment error: %v", e.Err)
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

// Run executes the product contract check.
func Run(ctx context.Context, opts Options) error {
	guiRoot := opts.GUIRoot
	if guiRoot == "" {
		found, err := findGUIRoot()
		if err != nil {
			return err
		}
		guiRoot = found
	}
	repoRoot := opts.RepoRoot
	if repoRoot == "" {
		repoRoot = filepath.Dir(guiRoot)
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

	// The driver imports postcss as a bare specifier, so it must live
	// under the gui root where gui/node_modules/postcss is resolvable.
	driverDir, err := os.MkdirTemp(guiRoot, ".product-contract-")
	if err != nil {
		return &EnvError{Path: guiRoot, Err: fmt.Errorf("creating driver directory: %w", err)}
	}
	defer func() { _ = os.RemoveAll(driverDir) }()

	driverPath := filepath.Join(driverDir, "driver.mjs")
	if err := os.WriteFile(driverPath, driverSource, 0o644); err != nil {
		return &EnvError{Path: driverPath, Err: fmt.Errorf("writing driver: %w", err)}
	}

	absRepo, err := filepath.Abs(repoRoot)
	if err != nil {
		return &EnvError{Path: repoRoot, Err: err}
	}

	cmd := exec.CommandContext(ctx, node, driverPath, absRepo)
	cmd.Dir = guiRoot
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			switch exitErr.ExitCode() {
			case 1:
				return &ContractError{ExitCode: 1}
			default:
				// Exit code 2 (missing plans document, missing root
				// argument) and any unexpected driver crash are
				// environment/IO failures, not contract violations.
				return &EnvError{Err: fmt.Errorf("driver exited with code %d", exitErr.ExitCode())}
			}
		}
		return &EnvError{Err: fmt.Errorf("running driver: %w", err)}
	}
	return nil
}

// findGUIRoot walks upwards from the working directory for
// gui/scripts/check-product-contract.mjs, mirroring guicontract.findGUIRoot.
func findGUIRoot() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", &EnvError{Err: err}
	}
	marker := func(dir string) bool {
		_, err := os.Stat(filepath.Join(dir, "scripts", "check-product-contract.mjs"))
		return err == nil
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
