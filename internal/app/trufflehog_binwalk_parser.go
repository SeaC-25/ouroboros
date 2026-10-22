package app

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/ouroboros/ouroboros/internal/eventlog"
)

const maxArtifactScanParserBytes = 1 << 20

// trufflehogParserEvents converts TruffleHog JSONL output into secret facts.
func trufflehogParserEvents(runID string, request ToolExecutionRequest, result ToolExecutionResult) ([]eventlog.Event, error) {
	if len(result.Transcript) > maxArtifactScanParserBytes {
		return nil, fmt.Errorf("trufflehog parser output exceeds %d bytes", maxArtifactScanParserBytes)
	}
	output := strings.TrimSpace(result.Transcript)
	if output == "" {
		return nil, fmt.Errorf("trufflehog parser output is empty")
	}

	findings := []secretFinding{}
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var record map[string]any
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			return nil, fmt.Errorf("parse trufflehog JSONL: %w", err)
		}
		detector := firstNonEmptyRawString(record, "DetectorName", "detector_name")
		raw := firstNonEmptyRawString(record, "Raw", "raw")
		if detector == "" {
			continue
		}
		file, lineNumber := trufflehogSourceLocation(record)
		findings = append(findings, secretFinding{
			Rule:   detector,
			File:   file,
			Line:   lineNumber,
			Secret: raw,
			Match:  raw,
		})
	}
	if len(findings) == 0 {
		return nil, fmt.Errorf("trufflehog output contains no findings")
	}

	events := []eventlog.Event{parserStatusEvent(runID, request, result, "trufflehog_json", "parsed", "", len(findings))}
	for index, finding := range findings {
		events = append(events, secretFindingFactEvent(runID, request, result, finding, index))
	}
	return events, nil
}

// trufflehogSourceLocation digs the file/line out of SourceMetadata.Data.
func trufflehogSourceLocation(record map[string]any) (string, int) {
	metadata, ok := record["SourceMetadata"].(map[string]any)
	if !ok {
		return firstNonEmptyRawString(record, "File", "file"), 1
	}
	data, ok := metadata["Data"].(map[string]any)
	if !ok {
		return firstNonEmptyRawString(record, "File", "file"), 1
	}
	for _, key := range []string{"Filesystem", "filesystem", "Git", "git"} {
		entry, ok := data[key].(map[string]any)
		if !ok {
			continue
		}
		file := firstNonEmptyRawString(entry, "file", "File")
		line := rawIntValue(entry["line"], entry["Line"])
		if file != "" {
			if line <= 0 {
				line = 1
			}
			return file, line
		}
	}
	return firstNonEmptyRawString(record, "File", "file"), 1
}

// binwalkParserEvents converts binwalk-style signature reports into artifact
// facts. Entries look like "0x1E0 gzip compressed data" (hex or decimal).
func binwalkParserEvents(runID string, request ToolExecutionRequest, result ToolExecutionResult) ([]eventlog.Event, error) {
	if len(result.Transcript) > maxArtifactScanParserBytes {
		return nil, fmt.Errorf("binwalk parser output exceeds %d bytes", maxArtifactScanParserBytes)
	}
	output := strings.TrimSpace(result.Transcript)
	if output == "" {
		return nil, fmt.Errorf("binwalk parser output is empty")
	}

	findings := []firmwareSignature{}
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(strings.ToUpper(line), "DECIMAL") {
			continue
		}
		offset, label, ok := parseBinwalkLine(line)
		if !ok {
			continue
		}
		findings = append(findings, firmwareSignature{Offset: offset, Label: label})
	}
	if len(findings) == 0 {
		return nil, fmt.Errorf("binwalk output contains no signatures")
	}

	events := []eventlog.Event{parserStatusEvent(runID, request, result, "binwalk_text", "parsed", "", len(findings))}
	for index, finding := range findings {
		events = append(events, firmwareSignatureFactEvent(runID, request, result, finding, index))
	}
	return events, nil
}

// parseBinwalkLine splits "offset description" from a scan line.
func parseBinwalkLine(line string) (string, string, bool) {
	fields := strings.Fields(line)
	if len(fields) < 2 {
		return "", "", false
	}
	offset := fields[0]
	if !strings.HasPrefix(strings.ToLower(offset), "0x") {
		if _, err := strconv.Atoi(offset); err != nil {
			return "", "", false
		}
	}
	return offset, strings.Join(fields[1:], " "), true
}

type firmwareSignature struct {
	Offset string
	Label  string
}

func firmwareSignatureFactEvent(runID string, request ToolExecutionRequest, result ToolExecutionResult, record firmwareSignature, index int) eventlog.Event {
	label := record.Label
	return eventlog.Event{
		RunID: runID,
		Type:  "fact",
		Payload: mustJSON(map[string]any{
			"fact_id":      "fact-firmware-" + safeFactIDPart(request.Node.ID) + "-" + strconv.Itoa(index),
			"kind":         "artifact_content",
			"statement":    "firmware | offset " + record.Offset + " | " + label,
			"confidence":   0.8,
			"source":       "parser.binwalk_text",
			"parser":       "binwalk_text",
			"parser_id":    "binwalk_text",
			"plan_node_id": request.Node.ID,
			"tool":         request.Node.ManifestName,
			"tool_id":      strings.TrimSpace(request.Node.ToolID),
			"operation":    strings.TrimSpace(request.Node.Operation),
			"execution_id": parserExecutionID(request, result),
			"offset":       record.Offset,
			"description":  label,
		}),
	}
}
