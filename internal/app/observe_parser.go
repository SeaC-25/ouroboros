package app

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/ouroboros/ouroboros/internal/eventlog"
)

// observeParserEvents turns a raw http_observe result into observation
// events. It deliberately emits no finding/verdict facts: the observation is
// evidence the model reasons over, not a conclusion.
func observeParserEvents(runID string, request ToolExecutionRequest, result ToolExecutionResult) ([]eventlog.Event, error) {
	if len(result.Transcript) > maxArtifactScanParserBytes {
		return nil, fmt.Errorf("observe parser output exceeds %d bytes", maxArtifactScanParserBytes)
	}
	output := strings.TrimSpace(result.Transcript)
	if output == "" {
		return nil, fmt.Errorf("observe parser output is empty")
	}
	if strings.TrimSpace(request.Node.Operation) != "http_observe" {
		return nil, fmt.Errorf("observe parser requires http_observe tool execution")
	}
	var observation struct {
		URL         string   `json:"url"`
		Method      string   `json:"method"`
		Status      int      `json:"status"`
		StatusText  string   `json:"status_text"`
		BodySize    int      `json:"body_size"`
		ElapsedMS   float64  `json:"elapsed_ms"`
		Server      string   `json:"server"`
		ContentType string   `json:"content_type"`
		SetCookies  []string `json:"set_cookies"`
		Location    string   `json:"location"`
		Forms       []struct {
			Action string   `json:"action"`
			Method string   `json:"method"`
			Fields []string `json:"fields"`
		} `json:"forms"`
		Links    []string `json:"links"`
		JSONKeys []string `json:"json_keys"`
		Diff     *struct {
			StatusChanged  bool     `json:"status_changed"`
			ControlStatus  int      `json:"control_status"`
			SizeDelta      int      `json:"size_delta"`
			BodyChanged    bool     `json:"body_changed"`
			ElapsedDeltaMS float64  `json:"elapsed_delta_ms"`
			HeaderAdded    []string `json:"header_added"`
			HeaderRemoved  []string `json:"header_removed"`
		} `json:"diff"`
	}
	if err := json.Unmarshal([]byte(output), &observation); err != nil {
		return nil, fmt.Errorf("parse observation: %w", err)
	}

	events := []eventlog.Event{parserStatusEvent(runID, request, result, "observe_json", "parsed", "", 1)}
	payload := map[string]any{
		"fact_id":      strings.Join([]string{"fact", "observe", safeFactIDPart(request.Node.ID), "observation"}, "-"),
		"kind":         "observation",
		"statement":    observeStatement(&observation),
		"confidence":   1.0,
		"source":       "parser.observe_json",
		"parser":       "observe_json",
		"parser_id":    "observe_json",
		"plan_node_id": request.Node.ID,
		"tool":         request.Node.ManifestName,
		"tool_id":      strings.TrimSpace(request.Node.ToolID),
		"operation":    strings.TrimSpace(request.Node.Operation),
		"execution_id": parserExecutionID(request, result),
		"url":          observation.URL,
		"target":       observation.URL,
		"status":       observation.Status,
		"status_text":  observation.StatusText,
		"body_size":    observation.BodySize,
		"elapsed_ms":   observation.ElapsedMS,
		"server":       observation.Server,
		"content_type": observation.ContentType,
		"set_cookies":  observation.SetCookies,
		"location":     observation.Location,
		"forms":        observation.Forms,
		"links":        observation.Links,
		"json_keys":    observation.JSONKeys,
	}
	if observation.Diff != nil {
		payload["diff"] = observation.Diff
	}
	events = append(events, eventlog.Event{
		RunID:   runID,
		Type:    eventlog.EventFact,
		Payload: mustJSON(payload),
	})
	return events, nil
}

// observeStatement renders a compact human-readable observation line.
func observeStatement(o *struct {
	URL         string   `json:"url"`
	Method      string   `json:"method"`
	Status      int      `json:"status"`
	StatusText  string   `json:"status_text"`
	BodySize    int      `json:"body_size"`
	ElapsedMS   float64  `json:"elapsed_ms"`
	Server      string   `json:"server"`
	ContentType string   `json:"content_type"`
	SetCookies  []string `json:"set_cookies"`
	Location    string   `json:"location"`
	Forms       []struct {
		Action string   `json:"action"`
		Method string   `json:"method"`
		Fields []string `json:"fields"`
	} `json:"forms"`
	Links    []string `json:"links"`
	JSONKeys []string `json:"json_keys"`
	Diff     *struct {
		StatusChanged  bool     `json:"status_changed"`
		ControlStatus  int      `json:"control_status"`
		SizeDelta      int      `json:"size_delta"`
		BodyChanged    bool     `json:"body_changed"`
		ElapsedDeltaMS float64  `json:"elapsed_delta_ms"`
		HeaderAdded    []string `json:"header_added"`
		HeaderRemoved  []string `json:"header_removed"`
	} `json:"diff"`
}) string {
	parts := []string{
		"observe |", o.Method, o.URL,
		fmt.Sprintf("status=%d", o.Status),
		fmt.Sprintf("size=%d", o.BodySize),
		fmt.Sprintf("elapsed_ms=%.1f", o.ElapsedMS),
	}
	if o.Server != "" {
		parts = append(parts, "server="+o.Server)
	}
	if len(o.Forms) > 0 {
		parts = append(parts, fmt.Sprintf("forms=%d", len(o.Forms)))
	}
	if len(o.Links) > 0 {
		parts = append(parts, fmt.Sprintf("links=%d", len(o.Links)))
	}
	if o.Diff != nil {
		parts = append(parts, fmt.Sprintf("diff(status %d→%d, size Δ%d, elapsed Δ%.1fms)",
			o.Diff.ControlStatus, o.Status, o.Diff.SizeDelta, o.Diff.ElapsedDeltaMS))
	}
	return strings.Join(parts, " ")
}
