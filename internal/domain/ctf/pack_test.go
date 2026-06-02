package ctf

import (
	"strings"
	"testing"

	"github.com/ouroboros/ouroboros/internal/compiler"
	"github.com/ouroboros/ouroboros/internal/domain"
	"github.com/ouroboros/ouroboros/internal/frontier"
)

func TestPackDeclaresCTFCapabilitiesAndCoverage(t *testing.T) {
	pack := NewPack()
	var _ domain.Pack = pack

	if err := domain.ValidatePack(pack); err != nil {
		t.Fatalf("validate ctf/lab pack: %v", err)
	}
	if pack.Name() != "ctf-lab" {
		t.Fatalf("unexpected pack name: %s", pack.Name())
	}
	if pack.Domain() != frontier.DomainCTF {
		t.Fatalf("unexpected domain: %s", pack.Domain())
	}

	actionsByCapability := map[string]compiler.SemanticAction{}
	for _, capability := range pack.Capabilities() {
		actionsByCapability[capability.Name] = capability.Action
	}
	expected := map[string]compiler.SemanticAction{
		"ctf.challenge_classification": compiler.ActionFingerprintSurface,
		"ctf.artifact_analysis":        compiler.ActionCollectEvidence,
		"ctf.fast_hypothesis":          compiler.ActionGenerateCandidates,
		"ctf.candidate_verification":   compiler.ActionVerifyCandidate,
		"ctf.result_proof":             compiler.ActionProduceReport,
	}
	for name, action := range expected {
		if actionsByCapability[name] != action {
			t.Fatalf("expected capability %s -> %s, got %s", name, action, actionsByCapability[name])
		}
	}

	dimensions := map[frontier.Dimension]bool{}
	for _, dimension := range pack.CoverageDimensions() {
		dimensions[dimension] = true
	}
	for _, dimension := range []frontier.Dimension{frontier.DimensionHypotheses, frontier.DimensionArtifacts, frontier.DimensionProofs} {
		if !dimensions[dimension] {
			t.Fatalf("expected coverage dimension %s in %#v", dimension, pack.CoverageDimensions())
		}
	}
}

func TestChallengeTargetProducesThreeHypothesesBeforeBroadEnumeration(t *testing.T) {
	pack := NewPack()

	hypotheses, err := pack.InitialHypotheses(domain.Target{Kind: domain.TargetChallenge, Value: "pwnable-login"})
	if err != nil {
		t.Fatalf("initial hypotheses: %v", err)
	}
	if len(hypotheses) != 3 {
		t.Fatalf("expected exactly three initial hypotheses, got %#v", hypotheses)
	}

	expectedActions := []compiler.SemanticAction{
		compiler.ActionFingerprintSurface,
		compiler.ActionCollectEvidence,
		compiler.ActionGenerateCandidates,
	}
	expectedCoverage := []frontier.Dimension{
		frontier.DimensionHypotheses,
		frontier.DimensionArtifacts,
		frontier.DimensionHypotheses,
	}
	for i, hypothesis := range hypotheses {
		if hypothesis.Target != "pwnable-login" {
			t.Fatalf("hypothesis target not propagated: %#v", hypothesis)
		}
		if hypothesis.Action != expectedActions[i] {
			t.Fatalf("hypothesis %d action: expected %s, got %s", i, expectedActions[i], hypothesis.Action)
		}
		if hypothesis.Action == compiler.ActionEnumeratePaths {
			t.Fatalf("hypothesis %d should not start broad enumeration: %#v", i, hypothesis)
		}
		if hypothesis.CoverageDimension != expectedCoverage[i] {
			t.Fatalf("hypothesis %d coverage: expected %s, got %s", i, expectedCoverage[i], hypothesis.CoverageDimension)
		}
		if hypothesis.Confidence <= 0 || hypothesis.Confidence > 1 {
			t.Fatalf("hypothesis confidence out of range: %#v", hypothesis)
		}
	}
}

func TestPackRejectsNonChallengeTarget(t *testing.T) {
	_, err := NewPack().InitialHypotheses(domain.Target{Kind: domain.TargetURL, Value: "https://app.example"})
	if err == nil {
		t.Fatalf("expected URL target to fail")
	}
	if !strings.Contains(err.Error(), "challenge target") {
		t.Fatalf("expected challenge target error, got %v", err)
	}
}
