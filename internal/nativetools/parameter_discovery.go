package nativetools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"sync"
)

// OpParameterDiscovery is the arjun-equivalent operation.
const OpParameterDiscovery = "parameter_discovery"

// candidateParameters is the built-in probe list. It stays deliberately short
// so a native run is fast and polite; the managed arjun remains available for
// large dictionaries.
var candidateParameters = []string{
	"id", "page", "q", "query", "search", "s", "lang", "file", "path", "url",
	"redirect", "next", "callback", "debug", "admin", "token", "user", "name",
	"email", "type", "action", "view", "format", "download", "cmd", "exec",
}

// parameterBaseline captures how the endpoint responds without parameters so
// parameter-driven changes can be detected by status/body-length drift.
type parameterBaseline struct {
	Status int
	Length int64
}

// DiscoverParameters probes candidate parameter names and returns
// arjun-compatible JSON: a map of URL to discovered parameter names.
func DiscoverParameters(ctx context.Context, target string, opts Options) (string, error) {
	base := normalizeURL(target)
	if base == "" {
		return "", errors.New("parameter_discovery requires a target url")
	}
	parsed, err := url.Parse(base)
	if err != nil {
		return "", fmt.Errorf("parse parameter target: %w", err)
	}

	client := opts.httpClient()
	discoverCtx, cancel := context.WithTimeout(ctx, opts.timeout())
	defer cancel()

	baseline, err := parameterBaselineFor(discoverCtx, client, parsed)
	if err != nil {
		return "", err
	}

	type outcome struct {
		name    string
		changed bool
	}
	results := make([]outcome, len(candidateParameters))
	sem := make(chan struct{}, opts.maxWorkers())
	var wg sync.WaitGroup
	for index, name := range candidateParameters {
		wg.Add(1)
		go func(index int, name string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			results[index] = outcome{name: name, changed: parameterChangesResponse(discoverCtx, client, parsed, name, baseline)}
		}(index, name)
	}
	wg.Wait()

	discovered := []string{}
	for _, item := range results {
		if item.changed {
			discovered = append(discovered, item.name)
		}
	}
	sort.Strings(discovered)

	encoded, err := json.Marshal(map[string][]string{base: discovered})
	if err != nil {
		return "", fmt.Errorf("encode parameter results: %w", err)
	}
	return string(encoded), nil
}

func parameterBaselineFor(ctx context.Context, client *http.Client, base *url.URL) (parameterBaseline, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base.String(), nil)
	if err != nil {
		return parameterBaseline{}, fmt.Errorf("build parameter baseline request: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return parameterBaseline{}, fmt.Errorf("parameter baseline %s: %w", base.String(), err)
	}
	defer func() { _ = resp.Body.Close() }()
	return parameterBaseline{Status: resp.StatusCode, Length: responseBodyLength(resp)}, nil
}

func parameterChangesResponse(ctx context.Context, client *http.Client, base *url.URL, name string, baseline parameterBaseline) bool {
	probe := *base
	values := probe.Query()
	values.Set(name, "ouroboros-param-probe")
	probe.RawQuery = values.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, probe.String(), nil)
	if err != nil {
		return false
	}
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != baseline.Status {
		return true
	}
	delta := responseBodyLength(resp) - baseline.Length
	if delta < 0 {
		delta = -delta
	}
	return delta > 16
}

// responseBodyLength reads a bounded body and returns its byte length, since
// chunked responses report ContentLength -1.
func responseBodyLength(resp *http.Response) int64 {
	buffer := make([]byte, 32*1024)
	total := int64(0)
	for total < 8*1024*1024 {
		n, err := resp.Body.Read(buffer)
		total += int64(n)
		if err != nil {
			break
		}
	}
	return total
}
