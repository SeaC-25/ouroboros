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

type RuntimeWorkerOrchestrationSupervisorOptions struct {
	Execute                         bool
	Now                             time.Time
	RunIDPrefix                     string
	TransportMode                   string
	MaxCandidates                   int
	MaxRuns                         int
	LeaseDuration                   time.Duration
	TransportAuthToken              string
	ResultCallbackEndpoint          string
	IncludeLifecycleMaintenance     bool
	ExecuteLifecycleRetryDispatches bool
	DispatchLifecycleRetryClaims    bool
	LifecycleRetryTransportMode     string
}

type RuntimeWorkerOrchestrationSupervisorSummary struct {
	Runs                                  int `json:"runs"`
	MaxRuns                               int `json:"max_runs"`
	OrchestrationArtifactsCreated         int `json:"orchestration_artifacts_created"`
	LifecycleMaintenanceArtifactsCreated  int `json:"lifecycle_maintenance_artifacts_created"`
	LifecycleAuditArtifactsCreated        int `json:"lifecycle_audit_artifacts_created"`
	RetryDispatchExecutionsCreated        int `json:"retry_dispatch_executions_created,omitempty"`
	RetryDispatchClaimsCreated            int `json:"retry_dispatch_claims_created,omitempty"`
	RetryDispatchClaimsDispatched         int `json:"retry_dispatch_claims_dispatched,omitempty"`
	RetryDispatchTransportRequestsCreated int `json:"retry_dispatch_transport_requests_created,omitempty"`
	ClaimedCount                          int `json:"claimed_count"`
	DispatchedCount                       int `json:"dispatched_count"`
	SkippedCount                          int `json:"skipped_count"`
	FailedCount                           int `json:"failed_count"`
}

type RuntimeWorkerOrchestrationSupervisor struct {
	Version                         int                                         `json:"version"`
	Path                            string                                      `json:"path,omitempty"`
	Status                          string                                      `json:"status"`
	Mode                            string                                      `json:"mode"`
	CreatedAt                       string                                      `json:"created_at"`
	CompletedAt                     string                                      `json:"completed_at,omitempty"`
	RunIDPrefix                     string                                      `json:"run_id_prefix"`
	TransportMode                   string                                      `json:"transport_mode"`
	MaxCandidates                   int                                         `json:"max_candidates,omitempty"`
	IncludeLifecycleMaintenance     bool                                        `json:"include_lifecycle_maintenance,omitempty"`
	ExecuteLifecycleRetryDispatches bool                                        `json:"execute_lifecycle_retry_dispatches,omitempty"`
	DispatchLifecycleRetryClaims    bool                                        `json:"dispatch_lifecycle_retry_claims,omitempty"`
	LifecycleRetryTransportMode     string                                      `json:"lifecycle_retry_transport_mode,omitempty"`
	Summary                         RuntimeWorkerOrchestrationSupervisorSummary `json:"summary"`
	OrchestrationPaths              []string                                    `json:"orchestration_paths,omitempty"`
	RetryDispatchExecutionPaths     []string                                    `json:"retry_dispatch_execution_paths,omitempty"`
	Error                           string                                      `json:"error,omitempty"`
}

func CreateRuntimeWorkerOrchestrationSupervisor(root string, options RuntimeWorkerOrchestrationSupervisorOptions) (RuntimeWorkerOrchestrationSupervisor, error) {
	now := options.Now.UTC()
	if now.IsZero() {
		now = time.Now().UTC()
	}
	transportMode, err := normalizeRuntimeWorkerQueueDispatchTransport(options.TransportMode)
	if err != nil {
		return RuntimeWorkerOrchestrationSupervisor{}, err
	}
	lifecycleRetryTransportMode, err := normalizeWorkerLifecycleRetryTransportMode(options.LifecycleRetryTransportMode)
	if err != nil {
		return RuntimeWorkerOrchestrationSupervisor{}, err
	}
	if options.DispatchLifecycleRetryClaims && !options.Execute {
		return RuntimeWorkerOrchestrationSupervisor{}, fmt.Errorf("worker orchestration supervisor lifecycle retry handoff requires explicit execute")
	}
	if options.DispatchLifecycleRetryClaims && !options.IncludeLifecycleMaintenance {
		return RuntimeWorkerOrchestrationSupervisor{}, fmt.Errorf("worker orchestration supervisor lifecycle retry handoff requires lifecycle maintenance")
	}
	if options.DispatchLifecycleRetryClaims && !options.ExecuteLifecycleRetryDispatches {
		return RuntimeWorkerOrchestrationSupervisor{}, fmt.Errorf("worker orchestration supervisor lifecycle retry handoff requires lifecycle retry execution")
	}
	maxRuns := options.MaxRuns
	if maxRuns <= 0 {
		maxRuns = 1
	}
	runIDPrefix := strings.TrimSpace(options.RunIDPrefix)
	if runIDPrefix == "" {
		runIDPrefix = fmt.Sprintf("worker-orchestration-supervisor-run-%d", now.UnixNano())
	}
	supervisor := RuntimeWorkerOrchestrationSupervisor{
		Version:                         1,
		Status:                          "dry-run",
		Mode:                            "metadata-only",
		CreatedAt:                       now.Format(time.RFC3339Nano),
		RunIDPrefix:                     runIDPrefix,
		TransportMode:                   transportMode,
		MaxCandidates:                   options.MaxCandidates,
		IncludeLifecycleMaintenance:     options.IncludeLifecycleMaintenance,
		ExecuteLifecycleRetryDispatches: options.ExecuteLifecycleRetryDispatches,
		DispatchLifecycleRetryClaims:    options.DispatchLifecycleRetryClaims,
		LifecycleRetryTransportMode:     lifecycleRetryTransportMode,
		Summary: RuntimeWorkerOrchestrationSupervisorSummary{
			MaxRuns: maxRuns,
		},
	}
	if !options.Execute {
		if err := writeRuntimeWorkerOrchestrationSupervisor(root, &supervisor); err != nil {
			return supervisor, err
		}
		return supervisor, nil
	}

	supervisor.Status = "completed"
	for run := 0; run < maxRuns; run++ {
		runNow := now.Add(time.Duration(run) * time.Nanosecond)
		orchestration, err := CreateRuntimeWorkerOrchestration(root, RuntimeWorkerOrchestrationOptions{
			Execute:                         true,
			Now:                             runNow,
			RunID:                           fmt.Sprintf("%s-%02d", runIDPrefix, run+1),
			TransportMode:                   transportMode,
			MaxCandidates:                   options.MaxCandidates,
			LeaseDuration:                   options.LeaseDuration,
			TransportAuthToken:              options.TransportAuthToken,
			ResultCallbackEndpoint:          options.ResultCallbackEndpoint,
			IncludeLifecycleMaintenance:     options.IncludeLifecycleMaintenance,
			ExecuteLifecycleRetryDispatches: options.ExecuteLifecycleRetryDispatches,
			DispatchLifecycleRetryClaims:    options.DispatchLifecycleRetryClaims,
			LifecycleRetryTransportMode:     lifecycleRetryTransportMode,
		})
		if strings.TrimSpace(orchestration.Path) != "" {
			supervisor.OrchestrationPaths = append(supervisor.OrchestrationPaths, orchestration.Path)
			supervisor.Summary.OrchestrationArtifactsCreated++
		}
		supervisor.Summary.ClaimedCount += orchestration.ClaimedCount
		supervisor.Summary.DispatchedCount += orchestration.DispatchedCount
		supervisor.Summary.SkippedCount += orchestration.DispatchSkipped + orchestration.ClaimSkipped
		supervisor.Summary.FailedCount += orchestration.DispatchFailed + orchestration.ClaimFailed
		if strings.TrimSpace(orchestration.LifecycleMaintenancePath) != "" {
			supervisor.Summary.LifecycleMaintenanceArtifactsCreated++
		}
		if strings.TrimSpace(orchestration.LifecycleAuditPath) != "" {
			supervisor.Summary.LifecycleAuditArtifactsCreated++
		}
		supervisor.Summary.RetryDispatchExecutionsCreated += orchestration.RetryDispatchExecutionsCreated
		supervisor.Summary.RetryDispatchClaimsCreated += orchestration.RetryDispatchClaimsCreated
		supervisor.Summary.RetryDispatchClaimsDispatched += orchestration.RetryDispatchClaimsDispatched
		supervisor.Summary.RetryDispatchTransportRequestsCreated += orchestration.RetryDispatchTransportRequestsCreated
		supervisor.RetryDispatchExecutionPaths = append(supervisor.RetryDispatchExecutionPaths, orchestration.RetryDispatchExecutionPaths...)
		if err != nil {
			return failRuntimeWorkerOrchestrationSupervisor(root, supervisor, err)
		}
		supervisor.Summary.Runs++
	}
	supervisor.CompletedAt = now.Add(time.Duration(maxRuns) * time.Nanosecond).Format(time.RFC3339Nano)
	if err := writeRuntimeWorkerOrchestrationSupervisor(root, &supervisor); err != nil {
		return supervisor, err
	}
	return supervisor, nil
}

func ListRuntimeWorkerOrchestrationSupervisors(root string) ([]RuntimeWorkerOrchestrationSupervisor, error) {
	paths, err := filepath.Glob(filepath.Join(runtimeWorkerOrchestrationSupervisorDir(root), "*.json"))
	if err != nil {
		return nil, fmt.Errorf("glob worker orchestration supervisor artifacts: %w", err)
	}
	sort.Strings(paths)
	supervisors := make([]RuntimeWorkerOrchestrationSupervisor, 0, len(paths))
	for _, path := range paths {
		supervisor, err := readRuntimeWorkerOrchestrationSupervisorFile(path)
		if err != nil {
			return nil, err
		}
		supervisors = append(supervisors, supervisor)
	}
	sort.SliceStable(supervisors, func(i int, j int) bool {
		if supervisors[i].CreatedAt != supervisors[j].CreatedAt {
			return supervisors[i].CreatedAt < supervisors[j].CreatedAt
		}
		return supervisors[i].Path < supervisors[j].Path
	})
	return supervisors, nil
}

func ReadRuntimeWorkerOrchestrationSupervisor(root, supervisorPath string) (RuntimeWorkerOrchestrationSupervisor, error) {
	path, err := resolveRuntimeWorkerOrchestrationSupervisorPath(root, supervisorPath)
	if err != nil {
		return RuntimeWorkerOrchestrationSupervisor{}, err
	}
	return readRuntimeWorkerOrchestrationSupervisorFile(path)
}

func failRuntimeWorkerOrchestrationSupervisor(root string, supervisor RuntimeWorkerOrchestrationSupervisor, err error) (RuntimeWorkerOrchestrationSupervisor, error) {
	supervisor.Status = "failed"
	supervisor.Error = err.Error()
	if writeErr := writeRuntimeWorkerOrchestrationSupervisor(root, &supervisor); writeErr != nil {
		return supervisor, writeErr
	}
	return supervisor, err
}

func writeRuntimeWorkerOrchestrationSupervisor(root string, supervisor *RuntimeWorkerOrchestrationSupervisor) error {
	if err := os.MkdirAll(runtimeWorkerOrchestrationSupervisorDir(root), 0o755); err != nil {
		return fmt.Errorf("create worker orchestration supervisor directory: %w", err)
	}
	if strings.TrimSpace(supervisor.Path) == "" {
		supervisor.Path = filepath.Join(runtimeWorkerOrchestrationSupervisorDir(root), safeRuntimeWorkerOrchestrationSupervisorID(*supervisor)+".json")
	}
	data, err := json.MarshalIndent(supervisor, "", "  ")
	if err != nil {
		return fmt.Errorf("encode worker orchestration supervisor: %w", err)
	}
	data = append(data, '\n')
	tmp := supervisor.Path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("write worker orchestration supervisor: %w", err)
	}
	if err := os.Rename(tmp, supervisor.Path); err != nil {
		return fmt.Errorf("publish worker orchestration supervisor: %w", err)
	}
	return nil
}

func readRuntimeWorkerOrchestrationSupervisorFile(path string) (RuntimeWorkerOrchestrationSupervisor, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return RuntimeWorkerOrchestrationSupervisor{}, fmt.Errorf("read worker orchestration supervisor %s: %w", path, err)
	}
	var supervisor RuntimeWorkerOrchestrationSupervisor
	if err := json.Unmarshal(data, &supervisor); err != nil {
		return RuntimeWorkerOrchestrationSupervisor{}, fmt.Errorf("decode worker orchestration supervisor %s: %w", path, err)
	}
	supervisor.Path = path
	return supervisor, nil
}

func resolveRuntimeWorkerOrchestrationSupervisorPath(root, supervisorPath string) (string, error) {
	trimmed := strings.TrimSpace(supervisorPath)
	if trimmed == "" {
		return "", fmt.Errorf("worker orchestration supervisor path is required")
	}
	dir, err := filepath.Abs(runtimeWorkerOrchestrationSupervisorDir(root))
	if err != nil {
		return "", fmt.Errorf("resolve worker orchestration supervisor directory: %w", err)
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
		return "", fmt.Errorf("resolve worker orchestration supervisor path: %w", err)
	}
	rel, err := filepath.Rel(dir, candidate)
	if err != nil {
		return "", fmt.Errorf("resolve worker orchestration supervisor relative path: %w", err)
	}
	if rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("worker orchestration supervisor path must stay under %s", dir)
	}
	if filepath.Ext(candidate) != ".json" {
		return "", fmt.Errorf("worker orchestration supervisor path must be a json file")
	}
	return candidate, nil
}

func runtimeWorkerOrchestrationSupervisorDir(root string) string {
	return filepath.Join(root, "runtime", "worker-orchestration-supervisor-runs")
}

func safeRuntimeWorkerOrchestrationSupervisorID(supervisor RuntimeWorkerOrchestrationSupervisor) string {
	return safeInstallRequestName(strings.Join([]string{
		"worker-orchestration-supervisor",
		supervisor.RunIDPrefix,
		supervisor.Status,
		supervisor.Mode,
		supervisor.TransportMode,
		supervisor.CreatedAt,
		fmt.Sprintf("retry-handoff=%t", supervisor.DispatchLifecycleRetryClaims),
		supervisor.LifecycleRetryTransportMode,
		fmt.Sprintf("runs-%d", supervisor.Summary.Runs),
		fmt.Sprintf("max-%d", supervisor.Summary.MaxRuns),
		fmt.Sprintf("orchestrations-%d", supervisor.Summary.OrchestrationArtifactsCreated),
	}, "|"))
}
