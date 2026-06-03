package app

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/ouroboros/ouroboros/internal/compiler"
	"github.com/ouroboros/ouroboros/internal/eventlog"
)

func TestParserEventsForToolExecutionRecordsMobsfscanParseFailureWithoutFact(t *testing.T) {
	events := parserEventsForToolExecution("run-mobsfscan-parse-failure", ToolExecutionRequest{
		Node: compiler.PlanNode{
			ID:           "mobsfscan-parse-failure:01",
			Capability:   "ctf.artifact_analysis",
			ManifestName: "ctf-pack",
			ToolID:       "mobsfscan",
			Operation:    "static_code_scan",
			ParserID:     "mobsfscan_json",
		},
	}, ToolExecutionResult{
		ExecutionID: "native-mobsfscan",
		Status:      "completed",
		Transcript:  "not-json",
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
		ExecutionID  string `json:"execution_id"`
		FindingCount int    `json:"finding_count"`
		Message      string `json:"message"`
	}
	if err := json.Unmarshal(events[0].Payload, &payload); err != nil {
		t.Fatalf("decode parser event: %v", err)
	}
	if payload.Parser != "mobsfscan_json" || payload.ParserID != "mobsfscan_json" || payload.Status != "parse_failed" || payload.ToolID != "mobsfscan" || payload.ExecutionID != "native-mobsfscan" || payload.FindingCount != 0 {
		t.Fatalf("unexpected parse-failure payload: %#v", payload)
	}
	if payload.Message == "" {
		t.Fatalf("expected parse failure message")
	}
}

func TestParserEventsForToolExecutionScopesMobsfscanReportedFilePath(t *testing.T) {
	events := parserEventsForToolExecution("run-mobsfscan-path-scope", ToolExecutionRequest{
		Node: compiler.PlanNode{
			ID:           "mobsfscan-path-scope:01",
			Capability:   "ctf.artifact_analysis",
			ManifestName: "ctf-pack",
			ToolID:       "mobsfscan",
			Operation:    "static_code_scan",
			ParserID:     "mobsfscan_json",
		},
		ScopeTargets: []ScopeTarget{{Kind: "artifact", Value: "samples/App.java"}},
	}, ToolExecutionResult{
		ExecutionID: "native-mobsfscan",
		Status:      "completed",
		Transcript: `{
  "results": {
    "path_leak": {
      "metadata": {
        "description": "Unsafe path should not leak",
        "severity": "HIGH",
        "cwe": "CWE-22"
      },
      "files": [
        {
          "file_path": "/etc/passwd",
          "match_lines": [7, 7]
        }
      ]
    }
  }
}`,
	})

	if len(events) != 2 {
		t.Fatalf("expected parser event and one fact event, got %#v", events)
	}
	var payload struct {
		Source         string `json:"source"`
		Statement      string `json:"statement"`
		FilePath       string `json:"file_path"`
		ArtifactTarget string `json:"artifact_target"`
	}
	if err := json.Unmarshal(events[1].Payload, &payload); err != nil {
		t.Fatalf("decode fact event: %v", err)
	}
	if payload.Source != "parser.mobsfscan_json" {
		t.Fatalf("expected parser source fact, got %#v", payload)
	}
	if payload.FilePath != "samples/App.java" || payload.ArtifactTarget != "samples/App.java" {
		t.Fatalf("expected unsafe reported path to collapse to scoped artifact target, got %#v", payload)
	}
	if strings.Contains(payload.Statement, "/etc/passwd") || strings.Contains(payload.Statement, "..") {
		t.Fatalf("statement leaked unsafe reported path: %#v", payload)
	}
}

func TestMobsfscanCleanRelativeArtifactPathRejectsUnsafeSegments(t *testing.T) {
	tests := []struct {
		value string
		ok    bool
		want  string
	}{
		{value: "samples/App.java", ok: true, want: "samples/App.java"},
		{value: "samples//App.java", ok: true, want: "samples/App.java"},
		{value: "/etc/passwd"},
		{value: "../../outside"},
		{value: "samples/../outside"},
		{value: `C:\Windows\win.ini`},
	}
	for _, tt := range tests {
		got, ok := mobsfscanCleanRelativeArtifactPath(tt.value)
		if ok != tt.ok || got != tt.want {
			t.Fatalf("mobsfscanCleanRelativeArtifactPath(%q) = %q, %v; want %q, %v", tt.value, got, ok, tt.want, tt.ok)
		}
	}
}
