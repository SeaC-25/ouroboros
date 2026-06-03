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

const sqlmapParserMaxOutputBytes = 1 << 20

func sqlmapParserEvents(runID string, request ToolExecutionRequest, result ToolExecutionResult) ([]eventlog.Event, error) {
	output := strings.TrimSpace(result.Transcript)
	if output == "" {
		return nil, fmt.Errorf("sqlmap parser output is empty")
	}
	if len(output) > sqlmapParserMaxOutputBytes {
		return nil, fmt.Errorf("sqlmap parser output exceeds %d bytes", sqlmapParserMaxOutputBytes)
	}
	targetURL := sqlmapTargetURL(request)
	if strings.TrimSpace(targetURL) == "" {
		return nil, fmt.Errorf("sqlmap parser requires url scope target")
	}
	parsed, err := parseSqlmapJSON([]byte(output), targetURL, request.ScopeExclusions)
	if err != nil {
		return nil, err
	}
	events := []eventlog.Event{parserStatusEvent(runID, request, result, "sqlmap_json", "parsed", "", len(parsed))}
	for index, finding := range parsed {
		events = append(events, sqlmapFindingFactEvent(runID, request, result, finding, index))
	}
	return events, nil
}

type sqlmapFinding struct {
	MatchedURL string
	TargetURL  string
	Parameter  string
	Place      string
	Technique  string
	DBMS       string
	Title      string
	Confidence float64
}

func parseSqlmapJSON(output []byte, targetURL string, exclusions []string) ([]sqlmapFinding, error) {
	targetURL = strings.TrimSpace(targetURL)
	if targetURL == "" {
		return nil, fmt.Errorf("sqlmap parser requires url scope target")
	}
	if scopeTargetExcluded(ScopeTarget{Kind: "url", Value: targetURL}, exclusions) {
		return nil, nil
	}
	decoder := json.NewDecoder(bytes.NewReader(bytes.TrimSpace(output)))
	findings := []sqlmapFinding{}
	recordNumber := 0
	seen := map[string]struct{}{}
	for {
		var raw json.RawMessage
		err := decoder.Decode(&raw)
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("parse sqlmap JSON record %d: %w", recordNumber+1, err)
		}
		recordNumber++
		records, err := sqlmapFindingRecords(raw)
		if err != nil {
			return nil, fmt.Errorf("parse sqlmap JSON record %d: %w", recordNumber, err)
		}
		for _, record := range records {
			finding := sqlmapFindingFromRecord(record, targetURL, exclusions)
			if finding.MatchedURL == "" {
				continue
			}
			key := strings.Join([]string{finding.MatchedURL, finding.Parameter, finding.Place}, "\x00")
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			findings = append(findings, finding)
		}
	}
	if recordNumber == 0 {
		return nil, fmt.Errorf("sqlmap parser output is empty")
	}
	return findings, nil
}

func sqlmapFindingRecords(raw json.RawMessage) ([]map[string]json.RawMessage, error) {
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
	for _, name := range []string{"findings", "results"} {
		rawRecords, ok := record[name]
		if !ok {
			continue
		}
		if err := json.Unmarshal(rawRecords, &records); err != nil {
			return nil, fmt.Errorf("parse sqlmap %s array: %w", name, err)
		}
		return records, nil
	}
	return []map[string]json.RawMessage{record}, nil
}

func sqlmapFindingFromRecord(record map[string]json.RawMessage, targetURL string, exclusions []string) sqlmapFinding {
	parameter := arjunCleanParameterName(httpxStringField(record, "parameter", "param"))
	technique := sqlmapTechnique(record)
	title := sqlmapCleanMetadataValue(httpxStringField(record, "title", "type"), 120)
	dbms := sqlmapCleanMetadataValue(httpxStringField(record, "dbms"), 80)
	if !sqlmapMeaningfulRecord(record, parameter, technique, title, dbms) {
		return sqlmapFinding{}
	}
	rawURL := sqlmapFindingURL(record)
	if rawURL != "" {
		if !webURLWithinScope(rawURL, targetURL) {
			return sqlmapFinding{}
		}
		if scopeTargetExcluded(ScopeTarget{Kind: "url", Value: rawURL}, exclusions) {
			return sqlmapFinding{}
		}
	}
	matchedURL := sqlmapScopedMatchedURL(rawURL, targetURL)
	if matchedURL == "" {
		return sqlmapFinding{}
	}
	return sqlmapFinding{
		MatchedURL: matchedURL,
		TargetURL:  webEvidenceURL(targetURL),
		Parameter:  parameter,
		Place:      sqlmapPlace(httpxStringField(record, "place", "parameter_place")),
		Technique:  technique,
		DBMS:       dbms,
		Title:      title,
		Confidence: sqlmapConfidence(record, technique, title, dbms),
	}
}

func sqlmapMeaningfulRecord(record map[string]json.RawMessage, parameter string, technique string, title string, dbms string) bool {
	if sqlmapBoolField(record, "vulnerable", "injectable") {
		return parameter != "" || technique != "" || title != "" || dbms != ""
	}
	if technique != "" {
		return parameter != ""
	}
	return parameter != "" && (title != "" || dbms != "")
}

func sqlmapFindingURL(record map[string]json.RawMessage) string {
	for _, value := range httpxStringFields(record, "url", "target_url", "target", "endpoint", "matched_url", "matched-url", "matchedURL") {
		if parsedWebURL(value) != nil {
			return value
		}
	}
	return ""
}

func sqlmapScopedMatchedURL(reported string, target string) string {
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

func sqlmapTechnique(record map[string]json.RawMessage) string {
	if value := sqlmapCleanMetadataValue(httpxStringField(record, "technique"), 120); value != "" {
		return value
	}
	raw, ok := record["techniques"]
	if !ok {
		return ""
	}
	var values []string
	if err := json.Unmarshal(raw, &values); err == nil {
		clean := []string{}
		for _, value := range values {
			value = sqlmapCleanMetadataValue(value, 120)
			if value != "" {
				clean = append(clean, value)
			}
		}
		return strings.Join(clean, ",")
	}
	return ""
}

func sqlmapPlace(value string) string {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case "GET", "POST", "COOKIE", "HEADER", "URI":
		return strings.ToUpper(strings.TrimSpace(value))
	default:
		return sqlmapCleanMetadataValue(value, 32)
	}
}

func sqlmapCleanMetadataValue(value string, maxLen int) string {
	value = nucleiCleanMetadataValue(value, maxLen)
	for _, blocked := range []string{"authorization", "bearer", "cookie", "secret-token"} {
		if strings.Contains(strings.ToLower(value), blocked) {
			return ""
		}
	}
	return value
}

func sqlmapBoolField(record map[string]json.RawMessage, names ...string) bool {
	for _, name := range names {
		raw, ok := record[name]
		if !ok {
			continue
		}
		var value bool
		if err := json.Unmarshal(raw, &value); err == nil {
			return value
		}
	}
	return false
}

func sqlmapConfidence(record map[string]json.RawMessage, technique string, title string, dbms string) float64 {
	if sqlmapBoolField(record, "vulnerable", "injectable") || technique != "" {
		return 0.85
	}
	if title != "" || dbms != "" {
		return 0.75
	}
	return 0.6
}

func sqlmapFindingFactEvent(runID string, request ToolExecutionRequest, result ToolExecutionResult, finding sqlmapFinding, index int) eventlog.Event {
	return eventlog.Event{
		RunID: runID,
		Type:  eventlog.EventFact,
		Payload: mustJSON(map[string]any{
			"fact_id":      sqlmapFindingFactID(request, finding, index),
			"kind":         "finding",
			"statement":    sqlmapFindingStatement(finding),
			"confidence":   finding.Confidence,
			"source":       "parser.sqlmap_json",
			"parser":       "sqlmap_json",
			"parser_id":    "sqlmap_json",
			"plan_node_id": request.Node.ID,
			"tool":         request.Node.ManifestName,
			"tool_id":      strings.TrimSpace(request.Node.ToolID),
			"operation":    strings.TrimSpace(request.Node.Operation),
			"execution_id": parserExecutionID(request, result),
			"matched_url":  finding.MatchedURL,
			"target_url":   finding.TargetURL,
			"parameter":    finding.Parameter,
			"place":        finding.Place,
			"technique":    finding.Technique,
			"dbms":         finding.DBMS,
			"title":        finding.Title,
		}),
	}
}

func sqlmapFindingFactID(request ToolExecutionRequest, finding sqlmapFinding, index int) string {
	parts := []string{
		"fact",
		"sqlmap",
		safeFactIDPart(request.Node.ID),
		safeFactIDPart(finding.MatchedURL),
		safeFactIDPart(finding.Parameter),
		strconv.Itoa(index),
	}
	return strings.Join(parts, "-")
}

func sqlmapFindingStatement(finding sqlmapFinding) string {
	parts := []string{"sqlmap", finding.MatchedURL}
	if finding.Parameter != "" {
		parts = append(parts, "parameter="+finding.Parameter)
	}
	for _, value := range []string{finding.Technique, finding.DBMS, finding.Title} {
		if strings.TrimSpace(value) != "" {
			parts = append(parts, strings.TrimSpace(value))
		}
	}
	return strings.Join(parts, " | ")
}

func sqlmapTargetURL(request ToolExecutionRequest) string {
	if target, ok := scopedTargetByKind(request.ScopeTargets, "url"); ok {
		return strings.TrimSpace(target.Value)
	}
	return ""
}
