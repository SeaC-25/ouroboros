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

const arjunParserMaxOutputBytes = 1 << 20

func arjunParserEvents(runID string, request ToolExecutionRequest, result ToolExecutionResult) ([]eventlog.Event, error) {
	output := strings.TrimSpace(result.Transcript)
	if output == "" {
		return nil, fmt.Errorf("arjun parser output is empty")
	}
	if len(output) > arjunParserMaxOutputBytes {
		return nil, fmt.Errorf("arjun parser output exceeds %d bytes", arjunParserMaxOutputBytes)
	}
	targetURL := arjunTargetURL(request)
	if strings.TrimSpace(targetURL) == "" {
		return nil, fmt.Errorf("arjun parser requires url scope target")
	}
	parsed, err := parseArjunJSON([]byte(output), targetURL, request.ScopeExclusions)
	if err != nil {
		return nil, err
	}
	events := []eventlog.Event{parserStatusEvent(runID, request, result, "arjun_json", "parsed", "", len(parsed))}
	for index, parameter := range parsed {
		events = append(events, arjunParameterFactEvent(runID, request, result, parameter, index))
	}
	return events, nil
}

type arjunParameter struct {
	URL        string
	TargetURL  string
	Parameter  string
	Confidence float64
}

func parseArjunJSON(output []byte, targetURL string, exclusions []string) ([]arjunParameter, error) {
	targetURL = strings.TrimSpace(targetURL)
	if targetURL == "" {
		return nil, fmt.Errorf("arjun parser requires url scope target")
	}
	if scopeTargetExcluded(ScopeTarget{Kind: "url", Value: targetURL}, exclusions) {
		return nil, nil
	}
	decoder := json.NewDecoder(bytes.NewReader(bytes.TrimSpace(output)))
	parameters := []arjunParameter{}
	recordNumber := 0
	seen := map[string]struct{}{}
	for {
		var raw json.RawMessage
		err := decoder.Decode(&raw)
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("parse arjun JSON record %d: %w", recordNumber+1, err)
		}
		recordNumber++
		records, err := arjunParameterRecords(raw, targetURL, exclusions)
		if err != nil {
			return nil, fmt.Errorf("parse arjun JSON record %d: %w", recordNumber, err)
		}
		for _, parameter := range records {
			key := parameter.URL + "\x00" + parameter.Parameter
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			parameters = append(parameters, parameter)
		}
	}
	if recordNumber == 0 {
		return nil, fmt.Errorf("arjun parser output is empty")
	}
	return parameters, nil
}

func arjunParameterRecords(raw json.RawMessage, targetURL string, exclusions []string) ([]arjunParameter, error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return nil, nil
	}
	var array []json.RawMessage
	if err := json.Unmarshal(raw, &array); err == nil {
		parameters := []arjunParameter{}
		for _, item := range array {
			parsed, err := arjunParameterRecords(item, targetURL, exclusions)
			if err != nil {
				return nil, err
			}
			parameters = append(parameters, parsed...)
		}
		return parameters, nil
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(raw, &object); err != nil {
		return nil, err
	}
	return arjunParameterRecordsFromObject(object, targetURL, exclusions)
}

func arjunParameterRecordsFromObject(object map[string]json.RawMessage, targetURL string, exclusions []string) ([]arjunParameter, error) {
	parameters := []arjunParameter{}
	for key, value := range object {
		key = strings.TrimSpace(key)
		if key == "" || arjunIgnoredTopLevelKey(key) {
			continue
		}
		if parsedWebURL(key) != nil {
			parameters = append(parameters, arjunParametersForURL(key, value, targetURL, exclusions)...)
			continue
		}
		if strings.EqualFold(key, "url") || strings.EqualFold(key, "target") || strings.EqualFold(key, "target_url") {
			reportedURL := arjunStringFromRaw(value)
			if reportedURL != "" {
				parameters = append(parameters, arjunParametersFromSiblingObject(reportedURL, object, targetURL, exclusions)...)
			}
		}
	}
	if len(parameters) > 0 {
		return parameters, nil
	}
	parameters = append(parameters, arjunParametersFromSiblingObject("", object, targetURL, exclusions)...)
	return parameters, nil
}

func arjunParametersForURL(rawURL string, rawParams json.RawMessage, targetURL string, exclusions []string) []arjunParameter {
	if !webURLWithinScope(rawURL, targetURL) || scopeTargetExcluded(ScopeTarget{Kind: "url", Value: rawURL}, exclusions) {
		return nil
	}
	url := webEvidenceURL(rawURL)
	if url == "" {
		return nil
	}
	return arjunParametersForEvidenceURL(url, targetURL, rawParams)
}

func arjunParametersFromSiblingObject(rawURL string, object map[string]json.RawMessage, targetURL string, exclusions []string) []arjunParameter {
	if strings.TrimSpace(rawURL) == "" {
		rawURL = targetURL
	}
	if !webURLWithinScope(rawURL, targetURL) || scopeTargetExcluded(ScopeTarget{Kind: "url", Value: rawURL}, exclusions) {
		return nil
	}
	url := webEvidenceURL(rawURL)
	if url == "" {
		return nil
	}
	for _, name := range []string{"parameters", "params", "param", "query_params", "query-params", "queryParams"} {
		if rawParams, ok := object[name]; ok {
			return arjunParametersForEvidenceURL(url, targetURL, rawParams)
		}
	}
	return nil
}

func arjunParametersForEvidenceURL(url string, targetURL string, rawParams json.RawMessage) []arjunParameter {
	names := arjunParameterNames(rawParams)
	parameters := []arjunParameter{}
	for _, name := range names {
		name = arjunCleanParameterName(name)
		if name == "" {
			continue
		}
		parameters = append(parameters, arjunParameter{
			URL:        url,
			TargetURL:  webEvidenceURL(targetURL),
			Parameter:  name,
			Confidence: 0.65,
		})
	}
	return parameters
}

func arjunParameterNames(raw json.RawMessage) []string {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return nil
	}
	var stringsArray []string
	if err := json.Unmarshal(raw, &stringsArray); err == nil {
		return stringsArray
	}
	var objectArray []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &objectArray); err == nil {
		names := []string{}
		for _, object := range objectArray {
			if name := httpxStringField(object, "name", "param", "parameter", "key"); name != "" {
				names = append(names, name)
			}
		}
		return names
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(raw, &object); err == nil {
		names := []string{}
		for key, value := range object {
			if arjunIgnoredTopLevelKey(key) {
				continue
			}
			if name := httpxStringField(map[string]json.RawMessage{"value": value}, "value"); name != "" && !strings.EqualFold(name, key) {
				names = append(names, name)
				continue
			}
			names = append(names, key)
		}
		return names
	}
	if name := arjunStringFromRaw(raw); name != "" {
		return []string{name}
	}
	return nil
}

func arjunStringFromRaw(raw json.RawMessage) string {
	var value string
	if err := json.Unmarshal(raw, &value); err == nil {
		return strings.TrimSpace(value)
	}
	return ""
}

func arjunIgnoredTopLevelKey(key string) bool {
	switch strings.ToLower(strings.TrimSpace(key)) {
	case "", "raw", "headers", "header", "request", "response", "body", "payload", "evidence", "cookies", "cookie", "include", "includes", "error", "errors":
		return true
	default:
		return false
	}
}

func arjunParameterFactEvent(runID string, request ToolExecutionRequest, result ToolExecutionResult, parameter arjunParameter, index int) eventlog.Event {
	return eventlog.Event{
		RunID: runID,
		Type:  eventlog.EventFact,
		Payload: mustJSON(map[string]any{
			"fact_id":      arjunParameterFactID(request, parameter, index),
			"kind":         "surface",
			"statement":    arjunParameterStatement(parameter),
			"confidence":   parameter.Confidence,
			"source":       "parser.arjun_json",
			"parser":       "arjun_json",
			"parser_id":    "arjun_json",
			"plan_node_id": request.Node.ID,
			"tool":         request.Node.ManifestName,
			"tool_id":      strings.TrimSpace(request.Node.ToolID),
			"operation":    strings.TrimSpace(request.Node.Operation),
			"execution_id": parserExecutionID(request, result),
			"url":          parameter.URL,
			"target_url":   parameter.TargetURL,
			"parameter":    parameter.Parameter,
		}),
	}
}

func arjunParameterFactID(request ToolExecutionRequest, parameter arjunParameter, index int) string {
	parts := []string{
		"fact",
		"arjun",
		safeFactIDPart(request.Node.ID),
		safeFactIDPart(parameter.URL),
		safeFactIDPart(parameter.Parameter),
		strconv.Itoa(index),
	}
	return strings.Join(parts, "-")
}

func arjunParameterStatement(parameter arjunParameter) string {
	return strings.Join([]string{"arjun", parameter.URL, "parameter=" + parameter.Parameter}, " | ")
}

func arjunTargetURL(request ToolExecutionRequest) string {
	if target, ok := scopedTargetByKind(request.ScopeTargets, "url"); ok {
		return strings.TrimSpace(target.Value)
	}
	return ""
}

func arjunCleanParameterName(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || strings.ContainsAny(value, "\x00\r\n\t ?#&=/") {
		return ""
	}
	return nucleiCleanMetadataValue(value, 120)
}
