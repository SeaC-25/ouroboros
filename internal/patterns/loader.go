package patterns

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

func LoadDir(path string) ([]Pattern, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, fmt.Errorf("read pattern directory: %w", err)
	}
	sort.Slice(entries, func(i int, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})

	patterns := []Pattern{}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yaml") {
			continue
		}
		pattern, err := LoadFile(filepath.Join(path, entry.Name()))
		if err != nil {
			return nil, err
		}
		patterns = append(patterns, pattern)
	}
	return patterns, nil
}

func LoadFile(path string) (Pattern, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Pattern{}, fmt.Errorf("read pattern file %s: %w", path, err)
	}
	var pattern Pattern
	if err := yaml.Unmarshal(data, &pattern); err != nil {
		return Pattern{}, fmt.Errorf("parse pattern file %s: %w", path, err)
	}
	if err := Validate(pattern); err != nil {
		return Pattern{}, fmt.Errorf("validate pattern file %s: %w", path, err)
	}
	return pattern, nil
}

func Validate(pattern Pattern) error {
	var errs []error
	if strings.TrimSpace(pattern.Name) == "" {
		errs = append(errs, errors.New("pattern name is required"))
	}
	if pattern.MaxSteps <= 0 {
		errs = append(errs, errors.New("pattern max_steps must be positive"))
	}
	if len(pattern.Nodes) == 0 {
		errs = append(errs, errors.New("pattern must define nodes"))
	}

	nodeIDs := map[string]Node{}
	hasTerminalResult := false
	for _, node := range pattern.Nodes {
		if strings.TrimSpace(node.ID) == "" {
			errs = append(errs, errors.New("node id is required"))
			continue
		}
		if _, exists := nodeIDs[node.ID]; exists {
			errs = append(errs, fmt.Errorf("duplicate node id: %s", node.ID))
		}
		nodeIDs[node.ID] = node
		if node.Terminal && node.Kind == "result" {
			hasTerminalResult = true
		}
	}
	if !hasTerminalResult {
		errs = append(errs, errors.New("pattern must include at least one terminal result node"))
	}

	conditions := map[string]bool{}
	for _, condition := range pattern.Conditions {
		if strings.TrimSpace(condition.Name) == "" {
			errs = append(errs, errors.New("condition name is required"))
			continue
		}
		conditions[condition.Name] = true
	}

	for _, edge := range pattern.Edges {
		if _, ok := nodeIDs[edge.From]; !ok {
			errs = append(errs, fmt.Errorf("edge references missing from node: %s", edge.From))
		}
		if _, ok := nodeIDs[edge.To]; !ok {
			errs = append(errs, fmt.Errorf("edge references missing to node: %s", edge.To))
		}
		if edge.Condition != "" && !conditions[edge.Condition] {
			errs = append(errs, fmt.Errorf("edge references missing condition: %s", edge.Condition))
		}
	}

	return errors.Join(errs...)
}
