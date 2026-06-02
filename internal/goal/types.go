package goal

type GoalStack struct {
	Mission         string
	Coverage        []CoverageGoal
	Evidence        []EvidenceGoal
	Report          ReportGoal
	Weights         Weights
	SuccessCriteria []string
}

type CoverageGoal struct {
	Dimension string
	Target    string
}

type EvidenceGoal struct {
	Kind     string
	Required bool
}

type ReportGoal struct {
	Format   string
	Sections []string
}

type Weights struct {
	Mission  float64
	Coverage float64
	Evidence float64
	Report   float64
}
