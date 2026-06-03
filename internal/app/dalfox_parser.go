package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/ouroboros/ouroboros/internal/eventlog"
)

const dalfoxParserMaxOutputBytes = 1 << 20

func dalfoxParserEvents(runID string, request ToolExecutionRequest, result ToolExecutionResult) ([]eventlog.Event, error) {
	output := strings.TrimSpace(result.Transcript)
	if output == "" {
		return nil, fmt.Errorf("dalfox parser output is empty")
	}
	if len(output) > dalfoxParserMaxOutputBytes {
		return nil, fmt.Errorf("dalfox parser output exceeds %d bytes", dalfoxParserMaxOutputBytes)
	}
	targetURL := dalfoxTargetURL(request)
	if strings.TrimSpace(targetURL) == "" {
		return nil, fmt.Errorf("dalfox parser requires url scope target")
	}
	parsed, err := parseDalfoxJSON([]byte(output), targetURL, request.ScopeExclusions)
	if err != nil {
		return nil, err
	}
	events := []eventlog.Event{parserStatusEvent(runID, request, result, "dalfox_json", "parsed", "", len(parsed))}
	for index, finding := range parsed {
		events = append(events, dalfoxFindingFactEvent(runID, request, result, finding, index))
	}
	return events, nil
}

type dalfoxFinding struct {
	Type       string
	InjectType string
	PoCType    string
	Method     string
	Param      string
	Severity   string
	CWE        string
	MatchedURL string
	TargetURL  string
	Confidence float64
}

func parseDalfoxJSON(output []byte, targetURL string, exclusions []string) ([]dalfoxFinding, error) {
	targetURL = strings.TrimSpace(targetURL)
	if targetURL == "" {
		return nil, fmt.Errorf("dalfox parser requires url scope target")
	}
	if scopeTargetExcluded(ScopeTarget{Kind: "url", Value: targetURL}, exclusions) {
		return nil, nil
	}
	decoder := json.NewDecoder(bytes.NewReader(bytes.TrimSpace(output)))
	findings := []dalfoxFinding{}
	recordNumber := 0
	for {
		var raw json.RawMessage
		err := decoder.Decode(&raw)
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("parse dalfox JSON record %d: %w", recordNumber+1, err)
		}
		recordNumber++
		records, err := dalfoxPoCRecords(raw)
		if err != nil {
			return nil, fmt.Errorf("parse dalfox JSON record %d: %w", recordNumber, err)
		}
		for _, record := range records {
			finding := dalfoxFindingFromRecord(record, targetURL, exclusions)
			if finding.MatchedURL == "" {
				continue
			}
			findings = append(findings, finding)
		}
	}
	if recordNumber == 0 {
		return nil, fmt.Errorf("dalfox parser output is empty")
	}
	return findings, nil
}

func dalfoxPoCRecords(raw json.RawMessage) ([]map[string]json.RawMessage, error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return nil, nil
	}
	var records []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &records); err == nil {
		return records, nil
	}
	var record map[string]json.RawMessage
	if err := json.Unmarshal(raw, &record); err != nil {
		return nil, err
	}
	for _, name := range []string{"pocs", "PoCs", "pocs_json", "results"} {
		rawRecords, ok := record[name]
		if !ok {
			continue
		}
		if err := json.Unmarshal(rawRecords, &records); err != nil {
			return nil, fmt.Errorf("parse dalfox %s array: %w", name, err)
		}
		return records, nil
	}
	return []map[string]json.RawMessage{record}, nil
}

func dalfoxFindingFromRecord(record map[string]json.RawMessage, targetURL string, exclusions []string) dalfoxFinding {
	if !dalfoxMeaningfulRecord(record) {
		return dalfoxFinding{}
	}
	rawMatchedURL := dalfoxFindingURL(record)
	if rawMatchedURL != "" {
		if !webURLWithinScope(rawMatchedURL, targetURL) {
			return dalfoxFinding{}
		}
		if scopeTargetExcluded(ScopeTarget{Kind: "url", Value: rawMatchedURL}, exclusions) {
			return dalfoxFinding{}
		}
	}
	matchedURL := dalfoxScopedMatchedURL(rawMatchedURL, targetURL)
	if matchedURL == "" {
		return dalfoxFinding{}
	}
	severity := dalfoxSeverity(httpxStringField(record, "severity"))
	findingType := dalfoxCleanMetadataValue(httpxStringField(record, "type"), 32)
	return dalfoxFinding{
		Type:       findingType,
		InjectType: dalfoxCleanMetadataValue(httpxStringField(record, "inject_type", "inject-type", "injectType"), 80),
		PoCType:    dalfoxCleanMetadataValue(httpxStringField(record, "poc_type", "poc-type", "pocType"), 80),
		Method:     dalfoxHTTPMethod(httpxStringField(record, "method")),
		Param:      dalfoxCleanMetadataValue(httpxStringField(record, "param", "parameter"), 120),
		Severity:   severity,
		CWE:        dalfoxCWE(httpxStringField(record, "cwe")),
		MatchedURL: matchedURL,
		TargetURL:  webEvidenceURL(targetURL),
		Confidence: dalfoxConfidence(severity, findingType),
	}
}

func dalfoxFindingFactEvent(runID string, request ToolExecutionRequest, result ToolExecutionResult, finding dalfoxFinding, index int) eventlog.Event {
	return eventlog.Event{
		RunID: runID,
		Type:  eventlog.EventFact,
		Payload: mustJSON(map[string]any{
			"fact_id":      dalfoxFindingFactID(request, finding, index),
			"kind":         "finding",
			"statement":    dalfoxFindingStatement(finding),
			"confidence":   finding.Confidence,
			"source":       "parser.dalfox_json",
			"parser":       "dalfox_json",
			"parser_id":    "dalfox_json",
			"plan_node_id": request.Node.ID,
			"tool":         request.Node.ManifestName,
			"tool_id":      strings.TrimSpace(request.Node.ToolID),
			"operation":    strings.TrimSpace(request.Node.Operation),
			"execution_id": parserExecutionID(request, result),
			"type":         finding.Type,
			"inject_type":  finding.InjectType,
			"poc_type":     finding.PoCType,
			"method":       finding.Method,
			"param":        finding.Param,
			"severity":     finding.Severity,
			"cwe":          finding.CWE,
			"matched_url":  finding.MatchedURL,
			"target_url":   finding.TargetURL,
		}),
	}
}

func dalfoxFindingFactID(request ToolExecutionRequest, finding dalfoxFinding, index int) string {
	parts := []string{
		"fact",
		"dalfox",
		safeFactIDPart(request.Node.ID),
		safeFactIDPart(finding.Type),
		safeFactIDPart(finding.Param),
		safeFactIDPart(finding.MatchedURL),
		strconv.Itoa(index),
	}
	return strings.Join(parts, "-")
}

func dalfoxFindingStatement(finding dalfoxFinding) string {
	parts := []string{"dalfox"}
	for _, value := range []string{finding.Type, finding.Severity, finding.MatchedURL, finding.Param, finding.CWE} {
		if strings.TrimSpace(value) != "" {
			parts = append(parts, strings.TrimSpace(value))
		}
	}
	return strings.Join(parts, " | ")
}

func dalfoxTargetURL(request ToolExecutionRequest) string {
	if target, ok := scopedTargetByKind(request.ScopeTargets, "url"); ok {
		return strings.TrimSpace(target.Value)
	}
	return ""
}

func dalfoxMeaningfulRecord(record map[string]json.RawMessage) bool {
	if len(record) == 0 {
		return false
	}
	for _, name := range []string{"type", "inject_type", "inject-type", "injectType", "poc_type", "poc-type", "pocType", "method", "data", "url", "matched_url", "matched-url", "matchedURL", "param", "parameter", "severity", "cwe"} {
		if value := httpxStringField(record, name); value != "" {
			return true
		}
	}
	return false
}

func dalfoxFindingURL(record map[string]json.RawMessage) string {
	for _, value := range httpxStringFields(record, "data", "url", "matched_url", "matched-url", "matchedURL") {
		if parsedWebURL(value) != nil {
			return value
		}
	}
	return ""
}

func dalfoxScopedMatchedURL(reported string, target string) string {
	reported = strings.TrimSpace(reported)
	target = strings.TrimSpace(target)
	if target == "" {
		return ""
	}
	if reported != "" && webURLWithinScope(reported, target) {
		return webEvidenceURL(reported)
	}
	return webEvidenceURL(target)
}

func dalfoxSeverity(value string) string {
	switch strings.ToUpper(dalfoxCleanMetadataValue(value, 32)) {
	case "CRITICAL":
		return "CRITICAL"
	case "HIGH":
		return "HIGH"
	case "MEDIUM":
		return "MEDIUM"
	case "LOW":
		return "LOW"
	case "INFO", "INFORMATIONAL":
		return "INFO"
	default:
		return "UNKNOWN"
	}
}

func dalfoxCWE(value string) string {
	value = strings.ToUpper(dalfoxCleanMetadataValue(value, 64))
	if nucleiValidCWEID(value) {
		return value
	}
	return ""
}

func dalfoxHTTPMethod(value string) string {
	value = strings.ToUpper(dalfoxCleanMetadataValue(value, 16))
	switch value {
	case "GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS":
		return value
	default:
		return ""
	}
}

func dalfoxCleanMetadataValue(value string, maxRunes int) string {
	return nucleiCleanMetadataValue(value, maxRunes)
}

func dalfoxConfidence(severity string, findingType string) float64 {
	switch severity {
	case "CRITICAL", "HIGH":
		return 0.9
	case "MEDIUM":
		return 0.8
	case "LOW":
		return 0.65
	}
	if strings.EqualFold(strings.TrimSpace(findingType), "V") {
		return 0.85
	}
	return 0.55
}
