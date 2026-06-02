package sisyphus

import (
	"errors"
	"fmt"
	"strings"

	"github.com/ouroboros/ouroboros/internal/blackboard"
)

type Engine struct{}

func (e Engine) Step(state blackboard.ProjectState) Task {
	if len(state.Facts) == 0 && len(state.Intents) == 0 {
		return Task{
			Kind:   TaskBootstrap,
			Reason: "project has no facts or intents",
		}
	}

	for _, intent := range state.Intents {
		if intent.State == blackboard.IntentOpen {
			return Task{
				Kind:     TaskExplore,
				IntentID: intent.ID,
				Reason:   "open intent is ready for exploration",
			}
		}
	}

	return Task{
		Kind:   TaskReason,
		Reason: "no open intents are available",
	}
}

func (e Engine) ConcludeFallback(store *blackboard.MemoryStore, intentID string, leaseID string, attempt Attempt, failure FailureReason) (TaskResult, error) {
	if store == nil {
		return TaskResult{}, errors.New("blackboard store is required")
	}
	if strings.TrimSpace(intentID) == "" {
		return TaskResult{}, errors.New("intent id is required")
	}
	if strings.TrimSpace(leaseID) == "" {
		return TaskResult{}, errors.New("lease id is required")
	}
	if !isFallbackFailure(failure.Kind) {
		return TaskResult{}, fmt.Errorf("unsupported fallback failure kind: %s", failure.Kind)
	}

	summary := summarizePartialTranscript(attempt.PartialTranscript)
	if summary == "" {
		summary = "attempt failed before producing reusable transcript"
	}

	concluded, err := store.ConcludeIntent(intentID, leaseID, blackboard.Fact{
		Kind:       "attempt_summary",
		Statement:  fmt.Sprintf("attempt %s concluded after %s: %s", attempt.ID, failure.Kind, summary),
		Confidence: 0.5,
		Source:     "sisyphus.conclude",
	})
	if err != nil {
		return TaskResult{}, err
	}

	return TaskResult{
		Task: Task{
			Kind:     TaskConclude,
			IntentID: intentID,
			Reason:   "attempt failed with partial transcript",
		},
		Attempt:       attempt,
		Failure:       failure,
		Fact:          concluded.Fact,
		MutationHints: extractReusableLeads(attempt.PartialTranscript),
	}, nil
}

func isFallbackFailure(kind string) bool {
	switch kind {
	case FailureTimeout, FailureParseFailed, FailureWorkerExit:
		return true
	default:
		return false
	}
}

func summarizePartialTranscript(transcript string) string {
	for _, line := range strings.Split(transcript, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(strings.ToLower(line), "next command:") {
			continue
		}
		return line
	}
	return ""
}

func extractReusableLeads(transcript string) []MutationHint {
	hints := []MutationHint{}
	for _, line := range strings.Split(transcript, "\n") {
		line = strings.TrimSpace(line)
		lower := strings.ToLower(line)
		switch {
		case strings.HasPrefix(lower, "lead:"):
			hints = append(hints, MutationHint{Kind: "reusable_lead", Message: strings.TrimSpace(line[len("lead:"):])})
		case strings.HasPrefix(lower, "hint:"):
			hints = append(hints, MutationHint{Kind: "reusable_lead", Message: strings.TrimSpace(line[len("hint:"):])})
		case strings.HasPrefix(lower, "reusable:"):
			hints = append(hints, MutationHint{Kind: "reusable_lead", Message: strings.TrimSpace(line[len("reusable:"):])})
		}
	}
	return hints
}
