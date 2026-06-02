package app

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const DefaultWorkerLeaseStaleAfter = 15 * time.Minute

const defaultWorkerLeaseStaleAfter = DefaultWorkerLeaseStaleAfter

type WorkerLeaseArtifact struct {
	Version                  int    `json:"version"`
	Path                     string `json:"-"`
	RunID                    string `json:"run_id"`
	ExecutionID              string `json:"execution_id"`
	ResultExecutionID        string `json:"result_execution_id,omitempty"`
	Tool                     string `json:"tool"`
	Action                   string `json:"action"`
	Capability               string `json:"capability"`
	WorkerID                 string `json:"worker_id"`
	Status                   string `json:"status"`
	AcquiredAt               string `json:"acquired_at"`
	HeartbeatAt              string `json:"heartbeat_at,omitempty"`
	HeartbeatBy              string `json:"heartbeat_by,omitempty"`
	HeartbeatReason          string `json:"heartbeat_reason,omitempty"`
	ReleasedAt               string `json:"released_at,omitempty"`
	ReleasedBy               string `json:"released_by,omitempty"`
	ReleaseReason            string `json:"release_reason,omitempty"`
	AttemptDir               string `json:"attempt_dir,omitempty"`
	Error                    string `json:"error,omitempty"`
	Stale                    bool   `json:"-"`
	WorkerPoolMaxConcurrency int    `json:"worker_pool_max_concurrency,omitempty"`
	PackMaxConcurrency       int    `json:"pack_max_concurrency,omitempty"`
	ResourceCPUUnits         int    `json:"resource_cpu_units,omitempty"`
	ResourceMemoryMB         int    `json:"resource_memory_mb,omitempty"`
}

type WorkerLeaseReclaimPlan struct {
	Ready   []WorkerLeaseReclaimCandidate `json:"ready"`
	Skipped []WorkerLeaseReclaimSkip      `json:"skipped"`
}

type WorkerLeaseReclaimCandidate struct {
	Lease WorkerLeaseArtifact `json:"lease"`
	Mode  string              `json:"mode"`
}

type WorkerLeaseReclaimSkip struct {
	Lease  WorkerLeaseArtifact `json:"lease"`
	Reason string              `json:"reason"`
}

type WorkerLeaseReclaimDispatch struct {
	Version           int                           `json:"version"`
	Path              string                        `json:"-"`
	Status            string                        `json:"status"`
	Mode              string                        `json:"mode"`
	CreatedAt         string                        `json:"created_at"`
	StaleAfterSeconds int64                         `json:"stale_after_seconds"`
	Ready             []WorkerLeaseReclaimCandidate `json:"ready"`
	Skipped           []WorkerLeaseReclaimSkip      `json:"skipped"`
}

type WorkerLeaseHeartbeatOptions struct {
	Heartbeat   bool
	Now         time.Time
	HeartbeatBy string
	Reason      string
}

type WorkerLeaseReleaseOptions struct {
	Release           bool
	Status            string
	Now               time.Time
	ReleasedBy        string
	Reason            string
	Error             string
	ResultExecutionID string
	AttemptDir        string
}

type WorkerLeaseReclaimExecutionOptions struct {
	Execute    bool
	Now        time.Time
	StaleAfter time.Duration
}

type WorkerLeaseReclaimExecution struct {
	Version           int                                 `json:"version"`
	Path              string                              `json:"-"`
	Status            string                              `json:"status"`
	Mode              string                              `json:"mode"`
	DispatchPath      string                              `json:"dispatch_path"`
	ExecutedAt        string                              `json:"executed_at"`
	StaleAfterSeconds int64                               `json:"stale_after_seconds"`
	Results           []WorkerLeaseReclaimExecutionResult `json:"results"`
	Skipped           []WorkerLeaseReclaimSkip            `json:"skipped"`
}

type WorkerLeaseReclaimExecutionResult struct {
	LeasePath      string `json:"lease_path"`
	RunID          string `json:"run_id"`
	ExecutionID    string `json:"execution_id"`
	Tool           string `json:"tool"`
	Capability     string `json:"capability"`
	WorkerID       string `json:"worker_id"`
	PreviousStatus string `json:"previous_status"`
	Status         string `json:"status"`
	ReclaimedAt    string `json:"reclaimed_at"`
	Error          string `json:"error,omitempty"`
}

func acquireWorkerLease(root string, runID string, request ToolExecutionRequest, requireSafetyAuthorization bool) error {
	now := time.Now().UTC()
	if requireSafetyAuthorization {
		if err := requireRuntimeWorkerSideEffectSafetyAuthorization(root, now); err != nil {
			return err
		}
	}
	timestamp := now.Format(time.RFC3339Nano)
	lease := WorkerLeaseArtifact{
		Version:                  1,
		RunID:                    runID,
		ExecutionID:              workerLeaseExecutionID(request),
		Tool:                     request.Node.ManifestName,
		Action:                   string(request.Node.Action),
		Capability:               request.Node.Capability,
		WorkerID:                 request.WorkerID,
		Status:                   "active",
		AcquiredAt:               timestamp,
		HeartbeatAt:              timestamp,
		WorkerPoolMaxConcurrency: request.WorkerPoolMaxConcurrency,
		PackMaxConcurrency:       request.PackMaxConcurrency,
		ResourceCPUUnits:         request.ResourceCPUUnits,
		ResourceMemoryMB:         request.ResourceMemoryMB,
	}
	return createWorkerLeaseArtifact(root, lease)
}

func completeWorkerLease(root string, runID string, request ToolExecutionRequest, result ToolExecutionResult, runErr error, requireSafetyAuthorization bool) error {
	leasePath := workerLeasePath(root, runID, workerLeaseExecutionID(request))
	data, err := os.ReadFile(leasePath)
	if err != nil {
		return fmt.Errorf("read worker lease: %w", err)
	}
	var lease WorkerLeaseArtifact
	if err := json.Unmarshal(data, &lease); err != nil {
		return fmt.Errorf("decode worker lease: %w", err)
	}
	status := strings.TrimSpace(result.Status)
	if status == "" {
		status = "completed"
	}
	if runErr != nil {
		status = "failed"
		lease.Error = strings.TrimSpace(runErr.Error())
	}
	if lease.Error == "" && status == "failed" {
		lease.Error = strings.TrimSpace(result.Transcript)
	}
	lease.Status = status
	now := time.Now().UTC()
	if requireSafetyAuthorization {
		if err := requireRuntimeWorkerSideEffectSafetyAuthorization(root, now); err != nil {
			return err
		}
	}
	timestamp := now.Format(time.RFC3339Nano)
	lease.HeartbeatAt = timestamp
	lease.ReleasedAt = timestamp
	lease.ResultExecutionID = strings.TrimSpace(result.ExecutionID)
	lease.AttemptDir = strings.TrimSpace(result.AttemptDir)
	return writeWorkerLeaseArtifact(root, lease)
}

func ListWorkerLeases(root string) ([]WorkerLeaseArtifact, error) {
	dir := workerLeaseRoot(root)
	if _, err := os.Stat(dir); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("stat worker leases: %w", err)
	}
	var leases []WorkerLeaseArtifact
	err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || filepath.Ext(path) != ".json" {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		var lease WorkerLeaseArtifact
		if err := json.Unmarshal(data, &lease); err != nil {
			return fmt.Errorf("decode worker lease %s: %w", path, err)
		}
		lease.Path = path
		leases = append(leases, lease)
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.SliceStable(leases, func(i int, j int) bool {
		if leases[i].RunID != leases[j].RunID {
			return leases[i].RunID < leases[j].RunID
		}
		if leases[i].ExecutionID != leases[j].ExecutionID {
			return leases[i].ExecutionID < leases[j].ExecutionID
		}
		return leases[i].Path < leases[j].Path
	})
	return leases, nil
}

func ListWorkerLeasesWithStaleness(root string, now time.Time, staleAfter time.Duration) ([]WorkerLeaseArtifact, error) {
	leases, err := ListWorkerLeases(root)
	if err != nil {
		return nil, err
	}
	for i := range leases {
		leases[i].Stale = workerLeaseIsStale(leases[i], now.UTC(), staleAfter)
	}
	return leases, nil
}

func PlanWorkerLeaseReclaims(root string, now time.Time, staleAfter time.Duration) (WorkerLeaseReclaimPlan, error) {
	leases, err := ListWorkerLeasesWithStaleness(root, now, staleAfter)
	if err != nil {
		return WorkerLeaseReclaimPlan{}, err
	}
	return workerLeaseReclaimPlanFromLeases(leases), nil
}

func UpdateWorkerLeaseHeartbeat(root string, runID string, executionID string, options WorkerLeaseHeartbeatOptions) (WorkerLeaseArtifact, error) {
	if !options.Heartbeat {
		return WorkerLeaseArtifact{}, fmt.Errorf("worker lease heartbeat requires explicit heartbeat")
	}
	now := options.Now
	if now.IsZero() {
		now = time.Now().UTC()
	} else {
		now = now.UTC()
	}
	lease, err := readWorkerLeaseByIdentity(root, runID, executionID)
	if err != nil {
		return WorkerLeaseArtifact{}, err
	}
	if strings.TrimSpace(lease.Status) != "active" {
		return WorkerLeaseArtifact{}, fmt.Errorf("worker lease heartbeat requires active lease")
	}
	if err := requireRuntimeWorkerSideEffectSafetyAuthorization(root, now); err != nil {
		return WorkerLeaseArtifact{}, err
	}
	lease.HeartbeatAt = now.Format(time.RFC3339Nano)
	if heartbeatBy := strings.TrimSpace(options.HeartbeatBy); heartbeatBy != "" {
		lease.HeartbeatBy = heartbeatBy
	}
	if reason := strings.TrimSpace(options.Reason); reason != "" {
		lease.HeartbeatReason = reason
	}
	if err := writeWorkerLeaseArtifact(root, lease); err != nil {
		return WorkerLeaseArtifact{}, err
	}
	return readWorkerLeaseByIdentity(root, runID, executionID)
}

func ReleaseWorkerLease(root string, runID string, executionID string, options WorkerLeaseReleaseOptions) (WorkerLeaseArtifact, error) {
	if !options.Release {
		return WorkerLeaseArtifact{}, fmt.Errorf("worker lease release requires explicit release")
	}
	status := strings.TrimSpace(options.Status)
	if status != "completed" && status != "failed" {
		return WorkerLeaseArtifact{}, fmt.Errorf("worker lease release status must be completed or failed")
	}
	now := options.Now
	if now.IsZero() {
		now = time.Now().UTC()
	} else {
		now = now.UTC()
	}
	lease, err := readWorkerLeaseByIdentity(root, runID, executionID)
	if err != nil {
		return WorkerLeaseArtifact{}, err
	}
	if strings.TrimSpace(lease.Status) != "active" {
		return WorkerLeaseArtifact{}, fmt.Errorf("worker lease release requires active lease")
	}
	if err := requireRuntimeWorkerSideEffectSafetyAuthorization(root, now); err != nil {
		return WorkerLeaseArtifact{}, err
	}
	nowText := now.Format(time.RFC3339Nano)
	lease.Status = status
	lease.HeartbeatAt = nowText
	lease.ReleasedAt = nowText
	if releasedBy := strings.TrimSpace(options.ReleasedBy); releasedBy != "" {
		lease.ReleasedBy = releasedBy
	}
	if reason := strings.TrimSpace(options.Reason); reason != "" {
		lease.ReleaseReason = reason
	}
	if resultExecutionID := strings.TrimSpace(options.ResultExecutionID); resultExecutionID != "" {
		lease.ResultExecutionID = resultExecutionID
	}
	if attemptDir := strings.TrimSpace(options.AttemptDir); attemptDir != "" {
		lease.AttemptDir = attemptDir
	}
	if status == "failed" {
		lease.Error = strings.TrimSpace(options.Error)
	}
	if err := writeWorkerLeaseArtifact(root, lease); err != nil {
		return WorkerLeaseArtifact{}, err
	}
	return readWorkerLeaseByIdentity(root, runID, executionID)
}

func CreateWorkerLeaseReclaimDispatch(root string, now time.Time, staleAfter time.Duration) (WorkerLeaseReclaimDispatch, error) {
	plan, err := PlanWorkerLeaseReclaims(root, now, staleAfter)
	if err != nil {
		return WorkerLeaseReclaimDispatch{}, err
	}
	dispatch := WorkerLeaseReclaimDispatch{
		Version:           1,
		Status:            "dry-run",
		Mode:              "metadata-only",
		CreatedAt:         now.UTC().Format(time.RFC3339Nano),
		StaleAfterSeconds: int64(staleAfter / time.Second),
		Ready:             append([]WorkerLeaseReclaimCandidate(nil), plan.Ready...),
		Skipped:           append([]WorkerLeaseReclaimSkip(nil), plan.Skipped...),
	}
	dir := workerLeaseReclaimDispatchDir(root)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return WorkerLeaseReclaimDispatch{}, fmt.Errorf("create worker lease reclaim dispatch directory: %w", err)
	}
	dispatch.Path = filepath.Join(dir, safeWorkerLeaseReclaimDispatchName(plan)+".json")
	data, err := json.MarshalIndent(dispatch, "", "  ")
	if err != nil {
		return WorkerLeaseReclaimDispatch{}, fmt.Errorf("encode worker lease reclaim dispatch: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(dispatch.Path, data, 0o644); err != nil {
		return WorkerLeaseReclaimDispatch{}, fmt.Errorf("write worker lease reclaim dispatch: %w", err)
	}
	return dispatch, nil
}

func ListWorkerLeaseReclaimDispatches(root string) ([]WorkerLeaseReclaimDispatch, error) {
	paths, err := filepath.Glob(filepath.Join(workerLeaseReclaimDispatchDir(root), "*.json"))
	if err != nil {
		return nil, fmt.Errorf("glob worker lease reclaim dispatches: %w", err)
	}
	sort.Strings(paths)
	dispatches := make([]WorkerLeaseReclaimDispatch, 0, len(paths))
	for _, path := range paths {
		dispatch, err := readWorkerLeaseReclaimDispatchFile(path)
		if err != nil {
			return nil, err
		}
		dispatches = append(dispatches, dispatch)
	}
	return dispatches, nil
}

func ReadWorkerLeaseReclaimDispatch(root, dispatchPath string) (WorkerLeaseReclaimDispatch, error) {
	path, err := resolveWorkerLeaseReclaimDispatchPath(root, dispatchPath)
	if err != nil {
		return WorkerLeaseReclaimDispatch{}, err
	}
	return readWorkerLeaseReclaimDispatchFile(path)
}

func readWorkerLeaseReclaimDispatchFile(path string) (WorkerLeaseReclaimDispatch, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return WorkerLeaseReclaimDispatch{}, fmt.Errorf("read worker lease reclaim dispatch %s: %w", path, err)
	}
	var dispatch WorkerLeaseReclaimDispatch
	if err := json.Unmarshal(data, &dispatch); err != nil {
		return WorkerLeaseReclaimDispatch{}, fmt.Errorf("decode worker lease reclaim dispatch %s: %w", path, err)
	}
	dispatch.Path = path
	return dispatch, nil
}

func ExecuteWorkerLeaseReclaimDispatch(root, dispatchPath string, options WorkerLeaseReclaimExecutionOptions) (WorkerLeaseReclaimExecution, error) {
	if !options.Execute {
		return WorkerLeaseReclaimExecution{}, fmt.Errorf("worker lease reclaim execution requires explicit execute")
	}
	dispatch, err := ReadWorkerLeaseReclaimDispatch(root, dispatchPath)
	if err != nil {
		return WorkerLeaseReclaimExecution{}, err
	}
	if strings.TrimSpace(dispatch.Status) != "dry-run" || strings.TrimSpace(dispatch.Mode) != "metadata-only" {
		return WorkerLeaseReclaimExecution{}, fmt.Errorf("worker lease reclaim dispatch must be dry-run metadata-only before execution")
	}
	now := options.Now.UTC()
	if now.IsZero() {
		now = time.Now().UTC()
	}
	staleAfter := options.StaleAfter
	if staleAfter <= 0 && dispatch.StaleAfterSeconds > 0 {
		staleAfter = time.Duration(dispatch.StaleAfterSeconds) * time.Second
	}
	if staleAfter <= 0 {
		staleAfter = DefaultWorkerLeaseStaleAfter
	}
	if err := requireRuntimeWorkerSideEffectSafetyAuthorization(root, now); err != nil {
		return WorkerLeaseReclaimExecution{}, err
	}
	execution := WorkerLeaseReclaimExecution{
		Version:           1,
		Status:            "completed",
		Mode:              "execute",
		DispatchPath:      dispatch.Path,
		ExecutedAt:        now.Format(time.RFC3339Nano),
		StaleAfterSeconds: int64(staleAfter / time.Second),
		Skipped:           append([]WorkerLeaseReclaimSkip(nil), dispatch.Skipped...),
	}
	for _, candidate := range dispatch.Ready {
		lease, err := readWorkerLeaseByIdentity(root, candidate.Lease.RunID, candidate.Lease.ExecutionID)
		if err != nil {
			execution.Skipped = append(execution.Skipped, WorkerLeaseReclaimSkip{
				Lease:  candidate.Lease,
				Reason: "missing",
			})
			continue
		}
		lease.Stale = workerLeaseIsStale(lease, now, staleAfter)
		plan := workerLeaseReclaimPlanFromLeases([]WorkerLeaseArtifact{lease})
		if len(plan.Ready) == 0 {
			reason := "not-ready"
			if len(plan.Skipped) > 0 {
				reason = plan.Skipped[0].Reason
			}
			execution.Skipped = append(execution.Skipped, WorkerLeaseReclaimSkip{
				Lease:  lease,
				Reason: reason,
			})
			continue
		}
		previousStatus := strings.TrimSpace(lease.Status)
		lease.Status = "reclaimed"
		lease.HeartbeatAt = now.Format(time.RFC3339Nano)
		lease.ReleasedAt = now.Format(time.RFC3339Nano)
		if err := writeWorkerLeaseArtifact(root, lease); err != nil {
			execution.Status = "failed"
			result := workerLeaseReclaimExecutionResult(lease, previousStatus, "failed", now)
			result.Error = err.Error()
			execution.Results = append(execution.Results, result)
			if writeErr := writeWorkerLeaseReclaimExecution(root, &execution); writeErr != nil {
				return execution, writeErr
			}
			return execution, fmt.Errorf("reclaim worker lease %s/%s: %w", lease.RunID, lease.ExecutionID, err)
		}
		execution.Results = append(execution.Results, workerLeaseReclaimExecutionResult(lease, previousStatus, "reclaimed", now))
	}
	if err := writeWorkerLeaseReclaimExecution(root, &execution); err != nil {
		return execution, err
	}
	return execution, nil
}

func ListWorkerLeaseReclaimExecutions(root string) ([]WorkerLeaseReclaimExecution, error) {
	paths, err := filepath.Glob(filepath.Join(workerLeaseReclaimExecutionDir(root), "*.json"))
	if err != nil {
		return nil, fmt.Errorf("glob worker lease reclaim executions: %w", err)
	}
	sort.Strings(paths)
	executions := make([]WorkerLeaseReclaimExecution, 0, len(paths))
	for _, path := range paths {
		execution, err := readWorkerLeaseReclaimExecutionFile(path)
		if err != nil {
			return nil, err
		}
		executions = append(executions, execution)
	}
	return executions, nil
}

func ReadWorkerLeaseReclaimExecution(root, executionPath string) (WorkerLeaseReclaimExecution, error) {
	path, err := resolveWorkerLeaseReclaimExecutionPath(root, executionPath)
	if err != nil {
		return WorkerLeaseReclaimExecution{}, err
	}
	return readWorkerLeaseReclaimExecutionFile(path)
}

func readWorkerLeaseReclaimExecutionFile(path string) (WorkerLeaseReclaimExecution, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return WorkerLeaseReclaimExecution{}, fmt.Errorf("read worker lease reclaim execution %s: %w", path, err)
	}
	var execution WorkerLeaseReclaimExecution
	if err := json.Unmarshal(data, &execution); err != nil {
		return WorkerLeaseReclaimExecution{}, fmt.Errorf("decode worker lease reclaim execution %s: %w", path, err)
	}
	execution.Path = path
	return execution, nil
}

func writeWorkerLeaseReclaimExecution(root string, execution *WorkerLeaseReclaimExecution) error {
	dir := workerLeaseReclaimExecutionDir(root)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create worker lease reclaim execution directory: %w", err)
	}
	if strings.TrimSpace(execution.Path) == "" {
		execution.Path = filepath.Join(dir, safeWorkerLeaseReclaimExecutionName(*execution)+".json")
	}
	data, err := json.MarshalIndent(execution, "", "  ")
	if err != nil {
		return fmt.Errorf("encode worker lease reclaim execution: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(execution.Path, data, 0o644); err != nil {
		return fmt.Errorf("write worker lease reclaim execution: %w", err)
	}
	return nil
}

func workerLeaseReclaimExecutionResult(lease WorkerLeaseArtifact, previousStatus string, status string, now time.Time) WorkerLeaseReclaimExecutionResult {
	return WorkerLeaseReclaimExecutionResult{
		LeasePath:      lease.Path,
		RunID:          lease.RunID,
		ExecutionID:    lease.ExecutionID,
		Tool:           lease.Tool,
		Capability:     lease.Capability,
		WorkerID:       lease.WorkerID,
		PreviousStatus: previousStatus,
		Status:         status,
		ReclaimedAt:    now.UTC().Format(time.RFC3339Nano),
	}
}

func workerLeaseReclaimPlanFromLeases(leases []WorkerLeaseArtifact) WorkerLeaseReclaimPlan {
	var plan WorkerLeaseReclaimPlan
	for _, lease := range leases {
		status := strings.TrimSpace(lease.Status)
		if status != "active" {
			plan.Skipped = append(plan.Skipped, WorkerLeaseReclaimSkip{
				Lease:  lease,
				Reason: "not-active",
			})
			continue
		}
		if !lease.Stale {
			plan.Skipped = append(plan.Skipped, WorkerLeaseReclaimSkip{
				Lease:  lease,
				Reason: "active-not-stale",
			})
			continue
		}
		plan.Ready = append(plan.Ready, WorkerLeaseReclaimCandidate{
			Lease: lease,
			Mode:  "metadata-only",
		})
	}
	return plan
}

func workerLeaseIsStale(lease WorkerLeaseArtifact, now time.Time, staleAfter time.Duration) bool {
	if staleAfter <= 0 || strings.TrimSpace(lease.Status) != "active" {
		return false
	}
	timestamp := strings.TrimSpace(lease.HeartbeatAt)
	if timestamp == "" {
		timestamp = strings.TrimSpace(lease.AcquiredAt)
	}
	if timestamp == "" {
		return true
	}
	heartbeat, err := time.Parse(time.RFC3339Nano, timestamp)
	if err != nil {
		return true
	}
	return now.Sub(heartbeat) > staleAfter
}

func writeWorkerLeaseArtifact(root string, lease WorkerLeaseArtifact) error {
	path := workerLeasePath(root, lease.RunID, lease.ExecutionID)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create worker lease directory: %w", err)
	}
	data, err := json.MarshalIndent(lease, "", "  ")
	if err != nil {
		return fmt.Errorf("encode worker lease: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("write worker lease: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("publish worker lease: %w", err)
	}
	return nil
}

func readWorkerLeaseByIdentity(root string, runID string, executionID string) (WorkerLeaseArtifact, error) {
	path := workerLeasePath(root, runID, executionID)
	data, err := os.ReadFile(path)
	if err != nil {
		return WorkerLeaseArtifact{}, fmt.Errorf("read worker lease %s: %w", path, err)
	}
	var lease WorkerLeaseArtifact
	if err := json.Unmarshal(data, &lease); err != nil {
		return WorkerLeaseArtifact{}, fmt.Errorf("decode worker lease %s: %w", path, err)
	}
	lease.Path = path
	return lease, nil
}

func createWorkerLeaseArtifact(root string, lease WorkerLeaseArtifact) error {
	path := workerLeasePath(root, lease.RunID, lease.ExecutionID)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create worker lease directory: %w", err)
	}
	data, err := json.MarshalIndent(lease, "", "  ")
	if err != nil {
		return fmt.Errorf("encode worker lease: %w", err)
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		if os.IsExist(err) {
			return workerLeaseCollisionError(path)
		}
		return fmt.Errorf("create worker lease: %w", err)
	}
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return fmt.Errorf("write worker lease: %w", err)
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(path)
		return fmt.Errorf("close worker lease: %w", err)
	}
	return nil
}

func workerLeaseCollisionError(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("worker lease already exists: %s", path)
	}
	var existing WorkerLeaseArtifact
	if err := json.Unmarshal(data, &existing); err != nil {
		return fmt.Errorf("worker lease already exists and cannot be decoded: %s: %w", path, err)
	}
	status := strings.TrimSpace(existing.Status)
	if status == "" {
		status = "unknown"
	}
	if status == "active" {
		return fmt.Errorf("active worker lease already exists: %s", path)
	}
	return fmt.Errorf("worker lease already exists with status %s: %s", status, path)
}

func workerLeaseRoot(root string) string {
	return filepath.Join(root, "runtime", "worker-leases")
}

func workerLeaseReclaimDispatchDir(root string) string {
	return filepath.Join(root, "runtime", "worker-lease-reclaims")
}

func workerLeaseReclaimExecutionDir(root string) string {
	return filepath.Join(root, "runtime", "worker-lease-reclaim-executions")
}

func workerLeasePath(root string, runID string, executionID string) string {
	return filepath.Join(workerLeaseRoot(root), safeExecutionID(runID), safeExecutionID(executionID)+".json")
}

func workerLeaseExecutionID(request ToolExecutionRequest) string {
	return "tool-" + strings.ReplaceAll(request.Node.ID, ":", "-")
}

func resolveWorkerLeaseReclaimDispatchPath(root, dispatchPath string) (string, error) {
	trimmed := strings.TrimSpace(dispatchPath)
	if trimmed == "" {
		return "", fmt.Errorf("worker lease reclaim dispatch path is required")
	}
	dir, err := filepath.Abs(workerLeaseReclaimDispatchDir(root))
	if err != nil {
		return "", fmt.Errorf("resolve worker lease reclaim dispatch directory: %w", err)
	}
	candidate := trimmed
	if !filepath.IsAbs(candidate) {
		candidate = filepath.Join(dir, candidate)
	}
	candidate, err = filepath.Abs(candidate)
	if err != nil {
		return "", fmt.Errorf("resolve worker lease reclaim dispatch path: %w", err)
	}
	rel, err := filepath.Rel(dir, candidate)
	if err != nil {
		return "", fmt.Errorf("resolve worker lease reclaim dispatch relative path: %w", err)
	}
	if rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("worker lease reclaim dispatch path must stay under %s", dir)
	}
	if filepath.Ext(candidate) != ".json" {
		return "", fmt.Errorf("worker lease reclaim dispatch path must be a json file")
	}
	return candidate, nil
}

func resolveWorkerLeaseReclaimExecutionPath(root, executionPath string) (string, error) {
	trimmed := strings.TrimSpace(executionPath)
	if trimmed == "" {
		return "", fmt.Errorf("worker lease reclaim execution path is required")
	}
	dir, err := filepath.Abs(workerLeaseReclaimExecutionDir(root))
	if err != nil {
		return "", fmt.Errorf("resolve worker lease reclaim execution directory: %w", err)
	}
	candidate := trimmed
	if !filepath.IsAbs(candidate) {
		candidate = filepath.Join(dir, candidate)
	}
	candidate, err = filepath.Abs(candidate)
	if err != nil {
		return "", fmt.Errorf("resolve worker lease reclaim execution path: %w", err)
	}
	rel, err := filepath.Rel(dir, candidate)
	if err != nil {
		return "", fmt.Errorf("resolve worker lease reclaim execution relative path: %w", err)
	}
	if rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("worker lease reclaim execution path must stay under %s", dir)
	}
	if filepath.Ext(candidate) != ".json" {
		return "", fmt.Errorf("worker lease reclaim execution path must be a json file")
	}
	return candidate, nil
}

func safeWorkerLeaseReclaimDispatchName(plan WorkerLeaseReclaimPlan) string {
	parts := []string{
		"worker-lease-reclaim",
		fmt.Sprintf("ready=%d skipped=%d", len(plan.Ready), len(plan.Skipped)),
	}
	for _, candidate := range plan.Ready {
		lease := candidate.Lease
		parts = append(parts, lease.Path, lease.RunID, lease.ExecutionID, lease.Tool, lease.Capability, lease.WorkerID, candidate.Mode)
	}
	for _, skipped := range plan.Skipped {
		lease := skipped.Lease
		parts = append(parts, lease.Path, lease.RunID, lease.ExecutionID, lease.Tool, lease.Capability, lease.WorkerID, lease.Status, skipped.Reason)
	}
	return safeInstallRequestName(strings.Join(parts, "|"))
}

func safeWorkerLeaseReclaimExecutionName(execution WorkerLeaseReclaimExecution) string {
	parts := []string{
		"worker-lease-reclaim-execution",
		execution.DispatchPath,
		execution.ExecutedAt,
		fmt.Sprintf("results=%d skipped=%d", len(execution.Results), len(execution.Skipped)),
	}
	for _, result := range execution.Results {
		parts = append(parts, result.LeasePath, result.RunID, result.ExecutionID, result.WorkerID, result.Status)
	}
	for _, skipped := range execution.Skipped {
		lease := skipped.Lease
		parts = append(parts, lease.Path, lease.RunID, lease.ExecutionID, lease.WorkerID, skipped.Reason)
	}
	return safeInstallRequestName(strings.Join(parts, "|"))
}
