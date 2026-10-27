package nativetools

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/url"
	"sort"
)

// OpTemplateScan is the nuclei-equivalent (HTTP template engine subset).
const OpTemplateScan = "template_scan"

// templateRequest is the nuclei-compatible HTTP template request model the
// engine (template_engine.go) executes: method, paths, headers, body,
// matchers (all/any pair or unified list with a condition) and extractors.
type templateRequest struct {
	Method      string
	Path        []string
	Paths       []string // YAML alias of Path
	Headers     map[string]string
	Body        string
	ContentType string
	// Matchers/MatchersAny is the legacy pair (AND within all, OR across any);
	// Matchers is the unified list combined by Condition ("or" default).
	Matchers    []templateMatcher
	MatchersAll []templateMatcher
	MatchersAny []templateMatcher
	Extractors  []templateExtractor
	Condition   string
	// Interact marks a request whose matchers depend on an out-of-band
	// callback ({{interacturl}} in the payload, confirmed by polling the
	// interaction endpoint — see template_engine.go).
	Interact bool
	// StopAtFirstMatch ends the request chain once one path matches.
	StopAtFirstMatch bool
	// Redirects follows 3xx responses for this request (engine default is
	// to preserve them for redirect-based matchers).
	Redirects bool
}

type templateMatcher struct {
	Name      string
	Type      string // status | word | words | regex | binary | size | dsl
	Status    []int
	Words     []string
	Regex     []string
	Binary    []string
	Size      []int
	Part      string // body | header | all
	Negative  bool
	Condition string // or | and within this matcher's values
	DSL       []string
}

type templateExtractor struct {
	Type  string // regex | kval | json | xpath
	Name  string
	Part  string
	Regex []string
	KVal  []string
	JSON  []string
	XPath []string
}

type builtInTemplate struct {
	ID       string
	Name     string
	Severity string
	CWE      string
	Requests []templateRequest
}

// nucleiRecord mirrors the nuclei JSONL fields the parser reads
// (internal/app/nuclei_parser.go: template-id, type, matched-at, info).
type nucleiRecord struct {
	TemplateID     string `json:"template-id"`
	TemplateURL    string `json:"template-url,omitempty"`
	Type           string `json:"type"`
	MatchedAt      string `json:"matched-at"`
	MatcherName    string `json:"matcher-name,omitempty"`
	ExtractedValue string `json:"extracted-results,omitempty"`
	Info           struct {
		Name           string `json:"name"`
		Severity       string `json:"severity"`
		Description    string `json:"description,omitempty"`
		Classification struct {
			CWEID []string `json:"cwe-id,omitempty"`
			CVEID []string `json:"cve-id,omitempty"`
		} `json:"classification,omitempty"`
	} `json:"info"`
}

// builtInTemplates is the embedded rule set. It covers the high-signal,
// body-observable exposures a native scan can decide without a full DSL:
// the managed nuclei bundle remains the reference for the upstream catalogue.
var builtInTemplates = []builtInTemplate{
	{
		ID:       "exposed-git-config",
		Name:     "Exposed Git config",
		Severity: "medium",
		CWE:      "CWE-538",
		Requests: []templateRequest{{
			Method: http.MethodGet,
			Path:   []string{"/.git/config"},
			MatchersAll: []templateMatcher{
				{Type: "status", Status: []int{200}},
				{Type: "words", Words: []string{"[core]", "repositoryformatversion"}, Part: "body"},
			},
		}},
	},
	{
		ID:       "exposed-env-file",
		Name:     "Exposed .env file",
		Severity: "high",
		CWE:      "CWE-538",
		Requests: []templateRequest{{
			Method:      http.MethodGet,
			Path:        []string{"/.env"},
			MatchersAny: []templateMatcher{{Type: "status", Status: []int{200}}},
			MatchersAll: []templateMatcher{{Type: "regex", Regex: []string{`(?m)^[A-Z0-9_]+=`}, Part: "body"}},
		}},
	},
	{
		ID:       "directory-listing",
		Name:     "Directory listing enabled",
		Severity: "low",
		CWE:      "CWE-548",
		Requests: []templateRequest{{
			Method:      http.MethodGet,
			Path:        []string{"/"},
			MatchersAll: []templateMatcher{{Type: "words", Words: []string{"Index of /"}, Part: "body"}},
		}},
	},
	{
		ID:       "phpinfo-disclosure",
		Name:     "phpinfo() exposed",
		Severity: "medium",
		CWE:      "CWE-200",
		Requests: []templateRequest{{
			Method:      http.MethodGet,
			Path:        []string{"/phpinfo.php", "/info.php"},
			MatchersAll: []templateMatcher{{Type: "words", Words: []string{"phpinfo()", "PHP Version"}, Part: "body"}},
		}},
	},
	{
		ID:       "server-status-exposed",
		Name:     "Apache server-status exposed",
		Severity: "medium",
		CWE:      "CWE-200",
		Requests: []templateRequest{{
			Method:      http.MethodGet,
			Path:        []string{"/server-status"},
			MatchersAll: []templateMatcher{{Type: "words", Words: []string{"Apache Server Status"}, Part: "body"}},
		}},
	},
	{
		ID:       "swagger-ui-exposed",
		Name:     "Swagger UI exposed",
		Severity: "info",
		CWE:      "CWE-200",
		Requests: []templateRequest{{
			Method:      http.MethodGet,
			Path:        []string{"/swagger-ui.html", "/swagger/index.html", "/api-docs"},
			MatchersAny: []templateMatcher{{Type: "words", Words: []string{"swagger-ui", "Swagger UI"}, Part: "body"}},
		}},
	},
	{
		ID:       "backup-archive-exposed",
		Name:     "Backup archive exposed",
		Severity: "high",
		CWE:      "CWE-530",
		Requests: []templateRequest{{
			Method: http.MethodGet,
			Path:   []string{"/backup.zip", "/backup.tar.gz", "/db.sql"},
			MatchersAll: []templateMatcher{
				{Type: "status", Status: []int{200}},
				{Type: "regex", Regex: []string{`(?m)^(-- MySQL dump|PK\x03\x04|\x1f\x8b)`}, Part: "body"},
			},
		}},
	},
	{
		ID:       "security-headers-missing",
		Name:     "Missing security headers",
		Severity: "info",
		CWE:      "CWE-693",
		Requests: []templateRequest{{
			Method: http.MethodGet,
			Path:   []string{"/"},
			MatchersAll: []templateMatcher{
				{Type: "status", Status: []int{200}},
				{
					Type:     "regex",
					Negative: true,
					Part:     "header",
					Regex:    []string{`(?i)x-content-type-options:`},
				},
			},
		}},
	},
	{
		ID:       "cors-wildcard",
		Name:     "CORS wildcard origin allowed",
		Severity: "low",
		CWE:      "CWE-942",
		Requests: []templateRequest{{
			Method:  http.MethodGet,
			Path:    []string{"/"},
			Headers: map[string]string{"Origin": "https://evil.example"},
			MatchersAll: []templateMatcher{
				{Type: "status", Status: []int{200}},
				{Type: "regex", Regex: []string{`(?im)^access-control-allow-origin:\s*\*`}, Part: "header"},
			},
		}},
	},
	{
		ID:       "open-redirect",
		Name:     "Open redirect via return parameter",
		Severity: "medium",
		CWE:      "CWE-601",
		Requests: []templateRequest{{
			Method: http.MethodGet,
			Path:   []string{"/?url=https://evil.example&next=https://evil.example&redirect=https://evil.example&return=https://evil.example"},
			MatchersAll: []templateMatcher{
				{Type: "status", Status: []int{301, 302, 303, 307, 308}},
				{Type: "regex", Regex: []string{`(?im)^location:\s*https?://(www\.)?evil\.example`}, Part: "header"},
			},
		}},
	},
	{
		ID:       "crlf-injection",
		Name:     "CRLF injection in redirect header",
		Severity: "medium",
		CWE:      "CWE-93",
		Requests: []templateRequest{{
			Method: http.MethodGet,
			Path:   []string{"/?redirect=%0d%0aX-Ouro-Injected:%201"},
			MatchersAll: []templateMatcher{
				{Type: "regex", Regex: []string{`(?im)^x-ouro-injected:`}, Part: "header"},
			},
		}},
	},
	{
		ID:       "path-traversal",
		Name:     "Path traversal reads system files",
		Severity: "high",
		CWE:      "CWE-22",
		Requests: []templateRequest{{
			Method: http.MethodGet,
			Path:   []string{"/?file=../../../../etc/passwd", "/?page=..%2f..%2f..%2f..%2fetc%2fpasswd"},
			MatchersAll: []templateMatcher{
				{Type: "status", Status: []int{200}},
				{Type: "regex", Regex: []string{`root:[x*!]:0:0:`}, Part: "body"},
			},
		}},
	},
	{
		ID:       "host-header-injection",
		Name:     "Host header reflected in response",
		Severity: "low",
		CWE:      "CWE-74",
		Requests: []templateRequest{{
			Method:  http.MethodGet,
			Path:    []string{"/"},
			Headers: map[string]string{"X-Forwarded-Host": "ouroboros-hostprobe"},
			MatchersAll: []templateMatcher{
				{Type: "status", Status: []int{200}},
				{Type: "word", Words: []string{"ouroboros-hostprobe"}, Part: "all"},
			},
		}},
	},
	{
		ID:       "put-method-enabled",
		Name:     "HTTP PUT method enabled",
		Severity: "medium",
		CWE:      "CWE-650",
		Requests: []templateRequest{{
			Method: http.MethodPut,
			Path:   []string{"/ouro-probe-put.txt"},
			Body:   "ouroboros-put-probe",
			MatchersAll: []templateMatcher{
				{Type: "status", Status: []int{201, 204}},
			},
		}},
	},
	{
		ID:       "trace-method-enabled",
		Name:     "HTTP TRACE method enabled (XST)",
		Severity: "low",
		CWE:      "CWE-200",
		Requests: []templateRequest{{
			Method: "TRACE",
			Path:   []string{"/"},
			MatchersAll: []templateMatcher{
				{Type: "status", Status: []int{200}},
				{Type: "regex", Regex: []string{`(?im)^X-Ouro-Trace:`}, Part: "all"},
			},
		}},
	},
	{
		ID:       "ssrf-url-parameter",
		Name:     "SSRF via URL parameter (out-of-band)",
		Severity: "high",
		CWE:      "CWE-918",
		Requests: []templateRequest{{
			Method:   http.MethodGet,
			Path:     []string{"/?url={{interacturl}}&dest={{interacturl}}&redirect={{interacturl}}&image={{interacturl}}&fetch={{interacturl}}"},
			Interact: true,
			MatchersAll: []templateMatcher{
				{Type: "status", Status: []int{200, 201, 204, 301, 302, 303, 307, 308}},
			},
		}},
	},
	{
		ID:       "xxe-xml-parameter",
		Name:     "XXE via XML entity (out-of-band)",
		Severity: "high",
		CWE:      "CWE-611",
		Requests: []templateRequest{{
			Method:      http.MethodPost,
			Path:        []string{"/", "/api/xml", "/xmlrpc.php"},
			ContentType: "application/xml",
			Body: `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE foo [<!ENTITY xxe SYSTEM "{{interacturl}}">]>
<root><data>&xxe;</data></root>`,
			Interact: true,
			MatchersAll: []templateMatcher{
				{Type: "status", Status: []int{200, 201, 204, 400, 500}},
			},
		}},
	},
	{
		ID:       "file-upload-form",
		Name:     "File upload endpoint accepts scripts",
		Severity: "high",
		CWE:      "CWE-434",
		Requests: []templateRequest{{
			Method:      http.MethodPost,
			Path:        []string{"/upload", "/file/upload", "/api/upload"},
			ContentType: "multipart/form-data; boundary=OUROBOUNDARY",
			Body: "--OUROBOUNDARY\r\n" +
				"Content-Disposition: form-data; name=\"file\"; filename=\"ouro-probe.aspx\"\r\n" +
				"Content-Type: application/octet-stream\r\n\r\n" +
				"<%@ Page Language=\"C#\" %>OUROUPLOADPROBE\r\n" +
				"--OUROBOUNDARY--\r\n",
			MatchersAll: []templateMatcher{
				{Type: "status", Status: []int{200, 201}},
				{Type: "word", Words: []string{"upload", "file", "saved", "success"}, Condition: "or", Part: "all"},
			},
		}},
	},
}

// ScanTemplates runs the embedded rule set plus any external templates
// (OURO_TEMPLATES_DIR) against the target and emits nuclei-compatible
// JSONL. The engine (template_engine.go) executes the full matcher and
// extractor language; code/network templates stay with the managed bundle.
func ScanTemplates(ctx context.Context, target string, opts Options) (string, error) {
	base := normalizeURL(target)
	if base == "" {
		return "", fmt.Errorf("template_scan requires a target url")
	}
	parsed, err := url.Parse(base)
	if err != nil {
		return "", fmt.Errorf("parse template target: %w", err)
	}

	engine := &templateEngine{
		client:       opts.httpClientPreserveRedirects(),
		followClient: opts.httpClient(),
		target:       parsed,
		timeout:      opts.timeout(),
	}
	scanCtx, cancel := context.WithTimeout(ctx, opts.timeout())
	defer cancel()

	reported := []templateReportedExt{}
	for i := range builtInTemplates {
		if err := scanCtx.Err(); err != nil {
			return "", err
		}
		reported = append(reported, engine.runTemplate(scanCtx, builtinToEngine(&builtInTemplates[i]))...)
	}
	external, _ := loadExternalTemplates()
	for i := range external {
		if err := scanCtx.Err(); err != nil {
			return "", err
		}
		reported = append(reported, engine.runTemplate(scanCtx, external[i])...)
	}
	return reportAsNucleiRecords(reported), nil
}

// builtinToEngine adapts a built-in template into the engine model.
func builtinToEngine(tmpl *builtInTemplate) engineTemplate {
	converted := engineTemplate{
		ID:          tmpl.ID,
		Name:        tmpl.Name,
		Severity:    tmpl.Severity,
		Description: tmpl.Name,
		Requests:    make([]templateRequest, len(tmpl.Requests)),
	}
	if tmpl.CWE != "" {
		converted.CWE = []string{tmpl.CWE}
	}
	for i := range tmpl.Requests {
		request := tmpl.Requests[i]
		request.Paths = request.Path
		if request.Method == "" {
			request.Method = http.MethodGet
		}
		converted.Requests[i] = request
	}
	return converted
}

func headerString(resp *http.Response) string {
	builder := bytes.Buffer{}
	keys := make([]string, 0, len(resp.Header))
	for key := range resp.Header {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		for _, value := range resp.Header[key] {
			builder.WriteString(key)
			builder.WriteString(": ")
			builder.WriteString(value)
			builder.WriteString("\n")
		}
	}
	return builder.String()
}
