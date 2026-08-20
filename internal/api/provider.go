package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/ouroboros/ouroboros/internal/app"
)

func (s *Server) handleProvider(w http.ResponseWriter, r *http.Request) {
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
