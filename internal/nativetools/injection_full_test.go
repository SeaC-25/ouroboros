package nativetools

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func decodeInjectionFindings(t *testing.T, out string) []sqlmapFinding {
	t.Helper()
	var findings []sqlmapFinding
	if err := json.Unmarshal([]byte(out), &findings); err != nil {
		t.Fatalf("decode findings: %v (%s)", err, out)
	}
	return findings
}

func hasFinding(findings []sqlmapFinding, param, technique string) bool {
	for _, finding := range findings {
		if finding.Parameter == param && strings.Contains(strings.Join(finding.Techniques, ","), technique) {
			return true
		}
	}
	return false
}

func TestDetectInjectionTimeBasedSleeps(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Query().Get("id"), "SLEEP") {
			time.Sleep(5 * time.Second)
		}
		_, _ = w.Write([]byte("ok"))
	}))
	defer server.Close()

	out, err := DetectInjection(context.Background(), server.URL, Options{Timeout: 60 * time.Second})
	if err != nil {
		t.Fatalf("DetectInjection: %v", err)
	}
	findings := decodeInjectionFindings(t, out)
	if !hasFinding(findings, "id", "time-based") {
		t.Fatalf("expected a time-based finding for id, got: %s", out)
	}
}

func TestDetectInjectionUnionColumnEcho(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		value := r.URL.Query().Get("id")
		if strings.Contains(value, "UNION SELECT") {
			// Vulnerable endpoint: the UNION row (with the sentinel) is
			// echoed back.
			_, _ = w.Write([]byte("1 alice\n" + unionMarker + " NULL NULL"))
			return
		}
		_, _ = w.Write([]byte("1 alice"))
	}))
	defer server.Close()

	out, err := DetectInjection(context.Background(), server.URL, Options{Timeout: 30 * time.Second})
	if err != nil {
		t.Fatalf("DetectInjection: %v", err)
	}
	findings := decodeInjectionFindings(t, out)
	if !hasFinding(findings, "id", "UNION") {
		t.Fatalf("expected a UNION finding for id, got: %s", out)
	}
}

func TestDetectInjectionBooleanBlindDifferential(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("id") {
		case "1 AND 1=1":
			_, _ = w.Write([]byte("1 alice 42")) // mirrors baseline length
		case "1 AND 1=2":
			_, _ = w.Write([]byte("no rows")) // differs
		default:
			_, _ = w.Write([]byte("1 alice 42"))
		}
	}))
	defer server.Close()

	out, err := DetectInjection(context.Background(), server.URL, Options{Timeout: 30 * time.Second})
	if err != nil {
		t.Fatalf("DetectInjection: %v", err)
	}
	findings := decodeInjectionFindings(t, out)
	if !hasFinding(findings, "id", "boolean-based") {
		t.Fatalf("expected a boolean-based finding for id, got: %s", out)
	}
}

func TestDetectInjectionDiscoversFormParameters(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		value := r.URL.Query().Get("productid")
		if strings.Contains(value, "'") {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte("SQLite3.OperationalError: unrecognized token"))
			return
		}
		_, _ = w.Write([]byte(`<html><body><form action="/search" method="get"><input name="productid" type="text"></form></body></html>`))
	}))
	defer server.Close()

	out, err := DetectInjection(context.Background(), server.URL, Options{Timeout: 30 * time.Second})
	if err != nil {
		t.Fatalf("DetectInjection: %v", err)
	}
	findings := decodeInjectionFindings(t, out)
	if !hasFinding(findings, "productid", "error-based") {
		t.Fatalf("expected the discovered productid parameter to be flagged, got: %s", out)
	}
	for _, finding := range findings {
		if finding.Parameter == "productid" && finding.DBMS != "SQLite" {
			t.Fatalf("expected SQLite fingerprint, got %q", finding.DBMS)
		}
	}
}

func TestDetectInjectionPostPlaceOnFormTarget(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			if err := r.ParseForm(); err == nil && strings.Contains(r.PostForm.Get("id"), "'") {
				_, _ = w.Write([]byte("you have an error in your SQL syntax"))
				return
			}
			_, _ = w.Write([]byte("ok"))
			return
		}
		_, _ = w.Write([]byte(`<html><form method="post" action="/login"><input name="id"></form></html>`))
	}))
	defer server.Close()

	out, err := DetectInjection(context.Background(), server.URL, Options{Timeout: 30 * time.Second})
	if err != nil {
		t.Fatalf("DetectInjection: %v", err)
	}
	findings := decodeInjectionFindings(t, out)
	found := false
	for _, finding := range findings {
		if finding.Place == "POST" && finding.Parameter == "id" {
			found = true
			if finding.DBMS != "MySQL" {
				t.Fatalf("expected MySQL fingerprint on POST finding, got %q", finding.DBMS)
			}
		}
	}
	if !found {
		t.Fatalf("expected a POST-place finding for id, got: %s", out)
	}
}

func TestDetectInjectionStackedChangesResponse(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		value := r.URL.Query().Get("id")
		if strings.Contains(value, ";") {
			calls.Add(1)
			_, _ = w.Write([]byte("second result set")) // differs from baseline, no error
			return
		}
		_, _ = w.Write([]byte("1 alice"))
	}))
	defer server.Close()

	out, err := DetectInjection(context.Background(), server.URL, Options{Timeout: 30 * time.Second})
	if err != nil {
		t.Fatalf("DetectInjection: %v", err)
	}
	findings := decodeInjectionFindings(t, out)
	if !hasFinding(findings, "id", "stacked") {
		t.Fatalf("expected a stacked finding for id, got: %s", out)
	}
}
