package blackboard

import "time"

// Evidence gate levels for Fact.EvidenceLevel: a fact derived from raw
// observations starts tentative; a successful observation replay that
// reproduces the expected signal promotes it to verified. Zero means no
// evidence gate ran (facts asserted without observation evidence, legacy
// journal records) — downstream chains must not treat them as verified.
const (
	// EvidenceNone marks facts with no evidence-gate classification. It is
	// the zero value so legacy JSONL records without the field load as 0.
	EvidenceNone EvidenceLevel = 0
	// EvidenceTentative: the fact was concluded over raw observations but
	// the expected signal has not been replay-verified.
	EvidenceTentative EvidenceLevel = 2
	// EvidenceVerified: ACTION:VERIFY replayed the supporting observation
	// and reproduced the expected signal.
	EvidenceVerified EvidenceLevel = 3
)

// EvidenceLevel grades how strongly the recorded evidence supports a fact
// (none / tentative / verified). Serialized as a plain int in the journals.
type EvidenceLevel = int

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

	// EvidenceLevel grades the recorded evidence (0 none / 2 tentative /
	// 3 verified). Facts written by the chat BLACKBOARD action start at
	// EvidenceTentative; ACTION:VERIFY replaying a supporting observation
	// promotes every fact back-linking it to EvidenceVerified. Legacy
	// records without this field load as the zero value (EvidenceNone).
	EvidenceLevel EvidenceLevel
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
