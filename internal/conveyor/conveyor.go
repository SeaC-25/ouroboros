// Package conveyor implements the GateMesh L1 reflex belt
// (doc/gatemesh-architecture-v1.md §L1): the 12-action mapping ported from
// KaliMCP's graph_actions.py + RuleRouter scoring, expressed as predicate
// matching over entity capabilities. Intent identification is deleted from
// the vocabulary: an action fires the moment an entity satisfies its
// requires/excludes mask, and ties are broken by the RuleRouter formula
// (confidence − source penalty − 0.01 × phase order).
package conveyor

import "strings"

// Capability is a marker an entity carries (KaliMCP node semantics mapped to
// flat strings; hypergraph-style N-to-1 combos are expressed as multi-capability
// requires lists).
type Capability string

const (
	CapWebEndpoint   Capability = "web_endpoint"  // reachable URL/host surface
	CapHost          Capability = "host"          // network host with ports
	CapDomain        Capability = "domain"        // DNS-resolvable scope item
	CapAuthSession   Capability = "auth_session"  // live credentials mounted
	CapFingerprinted Capability = "fingerprinted" // tech stack known
	CapWAFBlocked    Capability = "waf_blocked"   // target countermeasure hit
	CapTestedAuth    Capability = "tested_auth"   // auth attack already tried
	CapSourceCode    Capability = "source_code"   // static artifact available
	CapHistoryRoute  Capability = "history_route" // archived route discovered
)

// Phase is the KaliMCP phase vocabulary.
type Phase string

const (
	PhaseRecon       Phase = "RECON"
	PhaseFingerprint Phase = "FINGERPRINT"
	PhaseVerify      Phase = "VERIFY"
	PhaseExpand      Phase = "EXPAND"
)

// ActionSpec is one semantic action (port of KaliMCP graph_actions.ActionSpec).
type ActionSpec struct {
	Action           string
	Tool             string // executor binding: "curl" | "fastsec" | "nativetools:<op>"
	Phase            Phase
	Session          bool // shares the per-task cookie jar
	NeedsCredentials bool // skipped when no credentials available
	Requires         []Capability
	Excludes         []Capability
	// PhaseOrder mirrors the RuleRouter cheap-first table (http_probe=0 …
	// verify=11). Lower = tried earlier at equal confidence.
	PhaseOrder int
}

// Source labels where a candidate came from (RuleRouter penalty semantics).
type Source string

const (
	SourceDefault Source = "default" // explicit graph checks
	SourceInsight Source = "insight" // synthesized hypotheses
)

// SourcePenalty maps source → score penalty (RuleRouter: default −0.05,
// insight −0.15).
func SourcePenalty(s Source) float64 {
	switch s {
	case SourceInsight:
		return 0.15
	case SourceDefault:
		return 0.05
	}
	return 0
}

// Entity is the lightweight state carrier for predicate matching.
type Entity struct {
	ID           string
	Confidence   float64
	Source       Source
	Capabilities map[Capability]bool
	// UserPass carries seeded credentials ("user:pass") when the entity was
	// armed with an auth session (KaliMCP account-node semantics).
	UserPass string
	// SessionGen increments on each successful re-authentication so the
	// agent's idempotency keys distinguish pre-login from post-login runs
	// of the same action (a login changes preconditions → re-execution is
	// legitimate, not a duplicate).
	SessionGen int
}

// Credentials splits UserPass into user and password. hasCreds is false
// when no credentials are known.
func (e Entity) Credentials() (user, pass string, hasCreds bool) {
	if e.UserPass == "" {
		return "", "", false
	}
	parts := strings.SplitN(e.UserPass, ":", 2)
	if len(parts) != 2 || parts[0] == "" {
		return "", "", false
	}
	return parts[0], parts[1], true
}

// Has reports whether the entity carries a capability.
func (e Entity) Has(c Capability) bool { return e.Capabilities[c] }

// Satisfied reports whether the entity matches the action's requires/excludes
// mask: every Requires present, no Excludes present.
func (a ActionSpec) Satisfied(e Entity) bool {
	for _, c := range a.Requires {
		if !e.Has(c) {
			return false
		}
	}
	for _, c := range a.Excludes {
		if e.Has(c) {
			return false
		}
	}
	return true
}

// Score is the RuleRouter formula: confidence − source penalty − 0.01×phase.
func (a ActionSpec) Score(e Entity) float64 {
	return e.Confidence - SourcePenalty(e.Source) - 0.01*float64(a.PhaseOrder)
}

// Registry is the 12-action table (KaliMCP graph_actions.py mapping).
type Registry struct {
	actions []ActionSpec
}

// DefaultRegistry returns the 12 canonical actions with KaliMCP semantics:
// curl family carries session; fastsec family covers scan/fingerprint/verify.
func DefaultRegistry() *Registry {
	return &Registry{actions: []ActionSpec{
		{Action: "http_probe", Tool: "curl", Phase: PhaseRecon, Session: true, PhaseOrder: 0,
			Requires: []Capability{CapWebEndpoint}},
		{Action: "body_fetch", Tool: "curl", Phase: PhaseRecon, Session: true, PhaseOrder: 1,
			Requires: []Capability{CapWebEndpoint}},
		{Action: "auth_login", Tool: "curl", Phase: PhaseExpand, Session: true, NeedsCredentials: true, PhaseOrder: 2,
			Requires: []Capability{CapWebEndpoint, CapAuthSession}, Excludes: []Capability{CapTestedAuth}},
		{Action: "port_scan", Tool: "fastsec", Phase: PhaseRecon, PhaseOrder: 3,
			Requires: []Capability{CapHost}},
		{Action: "fingerprint", Tool: "fastsec", Phase: PhaseFingerprint, PhaseOrder: 4,
			Requires: []Capability{CapWebEndpoint}, Excludes: []Capability{CapFingerprinted}},
		{Action: "cms_scan", Tool: "fastsec", Phase: PhaseFingerprint, PhaseOrder: 5,
			Requires: []Capability{CapWebEndpoint}},
		{Action: "osint", Tool: "fastsec", Phase: PhaseRecon, PhaseOrder: 6,
			Requires: []Capability{CapDomain}},
		{Action: "shallow_dir", Tool: "fastsec", Phase: PhaseRecon, PhaseOrder: 7,
			Requires: []Capability{CapWebEndpoint}},
		{Action: "sqli_probe", Tool: "fastsec", Phase: PhaseVerify, PhaseOrder: 8,
			Requires: []Capability{CapWebEndpoint, CapFingerprinted}},
		{Action: "xss_probe", Tool: "fastsec", Phase: PhaseVerify, PhaseOrder: 9,
			Requires: []Capability{CapWebEndpoint, CapFingerprinted}},
		{Action: "nuclei_subset", Tool: "fastsec", Phase: PhaseVerify, PhaseOrder: 10,
			Requires: []Capability{CapWebEndpoint, CapFingerprinted}},
		{Action: "verify_finding", Tool: "internal", Phase: PhaseVerify, PhaseOrder: 11,
			Requires: []Capability{CapWebEndpoint}},
	}}
}

// Actions returns the registry contents (defensive copy).
func (r *Registry) Actions() []ActionSpec {
	out := make([]ActionSpec, len(r.actions))
	copy(out, r.actions)
	return out
}

// Candidate is one matched (entity, action) pair ready for scoring.
type Candidate struct {
	Entity Entity
	Spec   ActionSpec
	Score  float64
}

// Matched returns all actions whose mask the entity satisfies. Actions with
// NeedsCredentials are skipped when hasCredentials is false (auth_login must
// not burn steps when no credentials are known — KaliMCP auto_loop semantics).
func (r *Registry) Matched(e Entity, hasCredentials bool) []Candidate {
	var out []Candidate
	for _, a := range r.actions {
		if a.NeedsCredentials && !hasCredentials {
			continue
		}
		if a.Satisfied(e) {
			out = append(out, Candidate{Entity: e, Spec: a, Score: a.Score(e)})
		}
	}
	return out
}

// Select returns candidates ordered best-score-first (stable on phase order
// for ties — matching RuleRouter's cheap-first deterministic behavior).
func Select(candidates []Candidate) []Candidate {
	out := make([]Candidate, len(candidates))
	copy(out, candidates)
	// insertion sort keeps stability without importing sort.Slice stability caveats
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j].Score > out[j-1].Score; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}
