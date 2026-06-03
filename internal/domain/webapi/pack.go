package webapi

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
	return "web-api"
}

func (Pack) Domain() frontier.Domain {
	return frontier.DomainWebAPI
}

func (Pack) Capabilities() []domain.Capability {
	return []domain.Capability{
		{
			Name:             "web.discovery",
			Action:           compiler.ActionDiscoverAssets,
			Description:      "discover the root web/API surface for a URL target",
			ExpectedEvidence: []evidence.NodeType{evidence.NodeTarget, evidence.NodeSurface},
		},
		{
			Name:             "web.route_enumeration",
			Action:           compiler.ActionEnumeratePaths,
			Description:      "enumerate application routes and API paths",
			ExpectedEvidence: []evidence.NodeType{evidence.NodeSurface},
		},
		{
			Name:             "web.auth_state_tracking",
			Action:           compiler.ActionFingerprintSurface,
			Description:      "track anonymous, authenticated, expired, and elevated auth states",
			ExpectedEvidence: []evidence.NodeType{evidence.NodeIdentity, evidence.NodeSurface},
		},
		{
			Name:             "web.candidate_generation",
			Action:           compiler.ActionGenerateCandidates,
			Description:      "generate candidate parameters, routes, and authorization checks",
			ExpectedEvidence: []evidence.NodeType{evidence.NodeHypothesis},
		},
		{
			Name:             "evidence.collection",
			Action:           compiler.ActionCollectEvidence,
			Description:      "collect request, response, proof, and reproduction artifacts",
			ExpectedEvidence: []evidence.NodeType{evidence.NodeArtifact},
		},
	}
}

func (Pack) CoverageDimensions() []frontier.Dimension {
	return []frontier.Dimension{
		frontier.DimensionRoutes,
		frontier.DimensionParams,
		frontier.DimensionAuthStates,
	}
}

func (Pack) Parsers() []compiler.ParserSpec {
	return []compiler.ParserSpec{
		{Name: "route_observation_parser", FactKind: "surface", StatementField: "route", ConfidenceField: "confidence", RequiredFields: []string{"route"}},
		{Name: "auth_state_parser", FactKind: "identity", StatementField: "auth_state", ConfidenceField: "confidence", RequiredFields: []string{"auth_state"}},
		{Name: "web_evidence_parser", FactKind: "artifact", StatementField: "evidence_ref", ConfidenceField: "confidence", RequiredFields: []string{"evidence_ref"}},
	}
}

func (Pack) StrategyPatterns() []patterns.Pattern {
	return []patterns.Pattern{
		{Name: "explore_plan_execute", Version: "0.1.0"},
		{Name: "ralphloop", Version: "0.1.0"},
	}
}
