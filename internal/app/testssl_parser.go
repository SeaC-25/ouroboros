package app

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/ouroboros/ouroboros/internal/eventlog"
)

// maxTLSParserBytes bounds TLS assessment output, matching the other parsers.
const maxTLSParserBytes = 1 << 20

// testsslParserEvents converts testssl.sh JSON findings into TLS assessment
// facts. The tool previously had a parser contract but no implementation.
func testsslParserEvents(runID string, request ToolExecutionRequest, result ToolExecutionResult) ([]eventlog.Event, error) {
	if len(result.Transcript) > maxTLSParserBytes {
		return nil, fmt.Errorf("testssl parser output exceeds %d bytes", maxTLSParserBytes)
	}
	output := strings.TrimSpace(result.Transcript)
	if output == "" {
		return nil, fmt.Errorf("testssl parser output is empty")
	}

	var records []map[string]any
	if err := json.Unmarshal([]byte(output), &records); err != nil {
		return nil, fmt.Errorf("parse testssl JSON: %w", err)
	}

	findings := make([]tlsFinding, 0, len(records))
	for _, record := range records {
		finding, ok := tlsFindingFromRaw(record)
		if !ok {
			continue
		}
		findings = append(findings, finding)
	}
	if len(findings) == 0 {
		return nil, fmt.Errorf("testssl output contains no findings")
	}
	sort.SliceStable(findings, func(i, j int) bool {
		if findings[i].ID != findings[j].ID {
			return findings[i].ID < findings[j].ID
		}
		return findings[i].Finding < findings[j].Finding
	})

	events := []eventlog.Event{parserStatusEvent(runID, request, result, "testssl_json", "parsed", "", len(findings))}
	for index, finding := range findings {
		events = append(events, tlsFindingFactEvent(runID, request, result, "testssl_json", finding, index))
	}
	return events, nil
}

// sslscanParserEvents converts sslscan XML output into TLS assessment facts.
func sslscanParserEvents(runID string, request ToolExecutionRequest, result ToolExecutionResult) ([]eventlog.Event, error) {
	if len(result.Transcript) > maxTLSParserBytes {
		return nil, fmt.Errorf("sslscan parser output exceeds %d bytes", maxTLSParserBytes)
	}
	output := strings.TrimSpace(result.Transcript)
	if output == "" {
		return nil, fmt.Errorf("sslscan parser output is empty")
	}

	findings, err := sslscanFindingsFromXML(output)
	if err != nil {
		return nil, err
	}
	if len(findings) == 0 {
		return nil, fmt.Errorf("sslscan output contains no findings")
	}

	events := []eventlog.Event{parserStatusEvent(runID, request, result, "sslscan_xml", "parsed", "", len(findings))}
	for index, finding := range findings {
		events = append(events, tlsFindingFactEvent(runID, request, result, "sslscan_xml", finding, index))
	}
	return events, nil
}

type tlsFinding struct {
	ID       string
	Severity string
	Finding  string
	Protocol string
}

func tlsFindingFromRaw(record map[string]any) (tlsFinding, bool) {
	finding := tlsFinding{
		ID:       firstNonEmptyRawString(record, "id", "ID"),
		Severity: strings.ToUpper(firstNonEmptyRawString(record, "severity", "Severity")),
		Finding:  firstNonEmptyRawString(record, "finding", "Finding"),
		Protocol: firstNonEmptyRawString(record, "protocol", "Protocol"),
	}
	if finding.ID == "" || finding.Finding == "" {
		return tlsFinding{}, false
	}
	if finding.Severity == "" {
		finding.Severity = "INFO"
	}
	return finding, true
}

func tlsFindingFactEvent(runID string, request ToolExecutionRequest, result ToolExecutionResult, parserID string, finding tlsFinding, index int) eventlog.Event {
	payload := map[string]any{
		"fact_id":      "fact-tls-" + safeFactIDPart(request.Node.ID) + "-" + safeFactIDPart(finding.ID) + "-" + strconv.Itoa(index),
		"kind":         "tls_assessment",
		"statement":    "tls | " + finding.ID + " | " + finding.Severity + " | " + finding.Finding,
		"confidence":   0.85,
		"source":       "parser." + parserID,
		"parser":       parserID,
		"parser_id":    parserID,
		"plan_node_id": request.Node.ID,
		"tool":         request.Node.ManifestName,
		"tool_id":      strings.TrimSpace(request.Node.ToolID),
		"operation":    strings.TrimSpace(request.Node.Operation),
		"execution_id": parserExecutionID(request, result),
		"check_id":     finding.ID,
		"severity":     finding.Severity,
		"finding":      finding.Finding,
	}
	if finding.Protocol != "" {
		payload["protocol"] = finding.Protocol
	}
	return eventlog.Event{RunID: runID, Type: "fact", Payload: mustJSON(payload)}
}
