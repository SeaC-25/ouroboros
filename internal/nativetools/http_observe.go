package nativetools

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"
)

// OpHTTPObserve is the raw-observation primitive: the caller (the model,
// through the plan node) describes an exact HTTP request shape and receives
// the complete observable response — status, headers, timing, size, redirect
// chain, cookie changes, body digest, extracted forms/links, and a
// differential against an unmodified baseline request. It deliberately makes
// no vulnerability judgement; consuming the observations is the model's job.
const OpHTTPObserve = "http_observe"

// ObserveSpec is the request shape the caller wants executed. All fields are
// optional except Path; zero values mean "plain GET with no overrides".
type ObserveSpec struct {
	Method      string            `json:"method,omitempty"`
	Path        string            `json:"path"`
	Query       map[string]string `json:"query,omitempty"`
	Headers     map[string]string `json:"headers,omitempty"`
	Body        string            `json:"body,omitempty"`
	ContentType string            `json:"content_type,omitempty"`
	// Baseline requests the same method/path without query/body overrides so
	// the response diff is computed against the caller's own control.
	Baseline bool `json:"baseline,omitempty"`
}

// ObserveResult is the full observable outcome of one request.
type ObserveResult struct {
	URL string `json:"url"`
	// Request echo (what actually went out, after defaults were applied).
	Method      string            `json:"method"`
	Query       map[string]string `json:"query,omitempty"`
	Headers     map[string]string `json:"headers,omitempty"`
	ContentType string            `json:"content_type,omitempty"`
	// Raw response observations.
	Status          int                 `json:"status"`
	StatusText      string              `json:"status_text"`
	ResponseHeaders map[string][]string `json:"response_headers"`
	SetCookies      []string            `json:"set_cookies,omitempty"`
	Location        string              `json:"location,omitempty"`
	RedirectChain   []string            `json:"redirect_chain,omitempty"`
	BodySize        int                 `json:"body_size"`
	BodySHA256      string              `json:"body_sha256"`
	BodyPreview     string              `json:"body_preview,omitempty"`
	ElapsedMS       float64             `json:"elapsed_ms"`
	Server          string              `json:"server,omitempty"`
	XPoweredBy      string              `json:"x_powered_by,omitempty"`
	// Structured observations the model reasons over.
	Forms    []ObserveForm `json:"forms,omitempty"`
	Links    []string      `json:"links,omitempty"`
	JSONKeys []string      `json:"json_keys,omitempty"`
	// Diff is present when Baseline was requested and both requests
	// succeeded; it compares the probe response against the control.
	Diff *ObserveDiff `json:"diff,omitempty"`
}

// ObserveForm is one discovered HTML form.
type ObserveForm struct {
	Action string   `json:"action"`
	Method string   `json:"method,omitempty"`
	Fields []string `json:"fields,omitempty"`
}

// ObserveDiff compares probe vs control responses.
type ObserveDiff struct {
	StatusChanged  bool     `json:"status_changed"`
	ControlStatus  int      `json:"control_status"`
	SizeDelta      int      `json:"size_delta"`
	ControlSize    int      `json:"control_size"`
	BodyChanged    bool     `json:"body_changed"`
	HeaderAdded    []string `json:"header_added,omitempty"`
	HeaderRemoved  []string `json:"header_removed,omitempty"`
	ElapsedDeltaMS float64  `json:"elapsed_delta_ms"`
}

// bodyPreviewLimit bounds the body excerpt handed back.
const bodyPreviewLimit = 2048

var (
	formActionPattern = regexp.MustCompile(`(?is)<form[^>]*\baction=["']?([^"'\s>]+)["']?[^>]*>`)
	formMethodPattern = regexp.MustCompile(`(?is)<form[^>]*\bmethod=["']?([a-z]+)["']?`)
	inputNamePattern  = regexp.MustCompile(`(?is)<(?:input|textarea|select)[^>]*\bname=["']?([a-z0-9_\-.]+)["']?`)
	linkHrefPattern   = regexp.MustCompile(`(?is)<a[^>]*\bhref=["']?([^"'\s>]+)["']?`)
	jsonTopKeyPattern = regexp.MustCompile(`"([A-Za-z0-9_\-]+)"\s*:`)
)

// ObserveHTTP executes the spec against the target and returns the raw
// observations. It never classifies anything — no severity, no verdict, no
// signature matching beyond structural extraction (forms/links/JSON keys).
func ObserveHTTP(ctx context.Context, target string, spec ObserveSpec, opts Options) (string, error) {
	base := normalizeURL(target)
	if base == "" {
		return "", fmt.Errorf("http_observe requires a target url")
	}
	parsed, err := url.Parse(base)
	if err != nil {
		return "", fmt.Errorf("parse observe target: %w", err)
	}
	method := strings.ToUpper(strings.TrimSpace(spec.Method))
	if method == "" {
		method = http.MethodGet
	}

	client := opts.httpClientPreserveRedirects()
	probe, err := executeObserve(ctx, client, parsed, method, spec)
	if err != nil {
		return "", fmt.Errorf("observe probe: %w", err)
	}

	var control *ObserveResult
	if spec.Baseline && (len(spec.Query) > 0 || spec.Body != "" || len(spec.Headers) > 0) {
		controlSpec := ObserveSpec{Method: method, Path: spec.Path}
		control, err = executeObserve(ctx, client, parsed, method, controlSpec)
		if err == nil {
			probe.Diff = diffObservations(probe, control)
		}
	}

	encoded, err := json.Marshal(probe)
	if err != nil {
		return "", fmt.Errorf("encode observe result: %w", err)
	}
	return string(encoded), nil
}

// executeObserve performs one request described by spec and records every
// observable aspect of the exchange.
func executeObserve(ctx context.Context, client *http.Client, base *url.URL, method string, spec ObserveSpec) (*ObserveResult, error) {
	target := *base
	rawPath := spec.Path
	if rawPath == "" {
		// Empty spec path: observe the target URL itself — path and query
		// stay exactly as the caller supplied them (the plan-node target
		// already carries the request shape).
		target.Path = base.Path
		target.RawQuery = base.RawQuery
	} else {
		if !strings.HasPrefix(rawPath, "/") {
			rawPath = "/" + rawPath
		}
		// Spec path may carry a query: split so url.URL keeps them distinct.
		rawPathOnly, rawQuery, _ := strings.Cut(rawPath, "?")
		target.Path = strings.TrimSuffix(base.Path, "/") + rawPathOnly
		target.RawQuery = rawQuery
	}
	if len(spec.Query) > 0 {
		values := target.Query()
		keys := make([]string, 0, len(spec.Query))
		for key := range spec.Query {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			values.Set(key, spec.Query[key])
		}
		target.RawQuery = values.Encode()
	}

	var body io.Reader
	if spec.Body != "" {
		body = strings.NewReader(spec.Body)
	}
	req, err := http.NewRequestWithContext(ctx, method, target.String(), body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "ouroboros-native-observe/1")
	if spec.ContentType != "" {
		req.Header.Set("Content-Type", spec.ContentType)
	}
	headerKeys := make([]string, 0, len(spec.Headers))
	for key := range spec.Headers {
		headerKeys = append(headerKeys, key)
	}
	sort.Strings(headerKeys)
	for _, key := range headerKeys {
		req.Header.Set(key, spec.Headers[key])
	}

	started := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	elapsed := time.Since(started)
	defer func() { _ = resp.Body.Close() }()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, fmt.Errorf("read observe body: %w", err)
	}

	result := &ObserveResult{
		URL:             target.String(),
		Method:          method,
		Query:           spec.Query,
		Headers:         spec.Headers,
		ContentType:     spec.ContentType,
		Status:          resp.StatusCode,
		StatusText:      http.StatusText(resp.StatusCode),
		ResponseHeaders: resp.Header,
		SetCookies:      resp.Header.Values("Set-Cookie"),
		Location:        resp.Header.Get("Location"),
		BodySize:        len(raw),
		BodySHA256:      sha256Hex(raw),
		ElapsedMS:       float64(elapsed.Microseconds()) / 1000.0,
		Server:          resp.Header.Get("Server"),
		XPoweredBy:      resp.Header.Get("X-Powered-By"),
	}
	if preview := string(raw); preview != "" {
		if len(preview) > bodyPreviewLimit {
			preview = preview[:bodyPreviewLimit]
		}
		result.BodyPreview = preview
	}
	// True redirect chain: the client preserves 3xx, so follow Location
	// manually (cap 10 hops). Via is a proxy header — stuffing it into
	// redirect_chain misled reasoning (proxy hop count ≠ redirects).
	chain := []string{}
	if location := resp.Header.Get("Location"); location != "" && isRedirectStatus(resp.StatusCode) {
		next := location
		for hop := 0; hop < 10 && next != ""; hop++ {
			resolved := next
			if parsedLocation, err := url.Parse(next); err == nil && !parsedLocation.IsAbs() {
				resolved = base.ResolveReference(parsedLocation).String()
			}
			chain = append(chain, resolved)
			hopReq, err := http.NewRequestWithContext(ctx, http.MethodGet, resolved, nil)
			if err != nil {
				break
			}
			hopReq.Header.Set("User-Agent", "ouroboros-native-observe/1")
			hopResp, err := client.Do(hopReq)
			if err != nil {
				break
			}
			_, _ = io.Copy(io.Discard, io.LimitReader(hopResp.Body, 4<<10))
			_ = hopResp.Body.Close()
			if !isRedirectStatus(hopResp.StatusCode) {
				break
			}
			next = hopResp.Header.Get("Location")
		}
	}
	if len(chain) > 0 {
		result.RedirectChain = chain
	}
	result.Forms = observeForms(string(raw))

	result.JSONKeys = observeJSONKeys(string(raw))
	return result, nil
}

// isRedirectStatus reports whether the code is a hop-worthy 3xx.
func isRedirectStatus(code int) bool {
	switch code {
	case 301, 302, 303, 307, 308:
		return true
	}
	return false
}

// observeForms extracts <form> shapes (action/method/fields).
func observeForms(body string) []ObserveForm {
	forms := []ObserveForm{}
	for _, block := range regexp.MustCompile(`(?is)<form\b.*?</form>`).FindAllString(body, -1) {
		form := ObserveForm{}
		if m := formActionPattern.FindStringSubmatch(block); m != nil {
			form.Action = m[1]
		}
		if m := formMethodPattern.FindStringSubmatch(block); m != nil {
			form.Method = strings.ToUpper(m[1])
		}
		for _, m := range inputNamePattern.FindAllStringSubmatch(block, -1) {
			form.Fields = append(form.Fields, m[1])
		}
		forms = append(forms, form)
	}
	return forms
}

// observeLinks extracts same-document href values.
func observeLinks(body string) []string {
	seen := map[string]bool{}
	links := []string{}
	for _, m := range linkHrefPattern.FindAllStringSubmatch(body, -1) {
		href := m[1]
		if href == "" || href == "#" || seen[href] {
			continue
		}
		seen[href] = true
		links = append(links, href)
	}
	if len(links) > 200 {
		links = links[:200]
	}
	return links
}

// observeJSONKeys lists top-level keys of a JSON object body.
func observeJSONKeys(body string) []string {
	trimmed := strings.TrimSpace(body)
	if !strings.HasPrefix(trimmed, "{") || !strings.HasSuffix(trimmed, "}") {
		return nil
	}
	keys := []string{}
	seen := map[string]bool{}
	for _, m := range jsonTopKeyPattern.FindAllStringSubmatch(body, 50) {
		if !seen[m[1]] {
			seen[m[1]] = true
			keys = append(keys, m[1])
		}
	}
	return keys
}

// diffObservations compares a probe result against its control.
func diffObservations(probe, control *ObserveResult) *ObserveDiff {
	diff := &ObserveDiff{
		StatusChanged:  probe.Status != control.Status,
		ControlStatus:  control.Status,
		SizeDelta:      probe.BodySize - control.BodySize,
		ControlSize:    control.BodySize,
		BodyChanged:    probe.BodySHA256 != control.BodySHA256,
		ElapsedDeltaMS: probe.ElapsedMS - control.ElapsedMS,
	}
	probeSet := map[string]bool{}
	for key := range probe.ResponseHeaders {
		probeSet[strings.ToLower(key)] = true
	}
	controlSet := map[string]bool{}
	for key := range control.ResponseHeaders {
		controlSet[strings.ToLower(key)] = true
	}
	for key := range probeSet {
		if !controlSet[key] && key != "date" && key != "content-length" {
			diff.HeaderAdded = append(diff.HeaderAdded, key)
		}
	}
	for key := range controlSet {
		if !probeSet[key] && key != "date" && key != "content-length" {
			diff.HeaderRemoved = append(diff.HeaderRemoved, key)
		}
	}
	sort.Strings(diff.HeaderAdded)
	sort.Strings(diff.HeaderRemoved)
	return diff
}

// sha256Hex hashes data for the body digest.
func sha256Hex(data []byte) string {
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}
