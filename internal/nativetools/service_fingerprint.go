package nativetools

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/xml"
	"fmt"
	"net"
	"regexp"
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
	21:    "ftp",
	22:    "ssh",
	23:    "telnet",
	25:    "smtp",
	53:    "domain",
	80:    "http",
	110:   "pop3",
	111:   "rpcbind",
	135:   "msrpc",
	139:   "netbios-ssn",
	143:   "imap",
	161:   "snmp",
	389:   "ldap",
	443:   "https",
	445:   "microsoft-ds",
	465:   "smtps",
	502:   "modbus",
	514:   "syslog",
	515:   "printer",
	548:   "afp",
	554:   "rtsp",
	587:   "smtp",
	623:   "asf-rmcp",
	631:   "ipp",
	636:   "ldapssl",
	873:   "rsync",
	993:   "imaps",
	995:   "pop3s",
	1080:  "socks",
	1099:  "java-rmi",
	1194:  "openvpn",
	1433:  "ms-sql-s",
	1521:  "oracle",
	1723:  "pptp",
	1883:  "mqtt",
	2049:  "nfs",
	2082:  "cpanel",
	2083:  "cpanel",
	2181:  "zookeeper",
	2375:  "docker",
	2376:  "docker",
	2377:  "docker-swarm",
	3128:  "squid-proxy",
	3268:  "globalcatLDAP",
	3306:  "mysql",
	3389:  "ms-wbt-server",
	4444:  "metasploit",
	5000:  "upnp",
	5060:  "sip",
	5222:  "xmpp-client",
	5432:  "postgresql",
	5555:  "adb",
	5672:  "amqp",
	5900:  "vnc",
	5901:  "vnc",
	5984:  "couchdb",
	5985:  "wsman",
	5986:  "wsman",
	6379:  "redis",
	6443:  "kubernetes",
	6667:  "irc",
	8000:  "http-alt",
	8009:  "ajp13",
	8080:  "http-proxy",
	8081:  "http-alt",
	8443:  "https-alt",
	8500:  "consul",
	8888:  "http-alt",
	9000:  "cslistener",
	9042:  "cassandra",
	9092:  "kafka",
	9200:  "elasticsearch",
	9300:  "elasticsearch",
	11211: "memcached",
	15672: "rabbitmq-management",
	27017: "mongodb",
	27018: "mongodb",
	28017: "mongodb-web",
	50070: "hadoop-namenode",
	63790: "redis-cli",
}

// udpProbeServices are the UDP ports probed with a service-specific request
// packet. UDP is best effort: only responses are reported, silence is not
// treated as closed.
var udpProbeServices = []struct {
	port    int
	name    string
	payload []byte
}{
	// DNS: recursive query for the root NS names, RD flag set.
	{53, "domain", []byte{
		0x8a, 0x01, 0x01, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x06, 0x00, 0x01, // root NS query with class ANY, RFC 8499 style
	}},
	{123, "ntp", ntpClientPacket()},
	{161, "snmp", snmpGetRequest()},
	{1900, "ssdp", []byte("M-SEARCH * HTTP/1.1\r\nHost: 239.255.255.250:1900\r\nMan: \"ssdp:discover\"\r\nMX: 1\r\nST: ssdp:all\r\n\r\n")},
	{5353, "mdns", []byte{
		0x00, 0x00, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x09, '_', 's', 'e', 'r', 'v', 'i', 'c', 'e', 0x04, '_', 'u', 'd', 'p',
		0x05, 'l', 'o', 'c', 'a', 'l', 0x00, 0x00, 0x0C, 0x00, 0x01,
	}},
	{11211, "memcached", []byte("version\r\n")},
}

func ntpClientPacket() []byte {
	// NTPv3 client mode request: LI=0 VN=3 Mode=3 in the first byte.
	packet := make([]byte, 48)
	packet[0] = 0x1B
	return packet
}

func snmpGetRequest() []byte {
	// SNMPv2c GET for sysDescr.0 with the widely used "public" community:
	// a structurally valid packet any SNMP agent will answer (or reject with
	// an authentication trap - both prove the port is open).
	payload := []byte{
		0x30, 0x29, // SEQUENCE
		0x02, 0x01, 0x01, // INTEGER version 1 (v2c)
		0x04, 0x06, 'p', 'u', 'b', 'l', 'i', 'c', // community
		0xA0, 0x1C, // GetRequest
		0x02, 0x04, 0x4f, 0x55, 0x52, 0x4f, // request-id
		0x02, 0x01, 0x00, // error-status
		0x02, 0x01, 0x00, // error-index
		0x30, 0x0E, // varbind list
		0x30, 0x0C, // varbind
		0x06, 0x08, 0x2B, 0x06, 0x01, 0x02, 0x01, 0x01, 0x01, 0x00, // sysDescr.0
		0x05, 0x00, // NULL
	}
	return payload
}

// FingerprintServices performs a TCP connect scan with banner grabbing and a
// best-effort UDP probe of common ports, emitting nmap-compatible XML.
func FingerprintServices(ctx context.Context, target string, opts Options) (string, error) {
	hosts, ports, err := parseScanTarget(target)
	if err != nil {
		return "", err
	}

	scanCtx, cancel := context.WithTimeout(ctx, opts.timeout())
	defer cancel()

	run := nmapRun{Scanner: "ouroboros-native-nmap", Args: fmt.Sprintf("-sT -Pn -sU %s", target)}
	for _, host := range hosts {
		if err := scanCtx.Err(); err != nil {
			return "", err
		}
		nmapHost := nmapHost{
			Status:    nmapStatus{State: "up", Reason: "connect"},
			Addresses: []nmapAddress{{Addr: host, AddrType: addressType(host)}},
		}
		openPorts := scanHostPorts(scanCtx, host, ports, opts.maxWorkers())
		openPorts = append(openPorts, probeUDPServices(scanCtx, host, opts.maxWorkers())...)
		sort.Slice(openPorts, func(i, j int) bool {
			if openPorts[i].Protocol != openPorts[j].Protocol {
				return openPorts[i].Protocol < openPorts[j].Protocol
			}
			return openPorts[i].PortID < openPorts[j].PortID
		})
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

	// TLS ports get a real handshake so the certificate leaks identity even
	// without an application banner.
	if entry.Service.Tunnel == "ssl" {
		if summary := tlsBannerProbe(ctx, address); summary != "" {
			entry.Service.Method = "probed"
			entry.Service.Conf = 10
			entry.Service.Name = serviceFallbackName(port, "https")
			entry.Service.Product = summary
		}
		return entry
	}

	banner := readBanner(conn)
	if banner == "" {
		// Some services are silent until spoken to; send protocol pings.
		if payload, ok := tcpProbePayload(port); ok {
			if _, err := conn.Write(payload); err == nil {
				banner = readBanner(conn)
			}
		}
	}
	if product, version, ok := parseBanner(banner); ok {
		entry.Service.Product = product
		entry.Service.Version = version
		entry.Service.Method = "probed"
		entry.Service.Conf = 10
		if entry.Service.Name == "unknown" {
			entry.Service.Name = strings.ToLower(product)
		}
	}
	if entry.Service.Name == "unknown" {
		if product, _, ok := serviceFromBanner(banner); ok && product != "" {
			entry.Service.Name = strings.ToLower(product)
			entry.Service.Method = "probed"
			entry.Service.Conf = 10
		}
	}
	return entry
}

// serviceFallbackName keeps the port-table name when the TLS probe produces a
// product string only.
func serviceFallbackName(port int, fallback string) string {
	if name, ok := serviceFingerprints[port]; ok {
		return name
	}
	return fallback
}

// tlsBannerProbe completes a real TLS handshake (certificates intentionally
// unverified) and returns "leaf-CN issuer-CN" as the identity banner.
func tlsBannerProbe(ctx context.Context, address string) string {
	dialer := &net.Dialer{Timeout: 3 * time.Second}
	conn, err := tls.DialWithDialer(dialer, "tcp", address, &tls.Config{
		MinVersion:         tls.VersionTLS10,
		MaxVersion:         tls.VersionTLS13,
		InsecureSkipVerify: true,
	})
	if err != nil {
		_ = ctx.Err()
		return ""
	}
	defer func() { _ = conn.Close() }()
	state := conn.ConnectionState()
	if len(state.PeerCertificates) == 0 {
		return ""
	}
	cert := state.PeerCertificates[0]
	subject := cert.Subject.CommonName
	if subject == "" {
		subject = cert.Subject.String()
	}
	issuer := cert.Issuer.CommonName
	if issuer == "" {
		issuer = cert.Issuer.String()
	}
	return strings.TrimSpace(subject + " issued by " + issuer)
}

// tcpProbePayload returns the byte payload that makes silent services on a
// known port speak first.
func tcpProbePayload(port int) ([]byte, bool) {
	switch port {
	case 25, 587, 465: // SMTP family (465 is usually TLS but answers EHLO on plaintext)
		return []byte("EHLO ouroboros\r\n"), true
	case 6379: // Redis: PING echoes +PONG
		return []byte("PING\r\n"), true
	case 11211: // memcached
		return []byte("version\r\n"), true
	case 21, 23, 110, 143, 5900: // FTP/telnet/POP3/IMAP/VNC greet first; a newline is harmless
		return []byte("\r\n"), true
	case 502, 1883, 2181: // Modbus, MQTT, ZooKeeper: minimal pings
		if port == 502 {
			// Modbus TCP read-coils request, transaction id 0.
			return []byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x06, 0x01, 0x01, 0x00, 0x00, 0x00, 0x01}, true
		}
		if port == 1883 {
			// MQTT PINGREQ with fixed header only.
			return []byte{0xC0, 0x00}, true
		}
		return []byte("mntr\n"), true
	case 3306: // MySQL: server greets first, but old servers need a kick
		return []byte("\r\n"), true
	case 8080, 8000, 8009, 8888, 9000:
		if port == 8009 {
			// AJP13 prefix + generic forward request magic.
			return []byte{0x12, 0x34, 0x00, 0x01, 0x0a}, true
		}
		return []byte("HEAD / HTTP/1.0\r\n\r\n"), true
	}
	return nil, false
}

// probeUDPServices sends service-specific request datagrams to the common UDP
// ports and reports the ones that answer. Silence is reported as nothing: a
// quiet UDP port is neither open nor closed from a single unconnected socket.
func probeUDPServices(ctx context.Context, host string, workers int) []nmapPort {
	sem := make(chan struct{}, min(workers, 6))
	var wg sync.WaitGroup
	results := make([]nmapPort, len(udpProbeServices))
	for index, probe := range udpProbeServices {
		wg.Add(1)
		go func(index int, probe struct {
			port    int
			name    string
			payload []byte
		}) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			if err := ctx.Err(); err != nil {
				return
			}
			entry, ok := udpProbe(host, probe.port, probe.name, probe.payload)
			if ok {
				results[index] = entry
			}
		}(index, probe)
	}
	wg.Wait()

	open := make([]nmapPort, 0, len(results))
	for _, entry := range results {
		if entry.Protocol != "" {
			open = append(open, entry)
		}
	}
	return open
}

// udpProbe sends one request datagram and waits briefly for a response.
func udpProbe(host string, port int, name string, payload []byte) (nmapPort, bool) {
	deadline := time.Now().Add(2 * time.Second)
	dialer := &net.Dialer{Timeout: 2 * time.Second}
	conn, err := dialer.Dial("udp", net.JoinHostPort(host, strconv.Itoa(port)))
	if err != nil {
		return nmapPort{}, false
	}
	defer func() { _ = conn.Close() }()
	_ = conn.SetDeadline(deadline)
	if _, err := conn.Write(payload); err != nil {
		return nmapPort{}, false
	}
	buffer := make([]byte, 4096)
	length, err := conn.Read(buffer)
	if err != nil || length == 0 {
		return nmapPort{}, false
	}
	return nmapPort{
		Protocol: "udp",
		PortID:   port,
		State:    nmapState{State: "open", Reason: "udp-response"},
		Service:  nmapService{Name: name, Method: "probed", Conf: 7},
	}, true
}

// readBanner reads whatever the service volunteers within a short window: a
// single greeting line (SSH/FTP/SMTP/Redis) or, when the first line opens an
// HTTP response, the complete response head through its blank terminator so
// Server/X-Powered-By headers survive for parseBanner.
func readBanner(conn net.Conn) string {
	_ = conn.SetReadDeadline(time.Now().Add(750 * time.Millisecond))
	reader := bufio.NewReader(conn)
	first, err := reader.ReadString('\n')
	if err != nil && strings.TrimSpace(first) == "" {
		return ""
	}
	banner := first
	if httpLinePattern.MatchString(first) {
		for bannerLen := len(banner); bannerLen < 16384; bannerLen = len(banner) {
			line, readErr := reader.ReadString('\n')
			banner += line
			if readErr != nil || strings.TrimSpace(line) == "" {
				break
			}
		}
	}
	return strings.TrimSpace(banner)
}

// httpLinePattern recognizes an HTTP/1.x status line opening a banner.
var httpLinePattern = regexp.MustCompile(`(?i)^HTTP/1\.[01] \d{3}`)

// versionPattern captures a trailing version-looking token like 2.4.41, 9.6,
// 1_8_0_302 or 2023.10.1.
var versionPattern = regexp.MustCompile(`[0-9]+(?:[._-][0-9]+)+`)

// serviceFromBanner extracts "Product Version" from common banners: SSH
// identity strings, HTTP Server headers, database greetings and line-oriented
// server hello messages.
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

// parseBanner normalizes a banner into (product, version): SSH version
// strings, HTTP Server/X-Powered-By headers and multi-token server greetings
// are recognized; product names are canonicalized to the nmap service names.
func parseBanner(banner string) (string, string, bool) {
	banner = strings.TrimSpace(banner)
	if banner == "" {
		return "", "", false
	}

	// SSH: RFC 4253 "SSH-protoversion-softwareversion" plus optional comment.
	if strings.HasPrefix(banner, "SSH-") {
		return parseSSHBanner(banner)
	}

	// HTTP response: Server and X-Powered-By headers carry the identity.
	if product, version, ok := parseHTTPServerBanner(banner); ok {
		return product, version, true
	}

	// Line-oriented greetings: FTP 220, SMTP 220, POP3 +OK, IMAP * OK,
	// Redis -/+ replies, memcached VERSION, MongoDB i ... lines.
	if product, version, ok := parseGreetingBanner(banner); ok {
		return product, version, true
	}

	return "", "", false
}

// parseSSHBanner splits "SSH-2.0-OpenSSH_9.6p1 Debian-2" into product and
// version, dropping the protocol version prefix and trailing comment.
func parseSSHBanner(banner string) (string, string, bool) {
	rest, ok := strings.CutPrefix(banner, "SSH-")
	if !ok {
		return "", "", false
	}
	software, _ := strings.CutPrefix(rest, "1.99-")
	if !strings.Contains(software, "-") {
		// Non-conformant banner without a protocol version.
		return "", "", false
	}
	software = software[strings.Index(software, "-")+1:]
	if comment, _, hasComment := strings.Cut(software, " "); hasComment {
		software = strings.TrimSpace(comment)
	}
	product, version, _ := strings.Cut(software, "_")
	product = strings.TrimSpace(product)
	version = strings.TrimSpace(version)
	if product == "" {
		return "", "", false
	}
	if version == "" {
		version = "unknown"
	}
	// Strip a trailing package suffix like 9.6p1 -> keep as version; distro
	// build tags after a space were already dropped above.
	return "OpenSSH", version, true
}

// httpHeaderPattern matches Server:/X-Powered-By: headers in a response head.
var httpHeaderPattern = regexp.MustCompile(`(?im)^(server|x-powered-by):[ \t]*(.+)$`)

// parseHTTPServerBanner extracts product and version from an HTTP response
// head if the first line looks like an HTTP status line.
func parseHTTPServerBanner(banner string) (string, string, bool) {
	firstLine, _, found := strings.Cut(banner, "\n")
	firstLine = strings.TrimSpace(firstLine)
	if !found || !strings.HasPrefix(firstLine, "HTTP/") {
		return "", "", false
	}
	product, version := "", ""
	for _, match := range httpHeaderPattern.FindAllStringSubmatch(banner, -1) {
		value := strings.TrimSpace(match[2])
		headerProduct, headerVersion, _ := splitProductVersion(value)
		if headerProduct == "" {
			continue
		}
		if strings.EqualFold(match[1], "server") {
			product, version = headerProduct, headerVersion
			continue
		}
		if product == "" {
			product, version = headerProduct, headerVersion
		}
	}
	if product == "" {
		return "", "", false
	}
	return product, version, true
}

// parseGreetingBanner recognizes product/line conventions of common TCP
// services that greet or answer first.
func parseGreetingBanner(banner string) (string, string, bool) {
	lower := strings.ToLower(banner)
	switch {
	case strings.HasPrefix(lower, "220 "), strings.HasPrefix(lower, "220-"):
		// FTP and SMTP share the 220 greeting; choose by port-agnostic cues.
		if strings.Contains(lower, "ftp") || strings.Contains(lower, "filezilla") || strings.Contains(lower, "vsftpd") || strings.Contains(lower, "proftpd") || strings.Contains(lower, "pure-ftpd") {
			return splitProductVersion(strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(banner, "220 "), "220-")))
		}
		return splitProductVersion(extractESMTPProduct(strings.TrimSpace(strings.TrimPrefix(banner, "220"))))
	case strings.HasPrefix(lower, "+ok"):
		// POP3: +OK Dovecot ready.
		return splitProductVersion(strings.TrimSpace(strings.TrimPrefix(banner, "+OK")))
	case strings.HasPrefix(banner, "* OK"), strings.HasPrefix(banner, "* BYE"):
		// IMAP: * OK [CAPABILITY ...] Dovecot ready.
		fields := strings.Fields(banner)
		for index := len(fields) - 1; index >= 2; index-- {
			if looksLikeVersion(fields[index]) {
				return fields[index-1], fields[index], true
			}
		}
		if len(fields) >= 3 {
			return fields[2], "", true
		}
		return "", "", false
	case strings.HasPrefix(lower, "version"):
		// memcached "VERSION 1.6.9".
		return splitProductVersion(strings.TrimSpace(strings.TrimPrefix(banner, "VERSION")))
	case strings.HasPrefix(banner, "-ERR"), strings.HasPrefix(banner, "+PONG"), strings.HasPrefix(banner, "-PONG"):
		// Redis: a PING echo proves Redis but carries no version.
		return "redis", "", true
	case strings.HasPrefix(banner, "220 ") && strings.Contains(lower, "mysql"):
		return splitProductVersion(strings.TrimPrefix(banner, "220 "))
	}
	if product, version, ok := splitProductVersion(banner); ok && product != "" {
		return product, version, true
	}
	return "", "", false
}

// extractESMTPProduct pulls "Postfix" / "Exchange" / "ESMTP ..." tokens from
// an SMTP greeting.
func extractESMTPProduct(greeting string) string {
	fields := strings.Fields(greeting)
	product := ""
	for index, field := range fields {
		if index == 0 {
			continue
		}
		if strings.HasSuffix(strings.ToLower(field), ".com") || strings.HasSuffix(strings.ToLower(field), ".net") || strings.HasSuffix(strings.ToLower(field), ".org") {
			continue
		}
		if looksLikeVersion(field) {
			continue
		}
		if field == "ESMTP" || field == "Service" || field == "ready" || strings.EqualFold(field, "ready.") {
			continue
		}
		product = field
		break
	}
	return product
}

// splitProductVersion takes "OpenSSH 9.6p1" or "nginx/1.24.0" and returns the
// product and version tokens.
func splitProductVersion(value string) (string, string, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", "", false
	}
	if product, version, found := strings.Cut(value, "/"); found && looksLikeVersion(version) {
		return strings.TrimSpace(product), strings.TrimSpace(version), true
	}
	if match := versionPattern.FindStringIndex(value); match != nil {
		product := strings.TrimSpace(value[:match[0]])
		version := value[match[0]:]
		if match[0] == 0 {
			// Version-first banner: no product to name.
			return "", version, true
		}
		product = strings.TrimRight(product, " \t:-")
		if product == "" {
			return "", version, true
		}
		return lastWord(product), version, true
	}
	fields := strings.Fields(value)
	if len(fields) > 0 {
		return fields[0], "", true
	}
	return "", "", false
}

// lastWord keeps the final token of a multi-word product prefix (e.g.
// "Microsoft ESMTP MAIL Service ready" -> "Service" is filtered upstream, so
// this handles "Apache Coyote 1.1" -> "Coyote").
func lastWord(value string) string {
	fields := strings.Fields(value)
	if len(fields) == 0 {
		return value
	}
	return fields[len(fields)-1]
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
		case r == '.', r == '_', r == '-', r == '(', r == ')', r == 'p', r == 'b', r == 'a':
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
