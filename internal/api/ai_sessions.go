package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const defaultAIConversationSessionID = "workspace-current"

func (s *Server) handleAIConversationSessions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		respondError(w, http.StatusMethodNotAllowed, "不支持的会话列表方法")
		return
	}
	respondJSON(w, http.StatusOK, map[string]any{"sessions": []map[string]any{s.aiConversationSessionSummary()}})
}

func (s *Server) handleAIConversationSessionNew(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		respondError(w, http.StatusMethodNotAllowed, "不支持的新建会话方法")
		return
	}
	if err := s.writeAIConversation([]ChatMessage{}); err != nil {
		respondError(w, http.StatusInternalServerError, "清空当前会话失败")
		return
	}
	s.respondAIConversationSessionOperation(w)
}

func (s *Server) handleAIConversationSessionRestore(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		respondError(w, http.StatusMethodNotAllowed, "不支持的恢复会话方法")
		return
	}
	var request struct {
		SessionID string `json:"sessionId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil || strings.TrimSpace(request.SessionID) != defaultAIConversationSessionID {
		respondError(w, http.StatusNotFound, "会话不存在")
		return
	}
	s.respondAIConversationSessionOperation(w)
}

func (s *Server) aiConversationSessionSummary() map[string]any {
	messages := s.loadAIConversation()
	var startedAt, updatedAt int64
	var latestUser, latestAssistant string
	for _, message := range messages {
		if startedAt == 0 || message.Timestamp < startedAt {
			startedAt = message.Timestamp
		}
		if message.Timestamp > updatedAt {
			updatedAt = message.Timestamp
		}
		if message.Role == "user" {
			latestUser = message.Content
		}
		if message.Role == "assistant" {
			latestAssistant = message.Content
		}
	}
	return map[string]any{
		"id": defaultAIConversationSessionID, "path": filepath.ToSlash(filepath.Join(s.workspace, "runtime", "ai-conversation.jsonl")), "current": true,
		"messageCount": len(messages), "userCount": countChatRole(messages, "user"), "assistantCount": countChatRole(messages, "assistant"), "toolCount": countChatRole(messages, "tool"),
		"startedAt": unixOrNil(startedAt), "updatedAt": unixOrNil(updatedAt), "latestUserGoal": latestUser, "latestAssistantSummary": latestAssistant,
	}
}

func countChatRole(messages []ChatMessage, role string) int {
	count := 0
	for _, message := range messages {
		if message.Role == role {
			count++
		}
	}
	return count
}
func unixOrNil(value int64) any {
	if value == 0 {
		return nil
	}
	return time.Unix(value, 0).UTC().Format(time.RFC3339Nano)
}

func (s *Server) respondAIConversationSessionOperation(w http.ResponseWriter) {
	summary := s.aiConversationSessionSummary()
	messages := s.loadAIConversation()
	conversation := make([]map[string]any, 0, len(messages))
	for _, message := range messages {
		conversation = append(conversation, map[string]any{"id": message.ID, "role": message.Role, "content": message.Content, "createdAt": message.Timestamp})
	}
	payload := map[string]any{"session": summary, "sessions": []map[string]any{summary}, "conversation": map[string]any{"messages": conversation}}
	respondJSON(w, http.StatusOK, payload)
}

func (s *Server) handleAIConversationSearch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		respondError(w, http.StatusMethodNotAllowed, "不支持的会话搜索方法")
		return
	}
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if query == "" {
		respondJSON(w, http.StatusOK, map[string]any{"query": "", "results": []any{}, "count": 0})
		return
	}
	results := []map[string]any{}
	for _, message := range s.loadAIConversation() {
		if strings.Contains(strings.ToLower(message.Content), strings.ToLower(query)) {
			results = append(results, map[string]any{"id": message.ID, "sessionId": defaultAIConversationSessionID, "sessionCurrent": true, "messageId": message.ID, "role": message.Role, "snippet": message.Content, "contentSnippet": message.Content, "path": filepath.ToSlash(filepath.Join(s.workspace, "runtime", "ai-conversation.jsonl")), "createdAt": message.Timestamp, "contextReferenceCount": len(message.Refs)})
		}
	}
	respondJSON(w, http.StatusOK, map[string]any{"query": query, "results": results, "count": len(results)})
}

func (s *Server) handleAIConversationExport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		respondError(w, http.StatusMethodNotAllowed, "不支持的会话导出方法")
		return
	}
	var request struct {
		SessionID string `json:"sessionId"`
		Format    string `json:"format"`
	}
	_ = json.NewDecoder(r.Body).Decode(&request)
	if request.SessionID != "" && request.SessionID != defaultAIConversationSessionID {
		respondError(w, http.StatusNotFound, "会话不存在")
		return
	}
	format := strings.ToLower(strings.TrimSpace(request.Format))
	if format == "" {
		format = "markdown"
	}
	if format != "markdown" && format != "json" {
		respondError(w, http.StatusBadRequest, "不支持的导出格式")
		return
	}
	messages := s.loadAIConversation()
	exportDir := filepath.Join(s.workspace, "runtime", "exports")
	if err := os.MkdirAll(exportDir, 0o755); err != nil {
		respondError(w, http.StatusInternalServerError, "创建导出目录失败")
		return
	}
	path := filepath.Join(exportDir, "ai-conversation-"+time.Now().UTC().Format("20060102-150405")+"."+format)
	var data []byte
	var err error
	if format == "json" {
		data, err = json.MarshalIndent(messages, "", "  ")
		data = append(data, '\n')
	} else {
		var builder strings.Builder
		builder.WriteString("# AI 会话\n\n")
		for _, message := range messages {
			builder.WriteString(fmt.Sprintf("## %s\n\n%s\n\n", message.Role, message.Content))
		}
		data = []byte(builder.String())
	}
	if err != nil {
		respondError(w, http.StatusInternalServerError, "生成会话导出失败")
		return
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		respondError(w, http.StatusInternalServerError, "写入会话导出失败")
		return
	}
	respondJSON(w, http.StatusOK, map[string]any{"sessionId": defaultAIConversationSessionID, "path": filepath.ToSlash(path), "format": format, "messageCount": len(messages), "exportedAt": time.Now().UTC().Format(time.RFC3339Nano), "summary": "已导出当前 AI 会话"})
}

func (s *Server) writeAIConversation(messages []ChatMessage) error {
	path := filepath.Join(s.workspace, "runtime", "ai-conversation.jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()
	for _, message := range messages {
		data, err := json.Marshal(message)
		if err != nil {
			return err
		}
		if _, err := file.Write(append(data, '\n')); err != nil {
			return err
		}
	}
	return nil
}
