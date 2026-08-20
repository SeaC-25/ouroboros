package patterns

import (
	"errors"
	"fmt"
)

// RalphLoop implements the RalphLoop pattern: persistent retry with bounded mutation.
// It maintains attempt history and generates mutations from failures.
type RalphLoop struct {
	pattern       Pattern
	currentNode   string
	attemptCount  int
	maxAttempts   int
	mutationBudget int
	mutations     []Mutation
}

// NewRalphLoop creates a RalphLoop executor from a pattern definition.
func NewRalphLoop(pattern Pattern) (*RalphLoop, error) {
	if pattern.Name != "ralphloop" {
		return nil, fmt.Errorf("pattern name mismatch: expected ralphloop, got %s", pattern.Name)
	}
	if err := Validate(pattern); err != nil {
		return nil, fmt.Errorf("invalid pattern: %w", err)
	}

	return &RalphLoop{
		pattern:        pattern,
		currentNode:    "attempt",
		maxAttempts:    pattern.MaxSteps,
		mutationBudget: pattern.MaxSteps / 2,
	}, nil
}

// Step advances the RalphLoop state machine by one step.
func (rl *RalphLoop) Step(results ConditionResults) (Transition, error) {
	transition, err := Next(rl.pattern, rl.currentNode, results)
	if err != nil {
		return Transition{}, err
	}

	rl.currentNode = transition.To

	// Track attempt count
	if transition.To == "attempt" {
		rl.attemptCount++
	}

	return transition, nil
}

// EvaluateConditions assesses the current state and returns condition results.
func (rl *RalphLoop) EvaluateConditions(ctx ExecutionContext) ConditionResults {
	results := make(ConditionResults)

	switch rl.currentNode {
	case "attempt":
		// Always transition to evaluate after attempt
		results["always"] = true

	case "evaluate":
		// Check if objective is solved
		if ctx.ObjectiveMet {
			results["solved"] = true
		} else if ctx.LastAttemptFailed && len(ctx.MutationHints) > 0 {
			results["failed_retryable"] = true
		} else if rl.attemptCount >= rl.maxAttempts {
			results["exhausted"] = true
		}

	case "mutate":
		// Check if mutation budget remains
		if len(rl.mutations) < rl.mutationBudget {
			results["budget_available"] = true
		} else {
			results["exhausted"] = true
		}
	}

	return results
}

// AddMutation records a mutation for the next attempt.
func (rl *RalphLoop) AddMutation(mutation Mutation) error {
	if len(rl.mutations) >= rl.mutationBudget {
		return errors.New("mutation budget exhausted")
	}
	rl.mutations = append(rl.mutations, mutation)
	return nil
}

// CurrentNode returns the current state node.
func (rl *RalphLoop) CurrentNode() string {
	return rl.currentNode
}

// IsTerminal checks if the current node is terminal.
func (rl *RalphLoop) IsTerminal() bool {
	for _, node := range rl.pattern.Nodes {
		if node.ID == rl.currentNode && node.Terminal {
			return true
		}
	}
	return false
}

// Progress returns the current execution progress.
func (rl *RalphLoop) Progress() Progress {
	return Progress{
		CurrentNode:   rl.currentNode,
		StepCount:     rl.attemptCount,
		MaxSteps:      rl.maxAttempts,
		IsTerminal:    rl.IsTerminal(),
		MutationCount: len(rl.mutations),
	}
}

// Reset resets the RalphLoop to initial state.
func (rl *RalphLoop) Reset() {
	rl.currentNode = "attempt"
	rl.attemptCount = 0
	rl.mutations = nil
}

// Mutation represents a strategy mutation.
type Mutation struct {
	Kind        string
	Description string
	Parameters  map[string]interface{}
}

// ExecutionContext provides state for condition evaluation.
type ExecutionContext struct {
	ObjectiveMet     bool
	LastAttemptFailed bool
	MutationHints    []string
	AvailableWorkers int
	FactCount        int
	Evidence         []string
}

// Progress captures execution progress.
type Progress struct {
	CurrentNode   string
	StepCount     int
	MaxSteps      int
	IsTerminal    bool
	MutationCount int
}
