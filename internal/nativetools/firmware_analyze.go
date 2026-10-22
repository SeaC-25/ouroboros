package nativetools

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// OpFirmwareAnalyze is the binwalk-equivalent (signature scan) operation.
const OpFirmwareAnalyze = "firmware_analyze"

// firmwareSignature is one magic-byte pattern.
type firmwareSignature struct {
	Offset    int
	Magic     []byte
	Label     string
	Extractor string
}

// builtInFirmwareSignatures covers the containers a CTF firmware triage needs.
// Extraction (unsquashfs, jefferson, ...) stays out of scope; those paths are
// reported so the operator can hand the artifact to the managed toolchain.
var builtInFirmwareSignatures = []firmwareSignature{
	{0, []byte{0x1F, 0x8B}, "gzip compressed data", "gzip"},
	{0, []byte("BZh"), "bzip2 compressed data", "bzip2"},
	{0, []byte{0xFD, '7', 'z', 'X', 'Z', 0x00}, "xz compressed data", "xz"},
	{0, []byte("hsqs"), "Squashfs filesystem", "unsquashfs"},
	{0, []byte{0x27, 0x05, 0x19, 0x56}, "uImage header (kernel)", "dd"},
	{0, []byte{0xD0, 0x0D, 0xFE, 0xED}, "Linux kernel ARM boot executable", "dd"},
	{0, []byte{0x7F, 'E', 'L', 'F'}, "ELF executable", "readelf"},
	{0, []byte("PK\x03\x04"), "Zip archive data", "unzip"},
	{0, []byte{0x85, 0x19}, "JFFS2 filesystem", "jefferson"},
	{0, []byte("UBI#"), "UBI erase count header", "ubireader"},
	{0, []byte("ANDROID!"), "Android bootimg", "unpackbootimg"},
	{0, []byte("CRAMFS"), "CRAMFS filesystem", "cramfsck"},
	{0, []byte("SQSH"), "Squashfs (big-endian)", "unsquashfs"},
	{0, []byte{'P', 'K', 0x05, 0x06}, "Zip archive (empty)", "unzip"},
}

// FirmwareFindings is one scan result line.
type FirmwareFinding struct {
	Offset    int
	Label     string
	Extractor string
}

// AnalyzeFirmware scans the target for embedded file signatures and returns the
// binwalk-style text report (decimal offset, description, extraction hint).
func AnalyzeFirmware(ctx context.Context, target string, _ Options) (string, error) {
	target = strings.TrimSpace(target)
	if target == "" {
		return "", errors.New("firmware_analyze requires a file path")
	}
	info, err := os.Stat(target)
	if err != nil {
		return "", fmt.Errorf("stat firmware target: %w", err)
	}
	if info.IsDir() {
		return "", fmt.Errorf("firmware_analyze requires a file, got directory %s", target)
	}
	data, err := os.ReadFile(target)
	if err != nil {
		return "", fmt.Errorf("read firmware target: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}

	findings := scanFirmwareSignatures(data)
	if len(findings) == 0 {
		return "", nil
	}

	builder := strings.Builder{}
	fmt.Fprintf(&builder, "DECIMAL       HEXADECIMAL     DESCRIPTION\n")
	for _, finding := range findings {
		fmt.Fprintf(&builder, "%-13d 0x%-13X %s\n", finding.Offset, finding.Offset, finding.Label)
		fmt.Fprintf(&builder, "%-13s %-13s extract with: %s\n", "", "", finding.Extractor)
	}
	names := fmt.Sprintf("scanned %d bytes of %s", len(data), filepath.Base(target))
	builder.WriteString(names)
	return strings.TrimRight(builder.String(), "\n"), nil
}

// scanFirmwareSignatures locates every known magic in data.
func scanFirmwareSignatures(data []byte) []FirmwareFinding {
	findings := []FirmwareFinding{}
	for _, signature := range builtInFirmwareSignatures {
		for offset := 0; offset+len(signature.Magic) <= len(data); {
			index := bytes.Index(data[offset:], signature.Magic)
			if index < 0 {
				break
			}
			findings = append(findings, FirmwareFinding{
				Offset:    offset + index,
				Label:     signature.Label,
				Extractor: signature.Extractor,
			})
			offset += index + 1
			if len(findings) >= 256 {
				return finishScan(findings)
			}
		}
	}
	return finishScan(findings)
}

func finishScan(findings []FirmwareFinding) []FirmwareFinding {
	sort.Slice(findings, func(i, j int) bool {
		if findings[i].Offset != findings[j].Offset {
			return findings[i].Offset < findings[j].Offset
		}
		return findings[i].Label < findings[j].Label
	})
	return findings
}
