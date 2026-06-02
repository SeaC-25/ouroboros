package infra

import (
	"strings"
	"testing"

	"github.com/ouroboros/ouroboros/internal/compiler"
	"github.com/ouroboros/ouroboros/internal/domain"
	"github.com/ouroboros/ouroboros/internal/frontier"
)

func TestPackDeclaresInfraCapabilitiesAndCoverage(t *testing.T) {
	pack := NewPack()
	var _ domain.Pack = pack

	if err := domain.ValidatePack(pack); err != nil {
		t.Fatalf("validate infra pack: %v", err)
	}
	if pack.Name() != "infra-internal" {
		t.Fatalf("unexpected pack name: %s", pack.Name())
	}
	if pack.Domain() != frontier.DomainInfra {
		t.Fatalf("unexpected domain: %s", pack.Domain())
	}

	actionsByCapability := map[string]compiler.SemanticAction{}
	for _, capability := range pack.Capabilities() {
		actionsByCapability[capability.Name] = capability.Action
	}
	expected := map[string]compiler.SemanticAction{
		"infra.host_discovery":      compiler.ActionDiscoverAssets,
		"infra.service_enumeration": compiler.ActionFingerprintSurface,
		"infra.identity_graph":      compiler.ActionFingerprintSurface,
		"infra.path_hypothesis":     compiler.ActionGenerateCandidates,
		"evidence.collection":       compiler.ActionCollectEvidence,
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
	for _, dimension := range []frontier.Dimension{frontier.DimensionHosts, frontier.DimensionServices, frontier.DimensionIdentities} {
		if !dimensions[dimension] {
			t.Fatalf("expected coverage dimension %s in %#v", dimension, pack.CoverageDimensions())
		}
	}
}

func TestCIDRTargetProducesHostServiceCoverageHypotheses(t *testing.T) {
	pack := NewPack()

	hypotheses, err := pack.InitialHypotheses(domain.Target{Kind: domain.TargetCIDR, Value: "10.0.0.0/24"})
	if err != nil {
		t.Fatalf("initial hypotheses: %v", err)
	}
	if len(hypotheses) < 3 {
		t.Fatalf("expected at least three initial hypotheses, got %#v", hypotheses)
	}

	coverage := map[frontier.Dimension]bool{}
	actions := map[compiler.SemanticAction]bool{}
	for _, hypothesis := range hypotheses {
		if hypothesis.Target != "10.0.0.0/24" {
			t.Fatalf("hypothesis target not propagated: %#v", hypothesis)
		}
		if hypothesis.Confidence <= 0 || hypothesis.Confidence > 1 {
			t.Fatalf("hypothesis confidence out of range: %#v", hypothesis)
		}
		coverage[hypothesis.CoverageDimension] = true
		actions[hypothesis.Action] = true
	}
	for _, dimension := range []frontier.Dimension{frontier.DimensionHosts, frontier.DimensionServices, frontier.DimensionIdentities} {
		if !coverage[dimension] {
			t.Fatalf("expected coverage dimension %s in hypotheses %#v", dimension, hypotheses)
		}
	}
	for _, action := range []compiler.SemanticAction{
		compiler.ActionDiscoverAssets,
		compiler.ActionFingerprintSurface,
		compiler.ActionGenerateCandidates,
	} {
		if !actions[action] {
			t.Fatalf("expected hypothesis action %s in %#v", action, hypotheses)
		}
	}
}

func TestPackRejectsNonCIDRTarget(t *testing.T) {
	_, err := NewPack().InitialHypotheses(domain.Target{Kind: domain.TargetURL, Value: "https://app.example"})
	if err == nil {
		t.Fatalf("expected URL target to fail")
	}
	if !strings.Contains(err.Error(), "cidr target") {
		t.Fatalf("expected cidr target error, got %v", err)
	}
}
