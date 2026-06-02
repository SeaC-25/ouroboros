package modelruntime

import "time"

type UsageRecord struct {
	RunID            string
	AgentID          string
	Role             string
	Provider         string
	Model            string
	InputTokens      int
	OutputTokens     int
	CacheReadTokens  int
	CacheWriteTokens int
	Cost             float64
	Latency          time.Duration
	Failed           bool
}

type Aggregate struct {
	InputTokens      int
	OutputTokens     int
	CacheReadTokens  int
	CacheWriteTokens int
	Cost             float64
	Latency          time.Duration
	FailureCount     int
}
