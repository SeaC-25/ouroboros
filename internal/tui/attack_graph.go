package tui

import (
	"fmt"
	"sort"

	"github.com/ouroboros/ouroboros/internal/evidence"
)

type AttackGraph struct {
	Nodes          []evidence.Node
	Edges          []evidence.Edge
	SelectedNodeID string
}

func (g AttackGraph) SelectNode(nodeID string) AttackGraph {
	g.SelectedNodeID = nodeID
	return g
}

func (g AttackGraph) SelectedNode() (evidence.Node, bool) {
	for _, node := range g.Nodes {
		if node.ID == g.SelectedNodeID {
			return node, true
		}
	}
	return evidence.Node{}, false
}

func (g AttackGraph) Render(width int, height int) string {
	return renderPanel("Attack Graph", g.Rows(), width, height)
}

func (g AttackGraph) Rows() []string {
	rows := []string{}
	nodes := append([]evidence.Node(nil), g.Nodes...)
	sort.SliceStable(nodes, func(i int, j int) bool {
		return nodes[i].ID < nodes[j].ID
	})
	for _, node := range nodes {
		marker := "  "
		if node.ID == g.SelectedNodeID {
			marker = "> "
		}
		rows = append(rows, fmt.Sprintf("%s%s [%s] %s", marker, node.ID, node.Type, node.Label))
	}
	edges := append([]evidence.Edge(nil), g.Edges...)
	sort.SliceStable(edges, func(i int, j int) bool {
		if edges[i].From != edges[j].From {
			return edges[i].From < edges[j].From
		}
		if edges[i].Type != edges[j].Type {
			return edges[i].Type < edges[j].Type
		}
		return edges[i].To < edges[j].To
	})
	for _, edge := range edges {
		rows = append(rows, fmt.Sprintf("%s -%s-> %s", edge.From, edge.Type, edge.To))
	}
	if len(rows) == 0 {
		rows = append(rows, "no attack graph nodes")
	}
	return rows
}
