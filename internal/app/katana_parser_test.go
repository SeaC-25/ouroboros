package app

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/ouroboros/ouroboros/internal/compiler"
	"github.com/ouroboros/ouroboros/internal/eventlog"
)

func TestParserEventsForToolExecutionPromotesKatanaRouteSurface(t *testing.T) {
	events := parserEventsForToolExecution("run-katana-route", ToolExecutionRequest{
		Node: compiler.PlanNode{
			ID:           "katana-route:01",
			Capability:   "web.route_enumeration",
			ManifestName: "web-api-pack",
			ToolID:       "katana",
			Operation:    "crawl",
			ParserID:     "katana_jsonl",
		},
		ScopeTargets: []ScopeTarget{{Kind: "url", Value: "https://app.example"}},
	}, ToolExecutionResult{
		ExecutionID: "native-katana",
		Status:      "completed",
		Transcript: strings.Join([]string{
			`{"url":"https://app.example/login?token=secret-token#frag","method":"GET","status_code":200,"content_length":6726,"depth":2,"source":"https://app.example/raw?secret=secret-token","tag":"a","attribute":"href","headers":{"Authorization":"Bearer secret-token"},"raw":"secret-token","request":"secret-token","response":"secret-token","body":"secret-token","forms":[{"parameters":["secret-token"]}],"xhrrequests":[{"endpoint":"https://app.example/api?token=secret-token"}],"knowledgebase":{"secret":"secret-token"},"technologies":["secret-token"],"storedresponsepath":"/tmp/secret-token"}`,
			`{"request":{"endpoint":"https://app.example/account","method":"POST","body":"password=secret-token","headers":{"Authorization":"Bearer secret-token"},"raw":"secret-token"},"response":{"status_code":204,"content_length":0,"headers":{"set-cookie":"secret-token"},"body":"secret-token","raw":"secret-token"},"timestamp":"2026-06-03T00:00:00Z"}`,
		}, "\n"),
	})

	if len(events) != 3 {
		t.Fatalf("expected parser event and two route facts, got %#v", events)
	}
	if events[0].Type != eventlog.EventParser || events[1].Type != eventlog.EventFact || events[2].Type != eventlog.EventFact {
		t.Fatalf("expected parser event followed by route fact events, got %#v", events)
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
	if parserPayload.Parser != "katana_jsonl" || parserPayload.Status != "parsed" || parserPayload.FindingCount != 2 || parserPayload.ToolID != "katana" {
		t.Fatalf("unexpected katana parser payload: %#v", parserPayload)
	}
	for _, event := range events[1:] {
		payloadText := string(event.Payload)
		for _, leaked := range []string{"secret-token", "Authorization", "Bearer", "set-cookie", "forms", "xhrrequests", "knowledgebase", "storedresponsepath", "timestamp"} {
			if strings.Contains(payloadText, leaked) {
				t.Fatalf("katana route fact leaked untrusted output %q: %s", leaked, payloadText)
			}
		}
		var rawPayload map[string]json.RawMessage
		if err := json.Unmarshal(event.Payload, &rawPayload); err != nil {
			t.Fatalf("decode raw fact event: %v", err)
		}
		for _, forbiddenKey := range []string{"tag", "attribute", "headers", "raw", "request", "response", "body", "forms", "xhrrequests", "knowledgebase", "technologies", "storedresponsepath", "timestamp"} {
			if _, ok := rawPayload[forbiddenKey]; ok {
				t.Fatalf("katana route fact persisted forbidden key %q: %s", forbiddenKey, payloadText)
			}
		}
	}
	var first struct {
		Source        string  `json:"source"`
		Kind          string  `json:"kind"`
		Statement     string  `json:"statement"`
		Confidence    float64 `json:"confidence"`
		URL           string  `json:"url"`
		TargetURL     string  `json:"target_url"`
		Method        string  `json:"method"`
		StatusCode    int     `json:"status_code"`
		ContentLength int64   `json:"content_length"`
		Depth         int     `json:"depth"`
		ParserID      string  `json:"parser_id"`
	}
	if err := json.Unmarshal(events[1].Payload, &first); err != nil {
		t.Fatalf("decode first fact event: %v", err)
	}
	if first.Source != "parser.katana_jsonl" || first.Kind != "surface" || first.ParserID != "katana_jsonl" {
		t.Fatalf("unexpected katana route identity: %#v", first)
	}
	if first.URL != "https://app.example/login" || first.TargetURL != "https://app.example" || first.Method != "GET" || first.StatusCode != 200 || first.ContentLength != 6726 || first.Depth != 2 {
		t.Fatalf("unexpected first katana route metadata: %#v", first)
	}
	if !strings.Contains(first.Statement, "katana") || !strings.Contains(first.Statement, "https://app.example/login") || !strings.Contains(first.Statement, "method=GET") || !strings.Contains(first.Statement, "status=200") {
		t.Fatalf("expected concise katana route statement, got %#v", first)
	}
	var second struct {
		URL           string `json:"url"`
		Method        string `json:"method"`
		StatusCode    int    `json:"status_code"`
		ContentLength int64  `json:"content_length"`
	}
	if err := json.Unmarshal(events[2].Payload, &second); err != nil {
		t.Fatalf("decode second fact event: %v", err)
	}
	if second.URL != "https://app.example/account" || second.Method != "POST" || second.StatusCode != 204 || second.ContentLength != 0 {
		t.Fatalf("unexpected nested katana route metadata: %#v", second)
	}
}

func TestParserEventsForToolExecutionRequiresKatanaURLScope(t *testing.T) {
	events := parserEventsForToolExecution("run-katana-no-url-scope", ToolExecutionRequest{
		Node: compiler.PlanNode{
			ID:           "katana-no-url-scope:01",
			Capability:   "web.route_enumeration",
			ManifestName: "web-api-pack",
			ToolID:       "katana",
			Operation:    "crawl",
			ParserID:     "katana_jsonl",
		},
		ScopeTargets: []ScopeTarget{{Kind: "domain", Value: "example.com"}},
	}, ToolExecutionResult{
		ExecutionID: "native-katana",
		Status:      "completed",
		Transcript:  `{"url":"https://app.example/login","status_code":200}`,
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
	if payload.Parser != "katana_jsonl" || payload.Status != "parse_failed" || payload.FindingCount != 0 {
		t.Fatalf("unexpected no-scope parse-failure payload: %#v", payload)
	}
}

func TestParserEventsForToolExecutionSkipsOutOfScopeKatanaRoutes(t *testing.T) {
	events := parserEventsForToolExecution("run-katana-out-of-scope", ToolExecutionRequest{
		Node: compiler.PlanNode{
			ID:           "katana-out-of-scope:01",
			Capability:   "web.route_enumeration",
			ManifestName: "web-api-pack",
			ToolID:       "katana",
			Operation:    "crawl",
			ParserID:     "katana_jsonl",
		},
		ScopeTargets: []ScopeTarget{{Kind: "url", Value: "https://app.example/app"}},
	}, ToolExecutionResult{
		ExecutionID: "native-katana",
		Status:      "completed",
		Transcript: strings.Join([]string{
			`{"url":"https://app.example/app/dashboard?token=secret-token","status_code":200}`,
			`{"url":"https://app.example/admin?token=secret-token","status_code":200}`,
			`{"url":"https://evil.example/out?token=secret-token","status_code":200}`,
		}, "\n"),
	})

	if len(events) != 2 {
		t.Fatalf("expected parser event and one in-scope route fact, got %#v", events)
	}
	var parserPayload struct {
		Parser       string `json:"parser"`
		Status       string `json:"status"`
		FindingCount int    `json:"finding_count"`
	}
	if err := json.Unmarshal(events[0].Payload, &parserPayload); err != nil {
		t.Fatalf("decode parser event: %v", err)
	}
	if parserPayload.Parser != "katana_jsonl" || parserPayload.Status != "parsed" || parserPayload.FindingCount != 1 {
		t.Fatalf("unexpected parser payload: %#v", parserPayload)
	}
	var payload struct {
		URL       string `json:"url"`
		TargetURL string `json:"target_url"`
	}
	if err := json.Unmarshal(events[1].Payload, &payload); err != nil {
		t.Fatalf("decode route fact: %v", err)
	}
	if payload.URL != "https://app.example/app/dashboard" || payload.TargetURL != "https://app.example/app" {
		t.Fatalf("expected only in-scope route under path scope, got %#v", payload)
	}
	for _, leaked := range []string{"evil.example", "token=", "secret-token", "https://app.example/admin"} {
		if strings.Contains(string(events[1].Payload), leaked) {
			t.Fatalf("katana route fact leaked out-of-scope/query data %q: %s", leaked, events[1].Payload)
		}
	}
}

func TestParserEventsForToolExecutionClampsKatanaNumericAndMethodFields(t *testing.T) {
	events := parserEventsForToolExecution("run-katana-clamps", ToolExecutionRequest{
		Node: compiler.PlanNode{
			ID:           "katana-clamps:01",
			Capability:   "web.route_enumeration",
			ManifestName: "web-api-pack",
			ToolID:       "katana",
			Operation:    "crawl",
			ParserID:     "katana_jsonl",
		},
		ScopeTargets: []ScopeTarget{{Kind: "url", Value: "https://app.example"}},
	}, ToolExecutionResult{
		ExecutionID: "native-katana",
		Status:      "completed",
		Transcript:  `{"url":"https://app.example/route","method":"GET /secret-token","status_code":999,"content_length":-42,"depth":-1}`,
	})

	if len(events) != 2 {
		t.Fatalf("expected parser event and one route fact, got %#v", events)
	}
	var payload struct {
		Statement     string `json:"statement"`
		Method        string `json:"method"`
		StatusCode    int    `json:"status_code"`
		ContentLength int64  `json:"content_length"`
		Depth         int    `json:"depth"`
	}
	if err := json.Unmarshal(events[1].Payload, &payload); err != nil {
		t.Fatalf("decode route fact: %v", err)
	}
	if payload.Method != "" || payload.StatusCode != 0 || payload.ContentLength != 0 || payload.Depth != 0 {
		t.Fatalf("expected invalid fields to clamp to zero/empty, got %#v", payload)
	}
	for _, leaked := range []string{"secret-token", "status=999", "length=-42", "depth=-1"} {
		if strings.Contains(string(events[1].Payload), leaked) || strings.Contains(payload.Statement, leaked) {
			t.Fatalf("katana route fact leaked invalid field %q: %s", leaked, events[1].Payload)
		}
	}
}

func TestParserEventsForToolExecutionRecordsKatanaParseFailureWithoutFact(t *testing.T) {
	events := parserEventsForToolExecution("run-katana-parse-failure", ToolExecutionRequest{
		Node: compiler.PlanNode{
			ID:           "katana-parse-failure:01",
			Capability:   "web.route_enumeration",
			ManifestName: "web-api-pack",
			ToolID:       "katana",
			Operation:    "crawl",
			ParserID:     "katana_jsonl",
		},
		ScopeTargets: []ScopeTarget{{Kind: "url", Value: "https://app.example"}},
	}, ToolExecutionResult{
		ExecutionID: "native-katana",
		Status:      "completed",
		Transcript:  "not-jsonl",
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
	if payload.Parser != "katana_jsonl" || payload.Status != "parse_failed" || payload.ToolID != "katana" || payload.FindingCount != 0 || payload.Message == "" {
		t.Fatalf("unexpected parse-failure payload: %#v", payload)
	}
}
