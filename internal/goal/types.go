package goal

import "sync"

type GoalID string

type GoalType string

const (
	GoalTypeMission  GoalType = "mission"
	GoalTypeCoverage GoalType = "coverage"
	GoalTypeEvidence GoalType = "evidence"
	GoalTypeReport   GoalType = "report"
)

type Priority int

const (
	PriorityLow      Priority = 1
	PriorityMedium   Priority = 2
	PriorityHigh     Priority = 3
	PriorityCritical Priority = 4
)

type GoalStatus string

const (
	GoalStatusPending  GoalStatus = "pending"
	GoalStatusActive   GoalStatus = "active"
	GoalStatusComplete GoalStatus = "complete"
)

type GoalNode struct {
	ID           GoalID
	Type         GoalType
	Title        string
	Priority     Priority
	Dependencies []GoalID
	Status       GoalStatus
	Progress     float64
}

type GoalStack struct {
	Mission         string
	Coverage        []CoverageGoal
	Evidence        []EvidenceGoal
	Report          ReportGoal
	Weights         Weights
	SuccessCriteria []string

	// Runtime state (initialized lazily)
	mu          sync.RWMutex
	queue       *PriorityQueue
	depGraph    *DependencyGraph
	goals       map[GoalID]*GoalNode
	completed   map[GoalID]bool
	activeGoals map[GoalID]bool
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

type CompletionSnapshot struct {
	Total     int
	Completed int
	Progress  float64
	ByType    map[GoalType]TypeCompletion
}

type TypeCompletion struct {
	Total     int
	Completed int
	Progress  float64
}
