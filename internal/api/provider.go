package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/ouroboros/ouroboros/internal/app"
)

func (s *Server) handleProvider(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/api/provider/") {
		if r.Method != http.MethodDelete {
			respondError(w, http.StatusMethodNotAllowed, "不支持的模型服务删除方法")
			return
		}
		name := strings.TrimPrefix(r.URL.Path, "/api/provider/")
		config, err := ProjectProviderConfig(s.workspace)
		if err != nil {
			respondError(w, http.StatusInternalServerError, "读取模型服务配置失败")
			return
		}
		for _, provider := range config.Providers {
			if provider.Name == name {
				if _, err := app.ConfigureOpenAICompatibleProvider(s.workspace, app.OpenAICompatibleProviderConfiguration{Name: provider.Name, BaseURL: provider.BaseURL, Model: provider.Model, APIKeyEnv: provider.APIKeyEnv, Enabled: false, TimeoutMS: provider.TimeoutMS, ContextWindowTokens: provider.ContextWindowTokens, Capabilities: provider.Capabilities, RoutingStrategy: config.RoutingStrategy}); err != nil {
					respondError(w, http.StatusBadRequest, "停用模型服务失败")
					return
				}
				updated, _ := ProjectProviderConfig(s.workspace)
				respondJSON(w, http.StatusOK, updated)
				return
			}
		}
		respondError(w, http.StatusNotFound, "模型服务不存在")
		return
	}
	switch r.Method {
	case http.MethodGet:
		config, err := ProjectProviderConfig(s.workspace)
		if err != nil {
			respondError(w, http.StatusInternalServerError, fmt.Sprintf("读取模型服务配置失败: %v", err))
			return
		}
		respondJSON(w, http.StatusOK, config)
	case http.MethodPut:
		var request ProviderUpdateRequestDTO
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			respondError(w, http.StatusBadRequest, fmt.Sprintf("解析模型服务配置失败: %v", err))
			return
		}
		if err := saveProviderConfig(s.workspace, request); err != nil {
			respondError(w, http.StatusBadRequest, fmt.Sprintf("保存模型服务配置失败: %v", err))
			return
		}
		config, err := ProjectProviderConfig(s.workspace)
		if err != nil {
			respondError(w, http.StatusInternalServerError, fmt.Sprintf("读取模型服务配置失败: %v", err))
			return
		}
		s.BroadcastEvent("provider:updated", config.Active)
		respondJSON(w, http.StatusOK, config)
	default:
		respondError(w, http.StatusMethodNotAllowed, "不支持的模型服务方法")
	}
}

func (s *Server) handleProviderTest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		respondError(w, http.StatusMethodNotAllowed, "不支持的模型服务测试方法")
		return
	}
	var request struct {
		ProviderName string `json:"providerName"`
		Model        string `json:"model"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		respondError(w, http.StatusBadRequest, "解析模型服务测试请求失败")
		return
	}
	config, err := ProjectProviderConfig(s.workspace)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "读取模型服务配置失败")
		return
	}
	provider, model := config.Active.Name, config.Active.Model
	if strings.TrimSpace(request.ProviderName) != "" {
		provider = strings.TrimSpace(request.ProviderName)
	}
	if strings.TrimSpace(request.Model) != "" {
		model = strings.TrimSpace(request.Model)
	}
	started := time.Now()
	result, generationErr := app.GenerateProviderMessage(context.Background(), s.workspace, app.ProviderGenerationRequest{UserPrompt: "连接测试", MaxTokens: 32, RequiredCapabilities: []string{"chat"}})
	if generationErr != nil {
		respondJSON(w, http.StatusOK, map[string]any{"provider": provider, "model": model, "status": "failed", "content": generationErr.Error(), "latencyMs": time.Since(started).Milliseconds(), "config": config})
		return
	}
	respondJSON(w, http.StatusOK, map[string]any{"provider": result.ProviderName, "model": result.Model, "status": "ready", "content": result.Text, "latencyMs": result.Latency.Milliseconds(), "usage": map[string]any{"inputTokens": result.InputTokens, "outputTokens": result.OutputTokens, "totalTokens": result.TotalTokens, "costUsd": result.EstimatedCostUSD, "costKnown": result.EstimatedCostAvailable}, "config": config})
}

func saveProviderConfig(workspace string, request ProviderUpdateRequestDTO) error {
	providerType := strings.TrimSpace(request.Type)
	if providerType == "" {
		providerType = "openai_compatible"
	}
	if providerType != "openai_compatible" {
		return fmt.Errorf("provider type is not supported by the current writer: %s", providerType)
	}
	_, err := app.ConfigureOpenAICompatibleProvider(workspace, app.OpenAICompatibleProviderConfiguration{
		Name:                request.Name,
		BaseURL:             request.BaseURL,
		Model:               request.Model,
		APIKeyEnv:           request.APIKeyEnv,
		Enabled:             request.Enabled,
		TimeoutMS:           request.TimeoutMS,
		ContextWindowTokens: request.ContextWindowTokens,
		Capabilities:        request.Capabilities,
		RoutingStrategy:     request.RoutingStrategy,
	})
	return err
}
