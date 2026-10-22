package packager

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// The methods below implement the individual steps of package-desktop.mjs.
// Each returns a process exit code (0 = success), mirroring the mjs script's
// `process.exit(result.status ?? 1)` behaviour, and prints diagnostics to
// Options.Stderr on failure.

// runFrontendBuild mirrors runFrontendBuild(): `npm run build` in the gui
// root (via cmd.exe on Windows) with VITE_OURO_API_BASE_URL emptied.
func (o Options) runFrontendBuild(ctx context.Context) int {
	command, args := "npm", []string{"run", "build"}
	if o.Platform == "windows" {
		command, args = "cmd.exe", []string{"/d", "/s", "/c", "npm run build"}
	}
	env := append(append([]string{}, o.Env...), "VITE_OURO_API_BASE_URL=")
	code, err := o.Runner.Run(ctx, o.GUIroot, env, command, args...)
	if err != nil {
		fmt.Fprintf(o.Stderr, "desktop frontend build failed: %v\n", err)
		return ExitFailure
	}
	return code
}

// buildBackend mirrors buildBackend(): creates the output directory, then
// runs scripts/build.sh with BIN=<output> from the repo root (via sh.exe on
// Windows).
func (o Options) buildBackend(ctx context.Context, l Layout) int {
	if err := o.FS.MkdirAll(filepath.Dir(l.ResourceBinary), 0o777); err != nil {
		fmt.Fprintf(o.Stderr, "failed to create %s: %v\n", filepath.Dir(l.ResourceBinary), err)
		return ExitIOError
	}
	command, args := l.BuildScript, []string(nil)
	if o.Platform == "windows" {
		command, args = "sh.exe", []string{l.BuildScript}
	}
	env := append(append([]string{}, o.Env...), "BIN="+l.ResourceBinary)
	code, err := o.Runner.Run(ctx, o.RepoRoot, env, command, args...)
	if err != nil {
		fmt.Fprintf(o.Stderr, "desktop backend build failed: %v\n", err)
		return ExitFailure
	}
	return code
}

// assertFile mirrors assertFile(): the path must exist and be a non-empty
// regular file.
func (o Options) assertFile(path, label string) int {
	info, err := o.FS.Stat(path)
	if err != nil {
		fmt.Fprintf(o.Stderr, "%s was not created: %s\n", label, path)
		return ExitFailure
	}
	if !info.Mode().IsRegular() || info.Size() == 0 {
		fmt.Fprintf(o.Stderr, "%s is invalid: %s\n", label, path)
		return ExitFailure
	}
	return ExitOK
}

// copyElectronRuntime mirrors copyElectronRuntime() incl.
// renameElectronBinary().
func (o Options) copyElectronRuntime(l Layout) int {
	if _, err := o.FS.Stat(l.ElectronDist); err != nil {
		fmt.Fprintf(o.Stderr, "Electron runtime not found: %s\n", l.ElectronDist)
		return ExitFailure
	}
	if err := o.FS.RemoveAll(l.DesktopDir); err != nil {
		fmt.Fprintf(o.Stderr, "failed to remove %s: %v\n", l.DesktopDir, err)
		return ExitIOError
	}
	if err := o.FS.MkdirAll(filepath.Dir(l.DesktopDir), 0o777); err != nil {
		fmt.Fprintf(o.Stderr, "failed to create %s: %v\n", filepath.Dir(l.DesktopDir), err)
		return ExitIOError
	}
	if err := o.FS.CopyDir(l.ElectronDist, l.DesktopDir); err != nil {
		fmt.Fprintf(o.Stderr, "failed to copy Electron runtime to %s: %v\n", l.DesktopDir, err)
		return ExitIOError
	}
	return o.renameElectronBinary(l.DesktopDir)
}

func (o Options) renameElectronBinary(destination string) int {
	sourceName, targetName := "electron", "Ouroboros"
	if o.Platform == "windows" {
		sourceName, targetName = "electron.exe", "Ouroboros.exe"
	}
	sourcePath := filepath.Join(destination, sourceName)
	targetPath := filepath.Join(destination, targetName)
	if _, err := o.FS.Stat(sourcePath); err != nil {
		return ExitOK // mjs: rename only when the electron binary exists
	}
	if err := o.FS.Rename(sourcePath, targetPath); err != nil {
		fmt.Fprintf(o.Stderr, "failed to rename %s to %s: %v\n", sourcePath, targetPath, err)
		return ExitIOError
	}
	if o.Platform != "windows" {
		if err := o.FS.Chmod(targetPath, 0o755); err != nil {
			fmt.Fprintf(o.Stderr, "failed to chmod %s: %v\n", targetPath, err)
			return ExitIOError
		}
	}
	return ExitOK
}

// copyAppBundle mirrors copyAppBundle(): renderer + main bundles plus a
// synthesized package.json into resources/app of the staged package.
func (o Options) copyAppBundle(l Layout) int {
	if err := o.FS.MkdirAll(l.PackagedAppDir, 0o777); err != nil {
		fmt.Fprintf(o.Stderr, "failed to create %s: %v\n", l.PackagedAppDir, err)
		return ExitIOError
	}
	pairs := [][2]string{
		{filepath.Join(o.GUIroot, "dist", "renderer"), filepath.Join(l.PackagedAppDir, "dist", "renderer")},
		{filepath.Join(o.GUIroot, "dist", "main"), filepath.Join(l.PackagedAppDir, "dist", "main")},
	}
	for _, pair := range pairs {
		if err := o.FS.CopyDir(pair[0], pair[1]); err != nil {
			fmt.Fprintf(o.Stderr, "failed to copy %s to %s: %v\n", pair[0], pair[1], err)
			return ExitIOError
		}
	}
	target := filepath.Join(l.PackagedAppDir, "package.json")
	if err := o.FS.WriteFile(target, []byte(PackagedAppPackageJSON()), 0o666); err != nil {
		fmt.Fprintf(o.Stderr, "failed to write %s: %v\n", target, err)
		return ExitIOError
	}
	return ExitOK
}

// writeLinuxDesktopEntry mirrors writeLinuxDesktopEntry(): Linux-only, writes
// the .desktop file and its installer script into the staged package.
func (o Options) writeLinuxDesktopEntry(l Layout) int {
	if o.Platform != "linux" {
		return ExitOK
	}
	if err := o.FS.WriteFile(l.LinuxEntryPath, []byte(linuxPackageDesktopEntry()), 0o666); err != nil {
		fmt.Fprintf(o.Stderr, "failed to write %s: %v\n", l.LinuxEntryPath, err)
		return ExitIOError
	}
	if err := o.FS.WriteFile(l.LinuxInstaller, []byte(LinuxDesktopInstallerScript(filepath.Join)), 0o666); err != nil {
		fmt.Fprintf(o.Stderr, "failed to write %s: %v\n", l.LinuxInstaller, err)
		return ExitIOError
	}
	if err := o.FS.Chmod(l.LinuxInstaller, 0o755); err != nil {
		fmt.Fprintf(o.Stderr, "failed to chmod %s: %v\n", l.LinuxInstaller, err)
		return ExitIOError
	}
	if code := o.assertFile(l.LinuxEntryPath, "packaged Linux desktop entry"); code != 0 {
		return code
	}
	return o.assertFile(l.LinuxInstaller, "packaged Linux desktop entry installer")
}

// Run starts the command with stdio wired through (stdio: 'inherit' in the
// mjs script) and waits for it. env is the complete environment; when nil,
// os.Environ() is used.
func (e ExecRunner) Run(ctx context.Context, dir string, env []string, name string, args ...string) (int, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	if env != nil {
		cmd.Env = env
	}
	cmd.Stdout = e.Stdout
	if cmd.Stdout == nil {
		cmd.Stdout = os.Stdout
	}
	cmd.Stderr = e.Stderr
	if cmd.Stderr == nil {
		cmd.Stderr = os.Stderr
	}
	cmd.Stdin = e.Stdin
	if cmd.Stdin == nil {
		cmd.Stdin = os.Stdin
	}
	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return exitErr.ExitCode(), nil
		}
		return ExitFailure, err
	}
	return ExitOK, nil
}

// PackagedAppPackageJSON is the package.json payload copyAppBundle() writes
// into resources/app (1:1 with the mjs JSON.stringify(..., null, 2) output).
func PackagedAppPackageJSON() string {
	return `{
  "name": "ouroboros-desktop",
  "productName": "Ouroboros",
  "desktopName": "` + LinuxDesktopEntryName + `",
  "version": "0.1.0",
  "type": "module",
  "main": "dist/main/main.js"
}
`
}
