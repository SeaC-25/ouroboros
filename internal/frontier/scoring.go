package frontier

func Score(path CandidatePath) float64 {
	return path.MissionValue +
		path.UnlockPotential +
		path.EvidenceConfidence +
		path.Novelty -
		path.Cost -
		path.DuplicateRisk -
		path.Noise -
		path.ExhaustedPenalty
}
