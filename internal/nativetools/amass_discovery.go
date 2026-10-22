package nativetools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
)

// amassRecord mirrors the amass JSONL fields the parser reads
// (internal/app/amass_parser.go: name/host/subdomain → subdomain facts).
type amassRecord struct {
	Name   string `json:"name"`
	Domain string `json:"domain,omitempty"`
}

// amassCTRecord is the crt.sh JSON record shape (name_value lists names).
type amassCTRecord struct {
	NameValue string `json:"name_value"`
}

// DiscoverSubdomainsAmass queries the certificate-transparency passive source
// and emits amass-compatible JSONL. It reuses the same passive source as the
// subfinder replacement; the managed amass remains the reference for its full
// source list, DNS enumeration and graph mode.
func DiscoverSubdomainsAmass(ctx context.Context, target string, opts Options) (string, error) {
	domain := strings.TrimSpace(strings.TrimSuffix(target, "."))
	if domain == "" {
		return "", fmt.Errorf("subdomain_discovery (amass) requires a domain target")
	}

	client := opts.httpClient()
	probeCtx, cancel := context.WithTimeout(ctx, opts.timeout())
	defer cancel()

	endpoint := fmt.Sprintf(ctEndpoint(), url.QueryEscape(domain))
	req, err := http.NewRequestWithContext(probeCtx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", fmt.Errorf("build amass request: %w", err)
	}
	req.Header.Set("User-Agent", "ouroboros-native-amass/1")

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("query passive source: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("passive source returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1024*1024))
	if err != nil {
		return "", fmt.Errorf("read passive source response: %w", err)
	}

	var ctRecords []amassCTRecord
	if err := json.Unmarshal(body, &ctRecords); err != nil {
		return "", fmt.Errorf("decode certificate transparency response: %w", err)
	}

	seen := map[string]bool{}
	hosts := []string{}
	for _, ctRecord := range ctRecords {
		for _, raw := range strings.Split(ctRecord.NameValue, "\n") {
			name := strings.ToLower(strings.TrimSpace(raw))
			name = strings.TrimSuffix(name, ".")
			if name == "" || seen[name] {
				continue
			}
			// Only report names within the target domain, matching the parser's
			// scope filter (subfinderDNSNameWithinScope).
			if name != domain && !strings.HasSuffix(name, "."+domain) {
				continue
			}
			seen[name] = true
			hosts = append(hosts, name)
		}
	}
	sort.Strings(hosts)

	builder := strings.Builder{}
	for _, host := range hosts {
		record := amassRecord{Name: host, Domain: domain}
		encoded, err := json.Marshal(record)
		if err != nil {
			return "", fmt.Errorf("encode amass record: %w", err)
		}
		builder.Write(encoded)
		builder.WriteByte('\n')
	}
	return strings.TrimRight(builder.String(), "\n"), nil
}
