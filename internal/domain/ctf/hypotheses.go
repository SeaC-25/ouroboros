package ctf

import (
	"fmt"
	"strings"

	"github.com/ouroboros/ouroboros/internal/compiler"
	"github.com/ouroboros/ouroboros/internal/domain"
	"github.com/ouroboros/ouroboros/internal/evidence"
	"github.com/ouroboros/ouroboros/internal/frontier"
)

func (Pack) InitialHypotheses(target domain.Target) ([]domain.Hypothesis, error) {
	if target.Kind != domain.TargetChallenge && target.Kind != domain.TargetArtifact {
		return nil, fmt.Errorf("ctf-lab pack requires challenge or artifact target, got %s", target.Kind)
	}
	targetValue := strings.TrimSpace(target.Value)
	if targetValue == "" {
		return nil, fmt.Errorf("ctf-lab pack requires non-empty challenge or artifact target")
	}
	if target.Kind == domain.TargetArtifact {
		return []domain.Hypothesis{
			{
				ID:                "ctf-analyze-artifact",
				Title:             "analyze provided artifact",
				Target:            targetValue,
				Rationale:         "explicit local artifacts can contain the strongest static signals before broader challenge solving",
				Action:            compiler.ActionCollectEvidence,
				CoverageDimension: frontier.DimensionArtifacts,
				ExpectedEvidence:  []evidence.NodeType{evidence.NodeArtifact},
				Confidence:        0.7,
			},
		}, nil
	}

	return []domain.Hypothesis{
		{
			ID:                "ctf-classify-challenge",
			Title:             "classify challenge type",
			Target:            targetValue,
			Rationale:         "category and constraint classification narrows early solve paths before expensive enumeration",
			Action:            compiler.ActionFingerprintSurface,
			CoverageDimension: frontier.DimensionHypotheses,
			ExpectedEvidence:  []evidence.NodeType{evidence.NodeHypothesis},
			Confidence:        0.72,
		},
		{
			ID:                "ctf-analyze-artifacts",
			Title:             "analyze provided artifacts",
			Target:            targetValue,
			Rationale:         "provided files and prompt details usually contain the strongest initial signals",
			Action:            compiler.ActionCollectEvidence,
			CoverageDimension: frontier.DimensionArtifacts,
			ExpectedEvidence:  []evidence.NodeType{evidence.NodeArtifact},
			Confidence:        0.68,
		},
		{
			ID:                "ctf-generate-fast-hypotheses",
			Title:             "generate fast solve hypotheses",
			Target:            targetValue,
			Rationale:         "early CTF progress depends on testing a small set of likely approaches before broad enumeration",
			Action:            compiler.ActionGenerateCandidates,
			CoverageDimension: frontier.DimensionHypotheses,
			ExpectedEvidence:  []evidence.NodeType{evidence.NodeHypothesis},
			Confidence:        0.62,
		},
	}, nil
}
