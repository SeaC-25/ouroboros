package goal

import (
	"errors"
	"fmt"
)

// ensureInitialized initializes the runtime state if needed.
func (s *GoalStack) ensureInitialized() {
	if s.queue == nil {
		s.queue = NewPriorityQueue()
	}
	if s.depGraph == nil {
		s.depGraph = NewDependencyGraph()
	}
	if s.goals == nil {
		s.goals = make(map[GoalID]*GoalNode)
	}
	if s.completed == nil {
		s.completed = make(map[GoalID]bool)
	}
	if s.activeGoals == nil {
		s.activeGoals = make(map[GoalID]bool)
	}
}

func (s *GoalStack) Push(goal GoalNode) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureInitialized()
	if goal.ID == "" {
		return errors.New("goal ID is required")
	}
	if _, exists := s.goals[goal.ID]; exists {
		return errors.New("goal already exists")
	}

	// Initialize goal state
	if goal.Status == "" {
		goal.Status = GoalStatusPending
	}
	if goal.Priority == 0 {
		goal.Priority = PriorityMedium
	}

	// Store goal
	goalCopy := goal
	s.goals[goal.ID] = &goalCopy

	// Add to dependency graph
	s.depGraph.AddGoal(goal.ID)
	for _, dep := range goal.Dependencies {
		if _, exists := s.goals[dep]; !exists {
			return fmt.Errorf("dependency goal %s not found", dep)
		}
		if err := s.depGraph.AddDependency(goal.ID, dep); err != nil {
			return fmt.Errorf("add dependency: %w", err)
		}
	}

	// Check if goal is ready (all dependencies met)
	if s.isGoalReady(goal.ID) {
		s.queue.Enqueue(goal)
	}

	return nil
}

func (s *GoalStack) Pop() (GoalNode, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	goal, ok := s.queue.Dequeue()
	if !ok {
		return GoalNode{}, false
	}

	// Mark as active
	if stored := s.goals[goal.ID]; stored != nil {
		stored.Status = GoalStatusActive
		s.activeGoals[goal.ID] = true
		goal = *stored
	}

	return goal, true
}

func (s *GoalStack) Peek() (GoalNode, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	goal, ok := s.queue.Peek()
	if !ok {
		return GoalNode{}, false
	}

	return goal, true
}

func (s *GoalStack) Complete(id GoalID) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	goal, exists := s.goals[id]
	if !exists {
		return errors.New("goal not found")
	}

	goal.Status = GoalStatusComplete
	goal.Progress = 1.0
	s.completed[id] = true
	delete(s.activeGoals, id)

	// Check if any dependent goals are now ready
	s.enqueueReadyDependents(id)

	return nil
}

func (s *GoalStack) UpdateProgress(id GoalID, progress float64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if progress < 0.0 || progress > 1.0 {
		return errors.New("progress must be between 0.0 and 1.0")
	}

	goal, exists := s.goals[id]
	if !exists {
		return errors.New("goal not found")
	}

	goal.Progress = progress
	return nil
}

func (s *GoalStack) Completion() CompletionSnapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()

	snapshot := CompletionSnapshot{
		Total:     len(s.goals),
		Completed: len(s.completed),
		ByType:    make(map[GoalType]TypeCompletion),
	}

	// Calculate aggregate progress
	totalProgress := 0.0
	byType := make(map[GoalType]*TypeCompletion)

	for _, goal := range s.goals {
		totalProgress += goal.Progress

		if _, exists := byType[goal.Type]; !exists {
			byType[goal.Type] = &TypeCompletion{}
		}
		tc := byType[goal.Type]
		tc.Total++
		tc.Progress += goal.Progress
		if s.completed[goal.ID] {
			tc.Completed++
		}
	}

	if len(s.goals) > 0 {
		snapshot.Progress = totalProgress / float64(len(s.goals))
	}

	for goalType, tc := range byType {
		snapshot.ByType[goalType] = *tc
	}

	return snapshot
}

func (s *GoalStack) isGoalReady(id GoalID) bool {
	deps := s.depGraph.Dependencies(id)
	for _, dep := range deps {
		if !s.completed[dep] {
			return false
		}
	}
	return true
}

func (s *GoalStack) enqueueReadyDependents(completedID GoalID) {
	ready := s.depGraph.ReadyGoals(s.completed)
	for _, goalID := range ready {
		if s.completed[goalID] || s.activeGoals[goalID] {
			continue
		}
		// Check if this goal is already in the queue
		if goal, exists := s.goals[goalID]; exists {
			// Only enqueue if it's not already queued
			if goal.Status == GoalStatusPending {
				s.queue.Enqueue(*goal)
			}
		}
	}
}
