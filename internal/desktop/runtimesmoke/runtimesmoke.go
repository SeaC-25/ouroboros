// Package runtimesmoke is the Go port of gui/scripts/run-desktop-runtime-smoke.mjs.
//
// It validates the packaged desktop runtime smoke prerequisites and delegates
// to scripts/smoke_desktop_package_runtime.sh via sh (sh.exe on Windows),
// forwarding the process exit code. After the smoke run it cleans up leftover
// packaged Electron processes on Windows via PowerShell.
//
// Exit codes mirror the original script: 0 = pass, 1 = smoke failed or the
// shell could not start (environment/IO problems also surface through the
// delegated shell script's own exit codes).
package runtimesmoke

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/spf13/cobra"
)

const (
	// defaultDebugPort mirrors the original cleanup needle default.
	defaultDebugPort = "19224"
	// defaultPackageDirName follows the packager's output convention
	// (Ouroboros-{GOOS}-{GOARCH} under gui/dist/desktop).
	defaultPackageDirName = "Ouroboros-" + runtime.GOOS + "-" + runtime.GOARCH

	envDebugPort  = "OURO_DESKTOP_DEBUG_PORT"
	envPackageDir = "OURO_DESKTOP_PACKAGE_DIR"
	envSmokeMode  = "OURO_DESKTOP_PACKAGE_RUNTIME_SMOKE_MODE"
	envWSModule   = "OURO_DESKTOP_PACKAGE_RUNTIME_WS_MODULE"
)

// Options carries every injectable dependency of the smoke runner.
type Options struct {
	// RepoRoot is the repository root; smoke paths are resolved against it.
	RepoRoot string
	// Mode is the optional smoke mode (quick|diagnostics|ai-lite); empty = full run.
	Mode string
	// Env is the inherited environment (defaults to os.Environ).
	Env []string
	// Getenv reads override values; defaults to os.Getenv.
	Getenv func(string) string
	// FileExists reports whether path exists; defaults to os.Stat-based check.
	FileExists func(string) bool
	// Run executes the smoke command and reports its outcome; defaults to
	// runCommand (exec-based).
	Run func(cmd *exec.Cmd) RunResult
	// CleanupWindows runs the residual-process sweep; defaults to PowerShell runner.
	CleanupWindows func(port, packageDirBase string)
	// Stdout/Stderr/Stdin wired to the child process.
	Stdout io.Writer
	Stderr io.Writer
	Stdin  io.Reader
}

func (o *Options) defaults() {
	if o.Getenv == nil {
		o.Getenv = os.Getenv
	}
	if o.Env == nil {
		o.Env = os.Environ()
	}
	if o.FileExists == nil {
		o.FileExists = func(path string) bool {
			_, err := os.Stat(path)
			return err == nil
		}
	}
	if o.Stdout == nil {
		o.Stdout = os.Stdout
	}
	if o.Stderr == nil {
		o.Stderr = os.Stderr
	}
	if o.Stdin == nil {
		o.Stdin = os.Stdin
	}
	if o.Run == nil {
		o.Run = runCommand
	}
	if o.CleanupWindows == nil {
		o.CleanupWindows = cleanupWindowsSmokeProcesses
	}
}

// Execute performs the full smoke flow and returns the process exit code.
func Execute(opts Options) int {
	opts.defaults()

	smokeScript := filepath.Join(opts.RepoRoot, "scripts", "smoke_desktop_package_runtime.sh")
	if !opts.FileExists(smokeScript) {
		fmt.Fprintf(opts.Stderr, "desktop runtime smoke script missing: %s\n", smokeScript)
		return 1
	}
	webSocketModule := filepath.Join(opts.RepoRoot, "gui", "node_modules", "ws", "index.js")
	if !opts.FileExists(webSocketModule) {
		fmt.Fprintf(opts.Stderr, "desktop runtime smoke WebSocket module missing: %s\n", webSocketModule)
		return 1
	}

	shell := "sh"
	if runtime.GOOS == "windows" {
		shell = "sh.exe"
	}

	env := append([]string{}, opts.Env...)
	env = append(env, envWSModule+"="+webSocketModule)
	if mode := strings.TrimSpace(opts.Mode); mode != "" {
		env = append(env, envSmokeMode+"="+mode)
	}

	cmd := exec.Command(shell, smokeScript)
	cmd.Dir = opts.RepoRoot
	cmd.Env = env
	cmd.Stdout = opts.Stdout
	cmd.Stderr = opts.Stderr
	cmd.Stdin = opts.Stdin
	prepareCommand(cmd)

	result := opts.Run(cmd)

	// Residual-process sweep: best effort, mirrors the original script which
	// runs it unconditionally after spawnSync returns.
	cleanupSmokeProcesses(opts)

	if result.StartErr != nil {
		fmt.Fprintf(opts.Stderr, "desktop runtime smoke could not start %s: %v\n", shell, result.StartErr)
		return 1
	}
	if result.Signaled {
		fmt.Fprintf(opts.Stderr, "desktop runtime smoke stopped by signal\n")
		return 1
	}
	return result.ExitCode
}

// RunResult mirrors Node's spawnSync triple {status, signal, error}.
type RunResult struct {
	// StartErr is set when the shell could not be spawned at all.
	StartErr error
	// Signaled is true when the child was terminated by a signal (no exit code).
	Signaled bool
	// ExitCode is the child exit status when it exited normally.
	ExitCode int
}

// runCommand is the production Run implementation.
func runCommand(cmd *exec.Cmd) RunResult {
	err := cmd.Run()
	if err == nil {
		return RunResult{}
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		return RunResult{StartErr: err}
	}
	if code := exitErr.ExitCode(); code >= 0 {
		return RunResult{ExitCode: code}
	}
	return RunResult{Signaled: true}
}

// cleanupSmokeProcesses resolves the cleanup parameters from env with the
// original defaults and invokes the Windows sweep (no-op elsewhere).
func cleanupSmokeProcesses(opts Options) {
	if runtime.GOOS != "windows" {
		return
	}
	getenv := opts.Getenv
	port := resolvePortFunc(getenv)
	if !isDigits(port) {
		return
	}
	opts.CleanupWindows(port, resolveRootFunc(getenv))
}

// resolvePort returns OURO_DESKTOP_DEBUG_PORT trimmed, or the original
// default 19224. Kept as a tiny seam so tests can validate resolution without
// touching the real environment.
func resolvePort(env map[string]string) string {
	return resolvePortFunc(func(k string) string { return env[k] })
}

func resolvePortFunc(getenv func(string) string) string {
	if v := strings.TrimSpace(getenv(envDebugPort)); v != "" {
		return v
	}
	return defaultDebugPort
}

// resolvePackageRoot returns the basename of OURO_DESKTOP_PACKAGE_DIR, or the
// original hardcoded package directory name when unset/unusable.
func resolvePackageRoot(env map[string]string) string {
	return resolveRootFunc(func(k string) string { return env[k] })
}

func resolveRootFunc(getenv func(string) string) string {
	dir := strings.TrimSpace(getenv(envPackageDir))
	if dir == "" {
		return defaultPackageDirName
	}
	base := filepath.Base(filepath.Clean(dir))
	if base == "." || base == string(filepath.Separator) || base == string(filepath.VolumeName(dir)) {
		return defaultPackageDirName
	}
	return base
}

func isDigits(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// cleanupWindowsSmokeProcesses kills packaged Electron leftovers whose command
// line references the debug port and whose executable lives under the package
// directory name, exactly matching the original PowerShell one-liner.
func cleanupWindowsSmokeProcesses(port, packageDirBase string) {
	command := strings.Join([]string{
		"$needle = '--remote-debugging-port=" + port + "'",
		"$root = '" + strings.ReplaceAll(packageDirBase, "'", "''") + "'",
		"Get-CimInstance Win32_Process | Where-Object { $_.CommandLine -and $_.CommandLine.Contains($needle) -and $_.ExecutablePath -and $_.ExecutablePath.Contains($root) } | ForEach-Object { Stop-Process -Id $_.ProcessId -Force -ErrorAction SilentlyContinue }",
	}, "; ")
	cmd := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", command)
	_ = cmd.Run()
}

// ValidModes enumerates the subcommand modes kept from the original wrapper.
var ValidModes = []string{"quick", "diagnostics", "ai-lite"}

// NewCommand builds the cobra command for `desktop runtime-smoke [mode]`.
// Integrators register it under cmd/desktop.
func NewCommand() *cobra.Command {
	var repoRoot string
	cmd := &cobra.Command{
		Use:   "runtime-smoke [mode]",
		Short: "Run the packaged desktop runtime smoke (Go port of run-desktop-runtime-smoke.mjs)",
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) > 1 {
				return fmt.Errorf("expected at most one mode argument, got %d", len(args))
			}
			if len(args) == 1 {
				for _, mode := range ValidModes {
					if args[0] == mode {
						return nil
					}
				}
				return fmt.Errorf("unknown smoke mode %q (valid: %s)", args[0], strings.Join(ValidModes, ", "))
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			root := repoRoot
			if root == "" {
				wd, err := os.Getwd()
				if err != nil {
					return fmt.Errorf("resolve working directory: %w", err)
				}
				root = wd
			}
			mode := ""
			if len(args) == 1 {
				mode = args[0]
			}
			code := Execute(Options{RepoRoot: root, Mode: mode})
			if code != 0 {
				// os.Exit-style semantics: propagate the exact code.
				return exitCodeError{code: code}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&repoRoot, "repo-root", "", "repository root (defaults to working directory)")
	return cmd
}

// ExitCodeError carries a non-zero exit code the CLI entrypoint must forward
// verbatim (cobra would otherwise coerce everything to 1).
type ExitCodeError interface {
	error
	ExitCode() int
}

type exitCodeError struct{ code int }

func (e exitCodeError) Error() string { return fmt.Sprintf("exit status %d", e.code) }
func (e exitCodeError) ExitCode() int { return e.code }
