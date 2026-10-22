package nativetools

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"sort"
	"strings"
)

// OpDNSRecon is the dnsrecon-equivalent operation. It shares the dns_probe
// target kinds but emits dnsrecon's record shape.
const OpDNSRecon = "dns_probe"

// dnsreconRecord mirrors the dnsrecon JSON record fields the parser reads
// (internal/app/dnsrecon_parser.go: type, name, address).
type dnsreconRecord struct {
	Type    string `json:"type"`
	Name    string `json:"name"`
	Address string `json:"address,omitempty"`
	Target  string `json:"target,omitempty"`
}

// DNSReconRecord enumerates A/AAAA/CNAME/MX/NS/TXT records for a domain and
// emits dnsrecon-compatible JSON. AXFR zone transfers and dictionary brute
// force stay out of scope (they need a full DNS client library).
func DNSReconRecord(ctx context.Context, target string, opts Options) (string, error) {
	domain := strings.TrimSpace(strings.TrimSuffix(target, "."))
	if domain == "" {
		return "", fmt.Errorf("dnsrecon probe requires a domain target")
	}

	probeCtx, cancel := context.WithTimeout(ctx, opts.timeout())
	defer cancel()

	resolver := &net.Resolver{}
	records := []dnsreconRecord{}

	if addrs, err := resolver.LookupHost(probeCtx, domain); err == nil {
		for _, addr := range addrs {
			parsed := net.ParseIP(addr)
			recordType := "AAAA"
			if parsed != nil && parsed.To4() != nil {
				recordType = "A"
			}
			records = append(records, dnsreconRecord{Type: recordType, Name: domain, Address: addr})
		}
	}
	if cname, err := resolver.LookupCNAME(probeCtx, domain); err == nil {
		cname = strings.TrimSuffix(strings.TrimSpace(cname), ".")
		if cname != "" && !strings.EqualFold(cname, domain) {
			records = append(records, dnsreconRecord{Type: "CNAME", Name: domain, Address: cname})
		}
	}
	if hosts, err := resolver.LookupMX(probeCtx, domain); err == nil {
		for _, host := range hosts {
			name := strings.TrimSuffix(strings.TrimSpace(host.Host), ".")
			if name == "" {
				continue
			}
			records = append(records, dnsreconRecord{Type: "MX", Name: domain, Address: name})
		}
	}
	if names, err := resolver.LookupNS(probeCtx, domain); err == nil {
		for _, ns := range names {
			name := strings.TrimSuffix(strings.TrimSpace(ns.Host), ".")
			if name == "" {
				continue
			}
			records = append(records, dnsreconRecord{Type: "NS", Name: domain, Address: name})
		}
	}
	if values, err := resolver.LookupTXT(probeCtx, domain); err == nil {
		for _, value := range values {
			value = strings.TrimSpace(value)
			if value == "" {
				continue
			}
			records = append(records, dnsreconRecord{Type: "TXT", Name: domain, Address: value})
		}
	}

	if len(records) == 0 {
		return "", nil
	}
	sort.SliceStable(records, func(i, j int) bool {
		if records[i].Type != records[j].Type {
			return records[i].Type < records[j].Type
		}
		return records[i].Address < records[j].Address
	})
	encoded, err := json.Marshal(records)
	if err != nil {
		return "", fmt.Errorf("encode dnsrecon records: %w", err)
	}
	return string(encoded), nil
}
