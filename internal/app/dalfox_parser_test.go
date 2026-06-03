package app

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/ouroboros/ouroboros/internal/compiler"
	"github.com/ouroboros/ouroboros/internal/eventlog"
)

func TestParserEventsForToolExecutionPromotesDalfoxFinding(t *testing.T) {
	events := parserEventsForToolExecution("run-dalfox-finding", ToolExecutionRequest{
		Node: compiler.PlanNode{
			ID:           "dalfox-finding:01",
			Capability:   "web.candidate_generation",
			ManifestName: "web-api-pack",
			ToolID:       "dalfox",
			Operation:    "xss_scan",
			ParserID:     "dalfox_json",
		},
		ScopeTargets: []ScopeTarget{{Kind: "url", Value: "https://app.example/search?q=hello"}},
	}, ToolExecutionResult{
		ExecutionID: "native-dalfox",
		Status:      "completed",
		Transcript:  `{"pocs":[{"type":"V","inject_type":"inHTML","poc_type":"verified","method":"GET","data":"https://app.example/search?q=%3Cscript%3Esecret-token%3C%2Fscript%3E#frag","param":"q","payload":"secret-token","evidence":"secret-token","cwe":"CWE-79","severity":"high","message_id":42,"message_str":"secret-token","raw_request":"GET /search?q=secret-token HTTP/1.1\r\nAuthorization: Bearer secret-token","raw_response":"HTTP/1.1 200 OK\r\n\r\nsecret-token"}]}`,
	})

	if len(events) != 2 {
		t.Fatalf("expected parser event and one dalfox finding fact, got %#v", events)
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
	if parserPayload.Parser != "dalfox_json" || parserPayload.Status != "parsed" || parserPayload.FindingCount != 1 || parserPayload.ToolID != "dalfox" {
		t.Fatalf("unexpected dalfox parser payload: %#v", parserPayload)
	}

	payloadText := string(events[1].Payload)
	for _, leaked := range []string{"secret-token", "Authorization", "Bearer", "raw_request", "raw_response", "payload", "evidence", "message_str"} {
		if strings.Contains(payloadText, leaked) {
			t.Fatalf("dalfox finding fact leaked untrusted output %q: %s", leaked, payloadText)
		}
	}
	var rawPayload map[string]json.RawMessage
	if err := json.Unmarshal(events[1].Payload, &rawPayload); err != nil {
		t.Fatalf("decode raw fact event: %v", err)
	}
	for _, forbiddenKey := range []string{"data", "payload", "evidence", "message_id", "message_str", "raw_request", "raw_response", "request", "response", "headers", "body"} {
		if _, ok := rawPayload[forbiddenKey]; ok {
			t.Fatalf("dalfox finding fact persisted forbidden key %q: %s", forbiddenKey, payloadText)
		}
	}

	var payload struct {
		Source     string  `json:"source"`
		Kind       string  `json:"kind"`
		Statement  string  `json:"statement"`
		Confidence float64 `json:"confidence"`
		Type       string  `json:"type"`
		InjectType string  `json:"inject_type"`
		PoCType    string  `json:"poc_type"`
		Method     string  `json:"method"`
		Param      string  `json:"param"`
		Severity   string  `json:"severity"`
		CWE        string  `json:"cwe"`
		MatchedURL string  `json:"matched_url"`
		TargetURL  string  `json:"target_url"`
		ParserID   string  `json:"parser_id"`
	}
	if err := json.Unmarshal(events[1].Payload, &payload); err != nil {
		t.Fatalf("decode fact event: %v", err)
	}
	if payload.Source != "parser.dalfox_json" || payload.Kind != "finding" || payload.ParserID != "dalfox_json" {
		t.Fatalf("unexpected dalfox finding identity: %#v", payload)
	}
	if payload.Type != "V" || payload.InjectType != "inHTML" || payload.PoCType != "verified" || payload.Method != "GET" || payload.Param != "q" || payload.Severity != "HIGH" || payload.CWE != "CWE-79" {
		t.Fatalf("unexpected dalfox finding metadata: %#v", payload)
	}
	if payload.MatchedURL != "https://app.example/search" || payload.TargetURL != "https://app.example/search" {
		t.Fatalf("expected sanitized dalfox URL metadata, got %#v", payload)
	}
	for _, want := range []string{"dalfox", "V", "HIGH", "https://app.example/search", "q", "CWE-79"} {
		if !strings.Contains(payload.Statement, want) {
			t.Fatalf("expected dalfox statement to contain %q, got %#v", want, payload)
		}
	}
	if payload.Confidence < 0.85 {
		t.Fatalf("expected high-confidence dalfox finding, got %#v", payload)
	}
}

func TestParserEventsForToolExecutionRequiresDalfoxURLScope(t *testing.T) {
	events := parserEventsForToolExecution("run-dalfox-no-url-scope", ToolExecutionRequest{
		Node: compiler.PlanNode{
			ID:           "dalfox-no-url-scope:01",
			Capability:   "web.candidate_generation",
			ManifestName: "web-api-pack",
			ToolID:       "dalfox",
			Operation:    "xss_scan",
			ParserID:     "dalfox_json",
		},
		ScopeTargets: []ScopeTarget{{Kind: "domain", Value: "example.com"}},
	}, ToolExecutionResult{
		ExecutionID: "native-dalfox",
		Status:      "completed",
		Transcript:  `{"pocs":[{"type":"V","data":"https://app.example/search?q=x","param":"q","severity":"high"}]}`,
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
	if payload.Parser != "dalfox_json" || payload.Status != "parse_failed" || payload.FindingCount != 0 {
		t.Fatalf("unexpected no-scope parse-failure payload: %#v", payload)
	}
}

func TestParserEventsForToolExecutionSkipsExcludedDalfoxFindings(t *testing.T) {
	events := parserEventsForToolExecution("run-dalfox-excluded-finding", ToolExecutionRequest{
		Node: compiler.PlanNode{
			ID:           "dalfox-excluded-finding:01",
			Capability:   "web.candidate_generation",
			ManifestName: "web-api-pack",
			ToolID:       "dalfox",
			Operation:    "xss_scan",
			ParserID:     "dalfox_json",
		},
		ScopeTargets:    []ScopeTarget{{Kind: "url", Value: "https://app.example"}},
		ScopeExclusions: []string{"https://app.example/admin"},
	}, ToolExecutionResult{
		ExecutionID: "native-dalfox",
		Status:      "completed",
		Transcript: strings.Join([]string{
			`{"type":"V","data":"https://app.example/admin?token=secret-token","param":"x","severity":"high","payload":"secret-token"}`,
			`{"type":"G","data":"https://app.example/login","param":"next","severity":"low"}`,
			`{"pocs":[{}]}`,
		}, "\n"),
	})

	if len(events) != 2 {
		t.Fatalf("expected parser event and one non-excluded dalfox finding fact, got %#v", events)
	}
	var parserPayload struct {
		Parser       string `json:"parser"`
		Status       string `json:"status"`
		FindingCount int    `json:"finding_count"`
	}
	if err := json.Unmarshal(events[0].Payload, &parserPayload); err != nil {
		t.Fatalf("decode parser payload: %v", err)
	}
	if parserPayload.Parser != "dalfox_json" || parserPayload.Status != "parsed" || parserPayload.FindingCount != 1 {
		t.Fatalf("unexpected parser payload: %#v", parserPayload)
	}
	var payload struct {
		MatchedURL string `json:"matched_url"`
		Param      string `json:"param"`
	}
	if err := json.Unmarshal(events[1].Payload, &payload); err != nil {
		t.Fatalf("decode finding fact: %v", err)
	}
	if payload.MatchedURL != "https://app.example/login" || payload.Param != "next" {
		t.Fatalf("expected only non-excluded finding, got %#v", payload)
	}
	for _, leaked := range []string{"https://app.example/admin", "token=", "secret-token"} {
		if strings.Contains(string(events[1].Payload), leaked) {
			t.Fatalf("dalfox finding fact leaked excluded/query data %q: %s", leaked, events[1].Payload)
		}
	}
}

func TestParserEventsForToolExecutionSkipsOutOfScopeDalfoxFindings(t *testing.T) {
	events := parserEventsForToolExecution("run-dalfox-out-of-scope-finding", ToolExecutionRequest{
		Node: compiler.PlanNode{
			ID:           "dalfox-out-of-scope-finding:01",
			Capability:   "web.candidate_generation",
			ManifestName: "web-api-pack",
			ToolID:       "dalfox",
			Operation:    "xss_scan",
			ParserID:     "dalfox_json",
		},
		ScopeTargets: []ScopeTarget{{Kind: "url", Value: "https://app.example/app"}},
	}, ToolExecutionResult{
		ExecutionID: "native-dalfox",
		Status:      "completed",
		Transcript: strings.Join([]string{
			`{"type":"V","data":"https://app.example/app/search?q=secret-token","param":"q","severity":"high","payload":"secret-token"}`,
			`{"type":"V","data":"https://app.example/admin?q=secret-token","param":"admin","severity":"high","payload":"secret-token"}`,
			`{"type":"V","data":"https://evil.example/search?q=secret-token","param":"evil","severity":"high","payload":"secret-token"}`,
		}, "\n"),
	})

	if len(events) != 2 {
		t.Fatalf("expected parser event and one in-scope dalfox finding fact, got %#v", events)
	}
	var parserPayload struct {
		Parser       string `json:"parser"`
		Status       string `json:"status"`
		FindingCount int    `json:"finding_count"`
	}
	if err := json.Unmarshal(events[0].Payload, &parserPayload); err != nil {
		t.Fatalf("decode parser payload: %v", err)
	}
	if parserPayload.Parser != "dalfox_json" || parserPayload.Status != "parsed" || parserPayload.FindingCount != 1 {
		t.Fatalf("unexpected parser payload: %#v", parserPayload)
	}
	var payload struct {
		MatchedURL string `json:"matched_url"`
		TargetURL  string `json:"target_url"`
		Param      string `json:"param"`
	}
	if err := json.Unmarshal(events[1].Payload, &payload); err != nil {
		t.Fatalf("decode finding fact: %v", err)
	}
	if payload.MatchedURL != "https://app.example/app/search" || payload.TargetURL != "https://app.example/app" || payload.Param != "q" {
		t.Fatalf("expected only in-scope finding under path scope, got %#v", payload)
	}
	for _, leaked := range []string{"evil.example", "https://app.example/admin", "token=", "secret-token", "admin", "evil"} {
		if strings.Contains(string(events[1].Payload), leaked) {
			t.Fatalf("dalfox finding fact leaked out-of-scope/query data %q: %s", leaked, events[1].Payload)
		}
	}
}

func TestParserEventsForToolExecutionRecordsDalfoxParseFailureWithoutFact(t *testing.T) {
	events := parserEventsForToolExecution("run-dalfox-parse-failure", ToolExecutionRequest{
		Node: compiler.PlanNode{
			ID:           "dalfox-parse-failure:01",
			Capability:   "web.candidate_generation",
			ManifestName: "web-api-pack",
			ToolID:       "dalfox",
			Operation:    "xss_scan",
			ParserID:     "dalfox_json",
		},
		ScopeTargets: []ScopeTarget{{Kind: "url", Value: "https://app.example"}},
	}, ToolExecutionResult{
		ExecutionID: "native-dalfox",
		Status:      "completed",
		Transcript:  "not-json",
	})

	if len(events) != 1 || events[0].Type != eventlog.EventParser {
		t.Fatalf("expected one parse-failure parser event, got %#v", events)
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
	if payload.Parser != "dalfox_json" || payload.Status != "parse_failed" || payload.ToolID != "dalfox" || payload.FindingCount != 0 || payload.Message == "" {
		t.Fatalf("unexpected parse-failure payload: %#v", payload)
	}
}
