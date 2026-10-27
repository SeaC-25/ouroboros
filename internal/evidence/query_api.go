package evidence

import (
	"errors"
	"fmt"
	"sort"
)

var (
	// ErrQueryGraphRequired is returned when a query is executed without a graph.
	ErrQueryGraphRequired = errors.New("evidence query graph is required")

	// ErrQueryFromRequired is returned when a query has no source node ID.
	ErrQueryFromRequired = errors.New("evidence query from node is required")

	// ErrQueryToRequired is returned when a query has no destination node ID.
	ErrQueryToRequired = errors.New("evidence query to node is required")

	// ErrNodeNotFound is wrapped when either endpoint does not exist in the graph.
	ErrNodeNotFound = errors.New("node not found")
)

// Projector is the evidence graph projection queried by QueryBuilder.
type Projector = Graph

// QueryBuilder builds a directed evidence graph path query.
type QueryBuilder struct {
	graph *Projector
	from  string
	to    string
}

// Path is a directed traversal from one evidence node to another.
type Path struct {
	Nodes []string
	Edges []Edge
}

// From starts a query from nodeID.
func (g Graph) From(nodeID string) *QueryBuilder {
	return (&QueryBuilder{graph: (*Projector)(&g)}).From(nodeID)
}

// To starts a query that targets nodeID.
func (g Graph) To(nodeID string) *QueryBuilder {
	return (&QueryBuilder{graph: (*Projector)(&g)}).To(nodeID)
}

// From sets the source node for the query.
func (q *QueryBuilder) From(nodeID string) *QueryBuilder {
	if q == nil {
		return &QueryBuilder{from: nodeID}
	}
	q.from = nodeID
	return q
}

// To sets the destination node for the query.
func (q *QueryBuilder) To(nodeID string) *QueryBuilder {
	if q == nil {
		return &QueryBuilder{to: nodeID}
	}
	q.to = nodeID
	return q
}

// Execute returns all shortest directed paths between the selected endpoints.
func (q *QueryBuilder) Execute() ([]Path, error) {
	if q == nil || q.graph == nil {
		return nil, ErrQueryGraphRequired
	}
	if q.from == "" {
		return nil, ErrQueryFromRequired
	}
	if q.to == "" {
		return nil, ErrQueryToRequired
	}
	if _, ok := q.graph.Node(q.from); !ok {
		return nil, fmt.Errorf("evidence query from node %q: %w", q.from, ErrNodeNotFound)
	}
	if _, ok := q.graph.Node(q.to); !ok {
		return nil, fmt.Errorf("evidence query to node %q: %w", q.to, ErrNodeNotFound)
	}
	if q.from == q.to {
		return []Path{{Nodes: []string{q.from}}}, nil
	}

	return findShortestPaths(q.graph, q.from, q.to), nil
}

type pathSearchState struct {
	node  string
	path  Path
	seen  map[string]struct{}
	depth int
}

func findShortestPaths(graph *Projector, from string, to string) []Path {
	outgoing := outgoingEdges(graph.Edges())
	queue := []pathSearchState{{
		node: from,
		path: Path{Nodes: []string{from}},
		seen: map[string]struct{}{from: {}},
	}}

	shortestDepth := -1
	paths := []Path{}

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		if shortestDepth >= 0 && current.depth >= shortestDepth {
			continue
		}

		for _, edge := range outgoing[current.node] {
			if _, ok := current.seen[edge.To]; ok {
				continue
			}

			nextPath := appendPathEdge(current.path, edge)
			nextDepth := current.depth + 1
			if edge.To == to {
				if shortestDepth == -1 {
					shortestDepth = nextDepth
				}
				if nextDepth == shortestDepth {
					paths = append(paths, nextPath)
				}
				continue
			}

			if shortestDepth >= 0 && nextDepth >= shortestDepth {
				continue
			}

			nextSeen := cloneSeen(current.seen)
			nextSeen[edge.To] = struct{}{}
			queue = append(queue, pathSearchState{
				node:  edge.To,
				path:  nextPath,
				seen:  nextSeen,
				depth: nextDepth,
			})
		}
	}

	return paths
}

func outgoingEdges(edges []Edge) map[string][]Edge {
	outgoing := make(map[string][]Edge)
	for _, edge := range edges {
		if edge.From == "" || edge.To == "" {
			continue
		}
		outgoing[edge.From] = append(outgoing[edge.From], edge)
	}
	for from := range outgoing {
		sort.SliceStable(outgoing[from], func(i int, j int) bool {
			left := outgoing[from][i]
			right := outgoing[from][j]
			if left.To != right.To {
				return left.To < right.To
			}
			if left.Type != right.Type {
				return left.Type < right.Type
			}
			return left.From < right.From
		})
	}
	return outgoing
}

func appendPathEdge(path Path, edge Edge) Path {
	next := Path{
		Nodes: make([]string, 0, len(path.Nodes)+1),
		Edges: make([]Edge, 0, len(path.Edges)+1),
	}
	next.Nodes = append(next.Nodes, path.Nodes...)
	next.Nodes = append(next.Nodes, edge.To)
	next.Edges = append(next.Edges, path.Edges...)
	next.Edges = append(next.Edges, edge)
	return next
}

func cloneSeen(seen map[string]struct{}) map[string]struct{} {
	cloned := make(map[string]struct{}, len(seen))
	for nodeID := range seen {
		cloned[nodeID] = struct{}{}
	}
	return cloned
}
