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

// Crawl performs a breadth-first static HTML crawl limited to the target
// origin and emits katana-compatible JSONL. JavaScript execution is out of
// scope; the managed katana remains available for rendered crawling.
func Crawl(ctx context.Context, target string, opts Options) (string, error) {
	start := normalizeURL(target)
	if start == "" {
		return "", errors.New("crawl requires a target url")
	}
	parsed, err := url.Parse(start)
	if err != nil {
		return "", fmt.Errorf("parse crawl target: %w", err)
	}
	origin := parsed.Scheme + "://" + parsed.Host

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

				body, status, err := fetchForCrawl(crawlCtx, client, item.url)
				if err != nil {
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
				if !strings.Contains(strings.ToLower(body), "text/html") && !strings.Contains(body, "<html") && !strings.Contains(body, "<a ") {
					return
				}
				for _, link := range extractLinks(body, item.url) {
					if !strings.HasPrefix(link, origin) || seen[link] {
						continue
					}
					seen[link] = true
					queue = append(queue, queued{url: link, depth: item.depth + 1})
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

func fetchForCrawl(ctx context.Context, client *http.Client, target string) (string, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return "", 0, err
	}
	req.Header.Set("User-Agent", "ouroboros-native-katana/1")
	resp, err := client.Do(req)
	if err != nil {
		return "", 0, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 400 {
		return "", resp.StatusCode, nil
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
	return string(body), resp.StatusCode, nil
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
