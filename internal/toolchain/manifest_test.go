package toolchain

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadManifestsFromToolchainPacks(t *testing.T) {
	manifests, err := LoadManifests(filepath.Join("..", "..", "toolchains"))
	if err != nil {
		t.Fatalf("load manifests: %v", err)
	}
	if len(manifests) != 3 {
		t.Fatalf("expected three manifests, got %d", len(manifests))
	}

	byName := map[string]Manifest{}
	for _, manifest := range manifests {
		byName[manifest.Name] = manifest
	}
	for _, name := range []string{"web-api-pack", "infra-pack", "ctf-pack"} {
		manifest, ok := byName[name]
		if !ok {
			t.Fatalf("expected manifest %s", name)
		}
		if err := ValidateManifest(manifest); err != nil {
			t.Fatalf("expected %s to validate: %v", name, err)
		}
	}
}

func TestBuiltInAppSecToolDependenciesAreDeclared(t *testing.T) {
	manifests, err := LoadManifests(filepath.Join("..", "..", "toolchains"))
	if err != nil {
		t.Fatalf("load manifests: %v", err)
	}

	byName := map[string]Manifest{}
	for _, manifest := range manifests {
		byName[manifest.Name] = manifest
	}

	assertToolDependencies(t, byName["web-api-pack"], []string{
		"nuclei",
		"nuclei-templates",
		"httpx",
		"subfinder",
		"ffuf",
		"katana",
		"gau",
		"waybackurls",
		"dalfox",
		"arjun",
		"sqlmap",
	})
	assertToolDependencies(t, byName["infra-pack"], []string{
		"nmap",
		"naabu",
		"dnsx",
		"amass",
		"dnsrecon",
		"testssl.sh",
		"sslscan",
	})
	assertToolDependencies(t, byName["ctf-pack"], []string{
		"apktool",
		"jadx",
		"mobsfscan",
		"semgrep",
	})
}

func TestBuiltInAppSecToolCatalogIncludesCoreTools(t *testing.T) {
	catalog := BuiltInAppSecTools()
	byName := map[string]BuiltInTool{}
	for _, tool := range catalog {
		byName[tool.Name] = tool
	}
	for name, pack := range map[string]string{
		"nuclei":    "web-api-pack",
		"httpx":     "web-api-pack",
		"subfinder": "web-api-pack",
		"ffuf":      "web-api-pack",
		"katana":    "web-api-pack",
		"nmap":      "infra-pack",
		"naabu":     "infra-pack",
		"dnsx":      "infra-pack",
	} {
		tool, ok := byName[name]
		if !ok {
			t.Fatalf("expected catalog tool %s in %#v", name, catalog)
		}
		if tool.Pack != pack {
			t.Fatalf("expected %s pack %s, got %#v", name, pack, tool)
		}
		if tool.Category == "" || len(tool.Capabilities) == 0 {
			t.Fatalf("expected catalog metadata for %s, got %#v", name, tool)
		}
	}
}

func TestValidateManifestRejectsMissingCapability(t *testing.T) {
	manifest := Manifest{
		Name:        "bad-pack",
		Version:     "1.0.0",
		Runner:      "native",
		Parser:      "parser",
		Healthcheck: "built_in",
	}

	if err := ValidateManifest(manifest); err == nil {
		t.Fatalf("expected missing capability to fail")
	}
}

func TestValidateManifestRejectsMissingVersionLock(t *testing.T) {
	manifest := Manifest{
		Name:         "bad-pack",
		Capabilities: []string{"web.discovery"},
		Runner:       "native",
		Parser:       "parser",
		Healthcheck:  "built_in",
	}

	if err := ValidateManifest(manifest); err == nil {
		t.Fatalf("expected missing version to fail")
	}
}

func TestValidateManifestRequiresExecHealthCommand(t *testing.T) {
	manifest := validManifest()
	manifest.Healthcheck = "exec"

	if err := ValidateManifest(manifest); err == nil {
		t.Fatalf("expected exec healthcheck without command to fail")
	}

	manifest.HealthCommand = []string{"probe", "--version"}
	if err := ValidateManifest(manifest); err != nil {
		t.Fatalf("expected exec healthcheck with command to validate: %v", err)
	}
}

func TestValidateManifestRequiresCommandRunnerCommand(t *testing.T) {
	manifest := validManifest()
	manifest.Runner = "command"

	if err := ValidateManifest(manifest); err == nil {
		t.Fatalf("expected command runner without run_command to fail")
	}

	manifest.RunCommand = []string{"printf", "ok\n"}
	if err := ValidateManifest(manifest); err != nil {
		t.Fatalf("expected command runner with run_command to validate: %v", err)
	}
}

func TestValidateManifestRejectsNegativeRunTimeout(t *testing.T) {
	manifest := validManifest()
	manifest.Runner = "command"
	manifest.RunCommand = []string{"printf", "ok\n"}
	manifest.RunTimeoutMS = -1

	if err := ValidateManifest(manifest); err == nil {
		t.Fatalf("expected negative run timeout to fail")
	}

	manifest.RunTimeoutMS = 1000
	if err := ValidateManifest(manifest); err != nil {
		t.Fatalf("expected positive run timeout to validate: %v", err)
	}
}

func TestLoadManifestParsesRunTimeoutMS(t *testing.T) {
	path := filepath.Join(t.TempDir(), "manifest.yaml")
	data := `name: web-api-pack
version: "0.1.0"
capabilities:
  - web.discovery
runner: command
run_command:
  - printf
  - "ok\n"
run_timeout_ms: 250
parser: web_parser
healthcheck: built_in
`
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	manifest, err := LoadManifest(path)
	if err != nil {
		t.Fatalf("load manifest: %v", err)
	}
	if manifest.RunTimeoutMS != 250 {
		t.Fatalf("expected run timeout to be parsed, got %d", manifest.RunTimeoutMS)
	}
}

func TestLoadManifestParsesDependencies(t *testing.T) {
	path := filepath.Join(t.TempDir(), "manifest.yaml")
	data := `name: web-api-pack
version: "0.1.0"
capabilities:
  - web.discovery
runner: native
parser: web_parser
healthcheck: built_in
dependencies:
  - name: runtime-base
    kind: binary
    install_command:
      - ouro-toolchain
      - install
      - runtime-base
  - name: httpx
    kind: binary
    version: "1.3.0"
    depends_on:
      - runtime-base
    health_command:
      - httpx
      - -version
    install_hint: install bundled httpx pack
    install_command:
      - ouro-toolchain
      - install
      - httpx
`
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	manifest, err := LoadManifest(path)
	if err != nil {
		t.Fatalf("load manifest: %v", err)
	}
	if len(manifest.Dependencies) != 2 {
		t.Fatalf("expected two dependencies, got %#v", manifest.Dependencies)
	}
	dependency := manifest.Dependencies[1]
	if dependency.Name != "httpx" || dependency.Kind != "binary" || dependency.Version != "1.3.0" {
		t.Fatalf("unexpected dependency: %#v", dependency)
	}
	if len(dependency.DependsOn) != 1 || dependency.DependsOn[0] != "runtime-base" {
		t.Fatalf("expected dependency depends_on to be parsed, got %#v", dependency.DependsOn)
	}
	if strings.Join(dependency.HealthCommand, " ") != "httpx -version" {
		t.Fatalf("expected dependency health command, got %#v", dependency.HealthCommand)
	}
	if dependency.InstallHint == "" {
		t.Fatalf("expected install hint")
	}
	if strings.Join(dependency.InstallCommand, " ") != "ouro-toolchain install httpx" {
		t.Fatalf("expected dependency install command, got %#v", dependency.InstallCommand)
	}
}

func TestLoadManifestParsesDependencyProvenanceAndUpdateMetadata(t *testing.T) {
	path := filepath.Join(t.TempDir(), "manifest.yaml")
	data := `name: web-api-pack
version: "0.1.0"
capabilities:
  - web.discovery
runner: native
parser: web_parser
healthcheck: built_in
dependencies:
  - name: ouro-httpx-worker
    kind: container_image
    version: "1.0.0"
    latest_version: "1.1.0"
    source: ghcr.io/ouroboros/httpx-worker
    image_digest: sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
`
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	manifest, err := LoadManifest(path)
	if err != nil {
		t.Fatalf("load manifest: %v", err)
	}
	dependency := manifest.Dependencies[0]
	if dependency.LatestVersion != "1.1.0" || dependency.Source != "ghcr.io/ouroboros/httpx-worker" {
		t.Fatalf("expected update/source metadata, got %#v", dependency)
	}
	if dependency.ImageDigest != "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" {
		t.Fatalf("expected image digest, got %#v", dependency)
	}
}

func TestLoadManifestParsesDependencyMetadataSource(t *testing.T) {
	path := filepath.Join(t.TempDir(), "manifest.yaml")
	data := `name: web-api-pack
version: "0.1.0"
capabilities:
  - web.discovery
runner: native
parser: web_parser
healthcheck: built_in
dependencies:
  - name: ouro-httpx-worker
    kind: container_image
    version: "1.0.0"
    source: ghcr.io/ouroboros/httpx-worker
    image_digest: sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
    metadata_source: updates/httpx-worker.json
`
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	manifest, err := LoadManifest(path)
	if err != nil {
		t.Fatalf("load manifest: %v", err)
	}
	dependency := manifest.Dependencies[0]
	if dependency.MetadataSource != "updates/httpx-worker.json" {
		t.Fatalf("expected metadata source, got %#v", dependency)
	}
	if manifest.Path != path {
		t.Fatalf("expected manifest path to be retained for relative metadata, got %#v", manifest.Path)
	}
}

func TestValidateManifestRejectsInvalidDependencies(t *testing.T) {
	manifest := validManifest()
	manifest.Dependencies = []Dependency{{Kind: "binary"}}
	err := ValidateManifest(manifest)
	if err == nil || !strings.Contains(err.Error(), "dependency 0 name") {
		t.Fatalf("expected dependency name validation error, got %v", err)
	}

	manifest.Dependencies = []Dependency{{Name: "httpx", Kind: "binary", HealthCommand: []string{"httpx", ""}}}
	err = ValidateManifest(manifest)
	if err == nil || !strings.Contains(err.Error(), "dependency 0 health_command 1") {
		t.Fatalf("expected dependency health command validation error, got %v", err)
	}

	manifest.Dependencies = []Dependency{{Name: "httpx", Kind: "binary", InstallCommand: []string{"ouro-toolchain", ""}}}
	err = ValidateManifest(manifest)
	if err == nil || !strings.Contains(err.Error(), "dependency 0 install_command 1") {
		t.Fatalf("expected dependency install command validation error, got %v", err)
	}

	manifest.Dependencies = []Dependency{
		{Name: "httpx", Kind: "binary", DependsOn: []string{""}},
	}
	err = ValidateManifest(manifest)
	if err == nil || !strings.Contains(err.Error(), "dependency 0 depends_on 0 is empty") {
		t.Fatalf("expected empty depends_on validation error, got %v", err)
	}

	manifest.Dependencies = []Dependency{
		{Name: "httpx", Kind: "binary", DependsOn: []string{"missing-runtime"}},
	}
	err = ValidateManifest(manifest)
	if err == nil || !strings.Contains(err.Error(), "depends_on missing-runtime is not declared") {
		t.Fatalf("expected unknown depends_on validation error, got %v", err)
	}

	manifest.Dependencies = []Dependency{
		{Name: "httpx", Kind: "binary", DependsOn: []string{"httpx"}},
	}
	err = ValidateManifest(manifest)
	if err == nil || !strings.Contains(err.Error(), "depends_on itself") {
		t.Fatalf("expected self dependency validation error, got %v", err)
	}

	manifest.Dependencies = []Dependency{
		{Name: "runtime-base", Kind: "binary", DependsOn: []string{"httpx"}},
		{Name: "httpx", Kind: "binary", DependsOn: []string{"runtime-base"}},
	}
	err = ValidateManifest(manifest)
	if err == nil || !strings.Contains(err.Error(), "dependency cycle") {
		t.Fatalf("expected dependency cycle validation error, got %v", err)
	}
}

func TestValidateManifestRejectsRequiredContainerImageWithoutDigest(t *testing.T) {
	manifest := validManifest()
	manifest.Dependencies = []Dependency{{Name: "ouro-httpx-worker", Kind: "container_image", Version: "1.0.0"}}
	err := ValidateManifest(manifest)
	if err == nil || !strings.Contains(err.Error(), "image_digest") {
		t.Fatalf("expected image_digest validation error, got %v", err)
	}
}

func TestValidateManifestRejectsNegativeRunTimeoutMS(t *testing.T) {
	manifest := validManifest()
	manifest.RunTimeoutMS = -1

	err := ValidateManifest(manifest)
	if err == nil {
		t.Fatalf("expected negative run timeout to fail")
	}
	if !strings.Contains(err.Error(), "run_timeout_ms") {
		t.Fatalf("expected run_timeout_ms validation error, got %v", err)
	}
}

func assertToolDependencies(t *testing.T, manifest Manifest, names []string) {
	t.Helper()
	if manifest.Name == "" {
		t.Fatalf("expected manifest to be present")
	}
	byName := map[string]Dependency{}
	for _, dependency := range manifest.Dependencies {
		byName[dependency.Name] = dependency
	}
	for _, name := range names {
		dependency, ok := byName[name]
		if !ok {
			t.Fatalf("expected %s dependency in %s, got %#v", name, manifest.Name, manifest.Dependencies)
		}
		if dependency.Kind == "" {
			t.Fatalf("expected %s dependency kind", name)
		}
		if len(dependency.HealthCommand) == 0 {
			t.Fatalf("expected %s health command", name)
		}
		if dependency.InstallHint == "" {
			t.Fatalf("expected %s install hint", name)
		}
		if len(dependency.InstallCommand) == 0 {
			t.Fatalf("expected %s install command", name)
		}
		if len(dependency.InstallCommand) < 3 || dependency.InstallCommand[0] != "ouro-toolchain" || dependency.InstallCommand[1] != "install" || dependency.InstallCommand[2] != name {
			t.Fatalf("expected controlled installer command for %s, got %#v", name, dependency.InstallCommand)
		}
	}
}
