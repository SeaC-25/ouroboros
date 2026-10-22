package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ouroboros/ouroboros/internal/compiler"
)

func runNativeOperation(t *testing.T, operation, parserID, targetKind, targetValue string) string {
	t.Helper()
	statements := runNativeToolForTest(t, operation, parserID, targetKind, targetValue)
	return strings.Join(statements, "\n")
}

func TestNativeTrufflehogSecretScanReachesParser(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "creds.env"), []byte("token=ghp_"+strings.Repeat("c", 36)+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	joined := runNativeOperation(t, "secret_scan", "trufflehog_json", "artifact", dir)
	if !strings.Contains(joined, `"kind":"secret"`) {
		t.Fatalf("no secret fact from trufflehog-shaped output: %s", joined)
	}
	if !strings.Contains(joined, "github-pat") || !strings.Contains(joined, "creds.env") {
		t.Fatalf("detector or file missing: %s", joined)
	}
}

func TestNativeBinwalkFirmwareReachesParser(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fw.bin")
	payload := append([]byte{0x1F, 0x8B, 0x08}, make([]byte, 8)...)
	payload = append(payload, []byte("hsqs")...)
	if err := os.WriteFile(path, payload, 0o644); err != nil {
		t.Fatal(err)
	}
	joined := runNativeOperation(t, "firmware_analyze", "binwalk_text", "artifact", path)
	if !strings.Contains(joined, `"kind":"artifact_content"`) {
		t.Fatalf("no firmware fact: %s", joined)
	}
	if !strings.Contains(joined, "gzip") || !strings.Contains(joined, "Squashfs") {
		t.Fatalf("signature descriptions missing: %s", joined)
	}
}

func TestNativeNmapServiceFingerprintReachesParser(t *testing.T) {
	// 127.0.0.1 with the default connect-scan port set; expect at least a
	// structured XML document the nmap parser accepts (no open ports is fine).
	joined := runNativeOperation(t, "service_fingerprint", "nmap_xml", "host", "127.0.0.1:1")
	if !strings.Contains(joined, "parser") && !strings.Contains(joined, "fact") {
		t.Fatalf("no parser events for nmap node: %s", joined)
	}
}

func TestTestsslFindingParserRejectsMalformedJSON(t *testing.T) {
	request := ToolExecutionRequest{Node: compiler.PlanNode{ID: "n", ParserID: "testssl_json"}}
	if _, err := testsslParserEvents("run", request, ToolExecutionResult{Status: "completed", Transcript: "{not json"}); err == nil {
		t.Fatal("expected error for malformed testssl JSON")
	}
}

func TestSslscanFindingParserReadsProtocols(t *testing.T) {
	request := ToolExecutionRequest{Node: compiler.PlanNode{ID: "n", ParserID: "sslscan_xml"}}
	document := `<document><ssltest type="protocol" version="TLSv1.2" supported="true" accepted="true"><status>accepted</status></ssltest>` +
		`<cipher status="accepted" sslversion="TLSv1.2" bits="256" cipher="ECDHE-RSA-AES256-GCM-SHA384"/></document>`
	events, err := sslscanParserEvents("run", request, ToolExecutionResult{Status: "completed", Transcript: document})
	if err != nil {
		t.Fatalf("sslscanParserEvents: %v", err)
	}
	if len(events) < 3 {
		t.Fatalf("expected status + protocol + cipher events, got %d", len(events))
	}
	joined := ""
	for _, event := range events {
		joined += string(event.Payload)
	}
	if !strings.Contains(joined, `"kind":"tls_assessment"`) {
		t.Fatalf("no tls assessment fact: %s", joined)
	}
	if !strings.Contains(joined, "ECDHE-RSA-AES256-GCM-SHA384") {
		t.Fatalf("cipher missing from facts: %s", joined)
	}
}

func TestBinwalkParserRejectsEmptyOutput(t *testing.T) {
	request := ToolExecutionRequest{Node: compiler.PlanNode{ID: "n", ParserID: "binwalk_text"}}
	if _, err := binwalkParserEvents("run", request, ToolExecutionResult{Status: "completed", Transcript: "   "}); err == nil {
		t.Fatal("expected error for empty binwalk output")
	}
}
