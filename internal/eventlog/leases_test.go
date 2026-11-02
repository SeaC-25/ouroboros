package eventlog

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
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
	if !errors.Is(err, ErrPoisonPill) {
		t.Fatalf("third claim must be ErrPoisonPill, got: %v", err)
	}
}

// TestLeasePoisonPillAfterTwoOrphanRecoveries reproduces the L0 poison-pill
// rule: the same idempotency key crashing twice (each crash surfaced as an
// orphaned lease reclaimed by RecoverOrphans) must make the third Claim
// return ErrPoisonPill carrying the key and a switch-tool advice, while
// unrelated keys stay fully claimable (no cross-key contagion).
func TestLeasePoisonPillAfterTwoOrphanRecoveries(t *testing.T) {
	_, leases := newTestLeaseStore(t)
	ctx := context.Background()
	key := IdempotencyKey("http://crash-twice/", "port_scan", "tcp")

	for i := range 2 {
		if err := leases.Claim(ctx, ExecutionLease{Key: key, EntityID: "e1", ToolName: "port_scan", WorkerID: "w1", TimeoutMS: 30}); err != nil {
			t.Fatalf("claim %d: %v", i+1, err)
		}
		if err := leases.Transition(ctx, key, LeaseRunning, ""); err != nil {
			t.Fatalf("running %d: %v", i+1, err)
		}
		// The worker "crashed": its heartbeat goes silent past the 30ms TTL.
		time.Sleep(80 * time.Millisecond)
		requeue, quarantined, err := leases.RecoverOrphans(ctx, timeNowMS()+1000)
		if err != nil {
			t.Fatalf("recover %d: %v", i+1, err)
		}
		recovered := 0
		for _, o := range requeue {
			if o.Key == key {
				recovered++
			}
		}
		for _, o := range quarantined {
			if o.Key == key {
				recovered++
			}
		}
		if recovered != 1 {
			t.Fatalf("recovery %d must reclaim exactly the crashed key, got requeue=%+v quarantined=%+v", i+1, requeue, quarantined)
		}
		// The reclaim must have recorded the crash on the row.
		var crashes int
		if err := leases.db.QueryRowContext(ctx,
			`SELECT crash_count FROM execution_leases WHERE idempotency_key=?`, key).Scan(&crashes); err != nil {
			t.Fatalf("read crash_count after recovery %d: %v", i+1, err)
		}
		if crashes != i+1 {
			t.Fatalf("crash_count must be %d after recovery %d, got %d", i+1, i+1, crashes)
		}
		// Different key must not be affected by the poisoned one.
		otherKey := IdempotencyKey(fmt.Sprintf("http://healthy-%d/", i), "dir_fuzz", "fast")
		if err := leases.Claim(ctx, ExecutionLease{Key: otherKey, EntityID: "e2", ToolName: "dir_fuzz", WorkerID: "w2", TimeoutMS: 600000}); err != nil {
			t.Fatalf("unrelated key claim after recovery %d: %v", i+1, err)
		}
	}

	err := leases.Claim(ctx, ExecutionLease{Key: key, EntityID: "e1", ToolName: "port_scan", WorkerID: "w1", TimeoutMS: 60000})
	if !errors.Is(err, ErrPoisonPill) {
		t.Fatalf("third claim after two crash recoveries must be ErrPoisonPill, got: %v", err)
	}
	if !strings.Contains(err.Error(), key) || !strings.Contains(err.Error(), "tool") {
		t.Fatalf("ErrPoisonPill must carry the key and switch-tool advice, got: %v", err)
	}
}

// TestLeaseCrashCountMigrationOnLegacySchema builds a pre-L0
// execution_leases table without the crash_count column and verifies
// NewLeaseStore migrates it (PRAGMA table_info check + ALTER TABLE), with
// legacy rows defaulting to 0 crashes.
func TestLeaseCrashCountMigrationOnLegacySchema(t *testing.T) {
	store, err := OpenSQLite(filepath.Join(t.TempDir(), "events.db"))
	if err != nil {
		t.Fatalf("open eventlog store: %v", err)
	}
	defer func() { _ = store.Close() }()

	// Legacy schema: exactly the old CREATE TABLE, no crash_count.
	legacySQL := `CREATE TABLE IF NOT EXISTS execution_leases (
		idempotency_key TEXT PRIMARY KEY,
		entity_id TEXT NOT NULL,
		tool_name TEXT NOT NULL,
		status TEXT NOT NULL CHECK(status IN ('CLAIMED','RUNNING','ARTIFACT_SAVED','COMMITTED','FAILED','ABORTED')),
		worker_id TEXT NOT NULL,
		heartbeat_at INTEGER NOT NULL,
		timeout_ms INTEGER NOT NULL,
		attempt_count INTEGER NOT NULL DEFAULT 1,
		raw_artifact_path TEXT,
		created_at TEXT NOT NULL
	);`
	if _, err := store.db.Exec(legacySQL); err != nil {
		t.Fatalf("create legacy execution_leases: %v", err)
	}
	// Sanity: the fixture really lacks the column before migrating.
	var missing int
	if err := store.db.QueryRow(
		`SELECT COUNT(*) FROM pragma_table_info('execution_leases') WHERE name='crash_count'`).Scan(&missing); err != nil {
		t.Fatalf("inspect legacy schema: %v", err)
	}
	if missing != 0 {
		t.Fatalf("legacy fixture must lack crash_count before migration")
	}

	if _, err := NewLeaseStore(store); err != nil {
		t.Fatalf("NewLeaseStore must migrate legacy schema: %v", err)
	}

	// The column now exists: a legacy-shaped INSERT (no crash_count) works
	// and the migrated row defaults to 0.
	if _, err := store.db.Exec(`INSERT INTO execution_leases
		(idempotency_key, entity_id, tool_name, status, worker_id, heartbeat_at, timeout_ms, attempt_count, raw_artifact_path, created_at)
		VALUES ('legacy-key', 'e1', 'port_scan', 'FAILED', 'w0', 1, 1, 1, NULL, '2020-01-01T00:00:00Z')`); err != nil {
		t.Fatalf("legacy-shaped insert after migration must succeed: %v", err)
	}
	var crashes int
	if err := store.db.QueryRow(
		`SELECT crash_count FROM execution_leases WHERE idempotency_key='legacy-key'`).Scan(&crashes); err != nil {
		t.Fatalf("read crash_count from migrated legacy row: %v", err)
	}
	if crashes != 0 {
		t.Fatalf("migrated legacy row must default crash_count=0, got %d", crashes)
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

// TestLeaseConcurrentRetryClaimSingleWinner drives the Claim retry path from
// many goroutines at once: exactly one may flip the FAILED row back to
// CLAIMED; every other racer must observe ErrDuplicateClaim (or ErrPoisonPill
// if it arrived after attempts hit the threshold) so two workers never
// double-execute the same idempotency key via the retry path.
func TestLeaseConcurrentRetryClaimSingleWinner(t *testing.T) {
	_, leases := newTestLeaseStore(t)
	ctx := context.Background()
	key := IdempotencyKey("http://retry-race/", "port_scan", "tcp")

	// One FAILED attempt under the poison threshold → retryable.
	if err := leases.Claim(ctx, ExecutionLease{Key: key, EntityID: "e1", ToolName: "port_scan", WorkerID: "w0", TimeoutMS: 60000}); err != nil {
		t.Fatalf("seed claim: %v", err)
	}
	if err := leases.Transition(ctx, key, LeaseFailed, ""); err != nil {
		t.Fatalf("seed fail: %v", err)
	}

	const racers = 8
	type outcome struct {
		worker int
		err    error
	}
	results := make(chan outcome, racers)
	for i := range racers {
		go func(worker int) {
			err := leases.Claim(ctx, ExecutionLease{Key: key, EntityID: "e1", ToolName: "port_scan", WorkerID: fmt.Sprintf("w%d", worker+1), TimeoutMS: 60000})
			results <- outcome{worker: worker, err: err}
		}(i)
	}
	wins, dupes, other := 0, 0, 0
	var firstOther error
	for range racers {
		o := <-results
		switch {
		case o.err == nil:
			wins++
		case errors.Is(o.err, ErrDuplicateClaim):
			dupes++
		case errors.Is(o.err, ErrPoisonPill):
			// Only possible if the crash/attempt threshold was reached; still
			// an acceptable "not executed" outcome, never a silent second run.
			dupes++
		default:
			other++
			if firstOther == nil {
				firstOther = o.err
			}
		}
	}
	if other != 0 {
		t.Fatalf("unexpected errors from racers: %v", firstOther)
	}
	if wins != 1 {
		t.Fatalf("exactly one retry claim must win, got %d wins / %d dupes", wins, dupes)
	}
	if wins+dupes != racers {
		t.Fatalf("every racer must get a verdict, got %d wins + %d dupes != %d", wins, dupes, racers)
	}

	// The surviving row must be CLAIMED by the single winner with attempt
	// count advanced exactly once from the seed's 1.
	var status string
	var attempts int
	if err := leases.db.QueryRowContext(ctx,
		`SELECT status, attempt_count FROM execution_leases WHERE idempotency_key=?`, key).Scan(&status, &attempts); err != nil {
		t.Fatalf("read back lease row: %v", err)
	}
	if status != string(LeaseClaimed) || attempts != 2 {
		t.Fatalf("winner must leave CLAIMED/attempt=2, got %s/%d", status, attempts)
	}
}

// TestLeaseOrphanRecoverySkipsFreshHeartbeat reproduces the RecoverOrphans
// kill window: recovery scans expired-heartbeat rows, then a live worker
// heartbeats (via Transition, which bumps heartbeat_at) before the flip. The
// conditional TransitionIfStale must skip the revived row instead of
// marking a live run FAILED.
func TestLeaseOrphanRecoverySkipsFreshHeartbeat(t *testing.T) {
	_, leases := newTestLeaseStore(t)
	ctx := context.Background()

	liveKey := IdempotencyKey("http://alive-race/", "http_probe", "h")
	if err := leases.Claim(ctx, ExecutionLease{Key: liveKey, EntityID: "e1", ToolName: "http_probe", WorkerID: "w-live", TimeoutMS: 50}); err != nil {
		t.Fatalf("claim live: %v", err)
	}
	if err := leases.Transition(ctx, liveKey, LeaseRunning, ""); err != nil {
		t.Fatalf("running live: %v", err)
	}

	// Wait out the 50ms TTL so recovery's scan lists the live lease as an
	// orphan candidate.
	time.Sleep(80 * time.Millisecond)

	// Simulate the race window directly: the heartbeat lands between scan
	// and flip, so use the pre-heartbeat value as the stale marker.
	var heartbeatAt int64
	if err := leases.db.QueryRowContext(ctx,
		`SELECT heartbeat_at FROM execution_leases WHERE idempotency_key=?`, liveKey).Scan(&heartbeatAt); err != nil {
		t.Fatalf("read heartbeat: %v", err)
	}
	if err := leases.Transition(ctx, liveKey, LeaseRunning, ""); err != nil {
		t.Fatalf("live worker heartbeat: %v", err)
	}

	// Recovery's flip compares against the scanned (pre-heartbeat) value;
	// the row no longer matches → skipped.
	if err := leases.TransitionIfStale(ctx, liveKey, LeaseFailed, heartbeatAt); !errors.Is(err, ErrLeaseRevived) {
		t.Fatalf("stale flip on refreshed row must return ErrLeaseRevived, got %v", err)
	}
	var status string
	if err := leases.db.QueryRowContext(ctx,
		`SELECT status FROM execution_leases WHERE idempotency_key=?`, liveKey).Scan(&status); err != nil {
		t.Fatalf("read back live lease: %v", err)
	}
	if status != string(LeaseRunning) {
		t.Fatalf("live worker must survive recovery, got status %s", status)
	}
}

// TestLeaseOrphanRecoveryStillRequeuesDead confirms the conditional flip
// still requeues genuinely dead leases (unchanged heartbeat passes the
// stale check).
func TestLeaseOrphanRecoveryStillRequeuesDead(t *testing.T) {
	_, leases := newTestLeaseStore(t)
	ctx := context.Background()

	deadKey := IdempotencyKey("http://dead-still/", "http_probe", "d")
	if err := leases.Claim(ctx, ExecutionLease{Key: deadKey, EntityID: "e2", ToolName: "http_probe", WorkerID: "w-dead", TimeoutMS: 30}); err != nil {
		t.Fatalf("claim dead: %v", err)
	}
	if err := leases.Transition(ctx, deadKey, LeaseRunning, ""); err != nil {
		t.Fatalf("running dead: %v", err)
	}

	nowMS := timeNowMS() + 1000
	requeue, quarantined, err := leases.RecoverOrphans(ctx, nowMS)
	if err != nil {
		t.Fatalf("recover orphans: %v", err)
	}
	if len(quarantined) != 0 {
		t.Fatalf("expected no quarantined, got %d", len(quarantined))
	}
	if len(requeue) != 1 || requeue[0].Key != deadKey {
		t.Fatalf("expected exactly the dead key in requeue, got %+v", requeue)
	}
	var status string
	if err := leases.db.QueryRowContext(ctx,
		`SELECT status FROM execution_leases WHERE idempotency_key=?`, deadKey).Scan(&status); err != nil {
		t.Fatalf("read back dead lease: %v", err)
	}
	if status != string(LeaseFailed) {
		t.Fatalf("dead lease must end FAILED for requeue, got %s", status)
	}
}

func timeNowMS() int64 {
	return time.Now().UnixMilli()
}
