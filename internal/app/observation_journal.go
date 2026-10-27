package app

import (
	"bufio"
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/ouroboros/ouroboros/internal/blackboard"
)

// workspaceObservation mirrors the observation JSONL record the AI chat
// layer persists (runtime/observations.jsonl).
type workspaceObservation struct {
	ID        string `json:"id"`
	Kind      string `json:"kind"`
	Target    string `json:"target"`
	Payload   string `json:"payload,omitempty"`
	Source    string `json:"source,omitempty"`
	CreatedAt int64  `json:"createdAt"`
}

// loadWorkspaceObservations reads the durable observation journal. These
// observations are cross-run evidence: the closed-loop planner merges them
// into every snapshot so the reasoning task always sees the full backlog,
// not just observations produced inside the current run.
func loadWorkspaceObservations(root string) []blackboard.Observation {
	path := filepath.Join(root, "runtime", "observations.jsonl")
	file, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer func() { _ = file.Close() }()

	var observations []blackboard.Observation
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 4<<20)
	for scanner.Scan() {
		var record workspaceObservation
		if json.Unmarshal(scanner.Bytes(), &record) != nil {
			continue
		}
		if record.ID == "" || record.Target == "" {
			continue
		}
		createdAt := time.Unix(record.CreatedAt, 0).UTC()
		if record.CreatedAt == 0 {
			createdAt = time.Time{}
		}
		observations = append(observations, blackboard.Observation{
			ID:        record.ID,
			Kind:      record.Kind,
			Target:    record.Target,
			Payload:   record.Payload,
			Source:    record.Source,
			CreatedAt: createdAt,
		})
	}
	if err := scanner.Err(); err != nil {
		// Degrade gracefully: partial journal still feeds the backlog, but
		// the IO failure must not vanish silently.
		log.Printf("observation journal read failed (partial): %v: %s", err, path)
	}
	return observations
}

// mergeWorkspaceObservations unions the run-scoped snapshot observations
// with the durable journal, skipping duplicates by ID. It also unions the
// journal's fact conclusions (their source_observations back-links), so a
// conclusion recorded in a previous run still drains the backlog in the
// next one — the cross-run half of the reasoning loop.
func mergeWorkspaceObservations(root string, state blackboard.ProjectState) blackboard.ProjectState {
	seen := make(map[string]struct{}, len(state.Observations))
	for _, obs := range state.Observations {
		seen[obs.ID] = struct{}{}
	}
	for _, obs := range loadWorkspaceObservations(root) {
		if _, ok := seen[obs.ID]; ok {
			continue
		}
		state.Observations = append(state.Observations, obs)
	}
	for _, fact := range loadWorkspaceJournalFacts(root) {
		if len(fact.SourceObservations) == 0 {
			continue
		}
		state.Facts = append(state.Facts, fact)
	}
	return state
}

// journalFact mirrors the fact JSONL record the AI chat layer persists
// (runtime/blackboard.jsonl) — only the back-link fields matter here.
type journalFact struct {
	ID                 string   `json:"id"`
	Kind               string   `json:"kind"`
	Statement          string   `json:"statement"`
	Source             string   `json:"source"`
	SourceObservations []string `json:"source_observations,omitempty"`
	CreatedAt          int64    `json:"createdAt"`
}

// loadWorkspaceJournalFacts reads concluded facts that carry observation
// back-links. Facts without back-links are skipped: only evidence-carrying
// conclusions participate in backlog draining.
func loadWorkspaceJournalFacts(root string) []blackboard.Fact {
	path := filepath.Join(root, "runtime", "blackboard.jsonl")
	file, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer func() { _ = file.Close() }()

	var facts []blackboard.Fact
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 4<<20)
	for scanner.Scan() {
		var record journalFact
		if json.Unmarshal(scanner.Bytes(), &record) != nil {
			continue
		}
		if record.ID == "" || len(record.SourceObservations) == 0 {
			continue
		}
		facts = append(facts, blackboard.Fact{
			ID:                 record.ID,
			Kind:               record.Kind,
			Statement:          record.Statement,
			Source:             record.Source,
			SourceObservations: record.SourceObservations,
			CreatedAt:          time.Unix(record.CreatedAt, 0).UTC(),
		})
	}
	if err := scanner.Err(); err != nil {
		// Degrade gracefully: partial fact journal still drains backlinks,
		// but the IO failure must not vanish silently.
		log.Printf("fact journal read failed (partial): %v: %s", err, path)
	}
	return facts
}
