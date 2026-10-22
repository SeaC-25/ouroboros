package app

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/ouroboros/ouroboros/internal/compiler"
	"github.com/ouroboros/ouroboros/internal/eventlog"
)

func hydraRequest() ToolExecutionRequest {
	return ToolExecutionRequest{
		Node: compiler.PlanNode{
			ID:           "hydra-node:01",
			Capability:   "infra.identity_graph",
			ManifestName: "infra-pack",
			ToolID:       "hydra",
			Operation:    "auth_test",
			ParserID:     "auth_test_jsonl",
		},
	}
}

func hydraTranscript(lines ...string) string {
	return strings.Join(lines, "\n")
}

func TestParserEventsForToolExecutionParsesHydraCredentialHits(t *testing.T) {
	events := parserEventsForToolExecution("run-hydra", hydraRequest(), ToolExecutionResult{
		ExecutionID: "native-hydra",
		Status:      "completed",
		Transcript: hydraTranscript(
			`{"username":"admin","password":"s3cret","place":"http-basic","success":true,"attempts":2}`,
			`{"username":"root","password":"letmein","place":"form","success":false,"attempts":3}`,
			`{"username":"ops","password":"hunter2","place":"form","success":true,"attempts":7}`,
		),
	})

	if len(events) != 3 {
		t.Fatalf("expected 3 events (1 status + 2 hit facts), got %d: %#v", len(events), events)
	}
	if events[0].Type != eventlog.EventParser {
		t.Fatalf("expected a leading parser status event, got %#v", events[0])
	}
	var status struct {
		Parser       string `json:"parser"`
		Status       string `json:"status"`
		FindingCount int    `json:"finding_count"`
	}
	if err := json.Unmarshal(events[0].Payload, &status); err != nil {
		t.Fatalf("decode status: %v", err)
	}
	if status.Parser != "auth_test_jsonl" || status.Status != "parsed" || status.FindingCount != 2 {
		t.Fatalf("unexpected status: %#v", status)
	}

	var first map[string]any
	if events[1].Type != eventlog.EventFact {
		t.Fatalf("expected fact event, got %#v", events[1])
	}
	if err := json.Unmarshal(events[1].Payload, &first); err != nil {
		t.Fatalf("decode fact: %v", err)
	}
	if first["kind"] != "finding" || first["parser_id"] != "auth_test_jsonl" {
		t.Fatalf("unexpected fact kind/parser: %#v", first)
	}
	if first["username"] != "admin" {
		t.Fatalf("expected admin username, got %#v", first)
	}
	if first["place"] != "http-basic" {
		t.Fatalf("expected http-basic place, got %#v", first)
	}
	if first["attempts"] != float64(2) {
		t.Fatalf("expected attempts 2, got %#v", first)
	}
	statement, _ := first["statement"].(string)
	if !strings.Contains(statement, "user=admin") || !strings.Contains(statement, "pass=") {
		t.Fatalf("expected statement to carry user and masked pass, got %q", statement)
	}
	if strings.Contains(statement, "s3cret") || strings.Contains(string(events[1].Payload), `"s3cret"`) {
		t.Fatalf("expected plaintext password to be masked, got %q", string(events[1].Payload))
	}
	if !strings.Contains(statement, "s3") || !strings.Contains(statement, "et") {
		t.Fatalf("expected masked password to keep first/last two chars, got %q", statement)
	}
	if strings.Count(statement, "·") != 2 {
		t.Fatalf("expected two mask dots for a 6-char password, got %q", statement)
	}
}

func TestParserEventsForToolExecutionMasksLongPasswords(t *testing.T) {
	events := parserEventsForToolExecution("run-hydra-mask", hydraRequest(), ToolExecutionResult{
		ExecutionID: "native-hydra",
		Status:      "completed",
		Transcript:  `{"username":"admin","password":"sup3rl0ngp4ssw0rd","place":"form","success":true,"attempts":5}`,
	})

	if len(events) != 2 {
		t.Fatalf("expected 2 events (1 status + 1 fact), got %d: %#v", len(events), events)
	}
	var fact map[string]any
	if err := json.Unmarshal(events[1].Payload, &fact); err != nil {
		t.Fatalf("decode fact: %v", err)
	}
	if fact["password"] != "su·············rd" {
		t.Fatalf("expected long password masked to first/last two, got %#v", fact["password"])
	}
	payload := string(events[1].Payload)
	if strings.Contains(payload, "sup3rl0ngp4ssw0rd") {
		t.Fatalf("plaintext long password leaked into payload: %s", payload)
	}
}

func TestParserEventsForToolExecutionRejectsShortPasswordMask(t *testing.T) {
	events := parserEventsForToolExecution("run-hydra-short", hydraRequest(), ToolExecutionResult{
		ExecutionID: "native-hydra",
		Status:      "completed",
		Transcript:  `{"username":"admin","password":"ab","place":"form","success":true,"attempts":1}`,
	})
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d: %#v", len(events), events)
	}
	var fact map[string]any
	if err := json.Unmarshal(events[1].Payload, &fact); err != nil {
		t.Fatalf("decode fact: %v", err)
	}
	if fact["password"] != "··" {
		t.Fatalf("expected short password fully masked, got %#v", fact["password"])
	}
}

func TestParserEventsForToolExecutionSkipsHydraFailuresWithoutFacts(t *testing.T) {
	// All-failure output produces one parse_failed status event and no facts.
	events := parserEventsForToolExecution("run-hydra-all-fail", hydraRequest(), ToolExecutionResult{
		ExecutionID: "native-hydra",
		Status:      "completed",
		Transcript: hydraTranscript(
			`{"username":"admin","password":"letmein","place":"form","success":false,"attempts":1}`,
			`{"username":"root","password":"qwerty","place":"http-basic","success":false,"attempts":2}`,
		),
	})
	if len(events) != 1 {
		t.Fatalf("expected exactly one parse_failed event, got %#v", events)
	}
	if events[0].Type != eventlog.EventParser {
		t.Fatalf("expected parser event, got %#v", events[0])
	}
	var status struct {
		Status       string `json:"status"`
		FindingCount int    `json:"finding_count"`
	}
	if err := json.Unmarshal(events[0].Payload, &status); err != nil {
		t.Fatalf("decode status: %v", err)
	}
	if status.Status != "parse_failed" {
		t.Fatalf("expected parse_failed, got %q", status.Status)
	}
	if status.FindingCount != 0 {
		t.Fatalf("expected finding_count 0, got %d", status.FindingCount)
	}
	if strings.Contains(string(events[0].Payload), "fact_id") {
		t.Fatalf("failures must not emit facts, got %s", string(events[0].Payload))
	}
}

func TestParserEventsForToolExecutionRecordsHydraParseFailureWithoutFact(t *testing.T) {
	events := parserEventsForToolExecution("run-hydra-parse-failure", hydraRequest(), ToolExecutionResult{
		ExecutionID: "native-hydra",
		Status:      "completed",
		Transcript:  "not-jsonl",
	})
	if len(events) != 1 {
		t.Fatalf("expected a single parse_failed event, got %#v", events)
	}
	if events[0].Type != eventlog.EventParser {
		t.Fatalf("expected parser event, got %#v", events[0])
	}
	var status struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(events[0].Payload, &status); err != nil {
		t.Fatalf("decode status: %v", err)
	}
	if status.Status != "parse_failed" {
		t.Fatalf("expected parse_failed, got %q", status.Status)
	}
	if strings.Contains(string(events[0].Payload), "fact_id") {
		t.Fatalf("parse failure must not emit facts, got %s", string(events[0].Payload))
	}
}

func TestParserEventsForToolExecutionRejectsHydraWrongTool(t *testing.T) {
	request := hydraRequest()
	request.Node.ToolID = "medusa"
	events := parserEventsForToolExecution("run-hydra-wrong-tool", request, ToolExecutionResult{
		ExecutionID: "native-hydra",
		Status:      "completed",
		Transcript:  `{"username":"admin","password":"s3cret","place":"form","success":true,"attempts":1}`,
	})
	if len(events) != 1 {
		t.Fatalf("expected a single parse_failed event, got %#v", events)
	}
	var status struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(events[0].Payload, &status); err != nil {
		t.Fatalf("decode status: %v", err)
	}
	if status.Status != "parse_failed" {
		t.Fatalf("expected parse_failed for non-hydra tool, got %q", status.Status)
	}
}

func TestParserEventsForToolExecutionRejectsHydraWrongOperation(t *testing.T) {
	request := hydraRequest()
	request.Node.Operation = "http_probe"
	events := parserEventsForToolExecution("run-hydra-wrong-op", request, ToolExecutionResult{
		ExecutionID: "native-hydra",
		Status:      "completed",
		Transcript:  `{"username":"admin","password":"s3cret","place":"form","success":true,"attempts":1}`,
	})
	if len(events) != 1 {
		t.Fatalf("expected a single parse_failed event, got %#v", events)
	}
	var status struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(events[0].Payload, &status); err != nil {
		t.Fatalf("decode status: %v", err)
	}
	if status.Status != "parse_failed" {
		t.Fatalf("expected parse_failed for non-auth_test operation, got %q", status.Status)
	}
}

func TestParserEventsForToolExecutionCapsRawHydraJSONLOutput(t *testing.T) {
	events := parserEventsForToolExecution("run-hydra-oversized", hydraRequest(), ToolExecutionResult{
		ExecutionID: "native-hydra",
		Status:      "completed",
		Transcript:  strings.Repeat(" ", (1<<20)+1) + `{"username":"admin","password":"s3cret","place":"form","success":true,"attempts":1}`,
	})
	if len(events) != 1 {
		t.Fatalf("expected a single parse_failed event for oversized output, got %#v", events)
	}
	var status struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(events[0].Payload, &status); err != nil {
		t.Fatalf("decode status: %v", err)
	}
	if status.Status != "parse_failed" {
		t.Fatalf("expected parse_failed, got %q", status.Status)
	}
}
