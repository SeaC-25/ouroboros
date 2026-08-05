package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
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
		Title          string `json:"title,omitempty"`
		Goal           string `json:"goal,omitempty"`
		Kind           string `json:"kind,omitempty"`
		Command        string `json:"command,omitempty"`
		ExpectedResult string `json:"expectedResult,omitempty"`
		Priority       string `json:"priority,omitempty"`
		Source         string `json:"source,omitempty"`
		FilePath       string `json:"filePath,omitempty"`
	} `json:"input,omitempty"`
	Arguments string `json:"arguments,omitempty"`
}

// AIActionResultDTO 匹配前端 AIActionResult
type AIActionResultDTO struct {
	Kind            string `json:"kind"`
	Operation       string `json:"operation,omitempty"`
	Summary         string `json:"summary,omitempty"`
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
}

// AIActionRunResponseDTO 匹配前端 AIActionRunResponse
type AIActionRunResponseDTO struct {
	Action AIActionResponseDTO `json:"action"`
	Result AIActionResultDTO   `json:"result"`
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

	if strings.TrimSpace(request.ActionID) != "orchestrator.run" {
		respondError(w, http.StatusBadRequest, fmt.Sprintf("未知的AI动作: %s", request.ActionID))
		return
	}

	if request.Input == nil {
		respondError(w, http.StatusBadRequest, "缺少AI动作输入参数")
		return
	}
	input := request.Input
	if strings.TrimSpace(input.Title) == "" || strings.TrimSpace(input.Goal) == "" ||
		strings.TrimSpace(input.Kind) == "" || strings.TrimSpace(input.Command) == "" ||
		strings.TrimSpace(input.ExpectedResult) == "" {
		respondError(w, http.StatusBadRequest, "title, goal, kind, command, expectedResult 为必填参数")
		return
	}

	// 构造给真实 AI provider 的 prompt
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

	action := AIActionResponseDTO{
		ID:           "orchestrator.run",
		Kind:         "orchestrator",
		Label:        "AI 编排执行",
		Status:       "started",
		Description:  fmt.Sprintf("已使用 %s 模型生成执行计划并启动真实执行", result.Model),
		Capabilities: []string{"chat", "reasoning"},
	}
	if !started {
		action.Status = "failed"
		action.Description = fmt.Sprintf("已使用 %s 模型生成执行计划，但真实执行启动失败", result.Model)
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
		Action: action,
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
