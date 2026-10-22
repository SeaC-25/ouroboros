package nativetools

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestEnvEndpointOverridesCTSource pins the OURO_CT_ENDPOINT contract: when
// set, the subfinder handler must query it instead of crt.sh.
func TestEnvEndpointOverridesCTSource(t *testing.T) {
	var hitPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hitPath = r.URL.String()
		_, _ = w.Write([]byte(`[{"name_value": "env.example.com"}]`))
	}))
	defer server.Close()

	t.Setenv(EnvCTEndpoint, server.URL+"/?q=%%25.%s&output=json")
	resetEnv()
	defer resetEnv()

	if _, err := DiscoverSubdomains(context.Background(), "example.com", Options{Timeout: 5 * time.Second}); err != nil {
		t.Fatalf("DiscoverSubdomains: %v", err)
	}
	if !strings.Contains(hitPath, "q=%25.example.com") {
		t.Fatalf("expected the override endpoint to be queried, got %q", hitPath)
	}
}

// TestEnvLimitsApply pins the OURO_CRAWL_LIMIT contract: a limit of 1 stops
// the crawl after a single route.
func TestEnvLimitsApply(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<html><a href="/next">next</a></html>`))
	}))
	defer server.Close()

	t.Setenv(EnvCrawlLimit, "1")
	resetEnv()
	defer resetEnv()

	out, err := Crawl(context.Background(), server.URL, Options{Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("Crawl: %v", err)
	}
	lines := 0
	for _, line := range strings.Split(out, "\n") {
		if strings.TrimSpace(line) != "" {
			lines++
		}
	}
	if lines != 1 {
		t.Fatalf("expected exactly 1 crawled route under the limit, got %d: %s", lines, out)
	}
}

// TestEnvInvalidLimitFallsBack pins that a garbage limit falls back to the
// default instead of failing the scan.
func TestEnvInvalidLimitFallsBack(t *testing.T) {
	t.Setenv(EnvCrawlLimit, "not-a-number")
	resetEnv()
	defer resetEnv()

	if limit := crawlRouteLimit(); limit != defaultCrawlRoutes {
		t.Fatalf("expected fallback %d, got %d", defaultCrawlRoutes, limit)
	}
}
