package sisyphus

import (
	"fmt"
	"strings"

	"github.com/ouroboros/ouroboros/internal/blackboard"
)

// Mutator generates intelligent mutations based on failure patterns and context.
type Mutator struct {
	analyzer FailureAnalyzer
}

// GenerateMutations creates a set of mutation hints based on attempt context.
func (m *Mutator) GenerateMutations(attempt Attempt, failure FailureReason, state blackboard.ProjectState) []MutationHint {
	hints := []MutationHint{}

	// Extract explicit hints from transcript
	hints = append(hints, m.extractExplicitHints(attempt.PartialTranscript)...)

	// Generate scope reduction hints for timeouts
	if failure.Kind == FailureTimeout {
		hints = append(hints, m.generateScopeReductionHints(attempt, state)...)
	}

	// Generate tool selection hints for parse failures
	if failure.Kind == FailureParseFailed {
		hints = append(hints, m.generateToolMutationHints(attempt)...)
	}

	// Generate retry hints for worker exits
	if failure.Kind == FailureWorkerExit {
		hints = append(hints, m.generateRetryHints(attempt)...)
	}

	// Deduplicate and rank hints
	hints = m.deduplicateHints(hints)
	hints = m.rankHints(hints, failure)

	return hints
}

// extractExplicitHints extracts lead/hint/reusable markers from transcript.
func (m *Mutator) extractExplicitHints(transcript string) []MutationHint {
	hints := []MutationHint{}

	for _, line := range strings.Split(transcript, "\n") {
		line = strings.TrimSpace(line)
		lower := strings.ToLower(line)

		switch {
		case strings.HasPrefix(lower, "lead:"):
			hints = append(hints, MutationHint{
				Kind:    "reusable_lead",
				Message: strings.TrimSpace(line[len("lead:"):]),
			})
		case strings.HasPrefix(lower, "hint:"):
			hints = append(hints, MutationHint{
				Kind:    "reusable_lead",
				Message: strings.TrimSpace(line[len("hint:"):]),
			})
		case strings.HasPrefix(lower, "reusable:"):
			hints = append(hints, MutationHint{
				Kind:    "reusable_lead",
				Message: strings.TrimSpace(line[len("reusable:"):]),
			})
		}
	}

	return hints
}

// generateScopeReductionHints suggests ways to reduce scope for timed-out tasks.
func (m *Mutator) generateScopeReductionHints(attempt Attempt, state blackboard.ProjectState) []MutationHint {
	hints := []MutationHint{}

	transcript := strings.ToLower(attempt.PartialTranscript)

	// If transcript mentions enumeration, suggest focusing
	if strings.Contains(transcript, "enumerate") || strings.Contains(transcript, "scan") {
		hints = append(hints, MutationHint{
			Kind:    "reduce_scope",
			Message: "focus on high-value paths instead of full enumeration",
		})
	}

	// If transcript mentions authentication, suggest prioritizing
	if strings.Contains(transcript, "auth") || strings.Contains(transcript, "login") {
		hints = append(hints, MutationHint{
			Kind:    "prioritize",
			Message: "prioritize authenticated paths over anonymous exploration",
		})
	}

	// If many facts exist, suggest leveraging them
	if len(state.Facts) > 5 {
		hints = append(hints, MutationHint{
			Kind:    "leverage_facts",
			Message: "use existing facts to narrow exploration scope",
		})
	}

	return hints
}

// generateToolMutationHints suggests alternative tools for parse failures.
func (m *Mutator) generateToolMutationHints(attempt Attempt) []MutationHint {
	hints := []MutationHint{}

	transcript := strings.ToLower(attempt.PartialTranscript)

	// Suggest alternative parsers
	if strings.Contains(transcript, "json") {
		hints = append(hints, MutationHint{
			Kind:    "alternative_parser",
			Message: "try lenient JSON parser or raw output capture",
		})
	}

	if strings.Contains(transcript, "xml") || strings.Contains(transcript, "html") {
		hints = append(hints, MutationHint{
			Kind:    "alternative_parser",
			Message: "try HTML-aware parser or text extraction",
		})
	}

	// Generic fallback
	hints = append(hints, MutationHint{
		Kind:    "alternative_tool",
		Message: "consider alternative tool with simpler output format",
	})

	return hints
}

// generateRetryHints suggests retry strategies for worker exits.
func (m *Mutator) generateRetryHints(attempt Attempt) []MutationHint {
	hints := []MutationHint{}

	// Suggest reducing concurrency
	hints = append(hints, MutationHint{
		Kind:    "reduce_concurrency",
		Message: "reduce parallel operations to conserve resources",
	})

	// Suggest adding delays
	hints = append(hints, MutationHint{
		Kind:    "add_delay",
		Message: "add throttling delay between operations",
	})

	return hints
}

// deduplicateHints removes duplicate mutation hints.
func (m *Mutator) deduplicateHints(hints []MutationHint) []MutationHint {
	seen := make(map[string]bool)
	unique := []MutationHint{}

	for _, hint := range hints {
		key := fmt.Sprintf("%s:%s", hint.Kind, hint.Message)
		if !seen[key] {
			seen[key] = true
			unique = append(unique, hint)
		}
	}

	return unique
}

// rankHints orders hints by relevance to the failure.
func (m *Mutator) rankHints(hints []MutationHint, failure FailureReason) []MutationHint {
	// Simple ranking: explicit hints first, then generated hints
	explicit := []MutationHint{}
	generated := []MutationHint{}

	for _, hint := range hints {
		if hint.Kind == "reusable_lead" {
			explicit = append(explicit, hint)
		} else {
			generated = append(generated, hint)
		}
	}

	return append(explicit, generated...)
}

// MutateIntent creates a modified intent based on mutation hints.
func (m *Mutator) MutateIntent(original blackboard.Intent, hints []MutationHint) blackboard.Intent {
	mutated := original
	mutated.ID = "" // Will be assigned new ID on creation

	// Apply hints to goal
	for _, hint := range hints {
		switch hint.Kind {
		case "reduce_scope":
			mutated.Goal = m.applyScopeReduction(mutated.Goal, hint.Message)
		case "prioritize":
			mutated.Goal = m.applyPrioritization(mutated.Goal, hint.Message)
		case "alternative_tool":
			mutated.Goal = m.applyToolMutation(mutated.Goal, hint.Message)
		}
	}

	return mutated
}

// applyScopeReduction modifies goal to reduce scope.
func (m *Mutator) applyScopeReduction(goal string, hint string) string {
	if strings.Contains(strings.ToLower(goal), "enumerate") {
		return strings.Replace(goal, "enumerate", "focus enumeration on", 1)
	}
	return goal + " (scope: " + hint + ")"
}

// applyPrioritization modifies goal to adjust priority.
func (m *Mutator) applyPrioritization(goal string, hint string) string {
	return goal + " (priority: " + hint + ")"
}

// applyToolMutation modifies goal to suggest tool changes.
func (m *Mutator) applyToolMutation(goal string, hint string) string {
	return goal + " (tool: " + hint + ")"
}

// AnalyzeMutationHistory tracks mutation effectiveness over time.
type MutationHistory struct {
	Mutations      []MutationRecord
	SuccessRate    float64
	MostEffective  []string
	LeastEffective []string
}

type MutationRecord struct {
	IntentID      string
	Hints         []MutationHint
	Succeeded     bool
	FactsProduced int
}

// TrackMutation records a mutation attempt outcome.
func (mh *MutationHistory) TrackMutation(record MutationRecord) {
	mh.Mutations = append(mh.Mutations, record)

	// Recalculate success rate
	successes := 0
	for _, m := range mh.Mutations {
		if m.Succeeded {
			successes++
		}
	}
	if len(mh.Mutations) > 0 {
		mh.SuccessRate = float64(successes) / float64(len(mh.Mutations))
	}
}

// GetMutationRecommendations suggests mutation types based on history.
func (mh *MutationHistory) GetMutationRecommendations() []string {
	if len(mh.Mutations) < 3 {
		return []string{"insufficient_history"}
	}

	kindSuccess := make(map[string]int)
	kindTotal := make(map[string]int)

	for _, record := range mh.Mutations {
		for _, hint := range record.Hints {
			kindTotal[hint.Kind]++
			if record.Succeeded {
				kindSuccess[hint.Kind]++
			}
		}
	}

	recommendations := []string{}
	for kind, total := range kindTotal {
		if total >= 2 {
			successRate := float64(kindSuccess[kind]) / float64(total)
			if successRate > 0.6 {
				recommendations = append(recommendations, kind)
			}
		}
	}

	return recommendations
}
