package app

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"net/netip"
	"strconv"
	"strings"

	"github.com/ouroboros/ouroboros/internal/eventlog"
)

const dnsxParserMaxOutputBytes = 1 << 20

func dnsxParserEvents(runID string, request ToolExecutionRequest, result ToolExecutionResult) ([]eventlog.Event, error) {
	if len(result.Transcript) > dnsxParserMaxOutputBytes {
		return nil, fmt.Errorf("dnsx parser output exceeds %d bytes", dnsxParserMaxOutputBytes)
	}
	output := strings.TrimSpace(result.Transcript)
	if output == "" {
		return nil, fmt.Errorf("dnsx parser output is empty")
	}
	if strings.TrimSpace(request.Node.ToolID) != "dnsx" || strings.TrimSpace(request.Node.Operation) != "dns_probe" {
		return nil, fmt.Errorf("dnsx parser requires dnsx/dns_probe tool execution")
	}
	targetDomain, err := dnsxTargetDomain(request)
	if err != nil {
		return nil, err
	}
	parsed, err := parseDnsxJSONL([]byte(output), targetDomain, request.ScopeExclusions)
	if err != nil {
		return nil, err
	}
	events := []eventlog.Event{parserStatusEvent(runID, request, result, "dnsx_jsonl", "parsed", "", len(parsed))}
	for index, surface := range parsed {
		events = append(events, dnsxSurfaceFactEvent(runID, request, result, surface, index))
	}
	return events, nil
}

type dnsxSurface struct {
	Domain     string
	Host       string
	RecordType string
	Value      string
	Address    string
	CNAME      string
	Confidence float64
}

func parseDnsxJSONL(output []byte, targetDomain string, exclusions []string) ([]dnsxSurface, error) {
	scanner := bufio.NewScanner(bytes.NewReader(output))
	scanner.Buffer(make([]byte, 0, 64*1024), dnsxParserMaxOutputBytes)
	surfaces := []dnsxSurface{}
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var record map[string]json.RawMessage
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			return nil, fmt.Errorf("parse dnsx JSONL line %d: %w", lineNumber, err)
		}
		surfaces = append(surfaces, dnsxSurfacesFromRecord(record, targetDomain, exclusions)...)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan dnsx JSONL: %w", err)
	}
	return surfaces, nil
}

func dnsxSurfacesFromRecord(record map[string]json.RawMessage, targetDomain string, exclusions []string) []dnsxSurface {
	host, ok := dnsxRecordHost(record)
	if !ok || !subfinderDNSNameWithinScope(host, targetDomain) || subfinderDNSNameExcluded(host, exclusions) {
		return nil
	}
	surfaces := []dnsxSurface{}
	for _, value := range dnsxStringListField(record, "a") {
		addr, err := netip.ParseAddr(strings.TrimSpace(value))
		if err != nil || !addr.Is4() {
			continue
		}
		surfaces = append(surfaces, dnsxSurface{
			Domain:     targetDomain,
			Host:       host,
			RecordType: "A",
			Value:      addr.String(),
			Address:    addr.String(),
			Confidence: 0.65,
		})
	}
	for _, value := range dnsxStringListField(record, "aaaa") {
		addr, err := netip.ParseAddr(strings.TrimSpace(value))
		if err != nil || !addr.Is6() {
			continue
		}
		surfaces = append(surfaces, dnsxSurface{
			Domain:     targetDomain,
			Host:       host,
			RecordType: "AAAA",
			Value:      addr.String(),
			Address:    addr.String(),
			Confidence: 0.65,
		})
	}
	for _, value := range dnsxStringListField(record, "cname") {
		cname, ok := normalizeSubfinderDNSName(value)
		if !ok || !subfinderDNSNameWithinScope(cname, targetDomain) || subfinderDNSNameExcluded(cname, exclusions) {
			continue
		}
		surfaces = append(surfaces, dnsxSurface{
			Domain:     targetDomain,
			Host:       host,
			RecordType: "CNAME",
			Value:      cname,
			CNAME:      cname,
			Confidence: 0.65,
		})
	}
	return surfaces
}

func dnsxRecordHost(record map[string]json.RawMessage) (string, bool) {
	for _, value := range subfinderStringFields(record, "host", "input", "domain") {
		host, ok := normalizeSubfinderDNSName(value)
		if ok {
			return host, true
		}
	}
	return "", false
}

func dnsxStringListField(record map[string]json.RawMessage, name string) []string {
	raw, ok := record[name]
	if !ok {
		return nil
	}
	var values []string
	if err := json.Unmarshal(raw, &values); err == nil {
		return values
	}
	var value string
	if err := json.Unmarshal(raw, &value); err == nil {
		return []string{value}
	}
	return nil
}

func dnsxSurfaceFactEvent(runID string, request ToolExecutionRequest, result ToolExecutionResult, surface dnsxSurface, index int) eventlog.Event {
	payload := map[string]any{
		"fact_id":      dnsxSurfaceFactID(request, surface, index),
		"kind":         "surface",
		"statement":    dnsxSurfaceStatement(surface),
		"confidence":   surface.Confidence,
		"source":       "parser.dnsx_jsonl",
		"parser":       "dnsx_jsonl",
		"parser_id":    "dnsx_jsonl",
		"plan_node_id": request.Node.ID,
		"tool":         request.Node.ManifestName,
		"tool_id":      strings.TrimSpace(request.Node.ToolID),
		"operation":    strings.TrimSpace(request.Node.Operation),
		"execution_id": parserExecutionID(request, result),
		"domain":       surface.Domain,
		"host":         surface.Host,
		"record_type":  surface.RecordType,
		"value":        surface.Value,
	}
	if surface.Address != "" {
		payload["address"] = surface.Address
	}
	if surface.CNAME != "" {
		payload["cname"] = surface.CNAME
	}
	return eventlog.Event{
		RunID:   runID,
		Type:    eventlog.EventFact,
		Payload: mustJSON(payload),
	}
}

func dnsxTargetDomain(request ToolExecutionRequest) (string, error) {
	target, ok := scopedTargetByKind(request.ScopeTargets, "domain")
	if !ok {
		return "", fmt.Errorf("dnsx parser requires explicit domain scope target")
	}
	domain, ok := normalizeSubfinderDNSName(target.Value)
	if !ok {
		return "", fmt.Errorf("dnsx parser domain scope target is invalid")
	}
	return domain, nil
}

func dnsxSurfaceFactID(request ToolExecutionRequest, surface dnsxSurface, index int) string {
	parts := []string{"fact", "dnsx", safeFactIDPart(request.Node.ID), safeFactIDPart(surface.Host), safeFactIDPart(surface.RecordType), strconv.Itoa(index)}
	return strings.Join(parts, "-")
}

func dnsxSurfaceStatement(surface dnsxSurface) string {
	return "dnsx | " + surface.Host + " | " + surface.RecordType + "=" + surface.Value + " | domain=" + surface.Domain
}
