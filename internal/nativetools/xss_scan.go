package nativetools

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"sync"
)

// OpXSSScan is the dalfox-equivalent (reflection probe) operation.
const OpXSSScan = "xss_scan"

// xssMarker is the benign probe value. Nothing is executed: the check is pure
// reflection/escaping analysis on the response body.
const xssMarker = "ouroborosXSSprobe9137"

// xssContextPatterns classify how the marker was reflected.
var xssContextPatterns = []struct {
	Name     string
	Pattern  *regexp.Regexp
	Severity string
}{
	{"in_script", regexp.MustCompile(`(?i)<script[^>]*>[^<]*` + xssMarker), "HIGH"},
	{"in_attribute", regexp.MustCompile(`(?i)="[^"]*` + xssMarker), "MEDIUM"},
	{"in_html_text", regexp.MustCompile(`(?i)>[^<]*` + xssMarker), "MEDIUM"},
}

// dalfoxPoC mirrors the dalfox PoC record shape the parser reads
// (internal/app/dalfox_parser.go: type, inject_type, method, param, severity, data).
type dalfoxPoC struct {
	Type       string `json:"type"`
	InjectType string `json:"inject_type"`
	Method     string `json:"method"`
	Param      string `json:"param"`
	Severity   string `json:"severity"`
	Data       string `json:"data"`
}

type dalfoxOutput struct {
	PoCs []dalfoxPoC `json:"pocs"`
}

// candidateXSSParams are probed when baseline parameter discovery finds
// nothing (or the baseline cannot be fetched).
var candidateXSSParams = []string{"q", "s", "search", "query", "id", "name", "keyword", "callback", "redirect", "url", "next", "lang", "page", "view"}

// ScanReflectedXSS discovers candidate parameters from the target's baseline
// HTML (input names inside <form> blocks plus query keys of <a href> links)
// and falls back to the built-in list when nothing can be extracted. It then
// injects a benign marker into each candidate and reports reflections with
// their HTML context. DOM detection is a static heuristic: reflections within
// ±200 bytes of a DOM sink/source statement are reported as
// dom_sink_reflection without executing any JavaScript, so co-occurrence is
// reported as-is and false positives are expected. Payload generation for
// actual exploitation stays out of scope; the managed dalfox remains the
// reference for that.
func ScanReflectedXSS(ctx context.Context, target string, opts Options) (string, error) {
	base := normalizeURL(target)
	if base == "" {
		return "", fmt.Errorf("xss_scan requires a target url")
	}
	parsed, err := url.Parse(base)
	if err != nil {
		return "", fmt.Errorf("parse xss target: %w", err)
	}

	client := opts.httpClient()
	scanCtx, cancel := context.WithTimeout(ctx, opts.timeout())
	defer cancel()

	params := xssDiscoveredParams(xssBaselineBody(scanCtx, client, parsed))
	if len(params) == 0 {
		params = candidateXSSParams
	}

	type outcome struct {
		poc dalfoxPoC
		ok  bool
	}
	results := make([]outcome, len(params))
	sem := make(chan struct{}, opts.maxWorkers())
	var wg sync.WaitGroup
	for index, param := range params {
		wg.Add(1)
		go func(index int, param string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			poc, ok := probeXSSParam(scanCtx, client, parsed, param)
			results[index] = outcome{poc: poc, ok: ok}
		}(index, param)
	}
	wg.Wait()

	output := dalfoxOutput{PoCs: []dalfoxPoC{}}
	for _, item := range results {
		if item.ok {
			output.PoCs = append(output.PoCs, item.poc)
		}
	}
	sort.Slice(output.PoCs, func(i, j int) bool { return output.PoCs[i].Param < output.PoCs[j].Param })

	encoded, err := json.Marshal(output)
	if err != nil {
		return "", fmt.Errorf("encode xss findings: %w", err)
	}
	return string(encoded), nil
}

func probeXSSParam(ctx context.Context, client *http.Client, base *url.URL, param string) (dalfoxPoC, bool) {
	probe := *base
	values := probe.Query()
	values.Set(param, xssMarker)
	probe.RawQuery = values.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, probe.String(), nil)
	if err != nil {
		return dalfoxPoC{}, false
	}
	req.Header.Set("User-Agent", "ouroboros-native-dalfox/1")
	resp, err := client.Do(req)
	if err != nil {
		return dalfoxPoC{}, false
	}
	defer func() { _ = resp.Body.Close() }()

	body := make([]byte, 0, 8192)
	buffer := make([]byte, 8192)
	for len(body) < 512*1024 {
		n, err := resp.Body.Read(buffer)
		if n > 0 {
			body = append(body, buffer[:n]...)
		}
		if err != nil {
			break
		}
	}
	text := string(body)
	if !strings.Contains(text, xssMarker) {
		return dalfoxPoC{}, false
	}

	severity, injectType := "LOW", "in_html_comment"
	for _, candidate := range xssContextPatterns {
		if candidate.Pattern.MatchString(text) {
			severity, injectType = candidate.Severity, candidate.Name
			break
		}
	}
	if xssDOMReflection(text) {
		severity, injectType = "HIGH", "dom_sink_reflection"
	}
	return dalfoxPoC{
		Type:       "REFLECTED",
		InjectType: injectType,
		Method:     http.MethodGet,
		Param:      param,
		Severity:   severity,
		Data:       probe.String(),
	}, true
}

// xssBaselineBody fetches the target page without parameters and returns its
// body, trimmed to the same 512 KiB cap the probe uses. Errors are swallowed:
// parameter discovery is best-effort and the caller falls back to the built-in
// candidate list.
func xssBaselineBody(ctx context.Context, client *http.Client, base *url.URL) string {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base.String(), nil)
	if err != nil {
		return ""
	}
	req.Header.Set("User-Agent", "ouroboros-native-dalfox/1")
	resp, err := client.Do(req)
	if err != nil {
		return ""
	}
	defer func() { _ = resp.Body.Close() }()

	body := make([]byte, 0, 8192)
	buffer := make([]byte, 8192)
	for len(body) < 512*1024 {
		n, err := resp.Body.Read(buffer)
		if n > 0 {
			body = append(body, buffer[:n]...)
		}
		if err != nil {
			break
		}
	}
	return string(body)
}

// xssFormBlockPattern isolates <form ...>...</form> blocks so <input> names
// outside forms do not pollute the candidate set.
var xssFormBlockPattern = regexp.MustCompile(`(?is)<form\b.*?</form\s*>`)

// xssInputNamePattern captures <input name="..."> (or name='...' / name=x).
var xssInputNamePattern = regexp.MustCompile(`(?i)<input\b[^>]*?\bname\s*=\s*("([^"]*)"|'([^']*)'|([^\s"'>]+))`)

// xssHrefQueryPattern captures the query string of <a href="...?..."> links.
var xssHrefQueryPattern = regexp.MustCompile(`(?i)<a\b[^>]*?\bhref\s*=\s*("([^"]*)"|'([^']*)'|([^\s"'>]+))`)

// xssDiscoveredParams extracts candidate parameter names from baseline HTML:
// input names inside <form> blocks and query keys of <a href> links. Duplicates
// are removed while keeping first-seen order.
func xssDiscoveredParams(body string) []string {
	if strings.TrimSpace(body) == "" {
		return nil
	}
	seen := make(map[string]struct{})
	params := []string{}

	add := func(name string) {
		name = strings.TrimSpace(name)
		if name == "" {
			return
		}
		if _, exists := seen[name]; exists {
			return
		}
		seen[name] = struct{}{}
		params = append(params, name)
	}

	for _, block := range xssFormBlockPattern.FindAllString(body, -1) {
		for _, match := range xssInputNamePattern.FindAllStringSubmatch(block, -1) {
			if name := match[2]; name != "" {
				add(name)
			} else if name := match[3]; name != "" {
				add(name)
			} else {
				add(match[4])
			}
		}
	}
	for _, match := range xssHrefQueryPattern.FindAllStringSubmatch(body, -1) {
		href := match[2]
		if href == "" {
			href = match[3]
		}
		if href == "" {
			href = match[4]
		}
		if parsed, err := url.Parse(href); err == nil {
			for key := range parsed.Query() {
				add(key)
			}
			continue
		}
		// Fallback for hrefs url.Parse rejects: split on '?' manually.
		if question := strings.Index(href, "?"); question >= 0 {
			for _, pair := range strings.Split(href[question+1:], "&") {
				if key := strings.SplitN(pair, "=", 2)[0]; key != "" {
					add(key)
				}
			}
		}
	}
	return params
}

// xssDOMSinkPattern matches statements that assign or feed DOM XSS
// sources/sinks (document.write, innerHTML, eval, location.href/href/search
// assignments). It deliberately over-approximates; see the ScanReflectedXSS
// doc comment.
var xssDOMSinkPattern = regexp.MustCompile(`(?i)(document\.write|innerHTML|eval\s*\(|location\.href|location\.search)`)

// xssDOMWindow is how close a reflection must sit to a sink to be reported.
const xssDOMWindow = 200

// xssDOMReflection reports whether the marker appears within xssDOMWindow
// bytes of a DOM sink/source statement in the response body.
func xssDOMReflection(body string) bool {
	sinks := xssDOMSinkPattern.FindAllStringIndex(body, -1)
	if len(sinks) == 0 {
		return false
	}
	offset := 0
	for {
		index := strings.Index(body[offset:], xssMarker)
		if index < 0 {
			return false
		}
		markerStart := offset + index
		markerEnd := markerStart + len(xssMarker)
		for _, span := range sinks {
			// Sinks are collected left-to-right, so once a sink starts after
			// the window's end no later sink can be inside either window half.
			if span[0] > markerEnd+xssDOMWindow {
				break
			}
			// Skip sinks that end before the window's start.
			if span[1] < markerStart-xssDOMWindow {
				continue
			}
			return true
		}
		offset = markerEnd
	}
}
