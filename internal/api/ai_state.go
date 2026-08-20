package api

import "net/http"

// handleAIBlackboard 返回工作区中真实持久化的黑板条目。
func (s *Server) handleAIBlackboard(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		respondError(w, http.StatusMethodNotAllowed, "不支持的黑板请求方法")
		return
	}
	respondJSON(w, http.StatusOK, map[string]any{"items": s.loadBlackboardFacts()})
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
	respondJSON(w, http.StatusOK, map[string]any{"items": s.loadWorkspacePrompts()})
}

// handleAIHarnessCases 返回工作区中真实持久化的 harness cases。
func (s *Server) handleAIHarnessCases(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		respondError(w, http.StatusMethodNotAllowed, "不支持的评测用例请求方法")
		return
	}
	respondJSON(w, http.StatusOK, map[string]any{"items": s.loadHarnessCases()})
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
