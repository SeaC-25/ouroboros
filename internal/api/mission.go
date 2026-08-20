package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/ouroboros/ouroboros/internal/eventlog"
	"gopkg.in/yaml.v3"
)

const missionConfigName = "mission.yaml"

func (s *Server) handleMission(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		mission, err := ProjectMission(s.workspace)
		if err != nil {
			respondError(w, http.StatusInternalServerError, fmt.Sprintf("读取任务失败: %v", err))
			return
		}
		respondJSON(w, http.StatusOK, mission)
	case http.MethodPut:
		var request MissionUpdateRequestDTO
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			respondError(w, http.StatusBadRequest, fmt.Sprintf("解析任务失败: %v", err))
			return
		}
		mission, err := SaveMission(s.workspace, request)
		if err != nil {
			respondError(w, http.StatusBadRequest, fmt.Sprintf("保存任务失败: %v", err))
			return
		}
		respondJSON(w, http.StatusOK, mission)
	default:
		respondError(w, http.StatusMethodNotAllowed, "不支持的任务方法")
	}
}

func (s *Server) handleMissionStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		respondError(w, http.StatusMethodNotAllowed, "不支持的任务启动方法")
		return
	}
	var request MissionUpdateRequestDTO
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		respondError(w, http.StatusBadRequest, fmt.Sprintf("解析任务启动失败: %v", err))
		return
	}
	response, err := s.StartMission(request)
	if err != nil {
		respondError(w, http.StatusBadRequest, fmt.Sprintf("启动任务失败: %v", err))
		return
	}
	respondJSON(w, http.StatusAccepted, response)
}

func ProjectMission(workspace string) (MissionDTO, error) {
	path := filepath.Join(workspace, missionConfigName)
	mission := MissionDTO{Path: missionConfigName, Status: "missing"}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return mission, nil
		}
		return MissionDTO{}, err
	}
	document := map[string]any{}
	if len(strings.TrimSpace(string(data))) > 0 {
		if err := yaml.Unmarshal(data, &document); err != nil {
			return MissionDTO{}, err
		}
	}
	fields := mapField(document, "mission")
	mission.Name = stringField(fields, "name")
	mission.Goal = stringField(fields, "goal")
	mission.Command = firstNonEmpty(stringField(fields, "command"), stringField(fields, "start_command"))
	mission.Status = "empty"
	if strings.TrimSpace(mission.Goal) != "" || strings.TrimSpace(mission.Command) != "" {
		mission.Status = "ready"
	}
	return mission, nil
}

func SaveMission(workspace string, request MissionUpdateRequestDTO) (MissionDTO, error) {
	if strings.TrimSpace(workspace) == "" {
		return MissionDTO{}, fmt.Errorf("workspace path is required")
	}
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		return MissionDTO{}, fmt.Errorf("create workspace: %w", err)
	}
	path := filepath.Join(workspace, missionConfigName)
	document := map[string]any{"version": 1}
	if data, err := os.ReadFile(path); err == nil && len(strings.TrimSpace(string(data))) > 0 {
		if err := yaml.Unmarshal(data, &document); err != nil {
			return MissionDTO{}, err
		}
	} else if err != nil && !os.IsNotExist(err) {
		return MissionDTO{}, err
	}
	fields := mapField(document, "mission")
	if fields == nil {
		fields = map[string]any{}
	}
	if strings.TrimSpace(request.Name) != "" {
		fields["name"] = strings.TrimSpace(request.Name)
	}
	fields["goal"] = strings.TrimSpace(request.Goal)
	fields["command"] = strings.TrimSpace(request.Command)
	document["mission"] = fields
	data, err := yaml.Marshal(document)
	if err != nil {
		return MissionDTO{}, err
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return MissionDTO{}, err
	}
	return ProjectMission(workspace)
}

func (s *Server) StartMission(request MissionUpdateRequestDTO) (MissionStartResponseDTO, error) {
	mission, err := SaveMission(s.workspace, request)
	if err != nil {
		return MissionStartResponseDTO{}, err
	}
	if strings.TrimSpace(mission.Command) == "" {
		return MissionStartResponseDTO{}, fmt.Errorf("mission command is required")
	}
	execution, err := s.StartExecution(ExecutionCreateRequestDTO{Command: mission.Command})
	if err != nil {
		return MissionStartResponseDTO{}, err
	}
	s.appendMissionEvent(execution.ID, mission)
	s.BroadcastEvent("mission:started", map[string]any{
		"mission":   mission,
		"execution": execution,
	})
	return MissionStartResponseDTO{Mission: mission, Execution: execution}, nil
}

func (s *Server) appendMissionEvent(runID string, mission MissionDTO) {
	if s.eventStore == nil {
		return
	}
	payload := map[string]any{
		"mission_name": mission.Name,
		"goal":         mission.Goal,
		"command":      mission.Command,
		"path":         mission.Path,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return
	}
	_, _ = s.eventStore.Append(context.Background(), eventlog.Event{RunID: runID, Type: eventlog.EventGoal, Payload: data})
}
