package webapi

import (
	"strings"
	"testing"

	"github.com/ouroboros/ouroboros/internal/compiler"
	"github.com/ouroboros/ouroboros/internal/domain"
	"github.com/ouroboros/ouroboros/internal/frontier"
)

func TestPackDeclaresWebAPICapabilitiesAndCoverage(t *testing.T) {
	pack := NewPack()
	var _ domain.Pack = pack

	if err := domain.ValidatePack(pack); err != nil {
		t.Fatalf("validate web/api pack: %v", err)
	}
	if pack.Name() != "web-api" {
		t.Fatalf("unexpected pack name: %s", pack.Name())
	}
	if pack.Domain() != frontier.DomainWebAPI {
		t.Fatalf("unexpected domain: %s", pack.Domain())
	}

	actionsByCapability := map[string]compiler.SemanticAction{}
	for _, capability := range pack.Capabilities() {
		actionsByCapability[capability.Name] = capability.Action
	}
	expected := map[string]compiler.SemanticAction{
		"web.discovery":            compiler.ActionDiscoverAssets,
		"web.route_enumeration":    compiler.ActionEnumeratePaths,
		"web.auth_state_tracking":  compiler.ActionFingerprintSurface,
		"web.candidate_generation": compiler.ActionGenerateCandidates,
		"evidence.collection":      compiler.ActionCollectEvidence,
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
	for _, dimension := range []frontier.Dimension{frontier.DimensionRoutes, frontier.DimensionParams, frontier.DimensionAuthStates} {
		if !dimensions[dimension] {
			t.Fatalf("expected coverage dimension %s in %#v", dimension, pack.CoverageDimensions())
		}
	}
}

func TestURLTargetProducesInitialHypotheses(t *testing.T) {
	pack := NewPack()

	hypotheses, err := pack.InitialHypotheses(domain.Target{Kind: domain.TargetURL, Value: "https://app.example"})
	if err != nil {
		t.Fatalf("initial hypotheses: %v", err)
	}
	if len(hypotheses) < 3 {
		t.Fatalf("expected at least three initial hypotheses, got %#v", hypotheses)
	}

	actions := map[compiler.SemanticAction]bool{}
	coverage := map[frontier.Dimension]bool{}
	for _, hypothesis := range hypotheses {
		if hypothesis.Target != "https://app.example" {
			t.Fatalf("hypothesis target not propagated: %#v", hypothesis)
		}
		if hypothesis.Confidence <= 0 || hypothesis.Confidence > 1 {
			t.Fatalf("hypothesis confidence out of range: %#v", hypothesis)
		}
		actions[hypothesis.Action] = true
		coverage[hypothesis.CoverageDimension] = true
	}

	for _, action := range []compiler.SemanticAction{
		compiler.ActionDiscoverAssets,
		compiler.ActionEnumeratePaths,
		compiler.ActionFingerprintSurface,
		compiler.ActionGenerateCandidates,
	} {
		if !actions[action] {
			t.Fatalf("expected hypothesis action %s in %#v", action, hypotheses)
		}
	}
	for _, dimension := range []frontier.Dimension{frontier.DimensionRoutes, frontier.DimensionParams, frontier.DimensionAuthStates} {
		if !coverage[dimension] {
			t.Fatalf("expected coverage dimension %s in hypotheses %#v", dimension, hypotheses)
		}
	}
}

func TestPackRejectsNonURLTarget(t *testing.T) {
	_, err := NewPack().InitialHypotheses(domain.Target{Kind: domain.TargetCIDR, Value: "10.0.0.0/24"})
	if err == nil {
		t.Fatalf("expected CIDR target to fail")
	}
	if !strings.Contains(err.Error(), "url target") {
		t.Fatalf("expected url target error, got %v", err)
	}
}
