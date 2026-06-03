package app

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/ouroboros/ouroboros/internal/compiler"
	"github.com/ouroboros/ouroboros/internal/eventlog"
)

func TestParserEventsForToolExecutionPromotesAmassSubdomainSurface(t *testing.T) {
	events := parserEventsForToolExecution("run-amass-surface", amassParserRequest([]ScopeTarget{{Kind: "domain", Value: "Example.COM"}}, nil), ToolExecutionResult{
		ExecutionID: "native-amass",
		Status:      "completed",
		Transcript: strings.Join([]string{
			`{"name":"API.Example.COM.","domain":"example.com","addresses":[{"ip":"192.0.2.10","cidr":"192.0.2.0/24","asn":64500}],"sources":["secret-token"],"tag":"cert","timestamp":"2026-06-03T00:00:00Z"}`,
			`{"name":"www.example.com","source":"secret-token","addr":"192.0.2.11"}`,
			`{"name":"evil.example.net","source":"secret-token"}`,
		}, "\n"),
	})

	if len(events) != 3 {
		t.Fatalf("expected parser event and two amass surface facts, got %#v", events)
	}
	if events[0].Type != eventlog.EventParser || events[1].Type != eventlog.EventFact || events[2].Type != eventlog.EventFact {
		t.Fatalf("expected parser event followed by fact events, got %#v", events)
	}
	var parserPayload struct {
		Parser       string `json:"parser"`
		ParserID     string `json:"parser_id"`
		Status       string `json:"status"`
		ToolID       string `json:"tool_id"`
		FindingCount int    `json:"finding_count"`
	}
	if err := json.Unmarshal(events[0].Payload, &parserPayload); err != nil {
		t.Fatalf("decode parser event: %v", err)
	}
	if parserPayload.Parser != "amass_json" || parserPayload.ParserID != "amass_json" || parserPayload.Status != "parsed" || parserPayload.ToolID != "amass" || parserPayload.FindingCount != 2 {
		t.Fatalf("unexpected amass parser payload: %#v", parserPayload)
	}

	first := decodeAmassSurfacePayload(t, events[1])
	second := decodeAmassSurfacePayload(t, events[2])
	if first.Source != "parser.amass_json" || first.Kind != "surface" || first.ParserID != "amass_json" {
		t.Fatalf("unexpected amass surface identity: %#v", first)
	}
	if first.Domain != "example.com" || first.Subdomain != "api.example.com" || first.Statement != "amass | api.example.com | domain=example.com" {
		t.Fatalf("unexpected first amass surface metadata: %#v", first)
	}
	if second.Domain != "example.com" || second.Subdomain != "www.example.com" {
		t.Fatalf("unexpected second amass surface metadata: %#v", second)
	}
	for _, event := range events[1:] {
		payloadText := string(event.Payload)
		for _, leaked := range []string{"secret-token", "192.0.2", "64500", "cert", "timestamp", "evil.example.net", "addresses", "sources"} {
			if strings.Contains(payloadText, leaked) {
				t.Fatalf("amass surface fact leaked untrusted output %q: %s", leaked, payloadText)
			}
		}
	}
}

func TestParserEventsForToolExecutionRequiresAmassDomainScope(t *testing.T) {
	events := parserEventsForToolExecution("run-amass-url-scope", amassParserRequest([]ScopeTarget{{Kind: "url", Value: "https://app.example.com"}}, nil), ToolExecutionResult{
		ExecutionID: "native-amass",
		Status:      "completed",
		Transcript:  `{"name":"api.example.com"}`,
	})

	if len(events) != 1 || events[0].Type != eventlog.EventParser {
		t.Fatalf("expected one parser failure event without explicit domain scope, got %#v", events)
	}
	var payload struct {
		Parser       string `json:"parser"`
		Status       string `json:"status"`
		FindingCount int    `json:"finding_count"`
		Message      string `json:"message"`
	}
	if err := json.Unmarshal(events[0].Payload, &payload); err != nil {
		t.Fatalf("decode parser event: %v", err)
	}
	if payload.Parser != "amass_json" || payload.Status != "parse_failed" || payload.FindingCount != 0 || !strings.Contains(payload.Message, "domain scope") {
		t.Fatalf("unexpected amass no-domain parse-failure payload: %#v", payload)
	}
}

func TestParserEventsForToolExecutionRecordsAmassParseFailureWithoutFact(t *testing.T) {
	events := parserEventsForToolExecution("run-amass-parse-failure", amassParserRequest([]ScopeTarget{{Kind: "domain", Value: "example.com"}}, nil), ToolExecutionResult{
		ExecutionID: "native-amass",
		Status:      "completed",
		Transcript:  "not-json",
	})

	if len(events) != 1 || events[0].Type != eventlog.EventParser {
		t.Fatalf("expected one parser failure event, got %#v", events)
	}
	var payload struct {
		Parser       string `json:"parser"`
		Status       string `json:"status"`
		FindingCount int    `json:"finding_count"`
	}
	if err := json.Unmarshal(events[0].Payload, &payload); err != nil {
		t.Fatalf("decode parser event: %v", err)
	}
	if payload.Parser != "amass_json" || payload.Status != "parse_failed" || payload.FindingCount != 0 {
		t.Fatalf("unexpected amass parse-failure payload: %#v", payload)
	}
}

func amassParserRequest(targets []ScopeTarget, exclusions []string) ToolExecutionRequest {
	return ToolExecutionRequest{
		Node: compiler.PlanNode{
			ID:           "amass-surface:01",
			Capability:   "infra.host_discovery",
			ManifestName: "infra-pack",
			ToolID:       "amass",
			Operation:    "subdomain_discovery",
			ParserID:     "amass_json",
		},
		ScopeTargets:    targets,
		ScopeExclusions: exclusions,
		IsolationMode:   "process",
	}
}

type amassSurfacePayload struct {
	Source     string  `json:"source"`
	Kind       string  `json:"kind"`
	Statement  string  `json:"statement"`
	ParserID   string  `json:"parser_id"`
	Domain     string  `json:"domain"`
	Subdomain  string  `json:"subdomain"`
	Confidence float64 `json:"confidence"`
}

func decodeAmassSurfacePayload(t *testing.T, event eventlog.Event) amassSurfacePayload {
	t.Helper()
	if event.Type != eventlog.EventFact {
		t.Fatalf("expected fact event, got %#v", event)
	}
	var payload amassSurfacePayload
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		t.Fatalf("decode amass surface payload: %v", err)
	}
	return payload
}
