package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/ouroboros/ouroboros/internal/toolchain"
	"gopkg.in/yaml.v3"
)

func (s *Server) handleToolchain(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		dto, err := ProjectToolchain(s.workspace)
		if err != nil {
			respondError(w, http.StatusInternalServerError, fmt.Sprintf("读取工具链配置失败: %v", err))
			return
		}
		respondJSON(w, http.StatusOK, dto)
	default:
		respondError(w, http.StatusMethodNotAllowed, "不支持的工具链方法")
	}
}

func (s *Server) handleToolchainPack(w http.ResponseWriter, r *http.Request) {
	packID := strings.TrimPrefix(r.URL.Path, "/api/toolchain/packs/")
	switch r.Method {
	case http.MethodPut:
		var request ToolchainPackUpdateRequestDTO
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			respondError(w, http.StatusBadRequest, fmt.Sprintf("解析工具链配置失败: %v", err))
			return
		}
		pack, err := saveToolchainPack(s.workspace, packID, request)
		if err != nil {
			respondError(w, http.StatusBadRequest, fmt.Sprintf("保存工具链配置失败: %v", err))
			return
		}
		s.BroadcastEvent("toolchain:updated", pack)
		respondJSON(w, http.StatusOK, pack)
	default:
		respondError(w, http.StatusMethodNotAllowed, "不支持的工具链包方法")
	}
}

func (s *Server) handleToolchainActions(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/api/toolchain/actions" {
		respondError(w, http.StatusNotFound, "工具动作不存在")
		return
	}
	switch r.Method {
	case http.MethodGet:
		toolchainDTO, err := ProjectToolchain(s.workspace)
		if err != nil {
			respondError(w, http.StatusInternalServerError, fmt.Sprintf("读取工具动作失败: %v", err))
			return
		}
		respondJSON(w, http.StatusOK, map[string]any{"actions": ProjectToolchainActions(toolchainDTO)})
	default:
		respondError(w, http.StatusMethodNotAllowed, "不支持的工具动作方法")
	}
}

func (s *Server) handleToolchainActionByID(w http.ResponseWriter, r *http.Request) {
	actionID, action, err := parseToolchainActionRoute(r.URL.Path)
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	if action != "run" {
		respondError(w, http.StatusNotFound, "工具动作不存在")
		return
	}
	if r.Method != http.MethodPost {
		respondError(w, http.StatusMethodNotAllowed, "不支持的工具动作执行方法")
		return
	}
	var request ToolchainActionRunRequestDTO
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		respondError(w, http.StatusBadRequest, fmt.Sprintf("解析工具执行请求失败: %v", err))
		return
	}
	response, err := s.StartToolchainAction(actionID, request)
	if err != nil {
		respondError(w, http.StatusBadRequest, fmt.Sprintf("启动工具动作失败: %v", err))
		return
	}
	respondJSON(w, http.StatusAccepted, response)
}

func (s *Server) StartToolchainAction(actionID string, request ToolchainActionRunRequestDTO) (ToolchainActionRunResponseDTO, error) {
	action, err := findToolchainAction(s.workspace, actionID)
	if err != nil {
		return ToolchainActionRunResponseDTO{}, err
	}
	if len(action.Command) == 0 {
		return ToolchainActionRunResponseDTO{}, fmt.Errorf("tool action has no run command: %s", actionID)
	}
	env := copyStringMap(request.Env)
	if env == nil {
		env = map[string]string{}
	}
	env["OURO_TOOL_ACTION_ID"] = action.ID
	env["OURO_TOOLCHAIN_PACK_ID"] = action.PackID
	env["OURO_TOOLCHAIN_PACK_NAME"] = action.PackName
	env["OURO_TOOL_ARGUMENTS"] = strings.TrimSpace(request.Arguments)
	env["OURO_TOOL_MANIFEST_PATH"] = action.ManifestPath

	commandParts := append([]string{}, action.Command...)
	commandParts = append(commandParts, parseToolArguments(request.Arguments)...)
	execution, err := s.StartExecution(ExecutionCreateRequestDTO{
		Command: commandParts[0],
		Args:    append([]string{}, commandParts[1:]...),
		CWD:     request.CWD,
		Env:     env,
	})
	if err != nil {
		return ToolchainActionRunResponseDTO{}, err
	}
	s.BroadcastEvent("toolchain:action-started", map[string]any{"action": action, "execution": execution})
	return ToolchainActionRunResponseDTO{Action: action, Execution: execution}, nil
}

func findToolchainAction(workspace string, actionID string) (ToolchainActionDTO, error) {
	actionID = strings.TrimSpace(actionID)
	if actionID == "" {
		return ToolchainActionDTO{}, fmt.Errorf("tool action id is required")
	}
	toolchainDTO, err := ProjectToolchain(workspace)
	if err != nil {
		return ToolchainActionDTO{}, err
	}
	for _, action := range ProjectToolchainActions(toolchainDTO) {
		if action.ID == actionID {
			return action, nil
		}
	}
	return ToolchainActionDTO{}, fmt.Errorf("tool action not found: %s", actionID)
}

func parseToolchainActionRoute(requestPath string) (string, string, error) {
	tail := strings.Trim(strings.TrimPrefix(requestPath, "/api/toolchain/actions/"), "/")
	if tail == "" || tail == requestPath {
		return "", "", fmt.Errorf("tool action id is required")
	}
	parts := strings.Split(tail, "/")
	actionID := strings.TrimSpace(parts[0])
	if actionID == "" {
		return "", "", fmt.Errorf("tool action id is required")
	}
	if len(parts) < 2 {
		return actionID, "", nil
	}
	return actionID, strings.TrimSpace(parts[1]), nil
}

func parseToolArguments(arguments string) []string {
	arguments = strings.TrimSpace(arguments)
	if arguments == "" {
		return nil
	}
	return strings.Fields(arguments)
}

func saveToolchainPack(workspace string, packID string, request ToolchainPackUpdateRequestDTO) (ToolchainPackDTO, error) {
	manifest, err := toolchainManifestForPack(workspace, packID)
	if err != nil {
		return ToolchainPackDTO{}, err
	}
	manifest.Name = strings.TrimSpace(request.Name)
	manifest.Version = strings.TrimSpace(request.Version)
	manifest.Capabilities = trimmedNonEmptyStrings(request.Capabilities)
	manifest.Runner = strings.TrimSpace(request.Runner)
	manifest.RunCommand = trimmedNonEmptyStrings(request.RunCommand)
	manifest.RunTimeoutMS = request.RunTimeoutMS
	manifest.Parser = strings.TrimSpace(request.Parser)
	manifest.Healthcheck = strings.TrimSpace(request.Healthcheck)
	manifest.HealthCommand = trimmedNonEmptyStrings(request.HealthCommand)
	if err := toolchain.ValidateManifest(manifest); err != nil {
		return ToolchainPackDTO{}, fmt.Errorf("invalid manifest: %w", err)
	}
	if err := writeToolchainManifest(manifest.Path, manifest); err != nil {
		return ToolchainPackDTO{}, err
	}
	toolchainDTO, err := ProjectToolchain(workspace)
	if err != nil {
		return ToolchainPackDTO{}, err
	}
	for _, pack := range toolchainDTO.Packs {
		if pack.ID == strings.TrimSpace(packID) {
			return pack, nil
		}
	}
	return ToolchainPackDTO{}, fmt.Errorf("saved toolchain pack not found: %s", packID)
}

func toolchainManifestForPack(workspace string, packID string) (toolchain.Manifest, error) {
	packID = strings.TrimSpace(packID)
	if packID == "" {
		return toolchain.Manifest{}, fmt.Errorf("toolchain pack id is required")
	}
	manifests, err := toolchain.LoadManifests(filepath.Join(workspace, "toolchain"))
	if err != nil {
		return toolchain.Manifest{}, err
	}
	for _, manifest := range manifests {
		if toolchainPackID(manifest) == packID {
			return manifest, nil
		}
	}
	return toolchain.Manifest{}, fmt.Errorf("toolchain pack not found: %s", packID)
}

func toolchainPackID(manifest toolchain.Manifest) string {
	return strings.TrimSuffix(filepath.Base(filepath.Dir(manifest.Path)), "-pack")
}

func writeToolchainManifest(path string, manifest toolchain.Manifest) error {
	if info, err := os.Lstat(path); err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("toolchain manifest must not be a symlink")
		}
		if info.IsDir() {
			return fmt.Errorf("toolchain manifest is a directory")
		}
	} else {
		return fmt.Errorf("stat toolchain manifest: %w", err)
	}
	data, err := yaml.Marshal(manifest)
	if err != nil {
		return fmt.Errorf("encode toolchain manifest: %w", err)
	}
	dir := filepath.Dir(path)
	temp, err := os.CreateTemp(dir, ".manifest-*.yaml")
	if err != nil {
		return fmt.Errorf("create toolchain manifest temp file: %w", err)
	}
	tempName := temp.Name()
	defer func() {
		_ = os.Remove(tempName)
	}()
	if _, err := temp.Write(data); err != nil {
		_ = temp.Close()
		return fmt.Errorf("write toolchain manifest temp file: %w", err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("close toolchain manifest temp file: %w", err)
	}
	if err := os.Rename(tempName, path); err != nil {
		return fmt.Errorf("write toolchain manifest: %w", err)
	}
	return nil
}

func trimmedNonEmptyStrings(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			result = append(result, value)
		}
	}
	return result
}
