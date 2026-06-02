package compiler

import (
	"github.com/ouroboros/ouroboros/internal/evidence"
	"github.com/ouroboros/ouroboros/internal/frontier"
	"github.com/ouroboros/ouroboros/internal/toolchain"
)

type SemanticAction string

const (
	ActionDiscoverAssets     SemanticAction = "discover_assets"
	ActionFingerprintSurface SemanticAction = "fingerprint_surface"
	ActionEnumeratePaths     SemanticAction = "enumerate_paths"
	ActionGenerateCandidates SemanticAction = "generate_candidates"
	ActionVerifyCandidate    SemanticAction = "verify_candidate"
	ActionCollectEvidence    SemanticAction = "collect_evidence"
	ActionProduceReport      SemanticAction = "produce_report"
)

type EvidenceSlice struct {
	Nodes []evidence.Node
	Edges []evidence.Edge
}

type PlanGraph struct {
	Action   SemanticAction
	Inputs   EvidenceSlice
	Nodes    []PlanNode
	Edges    []PlanEdge
	Warnings []string
}

type PlanNode struct {
	ID               string
	Action           SemanticAction
	Capability       string
	ManifestName     string
	ToolID           string `json:"tool_id,omitempty"`
	AdapterID        string `json:"adapter_id,omitempty"`
	Operation        string `json:"operation,omitempty"`
	ParserID         string `json:"parser_id,omitempty"`
	TargetKind       string `json:"target_kind,omitempty"`
	DependencyName   string `json:"dependency_name,omitempty"`
	DependencyKind   string `json:"dependency_kind,omitempty"`
	Runner           string
	RunCommand       []string
	RunTimeoutMS     int
	Parser           string
	Dependencies     []toolchain.Dependency
	InputNodeIDs     []string
	ExpectedEvidence []evidence.NodeType
	SelectionScore   float64
	SelectionSignals []string
}

type PlanEdge struct {
	From string
	To   string
	Type string
}

type CompileBudget struct {
	MaxPlanNodes int
}

type DomainPriority struct {
	Domain frontier.Domain
	Weight float64
}

type CandidateFrontierScore struct {
	ManifestName string
	Capability   string
	Path         frontier.CandidatePath
}

type CandidateHistory struct {
	ManifestName string
	Capability   string
	Successes    int
	Failures     int
}
