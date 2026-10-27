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

	// SourceObservations lists the observation IDs this fact was derived
	// from — the evidence backlink from a model conclusion to the raw
	// observations it reassembled. Empty for facts asserted without
	// observation evidence.
	SourceObservations []string
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

// Observation is a raw measurement captured by a tool run (http_observe
// output, port probe timings, TLS handshakes…) before any interpretation.
// The observation-reasoning loop: observations are collected first, the
// model reasons over them, and conclusions become Facts. Observations stay
// queryable so later reasoning can revisit the original evidence.
type Observation struct {
	ID        string
	Kind      string // e.g. http_observe, port_scan, tls_probe
	Target    string
	Payload   string // raw observation JSON/document
	Source    string
	CreatedAt time.Time
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
	Facts        []Fact
	Intents      []Intent
	Hints        []Hint
	Observations []Observation
}

type ConcludeResult struct {
	Intent Intent
	Fact   Fact
}
