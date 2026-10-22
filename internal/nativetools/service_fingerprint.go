package nativetools

import (
	"bufio"
	"context"
	"encoding/xml"
	"fmt"
	"net"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// OpServiceFingerprint is the nmap-equivalent (connect scan + banner) operation.
const OpServiceFingerprint = "service_fingerprint"

// nmapRun is the Nmap XML subset the nmap parser reads
// (internal/app/nmap_parser.go: host/address, ports/port/state/service).
type nmapRun struct {
	XMLName xml.Name   `xml:"nmaprun"`
	Scanner string     `xml:"scanner,attr"`
	Args    string     `xml:"args,attr"`
	Hosts   []nmapHost `xml:"host"`
}

type nmapHost struct {
	Status    nmapStatus    `xml:"status"`
	Addresses []nmapAddress `xml:"address"`
	Ports     nmapPorts     `xml:"ports"`
}

type nmapStatus struct {
	State  string `xml:"state,attr"`
	Reason string `xml:"reason,attr"`
}

type nmapAddress struct {
	Addr     string `xml:"addr,attr"`
	AddrType string `xml:"addrtype,attr"`
}

type nmapPorts struct {
	Ports []nmapPort `xml:"port"`
}

type nmapPort struct {
	Protocol string      `xml:"protocol,attr"`
	PortID   int         `xml:"portid,attr"`
	State    nmapState   `xml:"state"`
	Service  nmapService `xml:"service"`
}

type nmapState struct {
	State  string `xml:"state,attr"`
	Reason string `xml:"reason,attr"`
}

type nmapService struct {
	Name    string `xml:"name,attr"`
	Product string `xml:"product,attr,omitempty"`
	Version string `xml:"version,attr,omitempty"`
	Tunnel  string `xml:"tunnel,attr,omitempty"`
	Method  string `xml:"method,attr,omitempty"`
	Conf    int    `xml:"conf,attr,omitempty"`
}

// serviceFingerprints maps a port to the service name a banner/port implies.
// SYN scanning, OS detection and NSE scripts stay out of scope: they need raw
// sockets and the NSE script library; the managed nmap remains the reference
// for those.
var serviceFingerprints = map[int]string{
	21: "ftp", 22: "ssh", 23: "telnet", 25: "smtp", 53: "domain",
	80: "http", 110: "pop3", 143: "imap", 443: "https", 445: "microsoft-ds",
	993: "imaps", 995: "pop3s", 1433: "ms-sql-s", 1521: "oracle",
	3306: "mysql", 3389: "ms-wbt-server", 5432: "postgresql", 5900: "vnc",
	6379: "redis", 8080: "http-proxy", 8443: "https-alt", 9200: "elasticsearch",
	27017: "mongodb",
}

// FingerprintServices performs a TCP connect scan with banner grabbing and
// emits nmap-compatible XML.
func FingerprintServices(ctx context.Context, target string, opts Options) (string, error) {
	hosts, ports, err := parseScanTarget(target)
	if err != nil {
		return "", err
	}

	scanCtx, cancel := context.WithTimeout(ctx, opts.timeout())
	defer cancel()

	run := nmapRun{Scanner: "ouroboros-native-nmap", Args: fmt.Sprintf("-sT -Pn %s", target)}
	for _, host := range hosts {
		if err := scanCtx.Err(); err != nil {
			return "", err
		}
		nmapHost := nmapHost{
			Status:    nmapStatus{State: "up", Reason: "connect"},
			Addresses: []nmapAddress{{Addr: host, AddrType: addressType(host)}},
		}
		openPorts := scanHostPorts(scanCtx, host, ports, opts.maxWorkers())
		sort.Slice(openPorts, func(i, j int) bool { return openPorts[i].PortID < openPorts[j].PortID })
		nmapHost.Ports = nmapPorts{Ports: openPorts}
		run.Hosts = append(run.Hosts, nmapHost)
	}

	encoded, err := xml.MarshalIndent(run, "", "  ")
	if err != nil {
		return "", fmt.Errorf("encode nmap document: %w", err)
	}
	return string(encoded), nil
}

func scanHostPorts(ctx context.Context, host string, ports []int, workers int) []nmapPort {
	results := make([]nmapPort, len(ports))
	sem := make(chan struct{}, workers)
	var wg sync.WaitGroup
	for index, port := range ports {
		wg.Add(1)
		go func(index, port int) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			results[index] = probeService(ctx, host, port)
		}(index, port)
	}
	wg.Wait()

	open := make([]nmapPort, 0, len(results))
	for _, result := range results {
		if result.State.State == "open" {
			open = append(open, result)
		}
	}
	return open
}

func probeService(ctx context.Context, host string, port int) nmapPort {
	entry := nmapPort{
		Protocol: "tcp",
		PortID:   port,
		State:    nmapState{State: "closed", Reason: "conn-refused"},
		Service:  nmapService{Name: "unknown", Conf: 3},
	}
	address := net.JoinHostPort(host, strconv.Itoa(port))
	dialer := &net.Dialer{Timeout: 3 * time.Second}
	conn, err := dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		return entry
	}
	defer func() { _ = conn.Close() }()

	entry.State = nmapState{State: "open", Reason: "syn-ack"}
	if name, ok := serviceFingerprints[port]; ok {
		entry.Service = nmapService{Name: name, Conf: 10}
		if port == 443 || port == 8443 {
			entry.Service.Tunnel = "ssl"
			entry.Service.Method = "probed"
		}
	}
	banner := readBanner(conn)
	if product, version, ok := serviceFromBanner(banner); ok {
		entry.Service.Product = product
		entry.Service.Version = version
		entry.Service.Method = "probed"
		entry.Service.Conf = 10
		if entry.Service.Name == "unknown" {
			entry.Service.Name = strings.ToLower(product)
		}
	}
	return entry
}

// readBanner reads whatever the service volunteers within a short window.
func readBanner(conn net.Conn) string {
	_ = conn.SetReadDeadline(time.Now().Add(750 * time.Millisecond))
	scanner := bufio.NewScanner(conn)
	scanner.Buffer(make([]byte, 0, 4096), 8192)
	if !scanner.Scan() {
		return ""
	}
	return strings.TrimSpace(scanner.Text())
}

// serviceFromBanner extracts "Product Version" from common banners.
func serviceFromBanner(banner string) (string, string, bool) {
	banner = strings.TrimSpace(banner)
	if banner == "" {
		return "", "", false
	}
	fields := strings.Fields(banner)
	if len(fields) == 0 {
		return "", "", false
	}
	product := fields[0]
	if len(fields) > 1 && looksLikeVersion(fields[1]) {
		return product, strings.Trim(fields[1], "()"), true
	}
	return product, "", true
}

func looksLikeVersion(value string) bool {
	if value == "" {
		return false
	}
	digits := 0
	for _, r := range value {
		switch {
		case r >= '0' && r <= '9':
			digits++
		case r == '.', r == '_', r == '-', r == '(', r == ')':
		default:
			return false
		}
	}
	return digits > 0
}

func addressType(host string) string {
	if net.ParseIP(host) != nil {
		return "ipv4"
	}
	return "ipv4"
}
