package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/ouroboros/ouroboros/internal/eventlog"
)

// Server 是HTTP API服务器
type Server struct {
	workspace  string
	eventStore *eventlog.SQLiteStore
	port       int
	server     *http.Server
	clients    map[*websocket.Conn]bool
	clientsMu  sync.Mutex
	wsWriteMu  sync.Mutex
	broadcast  chan []byte
	execMu     sync.Mutex
	executions map[string]context.CancelFunc
	done       chan struct{} // 用于优雅停止 goroutine
	stopOnce   sync.Once
}

// NewServer 创建新的API服务器
func NewServer(workspace string, port int) (*Server, error) {
	// 打开事件日志数据库
	eventStorePath := workspace + "/replay/events.db"
	store, err := eventlog.OpenSQLite(eventStorePath)
	if err != nil {
		return nil, fmt.Errorf("打开事件数据库失败: %w", err)
	}

	s := &Server{
		workspace:  workspace,
		eventStore: store,
		port:       port,
		clients:    make(map[*websocket.Conn]bool),
		broadcast:  make(chan []byte, 256),
		executions: make(map[string]context.CancelFunc),
		done:       make(chan struct{}),
	}

	// 启动广播goroutine
	go s.handleBroadcast()

	return s, nil
}

// Start 启动HTTP服务器
func (s *Server) Start() error {
	mux := http.NewServeMux()

	// 注册路由
	mux.HandleFunc("/api/health", s.handleHealth)
	mux.HandleFunc("/api/gui/bootstrap", s.handleGUIBootstrap)
	mux.HandleFunc("/api/provider", s.handleProvider)
	mux.HandleFunc("/api/provider/test", s.handleProviderTest)
	mux.HandleFunc("/api/provider/", s.handleProvider)
	mux.HandleFunc("/api/mission/start", s.handleMissionStart)
	mux.HandleFunc("/api/mission", s.handleMission)
	mux.HandleFunc("/api/workspace/info", s.handleWorkspaceInfo)
	mux.HandleFunc("/api/workspace/files", s.handleWorkspaceFiles)
	mux.HandleFunc("/api/workspace/file", s.handleWorkspaceFile)
	mux.HandleFunc("/api/workspace/search", s.handleWorkspaceSearch)
	mux.HandleFunc("/api/workspace/settings", s.handleWorkspaceSettings)
	mux.HandleFunc("/api/executions", s.handleExecutions)
	mux.HandleFunc("/api/executions/", s.handleExecutionByID)
	mux.HandleFunc("/api/evidence/graph/export", s.handleEvidenceGraphExport)
	mux.HandleFunc("/api/evidence/graph", s.handleEvidenceGraph)
	mux.HandleFunc("/api/evidence/nodes", s.handleEvidenceNodes)
	mux.HandleFunc("/api/reports/export", s.handleReportExport)
	mux.HandleFunc("/api/runs", s.handleRuns)
	mux.HandleFunc("/api/events", s.handleEvents)
	mux.HandleFunc("/api/runtime/workers", s.handleRuntimeWorkers)
	mux.HandleFunc("/api/agents/profiles", s.handleAgentProfiles)
	mux.HandleFunc("/api/agents/profiles/", s.handleAgentProfiles)
	mux.HandleFunc("/api/agents/tasks", s.handleAgentTasksHTTP)
	mux.HandleFunc("/api/agents/tasks/", s.handleAgentTasksHTTP)
	mux.HandleFunc("/api/harness", s.handleHarnessHTTP)
	mux.HandleFunc("/api/harness/cases", s.handleHarnessHTTP)
	mux.HandleFunc("/api/harness/cases/", s.handleHarnessHTTP)
	mux.HandleFunc("/api/prompts", s.handlePromptAliasHTTP)
	mux.HandleFunc("/api/toolchain", s.handleToolchain)
	mux.HandleFunc("/api/toolchain/actions", s.handleToolchainActions)
	mux.HandleFunc("/api/toolchain/actions/", s.handleToolchainActionByID)
	mux.HandleFunc("/api/toolchain/packs/", s.handleToolchainPack)
	mux.HandleFunc("/api/toolchain/health", s.handleToolchainHealth)
	mux.HandleFunc("/api/frontier/queue", s.handleFrontierQueue)
	mux.HandleFunc("/api/frontier/monitor", s.handleFrontierMonitor)
	mux.HandleFunc("/api/frontier/dependencies", s.handleFrontierDependencies)
	mux.HandleFunc("/api/ai/actions", s.handleAIActions)
	mux.HandleFunc("/api/ai/actions/run", s.handleAIActionRun)
	mux.HandleFunc("/api/ai/chat", s.handleAIChat)
	mux.HandleFunc("/api/ai/conversation/sessions", s.handleAIConversationSessions)
	mux.HandleFunc("/api/ai/conversation/sessions/new", s.handleAIConversationSessionNew)
	mux.HandleFunc("/api/ai/conversation/sessions/restore", s.handleAIConversationSessionRestore)
	mux.HandleFunc("/api/ai/conversation/search", s.handleAIConversationSearch)
	mux.HandleFunc("/api/ai/conversation/export", s.handleAIConversationExport)
	mux.HandleFunc("/api/ai/blackboard", s.handleAIBlackboard)
	mux.HandleFunc("/api/ai/blackboard/", s.handleAIBlackboard)
	mux.HandleFunc("/api/ai/agents", s.handleAIAgents)
	mux.HandleFunc("/api/ai/prompts", s.handleAIPrompts)
	mux.HandleFunc("/api/ai/harness/cases", s.handleAIHarnessCases)
	mux.HandleFunc("/api/ai/harness/results", s.handleAIHarnessResults)
	mux.HandleFunc("/api/ai/swarm/runs", s.handleAISwarmRuns)
	mux.HandleFunc("/api/events/stream", s.handleWebSocket)

	// CORS中间件
	handler := corsMiddleware(mux)

	s.server = &http.Server{
		Addr:    fmt.Sprintf(":%d", s.port),
		Handler: handler,
	}

	log.Printf("🚀 Ouroboros API服务器启动在 http://localhost:%d", s.port)
	return s.server.ListenAndServe()
}

// Stop 停止服务器
func (s *Server) Stop(ctx context.Context) error {
	// Closing done must be safe when shutdown is requested more than once.
	if s.done != nil {
		s.stopOnce.Do(func() { close(s.done) })
	}
	// 2. 停止所有活动执行
	s.stopActiveExecutions()

	// 3. 关闭所有WebSocket连接
	s.clientsMu.Lock()
	for client := range s.clients {
		client.Close()
	}
	s.clients = make(map[*websocket.Conn]bool) // 清空map
	s.clientsMu.Unlock()

	// 4. 关闭 SQLite 连接
	if s.eventStore != nil {
		if err := s.eventStore.Close(); err != nil {
			log.Printf("关闭事件存储失败: %v", err)
		}
	}

	// 5. 关闭 HTTP 服务器
	if s.server != nil {
		return s.server.Shutdown(ctx)
	}
	return nil
}

// corsMiddleware 添加CORS头
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 允许Electron renderer访问
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		// 处理预检请求
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// handleHealth 健康检查
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, http.StatusOK, map[string]interface{}{
		"status": "healthy",
		"time":   time.Now().Unix(),
	})
}

// handleWorkspaceInfo 工作区信息
func (s *Server) handleWorkspaceInfo(w http.ResponseWriter, r *http.Request) {
	workspace := ProjectWorkspace(s.workspace)
	respondJSON(w, http.StatusOK, map[string]interface{}{
		"workspace": workspace.Path,
		"name":      workspace.Name,
		"status":    workspace.Status,
	})
}

// handleWorkspaceFiles 返回真实工作区文件列表
func (s *Server) handleWorkspaceFiles(w http.ResponseWriter, r *http.Request) {
	files, err := ProjectWorkspaceFiles(s.workspace)
	if err != nil {
		respondError(w, http.StatusInternalServerError, fmt.Sprintf("获取工作区文件失败: %v", err))
		return
	}
	respondJSON(w, http.StatusOK, map[string]interface{}{"files": files})
}

// handleWorkspaceFile 读取或保存普通工作区文本文件
func (s *Server) handleWorkspaceFile(w http.ResponseWriter, r *http.Request) {
	filePath := r.URL.Query().Get("path")
	switch r.Method {
	case http.MethodGet:
		file, err := ReadWorkspaceFile(s.workspace, filePath)
		if err != nil {
			respondError(w, http.StatusBadRequest, fmt.Sprintf("读取工作区文件失败: %v", err))
			return
		}
		respondJSON(w, http.StatusOK, file)
	case http.MethodPut:
		var request struct {
			Content string `json:"content"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			respondError(w, http.StatusBadRequest, fmt.Sprintf("解析保存内容失败: %v", err))
			return
		}
		file, err := WriteWorkspaceFile(s.workspace, filePath, request.Content)
		if err != nil {
			respondError(w, http.StatusBadRequest, fmt.Sprintf("保存工作区文件失败: %v", err))
			return
		}
		respondJSON(w, http.StatusOK, file)
	default:
		respondError(w, http.StatusMethodNotAllowed, "不支持的工作区文件方法")
	}
}

// handleWorkspaceSearch 搜索普通工作区文本文件
func (s *Server) handleWorkspaceSearch(w http.ResponseWriter, r *http.Request) {
	results, err := SearchWorkspace(s.workspace, r.URL.Query().Get("q"))
	if err != nil {
		respondError(w, http.StatusInternalServerError, fmt.Sprintf("搜索工作区失败: %v", err))
		return
	}
	respondJSON(w, http.StatusOK, map[string]interface{}{"results": results})
}

// handleGUIBootstrap 返回桌面GUI首屏所需的聚合数据
func (s *Server) handleGUIBootstrap(w http.ResponseWriter, r *http.Request) {
	dto, err := ProjectGUIBootstrap(r.Context(), s.workspace, s.eventStore)
	if err != nil {
		respondError(w, http.StatusInternalServerError, fmt.Sprintf("投影GUI数据失败: %v", err))
		return
	}
	respondJSON(w, http.StatusOK, dto)
}

// handleEvidenceGraph 证据图数据
func (s *Server) handleEvidenceGraph(w http.ResponseWriter, r *http.Request) {
	graph, err := ProjectEvidenceGraph(r.Context(), s.workspace, s.eventStore)
	if err != nil {
		respondError(w, http.StatusInternalServerError, fmt.Sprintf("获取证据图失败: %v", err))
		return
	}
	respondJSON(w, http.StatusOK, graph)
}

func (s *Server) handleEvidenceGraphExport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		respondError(w, http.StatusMethodNotAllowed, "不支持的图谱导出方法")
		return
	}
	export, err := ExportEvidenceGraph(r.Context(), s.workspace, s.eventStore)
	if err != nil {
		respondError(w, http.StatusInternalServerError, fmt.Sprintf("导出证据图失败: %v", err))
		return
	}
	respondJSON(w, http.StatusOK, export)
}

// handleEvidenceNodes 节点列表
func (s *Server) handleEvidenceNodes(w http.ResponseWriter, r *http.Request) {
	graph, err := ProjectEvidenceGraph(r.Context(), s.workspace, s.eventStore)
	if err != nil {
		respondError(w, http.StatusInternalServerError, fmt.Sprintf("获取证据节点失败: %v", err))
		return
	}
	respondJSON(w, http.StatusOK, map[string]interface{}{
		"nodes": graph.Nodes,
	})
}

func (s *Server) handleReportExport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		respondError(w, http.StatusMethodNotAllowed, "不支持的报告导出方法")
		return
	}
	export, err := ExportReport(r.Context(), s.workspace, s.eventStore)
	if err != nil {
		respondError(w, http.StatusInternalServerError, fmt.Sprintf("导出报告失败: %v", err))
		return
	}
	respondJSON(w, http.StatusOK, export)
}

// handleRuns 运行历史
func (s *Server) handleRuns(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	runIDs, err := s.eventStore.ListRunIDs(ctx)
	if err != nil {
		respondError(w, http.StatusInternalServerError, fmt.Sprintf("获取运行列表失败: %v", err))
		return
	}

	runs := []RunDTO{}
	for _, runID := range runIDs {
		events, err := s.eventStore.Replay(ctx, runID)
		if err != nil {
			respondError(w, http.StatusInternalServerError, fmt.Sprintf("获取运行事件失败: %v", err))
			return
		}
		runs = append(runs, RunDTO{ID: runID, Status: runStatus(events), StartedAt: firstEventUnix(events)})
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"runs": runs,
	})
}

// handleEvents exposes the same persisted event projection used by replay and the GUI.
func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		respondError(w, http.StatusMethodNotAllowed, "不支持的事件查询方法")
		return
	}

	events, err := ProjectReplayEvents(r.Context(), s.eventStore)
	if err != nil {
		respondError(w, http.StatusInternalServerError, fmt.Sprintf("获取事件日志失败: %v", err))
		return
	}

	query := r.URL.Query()
	runID := query.Get("runId")
	lane := query.Get("lane")
	eventType := query.Get("type")
	filtered := make([]ReplayEventDTO, 0, len(events))
	for _, event := range events {
		if runID != "" && event.RunID != runID {
			continue
		}
		if lane != "" && event.Lane != lane {
			continue
		}
		if eventType != "" && event.Type != eventType {
			continue
		}
		filtered = append(filtered, event)
	}

	limit := 0
	if rawLimit := query.Get("limit"); rawLimit != "" {
		if parsed, parseErr := strconv.Atoi(rawLimit); parseErr == nil && parsed > 0 {
			limit = parsed
		}
	}
	if limit > 0 && len(filtered) > limit {
		filtered = filtered[len(filtered)-limit:]
	}

	runs, err := ProjectRuns(r.Context(), s.eventStore)
	if err != nil {
		respondError(w, http.StatusInternalServerError, fmt.Sprintf("获取事件运行列表失败: %v", err))
		return
	}
	respondJSON(w, http.StatusOK, map[string]any{
		"runs":   runs,
		"events": filtered,
		"count":  len(filtered),
		"runId":  runID,
		"lane":   lane,
		"type":   eventType,
		"limit":  limit,
	})
}

// handleToolchainHealth 工具链健康状态
func (s *Server) handleToolchainHealth(w http.ResponseWriter, r *http.Request) {
	toolchainDTO, err := ProjectToolchain(s.workspace)
	if err != nil {
		respondError(w, http.StatusInternalServerError, fmt.Sprintf("获取工具链状态失败: %v", err))
		return
	}
	packs := map[string]interface{}{}
	for _, pack := range toolchainDTO.Packs {
		packs[pack.ID] = map[string]interface{}{
			"status":       pack.Status,
			"toolsReady":   pack.ToolsReady,
			"name":         pack.Name,
			"manifestPath": pack.ManifestPath,
		}
	}
	respondJSON(w, http.StatusOK, map[string]interface{}{
		"status":     toolchainDTO.Status,
		"toolsReady": toolchainDTO.ToolsReady,
		"packs":      packs,
	})
}

// WebSocket升级器
var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // 允许所有来源（生产环境需要更严格的检查）
	},
}

// handleWebSocket 处理WebSocket连接
func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket升级失败: %v", err)
		return
	}

	// 注册客户端
	s.clientsMu.Lock()
	if s.clients == nil {
		s.clients = map[*websocket.Conn]bool{}
	}
	s.clients[conn] = true
	clientCount := len(s.clients)
	s.clientsMu.Unlock()

	log.Printf("✅ WebSocket客户端已连接（当前连接数: %d）", clientCount)

	// 发送欢迎消息
	welcomeMsg := map[string]interface{}{
		"type":    "connected",
		"message": "WebSocket连接成功",
		"time":    time.Now().Unix(),
	}
	if data, err := json.Marshal(welcomeMsg); err == nil {
		_ = s.writeWebSocketMessage(conn, websocket.TextMessage, data)
	}

	// 启动心跳
	go s.keepAlive(conn)

	// 读取消息（保持连接）
	defer func() {
		s.clientsMu.Lock()
		delete(s.clients, conn)
		clientCount := len(s.clients)
		s.clientsMu.Unlock()
		conn.Close()
		log.Printf("❌ WebSocket客户端已断开（当前连接数: %d）", clientCount)
	}()

	for {
		_, _, err := conn.ReadMessage()
		if err != nil {
			break
		}
	}
}

// keepAlive 发送心跳保持连接
func (s *Server) keepAlive(conn *websocket.Conn) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.clientsMu.Lock()
			exists := s.clients[conn]
			s.clientsMu.Unlock()

			if !exists {
				return // 连接已被移除
			}

			err := s.writeWebSocketMessage(conn, websocket.PingMessage, nil)
			if err != nil {
				return
			}
		case <-s.done:
			// 服务器停止时退出
			return
		}
	}
}

// handleBroadcast 处理广播消息
func (s *Server) handleBroadcast() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			event := map[string]interface{}{
				"type":    "heartbeat",
				"payload": map[string]interface{}{},
				"time":    time.Now().Unix(),
			}

			data, err := json.Marshal(event)
			if err != nil {
				continue
			}

			// 先复制客户端列表，再释放锁
			s.clientsMu.Lock()
			clients := make([]*websocket.Conn, 0, len(s.clients))
			for client := range s.clients {
				clients = append(clients, client)
			}
			s.clientsMu.Unlock()

			// 在锁外发送消息
			toRemove := []*websocket.Conn{}
			for _, client := range clients {
				err := s.writeWebSocketMessage(client, websocket.TextMessage, data)
				if err != nil {
					toRemove = append(toRemove, client)
				}
			}

			// 批量删除失败的连接
			if len(toRemove) > 0 {
				s.clientsMu.Lock()
				for _, client := range toRemove {
					if s.clients[client] {
						client.Close()
						delete(s.clients, client)
					}
				}
				s.clientsMu.Unlock()
			}

		case <-s.done:
			// 优雅停止
			return
		}
	}
}

// BroadcastEvent 广播事件到所有WebSocket客户端
func (s *Server) BroadcastEvent(eventType string, payload interface{}) {
	event := map[string]interface{}{
		"type":    eventType,
		"payload": payload,
		"time":    time.Now().Unix(),
	}

	data, err := json.Marshal(event)
	if err != nil {
		log.Printf("序列化事件失败: %v", err)
		return
	}

	// 先复制客户端列表
	s.clientsMu.Lock()
	clients := make([]*websocket.Conn, 0, len(s.clients))
	for client := range s.clients {
		clients = append(clients, client)
	}
	s.clientsMu.Unlock()

	// 在锁外发送消息
	toRemove := []*websocket.Conn{}
	for _, client := range clients {
		err := s.writeWebSocketMessage(client, websocket.TextMessage, data)
		if err != nil {
			toRemove = append(toRemove, client)
		}
	}

	// 批量删除失败的连接
	if len(toRemove) > 0 {
		s.clientsMu.Lock()
		for _, client := range toRemove {
			if s.clients[client] {
				client.Close()
				delete(s.clients, client)
			}
		}
		s.clientsMu.Unlock()
	}
}

func (s *Server) writeWebSocketMessage(conn *websocket.Conn, messageType int, data []byte) error {
	s.wsWriteMu.Lock()
	defer s.wsWriteMu.Unlock()
	return conn.WriteMessage(messageType, data)
}

// respondJSON 返回JSON响应
func respondJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

// respondError 返回错误响应
func respondError(w http.ResponseWriter, status int, message string) {
	respondJSON(w, status, map[string]interface{}{
		"error": message,
	})
}
