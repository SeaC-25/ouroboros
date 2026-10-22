package nativetools

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"strings"
)

// dnsxRecord mirrors the dnsx JSONL fields the dnsx parser reads
// (internal/app/dnsx_parser.go: host|input, a/aaaa/cname arrays).
type dnsxRecord struct {
	Host  string   `json:"host"`
	A     []string `json:"a,omitempty"`
	AAAA  []string `json:"aaaa,omitempty"`
	CNAME []string `json:"cname,omitempty"`
}

// DNSProbe resolves A/AAAA/CNAME records for a domain and emits
// dnsx-compatible JSONL. Resolution failures produce no record (like dnsx).
func DNSProbe(ctx context.Context, target string, opts Options) (string, error) {
	host := strings.TrimSpace(target)
	if host == "" {
		return "", fmt.Errorf("dns_probe requires a domain target")
	}
	host = strings.TrimSuffix(host, ".")

	probeCtx, cancel := context.WithTimeout(ctx, opts.timeout())
	defer cancel()

	record := dnsxRecord{Host: host}
	resolver := net.Resolver{}

	if addrs, err := resolver.LookupHost(probeCtx, host); err == nil {
		for _, addr := range addrs {
			parsed := net.ParseIP(addr)
			switch {
			case parsed == nil:
				continue
			case parsed.To4() != nil:
				record.A = append(record.A, addr)
			default:
				record.AAAA = append(record.AAAA, addr)
			}
		}
	}
	if cname, err := resolver.LookupCNAME(probeCtx, host); err == nil {
		cname = strings.TrimSuffix(strings.TrimSpace(cname), ".")
		if cname != "" && !strings.EqualFold(cname, host) {
			record.CNAME = append(record.CNAME, cname)
		}
	}

	if len(record.A) == 0 && len(record.AAAA) == 0 && len(record.CNAME) == 0 {
		return "", nil
	}
	encoded, err := json.Marshal(record)
	if err != nil {
		return "", fmt.Errorf("encode dns record: %w", err)
	}
	return string(encoded), nil
}
