// Package devmgr mirrors gui/scripts/dev-desktop.mjs: build the desktop main
// bundle, launch a Vite dev server, wait for its development URL, then spawn
// the Electron runtime with VITE_DEV_SERVER_URL injected. Incoming
// SIGINT/SIGTERM are forwarded to the currently active child; lingering
// children are escalated SIGTERM -> (grace) -> SIGKILL, matching the mjs
// signal semantics.
//
// Node.js-specific work (vite dev server, electron spawn with the electron
// package resolved path) runs inside the embedded drivers under drivers/ so
// the behaviour stays 1:1 with the mjs script; the Go side owns process
// orchestration only.
//
// Exit-code contract (mirrors the mjs script and the shared desktop policy):
//
//	0 — the dev session ran to completion successfully
//	130/143 — the orchestrator was asked to shut down by SIGINT/SIGTERM
//	1 — a child failed (contract failure of the dev session itself); the
//	    error is a *ChildError carrying the underlying exit code
//	2 — environment / IO failure (node missing, gui tree not found, ...)
package devmgr

import (
	"errors"
	"fmt"
	"path/filepath"
)

// gracefulShutdownTimeout mirrors gracefulShutdownMs in dev-desktop.mjs.
const gracefulShutdownMilliseconds = 10000

// forcedShutdownTimeout mirrors forcedShutdownMs in dev-desktop.mjs.
const forcedShutdownMilliseconds = 2000

// marker is emitted by the vite driver on stdout followed by the resolved
// development URL; see drivers/vite-driver.mjs.
const marker = "OURO_DEV_SERVER_URL"

// Options configures Run.
type Options struct {
	// GUIRoot is the gui/ directory containing scripts/ and its own
	// node_modules. When empty the orchestrator walks upwards from the
	// current working directory looking for gui/scripts/dev-desktop.mjs,
	// mirroring how the mjs script resolves guiRoot from its own location.
	GUIRoot string

	// LogLevel, when non-empty, is passed to the vite dev server (matches
	// options.logLevel in the mjs script).
	LogLevel string
}

// ChildError is a contract failure: a child of the dev session exited badly.
// Code mirrors process.exitCode propagation in the mjs assertSuccessfulChild.
type ChildError struct {
	Label string
	Err   error
	// Code is the child exit code when the failure came from a non-zero
	// exit; -signal semantics are represented with a negative signal value.
	Code int
}

func (e *ChildError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %v", e.Label, e.Err)
	}
	return fmt.Sprintf("%s failed with exit code %d", e.Label, e.Code)
}

// Unwrap exposes the underlying spawn error, when any.
func (e *ChildError) Unwrap() error { return e.Err }

// EnvError is an environment / IO failure (exit code 2).
type EnvError struct {
	What string
	Err  error
}

func (e *EnvError) Error() string {
	return fmt.Sprintf("%s: %v", e.What, e.Err)
}

// Unwrap exposes the wrapped environment error.
func (e *EnvError) Unwrap() error { return e.Err }

// ExitCode maps a Run result onto the shared exit-code contract: 0 success,
// 130/143 for operator interrupt termination (mirrors signalExitCode in the
// mjs), the failing child's own exit code for contract failures (falling
// back to 1), and 2 for environment / IO failures.
func ExitCode(err error) int {
	if err == nil {
		return 0
	}
	var sig *SignalExitError
	if errors.As(err, &sig) {
		if sig.Signal == "SIGINT" {
			return 130
		}
		return 143
	}
	var child *ChildError
	if errors.As(err, &child) {
		if child.Code > 0 {
			return child.Code
		}
		return 1
	}
	return 2
}

// SignalExitError reports an orchestrator shutdown requested by SIGINT /
// SIGTERM while children were still active, mirroring the
// `return signalExitCode(requestedSignal)` paths in dev-desktop.mjs.
type SignalExitError struct {
	Signal string
}

func (e *SignalExitError) Error() string {
	return fmt.Sprintf("desktop development terminated by %s", e.Signal)
}

// Run executes the desktop development session underneath the configured gui
// root using the real Node.js/Electron toolchain.
func Run(opts Options) error {
	root := opts.GUIRoot
	if root == "" {
		var err error
		if root, err = findGUIRoot(); err != nil {
			return err
		}
	} else if !isGUIRoot(root) {
		return &EnvError{What: fmt.Sprintf("gui root %s", root), Err: errors.New("missing scripts/dev-desktop.mjs")}
	}
	return RunWithRunner(root, opts, NewExecRunner(root))
}

// isGUIRoot reports whether dir looks like the gui root the mjs script
// resolves (has scripts/dev-desktop.mjs).
func isGUIRoot(dir string) bool {
	return fileExists(filepath.Join(dir, "scripts", "dev-desktop.mjs"))
}
