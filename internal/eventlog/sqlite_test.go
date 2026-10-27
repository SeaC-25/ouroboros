package eventlog

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestSQLiteStoreAppendsAndReplaysByRunIDInOrder(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	defer store.Close()

	first, err := store.Append(ctx, Event{
		RunID:   "run-a",
		Type:    EventGoal,
		Payload: json.RawMessage(`{"goal":"verify admin route"}`),
	})
	if err != nil {
		t.Fatalf("append first: %v", err)
	}
	second, err := store.Append(ctx, Event{
		RunID:   "run-a",
		Type:    EventFact,
		Payload: json.RawMessage(`{"fact":"admin route discovered"}`),
	})
	if err != nil {
		t.Fatalf("append second: %v", err)
	}
	if _, err := store.Append(ctx, Event{
		RunID:   "run-b",
		Type:    EventTool,
		Payload: json.RawMessage(`{"tool":"httpx"}`),
	}); err != nil {
		t.Fatalf("append other run: %v", err)
	}

	events, err := store.Replay(ctx, "run-a")
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("expected two events for run-a, got %d", len(events))
	}
	if events[0].ID != first.ID || events[1].ID != second.ID {
		t.Fatalf("events were not replayed in append order: %#v", events)
	}
	if events[0].CreatedAt.IsZero() || events[1].CreatedAt.IsZero() {
		t.Fatalf("expected created timestamps")
	}
}

func TestSQLiteStoreRoundTripsJSONPayload(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	defer store.Close()

	payload := json.RawMessage(`{"target":"https://example.test","ports":[80,443],"verified":true}`)
	written, err := store.Append(ctx, Event{
		RunID:   "run-json",
		Type:    EventVerification,
		Payload: payload,
	})
	if err != nil {
		t.Fatalf("append: %v", err)
	}

	events, err := store.Replay(ctx, "run-json")
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected one event, got %d", len(events))
	}
	if events[0].ID != written.ID {
		t.Fatalf("expected event id %d, got %d", written.ID, events[0].ID)
	}

	var decoded struct {
		Target   string `json:"target"`
		Ports    []int  `json:"ports"`
		Verified bool   `json:"verified"`
	}
	if err := json.Unmarshal(events[0].Payload, &decoded); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if decoded.Target != "https://example.test" || len(decoded.Ports) != 2 || !decoded.Verified {
		t.Fatalf("unexpected payload: %#v", decoded)
	}
}

func TestSQLiteStoreEnablesWALMode(t *testing.T) {
	store := openTestStore(t)
	defer store.Close()

	var mode string
	if err := store.db.QueryRow("PRAGMA journal_mode").Scan(&mode); err != nil {
		t.Fatalf("query journal mode: %v", err)
	}
	if mode != "wal" {
		t.Fatalf("expected wal journal mode, got %q", mode)
	}
}

func TestOpenSQLiteReadOnlyListsRunIDsWithoutWALSidecars(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "events.db")
	store, err := OpenSQLite(path)
	if err != nil {
		t.Fatalf("open sqlite store: %v", err)
	}
	defer func() { _ = store.Close() }()
	if _, err := store.Append(ctx, Event{RunID: "run-a", Type: EventGoal, Payload: json.RawMessage(`{"goal":"a"}`)}); err != nil {
		t.Fatalf("append run-a: %v", err)
	}
	if _, err := store.Append(ctx, Event{RunID: "run-b", Type: EventGoal, Payload: json.RawMessage(`{"goal":"b"}`)}); err != nil {
		t.Fatalf("append run-b: %v", err)
	}
	// Deliberately NOT checkpointing and NOT closing the writer: the
	// read-only handle must coexist with the live WAL and see events that
	// exist only there (the old immutable=1 posture hid them).

	readOnly, err := OpenSQLiteReadOnly(path)
	if err != nil {
		t.Fatalf("open read-only sqlite store: %v", err)
	}
	defer readOnly.Close()
	runIDs, err := readOnly.ListRunIDs(ctx)
	if err != nil {
		t.Fatalf("list run ids read-only: %v", err)
	}
	if len(runIDs) != 2 || runIDs[0] != "run-a" || runIDs[1] != "run-b" {
		t.Fatalf("unexpected read-only run ids: %#v", runIDs)
	}
	if _, err := readOnly.Append(ctx, Event{RunID: "run-c", Type: EventGoal, Payload: json.RawMessage(`{"goal":"c"}`)}); err == nil {
		t.Fatalf("read-only sqlite store must reject appends")
	}
	// The old immutable=1 flag made the read-only handle blind to WAL-only
	// events; the contract now is live visibility, so sidecar presence is
	// acceptable (same directory is writable by the desktop app anyway).
	// Guard the real invariant instead: a WAL-only event appended after the
	// read-only handle exists must be visible on the next query.
	if _, err := store.Append(ctx, Event{RunID: "run-a", Type: EventHint, Payload: json.RawMessage(`{"h":"late"}`)}); err != nil {
		t.Fatalf("append late hint: %v", err)
	}
	events, err := readOnly.Replay(ctx, "run-a")
	if err != nil {
		t.Fatalf("replay run-a read-only: %v", err)
	}
	sawLate := false
	for _, event := range events {
		if event.Type == EventHint {
			sawLate = true
		}
	}
	if !sawLate {
		t.Fatalf("read-only store must see the WAL-only late hint event")
	}
}

func TestEventTypesAreDeclared(t *testing.T) {
	types := []EventType{
		EventGoal,
		EventFact,
		EventIntent,
		EventHint,
		EventAttempt,
		EventAgent,
		EventTool,
		EventParser,
		EventVerification,
		EventMutation,
		EventReplay,
	}
	for _, eventType := range types {
		if eventType == "" {
			t.Fatalf("event type must not be empty")
		}
	}
}

func removeSQLiteSidecarIfExists(t *testing.T, path string) {
	t.Helper()
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		t.Fatalf("remove sqlite sidecar %s: %v", path, err)
	}
}

func openTestStore(t *testing.T) *SQLiteStore {
	t.Helper()
	store, err := OpenSQLite(filepath.Join(t.TempDir(), "events.db"))
	if err != nil {
		t.Fatalf("open sqlite store: %v", err)
	}
	return store
}

func TestSQLiteStoreRejectsInvalidEvent(t *testing.T) {
	store := openTestStore(t)
	defer store.Close()

	_, err := store.Append(context.Background(), Event{
		RunID:   "",
		Type:    EventGoal,
		Payload: json.RawMessage(`{}`),
	})
	if err == nil {
		t.Fatalf("expected empty run id to fail")
	}

	_, err = store.Append(context.Background(), Event{
		RunID:   "run-a",
		Type:    EventGoal,
		Payload: json.RawMessage(`{invalid-json`),
	})
	if err == nil {
		t.Fatalf("expected invalid JSON payload to fail")
	}
}

func TestReplayMissingRunReturnsEmptySlice(t *testing.T) {
	store := openTestStore(t)
	defer store.Close()

	events, err := store.Replay(context.Background(), "missing")
	if err != nil {
		t.Fatalf("replay missing run: %v", err)
	}
	if events == nil {
		t.Fatalf("expected empty slice, not nil")
	}
	if len(events) != 0 {
		t.Fatalf("expected no events, got %d", len(events))
	}
}
