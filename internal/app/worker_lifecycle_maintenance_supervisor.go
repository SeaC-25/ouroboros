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

type WorkerLifecycleMaintenanceSupervisorOptions struct {
	Execute                    bool
	ExecuteRetryDispatches     bool
	DispatchRetryClaims        bool
	RetryDispatchTransportMode string
	ExecuteLeaseRecovery       bool
	Now                        time.Time
	StaleAfter                 time.Duration
	MaxRuns                    int
}

type WorkerLifecycleMaintenanceSupervisorSummary struct {
	Runs                                  int `json:"runs"`
	MaxRuns                               int `json:"max_runs"`
	ActionsTotal                          int `json:"actions_total"`
	AckActions                            int `json:"ack_actions"`
	RunActions                            int `json:"run_actions"`
	DaemonControlActions                  int `json:"daemon_control_actions"`
	RetryDispatchExecuteActions           int `json:"retry_dispatch_execute_actions"`
	ReclaimActions                        int `json:"reclaim_actions"`
	ReconciliationActions                 int `json:"reconciliation_actions"`
	SkippedUnsafeActions                  int `json:"skipped_unsafe_actions"`
	MaintenanceArtifactsCreated           int `json:"maintenance_artifacts_created"`
	AuditArtifactsCreated                 int `json:"audit_artifacts_created"`
	RetryDispatchExecutionsCreated        int `json:"retry_dispatch_executions_created"`
	RetryDispatchClaimsCreated            int `json:"retry_dispatch_claims_created"`
	RetryDispatchClaimsDispatched         int `json:"retry_dispatch_claims_dispatched,omitempty"`
	RetryDispatchTransportRequestsCreated int `json:"retry_dispatch_transport_requests_created,omitempty"`
	ReclaimExecutionsCreated              int `json:"reclaim_executions_created"`
	ReclaimResultsCreated                 int `json:"reclaim_results_created"`
	ReconciliationExecutionsCreated       int `json:"reconciliation_executions_created"`
	ReconciliationResultsCreated          int `json:"reconciliation_results_created"`
}

type WorkerLifecycleMaintenanceSupervisor struct {
	Version                      int                                         `json:"version"`
	Path                         string                                      `json:"path,omitempty"`
	Status                       string                                      `json:"status"`
	Mode                         string                                      `json:"mode"`
	CreatedAt                    string                                      `json:"created_at"`
	CompletedAt                  string                                      `json:"completed_at,omitempty"`
	StaleAfterSeconds            int64                                       `json:"stale_after_seconds"`
	Summary                      WorkerLifecycleMaintenanceSupervisorSummary `json:"summary"`
	MaintenancePaths             []string                                    `json:"maintenance_paths,omitempty"`
	AuditPaths                   []string                                    `json:"audit_paths,omitempty"`
	RetryDispatchExecutionPaths  []string                                    `json:"retry_dispatch_execution_paths,omitempty"`
	ReclaimExecutionPaths        []string                                    `json:"reclaim_execution_paths,omitempty"`
	ReconciliationExecutionPaths []string                                    `json:"reconciliation_execution_paths,omitempty"`
	DispatchRetryClaims          bool                                        `json:"dispatch_retry_claims,omitempty"`
	RetryDispatchTransportMode   string                                      `json:"retry_dispatch_transport_mode,omitempty"`
	ExecuteLeaseRecovery         bool                                        `json:"execute_lease_recovery,omitempty"`
	Error                        string                                      `json:"error,omitempty"`
}

func CreateWorkerLifecycleMaintenanceSupervisor(root string, options WorkerLifecycleMaintenanceSupervisorOptions) (WorkerLifecycleMaintenanceSupervisor, error) {
	now := options.Now.UTC()
	if now.IsZero() {
		now = time.Now().UTC()
	}
	staleAfter := options.StaleAfter
	if staleAfter <= 0 {
		staleAfter = DefaultWorkerLeaseStaleAfter
	}
	maxRuns := options.MaxRuns
	if maxRuns <= 0 {
		maxRuns = 1
	}
	retryDispatchTransportMode, err := normalizeWorkerLifecycleRetryHandoffOptions("worker lifecycle maintenance supervisor", options.Execute, options.ExecuteRetryDispatches, options.DispatchRetryClaims, options.RetryDispatchTransportMode)
	if err != nil {
		return WorkerLifecycleMaintenanceSupervisor{}, err
	}
	supervisor := WorkerLifecycleMaintenanceSupervisor{
		Version:                    1,
		Status:                     "dry-run",
		Mode:                       "metadata-only",
		CreatedAt:                  now.Format(time.RFC3339Nano),
		StaleAfterSeconds:          int64(staleAfter / time.Second),
		DispatchRetryClaims:        options.DispatchRetryClaims,
		RetryDispatchTransportMode: retryDispatchTransportMode,
		ExecuteLeaseRecovery:       options.ExecuteLeaseRecovery,
		Summary: WorkerLifecycleMaintenanceSupervisorSummary{
			MaxRuns: maxRuns,
		},
	}
	if !options.Execute {
		if err := writeWorkerLifecycleMaintenanceSupervisor(root, &supervisor); err != nil {
			return supervisor, err
		}
		return supervisor, nil
	}

	supervisor.Status = "completed"
	for run := 0; run < maxRuns; run++ {
		runNow := now.Add(time.Duration(run) * time.Nanosecond)
		maintenance, err := CreateWorkerLifecycleMaintenance(root, WorkerLifecycleMaintenanceOptions{
			Execute:                    true,
			ExecuteRetryDispatches:     options.ExecuteRetryDispatches,
			DispatchRetryClaims:        options.DispatchRetryClaims,
			RetryDispatchTransportMode: retryDispatchTransportMode,
			ExecuteLeaseRecovery:       options.ExecuteLeaseRecovery,
			Now:                        runNow,
			StaleAfter:                 staleAfter,
		})
		if err != nil {
			return failWorkerLifecycleMaintenanceSupervisor(root, supervisor, err)
		}
		supervisor.MaintenancePaths = append(supervisor.MaintenancePaths, maintenance.Path)
		supervisor.RetryDispatchExecutionPaths = append(supervisor.RetryDispatchExecutionPaths, maintenance.RetryDispatchExecutionPaths...)
		if strings.TrimSpace(maintenance.ReclaimExecutionPath) != "" {
			supervisor.ReclaimExecutionPaths = append(supervisor.ReclaimExecutionPaths, maintenance.ReclaimExecutionPath)
		}
		if strings.TrimSpace(maintenance.ReconciliationExecutionPath) != "" {
			supervisor.ReconciliationExecutionPaths = append(supervisor.ReconciliationExecutionPaths, maintenance.ReconciliationExecutionPath)
		}
		supervisor.Summary.MaintenanceArtifactsCreated++
		aggregateWorkerLifecycleMaintenanceSupervisorSummary(&supervisor.Summary, maintenance.Summary)

		audit, err := CreateWorkerLifecycleAudit(root, runNow, staleAfter)
		if err != nil {
			return failWorkerLifecycleMaintenanceSupervisor(root, supervisor, err)
		}
		supervisor.AuditPaths = append(supervisor.AuditPaths, audit.Path)
		supervisor.Summary.AuditArtifactsCreated++
		supervisor.Summary.Runs++
	}
	supervisor.CompletedAt = now.Add(time.Duration(maxRuns) * time.Nanosecond).Format(time.RFC3339Nano)
	if err := writeWorkerLifecycleMaintenanceSupervisor(root, &supervisor); err != nil {
		return supervisor, err
	}
	return supervisor, nil
}

func ListWorkerLifecycleMaintenanceSupervisors(root string) ([]WorkerLifecycleMaintenanceSupervisor, error) {
	paths, err := filepath.Glob(filepath.Join(workerLifecycleMaintenanceSupervisorDir(root), "*.json"))
	if err != nil {
		return nil, fmt.Errorf("glob worker lifecycle maintenance supervisor artifacts: %w", err)
	}
	sort.Strings(paths)
	supervisors := make([]WorkerLifecycleMaintenanceSupervisor, 0, len(paths))
	for _, path := range paths {
		supervisor, err := readWorkerLifecycleMaintenanceSupervisorFile(path)
		if err != nil {
			return nil, err
		}
		supervisors = append(supervisors, supervisor)
	}
	return supervisors, nil
}

func ReadWorkerLifecycleMaintenanceSupervisor(root, supervisorPath string) (WorkerLifecycleMaintenanceSupervisor, error) {
	path, err := resolveWorkerLifecycleMaintenanceSupervisorPath(root, supervisorPath)
	if err != nil {
		return WorkerLifecycleMaintenanceSupervisor{}, err
	}
	return readWorkerLifecycleMaintenanceSupervisorFile(path)
}

func failWorkerLifecycleMaintenanceSupervisor(root string, supervisor WorkerLifecycleMaintenanceSupervisor, err error) (WorkerLifecycleMaintenanceSupervisor, error) {
	supervisor.Status = "failed"
	supervisor.Error = err.Error()
	if writeErr := writeWorkerLifecycleMaintenanceSupervisor(root, &supervisor); writeErr != nil {
		return supervisor, writeErr
	}
	return supervisor, err
}

func writeWorkerLifecycleMaintenanceSupervisor(root string, supervisor *WorkerLifecycleMaintenanceSupervisor) error {
	if err := os.MkdirAll(workerLifecycleMaintenanceSupervisorDir(root), 0o755); err != nil {
		return fmt.Errorf("create worker lifecycle maintenance supervisor directory: %w", err)
	}
	if strings.TrimSpace(supervisor.Path) == "" {
		supervisor.Path = filepath.Join(workerLifecycleMaintenanceSupervisorDir(root), safeWorkerLifecycleMaintenanceSupervisorID(*supervisor)+".json")
	}
	data, err := json.MarshalIndent(supervisor, "", "  ")
	if err != nil {
		return fmt.Errorf("encode worker lifecycle maintenance supervisor: %w", err)
	}
	data = append(data, '\n')
	tmp := supervisor.Path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("write worker lifecycle maintenance supervisor: %w", err)
	}
	if err := os.Rename(tmp, supervisor.Path); err != nil {
		return fmt.Errorf("publish worker lifecycle maintenance supervisor: %w", err)
	}
	return nil
}

func aggregateWorkerLifecycleMaintenanceSupervisorSummary(summary *WorkerLifecycleMaintenanceSupervisorSummary, maintenance WorkerLifecycleMaintenanceSummary) {
	summary.ActionsTotal += maintenance.ActionsTotal
	summary.AckActions += maintenance.AckActions
	summary.RunActions += maintenance.RunActions
	summary.DaemonControlActions += maintenance.DaemonControlActions
	summary.RetryDispatchExecuteActions += maintenance.RetryDispatchExecuteActions
	summary.ReclaimActions += maintenance.ReclaimActions
	summary.ReconciliationActions += maintenance.ReconciliationActions
	summary.SkippedUnsafeActions += maintenance.SkippedUnsafeActions
	summary.RetryDispatchExecutionsCreated += maintenance.RetryDispatchExecutionsCreated
	summary.RetryDispatchClaimsCreated += maintenance.RetryDispatchClaimsCreated
	summary.RetryDispatchClaimsDispatched += maintenance.RetryDispatchClaimsDispatched
	summary.RetryDispatchTransportRequestsCreated += maintenance.RetryDispatchTransportRequestsCreated
	summary.ReclaimExecutionsCreated += maintenance.ReclaimExecutionsCreated
	summary.ReclaimResultsCreated += maintenance.ReclaimResultsCreated
	summary.ReconciliationExecutionsCreated += maintenance.ReconciliationExecutionsCreated
	summary.ReconciliationResultsCreated += maintenance.ReconciliationResultsCreated
}

func readWorkerLifecycleMaintenanceSupervisorFile(path string) (WorkerLifecycleMaintenanceSupervisor, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return WorkerLifecycleMaintenanceSupervisor{}, fmt.Errorf("read worker lifecycle maintenance supervisor %s: %w", path, err)
	}
	var supervisor WorkerLifecycleMaintenanceSupervisor
	if err := json.Unmarshal(data, &supervisor); err != nil {
		return WorkerLifecycleMaintenanceSupervisor{}, fmt.Errorf("decode worker lifecycle maintenance supervisor %s: %w", path, err)
	}
	supervisor.Path = path
	return supervisor, nil
}

func resolveWorkerLifecycleMaintenanceSupervisorPath(root, supervisorPath string) (string, error) {
	trimmed := strings.TrimSpace(supervisorPath)
	if trimmed == "" {
		return "", fmt.Errorf("worker lifecycle maintenance supervisor path is required")
	}
	dir, err := filepath.Abs(workerLifecycleMaintenanceSupervisorDir(root))
	if err != nil {
		return "", fmt.Errorf("resolve worker lifecycle maintenance supervisor directory: %w", err)
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
		return "", fmt.Errorf("resolve worker lifecycle maintenance supervisor path: %w", err)
	}
	rel, err := filepath.Rel(dir, candidate)
	if err != nil {
		return "", fmt.Errorf("resolve worker lifecycle maintenance supervisor relative path: %w", err)
	}
	if rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("worker lifecycle maintenance supervisor path must stay under %s", dir)
	}
	if filepath.Ext(candidate) != ".json" {
		return "", fmt.Errorf("worker lifecycle maintenance supervisor path must be a json file")
	}
	return candidate, nil
}

func workerLifecycleMaintenanceSupervisorDir(root string) string {
	return filepath.Join(root, "runtime", "worker-lifecycle-maintenance-supervisor-runs")
}

func safeWorkerLifecycleMaintenanceSupervisorID(supervisor WorkerLifecycleMaintenanceSupervisor) string {
	return safeInstallRequestName(strings.Join([]string{
		"worker-lifecycle-maintenance-supervisor",
		supervisor.Status,
		supervisor.Mode,
		supervisor.CreatedAt,
		fmt.Sprintf("runs-%d", supervisor.Summary.Runs),
		fmt.Sprintf("max-%d", supervisor.Summary.MaxRuns),
		fmt.Sprintf("maintenance-%d", supervisor.Summary.MaintenanceArtifactsCreated),
		fmt.Sprintf("audits-%d", supervisor.Summary.AuditArtifactsCreated),
		fmt.Sprintf("retry-executions-%d", supervisor.Summary.RetryDispatchExecutionsCreated),
		fmt.Sprintf("retry-claims-%d", supervisor.Summary.RetryDispatchClaimsCreated),
		fmt.Sprintf("lease-recovery=%t", supervisor.ExecuteLeaseRecovery),
		fmt.Sprintf("reclaim-executions-%d", supervisor.Summary.ReclaimExecutionsCreated),
		fmt.Sprintf("reconcile-executions-%d", supervisor.Summary.ReconciliationExecutionsCreated),
	}, "|"))
}
