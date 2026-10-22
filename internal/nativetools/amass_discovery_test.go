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

// withTestCTSource points the shared crt.sh passive source at a test server
// through the OURO_CT_ENDPOINT contract: %%25 renders %25 and %s receives the
// escaped domain.
func withTestCTSource(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Setenv(EnvCTEndpoint, server.URL+"/?q=%%25.%s&output=json")
	resetEnv()
	t.Cleanup(func() {
		resetEnv()
		server.Close()
	})
	return server
}

func TestDiscoverSubdomainsAmassEmitsAmassRecords(t *testing.T) {
	var gotQuery string
	withTestCTSource(t, func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		// Mirror crt.sh's JSON array of name_value records.
		_, _ = w.Write([]byte(`[
			{"name_value": "example.com\nwww.example.com"},
			{"name_value": "api.example.com"},
			{"name_value": "out-of-scope.other.com"},
			{"name_value": "www.example.com"}
		]`))
	})

	out, err := DiscoverSubdomainsAmass(context.Background(), "example.com", Options{Timeout: 10 * time.Second})
	if err != nil {
		t.Fatalf("DiscoverSubdomainsAmass: %v", err)
	}

	// The wildcard query must carry the escaped domain: q=%25.example.com.
	if !strings.Contains(gotQuery, "q=%25.example.com") {
		t.Fatalf("expected the domain inside the wildcard query, got %q", gotQuery)
	}

	records := []amassRecord{}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var record amassRecord
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatalf("decode amass record %q: %v", line, err)
		}
		records = append(records, record)
	}

	names := map[string]bool{}
	for _, record := range records {
		names[record.Name] = true
		if record.Domain != "example.com" {
			t.Fatalf("unexpected domain on record: %#v", record)
		}
	}
	for _, want := range []string{"example.com", "www.example.com", "api.example.com"} {
		if !names[want] {
			t.Fatalf("expected %s in results, got: %s", want, out)
		}
	}
	if names["out-of-scope.other.com"] {
		t.Fatalf("out-of-scope host leaked into results: %s", out)
	}
}

func TestDiscoverSubdomainsEmitsSubfinderRecords(t *testing.T) {
	withTestCTSource(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[
			{"name_value": "example.com\nwww.example.com"},
			{"name_value": "api.example.com"},
			{"name_value": "other.org"}
		]`))
	})

	out, err := DiscoverSubdomains(context.Background(), "example.com", Options{Timeout: 10 * time.Second})
	if err != nil {
		t.Fatalf("DiscoverSubdomains: %v", err)
	}
	hosts := map[string]bool{}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var record subfinderRecord
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatalf("decode subfinder record %q: %v", line, err)
		}
		hosts[record.Host] = true
	}
	for _, want := range []string{"example.com", "www.example.com", "api.example.com"} {
		if !hosts[want] {
			t.Fatalf("expected %s in results, got: %s", want, out)
		}
	}
	if hosts["other.org"] {
		t.Fatalf("out-of-scope host leaked into results: %s", out)
	}
}

func TestDiscoverSubdomainsAmassEmptyDomainIsAnError(t *testing.T) {
	if _, err := DiscoverSubdomainsAmass(context.Background(), "  ", Options{Timeout: time.Second}); err == nil {
		t.Fatal("expected an error for an empty domain")
	}
}

func TestDiscoverSubdomainsAmassSourceFailureIsAnError(t *testing.T) {
	withTestCTSource(t, func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	})

	if _, err := DiscoverSubdomainsAmass(context.Background(), "example.com", Options{Timeout: 5 * time.Second}); err == nil {
		t.Fatal("expected an error when the passive source fails")
	}
}
