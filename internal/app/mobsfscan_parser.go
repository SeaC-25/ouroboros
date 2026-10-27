package app

import (
	"encoding/json"
	"fmt"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/ouroboros/ouroboros/internal/eventlog"
)

const mobsfscanParserMaxOutputBytes = 1 << 20

var unsafeFactIDChars = regexp.MustCompile(`[^A-Za-z0-9_.-]+`)

func parserEventsForToolExecution(runID string, request ToolExecutionRequest, result ToolExecutionResult) []eventlog.Event {
	status := strings.TrimSpace(result.Status)
	if status != "" && status != "completed" {
		return nil
	}
	parserID := parserIDForRequest(request)
	switch parserID {
	case "mobsfscan_json":
		events, err := mobsfscanParserEvents(runID, request, result)
		if err != nil {
			return []eventlog.Event{parserStatusEvent(runID, request, result, parserID, "parse_failed", err.Error(), 0)}
		}
		return events
	case "semgrep_sarif":
		events, err := semgrepParserEvents(runID, request, result)
		if err != nil {
			return []eventlog.Event{parserStatusEvent(runID, request, result, parserID, "parse_failed", err.Error(), 0)}
		}
		return events
	case "nuclei_jsonl":
		events, err := nucleiParserEvents(runID, request, result)
		if err != nil {
			return []eventlog.Event{parserStatusEvent(runID, request, result, parserID, "parse_failed", err.Error(), 0)}
		}
		return events
	case "dalfox_json":
		events, err := dalfoxParserEvents(runID, request, result)
		if err != nil {
			return []eventlog.Event{parserStatusEvent(runID, request, result, parserID, "parse_failed", err.Error(), 0)}
		}
		return events
	case "arjun_json":
		events, err := arjunParserEvents(runID, request, result)
		if err != nil {
			return []eventlog.Event{parserStatusEvent(runID, request, result, parserID, "parse_failed", err.Error(), 0)}
		}
		return events
	case "sqlmap_json":
		events, err := sqlmapParserEvents(runID, request, result)
		if err != nil {
			return []eventlog.Event{parserStatusEvent(runID, request, result, parserID, "parse_failed", err.Error(), 0)}
		}
		return events
	case "httpx_jsonl":
		events, err := httpxParserEvents(runID, request, result)
		if err != nil {
			return []eventlog.Event{parserStatusEvent(runID, request, result, parserID, "parse_failed", err.Error(), 0)}
		}
		return events
	case "nmap_xml":
		events, err := nmapParserEvents(runID, request, result)
		if err != nil {
			return []eventlog.Event{parserStatusEvent(runID, request, result, parserID, "parse_failed", err.Error(), 0)}
		}
		return events
	case "subfinder_jsonl":
		events, err := subfinderParserEvents(runID, request, result)
		if err != nil {
			return []eventlog.Event{parserStatusEvent(runID, request, result, parserID, "parse_failed", err.Error(), 0)}
		}
		return events
	case "naabu_jsonl":
		events, err := naabuParserEvents(runID, request, result)
		if err != nil {
			return []eventlog.Event{parserStatusEvent(runID, request, result, parserID, "parse_failed", err.Error(), 0)}
		}
		return events
	case "dnsx_jsonl":
		events, err := dnsxParserEvents(runID, request, result)
		if err != nil {
			return []eventlog.Event{parserStatusEvent(runID, request, result, parserID, "parse_failed", err.Error(), 0)}
		}
		return events
	case "katana_jsonl":
		events, err := katanaParserEvents(runID, request, result)
		if err != nil {
			return []eventlog.Event{parserStatusEvent(runID, request, result, parserID, "parse_failed", err.Error(), 0)}
		}
		return events
	case "ffuf_json":
		events, err := ffufParserEvents(runID, request, result)
		if err != nil {
			return []eventlog.Event{parserStatusEvent(runID, request, result, parserID, "parse_failed", err.Error(), 0)}
		}
		return events
	case "url_list_text":
		events, err := urlListParserEvents(runID, request, result)
		if err != nil {
			return []eventlog.Event{parserStatusEvent(runID, request, result, parserID, "parse_failed", err.Error(), 0)}
		}
		return events
	case "amass_json":
		events, err := amassParserEvents(runID, request, result)
		if err != nil {
			return []eventlog.Event{parserStatusEvent(runID, request, result, parserID, "parse_failed", err.Error(), 0)}
		}
		return events
	case "dnsrecon_json":
		events, err := dnsreconParserEvents(runID, request, result)
		if err != nil {
			return []eventlog.Event{parserStatusEvent(runID, request, result, parserID, "parse_failed", err.Error(), 0)}
		}
		return events
	case "exiftool_json":
		events, err := exiftoolParserEvents(runID, request, result)
		if err != nil {
			return []eventlog.Event{parserStatusEvent(runID, request, result, parserID, "parse_failed", err.Error(), 0)}
		}
		return events
	case "gitleaks_json":
		events, err := gitleaksParserEvents(runID, request, result)
		if err != nil {
			return []eventlog.Event{parserStatusEvent(runID, request, result, parserID, "parse_failed", err.Error(), 0)}
		}
		return events
	case "testssl_json":
		events, err := testsslParserEvents(runID, request, result)
		if err != nil {
			return []eventlog.Event{parserStatusEvent(runID, request, result, parserID, "parse_failed", err.Error(), 0)}
		}
		return events
	case "sslscan_xml":
		events, err := sslscanParserEvents(runID, request, result)
		if err != nil {
			return []eventlog.Event{parserStatusEvent(runID, request, result, parserID, "parse_failed", err.Error(), 0)}
		}
		return events
	case "trufflehog_json":
		events, err := trufflehogParserEvents(runID, request, result)
		if err != nil {
			return []eventlog.Event{parserStatusEvent(runID, request, result, parserID, "parse_failed", err.Error(), 0)}
		}
		return events
	case "apktool_text":
		events, err := apktoolParserEvents(runID, request, result)
		if err != nil {
			return []eventlog.Event{parserStatusEvent(runID, request, result, parserID, "parse_failed", err.Error(), 0)}
		}
		return events
	case "jadx_metadata_json":
		events, err := jadxParserEvents(runID, request, result)
		if err != nil {
			return []eventlog.Event{parserStatusEvent(runID, request, result, parserID, "parse_failed", err.Error(), 0)}
		}
		return events
	case "binwalk_text":
		events, err := binwalkParserEvents(runID, request, result)
		if err != nil {
			return []eventlog.Event{parserStatusEvent(runID, request, result, parserID, "parse_failed", err.Error(), 0)}
		}
		return events
	case "auth_test_jsonl":
		events, err := authTestParserEvents(runID, request, result)
		if err != nil {
			return []eventlog.Event{parserStatusEvent(runID, request, result, parserID, "parse_failed", err.Error(), 0)}
		}
		return events
	case "observe_json":
		events, err := observeParserEvents(runID, request, result)
		if err != nil {
			return []eventlog.Event{parserStatusEvent(runID, request, result, parserID, "parse_failed", err.Error(), 0)}
		}
		return events
	default:
		return nil
	}
}

func mobsfscanParserEvents(runID string, request ToolExecutionRequest, result ToolExecutionResult) ([]eventlog.Event, error) {
	output := strings.TrimSpace(result.Transcript)
	if output == "" {
		return nil, fmt.Errorf("mobsfscan parser output is empty")
	}
	if len(output) > mobsfscanParserMaxOutputBytes {
		return nil, fmt.Errorf("mobsfscan parser output exceeds %d bytes", mobsfscanParserMaxOutputBytes)
	}
	parsed, err := parseMobsfscanOutput([]byte(output), mobsfscanArtifactTargetIdentifier(request))
	if err != nil {
		return nil, err
	}
	events := []eventlog.Event{parserStatusEvent(runID, request, result, "mobsfscan_json", "parsed", "", len(parsed))}
	for index, finding := range parsed {
		events = append(events, mobsfscanFindingFactEvent(runID, request, result, finding, index))
	}
	return events, nil
}

type mobsfscanOutput struct {
	Results map[string]mobsfscanResult `json:"results"`
}

type mobsfscanResult struct {
	Metadata mobsfscanMetadata `json:"metadata"`
	Files    []mobsfscanFile   `json:"files"`
}

type mobsfscanMetadata struct {
	Description string `json:"description"`
	Severity    string `json:"severity"`
	CWE         string `json:"cwe"`
	OWASP       string `json:"owasp-mobile"`
	MASVS       string `json:"masvs"`
}

type mobsfscanFile struct {
	FilePath    string `json:"file_path"`
	MatchLines  []int  `json:"match_lines"`
	LineNumber  int    `json:"line_number"`
	StartLine   int    `json:"start_line"`
	MatchString string `json:"match_string"`
}

type mobsfscanFinding struct {
	RuleID      string
	Severity    string
	Description string
	CWE         string
	OWASP       string
	MASVS       string
	Location    string
	FilePath    string
	Artifact    string
	LineStart   int
	LineEnd     int
	Confidence  float64
}

func parseMobsfscanOutput(output []byte, artifactTarget string) ([]mobsfscanFinding, error) {
	var decoded mobsfscanOutput
	if err := json.Unmarshal(output, &decoded); err != nil {
		return nil, fmt.Errorf("parse mobsfscan JSON: %w", err)
	}
	if len(decoded.Results) == 0 {
		return nil, nil
	}
	ruleIDs := make([]string, 0, len(decoded.Results))
	for ruleID := range decoded.Results {
		ruleIDs = append(ruleIDs, ruleID)
	}
	sort.Strings(ruleIDs)
	findings := make([]mobsfscanFinding, 0, len(ruleIDs))
	for _, ruleID := range ruleIDs {
		result := decoded.Results[ruleID]
		metadata := result.Metadata
		statement := strings.TrimSpace(metadata.Description)
		if statement == "" {
			statement = strings.TrimSpace(ruleID)
		}
		filePath, lineStart, lineEnd := mobsfscanPrimaryFile(result.Files, artifactTarget)
		findings = append(findings, mobsfscanFinding{
			RuleID:      strings.TrimSpace(ruleID),
			Severity:    strings.ToUpper(strings.TrimSpace(metadata.Severity)),
			Description: statement,
			CWE:         strings.TrimSpace(metadata.CWE),
			OWASP:       strings.TrimSpace(metadata.OWASP),
			MASVS:       strings.TrimSpace(metadata.MASVS),
			Location:    mobsfscanLocation(filePath, lineStart),
			FilePath:    filePath,
			Artifact:    artifactTarget,
			LineStart:   lineStart,
			LineEnd:     lineEnd,
			Confidence:  mobsfscanConfidence(metadata.Severity),
		})
	}
	return findings, nil
}

func mobsfscanFindingFactEvent(runID string, request ToolExecutionRequest, result ToolExecutionResult, finding mobsfscanFinding, index int) eventlog.Event {
	return eventlog.Event{
		RunID: runID,
		Type:  eventlog.EventFact,
		Payload: mustJSON(map[string]any{
			"fact_id":         mobsfscanFindingFactID(request, finding, index),
			"kind":            "finding",
			"statement":       mobsfscanFindingStatement(finding),
			"confidence":      finding.Confidence,
			"source":          "parser.mobsfscan_json",
			"parser":          "mobsfscan_json",
			"parser_id":       "mobsfscan_json",
			"plan_node_id":    request.Node.ID,
			"tool":            request.Node.ManifestName,
			"tool_id":         strings.TrimSpace(request.Node.ToolID),
			"operation":       strings.TrimSpace(request.Node.Operation),
			"execution_id":    parserExecutionID(request, result),
			"rule_id":         finding.RuleID,
			"severity":        finding.Severity,
			"cwe":             finding.CWE,
			"owasp_mobile":    finding.OWASP,
			"masvs":           finding.MASVS,
			"file_path":       finding.FilePath,
			"artifact_target": finding.Artifact,
			"line_start":      finding.LineStart,
			"line_end":        finding.LineEnd,
		}),
	}
}

func parserStatusEvent(runID string, request ToolExecutionRequest, result ToolExecutionResult, parserID string, status string, message string, findingCount int) eventlog.Event {
	parserID = strings.TrimSpace(parserID)
	payload := map[string]any{
		"parser":        parserID,
		"parser_id":     parserID,
		"status":        strings.TrimSpace(status),
		"plan_node_id":  request.Node.ID,
		"tool":          request.Node.ManifestName,
		"tool_id":       strings.TrimSpace(request.Node.ToolID),
		"operation":     strings.TrimSpace(request.Node.Operation),
		"capability":    strings.TrimSpace(request.Node.Capability),
		"execution_id":  parserExecutionID(request, result),
		"finding_count": findingCount,
	}
	if strings.TrimSpace(message) != "" {
		payload["message"] = strings.TrimSpace(message)
	}
	return eventlog.Event{
		RunID:   runID,
		Type:    eventlog.EventParser,
		Payload: mustJSON(payload),
	}
}

func parserIDForRequest(request ToolExecutionRequest) string {
	if parserID := strings.TrimSpace(request.Node.ParserID); parserID != "" {
		return parserID
	}
	return strings.TrimSpace(request.Node.Parser)
}

func parserExecutionID(request ToolExecutionRequest, result ToolExecutionResult) string {
	if executionID := strings.TrimSpace(result.ExecutionID); executionID != "" {
		return executionID
	}
	return "tool-" + strings.ReplaceAll(request.Node.ID, ":", "-")
}

func mobsfscanFindingFactID(request ToolExecutionRequest, finding mobsfscanFinding, index int) string {
	parts := []string{"fact", "mobsfscan", safeFactIDPart(request.Node.ID), safeFactIDPart(finding.RuleID), strconv.Itoa(index)}
	return strings.Join(parts, "-")
}

func safeFactIDPart(value string) string {
	value = unsafeFactIDChars.ReplaceAllString(strings.TrimSpace(value), "-")
	value = strings.Trim(value, "-")
	if value == "" {
		return "unknown"
	}
	return value
}

func mobsfscanFindingStatement(finding mobsfscanFinding) string {
	parts := []string{"mobsfscan"}
	for _, value := range []string{finding.RuleID, finding.Severity, finding.Location, finding.CWE, finding.OWASP, finding.MASVS, finding.Description} {
		if strings.TrimSpace(value) != "" {
			parts = append(parts, strings.TrimSpace(value))
		}
	}
	return strings.Join(parts, " | ")
}

func mobsfscanPrimaryFile(files []mobsfscanFile, artifactTarget string) (string, int, int) {
	if len(files) == 0 {
		return artifactTarget, 0, 0
	}
	file := files[0]
	lineStart, lineEnd := mobsfscanLineRange(file)
	return mobsfscanScopedReportedPath(file.FilePath, artifactTarget), lineStart, lineEnd
}

func mobsfscanLocation(filePath string, lineStart int) string {
	if lineStart > 0 {
		if strings.TrimSpace(filePath) == "" {
			return strconv.Itoa(lineStart)
		}
		return strings.TrimSpace(filePath) + ":" + strconv.Itoa(lineStart)
	}
	return strings.TrimSpace(filePath)
}

func mobsfscanLineRange(file mobsfscanFile) (int, int) {
	if len(file.MatchLines) > 0 && file.MatchLines[0] > 0 {
		lineEnd := file.MatchLines[0]
		if len(file.MatchLines) > 1 && file.MatchLines[1] > 0 {
			lineEnd = file.MatchLines[1]
		}
		return file.MatchLines[0], lineEnd
	}
	if file.LineNumber > 0 {
		return file.LineNumber, file.LineNumber
	}
	if file.StartLine > 0 {
		return file.StartLine, file.StartLine
	}
	return 0, 0
}

func mobsfscanConfidence(severity string) float64 {
	switch strings.ToUpper(strings.TrimSpace(severity)) {
	case "CRITICAL":
		return 0.95
	case "HIGH", "ERROR":
		return 0.9
	case "MEDIUM", "WARNING":
		return 0.75
	case "LOW":
		return 0.6
	case "INFO", "INFORMATIONAL":
		return 0.5
	default:
		return 0.55
	}
}

func mobsfscanArtifactTargetIdentifier(request ToolExecutionRequest) string {
	target, ok := scopedTargetByKind(request.ScopeTargets, "artifact")
	if !ok {
		return ""
	}
	targetValue := strings.TrimSpace(target.Value)
	if targetValue == "" {
		return ""
	}
	if strings.TrimSpace(request.WorkspaceRoot) != "" {
		if resolvedTarget, err := workspaceArtifactPath(request.WorkspaceRoot, targetValue); err == nil {
			if artifactRoot, err := filepath.EvalSymlinks(filepath.Join(request.WorkspaceRoot, "artifacts")); err == nil {
				if relative, err := filepath.Rel(artifactRoot, resolvedTarget); err == nil {
					if clean, ok := mobsfscanCleanRelativeArtifactPath(relative); ok {
						return clean
					}
				}
			}
		}
	}
	if clean, ok := mobsfscanCleanRelativeArtifactPath(targetValue); ok {
		return clean
	}
	return "artifact"
}

func mobsfscanScopedReportedPath(reportedPath string, artifactTarget string) string {
	artifactTarget = strings.TrimSpace(artifactTarget)
	reportedPath, ok := mobsfscanCleanRelativeArtifactPath(reportedPath)
	if !ok {
		return artifactTarget
	}
	if artifactTarget == "" || reportedPath == artifactTarget {
		return reportedPath
	}
	return artifactTarget + "#" + reportedPath
}

func mobsfscanCleanRelativeArtifactPath(value string) (string, bool) {
	value = strings.TrimSpace(strings.ReplaceAll(value, "\\", "/"))
	if value == "" || strings.ContainsRune(value, 0) || strings.HasPrefix(value, "/") || strings.Contains(value, ":") {
		return "", false
	}
	for _, segment := range strings.Split(value, "/") {
		if segment == ".." {
			return "", false
		}
	}
	clean := path.Clean(value)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") || path.IsAbs(clean) {
		return "", false
	}
	return clean, true
}
