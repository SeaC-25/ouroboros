package app

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/ouroboros/ouroboros/internal/eventlog"
)

const secretScanParserMaxBytes = 1 << 20

// gitleaksParserEvents converts gitleaks JSON output into secret finding
// facts. Previously the tool had a parser contract but no implementation, so
// leaked secrets never became evidence.
func gitleaksParserEvents(runID string, request ToolExecutionRequest, result ToolExecutionResult) ([]eventlog.Event, error) {
	if len(result.Transcript) > secretScanParserMaxBytes {
		return nil, fmt.Errorf("gitleaks parser output exceeds %d bytes", secretScanParserMaxBytes)
	}
	output := strings.TrimSpace(result.Transcript)
	if output == "" {
		return nil, fmt.Errorf("gitleaks parser output is empty")
	}

	var raw []map[string]any
	if err := json.Unmarshal([]byte(output), &raw); err != nil {
		return nil, fmt.Errorf("parse gitleaks JSON: %w", err)
	}

	findings := make([]secretFinding, 0, len(raw))
	for _, record := range raw {
		finding, ok := secretFindingFromRaw(record)
		if !ok {
			continue
		}
		findings = append(findings, finding)
	}
	sort.SliceStable(findings, func(i, j int) bool {
		if findings[i].File != findings[j].File {
			return findings[i].File < findings[j].File
		}
		return findings[i].Line < findings[j].Line
	})

	events := []eventlog.Event{parserStatusEvent(runID, request, result, "gitleaks_json", "parsed", "", len(findings))}
	for index, finding := range findings {
		events = append(events, secretFindingFactEvent(runID, request, result, finding, index))
	}
	return events, nil
}

type secretFinding struct {
	Rule   string
	File   string
	Line   int
	Secret string
	Match  string
}

func secretFindingFromRaw(record map[string]any) (secretFinding, bool) {
	rule := firstNonEmptyRawString(record, "RuleID", "rule_id", "rule")
	file := firstNonEmptyRawString(record, "File", "file")
	if rule == "" || file == "" {
		return secretFinding{}, false
	}
	finding := secretFinding{
		Rule:   rule,
		File:   file,
		Line:   rawIntValue(record["StartLine"], record["start_line"], record["line"]),
		Secret: firstNonEmptyRawString(record, "Secret", "secret", "Match"),
		Match:  firstNonEmptyRawString(record, "Match", "match", "line"),
	}
	if finding.Line <= 0 {
		finding.Line = 1
	}
	return finding, true
}

func firstNonEmptyRawString(record map[string]any, keys ...string) string {
	for _, key := range keys {
		if value := rawStringValue(record[key]); value != "" {
			return value
		}
	}
	return ""
}

// rawIntValue accepts the numeric shapes JSON decoders produce.
func rawIntValue(values ...any) int {
	for _, value := range values {
		switch typed := value.(type) {
		case float64:
			return int(typed)
		case int:
			return typed
		case json.Number:
			if parsed, err := typed.Int64(); err == nil {
				return int(parsed)
			}
		case string:
			parsed := 0
			if _, err := fmt.Sscanf(strings.TrimSpace(typed), "%d", &parsed); err == nil {
				return parsed
			}
		}
	}
	return 0
}

func secretFindingFactEvent(runID string, request ToolExecutionRequest, result ToolExecutionResult, finding secretFinding, index int) eventlog.Event {
	return eventlog.Event{
		RunID: runID,
		Type:  "fact",
		Payload: mustJSON(map[string]any{
			"fact_id":      "fact-secret-" + safeFactIDPart(request.Node.ID) + "-" + safeFactIDPart(finding.File) + "-" + fmt.Sprint(index),
			"kind":         "secret",
			"statement":    "secret | " + finding.Rule + " | " + finding.File + ":" + fmt.Sprint(finding.Line),
			"confidence":   0.85,
			"source":       "parser.gitleaks_json",
			"parser":       "gitleaks_json",
			"parser_id":    "gitleaks_json",
			"plan_node_id": request.Node.ID,
			"tool":         request.Node.ManifestName,
			"tool_id":      strings.TrimSpace(request.Node.ToolID),
			"operation":    strings.TrimSpace(request.Node.Operation),
			"execution_id": parserExecutionID(request, result),
			"rule_id":      finding.Rule,
			"file":         finding.File,
			"line":         finding.Line,
			"match":        finding.Match,
		}),
	}
}
