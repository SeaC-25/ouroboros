package api

import (
	"net/http"
	"path/filepath"
	"time"

	"github.com/ouroboros/ouroboros/internal/app"
)

// handleRuntimeWorkers projects the persisted worker registry for the runtime workbench.
func (s *Server) handleRuntimeWorkers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		respondError(w, http.StatusMethodNotAllowed, "不支持的运行时 worker 请求方法")
		return
	}

	now := time.Now().UTC()
	workers, err := app.ListRuntimeWorkersWithStaleness(s.workspace, now, app.DefaultRuntimeWorkerStaleAfter)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "读取运行时 worker 注册表失败")
		return
	}
	orchestrations, err := app.ListRuntimeWorkerOrchestrations(s.workspace)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "读取运行时编排记录失败")
		return
	}

	workerRows := make([]map[string]any, 0, len(workers))
	counts := map[string]int{}
	for _, worker := range workers {
		status := worker.Status
		if worker.Stale && status != "offline" {
			status = "offline"
		}
		counts[status]++
		counts[worker.Mode]++
		workerRows = append(workerRows, map[string]any{
			"workerId":     worker.WorkerID,
			"mode":         worker.Mode,
			"status":       status,
			"endpoint":     worker.Endpoint,
			"capabilities": worker.Capabilities,
			"labels":       worker.Labels,
			"registeredAt": worker.RegisteredAt,
			"heartbeatAt":  worker.HeartbeatAt,
			"stale":        worker.Stale,
			"path":         filepath.ToSlash(worker.Path),
		})
	}

	orches := make([]map[string]any, 0, len(orchestrations))
	for _, orchestration := range orchestrations {
		orches = append(orches, map[string]any{
			"path":                                  filepath.ToSlash(orchestration.Path),
			"status":                                orchestration.Status,
			"mode":                                  orchestration.Mode,
			"runId":                                 orchestration.RunID,
			"transportMode":                         orchestration.TransportMode,
			"createdAt":                             orchestration.CreatedAt,
			"maxCandidates":                         orchestration.MaxCandidates,
			"placementReady":                        orchestration.PlacementReady,
			"placementSkipped":                      orchestration.PlacementSkipped,
			"placementPlanPath":                     orchestration.PlacementPlanPath,
			"claimedCount":                          orchestration.ClaimedCount,
			"claimSkipped":                          orchestration.ClaimSkipped,
			"claimFailed":                           orchestration.ClaimFailed,
			"claimExecutionPaths":                   orchestration.ClaimExecutionPaths,
			"placementDispatchPlanPath":             orchestration.PlacementDispatchPlanPath,
			"dispatchReady":                         orchestration.DispatchReady,
			"dispatchSkipped":                       orchestration.DispatchSkipped,
			"placementDispatchBatchPath":            orchestration.PlacementDispatchBatchPath,
			"placementDispatchBatchStatus":          orchestration.PlacementDispatchBatchState,
			"dispatchedCount":                       orchestration.DispatchedCount,
			"dispatchFailed":                        orchestration.DispatchFailed,
			"includeLifecycleMaintenance":           orchestration.IncludeLifecycleMaintenance,
			"executeLifecycleRetryDispatches":       orchestration.ExecuteLifecycleRetryDispatches,
			"dispatchLifecycleRetryClaims":          orchestration.DispatchLifecycleRetryClaims,
			"lifecycleRetryTransportMode":           orchestration.LifecycleRetryTransportMode,
			"lifecycleMaintenancePath":              orchestration.LifecycleMaintenancePath,
			"lifecycleAuditPath":                    orchestration.LifecycleAuditPath,
			"retryDispatchExecutionsCreated":        orchestration.RetryDispatchExecutionsCreated,
			"retryDispatchClaimsCreated":            orchestration.RetryDispatchClaimsCreated,
			"retryDispatchClaimsDispatched":         orchestration.RetryDispatchClaimsDispatched,
			"retryDispatchTransportRequestsCreated": orchestration.RetryDispatchTransportRequestsCreated,
			"retryDispatchExecutionPaths":           orchestration.RetryDispatchExecutionPaths,
			"error":                                 orchestration.Error,
		})
	}

	respondJSON(w, http.StatusOK, map[string]any{
		"workers":           workerRows,
		"orchestrations":    orches,
		"counts":            counts,
		"updatedAt":         now.Format(time.RFC3339Nano),
		"staleAfterSeconds": int(app.DefaultRuntimeWorkerStaleAfter / time.Second),
	})
}
