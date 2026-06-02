package webapi

import (
	"fmt"
	"strings"

	"github.com/ouroboros/ouroboros/internal/compiler"
	"github.com/ouroboros/ouroboros/internal/domain"
	"github.com/ouroboros/ouroboros/internal/evidence"
	"github.com/ouroboros/ouroboros/internal/frontier"
)

func (Pack) InitialHypotheses(target domain.Target) ([]domain.Hypothesis, error) {
	if target.Kind != domain.TargetURL {
		return nil, fmt.Errorf("web-api pack requires url target, got %s", target.Kind)
	}
	targetValue := strings.TrimSpace(target.Value)
	if targetValue == "" {
		return nil, fmt.Errorf("web-api pack requires non-empty url target")
	}

	return []domain.Hypothesis{
		{
			ID:                "web-discover-surface",
			Title:             "discover initial web/API surface",
			Target:            targetValue,
			Rationale:         "baseline response, redirects, and exposed route hints determine the first useful frontier",
			Action:            compiler.ActionDiscoverAssets,
			CoverageDimension: frontier.DimensionRoutes,
			ExpectedEvidence:  []evidence.NodeType{evidence.NodeTarget, evidence.NodeSurface},
			Confidence:        0.65,
		},
		{
			ID:                "web-enumerate-routes",
			Title:             "enumerate routable paths",
			Target:            targetValue,
			Rationale:         "route inventory controls breadth before deeper candidate verification",
			Action:            compiler.ActionEnumeratePaths,
			CoverageDimension: frontier.DimensionRoutes,
			ExpectedEvidence:  []evidence.NodeType{evidence.NodeSurface},
			Confidence:        0.7,
		},
		{
			ID:                "web-map-auth-states",
			Title:             "map authentication states",
			Target:            targetValue,
			Rationale:         "authorization and session-state differences are high-value Web/API pivots",
			Action:            compiler.ActionFingerprintSurface,
			CoverageDimension: frontier.DimensionAuthStates,
			ExpectedEvidence:  []evidence.NodeType{evidence.NodeIdentity, evidence.NodeSurface},
			Confidence:        0.6,
		},
		{
			ID:                "web-generate-candidates",
			Title:             "generate parameter and authorization candidates",
			Target:            targetValue,
			Rationale:         "parameters and auth boundaries produce candidate findings after route discovery",
			Action:            compiler.ActionGenerateCandidates,
			CoverageDimension: frontier.DimensionParams,
			ExpectedEvidence:  []evidence.NodeType{evidence.NodeHypothesis},
			Confidence:        0.55,
		},
	}, nil
}
