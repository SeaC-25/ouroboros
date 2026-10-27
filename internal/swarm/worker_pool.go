package swarm

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// WorkerPool manages a pool of workers for concurrent task execution.
type WorkerPool struct {
	mu          sync.RWMutex
	workers     map[string]*Worker
	maxWorkers  int
	idleWorkers chan string
	activeCount int
	taskQueue   chan WorkItem
	shutdownCh  chan struct{}
	wg          sync.WaitGroup
}

// Worker represents a single worker in the pool.
type Worker struct {
	ID            string
	Role          AgentRole
	Status        WorkerStatus
	CurrentTask   *AgentTask
	TasksExecuted int
	CreatedAt     time.Time
	LastActiveAt  time.Time
}

type WorkerStatus string

const (
	WorkerIdle    WorkerStatus = "idle"
	WorkerActive  WorkerStatus = "active"
	WorkerBlocked WorkerStatus = "blocked"
	WorkerStopped WorkerStatus = "stopped"
)

// WorkItem represents a unit of work to be executed.
type WorkItem struct {
	Task       AgentTask
	Ctx        context.Context
	ResultCh   chan WorkResult
	Priority   int
	EnqueuedAt time.Time
}

// WorkResult contains the result of task execution.
type WorkResult struct {
	Result AgentResult
	Err    error
}

// NewWorkerPool creates a new worker pool.
func NewWorkerPool(maxWorkers int, queueSize int) *WorkerPool {
	if maxWorkers <= 0 {
		maxWorkers = 4
	}
	if queueSize <= 0 {
		queueSize = 100
	}

	return &WorkerPool{
		workers:     make(map[string]*Worker),
		maxWorkers:  maxWorkers,
		idleWorkers: make(chan string, maxWorkers),
		taskQueue:   make(chan WorkItem, queueSize),
		shutdownCh:  make(chan struct{}),
	}
}

// Start initializes the worker pool and begins processing tasks.
func (wp *WorkerPool) Start(ctx context.Context, orchestrator *Orchestrator) error {
	if orchestrator == nil {
		return errors.New("orchestrator is required")
	}

	// Spawn initial workers
	for i := 0; i < wp.maxWorkers; i++ {
		worker := wp.createWorker()
		wp.workers[worker.ID] = worker
		wp.idleWorkers <- worker.ID

		// Start worker goroutine
		wp.wg.Add(1)
		go wp.runWorker(ctx, worker, orchestrator)
	}

	return nil
}

// Submit adds a task to the work queue.
func (wp *WorkerPool) Submit(ctx context.Context, task AgentTask, priority int) (<-chan WorkResult, error) {
	wp.mu.RLock()
	defer wp.mu.RUnlock()

	select {
	case <-wp.shutdownCh:
		return nil, errors.New("worker pool is shutting down")
	default:
	}

	resultCh := make(chan WorkResult, 1)
	workItem := WorkItem{
		Task:       task,
		Ctx:        ctx,
		ResultCh:   resultCh,
		Priority:   priority,
		EnqueuedAt: time.Now(),
	}

	select {
	case wp.taskQueue <- workItem:
		return resultCh, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// runWorker is the main loop for a single worker.
func (wp *WorkerPool) runWorker(ctx context.Context, worker *Worker, orchestrator *Orchestrator) {
	defer wp.wg.Done()

	for {
		select {
		case <-ctx.Done():
			wp.markWorkerStopped(worker.ID)
			return
		case <-wp.shutdownCh:
			wp.markWorkerStopped(worker.ID)
			return
		case workItem := <-wp.taskQueue:
			wp.executeTask(workItem, worker, orchestrator)

			// Return worker to idle pool
			select {
			case wp.idleWorkers <- worker.ID:
			default:
			}
		}
	}
}

// executeTask executes a single task using the worker.
func (wp *WorkerPool) executeTask(item WorkItem, worker *Worker, orchestrator *Orchestrator) {
	wp.markWorkerActive(worker.ID, &item.Task)

	result, err := orchestrator.Execute(item.Ctx, item.Task)

	item.ResultCh <- WorkResult{
		Result: result,
		Err:    err,
	}
	close(item.ResultCh)

	wp.markWorkerIdle(worker.ID)
	worker.TasksExecuted++
}

// createWorker creates a new worker instance.
func (wp *WorkerPool) createWorker() *Worker {
	id := generateWorkerID()
	return &Worker{
		ID:        id,
		Status:    WorkerIdle,
		CreatedAt: time.Now(),
	}
}

// markWorkerActive marks a worker as actively executing a task.
func (wp *WorkerPool) markWorkerActive(workerID string, task *AgentTask) {
	wp.mu.Lock()
	defer wp.mu.Unlock()

	if worker, ok := wp.workers[workerID]; ok {
		worker.Status = WorkerActive
		worker.CurrentTask = task
		worker.LastActiveAt = time.Now()
		wp.activeCount++
	}
}

// markWorkerIdle marks a worker as idle.
func (wp *WorkerPool) markWorkerIdle(workerID string) {
	wp.mu.Lock()
	defer wp.mu.Unlock()

	if worker, ok := wp.workers[workerID]; ok {
		worker.Status = WorkerIdle
		worker.CurrentTask = nil
		if wp.activeCount > 0 {
			wp.activeCount--
		}
	}
}

// markWorkerStopped marks a worker as stopped.
func (wp *WorkerPool) markWorkerStopped(workerID string) {
	wp.mu.Lock()
	defer wp.mu.Unlock()

	if worker, ok := wp.workers[workerID]; ok {
		worker.Status = WorkerStopped
		worker.CurrentTask = nil
		if wp.activeCount > 0 {
			wp.activeCount--
		}
	}
}

// GetWorkerStats returns current worker pool statistics.
func (wp *WorkerPool) GetWorkerStats() WorkerStats {
	wp.mu.RLock()
	defer wp.mu.RUnlock()

	stats := WorkerStats{
		TotalWorkers:  len(wp.workers),
		ActiveWorkers: wp.activeCount,
		QueuedTasks:   len(wp.taskQueue),
	}

	stats.IdleWorkers = stats.TotalWorkers - stats.ActiveWorkers

	return stats
}

// Shutdown gracefully shuts down the worker pool.
func (wp *WorkerPool) Shutdown(ctx context.Context) error {
	close(wp.shutdownCh)

	// Wait for all workers to finish with timeout
	done := make(chan struct{})
	go func() {
		wp.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// WorkerStats contains worker pool statistics.
type WorkerStats struct {
	TotalWorkers  int
	ActiveWorkers int
	IdleWorkers   int
	QueuedTasks   int
}

var workerIDCounter int

func generateWorkerID() string {
	workerIDCounter++
	return fmt.Sprintf("worker-%d", workerIDCounter)
}
