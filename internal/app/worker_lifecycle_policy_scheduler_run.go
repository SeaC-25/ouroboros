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

type WorkerLifecyclePolicySchedulerRunOptions struct {
	Execute bool
	MaxRuns int
	Now     time.Time
	Actor   string
	Reason  string
}

type WorkerLifecyclePolicySchedulerRun struct {
	Version                      int                         `json:"version"`
	Path                         string                      `json:"path,omitempty"`
	Status                       string                      `json:"status"`
	Mode                         string                      `json:"mode"`
	CreatedAt                    string                      `json:"created_at"`
	CompletedAt                  string                      `json:"completed_at,omitempty"`
	Actor                        string                      `json:"actor"`
	Reason                       string                      `json:"reason,omitempty"`
	SelectionPath                string                      `json:"selection_path"`
	PolicyPath                   string                      `json:"policy_path"`
	PolicyBasename               string                      `json:"policy_basename"`
	PollIntervalMilliseconds     int64                       `json:"poll_interval_ms"`
	MaxRunsRequested             int                         `json:"max_runs_requested"`
	MaxRuns                      int                         `json:"max_runs,omitempty"`
	PolicyMaxSupervisorRuns      int                         `json:"policy_max_supervisor_runs"`
	TransportMode                string                      `json:"transport"`
	IncludeLifecycleMaintenance  bool                        `json:"include_lifecycle_maintenance"`
	ExecuteLifecycleRetries      bool                        `json:"execute_lifecycle_retries"`
	DispatchLifecycleRetryClaims bool                        `json:"dispatch_lifecycle_retry_claims,omitempty"`
	LifecycleRetryTransportMode  string                      `json:"lifecycle_retry_transport_mode,omitempty"`
	ExecuteLeaseRecovery         bool                        `json:"execute_lease_recovery"`
	PolicySummary                WorkerLifecycleAuditSummary `json:"policy_summary"`
	PolicyRunPaths               []string                    `json:"policy_run_paths,omitempty"`
	PolicyRunsCreated            int                         `json:"policy_runs_created"`
	SupervisorCreated            bool                        `json:"supervisor_created,omitempty"`
	SupervisorsCreated           int                         `json:"supervisors_created"`
	SkipReason                   string                      `json:"skip_reason,omitempty"`
	Error                        string                      `json:"error,omitempty"`
}

func CreateWorkerLifecyclePolicySchedulerRun(root string, options WorkerLifecyclePolicySchedulerRunOptions) (WorkerLifecyclePolicySchedulerRun, error) {
	if options.Execute && options.MaxRuns <= 0 {
		return WorkerLifecyclePolicySchedulerRun{}, fmt.Errorf("worker lifecycle policy scheduler run max runs must be positive")
	}
	if !options.Execute && options.MaxRuns < 0 {
		return WorkerLifecyclePolicySchedulerRun{}, fmt.Errorf("worker lifecycle policy scheduler run max runs cannot be negative")
	}
	selection, policy, err := currentWorkerLifecyclePolicySelection(root)
	if err != nil {
		return WorkerLifecyclePolicySchedulerRun{}, err
	}
	now := options.Now.UTC()
	if now.IsZero() {
		now = time.Now().UTC()
	}
	actor := strings.TrimSpace(options.Actor)
	if actor == "" {
		actor = "operator"
	}
	run := WorkerLifecyclePolicySchedulerRun{
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
		MaxRunsRequested:             options.MaxRuns,
		MaxRuns:                      options.MaxRuns,
		PolicyMaxSupervisorRuns:      policy.MaxSupervisorRuns,
		TransportMode:                policy.TransportMode,
		IncludeLifecycleMaintenance:  policy.IncludeLifecycleMaintenance,
		ExecuteLifecycleRetries:      policy.ExecuteLifecycleRetries,
		DispatchLifecycleRetryClaims: policy.DispatchLifecycleRetryClaims,
		LifecycleRetryTransportMode:  policy.LifecycleRetryTransportMode,
		ExecuteLeaseRecovery:         policy.ExecuteLeaseRecovery,
		PolicySummary:                policy.Summary,
	}
	if !options.Execute {
		if err := writeWorkerLifecyclePolicySchedulerRun(root, &run); err != nil {
			return run, err
		}
		return run, nil
	}
	if policy.PollIntervalMilliseconds == 0 {
		run.Status = "completed"
		run.SkipReason = "lifecycle-policy-scheduler-disabled"
		run.CompletedAt = now.Format(time.RFC3339Nano)
		if err := writeWorkerLifecyclePolicySchedulerRun(root, &run); err != nil {
			return run, err
		}
		return run, nil
	}

	run.Status = "completed"
	for i := 0; i < options.MaxRuns; i++ {
		childNow := now.Add(time.Duration(i) * time.Second)
		child, err := CreateWorkerLifecyclePolicyRun(root, WorkerLifecyclePolicyRunOptions{
			Execute: true,
			Now:     childNow,
			Actor:   actor,
			Reason:  scheduledWorkerLifecyclePolicyRunReason(options.Reason, i+1, options.MaxRuns),
		})
		if err != nil {
			run.Status = "failed"
			run.Error = err.Error()
			if writeErr := writeWorkerLifecyclePolicySchedulerRun(root, &run); writeErr != nil {
				return run, writeErr
			}
			return run, err
		}
		run.PolicyRunPaths = append(run.PolicyRunPaths, child.Path)
		run.PolicyRunsCreated++
		if strings.TrimSpace(child.SupervisorPath) != "" {
			run.SupervisorCreated = true
			run.SupervisorsCreated++
		}
	}
	run.CompletedAt = now.Add(time.Duration(options.MaxRuns) * time.Second).Format(time.RFC3339Nano)
	if err := writeWorkerLifecyclePolicySchedulerRun(root, &run); err != nil {
		return run, err
	}
	return run, nil
}

func ListWorkerLifecyclePolicySchedulerRuns(root string) ([]WorkerLifecyclePolicySchedulerRun, error) {
	paths, err := filepath.Glob(filepath.Join(workerLifecyclePolicySchedulerRunDir(root), "*.json"))
	if err != nil {
		return nil, fmt.Errorf("glob worker lifecycle policy scheduler run artifacts: %w", err)
	}
	sort.Strings(paths)
	runs := make([]WorkerLifecyclePolicySchedulerRun, 0, len(paths))
	for _, path := range paths {
		run, err := readWorkerLifecyclePolicySchedulerRunFile(path)
		if err != nil {
			return nil, err
		}
		runs = append(runs, run)
	}
	return runs, nil
}

func ReadWorkerLifecyclePolicySchedulerRun(root, runPath string) (WorkerLifecyclePolicySchedulerRun, error) {
	path, err := resolveWorkerLifecyclePolicySchedulerRunPath(root, runPath)
	if err != nil {
		return WorkerLifecyclePolicySchedulerRun{}, err
	}
	return readWorkerLifecyclePolicySchedulerRunFile(path)
}

func writeWorkerLifecyclePolicySchedulerRun(root string, run *WorkerLifecyclePolicySchedulerRun) error {
	if err := os.MkdirAll(workerLifecyclePolicySchedulerRunDir(root), 0o755); err != nil {
		return fmt.Errorf("create worker lifecycle policy scheduler run directory: %w", err)
	}
	if strings.TrimSpace(run.Path) == "" {
		run.Path = filepath.Join(workerLifecyclePolicySchedulerRunDir(root), safeWorkerLifecyclePolicySchedulerRunName(*run)+".json")
	}
	data, err := json.MarshalIndent(run, "", "  ")
	if err != nil {
		return fmt.Errorf("encode worker lifecycle policy scheduler run: %w", err)
	}
	data = append(data, '\n')
	tmp := run.Path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("write worker lifecycle policy scheduler run: %w", err)
	}
	if err := os.Rename(tmp, run.Path); err != nil {
		return fmt.Errorf("publish worker lifecycle policy scheduler run: %w", err)
	}
	return nil
}

func readWorkerLifecyclePolicySchedulerRunFile(path string) (WorkerLifecyclePolicySchedulerRun, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return WorkerLifecyclePolicySchedulerRun{}, fmt.Errorf("read worker lifecycle policy scheduler run %s: %w", path, err)
	}
	var run WorkerLifecyclePolicySchedulerRun
	if err := json.Unmarshal(data, &run); err != nil {
		return WorkerLifecyclePolicySchedulerRun{}, fmt.Errorf("decode worker lifecycle policy scheduler run %s: %w", path, err)
	}
	run.Path = path
	return run, nil
}

func resolveWorkerLifecyclePolicySchedulerRunPath(root, runPath string) (string, error) {
	trimmed := strings.TrimSpace(runPath)
	if trimmed == "" {
		return "", fmt.Errorf("worker lifecycle policy scheduler run path is required")
	}
	dir, err := filepath.Abs(workerLifecyclePolicySchedulerRunDir(root))
	if err != nil {
		return "", fmt.Errorf("resolve worker lifecycle policy scheduler run directory: %w", err)
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
		return "", fmt.Errorf("resolve worker lifecycle policy scheduler run path: %w", err)
	}
	rel, err := filepath.Rel(dir, candidate)
	if err != nil {
		return "", fmt.Errorf("resolve worker lifecycle policy scheduler run relative path: %w", err)
	}
	if rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("worker lifecycle policy scheduler run path must stay under %s", dir)
	}
	if filepath.Ext(candidate) != ".json" {
		return "", fmt.Errorf("worker lifecycle policy scheduler run path must be a json file")
	}
	return candidate, nil
}

func workerLifecyclePolicySchedulerRunDir(root string) string {
	return filepath.Join(root, "runtime", "worker-lifecycle-policy-scheduler-runs")
}

func safeWorkerLifecyclePolicySchedulerRunName(run WorkerLifecyclePolicySchedulerRun) string {
	return safeInstallRequestName(strings.Join([]string{
		"worker-lifecycle-policy-scheduler-run",
		run.CreatedAt,
		run.Status,
		run.Mode,
		run.Actor,
		filepath.Base(run.PolicyPath),
		fmt.Sprintf("poll-ms=%d", run.PollIntervalMilliseconds),
		fmt.Sprintf("max-runs=%d", run.MaxRunsRequested),
		fmt.Sprintf("policy-runs=%d", run.PolicyRunsCreated),
		fmt.Sprintf("supervisors=%d", run.SupervisorsCreated),
		fmt.Sprintf("retry-handoff=%t", run.DispatchLifecycleRetryClaims),
		run.LifecycleRetryTransportMode,
		fmt.Sprintf("lease-recovery=%t", run.ExecuteLeaseRecovery),
		run.SkipReason,
	}, "|"))
}

func scheduledWorkerLifecyclePolicyRunReason(reason string, index, total int) string {
	trimmed := strings.TrimSpace(reason)
	suffix := fmt.Sprintf("scheduled policy run %d/%d", index, total)
	if trimmed == "" {
		return suffix
	}
	return trimmed + "; " + suffix
}
