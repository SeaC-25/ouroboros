package frontier

import "testing"

func TestScoreRewardsHighMissionValue(t *testing.T) {
	low := CandidatePath{
		MissionValue:       0.2,
		UnlockPotential:    0.4,
		EvidenceConfidence: 0.4,
		Novelty:            0.2,
		Cost:               0.2,
	}
	high := low
	high.MissionValue = 0.9

	if Score(high) <= Score(low) {
		t.Fatalf("expected high mission value to score higher: high=%f low=%f", Score(high), Score(low))
	}
}

func TestScoreAppliesExhaustedPenalty(t *testing.T) {
	path := CandidatePath{
		MissionValue:       0.8,
		UnlockPotential:    0.6,
		EvidenceConfidence: 0.5,
		Novelty:            0.3,
		Cost:               0.2,
	}
	exhausted := path
	exhausted.ExhaustedPenalty = 1.0

	if Score(exhausted) >= Score(path) {
		t.Fatalf("expected exhausted path to score lower: exhausted=%f path=%f", Score(exhausted), Score(path))
	}
}

func TestScoreAppliesDuplicatePenalty(t *testing.T) {
	path := CandidatePath{
		MissionValue:       0.7,
		UnlockPotential:    0.7,
		EvidenceConfidence: 0.6,
		Novelty:            0.4,
		Cost:               0.2,
	}
	duplicate := path
	duplicate.DuplicateRisk = 0.8

	if Score(duplicate) >= Score(path) {
		t.Fatalf("expected duplicate path to score lower: duplicate=%f path=%f", Score(duplicate), Score(path))
	}
}

func TestScoreRewardsNovelty(t *testing.T) {
	path := CandidatePath{
		MissionValue:       0.4,
		UnlockPotential:    0.4,
		EvidenceConfidence: 0.4,
		Cost:               0.2,
	}
	novel := path
	novel.Novelty = 0.9

	if Score(novel) <= Score(path) {
		t.Fatalf("expected novel path to score higher: novel=%f path=%f", Score(novel), Score(path))
	}
}

func TestQueuePopsHighestScoreFirst(t *testing.T) {
	queue := NewQueue()
	queue.Push(CandidatePath{
		ID:                 "low",
		Target:             "https://example.test",
		Hypothesis:         "low confidence path",
		MissionValue:       0.2,
		UnlockPotential:    0.1,
		EvidenceConfidence: 0.1,
		Cost:               0.1,
	})
	queue.Push(CandidatePath{
		ID:                 "high",
		Target:             "https://example.test/admin",
		Hypothesis:         "high value admin path",
		ExpectedUnlock:     "role escalation evidence",
		MissionValue:       0.9,
		UnlockPotential:    0.8,
		EvidenceConfidence: 0.7,
		Novelty:            0.4,
		Cost:               0.2,
	})

	next, ok := queue.Pop()
	if !ok {
		t.Fatalf("expected queued path")
	}
	if next.ID != "high" {
		t.Fatalf("expected highest score first, got %s", next.ID)
	}
}
