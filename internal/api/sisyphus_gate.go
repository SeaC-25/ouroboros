package api

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
)

// The Sisyphus gate breaks observation-only loops: ACTION:OBSERVE costs a
// real network request and lands raw evidence on the blackboard, but raw
// evidence only becomes progress when a conclusion consumes it. A model
// that keeps observing without concluding re-buries the signal it already
// has (Sisyphus pushing the boulder uphill). This gate counts the
// session-level backlog of unconsumed observations (Server.unprocessedObs)
// and refuses new observations once it reaches sisyphusGateThreshold,
// naming the exact obs-ids the model must cite via ACTION:BLACKBOARD or
// consume via ACTION:VERIFY before new evidence is accepted.
//
// Scope note: the backlog is intentionally shared across the parent chat
// loop and every spawned sub-loop — sub-loops run
// parseAndExecuteAIActions on the same Server, and unprocessed evidence
// anywhere is a global signal (the conclusion that consumes it may come
// from any loop).

// sisyphusGateThreshold is the backlog size at which new observations are
// refused. Three unconsumed observations mean the model has three distinct
// pieces of evidence awaiting a conclusion — enough to conclude or verify
// something before gathering more.
const sisyphusGateThreshold = 3

// sisyphusGateListLimit caps how many obs-ids the refusal message lists.
const sisyphusGateListLimit = 8

// sisyphusObservationDedupSize bounds the request-level dedup cache: the
// last N identical observation requests short-circuit (no network request,
// no backlog entry, no gate refusal) because a repeated identical request
// is not new evidence.
const sisyphusObservationDedupSize = 128

// sisyphusObservationKey fingerprints one ACTION:OBSERVE request
// (URL + raw spec text). Two identical requests observe the same thing —
// the second one adds no new evidence even if the server mutated state,
// and re-issuing it while under gate refusal is exactly the loop the gate
// exists to stop.
func sisyphusObservationKey(targetURL string, specJSON string) string {
	sum := sha256.Sum256([]byte(targetURL + "\x00" + specJSON))
	return hex.EncodeToString(sum[:])
}

// sisyphusObservationDedup is a bounded FIFO cache of recently served
// observation request fingerprints. Guarded by its own mutex; small
// (≤ sisyphusObservationDedupSize entries), replaced wholesale on
// eviction.
type sisyphusObservationDedup struct {
	mu      sync.Mutex
	entries map[string]struct{}
	order   []string
}

func newSisyphusObservationDedup() *sisyphusObservationDedup {
	return &sisyphusObservationDedup{entries: make(map[string]struct{}, sisyphusObservationDedupSize)}
}

// seen reports whether the key was already served, and records it when new.
func (d *sisyphusObservationDedup) seen(key string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	if _, ok := d.entries[key]; ok {
		return true
	}
	if len(d.order) >= sisyphusObservationDedupSize {
		delete(d.entries, d.order[0])
		d.order = d.order[1:]
	}
	d.entries[key] = struct{}{}
	d.order = append(d.order, key)
	return false
}

// sisyphusObsDedup is the process-wide request dedup cache. It is global
// (not per-Server) on purpose: identical observation requests across
// parent loop and sub-loops share one truth for "was this already asked".
// Keyed by URL+spec content, never by workspace, so distinct workspaces
// observing genuinely different URLs never collide; same URL+spec in two
// workspaces is the same request to the same target and a hit is correct.
var sisyphusObsDedup = newSisyphusObservationDedup()

// sisyphusDedupSummary is the fixed response for a repeated identical
// observation: it names the contract explicitly so the model knows why no
// new observation was recorded and what unblocks progress.
const sisyphusDedupSummary = "DEDUP_CACHE_HIT: 与近期观测完全相同的请求（同 URL+spec）未重复执行，也未产生新证据。请基于已有观测得出结论：ACTION:BLACKBOARD <结论>（引用对应 obs-id）核销积压证据，或 ACTION:VERIFY <观测ID> <预期信号> 晋升。确需重观测请改变请求（如不同 path/header/body）。"

// sisyphusGateRefusal renders the gate refusal for the current backlog.
// The message names the pending obs-ids (capped at sisyphusGateListLimit)
// so the model can cite them verbatim in its next ACTION:BLACKBOARD.
func sisyphusGateRefusal(pending []string) string {
	listed := pending
	if len(listed) > sisyphusGateListLimit {
		listed = listed[:sisyphusGateListLimit]
	}
	var b strings.Builder
	fmt.Fprintf(&b, "[SISYPHUS_GATE] 你已累积 %d 条未核销观测（%s）。", len(pending), strings.Join(listed, "、"))
	b.WriteString("工具仅做零判定观测——请先用 ACTION:BLACKBOARD <结论>（引用这些 obs-id）核销上述证据，或 ACTION:VERIFY 晋升，再发起新观测。")
	return b.String()
}

// pendingObsCount returns the number of unconsumed observations and their
// IDs in registration order (oldest first — the debt the model accumulated
// first). Volume is bounded by the gate itself; a Range scan is fine.
func (s *Server) pendingObsCount() (int, []string) {
	s.unprocessedObs.Range(func(key, _ any) bool {
		id, _ := key.(string)
		s.unprocessedObs.Store(key, id) // no-op touch keeps Range single-pass
		return true
	})
	// Range order is unspecified: sort by numeric timestamp embedded in the
	// obs id (obs-<nano>[-<hex4>]) so the refusal lists oldest debt first.
	type entry struct {
		id    string
		nanos int64
	}
	entries := make([]entry, 0, 16)
	s.unprocessedObs.Range(func(key, _ any) bool {
		id, ok := key.(string)
		if ok {
			entries = append(entries, entry{id: id, nanos: obsIDTimestamp(id)})
		}
		return true
	})
	// insertion sort by nanos, stable and allocation-free at this size
	for i := 1; i < len(entries); i++ {
		for j := i; j > 0 && entries[j].nanos < entries[j-1].nanos; j-- {
			entries[j], entries[j-1] = entries[j-1], entries[j]
		}
	}
	ids := make([]string, 0, len(entries))
	for _, e := range entries {
		ids = append(ids, e.id)
	}
	return len(ids), ids
}

// obsIDTimestamp extracts the nanosecond timestamp from an
// obs-<nano>[-<hex4>] id; unparseable ids sort last (0 sentinel handled by
// keeping them after every parseable entry via max int64).
func obsIDTimestamp(id string) int64 {
	rest, ok := strings.CutPrefix(id, "obs-")
	if !ok {
		return int64(1) << 62
	}
	end := strings.IndexByte(rest, '-')
	if end < 0 {
		end = len(rest)
	}
	var nanos int64
	for i := 0; i < end; i++ {
		c := rest[i]
		if c < '0' || c > '9' {
			return int64(1) << 62
		}
		if nanos > (int64(1)<<62)/10 {
			return int64(1) << 62
		}
		nanos = nanos*10 + int64(c-'0')
	}
	return nanos
}

// consumeObservations deletes every listed observation ID from the
// unconsumed backlog. Missing ids are ignored (already consumed) —
// consumption is idempotent.
func (s *Server) consumeObservations(obsIDs []string) {
	for _, id := range obsIDs {
		if id != "" {
			s.unprocessedObs.Delete(id)
		}
	}
}

// checkSisyphusGate is the pre-request circuit breaker for ACTION:OBSERVE.
// It returns the refusal result summary when the backlog is at/over
// threshold, or "" when the observation may proceed.
func (s *Server) checkSisyphusGate() string {
	pendingCount, pendingIDs := s.pendingObsCount()
	if pendingCount >= sisyphusGateThreshold {
		return sisyphusGateRefusal(pendingIDs)
	}
	return ""
}

// registerUnprocessedObservation enqueues a freshly persisted raw
// observation into the backlog. Called only after appendBlackboardObservation
// succeeded (a failed persist is not evidence awaiting consumption).
func (s *Server) registerUnprocessedObservation(obsID string) {
	if obsID != "" {
		s.unprocessedObs.Store(obsID, true)
	}
}
