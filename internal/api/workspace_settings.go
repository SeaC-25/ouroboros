package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type workspaceSettingsFile struct {
	Version   int                       `yaml:"version"`
	Workspace workspaceSettingsFileBody `yaml:"workspace"`
}

type workspaceSettingsFileBody struct {
	Name        string   `yaml:"name"`
	Description string   `yaml:"description,omitempty"`
	Operator    string   `yaml:"operator,omitempty"`
	Tags        []string `yaml:"tags,omitempty"`
}

func (s *Server) handleWorkspaceSettings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		settings, err := ProjectWorkspaceSettings(s.workspace)
		if err != nil {
			respondError(w, http.StatusInternalServerError, fmt.Sprintf("读取工作区设置失败: %v", err))
			return
		}
		respondJSON(w, http.StatusOK, settings)
	case http.MethodPut:
		var request WorkspaceSettingsUpdateRequestDTO
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			respondError(w, http.StatusBadRequest, fmt.Sprintf("解析工作区设置失败: %v", err))
			return
		}
		settings, err := SaveWorkspaceSettings(s.workspace, request)
		if err != nil {
			respondError(w, http.StatusBadRequest, fmt.Sprintf("保存工作区设置失败: %v", err))
			return
		}
		s.BroadcastEvent("workspace:changed", settings)
		respondJSON(w, http.StatusOK, settings)
	default:
		respondError(w, http.StatusMethodNotAllowed, "不支持的工作区设置方法")
	}
}

func ProjectWorkspaceSettings(workspace string) (WorkspaceDTO, error) {
	dto := projectWorkspaceBase(workspace)
	dto.ConfigPath = filepath.Join(workspace, "workspace.yaml")
	config, found, err := loadWorkspaceSettings(dto.ConfigPath)
	if err != nil {
		return WorkspaceDTO{}, err
	}
	if !found {
		return dto, nil
	}
	if name := strings.TrimSpace(config.Workspace.Name); name != "" {
		dto.Name = name
	}
	dto.Description = strings.TrimSpace(config.Workspace.Description)
	dto.Operator = strings.TrimSpace(config.Workspace.Operator)
	dto.Tags = trimmedNonEmptyStrings(config.Workspace.Tags)
	return dto, nil
}

func SaveWorkspaceSettings(workspace string, request WorkspaceSettingsUpdateRequestDTO) (WorkspaceDTO, error) {
	path := filepath.Join(workspace, "workspace.yaml")
	config := workspaceSettingsFile{
		Version: 1,
		Workspace: workspaceSettingsFileBody{
			Name:        strings.TrimSpace(request.Name),
			Description: strings.TrimSpace(request.Description),
			Operator:    strings.TrimSpace(request.Operator),
			Tags:        trimmedNonEmptyStrings(request.Tags),
		},
	}
	if config.Workspace.Name == "" {
		config.Workspace.Name = projectWorkspaceBase(workspace).Name
	}
	if err := writeWorkspaceSettings(path, config); err != nil {
		return WorkspaceDTO{}, err
	}
	return ProjectWorkspaceSettings(workspace)
}

func loadWorkspaceSettings(path string) (workspaceSettingsFile, bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return workspaceSettingsFile{}, false, nil
		}
		return workspaceSettingsFile{}, false, err
	}
	var config workspaceSettingsFile
	if err := yaml.Unmarshal(data, &config); err != nil {
		return workspaceSettingsFile{}, true, err
	}
	return config, true, nil
}

func writeWorkspaceSettings(path string, config workspaceSettingsFile) error {
	if info, err := os.Lstat(path); err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("workspace.yaml must not be a symlink")
		}
		if info.IsDir() {
			return fmt.Errorf("workspace.yaml is a directory")
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat workspace settings: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create workspace settings directory: %w", err)
	}
	data, err := yaml.Marshal(config)
	if err != nil {
		return fmt.Errorf("encode workspace settings: %w", err)
	}
	temp, err := os.CreateTemp(filepath.Dir(path), ".workspace-*.yaml")
	if err != nil {
		return fmt.Errorf("create workspace settings temp file: %w", err)
	}
	tempName := temp.Name()
	defer func() {
		_ = os.Remove(tempName)
	}()
	if _, err := temp.Write(data); err != nil {
		_ = temp.Close()
		return fmt.Errorf("write workspace settings temp file: %w", err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("close workspace settings temp file: %w", err)
	}
	if err := os.Rename(tempName, path); err != nil {
		return fmt.Errorf("write workspace settings: %w", err)
	}
	return nil
}
