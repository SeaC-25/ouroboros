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

const DefaultRuntimeWorkerQueueClaimLeaseDuration = 5 * time.Minute

type RuntimeWorkerQueueClaim struct {
	Version               int                          `json:"version"`
	Path                  string                       `json:"-"`
	ClaimID               string                       `json:"claim_id"`
	Status                string                       `json:"status"`
	Mode                  string                       `json:"mode"`
	WorkerID              string                       `json:"worker_id"`
	WorkerMode            string                       `json:"worker_mode"`
	WorkerEndpoint        string                       `json:"worker_endpoint,omitempty"`
	RunID                 string                       `json:"run_id"`
	ExecutionID           string                       `json:"execution_id"`
	Tool                  string                       `json:"tool"`
	Capability            string                       `json:"capability"`
	ToolPlan              RuntimeWorkerToolPlanPayload `json:"tool_plan,omitempty"`
	ClaimedAt             string                       `json:"claimed_at"`
	ExpiresAt             string                       `json:"expires_at"`
	LeaseSeconds          int64                        `json:"lease_seconds"`
	DispatchedAt          string                       `json:"dispatched_at,omitempty"`
	DispatchExecutionPath string                       `json:"dispatch_execution_path,omitempty"`
	CanceledAt            string                       `json:"canceled_at,omitempty"`
	CanceledBy            string                       `json:"canceled_by,omitempty"`
	CancelReason          string                       `json:"cancel_reason,omitempty"`
	ExpiredAt             string                       `json:"expired_at,omitempty"`
	ExpiredBy             string                       `json:"expired_by,omitempty"`
	ExpireReason          string                       `json:"expire_reason,omitempty"`
	RenewedAt             string                       `json:"renewed_at,omitempty"`
	RenewedBy             string                       `json:"renewed_by,omitempty"`
	RenewReason           string                       `json:"renew_reason,omitempty"`
	ResultStatus          string                       `json:"result_status,omitempty"`
	ResultPath            string                       `json:"result_path,omitempty"`
	CompletedAt           string                       `json:"completed_at,omitempty"`
}

type RuntimeWorkerQueueClaimOptions struct {
	Now           time.Time
	LeaseDuration time.Duration
}

type RuntimeWorkerQueueClaimCancelOptions struct {
	Cancel     bool
	Now        time.Time
	CanceledBy string
	Reason     string
}

type RuntimeWorkerQueueClaimExpireOptions struct {
	Expire    bool
	Now       time.Time
	ExpiredBy string
	Reason    string
}

type RuntimeWorkerQueueClaimRenewOptions struct {
	Renew         bool
	Now           time.Time
	LeaseDuration time.Duration
	RenewedBy     string
	Reason        string
}

func CreateRuntimeWorkerQueueClaim(root string, claim RuntimeWorkerQueueClaim, options RuntimeWorkerQueueClaimOptions) (RuntimeWorkerQueueClaim, error) {
	workerRef := strings.TrimSpace(claim.WorkerID)
	if workerRef == "" {
		return RuntimeWorkerQueueClaim{}, fmt.Errorf("runtime worker id is required")
	}
	runID := strings.TrimSpace(claim.RunID)
	if runID == "" {
		return RuntimeWorkerQueueClaim{}, fmt.Errorf("queue claim run id is required")
	}
	executionID := strings.TrimSpace(claim.ExecutionID)
	if executionID == "" {
		return RuntimeWorkerQueueClaim{}, fmt.Errorf("queue claim execution id is required")
	}
	tool := strings.TrimSpace(claim.Tool)
	if tool == "" {
		return RuntimeWorkerQueueClaim{}, fmt.Errorf("queue claim tool is required")
	}
	capability := strings.TrimSpace(claim.Capability)
	if capability == "" {
		return RuntimeWorkerQueueClaim{}, fmt.Errorf("queue claim capability is required")
	}
	now := options.Now.UTC()
	if now.IsZero() {
		now = time.Now().UTC()
	}
	worker, err := ReadRuntimeWorker(root, workerRef)
	if err != nil {
		return RuntimeWorkerQueueClaim{}, err
	}
	if strings.TrimSpace(worker.Status) == "offline" {
		return RuntimeWorkerQueueClaim{}, fmt.Errorf("runtime worker is offline: %s", worker.WorkerID)
	}
	if runtimeWorkerIsStale(worker, now, DefaultRuntimeWorkerStaleAfter) {
		return RuntimeWorkerQueueClaim{}, fmt.Errorf("runtime worker is stale: %s", worker.WorkerID)
	}
	if !runtimeWorkerHasCapability(worker, capability) {
		return RuntimeWorkerQueueClaim{}, fmt.Errorf("runtime worker %s does not advertise capability %s", worker.WorkerID, capability)
	}
	leaseDuration := options.LeaseDuration
	if leaseDuration <= 0 {
		leaseDuration = DefaultRuntimeWorkerQueueClaimLeaseDuration
	}
	claimedAt := now.Format(time.RFC3339Nano)
	claim = RuntimeWorkerQueueClaim{
		Version:        1,
		ClaimID:        safeRuntimeWorkerQueueClaimID(worker.WorkerID, runID, executionID, capability, claimedAt),
		Status:         "claimed",
		Mode:           "metadata-only",
		WorkerID:       worker.WorkerID,
		WorkerMode:     worker.Mode,
		WorkerEndpoint: worker.Endpoint,
		RunID:          runID,
		ExecutionID:    executionID,
		Tool:           tool,
		Capability:     capability,
		ToolPlan:       normalizeRuntimeWorkerToolPlanPayload(claim.ToolPlan, tool),
		ClaimedAt:      claimedAt,
		ExpiresAt:      now.Add(leaseDuration).Format(time.RFC3339Nano),
		LeaseSeconds:   int64(leaseDuration / time.Second),
	}
	claim.Path = filepath.Join(runtimeWorkerQueueClaimDir(root), claim.ClaimID+".json")
	if err := writeRuntimeWorkerQueueClaimArtifact(claim); err != nil {
		return RuntimeWorkerQueueClaim{}, err
	}
	return claim, nil
}

func ListRuntimeWorkerQueueClaims(root string) ([]RuntimeWorkerQueueClaim, error) {
	paths, err := filepath.Glob(filepath.Join(runtimeWorkerQueueClaimDir(root), "*.json"))
	if err != nil {
		return nil, fmt.Errorf("glob worker queue claims: %w", err)
	}
	sort.Strings(paths)
	claims := make([]RuntimeWorkerQueueClaim, 0, len(paths))
	for _, path := range paths {
		claim, err := readRuntimeWorkerQueueClaimFile(path)
		if err != nil {
			return nil, err
		}
		claims = append(claims, claim)
	}
	sort.SliceStable(claims, func(i int, j int) bool {
		if claims[i].ClaimedAt != claims[j].ClaimedAt {
			return claims[i].ClaimedAt < claims[j].ClaimedAt
		}
		if claims[i].WorkerID != claims[j].WorkerID {
			return claims[i].WorkerID < claims[j].WorkerID
		}
		return claims[i].Path < claims[j].Path
	})
	return claims, nil
}

func ReadRuntimeWorkerQueueClaim(root, claimPath string) (RuntimeWorkerQueueClaim, error) {
	path, err := resolveRuntimeWorkerQueueClaimPath(root, claimPath)
	if err != nil {
		return RuntimeWorkerQueueClaim{}, err
	}
	return readRuntimeWorkerQueueClaimFile(path)
}

func CancelRuntimeWorkerQueueClaim(root, claimPath string, options RuntimeWorkerQueueClaimCancelOptions) (RuntimeWorkerQueueClaim, error) {
	if !options.Cancel {
		return RuntimeWorkerQueueClaim{}, fmt.Errorf("worker queue claim cancel requires explicit cancel")
	}
	claim, err := ReadRuntimeWorkerQueueClaim(root, claimPath)
	if err != nil {
		return RuntimeWorkerQueueClaim{}, err
	}
	if strings.TrimSpace(claim.Status) != "claimed" {
		return RuntimeWorkerQueueClaim{}, fmt.Errorf("worker queue claim must be claimed before cancel: %s", claim.Status)
	}
	now := options.Now.UTC()
	if now.IsZero() {
		now = time.Now().UTC()
	}
	claim.Status = "canceled"
	claim.CanceledAt = now.Format(time.RFC3339Nano)
	claim.CanceledBy = strings.TrimSpace(options.CanceledBy)
	claim.CancelReason = strings.TrimSpace(options.Reason)
	if err := writeRuntimeWorkerQueueClaimArtifact(claim); err != nil {
		return RuntimeWorkerQueueClaim{}, err
	}
	return claim, nil
}

func ExpireRuntimeWorkerQueueClaim(root, claimPath string, options RuntimeWorkerQueueClaimExpireOptions) (RuntimeWorkerQueueClaim, error) {
	if !options.Expire {
		return RuntimeWorkerQueueClaim{}, fmt.Errorf("worker queue claim expire requires explicit expire")
	}
	claim, err := ReadRuntimeWorkerQueueClaim(root, claimPath)
	if err != nil {
		return RuntimeWorkerQueueClaim{}, err
	}
	if strings.TrimSpace(claim.Status) != "claimed" {
		return RuntimeWorkerQueueClaim{}, fmt.Errorf("worker queue claim must be claimed before expire: %s", claim.Status)
	}
	expiresAt := strings.TrimSpace(claim.ExpiresAt)
	if expiresAt == "" {
		return RuntimeWorkerQueueClaim{}, fmt.Errorf("worker queue claim expiry is required")
	}
	expiry, err := time.Parse(time.RFC3339Nano, expiresAt)
	if err != nil {
		return RuntimeWorkerQueueClaim{}, fmt.Errorf("worker queue claim expiry is invalid: %w", err)
	}
	now := options.Now.UTC()
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if !now.After(expiry) {
		return RuntimeWorkerQueueClaim{}, fmt.Errorf("worker queue claim has not expired: %s", claim.ExpiresAt)
	}
	claim.Status = "expired"
	claim.ExpiredAt = now.Format(time.RFC3339Nano)
	claim.ExpiredBy = strings.TrimSpace(options.ExpiredBy)
	claim.ExpireReason = strings.TrimSpace(options.Reason)
	if err := writeRuntimeWorkerQueueClaimArtifact(claim); err != nil {
		return RuntimeWorkerQueueClaim{}, err
	}
	return claim, nil
}

func RenewRuntimeWorkerQueueClaim(root, claimPath string, options RuntimeWorkerQueueClaimRenewOptions) (RuntimeWorkerQueueClaim, error) {
	if !options.Renew {
		return RuntimeWorkerQueueClaim{}, fmt.Errorf("worker queue claim renew requires explicit renew")
	}
	claim, err := ReadRuntimeWorkerQueueClaim(root, claimPath)
	if err != nil {
		return RuntimeWorkerQueueClaim{}, err
	}
	if strings.TrimSpace(claim.Status) != "claimed" {
		return RuntimeWorkerQueueClaim{}, fmt.Errorf("worker queue claim must be claimed before renew: %s", claim.Status)
	}
	leaseDuration := options.LeaseDuration
	if leaseDuration <= 0 {
		return RuntimeWorkerQueueClaim{}, fmt.Errorf("worker queue claim renewal lease duration is required")
	}
	expiresAt := strings.TrimSpace(claim.ExpiresAt)
	if expiresAt == "" {
		return RuntimeWorkerQueueClaim{}, fmt.Errorf("worker queue claim expiry is required")
	}
	expiry, err := time.Parse(time.RFC3339Nano, expiresAt)
	if err != nil {
		return RuntimeWorkerQueueClaim{}, fmt.Errorf("worker queue claim expiry is invalid: %w", err)
	}
	now := options.Now.UTC()
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if now.After(expiry) {
		return RuntimeWorkerQueueClaim{}, fmt.Errorf("worker queue claim has expired: %s", claim.ExpiresAt)
	}
	claim.RenewedAt = now.Format(time.RFC3339Nano)
	claim.RenewedBy = strings.TrimSpace(options.RenewedBy)
	claim.RenewReason = strings.TrimSpace(options.Reason)
	claim.LeaseSeconds = int64(leaseDuration / time.Second)
	claim.ExpiresAt = now.Add(leaseDuration).Format(time.RFC3339Nano)
	if err := writeRuntimeWorkerQueueClaimArtifact(claim); err != nil {
		return RuntimeWorkerQueueClaim{}, err
	}
	return claim, nil
}

func readRuntimeWorkerQueueClaimFile(path string) (RuntimeWorkerQueueClaim, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return RuntimeWorkerQueueClaim{}, fmt.Errorf("read worker queue claim %s: %w", path, err)
	}
	var claim RuntimeWorkerQueueClaim
	if err := json.Unmarshal(data, &claim); err != nil {
		return RuntimeWorkerQueueClaim{}, fmt.Errorf("decode worker queue claim %s: %w", path, err)
	}
	claim.Path = path
	return claim, nil
}

func writeRuntimeWorkerQueueClaimArtifact(claim RuntimeWorkerQueueClaim) error {
	if err := os.MkdirAll(filepath.Dir(claim.Path), 0o755); err != nil {
		return fmt.Errorf("create worker queue claim directory: %w", err)
	}
	data, err := json.MarshalIndent(claim, "", "  ")
	if err != nil {
		return fmt.Errorf("encode worker queue claim: %w", err)
	}
	data = append(data, '\n')
	tmp := claim.Path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("write worker queue claim: %w", err)
	}
	if err := os.Rename(tmp, claim.Path); err != nil {
		return fmt.Errorf("publish worker queue claim: %w", err)
	}
	return nil
}

func runtimeWorkerQueueClaimDir(root string) string {
	return filepath.Join(root, "runtime", "worker-queue-claims")
}

func resolveRuntimeWorkerQueueClaimPath(root, claimPath string) (string, error) {
	trimmed := strings.TrimSpace(claimPath)
	if trimmed == "" {
		return "", fmt.Errorf("worker queue claim path is required")
	}
	dir, err := filepath.Abs(runtimeWorkerQueueClaimDir(root))
	if err != nil {
		return "", fmt.Errorf("resolve worker queue claim directory: %w", err)
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
		return "", fmt.Errorf("resolve worker queue claim path: %w", err)
	}
	rel, err := filepath.Rel(dir, candidate)
	if err != nil {
		return "", fmt.Errorf("resolve worker queue claim relative path: %w", err)
	}
	if rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("worker queue claim path must stay under %s", dir)
	}
	if filepath.Ext(candidate) != ".json" {
		return "", fmt.Errorf("worker queue claim path must be a json file")
	}
	return candidate, nil
}

func runtimeWorkerHasCapability(worker RuntimeWorkerArtifact, capability string) bool {
	for _, advertised := range worker.Capabilities {
		if strings.TrimSpace(advertised) == capability {
			return true
		}
	}
	return false
}

func safeRuntimeWorkerQueueClaimID(workerID, runID, executionID, capability, claimedAt string) string {
	return safeInstallRequestName(strings.Join([]string{
		"worker-queue-claim",
		workerID,
		runID,
		executionID,
		capability,
		claimedAt,
	}, "|"))
}
