package toolchain

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

type Manifest struct {
	Path          string       `yaml:"-"`
	Name          string       `yaml:"name"`
	Version       string       `yaml:"version"`
	Capabilities  []string     `yaml:"capabilities"`
	Runner        string       `yaml:"runner"`
	RunCommand    []string     `yaml:"run_command,omitempty"`
	RunTimeoutMS  int          `yaml:"run_timeout_ms,omitempty"`
	Parser        string       `yaml:"parser"`
	Healthcheck   string       `yaml:"healthcheck"`
	HealthCommand []string     `yaml:"health_command,omitempty"`
	Dependencies  []Dependency `yaml:"dependencies,omitempty"`
}

type Dependency struct {
	Name           string   `yaml:"name"`
	Kind           string   `yaml:"kind,omitempty"`
	Version        string   `yaml:"version,omitempty"`
	LatestVersion  string   `yaml:"latest_version,omitempty"`
	Source         string   `yaml:"source,omitempty"`
	ImageDigest    string   `yaml:"image_digest,omitempty"`
	MetadataSource string   `yaml:"metadata_source,omitempty"`
	DependsOn      []string `yaml:"depends_on,omitempty"`
	Optional       bool     `yaml:"optional,omitempty"`
	HealthCommand  []string `yaml:"health_command,omitempty"`
	InstallHint    string   `yaml:"install_hint,omitempty"`
	InstallCommand []string `yaml:"install_command,omitempty"`
}

func LoadManifests(root string) ([]Manifest, error) {
	paths, err := filepath.Glob(filepath.Join(root, "*", "manifest.yaml"))
	if err != nil {
		return nil, fmt.Errorf("glob manifests: %w", err)
	}
	sort.Strings(paths)

	manifests := make([]Manifest, 0, len(paths))
	for _, path := range paths {
		manifest, err := LoadManifest(path)
		if err != nil {
			return nil, err
		}
		manifests = append(manifests, manifest)
	}
	return manifests, nil
}

func LoadManifest(path string) (Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Manifest{}, fmt.Errorf("read manifest %s: %w", path, err)
	}
	var manifest Manifest
	if err := yaml.Unmarshal(data, &manifest); err != nil {
		return Manifest{}, fmt.Errorf("parse manifest %s: %w", path, err)
	}
	manifest.Path = path
	if err := ValidateManifest(manifest); err != nil {
		return Manifest{}, fmt.Errorf("validate manifest %s: %w", path, err)
	}
	return manifest, nil
}

func ValidateManifest(manifest Manifest) error {
	var errs []error
	if strings.TrimSpace(manifest.Name) == "" {
		errs = append(errs, errors.New("manifest name is required"))
	}
	if strings.TrimSpace(manifest.Version) == "" {
		errs = append(errs, errors.New("manifest version lock is required"))
	}
	if len(manifest.Capabilities) == 0 {
		errs = append(errs, errors.New("manifest must declare at least one capability"))
	}
	for i, capability := range manifest.Capabilities {
		if strings.TrimSpace(capability) == "" {
			errs = append(errs, fmt.Errorf("capability %d is empty", i))
		}
	}
	if strings.TrimSpace(manifest.Runner) == "" {
		errs = append(errs, errors.New("manifest runner is required"))
	}
	if strings.TrimSpace(manifest.Runner) == "command" {
		if len(manifest.RunCommand) == 0 {
			errs = append(errs, errors.New("command runner requires run_command"))
		}
		for i, part := range manifest.RunCommand {
			if strings.TrimSpace(part) == "" {
				errs = append(errs, fmt.Errorf("run_command %d is empty", i))
			}
		}
	}
	if manifest.RunTimeoutMS < 0 {
		errs = append(errs, errors.New("run_timeout_ms must be non-negative"))
	}
	if strings.TrimSpace(manifest.Parser) == "" {
		errs = append(errs, errors.New("manifest parser is required"))
	}
	if strings.TrimSpace(manifest.Healthcheck) == "" {
		errs = append(errs, errors.New("manifest healthcheck is required"))
	}
	if strings.TrimSpace(manifest.Healthcheck) == "exec" {
		if len(manifest.HealthCommand) == 0 {
			errs = append(errs, errors.New("exec healthcheck requires health_command"))
		}
		for i, part := range manifest.HealthCommand {
			if strings.TrimSpace(part) == "" {
				errs = append(errs, fmt.Errorf("health_command %d is empty", i))
			}
		}
	}
	dependencyNames := map[string]int{}
	for i, dependency := range manifest.Dependencies {
		dependencyName := strings.TrimSpace(dependency.Name)
		if dependencyName == "" {
			errs = append(errs, fmt.Errorf("dependency %d name is required", i))
		} else if previous, ok := dependencyNames[dependencyName]; ok {
			errs = append(errs, fmt.Errorf("dependency %d name duplicates dependency %d: %s", i, previous, dependencyName))
		} else {
			dependencyNames[dependencyName] = i
		}
		switch dependencyKind(dependency) {
		case "binary", "container_image":
		default:
			errs = append(errs, fmt.Errorf("dependency %d kind is unsupported: %s", i, dependency.Kind))
		}
		if dependencyKind(dependency) == "container_image" && !dependency.Optional {
			if strings.TrimSpace(dependency.ImageDigest) == "" {
				errs = append(errs, fmt.Errorf("dependency %d image_digest is required for required container_image dependencies", i))
			} else if !strings.HasPrefix(strings.TrimSpace(dependency.ImageDigest), "sha256:") {
				errs = append(errs, fmt.Errorf("dependency %d image_digest must use sha256", i))
			}
		}
		for j, part := range dependency.HealthCommand {
			if strings.TrimSpace(part) == "" {
				errs = append(errs, fmt.Errorf("dependency %d health_command %d is empty", i, j))
			}
		}
		for j, part := range dependency.InstallCommand {
			if strings.TrimSpace(part) == "" {
				errs = append(errs, fmt.Errorf("dependency %d install_command %d is empty", i, j))
			}
		}
		for j, dependencyName := range dependency.DependsOn {
			dependencyName = strings.TrimSpace(dependencyName)
			switch {
			case dependencyName == "":
				errs = append(errs, fmt.Errorf("dependency %d depends_on %d is empty", i, j))
			case dependencyName == strings.TrimSpace(dependency.Name):
				errs = append(errs, fmt.Errorf("dependency %d depends_on itself: %s", i, dependencyName))
			}
		}
	}
	errs = append(errs, validateDependencyReferences(manifest, dependencyNames)...)
	return errors.Join(errs...)
}

func validateDependencyReferences(manifest Manifest, dependencyNames map[string]int) []error {
	var errs []error
	graph := map[string][]string{}
	for i, dependency := range manifest.Dependencies {
		name := strings.TrimSpace(dependency.Name)
		if name == "" {
			continue
		}
		for _, dependsOn := range dependency.DependsOn {
			dependsOn = strings.TrimSpace(dependsOn)
			if dependsOn == "" || dependsOn == name {
				continue
			}
			if _, ok := dependencyNames[dependsOn]; !ok {
				errs = append(errs, fmt.Errorf("dependency %d depends_on %s is not declared", i, dependsOn))
				continue
			}
			graph[name] = append(graph[name], dependsOn)
		}
	}
	if cycle := findDependencyCycle(graph); len(cycle) > 0 {
		errs = append(errs, fmt.Errorf("dependency cycle detected: %s", strings.Join(cycle, " -> ")))
	}
	return errs
}

func findDependencyCycle(graph map[string][]string) []string {
	const (
		unvisited = 0
		visiting  = 1
		visited   = 2
	)
	state := map[string]int{}
	stack := []string{}
	var visit func(string) []string
	visit = func(name string) []string {
		switch state[name] {
		case visiting:
			for i, stacked := range stack {
				if stacked == name {
					return append(append([]string(nil), stack[i:]...), name)
				}
			}
			return []string{name, name}
		case visited:
			return nil
		}
		state[name] = visiting
		stack = append(stack, name)
		for _, next := range graph[name] {
			if cycle := visit(next); len(cycle) > 0 {
				return cycle
			}
		}
		stack = stack[:len(stack)-1]
		state[name] = visited
		return nil
	}

	names := make([]string, 0, len(graph))
	for name := range graph {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if cycle := visit(name); len(cycle) > 0 {
			return cycle
		}
	}
	return nil
}
