package patterns

import (
	"path/filepath"
	"testing"
)

func TestLoadBuiltInPatterns(t *testing.T) {
	loaded, err := LoadDir(filepath.Join("..", "..", "configs", "patterns"))
	if err != nil {
		t.Fatalf("load patterns: %v", err)
	}

	byName := map[string]Pattern{}
	for _, pattern := range loaded {
		byName[pattern.Name] = pattern
	}

	for _, name := range []string{"ralphloop", "explore_plan_execute", "ant_colony"} {
		pattern, ok := byName[name]
		if !ok {
			t.Fatalf("expected built-in pattern %s", name)
		}
		if pattern.MaxSteps <= 0 {
			t.Fatalf("expected max steps for %s", name)
		}
		if len(pattern.Metadata) == 0 {
			t.Fatalf("expected metadata for %s", name)
		}
		if err := Validate(pattern); err != nil {
			t.Fatalf("expected %s to validate: %v", name, err)
		}
	}
}

func TestValidateRejectsMissingNodeReference(t *testing.T) {
	pattern := Pattern{
		Name:     "bad_reference",
		MaxSteps: 3,
		Nodes: []Node{
			{ID: "start", Kind: "action"},
			{ID: "done", Kind: "result", Terminal: true},
		},
		Edges: []Edge{
			{From: "start", To: "missing", Condition: "failed"},
		},
		Conditions: []Condition{
			{Name: "failed", Description: "failed transition"},
		},
	}

	if err := Validate(pattern); err == nil {
		t.Fatalf("expected missing node reference to fail")
	}
}

func TestValidateRequiresTerminalResultNode(t *testing.T) {
	pattern := Pattern{
		Name:     "no_terminal",
		MaxSteps: 3,
		Nodes: []Node{
			{ID: "start", Kind: "action"},
			{ID: "evaluate", Kind: "decision"},
		},
		Edges: []Edge{
			{From: "start", To: "evaluate", Condition: "always"},
		},
		Conditions: []Condition{
			{Name: "always", Description: "unconditional transition"},
		},
	}

	if err := Validate(pattern); err == nil {
		t.Fatalf("expected missing terminal result node to fail")
	}
}
