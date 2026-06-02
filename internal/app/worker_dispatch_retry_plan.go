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

type RuntimeWorkerDispatchRetrySummary struct {
	Total   int `json:"total"`
	Ready   int `json:"ready"`
	Skipped int `json:"skipped"`
}

type RuntimeWorkerDispatchRetryCandidate struct {
	Status               string                       `json:"status"`
	Gate                 string                       `json:"gate"`
	Mode                 string                       `json:"mode"`
	Reason               string                       `json:"reason"`
	ResultPath           string                       `json:"result_path"`
	DispatchPath         string                       `json:"dispatch_path"`
	ClaimPath            string                       `json:"claim_path"`
	TransportRequestPath string                       `json:"transport_request_path,omitempty"`
	WorkerID             string                       `json:"worker_id"`
	RunID                string                       `json:"run_id"`
	ExecutionID          string                       `json:"execution_id"`
	Tool                 string                       `json:"tool"`
	Capability           string                       `json:"capability"`
	ToolPlan             RuntimeWorkerToolPlanPayload `json:"tool_plan,omitempty"`
	LastStatus           string                       `json:"last_status"`
	LastOutput           string                       `json:"last_output,omitempty"`
	LastError            string                       `json:"last_error,omitempty"`
}

type RuntimeWorkerDispatchRetrySkip struct {
	ResultPath   string `json:"result_path"`
	DispatchPath string `json:"dispatch_path"`
	ClaimPath    string `json:"claim_path"`
	WorkerID     string `json:"worker_id"`
	RunID        string `json:"run_id"`
	ExecutionID  string `json:"execution_id"`
	Tool         string `json:"tool"`
	Capability   string `json:"capability"`
	Status       string `json:"status"`
	Reason       string `json:"reason"`
}

type RuntimeWorkerDispatchRetryPlan struct {
	Version   int                                   `json:"version"`
	Path      string                                `json:"-"`
	Status    string                                `json:"status"`
	Mode      string                                `json:"mode"`
	CreatedAt string                                `json:"created_at,omitempty"`
	Summary   RuntimeWorkerDispatchRetrySummary     `json:"summary"`
	Ready     []RuntimeWorkerDispatchRetryCandidate `json:"ready"`
	Skipped   []RuntimeWorkerDispatchRetrySkip      `json:"skipped"`
}

func PlanRuntimeWorkerDispatchRetries(root string) (RuntimeWorkerDispatchRetryPlan, error) {
	results, err := ListRuntimeWorkerQueueDispatchResults(root)
	if err != nil {
		return RuntimeWorkerDispatchRetryPlan{}, err
	}
	plan := RuntimeWorkerDispatchRetryPlan{
		Version: 1,
		Status:  "dry-run",
		Mode:    "metadata-only",
	}
	for _, result := range results {
		dispatch, err := ReadRuntimeWorkerQueueDispatchExecution(root, result.DispatchExecutionPath)
		if err != nil {
			return RuntimeWorkerDispatchRetryPlan{}, fmt.Errorf("read worker dispatch for retry plan: %w", err)
		}
		status := strings.TrimSpace(result.Status)
		plan.Summary.Total++
		if status == "failed" {
			plan.Ready = append(plan.Ready, RuntimeWorkerDispatchRetryCandidate{
				Status:               "ready",
				Gate:                 "worker-dispatch-retry-review",
				Mode:                 "metadata-only",
				Reason:               "failed-dispatch-result",
				ResultPath:           result.Path,
				DispatchPath:         dispatch.Path,
				ClaimPath:            result.ClaimPath,
				TransportRequestPath: dispatch.TransportRequestPath,
				WorkerID:             result.WorkerID,
				RunID:                result.RunID,
				ExecutionID:          result.ExecutionID,
				Tool:                 result.Tool,
				Capability:           result.Capability,
				ToolPlan:             normalizeRuntimeWorkerToolPlanPayload(result.ToolPlan, result.Tool),
				LastStatus:           status,
				LastOutput:           result.Output,
				LastError:            result.Error,
			})
			plan.Summary.Ready++
			continue
		}
		plan.Skipped = append(plan.Skipped, RuntimeWorkerDispatchRetrySkip{
			ResultPath:   result.Path,
			DispatchPath: dispatch.Path,
			ClaimPath:    result.ClaimPath,
			WorkerID:     result.WorkerID,
			RunID:        result.RunID,
			ExecutionID:  result.ExecutionID,
			Tool:         result.Tool,
			Capability:   result.Capability,
			Status:       status,
			Reason:       "result-not-failed",
		})
		plan.Summary.Skipped++
	}
	return plan, nil
}

func CreateRuntimeWorkerDispatchRetryPlan(root string, now time.Time) (RuntimeWorkerDispatchRetryPlan, error) {
	if now.IsZero() {
		now = time.Now().UTC()
	} else {
		now = now.UTC()
	}
	plan, err := PlanRuntimeWorkerDispatchRetries(root)
	if err != nil {
		return RuntimeWorkerDispatchRetryPlan{}, err
	}
	plan.CreatedAt = now.Format(time.RFC3339Nano)
	dir := runtimeWorkerDispatchRetryPlanDir(root)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return RuntimeWorkerDispatchRetryPlan{}, fmt.Errorf("create worker dispatch retry plan directory: %w", err)
	}
	plan.Path = filepath.Join(dir, safeRuntimeWorkerDispatchRetryPlanName(plan)+".json")
	data, err := json.MarshalIndent(plan, "", "  ")
	if err != nil {
		return RuntimeWorkerDispatchRetryPlan{}, fmt.Errorf("encode worker dispatch retry plan: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(plan.Path, data, 0o644); err != nil {
		return RuntimeWorkerDispatchRetryPlan{}, fmt.Errorf("write worker dispatch retry plan: %w", err)
	}
	return plan, nil
}

func ListRuntimeWorkerDispatchRetryPlans(root string) ([]RuntimeWorkerDispatchRetryPlan, error) {
	paths, err := filepath.Glob(filepath.Join(runtimeWorkerDispatchRetryPlanDir(root), "*.json"))
	if err != nil {
		return nil, fmt.Errorf("glob worker dispatch retry plans: %w", err)
	}
	sort.Strings(paths)
	plans := make([]RuntimeWorkerDispatchRetryPlan, 0, len(paths))
	for _, path := range paths {
		plan, err := readRuntimeWorkerDispatchRetryPlanFile(path)
		if err != nil {
			return nil, err
		}
		plans = append(plans, plan)
	}
	return plans, nil
}

func ReadRuntimeWorkerDispatchRetryPlan(root, planPath string) (RuntimeWorkerDispatchRetryPlan, error) {
	path, err := resolveRuntimeWorkerDispatchRetryPlanPath(root, planPath)
	if err != nil {
		return RuntimeWorkerDispatchRetryPlan{}, err
	}
	return readRuntimeWorkerDispatchRetryPlanFile(path)
}

func readRuntimeWorkerDispatchRetryPlanFile(path string) (RuntimeWorkerDispatchRetryPlan, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return RuntimeWorkerDispatchRetryPlan{}, fmt.Errorf("read worker dispatch retry plan %s: %w", path, err)
	}
	var plan RuntimeWorkerDispatchRetryPlan
	if err := json.Unmarshal(data, &plan); err != nil {
		return RuntimeWorkerDispatchRetryPlan{}, fmt.Errorf("decode worker dispatch retry plan %s: %w", path, err)
	}
	plan.Path = path
	return plan, nil
}

func runtimeWorkerDispatchRetryPlanDir(root string) string {
	return filepath.Join(root, "runtime", "worker-dispatch-retry-plans")
}

func resolveRuntimeWorkerDispatchRetryPlanPath(root, planPath string) (string, error) {
	trimmed := strings.TrimSpace(planPath)
	if trimmed == "" {
		return "", fmt.Errorf("worker dispatch retry plan path is required")
	}
	dir, err := filepath.Abs(runtimeWorkerDispatchRetryPlanDir(root))
	if err != nil {
		return "", fmt.Errorf("resolve worker dispatch retry plan directory: %w", err)
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
		return "", fmt.Errorf("resolve worker dispatch retry plan path: %w", err)
	}
	rel, err := filepath.Rel(dir, candidate)
	if err != nil {
		return "", fmt.Errorf("resolve worker dispatch retry plan relative path: %w", err)
	}
	if rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("worker dispatch retry plan path must stay under %s", dir)
	}
	if filepath.Ext(candidate) != ".json" {
		return "", fmt.Errorf("worker dispatch retry plan path must be a json file")
	}
	return candidate, nil
}

func safeRuntimeWorkerDispatchRetryPlanName(plan RuntimeWorkerDispatchRetryPlan) string {
	parts := []string{
		"worker-dispatch-retry-plan",
		plan.CreatedAt,
		fmt.Sprintf("total=%d ready=%d skipped=%d", plan.Summary.Total, plan.Summary.Ready, plan.Summary.Skipped),
	}
	for _, candidate := range plan.Ready {
		parts = append(parts, candidate.ResultPath, candidate.DispatchPath, candidate.ClaimPath, candidate.WorkerID, candidate.RunID, candidate.ExecutionID, candidate.LastStatus, candidate.LastError)
	}
	for _, skipped := range plan.Skipped {
		parts = append(parts, skipped.ResultPath, skipped.DispatchPath, skipped.ClaimPath, skipped.WorkerID, skipped.RunID, skipped.ExecutionID, skipped.Status, skipped.Reason)
	}
	name := safeInstallRequestName(strings.Join(parts, "|"))
	if index := strings.LastIndex(name, "-"); index >= 0 && len(name[index+1:]) == 12 {
		return "worker-dispatch-retry-plan-" + name[index+1:]
	}
	return name
}
