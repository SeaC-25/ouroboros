package app

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/ouroboros/ouroboros/internal/compiler"
	"github.com/ouroboros/ouroboros/internal/eventlog"
)

func TestParserEventsForToolExecutionPromotesFfufRouteSurface(t *testing.T) {
	events := parserEventsForToolExecution("run-ffuf-route", ToolExecutionRequest{
		Node: compiler.PlanNode{
			ID:           "ffuf-route:01",
			Capability:   "web.route_enumeration",
			ManifestName: "web-api-pack",
			ToolID:       "ffuf",
			Operation:    "path_fuzz",
			ParserID:     "ffuf_json",
		},
		ScopeTargets: []ScopeTarget{{Kind: "url", Value: "https://app.example"}},
	}, ToolExecutionResult{
		ExecutionID: "native-ffuf",
		Status:      "completed",
		Transcript: strings.Join([]string{
			`{"input":{"FUZZ":"secret-token"},"position":1,"status":200,"length":6726,"words":120,"lines":32,"content-type":"text/html; charset=utf-8","redirectlocation":"https://evil.example/secret-token","url":"https://app.example/admin?token=secret-token#frag","duration":123456,"resultfile":"/tmp/secret-token","host":"app.example","scraper":{"secret":["secret-token"]}}`,
			`{"results":[{"input":{"FUZZ":"api-secret-token"},"status":204,"length":0,"words":0,"lines":0,"url":"https://app.example/api","content-type":"application/json","raw":"secret-token","request":"secret-token","response":"secret-token","headers":{"Authorization":"Bearer secret-token"}}]}`,
		}, "\n"),
	})

	if len(events) != 3 {
		t.Fatalf("expected parser event and two ffuf route facts, got %#v", events)
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
	if parserPayload.Parser != "ffuf_json" || parserPayload.Status != "parsed" || parserPayload.FindingCount != 2 || parserPayload.ToolID != "ffuf" {
		t.Fatalf("unexpected ffuf parser payload: %#v", parserPayload)
	}
	for _, event := range events[1:] {
		payloadText := string(event.Payload)
		for _, leaked := range []string{"secret-token", "Authorization", "Bearer", "evil.example", "resultfile", "scraper", "redirectlocation"} {
			if strings.Contains(payloadText, leaked) {
				t.Fatalf("ffuf route fact leaked untrusted output %q: %s", leaked, payloadText)
			}
		}
		var rawPayload map[string]json.RawMessage
		if err := json.Unmarshal(event.Payload, &rawPayload); err != nil {
			t.Fatalf("decode raw fact event: %v", err)
		}
		for _, forbiddenKey := range []string{"input", "position", "redirectlocation", "duration", "resultfile", "host", "scraper", "raw", "request", "response", "headers", "body"} {
			if _, ok := rawPayload[forbiddenKey]; ok {
				t.Fatalf("ffuf route fact persisted forbidden key %q: %s", forbiddenKey, payloadText)
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
		StatusCode    int     `json:"status_code"`
		ContentLength int64   `json:"content_length"`
		ContentWords  int64   `json:"content_words"`
		ContentLines  int64   `json:"content_lines"`
		ContentType   string  `json:"content_type"`
		ParserID      string  `json:"parser_id"`
	}
	if err := json.Unmarshal(events[1].Payload, &first); err != nil {
		t.Fatalf("decode first fact event: %v", err)
	}
	if first.Source != "parser.ffuf_json" || first.Kind != "surface" || first.ParserID != "ffuf_json" {
		t.Fatalf("unexpected ffuf route identity: %#v", first)
	}
	if first.URL != "https://app.example/admin" || first.TargetURL != "https://app.example" || first.StatusCode != 200 || first.ContentLength != 6726 || first.ContentWords != 120 || first.ContentLines != 32 || first.ContentType != "text/html" {
		t.Fatalf("unexpected first ffuf route metadata: %#v", first)
	}
	if !strings.Contains(first.Statement, "ffuf") || !strings.Contains(first.Statement, "https://app.example/admin") || !strings.Contains(first.Statement, "status=200") || !strings.Contains(first.Statement, "length=6726") {
		t.Fatalf("expected concise ffuf route statement, got %#v", first)
	}
	var second struct {
		URL           string `json:"url"`
		StatusCode    int    `json:"status_code"`
		ContentLength int64  `json:"content_length"`
		ContentWords  int64  `json:"content_words"`
		ContentLines  int64  `json:"content_lines"`
		ContentType   string `json:"content_type"`
	}
	if err := json.Unmarshal(events[2].Payload, &second); err != nil {
		t.Fatalf("decode second fact event: %v", err)
	}
	if second.URL != "https://app.example/api" || second.StatusCode != 204 || second.ContentLength != 0 || second.ContentWords != 0 || second.ContentLines != 0 || second.ContentType != "application/json" {
		t.Fatalf("unexpected wrapped ffuf route metadata: %#v", second)
	}
}

func TestParserEventsForToolExecutionRequiresFfufURLScope(t *testing.T) {
	events := parserEventsForToolExecution("run-ffuf-no-url-scope", ToolExecutionRequest{
		Node: compiler.PlanNode{
			ID:           "ffuf-no-url-scope:01",
			Capability:   "web.route_enumeration",
			ManifestName: "web-api-pack",
			ToolID:       "ffuf",
			Operation:    "path_fuzz",
			ParserID:     "ffuf_json",
		},
		ScopeTargets: []ScopeTarget{{Kind: "domain", Value: "example.com"}},
	}, ToolExecutionResult{
		ExecutionID: "native-ffuf",
		Status:      "completed",
		Transcript:  `{"url":"https://app.example/admin","status":200}`,
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
	if payload.Parser != "ffuf_json" || payload.Status != "parse_failed" || payload.FindingCount != 0 {
		t.Fatalf("unexpected no-scope parse-failure payload: %#v", payload)
	}
}

func TestParserEventsForToolExecutionSkipsOutOfScopeFfufRoutes(t *testing.T) {
	events := parserEventsForToolExecution("run-ffuf-out-of-scope", ToolExecutionRequest{
		Node: compiler.PlanNode{
			ID:           "ffuf-out-of-scope:01",
			Capability:   "web.route_enumeration",
			ManifestName: "web-api-pack",
			ToolID:       "ffuf",
			Operation:    "path_fuzz",
			ParserID:     "ffuf_json",
		},
		ScopeTargets: []ScopeTarget{{Kind: "url", Value: "https://app.example/app"}},
	}, ToolExecutionResult{
		ExecutionID: "native-ffuf",
		Status:      "completed",
		Transcript: strings.Join([]string{
			`{"url":"https://app.example/app/admin?token=secret-token","status":200}`,
			`{"url":"https://app.example/admin?token=secret-token","status":200}`,
			`{"url":"https://evil.example/out?token=secret-token","status":200}`,
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
		t.Fatalf("decode parser payload: %v", err)
	}
	if parserPayload.Parser != "ffuf_json" || parserPayload.Status != "parsed" || parserPayload.FindingCount != 1 {
		t.Fatalf("unexpected parser payload: %#v", parserPayload)
	}
	var payload struct {
		URL       string `json:"url"`
		TargetURL string `json:"target_url"`
	}
	if err := json.Unmarshal(events[1].Payload, &payload); err != nil {
		t.Fatalf("decode route fact: %v", err)
	}
	if payload.URL != "https://app.example/app/admin" || payload.TargetURL != "https://app.example/app" {
		t.Fatalf("expected only in-scope route under path scope, got %#v", payload)
	}
	for _, leaked := range []string{"evil.example", "token=", "secret-token", "https://app.example/admin"} {
		if strings.Contains(string(events[1].Payload), leaked) {
			t.Fatalf("ffuf route fact leaked out-of-scope/query data %q: %s", leaked, events[1].Payload)
		}
	}
}

func TestParserEventsForToolExecutionSkipsExcludedFfufRoutes(t *testing.T) {
	events := parserEventsForToolExecution("run-ffuf-excluded-route", ToolExecutionRequest{
		Node: compiler.PlanNode{
			ID:           "ffuf-excluded-route:01",
			Capability:   "web.route_enumeration",
			ManifestName: "web-api-pack",
			ToolID:       "ffuf",
			Operation:    "path_fuzz",
			ParserID:     "ffuf_json",
		},
		ScopeTargets:    []ScopeTarget{{Kind: "url", Value: "https://app.example"}},
		ScopeExclusions: []string{"https://app.example/admin"},
	}, ToolExecutionResult{
		ExecutionID: "native-ffuf",
		Status:      "completed",
		Transcript: strings.Join([]string{
			`{"url":"https://app.example/admin?token=secret-token","status":200}`,
			`{"url":"https://app.example/login","status":200}`,
		}, "\n"),
	})

	if len(events) != 2 {
		t.Fatalf("expected parser event and one non-excluded route fact, got %#v", events)
	}
	var parserPayload struct {
		Parser       string `json:"parser"`
		Status       string `json:"status"`
		FindingCount int    `json:"finding_count"`
	}
	if err := json.Unmarshal(events[0].Payload, &parserPayload); err != nil {
		t.Fatalf("decode parser payload: %v", err)
	}
	if parserPayload.Parser != "ffuf_json" || parserPayload.Status != "parsed" || parserPayload.FindingCount != 1 {
		t.Fatalf("unexpected parser payload: %#v", parserPayload)
	}
	var payload struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(events[1].Payload, &payload); err != nil {
		t.Fatalf("decode route fact: %v", err)
	}
	if payload.URL != "https://app.example/login" {
		t.Fatalf("expected only non-excluded route, got %#v", payload)
	}
	for _, leaked := range []string{"https://app.example/admin", "token=", "secret-token"} {
		if strings.Contains(string(events[1].Payload), leaked) {
			t.Fatalf("ffuf route fact leaked excluded/query data %q: %s", leaked, events[1].Payload)
		}
	}
}

func TestParserEventsForToolExecutionClampsFfufNumericFields(t *testing.T) {
	events := parserEventsForToolExecution("run-ffuf-clamps", ToolExecutionRequest{
		Node: compiler.PlanNode{
			ID:           "ffuf-clamps:01",
			Capability:   "web.route_enumeration",
			ManifestName: "web-api-pack",
			ToolID:       "ffuf",
			Operation:    "path_fuzz",
			ParserID:     "ffuf_json",
		},
		ScopeTargets: []ScopeTarget{{Kind: "url", Value: "https://app.example"}},
	}, ToolExecutionResult{
		ExecutionID: "native-ffuf",
		Status:      "completed",
		Transcript:  `{"url":"https://app.example/route","status":999,"length":-42,"words":-7,"lines":-3,"content-type":"not a content type"}`,
	})

	if len(events) != 2 {
		t.Fatalf("expected parser event and one route fact, got %#v", events)
	}
	var payload struct {
		Statement     string `json:"statement"`
		StatusCode    int    `json:"status_code"`
		ContentLength int64  `json:"content_length"`
		ContentWords  int64  `json:"content_words"`
		ContentLines  int64  `json:"content_lines"`
		ContentType   string `json:"content_type"`
	}
	if err := json.Unmarshal(events[1].Payload, &payload); err != nil {
		t.Fatalf("decode route fact: %v", err)
	}
	if payload.StatusCode != 0 || payload.ContentLength != 0 || payload.ContentWords != 0 || payload.ContentLines != 0 || payload.ContentType != "" {
		t.Fatalf("expected invalid fields to clamp to zero/empty, got %#v", payload)
	}
	for _, leaked := range []string{"status=999", "length=-42", "words=-7", "lines=-3", "not a content type"} {
		if strings.Contains(string(events[1].Payload), leaked) || strings.Contains(payload.Statement, leaked) {
			t.Fatalf("ffuf route fact leaked invalid field %q: %s", leaked, events[1].Payload)
		}
	}
}

func TestParserEventsForToolExecutionRecordsFfufParseFailureWithoutFact(t *testing.T) {
	events := parserEventsForToolExecution("run-ffuf-parse-failure", ToolExecutionRequest{
		Node: compiler.PlanNode{
			ID:           "ffuf-parse-failure:01",
			Capability:   "web.route_enumeration",
			ManifestName: "web-api-pack",
			ToolID:       "ffuf",
			Operation:    "path_fuzz",
			ParserID:     "ffuf_json",
		},
		ScopeTargets: []ScopeTarget{{Kind: "url", Value: "https://app.example"}},
	}, ToolExecutionResult{
		ExecutionID: "native-ffuf",
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
	if payload.Parser != "ffuf_json" || payload.Status != "parse_failed" || payload.ToolID != "ffuf" || payload.FindingCount != 0 || payload.Message == "" {
		t.Fatalf("unexpected parse-failure payload: %#v", payload)
	}
}
