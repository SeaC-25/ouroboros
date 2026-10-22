package nativetools

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// decodeJSONLine decodes one JSON line.
func decodeJSONLine(line string, target any) error {
	return json.Unmarshal([]byte(line), target)
}

// mustParseURL parses a test URL.
func mustParseURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	parsed, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parse %q: %v", raw, err)
	}
	return parsed
}

func decodeTemplateLines(t *testing.T, out string) []nucleiRecord {
	t.Helper()
	records := []nucleiRecord{}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var record nucleiRecord
		if err := decodeJSONLine(line, &record); err != nil {
			t.Fatalf("decode nuclei record %q: %v", line, err)
		}
		records = append(records, record)
	}
	return records
}

func findTemplate(records []nucleiRecord, id string) *nucleiRecord {
	for i := range records {
		if records[i].TemplateID == id {
			return &records[i]
		}
	}
	return nil
}

func TestEngineParsesNucleiYAMLTemplate(t *testing.T) {
	source := `id: external-exposed-admin
info:
  name: External admin panel
  author: ouroboros
  severity: high
  description: Admin panel visible
  classification:
    cwe-id: CWE-200
http:
  requests:
    - method: GET
      path:
        - "/admin"
      matchers:
        - type: status
          status: [200]
        - type: word
          words: ["admin", "panel"]
          condition: and
      extractors:
        - type: regex
          name: version
          regex:
            - 'version[":= ]+([0-9.]+)'
`
	tmpl, err := parseNucleiYAML(source)
	if err != nil {
		t.Fatalf("parseNucleiYAML: %v", err)
	}
	if tmpl.ID != "external-exposed-admin" || tmpl.Severity != "high" {
		t.Fatalf("unexpected template header: %#v", tmpl)
	}
	if len(tmpl.CWE) != 1 || tmpl.CWE[0] != "CWE-200" {
		t.Fatalf("unexpected CWE: %#v", tmpl.CWE)
	}
	if len(tmpl.Requests) != 1 || tmpl.Requests[0].Method != "GET" {
		t.Fatalf("unexpected requests: %#v", tmpl.Requests)
	}
	if len(tmpl.Requests[0].Matchers) != 2 {
		t.Fatalf("expected 2 matchers, got %#v", tmpl.Requests[0].Matchers)
	}
	if len(tmpl.Requests[0].Extractors) != 1 || tmpl.Requests[0].Extractors[0].Name != "version" {
		t.Fatalf("expected the version extractor, got %#v", tmpl.Requests[0].Extractors)
	}
}

func TestEngineRejectsTemplateWithoutMatchersOrExtractors(t *testing.T) {
	if _, err := parseNucleiYAML("id: broken\nhttp:\n  requests:\n    - method: GET\n      path:\n        - \"/\"\n"); err == nil {
		t.Fatal("expected an error for a request with no matchers and no extractors")
	}
	if _, err := parseNucleiYAML("http:\n  requests:\n    - method: GET\n"); err == nil {
		t.Fatal("expected an error for a template without id")
	}
}

func TestEngineRunsExternalTemplatesFromDir(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/legacy-console" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`<html><body>weblogic console version 12.2.1</body></html>`))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	dir := t.TempDir()
	template := `id: legacy-console-exposed
info:
  name: Legacy console exposed
  severity: critical
http:
  requests:
    - method: GET
      path:
        - "/legacy-console"
      matchers:
        - type: word
          words: ["weblogic console"]
      extractors:
        - type: regex
          name: version
          regex:
            - 'version ([0-9.]+)'
`
	if err := os.WriteFile(filepath.Join(dir, "legacy-console.yaml"), []byte(template), 0o644); err != nil {
		t.Fatalf("write template: %v", err)
	}
	t.Setenv(EnvTemplatesDir, dir)

	out, err := ScanTemplates(context.Background(), server.URL, Options{Timeout: 10 * time.Second})
	if err != nil {
		t.Fatalf("ScanTemplates: %v", err)
	}
	records := decodeTemplateLines(t, out)
	record := findTemplate(records, "legacy-console-exposed")
	if record == nil {
		t.Fatalf("expected the external template to fire, got: %s", out)
	}
	if record.Info.Severity != "critical" {
		t.Fatalf("unexpected severity: %#v", record.Info)
	}
	if !strings.Contains(record.ExtractedValue, "version=12.2.1") {
		t.Fatalf("expected the version extractor result, got %q", record.ExtractedValue)
	}
}

func TestEngineDSLMatcherEvaluates(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"status": "ok", "debug": true}`))
	}))
	defer server.Close()

	engine := &templateEngine{client: http.DefaultClient}
	parsed := mustParseURL(t, server.URL)
	engine.target = parsed

	tmpl := engineTemplate{
		ID: "dsl-debug-endpoint", Name: "Debug endpoint", Severity: "medium",
		Requests: []templateRequest{{
			Method: http.MethodGet, Paths: []string{"/"},
			Matchers: []templateMatcher{
				{Type: "dsl", DSL: []string{`status_code == 200`, `contains(body, "\"debug\": true")`}},
			},
		}},
	}
	reported := engine.runTemplate(context.Background(), tmpl)
	if len(reported) != 1 {
		t.Fatalf("expected the DSL matcher to pass, got %#v", reported)
	}

	tmpl.Requests[0].Matchers[0].DSL = []string{`status_code == 404`, `contains(body, "\"debug\": true")`}
	reported = engine.runTemplate(context.Background(), tmpl)
	if len(reported) != 0 {
		t.Fatalf("expected the DSL matcher to fail on status, got %#v", reported)
	}
}

func TestEngineExtractorFlowsIntoSecondRequest(t *testing.T) {
	var secondPathSeen string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/first":
			_, _ = w.Write([]byte(`token: abc123xyz`))
		case "/use":
			secondPathSeen = r.URL.RawQuery
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("ok"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	engine := &templateEngine{client: http.DefaultClient, target: mustParseURL(t, server.URL)}
	tmpl := engineTemplate{
		ID: "two-step-flow", Name: "Two step", Severity: "low",
		Requests: []templateRequest{
			{
				Method: http.MethodGet, Paths: []string{"/first"},
				Extractors: []templateExtractor{
					{Type: "kval", Name: "token", KVal: []string{"token"}},
				},
			},
			{
				Method: http.MethodGet, Paths: []string{"/use?t={{token}}"},
				Matchers: []templateMatcher{
					{Type: "status", Status: []int{200}},
				},
			},
		},
	}
	reported := engine.runTemplate(context.Background(), tmpl)
	if secondPathSeen != "t=abc123xyz" {
		t.Fatalf("expected the extractor value to flow into request 2, got %q", secondPathSeen)
	}
	// Both requests report: the extractor-only first request and the
	// matched second one carrying the extracted token.
	if len(reported) != 2 {
		t.Fatalf("expected both requests to report, got %#v", reported)
	}
	if !strings.Contains(reported[1].URL, "t=abc123xyz") {
		t.Fatalf("expected the second request URL to carry the token, got %q", reported[1].URL)
	}
}

func TestEngineWordConditionOrSemantics(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("only-alpha present"))
	}))
	defer server.Close()

	engine := &templateEngine{client: http.DefaultClient, target: mustParseURL(t, server.URL)}
	request := templateRequest{
		Method: http.MethodGet, Paths: []string{"/"},
		Matchers: []templateMatcher{
			{Type: "word", Words: []string{"alpha", "omega"}, Condition: "or"},
		},
	}
	resp, body, headers, _, err := engine.fetch(context.Background(), request, "/")
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if !matcherEvaluate(request.Matchers[0], resp, body, headers) {
		t.Fatal("expected word:or to pass when one word matches")
	}
	// Default (no condition): nuclei ANDs the words — omega is absent, so
	// the matcher must fail once the condition is dropped.
	request.Matchers[0].Condition = ""
	if matcherEvaluate(request.Matchers[0], resp, body, headers) {
		t.Fatal("expected default AND semantics to fail when only one of two words matches")
	}
	// Single-word list: AND over one word degenerates to containment.
	request.Matchers[0].Words = []string{"alpha"}
	if !matcherEvaluate(request.Matchers[0], resp, body, headers) {
		t.Fatal("expected a single-word matcher to pass when the word is present")
	}
}

func TestEngineJSONExtractor(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"user": {"name": "ouro", "roles": ["admin", "user"]}}`))
	}))
	defer server.Close()

	engine := &templateEngine{client: http.DefaultClient, target: mustParseURL(t, server.URL)}
	request := templateRequest{
		Method: http.MethodGet, Paths: []string{"/"},
		Extractors: []templateExtractor{
			{Type: "json", Name: "role", JSON: []string{"user.roles[0]"}},
		},
		Matchers: []templateMatcher{
			{Type: "status", Status: []int{200}},
		},
	}
	reported := engine.runTemplate(context.Background(), engineTemplate{
		ID: "json-extract", Name: "json", Severity: "info", Requests: []templateRequest{request},
	})
	if len(reported) != 1 || reported[0].Extracts["role"] != "admin" {
		t.Fatalf("expected the json extractor to read user.roles[0]=admin, got %#v", reported)
	}
}

func TestScanTemplatesCORSWildcardFires(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		_, _ = w.Write([]byte("hello"))
	}))
	defer server.Close()

	out, err := ScanTemplates(context.Background(), server.URL, Options{Timeout: 10 * time.Second})
	if err != nil {
		t.Fatalf("ScanTemplates: %v", err)
	}
	if findTemplate(decodeTemplateLines(t, out), "cors-wildcard") == nil {
		t.Fatalf("expected cors-wildcard finding, got: %s", out)
	}
}

func TestScanTemplatesOpenRedirectFires(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			if target := r.URL.Query().Get("url"); target != "" {
				w.Header().Set("Location", target)
				w.WriteHeader(http.StatusFound)
				return
			}
		}
		_, _ = w.Write([]byte("home"))
	}))
	defer server.Close()

	out, err := ScanTemplates(context.Background(), server.URL, Options{Timeout: 10 * time.Second})
	if err != nil {
		t.Fatalf("ScanTemplates: %v", err)
	}
	if findTemplate(decodeTemplateLines(t, out), "open-redirect") == nil {
		t.Fatalf("expected open-redirect finding, got: %s", out)
	}
}

func TestScanTemplatesCleanTargetStaysCleanWithNewTemplates(t *testing.T) {
	// Every request 404s and no header injection: none of the 13 built-in
	// templates may fire.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer server.Close()

	out, err := ScanTemplates(context.Background(), server.URL, Options{Timeout: 10 * time.Second})
	if err != nil {
		t.Fatalf("ScanTemplates: %v", err)
	}
	if records := decodeTemplateLines(t, out); len(records) != 0 {
		t.Fatalf("expected no findings on a clean target, got: %s", out)
	}
}
