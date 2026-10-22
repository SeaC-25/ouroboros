// Package packager is a Go port of gui/scripts/package-desktop.mjs.
// It builds the desktop backend binary (--dir-less mode) or assembles the
// full Electron desktop package (--dir mode), with a --dry-run mode that
// prints the planned steps without touching disk.
package packager

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
)

// Exit codes (shared contract): 0 ok, 1 contract/build failure, 2 env/IO error.
const (
	ExitOK      = 0
	ExitFailure = 1
	ExitIOError = 2
)

// Runner executes external build commands. Implementations must return the
// child's exit code; err is non-nil only when the command could not start.
type Runner interface {
	Run(ctx context.Context, dir string, env []string, name string, args ...string) (code int, err error)
}

// ExecRunner shells out via os/exec.
type ExecRunner struct {
	Stdout io.Writer
	Stderr io.Writer
	Stdin  io.Reader
}

// Options controls a packager run. Zero-value fields fall back to the same
// defaults the mjs script derives from its own location and platform.
type Options struct {
	// GUIroot is the gui/ directory; defaults to <RepoRoot>/gui.
	GUIroot string
	// RepoRoot is the repository root; defaults to the parent of GUIroot.
	RepoRoot string
	// Platform/Arch override runtime.GOOS/GOARCH (mainly for tests).
	Platform string
	Arch     string
	// DryRun prints the step list and exits 0 without touching disk.
	DryRun bool
	// DirMode selects the full package assembly (--dir); false = backend-only.
	DirMode bool

	FS     FS
	Runner Runner
	Stdout io.Writer
	Stderr io.Writer
	// Env is the base environment for child commands; nil = os.Environ().
	Env []string
}

func (o *Options) defaults() {
	if o.Platform == "" {
		o.Platform = runtime.GOOS
	}
	if o.Arch == "" {
		o.Arch = runtime.GOARCH
	}
	if o.GUIroot == "" && o.RepoRoot != "" {
		o.GUIroot = filepath.Join(o.RepoRoot, "gui")
	}
	if o.RepoRoot == "" && o.GUIroot != "" {
		o.RepoRoot = filepath.Dir(o.GUIroot)
	}
	if o.FS == nil {
		o.FS = OSFS{}
	}
	if o.Stdout == nil {
		o.Stdout = os.Stdout
	}
	if o.Stderr == nil {
		o.Stderr = os.Stderr
	}
	if o.Env == nil {
		o.Env = os.Environ()
	}
	if o.Runner == nil {
		o.Runner = ExecRunner{}
	}
}

// Layout holds the derived paths for a run.
type Layout struct {
	BinaryName      string
	ResourceBinary  string // gui/dist/resources/ouro[.exe]
	DesktopDir      string // gui/dist/desktop/Ouroboros-<platform>-<arch>
	ElectronDist    string // gui/node_modules/electron/dist
	PackagedAppDir  string
	PackagedBackend string
	LinuxEntryPath  string
	LinuxInstaller  string
	BuildScript     string
}

func (o Options) layout() Layout {
	gui := o.GUIroot
	binaryName := "ouro"
	if o.Platform == "windows" {
		binaryName = "ouro.exe"
	}
	desktopDir := filepath.Join(gui, "dist", "desktop",
		fmt.Sprintf("Ouroboros-%s-%s", o.Platform, o.Arch))
	return Layout{
		BinaryName:      binaryName,
		ResourceBinary:  filepath.Join(gui, "dist", "resources", binaryName),
		DesktopDir:      desktopDir,
		ElectronDist:    filepath.Join(gui, "node_modules", "electron", "dist"),
		PackagedAppDir:  filepath.Join(desktopDir, "resources", "app"),
		PackagedBackend: filepath.Join(desktopDir, "resources", binaryName),
		LinuxEntryPath:  filepath.Join(desktopDir, LinuxDesktopEntryName),
		LinuxInstaller:  filepath.Join(desktopDir, "install-desktop-entry.sh"),
		BuildScript:     filepath.Join(o.RepoRoot, "scripts", "build.sh"),
	}
}

// Steps returns the dry-run step list, 1:1 with the mjs script's output.
func Steps(o Options) []string {
	o.defaults()
	l := o.layout()
	if !o.DirMode {
		return []string{
			fmt.Sprintf("mkdir -p %s", filepath.Dir(l.ResourceBinary)),
			fmt.Sprintf("BIN=%s %s", l.ResourceBinary, l.BuildScript),
		}
	}
	app := l.PackagedAppDir
	return []string{
		fmt.Sprintf("npm --prefix %s run build", o.GUIroot),
		fmt.Sprintf("mkdir -p %s", filepath.Dir(l.ResourceBinary)),
		fmt.Sprintf("BIN=%s %s", l.ResourceBinary, l.BuildScript),
		fmt.Sprintf("rm -rf %s", l.DesktopDir),
		fmt.Sprintf("copyElectronRuntime %s %s", l.ElectronDist, l.DesktopDir),
		fmt.Sprintf("mkdir -p %s", app),
		fmt.Sprintf("copy %s %s", filepath.Join(o.GUIroot, "dist", "renderer"),
			filepath.Join(app, "dist", "renderer")),
		fmt.Sprintf("copy %s %s", filepath.Join(o.GUIroot, "dist", "main"),
			filepath.Join(app, "dist", "main")),
		fmt.Sprintf("write %s", filepath.Join(app, "package.json")),
		fmt.Sprintf("copy %s %s", l.ResourceBinary, l.PackagedBackend),
		fmt.Sprintf("write %s", l.LinuxEntryPath),
		fmt.Sprintf("write %s", l.LinuxInstaller),
	}
}

// Run executes the packaging flow and returns the process exit code.
func Run(ctx context.Context, o Options) int {
	o.defaults()
	if o.DryRun {
		for _, step := range Steps(o) {
			fmt.Fprintln(o.Stdout, step)
		}
		return ExitOK
	}

	if o.DirMode {
		if code := o.runFrontendBuild(ctx); code != 0 {
			return code
		}
	}

	l := o.layout()
	if code := o.buildBackend(ctx, l); code != 0 {
		return code
	}
	if code := o.assertFile(l.ResourceBinary, "desktop backend binary"); code != 0 {
		return code
	}

	if !o.DirMode {
		fmt.Fprintf(o.Stdout, "desktop backend ready: %s\n", l.ResourceBinary)
		return ExitOK
	}

	if code := o.copyElectronRuntime(l); code != 0 {
		return code
	}
	if code := o.copyAppBundle(l); code != 0 {
		return code
	}
	if err := o.FS.CopyFile(l.ResourceBinary, l.PackagedBackend); err != nil {
		fmt.Fprintf(o.Stderr, "failed to copy backend binary to %s: %v\n", l.PackagedBackend, err)
		return ExitIOError
	}
	if o.Platform != "windows" {
		if err := o.FS.Chmod(l.PackagedBackend, 0o755); err != nil {
			fmt.Fprintf(o.Stderr, "failed to chmod %s: %v\n", l.PackagedBackend, err)
			return ExitIOError
		}
	}
	if code := o.assertFile(l.PackagedBackend, "packaged desktop backend binary"); code != 0 {
		return code
	}
	if code := o.writeLinuxDesktopEntry(l); code != 0 {
		return code
	}

	fmt.Fprintf(o.Stdout, "package:desktop:dir: %s\n", l.DesktopDir)
	return ExitOK
}
