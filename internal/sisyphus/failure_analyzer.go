package sisyphus

import (
	"strings"
)

// FailureAnalyzer classifies failure patterns and extracts actionable insights.
type FailureAnalyzer struct{}

// AnalyzeFailure performs deep analysis of an attempt failure.
func (a *FailureAnalyzer) AnalyzeFailure(attempt Attempt, failure FailureReason) FailureAnalysis {
	analysis := FailureAnalysis{
		PrimaryCategory:   a.categorizeFailure(failure),
		Severity:          a.assessSeverity(failure, attempt),
		RecoveryStrategy:  a.suggestRecovery(failure, attempt),
		ExtractedInsights: a.extractInsights(attempt.PartialTranscript),
		SalvageValue:      a.assessSalvageValue(attempt.PartialTranscript),
	}

	return analysis
}

// categorizeFailure maps failure kinds to broader categories.
func (a *FailureAnalyzer) categorizeFailure(failure FailureReason) FailureCategory {
	switch failure.Kind {
	case FailureTimeout:
		return CategoryResourceExhaustion
	case FailureParseFailed:
		return CategoryDataQuality
	case FailureWorkerExit:
		return CategoryProcessFailure
	default:
		return CategoryUnknown
	}
}

// assessSeverity determines how critical the failure is.
func (a *FailureAnalyzer) assessSeverity(failure FailureReason, attempt Attempt) FailureSeverity {
	// Timeout with substantial transcript is less severe
	if failure.Kind == FailureTimeout && len(attempt.PartialTranscript) > 100 {
		return SeverityModerate
	}

	// Parser failures are generally recoverable
	if failure.Kind == FailureParseFailed {
		return SeverityLow
	}

	// Worker exits are more concerning
	if failure.Kind == FailureWorkerExit {
		return SeverityHigh
	}

	return SeverityModerate
}

// suggestRecovery recommends next steps based on failure pattern.
func (a *FailureAnalyzer) suggestRecovery(failure FailureReason, attempt Attempt) RecoveryStrategy {
	switch failure.Kind {
	case FailureTimeout:
		if strings.Contains(strings.ToLower(attempt.PartialTranscript), "scope") {
			return RecoveryReduceScope
		}
		return RecoveryIncreaseTimeout
	case FailureParseFailed:
		return RecoveryRetryWithDifferentParser
	case FailureWorkerExit:
		if strings.Contains(failure.Message, "resource") {
			return RecoveryWaitAndRetry
		}
		return RecoveryRetryWithDifferentWorker
	default:
		return RecoveryManualIntervention
	}
}

// extractInsights mines valuable information from partial transcripts.
func (a *FailureAnalyzer) extractInsights(transcript string) []Insight {
	insights := []Insight{}

	lines := strings.Split(transcript, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		lower := strings.ToLower(line)

		// Detect observation patterns
		if strings.Contains(lower, "observed") || strings.Contains(lower, "detected") {
			insights = append(insights, Insight{
				Kind:    InsightObservation,
				Content: line,
				Value:   0.7,
			})
		}

		// Detect error indicators
		if strings.Contains(lower, "error") || strings.Contains(lower, "failed") {
			insights = append(insights, Insight{
				Kind:    InsightError,
				Content: line,
				Value:   0.5,
			})
		}

		// Detect valuable endpoints
		if strings.Contains(lower, "/") && (strings.Contains(lower, "admin") || strings.Contains(lower, "api")) {
			insights = append(insights, Insight{
				Kind:    InsightEndpoint,
				Content: line,
				Value:   0.8,
			})
		}

		// Detect credentials or tokens
		if strings.Contains(lower, "token") || strings.Contains(lower, "credential") || strings.Contains(lower, "password") {
			insights = append(insights, Insight{
				Kind:    InsightCredential,
				Content: line,
				Value:   0.9,
			})
		}
	}

	return insights
}

// assessSalvageValue determines how much useful information can be extracted.
func (a *FailureAnalyzer) assessSalvageValue(transcript string) float64 {
	if len(transcript) == 0 {
		return 0.0
	}

	score := 0.0
	lines := strings.Split(transcript, "\n")
	meaningfulLines := 0

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(strings.ToLower(line), "next command") {
			continue
		}
		meaningfulLines++

		lower := strings.ToLower(line)
		if strings.Contains(lower, "lead:") || strings.Contains(lower, "hint:") || strings.Contains(lower, "reusable:") {
			score += 0.3
		}
		if strings.Contains(lower, "observed") || strings.Contains(lower, "detected") || strings.Contains(lower, "confirmed") {
			score += 0.2
		}
	}

	// Normalize by meaningful line count
	if meaningfulLines > 0 {
		score = score / float64(meaningfulLines)
		if score > 1.0 {
			score = 1.0
		}
	}

	return score
}

// ClassifyFailurePattern identifies recurring failure modes.
func (a *FailureAnalyzer) ClassifyFailurePattern(recentFailures []FailureReason) FailurePattern {
	if len(recentFailures) < 2 {
		return PatternIsolated
	}

	// Check for repeated failure kinds
	kindCounts := make(map[string]int)
	for _, failure := range recentFailures {
		kindCounts[failure.Kind]++
	}

	for _, count := range kindCounts {
		if count >= len(recentFailures)/2 {
			return PatternRecurring
		}
	}

	return PatternIntermittent
}

// FailureAnalysis contains detailed failure assessment.
type FailureAnalysis struct {
	PrimaryCategory   FailureCategory
	Severity          FailureSeverity
	RecoveryStrategy  RecoveryStrategy
	ExtractedInsights []Insight
	SalvageValue      float64
}

type FailureCategory string

const (
	CategoryResourceExhaustion FailureCategory = "resource_exhaustion"
	CategoryDataQuality        FailureCategory = "data_quality"
	CategoryProcessFailure     FailureCategory = "process_failure"
	CategoryUnknown            FailureCategory = "unknown"
)

type FailureSeverity string

const (
	SeverityLow      FailureSeverity = "low"
	SeverityModerate FailureSeverity = "moderate"
	SeverityHigh     FailureSeverity = "high"
	SeverityCritical FailureSeverity = "critical"
)

type RecoveryStrategy string

const (
	RecoveryReduceScope              RecoveryStrategy = "reduce_scope"
	RecoveryIncreaseTimeout          RecoveryStrategy = "increase_timeout"
	RecoveryRetryWithDifferentParser RecoveryStrategy = "retry_different_parser"
	RecoveryRetryWithDifferentWorker RecoveryStrategy = "retry_different_worker"
	RecoveryWaitAndRetry             RecoveryStrategy = "wait_and_retry"
	RecoveryManualIntervention       RecoveryStrategy = "manual_intervention"
)

type Insight struct {
	Kind    InsightKind
	Content string
	Value   float64
}

type InsightKind string

const (
	InsightObservation InsightKind = "observation"
	InsightError       InsightKind = "error"
	InsightEndpoint    InsightKind = "endpoint"
	InsightCredential  InsightKind = "credential"
	InsightLead        InsightKind = "lead"
)

type FailurePattern string

const (
	PatternIsolated    FailurePattern = "isolated"
	PatternRecurring   FailurePattern = "recurring"
	PatternIntermittent FailurePattern = "intermittent"
)
