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

func TestHTTPProbeEmitsHTTPXCompatibleJSONL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Server", "ouro-test/1")
		_, _ = w.Write([]byte("<html><head><title> Ouroboros Probe </title></head><body>ok</body></html>"))
	}))
	defer server.Close()

	out, err := HTTPProbe(context.Background(), server.URL, Options{Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("HTTPProbe: %v", err)
	}

	var record map[string]any
	if err := json.Unmarshal([]byte(out), &record); err != nil {
		t.Fatalf("output is not a single JSON object: %v (%s)", err, out)
	}
	if got := record["url"]; got != server.URL {
		t.Fatalf("url = %v, want %s", got, server.URL)
	}
	if got := record["status_code"]; got != float64(http.StatusOK) {
		t.Fatalf("status_code = %v", got)
	}
	if got := record["content_type"]; got != "text/html; charset=utf-8" {
		t.Fatalf("content_type = %v", got)
	}
	if got := record["title"]; got != "Ouroboros Probe" {
		t.Fatalf("title = %v", got)
	}
	if got := record["webserver"]; got != "ouro-test/1" {
		t.Fatalf("webserver = %v", got)
	}
	if _, ok := record["content_length"]; !ok {
		t.Fatal("content_length missing")
	}
}

func TestHTTPProbeRecordsRedirectFinalURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/final" {
			_, _ = w.Write([]byte("done"))
			return
		}
		http.Redirect(w, r, "/final", http.StatusFound)
	}))
	defer server.Close()

	out, err := HTTPProbe(context.Background(), server.URL, Options{Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("HTTPProbe: %v", err)
	}
	var record map[string]any
	if err := json.Unmarshal([]byte(out), &record); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got := record["status_code"]; got != float64(http.StatusOK) {
		t.Fatalf("status_code after redirect = %v", got)
	}
	final, _ := record["final_url"].(string)
	if !strings.HasSuffix(final, "/final") {
		t.Fatalf("final_url = %q, want suffix /final", final)
	}
}

func TestHTTPProbeTimeoutIsAnError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(300 * time.Millisecond)
	}))
	defer server.Close()

	_, err := HTTPProbe(context.Background(), server.URL, Options{Timeout: 50 * time.Millisecond})
	if err == nil {
		t.Fatal("expected timeout error")
	}
}

func TestHTTPProbeRequiresTarget(t *testing.T) {
	if _, err := HTTPProbe(context.Background(), "   ", Options{}); err == nil {
		t.Fatal("expected error for empty target")
	}
}

func TestRunRejectsUnknownOperation(t *testing.T) {
	if _, err := Run(context.Background(), "not_a_real_operation", Target{Value: "x"}, Options{}); err == nil {
		t.Fatal("expected error for unknown operation")
	}
}

func TestPortScanFindsOpenTestPort(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer server.Close()
	// httptest listens on 127.0.0.1:<port>; scan that exact port.
	host, port, ok := strings.Cut(strings.TrimPrefix(server.URL, "http://"), ":")
	if !ok {
		t.Fatalf("unexpected server URL %s", server.URL)
	}

	out, err := PortScan(context.Background(), host+":"+port, Options{Timeout: 5 * time.Second, MaxWorkers: 4})
	if err != nil {
		t.Fatalf("PortScan: %v", err)
	}
	if !strings.Contains(out, `"port":`+port) {
		t.Fatalf("open port %s not reported: %q", port, out)
	}
	if !strings.Contains(out, `"protocol":"tcp"`) {
		t.Fatalf("protocol missing: %q", out)
	}
}

func TestParsePortSpecBounds(t *testing.T) {
	if _, err := parsePortSpec("0"); err == nil {
		t.Fatal("port 0 must be rejected")
	}
	if _, err := parsePortSpec("70000"); err == nil {
		t.Fatal("port 70000 must be rejected")
	}
	ports, err := parsePortSpec("80,443")
	if err != nil || len(ports) != 2 {
		t.Fatalf("parsePortSpec(80,443) = %v, %v", ports, err)
	}
}

func TestDNSProbeSkipsUnresolvableHost(t *testing.T) {
	out, err := DNSProbe(context.Background(), "ouroboros-native-tools.invalid", Options{Timeout: 3 * time.Second})
	if err != nil {
		t.Fatalf("DNSProbe: %v", err)
	}
	if out != "" {
		t.Fatalf("expected empty output for unresolvable host, got %q", out)
	}
}
