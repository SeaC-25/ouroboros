package goal

import (
	"errors"
	"sync"
)

// DependencyGraph represents a directed acyclic graph (DAG) of goal dependencies.
type DependencyGraph struct {
	mu           sync.RWMutex
	goals        map[GoalID]bool
	dependencies map[GoalID][]GoalID // goal -> list of goals it depends on
	dependents   map[GoalID][]GoalID // goal -> list of goals that depend on it
}

func NewDependencyGraph() *DependencyGraph {
	return &DependencyGraph{
		goals:        make(map[GoalID]bool),
		dependencies: make(map[GoalID][]GoalID),
		dependents:   make(map[GoalID][]GoalID),
	}
}

func (dg *DependencyGraph) AddGoal(id GoalID) {
	dg.mu.Lock()
	defer dg.mu.Unlock()

	if !dg.goals[id] {
		dg.goals[id] = true
		dg.dependencies[id] = []GoalID{}
		dg.dependents[id] = []GoalID{}
	}
}

func (dg *DependencyGraph) AddDependency(goal GoalID, dependsOn GoalID) error {
	dg.mu.Lock()
	defer dg.mu.Unlock()

	if !dg.goals[goal] {
		return errors.New("goal not found")
	}
	if !dg.goals[dependsOn] {
		return errors.New("dependency goal not found")
	}

	// Check for cycle
	if dg.wouldCreateCycle(goal, dependsOn) {
		return errors.New("adding this dependency would create a cycle")
	}

	// Add the dependency
	dg.dependencies[goal] = append(dg.dependencies[goal], dependsOn)
	dg.dependents[dependsOn] = append(dg.dependents[dependsOn], goal)

	return nil
}

func (dg *DependencyGraph) wouldCreateCycle(from GoalID, to GoalID) bool {
	// If 'from' depends on 'to', and 'to' can reach 'from' through existing dependencies,
	// then adding this edge would create a cycle.
	visited := make(map[GoalID]bool)
	return dg.canReach(to, from, visited)
}

func (dg *DependencyGraph) canReach(from GoalID, target GoalID, visited map[GoalID]bool) bool {
	if from == target {
		return true
	}
	if visited[from] {
		return false
	}
	visited[from] = true

	for _, dep := range dg.dependencies[from] {
		if dg.canReach(dep, target, visited) {
			return true
		}
	}
	return false
}

func (dg *DependencyGraph) ReadyGoals(completed map[GoalID]bool) []GoalID {
	dg.mu.RLock()
	defer dg.mu.RUnlock()

	ready := []GoalID{}
	for goal := range dg.goals {
		if completed[goal] {
			continue
		}

		allDepsMet := true
		for _, dep := range dg.dependencies[goal] {
			if !completed[dep] {
				allDepsMet = false
				break
			}
		}

		if allDepsMet {
			ready = append(ready, goal)
		}
	}

	return ready
}

func (dg *DependencyGraph) TopologicalOrder() ([]GoalID, error) {
	dg.mu.RLock()
	defer dg.mu.RUnlock()

	// Kahn's algorithm
	inDegree := make(map[GoalID]int)
	for goal := range dg.goals {
		inDegree[goal] = len(dg.dependencies[goal])
	}

	queue := []GoalID{}
	for goal, degree := range inDegree {
		if degree == 0 {
			queue = append(queue, goal)
		}
	}

	result := []GoalID{}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		result = append(result, current)

		for _, dependent := range dg.dependents[current] {
			inDegree[dependent]--
			if inDegree[dependent] == 0 {
				queue = append(queue, dependent)
			}
		}
	}

	if len(result) != len(dg.goals) {
		return nil, errors.New("cycle detected in dependency graph")
	}

	return result, nil
}

func (dg *DependencyGraph) Dependencies(goal GoalID) []GoalID {
	dg.mu.RLock()
	defer dg.mu.RUnlock()

	deps := dg.dependencies[goal]
	result := make([]GoalID, len(deps))
	copy(result, deps)
	return result
}
