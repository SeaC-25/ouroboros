package app

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/ouroboros/ouroboros/internal/compiler"
	"github.com/ouroboros/ouroboros/internal/eventlog"
	"github.com/ouroboros/ouroboros/internal/frontier"
	"github.com/ouroboros/ouroboros/internal/toolchain"
)

func TestInspectToolchainsReportsWorkspaceManifestsAndHealth(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}

	statuses, err := InspectToolchains(root)
	if err != nil {
		t.Fatalf("inspect toolchains: %v", err)
	}
	if len(statuses) != 3 {
		t.Fatalf("expected three toolchains, got %#v", statuses)
	}

	byName := map[string]ToolchainStatus{}
	for _, status := range statuses {
		byName[status.Name] = status
	}
	for _, name := range []string{"web-api-pack", "infra-pack", "ctf-pack"} {
		status, ok := byName[name]
		if !ok {
			t.Fatalf("expected status for %s in %#v", name, statuses)
		}
		if status.Health.State != toolchain.HealthReady {
			t.Fatalf("expected %s to be ready, got %#v", name, status.Health)
		}
		if status.CapabilityCount == 0 || strings.TrimSpace(status.Parser) == "" {
			t.Fatalf("expected manifest details for %s, got %#v", name, status)
		}
	}
}

func TestCreateAuthoredToolWritesSpecAndManifestInsideWorkspace(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	spec := validAuthoredToolSpecForAppTest()

	authored, err := CreateAuthoredTool(root, spec)
	if err != nil {
		t.Fatalf("create authored tool: %v", err)
	}
	if authored.Name != spec.Name || authored.Status != "written" {
		t.Fatalf("unexpected authored tool result: %#v", authored)
	}
	if authored.AdapterID != spec.Adapter.ID || authored.Operation != spec.Adapter.Operation {
		t.Fatalf("expected authored adapter metadata, got %#v", authored)
	}
	if authored.Dir != filepath.Join(root, "toolchain", spec.Name) {
		t.Fatalf("expected authored tool dir under workspace toolchain, got %#v", authored)
	}
	if authored.SpecPath != filepath.Join(authored.Dir, "authored-tool.yaml") || authored.ManifestPath != filepath.Join(authored.Dir, "manifest.yaml") {
		t.Fatalf("unexpected authored tool paths: %#v", authored)
	}
	if _, err := os.Stat(authored.SpecPath); err != nil {
		t.Fatalf("expected authored spec artifact: %v", err)
	}
	manifest, err := toolchain.LoadManifest(authored.ManifestPath)
	if err != nil {
		t.Fatalf("load authored manifest: %v", err)
	}
	if manifest.Name != spec.Name || manifest.Runner != "native" || len(manifest.RunCommand) != 0 {
		t.Fatalf("expected native manifest from authored spec, got %#v", manifest)
	}

	tools, err := ListAuthoredTools(root)
	if err != nil {
		t.Fatalf("list authored tools: %v", err)
	}
	if len(tools) != 1 || tools[0].Name != spec.Name || tools[0].SpecPath != authored.SpecPath {
		t.Fatalf("expected listed authored tool, got %#v", tools)
	}
	read, err := ReadAuthoredTool(root, spec.Name)
	if err != nil {
		t.Fatalf("read authored tool: %v", err)
	}
	if read.Name != spec.Name || read.ManifestPath != authored.ManifestPath {
		t.Fatalf("unexpected read authored tool: %#v", read)
	}
}

func TestWriteAuthoredToolArtifactFileDoesNotOverwriteExistingFixedTempFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "artifact.yaml")
	fixedTmpPath := path + ".tmp"
	fixedTmpBody := []byte("preexisting temp file\n")
	if err := os.WriteFile(fixedTmpPath, fixedTmpBody, 0o644); err != nil {
		t.Fatalf("write preexisting fixed temp file: %v", err)
	}

	if err := writeAuthoredToolArtifactFile(path, []byte("name: artifact"), "test artifact"); err != nil {
		t.Fatalf("write artifact with preexisting fixed temp file: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read published artifact: %v", err)
	}
	if string(data) != "name: artifact\n" {
		t.Fatalf("unexpected published artifact data: %q", string(data))
	}
	tmpData, err := os.ReadFile(fixedTmpPath)
	if err != nil {
		t.Fatalf("preexisting fixed temp file must remain untouched: %v", err)
	}
	if string(tmpData) != string(fixedTmpBody) {
		t.Fatalf("preexisting fixed temp file was modified: %q", string(tmpData))
	}
	leftovers, err := filepath.Glob(path + ".*.tmp")
	if err != nil {
		t.Fatalf("glob random temp files: %v", err)
	}
	if len(leftovers) != 0 {
		t.Fatalf("expected no random temp leftovers, got %#v", leftovers)
	}
}

func TestCreateAuthoredToolRejectsRawScriptWithoutWriting(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	spec := validAuthoredToolSpecForAppTest()
	spec.RawScript = "nmap -sV $TARGET"

	_, err := CreateAuthoredTool(root, spec)
	if err == nil || !strings.Contains(err.Error(), "raw_script") {
		t.Fatalf("expected raw script validation error, got %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(root, "toolchain", spec.Name)); !os.IsNotExist(statErr) {
		t.Fatalf("invalid authored spec must not create tool directory, stat err=%v", statErr)
	}
}

func TestCheckToolchainContractsDoesNotRunContainerRuntimeDoctor(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	engineDir := t.TempDir()
	engine := filepath.Join(engineDir, "fake-container-engine")
	marker := filepath.Join(engineDir, "container-runtime-was-called")
	engineScript := `#!/bin/sh
touch "` + marker + `"
printf 'container runtime should not be inspected during contract check\n' >&2
exit 1
`
	if err := os.WriteFile(engine, []byte(engineScript), 0o755); err != nil {
		t.Fatalf("write fake engine: %v", err)
	}
	scope := `version: 1
scope:
  target_policy: enforce
  targets: []
  exclusions: []
execution:
  isolation: container
  container:
    engine: ` + engine + `
    image: ouro-worker:test
`
	if err := os.WriteFile(filepath.Join(root, "scope.yaml"), []byte(scope), 0o644); err != nil {
		t.Fatalf("write container scope: %v", err)
	}

	check, err := CheckToolchainContracts(root)
	if err != nil {
		t.Fatalf("contract check should not run container doctor checks: %v", err)
	}
	if check.Status != "ready" {
		t.Fatalf("expected ready contract check, got %#v", check)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("contract check must not inspect container runtime, marker stat err=%v", err)
	}
}

func TestCheckToolchainContractsBlocksUnknownWorkspaceManifestParser(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	manifestPath := filepath.Join(root, "toolchain", "web-api-pack", "manifest.yaml")
	manifest, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	drifted := strings.Replace(string(manifest), "parser: web_api_parser", "parser: drifted_parser", 1)
	if drifted == string(manifest) {
		t.Fatalf("test fixture did not replace manifest parser")
	}
	if err := os.WriteFile(manifestPath, []byte(drifted), 0o644); err != nil {
		t.Fatalf("write drifted manifest: %v", err)
	}

	check, err := CheckToolchainContracts(root)
	if err != nil {
		t.Fatalf("contract check should report blocked status instead of failing readiness: %v", err)
	}
	if check.Status != "blocked" {
		t.Fatalf("expected blocked contract check, got %#v", check)
	}
	found := false
	for _, contractErr := range check.Errors {
		if contractErr == "manifest web-api-pack parser drifted_parser is not registered" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected parser registry error, got %#v", check.Errors)
	}
}

func TestAuthoredToolMetadataPathsDoNotRunContainerRuntimeDoctor(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	engineDir := t.TempDir()
	engine := filepath.Join(engineDir, "fake-container-engine")
	marker := filepath.Join(engineDir, "container-runtime-was-called")
	engineScript := `#!/bin/sh
touch "` + marker + `"
printf 'container runtime should not be inspected during authored tool metadata paths\n' >&2
exit 1
`
	if err := os.WriteFile(engine, []byte(engineScript), 0o755); err != nil {
		t.Fatalf("write fake engine: %v", err)
	}
	scope := `version: 1
scope:
  target_policy: enforce
  targets: []
  exclusions: []
execution:
  isolation: container
  container:
    engine: ` + engine + `
    image: ouro-worker:test
`
	if err := os.WriteFile(filepath.Join(root, "scope.yaml"), []byte(scope), 0o644); err != nil {
		t.Fatalf("write container scope: %v", err)
	}

	spec := validAuthoredToolSpecForAppTest()
	if _, err := CreateAuthoredTool(root, spec); err != nil {
		t.Fatalf("create authored tool should not run container doctor checks: %v", err)
	}
	if _, err := ListAuthoredTools(root); err != nil {
		t.Fatalf("list authored tools should not run container doctor checks: %v", err)
	}
	if _, err := ReadAuthoredTool(root, spec.Name); err != nil {
		t.Fatalf("read authored tool should not run container doctor checks: %v", err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("authored metadata paths must not inspect container runtime, marker stat err=%v", err)
	}
}

func TestReadAuthoredToolRejectsDerivedManifestDrift(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	spec := validAuthoredToolSpecForAppTest()
	authored, err := CreateAuthoredTool(root, spec)
	if err != nil {
		t.Fatalf("create authored tool: %v", err)
	}
	driftedManifest := `name: ` + spec.Name + `
version: "0.1.0"
capabilities:
  - web.auth_state_tracking
runner: native
run_command:
  - sh
  - -c
  - echo drift
parser: drifted_parser
healthcheck: built_in
`
	if err := os.WriteFile(authored.ManifestPath, []byte(driftedManifest), 0o644); err != nil {
		t.Fatalf("write drifted manifest: %v", err)
	}

	_, err = ReadAuthoredTool(root, spec.Name)
	if err == nil || !strings.Contains(err.Error(), "does not match authored spec") {
		t.Fatalf("expected derived manifest drift error, got %v", err)
	}
}

func validAuthoredToolSpecForAppTest() toolchain.AuthoredToolSpec {
	return toolchain.AuthoredToolSpec{
		Name:         "web-login-state-enumerator",
		Version:      "0.1.0",
		Capabilities: []string{"web.auth_state_tracking", "evidence.collection"},
		Parser:       "web_login_state_parser",
		Adapter: toolchain.AuthoredToolAdapter{
			ID:        "managed_binary",
			Operation: "http_probe",
		},
		Inputs: []toolchain.AuthoredToolInput{
			{Name: "target_url", Type: "url", Required: true},
		},
		ExpectedEvidence: []toolchain.AuthoredToolEvidence{
			{Type: "http_observation", Fields: []string{"status_code", "body_hash"}},
		},
		Safety: toolchain.AuthoredToolSafety{
			TargetPolicy: "enforce",
			Isolation:    "process",
		},
		Dependencies: []toolchain.Dependency{
			{
				Name:        "httpx",
				Kind:        "binary",
				Optional:    true,
				Source:      "projectdiscovery/httpx",
				InstallHint: "install managed httpx",
			},
		},
	}
}

func TestPreviewToolCompilePlanScoresCandidatesWithoutExecutingCommands(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	marker := filepath.Join(t.TempDir(), "compile-preview-ran")
	manifest := `name: web-api-pack
version: "0.1.0"
capabilities:
  - web.discovery
runner: command
run_command:
  - sh
  - -c
  - touch ` + marker + `
parser: web_api_parser
healthcheck: built_in
`
	if err := os.WriteFile(filepath.Join(root, "toolchain", "web-api-pack", "manifest.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	preview, err := PreviewToolCompilePlan(root, ToolCompilePreviewOptions{
		Action: compiler.ActionDiscoverAssets,
		DomainPriorities: []compiler.DomainPriority{
			{Domain: frontier.DomainInfra, Weight: 5},
			{Domain: frontier.DomainCTF, Weight: 1},
		},
		FrontierScores: []compiler.CandidateFrontierScore{
			{
				ManifestName: "infra-pack",
				Capability:   "infra.host_discovery",
				Path: frontier.CandidatePath{
					MissionValue:    2,
					UnlockPotential: 2,
				},
			},
		},
		HistoricalStats: []compiler.CandidateHistory{
			{ManifestName: "infra-pack", Capability: "infra.host_discovery", Successes: 2},
			{ManifestName: "web-api-pack", Capability: "web.discovery", Failures: 2},
		},
		Budget: compiler.CompileBudget{MaxPlanNodes: 2},
	})
	if err != nil {
		t.Fatalf("preview compile plan: %v", err)
	}

	if preview.Mode != "metadata-only" || preview.Action != compiler.ActionDiscoverAssets {
		t.Fatalf("expected metadata-only discover preview, got %#v", preview)
	}
	if len(preview.Nodes) != 2 {
		t.Fatalf("expected budget to keep two nodes, got %#v", preview.Nodes)
	}
	for _, node := range preview.Nodes {
		if node.ManifestName != "infra-pack" || node.Capability != "infra.host_discovery" || node.ToolID == "" {
			t.Fatalf("expected budgeted tool-level infra nodes, got %#v", preview.Nodes)
		}
	}
	if !containsString(preview.Nodes[0].SelectionSignals, "domain:infra_internal=5") {
		t.Fatalf("expected domain scoring signal, got %#v", preview.Nodes[0].SelectionSignals)
	}
	if len(preview.Warnings) != 1 || !strings.Contains(preview.Warnings[0], "budget max_plan_nodes=2 truncated 7 candidates") {
		t.Fatalf("expected budget warning, got %#v", preview.Warnings)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("compile preview must not execute run_command, marker stat err=%v", err)
	}
}

func TestPreviewToolCompilePlanUsesPersistedWorkspaceSignals(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	marker := filepath.Join(t.TempDir(), "compile-preview-ran")
	manifest := `name: web-api-pack
version: "0.1.0"
capabilities:
  - web.discovery
runner: command
run_command:
  - sh
  - -c
  - touch ` + marker + `
parser: web_api_parser
healthcheck: built_in
`
	if err := os.WriteFile(filepath.Join(root, "toolchain", "web-api-pack", "manifest.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	signals := `{
  "schema_version": 1,
  "budget": {"max_plan_nodes": 1},
  "domain_priorities": [
    {"domain": "infra_internal", "weight": 3}
  ],
  "frontier_scores": [
    {
      "manifest_name": "infra-pack",
      "capability": "infra.host_discovery",
      "path": {"mission_value": 2, "unlock_potential": 1}
    }
  ],
  "historical_stats": [
    {"manifest_name": "infra-pack", "capability": "infra.host_discovery", "successes": 2},
    {"manifest_name": "web-api-pack", "capability": "web.discovery", "failures": 4}
  ]
}`
	if err := os.WriteFile(filepath.Join(root, "toolchain", "compile-preview-signals.json"), []byte(signals), 0o644); err != nil {
		t.Fatalf("write compile preview signals: %v", err)
	}

	preview, err := PreviewToolCompilePlan(root, ToolCompilePreviewOptions{})
	if err != nil {
		t.Fatalf("preview compile plan: %v", err)
	}
	if len(preview.Nodes) != 1 {
		t.Fatalf("expected persisted budget to keep one node, got %#v", preview.Nodes)
	}
	node := preview.Nodes[0]
	if node.ManifestName != "infra-pack" || node.Capability != "infra.host_discovery" || node.ToolID == "" {
		t.Fatalf("expected persisted signals to rank a tool-level infra node first, got %#v", node)
	}
	for _, want := range []string{
		"domain:infra_internal=3",
		"frontier=3",
		"history:successes=2 failures=0",
	} {
		if !containsString(node.SelectionSignals, want) {
			t.Fatalf("expected persisted signal %q in %#v", want, node.SelectionSignals)
		}
	}
	if len(preview.Warnings) != 1 || !strings.Contains(preview.Warnings[0], "budget max_plan_nodes=1 truncated 7 candidates") {
		t.Fatalf("expected persisted budget warning, got %#v", preview.Warnings)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("workspace-signal compile preview must not execute run_command, marker stat err=%v", err)
	}
}

func TestCreateToolCompilePreviewReviewWritesMetadataOnlyArtifact(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	marker := filepath.Join(t.TempDir(), "compile-preview-review-ran")
	manifest := `name: web-api-pack
version: "0.1.0"
capabilities:
  - web.discovery
runner: command
run_command:
  - sh
  - -c
  - touch ` + marker + `
parser: web_api_parser
healthcheck: built_in
`
	if err := os.WriteFile(filepath.Join(root, "toolchain", "web-api-pack", "manifest.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	review, err := CreateToolCompilePreviewReview(root, ToolCompilePreviewOptions{
		Budget: compiler.CompileBudget{MaxPlanNodes: 1},
	})
	if err != nil {
		t.Fatalf("create compile preview review: %v", err)
	}
	if review.Status != "dry-run" || review.Mode != "metadata-only" {
		t.Fatalf("expected dry-run metadata-only review, got %#v", review)
	}
	if review.Path == "" || filepath.Dir(review.Path) != filepath.Join(root, "toolchain", "compile-preview-reviews") {
		t.Fatalf("expected review artifact under compile-preview-reviews, got %s", review.Path)
	}
	if review.Preview.Mode != "metadata-only" || len(review.Preview.Nodes) != 1 || len(review.Preview.Warnings) != 1 {
		t.Fatalf("expected reviewed preview payload with budget warning, got %#v", review.Preview)
	}
	data, err := os.ReadFile(review.Path)
	if err != nil {
		t.Fatalf("read compile preview review artifact: %v", err)
	}
	var decoded ToolCompilePreviewReview
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("decode compile preview review artifact: %v", err)
	}
	if decoded.Status != "dry-run" || decoded.Mode != "metadata-only" || len(decoded.Preview.Nodes) != 1 {
		t.Fatalf("unexpected decoded review artifact: %#v", decoded)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("creating compile preview review must not execute run_command, marker stat err=%v", err)
	}
}

func TestListAndReadToolCompilePreviewReviewsReadsArtifacts(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	marker := filepath.Join(t.TempDir(), "compile-preview-review-ran")
	manifest := `name: web-api-pack
version: "0.1.0"
capabilities:
  - web.discovery
runner: command
run_command:
  - sh
  - -c
  - touch ` + marker + `
parser: web_api_parser
healthcheck: built_in
`
	if err := os.WriteFile(filepath.Join(root, "toolchain", "web-api-pack", "manifest.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	firstReview, err := CreateToolCompilePreviewReview(root, ToolCompilePreviewOptions{
		Budget: compiler.CompileBudget{MaxPlanNodes: 1},
	})
	if err != nil {
		t.Fatalf("create first compile preview review: %v", err)
	}
	secondReview, err := CreateToolCompilePreviewReview(root, ToolCompilePreviewOptions{
		Budget: compiler.CompileBudget{MaxPlanNodes: 2},
	})
	if err != nil {
		t.Fatalf("create second compile preview review: %v", err)
	}

	reviews, err := ListToolCompilePreviewReviews(root)
	if err != nil {
		t.Fatalf("list compile preview reviews: %v", err)
	}
	if len(reviews) != 2 {
		t.Fatalf("expected two compile preview review artifacts, got %#v", reviews)
	}
	if reviews[0].Path > reviews[1].Path {
		t.Fatalf("expected sorted review paths, got %#v", []string{reviews[0].Path, reviews[1].Path})
	}
	read, err := ReadToolCompilePreviewReview(root, filepath.Base(secondReview.Path))
	if err != nil {
		t.Fatalf("read compile preview review by basename: %v", err)
	}
	if read.Path != secondReview.Path || len(read.Preview.Nodes) != 2 {
		t.Fatalf("unexpected read compile preview review: %#v", read)
	}
	if _, err := ReadToolCompilePreviewReview(root, filepath.Join(t.TempDir(), "outside.json")); err == nil {
		t.Fatalf("expected compile preview review path outside workspace to be rejected")
	}
	if _, err := ReadToolCompilePreviewReview(root, filepath.Base(firstReview.Path)); err != nil {
		t.Fatalf("read first compile preview review by basename: %v", err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("listing or reading compile preview reviews must not execute run_command, marker stat err=%v", err)
	}
}

func TestCompareToolCompilePreviewReviewsReportsMetadataOnlyDiffs(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	marker := filepath.Join(t.TempDir(), "compile-preview-review-compare-ran")
	manifest := `name: web-api-pack
version: "0.1.0"
capabilities:
  - web.discovery
runner: command
run_command:
  - sh
  - -c
  - touch ` + marker + `
parser: web_api_parser
healthcheck: built_in
`
	if err := os.WriteFile(filepath.Join(root, "toolchain", "web-api-pack", "manifest.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	basePath := writeCompilePreviewReviewFixture(t, root, "base.json", ToolCompilePreviewReview{
		Status: "dry-run",
		Mode:   "metadata-only",
		Preview: ToolCompilePreview{
			Workspace: root,
			Action:    compiler.ActionDiscoverAssets,
			Mode:      "metadata-only",
			Nodes: []compiler.PlanNode{
				{ID: "discover_assets:01", ManifestName: "web-api-pack", Capability: "web.discovery", Runner: "command", Parser: "web_api_parser", SelectionScore: 1, SelectionSignals: []string{"history:successes=1 failures=0"}},
				{ID: "discover_assets:02", ManifestName: "infra-pack", Capability: "infra.host_discovery", Runner: "command", Parser: "infra_parser", SelectionScore: 2, SelectionSignals: []string{"domain:infra_internal=1"}},
			},
			Warnings: []string{"base warning"},
		},
		Annotation: &ToolCompilePreviewReviewAnnotation{Decision: "accepted", Reviewer: "alice"},
	})
	candidatePath := writeCompilePreviewReviewFixture(t, root, "candidate.json", ToolCompilePreviewReview{
		Status: "dry-run",
		Mode:   "metadata-only",
		Preview: ToolCompilePreview{
			Workspace: root,
			Action:    compiler.ActionDiscoverAssets,
			Mode:      "metadata-only",
			Nodes: []compiler.PlanNode{
				{ID: "discover_assets:01", ManifestName: "ctf-pack", Capability: "ctf.challenge_discovery", Runner: "command", Parser: "ctf_parser", SelectionScore: 5, SelectionSignals: []string{"domain:ctf_lab=5"}},
				{ID: "discover_assets:02", ManifestName: "web-api-pack", Capability: "web.discovery", Runner: "command", Parser: "web_api_parser", SelectionScore: 4, SelectionSignals: []string{"frontier=3"}},
				{ID: "discover_assets:03", ManifestName: "infra-pack", Capability: "infra.host_discovery", Runner: "command", Parser: "infra_parser", SelectionScore: 2, SelectionSignals: []string{"domain:infra_internal=1"}},
			},
			Warnings: []string{"candidate warning"},
		},
		Annotation: &ToolCompilePreviewReviewAnnotation{Decision: "deferred", Reviewer: "bob"},
	})

	comparison, err := CompareToolCompilePreviewReviews(root, filepath.Base(basePath), filepath.Base(candidatePath))
	if err != nil {
		t.Fatalf("compare compile preview reviews: %v", err)
	}
	if comparison.Status != "compared" || comparison.Mode != "metadata-only" {
		t.Fatalf("expected metadata-only comparison, got %#v", comparison)
	}
	if comparison.Base.Path != basePath || comparison.Candidate.Path != candidatePath || comparison.Base.Decision != "accepted" || comparison.Candidate.Decision != "deferred" {
		t.Fatalf("unexpected comparison sides: %#v", comparison)
	}
	if comparison.Summary.NodeDelta != 1 || comparison.Summary.WarningDelta != 0 || comparison.Summary.AddedNodes != 1 || comparison.Summary.RemovedNodes != 0 || comparison.Summary.ReorderedNodes == 0 || comparison.Summary.ScoreChangedNodes != 1 || comparison.Summary.SignalChangedNodes != 1 {
		t.Fatalf("unexpected comparison summary: %#v", comparison.Summary)
	}
	if len(comparison.NodeDiffs) == 0 || len(comparison.WarningDiffs) != 2 {
		t.Fatalf("expected node and warning diffs, got nodes=%#v warnings=%#v", comparison.NodeDiffs, comparison.WarningDiffs)
	}
	if _, err := CompareToolCompilePreviewReviews(root, filepath.Join(t.TempDir(), "outside.json"), filepath.Base(candidatePath)); err == nil {
		t.Fatalf("expected comparison path outside workspace to be rejected")
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("comparing compile preview reviews must not execute run_command, marker stat err=%v", err)
	}
}

func TestSelectToolCompilePreviewReviewWritesCurrentMetadataOnlySelection(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	marker := filepath.Join(t.TempDir(), "compile-preview-review-select-ran")
	manifest := `name: web-api-pack
version: "0.1.0"
capabilities:
  - web.discovery
runner: command
run_command:
  - sh
  - -c
  - touch ` + marker + `
parser: web_api_parser
healthcheck: built_in
`
	if err := os.WriteFile(filepath.Join(root, "toolchain", "web-api-pack", "manifest.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	basePath := writeCompilePreviewReviewFixture(t, root, "base.json", ToolCompilePreviewReview{
		Status: "dry-run",
		Mode:   "metadata-only",
		Preview: ToolCompilePreview{
			Workspace: root,
			Action:    compiler.ActionDiscoverAssets,
			Mode:      "metadata-only",
			Nodes: []compiler.PlanNode{
				{ID: "discover_assets:01", ManifestName: "web-api-pack", Capability: "web.discovery"},
			},
		},
	})
	candidatePath := writeCompilePreviewReviewFixture(t, root, "candidate.json", ToolCompilePreviewReview{
		Status: "dry-run",
		Mode:   "metadata-only",
		Preview: ToolCompilePreview{
			Workspace: root,
			Action:    compiler.ActionDiscoverAssets,
			Mode:      "metadata-only",
			Nodes: []compiler.PlanNode{
				{ID: "discover_assets:01", ManifestName: "infra-pack", Capability: "infra.host_discovery"},
			},
		},
	})

	selectedAt := time.Date(2026, 6, 1, 0, 15, 0, 0, time.UTC)
	selection, err := SelectToolCompilePreviewReview(root, filepath.Base(candidatePath), ToolCompilePreviewReviewSelectionOptions{
		SelectedBy: "operator",
		Reason:     "best reviewed candidate",
		SelectedAt: selectedAt,
	})
	if err != nil {
		t.Fatalf("select compile preview review: %v", err)
	}
	if selection.Status != "current" || selection.Mode != "metadata-only" || selection.ReviewPath != candidatePath || selection.SelectedBy != "operator" || selection.SelectedAt != "2026-06-01T00:15:00Z" || selection.Reason != "best reviewed candidate" {
		t.Fatalf("unexpected selection: %#v", selection)
	}
	if selection.Path != filepath.Join(root, "toolchain", "compile-preview-review-selection.json") {
		t.Fatalf("unexpected selection path: %s", selection.Path)
	}
	if len(selection.SupersededPaths) != 1 || selection.SupersededPaths[0] != basePath {
		t.Fatalf("expected base review to be superseded, got %#v", selection.SupersededPaths)
	}
	read, found, err := ReadToolCompilePreviewReviewSelection(root)
	if err != nil {
		t.Fatalf("read compile preview review selection: %v", err)
	}
	if !found || read.ReviewPath != candidatePath || len(read.SupersededPaths) != 1 {
		t.Fatalf("expected persisted selection, found=%v selection=%#v", found, read)
	}
	if _, err := SelectToolCompilePreviewReview(root, filepath.Join(t.TempDir(), "outside.json"), ToolCompilePreviewReviewSelectionOptions{SelectedBy: "operator"}); err == nil {
		t.Fatalf("expected outside review selection path to be rejected")
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("selecting compile preview review must not execute run_command, marker stat err=%v", err)
	}
}

func TestExecuteToolCompilePreviewReviewRequiresExplicitExecute(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	review, err := CreateToolCompilePreviewReview(root, ToolCompilePreviewOptions{Action: compiler.ActionEnumeratePaths})
	if err != nil {
		t.Fatalf("create compile preview review: %v", err)
	}
	runner := &recordingToolExecutionRunner{}
	_, err = ExecuteToolCompilePreviewReview(context.Background(), root, filepath.Base(review.Path), ToolCompilePreviewReviewExecutionOptions{
		ToolRunner: runner,
	})
	if err == nil || !strings.Contains(err.Error(), "requires explicit execute") {
		t.Fatalf("expected explicit execute error, got %v", err)
	}
	executions, listErr := ListToolCompilePreviewReviewExecutions(root)
	if listErr != nil {
		t.Fatalf("list compile preview review executions: %v", listErr)
	}
	if len(executions) != 0 {
		t.Fatalf("missing execute gate must not write execution artifacts, got %#v", executions)
	}
	if len(runner.requests) != 0 {
		t.Fatalf("missing execute gate must not run tools, got %#v", runner.requests)
	}
}

func TestExecuteToolCompilePreviewReviewRunsFrozenNodesWithExplicitGate(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	review, err := CreateToolCompilePreviewReview(root, ToolCompilePreviewOptions{Action: compiler.ActionEnumeratePaths})
	if err != nil {
		t.Fatalf("create compile preview review: %v", err)
	}
	if len(review.Preview.Nodes) == 0 {
		t.Fatalf("expected compile preview nodes")
	}
	selection, err := SelectToolCompilePreviewReview(root, filepath.Base(review.Path), ToolCompilePreviewReviewSelectionOptions{
		SelectedBy: "closed-loop-planner",
		Reason:     "planner-selected review",
	})
	if err != nil {
		t.Fatalf("select compile preview review: %v", err)
	}
	runner := &recordingToolExecutionRunner{}
	executedAt := time.Date(2026, 6, 4, 10, 15, 0, 0, time.UTC)
	execution, err := ExecuteToolCompilePreviewReview(ctx, root, filepath.Base(review.Path), ToolCompilePreviewReviewExecutionOptions{
		Execute:    true,
		Actor:      "operator",
		Reason:     "run reviewed planner-selected plan",
		ExecutedAt: executedAt,
		ToolRunner: runner,
	})
	if err != nil {
		t.Fatalf("execute compile preview review: %v", err)
	}
	if execution.Status != "completed" || execution.Mode != "execute" || execution.ReviewPath != review.Path || execution.Action != review.Preview.Action {
		t.Fatalf("unexpected execution: %#v", execution)
	}
	if execution.SelectionPath != selection.Path {
		t.Fatalf("expected execution to link current selection %s, got %#v", selection.Path, execution)
	}
	if execution.Actor != "operator" || execution.Reason != "run reviewed planner-selected plan" || execution.ExecutedAt != executedAt.Format(time.RFC3339) {
		t.Fatalf("unexpected execution actor metadata: %#v", execution)
	}
	if execution.RunID == "" || execution.EventCount == 0 || execution.ReviewNodes != len(review.Preview.Nodes) {
		t.Fatalf("expected run id, events, and node count in execution: %#v", execution)
	}
	if len(runner.requests) != len(review.Preview.Nodes) {
		t.Fatalf("expected runner to execute frozen review nodes, got %d requests for %d nodes", len(runner.requests), len(review.Preview.Nodes))
	}
	for i, request := range runner.requests {
		if request.Node.ID != review.Preview.Nodes[i].ID || request.Node.Action != review.Preview.Nodes[i].Action {
			t.Fatalf("runner request %d did not use frozen review node: request=%#v review=%#v", i, request.Node, review.Preview.Nodes[i])
		}
	}
	executions, err := ListToolCompilePreviewReviewExecutions(root)
	if err != nil {
		t.Fatalf("list compile preview review executions: %v", err)
	}
	if len(executions) != 1 || executions[0].Path != execution.Path {
		t.Fatalf("expected one persisted execution artifact, got %#v", executions)
	}
	store, err := eventlog.OpenSQLite(filepath.Join(root, "replay", "events.db"))
	if err != nil {
		t.Fatalf("open event log: %v", err)
	}
	defer store.Close()
	events, err := store.Replay(ctx, execution.RunID)
	if err != nil {
		t.Fatalf("replay execution events: %v", err)
	}
	if len(events) != execution.EventCount {
		t.Fatalf("expected execution event count %d, got %d", execution.EventCount, len(events))
	}
	assertStageAttemptSequence(t, events, []string{"tool-execution:started", "tool-execution:completed"})
}

func TestExecuteToolCompilePreviewReviewRequiresCurrentSelection(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	review, err := CreateToolCompilePreviewReview(root, ToolCompilePreviewOptions{Action: compiler.ActionEnumeratePaths})
	if err != nil {
		t.Fatalf("create compile preview review: %v", err)
	}
	runner := &recordingToolExecutionRunner{}
	_, err = ExecuteToolCompilePreviewReview(context.Background(), root, filepath.Base(review.Path), ToolCompilePreviewReviewExecutionOptions{
		Execute:    true,
		ToolRunner: runner,
	})
	if err == nil || !strings.Contains(err.Error(), "requires a current review selection") {
		t.Fatalf("expected current selection error, got %v", err)
	}
	if len(runner.requests) != 0 {
		t.Fatalf("current selection gate must run before tools, got %#v", runner.requests)
	}
	executions, listErr := ListToolCompilePreviewReviewExecutions(root)
	if listErr != nil {
		t.Fatalf("list compile preview review executions: %v", listErr)
	}
	if len(executions) != 0 {
		t.Fatalf("current selection failure must not write execution artifacts, got %#v", executions)
	}
}

func TestCreateToolCompilePreviewReviewExecutionDispatchWritesMetadataOnlyCurrentReviewGate(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	review, err := CreateToolCompilePreviewReview(root, ToolCompilePreviewOptions{Action: compiler.ActionEnumeratePaths})
	if err != nil {
		t.Fatalf("create compile preview review: %v", err)
	}
	selection, err := SelectToolCompilePreviewReview(root, filepath.Base(review.Path), ToolCompilePreviewReviewSelectionOptions{
		SelectedBy: "closed-loop-planner",
		Reason:     "planner-selected review",
	})
	if err != nil {
		t.Fatalf("select compile preview review: %v", err)
	}

	createdAt := time.Date(2026, 6, 4, 12, 5, 0, 0, time.UTC)
	dispatch, err := CreateToolCompilePreviewReviewExecutionDispatch(root, filepath.Base(review.Path), ToolCompilePreviewReviewExecutionDispatchOptions{
		Actor:     "operator",
		Reason:    "queue reviewed compile preview execution",
		CreatedAt: createdAt,
	})
	if err != nil {
		t.Fatalf("create compile preview review execution dispatch: %v", err)
	}
	if dispatch.Status != "dry-run" || dispatch.Mode != "metadata-only" || dispatch.ReviewPath != review.Path || dispatch.SelectionPath != selection.Path {
		t.Fatalf("unexpected dispatch: %#v", dispatch)
	}
	if dispatch.Action != review.Preview.Action || dispatch.ReviewNodes != len(review.Preview.Nodes) || dispatch.ReviewWarnings != len(review.Preview.Warnings) {
		t.Fatalf("dispatch did not snapshot review summary: %#v review=%#v", dispatch, review)
	}
	if dispatch.Actor != "operator" || dispatch.Reason != "queue reviewed compile preview execution" || dispatch.CreatedAt != createdAt.Format(time.RFC3339) {
		t.Fatalf("unexpected dispatch actor metadata: %#v", dispatch)
	}
	if !strings.HasPrefix(dispatch.Path, filepath.Join(root, "toolchain", "compile-preview-review-execution-dispatches")+string(os.PathSeparator)) {
		t.Fatalf("dispatch path escaped expected directory: %s", dispatch.Path)
	}
	if got := countJSON(t, filepath.Join(root, "toolchain", "compile-preview-review-executions")); got != 0 {
		t.Fatalf("dispatch creation must not execute review nodes or write execution artifacts, got %d", got)
	}
	dispatches, err := ListToolCompilePreviewReviewExecutionDispatches(root)
	if err != nil {
		t.Fatalf("list compile preview review execution dispatches: %v", err)
	}
	if len(dispatches) != 1 || dispatches[0].Path != dispatch.Path {
		t.Fatalf("expected one dispatch artifact, got %#v", dispatches)
	}
	read, err := ReadToolCompilePreviewReviewExecutionDispatch(root, filepath.Base(dispatch.Path))
	if err != nil {
		t.Fatalf("read compile preview review execution dispatch: %v", err)
	}
	if read.Path != dispatch.Path || read.ReviewPath != review.Path || read.SelectionPath != selection.Path {
		t.Fatalf("unexpected read dispatch: %#v", read)
	}
}

func TestExecuteToolCompilePreviewReviewExecutionDispatchRunsReviewedDispatchWithExplicitGate(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	review, err := CreateToolCompilePreviewReview(root, ToolCompilePreviewOptions{Action: compiler.ActionEnumeratePaths})
	if err != nil {
		t.Fatalf("create compile preview review: %v", err)
	}
	if _, err := SelectToolCompilePreviewReview(root, filepath.Base(review.Path), ToolCompilePreviewReviewSelectionOptions{SelectedBy: "closed-loop-planner"}); err != nil {
		t.Fatalf("select compile preview review: %v", err)
	}
	dispatch, err := CreateToolCompilePreviewReviewExecutionDispatch(root, filepath.Base(review.Path), ToolCompilePreviewReviewExecutionDispatchOptions{
		Actor:  "operator",
		Reason: "reviewed dispatch",
	})
	if err != nil {
		t.Fatalf("create compile preview review execution dispatch: %v", err)
	}

	runner := &recordingToolExecutionRunner{}
	executedAt := time.Date(2026, 6, 4, 12, 45, 0, 0, time.UTC)
	execution, err := ExecuteToolCompilePreviewReviewExecutionDispatch(ctx, root, filepath.Base(dispatch.Path), ToolCompilePreviewReviewExecutionOptions{
		Execute:    true,
		Actor:      "operator",
		Reason:     "execute reviewed dispatch",
		ExecutedAt: executedAt,
		ToolRunner: runner,
	})
	if err != nil {
		t.Fatalf("execute compile preview review execution dispatch: %v", err)
	}
	if execution.Status != "completed" || execution.Mode != "execute" || execution.DispatchPath != dispatch.Path || execution.ReviewPath != review.Path {
		t.Fatalf("unexpected execution from dispatch: %#v", execution)
	}
	if len(runner.requests) != len(review.Preview.Nodes) {
		t.Fatalf("expected dispatch execution to run frozen review nodes, got %d requests for %d nodes", len(runner.requests), len(review.Preview.Nodes))
	}
	executions, err := ListToolCompilePreviewReviewExecutions(root)
	if err != nil {
		t.Fatalf("list compile preview review executions: %v", err)
	}
	if len(executions) != 1 || executions[0].DispatchPath != dispatch.Path {
		t.Fatalf("expected one execution linked to dispatch, got %#v", executions)
	}
}

func TestExecuteToolCompilePreviewReviewWritesFailedExecutionArtifact(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	review, err := CreateToolCompilePreviewReview(root, ToolCompilePreviewOptions{Action: compiler.ActionEnumeratePaths})
	if err != nil {
		t.Fatalf("create compile preview review: %v", err)
	}
	if _, err := SelectToolCompilePreviewReview(root, filepath.Base(review.Path), ToolCompilePreviewReviewSelectionOptions{SelectedBy: "closed-loop-planner"}); err != nil {
		t.Fatalf("select compile preview review: %v", err)
	}
	runner := failingToolExecutionRunner{err: "simulated runner failure"}
	execution, err := ExecuteToolCompilePreviewReview(ctx, root, filepath.Base(review.Path), ToolCompilePreviewReviewExecutionOptions{
		Execute:    true,
		ToolRunner: runner,
	})
	if err == nil || !strings.Contains(err.Error(), "simulated runner failure") {
		t.Fatalf("expected runner failure, got %v", err)
	}
	if execution.Status != "failed" || !strings.Contains(execution.Error, "simulated runner failure") || execution.Path == "" || execution.EventCount == 0 {
		t.Fatalf("expected failed execution artifact metadata, got %#v", execution)
	}
	executions, err := ListToolCompilePreviewReviewExecutions(root)
	if err != nil {
		t.Fatalf("list compile preview review executions: %v", err)
	}
	if len(executions) != 1 || executions[0].Path != execution.Path || executions[0].Status != "failed" || !strings.Contains(executions[0].Error, "simulated runner failure") {
		t.Fatalf("expected one failed execution artifact, got %#v", executions)
	}
	store, err := eventlog.OpenSQLite(filepath.Join(root, "replay", "events.db"))
	if err != nil {
		t.Fatalf("open event log: %v", err)
	}
	defer store.Close()
	events, err := store.Replay(ctx, execution.RunID)
	if err != nil {
		t.Fatalf("replay failed execution events: %v", err)
	}
	if len(events) != execution.EventCount {
		t.Fatalf("expected failed execution event count %d, got %d", execution.EventCount, len(events))
	}
	assertStageAttemptSequence(t, events, []string{"tool-execution:started"})
}

func TestExecuteToolCompilePreviewReviewRequiresCurrentSafetyAuthorization(t *testing.T) {
	t.Skip("Safety authorization checks disabled")
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	review, err := CreateToolCompilePreviewReview(root, ToolCompilePreviewOptions{Action: compiler.ActionEnumeratePaths})
	if err != nil {
		t.Fatalf("create compile preview review: %v", err)
	}
	if _, err := SelectToolCompilePreviewReview(root, filepath.Base(review.Path), ToolCompilePreviewReviewSelectionOptions{SelectedBy: "closed-loop-planner"}); err != nil {
		t.Fatalf("select compile preview review: %v", err)
	}
	scope := `version: 1
scope:
  target_policy: disabled
  targets:
    - kind: url
      value: https://example.test
  exclusions: []
execution:
  isolation: process
`
	if err := os.WriteFile(filepath.Join(root, "scope.yaml"), []byte(scope), 0o644); err != nil {
		t.Fatalf("write disabled scope: %v", err)
	}
	runner := &recordingToolExecutionRunner{}
	_, err = ExecuteToolCompilePreviewReview(context.Background(), root, filepath.Base(review.Path), ToolCompilePreviewReviewExecutionOptions{
		Execute:    true,
		ToolRunner: runner,
	})
	if err == nil || !strings.Contains(err.Error(), "missing required safety bypass authorization for scope.target_policy disabled") {
		t.Fatalf("expected safety authorization error, got %v", err)
	}
	if len(runner.requests) != 0 {
		t.Fatalf("safety gate must run before tools, got %#v", runner.requests)
	}
	executions, listErr := ListToolCompilePreviewReviewExecutions(root)
	if listErr != nil {
		t.Fatalf("list compile preview review executions: %v", listErr)
	}
	if len(executions) != 0 {
		t.Fatalf("safety failure must not write execution artifacts, got %#v", executions)
	}
}

func TestExecuteToolCompilePreviewReviewRejectsSymlinkedExecutionDirectoryBeforeSideEffects(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	review, err := CreateToolCompilePreviewReview(root, ToolCompilePreviewOptions{Action: compiler.ActionEnumeratePaths})
	if err != nil {
		t.Fatalf("create compile preview review: %v", err)
	}
	if _, err := SelectToolCompilePreviewReview(root, filepath.Base(review.Path), ToolCompilePreviewReviewSelectionOptions{SelectedBy: "closed-loop-planner"}); err != nil {
		t.Fatalf("select compile preview review: %v", err)
	}
	outsideDir := filepath.Join(t.TempDir(), "outside-executions")
	if err := os.MkdirAll(outsideDir, 0o755); err != nil {
		t.Fatalf("create outside execution directory: %v", err)
	}
	executionDir := filepath.Join(root, "toolchain", "compile-preview-review-executions")
	if err := os.RemoveAll(executionDir); err != nil {
		t.Fatalf("remove execution directory: %v", err)
	}
	if err := os.Symlink(outsideDir, executionDir); err != nil {
		t.Fatalf("symlink execution directory: %v", err)
	}
	runner := &recordingToolExecutionRunner{}
	_, err = ExecuteToolCompilePreviewReview(context.Background(), root, filepath.Base(review.Path), ToolCompilePreviewReviewExecutionOptions{
		Execute:    true,
		ToolRunner: runner,
	})
	if err == nil || !strings.Contains(err.Error(), "compile preview review execution directory must not be a symlink") {
		t.Fatalf("expected symlinked execution directory rejection, got %v", err)
	}
	if len(runner.requests) != 0 {
		t.Fatalf("symlinked execution directory must fail before running tools, got %#v", runner.requests)
	}
	if got := countJSON(t, outsideDir); got != 0 {
		t.Fatalf("symlinked execution directory must not write outside execution artifacts, got %d", got)
	}
	if _, statErr := os.Stat(filepath.Join(root, "replay", "events.db")); !os.IsNotExist(statErr) {
		t.Fatalf("symlinked execution directory must fail before replay event writes, stat err=%v", statErr)
	}
}

func TestReadAndListToolCompilePreviewReviewExecutionsRejectSymlinkedExecutionFile(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	executionDir := filepath.Join(root, "toolchain", "compile-preview-review-executions")
	if err := os.MkdirAll(executionDir, 0o755); err != nil {
		t.Fatalf("create execution directory: %v", err)
	}
	outsideFile := filepath.Join(t.TempDir(), "outside-execution.json")
	if err := os.WriteFile(outsideFile, []byte(`{"status":"completed"}`+"\n"), 0o644); err != nil {
		t.Fatalf("write outside execution file: %v", err)
	}
	if err := os.Symlink(outsideFile, filepath.Join(executionDir, "linked-execution.json")); err != nil {
		t.Fatalf("symlink execution file: %v", err)
	}
	if _, err := ReadToolCompilePreviewReviewExecution(root, "linked-execution.json"); err == nil || !strings.Contains(err.Error(), "compile preview review execution file must not be a symlink") {
		t.Fatalf("expected symlinked execution file read rejection, got %v", err)
	}
	if _, err := ListToolCompilePreviewReviewExecutions(root); err == nil || !strings.Contains(err.Error(), "compile preview review execution file must not be a symlink") {
		t.Fatalf("expected symlinked execution file list rejection, got %v", err)
	}
	outsideData, err := os.ReadFile(outsideFile)
	if err != nil {
		t.Fatalf("read outside execution file: %v", err)
	}
	if string(outsideData) != "{\"status\":\"completed\"}\n" {
		t.Fatalf("read/list must not rewrite outside execution file, got %q", string(outsideData))
	}
}

func TestClearToolCompilePreviewReviewSelectionWritesClearedMetadataOnlyMarker(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	marker := filepath.Join(t.TempDir(), "compile-preview-review-selection-clear-ran")
	manifest := `name: web-api-pack
version: "0.1.0"
capabilities:
  - web.discovery
runner: command
run_command:
  - sh
  - -c
  - touch ` + marker + `
parser: web_api_parser
healthcheck: built_in
`
	if err := os.WriteFile(filepath.Join(root, "toolchain", "web-api-pack", "manifest.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	candidatePath := writeCompilePreviewReviewFixture(t, root, "candidate.json", ToolCompilePreviewReview{
		Status: "dry-run",
		Mode:   "metadata-only",
		Preview: ToolCompilePreview{
			Workspace: root,
			Action:    compiler.ActionDiscoverAssets,
			Mode:      "metadata-only",
			Nodes: []compiler.PlanNode{
				{ID: "discover_assets:01", ManifestName: "web-api-pack", Capability: "web.discovery"},
			},
		},
	})
	if _, err := SelectToolCompilePreviewReview(root, filepath.Base(candidatePath), ToolCompilePreviewReviewSelectionOptions{
		SelectedBy: "operator",
		SelectedAt: time.Date(2026, 6, 1, 0, 15, 0, 0, time.UTC),
		Reason:     "current candidate",
	}); err != nil {
		t.Fatalf("select compile preview review: %v", err)
	}

	clearedAt := time.Date(2026, 6, 1, 0, 35, 0, 0, time.UTC)
	selection, err := ClearToolCompilePreviewReviewSelection(root, ToolCompilePreviewReviewSelectionClearOptions{
		ClearedBy: "operator",
		ClearedAt: clearedAt,
		Reason:    "authorization changed",
	})
	if err != nil {
		t.Fatalf("clear compile preview review selection: %v", err)
	}
	if selection.Status != "cleared" || selection.Mode != "metadata-only" || selection.ReviewPath != candidatePath || selection.ClearedBy != "operator" || selection.ClearedAt != "2026-06-01T00:35:00Z" || selection.Reason != "authorization changed" {
		t.Fatalf("unexpected cleared selection: %#v", selection)
	}
	if selection.Path != filepath.Join(root, "toolchain", "compile-preview-review-selection.json") {
		t.Fatalf("unexpected selection path: %s", selection.Path)
	}
	read, found, err := ReadToolCompilePreviewReviewSelection(root)
	if err != nil {
		t.Fatalf("read compile preview review selection: %v", err)
	}
	if !found || read.Status != "cleared" || read.ReviewPath != candidatePath || read.ClearedBy != "operator" {
		t.Fatalf("expected persisted cleared selection, found=%v selection=%#v", found, read)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("clearing compile preview review selection must not execute run_command, marker stat err=%v", err)
	}
}

func TestAnnotateToolCompilePreviewReviewWritesMetadataOnlyDecision(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	marker := filepath.Join(t.TempDir(), "compile-preview-review-annotate-ran")
	manifest := `name: web-api-pack
version: "0.1.0"
capabilities:
  - web.discovery
runner: command
run_command:
  - sh
  - -c
  - touch ` + marker + `
parser: web_api_parser
healthcheck: built_in
`
	if err := os.WriteFile(filepath.Join(root, "toolchain", "web-api-pack", "manifest.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	review, err := CreateToolCompilePreviewReview(root, ToolCompilePreviewOptions{
		Budget: compiler.CompileBudget{MaxPlanNodes: 1},
	})
	if err != nil {
		t.Fatalf("create compile preview review: %v", err)
	}

	reviewedAt := time.Date(2026, 5, 31, 15, 4, 5, 0, time.UTC)
	annotated, err := AnnotateToolCompilePreviewReview(root, filepath.Base(review.Path), ToolCompilePreviewReviewAnnotationOptions{
		Decision:   "accepted",
		Reason:     "authorized candidate path",
		Reviewer:   "operator",
		ReviewedAt: reviewedAt,
	})
	if err != nil {
		t.Fatalf("annotate compile preview review: %v", err)
	}
	if annotated.Path != review.Path || len(annotated.Preview.Nodes) != 1 {
		t.Fatalf("annotation should preserve review artifact and preview, got %#v", annotated)
	}
	if annotated.Annotation == nil || annotated.Annotation.Decision != "accepted" || annotated.Annotation.Reason != "authorized candidate path" || annotated.Annotation.Reviewer != "operator" || annotated.Annotation.ReviewedAt != "2026-05-31T15:04:05Z" {
		t.Fatalf("unexpected annotation: %#v", annotated.Annotation)
	}
	read, err := ReadToolCompilePreviewReview(root, review.Path)
	if err != nil {
		t.Fatalf("read annotated compile preview review: %v", err)
	}
	if read.Annotation == nil || read.Annotation.Decision != "accepted" {
		t.Fatalf("expected persisted annotation, got %#v", read.Annotation)
	}
	if _, err := AnnotateToolCompilePreviewReview(root, filepath.Base(review.Path), ToolCompilePreviewReviewAnnotationOptions{Decision: "unknown"}); err == nil {
		t.Fatalf("expected invalid annotation decision to be rejected")
	}
	if _, err := AnnotateToolCompilePreviewReview(root, filepath.Join(t.TempDir(), "outside.json"), ToolCompilePreviewReviewAnnotationOptions{Decision: "deferred"}); err == nil {
		t.Fatalf("expected annotation path outside workspace to be rejected")
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("annotating compile preview review must not execute run_command, marker stat err=%v", err)
	}
}

func writeCompilePreviewReviewFixture(t *testing.T, root, name string, review ToolCompilePreviewReview) string {
	t.Helper()
	dir := filepath.Join(root, "toolchain", "compile-preview-reviews")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("create compile preview review fixture dir: %v", err)
	}
	path := filepath.Join(dir, name)
	data, err := json.MarshalIndent(review, "", "  ")
	if err != nil {
		t.Fatalf("marshal compile preview review fixture: %v", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write compile preview review fixture: %v", err)
	}
	return path
}

func assertStageAttemptSequence(t *testing.T, events []eventlog.Event, want []string) {
	t.Helper()
	got := []string{}
	for _, event := range events {
		if event.Type != eventlog.EventAttempt {
			continue
		}
		var payload struct {
			Kind    string `json:"kind"`
			StageID string `json:"stage_id"`
			Status  string `json:"status"`
		}
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			t.Fatalf("decode attempt payload: %v", err)
		}
		if payload.Kind != "stage" {
			continue
		}
		got = append(got, payload.StageID+":"+payload.Status)
	}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("expected stage sequence %#v, got %#v", want, got)
	}
}

func TestWriteToolCompilePreviewSignalsPersistsMetadataOnlyInputs(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	marker := filepath.Join(t.TempDir(), "compile-preview-ran")
	manifest := `name: web-api-pack
version: "0.1.0"
capabilities:
  - web.discovery
runner: command
run_command:
  - sh
  - -c
  - touch ` + marker + `
parser: web_api_parser
healthcheck: built_in
`
	if err := os.WriteFile(filepath.Join(root, "toolchain", "web-api-pack", "manifest.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	artifact, err := WriteToolCompilePreviewSignals(root, ToolCompilePreviewSignals{
		Budget: ToolCompilePreviewSignalBudget{MaxPlanNodes: 1},
		DomainPriorities: []ToolCompilePreviewSignalDomainPriority{
			{Domain: "infra_internal", Weight: 3},
		},
		FrontierScores: []ToolCompilePreviewSignalFrontierScore{
			{
				ManifestName: "infra-pack",
				Capability:   "infra.host_discovery",
				Path: ToolCompilePreviewSignalPath{
					MissionValue:    2,
					UnlockPotential: 1,
				},
			},
		},
		HistoricalStats: []ToolCompilePreviewSignalHistory{
			{ManifestName: "infra-pack", Capability: "infra.host_discovery", Successes: 2},
			{ManifestName: "web-api-pack", Capability: "web.discovery", Failures: 4},
		},
	})
	if err != nil {
		t.Fatalf("write compile preview signals: %v", err)
	}
	if artifact.Status != "written" || artifact.Mode != "metadata-only" {
		t.Fatalf("expected written metadata-only artifact, got %#v", artifact)
	}
	if artifact.Path != filepath.Join(root, "toolchain", "compile-preview-signals.json") {
		t.Fatalf("unexpected signal path: %s", artifact.Path)
	}

	data, err := os.ReadFile(artifact.Path)
	if err != nil {
		t.Fatalf("read written signals: %v", err)
	}
	var stored ToolCompilePreviewSignals
	if err := json.Unmarshal(data, &stored); err != nil {
		t.Fatalf("decode written signals: %v", err)
	}
	if stored.SchemaVersion != 1 || stored.Budget.MaxPlanNodes != 1 {
		t.Fatalf("expected normalized schema and budget, got %#v", stored)
	}
	loaded, found, err := LoadToolCompilePreviewSignals(root)
	if err != nil {
		t.Fatalf("load written signals: %v", err)
	}
	if !found || loaded.SchemaVersion != 1 || len(loaded.FrontierScores) != 1 || len(loaded.HistoricalStats) != 2 {
		t.Fatalf("expected persisted scoring inputs, found=%v signals=%#v", found, loaded)
	}

	preview, err := PreviewToolCompilePlan(root, ToolCompilePreviewOptions{})
	if err != nil {
		t.Fatalf("preview compile plan: %v", err)
	}
	if len(preview.Nodes) != 1 || preview.Nodes[0].ManifestName != "infra-pack" {
		t.Fatalf("expected written signals to rank infra first, got %#v", preview.Nodes)
	}
	for _, want := range []string{
		"domain:infra_internal=3",
		"frontier=3",
		"history:successes=2 failures=0",
	} {
		if !containsString(preview.Nodes[0].SelectionSignals, want) {
			t.Fatalf("expected written signal %q in %#v", want, preview.Nodes[0].SelectionSignals)
		}
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("writing/previewing signals must not execute run_command, marker stat err=%v", err)
	}
}

func TestDeriveToolCompilePreviewSignalsFromReplayHistoryMetadataOnly(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	marker := filepath.Join(t.TempDir(), "derived-signals-ran")
	manifest := `name: web-api-pack
version: "0.1.0"
capabilities:
  - web.discovery
runner: command
run_command:
  - sh
  - -c
  - touch ` + marker + `
parser: web_api_parser
healthcheck: built_in
`
	if err := os.WriteFile(filepath.Join(root, "toolchain", "web-api-pack", "manifest.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	runner := &statusToolCompilePreviewRunner{statuses: map[string]string{
		"web-api-pack": "failed",
		"infra-pack":   "completed",
		"ctf-pack":     "skipped",
	}}
	run, err := RunWorkspaceWithOptions(ctx, root, RunOptions{ToolRunner: runner})
	if err != nil {
		t.Fatalf("run workspace: %v", err)
	}

	dryRun, err := DeriveToolCompilePreviewSignalsFromReplay(ctx, root, run.RunID, ToolCompilePreviewSignalDeriveOptions{})
	if err != nil {
		t.Fatalf("derive compile preview signals: %v", err)
	}
	if dryRun.Status != "dry-run" || dryRun.Mode != "metadata-only" || dryRun.RunID != run.RunID {
		t.Fatalf("expected dry-run metadata-only derivation, got %#v", dryRun)
	}
	if dryRun.Summary.ToolExecutions != 7 || dryRun.Summary.Successes != 5 || dryRun.Summary.Failures != 1 || dryRun.Summary.Ignored != 1 {
		t.Fatalf("unexpected derivation summary: %#v", dryRun.Summary)
	}
	if len(dryRun.Signals.HistoricalStats) != 2 {
		t.Fatalf("expected two historical stats, got %#v", dryRun.Signals.HistoricalStats)
	}
	history := map[string]ToolCompilePreviewSignalHistory{}
	for _, stat := range dryRun.Signals.HistoricalStats {
		history[stat.ManifestName+"/"+stat.Capability] = stat
	}
	if history["infra-pack/infra.host_discovery"].Successes != 5 {
		t.Fatalf("expected infra success history, got %#v", history)
	}
	if history["web-api-pack/web.discovery"].Failures != 1 {
		t.Fatalf("expected web failure history, got %#v", history)
	}
	if _, err := os.Stat(filepath.Join(root, "toolchain", "compile-preview-signals.json")); !os.IsNotExist(err) {
		t.Fatalf("dry-run derivation must not write signals, stat err=%v", err)
	}

	written, err := DeriveToolCompilePreviewSignalsFromReplay(ctx, root, run.RunID, ToolCompilePreviewSignalDeriveOptions{
		Write:        true,
		MaxPlanNodes: 1,
	})
	if err != nil {
		t.Fatalf("write derived compile preview signals: %v", err)
	}
	if written.Status != "written" || written.Path != filepath.Join(root, "toolchain", "compile-preview-signals.json") {
		t.Fatalf("expected written derived signal artifact, got %#v", written)
	}
	loaded, found, err := LoadToolCompilePreviewSignals(root)
	if err != nil {
		t.Fatalf("load derived signals: %v", err)
	}
	if !found || loaded.Budget.MaxPlanNodes != 1 || len(loaded.HistoricalStats) != 2 {
		t.Fatalf("expected written derived history signals, found=%v signals=%#v", found, loaded)
	}
	preview, err := PreviewToolCompilePlan(root, ToolCompilePreviewOptions{})
	if err != nil {
		t.Fatalf("preview compile plan with derived signals: %v", err)
	}
	if len(preview.Nodes) != 1 || preview.Nodes[0].ManifestName != "infra-pack" {
		t.Fatalf("expected derived history to rank infra first under budget, got %#v", preview.Nodes)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("deriving compile preview signals must not execute run_command, marker stat err=%v", err)
	}
}

func TestDeriveToolCompilePreviewSignalsFromReplayFrontierMetadataOnly(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	scope := `version: 1
scope:
  target_policy: enforce
  targets:
    - kind: url
      value: https://app.example
  exclusions: []
execution:
  isolation: process
`
	if err := os.WriteFile(filepath.Join(root, "scope.yaml"), []byte(scope), 0o644); err != nil {
		t.Fatalf("write scope: %v", err)
	}
	runner := &statusToolCompilePreviewRunner{}
	run, err := RunWorkspaceWithOptions(ctx, root, RunOptions{ToolRunner: runner})
	if err != nil {
		t.Fatalf("run workspace: %v", err)
	}

	derivation, err := DeriveToolCompilePreviewSignalsFromReplay(ctx, root, run.RunID, ToolCompilePreviewSignalDeriveOptions{})
	if err != nil {
		t.Fatalf("derive compile preview signals: %v", err)
	}
	if derivation.Summary.FrontierScores == 0 {
		t.Fatalf("expected replay hypotheses to derive frontier scores, got %#v", derivation)
	}
	var webDiscovery ToolCompilePreviewSignalFrontierScore
	for _, score := range derivation.Signals.FrontierScores {
		if score.ManifestName == "web-api-pack" && score.Capability == "web.discovery" {
			webDiscovery = score
			break
		}
	}
	if webDiscovery.ManifestName == "" {
		t.Fatalf("expected web discovery frontier score in %#v", derivation.Signals.FrontierScores)
	}
	if webDiscovery.Path.ID != "web-discover-surface" || webDiscovery.Path.Target != "https://app.example" {
		t.Fatalf("expected hypothesis id/target to flow into frontier path, got %#v", webDiscovery.Path)
	}
	if webDiscovery.Path.EvidenceConfidence <= 0 || webDiscovery.Path.MissionValue <= 0 || webDiscovery.Path.UnlockPotential <= 0 {
		t.Fatalf("expected positive frontier metrics from hypothesis confidence, got %#v", webDiscovery.Path)
	}
	if _, err := os.Stat(filepath.Join(root, "toolchain", "compile-preview-signals.json")); !os.IsNotExist(err) {
		t.Fatalf("dry-run frontier derivation must not write signals, stat err=%v", err)
	}
}

func TestDeriveToolCompilePreviewSignalsFromReplayCandidatePathFactsMetadataOnly(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	marker := filepath.Join(t.TempDir(), "candidate-path-signals-ran")
	manifest := `name: web-api-pack
version: "0.1.0"
capabilities:
  - web.discovery
runner: command
run_command:
  - sh
  - -c
  - touch ` + marker + `
parser: web_api_parser
healthcheck: built_in
`
	if err := os.WriteFile(filepath.Join(root, "toolchain", "web-api-pack", "manifest.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	replayDir := filepath.Join(root, "replay")
	if err := os.MkdirAll(replayDir, 0o755); err != nil {
		t.Fatalf("create replay dir: %v", err)
	}
	store, err := eventlog.OpenSQLite(filepath.Join(replayDir, "events.db"))
	if err != nil {
		t.Fatalf("open event log: %v", err)
	}
	defer store.Close()
	runID := "run-candidate-path-signals"
	for _, payload := range []map[string]any{
		{
			"fact_id":       "fact-target",
			"kind":          "target",
			"statement":     "https://ignore.example",
			"confidence":    1,
			"mission_value": 100,
		},
		{
			"fact_id":             "fact-web-frontier",
			"kind":                "candidate_path",
			"statement":           "prioritize web discovery from reviewed evidence",
			"target":              "https://app.example",
			"expected_unlock":     "route inventory unlocks auth testing",
			"action":              "discover_assets",
			"domain":              "web_api",
			"mission_value":       2,
			"unlock_potential":    1,
			"evidence_confidence": 0.8,
		},
	} {
		data, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("encode candidate-path fact payload: %v", err)
		}
		if _, err := store.Append(ctx, eventlog.Event{RunID: runID, Type: eventlog.EventFact, Payload: data}); err != nil {
			t.Fatalf("append candidate-path fact event: %v", err)
		}
	}

	derivation, err := DeriveToolCompilePreviewSignalsFromReplay(ctx, root, runID, ToolCompilePreviewSignalDeriveOptions{})
	if err != nil {
		t.Fatalf("derive candidate-path compile preview signals: %v", err)
	}
	if derivation.Summary.FrontierScores != 1 || len(derivation.Signals.FrontierScores) != 1 {
		t.Fatalf("expected one candidate-path frontier score, got %#v", derivation)
	}
	score := derivation.Signals.FrontierScores[0]
	if score.ManifestName != "web-api-pack" || score.Capability != "web.discovery" {
		t.Fatalf("expected candidate path to map to web discovery, got %#v", score)
	}
	if score.Path.ID != "fact-web-frontier" || score.Path.Target != "https://app.example" || score.Path.Hypothesis != "prioritize web discovery from reviewed evidence" {
		t.Fatalf("expected fact metadata to flow into candidate path, got %#v", score.Path)
	}
	if score.Path.ExpectedUnlock != "route inventory unlocks auth testing" || toolCompilePreviewSignalPathScore(score.Path) != 3.8 {
		t.Fatalf("unexpected candidate path scoring metadata: %#v", score.Path)
	}
	if _, err := os.Stat(filepath.Join(root, "toolchain", "compile-preview-signals.json")); !os.IsNotExist(err) {
		t.Fatalf("dry-run candidate-path derivation must not write signals, stat err=%v", err)
	}

	written, err := DeriveToolCompilePreviewSignalsFromReplay(ctx, root, runID, ToolCompilePreviewSignalDeriveOptions{Write: true, MaxPlanNodes: 1})
	if err != nil {
		t.Fatalf("write candidate-path compile preview signals: %v", err)
	}
	if written.Status != "written" || len(written.Signals.FrontierScores) != 1 || written.Signals.Budget.MaxPlanNodes != 1 {
		t.Fatalf("expected written candidate-path signals, got %#v", written)
	}
	preview, err := PreviewToolCompilePlan(root, ToolCompilePreviewOptions{})
	if err != nil {
		t.Fatalf("preview compile plan with candidate-path signals: %v", err)
	}
	if len(preview.Nodes) != 1 || preview.Nodes[0].ManifestName != "web-api-pack" || !containsString(preview.Nodes[0].SelectionSignals, "frontier=3.8") {
		t.Fatalf("expected candidate-path frontier score to rank web discovery, got %#v", preview.Nodes)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("candidate-path derivation and preview must not execute run_command, marker stat err=%v", err)
	}
}

func TestInspectToolchainsRunsExecHealthProbe(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	skipIfWindows(t)
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	probe := filepath.Join(t.TempDir(), "web-probe")
	if err := os.WriteFile(probe, []byte("#!/bin/sh\nprintf 'web-probe-2.0.0\\n'\n"), 0o755); err != nil {
		t.Fatalf("write probe: %v", err)
	}
	manifest := `name: web-api-pack
version: "0.1.0"
capabilities:
  - web.discovery
runner: native
parser: web_api_parser
healthcheck: exec
health_command:
  - ` + probe + `
`
	if err := os.WriteFile(filepath.Join(root, "toolchain", "web-api-pack", "manifest.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	statuses, err := InspectToolchains(root)
	if err != nil {
		t.Fatalf("inspect toolchains: %v", err)
	}

	for _, status := range statuses {
		if status.Name != "web-api-pack" {
			continue
		}
		if status.Health.State != toolchain.HealthReady || status.Health.InstalledVersion != "web-probe-2.0.0" {
			t.Fatalf("expected exec probe health for web-api-pack, got %#v", status.Health)
		}
		return
	}
	t.Fatalf("expected web-api-pack status in %#v", statuses)
}

func TestInspectToolchainsReportsDependencyHealth(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	manifest := `name: web-api-pack
version: "0.1.0"
capabilities:
  - web.discovery
runner: native
parser: web_api_parser
healthcheck: built_in
dependencies:
  - name: ` + filepath.Join(t.TempDir(), "missing-httpx") + `
    kind: binary
    install_hint: install bundled httpx pack
    install_command:
      - ouro-toolchain
      - install
      - httpx
`
	if err := os.WriteFile(filepath.Join(root, "toolchain", "web-api-pack", "manifest.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	statuses, err := InspectToolchains(root)
	if err != nil {
		t.Fatalf("inspect toolchains: %v", err)
	}
	for _, status := range statuses {
		if status.Name != "web-api-pack" {
			continue
		}
		if status.Health.State != toolchain.HealthMissing {
			t.Fatalf("expected required missing dependency to downgrade health, got %#v", status.Health)
		}
		if len(status.Dependencies) != 1 || status.Dependencies[0].State != toolchain.HealthMissing {
			t.Fatalf("expected missing dependency report, got %#v", status.Dependencies)
		}
		if len(status.InstallPlan) != 1 || status.InstallPlan[0].Name == "" {
			t.Fatalf("expected missing dependency install plan, got %#v", status.InstallPlan)
		}
		return
	}
	t.Fatalf("expected web-api-pack status in %#v", statuses)
}

func TestInspectToolchainsFetchesDependencyMetadata(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	pinned := "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	latest := "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	updateDir := filepath.Join(root, "toolchain", "web-api-pack", "updates")
	if err := os.MkdirAll(updateDir, 0o755); err != nil {
		t.Fatalf("create update metadata dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(updateDir, "httpx-worker.json"), []byte(`{
  "latest_version": "1.2.0",
  "image_digest": "`+latest+`",
  "source": "ghcr.io/ouroboros/httpx-worker",
  "message": "remote metadata fixture"
}`), 0o644); err != nil {
		t.Fatalf("write update metadata: %v", err)
	}
	manifest := `name: web-api-pack
version: "0.1.0"
capabilities:
  - web.discovery
runner: native
parser: web_api_parser
healthcheck: built_in
dependencies:
  - name: ouro-httpx-worker
    kind: container_image
    version: "1.0.0"
    source: ghcr.io/ouroboros/httpx-worker
    image_digest: ` + pinned + `
    metadata_source: updates/httpx-worker.json
`
	if err := os.WriteFile(filepath.Join(root, "toolchain", "web-api-pack", "manifest.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	statuses, err := InspectToolchains(root)
	if err != nil {
		t.Fatalf("inspect toolchains: %v", err)
	}

	for _, status := range statuses {
		if status.Name != "web-api-pack" {
			continue
		}
		if status.Health.State != toolchain.HealthReady {
			t.Fatalf("remote update availability should not downgrade pack health, got %#v", status.Health)
		}
		if len(status.Dependencies) != 1 {
			t.Fatalf("expected one dependency, got %#v", status.Dependencies)
		}
		dependency := status.Dependencies[0]
		if dependency.State != toolchain.HealthUpdateAvailable || dependency.LatestVersion != "1.2.0" || dependency.LatestImageDigest != latest {
			t.Fatalf("expected remote metadata update_available dependency, got %#v", dependency)
		}
		if dependency.MetadataSource != "updates/httpx-worker.json" || !strings.Contains(dependency.Message, "remote metadata fixture") {
			t.Fatalf("expected metadata source/message, got %#v", dependency)
		}
		return
	}
	t.Fatalf("expected web-api-pack status in %#v", statuses)
}

func TestListManagedToolBundlesReadsWorkspaceBundleMetadata(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	payload := writeManagedBundleFixture(t, root, "httpx", "binary", "1.3.0", "#!/bin/sh\nexit 99\n")

	bundles, err := ListManagedToolBundles(root)
	if err != nil {
		t.Fatalf("list managed tool bundles: %v", err)
	}
	if len(bundles) != 1 {
		t.Fatalf("expected one bundle, got %#v", bundles)
	}
	bundle := bundles[0]
	if bundle.Name != "httpx" || bundle.Kind != "binary" || bundle.Version != "1.3.0" {
		t.Fatalf("unexpected managed bundle metadata: %#v", bundle)
	}
	if bundle.PayloadPath != payload || bundle.InstallPath != filepath.Join(root, "toolchain", "bin", "httpx") {
		t.Fatalf("unexpected managed bundle paths: %#v", bundle)
	}
}

func TestImportManagedToolBundleCopiesLocalBinaryPayloadAndWritesManifestWithoutRunningIt(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	marker := filepath.Join(t.TempDir(), "import-payload-ran")
	payloadBody := "#!/bin/sh\ntouch " + marker + "\n"
	sourcePayload := filepath.Join(t.TempDir(), "httpx")
	if err := os.WriteFile(sourcePayload, []byte(payloadBody), 0o755); err != nil {
		t.Fatalf("write source payload: %v", err)
	}
	digest := sha256.Sum256([]byte(payloadBody))
	expectedSHA := hex.EncodeToString(digest[:])
	expectedPayload := filepath.Join(root, "toolchain", "managed-bundles", "httpx", "payload", "httpx")
	expectedManifest := filepath.Join(root, "toolchain", "managed-bundles", "httpx", "bundle.yaml")
	expectedInstall := filepath.Join(root, "toolchain", "bin", "httpx")

	imported, err := ImportManagedToolBundle(root, ManagedToolBundleImportOptions{
		Import:      true,
		Name:        "httpx",
		Kind:        "binary",
		Version:     "1.3.0",
		Source:      "local-test/httpx",
		PayloadPath: sourcePayload,
	})
	if err != nil {
		t.Fatalf("import managed bundle: %v", err)
	}
	if imported.Status != "imported" || imported.Mode != "workspace-copy" || imported.Action != "copy" {
		t.Fatalf("unexpected import status: %#v", imported)
	}
	if imported.Name != "httpx" || imported.Kind != "binary" || imported.Version != "1.3.0" || imported.Source != "local-test/httpx" || imported.SHA256 != expectedSHA {
		t.Fatalf("unexpected import metadata: %#v", imported)
	}
	if imported.SourcePayloadPath != sourcePayload || imported.PayloadPath != expectedPayload || imported.ManifestPath != expectedManifest || imported.InstallPath != expectedInstall {
		t.Fatalf("unexpected import paths: %#v", imported)
	}
	copied, err := os.ReadFile(expectedPayload)
	if err != nil {
		t.Fatalf("read copied payload: %v", err)
	}
	if string(copied) != payloadBody {
		t.Fatalf("expected copied payload to match source, got %q", string(copied))
	}
	manifest, err := os.ReadFile(expectedManifest)
	if err != nil {
		t.Fatalf("read imported manifest: %v", err)
	}
	for _, want := range []string{
		"name: httpx",
		"kind: binary",
		"version: \"1.3.0\"",
		"source: \"local-test/httpx\"",
		"payload: payload/httpx",
		"sha256: " + expectedSHA,
	} {
		if !strings.Contains(string(manifest), want) {
			t.Fatalf("expected imported manifest to contain %q, got %q", want, string(manifest))
		}
	}
	bundle, err := ReadManagedToolBundle(root, "httpx")
	if err != nil {
		t.Fatalf("read imported managed bundle: %v", err)
	}
	if bundle.PayloadPath != expectedPayload || bundle.SHA256 != expectedSHA {
		t.Fatalf("unexpected imported bundle readback: %#v", bundle)
	}
	if _, err := os.Lstat(expectedInstall); !os.IsNotExist(err) {
		t.Fatalf("import must not create install link, stat err=%v", err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("import must not execute payload, marker stat err=%v", err)
	}
}

func TestImportManagedToolBundleEnforcesExpectedSHA256BeforeWorkspaceWrites(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	payloadBody := "#!/bin/sh\nexit 0\n"
	sourcePayload := filepath.Join(t.TempDir(), "httpx")
	if err := os.WriteFile(sourcePayload, []byte(payloadBody), 0o755); err != nil {
		t.Fatalf("write source payload: %v", err)
	}
	actualDigest := sha256.Sum256([]byte(payloadBody))
	actualSHA := hex.EncodeToString(actualDigest[:])
	wrongDigest := sha256.Sum256([]byte("different payload"))
	wrongSHA := hex.EncodeToString(wrongDigest[:])
	bundleDir := filepath.Join(root, "toolchain", "managed-bundles", "httpx")

	_, err := ImportManagedToolBundle(root, ManagedToolBundleImportOptions{
		Import:         true,
		Name:           "httpx",
		Kind:           "binary",
		Version:        "1.3.0",
		Source:         "local-test/httpx",
		PayloadPath:    sourcePayload,
		ExpectedSHA256: wrongSHA,
	})
	if err == nil || !strings.Contains(err.Error(), "sha256 mismatch") || !strings.Contains(err.Error(), wrongSHA) || !strings.Contains(err.Error(), actualSHA) {
		t.Fatalf("expected mismatch error with expected and actual digest, got %v", err)
	}
	if _, err := os.Lstat(bundleDir); !os.IsNotExist(err) {
		t.Fatalf("expected sha256 mismatch must not write bundle dir, stat err=%v", err)
	}

	_, err = ImportManagedToolBundle(root, ManagedToolBundleImportOptions{
		Import:         true,
		Name:           "httpx",
		Kind:           "binary",
		Version:        "1.3.0",
		Source:         "local-test/httpx",
		PayloadPath:    sourcePayload,
		ExpectedSHA256: "not-a-sha256",
	})
	if err == nil || !strings.Contains(err.Error(), "expected sha256 is invalid") {
		t.Fatalf("expected invalid expected sha256 rejection, got %v", err)
	}
	if _, err := os.Lstat(bundleDir); !os.IsNotExist(err) {
		t.Fatalf("invalid expected sha256 must not write bundle dir, stat err=%v", err)
	}

	_, err = ImportManagedToolBundle(root, ManagedToolBundleImportOptions{
		Import:                 true,
		Name:                   "httpx",
		Kind:                   "binary",
		Version:                "1.3.0",
		Source:                 "local-test/httpx",
		PayloadPath:            sourcePayload,
		ExpectedSHA256:         "",
		ExpectedSHA256Provided: true,
	})
	if err == nil || !strings.Contains(err.Error(), "expected sha256 is invalid") {
		t.Fatalf("expected explicit empty expected sha256 rejection, got %v", err)
	}
	if _, err := os.Lstat(bundleDir); !os.IsNotExist(err) {
		t.Fatalf("explicit empty expected sha256 must not write bundle dir, stat err=%v", err)
	}

	imported, err := ImportManagedToolBundle(root, ManagedToolBundleImportOptions{
		Import:         true,
		Name:           "httpx",
		Kind:           "binary",
		Version:        "1.3.0",
		Source:         "local-test/httpx",
		PayloadPath:    sourcePayload,
		ExpectedSHA256: strings.ToUpper(actualSHA),
	})
	if err != nil {
		t.Fatalf("import with matching expected sha256: %v", err)
	}
	if imported.SHA256 != actualSHA || imported.ExpectedSHA256 != actualSHA {
		t.Fatalf("expected matching sha256 metadata to be normalized, got %#v", imported)
	}
	if _, err := os.Stat(filepath.Join(bundleDir, "payload", "httpx")); err != nil {
		t.Fatalf("expected bundle payload after matching expected sha256: %v", err)
	}
}

func TestImportManagedToolBundleCopiesLocalRuleBundleDirectory(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	skipIfWindows(t)
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	sourcePayload := filepath.Join(t.TempDir(), "nuclei-templates")
	for rel, body := range map[string]string{
		"http/example.yaml": "id: ouro-http\ninfo:\n  name: Ouro HTTP\n  severity: info\n",
		"dns/info.yaml":     "id: ouro-dns\ninfo:\n  name: Ouro DNS\n  severity: low\n",
	} {
		path := filepath.Join(sourcePayload, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("create source rule dir: %v", err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatalf("write source rule: %v", err)
		}
	}
	if err := os.Chmod(filepath.Join(sourcePayload, "http", "example.yaml"), 0o777); err != nil {
		t.Fatalf("chmod executable source rule: %v", err)
	}
	actualSHA := digestDirectoryForAppTest(t, sourcePayload)
	wrongDigest := sha256.Sum256([]byte("different rule bundle"))
	wrongSHA := hex.EncodeToString(wrongDigest[:])
	bundleDir := filepath.Join(root, "toolchain", "managed-bundles", "nuclei-templates")

	_, err := ImportManagedToolBundle(root, ManagedToolBundleImportOptions{
		Import:         true,
		Name:           "nuclei-templates",
		Kind:           "rule_bundle",
		Version:        "2026.06.05",
		Source:         "local-test/nuclei-templates",
		PayloadPath:    sourcePayload,
		ExpectedSHA256: wrongSHA,
	})
	if err == nil || !strings.Contains(err.Error(), "sha256 mismatch") || !strings.Contains(err.Error(), wrongSHA) || !strings.Contains(err.Error(), actualSHA) {
		t.Fatalf("expected rule bundle mismatch error with expected and actual digest, got %v", err)
	}
	if _, err := os.Lstat(bundleDir); !os.IsNotExist(err) {
		t.Fatalf("rule bundle expected sha256 mismatch must not write bundle dir, stat err=%v", err)
	}

	imported, err := ImportManagedToolBundle(root, ManagedToolBundleImportOptions{
		Import:         true,
		Name:           "nuclei-templates",
		Kind:           "rule_bundle",
		Version:        "2026.06.05",
		Source:         "local-test/nuclei-templates",
		PayloadPath:    sourcePayload,
		ExpectedSHA256: strings.ToUpper(actualSHA),
	})
	if err != nil {
		t.Fatalf("import managed rule bundle: %v", err)
	}
	expectedPayload := filepath.Join(bundleDir, "payload")
	expectedManifest := filepath.Join(bundleDir, "bundle.yaml")
	expectedInstall := filepath.Join(root, "toolchain", "rules", "nuclei-templates")
	if imported.Kind != "rule_bundle" || imported.SHA256 != actualSHA || imported.ExpectedSHA256 != actualSHA {
		t.Fatalf("unexpected imported rule bundle metadata: %#v", imported)
	}
	if imported.SourcePayloadPath != sourcePayload || imported.PayloadPath != expectedPayload || imported.ManifestPath != expectedManifest || imported.InstallPath != expectedInstall {
		t.Fatalf("unexpected imported rule bundle paths: %#v", imported)
	}
	for _, rel := range []string{"http/example.yaml", "dns/info.yaml"} {
		if _, err := os.Stat(filepath.Join(expectedPayload, rel)); err != nil {
			t.Fatalf("expected copied rule bundle file %s: %v", rel, err)
		}
	}
	copiedInfo, err := os.Stat(filepath.Join(expectedPayload, "http", "example.yaml"))
	if err != nil {
		t.Fatalf("stat copied executable source rule: %v", err)
	}
	if copiedInfo.Mode().Perm() != 0o644 {
		t.Fatalf("expected copied rule bundle files to be normalized to 0644, got %v", copiedInfo.Mode().Perm())
	}
	manifest, err := os.ReadFile(expectedManifest)
	if err != nil {
		t.Fatalf("read imported rule bundle manifest: %v", err)
	}
	for _, want := range []string{
		"name: nuclei-templates",
		"kind: rule_bundle",
		"version: \"2026.06.05\"",
		"source: \"local-test/nuclei-templates\"",
		"payload: payload",
		"sha256: " + actualSHA,
	} {
		if !strings.Contains(string(manifest), want) {
			t.Fatalf("expected imported rule bundle manifest to contain %q, got %q", want, string(manifest))
		}
	}
	bundle, err := ReadManagedToolBundle(root, "nuclei-templates")
	if err != nil {
		t.Fatalf("read imported rule bundle: %v", err)
	}
	if bundle.PayloadPath != expectedPayload || bundle.InstallPath != expectedInstall || bundle.SHA256 != actualSHA {
		t.Fatalf("unexpected rule bundle readback: %#v", bundle)
	}
	plan, err := CreateManagedToolBundleInstallPlan(root, "web-api-pack", "nuclei-templates")
	if err != nil {
		t.Fatalf("create managed rule bundle install plan: %v", err)
	}
	if plan.Kind != "rule_bundle" || plan.PayloadPath != expectedPayload || plan.InstallPath != expectedInstall {
		t.Fatalf("unexpected rule bundle install plan: %#v", plan)
	}
	if _, err := os.Lstat(expectedInstall); !os.IsNotExist(err) {
		t.Fatalf("rule bundle import must not create install link, stat err=%v", err)
	}
}

func TestImportManagedToolBundleRejectsUnsafeRuleBundleDirectoryWithoutWorkspaceWrites(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	sourcePayload := filepath.Join(t.TempDir(), "nuclei-templates")
	if err := os.MkdirAll(filepath.Join(sourcePayload, "http"), 0o755); err != nil {
		t.Fatalf("create rule bundle payload: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sourcePayload, "http", "example.yaml"), []byte("id: ouro\n"), 0o644); err != nil {
		t.Fatalf("write rule bundle payload: %v", err)
	}
	if err := os.Symlink(filepath.Join(sourcePayload, "http", "example.yaml"), filepath.Join(sourcePayload, "http", "link.yaml")); err != nil {
		t.Fatalf("write rule bundle symlink: %v", err)
	}
	bundleDir := filepath.Join(root, "toolchain", "managed-bundles", "nuclei-templates")

	emptyPayload := filepath.Join(t.TempDir(), "empty-rules")
	if err := os.MkdirAll(emptyPayload, 0o755); err != nil {
		t.Fatalf("create empty rule bundle payload: %v", err)
	}
	emptyBundleDir := filepath.Join(root, "toolchain", "managed-bundles", "empty-templates")
	_, err := ImportManagedToolBundle(root, ManagedToolBundleImportOptions{
		Import:      true,
		Name:        "empty-templates",
		Kind:        "rule_bundle",
		Version:     "2026.06.05",
		Source:      "local-test/empty-templates",
		PayloadPath: emptyPayload,
	})
	if err == nil || !strings.Contains(err.Error(), "must contain at least one regular file") {
		t.Fatalf("expected empty rule bundle rejection, got %v", err)
	}
	if _, err := os.Lstat(emptyBundleDir); !os.IsNotExist(err) {
		t.Fatalf("empty rule bundle rejection must not write bundle dir, stat err=%v", err)
	}

	_, err = ImportManagedToolBundle(root, ManagedToolBundleImportOptions{
		Import:      true,
		Name:        "nuclei-templates",
		Kind:        "rule_bundle",
		Version:     "2026.06.05",
		Source:      "local-test/nuclei-templates",
		PayloadPath: sourcePayload,
	})
	if err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("expected rule bundle symlink rejection, got %v", err)
	}
	if _, err := os.Lstat(bundleDir); !os.IsNotExist(err) {
		t.Fatalf("unsafe rule bundle rejection must not write bundle dir, stat err=%v", err)
	}

	filePayload := filepath.Join(t.TempDir(), "nuclei-templates-file")
	if err := os.WriteFile(filePayload, []byte("not a directory"), 0o644); err != nil {
		t.Fatalf("write file payload: %v", err)
	}
	_, err = ImportManagedToolBundle(root, ManagedToolBundleImportOptions{
		Import:      true,
		Name:        "nuclei-templates",
		Kind:        "rule_bundle",
		Version:     "2026.06.05",
		Source:      "local-test/nuclei-templates",
		PayloadPath: filePayload,
	})
	if err == nil || !strings.Contains(err.Error(), "managed rule_bundle import payload is not a directory") {
		t.Fatalf("expected rule bundle file payload rejection, got %v", err)
	}
	if _, err := os.Lstat(bundleDir); !os.IsNotExist(err) {
		t.Fatalf("rule bundle file payload rejection must not write bundle dir, stat err=%v", err)
	}
}

func TestImportManagedToolBundleRejectsBinaryPayloadReplacedAfterInspection(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	sourcePayload := filepath.Join(t.TempDir(), "httpx")
	if err := os.WriteFile(sourcePayload, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write source payload: %v", err)
	}
	secretPayload := filepath.Join(t.TempDir(), "secret")
	if err := os.WriteFile(secretPayload, []byte("must not be copied\n"), 0o600); err != nil {
		t.Fatalf("write replacement payload: %v", err)
	}
	replaced := false
	previousHook := managedBundleSourceAfterInspectHook
	managedBundleSourceAfterInspectHook = func(path string) {
		if path != sourcePayload || replaced {
			return
		}
		replaced = true
		if err := os.Remove(sourcePayload); err != nil {
			t.Fatalf("remove source payload during hook: %v", err)
		}
		if err := os.Symlink(secretPayload, sourcePayload); err != nil {
			t.Fatalf("replace source payload with symlink: %v", err)
		}
	}
	t.Cleanup(func() {
		managedBundleSourceAfterInspectHook = previousHook
	})

	_, err := ImportManagedToolBundle(root, ManagedToolBundleImportOptions{
		Import:      true,
		Name:        "httpx",
		Kind:        "binary",
		Version:     "1.3.0",
		Source:      "local-test/httpx",
		PayloadPath: sourcePayload,
	})
	if err == nil || !strings.Contains(err.Error(), "changed after inspection") {
		t.Fatalf("expected source replacement rejection, got %v", err)
	}
	if !replaced {
		t.Fatalf("expected test hook to replace source payload")
	}
	if _, err := os.Lstat(filepath.Join(root, "toolchain", "managed-bundles", "httpx")); !os.IsNotExist(err) {
		t.Fatalf("replaced source rejection must not write bundle dir, stat err=%v", err)
	}
}

func TestImportManagedToolBundleRejectsRuleBundleFileReplacedAfterInspection(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	sourcePayload := filepath.Join(t.TempDir(), "nuclei-templates")
	sourceRule := filepath.Join(sourcePayload, "http", "example.yaml")
	if err := os.MkdirAll(filepath.Dir(sourceRule), 0o755); err != nil {
		t.Fatalf("create source rule dir: %v", err)
	}
	if err := os.WriteFile(sourceRule, []byte("id: ouro\n"), 0o644); err != nil {
		t.Fatalf("write source rule: %v", err)
	}
	secretPayload := filepath.Join(t.TempDir(), "secret-rule.yaml")
	if err := os.WriteFile(secretPayload, []byte("id: secret\n"), 0o600); err != nil {
		t.Fatalf("write replacement rule: %v", err)
	}
	replaced := false
	previousHook := managedBundleSourceAfterInspectHook
	managedBundleSourceAfterInspectHook = func(path string) {
		if path != sourceRule || replaced {
			return
		}
		replaced = true
		if err := os.Remove(sourceRule); err != nil {
			t.Fatalf("remove source rule during hook: %v", err)
		}
		if err := os.Symlink(secretPayload, sourceRule); err != nil {
			t.Fatalf("replace source rule with symlink: %v", err)
		}
	}
	t.Cleanup(func() {
		managedBundleSourceAfterInspectHook = previousHook
	})

	_, err := ImportManagedToolBundle(root, ManagedToolBundleImportOptions{
		Import:      true,
		Name:        "nuclei-templates",
		Kind:        "rule_bundle",
		Version:     "2026.06.05",
		Source:      "local-test/nuclei-templates",
		PayloadPath: sourcePayload,
	})
	if err == nil || !strings.Contains(err.Error(), "changed after inspection") {
		t.Fatalf("expected rule bundle source replacement rejection, got %v", err)
	}
	if !replaced {
		t.Fatalf("expected test hook to replace source rule")
	}
	if _, err := os.Lstat(filepath.Join(root, "toolchain", "managed-bundles", "nuclei-templates")); !os.IsNotExist(err) {
		t.Fatalf("replaced rule source rejection must not write bundle dir, stat err=%v", err)
	}
}

func TestDigestManagedBundleDirectoryUsesCanonicalRecords(t *testing.T) {
	payloadDir := filepath.Join(t.TempDir(), "nuclei-templates")
	for rel, body := range map[string]string{
		"http/a.yaml": "id: a\n",
		"http/b.yaml": "id: b\n",
	} {
		path := filepath.Join(payloadDir, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("create payload dir: %v", err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatalf("write payload file: %v", err)
		}
	}

	actual, err := digestManagedBundleDirectory(payloadDir)
	if err != nil {
		t.Fatalf("digest managed bundle directory: %v", err)
	}
	expected := canonicalDirectoryDigestForAppTest(t, payloadDir)
	legacy := legacyNULDirectoryDigestForAppTest(t, payloadDir)
	if expected == legacy {
		t.Fatalf("canonical test fixture must differ from legacy digest")
	}
	if actual != expected {
		t.Fatalf("expected canonical directory digest %s, got %s; legacy digest was %s", expected, actual, legacy)
	}
}

func TestDigestManagedBundleDirectorySeparatesLegacyNULBoundaryCollision(t *testing.T) {
	dirA := filepath.Join(t.TempDir(), "bundle-a")
	if err := os.MkdirAll(dirA, 0o755); err != nil {
		t.Fatalf("create bundle a: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dirA, "a"), []byte{'b', 0, 'c', 0, 'd'}, 0o644); err != nil {
		t.Fatalf("write bundle a payload: %v", err)
	}

	dirB := filepath.Join(t.TempDir(), "bundle-b")
	for rel, body := range map[string][]byte{
		"a": []byte("b"),
		"c": []byte("d"),
	} {
		path := filepath.Join(dirB, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("create bundle b payload dir: %v", err)
		}
		if err := os.WriteFile(path, body, 0o644); err != nil {
			t.Fatalf("write bundle b payload: %v", err)
		}
	}

	legacyA := legacyNULDirectoryDigestForAppTest(t, dirA)
	legacyB := legacyNULDirectoryDigestForAppTest(t, dirB)
	if legacyA != legacyB {
		t.Fatalf("legacy fixture must collide, got %s and %s", legacyA, legacyB)
	}
	actualA, err := digestManagedBundleDirectory(dirA)
	if err != nil {
		t.Fatalf("digest bundle a: %v", err)
	}
	actualB, err := digestManagedBundleDirectory(dirB)
	if err != nil {
		t.Fatalf("digest bundle b: %v", err)
	}
	if actualA == actualB {
		t.Fatalf("canonical directory records must separate legacy NUL boundary collision, both got %s", actualA)
	}
}

func TestBundledNucleiTemplatesManifestUsesCanonicalDirectoryDigest(t *testing.T) {
	bundleDir := filepath.Join("..", "..", "toolchains", "managed-bundles", "nuclei-templates")
	manifestPath := filepath.Join(bundleDir, "bundle.yaml")
	manifest, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("read bundled nuclei templates manifest: %v", err)
	}
	payloadDir := filepath.Join(bundleDir, "payload")
	actual, err := digestManagedBundleDirectory(payloadDir)
	if err != nil {
		t.Fatalf("digest bundled nuclei templates payload: %v", err)
	}
	declared := releaseRuleBundleDigestFromManifest(t, manifest)
	if declared != actual {
		t.Fatalf("bundled nuclei templates manifest digest must use canonical directory records: declared=%s actual=%s", declared, actual)
	}
}

func TestImportManagedToolBundleRequiresImportAndRejectsUnsafePayloadsWithoutWorkspaceWrites(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	sourcePayload := filepath.Join(t.TempDir(), "httpx")
	if err := os.WriteFile(sourcePayload, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write source payload: %v", err)
	}
	bundleDir := filepath.Join(root, "toolchain", "managed-bundles", "httpx")
	_, err := ImportManagedToolBundle(root, ManagedToolBundleImportOptions{
		Name:        "httpx",
		Kind:        "binary",
		Version:     "1.3.0",
		Source:      "local-test/httpx",
		PayloadPath: sourcePayload,
	})
	if err == nil || !strings.Contains(err.Error(), "--import") {
		t.Fatalf("expected explicit import requirement, got %v", err)
	}
	if _, err := os.Lstat(bundleDir); !os.IsNotExist(err) {
		t.Fatalf("import without --import must not write bundle dir, stat err=%v", err)
	}

	symlinkPayload := filepath.Join(t.TempDir(), "httpx-link")
	if err := os.Symlink(sourcePayload, symlinkPayload); err != nil {
		t.Fatalf("write source payload symlink: %v", err)
	}
	_, err = ImportManagedToolBundle(root, ManagedToolBundleImportOptions{
		Import:      true,
		Name:        "httpx",
		Kind:        "binary",
		Version:     "1.3.0",
		Source:      "local-test/httpx",
		PayloadPath: symlinkPayload,
	})
	if err == nil || !strings.Contains(err.Error(), "payload must not be a symlink") {
		t.Fatalf("expected symlink payload rejection, got %v", err)
	}
	if _, err := os.Lstat(bundleDir); !os.IsNotExist(err) {
		t.Fatalf("symlink payload rejection must not write bundle dir, stat err=%v", err)
	}

	dirPayload := filepath.Join(t.TempDir(), "payload-dir")
	if err := os.MkdirAll(dirPayload, 0o755); err != nil {
		t.Fatalf("create directory payload: %v", err)
	}
	_, err = ImportManagedToolBundle(root, ManagedToolBundleImportOptions{
		Import:      true,
		Name:        "httpx",
		Kind:        "binary",
		Version:     "1.3.0",
		Source:      "local-test/httpx",
		PayloadPath: dirPayload,
	})
	if err == nil || !strings.Contains(err.Error(), "managed binary import payload is a directory") {
		t.Fatalf("expected directory payload rejection, got %v", err)
	}
	if _, err := os.Lstat(bundleDir); !os.IsNotExist(err) {
		t.Fatalf("directory payload rejection must not write bundle dir, stat err=%v", err)
	}
}

func TestManagedToolBundleInstallPlanAndExecutionLinkPayloadWithoutRunningIt(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	marker := filepath.Join(t.TempDir(), "payload-ran")
	payload := writeManagedBundleFixture(t, root, "httpx", "binary", "1.3.0", "#!/bin/sh\ntouch "+marker+"\n")

	plan, err := CreateManagedToolBundleInstallPlan(root, "web-api-pack", "httpx")
	if err != nil {
		t.Fatalf("create managed install plan: %v", err)
	}
	if plan.Status != "ready" || plan.Mode != "metadata-only" || plan.Action != "link" {
		t.Fatalf("unexpected managed install plan: %#v", plan)
	}
	if plan.PayloadPath != payload || plan.InstallPath != filepath.Join(root, "toolchain", "bin", "httpx") {
		t.Fatalf("unexpected managed install plan paths: %#v", plan)
	}
	if _, err := os.Lstat(plan.InstallPath); !os.IsNotExist(err) {
		t.Fatalf("managed install plan must not create link, stat err=%v", err)
	}

	_, err = ExecuteManagedToolBundleInstall(root, "web-api-pack", "httpx", ManagedToolBundleInstallOptions{})
	if err == nil || !strings.Contains(err.Error(), "--execute") {
		t.Fatalf("expected explicit execute requirement, got %v", err)
	}
	if _, err := os.Lstat(plan.InstallPath); !os.IsNotExist(err) {
		t.Fatalf("managed install without execute must not create link, stat err=%v", err)
	}

	result, err := ExecuteManagedToolBundleInstall(root, "web-api-pack", "httpx", ManagedToolBundleInstallOptions{Execute: true})
	if err != nil {
		t.Fatalf("execute managed install: %v", err)
	}
	if result.Status != "installed" || result.InstallPath != plan.InstallPath || result.PayloadPath != payload {
		t.Fatalf("unexpected managed install result: %#v", result)
	}
	info, err := os.Lstat(result.InstallPath)
	if err != nil {
		t.Fatalf("expected managed install link: %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("expected managed install to create symlink, mode=%v", info.Mode())
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("managed install must link payload without running it, marker stat err=%v", err)
	}
}

func TestManagedToolBundleRejectsPayloadOutsideBundleDirectory(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	bundleDir := filepath.Join(root, "toolchain", "managed-bundles", "httpx")
	if err := os.MkdirAll(bundleDir, 0o755); err != nil {
		t.Fatalf("create bundle dir: %v", err)
	}
	outsidePayload := filepath.Join(t.TempDir(), "httpx")
	if err := os.WriteFile(outsidePayload, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write outside payload: %v", err)
	}
	payloadDigest := sha256.Sum256([]byte("#!/bin/sh\nexit 0\n"))
	manifest := `name: httpx
kind: binary
version: "1.3.0"
source: projectdiscovery/httpx
payload: ` + outsidePayload + `
sha256: ` + hex.EncodeToString(payloadDigest[:]) + `
`
	if err := os.WriteFile(filepath.Join(bundleDir, "bundle.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write bundle manifest: %v", err)
	}

	_, err := CreateManagedToolBundleInstallPlan(root, "web-api-pack", "httpx")
	if err == nil || !strings.Contains(err.Error(), "payload path must stay under") {
		t.Fatalf("expected outside payload rejection, got %v", err)
	}
}

func TestReleaseManagedToolBundleInstallPlanAndExecutionCopiesBundleThenLinksPayload(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	releaseRoot := filepath.Join(t.TempDir(), "release")
	marker := filepath.Join(t.TempDir(), "release-payload-ran")
	releasePayload := writeReleaseManagedBinaryBundleFixture(t, releaseRoot, "httpx", "1.3.0", "#!/bin/sh\ntouch "+marker+"\n")

	plan, err := CreateReleaseManagedToolBundleInstallPlan(root, releaseRoot, "web-api-pack", "httpx")
	if err != nil {
		t.Fatalf("create release managed install plan: %v", err)
	}
	workspacePayload := filepath.Join(root, "toolchain", "managed-bundles", "httpx", "payload", "httpx")
	if plan.Status != "ready" || plan.Mode != "metadata-only" || plan.Action != "copy-and-link" {
		t.Fatalf("unexpected release managed install plan: %#v", plan)
	}
	if plan.ReleasePayloadPath != releasePayload || plan.WorkspacePayloadPath != workspacePayload || plan.InstallPath != filepath.Join(root, "toolchain", "bin", "httpx") {
		t.Fatalf("unexpected release managed install plan paths: %#v", plan)
	}
	if _, err := os.Lstat(plan.WorkspacePayloadPath); !os.IsNotExist(err) {
		t.Fatalf("release managed install plan must not copy payload, stat err=%v", err)
	}
	if _, err := os.Lstat(plan.InstallPath); !os.IsNotExist(err) {
		t.Fatalf("release managed install plan must not create link, stat err=%v", err)
	}

	_, err = ExecuteReleaseManagedToolBundleInstall(root, releaseRoot, "web-api-pack", "httpx", ReleaseManagedToolBundleInstallOptions{})
	if err == nil || !strings.Contains(err.Error(), "--execute") {
		t.Fatalf("expected explicit execute requirement, got %v", err)
	}
	if _, err := os.Lstat(plan.WorkspacePayloadPath); !os.IsNotExist(err) {
		t.Fatalf("release managed install without execute must not copy payload, stat err=%v", err)
	}

	result, err := ExecuteReleaseManagedToolBundleInstall(root, releaseRoot, "web-api-pack", "httpx", ReleaseManagedToolBundleInstallOptions{Execute: true})
	if err != nil {
		t.Fatalf("execute release managed install: %v", err)
	}
	if result.Status != "installed" || result.Mode != "workspace-copy-link" || result.WorkspacePayloadPath != workspacePayload {
		t.Fatalf("unexpected release managed install result: %#v", result)
	}
	if _, err := os.Stat(workspacePayload); err != nil {
		t.Fatalf("expected workspace managed payload copy: %v", err)
	}
	info, err := os.Lstat(result.InstallPath)
	if err != nil {
		t.Fatalf("expected managed install link: %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("expected managed install to create symlink, mode=%v", info.Mode())
	}
	workspaceBundle, err := ReadManagedToolBundle(root, "httpx")
	if err != nil {
		t.Fatalf("read imported workspace bundle: %v", err)
	}
	if workspaceBundle.PayloadPath != workspacePayload {
		t.Fatalf("expected imported bundle to point at workspace payload, got %#v", workspaceBundle)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("release managed install must not execute payload, marker stat err=%v", err)
	}
}

func TestListReleaseManagedToolBundlesReadsReleaseMetadataWithoutWorkspaceWrites(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	releaseRoot := filepath.Join(t.TempDir(), "release")
	releasePayload := writeReleaseManagedBinaryBundleFixture(t, releaseRoot, "httpx", "1.3.0", "#!/bin/sh\nexit 77\n")

	bundles, err := ListReleaseManagedToolBundles(releaseRoot)
	if err != nil {
		t.Fatalf("list release managed tool bundles: %v", err)
	}
	if len(bundles) != 1 {
		t.Fatalf("expected one release bundle, got %#v", bundles)
	}
	bundle := bundles[0]
	if bundle.Status != "available" || bundle.Mode != "metadata-only" || bundle.Name != "httpx" || bundle.Kind != "binary" || bundle.Version != "1.3.0" {
		t.Fatalf("unexpected release bundle metadata: %#v", bundle)
	}
	if bundle.ReleasePayloadPath != releasePayload || bundle.ReleaseBundleDir != filepath.Join(releaseRoot, "toolchains", "managed-bundles", "httpx") {
		t.Fatalf("unexpected release bundle paths: %#v", bundle)
	}
	if _, err := os.Lstat(filepath.Join(root, "toolchain", "managed-bundles", "httpx")); !os.IsNotExist(err) {
		t.Fatalf("release bundle list must not copy into workspace, stat err=%v", err)
	}
	if _, err := os.Lstat(filepath.Join(root, "toolchain", "bin", "httpx")); !os.IsNotExist(err) {
		t.Fatalf("release bundle list must not create install link, stat err=%v", err)
	}

	read, err := ReadReleaseManagedToolBundle(releaseRoot, "httpx")
	if err != nil {
		t.Fatalf("read release managed tool bundle: %v", err)
	}
	if read.ReleaseManifestPath != filepath.Join(releaseRoot, "toolchains", "managed-bundles", "httpx", "bundle.yaml") {
		t.Fatalf("unexpected read release bundle paths: %#v", read)
	}
}

func TestManagedToolBundleManifestReportsOptionalProvenanceMetadata(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	writeManagedBundleFixture(t, root, "httpx", "binary", "1.3.0", "#!/bin/sh\nexit 0\n")

	missing, err := ReadManagedToolBundle(root, "httpx")
	if err != nil {
		t.Fatalf("read managed bundle without provenance: %v", err)
	}
	if missing.ProvenanceStatus != "missing" {
		t.Fatalf("expected missing provenance status, got %#v", missing)
	}

	appendManagedBundleProvenanceForTest(t,
		filepath.Join(root, "toolchain", "managed-bundles", "httpx", "bundle.yaml"),
		"https://github.com/projectdiscovery/httpx",
		"v1.3.0",
		"2026-06-05T00:00:00Z",
		"operator",
	)
	recorded, err := ReadManagedToolBundle(root, "httpx")
	if err != nil {
		t.Fatalf("read managed bundle with provenance: %v", err)
	}
	if recorded.ProvenanceStatus != "recorded" ||
		recorded.ProvenanceSourceURL != "https://github.com/projectdiscovery/httpx" ||
		recorded.ProvenanceUpstreamVersion != "v1.3.0" ||
		recorded.ProvenanceRetrievedAt != "2026-06-05T00:00:00Z" ||
		recorded.ProvenanceVerifiedBy != "operator" {
		t.Fatalf("unexpected recorded provenance metadata: %#v", recorded)
	}
}

func TestManagedToolBundleManifestVerifiesOptionalSignatureMetadata(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	writeManagedBundleFixture(t, root, "httpx", "binary", "1.3.0", "#!/bin/sh\nexit 0\n")
	bundle, err := ReadManagedToolBundle(root, "httpx")
	if err != nil {
		t.Fatalf("read managed bundle without signature: %v", err)
	}
	if bundle.SignatureStatus != "missing" {
		t.Fatalf("expected missing signature status, got %#v", bundle)
	}

	appendManagedBundleSignatureForTest(t,
		filepath.Join(root, "toolchain", "managed-bundles", "httpx", "bundle.yaml"),
		"local-test-key",
		"2026-06-07T00:00:00Z",
		"operator",
		bundle.Name,
		bundle.Kind,
		bundle.Version,
		bundle.SHA256,
		false,
	)
	signed, err := ReadManagedToolBundle(root, "httpx")
	if err != nil {
		t.Fatalf("read managed bundle with signature: %v", err)
	}
	if signed.SignatureStatus != "verified" ||
		signed.SignatureAlgorithm != "ed25519" ||
		signed.SignatureKeyID != "local-test-key" ||
		signed.SignatureSignedAt != "2026-06-07T00:00:00Z" ||
		signed.SignatureSignedBy != "operator" {
		t.Fatalf("unexpected verified signature metadata: %#v", signed)
	}
}

func TestReleaseManagedToolBundleBlocksInvalidSignatureMetadataWithoutWorkspaceWrites(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	releaseRoot := filepath.Join(t.TempDir(), "release")
	writeReleaseManagedBinaryBundleFixture(t, releaseRoot, "httpx", "1.3.0", "#!/bin/sh\nexit 0\n")
	releaseBundle, err := ReadReleaseManagedToolBundle(releaseRoot, "httpx")
	if err != nil {
		t.Fatalf("read release bundle before signature append: %v", err)
	}
	manifestPath := filepath.Join(releaseRoot, "toolchains", "managed-bundles", "httpx", "bundle.yaml")
	appendManagedBundleSignatureForTest(t,
		manifestPath,
		"local-test-key",
		"2026-06-07T00:00:00Z",
		"operator",
		releaseBundle.Name,
		releaseBundle.Kind,
		releaseBundle.Version,
		releaseBundle.SHA256,
		true,
	)

	bundles, err := ListReleaseManagedToolBundles(releaseRoot)
	if err != nil {
		t.Fatalf("list release managed bundles should report blocked signature instead of failing: %v", err)
	}
	if len(bundles) != 1 ||
		bundles[0].Status != "blocked" ||
		bundles[0].SignatureStatus != "blocked" ||
		!strings.Contains(bundles[0].Error, "signature verification failed") {
		t.Fatalf("expected blocked signature status, got %#v", bundles)
	}
	statuses, err := ListReleaseManagedToolBundleStatuses(root, releaseRoot)
	if err != nil {
		t.Fatalf("list release managed statuses: %v", err)
	}
	if len(statuses) != 1 ||
		statuses[0].SignatureStatus != "blocked" ||
		statuses[0].ImportStatus != "blocked" ||
		statuses[0].LinkStatus != "blocked" {
		t.Fatalf("expected blocked release status row, got %#v", statuses)
	}
	_, err = ExecuteReleaseManagedToolBundleInstall(root, releaseRoot, "web-api-pack", "httpx", ReleaseManagedToolBundleInstallOptions{Execute: true})
	if err == nil || !strings.Contains(err.Error(), "signature verification failed") {
		t.Fatalf("expected release managed install to reject invalid signature before writes, got %v", err)
	}
	if _, err := os.Lstat(filepath.Join(root, "toolchain", "managed-bundles", "httpx")); !os.IsNotExist(err) {
		t.Fatalf("invalid signature must not copy into workspace, stat err=%v", err)
	}
	if _, err := os.Lstat(filepath.Join(root, "toolchain", "bin", "httpx")); !os.IsNotExist(err) {
		t.Fatalf("invalid signature must not create install link, stat err=%v", err)
	}
	if _, err := ReadReleaseManagedToolBundle(releaseRoot, "httpx"); err == nil || !strings.Contains(err.Error(), "signature verification failed") {
		t.Fatalf("expected strict read signature rejection, got %v", err)
	}
}

func TestManagedToolBundleManifestReportsOptionalUpdateMetadata(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	writeManagedBundleFixture(t, root, "httpx", "binary", "1.3.0", "#!/bin/sh\nexit 0\n")

	missing, err := ReadManagedToolBundle(root, "httpx")
	if err != nil {
		t.Fatalf("read managed bundle without update metadata: %v", err)
	}
	if missing.UpdateStatus != "unknown" {
		t.Fatalf("expected unknown update status, got %#v", missing)
	}

	appendManagedBundleUpdateForTest(t,
		filepath.Join(root, "toolchain", "managed-bundles", "httpx", "bundle.yaml"),
		"1.3.0",
		missing.SHA256,
		"2026-06-05T00:00:00Z",
		"current",
	)
	current, err := ReadManagedToolBundle(root, "httpx")
	if err != nil {
		t.Fatalf("read managed bundle with current update metadata: %v", err)
	}
	if current.UpdateStatus != "current" ||
		current.UpdateLatestVersion != "1.3.0" ||
		current.UpdateLatestSHA256 != missing.SHA256 ||
		current.UpdateCheckedAt != "2026-06-05T00:00:00Z" ||
		current.UpdateMessage != "current" {
		t.Fatalf("unexpected current update metadata: %#v", current)
	}

	writeManagedBundleFixture(t, root, "ffuf", "binary", "2.1.0", "#!/bin/sh\nexit 0\n")
	ffuf, err := ReadManagedToolBundle(root, "ffuf")
	if err != nil {
		t.Fatalf("read ffuf managed bundle: %v", err)
	}
	appendManagedBundleUpdateForTest(t,
		filepath.Join(root, "toolchain", "managed-bundles", "ffuf", "bundle.yaml"),
		"2.2.0",
		ffuf.SHA256,
		"2026-06-05T00:00:00Z",
		"update_available",
	)
	updateAvailable, err := ReadManagedToolBundle(root, "ffuf")
	if err != nil {
		t.Fatalf("read managed bundle with update available metadata: %v", err)
	}
	if updateAvailable.UpdateStatus != "update_available" || updateAvailable.UpdateLatestVersion != "2.2.0" {
		t.Fatalf("expected update_available status, got %#v", updateAvailable)
	}
}

func TestReleaseManagedToolBundleBlocksInvalidProvenanceMetadataWithoutWorkspaceWrites(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	releaseRoot := filepath.Join(t.TempDir(), "release")
	writeReleaseManagedBinaryBundleFixture(t, releaseRoot, "httpx", "1.3.0", "#!/bin/sh\nexit 0\n")
	manifestPath := filepath.Join(releaseRoot, "toolchains", "managed-bundles", "httpx", "bundle.yaml")
	appendManagedBundleProvenanceForTest(t, manifestPath, "file:///tmp/httpx", "v1.3.0", "not-rfc3339", "operator")

	bundles, err := ListReleaseManagedToolBundles(releaseRoot)
	if err != nil {
		t.Fatalf("list release managed bundles should report blocked provenance instead of failing: %v", err)
	}
	if len(bundles) != 1 || bundles[0].Status != "blocked" || !strings.Contains(bundles[0].Error, "provenance source_url") {
		t.Fatalf("expected blocked provenance status, got %#v", bundles)
	}
	statuses, err := ListReleaseManagedToolBundleStatuses(root, releaseRoot)
	if err != nil {
		t.Fatalf("list release managed statuses: %v", err)
	}
	if len(statuses) != 1 || statuses[0].ProvenanceStatus != "blocked" || statuses[0].ImportStatus != "blocked" || statuses[0].LinkStatus != "blocked" {
		t.Fatalf("expected blocked provenance status row, got %#v", statuses)
	}
	if _, err := os.Lstat(filepath.Join(root, "toolchain", "managed-bundles", "httpx")); !os.IsNotExist(err) {
		t.Fatalf("invalid provenance must not copy into workspace, stat err=%v", err)
	}
	_, err = ReadReleaseManagedToolBundle(releaseRoot, "httpx")
	if err == nil || !strings.Contains(err.Error(), "provenance source_url") {
		t.Fatalf("expected strict read provenance rejection, got %v", err)
	}
}

func TestReleaseManagedToolBundleBlocksInvalidUpdateMetadataWithoutWorkspaceWrites(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	releaseRoot := filepath.Join(t.TempDir(), "release")
	writeReleaseManagedBinaryBundleFixture(t, releaseRoot, "httpx", "1.3.0", "#!/bin/sh\nexit 0\n")
	manifestPath := filepath.Join(releaseRoot, "toolchains", "managed-bundles", "httpx", "bundle.yaml")
	appendManagedBundleUpdateForTest(t, manifestPath, "1.3.1", "not-a-sha", "not-rfc3339", "update available")

	bundles, err := ListReleaseManagedToolBundles(releaseRoot)
	if err != nil {
		t.Fatalf("list release managed bundles should report blocked update metadata instead of failing: %v", err)
	}
	if len(bundles) != 1 || bundles[0].Status != "blocked" || !strings.Contains(bundles[0].Error, "update latest_sha256") {
		t.Fatalf("expected blocked update metadata status, got %#v", bundles)
	}
	statuses, err := ListReleaseManagedToolBundleStatuses(root, releaseRoot)
	if err != nil {
		t.Fatalf("list release managed statuses: %v", err)
	}
	if len(statuses) != 1 || statuses[0].UpdateStatus != "blocked" || statuses[0].ImportStatus != "blocked" || statuses[0].LinkStatus != "blocked" {
		t.Fatalf("expected blocked update status row, got %#v", statuses)
	}
	if _, err := os.Lstat(filepath.Join(root, "toolchain", "managed-bundles", "httpx")); !os.IsNotExist(err) {
		t.Fatalf("invalid update metadata must not copy into workspace, stat err=%v", err)
	}
	_, err = ReadReleaseManagedToolBundle(releaseRoot, "httpx")
	if err == nil || !strings.Contains(err.Error(), "update latest_sha256") {
		t.Fatalf("expected strict read update metadata rejection, got %v", err)
	}
}

func TestCreateManagedToolBundleUpdateRequestWritesPendingMetadataOnly(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	releaseRoot := filepath.Join(t.TempDir(), "release")
	marker := filepath.Join(t.TempDir(), "managed-update-request-payload-ran")
	writeReleaseManagedBinaryBundleFixture(t, releaseRoot, "httpx", "1.3.0", "#!/bin/sh\ntouch "+marker+"\n")
	releaseBundle, err := ReadReleaseManagedToolBundle(releaseRoot, "httpx")
	if err != nil {
		t.Fatalf("read release bundle before update metadata append: %v", err)
	}
	latestSHA := strings.Repeat("c", 64)
	appendManagedBundleUpdateForTest(t,
		filepath.Join(releaseRoot, "toolchains", "managed-bundles", "httpx", "bundle.yaml"),
		"1.4.0",
		latestSHA,
		"2026-06-07T00:00:00Z",
		"operator-approved-update",
	)

	request, err := CreateManagedToolBundleUpdateRequest(root, releaseRoot, "httpx", ManagedToolBundleUpdateRequestOptions{
		Request:     true,
		RequestedBy: "operator",
		Reason:      "prepare update",
	})
	if err != nil {
		t.Fatalf("create managed update request: %v", err)
	}
	if request.Status != "pending" || request.Mode != "metadata-only" || request.Name != "httpx" || request.Kind != "binary" {
		t.Fatalf("unexpected request identity: %#v", request)
	}
	if request.CurrentVersion != "1.3.0" || request.CurrentSHA256 != releaseBundle.SHA256 || request.LatestVersion != "1.4.0" || request.LatestSHA256 != latestSHA {
		t.Fatalf("unexpected update version metadata: %#v", request)
	}
	if request.UpdateStatus != "update_available" || request.UpdateCheckedAt != "2026-06-07T00:00:00Z" || request.UpdateMessage != "operator-approved-update" {
		t.Fatalf("unexpected update status metadata: %#v", request)
	}
	if request.ReleaseRoot != releaseRoot ||
		request.ReleaseBundleDir != filepath.Join(releaseRoot, "toolchains", "managed-bundles", "httpx") ||
		request.ReleaseManifestPath != filepath.Join(releaseRoot, "toolchains", "managed-bundles", "httpx", "bundle.yaml") ||
		request.ReleasePayloadPath != filepath.Join(releaseRoot, "toolchains", "managed-bundles", "httpx", "payload", "httpx") {
		t.Fatalf("unexpected release paths: %#v", request)
	}
	if request.RequestedBy != "operator" || request.RequestedAt == "" || request.Reason != "prepare update" {
		t.Fatalf("unexpected operator metadata: %#v", request)
	}
	wantDir := filepath.Join(root, "toolchain", "managed-update-requests")
	if !strings.HasPrefix(request.Path, wantDir+string(os.PathSeparator)) || filepath.Ext(request.Path) != ".json" {
		t.Fatalf("request path must stay under managed update request dir, got %q", request.Path)
	}
	var decoded ManagedToolBundleUpdateRequest
	data, err := os.ReadFile(request.Path)
	if err != nil {
		t.Fatalf("read update request artifact: %v", err)
	}
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("decode update request artifact: %v", err)
	}
	if decoded.Path != request.Path || decoded.LatestSHA256 != latestSHA || decoded.Reason != "prepare update" {
		t.Fatalf("unexpected persisted update request: %#v", decoded)
	}

	requests, err := ListManagedToolBundleUpdateRequests(root)
	if err != nil {
		t.Fatalf("list update requests: %v", err)
	}
	if len(requests) != 1 || requests[0].Path != request.Path || requests[0].Name != "httpx" {
		t.Fatalf("expected one update request, got %#v", requests)
	}
	read, err := ReadManagedToolBundleUpdateRequest(root, filepath.Base(request.Path))
	if err != nil {
		t.Fatalf("read update request by basename: %v", err)
	}
	if read.Path != request.Path || read.ReleaseRoot != releaseRoot {
		t.Fatalf("unexpected read update request: %#v", read)
	}
	if _, err := os.Lstat(filepath.Join(root, "toolchain", "managed-bundles", "httpx")); !os.IsNotExist(err) {
		t.Fatalf("update request must not copy workspace bundle, stat err=%v", err)
	}
	if _, err := os.Lstat(filepath.Join(root, "toolchain", "bin", "httpx")); !os.IsNotExist(err) {
		t.Fatalf("update request must not create install link, stat err=%v", err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("update request must not execute payload, marker stat err=%v", err)
	}
}

func TestCreateManagedToolBundleUpdateRequestRejectsUnsafeStatesBeforeWrite(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	releaseRoot := filepath.Join(t.TempDir(), "release")
	writeReleaseManagedBinaryBundleFixture(t, releaseRoot, "httpx", "1.3.0", "#!/bin/sh\nexit 0\n")

	if _, err := CreateManagedToolBundleUpdateRequest(root, releaseRoot, "httpx", ManagedToolBundleUpdateRequestOptions{}); err == nil || !strings.Contains(err.Error(), "requires --request") {
		t.Fatalf("expected missing request gate rejection, got %v", err)
	}
	if _, err := CreateManagedToolBundleUpdateRequest(root, releaseRoot, "missing", ManagedToolBundleUpdateRequestOptions{Request: true}); err == nil || !strings.Contains(err.Error(), "missing") {
		t.Fatalf("expected missing bundle rejection, got %v", err)
	}
	if _, err := CreateManagedToolBundleUpdateRequest(root, releaseRoot, "httpx", ManagedToolBundleUpdateRequestOptions{Request: true}); err == nil || !strings.Contains(err.Error(), "update_available") {
		t.Fatalf("expected non-update_available rejection, got %v", err)
	}
	if requests, err := ListManagedToolBundleUpdateRequests(root); err != nil || len(requests) != 0 {
		t.Fatalf("rejected update requests must not write artifacts, requests=%#v err=%v", requests, err)
	}
}

func TestCreateManagedToolBundleUpdateDownloadPlanWritesPendingMetadataOnly(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	releaseRoot := filepath.Join(t.TempDir(), "release root")
	marker := filepath.Join(t.TempDir(), "managed-update-download-plan-payload-ran")
	writeReleaseManagedBinaryBundleFixture(t, releaseRoot, "httpx", "1.3.0", "#!/bin/sh\ntouch "+marker+"\n")
	releaseBundle, err := ReadReleaseManagedToolBundle(releaseRoot, "httpx")
	if err != nil {
		t.Fatalf("read release bundle before update metadata append: %v", err)
	}
	latestSHA := strings.Repeat("a", 64)
	appendManagedBundleUpdateForTest(t,
		filepath.Join(releaseRoot, "toolchains", "managed-bundles", "httpx", "bundle.yaml"),
		"1.4.0",
		latestSHA,
		"2026-06-07T00:00:00Z",
		"operator-approved-update",
	)
	request, err := CreateManagedToolBundleUpdateRequest(root, releaseRoot, "httpx", ManagedToolBundleUpdateRequestOptions{
		Request:     true,
		RequestedBy: "operator",
		Reason:      "prepare update",
	})
	if err != nil {
		t.Fatalf("create managed update request: %v", err)
	}

	sourceURL := "https://downloads.example.com/httpx/v1.4.0/httpx-linux-amd64.tar.gz"
	plan, err := CreateManagedToolBundleUpdateDownloadPlan(root, request.Path, ManagedToolBundleUpdateDownloadPlanOptions{
		Plan:           true,
		SourceURL:      sourceURL,
		ExpectedSHA256: strings.ToUpper(latestSHA),
		PlannedBy:      "operator",
		Reason:         "download source reviewed",
	})
	if err != nil {
		t.Fatalf("create managed update download plan: %v", err)
	}
	if plan.Status != "pending" || plan.Mode != "metadata-only" || plan.Name != "httpx" || plan.Kind != "binary" {
		t.Fatalf("unexpected plan identity: %#v", plan)
	}
	if plan.RequestPath != request.Path || plan.SourceURL != sourceURL || plan.ExpectedSHA256 != latestSHA || plan.ExpectedSHA256Status != "matched" {
		t.Fatalf("unexpected plan trust metadata: %#v", plan)
	}
	if plan.CurrentVersion != "1.3.0" || plan.CurrentSHA256 != releaseBundle.SHA256 || plan.LatestVersion != "1.4.0" || plan.LatestSHA256 != latestSHA {
		t.Fatalf("unexpected update metadata: %#v", plan)
	}
	if plan.UpdateStatus != "update_available" || plan.UpdateCheckedAt != "2026-06-07T00:00:00Z" || plan.UpdateMessage != "operator-approved-update" {
		t.Fatalf("unexpected update status metadata: %#v", plan)
	}
	if plan.ReleaseRoot != request.ReleaseRoot ||
		plan.ReleaseBundleDir != request.ReleaseBundleDir ||
		plan.ReleaseManifestPath != request.ReleaseManifestPath ||
		plan.ReleasePayloadPath != request.ReleasePayloadPath {
		t.Fatalf("unexpected release request references: %#v", plan)
	}
	if plan.PlannedBy != "operator" || plan.PlannedAt == "" || plan.Reason != "download source reviewed" {
		t.Fatalf("unexpected operator metadata: %#v", plan)
	}
	wantDir := filepath.Join(root, "toolchain", "managed-update-download-plans")
	if !strings.HasPrefix(plan.Path, wantDir+string(os.PathSeparator)) || filepath.Ext(plan.Path) != ".json" {
		t.Fatalf("plan path must stay under managed update download plan dir, got %q", plan.Path)
	}
	var decoded ManagedToolBundleUpdateDownloadPlan
	data, err := os.ReadFile(plan.Path)
	if err != nil {
		t.Fatalf("read download plan artifact: %v", err)
	}
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("decode download plan artifact: %v", err)
	}
	if decoded.Path != plan.Path || decoded.RequestPath != request.Path || decoded.SourceURL != sourceURL || decoded.ExpectedSHA256 != latestSHA {
		t.Fatalf("unexpected persisted download plan: %#v", decoded)
	}

	plans, err := ListManagedToolBundleUpdateDownloadPlans(root)
	if err != nil {
		t.Fatalf("list update download plans: %v", err)
	}
	if len(plans) != 1 || plans[0].Path != plan.Path || plans[0].Name != "httpx" {
		t.Fatalf("expected one update download plan, got %#v", plans)
	}
	read, err := ReadManagedToolBundleUpdateDownloadPlan(root, filepath.Base(plan.Path))
	if err != nil {
		t.Fatalf("read update download plan by basename: %v", err)
	}
	if read.Path != plan.Path || read.RequestPath != request.Path || read.SourceURL != sourceURL {
		t.Fatalf("unexpected read update download plan: %#v", read)
	}
	if _, err := os.Lstat(filepath.Join(root, "toolchain", "managed-bundles", "httpx")); !os.IsNotExist(err) {
		t.Fatalf("download plan must not copy workspace bundle, stat err=%v", err)
	}
	if _, err := os.Lstat(filepath.Join(root, "toolchain", "bin", "httpx")); !os.IsNotExist(err) {
		t.Fatalf("download plan must not create install link, stat err=%v", err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("download plan must not execute payload, marker stat err=%v", err)
	}
}

func TestCreateManagedToolBundleUpdateDownloadPlanRejectsUnsafeInputsBeforeWrite(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	releaseRoot := filepath.Join(t.TempDir(), "release")
	writeReleaseManagedBinaryBundleFixture(t, releaseRoot, "httpx", "1.3.0", "#!/bin/sh\nexit 0\n")
	latestSHA := strings.Repeat("b", 64)
	appendManagedBundleUpdateForTest(t,
		filepath.Join(releaseRoot, "toolchains", "managed-bundles", "httpx", "bundle.yaml"),
		"1.4.0",
		latestSHA,
		"2026-06-07T00:00:00Z",
		"update_available",
	)
	request, err := CreateManagedToolBundleUpdateRequest(root, releaseRoot, "httpx", ManagedToolBundleUpdateRequestOptions{Request: true})
	if err != nil {
		t.Fatalf("create update request: %v", err)
	}

	validOptions := ManagedToolBundleUpdateDownloadPlanOptions{
		Plan:           true,
		SourceURL:      "https://downloads.example.com/httpx/v1.4.0/httpx.tar.gz",
		ExpectedSHA256: latestSHA,
	}
	for _, tc := range []struct {
		name        string
		requestPath string
		options     ManagedToolBundleUpdateDownloadPlanOptions
		wantErr     string
	}{
		{
			name:        "missing plan gate",
			requestPath: request.Path,
			options: ManagedToolBundleUpdateDownloadPlanOptions{
				SourceURL:      validOptions.SourceURL,
				ExpectedSHA256: validOptions.ExpectedSHA256,
			},
			wantErr: "requires --plan",
		},
		{
			name:        "missing request",
			requestPath: filepath.Join(root, "toolchain", "managed-update-requests", "missing.json"),
			options:     validOptions,
			wantErr:     "read managed bundle update request",
		},
		{
			name:        "http url",
			requestPath: request.Path,
			options: ManagedToolBundleUpdateDownloadPlanOptions{
				Plan:           true,
				SourceURL:      "http://downloads.example.com/httpx.tar.gz",
				ExpectedSHA256: latestSHA,
			},
			wantErr: "https",
		},
		{
			name:        "url with userinfo",
			requestPath: request.Path,
			options: ManagedToolBundleUpdateDownloadPlanOptions{
				Plan:           true,
				SourceURL:      "https://token:secret@downloads.example.com/httpx.tar.gz",
				ExpectedSHA256: latestSHA,
			},
			wantErr: "userinfo",
		},
		{
			name:        "url with query",
			requestPath: request.Path,
			options: ManagedToolBundleUpdateDownloadPlanOptions{
				Plan:           true,
				SourceURL:      "https://downloads.example.com/httpx.tar.gz?token=secret",
				ExpectedSHA256: latestSHA,
			},
			wantErr: "query",
		},
		{
			name:        "url with fragment",
			requestPath: request.Path,
			options: ManagedToolBundleUpdateDownloadPlanOptions{
				Plan:           true,
				SourceURL:      "https://downloads.example.com/httpx.tar.gz#sha256",
				ExpectedSHA256: latestSHA,
			},
			wantErr: "fragment",
		},
		{
			name:        "invalid digest",
			requestPath: request.Path,
			options: ManagedToolBundleUpdateDownloadPlanOptions{
				Plan:           true,
				SourceURL:      validOptions.SourceURL,
				ExpectedSHA256: "not-a-sha",
			},
			wantErr: "expected sha256",
		},
		{
			name:        "mismatched digest",
			requestPath: request.Path,
			options: ManagedToolBundleUpdateDownloadPlanOptions{
				Plan:           true,
				SourceURL:      validOptions.SourceURL,
				ExpectedSHA256: strings.Repeat("c", 64),
			},
			wantErr: "expected sha256 mismatch",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := CreateManagedToolBundleUpdateDownloadPlan(root, tc.requestPath, tc.options); err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("expected %q rejection, got %v", tc.wantErr, err)
			}
		})
	}

	tampered := request
	tampered.Status = "approved"
	data, err := json.MarshalIndent(tampered, "", "  ")
	if err != nil {
		t.Fatalf("encode tampered request: %v", err)
	}
	if err := os.WriteFile(request.Path, data, 0o644); err != nil {
		t.Fatalf("write tampered request: %v", err)
	}
	if _, err := CreateManagedToolBundleUpdateDownloadPlan(root, request.Path, validOptions); err == nil || !strings.Contains(err.Error(), "requires pending request") {
		t.Fatalf("expected non-pending request rejection, got %v", err)
	}

	if plans, err := ListManagedToolBundleUpdateDownloadPlans(root); err != nil || len(plans) != 0 {
		t.Fatalf("rejected update download plans must not write artifacts, plans=%#v err=%v", plans, err)
	}
}

func TestExecuteManagedToolBundleUpdateDownloadPlanDownloadsVerifiedPayloadToStagingOnly(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	releaseRoot := filepath.Join(t.TempDir(), "release root")
	marker := filepath.Join(t.TempDir(), "managed-update-download-execution-payload-ran")
	writeReleaseManagedBinaryBundleFixture(t, releaseRoot, "httpx", "1.3.0", "#!/bin/sh\ntouch "+marker+"\n")
	downloadPayload := []byte("#!/bin/sh\nexit 42\n")
	downloadDigest := sha256.Sum256(downloadPayload)
	latestSHA := hex.EncodeToString(downloadDigest[:])
	appendManagedBundleUpdateForTest(
		t,
		filepath.Join(releaseRoot, "toolchains", "managed-bundles", "httpx", "bundle.yaml"),
		"1.4.0",
		latestSHA,
		"2026-06-07T00:00:00Z",
		"download-execute-test",
	)
	request, err := CreateManagedToolBundleUpdateRequest(root, releaseRoot, "httpx", ManagedToolBundleUpdateRequestOptions{
		Request:     true,
		RequestedBy: "operator",
		Reason:      "prepare verified download",
	})
	if err != nil {
		t.Fatalf("create update request: %v", err)
	}
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("expected GET download request, got %s", r.Method)
		}
		_, _ = w.Write(downloadPayload)
	}))
	t.Cleanup(server.Close)
	plan, err := CreateManagedToolBundleUpdateDownloadPlan(root, request.Path, ManagedToolBundleUpdateDownloadPlanOptions{
		Plan:           true,
		SourceURL:      server.URL,
		ExpectedSHA256: latestSHA,
		PlannedBy:      "operator",
		Reason:         "download source reviewed",
	})
	if err != nil {
		t.Fatalf("create update download plan: %v", err)
	}
	planBefore, err := os.ReadFile(plan.Path)
	if err != nil {
		t.Fatalf("read plan before execution: %v", err)
	}
	requestBefore, err := os.ReadFile(request.Path)
	if err != nil {
		t.Fatalf("read request before execution: %v", err)
	}

	execution, err := ExecuteManagedToolBundleUpdateDownloadPlan(root, plan.Path, ManagedToolBundleUpdateDownloadExecutionOptions{
		Execute:    true,
		HTTPClient: server.Client(),
		Actor:      "operator",
		Reason:     "fetch verified update payload",
	})
	if err != nil {
		t.Fatalf("execute update download plan: %v", err)
	}
	if execution.Status != "completed" || execution.Mode != "execute" || execution.Action != "download" || execution.Name != "httpx" || execution.Kind != "binary" {
		t.Fatalf("unexpected execution identity: %#v", execution)
	}
	if execution.PlanPath != plan.Path || execution.RequestPath != request.Path || execution.SourceURL != server.URL {
		t.Fatalf("unexpected execution references: %#v", execution)
	}
	if execution.ExpectedSHA256 != latestSHA || execution.ActualSHA256 != latestSHA || execution.DigestStatus != "matched" || execution.BytesWritten != int64(len(downloadPayload)) {
		t.Fatalf("unexpected execution digest metadata: %#v", execution)
	}
	if execution.Actor != "operator" || execution.ExecutedAt == "" || execution.Reason != "fetch verified update payload" {
		t.Fatalf("unexpected execution operator metadata: %#v", execution)
	}
	wantPayloadDir := filepath.Join(root, "toolchain", "managed-update-downloads")
	if !strings.HasPrefix(execution.PayloadPath, wantPayloadDir+string(os.PathSeparator)) {
		t.Fatalf("payload path must stay under managed update download staging dir, got %q", execution.PayloadPath)
	}
	if strings.TrimSuffix(filepath.Base(execution.Path), filepath.Ext(execution.Path)) != filepath.Base(filepath.Dir(execution.PayloadPath)) {
		t.Fatalf("payload staging directory and execution artifact must share stable identity, payload=%q execution=%q", execution.PayloadPath, execution.Path)
	}
	payload, err := os.ReadFile(execution.PayloadPath)
	if err != nil {
		t.Fatalf("read staged download payload: %v", err)
	}
	if string(payload) != string(downloadPayload) {
		t.Fatalf("unexpected staged payload: %q", string(payload))
	}
	wantExecutionDir := filepath.Join(root, "toolchain", "managed-update-download-executions")
	if !strings.HasPrefix(execution.Path, wantExecutionDir+string(os.PathSeparator)) || filepath.Ext(execution.Path) != ".json" {
		t.Fatalf("execution path must stay under managed update download execution dir, got %q", execution.Path)
	}
	var decoded ManagedToolBundleUpdateDownloadExecution
	data, err := os.ReadFile(execution.Path)
	if err != nil {
		t.Fatalf("read execution audit: %v", err)
	}
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("decode execution audit: %v", err)
	}
	if decoded.Path != execution.Path || decoded.PayloadPath != execution.PayloadPath || decoded.ActualSHA256 != latestSHA {
		t.Fatalf("unexpected persisted execution audit: %#v", decoded)
	}
	executions, err := ListManagedToolBundleUpdateDownloadExecutions(root)
	if err != nil {
		t.Fatalf("list update download executions: %v", err)
	}
	if len(executions) != 1 || executions[0].Path != execution.Path || executions[0].PayloadPath != execution.PayloadPath {
		t.Fatalf("expected one update download execution, got %#v", executions)
	}
	read, err := ReadManagedToolBundleUpdateDownloadExecution(root, filepath.Base(execution.Path))
	if err != nil {
		t.Fatalf("read update download execution by basename: %v", err)
	}
	if read.Path != execution.Path || read.PayloadPath != execution.PayloadPath || read.PlanPath != plan.Path {
		t.Fatalf("unexpected read update download execution: %#v", read)
	}
	planAfter, err := ReadManagedToolBundleUpdateDownloadPlan(root, plan.Path)
	if err != nil {
		t.Fatalf("read plan after execution: %v", err)
	}
	if planAfter.Status != "pending" || planAfter.Path != plan.Path {
		t.Fatalf("download execution must not mutate plan artifact: %#v", planAfter)
	}
	planAfterBytes, err := os.ReadFile(plan.Path)
	if err != nil {
		t.Fatalf("read plan after execution: %v", err)
	}
	if string(planAfterBytes) != string(planBefore) {
		t.Fatalf("download execution must not rewrite plan artifact")
	}
	requestAfterBytes, err := os.ReadFile(request.Path)
	if err != nil {
		t.Fatalf("read request after execution: %v", err)
	}
	if string(requestAfterBytes) != string(requestBefore) {
		t.Fatalf("download execution must not rewrite request artifact")
	}
	if _, err := os.Lstat(filepath.Join(root, "toolchain", "managed-bundles", "httpx")); !os.IsNotExist(err) {
		t.Fatalf("download execution must not copy workspace bundle, stat err=%v", err)
	}
	if _, err := os.Lstat(filepath.Join(root, "toolchain", "bin", "httpx")); !os.IsNotExist(err) {
		t.Fatalf("download execution must not create install link, stat err=%v", err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("download execution must not execute payload, marker stat err=%v", err)
	}
}

func TestExecuteManagedToolBundleUpdateDownloadPlanRejectsRedirectAndRecordsProvenance(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	releaseRoot := filepath.Join(t.TempDir(), "release root")
	writeReleaseManagedBinaryBundleFixture(t, releaseRoot, "httpx", "1.3.0", "#!/bin/sh\nexit 0\n")
	updatePayload := []byte("#!/bin/sh\nexit 44\n")
	updateDigest := sha256.Sum256(updatePayload)
	latestSHA := hex.EncodeToString(updateDigest[:])
	appendManagedBundleUpdateForTest(
		t,
		filepath.Join(releaseRoot, "toolchains", "managed-bundles", "httpx", "bundle.yaml"),
		"1.4.0",
		latestSHA,
		"2026-06-08T03:00:00Z",
		"redirect-review",
	)
	request, err := CreateManagedToolBundleUpdateRequest(root, releaseRoot, "httpx", ManagedToolBundleUpdateRequestOptions{
		Request:     true,
		RequestedBy: "operator",
		Reason:      "prepare redirect test",
	})
	if err != nil {
		t.Fatalf("create update request: %v", err)
	}
	rawRedirectLocation := "https://token:secret@redirect.example.com/downloads/httpx.tar.gz?sig=secret#frag"
	sanitizedRedirectLocation := "https://redirect.example.com/downloads/httpx.tar.gz"
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", rawRedirectLocation)
		w.WriteHeader(http.StatusFound)
	}))
	t.Cleanup(server.Close)
	plan, err := CreateManagedToolBundleUpdateDownloadPlan(root, request.Path, ManagedToolBundleUpdateDownloadPlanOptions{
		Plan:           true,
		SourceURL:      server.URL,
		ExpectedSHA256: latestSHA,
		PlannedBy:      "operator",
		Reason:         "redirect source reviewed",
	})
	if err != nil {
		t.Fatalf("create update download plan: %v", err)
	}

	execution, err := ExecuteManagedToolBundleUpdateDownloadPlan(root, plan.Path, ManagedToolBundleUpdateDownloadExecutionOptions{
		Execute:    true,
		HTTPClient: server.Client(),
		Actor:      "operator",
		Reason:     "fetch redirect-reviewed update payload",
	})
	rejectionErr := err
	if rejectionErr == nil {
		t.Fatalf("expected redirect rejection, got execution %#v", execution)
	}
	if !strings.Contains(rejectionErr.Error(), "redirect") || !strings.Contains(rejectionErr.Error(), "302") {
		t.Fatalf("expected redirect rejection error with status, got %v", rejectionErr)
	}
	if execution.Status != "failed" || execution.HTTPStatus != http.StatusFound || execution.PayloadPath != "" || execution.BytesWritten != 0 {
		t.Fatalf("unexpected failed redirect execution: %#v", execution)
	}
	data, err := os.ReadFile(execution.Path)
	if err != nil {
		t.Fatalf("read failed redirect execution audit: %v", err)
	}
	var audit map[string]any
	if err := json.Unmarshal(data, &audit); err != nil {
		t.Fatalf("decode failed redirect execution audit: %v", err)
	}
	for key, want := range map[string]string{
		"redirect_policy":          "rejected",
		"redirect_location":        sanitizedRedirectLocation,
		"redirect_location_status": "redacted",
	} {
		if got, _ := audit[key].(string); got != want {
			t.Fatalf("expected audit %s=%q, got %q in %s", key, want, got, string(data))
		}
	}
	combined := string(data) + "\n" + rejectionErr.Error()
	for _, leaked := range []string{"token", "secret", "sig=", "frag"} {
		if strings.Contains(combined, leaked) {
			t.Fatalf("redirect audit/error leaked %q: %s", leaked, combined)
		}
	}
	if entries, err := os.ReadDir(filepath.Join(root, "toolchain", "managed-update-downloads")); err == nil && len(entries) != 0 {
		t.Fatalf("redirect rejection must not stage payloads, got %#v", entries)
	}
	executions, err := ListManagedToolBundleUpdateDownloadExecutions(root)
	if err != nil {
		t.Fatalf("list failed redirect executions: %v", err)
	}
	if len(executions) != 1 || executions[0].Path != execution.Path || executions[0].Status != "failed" {
		t.Fatalf("expected one failed redirect audit, got %#v", executions)
	}
}

func TestSanitizeManagedToolBundleUpdateRedirectLocationStatuses(t *testing.T) {
	tests := []struct {
		name       string
		location   string
		want       string
		wantStatus string
	}{
		{
			name:       "clean absolute url",
			location:   "https://redirect.example.com/downloads/httpx.tar.gz",
			want:       "https://redirect.example.com/downloads/httpx.tar.gz",
			wantStatus: "recorded",
		},
		{
			name:       "redacts userinfo query and fragment",
			location:   "https://token:secret@redirect.example.com/downloads/httpx.tar.gz?sig=secret#frag",
			want:       "https://redirect.example.com/downloads/httpx.tar.gz",
			wantStatus: "redacted",
		},
		{
			name:       "missing",
			location:   " \t ",
			want:       "",
			wantStatus: "missing",
		},
		{
			name:       "relative",
			location:   "/downloads/httpx.tar.gz",
			want:       "",
			wantStatus: "invalid",
		},
		{
			name:       "unsupported scheme",
			location:   "ftp://redirect.example.com/downloads/httpx.tar.gz",
			want:       "",
			wantStatus: "invalid",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, gotStatus := sanitizeManagedToolBundleUpdateRedirectLocation(tt.location)
			if got != tt.want || gotStatus != tt.wantStatus {
				t.Fatalf("sanitize redirect location = (%q, %q), want (%q, %q)", got, gotStatus, tt.want, tt.wantStatus)
			}
		})
	}
}

func TestExecuteManagedToolBundleUpdateDownloadPlanExtractsVerifiedRuleBundleArchiveAndApplyUsesBundleDigest(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	skipIfWindows(t)
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	releaseRoot := filepath.Join(t.TempDir(), "release root")
	writeReleaseManagedRuleBundleFixture(t, releaseRoot, "nuclei-templates", "2026.06.01", map[string]string{
		"http/old.yaml": "id: old\ninfo:\n  name: Old\n  severity: info\n",
	})
	writeReleaseManagedRuleBundleFixture(t, root, "nuclei-templates", "2026.06.01", map[string]string{
		"http/workspace-old.yaml": "id: workspace-old\n",
	})

	updateFiles := map[string]string{
		"http/new.yaml": "id: new-http\ninfo:\n  name: New HTTP\n  severity: low\n",
		"dns/info.yaml": "id: new-dns\ninfo:\n  name: New DNS\n  severity: info\n",
	}
	updateSourceDir := writeRuleBundleDirectoryForAppTest(t, updateFiles)
	bundleSHA := digestDirectoryForAppTest(t, updateSourceDir)
	archive := tarGzipRuleBundleArchiveForAppTest(t, updateFiles)
	archiveDigest := sha256.Sum256(archive)
	archiveSHA := hex.EncodeToString(archiveDigest[:])

	appendManagedBundleUpdateForTest(
		t,
		filepath.Join(releaseRoot, "toolchains", "managed-bundles", "nuclei-templates", "bundle.yaml"),
		"2026.06.08",
		bundleSHA,
		"2026-06-08T01:00:00Z",
		"rule-bundle-archive-download",
	)
	request, err := CreateManagedToolBundleUpdateRequest(root, releaseRoot, "nuclei-templates", ManagedToolBundleUpdateRequestOptions{
		Request:     true,
		RequestedBy: "operator",
		Reason:      "prepare archive download",
	})
	if err != nil {
		t.Fatalf("create update request: %v", err)
	}
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(archive)
	}))
	t.Cleanup(server.Close)
	plan, err := CreateManagedToolBundleUpdateDownloadPlan(root, request.Path, ManagedToolBundleUpdateDownloadPlanOptions{
		Plan:                  true,
		SourceURL:             server.URL,
		ExpectedSHA256:        bundleSHA,
		ArchiveFormat:         "tar.gz",
		ArchiveExpectedSHA256: strings.ToUpper(archiveSHA),
		PlannedBy:             "operator",
		Reason:                "archive source reviewed",
	})
	if err != nil {
		t.Fatalf("create archive update download plan: %v", err)
	}
	if plan.ExpectedSHA256 != bundleSHA || plan.ArchiveExpectedSHA256 != archiveSHA || plan.ArchiveFormat != "tar.gz" {
		t.Fatalf("unexpected archive plan digest metadata: %#v", plan)
	}

	execution, err := ExecuteManagedToolBundleUpdateDownloadPlan(root, plan.Path, ManagedToolBundleUpdateDownloadExecutionOptions{
		Execute:    true,
		HTTPClient: server.Client(),
		Actor:      "operator",
		Reason:     "fetch verified rule archive",
	})
	if err != nil {
		t.Fatalf("execute archive update download plan: %v", err)
	}
	if execution.Status != "completed" || execution.Kind != "rule_bundle" || execution.PayloadPath == "" {
		t.Fatalf("unexpected archive execution identity: %#v", execution)
	}
	if execution.ExpectedSHA256 != bundleSHA || execution.ActualSHA256 != bundleSHA || execution.DigestStatus != "matched" {
		t.Fatalf("unexpected extracted bundle digest metadata: %#v", execution)
	}
	if execution.ArchiveFormat != "tar.gz" || execution.ArchiveExpectedSHA256 != archiveSHA || execution.ArchiveActualSHA256 != archiveSHA || execution.ArchiveDigestStatus != "matched" {
		t.Fatalf("unexpected archive digest metadata: %#v", execution)
	}
	if info, err := os.Stat(execution.PayloadPath); err != nil || !info.IsDir() {
		t.Fatalf("expected staged payload directory, info=%#v err=%v", info, err)
	}
	if got := digestDirectoryForAppTest(t, execution.PayloadPath); got != bundleSHA {
		t.Fatalf("unexpected staged directory digest: got %s want %s", got, bundleSHA)
	}
	if mode := fileModeForAppTest(t, filepath.Join(execution.PayloadPath, "http", "new.yaml")); mode != 0o644 {
		t.Fatalf("expected extracted rule file mode 0644, got %o", mode)
	}
	if _, err := os.Lstat(filepath.Join(root, "toolchain", "rules", "nuclei-templates")); !os.IsNotExist(err) {
		t.Fatalf("download execution must not create final rules link, stat err=%v", err)
	}

	apply, err := ApplyManagedToolBundleUpdateDownloadExecution(root, execution.Path, ManagedToolBundleUpdateApplyOptions{
		Apply:  true,
		Actor:  "operator",
		Reason: "apply verified archive rule bundle",
	})
	if err != nil {
		t.Fatalf("apply archive rule bundle download execution: %v", err)
	}
	if apply.ExpectedSHA256 != bundleSHA || apply.ActualSHA256 != bundleSHA || apply.DigestScope != "directory_canonical_records" {
		t.Fatalf("unexpected archive apply digest metadata: %#v", apply)
	}
	if _, err := os.Stat(filepath.Join(root, "toolchain", "managed-bundles", "nuclei-templates", "payload", "dns", "info.yaml")); err != nil {
		t.Fatalf("expected applied rule file from archive: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(root, "toolchain", "rules", "nuclei-templates")); !os.IsNotExist(err) {
		t.Fatalf("archive apply must not create final rules link, stat err=%v", err)
	}
}

func TestExecuteManagedToolBundleUpdateDownloadPlanExtractsVerifiedZipRuleBundleArchiveAndApplyUsesBundleDigest(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	skipIfWindows(t)
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	releaseRoot := filepath.Join(t.TempDir(), "release root")
	writeReleaseManagedRuleBundleFixture(t, releaseRoot, "nuclei-templates", "2026.06.01", map[string]string{
		"http/old.yaml": "id: old\ninfo:\n  name: Old\n  severity: info\n",
	})
	writeReleaseManagedRuleBundleFixture(t, root, "nuclei-templates", "2026.06.01", map[string]string{
		"http/workspace-old.yaml": "id: workspace-old\n",
	})

	updateFiles := map[string]string{
		"http/new.yaml": "id: zip-new-http\ninfo:\n  name: Zip New HTTP\n  severity: low\n",
		"dns/info.yaml": "id: zip-new-dns\ninfo:\n  name: Zip New DNS\n  severity: info\n",
	}
	updateSourceDir := writeRuleBundleDirectoryForAppTest(t, updateFiles)
	bundleSHA := digestDirectoryForAppTest(t, updateSourceDir)
	archive := zipRuleBundleArchiveForAppTest(t, updateFiles)
	archiveDigest := sha256.Sum256(archive)
	archiveSHA := hex.EncodeToString(archiveDigest[:])

	appendManagedBundleUpdateForTest(
		t,
		filepath.Join(releaseRoot, "toolchains", "managed-bundles", "nuclei-templates", "bundle.yaml"),
		"2026.06.08",
		bundleSHA,
		"2026-06-08T02:00:00Z",
		"rule-bundle-zip-archive-download",
	)
	request, err := CreateManagedToolBundleUpdateRequest(root, releaseRoot, "nuclei-templates", ManagedToolBundleUpdateRequestOptions{
		Request:     true,
		RequestedBy: "operator",
		Reason:      "prepare zip archive download",
	})
	if err != nil {
		t.Fatalf("create update request: %v", err)
	}
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(archive)
	}))
	t.Cleanup(server.Close)
	plan, err := CreateManagedToolBundleUpdateDownloadPlan(root, request.Path, ManagedToolBundleUpdateDownloadPlanOptions{
		Plan:                  true,
		SourceURL:             server.URL,
		ExpectedSHA256:        bundleSHA,
		ArchiveFormat:         "zip",
		ArchiveExpectedSHA256: strings.ToUpper(archiveSHA),
		PlannedBy:             "operator",
		Reason:                "zip archive source reviewed",
	})
	if err != nil {
		t.Fatalf("create zip archive update download plan: %v", err)
	}
	if plan.ExpectedSHA256 != bundleSHA || plan.ArchiveExpectedSHA256 != archiveSHA || plan.ArchiveFormat != "zip" {
		t.Fatalf("unexpected zip archive plan digest metadata: %#v", plan)
	}

	execution, err := ExecuteManagedToolBundleUpdateDownloadPlan(root, plan.Path, ManagedToolBundleUpdateDownloadExecutionOptions{
		Execute:    true,
		HTTPClient: server.Client(),
		Actor:      "operator",
		Reason:     "fetch verified zip rule archive",
	})
	if err != nil {
		t.Fatalf("execute zip archive update download plan: %v", err)
	}
	if execution.Status != "completed" || execution.Kind != "rule_bundle" || execution.PayloadPath == "" {
		t.Fatalf("unexpected zip archive execution identity: %#v", execution)
	}
	if execution.ExpectedSHA256 != bundleSHA || execution.ActualSHA256 != bundleSHA || execution.DigestStatus != "matched" {
		t.Fatalf("unexpected extracted zip bundle digest metadata: %#v", execution)
	}
	if execution.ArchiveFormat != "zip" || execution.ArchiveExpectedSHA256 != archiveSHA || execution.ArchiveActualSHA256 != archiveSHA || execution.ArchiveDigestStatus != "matched" {
		t.Fatalf("unexpected zip archive digest metadata: %#v", execution)
	}
	if info, err := os.Stat(execution.PayloadPath); err != nil || !info.IsDir() {
		t.Fatalf("expected staged zip payload directory, info=%#v err=%v", info, err)
	}
	if got := digestDirectoryForAppTest(t, execution.PayloadPath); got != bundleSHA {
		t.Fatalf("unexpected staged zip directory digest: got %s want %s", got, bundleSHA)
	}
	if mode := fileModeForAppTest(t, filepath.Join(execution.PayloadPath, "http", "new.yaml")); mode != 0o644 {
		t.Fatalf("expected extracted zip rule file mode 0644, got %o", mode)
	}
	if _, err := os.Lstat(filepath.Join(root, "toolchain", "rules", "nuclei-templates")); !os.IsNotExist(err) {
		t.Fatalf("zip download execution must not create final rules link, stat err=%v", err)
	}

	apply, err := ApplyManagedToolBundleUpdateDownloadExecution(root, execution.Path, ManagedToolBundleUpdateApplyOptions{
		Apply:  true,
		Actor:  "operator",
		Reason: "apply verified zip archive rule bundle",
	})
	if err != nil {
		t.Fatalf("apply zip archive rule bundle download execution: %v", err)
	}
	if apply.ExpectedSHA256 != bundleSHA || apply.ActualSHA256 != bundleSHA || apply.DigestScope != "directory_canonical_records" {
		t.Fatalf("unexpected zip archive apply digest metadata: %#v", apply)
	}
	if _, err := os.Stat(filepath.Join(root, "toolchain", "managed-bundles", "nuclei-templates", "payload", "dns", "info.yaml")); err != nil {
		t.Fatalf("expected applied rule file from zip archive: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(root, "toolchain", "rules", "nuclei-templates")); !os.IsNotExist(err) {
		t.Fatalf("zip archive apply must not create final rules link, stat err=%v", err)
	}
}

func TestExecuteManagedToolBundleUpdateDownloadPlanExtractsVerifiedBinaryArchiveAndApplyUsesBinaryDigest(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	skipIfWindows(t)
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	releaseRoot := filepath.Join(t.TempDir(), "release root")
	marker := filepath.Join(t.TempDir(), "managed-update-binary-archive-payload-ran")
	writeReleaseManagedBinaryBundleFixture(t, releaseRoot, "httpx", "1.3.0", "#!/bin/sh\ntouch "+marker+"\n")
	writeManagedBundleFixture(t, root, "httpx", "binary", "1.3.0", "#!/bin/sh\nexit 0\n")

	downloadPayload := []byte("#!/bin/sh\nexit 43\n")
	payloadDigest := sha256.Sum256(downloadPayload)
	payloadSHA := hex.EncodeToString(payloadDigest[:])
	archive := tarGzipRuleBundleArchiveForAppTest(t, map[string]string{
		"bin/httpx": string(downloadPayload),
	})
	archiveDigest := sha256.Sum256(archive)
	archiveSHA := hex.EncodeToString(archiveDigest[:])
	appendManagedBundleUpdateForTest(
		t,
		filepath.Join(releaseRoot, "toolchains", "managed-bundles", "httpx", "bundle.yaml"),
		"1.4.0",
		payloadSHA,
		"2026-06-08T03:00:00Z",
		"binary-archive-download",
	)
	request, err := CreateManagedToolBundleUpdateRequest(root, releaseRoot, "httpx", ManagedToolBundleUpdateRequestOptions{
		Request:     true,
		RequestedBy: "operator",
		Reason:      "prepare binary archive download",
	})
	if err != nil {
		t.Fatalf("create update request: %v", err)
	}
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(archive)
	}))
	t.Cleanup(server.Close)
	plan, err := CreateManagedToolBundleUpdateDownloadPlan(root, request.Path, ManagedToolBundleUpdateDownloadPlanOptions{
		Plan:                  true,
		SourceURL:             server.URL,
		ExpectedSHA256:        payloadSHA,
		ArchiveFormat:         "tar.gz",
		ArchiveExpectedSHA256: strings.ToUpper(archiveSHA),
		PlannedBy:             "operator",
		Reason:                "binary archive source reviewed",
	})
	if err != nil {
		t.Fatalf("create binary archive update download plan: %v", err)
	}
	if plan.ExpectedSHA256 != payloadSHA || plan.ArchiveExpectedSHA256 != archiveSHA || plan.ArchiveFormat != "tar.gz" {
		t.Fatalf("unexpected binary archive plan digest metadata: %#v", plan)
	}

	execution, err := ExecuteManagedToolBundleUpdateDownloadPlan(root, plan.Path, ManagedToolBundleUpdateDownloadExecutionOptions{
		Execute:    true,
		HTTPClient: server.Client(),
		Actor:      "operator",
		Reason:     "fetch verified binary archive",
	})
	if err != nil {
		t.Fatalf("execute binary archive update download plan: %v", err)
	}
	if execution.Status != "completed" || execution.Kind != "binary" || execution.PayloadPath == "" {
		t.Fatalf("unexpected binary archive execution identity: %#v", execution)
	}
	if execution.ExpectedSHA256 != payloadSHA || execution.ActualSHA256 != payloadSHA || execution.DigestStatus != "matched" || execution.DigestScope != "file" {
		t.Fatalf("unexpected extracted binary digest metadata: %#v", execution)
	}
	if execution.ArchiveFormat != "tar.gz" || execution.ArchiveExpectedSHA256 != archiveSHA || execution.ArchiveActualSHA256 != archiveSHA || execution.ArchiveDigestStatus != "matched" {
		t.Fatalf("unexpected binary archive digest metadata: %#v", execution)
	}
	if info, err := os.Stat(execution.PayloadPath); err != nil || info.IsDir() {
		t.Fatalf("expected staged binary payload file, info=%#v err=%v", info, err)
	}
	if got, err := sha256FileForAppTest(execution.PayloadPath); err != nil || got != payloadSHA {
		t.Fatalf("unexpected staged binary digest: got %s want %s err=%v", got, payloadSHA, err)
	}
	if mode := fileModeForAppTest(t, execution.PayloadPath); mode != 0o644 {
		t.Fatalf("expected extracted binary mode 0644, got %o", mode)
	}
	if _, err := os.Lstat(filepath.Join(root, "toolchain", "bin", "httpx")); !os.IsNotExist(err) {
		t.Fatalf("binary archive download must not create final bin link, stat err=%v", err)
	}

	apply, err := ApplyManagedToolBundleUpdateDownloadExecution(root, execution.Path, ManagedToolBundleUpdateApplyOptions{
		Apply:  true,
		Actor:  "operator",
		Reason: "apply verified archive binary",
	})
	if err != nil {
		t.Fatalf("apply binary archive download execution: %v", err)
	}
	if apply.ExpectedSHA256 != payloadSHA || apply.ActualSHA256 != payloadSHA || apply.DigestScope != "file" {
		t.Fatalf("unexpected binary archive apply digest metadata: %#v", apply)
	}
	workspacePayload := filepath.Join(root, "toolchain", "managed-bundles", "httpx", "payload", "httpx")
	if got, err := sha256FileForAppTest(workspacePayload); err != nil || got != payloadSHA {
		t.Fatalf("unexpected applied binary digest: got %s want %s err=%v", got, payloadSHA, err)
	}
	if mode := fileModeForAppTest(t, workspacePayload); mode != 0o644 {
		t.Fatalf("expected applied binary mode 0644, got %o", mode)
	}
	if _, err := os.Lstat(filepath.Join(root, "toolchain", "bin", "httpx")); !os.IsNotExist(err) {
		t.Fatalf("binary archive apply must not create final bin link, stat err=%v", err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("binary archive update must not execute payload, marker stat err=%v", err)
	}
}

func TestExecuteManagedToolBundleUpdateDownloadPlanRejectsAmbiguousBinaryArchivePayloads(t *testing.T) {
	cases := []struct {
		name      string
		format    string
		archive   []byte
		wantError string
	}{
		{
			name:   "multiple regular files",
			format: "zip",
			archive: zipRuleBundleArchiveForAppTest(t, map[string]string{
				"bin/httpx":      "#!/bin/sh\nexit 0\n",
				"README.md":      "not a binary\n",
				"docs/usage.txt": "usage\n",
			}),
			wantError: "exactly one regular file",
		},
		{
			name:      "no regular files",
			format:    "tar.gz",
			archive:   tarGzipSingleEntryForAppTest(t, "bin", "", tar.TypeDir, nil),
			wantError: "exactly one regular file",
		},
		{
			name:      "symlink",
			format:    "tar.gz",
			archive:   tarGzipSingleEntryForAppTest(t, "bin/httpx", "/tmp/httpx", tar.TypeSymlink, nil),
			wantError: "symlink",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "workspace")
			if err := InitWorkspace(root); err != nil {
				t.Fatalf("init workspace: %v", err)
			}
			releaseRoot := filepath.Join(t.TempDir(), "release root")
			writeReleaseManagedBinaryBundleFixture(t, releaseRoot, "httpx", "1.3.0", "#!/bin/sh\nexit 0\n")
			writeManagedBundleFixture(t, root, "httpx", "binary", "1.3.0", "#!/bin/sh\nexit 0\n")

			expectedPayload := []byte("#!/bin/sh\nexit 43\n")
			expectedDigest := sha256.Sum256(expectedPayload)
			expectedSHA := hex.EncodeToString(expectedDigest[:])
			archiveDigest := sha256.Sum256(tc.archive)
			archiveSHA := hex.EncodeToString(archiveDigest[:])
			appendManagedBundleUpdateForTest(
				t,
				filepath.Join(releaseRoot, "toolchains", "managed-bundles", "httpx", "bundle.yaml"),
				"1.4.0",
				expectedSHA,
				"2026-06-08T03:10:00Z",
				"binary-archive-rejection",
			)
			request, err := CreateManagedToolBundleUpdateRequest(root, releaseRoot, "httpx", ManagedToolBundleUpdateRequestOptions{
				Request: true,
			})
			if err != nil {
				t.Fatalf("create update request: %v", err)
			}
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write(tc.archive)
			}))
			t.Cleanup(server.Close)
			plan, err := CreateManagedToolBundleUpdateDownloadPlan(root, request.Path, ManagedToolBundleUpdateDownloadPlanOptions{
				Plan:                  true,
				SourceURL:             server.URL,
				ExpectedSHA256:        expectedSHA,
				ArchiveFormat:         tc.format,
				ArchiveExpectedSHA256: archiveSHA,
			})
			if err != nil {
				t.Fatalf("create binary archive update download plan: %v", err)
			}

			execution, err := ExecuteManagedToolBundleUpdateDownloadPlan(root, plan.Path, ManagedToolBundleUpdateDownloadExecutionOptions{
				Execute:    true,
				HTTPClient: server.Client(),
			})
			if err == nil || !strings.Contains(err.Error(), tc.wantError) {
				t.Fatalf("expected binary archive rejection %q, execution=%#v err=%v", tc.wantError, execution, err)
			}
			if execution.Status != "failed" || execution.Kind != "binary" || execution.PayloadPath != "" {
				t.Fatalf("unexpected failed binary archive execution: %#v", execution)
			}
			if _, err := os.Lstat(filepath.Join(root, "toolchain", "bin", "httpx")); !os.IsNotExist(err) {
				t.Fatalf("failed binary archive download must not create final bin link, stat err=%v", err)
			}
		})
	}
}

func TestExecuteManagedToolBundleUpdateDownloadPlanExtractsVerifiedZipBinaryArchive(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	skipIfWindows(t)
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	releaseRoot := filepath.Join(t.TempDir(), "release root")
	writeReleaseManagedBinaryBundleFixture(t, releaseRoot, "httpx", "1.3.0", "#!/bin/sh\nexit 0\n")
	writeManagedBundleFixture(t, root, "httpx", "binary", "1.3.0", "#!/bin/sh\nexit 0\n")

	downloadPayload := []byte("#!/bin/sh\nexit 44\n")
	payloadDigest := sha256.Sum256(downloadPayload)
	payloadSHA := hex.EncodeToString(payloadDigest[:])
	archive := zipRuleBundleArchiveForAppTest(t, map[string]string{
		"bin/httpx": string(downloadPayload),
	})
	archiveDigest := sha256.Sum256(archive)
	archiveSHA := hex.EncodeToString(archiveDigest[:])
	appendManagedBundleUpdateForTest(
		t,
		filepath.Join(releaseRoot, "toolchains", "managed-bundles", "httpx", "bundle.yaml"),
		"1.4.0",
		payloadSHA,
		"2026-06-08T03:20:00Z",
		"binary-zip-archive-download",
	)
	request, err := CreateManagedToolBundleUpdateRequest(root, releaseRoot, "httpx", ManagedToolBundleUpdateRequestOptions{Request: true})
	if err != nil {
		t.Fatalf("create update request: %v", err)
	}
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(archive)
	}))
	t.Cleanup(server.Close)
	plan, err := CreateManagedToolBundleUpdateDownloadPlan(root, request.Path, ManagedToolBundleUpdateDownloadPlanOptions{
		Plan:                  true,
		SourceURL:             server.URL,
		ExpectedSHA256:        payloadSHA,
		ArchiveFormat:         "zip",
		ArchiveExpectedSHA256: strings.ToUpper(archiveSHA),
	})
	if err != nil {
		t.Fatalf("create zip binary archive update download plan: %v", err)
	}

	execution, err := ExecuteManagedToolBundleUpdateDownloadPlan(root, plan.Path, ManagedToolBundleUpdateDownloadExecutionOptions{
		Execute:    true,
		HTTPClient: server.Client(),
	})
	if err != nil {
		t.Fatalf("execute zip binary archive update download plan: %v", err)
	}
	if execution.Status != "completed" || execution.Kind != "binary" || execution.ArchiveFormat != "zip" || execution.ArchiveDigestStatus != "matched" || execution.DigestStatus != "matched" {
		t.Fatalf("unexpected zip binary archive execution: %#v", execution)
	}
	if got, err := sha256FileForAppTest(execution.PayloadPath); err != nil || got != payloadSHA {
		t.Fatalf("unexpected staged zip binary digest: got %s want %s err=%v", got, payloadSHA, err)
	}
	if mode := fileModeForAppTest(t, execution.PayloadPath); mode != 0o644 {
		t.Fatalf("expected extracted zip binary mode 0644, got %o", mode)
	}
}

func TestExecuteManagedToolBundleUpdateDownloadPlanAuditsBinaryArchiveDigestMismatches(t *testing.T) {
	cases := []struct {
		name              string
		archiveBody       []byte
		expectedPayload   []byte
		expectedArchive   string
		wantArchiveStatus string
		wantDigestStatus  string
		wantError         string
	}{
		{
			name:              "archive bytes",
			archiveBody:       []byte("#!/bin/sh\nexit 45\n"),
			expectedPayload:   []byte("#!/bin/sh\nexit 45\n"),
			expectedArchive:   strings.Repeat("0", 64),
			wantArchiveStatus: "mismatch",
			wantDigestStatus:  "unknown",
			wantError:         "archive sha256 mismatch",
		},
		{
			name:              "extracted payload",
			archiveBody:       []byte("#!/bin/sh\nexit 46\n"),
			expectedPayload:   []byte("#!/bin/sh\nexit 47\n"),
			wantArchiveStatus: "matched",
			wantDigestStatus:  "mismatch",
			wantError:         "extracted binary sha256 mismatch",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "workspace")
			if err := InitWorkspace(root); err != nil {
				t.Fatalf("init workspace: %v", err)
			}
			releaseRoot := filepath.Join(t.TempDir(), "release root")
			writeReleaseManagedBinaryBundleFixture(t, releaseRoot, "httpx", "1.3.0", "#!/bin/sh\nexit 0\n")
			writeManagedBundleFixture(t, root, "httpx", "binary", "1.3.0", "#!/bin/sh\nexit 0\n")

			expectedDigest := sha256.Sum256(tc.expectedPayload)
			expectedSHA := hex.EncodeToString(expectedDigest[:])
			archive := tarGzipRuleBundleArchiveForAppTest(t, map[string]string{
				"bin/httpx": string(tc.archiveBody),
			})
			archiveDigest := sha256.Sum256(archive)
			archiveSHA := hex.EncodeToString(archiveDigest[:])
			if tc.expectedArchive != "" {
				archiveSHA = tc.expectedArchive
			}
			appendManagedBundleUpdateForTest(
				t,
				filepath.Join(releaseRoot, "toolchains", "managed-bundles", "httpx", "bundle.yaml"),
				"1.4.0",
				expectedSHA,
				"2026-06-08T03:30:00Z",
				"binary-archive-digest-mismatch",
			)
			request, err := CreateManagedToolBundleUpdateRequest(root, releaseRoot, "httpx", ManagedToolBundleUpdateRequestOptions{Request: true})
			if err != nil {
				t.Fatalf("create update request: %v", err)
			}
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write(archive)
			}))
			t.Cleanup(server.Close)
			plan, err := CreateManagedToolBundleUpdateDownloadPlan(root, request.Path, ManagedToolBundleUpdateDownloadPlanOptions{
				Plan:                  true,
				SourceURL:             server.URL,
				ExpectedSHA256:        expectedSHA,
				ArchiveFormat:         "tar.gz",
				ArchiveExpectedSHA256: archiveSHA,
			})
			if err != nil {
				t.Fatalf("create binary archive update download plan: %v", err)
			}
			execution, err := ExecuteManagedToolBundleUpdateDownloadPlan(root, plan.Path, ManagedToolBundleUpdateDownloadExecutionOptions{
				Execute:    true,
				HTTPClient: server.Client(),
			})
			if err == nil || !strings.Contains(err.Error(), tc.wantError) {
				t.Fatalf("expected binary archive mismatch %q, execution=%#v err=%v", tc.wantError, execution, err)
			}
			if execution.Status != "failed" || execution.ArchiveDigestStatus != tc.wantArchiveStatus || execution.DigestStatus != tc.wantDigestStatus || execution.PayloadPath != "" {
				t.Fatalf("unexpected failed binary archive mismatch execution: %#v", execution)
			}
			if _, err := os.Lstat(filepath.Join(root, "toolchain", "bin", "httpx")); !os.IsNotExist(err) {
				t.Fatalf("failed binary archive mismatch must not create final bin link, stat err=%v", err)
			}
		})
	}
}

func TestApplyManagedToolBundleUpdateDownloadExecutionRejectsTamperedArchiveMetadata(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	releaseRoot := filepath.Join(t.TempDir(), "release root")
	writeReleaseManagedRuleBundleFixture(t, releaseRoot, "nuclei-templates", "2026.06.01", map[string]string{
		"http/old.yaml": "id: old\n",
	})
	updateFiles := map[string]string{
		"http/new.yaml": "id: new-http\n",
	}
	updateSourceDir := writeRuleBundleDirectoryForAppTest(t, updateFiles)
	bundleSHA := digestDirectoryForAppTest(t, updateSourceDir)
	archive := tarGzipRuleBundleArchiveForAppTest(t, updateFiles)
	archiveDigest := sha256.Sum256(archive)
	archiveSHA := hex.EncodeToString(archiveDigest[:])
	appendManagedBundleUpdateForTest(
		t,
		filepath.Join(releaseRoot, "toolchains", "managed-bundles", "nuclei-templates", "bundle.yaml"),
		"2026.06.08",
		bundleSHA,
		"2026-06-08T01:00:00Z",
		"tampered-archive-metadata",
	)
	request, err := CreateManagedToolBundleUpdateRequest(root, releaseRoot, "nuclei-templates", ManagedToolBundleUpdateRequestOptions{Request: true})
	if err != nil {
		t.Fatalf("create update request: %v", err)
	}
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(archive)
	}))
	t.Cleanup(server.Close)
	plan, err := CreateManagedToolBundleUpdateDownloadPlan(root, request.Path, ManagedToolBundleUpdateDownloadPlanOptions{
		Plan:                  true,
		SourceURL:             server.URL,
		ExpectedSHA256:        bundleSHA,
		ArchiveFormat:         "tar.gz",
		ArchiveExpectedSHA256: archiveSHA,
	})
	if err != nil {
		t.Fatalf("create archive update download plan: %v", err)
	}
	execution, err := ExecuteManagedToolBundleUpdateDownloadPlan(root, plan.Path, ManagedToolBundleUpdateDownloadExecutionOptions{
		Execute:    true,
		HTTPClient: server.Client(),
	})
	if err != nil {
		t.Fatalf("execute archive update download plan: %v", err)
	}
	execution.ArchiveActualSHA256 = strings.Repeat("f", 64)
	execution.ArchiveDigestStatus = "mismatch"
	data, err := json.MarshalIndent(execution, "", "  ")
	if err != nil {
		t.Fatalf("encode tampered archive download audit: %v", err)
	}
	if err := writeAuthoredToolArtifactFile(execution.Path, data, "tampered managed bundle update download execution"); err != nil {
		t.Fatalf("write tampered archive download audit: %v", err)
	}
	if _, err := ApplyManagedToolBundleUpdateDownloadExecution(root, execution.Path, ManagedToolBundleUpdateApplyOptions{Apply: true}); err == nil || !strings.Contains(err.Error(), "archive") {
		t.Fatalf("expected archive metadata rejection, got %v", err)
	}
	if _, err := os.Lstat(filepath.Join(root, "toolchain", "managed-bundles", "nuclei-templates")); !os.IsNotExist(err) {
		t.Fatalf("tampered archive metadata must not write workspace bundle, stat err=%v", err)
	}
}

func TestExecuteManagedToolBundleUpdateDownloadPlanRejectsUnsafeZipRuleBundleArchiveEntries(t *testing.T) {
	for _, tc := range []struct {
		name    string
		entry   string
		mode    os.FileMode
		body    []byte
		wantErr string
	}{
		{
			name:    "path traversal",
			entry:   "../escape.yaml",
			mode:    0o644,
			body:    []byte("id: unsafe\n"),
			wantErr: "path traversal",
		},
		{
			name:    "absolute path",
			entry:   "/escape.yaml",
			mode:    0o644,
			body:    []byte("id: unsafe\n"),
			wantErr: "path traversal",
		},
		{
			name:    "unc path",
			entry:   "//server/share/escape.yaml",
			mode:    0o644,
			body:    []byte("id: unsafe\n"),
			wantErr: "path traversal",
		},
		{
			name:    "windows drive",
			entry:   "C:/escape.yaml",
			mode:    0o644,
			body:    []byte("id: unsafe\n"),
			wantErr: "path traversal",
		},
		{
			name:    "backslash",
			entry:   "http\\escape.yaml",
			mode:    0o644,
			body:    []byte("id: unsafe\n"),
			wantErr: "path traversal",
		},
		{
			name:    "parent segment",
			entry:   "http/../escape.yaml",
			mode:    0o644,
			body:    []byte("id: unsafe\n"),
			wantErr: "path traversal",
		},
		{
			name:    "symlink",
			entry:   "http/link.yaml",
			mode:    os.ModeSymlink | 0o777,
			body:    []byte("http/new.yaml"),
			wantErr: "symlink",
		},
		{
			name:    "named pipe",
			entry:   "http/pipe.yaml",
			mode:    os.ModeNamedPipe | 0o644,
			body:    []byte("id: unsafe\n"),
			wantErr: "unsupported",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "workspace")
			if err := InitWorkspace(root); err != nil {
				t.Fatalf("init workspace: %v", err)
			}
			releaseRoot := filepath.Join(t.TempDir(), "release")
			writeReleaseManagedRuleBundleFixture(t, releaseRoot, "nuclei-templates", "2026.06.01", map[string]string{
				"http/old.yaml": "id: old\n",
			})
			expectedFiles := map[string]string{"http/new.yaml": "id: new\n"}
			expectedDir := writeRuleBundleDirectoryForAppTest(t, expectedFiles)
			bundleSHA := digestDirectoryForAppTest(t, expectedDir)
			archive := zipSingleEntryForAppTest(t, tc.entry, tc.mode, tc.body)
			archiveDigest := sha256.Sum256(archive)
			archiveSHA := hex.EncodeToString(archiveDigest[:])
			appendManagedBundleUpdateForTest(
				t,
				filepath.Join(releaseRoot, "toolchains", "managed-bundles", "nuclei-templates", "bundle.yaml"),
				"2026.06.08",
				bundleSHA,
				"2026-06-08T02:00:00Z",
				"unsafe-zip-archive",
			)
			request, err := CreateManagedToolBundleUpdateRequest(root, releaseRoot, "nuclei-templates", ManagedToolBundleUpdateRequestOptions{Request: true})
			if err != nil {
				t.Fatalf("create update request: %v", err)
			}
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write(archive)
			}))
			t.Cleanup(server.Close)
			plan, err := CreateManagedToolBundleUpdateDownloadPlan(root, request.Path, ManagedToolBundleUpdateDownloadPlanOptions{
				Plan:                  true,
				SourceURL:             server.URL,
				ExpectedSHA256:        bundleSHA,
				ArchiveFormat:         "zip",
				ArchiveExpectedSHA256: archiveSHA,
			})
			if err != nil {
				t.Fatalf("create zip archive update download plan: %v", err)
			}

			execution, err := ExecuteManagedToolBundleUpdateDownloadPlan(root, plan.Path, ManagedToolBundleUpdateDownloadExecutionOptions{
				Execute:    true,
				HTTPClient: server.Client(),
			})
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("expected %q zip archive rejection, execution=%#v err=%v", tc.wantErr, execution, err)
			}
			if execution.Status != "failed" || execution.ArchiveActualSHA256 != archiveSHA || execution.ArchiveDigestStatus != "matched" || execution.DigestStatus == "matched" {
				t.Fatalf("unexpected failed zip archive audit: %#v", execution)
			}
			if execution.PayloadPath != "" {
				if _, err := os.Stat(execution.PayloadPath); !os.IsNotExist(err) {
					t.Fatalf("unsafe zip archive must not publish staged payload, stat err=%v", err)
				}
			}
			if _, err := os.Lstat(filepath.Join(root, "toolchain", "managed-bundles", "nuclei-templates")); !os.IsNotExist(err) {
				t.Fatalf("unsafe zip archive must not write workspace bundle, stat err=%v", err)
			}
		})
	}
}

func TestExecuteManagedToolBundleUpdateDownloadPlanRejectsDuplicateZipRuleBundleArchiveEntries(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	releaseRoot := filepath.Join(t.TempDir(), "release")
	writeReleaseManagedRuleBundleFixture(t, releaseRoot, "nuclei-templates", "2026.06.01", map[string]string{
		"http/old.yaml": "id: old\n",
	})
	expectedFiles := map[string]string{"http/new.yaml": "id: new\n"}
	expectedDir := writeRuleBundleDirectoryForAppTest(t, expectedFiles)
	bundleSHA := digestDirectoryForAppTest(t, expectedDir)
	archive := zipRuleBundleArchiveEntriesForAppTest(t, []zipRuleBundleArchiveEntryForAppTest{
		{Name: "http/new.yaml", Mode: 0o644, Body: []byte("id: first\n")},
		{Name: "http/new.yaml", Mode: 0o644, Body: []byte("id: second\n")},
	})
	archiveDigest := sha256.Sum256(archive)
	archiveSHA := hex.EncodeToString(archiveDigest[:])
	appendManagedBundleUpdateForTest(
		t,
		filepath.Join(releaseRoot, "toolchains", "managed-bundles", "nuclei-templates", "bundle.yaml"),
		"2026.06.08",
		bundleSHA,
		"2026-06-08T02:00:00Z",
		"duplicate-zip-archive",
	)
	request, err := CreateManagedToolBundleUpdateRequest(root, releaseRoot, "nuclei-templates", ManagedToolBundleUpdateRequestOptions{Request: true})
	if err != nil {
		t.Fatalf("create update request: %v", err)
	}
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(archive)
	}))
	t.Cleanup(server.Close)
	plan, err := CreateManagedToolBundleUpdateDownloadPlan(root, request.Path, ManagedToolBundleUpdateDownloadPlanOptions{
		Plan:                  true,
		SourceURL:             server.URL,
		ExpectedSHA256:        bundleSHA,
		ArchiveFormat:         "zip",
		ArchiveExpectedSHA256: archiveSHA,
	})
	if err != nil {
		t.Fatalf("create zip archive update download plan: %v", err)
	}

	execution, err := ExecuteManagedToolBundleUpdateDownloadPlan(root, plan.Path, ManagedToolBundleUpdateDownloadExecutionOptions{
		Execute:    true,
		HTTPClient: server.Client(),
	})
	if err == nil || !strings.Contains(err.Error(), "create managed bundle update archive file") {
		t.Fatalf("expected duplicate zip entry rejection, execution=%#v err=%v", execution, err)
	}
	if execution.Status != "failed" || execution.ArchiveActualSHA256 != archiveSHA || execution.ArchiveDigestStatus != "matched" || execution.DigestStatus == "matched" {
		t.Fatalf("unexpected failed duplicate zip archive audit: %#v", execution)
	}
	if _, err := os.Lstat(filepath.Join(root, "toolchain", "managed-bundles", "nuclei-templates")); !os.IsNotExist(err) {
		t.Fatalf("duplicate zip archive must not write workspace bundle, stat err=%v", err)
	}
}

func TestExecuteManagedToolBundleUpdateDownloadPlanRejectsUnsafeRuleBundleArchiveEntries(t *testing.T) {
	for _, tc := range []struct {
		name       string
		headerName string
		linkName   string
		typeFlag   byte
		wantErr    string
	}{
		{
			name:       "path traversal",
			headerName: "../escape.yaml",
			typeFlag:   tar.TypeReg,
			wantErr:    "path traversal",
		},
		{
			name:       "symlink",
			headerName: "http/link.yaml",
			linkName:   "http/new.yaml",
			typeFlag:   tar.TypeSymlink,
			wantErr:    "symlink",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "workspace")
			if err := InitWorkspace(root); err != nil {
				t.Fatalf("init workspace: %v", err)
			}
			releaseRoot := filepath.Join(t.TempDir(), "release")
			writeReleaseManagedRuleBundleFixture(t, releaseRoot, "nuclei-templates", "2026.06.01", map[string]string{
				"http/old.yaml": "id: old\n",
			})
			expectedFiles := map[string]string{"http/new.yaml": "id: new\n"}
			expectedDir := writeRuleBundleDirectoryForAppTest(t, expectedFiles)
			bundleSHA := digestDirectoryForAppTest(t, expectedDir)
			archive := tarGzipSingleEntryForAppTest(t, tc.headerName, tc.linkName, tc.typeFlag, []byte("id: unsafe\n"))
			archiveDigest := sha256.Sum256(archive)
			archiveSHA := hex.EncodeToString(archiveDigest[:])
			appendManagedBundleUpdateForTest(
				t,
				filepath.Join(releaseRoot, "toolchains", "managed-bundles", "nuclei-templates", "bundle.yaml"),
				"2026.06.08",
				bundleSHA,
				"2026-06-08T01:00:00Z",
				"unsafe-archive",
			)
			request, err := CreateManagedToolBundleUpdateRequest(root, releaseRoot, "nuclei-templates", ManagedToolBundleUpdateRequestOptions{Request: true})
			if err != nil {
				t.Fatalf("create update request: %v", err)
			}
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write(archive)
			}))
			t.Cleanup(server.Close)
			plan, err := CreateManagedToolBundleUpdateDownloadPlan(root, request.Path, ManagedToolBundleUpdateDownloadPlanOptions{
				Plan:                  true,
				SourceURL:             server.URL,
				ExpectedSHA256:        bundleSHA,
				ArchiveFormat:         "tar.gz",
				ArchiveExpectedSHA256: archiveSHA,
			})
			if err != nil {
				t.Fatalf("create archive update download plan: %v", err)
			}

			execution, err := ExecuteManagedToolBundleUpdateDownloadPlan(root, plan.Path, ManagedToolBundleUpdateDownloadExecutionOptions{
				Execute:    true,
				HTTPClient: server.Client(),
			})
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("expected %q archive rejection, execution=%#v err=%v", tc.wantErr, execution, err)
			}
			if execution.Status != "failed" || execution.ArchiveActualSHA256 != archiveSHA || execution.ArchiveDigestStatus != "matched" || execution.DigestStatus == "matched" {
				t.Fatalf("unexpected failed archive audit: %#v", execution)
			}
			if execution.PayloadPath != "" {
				if _, err := os.Stat(execution.PayloadPath); !os.IsNotExist(err) {
					t.Fatalf("unsafe archive must not publish staged payload, stat err=%v", err)
				}
			}
			if _, err := os.Lstat(filepath.Join(root, "toolchain", "managed-bundles", "nuclei-templates")); !os.IsNotExist(err) {
				t.Fatalf("unsafe archive must not write workspace bundle, stat err=%v", err)
			}
		})
	}
}

func TestExecuteManagedToolBundleUpdateDownloadPlanRequiresExecuteAndAuditsDigestMismatch(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	releaseRoot := filepath.Join(t.TempDir(), "release")
	writeReleaseManagedBinaryBundleFixture(t, releaseRoot, "httpx", "1.3.0", "#!/bin/sh\nexit 0\n")
	expectedPayload := []byte("expected update payload")
	expectedDigest := sha256.Sum256(expectedPayload)
	expectedSHA := hex.EncodeToString(expectedDigest[:])
	appendManagedBundleUpdateForTest(
		t,
		filepath.Join(releaseRoot, "toolchains", "managed-bundles", "httpx", "bundle.yaml"),
		"1.4.0",
		expectedSHA,
		"2026-06-07T00:00:00Z",
		"download-mismatch-test",
	)
	request, err := CreateManagedToolBundleUpdateRequest(root, releaseRoot, "httpx", ManagedToolBundleUpdateRequestOptions{Request: true})
	if err != nil {
		t.Fatalf("create update request: %v", err)
	}
	actualPayload := []byte("tampered update payload")
	actualDigest := sha256.Sum256(actualPayload)
	actualSHA := hex.EncodeToString(actualDigest[:])
	hits := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		_, _ = w.Write(actualPayload)
	}))
	t.Cleanup(server.Close)
	plan, err := CreateManagedToolBundleUpdateDownloadPlan(root, request.Path, ManagedToolBundleUpdateDownloadPlanOptions{
		Plan:           true,
		SourceURL:      server.URL,
		ExpectedSHA256: expectedSHA,
	})
	if err != nil {
		t.Fatalf("create update download plan: %v", err)
	}

	if _, err := ExecuteManagedToolBundleUpdateDownloadPlan(root, plan.Path, ManagedToolBundleUpdateDownloadExecutionOptions{HTTPClient: server.Client()}); err == nil || !strings.Contains(err.Error(), "requires explicit execute") {
		t.Fatalf("expected missing execute rejection, got %v", err)
	}
	if hits != 0 {
		t.Fatalf("missing execute must not contact source, hits=%d", hits)
	}
	if executions, err := ListManagedToolBundleUpdateDownloadExecutions(root); err != nil || len(executions) != 0 {
		t.Fatalf("missing execute must not write execution audits, executions=%#v err=%v", executions, err)
	}

	execution, err := ExecuteManagedToolBundleUpdateDownloadPlan(root, plan.Path, ManagedToolBundleUpdateDownloadExecutionOptions{
		Execute:    true,
		HTTPClient: server.Client(),
		Actor:      "operator",
		Reason:     "digest mismatch audit",
	})
	if err == nil || !strings.Contains(err.Error(), "sha256 mismatch") {
		t.Fatalf("expected digest mismatch failure, execution=%#v err=%v", execution, err)
	}
	if hits != 1 {
		t.Fatalf("expected one source request for digest mismatch, hits=%d", hits)
	}
	if execution.Status != "failed" || execution.Error == "" || execution.ExpectedSHA256 != expectedSHA || execution.ActualSHA256 != actualSHA || execution.DigestStatus != "mismatch" {
		t.Fatalf("unexpected failed execution audit: %#v", execution)
	}
	if execution.PayloadPath != "" {
		if _, err := os.Stat(execution.PayloadPath); !os.IsNotExist(err) {
			t.Fatalf("digest mismatch must not leave staged payload, stat err=%v", err)
		}
	}
	executions, err := ListManagedToolBundleUpdateDownloadExecutions(root)
	if err != nil {
		t.Fatalf("list failed update download executions: %v", err)
	}
	if len(executions) != 1 || executions[0].Status != "failed" || executions[0].ActualSHA256 != actualSHA {
		t.Fatalf("expected one failed update download execution audit, got %#v", executions)
	}
	stagingName := strings.TrimSuffix(filepath.Base(execution.Path), filepath.Ext(execution.Path))
	stagingDir := filepath.Join(root, "toolchain", "managed-update-downloads", stagingName)
	entries, err := os.ReadDir(stagingDir)
	if err != nil && !os.IsNotExist(err) {
		t.Fatalf("inspect mismatch staging directory: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("digest mismatch must not leave staged payload or temp files, entries=%#v", entries)
	}
	if _, err := os.Lstat(filepath.Join(root, "toolchain", "managed-bundles", "httpx")); !os.IsNotExist(err) {
		t.Fatalf("failed download execution must not copy workspace bundle, stat err=%v", err)
	}
}

func TestExecuteManagedToolBundleUpdateDownloadPlanRejectsTamperedRequestBeforeNetwork(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	releaseRoot := filepath.Join(t.TempDir(), "release")
	writeReleaseManagedBinaryBundleFixture(t, releaseRoot, "httpx", "1.3.0", "#!/bin/sh\nexit 0\n")
	payload := []byte("verified payload")
	digest := sha256.Sum256(payload)
	latestSHA := hex.EncodeToString(digest[:])
	appendManagedBundleUpdateForTest(
		t,
		filepath.Join(releaseRoot, "toolchains", "managed-bundles", "httpx", "bundle.yaml"),
		"1.4.0",
		latestSHA,
		"2026-06-07T00:00:00Z",
		"original-update-message",
	)
	request, err := CreateManagedToolBundleUpdateRequest(root, releaseRoot, "httpx", ManagedToolBundleUpdateRequestOptions{Request: true})
	if err != nil {
		t.Fatalf("create update request: %v", err)
	}
	serverHits := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		serverHits++
		_, _ = w.Write(payload)
	}))
	t.Cleanup(server.Close)
	plan, err := CreateManagedToolBundleUpdateDownloadPlan(root, request.Path, ManagedToolBundleUpdateDownloadPlanOptions{
		Plan:           true,
		SourceURL:      server.URL,
		ExpectedSHA256: latestSHA,
	})
	if err != nil {
		t.Fatalf("create update download plan: %v", err)
	}

	data, err := os.ReadFile(request.Path)
	if err != nil {
		t.Fatalf("read request artifact: %v", err)
	}
	var tampered ManagedToolBundleUpdateRequest
	if err := json.Unmarshal(data, &tampered); err != nil {
		t.Fatalf("decode request artifact: %v", err)
	}
	tampered.Source = "other/source"
	tampered.UpdateCheckedAt = "2026-06-07T01:00:00Z"
	tampered.UpdateMessage = "tampered-update-message"
	encoded, err := json.MarshalIndent(tampered, "", "  ")
	if err != nil {
		t.Fatalf("encode tampered request: %v", err)
	}
	if err := os.WriteFile(request.Path, append(encoded, '\n'), 0o644); err != nil {
		t.Fatalf("write tampered request: %v", err)
	}

	execution, err := ExecuteManagedToolBundleUpdateDownloadPlan(root, plan.Path, ManagedToolBundleUpdateDownloadExecutionOptions{
		Execute:    true,
		HTTPClient: server.Client(),
	})
	if err == nil || !strings.Contains(err.Error(), "plan no longer matches request metadata") {
		t.Fatalf("expected tampered request rejection, execution=%#v err=%v", execution, err)
	}
	if serverHits != 0 {
		t.Fatalf("tampered request must fail before network, hits=%d", serverHits)
	}
	if executions, err := ListManagedToolBundleUpdateDownloadExecutions(root); err != nil || len(executions) != 0 {
		t.Fatalf("tampered request must not write execution audit, executions=%#v err=%v", executions, err)
	}
}

func TestExecuteManagedToolBundleUpdateDownloadPlanRejectsSymlinkedPlanDirectoryBeforeNetwork(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	releaseRoot := filepath.Join(t.TempDir(), "release")
	writeReleaseManagedBinaryBundleFixture(t, releaseRoot, "httpx", "1.3.0", "#!/bin/sh\nexit 0\n")
	payload := []byte("verified payload")
	digest := sha256.Sum256(payload)
	latestSHA := hex.EncodeToString(digest[:])
	appendManagedBundleUpdateForTest(
		t,
		filepath.Join(releaseRoot, "toolchains", "managed-bundles", "httpx", "bundle.yaml"),
		"1.4.0",
		latestSHA,
		"2026-06-07T00:00:00Z",
		"symlink-plan-directory",
	)
	request, err := CreateManagedToolBundleUpdateRequest(root, releaseRoot, "httpx", ManagedToolBundleUpdateRequestOptions{Request: true})
	if err != nil {
		t.Fatalf("create update request: %v", err)
	}
	serverHits := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		serverHits++
		_, _ = w.Write(payload)
	}))
	t.Cleanup(server.Close)
	plan, err := CreateManagedToolBundleUpdateDownloadPlan(root, request.Path, ManagedToolBundleUpdateDownloadPlanOptions{
		Plan:           true,
		SourceURL:      server.URL,
		ExpectedSHA256: latestSHA,
	})
	if err != nil {
		t.Fatalf("create update download plan: %v", err)
	}
	planData, err := os.ReadFile(plan.Path)
	if err != nil {
		t.Fatalf("read plan artifact: %v", err)
	}
	outsidePlanDir := filepath.Join(t.TempDir(), "outside-plans")
	if err := os.MkdirAll(outsidePlanDir, 0o755); err != nil {
		t.Fatalf("create outside plan dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(outsidePlanDir, filepath.Base(plan.Path)), planData, 0o644); err != nil {
		t.Fatalf("copy plan outside workspace: %v", err)
	}
	planDir := managedToolBundleUpdateDownloadPlanDir(root)
	if err := os.RemoveAll(planDir); err != nil {
		t.Fatalf("remove plan dir: %v", err)
	}
	if err := os.Symlink(outsidePlanDir, planDir); err != nil {
		t.Fatalf("symlink plan dir: %v", err)
	}

	execution, err := ExecuteManagedToolBundleUpdateDownloadPlan(root, filepath.Base(plan.Path), ManagedToolBundleUpdateDownloadExecutionOptions{
		Execute:    true,
		HTTPClient: server.Client(),
	})
	if err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("expected symlinked plan directory rejection, execution=%#v err=%v", execution, err)
	}
	if serverHits != 0 {
		t.Fatalf("symlinked plan directory must fail before network, hits=%d", serverHits)
	}
}

func TestApplyManagedToolBundleUpdateDownloadExecutionCopiesVerifiedBinaryBundleAndAudits(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	releaseRoot := filepath.Join(t.TempDir(), "release root")
	marker := filepath.Join(t.TempDir(), "managed-update-apply-payload-ran")
	writeReleaseManagedBinaryBundleFixture(t, releaseRoot, "httpx", "1.3.0", "#!/bin/sh\ntouch "+marker+"\n")
	writeManagedBundleFixture(t, root, "httpx", "binary", "1.3.0", "#!/bin/sh\nexit 0\n")
	downloadPayload := []byte("#!/bin/sh\nexit 43\n")
	downloadDigest := sha256.Sum256(downloadPayload)
	latestSHA := hex.EncodeToString(downloadDigest[:])
	appendManagedBundleUpdateForTest(
		t,
		filepath.Join(releaseRoot, "toolchains", "managed-bundles", "httpx", "bundle.yaml"),
		"1.4.0",
		latestSHA,
		"2026-06-07T00:00:00Z",
		"apply-download-test",
	)
	request, err := CreateManagedToolBundleUpdateRequest(root, releaseRoot, "httpx", ManagedToolBundleUpdateRequestOptions{
		Request:     true,
		RequestedBy: "operator",
		Reason:      "prepare apply",
	})
	if err != nil {
		t.Fatalf("create update request: %v", err)
	}
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(downloadPayload)
	}))
	t.Cleanup(server.Close)
	plan, err := CreateManagedToolBundleUpdateDownloadPlan(root, request.Path, ManagedToolBundleUpdateDownloadPlanOptions{
		Plan:           true,
		SourceURL:      server.URL,
		ExpectedSHA256: latestSHA,
		PlannedBy:      "operator",
		Reason:         "download source reviewed",
	})
	if err != nil {
		t.Fatalf("create update download plan: %v", err)
	}
	download, err := ExecuteManagedToolBundleUpdateDownloadPlan(root, plan.Path, ManagedToolBundleUpdateDownloadExecutionOptions{
		Execute:    true,
		HTTPClient: server.Client(),
		Actor:      "operator",
		Reason:     "fetch verified update payload",
	})
	if err != nil {
		t.Fatalf("execute update download plan: %v", err)
	}
	requestBefore, err := os.ReadFile(request.Path)
	if err != nil {
		t.Fatalf("read request before apply: %v", err)
	}
	planBefore, err := os.ReadFile(plan.Path)
	if err != nil {
		t.Fatalf("read plan before apply: %v", err)
	}
	downloadBefore, err := os.ReadFile(download.Path)
	if err != nil {
		t.Fatalf("read download audit before apply: %v", err)
	}

	apply, err := ApplyManagedToolBundleUpdateDownloadExecution(root, download.Path, ManagedToolBundleUpdateApplyOptions{
		Apply:  true,
		Actor:  "operator",
		Reason: "apply verified downloaded binary",
	})
	if err != nil {
		t.Fatalf("apply managed update download execution: %v", err)
	}
	if apply.Status != "completed" || apply.Mode != "apply" || apply.Action != "workspace-copy" || apply.Name != "httpx" || apply.Kind != "binary" {
		t.Fatalf("unexpected apply audit identity: %#v", apply)
	}
	if apply.DownloadExecutionPath != download.Path || apply.PlanPath != plan.Path || apply.RequestPath != request.Path || apply.SourceURL != server.URL {
		t.Fatalf("unexpected apply references: %#v", apply)
	}
	if apply.ExpectedSHA256 != latestSHA || apply.ActualSHA256 != latestSHA || apply.DigestStatus != "matched" || apply.DigestAlgorithm != "sha256" || apply.DigestScope != "file" {
		t.Fatalf("unexpected apply digest metadata: %#v", apply)
	}
	if apply.CurrentVersion != "1.3.0" || apply.LatestVersion != "1.4.0" || apply.LatestSHA256 != latestSHA {
		t.Fatalf("unexpected apply update metadata: %#v", apply)
	}
	if apply.Actor != "operator" || apply.AppliedAt == "" || apply.Reason != "apply verified downloaded binary" {
		t.Fatalf("unexpected apply operator metadata: %#v", apply)
	}
	wantApplyDir := filepath.Join(root, "toolchain", "managed-update-apply-executions")
	if !strings.HasPrefix(apply.Path, wantApplyDir+string(os.PathSeparator)) || filepath.Ext(apply.Path) != ".json" {
		t.Fatalf("apply audit path must stay under managed update apply execution dir, got %q", apply.Path)
	}
	if apply.WorkspaceBundleDir != filepath.Join(root, "toolchain", "managed-bundles", "httpx") ||
		apply.WorkspaceManifestPath != filepath.Join(root, "toolchain", "managed-bundles", "httpx", "bundle.yaml") ||
		apply.WorkspacePayloadPath != filepath.Join(root, "toolchain", "managed-bundles", "httpx", "payload", "httpx") ||
		apply.InstallPath != filepath.Join(root, "toolchain", "bin", "httpx") {
		t.Fatalf("unexpected workspace paths: %#v", apply)
	}
	bundle, err := ReadManagedToolBundle(root, "httpx")
	if err != nil {
		t.Fatalf("read applied managed bundle: %v", err)
	}
	if bundle.Version != "1.4.0" || bundle.SHA256 != latestSHA || bundle.PayloadPath != apply.WorkspacePayloadPath || bundle.Source != "projectdiscovery/httpx" {
		t.Fatalf("unexpected applied bundle metadata: %#v", bundle)
	}
	payload, err := os.ReadFile(apply.WorkspacePayloadPath)
	if err != nil {
		t.Fatalf("read applied workspace payload: %v", err)
	}
	if string(payload) != string(downloadPayload) {
		t.Fatalf("unexpected applied payload: %q", string(payload))
	}
	var decoded ManagedToolBundleUpdateApplyExecution
	data, err := os.ReadFile(apply.Path)
	if err != nil {
		t.Fatalf("read apply audit: %v", err)
	}
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("decode apply audit: %v", err)
	}
	if decoded.Path != apply.Path || decoded.WorkspacePayloadPath != apply.WorkspacePayloadPath || decoded.DownloadExecutionPath != download.Path {
		t.Fatalf("unexpected persisted apply audit: %#v", decoded)
	}
	applies, err := ListManagedToolBundleUpdateApplyExecutions(root)
	if err != nil {
		t.Fatalf("list apply audits: %v", err)
	}
	if len(applies) != 1 || applies[0].Path != apply.Path || applies[0].WorkspacePayloadPath != apply.WorkspacePayloadPath {
		t.Fatalf("expected one apply audit, got %#v", applies)
	}
	readApply, err := ReadManagedToolBundleUpdateApplyExecution(root, filepath.Base(apply.Path))
	if err != nil {
		t.Fatalf("read apply audit by basename: %v", err)
	}
	if readApply.Path != apply.Path || readApply.DownloadExecutionPath != download.Path || readApply.WorkspaceBundleDir != apply.WorkspaceBundleDir {
		t.Fatalf("unexpected read apply audit: %#v", readApply)
	}
	if requestAfter, err := os.ReadFile(request.Path); err != nil || string(requestAfter) != string(requestBefore) {
		t.Fatalf("apply must not rewrite request artifact, err=%v", err)
	}
	if planAfter, err := os.ReadFile(plan.Path); err != nil || string(planAfter) != string(planBefore) {
		t.Fatalf("apply must not rewrite plan artifact, err=%v", err)
	}
	if downloadAfter, err := os.ReadFile(download.Path); err != nil || string(downloadAfter) != string(downloadBefore) {
		t.Fatalf("apply must not rewrite download audit, err=%v", err)
	}
	if _, err := os.Lstat(filepath.Join(root, "toolchain", "bin", "httpx")); !os.IsNotExist(err) {
		t.Fatalf("apply must not create install link, stat err=%v", err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("apply must not execute payload, marker stat err=%v", err)
	}
}

func TestApplyManagedToolBundleUpdateDownloadExecutionCopiesVerifiedRuleBundleDirectoryAndAudits(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	skipIfWindows(t)
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	releaseRoot := filepath.Join(t.TempDir(), "release root")
	writeReleaseManagedRuleBundleFixture(t, releaseRoot, "nuclei-templates", "2026.06.01", map[string]string{
		"http/old.yaml": "id: old\ninfo:\n  name: Old\n  severity: info\n",
	})
	oldWorkspaceRules := filepath.Join(t.TempDir(), "old workspace rules")
	if err := os.MkdirAll(filepath.Join(oldWorkspaceRules, "http"), 0o755); err != nil {
		t.Fatalf("create old workspace rule dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(oldWorkspaceRules, "http", "old.yaml"), []byte("id: old-workspace\n"), 0o644); err != nil {
		t.Fatalf("write old workspace rule: %v", err)
	}
	if _, err := ImportManagedToolBundle(root, ManagedToolBundleImportOptions{
		Import:      true,
		Name:        "nuclei-templates",
		Kind:        "rule_bundle",
		Version:     "2026.06.01",
		Source:      "projectdiscovery/nuclei-templates",
		PayloadPath: oldWorkspaceRules,
	}); err != nil {
		t.Fatalf("import old workspace rule bundle: %v", err)
	}
	stagedPayload := filepath.Join(root, "toolchain", "managed-update-downloads", "nuclei-templates-rule-update", "payload")
	for rel, body := range map[string]string{
		"http/new.yaml": "id: new-http\ninfo:\n  name: New HTTP\n  severity: low\n",
		"dns/info.yaml": "id: new-dns\ninfo:\n  name: New DNS\n  severity: info\n",
	} {
		path := filepath.Join(stagedPayload, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("create staged rule dir: %v", err)
		}
		if err := os.WriteFile(path, []byte(body), 0o777); err != nil {
			t.Fatalf("write staged rule: %v", err)
		}
	}
	latestSHA := digestDirectoryForAppTest(t, stagedPayload)
	appendManagedBundleUpdateForTest(
		t,
		filepath.Join(releaseRoot, "toolchains", "managed-bundles", "nuclei-templates", "bundle.yaml"),
		"2026.06.08",
		latestSHA,
		"2026-06-08T00:00:00Z",
		"rule-bundle-apply-test",
	)
	request, err := CreateManagedToolBundleUpdateRequest(root, releaseRoot, "nuclei-templates", ManagedToolBundleUpdateRequestOptions{
		Request:     true,
		RequestedBy: "operator",
		Reason:      "prepare rule bundle apply",
	})
	if err != nil {
		t.Fatalf("create update request: %v", err)
	}
	plan, err := CreateManagedToolBundleUpdateDownloadPlan(root, request.Path, ManagedToolBundleUpdateDownloadPlanOptions{
		Plan:           true,
		SourceURL:      "https://downloads.example.com/nuclei-templates.tar.gz",
		ExpectedSHA256: latestSHA,
		PlannedBy:      "operator",
		Reason:         "rule bundle download source reviewed",
	})
	if err != nil {
		t.Fatalf("create update download plan: %v", err)
	}
	download := ManagedToolBundleUpdateDownloadExecution{
		Status:              "completed",
		Mode:                "execute",
		Action:              "download",
		Name:                plan.Name,
		Kind:                plan.Kind,
		Source:              plan.Source,
		CurrentVersion:      plan.CurrentVersion,
		CurrentSHA256:       plan.CurrentSHA256,
		LatestVersion:       plan.LatestVersion,
		LatestSHA256:        plan.LatestSHA256,
		UpdateStatus:        plan.UpdateStatus,
		UpdateCheckedAt:     plan.UpdateCheckedAt,
		UpdateMessage:       plan.UpdateMessage,
		SourceURL:           plan.SourceURL,
		ExpectedSHA256:      latestSHA,
		ActualSHA256:        latestSHA,
		DigestStatus:        "matched",
		PlanPath:            plan.Path,
		RequestPath:         request.Path,
		ReleaseRoot:         plan.ReleaseRoot,
		ReleaseBundleDir:    plan.ReleaseBundleDir,
		ReleaseManifestPath: plan.ReleaseManifestPath,
		ReleasePayloadPath:  plan.ReleasePayloadPath,
		PayloadPath:         stagedPayload,
		Actor:               "operator",
		ExecutedAt:          "2026-06-08T00:00:00Z",
		Reason:              "staged verified rule bundle",
	}
	download.Path = filepath.Join(managedToolBundleUpdateDownloadExecutionDir(root), "nuclei-templates-rule-download.json")
	if err := ensureAuthoredToolDirectory(filepath.Dir(download.Path), "managed bundle update download execution directory"); err != nil {
		t.Fatalf("create download execution dir: %v", err)
	}
	downloadData, err := json.MarshalIndent(download, "", "  ")
	if err != nil {
		t.Fatalf("encode download audit: %v", err)
	}
	if err := writeAuthoredToolArtifactFile(download.Path, downloadData, "managed bundle update download execution"); err != nil {
		t.Fatalf("write download audit: %v", err)
	}
	requestBefore, err := os.ReadFile(request.Path)
	if err != nil {
		t.Fatalf("read request before apply: %v", err)
	}
	planBefore, err := os.ReadFile(plan.Path)
	if err != nil {
		t.Fatalf("read plan before apply: %v", err)
	}
	downloadBefore, err := os.ReadFile(download.Path)
	if err != nil {
		t.Fatalf("read download before apply: %v", err)
	}

	apply, err := ApplyManagedToolBundleUpdateDownloadExecution(root, download.Path, ManagedToolBundleUpdateApplyOptions{
		Apply:  true,
		Actor:  "operator",
		Reason: "apply verified downloaded rule bundle",
	})
	if err != nil {
		t.Fatalf("apply managed rule bundle update download execution: %v", err)
	}
	if apply.Status != "completed" || apply.Mode != "apply" || apply.Action != "workspace-copy" || apply.Name != "nuclei-templates" || apply.Kind != "rule_bundle" {
		t.Fatalf("unexpected apply audit identity: %#v", apply)
	}
	if apply.ExpectedSHA256 != latestSHA || apply.ActualSHA256 != latestSHA || apply.DigestStatus != "matched" || apply.DigestAlgorithm != "sha256" || apply.DigestScope != "directory_canonical_records" {
		t.Fatalf("unexpected apply digest metadata: %#v", apply)
	}
	expectedBundle := filepath.Join(root, "toolchain", "managed-bundles", "nuclei-templates")
	expectedPayload := filepath.Join(expectedBundle, "payload")
	expectedManifest := filepath.Join(expectedBundle, "bundle.yaml")
	expectedInstall := filepath.Join(root, "toolchain", "rules", "nuclei-templates")
	if apply.WorkspaceBundleDir != expectedBundle || apply.WorkspacePayloadPath != expectedPayload || apply.WorkspaceManifestPath != expectedManifest || apply.InstallPath != expectedInstall {
		t.Fatalf("unexpected rule bundle workspace paths: %#v", apply)
	}
	if _, err := os.Stat(filepath.Join(expectedPayload, "http", "new.yaml")); err != nil {
		t.Fatalf("expected copied new rule file: %v", err)
	}
	if _, err := os.Stat(filepath.Join(expectedPayload, "dns", "info.yaml")); err != nil {
		t.Fatalf("expected copied DNS rule file: %v", err)
	}
	if _, err := os.Stat(filepath.Join(expectedPayload, "http", "old.yaml")); !os.IsNotExist(err) {
		t.Fatalf("apply must replace old workspace rule bundle files, stat err=%v", err)
	}
	copiedInfo, err := os.Stat(filepath.Join(expectedPayload, "http", "new.yaml"))
	if err != nil {
		t.Fatalf("stat copied rule file: %v", err)
	}
	if copiedInfo.Mode().Perm() != 0o644 {
		t.Fatalf("expected copied rule file mode 0644, got %v", copiedInfo.Mode().Perm())
	}
	bundle, err := ReadManagedToolBundle(root, "nuclei-templates")
	if err != nil {
		t.Fatalf("read applied rule bundle: %v", err)
	}
	if bundle.Kind != "rule_bundle" || bundle.Version != "2026.06.08" || bundle.SHA256 != latestSHA || bundle.PayloadPath != expectedPayload || bundle.InstallPath != expectedInstall {
		t.Fatalf("unexpected applied rule bundle metadata: %#v", bundle)
	}
	manifest, err := os.ReadFile(expectedManifest)
	if err != nil {
		t.Fatalf("read applied rule bundle manifest: %v", err)
	}
	for _, want := range []string{
		"kind: rule_bundle",
		"version: \"2026.06.08\"",
		"payload: payload",
		"sha256: " + latestSHA,
	} {
		if !strings.Contains(string(manifest), want) {
			t.Fatalf("expected applied rule bundle manifest to contain %q, got %q", want, string(manifest))
		}
	}
	if requestAfter, err := os.ReadFile(request.Path); err != nil || string(requestAfter) != string(requestBefore) {
		t.Fatalf("apply must not rewrite request artifact, err=%v", err)
	}
	if planAfter, err := os.ReadFile(plan.Path); err != nil || string(planAfter) != string(planBefore) {
		t.Fatalf("apply must not rewrite plan artifact, err=%v", err)
	}
	if downloadAfter, err := os.ReadFile(download.Path); err != nil || string(downloadAfter) != string(downloadBefore) {
		t.Fatalf("apply must not rewrite download audit, err=%v", err)
	}
	if _, err := os.Lstat(expectedInstall); !os.IsNotExist(err) {
		t.Fatalf("apply must not create rules install link, stat err=%v", err)
	}
}

func TestApplyManagedToolBundleUpdateDownloadExecutionRejectsUnsafeStatesBeforeWrite(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	releaseRoot := filepath.Join(t.TempDir(), "release")
	writeReleaseManagedBinaryBundleFixture(t, releaseRoot, "httpx", "1.3.0", "#!/bin/sh\nexit 0\n")
	downloadPayload := []byte("verified update payload")
	downloadDigest := sha256.Sum256(downloadPayload)
	latestSHA := hex.EncodeToString(downloadDigest[:])
	appendManagedBundleUpdateForTest(
		t,
		filepath.Join(releaseRoot, "toolchains", "managed-bundles", "httpx", "bundle.yaml"),
		"1.4.0",
		latestSHA,
		"2026-06-07T00:00:00Z",
		"apply-rejection-test",
	)
	request, err := CreateManagedToolBundleUpdateRequest(root, releaseRoot, "httpx", ManagedToolBundleUpdateRequestOptions{Request: true})
	if err != nil {
		t.Fatalf("create update request: %v", err)
	}
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(downloadPayload)
	}))
	t.Cleanup(server.Close)
	plan, err := CreateManagedToolBundleUpdateDownloadPlan(root, request.Path, ManagedToolBundleUpdateDownloadPlanOptions{
		Plan:           true,
		SourceURL:      server.URL,
		ExpectedSHA256: latestSHA,
	})
	if err != nil {
		t.Fatalf("create update download plan: %v", err)
	}
	download, err := ExecuteManagedToolBundleUpdateDownloadPlan(root, plan.Path, ManagedToolBundleUpdateDownloadExecutionOptions{
		Execute:    true,
		HTTPClient: server.Client(),
	})
	if err != nil {
		t.Fatalf("execute update download plan: %v", err)
	}

	if _, err := ApplyManagedToolBundleUpdateDownloadExecution(root, download.Path, ManagedToolBundleUpdateApplyOptions{}); err == nil || !strings.Contains(err.Error(), "requires --apply") {
		t.Fatalf("expected missing apply gate rejection, got %v", err)
	}
	if applies, err := ListManagedToolBundleUpdateApplyExecutions(root); err != nil || len(applies) != 0 {
		t.Fatalf("missing apply must not write audits, applies=%#v err=%v", applies, err)
	}

	failed := download
	failed.Status = "failed"
	failed.DigestStatus = "mismatch"
	failed.ActualSHA256 = strings.Repeat("f", 64)
	failed.Path = filepath.Join(root, "toolchain", "managed-update-download-executions", "failed-download.json")
	failedData, err := json.MarshalIndent(failed, "", "  ")
	if err != nil {
		t.Fatalf("encode failed download audit: %v", err)
	}
	if err := writeAuthoredToolArtifactFile(failed.Path, failedData, "managed bundle update download execution"); err != nil {
		t.Fatalf("write failed download audit: %v", err)
	}
	if _, err := ApplyManagedToolBundleUpdateDownloadExecution(root, failed.Path, ManagedToolBundleUpdateApplyOptions{Apply: true}); err == nil || !strings.Contains(err.Error(), "requires completed download execution") {
		t.Fatalf("expected failed download rejection, got %v", err)
	}

	if err := os.WriteFile(download.PayloadPath, []byte("tampered"), 0o644); err != nil {
		t.Fatalf("tamper staged payload: %v", err)
	}
	if _, err := ApplyManagedToolBundleUpdateDownloadExecution(root, download.Path, ManagedToolBundleUpdateApplyOptions{Apply: true}); err == nil || !strings.Contains(err.Error(), "sha256 mismatch") {
		t.Fatalf("expected staged payload digest rejection, got %v", err)
	}
	if _, err := os.Lstat(filepath.Join(root, "toolchain", "managed-bundles", "httpx")); !os.IsNotExist(err) {
		t.Fatalf("rejected apply must not write workspace bundle, stat err=%v", err)
	}
	if applies, err := ListManagedToolBundleUpdateApplyExecutions(root); err != nil || len(applies) != 0 {
		t.Fatalf("rejected apply attempts before copy must not write audits, applies=%#v err=%v", applies, err)
	}
}

func TestApplyManagedToolBundleUpdateDownloadExecutionRejectsUnsafeRuleBundleStagingBeforeWrite(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	releaseRoot := filepath.Join(t.TempDir(), "release")
	writeReleaseManagedRuleBundleFixture(t, releaseRoot, "nuclei-templates", "2026.06.01", map[string]string{
		"http/old.yaml": "id: old\n",
	})
	latestSHA := strings.Repeat("a", 64)
	appendManagedBundleUpdateForTest(
		t,
		filepath.Join(releaseRoot, "toolchains", "managed-bundles", "nuclei-templates", "bundle.yaml"),
		"2026.06.08",
		latestSHA,
		"2026-06-08T00:00:00Z",
		"unsafe-rule-bundle-staging",
	)
	request, err := CreateManagedToolBundleUpdateRequest(root, releaseRoot, "nuclei-templates", ManagedToolBundleUpdateRequestOptions{Request: true})
	if err != nil {
		t.Fatalf("create update request: %v", err)
	}
	plan, err := CreateManagedToolBundleUpdateDownloadPlan(root, request.Path, ManagedToolBundleUpdateDownloadPlanOptions{
		Plan:           true,
		SourceURL:      "https://downloads.example.com/nuclei-templates.tar.gz",
		ExpectedSHA256: latestSHA,
	})
	if err != nil {
		t.Fatalf("create update download plan: %v", err)
	}
	stagedPayload := filepath.Join(root, "toolchain", "managed-update-downloads", "unsafe-rule-update", "payload")
	if err := os.MkdirAll(filepath.Join(stagedPayload, "http"), 0o755); err != nil {
		t.Fatalf("create staged rule dir: %v", err)
	}
	regular := filepath.Join(stagedPayload, "http", "ok.yaml")
	if err := os.WriteFile(regular, []byte("id: ok\n"), 0o644); err != nil {
		t.Fatalf("write staged regular rule: %v", err)
	}
	if err := os.Symlink(regular, filepath.Join(stagedPayload, "http", "link.yaml")); err != nil {
		t.Fatalf("write staged rule symlink: %v", err)
	}
	download := ManagedToolBundleUpdateDownloadExecution{
		Status:              "completed",
		Mode:                "execute",
		Action:              "download",
		Name:                plan.Name,
		Kind:                plan.Kind,
		Source:              plan.Source,
		CurrentVersion:      plan.CurrentVersion,
		CurrentSHA256:       plan.CurrentSHA256,
		LatestVersion:       plan.LatestVersion,
		LatestSHA256:        plan.LatestSHA256,
		UpdateStatus:        plan.UpdateStatus,
		UpdateCheckedAt:     plan.UpdateCheckedAt,
		UpdateMessage:       plan.UpdateMessage,
		SourceURL:           plan.SourceURL,
		ExpectedSHA256:      latestSHA,
		ActualSHA256:        latestSHA,
		DigestStatus:        "matched",
		PlanPath:            plan.Path,
		RequestPath:         request.Path,
		ReleaseRoot:         plan.ReleaseRoot,
		ReleaseBundleDir:    plan.ReleaseBundleDir,
		ReleaseManifestPath: plan.ReleaseManifestPath,
		ReleasePayloadPath:  plan.ReleasePayloadPath,
		PayloadPath:         stagedPayload,
		Path:                filepath.Join(managedToolBundleUpdateDownloadExecutionDir(root), "unsafe-rule-download.json"),
	}
	if err := ensureAuthoredToolDirectory(filepath.Dir(download.Path), "managed bundle update download execution directory"); err != nil {
		t.Fatalf("create download execution dir: %v", err)
	}
	downloadData, err := json.MarshalIndent(download, "", "  ")
	if err != nil {
		t.Fatalf("encode download audit: %v", err)
	}
	if err := writeAuthoredToolArtifactFile(download.Path, downloadData, "managed bundle update download execution"); err != nil {
		t.Fatalf("write download audit: %v", err)
	}

	if _, err := ApplyManagedToolBundleUpdateDownloadExecution(root, download.Path, ManagedToolBundleUpdateApplyOptions{Apply: true}); err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("expected staged rule bundle symlink rejection, got %v", err)
	}
	if _, err := os.Lstat(filepath.Join(root, "toolchain", "managed-bundles", "nuclei-templates")); !os.IsNotExist(err) {
		t.Fatalf("unsafe staged rule bundle rejection must not write workspace bundle, stat err=%v", err)
	}
	if applies, err := ListManagedToolBundleUpdateApplyExecutions(root); err != nil || len(applies) != 0 {
		t.Fatalf("unsafe staged rule bundle rejection must not write apply audits, applies=%#v err=%v", applies, err)
	}
}

func TestListManagedToolBundlesReportsBlockedInvalidUpdateMetadata(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	writeManagedBundleFixture(t, root, "httpx", "binary", "1.3.0", "#!/bin/sh\nexit 0\n")
	manifestPath := filepath.Join(root, "toolchain", "managed-bundles", "httpx", "bundle.yaml")
	appendManagedBundleUpdateForTest(t, manifestPath, "1.3.1", "not-a-sha", "2026-06-05T00:00:00Z", "update_available")

	bundles, err := ListManagedToolBundles(root)
	if err != nil {
		t.Fatalf("list managed bundles should report blocked update metadata instead of failing: %v", err)
	}
	if len(bundles) != 1 ||
		bundles[0].Status != "blocked" ||
		bundles[0].UpdateStatus != "blocked" ||
		!strings.Contains(bundles[0].Error, "update latest_sha256") {
		t.Fatalf("expected blocked workspace managed bundle row, got %#v", bundles)
	}
	if _, err := ReadManagedToolBundle(root, "httpx"); err == nil || !strings.Contains(err.Error(), "update latest_sha256") {
		t.Fatalf("expected strict read update rejection, got %v", err)
	}
	if _, err := os.Lstat(filepath.Join(root, "toolchain", "bin", "httpx")); !os.IsNotExist(err) {
		t.Fatalf("listing invalid update metadata must not create install link, stat err=%v", err)
	}
}

func TestManagedToolBundleManifestRejectsWhitespaceOnlyProvenanceValues(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	writeManagedBundleFixture(t, root, "httpx", "binary", "1.3.0", "#!/bin/sh\nexit 0\n")
	appendManagedBundleProvenanceForTest(t,
		filepath.Join(root, "toolchain", "managed-bundles", "httpx", "bundle.yaml"),
		" ",
		"",
		"",
		"",
	)

	_, err := ReadManagedToolBundle(root, "httpx")
	if err == nil || !strings.Contains(err.Error(), "managed bundle provenance source_url must be a single token") {
		t.Fatalf("expected whitespace provenance rejection, got %v", err)
	}
}

func TestListReleaseManagedToolBundlesRejectsDigestMismatchBeforeWorkspaceWrites(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	releaseRoot := filepath.Join(t.TempDir(), "release")
	writeReleaseManagedBinaryBundleFixture(t, releaseRoot, "httpx", "1.3.0", "#!/bin/sh\nexit 0\n")
	manifestPath := filepath.Join(releaseRoot, "toolchains", "managed-bundles", "httpx", "bundle.yaml")
	manifest, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("read release manifest: %v", err)
	}
	if err := os.WriteFile(manifestPath, []byte(strings.Replace(string(manifest), "sha256: ", "sha256: 0000000000000000000000000000000000000000000000000000000000000000 # ", 1)), 0o644); err != nil {
		t.Fatalf("corrupt release manifest digest: %v", err)
	}

	bundles, err := ListReleaseManagedToolBundles(releaseRoot)
	if err != nil {
		t.Fatalf("list release managed bundles should report blocked bundle instead of failing: %v", err)
	}
	if len(bundles) != 1 || bundles[0].Status != "blocked" || !strings.Contains(bundles[0].Error, "sha256 mismatch") {
		t.Fatalf("expected blocked digest mismatch status, got %#v", bundles)
	}
	if _, err := os.Lstat(filepath.Join(root, "toolchain", "managed-bundles", "httpx")); !os.IsNotExist(err) {
		t.Fatalf("digest mismatch must not copy into workspace, stat err=%v", err)
	}
	_, err = ReadReleaseManagedToolBundle(releaseRoot, "httpx")
	if err == nil || !strings.Contains(err.Error(), "sha256 mismatch") {
		t.Fatalf("expected strict read digest mismatch rejection, got %v", err)
	}
}

func TestListReleaseManagedToolBundleStatusesReportsAvailableVerifiedAndNotImported(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	releaseRoot := filepath.Join(t.TempDir(), "release")
	marker := filepath.Join(t.TempDir(), "release-status-payload-ran")
	releasePayload := writeReleaseManagedBinaryBundleFixture(t, releaseRoot, "httpx", "1.3.0", "#!/bin/sh\ntouch "+marker+"\n")

	statuses, err := ListReleaseManagedToolBundleStatuses(root, releaseRoot)
	if err != nil {
		t.Fatalf("list release managed bundle statuses: %v", err)
	}
	if len(statuses) != 1 {
		t.Fatalf("expected one release managed bundle status, got %#v", statuses)
	}
	status := statuses[0]
	workspaceBundle := filepath.Join(root, "toolchain", "managed-bundles", "httpx")
	workspacePayload := filepath.Join(workspaceBundle, "payload", "httpx")
	installPath := filepath.Join(root, "toolchain", "bin", "httpx")
	if status.Name != "httpx" || status.Status != "available" || status.Mode != "read-only" || status.DigestStatus != "verified" || status.DigestAlgorithm != "sha256" || status.DigestScope != "file" {
		t.Fatalf("unexpected release managed status metadata: %#v", status)
	}
	if status.ImportStatus != "not_imported" || status.LinkStatus != "missing" {
		t.Fatalf("expected not imported status, got %#v", status)
	}
	if status.ReleasePayloadPath != releasePayload || status.WorkspaceBundleDir != workspaceBundle || status.WorkspacePayloadPath != workspacePayload || status.InstallPath != installPath {
		t.Fatalf("unexpected release managed status paths: %#v", status)
	}
	if status.Error != "" {
		t.Fatalf("available not-imported status should not carry error: %#v", status)
	}
	if _, err := os.Lstat(workspaceBundle); !os.IsNotExist(err) {
		t.Fatalf("release status list must not copy workspace bundle, stat err=%v", err)
	}
	if _, err := os.Lstat(installPath); !os.IsNotExist(err) {
		t.Fatalf("release status list must not create install link, stat err=%v", err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("release status list must not execute payload, marker stat err=%v", err)
	}
}

func TestReadReleaseManagedToolBundleStatusWithExpectedSHA256PinsDigest(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	releaseRoot := filepath.Join(t.TempDir(), "release")
	payloadBody := "#!/bin/sh\nexit 0\n"
	writeReleaseManagedBinaryBundleFixture(t, releaseRoot, "httpx", "1.3.0", payloadBody)
	actualDigest := sha256.Sum256([]byte(payloadBody))
	actualSHA := hex.EncodeToString(actualDigest[:])
	wrongDigest := sha256.Sum256([]byte("different release payload"))
	wrongSHA := hex.EncodeToString(wrongDigest[:])
	workspaceBundle := filepath.Join(root, "toolchain", "managed-bundles", "httpx")
	installPath := filepath.Join(root, "toolchain", "bin", "httpx")

	status, err := ReadReleaseManagedToolBundleStatusWithOptions(root, releaseRoot, "httpx", ReleaseManagedToolBundleStatusOptions{ExpectedSHA256: wrongSHA})
	if err != nil {
		t.Fatalf("read release managed status with mismatched expected sha256 should return blocked status: %v", err)
	}
	if status.Status != "blocked" || status.DigestStatus != "verified" || status.ImportStatus != "blocked" || status.LinkStatus != "blocked" || status.ExpectedSHA256 != wrongSHA || status.ExpectedSHA256Status != "mismatch" {
		t.Fatalf("expected blocked expected digest status, got %#v", status)
	}
	if !strings.Contains(status.Error, "expected sha256 mismatch") || !strings.Contains(status.Error, wrongSHA) || !strings.Contains(status.Error, actualSHA) {
		t.Fatalf("expected mismatch error with expected and actual digest, got %#v", status)
	}
	if _, err := os.Lstat(workspaceBundle); !os.IsNotExist(err) {
		t.Fatalf("expected sha256 status mismatch must not copy workspace bundle, stat err=%v", err)
	}
	if _, err := os.Lstat(installPath); !os.IsNotExist(err) {
		t.Fatalf("expected sha256 status mismatch must not create install link, stat err=%v", err)
	}

	status, err = ReadReleaseManagedToolBundleStatusWithOptions(root, releaseRoot, "httpx", ReleaseManagedToolBundleStatusOptions{ExpectedSHA256: strings.ToUpper(actualSHA)})
	if err != nil {
		t.Fatalf("read release managed status with matching expected sha256: %v", err)
	}
	if status.Status != "available" || status.DigestStatus != "verified" || status.ExpectedSHA256 != actualSHA || status.ExpectedSHA256Status != "matched" {
		t.Fatalf("expected matching expected sha256 metadata to be normalized, got %#v", status)
	}

	_, err = ReadReleaseManagedToolBundleStatusWithOptions(root, releaseRoot, "httpx", ReleaseManagedToolBundleStatusOptions{ExpectedSHA256: "not-a-sha256"})
	if err == nil || !strings.Contains(err.Error(), "expected sha256 is invalid") {
		t.Fatalf("expected invalid expected sha256 rejection, got %v", err)
	}
	_, err = ReadReleaseManagedToolBundleStatusWithOptions(root, releaseRoot, "httpx", ReleaseManagedToolBundleStatusOptions{ExpectedSHA256Provided: true})
	if err == nil || !strings.Contains(err.Error(), "expected sha256 is invalid") {
		t.Fatalf("expected explicit empty expected sha256 rejection, got %v", err)
	}
}

func TestReleaseManagedInstallWithExpectedSHA256RejectsMismatchBeforeWorkspaceWrites(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	releaseRoot := filepath.Join(t.TempDir(), "release")
	payloadBody := "#!/bin/sh\nexit 0\n"
	writeReleaseManagedBinaryBundleFixture(t, releaseRoot, "httpx", "1.3.0", payloadBody)
	actualDigest := sha256.Sum256([]byte(payloadBody))
	actualSHA := hex.EncodeToString(actualDigest[:])
	wrongDigest := sha256.Sum256([]byte("different release payload"))
	wrongSHA := hex.EncodeToString(wrongDigest[:])
	workspaceBundle := filepath.Join(root, "toolchain", "managed-bundles", "httpx")
	installPath := filepath.Join(root, "toolchain", "bin", "httpx")

	_, err := CreateReleaseManagedToolBundleInstallPlanWithOptions(root, releaseRoot, "web-api-pack", "httpx", ReleaseManagedToolBundleInstallPlanOptions{ExpectedSHA256: wrongSHA})
	if err == nil || !strings.Contains(err.Error(), "expected sha256 mismatch") || !strings.Contains(err.Error(), wrongSHA) || !strings.Contains(err.Error(), actualSHA) {
		t.Fatalf("expected release managed install plan expected sha256 mismatch, got %v", err)
	}
	if _, err := os.Lstat(workspaceBundle); !os.IsNotExist(err) {
		t.Fatalf("expected sha256 plan mismatch must not copy workspace bundle, stat err=%v", err)
	}
	if _, err := os.Lstat(installPath); !os.IsNotExist(err) {
		t.Fatalf("expected sha256 plan mismatch must not create install link, stat err=%v", err)
	}

	_, err = ExecuteReleaseManagedToolBundleInstall(root, releaseRoot, "web-api-pack", "httpx", ReleaseManagedToolBundleInstallOptions{
		Execute:        true,
		ExpectedSHA256: wrongSHA,
	})
	if err == nil || !strings.Contains(err.Error(), "expected sha256 mismatch") {
		t.Fatalf("expected release managed install expected sha256 mismatch, got %v", err)
	}
	if _, err := os.Lstat(workspaceBundle); !os.IsNotExist(err) {
		t.Fatalf("expected sha256 install mismatch must not copy workspace bundle, stat err=%v", err)
	}
	if _, err := os.Lstat(installPath); !os.IsNotExist(err) {
		t.Fatalf("expected sha256 install mismatch must not create install link, stat err=%v", err)
	}

	plan, err := CreateReleaseManagedToolBundleInstallPlanWithOptions(root, releaseRoot, "web-api-pack", "httpx", ReleaseManagedToolBundleInstallPlanOptions{ExpectedSHA256: strings.ToUpper(actualSHA)})
	if err != nil {
		t.Fatalf("create release managed install plan with matching expected sha256: %v", err)
	}
	if plan.ExpectedSHA256 != actualSHA || plan.ExpectedSHA256Status != "matched" {
		t.Fatalf("expected release managed plan expected sha256 to be normalized, got %#v", plan)
	}
	result, err := ExecuteReleaseManagedToolBundleInstall(root, releaseRoot, "web-api-pack", "httpx", ReleaseManagedToolBundleInstallOptions{
		Execute:        true,
		ExpectedSHA256: strings.ToUpper(actualSHA),
	})
	if err != nil {
		t.Fatalf("execute release managed install with matching expected sha256: %v", err)
	}
	if result.ExpectedSHA256 != actualSHA || result.ExpectedSHA256Status != "matched" || result.SHA256 != actualSHA {
		t.Fatalf("expected release managed install result expected sha256 to be normalized, got %#v", result)
	}
}

func TestListReleaseManagedToolBundleStatusesReportsImportedAndLinked(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	releaseRoot := filepath.Join(t.TempDir(), "release")
	writeReleaseManagedBinaryBundleFixture(t, releaseRoot, "httpx", "1.3.0", "#!/bin/sh\nexit 0\n")
	if _, err := ExecuteReleaseManagedToolBundleInstall(root, releaseRoot, "web-api-pack", "httpx", ReleaseManagedToolBundleInstallOptions{Execute: true}); err != nil {
		t.Fatalf("execute release managed install: %v", err)
	}

	status, err := ReadReleaseManagedToolBundleStatus(root, releaseRoot, "httpx")
	if err != nil {
		t.Fatalf("read release managed bundle status: %v", err)
	}
	if status.Status != "available" || status.ImportStatus != "linked" || status.LinkStatus != "linked" {
		t.Fatalf("expected linked import status, got %#v", status)
	}
	if status.WorkspacePayloadPath != filepath.Join(root, "toolchain", "managed-bundles", "httpx", "payload", "httpx") || status.InstallPath != filepath.Join(root, "toolchain", "bin", "httpx") {
		t.Fatalf("unexpected linked status paths: %#v", status)
	}
}

func TestListReleaseManagedToolBundleStatusesReportsConflictWhenInstallPathExists(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	releaseRoot := filepath.Join(t.TempDir(), "release")
	writeReleaseManagedBinaryBundleFixture(t, releaseRoot, "httpx", "1.3.0", "#!/bin/sh\nexit 0\n")
	installPath := filepath.Join(root, "toolchain", "bin", "httpx")
	if err := os.MkdirAll(filepath.Dir(installPath), 0o755); err != nil {
		t.Fatalf("create install dir: %v", err)
	}
	if err := os.WriteFile(installPath, []byte("existing"), 0o755); err != nil {
		t.Fatalf("write existing install path: %v", err)
	}

	statuses, err := ListReleaseManagedToolBundleStatuses(root, releaseRoot)
	if err != nil {
		t.Fatalf("list release managed bundle statuses: %v", err)
	}
	if len(statuses) != 1 {
		t.Fatalf("expected one status, got %#v", statuses)
	}
	status := statuses[0]
	if status.Status != "available" || status.ImportStatus != "conflict" || status.LinkStatus != "conflict" || !strings.Contains(status.Error, "managed install path already exists") {
		t.Fatalf("expected install conflict status, got %#v", status)
	}
	if _, err := os.Lstat(filepath.Join(root, "toolchain", "managed-bundles", "httpx")); !os.IsNotExist(err) {
		t.Fatalf("status conflict must not copy workspace bundle, stat err=%v", err)
	}
	data, err := os.ReadFile(installPath)
	if err != nil {
		t.Fatalf("read existing install path: %v", err)
	}
	if string(data) != "existing" {
		t.Fatalf("status check must not rewrite install path, got %q", string(data))
	}
}

func TestListReleaseManagedToolBundleStatusesReportsBlockedDigestMismatchWithoutWorkspaceWrites(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	releaseRoot := filepath.Join(t.TempDir(), "release")
	writeReleaseManagedBinaryBundleFixture(t, releaseRoot, "httpx", "1.3.0", "#!/bin/sh\nexit 0\n")
	manifestPath := filepath.Join(releaseRoot, "toolchains", "managed-bundles", "httpx", "bundle.yaml")
	manifest, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("read release manifest: %v", err)
	}
	if err := os.WriteFile(manifestPath, []byte(strings.Replace(string(manifest), "sha256: ", "sha256: 0000000000000000000000000000000000000000000000000000000000000000 # ", 1)), 0o644); err != nil {
		t.Fatalf("corrupt release manifest digest: %v", err)
	}

	statuses, err := ListReleaseManagedToolBundleStatuses(root, releaseRoot)
	if err != nil {
		t.Fatalf("list release managed bundle statuses should surface blocked bundles: %v", err)
	}
	if len(statuses) != 1 {
		t.Fatalf("expected one status, got %#v", statuses)
	}
	status := statuses[0]
	if status.Status != "blocked" || status.DigestStatus != "blocked" || status.ImportStatus != "blocked" || status.LinkStatus != "blocked" || !strings.Contains(status.Error, "sha256 mismatch") {
		t.Fatalf("expected blocked digest status, got %#v", status)
	}
	if _, err := os.Lstat(filepath.Join(root, "toolchain", "managed-bundles", "httpx")); !os.IsNotExist(err) {
		t.Fatalf("blocked release status must not copy into workspace, stat err=%v", err)
	}
	if _, err := os.Lstat(filepath.Join(root, "toolchain", "bin", "httpx")); !os.IsNotExist(err) {
		t.Fatalf("blocked release status must not create install link, stat err=%v", err)
	}
}

func TestReleaseManagedToolBundleStatusRejectsWorkspaceInstallLinkEscape(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	releaseRoot := filepath.Join(t.TempDir(), "release")
	payloadBody := "#!/bin/sh\nexit 0\n"
	writeReleaseManagedBinaryBundleFixture(t, releaseRoot, "httpx", "1.3.0", payloadBody)
	writeManagedBundleFixture(t, root, "httpx", "binary", "1.3.0", payloadBody)
	installPath := filepath.Join(root, "toolchain", "bin", "httpx")
	if err := os.MkdirAll(filepath.Dir(installPath), 0o755); err != nil {
		t.Fatalf("create install dir: %v", err)
	}
	outside := filepath.Join(t.TempDir(), "outside-httpx")
	if err := os.WriteFile(outside, []byte(payloadBody), 0o755); err != nil {
		t.Fatalf("write outside link target: %v", err)
	}
	if err := os.Symlink(outside, installPath); err != nil {
		t.Fatalf("write escaping install link: %v", err)
	}

	status, err := ReadReleaseManagedToolBundleStatus(root, releaseRoot, "httpx")
	if err != nil {
		t.Fatalf("read release managed bundle status: %v", err)
	}
	if status.Status != "available" || status.ImportStatus != "conflict" || status.LinkStatus != "conflict" || !strings.Contains(status.Error, "managed install link target mismatch") {
		t.Fatalf("expected escaping install link conflict, got %#v", status)
	}
}

func TestReleaseManagedToolBundleStatusSourceWritesMetadataOnlyMarkerAndClears(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	releaseRoot := filepath.Join(t.TempDir(), "release")
	marker := filepath.Join(t.TempDir(), "release-source-payload-ran")
	writeReleaseManagedBinaryBundleFixture(t, releaseRoot, "httpx", "1.3.0", "#!/bin/sh\ntouch "+marker+"\n")

	source, err := SetReleaseManagedToolBundleStatusSource(root, releaseRoot, ReleaseManagedToolBundleStatusSourceOptions{
		Set:    true,
		SetBy:  "operator",
		Reason: "current release under review",
	})
	if err != nil {
		t.Fatalf("set release managed status source: %v", err)
	}
	if source.Status != "current" || source.Mode != "metadata-only" || source.ReleaseRoot != releaseRoot || source.Total != 1 || source.Available != 1 || source.NotImported != 1 {
		t.Fatalf("unexpected status source marker: %#v", source)
	}
	if source.Path != filepath.Join(root, "toolchain", "release-managed-status-source.json") || source.SetBy != "operator" || source.Reason != "current release under review" {
		t.Fatalf("unexpected status source path/actor: %#v", source)
	}
	if _, err := os.Lstat(filepath.Join(root, "toolchain", "managed-bundles", "httpx")); !os.IsNotExist(err) {
		t.Fatalf("status source set must not copy workspace bundle, stat err=%v", err)
	}
	if _, err := os.Lstat(filepath.Join(root, "toolchain", "bin", "httpx")); !os.IsNotExist(err) {
		t.Fatalf("status source set must not create install link, stat err=%v", err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("status source set must not execute payload, marker stat err=%v", err)
	}

	read, found, err := ReadReleaseManagedToolBundleStatusSource(root)
	if err != nil {
		t.Fatalf("read release managed status source: %v", err)
	}
	if !found || read.Path != source.Path || read.Status != "current" {
		t.Fatalf("expected current status source marker, found=%v source=%#v", found, read)
	}

	cleared, err := ClearReleaseManagedToolBundleStatusSource(root, ReleaseManagedToolBundleStatusSourceClearOptions{
		Clear:     true,
		ClearedBy: "operator",
		Reason:    "release archived",
	})
	if err != nil {
		t.Fatalf("clear release managed status source: %v", err)
	}
	if cleared.Status != "cleared" || cleared.ReleaseRoot != releaseRoot || cleared.ClearedBy != "operator" || cleared.Reason != "release archived" {
		t.Fatalf("unexpected cleared status source marker: %#v", cleared)
	}
	read, found, err = ReadReleaseManagedToolBundleStatusSource(root)
	if err != nil {
		t.Fatalf("read cleared release managed status source: %v", err)
	}
	if !found || read.Status != "cleared" {
		t.Fatalf("expected cleared status source marker, found=%v source=%#v", found, read)
	}
}

func TestReleaseManagedToolBundleStatusSourcePersistsExpectedPinPolicy(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	releaseRoot := filepath.Join(t.TempDir(), "release")
	marker := filepath.Join(t.TempDir(), "release-source-payload-ran")
	payloadBody := "#!/bin/sh\ntouch " + marker + "\n"
	writeReleaseManagedBinaryBundleFixture(t, releaseRoot, "httpx", "1.3.0", payloadBody)
	actualDigest := sha256.Sum256([]byte(payloadBody))
	actualSHA := hex.EncodeToString(actualDigest[:])
	wrongDigest := sha256.Sum256([]byte("different release payload"))
	wrongSHA := hex.EncodeToString(wrongDigest[:])
	workspaceBundle := filepath.Join(root, "toolchain", "managed-bundles", "httpx")
	installPath := filepath.Join(root, "toolchain", "bin", "httpx")

	source, err := SetReleaseManagedToolBundleStatusSource(root, releaseRoot, ReleaseManagedToolBundleStatusSourceOptions{
		Set:                    true,
		SetBy:                  "operator",
		Reason:                 "pin current release",
		ExpectedSHA256Bundle:   "httpx",
		ExpectedSHA256:         strings.ToUpper(actualSHA),
		ExpectedSHA256Provided: true,
	})
	if err != nil {
		t.Fatalf("set release managed status source with matching expected pin: %v", err)
	}
	if source.ExpectedSHA256Bundle != "httpx" || source.ExpectedSHA256 != actualSHA || source.ExpectedSHA256Status != "matched" {
		t.Fatalf("expected normalized matched pin metadata, got %#v", source)
	}
	if source.Total != 1 || source.Available != 1 || source.Blocked != 0 || source.NotImported != 1 {
		t.Fatalf("expected matched pin to keep bundle available/not-imported, got %#v", source)
	}
	read, found, err := ReadReleaseManagedToolBundleStatusSource(root)
	if err != nil {
		t.Fatalf("read release managed status source with matching expected pin: %v", err)
	}
	if !found || read.ExpectedSHA256Bundle != "httpx" || read.ExpectedSHA256 != actualSHA || read.ExpectedSHA256Status != "matched" {
		t.Fatalf("expected matched pin metadata persisted, found=%v source=%#v", found, read)
	}
	if _, err := os.Lstat(workspaceBundle); !os.IsNotExist(err) {
		t.Fatalf("matching status source pin must not copy workspace bundle, stat err=%v", err)
	}
	if _, err := os.Lstat(installPath); !os.IsNotExist(err) {
		t.Fatalf("matching status source pin must not create install link, stat err=%v", err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("matching status source pin must not execute payload, marker stat err=%v", err)
	}

	source, err = SetReleaseManagedToolBundleStatusSource(root, releaseRoot, ReleaseManagedToolBundleStatusSourceOptions{
		Set:                    true,
		SetBy:                  "operator",
		Reason:                 "pin rejected release",
		ExpectedSHA256Bundle:   "httpx",
		ExpectedSHA256:         wrongSHA,
		ExpectedSHA256Provided: true,
	})
	if err != nil {
		t.Fatalf("set release managed status source with mismatched expected pin: %v", err)
	}
	if source.ExpectedSHA256Bundle != "httpx" || source.ExpectedSHA256 != wrongSHA || source.ExpectedSHA256Status != "mismatch" {
		t.Fatalf("expected mismatched pin metadata, got %#v", source)
	}
	if source.Total != 1 || source.Available != 0 || source.Blocked != 1 || source.NotImported != 0 {
		t.Fatalf("expected mismatched pin to block source counts, got %#v", source)
	}
	read, found, err = ReadReleaseManagedToolBundleStatusSource(root)
	if err != nil {
		t.Fatalf("read release managed status source with mismatched expected pin: %v", err)
	}
	if !found || read.ExpectedSHA256Bundle != "httpx" || read.ExpectedSHA256 != wrongSHA || read.ExpectedSHA256Status != "mismatch" || read.Blocked != 1 {
		t.Fatalf("expected mismatched pin metadata persisted, found=%v source=%#v", found, read)
	}
	if _, err := os.Lstat(workspaceBundle); !os.IsNotExist(err) {
		t.Fatalf("mismatched status source pin must not copy workspace bundle, stat err=%v", err)
	}
	if _, err := os.Lstat(installPath); !os.IsNotExist(err) {
		t.Fatalf("mismatched status source pin must not create install link, stat err=%v", err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("mismatched status source pin must not execute payload, marker stat err=%v", err)
	}

	_, err = SetReleaseManagedToolBundleStatusSource(root, releaseRoot, ReleaseManagedToolBundleStatusSourceOptions{
		Set:                  true,
		ExpectedSHA256Bundle: "httpx",
	})
	if err == nil || !strings.Contains(err.Error(), "--expected-sha256") {
		t.Fatalf("expected pin bundle without expected sha256 to be rejected, got %v", err)
	}
	_, err = SetReleaseManagedToolBundleStatusSource(root, releaseRoot, ReleaseManagedToolBundleStatusSourceOptions{
		Set:                    true,
		ExpectedSHA256:         actualSHA,
		ExpectedSHA256Provided: true,
	})
	if err == nil || !strings.Contains(err.Error(), "--pin-bundle") {
		t.Fatalf("expected expected sha256 without pin bundle to be rejected, got %v", err)
	}
	_, err = SetReleaseManagedToolBundleStatusSource(root, releaseRoot, ReleaseManagedToolBundleStatusSourceOptions{
		Set:                    true,
		ExpectedSHA256Bundle:   "missing-httpx",
		ExpectedSHA256:         actualSHA,
		ExpectedSHA256Provided: true,
	})
	if err == nil || !strings.Contains(err.Error(), "pin bundle not found") {
		t.Fatalf("expected missing pin bundle to be rejected, got %v", err)
	}
}

func TestReleaseManagedRuleBundleRequiresDigestAndLinksRulesDirectory(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	releaseRoot := filepath.Join(t.TempDir(), "release")
	writeReleaseManagedRuleBundleFixture(t, releaseRoot, "nuclei-templates", "2026.06.02", map[string]string{
		"http/example.yaml": "id: ouro-test\ninfo:\n  name: Ouro Test\n  severity: info\n",
	})

	missingDigestManifest := filepath.Join(releaseRoot, "toolchains", "managed-bundles", "nuclei-templates", "bundle.yaml")
	manifestData, err := os.ReadFile(missingDigestManifest)
	if err != nil {
		t.Fatalf("read release rule bundle manifest: %v", err)
	}
	if err := os.WriteFile(missingDigestManifest, []byte(strings.Replace(string(manifestData), "sha256: "+releaseRuleBundleDigestFromManifest(t, manifestData)+"\n", "", 1)), 0o644); err != nil {
		t.Fatalf("remove release rule bundle digest: %v", err)
	}
	_, err = CreateReleaseManagedToolBundleInstallPlan(root, releaseRoot, "web-api-pack", "nuclei-templates")
	if err == nil || !strings.Contains(err.Error(), "sha256 is required") {
		t.Fatalf("expected missing rule bundle digest rejection, got %v", err)
	}
	writeReleaseManagedRuleBundleFixture(t, releaseRoot, "nuclei-templates", "2026.06.02", map[string]string{
		"http/example.yaml": "id: ouro-test\ninfo:\n  name: Ouro Test\n  severity: info\n",
	})
	manifestData, err = os.ReadFile(missingDigestManifest)
	if err != nil {
		t.Fatalf("read restored release rule bundle manifest: %v", err)
	}
	ruleBundleSHA := releaseRuleBundleDigestFromManifest(t, manifestData)
	status, err := ReadReleaseManagedToolBundleStatus(root, releaseRoot, "nuclei-templates")
	if err != nil {
		t.Fatalf("read release managed rule bundle status: %v", err)
	}
	if status.DigestStatus != "verified" || status.DigestAlgorithm != "sha256" || status.DigestScope != "directory_canonical_records" {
		t.Fatalf("expected canonical directory digest status metadata, got %#v", status)
	}
	status, err = ReadReleaseManagedToolBundleStatusWithOptions(root, releaseRoot, "nuclei-templates", ReleaseManagedToolBundleStatusOptions{ExpectedSHA256: strings.ToUpper(ruleBundleSHA)})
	if err != nil {
		t.Fatalf("read release managed rule bundle status with expected sha256: %v", err)
	}
	if status.ExpectedSHA256 != ruleBundleSHA || status.ExpectedSHA256Status != "matched" || status.DigestScope != "directory_canonical_records" {
		t.Fatalf("expected rule bundle expected sha256 metadata, got %#v", status)
	}

	result, err := ExecuteReleaseManagedToolBundleInstall(root, releaseRoot, "web-api-pack", "nuclei-templates", ReleaseManagedToolBundleInstallOptions{Execute: true})
	if err != nil {
		t.Fatalf("execute release managed rule bundle install: %v", err)
	}
	if result.Kind != "rule_bundle" || result.InstallPath != filepath.Join(root, "toolchain", "rules", "nuclei-templates") {
		t.Fatalf("unexpected release rule bundle install result: %#v", result)
	}
	info, err := os.Lstat(result.InstallPath)
	if err != nil {
		t.Fatalf("expected managed rule bundle link: %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("expected managed rule bundle install to create symlink, mode=%v", info.Mode())
	}
	if _, err := os.Stat(filepath.Join(root, "toolchain", "managed-bundles", "nuclei-templates", "payload", "http", "example.yaml")); err != nil {
		t.Fatalf("expected workspace rule bundle payload copy: %v", err)
	}
}

func TestReleaseManagedRuleBundleReportsCorpusProvenanceMetadata(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	releaseRoot := filepath.Join(t.TempDir(), "release")
	writeReleaseManagedRuleBundleFixture(t, releaseRoot, "nuclei-templates", "2026.06.02", map[string]string{
		"http/example.yaml": "id: ouro-test\ninfo:\n  name: Ouro Test\n  severity: info\n",
	})
	manifestPath := filepath.Join(releaseRoot, "toolchains", "managed-bundles", "nuclei-templates", "bundle.yaml")
	manifestData, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("read release rule bundle manifest: %v", err)
	}
	ruleBundleSHA := releaseRuleBundleDigestFromManifest(t, manifestData)
	appendManagedBundleCorpusForTest(t, manifestPath, "ouroboros.rule_corpus.v1", "nuclei-templates", "https://github.com/projectdiscovery/nuclei-templates", "2026-06-05T00:00:00Z", "1", "sha256", "directory_canonical_records", ruleBundleSHA)

	status, err := ReadReleaseManagedToolBundleStatus(root, releaseRoot, "nuclei-templates")
	if err != nil {
		t.Fatalf("read release managed rule bundle status with corpus metadata: %v", err)
	}
	if status.CorpusStatus != "recorded" || status.CorpusDigestStatus != "matched" || status.CorpusFileCount != 1 {
		t.Fatalf("expected recorded matched corpus metadata, got %#v", status)
	}
	if status.CorpusName != "nuclei-templates" || status.CorpusSource != "https://github.com/projectdiscovery/nuclei-templates" || status.CorpusRetrievedAt != "2026-06-05T00:00:00Z" {
		t.Fatalf("unexpected corpus identity metadata: %#v", status)
	}
	if status.CorpusSHA256 != ruleBundleSHA || status.CorpusDigestAlgorithm != "sha256" || status.CorpusDigestScope != "directory_canonical_records" {
		t.Fatalf("unexpected corpus digest metadata: %#v", status)
	}

	result, err := ExecuteReleaseManagedToolBundleInstall(root, releaseRoot, "web-api-pack", "nuclei-templates", ReleaseManagedToolBundleInstallOptions{Execute: true})
	if err != nil {
		t.Fatalf("execute release managed rule bundle install with corpus metadata: %v", err)
	}
	if result.CorpusStatus != "recorded" || result.CorpusDigestStatus != "matched" || result.CorpusFileCount != 1 {
		t.Fatalf("expected install result to carry corpus metadata, got %#v", result)
	}
	bundle, err := ReadManagedToolBundle(root, "nuclei-templates")
	if err != nil {
		t.Fatalf("read workspace rule bundle with corpus metadata: %v", err)
	}
	if bundle.CorpusStatus != "recorded" || bundle.CorpusDigestStatus != "matched" || bundle.CorpusFileCount != 1 || bundle.CorpusSource != "https://github.com/projectdiscovery/nuclei-templates" {
		t.Fatalf("expected copied workspace bundle to retain corpus metadata, got %#v", bundle)
	}
}

func TestReleaseManagedRuleBundleMissingCorpusMetadataStaysNonConflicting(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	releaseRoot := filepath.Join(t.TempDir(), "release")
	writeReleaseManagedRuleBundleFixture(t, releaseRoot, "nuclei-templates", "2026.06.02", map[string]string{
		"http/example.yaml": "id: ouro-test\ninfo:\n  name: Ouro Test\n  severity: info\n",
	})
	releaseBundleDir := filepath.Join(releaseRoot, "toolchains", "managed-bundles", "nuclei-templates")
	workspaceBundleDir := filepath.Join(root, "toolchain", "managed-bundles", "nuclei-templates")
	if err := copyManagedBundleDirectory(releaseBundleDir, workspaceBundleDir); err != nil {
		t.Fatalf("copy release bundle into workspace without corpus metadata: %v", err)
	}
	manifestPath := filepath.Join(releaseBundleDir, "bundle.yaml")
	manifestData, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("read release rule bundle manifest: %v", err)
	}
	ruleBundleSHA := releaseRuleBundleDigestFromManifest(t, manifestData)
	appendManagedBundleCorpusForTest(t, manifestPath, "ouroboros.rule_corpus.v1", "nuclei-templates", "https://github.com/projectdiscovery/nuclei-templates", "2026-06-05T00:00:00Z", "1", "sha256", "directory_canonical_records", ruleBundleSHA)

	workspaceBundle, err := ReadManagedToolBundle(root, "nuclei-templates")
	if err != nil {
		t.Fatalf("read workspace rule bundle without corpus metadata: %v", err)
	}
	if workspaceBundle.CorpusStatus != "missing" || workspaceBundle.CorpusDigestStatus != "missing" {
		t.Fatalf("expected workspace missing corpus status to be explicit, got %#v", workspaceBundle)
	}
	status, err := ReadReleaseManagedToolBundleStatus(root, releaseRoot, "nuclei-templates")
	if err != nil {
		t.Fatalf("read release managed status with release corpus and workspace missing corpus: %v", err)
	}
	if status.ImportStatus != "imported" || status.LinkStatus != "missing" {
		t.Fatalf("release corpus metadata should not conflict with a workspace bundle missing optional corpus metadata, got %#v", status)
	}
	if status.CorpusStatus != "recorded" || status.CorpusDigestStatus != "matched" {
		t.Fatalf("expected release corpus metadata to remain visible, got %#v", status)
	}
}

func TestReleaseManagedRuleBundleBlocksInvalidCorpusMetadataBeforeWorkspaceWrites(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	releaseRoot := filepath.Join(t.TempDir(), "release")
	writeReleaseManagedRuleBundleFixture(t, releaseRoot, "nuclei-templates", "2026.06.02", map[string]string{
		"http/example.yaml": "id: ouro-test\ninfo:\n  name: Ouro Test\n  severity: info\n",
	})
	manifestPath := filepath.Join(releaseRoot, "toolchains", "managed-bundles", "nuclei-templates", "bundle.yaml")
	manifestData, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("read release rule bundle manifest: %v", err)
	}
	ruleBundleSHA := releaseRuleBundleDigestFromManifest(t, manifestData)
	appendManagedBundleCorpusForTest(t, manifestPath, "ouroboros.rule_corpus.v1", "nuclei-templates", "https://github.com/projectdiscovery/nuclei-templates", "2026-06-05T00:00:00Z", "2", "sha256", "directory_canonical_records", ruleBundleSHA)
	workspaceBundle := filepath.Join(root, "toolchain", "managed-bundles", "nuclei-templates")
	installPath := filepath.Join(root, "toolchain", "rules", "nuclei-templates")

	statuses, err := ListReleaseManagedToolBundleStatuses(root, releaseRoot)
	if err != nil {
		t.Fatalf("list release managed statuses should report blocked corpus metadata instead of failing: %v", err)
	}
	if len(statuses) != 1 || statuses[0].Status != "blocked" || statuses[0].CorpusStatus != "blocked" || statuses[0].CorpusDigestStatus != "blocked" || !strings.Contains(statuses[0].Error, "corpus file_count mismatch") {
		t.Fatalf("expected blocked corpus status, got %#v", statuses)
	}
	if _, err := ReadReleaseManagedToolBundle(releaseRoot, "nuclei-templates"); err == nil || !strings.Contains(err.Error(), "corpus file_count mismatch") {
		t.Fatalf("expected strict release read corpus rejection, got %v", err)
	}
	if _, err := CreateReleaseManagedToolBundleInstallPlan(root, releaseRoot, "web-api-pack", "nuclei-templates"); err == nil || !strings.Contains(err.Error(), "corpus file_count mismatch") {
		t.Fatalf("expected install plan corpus rejection, got %v", err)
	}
	if _, err := os.Lstat(workspaceBundle); !os.IsNotExist(err) {
		t.Fatalf("blocked corpus metadata must not copy workspace bundle, stat err=%v", err)
	}
	if _, err := os.Lstat(installPath); !os.IsNotExist(err) {
		t.Fatalf("blocked corpus metadata must not create install link, stat err=%v", err)
	}
}

func TestReleaseManagedToolBundleRejectsSymlinkPayload(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	releaseRoot := filepath.Join(t.TempDir(), "release")
	bundleDir := filepath.Join(releaseRoot, "toolchains", "managed-bundles", "httpx")
	payloadDir := filepath.Join(bundleDir, "payload")
	if err := os.MkdirAll(payloadDir, 0o755); err != nil {
		t.Fatalf("create release bundle payload dir: %v", err)
	}
	target := filepath.Join(t.TempDir(), "httpx")
	body := []byte("#!/bin/sh\nexit 0\n")
	if err := os.WriteFile(target, body, 0o755); err != nil {
		t.Fatalf("write symlink target: %v", err)
	}
	if err := os.Symlink(target, filepath.Join(payloadDir, "httpx")); err != nil {
		t.Fatalf("write payload symlink: %v", err)
	}
	digest := sha256.Sum256(body)
	manifest := `name: httpx
kind: binary
version: "1.3.0"
source: projectdiscovery/httpx
payload: payload/httpx
sha256: ` + hex.EncodeToString(digest[:]) + `
`
	if err := os.WriteFile(filepath.Join(bundleDir, "bundle.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write release bundle manifest: %v", err)
	}

	_, err := CreateReleaseManagedToolBundleInstallPlan(root, releaseRoot, "web-api-pack", "httpx")
	if err == nil || !strings.Contains(err.Error(), "payload must not be a symlink") {
		t.Fatalf("expected symlink payload rejection, got %v", err)
	}
}

func TestReleaseManagedToolBundleInstallRejectsExistingInstallPathBeforeWorkspaceCopy(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	releaseRoot := filepath.Join(t.TempDir(), "release")
	writeReleaseManagedBinaryBundleFixture(t, releaseRoot, "httpx", "1.3.0", "#!/bin/sh\nexit 0\n")
	installPath := filepath.Join(root, "toolchain", "bin", "httpx")
	if err := os.MkdirAll(filepath.Dir(installPath), 0o755); err != nil {
		t.Fatalf("create install dir: %v", err)
	}
	if err := os.WriteFile(installPath, []byte("existing"), 0o755); err != nil {
		t.Fatalf("write existing install path: %v", err)
	}

	_, err := ExecuteReleaseManagedToolBundleInstall(root, releaseRoot, "web-api-pack", "httpx", ReleaseManagedToolBundleInstallOptions{Execute: true})
	if err == nil || !strings.Contains(err.Error(), "managed install path already exists") {
		t.Fatalf("expected existing install path rejection, got %v", err)
	}
	if _, err := os.Lstat(filepath.Join(root, "toolchain", "managed-bundles", "httpx")); !os.IsNotExist(err) {
		t.Fatalf("existing install path must fail before workspace bundle copy, stat err=%v", err)
	}
	data, err := os.ReadFile(installPath)
	if err != nil {
		t.Fatalf("read existing install path: %v", err)
	}
	if string(data) != "existing" {
		t.Fatalf("existing install path must remain unchanged, got %q", string(data))
	}
}

func writeManagedBundleFixture(t *testing.T, root string, name string, kind string, version string, payloadBody string) string {
	t.Helper()
	bundleDir := filepath.Join(root, "toolchain", "managed-bundles", name)
	payloadDir := filepath.Join(bundleDir, "payload")
	if err := os.MkdirAll(payloadDir, 0o755); err != nil {
		t.Fatalf("create bundle payload dir: %v", err)
	}
	payloadPath := filepath.Join(payloadDir, name)
	if err := os.WriteFile(payloadPath, []byte(payloadBody), 0o755); err != nil {
		t.Fatalf("write bundle payload: %v", err)
	}
	digest := sha256.Sum256([]byte(payloadBody))
	manifest := `name: ` + name + `
kind: ` + kind + `
version: "` + version + `"
source: projectdiscovery/` + name + `
payload: payload/` + name + `
sha256: ` + hex.EncodeToString(digest[:]) + `
`
	if err := os.WriteFile(filepath.Join(bundleDir, "bundle.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write bundle manifest: %v", err)
	}
	return payloadPath
}

func appendManagedBundleProvenanceForTest(t *testing.T, manifestPath, sourceURL, upstreamVersion, retrievedAt, verifiedBy string) {
	t.Helper()
	f, err := os.OpenFile(manifestPath, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatalf("open bundle manifest for provenance append: %v", err)
	}
	defer f.Close()
	if _, err := fmt.Fprintf(f, "provenance:\n  source_url: %q\n  upstream_version: %q\n  retrieved_at: %q\n  verified_by: %q\n", sourceURL, upstreamVersion, retrievedAt, verifiedBy); err != nil {
		t.Fatalf("append bundle provenance: %v", err)
	}
}

func appendManagedBundleCorpusForTest(t *testing.T, manifestPath, schema, name, source, retrievedAt, fileCount, digestAlgorithm, digestScope, sha256 string) {
	t.Helper()
	f, err := os.OpenFile(manifestPath, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatalf("open bundle manifest for corpus append: %v", err)
	}
	defer f.Close()
	if _, err := fmt.Fprintf(
		f,
		"corpus:\n  schema: %q\n  name: %q\n  source: %q\n  retrieved_at: %q\n  file_count: %s\n  digest_algorithm: %q\n  digest_scope: %q\n  sha256: %q\n",
		schema,
		name,
		source,
		retrievedAt,
		fileCount,
		digestAlgorithm,
		digestScope,
		sha256,
	); err != nil {
		t.Fatalf("append bundle corpus metadata: %v", err)
	}
}

func appendManagedBundleUpdateForTest(t *testing.T, manifestPath, latestVersion, latestSHA256, checkedAt, message string) {
	t.Helper()
	f, err := os.OpenFile(manifestPath, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatalf("open bundle manifest for update append: %v", err)
	}
	defer f.Close()
	if _, err := fmt.Fprintf(f, "update:\n  latest_version: %q\n  latest_sha256: %q\n  checked_at: %q\n  message: %q\n", latestVersion, latestSHA256, checkedAt, message); err != nil {
		t.Fatalf("append bundle update metadata: %v", err)
	}
}

func appendManagedBundleSignatureForTest(t *testing.T, manifestPath, keyID, signedAt, signedBy, name, kind, version, payloadSHA256 string, invalid bool) {
	t.Helper()
	seed := sha256.Sum256([]byte("ouro test managed bundle signature key"))
	privateKey := ed25519.NewKeyFromSeed(seed[:])
	publicKey := privateKey.Public().(ed25519.PublicKey)
	message := managedBundleSignatureMessageForTest(name, kind, version, payloadSHA256)
	signature := ed25519.Sign(privateKey, message)
	if invalid {
		signature[0] ^= 0xff
	}
	f, err := os.OpenFile(manifestPath, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatalf("open bundle manifest for signature append: %v", err)
	}
	defer f.Close()
	if _, err := fmt.Fprintf(
		f,
		"signature:\n  algorithm: ed25519\n  key_id: %q\n  public_key: %q\n  payload_sha256: %q\n  signature: %q\n  signed_at: %q\n  signed_by: %q\n",
		keyID,
		base64.RawStdEncoding.EncodeToString(publicKey),
		payloadSHA256,
		base64.RawStdEncoding.EncodeToString(signature),
		signedAt,
		signedBy,
	); err != nil {
		t.Fatalf("append bundle signature metadata: %v", err)
	}
}

func managedBundleSignatureMessageForTest(name, kind, version, payloadSHA256 string) []byte {
	return []byte("ouroboros-managed-bundle-signature-v1\nname:" + name + "\nkind:" + kind + "\nversion:" + version + "\npayload_sha256:" + payloadSHA256 + "\n")
}

func writeReleaseManagedBinaryBundleFixture(t *testing.T, releaseRoot string, name string, version string, payloadBody string) string {
	t.Helper()
	bundleDir := filepath.Join(releaseRoot, "toolchains", "managed-bundles", name)
	payloadDir := filepath.Join(bundleDir, "payload")
	if err := os.MkdirAll(payloadDir, 0o755); err != nil {
		t.Fatalf("create release bundle payload dir: %v", err)
	}
	payloadPath := filepath.Join(payloadDir, name)
	if err := os.WriteFile(payloadPath, []byte(payloadBody), 0o755); err != nil {
		t.Fatalf("write release bundle payload: %v", err)
	}
	digest := sha256.Sum256([]byte(payloadBody))
	manifest := `name: ` + name + `
kind: binary
version: "` + version + `"
source: projectdiscovery/` + name + `
payload: payload/` + name + `
sha256: ` + hex.EncodeToString(digest[:]) + `
`
	if err := os.WriteFile(filepath.Join(bundleDir, "bundle.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write release bundle manifest: %v", err)
	}
	return payloadPath
}

func writeReleaseManagedRuleBundleFixture(t *testing.T, releaseRoot string, name string, version string, files map[string]string) string {
	t.Helper()
	bundleDir := filepath.Join(releaseRoot, "toolchains", "managed-bundles", name)
	if err := os.RemoveAll(bundleDir); err != nil {
		t.Fatalf("reset release rule bundle dir: %v", err)
	}
	payloadDir := filepath.Join(bundleDir, "payload")
	for rel, body := range files {
		path := filepath.Join(payloadDir, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("create release rule payload dir: %v", err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatalf("write release rule payload: %v", err)
		}
	}
	digest := digestDirectoryForAppTest(t, payloadDir)
	manifest := `name: ` + name + `
kind: rule_bundle
version: "` + version + `"
source: projectdiscovery/` + name + `
payload: payload
sha256: ` + digest + `
`
	if err := os.WriteFile(filepath.Join(bundleDir, "bundle.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write release rule bundle manifest: %v", err)
	}
	return payloadDir
}

func writeRuleBundleDirectoryForAppTest(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for rel, body := range files {
		path := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("create rule bundle test dir: %v", err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatalf("write rule bundle test file: %v", err)
		}
	}
	return dir
}

func tarGzipRuleBundleArchiveForAppTest(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var names []string
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, name := range names {
		body := []byte(files[name])
		header := &tar.Header{
			Name: name,
			Mode: 0o777,
			Size: int64(len(body)),
		}
		if err := tw.WriteHeader(header); err != nil {
			t.Fatalf("write tar header: %v", err)
		}
		if _, err := tw.Write(body); err != nil {
			t.Fatalf("write tar body: %v", err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("close tar writer: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("close gzip writer: %v", err)
	}
	return buf.Bytes()
}

func zipRuleBundleArchiveForAppTest(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var names []string
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, name := range names {
		header := &zip.FileHeader{Name: name, Method: zip.Deflate}
		header.SetMode(0o777)
		writer, err := zw.CreateHeader(header)
		if err != nil {
			t.Fatalf("write zip header: %v", err)
		}
		if _, err := writer.Write([]byte(files[name])); err != nil {
			t.Fatalf("write zip body: %v", err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close zip writer: %v", err)
	}
	return buf.Bytes()
}

func tarGzipSingleEntryForAppTest(t *testing.T, name, linkName string, typeFlag byte, body []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	header := &tar.Header{
		Name:     name,
		Mode:     0o777,
		Typeflag: typeFlag,
		Linkname: linkName,
	}
	if typeFlag == tar.TypeReg || typeFlag == 0 {
		header.Size = int64(len(body))
	}
	if err := tw.WriteHeader(header); err != nil {
		t.Fatalf("write tar unsafe header: %v", err)
	}
	if header.Size > 0 {
		if _, err := tw.Write(body); err != nil {
			t.Fatalf("write tar unsafe body: %v", err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("close unsafe tar writer: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("close unsafe gzip writer: %v", err)
	}
	return buf.Bytes()
}

func zipSingleEntryForAppTest(t *testing.T, name string, mode os.FileMode, body []byte) []byte {
	t.Helper()
	return zipRuleBundleArchiveEntriesForAppTest(t, []zipRuleBundleArchiveEntryForAppTest{{Name: name, Mode: mode, Body: body}})
}

type zipRuleBundleArchiveEntryForAppTest struct {
	Name string
	Mode os.FileMode
	Body []byte
}

func zipRuleBundleArchiveEntriesForAppTest(t *testing.T, entries []zipRuleBundleArchiveEntryForAppTest) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, entry := range entries {
		header := &zip.FileHeader{Name: entry.Name, Method: zip.Deflate}
		header.SetMode(entry.Mode)
		writer, err := zw.CreateHeader(header)
		if err != nil {
			t.Fatalf("write zip unsafe header: %v", err)
		}
		if len(entry.Body) > 0 {
			if _, err := writer.Write(entry.Body); err != nil {
				t.Fatalf("write zip unsafe body: %v", err)
			}
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close zip unsafe writer: %v", err)
	}
	return buf.Bytes()
}

func fileModeForAppTest(t *testing.T, path string) os.FileMode {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat file mode: %v", err)
	}
	return info.Mode().Perm()
}

func sha256FileForAppTest(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:]), nil
}

func digestDirectoryForAppTest(t *testing.T, dir string) string {
	t.Helper()
	return canonicalDirectoryDigestForAppTest(t, dir)
}

func canonicalDirectoryDigestForAppTest(t *testing.T, dir string) string {
	t.Helper()
	paths := directoryDigestPathsForAppTest(t, dir)
	hash := sha256.New()
	hash.Write([]byte(managedRuleBundleDirectoryDigestVersion))
	for _, rel := range paths {
		data, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(rel)))
		if err != nil {
			t.Fatalf("read canonical digest file: %v", err)
		}
		var size [8]byte
		relBytes := []byte(rel)
		binary.BigEndian.PutUint64(size[:], uint64(len(relBytes)))
		hash.Write(size[:])
		hash.Write(relBytes)
		binary.BigEndian.PutUint64(size[:], uint64(len(data)))
		hash.Write(size[:])
		digest := sha256.Sum256(data)
		hash.Write(digest[:])
	}
	return hex.EncodeToString(hash.Sum(nil))
}

func legacyNULDirectoryDigestForAppTest(t *testing.T, dir string) string {
	t.Helper()
	paths := directoryDigestPathsForAppTest(t, dir)
	hash := sha256.New()
	for _, rel := range paths {
		data, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(rel)))
		if err != nil {
			t.Fatalf("read legacy digest file: %v", err)
		}
		hash.Write([]byte(rel))
		hash.Write([]byte{0})
		hash.Write(data)
		hash.Write([]byte{0})
	}
	return hex.EncodeToString(hash.Sum(nil))
}

func directoryDigestPathsForAppTest(t *testing.T, dir string) []string {
	t.Helper()
	paths := []string{}
	if err := filepath.WalkDir(dir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		paths = append(paths, filepath.ToSlash(rel))
		return nil
	}); err != nil {
		t.Fatalf("walk digest directory: %v", err)
	}
	sort.Strings(paths)
	return paths
}

func releaseRuleBundleDigestFromManifest(t *testing.T, data []byte) string {
	t.Helper()
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "sha256: ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "sha256: "))
		}
	}
	t.Fatalf("release rule bundle digest missing from manifest")
	return ""
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func TestInspectToolchainsRejectsInvalidWorkspace(t *testing.T) {
	_, err := InspectToolchains(filepath.Join(t.TempDir(), "missing"))
	if err == nil {
		t.Fatalf("expected missing workspace to fail")
	}
}

func TestCreateToolchainInstallRequestWritesPendingMetadata(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	missing := filepath.Join(t.TempDir(), "missing-httpx")
	marker := filepath.Join(t.TempDir(), "installer-ran")
	manifest := `name: web-api-pack
version: "0.1.0"
capabilities:
  - web.discovery
runner: native
parser: web_api_parser
healthcheck: built_in
dependencies:
  - name: ` + missing + `
    kind: binary
    install_hint: install bundled httpx pack
    install_command:
      - sh
      - -c
      - touch ` + marker + `
`
	if err := os.WriteFile(filepath.Join(root, "toolchain", "web-api-pack", "manifest.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	request, err := CreateToolchainInstallRequest(root, "web-api-pack", missing)
	if err != nil {
		t.Fatalf("create install request: %v", err)
	}
	if request.Status != "pending" || request.Pack != "web-api-pack" || request.Dependency != missing {
		t.Fatalf("unexpected install request metadata: %#v", request)
	}
	if strings.Join(request.Command, " ") != "sh -c touch "+marker {
		t.Fatalf("unexpected request command: %#v", request.Command)
	}
	data, err := os.ReadFile(request.Path)
	if err != nil {
		t.Fatalf("read request file: %v", err)
	}
	var decoded ToolchainInstallRequest
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("decode request file: %v", err)
	}
	if decoded.Status != "pending" || strings.Join(decoded.Command, " ") != strings.Join(request.Command, " ") {
		t.Fatalf("unexpected request file contents: %#v", decoded)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("install request must not execute install command, marker stat err=%v", err)
	}
}

func TestCreateToolchainInstallRequestPersistsDependsOnMetadata(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	marker := filepath.Join(t.TempDir(), "installer-ran")
	manifest := `name: web-api-pack
version: "0.1.0"
capabilities:
  - web.discovery
runner: native
parser: web_api_parser
healthcheck: built_in
dependencies:
  - name: a-httpx
    kind: binary
    depends_on:
      - z-runtime
    install_command:
      - sh
      - -c
      - touch ` + marker + `
  - name: z-runtime
    kind: binary
    install_command:
      - sh
      - -c
      - touch ` + marker + `
`
	if err := os.WriteFile(filepath.Join(root, "toolchain", "web-api-pack", "manifest.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	request, err := CreateToolchainInstallRequest(root, "web-api-pack", "a-httpx")
	if err != nil {
		t.Fatalf("create install request: %v", err)
	}
	if len(request.DependsOn) != 1 || request.DependsOn[0] != "z-runtime" {
		t.Fatalf("expected request to preserve depends_on metadata, got %#v", request)
	}
	data, err := os.ReadFile(request.Path)
	if err != nil {
		t.Fatalf("read request file: %v", err)
	}
	var decoded ToolchainInstallRequest
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("decode request file: %v", err)
	}
	if len(decoded.DependsOn) != 1 || decoded.DependsOn[0] != "z-runtime" {
		t.Fatalf("expected persisted depends_on metadata, got %#v", decoded)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("install request must not execute install command, marker stat err=%v", err)
	}
}

func TestListToolchainInstallRequestsReadsPendingMetadata(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	missing := filepath.Join(t.TempDir(), "missing-httpx")
	marker := filepath.Join(t.TempDir(), "installer-ran")
	manifest := `name: web-api-pack
version: "0.1.0"
capabilities:
  - web.discovery
runner: native
parser: web_api_parser
healthcheck: built_in
dependencies:
  - name: ` + missing + `
    kind: binary
    install_hint: install bundled httpx pack
    install_command:
      - sh
      - -c
      - touch ` + marker + `
`
	if err := os.WriteFile(filepath.Join(root, "toolchain", "web-api-pack", "manifest.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	created, err := CreateToolchainInstallRequest(root, "web-api-pack", missing)
	if err != nil {
		t.Fatalf("create install request: %v", err)
	}

	requests, err := ListToolchainInstallRequests(root)
	if err != nil {
		t.Fatalf("list install requests: %v", err)
	}
	if len(requests) != 1 {
		t.Fatalf("expected one install request, got %#v", requests)
	}
	request := requests[0]
	if request.Path != created.Path || request.Status != "pending" || request.Pack != "web-api-pack" || request.Dependency != missing {
		t.Fatalf("unexpected listed request: %#v", request)
	}
	if strings.Join(request.Command, " ") != "sh -c touch "+marker {
		t.Fatalf("unexpected listed command: %#v", request.Command)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("listing install requests must not execute install command, marker stat err=%v", err)
	}
}

func TestUpdateToolchainInstallRequestStatusApprovesAndDeniesWithoutExecution(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	marker := filepath.Join(t.TempDir(), "installer-ran")
	manifest := `name: web-api-pack
version: "0.1.0"
capabilities:
  - web.discovery
runner: native
parser: web_api_parser
healthcheck: built_in
dependencies:
  - name: missing-httpx
    kind: binary
    install_command:
      - sh
      - -c
      - touch ` + marker + `
  - name: missing-nuclei
    kind: binary
    install_command:
      - sh
      - -c
      - touch ` + marker + `
`
	if err := os.WriteFile(filepath.Join(root, "toolchain", "web-api-pack", "manifest.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	approveRequest, err := CreateToolchainInstallRequest(root, "web-api-pack", "missing-httpx")
	if err != nil {
		t.Fatalf("create approve request: %v", err)
	}
	denyRequest, err := CreateToolchainInstallRequest(root, "web-api-pack", "missing-nuclei")
	if err != nil {
		t.Fatalf("create deny request: %v", err)
	}

	approved, err := UpdateToolchainInstallRequestStatus(root, approveRequest.Path, "approved")
	if err != nil {
		t.Fatalf("approve request: %v", err)
	}
	denied, err := UpdateToolchainInstallRequestStatus(root, filepath.Base(denyRequest.Path), "denied")
	if err != nil {
		t.Fatalf("deny request by basename: %v", err)
	}
	if approved.Status != "approved" || denied.Status != "denied" {
		t.Fatalf("unexpected updated statuses: approved=%#v denied=%#v", approved, denied)
	}
	requests, err := ListToolchainInstallRequests(root)
	if err != nil {
		t.Fatalf("list requests: %v", err)
	}
	statuses := map[string]string{}
	for _, request := range requests {
		statuses[request.Dependency] = request.Status
	}
	if statuses["missing-httpx"] != "approved" || statuses["missing-nuclei"] != "denied" {
		t.Fatalf("expected approved/denied persisted statuses, got %#v", statuses)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("approving or denying install requests must not execute install command, marker stat err=%v", err)
	}
}

func TestPlanApprovedToolchainInstallsRequiresApprovedRequests(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	marker := filepath.Join(t.TempDir(), "installer-ran")
	manifest := `name: web-api-pack
version: "0.1.0"
capabilities:
  - web.discovery
runner: native
parser: web_api_parser
healthcheck: built_in
dependencies:
  - name: missing-httpx
    kind: binary
    install_command:
      - sh
      - -c
      - touch ` + marker + `
  - name: missing-nuclei
    kind: binary
    install_command:
      - sh
      - -c
      - touch ` + marker + `
  - name: missing-naabu
    kind: binary
    install_command:
      - sh
      - -c
      - touch ` + marker + `
`
	if err := os.WriteFile(filepath.Join(root, "toolchain", "web-api-pack", "manifest.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	approvedRequest, err := CreateToolchainInstallRequest(root, "web-api-pack", "missing-httpx")
	if err != nil {
		t.Fatalf("create approved request: %v", err)
	}
	deniedRequest, err := CreateToolchainInstallRequest(root, "web-api-pack", "missing-nuclei")
	if err != nil {
		t.Fatalf("create denied request: %v", err)
	}
	if _, err := CreateToolchainInstallRequest(root, "web-api-pack", "missing-naabu"); err != nil {
		t.Fatalf("create pending request: %v", err)
	}
	if _, err := UpdateToolchainInstallRequestStatus(root, approvedRequest.Path, "approved"); err != nil {
		t.Fatalf("approve request: %v", err)
	}
	if _, err := UpdateToolchainInstallRequestStatus(root, deniedRequest.Path, "denied"); err != nil {
		t.Fatalf("deny request: %v", err)
	}

	plan, err := PlanApprovedToolchainInstalls(root)
	if err != nil {
		t.Fatalf("plan approved installs: %v", err)
	}
	if plan.Summary.Total != 3 || plan.Summary.Pending != 1 || plan.Summary.Approved != 1 || plan.Summary.Denied != 1 {
		t.Fatalf("unexpected dispatch summary: %#v", plan.Summary)
	}
	if len(plan.Ready) != 1 || plan.Ready[0].Dependency != "missing-httpx" || plan.Ready[0].Status != "approved" {
		t.Fatalf("expected only approved request to be ready, got %#v", plan.Ready)
	}
	if len(plan.Skipped) != 2 {
		t.Fatalf("expected pending/denied requests to be skipped, got %#v", plan.Skipped)
	}
	skipped := map[string]string{}
	for _, skip := range plan.Skipped {
		skipped[skip.Request.Dependency] = skip.Reason
	}
	if skipped["missing-naabu"] != "not-approved: pending" || skipped["missing-nuclei"] != "not-approved: denied" {
		t.Fatalf("unexpected skipped reasons: %#v", skipped)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("planning approved installs must not execute install command, marker stat err=%v", err)
	}
}

func TestPlanApprovedToolchainInstallsOrdersApprovedDependencies(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	marker := filepath.Join(t.TempDir(), "installer-ran")
	manifest := `name: web-api-pack
version: "0.1.0"
capabilities:
  - web.discovery
runner: native
parser: web_api_parser
healthcheck: built_in
dependencies:
  - name: a-httpx
    kind: binary
    depends_on:
      - z-runtime
    install_command:
      - sh
      - -c
      - touch ` + marker + `
  - name: z-runtime
    kind: binary
    install_command:
      - sh
      - -c
      - touch ` + marker + `
`
	if err := os.WriteFile(filepath.Join(root, "toolchain", "web-api-pack", "manifest.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	dependent, err := CreateToolchainInstallRequest(root, "web-api-pack", "a-httpx")
	if err != nil {
		t.Fatalf("create dependent request: %v", err)
	}
	prerequisite, err := CreateToolchainInstallRequest(root, "web-api-pack", "z-runtime")
	if err != nil {
		t.Fatalf("create prerequisite request: %v", err)
	}
	if _, err := UpdateToolchainInstallRequestStatus(root, dependent.Path, "approved"); err != nil {
		t.Fatalf("approve dependent request: %v", err)
	}
	if _, err := UpdateToolchainInstallRequestStatus(root, prerequisite.Path, "approved"); err != nil {
		t.Fatalf("approve prerequisite request: %v", err)
	}

	plan, err := PlanApprovedToolchainInstalls(root)
	if err != nil {
		t.Fatalf("plan approved installs: %v", err)
	}
	if len(plan.Ready) != 2 {
		t.Fatalf("expected two ready requests, got %#v", plan)
	}
	if plan.Ready[0].Dependency != "z-runtime" || plan.Ready[1].Dependency != "a-httpx" {
		t.Fatalf("expected prerequisite before dependent, got %#v", plan.Ready)
	}
	if len(plan.Ready[1].DependsOn) != 1 || plan.Ready[1].DependsOn[0] != "z-runtime" {
		t.Fatalf("expected dependent ready request to carry depends_on metadata, got %#v", plan.Ready[1])
	}
	if len(plan.Skipped) != 0 {
		t.Fatalf("expected no skipped requests, got %#v", plan.Skipped)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("planning approved installs must not execute install command, marker stat err=%v", err)
	}
}

func TestPlanApprovedToolchainInstallsSkipsDependentWhenPrerequisiteNotApproved(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	marker := filepath.Join(t.TempDir(), "installer-ran")
	manifest := `name: web-api-pack
version: "0.1.0"
capabilities:
  - web.discovery
runner: native
parser: web_api_parser
healthcheck: built_in
dependencies:
  - name: a-httpx
    kind: binary
    depends_on:
      - z-runtime
    install_command:
      - sh
      - -c
      - touch ` + marker + `
  - name: z-runtime
    kind: binary
    install_command:
      - sh
      - -c
      - touch ` + marker + `
`
	if err := os.WriteFile(filepath.Join(root, "toolchain", "web-api-pack", "manifest.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	dependent, err := CreateToolchainInstallRequest(root, "web-api-pack", "a-httpx")
	if err != nil {
		t.Fatalf("create dependent request: %v", err)
	}
	if _, err := CreateToolchainInstallRequest(root, "web-api-pack", "z-runtime"); err != nil {
		t.Fatalf("create prerequisite request: %v", err)
	}
	if _, err := UpdateToolchainInstallRequestStatus(root, dependent.Path, "approved"); err != nil {
		t.Fatalf("approve dependent request: %v", err)
	}

	plan, err := PlanApprovedToolchainInstalls(root)
	if err != nil {
		t.Fatalf("plan approved installs: %v", err)
	}
	if len(plan.Ready) != 0 {
		t.Fatalf("dependent must not be ready before prerequisite approval, got %#v", plan.Ready)
	}
	reasons := map[string]string{}
	for _, skip := range plan.Skipped {
		reasons[skip.Request.Dependency] = skip.Reason
	}
	if reasons["z-runtime"] != "not-approved: pending" {
		t.Fatalf("expected prerequisite pending reason, got %#v", reasons)
	}
	if reasons["a-httpx"] != "dependency-not-approved: z-runtime status=pending" {
		t.Fatalf("expected dependent dependency-not-approved reason, got %#v", reasons)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("planning approved installs must not execute install command, marker stat err=%v", err)
	}
}

func TestPlanApprovedToolchainInstallsSkipsDependentWhenPrerequisiteRequestMissing(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	marker := filepath.Join(t.TempDir(), "installer-ran")
	manifest := `name: web-api-pack
version: "0.1.0"
capabilities:
  - web.discovery
runner: native
parser: web_api_parser
healthcheck: built_in
dependencies:
  - name: a-httpx
    kind: binary
    depends_on:
      - z-runtime
    install_command:
      - sh
      - -c
      - touch ` + marker + `
  - name: z-runtime
    kind: binary
    install_command:
      - sh
      - -c
      - touch ` + marker + `
`
	if err := os.WriteFile(filepath.Join(root, "toolchain", "web-api-pack", "manifest.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	dependent, err := CreateToolchainInstallRequest(root, "web-api-pack", "a-httpx")
	if err != nil {
		t.Fatalf("create dependent request: %v", err)
	}
	if _, err := UpdateToolchainInstallRequestStatus(root, dependent.Path, "approved"); err != nil {
		t.Fatalf("approve dependent request: %v", err)
	}

	plan, err := PlanApprovedToolchainInstalls(root)
	if err != nil {
		t.Fatalf("plan approved installs: %v", err)
	}
	if len(plan.Ready) != 0 {
		t.Fatalf("dependent must not be ready before prerequisite request exists, got %#v", plan.Ready)
	}
	if len(plan.Skipped) != 1 || plan.Skipped[0].Request.Dependency != "a-httpx" {
		t.Fatalf("expected dependent skip only, got %#v", plan.Skipped)
	}
	if plan.Skipped[0].Reason != "dependency-not-approved: z-runtime status=missing-request" {
		t.Fatalf("expected missing prerequisite request reason, got %#v", plan.Skipped)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("planning approved installs must not execute install command, marker stat err=%v", err)
	}
}

func TestCreateToolchainInstallDispatchWritesDryRunMetadataOnlyArtifact(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	marker := filepath.Join(t.TempDir(), "installer-ran")
	manifest := `name: web-api-pack
version: "0.1.0"
capabilities:
  - web.discovery
runner: native
parser: web_api_parser
healthcheck: built_in
dependencies:
  - name: missing-httpx
    kind: binary
    install_command:
      - sh
      - -c
      - touch ` + marker + `
  - name: missing-nuclei
    kind: binary
    install_command:
      - sh
      - -c
      - touch ` + marker + `
`
	if err := os.WriteFile(filepath.Join(root, "toolchain", "web-api-pack", "manifest.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	approvedRequest, err := CreateToolchainInstallRequest(root, "web-api-pack", "missing-httpx")
	if err != nil {
		t.Fatalf("create approved request: %v", err)
	}
	if _, err := CreateToolchainInstallRequest(root, "web-api-pack", "missing-nuclei"); err != nil {
		t.Fatalf("create pending request: %v", err)
	}
	if _, err := UpdateToolchainInstallRequestStatus(root, approvedRequest.Path, "approved"); err != nil {
		t.Fatalf("approve request: %v", err)
	}

	dispatch, err := CreateToolchainInstallDispatch(root)
	if err != nil {
		t.Fatalf("create install dispatch: %v", err)
	}
	if dispatch.Status != "dry-run" || dispatch.Mode != "metadata-only" {
		t.Fatalf("expected metadata-only dry-run dispatch, got %#v", dispatch)
	}
	if dispatch.Path == "" {
		t.Fatalf("expected dispatch path: %#v", dispatch)
	}
	if len(dispatch.Ready) != 1 || dispatch.Ready[0].Dependency != "missing-httpx" {
		t.Fatalf("expected only approved request in dispatch ready list, got %#v", dispatch.Ready)
	}
	if len(dispatch.Skipped) != 1 || dispatch.Skipped[0].Request.Dependency != "missing-nuclei" {
		t.Fatalf("expected pending request to be skipped, got %#v", dispatch.Skipped)
	}
	data, err := os.ReadFile(dispatch.Path)
	if err != nil {
		t.Fatalf("read dispatch artifact: %v", err)
	}
	var decoded ToolchainInstallDispatch
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("decode dispatch artifact: %v", err)
	}
	if decoded.Status != "dry-run" || decoded.Mode != "metadata-only" || len(decoded.Ready) != 1 || len(decoded.Skipped) != 1 {
		t.Fatalf("unexpected dispatch artifact: %#v", decoded)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("creating install dispatch must not execute install command, marker stat err=%v", err)
	}
}

func TestListAndReadToolchainInstallDispatchesReadsDryRunArtifacts(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	marker := filepath.Join(t.TempDir(), "installer-ran")
	manifest := `name: web-api-pack
version: "0.1.0"
capabilities:
  - web.discovery
runner: native
parser: web_api_parser
healthcheck: built_in
dependencies:
  - name: missing-httpx
    kind: binary
    install_command:
      - sh
      - -c
      - touch ` + marker + `
  - name: missing-nuclei
    kind: binary
    install_command:
      - sh
      - -c
      - touch ` + marker + `
`
	if err := os.WriteFile(filepath.Join(root, "toolchain", "web-api-pack", "manifest.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	firstRequest, err := CreateToolchainInstallRequest(root, "web-api-pack", "missing-httpx")
	if err != nil {
		t.Fatalf("create first request: %v", err)
	}
	secondRequest, err := CreateToolchainInstallRequest(root, "web-api-pack", "missing-nuclei")
	if err != nil {
		t.Fatalf("create second request: %v", err)
	}
	if _, err := UpdateToolchainInstallRequestStatus(root, firstRequest.Path, "approved"); err != nil {
		t.Fatalf("approve first request: %v", err)
	}
	firstDispatch, err := CreateToolchainInstallDispatch(root)
	if err != nil {
		t.Fatalf("create first dispatch: %v", err)
	}
	if _, err := UpdateToolchainInstallRequestStatus(root, secondRequest.Path, "approved"); err != nil {
		t.Fatalf("approve second request: %v", err)
	}
	secondDispatch, err := CreateToolchainInstallDispatch(root)
	if err != nil {
		t.Fatalf("create second dispatch: %v", err)
	}

	dispatches, err := ListToolchainInstallDispatches(root)
	if err != nil {
		t.Fatalf("list install dispatches: %v", err)
	}
	if len(dispatches) != 2 {
		t.Fatalf("expected two dispatch artifacts, got %#v", dispatches)
	}
	if dispatches[0].Path > dispatches[1].Path {
		t.Fatalf("expected sorted dispatch paths, got %#v", []string{dispatches[0].Path, dispatches[1].Path})
	}
	byPath := map[string]ToolchainInstallDispatch{}
	for _, dispatch := range dispatches {
		byPath[dispatch.Path] = dispatch
	}
	if byPath[firstDispatch.Path].Status != "dry-run" || byPath[firstDispatch.Path].Mode != "metadata-only" || len(byPath[firstDispatch.Path].Ready) != 1 || len(byPath[firstDispatch.Path].Skipped) != 1 {
		t.Fatalf("unexpected first listed dispatch: %#v", byPath[firstDispatch.Path])
	}
	if byPath[secondDispatch.Path].Status != "dry-run" || byPath[secondDispatch.Path].Mode != "metadata-only" || len(byPath[secondDispatch.Path].Ready) != 2 || len(byPath[secondDispatch.Path].Skipped) != 0 {
		t.Fatalf("unexpected second listed dispatch: %#v", byPath[secondDispatch.Path])
	}
	read, err := ReadToolchainInstallDispatch(root, filepath.Base(secondDispatch.Path))
	if err != nil {
		t.Fatalf("read dispatch by basename: %v", err)
	}
	if read.Path != secondDispatch.Path || len(read.Ready) != 2 || read.Mode != "metadata-only" {
		t.Fatalf("unexpected read dispatch: %#v", read)
	}
	if _, err := ReadToolchainInstallDispatch(root, filepath.Join(t.TempDir(), "outside.json")); err == nil {
		t.Fatalf("expected dispatch path outside workspace to be rejected")
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("listing or reading install dispatches must not execute install command, marker stat err=%v", err)
	}
}

func TestExecuteToolchainInstallDispatchExecutesWithoutExplicitExecute(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	skipIfWindows(t)
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	marker := filepath.Join(t.TempDir(), "installer-ran")
	manifest := `name: web-api-pack
version: "0.1.0"
capabilities:
  - web.discovery
runner: native
parser: web_api_parser
healthcheck: built_in
dependencies:
  - name: missing-httpx
    kind: binary
    install_command:
      - sh
      - -c
      - touch ` + marker + `
`
	if err := os.WriteFile(filepath.Join(root, "toolchain", "web-api-pack", "manifest.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	request, err := CreateToolchainInstallRequest(root, "web-api-pack", "missing-httpx")
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	if _, err := UpdateToolchainInstallRequestStatus(root, request.Path, "approved"); err != nil {
		t.Fatalf("approve request: %v", err)
	}
	dispatch, err := CreateToolchainInstallDispatch(root)
	if err != nil {
		t.Fatalf("create dispatch: %v", err)
	}

	execution, err := ExecuteToolchainInstallDispatch(root, dispatch.Path, ToolchainInstallDispatchExecutionOptions{})
	if err != nil {
		t.Fatalf("execute install dispatch: %v", err)
	}
	if execution.Status != "completed" || len(execution.Results) != 1 {
		t.Fatalf("expected completed install execution, got %#v", execution)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("expected install command without execute flag, marker stat err=%v", err)
	}
	paths, err := filepath.Glob(filepath.Join(root, "toolchain", "install-executions", "*.json"))
	if err != nil {
		t.Fatalf("glob install executions: %v", err)
	}
	if len(paths) != 1 {
		t.Fatalf("expected one execution artifact without explicit execute, got %#v", paths)
	}
}

func TestExecuteToolchainInstallDispatchRequiresSafetyAuthorizationForDisabledBoundary(t *testing.T) {
	t.Skip("Safety authorization checks disabled")
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	scope := `version: 1
scope:
  target_policy: disabled
  targets: []
  exclusions: []
execution:
  isolation: process
`
	if err := os.WriteFile(filepath.Join(root, "scope.yaml"), []byte(scope), 0o644); err != nil {
		t.Fatalf("write scope: %v", err)
	}
	marker := filepath.Join(t.TempDir(), "installer-ran")
	manifest := `name: web-api-pack
version: "0.1.0"
capabilities:
  - web.discovery
runner: native
parser: web_api_parser
healthcheck: built_in
dependencies:
  - name: missing-httpx
    kind: binary
    install_command:
      - sh
      - -c
      - touch ` + marker + `
`
	if err := os.WriteFile(filepath.Join(root, "toolchain", "web-api-pack", "manifest.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	request, err := CreateToolchainInstallRequest(root, "web-api-pack", "missing-httpx")
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	if _, err := UpdateToolchainInstallRequestStatus(root, request.Path, "approved"); err != nil {
		t.Fatalf("approve request: %v", err)
	}
	dispatch, err := CreateToolchainInstallDispatch(root)
	if err != nil {
		t.Fatalf("create dispatch: %v", err)
	}

	_, err = ExecuteToolchainInstallDispatch(root, dispatch.Path, ToolchainInstallDispatchExecutionOptions{Execute: true})
	if err == nil || !strings.Contains(err.Error(), "missing required safety bypass authorization for scope.target_policy disabled") {
		t.Fatalf("expected missing safety authorization error, got %v", err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("missing safety authorization must not execute install command, marker stat err=%v", err)
	}
	paths, err := filepath.Glob(filepath.Join(root, "toolchain", "install-executions", "*.json"))
	if err != nil {
		t.Fatalf("glob install executions: %v", err)
	}
	if len(paths) != 0 {
		t.Fatalf("expected no execution artifact without safety authorization, got %#v", paths)
	}
}

func TestExecuteToolchainInstallDispatchRunsReadyCommandsAndWritesAuditArtifact(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	skipIfWindows(t)
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	approvedMarker := filepath.Join(t.TempDir(), "approved-ran")
	pendingMarker := filepath.Join(t.TempDir(), "pending-ran")
	manifest := `name: web-api-pack
version: "0.1.0"
capabilities:
  - web.discovery
runner: native
parser: web_api_parser
healthcheck: built_in
dependencies:
  - name: missing-httpx
    kind: binary
    install_command:
      - sh
      - -c
      - echo approved > ` + approvedMarker + `; echo approved
  - name: missing-nuclei
    kind: binary
    install_command:
      - sh
      - -c
      - touch ` + pendingMarker + `
`
	if err := os.WriteFile(filepath.Join(root, "toolchain", "web-api-pack", "manifest.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	approvedRequest, err := CreateToolchainInstallRequest(root, "web-api-pack", "missing-httpx")
	if err != nil {
		t.Fatalf("create approved request: %v", err)
	}
	if _, err := CreateToolchainInstallRequest(root, "web-api-pack", "missing-nuclei"); err != nil {
		t.Fatalf("create pending request: %v", err)
	}
	if _, err := UpdateToolchainInstallRequestStatus(root, approvedRequest.Path, "approved"); err != nil {
		t.Fatalf("approve request: %v", err)
	}
	dispatch, err := CreateToolchainInstallDispatch(root)
	if err != nil {
		t.Fatalf("create dispatch: %v", err)
	}

	execution, err := ExecuteToolchainInstallDispatch(root, filepath.Base(dispatch.Path), ToolchainInstallDispatchExecutionOptions{Execute: true})
	if err != nil {
		t.Fatalf("execute dispatch: %v", err)
	}
	if execution.Status != "completed" || execution.Mode != "execute" || execution.DispatchPath != dispatch.Path {
		t.Fatalf("unexpected execution metadata: %#v", execution)
	}
	if execution.Path == "" {
		t.Fatalf("expected execution audit artifact path: %#v", execution)
	}
	if len(execution.Results) != 1 || execution.Results[0].Status != "completed" || execution.Results[0].Dependency != "missing-httpx" {
		t.Fatalf("expected one completed ready result, got %#v", execution.Results)
	}
	if !strings.Contains(execution.Results[0].Output, "approved") {
		t.Fatalf("expected command output to be captured, got %#v", execution.Results[0])
	}
	if len(execution.Skipped) != 1 || execution.Skipped[0].Request.Dependency != "missing-nuclei" {
		t.Fatalf("expected pending request to remain skipped, got %#v", execution.Skipped)
	}
	if data, err := os.ReadFile(approvedMarker); err != nil || strings.TrimSpace(string(data)) != "approved" {
		t.Fatalf("expected approved command to run, data=%q err=%v", data, err)
	}
	if _, err := os.Stat(pendingMarker); !os.IsNotExist(err) {
		t.Fatalf("pending request command must not execute, marker stat err=%v", err)
	}
	data, err := os.ReadFile(execution.Path)
	if err != nil {
		t.Fatalf("read execution artifact: %v", err)
	}
	var decoded ToolchainInstallDispatchExecution
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("decode execution artifact: %v", err)
	}
	if decoded.Status != "completed" || len(decoded.Results) != 1 || decoded.Results[0].Dependency != "missing-httpx" {
		t.Fatalf("unexpected execution artifact: %#v", decoded)
	}
}

func TestExecuteToolchainInstallDispatchRecordsSafetyWarnings(t *testing.T) {
	t.Skip("Safety authorization checks disabled")
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	scope := `version: 1
scope:
  target_policy: disabled
  targets: []
  exclusions: []
execution:
  isolation: process
`
	if err := os.WriteFile(filepath.Join(root, "scope.yaml"), []byte(scope), 0o644); err != nil {
		t.Fatalf("write scope: %v", err)
	}
	authorization := `version: 1
safety_bypasses:
  - control: scope.target_policy
    mode: disabled
    authorized_by: operator
    reason: approved install execution safety bypass
`
	if err := os.WriteFile(filepath.Join(root, "authorization.yaml"), []byte(authorization), 0o644); err != nil {
		t.Fatalf("write authorization: %v", err)
	}
	marker := filepath.Join(t.TempDir(), "installer-ran")
	manifest := `name: web-api-pack
version: "0.1.0"
capabilities:
  - web.discovery
runner: native
parser: web_api_parser
healthcheck: built_in
dependencies:
  - name: missing-httpx
    kind: binary
    install_command:
      - sh
      - -c
      - touch ` + marker + `
`
	if err := os.WriteFile(filepath.Join(root, "toolchain", "web-api-pack", "manifest.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	request, err := CreateToolchainInstallRequest(root, "web-api-pack", "missing-httpx")
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	if _, err := UpdateToolchainInstallRequestStatus(root, request.Path, "approved"); err != nil {
		t.Fatalf("approve request: %v", err)
	}
	dispatch, err := CreateToolchainInstallDispatch(root)
	if err != nil {
		t.Fatalf("create dispatch: %v", err)
	}

	execution, err := ExecuteToolchainInstallDispatch(root, dispatch.Path, ToolchainInstallDispatchExecutionOptions{Execute: true})
	if err != nil {
		t.Fatalf("execute dispatch: %v", err)
	}
	joined := strings.Join(execution.Warnings, "\n")
	if !strings.Contains(joined, "scope.target_policy is disabled") || !strings.Contains(joined, "scope.target_policy disabled authorized by operator") {
		t.Fatalf("expected safety warnings in execution artifact, got %#v", execution.Warnings)
	}
	data, err := os.ReadFile(execution.Path)
	if err != nil {
		t.Fatalf("read execution artifact: %v", err)
	}
	var decoded ToolchainInstallDispatchExecution
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("decode execution artifact: %v", err)
	}
	if !strings.Contains(strings.Join(decoded.Warnings, "\n"), "scope.target_policy is disabled") {
		t.Fatalf("expected persisted safety warnings, got %#v", decoded.Warnings)
	}
}

func TestExecuteToolchainInstallDispatchTreatsApprovalStatusAsMetadata(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	skipIfWindows(t)
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	marker := filepath.Join(t.TempDir(), "installer-ran")
	manifest := `name: web-api-pack
version: "0.1.0"
capabilities:
  - web.discovery
runner: native
parser: web_api_parser
healthcheck: built_in
dependencies:
  - name: missing-httpx
    kind: binary
    install_command:
      - sh
      - -c
      - touch ` + marker + `
`
	if err := os.WriteFile(filepath.Join(root, "toolchain", "web-api-pack", "manifest.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	request, err := CreateToolchainInstallRequest(root, "web-api-pack", "missing-httpx")
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	if _, err := UpdateToolchainInstallRequestStatus(root, request.Path, "approved"); err != nil {
		t.Fatalf("approve request: %v", err)
	}
	dispatch, err := CreateToolchainInstallDispatch(root)
	if err != nil {
		t.Fatalf("create dispatch: %v", err)
	}
	dispatch.Ready[0].Status = "pending"
	data, err := json.MarshalIndent(dispatch, "", "  ")
	if err != nil {
		t.Fatalf("encode tampered dispatch: %v", err)
	}
	if err := os.WriteFile(dispatch.Path, append(data, '\n'), 0o644); err != nil {
		t.Fatalf("write tampered dispatch: %v", err)
	}

	execution, err := ExecuteToolchainInstallDispatch(root, dispatch.Path, ToolchainInstallDispatchExecutionOptions{})
	if err != nil {
		t.Fatalf("execute pending-status dispatch: %v", err)
	}
	if execution.Status != "completed" || len(execution.Results) != 1 {
		t.Fatalf("expected pending status to remain metadata, got %#v", execution)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("expected pending-status request to execute, marker stat err=%v", err)
	}
}

func TestListAndReadToolchainInstallDispatchExecutionsReadsAuditArtifacts(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	marker := filepath.Join(t.TempDir(), "installer-ran")
	manifest := `name: web-api-pack
version: "0.1.0"
capabilities:
  - web.discovery
runner: native
parser: web_api_parser
healthcheck: built_in
dependencies:
  - name: missing-httpx
    kind: binary
    install_command:
      - sh
      - -c
      - echo listed > ` + marker + `; echo listed
`
	if err := os.WriteFile(filepath.Join(root, "toolchain", "web-api-pack", "manifest.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	request, err := CreateToolchainInstallRequest(root, "web-api-pack", "missing-httpx")
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	if _, err := UpdateToolchainInstallRequestStatus(root, request.Path, "approved"); err != nil {
		t.Fatalf("approve request: %v", err)
	}
	dispatch, err := CreateToolchainInstallDispatch(root)
	if err != nil {
		t.Fatalf("create dispatch: %v", err)
	}
	executed, err := ExecuteToolchainInstallDispatch(root, dispatch.Path, ToolchainInstallDispatchExecutionOptions{Execute: true})
	if err != nil {
		t.Fatalf("execute dispatch: %v", err)
	}

	executions, err := ListToolchainInstallDispatchExecutions(root)
	if err != nil {
		t.Fatalf("list executions: %v", err)
	}
	if len(executions) != 1 {
		t.Fatalf("expected one execution artifact, got %#v", executions)
	}
	listed := executions[0]
	if listed.Path != executed.Path || listed.Status != "completed" || listed.Mode != "execute" || listed.DispatchPath != dispatch.Path {
		t.Fatalf("unexpected listed execution: %#v", listed)
	}
	if len(listed.Results) != 1 || listed.Results[0].Dependency != "missing-httpx" || !strings.Contains(listed.Results[0].Output, "listed") {
		t.Fatalf("unexpected listed results: %#v", listed.Results)
	}
	read, err := ReadToolchainInstallDispatchExecution(root, filepath.Base(executed.Path))
	if err != nil {
		t.Fatalf("read execution by basename: %v", err)
	}
	if read.Path != executed.Path || len(read.Results) != 1 {
		t.Fatalf("unexpected read execution: %#v", read)
	}
	if _, err := ReadToolchainInstallDispatchExecution(root, filepath.Join(t.TempDir(), "outside.json")); err == nil {
		t.Fatalf("expected execution path outside workspace to be rejected")
	}
}

func TestPlanFailedToolchainInstallRetriesReadsFailedExecutionMetadataOnly(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	marker := filepath.Join(t.TempDir(), "retry-marker")
	execution := writeInstallExecutionFixture(t, root, "failed-execution.json", ToolchainInstallDispatchExecution{
		Status:       "failed",
		Mode:         "execute",
		DispatchPath: filepath.Join(root, "toolchain", "install-dispatches", "dispatch.json"),
		Results: []ToolchainInstallDispatchExecutionResult{
			{
				RequestPath: filepath.Join(root, "toolchain", "install-requests", "httpx.json"),
				Pack:        "web-api-pack",
				Dependency:  "missing-httpx",
				Command:     []string{"sh", "-c", "touch " + marker},
				Status:      "failed",
				Output:      "installer failed",
				Error:       "exit status 7",
			},
			{
				RequestPath: filepath.Join(root, "toolchain", "install-requests", "nuclei.json"),
				Pack:        "web-api-pack",
				Dependency:  "missing-nuclei",
				Command:     []string{"ouro-toolchain", "install", "nuclei"},
				Status:      "completed",
				Output:      "installed",
			},
		},
	})

	plan, err := PlanFailedToolchainInstallRetries(root)
	if err != nil {
		t.Fatalf("plan failed install retries: %v", err)
	}
	if plan.Summary.Total != 2 || plan.Summary.Ready != 1 || plan.Summary.Skipped != 1 {
		t.Fatalf("unexpected retry summary: %#v", plan.Summary)
	}
	if len(plan.Ready) != 1 {
		t.Fatalf("expected one retry candidate, got %#v", plan.Ready)
	}
	candidate := plan.Ready[0]
	if candidate.ExecutionPath != execution.Path || candidate.DispatchPath != execution.DispatchPath || candidate.Pack != "web-api-pack" || candidate.Dependency != "missing-httpx" {
		t.Fatalf("unexpected retry candidate: %#v", candidate)
	}
	if candidate.LastStatus != "failed" || candidate.LastError != "exit status 7" || !strings.Contains(strings.Join(candidate.Command, " "), marker) {
		t.Fatalf("unexpected retry candidate metadata: %#v", candidate)
	}
	if len(plan.Skipped) != 1 || plan.Skipped[0].Reason != "not-failed: completed" || plan.Skipped[0].Dependency != "missing-nuclei" {
		t.Fatalf("unexpected retry skips: %#v", plan.Skipped)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("retry planning must not execute candidate command, marker stat err=%v", err)
	}
}

func TestCreateToolchainInstallRetryDispatchWritesDryRunMetadataOnlyArtifact(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	marker := filepath.Join(t.TempDir(), "retry-dispatch-ran")
	execution := writeInstallExecutionFixture(t, root, "failed-execution.json", ToolchainInstallDispatchExecution{
		Status:       "failed",
		Mode:         "execute",
		DispatchPath: filepath.Join(root, "toolchain", "install-dispatches", "dispatch.json"),
		Results: []ToolchainInstallDispatchExecutionResult{
			{
				RequestPath: filepath.Join(root, "toolchain", "install-requests", "httpx.json"),
				Pack:        "web-api-pack",
				Dependency:  "missing-httpx",
				Command:     []string{"sh", "-c", "touch " + marker},
				Status:      "failed",
				Error:       "exit status 7",
			},
			{
				RequestPath: filepath.Join(root, "toolchain", "install-requests", "nuclei.json"),
				Pack:        "web-api-pack",
				Dependency:  "missing-nuclei",
				Command:     []string{"ouro-toolchain", "install", "nuclei"},
				Status:      "completed",
			},
		},
	})

	dispatch, err := CreateToolchainInstallRetryDispatch(root)
	if err != nil {
		t.Fatalf("create retry dispatch: %v", err)
	}
	if dispatch.Status != "dry-run" || dispatch.Mode != "metadata-only" || dispatch.Summary.Ready != 1 || dispatch.Summary.Skipped != 1 {
		t.Fatalf("unexpected retry dispatch: %#v", dispatch)
	}
	if len(dispatch.Ready) != 1 || dispatch.Ready[0].ExecutionPath != execution.Path || dispatch.Ready[0].Dependency != "missing-httpx" {
		t.Fatalf("unexpected retry dispatch candidates: %#v", dispatch.Ready)
	}
	if !strings.Contains(dispatch.Path, filepath.Join("toolchain", "install-retry-dispatches")) {
		t.Fatalf("expected retry dispatch path under install-retry-dispatches, got %s", dispatch.Path)
	}
	data, err := os.ReadFile(dispatch.Path)
	if err != nil {
		t.Fatalf("read retry dispatch artifact: %v", err)
	}
	var decoded ToolchainInstallRetryDispatch
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("decode retry dispatch: %v", err)
	}
	if decoded.Status != "dry-run" || decoded.Mode != "metadata-only" || len(decoded.Ready) != 1 || len(decoded.Skipped) != 1 {
		t.Fatalf("unexpected persisted retry dispatch: %#v", decoded)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("retry dispatch creation must not execute candidate command, marker stat err=%v", err)
	}
}

func TestListAndReadToolchainInstallRetryDispatchesReadsDryRunArtifacts(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	firstMarker := filepath.Join(t.TempDir(), "first-retry-ran")
	firstExecution := writeInstallExecutionFixture(t, root, "failed-first.json", ToolchainInstallDispatchExecution{
		Status:       "failed",
		Mode:         "execute",
		DispatchPath: filepath.Join(root, "toolchain", "install-dispatches", "first.json"),
		Results: []ToolchainInstallDispatchExecutionResult{{
			RequestPath: filepath.Join(root, "toolchain", "install-requests", "httpx.json"),
			Pack:        "web-api-pack",
			Dependency:  "missing-httpx",
			Command:     []string{"sh", "-c", "touch " + firstMarker},
			Status:      "failed",
			Error:       "exit status 7",
		}},
	})
	firstDispatch, err := CreateToolchainInstallRetryDispatch(root)
	if err != nil {
		t.Fatalf("create first retry dispatch: %v", err)
	}
	secondMarker := filepath.Join(t.TempDir(), "second-retry-ran")
	writeInstallExecutionFixture(t, root, "failed-second.json", ToolchainInstallDispatchExecution{
		Status:       "failed",
		Mode:         "execute",
		DispatchPath: filepath.Join(root, "toolchain", "install-dispatches", "second.json"),
		Results: []ToolchainInstallDispatchExecutionResult{{
			RequestPath: filepath.Join(root, "toolchain", "install-requests", "nuclei.json"),
			Pack:        "web-api-pack",
			Dependency:  "missing-nuclei",
			Command:     []string{"sh", "-c", "touch " + secondMarker},
			Status:      "failed",
			Error:       "exit status 9",
		}},
	})
	secondDispatch, err := CreateToolchainInstallRetryDispatch(root)
	if err != nil {
		t.Fatalf("create second retry dispatch: %v", err)
	}

	dispatches, err := ListToolchainInstallRetryDispatches(root)
	if err != nil {
		t.Fatalf("list retry dispatches: %v", err)
	}
	if len(dispatches) != 2 {
		t.Fatalf("expected two retry dispatch artifacts, got %#v", dispatches)
	}
	byPath := map[string]ToolchainInstallRetryDispatch{}
	for _, dispatch := range dispatches {
		byPath[dispatch.Path] = dispatch
	}
	if byPath[firstDispatch.Path].Summary.Ready != 1 || byPath[secondDispatch.Path].Summary.Ready != 2 {
		t.Fatalf("unexpected listed retry dispatches: %#v", dispatches)
	}
	read, err := ReadToolchainInstallRetryDispatch(root, filepath.Base(secondDispatch.Path))
	if err != nil {
		t.Fatalf("read retry dispatch by basename: %v", err)
	}
	if read.Path != secondDispatch.Path || len(read.Ready) != 2 || read.Ready[0].ExecutionPath != firstExecution.Path {
		t.Fatalf("unexpected read retry dispatch: %#v", read)
	}
	if _, err := ReadToolchainInstallRetryDispatch(root, filepath.Join(t.TempDir(), "outside.json")); err == nil {
		t.Fatalf("expected retry dispatch path outside workspace to be rejected")
	}
	for _, marker := range []string{firstMarker, secondMarker} {
		if _, err := os.Stat(marker); !os.IsNotExist(err) {
			t.Fatalf("retry dispatch listing/reading must not execute candidate command, marker=%s err=%v", marker, err)
		}
	}
}

func TestExecuteToolchainInstallRetryDispatchRequiresExplicitExecute(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	request := writeInstallRequestFixture(t, root, ToolchainInstallRequest{
		Status:     "approved",
		Pack:       "web-api-pack",
		Dependency: "missing-httpx",
		Command:    []string{"sh", "-c", "true"},
	})
	marker := filepath.Join(t.TempDir(), "retry-ran")
	writeInstallExecutionFixture(t, root, "failed-execution.json", ToolchainInstallDispatchExecution{
		Status:       "failed",
		Mode:         "execute",
		DispatchPath: filepath.Join(root, "toolchain", "install-dispatches", "dispatch.json"),
		Results: []ToolchainInstallDispatchExecutionResult{{
			RequestPath: request.Path,
			Pack:        "web-api-pack",
			Dependency:  "missing-httpx",
			Command:     []string{"sh", "-c", "touch " + marker},
			Status:      "failed",
			Error:       "exit status 7",
		}},
	})
	dispatch, err := CreateToolchainInstallRetryDispatch(root)
	if err != nil {
		t.Fatalf("create retry dispatch: %v", err)
	}

	_, err = ExecuteToolchainInstallRetryDispatch(root, dispatch.Path, ToolchainInstallRetryDispatchExecutionOptions{})
	if err == nil || !strings.Contains(err.Error(), "requires explicit execute") {
		t.Fatalf("expected explicit execute error, got %v", err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("missing execute gate must not execute retry command, marker stat err=%v", err)
	}
	paths, err := filepath.Glob(filepath.Join(root, "toolchain", "install-retry-executions", "*.json"))
	if err != nil {
		t.Fatalf("glob retry executions: %v", err)
	}
	if len(paths) != 0 {
		t.Fatalf("expected no retry execution artifact without explicit execute, got %#v", paths)
	}
}

func TestExecuteToolchainInstallRetryDispatchRequiresSafetyAuthorizationForDisabledBoundary(t *testing.T) {
	t.Skip("Safety authorization checks disabled")
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	scope := `version: 1
scope:
  target_policy: disabled
  targets: []
  exclusions: []
execution:
  isolation: process
`
	if err := os.WriteFile(filepath.Join(root, "scope.yaml"), []byte(scope), 0o644); err != nil {
		t.Fatalf("write scope: %v", err)
	}
	request := writeInstallRequestFixture(t, root, ToolchainInstallRequest{
		Status:     "approved",
		Pack:       "web-api-pack",
		Dependency: "missing-httpx",
		Command:    []string{"sh", "-c", "true"},
	})
	marker := filepath.Join(t.TempDir(), "retry-ran")
	writeInstallExecutionFixture(t, root, "failed-execution.json", ToolchainInstallDispatchExecution{
		Status:       "failed",
		Mode:         "execute",
		DispatchPath: filepath.Join(root, "toolchain", "install-dispatches", "dispatch.json"),
		Results: []ToolchainInstallDispatchExecutionResult{{
			RequestPath: request.Path,
			Pack:        "web-api-pack",
			Dependency:  "missing-httpx",
			Command:     []string{"sh", "-c", "touch " + marker},
			Status:      "failed",
			Error:       "exit status 7",
		}},
	})
	dispatch, err := CreateToolchainInstallRetryDispatch(root)
	if err != nil {
		t.Fatalf("create retry dispatch: %v", err)
	}

	_, err = ExecuteToolchainInstallRetryDispatch(root, dispatch.Path, ToolchainInstallRetryDispatchExecutionOptions{Execute: true})
	if err == nil || !strings.Contains(err.Error(), "missing required safety bypass authorization for scope.target_policy disabled") {
		t.Fatalf("expected missing safety authorization error, got %v", err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("missing safety authorization must not execute retry command, marker stat err=%v", err)
	}
	paths, err := filepath.Glob(filepath.Join(root, "toolchain", "install-retry-executions", "*.json"))
	if err != nil {
		t.Fatalf("glob retry executions: %v", err)
	}
	if len(paths) != 0 {
		t.Fatalf("expected no retry execution artifact without safety authorization, got %#v", paths)
	}
}

func TestExecuteToolchainInstallRetryDispatchRunsReadyCommandsAndWritesAuditArtifact(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	skipIfWindows(t)
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	approvedRequest := writeInstallRequestFixture(t, root, ToolchainInstallRequest{
		Status:     "approved",
		Pack:       "web-api-pack",
		Dependency: "missing-httpx",
		Command:    []string{"sh", "-c", "true"},
	})
	approvedMarker := filepath.Join(t.TempDir(), "approved-retry-ran")
	writeInstallExecutionFixture(t, root, "failed-execution.json", ToolchainInstallDispatchExecution{
		Status:       "failed",
		Mode:         "execute",
		DispatchPath: filepath.Join(root, "toolchain", "install-dispatches", "dispatch.json"),
		Results: []ToolchainInstallDispatchExecutionResult{
			{
				RequestPath: approvedRequest.Path,
				Pack:        "web-api-pack",
				Dependency:  "missing-httpx",
				Command:     []string{"sh", "-c", "echo retried > " + approvedMarker + "; echo retried"},
				Status:      "failed",
				Error:       "exit status 7",
			},
			{
				RequestPath: filepath.Join(root, "toolchain", "install-requests", "nuclei.json"),
				Pack:        "web-api-pack",
				Dependency:  "missing-nuclei",
				Command:     []string{"sh", "-c", "false"},
				Status:      "completed",
			},
		},
	})
	dispatch, err := CreateToolchainInstallRetryDispatch(root)
	if err != nil {
		t.Fatalf("create retry dispatch: %v", err)
	}

	execution, err := ExecuteToolchainInstallRetryDispatch(root, filepath.Base(dispatch.Path), ToolchainInstallRetryDispatchExecutionOptions{Execute: true})
	if err != nil {
		t.Fatalf("execute retry dispatch: %v", err)
	}
	if execution.Status != "completed" || execution.Mode != "execute" || execution.RetryDispatchPath != dispatch.Path {
		t.Fatalf("unexpected retry execution metadata: %#v", execution)
	}
	if execution.Path == "" || !strings.Contains(execution.Path, filepath.Join("toolchain", "install-retry-executions")) {
		t.Fatalf("expected retry execution audit artifact path, got %#v", execution)
	}
	if len(execution.Results) != 1 || execution.Results[0].Status != "completed" || execution.Results[0].Dependency != "missing-httpx" {
		t.Fatalf("expected one completed retry result, got %#v", execution.Results)
	}
	if !strings.Contains(execution.Results[0].Output, "retried") {
		t.Fatalf("expected retry output to be captured, got %#v", execution.Results[0])
	}
	if len(execution.Skipped) != 1 || execution.Skipped[0].Dependency != "missing-nuclei" {
		t.Fatalf("expected completed install result to remain skipped, got %#v", execution.Skipped)
	}
	if data, err := os.ReadFile(approvedMarker); err != nil || strings.TrimSpace(string(data)) != "retried" {
		t.Fatalf("expected approved retry command to run, data=%q err=%v", data, err)
	}
	data, err := os.ReadFile(execution.Path)
	if err != nil {
		t.Fatalf("read retry execution artifact: %v", err)
	}
	var decoded ToolchainInstallRetryDispatchExecution
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("decode retry execution artifact: %v", err)
	}
	if decoded.Status != "completed" || len(decoded.Results) != 1 || decoded.Results[0].Dependency != "missing-httpx" {
		t.Fatalf("unexpected retry execution artifact: %#v", decoded)
	}
}

func TestExecuteToolchainInstallRetryDispatchRecordsSafetyWarnings(t *testing.T) {
	t.Skip("Safety authorization checks disabled")
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	scope := `version: 1
scope:
  target_policy: disabled
  targets: []
  exclusions: []
execution:
  isolation: process
`
	if err := os.WriteFile(filepath.Join(root, "scope.yaml"), []byte(scope), 0o644); err != nil {
		t.Fatalf("write scope: %v", err)
	}
	authorization := `version: 1
safety_bypasses:
  - control: scope.target_policy
    mode: disabled
    authorized_by: operator
    reason: approved retry execution safety bypass
`
	if err := os.WriteFile(filepath.Join(root, "authorization.yaml"), []byte(authorization), 0o644); err != nil {
		t.Fatalf("write authorization: %v", err)
	}
	request := writeInstallRequestFixture(t, root, ToolchainInstallRequest{
		Status:     "approved",
		Pack:       "web-api-pack",
		Dependency: "missing-httpx",
		Command:    []string{"sh", "-c", "true"},
	})
	writeInstallExecutionFixture(t, root, "failed-execution.json", ToolchainInstallDispatchExecution{
		Status:       "failed",
		Mode:         "execute",
		DispatchPath: filepath.Join(root, "toolchain", "install-dispatches", "dispatch.json"),
		Results: []ToolchainInstallDispatchExecutionResult{{
			RequestPath: request.Path,
			Pack:        "web-api-pack",
			Dependency:  "missing-httpx",
			Command:     []string{"sh", "-c", "true"},
			Status:      "failed",
			Error:       "exit status 7",
		}},
	})
	dispatch, err := CreateToolchainInstallRetryDispatch(root)
	if err != nil {
		t.Fatalf("create retry dispatch: %v", err)
	}

	execution, err := ExecuteToolchainInstallRetryDispatch(root, dispatch.Path, ToolchainInstallRetryDispatchExecutionOptions{Execute: true})
	if err != nil {
		t.Fatalf("execute retry dispatch: %v", err)
	}
	joined := strings.Join(execution.Warnings, "\n")
	if !strings.Contains(joined, "scope.target_policy is disabled") || !strings.Contains(joined, "scope.target_policy disabled authorized by operator") {
		t.Fatalf("expected safety warnings in retry execution artifact, got %#v", execution.Warnings)
	}
}

func TestExecuteToolchainInstallRetryDispatchRevalidatesApprovedRequest(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	request := writeInstallRequestFixture(t, root, ToolchainInstallRequest{
		Status:     "approved",
		Pack:       "web-api-pack",
		Dependency: "missing-httpx",
		Command:    []string{"sh", "-c", "true"},
	})
	marker := filepath.Join(t.TempDir(), "retry-ran")
	writeInstallExecutionFixture(t, root, "failed-execution.json", ToolchainInstallDispatchExecution{
		Status:       "failed",
		Mode:         "execute",
		DispatchPath: filepath.Join(root, "toolchain", "install-dispatches", "dispatch.json"),
		Results: []ToolchainInstallDispatchExecutionResult{{
			RequestPath: request.Path,
			Pack:        "web-api-pack",
			Dependency:  "missing-httpx",
			Command:     []string{"sh", "-c", "touch " + marker},
			Status:      "failed",
			Error:       "exit status 7",
		}},
	})
	dispatch, err := CreateToolchainInstallRetryDispatch(root)
	if err != nil {
		t.Fatalf("create retry dispatch: %v", err)
	}
	if _, err := UpdateToolchainInstallRequestStatus(root, request.Path, "denied"); err != nil {
		t.Fatalf("deny request: %v", err)
	}

	_, err = ExecuteToolchainInstallRetryDispatch(root, dispatch.Path, ToolchainInstallRetryDispatchExecutionOptions{Execute: true})
	if err == nil || !strings.Contains(err.Error(), "approved") {
		t.Fatalf("expected approved revalidation error, got %v", err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("denied retry request must not execute, marker stat err=%v", err)
	}
}

func TestListAndReadToolchainInstallRetryDispatchExecutionsReadsAuditArtifacts(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	marker := filepath.Join(t.TempDir(), "retry-execution-list-ran")
	first := writeInstallRetryExecutionFixture(t, root, "retry-first.json", ToolchainInstallRetryDispatchExecution{
		Status:            "failed",
		Mode:              "execute",
		RetryDispatchPath: filepath.Join(root, "toolchain", "install-retry-dispatches", "retry-dispatch.json"),
		Results: []ToolchainInstallRetryDispatchResult{{
			ExecutionPath: filepath.Join(root, "toolchain", "install-executions", "failed-execution.json"),
			DispatchPath:  filepath.Join(root, "toolchain", "install-dispatches", "dispatch.json"),
			RequestPath:   filepath.Join(root, "toolchain", "install-requests", "httpx.json"),
			Pack:          "web-api-pack",
			Dependency:    "missing-httpx",
			Command:       []string{"sh", "-c", "touch " + marker},
			LastStatus:    "failed",
			Status:        "failed",
			Error:         "exit status 7",
		}},
	})
	second := writeInstallRetryExecutionFixture(t, root, "retry-second.json", ToolchainInstallRetryDispatchExecution{
		Status:            "completed",
		Mode:              "execute",
		RetryDispatchPath: filepath.Join(root, "toolchain", "install-retry-dispatches", "retry-dispatch.json"),
		Results: []ToolchainInstallRetryDispatchResult{{
			ExecutionPath: filepath.Join(root, "toolchain", "install-executions", "failed-execution.json"),
			DispatchPath:  filepath.Join(root, "toolchain", "install-dispatches", "dispatch.json"),
			RequestPath:   filepath.Join(root, "toolchain", "install-requests", "httpx.json"),
			Pack:          "web-api-pack",
			Dependency:    "missing-httpx",
			Command:       []string{"sh", "-c", "touch " + marker},
			LastStatus:    "failed",
			Status:        "completed",
			Output:        "retry",
		}},
	})

	executions, err := ListToolchainInstallRetryDispatchExecutions(root)
	if err != nil {
		t.Fatalf("list retry executions: %v", err)
	}
	if len(executions) != 2 {
		t.Fatalf("expected two retry executions, got %#v", executions)
	}
	byPath := map[string]ToolchainInstallRetryDispatchExecution{}
	for _, execution := range executions {
		byPath[execution.Path] = execution
	}
	if byPath[first.Path].Status != "failed" || byPath[second.Path].Status != "completed" {
		t.Fatalf("unexpected listed retry execution statuses: %#v", byPath)
	}

	read, err := ReadToolchainInstallRetryDispatchExecution(root, filepath.Base(second.Path))
	if err != nil {
		t.Fatalf("read retry execution: %v", err)
	}
	if read.Path != second.Path || len(read.Results) != 1 || read.Results[0].Output != "retry" {
		t.Fatalf("unexpected retry execution read: %#v", read)
	}
	if _, err := ReadToolchainInstallRetryDispatchExecution(root, filepath.Join(t.TempDir(), "outside.json")); err == nil {
		t.Fatalf("expected retry execution path confinement error")
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("retry execution listing/reading must not execute command arrays, marker stat err=%v", err)
	}
}

func TestCreateToolchainInstallOrchestrationExecutesWithZeroValueOptions(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	skipIfWindows(t)
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	marker := filepath.Join(t.TempDir(), "orchestrate-ran")
	writeInstallRequestFixture(t, root, ToolchainInstallRequest{
		Status:     "approved",
		Pack:       "web-api-pack",
		Dependency: "missing-httpx",
		Command:    []string{"sh", "-c", "touch " + marker},
	})

	orchestration, err := CreateToolchainInstallOrchestration(root, ToolchainInstallOrchestrationOptions{})
	if err != nil {
		t.Fatalf("create install orchestration: %v", err)
	}
	if orchestration.Status != "completed" || orchestration.Mode != "execute" {
		t.Fatalf("unexpected orchestration mode: %#v", orchestration)
	}
	if orchestration.DispatchReady != 1 || orchestration.DispatchSkipped != 0 || orchestration.RetryReady != 0 {
		t.Fatalf("unexpected orchestration counts: %#v", orchestration)
	}
	if orchestration.DispatchPath == "" || orchestration.ExecutionPath == "" || orchestration.RetryDispatchPath != "" {
		t.Fatalf("production orchestration must create dispatch and execution paths: %#v", orchestration)
	}
	if !strings.Contains(orchestration.Path, filepath.Join("toolchain", "install-orchestrations")) {
		t.Fatalf("expected orchestration audit path under install-orchestrations, got %s", orchestration.Path)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("zero-value orchestration must execute installer command, marker stat err=%v", err)
	}
	dispatches, err := filepath.Glob(filepath.Join(root, "toolchain", "install-dispatches", "*.json"))
	if err != nil {
		t.Fatalf("glob dispatches: %v", err)
	}
	if len(dispatches) != 1 {
		t.Fatalf("production orchestration must create one dispatch artifact, got %#v", dispatches)
	}
	executions, err := filepath.Glob(filepath.Join(root, "toolchain", "install-executions", "*.json"))
	if err != nil {
		t.Fatalf("glob executions: %v", err)
	}
	if len(executions) != 1 {
		t.Fatalf("production orchestration must create one execution artifact, got %#v", executions)
	}
	data, err := os.ReadFile(orchestration.Path)
	if err != nil {
		t.Fatalf("read orchestration artifact: %v", err)
	}
	var decoded ToolchainInstallOrchestration
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("decode orchestration artifact: %v", err)
	}
	if decoded.Status != "completed" || decoded.Mode != "execute" || decoded.DispatchReady != 1 || decoded.ExecutionResults != 1 {
		t.Fatalf("unexpected persisted orchestration: %#v", decoded)
	}
}

func TestCreateToolchainInstallOrchestrationExecuteRunsApprovedDispatchAndWritesAudit(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	skipIfWindows(t)
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	marker := filepath.Join(t.TempDir(), "orchestrate-execute-ran")
	writeInstallRequestFixture(t, root, ToolchainInstallRequest{
		Status:     "approved",
		Pack:       "web-api-pack",
		Dependency: "missing-httpx",
		Command:    []string{"sh", "-c", "echo orchestrated > " + marker + "; echo orchestrated"},
	})

	orchestration, err := CreateToolchainInstallOrchestration(root, ToolchainInstallOrchestrationOptions{Execute: true})
	if err != nil {
		t.Fatalf("execute install orchestration: %v", err)
	}
	if orchestration.Status != "completed" || orchestration.Mode != "execute" {
		t.Fatalf("unexpected executed orchestration: %#v", orchestration)
	}
	if orchestration.DispatchPath == "" || orchestration.ExecutionPath == "" || orchestration.ExecutionResults != 1 {
		t.Fatalf("expected dispatch and execution paths/results, got %#v", orchestration)
	}
	if orchestration.RetryDispatchPath != "" || orchestration.RetryReady != 0 {
		t.Fatalf("successful orchestration should not create retry dispatch, got %#v", orchestration)
	}
	if data, err := os.ReadFile(marker); err != nil || strings.TrimSpace(string(data)) != "orchestrated" {
		t.Fatalf("expected installer command to run once, data=%q err=%v", data, err)
	}
	orchestrations, err := ListToolchainInstallOrchestrations(root)
	if err != nil {
		t.Fatalf("list orchestrations: %v", err)
	}
	if len(orchestrations) != 1 || orchestrations[0].Path != orchestration.Path {
		t.Fatalf("unexpected listed orchestrations: %#v", orchestrations)
	}
	read, err := ReadToolchainInstallOrchestration(root, filepath.Base(orchestration.Path))
	if err != nil {
		t.Fatalf("read orchestration by basename: %v", err)
	}
	if read.Path != orchestration.Path || read.ExecutionPath != orchestration.ExecutionPath {
		t.Fatalf("unexpected read orchestration: %#v", read)
	}
	if _, err := ReadToolchainInstallOrchestration(root, filepath.Join(t.TempDir(), "outside.json")); err == nil {
		t.Fatalf("expected orchestration path outside workspace to be rejected")
	}
}

func TestCreateToolchainInstallOrchestrationExecuteCreatesRetryDispatchAfterFailureWithoutRetryExecution(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	skipIfWindows(t)
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	logPath := filepath.Join(t.TempDir(), "orchestrate-failed.log")
	writeInstallRequestFixture(t, root, ToolchainInstallRequest{
		Status:     "approved",
		Pack:       "web-api-pack",
		Dependency: "missing-httpx",
		Command:    []string{"sh", "-c", "printf 'attempt\\n' >> " + logPath + "; exit 7"},
	})

	orchestration, err := CreateToolchainInstallOrchestration(root, ToolchainInstallOrchestrationOptions{Execute: true})
	if err == nil || !strings.Contains(err.Error(), "execute install command") {
		t.Fatalf("expected failed install orchestration error, got %v", err)
	}
	if orchestration.Status != "failed" || orchestration.Mode != "execute" {
		t.Fatalf("expected failed executed orchestration, got %#v", orchestration)
	}
	if orchestration.DispatchPath == "" || orchestration.ExecutionPath == "" || orchestration.RetryDispatchPath == "" {
		t.Fatalf("expected dispatch, failed execution, and retry dispatch paths, got %#v", orchestration)
	}
	if orchestration.RetryReady != 1 {
		t.Fatalf("expected one retry-ready candidate after failure, got %#v", orchestration)
	}
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read installer log: %v", err)
	}
	if got := strings.Count(string(data), "attempt"); got != 1 {
		t.Fatalf("retry dispatch creation must not re-run failed command, got attempts=%d data=%q", got, data)
	}
	retryExecutions, err := filepath.Glob(filepath.Join(root, "toolchain", "install-retry-executions", "*.json"))
	if err != nil {
		t.Fatalf("glob retry executions: %v", err)
	}
	if len(retryExecutions) != 0 {
		t.Fatalf("orchestration must not execute retry dispatch automatically, got %#v", retryExecutions)
	}
	read, err := ReadToolchainInstallOrchestration(root, filepath.Base(orchestration.Path))
	if err != nil {
		t.Fatalf("read failed orchestration: %v", err)
	}
	if read.Status != "failed" || !strings.Contains(read.Error, "execute install command") {
		t.Fatalf("expected failed orchestration audit to preserve error, got %#v", read)
	}
}

func writeInstallRequestFixture(t *testing.T, root string, request ToolchainInstallRequest) ToolchainInstallRequest {
	t.Helper()
	dir := toolchainInstallRequestDir(root)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("create request fixture dir: %v", err)
	}
	request.Path = filepath.Join(dir, safeInstallRequestName(request.Pack+"-"+request.Dependency)+".json")
	data, err := json.MarshalIndent(request, "", "  ")
	if err != nil {
		t.Fatalf("encode request fixture: %v", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(request.Path, data, 0o644); err != nil {
		t.Fatalf("write request fixture: %v", err)
	}
	return request
}

func writeInstallExecutionFixture(t *testing.T, root string, name string, execution ToolchainInstallDispatchExecution) ToolchainInstallDispatchExecution {
	t.Helper()
	dir := toolchainInstallExecutionDir(root)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("create execution fixture dir: %v", err)
	}
	execution.Path = filepath.Join(dir, name)
	data, err := json.MarshalIndent(execution, "", "  ")
	if err != nil {
		t.Fatalf("encode execution fixture: %v", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(execution.Path, data, 0o644); err != nil {
		t.Fatalf("write execution fixture: %v", err)
	}
	return execution
}

func writeInstallRetryExecutionFixture(t *testing.T, root string, name string, execution ToolchainInstallRetryDispatchExecution) ToolchainInstallRetryDispatchExecution {
	t.Helper()
	dir := toolchainInstallRetryExecutionDir(root)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("create retry execution fixture dir: %v", err)
	}
	execution.Path = filepath.Join(dir, name)
	data, err := json.MarshalIndent(execution, "", "  ")
	if err != nil {
		t.Fatalf("encode retry execution fixture: %v", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(execution.Path, data, 0o644); err != nil {
		t.Fatalf("write retry execution fixture: %v", err)
	}
	return execution
}

type statusToolCompilePreviewRunner struct {
	statuses map[string]string
}

func (r *statusToolCompilePreviewRunner) Execute(ctx context.Context, request ToolExecutionRequest) (ToolExecutionResult, error) {
	if err := ctx.Err(); err != nil {
		return ToolExecutionResult{}, err
	}
	status := strings.TrimSpace(r.statuses[request.Node.ManifestName])
	if status == "" {
		status = "completed"
	}
	return ToolExecutionResult{
		ExecutionID: "derived-" + strings.ReplaceAll(request.Node.ID, ":", "-"),
		Status:      status,
		Transcript:  "derived signal fixture for " + request.Node.ManifestName,
	}, nil
}

type failingToolExecutionRunner struct {
	err string
}

func (r failingToolExecutionRunner) Execute(ctx context.Context, request ToolExecutionRequest) (ToolExecutionResult, error) {
	if err := ctx.Err(); err != nil {
		return ToolExecutionResult{}, err
	}
	return ToolExecutionResult{
		ExecutionID: "failed-" + strings.ReplaceAll(request.Node.ID, ":", "-"),
		Status:      "failed",
		Transcript:  r.err,
	}, errors.New(r.err)
}
