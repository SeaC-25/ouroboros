package ouroboros_test

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/ouroboros/ouroboros/internal/app"
)

func TestReleaseScriptCreatesArchiveAndChecksums(t *testing.T) {
	dist := t.TempDir()
	version := "0.1.0-test"
	releaseTestBundle := "ouro-release-test-tool"
	writeReleaseScriptManagedBundleFixture(t, releaseTestBundle)
	env := append(os.Environ(),
		"GOPROXY=https://goproxy.cn,direct",
		"DIST_DIR="+dist,
		"VERSION="+version,
		"GOOS="+runtime.GOOS,
		"GOARCH="+runtime.GOARCH,
	)
	cmd := exec.Command("sh", "./scripts/release.sh")
	cmd.Env = env
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("release script failed: %v\n%s", err, output)
	}

	archiveName := "ouro_" + version + "_" + runtime.GOOS + "_" + runtime.GOARCH + ".tar.gz"
	archivePath := filepath.Join(dist, archiveName)
	if _, err := os.Stat(archivePath); err != nil {
		t.Fatalf("expected release archive %s: %v\noutput:\n%s", archivePath, err, output)
	}
	sumBytes, err := os.ReadFile(filepath.Join(dist, "SHA256SUMS"))
	if err != nil {
		t.Fatalf("read SHA256SUMS: %v", err)
	}
	sumText := string(sumBytes)
	if !strings.Contains(sumText, archiveName) {
		t.Fatalf("expected SHA256SUMS to include archive name %q, got %q", archiveName, sumText)
	}

	members := tarGzMembers(t, archivePath)
	for _, want := range []string{
		"ouro_" + version + "_" + runtime.GOOS + "_" + runtime.GOARCH + "/ouro",
		"ouro_" + version + "_" + runtime.GOOS + "_" + runtime.GOARCH + "/README.md",
		"ouro_" + version + "_" + runtime.GOOS + "_" + runtime.GOARCH + "/install.sh",
		"ouro_" + version + "_" + runtime.GOOS + "_" + runtime.GOARCH + "/toolchains/managed-bundles/" + releaseTestBundle + "/bundle.yaml",
		"ouro_" + version + "_" + runtime.GOOS + "_" + runtime.GOARCH + "/toolchains/managed-bundles/" + releaseTestBundle + "/payload/" + releaseTestBundle,
	} {
		if !members[want] {
			t.Fatalf("expected archive member %q in %#v", want, members)
		}
	}
}

func TestInstallScriptInstallsFromLocalArchiveWithoutNetwork(t *testing.T) {
	releaseRoot := t.TempDir()
	payloadDir := filepath.Join(releaseRoot, "ouro_0.1.0-test_linux_amd64")
	if err := os.MkdirAll(payloadDir, 0o755); err != nil {
		t.Fatalf("mkdir payload: %v", err)
	}
	binary := filepath.Join(payloadDir, "ouro")
	if err := os.WriteFile(binary, []byte("#!/usr/bin/env sh\nprintf 'fake ouro\\n'\n"), 0o755); err != nil {
		t.Fatalf("write fake binary: %v", err)
	}
	bundleDir := filepath.Join(payloadDir, "toolchains", "managed-bundles", "httpx")
	if err := os.MkdirAll(filepath.Join(bundleDir, "payload"), 0o755); err != nil {
		t.Fatalf("mkdir fake bundle payload: %v", err)
	}
	if err := os.WriteFile(filepath.Join(bundleDir, "bundle.yaml"), []byte("name: httpx\nkind: binary\nversion: \"1.3.0\"\nsource: test\npayload: payload/httpx\nsha256: 0000000000000000000000000000000000000000000000000000000000000000\n"), 0o644); err != nil {
		t.Fatalf("write fake bundle manifest: %v", err)
	}
	if err := os.WriteFile(filepath.Join(bundleDir, "payload", "httpx"), []byte("#!/usr/bin/env sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write fake bundle payload: %v", err)
	}
	archivePath := filepath.Join(releaseRoot, "ouro_0.1.0-test_linux_amd64.tar.gz")
	createTarGz(
		t,
		archivePath,
		releaseRoot,
		filepath.Base(payloadDir)+"/ouro",
		filepath.Base(payloadDir)+"/toolchains/managed-bundles/httpx/bundle.yaml",
		filepath.Base(payloadDir)+"/toolchains/managed-bundles/httpx/payload/httpx",
	)

	prefix := filepath.Join(t.TempDir(), "prefix")
	cmd := exec.Command("sh", "./scripts/install.sh", archivePath)
	cmd.Env = append(os.Environ(), "PREFIX="+prefix)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("install script failed: %v\n%s", err, output)
	}
	installed := filepath.Join(prefix, "bin", "ouro")
	info, err := os.Stat(installed)
	if err != nil {
		t.Fatalf("expected installed binary %s: %v\noutput:\n%s", installed, err, output)
	}
	if info.Mode()&0o111 == 0 {
		t.Fatalf("expected installed binary to be executable, mode=%s", info.Mode())
	}
	installedBundle := filepath.Join(prefix, "share", "ouroboros", "toolchains", "managed-bundles", "httpx", "bundle.yaml")
	if _, err := os.Stat(installedBundle); err != nil {
		t.Fatalf("expected installed managed bundle metadata %s: %v\noutput:\n%s", installedBundle, err, output)
	}
}

func TestReleaseMakefileAndCIEntrypoints(t *testing.T) {
	makefileBytes, err := os.ReadFile("Makefile")
	if err != nil {
		t.Fatalf("read Makefile: %v", err)
	}
	makefile := string(makefileBytes)
	for _, want := range []string{".PHONY: test build doctor doctor-strict release clean", "doctor-strict:", "--require-safety-authorization", "release:", "./scripts/release.sh"} {
		if !strings.Contains(makefile, want) {
			t.Fatalf("expected Makefile to contain %q, got:\n%s", want, makefile)
		}
	}

	workflowBytes, err := os.ReadFile(filepath.Join(".github", "workflows", "ci.yml"))
	if err != nil {
		t.Fatalf("read CI workflow: %v", err)
	}
	workflow := string(workflowBytes)
	for _, want := range []string{"make test", "make doctor-strict", "make build", "make release"} {
		if !strings.Contains(workflow, want) {
			t.Fatalf("expected CI workflow to contain %q, got:\n%s", want, workflow)
		}
	}
}

func TestMakeDoctorKeepsSafetyBypassWarningsNonBlocking(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := app.InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	scope := `version: 1
scope:
  target_policy: disabled
  targets: []
  exclusions: []
execution:
  isolation: disabled
`
	if err := os.WriteFile(filepath.Join(root, "scope.yaml"), []byte(scope), 0o644); err != nil {
		t.Fatalf("write scope: %v", err)
	}

	cmd := exec.Command("make", "doctor")
	cmd.Env = append(os.Environ(), "GOPROXY=https://goproxy.cn,direct", "WORKSPACE="+root)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("default make doctor should remain warning-only: %v\n%s", err, output)
	}
	for _, want := range []string{
		"workspace: ready",
		"missing safety bypass authorization for scope.target_policy disabled",
		"missing safety bypass authorization for execution.isolation disabled",
	} {
		if !strings.Contains(string(output), want) {
			t.Fatalf("expected default make doctor output to contain %q, got:\n%s", want, output)
		}
	}
}

func TestMakeDoctorStrictRequiresSafetyBypassAuthorization(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := app.InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	scope := `version: 1
scope:
  target_policy: disabled
  targets: []
  exclusions: []
execution:
  isolation: disabled
`
	if err := os.WriteFile(filepath.Join(root, "scope.yaml"), []byte(scope), 0o644); err != nil {
		t.Fatalf("write scope: %v", err)
	}

	cmd := exec.Command("make", "doctor-strict")
	cmd.Env = append(os.Environ(), "GOPROXY=https://goproxy.cn,direct", "WORKSPACE="+root)
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected strict make doctor to fail without safety authorization, got output:\n%s", output)
	}
	for _, want := range []string{
		"missing required safety bypass authorization for scope.target_policy disabled",
		"missing required safety bypass authorization for execution.isolation disabled",
	} {
		if !strings.Contains(string(output), want) {
			t.Fatalf("expected strict make doctor output to contain %q, got:\n%s", want, output)
		}
	}
}

func TestMakeReleaseSupportsRelativeDistDir(t *testing.T) {
	relativeDist := "dist-test-release-scripts"
	t.Cleanup(func() {
		_ = os.RemoveAll(relativeDist)
	})
	version := "0.1.0-make-test"
	cmd := exec.Command("make", "release")
	cmd.Env = append(os.Environ(),
		"GOPROXY=https://goproxy.cn,direct",
		"DIST_DIR="+relativeDist,
		"VERSION="+version,
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("make release failed: %v\n%s", err, output)
	}
	archiveName := "ouro_" + version + "_" + runtime.GOOS + "_" + runtime.GOARCH + ".tar.gz"
	if _, err := os.Stat(filepath.Join(relativeDist, archiveName)); err != nil {
		t.Fatalf("expected relative dist archive: %v\noutput:\n%s", err, output)
	}
	if _, err := os.Stat(filepath.Join(relativeDist, "SHA256SUMS")); err != nil {
		t.Fatalf("expected relative dist SHA256SUMS: %v\noutput:\n%s", err, output)
	}
}

func tarGzMembers(t *testing.T, archivePath string) map[string]bool {
	t.Helper()
	file, err := os.Open(archivePath)
	if err != nil {
		t.Fatalf("open archive: %v", err)
	}
	defer file.Close()
	gz, err := gzip.NewReader(file)
	if err != nil {
		t.Fatalf("open gzip: %v", err)
	}
	defer gz.Close()
	reader := tar.NewReader(gz)
	members := map[string]bool{}
	for {
		header, err := reader.Next()
		if err != nil {
			if err.Error() == "EOF" {
				break
			}
			t.Fatalf("read tar: %v", err)
		}
		members[header.Name] = true
	}
	return members
}

func createTarGz(t *testing.T, archivePath string, root string, names ...string) {
	t.Helper()
	out, err := os.Create(archivePath)
	if err != nil {
		t.Fatalf("create archive: %v", err)
	}
	defer out.Close()
	gz := gzip.NewWriter(out)
	defer gz.Close()
	writer := tar.NewWriter(gz)
	defer writer.Close()
	for _, name := range names {
		path := filepath.Join(root, filepath.FromSlash(name))
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat tar member: %v", err)
		}
		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			t.Fatalf("tar header: %v", err)
		}
		header.Name = name
		if err := writer.WriteHeader(header); err != nil {
			t.Fatalf("write tar header: %v", err)
		}
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read tar member: %v", err)
		}
		if _, err := writer.Write(content); err != nil {
			t.Fatalf("write tar member: %v", err)
		}
	}
}

func writeReleaseScriptManagedBundleFixture(t *testing.T, name string) {
	t.Helper()
	bundleDir := filepath.Join("toolchains", "managed-bundles", name)
	t.Cleanup(func() {
		_ = os.RemoveAll(bundleDir)
	})
	payloadBody := []byte("#!/usr/bin/env sh\nexit 0\n")
	if err := os.MkdirAll(filepath.Join(bundleDir, "payload"), 0o755); err != nil {
		t.Fatalf("create release script bundle payload dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(bundleDir, "payload", name), payloadBody, 0o755); err != nil {
		t.Fatalf("write release script bundle payload: %v", err)
	}
	digest := sha256.Sum256(payloadBody)
	manifest := `name: ` + name + `
kind: binary
version: "1.3.0"
source: projectdiscovery/` + name + `
payload: payload/` + name + `
sha256: ` + hex.EncodeToString(digest[:]) + `
`
	if err := os.WriteFile(filepath.Join(bundleDir, "bundle.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write release script bundle manifest: %v", err)
	}
}
