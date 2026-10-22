package nativetools

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"encoding/xml"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestPathFuzzReportsNonNotFoundPaths(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/admin", "/login":
			_, _ = w.Write([]byte("welcome"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	out, err := PathFuzz(context.Background(), server.URL, Options{Timeout: 10 * time.Second, MaxWorkers: 4})
	if err != nil {
		t.Fatalf("PathFuzz: %v", err)
	}
	var ffuf ffufOutput
	if err := json.Unmarshal([]byte(out), &ffuf); err != nil {
		t.Fatalf("decode fuzz output: %v (%s)", err, out)
	}
	found := map[string]bool{}
	for _, result := range ffuf.Results {
		found[strings.TrimPrefix(result.URL, server.URL)] = true
		if result.StatusCode != http.StatusOK {
			t.Fatalf("unexpected status %d for %s", result.StatusCode, result.URL)
		}
	}
	if !found["/admin"] || !found["/login"] {
		t.Fatalf("expected /admin and /login, got %#v", ffuf.Results)
	}
}

func TestCrawlStaysOnOriginAndRespectsDepth(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			_, _ = w.Write([]byte(`<html><a href="/one">one</a><a href="https://example.com/out">out</a></html>`))
		case "/one":
			_, _ = w.Write([]byte(`<html><a href="/two">two</a></html>`))
		case "/two":
			_, _ = w.Write([]byte(`<html><a href="/three">three</a></html>`))
		default:
			_, _ = w.Write([]byte("leaf"))
		}
	}))
	defer server.Close()

	out, err := Crawl(context.Background(), server.URL, Options{Timeout: 10 * time.Second, MaxWorkers: 2})
	if err != nil {
		t.Fatalf("Crawl: %v", err)
	}
	if strings.Contains(out, "example.com") {
		t.Fatalf("crawl escaped the target origin: %s", out)
	}
	if !strings.Contains(out, "/one\"") || !strings.Contains(out, "/two\"") {
		t.Fatalf("expected depth-2 coverage: %s", out)
	}
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		var record katanaRecord
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatalf("crawl output is not JSONL: %v (%s)", err, line)
		}
		if record.Method != http.MethodGet {
			t.Fatalf("unexpected method %q", record.Method)
		}
	}
}

func TestParameterDiscoveryDetectsServedParameter(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("id") == "" {
			_, _ = w.Write([]byte("short"))
			return
		}
		_, _ = w.Write([]byte(strings.Repeat("x", 4096)))
	}))
	defer server.Close()

	out, err := DiscoverParameters(context.Background(), server.URL, Options{Timeout: 15 * time.Second, MaxWorkers: 8})
	if err != nil {
		t.Fatalf("DiscoverParameters: %v", err)
	}
	var discovered map[string][]string
	if err := json.Unmarshal([]byte(out), &discovered); err != nil {
		t.Fatalf("decode parameter output: %v (%s)", err, out)
	}
	params := discovered[server.URL]
	if len(params) == 0 || params[0] != "id" {
		t.Fatalf("expected id to be discovered, got %#v", params)
	}
}

func TestAssessTLSReportsProtocolsAndCertificate(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer server.Close()

	out, err := AssessTLS(context.Background(), strings.TrimPrefix(server.URL, "https://"), Options{Timeout: 10 * time.Second})
	if err != nil {
		t.Fatalf("AssessTLS: %v", err)
	}
	var document sslscanXML
	if err := xml.Unmarshal([]byte(out), &document); err != nil {
		t.Fatalf("decode sslscan XML: %v (%s)", err, out)
	}
	accepted := 0
	for _, protocol := range document.Protocols {
		if protocol.Accepted {
			accepted++
		}
	}
	if accepted == 0 {
		t.Fatalf("no accepted protocol reported: %s", out)
	}
	if document.SSLCert.Subject == "" || document.SSLCert.NotAfter == "" {
		t.Fatalf("certificate summary missing: %s", out)
	}
	if !document.SSLCert.SelfSigned {
		t.Fatalf("httptest certificate must be reported self-signed: %s", out)
	}
}

func TestNativeOperationsAreUnique(t *testing.T) {
	operations := []string{
		OpHTTPProbe, OpPortScan, OpDNSProbe, OpMetadataExtract, OpSecretScan,
		OpPathFuzz, OpCrawl, OpParameterDiscovery, OpTLSAssessment,
		OpSubdomainDiscovery, OpArchiveDiscovery,
	}
	seen := map[string]bool{}
	for _, operation := range operations {
		if seen[operation] {
			t.Fatalf("duplicate operation %q", operation)
		}
		seen[operation] = true
	}
	if tls.VersionTLS13 == 0 {
		t.Fatal("tls constants unavailable")
	}
}
