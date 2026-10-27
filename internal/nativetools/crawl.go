package nativetools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"sync"
)

// OpCrawl is the katana-equivalent (static link extraction) operation.
const OpCrawl = "crawl"

// crawlRouteLimit caps a native crawl so a hostile site cannot exhaust memory.
// It reads OURO_CRAWL_LIMIT at call time (default 256; see env.go) so an
// operator or test can change it without restarting the process.

// katanaRecord mirrors the katana JSONL fields the katana parser reads
// (internal/app/katana_parser.go: url|endpoint, method, depth, response.status_code).
type katanaRecord struct {
	URL      string `json:"url"`
	Method   string `json:"method"`
	Depth    int    `json:"depth"`
	Response struct {
		StatusCode int `json:"status_code"`
	} `json:"response"`
}

var htmlLinkPattern = regexp.MustCompile(`(?i)(?:href|src|action)\s*=\s*["']([^"'#]+)["']`)

// metaRefreshPattern captures <meta http-equiv="refresh" content="N; url=...">;
// the URL lives inside the content attribute so htmlLinkPattern cannot see it.
var metaRefreshPattern = regexp.MustCompile(`(?i)<meta[^>]*http-equiv\s*=\s*["']?refresh["']?[^>]*content\s*=\s*["'][^"']*url=([^"'>]+)["']`)

// htmlCommentPattern captures path-like tokens inside HTML comments
// (<!-- /backup/admin -->, <!-- old: /api/v1/legacy -->).
var htmlCommentPattern = regexp.MustCompile(`(?i)(?:/[a-z0-9_\-.\%@]{2,}){2,}`)

// htmlCommentBlockPattern isolates <!-- ... --> bodies so comment path
// extraction only sees tokens the site itself left inside comments.
var htmlCommentBlockPattern = regexp.MustCompile(`(?s)<!--(.*?)-->`)

// jsRoutePattern extracts path-like string literals from JavaScript bodies:
// quoted "/api/..." routes, '/static/...' assets and fetch/axios URL arguments.
var jsRoutePattern = regexp.MustCompile(`(?i)(?:fetch|axios(?:\.(?:get|post|put|delete|patch))?|url)\s*[:(]\s*["` + "`" + `]([^"` + "`" + `]+)["` + "`" + `]`)

// Crawl performs a breadth-first static crawl limited to the target origin
// and emits katana-compatible JSONL. It mines HTML attributes, meta-refresh
// targets, HTML comment paths and JavaScript string literals for route
// candidates; executing JavaScript is out of scope and the managed katana
// remains available for rendered crawling.
func Crawl(ctx context.Context, target string, opts Options) (string, error) {
	start := normalizeURL(target)
	if start == "" {
		return "", errors.New("crawl requires a target url")
	}
	parsed, err := url.Parse(start)
	if err != nil {
		return "", fmt.Errorf("parse crawl target: %w", err)
	}
	// sameOrigin rejects prefix tricks (https://target.com.evil.com/): the
	// link must parse to the identical scheme+host, not merely start with
	// an origin string prefix.
	sameOrigin := func(link string) bool {
		parsedLink, err := url.Parse(link)
		if err != nil {
			return false
		}
		return parsedLink.Scheme == parsed.Scheme && parsedLink.Host == parsed.Host
	}

	client := opts.httpClient()
	crawlCtx, cancel := context.WithTimeout(ctx, opts.timeout())
	defer cancel()

	const maxDepth = 2
	type queued struct {
		url   string
		depth int
	}
	queue := []queued{{url: start, depth: 0}}
	seen := map[string]bool{start: true}
	records := []katanaRecord{}
	mu := sync.Mutex{}
	sem := make(chan struct{}, opts.maxWorkers())
	var wg sync.WaitGroup

	for len(queue) > 0 && len(records) < crawlRouteLimit() {
		batch := queue
		queue = nil
		for _, item := range batch {
			if item.depth > maxDepth {
				continue
			}
			wg.Add(1)
			go func(item queued) {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()

				body, status, contentType, err := fetchForCrawl(crawlCtx, client, item.url)
				if err != nil {
					return
				}
				if status >= 400 {
					// katana semantics: only successfully fetched routes are
					// recorded. 4xx/5xx bodies became ghost-route facts on
					// the blackboard (404 pages parsed as surfaces).
					return
				}
				record := katanaRecord{URL: item.url, Method: http.MethodGet, Depth: item.depth}
				record.Response.StatusCode = status

				mu.Lock()
				defer mu.Unlock()
				if len(records) >= crawlRouteLimit() {
					return
				}
				records = append(records, record)
				lowerBody := strings.ToLower(body)
				isHTML := strings.Contains(lowerBody, "text/html") || strings.Contains(body, "<html") || strings.Contains(body, "<a ")
				isJS := isJSBody(contentType, item.url)
				if !isHTML && !isJS {
					return
				}
				for _, link := range extractLinks(body, item.url) {
					if !sameOrigin(link) || seen[link] {
						continue
					}
					seen[link] = true
					queue = append(queue, queued{url: link, depth: item.depth + 1})
				}
				if isHTML {
					for _, link := range extractMetaRefreshLinks(body, item.url) {
						if !sameOrigin(link) || seen[link] {
							continue
						}
						seen[link] = true
						queue = append(queue, queued{url: link, depth: item.depth + 1})
					}
					for _, link := range extractCommentPaths(body, item.url) {
						if !sameOrigin(link) || seen[link] {
							continue
						}
						seen[link] = true
						queue = append(queue, queued{url: link, depth: item.depth + 1})
					}
				}
				if isJS {
					for _, link := range extractJSRoutes(body, item.url) {
						if !sameOrigin(link) || seen[link] {
							continue
						}
						seen[link] = true
						queue = append(queue, queued{url: link, depth: item.depth + 1})
					}
				}
			}(item)
		}
		wg.Wait()
	}

	sort.Slice(records, func(i, j int) bool { return records[i].URL < records[j].URL })
	builder := strings.Builder{}
	for _, record := range records {
		encoded, err := json.Marshal(record)
		if err != nil {
			return "", fmt.Errorf("encode crawl record: %w", err)
		}
		builder.Write(encoded)
		builder.WriteByte('\n')
	}
	return strings.TrimRight(builder.String(), "\n"), nil
}

func fetchForCrawl(ctx context.Context, client *http.Client, target string) (string, int, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return "", 0, "", err
	}
	req.Header.Set("User-Agent", "ouroboros-native-katana/1")
	resp, err := client.Do(req)
	if err != nil {
		return "", 0, "", err
	}
	defer func() { _ = resp.Body.Close() }()
	contentType := resp.Header.Get("Content-Type")
	if resp.StatusCode >= 400 {
		return "", resp.StatusCode, contentType, nil
	}
	body := make([]byte, 0, 4096)
	buffer := make([]byte, 4096)
	for {
		n, err := resp.Body.Read(buffer)
		if n > 0 {
			body = append(body, buffer[:n]...)
		}
		if err != nil || len(body) > 256*1024 {
			break
		}
	}
	return string(body), resp.StatusCode, contentType, nil
}

// extractLinks resolves href/src/action values against the page URL.
func extractLinks(body, pageURL string) []string {
	base, err := url.Parse(pageURL)
	if err != nil {
		return nil
	}
	links := []string{}
	for _, match := range htmlLinkPattern.FindAllStringSubmatch(body, -1) {
		raw := strings.TrimSpace(match[1])
		if raw == "" || strings.HasPrefix(raw, "javascript:") || strings.HasPrefix(raw, "mailto:") {
			continue
		}
		reference, err := url.Parse(raw)
		if err != nil {
			continue
		}
		resolved := base.ResolveReference(reference)
		if resolved.Scheme != "http" && resolved.Scheme != "https" {
			continue
		}
		resolved.Fragment = ""
		links = append(links, resolved.String())
	}
	return links
}

// extractMetaRefreshLinks pulls redirect targets out of
// <meta http-equiv="refresh" content="N; url=/next"> tags.
func extractMetaRefreshLinks(body, pageURL string) []string {
	base, err := url.Parse(pageURL)
	if err != nil {
		return nil
	}
	links := []string{}
	for _, match := range metaRefreshPattern.FindAllStringSubmatch(body, -1) {
		raw := strings.TrimSpace(match[1])
		if raw == "" {
			continue
		}
		reference, err := url.Parse(raw)
		if err != nil {
			continue
		}
		resolved := base.ResolveReference(reference)
		if resolved.Scheme != "http" && resolved.Scheme != "https" {
			continue
		}
		resolved.Fragment = ""
		links = append(links, resolved.String())
	}
	return links
}

// extractCommentPaths mines path-like tokens (two or more /segments) out of
// HTML comments; developers routinely leave staging routes behind.
func extractCommentPaths(body, pageURL string) []string {
	base, err := url.Parse(pageURL)
	if err != nil {
		return nil
	}
	links := []string{}
	for _, block := range htmlCommentBlockPattern.FindAllStringSubmatch(body, -1) {
		for _, raw := range htmlCommentPattern.FindAllString(block[1], -1) {
			raw = strings.TrimSpace(raw)
			if raw == "" {
				continue
			}
			reference, err := url.Parse(raw)
			if err != nil {
				continue
			}
			resolved := base.ResolveReference(reference)
			if resolved.Scheme != "http" && resolved.Scheme != "https" {
				continue
			}
			resolved.Fragment = ""
			links = append(links, resolved.String())
		}
	}
	return links
}

// extractJSRoutes resolves string literals from fetch/axios/url= call sites
// and quoted paths inside JavaScript bodies against the script URL.
func extractJSRoutes(body, pageURL string) []string {
	base, err := url.Parse(pageURL)
	if err != nil {
		return nil
	}
	links := []string{}
	for _, match := range jsRoutePattern.FindAllStringSubmatch(body, -1) {
		raw := strings.TrimSpace(match[1])
		if raw == "" || strings.HasPrefix(raw, "//") || !strings.Contains(raw, "/") {
			continue
		}
		if strings.HasPrefix(raw, "javascript:") || strings.HasPrefix(raw, "mailto:") || strings.HasPrefix(raw, "data:") {
			continue
		}
		reference, err := url.Parse(raw)
		if err != nil {
			continue
		}
		resolved := base.ResolveReference(reference)
		if resolved.Scheme != "http" && resolved.Scheme != "https" {
			continue
		}
		resolved.Fragment = ""
		links = append(links, resolved.String())
	}
	return links
}

// isJSBody reports whether a fetched payload should be mined for JavaScript
// route strings: an explicit JS content type or a .js URL path.
func isJSBody(contentType, pageURL string) bool {
	lowered := strings.ToLower(contentType)
	if strings.Contains(lowered, "application/javascript") || strings.Contains(lowered, "text/javascript") ||
		strings.Contains(lowered, "application/x-javascript") {
		return true
	}
	parsed, err := url.Parse(pageURL)
	if err != nil {
		return false
	}
	return strings.HasSuffix(strings.ToLower(parsed.Path), ".js")
}
