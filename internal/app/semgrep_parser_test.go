package app

import (
	"encoding/json"
	"testing"

	"github.com/ouroboros/ouroboros/internal/compiler"
	"github.com/ouroboros/ouroboros/internal/eventlog"
)

const semgrepTestSARIF = `{
  "$schema": "https://json.schemastore.org/sarif-2.1.0.json",
  "version": "2.1.0",
  "runs": [
    {
      "tool": {
        "driver": {
          "name": "Semgrep",
          "rules": [
            {
              "id": "aws-access-key-literal",
              "shortDescription": {"text": "Hardcoded AWS access key"},
              "properties": {"precision": "medium", "cwe": "CWE-798"}
            }
          ]
        }
      },
      "results": [
        {
          "ruleId": "aws-access-key-literal",
          "level": "error",
          "message": {"text": "Hardcoded AWS access key"},
          "locations": [
            {
              "physicalLocation": {
                "artifactLocation": {"uri": "config.py"},
                "region": {"startLine": 3}
              }
            }
          ]
        },
        {
          "ruleId": "tls-verification-disabled",
          "level": "warning",
          "message": {"text": "TLS certificate verification disabled"},
          "locations": [
            {
              "physicalLocation": {
                "artifactLocation": {"uri": "client.py"},
                "region": {"startLine": 9}
              }
            }
          ]
        }
      ]
    }
  ]
}`

func TestSemgrepParserEventsEmitFindingFacts(t *testing.T) {
	events, err := semgrepParserEvents("run-semgrep-parse", ToolExecutionRequest{
		Node: compiler.PlanNode{
			ID:           "semgrep-parse:01",
			Capability:   "ctf.artifact_analysis",
			ManifestName: "ctf-pack",
			ToolID:       "semgrep",
			Operation:    "static_code_scan",
			ParserID:     "semgrep_sarif",
		},
		ScopeTargets: []ScopeTarget{{Kind: "artifact", Value: "config.py"}},
	}, ToolExecutionResult{
		ExecutionID: "native-semgrep",
		Status:      "completed",
		Transcript:  semgrepTestSARIF,
	})
	if err != nil {
		t.Fatalf("semgrepParserEvents: %v", err)
	}
	if len(events) != 3 {
		t.Fatalf("expected parser event and 2 fact events, got %#v", events)
	}
	if events[0].Type != eventlog.EventParser {
		t.Fatalf("expected parser event, got %#v", events[0])
	}
	var status struct {
		Parser       string `json:"parser"`
		Status       string `json:"status"`
		FindingCount int    `json:"finding_count"`
	}
	if err := json.Unmarshal(events[0].Payload, &status); err != nil {
		t.Fatalf("decode parser event: %v", err)
	}
	if status.Parser != "semgrep_sarif" || status.Status != "parsed" || status.FindingCount != 2 {
		t.Fatalf("unexpected parser payload: %#v", status)
	}

	var first struct {
		FactID     string  `json:"fact_id"`
		Kind       string  `json:"kind"`
		Statement  string  `json:"statement"`
		Confidence float64 `json:"confidence"`
		Source     string  `json:"source"`
		ParserID   string  `json:"parser_id"`
		ToolID     string  `json:"tool_id"`
		Operation  string  `json:"operation"`
		RuleID     string  `json:"rule_id"`
		Severity   string  `json:"severity"`
		CWE        string  `json:"cwe"`
		FilePath   string  `json:"file_path"`
		LineStart  int     `json:"line_start"`
		LineEnd    int     `json:"line_end"`
	}
	if err := json.Unmarshal(events[1].Payload, &first); err != nil {
		t.Fatalf("decode fact event: %v", err)
	}
	if first.Kind != "finding" || first.RuleID != "aws-access-key-literal" || first.Severity != "error" {
		t.Fatalf("unexpected fact identity: %#v", first)
	}
	if first.CWE != "CWE-798" {
		t.Fatalf("expected CWE from driver rules, got %q", first.CWE)
	}
	if first.FilePath != "config.py" || first.LineStart != 3 || first.LineEnd != 3 {
		t.Fatalf("unexpected location: %#v", first)
	}
	if first.Statement == "" || first.Confidence != 0.9 || first.Source != "parser.semgrep_sarif" || first.ParserID != "semgrep_sarif" {
		t.Fatalf("unexpected fact metadata: %#v", first)
	}
	if first.ToolID != "semgrep" || first.Operation != "static_code_scan" {
		t.Fatalf("unexpected fact tool binding: %#v", first)
	}

	var second struct {
		RuleID   string `json:"rule_id"`
		Severity string `json:"severity"`
		CWE      string `json:"cwe"`
	}
	if err := json.Unmarshal(events[2].Payload, &second); err != nil {
		t.Fatalf("decode second fact: %v", err)
	}
	if second.RuleID != "tls-verification-disabled" || second.Severity != "warning" {
		t.Fatalf("unexpected second fact: %#v", second)
	}
	if second.CWE != "" {
		t.Fatalf("rule without cwe property should yield empty cwe, got %q", second.CWE)
	}
}

func TestSemgrepParserEventsRejectsForeignTool(t *testing.T) {
	events, err := semgrepParserEvents("run-semgrep-foreign", ToolExecutionRequest{
		Node: compiler.PlanNode{
			ID:        "mobsfscan-foreign:01",
			ToolID:    "mobsfscan",
			Operation: "static_code_scan",
			ParserID:  "semgrep_sarif",
		},
	}, ToolExecutionResult{
		ExecutionID: "native-mobsfscan",
		Status:      "completed",
		Transcript:  semgrepTestSARIF,
	})
	if err == nil {
		t.Fatalf("expected tool mismatch error, got events %#v", events)
	}
	if len(events) != 0 {
		t.Fatalf("expected no events on error, got %#v", events)
	}
}

func TestParserEventsForToolExecutionRecordsSemgrepParseFailure(t *testing.T) {
	events := parserEventsForToolExecution("run-semgrep-parse-failure", ToolExecutionRequest{
		Node: compiler.PlanNode{
			ID:           "semgrep-parse-failure:01",
			Capability:   "ctf.artifact_analysis",
			ManifestName: "ctf-pack",
			ToolID:       "semgrep",
			Operation:    "static_code_scan",
			ParserID:     "semgrep_sarif",
		},
	}, ToolExecutionResult{
		ExecutionID: "native-semgrep",
		Status:      "completed",
		Transcript:  "not-sarif",
	})

	if len(events) != 1 {
		t.Fatalf("expected one parse-failure event, got %#v", events)
	}
	if events[0].Type != eventlog.EventParser {
		t.Fatalf("expected parser event, got %#v", events[0])
	}
	var payload struct {
		Parser       string `json:"parser"`
		ParserID     string `json:"parser_id"`
		Status       string `json:"status"`
		ToolID       string `json:"tool_id"`
		FindingCount int    `json:"finding_count"`
		Message      string `json:"message"`
	}
	if err := json.Unmarshal(events[0].Payload, &payload); err != nil {
		t.Fatalf("decode parser event: %v", err)
	}
	if payload.Parser != "semgrep_sarif" || payload.ParserID != "semgrep_sarif" || payload.Status != "parse_failed" || payload.ToolID != "semgrep" || payload.FindingCount != 0 {
		t.Fatalf("unexpected parse-failure payload: %#v", payload)
	}
	if payload.Message == "" {
		t.Fatal("expected parse failure message")
	}
}
