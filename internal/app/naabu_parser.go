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

const naabuParserMaxOutputBytes = 1 << 20

func naabuParserEvents(runID string, request ToolExecutionRequest, result ToolExecutionResult) ([]eventlog.Event, error) {
	if len(result.Transcript) > naabuParserMaxOutputBytes {
		return nil, fmt.Errorf("naabu parser output exceeds %d bytes", naabuParserMaxOutputBytes)
	}
	output := strings.TrimSpace(result.Transcript)
	if output == "" {
		return nil, fmt.Errorf("naabu parser output is empty")
	}
	if strings.TrimSpace(request.Node.ToolID) != "naabu" || strings.TrimSpace(request.Node.Operation) != "port_scan" {
		return nil, fmt.Errorf("naabu parser requires naabu/port_scan tool execution")
	}
	targetCIDR, prefix, err := naabuTargetCIDR(request)
	if err != nil {
		return nil, err
	}
	parsed, err := parseNaabuJSONL([]byte(output), targetCIDR, prefix, request.ScopeExclusions)
	if err != nil {
		return nil, err
	}
	events := []eventlog.Event{parserStatusEvent(runID, request, result, "naabu_jsonl", "parsed", "", len(parsed))}
	for index, surface := range parsed {
		events = append(events, naabuSurfaceFactEvent(runID, request, result, surface, index))
	}
	return events, nil
}

type naabuSurface struct {
	IP         string
	Port       int
	Protocol   string
	Endpoint   string
	TargetCIDR string
	Confidence float64
}

func parseNaabuJSONL(output []byte, targetCIDR string, prefix netip.Prefix, exclusions []string) ([]naabuSurface, error) {
	scanner := bufio.NewScanner(bytes.NewReader(output))
	scanner.Buffer(make([]byte, 0, 64*1024), naabuParserMaxOutputBytes)
	surfaces := []naabuSurface{}
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var record map[string]json.RawMessage
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			return nil, fmt.Errorf("parse naabu JSONL line %d: %w", lineNumber, err)
		}
		surface, ok := naabuSurfaceFromRecord(record, targetCIDR, prefix, exclusions)
		if ok {
			surfaces = append(surfaces, surface)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan naabu JSONL: %w", err)
	}
	return surfaces, nil
}

func naabuSurfaceFromRecord(record map[string]json.RawMessage, targetCIDR string, prefix netip.Prefix, exclusions []string) (naabuSurface, bool) {
	addr, ok := naabuRecordIP(record)
	if !ok || !prefix.Contains(addr) || naabuIPExcluded(addr, exclusions) {
		return naabuSurface{}, false
	}
	port, ok := naabuRecordPort(record)
	if !ok {
		return naabuSurface{}, false
	}
	protocol := naabuRecordProtocol(record)
	if protocol == "" {
		return naabuSurface{}, false
	}
	endpoint := naabuEndpoint(addr, port)
	return naabuSurface{
		IP:         addr.String(),
		Port:       port,
		Protocol:   protocol,
		Endpoint:   endpoint,
		TargetCIDR: targetCIDR,
		Confidence: 0.65,
	}, true
}

func naabuSurfaceFactEvent(runID string, request ToolExecutionRequest, result ToolExecutionResult, surface naabuSurface, index int) eventlog.Event {
	return eventlog.Event{
		RunID: runID,
		Type:  eventlog.EventFact,
		Payload: mustJSON(map[string]any{
			"fact_id":      naabuSurfaceFactID(request, surface, index),
			"kind":         "surface",
			"statement":    naabuSurfaceStatement(surface),
			"confidence":   surface.Confidence,
			"source":       "parser.naabu_jsonl",
			"parser":       "naabu_jsonl",
			"parser_id":    "naabu_jsonl",
			"plan_node_id": request.Node.ID,
			"tool":         request.Node.ManifestName,
			"tool_id":      strings.TrimSpace(request.Node.ToolID),
			"operation":    strings.TrimSpace(request.Node.Operation),
			"execution_id": parserExecutionID(request, result),
			"ip":           surface.IP,
			"port":         surface.Port,
			"protocol":     surface.Protocol,
			"endpoint":     surface.Endpoint,
			"target_cidr":  surface.TargetCIDR,
		}),
	}
}

func naabuTargetCIDR(request ToolExecutionRequest) (string, netip.Prefix, error) {
	target, ok := scopedTargetByKind(request.ScopeTargets, "cidr")
	if !ok {
		return "", netip.Prefix{}, fmt.Errorf("naabu parser requires cidr scope target")
	}
	prefix, err := netip.ParsePrefix(strings.TrimSpace(target.Value))
	if err != nil {
		return "", netip.Prefix{}, fmt.Errorf("naabu parser requires valid cidr scope target: %w", err)
	}
	return strings.TrimSpace(target.Value), prefix, nil
}

func naabuRecordIP(record map[string]json.RawMessage) (netip.Addr, bool) {
	for _, name := range []string{"ip", "host"} {
		raw, ok := record[name]
		if !ok {
			continue
		}
		var value string
		if err := json.Unmarshal(raw, &value); err != nil {
			continue
		}
		addr, err := netip.ParseAddr(strings.TrimSpace(value))
		if err == nil {
			return addr, true
		}
	}
	return netip.Addr{}, false
}

func naabuRecordPort(record map[string]json.RawMessage) (int, bool) {
	raw, ok := record["port"]
	if !ok {
		return 0, false
	}
	var numeric int
	if err := json.Unmarshal(raw, &numeric); err == nil {
		return naabuValidPort(numeric)
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return 0, false
	}
	port, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return 0, false
	}
	return naabuValidPort(port)
}

func naabuValidPort(port int) (int, bool) {
	if port < 1 || port > 65535 {
		return 0, false
	}
	return port, true
}

func naabuRecordProtocol(record map[string]json.RawMessage) string {
	protocol := strings.ToLower(naabuStringField(record, "protocol"))
	if protocol == "" {
		return "tcp"
	}
	switch protocol {
	case "tcp", "udp":
		return protocol
	default:
		return ""
	}
}

func naabuStringField(record map[string]json.RawMessage, name string) string {
	raw, ok := record[name]
	if !ok {
		return ""
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return ""
	}
	return nucleiCleanMetadataValue(value, 32)
}

func naabuIPExcluded(addr netip.Addr, exclusions []string) bool {
	return scopeTargetExcluded(ScopeTarget{Kind: "cidr", Value: addr.String()}, exclusions)
}

func naabuEndpoint(addr netip.Addr, port int) string {
	if addr.Is6() {
		return "[" + addr.String() + "]:" + strconv.Itoa(port)
	}
	return addr.String() + ":" + strconv.Itoa(port)
}

func naabuSurfaceFactID(request ToolExecutionRequest, surface naabuSurface, index int) string {
	parts := []string{
		"fact",
		"naabu",
		safeFactIDPart(request.Node.ID),
		safeFactIDPart(surface.IP),
		safeFactIDPart(surface.Protocol),
		strconv.Itoa(surface.Port),
		strconv.Itoa(index),
	}
	return strings.Join(parts, "-")
}

func naabuSurfaceStatement(surface naabuSurface) string {
	return strings.Join([]string{
		"naabu",
		surface.Endpoint,
		"open",
		"protocol=" + surface.Protocol,
	}, " | ")
}
