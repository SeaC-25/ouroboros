package compiler

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/ouroboros/ouroboros/internal/blackboard"
	"github.com/ouroboros/ouroboros/internal/eventlog"
	"github.com/ouroboros/ouroboros/internal/sisyphus"
)

type ParserSpec struct {
	Name            string
	FactKind        string
	StatementField  string
	ConfidenceField string
	RequiredFields  []string
}

type ParserBindingRequest struct {
	RunID    string
	PlanNode PlanNode
	Spec     ParserSpec
	Output   []byte
	Store    *blackboard.MemoryStore
	IntentID string
	LeaseID  string
	Attempt  sisyphus.Attempt
}

type ParserBindingResult struct {
	Fact     blackboard.Fact
	Events   []eventlog.Event
	Fallback *sisyphus.TaskResult
}

func BindParserOutput(request ParserBindingRequest) (ParserBindingResult, error) {
	if strings.TrimSpace(request.RunID) == "" {
		return ParserBindingResult{}, errors.New("run id is required")
	}
	if request.Store == nil {
		return ParserBindingResult{}, errors.New("blackboard store is required")
	}
	if err := validateParserSpec(request.Spec); err != nil {
		return ParserBindingResult{}, err
	}
	if request.PlanNode.Parser != "" && request.PlanNode.Parser != request.Spec.Name {
		return ParserBindingResult{}, fmt.Errorf("plan parser %s does not match parser spec %s", request.PlanNode.Parser, request.Spec.Name)
	}

	parsed, err := parseToolOutput(request.Output, request.Spec)
	if err != nil {
		return concludeAfterParseFailure(request, err)
	}

	fact, err := request.Store.AppendFact(blackboard.Fact{
		Kind:       request.Spec.FactKind,
		Statement:  parsed.Statement,
		Confidence: parsed.Confidence,
		Source:     "parser." + request.Spec.Name,
	})
	if err != nil {
		return ParserBindingResult{}, err
	}

	events := []eventlog.Event{
		newParserEvent(request.RunID, request.PlanNode.ID, request.Spec.Name, "parsed", ""),
		newFactEvent(request.RunID, request.PlanNode.ID, request.Spec.Name, fact),
	}
	return ParserBindingResult{
		Fact:   fact,
		Events: events,
	}, nil
}

type parsedToolOutput struct {
	Statement  string
	Confidence float64
}

func validateParserSpec(spec ParserSpec) error {
	var errs []error
	if strings.TrimSpace(spec.Name) == "" {
		errs = append(errs, errors.New("parser spec name is required"))
	}
	if strings.TrimSpace(spec.FactKind) == "" {
		errs = append(errs, errors.New("parser fact kind is required"))
	}
	if strings.TrimSpace(spec.StatementField) == "" {
		errs = append(errs, errors.New("parser statement field is required"))
	}
	return errors.Join(errs...)
}

func parseToolOutput(output []byte, spec ParserSpec) (parsedToolOutput, error) {
	if len(output) == 0 {
		return parsedToolOutput{}, errors.New("tool output is empty")
	}

	var decoded map[string]any
	if err := json.Unmarshal(output, &decoded); err != nil {
		return parsedToolOutput{}, fmt.Errorf("parse JSON tool output: %w", err)
	}
	for _, field := range spec.RequiredFields {
		if strings.TrimSpace(fieldValue(decoded[field])) == "" {
			return parsedToolOutput{}, fmt.Errorf("required parser field %s is missing", field)
		}
	}

	statement := fieldValue(decoded[spec.StatementField])
	if strings.TrimSpace(statement) == "" {
		return parsedToolOutput{}, fmt.Errorf("statement field %s is missing", spec.StatementField)
	}
	confidence := 0.5
	if spec.ConfidenceField != "" {
		confidence = confidenceValue(decoded[spec.ConfidenceField], confidence)
	}
	return parsedToolOutput{
		Statement:  statement,
		Confidence: confidence,
	}, nil
}

func concludeAfterParseFailure(request ParserBindingRequest, parseErr error) (ParserBindingResult, error) {
	if strings.TrimSpace(request.IntentID) == "" {
		return ParserBindingResult{}, fmt.Errorf("parser failed and intent id is required for conclude fallback: %w", parseErr)
	}
	if strings.TrimSpace(request.LeaseID) == "" {
		return ParserBindingResult{}, fmt.Errorf("parser failed and lease id is required for conclude fallback: %w", parseErr)
	}

	attempt := request.Attempt
	if strings.TrimSpace(attempt.IntentID) == "" {
		attempt.IntentID = request.IntentID
	}
	if attempt.TaskKind == "" {
		attempt.TaskKind = sisyphus.TaskExplore
	}
	if strings.TrimSpace(attempt.PartialTranscript) == "" {
		attempt.PartialTranscript = string(request.Output)
	}

	failure := sisyphus.FailureReason{
		Kind:    sisyphus.FailureParseFailed,
		Message: fmt.Sprintf("parser %s failed: %v", request.Spec.Name, parseErr),
	}
	fallback, err := (sisyphus.Engine{}).ConcludeFallback(request.Store, request.IntentID, request.LeaseID, attempt, failure)
	if err != nil {
		return ParserBindingResult{}, err
	}

	events := []eventlog.Event{
		newParserEvent(request.RunID, request.PlanNode.ID, request.Spec.Name, "parse_failed", failure.Message),
		newFactEvent(request.RunID, request.PlanNode.ID, request.Spec.Name, fallback.Fact),
	}
	return ParserBindingResult{
		Fact:     fallback.Fact,
		Events:   events,
		Fallback: &fallback,
	}, nil
}

func fieldValue(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(typed)
	default:
		data, err := json.Marshal(typed)
		if err != nil {
			return strings.TrimSpace(fmt.Sprint(typed))
		}
		return strings.TrimSpace(string(data))
	}
}

func confidenceValue(value any, fallback float64) float64 {
	var parsed float64
	switch typed := value.(type) {
	case float64:
		parsed = typed
	case float32:
		parsed = float64(typed)
	case int:
		parsed = float64(typed)
	case string:
		value, err := strconv.ParseFloat(strings.TrimSpace(typed), 64)
		if err != nil {
			return fallback
		}
		parsed = value
	default:
		return fallback
	}
	if parsed < 0 {
		return 0
	}
	if parsed > 1 {
		return 1
	}
	return parsed
}

func newParserEvent(runID string, planNodeID string, parserName string, status string, message string) eventlog.Event {
	return eventlog.Event{
		RunID: runID,
		Type:  eventlog.EventParser,
		Payload: mustJSON(map[string]any{
			"plan_node_id": planNodeID,
			"parser":       parserName,
			"status":       status,
			"message":      message,
		}),
		CreatedAt: time.Now().UTC(),
	}
}

func newFactEvent(runID string, planNodeID string, parserName string, fact blackboard.Fact) eventlog.Event {
	return eventlog.Event{
		RunID: runID,
		Type:  eventlog.EventFact,
		Payload: mustJSON(map[string]any{
			"plan_node_id": planNodeID,
			"parser":       parserName,
			"fact_id":      fact.ID,
			"kind":         fact.Kind,
			"statement":    fact.Statement,
			"confidence":   fact.Confidence,
			"source":       fact.Source,
		}),
		CreatedAt: time.Now().UTC(),
	}
}

func mustJSON(value any) json.RawMessage {
	data, err := json.Marshal(value)
	if err != nil {
		return json.RawMessage(`{}`)
	}
	return json.RawMessage(data)
}
