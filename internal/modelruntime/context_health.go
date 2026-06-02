package modelruntime

type ContextState string

const (
	ContextOK              ContextState = "ok"
	ContextRollingSummary  ContextState = "rolling_summary"
	ContextCompactAttempts ContextState = "compact_attempts"
	ContextCheckpoint      ContextState = "checkpoint"
)

type ContextWindow struct {
	UsedTokens    int
	MaxTokens     int
	PinnedObjects []string
}

type ContextHealth struct {
	UsedTokens    int
	MaxTokens     int
	PercentUsed   int
	State         ContextState
	NextThreshold int
	PinnedObjects []string
}

func EvaluateContext(window ContextWindow) ContextHealth {
	percent := 0
	if window.MaxTokens > 0 {
		percent = window.UsedTokens * 100 / window.MaxTokens
	}

	state := ContextOK
	nextThreshold := 70
	switch {
	case percent >= 95:
		state = ContextCheckpoint
		nextThreshold = 100
	case percent >= 85:
		state = ContextCompactAttempts
		nextThreshold = 95
	case percent >= 70:
		state = ContextRollingSummary
		nextThreshold = 85
	}

	return ContextHealth{
		UsedTokens:    window.UsedTokens,
		MaxTokens:     window.MaxTokens,
		PercentUsed:   percent,
		State:         state,
		NextThreshold: nextThreshold,
		PinnedObjects: append([]string(nil), window.PinnedObjects...),
	}
}
