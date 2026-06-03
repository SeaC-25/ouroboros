package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/ouroboros/ouroboros/internal/eventlog"
)

const ffufParserMaxOutputBytes = 1 << 20

func ffufParserEvents(runID string, request ToolExecutionRequest, result ToolExecutionResult) ([]eventlog.Event, error) {
	output := strings.TrimSpace(result.Transcript)
	if output == "" {
		return nil, fmt.Errorf("ffuf parser output is empty")
	}
	if len(output) > ffufParserMaxOutputBytes {
		return nil, fmt.Errorf("ffuf parser output exceeds %d bytes", ffufParserMaxOutputBytes)
	}
	targetURL := ffufTargetURL(request)
	if strings.TrimSpace(targetURL) == "" {
		return nil, fmt.Errorf("ffuf parser requires url scope target")
	}
	parsed, err := parseFfufJSON([]byte(output), targetURL, request.ScopeExclusions)
	if err != nil {
		return nil, err
	}
	events := []eventlog.Event{parserStatusEvent(runID, request, result, "ffuf_json", "parsed", "", len(parsed))}
	for index, route := range parsed {
		events = append(events, ffufRouteFactEvent(runID, request, result, route, index))
	}
	return events, nil
}

type ffufRoute struct {
	URL           string
	TargetURL     string
	StatusCode    int
	ContentLength int64
	ContentWords  int64
	ContentLines  int64
	ContentType   string
	Confidence    float64
}

func parseFfufJSON(output []byte, targetURL string, exclusions []string) ([]ffufRoute, error) {
	decoder := json.NewDecoder(bytes.NewReader(bytes.TrimSpace(output)))
	routes := []ffufRoute{}
	recordNumber := 0
	for {
		var raw json.RawMessage
		err := decoder.Decode(&raw)
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("parse ffuf JSON record %d: %w", recordNumber+1, err)
		}
		recordNumber++
		records, err := ffufResultRecords(raw)
		if err != nil {
			return nil, fmt.Errorf("parse ffuf JSON record %d: %w", recordNumber, err)
		}
		for _, record := range records {
			route := ffufRouteFromRecord(record, targetURL, exclusions)
			if route.URL == "" {
				continue
			}
			routes = append(routes, route)
		}
	}
	if recordNumber == 0 {
		return nil, fmt.Errorf("ffuf parser output is empty")
	}
	return routes, nil
}

func ffufResultRecords(raw json.RawMessage) ([]map[string]json.RawMessage, error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return nil, nil
	}
	var records []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &records); err == nil {
		return records, nil
	}
	var record map[string]json.RawMessage
	if err := json.Unmarshal(raw, &record); err != nil {
		return nil, err
	}
	if results, ok := record["results"]; ok {
		if err := json.Unmarshal(results, &records); err != nil {
			return nil, fmt.Errorf("parse ffuf results array: %w", err)
		}
		return records, nil
	}
	return []map[string]json.RawMessage{record}, nil
}

func ffufRouteFromRecord(record map[string]json.RawMessage, targetURL string, exclusions []string) ffufRoute {
	rawURL := ffufRouteURL(record)
	if rawURL == "" || !webURLWithinScope(rawURL, targetURL) || ffufURLExcluded(rawURL, exclusions) {
		return ffufRoute{}
	}
	url := webEvidenceURL(rawURL)
	if url == "" {
		return ffufRoute{}
	}
	statusCode := ffufStatusCode(record)
	return ffufRoute{
		URL:           url,
		TargetURL:     webEvidenceURL(targetURL),
		StatusCode:    statusCode,
		ContentLength: ffufNonNegativeInt64(record, "length", "content_length", "content-length", "contentLength", "contentlength"),
		ContentWords:  ffufNonNegativeInt64(record, "words", "content_words", "content-words", "contentWords", "contentwords"),
		ContentLines:  ffufNonNegativeInt64(record, "lines", "content_lines", "content-lines", "contentLines", "contentlines"),
		ContentType:   httpxContentType(record),
		Confidence:    ffufConfidence(statusCode),
	}
}

func ffufURLExcluded(rawURL string, exclusions []string) bool {
	return scopeTargetExcluded(ScopeTarget{Kind: "url", Value: rawURL}, exclusions)
}

func ffufRouteFactEvent(runID string, request ToolExecutionRequest, result ToolExecutionResult, route ffufRoute, index int) eventlog.Event {
	return eventlog.Event{
		RunID: runID,
		Type:  eventlog.EventFact,
		Payload: mustJSON(map[string]any{
			"fact_id":        ffufRouteFactID(request, route, index),
			"kind":           "surface",
			"statement":      ffufRouteStatement(route),
			"confidence":     route.Confidence,
			"source":         "parser.ffuf_json",
			"parser":         "ffuf_json",
			"parser_id":      "ffuf_json",
			"plan_node_id":   request.Node.ID,
			"tool":           request.Node.ManifestName,
			"tool_id":        strings.TrimSpace(request.Node.ToolID),
			"operation":      strings.TrimSpace(request.Node.Operation),
			"execution_id":   parserExecutionID(request, result),
			"url":            route.URL,
			"target_url":     route.TargetURL,
			"status_code":    route.StatusCode,
			"content_length": route.ContentLength,
			"content_words":  route.ContentWords,
			"content_lines":  route.ContentLines,
			"content_type":   route.ContentType,
		}),
	}
}

func ffufRouteFactID(request ToolExecutionRequest, route ffufRoute, index int) string {
	parts := []string{
		"fact",
		"ffuf",
		safeFactIDPart(request.Node.ID),
		safeFactIDPart(route.URL),
		strconv.Itoa(route.StatusCode),
		strconv.Itoa(index),
	}
	return strings.Join(parts, "-")
}

func ffufRouteStatement(route ffufRoute) string {
	parts := []string{"ffuf", route.URL}
	if route.StatusCode > 0 {
		parts = append(parts, "status="+strconv.Itoa(route.StatusCode))
	}
	if route.ContentLength > 0 {
		parts = append(parts, "length="+strconv.FormatInt(route.ContentLength, 10))
	}
	if route.ContentWords > 0 {
		parts = append(parts, "words="+strconv.FormatInt(route.ContentWords, 10))
	}
	if route.ContentLines > 0 {
		parts = append(parts, "lines="+strconv.FormatInt(route.ContentLines, 10))
	}
	if route.ContentType != "" {
		parts = append(parts, "content_type="+route.ContentType)
	}
	return strings.Join(parts, " | ")
}

func ffufTargetURL(request ToolExecutionRequest) string {
	if target, ok := scopedTargetByKind(request.ScopeTargets, "url"); ok {
		return strings.TrimSpace(target.Value)
	}
	return ""
}

func ffufRouteURL(record map[string]json.RawMessage) string {
	for _, value := range httpxStringFields(record, "url") {
		if parsedWebURL(value) != nil {
			return value
		}
	}
	return ""
}

func ffufStatusCode(record map[string]json.RawMessage) int {
	statusCode := httpxIntField(record, "status", "status_code", "status-code", "statusCode", "statuscode")
	if statusCode < 100 || statusCode > 599 {
		return 0
	}
	return statusCode
}

func ffufNonNegativeInt64(record map[string]json.RawMessage, names ...string) int64 {
	value := httpxInt64Field(record, names...)
	if value < 0 {
		return 0
	}
	return value
}

func ffufConfidence(statusCode int) float64 {
	if statusCode >= 100 && statusCode <= 599 {
		return 0.7
	}
	return 0.6
}
