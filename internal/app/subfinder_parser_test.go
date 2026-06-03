package app

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/ouroboros/ouroboros/internal/compiler"
	"github.com/ouroboros/ouroboros/internal/eventlog"
)

func TestParserEventsForToolExecutionPromotesSubfinderSubdomainSurface(t *testing.T) {
	events := parserEventsForToolExecution("run-subfinder-surface", subfinderParserRequest([]ScopeTarget{{Kind: "domain", Value: "Example.COM"}}, nil, ""), ToolExecutionResult{
		ExecutionID: "native-subfinder",
		Status:      "completed",
		Transcript: strings.Join([]string{
			`{"input":"ignored.example.com","host":"API.Example.COM.","source":"secret-token","ip":"192.0.2.10","resolver":"1.1.1.1","timestamp":"2026-06-03T00:00:00Z","url":"https://api.example.com"}`,
			`{"subdomain":"www.example.com","sources":["secret-token"],"error":"secret-token"}`,
		}, "\n"),
	})

	if len(events) != 3 {
		t.Fatalf("expected parser event and two surface facts, got %#v", events)
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
	if parserPayload.Parser != "subfinder_jsonl" || parserPayload.ParserID != "subfinder_jsonl" || parserPayload.Status != "parsed" || parserPayload.ToolID != "subfinder" || parserPayload.FindingCount != 2 {
		t.Fatalf("unexpected subfinder parser payload: %#v", parserPayload)
	}

	first := decodeSubfinderSurfacePayload(t, events[1])
	second := decodeSubfinderSurfacePayload(t, events[2])
	if first.Source != "parser.subfinder_jsonl" || first.Kind != "surface" || first.ParserID != "subfinder_jsonl" {
		t.Fatalf("unexpected subfinder surface identity: %#v", first)
	}
	if first.Domain != "example.com" || first.Subdomain != "api.example.com" || first.Statement != "subfinder | api.example.com | domain=example.com" {
		t.Fatalf("unexpected first subfinder surface metadata: %#v", first)
	}
	if second.Domain != "example.com" || second.Subdomain != "www.example.com" || second.Statement != "subfinder | www.example.com | domain=example.com" {
		t.Fatalf("unexpected second subfinder surface metadata: %#v", second)
	}
	if first.Confidence < 0.6 || second.Confidence < 0.6 {
		t.Fatalf("expected useful subfinder confidence values: %#v %#v", first, second)
	}
	for _, event := range events[1:] {
		payloadText := string(event.Payload)
		for _, leaked := range []string{"secret-token", "192.0.2.10", "1.1.1.1", "2026-06-03", "https://", "sources", "resolver", "timestamp", "error", "url", "input"} {
			if strings.Contains(payloadText, leaked) {
				t.Fatalf("subfinder surface fact leaked untrusted output %q: %s", leaked, payloadText)
			}
		}
		var raw map[string]json.RawMessage
		if err := json.Unmarshal(event.Payload, &raw); err != nil {
			t.Fatalf("decode raw fact event: %v", err)
		}
		for key := range raw {
			if !allowedSubfinderFactKey(key) {
				t.Fatalf("subfinder surface fact persisted forbidden key %q: %s", key, payloadText)
			}
		}
	}
}

func TestParserEventsForToolExecutionRequiresSubfinderDomainScope(t *testing.T) {
	events := parserEventsForToolExecution("run-subfinder-url-scope", subfinderParserRequest([]ScopeTarget{{Kind: "url", Value: "https://app.example.com"}}, nil, "disabled"), ToolExecutionResult{
		ExecutionID: "native-subfinder",
		Status:      "completed",
		Transcript:  `{"host":"api.example.com"}`,
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
	if payload.Parser != "subfinder_jsonl" || payload.Status != "parse_failed" || payload.FindingCount != 0 || !strings.Contains(payload.Message, "domain scope") {
		t.Fatalf("unexpected no-domain-scope parse-failure payload: %#v", payload)
	}
}

func TestParserEventsForToolExecutionSkipsOutOfScopeSubfinderDomains(t *testing.T) {
	events := parserEventsForToolExecution("run-subfinder-skip-out-of-scope", subfinderParserRequest([]ScopeTarget{{Kind: "domain", Value: "example.com"}}, []string{"blocked.example.com", "*.internal.example.com"}, ""), ToolExecutionResult{
		ExecutionID: "native-subfinder",
		Status:      "completed",
		Transcript: strings.Join([]string{
			`{"host":"api.evil.test","input":"kept.example.com"}`,
			`{"host":"blocked.example.com"}`,
			`{"host":"www.internal.example.com"}`,
			`{"host":"kept.example.com"}`,
		}, "\n"),
	})

	if len(events) != 2 {
		t.Fatalf("expected parser event and one kept surface fact, got %#v", events)
	}
	var parserPayload struct {
		Parser       string `json:"parser"`
		Status       string `json:"status"`
		FindingCount int    `json:"finding_count"`
	}
	if err := json.Unmarshal(events[0].Payload, &parserPayload); err != nil {
		t.Fatalf("decode parser event: %v", err)
	}
	if parserPayload.Parser != "subfinder_jsonl" || parserPayload.Status != "parsed" || parserPayload.FindingCount != 1 {
		t.Fatalf("unexpected parser payload for skipped domains: %#v", parserPayload)
	}
	payload := decodeSubfinderSurfacePayload(t, events[1])
	if payload.Subdomain != "kept.example.com" || payload.Domain != "example.com" {
		t.Fatalf("unexpected kept surface payload: %#v", payload)
	}
	for _, leaked := range []string{"evil.test", "blocked.example.com", "internal.example.com"} {
		if strings.Contains(string(events[1].Payload), leaked) {
			t.Fatalf("subfinder surface leaked skipped domain %q: %s", leaked, events[1].Payload)
		}
	}
}

func TestParserEventsForToolExecutionRejectsUnsafeSubfinderHostValues(t *testing.T) {
	events := parserEventsForToolExecution("run-subfinder-unsafe-values", subfinderParserRequest([]ScopeTarget{{Kind: "domain", Value: "example.com"}}, nil, ""), ToolExecutionResult{
		ExecutionID: "native-subfinder",
		Status:      "completed",
		Transcript: strings.Join([]string{
			`{"host":"https://api.example.com"}`,
			`{"host":"api.example.com/path"}`,
			`{"host":"user@example.com"}`,
			`{"host":"api.example.com:443"}`,
			`{"host":"*.example.com"}`,
			`{"host":"bad label.example.com"}`,
			"{\"host\":\"bad\\u0007.example.com\"}",
			`{"host":"-bad.example.com"}`,
			`{"host":"bad-.example.com"}`,
			`{"host":"bad.example.com.."}`,
			`{"host":"good.example.com"}`,
		}, "\n"),
	})

	if len(events) != 2 {
		t.Fatalf("expected parser event and one valid fact, got %#v", events)
	}
	payload := decodeSubfinderSurfacePayload(t, events[1])
	if payload.Subdomain != "good.example.com" {
		t.Fatalf("expected only safe LDH host to promote, got %#v", payload)
	}
	for _, leaked := range []string{"https://", "/path", "user@", ":443", "*.", "bad label", "-bad", "bad-"} {
		if strings.Contains(string(events[1].Payload), leaked) {
			t.Fatalf("subfinder fact leaked rejected value %q: %s", leaked, events[1].Payload)
		}
	}
}

func TestParserEventsForToolExecutionRecordsSubfinderParseFailureWithoutFact(t *testing.T) {
	events := parserEventsForToolExecution("run-subfinder-parse-failure", subfinderParserRequest([]ScopeTarget{{Kind: "domain", Value: "example.com"}}, nil, ""), ToolExecutionResult{
		ExecutionID: "native-subfinder",
		Status:      "completed",
		Transcript:  "not-jsonl",
	})

	if len(events) != 1 {
		t.Fatalf("expected one parse-failure event, got %#v", events)
	}
	if events[0].Type != eventlog.EventParser {
		t.Fatalf("expected parser event, got %#v", events[0])
	}
	var payload struct {
		Parser       string `json:"parser"`
		Status       string `json:"status"`
		ToolID       string `json:"tool_id"`
		FindingCount int    `json:"finding_count"`
		Message      string `json:"message"`
	}
	if err := json.Unmarshal(events[0].Payload, &payload); err != nil {
		t.Fatalf("decode parser event: %v", err)
	}
	if payload.Parser != "subfinder_jsonl" || payload.Status != "parse_failed" || payload.ToolID != "subfinder" || payload.FindingCount != 0 || payload.Message == "" {
		t.Fatalf("unexpected parse-failure payload: %#v", payload)
	}
}

func TestParserEventsForToolExecutionCapsRawSubfinderJSONLOutput(t *testing.T) {
	events := parserEventsForToolExecution("run-subfinder-oversized-output", subfinderParserRequest([]ScopeTarget{{Kind: "domain", Value: "example.com"}}, nil, ""), ToolExecutionResult{
		ExecutionID: "native-subfinder",
		Status:      "completed",
		Transcript:  strings.Repeat(" ", subfinderParserMaxOutputBytes+1) + `{"host":"api.example.com"}`,
	})

	if len(events) != 1 {
		t.Fatalf("expected one parse-failure event, got %#v", events)
	}
	if events[0].Type != eventlog.EventParser {
		t.Fatalf("expected parser event, got %#v", events[0])
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
	if payload.Parser != "subfinder_jsonl" || payload.Status != "parse_failed" || payload.FindingCount != 0 || !strings.Contains(payload.Message, "exceeds") {
		t.Fatalf("unexpected oversized-output parse-failure payload: %#v", payload)
	}
}

func subfinderParserRequest(targets []ScopeTarget, exclusions []string, targetPolicy string) ToolExecutionRequest {
	return ToolExecutionRequest{
		Node: compiler.PlanNode{
			ID:           "subfinder-surface:01",
			Capability:   "web.discovery",
			ManifestName: "web-api-pack",
			ToolID:       "subfinder",
			Operation:    "subdomain_discovery",
			ParserID:     "subfinder_jsonl",
		},
		ScopeTargets:      targets,
		ScopeExclusions:   exclusions,
		ScopeTargetPolicy: targetPolicy,
	}
}

type subfinderSurfacePayload struct {
	Source      string  `json:"source"`
	Kind        string  `json:"kind"`
	Statement   string  `json:"statement"`
	Confidence  float64 `json:"confidence"`
	Parser      string  `json:"parser"`
	ParserID    string  `json:"parser_id"`
	PlanNodeID  string  `json:"plan_node_id"`
	Tool        string  `json:"tool"`
	ToolID      string  `json:"tool_id"`
	Operation   string  `json:"operation"`
	ExecutionID string  `json:"execution_id"`
	Domain      string  `json:"domain"`
	Subdomain   string  `json:"subdomain"`
}

func decodeSubfinderSurfacePayload(t *testing.T, event eventlog.Event) subfinderSurfacePayload {
	t.Helper()
	if event.Type != eventlog.EventFact {
		t.Fatalf("expected fact event, got %#v", event)
	}
	var payload subfinderSurfacePayload
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		t.Fatalf("decode fact event: %v", err)
	}
	return payload
}

func allowedSubfinderFactKey(key string) bool {
	switch key {
	case "fact_id", "kind", "statement", "confidence", "source", "parser", "parser_id", "plan_node_id", "tool", "tool_id", "operation", "execution_id", "domain", "subdomain":
		return true
	default:
		return false
	}
}
