package eventlog

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
)

// S4 idempotency spine (GateMesh architecture, doc/gatemesh-architecture-v1.md §L0).
//
// The execution_leases table is a four-phase micro-transaction ledger so a
// crashed `ouro agent run` resumes deterministically:
//
//	CLAIMED → RUNNING (+lease TTL) → ARTIFACT_SAVED → COMMITTED
//
// Recovery rules (implemented in RecoverOrphans):
//   - COMMITTED rows        → replayed into the in-memory state by the caller
//   - ARTIFACT_SAVED rows   → skip network interaction, re-run parsers only
//   - RUNNING rows w/ expired TTL → requeue (cache check first), unless the
//     same key crashed twice → poison-pill quarantine (tool×entity blacklisted)
//
// The same idempotency key hitting CLAIMED twice returns ErrDuplicateClaim so
// concurrent workers cannot double-execute.

const createExecutionLeasesSQL = `
CREATE TABLE IF NOT EXISTS execution_leases (
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
);
CREATE INDEX IF NOT EXISTS idx_leases_status_heartbeat ON execution_leases(status, heartbeat_at);
`

// LeaseStatus is the four-phase lifecycle of one idempotent tool action.
type LeaseStatus string

const (
	LeaseClaimed        LeaseStatus = "CLAIMED"
	LeaseRunning        LeaseStatus = "RUNNING"
	LeaseArtifactSaved  LeaseStatus = "ARTIFACT_SAVED"
	LeaseCommitted      LeaseStatus = "COMMITTED"
	LeaseFailed         LeaseStatus = "FAILED"
	LeaseAborted        LeaseStatus = "ABORTED"
)

// ErrDuplicateClaim means another worker already owns this idempotency key.
var ErrDuplicateClaim = errors.New("execution lease: duplicate claim")

// ErrPoisonPill means this tool×entity combination crashed twice and is
// quarantined; the caller must pick an alternative tool.
var ErrPoisonPill = errors.New("execution lease: poison-pill quarantine")

// PoisonThreshold is the attempt count after which a key is quarantined.
const PoisonThreshold = 2

// IdempotencyKey derives the dedupe key for one tool invocation:
// hash(target + tool + normalized args) — stable across restarts.
func IdempotencyKey(target, tool, normalizedArgs string) string {
	h := sha256.Sum256([]byte(target + "\x00" + tool + "\x00" + normalizedArgs))
	return hex.EncodeToString(h[:16])
}

// ExecutionLease is one row of the ledger.
type ExecutionLease struct {
	Key            string
	EntityID       string
	ToolName       string
	Status         LeaseStatus
	WorkerID       string
	HeartbeatAt    int64 // unix milliseconds
	TimeoutMS      int64
	AttemptCount   int
	RawArtifactPath string
}

// LeaseStore wraps the execution_leases table inside an existing SQLiteStore.
type LeaseStore struct {
	db *sql.DB
}

// NewLeaseStore attaches the ledger to a migrated SQLiteStore.
func NewLeaseStore(store *SQLiteStore) (*LeaseStore, error) {
	if store == nil || store.db == nil {
		return nil, fmt.Errorf("lease store: nil eventlog store")
	}
	if _, err := store.db.Exec(createExecutionLeasesSQL); err != nil {
		return nil, fmt.Errorf("create execution_leases: %w", err)
	}
	return &LeaseStore{db: store.db}, nil
}

// Claim inserts a CLAIMED row; ErrDuplicateClaim when the key is owned.
// A prior FAILED/ABORTED row with attempts < PoisonThreshold is overwritten
// (retry); attempts >= PoisonThreshold returns ErrPoisonPill.
func (l *LeaseStore) Claim(ctx context.Context, lease ExecutionLease) error {
	now := time.Now().UnixMilli()
	lease.HeartbeatAt = now

	// Poison-pill check first: crashed too often → quarantine.
	var attempts int
	var status string
	err := l.db.QueryRowContext(ctx,
		`SELECT attempt_count, status FROM execution_leases WHERE idempotency_key = ?`,
		lease.Key).Scan(&attempts, &status)
	if err == nil {
		if attempts >= PoisonThreshold && (status == string(LeaseFailed) || status == string(LeaseAborted)) {
			return ErrPoisonPill
		}
		if status == string(LeaseClaimed) || status == string(LeaseRunning) ||
			status == string(LeaseArtifactSaved) || status == string(LeaseCommitted) {
			return ErrDuplicateClaim
		}
		// FAILED/ABORTED under threshold → retry by replacing the row.
		_, err = l.db.ExecContext(ctx, `
			UPDATE execution_leases
			SET status=?, worker_id=?, heartbeat_at=?, timeout_ms=?, attempt_count=attempt_count+1, raw_artifact_path=NULL, created_at=?
			WHERE idempotency_key=?`,
			string(LeaseClaimed), lease.WorkerID, now, lease.TimeoutMS,
			time.Now().UTC().Format(time.RFC3339), lease.Key)
		if err != nil {
			return fmt.Errorf("lease claim retry: %w", err)
		}
		return nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("lease claim lookup: %w", err)
	}

	_, err = l.db.ExecContext(ctx, `
		INSERT INTO execution_leases
		(idempotency_key, entity_id, tool_name, status, worker_id, heartbeat_at, timeout_ms, attempt_count, raw_artifact_path, created_at)
		VALUES (?,?,?,?,?,?,?,1,NULL,?)`,
		lease.Key, lease.EntityID, lease.ToolName, string(LeaseClaimed),
		lease.WorkerID, now, lease.TimeoutMS, time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") || strings.Contains(err.Error(), "duplicate") {
			return ErrDuplicateClaim
		}
		return fmt.Errorf("lease claim insert: %w", err)
	}
	return nil
}

// Transition moves a lease to the next phase (RUNNING / ARTIFACT_SAVED / COMMITTED / FAILED / ABORTED).
func (l *LeaseStore) Transition(ctx context.Context, key string, next LeaseStatus, artifactPath string) error {
	var setArtifact string
	var arg interface{}
	if artifactPath != "" {
		setArtifact = ", raw_artifact_path=?"
		arg = artifactPath
	}
	q := `UPDATE execution_leases SET status=?, heartbeat_at=?` + setArtifact + ` WHERE idempotency_key=?`
	args := []interface{}{string(next), time.Now().UnixMilli()}
	if artifactPath != "" {
		args = append(args, arg)
	}
	args = append(args, key)
	res, err := l.db.ExecContext(ctx, q, args...)
	if err != nil {
		return fmt.Errorf("lease transition to %s: %w", next, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("lease transition to %s: no row for key %s", next, key)
	}
	return nil
}

// Heartbeat refreshes the RUNNING lease TTL.
func (l *LeaseStore) Heartbeat(ctx context.Context, key string) error {
	_, err := l.db.ExecContext(ctx,
		`UPDATE execution_leases SET heartbeat_at=? WHERE idempotency_key=? AND status=?`,
		time.Now().UnixMilli(), key, string(LeaseRunning))
	if err != nil {
		return fmt.Errorf("lease heartbeat: %w", err)
	}
	return nil
}

// OrphanLease is a RUNNING lease whose TTL expired (worker died mid-run).
type OrphanLease struct {
	ExecutionLease
	ExpiredMS int64
}

// RecoverOrphans returns expired RUNNING leases for requeue and transitions
// them to FAILED. Keys at/above the poison threshold are marked ABORTED and
// returned separately (caller must switch to an alternative tool).
func (l *LeaseStore) RecoverOrphans(ctx context.Context, nowMS int64) (requeue []OrphanLease, quarantined []OrphanLease, err error) {
	rows, err := l.db.QueryContext(ctx, `
		SELECT idempotency_key, entity_id, tool_name, worker_id, heartbeat_at, timeout_ms, attempt_count, raw_artifact_path
		FROM execution_leases
		WHERE status = ? AND (? - heartbeat_at) > timeout_ms`,
		string(LeaseRunning), nowMS)
	if err != nil {
		return nil, nil, fmt.Errorf("lease orphan scan: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var o OrphanLease
		var artifact sql.NullString
		if err := rows.Scan(&o.Key, &o.EntityID, &o.ToolName, &o.WorkerID, &o.HeartbeatAt, &o.TimeoutMS, &o.AttemptCount, &artifact); err != nil {
			return nil, nil, fmt.Errorf("lease orphan scan row: %w", err)
		}
		o.RawArtifactPath = artifact.String
		o.ExpiredMS = nowMS - o.HeartbeatAt
		if o.AttemptCount >= PoisonThreshold {
			quarantined = append(quarantined, o)
		} else {
			requeue = append(requeue, o)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("lease orphan iterate: %w", err)
	}

	// Flip their statuses.
	for _, o := range requeue {
		if err := l.Transition(ctx, o.Key, LeaseFailed, ""); err != nil {
			return requeue, quarantined, err
		}
	}
	for _, o := range quarantined {
		if err := l.Transition(ctx, o.Key, LeaseAborted, ""); err != nil {
			return requeue, quarantined, err
		}
	}
	return requeue, quarantined, nil
}
