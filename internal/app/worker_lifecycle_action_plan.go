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

type WorkerLifecycleActionSummary struct {
	Total                        int `json:"total"`
	AckPendingTransport          int `json:"ack_pending_transport"`
	RunAcknowledgedTransport     int `json:"run_acknowledged_transport"`
	ApplyDaemonControl           int `json:"apply_daemon_control"`
	ExecuteDispatchRetryDispatch int `json:"execute_dispatch_retry_dispatch"`
	ReclaimStaleLease            int `json:"reclaim_stale_lease"`
	ReconcileResultLease         int `json:"reconcile_result_lease"`
}

type WorkerLifecycleAction struct {
	Type              string `json:"type"`
	Gate              string `json:"gate"`
	Status            string `json:"status"`
	Mode              string `json:"mode"`
	Reason            string `json:"reason"`
	WorkerID          string `json:"worker_id,omitempty"`
	RunID             string `json:"run_id,omitempty"`
	ExecutionID       string `json:"execution_id,omitempty"`
	Tool              string `json:"tool,omitempty"`
	Capability        string `json:"capability,omitempty"`
	RequestPath       string `json:"request_path,omitempty"`
	LeasePath         string `json:"lease_path,omitempty"`
	ResultPath        string `json:"result_path,omitempty"`
	DispatchPath      string `json:"dispatch_path,omitempty"`
	RecommendedStatus string `json:"recommended_status,omitempty"`
}

type WorkerLifecycleActionPlanAnnotation struct {
	Decision   string `json:"decision"`
	Reason     string `json:"reason,omitempty"`
	Reviewer   string `json:"reviewer,omitempty"`
	ReviewedAt string `json:"reviewed_at,omitempty"`
}

type WorkerLifecycleActionPlanAnnotationOptions struct {
	Decision   string
	Reason     string
	Reviewer   string
	ReviewedAt time.Time
}

type WorkerLifecycleActionPlanSelection struct {
	Path                string                       `json:"path,omitempty"`
	Status              string                       `json:"status"`
	Mode                string                       `json:"mode"`
	PlanPath            string                       `json:"plan_path,omitempty"`
	PlanBasename        string                       `json:"plan_basename,omitempty"`
	ReviewPath          string                       `json:"review_path,omitempty"`
	StaleAfterSeconds   int64                        `json:"stale_after_seconds,omitempty"`
	Summary             WorkerLifecycleActionSummary `json:"summary,omitempty"`
	SupersededPlanPaths []string                     `json:"superseded_plan_paths,omitempty"`
	SelectedBy          string                       `json:"selected_by,omitempty"`
	SelectedAt          string                       `json:"selected_at,omitempty"`
	ClearedBy           string                       `json:"cleared_by,omitempty"`
	ClearedAt           string                       `json:"cleared_at,omitempty"`
	Reason              string                       `json:"reason,omitempty"`
}

type WorkerLifecycleActionPlanSelectionOptions struct {
	SelectedBy string
	SelectedAt time.Time
	Reason     string
}

type WorkerLifecycleActionPlanSelectionClearOptions struct {
	ClearedBy string
	ClearedAt time.Time
	Reason    string
}

type WorkerLifecycleActionPlan struct {
	Version           int                                  `json:"version"`
	Path              string                               `json:"-"`
	Status            string                               `json:"status"`
	Mode              string                               `json:"mode"`
	CreatedAt         string                               `json:"created_at"`
	ReviewPath        string                               `json:"review_path,omitempty"`
	StaleAfterSeconds int64                                `json:"stale_after_seconds"`
	Summary           WorkerLifecycleActionSummary         `json:"summary"`
	Actions           []WorkerLifecycleAction              `json:"actions"`
	Annotation        *WorkerLifecycleActionPlanAnnotation `json:"annotation,omitempty"`
}

func PlanWorkerLifecycleActionPlan(root, reviewPath string, now time.Time, staleAfter time.Duration) (WorkerLifecycleActionPlan, error) {
	if now.IsZero() {
		now = time.Now().UTC()
	} else {
		now = now.UTC()
	}
	if staleAfter <= 0 {
		staleAfter = DefaultWorkerLeaseStaleAfter
	}
	resolvedReviewPath := ""
	if strings.TrimSpace(reviewPath) != "" {
		review, err := ReadWorkerLifecycleReview(root, reviewPath)
		if err != nil {
			return WorkerLifecycleActionPlan{}, err
		}
		resolvedReviewPath = review.Path
	}
	review, err := PlanWorkerLifecycleReview(root, now, staleAfter)
	if err != nil {
		return WorkerLifecycleActionPlan{}, err
	}
	requests, err := ListRuntimeWorkerTransportDispatchRequests(root, "")
	if err != nil {
		return WorkerLifecycleActionPlan{}, err
	}
	controls, err := listRuntimeWorkerTransportDaemonControlRequestsForWorkspace(root)
	if err != nil {
		return WorkerLifecycleActionPlan{}, err
	}
	retryDispatches, err := ListRuntimeWorkerDispatchRetryDispatches(root)
	if err != nil {
		return WorkerLifecycleActionPlan{}, err
	}
	retryDispatchExecutions, err := ListRuntimeWorkerDispatchRetryDispatchExecutions(root)
	if err != nil {
		return WorkerLifecycleActionPlan{}, err
	}
	executedRetryDispatches := map[string]bool{}
	for _, execution := range retryDispatchExecutions {
		if strings.TrimSpace(execution.RetryDispatchPath) != "" {
			executedRetryDispatches[execution.RetryDispatchPath] = true
		}
	}
	plan := WorkerLifecycleActionPlan{
		Version:           1,
		Status:            "dry-run",
		Mode:              "metadata-only",
		CreatedAt:         now.Format(time.RFC3339Nano),
		ReviewPath:        resolvedReviewPath,
		StaleAfterSeconds: int64(staleAfter / time.Second),
	}
	for _, request := range requests {
		status := strings.TrimSpace(request.Status)
		switch status {
		case "pending":
			plan.Actions = append(plan.Actions, WorkerLifecycleAction{
				Type:         "ack-transport-request",
				Gate:         "worker-transport-ack",
				Status:       "ready",
				Mode:         "metadata-only",
				Reason:       "pending-file-outbox-request",
				WorkerID:     request.WorkerID,
				RunID:        request.RunID,
				ExecutionID:  request.ExecutionID,
				Tool:         request.Tool,
				Capability:   request.Capability,
				RequestPath:  request.Path,
				DispatchPath: request.DispatchExecutionPath,
			})
		case "acknowledged":
			if strings.TrimSpace(request.ResultPath) != "" {
				continue
			}
			plan.Actions = append(plan.Actions, WorkerLifecycleAction{
				Type:         "run-transport-request",
				Gate:         "worker-transport-run-once",
				Status:       "ready",
				Mode:         "metadata-only",
				Reason:       "acknowledged-file-outbox-request",
				WorkerID:     request.WorkerID,
				RunID:        request.RunID,
				ExecutionID:  request.ExecutionID,
				Tool:         request.Tool,
				Capability:   request.Capability,
				RequestPath:  request.Path,
				DispatchPath: request.DispatchExecutionPath,
			})
		}
	}
	for _, control := range controls {
		if strings.TrimSpace(control.Status) != "pending" {
			continue
		}
		plan.Actions = append(plan.Actions, WorkerLifecycleAction{
			Type:              "apply-daemon-control",
			Gate:              "worker-transport-run-daemon-enable-control",
			Status:            "ready",
			Mode:              "metadata-only",
			Reason:            "pending-daemon-control",
			WorkerID:          control.WorkerID,
			RequestPath:       control.Path,
			RecommendedStatus: control.Action,
		})
	}
	for _, dispatch := range retryDispatches {
		if strings.TrimSpace(dispatch.Status) != "dry-run" || strings.TrimSpace(dispatch.Mode) != "metadata-only" {
			continue
		}
		if dispatch.Summary.Ready <= 0 || executedRetryDispatches[dispatch.Path] {
			continue
		}
		plan.Actions = append(plan.Actions, WorkerLifecycleAction{
			Type:              "execute-dispatch-retry-dispatch",
			Gate:              "worker-dispatch-retry-dispatch-execute",
			Status:            "ready",
			Mode:              "metadata-only",
			Reason:            "ready-worker-dispatch-retry-dispatch",
			DispatchPath:      dispatch.Path,
			RecommendedStatus: "execute",
		})
	}
	for _, candidate := range review.Reclaim.Ready {
		lease := candidate.Lease
		plan.Actions = append(plan.Actions, WorkerLifecycleAction{
			Type:        "reclaim-stale-lease",
			Gate:        "worker-lease-reclaim",
			Status:      "ready",
			Mode:        "metadata-only",
			Reason:      "stale-active-worker-lease",
			WorkerID:    lease.WorkerID,
			RunID:       lease.RunID,
			ExecutionID: lease.ExecutionID,
			Tool:        lease.Tool,
			Capability:  lease.Capability,
			LeasePath:   lease.Path,
		})
	}
	for _, candidate := range review.Reconciliation.Ready {
		plan.Actions = append(plan.Actions, WorkerLifecycleAction{
			Type:              "reconcile-result-lease",
			Gate:              "worker-lease-result-reconcile",
			Status:            "ready",
			Mode:              "metadata-only",
			Reason:            candidate.Reason,
			WorkerID:          candidate.Result.WorkerID,
			RunID:             candidate.Result.RunID,
			ExecutionID:       candidate.Result.ExecutionID,
			Tool:              candidate.Result.Tool,
			Capability:        candidate.Result.Capability,
			LeasePath:         workerLifecycleActionDisplayPath(candidate.LeasePath, candidate.Lease.Path),
			ResultPath:        workerLifecycleActionDisplayPath(candidate.ResultPath, candidate.Result.Path),
			DispatchPath:      workerLifecycleActionDisplayPath(candidate.DispatchPath, candidate.Dispatch.Path),
			RecommendedStatus: candidate.RecommendedStatus,
		})
	}
	sort.SliceStable(plan.Actions, func(i, j int) bool {
		return workerLifecycleActionSortKey(plan.Actions[i]) < workerLifecycleActionSortKey(plan.Actions[j])
	})
	plan.Summary = workerLifecycleActionSummary(plan.Actions)
	return plan, nil
}

func CreateWorkerLifecycleActionPlan(root, reviewPath string, now time.Time, staleAfter time.Duration) (WorkerLifecycleActionPlan, error) {
	if now.IsZero() {
		now = time.Now().UTC()
	} else {
		now = now.UTC()
	}
	plan, err := PlanWorkerLifecycleActionPlan(root, reviewPath, now, staleAfter)
	if err != nil {
		return WorkerLifecycleActionPlan{}, err
	}
	dir := workerLifecycleActionPlanDir(root)
	if err := rejectUnsafeWorkerLifecycleActionPlanDir(dir); err != nil {
		return WorkerLifecycleActionPlan{}, err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return WorkerLifecycleActionPlan{}, fmt.Errorf("create worker lifecycle action plan directory: %w", err)
	}
	if err := requireWorkerLifecycleActionPlanDirectory(dir); err != nil {
		return WorkerLifecycleActionPlan{}, err
	}
	plan.Path = filepath.Join(dir, safeWorkerLifecycleActionPlanName(plan)+".json")
	if err := rejectUnsafeWorkerLifecycleActionPlanFile(plan.Path); err != nil {
		return WorkerLifecycleActionPlan{}, err
	}
	data, err := json.MarshalIndent(plan, "", "  ")
	if err != nil {
		return WorkerLifecycleActionPlan{}, fmt.Errorf("encode worker lifecycle action plan: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(plan.Path, data, 0o644); err != nil {
		return WorkerLifecycleActionPlan{}, fmt.Errorf("write worker lifecycle action plan: %w", err)
	}
	return plan, nil
}

func ListWorkerLifecycleActionPlans(root string) ([]WorkerLifecycleActionPlan, error) {
	paths, err := filepath.Glob(filepath.Join(workerLifecycleActionPlanDir(root), "*.json"))
	if err != nil {
		return nil, fmt.Errorf("glob worker lifecycle action plans: %w", err)
	}
	sort.Strings(paths)
	plans := make([]WorkerLifecycleActionPlan, 0, len(paths))
	for _, path := range paths {
		plan, err := readWorkerLifecycleActionPlanFile(path)
		if err != nil {
			return nil, err
		}
		plans = append(plans, plan)
	}
	return plans, nil
}

func ReadWorkerLifecycleActionPlan(root, planPath string) (WorkerLifecycleActionPlan, error) {
	path, err := resolveWorkerLifecycleActionPlanPath(root, planPath)
	if err != nil {
		return WorkerLifecycleActionPlan{}, err
	}
	return readWorkerLifecycleActionPlanFile(path)
}

func AnnotateWorkerLifecycleActionPlan(root, planPath string, options WorkerLifecycleActionPlanAnnotationOptions) (WorkerLifecycleActionPlan, error) {
	annotation, err := normalizeWorkerLifecycleActionPlanAnnotation(options)
	if err != nil {
		return WorkerLifecycleActionPlan{}, err
	}
	path, err := resolveWorkerLifecycleActionPlanPath(root, planPath)
	if err != nil {
		return WorkerLifecycleActionPlan{}, err
	}
	plan, err := readWorkerLifecycleActionPlanFile(path)
	if err != nil {
		return WorkerLifecycleActionPlan{}, err
	}
	plan.Annotation = &annotation
	if err := rejectUnsafeWorkerLifecycleActionPlanFile(path); err != nil {
		return WorkerLifecycleActionPlan{}, err
	}
	data, err := json.MarshalIndent(plan, "", "  ")
	if err != nil {
		return WorkerLifecycleActionPlan{}, fmt.Errorf("encode worker lifecycle action plan annotation: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return WorkerLifecycleActionPlan{}, fmt.Errorf("write worker lifecycle action plan annotation: %w", err)
	}
	plan.Path = path
	return plan, nil
}

func SelectWorkerLifecycleActionPlan(root, planPath string, options WorkerLifecycleActionPlanSelectionOptions) (WorkerLifecycleActionPlanSelection, error) {
	plan, err := ReadWorkerLifecycleActionPlan(root, planPath)
	if err != nil {
		return WorkerLifecycleActionPlanSelection{}, err
	}
	if err := validateSelectableWorkerLifecycleActionPlan(plan); err != nil {
		return WorkerLifecycleActionPlanSelection{}, err
	}
	current, found, err := ReadWorkerLifecycleActionPlanSelection(root)
	if err != nil {
		return WorkerLifecycleActionPlanSelection{}, err
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
	if found && workerLifecycleActionPlanSelectionIsCurrent(current) {
		currentPath := strings.TrimSpace(current.PlanPath)
		if currentPath != "" && currentPath != plan.Path {
			superseded = append(superseded, currentPath)
		}
	}
	selection := workerLifecycleActionPlanSelectionFromPlan(root, plan)
	selection.Status = "current"
	selection.SelectedBy = selectedBy
	selection.SelectedAt = selectedAt.UTC().Format(time.RFC3339)
	selection.ClearedBy = ""
	selection.ClearedAt = ""
	selection.Reason = strings.TrimSpace(options.Reason)
	selection.SupersededPlanPaths = superseded
	if err := writeWorkerLifecycleActionPlanSelection(root, selection); err != nil {
		return WorkerLifecycleActionPlanSelection{}, err
	}
	return selection, nil
}

func ClearWorkerLifecycleActionPlanSelection(root string, options WorkerLifecycleActionPlanSelectionClearOptions) (WorkerLifecycleActionPlanSelection, error) {
	selection, found, err := ReadWorkerLifecycleActionPlanSelection(root)
	if err != nil {
		return WorkerLifecycleActionPlanSelection{}, err
	}
	if !found {
		selection = WorkerLifecycleActionPlanSelection{
			Path:   workerLifecycleActionPlanSelectionPath(root),
			Status: "cleared",
			Mode:   "metadata-only",
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
	selection.Path = workerLifecycleActionPlanSelectionPath(root)
	selection.Status = "cleared"
	selection.Mode = "metadata-only"
	selection.ClearedBy = clearedBy
	selection.ClearedAt = clearedAt.UTC().Format(time.RFC3339)
	selection.Reason = strings.TrimSpace(options.Reason)
	if err := writeWorkerLifecycleActionPlanSelection(root, selection); err != nil {
		return WorkerLifecycleActionPlanSelection{}, err
	}
	return selection, nil
}

func ReadWorkerLifecycleActionPlanSelection(root string) (WorkerLifecycleActionPlanSelection, bool, error) {
	path := workerLifecycleActionPlanSelectionPath(root)
	if err := rejectUnsafeWorkerLifecycleActionPlanSelectionFile(path); err != nil {
		return WorkerLifecycleActionPlanSelection{}, false, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return WorkerLifecycleActionPlanSelection{}, false, nil
		}
		return WorkerLifecycleActionPlanSelection{}, false, fmt.Errorf("read worker lifecycle action plan selection %s: %w", path, err)
	}
	var selection WorkerLifecycleActionPlanSelection
	if err := json.Unmarshal(data, &selection); err != nil {
		return WorkerLifecycleActionPlanSelection{}, false, fmt.Errorf("decode worker lifecycle action plan selection %s: %w", path, err)
	}
	selection.Path = path
	return selection, true, nil
}

func readWorkerLifecycleActionPlanFile(path string) (WorkerLifecycleActionPlan, error) {
	if err := rejectUnsafeWorkerLifecycleActionPlanFile(path); err != nil {
		return WorkerLifecycleActionPlan{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return WorkerLifecycleActionPlan{}, fmt.Errorf("read worker lifecycle action plan %s: %w", path, err)
	}
	var plan WorkerLifecycleActionPlan
	if err := json.Unmarshal(data, &plan); err != nil {
		return WorkerLifecycleActionPlan{}, fmt.Errorf("decode worker lifecycle action plan %s: %w", path, err)
	}
	plan.Path = path
	return plan, nil
}

func workerLifecycleActionSummary(actions []WorkerLifecycleAction) WorkerLifecycleActionSummary {
	summary := WorkerLifecycleActionSummary{Total: len(actions)}
	for _, action := range actions {
		switch strings.TrimSpace(action.Type) {
		case "ack-transport-request":
			summary.AckPendingTransport++
		case "run-transport-request":
			summary.RunAcknowledgedTransport++
		case "apply-daemon-control":
			summary.ApplyDaemonControl++
		case "execute-dispatch-retry-dispatch":
			summary.ExecuteDispatchRetryDispatch++
		case "reclaim-stale-lease":
			summary.ReclaimStaleLease++
		case "reconcile-result-lease":
			summary.ReconcileResultLease++
		}
	}
	return summary
}

func workerLifecycleActionPlanDir(root string) string {
	return filepath.Join(root, "runtime", "worker-lifecycle-action-plans")
}

func workerLifecycleActionPlanSelectionPath(root string) string {
	return filepath.Join(root, "runtime", "worker-lifecycle-action-plan-selection.json")
}

func resolveWorkerLifecycleActionPlanPath(root, planPath string) (string, error) {
	trimmed := strings.TrimSpace(planPath)
	if trimmed == "" {
		return "", fmt.Errorf("worker lifecycle action plan path is required")
	}
	dir, err := filepath.Abs(workerLifecycleActionPlanDir(root))
	if err != nil {
		return "", fmt.Errorf("resolve worker lifecycle action plan directory: %w", err)
	}
	candidate := trimmed
	if !filepath.IsAbs(candidate) {
		candidate = filepath.Join(dir, candidate)
	}
	candidate, err = filepath.Abs(candidate)
	if err != nil {
		return "", fmt.Errorf("resolve worker lifecycle action plan path: %w", err)
	}
	rel, err := filepath.Rel(dir, candidate)
	if err != nil {
		return "", fmt.Errorf("resolve worker lifecycle action plan relative path: %w", err)
	}
	if rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("worker lifecycle action plan path must stay under %s", dir)
	}
	if filepath.Ext(candidate) != ".json" {
		return "", fmt.Errorf("worker lifecycle action plan path must be a json file")
	}
	return candidate, nil
}

func normalizeWorkerLifecycleActionPlanAnnotation(options WorkerLifecycleActionPlanAnnotationOptions) (WorkerLifecycleActionPlanAnnotation, error) {
	decision := strings.ToLower(strings.TrimSpace(options.Decision))
	switch decision {
	case "accepted", "deferred", "rejected":
	case "":
		return WorkerLifecycleActionPlanAnnotation{}, fmt.Errorf("worker lifecycle action plan annotation decision is required")
	default:
		return WorkerLifecycleActionPlanAnnotation{}, fmt.Errorf("unsupported worker lifecycle action plan annotation decision %q", options.Decision)
	}
	reviewer := strings.TrimSpace(options.Reviewer)
	if reviewer == "" {
		reviewer = "operator"
	}
	reviewedAt := options.ReviewedAt
	if reviewedAt.IsZero() {
		reviewedAt = time.Now().UTC()
	}
	return WorkerLifecycleActionPlanAnnotation{
		Decision:   decision,
		Reason:     strings.TrimSpace(options.Reason),
		Reviewer:   reviewer,
		ReviewedAt: reviewedAt.UTC().Format(time.RFC3339),
	}, nil
}

func validateSelectableWorkerLifecycleActionPlan(plan WorkerLifecycleActionPlan) error {
	if plan.Version != 1 {
		return fmt.Errorf("worker lifecycle action plan version is unsupported: %d", plan.Version)
	}
	if strings.TrimSpace(plan.Status) != "dry-run" || strings.TrimSpace(plan.Mode) != "metadata-only" {
		return fmt.Errorf("worker lifecycle action plan must be dry-run metadata-only before selection")
	}
	if plan.Annotation == nil || strings.TrimSpace(plan.Annotation.Decision) != "accepted" {
		return fmt.Errorf("worker lifecycle action plan selection requires accepted annotation")
	}
	return nil
}

func workerLifecycleActionPlanSelectionFromPlan(root string, plan WorkerLifecycleActionPlan) WorkerLifecycleActionPlanSelection {
	return WorkerLifecycleActionPlanSelection{
		Path:              workerLifecycleActionPlanSelectionPath(root),
		Mode:              "metadata-only",
		PlanPath:          plan.Path,
		PlanBasename:      filepath.Base(plan.Path),
		ReviewPath:        plan.ReviewPath,
		StaleAfterSeconds: plan.StaleAfterSeconds,
		Summary:           plan.Summary,
	}
}

func writeWorkerLifecycleActionPlanSelection(root string, selection WorkerLifecycleActionPlanSelection) error {
	runtimeDir := filepath.Join(root, "runtime")
	if err := rejectUnsafeWorkerLifecycleActionRuntimeDir(runtimeDir); err != nil {
		return err
	}
	if err := os.MkdirAll(runtimeDir, 0o755); err != nil {
		return fmt.Errorf("create worker lifecycle action plan selection directory: %w", err)
	}
	if err := requireWorkerLifecycleActionPlanDirectory(runtimeDir); err != nil {
		return err
	}
	selection.Path = workerLifecycleActionPlanSelectionPath(root)
	if err := rejectUnsafeWorkerLifecycleActionPlanSelectionFile(selection.Path); err != nil {
		return err
	}
	data, err := json.MarshalIndent(selection, "", "  ")
	if err != nil {
		return fmt.Errorf("encode worker lifecycle action plan selection: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(selection.Path, data, 0o644); err != nil {
		return fmt.Errorf("write worker lifecycle action plan selection: %w", err)
	}
	return nil
}

func workerLifecycleActionPlanSelectionIsCurrent(selection WorkerLifecycleActionPlanSelection) bool {
	return strings.TrimSpace(selection.Status) == "current"
}

func rejectUnsafeWorkerLifecycleActionRuntimeDir(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("inspect worker lifecycle action runtime directory: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("worker lifecycle action runtime directory must not be a symlink: %s", path)
	}
	if !info.IsDir() {
		return fmt.Errorf("worker lifecycle action runtime path is not a directory: %s", path)
	}
	return nil
}

func rejectUnsafeWorkerLifecycleActionPlanDir(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("inspect worker lifecycle action plan directory: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("worker lifecycle action plan directory must not be a symlink: %s", path)
	}
	if !info.IsDir() {
		return fmt.Errorf("worker lifecycle action plan path is not a directory: %s", path)
	}
	return nil
}

func requireWorkerLifecycleActionPlanDirectory(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("inspect worker lifecycle action plan directory: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("worker lifecycle action plan directory must not be a symlink: %s", path)
	}
	if !info.IsDir() {
		return fmt.Errorf("worker lifecycle action plan path is not a directory: %s", path)
	}
	return nil
}

func rejectUnsafeWorkerLifecycleActionPlanFile(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("inspect worker lifecycle action plan file: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("worker lifecycle action plan file must not be a symlink: %s", path)
	}
	if info.IsDir() {
		return fmt.Errorf("write worker lifecycle action plan: path is a directory: %s", path)
	}
	return nil
}

func rejectUnsafeWorkerLifecycleActionPlanSelectionFile(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("inspect worker lifecycle action plan selection file: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("worker lifecycle action plan selection file must not be a symlink: %s", path)
	}
	if info.IsDir() {
		return fmt.Errorf("write worker lifecycle action plan selection: path is a directory: %s", path)
	}
	return nil
}

func workerLifecycleActionSortKey(action WorkerLifecycleAction) string {
	return fmt.Sprintf("%02d|%s|%s|%s|%s",
		workerLifecycleActionTypeOrder(action.Type),
		action.WorkerID,
		action.RunID,
		action.ExecutionID,
		action.RequestPath+action.LeasePath+action.ResultPath,
	)
}

func workerLifecycleActionTypeOrder(actionType string) int {
	switch strings.TrimSpace(actionType) {
	case "ack-transport-request":
		return 10
	case "run-transport-request":
		return 20
	case "apply-daemon-control":
		return 30
	case "execute-dispatch-retry-dispatch":
		return 35
	case "reconcile-result-lease":
		return 40
	case "reclaim-stale-lease":
		return 50
	default:
		return 90
	}
}

func workerLifecycleActionDisplayPath(primary, fallback string) string {
	if strings.TrimSpace(primary) != "" {
		return primary
	}
	return fallback
}

func safeWorkerLifecycleActionPlanName(plan WorkerLifecycleActionPlan) string {
	parts := []string{
		"worker-lifecycle-action-plan",
		plan.CreatedAt,
		fmt.Sprintf("actions=%d ack=%d run=%d daemon=%d retry-dispatch=%d reclaim=%d reconcile=%d",
			plan.Summary.Total,
			plan.Summary.AckPendingTransport,
			plan.Summary.RunAcknowledgedTransport,
			plan.Summary.ApplyDaemonControl,
			plan.Summary.ExecuteDispatchRetryDispatch,
			plan.Summary.ReclaimStaleLease,
			plan.Summary.ReconcileResultLease,
		),
		filepath.Base(plan.ReviewPath),
	}
	for _, action := range plan.Actions {
		parts = append(parts, action.Type, action.WorkerID, action.RunID, action.ExecutionID, filepath.Base(action.RequestPath), filepath.Base(action.LeasePath), filepath.Base(action.ResultPath), filepath.Base(action.DispatchPath))
	}
	return safeInstallRequestName(strings.Join(parts, "|"))
}
