package nativetools

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestAnalyzeFirmwareReportsKnownSignatures(t *testing.T) {
	dir := t.TempDir()
	// gzip magic, then a Squashfs magic further in.
	payload := make([]byte, 0, 64)
	payload = append(payload, 0x1F, 0x8B, 0x08, 0x00)
	payload = append(payload, make([]byte, 32)...)
	payload = append(payload, []byte("hsqs")...)
	path := filepath.Join(dir, "firmware.bin")
	if err := os.WriteFile(path, payload, 0o644); err != nil {
		t.Fatal(err)
	}

	out, err := AnalyzeFirmware(context.Background(), path, Options{})
	if err != nil {
		t.Fatalf("AnalyzeFirmware: %v", err)
	}
	if !strings.Contains(out, "gzip compressed data") {
		t.Fatalf("gzip signature missing: %s", out)
	}
	if !strings.Contains(out, "Squashfs filesystem") {
		t.Fatalf("squashfs signature missing: %s", out)
	}
	if !strings.Contains(out, "0x24") && !strings.Contains(out, "36") {
		t.Fatalf("offset of the squashfs magic missing: %s", out)
	}
}

func TestAnalyzeFirmwareRejectsDirectory(t *testing.T) {
	if _, err := AnalyzeFirmware(context.Background(), t.TempDir(), Options{}); err == nil {
		t.Fatal("expected error for a directory target")
	}
}

func TestScanSecretsTrufflehogEmitsJSONL(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "creds.txt"), []byte("key: ghp_"+strings.Repeat("b", 36)+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := ScanSecretsTrufflehog(context.Background(), dir, Options{})
	if err != nil {
		t.Fatalf("ScanSecretsTrufflehog: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) == 0 || lines[0] == "" {
		t.Fatalf("no trufflehog findings: %q", out)
	}
	var finding trufflehogFinding
	if err := json.Unmarshal([]byte(lines[0]), &finding); err != nil {
		t.Fatalf("decode trufflehog finding: %v (%s)", err, lines[0])
	}
	if finding.DetectorName != "github-pat" {
		t.Fatalf("unexpected detector: %#v", finding)
	}
	if finding.SourceMetadata.Data.Filesystem.File == "" {
		t.Fatalf("filesystem metadata missing: %#v", finding)
	}
}

func TestFingerprintServicesProducesNmapXML(t *testing.T) {
	// A local listener stands in for a network service.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = listener.Close() }()
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		_, _ = conn.Write([]byte("SSH-2.0-OpenSSH_9.6\r\n"))
		_ = conn.Close()
	}()
	port := listener.Addr().(*net.TCPAddr).Port

	out, err := FingerprintServices(context.Background(), "127.0.0.1:"+strconv.Itoa(port), Options{Timeout: 5 * time.Second, MaxWorkers: 2})
	if err != nil {
		t.Fatalf("FingerprintServices: %v", err)
	}
	var run struct {
		Hosts []struct {
			Addresses []struct {
				Addr string `xml:"addr,attr"`
			} `xml:"address"`
			Ports struct {
				Ports []struct {
					PortID int `xml:"portid,attr"`
					State  struct {
						State string `xml:"state,attr"`
					} `xml:"state"`
					Service struct {
						Name    string `xml:"name,attr"`
						Product string `xml:"product,attr"`
					} `xml:"service"`
				} `xml:"port"`
			} `xml:"ports"`
		} `xml:"host"`
	}
	if err := xml.Unmarshal([]byte(out), &run); err != nil {
		t.Fatalf("decode nmap XML: %v (%s)", err, out)
	}
	if len(run.Hosts) != 1 || len(run.Hosts[0].Ports.Ports) != 1 {
		t.Fatalf("expected one host with one open port, got %s", out)
	}
	entry := run.Hosts[0].Ports.Ports[0]
	if entry.State.State != "open" || entry.PortID != port {
		t.Fatalf("unexpected port entry: %#v", entry)
	}
	if entry.Service.Product != "SSH-2.0-OpenSSH_9.6" && !strings.Contains(entry.Service.Product, "SSH") {
		t.Fatalf("banner product not captured: %#v", entry.Service)
	}
}

func TestAssessTLSTestsslReportsProtocolFindings(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer server.Close()
	host := strings.TrimPrefix(server.URL, "https://")

	out, err := AssessTLSTestssl(context.Background(), host, Options{Timeout: 10 * time.Second})
	if err != nil {
		t.Fatalf("AssessTLSTestssl: %v", err)
	}
	var findings []map[string]any
	if err := json.Unmarshal([]byte(out), &findings); err != nil {
		t.Fatalf("decode testssl output: %v (%s)", err, out)
	}
	offered := false
	for _, finding := range findings {
		if finding["id"] == "protocols" && strings.Contains(finding["finding"].(string), "offered") {
			offered = true
		}
	}
	if !offered {
		t.Fatalf("no offered protocol finding: %s", out)
	}
}
