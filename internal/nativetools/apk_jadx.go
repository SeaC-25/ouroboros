package nativetools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// jadxMetadata is the jadx_metadata_json contract: the APK inventory the
// jadx parser turns into artifact and surface facts. The jadx CLI itself has
// no JSON mode; this shape is the repository's own contract
// (internal/toolchain/authored_registry.go: jadx_metadata_json).
type jadxMetadata struct {
	Package            string   `json:"package"`
	VersionName        string   `json:"version_name"`
	VersionCode        string   `json:"version_code"`
	MinSDK             int      `json:"min_sdk"`
	TargetSDK          int      `json:"target_sdk"`
	Activities         []string `json:"activities"`
	Services           []string `json:"services"`
	Receivers          []string `json:"receivers"`
	Permissions        []string `json:"permissions"`
	ExportedComponents []string `json:"exported_components"`
	DexFiles           []string `json:"dex_files"`
	SourceFiles        []string `json:"source_files"`
	EntryCount         int      `json:"entry_count"`
	TotalDexMethods    uint64   `json:"total_dex_methods"`
}

// DecompileAPKJadx is the jadx-equivalent subset: it reports the APK
// inventory (manifest, components, dex method census) as JSON without
// decompiling dex to Java sources. The managed jadx remains the reference
// for full source recovery.
func DecompileAPKJadx(ctx context.Context, target string, _ Options) (string, error) {
	target = strings.TrimSpace(target)
	if target == "" {
		return "", errors.New("apk_decompile requires an artifact path")
	}
	manifest, err := readAPKManifest(target)
	if err != nil {
		return "", err
	}

	metadata := jadxMetadata{
		Package:            manifest.Package,
		VersionName:        manifest.VersionName,
		VersionCode:        manifest.VersionCode,
		MinSDK:             manifest.MinSDK,
		TargetSDK:          manifest.TargetSDK,
		Activities:         manifest.Activities,
		Services:           manifest.Services,
		Receivers:          manifest.Receivers,
		Permissions:        manifest.Permissions,
		ExportedComponents: manifest.ExportedComponents,
		DexFiles:           manifest.DexFiles,
		SourceFiles:        nil, // subset: no dex→Java decompilation
		EntryCount:         len(manifest.DexFiles),
		TotalDexMethods:    manifest.TotalDexMethods,
	}
	if metadata.Activities == nil {
		metadata.Activities = []string{}
	}
	if metadata.Services == nil {
		metadata.Services = []string{}
	}
	if metadata.Receivers == nil {
		metadata.Receivers = []string{}
	}
	if metadata.Permissions == nil {
		metadata.Permissions = []string{}
	}
	if metadata.ExportedComponents == nil {
		metadata.ExportedComponents = []string{}
	}
	if metadata.DexFiles == nil {
		metadata.DexFiles = []string{}
	}
	if metadata.SourceFiles == nil {
		metadata.SourceFiles = []string{}
	}

	encoded, err := json.Marshal(metadata)
	if err != nil {
		return "", fmt.Errorf("encode jadx metadata: %w", err)
	}
	return string(encoded), nil
}
