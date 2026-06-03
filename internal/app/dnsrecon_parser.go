package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/netip"
	"strconv"
	"strings"

	"github.com/ouroboros/ouroboros/internal/eventlog"
)

const dnsreconParserMaxOutputBytes = 1 << 20

func dnsreconParserEvents(runID string, request ToolExecutionRequest, result ToolExecutionResult) ([]eventlog.Event, error) {
	if len(result.Transcript) > dnsreconParserMaxOutputBytes {
		return nil, fmt.Errorf("dnsrecon parser output exceeds %d bytes", dnsreconParserMaxOutputBytes)
	}
	output := strings.TrimSpace(result.Transcript)
	if output == "" {
		return nil, fmt.Errorf("dnsrecon parser output is empty")
	}
	if strings.TrimSpace(request.Node.ToolID) != "dnsrecon" || strings.TrimSpace(request.Node.Operation) != "dns_probe" {
		return nil, fmt.Errorf("dnsrecon parser requires dnsrecon/dns_probe tool execution")
	}
	targetDomain, err := dnsreconTargetDomain(request)
	if err != nil {
		return nil, err
	}
	parsed, err := parseDnsreconJSON([]byte(output), targetDomain, request.ScopeExclusions)
	if err != nil {
		return nil, err
	}
	events := []eventlog.Event{parserStatusEvent(runID, request, result, "dnsrecon_json", "parsed", "", len(parsed))}
	for index, surface := range parsed {
		events = append(events, dnsreconSurfaceFactEvent(runID, request, result, surface, index))
	}
	return events, nil
}

type dnsreconSurface struct {
	Domain     string
	Host       string
	RecordType string
	Value      string
	Address    string
	CNAME      string
	Confidence float64
}

func parseDnsreconJSON(output []byte, targetDomain string, exclusions []string) ([]dnsreconSurface, error) {
	trimmed := bytes.TrimSpace(output)
	var records []map[string]json.RawMessage
	if err := json.Unmarshal(trimmed, &records); err != nil {
		var wrapper map[string][]map[string]json.RawMessage
		if wrapperErr := json.Unmarshal(trimmed, &wrapper); wrapperErr != nil {
			return nil, fmt.Errorf("parse dnsrecon JSON: %w", err)
		}
		for _, values := range wrapper {
			records = append(records, values...)
		}
	}
	surfaces := []dnsreconSurface{}
	for _, record := range records {
		surface := dnsreconSurfaceFromRecord(record, targetDomain, exclusions)
		if surface.Host == "" {
			continue
		}
		surfaces = append(surfaces, surface)
	}
	return surfaces, nil
}

func dnsreconSurfaceFromRecord(record map[string]json.RawMessage, targetDomain string, exclusions []string) dnsreconSurface {
	host, ok := dnsreconRecordHost(record)
	if !ok || !subfinderDNSNameWithinScope(host, targetDomain) || subfinderDNSNameExcluded(host, exclusions) {
		return dnsreconSurface{}
	}
	recordType := strings.ToUpper(nucleiCleanMetadataValue(httpxStringField(record, "type"), 16))
	if recordType == "" {
		recordType = strings.ToUpper(nucleiCleanMetadataValue(httpxStringField(record, "record_type"), 16))
	}
	switch recordType {
	case "A", "AAAA":
		address := dnsreconAddressValue(record)
		if address == "" {
			return dnsreconSurface{}
		}
		return dnsreconSurface{
			Domain:     targetDomain,
			Host:       host,
			RecordType: recordType,
			Value:      address,
			Address:    address,
			Confidence: 0.65,
		}
	case "CNAME", "NS":
		name := dnsreconNameValue(record)
		if name == "" || !subfinderDNSNameWithinScope(name, targetDomain) || subfinderDNSNameExcluded(name, exclusions) {
			return dnsreconSurface{}
		}
		surface := dnsreconSurface{
			Domain:     targetDomain,
			Host:       host,
			RecordType: recordType,
			Value:      name,
			Confidence: 0.65,
		}
		if recordType == "CNAME" {
			surface.CNAME = name
		}
		return surface
	default:
		return dnsreconSurface{}
	}
}

func dnsreconRecordHost(record map[string]json.RawMessage) (string, bool) {
	for _, value := range subfinderStringFields(record, "name", "host", "domain") {
		host, ok := normalizeSubfinderDNSName(value)
		if ok {
			return host, true
		}
	}
	return "", false
}

func dnsreconAddressValue(record map[string]json.RawMessage) string {
	for _, value := range subfinderStringFields(record, "address", "ip", "value") {
		addr, err := netip.ParseAddr(strings.TrimSpace(value))
		if err == nil {
			return addr.String()
		}
	}
	return ""
}

func dnsreconNameValue(record map[string]json.RawMessage) string {
	for _, value := range subfinderStringFields(record, "target", "address", "value") {
		name, ok := normalizeSubfinderDNSName(value)
		if ok {
			return name
		}
	}
	return ""
}

func dnsreconSurfaceFactEvent(runID string, request ToolExecutionRequest, result ToolExecutionResult, surface dnsreconSurface, index int) eventlog.Event {
	payload := map[string]any{
		"fact_id":      dnsreconSurfaceFactID(request, surface, index),
		"kind":         "surface",
		"statement":    dnsreconSurfaceStatement(surface),
		"confidence":   surface.Confidence,
		"source":       "parser.dnsrecon_json",
		"parser":       "dnsrecon_json",
		"parser_id":    "dnsrecon_json",
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
	return eventlog.Event{RunID: runID, Type: eventlog.EventFact, Payload: mustJSON(payload)}
}

func dnsreconTargetDomain(request ToolExecutionRequest) (string, error) {
	target, ok := scopedTargetByKind(request.ScopeTargets, "domain")
	if !ok {
		return "", fmt.Errorf("dnsrecon parser requires explicit domain scope target")
	}
	domain, ok := normalizeSubfinderDNSName(target.Value)
	if !ok {
		return "", fmt.Errorf("dnsrecon parser domain scope target is invalid")
	}
	return domain, nil
}

func dnsreconSurfaceFactID(request ToolExecutionRequest, surface dnsreconSurface, index int) string {
	parts := []string{"fact", "dnsrecon", safeFactIDPart(request.Node.ID), safeFactIDPart(surface.Host), safeFactIDPart(surface.RecordType), strconv.Itoa(index)}
	return strings.Join(parts, "-")
}

func dnsreconSurfaceStatement(surface dnsreconSurface) string {
	return "dnsrecon | " + surface.Host + " | " + surface.RecordType + "=" + surface.Value + " | domain=" + surface.Domain
}
