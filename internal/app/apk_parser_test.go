package app

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/ouroboros/ouroboros/internal/compiler"
	"github.com/ouroboros/ouroboros/internal/eventlog"
)

func apktoolRequest() ToolExecutionRequest {
	return ToolExecutionRequest{
		Node: compiler.PlanNode{
			ID:           "apktool-node:01",
			Capability:   "ctf.artifact_analysis",
			ManifestName: "ctf-pack",
			ToolID:       "apktool",
			Operation:    "apk_decompile",
			ParserID:     "apktool_text",
		},
	}
}

func TestParserEventsForToolExecutionParsesApktoolSummary(t *testing.T) {
	events := parserEventsForToolExecution("run-apktool", apktoolRequest(), ToolExecutionResult{
		ExecutionID: "native-apktool",
		Status:      "completed",
		Transcript: strings.Join([]string{
			"apktool | package=com.example.app",
			"apktool | versionName=1.0 versionCode=7",
			"apktool | sdk min=21 target=30",
			"apktool | uses-permission android.permission.INTERNET",
			"apktool | activity .MainActivity",
			"apktool | exported .MainActivity",
			"apktool | dex files=1 methods=42",
		}, "\n"),
	})

	if len(events) != 8 {
		t.Fatalf("expected 8 events (1 status + 7 facts), got %d: %#v", len(events), events)
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
	if status.Parser != "apktool_text" || status.Status != "parsed" || status.FindingCount != 7 {
		t.Fatalf("unexpected status: %#v", status)
	}

	var artifact map[string]any
	for _, event := range events[1:] {
		if event.Type != eventlog.EventFact {
			t.Fatalf("expected fact events, got %#v", event)
		}
		if strings.Contains(string(event.Payload), "package=com.example.app") {
			if err := json.Unmarshal(event.Payload, &artifact); err != nil {
				t.Fatalf("decode artifact fact: %v", err)
			}
		}
	}
	if artifact == nil {
		t.Fatalf("expected a package artifact fact, got %#v", events[1:])
	}
	if artifact["apktool_package"] != "com.example.app" {
		t.Fatalf("unexpected package payload: %#v", artifact)
	}
	if artifact["kind"] != "artifact" || artifact["parser_id"] != "apktool_text" {
		t.Fatalf("unexpected fact kind/parser: %#v", artifact)
	}
}

func TestParserEventsForToolExecutionRejectsApktoolWrongTool(t *testing.T) {
	request := apktoolRequest()
	request.Node.ToolID = "jadx"
	events := parserEventsForToolExecution("run-apktool-wrong", request, ToolExecutionResult{
		ExecutionID: "native-apktool",
		Status:      "completed",
		Transcript:  "apktool | package=com.example.app",
	})

	if len(events) != 1 || events[0].Type != eventlog.EventParser {
		t.Fatalf("expected one parse-failure event, got %#v", events)
	}
	var status struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(events[0].Payload, &status); err != nil {
		t.Fatalf("decode status: %v", err)
	}
	if status.Status != "parse_failed" {
		t.Fatalf("expected parse_failed, got %#v", status)
	}
}

func jadxRequest() ToolExecutionRequest {
	return ToolExecutionRequest{
		Node: compiler.PlanNode{
			ID:           "jadx-node:01",
			Capability:   "ctf.artifact_analysis",
			ManifestName: "ctf-pack",
			ToolID:       "jadx",
			Operation:    "apk_decompile",
			ParserID:     "jadx_metadata_json",
		},
	}
}

func TestParserEventsForToolExecutionParsesJadxMetadata(t *testing.T) {
	events := parserEventsForToolExecution("run-jadx", jadxRequest(), ToolExecutionResult{
		ExecutionID: "native-jadx",
		Status:      "completed",
		Transcript: `{
			"package": "com.example.app",
			"version_name": "1.0",
			"version_code": "7",
			"min_sdk": 21,
			"target_sdk": 30,
			"activities": [".MainActivity"],
			"services": [],
			"receivers": [],
			"permissions": ["android.permission.INTERNET"],
			"exported_components": [".MainActivity"],
			"dex_files": ["classes.dex"],
			"source_files": [],
			"entry_count": 1,
			"total_dex_methods": 42
		}`,
	})

	// 1 status + 1 artifact + 1 activity + 1 permission + 1 exported = 5.
	if len(events) != 5 {
		t.Fatalf("expected 5 events, got %d: %#v", len(events), events)
	}
	var artifact map[string]any
	var surfaces []map[string]any
	for _, event := range events[1:] {
		var payload map[string]any
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			t.Fatalf("decode fact: %v", err)
		}
		if payload["kind"] == "artifact" {
			artifact = payload
		} else {
			surfaces = append(surfaces, payload)
		}
	}
	if artifact == nil {
		t.Fatalf("expected an artifact fact, got %#v", events)
	}
	if artifact["package"] != "com.example.app" || artifact["total_dex_methods"] != float64(42) {
		t.Fatalf("unexpected artifact fact: %#v", artifact)
	}
	if len(surfaces) != 3 {
		t.Fatalf("expected 3 surface facts, got %#v", surfaces)
	}
	seen := map[string]bool{}
	for _, surface := range surfaces {
		seen[surface["component_type"].(string)+"|"+surface["component_name"].(string)] = true
		if surface["kind"] != "surface" {
			t.Fatalf("surface fact has wrong kind: %#v", surface)
		}
	}
	for _, want := range []string{"activity|.MainActivity", "permission|android.permission.INTERNET", "exported|.MainActivity"} {
		if !seen[want] {
			t.Fatalf("expected surface %s, got %#v", want, seen)
		}
	}
}

func TestParserEventsForToolExecutionRejectsJadxGarbage(t *testing.T) {
	events := parserEventsForToolExecution("run-jadx-garbage", jadxRequest(), ToolExecutionResult{
		ExecutionID: "native-jadx",
		Status:      "completed",
		Transcript:  "not json at all",
	})
	if len(events) != 1 || events[0].Type != eventlog.EventParser {
		t.Fatalf("expected one parse-failure event, got %#v", events)
	}
}
