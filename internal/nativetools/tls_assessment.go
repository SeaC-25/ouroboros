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

// AssessTLS performs a per-version handshake and certificate inspection,
// emitting sslscan-compatible XML. Vulnerability probes (Heartbleed, ROBOT,
// CCS injection) are out of scope.
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

		dialer := &net.Dialer{Timeout: opts.timeout()}
		conn, err := tls.DialWithDialer(dialer, "tcp", address, &tls.Config{
			MinVersion:         candidate.Version,
			MaxVersion:         candidate.Version,
			InsecureSkipVerify: true,
		})
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
		}
		document.Protocols = append(document.Protocols, entry)
	}

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
	case tls.TLS_AES_128_GCM_SHA256:
		return 128
	case tls.TLS_AES_256_GCM_SHA384, tls.TLS_CHACHA20_POLY1305_SHA256:
		return 256
	}
	return 0
}
