package app

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/ouroboros/ouroboros/internal/compiler"
	"github.com/ouroboros/ouroboros/internal/eventlog"
)

func TestParserEventsForToolExecutionPromotesDnsxRecordSurface(t *testing.T) {
	events := parserEventsForToolExecution("run-dnsx-surface", dnsxParserRequest([]ScopeTarget{{Kind: "domain", Value: "Example.COM"}}, nil, ""), ToolExecutionResult{
		ExecutionID: "native-dnsx",
		Status:      "completed",
		Transcript: strings.Join([]string{
			`{"host":"API.Example.COM.","a":["192.0.2.10"],"aaaa":["2001:db8::10"],"cname":["Alias.Example.COM."],"resolver":["1.1.1.1"],"raw":"secret-token","all":["secret-token"],"cdn":true,"cdn_name":"secret-token","asn":{"asn":"secret-token"},"timestamp":"2026-06-03T00:00:00Z","error":"secret-token"}`,
		}, "\n"),
	})

	if len(events) != 4 {
		t.Fatalf("expected parser event and three DNS record surface facts, got %#v", events)
	}
	if events[0].Type != eventlog.EventParser {
		t.Fatalf("expected first event to be parser event, got %#v", events[0])
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
	if parserPayload.Parser != "dnsx_jsonl" || parserPayload.ParserID != "dnsx_jsonl" || parserPayload.Status != "parsed" || parserPayload.ToolID != "dnsx" || parserPayload.FindingCount != 3 {
		t.Fatalf("unexpected dnsx parser payload: %#v", parserPayload)
	}

	records := map[string]string{}
	for _, event := range events[1:] {
		payload := decodeDnsxSurfacePayload(t, event)
		if payload.Source != "parser.dnsx_jsonl" || payload.Kind != "surface" || payload.ParserID != "dnsx_jsonl" {
			t.Fatalf("unexpected dnsx surface identity: %#v", payload)
		}
		if payload.Domain != "example.com" || payload.Host != "api.example.com" {
			t.Fatalf("unexpected dnsx scoped metadata: %#v", payload)
		}
		if !strings.Contains(payload.Statement, "dnsx") || !strings.Contains(payload.Statement, payload.Host) || !strings.Contains(payload.Statement, payload.RecordType) || !strings.Contains(payload.Statement, payload.Value) {
			t.Fatalf("dnsx statement is missing structured record details: %#v", payload)
		}
		if payload.Confidence < 0.6 {
			t.Fatalf("expected useful dnsx confidence, got %#v", payload)
		}
		records[payload.RecordType] = payload.Value

		payloadText := string(event.Payload)
		for _, leaked := range []string{"secret-token", "1.1.1.1", "resolver", "raw", "all", "cdn", "asn", "timestamp", "error"} {
			if strings.Contains(payloadText, leaked) {
				t.Fatalf("dnsx surface fact leaked untrusted output %q: %s", leaked, payloadText)
			}
		}
		var raw map[string]json.RawMessage
		if err := json.Unmarshal(event.Payload, &raw); err != nil {
			t.Fatalf("decode raw fact event: %v", err)
		}
		for key := range raw {
			if !allowedDnsxFactKey(key) {
				t.Fatalf("dnsx surface fact persisted forbidden key %q: %s", key, payloadText)
			}
		}
	}
	if records["A"] != "192.0.2.10" || records["AAAA"] != "2001:db8::10" || records["CNAME"] != "alias.example.com" {
		t.Fatalf("unexpected dnsx promoted records: %#v", records)
	}
}

func TestParserEventsForToolExecutionRequiresDnsxDomainScope(t *testing.T) {
	events := parserEventsForToolExecution("run-dnsx-url-scope", dnsxParserRequest([]ScopeTarget{{Kind: "url", Value: "https://app.example.com"}}, nil, "disabled"), ToolExecutionResult{
		ExecutionID: "native-dnsx",
		Status:      "completed",
		Transcript:  `{"host":"api.example.com","a":["192.0.2.10"]}`,
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
	if payload.Parser != "dnsx_jsonl" || payload.Status != "parse_failed" || payload.FindingCount != 0 || !strings.Contains(payload.Message, "domain scope") {
		t.Fatalf("unexpected no-domain-scope parse-failure payload: %#v", payload)
	}
}

func TestParserEventsForToolExecutionSkipsOutOfScopeDnsxRecords(t *testing.T) {
	events := parserEventsForToolExecution("run-dnsx-skip-out-of-scope", dnsxParserRequest([]ScopeTarget{{Kind: "domain", Value: "example.com"}}, []string{"blocked.example.com", "*.internal.example.com"}, ""), ToolExecutionResult{
		ExecutionID: "native-dnsx",
		Status:      "completed",
		Transcript: strings.Join([]string{
			`{"host":"api.evil.test","a":["198.51.100.9"],"cname":["kept.example.com"],"raw":"secret-token"}`,
			`{"host":"blocked.example.com","a":["192.0.2.20"],"raw":"secret-token"}`,
			`{"host":"www.internal.example.com","a":["192.0.2.21"],"raw":"secret-token"}`,
			`{"host":"kept.example.com","a":["192.0.2.22"],"cname":["outside.evil.test"]}`,
		}, "\n"),
	})

	if len(events) != 2 {
		t.Fatalf("expected parser event and one kept surface fact, got %#v", events)
	}
	payload := decodeDnsxSurfacePayload(t, events[1])
	if payload.Host != "kept.example.com" || payload.RecordType != "A" || payload.Value != "192.0.2.22" {
		t.Fatalf("expected only in-scope non-excluded A record to promote, got %#v", payload)
	}
	for _, leaked := range []string{"evil.test", "blocked.example.com", "internal.example.com", "192.0.2.20", "192.0.2.21", "secret-token"} {
		if strings.Contains(string(events[1].Payload), leaked) {
			t.Fatalf("dnsx surface leaked skipped value %q: %s", leaked, events[1].Payload)
		}
	}
}

func TestParserEventsForToolExecutionRecordsDnsxParseFailureWithoutFact(t *testing.T) {
	events := parserEventsForToolExecution("run-dnsx-parse-failure", dnsxParserRequest([]ScopeTarget{{Kind: "domain", Value: "example.com"}}, nil, ""), ToolExecutionResult{
		ExecutionID: "native-dnsx",
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
	if payload.Parser != "dnsx_jsonl" || payload.Status != "parse_failed" || payload.ToolID != "dnsx" || payload.FindingCount != 0 || payload.Message == "" {
		t.Fatalf("unexpected parse-failure payload: %#v", payload)
	}
}

func TestParserEventsForToolExecutionCapsRawDnsxJSONLOutput(t *testing.T) {
	events := parserEventsForToolExecution("run-dnsx-oversized-output", dnsxParserRequest([]ScopeTarget{{Kind: "domain", Value: "example.com"}}, nil, ""), ToolExecutionResult{
		ExecutionID: "native-dnsx",
		Status:      "completed",
		Transcript:  strings.Repeat(" ", (1<<20)+1) + `{"host":"api.example.com","a":["192.0.2.10"]}`,
	})

	if len(events) != 1 || events[0].Type != eventlog.EventParser {
		t.Fatalf("expected one parse-failure parser event, got %#v", events)
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
	if payload.Parser != "dnsx_jsonl" || payload.Status != "parse_failed" || payload.FindingCount != 0 || !strings.Contains(payload.Message, "exceeds") {
		t.Fatalf("unexpected oversized-output parse-failure payload: %#v", payload)
	}
}

func dnsxParserRequest(targets []ScopeTarget, exclusions []string, targetPolicy string) ToolExecutionRequest {
	return ToolExecutionRequest{
		Node: compiler.PlanNode{
			ID:           "dnsx-surface:01",
			Capability:   "infra.host_discovery",
			ManifestName: "infra-pack",
			ToolID:       "dnsx",
			Operation:    "dns_probe",
			ParserID:     "dnsx_jsonl",
		},
		ScopeTargets:      targets,
		ScopeExclusions:   exclusions,
		ScopeTargetPolicy: targetPolicy,
		IsolationMode:     "process",
	}
}

type dnsxSurfacePayload struct {
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
	Host        string  `json:"host"`
	RecordType  string  `json:"record_type"`
	Value       string  `json:"value"`
	Address     string  `json:"address"`
	CNAME       string  `json:"cname"`
}

func decodeDnsxSurfacePayload(t *testing.T, event eventlog.Event) dnsxSurfacePayload {
	t.Helper()
	if event.Type != eventlog.EventFact {
		t.Fatalf("expected fact event, got %#v", event)
	}
	var payload dnsxSurfacePayload
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		t.Fatalf("decode fact event: %v", err)
	}
	return payload
}

func allowedDnsxFactKey(key string) bool {
	switch key {
	case "fact_id", "kind", "statement", "confidence", "source", "parser", "parser_id", "plan_node_id", "tool", "tool_id", "operation", "execution_id", "domain", "host", "record_type", "value", "address", "cname":
		return true
	default:
		return false
	}
}
