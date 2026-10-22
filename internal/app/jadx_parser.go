package app

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/ouroboros/ouroboros/internal/eventlog"
)

// jadxMetadata mirrors the jadx_metadata_json contract the native handler
// emits (internal/nativetools/apk_jadx.go: jadxMetadata).
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

// jadxParserEvents converts jadx metadata JSON into one artifact fact plus
// component surface facts.
func jadxParserEvents(runID string, request ToolExecutionRequest, result ToolExecutionResult) ([]eventlog.Event, error) {
	if len(result.Transcript) > apkToolParserMaxOutputBytes {
		return nil, fmt.Errorf("jadx parser output exceeds %d bytes", apkToolParserMaxOutputBytes)
	}
	output := strings.TrimSpace(result.Transcript)
	if output == "" {
		return nil, fmt.Errorf("jadx parser output is empty")
	}
	if strings.TrimSpace(request.Node.ToolID) != "jadx" || strings.TrimSpace(request.Node.Operation) != "apk_decompile" {
		return nil, fmt.Errorf("jadx parser requires jadx/apk_decompile tool execution")
	}
	var metadata jadxMetadata
	if err := json.Unmarshal([]byte(output), &metadata); err != nil {
		return nil, fmt.Errorf("parse jadx metadata: %w", err)
	}

	factCount := 1
	for _, components := range [][]string{metadata.Activities, metadata.Services, metadata.Receivers, metadata.Permissions, metadata.ExportedComponents} {
		factCount += len(components)
	}
	events := []eventlog.Event{parserStatusEvent(runID, request, result, "jadx_metadata_json", "parsed", "", factCount)}
	events = append(events, jadxArtifactFactEvent(runID, request, result, metadata))
	for _, activity := range metadata.Activities {
		events = append(events, jadxComponentFactEvent(runID, request, result, "activity", activity))
	}
	for _, service := range metadata.Services {
		events = append(events, jadxComponentFactEvent(runID, request, result, "service", service))
	}
	for _, receiver := range metadata.Receivers {
		events = append(events, jadxComponentFactEvent(runID, request, result, "receiver", receiver))
	}
	for _, permission := range metadata.Permissions {
		events = append(events, jadxComponentFactEvent(runID, request, result, "permission", permission))
	}
	for _, exported := range metadata.ExportedComponents {
		events = append(events, jadxComponentFactEvent(runID, request, result, "exported", exported))
	}
	return events, nil
}

// jadxArtifactFactEvent summarizes the whole APK inventory.
func jadxArtifactFactEvent(runID string, request ToolExecutionRequest, result ToolExecutionResult, metadata jadxMetadata) eventlog.Event {
	statement := "jadx | " + metadata.Package + " | version_name=" + metadata.VersionName +
		" version_code=" + metadata.VersionCode +
		" min_sdk=" + strconv.Itoa(metadata.MinSDK) +
		" target_sdk=" + strconv.Itoa(metadata.TargetSDK) +
		" dex_methods=" + strconv.FormatUint(metadata.TotalDexMethods, 10)
	return eventlog.Event{
		RunID: runID,
		Type:  eventlog.EventFact,
		Payload: mustJSON(map[string]any{
			"fact_id":             strings.Join([]string{"fact", "jadx", safeFactIDPart(request.Node.ID), "artifact"}, "-"),
			"kind":                "artifact",
			"statement":           statement,
			"confidence":          0.9,
			"source":              "parser.jadx_metadata_json",
			"parser":              "jadx_metadata_json",
			"parser_id":           "jadx_metadata_json",
			"plan_node_id":        request.Node.ID,
			"tool":                request.Node.ManifestName,
			"tool_id":             strings.TrimSpace(request.Node.ToolID),
			"operation":           strings.TrimSpace(request.Node.Operation),
			"execution_id":        parserExecutionID(request, result),
			"package":             metadata.Package,
			"version_name":        metadata.VersionName,
			"version_code":        metadata.VersionCode,
			"min_sdk":             metadata.MinSDK,
			"target_sdk":          metadata.TargetSDK,
			"activities":          metadata.Activities,
			"services":            metadata.Services,
			"receivers":           metadata.Receivers,
			"permissions":         metadata.Permissions,
			"exported_components": metadata.ExportedComponents,
			"dex_files":           metadata.DexFiles,
			"source_files":        metadata.SourceFiles,
			"entry_count":         metadata.EntryCount,
			"total_dex_methods":   metadata.TotalDexMethods,
		}),
	}
}

// jadxComponentFactEvent emits one surface fact per manifest component.
func jadxComponentFactEvent(runID string, request ToolExecutionRequest, result ToolExecutionResult, componentType, name string) eventlog.Event {
	return eventlog.Event{
		RunID: runID,
		Type:  eventlog.EventFact,
		Payload: mustJSON(map[string]any{
			"fact_id":        strings.Join([]string{"fact", "jadx", safeFactIDPart(request.Node.ID), safeFactIDPart(componentType), safeFactIDPart(name)}, "-"),
			"kind":           "surface",
			"statement":      "jadx | " + componentType + " | " + name,
			"confidence":     0.9,
			"source":         "parser.jadx_metadata_json",
			"parser":         "jadx_metadata_json",
			"parser_id":      "jadx_metadata_json",
			"plan_node_id":   request.Node.ID,
			"tool":           request.Node.ManifestName,
			"tool_id":        strings.TrimSpace(request.Node.ToolID),
			"operation":      strings.TrimSpace(request.Node.Operation),
			"execution_id":   parserExecutionID(request, result),
			"component_type": componentType,
			"component_name": name,
		}),
	}
}
