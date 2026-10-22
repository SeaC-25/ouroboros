package nativetools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestScanStaticCodeSemgrepEmitsSARIFSubset(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "config.py"), []byte("API_KEY = \"AKIAIOSFODNN7EXAMPLE\"\npassword = \"hunter22secret\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "clean.py"), []byte("value = compute(answer)\nprint(value)\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	out, err := ScanStaticCodeSemgrep(context.Background(), dir, Options{})
	if err != nil {
		t.Fatalf("ScanStaticCodeSemgrep: %v", err)
	}

	var log sarifLog
	if err := json.Unmarshal([]byte(out), &log); err != nil {
		t.Fatalf("decode SARIF: %v (%s)", err, out)
	}
	if log.Version != "2.1.0" {
		t.Fatalf("version = %q, want 2.1.0", log.Version)
	}
	if len(log.Runs) != 1 || log.Runs[0].Tool.Driver.Name != "Semgrep" {
		t.Fatalf("unexpected tool driver: %#v", log.Runs)
	}

	results := log.Runs[0].Results
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d: %s", len(results), out)
	}
	byRule := map[string]sarifResult{}
	for _, result := range results {
		byRule[result.RuleID] = result
	}
	aws, ok := byRule["aws-access-key-literal"]
	if !ok {
		t.Fatalf("aws rule not detected: %#v", byRule)
	}
	if aws.Level != "error" {
		t.Fatalf("aws level = %q, want error", aws.Level)
	}
	if got := aws.Locations[0].PhysicalLocation.ArtifactLocation.URI; got != "config.py" {
		t.Fatalf("aws uri = %q, want relative config.py", got)
	}
	if got := aws.Locations[0].PhysicalLocation.Region.StartLine; got != 1 {
		t.Fatalf("aws startLine = %d, want 1", got)
	}
	if got := aws.Message.Text; got == "" {
		t.Fatal("aws result missing message text")
	}
	if _, ok := byRule["hardcoded-password"]; !ok {
		t.Fatalf("hardcoded password rule not detected: %#v", byRule)
	}
	for _, result := range results {
		if strings.Contains(result.Locations[0].PhysicalLocation.ArtifactLocation.URI, "clean.py") {
			t.Fatalf("false positive on clean file: %#v", result)
		}
	}

	ruleIDs := map[string]bool{}
	for _, rule := range log.Runs[0].Tool.Driver.Rules {
		ruleIDs[rule.ID] = true
		if rule.ShortDescription.Text == "" || rule.FullDescription.Text == "" {
			t.Fatalf("rule %s missing descriptions: %#v", rule.ID, rule)
		}
		if rule.Properties.Precision == "" {
			t.Fatalf("rule %s missing precision property", rule.ID)
		}
	}
	if !ruleIDs["aws-access-key-literal"] {
		t.Fatalf("driver rules missing aws rule: %#v", ruleIDs)
	}
}

func TestScanStaticCodeSemgrepSkipsBinaryFiles(t *testing.T) {
	dir := t.TempDir()
	blob := append([]byte{0x00}, []byte("AKIAIOSFODNN7EXAMPLE")...)
	if err := os.WriteFile(filepath.Join(dir, "blob.bin"), blob, 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := ScanStaticCodeSemgrep(context.Background(), dir, Options{})
	if err != nil {
		t.Fatalf("ScanStaticCodeSemgrep: %v", err)
	}
	if strings.Contains(out, "blob.bin") {
		t.Fatalf("binary content must be skipped: %s", out)
	}
	var log sarifLog
	if err := json.Unmarshal([]byte(out), &log); err != nil {
		t.Fatalf("decode SARIF: %v (%s)", err, out)
	}
	if len(log.Runs[0].Results) != 0 {
		t.Fatalf("expected zero results for binary-only target: %s", out)
	}
}

func TestScanStaticCodeSemgrepRequiresTarget(t *testing.T) {
	if _, err := ScanStaticCodeSemgrep(context.Background(), "   ", Options{}); err == nil {
		t.Fatal("expected error for empty target")
	}
}
