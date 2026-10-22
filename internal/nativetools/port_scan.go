package nativetools

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/netip"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// naabuRecord mirrors the naabu JSONL fields the naabu parser reads
// (internal/app/naabu_parser.go: ip|host, port, protocol).
type naabuRecord struct {
	IP       string `json:"ip,omitempty"`
	Host     string `json:"host,omitempty"`
	Port     int    `json:"port"`
	Protocol string `json:"protocol"`
}

// defaultScanPorts is the connect-scan port set used when the target does not
// embed a port range — deliberately small so a native scan stays polite.
var defaultScanPorts = []int{21, 22, 23, 25, 53, 80, 110, 143, 443, 445, 587, 993, 995, 1433, 1521, 3306, 3389, 5432, 5900, 6379, 8080, 8443, 8888, 9000, 9200, 27017}

// PortScan performs a TCP connect scan and emits naabu-compatible JSONL.
// It accepts "host", "host:port", "host:start-end" and CIDR notation.
func PortScan(ctx context.Context, target string, opts Options) (string, error) {
	hosts, ports, err := parseScanTarget(target)
	if err != nil {
		return "", err
	}

	scanCtx, cancel := context.WithTimeout(ctx, opts.timeout())
	defer cancel()

	type job struct {
		host string
		port int
	}
	jobs := make(chan job)
	results := make(chan naabuRecord, 64)

	workers := opts.maxWorkers()
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			dialer := &net.Dialer{Timeout: 3 * time.Second}
			for j := range jobs {
				address := net.JoinHostPort(j.host, strconv.Itoa(j.port))
				conn, err := dialer.DialContext(scanCtx, "tcp", address)
				if err != nil {
					continue
				}
				_ = conn.Close()
				select {
				case results <- naabuRecord{IP: j.host, Host: j.host, Port: j.port, Protocol: "tcp"}:
				case <-scanCtx.Done():
					return
				}
			}
		}()
	}

	go func() {
		defer close(jobs)
		for _, host := range hosts {
			for _, port := range ports {
				select {
				case jobs <- job{host: host, port: port}:
				case <-scanCtx.Done():
					return
				}
			}
		}
	}()
	go func() {
		wg.Wait()
		close(results)
	}()

	records := make([]naabuRecord, 0, 8)
	for record := range results {
		records = append(records, record)
	}
	sort.Slice(records, func(i, j int) bool {
		if records[i].Host != records[j].Host {
			return records[i].Host < records[j].Host
		}
		return records[i].Port < records[j].Port
	})

	var builder strings.Builder
	for _, record := range records {
		encoded, err := json.Marshal(record)
		if err != nil {
			return "", fmt.Errorf("encode scan record: %w", err)
		}
		builder.Write(encoded)
		builder.WriteByte('\n')
	}
	return strings.TrimRight(builder.String(), "\n"), nil
}

// parseScanTarget expands the target into hosts and ports.
func parseScanTarget(target string) ([]string, []int, error) {
	target = strings.TrimSpace(target)
	if target == "" {
		return nil, nil, fmt.Errorf("port_scan requires a host target")
	}

	host := target
	ports := defaultScanPorts

	if prefix, err := netip.ParsePrefix(target); err == nil {
		hosts := make([]string, 0, 16)
		for addr := prefix.Masked().Addr(); prefix.Contains(addr); addr = addr.Next() {
			hosts = append(hosts, addr.String())
			if len(hosts) >= 256 {
				break // keep native scans bounded
			}
		}
		return hosts, ports, nil
	}

	if strings.Count(target, ":") == 1 {
		hostPart, portPart, _ := strings.Cut(target, ":")
		if parsed, err := parsePortSpec(portPart); err == nil {
			host, ports = hostPart, parsed
		}
	}
	if strings.TrimSpace(host) == "" {
		return nil, nil, fmt.Errorf("port_scan target %q has no host", target)
	}
	return []string{host}, ports, nil
}

// parsePortSpec parses "443", "80,443" or "1-1024".
func parsePortSpec(spec string) ([]int, error) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return nil, fmt.Errorf("empty port spec")
	}
	ports := []int{}
	for _, part := range strings.Split(spec, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if start, end, ok := strings.Cut(part, "-"); ok {
			from, err := strconv.Atoi(strings.TrimSpace(start))
			if err != nil {
				return nil, err
			}
			to, err := strconv.Atoi(strings.TrimSpace(end))
			if err != nil {
				return nil, err
			}
			if from < 1 || to > 65535 || from > to {
				return nil, fmt.Errorf("port range %q out of bounds", part)
			}
			for port := from; port <= to && len(ports) < 4096; port++ {
				ports = append(ports, port)
			}
			continue
		}
		port, err := strconv.Atoi(part)
		if err != nil {
			return nil, err
		}
		if port < 1 || port > 65535 {
			return nil, fmt.Errorf("port %d out of bounds", port)
		}
		ports = append(ports, port)
	}
	if len(ports) == 0 {
		return nil, fmt.Errorf("port spec %q produced no ports", spec)
	}
	return ports, nil
}
