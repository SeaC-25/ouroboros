package compiler

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/ouroboros/ouroboros/internal/evidence"
	"github.com/ouroboros/ouroboros/internal/frontier"
	"github.com/ouroboros/ouroboros/internal/toolchain"
)

func TestSemanticActionsAreDeclared(t *testing.T) {
	actions := []SemanticAction{
		ActionDiscoverAssets,
		ActionFingerprintSurface,
		ActionEnumeratePaths,
		ActionGenerateCandidates,
		ActionVerifyCandidate,
		ActionCollectEvidence,
		ActionProduceReport,
	}

	seen := map[SemanticAction]bool{}
	for _, action := range actions {
		if strings.TrimSpace(string(action)) == "" {
			t.Fatalf("semantic action must not be empty")
		}
		if seen[action] {
			t.Fatalf("duplicate semantic action: %s", action)
		}
		seen[action] = true
	}
}

func TestCompileChoosesCapabilitiesNotHardCodedToolNames(t *testing.T) {
	plan, err := NewCompiler().Compile(CompileRequest{
		Action: ActionDiscoverAssets,
		Evidence: EvidenceSlice{
			Nodes: []evidence.Node{
				{
					ID:         "target-1",
					Type:       evidence.NodeTarget,
					Label:      "https://app.example",
					SourceKind: "target",
				},
			},
		},
		Manifests: []toolchain.Manifest{
			{
				Name:         "definitely-not-a-known-tool",
				Version:      "2026.05.29",
				Capabilities: []string{"web.discovery"},
				Runner:       "native",
				Parser:       "web_discovery_parser",
				Healthcheck:  "built_in",
			},
			{
				Name:         "famous-scanner-name",
				Version:      "1.0.0",
				Capabilities: []string{"infra.service_enumeration"},
				Runner:       "native",
				Parser:       "infra_parser",
				Healthcheck:  "built_in",
			},
		},
	})
	if err != nil {
		t.Fatalf("compile plan: %v", err)
	}

	if plan.Action != ActionDiscoverAssets {
		t.Fatalf("unexpected action: %s", plan.Action)
	}
	if len(plan.Nodes) != 1 {
		t.Fatalf("expected one plan node, got %d", len(plan.Nodes))
	}
	node := plan.Nodes[0]
	if node.ManifestName != "definitely-not-a-known-tool" {
		t.Fatalf("expected capability-bearing manifest, got %s", node.ManifestName)
	}
	if node.Capability != "web.discovery" {
		t.Fatalf("expected web.discovery capability, got %s", node.Capability)
	}
	if node.Runner != "native" || node.Parser != "web_discovery_parser" {
		t.Fatalf("expected runner/parser copied from manifest, got %s/%s", node.Runner, node.Parser)
	}
	if len(node.InputNodeIDs) != 1 || node.InputNodeIDs[0] != "target-1" {
		t.Fatalf("expected target evidence input, got %#v", node.InputNodeIDs)
	}
	if len(node.ExpectedEvidence) == 0 || node.ExpectedEvidence[0] != evidence.NodeSurface {
		t.Fatalf("expected surface evidence output, got %#v", node.ExpectedEvidence)
	}
}

func TestCompileEmitsToolLevelPlanNodesFromBuiltInCatalog(t *testing.T) {
	manifests, err := toolchain.LoadManifests(filepath.Join("..", "..", "toolchains"))
	if err != nil {
		t.Fatalf("load root manifests: %v", err)
	}

	plan, err := NewCompiler().Compile(CompileRequest{
		Action:    ActionDiscoverAssets,
		Manifests: manifests,
	})
	if err != nil {
		t.Fatalf("compile plan: %v", err)
	}
	byTool := planNodesByToolID(plan.Nodes)

	for _, expected := range []struct {
		toolID         string
		manifestName   string
		capability     string
		operation      string
		parser         string
		targetKind     string
		dependencyName string
		dependencyKind string
	}{
		{toolID: "httpx", manifestName: "web-api-pack", capability: "web.discovery", operation: "http_probe", parser: "httpx_jsonl", targetKind: "url", dependencyName: "httpx", dependencyKind: "binary"},
		{toolID: "subfinder", manifestName: "web-api-pack", capability: "web.discovery", operation: "subdomain_discovery", parser: "subfinder_jsonl", targetKind: "domain", dependencyName: "subfinder", dependencyKind: "binary"},
		{toolID: "nmap", manifestName: "infra-pack", capability: "infra.host_discovery", operation: "service_fingerprint", parser: "nmap_xml", targetKind: "host", dependencyName: "nmap", dependencyKind: "binary"},
		{toolID: "naabu", manifestName: "infra-pack", capability: "infra.host_discovery", operation: "port_scan", parser: "naabu_jsonl", targetKind: "host", dependencyName: "naabu", dependencyKind: "binary"},
	} {
		node, ok := byTool[expected.toolID]
		if !ok {
			t.Fatalf("expected tool-level node for %s, got %#v", expected.toolID, plan.Nodes)
		}
		if node.ManifestName != expected.manifestName || node.Capability != expected.capability {
			t.Fatalf("unexpected manifest/capability for %s: %#v", expected.toolID, node)
		}
		if node.AdapterID != "managed_binary" || node.Operation != expected.operation {
			t.Fatalf("unexpected adapter operation for %s: %#v", expected.toolID, node)
		}
		if node.Parser != expected.parser || node.ParserID != expected.parser {
			t.Fatalf("unexpected parser contract for %s: %#v", expected.toolID, node)
		}
		if node.TargetKind != expected.targetKind || node.DependencyName != expected.dependencyName || node.DependencyKind != expected.dependencyKind {
			t.Fatalf("unexpected target/dependency contract for %s: %#v", expected.toolID, node)
		}
		if len(node.Dependencies) != 1 || node.Dependencies[0].Name != expected.dependencyName {
			t.Fatalf("expected dependency subset for %s, got %#v", expected.toolID, node.Dependencies)
		}
	}
}

func TestCompileIncludesRuleBundleDependencyForToolLevelTemplateScan(t *testing.T) {
	manifests, err := toolchain.LoadManifests(filepath.Join("..", "..", "toolchains"))
	if err != nil {
		t.Fatalf("load root manifests: %v", err)
	}

	plan, err := NewCompiler().Compile(CompileRequest{
		Action:    ActionGenerateCandidates,
		Manifests: manifests,
	})
	if err != nil {
		t.Fatalf("compile plan: %v", err)
	}
	nuclei, ok := planNodesByToolID(plan.Nodes)["nuclei"]
	if !ok {
		t.Fatalf("expected nuclei tool-level node, got %#v", plan.Nodes)
	}
	if nuclei.Operation != "template_scan" || nuclei.ParserID != "nuclei_jsonl" {
		t.Fatalf("expected nuclei template scan contract, got %#v", nuclei)
	}
	if !dependencyNamesEqual(nuclei.Dependencies, []string{"nuclei", "nuclei-templates"}) {
		t.Fatalf("expected nuclei binary plus templates rule bundle, got %#v", nuclei.Dependencies)
	}
}

func TestCompileKeepsManifestLevelCapabilityWhenSamePackHasToolLevelCandidate(t *testing.T) {
	manifests, err := toolchain.LoadManifests(filepath.Join("..", "..", "toolchains"))
	if err != nil {
		t.Fatalf("load root manifests: %v", err)
	}

	plan, err := NewCompiler().Compile(CompileRequest{
		Action:    ActionProduceReport,
		Manifests: manifests,
	})
	if err != nil {
		t.Fatalf("compile plan: %v", err)
	}

	foundCTFProof := false
	for _, node := range plan.Nodes {
		if node.ManifestName == "ctf-pack" && node.Capability == "ctf.result_proof" && node.ToolID == "" {
			foundCTFProof = true
			break
		}
	}
	if !foundCTFProof {
		t.Fatalf("expected ctf.result_proof manifest-level node alongside tool-level candidates, got %#v", plan.Nodes)
	}
}

func TestCompileRejectsActionWithoutAvailableCapability(t *testing.T) {
	_, err := NewCompiler().Compile(CompileRequest{
		Action: ActionVerifyCandidate,
		Manifests: []toolchain.Manifest{
			{
				Name:         "discovery-only-pack",
				Version:      "0.1.0",
				Capabilities: []string{"web.discovery"},
				Runner:       "native",
				Parser:       "web_parser",
				Healthcheck:  "built_in",
			},
		},
	})
	if err == nil {
		t.Fatalf("expected missing capability to fail")
	}
	if !strings.Contains(err.Error(), "no available capability") {
		t.Fatalf("expected missing capability error, got %v", err)
	}
}

func TestCompileCarriesManifestRunCommand(t *testing.T) {
	plan, err := NewCompiler().Compile(CompileRequest{
		Action: ActionDiscoverAssets,
		Manifests: []toolchain.Manifest{
			{
				Name:         "web-command-pack",
				Version:      "0.1.0",
				Capabilities: []string{"web.discovery"},
				Runner:       "command",
				RunCommand:   []string{"printf", "surface\n"},
				RunTimeoutMS: 750,
				Parser:       "web_parser",
				Healthcheck:  "built_in",
			},
		},
	})
	if err != nil {
		t.Fatalf("compile plan: %v", err)
	}
	if len(plan.Nodes) != 1 {
		t.Fatalf("expected one node, got %#v", plan.Nodes)
	}
	if strings.Join(plan.Nodes[0].RunCommand, "\x00") != strings.Join([]string{"printf", "surface\n"}, "\x00") {
		t.Fatalf("expected run command copied from manifest, got %#v", plan.Nodes[0].RunCommand)
	}
	if plan.Nodes[0].RunTimeoutMS != 750 {
		t.Fatalf("expected run timeout copied from manifest, got %d", plan.Nodes[0].RunTimeoutMS)
	}
}

func TestCompileRanksCandidatesByFrontierDomainAndHistorySignals(t *testing.T) {
	plan, err := NewCompiler().Compile(CompileRequest{
		Action: ActionDiscoverAssets,
		Manifests: []toolchain.Manifest{
			{
				Name:         "web-pack",
				Version:      "0.1.0",
				Capabilities: []string{"web.discovery"},
				Runner:       "native",
				Parser:       "web_parser",
				Healthcheck:  "built_in",
			},
			{
				Name:         "infra-pack",
				Version:      "0.1.0",
				Capabilities: []string{"infra.host_discovery"},
				Runner:       "native",
				Parser:       "infra_parser",
				Healthcheck:  "built_in",
			},
			{
				Name:         "ctf-pack",
				Version:      "0.1.0",
				Capabilities: []string{"ctf.challenge_classification"},
				Runner:       "native",
				Parser:       "ctf_parser",
				Healthcheck:  "built_in",
			},
		},
		DomainPriorities: []DomainPriority{
			{Domain: frontier.DomainInfra, Weight: 5},
			{Domain: frontier.DomainCTF, Weight: 1},
		},
		FrontierScores: []CandidateFrontierScore{
			{
				ManifestName: "infra-pack",
				Capability:   "infra.host_discovery",
				Path: frontier.CandidatePath{
					MissionValue:       3,
					UnlockPotential:    2,
					EvidenceConfidence: 1,
					Novelty:            1,
					Cost:               1,
				},
			},
			{
				ManifestName: "ctf-pack",
				Capability:   "ctf.challenge_classification",
				Path: frontier.CandidatePath{
					MissionValue:    1,
					UnlockPotential: 1,
					Cost:            1,
				},
			},
		},
		HistoricalStats: []CandidateHistory{
			{ManifestName: "infra-pack", Capability: "infra.host_discovery", Successes: 4, Failures: 1},
			{ManifestName: "web-pack", Capability: "web.discovery", Successes: 0, Failures: 3},
		},
		Budget: CompileBudget{MaxPlanNodes: 2},
	})
	if err != nil {
		t.Fatalf("compile plan: %v", err)
	}

	if len(plan.Nodes) != 2 {
		t.Fatalf("expected budget to keep two nodes, got %#v", plan.Nodes)
	}
	if plan.Nodes[0].ManifestName != "infra-pack" || plan.Nodes[0].Capability != "infra.host_discovery" {
		t.Fatalf("expected frontier/domain/history signals to rank infra first, got %#v", plan.Nodes[0])
	}
	if plan.Nodes[1].ManifestName != "ctf-pack" || plan.Nodes[1].Capability != "ctf.challenge_classification" {
		t.Fatalf("expected ctf to remain after budget truncation, got %#v", plan.Nodes[1])
	}
	if plan.Nodes[0].SelectionScore <= plan.Nodes[1].SelectionScore {
		t.Fatalf("expected first node to carry a higher selection score, got %f <= %f", plan.Nodes[0].SelectionScore, plan.Nodes[1].SelectionScore)
	}
	if !containsString(plan.Nodes[0].SelectionSignals, "domain:infra_internal=5") {
		t.Fatalf("expected domain signal on first node, got %#v", plan.Nodes[0].SelectionSignals)
	}
	if !containsString(plan.Nodes[0].SelectionSignals, "history:successes=4 failures=1") {
		t.Fatalf("expected history signal on first node, got %#v", plan.Nodes[0].SelectionSignals)
	}
	if len(plan.Warnings) != 1 || !strings.Contains(plan.Warnings[0], "budget max_plan_nodes=2 truncated 3 candidates") {
		t.Fatalf("expected budget truncation warning, got %#v", plan.Warnings)
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func planNodesByToolID(nodes []PlanNode) map[string]PlanNode {
	byTool := map[string]PlanNode{}
	for _, node := range nodes {
		if strings.TrimSpace(node.ToolID) == "" {
			continue
		}
		byTool[node.ToolID] = node
	}
	return byTool
}

func dependencyNamesEqual(dependencies []toolchain.Dependency, want []string) bool {
	if len(dependencies) != len(want) {
		return false
	}
	byName := map[string]bool{}
	for _, dependency := range dependencies {
		byName[dependency.Name] = true
	}
	for _, name := range want {
		if !byName[name] {
			return false
		}
	}
	return true
}
