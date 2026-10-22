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

// decodeNucleiJSONL parses the JSONL document the nuclei parser consumes.
func decodeNucleiJSONL(t *testing.T, out string) []nucleiRecord {
	t.Helper()
	records := []nucleiRecord{}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var record nucleiRecord
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatalf("decode nuclei record %q: %v", line, err)
		}
		records = append(records, record)
	}
	return records
}

func findRecordsByID(records []nucleiRecord, id string) []nucleiRecord {
	matched := []nucleiRecord{}
	for _, record := range records {
		if record.TemplateID == id {
			matched = append(matched, record)
		}
	}
	return matched
}

func TestScanTemplatesReportsExposedEnvFile(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/.env" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("DB_PASSWORD=s3cret\nAPI_KEY=xyz\n"))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	out, err := ScanTemplates(context.Background(), server.URL, Options{Timeout: 10 * time.Second})
	if err != nil {
		t.Fatalf("ScanTemplates: %v", err)
	}
	records := decodeNucleiJSONL(t, out)
	findings := findRecordsByID(records, "exposed-env-file")
	if len(findings) != 1 {
		t.Fatalf("expected one exposed-env-file finding, got %d: %s", len(findings), out)
	}
	if findings[0].MatchedAt != server.URL+"/.env" {
		t.Fatalf("unexpected matched-at: %s", findings[0].MatchedAt)
	}
	if findings[0].Info.Severity != "high" || findings[0].Info.Classification.CWEID[0] != "CWE-538" {
		t.Fatalf("unexpected classification: %#v", findings[0].Info)
	}
}

func TestScanTemplatesCleanTargetReportsNothing(t *testing.T) {
	// Every path returns 404: no template may fire, including the negative
	// security-headers matcher (it is now gated on status 200).
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer server.Close()

	out, err := ScanTemplates(context.Background(), server.URL, Options{Timeout: 10 * time.Second})
	if err != nil {
		t.Fatalf("ScanTemplates: %v", err)
	}
	if records := decodeNucleiJSONL(t, out); len(records) != 0 {
		t.Fatalf("expected no findings, got: %s", out)
	}
}

func TestScanTemplatesMissingSecurityHeaderFires(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 200 without x-content-type-options: the negative header matcher fires.
		_, _ = w.Write([]byte("hello"))
	}))
	defer server.Close()

	out, err := ScanTemplates(context.Background(), server.URL, Options{Timeout: 10 * time.Second})
	if err != nil {
		t.Fatalf("ScanTemplates: %v", err)
	}
	if findings := findRecordsByID(decodeNucleiJSONL(t, out), "security-headers-missing"); len(findings) != 1 {
		t.Fatalf("expected security-headers-missing finding, got: %s", out)
	}
}

func TestScanTemplatesDirectoryListingDetection(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			_, _ = w.Write([]byte("<html><head><title>Index of /</title></head></html>"))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	out, err := ScanTemplates(context.Background(), server.URL, Options{Timeout: 10 * time.Second})
	if err != nil {
		t.Fatalf("ScanTemplates: %v", err)
	}
	if findings := findRecordsByID(decodeNucleiJSONL(t, out), "directory-listing"); len(findings) != 1 {
		t.Fatalf("expected directory-listing finding, got: %s", out)
	}
}

func TestScanTemplatesStatus200AloneDoesNotFireBackupTemplate(t *testing.T) {
	// A catch-all 200 server must not raise backup-archive-exposed (high):
	// the tightened matcher now also requires archive/dump content.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("hello"))
	}))
	defer server.Close()

	out, err := ScanTemplates(context.Background(), server.URL, Options{Timeout: 10 * time.Second})
	if err != nil {
		t.Fatalf("ScanTemplates: %v", err)
	}
	if findings := findRecordsByID(decodeNucleiJSONL(t, out), "backup-archive-exposed"); len(findings) != 0 {
		t.Fatalf("expected no backup findings on a plain 200 target, got: %s", out)
	}
}

func TestScanTemplatesRealBackupDumpFires(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/db.sql" {
			_, _ = w.Write([]byte("-- MySQL dump 10.13\nCREATE TABLE x;\n"))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	out, err := ScanTemplates(context.Background(), server.URL, Options{Timeout: 10 * time.Second})
	if err != nil {
		t.Fatalf("ScanTemplates: %v", err)
	}
	findings := findRecordsByID(decodeNucleiJSONL(t, out), "backup-archive-exposed")
	if len(findings) != 1 || findings[0].MatchedAt != server.URL+"/db.sql" {
		t.Fatalf("expected one db.sql backup finding, got: %s", out)
	}
}
