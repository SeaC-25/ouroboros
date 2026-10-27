package sisyphus

import (
	"strings"

	"github.com/ouroboros/ouroboros/internal/blackboard"
)

// StepFromObservations schedules the next task from raw observations instead
// of intents. The observation-reasoning loop: every unprocessed observation
// must reach the model first — the model analyzes the evidence, forms or
// updates hypotheses, and only conclusions are recorded as facts. Intents
// stay in the state but are demoted to "hypotheses awaiting verification";
// they no longer drive scheduling while observation work remains.
//
// Scheduling rules, in priority order:
//  1. No facts, no intents, no observations → bootstrap (unchanged).
//  2. Any unprocessed observation → a reasoning task so the model analyzes
//     the raw evidence before anything else is scheduled. The Reason field
//     names the observations awaiting analysis, giving the reasoning step
//     its evidence list.
//  3. Otherwise fall back to the intent-based Step logic (backward
//     compatible), with intents read as hypotheses awaiting verification.
func (e Engine) StepFromObservations(state blackboard.ProjectState) Task {
	pending := UnprocessedObservations(state)

	// Rule 1: empty project → bootstrap, identical to Step.
	if len(state.Facts) == 0 && len(state.Intents) == 0 && len(pending) == 0 {
		return Task{
			Kind:   TaskBootstrap,
			Reason: "project has no facts, intents, or observations",
		}
	}

	// Rule 2: unprocessed observations outrank everything. Even with open
	// intents, the loop schedules model reasoning over raw evidence first —
	// verification tasks for hypotheses (intents) come after the model has
	// digested the observation backlog.
	if len(pending) > 0 {
		ids := make([]string, 0, len(pending))
		for _, obs := range pending {
			ids = append(ids, obs.ID)
		}
		return Task{
			Kind:   TaskReason,
			Reason: "observations awaiting model analysis: " + strings.Join(ids, ","),
		}
	}

	// Rule 3: no observation backlog — defer to the intent-driven Step.
	// Intents are hypotheses awaiting verification in this reading.
	return e.Step(state)
}

// UnprocessedObservations returns observations not yet consumed by any fact.
// An observation is processed exactly when at least one fact lists it in
// SourceObservations — the evidence backlink a model conclusion records when
// it upgrades an observation into a fact.
func UnprocessedObservations(state blackboard.ProjectState) []blackboard.Observation {
	processed := ProcessedObservationIDs(state)
	pending := []blackboard.Observation{}
	for _, obs := range state.Observations {
		if _, ok := processed[obs.ID]; !ok {
			pending = append(pending, obs)
		}
	}
	return pending
}

// ProcessedObservationIDs collects every observation ID that existing facts
// already account for, so StepFromObservations stops scheduling reasoning
// for evidence the model has already concluded over.
func ProcessedObservationIDs(state blackboard.ProjectState) map[string]struct{} {
	processed := make(map[string]struct{})
	for _, fact := range state.Facts {
		for _, id := range fact.SourceObservations {
			if strings.TrimSpace(id) == "" {
				continue
			}
			processed[id] = struct{}{}
		}
	}
	return processed
}
