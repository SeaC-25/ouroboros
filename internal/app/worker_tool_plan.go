package app

import (
	"strings"
)

type RuntimeWorkerToolPlanPayload struct {
	Action           string   `json:"action,omitempty"`
	ManifestName     string   `json:"manifest_name,omitempty"`
	ToolID           string   `json:"tool_id,omitempty"`
	AdapterID        string   `json:"adapter_id,omitempty"`
	Operation        string   `json:"operation,omitempty"`
	ParserID         string   `json:"parser_id,omitempty"`
	TargetKind       string   `json:"target_kind,omitempty"`
	DependencyName   string   `json:"dependency_name,omitempty"`
	DependencyKind   string   `json:"dependency_kind,omitempty"`
	Runner           string   `json:"runner,omitempty"`
	RunCommand       []string `json:"run_command,omitempty"`
	RunTimeoutMS     int      `json:"run_timeout_ms,omitempty"`
	Parser           string   `json:"parser,omitempty"`
	InputNodeIDs     []string `json:"input_node_ids,omitempty"`
	ExpectedEvidence []string `json:"expected_evidence,omitempty"`
}

func (payload RuntimeWorkerToolPlanPayload) IsZero() bool {
	return strings.TrimSpace(payload.Action) == "" &&
		strings.TrimSpace(payload.ManifestName) == "" &&
		strings.TrimSpace(payload.ToolID) == "" &&
		strings.TrimSpace(payload.AdapterID) == "" &&
		strings.TrimSpace(payload.Operation) == "" &&
		strings.TrimSpace(payload.ParserID) == "" &&
		strings.TrimSpace(payload.TargetKind) == "" &&
		strings.TrimSpace(payload.DependencyName) == "" &&
		strings.TrimSpace(payload.DependencyKind) == "" &&
		strings.TrimSpace(payload.Runner) == "" &&
		len(trimRuntimeWorkerToolPlanStrings(payload.RunCommand)) == 0 &&
		payload.RunTimeoutMS == 0 &&
		strings.TrimSpace(payload.Parser) == "" &&
		len(trimRuntimeWorkerToolPlanStrings(payload.InputNodeIDs)) == 0 &&
		len(trimRuntimeWorkerToolPlanStrings(payload.ExpectedEvidence)) == 0
}

func runtimeWorkerToolPlanPayloadFromToolExecutionRequest(request ToolExecutionRequest) RuntimeWorkerToolPlanPayload {
	expectedEvidence := make([]string, 0, len(request.Node.ExpectedEvidence))
	for _, nodeType := range request.Node.ExpectedEvidence {
		expectedEvidence = append(expectedEvidence, string(nodeType))
	}
	return normalizeRuntimeWorkerToolPlanPayload(RuntimeWorkerToolPlanPayload{
		Action:           string(request.Node.Action),
		ManifestName:     request.Node.ManifestName,
		ToolID:           request.Node.ToolID,
		AdapterID:        request.Node.AdapterID,
		Operation:        request.Node.Operation,
		ParserID:         request.Node.ParserID,
		TargetKind:       request.Node.TargetKind,
		DependencyName:   request.Node.DependencyName,
		DependencyKind:   request.Node.DependencyKind,
		Runner:           request.Node.Runner,
		RunCommand:       append([]string(nil), request.Node.RunCommand...),
		RunTimeoutMS:     request.Node.RunTimeoutMS,
		Parser:           request.Node.Parser,
		InputNodeIDs:     append([]string(nil), request.Node.InputNodeIDs...),
		ExpectedEvidence: expectedEvidence,
	}, request.Node.ManifestName)
}

func normalizeRuntimeWorkerToolPlanPayload(payload RuntimeWorkerToolPlanPayload, fallbackManifestName string) RuntimeWorkerToolPlanPayload {
	normalized := RuntimeWorkerToolPlanPayload{
		Action:           strings.TrimSpace(payload.Action),
		ManifestName:     strings.TrimSpace(payload.ManifestName),
		ToolID:           strings.TrimSpace(payload.ToolID),
		AdapterID:        strings.TrimSpace(payload.AdapterID),
		Operation:        strings.TrimSpace(payload.Operation),
		ParserID:         strings.TrimSpace(payload.ParserID),
		TargetKind:       strings.TrimSpace(payload.TargetKind),
		DependencyName:   strings.TrimSpace(payload.DependencyName),
		DependencyKind:   strings.TrimSpace(payload.DependencyKind),
		Runner:           strings.TrimSpace(payload.Runner),
		RunCommand:       trimRuntimeWorkerToolPlanStrings(payload.RunCommand),
		RunTimeoutMS:     payload.RunTimeoutMS,
		Parser:           strings.TrimSpace(payload.Parser),
		InputNodeIDs:     trimRuntimeWorkerToolPlanStrings(payload.InputNodeIDs),
		ExpectedEvidence: trimRuntimeWorkerToolPlanStrings(payload.ExpectedEvidence),
	}
	if !normalized.IsZero() && normalized.ManifestName == "" {
		normalized.ManifestName = strings.TrimSpace(fallbackManifestName)
	}
	return normalized
}

func trimRuntimeWorkerToolPlanStrings(values []string) []string {
	trimmed := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		trimmed = append(trimmed, value)
	}
	return trimmed
}
