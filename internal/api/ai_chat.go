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
- 创建子目标：使用 ACTION:GOAL <标题> 格式
- 标记子目标完成：使用 ACTION:GOAL-DONE <标题> 格式
- 记录黑板发现：使用 ACTION:BLACKBOARD <发现内容> 格式
- 保存 prompt：使用 ACTION:PROMPT-SAVE <名称> <内容> 格式
- 运行 harness：使用 ACTION:HARNESS <prompt名称> <输入> 格式
- 保存 markdown 报告：使用 ACTION:REPORT <文件相对路径> <Markdown正文> 格式（单行）
- 创建 agent 任务：使用 ACTION:AGENT <角色> <目标> 格式
- 完成 agent 任务：使用 ACTION:AGENT-DONE <任务ID> 格式
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

	// 有真实动作结果时，把结果回填给模型做一次后续推理，让 AI 真正基于输出继续工作
	result.Text, refs, actionResults = s.continueAfterAIActions(result.Text, systemPrompt, refs, actionResults)

	// 用户明确要求创建/推进子目标时，即使模型没输出动作，也创建真实目标文件
	actionResults = s.applyExplicitGoalFallback(message, actionResults, &refs)

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

	// 报告文件
	reports, _ := ProjectReports(s.workspace)
	if len(reports) > 0 {
		reportList := make([]string, 0, min(len(reports), 10))
		for _, rep := range reports {
			reportList = append(reportList, fmt.Sprintf("  - %s: %s (%s)", rep.ID, rep.Title, rep.Status))
		}
		summary = append(summary, fmt.Sprintf("报告 (%d 个):\n%s", len(reports), strings.Join(reportList, "\n")))
	}

	// Harness 结果摘要
	harnessResults := s.loadWorkspaceHarnessResults()
	if len(harnessResults) > 0 {
		resultList := make([]string, 0, min(len(harnessResults), 10))
		for _, result := range harnessResults {
			resultList = append(resultList, fmt.Sprintf("  - %s (prompt=%s, model=%s)", result.ID, result.PromptName, result.Model))
		}
		summary = append(summary, fmt.Sprintf("Harness 结果 (%d 个):\n%s", len(harnessResults), strings.Join(resultList, "\n")))
	}

	// Prompt 注册表
	prompts := s.loadWorkspacePrompts()
	if len(prompts) > 0 {
		promptList := make([]string, 0, min(len(prompts), 10))
		for _, prompt := range prompts {
			promptList = append(promptList, fmt.Sprintf("  - %s: %s", prompt.Name, truncateForAI(prompt.Content, 80)))
		}
		summary = append(summary, fmt.Sprintf("Prompt 注册表 (%d 个):\n%s", len(prompts), strings.Join(promptList, "\n")))
	}

	// Agent 任务
	agentTasks := s.loadAgentTasks()
	if len(agentTasks) > 0 {
		taskList := make([]string, 0, min(len(agentTasks), 15))
		for _, task := range agentTasks {
			taskList = append(taskList, fmt.Sprintf("  - %s: %s (%s, 状态: %s)", task.ID, task.Goal, task.Role, task.Status))
		}
		summary = append(summary, fmt.Sprintf("Agent 任务 (%d 个):\n%s", len(agentTasks), strings.Join(taskList, "\n")))
	}

	// 黑板事实
	blackboardFacts := s.loadBlackboardFacts()
	if len(blackboardFacts) > 0 {
		factList := make([]string, 0, min(len(blackboardFacts), 15))
		for _, fact := range blackboardFacts {
			factList = append(factList, fmt.Sprintf("  - [%s] %s (%s)", fact.Kind, fact.Statement, fact.Source))
		}
		summary = append(summary, fmt.Sprintf("黑板事实 (%d 个):\n%s", len(blackboardFacts), strings.Join(factList, "\n")))
	}

	// 子目标
	goals := s.loadWorkspaceGoals()
	if len(goals) > 0 {
		goalList := make([]string, 0, len(goals))
		for _, goal := range goals {
			goalList = append(goalList, fmt.Sprintf("  - %s: %s (状态: %s)", goal.ID[:min(8, len(goal.ID))], goal.Title, goal.Status))
		}
		summary = append(summary, fmt.Sprintf("子目标 (%d 个):\n%s", len(goals), strings.Join(goalList, "\n")))
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
		} else if strings.HasPrefix(line, "ACTION:REPORT ") {
			reportSpec := strings.TrimPrefix(line, "ACTION:REPORT ")
			parts := strings.SplitN(strings.TrimSpace(reportSpec), " ", 2)
			if len(parts) == 2 {
				filePath := strings.TrimSpace(parts[0])
				body := strings.TrimSpace(parts[1])
				if filePath != "" {
					if err := s.writeWorkspaceFile(filePath, body); err != nil {
						results = append(results, ChatActionResult{Kind: "report", Summary: "报告写入失败: " + err.Error(), Path: filePath})
					} else {
						refs = append(refs, fmt.Sprintf("report:%s", filePath))
						results = append(results, ChatActionResult{Kind: "report", Summary: "已写入报告", Path: filePath, Output: truncateForAI(body, 2000)})
					}
				}
			}
		} else if strings.HasPrefix(line, "ACTION:BLACKBOARD ") {
			statement := strings.TrimPrefix(line, "ACTION:BLACKBOARD ")
			statement = strings.TrimSpace(statement)
			if statement != "" {
				fact, err := s.appendBlackboardFact("ai", statement)
				if err != nil {
					results = append(results, ChatActionResult{Kind: "blackboard", Summary: "记录黑板发现失败: " + err.Error()})
				} else {
					refs = append(refs, fmt.Sprintf("blackboard:%s", fact.ID))
					results = append(results, ChatActionResult{Kind: "blackboard", Summary: "已记录黑板发现", Path: fact.ID, Output: truncateForAI(fact.Statement, 2000)})
				}
			}
		} else if strings.HasPrefix(line, "ACTION:PROMPT-SAVE ") {
			promptSpec := strings.TrimPrefix(line, "ACTION:PROMPT-SAVE ")
			parts := strings.SplitN(strings.TrimSpace(promptSpec), " ", 2)
			if len(parts) == 2 {
				name := strings.TrimSpace(parts[0])
				content := strings.TrimSpace(parts[1])
				if name != "" && content != "" {
					prompt, err := s.saveWorkspacePrompt(name, content)
					if err != nil {
						results = append(results, ChatActionResult{Kind: "prompt", Summary: "保存 prompt 失败: " + err.Error(), Path: name})
					} else {
						refs = append(refs, fmt.Sprintf("prompt:%s:%s", prompt.Name, prompt.ID))
						results = append(results, ChatActionResult{Kind: "prompt", Summary: "已保存 prompt", Path: prompt.ID, Output: truncateForAI(prompt.Content, 2000)})
					}
				}
			}
		} else if strings.HasPrefix(line, "ACTION:HARNESS ") {
			harnessSpec := strings.TrimPrefix(line, "ACTION:HARNESS ")
			parts := strings.SplitN(strings.TrimSpace(harnessSpec), " ", 2)
			if len(parts) == 2 {
				promptName := strings.TrimSpace(parts[0])
				inputText := strings.TrimSpace(parts[1])
				result, err := s.runWorkspaceHarness(promptName, inputText)
				if err != nil {
					results = append(results, ChatActionResult{Kind: "harness", Summary: "运行 harness 失败: " + err.Error(), Path: promptName})
				} else {
				refs = append(refs, fmt.Sprintf("harness:%s", result.ID))
				results = append(results, ChatActionResult{Kind: "harness", Summary: "Harness 已运行", Path: result.ID, Output: truncateForAI(result.Output, 3000)})
				}
			}
		} else if strings.HasPrefix(line, "ACTION:AGENT ") {
			spec := strings.TrimPrefix(line, "ACTION:AGENT ")
			parts := strings.SplitN(strings.TrimSpace(spec), " ", 2)
			if len(parts) == 2 {
				role := strings.TrimSpace(parts[0])
				goal := strings.TrimSpace(parts[1])
				task, err := s.createAgentTask(role, goal)
				if err != nil {
					results = append(results, ChatActionResult{Kind: "agent", Summary: "创建 agent 任务失败: " + err.Error()})
				} else {
					refs = append(refs, fmt.Sprintf("agent:%s", task.ID))
					results = append(results, ChatActionResult{Kind: "agent", Summary: "已创建 agent 任务", Path: task.ID, Output: fmt.Sprintf("角色: %s\n目标: %s", task.Role, task.Goal)})
				}
			}
		} else if strings.HasPrefix(line, "ACTION:AGENT-DONE ") {
			taskID := strings.TrimPrefix(line, "ACTION:AGENT-DONE ")
			taskID = strings.TrimSpace(taskID)
			if taskID != "" {
				task, err := s.completeAgentTask(taskID)
				if err != nil {
					results = append(results, ChatActionResult{Kind: "agent", Summary: "完成 agent 任务失败: " + err.Error(), Path: taskID})
				} else {
					refs = append(refs, fmt.Sprintf("agent-done:%s", task.ID))
					results = append(results, ChatActionResult{Kind: "agent", Summary: "Agent 任务已完成", Path: task.ID, Output: task.Goal})
				}
			}
		} else if strings.HasPrefix(line, "ACTION:GOAL ") {
			title := strings.TrimPrefix(line, "ACTION:GOAL ")
			title = strings.TrimSpace(title)
			if title != "" {
				goal, err := s.createWorkspaceGoal(title)
				if err != nil {
					results = append(results, ChatActionResult{Kind: "goal", Summary: "创建子目标失败: " + err.Error()})
				} else {
					refs = append(refs, fmt.Sprintf("goal:%s:%s", goal.ID, goal.Title))
					results = append(results, ChatActionResult{Kind: "goal", Summary: "已创建子目标", Path: goal.ID, Output: truncateForAI(goal.Title, 500)})
				}
			}
		} else if strings.HasPrefix(line, "ACTION:GOAL-DONE ") {
			title := strings.TrimPrefix(line, "ACTION:GOAL-DONE ")
			title = strings.TrimSpace(title)
			if title != "" {
				goal, err := s.completeWorkspaceGoal(title)
				if err != nil {
					results = append(results, ChatActionResult{Kind: "goal", Summary: "推进子目标失败: " + err.Error()})
				} else {
					refs = append(refs, fmt.Sprintf("goal-done:%s", goal.ID))
					results = append(results, ChatActionResult{Kind: "goal", Summary: "子目标已完成", Path: goal.ID, Output: goal.Title})
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

type BlackboardFact struct {
	ID        string `json:"id"`
	Kind      string `json:"kind"`
	Statement string `json:"statement"`
	Source    string `json:"source"`
	CreatedAt int64  `json:"createdAt"`
}

func (s *Server) blackboardPath() string {
	return filepath.Join(s.workspace, "runtime", "blackboard.jsonl")
}

func (s *Server) loadBlackboardFacts() []BlackboardFact {
	path := s.blackboardPath()
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return []BlackboardFact{}
	}
	file, err := os.Open(path)
	if err != nil {
		return []BlackboardFact{}
	}
	defer file.Close()
	var facts []BlackboardFact
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var fact BlackboardFact
		if json.Unmarshal(scanner.Bytes(), &fact) == nil {
			facts = append(facts, fact)
		}
	}
	return facts
}

func (s *Server) appendBlackboardFact(kind string, statement string) (BlackboardFact, error) {
	runtimeDir := filepath.Join(s.workspace, "runtime")
	os.MkdirAll(runtimeDir, 0o755)
	path := s.blackboardPath()
	fact := BlackboardFact{
		ID:        fmt.Sprintf("fact-%d", time.Now().UnixNano()),
		Kind:      kind,
		Statement: statement,
		Source:    "ai-agent",
		CreatedAt: time.Now().Unix(),
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return BlackboardFact{}, err
	}
	defer file.Close()
	data, err := json.Marshal(fact)
	if err != nil {
		return BlackboardFact{}, err
	}
	_, err = file.Write(append(data, '\n'))
	return fact, err
}

type HarnessResult struct {
	ID         string `json:"id"`
	PromptName string `json:"promptName"`
	Input      string `json:"input"`
	Output     string `json:"output"`
	Model      string `json:"model"`
	CreatedAt  int64  `json:"createdAt"`
}

func (s *Server) harnessPath() string {
	return filepath.Join(s.workspace, "runtime", "harness-results.jsonl")
}

func (s *Server) loadWorkspaceHarnessResults() []HarnessResult {
	path := s.harnessPath()
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return []HarnessResult{}
	}
	file, err := os.Open(path)
	if err != nil {
		return []HarnessResult{}
	}
	defer file.Close()
	var results []HarnessResult
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var result HarnessResult
		if json.Unmarshal(scanner.Bytes(), &result) == nil {
			results = append(results, result)
		}
	}
	return results
}

func (s *Server) runWorkspaceHarness(promptName string, inputText string) (HarnessResult, error) {
	prompt, ok := s.findWorkspacePrompt(promptName)
	if !ok {
		return HarnessResult{}, fmt.Errorf("prompt 未找到: %s", promptName)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	result, err := app.GenerateProviderMessage(ctx, s.workspace, app.ProviderGenerationRequest{
		SystemPrompt:         prompt.Content,
		UserPrompt:           inputText,
		MaxTokens:            1024,
		RequiredCapabilities: []string{"chat"},
		Stream:               false,
	})
	if err != nil {
		return HarnessResult{}, err
	}
	harness := HarnessResult{
		ID:         fmt.Sprintf("harness-%d", time.Now().UnixNano()),
		PromptName: prompt.Name,
		Input:      inputText,
		Output:     result.Text,
		Model:      result.Model,
		CreatedAt:  time.Now().Unix(),
	}
	runtimeDir := filepath.Join(s.workspace, "runtime")
	os.MkdirAll(runtimeDir, 0o755)
	path := s.harnessPath()
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return HarnessResult{}, err
	}
	defer file.Close()
	data, err := json.Marshal(harness)
	if err != nil {
		return HarnessResult{}, err
	}
	_, err = file.Write(append(data, '\n'))
	return harness, err
}

type AgentTaskRecord struct {
	ID        string `json:"id"`
	Role      string `json:"role"`
	Goal      string `json:"goal"`
	Status    string `json:"status"`
	CreatedAt int64  `json:"createdAt"`
	UpdatedAt int64  `json:"updatedAt"`
}

func (s *Server) agentTasksPath() string {
	return filepath.Join(s.workspace, "runtime", "agent-tasks.jsonl")
}

func (s *Server) loadAgentTasks() []AgentTaskRecord {
	path := s.agentTasksPath()
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return []AgentTaskRecord{}
	}
	file, err := os.Open(path)
	if err != nil {
		return []AgentTaskRecord{}
	}
	defer file.Close()
	var tasks []AgentTaskRecord
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var task AgentTaskRecord
		if json.Unmarshal(scanner.Bytes(), &task) == nil {
			tasks = append(tasks, task)
		}
	}
	return tasks
}

func (s *Server) createAgentTask(role string, goal string) (AgentTaskRecord, error) {
	role = strings.TrimSpace(role)
	goal = strings.TrimSpace(goal)
	if role == "" || goal == "" {
		return AgentTaskRecord{}, fmt.Errorf("agent 角色和目标不能为空")
	}
	runtimeDir := filepath.Join(s.workspace, "runtime")
	os.MkdirAll(runtimeDir, 0o755)
	now := time.Now().Unix()
	task := AgentTaskRecord{ID: fmt.Sprintf("agent-%d", now), Role: role, Goal: goal, Status: "queued", CreatedAt: now, UpdatedAt: now}
	file, err := os.OpenFile(s.agentTasksPath(), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return AgentTaskRecord{}, err
	}
	defer file.Close()
	data, err := json.Marshal(task)
	if err != nil {
		return AgentTaskRecord{}, err
	}
	_, err = file.Write(append(data, '\n'))
	return task, err
}

func (s *Server) completeAgentTask(match string) (AgentTaskRecord, error) {
	match = strings.TrimSpace(match)
	tasks := s.loadAgentTasks()
	if len(tasks) == 0 {
		return AgentTaskRecord{}, fmt.Errorf("未找到 agent 任务")
	}
	path := s.agentTasksPath()
	updated := false
	for i, task := range tasks {
		if task.ID == match && task.Status != "complete" {
			tasks[i].Status = "complete"
			tasks[i].UpdatedAt = time.Now().Unix()
			updated = true
		}
	}
	if !updated {
		return AgentTaskRecord{}, fmt.Errorf("未找到待完成的 agent 任务: %s", match)
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return AgentTaskRecord{}, err
	}
	defer file.Close()
	for _, task := range tasks {
		data, _ := json.Marshal(task)
		file.Write(append(data, '\n'))
	}
	for _, task := range tasks {
		if task.ID == match {
			return task, nil
		}
	}
	return AgentTaskRecord{}, fmt.Errorf("完成的 agent 任务未找到")
}

type WorkspacePrompt struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Content   string `json:"content"`
	CreatedAt int64  `json:"createdAt"`
}

func (s *Server) promptsPath() string {
	return filepath.Join(s.workspace, "runtime", "prompts.jsonl")
}

func (s *Server) loadWorkspacePrompts() []WorkspacePrompt {
	path := s.promptsPath()
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return []WorkspacePrompt{}
	}
	file, err := os.Open(path)
	if err != nil {
		return []WorkspacePrompt{}
	}
	defer file.Close()
	var prompts []WorkspacePrompt
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var prompt WorkspacePrompt
		if json.Unmarshal(scanner.Bytes(), &prompt) == nil {
			prompts = append(prompts, prompt)
		}
	}
	return prompts
}

func (s *Server) findWorkspacePrompt(name string) (WorkspacePrompt, bool) {
	for _, prompt := range s.loadWorkspacePrompts() {
		if prompt.Name == strings.TrimSpace(name) {
			return prompt, true
		}
	}
	return WorkspacePrompt{}, false
}

func (s *Server) saveWorkspacePrompt(name string, content string) (WorkspacePrompt, error) {
	runtimeDir := filepath.Join(s.workspace, "runtime")
	os.MkdirAll(runtimeDir, 0o755)
	path := s.promptsPath()
	prompt := WorkspacePrompt{
		ID:        fmt.Sprintf("prompt-%d", time.Now().UnixNano()),
		Name:      strings.TrimSpace(name),
		Content:   content,
		CreatedAt: time.Now().Unix(),
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return WorkspacePrompt{}, err
	}
	defer file.Close()
	data, err := json.Marshal(prompt)
	if err != nil {
		return WorkspacePrompt{}, err
	}
	_, err = file.Write(append(data, '\n'))
	return prompt, err
}

type WorkspaceGoal struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Status    string `json:"status"`
	CreatedAt int64  `json:"createdAt"`
	UpdatedAt int64  `json:"updatedAt"`
}

func (s *Server) workspaceGoalPath() string {
	return filepath.Join(s.workspace, "runtime", "goals.jsonl")
}

func (s *Server) loadWorkspaceGoals() []WorkspaceGoal {
	path := s.workspaceGoalPath()
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return []WorkspaceGoal{}
	}
	file, err := os.Open(path)
	if err != nil {
		return []WorkspaceGoal{}
	}
	defer file.Close()
	var goals []WorkspaceGoal
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var goal WorkspaceGoal
		if json.Unmarshal(scanner.Bytes(), &goal) == nil {
			goals = append(goals, goal)
		}
	}
	return goals
}

func (s *Server) createWorkspaceGoal(title string) (WorkspaceGoal, error) {
	runtimeDir := filepath.Join(s.workspace, "runtime")
	os.MkdirAll(runtimeDir, 0o755)
	path := s.workspaceGoalPath()
	now := time.Now().Unix()
	goal := WorkspaceGoal{ID: fmt.Sprintf("goal-%d", now), Title: title, Status: "open", CreatedAt: now, UpdatedAt: now}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return WorkspaceGoal{}, err
	}
	defer file.Close()
	data, err := json.Marshal(goal)
	if err != nil {
		return WorkspaceGoal{}, err
	}
	_, err = file.Write(append(data, '\n'))
	return goal, err
}

func (s *Server) completeWorkspaceGoal(match string) (WorkspaceGoal, error) {
	goals := s.loadWorkspaceGoals()
	path := s.workspaceGoalPath()
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return WorkspaceGoal{}, fmt.Errorf("goals file missing")
	}
	matched := false
	now := time.Now().Unix()
	for i, goal := range goals {
		if strings.TrimSpace(goal.Title) == strings.TrimSpace(match) || goal.ID == match {
			if goal.Status != "completed" {
				goals[i].Status = "completed"
				goals[i].UpdatedAt = now
				matched = true
			}
		}
	}
	if !matched {
		return WorkspaceGoal{}, fmt.Errorf("子目标未找到: %s", match)
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return WorkspaceGoal{}, err
	}
	defer file.Close()
	for _, goal := range goals {
		data, _ := json.Marshal(goal)
		file.Write(append(data, '\n'))
	}
	for _, goal := range goals {
		if goal.Status == "completed" && (goal.Title == match || goal.ID == match) {
			return goal, nil
		}
	}
	return WorkspaceGoal{}, fmt.Errorf("completed goal not found")
}

func (s *Server) continueAfterAIActions(initialText string, systemPrompt string, refs []string, actionResults []ChatActionResult) (string, []string, []ChatActionResult) {
	var feedback []string
	for _, result := range actionResults {
		if result.Kind == "execute" || result.Kind == "tool" || result.Kind == "read" {
			if strings.TrimSpace(result.Output) != "" {
				feedback = append(feedback, fmt.Sprintf("[%s] %s\n%s", result.Kind, result.Summary, result.Output))
			}
		}
	}
	if len(feedback) == 0 {
		return initialText, refs, actionResults
	}
	followUpPrompt := "这是刚才动作的真实执行结果，请基于这些结果直接给出简洁的结论、下一步建议和需要关注的风险。不要再次输出 ACTION 标记。\n\n" + strings.Join(feedback, "\n\n")
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	followUp, err := app.GenerateProviderMessage(ctx, s.workspace, app.ProviderGenerationRequest{
		SystemPrompt:         systemPrompt,
		UserPrompt:           followUpPrompt,
		MaxTokens:            2048,
		RequiredCapabilities: []string{"chat"},
		Stream:               false,
	})
	if err != nil {
		return initialText, refs, actionResults
	}
	combined := initialText + "\n\n【动作结果分析】\n" + followUp.Text
	return combined, refs, actionResults
}

func (s *Server) applyExplicitGoalFallback(message string, results []ChatActionResult, refs *[]string) []ChatActionResult {
	lower := strings.ToLower(message)
	if !strings.Contains(lower, "子目标") && !strings.Contains(lower, "goal") {
		return results
	}
	// Already has goal action
	for _, result := range results {
		if result.Kind == "goal" {
			return results
		}
	}
	if strings.Contains(lower, "完成") || strings.Contains(lower, "推进") {
		title := strings.TrimSpace(strings.TrimPrefix(message, "完成子目标"))
		title = strings.TrimSpace(strings.TrimPrefix(title, "推进子目标"))
		title = strings.TrimSpace(strings.Trim(title, "：:。"))
		if title != "" {
			goal, err := s.completeWorkspaceGoal(title)
			if err == nil {
				*refs = append(*refs, fmt.Sprintf("goal-done:%s", goal.ID))
				return append(results, ChatActionResult{Kind: "goal", Summary: "子目标已完成", Path: goal.ID, Output: goal.Title})
			}
		}
	}
	// Default creation: parse after keywords like "创建子目标：" or trailing suffix
	title := message
	for _, prefix := range []string{"创建子目标", "新建子目标", "添加子目标", "子目标"} {
		if idx := strings.Index(title, prefix); idx >= 0 {
			title = strings.TrimSpace(strings.TrimPrefix(title[idx+len(prefix):], "："))
			title = strings.TrimSpace(strings.Trim(title, "：:。为"))
			break
		}
	}
	if title == "" {
		return results
	}
	goal, err := s.createWorkspaceGoal(title)
	if err != nil {
		return append(results, ChatActionResult{Kind: "goal", Summary: "创建子目标失败: " + err.Error()})
	}
	*refs = append(*refs, fmt.Sprintf("goal:%s:%s", goal.ID, goal.Title))
	return append(results, ChatActionResult{Kind: "goal", Summary: "已创建子目标", Path: goal.ID, Output: goal.Title})
}

func (s *Server) writeWorkspaceFile(relativePath string, content string) error {
	relativePath = strings.TrimSpace(relativePath)
	if relativePath == "" {
		return fmt.Errorf("文件路径不能为空")
	}
	root, err := filepath.Abs(s.workspace)
	if err != nil {
		return err
	}
	path, err := filepath.Abs(filepath.Join(root, relativePath))
	if err != nil {
		return err
	}
	if !strings.HasPrefix(path, filepath.Clean(root)+string(os.PathSeparator)) && path != filepath.Clean(root) {
		return fmt.Errorf("文件路径超出工作区: %s", relativePath)
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0o644)
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
