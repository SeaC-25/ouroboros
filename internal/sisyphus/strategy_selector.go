package sisyphus

import (
	"github.com/ouroboros/ouroboros/internal/blackboard"
)

// StrategySelector provides intelligent task scheduling based on project state.
type StrategySelector struct{}

// SelectNextTask determines the next task based on current project state.
// It implements a priority-based selection algorithm considering:
// - Project initialization state
// - Intent availability and priority
// - Fact coverage and quality
// - Reasoning opportunities
func (s *StrategySelector) SelectNextTask(state blackboard.ProjectState) Task {
	// Priority 1: Bootstrap empty projects
	if s.needsBootstrap(state) {
		return Task{
			Kind:   TaskBootstrap,
			Reason: "project has no facts or intents",
		}
	}

	// Priority 2: Explore open intents with highest potential
	if intent, ok := s.selectBestIntent(state); ok {
		return Task{
			Kind:     TaskExplore,
			IntentID: intent.ID,
			Reason:   "open intent is ready for exploration",
		}
	}

	// Priority 3: Reason about current facts to generate new intents
	if s.shouldReason(state) {
		return Task{
			Kind:   TaskReason,
			Reason: "no open intents are available",
		}
	}

	// Fallback: Reason (should rarely reach here)
	return Task{
		Kind:   TaskReason,
		Reason: "default reasoning fallback",
	}
}

// needsBootstrap checks if the project is in initial state.
func (s *StrategySelector) needsBootstrap(state blackboard.ProjectState) bool {
	return len(state.Facts) == 0 && len(state.Intents) == 0
}

// selectBestIntent finds the highest-priority open intent.
// Currently uses simple first-open selection, but can be extended with:
// - Intent priority scoring
// - Source fact quality assessment
// - Historical success rates
func (s *StrategySelector) selectBestIntent(state blackboard.ProjectState) (blackboard.Intent, bool) {
	for _, intent := range state.Intents {
		if intent.State == blackboard.IntentOpen {
			return intent, true
		}
	}
	return blackboard.Intent{}, false
}

// shouldReason determines if reasoning should be performed.
// Reasoning is valuable when:
// - Facts exist but no open intents
// - Recent facts suggest new exploration directions
func (s *StrategySelector) shouldReason(state blackboard.ProjectState) bool {
	return len(state.Facts) > 0
}

// ScoreIntent calculates a priority score for an intent based on:
// - Source fact quality (confidence scores)
// - Intent kind (explore > verify > document)
// - Coverage gaps it might fill
func (s *StrategySelector) ScoreIntent(intent blackboard.Intent, state blackboard.ProjectState) float64 {
	score := 1.0

	// Boost score based on source fact confidence
	factConfidenceSum := 0.0
	factCount := 0
	for _, factID := range intent.SourceFacts {
		for _, fact := range state.Facts {
			if fact.ID == factID {
				factConfidenceSum += fact.Confidence
				factCount++
				break
			}
		}
	}
	if factCount > 0 {
		score += factConfidenceSum / float64(factCount)
	}

	// Boost explore intents
	if intent.Kind == "explore" {
		score += 0.5
	}

	return score
}

// AnalyzeProgress assesses overall project progress based on state.
func (s *StrategySelector) AnalyzeProgress(state blackboard.ProjectState) ProgressMetrics {
	metrics := ProgressMetrics{
		TotalFacts:       len(state.Facts),
		TotalIntents:     len(state.Intents),
		OpenIntents:      0,
		ClaimedIntents:   0,
		ConcludedIntents: 0,
		AvgFactConfidence: 0.0,
	}

	for _, intent := range state.Intents {
		switch intent.State {
		case blackboard.IntentOpen:
			metrics.OpenIntents++
		case blackboard.IntentClaimed:
			metrics.ClaimedIntents++
		case blackboard.IntentConcluded:
			metrics.ConcludedIntents++
		}
	}

	if len(state.Facts) > 0 {
		confidenceSum := 0.0
		for _, fact := range state.Facts {
			confidenceSum += fact.Confidence
		}
		metrics.AvgFactConfidence = confidenceSum / float64(len(state.Facts))
	}

	return metrics
}

// ProgressMetrics captures project health indicators.
type ProgressMetrics struct {
	TotalFacts        int
	TotalIntents      int
	OpenIntents       int
	ClaimedIntents    int
	ConcludedIntents  int
	AvgFactConfidence float64
}
