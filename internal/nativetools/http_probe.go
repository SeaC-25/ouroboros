// Package nativetools implements in-repo Go replacements for the external
// security binaries the toolchain packs used to shell out to.
//
// Contract: each handler emits output byte-compatible with the external tool it
// replaces, so the existing parsers in internal/app keep working untouched
// (see docs/toolchain-go-roadmap.md §5-6). Handlers never re-implement scope or
// authorization logic — callers keep using the existing gates in internal/app.
package nativetools

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// AdapterID is the authored-tool adapter that selects in-process execution.
const AdapterID = "native_go"

// Operation IDs mirror the managed_binary operation names in
// internal/toolchain/authored_registry.go.
const (
	OpHTTPProbe = "http_probe"
	OpPortScan  = "port_scan"
	OpDNSProbe  = "dns_probe"
)

// Target is the scope target a handler operates on.
type Target struct {
	Kind  string
	Value string
}

// Options carries execution knobs shared by every handler.
type Options struct {
	// Timeout bounds the whole handler run; 0 means the handler default.
	Timeout time.Duration
	// MaxWorkers bounds concurrency for scanning handlers.
	MaxWorkers int
	// HTTPClient overrides the client used by HTTP handlers (tests).
	HTTPClient *http.Client
	// ParserID is the plan node's parser, letting one operation serve several
	// interchangeable tools by emitting the matching output shape (for example
	// tls_assessment → sslscan_xml vs testssl_json, or secret_scan →
	// gitleaks_json vs trufflehog_json).
	ParserID string
}

func (o Options) timeout() time.Duration {
	if o.Timeout > 0 {
		return o.Timeout
	}
	return 30 * time.Second
}

func (o Options) maxWorkers() int {
	if o.MaxWorkers > 0 {
		return o.MaxWorkers
	}
	return 10
}

func (o Options) httpClient() *http.Client {
	if o.HTTPClient != nil {
		return o.HTTPClient
	}
	transport := &http.Transport{
		TLSClientConfig:       &tls.Config{MinVersion: tls.VersionTLS12},
		MaxIdleConns:          o.maxWorkers(),
		MaxIdleConnsPerHost:   4,
		IdleConnTimeout:       30 * time.Second,
		ResponseHeaderTimeout: o.timeout(),
	}
	return &http.Client{
		Transport: transport,
		Timeout:   o.timeout(),
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return errors.New("stopped after 5 redirects")
			}
			return nil
		},
	}
}

// httpClientPreserveRedirects returns a client that surfaces 3xx responses
// instead of following them: redirect-based detectors (open redirect, CRLF)
// must observe the redirect itself.
func (o Options) httpClientPreserveRedirects() *http.Client {
	client := o.httpClient()
	if o.HTTPClient != nil {
		return client
	}
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}
	return client
}

// Run dispatches one operation against a single target.
func Run(ctx context.Context, operation string, target Target, opts Options) (string, error) {
	switch strings.TrimSpace(operation) {
	case OpHTTPProbe:
		return HTTPProbe(ctx, target.Value, opts)
	case OpPortScan:
		return PortScan(ctx, target.Value, opts)
	case OpDNSProbe:
		if strings.TrimSpace(opts.ParserID) == "dnsrecon_json" {
			return DNSReconRecord(ctx, target.Value, opts)
		}
		return DNSProbe(ctx, target.Value, opts)
	case OpMetadataExtract:
		return ExtractMetadata(ctx, target.Value, opts)
	case OpSecretScan:
		if strings.TrimSpace(opts.ParserID) == "trufflehog_json" {
			return ScanSecretsTrufflehog(ctx, target.Value, opts)
		}
		return ScanSecrets(ctx, target.Value, opts)
	case OpPathFuzz:
		return PathFuzz(ctx, target.Value, opts)
	case OpCrawl:
		return Crawl(ctx, target.Value, opts)
	case OpParameterDiscovery:
		return DiscoverParameters(ctx, target.Value, opts)
	case OpTLSAssessment:
		if strings.TrimSpace(opts.ParserID) == "testssl_json" {
			return AssessTLSTestssl(ctx, target.Value, opts)
		}
		return AssessTLS(ctx, target.Value, opts)
	case OpSubdomainDiscovery:
		if strings.TrimSpace(opts.ParserID) == "amass_json" {
			return DiscoverSubdomainsAmass(ctx, target.Value, opts)
		}
		return DiscoverSubdomains(ctx, target.Value, opts)
	case OpArchiveDiscovery:
		return DiscoverArchiveURLs(ctx, target.Value, opts)
	case OpFirmwareAnalyze:
		return AnalyzeFirmware(ctx, target.Value, opts)
	case OpServiceFingerprint:
		return FingerprintServices(ctx, target.Value, opts)
	case OpTemplateScan:
		return ScanTemplates(ctx, target.Value, opts)
	case OpStaticCodeScan:
		if strings.TrimSpace(opts.ParserID) == "semgrep_sarif" {
			return ScanStaticCodeSemgrep(ctx, target.Value, opts)
		}
		return ScanStaticCodeMobsf(ctx, target.Value, opts)
	case OpApkDecompile:
		if strings.TrimSpace(opts.ParserID) == "apktool_text" {
			return DecompileAPKApktool(ctx, target.Value, opts)
		}
		return DecompileAPKJadx(ctx, target.Value, opts)
	case OpXSSScan:
		return ScanReflectedXSS(ctx, target.Value, opts)
	case OpAuthTest:
		return TestAuth(ctx, target.Value, opts)
	default:
		return "", fmt.Errorf("native tool operation %q is not implemented", operation)
	}
}

// httpxRecord is the subset of the httpx JSONL schema the httpx parser reads
// (internal/app/httpx_parser.go: url|input, status_code, content_type,
// content_length, final_url|location).
type httpxRecord struct {
	URL           string `json:"url"`
	Input         string `json:"input"`
	StatusCode    int    `json:"status_code"`
	ContentType   string `json:"content_type"`
	ContentLength int64  `json:"content_length"`
	FinalURL      string `json:"final_url,omitempty"`
	Title         string `json:"title,omitempty"`
	WebServer     string `json:"webserver,omitempty"`
}

// HTTPProbe performs a single HTTP(S) round trip per target and emits one
// httpx-compatible JSONL record.
func HTTPProbe(ctx context.Context, target string, opts Options) (string, error) {
	target = normalizeURL(target)
	if target == "" {
		return "", errors.New("http_probe requires a target url")
	}
	client := opts.httpClient()

	probeCtx, cancel := context.WithTimeout(ctx, opts.timeout())
	defer cancel()

	req, err := http.NewRequestWithContext(probeCtx, http.MethodGet, target, nil)
	if err != nil {
		return "", fmt.Errorf("build probe request: %w", err)
	}
	req.Header.Set("User-Agent", "ouroboros-native-httpx/1")

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("probe %s: %w", target, err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 512*1024))
	if err != nil {
		return "", fmt.Errorf("read probe response: %w", err)
	}

	record := httpxRecord{
		URL:           target,
		Input:         target,
		StatusCode:    resp.StatusCode,
		ContentType:   strings.TrimSpace(resp.Header.Get("Content-Type")),
		ContentLength: int64(len(body)),
		WebServer:     strings.TrimSpace(resp.Header.Get("Server")),
		Title:         extractHTMLTitle(body),
	}
	if resp.Request != nil && resp.Request.URL != nil {
		record.FinalURL = resp.Request.URL.String()
	}
	if resp.ContentLength >= 0 {
		record.ContentLength = resp.ContentLength
	}

	encoded, err := json.Marshal(record)
	if err != nil {
		return "", fmt.Errorf("encode probe record: %w", err)
	}
	return string(encoded), nil
}

func normalizeURL(target string) string {
	target = strings.TrimSpace(target)
	if target == "" {
		return ""
	}
	if !strings.Contains(target, "://") {
		target = "https://" + target
	}
	return target
}

// extractHTMLTitle returns the trimmed <title> text, or "" when absent.
func extractHTMLTitle(body []byte) string {
	lower := strings.ToLower(string(body))
	start := strings.Index(lower, "<title")
	if start < 0 {
		return ""
	}
	open := strings.Index(lower[start:], ">")
	if open < 0 {
		return ""
	}
	rest := body[start+open+1:]
	end := strings.Index(strings.ToLower(string(rest)), "</title>")
	if end < 0 {
		return ""
	}
	return strings.TrimSpace(string(rest[:end]))
}
