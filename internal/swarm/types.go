package swarm

import "encoding/json"

type AgentRole string

const (
	RolePlanner  AgentRole = "planner"
	RoleWebAPI   AgentRole = "web-api"
	RoleInfra    AgentRole = "infra"
	RoleCTF      AgentRole = "ctf"
	RoleVerifier AgentRole = "verifier"
	RoleEvidence AgentRole = "evidence"
	RoleReporter AgentRole = "reporter"
	RoleContext  AgentRole = "context"
	RoleGraph    AgentRole = "graph"
)

type AgentState string

const (
	AgentIdle     AgentState = "idle"
	AgentQueued   AgentState = "queued"
	AgentRunning  AgentState = "running"
	AgentBlocked  AgentState = "blocked"
	AgentComplete AgentState = "complete"
	AgentFailed   AgentState = "failed"
)

type AgentTask struct {
	ID       string
	Role     AgentRole
	Goal     string
	IntentID string
}

type AgentResult struct {
	TaskID string
	State  AgentState
	Output StructuredOutput
}

type AgentThread struct {
	ID        string
	AgentID   string
	Role      AgentRole
	Context   []string
	TokenUsed int
}

type ProposalType string

const (
	ProposalFact     ProposalType = "fact"
	ProposalIntent   ProposalType = "intent"
	ProposalHint     ProposalType = "hint"
	ProposalFinding  ProposalType = "finding"
	ProposalMutation ProposalType = "mutation"
)

type StructuredOutput struct {
	Proposals []Proposal
}

type Proposal struct {
	Type    ProposalType
	Payload json.RawMessage
}

func KnownRoles() map[AgentRole]bool {
	return map[AgentRole]bool{
		RolePlanner:  true,
		RoleWebAPI:   true,
		RoleInfra:    true,
		RoleCTF:      true,
		RoleVerifier: true,
		RoleEvidence: true,
		RoleReporter: true,
		RoleContext:  true,
		RoleGraph:    true,
	}
}
