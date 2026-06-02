package frontier

type CandidatePath struct {
	ID                 string
	Target             string
	Hypothesis         string
	ExpectedUnlock     string
	MissionValue       float64
	UnlockPotential    float64
	EvidenceConfidence float64
	Novelty            float64
	Cost               float64
	DuplicateRisk      float64
	Noise              float64
	ExhaustedPenalty   float64
}
