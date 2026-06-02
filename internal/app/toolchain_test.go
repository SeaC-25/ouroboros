package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
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
				Name:           "httpx",
				Kind:           "binary",
				Optional:       true,
				HealthCommand:  []string{"httpx", "-version"},
				InstallHint:    "install managed httpx",
				InstallCommand: []string{"ouro-toolchain", "install", "httpx"},
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
	if preview.Nodes[0].ManifestName != "infra-pack" || preview.Nodes[0].SelectionScore <= preview.Nodes[1].SelectionScore {
		t.Fatalf("expected infra to rank first by scoring signals, got %#v", preview.Nodes)
	}
	if !containsString(preview.Nodes[0].SelectionSignals, "domain:infra_internal=5") {
		t.Fatalf("expected domain scoring signal, got %#v", preview.Nodes[0].SelectionSignals)
	}
	if len(preview.Warnings) != 1 || !strings.Contains(preview.Warnings[0], "budget max_plan_nodes=2 truncated 3 candidates") {
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
	if node.ManifestName != "infra-pack" || node.Capability != "infra.host_discovery" {
		t.Fatalf("expected persisted signals to rank infra first, got %#v", node)
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
	if len(preview.Warnings) != 1 || !strings.Contains(preview.Warnings[0], "budget max_plan_nodes=1 truncated 3 candidates") {
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
	if dryRun.Summary.ToolExecutions != 3 || dryRun.Summary.Successes != 1 || dryRun.Summary.Failures != 1 || dryRun.Summary.Ignored != 1 {
		t.Fatalf("unexpected derivation summary: %#v", dryRun.Summary)
	}
	if len(dryRun.Signals.HistoricalStats) != 2 {
		t.Fatalf("expected two historical stats, got %#v", dryRun.Signals.HistoricalStats)
	}
	history := map[string]ToolCompilePreviewSignalHistory{}
	for _, stat := range dryRun.Signals.HistoricalStats {
		history[stat.ManifestName+"/"+stat.Capability] = stat
	}
	if history["infra-pack/infra.host_discovery"].Successes != 1 {
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

func TestExecuteToolchainInstallDispatchRequiresExplicitExecute(t *testing.T) {
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

	_, err = ExecuteToolchainInstallDispatch(root, dispatch.Path, ToolchainInstallDispatchExecutionOptions{})
	if err == nil || !strings.Contains(err.Error(), "requires explicit execute") {
		t.Fatalf("expected explicit execute error, got %v", err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("missing execute gate must not execute install command, marker stat err=%v", err)
	}
	paths, err := filepath.Glob(filepath.Join(root, "toolchain", "install-executions", "*.json"))
	if err != nil {
		t.Fatalf("glob install executions: %v", err)
	}
	if len(paths) != 0 {
		t.Fatalf("expected no execution artifact without explicit execute, got %#v", paths)
	}
}

func TestExecuteToolchainInstallDispatchRequiresSafetyAuthorizationForDisabledBoundary(t *testing.T) {
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

func TestExecuteToolchainInstallDispatchRevalidatesApprovedReadyRequests(t *testing.T) {
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

	_, err = ExecuteToolchainInstallDispatch(root, dispatch.Path, ToolchainInstallDispatchExecutionOptions{Execute: true})
	if err == nil || !strings.Contains(err.Error(), "approved") {
		t.Fatalf("expected approved revalidation error, got %v", err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("tampered ready request must not execute, marker stat err=%v", err)
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

func TestCreateToolchainInstallOrchestrationWritesMetadataOnlyAuditWithoutExecution(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
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
	if orchestration.Status != "dry-run" || orchestration.Mode != "metadata-only" {
		t.Fatalf("unexpected orchestration mode: %#v", orchestration)
	}
	if orchestration.DispatchReady != 1 || orchestration.DispatchSkipped != 0 || orchestration.RetryReady != 0 {
		t.Fatalf("unexpected orchestration counts: %#v", orchestration)
	}
	if orchestration.DispatchPath != "" || orchestration.ExecutionPath != "" || orchestration.RetryDispatchPath != "" {
		t.Fatalf("metadata-only orchestration must not create dispatch/execution paths: %#v", orchestration)
	}
	if !strings.Contains(orchestration.Path, filepath.Join("toolchain", "install-orchestrations")) {
		t.Fatalf("expected orchestration audit path under install-orchestrations, got %s", orchestration.Path)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("metadata-only orchestration must not execute installer command, marker stat err=%v", err)
	}
	dispatches, err := filepath.Glob(filepath.Join(root, "toolchain", "install-dispatches", "*.json"))
	if err != nil {
		t.Fatalf("glob dispatches: %v", err)
	}
	if len(dispatches) != 0 {
		t.Fatalf("metadata-only orchestration must not create dispatch artifacts, got %#v", dispatches)
	}
	executions, err := filepath.Glob(filepath.Join(root, "toolchain", "install-executions", "*.json"))
	if err != nil {
		t.Fatalf("glob executions: %v", err)
	}
	if len(executions) != 0 {
		t.Fatalf("metadata-only orchestration must not create execution artifacts, got %#v", executions)
	}
	data, err := os.ReadFile(orchestration.Path)
	if err != nil {
		t.Fatalf("read orchestration artifact: %v", err)
	}
	var decoded ToolchainInstallOrchestration
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("decode orchestration artifact: %v", err)
	}
	if decoded.Status != "dry-run" || decoded.DispatchReady != 1 {
		t.Fatalf("unexpected persisted orchestration: %#v", decoded)
	}
}

func TestCreateToolchainInstallOrchestrationExecuteRunsApprovedDispatchAndWritesAudit(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
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
