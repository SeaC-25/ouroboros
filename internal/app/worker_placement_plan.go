package app

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/ouroboros/ouroboros/internal/compiler"
	"github.com/ouroboros/ouroboros/internal/evidence"
	"github.com/ouroboros/ouroboros/internal/toolchain"
)

type RuntimeWorkerPlacementPlanOptions struct {
	Now   time.Time
	RunID string
}

type RuntimeWorkerPlacementPlan struct {
	Version      int                               `json:"version"`
	Path         string                            `json:"-"`
	PlanID       string                            `json:"plan_id"`
	Status       string                            `json:"status"`
	Mode         string                            `json:"mode"`
	RunID        string                            `json:"run_id"`
	CreatedAt    string                            `json:"created_at"`
	ReadyCount   int                               `json:"ready_count"`
	SkippedCount int                               `json:"skipped_count"`
	Candidates   []RuntimeWorkerPlacementCandidate `json:"candidates"`
}

type RuntimeWorkerPlacementCandidate struct {
	Status               string                       `json:"status"`
	Mode                 string                       `json:"mode"`
	Reason               string                       `json:"reason,omitempty"`
	RunID                string                       `json:"run_id"`
	ExecutionID          string                       `json:"execution_id"`
	Tool                 string                       `json:"tool"`
	Capability           string                       `json:"capability"`
	Action               string                       `json:"action"`
	Runner               string                       `json:"runner,omitempty"`
	Parser               string                       `json:"parser,omitempty"`
	PlannedWorkerID      string                       `json:"planned_worker_id,omitempty"`
	WorkerID             string                       `json:"worker_id,omitempty"`
	WorkerMode           string                       `json:"worker_mode,omitempty"`
	WorkerEndpoint       string                       `json:"worker_endpoint,omitempty"`
	WorkerStatus         string                       `json:"worker_status,omitempty"`
	WorkerStale          bool                         `json:"worker_stale,omitempty"`
	RegisteredWorkerPath string                       `json:"registered_worker_path,omitempty"`
	ExecutionPolicy      string                       `json:"execution_policy,omitempty"`
	PolicyReason         string                       `json:"policy_reason,omitempty"`
	ToolPlan             RuntimeWorkerToolPlanPayload `json:"tool_plan,omitempty"`
}

type RuntimeWorkerPlacementClaimExecutionOptions struct {
	Execute              bool
	Now                  time.Time
	CandidateExecutionID string
	WorkerID             string
	LeaseDuration        time.Duration
}

type RuntimeWorkerPlacementClaimExecution struct {
	Version           int                          `json:"version"`
	Path              string                       `json:"-"`
	Status            string                       `json:"status"`
	Mode              string                       `json:"mode"`
	PlacementPlanPath string                       `json:"placement_plan_path"`
	PlanID            string                       `json:"plan_id"`
	RunID             string                       `json:"run_id"`
	ExecutionID       string                       `json:"execution_id"`
	Tool              string                       `json:"tool"`
	Capability        string                       `json:"capability"`
	Action            string                       `json:"action"`
	WorkerID          string                       `json:"worker_id"`
	WorkerMode        string                       `json:"worker_mode,omitempty"`
	WorkerEndpoint    string                       `json:"worker_endpoint,omitempty"`
	ToolPlan          RuntimeWorkerToolPlanPayload `json:"tool_plan,omitempty"`
	ExecutedAt        string                       `json:"executed_at"`
	ClaimPath         string                       `json:"claim_path,omitempty"`
	ClaimID           string                       `json:"claim_id,omitempty"`
	SkipReason        string                       `json:"skip_reason,omitempty"`
	Error             string                       `json:"error,omitempty"`
}

type RuntimeWorkerPlacementDispatchPlanOptions struct {
	Now           time.Time
	TransportMode string
}

type RuntimeWorkerPlacementDispatchPlan struct {
	Version       int                                       `json:"version"`
	Path          string                                    `json:"-"`
	PlanID        string                                    `json:"plan_id"`
	Status        string                                    `json:"status"`
	Mode          string                                    `json:"mode"`
	TransportMode string                                    `json:"transport_mode"`
	CreatedAt     string                                    `json:"created_at"`
	ReadyCount    int                                       `json:"ready_count"`
	SkippedCount  int                                       `json:"skipped_count"`
	Candidates    []RuntimeWorkerPlacementDispatchCandidate `json:"candidates"`
}

type RuntimeWorkerPlacementDispatchCandidate struct {
	Status                      string                       `json:"status"`
	Mode                        string                       `json:"mode"`
	Reason                      string                       `json:"reason,omitempty"`
	TransportMode               string                       `json:"transport_mode"`
	PlacementClaimExecutionPath string                       `json:"placement_claim_execution_path"`
	PlacementPlanPath           string                       `json:"placement_plan_path,omitempty"`
	ClaimPath                   string                       `json:"claim_path,omitempty"`
	ClaimID                     string                       `json:"claim_id,omitempty"`
	WorkerID                    string                       `json:"worker_id,omitempty"`
	WorkerMode                  string                       `json:"worker_mode,omitempty"`
	WorkerEndpoint              string                       `json:"worker_endpoint,omitempty"`
	RunID                       string                       `json:"run_id,omitempty"`
	ExecutionID                 string                       `json:"execution_id,omitempty"`
	Tool                        string                       `json:"tool,omitempty"`
	Capability                  string                       `json:"capability,omitempty"`
	Action                      string                       `json:"action,omitempty"`
	ToolPlan                    RuntimeWorkerToolPlanPayload `json:"tool_plan,omitempty"`
}

type RuntimeWorkerPlacementDispatchExecutionOptions struct {
	Execute                bool
	Now                    time.Time
	CandidateClaimPath     string
	CandidateExecutionID   string
	WorkerID               string
	TransportAuthToken     string
	ResultCallbackEndpoint string
}

type RuntimeWorkerPlacementDispatchExecution struct {
	Version                     int                          `json:"version"`
	Path                        string                       `json:"-"`
	Status                      string                       `json:"status"`
	Mode                        string                       `json:"mode"`
	TransportMode               string                       `json:"transport_mode"`
	PlacementDispatchPlanPath   string                       `json:"placement_dispatch_plan_path"`
	PlanID                      string                       `json:"plan_id"`
	PlacementClaimExecutionPath string                       `json:"placement_claim_execution_path"`
	ClaimPath                   string                       `json:"claim_path"`
	ClaimID                     string                       `json:"claim_id"`
	WorkerID                    string                       `json:"worker_id"`
	WorkerMode                  string                       `json:"worker_mode,omitempty"`
	WorkerEndpoint              string                       `json:"worker_endpoint,omitempty"`
	RunID                       string                       `json:"run_id"`
	ExecutionID                 string                       `json:"execution_id"`
	Tool                        string                       `json:"tool"`
	Capability                  string                       `json:"capability"`
	Action                      string                       `json:"action"`
	ToolPlan                    RuntimeWorkerToolPlanPayload `json:"tool_plan,omitempty"`
	ExecutedAt                  string                       `json:"executed_at"`
	QueueDispatchExecutionPath  string                       `json:"queue_dispatch_execution_path,omitempty"`
	TransportRequestPath        string                       `json:"transport_request_path,omitempty"`
	TransportEndpoint           string                       `json:"transport_endpoint,omitempty"`
	TransportHTTPStatus         int                          `json:"transport_http_status,omitempty"`
	TransportResponseBody       string                       `json:"transport_response_body,omitempty"`
	ResultCallbackEndpoint      string                       `json:"result_callback_endpoint,omitempty"`
	QueueDispatchResultPath     string                       `json:"queue_dispatch_result_path,omitempty"`
	QueueDispatchCompletedAt    string                       `json:"queue_dispatch_completed_at,omitempty"`
	SkipReason                  string                       `json:"skip_reason,omitempty"`
	Error                       string                       `json:"error,omitempty"`
}

type RuntimeWorkerPlacementDispatchBatchOptions struct {
	Execute                bool
	Now                    time.Time
	MaxCandidates          int
	TransportAuthToken     string
	ResultCallbackEndpoint string
}

type RuntimeWorkerPlacementDispatchBatch struct {
	Version                   int                                       `json:"version"`
	Path                      string                                    `json:"-"`
	Status                    string                                    `json:"status"`
	Mode                      string                                    `json:"mode"`
	TransportMode             string                                    `json:"transport_mode"`
	PlacementDispatchPlanPath string                                    `json:"placement_dispatch_plan_path"`
	PlanID                    string                                    `json:"plan_id"`
	ExecutedAt                string                                    `json:"executed_at"`
	MaxCandidates             int                                       `json:"max_candidates,omitempty"`
	TotalCount                int                                       `json:"total_count"`
	DispatchedCount           int                                       `json:"dispatched_count"`
	SkippedCount              int                                       `json:"skipped_count"`
	FailedCount               int                                       `json:"failed_count"`
	Executions                []RuntimeWorkerPlacementDispatchExecution `json:"executions"`
	Error                     string                                    `json:"error,omitempty"`
}

func PlanRuntimeWorkerPlacements(root string, options RuntimeWorkerPlacementPlanOptions) (RuntimeWorkerPlacementPlan, error) {
	now := options.Now.UTC()
	if now.IsZero() {
		now = time.Now().UTC()
	}
	runID := strings.TrimSpace(options.RunID)
	if runID == "" {
		runID = fmt.Sprintf("planned-run-%d", now.UnixNano())
	}

	requests, err := runtimeWorkerPlacementToolRequests(root, runID)
	if err != nil {
		return RuntimeWorkerPlacementPlan{}, err
	}
	workers, err := ListRuntimeWorkersWithStaleness(root, now, DefaultRuntimeWorkerStaleAfter)
	if err != nil {
		return RuntimeWorkerPlacementPlan{}, err
	}

	plan := RuntimeWorkerPlacementPlan{
		Version:   1,
		PlanID:    safeRuntimeWorkerPlacementPlanID(runID, now),
		Status:    "dry-run",
		Mode:      "metadata-only",
		RunID:     runID,
		CreatedAt: now.Format(time.RFC3339Nano),
	}
	for _, request := range requests {
		plan.Candidates = append(plan.Candidates, runtimeWorkerPlacementCandidatesForRequest(request, workers)...)
	}
	sortRuntimeWorkerPlacementCandidates(plan.Candidates)
	for _, candidate := range plan.Candidates {
		switch candidate.Status {
		case "ready":
			plan.ReadyCount++
		case "skipped":
			plan.SkippedCount++
		}
	}
	return plan, nil
}

func CreateRuntimeWorkerPlacementPlan(root string, options RuntimeWorkerPlacementPlanOptions) (RuntimeWorkerPlacementPlan, error) {
	plan, err := PlanRuntimeWorkerPlacements(root, options)
	if err != nil {
		return RuntimeWorkerPlacementPlan{}, err
	}
	plan.Path = filepath.Join(runtimeWorkerPlacementPlanDir(root), plan.PlanID+".json")
	if err := writeRuntimeWorkerPlacementPlanArtifact(plan); err != nil {
		return RuntimeWorkerPlacementPlan{}, err
	}
	return plan, nil
}

func ListRuntimeWorkerPlacementPlans(root string) ([]RuntimeWorkerPlacementPlan, error) {
	paths, err := filepath.Glob(filepath.Join(runtimeWorkerPlacementPlanDir(root), "*.json"))
	if err != nil {
		return nil, fmt.Errorf("glob worker placement plans: %w", err)
	}
	sort.Strings(paths)
	plans := make([]RuntimeWorkerPlacementPlan, 0, len(paths))
	for _, path := range paths {
		plan, err := readRuntimeWorkerPlacementPlanFile(path)
		if err != nil {
			return nil, err
		}
		plans = append(plans, plan)
	}
	sort.SliceStable(plans, func(i int, j int) bool {
		if plans[i].CreatedAt != plans[j].CreatedAt {
			return plans[i].CreatedAt < plans[j].CreatedAt
		}
		if plans[i].RunID != plans[j].RunID {
			return plans[i].RunID < plans[j].RunID
		}
		return plans[i].Path < plans[j].Path
	})
	return plans, nil
}

func ReadRuntimeWorkerPlacementPlan(root, planPath string) (RuntimeWorkerPlacementPlan, error) {
	path, err := resolveRuntimeWorkerPlacementPlanPath(root, planPath)
	if err != nil {
		return RuntimeWorkerPlacementPlan{}, err
	}
	return readRuntimeWorkerPlacementPlanFile(path)
}

func ExecuteRuntimeWorkerPlacementClaim(root, planPath string, options RuntimeWorkerPlacementClaimExecutionOptions) (RuntimeWorkerPlacementClaimExecution, error) {
	if !options.Execute {
		return RuntimeWorkerPlacementClaimExecution{}, fmt.Errorf("worker placement claim requires explicit execute")
	}
	plan, err := ReadRuntimeWorkerPlacementPlan(root, planPath)
	if err != nil {
		return RuntimeWorkerPlacementClaimExecution{}, err
	}
	if strings.TrimSpace(plan.Status) != "dry-run" || strings.TrimSpace(plan.Mode) != "metadata-only" {
		return RuntimeWorkerPlacementClaimExecution{}, fmt.Errorf("worker placement plan must be dry-run metadata-only")
	}
	candidate, err := selectRuntimeWorkerPlacementClaimCandidate(plan, options)
	if err != nil {
		return RuntimeWorkerPlacementClaimExecution{}, err
	}
	now := options.Now.UTC()
	if now.IsZero() {
		now = time.Now().UTC()
	}
	execution := runtimeWorkerPlacementClaimExecutionFromCandidate(plan, candidate, now)
	if reason := runtimeWorkerPlacementClaimSkipReason(root, candidate, now); reason != "" {
		execution.Status = "skipped"
		execution.SkipReason = reason
		if err := writeRuntimeWorkerPlacementClaimExecution(root, &execution); err != nil {
			return execution, err
		}
		return execution, nil
	}
	claim, err := CreateRuntimeWorkerQueueClaim(root, RuntimeWorkerQueueClaim{
		WorkerID:    candidate.WorkerID,
		RunID:       candidate.RunID,
		ExecutionID: candidate.ExecutionID,
		Tool:        candidate.Tool,
		Capability:  candidate.Capability,
		ToolPlan:    candidate.ToolPlan,
	}, RuntimeWorkerQueueClaimOptions{
		Now:           now,
		LeaseDuration: options.LeaseDuration,
	})
	if err != nil {
		execution.Status = "failed"
		execution.Error = err.Error()
		if writeErr := writeRuntimeWorkerPlacementClaimExecution(root, &execution); writeErr != nil {
			return execution, writeErr
		}
		return execution, err
	}
	execution.Status = "claimed"
	execution.ClaimPath = claim.Path
	execution.ClaimID = claim.ClaimID
	if err := writeRuntimeWorkerPlacementClaimExecution(root, &execution); err != nil {
		return execution, err
	}
	return execution, nil
}

func ListRuntimeWorkerPlacementClaimExecutions(root string) ([]RuntimeWorkerPlacementClaimExecution, error) {
	paths, err := filepath.Glob(filepath.Join(runtimeWorkerPlacementClaimExecutionDir(root), "*.json"))
	if err != nil {
		return nil, fmt.Errorf("glob worker placement claim executions: %w", err)
	}
	sort.Strings(paths)
	executions := make([]RuntimeWorkerPlacementClaimExecution, 0, len(paths))
	for _, path := range paths {
		execution, err := readRuntimeWorkerPlacementClaimExecutionFile(path)
		if err != nil {
			return nil, err
		}
		executions = append(executions, execution)
	}
	sort.SliceStable(executions, func(i int, j int) bool {
		if executions[i].ExecutedAt != executions[j].ExecutedAt {
			return executions[i].ExecutedAt < executions[j].ExecutedAt
		}
		if executions[i].WorkerID != executions[j].WorkerID {
			return executions[i].WorkerID < executions[j].WorkerID
		}
		return executions[i].Path < executions[j].Path
	})
	return executions, nil
}

func ReadRuntimeWorkerPlacementClaimExecution(root, executionPath string) (RuntimeWorkerPlacementClaimExecution, error) {
	path, err := resolveRuntimeWorkerPlacementClaimExecutionPath(root, executionPath)
	if err != nil {
		return RuntimeWorkerPlacementClaimExecution{}, err
	}
	return readRuntimeWorkerPlacementClaimExecutionFile(path)
}

func PlanRuntimeWorkerPlacementDispatches(root string, options RuntimeWorkerPlacementDispatchPlanOptions) (RuntimeWorkerPlacementDispatchPlan, error) {
	now := options.Now.UTC()
	if now.IsZero() {
		now = time.Now().UTC()
	}
	transportMode, err := normalizeRuntimeWorkerQueueDispatchTransport(options.TransportMode)
	if err != nil {
		return RuntimeWorkerPlacementDispatchPlan{}, err
	}
	claimExecutions, err := ListRuntimeWorkerPlacementClaimExecutions(root)
	if err != nil {
		return RuntimeWorkerPlacementDispatchPlan{}, err
	}
	plan := RuntimeWorkerPlacementDispatchPlan{
		Version:       1,
		PlanID:        safeRuntimeWorkerPlacementDispatchPlanID(now),
		Status:        "dry-run",
		Mode:          "metadata-only",
		TransportMode: transportMode,
		CreatedAt:     now.Format(time.RFC3339Nano),
		Candidates:    make([]RuntimeWorkerPlacementDispatchCandidate, 0, len(claimExecutions)),
	}
	for _, execution := range claimExecutions {
		plan.Candidates = append(plan.Candidates, runtimeWorkerPlacementDispatchCandidate(root, execution, now, transportMode))
	}
	sortRuntimeWorkerPlacementDispatchCandidates(plan.Candidates)
	for _, candidate := range plan.Candidates {
		switch candidate.Status {
		case "ready":
			plan.ReadyCount++
		case "skipped":
			plan.SkippedCount++
		}
	}
	return plan, nil
}

func CreateRuntimeWorkerPlacementDispatchPlan(root string, options RuntimeWorkerPlacementDispatchPlanOptions) (RuntimeWorkerPlacementDispatchPlan, error) {
	plan, err := PlanRuntimeWorkerPlacementDispatches(root, options)
	if err != nil {
		return RuntimeWorkerPlacementDispatchPlan{}, err
	}
	plan.Path = filepath.Join(runtimeWorkerPlacementDispatchPlanDir(root), plan.PlanID+".json")
	if err := writeRuntimeWorkerPlacementDispatchPlanArtifact(plan); err != nil {
		return RuntimeWorkerPlacementDispatchPlan{}, err
	}
	return plan, nil
}

func ListRuntimeWorkerPlacementDispatchPlans(root string) ([]RuntimeWorkerPlacementDispatchPlan, error) {
	paths, err := filepath.Glob(filepath.Join(runtimeWorkerPlacementDispatchPlanDir(root), "*.json"))
	if err != nil {
		return nil, fmt.Errorf("glob worker placement dispatch plans: %w", err)
	}
	sort.Strings(paths)
	plans := make([]RuntimeWorkerPlacementDispatchPlan, 0, len(paths))
	for _, path := range paths {
		plan, err := readRuntimeWorkerPlacementDispatchPlanFile(path)
		if err != nil {
			return nil, err
		}
		plans = append(plans, plan)
	}
	sort.SliceStable(plans, func(i int, j int) bool {
		if plans[i].CreatedAt != plans[j].CreatedAt {
			return plans[i].CreatedAt < plans[j].CreatedAt
		}
		return plans[i].Path < plans[j].Path
	})
	return plans, nil
}

func ReadRuntimeWorkerPlacementDispatchPlan(root, planPath string) (RuntimeWorkerPlacementDispatchPlan, error) {
	path, err := resolveRuntimeWorkerPlacementDispatchPlanPath(root, planPath)
	if err != nil {
		return RuntimeWorkerPlacementDispatchPlan{}, err
	}
	return readRuntimeWorkerPlacementDispatchPlanFile(path)
}

func ExecuteRuntimeWorkerPlacementDispatch(root, planPath string, options RuntimeWorkerPlacementDispatchExecutionOptions) (RuntimeWorkerPlacementDispatchExecution, error) {
	if !options.Execute {
		return RuntimeWorkerPlacementDispatchExecution{}, fmt.Errorf("worker placement dispatch requires explicit execute")
	}
	plan, err := ReadRuntimeWorkerPlacementDispatchPlan(root, planPath)
	if err != nil {
		return RuntimeWorkerPlacementDispatchExecution{}, err
	}
	if strings.TrimSpace(plan.Status) != "dry-run" || strings.TrimSpace(plan.Mode) != "metadata-only" {
		return RuntimeWorkerPlacementDispatchExecution{}, fmt.Errorf("worker placement dispatch plan must be dry-run metadata-only")
	}
	candidate, err := selectRuntimeWorkerPlacementDispatchCandidate(plan, options)
	if err != nil {
		return RuntimeWorkerPlacementDispatchExecution{}, err
	}
	now := options.Now.UTC()
	if now.IsZero() {
		now = time.Now().UTC()
	}
	execution := runtimeWorkerPlacementDispatchExecutionFromCandidate(plan, candidate, now)
	claim, err := ReadRuntimeWorkerQueueClaim(root, candidate.ClaimPath)
	if err != nil {
		execution.Status = "skipped"
		execution.SkipReason = "queue-claim-missing"
		if writeErr := writeRuntimeWorkerPlacementDispatchExecution(root, &execution); writeErr != nil {
			return execution, writeErr
		}
		return execution, nil
	}
	if reason := runtimeWorkerQueueDispatchSkipReason(root, claim, now); reason != "" {
		execution.Status = "skipped"
		execution.SkipReason = reason
		if err := writeRuntimeWorkerPlacementDispatchExecution(root, &execution); err != nil {
			return execution, err
		}
		return execution, nil
	}
	queueDispatch, err := ExecuteRuntimeWorkerQueueClaimDispatch(root, claim.Path, RuntimeWorkerQueueDispatchExecutionOptions{
		Execute:                true,
		Now:                    now,
		TransportMode:          candidate.TransportMode,
		TransportAuthToken:     options.TransportAuthToken,
		ResultCallbackEndpoint: options.ResultCallbackEndpoint,
	})
	execution = runtimeWorkerPlacementDispatchExecutionWithQueueDispatch(execution, queueDispatch)
	if err != nil {
		if strings.TrimSpace(execution.Status) == "" {
			execution.Status = "failed"
		}
		execution.Error = err.Error()
		if writeErr := writeRuntimeWorkerPlacementDispatchExecution(root, &execution); writeErr != nil {
			return execution, writeErr
		}
		return execution, err
	}
	if strings.TrimSpace(execution.Status) == "" {
		execution.Status = "dispatched"
	}
	if err := writeRuntimeWorkerPlacementDispatchExecution(root, &execution); err != nil {
		return execution, err
	}
	return execution, nil
}

func ListRuntimeWorkerPlacementDispatchExecutions(root string) ([]RuntimeWorkerPlacementDispatchExecution, error) {
	paths, err := filepath.Glob(filepath.Join(runtimeWorkerPlacementDispatchExecutionDir(root), "*.json"))
	if err != nil {
		return nil, fmt.Errorf("glob worker placement dispatch executions: %w", err)
	}
	sort.Strings(paths)
	executions := make([]RuntimeWorkerPlacementDispatchExecution, 0, len(paths))
	for _, path := range paths {
		execution, err := readRuntimeWorkerPlacementDispatchExecutionFile(path)
		if err != nil {
			return nil, err
		}
		executions = append(executions, execution)
	}
	sort.SliceStable(executions, func(i int, j int) bool {
		if executions[i].ExecutedAt != executions[j].ExecutedAt {
			return executions[i].ExecutedAt < executions[j].ExecutedAt
		}
		if executions[i].WorkerID != executions[j].WorkerID {
			return executions[i].WorkerID < executions[j].WorkerID
		}
		return executions[i].Path < executions[j].Path
	})
	return executions, nil
}

func ReadRuntimeWorkerPlacementDispatchExecution(root, executionPath string) (RuntimeWorkerPlacementDispatchExecution, error) {
	path, err := resolveRuntimeWorkerPlacementDispatchExecutionPath(root, executionPath)
	if err != nil {
		return RuntimeWorkerPlacementDispatchExecution{}, err
	}
	return readRuntimeWorkerPlacementDispatchExecutionFile(path)
}

func ExecuteRuntimeWorkerPlacementDispatchBatch(root, planPath string, options RuntimeWorkerPlacementDispatchBatchOptions) (RuntimeWorkerPlacementDispatchBatch, error) {
	if !options.Execute {
		return RuntimeWorkerPlacementDispatchBatch{}, fmt.Errorf("worker placement dispatch batch requires explicit execute")
	}
	plan, err := ReadRuntimeWorkerPlacementDispatchPlan(root, planPath)
	if err != nil {
		return RuntimeWorkerPlacementDispatchBatch{}, err
	}
	if strings.TrimSpace(plan.Status) != "dry-run" || strings.TrimSpace(plan.Mode) != "metadata-only" {
		return RuntimeWorkerPlacementDispatchBatch{}, fmt.Errorf("worker placement dispatch plan must be dry-run metadata-only")
	}
	now := options.Now.UTC()
	if now.IsZero() {
		now = time.Now().UTC()
	}
	batch := RuntimeWorkerPlacementDispatchBatch{
		Version:                   1,
		Status:                    "completed",
		Mode:                      "metadata-only",
		TransportMode:             plan.TransportMode,
		PlacementDispatchPlanPath: plan.Path,
		PlanID:                    plan.PlanID,
		ExecutedAt:                now.Format(time.RFC3339Nano),
		MaxCandidates:             options.MaxCandidates,
	}
	processed := 0
	for _, candidate := range plan.Candidates {
		if strings.TrimSpace(candidate.Status) != "ready" {
			continue
		}
		if options.MaxCandidates > 0 && processed >= options.MaxCandidates {
			break
		}
		processed++
		execution, err := ExecuteRuntimeWorkerPlacementDispatch(root, plan.Path, RuntimeWorkerPlacementDispatchExecutionOptions{
			Execute:                true,
			Now:                    now,
			CandidateClaimPath:     candidate.ClaimPath,
			CandidateExecutionID:   candidate.ExecutionID,
			WorkerID:               candidate.WorkerID,
			TransportAuthToken:     options.TransportAuthToken,
			ResultCallbackEndpoint: options.ResultCallbackEndpoint,
		})
		batch.Executions = append(batch.Executions, execution)
		runtimeWorkerPlacementDispatchBatchCount(&batch, execution)
		if err != nil {
			batch.Status = "failed"
			batch.Error = err.Error()
			if writeErr := writeRuntimeWorkerPlacementDispatchBatch(root, &batch); writeErr != nil {
				return batch, writeErr
			}
			return batch, err
		}
	}
	batch.TotalCount = len(batch.Executions)
	if batch.TotalCount == 0 || (batch.DispatchedCount == 0 && batch.SkippedCount > 0) {
		batch.Status = "skipped"
	}
	if batch.FailedCount > 0 {
		batch.Status = "failed"
	}
	if err := writeRuntimeWorkerPlacementDispatchBatch(root, &batch); err != nil {
		return batch, err
	}
	return batch, nil
}

func ListRuntimeWorkerPlacementDispatchBatches(root string) ([]RuntimeWorkerPlacementDispatchBatch, error) {
	paths, err := filepath.Glob(filepath.Join(runtimeWorkerPlacementDispatchBatchDir(root), "*.json"))
	if err != nil {
		return nil, fmt.Errorf("glob worker placement dispatch batches: %w", err)
	}
	sort.Strings(paths)
	batches := make([]RuntimeWorkerPlacementDispatchBatch, 0, len(paths))
	for _, path := range paths {
		batch, err := readRuntimeWorkerPlacementDispatchBatchFile(path)
		if err != nil {
			return nil, err
		}
		batches = append(batches, batch)
	}
	sort.SliceStable(batches, func(i int, j int) bool {
		if batches[i].ExecutedAt != batches[j].ExecutedAt {
			return batches[i].ExecutedAt < batches[j].ExecutedAt
		}
		return batches[i].Path < batches[j].Path
	})
	return batches, nil
}

func ReadRuntimeWorkerPlacementDispatchBatch(root, batchPath string) (RuntimeWorkerPlacementDispatchBatch, error) {
	path, err := resolveRuntimeWorkerPlacementDispatchBatchPath(root, batchPath)
	if err != nil {
		return RuntimeWorkerPlacementDispatchBatch{}, err
	}
	return readRuntimeWorkerPlacementDispatchBatchFile(path)
}

func runtimeWorkerPlacementToolRequests(root string, runID string) ([]ToolExecutionRequest, error) {
	manifests, err := toolchain.LoadManifests(filepath.Join(root, "toolchain"))
	if err != nil {
		return nil, fmt.Errorf("load toolchain manifests: %w", err)
	}
	scopeConfig, err := LoadScopeConfig(filepath.Join(root, "scope.yaml"))
	if err != nil {
		return nil, fmt.Errorf("load scope config: %w", err)
	}
	if err := ValidateScopeConfig(scopeConfig); err != nil {
		return nil, fmt.Errorf("invalid scope config: %w", err)
	}
	if err := validatePackExecutionPoliciesAgainstManifests(scopeConfig.Execution.PackPolicies, manifests); err != nil {
		return nil, fmt.Errorf("invalid scope config: %w", err)
	}
	plan, err := compiler.NewCompiler().Compile(compiler.CompileRequest{
		Action: compiler.ActionDiscoverAssets,
		Evidence: compiler.EvidenceSlice{
			Nodes: []evidence.Node{
				{
					ID:         "fact-workspace",
					Type:       evidence.NodeTarget,
					Label:      filepath.Base(root),
					SourceKind: "workspace",
				},
			},
		},
		Manifests: manifests,
	})
	if err != nil {
		return nil, fmt.Errorf("compile tool execution plan: %w", err)
	}
	return toolExecutionRequests(
		root,
		runID,
		plan.Nodes,
		scopeConfig.Scope.Targets,
		scopeConfig.Scope.Exclusions,
		scopeConfig.Scope.TargetPolicy,
		scopeConfig.Execution.Isolation,
		scopeConfig.Execution.Container,
		scopeConfig.Execution.ResourceLimits,
		scopeConfig.Execution.PackPolicies,
		scopeConfig.Execution.WorkerPools,
		nil,
	), nil
}

func runtimeWorkerPlacementCandidatesForRequest(request ToolExecutionRequest, workers []RuntimeWorkerArtifact) []RuntimeWorkerPlacementCandidate {
	base := runtimeWorkerPlacementCandidateFromRequest(request)
	if packExecutionPolicyDisabled(request.ExecutionPolicy) {
		base.Status = "skipped"
		base.Reason = "pack-policy-disabled"
		return []RuntimeWorkerPlacementCandidate{base}
	}

	var matching []RuntimeWorkerArtifact
	for _, worker := range workers {
		if runtimeWorkerHasCapability(worker, request.Node.Capability) {
			matching = append(matching, worker)
		}
	}
	if len(matching) == 0 {
		base.Status = "skipped"
		base.Reason = "no-capability-worker"
		return []RuntimeWorkerPlacementCandidate{base}
	}

	candidates := make([]RuntimeWorkerPlacementCandidate, 0, len(matching))
	for _, worker := range matching {
		candidate := base
		candidate.WorkerID = worker.WorkerID
		candidate.WorkerMode = worker.Mode
		candidate.WorkerEndpoint = worker.Endpoint
		candidate.WorkerStatus = worker.Status
		candidate.WorkerStale = worker.Stale
		candidate.RegisteredWorkerPath = worker.Path
		switch {
		case strings.TrimSpace(worker.Status) == "offline":
			candidate.Status = "skipped"
			candidate.Reason = "worker-offline"
		case worker.Stale:
			candidate.Status = "skipped"
			candidate.Reason = "worker-stale"
		default:
			candidate.Status = "ready"
		}
		candidates = append(candidates, candidate)
	}
	return candidates
}

func runtimeWorkerPlacementCandidateFromRequest(request ToolExecutionRequest) RuntimeWorkerPlacementCandidate {
	policy := strings.TrimSpace(request.ExecutionPolicy)
	if policy == "" {
		policy = "default"
	}
	return RuntimeWorkerPlacementCandidate{
		Status:          "skipped",
		Mode:            "metadata-only",
		RunID:           request.RunID,
		ExecutionID:     runtimeWorkerPlacementExecutionID(request.Node),
		Tool:            request.Node.ManifestName,
		Capability:      request.Node.Capability,
		Action:          string(request.Node.Action),
		Runner:          request.Node.Runner,
		Parser:          request.Node.Parser,
		PlannedWorkerID: request.WorkerID,
		ExecutionPolicy: policy,
		PolicyReason:    strings.TrimSpace(request.PolicyReason),
		ToolPlan:        runtimeWorkerToolPlanPayloadFromToolExecutionRequest(request),
	}
}

func selectRuntimeWorkerPlacementClaimCandidate(plan RuntimeWorkerPlacementPlan, options RuntimeWorkerPlacementClaimExecutionOptions) (RuntimeWorkerPlacementCandidate, error) {
	executionID := strings.TrimSpace(options.CandidateExecutionID)
	workerID := strings.TrimSpace(options.WorkerID)
	matches := make([]RuntimeWorkerPlacementCandidate, 0, 1)
	for _, candidate := range plan.Candidates {
		if strings.TrimSpace(candidate.Status) != "ready" {
			continue
		}
		if executionID != "" && strings.TrimSpace(candidate.ExecutionID) != executionID {
			continue
		}
		if workerID != "" && strings.TrimSpace(candidate.WorkerID) != workerID {
			continue
		}
		matches = append(matches, candidate)
	}
	if len(matches) == 0 {
		return RuntimeWorkerPlacementCandidate{}, fmt.Errorf("no ready worker placement candidate matched")
	}
	if len(matches) > 1 {
		return RuntimeWorkerPlacementCandidate{}, fmt.Errorf("worker placement candidate is ambiguous; provide --execution-id and --worker")
	}
	return matches[0], nil
}

func runtimeWorkerPlacementClaimExecutionFromCandidate(plan RuntimeWorkerPlacementPlan, candidate RuntimeWorkerPlacementCandidate, now time.Time) RuntimeWorkerPlacementClaimExecution {
	return RuntimeWorkerPlacementClaimExecution{
		Version:           1,
		Status:            "pending",
		Mode:              "metadata-only",
		PlacementPlanPath: plan.Path,
		PlanID:            plan.PlanID,
		RunID:             candidate.RunID,
		ExecutionID:       candidate.ExecutionID,
		Tool:              candidate.Tool,
		Capability:        candidate.Capability,
		Action:            candidate.Action,
		WorkerID:          candidate.WorkerID,
		WorkerMode:        candidate.WorkerMode,
		WorkerEndpoint:    candidate.WorkerEndpoint,
		ToolPlan:          normalizeRuntimeWorkerToolPlanPayload(candidate.ToolPlan, candidate.Tool),
		ExecutedAt:        now.Format(time.RFC3339Nano),
	}
}

func runtimeWorkerPlacementClaimSkipReason(root string, candidate RuntimeWorkerPlacementCandidate, now time.Time) string {
	if strings.TrimSpace(candidate.Status) != "ready" {
		return "candidate-not-ready"
	}
	worker, err := ReadRuntimeWorker(root, candidate.WorkerID)
	if err != nil {
		return "worker-missing"
	}
	if strings.TrimSpace(worker.Status) == "offline" {
		return "worker-offline"
	}
	if runtimeWorkerIsStale(worker, now, DefaultRuntimeWorkerStaleAfter) {
		return "worker-stale"
	}
	if !runtimeWorkerHasCapability(worker, candidate.Capability) {
		return "capability-mismatch"
	}
	return ""
}

func runtimeWorkerPlacementDispatchCandidate(root string, execution RuntimeWorkerPlacementClaimExecution, now time.Time, transportMode string) RuntimeWorkerPlacementDispatchCandidate {
	candidate := RuntimeWorkerPlacementDispatchCandidate{
		Status:                      "skipped",
		Mode:                        "metadata-only",
		TransportMode:               transportMode,
		PlacementClaimExecutionPath: execution.Path,
		PlacementPlanPath:           execution.PlacementPlanPath,
		ClaimPath:                   execution.ClaimPath,
		ClaimID:                     execution.ClaimID,
		WorkerID:                    execution.WorkerID,
		WorkerMode:                  execution.WorkerMode,
		WorkerEndpoint:              execution.WorkerEndpoint,
		RunID:                       execution.RunID,
		ExecutionID:                 execution.ExecutionID,
		Tool:                        execution.Tool,
		Capability:                  execution.Capability,
		Action:                      execution.Action,
		ToolPlan:                    normalizeRuntimeWorkerToolPlanPayload(execution.ToolPlan, execution.Tool),
	}
	if strings.TrimSpace(execution.Status) != "claimed" {
		candidate.Reason = "placement-claim-not-claimed"
		return candidate
	}
	if strings.TrimSpace(execution.ClaimPath) == "" {
		candidate.Reason = "queue-claim-missing"
		return candidate
	}
	claim, err := ReadRuntimeWorkerQueueClaim(root, execution.ClaimPath)
	if err != nil {
		candidate.Reason = "queue-claim-missing"
		return candidate
	}
	candidate.ClaimPath = claim.Path
	candidate.ClaimID = claim.ClaimID
	candidate.WorkerID = claim.WorkerID
	candidate.WorkerMode = claim.WorkerMode
	candidate.WorkerEndpoint = claim.WorkerEndpoint
	candidate.RunID = claim.RunID
	candidate.ExecutionID = claim.ExecutionID
	candidate.Tool = claim.Tool
	candidate.Capability = claim.Capability
	candidate.ToolPlan = normalizeRuntimeWorkerToolPlanPayload(claim.ToolPlan, claim.Tool)
	if reason := runtimeWorkerQueueDispatchSkipReason(root, claim, now); reason != "" {
		candidate.Reason = reason
		return candidate
	}
	candidate.Status = "ready"
	return candidate
}

func selectRuntimeWorkerPlacementDispatchCandidate(plan RuntimeWorkerPlacementDispatchPlan, options RuntimeWorkerPlacementDispatchExecutionOptions) (RuntimeWorkerPlacementDispatchCandidate, error) {
	claimPath := strings.TrimSpace(options.CandidateClaimPath)
	executionID := strings.TrimSpace(options.CandidateExecutionID)
	workerID := strings.TrimSpace(options.WorkerID)
	matches := make([]RuntimeWorkerPlacementDispatchCandidate, 0, 1)
	for _, candidate := range plan.Candidates {
		if strings.TrimSpace(candidate.Status) != "ready" {
			continue
		}
		if claimPath != "" && !runtimeWorkerPlacementDispatchClaimPathMatches(candidate.ClaimPath, claimPath) {
			continue
		}
		if executionID != "" && strings.TrimSpace(candidate.ExecutionID) != executionID {
			continue
		}
		if workerID != "" && strings.TrimSpace(candidate.WorkerID) != workerID {
			continue
		}
		matches = append(matches, candidate)
	}
	if len(matches) == 0 {
		return RuntimeWorkerPlacementDispatchCandidate{}, fmt.Errorf("no ready worker placement dispatch candidate matched")
	}
	if len(matches) > 1 {
		return RuntimeWorkerPlacementDispatchCandidate{}, fmt.Errorf("worker placement dispatch candidate is ambiguous; provide --claim, --execution-id, or --worker")
	}
	return matches[0], nil
}

func runtimeWorkerPlacementDispatchClaimPathMatches(candidatePath string, requested string) bool {
	candidatePath = strings.TrimSpace(candidatePath)
	requested = strings.TrimSpace(requested)
	if candidatePath == requested {
		return true
	}
	return filepath.Base(candidatePath) == requested
}

func runtimeWorkerPlacementDispatchExecutionFromCandidate(plan RuntimeWorkerPlacementDispatchPlan, candidate RuntimeWorkerPlacementDispatchCandidate, now time.Time) RuntimeWorkerPlacementDispatchExecution {
	return RuntimeWorkerPlacementDispatchExecution{
		Version:                     1,
		Status:                      "pending",
		Mode:                        "metadata-only",
		TransportMode:               candidate.TransportMode,
		PlacementDispatchPlanPath:   plan.Path,
		PlanID:                      plan.PlanID,
		PlacementClaimExecutionPath: candidate.PlacementClaimExecutionPath,
		ClaimPath:                   candidate.ClaimPath,
		ClaimID:                     candidate.ClaimID,
		WorkerID:                    candidate.WorkerID,
		WorkerMode:                  candidate.WorkerMode,
		WorkerEndpoint:              candidate.WorkerEndpoint,
		RunID:                       candidate.RunID,
		ExecutionID:                 candidate.ExecutionID,
		Tool:                        candidate.Tool,
		Capability:                  candidate.Capability,
		Action:                      candidate.Action,
		ToolPlan:                    normalizeRuntimeWorkerToolPlanPayload(candidate.ToolPlan, candidate.Tool),
		ExecutedAt:                  now.Format(time.RFC3339Nano),
	}
}

func runtimeWorkerPlacementDispatchExecutionWithQueueDispatch(execution RuntimeWorkerPlacementDispatchExecution, queueDispatch RuntimeWorkerQueueDispatchExecution) RuntimeWorkerPlacementDispatchExecution {
	execution.Status = queueDispatch.Status
	execution.QueueDispatchExecutionPath = queueDispatch.Path
	execution.TransportMode = queueDispatch.TransportMode
	execution.TransportRequestPath = queueDispatch.TransportRequestPath
	execution.TransportEndpoint = queueDispatch.TransportEndpoint
	execution.TransportHTTPStatus = queueDispatch.TransportHTTPStatus
	execution.TransportResponseBody = queueDispatch.TransportResponseBody
	execution.ResultCallbackEndpoint = queueDispatch.ResultCallbackEndpoint
	execution.QueueDispatchResultPath = queueDispatch.ResultPath
	execution.QueueDispatchCompletedAt = queueDispatch.CompletedAt
	execution.SkipReason = queueDispatch.SkipReason
	execution.Error = queueDispatch.Error
	if strings.TrimSpace(queueDispatch.ClaimPath) != "" {
		execution.ClaimPath = queueDispatch.ClaimPath
	}
	if strings.TrimSpace(queueDispatch.ClaimID) != "" {
		execution.ClaimID = queueDispatch.ClaimID
	}
	return execution
}

func runtimeWorkerPlacementDispatchBatchCount(batch *RuntimeWorkerPlacementDispatchBatch, execution RuntimeWorkerPlacementDispatchExecution) {
	switch strings.TrimSpace(execution.Status) {
	case "dispatched", "completed":
		batch.DispatchedCount++
	case "skipped":
		batch.SkippedCount++
	case "failed":
		batch.FailedCount++
	default:
		if strings.TrimSpace(execution.Error) != "" {
			batch.FailedCount++
		}
	}
	batch.TotalCount = len(batch.Executions)
}

func sortRuntimeWorkerPlacementCandidates(candidates []RuntimeWorkerPlacementCandidate) {
	sort.SliceStable(candidates, func(i int, j int) bool {
		if runtimeWorkerPlacementCandidateStatusRank(candidates[i].Status) != runtimeWorkerPlacementCandidateStatusRank(candidates[j].Status) {
			return runtimeWorkerPlacementCandidateStatusRank(candidates[i].Status) < runtimeWorkerPlacementCandidateStatusRank(candidates[j].Status)
		}
		if candidates[i].Tool != candidates[j].Tool {
			return candidates[i].Tool < candidates[j].Tool
		}
		if candidates[i].Capability != candidates[j].Capability {
			return candidates[i].Capability < candidates[j].Capability
		}
		if candidates[i].Status != candidates[j].Status {
			return candidates[i].Status < candidates[j].Status
		}
		if candidates[i].WorkerID != candidates[j].WorkerID {
			return candidates[i].WorkerID < candidates[j].WorkerID
		}
		return candidates[i].Reason < candidates[j].Reason
	})
}

func runtimeWorkerPlacementCandidateStatusRank(status string) int {
	switch strings.TrimSpace(status) {
	case "ready":
		return 0
	case "skipped":
		return 1
	default:
		return 2
	}
}

func sortRuntimeWorkerPlacementDispatchCandidates(candidates []RuntimeWorkerPlacementDispatchCandidate) {
	sort.SliceStable(candidates, func(i int, j int) bool {
		if candidates[i].Status != candidates[j].Status {
			return candidates[i].Status < candidates[j].Status
		}
		if candidates[i].WorkerID != candidates[j].WorkerID {
			return candidates[i].WorkerID < candidates[j].WorkerID
		}
		if candidates[i].ExecutionID != candidates[j].ExecutionID {
			return candidates[i].ExecutionID < candidates[j].ExecutionID
		}
		return candidates[i].Reason < candidates[j].Reason
	})
}

func writeRuntimeWorkerPlacementPlanArtifact(plan RuntimeWorkerPlacementPlan) error {
	if err := os.MkdirAll(filepath.Dir(plan.Path), 0o755); err != nil {
		return fmt.Errorf("create worker placement plan directory: %w", err)
	}
	data, err := json.MarshalIndent(plan, "", "  ")
	if err != nil {
		return fmt.Errorf("encode worker placement plan: %w", err)
	}
	data = append(data, '\n')
	tmp := plan.Path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("write worker placement plan: %w", err)
	}
	if err := os.Rename(tmp, plan.Path); err != nil {
		return fmt.Errorf("publish worker placement plan: %w", err)
	}
	return nil
}

func writeRuntimeWorkerPlacementClaimExecution(root string, execution *RuntimeWorkerPlacementClaimExecution) error {
	if err := os.MkdirAll(runtimeWorkerPlacementClaimExecutionDir(root), 0o755); err != nil {
		return fmt.Errorf("create worker placement claim execution directory: %w", err)
	}
	if strings.TrimSpace(execution.Path) == "" {
		execution.Path = filepath.Join(runtimeWorkerPlacementClaimExecutionDir(root), safeRuntimeWorkerPlacementClaimExecutionID(*execution)+".json")
	}
	data, err := json.MarshalIndent(execution, "", "  ")
	if err != nil {
		return fmt.Errorf("encode worker placement claim execution: %w", err)
	}
	data = append(data, '\n')
	tmp := execution.Path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("write worker placement claim execution: %w", err)
	}
	if err := os.Rename(tmp, execution.Path); err != nil {
		return fmt.Errorf("publish worker placement claim execution: %w", err)
	}
	return nil
}

func writeRuntimeWorkerPlacementDispatchPlanArtifact(plan RuntimeWorkerPlacementDispatchPlan) error {
	if err := os.MkdirAll(filepath.Dir(plan.Path), 0o755); err != nil {
		return fmt.Errorf("create worker placement dispatch plan directory: %w", err)
	}
	data, err := json.MarshalIndent(plan, "", "  ")
	if err != nil {
		return fmt.Errorf("encode worker placement dispatch plan: %w", err)
	}
	data = append(data, '\n')
	tmp := plan.Path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("write worker placement dispatch plan: %w", err)
	}
	if err := os.Rename(tmp, plan.Path); err != nil {
		return fmt.Errorf("publish worker placement dispatch plan: %w", err)
	}
	return nil
}

func writeRuntimeWorkerPlacementDispatchExecution(root string, execution *RuntimeWorkerPlacementDispatchExecution) error {
	if err := os.MkdirAll(runtimeWorkerPlacementDispatchExecutionDir(root), 0o755); err != nil {
		return fmt.Errorf("create worker placement dispatch execution directory: %w", err)
	}
	if strings.TrimSpace(execution.Path) == "" {
		execution.Path = filepath.Join(runtimeWorkerPlacementDispatchExecutionDir(root), safeRuntimeWorkerPlacementDispatchExecutionID(*execution)+".json")
	}
	data, err := json.MarshalIndent(execution, "", "  ")
	if err != nil {
		return fmt.Errorf("encode worker placement dispatch execution: %w", err)
	}
	data = append(data, '\n')
	tmp := execution.Path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("write worker placement dispatch execution: %w", err)
	}
	if err := os.Rename(tmp, execution.Path); err != nil {
		return fmt.Errorf("publish worker placement dispatch execution: %w", err)
	}
	return nil
}

func writeRuntimeWorkerPlacementDispatchBatch(root string, batch *RuntimeWorkerPlacementDispatchBatch) error {
	if err := os.MkdirAll(runtimeWorkerPlacementDispatchBatchDir(root), 0o755); err != nil {
		return fmt.Errorf("create worker placement dispatch batch directory: %w", err)
	}
	if strings.TrimSpace(batch.Path) == "" {
		batch.Path = filepath.Join(runtimeWorkerPlacementDispatchBatchDir(root), safeRuntimeWorkerPlacementDispatchBatchID(*batch)+".json")
	}
	data, err := json.MarshalIndent(batch, "", "  ")
	if err != nil {
		return fmt.Errorf("encode worker placement dispatch batch: %w", err)
	}
	data = append(data, '\n')
	tmp := batch.Path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("write worker placement dispatch batch: %w", err)
	}
	if err := os.Rename(tmp, batch.Path); err != nil {
		return fmt.Errorf("publish worker placement dispatch batch: %w", err)
	}
	return nil
}

func readRuntimeWorkerPlacementPlanFile(path string) (RuntimeWorkerPlacementPlan, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return RuntimeWorkerPlacementPlan{}, fmt.Errorf("read worker placement plan %s: %w", path, err)
	}
	var plan RuntimeWorkerPlacementPlan
	if err := json.Unmarshal(data, &plan); err != nil {
		return RuntimeWorkerPlacementPlan{}, fmt.Errorf("decode worker placement plan %s: %w", path, err)
	}
	plan.Path = path
	return plan, nil
}

func readRuntimeWorkerPlacementClaimExecutionFile(path string) (RuntimeWorkerPlacementClaimExecution, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return RuntimeWorkerPlacementClaimExecution{}, fmt.Errorf("read worker placement claim execution %s: %w", path, err)
	}
	var execution RuntimeWorkerPlacementClaimExecution
	if err := json.Unmarshal(data, &execution); err != nil {
		return RuntimeWorkerPlacementClaimExecution{}, fmt.Errorf("decode worker placement claim execution %s: %w", path, err)
	}
	execution.Path = path
	return execution, nil
}

func readRuntimeWorkerPlacementDispatchPlanFile(path string) (RuntimeWorkerPlacementDispatchPlan, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return RuntimeWorkerPlacementDispatchPlan{}, fmt.Errorf("read worker placement dispatch plan %s: %w", path, err)
	}
	var plan RuntimeWorkerPlacementDispatchPlan
	if err := json.Unmarshal(data, &plan); err != nil {
		return RuntimeWorkerPlacementDispatchPlan{}, fmt.Errorf("decode worker placement dispatch plan %s: %w", path, err)
	}
	plan.Path = path
	return plan, nil
}

func readRuntimeWorkerPlacementDispatchExecutionFile(path string) (RuntimeWorkerPlacementDispatchExecution, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return RuntimeWorkerPlacementDispatchExecution{}, fmt.Errorf("read worker placement dispatch execution %s: %w", path, err)
	}
	var execution RuntimeWorkerPlacementDispatchExecution
	if err := json.Unmarshal(data, &execution); err != nil {
		return RuntimeWorkerPlacementDispatchExecution{}, fmt.Errorf("decode worker placement dispatch execution %s: %w", path, err)
	}
	execution.Path = path
	return execution, nil
}

func readRuntimeWorkerPlacementDispatchBatchFile(path string) (RuntimeWorkerPlacementDispatchBatch, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return RuntimeWorkerPlacementDispatchBatch{}, fmt.Errorf("read worker placement dispatch batch %s: %w", path, err)
	}
	var batch RuntimeWorkerPlacementDispatchBatch
	if err := json.Unmarshal(data, &batch); err != nil {
		return RuntimeWorkerPlacementDispatchBatch{}, fmt.Errorf("decode worker placement dispatch batch %s: %w", path, err)
	}
	batch.Path = path
	return batch, nil
}

func runtimeWorkerPlacementPlanDir(root string) string {
	return filepath.Join(root, "runtime", "worker-placement-plans")
}

func runtimeWorkerPlacementClaimExecutionDir(root string) string {
	return filepath.Join(root, "runtime", "worker-placement-claim-executions")
}

func runtimeWorkerPlacementDispatchPlanDir(root string) string {
	return filepath.Join(root, "runtime", "worker-placement-dispatch-plans")
}

func runtimeWorkerPlacementDispatchExecutionDir(root string) string {
	return filepath.Join(root, "runtime", "worker-placement-dispatch-executions")
}

func runtimeWorkerPlacementDispatchBatchDir(root string) string {
	return filepath.Join(root, "runtime", "worker-placement-dispatch-batches")
}

func resolveRuntimeWorkerPlacementPlanPath(root, planPath string) (string, error) {
	trimmed := strings.TrimSpace(planPath)
	if trimmed == "" {
		return "", fmt.Errorf("worker placement plan path is required")
	}
	dir, err := filepath.Abs(runtimeWorkerPlacementPlanDir(root))
	if err != nil {
		return "", fmt.Errorf("resolve worker placement plan directory: %w", err)
	}
	candidate := trimmed
	if !filepath.IsAbs(candidate) {
		if filepath.Ext(candidate) == "" && !strings.Contains(candidate, string(os.PathSeparator)) {
			candidate += ".json"
		}
		candidate = filepath.Join(dir, candidate)
	}
	candidate, err = filepath.Abs(candidate)
	if err != nil {
		return "", fmt.Errorf("resolve worker placement plan path: %w", err)
	}
	rel, err := filepath.Rel(dir, candidate)
	if err != nil {
		return "", fmt.Errorf("resolve worker placement plan relative path: %w", err)
	}
	if rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("worker placement plan path must stay under %s", dir)
	}
	if filepath.Ext(candidate) != ".json" {
		return "", fmt.Errorf("worker placement plan path must be a json file")
	}
	return candidate, nil
}

func resolveRuntimeWorkerPlacementClaimExecutionPath(root, executionPath string) (string, error) {
	trimmed := strings.TrimSpace(executionPath)
	if trimmed == "" {
		return "", fmt.Errorf("worker placement claim execution path is required")
	}
	dir, err := filepath.Abs(runtimeWorkerPlacementClaimExecutionDir(root))
	if err != nil {
		return "", fmt.Errorf("resolve worker placement claim execution directory: %w", err)
	}
	candidate := trimmed
	if !filepath.IsAbs(candidate) {
		if filepath.Ext(candidate) == "" && !strings.Contains(candidate, string(os.PathSeparator)) {
			candidate += ".json"
		}
		candidate = filepath.Join(dir, candidate)
	}
	candidate, err = filepath.Abs(candidate)
	if err != nil {
		return "", fmt.Errorf("resolve worker placement claim execution path: %w", err)
	}
	rel, err := filepath.Rel(dir, candidate)
	if err != nil {
		return "", fmt.Errorf("resolve worker placement claim execution relative path: %w", err)
	}
	if rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("worker placement claim execution path must stay under %s", dir)
	}
	if filepath.Ext(candidate) != ".json" {
		return "", fmt.Errorf("worker placement claim execution path must be a json file")
	}
	return candidate, nil
}

func resolveRuntimeWorkerPlacementDispatchPlanPath(root, planPath string) (string, error) {
	trimmed := strings.TrimSpace(planPath)
	if trimmed == "" {
		return "", fmt.Errorf("worker placement dispatch plan path is required")
	}
	dir, err := filepath.Abs(runtimeWorkerPlacementDispatchPlanDir(root))
	if err != nil {
		return "", fmt.Errorf("resolve worker placement dispatch plan directory: %w", err)
	}
	candidate := trimmed
	if !filepath.IsAbs(candidate) {
		if filepath.Ext(candidate) == "" && !strings.Contains(candidate, string(os.PathSeparator)) {
			candidate += ".json"
		}
		candidate = filepath.Join(dir, candidate)
	}
	candidate, err = filepath.Abs(candidate)
	if err != nil {
		return "", fmt.Errorf("resolve worker placement dispatch plan path: %w", err)
	}
	rel, err := filepath.Rel(dir, candidate)
	if err != nil {
		return "", fmt.Errorf("resolve worker placement dispatch plan relative path: %w", err)
	}
	if rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("worker placement dispatch plan path must stay under %s", dir)
	}
	if filepath.Ext(candidate) != ".json" {
		return "", fmt.Errorf("worker placement dispatch plan path must be a json file")
	}
	return candidate, nil
}

func resolveRuntimeWorkerPlacementDispatchExecutionPath(root, executionPath string) (string, error) {
	trimmed := strings.TrimSpace(executionPath)
	if trimmed == "" {
		return "", fmt.Errorf("worker placement dispatch execution path is required")
	}
	dir, err := filepath.Abs(runtimeWorkerPlacementDispatchExecutionDir(root))
	if err != nil {
		return "", fmt.Errorf("resolve worker placement dispatch execution directory: %w", err)
	}
	candidate := trimmed
	if !filepath.IsAbs(candidate) {
		if filepath.Ext(candidate) == "" && !strings.Contains(candidate, string(os.PathSeparator)) {
			candidate += ".json"
		}
		candidate = filepath.Join(dir, candidate)
	}
	candidate, err = filepath.Abs(candidate)
	if err != nil {
		return "", fmt.Errorf("resolve worker placement dispatch execution path: %w", err)
	}
	rel, err := filepath.Rel(dir, candidate)
	if err != nil {
		return "", fmt.Errorf("resolve worker placement dispatch execution relative path: %w", err)
	}
	if rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("worker placement dispatch execution path must stay under %s", dir)
	}
	if filepath.Ext(candidate) != ".json" {
		return "", fmt.Errorf("worker placement dispatch execution path must be a json file")
	}
	return candidate, nil
}

func resolveRuntimeWorkerPlacementDispatchBatchPath(root, batchPath string) (string, error) {
	trimmed := strings.TrimSpace(batchPath)
	if trimmed == "" {
		return "", fmt.Errorf("worker placement dispatch batch path is required")
	}
	dir, err := filepath.Abs(runtimeWorkerPlacementDispatchBatchDir(root))
	if err != nil {
		return "", fmt.Errorf("resolve worker placement dispatch batch directory: %w", err)
	}
	candidate := trimmed
	if !filepath.IsAbs(candidate) {
		if filepath.Ext(candidate) == "" && !strings.Contains(candidate, string(os.PathSeparator)) {
			candidate += ".json"
		}
		candidate = filepath.Join(dir, candidate)
	}
	candidate, err = filepath.Abs(candidate)
	if err != nil {
		return "", fmt.Errorf("resolve worker placement dispatch batch path: %w", err)
	}
	rel, err := filepath.Rel(dir, candidate)
	if err != nil {
		return "", fmt.Errorf("resolve worker placement dispatch batch relative path: %w", err)
	}
	if rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("worker placement dispatch batch path must stay under %s", dir)
	}
	if filepath.Ext(candidate) != ".json" {
		return "", fmt.Errorf("worker placement dispatch batch path must be a json file")
	}
	return candidate, nil
}

func runtimeWorkerPlacementExecutionID(node compiler.PlanNode) string {
	return "tool-" + strings.ReplaceAll(node.ID, ":", "-")
}

func safeRuntimeWorkerPlacementPlanID(runID string, now time.Time) string {
	return safeInstallRequestName(strings.Join([]string{
		"worker-placement-plan",
		runID,
		now.UTC().Format(time.RFC3339Nano),
	}, "|"))
}

func safeRuntimeWorkerPlacementDispatchPlanID(now time.Time) string {
	return safeInstallRequestName(strings.Join([]string{
		"worker-placement-dispatch-plan",
		now.UTC().Format(time.RFC3339Nano),
	}, "|"))
}

func safeRuntimeWorkerPlacementDispatchExecutionID(execution RuntimeWorkerPlacementDispatchExecution) string {
	return safeInstallRequestName(strings.Join([]string{
		"worker-placement-dispatch",
		execution.PlanID,
		execution.TransportMode,
		execution.ClaimID,
		execution.WorkerID,
		execution.ExecutionID,
		execution.ExecutedAt,
		execution.SkipReason,
	}, "|"))
}

func safeRuntimeWorkerPlacementDispatchBatchID(batch RuntimeWorkerPlacementDispatchBatch) string {
	return safeInstallRequestName(strings.Join([]string{
		"worker-placement-dispatch-batch",
		batch.PlanID,
		batch.TransportMode,
		batch.ExecutedAt,
		fmt.Sprintf("%d", batch.TotalCount),
		fmt.Sprintf("%d", batch.DispatchedCount),
		fmt.Sprintf("%d", batch.SkippedCount),
		fmt.Sprintf("%d", batch.FailedCount),
	}, "|"))
}

func safeRuntimeWorkerPlacementClaimExecutionID(execution RuntimeWorkerPlacementClaimExecution) string {
	return safeInstallRequestName(strings.Join([]string{
		"worker-placement-claim",
		execution.PlanID,
		execution.WorkerID,
		execution.ExecutionID,
		execution.ExecutedAt,
	}, "|"))
}
