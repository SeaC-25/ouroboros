package nativetools

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestScanReflectedXSSDiscoversFormParameter(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		value := r.URL.Query().Get("userquery")
		if r.URL.RawQuery == "" {
			// Baseline fetch: the form advertises the parameter.
			_, _ = w.Write([]byte(`<html><body><form action="/search"><input name="userquery"></form></body></html>`))
			return
		}
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
	if len(document.PoCs) != 1 {
		t.Fatalf("expected exactly one finding, got %#v", document.PoCs)
	}
	if document.PoCs[0].Param != "userquery" {
		t.Fatalf("expected userquery to be probed and reported, got %#v", document.PoCs)
	}
	if document.PoCs[0].Type != "REFLECTED" {
		t.Fatalf("unexpected poc type: %#v", document.PoCs[0])
	}
}

func TestScanReflectedXSSDiscoversLinkParameter(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		if r.URL.RawQuery == "" {
			_, _ = w.Write([]byte(`<html><body><a href="/item?pid=1">item</a></body></html>`))
			return
		}
		value := r.URL.Query().Get("pid")
		_, _ = w.Write([]byte("<html><body>item details " + value + "</body></html>"))
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
	if len(document.PoCs) != 1 || document.PoCs[0].Param != "pid" {
		t.Fatalf("expected the pid link parameter to be probed and reported, got %#v", document.PoCs)
	}
	if document.PoCs[0].Severity != "MEDIUM" || document.PoCs[0].InjectType != "in_html_text" {
		t.Fatalf("unexpected classification: %#v", document.PoCs[0])
	}
}

func TestScanReflectedXSSReportsDOMSinkReflection(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		value := r.URL.Query().Get("userquery")
		if r.URL.RawQuery == "" {
			_, _ = w.Write([]byte(`<html><body><form><input name="userquery"></form><script>document.write('hello');</script></body></html>`))
			return
		}
		// Reflected inside a string literal right next to document.write.
		_, _ = w.Write([]byte("<html><body><script>document.write('" + value + "');</script></body></html>"))
	}))
	defer server.Close()

	out, err := ScanReflectedXSS(context.Background(), server.URL, Options{Timeout: 10 * time.Second, MaxWorkers: 4})
	if err != nil {
		t.Fatalf("ScanReflecttedXSS: %v", err)
	}
	var document dalfoxOutput
	if err := json.Unmarshal([]byte(out), &document); err != nil {
		t.Fatalf("decode dalfox output: %v (%s)", err, out)
	}
	if len(document.PoCs) != 1 {
		t.Fatalf("expected exactly one finding, got %#v", document.PoCs)
	}
	poc := document.PoCs[0]
	if poc.Param != "userquery" {
		t.Fatalf("expected userquery, got %#v", poc)
	}
	if poc.InjectType != "dom_sink_reflection" {
		t.Fatalf("expected dom_sink_reflection, got %#v", poc)
	}
	if poc.Severity != "HIGH" {
		t.Fatalf("expected HIGH severity, got %#v", poc)
	}
	if poc.Type != "REFLECTED" {
		t.Fatalf("unexpected poc type: %#v", poc)
	}
}
