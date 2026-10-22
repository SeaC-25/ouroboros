package nativetools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestScanSecretsDetectsEmbeddedRules(t *testing.T) {
	dir := t.TempDir()
	cases := map[string]string{
		"aws.env":    "AWS_ACCESS_KEY_ID=AKIAIOSFODNN7EXAMPLE\n",
		"github.txt": "token: ghp_" + strings.Repeat("a", 36) + "\n",
		"key.pem":    "-----BEGIN RSA PRIVATE KEY-----\nMIIE\n",
		"clean.txt":  "nothing to see here\n",
	}
	for name, content := range cases {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	out, err := ScanSecrets(context.Background(), dir, Options{})
	if err != nil {
		t.Fatalf("ScanSecrets: %v", err)
	}
	var leaks []Leak
	if err := json.Unmarshal([]byte(out), &leaks); err != nil {
		t.Fatalf("decode leaks: %v (%s)", err, out)
	}
	byRule := map[string]string{}
	for _, leak := range leaks {
		byRule[leak.RuleID] = filepath.Base(leak.File)
	}
	if byRule["aws-access-token"] != "aws.env" {
		t.Fatalf("aws rule not detected: %#v", byRule)
	}
	if byRule["github-pat"] != "github.txt" {
		t.Fatalf("github rule not detected: %#v", byRule)
	}
	if byRule["private-key"] != "key.pem" {
		t.Fatalf("private key rule not detected: %#v", byRule)
	}
	for _, leak := range leaks {
		if filepath.Base(leak.File) == "clean.txt" {
			t.Fatalf("false positive on clean file: %#v", leak)
		}
		if leak.StartLine <= 0 {
			t.Fatalf("leak missing line number: %#v", leak)
		}
	}
}

func TestScanSecretsSkipsBinaryFiles(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "blob.bin"), []byte{0x00, 'A', 'K', 'I', 'A', 'I', 'O', 'S', 'F', 'O', 'D', 'N', 'N', '7', 'E', 'X', 'A', 'M', 'P', 'L', 'E'}, 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := ScanSecrets(context.Background(), dir, Options{})
	if err != nil {
		t.Fatalf("ScanSecrets: %v", err)
	}
	if strings.Contains(out, "blob.bin") {
		t.Fatalf("binary content must be skipped: %s", out)
	}
}

func TestExtractMetadataPNGAndFallback(t *testing.T) {
	dir := t.TempDir()
	png := []byte("\x89PNG\r\n\x1a\n")
	ihdr := make([]byte, 25)
	copy(ihdr[4:8], "IHDR")
	ihdr[10], ihdr[11] = 0x01, 0x00 // width 256
	ihdr[14], ihdr[15] = 0x00, 0x40 // height 64
	ihdr[16], ihdr[17] = 8, 2
	if err := os.WriteFile(filepath.Join(dir, "shot.png"), append(png, ihdr...), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("plain"), 0o644); err != nil {
		t.Fatal(err)
	}

	out, err := ExtractMetadata(context.Background(), dir, Options{})
	if err != nil {
		t.Fatalf("ExtractMetadata: %v", err)
	}
	var records []map[string]any
	if err := json.Unmarshal([]byte(out), &records); err != nil {
		t.Fatalf("decode metadata: %v (%s)", err, out)
	}
	if len(records) != 2 {
		t.Fatalf("expected 2 records, got %d: %s", len(records), out)
	}
	foundPNG, foundFallback := false, false
	for _, record := range records {
		switch filepath.Base(record["SourceFile"].(string)) {
		case "shot.png":
			foundPNG = true
			if record["ImageWidth"] != float64(256) || record["ImageHeight"] != float64(64) {
				t.Fatalf("png dimensions wrong: %#v", record)
			}
			if record["File:FileType"] != "PNG" {
				t.Fatalf("png type wrong: %#v", record)
			}
		case "notes.txt":
			foundFallback = true
			if _, ok := record["File:FileSize"]; !ok {
				t.Fatalf("fallback record missing File:FileSize: %#v", record)
			}
		}
	}
	if !foundPNG || !foundFallback {
		t.Fatalf("expected both files inspected: %s", out)
	}
}

func TestExtractMetadataMissingTargetIsError(t *testing.T) {
	if _, err := ExtractMetadata(context.Background(), filepath.Join(t.TempDir(), "nope"), Options{}); err == nil {
		t.Fatal("expected error for missing target")
	}
}
