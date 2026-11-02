package api

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/ouroboros/ouroboros/internal/eventlog"
)

func skipIfWindows(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("test relies on POSIX shell probe execution and is Linux-only")
	}
}

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

// TestAutonomyContractExplorationClauseInjected 验证系统提示词注入探索阶段
// 条款（契约第 7 条）：对一条诱导直接解题的用户消息，主聊天系统提示词必须
// 包含探索阶段关键词。只测提示词组装，不调用真实模型。
func TestAutonomyContractExplorationClauseInjected(t *testing.T) {
	workspace := t.TempDir()
	server := &Server{workspace: workspace}

	// 模拟主聊天路径的提示词组装：工作区摘要 + 诱导性消息上下文。
	// 真实路径在 handleAIChatPost：systemPrompt 尾部拼接 autonomyContract。
	systemPrompt := fmt.Sprintf(`你是 Ouroboros IDE 的 AI 安全操作员。

工作区摘要:
%s

用户消息: 你要先做什么？直接开始解题吗

重要：所有回复使用中文。`, server.buildWorkspaceSummary())
	full := injectAutonomyContract(systemPrompt)

	for _, keyword := range []string{"探索阶段", "关键路径已打通"} {
		if !strings.Contains(full, keyword) {
			t.Fatalf("system prompt missing exploration clause keyword %q", keyword)
		}
	}
	// 契约是架构级附加段：必须位于基础提示词之后，完整追加而非覆盖。
	if !strings.HasPrefix(full, systemPrompt) {
		t.Fatalf("autonomy contract must append to, not replace, the base system prompt")
	}
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

func TestRunAgentTaskStartsRealExecution(t *testing.T) {
	skipIfWindows(t)
	workspace := t.TempDir()
	task, err := (&Server{workspace: workspace}).createAgentTask("operator", "执行真实命令")
	if err != nil {
		t.Fatalf("create agent task: %v", err)
	}
	server := &Server{workspace: workspace}
	updated, execution, err := server.runAgentTask(task.ID, "printf agent-ok")
	if err != nil {
		t.Fatalf("run agent task: %v", err)
	}
	if updated.Status != "running" || updated.ExecutionID != execution.ID {
		t.Fatalf("unexpected task after run: %#v execution=%#v", updated, execution)
	}
	time.Sleep(800 * time.Millisecond)
	logs, err := ReadExecutionLogs(workspace, execution.ID)
	if err != nil {
		t.Fatalf("read execution logs: %v", err)
	}
	if !strings.Contains(logs.Content, "agent-ok") {
		t.Fatalf("expected real execution log, got %q", logs.Content)
	}
}

func TestParseReportGenerateSpec(t *testing.T) {
	title, goal := parseReportGenerateSpec("当前工作区状态报告; 汇总真实执行与黑板状态")
	if title != "当前工作区状态报告" || goal != "汇总真实执行与黑板状态" {
		t.Fatalf("unexpected report generate spec: title=%q goal=%q", title, goal)
	}
	title, goal = parseReportGenerateSpec("仅标题")
	if title != "仅标题" || goal != "" {
		t.Fatalf("unexpected bare report title: title=%q goal=%q", title, goal)
	}
}

func TestCleanGeneratedReportMarkdownStripsReasoningPreamble(t *testing.T) {
	input := "用户要求我生成报告，让我先分析工作区。\n\n## 摘要\n\n真实状态。\n"
	cleaned := cleanGeneratedReportMarkdown(input)
	if strings.HasPrefix(cleaned, "用户要求我") || !strings.HasPrefix(cleaned, "## 摘要") {
		t.Fatalf("expected preamble stripped, got %q", cleaned)
	}
}
