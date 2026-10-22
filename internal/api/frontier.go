package api

import "net/http"

// The current server does not persist a frontier queue projection yet. These
// read-only envelopes keep the GUI contract explicit without inventing queue
// items or pretending that the capability is available.
func (s *Server) handleFrontierQueue(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		respondError(w, http.StatusMethodNotAllowed, "仅支持读取前沿队列")
		return
	}
	respondJSON(w, http.StatusOK, frontierUnavailableResponse())
}

func (s *Server) handleFrontierMonitor(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		respondError(w, http.StatusMethodNotAllowed, "仅支持读取前沿监控")
		return
	}
	response := frontierUnavailableResponse()
	response["activeAgentTasks"] = []any{}
	response["activePlannerPlans"] = []any{}
	response["runningExecutions"] = []any{}
	response["recentAgentEvents"] = []any{}
	respondJSON(w, http.StatusOK, response)
}

func (s *Server) handleFrontierDependencies(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		respondError(w, http.StatusMethodNotAllowed, "仅支持读取前沿依赖")
		return
	}
	response := frontierUnavailableResponse()
	response["clusters"] = []any{}
	response["candidates"] = []any{}
	respondJSON(w, http.StatusOK, response)
}

func frontierUnavailableResponse() map[string]any {
	return map[string]any{
		"status": "unavailable",
		"source": "frontier_projection_unavailable",
		"items":  []any{},
		"summary": map[string]any{
			"status":       "unavailable",
			"source":       "frontier_projection_unavailable",
			"queueStatus":  "unavailable",
			"queueItems":   0,
			"readyItems":   0,
			"blockedItems": 0,
		},
		"queue": map[string]any{
			"status": "unavailable",
			"items":  []any{},
		},
	}
}
