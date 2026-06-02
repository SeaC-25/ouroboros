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

type WorkerLifecyclePolicyOptions struct {
	Now                          time.Time
	StaleAfter                   time.Duration
	WorkerStaleAfter             time.Duration
	PollInterval                 time.Duration
	MaxSupervisorRuns            int
	TransportMode                string
	IncludeLifecycleMaintenance  bool
	ExecuteLifecycleRetries      bool
	DispatchLifecycleRetryClaims bool
	LifecycleRetryTransportMode  string
	ExecuteLeaseRecovery         bool
}

type WorkerLifecyclePolicy struct {
	Version                      int                         `json:"version"`
	Path                         string                      `json:"path,omitempty"`
	Status                       string                      `json:"status"`
	Mode                         string                      `json:"mode"`
	CreatedAt                    string                      `json:"created_at"`
	StaleAfterSeconds            int64                       `json:"stale_after_seconds"`
	WorkerStaleAfterSeconds      int64                       `json:"worker_stale_after_seconds"`
	PollIntervalSeconds          int64                       `json:"poll_interval_seconds"`
	PollIntervalMilliseconds     int64                       `json:"poll_interval_milliseconds"`
	MaxSupervisorRuns            int                         `json:"max_supervisor_runs"`
	TransportMode                string                      `json:"transport_mode"`
	IncludeLifecycleMaintenance  bool                        `json:"include_lifecycle_maintenance"`
	ExecuteLifecycleRetries      bool                        `json:"execute_lifecycle_retries"`
	DispatchLifecycleRetryClaims bool                        `json:"dispatch_lifecycle_retry_claims,omitempty"`
	LifecycleRetryTransportMode  string                      `json:"lifecycle_retry_transport_mode,omitempty"`
	ExecuteLeaseRecovery         bool                        `json:"execute_lease_recovery"`
	Summary                      WorkerLifecycleAuditSummary `json:"summary"`
}

type WorkerLifecyclePolicySelection struct {
	Version                      int                         `json:"version"`
	Path                         string                      `json:"path,omitempty"`
	Status                       string                      `json:"status"`
	Mode                         string                      `json:"mode"`
	PolicyPath                   string                      `json:"policy_path,omitempty"`
	PolicyBasename               string                      `json:"policy_basename,omitempty"`
	StaleAfterSeconds            int64                       `json:"stale_after_seconds,omitempty"`
	WorkerStaleAfterSeconds      int64                       `json:"worker_stale_after_seconds,omitempty"`
	PollIntervalSeconds          int64                       `json:"poll_interval_seconds,omitempty"`
	PollIntervalMilliseconds     int64                       `json:"poll_interval_milliseconds,omitempty"`
	MaxSupervisorRuns            int                         `json:"max_supervisor_runs,omitempty"`
	TransportMode                string                      `json:"transport_mode,omitempty"`
	IncludeLifecycleMaintenance  bool                        `json:"include_lifecycle_maintenance,omitempty"`
	ExecuteLifecycleRetries      bool                        `json:"execute_lifecycle_retries,omitempty"`
	DispatchLifecycleRetryClaims bool                        `json:"dispatch_lifecycle_retry_claims,omitempty"`
	LifecycleRetryTransportMode  string                      `json:"lifecycle_retry_transport_mode,omitempty"`
	ExecuteLeaseRecovery         bool                        `json:"execute_lease_recovery,omitempty"`
	Summary                      WorkerLifecycleAuditSummary `json:"summary,omitempty"`
	SupersededPolicyPaths        []string                    `json:"superseded_policy_paths,omitempty"`
	SelectedBy                   string                      `json:"selected_by,omitempty"`
	SelectedAt                   string                      `json:"selected_at,omitempty"`
	ClearedBy                    string                      `json:"cleared_by,omitempty"`
	ClearedAt                    string                      `json:"cleared_at,omitempty"`
	Reason                       string                      `json:"reason,omitempty"`
}

type WorkerLifecyclePolicySelectionOptions struct {
	SelectedBy string
	SelectedAt time.Time
	Reason     string
}

type WorkerLifecyclePolicySelectionClearOptions struct {
	ClearedBy string
	ClearedAt time.Time
	Reason    string
}

func PreviewWorkerLifecyclePolicy(root string, options WorkerLifecyclePolicyOptions) (WorkerLifecyclePolicy, error) {
	normalized, err := normalizeWorkerLifecyclePolicyOptions(options)
	if err != nil {
		return WorkerLifecyclePolicy{}, err
	}
	audit, err := PlanWorkerLifecycleAudit(root, normalized.now, normalized.staleAfter)
	if err != nil {
		return WorkerLifecyclePolicy{}, err
	}
	if err := applyWorkerLifecyclePolicyWorkerStaleness(root, normalized.now, normalized.workerStaleAfter, &audit.Summary); err != nil {
		return WorkerLifecyclePolicy{}, err
	}
	return WorkerLifecyclePolicy{
		Version:                      1,
		Status:                       "dry-run",
		Mode:                         "metadata-only",
		CreatedAt:                    normalized.now.Format(time.RFC3339Nano),
		StaleAfterSeconds:            int64(normalized.staleAfter / time.Second),
		WorkerStaleAfterSeconds:      int64(normalized.workerStaleAfter / time.Second),
		PollIntervalSeconds:          int64(normalized.pollInterval / time.Second),
		PollIntervalMilliseconds:     int64(normalized.pollInterval / time.Millisecond),
		MaxSupervisorRuns:            normalized.maxSupervisorRuns,
		TransportMode:                normalized.transportMode,
		IncludeLifecycleMaintenance:  options.IncludeLifecycleMaintenance,
		ExecuteLifecycleRetries:      options.ExecuteLifecycleRetries,
		DispatchLifecycleRetryClaims: options.DispatchLifecycleRetryClaims,
		LifecycleRetryTransportMode:  normalized.lifecycleRetryTransportMode,
		ExecuteLeaseRecovery:         options.ExecuteLeaseRecovery,
		Summary:                      audit.Summary,
	}, nil
}

func CreateWorkerLifecyclePolicy(root string, options WorkerLifecyclePolicyOptions) (WorkerLifecyclePolicy, error) {
	policy, err := PreviewWorkerLifecyclePolicy(root, options)
	if err != nil {
		return WorkerLifecyclePolicy{}, err
	}
	if err := writeWorkerLifecyclePolicy(root, &policy); err != nil {
		return policy, err
	}
	return policy, nil
}

func ListWorkerLifecyclePolicies(root string) ([]WorkerLifecyclePolicy, error) {
	paths, err := filepath.Glob(filepath.Join(workerLifecyclePolicyDir(root), "*.json"))
	if err != nil {
		return nil, fmt.Errorf("glob worker lifecycle policies: %w", err)
	}
	sort.Strings(paths)
	policies := make([]WorkerLifecyclePolicy, 0, len(paths))
	for _, path := range paths {
		policy, err := readWorkerLifecyclePolicyFile(path)
		if err != nil {
			return nil, err
		}
		policies = append(policies, policy)
	}
	return policies, nil
}

func ReadWorkerLifecyclePolicy(root, policyPath string) (WorkerLifecyclePolicy, error) {
	path, err := resolveWorkerLifecyclePolicyPath(root, policyPath)
	if err != nil {
		return WorkerLifecyclePolicy{}, err
	}
	return readWorkerLifecyclePolicyFile(path)
}

func SelectWorkerLifecyclePolicy(root, policyPath string, options WorkerLifecyclePolicySelectionOptions) (WorkerLifecyclePolicySelection, error) {
	policy, err := ReadWorkerLifecyclePolicy(root, policyPath)
	if err != nil {
		return WorkerLifecyclePolicySelection{}, err
	}
	current, found, err := ReadWorkerLifecyclePolicySelection(root)
	if err != nil {
		return WorkerLifecyclePolicySelection{}, err
	}
	selectedBy := strings.TrimSpace(options.SelectedBy)
	if selectedBy == "" {
		selectedBy = "operator"
	}
	selectedAt := options.SelectedAt
	if selectedAt.IsZero() {
		selectedAt = time.Now().UTC()
	}
	superseded := []string{}
	if found && workerLifecyclePolicySelectionIsCurrent(current) {
		currentPath := strings.TrimSpace(current.PolicyPath)
		if currentPath != "" && currentPath != policy.Path {
			superseded = append(superseded, currentPath)
		}
	}
	selection := workerLifecyclePolicySelectionFromPolicy(root, policy)
	selection.Status = "current"
	selection.SelectedBy = selectedBy
	selection.SelectedAt = selectedAt.UTC().Format(time.RFC3339)
	selection.ClearedBy = ""
	selection.ClearedAt = ""
	selection.Reason = strings.TrimSpace(options.Reason)
	selection.SupersededPolicyPaths = superseded
	if err := writeWorkerLifecyclePolicySelection(selection); err != nil {
		return WorkerLifecyclePolicySelection{}, err
	}
	return selection, nil
}

func ClearWorkerLifecyclePolicySelection(root string, options WorkerLifecyclePolicySelectionClearOptions) (WorkerLifecyclePolicySelection, error) {
	selection, found, err := ReadWorkerLifecyclePolicySelection(root)
	if err != nil {
		return WorkerLifecyclePolicySelection{}, err
	}
	if !found {
		selection = WorkerLifecyclePolicySelection{
			Version: 1,
			Path:    workerLifecyclePolicySelectionPath(root),
			Status:  "cleared",
			Mode:    "metadata-only",
		}
	}
	clearedBy := strings.TrimSpace(options.ClearedBy)
	if clearedBy == "" {
		clearedBy = "operator"
	}
	clearedAt := options.ClearedAt
	if clearedAt.IsZero() {
		clearedAt = time.Now().UTC()
	}
	selection.Version = 1
	selection.Path = workerLifecyclePolicySelectionPath(root)
	selection.Status = "cleared"
	selection.Mode = "metadata-only"
	selection.ClearedBy = clearedBy
	selection.ClearedAt = clearedAt.UTC().Format(time.RFC3339)
	selection.Reason = strings.TrimSpace(options.Reason)
	if err := writeWorkerLifecyclePolicySelection(selection); err != nil {
		return WorkerLifecyclePolicySelection{}, err
	}
	return selection, nil
}

func ReadWorkerLifecyclePolicySelection(root string) (WorkerLifecyclePolicySelection, bool, error) {
	path := workerLifecyclePolicySelectionPath(root)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return WorkerLifecyclePolicySelection{}, false, nil
		}
		return WorkerLifecyclePolicySelection{}, false, fmt.Errorf("read worker lifecycle policy selection %s: %w", path, err)
	}
	var selection WorkerLifecyclePolicySelection
	if err := json.Unmarshal(data, &selection); err != nil {
		return WorkerLifecyclePolicySelection{}, false, fmt.Errorf("decode worker lifecycle policy selection %s: %w", path, err)
	}
	selection.Path = path
	return selection, true, nil
}

func workerLifecyclePolicySelectionFromPolicy(root string, policy WorkerLifecyclePolicy) WorkerLifecyclePolicySelection {
	return WorkerLifecyclePolicySelection{
		Version:                      1,
		Path:                         workerLifecyclePolicySelectionPath(root),
		Mode:                         "metadata-only",
		PolicyPath:                   policy.Path,
		PolicyBasename:               filepath.Base(policy.Path),
		StaleAfterSeconds:            policy.StaleAfterSeconds,
		WorkerStaleAfterSeconds:      policy.WorkerStaleAfterSeconds,
		PollIntervalSeconds:          policy.PollIntervalSeconds,
		PollIntervalMilliseconds:     policy.PollIntervalMilliseconds,
		MaxSupervisorRuns:            policy.MaxSupervisorRuns,
		TransportMode:                policy.TransportMode,
		IncludeLifecycleMaintenance:  policy.IncludeLifecycleMaintenance,
		ExecuteLifecycleRetries:      policy.ExecuteLifecycleRetries,
		DispatchLifecycleRetryClaims: policy.DispatchLifecycleRetryClaims,
		LifecycleRetryTransportMode:  policy.LifecycleRetryTransportMode,
		ExecuteLeaseRecovery:         policy.ExecuteLeaseRecovery,
		Summary:                      policy.Summary,
	}
}

func writeWorkerLifecyclePolicySelection(selection WorkerLifecyclePolicySelection) error {
	if err := os.MkdirAll(filepath.Dir(selection.Path), 0o755); err != nil {
		return fmt.Errorf("create worker lifecycle policy selection directory: %w", err)
	}
	data, err := json.MarshalIndent(selection, "", "  ")
	if err != nil {
		return fmt.Errorf("encode worker lifecycle policy selection: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(selection.Path, data, 0o644); err != nil {
		return fmt.Errorf("write worker lifecycle policy selection: %w", err)
	}
	return nil
}

func workerLifecyclePolicySelectionIsCurrent(selection WorkerLifecyclePolicySelection) bool {
	return strings.TrimSpace(selection.Status) == "current"
}

func workerLifecyclePolicySelectionIsCleared(selection WorkerLifecyclePolicySelection) bool {
	return strings.TrimSpace(selection.Status) == "cleared"
}

type normalizedWorkerLifecyclePolicyOptions struct {
	now                         time.Time
	staleAfter                  time.Duration
	workerStaleAfter            time.Duration
	pollInterval                time.Duration
	maxSupervisorRuns           int
	transportMode               string
	lifecycleRetryTransportMode string
}

func normalizeWorkerLifecyclePolicyOptions(options WorkerLifecyclePolicyOptions) (normalizedWorkerLifecyclePolicyOptions, error) {
	now := options.Now.UTC()
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if options.PollInterval < 0 {
		return normalizedWorkerLifecyclePolicyOptions{}, fmt.Errorf("worker lifecycle policy poll interval cannot be negative")
	}
	if options.ExecuteLifecycleRetries && !options.IncludeLifecycleMaintenance {
		return normalizedWorkerLifecyclePolicyOptions{}, fmt.Errorf("worker lifecycle policy retry execution requires lifecycle maintenance")
	}
	if options.DispatchLifecycleRetryClaims && !options.IncludeLifecycleMaintenance {
		return normalizedWorkerLifecyclePolicyOptions{}, fmt.Errorf("worker lifecycle policy retry handoff requires lifecycle maintenance")
	}
	if options.DispatchLifecycleRetryClaims && !options.ExecuteLifecycleRetries {
		return normalizedWorkerLifecyclePolicyOptions{}, fmt.Errorf("worker lifecycle policy retry handoff requires lifecycle retry execution")
	}
	if options.ExecuteLeaseRecovery && !options.IncludeLifecycleMaintenance {
		return normalizedWorkerLifecyclePolicyOptions{}, fmt.Errorf("worker lifecycle policy lease recovery execution requires lifecycle maintenance")
	}
	transportMode, err := normalizeRuntimeWorkerQueueDispatchTransport(options.TransportMode)
	if err != nil {
		return normalizedWorkerLifecyclePolicyOptions{}, err
	}
	lifecycleRetryTransportMode, err := normalizeWorkerLifecycleRetryTransportMode(options.LifecycleRetryTransportMode)
	if err != nil {
		return normalizedWorkerLifecyclePolicyOptions{}, err
	}
	staleAfter := options.StaleAfter
	if staleAfter <= 0 {
		return normalizedWorkerLifecyclePolicyOptions{}, fmt.Errorf("worker lifecycle policy stale after must be positive")
	}
	workerStaleAfter := options.WorkerStaleAfter
	if workerStaleAfter <= 0 {
		return normalizedWorkerLifecyclePolicyOptions{}, fmt.Errorf("worker lifecycle policy worker stale after must be positive")
	}
	maxSupervisorRuns := options.MaxSupervisorRuns
	if maxSupervisorRuns <= 0 {
		return normalizedWorkerLifecyclePolicyOptions{}, fmt.Errorf("worker lifecycle policy max supervisor runs must be positive")
	}
	return normalizedWorkerLifecyclePolicyOptions{
		now:                         now,
		staleAfter:                  staleAfter,
		workerStaleAfter:            workerStaleAfter,
		pollInterval:                options.PollInterval,
		maxSupervisorRuns:           maxSupervisorRuns,
		transportMode:               transportMode,
		lifecycleRetryTransportMode: lifecycleRetryTransportMode,
	}, nil
}

func applyWorkerLifecyclePolicyWorkerStaleness(root string, now time.Time, workerStaleAfter time.Duration, summary *WorkerLifecycleAuditSummary) error {
	workers, err := ListRuntimeWorkersWithStaleness(root, now, workerStaleAfter)
	if err != nil {
		return err
	}
	summary.WorkerRegistrations = len(workers)
	summary.StaleWorkerRegistrations = 0
	summary.OfflineWorkerRegistrations = 0
	for _, worker := range workers {
		if worker.Stale {
			summary.StaleWorkerRegistrations++
		}
		if strings.TrimSpace(worker.Status) == "offline" {
			summary.OfflineWorkerRegistrations++
		}
	}
	return nil
}

func writeWorkerLifecyclePolicy(root string, policy *WorkerLifecyclePolicy) error {
	if err := os.MkdirAll(workerLifecyclePolicyDir(root), 0o755); err != nil {
		return fmt.Errorf("create worker lifecycle policy directory: %w", err)
	}
	if strings.TrimSpace(policy.Path) == "" {
		policy.Path = filepath.Join(workerLifecyclePolicyDir(root), safeWorkerLifecyclePolicyName(*policy)+".json")
	}
	data, err := json.MarshalIndent(policy, "", "  ")
	if err != nil {
		return fmt.Errorf("encode worker lifecycle policy: %w", err)
	}
	data = append(data, '\n')
	tmp := policy.Path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("write worker lifecycle policy: %w", err)
	}
	if err := os.Rename(tmp, policy.Path); err != nil {
		return fmt.Errorf("publish worker lifecycle policy: %w", err)
	}
	return nil
}

func readWorkerLifecyclePolicyFile(path string) (WorkerLifecyclePolicy, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return WorkerLifecyclePolicy{}, fmt.Errorf("read worker lifecycle policy %s: %w", path, err)
	}
	var policy WorkerLifecyclePolicy
	if err := json.Unmarshal(data, &policy); err != nil {
		return WorkerLifecyclePolicy{}, fmt.Errorf("decode worker lifecycle policy %s: %w", path, err)
	}
	policy.Path = path
	return policy, nil
}

func resolveWorkerLifecyclePolicyPath(root, policyPath string) (string, error) {
	trimmed := strings.TrimSpace(policyPath)
	if trimmed == "" {
		return "", fmt.Errorf("worker lifecycle policy path is required")
	}
	dir, err := filepath.Abs(workerLifecyclePolicyDir(root))
	if err != nil {
		return "", fmt.Errorf("resolve worker lifecycle policy directory: %w", err)
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
		return "", fmt.Errorf("resolve worker lifecycle policy path: %w", err)
	}
	rel, err := filepath.Rel(dir, candidate)
	if err != nil {
		return "", fmt.Errorf("resolve worker lifecycle policy relative path: %w", err)
	}
	if rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("worker lifecycle policy path must stay under %s", dir)
	}
	if filepath.Ext(candidate) != ".json" {
		return "", fmt.Errorf("worker lifecycle policy path must be a json file")
	}
	return candidate, nil
}

func workerLifecyclePolicyDir(root string) string {
	return filepath.Join(root, "runtime", "worker-lifecycle-policies")
}

func workerLifecyclePolicySelectionPath(root string) string {
	return filepath.Join(root, "runtime", "worker-lifecycle-policy-selection.json")
}

func safeWorkerLifecyclePolicyName(policy WorkerLifecyclePolicy) string {
	summary := policy.Summary
	return safeInstallRequestName(strings.Join([]string{
		"worker-lifecycle-policy",
		policy.CreatedAt,
		policy.Status,
		policy.Mode,
		fmt.Sprintf("stale=%d", policy.StaleAfterSeconds),
		fmt.Sprintf("worker-stale=%d", policy.WorkerStaleAfterSeconds),
		fmt.Sprintf("poll-ms=%d", policy.PollIntervalMilliseconds),
		fmt.Sprintf("max-runs=%d", policy.MaxSupervisorRuns),
		policy.TransportMode,
		fmt.Sprintf("maintenance=%t", policy.IncludeLifecycleMaintenance),
		fmt.Sprintf("retries=%t", policy.ExecuteLifecycleRetries),
		fmt.Sprintf("retry-handoff=%t", policy.DispatchLifecycleRetryClaims),
		policy.LifecycleRetryTransportMode,
		fmt.Sprintf("lease-recovery=%t", policy.ExecuteLeaseRecovery),
		fmt.Sprintf("workers=%d", summary.WorkerRegistrations),
		fmt.Sprintf("leases=%d", summary.WorkerLeases),
		fmt.Sprintf("artifacts=%d", summary.ArtifactTotal),
	}, "|"))
}
