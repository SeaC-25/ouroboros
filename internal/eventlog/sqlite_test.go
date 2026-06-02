package eventlog

import (
	"context"
	"encoding/json"
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
