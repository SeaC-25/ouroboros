package swarm

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/ouroboros/ouroboros/internal/blackboard"
)

// TaskDispatcher manages task distribution and scheduling.
type TaskDispatcher struct {
	mu             sync.RWMutex
	workerPool     *WorkerPool
	store          *blackboard.MemoryStore
	pendingTasks   []DispatchTask
	activeTasks    map[string]*DispatchTask
	completedTasks []*DispatchTask
	strategy       DispatchStrategy
}

// DispatchTask represents a task with dispatch metadata.
type DispatchTask struct {
	Task       AgentTask
	Priority   int
	Dependencies []string
	Status     TaskStatus
	AssignedWorker string
	SubmittedAt time.Time
	StartedAt   *time.Time
	CompletedAt *time.Time
	Result     *AgentResult
	Error      error
}

type TaskStatus string

const (
	TaskStatusPending   TaskStatus = "pending"
	TaskStatusReady     TaskStatus = "ready"
	TaskStatusDispatched TaskStatus = "dispatched"
	TaskStatusRunning   TaskStatus = "running"
	TaskStatusComplete  TaskStatus = "complete"
	TaskStatusFailed    TaskStatus = "failed"
)

type DispatchStrategy string

const (
	StrategyFIFO        DispatchStrategy = "fifo"
	StrategyPriority    DispatchStrategy = "priority"
	StrategyRoleAware   DispatchStrategy = "role_aware"
	StrategyLoadBalanced DispatchStrategy = "load_balanced"
)

// NewTaskDispatcher creates a new task dispatcher.
func NewTaskDispatcher(pool *WorkerPool, store *blackboard.MemoryStore) *TaskDispatcher {
	return &TaskDispatcher{
		workerPool:   pool,
		store:        store,
		activeTasks:  make(map[string]*DispatchTask),
		strategy:     StrategyPriority,
	}
}

// Dispatch adds a task to the dispatch queue.
func (td *TaskDispatcher) Dispatch(task AgentTask, priority int, dependencies []string) error {
	td.mu.Lock()
	defer td.mu.Unlock()

	dispatchTask := DispatchTask{
		Task:         task,
		Priority:     priority,
		Dependencies: dependencies,
		Status:       TaskStatusPending,
		SubmittedAt:  time.Now(),
	}

	td.pendingTasks = append(td.pendingTasks, dispatchTask)
	return nil
}

// ProcessQueue processes pending tasks and dispatches ready ones.
func (td *TaskDispatcher) ProcessQueue(ctx context.Context) error {
	td.mu.Lock()
	defer td.mu.Unlock()

	// Update task readiness
	td.updateTaskReadiness()

	// Select tasks to dispatch based on strategy
	tasksToDispatch := td.selectTasksToDispatch()

	// Dispatch selected tasks
	for _, task := range tasksToDispatch {
		if err := td.dispatchTask(ctx, task); err != nil {
			return err
		}
	}

	return nil
}

// updateTaskReadiness checks dependencies and marks tasks as ready.
func (td *TaskDispatcher) updateTaskReadiness() {
	for i := range td.pendingTasks {
		task := &td.pendingTasks[i]
		if task.Status != TaskStatusPending {
			continue
		}

		allDepsMet := true
		for _, depID := range task.Dependencies {
			if !td.isDependencySatisfied(depID) {
				allDepsMet = false
				break
			}
		}

		if allDepsMet {
			task.Status = TaskStatusReady
		}
	}
}

// isDependencySatisfied checks if a dependency task has completed.
func (td *TaskDispatcher) isDependencySatisfied(taskID string) bool {
	for _, completed := range td.completedTasks {
		if completed.Task.ID == taskID && completed.Status == TaskStatusComplete {
			return true
		}
	}
	return false
}

// selectTasksToDispatch selects tasks based on dispatch strategy.
func (td *TaskDispatcher) selectTasksToDispatch() []*DispatchTask {
	readyTasks := []*DispatchTask{}
	for i := range td.pendingTasks {
		task := &td.pendingTasks[i]
		if task.Status == TaskStatusReady {
			readyTasks = append(readyTasks, task)
		}
	}

	switch td.strategy {
	case StrategyFIFO:
		return readyTasks
	case StrategyPriority:
		return td.selectByPriority(readyTasks)
	case StrategyRoleAware:
		return td.selectByRole(readyTasks)
	case StrategyLoadBalanced:
		return td.selectByLoad(readyTasks)
	default:
		return readyTasks
	}
}

// selectByPriority sorts tasks by priority.
func (td *TaskDispatcher) selectByPriority(tasks []*DispatchTask) []*DispatchTask {
	// Simple bubble sort by priority (descending)
	sorted := make([]*DispatchTask, len(tasks))
	copy(sorted, tasks)

	for i := 0; i < len(sorted); i++ {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[j].Priority > sorted[i].Priority {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}

	return sorted
}

// selectByRole groups tasks by role for better affinity.
func (td *TaskDispatcher) selectByRole(tasks []*DispatchTask) []*DispatchTask {
	// For now, return as-is; can be extended with role-based scheduling
	return tasks
}

// selectByLoad considers worker pool load.
func (td *TaskDispatcher) selectByLoad(tasks []*DispatchTask) []*DispatchTask {
	stats := td.workerPool.GetWorkerStats()
	availableCapacity := stats.IdleWorkers

	if availableCapacity == 0 {
		return nil
	}

	// Return up to available capacity
	if len(tasks) <= availableCapacity {
		return tasks
	}
	return tasks[:availableCapacity]
}

// dispatchTask submits a task to the worker pool.
func (td *TaskDispatcher) dispatchTask(ctx context.Context, task *DispatchTask) error {
	resultCh, err := td.workerPool.Submit(ctx, task.Task, task.Priority)
	if err != nil {
		return err
	}

	task.Status = TaskStatusDispatched
	now := time.Now()
	task.StartedAt = &now
	td.activeTasks[task.Task.ID] = task

	// Start goroutine to handle result
	go td.handleTaskResult(task, resultCh)

	return nil
}

// handleTaskResult processes the task result asynchronously.
func (td *TaskDispatcher) handleTaskResult(task *DispatchTask, resultCh <-chan WorkResult) {
	result := <-resultCh

	td.mu.Lock()
	defer td.mu.Unlock()

	now := time.Now()
	task.CompletedAt = &now

	if result.Err != nil {
		task.Status = TaskStatusFailed
		task.Error = result.Err
	} else {
		task.Status = TaskStatusComplete
		task.Result = &result.Result
	}

	delete(td.activeTasks, task.Task.ID)
	td.completedTasks = append(td.completedTasks, task)

	// Remove from pending queue
	td.removePendingTask(task.Task.ID)
}

// removePendingTask removes a task from the pending queue.
func (td *TaskDispatcher) removePendingTask(taskID string) {
	newPending := []DispatchTask{}
	for _, t := range td.pendingTasks {
		if t.Task.ID != taskID {
			newPending = append(newPending, t)
		}
	}
	td.pendingTasks = newPending
}

// GetDispatchStats returns current dispatch statistics.
func (td *TaskDispatcher) GetDispatchStats() DispatchStats {
	td.mu.RLock()
	defer td.mu.RUnlock()

	stats := DispatchStats{
		PendingTasks:   len(td.pendingTasks),
		ActiveTasks:    len(td.activeTasks),
		CompletedTasks: len(td.completedTasks),
		Strategy:       td.strategy,
	}

	return stats
}

// SetStrategy changes the dispatch strategy.
func (td *TaskDispatcher) SetStrategy(strategy DispatchStrategy) error {
	td.mu.Lock()
	defer td.mu.Unlock()

	validStrategies := map[DispatchStrategy]bool{
		StrategyFIFO:         true,
		StrategyPriority:     true,
		StrategyRoleAware:    true,
		StrategyLoadBalanced: true,
	}

	if !validStrategies[strategy] {
		return errors.New("invalid dispatch strategy")
	}

	td.strategy = strategy
	return nil
}

// DispatchStats contains dispatch statistics.
type DispatchStats struct {
	PendingTasks   int
	ActiveTasks    int
	CompletedTasks int
	Strategy       DispatchStrategy
}
