package artifact

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestVaultWriteReadRoundTrip(t *testing.T) {
	vault, err := OpenVault(t.TempDir())
	if err != nil {
		t.Fatalf("open vault: %v", err)
	}

	ref, err := vault.Write("response.txt", "text/plain", []byte("HTTP/1.1 200 OK\n"))
	if err != nil {
		t.Fatalf("write artifact: %v", err)
	}
	if ref.Digest == "" {
		t.Fatalf("expected digest")
	}
	if ref.Size != int64(len("HTTP/1.1 200 OK\n")) {
		t.Fatalf("unexpected size: %d", ref.Size)
	}
	if ref.MediaType != "text/plain" {
		t.Fatalf("unexpected media type: %s", ref.MediaType)
	}

	data, err := vault.Read(ref.Digest)
	if err != nil {
		t.Fatalf("read artifact: %v", err)
	}
	if !bytes.Equal(data, []byte("HTTP/1.1 200 OK\n")) {
		t.Fatalf("unexpected artifact data: %q", string(data))
	}
	if _, err := os.Stat(ref.Path); err != nil {
		t.Fatalf("expected artifact file at ref path: %v", err)
	}
}

func TestVaultDeduplicatesDuplicateContent(t *testing.T) {
	vault, err := OpenVault(t.TempDir())
	if err != nil {
		t.Fatalf("open vault: %v", err)
	}

	first, err := vault.Write("first.log", "text/plain", []byte("same content"))
	if err != nil {
		t.Fatalf("write first: %v", err)
	}
	second, err := vault.Write("second.log", "text/plain", []byte("same content"))
	if err != nil {
		t.Fatalf("write second: %v", err)
	}

	if first.Digest != second.Digest {
		t.Fatalf("expected same digest, got %s and %s", first.Digest, second.Digest)
	}
	if first.Path != second.Path {
		t.Fatalf("expected same content path, got %s and %s", first.Path, second.Path)
	}
}

func TestVaultRejectsInvalidDigestRead(t *testing.T) {
	vault, err := OpenVault(t.TempDir())
	if err != nil {
		t.Fatalf("open vault: %v", err)
	}

	_, err = vault.Read("../escape")
	if err == nil {
		t.Fatalf("expected invalid digest to fail")
	}
	if !strings.Contains(err.Error(), "invalid digest") {
		t.Fatalf("expected invalid digest error, got %v", err)
	}
}
