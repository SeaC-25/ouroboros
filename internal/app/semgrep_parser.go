package app

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/ouroboros/ouroboros/internal/eventlog"
)

const semgrepParserMaxOutputBytes = 1 << 20

func semgrepParserEvents(runID string, request ToolExecutionRequest, result ToolExecutionResult) ([]eventlog.Event, error) {
	output := strings.TrimSpace(result.Transcript)
	if output == "" {
		return nil, fmt.Errorf("semgrep parser output is empty")
	}
	if len(output) > semgrepParserMaxOutputBytes {
		return nil, fmt.Errorf("semgrep parser output exceeds %d bytes", semgrepParserMaxOutputBytes)
	}
	if strings.TrimSpace(request.Node.ToolID) != "semgrep" || strings.TrimSpace(request.Node.Operation) != "static_code_scan" {
		return nil, fmt.Errorf("semgrep parser requires semgrep/static_code_scan tool execution")
	}
	parsed, err := parseSemgrepOutput([]byte(output))
	if err != nil {
		return nil, err
	}
	events := []eventlog.Event{parserStatusEvent(runID, request, result, "semgrep_sarif", "parsed", "", len(parsed))}
	for index, finding := range parsed {
		events = append(events, semgrepFindingFactEvent(runID, request, result, finding, index))
	}
	return events, nil
}

// sarifLog mirrors the SARIF 2.1.0 subset semgrep scan --sarif emits.
type sarifLog struct {
	Version string     `json:"version"`
	Runs    []sarifRun `json:"runs"`
}

type sarifRun struct {
	Tool    sarifTool     `json:"tool"`
	Results []sarifResult `json:"results"`
}

type sarifTool struct {
	Driver sarifDriver `json:"driver"`
}

type sarifDriver struct {
	Name  string      `json:"name"`
	Rules []sarifRule `json:"rules"`
}

type sarifRule struct {
	ID               string          `json:"id"`
	ShortDescription sarifMessage    `json:"shortDescription"`
	Properties       sarifProperties `json:"properties"`
}

type sarifProperties struct {
	Precision string `json:"precision"`
	CWE       string `json:"cwe,omitempty"`
}

type sarifMessage struct {
	Text string `json:"text"`
}

type sarifResult struct {
	RuleID    string          `json:"ruleId"`
	Level     string          `json:"level"`
	Message   sarifMessage    `json:"message"`
	Locations []sarifLocation `json:"locations"`
}

type sarifLocation struct {
	PhysicalLocation sarifPhysicalLocation `json:"physicalLocation"`
}

type sarifPhysicalLocation struct {
	ArtifactLocation sarifArtifactLocation `json:"artifactLocation"`
	Region           sarifRegion           `json:"region"`
}

type sarifArtifactLocation struct {
	URI string `json:"uri"`
}

type sarifRegion struct {
	StartLine int `json:"startLine"`
	EndLine   int `json:"endLine"`
}

type semgrepFinding struct {
	RuleID     string
	Level      string
	Message    string
	CWE        string
	FilePath   string
	LineStart  int
	LineEnd    int
	Confidence float64
}

func parseSemgrepOutput(output []byte) ([]semgrepFinding, error) {
	var decoded sarifLog
	if err := json.Unmarshal(output, &decoded); err != nil {
		return nil, fmt.Errorf("parse semgrep SARIF: %w", err)
	}
	findings := []semgrepFinding{}
	ruleDescriptions := map[string]string{}
	for _, run := range decoded.Runs {
		for _, rule := range run.Tool.Driver.Rules {
			if ruleDescriptions[rule.ID] == "" && strings.TrimSpace(rule.ShortDescription.Text) != "" {
				ruleDescriptions[rule.ID] = strings.TrimSpace(rule.ShortDescription.Text)
			}
		}
	}
	for _, run := range decoded.Runs {
		for _, result := range run.Results {
			ruleID := strings.TrimSpace(result.RuleID)
			if ruleID == "" {
				continue
			}
			uri, lineStart, lineEnd := sarifPrimaryLocation(result)
			message := strings.TrimSpace(result.Message.Text)
			if message == "" {
				message = ruleDescriptions[ruleID]
			}
			if message == "" {
				message = ruleID
			}
			level := strings.TrimSpace(result.Level)
			if level == "" {
				level = "warning"
			}
			findings = append(findings, semgrepFinding{
				RuleID:     ruleID,
				Level:      level,
				Message:    message,
				CWE:        sarifResultCWE(decoded, ruleID),
				FilePath:   uri,
				LineStart:  lineStart,
				LineEnd:    lineEnd,
				Confidence: semgrepConfidence(level),
			})
		}
	}
	return findings, nil
}

// sarifPrimaryLocation returns the first location's URI and line range.
func sarifPrimaryLocation(result sarifResult) (string, int, int) {
	if len(result.Locations) == 0 {
		return "", 0, 0
	}
	physical := result.Locations[0].PhysicalLocation
	uri := strings.TrimSpace(physical.ArtifactLocation.URI)
	lineStart := physical.Region.StartLine
	lineEnd := physical.Region.EndLine
	if lineEnd < lineStart {
		lineEnd = lineStart
	}
	return uri, lineStart, lineEnd
}

// sarifResultCWE looks up the rule's CWE property, mirroring how the mobsfscan
// parser surfaces the rule metadata on each fact.
func sarifResultCWE(log sarifLog, ruleID string) string {
	for _, run := range log.Runs {
		for _, rule := range run.Tool.Driver.Rules {
			if rule.ID != ruleID {
				continue
			}
			if cwe := strings.TrimSpace(rule.Properties.CWE); cwe != "" {
				return cwe
			}
		}
	}
	return ""
}

func semgrepConfidence(level string) float64 {
	switch strings.ToUpper(strings.TrimSpace(level)) {
	case "ERROR":
		return 0.9
	case "WARNING":
		return 0.75
	case "NOTE", "INFO":
		return 0.5
	default:
		return 0.55
	}
}

func semgrepFindingFactEvent(runID string, request ToolExecutionRequest, result ToolExecutionResult, finding semgrepFinding, index int) eventlog.Event {
	return eventlog.Event{
		RunID: runID,
		Type:  eventlog.EventFact,
		Payload: mustJSON(map[string]any{
			"fact_id":      semgrepFindingFactID(request, finding, index),
			"kind":         "finding",
			"statement":    finding.Message,
			"confidence":   finding.Confidence,
			"source":       "parser.semgrep_sarif",
			"parser":       "semgrep_sarif",
			"parser_id":    "semgrep_sarif",
			"plan_node_id": request.Node.ID,
			"tool":         request.Node.ManifestName,
			"tool_id":      strings.TrimSpace(request.Node.ToolID),
			"operation":    strings.TrimSpace(request.Node.Operation),
			"execution_id": parserExecutionID(request, result),
			"rule_id":      finding.RuleID,
			"severity":     finding.Level,
			"cwe":          finding.CWE,
			"file_path":    finding.FilePath,
			"line_start":   finding.LineStart,
			"line_end":     finding.LineEnd,
		}),
	}
}

func semgrepFindingFactID(request ToolExecutionRequest, finding semgrepFinding, index int) string {
	parts := []string{"fact", "semgrep", safeFactIDPart(request.Node.ID), safeFactIDPart(finding.RuleID), strconv.Itoa(index)}
	return strings.Join(parts, "-")
}
