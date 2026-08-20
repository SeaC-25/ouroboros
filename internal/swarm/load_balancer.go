package swarm

import (
	"sync"
	"time"
)

// LoadBalancer manages load distribution across workers.
type LoadBalancer struct {
	mu              sync.RWMutex
	workerLoad      map[string]*WorkerLoad
	roleDistribution map[AgentRole]int
	balancingPolicy BalancingPolicy
}

// WorkerLoad tracks the load on a single worker.
type WorkerLoad struct {
	WorkerID       string
	ActiveTasks    int
	CompletedTasks int
	FailedTasks    int
	TotalRuntime   time.Duration
	LastAssignment time.Time
	Load           float64 // 0.0 to 1.0
}

type BalancingPolicy string

const (
	PolicyRoundRobin    BalancingPolicy = "round_robin"
	PolicyLeastLoaded   BalancingPolicy = "least_loaded"
	PolicyRoleAffinity  BalancingPolicy = "role_affinity"
	PolicyWeightedRound BalancingPolicy = "weighted_round"
)

// NewLoadBalancer creates a new load balancer.
func NewLoadBalancer(policy BalancingPolicy) *LoadBalancer {
	return &LoadBalancer{
		workerLoad:       make(map[string]*WorkerLoad),
		roleDistribution: make(map[AgentRole]int),
		balancingPolicy:  policy,
	}
}

// SelectWorker selects the best worker for a task based on the balancing policy.
func (lb *LoadBalancer) SelectWorker(task AgentTask, availableWorkers []string) (string, error) {
	lb.mu.RLock()
	defer lb.mu.RUnlock()

	if len(availableWorkers) == 0 {
		return "", nil
	}

	switch lb.balancingPolicy {
	case PolicyRoundRobin:
		return lb.selectRoundRobin(availableWorkers), nil
	case PolicyLeastLoaded:
		return lb.selectLeastLoaded(availableWorkers), nil
	case PolicyRoleAffinity:
		return lb.selectByRoleAffinity(task.Role, availableWorkers), nil
	case PolicyWeightedRound:
		return lb.selectWeightedRound(availableWorkers), nil
	default:
		return availableWorkers[0], nil
	}
}

// selectRoundRobin uses simple round-robin selection.
func (lb *LoadBalancer) selectRoundRobin(workers []string) string {
	// Find worker with oldest last assignment
	var oldestWorker string
	var oldestTime time.Time

	for _, workerID := range workers {
		load, exists := lb.workerLoad[workerID]
		if !exists {
			return workerID // Prefer untracked workers (new)
		}
		if oldestWorker == "" || load.LastAssignment.Before(oldestTime) {
			oldestWorker = workerID
			oldestTime = load.LastAssignment
		}
	}

	return oldestWorker
}

// selectLeastLoaded selects the worker with the lowest current load.
func (lb *LoadBalancer) selectLeastLoaded(workers []string) string {
	var leastLoadedWorker string
	var minLoad float64 = 1.0

	for _, workerID := range workers {
		load, exists := lb.workerLoad[workerID]
		if !exists {
			return workerID // Prefer untracked workers (0 load)
		}
		if load.Load < minLoad {
			minLoad = load.Load
			leastLoadedWorker = workerID
		}
	}

	if leastLoadedWorker == "" {
		return workers[0]
	}
	return leastLoadedWorker
}

// selectByRoleAffinity prefers workers that recently handled the same role.
func (lb *LoadBalancer) selectByRoleAffinity(role AgentRole, workers []string) string {
	// For now, fall back to least loaded
	// Can be extended with role tracking per worker
	return lb.selectLeastLoaded(workers)
}

// selectWeightedRound uses weighted round-robin based on worker performance.
func (lb *LoadBalancer) selectWeightedRound(workers []string) string {
	// Calculate weights based on success rate
	type weightedWorker struct {
		ID     string
		Weight float64
	}

	weighted := []weightedWorker{}
	for _, workerID := range workers {
		load, exists := lb.workerLoad[workerID]
		if !exists {
			weighted = append(weighted, weightedWorker{ID: workerID, Weight: 1.0})
			continue
		}

		// Weight based on success rate
		totalTasks := load.CompletedTasks + load.FailedTasks
		if totalTasks == 0 {
			weighted = append(weighted, weightedWorker{ID: workerID, Weight: 1.0})
			continue
		}

		successRate := float64(load.CompletedTasks) / float64(totalTasks)
		weighted = append(weighted, weightedWorker{ID: workerID, Weight: successRate})
	}

	// Select worker with highest weight
	bestWorker := weighted[0].ID
	bestWeight := weighted[0].Weight
	for _, w := range weighted[1:] {
		if w.Weight > bestWeight {
			bestWorker = w.ID
			bestWeight = w.Weight
		}
	}

	return bestWorker
}

// RecordTaskAssignment records that a task was assigned to a worker.
func (lb *LoadBalancer) RecordTaskAssignment(workerID string, task AgentTask) {
	lb.mu.Lock()
	defer lb.mu.Unlock()

	load, exists := lb.workerLoad[workerID]
	if !exists {
		load = &WorkerLoad{
			WorkerID: workerID,
		}
		lb.workerLoad[workerID] = load
	}

	load.ActiveTasks++
	load.LastAssignment = time.Now()
	lb.updateLoad(load)

	lb.roleDistribution[task.Role]++
}

// RecordTaskCompletion records that a task completed on a worker.
func (lb *LoadBalancer) RecordTaskCompletion(workerID string, success bool, runtime time.Duration) {
	lb.mu.Lock()
	defer lb.mu.Unlock()

	load, exists := lb.workerLoad[workerID]
	if !exists {
		return
	}

	if load.ActiveTasks > 0 {
		load.ActiveTasks--
	}

	if success {
		load.CompletedTasks++
	} else {
		load.FailedTasks++
	}

	load.TotalRuntime += runtime
	lb.updateLoad(load)
}

// updateLoad recalculates the load value for a worker.
func (lb *LoadBalancer) updateLoad(load *WorkerLoad) {
	// Load is based on active tasks
	// Can be extended with more sophisticated metrics
	load.Load = float64(load.ActiveTasks) / 10.0
	if load.Load > 1.0 {
		load.Load = 1.0
	}
}

// GetLoadStats returns current load balancing statistics.
func (lb *LoadBalancer) GetLoadStats() LoadStats {
	lb.mu.RLock()
	defer lb.mu.RUnlock()

	stats := LoadStats{
		TotalWorkers:      len(lb.workerLoad),
		Policy:            lb.balancingPolicy,
		RoleDistribution:  make(map[AgentRole]int),
		AverageLoad:       0.0,
		MaxLoad:           0.0,
	}

	// Copy role distribution
	for role, count := range lb.roleDistribution {
		stats.RoleDistribution[role] = count
	}

	// Calculate load statistics
	if len(lb.workerLoad) > 0 {
		totalLoad := 0.0
		for _, load := range lb.workerLoad {
			totalLoad += load.Load
			if load.Load > stats.MaxLoad {
				stats.MaxLoad = load.Load
			}
		}
		stats.AverageLoad = totalLoad / float64(len(lb.workerLoad))
	}

	return stats
}

// GetWorkerLoad returns the load information for a specific worker.
func (lb *LoadBalancer) GetWorkerLoad(workerID string) (*WorkerLoad, bool) {
	lb.mu.RLock()
	defer lb.mu.RUnlock()

	load, exists := lb.workerLoad[workerID]
	if !exists {
		return nil, false
	}

	// Return a copy
	loadCopy := *load
	return &loadCopy, true
}

// ResetWorkerLoad resets the load tracking for a worker.
func (lb *LoadBalancer) ResetWorkerLoad(workerID string) {
	lb.mu.Lock()
	defer lb.mu.Unlock()

	delete(lb.workerLoad, workerID)
}

// SetBalancingPolicy changes the load balancing policy.
func (lb *LoadBalancer) SetBalancingPolicy(policy BalancingPolicy) {
	lb.mu.Lock()
	defer lb.mu.Unlock()

	lb.balancingPolicy = policy
}

// RebalanceWorkers suggests worker pool adjustments.
func (lb *LoadBalancer) RebalanceWorkers() RebalanceRecommendation {
	lb.mu.RLock()
	defer lb.mu.RUnlock()

	recommendation := RebalanceRecommendation{
		CurrentLoad: lb.calculateOverallLoad(),
	}

	// Suggest scaling based on load
	if recommendation.CurrentLoad > 0.8 {
		recommendation.Action = "scale_up"
		recommendation.Reason = "high load detected across workers"
	} else if recommendation.CurrentLoad < 0.2 && len(lb.workerLoad) > 1 {
		recommendation.Action = "scale_down"
		recommendation.Reason = "low load detected, excess capacity"
	} else {
		recommendation.Action = "maintain"
		recommendation.Reason = "load is balanced"
	}

	return recommendation
}

// calculateOverallLoad computes the average load across all workers.
func (lb *LoadBalancer) calculateOverallLoad() float64 {
	if len(lb.workerLoad) == 0 {
		return 0.0
	}

	totalLoad := 0.0
	for _, load := range lb.workerLoad {
		totalLoad += load.Load
	}

	return totalLoad / float64(len(lb.workerLoad))
}

// LoadStats contains load balancing statistics.
type LoadStats struct {
	TotalWorkers     int
	Policy           BalancingPolicy
	RoleDistribution map[AgentRole]int
	AverageLoad      float64
	MaxLoad          float64
}

// RebalanceRecommendation suggests pool adjustments.
type RebalanceRecommendation struct {
	CurrentLoad float64
	Action      string
	Reason      string
}
