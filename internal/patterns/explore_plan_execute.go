package patterns

import (
	"fmt"
)

// ExplorePlanExecute implements the EPE pattern: deliberate explore, plan, execute loop.
// It separates signal gathering, planning, and execution into distinct phases.
type ExplorePlanExecute struct {
	pattern       Pattern
	currentNode   string
	stepCount     int
	explorations  []Exploration
	plan          *Plan
	executions    []Execution
}

// NewExplorePlanExecute creates an EPE executor from a pattern definition.
func NewExplorePlanExecute(pattern Pattern) (*ExplorePlanExecute, error) {
	if pattern.Name != "explore_plan_execute" {
		return nil, fmt.Errorf("pattern name mismatch: expected explore_plan_execute, got %s", pattern.Name)
	}
	if err := Validate(pattern); err != nil {
		return nil, fmt.Errorf("invalid pattern: %w", err)
	}

	return &ExplorePlanExecute{
		pattern:     pattern,
		currentNode: "explore",
	}, nil
}

// Step advances the EPE state machine by one step.
func (epe *ExplorePlanExecute) Step(results ConditionResults) (Transition, error) {
	transition, err := Next(epe.pattern, epe.currentNode, results)
	if err != nil {
		return Transition{}, err
	}

	epe.currentNode = transition.To
	epe.stepCount++

	return transition, nil
}

// EvaluateConditions assesses the current state and returns condition results.
func (epe *ExplorePlanExecute) EvaluateConditions(ctx ExecutionContext) ConditionResults {
	results := make(ConditionResults)

	switch epe.currentNode {
	case "explore":
		// Check if enough signal has been gathered
		if len(epe.explorations) > 0 && epe.explorations[len(epe.explorations)-1].SignalFound {
			results["signal_found"] = true
		}

	case "plan":
		// Check if plan is ready
		if epe.plan != nil && epe.plan.Ready {
			results["plan_ready"] = true
		}

	case "execute":
		// Always transition to evaluate after execution
		results["always"] = true

	case "evaluate":
		// Determine next phase based on results
		if ctx.ObjectiveMet {
			results["objective_met"] = true
		} else if epe.needsMoreSignal(ctx) {
			results["needs_more_signal"] = true
		} else if epe.stepCount >= epe.pattern.MaxSteps {
			results["exhausted"] = true
		}
	}

	return results
}

// needsMoreSignal checks if exploration phase should repeat.
func (epe *ExplorePlanExecute) needsMoreSignal(ctx ExecutionContext) bool {
	// If last execution produced no useful facts, need more exploration
	if len(epe.executions) > 0 {
		lastExec := epe.executions[len(epe.executions)-1]
		return lastExec.FactsProduced == 0
	}
	return false
}

// RecordExploration adds an exploration result.
func (epe *ExplorePlanExecute) RecordExploration(exploration Exploration) {
	epe.explorations = append(epe.explorations, exploration)
}

// SetPlan sets the current execution plan.
func (epe *ExplorePlanExecute) SetPlan(plan Plan) {
	epe.plan = &plan
}

// RecordExecution adds an execution result.
func (epe *ExplorePlanExecute) RecordExecution(execution Execution) {
	epe.executions = append(epe.executions, execution)
}

// CurrentNode returns the current state node.
func (epe *ExplorePlanExecute) CurrentNode() string {
	return epe.currentNode
}

// IsTerminal checks if the current node is terminal.
func (epe *ExplorePlanExecute) IsTerminal() bool {
	for _, node := range epe.pattern.Nodes {
		if node.ID == epe.currentNode && node.Terminal {
			return true
		}
	}
	return false
}

// Progress returns the current execution progress.
func (epe *ExplorePlanExecute) Progress() Progress {
	return Progress{
		CurrentNode:   epe.currentNode,
		StepCount:     epe.stepCount,
		MaxSteps:      epe.pattern.MaxSteps,
		IsTerminal:    epe.IsTerminal(),
		MutationCount: 0, // EPE doesn't use mutations
	}
}

// Reset resets the EPE to initial state.
func (epe *ExplorePlanExecute) Reset() {
	epe.currentNode = "explore"
	epe.stepCount = 0
	epe.explorations = nil
	epe.plan = nil
	epe.executions = nil
}

// GetPlan returns the current plan if available.
func (epe *ExplorePlanExecute) GetPlan() *Plan {
	return epe.plan
}

// ExplorationPhase returns the current exploration phase.
func (epe *ExplorePlanExecute) ExplorationPhase() string {
	switch epe.currentNode {
	case "explore":
		return "gathering_signals"
	case "plan":
		return "planning"
	case "execute":
		return "executing"
	case "evaluate":
		return "evaluating"
	default:
		return "terminal"
	}
}

// Exploration represents an exploration result.
type Exploration struct {
	Description string
	SignalFound bool
	Facts       []string
}

// Plan represents an execution plan.
type Plan struct {
	Description string
	Steps       []string
	Ready       bool
}

// Execution represents an execution result.
type Execution struct {
	PlanStep      string
	Success       bool
	FactsProduced int
	Evidence      []string
}
