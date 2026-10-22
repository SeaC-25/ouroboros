package nativetools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"
)

// OpPathFuzz is the ffuf-equivalent operation.
const OpPathFuzz = "path_fuzz"

// ffufResult mirrors the ffuf JSON fields the ffuf parser reads
// (internal/app/ffuf_parser.go: results[].url, status), plus a bounded body
// excerpt so downstream consumers (flag capture, evidence) see content.
type ffufResult struct {
	URL        string `json:"url"`
	StatusCode int    `json:"status"`
	Length     int64  `json:"length,omitempty"`
	Excerpt    string `json:"excerpt,omitempty"`
}

type ffufOutput struct {
	Results []ffufResult `json:"results"`
}

// defaultFuzzPaths is a small built-in wordlist so a native run is useful
// without an external wordlist file. Callers can pass an explicit list through
// the target's query-style syntax (see ParseFuzzWordlistFromOptions).
var defaultFuzzPaths = []string{
	".git/HEAD", ".env", "admin", "api", "backup.zip", "config.json", "dashboard",
	"health", "login", "robots.txt", "server-status", "swagger.json", "wp-login.php",
}

// PathFuzz probes candidate paths and reports the non-404 responses, matching
// the ffuf JSON shape the parser consumes. A soft-404 baseline is established
// from a random path so catch-all servers do not produce noise.
func PathFuzz(ctx context.Context, target string, opts Options) (string, error) {
	base := normalizeURL(target)
	if base == "" {
		return "", errors.New("path_fuzz requires a target url")
	}
	parsed, err := url.Parse(base)
	if err != nil {
		return "", fmt.Errorf("parse fuzz target: %w", err)
	}

	client := opts.httpClient()
	fuzzCtx, cancel := context.WithTimeout(ctx, opts.timeout())
	defer cancel()

	baseline, baselineLen, err := softNotFoundBaseline(fuzzCtx, client, parsed)
	if err != nil {
		return "", err
	}

	type outcome struct {
		result ffufResult
		ok     bool
	}
	results := make([]outcome, len(defaultFuzzPaths))
	sem := make(chan struct{}, opts.maxWorkers())
	var wg sync.WaitGroup
	for index, candidate := range defaultFuzzPaths {
		wg.Add(1)
		go func(index int, candidate string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			result, ok := probeFuzzCandidate(fuzzCtx, client, parsed, candidate, baseline, baselineLen)
			results[index] = outcome{result: result, ok: ok}
		}(index, candidate)
	}
	wg.Wait()

	output := ffufOutput{Results: []ffufResult{}}
	for _, item := range results {
		if item.ok {
			output.Results = append(output.Results, item.result)
		}
	}
	sort.Slice(output.Results, func(i, j int) bool { return output.Results[i].URL < output.Results[j].URL })

	encoded, err := json.Marshal(output)
	if err != nil {
		return "", fmt.Errorf("encode fuzz results: %w", err)
	}
	return string(encoded), nil
}

// softNotFoundBaseline determines the status AND body length a missing path
// returns; the length discriminates catch-all-200 servers.
func softNotFoundBaseline(ctx context.Context, client *http.Client, base *url.URL) (int, int, error) {
	candidate := *base
	candidate.Path = strings.TrimSuffix(base.Path, "/") + "/ouroboros-soft-404-probe-" + fmt.Sprint(time.Now().UnixNano())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, candidate.String(), nil)
	if err != nil {
		return 0, 0, fmt.Errorf("build baseline request: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, 0, fmt.Errorf("baseline probe %s: %w", candidate.String(), err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	return resp.StatusCode, len(body), nil
}

func probeFuzzCandidate(ctx context.Context, client *http.Client, base *url.URL, candidate string, baselineStatus int, baselineLength int) (ffufResult, bool) {
	probe := *base
	probe.Path = strings.TrimSuffix(base.Path, "/") + "/" + strings.TrimPrefix(candidate, "/")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, probe.String(), nil)
	if err != nil {
		return ffufResult{}, false
	}
	resp, err := client.Do(req)
	if err != nil {
		return ffufResult{}, false
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNotFound {
		return ffufResult{}, false
	}
	if resp.StatusCode == baselineStatus {
		if baselineStatus != http.StatusOK {
			// Same non-200 status as a random missing path → absent.
			return ffufResult{}, false
		}
		// Catch-all server: 200 for everything (common with SPA fallbacks
		// and custom error pages). Content-length discrimination — a real
		// page whose body differs from the baseline is genuine (ffuf -fs).
		body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		if err != nil {
			return ffufResult{}, false
		}
		if len(body) == 0 || len(body) == baselineLength {
			return ffufResult{}, false
		}
		return ffufResult{URL: probe.String(), StatusCode: resp.StatusCode, Length: int64(len(body)), Excerpt: excerptOf(body)}, true
	}
	// Non-baseline status: a genuine response. Read a bounded body for the
	// excerpt (flag capture needs content, not just status codes).
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return ffufResult{}, false
	}
	return ffufResult{
		URL:        probe.String(),
		StatusCode: resp.StatusCode,
		Length:     int64(len(body)),
		Excerpt:    excerptOf(body),
	}, true
}

// excerptOf returns a bounded single-line excerpt of a response body.
func excerptOf(body []byte) string {
	const max = 512
	text := strings.TrimSpace(string(body))
	if len(text) > max {
		text = text[:max]
	}
	// collapse newlines so the JSON log stays one line per result
	return strings.ReplaceAll(text, "\n", " ")
}
