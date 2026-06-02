package ctf

import (
	"github.com/ouroboros/ouroboros/internal/compiler"
	"github.com/ouroboros/ouroboros/internal/domain"
	"github.com/ouroboros/ouroboros/internal/evidence"
	"github.com/ouroboros/ouroboros/internal/frontier"
	"github.com/ouroboros/ouroboros/internal/patterns"
)

type Pack struct{}

func NewPack() Pack {
	return Pack{}
}

func (Pack) Name() string {
	return "ctf-lab"
}

func (Pack) Domain() frontier.Domain {
	return frontier.DomainCTF
}

func (Pack) Capabilities() []domain.Capability {
	return []domain.Capability{
		{
			Name:             "ctf.challenge_classification",
			Action:           compiler.ActionFingerprintSurface,
			Description:      "classify a challenge by category, constraints, and likely solve paths",
			ExpectedEvidence: []evidence.NodeType{evidence.NodeHypothesis},
		},
		{
			Name:             "ctf.artifact_analysis",
			Action:           compiler.ActionCollectEvidence,
			Description:      "inspect challenge artifacts, prompts, source snippets, binaries, and provided files",
			ExpectedEvidence: []evidence.NodeType{evidence.NodeArtifact},
		},
		{
			Name:             "ctf.fast_hypothesis",
			Action:           compiler.ActionGenerateCandidates,
			Description:      "generate fast, high-signal solve hypotheses before broad enumeration",
			ExpectedEvidence: []evidence.NodeType{evidence.NodeHypothesis},
		},
		{
			Name:             "ctf.candidate_verification",
			Action:           compiler.ActionVerifyCandidate,
			Description:      "verify candidate solve paths against observed challenge behavior",
			ExpectedEvidence: []evidence.NodeType{evidence.NodeFinding},
		},
		{
			Name:             "ctf.result_proof",
			Action:           compiler.ActionProduceReport,
			Description:      "produce a compact proof of result with the evidence required to reproduce it",
			ExpectedEvidence: []evidence.NodeType{evidence.NodeFinding, evidence.NodeArtifact},
		},
	}
}

func (Pack) CoverageDimensions() []frontier.Dimension {
	return []frontier.Dimension{
		frontier.DimensionHypotheses,
		frontier.DimensionArtifacts,
		frontier.DimensionProofs,
	}
}

func (Pack) Parsers() []compiler.ParserSpec {
	return []compiler.ParserSpec{
		{Name: "challenge_classification_parser", FactKind: "hypothesis", StatementField: "classification", ConfidenceField: "confidence", RequiredFields: []string{"classification"}},
		{Name: "artifact_analysis_parser", FactKind: "artifact", StatementField: "artifact_ref", ConfidenceField: "confidence", RequiredFields: []string{"artifact_ref"}},
		{Name: "candidate_verification_parser", FactKind: "finding", StatementField: "candidate_result", ConfidenceField: "confidence", RequiredFields: []string{"candidate_result"}},
		{Name: "ctf_proof_parser", FactKind: "finding", StatementField: "proof", ConfidenceField: "confidence", RequiredFields: []string{"proof"}},
	}
}

func (Pack) StrategyPatterns() []patterns.Pattern {
	return []patterns.Pattern{
		{Name: "ralphloop", Version: "0.1.0"},
		{Name: "explore-plan-execute", Version: "0.1.0"},
	}
}
