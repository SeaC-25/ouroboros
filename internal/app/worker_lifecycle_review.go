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

type WorkerLifecycleLeaseSummary struct {
	Total        int            `json:"total"`
	Active       int            `json:"active"`
	StaleActive  int            `json:"stale_active"`
	FreshActive  int            `json:"fresh_active"`
	Completed    int            `json:"completed"`
	Failed       int            `json:"failed"`
	Reclaimed    int            `json:"reclaimed"`
	Other        int            `json:"other"`
	StatusCounts map[string]int `json:"status_counts,omitempty"`
}

type WorkerLifecycleQueueSummary struct {
	Total        int            `json:"total"`
	Claimed      int            `json:"claimed"`
	Dispatched   int            `json:"dispatched"`
	Completed    int            `json:"completed"`
	Failed       int            `json:"failed"`
	Canceled     int            `json:"canceled"`
	Expired      int            `json:"expired"`
	Other        int            `json:"other"`
	StatusCounts map[string]int `json:"status_counts,omitempty"`
}

type WorkerLifecycleTransportSummary struct {
	RequestsTotal            int            `json:"requests_total"`
	Pending                  int            `json:"pending"`
	Acknowledged             int            `json:"acknowledged"`
	Completed                int            `json:"completed"`
	Failed                   int            `json:"failed"`
	Other                    int            `json:"other"`
	DispatchResultsTotal     int            `json:"dispatch_results_total"`
	DispatchResultsCompleted int            `json:"dispatch_results_completed"`
	DispatchResultsFailed    int            `json:"dispatch_results_failed"`
	DispatchResultsOther     int            `json:"dispatch_results_other"`
	RequestStatusCounts      map[string]int `json:"request_status_counts,omitempty"`
	ResultStatusCounts       map[string]int `json:"result_status_counts,omitempty"`
}

type WorkerLifecycleReviewPlan struct {
	Mode              string                              `json:"mode"`
	StaleAfterSeconds int64                               `json:"stale_after_seconds"`
	LeaseSummary      WorkerLifecycleLeaseSummary         `json:"lease_summary"`
	QueueSummary      WorkerLifecycleQueueSummary         `json:"queue_summary"`
	TransportSummary  WorkerLifecycleTransportSummary     `json:"transport_summary"`
	Reclaim           WorkerLeaseReclaimPlan              `json:"reclaim"`
	Reconciliation    WorkerLeaseResultReconciliationPlan `json:"reconciliation"`
}

type WorkerLifecycleReview struct {
	Version           int                                 `json:"version"`
	Path              string                              `json:"-"`
	Status            string                              `json:"status"`
	Mode              string                              `json:"mode"`
	CreatedAt         string                              `json:"created_at"`
	StaleAfterSeconds int64                               `json:"stale_after_seconds"`
	LeaseSummary      WorkerLifecycleLeaseSummary         `json:"lease_summary"`
	QueueSummary      WorkerLifecycleQueueSummary         `json:"queue_summary"`
	TransportSummary  WorkerLifecycleTransportSummary     `json:"transport_summary"`
	Reclaim           WorkerLeaseReclaimPlan              `json:"reclaim"`
	Reconciliation    WorkerLeaseResultReconciliationPlan `json:"reconciliation"`
}

func PlanWorkerLifecycleReview(root string, now time.Time, staleAfter time.Duration) (WorkerLifecycleReviewPlan, error) {
	if now.IsZero() {
		now = time.Now().UTC()
	} else {
		now = now.UTC()
	}
	if staleAfter <= 0 {
		staleAfter = DefaultWorkerLeaseStaleAfter
	}
	leases, err := ListWorkerLeasesWithStaleness(root, now, staleAfter)
	if err != nil {
		return WorkerLifecycleReviewPlan{}, err
	}
	claims, err := ListRuntimeWorkerQueueClaims(root)
	if err != nil {
		return WorkerLifecycleReviewPlan{}, err
	}
	requests, err := ListRuntimeWorkerTransportDispatchRequests(root, "")
	if err != nil {
		return WorkerLifecycleReviewPlan{}, err
	}
	results, err := ListRuntimeWorkerQueueDispatchResults(root)
	if err != nil {
		return WorkerLifecycleReviewPlan{}, err
	}
	reconciliation, err := PlanWorkerLeaseResultReconciliation(root)
	if err != nil {
		return WorkerLifecycleReviewPlan{}, err
	}
	return WorkerLifecycleReviewPlan{
		Mode:              "metadata-only",
		StaleAfterSeconds: int64(staleAfter / time.Second),
		LeaseSummary:      workerLifecycleLeaseSummary(leases),
		QueueSummary:      workerLifecycleQueueSummary(claims),
		TransportSummary:  workerLifecycleTransportSummary(requests, results),
		Reclaim:           workerLeaseReclaimPlanFromLeases(leases),
		Reconciliation:    reconciliation,
	}, nil
}

func CreateWorkerLifecycleReview(root string, now time.Time, staleAfter time.Duration) (WorkerLifecycleReview, error) {
	if now.IsZero() {
		now = time.Now().UTC()
	} else {
		now = now.UTC()
	}
	plan, err := PlanWorkerLifecycleReview(root, now, staleAfter)
	if err != nil {
		return WorkerLifecycleReview{}, err
	}
	review := WorkerLifecycleReview{
		Version:           1,
		Status:            "dry-run",
		Mode:              "metadata-only",
		CreatedAt:         now.Format(time.RFC3339Nano),
		StaleAfterSeconds: plan.StaleAfterSeconds,
		LeaseSummary:      plan.LeaseSummary,
		QueueSummary:      plan.QueueSummary,
		TransportSummary:  plan.TransportSummary,
		Reclaim:           plan.Reclaim,
		Reconciliation:    plan.Reconciliation,
	}
	dir := workerLifecycleReviewDir(root)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return WorkerLifecycleReview{}, fmt.Errorf("create worker lifecycle review directory: %w", err)
	}
	review.Path = filepath.Join(dir, safeWorkerLifecycleReviewName(review)+".json")
	data, err := json.MarshalIndent(review, "", "  ")
	if err != nil {
		return WorkerLifecycleReview{}, fmt.Errorf("encode worker lifecycle review: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(review.Path, data, 0o644); err != nil {
		return WorkerLifecycleReview{}, fmt.Errorf("write worker lifecycle review: %w", err)
	}
	return review, nil
}

func ListWorkerLifecycleReviews(root string) ([]WorkerLifecycleReview, error) {
	paths, err := filepath.Glob(filepath.Join(workerLifecycleReviewDir(root), "*.json"))
	if err != nil {
		return nil, fmt.Errorf("glob worker lifecycle reviews: %w", err)
	}
	sort.Strings(paths)
	reviews := make([]WorkerLifecycleReview, 0, len(paths))
	for _, path := range paths {
		review, err := readWorkerLifecycleReviewFile(path)
		if err != nil {
			return nil, err
		}
		reviews = append(reviews, review)
	}
	return reviews, nil
}

func ReadWorkerLifecycleReview(root, reviewPath string) (WorkerLifecycleReview, error) {
	path, err := resolveWorkerLifecycleReviewPath(root, reviewPath)
	if err != nil {
		return WorkerLifecycleReview{}, err
	}
	return readWorkerLifecycleReviewFile(path)
}

func readWorkerLifecycleReviewFile(path string) (WorkerLifecycleReview, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return WorkerLifecycleReview{}, fmt.Errorf("read worker lifecycle review %s: %w", path, err)
	}
	var review WorkerLifecycleReview
	if err := json.Unmarshal(data, &review); err != nil {
		return WorkerLifecycleReview{}, fmt.Errorf("decode worker lifecycle review %s: %w", path, err)
	}
	review.Path = path
	return review, nil
}

func workerLifecycleLeaseSummary(leases []WorkerLeaseArtifact) WorkerLifecycleLeaseSummary {
	summary := WorkerLifecycleLeaseSummary{
		Total:        len(leases),
		StatusCounts: map[string]int{},
	}
	for _, lease := range leases {
		status := strings.TrimSpace(lease.Status)
		if status == "" {
			status = "unknown"
		}
		summary.StatusCounts[status]++
		switch status {
		case "active":
			summary.Active++
			if lease.Stale {
				summary.StaleActive++
			} else {
				summary.FreshActive++
			}
		case "completed":
			summary.Completed++
		case "failed":
			summary.Failed++
		case "reclaimed":
			summary.Reclaimed++
		default:
			summary.Other++
		}
	}
	if len(summary.StatusCounts) == 0 {
		summary.StatusCounts = nil
	}
	return summary
}

func workerLifecycleQueueSummary(claims []RuntimeWorkerQueueClaim) WorkerLifecycleQueueSummary {
	summary := WorkerLifecycleQueueSummary{
		Total:        len(claims),
		StatusCounts: map[string]int{},
	}
	for _, claim := range claims {
		status := strings.TrimSpace(claim.Status)
		if status == "" {
			status = "unknown"
		}
		summary.StatusCounts[status]++
		switch status {
		case "claimed":
			summary.Claimed++
		case "dispatched":
			summary.Dispatched++
		case "completed":
			summary.Completed++
		case "failed":
			summary.Failed++
		case "canceled":
			summary.Canceled++
		case "expired":
			summary.Expired++
		default:
			summary.Other++
		}
	}
	if len(summary.StatusCounts) == 0 {
		summary.StatusCounts = nil
	}
	return summary
}

func workerLifecycleTransportSummary(requests []RuntimeWorkerTransportDispatchRequest, results []RuntimeWorkerQueueDispatchResult) WorkerLifecycleTransportSummary {
	summary := WorkerLifecycleTransportSummary{
		RequestsTotal:        len(requests),
		DispatchResultsTotal: len(results),
		RequestStatusCounts:  map[string]int{},
		ResultStatusCounts:   map[string]int{},
	}
	for _, request := range requests {
		status := strings.TrimSpace(request.Status)
		if status == "" {
			status = "unknown"
		}
		summary.RequestStatusCounts[status]++
		switch status {
		case "pending":
			summary.Pending++
		case "acknowledged":
			summary.Acknowledged++
		case "completed":
			summary.Completed++
		case "failed":
			summary.Failed++
		default:
			summary.Other++
		}
	}
	for _, result := range results {
		status := strings.TrimSpace(result.Status)
		if status == "" {
			status = "unknown"
		}
		summary.ResultStatusCounts[status]++
		switch status {
		case "completed":
			summary.DispatchResultsCompleted++
		case "failed":
			summary.DispatchResultsFailed++
		default:
			summary.DispatchResultsOther++
		}
	}
	if len(summary.RequestStatusCounts) == 0 {
		summary.RequestStatusCounts = nil
	}
	if len(summary.ResultStatusCounts) == 0 {
		summary.ResultStatusCounts = nil
	}
	return summary
}

func workerLifecycleReviewDir(root string) string {
	return filepath.Join(root, "runtime", "worker-lifecycle-reviews")
}

func resolveWorkerLifecycleReviewPath(root, reviewPath string) (string, error) {
	trimmed := strings.TrimSpace(reviewPath)
	if trimmed == "" {
		return "", fmt.Errorf("worker lifecycle review path is required")
	}
	dir, err := filepath.Abs(workerLifecycleReviewDir(root))
	if err != nil {
		return "", fmt.Errorf("resolve worker lifecycle review directory: %w", err)
	}
	candidate := trimmed
	if !filepath.IsAbs(candidate) {
		candidate = filepath.Join(dir, candidate)
	}
	candidate, err = filepath.Abs(candidate)
	if err != nil {
		return "", fmt.Errorf("resolve worker lifecycle review path: %w", err)
	}
	rel, err := filepath.Rel(dir, candidate)
	if err != nil {
		return "", fmt.Errorf("resolve worker lifecycle review relative path: %w", err)
	}
	if rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("worker lifecycle review path must stay under %s", dir)
	}
	if filepath.Ext(candidate) != ".json" {
		return "", fmt.Errorf("worker lifecycle review path must be a json file")
	}
	return candidate, nil
}

func safeWorkerLifecycleReviewName(review WorkerLifecycleReview) string {
	parts := []string{
		"worker-lifecycle-review",
		review.CreatedAt,
		fmt.Sprintf("leases=%d active=%d stale=%d completed=%d failed=%d reclaimed=%d",
			review.LeaseSummary.Total,
			review.LeaseSummary.Active,
			review.LeaseSummary.StaleActive,
			review.LeaseSummary.Completed,
			review.LeaseSummary.Failed,
			review.LeaseSummary.Reclaimed,
		),
		fmt.Sprintf("reclaim=%d/%d reconciliation=%d/%d",
			len(review.Reclaim.Ready),
			len(review.Reclaim.Skipped),
			len(review.Reconciliation.Ready),
			len(review.Reconciliation.Skipped),
		),
	}
	for _, candidate := range review.Reclaim.Ready {
		parts = append(parts, candidate.Lease.RunID, candidate.Lease.ExecutionID, candidate.Lease.Status, candidate.Mode)
	}
	for _, candidate := range review.Reconciliation.Ready {
		parts = append(parts, candidate.Result.RunID, candidate.Result.ExecutionID, candidate.Result.Status, candidate.Lease.Status, candidate.Mode)
	}
	return safeInstallRequestName(strings.Join(parts, "|"))
}
