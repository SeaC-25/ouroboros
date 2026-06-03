package app

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/ouroboros/ouroboros/internal/eventlog"
)

const nucleiParserMaxOutputBytes = 1 << 20

var (
	nucleiCWEPattern = regexp.MustCompile(`(?i)^CWE-[0-9]+$`)
	nucleiCVEPattern = regexp.MustCompile(`(?i)^CVE-[0-9]{4}-[0-9A-Za-z]+$`)
)

func nucleiParserEvents(runID string, request ToolExecutionRequest, result ToolExecutionResult) ([]eventlog.Event, error) {
	output := strings.TrimSpace(result.Transcript)
	if output == "" {
		return nil, fmt.Errorf("nuclei parser output is empty")
	}
	if len(output) > nucleiParserMaxOutputBytes {
		return nil, fmt.Errorf("nuclei parser output exceeds %d bytes", nucleiParserMaxOutputBytes)
	}
	targetURL := nucleiTargetURL(request)
	if strings.TrimSpace(targetURL) == "" {
		return nil, fmt.Errorf("nuclei parser requires url scope target")
	}
	parsed, err := parseNucleiJSONL([]byte(output), targetURL)
	if err != nil {
		return nil, err
	}
	events := []eventlog.Event{parserStatusEvent(runID, request, result, "nuclei_jsonl", "parsed", "", len(parsed))}
	for index, finding := range parsed {
		events = append(events, nucleiFindingFactEvent(runID, request, result, finding, index))
	}
	return events, nil
}

type nucleiJSONLRecord struct {
	TemplateID  string     `json:"template-id"`
	Type        string     `json:"type"`
	MatchedAt   string     `json:"matched-at"`
	MatcherName string     `json:"matcher-name"`
	Info        nucleiInfo `json:"info"`
}

type nucleiInfo struct {
	Name           string               `json:"name"`
	Description    string               `json:"description"`
	Severity       string               `json:"severity"`
	Classification nucleiClassification `json:"classification"`
}

type nucleiClassification struct {
	CWE json.RawMessage `json:"cwe-id"`
	CVE json.RawMessage `json:"cve-id"`
}

type nucleiFinding struct {
	TemplateID  string
	Name        string
	Severity    string
	Description string
	Type        string
	MatcherName string
	MatchedURL  string
	TargetURL   string
	CWE         string
	CVE         string
	Confidence  float64
}

func parseNucleiJSONL(output []byte, targetURL string) ([]nucleiFinding, error) {
	scanner := bufio.NewScanner(bytes.NewReader(output))
	scanner.Buffer(make([]byte, 0, 64*1024), nucleiParserMaxOutputBytes)
	findings := []nucleiFinding{}
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var record nucleiJSONLRecord
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			return nil, fmt.Errorf("parse nuclei JSONL line %d: %w", lineNumber, err)
		}
		templateID := nucleiCleanMetadataValue(record.TemplateID, 200)
		name := nucleiCleanMetadataValue(record.Info.Name, 200)
		if templateID == "" && name == "" {
			continue
		}
		severity := nucleiSeverity(record.Info.Severity)
		matchedURL := nucleiScopedMatchedURL(record.MatchedAt, targetURL)
		findings = append(findings, nucleiFinding{
			TemplateID:  templateID,
			Name:        name,
			Severity:    severity,
			Description: nucleiCleanMetadataValue(record.Info.Description, 300),
			Type:        nucleiCleanMetadataValue(record.Type, 64),
			MatcherName: nucleiCleanMetadataValue(record.MatcherName, 120),
			MatchedURL:  matchedURL,
			TargetURL:   webEvidenceURL(targetURL),
			CWE:         firstNucleiClassificationID(record.Info.Classification.CWE, nucleiValidCWEID),
			CVE:         firstNucleiClassificationID(record.Info.Classification.CVE, nucleiValidCVEID),
			Confidence:  nucleiConfidence(severity),
		})
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan nuclei JSONL: %w", err)
	}
	return findings, nil
}

func nucleiFindingFactEvent(runID string, request ToolExecutionRequest, result ToolExecutionResult, finding nucleiFinding, index int) eventlog.Event {
	return eventlog.Event{
		RunID: runID,
		Type:  eventlog.EventFact,
		Payload: mustJSON(map[string]any{
			"fact_id":      nucleiFindingFactID(request, finding, index),
			"kind":         "finding",
			"statement":    nucleiFindingStatement(finding),
			"confidence":   finding.Confidence,
			"source":       "parser.nuclei_jsonl",
			"parser":       "nuclei_jsonl",
			"parser_id":    "nuclei_jsonl",
			"plan_node_id": request.Node.ID,
			"tool":         request.Node.ManifestName,
			"tool_id":      strings.TrimSpace(request.Node.ToolID),
			"operation":    strings.TrimSpace(request.Node.Operation),
			"execution_id": parserExecutionID(request, result),
			"template_id":  finding.TemplateID,
			"severity":     finding.Severity,
			"type":         finding.Type,
			"matcher_name": finding.MatcherName,
			"matched_url":  finding.MatchedURL,
			"target_url":   finding.TargetURL,
			"cwe":          finding.CWE,
			"cve":          finding.CVE,
		}),
	}
}

func nucleiFindingFactID(request ToolExecutionRequest, finding nucleiFinding, index int) string {
	parts := []string{"fact", "nuclei", safeFactIDPart(request.Node.ID), safeFactIDPart(finding.TemplateID), safeFactIDPart(finding.MatchedURL), strconv.Itoa(index)}
	return strings.Join(parts, "-")
}

func nucleiFindingStatement(finding nucleiFinding) string {
	parts := []string{"nuclei"}
	for _, value := range []string{finding.TemplateID, finding.Severity, finding.MatchedURL, finding.CWE, finding.CVE} {
		if strings.TrimSpace(value) != "" {
			parts = append(parts, strings.TrimSpace(value))
		}
	}
	return strings.Join(parts, " | ")
}

func nucleiTargetURL(request ToolExecutionRequest) string {
	if target, ok := scopedTargetByKind(request.ScopeTargets, "url"); ok {
		return strings.TrimSpace(target.Value)
	}
	return ""
}

func nucleiScopedMatchedURL(reported string, target string) string {
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

func webEvidenceURL(value string) string {
	parsed := parsedWebURL(value)
	if parsed == nil {
		return ""
	}
	path := parsed.EscapedPath()
	return strings.ToLower(parsed.Scheme) + "://" + canonicalWebURLHost(parsed) + path
}

func firstNucleiClassificationID(raw json.RawMessage, valid func(string) bool) string {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return ""
	}
	var values []string
	if err := json.Unmarshal(raw, &values); err == nil {
		for _, value := range values {
			value = strings.ToUpper(nucleiCleanMetadataValue(value, 64))
			if value != "" && valid(value) {
				return value
			}
		}
		return ""
	}
	var value string
	if err := json.Unmarshal(raw, &value); err == nil {
		value = strings.ToUpper(nucleiCleanMetadataValue(value, 64))
		if valid(value) {
			return value
		}
	}
	return ""
}

func nucleiValidCWEID(value string) bool {
	return nucleiCWEPattern.MatchString(strings.TrimSpace(value))
}

func nucleiValidCVEID(value string) bool {
	return nucleiCVEPattern.MatchString(strings.TrimSpace(value))
}

func nucleiSeverity(value string) string {
	switch strings.ToUpper(nucleiCleanMetadataValue(value, 32)) {
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

func nucleiCleanMetadataValue(value string, maxRunes int) string {
	value = strings.TrimSpace(value)
	value = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, value)
	if maxRunes <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) > maxRunes {
		return string(runes[:maxRunes])
	}
	return value
}

func nucleiConfidence(severity string) float64 {
	switch strings.ToUpper(strings.TrimSpace(severity)) {
	case "CRITICAL":
		return 0.95
	case "HIGH":
		return 0.9
	case "MEDIUM":
		return 0.75
	case "LOW":
		return 0.6
	case "INFO", "INFORMATIONAL":
		return 0.5
	default:
		return 0.55
	}
}
