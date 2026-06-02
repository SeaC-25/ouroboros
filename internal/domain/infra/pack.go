package infra

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
	return "infra-internal"
}

func (Pack) Domain() frontier.Domain {
	return frontier.DomainInfra
}

func (Pack) Capabilities() []domain.Capability {
	return []domain.Capability{
		{
			Name:             "infra.host_discovery",
			Action:           compiler.ActionDiscoverAssets,
			Description:      "discover live hosts and initial network targets in CIDR scope",
			ExpectedEvidence: []evidence.NodeType{evidence.NodeTarget},
		},
		{
			Name:             "infra.service_enumeration",
			Action:           compiler.ActionFingerprintSurface,
			Description:      "enumerate exposed ports, protocols, and service banners",
			ExpectedEvidence: []evidence.NodeType{evidence.NodeSurface},
		},
		{
			Name:             "infra.identity_graph",
			Action:           compiler.ActionFingerprintSurface,
			Description:      "observe identities, accounts, hosts, and trust hints",
			ExpectedEvidence: []evidence.NodeType{evidence.NodeIdentity},
		},
		{
			Name:             "infra.path_hypothesis",
			Action:           compiler.ActionGenerateCandidates,
			Description:      "generate internal movement and privilege path hypotheses",
			ExpectedEvidence: []evidence.NodeType{evidence.NodeHypothesis},
		},
		{
			Name:             "evidence.collection",
			Action:           compiler.ActionCollectEvidence,
			Description:      "collect service banners, protocol output, proofs, and path evidence",
			ExpectedEvidence: []evidence.NodeType{evidence.NodeArtifact},
		},
	}
}

func (Pack) CoverageDimensions() []frontier.Dimension {
	return []frontier.Dimension{
		frontier.DimensionHosts,
		frontier.DimensionServices,
		frontier.DimensionIdentities,
	}
}

func (Pack) Parsers() []compiler.ParserSpec {
	return []compiler.ParserSpec{
		{Name: "host_observation_parser", FactKind: "target", StatementField: "host", ConfidenceField: "confidence", RequiredFields: []string{"host"}},
		{Name: "service_observation_parser", FactKind: "surface", StatementField: "service", ConfidenceField: "confidence", RequiredFields: []string{"service"}},
		{Name: "identity_observation_parser", FactKind: "identity", StatementField: "identity", ConfidenceField: "confidence", RequiredFields: []string{"identity"}},
		{Name: "infra_evidence_parser", FactKind: "artifact", StatementField: "evidence_ref", ConfidenceField: "confidence", RequiredFields: []string{"evidence_ref"}},
	}
}

func (Pack) StrategyPatterns() []patterns.Pattern {
	return []patterns.Pattern{
		{Name: "explore-plan-execute", Version: "0.1.0"},
		{Name: "ant-colony", Version: "0.1.0"},
	}
}
