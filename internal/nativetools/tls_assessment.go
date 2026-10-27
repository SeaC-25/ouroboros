package nativetools

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/xml"
	"fmt"
	"net"
	"sort"
	"strings"
	"sync"
	"time"
)

// OpTLSAssessment is the sslscan-equivalent operation.
const OpTLSAssessment = "tls_assessment"

// sslscanXML mirrors the sslscan XML shape the sslscan_xml parser contract
// expects: inspected protocols, negotiated ciphers and the leaf certificate.
type sslscanXML struct {
	XMLName   xml.Name           `xml:"document"`
	Title     string             `xml:"title,attr,omitempty"`
	SSLCert   sslscanCertificate `xml:"sslcert"`
	Protocols []sslscanProtocol  `xml:"ssltest"`
	Ciphers   []sslscanCipher    `xml:"cipher"`
}

type sslscanProtocol struct {
	Type      string `xml:"type,attr"`
	Version   string `xml:"version,attr"`
	Supported bool   `xml:"supported,attr"`
	Accepted  bool   `xml:"accepted,attr"`
	Status    string `xml:"status"`
}

type sslscanCipher struct {
	Status   string `xml:"status,attr"`
	Protocol string `xml:"sslversion,attr"`
	Bits     int    `xml:"bits,attr"`
	Cipher   string `xml:"cipher,attr"`
}

type sslscanCertificate struct {
	Subject    string `xml:"subject"`
	Issuer     string `xml:"issuer"`
	NotBefore  string `xml:"not-valid-before"`
	NotAfter   string `xml:"not-valid-after"`
	Serial     string `xml:"serial,omitempty"`
	SigAlg     string `xml:"signature-algorithm,omitempty"`
	SelfSigned bool   `xml:"self-signed,omitempty"`
}

// tlsProbeVersions are attempted in order. SSLv2/SSLv3 are intentionally
// absent: crypto/tls cannot speak them, so the managed sslscan/testssl.sh
// binaries remain the reference for pre-TLS1.0 findings.
var tlsProbeVersions = []struct {
	Name    string
	Version uint16
}{
	{"TLSv1.0", tls.VersionTLS10},
	{"TLSv1.1", tls.VersionTLS11},
	{"TLSv1.2", tls.VersionTLS12},
	{"TLSv1.3", tls.VersionTLS13},
}

// AssessTLS performs a per-version handshake, an exhaustive per-cipher-suite
// handshake sweep (every suite crypto/tls can offer, secure and insecure) and
// certificate inspection, emitting sslscan-compatible XML. Vulnerability
// probes that need non-TLS record semantics stay with the managed sslscan.
func AssessTLS(ctx context.Context, target string, opts Options) (string, error) {
	host, port, err := splitHostPort(target, "443")
	if err != nil {
		return "", err
	}
	address := net.JoinHostPort(host, port)

	document := sslscanXML{Title: address}
	for _, candidate := range tlsProbeVersions {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		entry := sslscanProtocol{Type: "protocol", Version: candidate.Name, Status: "rejected"}

		conn, err := tlsProbeHandshake(ctx, address, &tls.Config{
			MinVersion:         candidate.Version,
			MaxVersion:         candidate.Version,
			InsecureSkipVerify: true,
		}, opts.timeout())
		if err == nil {
			entry.Supported = true
			entry.Accepted = true
			entry.Status = "accepted"
			state := conn.ConnectionState()
			if document.SSLCert.Subject == "" && len(state.PeerCertificates) > 0 {
				document.SSLCert = certificateSummary(state.PeerCertificates[0])
			}
			if name := tls.CipherSuiteName(state.CipherSuite); name != "" && !strings.HasPrefix(name, "0x") {
				document.Ciphers = append(document.Ciphers, sslscanCipher{
					Status:   "accepted",
					Protocol: candidate.Name,
					Cipher:   name,
					Bits:     cipherBits(state.CipherSuite),
				})
			}
			_ = conn.Close()
		} else if !tlsHandshakeRejected(err) {
			// Transport failure: the port was never reached, so the honest
			// status is "inconclusive", not a claim that the server refused.
			entry.Status = "inconclusive: " + tlsTransportError(err)
		}
		document.Protocols = append(document.Protocols, entry)
	}

	document.Ciphers = append(document.Ciphers, cipherSuiteSweep(ctx, address, opts)...)

	if len(document.Ciphers) == 0 {
		document.Ciphers = append(document.Ciphers, sslscanCipher{
			Status:   "failed",
			Protocol: "unknown",
			Cipher:   "no accepted cipher suites",
		})
	}
	sort.Slice(document.Ciphers, func(i, j int) bool {
		if document.Ciphers[i].Protocol != document.Ciphers[j].Protocol {
			return document.Ciphers[i].Protocol < document.Ciphers[j].Protocol
		}
		return document.Ciphers[i].Cipher < document.Ciphers[j].Cipher
	})

	encoded, err := xml.MarshalIndent(document, "", "  ")
	if err != nil {
		return "", fmt.Errorf("encode sslscan document: %w", err)
	}
	return string(encoded), nil
}

// tlsSuiteCandidate is one offerable cipher suite. TLS 1.3 suites cannot be
// pinned individually (the handshake ignores Config.CipherSuites for 1.3, and
// they are always offered), so they are probed as a set on their own version;
// TLS 1.0-1.2 suites are probed one at a time.
type tlsSuiteCandidate struct {
	id      uint16
	name    string
	tls13   bool
	bits    int
	version string
}

// tlsSuiteCandidates enumerates every cipher suite crypto/tls can offer:
// CipherSuites() plus InsecureCipherSuites(). An explicit Config.CipherSuites
// re-enables the insecure RC4/3DES/RSA-kex suites on the wire, so the sweep
// can detect servers still accepting them. TLS 1.3 suites are probed as a set
// (the 1.3 client hello always carries all of them; Config.CipherSuites is
// ignored for 1.3), each reported individually when the 1.3 handshake picks it.
func tlsSuiteCandidates() []tlsSuiteCandidate {
	candidates := []tlsSuiteCandidate{}
	for _, suite := range tls.CipherSuites() {
		if suite.SupportedVersions[0] == tls.VersionTLS13 {
			candidates = append(candidates, tlsSuiteCandidate{
				id:      suite.ID,
				name:    suite.Name,
				tls13:   true,
				bits:    cipherBits(suite.ID),
				version: "TLSv1.3",
			})
			continue
		}
		candidates = append(candidates, tlsSuiteCandidate{
			id:      suite.ID,
			name:    suite.Name,
			bits:    cipherBits(suite.ID),
			version: tlsSuiteMinVersion(suite.ID),
		})
	}
	for _, suite := range tls.InsecureCipherSuites() {
		candidates = append(candidates, tlsSuiteCandidate{
			id:      suite.ID,
			name:    suite.Name,
			bits:    cipherBits(suite.ID),
			version: tlsSuiteMinVersion(suite.ID),
		})
	}
	return candidates
}

// tlsSuiteMinVersion returns the protocol version the suite must be probed
// on: suites valid up to TLS 1.2 are probed on 1.2 (their newest common
// version), SHA256/known TLS1.2-only suites also on 1.2; the legacy floor is
// only needed when a server offers nothing newer.
func tlsSuiteMinVersion(id uint16) string {
	switch id {
	case tls.TLS_RSA_WITH_AES_128_CBC_SHA256, tls.TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA256,
		tls.TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA256, tls.TLS_RSA_WITH_AES_128_GCM_SHA256,
		tls.TLS_RSA_WITH_AES_256_GCM_SHA384,
		tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256, tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
		tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256, tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
		tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256, tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256:
		return "TLSv1.2"
	}
	return "TLSv1.0"
}

// tlsVersionMin maps a protocol label to the minimum version for a pinned
// handshake.
func tlsVersionMin(label string) uint16 {
	switch label {
	case "TLSv1.2":
		return tls.VersionTLS12
	case "TLSv1.1":
		return tls.VersionTLS11
	}
	return tls.VersionTLS10
}

// cipherSuiteSweep pins each TLS 1.0-1.2 cipher suite in its own handshake
// (MaxVersion=TLS1.2, so the server cannot dodge into 1.3) and reports the
// suites the server actually accepts. The TLS 1.3 set is probed once and each
// of the three suites is reported accepted if 1.3 itself is accepted.
func cipherSuiteSweep(ctx context.Context, address string, opts Options) []sslscanCipher {
	candidates := tlsSuiteCandidates()
	accepted := make([]sslscanCipher, len(candidates))
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, min(opts.maxWorkers(), 8))
	for index, candidate := range candidates {
		if candidate.tls13 {
			continue
		}
		wg.Add(1)
		go func(index int, candidate tlsSuiteCandidate) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			if err := ctx.Err(); err != nil {
				return
			}
			conn, err := tlsProbeHandshake(ctx, address, &tls.Config{
				MinVersion:         tlsVersionMin(candidate.version),
				MaxVersion:         tls.VersionTLS12,
				CipherSuites:       []uint16{candidate.id},
				InsecureSkipVerify: true,
			}, opts.timeout())
			if err != nil {
				return
			}
			state := conn.ConnectionState()
			_ = conn.Close()
			if state.CipherSuite != candidate.id {
				return
			}
			mu.Lock()
			accepted[index] = sslscanCipher{
				Status:   "accepted",
				Protocol: negotiatedVersionLabel(state.Version),
				Bits:     cipherBits(state.CipherSuite),
				Cipher:   candidate.name,
			}
			mu.Unlock()
		}(index, candidate)
	}
	wg.Wait()

	// TLS 1.3: the 1.3 client hello always carries all three 1.3 suites
	// (Config.CipherSuites is ignored for 1.3), so a single handshake proves
	// the whole set: all three reported accepted when 1.3 works, none when
	// the server refuses 1.3.
	if conn, err := tlsProbeHandshake(ctx, address, &tls.Config{
		MinVersion:         tls.VersionTLS13,
		MaxVersion:         tls.VersionTLS13,
		InsecureSkipVerify: true,
	}, opts.timeout()); err == nil {
		_ = conn.Close()
		for index, candidate := range candidates {
			if candidate.tls13 {
				accepted[index] = sslscanCipher{
					Status:   "accepted",
					Protocol: candidate.version,
					Bits:     candidate.bits,
					Cipher:   candidate.name,
				}
			}
		}
	}

	results := make([]sslscanCipher, 0, len(candidates))
	seen := map[string]bool{}
	for _, entry := range accepted {
		if entry.Cipher == "" {
			continue
		}
		// The same suite can be accepted on more than one protocol version;
		// keep one row per (version, cipher) pair.
		key := entry.Protocol + "/" + entry.Cipher
		if seen[key] {
			continue
		}
		seen[key] = true
		results = append(results, entry)
	}
	return results
}

// negotiatedVersionLabel names the protocol version a handshake concluded on.
func negotiatedVersionLabel(version uint16) string {
	switch version {
	case tls.VersionTLS13:
		return "TLSv1.3"
	case tls.VersionTLS12:
		return "TLSv1.2"
	case tls.VersionTLS11:
		return "TLSv1.1"
	}
	return "TLSv1.0"
}

func splitHostPort(target, defaultPort string) (string, string, error) {
	target = strings.TrimSpace(target)
	target = strings.TrimPrefix(strings.TrimPrefix(target, "https://"), "http://")
	target = strings.TrimSuffix(target, "/")
	if target == "" {
		return "", "", fmt.Errorf("tls_assessment requires a host target")
	}
	if host, port, err := net.SplitHostPort(target); err == nil {
		return host, port, nil
	}
	return target, defaultPort, nil
}

func certificateSummary(cert *x509.Certificate) sslscanCertificate {
	return sslscanCertificate{
		Subject:    cert.Subject.String(),
		Issuer:     cert.Issuer.String(),
		NotBefore:  cert.NotBefore.UTC().Format(time.RFC3339),
		NotAfter:   cert.NotAfter.UTC().Format(time.RFC3339),
		Serial:     cert.SerialNumber.String(),
		SigAlg:     cert.SignatureAlgorithm.String(),
		SelfSigned: cert.Subject.String() == cert.Issuer.String(),
	}
}

// cipherBits reports the effective symmetric key size for the suites Go can
// negotiate.
func cipherBits(id uint16) int {
	switch id {
	case tls.TLS_AES_128_GCM_SHA256,
		tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256, tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
		tls.TLS_RSA_WITH_AES_128_GCM_SHA256,
		tls.TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA256, tls.TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA256,
		tls.TLS_RSA_WITH_AES_128_CBC_SHA256,
		tls.TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA, tls.TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA,
		tls.TLS_RSA_WITH_AES_128_CBC_SHA,
		tls.TLS_RSA_WITH_RC4_128_SHA, tls.TLS_ECDHE_RSA_WITH_RC4_128_SHA,
		tls.TLS_ECDHE_ECDSA_WITH_RC4_128_SHA:
		return 128
	case tls.TLS_AES_256_GCM_SHA384, tls.TLS_CHACHA20_POLY1305_SHA256,
		tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384, tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
		tls.TLS_RSA_WITH_AES_256_GCM_SHA384,
		tls.TLS_ECDHE_ECDSA_WITH_AES_256_CBC_SHA, tls.TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA,
		tls.TLS_RSA_WITH_AES_256_CBC_SHA:
		return 256
	case tls.TLS_ECDHE_RSA_WITH_3DES_EDE_CBC_SHA, tls.TLS_RSA_WITH_3DES_EDE_CBC_SHA:
		return 112
	}
	return 0
}
