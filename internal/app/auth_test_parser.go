package app

import (
	"bufio"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/ouroboros/ouroboros/internal/eventlog"
)

// authTestParserMaxOutputBytes bounds the JSONL transcript the auth_test
// parser accepts, matching the other tool parsers.
const authTestParserMaxOutputBytes = 1 << 20

// authTestParserEvents converts hydra-style credential testing JSONL into
// finding facts for successful credentials only. The native handler
// (internal/nativetools/authtest.go) emits one record per attempted pair:
// {"username":...,"password":...,"place":...,"success":...,"attempts":N}.
func authTestParserEvents(runID string, request ToolExecutionRequest, result ToolExecutionResult) ([]eventlog.Event, error) {
	if len(result.Transcript) > authTestParserMaxOutputBytes {
		return nil, fmt.Errorf("auth_test parser output exceeds %d bytes", authTestParserMaxOutputBytes)
	}
	output := strings.TrimSpace(result.Transcript)
	if output == "" {
		return nil, fmt.Errorf("auth_test parser output is empty")
	}
	if strings.TrimSpace(request.Node.ToolID) != "hydra" || strings.TrimSpace(request.Node.Operation) != "auth_test" {
		return nil, fmt.Errorf("auth_test parser requires hydra/auth_test tool execution")
	}

	scanner := bufio.NewScanner(strings.NewReader(output))
	scanner.Buffer(make([]byte, 0, 64*1024), authTestParserMaxOutputBytes)
	hits := []authTestRecord{}
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var record authTestRecord
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			return nil, fmt.Errorf("parse auth_test JSONL: %w", err)
		}
		if !record.Success {
			continue
		}
		hits = append(hits, record)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan auth_test output: %w", err)
	}
	if len(hits) == 0 {
		return nil, fmt.Errorf("auth_test output contains no successful credentials")
	}

	events := []eventlog.Event{parserStatusEvent(runID, request, result, "auth_test_jsonl", "parsed", "", len(hits))}
	for index, hit := range hits {
		events = append(events, authTestFindingFactEvent(runID, request, result, hit, index))
	}
	return events, nil
}

// authTestRecord is one JSONL line emitted by the native auth_test handler.
type authTestRecord struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Place    string `json:"place"`
	Success  bool   `json:"success"`
	Attempts int    `json:"attempts"`
}

// maskAuthSecret keeps the first and last two characters of a discovered
// secret and masks the middle with middle dots, so reports never carry the
// full plaintext credential.
func maskAuthSecret(secret string) string {
	runes := []rune(strings.TrimSpace(secret))
	if len(runes) == 0 {
		return ""
	}
	if len(runes) <= 4 {
		return strings.Repeat("·", len(runes))
	}
	return string(runes[:2]) + strings.Repeat("·", len(runes)-4) + string(runes[len(runes)-2:])
}

// authTestFindingFactEvent emits one finding fact per successful credential.
func authTestFindingFactEvent(runID string, request ToolExecutionRequest, result ToolExecutionResult, record authTestRecord, index int) eventlog.Event {
	statement := strings.Join([]string{
		"hydra",
		"valid credentials",
		"user=" + strings.TrimSpace(record.Username),
		"pass=" + maskAuthSecret(record.Password),
	}, " | ")
	payload := map[string]any{
		"kind":         "finding",
		"statement":    statement,
		"confidence":   0.95,
		"source":       "parser.auth_test_jsonl",
		"parser":       "auth_test_jsonl",
		"parser_id":    "auth_test_jsonl",
		"plan_node_id": request.Node.ID,
		"tool":         request.Node.ManifestName,
		"tool_id":      strings.TrimSpace(request.Node.ToolID),
		"operation":    strings.TrimSpace(request.Node.Operation),
		"execution_id": parserExecutionID(request, result),
		"username":     strings.TrimSpace(record.Username),
		"password":     maskAuthSecret(record.Password),
		"place":        strings.TrimSpace(record.Place),
		"attempts":     record.Attempts,
		"fact_id":      strings.Join([]string{"fact", "hydra", safeFactIDPart(request.Node.ID), safeFactIDPart(record.Username), strconv.Itoa(index)}, "-"),
	}
	return eventlog.Event{
		RunID:   runID,
		Type:    eventlog.EventFact,
		Payload: mustJSON(payload),
	}
}
