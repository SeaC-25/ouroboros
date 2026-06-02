package swarm

import (
	"encoding/json"
	"sort"
	"time"

	"github.com/ouroboros/ouroboros/internal/eventlog"
)

type Lane string

const (
	LaneMission          Lane = "mission"
	LaneAgent            Lane = "agent"
	LaneObserver         Lane = "observer"
	LaneBlackboard       Lane = "blackboard"
	LaneToolchain        Lane = "toolchain"
	LaneSubmissionReport Lane = "submission-report"
)

type TimelineEvent struct {
	EventID   int64
	RunID     string
	Lane      Lane
	Type      eventlog.EventType
	Payload   json.RawMessage
	CreatedAt time.Time
}

func TimelineFromEvents(events []eventlog.Event) []TimelineEvent {
	timeline := make([]TimelineEvent, 0, len(events))
	for _, event := range events {
		timeline = append(timeline, TimelineEvent{
			EventID:   event.ID,
			RunID:     event.RunID,
			Lane:      LaneForEventType(event.Type),
			Type:      event.Type,
			Payload:   append(json.RawMessage(nil), event.Payload...),
			CreatedAt: event.CreatedAt,
		})
	}
	sort.SliceStable(timeline, func(i int, j int) bool {
		if timeline[i].CreatedAt.Equal(timeline[j].CreatedAt) {
			return timeline[i].EventID < timeline[j].EventID
		}
		return timeline[i].CreatedAt.Before(timeline[j].CreatedAt)
	})
	return timeline
}

func LaneForEventType(eventType eventlog.EventType) Lane {
	switch eventType {
	case eventlog.EventGoal:
		return LaneMission
	case eventlog.EventAgent, eventlog.EventAttempt:
		return LaneAgent
	case eventlog.EventFact, eventlog.EventIntent, eventlog.EventHint:
		return LaneBlackboard
	case eventlog.EventTool, eventlog.EventParser:
		return LaneToolchain
	case eventlog.EventReplay:
		return LaneSubmissionReport
	case eventlog.EventVerification, eventlog.EventMutation:
		return LaneObserver
	default:
		return LaneObserver
	}
}
