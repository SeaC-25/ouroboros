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

const httpxParserMaxOutputBytes = 1 << 20

var httpxContentTypePattern = regexp.MustCompile(`^[a-z0-9!#$&^_.+-]+/[a-z0-9!#$&^_.+-]+$`)

func httpxParserEvents(runID string, request ToolExecutionRequest, result ToolExecutionResult) ([]eventlog.Event, error) {
	output := strings.TrimSpace(result.Transcript)
	if output == "" {
		return nil, fmt.Errorf("httpx parser output is empty")
	}
	if len(output) > httpxParserMaxOutputBytes {
		return nil, fmt.Errorf("httpx parser output exceeds %d bytes", httpxParserMaxOutputBytes)
	}
	targetURL := httpxTargetURL(request)
	if strings.TrimSpace(targetURL) == "" {
		return nil, fmt.Errorf("httpx parser requires url scope target")
	}
	parsed, err := parseHttpxJSONL([]byte(output), targetURL)
	if err != nil {
		return nil, err
	}
	events := []eventlog.Event{parserStatusEvent(runID, request, result, "httpx_jsonl", "parsed", "", len(parsed))}
	for index, surface := range parsed {
		events = append(events, httpxSurfaceFactEvent(runID, request, result, surface, index))
	}
	return events, nil
}

type httpxSurface struct {
	URL           string
	TargetURL     string
	StatusCode    int
	ContentType   string
	ContentLength int64
	Confidence    float64
}

func parseHttpxJSONL(output []byte, targetURL string) ([]httpxSurface, error) {
	scanner := bufio.NewScanner(bytes.NewReader(output))
	scanner.Buffer(make([]byte, 0, 64*1024), httpxParserMaxOutputBytes)
	surfaces := []httpxSurface{}
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var record map[string]json.RawMessage
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			return nil, fmt.Errorf("parse httpx JSONL line %d: %w", lineNumber, err)
		}
		scopedURL := httpxScopedSurfaceURL(record, targetURL)
		if scopedURL.URL == "" {
			continue
		}
		statusCode := 0
		contentType := ""
		contentLength := int64(0)
		if scopedURL.ResponseScoped {
			statusCode = httpxStatusCode(record)
			contentType = httpxContentType(record)
			contentLength = httpxContentLength(record)
		}
		surfaces = append(surfaces, httpxSurface{
			URL:           scopedURL.URL,
			TargetURL:     webEvidenceURL(targetURL),
			StatusCode:    statusCode,
			ContentType:   contentType,
			ContentLength: contentLength,
			Confidence:    httpxConfidence(statusCode),
		})
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan httpx JSONL: %w", err)
	}
	return surfaces, nil
}

func httpxSurfaceFactEvent(runID string, request ToolExecutionRequest, result ToolExecutionResult, surface httpxSurface, index int) eventlog.Event {
	return eventlog.Event{
		RunID: runID,
		Type:  eventlog.EventFact,
		Payload: mustJSON(map[string]any{
			"fact_id":        httpxSurfaceFactID(request, surface, index),
			"kind":           "surface",
			"statement":      httpxSurfaceStatement(surface),
			"confidence":     surface.Confidence,
			"source":         "parser.httpx_jsonl",
			"parser":         "httpx_jsonl",
			"parser_id":      "httpx_jsonl",
			"plan_node_id":   request.Node.ID,
			"tool":           request.Node.ManifestName,
			"tool_id":        strings.TrimSpace(request.Node.ToolID),
			"operation":      strings.TrimSpace(request.Node.Operation),
			"execution_id":   parserExecutionID(request, result),
			"url":            surface.URL,
			"target_url":     surface.TargetURL,
			"status_code":    surface.StatusCode,
			"content_type":   surface.ContentType,
			"content_length": surface.ContentLength,
		}),
	}
}

func httpxSurfaceFactID(request ToolExecutionRequest, surface httpxSurface, index int) string {
	parts := []string{
		"fact",
		"httpx",
		safeFactIDPart(request.Node.ID),
		safeFactIDPart(surface.URL),
		strconv.Itoa(surface.StatusCode),
		strconv.Itoa(index),
	}
	return strings.Join(parts, "-")
}

func httpxSurfaceStatement(surface httpxSurface) string {
	parts := []string{"httpx", surface.URL}
	if surface.StatusCode > 0 {
		parts = append(parts, "status="+strconv.Itoa(surface.StatusCode))
	}
	if surface.ContentType != "" {
		parts = append(parts, "content_type="+surface.ContentType)
	}
	if surface.ContentLength > 0 {
		parts = append(parts, "length="+strconv.FormatInt(surface.ContentLength, 10))
	}
	return strings.Join(parts, " | ")
}

func httpxTargetURL(request ToolExecutionRequest) string {
	if target, ok := scopedTargetByKind(request.ScopeTargets, "url"); ok {
		return strings.TrimSpace(target.Value)
	}
	return ""
}

type httpxScopedURL struct {
	URL            string
	ResponseScoped bool
}

func httpxScopedSurfaceURL(record map[string]json.RawMessage, targetURL string) httpxScopedURL {
	targetURL = strings.TrimSpace(targetURL)
	if targetURL == "" {
		return httpxScopedURL{}
	}
	values := httpxStringFields(record, "url", "input")
	if httpxHasOutOfScopePrimaryURL(record, targetURL) {
		return httpxScopedURL{URL: webEvidenceURL(targetURL), ResponseScoped: false}
	}
	responseScoped := !httpxHasOutOfScopeFinalURL(record, targetURL)
	for _, value := range values {
		if webURLWithinScope(value, targetURL) {
			return httpxScopedURL{URL: webEvidenceURL(value), ResponseScoped: responseScoped}
		}
	}
	return httpxScopedURL{}
}

func httpxHasOutOfScopePrimaryURL(record map[string]json.RawMessage, targetURL string) bool {
	for _, value := range httpxStringFields(record, "url", "input") {
		if parsedWebURL(value) == nil {
			continue
		}
		if !webURLWithinScope(value, targetURL) {
			return true
		}
	}
	return false
}

func httpxHasOutOfScopeFinalURL(record map[string]json.RawMessage, targetURL string) bool {
	for _, value := range httpxStringFields(record, "final_url", "final-url", "location") {
		if parsedWebURL(value) == nil {
			continue
		}
		if !webURLWithinScope(value, targetURL) {
			return true
		}
	}
	return false
}

func httpxStatusCode(record map[string]json.RawMessage) int {
	statusCode := httpxIntField(record, "status_code", "status-code", "statusCode")
	if statusCode < 100 || statusCode > 599 {
		return 0
	}
	return statusCode
}

func httpxContentLength(record map[string]json.RawMessage) int64 {
	contentLength := httpxInt64Field(record, "content_length", "content-length", "contentLength")
	if contentLength < 0 {
		return 0
	}
	return contentLength
}

func httpxContentType(record map[string]json.RawMessage) string {
	value := httpxStringField(record, "content_type", "content-type", "contentType")
	if idx := strings.Index(value, ";"); idx >= 0 {
		value = value[:idx]
	}
	value = strings.ToLower(strings.TrimSpace(value))
	if !httpxContentTypePattern.MatchString(value) {
		return ""
	}
	return value
}

func httpxConfidence(statusCode int) float64 {
	if statusCode >= 100 && statusCode <= 599 {
		return 0.7
	}
	return 0.6
}

func httpxStringFields(record map[string]json.RawMessage, names ...string) []string {
	values := []string{}
	for _, name := range names {
		value := httpxStringField(record, name)
		if value != "" {
			values = append(values, value)
		}
	}
	return values
}

func httpxStringField(record map[string]json.RawMessage, names ...string) string {
	for _, name := range names {
		raw, ok := record[name]
		if !ok {
			continue
		}
		var value string
		if err := json.Unmarshal(raw, &value); err == nil {
			return nucleiCleanMetadataValue(value, 512)
		}
	}
	return ""
}

func httpxIntField(record map[string]json.RawMessage, names ...string) int {
	value := httpxInt64Field(record, names...)
	if value > int64(^uint(0)>>1) || value < -int64(^uint(0)>>1)-1 {
		return 0
	}
	return int(value)
}

func httpxInt64Field(record map[string]json.RawMessage, names ...string) int64 {
	for _, name := range names {
		raw, ok := record[name]
		if !ok {
			continue
		}
		var number int64
		if err := json.Unmarshal(raw, &number); err == nil {
			return number
		}
		var floatNumber float64
		if err := json.Unmarshal(raw, &floatNumber); err == nil {
			return int64(floatNumber)
		}
		var text string
		if err := json.Unmarshal(raw, &text); err == nil {
			parsed, err := strconv.ParseInt(strings.TrimSpace(text), 10, 64)
			if err == nil {
				return parsed
			}
		}
	}
	return 0
}
