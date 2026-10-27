package eventlog

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

type SQLiteStore struct {
	db *sql.DB
}

func OpenSQLite(path string) (*SQLiteStore, error) {
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("sqlite path is required")
	}

	// busy_timeout: the API server and a CLI run share this database; without
	// a wait window concurrent writers fail fast with SQLITE_BUSY (events
	// would be silently dropped by log-and-continue callers). One pooled
	// connection serializes writes inside the process and matches SQLite's
	// single-writer model.
	db, err := sql.Open("sqlite", path+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)")
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(1)

	store := &SQLiteStore{db: db}
	if err := store.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func OpenSQLiteReadOnly(path string) (*SQLiteStore, error) {
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("sqlite path is required")
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolve sqlite path: %w", err)
	}
	slashed := filepath.ToSlash(absPath)
	if !strings.HasPrefix(slashed, "/") {
		slashed = "/" + slashed
	}
	uri := url.URL{Scheme: "file", Path: slashed}
	query := uri.Query()
	query.Set("mode", "ro")
	// immutable=1 would tell SQLite the file never changes: it ignores the
	// WAL, so freshly committed events (the -wal file) stay invisible to
	// TUI/replay readers while a run is live, and a mid-read checkpoint can
	// surface torn pages. mode=ro alone is the correct read-only posture;
	// _query_only was never a valid modernc parameter (silent no-op).
	query.Add("_pragma", "busy_timeout(5000)")
	query.Add("_pragma", "foreign_keys(1)")
	uri.RawQuery = query.Encode()

	db, err := sql.Open("sqlite", uri.String())
	if err != nil {
		return nil, fmt.Errorf("open sqlite read-only: %w", err)
	}
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("open sqlite read-only: %w", err)
	}
	return &SQLiteStore{db: db}, nil
}

func (s *SQLiteStore) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *SQLiteStore) Append(ctx context.Context, event Event) (Event, error) {
	if strings.TrimSpace(event.RunID) == "" {
		return Event{}, errors.New("event run id is required")
	}
	if strings.TrimSpace(string(event.Type)) == "" {
		return Event{}, errors.New("event type is required")
	}
	if len(event.Payload) == 0 {
		event.Payload = json.RawMessage(`{}`)
	}
	if !json.Valid(event.Payload) {
		return Event{}, errors.New("event payload must be valid JSON")
	}

	event.CreatedAt = time.Now().UTC()
	result, err := s.db.ExecContext(
		ctx,
		`INSERT INTO events(run_id, type, payload, created_at) VALUES (?, ?, ?, ?)`,
		event.RunID,
		string(event.Type),
		[]byte(event.Payload),
		event.CreatedAt.Format(time.RFC3339Nano),
	)
	if err != nil {
		return Event{}, fmt.Errorf("insert event: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return Event{}, fmt.Errorf("read event id: %w", err)
	}
	event.ID = id
	event.Payload = append(json.RawMessage(nil), event.Payload...)
	return event, nil
}

func (s *SQLiteStore) Replay(ctx context.Context, runID string) ([]Event, error) {
	if strings.TrimSpace(runID) == "" {
		return nil, errors.New("run id is required")
	}

	rows, err := s.db.QueryContext(
		ctx,
		`SELECT id, run_id, type, payload, created_at FROM events WHERE run_id = ? ORDER BY id ASC`,
		runID,
	)
	if err != nil {
		return nil, fmt.Errorf("query events: %w", err)
	}
	defer rows.Close()

	events := []Event{}
	for rows.Next() {
		var event Event
		var eventType string
		var payload []byte
		var createdAt string
		if err := rows.Scan(&event.ID, &event.RunID, &eventType, &payload, &createdAt); err != nil {
			return nil, fmt.Errorf("scan event: %w", err)
		}
		parsedAt, err := time.Parse(time.RFC3339Nano, createdAt)
		if err != nil {
			return nil, fmt.Errorf("parse event timestamp: %w", err)
		}
		event.Type = EventType(eventType)
		event.Payload = append(json.RawMessage(nil), payload...)
		event.CreatedAt = parsedAt
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate events: %w", err)
	}
	return events, nil
}

func (s *SQLiteStore) ListRunIDs(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT run_id FROM events GROUP BY run_id ORDER BY MAX(id) ASC`)
	if err != nil {
		return nil, fmt.Errorf("query run ids: %w", err)
	}
	defer rows.Close()

	runIDs := []string{}
	for rows.Next() {
		var runID string
		if err := rows.Scan(&runID); err != nil {
			return nil, fmt.Errorf("scan run id: %w", err)
		}
		if strings.TrimSpace(runID) != "" {
			runIDs = append(runIDs, runID)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate run ids: %w", err)
	}
	return runIDs, nil
}

func (s *SQLiteStore) migrate() error {
	if _, err := s.db.Exec(`PRAGMA journal_mode=WAL`); err != nil {
		return fmt.Errorf("enable wal: %w", err)
	}
	if _, err := s.db.Exec(`PRAGMA foreign_keys=ON`); err != nil {
		return fmt.Errorf("enable foreign keys: %w", err)
	}
	if _, err := s.db.Exec(createEventsTableSQL); err != nil {
		return fmt.Errorf("create event tables: %w", err)
	}
	return nil
}
