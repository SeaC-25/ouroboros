package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRecordProviderGenerationUsageWritesMetadataOnlyRecord(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	now := time.Date(2026, 6, 4, 12, 30, 0, 0, time.UTC)
	record, err := RecordProviderGenerationUsage(root, ProviderGenerationResult{
		ProviderName:           "local-openai",
		Model:                  "gpt-test",
		Text:                   "do not persist generated text",
		InputTokens:            1000,
		OutputTokens:           500,
		TotalTokens:            1500,
		ContextWindowTokens:    128000,
		Capabilities:           []string{"json_mode", "tool_planning"},
		EstimatedCostUSD:       0.0075,
		EstimatedCostAvailable: true,
	}, ProviderUsageRecordOptions{Now: now})
	if err != nil {
		t.Fatalf("record provider generation usage: %v", err)
	}
	if record.SchemaVersion != 1 || record.Source != "provider-generate" || record.CreatedAt != "2026-06-04T12:30:00Z" {
		t.Fatalf("unexpected usage record metadata: %#v", record)
	}
	if record.ProviderName != "local-openai" || record.Model != "gpt-test" || record.InputTokens != 1000 || record.OutputTokens != 500 || record.TotalTokens != 1500 {
		t.Fatalf("unexpected usage record values: %#v", record)
	}
	if !record.EstimatedCostAvailable || record.EstimatedCostUSD != 0.0075 {
		t.Fatalf("unexpected usage record cost: %#v", record)
	}
	if !strings.HasPrefix(record.Path, filepath.Join(root, "runtime", "model-usage")+string(os.PathSeparator)) {
		t.Fatalf("expected record under runtime/model-usage, got %s", record.Path)
	}

	data, err := os.ReadFile(record.Path)
	if err != nil {
		t.Fatalf("read usage record: %v", err)
	}
	for _, leaked := range []string{"do not persist generated text", "Authorization", "Bearer"} {
		if strings.Contains(string(data), leaked) {
			t.Fatalf("usage record leaked sensitive content %q: %s", leaked, string(data))
		}
	}

	list, err := ListProviderUsageRecords(root)
	if err != nil {
		t.Fatalf("list usage records: %v", err)
	}
	if len(list) != 1 || list[0].Path != record.Path {
		t.Fatalf("unexpected usage list: %#v", list)
	}

	read, err := ReadProviderUsageRecord(root, filepath.Base(record.Path))
	if err != nil {
		t.Fatalf("read usage record by basename: %v", err)
	}
	if read.RecordID != record.RecordID || read.Path != record.Path {
		t.Fatalf("unexpected read record: %#v", read)
	}
}

func TestRecordProviderGenerationUsageRejectsSymlinkDirectory(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	outside := filepath.Join(t.TempDir(), "outside")
	if err := os.MkdirAll(outside, 0o755); err != nil {
		t.Fatalf("create outside dir: %v", err)
	}
	usageDir := filepath.Join(root, "runtime", "model-usage")
	if err := os.Symlink(outside, usageDir); err != nil {
		t.Fatalf("symlink usage dir: %v", err)
	}

	_, err := RecordProviderGenerationUsage(root, ProviderGenerationResult{
		ProviderName: "local-openai",
		Model:        "gpt-test",
		InputTokens:  1,
		OutputTokens: 1,
		TotalTokens:  2,
	}, ProviderUsageRecordOptions{})
	if err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("expected symlink usage directory rejection, got %v", err)
	}
	entries, err := os.ReadDir(outside)
	if err != nil {
		t.Fatalf("read outside dir: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected no writes through symlinked usage dir, got %#v", entries)
	}
}
