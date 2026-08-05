package api

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseAndExecuteAIActionsRealWorkspaceFiles(t *testing.T) {
	workspace := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspace, "tmp"), 0o755); err != nil {
		t.Fatalf("mkdir tmp: %v", err)
	}
	mustWrite(t, filepath.Join(workspace, "notes.md"), "核心\n密钥\n")
	mustWrite(t, filepath.Join(workspace, "tmp", "original.txt"), "original\n")

	server := &Server{workspace: workspace}
	refs, results := server.parseAndExecuteAIActions(strings.Join([]string{
		"ACTION:WRITE runtime/ai-test/new-file.txt hello",
		"ACTION:SEARCH 核心",
		"ACTION:RENAME tmp/original.txt tmp/renamed.txt",
		"ACTION:DELETE tmp/renamed.txt",
	}, "\n"))

	if !hasChatActionResult(results, "write") || !hasChatActionResult(results, "search") || !hasChatActionResult(results, "rename") || !hasChatActionResult(results, "delete") {
		t.Fatalf("expected write/search/rename/delete results, got %#v", results)
	}
	if len(refs) != 4 {
		t.Fatalf("expected 4 refs, got %#v", refs)
	}
	data, err := os.ReadFile(filepath.Join(workspace, "runtime", "ai-test", "new-file.txt"))
	if err != nil || string(data) != "hello" {
		t.Fatalf("expected created workspace file, data=%q err=%v", string(data), err)
	}
	if _, err := os.Stat(filepath.Join(workspace, "tmp", "renamed.txt")); !os.IsNotExist(err) {
		t.Fatalf("expected renamed file to be deleted, stat err=%v", err)
	}
}

func hasChatActionResult(results []ChatActionResult, kind string) bool {
	for _, result := range results {
		if result.Kind == kind {
			return true
		}
	}
	return false
}
