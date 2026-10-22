package app

import (
	"encoding/xml"
	"fmt"
	"strconv"
	"strings"
)

// sslscanDocument is the sslscan XML skeleton this parser consumes.
type sslscanDocument struct {
	XMLName   xml.Name             `xml:"document"`
	Protocols []sslscanXMLProtocol `xml:"ssltest"`
	Ciphers   []sslscanXMLCipher   `xml:"cipher"`
	Cert      sslscanXMLCert       `xml:"sslcert"`
}

type sslscanXMLProtocol struct {
	Type      string `xml:"type,attr"`
	Version   string `xml:"version,attr"`
	Supported string `xml:"supported,attr"`
	Accepted  string `xml:"accepted,attr"`
	Status    string `xml:"status"`
}

type sslscanXMLCipher struct {
	Status   string `xml:"status,attr"`
	Protocol string `xml:"sslversion,attr"`
	Bits     string `xml:"bits,attr"`
	Cipher   string `xml:"cipher,attr"`
}

type sslscanXMLCert struct {
	Subject    string `xml:"subject"`
	Issuer     string `xml:"issuer"`
	NotBefore  string `xml:"not-valid-before"`
	NotAfter   string `xml:"not-valid-after"`
	SelfSigned string `xml:"self-signed"`
}

// sslscanFindingsFromXML flattens an sslscan document into TLS findings.
func sslscanFindingsFromXML(output string) ([]tlsFinding, error) {
	var document sslscanDocument
	if err := xml.Unmarshal([]byte(output), &document); err != nil {
		return nil, fmt.Errorf("parse sslscan XML: %w", err)
	}

	findings := []tlsFinding{}
	for _, protocol := range document.Protocols {
		accepted := strings.EqualFold(protocol.Accepted, "true") || strings.EqualFold(protocol.Supported, "true")
		severity := "OK"
		description := protocol.Version + " not offered"
		if accepted {
			description = protocol.Version + " offered"
			if isDeprecatedTLS(protocol.Version) {
				severity = "MEDIUM"
				description = protocol.Version + " offered (deprecated protocol)"
			}
		} else {
			// Refusing a deprecated protocol is the desired outcome.
			severity = "OK"
		}
		findings = append(findings, tlsFinding{
			ID:       "protocol_" + protocol.Version,
			Severity: severity,
			Finding:  description,
			Protocol: protocol.Version,
		})
	}

	for _, cipher := range document.Ciphers {
		if !strings.EqualFold(cipher.Status, "accepted") {
			continue
		}
		bits := 0
		if parsed, err := strconv.Atoi(strings.TrimSpace(cipher.Bits)); err == nil {
			bits = parsed
		}
		severity := "INFO"
		if bits > 0 && bits < 128 {
			severity = "MEDIUM"
		}
		findings = append(findings, tlsFinding{
			ID:       "cipher_suite",
			Severity: severity,
			Finding:  cipher.Protocol + " accepted " + cipher.Cipher,
			Protocol: cipher.Protocol,
		})
	}

	if cert := document.Cert; cert.Subject != "" || cert.NotAfter != "" {
		severity := "OK"
		finding := "certificate subject " + cert.Subject + " issued by " + cert.Issuer
		if strings.EqualFold(strings.TrimSpace(cert.SelfSigned), "true") {
			severity = "MEDIUM"
			finding = "self-signed certificate for " + cert.Subject
		}
		findings = append(findings, tlsFinding{ID: "certificate", Severity: severity, Finding: finding})
	}
	return findings, nil
}

func isDeprecatedTLS(version string) bool {
	switch strings.ToUpper(strings.TrimSpace(version)) {
	case "TLSV1.0", "TLSV1.1", "SSLV2", "SSLV3":
		return true
	default:
		return false
	}
}
