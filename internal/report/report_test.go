package report

import (
	"strings"
	"testing"

	"github.com/ouroboros/ouroboros/internal/blackboard"
	"github.com/ouroboros/ouroboros/internal/evidence"
)

func TestProjectBuildsReportSectionsFromFixtureEvidenceGraph(t *testing.T) {
	projected := Project(fixtureEvidenceGraph())

	if len(projected.VerifiedFindings) != 1 {
		t.Fatalf("expected one verified finding, got %#v", projected.VerifiedFindings)
	}
	if projected.VerifiedFindings[0].ID != "fact-finding" || projected.VerifiedFindings[0].Title != "IDOR on /admin" {
		t.Fatalf("unexpected verified finding: %#v", projected.VerifiedFindings[0])
	}

	if len(projected.Paths) != 1 {
		t.Fatalf("expected one path, got %#v", projected.Paths)
	}
	path := projected.Paths[0]
	if path.FindingID != "fact-finding" || !contains(path.Steps, "https://app.example") || !contains(path.Steps, "test admin authorization") || !contains(path.Steps, "IDOR on /admin") {
		t.Fatalf("unexpected finding path: %#v", path)
	}

	if len(projected.EvidenceRefs) != 1 || projected.EvidenceRefs[0].Ref != "artifact://requests/admin-proof" {
		t.Fatalf("unexpected evidence refs: %#v", projected.EvidenceRefs)
	}
	if len(projected.ExhaustedAreas) != 1 || projected.ExhaustedAreas[0] != "login brute-force exhausted" {
		t.Fatalf("unexpected exhausted areas: %#v", projected.ExhaustedAreas)
	}
	if len(projected.NextRunSuggestions) != 1 || projected.NextRunSuggestions[0] != "re-run with authenticated editor role" {
		t.Fatalf("unexpected next-run suggestions: %#v", projected.NextRunSuggestions)
	}
}

func TestRenderMarkdownIncludesRequiredReportSections(t *testing.T) {
	markdown := RenderMarkdown(Project(fixtureEvidenceGraph()))

	for _, section := range []string{
		"# Ouroboros Report",
		"## Verified Findings",
		"## Attack Paths",
		"## Evidence References",
		"## Exhausted Areas",
		"## Next-Run Suggestions",
	} {
		if !strings.Contains(markdown, section) {
			t.Fatalf("expected markdown section %q in:\n%s", section, markdown)
		}
	}
	for _, content := range []string{
		"IDOR on /admin",
		"https://app.example -> test admin authorization -> IDOR on /admin",
		"artifact://requests/admin-proof",
		"login brute-force exhausted",
		"re-run with authenticated editor role",
	} {
		if !strings.Contains(markdown, content) {
			t.Fatalf("expected markdown content %q in:\n%s", content, markdown)
		}
	}
}

func fixtureEvidenceGraph() evidence.Graph {
	return evidence.Project(blackboard.ProjectState{
		Facts: []blackboard.Fact{
			{ID: "fact-target", Kind: "target", Statement: "https://app.example", Source: "scope"},
			{ID: "fact-finding", Kind: "finding", Statement: "IDOR on /admin", Source: "verified:manual"},
			{ID: "artifact-admin-proof", Kind: "artifact", Statement: "artifact://requests/admin-proof", Source: "vault"},
			{ID: "exhausted-login", Kind: "exhausted", Statement: "login brute-force exhausted", Source: "coverage"},
			{ID: "suggestion-editor", Kind: "suggestion", Statement: "re-run with authenticated editor role", Source: "replay"},
		},
		Intents: []blackboard.Intent{
			{
				ID:          "intent-auth",
				Kind:        "auth-check",
				Goal:        "test admin authorization",
				SourceFacts: []string{"fact-target"},
				ResultFact:  "fact-finding",
			},
		},
	})
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
