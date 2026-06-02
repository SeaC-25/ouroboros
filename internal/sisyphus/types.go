package sisyphus

import "github.com/ouroboros/ouroboros/internal/blackboard"

type TaskKind string

const (
	TaskBootstrap TaskKind = "bootstrap"
	TaskReason    TaskKind = "reason"
	TaskExplore   TaskKind = "explore"
	TaskConclude  TaskKind = "conclude"
)

type Task struct {
	Kind     TaskKind
	IntentID string
	Reason   string
}

type Attempt struct {
	ID                string
	IntentID          string
	TaskKind          TaskKind
	PartialTranscript string
}

type FailureReason struct {
	Kind    string
	Message string
}

const (
	FailureTimeout     = "timeout"
	FailureParseFailed = "parse_failed"
	FailureWorkerExit  = "worker_exit"
)

type MutationHint struct {
	Kind    string
	Message string
}

type TaskResult struct {
	Task          Task
	Attempt       Attempt
	Failure       FailureReason
	Fact          blackboard.Fact
	MutationHints []MutationHint
}
