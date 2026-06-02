package modelruntime

type Ledger struct {
	records []UsageRecord
}

func NewLedger() *Ledger {
	return &Ledger{}
}

func (l *Ledger) Record(record UsageRecord) {
	l.records = append(l.records, record)
}

func (l *Ledger) ByAgent(agentID string) Aggregate {
	return l.aggregate(func(record UsageRecord) bool {
		return record.AgentID == agentID
	})
}

func (l *Ledger) ByRole(role string) Aggregate {
	return l.aggregate(func(record UsageRecord) bool {
		return record.Role == role
	})
}

func (l *Ledger) ByRun(runID string) Aggregate {
	return l.aggregate(func(record UsageRecord) bool {
		return record.RunID == runID
	})
}

func (l *Ledger) ByModel(model string) Aggregate {
	return l.aggregate(func(record UsageRecord) bool {
		return record.Model == model
	})
}

func (l *Ledger) aggregate(match func(UsageRecord) bool) Aggregate {
	var aggregate Aggregate
	for _, record := range l.records {
		if !match(record) {
			continue
		}
		aggregate.InputTokens += record.InputTokens
		aggregate.OutputTokens += record.OutputTokens
		aggregate.CacheReadTokens += record.CacheReadTokens
		aggregate.CacheWriteTokens += record.CacheWriteTokens
		aggregate.Cost += record.Cost
		aggregate.Latency += record.Latency
		if record.Failed {
			aggregate.FailureCount++
		}
	}
	return aggregate
}
