package domain

import (
	"errors"
	"fmt"
	"strings"

	"github.com/ouroboros/ouroboros/internal/compiler"
	"github.com/ouroboros/ouroboros/internal/evidence"
	"github.com/ouroboros/ouroboros/internal/frontier"
	"github.com/ouroboros/ouroboros/internal/patterns"
)

type TargetKind string

const (
	TargetURL       TargetKind = "url"
	TargetCIDR      TargetKind = "cidr"
	TargetChallenge TargetKind = "challenge"
	TargetArtifact  TargetKind = "artifact"
)

type Target struct {
	Kind     TargetKind
	Value    string
	Metadata map[string]string
}

type Capability struct {
	Name             string
	Action           compiler.SemanticAction
	Description      string
	ExpectedEvidence []evidence.NodeType
}

type Hypothesis struct {
	ID                string
	Title             string
	Target            string
	Rationale         string
	Action            compiler.SemanticAction
	CoverageDimension frontier.Dimension
	ExpectedEvidence  []evidence.NodeType
	Confidence        float64
}

type Pack interface {
	Name() string
	Domain() frontier.Domain
	Capabilities() []Capability
	InitialHypotheses(Target) ([]Hypothesis, error)
	CoverageDimensions() []frontier.Dimension
	Parsers() []compiler.ParserSpec
	StrategyPatterns() []patterns.Pattern
}

func ValidatePack(pack Pack) error {
	if pack == nil {
		return errors.New("domain pack is required")
	}

	var errs []error
	if strings.TrimSpace(pack.Name()) == "" {
		errs = append(errs, errors.New("domain pack name is required"))
	}
	if strings.TrimSpace(string(pack.Domain())) == "" {
		errs = append(errs, errors.New("domain pack domain is required"))
	}

	capabilities := pack.Capabilities()
	if len(capabilities) == 0 {
		errs = append(errs, errors.New("domain pack capability list is required"))
	}
	for i, capability := range capabilities {
		if strings.TrimSpace(capability.Name) == "" {
			errs = append(errs, fmt.Errorf("domain pack capability %d name is required", i))
		}
		if strings.TrimSpace(string(capability.Action)) == "" {
			errs = append(errs, fmt.Errorf("domain pack capability %s action is required", capability.Name))
		}
	}

	if len(pack.CoverageDimensions()) == 0 {
		errs = append(errs, errors.New("domain pack coverage dimensions are required"))
	}

	parsers := pack.Parsers()
	if len(parsers) == 0 {
		errs = append(errs, errors.New("domain pack parser specs are required"))
	}
	for i, parser := range parsers {
		if strings.TrimSpace(parser.Name) == "" {
			errs = append(errs, fmt.Errorf("domain pack parser %d name is required", i))
		}
	}

	strategies := pack.StrategyPatterns()
	if len(strategies) == 0 {
		errs = append(errs, errors.New("domain pack strategy patterns are required"))
	}
	for i, strategy := range strategies {
		if strings.TrimSpace(strategy.Name) == "" {
			errs = append(errs, fmt.Errorf("domain pack strategy pattern %d name is required", i))
		}
	}

	return errors.Join(errs...)
}
