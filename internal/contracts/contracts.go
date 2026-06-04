package contracts

import (
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ouroboros/ouroboros/internal/compiler"
	"github.com/ouroboros/ouroboros/internal/domain"
	"github.com/ouroboros/ouroboros/internal/domain/ctf"
	"github.com/ouroboros/ouroboros/internal/domain/infra"
	"github.com/ouroboros/ouroboros/internal/domain/webapi"
	"github.com/ouroboros/ouroboros/internal/patterns"
	"github.com/ouroboros/ouroboros/internal/toolchain"
)

type Inputs struct {
	Manifests   []toolchain.Manifest
	ToolCatalog []toolchain.BuiltInTool
	DomainPacks []domain.Pack
	Patterns    []patterns.Pattern
}

func ValidateBuiltIn(repositoryRoot string) error {
	manifests, err := toolchain.LoadManifests(filepath.Join(repositoryRoot, "toolchains"))
	if err != nil {
		return err
	}
	loadedPatterns, err := patterns.LoadDir(filepath.Join(repositoryRoot, "configs", "patterns"))
	if err != nil {
		return err
	}
	return Validate(Inputs{
		Manifests:   manifests,
		ToolCatalog: toolchain.BuiltInAppSecTools(),
		DomainPacks: BuiltInDomainPacks(),
		Patterns:    loadedPatterns,
	})
}

func BuiltInDomainPacks() []domain.Pack {
	return []domain.Pack{
		webapi.NewPack(),
		infra.NewPack(),
		ctf.NewPack(),
	}
}

func BuiltInPatternContracts() []patterns.Pattern {
	return []patterns.Pattern{
		minimalPattern("ant_colony"),
		minimalPattern("explore_plan_execute"),
		minimalPattern("ralphloop"),
	}
}

func minimalPattern(name string) patterns.Pattern {
	return patterns.Pattern{
		Name:     name,
		Version:  "1",
		MaxSteps: 1,
		Nodes: []patterns.Node{
			{ID: "done", Kind: "result", Terminal: true},
		},
	}
}

func Validate(inputs Inputs) error {
	var errs []error
	parserRegistry := toolchain.DefaultAuthoredToolRegistry()
	manifestsByName, manifestCapabilities, manifestErrs := validateManifests(inputs.Manifests, parserRegistry)
	errs = append(errs, manifestErrs...)

	catalog := inputs.ToolCatalog
	if catalog == nil {
		catalog = toolchain.BuiltInAppSecTools()
	}
	catalogCapabilities, catalogErrs := validateToolCatalog(catalog, manifestsByName, parserRegistry)
	errs = append(errs, catalogErrs...)

	patternNames, patternErrs := validatePatterns(inputs.Patterns)
	errs = append(errs, patternErrs...)

	domainCapabilities, domainErrs := validateDomainPacks(inputs.DomainPacks, patternNames)
	errs = append(errs, domainErrs...)
	errs = append(errs, validateDomainCapabilitySupport(domainCapabilities, manifestCapabilities, catalogCapabilities)...)

	knownCapabilities := mergeCapabilitySets(manifestCapabilities, catalogCapabilities, domainCapabilityNames(domainCapabilities))
	errs = append(errs, validateSemanticActions(inputs.Manifests, catalog, knownCapabilities, domainCapabilities)...)

	return errors.Join(errs...)
}

func validateManifests(manifests []toolchain.Manifest, parserRegistry toolchain.AuthoredToolRegistry) (map[string]toolchain.Manifest, map[string]bool, []error) {
	byName := map[string]toolchain.Manifest{}
	capabilities := map[string]bool{}
	var errs []error
	if len(manifests) == 0 {
		errs = append(errs, errors.New("toolchain manifests are required"))
		return byName, capabilities, errs
	}
	for _, manifest := range manifests {
		if err := toolchain.ValidateManifest(manifest); err != nil {
			errs = append(errs, fmt.Errorf("validate manifest %s: %w", manifest.Name, err))
			continue
		}
		name := strings.TrimSpace(manifest.Name)
		if _, exists := byName[name]; exists {
			errs = append(errs, fmt.Errorf("duplicate manifest name: %s", name))
		}
		byName[name] = manifest
		parser := strings.TrimSpace(manifest.Parser)
		if parser != "" && !parserRegistry.ParserExists(parser) {
			errs = append(errs, fmt.Errorf("manifest %s parser %s is not registered", name, parser))
		}
		for _, capability := range manifest.Capabilities {
			if normalized := strings.TrimSpace(capability); normalized != "" {
				capabilities[normalized] = true
			}
		}
	}
	return byName, capabilities, errs
}

func validateToolCatalog(catalog []toolchain.BuiltInTool, manifestsByName map[string]toolchain.Manifest, parserRegistry toolchain.AuthoredToolRegistry) (map[string]bool, []error) {
	capabilities := map[string]bool{}
	var errs []error
	for i, tool := range catalog {
		name := strings.TrimSpace(tool.Name)
		pack := strings.TrimSpace(tool.Pack)
		if name == "" {
			errs = append(errs, fmt.Errorf("tool catalog entry %d name is required", i))
		}
		if pack == "" {
			label := name
			if label == "" {
				label = fmt.Sprintf("%d", i)
			}
			errs = append(errs, fmt.Errorf("tool catalog entry %s pack is required", label))
		}
		if name == "" || pack == "" {
			continue
		}
		manifest, ok := manifestsByName[pack]
		if !ok {
			errs = append(errs, fmt.Errorf("tool catalog entry %s references unknown manifest %s", name, pack))
		}
		manifestCapabilities := manifestCapabilitySet(manifest)
		manifestDependencies := manifestDependencySet(manifest)
		if len(tool.Capabilities) == 0 {
			errs = append(errs, fmt.Errorf("tool catalog entry %s capability list is required", name))
		}
		for capabilityIndex, capability := range tool.Capabilities {
			normalized := strings.TrimSpace(capability)
			if normalized == "" {
				errs = append(errs, fmt.Errorf("tool catalog entry %s capability %d is required", name, capabilityIndex))
				continue
			}
			capabilities[normalized] = true
			if ok && !manifestCapabilities[normalized] {
				errs = append(errs, fmt.Errorf("tool catalog entry %s capability %s is not declared by manifest %s", name, normalized, pack))
			}
		}
		dependency := strings.TrimSpace(tool.Dependency)
		if ok && dependency != "" && !manifestDependencies[dependency] {
			errs = append(errs, fmt.Errorf("tool catalog entry %s dependency %s is not declared by manifest %s", name, dependency, pack))
		}
		parser := strings.TrimSpace(tool.Parser)
		adapterID := strings.TrimSpace(tool.AdapterID)
		operationID := strings.TrimSpace(tool.Operation)
		if parser == "" {
			continue
		}
		if !parserRegistry.ParserExists(parser) {
			errs = append(errs, fmt.Errorf("tool catalog entry %s parser %s is not registered", name, parser))
			continue
		}
		if adapterID == "" && operationID == "" {
			continue
		}
		operation, operationOK := parserRegistry.AdapterOperation(adapterID, operationID)
		if !operationOK {
			errs = append(errs, fmt.Errorf("tool catalog entry %s operation %s/%s is not registered", name, adapterID, operationID))
			continue
		}
		if !containsTrimmed(operation.SupportedParsers, parser) {
			errs = append(errs, fmt.Errorf("tool catalog entry %s parser %s is not supported by operation %s/%s", name, parser, adapterID, operationID))
		}
	}
	return capabilities, errs
}

func containsTrimmed(values []string, needle string) bool {
	needle = strings.TrimSpace(needle)
	for _, value := range values {
		if strings.TrimSpace(value) == needle {
			return true
		}
	}
	return false
}

func manifestCapabilitySet(manifest toolchain.Manifest) map[string]bool {
	capabilities := map[string]bool{}
	for _, capability := range manifest.Capabilities {
		normalized := strings.TrimSpace(capability)
		if normalized != "" {
			capabilities[normalized] = true
		}
	}
	return capabilities
}

func manifestDependencySet(manifest toolchain.Manifest) map[string]bool {
	dependencies := map[string]bool{}
	for _, dependency := range manifest.Dependencies {
		name := strings.TrimSpace(dependency.Name)
		if name != "" {
			dependencies[name] = true
		}
	}
	return dependencies
}

func validateDomainCapabilitySupport(domainCapabilities map[string]compiler.SemanticAction, manifestCapabilities map[string]bool, catalogCapabilities map[string]bool) []error {
	var errs []error
	for capability := range domainCapabilities {
		if manifestCapabilities[capability] || catalogCapabilities[capability] {
			continue
		}
		errs = append(errs, fmt.Errorf("domain capability %s is not provided by workspace manifests or built-in tools", capability))
	}
	sort.Slice(errs, func(i, j int) bool {
		return errs[i].Error() < errs[j].Error()
	})
	return errs
}

func validatePatterns(loaded []patterns.Pattern) (map[string]bool, []error) {
	names := map[string]bool{}
	var errs []error
	if len(loaded) == 0 {
		errs = append(errs, errors.New("strategy patterns are required"))
		return names, errs
	}
	for _, pattern := range loaded {
		if err := patterns.Validate(pattern); err != nil {
			errs = append(errs, fmt.Errorf("validate strategy pattern %s: %w", pattern.Name, err))
			continue
		}
		name := strings.TrimSpace(pattern.Name)
		if names[name] {
			errs = append(errs, fmt.Errorf("duplicate strategy pattern name: %s", name))
		}
		names[name] = true
	}
	return names, errs
}

func validateDomainPacks(packs []domain.Pack, patternNames map[string]bool) (map[string]compiler.SemanticAction, []error) {
	capabilities := map[string]compiler.SemanticAction{}
	var errs []error
	if len(packs) == 0 {
		errs = append(errs, errors.New("domain packs are required"))
		return capabilities, errs
	}
	for i, pack := range packs {
		if pack == nil {
			errs = append(errs, fmt.Errorf("validate domain pack %d: %w", i, domain.ValidatePack(pack)))
			continue
		}
		packName := strings.TrimSpace(pack.Name())
		if packName == "" {
			packName = fmt.Sprintf("%d", i)
		}
		if err := domain.ValidatePack(pack); err != nil {
			errs = append(errs, fmt.Errorf("validate domain pack %s: %w", packName, err))
		}
		for _, capability := range pack.Capabilities() {
			name := strings.TrimSpace(capability.Name)
			if name == "" {
				continue
			}
			action := capability.Action
			if existing, ok := capabilities[name]; ok && existing != action {
				errs = append(errs, fmt.Errorf("domain capability %s maps to both %s and %s", name, existing, action))
				continue
			}
			capabilities[name] = action
		}
		for _, strategy := range pack.StrategyPatterns() {
			name := strings.TrimSpace(strategy.Name)
			if name == "" {
				continue
			}
			if !patternNames[name] {
				errs = append(errs, fmt.Errorf("domain pack %s references unknown strategy pattern %s", pack.Name(), name))
			}
		}
	}
	return capabilities, errs
}

func validateSemanticActions(manifests []toolchain.Manifest, catalog []toolchain.BuiltInTool, knownCapabilities map[string]bool, domainCapabilities map[string]compiler.SemanticAction) []error {
	var errs []error
	capabilitiesByAction := map[compiler.SemanticAction]map[string]bool{}
	for _, action := range compiler.DeclaredSemanticActions() {
		capabilities, ok := compiler.CapabilitiesForAction(action)
		if !ok || len(capabilities) == 0 {
			errs = append(errs, fmt.Errorf("semantic action %s has no declared capabilities", action))
			continue
		}
		capabilitiesByAction[action] = map[string]bool{}
		for _, capability := range capabilities {
			normalized := strings.TrimSpace(capability)
			if normalized == "" {
				errs = append(errs, fmt.Errorf("semantic action %s has an empty capability", action))
				continue
			}
			capabilitiesByAction[action][normalized] = true
			if !knownCapabilities[normalized] {
				errs = append(errs, fmt.Errorf("semantic action %s capability %s is not provided by domain packs, manifests, or built-in tools", action, normalized))
			}
		}
		if _, err := compiler.NewCompiler().Compile(compiler.CompileRequest{
			Action:      action,
			Manifests:   manifests,
			ToolCatalog: catalog,
		}); err != nil {
			errs = append(errs, fmt.Errorf("semantic action %s does not compile with built-in manifests: %w", action, err))
		}
	}
	for capability, action := range domainCapabilities {
		if !capabilitiesByAction[action][capability] {
			errs = append(errs, fmt.Errorf("domain capability %s declares action %s but compiler does not map that action to the capability", capability, action))
		}
	}
	sort.Slice(errs, func(i, j int) bool {
		return errs[i].Error() < errs[j].Error()
	})
	return errs
}

func domainCapabilityNames(capabilities map[string]compiler.SemanticAction) map[string]bool {
	names := map[string]bool{}
	for name := range capabilities {
		names[name] = true
	}
	return names
}

func mergeCapabilitySets(sets ...map[string]bool) map[string]bool {
	merged := map[string]bool{}
	for _, set := range sets {
		for name := range set {
			merged[name] = true
		}
	}
	return merged
}
