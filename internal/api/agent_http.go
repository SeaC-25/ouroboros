package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ouroboros/ouroboros/internal/app"
)

type agentProfileRequest struct {
	ID              string   `json:"id,omitempty"`
	Name            string   `json:"name"`
	Role            string   `json:"role"`
	Description     string   `json:"description,omitempty"`
	Instructions    string   `json:"instructions,omitempty"`
	Capabilities    []string `json:"capabilities,omitempty"`
	ProviderName    string   `json:"providerName,omitempty"`
	Model           string   `json:"model,omitempty"`
	PromptID        string   `json:"promptId,omitempty"`
	PromptVersionID string   `json:"promptVersionId,omitempty"`
}

func (s *Server) handleAgentProfiles(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/api/agents/profiles" {
		id := strings.TrimPrefix(r.URL.Path, "/api/agents/profiles/")
		switch r.Method {
		case http.MethodPatch:
			s.updateAgentProfileHTTP(w, r, id)
		case http.MethodDelete:
			s.deleteAgentProfileHTTP(w, r, id)
		default:
			respondError(w, http.StatusMethodNotAllowed, "不支持的智能体 profile 方法")
		}
		return
	}
	switch r.Method {
	case http.MethodGet:
		respondJSON(w, http.StatusOK, map[string]any{"profiles": s.loadWorkspaceAgentProfiles()})
	case http.MethodPost:
		s.createAgentProfileHTTP(w, r)
	default:
		respondError(w, http.StatusMethodNotAllowed, "不支持的智能体 profile 方法")
	}
}

func (s *Server) createAgentProfileHTTP(w http.ResponseWriter, r *http.Request) {
	var request agentProfileRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		respondError(w, http.StatusBadRequest, "解析智能体 profile 失败")
		return
	}
	request.Name, request.Role = strings.TrimSpace(request.Name), strings.TrimSpace(request.Role)
	if request.Name == "" || request.Role == "" {
		respondError(w, http.StatusBadRequest, "智能体 profile 名称和角色不能为空")
		return
	}
	if request.Capabilities == nil {
		request.Capabilities = []string{}
	}
	now := time.Now().Unix()
	profile := AgentProfileRecord{ID: fmt.Sprintf("profile-%d", time.Now().UnixNano()), Name: request.Name, Role: request.Role, Description: request.Description, Instructions: request.Instructions, Capabilities: request.Capabilities, ProviderName: request.ProviderName, Model: request.Model, PromptID: request.PromptID, PromptVersionID: request.PromptVersionID, CreatedAt: now, UpdatedAt: now}
	profiles := append(s.loadWorkspaceAgentProfiles(), profile)
	if err := s.writeAgentProfiles(profiles); err != nil {
		respondError(w, http.StatusInternalServerError, "保存智能体 profile 失败")
		return
	}
	respondJSON(w, http.StatusOK, profile)
}

func (s *Server) updateAgentProfileHTTP(w http.ResponseWriter, r *http.Request, id string) {
	profiles := s.loadWorkspaceAgentProfiles()
	var request agentProfileRequest
	var raw map[string]json.RawMessage
	body, err := io.ReadAll(r.Body)
	if err != nil || json.Unmarshal(body, &raw) != nil || json.Unmarshal(body, &request) != nil {
		respondError(w, http.StatusBadRequest, "解析智能体 profile 失败")
		return
	}
	for index := range profiles {
		if profiles[index].ID != id {
			continue
		}
		if strings.TrimSpace(request.Name) != "" {
			profiles[index].Name = strings.TrimSpace(request.Name)
		}
		if strings.TrimSpace(request.Role) != "" {
			profiles[index].Role = strings.TrimSpace(request.Role)
		}
		if _, ok := raw["description"]; ok {
			profiles[index].Description = request.Description
		}
		if _, ok := raw["instructions"]; ok {
			profiles[index].Instructions = request.Instructions
		}
		if _, ok := raw["capabilities"]; ok {
			profiles[index].Capabilities = request.Capabilities
		}
		if _, ok := raw["providerName"]; ok {
			profiles[index].ProviderName = request.ProviderName
		}
		if _, ok := raw["model"]; ok {
			profiles[index].Model = request.Model
		}
		if _, ok := raw["promptId"]; ok {
			profiles[index].PromptID = request.PromptID
		}
		if _, ok := raw["promptVersionId"]; ok {
			profiles[index].PromptVersionID = request.PromptVersionID
		}
		profiles[index].UpdatedAt = time.Now().Unix()
		if err := s.writeAgentProfiles(profiles); err != nil {
			respondError(w, http.StatusInternalServerError, "保存智能体 profile 失败")
			return
		}
		respondJSON(w, http.StatusOK, profiles[index])
		return
	}
	respondError(w, http.StatusNotFound, "智能体 profile 不存在")
}

func (s *Server) deleteAgentProfileHTTP(w http.ResponseWriter, r *http.Request, id string) {
	profiles := s.loadWorkspaceAgentProfiles()
	filtered := make([]AgentProfileRecord, 0, len(profiles))
	found := false
	for _, profile := range profiles {
		if profile.ID == id {
			found = true
			continue
		}
		filtered = append(filtered, profile)
	}
	if !found {
		respondError(w, http.StatusNotFound, "智能体 profile 不存在")
		return
	}
	if err := s.writeAgentProfiles(filtered); err != nil {
		respondError(w, http.StatusInternalServerError, "删除智能体 profile 失败")
		return
	}
	respondJSON(w, http.StatusOK, map[string]any{"deleted": id})
}

func (s *Server) writeAgentProfiles(profiles []AgentProfileRecord) error {
	if err := os.MkdirAll(filepath.Dir(s.agentProfilesPath()), 0o755); err != nil {
		return err
	}
	file, err := os.OpenFile(s.agentProfilesPath(), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()
	for _, profile := range profiles {
		data, err := json.Marshal(profile)
		if err != nil {
			return err
		}
		if _, err := file.Write(append(data, '\n')); err != nil {
			return err
		}
	}
	return nil
}

func (s *Server) handleAgentTasksHTTP(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/agents/tasks")
	if path == "" || path == "/" {
		if r.Method == http.MethodGet {
			respondJSON(w, http.StatusOK, map[string]any{"tasks": s.projectAgentTasks()})
			return
		}
		if r.Method == http.MethodPost {
			s.createAgentTaskHTTP(w, r)
			return
		}
	}
	if strings.HasPrefix(path, "/") {
		parts := strings.Split(strings.Trim(path, "/"), "/")
		if len(parts) == 1 && r.Method == http.MethodPatch {
			s.updateAgentTaskHTTP(w, r, parts[0])
			return
		}
		if len(parts) == 2 && parts[1] == "events" && r.Method == http.MethodPost {
			s.createAgentTaskEventHTTP(w, r, parts[0])
			return
		}
		if len(parts) == 2 && parts[1] == "run" && r.Method == http.MethodPost {
			s.runAgentTaskProviderHTTP(w, r, parts[0])
			return
		}
	}
	respondError(w, http.StatusNotFound, "智能体任务接口不存在")
}

func (s *Server) projectAgentTasks() []map[string]any {
	tasks := s.loadAgentTasks()
	rows := make([]map[string]any, 0, len(tasks))
	for _, task := range tasks {
		rows = append(rows, map[string]any{"id": task.ID, "title": firstNonEmpty(task.Title, task.Goal), "goal": task.Goal, "role": task.Role, "status": task.Status, "source": task.Source, "priority": task.Priority, "createdAt": task.CreatedAt, "updatedAt": task.UpdatedAt, "executionId": task.ExecutionID, "agentId": firstNonEmpty(task.AgentID, task.WorkerID)})
	}
	return rows
}

func (s *Server) createAgentTaskHTTP(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Title    string `json:"title"`
		Goal     string `json:"goal"`
		Role     string `json:"role"`
		Priority string `json:"priority"`
		Source   string `json:"source"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		respondError(w, http.StatusBadRequest, "解析智能体任务失败")
		return
	}
	role := strings.TrimSpace(request.Role)
	if role == "" {
		role = "operator"
	}
	task, err := s.createAgentTask(role, strings.TrimSpace(request.Goal))
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	task.Title = firstNonEmpty(request.Title, task.Goal)
	task.Priority = request.Priority
	task.Source = request.Source
	if err := s.writeAgentTasksWithRecord(task); err != nil {
		respondError(w, http.StatusInternalServerError, "保存智能体任务失败")
		return
	}
	row := map[string]any{"id": task.ID, "title": task.Title, "goal": task.Goal, "role": task.Role, "status": task.Status, "priority": task.Priority, "source": task.Source, "createdAt": task.CreatedAt, "updatedAt": task.UpdatedAt}
	respondJSON(w, http.StatusOK, row)
}

func (s *Server) writeAgentTasksWithRecord(updated AgentTaskRecord) error {
	tasks := s.loadAgentTasks()
	for i := range tasks {
		if tasks[i].ID == updated.ID {
			tasks[i] = updated
			return s.writeAgentTasks(tasks)
		}
	}
	return fmt.Errorf("智能体任务不存在: %s", updated.ID)
}

func (s *Server) updateAgentTaskHTTP(w http.ResponseWriter, r *http.Request, id string) {
	tasks := s.loadAgentTasks()
	var request map[string]any
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		respondError(w, http.StatusBadRequest, "解析智能体任务失败")
		return
	}
	for i := range tasks {
		if tasks[i].ID != id {
			continue
		}
		if value, ok := request["title"].(string); ok && strings.TrimSpace(value) != "" {
			tasks[i].Title = strings.TrimSpace(value)
		}
		if value, ok := request["goal"].(string); ok && strings.TrimSpace(value) != "" {
			tasks[i].Goal = strings.TrimSpace(value)
		}
		if value, ok := request["role"].(string); ok && strings.TrimSpace(value) != "" {
			tasks[i].Role = strings.TrimSpace(value)
		}
		if value, ok := request["status"].(string); ok && strings.TrimSpace(value) != "" {
			tasks[i].Status = strings.TrimSpace(value)
		}
		if value, ok := request["priority"].(string); ok {
			tasks[i].Priority = strings.TrimSpace(value)
		}
		if value, ok := request["source"].(string); ok {
			tasks[i].Source = strings.TrimSpace(value)
		}
		tasks[i].UpdatedAt = time.Now().Unix()
		if err := s.writeAgentTasks(tasks); err != nil {
			respondError(w, http.StatusInternalServerError, "保存智能体任务失败")
			return
		}
		respondJSON(w, http.StatusOK, map[string]any{"id": tasks[i].ID, "title": firstNonEmpty(tasks[i].Title, tasks[i].Goal), "goal": tasks[i].Goal, "role": tasks[i].Role, "status": tasks[i].Status, "priority": tasks[i].Priority, "source": tasks[i].Source, "createdAt": tasks[i].CreatedAt, "updatedAt": tasks[i].UpdatedAt, "executionId": tasks[i].ExecutionID, "agentId": firstNonEmpty(tasks[i].AgentID, tasks[i].WorkerID)})
		return
	}
	respondError(w, http.StatusNotFound, "智能体任务不存在")
}

type agentTaskEventRecord struct {
	ID        string `json:"id"`
	TaskID    string `json:"taskId"`
	Kind      string `json:"kind"`
	Status    string `json:"status,omitempty"`
	Title     string `json:"title,omitempty"`
	Summary   string `json:"summary,omitempty"`
	Detail    string `json:"detail,omitempty"`
	CreatedAt int64  `json:"createdAt"`
}

func (s *Server) createAgentTaskEventHTTP(w http.ResponseWriter, r *http.Request, taskID string) {
	var request agentTaskEventRecord
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		respondError(w, http.StatusBadRequest, "解析智能体事件失败")
		return
	}
	if request.Kind == "" {
		request.Kind = "note"
	}
	request.ID, request.TaskID, request.CreatedAt = fmt.Sprintf("agent-event-%d", time.Now().UnixNano()), taskID, time.Now().Unix()
	found := false
	for _, task := range s.loadAgentTasks() {
		if task.ID == taskID {
			found = true
			break
		}
	}
	if !found {
		respondError(w, http.StatusNotFound, "智能体任务不存在")
		return
	}
	if err := os.MkdirAll(filepath.Dir(s.agentTaskEventsPath()), 0o755); err != nil {
		respondError(w, http.StatusInternalServerError, "保存智能体事件失败")
		return
	}
	file, err := os.OpenFile(s.agentTaskEventsPath(), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "保存智能体事件失败")
		return
	}
	defer file.Close()
	data, _ := json.Marshal(request)
	if _, err := file.Write(append(data, '\n')); err != nil {
		respondError(w, http.StatusInternalServerError, "保存智能体事件失败")
		return
	}
	respondJSON(w, http.StatusOK, request)
}

func (s *Server) agentTaskEventsPath() string {
	return filepath.Join(s.workspace, "runtime", "agent-task-events.jsonl")
}

func (s *Server) runAgentTaskProviderHTTP(w http.ResponseWriter, r *http.Request, taskID string) {
	tasks := s.loadAgentTasks()
	var task *AgentTaskRecord
	for index := range tasks {
		if tasks[index].ID == taskID {
			task = &tasks[index]
			break
		}
	}
	if task == nil {
		respondError(w, http.StatusNotFound, "智能体任务不存在")
		return
	}
	var request struct {
		ProviderName string `json:"providerName"`
		Model        string `json:"model"`
		MaxTokens    int    `json:"maxTokens"`
		Source       string `json:"source"`
	}
	_ = json.NewDecoder(r.Body).Decode(&request)
	started := time.Now().UTC()
	result, err := app.GenerateProviderMessage(context.Background(), s.workspace, app.ProviderGenerationRequest{
		SystemPrompt:         "你是 Ouroboros 智能体执行器。只基于真实工作区上下文完成任务，输出简洁中文结果，不要编造事实。",
		UserPrompt:           task.Goal,
		MaxTokens:            request.MaxTokens,
		RequiredCapabilities: []string{"chat"},
		Stream:               false,
	})
	runID := fmt.Sprintf("agent-provider-run-%d", started.UnixNano())
	if err != nil {
		task.Status, task.UpdatedAt = "failed", time.Now().Unix()
		_ = s.writeAgentTasks(tasks)
		respondError(w, http.StatusBadGateway, fmt.Sprintf("智能体 provider 执行失败: %v", err))
		return
	}
	task.Status, task.UpdatedAt = "completed", time.Now().Unix()
	if err := s.writeAgentTasks(tasks); err != nil {
		respondError(w, http.StatusInternalServerError, "保存智能体执行状态失败")
		return
	}
	event := agentTaskEventRecord{ID: "agent-event-" + fmt.Sprintf("%d", time.Now().UnixNano()), TaskID: task.ID, Kind: "provider_run", Status: "completed", Title: "智能体 provider 执行", Summary: result.Text, CreatedAt: time.Now().Unix()}
	if err := s.appendAgentTaskEvent(event); err != nil {
		respondError(w, http.StatusInternalServerError, "保存智能体执行事件失败")
		return
	}
	respondJSON(w, http.StatusOK, map[string]any{
		"runId": runID, "status": "completed", "output": result.Text,
		"startedAt": started.Format(time.RFC3339Nano), "completedAt": time.Now().UTC().Format(time.RFC3339Nano),
		"durationMs": time.Since(started).Milliseconds(), "providerName": result.ProviderName, "model": result.Model,
		"inputTokens": result.InputTokens, "outputTokens": result.OutputTokens, "totalTokens": result.TotalTokens,
		"estimatedCostUsd": result.EstimatedCostUSD, "estimatedCostAvailable": result.EstimatedCostAvailable,
		"eventlogRunId": runID, "task": map[string]any{"id": task.ID, "title": firstNonEmpty(task.Title, task.Goal), "goal": task.Goal, "role": task.Role, "status": task.Status, "createdAt": task.CreatedAt, "updatedAt": task.UpdatedAt},
		"event": event,
	})
}

func (s *Server) appendAgentTaskEvent(event agentTaskEventRecord) error {
	if err := os.MkdirAll(filepath.Dir(s.agentTaskEventsPath()), 0o755); err != nil {
		return err
	}
	file, err := os.OpenFile(s.agentTaskEventsPath(), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}
	_, err = file.Write(append(data, '\n'))
	return err
}
