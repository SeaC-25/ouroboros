package app

import (
	"encoding/xml"
	"fmt"
	"net/netip"
	"strconv"
	"strings"

	"github.com/ouroboros/ouroboros/internal/eventlog"
)

const nmapParserMaxOutputBytes = 1 << 20

func nmapParserEvents(runID string, request ToolExecutionRequest, result ToolExecutionResult) ([]eventlog.Event, error) {
	output := strings.TrimSpace(result.Transcript)
	if output == "" {
		return nil, fmt.Errorf("nmap parser output is empty")
	}
	if len(output) > nmapParserMaxOutputBytes {
		return nil, fmt.Errorf("nmap parser output exceeds %d bytes", nmapParserMaxOutputBytes)
	}
	scope, prefix, err := nmapScopeTarget(request)
	if err != nil {
		return nil, err
	}
	parsed, err := parseNmapXML([]byte(output), scope, prefix)
	if err != nil {
		return nil, err
	}
	events := []eventlog.Event{parserStatusEvent(runID, request, result, "nmap_xml", "parsed", "", len(parsed))}
	for index, surface := range parsed {
		events = append(events, nmapSurfaceFactEvent(runID, request, result, surface, index))
	}
	return events, nil
}

type nmapXMLRun struct {
	Hosts []nmapXMLHost `xml:"host"`
}

type nmapXMLHost struct {
	Addresses []nmapXMLAddress `xml:"address"`
	Ports     []nmapXMLPort    `xml:"ports>port"`
}

type nmapXMLAddress struct {
	Addr     string `xml:"addr,attr"`
	AddrType string `xml:"addrtype,attr"`
}

type nmapXMLPort struct {
	Protocol string           `xml:"protocol,attr"`
	PortID   string           `xml:"portid,attr"`
	State    nmapXMLPortState `xml:"state"`
	Service  nmapXMLService   `xml:"service"`
}

type nmapXMLPortState struct {
	State string `xml:"state,attr"`
}

type nmapXMLService struct {
	Name   string `xml:"name,attr"`
	Tunnel string `xml:"tunnel,attr"`
	Conf   string `xml:"conf,attr"`
}

type nmapSurface struct {
	ScopeKind     string
	ScopeTarget   string
	Host          string
	Address       string
	AddressType   string
	Protocol      string
	Port          int
	PortState     string
	ServiceName   string
	ServiceTunnel string
	ServiceConf   int
	Confidence    float64
}

func parseNmapXML(output []byte, scope ScopeTarget, prefix netip.Prefix) ([]nmapSurface, error) {
	var decoded nmapXMLRun
	if err := xml.Unmarshal(output, &decoded); err != nil {
		return nil, fmt.Errorf("parse nmap XML: %w", err)
	}
	surfaces := []nmapSurface{}
	for _, host := range decoded.Hosts {
		binding, ok := nmapBindHost(host, scope, prefix)
		if !ok {
			continue
		}
		for _, port := range host.Ports {
			surface, ok := nmapSurfaceFromPort(binding, port)
			if !ok {
				continue
			}
			surfaces = append(surfaces, surface)
		}
	}
	return surfaces, nil
}

type nmapHostBinding struct {
	ScopeKind   string
	ScopeTarget string
	Host        string
	Address     string
	AddressType string
}

func nmapBindHost(host nmapXMLHost, scope ScopeTarget, prefix netip.Prefix) (nmapHostBinding, bool) {
	scopeKind := strings.TrimSpace(scope.Kind)
	scopeTarget := strings.TrimSpace(scope.Value)
	address := nmapPrimaryIPAddress(host.Addresses)
	switch scopeKind {
	case "host":
		return nmapHostBinding{
			ScopeKind:   scopeKind,
			ScopeTarget: scopeTarget,
			Host:        scopeTarget,
			Address:     address.Value,
			AddressType: address.Type,
		}, true
	case "cidr":
		if !address.Parsed.IsValid() || !prefix.Contains(address.Parsed) {
			return nmapHostBinding{}, false
		}
		return nmapHostBinding{
			ScopeKind:   scopeKind,
			ScopeTarget: scopeTarget,
			Host:        address.Value,
			Address:     address.Value,
			AddressType: address.Type,
		}, true
	default:
		return nmapHostBinding{}, false
	}
}

type nmapIPAddress struct {
	Value  string
	Type   string
	Parsed netip.Addr
}

func nmapPrimaryIPAddress(addresses []nmapXMLAddress) nmapIPAddress {
	for _, address := range addresses {
		value := strings.TrimSpace(address.Addr)
		parsed, err := netip.ParseAddr(value)
		if err != nil {
			continue
		}
		return nmapIPAddress{
			Value:  parsed.String(),
			Type:   nmapAddressType(address.AddrType, parsed),
			Parsed: parsed,
		}
	}
	return nmapIPAddress{}
}

func nmapAddressType(value string, address netip.Addr) string {
	value = strings.ToLower(nucleiCleanMetadataValue(value, 32))
	switch value {
	case "ipv4", "ipv6":
		return value
	}
	if address.Is4() {
		return "ipv4"
	}
	if address.Is6() {
		return "ipv6"
	}
	return ""
}

func nmapSurfaceFromPort(binding nmapHostBinding, port nmapXMLPort) (nmapSurface, bool) {
	state := strings.ToLower(strings.TrimSpace(port.State.State))
	if state != "open" {
		return nmapSurface{}, false
	}
	protocol := nmapProtocol(port.Protocol)
	if protocol == "" {
		return nmapSurface{}, false
	}
	portID, ok := nmapPortID(port.PortID)
	if !ok {
		return nmapSurface{}, false
	}
	serviceName := nmapServiceValue(port.Service.Name, 64)
	return nmapSurface{
		ScopeKind:     binding.ScopeKind,
		ScopeTarget:   binding.ScopeTarget,
		Host:          binding.Host,
		Address:       binding.Address,
		AddressType:   binding.AddressType,
		Protocol:      protocol,
		Port:          portID,
		PortState:     "open",
		ServiceName:   serviceName,
		ServiceTunnel: nmapServiceValue(port.Service.Tunnel, 32),
		ServiceConf:   nmapServiceConf(port.Service.Conf),
		Confidence:    nmapConfidence(port.Service.Conf),
	}, true
}

func nmapProtocol(value string) string {
	switch strings.ToLower(nucleiCleanMetadataValue(value, 16)) {
	case "tcp":
		return "tcp"
	case "udp":
		return "udp"
	case "sctp":
		return "sctp"
	default:
		return ""
	}
}

func nmapPortID(value string) (int, bool) {
	portID, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || portID < 1 || portID > 65535 {
		return 0, false
	}
	return portID, true
}

func nmapServiceValue(value string, maxRunes int) string {
	value = strings.ToLower(nucleiCleanMetadataValue(value, maxRunes))
	value = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.' {
			return r
		}
		return -1
	}, value)
	return strings.TrimSpace(value)
}

func nmapServiceConf(value string) int {
	conf, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || conf < 0 {
		return 0
	}
	return conf
}

func nmapConfidence(confValue string) float64 {
	conf := nmapServiceConf(confValue)
	if conf >= 8 {
		return 0.85
	}
	if conf > 0 {
		return 0.8
	}
	return 0.75
}

func nmapScopeTarget(request ToolExecutionRequest) (ScopeTarget, netip.Prefix, error) {
	if target, ok := scopedTargetByKind(request.ScopeTargets, "host"); ok {
		return target, netip.Prefix{}, nil
	}
	if target, ok := scopedTargetByKind(request.ScopeTargets, "cidr"); ok {
		prefix, err := netip.ParsePrefix(strings.TrimSpace(target.Value))
		if err != nil {
			return ScopeTarget{}, netip.Prefix{}, fmt.Errorf("nmap parser requires valid cidr scope target: %w", err)
		}
		return target, prefix, nil
	}
	return ScopeTarget{}, netip.Prefix{}, fmt.Errorf("nmap parser requires host or cidr scope target")
}

func nmapSurfaceFactEvent(runID string, request ToolExecutionRequest, result ToolExecutionResult, surface nmapSurface, index int) eventlog.Event {
	return eventlog.Event{
		RunID: runID,
		Type:  eventlog.EventFact,
		Payload: mustJSON(map[string]any{
			"fact_id":        nmapSurfaceFactID(request, surface, index),
			"kind":           "surface",
			"statement":      nmapSurfaceStatement(surface),
			"confidence":     surface.Confidence,
			"source":         "parser.nmap_xml",
			"parser":         "nmap_xml",
			"parser_id":      "nmap_xml",
			"plan_node_id":   request.Node.ID,
			"tool":           request.Node.ManifestName,
			"tool_id":        strings.TrimSpace(request.Node.ToolID),
			"operation":      strings.TrimSpace(request.Node.Operation),
			"execution_id":   parserExecutionID(request, result),
			"scope_kind":     surface.ScopeKind,
			"scope_target":   surface.ScopeTarget,
			"host":           surface.Host,
			"address":        surface.Address,
			"address_type":   surface.AddressType,
			"protocol":       surface.Protocol,
			"port":           surface.Port,
			"port_state":     surface.PortState,
			"service_name":   surface.ServiceName,
			"service_tunnel": surface.ServiceTunnel,
			"service_conf":   surface.ServiceConf,
		}),
	}
}

func nmapSurfaceFactID(request ToolExecutionRequest, surface nmapSurface, index int) string {
	parts := []string{
		"fact",
		"nmap",
		safeFactIDPart(request.Node.ID),
		safeFactIDPart(surface.ScopeKind),
		safeFactIDPart(surface.Host),
		safeFactIDPart(surface.Protocol),
		strconv.Itoa(surface.Port),
		strconv.Itoa(index),
	}
	return strings.Join(parts, "-")
}

func nmapSurfaceStatement(surface nmapSurface) string {
	return strings.Join([]string{
		"nmap",
		surface.Host,
		surface.Protocol + "/" + strconv.Itoa(surface.Port),
		"open",
		"service=" + surface.ServiceName,
	}, " | ")
}
