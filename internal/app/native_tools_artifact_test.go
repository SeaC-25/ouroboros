package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ouroboros/ouroboros/internal/compiler"
)

// runNativeToolForTest drives a native_go node end to end and returns the
// resulting parser events.
func runNativeToolForTest(t *testing.T, operation, parserID, targetKind, targetValue string) []string {
	t.Helper()
	request := ToolExecutionRequest{
		RunID:         "run-native",
		WorkspaceRoot: t.TempDir(),
		Node: compiler.PlanNode{
			ID:         "node-native-" + operation,
			ToolID:     "native",
			AdapterID:  nativeGoAdapterID,
			Operation:  operation,
			ParserID:   parserID,
			Runner:     "native",
			TargetKind: targetKind,
		},
		ScopeTargets: []ScopeTarget{{Kind: targetKind, Value: targetValue}},
	}
	result, runnable, err := executeNativeGoTool(context.Background(), request)
	if err != nil || !runnable {
		t.Fatalf("executeNativeGoTool(%s): runnable=%v err=%v", operation, runnable, err)
	}
	if result.Status != "completed" {
		t.Fatalf("status = %q transcript = %s", result.Status, result.Transcript)
	}
	events := parserEventsForToolExecution("run-native", request, result)
	statements := make([]string, 0, len(events))
	for _, event := range events {
		statements = append(statements, string(event.Payload))
	}
	if len(statements) == 0 {
		t.Fatalf("no parser events for %s (transcript: %s)", operation, result.Transcript)
	}
	return statements
}

func TestNativeMetadataExtractReachesParser(t *testing.T) {
	dir := t.TempDir()
	// Minimal ZIP: exercises the archive branch without external tooling.
	zipPath := filepath.Join(dir, "archive.zip")
	if err := os.WriteFile(zipPath, []byte("PK\x03\x04placeholder"), 0o644); err != nil {
		t.Fatal(err)
	}
	pngPath := filepath.Join(dir, "image.png")
	pngHeader := []byte("\x89PNG\r\n\x1a\n")
	ihdr := make([]byte, 25)
	copy(ihdr[4:8], "IHDR")
	ihdr[8], ihdr[9], ihdr[10], ihdr[11] = 0, 0, 0, 16
	ihdr[12], ihdr[13], ihdr[14], ihdr[15] = 0, 0, 0, 8
	ihdr[16], ihdr[17] = 8, 6
	if err := os.WriteFile(pngPath, append(pngHeader, ihdr...), 0o644); err != nil {
		t.Fatal(err)
	}

	statements := runNativeToolForTest(t, "metadata_extract", "exiftool_json", "artifact", dir)
	joined := strings.Join(statements, "\n")
	if !strings.Contains(joined, "artifact_metadata") {
		t.Fatalf("no artifact_metadata fact: %s", joined)
	}
	if !strings.Contains(joined, "image.png") {
		t.Fatalf("png path missing from facts: %s", joined)
	}
	if !strings.Contains(joined, "ImageWidth") {
		t.Fatalf("png IHDR tag missing: %s", joined)
	}
}

func TestNativeSecretScanReachesParser(t *testing.T) {
	dir := t.TempDir()
	leaky := filepath.Join(dir, "config.env")
	content := "AWS_ACCESS_KEY_ID=AKIAIOSFODNN7EXAMPLE\n" + "clean_line=hello\n"
	if err := os.WriteFile(leaky, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(dir, "blob.bin")
	if err := os.WriteFile(binary, []byte{0x00, 0xFF, 0x10}, 0o644); err != nil {
		t.Fatal(err)
	}

	statements := runNativeToolForTest(t, "secret_scan", "gitleaks_json", "artifact", dir)
	joined := strings.Join(statements, "\n")
	if !strings.Contains(joined, `"kind":"secret"`) {
		t.Fatalf("no secret fact: %s", joined)
	}
	if !strings.Contains(joined, "config.env") || !strings.Contains(joined, "aws-access-token") {
		t.Fatalf("rule/file missing from facts: %s", joined)
	}
	if strings.Contains(joined, "blob.bin") {
		t.Fatalf("binary file must be skipped: %s", joined)
	}
}

func TestExiftoolParserRejectsEmptyOutput(t *testing.T) {
	request := ToolExecutionRequest{Node: compiler.PlanNode{ID: "n", ParserID: "exiftool_json"}}
	if _, err := exiftoolParserEvents("run", request, ToolExecutionResult{Status: "completed", Transcript: "  "}); err == nil {
		t.Fatal("expected error for empty exiftool output")
	}
}

func TestGitleaksParserAcceptsUpstreamFieldNames(t *testing.T) {
	request := ToolExecutionRequest{Node: compiler.PlanNode{ID: "n", ParserID: "gitleaks_json"}}
	payload, _ := json.Marshal([]map[string]any{{
		"RuleID": "aws-access-token", "File": "a.env", "StartLine": 3, "Secret": "AKIA", "Match": "AWS=AKIA",
	}})
	events, err := gitleaksParserEvents("run", request, ToolExecutionResult{Status: "completed", Transcript: string(payload)})
	if err != nil {
		t.Fatalf("gitleaksParserEvents: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("expected status + finding events, got %d", len(events))
	}
	if !strings.Contains(string(events[1].Payload), `"rule_id":"aws-access-token"`) {
		t.Fatalf("rule id missing: %s", events[1].Payload)
	}
}
