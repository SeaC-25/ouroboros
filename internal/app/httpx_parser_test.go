package app

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/ouroboros/ouroboros/internal/compiler"
	"github.com/ouroboros/ouroboros/internal/eventlog"
)

func TestParserEventsForToolExecutionPromotesHttpxSurface(t *testing.T) {
	events := parserEventsForToolExecution("run-httpx-surface", ToolExecutionRequest{
		Node: compiler.PlanNode{
			ID:           "httpx-surface:01",
			Capability:   "web.discovery",
			ManifestName: "web-api-pack",
			ToolID:       "httpx",
			Operation:    "http_probe",
			ParserID:     "httpx_jsonl",
		},
		ScopeTargets: []ScopeTarget{{Kind: "url", Value: "https://app.example"}},
	}, ToolExecutionResult{
		ExecutionID: "native-httpx",
		Status:      "completed",
		Transcript:  `{"url":"https://app.example/login","input":"https://app.example","status_code":200,"content_type":"text/html; charset=utf-8","content_length":6726,"title":"secret-token admin login","webserver":"nginx secret-token","tech":["React","secret-token"],"header":"Authorization: Bearer secret-token","headers":{"Authorization":"Bearer secret-token"},"raw_header":"Authorization: Bearer secret-token","response-header":"Authorization: Bearer secret-token","raw":"GET / HTTP/1.1\r\nAuthorization: Bearer secret-token","request":"secret-token","response":"secret-token","body":"secret-token","csp":"secret-token","csp_data":{"policy":"secret-token"},"chain":["secret-token"],"location":"https://app.example/next","final_url":"https://app.example/login","screenshot":"secret-token","screenshot_bytes":"secret-token"}`,
	})

	if len(events) != 2 {
		t.Fatalf("expected parser event and one surface fact, got %#v", events)
	}
	if events[0].Type != eventlog.EventParser || events[1].Type != eventlog.EventFact {
		t.Fatalf("expected parser event followed by fact event, got %#v", events)
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
	if parserPayload.Parser != "httpx_jsonl" || parserPayload.Status != "parsed" || parserPayload.FindingCount != 1 || parserPayload.ToolID != "httpx" {
		t.Fatalf("unexpected httpx parser payload: %#v", parserPayload)
	}
	payloadText := string(events[1].Payload)
	for _, leaked := range []string{"secret-token", "Authorization", "Bearer", "admin login"} {
		if strings.Contains(payloadText, leaked) {
			t.Fatalf("httpx surface fact leaked untrusted output %q: %s", leaked, payloadText)
		}
	}
	var rawPayload map[string]json.RawMessage
	if err := json.Unmarshal(events[1].Payload, &rawPayload); err != nil {
		t.Fatalf("decode raw fact event: %v", err)
	}
	for _, forbiddenKey := range []string{"title", "webserver", "tech", "header", "headers", "raw_header", "response-header", "raw", "request", "response", "body", "csp", "csp_data", "chain", "location", "final_url", "screenshot", "screenshot_bytes"} {
		if _, ok := rawPayload[forbiddenKey]; ok {
			t.Fatalf("httpx surface fact persisted forbidden key %q: %s", forbiddenKey, payloadText)
		}
	}
	var payload struct {
		Source        string  `json:"source"`
		Kind          string  `json:"kind"`
		Statement     string  `json:"statement"`
		Confidence    float64 `json:"confidence"`
		URL           string  `json:"url"`
		TargetURL     string  `json:"target_url"`
		StatusCode    int     `json:"status_code"`
		ContentType   string  `json:"content_type"`
		ContentLength int64   `json:"content_length"`
		ParserID      string  `json:"parser_id"`
	}
	if err := json.Unmarshal(events[1].Payload, &payload); err != nil {
		t.Fatalf("decode fact event: %v", err)
	}
	if payload.Source != "parser.httpx_jsonl" || payload.Kind != "surface" || payload.ParserID != "httpx_jsonl" {
		t.Fatalf("unexpected httpx surface payload identity: %#v", payload)
	}
	if payload.URL != "https://app.example/login" || payload.TargetURL != "https://app.example" || payload.StatusCode != 200 || payload.ContentType != "text/html" || payload.ContentLength != 6726 {
		t.Fatalf("unexpected structured httpx surface metadata: %#v", payload)
	}
	if !strings.Contains(payload.Statement, "httpx") || !strings.Contains(payload.Statement, "https://app.example/login") || !strings.Contains(payload.Statement, "status=200") {
		t.Fatalf("expected concise httpx surface statement, got %#v", payload)
	}
	if payload.Confidence < 0.6 {
		t.Fatalf("expected completed httpx surface to have useful confidence, got %#v", payload)
	}
}

func TestParserEventsForToolExecutionRequiresHttpxURLScope(t *testing.T) {
	events := parserEventsForToolExecution("run-httpx-no-url-scope", ToolExecutionRequest{
		Node: compiler.PlanNode{
			ID:           "httpx-no-url-scope:01",
			Capability:   "web.discovery",
			ManifestName: "web-api-pack",
			ToolID:       "httpx",
			Operation:    "http_probe",
			ParserID:     "httpx_jsonl",
		},
		ScopeTargets: []ScopeTarget{{Kind: "domain", Value: "example.com"}},
	}, ToolExecutionResult{
		ExecutionID: "native-httpx",
		Status:      "completed",
		Transcript:  `{"url":"https://evil.example","status_code":200}`,
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
	if payload.Parser != "httpx_jsonl" || payload.Status != "parse_failed" || payload.FindingCount != 0 {
		t.Fatalf("unexpected no-scope parse-failure payload: %#v", payload)
	}
}

func TestParserEventsForToolExecutionSanitizesHttpxURLAndCollapsesOutOfScope(t *testing.T) {
	events := parserEventsForToolExecution("run-httpx-url-sanitize", ToolExecutionRequest{
		Node: compiler.PlanNode{
			ID:           "httpx-url-sanitize:01",
			Capability:   "web.discovery",
			ManifestName: "web-api-pack",
			ToolID:       "httpx",
			Operation:    "http_probe",
			ParserID:     "httpx_jsonl",
		},
		ScopeTargets: []ScopeTarget{{Kind: "url", Value: "https://app.example"}},
	}, ToolExecutionResult{
		ExecutionID: "native-httpx",
		Status:      "completed",
		Transcript: strings.Join([]string{
			`{"url":"https://user:secret-token@app.example/admin?token=secret-token#frag","status_code":302,"location":"https://evil.example/landing?token=secret-token","final_url":"https://evil.example/landing?token=secret-token","response":"secret-token"}`,
			`{"url":"https://evil.example/out-of-scope?token=secret-token","status_code":200,"body":"secret-token"}`,
		}, "\n"),
	})

	if len(events) != 3 {
		t.Fatalf("expected parser event and two surface facts, got %#v", events)
	}
	var first struct {
		Statement     string `json:"statement"`
		URL           string `json:"url"`
		TargetURL     string `json:"target_url"`
		StatusCode    int    `json:"status_code"`
		ContentType   string `json:"content_type"`
		ContentLength int64  `json:"content_length"`
	}
	if err := json.Unmarshal(events[1].Payload, &first); err != nil {
		t.Fatalf("decode first fact event: %v", err)
	}
	var second struct {
		Statement     string `json:"statement"`
		URL           string `json:"url"`
		TargetURL     string `json:"target_url"`
		StatusCode    int    `json:"status_code"`
		ContentType   string `json:"content_type"`
		ContentLength int64  `json:"content_length"`
	}
	if err := json.Unmarshal(events[2].Payload, &second); err != nil {
		t.Fatalf("decode second fact event: %v", err)
	}
	if first.URL != "https://app.example/admin" || first.TargetURL != "https://app.example" {
		t.Fatalf("expected in-scope URL to be sanitized, got %#v", first)
	}
	if second.URL != "https://app.example" || second.TargetURL != "https://app.example" {
		t.Fatalf("expected out-of-scope URL to collapse to scoped target, got %#v", second)
	}
	if first.StatusCode != 0 || first.ContentType != "" || first.ContentLength != 0 {
		t.Fatalf("out-of-scope final response metadata must not be attributed to scoped URL, got %#v", first)
	}
	if second.StatusCode != 0 || second.ContentType != "" || second.ContentLength != 0 {
		t.Fatalf("out-of-scope response metadata must not be attributed to scoped URL, got %#v", second)
	}
	for _, payload := range []string{string(events[1].Payload), string(events[2].Payload), first.Statement, second.Statement} {
		for _, leaked := range []string{"secret-token", "evil.example", "?", "#", "user:"} {
			if strings.Contains(payload, leaked) {
				t.Fatalf("httpx surface payload leaked untrusted URL/output %q: %s", leaked, payload)
			}
		}
	}
}

func TestParserEventsForToolExecutionTreatsOutOfScopeHttpxURLAsUntrustedEvenWithScopedInput(t *testing.T) {
	events := parserEventsForToolExecution("run-httpx-out-of-scope-url-with-input", ToolExecutionRequest{
		Node: compiler.PlanNode{
			ID:           "httpx-out-of-scope-url-with-input:01",
			Capability:   "web.discovery",
			ManifestName: "web-api-pack",
			ToolID:       "httpx",
			Operation:    "http_probe",
			ParserID:     "httpx_jsonl",
		},
		ScopeTargets: []ScopeTarget{{Kind: "url", Value: "https://app.example"}},
	}, ToolExecutionResult{
		ExecutionID: "native-httpx",
		Status:      "completed",
		Transcript:  `{"url":"https://evil.example/out?token=secret-token","input":"https://app.example","status_code":200,"content_type":"text/html","content_length":99}`,
	})

	if len(events) != 2 {
		t.Fatalf("expected parser event and collapsed surface fact, got %#v", events)
	}
	var payload struct {
		Statement     string `json:"statement"`
		URL           string `json:"url"`
		TargetURL     string `json:"target_url"`
		StatusCode    int    `json:"status_code"`
		ContentType   string `json:"content_type"`
		ContentLength int64  `json:"content_length"`
	}
	if err := json.Unmarshal(events[1].Payload, &payload); err != nil {
		t.Fatalf("decode fact event: %v", err)
	}
	if payload.URL != "https://app.example" || payload.TargetURL != "https://app.example" {
		t.Fatalf("expected out-of-scope url to collapse to scoped target, got %#v", payload)
	}
	if payload.StatusCode != 0 || payload.ContentType != "" || payload.ContentLength != 0 {
		t.Fatalf("out-of-scope url response metadata must not be attributed to scoped input, got %#v", payload)
	}
	for _, leaked := range []string{"evil.example", "secret-token", "status=200", "length=99"} {
		if strings.Contains(string(events[1].Payload), leaked) || strings.Contains(payload.Statement, leaked) {
			t.Fatalf("httpx surface fact leaked out-of-scope response data %q: %s", leaked, events[1].Payload)
		}
	}
}

func TestParserEventsForToolExecutionDoesNotPromoteHttpxFinalURLAsSurface(t *testing.T) {
	events := parserEventsForToolExecution("run-httpx-final-url-only", ToolExecutionRequest{
		Node: compiler.PlanNode{
			ID:           "httpx-final-url-only:01",
			Capability:   "web.discovery",
			ManifestName: "web-api-pack",
			ToolID:       "httpx",
			Operation:    "http_probe",
			ParserID:     "httpx_jsonl",
		},
		ScopeTargets: []ScopeTarget{{Kind: "url", Value: "https://app.example"}},
	}, ToolExecutionResult{
		ExecutionID: "native-httpx",
		Status:      "completed",
		Transcript:  `{"final_url":"https://app.example/redirected","status_code":200,"content_type":"text/html","content_length":99}`,
	})

	if len(events) != 1 || events[0].Type != eventlog.EventParser {
		t.Fatalf("expected parser event only for final_url-only record, got %#v", events)
	}
	var payload struct {
		Parser       string `json:"parser"`
		Status       string `json:"status"`
		FindingCount int    `json:"finding_count"`
	}
	if err := json.Unmarshal(events[0].Payload, &payload); err != nil {
		t.Fatalf("decode parser event: %v", err)
	}
	if payload.Parser != "httpx_jsonl" || payload.Status != "parsed" || payload.FindingCount != 0 {
		t.Fatalf("expected parsed zero-fact payload for final_url-only record, got %#v", payload)
	}
}

func TestParserEventsForToolExecutionClampsHttpxNumericFields(t *testing.T) {
	events := parserEventsForToolExecution("run-httpx-numeric-clamp", ToolExecutionRequest{
		Node: compiler.PlanNode{
			ID:           "httpx-numeric-clamp:01",
			Capability:   "web.discovery",
			ManifestName: "web-api-pack",
			ToolID:       "httpx",
			Operation:    "http_probe",
			ParserID:     "httpx_jsonl",
		},
		ScopeTargets: []ScopeTarget{{Kind: "url", Value: "https://app.example"}},
	}, ToolExecutionResult{
		ExecutionID: "native-httpx",
		Status:      "completed",
		Transcript:  `{"url":"https://app.example/login","status_code":999,"content_type":"text/html","content_length":-42}`,
	})

	if len(events) != 2 {
		t.Fatalf("expected parser event and one surface fact, got %#v", events)
	}
	var payload struct {
		Statement     string `json:"statement"`
		StatusCode    int    `json:"status_code"`
		ContentLength int64  `json:"content_length"`
	}
	if err := json.Unmarshal(events[1].Payload, &payload); err != nil {
		t.Fatalf("decode fact event: %v", err)
	}
	if payload.StatusCode != 0 || payload.ContentLength != 0 {
		t.Fatalf("expected invalid numeric fields to clamp to zero, got %#v", payload)
	}
	if strings.Contains(payload.Statement, "status=999") || strings.Contains(payload.Statement, "length=-42") {
		t.Fatalf("statement must not persist invalid numeric fields: %#v", payload)
	}
}

func TestParserEventsForToolExecutionRecordsHttpxParseFailureWithoutFact(t *testing.T) {
	events := parserEventsForToolExecution("run-httpx-parse-failure", ToolExecutionRequest{
		Node: compiler.PlanNode{
			ID:           "httpx-parse-failure:01",
			Capability:   "web.discovery",
			ManifestName: "web-api-pack",
			ToolID:       "httpx",
			Operation:    "http_probe",
			ParserID:     "httpx_jsonl",
		},
		ScopeTargets: []ScopeTarget{{Kind: "url", Value: "https://app.example"}},
	}, ToolExecutionResult{
		ExecutionID: "native-httpx",
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
	if payload.Parser != "httpx_jsonl" || payload.Status != "parse_failed" || payload.ToolID != "httpx" || payload.FindingCount != 0 || payload.Message == "" {
		t.Fatalf("unexpected parse-failure payload: %#v", payload)
	}
}
