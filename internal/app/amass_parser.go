package app

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/ouroboros/ouroboros/internal/eventlog"
)

const amassParserMaxOutputBytes = 1 << 20

func amassParserEvents(runID string, request ToolExecutionRequest, result ToolExecutionResult) ([]eventlog.Event, error) {
	if len(result.Transcript) > amassParserMaxOutputBytes {
		return nil, fmt.Errorf("amass parser output exceeds %d bytes", amassParserMaxOutputBytes)
	}
	output := strings.TrimSpace(result.Transcript)
	if output == "" {
		return nil, fmt.Errorf("amass parser output is empty")
	}
	if strings.TrimSpace(request.Node.ToolID) != "amass" || strings.TrimSpace(request.Node.Operation) != "subdomain_discovery" {
		return nil, fmt.Errorf("amass parser requires amass/subdomain_discovery tool execution")
	}
	targetDomain, err := amassTargetDomain(request)
	if err != nil {
		return nil, err
	}
	parsed, err := parseAmassJSON([]byte(output), targetDomain, request.ScopeExclusions)
	if err != nil {
		return nil, err
	}
	events := []eventlog.Event{parserStatusEvent(runID, request, result, "amass_json", "parsed", "", len(parsed))}
	for index, surface := range parsed {
		events = append(events, amassSurfaceFactEvent(runID, request, result, surface, index))
	}
	return events, nil
}

type amassSurface struct {
	Domain     string
	Subdomain  string
	Confidence float64
}

func parseAmassJSON(output []byte, targetDomain string, exclusions []string) ([]amassSurface, error) {
	trimmed := bytes.TrimSpace(output)
	if len(trimmed) == 0 {
		return nil, nil
	}
	if bytes.HasPrefix(trimmed, []byte("[")) {
		var records []map[string]json.RawMessage
		if err := json.Unmarshal(trimmed, &records); err != nil {
			return nil, fmt.Errorf("parse amass JSON array: %w", err)
		}
		return amassSurfacesFromRecords(records, targetDomain, exclusions), nil
	}

	scanner := bufio.NewScanner(bytes.NewReader(output))
	scanner.Buffer(make([]byte, 0, 64*1024), amassParserMaxOutputBytes)
	records := []map[string]json.RawMessage{}
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var record map[string]json.RawMessage
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			return nil, fmt.Errorf("parse amass JSON line %d: %w", lineNumber, err)
		}
		records = append(records, record)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan amass JSON: %w", err)
	}
	return amassSurfacesFromRecords(records, targetDomain, exclusions), nil
}

func amassSurfacesFromRecords(records []map[string]json.RawMessage, targetDomain string, exclusions []string) []amassSurface {
	surfaces := []amassSurface{}
	seen := map[string]bool{}
	for _, record := range records {
		for _, value := range subfinderStringFields(record, "name", "host", "subdomain") {
			name, ok := normalizeSubfinderDNSName(value)
			if !ok || !subfinderDNSNameWithinScope(name, targetDomain) || subfinderDNSNameExcluded(name, exclusions) || seen[name] {
				continue
			}
			seen[name] = true
			surfaces = append(surfaces, amassSurface{Domain: targetDomain, Subdomain: name, Confidence: 0.65})
			break
		}
	}
	return surfaces
}

func amassSurfaceFactEvent(runID string, request ToolExecutionRequest, result ToolExecutionResult, surface amassSurface, index int) eventlog.Event {
	return eventlog.Event{
		RunID: runID,
		Type:  eventlog.EventFact,
		Payload: mustJSON(map[string]any{
			"fact_id":      amassSurfaceFactID(request, surface, index),
			"kind":         "surface",
			"statement":    amassSurfaceStatement(surface),
			"confidence":   surface.Confidence,
			"source":       "parser.amass_json",
			"parser":       "amass_json",
			"parser_id":    "amass_json",
			"plan_node_id": request.Node.ID,
			"tool":         request.Node.ManifestName,
			"tool_id":      strings.TrimSpace(request.Node.ToolID),
			"operation":    strings.TrimSpace(request.Node.Operation),
			"execution_id": parserExecutionID(request, result),
			"domain":       surface.Domain,
			"subdomain":    surface.Subdomain,
		}),
	}
}

func amassTargetDomain(request ToolExecutionRequest) (string, error) {
	target, ok := scopedTargetByKind(request.ScopeTargets, "domain")
	if !ok {
		return "", fmt.Errorf("amass parser requires explicit domain scope target")
	}
	domain, ok := normalizeSubfinderDNSName(target.Value)
	if !ok {
		return "", fmt.Errorf("amass parser domain scope target is invalid")
	}
	return domain, nil
}

func amassSurfaceFactID(request ToolExecutionRequest, surface amassSurface, index int) string {
	parts := []string{"fact", "amass", safeFactIDPart(request.Node.ID), safeFactIDPart(surface.Subdomain), strconv.Itoa(index)}
	return strings.Join(parts, "-")
}

func amassSurfaceStatement(surface amassSurface) string {
	return "amass | " + surface.Subdomain + " | domain=" + surface.Domain
}
