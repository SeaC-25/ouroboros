package app

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/ouroboros/ouroboros/internal/compiler"
	"github.com/ouroboros/ouroboros/internal/eventlog"
)

func TestParserEventsForToolExecutionPromotesNaabuOpenPortSurface(t *testing.T) {
	events := parserEventsForToolExecution("run-naabu-surface", naabuParserRequest("naabu-surface:01", []ScopeTarget{{Kind: "cidr", Value: "10.0.0.0/24"}}, nil), ToolExecutionResult{
		ExecutionID: "native-naabu",
		Status:      "completed",
		Transcript:  `{"ip":"10.0.0.5","port":443,"protocol":"tcp","host":"secret.example","timestamp":"secret-token","cdn":"secret-token","banner":"secret-token","error":"secret-token"}`,
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
	if parserPayload.Parser != "naabu_jsonl" || parserPayload.Status != "parsed" || parserPayload.FindingCount != 1 || parserPayload.ToolID != "naabu" {
		t.Fatalf("unexpected naabu parser payload: %#v", parserPayload)
	}
	payloadText := string(events[1].Payload)
	for _, leaked := range []string{"secret-token", "secret.example", "banner", "timestamp", "cdn", "error"} {
		if strings.Contains(payloadText, leaked) {
			t.Fatalf("naabu surface fact leaked untrusted output %q: %s", leaked, payloadText)
		}
	}
	var rawPayload map[string]json.RawMessage
	if err := json.Unmarshal(events[1].Payload, &rawPayload); err != nil {
		t.Fatalf("decode raw fact event: %v", err)
	}
	for _, forbiddenKey := range []string{"host", "timestamp", "cdn", "banner", "error", "raw", "request", "response"} {
		if _, ok := rawPayload[forbiddenKey]; ok {
			t.Fatalf("naabu surface fact persisted forbidden key %q: %s", forbiddenKey, payloadText)
		}
	}
	var payload struct {
		Source     string  `json:"source"`
		Kind       string  `json:"kind"`
		Statement  string  `json:"statement"`
		Confidence float64 `json:"confidence"`
		IP         string  `json:"ip"`
		Port       int     `json:"port"`
		Protocol   string  `json:"protocol"`
		Endpoint   string  `json:"endpoint"`
		TargetCIDR string  `json:"target_cidr"`
		ParserID   string  `json:"parser_id"`
	}
	if err := json.Unmarshal(events[1].Payload, &payload); err != nil {
		t.Fatalf("decode fact event: %v", err)
	}
	if payload.Source != "parser.naabu_jsonl" || payload.Kind != "surface" || payload.ParserID != "naabu_jsonl" {
		t.Fatalf("unexpected naabu surface identity: %#v", payload)
	}
	if payload.IP != "10.0.0.5" || payload.Port != 443 || payload.Protocol != "tcp" || payload.Endpoint != "10.0.0.5:443" || payload.TargetCIDR != "10.0.0.0/24" {
		t.Fatalf("unexpected structured naabu surface metadata: %#v", payload)
	}
	for _, want := range []string{"naabu", "10.0.0.5:443", "open"} {
		if !strings.Contains(payload.Statement, want) {
			t.Fatalf("expected naabu statement to contain %q, got %#v", want, payload)
		}
	}
	if payload.Confidence < 0.6 {
		t.Fatalf("expected useful naabu confidence, got %#v", payload)
	}
}

func TestParserEventsForToolExecutionRequiresNaabuCIDRScope(t *testing.T) {
	events := parserEventsForToolExecution("run-naabu-no-cidr", naabuParserRequest("naabu-no-cidr:01", []ScopeTarget{{Kind: "host", Value: "api.example.com"}}, nil), ToolExecutionResult{
		ExecutionID: "native-naabu",
		Status:      "completed",
		Transcript:  `{"ip":"10.0.0.5","port":443}`,
	})

	if len(events) != 1 || events[0].Type != eventlog.EventParser {
		t.Fatalf("expected one parser failure event without CIDR scope, got %#v", events)
	}
	var payload struct {
		Parser       string `json:"parser"`
		Status       string `json:"status"`
		FindingCount int    `json:"finding_count"`
	}
	if err := json.Unmarshal(events[0].Payload, &payload); err != nil {
		t.Fatalf("decode parser event: %v", err)
	}
	if payload.Parser != "naabu_jsonl" || payload.Status != "parse_failed" || payload.FindingCount != 0 {
		t.Fatalf("unexpected no-scope parse-failure payload: %#v", payload)
	}
}

func TestParserEventsForToolExecutionEnforcesNaabuCIDRScopeAndSkipsOutOfScopeIP(t *testing.T) {
	events := parserEventsForToolExecution("run-naabu-scope", naabuParserRequest("naabu-scope:01", []ScopeTarget{{Kind: "cidr", Value: "10.0.0.0/24"}}, []string{"10.0.0.6"}), ToolExecutionResult{
		ExecutionID: "native-naabu",
		Status:      "completed",
		Transcript: strings.Join([]string{
			`{"ip":"10.0.0.5","port":80}`,
			`{"ip":"10.0.1.9","port":443,"banner":"secret-token"}`,
			`{"ip":"10.0.0.6","port":8443,"banner":"secret-token"}`,
		}, "\n"),
	})

	if len(events) != 2 {
		t.Fatalf("expected parser event and one in-scope surface fact, got %#v", events)
	}
	var payload struct {
		IP       string `json:"ip"`
		Endpoint string `json:"endpoint"`
	}
	if err := json.Unmarshal(events[1].Payload, &payload); err != nil {
		t.Fatalf("decode fact event: %v", err)
	}
	if payload.IP != "10.0.0.5" || payload.Endpoint != "10.0.0.5:80" {
		t.Fatalf("expected only non-excluded in-scope IP to be promoted, got %#v", payload)
	}
	for _, leaked := range []string{"10.0.1.9", "10.0.0.6", "8443", "secret-token"} {
		if strings.Contains(string(events[1].Payload), leaked) {
			t.Fatalf("naabu surface fact leaked skipped output %q: %s", leaked, events[1].Payload)
		}
	}
}

func TestParserEventsForToolExecutionValidatesNaabuIPAndPort(t *testing.T) {
	events := parserEventsForToolExecution("run-naabu-validate", naabuParserRequest("naabu-validate:01", []ScopeTarget{{Kind: "cidr", Value: "2001:db8::/64"}}, nil), ToolExecutionResult{
		ExecutionID: "native-naabu",
		Status:      "completed",
		Transcript: strings.Join([]string{
			`{"ip":"not-an-ip","port":443}`,
			`{"ip":"2001:db8::10","port":0}`,
			`{"ip":"2001:db8::11","port":70000}`,
			`{"host":"2001:db8::12","port":"8443","protocol":"udp"}`,
		}, "\n"),
	})

	if len(events) != 2 {
		t.Fatalf("expected parser event and one valid IPv6 surface fact, got %#v", events)
	}
	var payload struct {
		IP       string `json:"ip"`
		Port     int    `json:"port"`
		Protocol string `json:"protocol"`
		Endpoint string `json:"endpoint"`
	}
	if err := json.Unmarshal(events[1].Payload, &payload); err != nil {
		t.Fatalf("decode fact event: %v", err)
	}
	if payload.IP != "2001:db8::12" || payload.Port != 8443 || payload.Protocol != "udp" || payload.Endpoint != "[2001:db8::12]:8443" {
		t.Fatalf("expected valid host IP/string port to be promoted, got %#v", payload)
	}
}

func TestParserEventsForToolExecutionRecordsNaabuParseFailureWithoutFact(t *testing.T) {
	events := parserEventsForToolExecution("run-naabu-parse-failure", naabuParserRequest("naabu-parse-failure:01", []ScopeTarget{{Kind: "cidr", Value: "10.0.0.0/24"}}, nil), ToolExecutionResult{
		ExecutionID: "native-naabu",
		Status:      "completed",
		Transcript:  "not-jsonl",
	})

	if len(events) != 1 || events[0].Type != eventlog.EventParser {
		t.Fatalf("expected one parse-failure event, got %#v", events)
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
	if payload.Parser != "naabu_jsonl" || payload.Status != "parse_failed" || payload.ToolID != "naabu" || payload.FindingCount != 0 || payload.Message == "" {
		t.Fatalf("unexpected parse-failure payload: %#v", payload)
	}
}

func naabuParserRequest(id string, targets []ScopeTarget, exclusions []string) ToolExecutionRequest {
	return ToolExecutionRequest{
		Node: compiler.PlanNode{
			ID:           id,
			Capability:   "infra.port_scan",
			ManifestName: "infra-pack",
			ToolID:       "naabu",
			Operation:    "port_scan",
			ParserID:     "naabu_jsonl",
		},
		ScopeTargets:      targets,
		ScopeExclusions:   exclusions,
		ScopeTargetPolicy: "enforce",
		IsolationMode:     "process",
	}
}
