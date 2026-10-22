package nativetools

import (
	"os"
	"strconv"
	"sync"
)

// Project environment variables. Every externally observable knob of the
// native handlers (upstream endpoints, result caps) reads through here so an
// operator can point scans at a mirror or tighten limits without a rebuild.
// Unset variables keep the defaults below; see AGENTS.md.
const (
	// EnvCTEndpoint overrides the certificate-transparency source used by the
	// subfinder and amass handlers (format: "...?q=%%25.%s&output=json").
	EnvCTEndpoint = "OURO_CT_ENDPOINT"
	// EnvCDXEndpoint overrides the Internet Archive CDX endpoint used by the
	// gau/waybackurls handlers (format: "...url=%s...limit=%d").
	EnvCDXEndpoint = "OURO_CDX_ENDPOINT"
	// EnvArchiveLimit caps the number of archive URLs a native query returns.
	EnvArchiveLimit = "OURO_ARCHIVE_LIMIT"
	// EnvCrawlLimit caps the number of routes a native crawl visits.
	EnvCrawlLimit = "OURO_CRAWL_LIMIT"
)

// Default external endpoints. Tests override them through the same
// environment variables, so no test touches these values directly.
const (
	defaultCTEndpoint  = "https://crt.sh/?q=%%25.%s&output=json"
	defaultCDXEndpoint = "https://web.archive.org/cdx/search/cdx?url=%s&fl=original&collapse=urlkey&output=json&limit=%d"
)

// EnvTemplatesDir points the template engine at a directory of external
// nuclei-compatible YAML templates (OURO_TEMPLATES_DIR).
const EnvTemplatesDir = "OURO_TEMPLATES_DIR"

// EnvInteractURL is the interaction-callback base URL templates use for
// out-of-band checks (SSRF/XXE callbacks, OURO_INTERACT_URL).
const EnvInteractURL = "OURO_INTERACT_URL"

// Default result caps. Positive integers only; invalid or missing values
// fall back to these.
const (
	defaultArchiveResults = 2000
	defaultCrawlRoutes    = 256
)

var (
	envOnce    sync.Once
	envValues  map[string]string
	envNumbers map[string]int
)

// loadEnv reads the OURO_* environment variables once per process. Tests call
// resetEnv to re-read after setting or clearing a variable.
func loadEnv() {
	envOnce.Do(func() {
		envValues = map[string]string{
			EnvCTEndpoint:  os.Getenv(EnvCTEndpoint),
			EnvCDXEndpoint: os.Getenv(EnvCDXEndpoint),
		}
		envNumbers = map[string]int{
			EnvArchiveLimit: envInt(EnvArchiveLimit, defaultArchiveResults),
			EnvCrawlLimit:   envInt(EnvCrawlLimit, defaultCrawlRoutes),
		}
	})
}

// resetEnv re-reads the environment (tests only).
func resetEnv() {
	envOnce = sync.Once{}
	envValues = nil
	envNumbers = nil
	loadEnv()
}

// envInt reads a positive integer variable or returns fallback.
func envInt(name string, fallback int) int {
	raw := os.Getenv(name)
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}

// ctEndpoint returns the certificate-transparency endpoint.
func ctEndpoint() string {
	loadEnv()
	if endpoint := envValues[EnvCTEndpoint]; endpoint != "" {
		return endpoint
	}
	return defaultCTEndpoint
}

// cdxEndpointValue returns the Internet Archive CDX endpoint.
func cdxEndpointValue() string {
	loadEnv()
	if endpoint := envValues[EnvCDXEndpoint]; endpoint != "" {
		return endpoint
	}
	return defaultCDXEndpoint
}

// archiveLimit returns the cap on archive results.
func archiveLimit() int {
	loadEnv()
	return envNumbers[EnvArchiveLimit]
}

// crawlRouteLimit returns the cap on crawled routes.
func crawlRouteLimit() int {
	loadEnv()
	return envNumbers[EnvCrawlLimit]
}

// crawlLimit is the env_test.go name for crawlRouteLimit.
func crawlLimit() int { return crawlRouteLimit() }
