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

type WorkerLifecyclePolicyRunOptions struct {
	Execute bool
	Now     time.Time
	Actor   string
	Reason  string
}

type WorkerLifecyclePolicyRun struct {
	Version                      int                                         `json:"version"`
	Path                         string                                      `json:"path,omitempty"`
	Status                       string                                      `json:"status"`
	Mode                         string                                      `json:"mode"`
	CreatedAt                    string                                      `json:"created_at"`
	Actor                        string                                      `json:"actor"`
	Reason                       string                                      `json:"reason,omitempty"`
	SelectionPath                string                                      `json:"selection_path"`
	PolicyPath                   string                                      `json:"policy_path"`
	PolicyBasename               string                                      `json:"policy_basename"`
	PollIntervalMilliseconds     int64                                       `json:"poll_interval_ms"`
	MaxRuns                      int                                         `json:"max_runs"`
	TransportMode                string                                      `json:"transport"`
	IncludeLifecycleMaintenance  bool                                        `json:"include_lifecycle_maintenance"`
	ExecuteLifecycleRetries      bool                                        `json:"execute_lifecycle_retries"`
	DispatchLifecycleRetryClaims bool                                        `json:"dispatch_lifecycle_retry_claims,omitempty"`
	LifecycleRetryTransportMode  string                                      `json:"lifecycle_retry_transport_mode,omitempty"`
	ExecuteLeaseRecovery         bool                                        `json:"execute_lease_recovery"`
	PolicySummary                WorkerLifecycleAuditSummary                 `json:"policy_summary"`
	SupervisorPath               string                                      `json:"supervisor_path,omitempty"`
	SupervisorSummary            WorkerLifecycleMaintenanceSupervisorSummary `json:"supervisor_summary,omitempty"`
	SkipReason                   string                                      `json:"skip_reason,omitempty"`
	Error                        string                                      `json:"error,omitempty"`
}

func CreateWorkerLifecyclePolicyRun(root string, options WorkerLifecyclePolicyRunOptions) (WorkerLifecyclePolicyRun, error) {
	selection, policy, err := currentWorkerLifecyclePolicySelection(root)
	if err != nil {
		return WorkerLifecyclePolicyRun{}, err
	}
	now := options.Now.UTC()
	if now.IsZero() {
		now = time.Now().UTC()
	}
	actor := strings.TrimSpace(options.Actor)
	if actor == "" {
		actor = "operator"
	}
	run := WorkerLifecyclePolicyRun{
		Version:                      1,
		Status:                       "dry-run",
		Mode:                         "metadata-only",
		CreatedAt:                    now.Format(time.RFC3339Nano),
		Actor:                        actor,
		Reason:                       strings.TrimSpace(options.Reason),
		SelectionPath:                selection.Path,
		PolicyPath:                   policy.Path,
		PolicyBasename:               filepath.Base(policy.Path),
		PollIntervalMilliseconds:     policy.PollIntervalMilliseconds,
		MaxRuns:                      policy.MaxSupervisorRuns,
		TransportMode:                policy.TransportMode,
		IncludeLifecycleMaintenance:  policy.IncludeLifecycleMaintenance,
		ExecuteLifecycleRetries:      policy.ExecuteLifecycleRetries,
		DispatchLifecycleRetryClaims: policy.DispatchLifecycleRetryClaims,
		LifecycleRetryTransportMode:  policy.LifecycleRetryTransportMode,
		ExecuteLeaseRecovery:         policy.ExecuteLeaseRecovery,
		PolicySummary:                policy.Summary,
	}
	if !options.Execute {
		if err := writeWorkerLifecyclePolicyRun(root, &run); err != nil {
			return run, err
		}
		return run, nil
	}

	run.Status = "completed"
	if !policy.IncludeLifecycleMaintenance {
		run.SkipReason = "lifecycle-maintenance-disabled"
		if err := writeWorkerLifecyclePolicyRun(root, &run); err != nil {
			return run, err
		}
		return run, nil
	}

	supervisor, err := CreateWorkerLifecycleMaintenanceSupervisor(root, WorkerLifecycleMaintenanceSupervisorOptions{
		Execute:                    true,
		ExecuteRetryDispatches:     policy.ExecuteLifecycleRetries,
		DispatchRetryClaims:        policy.DispatchLifecycleRetryClaims,
		RetryDispatchTransportMode: policy.LifecycleRetryTransportMode,
		ExecuteLeaseRecovery:       policy.ExecuteLeaseRecovery,
		Now:                        now,
		StaleAfter:                 time.Duration(policy.StaleAfterSeconds) * time.Second,
		MaxRuns:                    policy.MaxSupervisorRuns,
	})
	if err != nil {
		run.Status = "failed"
		run.Error = err.Error()
		if writeErr := writeWorkerLifecyclePolicyRun(root, &run); writeErr != nil {
			return run, writeErr
		}
		return run, err
	}
	run.SupervisorPath = supervisor.Path
	run.SupervisorSummary = supervisor.Summary
	if err := writeWorkerLifecyclePolicyRun(root, &run); err != nil {
		return run, err
	}
	return run, nil
}

func ListWorkerLifecyclePolicyRuns(root string) ([]WorkerLifecyclePolicyRun, error) {
	paths, err := filepath.Glob(filepath.Join(workerLifecyclePolicyRunDir(root), "*.json"))
	if err != nil {
		return nil, fmt.Errorf("glob worker lifecycle policy run artifacts: %w", err)
	}
	sort.Strings(paths)
	runs := make([]WorkerLifecyclePolicyRun, 0, len(paths))
	for _, path := range paths {
		run, err := readWorkerLifecyclePolicyRunFile(path)
		if err != nil {
			return nil, err
		}
		runs = append(runs, run)
	}
	return runs, nil
}

func ReadWorkerLifecyclePolicyRun(root, runPath string) (WorkerLifecyclePolicyRun, error) {
	path, err := resolveWorkerLifecyclePolicyRunPath(root, runPath)
	if err != nil {
		return WorkerLifecyclePolicyRun{}, err
	}
	return readWorkerLifecyclePolicyRunFile(path)
}

func currentWorkerLifecyclePolicySelection(root string) (WorkerLifecyclePolicySelection, WorkerLifecyclePolicy, error) {
	selection, found, err := ReadWorkerLifecyclePolicySelection(root)
	if err != nil {
		return WorkerLifecyclePolicySelection{}, WorkerLifecyclePolicy{}, err
	}
	if !found || !workerLifecyclePolicySelectionIsCurrent(selection) {
		return WorkerLifecyclePolicySelection{}, WorkerLifecyclePolicy{}, fmt.Errorf("current worker lifecycle policy selection is required")
	}
	if strings.TrimSpace(selection.PolicyPath) == "" {
		return WorkerLifecyclePolicySelection{}, WorkerLifecyclePolicy{}, fmt.Errorf("current worker lifecycle policy selection requires policy path")
	}
	policy, err := ReadWorkerLifecyclePolicy(root, selection.PolicyPath)
	if err != nil {
		return WorkerLifecyclePolicySelection{}, WorkerLifecyclePolicy{}, err
	}
	if err := validateSelectedWorkerLifecyclePolicy(policy); err != nil {
		return WorkerLifecyclePolicySelection{}, WorkerLifecyclePolicy{}, fmt.Errorf("selected worker lifecycle policy is invalid: %w", err)
	}
	return selection, policy, nil
}

func validateSelectedWorkerLifecyclePolicy(policy WorkerLifecyclePolicy) error {
	if policy.StaleAfterSeconds <= 0 {
		return fmt.Errorf("worker lifecycle policy stale after must be positive")
	}
	if policy.WorkerStaleAfterSeconds <= 0 {
		return fmt.Errorf("worker lifecycle policy worker stale after must be positive")
	}
	if policy.PollIntervalSeconds < 0 || policy.PollIntervalMilliseconds < 0 {
		return fmt.Errorf("worker lifecycle policy poll interval cannot be negative")
	}
	if policy.MaxSupervisorRuns <= 0 {
		return fmt.Errorf("worker lifecycle policy max supervisor runs must be positive")
	}
	if _, err := normalizeRuntimeWorkerQueueDispatchTransport(policy.TransportMode); err != nil {
		return err
	}
	if _, err := normalizeWorkerLifecycleRetryTransportMode(policy.LifecycleRetryTransportMode); err != nil {
		return err
	}
	if policy.ExecuteLifecycleRetries && !policy.IncludeLifecycleMaintenance {
		return fmt.Errorf("worker lifecycle policy retry execution requires lifecycle maintenance")
	}
	if policy.DispatchLifecycleRetryClaims && !policy.IncludeLifecycleMaintenance {
		return fmt.Errorf("worker lifecycle policy retry handoff requires lifecycle maintenance")
	}
	if policy.DispatchLifecycleRetryClaims && !policy.ExecuteLifecycleRetries {
		return fmt.Errorf("worker lifecycle policy retry handoff requires lifecycle retry execution")
	}
	if policy.ExecuteLeaseRecovery && !policy.IncludeLifecycleMaintenance {
		return fmt.Errorf("worker lifecycle policy lease recovery execution requires lifecycle maintenance")
	}
	return nil
}

func writeWorkerLifecyclePolicyRun(root string, run *WorkerLifecyclePolicyRun) error {
	if err := os.MkdirAll(workerLifecyclePolicyRunDir(root), 0o755); err != nil {
		return fmt.Errorf("create worker lifecycle policy run directory: %w", err)
	}
	if strings.TrimSpace(run.Path) == "" {
		run.Path = filepath.Join(workerLifecyclePolicyRunDir(root), safeWorkerLifecyclePolicyRunName(*run)+".json")
	}
	data, err := json.MarshalIndent(run, "", "  ")
	if err != nil {
		return fmt.Errorf("encode worker lifecycle policy run: %w", err)
	}
	data = append(data, '\n')
	tmp := run.Path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("write worker lifecycle policy run: %w", err)
	}
	if err := os.Rename(tmp, run.Path); err != nil {
		return fmt.Errorf("publish worker lifecycle policy run: %w", err)
	}
	return nil
}

func readWorkerLifecyclePolicyRunFile(path string) (WorkerLifecyclePolicyRun, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return WorkerLifecyclePolicyRun{}, fmt.Errorf("read worker lifecycle policy run %s: %w", path, err)
	}
	var run WorkerLifecyclePolicyRun
	if err := json.Unmarshal(data, &run); err != nil {
		return WorkerLifecyclePolicyRun{}, fmt.Errorf("decode worker lifecycle policy run %s: %w", path, err)
	}
	run.Path = path
	return run, nil
}

func resolveWorkerLifecyclePolicyRunPath(root, runPath string) (string, error) {
	trimmed := strings.TrimSpace(runPath)
	if trimmed == "" {
		return "", fmt.Errorf("worker lifecycle policy run path is required")
	}
	dir, err := filepath.Abs(workerLifecyclePolicyRunDir(root))
	if err != nil {
		return "", fmt.Errorf("resolve worker lifecycle policy run directory: %w", err)
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
		return "", fmt.Errorf("resolve worker lifecycle policy run path: %w", err)
	}
	rel, err := filepath.Rel(dir, candidate)
	if err != nil {
		return "", fmt.Errorf("resolve worker lifecycle policy run relative path: %w", err)
	}
	if rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("worker lifecycle policy run path must stay under %s", dir)
	}
	if filepath.Ext(candidate) != ".json" {
		return "", fmt.Errorf("worker lifecycle policy run path must be a json file")
	}
	return candidate, nil
}

func workerLifecyclePolicyRunDir(root string) string {
	return filepath.Join(root, "runtime", "worker-lifecycle-policy-runs")
}

func safeWorkerLifecyclePolicyRunName(run WorkerLifecyclePolicyRun) string {
	return safeInstallRequestName(strings.Join([]string{
		"worker-lifecycle-policy-run",
		run.CreatedAt,
		run.Status,
		run.Mode,
		run.Actor,
		filepath.Base(run.PolicyPath),
		fmt.Sprintf("poll-ms=%d", run.PollIntervalMilliseconds),
		fmt.Sprintf("max-runs=%d", run.MaxRuns),
		run.TransportMode,
		fmt.Sprintf("maintenance=%t", run.IncludeLifecycleMaintenance),
		fmt.Sprintf("retries=%t", run.ExecuteLifecycleRetries),
		fmt.Sprintf("retry-handoff=%t", run.DispatchLifecycleRetryClaims),
		run.LifecycleRetryTransportMode,
		fmt.Sprintf("lease-recovery=%t", run.ExecuteLeaseRecovery),
		run.SkipReason,
	}, "|"))
}
