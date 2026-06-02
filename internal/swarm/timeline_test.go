package swarm

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/ouroboros/ouroboros/internal/eventlog"
)

func TestTimelineAssignsEventsToLanes(t *testing.T) {
	now := time.Now()
	events := []eventlog.Event{
		event(EventLaneID(1), eventlog.EventGoal, now),
		event(EventLaneID(2), eventlog.EventAgent, now.Add(time.Second)),
		event(EventLaneID(3), eventlog.EventMutation, now.Add(2*time.Second)),
		event(EventLaneID(4), eventlog.EventFact, now.Add(3*time.Second)),
		event(EventLaneID(5), eventlog.EventTool, now.Add(4*time.Second)),
		event(EventLaneID(6), eventlog.EventReplay, now.Add(5*time.Second)),
	}

	timeline := TimelineFromEvents(events)

	expected := []Lane{
		LaneMission,
		LaneAgent,
		LaneObserver,
		LaneBlackboard,
		LaneToolchain,
		LaneSubmissionReport,
	}
	if len(timeline) != len(expected) {
		t.Fatalf("expected %d timeline events, got %d", len(expected), len(timeline))
	}
	for i, lane := range expected {
		if timeline[i].Lane != lane {
			t.Fatalf("event %d expected lane %s, got %s", i, lane, timeline[i].Lane)
		}
	}
}

func TestTimelineEventsAreChronologicallyOrdered(t *testing.T) {
	now := time.Now()
	events := []eventlog.Event{
		event(2, eventlog.EventAgent, now.Add(2*time.Second)),
		event(1, eventlog.EventGoal, now),
		event(3, eventlog.EventTool, now.Add(time.Second)),
	}

	timeline := TimelineFromEvents(events)

	if timeline[0].EventID != 1 || timeline[1].EventID != 3 || timeline[2].EventID != 2 {
		t.Fatalf("timeline not sorted chronologically: %#v", timeline)
	}
}

func TestTimelinePreservesPayload(t *testing.T) {
	now := time.Now()
	events := []eventlog.Event{
		{
			ID:        1,
			RunID:     "run-a",
			Type:      eventlog.EventFact,
			Payload:   json.RawMessage(`{"statement":"target observed"}`),
			CreatedAt: now,
		},
	}

	timeline := TimelineFromEvents(events)

	if string(timeline[0].Payload) != `{"statement":"target observed"}` {
		t.Fatalf("unexpected payload: %s", string(timeline[0].Payload))
	}
}

type EventLaneID int64

func event(id EventLaneID, eventType eventlog.EventType, createdAt time.Time) eventlog.Event {
	return eventlog.Event{
		ID:        int64(id),
		RunID:     "run-a",
		Type:      eventType,
		Payload:   json.RawMessage(`{}`),
		CreatedAt: createdAt,
	}
}
