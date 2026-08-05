package api

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ouroboros/ouroboros/internal/eventlog"
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

func TestAppendAIConversationWritesReplayEvents(t *testing.T) {
	workspace := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspace, "replay"), 0o755); err != nil {
		t.Fatalf("mkdir replay: %v", err)
	}
	store, err := eventlog.OpenSQLite(filepath.Join(workspace, "replay", "events.db"))
	if err != nil {
		t.Fatalf("open event store: %v", err)
	}
	defer store.Close()

	server := &Server{workspace: workspace, eventStore: store}
	server.appendAIConversation(ChatMessage{ID: "msg-user", Role: "user", Content: "执行真实操作"})
	server.appendAIConversation(ChatMessage{ID: "msg-ai", Role: "assistant", Content: "已经开始", Actions: []ChatActionResult{{Kind: "list", Summary: "已列出文件"}}})

	events, err := store.Replay(context.Background(), "ai-chat")
	if err != nil {
		t.Fatalf("replay ai-chat: %v", err)
	}
	if len(events) != 3 {
		t.Fatalf("expected 3 replay events, got %d", len(events))
	}
	var payload map[string]any
	if err := json.Unmarshal(events[0].Payload, &payload); err != nil {
		t.Fatalf("decode first payload: %v", err)
	}
	if payload["kind"] != "ai_request" || payload["message_id"] != "msg-user" {
		t.Fatalf("unexpected first event payload: %#v", payload)
	}
	if events[1].Type != eventlog.EventFact || events[2].Type != eventlog.EventFact {
		t.Fatalf("expected assistant and action to be facts, got %s and %s", events[1].Type, events[2].Type)
	}
}
