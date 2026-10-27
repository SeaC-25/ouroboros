package evidence

import (
	"strings"

	"github.com/ouroboros/ouroboros/internal/blackboard"
)

func Project(state blackboard.ProjectState) Graph {
	graph := Graph{
		nodes: make(map[string]Node),
	}

	for _, fact := range state.Facts {
		graph.addNode(Node{
			ID:         fact.ID,
			Type:       nodeTypeForFact(fact.Kind),
			Label:      fact.Statement,
			SourceKind: fact.Kind,
		})
	}

	// Observations: raw pre-interpretation evidence becomes Artifact nodes so
	// the graph shows what was measured before any conclusion was drawn.
	for _, observation := range state.Observations {
		graph.addNode(Node{
			ID:         observation.ID,
			Type:       NodeArtifact,
			Label:      observation.Target,
			SourceKind: observation.Kind,
		})
	}

	for _, intent := range state.Intents {
		graph.addNode(Node{
			ID:         intent.ID,
			Type:       NodeHypothesis,
			Label:      intent.Goal,
			SourceKind: intent.Kind,
		})
		for _, factID := range intent.SourceFacts {
			graph.addEdge(Edge{
				From: intent.ID,
				To:   factID,
				Type: EdgeRequires,
			})
		}
		if intent.ResultFact != "" {
			graph.addEdge(Edge{
				From: intent.ResultFact,
				To:   intent.ID,
				Type: EdgeDerivedFrom,
			})
		}
	}

	return graph
}

func (g *Graph) addNode(node Node) {
	if node.ID == "" {
		return
	}
	g.nodes[node.ID] = node
}

func (g *Graph) addEdge(edge Edge) {
	if edge.From == "" || edge.To == "" {
		return
	}
	g.edges = append(g.edges, edge)
}

func nodeTypeForFact(kind string) NodeType {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "target":
		return NodeTarget
	case "surface":
		return NodeSurface
	case "identity":
		return NodeIdentity
	case "artifact":
		return NodeArtifact
	case "finding":
		return NodeFinding
	case "attempt":
		return NodeAttempt
	case "agent":
		return NodeAgent
	case "hypothesis":
		return NodeHypothesis
	default:
		return NodeArtifact
	}
}
