package compiler

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/ouroboros/ouroboros/internal/evidence"
	"github.com/ouroboros/ouroboros/internal/frontier"
	"github.com/ouroboros/ouroboros/internal/toolchain"
)

type Compiler struct{}

type CompileRequest struct {
	Action           SemanticAction
	Evidence         EvidenceSlice
	Manifests        []toolchain.Manifest
	Budget           CompileBudget
	DomainPriorities []DomainPriority
	FrontierScores   []CandidateFrontierScore
	HistoricalStats  []CandidateHistory
}

func NewCompiler() Compiler {
	return Compiler{}
}

func (Compiler) Compile(request CompileRequest) (PlanGraph, error) {
	capabilities, ok := capabilitiesForAction(request.Action)
	if !ok {
		return PlanGraph{}, fmt.Errorf("unknown semantic action: %s", request.Action)
	}
	if len(request.Manifests) == 0 {
		return PlanGraph{}, errors.New("toolchain manifests are required")
	}

	inputNodeIDs := evidenceInputNodeIDs(request.Evidence.Nodes)
	candidates, err := matchCapabilities(request.Manifests, capabilities)
	if err != nil {
		return PlanGraph{}, err
	}
	if len(candidates) == 0 {
		return PlanGraph{}, fmt.Errorf("no available capability for semantic action %s", request.Action)
	}
	candidates = rankCapabilityCandidates(candidates, request)

	warnings := []string(nil)
	if request.Budget.MaxPlanNodes > 0 && len(candidates) > request.Budget.MaxPlanNodes {
		warnings = append(warnings, fmt.Sprintf("budget max_plan_nodes=%d truncated %d candidates", request.Budget.MaxPlanNodes, len(candidates)))
		candidates = candidates[:request.Budget.MaxPlanNodes]
	}

	nodes := make([]PlanNode, 0, len(candidates))
	expectedEvidence := expectedEvidenceForAction(request.Action)
	for i, candidate := range candidates {
		nodes = append(nodes, PlanNode{
			ID:               fmt.Sprintf("%s:%02d", request.Action, i+1),
			Action:           request.Action,
			Capability:       candidate.capability,
			ManifestName:     candidate.manifest.Name,
			Runner:           candidate.manifest.Runner,
			RunCommand:       append([]string(nil), candidate.manifest.RunCommand...),
			RunTimeoutMS:     candidate.manifest.RunTimeoutMS,
			Parser:           candidate.manifest.Parser,
			Dependencies:     append([]toolchain.Dependency(nil), candidate.manifest.Dependencies...),
			InputNodeIDs:     append([]string(nil), inputNodeIDs...),
			ExpectedEvidence: append([]evidence.NodeType(nil), expectedEvidence...),
			SelectionScore:   candidate.selectionScore,
			SelectionSignals: append([]string(nil), candidate.selectionSignals...),
		})
	}

	return PlanGraph{
		Action:   request.Action,
		Inputs:   request.Evidence,
		Nodes:    nodes,
		Warnings: warnings,
	}, nil
}

type capabilityCandidate struct {
	priority         int
	capability       string
	manifest         toolchain.Manifest
	selectionScore   float64
	selectionSignals []string
}

func matchCapabilities(manifests []toolchain.Manifest, wanted []string) ([]capabilityCandidate, error) {
	wantedPriority := make(map[string]int, len(wanted))
	for priority, capability := range wanted {
		wantedPriority[capability] = priority
	}

	var candidates []capabilityCandidate
	for _, manifest := range manifests {
		if err := toolchain.ValidateManifest(manifest); err != nil {
			return nil, fmt.Errorf("validate manifest %s: %w", manifest.Name, err)
		}
		seen := map[string]bool{}
		for _, capability := range manifest.Capabilities {
			normalized := strings.TrimSpace(capability)
			priority, ok := wantedPriority[normalized]
			if !ok || seen[normalized] {
				continue
			}
			seen[normalized] = true
			candidates = append(candidates, capabilityCandidate{
				priority:   priority,
				capability: normalized,
				manifest:   manifest,
			})
		}
	}

	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].priority != candidates[j].priority {
			return candidates[i].priority < candidates[j].priority
		}
		if candidates[i].manifest.Name != candidates[j].manifest.Name {
			return candidates[i].manifest.Name < candidates[j].manifest.Name
		}
		return candidates[i].capability < candidates[j].capability
	})
	return candidates, nil
}

func rankCapabilityCandidates(candidates []capabilityCandidate, request CompileRequest) []capabilityCandidate {
	ranked := make([]capabilityCandidate, len(candidates))
	copy(ranked, candidates)

	domainWeights := domainPriorityWeights(request.DomainPriorities)
	frontierScores := frontierScoreByCandidate(request.FrontierScores)
	historyStats := historyByCandidate(request.HistoricalStats)
	for i := range ranked {
		candidate := &ranked[i]
		key := candidateKey(candidate.manifest.Name, candidate.capability)
		if domain, ok := domainForCapability(candidate.capability); ok {
			if weight, ok := domainWeights[domain]; ok {
				candidate.selectionScore += weight
				candidate.selectionSignals = append(candidate.selectionSignals, "domain:"+string(domain)+"="+formatScore(weight))
			}
		}
		if signal, ok := frontierScores[key]; ok {
			score := frontier.Score(signal.Path)
			candidate.selectionScore += score
			candidate.selectionSignals = append(candidate.selectionSignals, "frontier="+formatScore(score))
		}
		if history, ok := historyStats[key]; ok {
			score := float64(history.Successes - history.Failures)
			candidate.selectionScore += score
			candidate.selectionSignals = append(candidate.selectionSignals, fmt.Sprintf("history:successes=%d failures=%d", history.Successes, history.Failures))
		}
	}

	sort.SliceStable(ranked, func(i, j int) bool {
		if ranked[i].selectionScore != ranked[j].selectionScore {
			return ranked[i].selectionScore > ranked[j].selectionScore
		}
		if ranked[i].priority != ranked[j].priority {
			return ranked[i].priority < ranked[j].priority
		}
		if ranked[i].manifest.Name != ranked[j].manifest.Name {
			return ranked[i].manifest.Name < ranked[j].manifest.Name
		}
		return ranked[i].capability < ranked[j].capability
	})
	return ranked
}

func candidateKey(manifestName, capability string) string {
	return strings.TrimSpace(manifestName) + "\x00" + strings.TrimSpace(capability)
}

func domainPriorityWeights(priorities []DomainPriority) map[frontier.Domain]float64 {
	weights := make(map[frontier.Domain]float64, len(priorities))
	for _, priority := range priorities {
		if strings.TrimSpace(string(priority.Domain)) == "" {
			continue
		}
		weights[priority.Domain] += priority.Weight
	}
	return weights
}

func frontierScoreByCandidate(scores []CandidateFrontierScore) map[string]CandidateFrontierScore {
	byCandidate := make(map[string]CandidateFrontierScore, len(scores))
	for _, score := range scores {
		key := candidateKey(score.ManifestName, score.Capability)
		if strings.TrimSpace(key) == "\x00" {
			continue
		}
		byCandidate[key] = score
	}
	return byCandidate
}

func historyByCandidate(stats []CandidateHistory) map[string]CandidateHistory {
	byCandidate := make(map[string]CandidateHistory, len(stats))
	for _, stat := range stats {
		key := candidateKey(stat.ManifestName, stat.Capability)
		if strings.TrimSpace(key) == "\x00" {
			continue
		}
		current := byCandidate[key]
		current.ManifestName = strings.TrimSpace(stat.ManifestName)
		current.Capability = strings.TrimSpace(stat.Capability)
		current.Successes += stat.Successes
		current.Failures += stat.Failures
		byCandidate[key] = current
	}
	return byCandidate
}

func domainForCapability(capability string) (frontier.Domain, bool) {
	switch {
	case strings.HasPrefix(capability, "web."):
		return frontier.DomainWebAPI, true
	case strings.HasPrefix(capability, "infra."):
		return frontier.DomainInfra, true
	case strings.HasPrefix(capability, "ctf."):
		return frontier.DomainCTF, true
	default:
		return "", false
	}
}

func formatScore(score float64) string {
	return strconv.FormatFloat(score, 'g', -1, 64)
}

func evidenceInputNodeIDs(nodes []evidence.Node) []string {
	ids := make([]string, 0, len(nodes))
	for _, node := range nodes {
		if strings.TrimSpace(node.ID) == "" {
			continue
		}
		ids = append(ids, node.ID)
	}
	sort.Strings(ids)
	return ids
}

func capabilitiesForAction(action SemanticAction) ([]string, bool) {
	capabilities, ok := semanticCapabilities[action]
	return capabilities, ok
}

var semanticCapabilities = map[SemanticAction][]string{
	ActionDiscoverAssets: {
		"web.discovery",
		"infra.host_discovery",
		"ctf.challenge_classification",
	},
	ActionFingerprintSurface: {
		"web.fingerprint",
		"infra.service_enumeration",
		"ctf.artifact_analysis",
		"web.discovery",
	},
	ActionEnumeratePaths: {
		"web.route_enumeration",
		"infra.path_hypothesis",
		"ctf.hypothesis_generation",
	},
	ActionGenerateCandidates: {
		"web.candidate_generation",
		"infra.path_hypothesis",
		"ctf.hypothesis_generation",
	},
	ActionVerifyCandidate: {
		"web.candidate_verification",
		"infra.candidate_verification",
		"ctf.candidate_verification",
	},
	ActionCollectEvidence: {
		"evidence.collection",
		"ctf.result_proof",
	},
	ActionProduceReport: {
		"report.produce",
		"ctf.result_proof",
		"evidence.collection",
	},
}

func expectedEvidenceForAction(action SemanticAction) []evidence.NodeType {
	switch action {
	case ActionDiscoverAssets, ActionFingerprintSurface, ActionEnumeratePaths:
		return []evidence.NodeType{evidence.NodeSurface}
	case ActionGenerateCandidates:
		return []evidence.NodeType{evidence.NodeHypothesis}
	case ActionVerifyCandidate:
		return []evidence.NodeType{evidence.NodeFinding}
	case ActionCollectEvidence, ActionProduceReport:
		return []evidence.NodeType{evidence.NodeArtifact}
	default:
		return nil
	}
}
