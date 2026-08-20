package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/ouroboros/ouroboros/internal/eventlog"
	"github.com/ouroboros/ouroboros/internal/toolchain"
)

func TestResolveCatalogToolInvocationSupportsEveryBinaryCatalogEntry(t *testing.T) {
	root := catalogToolTestWorkspace(t)
	artifact := filepath.Join(root, "artifacts", "sample.apk")
	if err := os.WriteFile(artifact, []byte("fixture\n"), 0o644); err != nil {
		t.Fatalf("write artifact: %v", err)
	}

	binaryCount := 0
	for _, tool := range toolchain.BuiltInAppSecTools() {
		if tool.DependencyKind != "binary" {
			continue
		}
		binaryCount++
		catalogToolTestExecutable(t, root, tool.Dependency)
		t.Run(tool.Name, func(t *testing.T) {
			target := catalogToolTestTarget(tool.TargetKind)
			result, err := ResolveCatalogToolInvocation(CatalogToolInvocation{
				WorkspaceRoot: root,
				ToolID:        tool.Name,
				Operation:     tool.Operation,
				Target:        target,
				TargetKind:    tool.TargetKind,
				RunID:         "run-catalog",
				ExecutionID:   "exec-" + strings.ReplaceAll(tool.Name, ".", "-"),
			})
			if err != nil {
				t.Fatalf("resolve catalog invocation: %v", err)
			}
			wantExecutable := filepath.Join(root, "toolchain", "bin", tool.Dependency)
			if result.Executable != wantExecutable {
				t.Fatalf("expected workspace executable %q, got %q", wantExecutable, result.Executable)
			}
			if len(result.Argv) == 0 || result.Argv[0] != result.Executable {
				t.Fatalf("argv must start with resolved executable, got %#v", result.Argv)
			}
			if !slices.Equal(result.Args, result.Argv[1:]) {
				t.Fatalf("args and argv disagree: args=%#v argv=%#v", result.Args, result.Argv)
			}
			if result.ToolID != tool.Name || result.PackID != tool.Pack || result.Operation != tool.Operation || result.ParserID != tool.Parser {
				t.Fatalf("unstable catalog context: %#v", result)
			}
			if result.Target != target || result.TargetKind != tool.TargetKind || result.RunID != "run-catalog" {
				t.Fatalf("unstable target/execution context: %#v", result)
			}
			if tool.TargetKind == "artifact" && !filepath.IsAbs(result.ResolvedTarget) {
				t.Fatalf("artifact target must resolve to an absolute workspace path: %#v", result)
			}
		})
	}
	if binaryCount != 25 {
		t.Fatalf("expected 25 executable catalog entries, got %d", binaryCount)
	}
}

func TestResolveCatalogToolInvocationExposesManagedOutputs(t *testing.T) {
	root := catalogToolTestWorkspace(t)
	if err := os.WriteFile(filepath.Join(root, "artifacts", "sample.apk"), []byte("fixture\n"), 0o644); err != nil {
		t.Fatalf("write artifact: %v", err)
	}

	tests := []struct {
		toolID       string
		operation    string
		parserID     string
		wantFileExt  string
		wantDirCount int
	}{
		{toolID: "testssl.sh", operation: "tls_assessment", parserID: "testssl_json", wantFileExt: ".json"},
		{toolID: "sslscan", operation: "tls_assessment", parserID: "sslscan_xml", wantFileExt: ".xml"},
		{toolID: "apktool", operation: "apk_decompile", parserID: "apktool_text", wantDirCount: 1},
		{toolID: "jadx", operation: "apk_decompile", parserID: "jadx_metadata_json", wantDirCount: 1},
		{toolID: "semgrep", operation: "static_code_scan", parserID: "semgrep_sarif", wantFileExt: ".sarif"},
		{toolID: "gitleaks", operation: "secret_scan", parserID: "gitleaks_json", wantFileExt: ".json"},
	}
	for _, tt := range tests {
		catalogToolTestExecutable(t, root, tt.toolID)
		t.Run(tt.toolID, func(t *testing.T) {
			targetKind := "artifact"
			target := "sample.apk"
			if tt.operation == "tls_assessment" {
				targetKind = "host"
				target = "tls.example.test"
			}
			result, err := ResolveCatalogToolInvocation(CatalogToolInvocation{
				WorkspaceRoot: root,
				ToolID:        tt.toolID,
				Operation:     tt.operation,
				Target:        target,
				TargetKind:    targetKind,
				ExecutionID:   "exec-output-" + strings.ReplaceAll(tt.toolID, ".", "-"),
			})
			if err != nil {
				t.Fatalf("resolve catalog invocation: %v", err)
			}
			if result.ParserID != tt.parserID {
				t.Fatalf("expected parser %q, got %#v", tt.parserID, result)
			}
			if tt.wantFileExt != "" {
				if len(result.ManagedOutputFiles) != 1 || filepath.Ext(result.ManagedOutputFiles[0]) != tt.wantFileExt {
					t.Fatalf("expected one managed %s output, got %#v", tt.wantFileExt, result.ManagedOutputFiles)
				}
			}
			if len(result.ManagedOutputDirs) != tt.wantDirCount {
				t.Fatalf("expected %d managed output directories, got %#v", tt.wantDirCount, result.ManagedOutputDirs)
			}
		})
	}
}

func TestResolveCatalogToolInvocationHonorsExplicitTargetKinds(t *testing.T) {
	root := catalogToolTestWorkspace(t)
	if err := os.WriteFile(filepath.Join(root, "artifacts", "sample.apk"), []byte("fixture\n"), 0o644); err != nil {
		t.Fatalf("write artifact: %v", err)
	}
	for _, name := range []string{"naabu", "nmap", "testssl.sh", "mobsfscan"} {
		catalogToolTestExecutable(t, root, name)
	}

	tests := []struct {
		name       string
		toolID     string
		operation  string
		target     string
		targetKind string
	}{
		{name: "naabu host", toolID: "naabu", operation: "port_scan", target: "server.example.test", targetKind: "host"},
		{name: "naabu cidr", toolID: "naabu", operation: "port_scan", target: "192.0.2.0/24", targetKind: "cidr"},
		{name: "nmap host", toolID: "nmap", operation: "service_fingerprint", target: "server.example.test", targetKind: "host"},
		{name: "nmap cidr", toolID: "nmap", operation: "service_fingerprint", target: "192.0.2.0/24", targetKind: "cidr"},
		{name: "tls domain", toolID: "testssl.sh", operation: "tls_assessment", target: "tls.example.test", targetKind: "domain"},
		{name: "artifact", toolID: "mobsfscan", operation: "static_code_scan", target: "sample.apk", targetKind: "artifact"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ResolveCatalogToolInvocation(CatalogToolInvocation{
				WorkspaceRoot: root,
				ToolID:        tt.toolID,
				Operation:     tt.operation,
				Target:        tt.target,
				TargetKind:    tt.targetKind,
			})
			if err != nil {
				t.Fatalf("resolve catalog invocation: %v", err)
			}
			if result.TargetKind != tt.targetKind {
				t.Fatalf("caller target kind was replaced: got %q want %q", result.TargetKind, tt.targetKind)
			}
			if tt.targetKind == "artifact" {
				if result.ResolvedTarget != filepath.Join(root, "artifacts", "sample.apk") {
					t.Fatalf("artifact did not resolve under workspace: %#v", result)
				}
			}
		})
	}
}

func TestResolveCatalogToolInvocationResolutionAndArgumentBoundaries(t *testing.T) {
	skipIfWindows(t)
	t.Run("workspace binary wins over PATH and args stay atomic", func(t *testing.T) {
		root := catalogToolTestWorkspace(t)
		workspaceExecutable := catalogToolTestExecutable(t, root, "httpx")
		hostBin := t.TempDir()
		catalogToolTestHostExecutable(t, hostBin, "httpx")
		t.Setenv("PATH", hostBin)

		extra := []string{"--header", "X-Name: value with spaces", "literal*pattern"}
		result, err := ResolveCatalogToolInvocation(CatalogToolInvocation{
			WorkspaceRoot: root,
			ToolID:        "httpx",
			Operation:     "http_probe",
			Target:        "https://app.example.test",
			TargetKind:    "url",
			Args:          extra,
		})
		if err != nil {
			t.Fatalf("resolve catalog invocation: %v", err)
		}
		if result.Executable != workspaceExecutable {
			t.Fatalf("workspace executable must win, got %q", result.Executable)
		}
		if !slices.Equal(result.Args[len(result.Args)-len(extra):], extra) {
			t.Fatalf("caller arguments lost boundaries: got %#v want suffix %#v", result.Args, extra)
		}
	})

	t.Run("PATH is used when workspace binary is absent", func(t *testing.T) {
		root := catalogToolTestWorkspace(t)
		hostBin := t.TempDir()
		hostExecutable := catalogToolTestHostExecutable(t, hostBin, "httpx")
		t.Setenv("PATH", hostBin)

		result, err := ResolveCatalogToolInvocation(CatalogToolInvocation{
			WorkspaceRoot: root,
			ToolID:        "httpx",
			Operation:     "http_probe",
			Target:        "https://app.example.test",
			TargetKind:    "url",
		})
		if err != nil {
			t.Fatalf("resolve catalog invocation from PATH: %v", err)
		}
		if result.Executable != hostExecutable {
			t.Fatalf("expected PATH executable %q, got %q", hostExecutable, result.Executable)
		}
	})

	t.Run("missing binary is an invocation error", func(t *testing.T) {
		root := catalogToolTestWorkspace(t)
		t.Setenv("PATH", t.TempDir())
		_, err := ResolveCatalogToolInvocation(CatalogToolInvocation{
			WorkspaceRoot: root,
			ToolID:        "httpx",
			Operation:     "http_probe",
			Target:        "https://app.example.test",
			TargetKind:    "url",
		})
		if err == nil || !strings.Contains(err.Error(), "httpx") {
			t.Fatalf("expected missing httpx invocation error, got %v", err)
		}
	})
}

func TestResolveCatalogToolInvocationTreatsNucleiTemplatesAsOptional(t *testing.T) {
	root := catalogToolTestWorkspace(t)
	catalogToolTestExecutable(t, root, "nuclei")

	customArgs := []string{"-t", "custom templates/http checks"}
	result, err := ResolveCatalogToolInvocation(CatalogToolInvocation{
		WorkspaceRoot: root,
		ToolID:        "nuclei",
		Operation:     "template_scan",
		Target:        "https://app.example.test",
		TargetKind:    "url",
		Args:          customArgs,
	})
	if err != nil {
		t.Fatalf("resolve nuclei without managed templates: %v", err)
	}
	if !slices.Equal(result.Args[len(result.Args)-len(customArgs):], customArgs) {
		t.Fatalf("custom nuclei template args lost boundaries: %#v", result.Args)
	}
	if slices.Contains(result.Args[:len(result.Args)-len(customArgs)], "-t") {
		t.Fatalf("missing workspace bundle must not inject a template path: %#v", result.Args)
	}

	rulesPath := filepath.Join(root, "toolchain", "rules", "nuclei-templates")
	if err := os.MkdirAll(rulesPath, 0o755); err != nil {
		t.Fatalf("mkdir nuclei templates: %v", err)
	}
	result, err = ResolveCatalogToolInvocation(CatalogToolInvocation{
		WorkspaceRoot: root,
		ToolID:        "nuclei",
		Operation:     "template_scan",
		Target:        "https://app.example.test",
		TargetKind:    "url",
	})
	if err != nil {
		t.Fatalf("resolve nuclei with managed templates: %v", err)
	}
	if !catalogToolTestContainsPair(result.Args, "-t", rulesPath) {
		t.Fatalf("installed workspace bundle must be attached automatically: %#v", result.Args)
	}

	_, err = ResolveCatalogToolInvocation(CatalogToolInvocation{
		WorkspaceRoot: root,
		ToolID:        "nuclei-templates",
		Operation:     "template_scan",
		Target:        "https://app.example.test",
		TargetKind:    "url",
	})
	if err == nil || !strings.Contains(err.Error(), "not executable") {
		t.Fatalf("rule-bundle catalog entry must never execute, got %v", err)
	}
}

func TestCatalogToolParserEventsReconstructsSpecializedDispatcherContext(t *testing.T) {
	root := catalogToolTestWorkspace(t)
	catalogToolTestExecutable(t, root, "httpx")
	invocation, err := ResolveCatalogToolInvocation(CatalogToolInvocation{
		WorkspaceRoot: root,
		ToolID:        "httpx",
		Operation:     "http_probe",
		Target:        "https://app.example.test",
		TargetKind:    "url",
		RunID:         "run-catalog-parser",
		ExecutionID:   "exec-catalog-parser",
	})
	if err != nil {
		t.Fatalf("resolve catalog invocation: %v", err)
	}
	events := CatalogToolParserEvents(invocation, ToolExecutionResult{
		ExecutionID: "exec-catalog-parser",
		Status:      "completed",
		Transcript:  `{"url":"https://app.example.test","status_code":200,"title":"Home"}`,
	})
	if len(events) < 2 || events[0].Type != eventlog.EventParser || events[1].Type != eventlog.EventFact {
		t.Fatalf("expected genuine parser and fact events, got %#v", events)
	}
	for _, event := range events {
		if event.RunID != "run-catalog-parser" {
			t.Fatalf("parser event lost run context: %#v", event)
		}
		var payload map[string]any
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			t.Fatalf("decode parser event: %v", err)
		}
		if payload["tool_id"] != "httpx" || payload["operation"] != "http_probe" || payload["execution_id"] != "exec-catalog-parser" {
			t.Fatalf("parser event lost invocation context: %#v", payload)
		}
	}
}

func catalogToolTestWorkspace(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for _, dir := range []string{"artifacts", filepath.Join("toolchain", "bin"), filepath.Join("toolchain", "rules")} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}
	return root
}

func catalogToolTestExecutable(t *testing.T, root string, name string) string {
	t.Helper()
	return catalogToolTestHostExecutable(t, filepath.Join(root, "toolchain", "bin"), name)
}

func catalogToolTestHostExecutable(t *testing.T, dir string, name string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write executable %s: %v", name, err)
	}
	return path
}

func catalogToolTestTarget(kind string) string {
	switch kind {
	case "url":
		return "https://app.example.test"
	case "domain":
		return "example.test"
	case "host":
		return "server.example.test"
	case "artifact":
		return "sample.apk"
	default:
		return "target.example.test"
	}
}

func catalogToolTestContainsPair(values []string, first string, second string) bool {
	for index := 0; index+1 < len(values); index++ {
		if values[index] == first && values[index+1] == second {
			return true
		}
	}
	return false
}
