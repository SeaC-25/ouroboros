package app

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/ouroboros/ouroboros/internal/compiler"
	"github.com/ouroboros/ouroboros/internal/eventlog"
)

func TestParserEventsForToolExecutionPromotesDnsreconRecordSurface(t *testing.T) {
	events := parserEventsForToolExecution("run-dnsrecon-surface", dnsreconParserRequest([]ScopeTarget{{Kind: "domain", Value: "Example.COM"}}, nil), ToolExecutionResult{
		ExecutionID: "native-dnsrecon",
		Status:      "completed",
		Transcript: `[
{"type":"A","name":"API.Example.COM.","address":"192.0.2.10","target":"secret-token","strings":["secret-token"]},
{"type":"AAAA","name":"api.example.com","address":"2001:db8::10","zone_transfer":"secret-token"},
{"type":"CNAME","name":"www.example.com","target":"Alias.Example.COM.","raw":"secret-token"},
{"type":"NS","name":"example.com","target":"ns1.example.com"},
{"type":"A","name":"evil.example.net","address":"198.51.100.5"}
]`,
	})

	if len(events) != 5 {
		t.Fatalf("expected parser event and four dnsrecon surface facts, got %#v", events)
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
	if parserPayload.Parser != "dnsrecon_json" || parserPayload.ParserID != "dnsrecon_json" || parserPayload.Status != "parsed" || parserPayload.ToolID != "dnsrecon" || parserPayload.FindingCount != 4 {
		t.Fatalf("unexpected dnsrecon parser payload: %#v", parserPayload)
	}

	records := map[string]string{}
	for _, event := range events[1:] {
		payload := decodeDnsreconSurfacePayload(t, event)
		if payload.Source != "parser.dnsrecon_json" || payload.Kind != "surface" || payload.ParserID != "dnsrecon_json" {
			t.Fatalf("unexpected dnsrecon surface identity: %#v", payload)
		}
		if payload.Domain != "example.com" || payload.Host == "" || payload.RecordType == "" || payload.Value == "" {
			t.Fatalf("expected scoped dnsrecon metadata, got %#v", payload)
		}
		if !strings.Contains(payload.Statement, "dnsrecon") || !strings.Contains(payload.Statement, payload.RecordType) || !strings.Contains(payload.Statement, "domain=example.com") {
			t.Fatalf("dnsrecon statement is missing structured details: %#v", payload)
		}
		records[payload.RecordType+" "+payload.Host] = payload.Value
		for _, leaked := range []string{"secret-token", "evil.example.net", "strings", "raw", "zone_transfer"} {
			if strings.Contains(string(event.Payload), leaked) {
				t.Fatalf("dnsrecon surface leaked untrusted output %q: %s", leaked, event.Payload)
			}
		}
	}
	if records["A api.example.com"] != "192.0.2.10" || records["AAAA api.example.com"] != "2001:db8::10" || records["CNAME www.example.com"] != "alias.example.com" || records["NS example.com"] != "ns1.example.com" {
		t.Fatalf("unexpected dnsrecon promoted records: %#v", records)
	}
}

func TestParserEventsForToolExecutionRequiresDnsreconDomainScope(t *testing.T) {
	events := parserEventsForToolExecution("run-dnsrecon-url-scope", dnsreconParserRequest([]ScopeTarget{{Kind: "url", Value: "https://app.example.com"}}, nil), ToolExecutionResult{
		ExecutionID: "native-dnsrecon",
		Status:      "completed",
		Transcript:  `[{"type":"A","name":"api.example.com","address":"192.0.2.10"}]`,
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
	if payload.Parser != "dnsrecon_json" || payload.Status != "parse_failed" || payload.FindingCount != 0 || !strings.Contains(payload.Message, "domain scope") {
		t.Fatalf("unexpected dnsrecon no-domain parse-failure payload: %#v", payload)
	}
}

func TestParserEventsForToolExecutionRecordsDnsreconParseFailureWithoutFact(t *testing.T) {
	events := parserEventsForToolExecution("run-dnsrecon-parse-failure", dnsreconParserRequest([]ScopeTarget{{Kind: "domain", Value: "example.com"}}, nil), ToolExecutionResult{
		ExecutionID: "native-dnsrecon",
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
	if payload.Parser != "dnsrecon_json" || payload.Status != "parse_failed" || payload.FindingCount != 0 {
		t.Fatalf("unexpected dnsrecon parse-failure payload: %#v", payload)
	}
}

func dnsreconParserRequest(targets []ScopeTarget, exclusions []string) ToolExecutionRequest {
	return ToolExecutionRequest{
		Node: compiler.PlanNode{
			ID:           "dnsrecon-surface:01",
			Capability:   "infra.host_discovery",
			ManifestName: "infra-pack",
			ToolID:       "dnsrecon",
			Operation:    "dns_probe",
			ParserID:     "dnsrecon_json",
		},
		ScopeTargets:    targets,
		ScopeExclusions: exclusions,
		IsolationMode:   "process",
	}
}

type dnsreconSurfacePayload struct {
	Source     string  `json:"source"`
	Kind       string  `json:"kind"`
	Statement  string  `json:"statement"`
	ParserID   string  `json:"parser_id"`
	Domain     string  `json:"domain"`
	Host       string  `json:"host"`
	RecordType string  `json:"record_type"`
	Value      string  `json:"value"`
	Address    string  `json:"address"`
	CNAME      string  `json:"cname"`
	Confidence float64 `json:"confidence"`
}

func decodeDnsreconSurfacePayload(t *testing.T, event eventlog.Event) dnsreconSurfacePayload {
	t.Helper()
	if event.Type != eventlog.EventFact {
		t.Fatalf("expected fact event, got %#v", event)
	}
	var payload dnsreconSurfacePayload
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		t.Fatalf("decode dnsrecon surface payload: %v", err)
	}
	return payload
}
