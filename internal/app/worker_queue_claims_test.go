package app

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCreateRuntimeWorkerQueueClaimWritesMetadataOnlyArtifact(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	now := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)
	worker, err := RegisterRuntimeWorker(root, RuntimeWorkerArtifact{
		WorkerID:     "remote-web-1",
		Mode:         "remote",
		Status:       "available",
		Endpoint:     "tcp://127.0.0.1:9443",
		Capabilities: []string{"web.discovery", "web.enumeration"},
	}, RuntimeWorkerRegistrationOptions{Now: now})
	if err != nil {
		t.Fatalf("register runtime worker: %v", err)
	}

	claim, err := CreateRuntimeWorkerQueueClaim(root, RuntimeWorkerQueueClaim{
		WorkerID:    "remote-web-1",
		RunID:       "run-queue-1",
		ExecutionID: "tool-web",
		Tool:        "web-api-pack",
		Capability:  "web.discovery",
	}, RuntimeWorkerQueueClaimOptions{Now: now, LeaseDuration: 10 * time.Minute})
	if err != nil {
		t.Fatalf("create runtime worker queue claim: %v", err)
	}
	if claim.Version != 1 || claim.Status != "claimed" || claim.Mode != "metadata-only" {
		t.Fatalf("unexpected queue claim: %#v", claim)
	}
	if claim.WorkerID != "remote-web-1" || claim.WorkerMode != "remote" || claim.WorkerEndpoint != "tcp://127.0.0.1:9443" {
		t.Fatalf("expected worker metadata in claim, got %#v", claim)
	}
	if claim.ClaimedAt != now.Format(time.RFC3339Nano) || claim.ExpiresAt != now.Add(10*time.Minute).Format(time.RFC3339Nano) {
		t.Fatalf("expected deterministic claim timestamps, got %#v", claim)
	}
	if !strings.Contains(claim.Path, filepath.Join("runtime", "worker-queue-claims")) {
		t.Fatalf("expected claim under runtime/worker-queue-claims, got %s", claim.Path)
	}

	claims, err := ListRuntimeWorkerQueueClaims(root)
	if err != nil {
		t.Fatalf("list queue claims: %v", err)
	}
	if len(claims) != 1 || claims[0].Path != claim.Path || claims[0].ClaimID == "" {
		t.Fatalf("expected listed queue claim, got %#v", claims)
	}
	read, err := ReadRuntimeWorkerQueueClaim(root, filepath.Base(claim.Path))
	if err != nil {
		t.Fatalf("read queue claim: %v", err)
	}
	if read.ClaimID != claim.ClaimID || read.Capability != "web.discovery" {
		t.Fatalf("expected read queue claim, got %#v", read)
	}
	unchanged, err := ReadRuntimeWorker(root, filepath.Base(worker.Path))
	if err != nil {
		t.Fatalf("read runtime worker: %v", err)
	}
	if unchanged.Status != "available" {
		t.Fatalf("metadata-only queue claim must not mutate worker status, got %#v", unchanged)
	}
}

func TestCreateRuntimeWorkerQueueClaimRejectsStaleWorkerAndCapabilityMismatch(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	now := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)
	old := now.Add(-DefaultRuntimeWorkerStaleAfter - time.Minute)
	if _, err := RegisterRuntimeWorker(root, RuntimeWorkerArtifact{
		WorkerID:     "remote-stale",
		Mode:         "remote",
		Status:       "available",
		Capabilities: []string{"web.discovery"},
	}, RuntimeWorkerRegistrationOptions{Now: old}); err != nil {
		t.Fatalf("register stale worker: %v", err)
	}
	if _, err := CreateRuntimeWorkerQueueClaim(root, RuntimeWorkerQueueClaim{
		WorkerID:    "remote-stale",
		RunID:       "run-queue-1",
		ExecutionID: "tool-web",
		Tool:        "web-api-pack",
		Capability:  "web.discovery",
	}, RuntimeWorkerQueueClaimOptions{Now: now}); err == nil || !strings.Contains(err.Error(), "runtime worker is stale") {
		t.Fatalf("expected stale worker rejection, got %v", err)
	}

	if _, err := RegisterRuntimeWorker(root, RuntimeWorkerArtifact{
		WorkerID:     "remote-web-1",
		Mode:         "remote",
		Status:       "available",
		Capabilities: []string{"web.discovery"},
	}, RuntimeWorkerRegistrationOptions{Now: now}); err != nil {
		t.Fatalf("register runtime worker: %v", err)
	}
	if _, err := CreateRuntimeWorkerQueueClaim(root, RuntimeWorkerQueueClaim{
		WorkerID:    "remote-web-1",
		RunID:       "run-queue-2",
		ExecutionID: "tool-infra",
		Tool:        "infra-pack",
		Capability:  "infra.host_discovery",
	}, RuntimeWorkerQueueClaimOptions{Now: now}); err == nil || !strings.Contains(err.Error(), "does not advertise capability") {
		t.Fatalf("expected capability mismatch rejection, got %v", err)
	}
}

func TestReadRuntimeWorkerQueueClaimRejectsPathOutsideClaimDir(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}

	if _, err := ReadRuntimeWorkerQueueClaim(root, filepath.Join("..", "claim.json")); err == nil || !strings.Contains(err.Error(), "worker queue claim path must stay under") {
		t.Fatalf("expected path confinement error, got %v", err)
	}
}

func TestCancelRuntimeWorkerQueueClaimRequiresExplicitCancel(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	now := time.Date(2026, 5, 30, 18, 0, 0, 0, time.UTC)
	if _, err := RegisterRuntimeWorker(root, RuntimeWorkerArtifact{
		WorkerID:     "remote-web-1",
		Mode:         "remote",
		Status:       "available",
		Capabilities: []string{"web.discovery"},
	}, RuntimeWorkerRegistrationOptions{Now: now}); err != nil {
		t.Fatalf("register runtime worker: %v", err)
	}
	claim, err := CreateRuntimeWorkerQueueClaim(root, RuntimeWorkerQueueClaim{
		WorkerID:    "remote-web-1",
		RunID:       "run-cancel-1",
		ExecutionID: "tool-web",
		Tool:        "web-api-pack",
		Capability:  "web.discovery",
	}, RuntimeWorkerQueueClaimOptions{Now: now, LeaseDuration: 10 * time.Minute})
	if err != nil {
		t.Fatalf("create runtime worker queue claim: %v", err)
	}

	_, err = CancelRuntimeWorkerQueueClaim(root, claim.Path, RuntimeWorkerQueueClaimCancelOptions{
		Now: now.Add(time.Minute),
	})
	if err == nil || !strings.Contains(err.Error(), "requires explicit cancel") {
		t.Fatalf("expected explicit cancel error, got %v", err)
	}
	read, err := ReadRuntimeWorkerQueueClaim(root, claim.Path)
	if err != nil {
		t.Fatalf("read queue claim: %v", err)
	}
	if read.Status != "claimed" || read.CanceledAt != "" || read.CanceledBy != "" || read.CancelReason != "" {
		t.Fatalf("cancel without explicit flag must not mutate claim, got %#v", read)
	}
	dispatches, err := ListRuntimeWorkerQueueDispatchExecutions(root)
	if err != nil {
		t.Fatalf("list queue dispatch executions: %v", err)
	}
	if len(dispatches) != 0 {
		t.Fatalf("cancel without explicit flag must not create dispatch audits, got %#v", dispatches)
	}
}

func TestCancelRuntimeWorkerQueueClaimMarksClaimedArtifactCanceled(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	now := time.Date(2026, 5, 30, 18, 30, 0, 0, time.UTC)
	if _, err := RegisterRuntimeWorker(root, RuntimeWorkerArtifact{
		WorkerID:     "remote-web-1",
		Mode:         "remote",
		Status:       "available",
		Capabilities: []string{"web.discovery"},
	}, RuntimeWorkerRegistrationOptions{Now: now}); err != nil {
		t.Fatalf("register runtime worker: %v", err)
	}
	claim, err := CreateRuntimeWorkerQueueClaim(root, RuntimeWorkerQueueClaim{
		WorkerID:    "remote-web-1",
		RunID:       "run-cancel-2",
		ExecutionID: "tool-web",
		Tool:        "web-api-pack",
		Capability:  "web.discovery",
	}, RuntimeWorkerQueueClaimOptions{Now: now, LeaseDuration: 10 * time.Minute})
	if err != nil {
		t.Fatalf("create runtime worker queue claim: %v", err)
	}

	canceled, err := CancelRuntimeWorkerQueueClaim(root, filepath.Base(claim.Path), RuntimeWorkerQueueClaimCancelOptions{
		Cancel:     true,
		Now:        now.Add(2 * time.Minute),
		CanceledBy: "operator",
		Reason:     "release stale manual assignment",
	})
	if err != nil {
		t.Fatalf("cancel runtime worker queue claim: %v", err)
	}
	if canceled.Status != "canceled" || canceled.CanceledAt != now.Add(2*time.Minute).Format(time.RFC3339Nano) {
		t.Fatalf("expected canceled claim timestamps, got %#v", canceled)
	}
	if canceled.CanceledBy != "operator" || canceled.CancelReason != "release stale manual assignment" {
		t.Fatalf("expected cancel audit metadata, got %#v", canceled)
	}
	if canceled.DispatchedAt != "" || canceled.DispatchExecutionPath != "" {
		t.Fatalf("cancel must not dispatch claim, got %#v", canceled)
	}
	read, err := ReadRuntimeWorkerQueueClaim(root, claim.Path)
	if err != nil {
		t.Fatalf("read canceled queue claim: %v", err)
	}
	if read.Status != "canceled" || read.CanceledAt != canceled.CanceledAt || read.CanceledBy != "operator" {
		t.Fatalf("expected persisted canceled claim, got %#v", read)
	}
	dispatches, err := ListRuntimeWorkerQueueDispatchExecutions(root)
	if err != nil {
		t.Fatalf("list queue dispatch executions: %v", err)
	}
	if len(dispatches) != 0 {
		t.Fatalf("cancel must not create dispatch audit artifacts, got %#v", dispatches)
	}
}

func TestCancelRuntimeWorkerQueueClaimRejectsAlreadyDispatchedClaim(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	now := time.Date(2026, 5, 30, 19, 0, 0, 0, time.UTC)
	if _, err := RegisterRuntimeWorker(root, RuntimeWorkerArtifact{
		WorkerID:     "remote-web-1",
		Mode:         "remote",
		Status:       "available",
		Capabilities: []string{"web.discovery"},
	}, RuntimeWorkerRegistrationOptions{Now: now}); err != nil {
		t.Fatalf("register runtime worker: %v", err)
	}
	claim, err := CreateRuntimeWorkerQueueClaim(root, RuntimeWorkerQueueClaim{
		WorkerID:    "remote-web-1",
		RunID:       "run-cancel-3",
		ExecutionID: "tool-web",
		Tool:        "web-api-pack",
		Capability:  "web.discovery",
	}, RuntimeWorkerQueueClaimOptions{Now: now, LeaseDuration: 10 * time.Minute})
	if err != nil {
		t.Fatalf("create runtime worker queue claim: %v", err)
	}
	if _, err := ExecuteRuntimeWorkerQueueClaimDispatch(root, claim.Path, RuntimeWorkerQueueDispatchExecutionOptions{Execute: true, Now: now.Add(time.Minute)}); err != nil {
		t.Fatalf("dispatch queue claim: %v", err)
	}

	_, err = CancelRuntimeWorkerQueueClaim(root, claim.Path, RuntimeWorkerQueueClaimCancelOptions{
		Cancel: true,
		Now:    now.Add(2 * time.Minute),
	})
	if err == nil || !strings.Contains(err.Error(), "must be claimed before cancel") {
		t.Fatalf("expected dispatched claim rejection, got %v", err)
	}
	read, err := ReadRuntimeWorkerQueueClaim(root, claim.Path)
	if err != nil {
		t.Fatalf("read queue claim: %v", err)
	}
	if read.Status != "dispatched" || read.CanceledAt != "" {
		t.Fatalf("dispatched claim must remain dispatched, got %#v", read)
	}
}

func TestExpireRuntimeWorkerQueueClaimRequiresExplicitExpire(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	now := time.Date(2026, 5, 30, 20, 0, 0, 0, time.UTC)
	if _, err := RegisterRuntimeWorker(root, RuntimeWorkerArtifact{
		WorkerID:     "remote-web-1",
		Mode:         "remote",
		Status:       "available",
		Capabilities: []string{"web.discovery"},
	}, RuntimeWorkerRegistrationOptions{Now: now}); err != nil {
		t.Fatalf("register runtime worker: %v", err)
	}
	claim, err := CreateRuntimeWorkerQueueClaim(root, RuntimeWorkerQueueClaim{
		WorkerID:    "remote-web-1",
		RunID:       "run-expire-1",
		ExecutionID: "tool-web",
		Tool:        "web-api-pack",
		Capability:  "web.discovery",
	}, RuntimeWorkerQueueClaimOptions{Now: now, LeaseDuration: time.Minute})
	if err != nil {
		t.Fatalf("create runtime worker queue claim: %v", err)
	}

	_, err = ExpireRuntimeWorkerQueueClaim(root, claim.Path, RuntimeWorkerQueueClaimExpireOptions{
		Now: now.Add(2 * time.Minute),
	})
	if err == nil || !strings.Contains(err.Error(), "requires explicit expire") {
		t.Fatalf("expected explicit expire error, got %v", err)
	}
	read, err := ReadRuntimeWorkerQueueClaim(root, claim.Path)
	if err != nil {
		t.Fatalf("read queue claim: %v", err)
	}
	if read.Status != "claimed" || read.ExpiredAt != "" || read.ExpiredBy != "" || read.ExpireReason != "" {
		t.Fatalf("expire without explicit flag must not mutate claim, got %#v", read)
	}
	dispatches, err := ListRuntimeWorkerQueueDispatchExecutions(root)
	if err != nil {
		t.Fatalf("list queue dispatch executions: %v", err)
	}
	if len(dispatches) != 0 {
		t.Fatalf("expire without explicit flag must not create dispatch audits, got %#v", dispatches)
	}
}

func TestExpireRuntimeWorkerQueueClaimMarksExpiredClaim(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	now := time.Date(2026, 5, 30, 20, 30, 0, 0, time.UTC)
	if _, err := RegisterRuntimeWorker(root, RuntimeWorkerArtifact{
		WorkerID:     "remote-web-1",
		Mode:         "remote",
		Status:       "available",
		Capabilities: []string{"web.discovery"},
	}, RuntimeWorkerRegistrationOptions{Now: now}); err != nil {
		t.Fatalf("register runtime worker: %v", err)
	}
	claim, err := CreateRuntimeWorkerQueueClaim(root, RuntimeWorkerQueueClaim{
		WorkerID:    "remote-web-1",
		RunID:       "run-expire-2",
		ExecutionID: "tool-web",
		Tool:        "web-api-pack",
		Capability:  "web.discovery",
	}, RuntimeWorkerQueueClaimOptions{Now: now, LeaseDuration: time.Minute})
	if err != nil {
		t.Fatalf("create runtime worker queue claim: %v", err)
	}

	expired, err := ExpireRuntimeWorkerQueueClaim(root, filepath.Base(claim.Path), RuntimeWorkerQueueClaimExpireOptions{
		Expire:    true,
		Now:       now.Add(2 * time.Minute),
		ExpiredBy: "operator",
		Reason:    "lease elapsed before worker ack",
	})
	if err != nil {
		t.Fatalf("expire runtime worker queue claim: %v", err)
	}
	if expired.Status != "expired" || expired.ExpiredAt != now.Add(2*time.Minute).Format(time.RFC3339Nano) {
		t.Fatalf("expected expired claim timestamps, got %#v", expired)
	}
	if expired.ExpiredBy != "operator" || expired.ExpireReason != "lease elapsed before worker ack" {
		t.Fatalf("expected expire audit metadata, got %#v", expired)
	}
	if expired.DispatchedAt != "" || expired.DispatchExecutionPath != "" {
		t.Fatalf("expire must not dispatch claim, got %#v", expired)
	}
	read, err := ReadRuntimeWorkerQueueClaim(root, claim.Path)
	if err != nil {
		t.Fatalf("read expired queue claim: %v", err)
	}
	if read.Status != "expired" || read.ExpiredAt != expired.ExpiredAt || read.ExpiredBy != "operator" {
		t.Fatalf("expected persisted expired claim, got %#v", read)
	}
	dispatches, err := ListRuntimeWorkerQueueDispatchExecutions(root)
	if err != nil {
		t.Fatalf("list queue dispatch executions: %v", err)
	}
	if len(dispatches) != 0 {
		t.Fatalf("expire must not create dispatch audit artifacts, got %#v", dispatches)
	}
}

func TestExpireRuntimeWorkerQueueClaimRejectsFreshAndDispatchedClaims(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	now := time.Date(2026, 5, 30, 21, 0, 0, 0, time.UTC)
	if _, err := RegisterRuntimeWorker(root, RuntimeWorkerArtifact{
		WorkerID:     "remote-web-1",
		Mode:         "remote",
		Status:       "available",
		Capabilities: []string{"web.discovery"},
	}, RuntimeWorkerRegistrationOptions{Now: now}); err != nil {
		t.Fatalf("register runtime worker: %v", err)
	}
	fresh, err := CreateRuntimeWorkerQueueClaim(root, RuntimeWorkerQueueClaim{
		WorkerID:    "remote-web-1",
		RunID:       "run-expire-3",
		ExecutionID: "tool-fresh",
		Tool:        "web-api-pack",
		Capability:  "web.discovery",
	}, RuntimeWorkerQueueClaimOptions{Now: now, LeaseDuration: 10 * time.Minute})
	if err != nil {
		t.Fatalf("create fresh queue claim: %v", err)
	}
	_, err = ExpireRuntimeWorkerQueueClaim(root, fresh.Path, RuntimeWorkerQueueClaimExpireOptions{
		Expire: true,
		Now:    now.Add(time.Minute),
	})
	if err == nil || !strings.Contains(err.Error(), "has not expired") {
		t.Fatalf("expected fresh claim rejection, got %v", err)
	}

	dispatched, err := CreateRuntimeWorkerQueueClaim(root, RuntimeWorkerQueueClaim{
		WorkerID:    "remote-web-1",
		RunID:       "run-expire-4",
		ExecutionID: "tool-dispatched",
		Tool:        "web-api-pack",
		Capability:  "web.discovery",
	}, RuntimeWorkerQueueClaimOptions{Now: now, LeaseDuration: time.Minute})
	if err != nil {
		t.Fatalf("create dispatched queue claim: %v", err)
	}
	if _, err := ExecuteRuntimeWorkerQueueClaimDispatch(root, dispatched.Path, RuntimeWorkerQueueDispatchExecutionOptions{Execute: true, Now: now.Add(30 * time.Second)}); err != nil {
		t.Fatalf("dispatch queue claim: %v", err)
	}
	_, err = ExpireRuntimeWorkerQueueClaim(root, dispatched.Path, RuntimeWorkerQueueClaimExpireOptions{
		Expire: true,
		Now:    now.Add(2 * time.Minute),
	})
	if err == nil || !strings.Contains(err.Error(), "must be claimed before expire") {
		t.Fatalf("expected dispatched claim rejection, got %v", err)
	}
	read, err := ReadRuntimeWorkerQueueClaim(root, dispatched.Path)
	if err != nil {
		t.Fatalf("read dispatched claim: %v", err)
	}
	if read.Status != "dispatched" || read.ExpiredAt != "" {
		t.Fatalf("dispatched claim must remain dispatched, got %#v", read)
	}
}

func TestRenewRuntimeWorkerQueueClaimRequiresExplicitRenew(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	now := time.Date(2026, 5, 30, 21, 30, 0, 0, time.UTC)
	if _, err := RegisterRuntimeWorker(root, RuntimeWorkerArtifact{
		WorkerID:     "remote-web-1",
		Mode:         "remote",
		Status:       "available",
		Capabilities: []string{"web.discovery"},
	}, RuntimeWorkerRegistrationOptions{Now: now}); err != nil {
		t.Fatalf("register runtime worker: %v", err)
	}
	claim, err := CreateRuntimeWorkerQueueClaim(root, RuntimeWorkerQueueClaim{
		WorkerID:    "remote-web-1",
		RunID:       "run-renew-1",
		ExecutionID: "tool-web",
		Tool:        "web-api-pack",
		Capability:  "web.discovery",
	}, RuntimeWorkerQueueClaimOptions{Now: now, LeaseDuration: 5 * time.Minute})
	if err != nil {
		t.Fatalf("create runtime worker queue claim: %v", err)
	}

	_, err = RenewRuntimeWorkerQueueClaim(root, claim.Path, RuntimeWorkerQueueClaimRenewOptions{
		Now:           now.Add(2 * time.Minute),
		LeaseDuration: 10 * time.Minute,
	})
	if err == nil || !strings.Contains(err.Error(), "requires explicit renew") {
		t.Fatalf("expected explicit renew error, got %v", err)
	}
	read, err := ReadRuntimeWorkerQueueClaim(root, claim.Path)
	if err != nil {
		t.Fatalf("read queue claim: %v", err)
	}
	if read.Status != "claimed" || read.RenewedAt != "" || read.RenewedBy != "" || read.RenewReason != "" {
		t.Fatalf("renew without explicit flag must not mutate claim, got %#v", read)
	}
}

func TestRenewRuntimeWorkerQueueClaimExtendsClaimedArtifact(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	now := time.Date(2026, 5, 30, 22, 0, 0, 0, time.UTC)
	if _, err := RegisterRuntimeWorker(root, RuntimeWorkerArtifact{
		WorkerID:     "remote-web-1",
		Mode:         "remote",
		Status:       "available",
		Capabilities: []string{"web.discovery"},
	}, RuntimeWorkerRegistrationOptions{Now: now}); err != nil {
		t.Fatalf("register runtime worker: %v", err)
	}
	claim, err := CreateRuntimeWorkerQueueClaim(root, RuntimeWorkerQueueClaim{
		WorkerID:    "remote-web-1",
		RunID:       "run-renew-2",
		ExecutionID: "tool-web",
		Tool:        "web-api-pack",
		Capability:  "web.discovery",
	}, RuntimeWorkerQueueClaimOptions{Now: now, LeaseDuration: 5 * time.Minute})
	if err != nil {
		t.Fatalf("create runtime worker queue claim: %v", err)
	}

	renewed, err := RenewRuntimeWorkerQueueClaim(root, filepath.Base(claim.Path), RuntimeWorkerQueueClaimRenewOptions{
		Renew:         true,
		Now:           now.Add(2 * time.Minute),
		LeaseDuration: 10 * time.Minute,
		RenewedBy:     "operator",
		Reason:        "keep active worker assignment alive",
	})
	if err != nil {
		t.Fatalf("renew runtime worker queue claim: %v", err)
	}
	if renewed.Status != "claimed" {
		t.Fatalf("renew must keep claim claimed, got %#v", renewed)
	}
	if renewed.RenewedAt != now.Add(2*time.Minute).Format(time.RFC3339Nano) {
		t.Fatalf("expected renewal timestamp, got %#v", renewed)
	}
	if renewed.RenewedBy != "operator" || renewed.RenewReason != "keep active worker assignment alive" {
		t.Fatalf("expected renewal audit metadata, got %#v", renewed)
	}
	if renewed.LeaseSeconds != int64((10 * time.Minute) / time.Second) {
		t.Fatalf("expected updated lease seconds, got %#v", renewed)
	}
	if renewed.ExpiresAt != now.Add(12*time.Minute).Format(time.RFC3339Nano) {
		t.Fatalf("expected renewed expiry, got %#v", renewed)
	}
	read, err := ReadRuntimeWorkerQueueClaim(root, claim.Path)
	if err != nil {
		t.Fatalf("read renewed queue claim: %v", err)
	}
	if read.RenewedAt != renewed.RenewedAt || read.RenewedBy != "operator" || read.RenewReason != renewed.RenewReason {
		t.Fatalf("expected persisted renewal metadata, got %#v", read)
	}
	dispatches, err := ListRuntimeWorkerQueueDispatchExecutions(root)
	if err != nil {
		t.Fatalf("list queue dispatch executions: %v", err)
	}
	if len(dispatches) != 0 {
		t.Fatalf("renew must not create dispatch audit artifacts, got %#v", dispatches)
	}
}
