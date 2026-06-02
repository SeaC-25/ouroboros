package swarm

import "context"

type Driver interface {
	Run(ctx context.Context, task AgentTask) (AgentResult, error)
}
