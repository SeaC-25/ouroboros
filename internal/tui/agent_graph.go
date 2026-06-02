package tui

import (
	"fmt"
	"sort"

	"github.com/ouroboros/ouroboros/internal/swarm"
)

type AgentGraphNode struct {
	AgentID         string
	Role            swarm.AgentRole
	State           swarm.AgentState
	ProgressPercent int
}

type AgentGraph struct {
	Agents          []AgentGraphNode
	SelectedAgentID string
}

func (g AgentGraph) SelectAgent(agentID string) AgentGraph {
	g.SelectedAgentID = agentID
	return g
}

func (g AgentGraph) SelectedAgent() (AgentGraphNode, bool) {
	for _, agent := range g.Agents {
		if agent.AgentID == g.SelectedAgentID {
			return agent, true
		}
	}
	return AgentGraphNode{}, false
}

func (g AgentGraph) Render(width int, height int) string {
	return renderPanel("Agent Graph", g.Rows(), width, height)
}

func (g AgentGraph) Rows() []string {
	agents := append([]AgentGraphNode(nil), g.Agents...)
	sort.SliceStable(agents, func(i int, j int) bool {
		return agents[i].AgentID < agents[j].AgentID
	})
	rows := make([]string, 0, len(agents))
	for _, agent := range agents {
		marker := "  "
		if agent.AgentID == g.SelectedAgentID {
			marker = "> "
		}
		rows = append(rows, fmt.Sprintf(
			"%s%s %s %s %d%%",
			marker,
			valueOrDefault(agent.AgentID, "agent"),
			valueOrDefault(string(agent.Role), "unknown"),
			valueOrDefault(string(agent.State), "unknown"),
			agent.ProgressPercent,
		))
	}
	if len(rows) == 0 {
		rows = append(rows, "no agent nodes")
	}
	return rows
}
