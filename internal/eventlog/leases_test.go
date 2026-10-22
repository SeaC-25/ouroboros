package eventlog

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

// S4 idempotency spine tests: four-phase lifecycle, duplicate-claim rejection,
// poison-pill quarantine, orphan recovery.

func newTestLeaseStore(t *testing.T) (*SQLiteStore, *LeaseStore) {
	t.Helper()
	dir := t.TempDir()
	store, err := OpenSQLite(filepath.Join(dir, "events.db"))
	if err != nil {
		t.Fatalf("open eventlog store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	leases, err := NewLeaseStore(store)
	if err != nil {
		t.Fatalf("new lease store: %v", err)
	}
	return store, leases
}

func TestLeaseFourPhaseLifecycle(t *testing.T) {
	_, leases := newTestLeaseStore(t)
	ctx := context.Background()
	key := IdempotencyKey("http://target/", "http_probe", "GET")

	if err := leases.Claim(ctx, ExecutionLease{Key: key, EntityID: "e1", ToolName: "http_probe", WorkerID: "w1", TimeoutMS: 60000}); err != nil {
		t.Fatalf("claim: %v", err)
	}
	for _, phase := range []LeaseStatus{LeaseRunning, LeaseArtifactSaved, LeaseCommitted} {
		path := ""
		if phase == LeaseArtifactSaved {
			path = "/tmp/artifact.json"
		}
		if err := leases.Transition(ctx, key, phase, path); err != nil {
			t.Fatalf("transition to %s: %v", phase, err)
		}
	}
}

func TestLeaseDuplicateClaimRejected(t *testing.T) {
	_, leases := newTestLeaseStore(t)
	ctx := context.Background()
	key := IdempotencyKey("http://target/", "path_fuzz", "fast")

	if err := leases.Claim(ctx, ExecutionLease{Key: key, EntityID: "e1", ToolName: "path_fuzz", WorkerID: "w1", TimeoutMS: 60000}); err != nil {
		t.Fatalf("first claim: %v", err)
	}
	err := leases.Claim(ctx, ExecutionLease{Key: key, EntityID: "e1", ToolName: "path_fuzz", WorkerID: "w2", TimeoutMS: 60000})
	if err != ErrDuplicateClaim {
		t.Fatalf("second claim must be ErrDuplicateClaim, got: %v", err)
	}
}

func TestLeaseRetryAfterFailure(t *testing.T) {
	_, leases := newTestLeaseStore(t)
	ctx := context.Background()
	key := IdempotencyKey("http://target/", "port_scan", "tcp")

	_ = leases.Claim(ctx, ExecutionLease{Key: key, EntityID: "e1", ToolName: "port_scan", WorkerID: "w1", TimeoutMS: 60000})
	if err := leases.Transition(ctx, key, LeaseRunning, ""); err != nil {
		t.Fatalf("running: %v", err)
	}
	if err := leases.Transition(ctx, key, LeaseFailed, ""); err != nil {
		t.Fatalf("failed: %v", err)
	}
	// Retry after failure must succeed (attempt 2).
	if err := leases.Claim(ctx, ExecutionLease{Key: key, EntityID: "e1", ToolName: "port_scan", WorkerID: "w1", TimeoutMS: 60000}); err != nil {
		t.Fatalf("retry claim after failure: %v", err)
	}
}

func TestLeasePoisonPillQuarantine(t *testing.T) {
	_, leases := newTestLeaseStore(t)
	ctx := context.Background()
	key := IdempotencyKey("http://evil-crash-target/", "apk_jadx", "blob")

	// Two failed attempts → third claim must be quarantined.
	for i := range 2 {
		if err := leases.Claim(ctx, ExecutionLease{Key: key, EntityID: "e1", ToolName: "apk_jadx", WorkerID: "w1", TimeoutMS: 60000}); err != nil {
			t.Fatalf("claim %d: %v", i+1, err)
		}
		if err := leases.Transition(ctx, key, LeaseFailed, ""); err != nil {
			t.Fatalf("fail %d: %v", i+1, err)
		}
	}
	err := leases.Claim(ctx, ExecutionLease{Key: key, EntityID: "e1", ToolName: "apk_jadx", WorkerID: "w1", TimeoutMS: 60000})
	if err != ErrPoisonPill {
		t.Fatalf("third claim must be ErrPoisonPill, got: %v", err)
	}
}

func TestLeaseOrphanRecovery(t *testing.T) {
	_, leases := newTestLeaseStore(t)
	ctx := context.Background()

	// Live lease (fresh heartbeat) — must NOT be recovered.
	liveKey := IdempotencyKey("http://alive/", "http_probe", "a")
	_ = leases.Claim(ctx, ExecutionLease{Key: liveKey, EntityID: "e1", ToolName: "http_probe", WorkerID: "w1", TimeoutMS: 600000})
	_ = leases.Transition(ctx, liveKey, LeaseRunning, "")

	// Orphan lease (TTL expired) — must be requeued.
	orphanKey := IdempotencyKey("http://dead/", "http_probe", "b")
	_ = leases.Claim(ctx, ExecutionLease{Key: orphanKey, EntityID: "e2", ToolName: "http_probe", WorkerID: "w-dead", TimeoutMS: 1000})
	_ = leases.Transition(ctx, orphanKey, LeaseRunning, "")

	// now = heartbeat + 5s → orphan's 1s TTL expired, live's 600s TTL fine.
	nowMS := timeNowMS() + 5000
	requeue, quarantined, err := leases.RecoverOrphans(ctx, nowMS)
	if err != nil {
		t.Fatalf("recover orphans: %v", err)
	}
	if len(quarantined) != 0 {
		t.Fatalf("expected no quarantined, got %d", len(quarantined))
	}
	if len(requeue) != 1 || requeue[0].Key != orphanKey {
		t.Fatalf("expected exactly the orphan key in requeue, got %+v", requeue)
	}
	if requeue[0].ToolName != "http_probe" {
		t.Fatalf("orphan lease must carry tool name, got %q", requeue[0].ToolName)
	}
}

func timeNowMS() int64 {
	return time.Now().UnixMilli()
}
