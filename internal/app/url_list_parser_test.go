package app

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/ouroboros/ouroboros/internal/compiler"
	"github.com/ouroboros/ouroboros/internal/eventlog"
)

func TestParserEventsForToolExecutionPromotesURLListTextSurface(t *testing.T) {
	events := parserEventsForToolExecution("run-url-list", ToolExecutionRequest{
		Node: compiler.PlanNode{
			ID:           "archive-discovery:01",
			Capability:   "web.route_enumeration",
			ManifestName: "web-api-pack",
			ToolID:       "gau",
			Operation:    "archive_discovery",
			ParserID:     "url_list_text",
		},
		ScopeTargets: []ScopeTarget{{Kind: "domain", Value: "example.com"}},
	}, ToolExecutionResult{
		ExecutionID: "native-gau",
		Status:      "completed",
		Transcript: strings.Join([]string{
			"https://www.example.com/login?token=secret-token#frag",
			"https://api.example.com/v1/users",
			"https://evil.example.net/admin",
			"not a url",
		}, "\n"),
	})

	if len(events) != 3 {
		t.Fatalf("expected parser event and two URL surface facts, got %#v", events)
	}
	if events[0].Type != eventlog.EventParser || events[1].Type != eventlog.EventFact || events[2].Type != eventlog.EventFact {
		t.Fatalf("expected parser event followed by URL surface facts, got %#v", events)
	}
	var parserPayload struct {
		Parser       string `json:"parser"`
		Status       string `json:"status"`
		FindingCount int    `json:"finding_count"`
		ToolID       string `json:"tool_id"`
	}
	if err := json.Unmarshal(events[0].Payload, &parserPayload); err != nil {
		t.Fatalf("decode parser event: %v", err)
	}
	if parserPayload.Parser != "url_list_text" || parserPayload.Status != "parsed" || parserPayload.FindingCount != 2 || parserPayload.ToolID != "gau" {
		t.Fatalf("unexpected url list parser payload: %#v", parserPayload)
	}
	var first struct {
		Source     string  `json:"source"`
		Kind       string  `json:"kind"`
		Statement  string  `json:"statement"`
		Confidence float64 `json:"confidence"`
		URL        string  `json:"url"`
		Domain     string  `json:"domain"`
		ParserID   string  `json:"parser_id"`
	}
	if err := json.Unmarshal(events[1].Payload, &first); err != nil {
		t.Fatalf("decode first URL list fact: %v", err)
	}
	if first.Source != "parser.url_list_text" || first.Kind != "surface" || first.ParserID != "url_list_text" || first.URL != "https://www.example.com/login" || first.Domain != "example.com" {
		t.Fatalf("unexpected first URL list surface: %#v", first)
	}
	if !strings.Contains(first.Statement, "archive") || !strings.Contains(first.Statement, "https://www.example.com/login") || strings.Contains(first.Statement, "secret-token") {
		t.Fatalf("expected sanitized archive URL statement, got %#v", first.Statement)
	}
	for _, event := range events[1:] {
		payload := string(event.Payload)
		for _, leaked := range []string{"secret-token", "evil.example.net", "not a url", "query", "fragment"} {
			if strings.Contains(payload, leaked) {
				t.Fatalf("URL list fact leaked untrusted output %q: %s", leaked, payload)
			}
		}
	}
}

func TestParserEventsForToolExecutionRequiresURLListDomainScope(t *testing.T) {
	events := parserEventsForToolExecution("run-url-list-no-domain", ToolExecutionRequest{
		Node: compiler.PlanNode{
			ID:           "archive-discovery-no-domain:01",
			Capability:   "web.route_enumeration",
			ManifestName: "web-api-pack",
			ToolID:       "waybackurls",
			Operation:    "archive_discovery",
			ParserID:     "url_list_text",
		},
		ScopeTargets: []ScopeTarget{{Kind: "url", Value: "https://example.com"}},
	}, ToolExecutionResult{
		ExecutionID: "native-waybackurls",
		Status:      "completed",
		Transcript:  "https://example.com/login\n",
	})

	if len(events) != 1 || events[0].Type != eventlog.EventParser {
		t.Fatalf("expected one parser failure event without domain scope, got %#v", events)
	}
	var payload struct {
		Parser       string `json:"parser"`
		Status       string `json:"status"`
		FindingCount int    `json:"finding_count"`
	}
	if err := json.Unmarshal(events[0].Payload, &payload); err != nil {
		t.Fatalf("decode parser event: %v", err)
	}
	if payload.Parser != "url_list_text" || payload.Status != "parse_failed" || payload.FindingCount != 0 {
		t.Fatalf("unexpected no-scope parse-failure payload: %#v", payload)
	}
}
