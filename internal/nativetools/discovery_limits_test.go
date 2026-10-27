package nativetools

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// withTestCDXSource points the Internet Archive CDX endpoint at a test server
// through the OURO_CDX_ENDPOINT contract: %s receives the escaped "domain/*"
// wildcard, the second %s the result limit.
func withTestCDXSource(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Setenv(EnvCDXEndpoint, server.URL+"/cdx?url=%s&limit=%s&fl=original")
	t.Cleanup(server.Close)
	return server
}

func TestDiscoverArchiveURLsMultiDomainBestEffort(t *testing.T) {
	var queries []string
	withTestCDXSource(t, func(w http.ResponseWriter, r *http.Request) {
		queries = append(queries, r.URL.Query().Get("url"))
		if strings.HasPrefix(r.URL.Query().Get("url"), "bad.example.com") {
			http.Error(w, "boom", http.StatusInternalServerError)
			return
		}
		_, _ = w.Write([]byte(`[["original"],
			["https://good.example.com/index.html"],
			["https://sub.good.example.com/page.html"],
			["https://out-of-scope.other.com/x"],
			["https://good.example.com/index.html"]]`))
	})

	out, err := DiscoverArchiveURLs(context.Background(), "good.example.com, bad.example.com.", Options{Timeout: 10 * time.Second})
	if err != nil {
		// A single failing source must not sink results collected for the
		// healthy sibling domain.
		t.Fatalf("one failing domain must not fail the whole run: %v", err)
	}

	if len(queries) != 2 {
		t.Fatalf("expected both domains queried, got %q", queries)
	}
	for _, query := range queries {
		if !strings.Contains(query, "/*") {
			t.Fatalf("expected wildcard query, got %q", query)
		}
	}

	// Success from the healthy domain must survive the failing sibling.
	want := []string{
		"https://good.example.com/index.html",
		"https://sub.good.example.com/page.html",
	}
	if got := strings.Split(out, "\n"); len(got) != len(want) {
		t.Fatalf("expected exactly the healthy domain's URLs, got: %q", got)
	}
	for _, line := range want {
		if !strings.Contains(out, line) {
			t.Fatalf("expected %s in results, got: %s", line, out)
		}
	}
	if strings.Contains(out, "out-of-scope") {
		t.Fatalf("out-of-scope URL leaked: %s", out)
	}
}

func TestDiscoverArchiveURLsAllDomainsFailingIsError(t *testing.T) {
	withTestCDXSource(t, func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	})

	if _, err := DiscoverArchiveURLs(context.Background(), "a.example.com,b.example.com", Options{Timeout: 5 * time.Second}); err == nil {
		t.Fatal("expected an aggregated error when every domain fails")
	}
}

func TestAnalyzeFirmwareSkipsOversizedFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "huge.bin")
	if err := os.WriteFile(path, make([]byte, maxScanFileSize+1), 0o644); err != nil {
		t.Fatal(err)
	}

	out, err := AnalyzeFirmware(context.Background(), path, Options{})
	if err != nil {
		t.Fatalf("oversized file must be skipped, not failed: %v", err)
	}
	if !strings.Contains(out, "skipped") || !strings.Contains(out, "huge.bin") {
		t.Fatalf("skip reason missing from report: %s", out)
	}
}

func TestExtractMetadataSkipsOversizedFile(t *testing.T) {
	dir := t.TempDir()
	png := []byte("\x89PNG\r\n\x1a\n")
	if err := os.WriteFile(filepath.Join(dir, "ok.png"), png, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "huge.png"), make([]byte, maxScanFileSize+1), 0o644); err != nil {
		t.Fatal(err)
	}

	out, err := ExtractMetadata(context.Background(), dir, Options{})
	if err != nil {
		t.Fatalf("ExtractMetadata: %v", err)
	}
	if !strings.Contains(out, "huge.png") || !strings.Contains(out, "Skipped") {
		t.Fatalf("oversized file must be reported as skipped: %s", out)
	}
	if !strings.Contains(out, "ok.png") {
		t.Fatalf("healthy sibling must still be scanned: %s", out)
	}
}
