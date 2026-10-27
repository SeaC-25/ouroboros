package app

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type ToolchainInstallOrchestrationOptions struct {
	Execute bool
}

type ToolchainInstallOrchestration struct {
	Status              string                         `json:"status"`
	Mode                string                         `json:"mode"`
	Path                string                         `json:"path,omitempty"`
	RequestSummary      ToolchainInstallRequestSummary `json:"request_summary"`
	DispatchReady       int                            `json:"dispatch_ready"`
	DispatchSkipped     int                            `json:"dispatch_skipped"`
	RetryReady          int                            `json:"retry_ready"`
	RetrySkipped        int                            `json:"retry_skipped"`
	DispatchPath        string                         `json:"dispatch_path,omitempty"`
	DispatchStatus      string                         `json:"dispatch_status,omitempty"`
	ExecutionPath       string                         `json:"execution_path,omitempty"`
	ExecutionStatus     string                         `json:"execution_status,omitempty"`
	ExecutionResults    int                            `json:"execution_results,omitempty"`
	ExecutionSkipped    int                            `json:"execution_skipped,omitempty"`
	RetryDispatchPath   string                         `json:"retry_dispatch_path,omitempty"`
	RetryDispatchStatus string                         `json:"retry_dispatch_status,omitempty"`
	Warnings            []string                       `json:"warnings,omitempty"`
	Error               string                         `json:"error,omitempty"`
}

func CreateToolchainInstallOrchestration(root string, options ToolchainInstallOrchestrationOptions) (ToolchainInstallOrchestration, error) {
	dispatchPlan, err := PlanApprovedToolchainInstalls(root)
	if err != nil {
		return ToolchainInstallOrchestration{}, err
	}
	retryPlan, err := PlanFailedToolchainInstallRetries(root)
	if err != nil {
		return ToolchainInstallOrchestration{}, err
	}
	orchestration := ToolchainInstallOrchestration{
		Status:          "dry-run",
		Mode:            "metadata-only",
		RequestSummary:  dispatchPlan.Summary,
		DispatchReady:   len(dispatchPlan.Ready),
		DispatchSkipped: len(dispatchPlan.Skipped),
		RetryReady:      retryPlan.Summary.Ready,
		RetrySkipped:    retryPlan.Summary.Skipped,
	}
	if !options.Execute {
		if err := writeToolchainInstallOrchestration(root, &orchestration); err != nil {
			return orchestration, err
		}
		return orchestration, nil
	}

	orchestration.Status = "completed"
	orchestration.Mode = "execute"
	dispatch, err := createToolchainInstallDispatch(root, dispatchPlan)
	if err != nil {
		orchestration.Status = "failed"
		orchestration.Error = err.Error()
		if writeErr := writeToolchainInstallOrchestration(root, &orchestration); writeErr != nil {
			return orchestration, writeErr
		}
		return orchestration, err
	}
	orchestration.DispatchPath = dispatch.Path
	orchestration.DispatchStatus = dispatch.Status
	orchestration.DispatchReady = len(dispatch.Ready)
	orchestration.DispatchSkipped = len(dispatch.Skipped)

	execution, execErr := ExecuteToolchainInstallDispatch(root, dispatch.Path, ToolchainInstallDispatchExecutionOptions{Execute: true})
	orchestration.ExecutionPath = execution.Path
	orchestration.ExecutionStatus = execution.Status
	orchestration.ExecutionResults = len(execution.Results)
	orchestration.ExecutionSkipped = len(execution.Skipped)
	orchestration.Warnings = append([]string(nil), execution.Warnings...)
	if execErr != nil {
		orchestration.Status = "failed"
		orchestration.Error = execErr.Error()
		if retryErr := attachInstallOrchestrationRetryDispatch(root, &orchestration); retryErr != nil {
			orchestration.Error = strings.TrimSpace(orchestration.Error + "; " + retryErr.Error())
			if writeErr := writeToolchainInstallOrchestration(root, &orchestration); writeErr != nil {
				return orchestration, writeErr
			}
			return orchestration, fmt.Errorf("%w; %v", execErr, retryErr)
		}
		if writeErr := writeToolchainInstallOrchestration(root, &orchestration); writeErr != nil {
			return orchestration, writeErr
		}
		return orchestration, execErr
	}

	retryPlan, err = PlanFailedToolchainInstallRetries(root)
	if err != nil {
		orchestration.Status = "failed"
		orchestration.Error = err.Error()
		if writeErr := writeToolchainInstallOrchestration(root, &orchestration); writeErr != nil {
			return orchestration, writeErr
		}
		return orchestration, err
	}
	orchestration.RetryReady = retryPlan.Summary.Ready
	orchestration.RetrySkipped = retryPlan.Summary.Skipped
	if err := writeToolchainInstallOrchestration(root, &orchestration); err != nil {
		return orchestration, err
	}
	return orchestration, nil
}

func attachInstallOrchestrationRetryDispatch(root string, orchestration *ToolchainInstallOrchestration) error {
	retryPlan, err := PlanFailedToolchainInstallRetries(root)
	if err != nil {
		return err
	}
	orchestration.RetryReady = retryPlan.Summary.Ready
	orchestration.RetrySkipped = retryPlan.Summary.Skipped
	if retryPlan.Summary.Ready == 0 {
		return nil
	}
	retryDispatch, err := CreateToolchainInstallRetryDispatch(root)
	if err != nil {
		return err
	}
	orchestration.RetryDispatchPath = retryDispatch.Path
	orchestration.RetryDispatchStatus = retryDispatch.Status
	orchestration.RetryReady = retryDispatch.Summary.Ready
	orchestration.RetrySkipped = retryDispatch.Summary.Skipped
	return nil
}

func ListToolchainInstallOrchestrations(root string) ([]ToolchainInstallOrchestration, error) {
	doctor, err := doctorWorkspaceMetadataOnly(root)
	if err != nil {
		return nil, err
	}
	if !doctor.Ready {
		return nil, fmt.Errorf("workspace is not ready: %v", doctor.Errors)
	}
	paths, err := filepath.Glob(filepath.Join(toolchainInstallOrchestrationDir(root), "*.json"))
	if err != nil {
		return nil, fmt.Errorf("glob install orchestrations: %w", err)
	}
	sort.Strings(paths)
	orchestrations := make([]ToolchainInstallOrchestration, 0, len(paths))
	for _, path := range paths {
		orchestration, err := readToolchainInstallOrchestrationFile(path)
		if err != nil {
			return nil, err
		}
		orchestrations = append(orchestrations, orchestration)
	}
	return orchestrations, nil
}

func ReadToolchainInstallOrchestration(root, orchestrationPath string) (ToolchainInstallOrchestration, error) {
	doctor, err := doctorWorkspaceMetadataOnly(root)
	if err != nil {
		return ToolchainInstallOrchestration{}, err
	}
	if !doctor.Ready {
		return ToolchainInstallOrchestration{}, fmt.Errorf("workspace is not ready: %v", doctor.Errors)
	}
	path, err := resolveToolchainInstallOrchestrationPath(root, orchestrationPath)
	if err != nil {
		return ToolchainInstallOrchestration{}, err
	}
	return readToolchainInstallOrchestrationFile(path)
}

func writeToolchainInstallOrchestration(root string, orchestration *ToolchainInstallOrchestration) error {
	dir := toolchainInstallOrchestrationDir(root)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create install orchestration directory: %w", err)
	}
	if strings.TrimSpace(orchestration.Path) == "" {
		orchestration.Path = filepath.Join(dir, safeInstallOrchestrationName(*orchestration)+".json")
	}
	data, err := json.MarshalIndent(orchestration, "", "  ")
	if err != nil {
		return fmt.Errorf("encode install orchestration: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(orchestration.Path, data, 0o644); err != nil {
		return fmt.Errorf("write install orchestration: %w", err)
	}
	return nil
}

func readToolchainInstallOrchestrationFile(path string) (ToolchainInstallOrchestration, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return ToolchainInstallOrchestration{}, fmt.Errorf("read install orchestration %s: %w", path, err)
	}
	var orchestration ToolchainInstallOrchestration
	if err := json.Unmarshal(data, &orchestration); err != nil {
		return ToolchainInstallOrchestration{}, fmt.Errorf("decode install orchestration %s: %w", path, err)
	}
	orchestration.Path = path
	return orchestration, nil
}

func resolveToolchainInstallOrchestrationPath(root, orchestrationPath string) (string, error) {
	trimmed := strings.TrimSpace(orchestrationPath)
	if trimmed == "" {
		return "", fmt.Errorf("install orchestration path is required")
	}
	dir, err := filepath.Abs(toolchainInstallOrchestrationDir(root))
	if err != nil {
		return "", fmt.Errorf("resolve install orchestration directory: %w", err)
	}
	candidate := trimmed
	if !filepath.IsAbs(candidate) {
		candidate = filepath.Join(dir, candidate)
	}
	candidate, err = filepath.Abs(candidate)
	if err != nil {
		return "", fmt.Errorf("resolve install orchestration path: %w", err)
	}
	rel, err := filepath.Rel(dir, candidate)
	if err != nil {
		return "", fmt.Errorf("resolve install orchestration relative path: %w", err)
	}
	if rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("install orchestration path must stay under %s", dir)
	}
	if filepath.Ext(candidate) != ".json" {
		return "", fmt.Errorf("install orchestration path must be a json file")
	}
	return candidate, nil
}

func toolchainInstallOrchestrationDir(root string) string {
	return filepath.Join(root, "toolchain", "install-orchestrations")
}

func safeInstallOrchestrationName(orchestration ToolchainInstallOrchestration) string {
	return safeInstallRequestName(strings.Join([]string{
		"install-orchestration",
		orchestration.Mode,
		orchestration.Status,
		fmt.Sprintf("dispatch-ready-%d", orchestration.DispatchReady),
		fmt.Sprintf("retry-ready-%d", orchestration.RetryReady),
		time.Now().UTC().Format(time.RFC3339Nano),
	}, "|"))
}
