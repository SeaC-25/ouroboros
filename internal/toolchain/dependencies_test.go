package toolchain

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCheckDependenciesReportsReadyBinaryDependency(t *testing.T) {
	probe := filepath.Join(t.TempDir(), "dep-probe")
	if err := os.WriteFile(probe, []byte("#!/bin/sh\nprintf 'dep-1.2.3\\n'\n"), 0o755); err != nil {
		t.Fatalf("write dependency probe: %v", err)
	}
	manifest := validManifest()
	manifest.Dependencies = []Dependency{
		{
			Name:          "dep-probe",
			Kind:          "binary",
			Version:       "1.2.3",
			HealthCommand: []string{probe, "--version"},
		},
	}

	reports := CheckDependencies(manifest, DefaultDependencyRunner{})

	if len(reports) != 1 {
		t.Fatalf("expected one dependency report, got %#v", reports)
	}
	report := reports[0]
	if report.State != HealthReady || report.InstalledVersion != "dep-1.2.3" || !report.Required {
		t.Fatalf("expected ready required dependency, got %#v", report)
	}
}

func TestCheckDependenciesReportsMissingRequiredBinaryDependency(t *testing.T) {
	manifest := validManifest()
	manifest.Dependencies = []Dependency{
		{
			Name:           filepath.Join(t.TempDir(), "missing-dependency"),
			Kind:           "binary",
			InstallHint:    "install missing dependency",
			InstallCommand: []string{"ouro-toolchain", "install", "missing-dependency"},
		},
	}

	reports := CheckDependencies(manifest, DefaultDependencyRunner{})

	if len(reports) != 1 {
		t.Fatalf("expected one dependency report, got %#v", reports)
	}
	report := reports[0]
	if report.State != HealthMissing || !report.Required || report.InstallHint == "" || len(report.InstallCommand) == 0 {
		t.Fatalf("expected missing required dependency, got %#v", report)
	}
}

func TestPlanDependencyInstallsIncludesMissingRequiredDependency(t *testing.T) {
	manifest := validManifest()
	manifest.Dependencies = []Dependency{
		{
			Name:           "httpx",
			Kind:           "binary",
			InstallHint:    "install bundled httpx pack",
			InstallCommand: []string{"ouro-toolchain", "install", "httpx"},
		},
	}
	reports := []DependencyReport{
		{
			Name:           "httpx",
			Kind:           "binary",
			Required:       true,
			State:          HealthMissing,
			InstallHint:    "install bundled httpx pack",
			InstallCommand: []string{"ouro-toolchain", "install", "httpx"},
		},
	}

	plan := PlanDependencyInstalls(manifest, reports)

	if len(plan) != 1 {
		t.Fatalf("expected one install step, got %#v", plan)
	}
	step := plan[0]
	if step.Name != "httpx" || step.State != HealthMissing || !step.Required {
		t.Fatalf("unexpected install step identity: %#v", step)
	}
	if strings.Join(step.Command, " ") != "ouro-toolchain install httpx" {
		t.Fatalf("expected install command in plan, got %#v", step.Command)
	}
	if step.Hint != "install bundled httpx pack" {
		t.Fatalf("expected install hint in plan, got %#v", step)
	}
}

func TestPlanDependencyInstallsOrdersTransitiveMissingDependencies(t *testing.T) {
	manifest := validManifest()
	manifest.Dependencies = []Dependency{
		{
			Name:           "a-httpx",
			Kind:           "binary",
			DependsOn:      []string{"z-runtime"},
			InstallCommand: []string{"ouro-toolchain", "install", "a-httpx"},
		},
		{
			Name:           "z-runtime",
			Kind:           "binary",
			InstallCommand: []string{"ouro-toolchain", "install", "z-runtime"},
		},
	}
	reports := []DependencyReport{
		{
			Name:           "a-httpx",
			Kind:           "binary",
			Required:       true,
			State:          HealthMissing,
			DependsOn:      []string{"z-runtime"},
			InstallCommand: []string{"ouro-toolchain", "install", "a-httpx"},
		},
		{
			Name:           "z-runtime",
			Kind:           "binary",
			Required:       true,
			State:          HealthMissing,
			InstallCommand: []string{"ouro-toolchain", "install", "z-runtime"},
		},
	}

	plan := PlanDependencyInstalls(manifest, reports)

	if len(plan) != 2 {
		t.Fatalf("expected two install steps, got %#v", plan)
	}
	if plan[0].Name != "z-runtime" || plan[1].Name != "a-httpx" {
		t.Fatalf("expected transitive prerequisite before dependent, got %#v", plan)
	}
	if len(plan[1].DependsOn) != 1 || plan[1].DependsOn[0] != "z-runtime" {
		t.Fatalf("expected dependent step to carry unresolved depends_on metadata, got %#v", plan[1])
	}
}

func TestPlanDependencyInstallsOmitsReadyTransitiveDependenciesFromDependsOn(t *testing.T) {
	manifest := validManifest()
	manifest.Dependencies = []Dependency{
		{
			Name:           "a-httpx",
			Kind:           "binary",
			DependsOn:      []string{"z-runtime"},
			InstallCommand: []string{"ouro-toolchain", "install", "a-httpx"},
		},
		{
			Name:           "z-runtime",
			Kind:           "binary",
			InstallCommand: []string{"ouro-toolchain", "install", "z-runtime"},
		},
	}
	reports := []DependencyReport{
		{
			Name:           "a-httpx",
			Kind:           "binary",
			Required:       true,
			State:          HealthMissing,
			DependsOn:      []string{"z-runtime"},
			InstallCommand: []string{"ouro-toolchain", "install", "a-httpx"},
		},
		{
			Name:     "z-runtime",
			Kind:     "binary",
			Required: true,
			State:    HealthReady,
		},
	}

	plan := PlanDependencyInstalls(manifest, reports)

	if len(plan) != 1 || plan[0].Name != "a-httpx" {
		t.Fatalf("expected only missing dependent install step, got %#v", plan)
	}
	if len(plan[0].DependsOn) != 0 {
		t.Fatalf("ready dependency should not block install step, got %#v", plan[0])
	}
}

func TestApplyDependencyHealthDowngradesRequiredMissingDependency(t *testing.T) {
	health := HealthReport{ManifestName: "web-api-pack", State: HealthReady, Message: "manifest ready"}
	dependencies := []DependencyReport{
		{Name: "httpx", State: HealthMissing, Required: true, Message: "not found"},
	}

	health = ApplyDependencyHealth(health, dependencies)

	if health.State != HealthMissing {
		t.Fatalf("expected required missing dependency to downgrade health, got %#v", health)
	}
}

func TestApplyDependencyHealthIgnoresOptionalMissingDependency(t *testing.T) {
	health := HealthReport{ManifestName: "web-api-pack", State: HealthReady, Message: "manifest ready"}
	dependencies := []DependencyReport{
		{Name: "optional-wordlist", State: HealthMissing, Required: false, Message: "not found"},
	}

	health = ApplyDependencyHealth(health, dependencies)

	if health.State != HealthReady {
		t.Fatalf("expected optional missing dependency to keep health ready, got %#v", health)
	}
}

func TestCheckDependenciesReportsContainerImageProvenanceAndUpdate(t *testing.T) {
	manifest := validManifest()
	manifest.Dependencies = []Dependency{
		{
			Name:          "ouro-httpx-worker",
			Kind:          "container_image",
			Version:       "1.0.0",
			LatestVersion: "1.1.0",
			Source:        "ghcr.io/ouroboros/httpx-worker",
			ImageDigest:   "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		},
	}

	reports := CheckDependencies(manifest, DefaultDependencyRunner{})

	if len(reports) != 1 {
		t.Fatalf("expected one dependency report, got %#v", reports)
	}
	report := reports[0]
	if report.State != HealthUpdateAvailable {
		t.Fatalf("expected update available for older image version, got %#v", report)
	}
	if report.Source != "ghcr.io/ouroboros/httpx-worker" || report.ImageDigest == "" {
		t.Fatalf("expected provenance fields in report, got %#v", report)
	}
}

func TestCheckDependenciesVerifiesContainerImageDigestFromHealthCommand(t *testing.T) {
	digest := "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	probe := filepath.Join(t.TempDir(), "image-inspect")
	if err := os.WriteFile(probe, []byte("#!/bin/sh\nprintf 'ghcr.io/ouroboros/httpx-worker@"+digest+"\\n'\n"), 0o755); err != nil {
		t.Fatalf("write image probe: %v", err)
	}
	manifest := validManifest()
	manifest.Dependencies = []Dependency{
		{
			Name:          "ouro-httpx-worker",
			Kind:          "container_image",
			Version:       "1.0.0",
			Source:        "ghcr.io/ouroboros/httpx-worker",
			ImageDigest:   digest,
			HealthCommand: []string{probe, "image", "inspect", "ghcr.io/ouroboros/httpx-worker"},
		},
	}

	reports := CheckDependencies(manifest, DefaultDependencyRunner{})

	if len(reports) != 1 {
		t.Fatalf("expected one dependency report, got %#v", reports)
	}
	report := reports[0]
	if report.State != HealthReady {
		t.Fatalf("expected verified digest to be ready, got %#v", report)
	}
	if !strings.Contains(report.InstalledVersion, digest) || !strings.Contains(report.Message, "digest verified") {
		t.Fatalf("expected verified digest details, got %#v", report)
	}
}

func TestCheckDependenciesReportsContainerImageDigestMismatch(t *testing.T) {
	expected := "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	actual := "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	probe := filepath.Join(t.TempDir(), "image-inspect")
	if err := os.WriteFile(probe, []byte("#!/bin/sh\nprintf 'ghcr.io/ouroboros/httpx-worker@"+actual+"\\n'\n"), 0o755); err != nil {
		t.Fatalf("write image probe: %v", err)
	}
	manifest := validManifest()
	manifest.Dependencies = []Dependency{
		{
			Name:          "ouro-httpx-worker",
			Kind:          "container_image",
			Version:       "1.0.0",
			Source:        "ghcr.io/ouroboros/httpx-worker",
			ImageDigest:   expected,
			HealthCommand: []string{probe, "image", "inspect", "ghcr.io/ouroboros/httpx-worker"},
		},
	}

	reports := CheckDependencies(manifest, DefaultDependencyRunner{})

	if len(reports) != 1 {
		t.Fatalf("expected one dependency report, got %#v", reports)
	}
	report := reports[0]
	if report.State != HealthFailed {
		t.Fatalf("expected digest mismatch to fail, got %#v", report)
	}
	if !strings.Contains(report.Message, "digest mismatch") || !strings.Contains(report.Message, expected) || !strings.Contains(report.Message, actual) {
		t.Fatalf("expected digest mismatch details, got %#v", report)
	}
}

func TestCheckDependenciesWithMetadataProviderReportsRemoteUpdate(t *testing.T) {
	pinned := "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	latest := "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	manifest := validManifest()
	manifest.Dependencies = []Dependency{
		{
			Name:           "ouro-httpx-worker",
			Kind:           "container_image",
			Version:        "1.0.0",
			Source:         "ghcr.io/ouroboros/httpx-worker",
			ImageDigest:    pinned,
			MetadataSource: "fixture://httpx-worker",
		},
	}
	provider := fakeMetadataProvider{
		metadata: DependencyMetadata{
			LatestVersion: "1.2.0",
			ImageDigest:   latest,
			Source:        "ghcr.io/ouroboros/httpx-worker",
			Message:       "published 2026-05-29",
		},
	}

	reports := CheckDependenciesWithMetadata(context.Background(), manifest, DefaultDependencyRunner{}, provider)

	if len(reports) != 1 {
		t.Fatalf("expected one dependency report, got %#v", reports)
	}
	report := reports[0]
	if report.State != HealthUpdateAvailable {
		t.Fatalf("expected remote metadata update_available, got %#v", report)
	}
	if report.LatestVersion != "1.2.0" || report.LatestImageDigest != latest || report.ImageDigest != pinned {
		t.Fatalf("expected latest version/digest from metadata and pinned digest retained, got %#v", report)
	}
	if report.MetadataSource != "fixture://httpx-worker" || !strings.Contains(report.Message, "published") {
		t.Fatalf("expected metadata source/message in report, got %#v", report)
	}
}

func TestDefaultMetadataProviderReadsRelativeFileMetadata(t *testing.T) {
	pinned := "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	latest := "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "updates"), 0o755); err != nil {
		t.Fatalf("create updates dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "updates", "httpx-worker.json"), []byte(`{
  "latest_version": "1.2.0",
  "image_digest": "`+latest+`",
  "source": "ghcr.io/ouroboros/httpx-worker",
  "message": "fixture metadata"
}`), 0o644); err != nil {
		t.Fatalf("write metadata fixture: %v", err)
	}
	manifest := validManifest()
	manifest.Path = filepath.Join(root, "manifest.yaml")
	manifest.Dependencies = []Dependency{
		{
			Name:           "ouro-httpx-worker",
			Kind:           "container_image",
			Version:        "1.0.0",
			Source:         "ghcr.io/ouroboros/httpx-worker",
			ImageDigest:    pinned,
			MetadataSource: "updates/httpx-worker.json",
		},
	}

	reports := CheckDependenciesWithMetadata(context.Background(), manifest, DefaultDependencyRunner{}, DefaultMetadataProvider{})

	if len(reports) != 1 {
		t.Fatalf("expected one dependency report, got %#v", reports)
	}
	report := reports[0]
	if report.State != HealthUpdateAvailable || report.LatestVersion != "1.2.0" || report.LatestImageDigest != latest {
		t.Fatalf("expected relative metadata fixture update, got %#v", report)
	}
	if !strings.Contains(report.Message, "fixture metadata") {
		t.Fatalf("expected metadata message in report, got %#v", report)
	}
}

type fakeMetadataProvider struct {
	metadata DependencyMetadata
	err      error
}

func (p fakeMetadataProvider) FetchDependencyMetadata(context.Context, Manifest, Dependency) (DependencyMetadata, error) {
	return p.metadata, p.err
}
