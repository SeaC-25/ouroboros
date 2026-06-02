package app

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type WorkerLeaseResultReconciliationPlan struct {
	Ready   []WorkerLeaseResultReconciliationCandidate `json:"ready"`
	Skipped []WorkerLeaseResultReconciliationSkip      `json:"skipped"`
}

type WorkerLeaseResultReconciliationCandidate struct {
	Lease             WorkerLeaseArtifact                 `json:"lease"`
	LeasePath         string                              `json:"lease_path,omitempty"`
	Dispatch          RuntimeWorkerQueueDispatchExecution `json:"dispatch"`
	DispatchPath      string                              `json:"dispatch_path,omitempty"`
	Result            RuntimeWorkerQueueDispatchResult    `json:"result"`
	ResultPath        string                              `json:"result_path,omitempty"`
	RecommendedStatus string                              `json:"recommended_status"`
	Reason            string                              `json:"reason"`
	Mode              string                              `json:"mode"`
}

type WorkerLeaseResultReconciliationSkip struct {
	Lease        WorkerLeaseArtifact                 `json:"lease,omitempty"`
	LeasePath    string                              `json:"lease_path,omitempty"`
	Dispatch     RuntimeWorkerQueueDispatchExecution `json:"dispatch,omitempty"`
	DispatchPath string                              `json:"dispatch_path,omitempty"`
	Result       RuntimeWorkerQueueDispatchResult    `json:"result"`
	ResultPath   string                              `json:"result_path,omitempty"`
	Reason       string                              `json:"reason"`
	Mode         string                              `json:"mode"`
}

type WorkerLeaseResultReconciliationDispatch struct {
	Version   int                                        `json:"version"`
	Path      string                                     `json:"-"`
	Status    string                                     `json:"status"`
	Mode      string                                     `json:"mode"`
	CreatedAt string                                     `json:"created_at"`
	Ready     []WorkerLeaseResultReconciliationCandidate `json:"ready"`
	Skipped   []WorkerLeaseResultReconciliationSkip      `json:"skipped"`
}

type WorkerLeaseResultReconciliationExecutionOptions struct {
	Execute    bool
	Now        time.Time
	ReleasedBy string
	Reason     string
}

type WorkerLeaseResultReconciliationExecution struct {
	Version      int                                              `json:"version"`
	Path         string                                           `json:"-"`
	Status       string                                           `json:"status"`
	Mode         string                                           `json:"mode"`
	DispatchPath string                                           `json:"dispatch_path"`
	ExecutedAt   string                                           `json:"executed_at"`
	Results      []WorkerLeaseResultReconciliationExecutionResult `json:"results"`
	Skipped      []WorkerLeaseResultReconciliationSkip            `json:"skipped"`
}

type WorkerLeaseResultReconciliationExecutionResult struct {
	LeasePath         string `json:"lease_path"`
	ResultPath        string `json:"result_path"`
	DispatchPath      string `json:"dispatch_path"`
	RunID             string `json:"run_id"`
	ExecutionID       string `json:"execution_id"`
	Tool              string `json:"tool"`
	Capability        string `json:"capability"`
	WorkerID          string `json:"worker_id"`
	PreviousStatus    string `json:"previous_status"`
	Status            string `json:"status"`
	ResultStatus      string `json:"result_status"`
	RecommendedStatus string `json:"recommended_status"`
}

func PlanWorkerLeaseResultReconciliation(root string) (WorkerLeaseResultReconciliationPlan, error) {
	results, err := ListRuntimeWorkerQueueDispatchResults(root)
	if err != nil {
		return WorkerLeaseResultReconciliationPlan{}, err
	}
	leases, err := ListWorkerLeases(root)
	if err != nil {
		return WorkerLeaseResultReconciliationPlan{}, err
	}
	dispatches, err := ListRuntimeWorkerQueueDispatchExecutions(root)
	if err != nil {
		return WorkerLeaseResultReconciliationPlan{}, err
	}
	leaseByIdentity := map[string]WorkerLeaseArtifact{}
	for _, lease := range leases {
		leaseByIdentity[workerLeaseReconciliationIdentity(lease.RunID, lease.ExecutionID)] = lease
	}
	dispatchByPath := map[string]RuntimeWorkerQueueDispatchExecution{}
	for _, dispatch := range dispatches {
		dispatchByPath[strings.TrimSpace(dispatch.Path)] = dispatch
		dispatchByPath[filepath.Base(strings.TrimSpace(dispatch.Path))] = dispatch
	}

	var plan WorkerLeaseResultReconciliationPlan
	for _, result := range results {
		dispatch := dispatchByPath[strings.TrimSpace(result.DispatchExecutionPath)]
		lease, ok := leaseByIdentity[workerLeaseReconciliationIdentity(result.RunID, result.ExecutionID)]
		if !ok {
			plan.Skipped = append(plan.Skipped, WorkerLeaseResultReconciliationSkip{
				Dispatch:     dispatch,
				DispatchPath: dispatch.Path,
				Result:       result,
				ResultPath:   result.Path,
				Reason:       "lease-missing",
				Mode:         "metadata-only",
			})
			continue
		}
		leaseStatus := strings.TrimSpace(lease.Status)
		resultStatus := strings.TrimSpace(result.Status)
		if leaseStatus == "active" {
			plan.Ready = append(plan.Ready, WorkerLeaseResultReconciliationCandidate{
				Lease:             lease,
				LeasePath:         lease.Path,
				Dispatch:          dispatch,
				DispatchPath:      dispatch.Path,
				Result:            result,
				ResultPath:        result.Path,
				RecommendedStatus: resultStatus,
				Reason:            "result-returned-active-lease",
				Mode:              "metadata-only",
			})
			continue
		}
		reason := "lease-status-mismatch"
		if leaseStatus == resultStatus {
			reason = "lease-aligned"
		}
		plan.Skipped = append(plan.Skipped, WorkerLeaseResultReconciliationSkip{
			Lease:        lease,
			LeasePath:    lease.Path,
			Dispatch:     dispatch,
			DispatchPath: dispatch.Path,
			Result:       result,
			ResultPath:   result.Path,
			Reason:       reason,
			Mode:         "metadata-only",
		})
	}
	sort.SliceStable(plan.Ready, func(i, j int) bool {
		return workerLeaseResultReconciliationSortKey(plan.Ready[i].Result) < workerLeaseResultReconciliationSortKey(plan.Ready[j].Result)
	})
	sort.SliceStable(plan.Skipped, func(i, j int) bool {
		return workerLeaseResultReconciliationSortKey(plan.Skipped[i].Result) < workerLeaseResultReconciliationSortKey(plan.Skipped[j].Result)
	})
	return plan, nil
}

func CreateWorkerLeaseResultReconciliationDispatch(root string, now time.Time) (WorkerLeaseResultReconciliationDispatch, error) {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	plan, err := PlanWorkerLeaseResultReconciliation(root)
	if err != nil {
		return WorkerLeaseResultReconciliationDispatch{}, err
	}
	dispatch := WorkerLeaseResultReconciliationDispatch{
		Version:   1,
		Status:    "dry-run",
		Mode:      "metadata-only",
		CreatedAt: now.UTC().Format(time.RFC3339Nano),
		Ready:     append([]WorkerLeaseResultReconciliationCandidate(nil), plan.Ready...),
		Skipped:   append([]WorkerLeaseResultReconciliationSkip(nil), plan.Skipped...),
	}
	dir := workerLeaseResultReconciliationDispatchDir(root)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return WorkerLeaseResultReconciliationDispatch{}, fmt.Errorf("create worker lease result reconciliation dispatch directory: %w", err)
	}
	dispatch.Path = filepath.Join(dir, safeWorkerLeaseResultReconciliationDispatchName(dispatch)+".json")
	data, err := json.MarshalIndent(dispatch, "", "  ")
	if err != nil {
		return WorkerLeaseResultReconciliationDispatch{}, fmt.Errorf("encode worker lease result reconciliation dispatch: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(dispatch.Path, data, 0o644); err != nil {
		return WorkerLeaseResultReconciliationDispatch{}, fmt.Errorf("write worker lease result reconciliation dispatch: %w", err)
	}
	return dispatch, nil
}

func ExecuteWorkerLeaseResultReconciliationDispatch(root, dispatchPath string, options WorkerLeaseResultReconciliationExecutionOptions) (WorkerLeaseResultReconciliationExecution, error) {
	if !options.Execute {
		return WorkerLeaseResultReconciliationExecution{}, fmt.Errorf("worker lease result reconciliation execution requires explicit execute")
	}
	dispatch, err := ReadWorkerLeaseResultReconciliationDispatch(root, dispatchPath)
	if err != nil {
		return WorkerLeaseResultReconciliationExecution{}, err
	}
	if strings.TrimSpace(dispatch.Status) != "dry-run" || strings.TrimSpace(dispatch.Mode) != "metadata-only" {
		return WorkerLeaseResultReconciliationExecution{}, fmt.Errorf("worker lease result reconciliation dispatch must be dry-run metadata-only before execution")
	}
	now := options.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if err := requireRuntimeWorkerSideEffectSafetyAuthorization(root, now); err != nil {
		return WorkerLeaseResultReconciliationExecution{}, err
	}
	execution := WorkerLeaseResultReconciliationExecution{
		Version:      1,
		Status:       "skipped",
		Mode:         "metadata-only",
		DispatchPath: dispatch.Path,
		ExecutedAt:   now.UTC().Format(time.RFC3339Nano),
		Skipped:      append([]WorkerLeaseResultReconciliationSkip(nil), dispatch.Skipped...),
	}
	for _, candidate := range dispatch.Ready {
		result, ok := workerLeaseResultReconciliationCurrentResult(root, candidate)
		if !ok {
			execution.Skipped = append(execution.Skipped, workerLeaseResultReconciliationSkipFromCandidate(candidate, WorkerLeaseArtifact{}, RuntimeWorkerQueueDispatchResult{}, "result-unavailable"))
			continue
		}
		if strings.TrimSpace(result.RunID) != strings.TrimSpace(candidate.Result.RunID) || strings.TrimSpace(result.ExecutionID) != strings.TrimSpace(candidate.Result.ExecutionID) {
			execution.Skipped = append(execution.Skipped, workerLeaseResultReconciliationSkipFromCandidate(candidate, WorkerLeaseArtifact{}, result, "result-identity-mismatch"))
			continue
		}
		status := strings.TrimSpace(candidate.RecommendedStatus)
		if status == "" {
			status = strings.TrimSpace(result.Status)
		}
		if strings.TrimSpace(result.Status) != "" && status != strings.TrimSpace(result.Status) {
			execution.Skipped = append(execution.Skipped, workerLeaseResultReconciliationSkipFromCandidate(candidate, WorkerLeaseArtifact{}, result, "result-status-mismatch"))
			continue
		}
		if status != "completed" && status != "failed" {
			execution.Skipped = append(execution.Skipped, workerLeaseResultReconciliationSkipFromCandidate(candidate, WorkerLeaseArtifact{}, result, "unsupported-result-status"))
			continue
		}
		lease, err := readWorkerLeaseByIdentity(root, candidate.Result.RunID, candidate.Result.ExecutionID)
		if err != nil {
			execution.Skipped = append(execution.Skipped, workerLeaseResultReconciliationSkipFromCandidate(candidate, WorkerLeaseArtifact{}, result, "lease-missing"))
			continue
		}
		if strings.TrimSpace(lease.Status) != "active" {
			execution.Skipped = append(execution.Skipped, workerLeaseResultReconciliationSkipFromCandidate(candidate, lease, result, "lease-not-active"))
			continue
		}
		releasedBy := strings.TrimSpace(options.ReleasedBy)
		if releasedBy == "" {
			releasedBy = "worker-lease-result-reconciliation"
		}
		reason := strings.TrimSpace(options.Reason)
		if reason == "" {
			reason = strings.TrimSpace(candidate.Reason)
		}
		resultPath := workerLeaseResultReconciliationDisplayPath(candidate.ResultPath, result.Path)
		resultExecutionID := filepath.Base(resultPath)
		if strings.TrimSpace(resultExecutionID) == "." || strings.TrimSpace(resultExecutionID) == "" {
			resultExecutionID = strings.TrimSpace(result.ExecutionID)
		}
		released, err := ReleaseWorkerLease(root, candidate.Result.RunID, candidate.Result.ExecutionID, WorkerLeaseReleaseOptions{
			Release:           true,
			Status:            status,
			Now:               now,
			ReleasedBy:        releasedBy,
			Reason:            reason,
			Error:             result.Error,
			ResultExecutionID: resultExecutionID,
		})
		if err != nil {
			execution.Skipped = append(execution.Skipped, workerLeaseResultReconciliationSkipFromCandidate(candidate, lease, result, "release-failed"))
			continue
		}
		execution.Results = append(execution.Results, WorkerLeaseResultReconciliationExecutionResult{
			LeasePath:         workerLeaseResultReconciliationDisplayPath(candidate.LeasePath, released.Path),
			ResultPath:        resultPath,
			DispatchPath:      workerLeaseResultReconciliationDisplayPath(candidate.DispatchPath, candidate.Dispatch.Path),
			RunID:             released.RunID,
			ExecutionID:       released.ExecutionID,
			Tool:              released.Tool,
			Capability:        released.Capability,
			WorkerID:          released.WorkerID,
			PreviousStatus:    lease.Status,
			Status:            released.Status,
			ResultStatus:      result.Status,
			RecommendedStatus: status,
		})
	}
	execution.Status = workerLeaseResultReconciliationExecutionStatus(execution)
	if err := writeWorkerLeaseResultReconciliationExecution(root, &execution); err != nil {
		return WorkerLeaseResultReconciliationExecution{}, err
	}
	return execution, nil
}

func ListWorkerLeaseResultReconciliationDispatches(root string) ([]WorkerLeaseResultReconciliationDispatch, error) {
	paths, err := filepath.Glob(filepath.Join(workerLeaseResultReconciliationDispatchDir(root), "*.json"))
	if err != nil {
		return nil, fmt.Errorf("glob worker lease result reconciliation dispatches: %w", err)
	}
	sort.Strings(paths)
	dispatches := make([]WorkerLeaseResultReconciliationDispatch, 0, len(paths))
	for _, path := range paths {
		dispatch, err := readWorkerLeaseResultReconciliationDispatchFile(path)
		if err != nil {
			return nil, err
		}
		dispatches = append(dispatches, dispatch)
	}
	return dispatches, nil
}

func ReadWorkerLeaseResultReconciliationDispatch(root, dispatchPath string) (WorkerLeaseResultReconciliationDispatch, error) {
	path, err := resolveWorkerLeaseResultReconciliationDispatchPath(root, dispatchPath)
	if err != nil {
		return WorkerLeaseResultReconciliationDispatch{}, err
	}
	return readWorkerLeaseResultReconciliationDispatchFile(path)
}

func ListWorkerLeaseResultReconciliationExecutions(root string) ([]WorkerLeaseResultReconciliationExecution, error) {
	paths, err := filepath.Glob(filepath.Join(workerLeaseResultReconciliationExecutionDir(root), "*.json"))
	if err != nil {
		return nil, fmt.Errorf("glob worker lease result reconciliation executions: %w", err)
	}
	sort.Strings(paths)
	executions := make([]WorkerLeaseResultReconciliationExecution, 0, len(paths))
	for _, path := range paths {
		execution, err := readWorkerLeaseResultReconciliationExecutionFile(path)
		if err != nil {
			return nil, err
		}
		executions = append(executions, execution)
	}
	return executions, nil
}

func ReadWorkerLeaseResultReconciliationExecution(root, executionPath string) (WorkerLeaseResultReconciliationExecution, error) {
	path, err := resolveWorkerLeaseResultReconciliationExecutionPath(root, executionPath)
	if err != nil {
		return WorkerLeaseResultReconciliationExecution{}, err
	}
	return readWorkerLeaseResultReconciliationExecutionFile(path)
}

func readWorkerLeaseResultReconciliationDispatchFile(path string) (WorkerLeaseResultReconciliationDispatch, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return WorkerLeaseResultReconciliationDispatch{}, fmt.Errorf("read worker lease result reconciliation dispatch %s: %w", path, err)
	}
	var dispatch WorkerLeaseResultReconciliationDispatch
	if err := json.Unmarshal(data, &dispatch); err != nil {
		return WorkerLeaseResultReconciliationDispatch{}, fmt.Errorf("decode worker lease result reconciliation dispatch %s: %w", path, err)
	}
	dispatch.Path = path
	return dispatch, nil
}

func readWorkerLeaseResultReconciliationExecutionFile(path string) (WorkerLeaseResultReconciliationExecution, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return WorkerLeaseResultReconciliationExecution{}, fmt.Errorf("read worker lease result reconciliation execution %s: %w", path, err)
	}
	var execution WorkerLeaseResultReconciliationExecution
	if err := json.Unmarshal(data, &execution); err != nil {
		return WorkerLeaseResultReconciliationExecution{}, fmt.Errorf("decode worker lease result reconciliation execution %s: %w", path, err)
	}
	execution.Path = path
	return execution, nil
}

func writeWorkerLeaseResultReconciliationExecution(root string, execution *WorkerLeaseResultReconciliationExecution) error {
	dir := workerLeaseResultReconciliationExecutionDir(root)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create worker lease result reconciliation execution directory: %w", err)
	}
	if strings.TrimSpace(execution.Path) == "" {
		execution.Path = filepath.Join(dir, safeWorkerLeaseResultReconciliationExecutionName(*execution)+".json")
	}
	data, err := json.MarshalIndent(execution, "", "  ")
	if err != nil {
		return fmt.Errorf("encode worker lease result reconciliation execution: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(execution.Path, data, 0o644); err != nil {
		return fmt.Errorf("write worker lease result reconciliation execution: %w", err)
	}
	return nil
}

func workerLeaseResultReconciliationCurrentResult(root string, candidate WorkerLeaseResultReconciliationCandidate) (RuntimeWorkerQueueDispatchResult, bool) {
	resultPath := workerLeaseResultReconciliationDisplayPath(candidate.ResultPath, candidate.Result.Path)
	if strings.TrimSpace(resultPath) == "" {
		return candidate.Result, strings.TrimSpace(candidate.Result.Status) != ""
	}
	result, err := ReadRuntimeWorkerQueueDispatchResult(root, resultPath)
	if err != nil {
		return RuntimeWorkerQueueDispatchResult{}, false
	}
	return result, true
}

func workerLeaseResultReconciliationSkipFromCandidate(candidate WorkerLeaseResultReconciliationCandidate, lease WorkerLeaseArtifact, result RuntimeWorkerQueueDispatchResult, reason string) WorkerLeaseResultReconciliationSkip {
	if strings.TrimSpace(lease.RunID) == "" {
		lease = candidate.Lease
	}
	if strings.TrimSpace(result.RunID) == "" {
		result = candidate.Result
	}
	return WorkerLeaseResultReconciliationSkip{
		Lease:        lease,
		LeasePath:    workerLeaseResultReconciliationDisplayPath(candidate.LeasePath, lease.Path),
		Dispatch:     candidate.Dispatch,
		DispatchPath: workerLeaseResultReconciliationDisplayPath(candidate.DispatchPath, candidate.Dispatch.Path),
		Result:       result,
		ResultPath:   workerLeaseResultReconciliationDisplayPath(candidate.ResultPath, result.Path),
		Reason:       reason,
		Mode:         "metadata-only",
	}
}

func workerLeaseResultReconciliationExecutionStatus(execution WorkerLeaseResultReconciliationExecution) string {
	if len(execution.Results) == 0 {
		return "skipped"
	}
	if len(execution.Skipped) > 0 {
		return "partial"
	}
	return "completed"
}

func workerLeaseReconciliationIdentity(runID, executionID string) string {
	return strings.TrimSpace(runID) + "\x00" + strings.TrimSpace(executionID)
}

func workerLeaseResultReconciliationSortKey(result RuntimeWorkerQueueDispatchResult) string {
	return strings.TrimSpace(result.RunID) + "\x00" + strings.TrimSpace(result.ExecutionID) + "\x00" + strings.TrimSpace(result.Path)
}

func workerLeaseResultReconciliationDispatchDir(root string) string {
	return filepath.Join(root, "runtime", "worker-lease-result-reconciliations")
}

func workerLeaseResultReconciliationExecutionDir(root string) string {
	return filepath.Join(root, "runtime", "worker-lease-result-reconciliation-executions")
}

func resolveWorkerLeaseResultReconciliationDispatchPath(root, dispatchPath string) (string, error) {
	trimmed := strings.TrimSpace(dispatchPath)
	if trimmed == "" {
		return "", fmt.Errorf("worker lease result reconciliation dispatch path is required")
	}
	dir, err := filepath.Abs(workerLeaseResultReconciliationDispatchDir(root))
	if err != nil {
		return "", fmt.Errorf("resolve worker lease result reconciliation dispatch directory: %w", err)
	}
	candidate := trimmed
	if !filepath.IsAbs(candidate) {
		candidate = filepath.Join(dir, candidate)
	}
	candidate, err = filepath.Abs(candidate)
	if err != nil {
		return "", fmt.Errorf("resolve worker lease result reconciliation dispatch path: %w", err)
	}
	rel, err := filepath.Rel(dir, candidate)
	if err != nil {
		return "", fmt.Errorf("resolve worker lease result reconciliation dispatch relative path: %w", err)
	}
	if rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("worker lease result reconciliation dispatch path must stay under %s", dir)
	}
	if filepath.Ext(candidate) != ".json" {
		return "", fmt.Errorf("worker lease result reconciliation dispatch path must be a json file")
	}
	return candidate, nil
}

func resolveWorkerLeaseResultReconciliationExecutionPath(root, executionPath string) (string, error) {
	trimmed := strings.TrimSpace(executionPath)
	if trimmed == "" {
		return "", fmt.Errorf("worker lease result reconciliation execution path is required")
	}
	dir, err := filepath.Abs(workerLeaseResultReconciliationExecutionDir(root))
	if err != nil {
		return "", fmt.Errorf("resolve worker lease result reconciliation execution directory: %w", err)
	}
	candidate := trimmed
	if !filepath.IsAbs(candidate) {
		candidate = filepath.Join(dir, candidate)
	}
	candidate, err = filepath.Abs(candidate)
	if err != nil {
		return "", fmt.Errorf("resolve worker lease result reconciliation execution path: %w", err)
	}
	rel, err := filepath.Rel(dir, candidate)
	if err != nil {
		return "", fmt.Errorf("resolve worker lease result reconciliation execution relative path: %w", err)
	}
	if rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("worker lease result reconciliation execution path must stay under %s", dir)
	}
	if filepath.Ext(candidate) != ".json" {
		return "", fmt.Errorf("worker lease result reconciliation execution path must be a json file")
	}
	return candidate, nil
}

func workerLeaseResultReconciliationDisplayPath(primary, fallback string) string {
	if strings.TrimSpace(primary) != "" {
		return primary
	}
	return fallback
}

func safeWorkerLeaseResultReconciliationDispatchName(dispatch WorkerLeaseResultReconciliationDispatch) string {
	parts := []string{
		"worker-lease-result-reconciliation",
		dispatch.CreatedAt,
		fmt.Sprintf("ready=%d skipped=%d", len(dispatch.Ready), len(dispatch.Skipped)),
	}
	for _, candidate := range dispatch.Ready {
		parts = append(
			parts,
			candidate.LeasePath,
			candidate.ResultPath,
			candidate.DispatchPath,
			candidate.Result.RunID,
			candidate.Result.ExecutionID,
			candidate.Result.Status,
			candidate.Lease.Status,
			candidate.RecommendedStatus,
			candidate.Reason,
			candidate.Mode,
		)
	}
	for _, skipped := range dispatch.Skipped {
		parts = append(
			parts,
			skipped.LeasePath,
			skipped.ResultPath,
			skipped.DispatchPath,
			skipped.Result.RunID,
			skipped.Result.ExecutionID,
			skipped.Result.Status,
			skipped.Lease.Status,
			skipped.Reason,
			skipped.Mode,
		)
	}
	return safeInstallRequestName(strings.Join(parts, "|"))
}

func safeWorkerLeaseResultReconciliationExecutionName(execution WorkerLeaseResultReconciliationExecution) string {
	parts := []string{
		"worker-lease-result-reconciliation-execution",
		execution.DispatchPath,
		execution.ExecutedAt,
		fmt.Sprintf("results=%d skipped=%d", len(execution.Results), len(execution.Skipped)),
	}
	for _, result := range execution.Results {
		parts = append(parts, result.LeasePath, result.ResultPath, result.DispatchPath, result.RunID, result.ExecutionID, result.WorkerID, result.Status)
	}
	for _, skipped := range execution.Skipped {
		parts = append(parts, skipped.LeasePath, skipped.ResultPath, skipped.DispatchPath, skipped.Result.RunID, skipped.Result.ExecutionID, skipped.Reason)
	}
	return safeInstallRequestName(strings.Join(parts, "|"))
}
