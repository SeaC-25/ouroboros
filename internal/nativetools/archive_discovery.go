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
)

// OpArchiveDiscovery is the gau/waybackurls-equivalent operation. Both tools
// share the same parser (url_list_text), so one handler covers both.
const OpArchiveDiscovery = "archive_discovery"

// The endpoint (OURO_CDX_ENDPOINT) and result cap (OURO_ARCHIVE_LIMIT) are
// read at call time through env.go.

// DiscoverArchiveURLs queries the Internet Archive CDX API and emits
// url_list_text output: one URL per line, de-duplicated and in-scope.
// Targets may carry several comma-separated domains; each is queried in
// turn and the merged result is de-duplicated across domains.
func DiscoverArchiveURLs(ctx context.Context, target string, opts Options) (string, error) {
	trimmed := strings.TrimSpace(strings.TrimSuffix(target, "."))
	if trimmed == "" {
		return "", fmt.Errorf("archive_discovery requires a domain target")
	}
	domains := strings.Split(trimmed, ",")

	client := opts.httpClient()

	// Best-effort across comma-separated domains: one failing source must not
	// sink results already collected for the others. Each domain gets its own
	// timeout window; the run only fails when every domain failed.
	urls := []string{}
	var errs []error
	for _, domain := range domains {
		domain = strings.TrimSpace(strings.TrimSuffix(domain, "."))
		if domain == "" {
			continue
		}
		queryCtx, cancel := context.WithTimeout(ctx, opts.timeout())
		domainURLs, err := queryArchiveCDX(queryCtx, client, domain)
		cancel()
		if err != nil {
			errs = append(errs, fmt.Errorf("domain %s: %w", domain, err))
			continue
		}
		urls = append(urls, domainURLs...)
	}
	if len(urls) == 0 && len(errs) > 0 {
		return "", errors.Join(errs...)
	}

	// Merge results from every domain; de-duplicate and cap the combined list.
	urls = dedupeArchiveURLs(urls)
	if len(urls) > archiveLimit() {
		urls = urls[:archiveLimit()]
	}
	if len(urls) == 0 {
		return "", nil
	}
	return strings.Join(urls, "\n"), nil
}

// queryArchiveCDX queries the CDX API for one domain and returns its
// de-duplicated in-scope URLs (query strings preserved verbatim).
func queryArchiveCDX(ctx context.Context, client *http.Client, domain string) ([]string, error) {
	endpoint := fmt.Sprintf(cdxEndpointValue(), url.QueryEscape(domain+"/*"), archiveLimit())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("build archive request: %w", err)
	}
	req.Header.Set("User-Agent", "ouroboros-native-gau/1")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("query archive source: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("archive source returned status %d", resp.StatusCode)
	}

	// Read at most 1MiB, matching amass_discovery.go:78, which reads the
	// certificate-transparency passive source with the same shared ceiling so
	// every passive-lookup response is bounded identically.
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1024*1024))
	if err != nil {
		return nil, fmt.Errorf("read archive response: %w", err)
	}

	return archiveURLsFromCDX(body, domain), nil
}

// dedupeArchiveURLs removes duplicate URLs across all queried domains.
func dedupeArchiveURLs(urls []string) []string {
	seen := make(map[string]bool, len(urls))
	unique := urls[:0:0]
	for _, value := range urls {
		if seen[value] {
			continue
		}
		seen[value] = true
		unique = append(unique, value)
	}
	sort.Strings(unique)
	return unique
}

// archiveURLsFromCDX parses the CDX JSON (array of arrays, first row is the
// header) and keeps in-scope http(s) URLs.
func archiveURLsFromCDX(body []byte, domain string) []string {
	var rows [][]string
	if err := json.Unmarshal(body, &rows); err != nil {
		return nil
	}
	suffix := "." + strings.ToLower(domain)
	seen := map[string]bool{}
	urls := []string{}
	for index, row := range rows {
		if index == 0 || len(row) == 0 {
			continue // header row
		}
		raw := strings.TrimSpace(row[0])
		if raw == "" {
			continue
		}
		parsed, err := url.Parse(raw)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
			continue
		}
		host := strings.ToLower(parsed.Hostname())
		if host != strings.ToLower(domain) && !strings.HasSuffix(host, suffix) {
			continue
		}
		if seen[raw] {
			continue
		}
		seen[raw] = true
		urls = append(urls, raw)
	}
	sort.Strings(urls)
	return urls
}
