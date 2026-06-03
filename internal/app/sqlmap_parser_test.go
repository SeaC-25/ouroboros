package app

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/ouroboros/ouroboros/internal/compiler"
	"github.com/ouroboros/ouroboros/internal/eventlog"
)

func TestParserEventsForToolExecutionPromotesSqlmapInjectionFinding(t *testing.T) {
	events := parserEventsForToolExecution("run-sqlmap-finding", ToolExecutionRequest{
		Node: compiler.PlanNode{
			ID:           "sqlmap-finding:01",
			Capability:   "web.candidate_generation",
			ManifestName: "web-api-pack",
			ToolID:       "sqlmap",
			Operation:    "injection_test",
			ParserID:     "sqlmap_json",
		},
		ScopeTargets: []ScopeTarget{{Kind: "url", Value: "https://app.example/search?q=hello"}},
	}, ToolExecutionResult{
		ExecutionID: "native-sqlmap",
		Status:      "completed",
		Transcript: `{
  "findings": [{
    "url": "https://app.example/search?q=secret-token#frag",
    "parameter": "q",
    "place": "GET",
    "technique": "boolean-based blind",
    "dbms": "PostgreSQL",
    "title": "SQL injection vulnerability",
    "vulnerable": true,
    "risk": 1,
    "level": 1,
    "payload": "secret-token",
    "request": "Authorization: Bearer secret-token",
    "response": "secret-token",
    "dump": {"users": ["secret-token"]},
    "headers": {"Authorization": "Bearer secret-token"}
  }]
}`,
	})

	if len(events) != 2 {
		t.Fatalf("expected parser event and one sqlmap finding fact, got %#v", events)
	}
	if events[0].Type != eventlog.EventParser || events[1].Type != eventlog.EventFact {
		t.Fatalf("expected parser event followed by finding fact event, got %#v", events)
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
	if parserPayload.Parser != "sqlmap_json" || parserPayload.Status != "parsed" || parserPayload.FindingCount != 1 || parserPayload.ToolID != "sqlmap" {
		t.Fatalf("unexpected sqlmap parser payload: %#v", parserPayload)
	}

	payloadText := string(events[1].Payload)
	for _, leaked := range []string{"secret-token", "Authorization", "Bearer", "payload", "request", "response", "dump", "headers", "users", "?", "#frag"} {
		if strings.Contains(payloadText, leaked) {
			t.Fatalf("sqlmap finding fact leaked untrusted output %q: %s", leaked, payloadText)
		}
	}
	var rawPayload map[string]json.RawMessage
	if err := json.Unmarshal(events[1].Payload, &rawPayload); err != nil {
		t.Fatalf("decode raw fact event: %v", err)
	}
	for _, forbiddenKey := range []string{"raw", "headers", "request", "response", "body", "payload", "evidence", "dump", "database", "tables", "columns", "rows"} {
		if _, ok := rawPayload[forbiddenKey]; ok {
			t.Fatalf("sqlmap finding fact persisted forbidden key %q: %s", forbiddenKey, payloadText)
		}
	}

	var payload struct {
		Source     string  `json:"source"`
		Kind       string  `json:"kind"`
		Statement  string  `json:"statement"`
		Confidence float64 `json:"confidence"`
		MatchedURL string  `json:"matched_url"`
		TargetURL  string  `json:"target_url"`
		Parameter  string  `json:"parameter"`
		Place      string  `json:"place"`
		Technique  string  `json:"technique"`
		DBMS       string  `json:"dbms"`
		Title      string  `json:"title"`
		ParserID   string  `json:"parser_id"`
	}
	if err := json.Unmarshal(events[1].Payload, &payload); err != nil {
		t.Fatalf("decode fact event: %v", err)
	}
	if payload.Source != "parser.sqlmap_json" || payload.Kind != "finding" || payload.ParserID != "sqlmap_json" {
		t.Fatalf("unexpected sqlmap finding identity: %#v", payload)
	}
	if payload.MatchedURL != "https://app.example/search" || payload.TargetURL != "https://app.example/search" || payload.Parameter != "q" || payload.Place != "GET" || payload.Technique != "boolean-based blind" || payload.DBMS != "PostgreSQL" || payload.Title != "SQL injection vulnerability" {
		t.Fatalf("unexpected sqlmap finding metadata: %#v", payload)
	}
	for _, want := range []string{"sqlmap", "https://app.example/search", "parameter=q", "boolean-based blind", "PostgreSQL"} {
		if !strings.Contains(payload.Statement, want) {
			t.Fatalf("expected sqlmap statement to contain %q, got %#v", want, payload)
		}
	}
	if payload.Confidence < 0.85 {
		t.Fatalf("expected high-confidence sqlmap finding, got %#v", payload)
	}
}

func TestParserEventsForToolExecutionRequiresSqlmapURLScope(t *testing.T) {
	events := parserEventsForToolExecution("run-sqlmap-no-url-scope", ToolExecutionRequest{
		Node: compiler.PlanNode{
			ID:           "sqlmap-no-url-scope:01",
			Capability:   "web.candidate_generation",
			ManifestName: "web-api-pack",
			ToolID:       "sqlmap",
			Operation:    "injection_test",
			ParserID:     "sqlmap_json",
		},
		ScopeTargets: []ScopeTarget{{Kind: "domain", Value: "example.com"}},
	}, ToolExecutionResult{
		ExecutionID: "native-sqlmap",
		Status:      "completed",
		Transcript:  `{"findings":[{"url":"https://app.example/search","parameter":"q","technique":"boolean-based blind","vulnerable":true}]}`,
	})

	if len(events) != 1 || events[0].Type != eventlog.EventParser {
		t.Fatalf("expected one parser failure event without URL scope, got %#v", events)
	}
	var payload struct {
		Parser       string `json:"parser"`
		Status       string `json:"status"`
		FindingCount int    `json:"finding_count"`
	}
	if err := json.Unmarshal(events[0].Payload, &payload); err != nil {
		t.Fatalf("decode parser event: %v", err)
	}
	if payload.Parser != "sqlmap_json" || payload.Status != "parse_failed" || payload.FindingCount != 0 {
		t.Fatalf("unexpected no-scope parse-failure payload: %#v", payload)
	}
}

func TestParserEventsForToolExecutionSkipsOutOfScopeExcludedAndWeakSqlmapFindings(t *testing.T) {
	events := parserEventsForToolExecution("run-sqlmap-scope-filter", ToolExecutionRequest{
		Node: compiler.PlanNode{
			ID:           "sqlmap-scope-filter:01",
			Capability:   "web.candidate_generation",
			ManifestName: "web-api-pack",
			ToolID:       "sqlmap",
			Operation:    "injection_test",
			ParserID:     "sqlmap_json",
		},
		ScopeTargets:    []ScopeTarget{{Kind: "url", Value: "https://app.example/app"}},
		ScopeExclusions: []string{"https://app.example/app/admin"},
	}, ToolExecutionResult{
		ExecutionID: "native-sqlmap",
		Status:      "completed",
		Transcript: `{"results":[
  {"url":"https://app.example/app/search?q=secret-token","param":"q","techniques":["time-based blind"],"vulnerable":true},
  {"url":"https://app.example/app/admin?id=secret-token","param":"admin","technique":"boolean-based blind","vulnerable":true},
  {"url":"https://app.example/root?id=secret-token","param":"root","technique":"boolean-based blind","vulnerable":true},
  {"url":"https://evil.example/out?id=secret-token","param":"evil","technique":"boolean-based blind","vulnerable":true},
  {"url":"https://app.example/app/weak?id=secret-token","param":"weak","vulnerable":false}
]}`,
	})

	if len(events) != 2 {
		t.Fatalf("expected parser event and one in-scope sqlmap finding fact, got %#v", events)
	}
	var parserPayload struct {
		Parser       string `json:"parser"`
		Status       string `json:"status"`
		FindingCount int    `json:"finding_count"`
	}
	if err := json.Unmarshal(events[0].Payload, &parserPayload); err != nil {
		t.Fatalf("decode parser payload: %v", err)
	}
	if parserPayload.Parser != "sqlmap_json" || parserPayload.Status != "parsed" || parserPayload.FindingCount != 1 {
		t.Fatalf("unexpected parser payload: %#v", parserPayload)
	}
	var payload struct {
		MatchedURL string `json:"matched_url"`
		TargetURL  string `json:"target_url"`
		Parameter  string `json:"parameter"`
		Technique  string `json:"technique"`
	}
	if err := json.Unmarshal(events[1].Payload, &payload); err != nil {
		t.Fatalf("decode finding fact: %v", err)
	}
	if payload.MatchedURL != "https://app.example/app/search" || payload.TargetURL != "https://app.example/app" || payload.Parameter != "q" || payload.Technique != "time-based blind" {
		t.Fatalf("expected only in-scope non-excluded sqlmap finding, got %#v", payload)
	}
	for _, leaked := range []string{"evil.example", "admin", "root", "weak", "token=", "secret-token"} {
		if strings.Contains(string(events[1].Payload), leaked) {
			t.Fatalf("sqlmap finding fact leaked out-of-scope/excluded/query data %q: %s", leaked, events[1].Payload)
		}
	}
}

func TestParserEventsForToolExecutionRecordsSqlmapParseFailureWithoutFact(t *testing.T) {
	events := parserEventsForToolExecution("run-sqlmap-parse-failure", ToolExecutionRequest{
		Node: compiler.PlanNode{
			ID:           "sqlmap-parse-failure:01",
			Capability:   "web.candidate_generation",
			ManifestName: "web-api-pack",
			ToolID:       "sqlmap",
			Operation:    "injection_test",
			ParserID:     "sqlmap_json",
		},
		ScopeTargets: []ScopeTarget{{Kind: "url", Value: "https://app.example"}},
	}, ToolExecutionResult{
		ExecutionID: "native-sqlmap",
		Status:      "completed",
		Transcript:  "not-json",
	})

	if len(events) != 1 || events[0].Type != eventlog.EventParser {
		t.Fatalf("expected one parser failure event for malformed sqlmap output, got %#v", events)
	}
	var payload struct {
		Parser       string `json:"parser"`
		Status       string `json:"status"`
		FindingCount int    `json:"finding_count"`
	}
	if err := json.Unmarshal(events[0].Payload, &payload); err != nil {
		t.Fatalf("decode parser event: %v", err)
	}
	if payload.Parser != "sqlmap_json" || payload.Status != "parse_failed" || payload.FindingCount != 0 {
		t.Fatalf("unexpected malformed parse-failure payload: %#v", payload)
	}
}
