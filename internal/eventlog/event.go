package eventlog

import (
	"encoding/json"
	"time"
)

type EventType string

const (
	EventGoal         EventType = "goal"
	EventFact         EventType = "fact"
	EventIntent       EventType = "intent"
	EventHint         EventType = "hint"
	EventAttempt      EventType = "attempt"
	EventAgent        EventType = "agent"
	EventTool         EventType = "tool"
	EventParser       EventType = "parser"
	EventVerification EventType = "verification"
	EventMutation     EventType = "mutation"
	EventReplay       EventType = "replay"
)

type Event struct {
	ID        int64
	RunID     string
	Type      EventType
	Payload   json.RawMessage
	CreatedAt time.Time
}
