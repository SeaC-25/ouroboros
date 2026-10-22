package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ouroboros/ouroboros/internal/compiler"
	"github.com/ouroboros/ouroboros/internal/eventlog"
	"github.com/ouroboros/ouroboros/internal/toolchain"
)

// TestNativeGoHTTPProbeProducesParserConsumableEvents proves the native_go
// adapter is a drop-in replacement for the managed httpx binary at the output
// contract level: the in-process handler's transcript must feed the existing
// httpx parser and yield web surface facts.
func TestNativeGoHTTPProbeProducesParserConsumableEvents(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<html><title>Native Probe</title></html>"))
	}))
	defer server.Close()

	root := t.TempDir()
	request := ToolExecutionRequest{
		RunID:         "run-native",
		WorkspaceRoot: root,
		Node: compiler.PlanNode{
			ID:         "node-native-http",
			ToolID:     "httpx",
			AdapterID:  nativeGoAdapterID,
			Operation:  "http_probe",
			ParserID:   "httpx_jsonl",
			Runner:     "native",
			TargetKind: "url",
		},
		ScopeTargets: []ScopeTarget{{Kind: "url", Value: server.URL}},
	}

	result, runnable, err := executeNativeGoTool(context.Background(), request)
	if err != nil {
		t.Fatalf("executeNativeGoTool: %v", err)
	}
	if !runnable {
		t.Fatal("native node with an in-scope url target must be runnable")
	}
	if result.Status != "completed" {
		t.Fatalf("status = %q, transcript = %s", result.Status, result.Transcript)
	}
	if !strings.Contains(result.Transcript, `"status_code":200`) {
		t.Fatalf("transcript missing probe result: %s", result.Transcript)
	}

	events := parserEventsForToolExecution("run-native", request, result)
	if len(events) == 0 {
		t.Fatal("native transcript produced no parser events")
	}
	// The parser emits a generic "fact" event whose payload carries the node
	// type; the native probe must yield a web surface like the managed httpx.
	if !eventsContainPayload(events, "surface") {
		t.Fatalf("expected a surface fact from the native probe, got %#v", eventKinds(events))
	}
}

func TestNativeGoUnknownAdapterFallsThrough(t *testing.T) {
	request := ToolExecutionRequest{
		Node: compiler.PlanNode{
			ID:        "node-managed",
			AdapterID: "managed_binary",
			Operation: "http_probe",
			Runner:    "native",
		},
	}
	if _, ok := nativeGoToolNode(request); ok {
		t.Fatal("managed_binary node must not be claimed by the native path")
	}
}

func TestNativeGoRejectsNodeWithoutInScopeTarget(t *testing.T) {
	request := ToolExecutionRequest{
		RunID:         "run-native",
		WorkspaceRoot: t.TempDir(),
		Node: compiler.PlanNode{
			ID:        "node-native-http",
			AdapterID: nativeGoAdapterID,
			Operation: "http_probe",
			Runner:    "native",
		},
		ScopeTargets: []ScopeTarget{{Kind: "url", Value: ""}},
	}
	if _, runnable, err := executeNativeGoTool(context.Background(), request); err != nil || runnable {
		t.Fatalf("node without an in-scope target must be non-runnable (runnable=%v err=%v)", runnable, err)
	}
}

func TestNativeGoAdapterContractMatchesManagedBinary(t *testing.T) {
	registry := toolchain.DefaultAuthoredToolRegistry()
	managed, ok := registry.AdapterOperation("managed_binary", "http_probe")
	if !ok {
		t.Fatal("managed_binary http_probe missing")
	}
	native, ok := registry.AdapterOperation(nativeGoAdapterID, "http_probe")
	if !ok {
		t.Fatal("native_go http_probe missing")
	}
	if len(native.SupportedParsers) != len(managed.SupportedParsers) {
		t.Fatalf("native parsers %v != managed parsers %v", native.SupportedParsers, managed.SupportedParsers)
	}
	// Dependency names stay recorded so the catalog contract (a tool's declared
	// dependency must be allowed by its operation) holds and the managed bundle
	// remains installable as a fallback; the native handler ignores them.
	if len(native.DependencyNames) != len(managed.DependencyNames) {
		t.Fatalf("native dependencies %v != managed dependencies %v", native.DependencyNames, managed.DependencyNames)
	}
}

// eventsContainPayload reports whether any event payload mentions needle.
func eventsContainPayload(events []eventlog.Event, needle string) bool {
	for _, event := range events {
		if strings.Contains(string(event.Payload), needle) {
			return true
		}
	}
	return false
}

// eventKinds lists event types plus payloads for failure diagnostics.
func eventKinds(events []eventlog.Event) []string {
	kinds := make([]string, 0, len(events))
	for _, event := range events {
		kinds = append(kinds, string(event.Type))
	}
	return kinds
}
