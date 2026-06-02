package evidence

type NodeType string

const (
	NodeTarget     NodeType = "Target"
	NodeSurface    NodeType = "Surface"
	NodeIdentity   NodeType = "Identity"
	NodeArtifact   NodeType = "Artifact"
	NodeFinding    NodeType = "Finding"
	NodeAttempt    NodeType = "Attempt"
	NodeHypothesis NodeType = "Hypothesis"
	NodeAgent      NodeType = "Agent"
)

type EdgeType string

const (
	EdgeObservedBy  EdgeType = "observed_by"
	EdgeSupports    EdgeType = "supports"
	EdgeContradicts EdgeType = "contradicts"
	EdgeDerivedFrom EdgeType = "derived_from"
	EdgeVerifiedBy  EdgeType = "verified_by"
	EdgeRequires    EdgeType = "requires"
	EdgeUnlocks     EdgeType = "unlocks"
	EdgeExhaustedBy EdgeType = "exhausted_by"
	EdgeAssignedTo  EdgeType = "assigned_to"
	EdgeProducedBy  EdgeType = "produced_by"
)

type Node struct {
	ID         string
	Type       NodeType
	Label      string
	SourceKind string
}

type Edge struct {
	From string
	To   string
	Type EdgeType
}

type Graph struct {
	nodes map[string]Node
	edges []Edge
}

func (g Graph) Node(id string) (Node, bool) {
	node, ok := g.nodes[id]
	return node, ok
}

func (g Graph) Nodes() []Node {
	nodes := make([]Node, 0, len(g.nodes))
	for _, node := range g.nodes {
		nodes = append(nodes, node)
	}
	return nodes
}

func (g Graph) Edges() []Edge {
	return append([]Edge(nil), g.edges...)
}

func (g Graph) EdgesOfType(edgeType EdgeType) []Edge {
	edges := []Edge{}
	for _, edge := range g.edges {
		if edge.Type == edgeType {
			edges = append(edges, edge)
		}
	}
	return edges
}
