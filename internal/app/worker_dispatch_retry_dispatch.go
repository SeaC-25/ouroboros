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

type RuntimeWorkerDispatchRetryDispatch struct {
	Version       int                                   `json:"version"`
	Path          string                                `json:"-"`
	Status        string                                `json:"status"`
	Mode          string                                `json:"mode"`
	CreatedAt     string                                `json:"created_at"`
	RetryPlanPath string                                `json:"retry_plan_path"`
	Summary       RuntimeWorkerDispatchRetrySummary     `json:"summary"`
	Ready         []RuntimeWorkerDispatchRetryCandidate `json:"ready"`
	Skipped       []RuntimeWorkerDispatchRetrySkip      `json:"skipped"`
}

type RuntimeWorkerDispatchRetryDispatchExecutionOptions struct {
	Execute                bool
	Now                    time.Time
	LeaseDuration          time.Duration
	DispatchClaims         bool
	TransportMode          string
	ResultCallbackEndpoint string
}

type RuntimeWorkerDispatchRetryDispatchExecutionSummary struct {
	Total      int `json:"total"`
	Ready      int `json:"ready"`
	Claimed    int `json:"claimed"`
	Dispatched int `json:"dispatched,omitempty"`
	Skipped    int `json:"skipped"`
	Failed     int `json:"failed"`
}

type RuntimeWorkerDispatchRetryDispatchExecutionResult struct {
	Status                    string                       `json:"status"`
	Reason                    string                       `json:"reason,omitempty"`
	Error                     string                       `json:"error,omitempty"`
	SourceResultPath          string                       `json:"source_result_path"`
	SourceDispatchPath        string                       `json:"source_dispatch_path"`
	SourceClaimPath           string                       `json:"source_claim_path"`
	TransportRequestPath      string                       `json:"transport_request_path,omitempty"`
	RetryClaimPath            string                       `json:"retry_claim_path,omitempty"`
	RetryClaimID              string                       `json:"retry_claim_id,omitempty"`
	RetryQueueDispatchPath    string                       `json:"retry_queue_dispatch_path,omitempty"`
	RetryTransportMode        string                       `json:"retry_transport_mode,omitempty"`
	RetryTransportRequestPath string                       `json:"retry_transport_request_path,omitempty"`
	WorkerID                  string                       `json:"worker_id"`
	RunID                     string                       `json:"run_id"`
	ExecutionID               string                       `json:"execution_id"`
	Tool                      string                       `json:"tool"`
	Capability                string                       `json:"capability"`
	ToolPlan                  RuntimeWorkerToolPlanPayload `json:"tool_plan,omitempty"`
	LastStatus                string                       `json:"last_status,omitempty"`
	LastOutput                string                       `json:"last_output,omitempty"`
	LastError                 string                       `json:"last_error,omitempty"`
}

type RuntimeWorkerDispatchRetryDispatchExecution struct {
	Version           int                                                 `json:"version"`
	Path              string                                              `json:"-"`
	Status            string                                              `json:"status"`
	Mode              string                                              `json:"mode"`
	RetryDispatchPath string                                              `json:"retry_dispatch_path"`
	ExecutedAt        string                                              `json:"executed_at"`
	Summary           RuntimeWorkerDispatchRetryDispatchExecutionSummary  `json:"summary"`
	Results           []RuntimeWorkerDispatchRetryDispatchExecutionResult `json:"results,omitempty"`
	Skipped           []RuntimeWorkerDispatchRetrySkip                    `json:"skipped,omitempty"`
}

func CreateRuntimeWorkerDispatchRetryDispatch(root, planPath string, now time.Time) (RuntimeWorkerDispatchRetryDispatch, error) {
	if now.IsZero() {
		now = time.Now().UTC()
	} else {
		now = now.UTC()
	}
	plan, err := ReadRuntimeWorkerDispatchRetryPlan(root, planPath)
	if err != nil {
		return RuntimeWorkerDispatchRetryDispatch{}, err
	}
	dispatch := RuntimeWorkerDispatchRetryDispatch{
		Version:       1,
		Status:        "dry-run",
		Mode:          "metadata-only",
		CreatedAt:     now.Format(time.RFC3339Nano),
		RetryPlanPath: plan.Path,
		Summary:       plan.Summary,
		Ready:         append([]RuntimeWorkerDispatchRetryCandidate(nil), plan.Ready...),
		Skipped:       append([]RuntimeWorkerDispatchRetrySkip(nil), plan.Skipped...),
	}
	dir := runtimeWorkerDispatchRetryDispatchDir(root)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return RuntimeWorkerDispatchRetryDispatch{}, fmt.Errorf("create worker dispatch retry dispatch directory: %w", err)
	}
	dispatch.Path = filepath.Join(dir, safeRuntimeWorkerDispatchRetryDispatchName(dispatch)+".json")
	data, err := json.MarshalIndent(dispatch, "", "  ")
	if err != nil {
		return RuntimeWorkerDispatchRetryDispatch{}, fmt.Errorf("encode worker dispatch retry dispatch: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(dispatch.Path, data, 0o644); err != nil {
		return RuntimeWorkerDispatchRetryDispatch{}, fmt.Errorf("write worker dispatch retry dispatch: %w", err)
	}
	return dispatch, nil
}

func ExecuteRuntimeWorkerDispatchRetryDispatch(root, dispatchPath string, options RuntimeWorkerDispatchRetryDispatchExecutionOptions) (RuntimeWorkerDispatchRetryDispatchExecution, error) {
	if !options.Execute {
		return RuntimeWorkerDispatchRetryDispatchExecution{}, fmt.Errorf("worker dispatch retry dispatch execution requires explicit execute")
	}
	now := options.Now.UTC()
	if now.IsZero() {
		now = time.Now().UTC()
	}
	dispatch, err := ReadRuntimeWorkerDispatchRetryDispatch(root, dispatchPath)
	if err != nil {
		return RuntimeWorkerDispatchRetryDispatchExecution{}, err
	}
	if strings.TrimSpace(dispatch.Status) != "dry-run" || strings.TrimSpace(dispatch.Mode) != "metadata-only" {
		return RuntimeWorkerDispatchRetryDispatchExecution{}, fmt.Errorf("worker dispatch retry dispatch must be dry-run metadata-only before execution")
	}
	transportMode := ""
	if options.DispatchClaims {
		var err error
		transportMode, err = normalizeRuntimeWorkerQueueDispatchTransport(options.TransportMode)
		if err != nil {
			return RuntimeWorkerDispatchRetryDispatchExecution{}, err
		}
		if transportMode == "http-json" {
			return RuntimeWorkerDispatchRetryDispatchExecution{}, fmt.Errorf("worker dispatch retry claim handoff supports metadata-only or file-outbox transport")
		}
	}
	if err := requireRuntimeWorkerSideEffectSafetyAuthorization(root, now); err != nil {
		return RuntimeWorkerDispatchRetryDispatchExecution{}, err
	}
	execution := RuntimeWorkerDispatchRetryDispatchExecution{
		Version:           1,
		Status:            "completed",
		Mode:              "execute",
		RetryDispatchPath: dispatch.Path,
		ExecutedAt:        now.Format(time.RFC3339Nano),
		Skipped:           append([]RuntimeWorkerDispatchRetrySkip(nil), dispatch.Skipped...),
	}
	execution.Summary.Total = len(dispatch.Ready) + len(dispatch.Skipped)
	execution.Summary.Ready = len(dispatch.Ready)
	execution.Summary.Skipped = len(dispatch.Skipped)
	for index, candidate := range dispatch.Ready {
		result := runtimeWorkerDispatchRetryDispatchExecutionResultFromCandidate(candidate)
		currentResult, err := ReadRuntimeWorkerQueueDispatchResult(root, candidate.ResultPath)
		if err != nil {
			result.Status = "failed"
			result.Error = err.Error()
			execution.Status = "failed"
			execution.Summary.Failed++
			execution.Results = append(execution.Results, result)
			continue
		}
		if reason := runtimeWorkerDispatchRetryCandidateMismatch(candidate, currentResult); reason != "" {
			result.Status = "skipped"
			result.Reason = reason
			execution.Summary.Skipped++
			execution.Results = append(execution.Results, result)
			continue
		}
		if strings.TrimSpace(currentResult.Status) != "failed" {
			result.Status = "skipped"
			result.Reason = "result-not-failed"
			result.LastStatus = currentResult.Status
			execution.Summary.Skipped++
			execution.Results = append(execution.Results, result)
			continue
		}
		claim, err := CreateRuntimeWorkerQueueClaim(root, RuntimeWorkerQueueClaim{
			WorkerID:    candidate.WorkerID,
			RunID:       candidate.RunID,
			ExecutionID: candidate.ExecutionID,
			Tool:        candidate.Tool,
			Capability:  candidate.Capability,
			ToolPlan:    candidate.ToolPlan,
		}, RuntimeWorkerQueueClaimOptions{
			Now:           now.Add(time.Duration(index) * time.Nanosecond),
			LeaseDuration: options.LeaseDuration,
		})
		if err != nil {
			result.Status = "skipped"
			result.Reason = err.Error()
			execution.Summary.Skipped++
			execution.Results = append(execution.Results, result)
			continue
		}
		result.Status = "claimed"
		result.Reason = "retry-queue-claim-created"
		result.RetryClaimPath = claim.Path
		result.RetryClaimID = claim.ClaimID
		execution.Summary.Claimed++
		if options.DispatchClaims {
			dispatchExecution, err := ExecuteRuntimeWorkerQueueClaimDispatch(root, claim.Path, RuntimeWorkerQueueDispatchExecutionOptions{
				Execute:                true,
				Now:                    now.Add(time.Duration(index)*time.Nanosecond + time.Nanosecond),
				TransportMode:          transportMode,
				ResultCallbackEndpoint: options.ResultCallbackEndpoint,
			})
			if strings.TrimSpace(dispatchExecution.Path) != "" {
				result.RetryQueueDispatchPath = dispatchExecution.Path
				result.RetryTransportMode = dispatchExecution.TransportMode
				result.RetryTransportRequestPath = dispatchExecution.TransportRequestPath
			}
			if err != nil {
				result.Status = "failed"
				result.Error = err.Error()
				execution.Status = "failed"
				execution.Summary.Failed++
				execution.Results = append(execution.Results, result)
				continue
			}
			switch strings.TrimSpace(dispatchExecution.Status) {
			case "dispatched":
				result.Status = "dispatched"
				result.Reason = "retry-queue-claim-dispatched"
				execution.Summary.Dispatched++
			case "skipped":
				result.Status = "claimed"
				result.Reason = "retry-queue-claim-dispatch-skipped"
				if strings.TrimSpace(dispatchExecution.SkipReason) != "" {
					result.Reason += ":" + dispatchExecution.SkipReason
				}
			case "failed":
				result.Status = "failed"
				result.Error = strings.TrimSpace(dispatchExecution.Error)
				if result.Error == "" {
					result.Error = "retry queue claim dispatch failed"
				}
				execution.Status = "failed"
				execution.Summary.Failed++
			}
		}
		execution.Results = append(execution.Results, result)
	}
	if err := writeRuntimeWorkerDispatchRetryDispatchExecution(root, &execution); err != nil {
		return execution, err
	}
	return execution, nil
}

func ListRuntimeWorkerDispatchRetryDispatches(root string) ([]RuntimeWorkerDispatchRetryDispatch, error) {
	paths, err := filepath.Glob(filepath.Join(runtimeWorkerDispatchRetryDispatchDir(root), "*.json"))
	if err != nil {
		return nil, fmt.Errorf("glob worker dispatch retry dispatches: %w", err)
	}
	sort.Strings(paths)
	dispatches := make([]RuntimeWorkerDispatchRetryDispatch, 0, len(paths))
	for _, path := range paths {
		dispatch, err := readRuntimeWorkerDispatchRetryDispatchFile(path)
		if err != nil {
			return nil, err
		}
		dispatches = append(dispatches, dispatch)
	}
	return dispatches, nil
}

func ListRuntimeWorkerDispatchRetryDispatchExecutions(root string) ([]RuntimeWorkerDispatchRetryDispatchExecution, error) {
	paths, err := filepath.Glob(filepath.Join(runtimeWorkerDispatchRetryDispatchExecutionDir(root), "*.json"))
	if err != nil {
		return nil, fmt.Errorf("glob worker dispatch retry dispatch executions: %w", err)
	}
	sort.Strings(paths)
	executions := make([]RuntimeWorkerDispatchRetryDispatchExecution, 0, len(paths))
	for _, path := range paths {
		execution, err := readRuntimeWorkerDispatchRetryDispatchExecutionFile(path)
		if err != nil {
			return nil, err
		}
		executions = append(executions, execution)
	}
	sort.SliceStable(executions, func(i int, j int) bool {
		if executions[i].ExecutedAt != executions[j].ExecutedAt {
			return executions[i].ExecutedAt < executions[j].ExecutedAt
		}
		return executions[i].Path < executions[j].Path
	})
	return executions, nil
}

func ReadRuntimeWorkerDispatchRetryDispatch(root, dispatchPath string) (RuntimeWorkerDispatchRetryDispatch, error) {
	path, err := resolveRuntimeWorkerDispatchRetryDispatchPath(root, dispatchPath)
	if err != nil {
		return RuntimeWorkerDispatchRetryDispatch{}, err
	}
	return readRuntimeWorkerDispatchRetryDispatchFile(path)
}

func ReadRuntimeWorkerDispatchRetryDispatchExecution(root, executionPath string) (RuntimeWorkerDispatchRetryDispatchExecution, error) {
	path, err := resolveRuntimeWorkerDispatchRetryDispatchExecutionPath(root, executionPath)
	if err != nil {
		return RuntimeWorkerDispatchRetryDispatchExecution{}, err
	}
	return readRuntimeWorkerDispatchRetryDispatchExecutionFile(path)
}

func readRuntimeWorkerDispatchRetryDispatchFile(path string) (RuntimeWorkerDispatchRetryDispatch, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return RuntimeWorkerDispatchRetryDispatch{}, fmt.Errorf("read worker dispatch retry dispatch %s: %w", path, err)
	}
	var dispatch RuntimeWorkerDispatchRetryDispatch
	if err := json.Unmarshal(data, &dispatch); err != nil {
		return RuntimeWorkerDispatchRetryDispatch{}, fmt.Errorf("decode worker dispatch retry dispatch %s: %w", path, err)
	}
	dispatch.Path = path
	return dispatch, nil
}

func readRuntimeWorkerDispatchRetryDispatchExecutionFile(path string) (RuntimeWorkerDispatchRetryDispatchExecution, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return RuntimeWorkerDispatchRetryDispatchExecution{}, fmt.Errorf("read worker dispatch retry dispatch execution %s: %w", path, err)
	}
	var execution RuntimeWorkerDispatchRetryDispatchExecution
	if err := json.Unmarshal(data, &execution); err != nil {
		return RuntimeWorkerDispatchRetryDispatchExecution{}, fmt.Errorf("decode worker dispatch retry dispatch execution %s: %w", path, err)
	}
	execution.Path = path
	return execution, nil
}

func runtimeWorkerDispatchRetryDispatchDir(root string) string {
	return filepath.Join(root, "runtime", "worker-dispatch-retry-dispatches")
}

func runtimeWorkerDispatchRetryDispatchExecutionDir(root string) string {
	return filepath.Join(root, "runtime", "worker-dispatch-retry-dispatch-executions")
}

func resolveRuntimeWorkerDispatchRetryDispatchPath(root, dispatchPath string) (string, error) {
	trimmed := strings.TrimSpace(dispatchPath)
	if trimmed == "" {
		return "", fmt.Errorf("worker dispatch retry dispatch path is required")
	}
	dir, err := filepath.Abs(runtimeWorkerDispatchRetryDispatchDir(root))
	if err != nil {
		return "", fmt.Errorf("resolve worker dispatch retry dispatch directory: %w", err)
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
		return "", fmt.Errorf("resolve worker dispatch retry dispatch path: %w", err)
	}
	rel, err := filepath.Rel(dir, candidate)
	if err != nil {
		return "", fmt.Errorf("resolve worker dispatch retry dispatch relative path: %w", err)
	}
	if rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("worker dispatch retry dispatch path must stay under %s", dir)
	}
	if filepath.Ext(candidate) != ".json" {
		return "", fmt.Errorf("worker dispatch retry dispatch path must be a json file")
	}
	return candidate, nil
}

func resolveRuntimeWorkerDispatchRetryDispatchExecutionPath(root, executionPath string) (string, error) {
	trimmed := strings.TrimSpace(executionPath)
	if trimmed == "" {
		return "", fmt.Errorf("worker dispatch retry dispatch execution path is required")
	}
	dir, err := filepath.Abs(runtimeWorkerDispatchRetryDispatchExecutionDir(root))
	if err != nil {
		return "", fmt.Errorf("resolve worker dispatch retry dispatch execution directory: %w", err)
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
		return "", fmt.Errorf("resolve worker dispatch retry dispatch execution path: %w", err)
	}
	rel, err := filepath.Rel(dir, candidate)
	if err != nil {
		return "", fmt.Errorf("resolve worker dispatch retry dispatch execution relative path: %w", err)
	}
	if rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("worker dispatch retry dispatch execution path must stay under %s", dir)
	}
	if filepath.Ext(candidate) != ".json" {
		return "", fmt.Errorf("worker dispatch retry dispatch execution path must be a json file")
	}
	return candidate, nil
}

func writeRuntimeWorkerDispatchRetryDispatchExecution(root string, execution *RuntimeWorkerDispatchRetryDispatchExecution) error {
	dir := runtimeWorkerDispatchRetryDispatchExecutionDir(root)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create worker dispatch retry dispatch execution directory: %w", err)
	}
	execution.Path = filepath.Join(dir, safeRuntimeWorkerDispatchRetryDispatchExecutionName(*execution)+".json")
	data, err := json.MarshalIndent(execution, "", "  ")
	if err != nil {
		return fmt.Errorf("encode worker dispatch retry dispatch execution: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(execution.Path, data, 0o644); err != nil {
		return fmt.Errorf("write worker dispatch retry dispatch execution: %w", err)
	}
	return nil
}

func runtimeWorkerDispatchRetryDispatchExecutionResultFromCandidate(candidate RuntimeWorkerDispatchRetryCandidate) RuntimeWorkerDispatchRetryDispatchExecutionResult {
	return RuntimeWorkerDispatchRetryDispatchExecutionResult{
		SourceResultPath:     candidate.ResultPath,
		SourceDispatchPath:   candidate.DispatchPath,
		SourceClaimPath:      candidate.ClaimPath,
		TransportRequestPath: candidate.TransportRequestPath,
		WorkerID:             candidate.WorkerID,
		RunID:                candidate.RunID,
		ExecutionID:          candidate.ExecutionID,
		Tool:                 candidate.Tool,
		Capability:           candidate.Capability,
		ToolPlan:             normalizeRuntimeWorkerToolPlanPayload(candidate.ToolPlan, candidate.Tool),
		LastStatus:           candidate.LastStatus,
		LastOutput:           candidate.LastOutput,
		LastError:            candidate.LastError,
	}
}

func runtimeWorkerDispatchRetryCandidateMismatch(candidate RuntimeWorkerDispatchRetryCandidate, result RuntimeWorkerQueueDispatchResult) string {
	if strings.TrimSpace(result.WorkerID) != strings.TrimSpace(candidate.WorkerID) {
		return "result-worker-mismatch"
	}
	if strings.TrimSpace(result.RunID) != strings.TrimSpace(candidate.RunID) {
		return "result-run-mismatch"
	}
	if strings.TrimSpace(result.ExecutionID) != strings.TrimSpace(candidate.ExecutionID) {
		return "result-execution-mismatch"
	}
	if strings.TrimSpace(result.DispatchExecutionPath) != strings.TrimSpace(candidate.DispatchPath) {
		return "result-dispatch-mismatch"
	}
	if strings.TrimSpace(result.ClaimPath) != strings.TrimSpace(candidate.ClaimPath) {
		return "result-claim-mismatch"
	}
	return ""
}

func safeRuntimeWorkerDispatchRetryDispatchName(dispatch RuntimeWorkerDispatchRetryDispatch) string {
	parts := []string{
		"worker-dispatch-retry-dispatch",
		dispatch.CreatedAt,
		dispatch.RetryPlanPath,
		fmt.Sprintf("total=%d ready=%d skipped=%d", dispatch.Summary.Total, dispatch.Summary.Ready, dispatch.Summary.Skipped),
	}
	for _, candidate := range dispatch.Ready {
		parts = append(parts, candidate.ResultPath, candidate.DispatchPath, candidate.ClaimPath, candidate.WorkerID, candidate.RunID, candidate.ExecutionID, candidate.LastStatus, candidate.LastError)
	}
	for _, skipped := range dispatch.Skipped {
		parts = append(parts, skipped.ResultPath, skipped.DispatchPath, skipped.ClaimPath, skipped.WorkerID, skipped.RunID, skipped.ExecutionID, skipped.Status, skipped.Reason)
	}
	name := safeInstallRequestName(strings.Join(parts, "|"))
	if index := strings.LastIndex(name, "-"); index >= 0 && len(name[index+1:]) == 12 {
		return "worker-dispatch-retry-dispatch-" + name[index+1:]
	}
	return name
}

func safeRuntimeWorkerDispatchRetryDispatchExecutionName(execution RuntimeWorkerDispatchRetryDispatchExecution) string {
	parts := []string{
		"worker-dispatch-retry-dispatch-execution",
		execution.ExecutedAt,
		execution.RetryDispatchPath,
		fmt.Sprintf("claimed=%d dispatched=%d skipped=%d failed=%d", execution.Summary.Claimed, execution.Summary.Dispatched, execution.Summary.Skipped, execution.Summary.Failed),
	}
	for _, result := range execution.Results {
		parts = append(parts, result.SourceResultPath, result.SourceDispatchPath, result.SourceClaimPath, result.RetryClaimPath, result.RetryQueueDispatchPath, result.RetryTransportRequestPath, result.WorkerID, result.RunID, result.ExecutionID, result.Status, result.Reason, result.Error)
	}
	name := safeInstallRequestName(strings.Join(parts, "|"))
	if index := strings.LastIndex(name, "-"); index >= 0 && len(name[index+1:]) == 12 {
		return "worker-dispatch-retry-dispatch-execution-" + name[index+1:]
	}
	return name
}
