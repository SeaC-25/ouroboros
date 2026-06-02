package report

import (
	"sort"
	"strings"

	"github.com/ouroboros/ouroboros/internal/evidence"
)

type Report struct {
	VerifiedFindings   []Finding
	Paths              []Path
	EvidenceRefs       []EvidenceRef
	ExhaustedAreas     []string
	NextRunSuggestions []string
}

type Finding struct {
	ID     string
	Title  string
	Source string
}

type Path struct {
	FindingID string
	Steps     []string
}

type EvidenceRef struct {
	ID     string
	Ref    string
	Source string
}

func Project(graph evidence.Graph) Report {
	nodes := graph.Nodes()
	sort.SliceStable(nodes, func(i int, j int) bool {
		return nodes[i].ID < nodes[j].ID
	})

	nodesByID := make(map[string]evidence.Node, len(nodes))
	for _, node := range nodes {
		nodesByID[node.ID] = node
	}

	report := Report{}
	for _, node := range nodes {
		switch {
		case node.Type == evidence.NodeFinding:
			report.VerifiedFindings = append(report.VerifiedFindings, Finding{
				ID:     node.ID,
				Title:  node.Label,
				Source: node.SourceKind,
			})
		case node.Type == evidence.NodeArtifact && normalized(node.SourceKind) == "artifact":
			report.EvidenceRefs = append(report.EvidenceRefs, EvidenceRef{
				ID:     node.ID,
				Ref:    node.Label,
				Source: node.SourceKind,
			})
		case normalized(node.SourceKind) == "exhausted":
			report.ExhaustedAreas = append(report.ExhaustedAreas, node.Label)
		case normalized(node.SourceKind) == "suggestion":
			report.NextRunSuggestions = append(report.NextRunSuggestions, node.Label)
		}
	}

	report.Paths = buildPaths(graph.Edges(), nodesByID, report.VerifiedFindings)
	return report
}

func buildPaths(edges []evidence.Edge, nodesByID map[string]evidence.Node, findings []Finding) []Path {
	sort.SliceStable(edges, func(i int, j int) bool {
		if edges[i].From != edges[j].From {
			return edges[i].From < edges[j].From
		}
		if edges[i].Type != edges[j].Type {
			return edges[i].Type < edges[j].Type
		}
		return edges[i].To < edges[j].To
	})

	derivedIntentByFinding := map[string]string{}
	requirementsByIntent := map[string][]string{}
	for _, edge := range edges {
		switch edge.Type {
		case evidence.EdgeDerivedFrom:
			derivedIntentByFinding[edge.From] = edge.To
		case evidence.EdgeRequires:
			requirementsByIntent[edge.From] = append(requirementsByIntent[edge.From], edge.To)
		}
	}

	paths := []Path{}
	for _, finding := range findings {
		intentID := derivedIntentByFinding[finding.ID]
		if intentID == "" {
			paths = append(paths, Path{FindingID: finding.ID, Steps: []string{finding.Title}})
			continue
		}

		steps := []string{}
		for _, requirementID := range requirementsByIntent[intentID] {
			if node, ok := nodesByID[requirementID]; ok && strings.TrimSpace(node.Label) != "" {
				steps = append(steps, node.Label)
			}
		}
		if intent, ok := nodesByID[intentID]; ok && strings.TrimSpace(intent.Label) != "" {
			steps = append(steps, intent.Label)
		}
		if strings.TrimSpace(finding.Title) != "" {
			steps = append(steps, finding.Title)
		}
		paths = append(paths, Path{FindingID: finding.ID, Steps: steps})
	}
	return paths
}

func normalized(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}
