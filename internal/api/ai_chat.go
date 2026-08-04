package api

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ouroboros/ouroboros/internal/app"
)

// ChatMessage 持久化的对话消息
type ChatMessage struct {
	ID        string `json:"id"`
	Role      string `json:"role"` // user / assistant
	Content   string `json:"content"`
	Timestamp int64  `json:"timestamp"`
	Refs      []string `json:"refs,omitempty"`
}

// ChatResponse 前端期望的返回结构
type ChatResponse struct {
	Messages   []ChatMessage `json:"messages"`
	Workspace  string        `json:"workspace"`
	Summary    string        `json:"summary"`
}

// ChatPostRequest 前端发送的消息
type ChatPostRequest struct {
	Message string `json:"message"`
}

// ChatPostResponse AI 回复
type ChatPostResponse struct {
	Message   ChatMessage `json:"message"`
	Context   []string    `json:"context,omitempty"`
}

func (s *Server) handleAIChat(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.handleAIChatGet(w, r)
	case http.MethodPost:
		s.handleAIChatPost(w, r)
	default:
		respondError(w, http.StatusMethodNotAllowed, "不支持的对话方法")
	}
}

// handleAIChatGet 获取对话历史和工作区摘要
func (s *Server) handleAIChatGet(w http.ResponseWriter, r *http.Request) {
	messages := s.loadAIConversation()
	summary := s.buildWorkspaceSummary()
	respondJSON(w, http.StatusOK, ChatResponse{
		Messages:  messages,
		Workspace: s.workspace,
		Summary:   summary,
	})
}

// handleAIChatPost 发送消息并获取 AI 回复
func (s *Server) handleAIChatPost(w http.ResponseWriter, r *http.Request) {
	var request ChatPostRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		respondError(w, http.StatusBadRequest, fmt.Sprintf("解析消息失败: %v", err))
		return
	}

	message := strings.TrimSpace(request.Message)
	if message == "" {
		respondError(w, http.StatusBadRequest, "消息不能为空")
		return
	}

	// 持久化用户消息
	now := time.Now().Unix()
	userMsg := ChatMessage{
		ID:        fmt.Sprintf("msg-%d", now),
		Role:      "user",
		Content:   message,
		Timestamp: now,
	}
	s.appendAIConversation(userMsg)

	// 构建系统提示，包含工作区上下文
	systemPrompt := fmt.Sprintf(`你是 Ouroboros IDE 的 AI 安全操作员。你帮助用户理解工作区、修改文件、运行任务、调用工具、调度多智能体、使用黑板算法、读取结果、保存证据、评测 prompt、运行 harness、回放过程、查看报告、管理配置，并持续推进真实安全工作。

工作区摘要:
%s

你可以：
- 回答用户关于工作区状态的问题
- 执行命令：使用 ACTION:EXECUTE <命令> 格式
- 读取文件：使用 ACTION:READ <文件路径> 格式
- 写入文件：使用 ACTION:WRITE <文件路径> 格式，后跟文件内容
- 列出文件：使用 ACTION:LIST 目录

重要：所有回复使用中文。`, s.buildWorkspaceSummary())

	// 调用真实 provider
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()

	result, err := app.GenerateProviderMessage(ctx, s.workspace, app.ProviderGenerationRequest{
		SystemPrompt:         systemPrompt,
		UserPrompt:           message,
		MaxTokens:            2048,
		RequiredCapabilities: []string{"chat"},
		Stream:               false,
	})
	if err != nil {
		log.Printf("AI chat generation failed: %v", err)
		// 回退到错误提示
		assistantMsg := ChatMessage{
			ID:        fmt.Sprintf("msg-%d", time.Now().Unix()),
			Role:      "assistant",
			Content:   fmt.Sprintf("AI 模型调用失败: %v。请检查 provider 配置。", err),
			Timestamp: time.Now().Unix(),
		}
		s.appendAIConversation(assistantMsg)
		respondJSON(w, http.StatusOK, ChatPostResponse{
			Message: assistantMsg,
			Context: []string{"provider_error"},
		})
		return
	}

	// 检查 AI 回复中是否有动作标记并执行
	actions := s.parseAndExecuteAIActions(result.Text)

	// 持久化 AI 回复
	assistantMsg := ChatMessage{
		ID:        fmt.Sprintf("msg-%d", time.Now().Unix()),
		Role:      "assistant",
		Content:   result.Text,
		Timestamp: time.Now().Unix(),
		Refs:      actions,
	}
	s.appendAIConversation(assistantMsg)

	respondJSON(w, http.StatusOK, ChatPostResponse{
		Message: assistantMsg,
		Context: actions,
	})
}

// buildWorkspaceSummary 构建工作区摘要给 AI
func (s *Server) buildWorkspaceSummary() string {
	var summary []string

	// Mission
	mission, err := ProjectMission(s.workspace)
	if err == nil {
		summary = append(summary, fmt.Sprintf("任务: %s (目标: %s, 状态: %s)", mission.Name, mission.Goal, mission.Status))
	}

	// Provider
	provider, err := ProjectProvider(s.workspace)
	if err == nil {
		summary = append(summary, fmt.Sprintf("模型: %s (Provider: %s, 状态: %s)", provider.Model, provider.Name, provider.Status))
	}

	// 文件列表
	files, err := ProjectWorkspaceFiles(s.workspace)
	if err == nil && len(files) > 0 {
		fileList := make([]string, 0, min(len(files), 20))
		for _, f := range files {
			fileList = append(fileList, fmt.Sprintf("  - %s (%s)", f.Path, f.Kind))
		}
		summary = append(summary, fmt.Sprintf("文件 (%d 个):\n%s", len(files), strings.Join(fileList, "\n")))
	}

	// 执行记录
	executions, err := ProjectExecutions(s.workspace)
	if err == nil && len(executions) > 0 {
		execList := make([]string, 0, min(len(executions), 10))
		for _, e := range executions {
			execList = append(execList, fmt.Sprintf("  - %s: %s (状态: %s)", e.ID, e.Command, e.Status))
		}
		summary = append(summary, fmt.Sprintf("执行记录 (%d 个):\n%s", len(executions), strings.Join(execList, "\n")))
	}

	// 工具链
	toolchain, err := ProjectToolchain(s.workspace)
	if err == nil {
		summary = append(summary, fmt.Sprintf("工具链: %s (工具就绪: %d)", toolchain.Status, toolchain.ToolsReady))
	}

	if len(summary) == 0 {
		return "工作区为空或尚未初始化"
	}
	return strings.Join(summary, "\n")
}

// loadAIConversation 加载持久化的对话历史
func (s *Server) loadAIConversation() []ChatMessage {
	path := filepath.Join(s.workspace, "runtime", "ai-conversation.jsonl")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return []ChatMessage{}
	}

	file, err := os.Open(path)
	if err != nil {
		return []ChatMessage{}
	}
	defer file.Close()

	var messages []ChatMessage
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024) // 1MB buffer
	for scanner.Scan() {
		var msg ChatMessage
		if err := json.Unmarshal(scanner.Bytes(), &msg); err != nil {
			continue
		}
		messages = append(messages, msg)
	}
	return messages
}

// appendAIConversation 追加消息到对话历史
func (s *Server) appendAIConversation(msg ChatMessage) {
	runtimeDir := filepath.Join(s.workspace, "runtime")
	os.MkdirAll(runtimeDir, 0o755)

	path := filepath.Join(runtimeDir, "ai-conversation.jsonl")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		log.Printf("Failed to append AI conversation: %v", err)
		return
	}
	defer file.Close()

	data, err := json.Marshal(msg)
	if err != nil {
		log.Printf("Failed to marshal AI message: %v", err)
		return
	}
	file.Write(data)
	file.WriteString("\n")
}

// parseAndExecuteAIActions 解析并执行 AI 回复中的动作标记
func (s *Server) parseAndExecuteAIActions(text string) []string {
	var refs []string
	lines := strings.Split(text, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "ACTION:EXECUTE ") {
			cmd := strings.TrimPrefix(line, "ACTION:EXECUTE ")
			cmd = strings.TrimSpace(cmd)
			if cmd != "" {
				execution, err := s.StartExecution(ExecutionCreateRequestDTO{
					Command: "bash",
					Args:    []string{"-c", cmd},
				})
				if err == nil {
					refs = append(refs, fmt.Sprintf("executed:%s", execution.ID))
				}
			}
		} else if strings.HasPrefix(line, "ACTION:READ ") {
			filePath := strings.TrimPrefix(line, "ACTION:READ ")
			filePath = strings.TrimSpace(filePath)
			if filePath != "" {
				if _, err := ReadWorkspaceFile(s.workspace, filePath); err == nil {
					refs = append(refs, fmt.Sprintf("read:%s", filePath))
				}
			}
		} else if strings.HasPrefix(line, "ACTION:WRITE ") {
			parts := strings.SplitN(strings.TrimPrefix(line, "ACTION:WRITE "), " ", 2)
			if len(parts) == 2 {
				filePath := strings.TrimSpace(parts[0])
				content := parts[1]
				if filePath != "" {
					if _, err := WriteWorkspaceFile(s.workspace, filePath, content); err == nil {
						refs = append(refs, fmt.Sprintf("written:%s", filePath))
					}
				}
			}
		}
	}
	return refs
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
