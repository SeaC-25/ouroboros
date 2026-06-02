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

type RuntimeWorkerOrchestrationOptions struct {
	Execute                         bool
	Now                             time.Time
	RunID                           string
	TransportMode                   string
	MaxCandidates                   int
	LeaseDuration                   time.Duration
	TransportAuthToken              string
	ResultCallbackEndpoint          string
	IncludeLifecycleMaintenance     bool
	ExecuteLifecycleRetryDispatches bool
	DispatchLifecycleRetryClaims    bool
	LifecycleRetryTransportMode     string
}

type RuntimeWorkerOrchestration struct {
	Version                               int      `json:"version"`
	Path                                  string   `json:"path,omitempty"`
	Status                                string   `json:"status"`
	Mode                                  string   `json:"mode"`
	RunID                                 string   `json:"run_id"`
	TransportMode                         string   `json:"transport_mode"`
	CreatedAt                             string   `json:"created_at"`
	MaxCandidates                         int      `json:"max_candidates,omitempty"`
	PlacementReady                        int      `json:"placement_ready"`
	PlacementSkipped                      int      `json:"placement_skipped"`
	PlacementPlanPath                     string   `json:"placement_plan_path,omitempty"`
	ClaimedCount                          int      `json:"claimed_count"`
	ClaimSkipped                          int      `json:"claim_skipped"`
	ClaimFailed                           int      `json:"claim_failed"`
	ClaimExecutionPaths                   []string `json:"claim_execution_paths,omitempty"`
	PlacementDispatchPlanPath             string   `json:"placement_dispatch_plan_path,omitempty"`
	DispatchReady                         int      `json:"dispatch_ready"`
	DispatchSkipped                       int      `json:"dispatch_skipped"`
	PlacementDispatchBatchPath            string   `json:"placement_dispatch_batch_path,omitempty"`
	PlacementDispatchBatchState           string   `json:"placement_dispatch_batch_status,omitempty"`
	DispatchedCount                       int      `json:"dispatched_count"`
	DispatchFailed                        int      `json:"dispatch_failed"`
	IncludeLifecycleMaintenance           bool     `json:"include_lifecycle_maintenance,omitempty"`
	ExecuteLifecycleRetryDispatches       bool     `json:"execute_lifecycle_retry_dispatches,omitempty"`
	DispatchLifecycleRetryClaims          bool     `json:"dispatch_lifecycle_retry_claims,omitempty"`
	LifecycleRetryTransportMode           string   `json:"lifecycle_retry_transport_mode,omitempty"`
	LifecycleMaintenancePath              string   `json:"lifecycle_maintenance_path,omitempty"`
	LifecycleAuditPath                    string   `json:"lifecycle_audit_path,omitempty"`
	RetryDispatchExecutionsCreated        int      `json:"retry_dispatch_executions_created,omitempty"`
	RetryDispatchClaimsCreated            int      `json:"retry_dispatch_claims_created,omitempty"`
	RetryDispatchClaimsDispatched         int      `json:"retry_dispatch_claims_dispatched,omitempty"`
	RetryDispatchTransportRequestsCreated int      `json:"retry_dispatch_transport_requests_created,omitempty"`
	RetryDispatchExecutionPaths           []string `json:"retry_dispatch_execution_paths,omitempty"`
	Error                                 string   `json:"error,omitempty"`
}

func CreateRuntimeWorkerOrchestration(root string, options RuntimeWorkerOrchestrationOptions) (RuntimeWorkerOrchestration, error) {
	now := options.Now.UTC()
	if now.IsZero() {
		now = time.Now().UTC()
	}
	transportMode, err := normalizeRuntimeWorkerQueueDispatchTransport(options.TransportMode)
	if err != nil {
		return RuntimeWorkerOrchestration{}, err
	}
	lifecycleRetryTransportMode, err := normalizeWorkerLifecycleRetryTransportMode(options.LifecycleRetryTransportMode)
	if err != nil {
		return RuntimeWorkerOrchestration{}, err
	}
	if options.DispatchLifecycleRetryClaims && !options.Execute {
		return RuntimeWorkerOrchestration{}, fmt.Errorf("worker orchestration lifecycle retry handoff requires explicit execute")
	}
	if options.DispatchLifecycleRetryClaims && !options.IncludeLifecycleMaintenance {
		return RuntimeWorkerOrchestration{}, fmt.Errorf("worker orchestration lifecycle retry handoff requires lifecycle maintenance")
	}
	if options.DispatchLifecycleRetryClaims && !options.ExecuteLifecycleRetryDispatches {
		return RuntimeWorkerOrchestration{}, fmt.Errorf("worker orchestration lifecycle retry handoff requires lifecycle retry execution")
	}
	runID := strings.TrimSpace(options.RunID)
	if runID == "" {
		runID = fmt.Sprintf("worker-orchestration-run-%d", now.UnixNano())
	}
	maxCandidates := options.MaxCandidates

	placementPreview, err := PlanRuntimeWorkerPlacements(root, RuntimeWorkerPlacementPlanOptions{
		Now:   now,
		RunID: runID,
	})
	if err != nil {
		return RuntimeWorkerOrchestration{}, err
	}
	orchestration := RuntimeWorkerOrchestration{
		Version:                         1,
		Status:                          "dry-run",
		Mode:                            "metadata-only",
		RunID:                           runID,
		TransportMode:                   transportMode,
		CreatedAt:                       now.Format(time.RFC3339Nano),
		MaxCandidates:                   maxCandidates,
		IncludeLifecycleMaintenance:     options.IncludeLifecycleMaintenance,
		ExecuteLifecycleRetryDispatches: options.ExecuteLifecycleRetryDispatches,
		DispatchLifecycleRetryClaims:    options.DispatchLifecycleRetryClaims,
		LifecycleRetryTransportMode:     lifecycleRetryTransportMode,
		PlacementReady:                  placementPreview.ReadyCount,
		PlacementSkipped:                placementPreview.SkippedCount,
	}
	if !options.Execute {
		if err := writeRuntimeWorkerOrchestration(root, &orchestration); err != nil {
			return orchestration, err
		}
		return orchestration, nil
	}

	orchestration.Status = "completed"
	orchestration.Mode = "execute"
	placementPlan, err := CreateRuntimeWorkerPlacementPlan(root, RuntimeWorkerPlacementPlanOptions{
		Now:   now,
		RunID: runID,
	})
	if err != nil {
		orchestration.Status = "failed"
		orchestration.Error = err.Error()
		if writeErr := writeRuntimeWorkerOrchestration(root, &orchestration); writeErr != nil {
			return orchestration, writeErr
		}
		return orchestration, err
	}
	orchestration.PlacementPlanPath = placementPlan.Path
	orchestration.PlacementReady = placementPlan.ReadyCount
	orchestration.PlacementSkipped = placementPlan.SkippedCount

	if err := claimRuntimeWorkerOrchestrationCandidates(root, placementPlan, options, now, &orchestration); err != nil {
		orchestration.Status = "failed"
		orchestration.Error = err.Error()
		if writeErr := writeRuntimeWorkerOrchestration(root, &orchestration); writeErr != nil {
			return orchestration, writeErr
		}
		return orchestration, err
	}
	if orchestration.ClaimedCount == 0 {
		orchestration.Status = "skipped"
		if err := writeRuntimeWorkerOrchestration(root, &orchestration); err != nil {
			return orchestration, err
		}
		return orchestration, nil
	}

	dispatchPlan, err := CreateRuntimeWorkerPlacementDispatchPlan(root, RuntimeWorkerPlacementDispatchPlanOptions{
		Now:           now,
		TransportMode: transportMode,
	})
	if err != nil {
		orchestration.Status = "failed"
		orchestration.Error = err.Error()
		if writeErr := writeRuntimeWorkerOrchestration(root, &orchestration); writeErr != nil {
			return orchestration, writeErr
		}
		return orchestration, err
	}
	orchestration.PlacementDispatchPlanPath = dispatchPlan.Path
	orchestration.DispatchReady = dispatchPlan.ReadyCount
	orchestration.DispatchSkipped = dispatchPlan.SkippedCount

	batch, err := ExecuteRuntimeWorkerPlacementDispatchBatch(root, dispatchPlan.Path, RuntimeWorkerPlacementDispatchBatchOptions{
		Execute:                true,
		Now:                    now,
		MaxCandidates:          maxCandidates,
		TransportAuthToken:     options.TransportAuthToken,
		ResultCallbackEndpoint: options.ResultCallbackEndpoint,
	})
	orchestration.PlacementDispatchBatchPath = batch.Path
	orchestration.PlacementDispatchBatchState = batch.Status
	orchestration.DispatchedCount = batch.DispatchedCount
	orchestration.DispatchSkipped = batch.SkippedCount
	orchestration.DispatchFailed = batch.FailedCount
	if err != nil {
		orchestration.Status = "failed"
		orchestration.Error = err.Error()
		if writeErr := writeRuntimeWorkerOrchestration(root, &orchestration); writeErr != nil {
			return orchestration, writeErr
		}
		return orchestration, err
	}
	if batch.Status == "skipped" && orchestration.DispatchedCount == 0 {
		orchestration.Status = "skipped"
	}
	if options.IncludeLifecycleMaintenance {
		if err := writeRuntimeWorkerOrchestration(root, &orchestration); err != nil {
			return orchestration, err
		}
		maintenance, err := CreateWorkerLifecycleMaintenance(root, WorkerLifecycleMaintenanceOptions{
			Execute:                    true,
			ExecuteRetryDispatches:     options.ExecuteLifecycleRetryDispatches,
			DispatchRetryClaims:        options.DispatchLifecycleRetryClaims,
			RetryDispatchTransportMode: lifecycleRetryTransportMode,
			Now:                        now,
			StaleAfter:                 DefaultWorkerLeaseStaleAfter,
		})
		orchestration.LifecycleMaintenancePath = maintenance.Path
		orchestration.RetryDispatchExecutionsCreated = maintenance.Summary.RetryDispatchExecutionsCreated
		orchestration.RetryDispatchClaimsCreated = maintenance.Summary.RetryDispatchClaimsCreated
		orchestration.RetryDispatchClaimsDispatched = maintenance.Summary.RetryDispatchClaimsDispatched
		orchestration.RetryDispatchTransportRequestsCreated = maintenance.Summary.RetryDispatchTransportRequestsCreated
		orchestration.RetryDispatchExecutionPaths = append(orchestration.RetryDispatchExecutionPaths, maintenance.RetryDispatchExecutionPaths...)
		if err != nil {
			orchestration.Status = "failed"
			orchestration.Error = err.Error()
			if writeErr := writeRuntimeWorkerOrchestration(root, &orchestration); writeErr != nil {
				return orchestration, writeErr
			}
			return orchestration, err
		}
		audit, err := CreateWorkerLifecycleAudit(root, now, DefaultWorkerLeaseStaleAfter)
		orchestration.LifecycleAuditPath = audit.Path
		if err != nil {
			orchestration.Status = "failed"
			orchestration.Error = err.Error()
			if writeErr := writeRuntimeWorkerOrchestration(root, &orchestration); writeErr != nil {
				return orchestration, writeErr
			}
			return orchestration, err
		}
	}
	if err := writeRuntimeWorkerOrchestration(root, &orchestration); err != nil {
		return orchestration, err
	}
	return orchestration, nil
}

func ListRuntimeWorkerOrchestrations(root string) ([]RuntimeWorkerOrchestration, error) {
	paths, err := filepath.Glob(filepath.Join(runtimeWorkerOrchestrationDir(root), "*.json"))
	if err != nil {
		return nil, fmt.Errorf("glob worker orchestrations: %w", err)
	}
	sort.Strings(paths)
	orchestrations := make([]RuntimeWorkerOrchestration, 0, len(paths))
	for _, path := range paths {
		orchestration, err := readRuntimeWorkerOrchestrationFile(path)
		if err != nil {
			return nil, err
		}
		orchestrations = append(orchestrations, orchestration)
	}
	sort.SliceStable(orchestrations, func(i int, j int) bool {
		if orchestrations[i].CreatedAt != orchestrations[j].CreatedAt {
			return orchestrations[i].CreatedAt < orchestrations[j].CreatedAt
		}
		if orchestrations[i].RunID != orchestrations[j].RunID {
			return orchestrations[i].RunID < orchestrations[j].RunID
		}
		return orchestrations[i].Path < orchestrations[j].Path
	})
	return orchestrations, nil
}

func ReadRuntimeWorkerOrchestration(root, orchestrationPath string) (RuntimeWorkerOrchestration, error) {
	path, err := resolveRuntimeWorkerOrchestrationPath(root, orchestrationPath)
	if err != nil {
		return RuntimeWorkerOrchestration{}, err
	}
	return readRuntimeWorkerOrchestrationFile(path)
}

func claimRuntimeWorkerOrchestrationCandidates(root string, plan RuntimeWorkerPlacementPlan, options RuntimeWorkerOrchestrationOptions, now time.Time, orchestration *RuntimeWorkerOrchestration) error {
	processed := 0
	for _, candidate := range plan.Candidates {
		if strings.TrimSpace(candidate.Status) != "ready" {
			continue
		}
		if options.MaxCandidates > 0 && processed >= options.MaxCandidates {
			break
		}
		processed++
		execution, err := ExecuteRuntimeWorkerPlacementClaim(root, plan.Path, RuntimeWorkerPlacementClaimExecutionOptions{
			Execute:              true,
			Now:                  now,
			CandidateExecutionID: candidate.ExecutionID,
			WorkerID:             candidate.WorkerID,
			LeaseDuration:        options.LeaseDuration,
		})
		if strings.TrimSpace(execution.Path) != "" {
			orchestration.ClaimExecutionPaths = append(orchestration.ClaimExecutionPaths, execution.Path)
		}
		switch strings.TrimSpace(execution.Status) {
		case "claimed":
			orchestration.ClaimedCount++
		case "skipped":
			orchestration.ClaimSkipped++
		case "failed":
			orchestration.ClaimFailed++
		}
		if err != nil {
			if strings.TrimSpace(execution.Status) != "failed" {
				orchestration.ClaimFailed++
			}
			return err
		}
	}
	return nil
}

func writeRuntimeWorkerOrchestration(root string, orchestration *RuntimeWorkerOrchestration) error {
	if err := os.MkdirAll(runtimeWorkerOrchestrationDir(root), 0o755); err != nil {
		return fmt.Errorf("create worker orchestration directory: %w", err)
	}
	if strings.TrimSpace(orchestration.Path) == "" {
		orchestration.Path = filepath.Join(runtimeWorkerOrchestrationDir(root), safeRuntimeWorkerOrchestrationID(*orchestration)+".json")
	}
	data, err := json.MarshalIndent(orchestration, "", "  ")
	if err != nil {
		return fmt.Errorf("encode worker orchestration: %w", err)
	}
	data = append(data, '\n')
	tmp := orchestration.Path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("write worker orchestration: %w", err)
	}
	if err := os.Rename(tmp, orchestration.Path); err != nil {
		return fmt.Errorf("publish worker orchestration: %w", err)
	}
	return nil
}

func readRuntimeWorkerOrchestrationFile(path string) (RuntimeWorkerOrchestration, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return RuntimeWorkerOrchestration{}, fmt.Errorf("read worker orchestration %s: %w", path, err)
	}
	var orchestration RuntimeWorkerOrchestration
	if err := json.Unmarshal(data, &orchestration); err != nil {
		return RuntimeWorkerOrchestration{}, fmt.Errorf("decode worker orchestration %s: %w", path, err)
	}
	orchestration.Path = path
	return orchestration, nil
}

func resolveRuntimeWorkerOrchestrationPath(root, orchestrationPath string) (string, error) {
	trimmed := strings.TrimSpace(orchestrationPath)
	if trimmed == "" {
		return "", fmt.Errorf("worker orchestration path is required")
	}
	dir, err := filepath.Abs(runtimeWorkerOrchestrationDir(root))
	if err != nil {
		return "", fmt.Errorf("resolve worker orchestration directory: %w", err)
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
		return "", fmt.Errorf("resolve worker orchestration path: %w", err)
	}
	rel, err := filepath.Rel(dir, candidate)
	if err != nil {
		return "", fmt.Errorf("resolve worker orchestration relative path: %w", err)
	}
	if rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("worker orchestration path must stay under %s", dir)
	}
	if filepath.Ext(candidate) != ".json" {
		return "", fmt.Errorf("worker orchestration path must be a json file")
	}
	return candidate, nil
}

func runtimeWorkerOrchestrationDir(root string) string {
	return filepath.Join(root, "runtime", "worker-orchestrations")
}

func safeRuntimeWorkerOrchestrationID(orchestration RuntimeWorkerOrchestration) string {
	return safeInstallRequestName(strings.Join([]string{
		"worker-orchestration",
		orchestration.RunID,
		orchestration.Mode,
		orchestration.Status,
		orchestration.TransportMode,
		orchestration.CreatedAt,
		fmt.Sprintf("retry-handoff=%t", orchestration.DispatchLifecycleRetryClaims),
		orchestration.LifecycleRetryTransportMode,
		fmt.Sprintf("placement-ready-%d", orchestration.PlacementReady),
		fmt.Sprintf("claimed-%d", orchestration.ClaimedCount),
		fmt.Sprintf("dispatched-%d", orchestration.DispatchedCount),
	}, "|"))
}
