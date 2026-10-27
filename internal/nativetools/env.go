package nativetools

import (
	"os"
	"strconv"
)

// Project environment variables. Every externally observable knob of the
// native handlers (upstream endpoints, result caps) reads through here so an
// operator can point scans at a mirror or tighten limits without a rebuild.
// Unset variables keep the defaults below; see AGENTS.md.
//

// EnvCTEndpoint overrides the certificate-transparency endpoint.
const EnvCTEndpoint = "OURO_CT_ENDPOINT"

// EnvCDXEndpoint overrides the Internet Archive CDX endpoint.
const EnvCDXEndpoint = "OURO_CDX_ENDPOINT"

// EnvArchiveLimit caps archive results (OURO_ARCHIVE_LIMIT).
const EnvArchiveLimit = "OURO_ARCHIVE_LIMIT"

// EnvCrawlLimit caps crawled routes (OURO_CRAWL_LIMIT).
const EnvCrawlLimit = "OURO_CRAWL_LIMIT"

// Default result caps. Positive integers only; invalid or missing values
// fall back to these.
const (
	defaultArchiveResults = 2000
	defaultCrawlRoutes    = 256
)

// Default external endpoints. Tests override them through the same
// environment variables, so no test touches these values directly.

// DefaultCTEndpoint is the certificate-transparency lookup base.
const defaultCTEndpoint = "https://crt.sh/?output=json&q="

// DefaultCDXEndpoint is the Internet Archive CDX lookup base.
const defaultCDXEndpoint = "https://web.archive.org/cdx/search/cdx?url="

// EnvTemplatesDir points the template engine at a directory of external
// nuclei-compatible YAML templates (OURO_TEMPLATES_DIR).
const EnvTemplatesDir = "OURO_TEMPLATES_DIR"

// EnvInteractURL is the interaction-callback base URL templates use for
// out-of-band checks (SSRF/XXE callbacks, OURO_INTERACT_URL).
const EnvInteractURL = "OURO_INTERACT_URL"

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

// resetEnv is a no-op retained for test compatibility: getters now read the
// environment at call time, so there is nothing to reset.
func resetEnv() {}

// ctEndpoint returns the certificate-transparency endpoint.
func ctEndpoint() string {
	if endpoint := os.Getenv(EnvCTEndpoint); endpoint != "" {
		return endpoint
	}
	return defaultCTEndpoint
}

// cdxEndpointValue returns the Internet Archive CDX endpoint.
func cdxEndpointValue() string {
	if endpoint := os.Getenv(EnvCDXEndpoint); endpoint != "" {
		return endpoint
	}
	return defaultCDXEndpoint
}

// archiveLimit returns the cap on archive results.
func archiveLimit() int {
	return envInt(EnvArchiveLimit, defaultArchiveResults)
}

// crawlRouteLimit returns the cap on crawled routes.
func crawlRouteLimit() int {
	return envInt(EnvCrawlLimit, defaultCrawlRoutes)
}
