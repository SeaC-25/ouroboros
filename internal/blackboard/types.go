package blackboard

import "time"

type IntentState string

const (
	IntentOpen      IntentState = "Open"
	IntentClaimed   IntentState = "Claimed"
	IntentConcluded IntentState = "Concluded"
	IntentReleased  IntentState = "Released"
)

type Fact struct {
	ID         string
	Kind       string
	Statement  string
	Confidence float64
	Source     string
	CreatedAt  time.Time
}

type Intent struct {
	ID          string
	Kind        string
	Goal        string
	SourceFacts []string
	State       IntentState
	Lease       *Lease
	ResultFact  string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type Hint struct {
	ID        string
	Kind      string
	Message   string
	Source    string
	CreatedAt time.Time
}

type Lease struct {
	ID        string
	IntentID  string
	AgentID   string
	ExpiresAt time.Time
	UpdatedAt time.Time
}

type ProjectState struct {
	Facts   []Fact
	Intents []Intent
	Hints   []Hint
}

type ConcludeResult struct {
	Intent Intent
	Fact   Fact
}
