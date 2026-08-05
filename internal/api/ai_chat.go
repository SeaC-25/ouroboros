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
	ID        string             `json:"id"`
	Role      string             `json:"role"` // user / assistant
	Content   string             `json:"content"`
	Timestamp int64              `json:"timestamp"`
	Refs      []string           `json:"refs,omitempty"`
	Actions   []ChatActionResult `json:"actions,omitempty"`
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

// ChatActionResult 动作执行后的结构化结果
type ChatActionResult struct {
	Kind    string `json:"kind"`
	Summary string `json:"summary"`
	Path    string `json:"path,omitempty"`
	Output  string `json:"output,omitempty"`
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
- 列出文件：使用 ACTION:LIST <目录> 格式
- 调用工具动作：使用 ACTION:TOOL <动作ID> <参数> 格式
- 每个动作占单独一行，动作本身必须放在回复末尾。
- 动作执行结果会自动追加显示，不需要在文字里猜测文件内容或命令输出。

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
	refs, actionResults := s.parseAndExecuteAIActions(result.Text)

	// 持久化 AI 回复
	assistantMsg := ChatMessage{
		ID:        fmt.Sprintf("msg-%d", time.Now().Unix()),
		Role:      "assistant",
		Content:   result.Text,
		Timestamp: time.Now().Unix(),
		Refs:      refs,
		Actions:   actionResults,
	}
	s.appendAIConversation(assistantMsg)

	respondJSON(w, http.StatusOK, ChatPostResponse{
		Message: assistantMsg,
		Context: refs,
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
		actions := ProjectToolchainActions(toolchain)
		if len(actions) > 0 {
			toolList := make([]string, 0, len(actions))
			for _, action := range actions {
				toolList = append(toolList, fmt.Sprintf("  - %s (%s, %s)", action.ID, action.Label, action.PackName))
			}
			summary = append(summary, fmt.Sprintf("可用工具动作 (%d 个):\n%s", len(actions), strings.Join(toolList, "\n")))
		} else {
			summary = append(summary, "可用工具动作: 0 个")
		}
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
func (s *Server) parseAndExecuteAIActions(text string) ([]string, []ChatActionResult) {
	var refs []string
	var results []ChatActionResult
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
				if err != nil {
					results = append(results, ChatActionResult{Kind: "execute", Summary: "启动失败: " + err.Error()})
					continue
				}
				refs = append(refs, fmt.Sprintf("executed:%s", execution.ID))
				// 给后台命令一点时间写出日志，然后回填已捕获输出
				time.Sleep(800 * time.Millisecond)
				output := ""
				if logs, logErr := ReadExecutionLogs(s.workspace, execution.ID); logErr == nil {
					output = truncateForAI(logs.Content, 4000)
				}
				results = append(results, ChatActionResult{
					Kind:    "execute",
					Summary: fmt.Sprintf("已执行命令，执行记录 %s", execution.ID),
					Path:    execution.LogPath,
					Output:  output,
				})
			}
		} else if strings.HasPrefix(line, "ACTION:LIST ") {
			dir := strings.TrimPrefix(line, "ACTION:LIST ")
			dir = strings.TrimSpace(dir)
			if dir == "." {
				dir = ""
			}
			files, err := ProjectWorkspaceFiles(s.workspace)
			if err == nil && len(files) > 0 {
				listed := []string{}
				for _, f := range files {
					if dir == "" || f.Path == dir || strings.HasPrefix(f.Path, strings.TrimSuffix(dir, "/")+"/") {
						listed = append(listed, f.Path)
					}
				}
				if len(listed) > 0 {
					refs = append(refs, fmt.Sprintf("listed:%d", len(listed)))
					results = append(results, ChatActionResult{
						Kind:    "list",
						Summary: fmt.Sprintf("列出 %d 个工作区文件", len(listed)),
						Path:    dir,
						Output:  strings.Join(listed, "\n"),
					})
				}
			}
		} else if strings.HasPrefix(line, "ACTION:READ ") {
			filePath := strings.TrimPrefix(line, "ACTION:READ ")
			filePath = strings.TrimSpace(filePath)
			filePath = strings.TrimPrefix(filePath, "./")
			if filePath != "" {
				file, err := ReadWorkspaceFile(s.workspace, filePath)
				if err != nil {
					results = append(results, ChatActionResult{Kind: "read", Summary: "读取失败: " + err.Error(), Path: filePath})
					continue
				}
				refs = append(refs, fmt.Sprintf("read:%s", filePath))
				results = append(results, ChatActionResult{
					Kind:    "read",
					Summary: fmt.Sprintf("已读取 %s（%d 字节）", filePath, file.Size),
					Path:    filePath,
					Output:  truncateForAI(file.Content, 4000),
				})
			}
		} else if strings.HasPrefix(line, "ACTION:WRITE ") {
			parts := strings.SplitN(strings.TrimPrefix(line, "ACTION:WRITE "), " ", 2)
			if len(parts) == 2 {
				filePath := strings.TrimSpace(parts[0])
				fileContent := parts[1]
				if filePath != "" {
					if _, err := WriteWorkspaceFile(s.workspace, filePath, fileContent); err != nil {
						results = append(results, ChatActionResult{Kind: "write", Summary: "写入失败: " + err.Error(), Path: filePath})
						continue
					}
					refs = append(refs, fmt.Sprintf("written:%s", filePath))
					results = append(results, ChatActionResult{
						Kind:    "write",
						Summary: "已写入文件",
						Path:    filePath,
						Output:  truncateForAI(fileContent, 2000),
					})
				}
			}
		} else if strings.HasPrefix(line, "ACTION:TOOL ") {
			toolSpec := strings.TrimPrefix(line, "ACTION:TOOL ")
			toolSpec = strings.TrimSpace(toolSpec)
			parts := strings.SplitN(toolSpec, " ", 2)
			if len(parts) > 0 {
				actionID := strings.TrimSpace(parts[0])
				arguments := ""
				if len(parts) == 2 {
					arguments = strings.TrimSpace(parts[1])
				}
				if actionID != "" {
					response, err := s.StartToolchainAction(actionID, ToolchainActionRunRequestDTO{Arguments: arguments})
					if err != nil {
						results = append(results, ChatActionResult{Kind: "tool", Summary: "工具动作失败: " + err.Error(), Path: actionID})
						continue
					}
					refs = append(refs, fmt.Sprintf("tool:%s:%s", actionID, response.Execution.ID))
					time.Sleep(800 * time.Millisecond)
					output := ""
					if logs, logErr := ReadExecutionLogs(s.workspace, response.Execution.ID); logErr == nil {
						output = truncateForAI(logs.Content, 4000)
					}
					results = append(results, ChatActionResult{
						Kind:    "tool",
						Summary: fmt.Sprintf("已启动工具动作 %s", response.Action.Label),
						Path:    response.Execution.LogPath,
						Output:  output,
					})
				}
			}
		}
	}
	return refs, results
}

func truncateForAI(value string, max int) string {
	value = strings.TrimSpace(value)
	if len(value) <= max {
		return value
	}
	return value[:max] + "\n...[已截断]"
}
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
