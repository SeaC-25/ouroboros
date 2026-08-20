package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/ouroboros/ouroboros/internal/eventlog"
)

const (
	executionRootName        = "executions"
	executionMetadataName    = "execution.json"
	executionLogName         = "output.log"
	executionArtifactDirName = "artifacts"
)

type executionMetadata struct {
	ID          string            `json:"id"`
	RetryOf     string            `json:"retryOf,omitempty"`
	Command     string            `json:"command"`
	Args        []string          `json:"args,omitempty"`
	CWD         string            `json:"cwd,omitempty"`
	Env         map[string]string `json:"env,omitempty"`
	Status      string            `json:"status"`
	StartedAt   int64             `json:"startedAt"`
	CompletedAt int64             `json:"completedAt,omitempty"`
	ExitCode    *int              `json:"exitCode,omitempty"`
	LogPath     string            `json:"logPath"`
	ArtifactDir string            `json:"artifactDir"`
}

func (s *Server) handleExecutions(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		executions, err := ProjectExecutions(s.workspace)
		if err != nil {
			respondError(w, http.StatusInternalServerError, fmt.Sprintf("获取执行列表失败: %v", err))
			return
		}
		respondJSON(w, http.StatusOK, map[string]any{"executions": executions})
	case http.MethodPost:
		var request ExecutionCreateRequestDTO
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			respondError(w, http.StatusBadRequest, fmt.Sprintf("解析执行请求失败: %v", err))
			return
		}
		execution, err := s.StartExecution(request)
		if err != nil {
			respondError(w, http.StatusBadRequest, fmt.Sprintf("启动执行失败: %v", err))
			return
		}
		respondJSON(w, http.StatusAccepted, execution)
	default:
		respondError(w, http.StatusMethodNotAllowed, "不支持的执行方法")
	}
}

func (s *Server) handleExecutionByID(w http.ResponseWriter, r *http.Request) {
	id, action, err := parseExecutionRoute(r.URL.Path)
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	switch action {
	case "":
		if r.Method != http.MethodGet {
			respondError(w, http.StatusMethodNotAllowed, "不支持的执行方法")
			return
		}
		execution, err := ReadExecution(s.workspace, id)
		respondExecutionResult(w, execution, err)
	case "logs":
		if r.Method != http.MethodGet {
			respondError(w, http.StatusMethodNotAllowed, "不支持的执行日志方法")
			return
		}
		logs, err := ReadExecutionLogs(s.workspace, id)
		if err != nil {
			respondExecutionError(w, err)
			return
		}
		respondJSON(w, http.StatusOK, logs)
	case "artifacts":
		if r.Method != http.MethodGet {
			respondError(w, http.StatusMethodNotAllowed, "不支持的执行产物方法")
			return
		}
		artifacts, err := ListExecutionArtifacts(s.workspace, id)
		if err != nil {
			respondExecutionError(w, err)
			return
		}
		respondJSON(w, http.StatusOK, map[string]any{"artifacts": artifacts})
	case "stop":
		if r.Method != http.MethodPost {
			respondError(w, http.StatusMethodNotAllowed, "不支持的停止方法")
			return
		}
		execution, err := s.StopExecution(id)
		respondExecutionResult(w, execution, err)
	case "retry":
		if r.Method != http.MethodPost {
			respondError(w, http.StatusMethodNotAllowed, "不支持的重试方法")
			return
		}
		execution, err := s.RetryExecution(id)
		if err != nil {
			respondExecutionError(w, err)
			return
		}
		respondJSON(w, http.StatusAccepted, execution)
	default:
		respondError(w, http.StatusNotFound, "执行资源不存在")
	}
}

func respondExecutionResult(w http.ResponseWriter, execution ExecutionDTO, err error) {
	if err != nil {
		respondExecutionError(w, err)
		return
	}
	respondJSON(w, http.StatusOK, execution)
}

func respondExecutionError(w http.ResponseWriter, err error) {
	status := http.StatusBadRequest
	if errors.Is(err, os.ErrNotExist) {
		status = http.StatusNotFound
	}
	respondError(w, status, err.Error())
}

func parseExecutionRoute(requestPath string) (string, string, error) {
	tail := strings.Trim(strings.TrimPrefix(requestPath, "/api/executions/"), "/")
	if tail == "" || tail == requestPath {
		return "", "", fmt.Errorf("execution id is required")
	}
	parts := strings.Split(tail, "/")
	id := strings.TrimSpace(parts[0])
	if err := validateExecutionID(id); err != nil {
		return "", "", err
	}
	if len(parts) == 1 {
		return id, "", nil
	}
	return id, strings.TrimSpace(parts[1]), nil
}

func (s *Server) StartExecution(request ExecutionCreateRequestDTO) (ExecutionDTO, error) {
	request.Command = strings.TrimSpace(request.Command)
	if request.Command == "" {
		return ExecutionDTO{}, fmt.Errorf("execution command is required")
	}
	workspaceRoot, err := filepath.Abs(s.workspace)
	if err != nil {
		return ExecutionDTO{}, err
	}
	if err := os.MkdirAll(filepath.Join(workspaceRoot, executionRootName), 0o755); err != nil {
		return ExecutionDTO{}, fmt.Errorf("create execution root: %w", err)
	}

	id := fmt.Sprintf("exec-%d", time.Now().UTC().UnixNano())
	executionDir := filepath.Join(workspaceRoot, executionRootName, id)
	artifactDir := filepath.Join(executionDir, executionArtifactDirName)
	if err := os.MkdirAll(artifactDir, 0o755); err != nil {
		return ExecutionDTO{}, fmt.Errorf("create execution artifacts: %w", err)
	}
	logPath := filepath.Join(executionDir, executionLogName)
	relLogPath, err := filepath.Rel(workspaceRoot, logPath)
	if err != nil {
		return ExecutionDTO{}, err
	}
	relArtifactDir, err := filepath.Rel(workspaceRoot, artifactDir)
	if err != nil {
		return ExecutionDTO{}, err
	}
	startedAt := time.Now().UTC().Unix()
	meta := executionMetadata{
		ID:          id,
		RetryOf:     strings.TrimSpace(request.RetryOf),
		Command:     request.Command,
		Args:        append([]string(nil), request.Args...),
		CWD:         strings.TrimSpace(request.CWD),
		Env:         copyStringMap(request.Env),
		Status:      "running",
		StartedAt:   startedAt,
		LogPath:     filepath.ToSlash(relLogPath),
		ArtifactDir: filepath.ToSlash(relArtifactDir),
	}
	if err := writeExecutionMetadata(workspaceRoot, meta); err != nil {
		return ExecutionDTO{}, err
	}

	ctx, cancel := context.WithCancel(context.Background())
	cmd := executionCommand(ctx, request)
	workDir, err := resolveExecutionCWD(workspaceRoot, request.CWD)
	if err != nil {
		cancel()
		return ExecutionDTO{}, err
	}
	cmd.Dir = workDir
	cmd.Env = executionEnv(workspaceRoot, id, artifactDir, request.Env)

	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		cancel()
		return ExecutionDTO{}, fmt.Errorf("open execution log: %w", err)
	}
	cmd.Stdout = logFile
	cmd.Stderr = logFile

	if err := cmd.Start(); err != nil {
		_ = logFile.Close()
		cancel()
		meta.Status = "failed"
		now := time.Now().UTC().Unix()
		meta.CompletedAt = now
		if writeErr := writeExecutionMetadata(workspaceRoot, meta); writeErr != nil {
			return ExecutionDTO{}, writeErr
		}
		s.appendExecutionEvent(id, meta, "failed", fmt.Sprintf("start process: %v", err))
		return meta.toDTO(), fmt.Errorf("start process: %w", err)
	}

	s.registerExecution(id, cancel)
	s.appendExecutionEvent(id, meta, "started", "")
	s.broadcastExecutionStatus("started", meta)

	go func() {
		err := cmd.Wait()
		_ = logFile.Close()
		s.unregisterExecution(id)
		final := meta
		final.CompletedAt = time.Now().UTC().Unix()
		if cmd.ProcessState != nil {
			exitCode := cmd.ProcessState.ExitCode()
			final.ExitCode = &exitCode
		}
		switch {
		case ctx.Err() != nil:
			final.Status = "stopped"
		case err != nil:
			final.Status = "failed"
		default:
			final.Status = "completed"
		}
		_ = writeExecutionMetadata(workspaceRoot, final)
		s.appendExecutionEvent(id, final, final.Status, readExecutionLogPreview(logPath))
		s.broadcastExecutionStatus(final.Status, final)
	}()

	return meta.toDTO(), nil
}

func (s *Server) StopExecution(id string) (ExecutionDTO, error) {
	if err := validateExecutionID(id); err != nil {
		return ExecutionDTO{}, err
	}
	s.execMu.Lock()
	cancel := s.executions[id]
	s.execMu.Unlock()
	if cancel != nil {
		cancel()
		if meta, err := readExecutionMetadata(s.workspace, id); err == nil {
			s.appendExecutionEvent(id, meta, "stop_requested", "")
			s.broadcastExecutionStatus("stop_requested", meta)
		}
	}
	return ReadExecution(s.workspace, id)
}

func (s *Server) RetryExecution(id string) (ExecutionDTO, error) {
	meta, err := readExecutionMetadata(s.workspace, id)
	if err != nil {
		return ExecutionDTO{}, err
	}
	return s.StartExecution(ExecutionCreateRequestDTO{
		Command: meta.Command,
		Args:    append([]string(nil), meta.Args...),
		CWD:     meta.CWD,
		Env:     copyStringMap(meta.Env),
		RetryOf: id,
	})
}

func (s *Server) registerExecution(id string, cancel context.CancelFunc) {
	s.execMu.Lock()
	defer s.execMu.Unlock()
	if s.executions == nil {
		s.executions = map[string]context.CancelFunc{}
	}
	s.executions[id] = cancel
}

func (s *Server) unregisterExecution(id string) {
	s.execMu.Lock()
	defer s.execMu.Unlock()
	delete(s.executions, id)
}

func (s *Server) stopActiveExecutions() {
	s.execMu.Lock()
	cancels := make([]context.CancelFunc, 0, len(s.executions))
	for _, cancel := range s.executions {
		if cancel != nil {
			cancels = append(cancels, cancel)
		}
	}
	s.execMu.Unlock()
	for _, cancel := range cancels {
		cancel()
	}
}

func (s *Server) appendExecutionEvent(runID string, meta executionMetadata, status string, transcript string) {
	if s.eventStore == nil {
		return
	}
	payload := map[string]any{
		"execution_id":     meta.ID,
		"retry_of":         meta.RetryOf,
		"command":          executionDisplayCommand(meta),
		"execution_status": status,
		"log_path":         meta.LogPath,
		"artifact_dir":     meta.ArtifactDir,
	}
	if value := strings.TrimSpace(meta.Env["OURO_TOOL_ACTION_ID"]); value != "" {
		payload["tool_action_id"] = value
	}
	if value := strings.TrimSpace(meta.Env["OURO_TOOLCHAIN_PACK_ID"]); value != "" {
		payload["toolchain_pack_id"] = value
	}
	if value := strings.TrimSpace(meta.Env["OURO_TOOL_ARGUMENTS"]); value != "" {
		payload["tool_arguments"] = value
	}
	if meta.ExitCode != nil {
		payload["exit_code"] = *meta.ExitCode
	}
	if strings.TrimSpace(transcript) != "" {
		payload["transcript"] = transcript
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return
	}
	_, _ = s.eventStore.Append(context.Background(), eventlog.Event{RunID: runID, Type: eventlog.EventTool, Payload: data})
}

func (s *Server) broadcastExecutionStatus(status string, meta executionMetadata) {
	eventStatus := strings.TrimSpace(status)
	if eventStatus == "completed" {
		eventStatus = "completed"
	}
	if eventStatus == "" {
		eventStatus = "updated"
	}
	s.BroadcastEvent("execution:"+eventStatus, meta.toDTO())
}

func ProjectExecutions(workspace string) ([]ExecutionDTO, error) {
	root := filepath.Join(workspace, executionRootName)
	if info, err := os.Stat(root); err != nil || !info.IsDir() {
		return []ExecutionDTO{}, nil
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	executions := []ExecutionDTO{}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if err := validateExecutionID(entry.Name()); err != nil {
			continue
		}
		meta, err := readExecutionMetadata(workspace, entry.Name())
		if err != nil {
			continue
		}
		executions = append(executions, meta.toDTO())
	}
	sort.SliceStable(executions, func(i, j int) bool {
		if executions[i].StartedAt == executions[j].StartedAt {
			return executions[i].ID > executions[j].ID
		}
		return executions[i].StartedAt > executions[j].StartedAt
	})
	return executions, nil
}

func ReadExecution(workspace string, id string) (ExecutionDTO, error) {
	meta, err := readExecutionMetadata(workspace, id)
	if err != nil {
		return ExecutionDTO{}, err
	}
	return meta.toDTO(), nil
}

func ReadExecutionLogs(workspace string, id string) (ExecutionLogDTO, error) {
	meta, err := readExecutionMetadata(workspace, id)
	if err != nil {
		return ExecutionLogDTO{}, err
	}
	path := filepath.Join(workspace, filepath.FromSlash(meta.LogPath))
	data, err := os.ReadFile(path)
	if err != nil {
		return ExecutionLogDTO{}, err
	}
	return ExecutionLogDTO{ID: id, Path: meta.LogPath, Content: string(data)}, nil
}

func ListExecutionArtifacts(workspace string, id string) ([]ExecutionArtifactDTO, error) {
	meta, err := readExecutionMetadata(workspace, id)
	if err != nil {
		return nil, err
	}
	root := filepath.Join(workspace, filepath.FromSlash(meta.ArtifactDir))
	if info, err := os.Stat(root); err != nil || !info.IsDir() {
		return []ExecutionArtifactDTO{}, nil
	}
	artifacts := []ExecutionArtifactDTO{}
	err = filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		workspaceRel, err := filepath.Rel(workspace, path)
		if err != nil {
			return err
		}
		artifacts = append(artifacts, ExecutionArtifactDTO{
			Path:           filepath.ToSlash(rel),
			FilePath:       filepath.ToSlash(workspaceRel),
			Name:           entry.Name(),
			Size:           info.Size(),
			ModifiedAt:     info.ModTime().Unix(),
			ContentPreview: readArtifactPreview(path, info.Size()),
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.SliceStable(artifacts, func(i, j int) bool { return artifacts[i].Path < artifacts[j].Path })
	return artifacts, nil
}

func readExecutionMetadata(workspace string, id string) (executionMetadata, error) {
	if err := validateExecutionID(id); err != nil {
		return executionMetadata{}, err
	}
	path := filepath.Join(workspace, executionRootName, id, executionMetadataName)
	data, err := os.ReadFile(path)
	if err != nil {
		return executionMetadata{}, err
	}
	var meta executionMetadata
	if err := json.Unmarshal(data, &meta); err != nil {
		return executionMetadata{}, err
	}
	if meta.ID == "" {
		meta.ID = id
	}
	return meta, nil
}

func writeExecutionMetadata(workspace string, meta executionMetadata) error {
	if err := validateExecutionID(meta.ID); err != nil {
		return err
	}
	path := filepath.Join(workspace, executionRootName, meta.ID, executionMetadataName)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

func validateExecutionID(id string) error {
	if strings.TrimSpace(id) == "" {
		return fmt.Errorf("execution id is required")
	}
	for _, r := range id {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			continue
		case r == '-', r == '_', r == '.':
			continue
		default:
			return fmt.Errorf("execution id has invalid character: %q", r)
		}
	}
	return nil
}

func executionCommand(ctx context.Context, request ExecutionCreateRequestDTO) *exec.Cmd {
	if len(request.Args) > 0 {
		return exec.CommandContext(ctx, request.Command, request.Args...)
	}
	if runtime.GOOS == "windows" {
		return exec.CommandContext(ctx, "cmd", "/C", request.Command)
	}
	return exec.CommandContext(ctx, "sh", "-lc", request.Command)
}

func resolveExecutionCWD(workspace string, cwd string) (string, error) {
	if strings.TrimSpace(cwd) == "" {
		return workspace, nil
	}
	if filepath.IsAbs(cwd) {
		return "", fmt.Errorf("execution cwd must be relative to workspace")
	}
	path := filepath.Join(workspace, filepath.Clean(cwd))
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(workspace, absPath)
	if err != nil {
		return "", err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("execution cwd must stay under workspace")
	}
	info, err := os.Stat(absPath)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("execution cwd is not a directory: %s", cwd)
	}
	return absPath, nil
}

func executionEnv(workspace string, id string, artifactDir string, extra map[string]string) []string {
	env := append([]string(nil), os.Environ()...)
	env = append(env,
		"OURO_WORKSPACE="+workspace,
		"OURO_EXECUTION_ID="+id,
		"OURO_ARTIFACT_DIR="+artifactDir,
	)
	keys := make([]string, 0, len(extra))
	for key := range extra {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if strings.TrimSpace(key) == "" || strings.Contains(key, "=") {
			continue
		}
		env = append(env, key+"="+extra[key])
	}
	return env
}

func (meta executionMetadata) toDTO() ExecutionDTO {
	displayCommand := executionDisplayCommand(meta)
	return ExecutionDTO{
		ID:          meta.ID,
		Label:       displayCommand,
		RetryOf:     meta.RetryOf,
		Command:     displayCommand,
		Status:      meta.Status,
		StartedAt:   meta.StartedAt,
		CompletedAt: meta.CompletedAt,
		ExitCode:    meta.ExitCode,
		LogPath:     meta.LogPath,
		ArtifactDir: meta.ArtifactDir,
	}
}

func executionDisplayCommand(meta executionMetadata) string {
	parts := append([]string{meta.Command}, meta.Args...)
	return commandText(parts)
}

func copyStringMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func readExecutionLogPreview(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	text := string(data)
	if len(text) <= 4096 {
		return text
	}
	return text[len(text)-4096:]
}

func readArtifactPreview(path string, size int64) string {
	if size > 64*1024 {
		return ""
	}
	data, err := os.ReadFile(path)
	if err != nil || strings.IndexByte(string(data), 0) >= 0 {
		return ""
	}
	text := string(data)
	if len(text) > 4096 {
		return text[:4096]
	}
	return text
}
