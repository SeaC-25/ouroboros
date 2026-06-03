package app

import (
	"bufio"
	"bytes"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/ouroboros/ouroboros/internal/eventlog"
)

const urlListParserMaxOutputBytes = 1 << 20

func urlListParserEvents(runID string, request ToolExecutionRequest, result ToolExecutionResult) ([]eventlog.Event, error) {
	if len(result.Transcript) > urlListParserMaxOutputBytes {
		return nil, fmt.Errorf("url list parser output exceeds %d bytes", urlListParserMaxOutputBytes)
	}
	output := strings.TrimSpace(result.Transcript)
	if output == "" {
		return nil, fmt.Errorf("url list parser output is empty")
	}
	if strings.TrimSpace(request.Node.Operation) != "archive_discovery" {
		return nil, fmt.Errorf("url list parser requires archive_discovery tool execution")
	}
	targetDomain, err := urlListTargetDomain(request)
	if err != nil {
		return nil, err
	}
	parsed, err := parseURLListText([]byte(output), targetDomain, request.ScopeExclusions)
	if err != nil {
		return nil, err
	}
	events := []eventlog.Event{parserStatusEvent(runID, request, result, "url_list_text", "parsed", "", len(parsed))}
	for index, surface := range parsed {
		events = append(events, urlListSurfaceFactEvent(runID, request, result, surface, index))
	}
	return events, nil
}

type urlListSurface struct {
	Domain     string
	URL        string
	Confidence float64
}

func parseURLListText(output []byte, targetDomain string, exclusions []string) ([]urlListSurface, error) {
	scanner := bufio.NewScanner(bytes.NewReader(output))
	scanner.Buffer(make([]byte, 0, 64*1024), urlListParserMaxOutputBytes)
	surfaces := []urlListSurface{}
	seen := map[string]bool{}
	for scanner.Scan() {
		rawURL := strings.TrimSpace(scanner.Text())
		if rawURL == "" {
			continue
		}
		surface := urlListSurfaceFromRawURL(rawURL, targetDomain, exclusions)
		if surface.URL == "" || seen[surface.URL] {
			continue
		}
		seen[surface.URL] = true
		surfaces = append(surfaces, surface)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan url list text: %w", err)
	}
	return surfaces, nil
}

func urlListSurfaceFromRawURL(rawURL string, targetDomain string, exclusions []string) urlListSurface {
	parsed := parsedWebURL(rawURL)
	if parsed == nil {
		return urlListSurface{}
	}
	host, ok := normalizeSubfinderDNSName(parsed.Hostname())
	if !ok || !subfinderDNSNameWithinScope(host, targetDomain) || subfinderDNSNameExcluded(host, exclusions) {
		return urlListSurface{}
	}
	if urlListURLExcluded(rawURL, exclusions) {
		return urlListSurface{}
	}
	cleanURL := webEvidenceURL(rawURL)
	if cleanURL == "" {
		return urlListSurface{}
	}
	return urlListSurface{Domain: targetDomain, URL: cleanURL, Confidence: 0.6}
}

func urlListSurfaceFactEvent(runID string, request ToolExecutionRequest, result ToolExecutionResult, surface urlListSurface, index int) eventlog.Event {
	return eventlog.Event{
		RunID: runID,
		Type:  eventlog.EventFact,
		Payload: mustJSON(map[string]any{
			"fact_id":      urlListSurfaceFactID(request, surface, index),
			"kind":         "surface",
			"statement":    urlListSurfaceStatement(surface),
			"confidence":   surface.Confidence,
			"source":       "parser.url_list_text",
			"parser":       "url_list_text",
			"parser_id":    "url_list_text",
			"plan_node_id": request.Node.ID,
			"tool":         request.Node.ManifestName,
			"tool_id":      strings.TrimSpace(request.Node.ToolID),
			"operation":    strings.TrimSpace(request.Node.Operation),
			"execution_id": parserExecutionID(request, result),
			"url":          surface.URL,
			"domain":       surface.Domain,
		}),
	}
}

func urlListTargetDomain(request ToolExecutionRequest) (string, error) {
	target, ok := scopedTargetByKind(request.ScopeTargets, "domain")
	if !ok {
		return "", fmt.Errorf("url list parser requires explicit domain scope target")
	}
	domain, ok := normalizeSubfinderDNSName(target.Value)
	if !ok {
		return "", fmt.Errorf("url list parser domain scope target is invalid")
	}
	return domain, nil
}

func urlListURLExcluded(rawURL string, exclusions []string) bool {
	cleanURL := webEvidenceURL(rawURL)
	if cleanURL == "" {
		return true
	}
	for _, exclusion := range exclusions {
		exclusion = strings.TrimSpace(exclusion)
		if exclusion == "" {
			continue
		}
		if parsedWebURL(exclusion) != nil {
			if webURLWithinScope(rawURL, exclusion) {
				return true
			}
			continue
		}
		if wildcardPatternMatches(exclusion, cleanURL) {
			return true
		}
	}
	return false
}

func urlListSurfaceFactID(request ToolExecutionRequest, surface urlListSurface, index int) string {
	parts := []string{"fact", "url-list", safeFactIDPart(request.Node.ID), safeFactIDPart(surface.URL), strconv.Itoa(index)}
	return strings.Join(parts, "-")
}

func urlListSurfaceStatement(surface urlListSurface) string {
	return "archive | " + surface.URL + " | domain=" + surface.Domain
}

func archiveDiscoveryDomainFromURL(value string) string {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || parsed.Host == "" {
		return ""
	}
	domain, ok := normalizeSubfinderDNSName(parsed.Hostname())
	if !ok {
		return ""
	}
	return domain
}
