package nativetools

import (
	"archive/zip"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
)

// apkManifest is the AndroidManifest subset both the apktool and the jadx
// handlers extract from an APK.
type apkManifest struct {
	Package            string
	VersionName        string
	VersionCode        string
	MinSDK             int
	TargetSDK          int
	Activities         []string
	Services           []string
	Receivers          []string
	Permissions        []string
	ExportedComponents []string
	DexFiles           []string
	TotalDexMethods    uint64
}

// readAPKManifest opens the APK (a ZIP), decodes AndroidManifest.xml (binary
// AXML) and classes*.dex headers.
func readAPKManifest(apkPath string) (*apkManifest, error) {
	reader, err := zip.OpenReader(apkPath)
	if err != nil {
		return nil, fmt.Errorf("open apk: %w", err)
	}
	defer func() { _ = reader.Close() }()

	manifestEntry := -1
	dexEntries := []string{}
	for i := range reader.File {
		name := reader.File[i].Name
		switch {
		case name == "AndroidManifest.xml":
			manifestEntry = i
		case name == "classes.dex" || (strings.HasPrefix(name, "classes") && strings.HasSuffix(name, ".dex")):
			dexEntries = append(dexEntries, name)
		}
	}
	if manifestEntry < 0 {
		return nil, errors.New("apk: AndroidManifest.xml not found")
	}
	sort.Strings(dexEntries)

	manifestData, err := readZIPEntry(reader.File[manifestEntry])
	if err != nil {
		return nil, fmt.Errorf("read manifest: %w", err)
	}
	elements, err := parseAXMLManifest(manifestData)
	if err != nil {
		return nil, fmt.Errorf("parse manifest: %w", err)
	}

	manifest := &apkManifest{DexFiles: dexEntries}
	seenPermission := map[string]bool{}
	for _, element := range elements {
		switch element.Name {
		case "manifest":
			manifest.Package = element.Attributes["package"]
			manifest.VersionName = element.Attributes["versionName"]
			manifest.VersionCode = element.Attributes["versionCode"]
		case "uses-sdk":
			manifest.MinSDK = axmlAttrInt(element.Attributes["minSdkVersion"])
			manifest.TargetSDK = axmlAttrInt(element.Attributes["targetSdkVersion"])
		case "uses-permission", "permission":
			name := element.Attributes["name"]
			if name != "" && !seenPermission[name] {
				seenPermission[name] = true
				manifest.Permissions = append(manifest.Permissions, name)
			}
		case "activity", "activity-alias", "service", "receiver":
			switch element.Name {
			case "activity", "activity-alias":
				apkComponent(element, &manifest.Activities)
			case "service":
				apkComponent(element, &manifest.Services)
			case "receiver":
				apkComponent(element, &manifest.Receivers)
			}
			manifest.ExportedComponents = appendExported(manifest.ExportedComponents, element)
		}
	}

	dexEntriesByName := map[string]*zip.File{}
	for i := range reader.File {
		dexEntriesByName[reader.File[i].Name] = reader.File[i]
	}
	for _, dexName := range dexEntries {
		entry, ok := dexEntriesByName[dexName]
		if !ok {
			continue
		}
		methods, err := dexMethodCount(entry)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", dexName, err)
		}
		manifest.TotalDexMethods += methods
	}
	return manifest, nil
}

// apkComponent appends the component name and reports whether one existed.
func apkComponent(element axmlElement, target *[]string) (string, bool) {
	name := element.Attributes["name"]
	if name == "" {
		return "", false
	}
	*target = append(*target, name)
	return name, true
}

// appendExported records a component as exported when its android:exported
// attribute is set or it declares an intent-filter (the AXML subset here only
// sees start elements, so presence of exported="true" is the signal).
func appendExported(exported []string, element axmlElement) []string {
	if element.Attributes["exported"] != "true" {
		return exported
	}
	name := element.Attributes["name"]
	if name == "" {
		return exported
	}
	return append(exported, name)
}

// axmlAttrInt parses an attribute that AXML rendered as a decimal string.
func axmlAttrInt(value string) int {
	n := 0
	for _, c := range strings.TrimSpace(value) {
		if c < '0' || c > '9' {
			return 0
		}
		n = n*10 + int(c-'0')
	}
	return n
}

// readZIPEntry reads one entry fully (bounded by 4 MiB).
func readZIPEntry(entry *zip.File) ([]byte, error) {
	reader, err := entry.Open()
	if err != nil {
		return nil, err
	}
	defer func() { _ = reader.Close() }()
	data, err := io.ReadAll(io.LimitReader(reader, 4<<20))
	if err != nil {
		return nil, err
	}
	return data, nil
}

// dexMethodCount reads method_ids_size from a dex header. Per the dex format
// spec the header is 0x70 bytes: magic(8) checksum(4) signature(20) then 4-byte
// fields from 0x20 (file_size, header_size, endian_tag, link_size, link_off,
// map_off, string_ids_size/off, type_ids_size/off, proto_ids_size/off,
// field_ids_size/off), putting method_ids_size at offset 0x58.
func dexMethodCount(entry *zip.File) (uint64, error) {
	reader, err := entry.Open()
	if err != nil {
		return 0, err
	}
	defer func() { _ = reader.Close() }()
	header := make([]byte, 0x70)
	if _, err := io.ReadFull(reader, header); err != nil {
		return 0, fmt.Errorf("read dex header in %s: %w", entry.Name, err)
	}
	if string(header[:4]) != "dex\n" {
		return 0, fmt.Errorf("invalid dex magic in %s", entry.Name)
	}
	return uint64(binary.LittleEndian.Uint32(header[0x58 : 0x58+4])), nil
}

// DecompileAPKApktool is the apktool-equivalent subset: it reads the APK
// structure (manifest, components, dex inventory) without extracting resources
// or smali, and emits a compact manifest summary in apktool_text format.
func DecompileAPKApktool(ctx context.Context, target string, opts Options) (string, error) {
	if strings.TrimSpace(target) == "" {
		return "", errors.New("apk_decompile requires an artifact path")
	}
	manifest, err := readAPKManifest(strings.TrimSpace(target))
	if err != nil {
		return "", err
	}

	var builder strings.Builder
	fmt.Fprintf(&builder, "apktool | package=%s\n", manifest.Package)
	fmt.Fprintf(&builder, "apktool | versionName=%s versionCode=%s\n", manifest.VersionName, manifest.VersionCode)
	fmt.Fprintf(&builder, "apktool | sdk min=%d target=%d\n", manifest.MinSDK, manifest.TargetSDK)
	for _, permission := range manifest.Permissions {
		fmt.Fprintf(&builder, "apktool | uses-permission %s\n", permission)
	}
	for _, activity := range manifest.Activities {
		fmt.Fprintf(&builder, "apktool | activity %s\n", activity)
	}
	for _, service := range manifest.Services {
		fmt.Fprintf(&builder, "apktool | service %s\n", service)
	}
	for _, receiver := range manifest.Receivers {
		fmt.Fprintf(&builder, "apktool | receiver %s\n", receiver)
	}
	for _, exported := range manifest.ExportedComponents {
		fmt.Fprintf(&builder, "apktool | exported %s\n", exported)
	}
	fmt.Fprintf(&builder, "apktool | dex files=%d methods=%d\n", len(manifest.DexFiles), manifest.TotalDexMethods)
	return strings.TrimRight(builder.String(), "\n"), nil
}
