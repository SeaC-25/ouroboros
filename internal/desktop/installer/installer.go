// Package installer is a Go port of gui/scripts/install-desktop.mjs,
// extended to self-contained cross-platform installs. The original script
// only supported Linux and shelled out to scripts/install_desktop_app.sh;
// here the install logic is implemented natively:
//
//   - linux:   copies the package to $XDG_DATA_HOME/ouroboros
//     (default ~/.local/share/ouroboros) and installs a .desktop file into
//     $XDG_DATA_HOME/applications
//   - windows: copies the package to %LOCALAPPDATA%\Programs\Ouroboros
//   - darwin:  copies the package to /Applications/Ouroboros
//
// Exit codes (shared contract): 0 ok, 1 contract failure, 2 env/IO error.
package installer

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/ouroboros/ouroboros/internal/desktop/packager"
)

const (
	ExitOK      = 0
	ExitFailure = 1
	ExitIOError = 2
)

// ActionKind identifies one filesystem/action step of an install plan.
type ActionKind int

const (
	ActionMkdirAll ActionKind = iota
	ActionRemoveAll
	ActionCopyDir
	ActionWriteFile
	ActionChmod
	// ActionRun executes an external command; failures are tolerated
	// (mirrors the mjs `update-desktop-database ... || true`).
	ActionRun
)

// Action is one planned install step.
type Action struct {
	Kind    ActionKind
	Path    string // MkdirAll/RemoveAll/Chmod target; CopyDir destination
	Src     string // CopyDir source
	Content string // WriteFile payload
	Mode    fs.FileMode
	Command string   // ActionRun executable
	Args    []string // ActionRun arguments
}

// Describe renders the action as a single dry-run line.
func (a Action) Describe() string {
	switch a.Kind {
	case ActionMkdirAll:
		return "mkdir -p " + a.Path
	case ActionRemoveAll:
		return "rm -rf " + a.Path
	case ActionCopyDir:
		return fmt.Sprintf("copy %s %s", a.Src, a.Path)
	case ActionWriteFile:
		return "write " + a.Path
	case ActionChmod:
		return fmt.Sprintf("chmod %04o %s", a.Mode, a.Path)
	case ActionRun:
		return "run " + a.Command + " " + strings.Join(a.Args, " ")
	default:
		return "unknown action"
	}
}

// Plan is the full install: where the staged package lives, where it will be
// installed, and the ordered actions to perform.
type Plan struct {
	Platform   string
	Arch       string
	PackageDir string // gui/dist/desktop/Ouroboros-<platform>-<arch>
	InstallDir string
	Actions    []Action
}

// Options controls plan creation and execution.
type Options struct {
	// GUIroot is the gui/ directory; defaults to <RepoRoot>/gui.
	GUIroot string
	// RepoRoot defaults to the parent of GUIroot.
	RepoRoot string
	// Platform/Arch override runtime.GOOS/GOARCH (mainly for tests).
	Platform string
	Arch     string

	// FS performs the filesystem actions; nil = packager.OSFS{}.
	// Runner executes ActionRun commands; nil = packager.ExecRunner{}.
	FS     packager.FS
	Runner packager.Runner
	// PackageBuilder builds the desktop package; nil = packager.Run.
	// Tests substitute a fake that assembles a fixture package.
	PackageBuilder func(ctx context.Context, o packager.Options) int
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
		o.FS = packager.OSFS{}
	}
	if o.Runner == nil {
		o.Runner = packager.ExecRunner{}
	}
	if o.PackageBuilder == nil {
		o.PackageBuilder = packager.Run
	}
}

// CreatePlan computes the install plan for the given options, mirroring
// createDesktopInstallPlan() but across linux/windows/darwin.
func CreatePlan(o Options) (Plan, error) {
	o.defaults()
	packageDir := filepath.Join(o.GUIroot, "dist", "desktop",
		fmt.Sprintf("Ouroboros-%s-%s", o.Platform, o.Arch))
	p := Plan{Platform: o.Platform, Arch: o.Arch, PackageDir: packageDir}

	switch o.Platform {
	case "linux":
		dataHome := os.Getenv("XDG_DATA_HOME")
		if dataHome == "" || !filepath.IsAbs(dataHome) {
			home, err := os.UserHomeDir()
			if err != nil {
				return Plan{}, fmt.Errorf("cannot resolve home directory: %w", err)
			}
			dataHome = filepath.Join(home, ".local", "share")
		}
		installDir := filepath.Join(dataHome, "ouroboros")
		applicationsDir := filepath.Join(dataHome, "applications")
		desktopFile := filepath.Join(applicationsDir, packager.LinuxDesktopEntryName)
		p.InstallDir = installDir
		p.Actions = []Action{
			{Kind: ActionMkdirAll, Path: dataHome, Mode: 0o755},
			{Kind: ActionRemoveAll, Path: installDir},
			{Kind: ActionCopyDir, Src: packageDir, Path: installDir},
			{Kind: ActionMkdirAll, Path: applicationsDir, Mode: 0o755},
			{Kind: ActionWriteFile, Path: desktopFile,
				Content: packager.LinuxDesktopEntryFor(path.Join, filepath.ToSlash(installDir)),
				Mode:    0o644},
			{Kind: ActionRun, Command: "update-desktop-database", Args: []string{applicationsDir}},
		}
	case "windows":
		base := os.Getenv("LOCALAPPDATA")
		if base == "" {
			home, err := os.UserHomeDir()
			if err != nil {
				return Plan{}, fmt.Errorf("cannot resolve LOCALAPPDATA or home directory: %w", err)
			}
			base = filepath.Join(home, "AppData", "Local")
		}
		installDir := filepath.Join(base, "Programs", "Ouroboros")
		p.InstallDir = installDir
		p.Actions = []Action{
			{Kind: ActionMkdirAll, Path: filepath.Dir(installDir), Mode: 0o755},
			{Kind: ActionRemoveAll, Path: installDir},
			{Kind: ActionCopyDir, Src: packageDir, Path: installDir},
		}
	case "darwin":
		installDir := filepath.Join("/Applications", "Ouroboros")
		p.InstallDir = installDir
		p.Actions = []Action{
			{Kind: ActionMkdirAll, Path: "/Applications", Mode: 0o755},
			{Kind: ActionRemoveAll, Path: installDir},
			{Kind: ActionCopyDir, Src: packageDir, Path: installDir},
		}
	default:
		return Plan{}, fmt.Errorf("Desktop build-and-install does not support platform %s", o.Platform)
	}
	return p, nil
}

// Execute performs the plan's actions against the real filesystem.
// ActionRun failures are logged to stderr but not fatal.
func Execute(ctx context.Context, p Plan, o Options, stderr io.Writer) int {
	o.defaults()
	for _, a := range p.Actions {
		var err error
		switch a.Kind {
		case ActionMkdirAll:
			err = o.FS.MkdirAll(a.Path, a.Mode)
		case ActionRemoveAll:
			err = o.FS.RemoveAll(a.Path)
		case ActionCopyDir:
			err = o.FS.CopyDir(a.Src, a.Path)
		case ActionWriteFile:
			err = o.FS.WriteFile(a.Path, []byte(a.Content), a.Mode)
		case ActionChmod:
			err = o.FS.Chmod(a.Path, a.Mode)
		case ActionRun:
			if code, runErr := o.Runner.Run(ctx, "", nil, a.Command, a.Args...); runErr != nil || code != 0 {
				fmt.Fprintf(stderr, "warning: %s failed (ignored): %v\n", a.Describe(), firstErr(runErr, fmt.Errorf("exit code %d", code)))
			}
			continue
		}
		if err != nil {
			fmt.Fprintf(stderr, "install step failed (%s): %v\n", a.Describe(), err)
			return ExitIOError
		}
	}
	return ExitOK
}

func firstErr(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return errors.New("unknown error")
}

// Main mirrors runDesktopInstall(): [--dry-run] only; builds the desktop
// package via packager, then (unless dry-run) installs the fresh package.
// It returns the process exit code; output goes to stdout/stderr.
func Main(ctx context.Context, argv []string, o Options, stdout, stderr io.Writer) int {
	for _, arg := range argv {
		if arg != "--dry-run" {
			fmt.Fprintf(stderr, "Ouroboros desktop build/install failed: Unknown argument: %s\n", arg)
			return ExitFailure
		}
	}
	dryRun := len(argv) > 0 && argv[0] == "--dry-run"
	o.defaults()

	plan, err := CreatePlan(o)
	if err != nil {
		fmt.Fprintf(stderr, "Ouroboros desktop build/install failed: %v\n", err)
		return ExitFailure
	}

	fmt.Fprintf(stdout, "desktop package source: %s\n", o.RepoRoot)
	fmt.Fprintf(stdout, "desktop package target: %s\n", plan.PackageDir)
	code := o.PackageBuilder(ctx, packager.Options{
		GUIroot:  o.GUIroot,
		RepoRoot: o.RepoRoot,
		Platform: o.Platform,
		Arch:     o.Arch,
		DryRun:   dryRun,
		DirMode:  true,
		Stdout:   stdout,
		Stderr:   stderr,
	})
	if code != 0 {
		fmt.Fprintf(stderr, "Desktop package build failed with exit code %d\n", code)
		return code
	}

	if dryRun {
		fmt.Fprintln(stdout, "dry-run install plan:")
		for _, a := range plan.Actions {
			fmt.Fprintln(stdout, "  "+a.Describe())
		}
		return ExitOK
	}

	fmt.Fprintf(stdout, "installing freshly built desktop package: %s\n", plan.PackageDir)
	if code := Execute(ctx, plan, o, stderr); code != 0 {
		return code
	}
	fmt.Fprintf(stdout, "Ouroboros desktop installed: %s\n", plan.InstallDir)
	return ExitOK
}
