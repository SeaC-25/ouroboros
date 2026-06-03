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

const subfinderParserMaxOutputBytes = 1 << 20

func subfinderParserEvents(runID string, request ToolExecutionRequest, result ToolExecutionResult) ([]eventlog.Event, error) {
	if len(result.Transcript) > subfinderParserMaxOutputBytes {
		return nil, fmt.Errorf("subfinder parser output exceeds %d bytes", subfinderParserMaxOutputBytes)
	}
	output := strings.TrimSpace(result.Transcript)
	if output == "" {
		return nil, fmt.Errorf("subfinder parser output is empty")
	}
	if strings.TrimSpace(request.Node.ToolID) != "subfinder" || strings.TrimSpace(request.Node.Operation) != "subdomain_discovery" {
		return nil, fmt.Errorf("subfinder parser requires subfinder/subdomain_discovery tool execution")
	}
	targetDomain, err := subfinderTargetDomain(request)
	if err != nil {
		return nil, err
	}
	parsed, err := parseSubfinderJSONL([]byte(output), targetDomain, request.ScopeExclusions)
	if err != nil {
		return nil, err
	}
	events := []eventlog.Event{parserStatusEvent(runID, request, result, "subfinder_jsonl", "parsed", "", len(parsed))}
	for index, surface := range parsed {
		events = append(events, subfinderSurfaceFactEvent(runID, request, result, surface, index))
	}
	return events, nil
}

type subfinderSurface struct {
	Domain     string
	Subdomain  string
	Confidence float64
}

func parseSubfinderJSONL(output []byte, targetDomain string, exclusions []string) ([]subfinderSurface, error) {
	scanner := bufio.NewScanner(bytes.NewReader(output))
	scanner.Buffer(make([]byte, 0, 64*1024), subfinderParserMaxOutputBytes)
	surfaces := []subfinderSurface{}
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var record map[string]json.RawMessage
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			return nil, fmt.Errorf("parse subfinder JSONL line %d: %w", lineNumber, err)
		}
		for _, value := range subfinderCandidateValues(record) {
			name, ok := normalizeSubfinderDNSName(value)
			if !ok || !subfinderDNSNameWithinScope(name, targetDomain) || subfinderDNSNameExcluded(name, exclusions) {
				continue
			}
			surfaces = append(surfaces, subfinderSurface{
				Domain:     targetDomain,
				Subdomain:  name,
				Confidence: 0.65,
			})
			break
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan subfinder JSONL: %w", err)
	}
	return surfaces, nil
}

func subfinderSurfaceFactEvent(runID string, request ToolExecutionRequest, result ToolExecutionResult, surface subfinderSurface, index int) eventlog.Event {
	return eventlog.Event{
		RunID: runID,
		Type:  eventlog.EventFact,
		Payload: mustJSON(map[string]any{
			"fact_id":      subfinderSurfaceFactID(request, surface, index),
			"kind":         "surface",
			"statement":    subfinderSurfaceStatement(surface),
			"confidence":   surface.Confidence,
			"source":       "parser.subfinder_jsonl",
			"parser":       "subfinder_jsonl",
			"parser_id":    "subfinder_jsonl",
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

func subfinderTargetDomain(request ToolExecutionRequest) (string, error) {
	target, ok := scopedTargetByKind(request.ScopeTargets, "domain")
	if !ok {
		return "", fmt.Errorf("subfinder parser requires explicit domain scope target")
	}
	domain, ok := normalizeSubfinderDNSName(target.Value)
	if !ok {
		return "", fmt.Errorf("subfinder parser domain scope target is invalid")
	}
	return domain, nil
}

func subfinderCandidateValues(record map[string]json.RawMessage) []string {
	values := subfinderStringFields(record, "host", "subdomain")
	if len(values) > 0 {
		return values
	}
	return subfinderStringFields(record, "input", "domain")
}

func subfinderStringFields(record map[string]json.RawMessage, names ...string) []string {
	values := []string{}
	for _, name := range names {
		raw, ok := record[name]
		if !ok {
			continue
		}
		var value string
		if err := json.Unmarshal(raw, &value); err == nil {
			values = append(values, value)
		}
	}
	return values
}

func normalizeSubfinderDNSName(value string) (string, bool) {
	if value == "" || strings.HasPrefix(value, "*.") {
		return "", false
	}
	if strings.HasSuffix(value, ".") {
		value = strings.TrimSuffix(value, ".")
	}
	if value == "" {
		return "", false
	}
	value = strings.ToLower(value)
	if len(value) > 253 {
		return "", false
	}
	labels := strings.Split(value, ".")
	for _, label := range labels {
		if !validSubfinderDNSLabel(label) {
			return "", false
		}
	}
	return value, true
}

func validSubfinderDNSLabel(label string) bool {
	if label == "" || len(label) > 63 || strings.HasPrefix(label, "-") || strings.HasSuffix(label, "-") {
		return false
	}
	for _, r := range label {
		if r < 0x21 || r == 0x7f || r > 0x7f {
			return false
		}
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			continue
		}
		return false
	}
	return true
}

func subfinderDNSNameWithinScope(name string, targetDomain string) bool {
	return name == targetDomain || strings.HasSuffix(name, "."+targetDomain)
}

func subfinderDNSNameExcluded(name string, exclusions []string) bool {
	for _, exclusion := range exclusions {
		exclusion = strings.ToLower(strings.TrimSpace(exclusion))
		if exclusion == "" {
			continue
		}
		if strings.HasPrefix(exclusion, "*.") {
			suffix, ok := normalizeSubfinderDNSName(strings.TrimPrefix(exclusion, "*."))
			if ok && strings.HasSuffix(name, "."+suffix) {
				return true
			}
			continue
		}
		normalized, ok := normalizeSubfinderDNSName(exclusion)
		if ok && name == normalized {
			return true
		}
		if strings.Contains(exclusion, "*") && wildcardPatternMatches(exclusion, name) {
			return true
		}
	}
	return false
}

func subfinderSurfaceFactID(request ToolExecutionRequest, surface subfinderSurface, index int) string {
	parts := []string{"fact", "subfinder", safeFactIDPart(request.Node.ID), safeFactIDPart(surface.Subdomain), strconv.Itoa(index)}
	return strings.Join(parts, "-")
}

func subfinderSurfaceStatement(surface subfinderSurface) string {
	return "subfinder | " + surface.Subdomain + " | domain=" + surface.Domain
}
