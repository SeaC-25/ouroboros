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

type RuntimeWorkerDispatchRetrySchedulerRunOptions struct {
	Execute             bool
	MaxRuns             int
	Now                 time.Time
	Actor               string
	Reason              string
	LeaseDuration       time.Duration
	DispatchRetryClaims bool
	RetryTransportMode  string
}

type RuntimeWorkerDispatchRetrySchedulerRun struct {
	Version                        int                               `json:"version"`
	Path                           string                            `json:"path,omitempty"`
	Status                         string                            `json:"status"`
	Mode                           string                            `json:"mode"`
	CreatedAt                      string                            `json:"created_at"`
	CompletedAt                    string                            `json:"completed_at,omitempty"`
	Actor                          string                            `json:"actor"`
	Reason                         string                            `json:"reason,omitempty"`
	MaxRunsRequested               int                               `json:"max_runs_requested"`
	MaxRuns                        int                               `json:"max_runs,omitempty"`
	DispatchRetryClaims            bool                              `json:"dispatch_retry_claims"`
	RetryTransportMode             string                            `json:"retry_transport_mode"`
	CurrentSummary                 RuntimeWorkerDispatchRetrySummary `json:"current_summary"`
	RetryPlanPaths                 []string                          `json:"retry_plan_paths,omitempty"`
	RetryDispatchPaths             []string                          `json:"retry_dispatch_paths,omitempty"`
	RetryDispatchExecutionPaths    []string                          `json:"retry_dispatch_execution_paths,omitempty"`
	RunsCreated                    int                               `json:"runs_created"`
	RetryPlansCreated              int                               `json:"retry_plans_created"`
	RetryDispatchesCreated         int                               `json:"retry_dispatches_created"`
	RetryDispatchExecutionsCreated int                               `json:"retry_dispatch_executions_created"`
	RetryClaimsCreated             int                               `json:"retry_claims_created"`
	RetryClaimsDispatched          int                               `json:"retry_claims_dispatched"`
	RetryTransportRequestsCreated  int                               `json:"retry_transport_requests_created"`
	SkipReason                     string                            `json:"skip_reason,omitempty"`
	Error                          string                            `json:"error,omitempty"`
}

func CreateRuntimeWorkerDispatchRetrySchedulerRun(root string, options RuntimeWorkerDispatchRetrySchedulerRunOptions) (RuntimeWorkerDispatchRetrySchedulerRun, error) {
	if options.Execute && options.MaxRuns <= 0 {
		return RuntimeWorkerDispatchRetrySchedulerRun{}, fmt.Errorf("worker dispatch retry scheduler run max runs must be positive")
	}
	if !options.Execute && options.MaxRuns < 0 {
		return RuntimeWorkerDispatchRetrySchedulerRun{}, fmt.Errorf("worker dispatch retry scheduler run max runs cannot be negative")
	}
	if options.DispatchRetryClaims && !options.Execute {
		return RuntimeWorkerDispatchRetrySchedulerRun{}, fmt.Errorf("worker dispatch retry scheduler run retry claim handoff requires execute")
	}
	transportMode := "metadata-only"
	if strings.TrimSpace(options.RetryTransportMode) != "" {
		transportMode = strings.TrimSpace(options.RetryTransportMode)
	}
	if options.DispatchRetryClaims {
		normalized, err := normalizeRuntimeWorkerQueueDispatchTransport(transportMode)
		if err != nil {
			return RuntimeWorkerDispatchRetrySchedulerRun{}, err
		}
		if normalized == "http-json" {
			return RuntimeWorkerDispatchRetrySchedulerRun{}, fmt.Errorf("worker dispatch retry claim handoff supports metadata-only or file-outbox transport")
		}
		transportMode = normalized
	}
	now := options.Now.UTC()
	if now.IsZero() {
		now = time.Now().UTC()
	}
	actor := strings.TrimSpace(options.Actor)
	if actor == "" {
		actor = "operator"
	}
	current, err := runtimeWorkerDispatchRetryPendingPlan(root)
	if err != nil {
		return RuntimeWorkerDispatchRetrySchedulerRun{}, err
	}
	run := RuntimeWorkerDispatchRetrySchedulerRun{
		Version:             1,
		Status:              "dry-run",
		Mode:                "metadata-only",
		CreatedAt:           now.Format(time.RFC3339Nano),
		Actor:               actor,
		Reason:              strings.TrimSpace(options.Reason),
		MaxRunsRequested:    options.MaxRuns,
		MaxRuns:             options.MaxRuns,
		DispatchRetryClaims: options.DispatchRetryClaims,
		RetryTransportMode:  transportMode,
		CurrentSummary:      current.Summary,
	}
	if !options.Execute {
		if err := writeRuntimeWorkerDispatchRetrySchedulerRun(root, &run); err != nil {
			return run, err
		}
		return run, nil
	}
	run.Status = "completed"
	run.Mode = "execute"
	if current.Summary.Ready == 0 {
		run.SkipReason = "no-worker-dispatch-retry-candidates"
		run.CompletedAt = now.Format(time.RFC3339Nano)
		if err := writeRuntimeWorkerDispatchRetrySchedulerRun(root, &run); err != nil {
			return run, err
		}
		return run, nil
	}
	seen := runtimeWorkerDispatchRetryExecutedSourceResultSetFromPlan(current)
	for i := 0; i < options.MaxRuns; i++ {
		childNow := now.Add(time.Duration(i) * time.Second)
		plan, err := runtimeWorkerDispatchRetryPendingPlan(root)
		if err != nil {
			run.Status = "failed"
			run.Error = err.Error()
			_ = writeRuntimeWorkerDispatchRetrySchedulerRun(root, &run)
			return run, err
		}
		plan = runtimeWorkerDispatchRetryFilterSeenPlan(plan, seen)
		if plan.Summary.Ready == 0 {
			if run.RunsCreated == 0 {
				run.SkipReason = "no-worker-dispatch-retry-candidates"
			}
			break
		}
		plan.CreatedAt = childNow.Format(time.RFC3339Nano)
		if err := writeRuntimeWorkerDispatchRetryPlanArtifact(root, &plan); err != nil {
			run.Status = "failed"
			run.Error = err.Error()
			_ = writeRuntimeWorkerDispatchRetrySchedulerRun(root, &run)
			return run, err
		}
		run.RetryPlanPaths = append(run.RetryPlanPaths, plan.Path)
		run.RetryPlansCreated++
		dispatch, err := CreateRuntimeWorkerDispatchRetryDispatch(root, filepath.Base(plan.Path), childNow.Add(time.Nanosecond))
		if err != nil {
			run.Status = "failed"
			run.Error = err.Error()
			_ = writeRuntimeWorkerDispatchRetrySchedulerRun(root, &run)
			return run, err
		}
		run.RetryDispatchPaths = append(run.RetryDispatchPaths, dispatch.Path)
		run.RetryDispatchesCreated++
		execution, err := ExecuteRuntimeWorkerDispatchRetryDispatch(root, filepath.Base(dispatch.Path), RuntimeWorkerDispatchRetryDispatchExecutionOptions{
			Execute:        true,
			Now:            childNow.Add(2 * time.Nanosecond),
			LeaseDuration:  options.LeaseDuration,
			DispatchClaims: options.DispatchRetryClaims,
			TransportMode:  transportMode,
		})
		if strings.TrimSpace(execution.Path) != "" {
			run.RetryDispatchExecutionPaths = append(run.RetryDispatchExecutionPaths, execution.Path)
			run.RetryDispatchExecutionsCreated++
			run.RetryClaimsCreated += execution.Summary.Claimed
			run.RetryClaimsDispatched += execution.Summary.Dispatched
			run.RetryTransportRequestsCreated += runtimeWorkerDispatchRetryTransportRequestsCreated(execution.Results)
			for _, result := range execution.Results {
				if strings.TrimSpace(result.SourceResultPath) != "" {
					seen[strings.TrimSpace(result.SourceResultPath)] = struct{}{}
				}
			}
		}
		run.RunsCreated++
		if err != nil {
			run.Status = "failed"
			run.Error = err.Error()
			_ = writeRuntimeWorkerDispatchRetrySchedulerRun(root, &run)
			return run, err
		}
	}
	run.CompletedAt = now.Add(time.Duration(run.RunsCreated) * time.Second).Format(time.RFC3339Nano)
	if err := writeRuntimeWorkerDispatchRetrySchedulerRun(root, &run); err != nil {
		return run, err
	}
	return run, nil
}

func ListRuntimeWorkerDispatchRetrySchedulerRuns(root string) ([]RuntimeWorkerDispatchRetrySchedulerRun, error) {
	paths, err := filepath.Glob(filepath.Join(runtimeWorkerDispatchRetrySchedulerRunDir(root), "*.json"))
	if err != nil {
		return nil, fmt.Errorf("glob worker dispatch retry scheduler run artifacts: %w", err)
	}
	sort.Strings(paths)
	runs := make([]RuntimeWorkerDispatchRetrySchedulerRun, 0, len(paths))
	for _, path := range paths {
		run, err := readRuntimeWorkerDispatchRetrySchedulerRunFile(path)
		if err != nil {
			return nil, err
		}
		runs = append(runs, run)
	}
	return runs, nil
}

func ReadRuntimeWorkerDispatchRetrySchedulerRun(root, runPath string) (RuntimeWorkerDispatchRetrySchedulerRun, error) {
	path, err := resolveRuntimeWorkerDispatchRetrySchedulerRunPath(root, runPath)
	if err != nil {
		return RuntimeWorkerDispatchRetrySchedulerRun{}, err
	}
	return readRuntimeWorkerDispatchRetrySchedulerRunFile(path)
}

func runtimeWorkerDispatchRetryPendingPlan(root string) (RuntimeWorkerDispatchRetryPlan, error) {
	plan, err := PlanRuntimeWorkerDispatchRetries(root)
	if err != nil {
		return RuntimeWorkerDispatchRetryPlan{}, err
	}
	return runtimeWorkerDispatchRetryFilterSeenPlan(plan, runtimeWorkerDispatchRetryExecutedSourceResults(root)), nil
}

func runtimeWorkerDispatchRetryExecutedSourceResults(root string) map[string]struct{} {
	executions, err := ListRuntimeWorkerDispatchRetryDispatchExecutions(root)
	if err != nil {
		return map[string]struct{}{}
	}
	seen := make(map[string]struct{})
	for _, execution := range executions {
		for _, result := range execution.Results {
			if source := strings.TrimSpace(result.SourceResultPath); source != "" {
				seen[source] = struct{}{}
			}
		}
	}
	return seen
}

func runtimeWorkerDispatchRetryExecutedSourceResultSetFromPlan(plan RuntimeWorkerDispatchRetryPlan) map[string]struct{} {
	seen := make(map[string]struct{})
	for _, skipped := range plan.Skipped {
		if skipped.Reason == "retry-already-executed" && strings.TrimSpace(skipped.ResultPath) != "" {
			seen[strings.TrimSpace(skipped.ResultPath)] = struct{}{}
		}
	}
	return seen
}

func runtimeWorkerDispatchRetryFilterSeenPlan(plan RuntimeWorkerDispatchRetryPlan, seen map[string]struct{}) RuntimeWorkerDispatchRetryPlan {
	if len(seen) == 0 {
		return plan
	}
	ready := make([]RuntimeWorkerDispatchRetryCandidate, 0, len(plan.Ready))
	skipped := append([]RuntimeWorkerDispatchRetrySkip(nil), plan.Skipped...)
	for _, candidate := range plan.Ready {
		source := strings.TrimSpace(candidate.ResultPath)
		if _, ok := seen[source]; ok && source != "" {
			skipped = append(skipped, RuntimeWorkerDispatchRetrySkip{
				ResultPath:   candidate.ResultPath,
				DispatchPath: candidate.DispatchPath,
				ClaimPath:    candidate.ClaimPath,
				WorkerID:     candidate.WorkerID,
				RunID:        candidate.RunID,
				ExecutionID:  candidate.ExecutionID,
				Tool:         candidate.Tool,
				Capability:   candidate.Capability,
				Status:       candidate.LastStatus,
				Reason:       "retry-already-executed",
			})
			continue
		}
		ready = append(ready, candidate)
	}
	plan.Ready = ready
	plan.Skipped = skipped
	plan.Summary.Ready = len(ready)
	plan.Summary.Skipped = len(skipped)
	plan.Summary.Total = len(ready) + len(skipped)
	return plan
}

func writeRuntimeWorkerDispatchRetryPlanArtifact(root string, plan *RuntimeWorkerDispatchRetryPlan) error {
	dir := runtimeWorkerDispatchRetryPlanDir(root)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create worker dispatch retry plan directory: %w", err)
	}
	if strings.TrimSpace(plan.Path) == "" {
		plan.Path = filepath.Join(dir, safeRuntimeWorkerDispatchRetryPlanName(*plan)+".json")
	}
	data, err := json.MarshalIndent(plan, "", "  ")
	if err != nil {
		return fmt.Errorf("encode worker dispatch retry plan: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(plan.Path, data, 0o644); err != nil {
		return fmt.Errorf("write worker dispatch retry plan: %w", err)
	}
	return nil
}

func writeRuntimeWorkerDispatchRetrySchedulerRun(root string, run *RuntimeWorkerDispatchRetrySchedulerRun) error {
	dir := runtimeWorkerDispatchRetrySchedulerRunDir(root)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create worker dispatch retry scheduler run directory: %w", err)
	}
	if strings.TrimSpace(run.Path) == "" {
		run.Path = filepath.Join(dir, safeRuntimeWorkerDispatchRetrySchedulerRunName(*run)+".json")
	}
	data, err := json.MarshalIndent(run, "", "  ")
	if err != nil {
		return fmt.Errorf("encode worker dispatch retry scheduler run: %w", err)
	}
	data = append(data, '\n')
	tmp := run.Path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("write worker dispatch retry scheduler run: %w", err)
	}
	if err := os.Rename(tmp, run.Path); err != nil {
		return fmt.Errorf("publish worker dispatch retry scheduler run: %w", err)
	}
	return nil
}

func readRuntimeWorkerDispatchRetrySchedulerRunFile(path string) (RuntimeWorkerDispatchRetrySchedulerRun, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return RuntimeWorkerDispatchRetrySchedulerRun{}, fmt.Errorf("read worker dispatch retry scheduler run %s: %w", path, err)
	}
	var run RuntimeWorkerDispatchRetrySchedulerRun
	if err := json.Unmarshal(data, &run); err != nil {
		return RuntimeWorkerDispatchRetrySchedulerRun{}, fmt.Errorf("decode worker dispatch retry scheduler run %s: %w", path, err)
	}
	run.Path = path
	return run, nil
}

func resolveRuntimeWorkerDispatchRetrySchedulerRunPath(root, runPath string) (string, error) {
	trimmed := strings.TrimSpace(runPath)
	if trimmed == "" {
		return "", fmt.Errorf("worker dispatch retry scheduler run path is required")
	}
	dir, err := filepath.Abs(runtimeWorkerDispatchRetrySchedulerRunDir(root))
	if err != nil {
		return "", fmt.Errorf("resolve worker dispatch retry scheduler run directory: %w", err)
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
		return "", fmt.Errorf("resolve worker dispatch retry scheduler run path: %w", err)
	}
	rel, err := filepath.Rel(dir, candidate)
	if err != nil {
		return "", fmt.Errorf("resolve worker dispatch retry scheduler run relative path: %w", err)
	}
	if rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("worker dispatch retry scheduler run path must stay under %s", dir)
	}
	if filepath.Ext(candidate) != ".json" {
		return "", fmt.Errorf("worker dispatch retry scheduler run path must be a json file")
	}
	return candidate, nil
}

func runtimeWorkerDispatchRetrySchedulerRunDir(root string) string {
	return filepath.Join(root, "runtime", "worker-dispatch-retry-scheduler-runs")
}

func safeRuntimeWorkerDispatchRetrySchedulerRunName(run RuntimeWorkerDispatchRetrySchedulerRun) string {
	parts := []string{
		"worker-dispatch-retry-scheduler-run",
		run.CreatedAt,
		run.Status,
		run.Mode,
		run.Actor,
		fmt.Sprintf("max-runs=%d", run.MaxRunsRequested),
		fmt.Sprintf("runs=%d", run.RunsCreated),
		fmt.Sprintf("plans=%d", run.RetryPlansCreated),
		fmt.Sprintf("dispatches=%d", run.RetryDispatchesCreated),
		fmt.Sprintf("executions=%d", run.RetryDispatchExecutionsCreated),
		fmt.Sprintf("claims=%d", run.RetryClaimsCreated),
		fmt.Sprintf("handoff=%t", run.DispatchRetryClaims),
		run.RetryTransportMode,
		run.SkipReason,
	}
	name := safeInstallRequestName(strings.Join(parts, "|"))
	if index := strings.LastIndex(name, "-"); index >= 0 && len(name[index+1:]) == 12 {
		return "worker-dispatch-retry-scheduler-run-" + name[index+1:]
	}
	return name
}
