package app

import (
	"bufio"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/ouroboros/ouroboros/internal/eventlog"
)

const apkToolParserMaxOutputBytes = 1 << 20

// apktoolParserEvents converts apktool_text manifest summary lines into
// artifact facts. The native handler emits "apktool | key=value" lines
// (package/versionName/sdk/uses-permission/activity/service/receiver/
// exported/dex); the parser keeps that contract.
func apktoolParserEvents(runID string, request ToolExecutionRequest, result ToolExecutionResult) ([]eventlog.Event, error) {
	if len(result.Transcript) > apkToolParserMaxOutputBytes {
		return nil, fmt.Errorf("apktool parser output exceeds %d bytes", apkToolParserMaxOutputBytes)
	}
	output := strings.TrimSpace(result.Transcript)
	if output == "" {
		return nil, fmt.Errorf("apktool parser output is empty")
	}
	if strings.TrimSpace(request.Node.ToolID) != "apktool" || strings.TrimSpace(request.Node.Operation) != "apk_decompile" {
		return nil, fmt.Errorf("apktool parser requires apktool/apk_decompile tool execution")
	}

	scanner := bufio.NewScanner(strings.NewReader(output))
	scanner.Buffer(make([]byte, 0, 64*1024), apkToolParserMaxOutputBytes)
	entries := []map[string]string{}
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		fields := strings.SplitN(line, "|", 2)
		if len(fields) != 2 {
			continue
		}
		entry := map[string]string{}
		for _, pair := range strings.Fields(strings.TrimSpace(fields[1])) {
			key, value, ok := strings.Cut(pair, "=")
			if ok {
				entry[key] = value
			}
		}
		if len(entry) == 0 {
			// A bare word line such as "uses-permission android.permission.X".
			entry["kind"] = strings.Fields(strings.TrimSpace(fields[1]))[0]
			entry["value"] = strings.Join(strings.Fields(strings.TrimSpace(fields[1]))[1:], " ")
		}
		entries = append(entries, entry)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan apktool output: %w", err)
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("apktool output contains no summary entries")
	}

	events := []eventlog.Event{parserStatusEvent(runID, request, result, "apktool_text", "parsed", "", len(entries))}
	for index, entry := range entries {
		events = append(events, apktoolSummaryFactEvent(runID, request, result, entry, index))
	}
	return events, nil
}

// apktoolSummaryFactEvent emits one fact per summary line.
func apktoolSummaryFactEvent(runID string, request ToolExecutionRequest, result ToolExecutionResult, entry map[string]string, index int) eventlog.Event {
	keys := make([]string, 0, len(entry))
	for key := range entry {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, key+"="+entry[key])
	}
	statement := "apktool | " + strings.Join(parts, " ")
	payload := map[string]any{
		"kind":         "artifact",
		"statement":    statement,
		"confidence":   0.9,
		"source":       "parser.apktool_text",
		"parser":       "apktool_text",
		"parser_id":    "apktool_text",
		"plan_node_id": request.Node.ID,
		"tool":         request.Node.ManifestName,
		"tool_id":      strings.TrimSpace(request.Node.ToolID),
		"operation":    strings.TrimSpace(request.Node.Operation),
		"execution_id": parserExecutionID(request, result),
		"fact_id":      strings.Join([]string{"fact", "apktool", safeFactIDPart(request.Node.ID), strconv.Itoa(index)}, "-"),
	}
	for key, value := range entry {
		payload["apktool_"+key] = value
	}
	return eventlog.Event{
		RunID:   runID,
		Type:    eventlog.EventFact,
		Payload: mustJSON(payload),
	}
}
