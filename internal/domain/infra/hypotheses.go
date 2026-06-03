package infra

import (
	"fmt"
	"net/netip"
	"strings"

	"github.com/ouroboros/ouroboros/internal/compiler"
	"github.com/ouroboros/ouroboros/internal/domain"
	"github.com/ouroboros/ouroboros/internal/evidence"
	"github.com/ouroboros/ouroboros/internal/frontier"
)

func (Pack) InitialHypotheses(target domain.Target) ([]domain.Hypothesis, error) {
	targetValue := strings.TrimSpace(target.Value)
	if targetValue == "" {
		return nil, fmt.Errorf("infra-internal pack requires non-empty target")
	}
	switch target.Kind {
	case domain.TargetCIDR:
		if _, err := netip.ParsePrefix(targetValue); err != nil {
			return nil, fmt.Errorf("infra-internal pack requires cidr target: %w", err)
		}
	case domain.TargetDomain, domain.TargetHost:
	default:
		return nil, fmt.Errorf("infra-internal pack requires cidr, domain, or host target, got %s", target.Kind)
	}

	return []domain.Hypothesis{
		{
			ID:                "infra-discover-hosts",
			Title:             "discover live hosts",
			Target:            targetValue,
			Rationale:         "host inventory establishes breadth and prevents over-focusing on a single visible service",
			Action:            compiler.ActionDiscoverAssets,
			CoverageDimension: frontier.DimensionHosts,
			ExpectedEvidence:  []evidence.NodeType{evidence.NodeTarget},
			Confidence:        0.7,
		},
		{
			ID:                "infra-enumerate-services",
			Title:             "enumerate services",
			Target:            targetValue,
			Rationale:         "service and protocol fingerprints determine reachable attack surfaces",
			Action:            compiler.ActionFingerprintSurface,
			CoverageDimension: frontier.DimensionServices,
			ExpectedEvidence:  []evidence.NodeType{evidence.NodeSurface},
			Confidence:        0.68,
		},
		{
			ID:                "infra-map-identities",
			Title:             "map observed identities",
			Target:            targetValue,
			Rationale:         "identities and trust hints enable internal path reasoning",
			Action:            compiler.ActionFingerprintSurface,
			CoverageDimension: frontier.DimensionIdentities,
			ExpectedEvidence:  []evidence.NodeType{evidence.NodeIdentity},
			Confidence:        0.55,
		},
		{
			ID:                "infra-generate-path-hypotheses",
			Title:             "generate internal path hypotheses",
			Target:            targetValue,
			Rationale:         "host, service, and identity observations should produce candidate lateral or privilege paths",
			Action:            compiler.ActionGenerateCandidates,
			CoverageDimension: frontier.DimensionIdentities,
			ExpectedEvidence:  []evidence.NodeType{evidence.NodeHypothesis},
			Confidence:        0.52,
		},
	}, nil
}
