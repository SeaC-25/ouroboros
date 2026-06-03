package domain

import (
	"strings"
	"testing"

	"github.com/ouroboros/ouroboros/internal/compiler"
	"github.com/ouroboros/ouroboros/internal/evidence"
	"github.com/ouroboros/ouroboros/internal/frontier"
	"github.com/ouroboros/ouroboros/internal/patterns"
)

func TestFakeDomainPackExposesSDKContracts(t *testing.T) {
	pack := fakePack{}
	var _ Pack = pack

	if err := ValidatePack(pack); err != nil {
		t.Fatalf("validate fake pack: %v", err)
	}
	if pack.Name() != "fake-web" {
		t.Fatalf("unexpected pack name: %s", pack.Name())
	}
	if pack.Domain() != frontier.DomainWebAPI {
		t.Fatalf("unexpected domain: %s", pack.Domain())
	}

	capabilities := pack.Capabilities()
	if len(capabilities) != 2 {
		t.Fatalf("expected two capabilities, got %#v", capabilities)
	}
	if capabilities[0].Name != "web.discovery" || capabilities[0].Action != compiler.ActionDiscoverAssets {
		t.Fatalf("unexpected first capability: %#v", capabilities[0])
	}

	hypotheses, err := pack.InitialHypotheses(Target{Kind: TargetURL, Value: "https://app.example"})
	if err != nil {
		t.Fatalf("initial hypotheses: %v", err)
	}
	if len(hypotheses) != 1 {
		t.Fatalf("expected one hypothesis, got %#v", hypotheses)
	}
	if hypotheses[0].Action != compiler.ActionEnumeratePaths {
		t.Fatalf("expected enumerate paths action, got %s", hypotheses[0].Action)
	}
	if hypotheses[0].CoverageDimension != frontier.DimensionRoutes {
		t.Fatalf("expected routes coverage, got %s", hypotheses[0].CoverageDimension)
	}

	if len(pack.CoverageDimensions()) != 1 || pack.CoverageDimensions()[0] != frontier.DimensionRoutes {
		t.Fatalf("unexpected coverage dimensions: %#v", pack.CoverageDimensions())
	}
	if len(pack.Parsers()) != 1 || pack.Parsers()[0].Name != "route_observation_parser" {
		t.Fatalf("unexpected parsers: %#v", pack.Parsers())
	}
	if len(pack.StrategyPatterns()) != 1 || pack.StrategyPatterns()[0].Name != "explore_plan_execute" {
		t.Fatalf("unexpected patterns: %#v", pack.StrategyPatterns())
	}
}

func TestValidatePackRejectsIncompletePack(t *testing.T) {
	err := ValidatePack(incompletePack{})
	if err == nil {
		t.Fatalf("expected incomplete pack to fail")
	}
	for _, expected := range []string{"capability", "coverage", "parser", "strategy"} {
		if !strings.Contains(err.Error(), expected) {
			t.Fatalf("expected validation error to mention %q, got %v", expected, err)
		}
	}
}

type fakePack struct{}

func (fakePack) Name() string {
	return "fake-web"
}

func (fakePack) Domain() frontier.Domain {
	return frontier.DomainWebAPI
}

func (fakePack) Capabilities() []Capability {
	return []Capability{
		{Name: "web.discovery", Action: compiler.ActionDiscoverAssets, ExpectedEvidence: []evidence.NodeType{evidence.NodeTarget}},
		{Name: "web.route_enumeration", Action: compiler.ActionEnumeratePaths, ExpectedEvidence: []evidence.NodeType{evidence.NodeSurface}},
	}
}

func (fakePack) InitialHypotheses(target Target) ([]Hypothesis, error) {
	return []Hypothesis{
		{
			ID:                "hyp-route-enumeration",
			Title:             "enumerate routes",
			Target:            target.Value,
			Action:            compiler.ActionEnumeratePaths,
			CoverageDimension: frontier.DimensionRoutes,
			ExpectedEvidence:  []evidence.NodeType{evidence.NodeSurface},
			Confidence:        0.7,
		},
	}, nil
}

func (fakePack) CoverageDimensions() []frontier.Dimension {
	return []frontier.Dimension{frontier.DimensionRoutes}
}

func (fakePack) Parsers() []compiler.ParserSpec {
	return []compiler.ParserSpec{
		{Name: "route_observation_parser", FactKind: "surface", StatementField: "route"},
	}
}

func (fakePack) StrategyPatterns() []patterns.Pattern {
	return []patterns.Pattern{{Name: "explore_plan_execute", Version: "0.1.0"}}
}

type incompletePack struct{}

func (incompletePack) Name() string {
	return "incomplete"
}

func (incompletePack) Domain() frontier.Domain {
	return frontier.DomainWebAPI
}

func (incompletePack) Capabilities() []Capability {
	return nil
}

func (incompletePack) InitialHypotheses(Target) ([]Hypothesis, error) {
	return nil, nil
}

func (incompletePack) CoverageDimensions() []frontier.Dimension {
	return nil
}

func (incompletePack) Parsers() []compiler.ParserSpec {
	return nil
}

func (incompletePack) StrategyPatterns() []patterns.Pattern {
	return nil
}
