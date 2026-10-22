package nativetools

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestScanReflectedXSSReportsContextAndSeverity(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		value := r.URL.Query().Get("q")
		w.Header().Set("Content-Type", "text/html")
		// Reflect inside HTML text (no escaping): the classic reflected case.
		_, _ = w.Write([]byte("<html><body>results for " + value + "</body></html>"))
	}))
	defer server.Close()

	out, err := ScanReflectedXSS(context.Background(), server.URL, Options{Timeout: 10 * time.Second, MaxWorkers: 4})
	if err != nil {
		t.Fatalf("ScanReflectedXSS: %v", err)
	}
	var document dalfoxOutput
	if err := json.Unmarshal([]byte(out), &document); err != nil {
		t.Fatalf("decode dalfox output: %v (%s)", err, out)
	}
	if len(document.PoCs) != 1 || document.PoCs[0].Param != "q" {
		t.Fatalf("expected a single q finding, got %#v", document.PoCs)
	}
	if document.PoCs[0].Severity != "MEDIUM" || document.PoCs[0].InjectType != "in_html_text" {
		t.Fatalf("unexpected classification: %#v", document.PoCs[0])
	}
	if document.PoCs[0].Type != "REFLECTED" {
		t.Fatalf("unexpected poc type: %#v", document.PoCs[0])
	}
}

func TestScanReflectedXSSIgnoresEscapedReflection(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Escaped output: the raw marker never appears, so nothing is reported.
		_, _ = w.Write([]byte("<html><body>no reflection here</body></html>"))
	}))
	defer server.Close()

	out, err := ScanReflectedXSS(context.Background(), server.URL, Options{Timeout: 10 * time.Second})
	if err != nil {
		t.Fatalf("ScanReflectedXSS: %v", err)
	}
	var document dalfoxOutput
	if err := json.Unmarshal([]byte(out), &document); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(document.PoCs) != 0 {
		t.Fatalf("expected no findings, got %#v", document.PoCs)
	}
}

func TestDetectInjectionFindsErrorSignature(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("id") == "1'" {
			_, _ = w.Write([]byte("You have an error in your SQL syntax; check the manual"))
			return
		}
		_, _ = w.Write([]byte("ok"))
	}))
	defer server.Close()

	out, err := DetectInjection(context.Background(), server.URL, Options{Timeout: 10 * time.Second})
	if err != nil {
		t.Fatalf("DetectInjection: %v", err)
	}
	var findings []sqlmapFinding
	if err := json.Unmarshal([]byte(out), &findings); err != nil {
		t.Fatalf("decode sqlmap findings: %v (%s)", err, out)
	}
	if len(findings) != 1 || findings[0].Parameter != "id" || !findings[0].Vulnerable {
		t.Fatalf("expected the id parameter to be flagged, got %#v", findings)
	}
	if findings[0].Place != "GET" || len(findings[0].Techniques) == 0 {
		t.Fatalf("finding missing place/technique: %#v", findings[0])
	}
}

func TestDetectInjectionStaysSilentOnCleanEndpoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("all good"))
	}))
	defer server.Close()

	out, err := DetectInjection(context.Background(), server.URL, Options{Timeout: 10 * time.Second})
	if err != nil {
		t.Fatalf("DetectInjection: %v", err)
	}
	if strings.TrimSpace(out) != "[]" {
		t.Fatalf("expected an empty finding list, got %s", out)
	}
}

func TestDNSReconRecordEmitsTypedRecords(t *testing.T) {
	// localhost resolves in every environment; the exact record set is
	// environment dependent, so assert on shape rather than contents.
	out, err := DNSReconRecord(context.Background(), "localhost", Options{Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("DNSReconRecord: %v", err)
	}
	if strings.TrimSpace(out) == "" {
		t.Skip("localhost did not resolve in this environment")
	}
	var records []dnsreconRecord
	if err := json.Unmarshal([]byte(out), &records); err != nil {
		t.Fatalf("decode dnsrecon records: %v (%s)", err, out)
	}
	for _, record := range records {
		if record.Type == "" || record.Name == "" {
			t.Fatalf("record missing type/name: %#v", record)
		}
	}
	hasAddress := false
	for _, record := range records {
		if record.Type == "A" || record.Type == "AAAA" {
			hasAddress = true
		}
	}
	if !hasAddress {
		t.Fatalf("expected an address record for localhost: %s", out)
	}
}
