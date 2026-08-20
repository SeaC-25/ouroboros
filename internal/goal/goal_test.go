package goal

import "testing"

func TestValidateAcceptsCompleteGoalStack(t *testing.T) {
	stack := GoalStack{
		Mission: "verify high-value web findings",
		Coverage: []CoverageGoal{
			{Dimension: "routes", Target: "discover and test authenticated routes"},
		},
		Evidence: []EvidenceGoal{
			{Kind: "raw_output", Required: true},
			{Kind: "reproduction", Required: true},
		},
		Report: ReportGoal{
			Format:   "markdown",
			Sections: []string{"summary", "findings", "evidence", "next_steps"},
		},
		Weights: Weights{
			Mission:  0.45,
			Coverage: 0.20,
			Evidence: 0.25,
			Report:   0.10,
		},
		SuccessCriteria: []string{
			"at least one verified finding or all high-value paths exhausted",
		},
	}

	if err := Validate(&stack); err != nil {
		t.Fatalf("expected valid stack: %v", err)
	}
}

func TestValidateRejectsEmptyMission(t *testing.T) {
	stack := validGoalStack()
	stack.Mission = " "

	if err := Validate(&stack); err == nil {
		t.Fatalf("expected empty mission to fail")
	}
}

func TestValidateRejectsUnnormalizedWeights(t *testing.T) {
	stack := validGoalStack()
	stack.Weights.Report = 0.40

	if err := Validate(&stack); err == nil {
		t.Fatalf("expected unnormalized weights to fail")
	}
}

func TestValidateRejectsMissingSuccessCriteria(t *testing.T) {
	stack := validGoalStack()
	stack.SuccessCriteria = nil

	if err := Validate(&stack); err == nil {
		t.Fatalf("expected missing success criteria to fail")
	}
}

func validGoalStack() GoalStack {
	return GoalStack{
		Mission: "obtain mission progress through verified evidence",
		Coverage: []CoverageGoal{
			{Dimension: "services", Target: "fingerprint exposed services"},
		},
		Evidence: []EvidenceGoal{
			{Kind: "artifact_ref", Required: true},
		},
		Report: ReportGoal{
			Format:   "markdown",
			Sections: []string{"summary"},
		},
		Weights: Weights{
			Mission:  0.40,
			Coverage: 0.20,
			Evidence: 0.30,
			Report:   0.10,
		},
		SuccessCriteria: []string{"mission progress is proven by evidence"},
	}
}
