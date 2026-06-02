package app

import (
	"testing"

	"github.com/ouroboros/ouroboros/internal/compiler"
)

func TestRuntimeWorkerToolPlanPayloadCarriesToolLevelIdentity(t *testing.T) {
	payload := runtimeWorkerToolPlanPayloadFromToolExecutionRequest(ToolExecutionRequest{
		Node: compiler.PlanNode{
			Action:         compiler.ActionDiscoverAssets,
			ManifestName:   "web-api-pack",
			ToolID:         "httpx",
			AdapterID:      "cli.exec",
			Operation:      "http_probe",
			ParserID:       "httpx.jsonl",
			TargetKind:     "url",
			DependencyName: "httpx",
			DependencyKind: "binary",
			Runner:         "command",
			Parser:         "httpx.jsonl",
		},
	})

	if payload.ToolID != "httpx" || payload.AdapterID != "cli.exec" || payload.Operation != "http_probe" || payload.ParserID != "httpx.jsonl" {
		t.Fatalf("expected tool-level execution identity, got %#v", payload)
	}
	if payload.TargetKind != "url" || payload.DependencyName != "httpx" || payload.DependencyKind != "binary" {
		t.Fatalf("expected tool-level target/dependency metadata, got %#v", payload)
	}
}

func TestWorkerTransportCommandEnvCarriesToolLevelIdentity(t *testing.T) {
	env := workerTransportCommandEnv("/tmp/ouro-workspace", "/tmp/ouro-run", RuntimeWorkerTransportDispatchRequest{
		WorkerID:    "remote-web-1",
		RunID:       "run-1",
		ExecutionID: "tool-httpx",
		Tool:        "web-api-pack",
		Capability:  "web.discovery",
		ToolPlan: RuntimeWorkerToolPlanPayload{
			ManifestName:   "web-api-pack",
			ToolID:         "httpx",
			AdapterID:      "cli.exec",
			Operation:      "http_probe",
			ParserID:       "httpx.jsonl",
			TargetKind:     "url",
			DependencyName: "httpx",
			DependencyKind: "binary",
		},
	})
	values := envValues(env)

	for key, want := range map[string]string{
		"OURO_TOOL_ID":         "httpx",
		"OURO_ADAPTER_ID":      "cli.exec",
		"OURO_OPERATION":       "http_probe",
		"OURO_PARSER_ID":       "httpx.jsonl",
		"OURO_TARGET_KIND":     "url",
		"OURO_DEPENDENCY_NAME": "httpx",
		"OURO_DEPENDENCY_KIND": "binary",
	} {
		if values[key] != want {
			t.Fatalf("expected %s=%q, got env %#v", key, want, values)
		}
	}
}

func envValues(env []string) map[string]string {
	values := map[string]string{}
	for _, entry := range env {
		for i, r := range entry {
			if r == '=' {
				values[entry[:i]] = entry[i+1:]
				break
			}
		}
	}
	return values
}
