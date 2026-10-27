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

// defaultFuzzPaths is a built-in wordlist so a native run is useful without
// an external wordlist file. Callers can pass an explicit list through the
// target's query-style syntax (see ParseFuzzWordlistFromOptions).
var defaultFuzzPaths = []string{
	// Version control and editor artifacts
	".git/HEAD", ".git/config", ".git/index", ".git/logs/HEAD", ".git/description",
	".svn/entries", ".svn/wc.db", ".hg/store", ".bzr/README", ".DS_Store",
	".hg/hgrc", ".git/refs/heads/master", ".gitignore", ".gitattributes",
	".svn/all-wcprops", ".CVS/Entries", "CVS/Entries", ".github/workflows",
	// Environment, secrets and credentials
	".env", ".env.local", ".env.production", ".env.development", ".env.backup",
	".env.example", ".env.bak", ".env.old", ".env.save", ".env.dist",
	".htaccess", ".htpasswd", ".aws/credentials", ".ssh/id_rsa", ".ssh/authorized_keys",
	"id_rsa", "web.config.bak", "wp-config.php.bak", "config.php.bak", "config.inc.php",
	"credentials.json", "service-account.json", "secrets.json", "secrets.yaml",
	// Config and application files
	"config.json", "config.yaml", "config.yml", "config.php", "config.ini",
	"config.js", "configuration.json", "app.config", "web.config", "appsettings.json",
	"settings.py", "local_settings.py", "application.properties", "application.yml",
	"database.yml", "database.php", "db.php", "settings.json", "composer.json",
	"composer.lock", "package.json", "package-lock.json", "yarn.lock", "Gemfile",
	"requirements.txt", "Pipfile", "go.mod", "Makefile", "Dockerfile", "docker-compose.yml",
	"docker-compose.yaml", ".dockerfile", "Jenkinsfile", ".gitlab-ci.yml", ".travis.yml",
	"phpinfo.php", "info.php", "test.php", "admin.php", "install.php", "setup.php",
	"config.php.old", "configuration.php", "constants.php", "wp-config.php",
	// Backups and archives
	"backup.zip", "backup.tar.gz", "backup.tar", "backup.sql", "backup.rar", "backup.7z",
	"backup.old", "backup.html", "site.zip", "site.tar.gz", "www.zip", "wwwroot.zip",
	"web.zip", "web.rar", "html.zip", "dump.sql", "dump.sql.gz", "db.sql", "database.sql",
	"database.sql.gz", "db_backup.sql", "mysqldump.sql", "data.sql", "archive.zip",
	"archive.tar.gz", "src.zip", "source.zip", "code.zip", "release.zip", "dist.zip",
	"1.zip", "1.rar", "1.tar.gz", "2.zip", "old.zip", "new.zip", "temp.zip", "tmp.zip",
	"download.zip", "uploads.zip", "backup/backup.zip", "bak.zip", "bak.tar.gz",
	// Admin panels and dashboards
	"admin", "admin/", "admin/login", "admin/dashboard", "admin/index.php", "admin.html",
	"admin/admin", "admin.php", "admin/admin.php", "administrator", "adminpanel",
	"admin_panel", "adminarea", "manager", "manager/html", "dashboard", "dashboard/",
	"cpanel", "panel", "controlpanel", "wp-admin", "wp-admin/", "wp-login.php",
	"wp-admin/admin-ajax.php", "administrator/index.php", "joomla/administrator",
	"admin/login.php", "admin/login.html", "login", "login/", "login.html", "login.php",
	"signin", "signup", "register", "auth", "account", "profile", "user/login",
	// API surface
	"api", "api/", "api/v1", "api/v2", "api/v3", "api/index.php", "api/docs",
	"api/swagger", "api/swagger.json", "api/openapi.json", "api/graphql",
	"graphql", "graphiql", "swagger", "swagger/", "swagger.json", "swagger-ui",
	"swagger-ui.html", "openapi.json", "openapi.yaml", "api-docs", "docs/api",
	"rest", "rest/v1", "v1", "v2", "api/health", "api/status", "api/version",
	"api/config", "api/users", "api/user", "api/admin", "api/debug", "api/test",
	// Debug and diagnostics
	"debug", "debug/", "debugger", "actuator", "actuator/health", "actuator/env",
	"actuator/heapdump", "actuator/mappings", "actuator/beans", "actuator/loggers",
	"actuator/metrics", "actuator/configprops", "actuator/trace", "actuator/info",
	"console", "console/", "_profiler", "_profiler/", "_wdt", "trace", "status",
	"phpinfo", "server-info", "server-status", "elmah.axd", "trace.axd", "guffaw",
	".well-known/", "diagnostics", "test", "test/", "tests", "dev", "development",
	"var_dump", "dump", "info", "environment", "env", "debug/vars", "pprof",
	"debug/pprof/", "metrics", "prometheus", "healthz", "readyz", "livez",
	// Logs
	"log", "logs", "logs/", "log/", "access.log", "error.log", "error_log", "debug.log",
	"access_log", "app.log", "application.log", "php_errors.log", "combined.log",
	"nginx-error.log", "nginx-access.log", "apache.log", "messages", "syslog",
	"var/log", "log/access.log", "log/error.log", "logs/error.log", "logs/access.log",
	"tomcat/logs", "logs/catalina.out", "catalina.out", "jwt.log", "audit.log",
	// Uploads and user content
	"upload", "upload/", "uploads", "uploads/", "files", "files/", "media", "media/",
	"attachments", "static", "static/", "assets", "assets/", "public", "public/",
	"images", "images/", "img", "js", "js/", "css", "css/", "tmp", "tmp/", "temp",
	"data", "data/", "storage", "storage/", "download", "downloads", "export",
	"file", "uploads/files", "uploads/images", "media/uploads", "uploads/2024",
	// Framework and app specifics
	"wp-content", "wp-content/", "wp-content/uploads", "wp-includes", "wp-json",
	"xmlrpc.php", "readme.html", "license.txt", "readme.txt", "CHANGELOG.md",
	"sites/default/files", "misc/drupal.js", "user/login", "node/", "index.php",
	"index.html", "index.bak", "index.php.bak", "index.php~", "main.php",
	"robots.txt", "sitemap.xml", "sitemap_index.xml", "crossdomain.xml",
	"security.txt", ".well-known/security.txt", "humans.txt", "favicon.ico",
	"apple-app-site-association", ".well-known/apple-app-site-association",
	"WEB-INF/web.xml", "META-INF/MANIFEST.MF", "jenkins/", "jenkins/script",
	"solr/", "solr/admin", "elasticsearch/", "_search", "kibana/", "grafana/",
	"jmx-console/", "web-console/", "axis2/", "jmx-console/HtmlAdaptor",
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
		result      ffufResult
		contentType string
		ok          bool
	}
	// First wave over the wordlist.
	results := make([]outcome, len(defaultFuzzPaths))
	sem := make(chan struct{}, opts.maxWorkers())
	var wg sync.WaitGroup
	for index, candidate := range defaultFuzzPaths {
		wg.Add(1)
		go func(index int, candidate string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			result, contentType, ok := probeFuzzCandidate(fuzzCtx, client, parsed, candidate, baseline, baselineLen)
			results[index] = outcome{result: result, contentType: contentType, ok: ok}
		}(index, candidate)
	}
	wg.Wait()

	// Collect hits and one-level subdirectory candidates: a hit that is a
	// directory (200/301/302 with a trailing slash or an HTML response) gets
	// one more layer of the same wordlist, still bounded by the workers.
	// fuzzMaxSubdirs bounds the recursion fan-out so a wildcard-heavy target
	// does not explode into thousands of extra probes.
	const fuzzMaxSubdirs = 16
	discovered := make([]ffufResult, 0, len(results))
	subdirs := []string{}
	for _, item := range results {
		if !item.ok {
			continue
		}
		discovered = append(discovered, item.result)
		if len(subdirs) < fuzzMaxSubdirs {
			if suffix := fuzzSubdirCandidate(item.result, item.contentType); suffix != "" {
				subdirs = append(subdirs, suffix)
			}
		}
	}

	// Second wave: one recursion level over discovered directories.
	type recursionOutcome struct {
		result ffufResult
		ok     bool
	}
	for _, subdir := range subdirs {
		jobs := make([]recursionOutcome, len(defaultFuzzPaths))
		for index, candidate := range defaultFuzzPaths {
			wg.Add(1)
			go func(index int, candidate string) {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()
				result, _, ok := probeFuzzCandidate(fuzzCtx, client, parsed, subdir+candidate, baseline, baselineLen)
				jobs[index] = recursionOutcome{result: result, ok: ok}
			}(index, candidate)
		}
		wg.Wait()
		for _, item := range jobs {
			if item.ok {
				discovered = append(discovered, item.result)
			}
		}
	}

	// 403 bypass probing: headers that make origin-based rules misfire.
	for _, item := range results {
		if item.ok || item.result.StatusCode != http.StatusForbidden {
			continue
		}
		if result, ok := probeFuzz403Bypass(fuzzCtx, client, parsed, item.result.URL); ok {
			discovered = append(discovered, result)
		}
	}

	sort.Slice(discovered, func(i, j int) bool { return discovered[i].URL < discovered[j].URL })
	output := ffufOutput{Results: discovered}
	if output.Results == nil {
		output.Results = []ffufResult{}
	}

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

func probeFuzzCandidate(ctx context.Context, client *http.Client, base *url.URL, candidate string, baselineStatus int, baselineLength int) (ffufResult, string, bool) {
	probe := *base
	probe.Path = strings.TrimSuffix(base.Path, "/") + "/" + strings.TrimPrefix(candidate, "/")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, probe.String(), nil)
	if err != nil {
		return ffufResult{}, "", false
	}
	resp, err := client.Do(req)
	if err != nil {
		return ffufResult{}, "", false
	}
	defer func() { _ = resp.Body.Close() }()
	contentType := resp.Header.Get("Content-Type")

	if resp.StatusCode == http.StatusNotFound {
		return ffufResult{}, "", false
	}
	if resp.StatusCode == baselineStatus {
		if baselineStatus != http.StatusOK {
			// Same non-200 status as a random missing path → absent.
			return ffufResult{}, contentType, false
		}
		// Catch-all server: 200 for everything (common with SPA fallbacks
		// and custom error pages). Content-length discrimination — a real
		// page whose body differs from the baseline is genuine (ffuf -fs).
		body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		if err != nil {
			return ffufResult{}, contentType, false
		}
		if len(body) == 0 || len(body) == baselineLength {
			return ffufResult{}, contentType, false
		}
		return ffufResult{URL: probe.String(), StatusCode: resp.StatusCode, Length: int64(len(body)), Excerpt: excerptOf(body)}, contentType, true
	}
	// Non-baseline status: a genuine response. Read a bounded body for the
	// excerpt (flag capture needs content, not just status codes).
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return ffufResult{}, contentType, false
	}
	return ffufResult{
		URL:        probe.String(),
		StatusCode: resp.StatusCode,
		Length:     int64(len(body)),
		Excerpt:    excerptOf(body),
	}, contentType, true
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

// fuzzSubdirCandidate reports the directory prefix worth recursing into when
// a hit looks like a directory: a trailing slash, an HTML response, or a
// redirect without a file extension. Returns "" for file-like hits.
func fuzzSubdirCandidate(result ffufResult, contentType string) string {
	parsed, err := url.Parse(result.URL)
	if err != nil {
		return ""
	}
	path := parsed.Path
	if path == "" || !strings.HasPrefix(path, "/") {
		return ""
	}
	isRedirect := result.StatusCode == http.StatusMovedPermanently ||
		result.StatusCode == http.StatusFound
	isDirectory := strings.HasSuffix(path, "/") ||
		isRedirect ||
		strings.Contains(strings.ToLower(contentType), "text/html")
	if !isDirectory && result.StatusCode == http.StatusOK &&
		!strings.Contains(path[strings.LastIndex(path, "/")+1:], ".") {
		// 200 without extension and without an HTML content type: the
		// extensionless path itself is the directory signal.
		isDirectory = true
	}
	if !isDirectory {
		return ""
	}
	if !strings.HasSuffix(path, "/") {
		path += "/"
	}
	return path
}

// probeFuzz403Bypass replays a 403 path with X-Original-URL and X-Rewrite-URL
// headers pointing at the path itself; a 200 response means the access rule
// only inspected the request path, and the bypass is reported as a hit.
func probeFuzz403Bypass(ctx context.Context, client *http.Client, base *url.URL, forbiddenURL string) (ffufResult, bool) {
	parsed, err := url.Parse(forbiddenURL)
	if err != nil {
		return ffufResult{}, false
	}
	forbiddenPath := parsed.Path
	if forbiddenPath == "" {
		return ffufResult{}, false
	}
	// The bypass probe hits the same URL with rewritten-path headers; the
	// router must see the forbidden path via the header, not the URL.
	probe := *base
	probe.Path = strings.TrimSuffix(base.Path, "/") + "/"

	for _, header := range []string{"X-Original-URL", "X-Rewrite-URL"} {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, probe.String(), nil)
		if err != nil {
			return ffufResult{}, false
		}
		req.Header.Set(header, forbiddenPath)
		resp, err := client.Do(req)
		if err != nil {
			continue
		}
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		_ = resp.Body.Close()
		if readErr != nil {
			continue
		}
		if resp.StatusCode == http.StatusOK && len(body) > 0 {
			return ffufResult{
				URL:        forbiddenURL,
				StatusCode: resp.StatusCode,
				Length:     int64(len(body)),
				Excerpt:    excerptOf([]byte("[403 bypass via " + header + "] " + string(body))),
			}, true
		}
	}
	return ffufResult{}, false
}
