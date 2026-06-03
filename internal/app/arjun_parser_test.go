package app

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/ouroboros/ouroboros/internal/compiler"
	"github.com/ouroboros/ouroboros/internal/eventlog"
)

func TestParserEventsForToolExecutionPromotesArjunParameterSurface(t *testing.T) {
	events := parserEventsForToolExecution("run-arjun-params", ToolExecutionRequest{
		Node: compiler.PlanNode{
			ID:           "arjun-params:01",
			Capability:   "web.route_enumeration",
			ManifestName: "web-api-pack",
			ToolID:       "arjun",
			Operation:    "parameter_discovery",
			ParserID:     "arjun_json",
		},
		ScopeTargets: []ScopeTarget{{Kind: "url", Value: "https://app.example/search"}},
	}, ToolExecutionResult{
		ExecutionID: "native-arjun",
		Status:      "completed",
		Transcript: `{
  "https://app.example/search?token=secret-token#frag": ["q", "page"],
  "https://evil.example/out?token=secret-token": ["evil"],
  "headers": {"Authorization": "Bearer secret-token"},
  "raw": "secret-token"
}`,
	})

	if len(events) != 3 {
		t.Fatalf("expected parser event and two parameter facts, got %#v", events)
	}
	if events[0].Type != eventlog.EventParser || events[1].Type != eventlog.EventFact || events[2].Type != eventlog.EventFact {
		t.Fatalf("expected parser event followed by parameter fact events, got %#v", events)
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
	if parserPayload.Parser != "arjun_json" || parserPayload.Status != "parsed" || parserPayload.FindingCount != 2 || parserPayload.ToolID != "arjun" {
		t.Fatalf("unexpected arjun parser payload: %#v", parserPayload)
	}
	for _, event := range events[1:] {
		payloadText := string(event.Payload)
		for _, leaked := range []string{"secret-token", "Authorization", "Bearer", "evil.example", "token=", "#frag", "raw", "headers"} {
			if strings.Contains(payloadText, leaked) {
				t.Fatalf("arjun parameter fact leaked untrusted output %q: %s", leaked, payloadText)
			}
		}
		var rawPayload map[string]json.RawMessage
		if err := json.Unmarshal(event.Payload, &rawPayload); err != nil {
			t.Fatalf("decode raw fact event: %v", err)
		}
		for _, forbiddenKey := range []string{"raw", "headers", "request", "response", "body", "payload", "evidence"} {
			if _, ok := rawPayload[forbiddenKey]; ok {
				t.Fatalf("arjun parameter fact persisted forbidden key %q: %s", forbiddenKey, payloadText)
			}
		}
	}
	var first struct {
		Source     string  `json:"source"`
		Kind       string  `json:"kind"`
		Statement  string  `json:"statement"`
		Confidence float64 `json:"confidence"`
		URL        string  `json:"url"`
		TargetURL  string  `json:"target_url"`
		Parameter  string  `json:"parameter"`
		ParserID   string  `json:"parser_id"`
	}
	if err := json.Unmarshal(events[1].Payload, &first); err != nil {
		t.Fatalf("decode first fact event: %v", err)
	}
	if first.Source != "parser.arjun_json" || first.Kind != "surface" || first.ParserID != "arjun_json" {
		t.Fatalf("unexpected arjun surface identity: %#v", first)
	}
	if first.URL != "https://app.example/search" || first.TargetURL != "https://app.example/search" || first.Parameter != "q" {
		t.Fatalf("unexpected first arjun parameter metadata: %#v", first)
	}
	if !strings.Contains(first.Statement, "arjun") || !strings.Contains(first.Statement, "https://app.example/search") || !strings.Contains(first.Statement, "parameter=q") {
		t.Fatalf("expected concise arjun parameter statement, got %#v", first)
	}
}

func TestParserEventsForToolExecutionRequiresArjunURLScope(t *testing.T) {
	events := parserEventsForToolExecution("run-arjun-no-url-scope", ToolExecutionRequest{
		Node: compiler.PlanNode{
			ID:           "arjun-no-url-scope:01",
			Capability:   "web.route_enumeration",
			ManifestName: "web-api-pack",
			ToolID:       "arjun",
			Operation:    "parameter_discovery",
			ParserID:     "arjun_json",
		},
		ScopeTargets: []ScopeTarget{{Kind: "domain", Value: "example.com"}},
	}, ToolExecutionResult{
		ExecutionID: "native-arjun",
		Status:      "completed",
		Transcript:  `{"https://app.example/search":["q"]}`,
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
	if payload.Parser != "arjun_json" || payload.Status != "parse_failed" || payload.FindingCount != 0 {
		t.Fatalf("unexpected no-scope parse-failure payload: %#v", payload)
	}
}

func TestParserEventsForToolExecutionSkipsOutOfScopeAndExcludedArjunParameters(t *testing.T) {
	events := parserEventsForToolExecution("run-arjun-out-of-scope", ToolExecutionRequest{
		Node: compiler.PlanNode{
			ID:           "arjun-out-of-scope:01",
			Capability:   "web.route_enumeration",
			ManifestName: "web-api-pack",
			ToolID:       "arjun",
			Operation:    "parameter_discovery",
			ParserID:     "arjun_json",
		},
		ScopeTargets:    []ScopeTarget{{Kind: "url", Value: "https://app.example/app"}},
		ScopeExclusions: []string{"https://app.example/app/admin"},
	}, ToolExecutionResult{
		ExecutionID: "native-arjun",
		Status:      "completed",
		Transcript: `{
  "https://app.example/app/search?token=secret-token": ["q"],
  "https://app.example/app/admin?token=secret-token": ["admin"],
  "https://app.example/root?token=secret-token": ["root"],
  "https://evil.example/out?token=secret-token": ["evil"]
}`,
	})

	if len(events) != 2 {
		t.Fatalf("expected parser event and one in-scope parameter fact, got %#v", events)
	}
	var parserPayload struct {
		Parser       string `json:"parser"`
		Status       string `json:"status"`
		FindingCount int    `json:"finding_count"`
	}
	if err := json.Unmarshal(events[0].Payload, &parserPayload); err != nil {
		t.Fatalf("decode parser payload: %v", err)
	}
	if parserPayload.Parser != "arjun_json" || parserPayload.Status != "parsed" || parserPayload.FindingCount != 1 {
		t.Fatalf("unexpected parser payload: %#v", parserPayload)
	}
	var payload struct {
		URL       string `json:"url"`
		TargetURL string `json:"target_url"`
		Parameter string `json:"parameter"`
	}
	if err := json.Unmarshal(events[1].Payload, &payload); err != nil {
		t.Fatalf("decode parameter fact: %v", err)
	}
	if payload.URL != "https://app.example/app/search" || payload.TargetURL != "https://app.example/app" || payload.Parameter != "q" {
		t.Fatalf("expected only in-scope non-excluded parameter, got %#v", payload)
	}
	for _, leaked := range []string{"evil.example", "token=", "secret-token", "admin", "root"} {
		if strings.Contains(string(events[1].Payload), leaked) {
			t.Fatalf("arjun parameter fact leaked out-of-scope/excluded/query data %q: %s", leaked, events[1].Payload)
		}
	}
}

func TestParserEventsForToolExecutionRecordsArjunParseFailureWithoutFact(t *testing.T) {
	events := parserEventsForToolExecution("run-arjun-parse-failure", ToolExecutionRequest{
		Node: compiler.PlanNode{
			ID:           "arjun-parse-failure:01",
			Capability:   "web.route_enumeration",
			ManifestName: "web-api-pack",
			ToolID:       "arjun",
			Operation:    "parameter_discovery",
			ParserID:     "arjun_json",
		},
		ScopeTargets: []ScopeTarget{{Kind: "url", Value: "https://app.example"}},
	}, ToolExecutionResult{
		ExecutionID: "native-arjun",
		Status:      "completed",
		Transcript:  `not-json`,
	})

	if len(events) != 1 || events[0].Type != eventlog.EventParser {
		t.Fatalf("expected one parser failure event for malformed arjun output, got %#v", events)
	}
	var payload struct {
		Parser       string `json:"parser"`
		Status       string `json:"status"`
		FindingCount int    `json:"finding_count"`
	}
	if err := json.Unmarshal(events[0].Payload, &payload); err != nil {
		t.Fatalf("decode parser event: %v", err)
	}
	if payload.Parser != "arjun_json" || payload.Status != "parse_failed" || payload.FindingCount != 0 {
		t.Fatalf("unexpected malformed parse-failure payload: %#v", payload)
	}
}
