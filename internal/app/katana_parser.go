package app

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/ouroboros/ouroboros/internal/eventlog"
)

const katanaParserMaxOutputBytes = 1 << 20

var katanaMethodPattern = regexp.MustCompile(`^[A-Z]{1,16}$`)

func katanaParserEvents(runID string, request ToolExecutionRequest, result ToolExecutionResult) ([]eventlog.Event, error) {
	output := strings.TrimSpace(result.Transcript)
	if output == "" {
		return nil, fmt.Errorf("katana parser output is empty")
	}
	if len(output) > katanaParserMaxOutputBytes {
		return nil, fmt.Errorf("katana parser output exceeds %d bytes", katanaParserMaxOutputBytes)
	}
	targetURL := katanaTargetURL(request)
	if strings.TrimSpace(targetURL) == "" {
		return nil, fmt.Errorf("katana parser requires url scope target")
	}
	parsed, err := parseKatanaJSONL([]byte(output), targetURL)
	if err != nil {
		return nil, err
	}
	events := []eventlog.Event{parserStatusEvent(runID, request, result, "katana_jsonl", "parsed", "", len(parsed))}
	for index, route := range parsed {
		events = append(events, katanaRouteFactEvent(runID, request, result, route, index))
	}
	return events, nil
}

type katanaRoute struct {
	URL           string
	TargetURL     string
	Method        string
	StatusCode    int
	ContentLength int64
	Depth         int
	Confidence    float64
}

func parseKatanaJSONL(output []byte, targetURL string) ([]katanaRoute, error) {
	scanner := bufio.NewScanner(bytes.NewReader(output))
	scanner.Buffer(make([]byte, 0, 64*1024), katanaParserMaxOutputBytes)
	routes := []katanaRoute{}
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var record map[string]json.RawMessage
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			return nil, fmt.Errorf("parse katana JSONL line %d: %w", lineNumber, err)
		}
		route := katanaRouteFromRecord(record, targetURL)
		if route.URL == "" {
			continue
		}
		routes = append(routes, route)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan katana JSONL: %w", err)
	}
	return routes, nil
}

func katanaRouteFromRecord(record map[string]json.RawMessage, targetURL string) katanaRoute {
	rawURL := katanaRouteURL(record)
	if rawURL == "" || !webURLWithinScope(rawURL, targetURL) {
		return katanaRoute{}
	}
	url := webEvidenceURL(rawURL)
	if url == "" {
		return katanaRoute{}
	}
	statusCode := katanaStatusCode(record)
	return katanaRoute{
		URL:           url,
		TargetURL:     webEvidenceURL(targetURL),
		Method:        katanaMethod(record),
		StatusCode:    statusCode,
		ContentLength: katanaContentLength(record),
		Depth:         katanaDepth(record),
		Confidence:    katanaConfidence(statusCode),
	}
}

func katanaRouteFactEvent(runID string, request ToolExecutionRequest, result ToolExecutionResult, route katanaRoute, index int) eventlog.Event {
	return eventlog.Event{
		RunID: runID,
		Type:  eventlog.EventFact,
		Payload: mustJSON(map[string]any{
			"fact_id":        katanaRouteFactID(request, route, index),
			"kind":           "surface",
			"statement":      katanaRouteStatement(route),
			"confidence":     route.Confidence,
			"source":         "parser.katana_jsonl",
			"parser":         "katana_jsonl",
			"parser_id":      "katana_jsonl",
			"plan_node_id":   request.Node.ID,
			"tool":           request.Node.ManifestName,
			"tool_id":        strings.TrimSpace(request.Node.ToolID),
			"operation":      strings.TrimSpace(request.Node.Operation),
			"execution_id":   parserExecutionID(request, result),
			"url":            route.URL,
			"target_url":     route.TargetURL,
			"method":         route.Method,
			"status_code":    route.StatusCode,
			"content_length": route.ContentLength,
			"depth":          route.Depth,
		}),
	}
}

func katanaRouteFactID(request ToolExecutionRequest, route katanaRoute, index int) string {
	parts := []string{
		"fact",
		"katana",
		safeFactIDPart(request.Node.ID),
		safeFactIDPart(route.URL),
		safeFactIDPart(route.Method),
		strconv.Itoa(route.StatusCode),
		strconv.Itoa(index),
	}
	return strings.Join(parts, "-")
}

func katanaRouteStatement(route katanaRoute) string {
	parts := []string{"katana", route.URL}
	if route.Method != "" {
		parts = append(parts, "method="+route.Method)
	}
	if route.StatusCode > 0 {
		parts = append(parts, "status="+strconv.Itoa(route.StatusCode))
	}
	if route.ContentLength > 0 {
		parts = append(parts, "length="+strconv.FormatInt(route.ContentLength, 10))
	}
	if route.Depth > 0 {
		parts = append(parts, "depth="+strconv.Itoa(route.Depth))
	}
	return strings.Join(parts, " | ")
}

func katanaTargetURL(request ToolExecutionRequest) string {
	if target, ok := scopedTargetByKind(request.ScopeTargets, "url"); ok {
		return strings.TrimSpace(target.Value)
	}
	return ""
}

func katanaRouteURL(record map[string]json.RawMessage) string {
	for _, value := range httpxStringFields(record, "url", "endpoint") {
		if parsedWebURL(value) != nil {
			return value
		}
	}
	if request := katanaNestedObject(record, "request"); request != nil {
		for _, value := range httpxStringFields(request, "url", "endpoint") {
			if parsedWebURL(value) != nil {
				return value
			}
		}
	}
	return ""
}

func katanaMethod(record map[string]json.RawMessage) string {
	value := httpxStringField(record, "method")
	if value == "" {
		if request := katanaNestedObject(record, "request"); request != nil {
			value = httpxStringField(request, "method")
		}
	}
	value = strings.ToUpper(nucleiCleanMetadataValue(value, 16))
	if !katanaMethodPattern.MatchString(value) {
		return ""
	}
	return value
}

func katanaStatusCode(record map[string]json.RawMessage) int {
	statusCode := httpxIntField(record, "status_code", "status-code", "statusCode", "statuscode")
	if statusCode == 0 {
		if response := katanaNestedObject(record, "response"); response != nil {
			statusCode = httpxIntField(response, "status_code", "status-code", "statusCode", "statuscode")
		}
	}
	if statusCode < 100 || statusCode > 599 {
		return 0
	}
	return statusCode
}

func katanaContentLength(record map[string]json.RawMessage) int64 {
	contentLength := httpxInt64Field(record, "content_length", "content-length", "contentLength", "contentlength")
	if contentLength == 0 {
		if response := katanaNestedObject(record, "response"); response != nil {
			contentLength = httpxInt64Field(response, "content_length", "content-length", "contentLength", "contentlength")
		}
	}
	if contentLength < 0 {
		return 0
	}
	return contentLength
}

func katanaDepth(record map[string]json.RawMessage) int {
	depth := httpxIntField(record, "depth")
	if depth < 0 {
		return 0
	}
	return depth
}

func katanaConfidence(statusCode int) float64 {
	if statusCode >= 100 && statusCode <= 599 {
		return 0.75
	}
	return 0.65
}

func katanaNestedObject(record map[string]json.RawMessage, name string) map[string]json.RawMessage {
	raw, ok := record[name]
	if !ok {
		return nil
	}
	var nested map[string]json.RawMessage
	if err := json.Unmarshal(raw, &nested); err != nil {
		return nil
	}
	return nested
}
