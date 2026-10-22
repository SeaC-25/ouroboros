package nativetools

import (
	"context"
	"encoding/json"
	"fmt"
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
func DiscoverArchiveURLs(ctx context.Context, target string, opts Options) (string, error) {
	domain := strings.TrimSpace(strings.TrimSuffix(target, "."))
	if domain == "" {
		return "", fmt.Errorf("archive_discovery requires a domain target")
	}

	client := opts.httpClient()
	queryCtx, cancel := context.WithTimeout(ctx, opts.timeout())
	defer cancel()

	endpoint := fmt.Sprintf(cdxEndpointValue(), url.QueryEscape(domain+"/*"), archiveLimit())
	req, err := http.NewRequestWithContext(queryCtx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", fmt.Errorf("build archive request: %w", err)
	}
	req.Header.Set("User-Agent", "ouroboros-native-gau/1")

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("query archive source: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("archive source returned status %d", resp.StatusCode)
	}

	body := make([]byte, 0, 64*1024)
	buffer := make([]byte, 32*1024)
	for {
		n, err := resp.Body.Read(buffer)
		if n > 0 {
			body = append(body, buffer[:n]...)
		}
		if err != nil || len(body) > 32*1024*1024 {
			break
		}
	}

	urls := archiveURLsFromCDX(body, domain)
	if len(urls) == 0 {
		return "", nil
	}
	return strings.Join(urls, "\n"), nil
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
