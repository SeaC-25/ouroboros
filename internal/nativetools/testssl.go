package nativetools

import (
	"context"
	crand "crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"net"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// testsslFinding mirrors the testssl.sh JSON finding shape
// (id / ip / port / severity / finding).
type testsslFinding struct {
	ID       string `json:"id"`
	IP       string `json:"ip"`
	Port     string `json:"port"`
	Severity string `json:"severity"`
	Finding  string `json:"finding"`
}

// AssessTLSTestssl emits testssl.sh shaped JSON for the checks crypto/tls can
// perform: protocol version acceptance, weak cipher suite probing, certificate
// validity properties and a real Heartbleed (CVE-2014-0160) probe built on raw
// TLS records.
func AssessTLSTestssl(ctx context.Context, target string, opts Options) (string, error) {
	host, port, err := splitHostPort(target, "443")
	if err != nil {
		return "", err
	}
	address := net.JoinHostPort(host, port)

	findings := []testsslFinding{}
	var leaf *x509.Certificate
	for _, candidate := range tlsProbeVersions {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		finding := testsslFinding{
			ID:       "protocols",
			IP:       host,
			Port:     port,
			Severity: "INFO",
			Finding:  candidate.Name + " not offered",
		}
		conn, err := tlsProbeHandshake(ctx, address, &tls.Config{
			MinVersion:         candidate.Version,
			MaxVersion:         candidate.Version,
			InsecureSkipVerify: true,
		}, opts.timeout())
		if err == nil {
			state := conn.ConnectionState()
			switch candidate.Version {
			case tls.VersionTLS10, tls.VersionTLS11:
				// A deprecated protocol being accepted is itself the finding.
				finding.Severity = "HIGH"
				finding.Finding = candidate.Name + " offered (deprecated protocol accepted)"
			default:
				finding.Severity = "OK"
				finding.Finding = candidate.Name + " offered"
			}
			if name := tls.CipherSuiteName(state.CipherSuite); name != "" && !strings.HasPrefix(name, "0x") {
				findings = append(findings, testsslFinding{
					ID:       "cipherlist",
					IP:       host,
					Port:     port,
					Severity: "INFO",
					Finding:  candidate.Name + " negotiated " + name,
				})
			}
			if len(state.PeerCertificates) > 0 {
				leaf = state.PeerCertificates[0]
			}
			_ = conn.Close()
		} else if !tlsHandshakeRejected(err) {
			// The TCP connection itself failed: nothing was learned about the
			// server, so report an inconclusive probe instead of "not offered".
			finding.Severity = "INFO"
			finding.Finding = candidate.Name + " probe inconclusive: " + tlsTransportError(err)
			findings = append(findings, finding)
			continue
		} else if candidate.Version == tls.VersionTLS10 || candidate.Version == tls.VersionTLS11 {
			// Deprecated protocol versions being refused is the desired outcome.
			finding.Severity = "OK"
		}
		findings = append(findings, finding)
	}
	if leaf != nil {
		findings = append(findings, certificateFindings(host, port, leaf, time.Now())...)
	}
	findings = append(findings, weakCipherFindings(ctx, host, port, address, opts)...)

	severity, text := probeHeartbleed(ctx, address, host, opts.timeout())
	findings = append(findings, testsslFinding{
		ID:       "heartbleed",
		IP:       host,
		Port:     port,
		Severity: severity,
		Finding:  text,
	})

	sort.SliceStable(findings, func(i, j int) bool { return findings[i].ID < findings[j].ID })
	encoded, err := json.MarshalIndent(findings, "", "  ")
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

// certificateFindings reports expiry, trust, hostname and signature strength
// properties of the leaf certificate.
func certificateFindings(host, port string, cert *x509.Certificate, now time.Time) []testsslFinding {
	findings := []testsslFinding{{
		ID:       "cert_chain_of_trust",
		IP:       host,
		Port:     port,
		Severity: "INFO",
		Finding:  "issuer " + cert.Issuer.String(),
	}}
	switch {
	case now.After(cert.NotAfter):
		findings = append(findings, testsslFinding{
			ID: "cert_expirationStatus", IP: host, Port: port, Severity: "CRITICAL",
			Finding: "certificate expired on " + cert.NotAfter.UTC().Format(time.RFC3339),
		})
	case cert.NotAfter.Sub(now) < 30*24*time.Hour:
		findings = append(findings, testsslFinding{
			ID: "cert_expirationStatus", IP: host, Port: port, Severity: "MEDIUM",
			Finding: "certificate expires soon: " + cert.NotAfter.UTC().Format(time.RFC3339),
		})
	default:
		findings = append(findings, testsslFinding{
			ID: "cert_expirationStatus", IP: host, Port: port, Severity: "OK",
			Finding: "certificate valid until " + cert.NotAfter.UTC().Format(time.RFC3339),
		})
	}
	if cert.Subject.String() == cert.Issuer.String() {
		findings = append(findings, testsslFinding{
			ID: "cert_notSelfSigned", IP: host, Port: port, Severity: "MEDIUM",
			Finding: "certificate is self-signed",
		})
	}
	if now.Before(cert.NotBefore) {
		findings = append(findings, testsslFinding{
			ID: "cert_notYetValid", IP: host, Port: port, Severity: "HIGH",
			Finding: "certificate is not yet valid (notBefore " + cert.NotBefore.UTC().Format(time.RFC3339) + ")",
		})
	}
	if net.ParseIP(host) == nil {
		if err := cert.VerifyHostname(host); err != nil {
			findings = append(findings, testsslFinding{
				ID: "cert_hostname_mismatch", IP: host, Port: port, Severity: "MEDIUM",
				Finding: "certificate does not match hostname " + host,
			})
		} else {
			findings = append(findings, testsslFinding{
				ID: "cert_hostname_match", IP: host, Port: port, Severity: "OK",
				Finding: "certificate matches hostname " + host,
			})
		}
	}
	algorithm := cert.SignatureAlgorithm.String()
	if isWeakCertificateSignature(cert.SignatureAlgorithm) {
		findings = append(findings, testsslFinding{
			ID: "cert_weak_signature", IP: host, Port: port, Severity: "MEDIUM",
			Finding: "certificate uses weak SHA-1 signature algorithm " + algorithm,
		})
	} else {
		findings = append(findings, testsslFinding{
			ID: "cert_weak_signature", IP: host, Port: port, Severity: "OK",
			Finding: "certificate signature algorithm " + algorithm,
		})
	}
	return findings
}

// isWeakCertificateSignature reports SHA-1 based certificate signatures.
func isWeakCertificateSignature(algorithm x509.SignatureAlgorithm) bool {
	switch algorithm {
	case x509.SHA1WithRSA, x509.ECDSAWithSHA1, x509.DSAWithSHA1:
		return true
	}
	return false
}

// weakCipherSuite is one legacy cipher suite the weak-cipher sweep forces in
// an otherwise permissive handshake. Config.CipherSuites re-enables the
// insecure RC4/3DES/RSA-kex suites, so offering a single ID makes the server
// either negotiate exactly that suite or refuse the handshake.
type weakCipherSuite struct {
	id   uint16
	kind string
}

var weakCipherProbeSuites = []weakCipherSuite{
	{tls.TLS_RSA_WITH_RC4_128_SHA, "rc4"},
	{tls.TLS_ECDHE_RSA_WITH_RC4_128_SHA, "rc4"},
	{tls.TLS_ECDHE_ECDSA_WITH_RC4_128_SHA, "rc4"},
	{tls.TLS_RSA_WITH_3DES_EDE_CBC_SHA, "3des"},
	{tls.TLS_ECDHE_RSA_WITH_3DES_EDE_CBC_SHA, "3des"},
	{tls.TLS_RSA_WITH_AES_128_CBC_SHA, "cbc"},
	{tls.TLS_RSA_WITH_AES_256_CBC_SHA, "cbc"},
	{tls.TLS_RSA_WITH_AES_128_CBC_SHA256, "cbc"},
	{tls.TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA, "cbc"},
	{tls.TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA, "cbc"},
	{tls.TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA256, "cbc"},
	{tls.TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA, "cbc"},
	{tls.TLS_ECDHE_ECDSA_WITH_AES_256_CBC_SHA, "cbc"},
	{tls.TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA256, "cbc"},
}

// weakCipherFindings forces each legacy cipher suite in a dedicated handshake
// and reports which families the server accepts. NULL/EXPORT/single-DES
// suites cannot be offered by crypto/tls at all and are reported as such.
func weakCipherFindings(ctx context.Context, host, port, address string, opts Options) []testsslFinding {
	accepted := make([]string, len(weakCipherProbeSuites))
	sem := make(chan struct{}, min(opts.maxWorkers(), 8))
	var wg sync.WaitGroup
	for index, suite := range weakCipherProbeSuites {
		wg.Add(1)
		go func(index int, suite weakCipherSuite) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			if err := ctx.Err(); err != nil {
				return
			}
			conn, err := tlsProbeHandshake(ctx, address, &tls.Config{
				MinVersion:         tls.VersionTLS10,
				MaxVersion:         tls.VersionTLS12,
				CipherSuites:       []uint16{suite.id},
				InsecureSkipVerify: true,
			}, opts.timeout())
			if err != nil {
				return
			}
			if conn.ConnectionState().CipherSuite == suite.id {
				accepted[index] = tls.CipherSuiteName(suite.id)
			}
			_ = conn.Close()
		}(index, suite)
	}
	wg.Wait()

	rc4, tdes, cbc := []string{}, []string{}, []string{}
	for index, suite := range weakCipherProbeSuites {
		if accepted[index] == "" {
			continue
		}
		switch suite.kind {
		case "rc4":
			rc4 = append(rc4, accepted[index])
		case "3des":
			tdes = append(tdes, accepted[index])
		case "cbc":
			cbc = append(cbc, accepted[index])
		}
	}

	findings := []testsslFinding{}
	findings = append(findings, weakCipherFinding(host, port, "cipherlist_RC4", "RC4", rc4, "HIGH"))
	findings = append(findings, weakCipherFinding(host, port, "cipherlist_3DES", "3DES", tdes, "HIGH"))
	findings = append(findings, weakCipherFinding(host, port, "cipherlist_CBC", "CBC-mode (non-AEAD)", cbc, "MEDIUM"))
	for _, blocked := range []struct{ id, label string }{
		{"cipherlist_NULL", "NULL"},
		{"cipherlist_EXPORT", "EXPORT"},
		{"cipherlist_DES", "single-DES"},
	} {
		findings = append(findings, testsslFinding{
			ID: blocked.id, IP: host, Port: port, Severity: "INFO",
			Finding: blocked.label + " suites are not testable natively: crypto/tls cannot offer them; managed testssl.sh remains the reference",
		})
	}
	return findings
}

func weakCipherFinding(host, port, id, label string, accepted []string, severity string) testsslFinding {
	if len(accepted) == 0 {
		return testsslFinding{
			ID: id, IP: host, Port: port, Severity: "OK",
			Finding: "no " + label + " cipher suite accepted",
		}
	}
	return testsslFinding{
		ID: id, IP: host, Port: port, Severity: severity,
		Finding: label + " cipher suites accepted: " + strings.Join(accepted, ", "),
	}
}

// tlsMaxRecordBody bounds record payloads the raw probe is willing to read.
const tlsMaxRecordBody = 18432

// tlsProbeHandshake splits the TCP connect from the TLS handshake so callers
// can tell "the server answered and refused" apart from "no TLS endpoint was
// ever reached". The returned error on failure is the handshake error alone.
func tlsProbeHandshake(ctx context.Context, address string, config *tls.Config, timeout time.Duration) (*tls.Conn, error) {
	dialer := &net.Dialer{Timeout: timeout}
	raw, err := dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		return nil, &tlsTransportFailure{cause: err}
	}
	conn := tls.Client(raw, config)
	conn.SetDeadline(time.Now().Add(timeout))
	if err := conn.HandshakeContext(ctx); err != nil {
		_ = raw.Close()
		return nil, err
	}
	return conn, nil
}

// tlsTransportFailure marks a probe that never reached a TLS-speaking server.
type tlsTransportFailure struct{ cause error }

func (e *tlsTransportFailure) Error() string { return e.cause.Error() }
func (e *tlsTransportFailure) Unwrap() error { return e.cause }

// tlsHandshakeRejected reports whether the server actively refused the
// handshake (alert, no shared suite/version, bad certificate...). Only then is
// a "not offered" statement about the server justified.
func tlsHandshakeRejected(err error) bool {
	var transport *tlsTransportFailure
	return !errors.As(err, &transport)
}

// tlsTransportError renders the reason a probe never reached the server.
func tlsTransportError(err error) string {
	var transport *tlsTransportFailure
	if errors.As(err, &transport) {
		var netErr net.Error
		if errors.As(transport.cause, &netErr) && netErr.Timeout() {
			return "connect timeout"
		}
		return transport.cause.Error()
	}
	return err.Error()
}

// probeHeartbleed performs a real CVE-2014-0160 probe: it crafts a raw TLS 1.2
// ClientHello that offers the RFC 6520 heartbeat extension (crypto/tls never
// offers it, and servers only enable heartbeat processing when it was
// offered), waits for ServerHelloDone, then sends a malformed heartbeat
// request declaring a 0x4000-byte payload while carrying none. A heartbeat
// response echoing back more than the 16 bytes of padding we sent proves the
// server copied out-of-bounds memory. Alerts, connection closes and timeouts
// all mean the server did not leak.
func probeHeartbleed(ctx context.Context, address, host string, timeout time.Duration) (string, string) {
	dialer := &net.Dialer{Timeout: timeout}
	conn, err := dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		return "INFO", "heartbleed probe inconclusive: connect failed (" + err.Error() + ")"
	}
	defer func() { _ = conn.Close() }()
	deadline := time.Now().Add(timeout)
	if ctxDeadline, ok := ctx.Deadline(); ok && ctxDeadline.Before(deadline) {
		deadline = ctxDeadline
	}
	_ = conn.SetDeadline(deadline)
	if _, err := conn.Write(buildHeartbeatClientHello(host)); err != nil {
		return "INFO", "heartbleed probe inconclusive: client hello write failed"
	}
	serverVersion, ok := readHandshakeUntilServerHelloDone(conn)
	if !ok {
		return "INFO", "heartbleed probe inconclusive: server did not reach a TLS 1.2 ServerHelloDone (TLS 1.3 only or handshake refused)"
	}
	request := []byte{0x01, 0x40, 0x00}            // heartbeat request declaring a 0x4000-byte payload
	request = append(request, make([]byte, 16)...) // RFC 6520 minimum padding, zero actual payload
	if _, err := conn.Write(tlsRecord(24, serverVersion, request)); err != nil {
		return "OK", "not vulnerable (server closed connection before heartbeat response)"
	}
	_ = conn.SetDeadline(time.Now().Add(timeout))
	for range 4 {
		header := make([]byte, 5)
		if _, err := io.ReadFull(conn, header); err != nil {
			return "OK", "not vulnerable (no heartbeat response: " + heartbleedReadError(err) + ")"
		}
		length := int(header[3])<<8 | int(header[4])
		if length > tlsMaxRecordBody {
			length = tlsMaxRecordBody
		}
		body := make([]byte, length)
		_, _ = io.ReadFull(conn, body)
		switch header[0] {
		case 24: // heartbeat record
			if len(body) >= 19 && body[0] == 2 {
				return "HIGH", "VULNERABLE - server echoed " + strconv.Itoa(len(body)-3) + " bytes of process memory (CVE-2014-0160)"
			}
			return "OK", "not vulnerable (malformed heartbeat rejected)"
		case 21: // alert
			return "OK", "not vulnerable (server alert)"
		case 22, 23, 20: // handshake/application data or ChangeCipherSpec: keep listening briefly
			continue
		default:
			return "OK", "not vulnerable (no heartbeat response)"
		}
	}
	return "OK", "not vulnerable (no heartbeat response)"
}

func heartbleedReadError(err error) string {
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return "timeout waiting for response"
	}
	return "connection closed"
}

// readHandshakeUntilServerHelloDone consumes raw handshake records until the
// ServerHelloDone message, returning the negotiated record version. It never
// performs key agreement: the heartbeat probe works pre-completion, exactly
// like the nmap ssl-heartbleed script.
func readHandshakeUntilServerHelloDone(conn net.Conn) (uint16, bool) {
	serverVersion := uint16(0x0303)
	var pending []byte
	for range 32 {
		header := make([]byte, 5)
		if _, err := io.ReadFull(conn, header); err != nil {
			return 0, false
		}
		length := int(header[3])<<8 | int(header[4])
		if length > tlsMaxRecordBody {
			return 0, false
		}
		body := make([]byte, length)
		if _, err := io.ReadFull(conn, body); err != nil {
			return 0, false
		}
		switch header[0] {
		case 21: // alert during handshake
			return 0, false
		case 22: // handshake
			if len(pending)+len(body) > 1<<20 {
				return 0, false
			}
			pending = append(pending, body...)
			for len(pending) >= 4 {
				messageLength := int(pending[1])<<16 | int(pending[2])<<8 | int(pending[3])
				if len(pending) < 4+messageLength {
					break
				}
				messageType := pending[0]
				message := pending[4 : 4+messageLength]
				pending = pending[4+messageLength:]
				switch messageType {
				case 2: // ServerHello
					if len(message) >= 2 {
						serverVersion = uint16(message[0])<<8 | uint16(message[1])
					}
				case 14: // ServerHelloDone
					return serverVersion, true
				}
			}
		case 20: // ChangeCipherSpec mid-handshake from an unusual server
			continue
		default:
			return 0, false
		}
	}
	return 0, false
}

// buildHeartbeatClientHello crafts the raw TLS 1.2 ClientHello the heartbeat
// probe needs: enough suites/curves for a common server to reach
// ServerHelloDone, plus the heartbeat extension that switches a vulnerable
// server into heartbeat processing.
func buildHeartbeatClientHello(host string) []byte {
	random := make([]byte, 32)
	if _, err := crand.Read(random); err != nil {
		binary.BigEndian.PutUint64(random, uint64(time.Now().UnixNano()))
	}
	suites := []uint16{
		0xC02F, 0xC030, 0xC02B, // ECDHE with AES-GCM
		0xC013, 0xC014, 0xC009, // ECDHE with AES-CBC
		0x009C, 0x002F, 0x0035, // RSA key exchange
	}
	body := []byte{0x03, 0x03} // client_version TLS 1.2
	body = append(body, random...)
	body = append(body, 0x00) // empty session id
	body = append(body, byte(len(suites)*2>>8), byte(len(suites)*2))
	for _, suite := range suites {
		body = append(body, byte(suite>>8), byte(suite))
	}
	body = append(body, 0x01, 0x00) // compression methods: null

	var extensions []byte
	if name, ok := heartbeatServerName(host); ok {
		sni := []byte{byte((len(name) + 3) >> 8), byte(len(name) + 3), 0x00, byte(len(name))}
		sni = append(sni, name...)
		extensions = append(extensions, tlsExtension(0x0000, sni)...)
	}
	groups := []uint16{0x001D, 0x0017, 0x0018} // x25519, secp256r1, secp384r1
	groupData := []byte{byte(len(groups) * 2 >> 8), byte(len(groups) * 2)}
	for _, group := range groups {
		groupData = append(groupData, byte(group>>8), byte(group))
	}
	extensions = append(extensions, tlsExtension(0x000A, groupData)...)
	extensions = append(extensions, tlsExtension(0x000B, []byte{0x01, 0x00})...) // ec_point_formats: uncompressed
	signatures := []uint16{0x0403, 0x0401, 0x0501, 0x0804, 0x0805, 0x0201}
	signatureData := []byte{byte(len(signatures) * 2 >> 8), byte(len(signatures) * 2)}
	for _, signature := range signatures {
		signatureData = append(signatureData, byte(signature>>8), byte(signature))
	}
	extensions = append(extensions, tlsExtension(0x000D, signatureData)...)
	extensions = append(extensions, tlsExtension(0xFF01, []byte{0x00})...) // renegotiation_info: empty
	extensions = append(extensions, tlsExtension(0x0017, nil)...)          // extended_master_secret
	extensions = append(extensions, tlsExtension(0x000F, nil)...)          // heartbeat: empty

	body = append(body, byte(len(extensions)>>8), byte(len(extensions)))
	body = append(body, extensions...)
	return tlsRecord(22, 0x0303, tlsHandshakeMessage(1, body))
}

func heartbeatServerName(host string) ([]byte, bool) {
	host = strings.ToLower(strings.TrimSpace(host))
	if host == "" || net.ParseIP(host) != nil || len(host) > 255 {
		return nil, false
	}
	return []byte(host), true
}

func tlsHandshakeMessage(messageType byte, body []byte) []byte {
	out := make([]byte, 4+len(body))
	out[0] = messageType
	out[1] = byte(len(body) >> 16)
	out[2] = byte(len(body) >> 8)
	out[3] = byte(len(body))
	copy(out[4:], body)
	return out
}

func tlsRecord(recordType byte, version uint16, payload []byte) []byte {
	out := make([]byte, 5+len(payload))
	out[0] = recordType
	out[1] = byte(version >> 8)
	out[2] = byte(version)
	out[3] = byte(len(payload) >> 8)
	out[4] = byte(len(payload))
	copy(out[5:], payload)
	return out
}

func tlsExtension(extensionType uint16, data []byte) []byte {
	out := make([]byte, 4+len(data))
	out[0] = byte(extensionType >> 8)
	out[1] = byte(extensionType)
	out[2] = byte(len(data) >> 8)
	out[3] = byte(len(data))
	copy(out[4:], data)
	return out
}
