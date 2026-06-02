package toolchain

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

type AuthoredToolSpec struct {
	Name             string                 `yaml:"name" json:"name"`
	Version          string                 `yaml:"version" json:"version"`
	Capabilities     []string               `yaml:"capabilities" json:"capabilities"`
	Runner           string                 `yaml:"runner,omitempty" json:"runner,omitempty"`
	RunCommand       []string               `yaml:"run_command,omitempty" json:"run_command,omitempty"`
	RawScript        string                 `yaml:"raw_script,omitempty" json:"raw_script,omitempty"`
	Parser           string                 `yaml:"parser" json:"parser"`
	Adapter          AuthoredToolAdapter    `yaml:"adapter" json:"adapter"`
	Inputs           []AuthoredToolInput    `yaml:"inputs" json:"inputs"`
	ExpectedEvidence []AuthoredToolEvidence `yaml:"expected_evidence" json:"expected_evidence"`
	Safety           AuthoredToolSafety     `yaml:"safety" json:"safety"`
	Dependencies     []Dependency           `yaml:"dependencies,omitempty" json:"dependencies,omitempty"`
}

type AuthoredToolAdapter struct {
	ID        string `yaml:"id" json:"id"`
	Operation string `yaml:"operation" json:"operation"`
}

type AuthoredToolInput struct {
	Name     string `yaml:"name" json:"name"`
	Type     string `yaml:"type" json:"type"`
	Required bool   `yaml:"required,omitempty" json:"required,omitempty"`
}

type AuthoredToolEvidence struct {
	Type   string   `yaml:"type" json:"type"`
	Fields []string `yaml:"fields" json:"fields"`
}

type AuthoredToolSafety struct {
	TargetPolicy              string `yaml:"target_policy" json:"target_policy"`
	Isolation                 string `yaml:"isolation" json:"isolation"`
	AllowTargetPolicyDisabled bool   `yaml:"allow_target_policy_disabled,omitempty" json:"allow_target_policy_disabled,omitempty"`
	AllowIsolationDisabled    bool   `yaml:"allow_isolation_disabled,omitempty" json:"allow_isolation_disabled,omitempty"`
}

func LoadAuthoredToolSpec(path string) (AuthoredToolSpec, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return AuthoredToolSpec{}, fmt.Errorf("read authored tool spec %s: %w", path, err)
	}
	var spec AuthoredToolSpec
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(&spec); err != nil {
		return AuthoredToolSpec{}, fmt.Errorf("parse authored tool spec %s: %w", path, err)
	}
	return spec, nil
}

func ValidateAuthoredToolSpec(spec AuthoredToolSpec) error {
	var errs []error
	manifest := authoredToolManifest(spec)
	if err := ValidateManifest(manifest); err != nil {
		errs = append(errs, err)
	}
	if strings.TrimSpace(spec.RawScript) != "" {
		errs = append(errs, errors.New("raw_script is not supported for authored native tools"))
	}
	if len(spec.RunCommand) > 0 {
		errs = append(errs, errors.New("run_command is not supported for authored native tools"))
	}
	if runner := strings.TrimSpace(spec.Runner); runner != "" && runner != "native" {
		errs = append(errs, fmt.Errorf("runner must be native for authored tools, got %s", runner))
	}
	errs = append(errs, validateAuthoredToolAdapter(spec.Adapter)...)
	errs = append(errs, validateAuthoredToolRegistryContracts(spec, DefaultAuthoredToolRegistry())...)
	errs = append(errs, validateAuthoredToolInputs(spec.Inputs)...)
	errs = append(errs, validateAuthoredToolEvidence(spec.ExpectedEvidence)...)
	errs = append(errs, validateAuthoredToolSafety(spec.Safety)...)
	errs = append(errs, validateAuthoredToolDependencies(spec.Dependencies)...)
	return errors.Join(errs...)
}

func AuthoredToolToManifest(spec AuthoredToolSpec) (Manifest, error) {
	if err := ValidateAuthoredToolSpec(spec); err != nil {
		return Manifest{}, err
	}
	return authoredToolManifest(spec), nil
}

func authoredToolManifest(spec AuthoredToolSpec) Manifest {
	return Manifest{
		Name:         strings.TrimSpace(spec.Name),
		Version:      strings.TrimSpace(spec.Version),
		Capabilities: trimmedStrings(spec.Capabilities),
		Runner:       "native",
		Parser:       strings.TrimSpace(spec.Parser),
		Healthcheck:  "built_in",
		Dependencies: spec.Dependencies,
	}
}

func validateAuthoredToolAdapter(adapter AuthoredToolAdapter) []error {
	var errs []error
	if strings.TrimSpace(adapter.ID) == "" {
		errs = append(errs, errors.New("adapter id is required"))
	}
	if strings.TrimSpace(adapter.Operation) == "" {
		errs = append(errs, errors.New("adapter operation is required"))
	}
	return errs
}

func validateAuthoredToolRegistryContracts(spec AuthoredToolSpec, registry AuthoredToolRegistry) []error {
	var errs []error
	parserID := strings.TrimSpace(spec.Parser)
	if parserID != "" && !registry.ParserExists(parserID) {
		errs = append(errs, fmt.Errorf("parser is not registered: %s", parserID))
	}

	adapterID := strings.TrimSpace(spec.Adapter.ID)
	operationID := strings.TrimSpace(spec.Adapter.Operation)
	if adapterID == "" || operationID == "" {
		return errs
	}
	if !registry.AdapterExists(adapterID) {
		errs = append(errs, fmt.Errorf("adapter id is not registered: %s", adapterID))
		return errs
	}
	operation, ok := registry.AdapterOperation(adapterID, operationID)
	if !ok {
		errs = append(errs, fmt.Errorf("adapter operation is not registered: %s/%s", adapterID, operationID))
		return errs
	}
	if parserID != "" && len(operation.SupportedParsers) > 0 && !containsTrimmed(operation.SupportedParsers, parserID) {
		errs = append(errs, fmt.Errorf("parser %s is not supported by adapter operation %s", parserID, operation.ID))
	}
	errs = append(errs, validateAuthoredToolOperationInputs(spec.Inputs, operation)...)
	errs = append(errs, validateAuthoredToolOperationDependencies(spec.Dependencies, operation)...)
	return errs
}

func validateAuthoredToolOperationInputs(inputs []AuthoredToolInput, operation AuthoredToolOperationDefinition) []error {
	var errs []error
	byName := make(map[string]AuthoredToolInput, len(inputs))
	for _, input := range inputs {
		name := strings.TrimSpace(input.Name)
		if name == "" {
			continue
		}
		byName[name] = input
	}
	for _, contract := range operation.RequiredInputs {
		name := strings.TrimSpace(contract.Name)
		if name == "" {
			continue
		}
		input, ok := byName[name]
		if !ok {
			errs = append(errs, fmt.Errorf("adapter operation %s requires input %s", operation.ID, name))
			continue
		}
		if !input.Required {
			errs = append(errs, fmt.Errorf("adapter operation %s requires input %s to be marked required", operation.ID, name))
		}
		inputType := strings.TrimSpace(input.Type)
		if len(contract.Types) > 0 && !containsTrimmed(contract.Types, inputType) {
			errs = append(errs, fmt.Errorf("input %s type must be one of %s", name, strings.Join(contract.Types, ", ")))
		}
	}
	return errs
}

func validateAuthoredToolOperationDependencies(dependencies []Dependency, operation AuthoredToolOperationDefinition) []error {
	if len(operation.DependencyNames) == 0 {
		return nil
	}
	declared := make(map[string]bool, len(dependencies))
	for _, dependency := range dependencies {
		name := strings.TrimSpace(dependency.Name)
		if name != "" {
			declared[name] = true
		}
	}
	for _, dependencyName := range operation.DependencyNames {
		if declared[strings.TrimSpace(dependencyName)] {
			return nil
		}
	}
	return []error{fmt.Errorf("adapter operation %s requires one of dependencies: %s", operation.ID, strings.Join(operation.DependencyNames, ", "))}
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

func validateAuthoredToolInputs(inputs []AuthoredToolInput) []error {
	if len(inputs) == 0 {
		return []error{errors.New("inputs must declare at least one input")}
	}
	var errs []error
	seen := map[string]int{}
	for i, input := range inputs {
		name := strings.TrimSpace(input.Name)
		if name == "" {
			errs = append(errs, fmt.Errorf("inputs %d name is required", i))
		} else if previous, ok := seen[name]; ok {
			errs = append(errs, fmt.Errorf("inputs %d name duplicates inputs %d: %s", i, previous, name))
		} else {
			seen[name] = i
		}
		if strings.TrimSpace(input.Type) == "" {
			errs = append(errs, fmt.Errorf("inputs %d type is required", i))
		}
	}
	return errs
}

func validateAuthoredToolEvidence(evidence []AuthoredToolEvidence) []error {
	if len(evidence) == 0 {
		return []error{errors.New("expected_evidence must declare at least one evidence type")}
	}
	var errs []error
	for i, item := range evidence {
		if strings.TrimSpace(item.Type) == "" {
			errs = append(errs, fmt.Errorf("expected_evidence %d type is required", i))
		}
		fields := trimmedStrings(item.Fields)
		if len(fields) == 0 {
			errs = append(errs, fmt.Errorf("expected_evidence %d fields are required", i))
		}
	}
	return errs
}

func validateAuthoredToolDependencies(dependencies []Dependency) []error {
	var errs []error
	for i, dependency := range dependencies {
		if len(dependency.HealthCommand) > 0 {
			errs = append(errs, fmt.Errorf("dependencies %d health_command is not supported for authored native tools", i))
		}
		if len(dependency.InstallCommand) > 0 {
			errs = append(errs, fmt.Errorf("dependencies %d install_command is not supported for authored native tools", i))
		}
	}
	return errs
}

func validateAuthoredToolSafety(safety AuthoredToolSafety) []error {
	var errs []error
	switch strings.TrimSpace(safety.TargetPolicy) {
	case "":
		errs = append(errs, errors.New("safety target_policy is required"))
	case "enforce":
	case "disabled":
		if !safety.AllowTargetPolicyDisabled {
			errs = append(errs, errors.New("safety target_policy disabled requires allow_target_policy_disabled"))
		}
	default:
		errs = append(errs, fmt.Errorf("safety target_policy is unsupported: %s", safety.TargetPolicy))
	}
	switch strings.TrimSpace(safety.Isolation) {
	case "":
		errs = append(errs, errors.New("safety isolation is required"))
	case "process", "container":
	case "disabled":
		if !safety.AllowIsolationDisabled {
			errs = append(errs, errors.New("safety isolation disabled requires allow_isolation_disabled"))
		}
	default:
		errs = append(errs, fmt.Errorf("safety isolation is unsupported: %s", safety.Isolation))
	}
	return errs
}
