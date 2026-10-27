package nativetools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"
)

// OpSubdomainDiscovery is the subfinder-equivalent operation.
const OpSubdomainDiscovery = "subdomain_discovery"

// subfinderRecord mirrors the subfinder JSONL field the parser reads
// (internal/app/subfinder_parser.go: host|subdomain).
type subfinderRecord struct {
	Host string `json:"host"`
}

// hostnamePattern matches DNS labels that look like real hostnames in
// certificate-transparency payloads.
var hostnamePattern = regexp.MustCompile(`(?i)\b([a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?(?:\.[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?)+)\b`)

// The certificate-transparency source (crt.sh by default) is read at call
// time through OURO_CT_ENDPOINT (see env.go); tests swap it via t.Setenv.

// DiscoverSubdomains queries passive sources and emits subfinder-compatible
// JSONL. Active brute force is out of scope; the managed subfinder remains
// available for its full source list.
func DiscoverSubdomains(ctx context.Context, target string, opts Options) (string, error) {
	domain := strings.TrimSpace(strings.TrimSuffix(target, "."))
	if domain == "" {
		return "", fmt.Errorf("subdomain_discovery requires a domain target")
	}

	client := opts.httpClient()
	probeCtx, cancel := context.WithTimeout(ctx, opts.timeout())
	defer cancel()

	endpoint := fmt.Sprintf(ctEndpoint(), url.QueryEscape(domain))
	req, err := http.NewRequestWithContext(probeCtx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", fmt.Errorf("build subdomain request: %w", err)
	}
	req.Header.Set("User-Agent", "ouroboros-native-subfinder/1")

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("query passive source: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("passive source returned status %d", resp.StatusCode)
	}

	// Read at most 1MiB, matching amass_discovery.go:78, which consumes the
	// same certificate-transparency source (OURO_CT_ENDPOINT). One shared
	// ceiling keeps the two passive-subdomain readers consistent.
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1024*1024))
	if err != nil {
		return "", fmt.Errorf("read passive source response: %w", err)
	}

	hosts := subdomainsFromCertificateTransparency(body, domain)
	builder := strings.Builder{}
	for _, host := range hosts {
		encoded, err := json.Marshal(subfinderRecord{Host: host})
		if err != nil {
			return "", fmt.Errorf("encode subdomain record: %w", err)
		}
		builder.Write(encoded)
		builder.WriteByte('\n')
	}
	return strings.TrimRight(builder.String(), "\n"), nil
}

// subdomainsFromCertificateTransparency extracts in-scope hostnames from a
// crt.sh JSON payload (records carry name_value with newline-separated names).
func subdomainsFromCertificateTransparency(body []byte, domain string) []string {
	type crtRecord struct {
		NameValue string `json:"name_value"`
		CommonNam string `json:"common_name"`
	}
	var records []crtRecord
	if err := json.Unmarshal(body, &records); err != nil {
		// Some deployments return a single object; fall back to raw scanning.
		records = nil
	}

	candidates := []string{}
	for _, record := range records {
		for _, value := range strings.Split(record.NameValue, "\n") {
			candidates = append(candidates, value)
		}
		candidates = append(candidates, record.CommonNam)
	}
	if len(candidates) == 0 {
		for _, match := range hostnamePattern.FindAllString(string(body), -1) {
			candidates = append(candidates, match)
		}
	}

	suffix := "." + strings.ToLower(domain)
	seen := map[string]bool{}
	hosts := []string{}
	for _, candidate := range candidates {
		host := strings.ToLower(strings.TrimSpace(strings.TrimPrefix(candidate, "*.")))
		if host == "" || !strings.HasSuffix(host, suffix) && host != strings.ToLower(domain) {
			continue
		}
		if !hostnamePattern.MatchString(host) || seen[host] {
			continue
		}
		seen[host] = true
		hosts = append(hosts, host)
	}
	sort.Strings(hosts)
	return hosts
}

var _ = time.Second
