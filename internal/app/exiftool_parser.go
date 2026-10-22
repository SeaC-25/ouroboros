package app

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/ouroboros/ouroboros/internal/eventlog"
)

const artifactMetadataParserMaxBytes = 1 << 20

// exiftoolParserEvents converts `exiftool -json` output into artifact
// metadata facts. Until this parser existed, exiftool ran but its evidence
// never reached the event log (see docs/toolchain-go-roadmap.md §1).
func exiftoolParserEvents(runID string, request ToolExecutionRequest, result ToolExecutionResult) ([]eventlog.Event, error) {
	if len(result.Transcript) > artifactMetadataParserMaxBytes {
		return nil, fmt.Errorf("exiftool parser output exceeds %d bytes", artifactMetadataParserMaxBytes)
	}
	output := strings.TrimSpace(result.Transcript)
	if output == "" {
		return nil, fmt.Errorf("exiftool parser output is empty")
	}

	var records []map[string]any
	if err := json.Unmarshal([]byte(output), &records); err != nil {
		// exiftool emits a JSON object when a single file is inspected without
		// -json aggregation in some versions; accept both shapes.
		var single map[string]any
		if singleErr := json.Unmarshal([]byte(output), &single); singleErr != nil {
			return nil, fmt.Errorf("parse exiftool JSON: %w", err)
		}
		records = []map[string]any{single}
	}

	metadata := []artifactMetadataRecord{}
	for _, record := range records {
		parsed, ok := artifactMetadataFromRaw(record)
		if !ok {
			continue
		}
		metadata = append(metadata, parsed)
	}
	if len(metadata) == 0 {
		return nil, fmt.Errorf("exiftool output contains no metadata records")
	}
	sort.SliceStable(metadata, func(i, j int) bool { return metadata[i].Path < metadata[j].Path })

	events := []eventlog.Event{parserStatusEvent(runID, request, result, "exiftool_json", "parsed", "", len(metadata))}
	for index, record := range metadata {
		events = append(events, artifactMetadataFactEvent(runID, request, result, record, index))
	}
	return events, nil
}

type artifactMetadataRecord struct {
	Path string
	Tags map[string]string
}

func artifactMetadataFromRaw(record map[string]any) (artifactMetadataRecord, bool) {
	path := rawStringValue(record["SourceFile"])
	if path == "" {
		path = rawStringValue(record["SourceFileRelativePath"])
	}
	if path == "" {
		return artifactMetadataRecord{}, false
	}
	tags := make(map[string]string, len(record))
	for key, value := range record {
		if key == "SourceFile" || key == "SourceFileRelativePath" {
			continue
		}
		tags[key] = scalarText(value)
	}
	if len(tags) == 0 {
		return artifactMetadataRecord{}, false
	}
	return artifactMetadataRecord{Path: path, Tags: tags}, true
}

// scalarText renders a JSON scalar the way exiftool prints it (strings as-is,
// numbers without exponent for integers, arrays joined by comma).
func scalarText(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return typed
	case bool:
		return strconv.FormatBool(typed)
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64)
	case []any:
		parts := make([]string, 0, len(typed))
		for _, item := range typed {
			parts = append(parts, scalarText(item))
		}
		return strings.Join(parts, ", ")
	default:
		encoded, err := json.Marshal(typed)
		if err != nil {
			return fmt.Sprintf("%v", typed)
		}
		return string(encoded)
	}
}

func artifactMetadataFactEvent(runID string, request ToolExecutionRequest, result ToolExecutionResult, record artifactMetadataRecord, index int) eventlog.Event {
	return eventlog.Event{
		RunID: runID,
		Type:  "fact",
		Payload: mustJSON(map[string]any{
			"fact_id":      "fact-metadata-" + safeFactIDPart(request.Node.ID) + "-" + safeFactIDPart(record.Path) + "-" + strconv.Itoa(index),
			"kind":         "artifact_metadata",
			"statement":    "metadata | " + record.Path + " | tags=" + strconv.Itoa(len(record.Tags)),
			"confidence":   0.9,
			"source":       "parser.exiftool_json",
			"parser":       "exiftool_json",
			"parser_id":    "exiftool_json",
			"plan_node_id": request.Node.ID,
			"tool":         request.Node.ManifestName,
			"tool_id":      strings.TrimSpace(request.Node.ToolID),
			"operation":    strings.TrimSpace(request.Node.Operation),
			"execution_id": parserExecutionID(request, result),
			"path":         record.Path,
			"tags":         record.Tags,
		}),
	}
}

// rawStringValue renders a decoded JSON scalar as text (strings verbatim,
// numbers and bools formatted), used by the metadata and secret-scan parsers.
func rawStringValue(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return typed
	case bool:
		return strconv.FormatBool(typed)
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64)
	default:
		return fmt.Sprintf("%v", typed)
	}
}
