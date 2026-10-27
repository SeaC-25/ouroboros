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

// AIActionResponseDTO 匹配前端 AIActionResponse
type AIActionResponseDTO struct {
	ID           string   `json:"id"`
	Kind         string   `json:"kind"`
	Label        string   `json:"label"`
	Status       string   `json:"status"`
	Description  string   `json:"description,omitempty"`
	PackID       string   `json:"packId,omitempty"`
	PackName     string   `json:"packName,omitempty"`
	Command      []string `json:"command,omitempty"`
	CommandText  string   `json:"commandText,omitempty"`
	RunTimeoutMs int      `json:"runTimeoutMs,omitempty"`
	ManifestPath string   `json:"manifestPath,omitempty"`
	Capabilities []string `json:"capabilities,omitempty"`
	Runner       string   `json:"runner,omitempty"`
	Parser       string   `json:"parser,omitempty"`
	ArgumentHint string   `json:"argumentHint,omitempty"`
}

// AIActionsListResponse 匹配前端 AIActionsResponsePayload
type AIActionsListResponse struct {
	Actions []AIActionResponseDTO `json:"actions"`
}

// AIActionRunRequestDTO 匹配前端 AIActionRunRequest
type AIActionRunRequestDTO struct {
	ActionID string `json:"actionId"`
	Input    *struct {
		Title          string   `json:"title,omitempty"`
		Goal           string   `json:"goal,omitempty"`
		Kind           string   `json:"kind,omitempty"`
		Command        string   `json:"command,omitempty"`
		ExpectedResult string   `json:"expectedResult,omitempty"`
		Priority       string   `json:"priority,omitempty"`
		Source         string   `json:"source,omitempty"`
		FilePath       string   `json:"filePath,omitempty"`
		Content        string   `json:"content,omitempty"`
		Role           string   `json:"role,omitempty"`
		PromptName     string   `json:"promptName,omitempty"`
		PromptContent  string   `json:"promptContent,omitempty"`
		InputText      string   `json:"inputText,omitempty"`
		HarnessCaseId  string   `json:"harnessCaseId,omitempty"`
		Target         string   `json:"target,omitempty"`
		Roles          []string `json:"roles,omitempty"`
		Commands       []string `json:"commands,omitempty"`
		Agents         []string `json:"agents,omitempty"`
		ExecutionID    string   `json:"executionId,omitempty"`
		Reason         string   `json:"reason,omitempty"`
	} `json:"input,omitempty"`
	Arguments string `json:"arguments,omitempty"`
}

// AIActionFileDiffDTO matches the frontend HarnessFileDiffResponse shape
// (path/operation/summary/diff) — the old scalar fileDiff had no consumer.
type AIActionFileDiffDTO struct {
	Path      string `json:"path,omitempty"`
	Operation string `json:"operation,omitempty"`
	Summary   string `json:"summary,omitempty"`
	Diff      string `json:"diff,omitempty"`
}

// AIActionResultDTO 匹配前端 AIActionResult
type AIActionResultDTO struct {
	Kind            string                `json:"kind"`
	Operation       string                `json:"operation,omitempty"`
	Summary         string                `json:"summary,omitempty"`
	FileDiffs       []AIActionFileDiffDTO `json:"fileDiffs,omitempty"`
	OrchestratorRun *struct {
		ID             string `json:"id"`
		ExecutionID    string `json:"executionId,omitempty"`
		Title          string `json:"title"`
		Goal           string `json:"goal"`
		Kind           string `json:"kind"`
		Command        string `json:"command"`
		ExpectedResult string `json:"expectedResult"`
		Priority       string `json:"priority,omitempty"`
		Source         string `json:"source,omitempty"`
		FilePath       string `json:"filePath,omitempty"`
		Status         string `json:"status"`
		CreatedAt      int64  `json:"createdAt"`
	} `json:"orchestratorRun,omitempty"`
	BlackboardItem *BlackboardFact        `json:"blackboardItem,omitempty"`
	Observation    *BlackboardObservation `json:"observation,omitempty"`
	AgentTask      *AgentTaskRecord       `json:"agentTask,omitempty"`
	Prompt         *WorkspacePrompt       `json:"prompt,omitempty"`
	HarnessCase    *WorkspaceHarnessCase  `json:"harnessCase,omitempty"`
	HarnessRun     *HarnessResult         `json:"harnessRun,omitempty"`
	SwarmRun       *SwarmWorkflowRun      `json:"swarmRun,omitempty"`
	Execution      *ExecutionDTO          `json:"execution,omitempty"`
}

// AIActionRunResponseDTO 匹配前端 AIActionRunResponse
type AIActionRunResponseDTO struct {
	Action AIActionResponseDTO `json:"action"`
	Result AIActionResultDTO   `json:"result"`
}

// ReportGenerateInput 报告生成动作的真实输入。
type ReportGenerateInput struct {
	Title    string
	Goal     string
	FilePath string
}

// ReportGenerateResult 报告生成动作的真实结果。
type ReportGenerateResult struct {
	RelativePath string
	Model        string
	Text         string
	Diff         string
}

// SwarmWorkflowRun 多智能体调度的一次真实运行记录
type SwarmWorkflowRun struct {
	ID          string                     `json:"id"`
	Title       string                     `json:"title"`
	Goal        string                     `json:"goal"`
	Status      string                     `json:"status"`
	Tasks       []AgentTaskRecord          `json:"tasks,omitempty"`
	Facts       []BlackboardFact           `json:"facts,omitempty"`
	Executions  []ExecutionDTO             `json:"executions,omitempty"`
	Summary     string                     `json:"summary,omitempty"`
	CreatedAt   int64                      `json:"createdAt"`
	UpdatedAt   int64                      `json:"updatedAt"`
	AgentOutput map[string]string          `json:"agentOutput,omitempty"`
	Timeline    []SwarmWorkflowStep        `json:"timeline,omitempty"`
	Inputs      SwarmWorkflowInputSnapshot `json:"inputs,omitempty"`
}

type SwarmWorkflowInputSnapshot struct {
	Title    string   `json:"title,omitempty"`
	Goal     string   `json:"goal,omitempty"`
	Roles    []string `json:"roles,omitempty"`
	Commands []string `json:"commands,omitempty"`
}

type SwarmWorkflowStep struct {
	Step      string `json:"step"`
	Detail    string `json:"detail,omitempty"`
	CreatedAt int64  `json:"createdAt"`
}

// handleAIActions 返回可用的 AI 动作列表
func (s *Server) handleAIActions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		respondError(w, http.StatusMethodNotAllowed, "不支持的AI动作方法")
		return
	}
	actions := []AIActionResponseDTO{
		{
			ID:           "orchestrator.run",
			Kind:         "orchestrator",
			Label:        "AI 编排执行",
			Status:       "ready",
			Description:  "使用 AI 模型分析目标并生成执行计划，然后自动运行",
			Capabilities: []string{"chat", "reasoning"},
		},
		{
			ID:           "swarm.run",
			Kind:         "swarm",
			Label:        "多智能体调度",
			Status:       "ready",
			Description:  "真实分派多个智能体任务、写入黑板并启动命令执行",
			Capabilities: []string{"chat", "reasoning", "agent", "blackboard", "execution"},
		},
		{
			ID:           "failure.recover",
			Kind:         "failure",
			Label:        "失败恢复与重试",
			Status:       "ready",
			Description:  "读取真实失败执行、写入黑板并启动带 retryOf 的重试执行",
			Capabilities: []string{"chat", "reasoning", "execution", "blackboard"},
		},
		{
			ID:           "report.generate",
			Kind:         "report",
			Label:        "生成安全报告",
			Status:       "ready",
			Description:  "基于真实工作区状态调用 AI 生成 Markdown 报告并落盘",
			Capabilities: []string{"chat", "reasoning", "report"},
		},
		{
			ID:           "blackboard.create",
			Kind:         "blackboard",
			Label:        "记录结论笔记",
			Status:       "ready",
			Description:  "把模型推理得出的结论笔记持久化到黑板（动作 ID 保留，供前端/回放兼容）",
			Capabilities: []string{"chat", "reasoning"},
		},
		{
			ID:           "observation.create",
			Kind:         "observation",
			Label:        "记录原始观测",
			Status:       "ready",
			Description:  "把一次原始观测（如 http_observe 输出）写入观测存储，不做任何判定",
			Capabilities: []string{"chat", "reasoning"},
		},
		{
			ID:           "file.write",
			Kind:         "file",
			Label:        "写入工作区文件",
			Status:       "ready",
			Description:  "将内容真实写入工作区文件并返回 diff",
			Capabilities: []string{"chat", "reasoning"},
		},
		{
			ID:           "agent.create",
			Kind:         "agent",
			Label:        "创建智能体任务",
			Status:       "ready",
			Description:  "创建真实的智能体任务并写入工作区",
			Capabilities: []string{"chat", "reasoning"},
		},
		{
			ID:           "prompt.save",
			Kind:         "prompt",
			Label:        "保存 Prompt",
			Status:       "ready",
			Description:  "把 prompt 保存到工作区 prompt registry",
			Capabilities: []string{"chat", "reasoning"},
		},
		{
			ID:           "harness.create",
			Kind:         "harness",
			Label:        "创建评测用例",
			Status:       "ready",
			Description:  "创建持久化的 harness case",
			Capabilities: []string{"chat", "reasoning"},
		},
		{
			ID:           "harness.run",
			Kind:         "harness",
			Label:        "运行评测",
			Status:       "ready",
			Description:  "用已保存 prompt 运行真实模型评测",
			Capabilities: []string{"chat", "reasoning"},
		},
	}
	respondJSON(w, http.StatusOK, AIActionsListResponse{Actions: actions})
}

// handleAIActionRun 执行 AI 动作并返回真实模型生成结果
func (s *Server) handleAIActionRun(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		respondError(w, http.StatusMethodNotAllowed, "不支持的AI动作方法")
		return
	}

	var request AIActionRunRequestDTO
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		respondError(w, http.StatusBadRequest, fmt.Sprintf("解析AI动作请求失败: %v", err))
		return
	}

	if request.Input == nil {
		respondError(w, http.StatusBadRequest, "缺少AI动作输入参数")
		return
	}
	input := request.Input

	switch request.ActionID {
	case "blackboard.create":
		s.runBlackboardCreate(w, request.ActionID, input)
	case "observation.create":
		s.runObservationCreate(w, request.ActionID, input)
	case "file.write":
		s.runFileWrite(w, request.ActionID, input)
	case "agent.create":
		s.runAgentCreate(w, request.ActionID, input)
	case "prompt.save":
		s.runPromptSave(w, request.ActionID, input)
	case "harness.create":
		s.runHarnessCreate(w, request.ActionID, input)
	case "harness.run":
		s.runHarnessRun(w, request.ActionID, input)
	case "swarm.run":
		s.runSwarmWorkflow(w, request.ActionID, input)
	case "failure.recover":
		s.runFailureRecover(w, request.ActionID, input)
	case "report.generate":
		s.runReportGenerate(w, request.ActionID, input)
	case "orchestrator.run":
		s.runOrchestratorAI(w, r, request.ActionID, input)
	default:
		respondError(w, http.StatusBadRequest, fmt.Sprintf("未知的AI动作: %s", request.ActionID))
	}
}

func (s *Server) respondAIActionSuccess(w http.ResponseWriter, actionID string, actionStatus, description string, result AIActionResultDTO) {
	respondJSON(w, http.StatusOK, AIActionRunResponseDTO{
		Action: AIActionResponseDTO{
			ID:           actionID,
			Kind:         aiActionKind(actionID),
			Label:        aiActionLabel(actionID),
			Status:       actionStatus,
			Description:  description,
			Capabilities: []string{"chat", "reasoning"},
		},
		Result: result,
	})
}

func aiActionKind(actionID string) string {
	switch actionID {
	case "orchestrator.run":
		return "orchestrator"
	case "blackboard.create":
		return "blackboard"
	case "observation.create":
		return "observation"
	case "file.write":
		return "file"
	case "agent.create":
		return "agent"
	case "prompt.save":
		return "prompt"
	case "harness.create", "harness.run":
		return "harness"
	case "swarm.run":
		return "swarm"
	case "failure.recover":
		return "failure"
	case "report.generate":
		return "report"
	default:
		return "command"
	}
}

func aiActionLabel(actionID string) string {
	switch actionID {
	case "orchestrator.run":
		return "AI 编排执行"
	case "blackboard.create":
		return "记录结论笔记"
	case "observation.create":
		return "记录原始观测"
	case "file.write":
		return "写入工作区文件"
	case "agent.create":
		return "创建智能体任务"
	case "prompt.save":
		return "保存 Prompt"
	case "harness.create":
		return "创建评测用例"
	case "harness.run":
		return "运行评测"
	case "swarm.run":
		return "多智能体调度"
	case "failure.recover":
		return "失败恢复与重试"
	case "report.generate":
		return "生成安全报告"
	default:
		return actionID
	}
}

func (s *Server) runBlackboardCreate(w http.ResponseWriter, actionID string, input *struct {
	Title          string   `json:"title,omitempty"`
	Goal           string   `json:"goal,omitempty"`
	Kind           string   `json:"kind,omitempty"`
	Command        string   `json:"command,omitempty"`
	ExpectedResult string   `json:"expectedResult,omitempty"`
	Priority       string   `json:"priority,omitempty"`
	Source         string   `json:"source,omitempty"`
	FilePath       string   `json:"filePath,omitempty"`
	Content        string   `json:"content,omitempty"`
	Role           string   `json:"role,omitempty"`
	PromptName     string   `json:"promptName,omitempty"`
	PromptContent  string   `json:"promptContent,omitempty"`
	InputText      string   `json:"inputText,omitempty"`
	HarnessCaseId  string   `json:"harnessCaseId,omitempty"`
	Target         string   `json:"target,omitempty"`
	Roles          []string `json:"roles,omitempty"`
	Commands       []string `json:"commands,omitempty"`
	Agents         []string `json:"agents,omitempty"`
	ExecutionID    string   `json:"executionId,omitempty"`
	Reason         string   `json:"reason,omitempty"`
}) {
	content := strings.TrimSpace(input.Content)
	if content == "" {
		respondError(w, http.StatusBadRequest, "记录结论笔记需要 content")
		return
	}
	// blackboard.create 动作 ID 保留（前端/回放兼容），语义已从"发现"降级为
	// "模型推理得出的结论笔记"——观测证据走 observation.create。
	fact, err := s.appendBlackboardFact("action", content)
	if err != nil {
		respondError(w, http.StatusInternalServerError, fmt.Sprintf("记录结论笔记失败: %v", err))
		return
	}
	factCopy := fact
	s.respondAIActionSuccess(w, actionID, "ready", "已记录结论笔记", AIActionResultDTO{
		Kind:           "blackboard",
		Operation:      "blackboard_create",
		Summary:        "已记录结论笔记",
		BlackboardItem: &factCopy,
	})
}

// runObservationCreate 落一条原始观测：kind/target/payload 由调用方给出，
// 观测先入存储，判定由模型后续推理得出——这里不做任何解释。
func (s *Server) runObservationCreate(w http.ResponseWriter, actionID string, input *struct {
	Title          string   `json:"title,omitempty"`
	Goal           string   `json:"goal,omitempty"`
	Kind           string   `json:"kind,omitempty"`
	Command        string   `json:"command,omitempty"`
	ExpectedResult string   `json:"expectedResult,omitempty"`
	Priority       string   `json:"priority,omitempty"`
	Source         string   `json:"source,omitempty"`
	FilePath       string   `json:"filePath,omitempty"`
	Content        string   `json:"content,omitempty"`
	Role           string   `json:"role,omitempty"`
	PromptName     string   `json:"promptName,omitempty"`
	PromptContent  string   `json:"promptContent,omitempty"`
	InputText      string   `json:"inputText,omitempty"`
	HarnessCaseId  string   `json:"harnessCaseId,omitempty"`
	Target         string   `json:"target,omitempty"`
	Roles          []string `json:"roles,omitempty"`
	Commands       []string `json:"commands,omitempty"`
	Agents         []string `json:"agents,omitempty"`
	ExecutionID    string   `json:"executionId,omitempty"`
	Reason         string   `json:"reason,omitempty"`
}) {
	kind := strings.TrimSpace(input.Kind)
	target := strings.TrimSpace(input.Target)
	payload := strings.TrimSpace(input.Content)
	if kind == "" {
		respondError(w, http.StatusBadRequest, "记录原始观测需要 kind")
		return
	}
	if target == "" {
		respondError(w, http.StatusBadRequest, "记录原始观测需要 target")
		return
	}
	if payload == "" {
		payload = strings.TrimSpace(input.PromptContent)
	}
	if payload == "" {
		payload = strings.TrimSpace(input.InputText)
	}
	observation, err := s.appendBlackboardObservation(kind, target, payload, strings.TrimSpace(input.Source))
	if err != nil {
		respondError(w, http.StatusInternalServerError, fmt.Sprintf("记录原始观测失败: %v", err))
		return
	}
	s.respondAIActionSuccess(w, actionID, "ready", "已记录原始观测", AIActionResultDTO{
		Kind:        "observation",
		Operation:   "observation_create",
		Summary:     "已记录原始观测",
		Observation: &observation,
	})
}

func (s *Server) runFileWrite(w http.ResponseWriter, actionID string, input *struct {
	Title          string   `json:"title,omitempty"`
	Goal           string   `json:"goal,omitempty"`
	Kind           string   `json:"kind,omitempty"`
	Command        string   `json:"command,omitempty"`
	ExpectedResult string   `json:"expectedResult,omitempty"`
	Priority       string   `json:"priority,omitempty"`
	Source         string   `json:"source,omitempty"`
	FilePath       string   `json:"filePath,omitempty"`
	Content        string   `json:"content,omitempty"`
	Role           string   `json:"role,omitempty"`
	PromptName     string   `json:"promptName,omitempty"`
	PromptContent  string   `json:"promptContent,omitempty"`
	InputText      string   `json:"inputText,omitempty"`
	HarnessCaseId  string   `json:"harnessCaseId,omitempty"`
	Target         string   `json:"target,omitempty"`
	Roles          []string `json:"roles,omitempty"`
	Commands       []string `json:"commands,omitempty"`
	Agents         []string `json:"agents,omitempty"`
	ExecutionID    string   `json:"executionId,omitempty"`
	Reason         string   `json:"reason,omitempty"`
}) {
	filePath := strings.TrimSpace(input.FilePath)
	content := strings.TrimSpace(input.Content)
	if content == "" {
		content = strings.TrimSpace(input.PromptContent)
	}
	if filePath == "" || content == "" {
		respondError(w, http.StatusBadRequest, "写入工作区文件需要 filePath 和 content")
		return
	}
	oldContent := ""
	if existing, readErr := ReadWorkspaceFile(s.workspace, filePath); readErr == nil {
		oldContent = existing.Content
	}
	if _, err := WriteWorkspaceFile(s.workspace, filePath, content); err != nil {
		respondError(w, http.StatusInternalServerError, fmt.Sprintf("写入工作区文件失败: %v", err))
		return
	}
	s.respondAIActionSuccess(w, actionID, "ready", "已写入工作区文件", AIActionResultDTO{
		Kind:      "file",
		Operation: "file_write",
		Summary:   "已写入工作区文件",
		FileDiffs: []AIActionFileDiffDTO{{Path: filePath, Operation: "write", Diff: unifiedLineDiff(filePath, oldContent, content)}},
	})
}

func (s *Server) runAgentCreate(w http.ResponseWriter, actionID string, input *struct {
	Title          string   `json:"title,omitempty"`
	Goal           string   `json:"goal,omitempty"`
	Kind           string   `json:"kind,omitempty"`
	Command        string   `json:"command,omitempty"`
	ExpectedResult string   `json:"expectedResult,omitempty"`
	Priority       string   `json:"priority,omitempty"`
	Source         string   `json:"source,omitempty"`
	FilePath       string   `json:"filePath,omitempty"`
	Content        string   `json:"content,omitempty"`
	Role           string   `json:"role,omitempty"`
	PromptName     string   `json:"promptName,omitempty"`
	PromptContent  string   `json:"promptContent,omitempty"`
	InputText      string   `json:"inputText,omitempty"`
	HarnessCaseId  string   `json:"harnessCaseId,omitempty"`
	Target         string   `json:"target,omitempty"`
	Roles          []string `json:"roles,omitempty"`
	Commands       []string `json:"commands,omitempty"`
	Agents         []string `json:"agents,omitempty"`
	ExecutionID    string   `json:"executionId,omitempty"`
	Reason         string   `json:"reason,omitempty"`
}) {
	goal := strings.TrimSpace(input.Goal)
	if goal == "" {
		respondError(w, http.StatusBadRequest, "创建智能体任务需要 goal")
		return
	}
	role := strings.TrimSpace(input.Role)
	if role == "" {
		role = strings.TrimSpace(input.Title)
	}
	task, err := s.createAgentTask(role, goal)
	if err != nil {
		respondError(w, http.StatusInternalServerError, fmt.Sprintf("创建智能体任务失败: %v", err))
		return
	}
	taskCopy := task
	s.respondAIActionSuccess(w, actionID, "ready", "已创建智能体任务", AIActionResultDTO{
		Kind:      "agent",
		Operation: "agent_create",
		Summary:   "已创建智能体任务",
		AgentTask: &taskCopy,
	})
}

func (s *Server) runPromptSave(w http.ResponseWriter, actionID string, input *struct {
	Title          string   `json:"title,omitempty"`
	Goal           string   `json:"goal,omitempty"`
	Kind           string   `json:"kind,omitempty"`
	Command        string   `json:"command,omitempty"`
	ExpectedResult string   `json:"expectedResult,omitempty"`
	Priority       string   `json:"priority,omitempty"`
	Source         string   `json:"source,omitempty"`
	FilePath       string   `json:"filePath,omitempty"`
	Content        string   `json:"content,omitempty"`
	Role           string   `json:"role,omitempty"`
	PromptName     string   `json:"promptName,omitempty"`
	PromptContent  string   `json:"promptContent,omitempty"`
	InputText      string   `json:"inputText,omitempty"`
	HarnessCaseId  string   `json:"harnessCaseId,omitempty"`
	Target         string   `json:"target,omitempty"`
	Roles          []string `json:"roles,omitempty"`
	Commands       []string `json:"commands,omitempty"`
	Agents         []string `json:"agents,omitempty"`
	ExecutionID    string   `json:"executionId,omitempty"`
	Reason         string   `json:"reason,omitempty"`
}) {
	name := strings.TrimSpace(input.Title)
	if name == "" {
		name = strings.TrimSpace(input.PromptName)
	}
	content := strings.TrimSpace(input.Content)
	if content == "" {
		content = strings.TrimSpace(input.PromptContent)
	}
	if name == "" || content == "" {
		respondError(w, http.StatusBadRequest, "保存 Prompt 需要名称和内容")
		return
	}
	prompt, err := s.saveWorkspacePrompt(name, content)
	if err != nil {
		respondError(w, http.StatusInternalServerError, fmt.Sprintf("保存 Prompt 失败: %v", err))
		return
	}
	promptCopy := prompt
	s.respondAIActionSuccess(w, actionID, "ready", "已保存 Prompt: "+prompt.Name, AIActionResultDTO{
		Kind:      "prompt",
		Operation: "prompt_save",
		Summary:   "已保存 Prompt: " + prompt.Name,
		Prompt:    &promptCopy,
	})
}

func (s *Server) runHarnessCreate(w http.ResponseWriter, actionID string, input *struct {
	Title          string   `json:"title,omitempty"`
	Goal           string   `json:"goal,omitempty"`
	Kind           string   `json:"kind,omitempty"`
	Command        string   `json:"command,omitempty"`
	ExpectedResult string   `json:"expectedResult,omitempty"`
	Priority       string   `json:"priority,omitempty"`
	Source         string   `json:"source,omitempty"`
	FilePath       string   `json:"filePath,omitempty"`
	Content        string   `json:"content,omitempty"`
	Role           string   `json:"role,omitempty"`
	PromptName     string   `json:"promptName,omitempty"`
	PromptContent  string   `json:"promptContent,omitempty"`
	InputText      string   `json:"inputText,omitempty"`
	HarnessCaseId  string   `json:"harnessCaseId,omitempty"`
	Target         string   `json:"target,omitempty"`
	Roles          []string `json:"roles,omitempty"`
	Commands       []string `json:"commands,omitempty"`
	Agents         []string `json:"agents,omitempty"`
	ExecutionID    string   `json:"executionId,omitempty"`
	Reason         string   `json:"reason,omitempty"`
}) {
	title := strings.TrimSpace(input.Title)
	if title == "" {
		title = strings.TrimSpace(input.Goal)
	}
	if title == "" {
		respondError(w, http.StatusBadRequest, "创建评测用例需要 title")
		return
	}
	testCase, err := s.createHarnessCase(WorkspaceHarnessCase{
		Title:          title,
		Goal:           input.Goal,
		Kind:           input.Kind,
		Command:        input.Command,
		ExpectedResult: input.ExpectedResult,
		Source:         input.Source,
		FilePath:       input.FilePath,
		PromptName:     input.PromptName,
		InputText:      input.InputText,
	})
	if err != nil {
		respondError(w, http.StatusInternalServerError, fmt.Sprintf("创建评测用例失败: %v", err))
		return
	}
	testCaseCopy := testCase
	s.respondAIActionSuccess(w, actionID, "ready", "已创建评测用例", AIActionResultDTO{
		Kind:        "harness",
		Operation:   "harness_create",
		Summary:     "已创建评测用例",
		HarnessCase: &testCaseCopy,
	})
}

func (s *Server) runHarnessRun(w http.ResponseWriter, actionID string, input *struct {
	Title          string   `json:"title,omitempty"`
	Goal           string   `json:"goal,omitempty"`
	Kind           string   `json:"kind,omitempty"`
	Command        string   `json:"command,omitempty"`
	ExpectedResult string   `json:"expectedResult,omitempty"`
	Priority       string   `json:"priority,omitempty"`
	Source         string   `json:"source,omitempty"`
	FilePath       string   `json:"filePath,omitempty"`
	Content        string   `json:"content,omitempty"`
	Role           string   `json:"role,omitempty"`
	PromptName     string   `json:"promptName,omitempty"`
	PromptContent  string   `json:"promptContent,omitempty"`
	InputText      string   `json:"inputText,omitempty"`
	HarnessCaseId  string   `json:"harnessCaseId,omitempty"`
	Target         string   `json:"target,omitempty"`
	Roles          []string `json:"roles,omitempty"`
	Commands       []string `json:"commands,omitempty"`
	Agents         []string `json:"agents,omitempty"`
	ExecutionID    string   `json:"executionId,omitempty"`
	Reason         string   `json:"reason,omitempty"`
}) {
	promptName := strings.TrimSpace(input.PromptName)
	inputText := strings.TrimSpace(input.InputText)
	harnessCaseID := strings.TrimSpace(input.HarnessCaseId)
	var result HarnessResult
	var err error
	if harnessCaseID != "" {
		result, err = s.runWorkspaceHarnessCase(harnessCaseID)
	} else if promptName != "" && inputText != "" {
		result, err = s.runWorkspaceHarness(promptName, inputText)
	} else {
		err = fmt.Errorf("需要 harnessCaseId，或同时提供 promptName 和 inputText")
	}
	if err != nil {
		respondError(w, http.StatusInternalServerError, fmt.Sprintf("运行评测失败: %v", err))
		return
	}
	resultCopy := result
	s.respondAIActionSuccess(w, actionID, "ready", result.ID, AIActionResultDTO{
		Kind:       "harness",
		Operation:  "harness_run",
		Summary:    fmt.Sprintf("评测已运行: %s\n%s", result.ID, result.Output),
		HarnessRun: &resultCopy,
	})
}

func (s *Server) runReportGenerate(w http.ResponseWriter, actionID string, input *struct {
	Title          string   `json:"title,omitempty"`
	Goal           string   `json:"goal,omitempty"`
	Kind           string   `json:"kind,omitempty"`
	Command        string   `json:"command,omitempty"`
	ExpectedResult string   `json:"expectedResult,omitempty"`
	Priority       string   `json:"priority,omitempty"`
	Source         string   `json:"source,omitempty"`
	FilePath       string   `json:"filePath,omitempty"`
	Content        string   `json:"content,omitempty"`
	Role           string   `json:"role,omitempty"`
	PromptName     string   `json:"promptName,omitempty"`
	PromptContent  string   `json:"promptContent,omitempty"`
	InputText      string   `json:"inputText,omitempty"`
	HarnessCaseId  string   `json:"harnessCaseId,omitempty"`
	Target         string   `json:"target,omitempty"`
	Roles          []string `json:"roles,omitempty"`
	Commands       []string `json:"commands,omitempty"`
	Agents         []string `json:"agents,omitempty"`
	ExecutionID    string   `json:"executionId,omitempty"`
	Reason         string   `json:"reason,omitempty"`
}) {
	goal := strings.TrimSpace(input.Goal)
	if goal == "" {
		goal = strings.TrimSpace(input.ExpectedResult)
	}
	report, err := s.generateWorkspaceReport(ReportGenerateInput{
		Title:    strings.TrimSpace(input.Title),
		Goal:     goal,
		FilePath: strings.TrimSpace(input.FilePath),
	})
	if err != nil {
		respondError(w, http.StatusInternalServerError, fmt.Sprintf("AI 报告生成失败: %v", err))
		return
	}
	s.respondAIActionSuccess(w, actionID, "ready", "已生成真实 AI 报告", AIActionResultDTO{
		Kind:      "report",
		Operation: "report_generate",
		Summary:   fmt.Sprintf("已生成报告 %s（模型: %s）", report.RelativePath, report.Model),
		FileDiffs: []AIActionFileDiffDTO{{Path: report.RelativePath, Operation: "report", Diff: report.Diff}},
	})
}

func (s *Server) generateWorkspaceReport(input ReportGenerateInput) (ReportGenerateResult, error) {
	title := strings.TrimSpace(input.Title)
	if title == "" {
		title = "AI 安全报告"
	}
	goal := strings.TrimSpace(input.Goal)
	systemPrompt := "你是 Ouroboros 的 AI 安全报告生成器。你只能基于给定的真实工作区状态输出中文 Markdown 安全报告。第一行必须是 Markdown 标题或报告正文标题，直接输出完整报告。禁止输出思考过程、内部推理、自我介绍、开场白、解释说明或“用户要求我”这类内容。报告必须包含摘要、关键发现、执行记录、证据、黑板状态、agent 状态、harness 结果和下一步建议。不要编造不存在的事实，只汇总给定上下文，并明确标注建议。"
	userPrompt := "报告标题: " + title
	if goal != "" {
		userPrompt += "\n补充目标: " + goal
	}
	userPrompt += "\n\n工作区状态:\n" + s.buildWorkspaceSummary()
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()
	result, err := app.GenerateProviderMessage(ctx, s.workspace, app.ProviderGenerationRequest{
		SystemPrompt:         systemPrompt,
		UserPrompt:           userPrompt,
		MaxTokens:            4096,
		RequiredCapabilities: []string{"chat"},
		Stream:               false,
		TimeoutMS:            180000,
	})
	if err != nil {
		return ReportGenerateResult{}, err
	}
	text := cleanGeneratedReportMarkdown(result.Text)
	relativePath := strings.TrimSpace(input.FilePath)
	if relativePath == "" {
		relativePath = fmt.Sprintf("reports/ai-report-%d.md", time.Now().Unix())
	}
	content := fmt.Sprintf("# %s\n\n> 由 Ouroboros AI 基于真实工作区状态生成\n> 模型: %s\n\n%s\n", title, result.Model, text)
	oldContent := ""
	if existing, readErr := ReadWorkspaceFile(s.workspace, relativePath); readErr == nil {
		oldContent = existing.Content
	}
	if _, err := WriteWorkspaceFile(s.workspace, relativePath, content); err != nil {
		return ReportGenerateResult{}, fmt.Errorf("写入报告失败: %w", err)
	}
	return ReportGenerateResult{
		RelativePath: relativePath,
		Model:        result.Model,
		Text:         text,
		Diff:         unifiedLineDiff(relativePath, oldContent, content),
	}, nil
}

func cleanGeneratedReportMarkdown(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return text
	}
	lines := strings.Split(text, "\n")
	start := -1
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") {
			start = i
			break
		}
	}
	if start > 0 {
		return strings.TrimSpace(strings.Join(lines[start:], "\n"))
	}
	return text
}

func (s *Server) runFailureRecover(w http.ResponseWriter, actionID string, input *struct {
	Title          string   `json:"title,omitempty"`
	Goal           string   `json:"goal,omitempty"`
	Kind           string   `json:"kind,omitempty"`
	Command        string   `json:"command,omitempty"`
	ExpectedResult string   `json:"expectedResult,omitempty"`
	Priority       string   `json:"priority,omitempty"`
	Source         string   `json:"source,omitempty"`
	FilePath       string   `json:"filePath,omitempty"`
	Content        string   `json:"content,omitempty"`
	Role           string   `json:"role,omitempty"`
	PromptName     string   `json:"promptName,omitempty"`
	PromptContent  string   `json:"promptContent,omitempty"`
	InputText      string   `json:"inputText,omitempty"`
	HarnessCaseId  string   `json:"harnessCaseId,omitempty"`
	Target         string   `json:"target,omitempty"`
	Roles          []string `json:"roles,omitempty"`
	Commands       []string `json:"commands,omitempty"`
	Agents         []string `json:"agents,omitempty"`
	ExecutionID    string   `json:"executionId,omitempty"`
	Reason         string   `json:"reason,omitempty"`
}) {
	executionID := strings.TrimSpace(input.ExecutionID)
	if executionID == "" {
		respondError(w, http.StatusBadRequest, "失败恢复需要 executionId")
		return
	}
	original, retry, fact, err := s.recoverFailedExecution(executionID, input.Reason, input.Command)
	if err != nil {
		respondError(w, http.StatusInternalServerError, fmt.Sprintf("失败恢复执行失败: %v", err))
		return
	}
	factCopy := fact
	retryCopy := retry
	s.respondAIActionSuccess(w, actionID, "started", fmt.Sprintf("已启动失败恢复执行 %s", retry.ID), AIActionResultDTO{
		Kind:           "failure",
		Operation:      "failure_recover",
		Summary:        fmt.Sprintf("原始执行: %s (状态: %s)\n重试执行: %s", original.ID, original.Status, retry.ID),
		BlackboardItem: &factCopy,
		Execution:      &retryCopy,
	})
}

func (s *Server) recoverFailedExecution(executionID string, reason string, replacement string) (ExecutionDTO, ExecutionDTO, BlackboardFact, error) {
	original, err := ReadExecution(s.workspace, executionID)
	if err != nil {
		return ExecutionDTO{}, ExecutionDTO{}, BlackboardFact{}, err
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		if logs, logErr := ReadExecutionLogs(s.workspace, executionID); logErr == nil {
			reason = truncateForAI(logs.Content, 500)
		}
	}
	retry, err := s.RetryExecution(executionID)
	if err != nil && strings.TrimSpace(replacement) != "" {
		retry, err = s.StartExecution(ExecutionCreateRequestDTO{
			Command: "bash",
			Args:    []string{"-c", strings.TrimSpace(replacement)},
			RetryOf: executionID,
		})
	}
	if err != nil {
		return ExecutionDTO{}, ExecutionDTO{}, BlackboardFact{}, err
	}
	fact, err := s.appendBlackboardFact("failure", fmt.Sprintf("失败恢复: %s -> %s (原因: %s)", executionID, retry.ID, reason))
	if err != nil {
		return ExecutionDTO{}, ExecutionDTO{}, BlackboardFact{}, err
	}
	return original, retry, fact, nil
}

func (s *Server) runSwarmWorkflow(w http.ResponseWriter, actionID string, input *struct {
	Title          string   `json:"title,omitempty"`
	Goal           string   `json:"goal,omitempty"`
	Kind           string   `json:"kind,omitempty"`
	Command        string   `json:"command,omitempty"`
	ExpectedResult string   `json:"expectedResult,omitempty"`
	Priority       string   `json:"priority,omitempty"`
	Source         string   `json:"source,omitempty"`
	FilePath       string   `json:"filePath,omitempty"`
	Content        string   `json:"content,omitempty"`
	Role           string   `json:"role,omitempty"`
	PromptName     string   `json:"promptName,omitempty"`
	PromptContent  string   `json:"promptContent,omitempty"`
	InputText      string   `json:"inputText,omitempty"`
	HarnessCaseId  string   `json:"harnessCaseId,omitempty"`
	Target         string   `json:"target,omitempty"`
	Roles          []string `json:"roles,omitempty"`
	Commands       []string `json:"commands,omitempty"`
	Agents         []string `json:"agents,omitempty"`
	ExecutionID    string   `json:"executionId,omitempty"`
	Reason         string   `json:"reason,omitempty"`
}) {
	goal := strings.TrimSpace(input.Goal)
	if goal == "" {
		respondError(w, http.StatusBadRequest, "多智能体调度需要 goal")
		return
	}
	title := strings.TrimSpace(input.Title)
	if title == "" {
		title = goal
	}
	roles := []string{"Planner", "Operator", "Evidence Analyst"}
	commands := []string{"echo goal=" + goal, "pwd", "ls -la"}
	if len(input.Roles) > 0 {
		roles = input.Roles
	}
	if len(input.Commands) > 0 {
		commands = input.Commands
	}
	if len(input.Agents) > 0 {
		parsed := parseSwarmAgentPairs(input.Agents)
		if len(parsed) > 0 {
			roles = make([]string, 0, len(parsed))
			commands = make([]string, 0, len(parsed))
			for role, command := range parsed {
				roles = append(roles, role)
				commands = append(commands, command)
			}
		}
	}
	run := SwarmWorkflowRun{
		ID:          fmt.Sprintf("swarm-%d", time.Now().UnixNano()),
		Title:       title,
		Goal:        goal,
		Status:      "running",
		CreatedAt:   time.Now().Unix(),
		UpdatedAt:   time.Now().Unix(),
		AgentOutput: map[string]string{},
		Inputs: SwarmWorkflowInputSnapshot{
			Title:    title,
			Goal:     goal,
			Roles:    append([]string{}, roles...),
			Commands: append([]string{}, commands...),
		},
	}
	run.Timeline = append(run.Timeline, SwarmWorkflowStep{Step: "started", Detail: title, CreatedAt: time.Now().Unix()})
	failures := []string{}
	for i, role := range roles {
		role = strings.TrimSpace(role)
		if role == "" {
			continue
		}
		cmd := ""
		if i < len(commands) {
			cmd = strings.TrimSpace(commands[i])
		}
		agentGoal := goal
		task, err := s.createAgentTask(role, agentGoal)
		if err != nil {
			failures = append(failures, fmt.Sprintf("%s: %v", role, err))
			continue
		}
		run.Tasks = append(run.Tasks, task)
		run.Timeline = append(run.Timeline, SwarmWorkflowStep{Step: "agent_created", Detail: task.ID, CreatedAt: time.Now().Unix()})
		env := map[string]string{
			"OURO_SWARM_RUN_ID":  run.ID,
			"OURO_AGENT_ROLE":    role,
			"OURO_AGENT_GOAL":    agentGoal,
			"OURO_AGENT_TASK_ID": task.ID,
		}
		execution, execErr := s.StartExecution(ExecutionCreateRequestDTO{
			Command: "bash",
			Args:    []string{"-c", cmd},
			Env:     env,
		})
		if execErr != nil {
			failures = append(failures, fmt.Sprintf("%s: %v", role, execErr))
			run.Timeline = append(run.Timeline, SwarmWorkflowStep{Step: "agent_failed", Detail: task.ID + ": " + execErr.Error(), CreatedAt: time.Now().Unix()})
			continue
		}
		run.Executions = append(run.Executions, execution)
		if err := s.markAgentTaskRunning(task.ID, cmd, execution.ID); err != nil {
			run.Timeline = append(run.Timeline, SwarmWorkflowStep{Step: "agent_status_warning", Detail: err.Error(), CreatedAt: time.Now().Unix()})
		}
		run.Timeline = append(run.Timeline, SwarmWorkflowStep{Step: "agent_started", Detail: execution.ID, CreatedAt: time.Now().Unix()})
		_, workerErr := app.RegisterRuntimeWorker(s.workspace, app.RuntimeWorkerArtifact{
			WorkerID:     "swarm-" + run.ID + "-" + task.ID,
			Mode:         "local",
			Status:       "busy",
			Capabilities: []string{"ai", "execution", "agent"},
			Labels: map[string]string{
				"swarm_run_id":  run.ID,
				"agent_role":    role,
				"agent_task_id": task.ID,
				"execution_id":  execution.ID,
			},
		}, app.RuntimeWorkerRegistrationOptions{})
		if workerErr != nil {
			run.Timeline = append(run.Timeline, SwarmWorkflowStep{Step: "worker_warning", Detail: workerErr.Error(), CreatedAt: time.Now().Unix()})
		}
	}
	if _, err := s.appendBlackboardFact("swarm", fmt.Sprintf("多智能体调度已启动: %s (%s)", title, goal)); err != nil {
		run.Timeline = append(run.Timeline, SwarmWorkflowStep{Step: "blackboard_warning", Detail: err.Error(), CreatedAt: time.Now().Unix()})
	} else {
		run.Timeline = append(run.Timeline, SwarmWorkflowStep{Step: "blackboard_fact", Detail: "已写入黑板", CreatedAt: time.Now().Unix()})
	}
	if len(failures) > 0 {
		run.Status = "degraded"
		run.Summary = "多智能体调度已启动，部分任务失败:\n" + strings.Join(failures, "\n")
	} else {
		run.Status = "running"
		run.Summary = "多智能体调度已启动，所有任务已真实分派"
	}
	run.Tasks = s.refreshAgentTaskStatuses(run.Tasks)
	run.UpdatedAt = time.Now().Unix()
	if err := s.appendSwarmWorkflowRun(run); err != nil {
		respondError(w, http.StatusInternalServerError, fmt.Sprintf("保存多智能体调度记录失败: %v", err))
		return
	}
	runCopy := run
	s.respondAIActionSuccess(w, actionID, run.Status, run.Summary, AIActionResultDTO{
		Kind:      "swarm",
		Operation: "swarm_run",
		Summary:   run.Summary,
		SwarmRun:  &runCopy,
	})
}

func parseSwarmAgentPairs(specs []string) map[string]string {
	parsed := map[string]string{}
	for _, spec := range specs {
		parts := strings.SplitN(strings.TrimSpace(spec), ":", 2)
		if len(parts) != 2 {
			continue
		}
		if role := strings.TrimSpace(parts[0]); role != "" {
			parsed[role] = strings.TrimSpace(parts[1])
		}
	}
	return parsed
}

func (s *Server) refreshAgentTaskStatuses(tasks []AgentTaskRecord) []AgentTaskRecord {
	byID := map[string]AgentTaskRecord{}
	for _, task := range s.loadAgentTasks() {
		byID[task.ID] = task
	}
	for i := range tasks {
		if updated, ok := byID[tasks[i].ID]; ok {
			tasks[i] = updated
		}
	}
	return tasks
}

func (s *Server) swarmWorkflowRunsPath() string {
	return filepath.Join(s.workspace, "runtime", "swarm-workflows.jsonl")
}

func (s *Server) appendSwarmWorkflowRun(run SwarmWorkflowRun) error {
	runtimeDir := filepath.Join(s.workspace, "runtime")
	os.MkdirAll(runtimeDir, 0o755)
	file, err := os.OpenFile(s.swarmWorkflowRunsPath(), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()
	data, err := json.Marshal(run)
	if err != nil {
		return err
	}
	_, err = file.Write(append(data, '\n'))
	return err
}

func (s *Server) loadSwarmWorkflowRuns() []SwarmWorkflowRun {
	path := s.swarmWorkflowRunsPath()
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return []SwarmWorkflowRun{}
	}
	file, err := os.Open(path)
	if err != nil {
		return []SwarmWorkflowRun{}
	}
	defer file.Close()
	var runs []SwarmWorkflowRun
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
	for scanner.Scan() {
		var run SwarmWorkflowRun
		if json.Unmarshal(scanner.Bytes(), &run) == nil {
			runs = append(runs, run)
		}
	}
	return runs
}

func (s *Server) runOrchestratorAI(w http.ResponseWriter, r *http.Request, actionID string, input *struct {
	Title          string   `json:"title,omitempty"`
	Goal           string   `json:"goal,omitempty"`
	Kind           string   `json:"kind,omitempty"`
	Command        string   `json:"command,omitempty"`
	ExpectedResult string   `json:"expectedResult,omitempty"`
	Priority       string   `json:"priority,omitempty"`
	Source         string   `json:"source,omitempty"`
	FilePath       string   `json:"filePath,omitempty"`
	Content        string   `json:"content,omitempty"`
	Role           string   `json:"role,omitempty"`
	PromptName     string   `json:"promptName,omitempty"`
	PromptContent  string   `json:"promptContent,omitempty"`
	InputText      string   `json:"inputText,omitempty"`
	HarnessCaseId  string   `json:"harnessCaseId,omitempty"`
	Target         string   `json:"target,omitempty"`
	Roles          []string `json:"roles,omitempty"`
	Commands       []string `json:"commands,omitempty"`
	Agents         []string `json:"agents,omitempty"`
	ExecutionID    string   `json:"executionId,omitempty"`
	Reason         string   `json:"reason,omitempty"`
}) {
	if strings.TrimSpace(input.Title) == "" || strings.TrimSpace(input.Goal) == "" ||
		strings.TrimSpace(input.Kind) == "" || strings.TrimSpace(input.Command) == "" ||
		strings.TrimSpace(input.ExpectedResult) == "" {
		respondError(w, http.StatusBadRequest, "title, goal, kind, command, expectedResult 为必填参数")
		return
	}

	systemPrompt := "你是 Ouroboros IDE 的 AI 编排助手。请根据给定目标，输出简明的中文执行计划。不要输出多余内容，只输出计划本身。"
	userPrompt := fmt.Sprintf(
		"标题: %s\n目标: %s\n类型: %s\n命令: %s\n期望结果: %s\n优先级: %s\n来源: %s\n文件: %s\n\n请生成执行计划。",
		input.Title, input.Goal, input.Kind, input.Command, input.ExpectedResult,
		input.Priority, input.Source, input.FilePath,
	)

	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()

	result, err := app.GenerateProviderMessage(ctx, s.workspace, app.ProviderGenerationRequest{
		SystemPrompt:         systemPrompt,
		UserPrompt:           userPrompt,
		MaxTokens:            1024,
		RequiredCapabilities: []string{"chat"},
		Stream:               false,
	})
	if err != nil {
		log.Printf("AI provider generation failed: %v", err)
		respondError(w, http.StatusInternalServerError, fmt.Sprintf("AI模型调用失败: %v", err))
		return
	}

	now := time.Now().Unix()
	runID := fmt.Sprintf("orchestrator-run-%d", now)

	execution := ExecutionDTO{}
	started := false
	if strings.TrimSpace(input.Command) != "" {
		if ex, execErr := s.StartExecution(ExecutionCreateRequestDTO{Command: "bash", Args: []string{"-c", input.Command}}); execErr == nil {
			execution = ex
			started = true
		} else {
			log.Printf("orchestrator.run failed to start execution: %v", execErr)
		}
	}

	actionStatus := "started"
	description := fmt.Sprintf("已使用 %s 模型生成执行计划并启动真实执行", result.Model)
	if !started {
		actionStatus = "failed"
		description = fmt.Sprintf("已使用 %s 模型生成执行计划，但真实执行启动失败", result.Model)
	}
	status := "started"
	if !started {
		status = "failed"
	}
	summary := result.Text
	operation := "ai_generate"
	if started {
		summary = result.Text + "\n\n已启动真实执行: " + execution.ID
		operation = "ai_generate_execute"
	}

	respondJSON(w, http.StatusOK, AIActionRunResponseDTO{
		Action: AIActionResponseDTO{
			ID:           "orchestrator.run",
			Kind:         "orchestrator",
			Label:        "AI 编排执行",
			Status:       actionStatus,
			Description:  description,
			Capabilities: []string{"chat", "reasoning"},
		},
		Result: AIActionResultDTO{
			Kind:      "orchestrator_run",
			Operation: operation,
			Summary:   summary,
			OrchestratorRun: &struct {
				ID             string `json:"id"`
				ExecutionID    string `json:"executionId,omitempty"`
				Title          string `json:"title"`
				Goal           string `json:"goal"`
				Kind           string `json:"kind"`
				Command        string `json:"command"`
				ExpectedResult string `json:"expectedResult"`
				Priority       string `json:"priority,omitempty"`
				Source         string `json:"source,omitempty"`
				FilePath       string `json:"filePath,omitempty"`
				Status         string `json:"status"`
				CreatedAt      int64  `json:"createdAt"`
			}{
				ID:             runID,
				ExecutionID:    execution.ID,
				Title:          input.Title,
				Goal:           input.Goal,
				Kind:           input.Kind,
				Command:        input.Command,
				ExpectedResult: input.ExpectedResult,
				Priority:       input.Priority,
				Source:         input.Source,
				FilePath:       input.FilePath,
				Status:         status,
				CreatedAt:      now,
			},
		},
	})
}
