package nativetools

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"net"
	"sort"
	"strings"
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

// AssessTLSTestssl emits testssl.sh shaped JSON for the configuration checks
// crypto/tls can perform (protocol versions, cipher, certificate validity).
// Vulnerability probes (Heartbleed, ROBOT, CCS injection, ticketbleed) are out
// of scope and remain the managed testssl.sh's job.
func AssessTLSTestssl(ctx context.Context, target string, opts Options) (string, error) {
	host, port, err := splitHostPort(target, "443")
	if err != nil {
		return "", err
	}
	address := net.JoinHostPort(host, port)

	findings := []testsslFinding{}
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
		dialer := &net.Dialer{Timeout: opts.timeout()}
		conn, err := tls.DialWithDialer(dialer, "tcp", address, &tls.Config{
			MinVersion:         candidate.Version,
			MaxVersion:         candidate.Version,
			InsecureSkipVerify: true,
		})
		if err == nil {
			state := conn.ConnectionState()
			finding.Severity = "OK"
			finding.Finding = candidate.Name + " offered"
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
				findings = append(findings, certificateFindings(host, port, state.PeerCertificates[0], time.Now())...)
			}
			_ = conn.Close()
		} else if candidate.Version == tls.VersionTLS10 || candidate.Version == tls.VersionTLS11 {
			// Deprecated protocol versions being refused is the desired outcome.
			finding.Severity = "OK"
		}
		findings = append(findings, finding)
	}

	sort.SliceStable(findings, func(i, j int) bool { return findings[i].ID < findings[j].ID })
	encoded, err := json.MarshalIndent(findings, "", "  ")
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

// certificateFindings reports expiry and trust properties of the leaf cert.
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
	return findings
}
