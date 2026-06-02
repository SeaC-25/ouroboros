package app

import (
	"strings"
)

type RuntimeWorkerToolPlanPayload struct {
	Action           string   `json:"action,omitempty"`
	ManifestName     string   `json:"manifest_name,omitempty"`
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
