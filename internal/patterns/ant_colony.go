package patterns

import (
	"fmt"
	"sort"
)

// AntColony implements the Ant Colony pattern: parallel path search with shared trail memory.
// It manages multiple workers exploring paths and depositing pheromones to guide convergence.
type AntColony struct {
	pattern       Pattern
	currentNode   string
	stepCount     int
	trails        map[string]*Trail
	workers       []*Worker
	maxWorkers    int
	explorations  int
}

// NewAntColony creates an AntColony executor from a pattern definition.
func NewAntColony(pattern Pattern) (*AntColony, error) {
	if pattern.Name != "ant_colony" {
		return nil, fmt.Errorf("pattern name mismatch: expected ant_colony, got %s", pattern.Name)
	}
	if err := Validate(pattern); err != nil {
		return nil, fmt.Errorf("invalid pattern: %w", err)
	}

	return &AntColony{
		pattern:     pattern,
		currentNode: "spawn",
		trails:      make(map[string]*Trail),
		maxWorkers:  4, // Default worker pool size
	}, nil
}

// Step advances the AntColony state machine by one step.
func (ac *AntColony) Step(results ConditionResults) (Transition, error) {
	transition, err := Next(ac.pattern, ac.currentNode, results)
	if err != nil {
		return Transition{}, err
	}

	ac.currentNode = transition.To
	ac.stepCount++

	return transition, nil
}

// EvaluateConditions assesses the current state and returns condition results.
func (ac *AntColony) EvaluateConditions(ctx ExecutionContext) ConditionResults {
	results := make(ConditionResults)

	switch ac.currentNode {
	case "spawn":
		// Check if workers are available
		if len(ac.workers) < ac.maxWorkers && ctx.AvailableWorkers > 0 {
			results["workers_available"] = true
		}

	case "explore_path":
		// Check if exploration found useful signal
		if ac.explorations > 0 {
			results["signal_found"] = true
		}

	case "deposit":
		// Always transition to converge after deposit
		results["always"] = true

	case "converge":
		// Determine if trail is strong enough to exploit
		strongestTrail := ac.GetStrongestTrail()
		if strongestTrail != nil && strongestTrail.Strength > 0.7 {
			results["strong_trail"] = true
		} else if strongestTrail != nil && strongestTrail.Strength > 0.0 {
			results["weak_trail"] = true
		} else if ac.stepCount >= ac.pattern.MaxSteps {
			results["exhausted"] = true
		}

	case "exploit_path":
		// Check if objective is met
		if ctx.ObjectiveMet {
			results["objective_met"] = true
		}
	}

	return results
}

// SpawnWorker creates a new worker to explore paths.
func (ac *AntColony) SpawnWorker(workerID string) *Worker {
	worker := &Worker{
		ID:     workerID,
		Status: "idle",
	}
	ac.workers = append(ac.workers, worker)
	return worker
}

// DepositPheromone adds pheromone to a trail based on exploration results.
func (ac *AntColony) DepositPheromone(pathID string, strength float64, evidence []string) {
	trail, exists := ac.trails[pathID]
	if !exists {
		trail = &Trail{
			PathID:   pathID,
			Strength: 0.0,
		}
		ac.trails[pathID] = trail
	}

	// Add pheromone (capped at 1.0)
	trail.Strength += strength
	if trail.Strength > 1.0 {
		trail.Strength = 1.0
	}

	trail.Evidence = append(trail.Evidence, evidence...)
	trail.Visits++
	ac.explorations++
}

// GetStrongestTrail returns the trail with highest pheromone strength.
func (ac *AntColony) GetStrongestTrail() *Trail {
	if len(ac.trails) == 0 {
		return nil
	}

	var strongest *Trail
	for _, trail := range ac.trails {
		if strongest == nil || trail.Strength > strongest.Strength {
			strongest = trail
		}
	}
	return strongest
}

// GetRankedTrails returns all trails sorted by strength.
func (ac *AntColony) GetRankedTrails() []*Trail {
	trails := make([]*Trail, 0, len(ac.trails))
	for _, trail := range ac.trails {
		trails = append(trails, trail)
	}

	sort.Slice(trails, func(i, j int) bool {
		return trails[i].Strength > trails[j].Strength
	})

	return trails
}

// EvaporatePheromones reduces all trail strengths over time.
func (ac *AntColony) EvaporatePheromones(rate float64) {
	for _, trail := range ac.trails {
		trail.Strength *= (1.0 - rate)
		if trail.Strength < 0.01 {
			trail.Strength = 0.0
		}
	}
}

// CurrentNode returns the current state node.
func (ac *AntColony) CurrentNode() string {
	return ac.currentNode
}

// IsTerminal checks if the current node is terminal.
func (ac *AntColony) IsTerminal() bool {
	for _, node := range ac.pattern.Nodes {
		if node.ID == ac.currentNode && node.Terminal {
			return true
		}
	}
	return false
}

// Progress returns the current execution progress.
func (ac *AntColony) Progress() Progress {
	return Progress{
		CurrentNode:   ac.currentNode,
		StepCount:     ac.stepCount,
		MaxSteps:      ac.pattern.MaxSteps,
		IsTerminal:    ac.IsTerminal(),
		MutationCount: len(ac.trails), // Use trail count as proxy for diversity
	}
}

// Reset resets the AntColony to initial state.
func (ac *AntColony) Reset() {
	ac.currentNode = "spawn"
	ac.stepCount = 0
	ac.trails = make(map[string]*Trail)
	ac.workers = nil
	ac.explorations = 0
}

// GetTrailSummary provides a summary of trail exploration.
func (ac *AntColony) GetTrailSummary() TrailSummary {
	summary := TrailSummary{
		TotalTrails:   len(ac.trails),
		ActiveWorkers: len(ac.workers),
		Explorations:  ac.explorations,
	}

	if strongest := ac.GetStrongestTrail(); strongest != nil {
		summary.StrongestPath = strongest.PathID
		summary.MaxStrength = strongest.Strength
	}

	return summary
}

// Trail represents a pheromone trail deposited by workers.
type Trail struct {
	PathID   string
	Strength float64
	Evidence []string
	Visits   int
}

// Worker represents a parallel exploration worker.
type Worker struct {
	ID            string
	Status        string
	CurrentPath   string
	FactsFound    int
}

// TrailSummary provides overview of trail state.
type TrailSummary struct {
	TotalTrails   int
	ActiveWorkers int
	Explorations  int
	StrongestPath string
	MaxStrength   float64
}
