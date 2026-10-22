package installer

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ouroboros/ouroboros/internal/desktop/packager"
)

// fakePackageBuilder fabricates the staged package directory the mjs flow
// expects packager to have produced.
func fakePackageBuilder(t *testing.T) func(context.Context, packager.Options) int {
	t.Helper()
	return func(_ context.Context, o packager.Options) int {
		dir := filepath.Join(o.GUIroot, "dist", "desktop", "Ouroboros-"+o.Platform+"-"+o.Arch)
		mustWrite(t, filepath.Join(dir, "Ouroboros"), "elf")
		mustWrite(t, filepath.Join(dir, "resources", "app", "dist", "main", "main.js"), "js")
		return packager.ExitOK
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func baseOptions(t *testing.T, platform string) Options {
	t.Helper()
	gui := t.TempDir()
	return Options{
		GUIroot:        gui,
		RepoRoot:       t.TempDir(),
		Platform:       platform,
		Arch:           "amd64",
		PackageBuilder: fakePackageBuilder(t),
	}
}

func TestCreatePlanUnsupportedPlatform(t *testing.T) {
	o := baseOptions(t, "plan9")
	if _, err := CreatePlan(o); err == nil || !strings.Contains(err.Error(), "plan9") {
		t.Fatalf("unsupported platform must fail with platform name: %v", err)
	}
}

func TestPlanLinuxXDG(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv("XDG_DATA_HOME", xdg)
	p, err := CreatePlan(baseOptions(t, "linux"))
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(xdg, "ouroboros"); p.InstallDir != want {
		t.Fatalf("InstallDir = %q, want %q", p.InstallDir, want)
	}
	kinds := make([]ActionKind, len(p.Actions))
	for i, a := range p.Actions {
		kinds[i] = a.Kind
	}
	wantKinds := []ActionKind{ActionMkdirAll, ActionRemoveAll, ActionCopyDir, ActionMkdirAll, ActionWriteFile, ActionRun}
	for i := range wantKinds {
		if kinds[i] != wantKinds[i] {
			t.Fatalf("action %d kind = %v, want %v", i, kinds[i], wantKinds[i])
		}
	}
	entry := p.Actions[4]
	if !strings.HasSuffix(entry.Path, packager.LinuxDesktopEntryName) {
		t.Fatalf("desktop entry path wrong: %s", entry.Path)
	}
	if !strings.Contains(entry.Content, "Name=Ouroboros") || !strings.Contains(entry.Content, "MimeType=x-scheme-handler/ouroboros;") {
		t.Fatalf("desktop entry content wrong:\n%s", entry.Content)
	}
	if !strings.Contains(entry.Content, "Exec=\"") {
		t.Fatalf("installed entry must Exec the installed binary: %s", entry.Content)
	}
	if p.Actions[5].Command != "update-desktop-database" {
		t.Fatalf("last action should refresh the desktop database: %+v", p.Actions[5])
	}
}

func TestPlanLinuxNoXDG(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", "")
	p, err := CreatePlan(baseOptions(t, "linux"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(p.InstallDir, filepath.Join(".local", "share", "ouroboros")) {
		t.Fatalf("InstallDir should fall back to ~/.local/share/ouroboros: %s", p.InstallDir)
	}
}

func TestPlanWindows(t *testing.T) {
	base := t.TempDir()
	t.Setenv("LOCALAPPDATA", base)
	p, err := CreatePlan(baseOptions(t, "windows"))
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(base, "Programs", "Ouroboros"); p.InstallDir != want {
		t.Fatalf("InstallDir = %q, want %q", p.InstallDir, want)
	}
}

func TestPlanDarwinIsApplications(t *testing.T) {
	p, err := CreatePlan(baseOptions(t, "darwin"))
	if err != nil {
		t.Fatal(err)
	}
	if p.InstallDir != filepath.Join("/Applications", "Ouroboros") {
		t.Fatalf("InstallDir = %q", p.InstallDir)
	}
}

// noopRunner swallows update-desktop-database without a real binary.
type noopRunner struct{}

func (noopRunner) Run(context.Context, string, []string, string, ...string) (int, error) {
	return 0, nil
}

func TestExecuteInstallsFiles(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv("XDG_DATA_HOME", xdg)
	o := baseOptions(t, "linux")
	o.Runner = noopRunner{}

	p, err := CreatePlan(o)
	if err != nil {
		t.Fatal(err)
	}
	// Simulate the freshly built package.
	mustWrite(t, filepath.Join(p.PackageDir, "Ouroboros"), "elf")
	mustWrite(t, filepath.Join(p.PackageDir, "resources", "app", "dist", "main", "main.js"), "js")

	var stderr bytes.Buffer
	if code := Execute(context.Background(), p, o, &stderr); code != ExitOK {
		t.Fatalf("Execute exit = %d, stderr: %s", code, stderr.String())
	}
	for _, want := range []string{
		filepath.Join(p.InstallDir, "Ouroboros"),
		filepath.Join(p.InstallDir, "resources", "app", "dist", "main", "main.js"),
		filepath.Join(xdg, "applications", packager.LinuxDesktopEntryName),
	} {
		if info, err := os.Stat(want); err != nil || info.Size() == 0 {
			t.Errorf("installed artifact missing: %s (%v)", want, err)
		}
	}
	entry, err := os.ReadFile(filepath.Join(xdg, "applications", packager.LinuxDesktopEntryName))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(entry), filepath.ToSlash(p.InstallDir)) {
		t.Errorf("desktop entry must point into the install dir:\n%s", entry)
	}

	// Reinstall: stale payload must be replaced (rm -rf semantics).
	mustWrite(t, filepath.Join(p.InstallDir, "stale.txt"), "stale")
	_ = os.Remove(filepath.Join(p.PackageDir, "stale.txt"))
	if code := Execute(context.Background(), p, o, &stderr); code != ExitOK {
		t.Fatalf("reinstall exit = %d", code)
	}
	if _, err := os.Stat(filepath.Join(p.InstallDir, "stale.txt")); !os.IsNotExist(err) {
		t.Errorf("stale file must be removed on reinstall, err=%v", err)
	}
}

func TestMainDryRunPrintsPlanAndTouchesNothing(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv("XDG_DATA_HOME", xdg)
	o := baseOptions(t, "linux")
	var out, errOut bytes.Buffer
	code := Main(context.Background(), []string{"--dry-run"}, o, &out, &errOut)
	if code != ExitOK {
		t.Fatalf("exit = %d, stderr: %s", code, errOut.String())
	}
	text := out.String()
	for _, want := range []string{"desktop package source:", "desktop package target:", "dry-run install plan:", "mkdir -p", "copy ", "write "} {
		if !strings.Contains(text, want) {
			t.Errorf("dry-run output missing %q:\n%s", want, text)
		}
	}
	if !strings.Contains(text, "Ouroboros-linux-amd64") {
		t.Errorf("target dir must name platform/arch:\n%s", text)
	}
	entries, _ := os.ReadDir(xdg)
	if len(entries) != 0 {
		t.Fatalf("dry-run must not write into XDG_DATA_HOME: %v", entries)
	}
}

func TestMainUnknownArg(t *testing.T) {
	var out, errOut bytes.Buffer
	code := Main(context.Background(), []string{"--bogus"}, baseOptions(t, "linux"), &out, &errOut)
	if code != ExitFailure {
		t.Fatalf("unknown arg must exit 1, got %d", code)
	}
	if !strings.Contains(errOut.String(), "Unknown argument: --bogus") {
		t.Fatalf("stderr mismatch: %q", errOut.String())
	}
}

func TestMainFullInstallFlow(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv("XDG_DATA_HOME", xdg)
	o := baseOptions(t, "linux")
	o.Runner = noopRunner{}
	var out, errOut bytes.Buffer
	if code := Main(context.Background(), nil, o, &out, &errOut); code != ExitOK {
		t.Fatalf("exit = %d, stderr: %s", code, errOut.String())
	}
	if !strings.Contains(out.String(), "installing freshly built desktop package:") {
		t.Fatalf("missing install banner:\n%s", out.String())
	}
	if _, err := os.Stat(filepath.Join(xdg, "ouroboros", "Ouroboros")); err != nil {
		t.Fatalf("installed binary missing: %v", err)
	}
}
