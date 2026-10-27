package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// handleAIBlackboard 返回工作区中真实持久化的黑板条目。
func (s *Server) handleAIBlackboard(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/ai/blackboard")
	if path == "" || path == "/" {
		switch r.Method {
		case http.MethodGet:
			s.respondBlackboardQuery(w, r)
		case http.MethodPost:
			s.createBlackboardHTTP(w, r)
		default:
			respondError(w, http.StatusMethodNotAllowed, "不支持的黑板请求方法")
		}
		return
	}
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) == 1 && parts[0] == "observations" && r.Method == http.MethodGet {
		s.respondBlackboardObservations(w, r)
		return
	}
	if len(parts) == 2 && parts[0] == "items" && r.Method == http.MethodPatch {
		s.updateBlackboardHTTP(w, r, parts[1])
		return
	}
	if len(parts) == 2 && parts[0] == "items" && parts[1] == "batch" && r.Method == http.MethodPost {
		s.batchUpdateBlackboardHTTP(w, r)
		return
	}
	if len(parts) == 3 && parts[0] == "items" && parts[2] == "dependencies" && r.Method == http.MethodPost {
		s.linkBlackboardDependenciesHTTP(w, r, parts[1])
		return
	}
	respondError(w, http.StatusNotFound, "黑板接口不存在")
}

// handleBlackboardObservations 是 GET /api/blackboard/observations 的入口
// （前端契约端点），与 /api/ai/blackboard/observations 同一实现。
func (s *Server) handleBlackboardObservations(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		respondError(w, http.StatusMethodNotAllowed, "仅支持读取原始观测")
		return
	}
	s.respondBlackboardObservations(w, r)
}

func blackboardItemDTO(fact BlackboardFact) map[string]any {
	status, priority := fact.Status, fact.Priority
	if status == "" {
		status = "open"
	}
	if priority == "" {
		priority = "normal"
	}
	updatedAt := fact.UpdatedAt
	if updatedAt == 0 {
		updatedAt = fact.CreatedAt
	}
	return map[string]any{"id": fact.ID, "kind": fact.Kind, "title": fact.Statement, "content": fact.Statement, "status": status, "priority": priority, "source": fact.Source, "dependencyIds": fact.DependencyIDs, "createdAt": fact.CreatedAt, "updatedAt": updatedAt}
}

// blackboardObservationDTO 是原始观测的对外投影：payload 原样透传给前端，
// 不做任何解释/判定。
func blackboardObservationDTO(observation BlackboardObservation) map[string]any {
	return map[string]any{"id": observation.ID, "kind": observation.Kind, "target": observation.Target, "payload": observation.Payload, "source": observation.Source, "createdAt": observation.CreatedAt}
}

// respondBlackboardObservations 返回工作区中真实持久化的原始观测列表
// （GET /api/ai/blackboard/observations），支持 kind/target/source/q 过滤与 limit。
func (s *Server) respondBlackboardObservations(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	observations := s.loadBlackboardObservations()
	items := make([]map[string]any, 0, len(observations))
	for _, observation := range observations {
		if query.Get("kind") != "" && query.Get("kind") != observation.Kind || query.Get("target") != "" && query.Get("target") != observation.Target || query.Get("source") != "" && query.Get("source") != observation.Source {
			continue
		}
		if q := strings.ToLower(strings.TrimSpace(query.Get("q"))); q != "" && !strings.Contains(strings.ToLower(observation.Payload), q) {
			continue
		}
		items = append(items, blackboardObservationDTO(observation))
	}
	if rawLimit := strings.TrimSpace(query.Get("limit")); rawLimit != "" {
		if limit, err := strconv.Atoi(rawLimit); err == nil && limit >= 0 && limit < len(items) {
			items = items[len(items)-limit:]
		}
	}
	respondJSON(w, http.StatusOK, map[string]any{"path": filepath.ToSlash(s.observationsPath()), "observations": items, "count": len(items)})
}

func (s *Server) respondBlackboardQuery(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	facts := s.loadBlackboardFacts()
	items := make([]map[string]any, 0, len(facts))
	for _, fact := range facts {
		status, priority := fact.Status, fact.Priority
		if status == "" {
			status = "open"
		}
		if priority == "" {
			priority = "normal"
		}
		if query.Get("status") != "" && query.Get("status") != status || query.Get("priority") != "" && query.Get("priority") != priority || query.Get("kind") != "" && query.Get("kind") != fact.Kind || query.Get("source") != "" && query.Get("source") != fact.Source {
			continue
		}
		if q := strings.ToLower(strings.TrimSpace(query.Get("q"))); q != "" && !strings.Contains(strings.ToLower(fact.Statement), q) {
			continue
		}
		items = append(items, blackboardItemDTO(fact))
	}
	respondJSON(w, http.StatusOK, map[string]any{"path": filepath.ToSlash(s.blackboardPath()), "items": items})
}

func (s *Server) createBlackboardHTTP(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Title         string   `json:"title"`
		Content       string   `json:"content"`
		Kind          string   `json:"kind"`
		Status        string   `json:"status"`
		Priority      string   `json:"priority"`
		Source        string   `json:"source"`
		DependencyIDs []string `json:"dependencyIds"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		respondError(w, http.StatusBadRequest, "解析黑板条目失败")
		return
	}
	title := strings.TrimSpace(request.Title)
	content := strings.TrimSpace(request.Content)
	if title == "" {
		title = content
	}
	if content == "" {
		content = title
	}
	if title == "" {
		respondError(w, http.StatusBadRequest, "黑板条目标题不能为空")
		return
	}
	now := time.Now().Unix()
	fact := BlackboardFact{ID: fmt.Sprintf("fact-%d", time.Now().UnixNano()), Kind: firstNonEmpty(request.Kind, "note"), Statement: title, Source: firstNonEmpty(request.Source, "gui"), Status: firstNonEmpty(request.Status, "open"), Priority: firstNonEmpty(request.Priority, "normal"), DependencyIDs: request.DependencyIDs, CreatedAt: now, UpdatedAt: now}
	facts := append(s.loadBlackboardFacts(), fact)
	if err := s.writeBlackboardFacts(facts); err != nil {
		respondError(w, http.StatusInternalServerError, "保存黑板条目失败")
		return
	}
	respondJSON(w, http.StatusOK, blackboardItemDTO(fact))
}

func (s *Server) writeBlackboardFacts(facts []BlackboardFact) error {
	// stateMu: serialize against appends and other rewrites (see Server).
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	if err := os.MkdirAll(filepath.Dir(s.blackboardPath()), 0o755); err != nil {
		return err
	}
	file, err := os.OpenFile(s.blackboardPath(), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()
	for _, fact := range facts {
		data, err := json.Marshal(fact)
		if err != nil {
			return err
		}
		if _, err := file.Write(append(data, '\n')); err != nil {
			return err
		}
	}
	return nil
}

func (s *Server) updateBlackboardHTTP(w http.ResponseWriter, r *http.Request, id string) {
	facts := s.loadBlackboardFacts()
	var request map[string]any
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		respondError(w, http.StatusBadRequest, "解析黑板条目失败")
		return
	}
	for i := range facts {
		if facts[i].ID != id {
			continue
		}
		if value, ok := request["title"].(string); ok && strings.TrimSpace(value) != "" {
			facts[i].Statement = strings.TrimSpace(value)
		}
		if value, ok := request["content"].(string); ok && strings.TrimSpace(value) != "" {
			facts[i].Statement = strings.TrimSpace(value)
		}
		if value, ok := request["kind"].(string); ok && strings.TrimSpace(value) != "" {
			facts[i].Kind = strings.TrimSpace(value)
		}
		if value, ok := request["status"].(string); ok {
			facts[i].Status = strings.TrimSpace(value)
		}
		if value, ok := request["priority"].(string); ok {
			facts[i].Priority = strings.TrimSpace(value)
		}
		if value, ok := request["source"].(string); ok {
			facts[i].Source = strings.TrimSpace(value)
		}
		facts[i].UpdatedAt = time.Now().Unix()
		if err := s.writeBlackboardFacts(facts); err != nil {
			respondError(w, http.StatusInternalServerError, "保存黑板条目失败")
			return
		}
		respondJSON(w, http.StatusOK, blackboardItemDTO(facts[i]))
		return
	}
	respondError(w, http.StatusNotFound, "黑板条目不存在")
}

func (s *Server) batchUpdateBlackboardHTTP(w http.ResponseWriter, r *http.Request) {
	var request struct {
		IDs    []string       `json:"ids"`
		Update map[string]any `json:"update"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		respondError(w, http.StatusBadRequest, "解析黑板批量更新失败")
		return
	}
	facts := s.loadBlackboardFacts()
	count := 0
	for i := range facts {
		for _, id := range request.IDs {
			if facts[i].ID != id {
				continue
			}
			if value, ok := request.Update["status"].(string); ok {
				facts[i].Status = value
			}
			if value, ok := request.Update["priority"].(string); ok {
				facts[i].Priority = value
			}
			facts[i].UpdatedAt = time.Now().Unix()
			count++
		}
	}
	if err := s.writeBlackboardFacts(facts); err != nil {
		respondError(w, http.StatusInternalServerError, "保存黑板批量更新失败")
		return
	}
	items := make([]map[string]any, 0, len(facts))
	for _, fact := range facts {
		items = append(items, blackboardItemDTO(fact))
	}
	respondJSON(w, http.StatusOK, map[string]any{"path": filepath.ToSlash(s.blackboardPath()), "items": items, "count": count})
}

func (s *Server) linkBlackboardDependenciesHTTP(w http.ResponseWriter, r *http.Request, id string) {
	var request struct {
		DependencyIDs []string `json:"dependencyIds"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		respondError(w, http.StatusBadRequest, "解析黑板依赖失败")
		return
	}
	facts := s.loadBlackboardFacts()
	for i := range facts {
		if facts[i].ID == id {
			facts[i].DependencyIDs = request.DependencyIDs
			facts[i].UpdatedAt = time.Now().Unix()
			if err := s.writeBlackboardFacts(facts); err != nil {
				respondError(w, http.StatusInternalServerError, "保存黑板依赖失败")
				return
			}
			dependencies := make([]map[string]any, 0)
			for _, dependencyID := range request.DependencyIDs {
				for _, candidate := range facts {
					if candidate.ID == dependencyID {
						dependencies = append(dependencies, blackboardItemDTO(candidate))
					}
				}
			}
			respondJSON(w, http.StatusOK, map[string]any{"path": filepath.ToSlash(s.blackboardPath()), "count": len(dependencies), "item": blackboardItemDTO(facts[i]), "dependencies": dependencies})
			return
		}
	}
	respondError(w, http.StatusNotFound, "黑板条目不存在")
}

// handleAIAgents 返回工作区中真实持久化的智能体任务。
func (s *Server) handleAIAgents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		respondError(w, http.StatusMethodNotAllowed, "不支持的智能体请求方法")
		return
	}
	respondJSON(w, http.StatusOK, map[string]any{"items": s.loadAgentTasks()})
}

// handleAIPrompts 返回工作区中真实持久化的 prompt registry。
func (s *Server) handleAIPrompts(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		respondError(w, http.StatusMethodNotAllowed, "不支持的 prompt 请求方法")
		return
	}
	prompts := s.loadWorkspacePrompts()
	projected := make([]map[string]any, 0, len(prompts))
	for _, prompt := range prompts {
		versionID := prompt.ID + "@1"
		projected = append(projected, map[string]any{
			"id":            prompt.ID,
			"title":         prompt.Name,
			"kind":          "workspace",
			"activeVersion": versionID,
			"versions": []map[string]any{{
				"id":      versionID,
				"title":   prompt.Name,
				"content": prompt.Content,
			}},
		})
	}
	respondJSON(w, http.StatusOK, map[string]any{
		"version": 1,
		"path":    filepath.ToSlash(s.promptsPath()),
		"prompts": projected,
	})
}

// handleAIHarnessCases 返回工作区中真实持久化的 harness cases。
func (s *Server) handleAIHarnessCases(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		respondError(w, http.StatusMethodNotAllowed, "不支持的评测用例请求方法")
		return
	}
	respondJSON(w, http.StatusOK, map[string]any{"items": s.loadHarnessCases()})
}

func (s *Server) handleHarnessHTTP(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/harness")
	if path == "" || path == "/" {
		if r.Method != http.MethodGet {
			respondError(w, http.StatusMethodNotAllowed, "不支持的评测状态方法")
			return
		}
		cases := s.loadHarnessCases()
		results := s.loadWorkspaceHarnessResults()
		respondJSON(w, http.StatusOK, map[string]any{
			"path":       filepath.ToSlash(s.harnessCasesPath()),
			"resultPath": filepath.ToSlash(s.harnessPath()),
			"cases":      cases,
			"results":    results,
			"counts":     map[string]int{"totalCases": len(cases), "totalResults": len(results)},
		})
		return
	}
	trimmed := strings.Trim(path, "/")
	parts := strings.Split(trimmed, "/")
	if len(parts) == 1 && parts[0] == "cases" && r.Method == http.MethodGet {
		respondJSON(w, http.StatusOK, map[string]any{"cases": s.loadHarnessCases()})
		return
	}
	if len(parts) == 1 && parts[0] == "cases" && r.Method == http.MethodPost {
		var record WorkspaceHarnessCase
		if err := json.NewDecoder(r.Body).Decode(&record); err != nil {
			respondError(w, http.StatusBadRequest, "解析评测用例失败")
			return
		}
		created, err := s.createHarnessCase(record)
		if err != nil {
			respondError(w, http.StatusBadRequest, err.Error())
			return
		}
		respondJSON(w, http.StatusOK, created)
		return
	}
	if len(parts) == 3 && parts[0] == "cases" && parts[2] == "run" && r.Method == http.MethodPost {
		result, err := s.runWorkspaceHarnessCase(parts[1])
		if err != nil {
			respondError(w, http.StatusInternalServerError, err.Error())
			return
		}
		testCase, found := s.findHarnessCase(parts[1])
		if !found {
			respondError(w, http.StatusNotFound, "评测用例不存在")
			return
		}
		respondJSON(w, http.StatusOK, map[string]any{
			"case": testCase,
			"result": map[string]any{
				"id": result.ID, "caseId": testCase.ID, "caseTitle": testCase.Title,
				"status": "completed", "command": testCase.Command, "expectedResult": testCase.ExpectedResult,
				"startedAt": result.CreatedAt, "completedAt": result.CreatedAt,
				"summary": result.Output, "output": result.Output, "model": result.Model,
			},
		})
		return
	}
	respondError(w, http.StatusNotFound, "评测接口不存在")
}

func (s *Server) handlePromptAliasHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		s.handleAIPrompts(w, r)
		return
	}
	if r.Method != http.MethodPut {
		respondError(w, http.StatusMethodNotAllowed, "不支持的 prompt 方法")
		return
	}
	var payload struct {
		Prompts []struct {
			Title    string `json:"title"`
			Versions []struct {
				Content string `json:"content"`
			} `json:"versions"`
		} `json:"prompts"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		respondError(w, http.StatusBadRequest, "解析 prompt 注册表失败")
		return
	}
	for _, prompt := range payload.Prompts {
		if len(prompt.Versions) == 0 || strings.TrimSpace(prompt.Title) == "" {
			continue
		}
		if _, err := s.saveWorkspacePrompt(prompt.Title, prompt.Versions[len(prompt.Versions)-1].Content); err != nil {
			respondError(w, http.StatusInternalServerError, "保存 prompt 注册表失败")
			return
		}
	}
	readRequest := r.Clone(r.Context())
	readRequest.Method = http.MethodGet
	s.handleAIPrompts(w, readRequest)
}

// handleAIHarnessResults 返回工作区中真实持久化的 harness results。
func (s *Server) handleAIHarnessResults(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		respondError(w, http.StatusMethodNotAllowed, "不支持的评测结果请求方法")
		return
	}
	respondJSON(w, http.StatusOK, map[string]any{"items": s.loadWorkspaceHarnessResults()})
}

// handleAISwarmRuns 返回工作区中真实持久化的多智能体调度记录。
func (s *Server) handleAISwarmRuns(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		respondError(w, http.StatusMethodNotAllowed, "不支持的多智能体调度请求方法")
		return
	}
	respondJSON(w, http.StatusOK, map[string]any{"items": s.loadSwarmWorkflowRuns()})
}
