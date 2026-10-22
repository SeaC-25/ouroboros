package nativetools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestDecompileAPKJadxEmitsMetadataJSON(t *testing.T) {
	apkPath := buildTestAPK(t)

	out, err := DecompileAPKJadx(context.Background(), apkPath, Options{Timeout: 10 * time.Second})
	if err != nil {
		t.Fatalf("DecompileAPKJadx: %v", err)
	}
	var metadata jadxMetadata
	if err := json.Unmarshal([]byte(out), &metadata); err != nil {
		t.Fatalf("decode jadx metadata: %v (%s)", err, out)
	}
	if metadata.Package != "com.example.app" {
		t.Fatalf("unexpected package: %s", metadata.Package)
	}
	if metadata.VersionName != "1.0" || metadata.VersionCode != "7" {
		t.Fatalf("unexpected version: %s/%s", metadata.VersionName, metadata.VersionCode)
	}
	if metadata.MinSDK != 21 || metadata.TargetSDK != 30 {
		t.Fatalf("unexpected sdk levels: %d/%d", metadata.MinSDK, metadata.TargetSDK)
	}
	if len(metadata.Activities) != 1 || metadata.Activities[0] != ".MainActivity" {
		t.Fatalf("unexpected activities: %#v", metadata.Activities)
	}
	if len(metadata.ExportedComponents) != 1 || metadata.ExportedComponents[0] != ".MainActivity" {
		t.Fatalf("unexpected exported components: %#v", metadata.ExportedComponents)
	}
	if len(metadata.Permissions) != 1 || metadata.Permissions[0] != "android.permission.INTERNET" {
		t.Fatalf("unexpected permissions: %#v", metadata.Permissions)
	}
	if metadata.TotalDexMethods != 42 || len(metadata.DexFiles) != 1 || metadata.DexFiles[0] != "classes.dex" {
		t.Fatalf("unexpected dex inventory: files=%#v methods=%d", metadata.DexFiles, metadata.TotalDexMethods)
	}
	if metadata.EntryCount != 1 {
		t.Fatalf("unexpected entry count: %d", metadata.EntryCount)
	}
}

func TestDecompileAPKJadxEmptyPathIsAnError(t *testing.T) {
	if _, err := DecompileAPKJadx(context.Background(), "  ", Options{Timeout: time.Second}); err == nil {
		t.Fatal("expected an error for an empty artifact path")
	}
	if _, err := DecompileAPKJadx(context.Background(), "does-not-exist.apk", Options{Timeout: time.Second}); err == nil {
		t.Fatal("expected an error for a missing artifact")
	}
}

// TestDecompileAPKJadxOutputShapeIsStable pins the JSON key contract the
// parser (internal/app/apk_parser.go) consumes.
func TestDecompileAPKJadxOutputShapeIsStable(t *testing.T) {
	apkPath := buildTestAPK(t)
	out, err := DecompileAPKJadx(context.Background(), apkPath, Options{Timeout: 10 * time.Second})
	if err != nil {
		t.Fatalf("DecompileAPKJadx: %v", err)
	}
	var raw map[string]any
	if err := json.Unmarshal([]byte(out), &raw); err != nil {
		t.Fatalf("decode raw metadata: %v", err)
	}
	for _, key := range []string{
		"package", "version_name", "version_code", "min_sdk", "target_sdk",
		"activities", "services", "receivers", "permissions", "exported_components",
		"dex_files", "source_files", "entry_count", "total_dex_methods",
	} {
		if _, ok := raw[key]; !ok {
			t.Fatalf("metadata JSON is missing key %q: %s", key, out)
		}
	}
	if strings.Contains(out, "null") {
		t.Fatalf("metadata JSON must not contain null slices: %s", out)
	}
}
