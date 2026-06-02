package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/ouroboros/ouroboros/internal/eventlog"
)

type ReportProjectionDispatchOptions struct {
	Now    time.Time
	Actor  string
	Reason string
}

type ReportProjectionDispatchExecutionOptions struct {
	Execute bool
	Now     time.Time
	Actor   string
	Reason  string
}

type ReportProjectionDispatchExecutionAnnotation struct {
	Decision   string `json:"decision"`
	Reason     string `json:"reason,omitempty"`
	Reviewer   string `json:"reviewer,omitempty"`
	ReviewedAt string `json:"reviewed_at,omitempty"`
}

type ReportProjectionDispatchExecutionAnnotationOptions struct {
	Decision   string
	Reason     string
	Reviewer   string
	ReviewedAt time.Time
}

type ReportProjectionDispatchExecutionSelection struct {
	Path            string   `json:"path,omitempty"`
	Status          string   `json:"status"`
	Mode            string   `json:"mode"`
	ExecutionPath   string   `json:"execution_path,omitempty"`
	DispatchPath    string   `json:"dispatch_path,omitempty"`
	RunID           string   `json:"run_id,omitempty"`
	ReportPath      string   `json:"report_path,omitempty"`
	EventCount      int      `json:"event_count,omitempty"`
	Written         bool     `json:"written,omitempty"`
	SupersededPaths []string `json:"superseded_paths,omitempty"`
	SelectedBy      string   `json:"selected_by,omitempty"`
	SelectedAt      string   `json:"selected_at,omitempty"`
	ClearedBy       string   `json:"cleared_by,omitempty"`
	ClearedAt       string   `json:"cleared_at,omitempty"`
	Reason          string   `json:"reason,omitempty"`
}

type ReportProjectionDispatchExecutionSelectionOptions struct {
	SelectedBy string
	SelectedAt time.Time
	Reason     string
}

type ReportProjectionDispatchExecutionSelectionClearOptions struct {
	ClearedBy string
	ClearedAt time.Time
	Reason    string
}

type ReportProjectionDispatch struct {
	Version      int    `json:"version"`
	Path         string `json:"path,omitempty"`
	Status       string `json:"status"`
	Mode         string `json:"mode"`
	CreatedAt    string `json:"created_at"`
	Actor        string `json:"actor"`
	Reason       string `json:"reason,omitempty"`
	RunID        string `json:"run_id"`
	ReportPath   string `json:"report_path"`
	EventCount   int    `json:"event_count"`
	ReplayDigest string `json:"replay_digest"`
}

type ReportProjectionDispatchExecution struct {
	Version      int                                          `json:"version"`
	Path         string                                       `json:"path,omitempty"`
	Status       string                                       `json:"status"`
	Mode         string                                       `json:"mode"`
	ExecutedAt   string                                       `json:"executed_at"`
	Actor        string                                       `json:"actor"`
	Reason       string                                       `json:"reason,omitempty"`
	DispatchPath string                                       `json:"dispatch_path"`
	RunID        string                                       `json:"run_id"`
	ReportPath   string                                       `json:"report_path"`
	EventCount   int                                          `json:"event_count"`
	Written      bool                                         `json:"written"`
	Error        string                                       `json:"error,omitempty"`
	Annotation   *ReportProjectionDispatchExecutionAnnotation `json:"annotation,omitempty"`
}

func CreateReportProjectionDispatch(ctx context.Context, root, runID string, options ReportProjectionDispatchOptions) (ReportProjectionDispatch, error) {
	projection, err := ProjectRunReport(ctx, root, runID, ReportProjectionOptions{})
	if err != nil {
		return ReportProjectionDispatch{}, err
	}
	now := options.Now.UTC()
	if now.IsZero() {
		now = time.Now().UTC()
	}
	actor := strings.TrimSpace(options.Actor)
	if actor == "" {
		actor = "operator"
	}
	dispatch := ReportProjectionDispatch{
		Version:      1,
		Status:       "dry-run",
		Mode:         "metadata-only",
		CreatedAt:    now.Format(time.RFC3339Nano),
		Actor:        actor,
		Reason:       strings.TrimSpace(options.Reason),
		RunID:        projection.RunID,
		ReportPath:   projection.ReportPath,
		EventCount:   projection.EventCount,
		ReplayDigest: projection.ReplayDigest,
	}
	if err := writeReportProjectionDispatch(root, &dispatch); err != nil {
		return dispatch, err
	}
	return dispatch, nil
}

func ExecuteReportProjectionDispatch(ctx context.Context, root, dispatchPath string, options ReportProjectionDispatchExecutionOptions) (ReportProjectionDispatchExecution, error) {
	if !options.Execute {
		return ReportProjectionDispatchExecution{}, fmt.Errorf("report projection dispatch execution requires explicit execute")
	}
	dispatch, err := ReadReportProjectionDispatch(root, dispatchPath)
	if err != nil {
		return ReportProjectionDispatchExecution{}, err
	}
	if dispatch.Version != 1 {
		return ReportProjectionDispatchExecution{}, fmt.Errorf("report projection dispatch version is unsupported: %d", dispatch.Version)
	}
	if reportProjectionDisplayStatus(dispatch.Status) != "dry-run" || reportProjectionDisplayStatus(dispatch.Mode) != "metadata-only" {
		return ReportProjectionDispatchExecution{}, fmt.Errorf("report projection dispatch must be dry-run metadata-only before execution")
	}
	expectedReportPath, err := reportProjectionPath(root, dispatch.RunID)
	if err != nil {
		return ReportProjectionDispatchExecution{}, err
	}
	if strings.TrimSpace(dispatch.ReportPath) != expectedReportPath {
		return ReportProjectionDispatchExecution{}, fmt.Errorf("report projection dispatch report path mismatch: %s", dispatch.ReportPath)
	}
	if err := validateReportProjectionDispatchEventState(ctx, root, dispatch); err != nil {
		return ReportProjectionDispatchExecution{}, err
	}
	now := options.Now.UTC()
	if now.IsZero() {
		now = time.Now().UTC()
	}
	actor := strings.TrimSpace(options.Actor)
	if actor == "" {
		actor = "operator"
	}
	execution := ReportProjectionDispatchExecution{
		Version:      1,
		Status:       "completed",
		Mode:         "execute",
		ExecutedAt:   now.Format(time.RFC3339Nano),
		Actor:        actor,
		Reason:       strings.TrimSpace(options.Reason),
		DispatchPath: dispatch.Path,
		RunID:        dispatch.RunID,
		ReportPath:   expectedReportPath,
	}
	projection, err := ProjectRunReport(ctx, root, dispatch.RunID, ReportProjectionOptions{
		Write:                true,
		ExpectedEventCount:   dispatch.EventCount,
		ExpectedReplayDigest: dispatch.ReplayDigest,
	})
	if err != nil {
		execution.Status = "failed"
		execution.Error = err.Error()
		execution.EventCount = dispatch.EventCount
		if refreshErr := refreshReportProjectionDispatchExecutionState(ctx, root, &execution); refreshErr != nil {
			execution.Error = strings.TrimSpace(execution.Error + "; refresh execution state: " + refreshErr.Error())
		}
		if writeErr := writeReportProjectionDispatchExecution(root, &execution); writeErr != nil {
			return execution, writeErr
		}
		return execution, err
	}
	execution.ReportPath = projection.ReportPath
	execution.EventCount = projection.EventCount
	execution.Written = projection.Written
	if err := writeReportProjectionDispatchExecution(root, &execution); err != nil {
		return execution, err
	}
	return execution, nil
}

func validateReportProjectionDispatchEventState(ctx context.Context, root string, dispatch ReportProjectionDispatch) error {
	store, err := eventlog.OpenSQLite(filepath.Join(root, "replay", "events.db"))
	if err != nil {
		return err
	}
	defer store.Close()
	events, err := store.Replay(ctx, dispatch.RunID)
	if err != nil {
		return err
	}
	if len(events) == 0 {
		return fmt.Errorf("run %s has no replay events", dispatch.RunID)
	}
	if existing := replayReportPath(events); strings.TrimSpace(existing) != "" {
		return fmt.Errorf("run %s already has a report projection: %s", dispatch.RunID, existing)
	}
	if len(events) != dispatch.EventCount {
		return fmt.Errorf("report projection dispatch event count mismatch: dispatch=%d current=%d", dispatch.EventCount, len(events))
	}
	currentDigest := reportProjectionReplayDigest(events)
	if strings.TrimSpace(dispatch.ReplayDigest) == "" {
		return fmt.Errorf("report projection dispatch replay digest is required")
	}
	if currentDigest != dispatch.ReplayDigest {
		return fmt.Errorf("report projection dispatch replay digest mismatch: dispatch=%s current=%s", dispatch.ReplayDigest, currentDigest)
	}
	return nil
}

func refreshReportProjectionDispatchExecutionState(ctx context.Context, root string, execution *ReportProjectionDispatchExecution) error {
	store, err := eventlog.OpenSQLite(filepath.Join(root, "replay", "events.db"))
	if err != nil {
		return err
	}
	defer store.Close()
	events, err := store.Replay(ctx, execution.RunID)
	if err != nil {
		return err
	}
	execution.EventCount = len(events)
	if existing := replayReportPath(events); strings.TrimSpace(existing) != "" {
		execution.ReportPath = existing
	}
	if strings.TrimSpace(execution.ReportPath) != "" {
		if info, err := os.Stat(execution.ReportPath); err == nil && !info.IsDir() {
			execution.Written = true
		} else {
			execution.Written = false
		}
	}
	return nil
}

func ListReportProjectionDispatches(root string) ([]ReportProjectionDispatch, error) {
	if err := validateReportProjectionWorkspace(root); err != nil {
		return nil, err
	}
	if err := rejectUnsafeReportProjectionDispatchDir(reportProjectionDispatchDir(root)); err != nil {
		return nil, err
	}
	paths, err := filepath.Glob(filepath.Join(reportProjectionDispatchDir(root), "*.json"))
	if err != nil {
		return nil, fmt.Errorf("glob report projection dispatches: %w", err)
	}
	sort.Strings(paths)
	dispatches := make([]ReportProjectionDispatch, 0, len(paths))
	for _, path := range paths {
		dispatch, err := readReportProjectionDispatchFile(path)
		if err != nil {
			return nil, err
		}
		dispatches = append(dispatches, dispatch)
	}
	return dispatches, nil
}

func ListReportProjectionDispatchExecutions(root string) ([]ReportProjectionDispatchExecution, error) {
	if err := validateReportProjectionWorkspace(root); err != nil {
		return nil, err
	}
	if err := rejectUnsafeReportProjectionDispatchExecutionDir(reportProjectionDispatchExecutionDir(root)); err != nil {
		return nil, err
	}
	paths, err := filepath.Glob(filepath.Join(reportProjectionDispatchExecutionDir(root), "*.json"))
	if err != nil {
		return nil, fmt.Errorf("glob report projection dispatch executions: %w", err)
	}
	sort.Strings(paths)
	executions := make([]ReportProjectionDispatchExecution, 0, len(paths))
	for _, path := range paths {
		execution, err := readReportProjectionDispatchExecutionFile(path)
		if err != nil {
			return nil, err
		}
		executions = append(executions, execution)
	}
	return executions, nil
}

func ReadReportProjectionDispatch(root, dispatchPath string) (ReportProjectionDispatch, error) {
	if err := validateReportProjectionWorkspace(root); err != nil {
		return ReportProjectionDispatch{}, err
	}
	path, err := resolveReportProjectionDispatchPath(root, dispatchPath)
	if err != nil {
		return ReportProjectionDispatch{}, err
	}
	return readReportProjectionDispatchFile(path)
}

func ReadReportProjectionDispatchExecution(root, executionPath string) (ReportProjectionDispatchExecution, error) {
	if err := validateReportProjectionWorkspace(root); err != nil {
		return ReportProjectionDispatchExecution{}, err
	}
	path, err := resolveReportProjectionDispatchExecutionPath(root, executionPath)
	if err != nil {
		return ReportProjectionDispatchExecution{}, err
	}
	return readReportProjectionDispatchExecutionFile(path)
}

func AnnotateReportProjectionDispatchExecution(root, executionPath string, options ReportProjectionDispatchExecutionAnnotationOptions) (ReportProjectionDispatchExecution, error) {
	annotation, err := normalizeReportProjectionDispatchExecutionAnnotation(options)
	if err != nil {
		return ReportProjectionDispatchExecution{}, err
	}
	if err := validateReportProjectionWorkspace(root); err != nil {
		return ReportProjectionDispatchExecution{}, err
	}
	path, err := resolveReportProjectionDispatchExecutionPath(root, executionPath)
	if err != nil {
		return ReportProjectionDispatchExecution{}, err
	}
	execution, err := readReportProjectionDispatchExecutionFile(path)
	if err != nil {
		return ReportProjectionDispatchExecution{}, err
	}
	execution.Annotation = &annotation
	if err := rejectUnsafeReportProjectionDispatchExecutionFile(path); err != nil {
		return ReportProjectionDispatchExecution{}, err
	}
	data, err := json.MarshalIndent(execution, "", "  ")
	if err != nil {
		return ReportProjectionDispatchExecution{}, fmt.Errorf("encode report projection dispatch execution annotation: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return ReportProjectionDispatchExecution{}, fmt.Errorf("write report projection dispatch execution annotation: %w", err)
	}
	execution.Path = path
	return execution, nil
}

func SelectReportProjectionDispatchExecution(root, executionPath string, options ReportProjectionDispatchExecutionSelectionOptions) (ReportProjectionDispatchExecutionSelection, error) {
	execution, err := ReadReportProjectionDispatchExecution(root, executionPath)
	if err != nil {
		return ReportProjectionDispatchExecutionSelection{}, err
	}
	if err := validateSelectableReportProjectionDispatchExecution(root, execution); err != nil {
		return ReportProjectionDispatchExecutionSelection{}, err
	}
	executions, err := ListReportProjectionDispatchExecutions(root)
	if err != nil {
		return ReportProjectionDispatchExecutionSelection{}, err
	}
	selectedBy := strings.TrimSpace(options.SelectedBy)
	if selectedBy == "" {
		selectedBy = "operator"
	}
	selectedAt := options.SelectedAt
	if selectedAt.IsZero() {
		selectedAt = time.Now().UTC()
	}
	superseded := make([]string, 0, len(executions))
	for _, candidate := range executions {
		if candidate.Path == "" || candidate.Path == execution.Path {
			continue
		}
		if candidate.Status != "completed" || !candidate.Written {
			continue
		}
		superseded = append(superseded, candidate.Path)
	}
	sort.Strings(superseded)
	selection := ReportProjectionDispatchExecutionSelection{
		Path:            reportProjectionDispatchExecutionSelectionPath(root),
		Status:          "current",
		Mode:            "metadata-only",
		ExecutionPath:   execution.Path,
		DispatchPath:    execution.DispatchPath,
		RunID:           execution.RunID,
		ReportPath:      execution.ReportPath,
		EventCount:      execution.EventCount,
		Written:         execution.Written,
		SupersededPaths: superseded,
		SelectedBy:      selectedBy,
		SelectedAt:      selectedAt.UTC().Format(time.RFC3339),
		Reason:          strings.TrimSpace(options.Reason),
	}
	if err := writeReportProjectionDispatchExecutionSelection(root, selection); err != nil {
		return ReportProjectionDispatchExecutionSelection{}, err
	}
	return selection, nil
}

func ClearReportProjectionDispatchExecutionSelection(root string, options ReportProjectionDispatchExecutionSelectionClearOptions) (ReportProjectionDispatchExecutionSelection, error) {
	selection, found, err := ReadReportProjectionDispatchExecutionSelection(root)
	if err != nil {
		return ReportProjectionDispatchExecutionSelection{}, err
	}
	if !found {
		selection = ReportProjectionDispatchExecutionSelection{
			Path:   reportProjectionDispatchExecutionSelectionPath(root),
			Status: "cleared",
			Mode:   "metadata-only",
		}
	}
	clearedBy := strings.TrimSpace(options.ClearedBy)
	if clearedBy == "" {
		clearedBy = "operator"
	}
	clearedAt := options.ClearedAt
	if clearedAt.IsZero() {
		clearedAt = time.Now().UTC()
	}
	selection.Path = reportProjectionDispatchExecutionSelectionPath(root)
	selection.Status = "cleared"
	selection.Mode = "metadata-only"
	selection.ClearedBy = clearedBy
	selection.ClearedAt = clearedAt.UTC().Format(time.RFC3339)
	selection.Reason = strings.TrimSpace(options.Reason)
	if err := writeReportProjectionDispatchExecutionSelection(root, selection); err != nil {
		return ReportProjectionDispatchExecutionSelection{}, err
	}
	return selection, nil
}

func ReadReportProjectionDispatchExecutionSelection(root string) (ReportProjectionDispatchExecutionSelection, bool, error) {
	if err := validateReportProjectionWorkspace(root); err != nil {
		return ReportProjectionDispatchExecutionSelection{}, false, err
	}
	path := reportProjectionDispatchExecutionSelectionPath(root)
	if err := rejectUnsafeReportProjectionSelectionFile(path); err != nil {
		return ReportProjectionDispatchExecutionSelection{}, false, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return ReportProjectionDispatchExecutionSelection{}, false, nil
		}
		return ReportProjectionDispatchExecutionSelection{}, false, fmt.Errorf("read report projection selection %s: %w", path, err)
	}
	var selection ReportProjectionDispatchExecutionSelection
	if err := json.Unmarshal(data, &selection); err != nil {
		return ReportProjectionDispatchExecutionSelection{}, false, fmt.Errorf("decode report projection selection %s: %w", path, err)
	}
	selection.Path = path
	return selection, true, nil
}

func writeReportProjectionDispatch(root string, dispatch *ReportProjectionDispatch) error {
	reportsDir := filepath.Join(root, "reports")
	if err := rejectUnsafeReportProjectionReportsDir(reportsDir); err != nil {
		return err
	}
	if err := os.MkdirAll(reportsDir, 0o755); err != nil {
		return fmt.Errorf("create reports directory: %w", err)
	}
	if err := requireReportProjectionDirectory(reportsDir, "reports directory"); err != nil {
		return err
	}
	dir := reportProjectionDispatchDir(root)
	if err := rejectUnsafeReportProjectionDispatchDir(dir); err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create report projection dispatch directory: %w", err)
	}
	if err := requireReportProjectionDirectory(dir, "report projection dispatch directory"); err != nil {
		return err
	}
	dispatch.Path = filepath.Join(dir, safeReportProjectionDispatchName(*dispatch)+".json")
	if err := rejectUnsafeReportProjectionDispatchFile(dispatch.Path); err != nil {
		return err
	}
	data, err := json.MarshalIndent(dispatch, "", "  ")
	if err != nil {
		return fmt.Errorf("encode report projection dispatch: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(dispatch.Path, data, 0o644); err != nil {
		return fmt.Errorf("write report projection dispatch: %w", err)
	}
	return nil
}

func writeReportProjectionDispatchExecution(root string, execution *ReportProjectionDispatchExecution) error {
	reportsDir := filepath.Join(root, "reports")
	if err := rejectUnsafeReportProjectionReportsDir(reportsDir); err != nil {
		return err
	}
	if err := os.MkdirAll(reportsDir, 0o755); err != nil {
		return fmt.Errorf("create reports directory: %w", err)
	}
	if err := requireReportProjectionDirectory(reportsDir, "reports directory"); err != nil {
		return err
	}
	dir := reportProjectionDispatchExecutionDir(root)
	if err := rejectUnsafeReportProjectionDispatchExecutionDir(dir); err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create report projection dispatch execution directory: %w", err)
	}
	if err := requireReportProjectionDirectory(dir, "report projection dispatch execution directory"); err != nil {
		return err
	}
	execution.Path = filepath.Join(dir, safeReportProjectionDispatchExecutionName(*execution)+".json")
	if err := rejectUnsafeReportProjectionDispatchExecutionFile(execution.Path); err != nil {
		return err
	}
	data, err := json.MarshalIndent(execution, "", "  ")
	if err != nil {
		return fmt.Errorf("encode report projection dispatch execution: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(execution.Path, data, 0o644); err != nil {
		return fmt.Errorf("write report projection dispatch execution: %w", err)
	}
	return nil
}

func writeReportProjectionDispatchExecutionSelection(root string, selection ReportProjectionDispatchExecutionSelection) error {
	reportsDir := filepath.Join(root, "reports")
	if err := rejectUnsafeReportProjectionReportsDir(reportsDir); err != nil {
		return err
	}
	if err := os.MkdirAll(reportsDir, 0o755); err != nil {
		return fmt.Errorf("create reports directory: %w", err)
	}
	if err := requireReportProjectionDirectory(reportsDir, "reports directory"); err != nil {
		return err
	}
	selection.Path = reportProjectionDispatchExecutionSelectionPath(root)
	if err := rejectUnsafeReportProjectionSelectionFile(selection.Path); err != nil {
		return err
	}
	data, err := json.MarshalIndent(selection, "", "  ")
	if err != nil {
		return fmt.Errorf("encode report projection selection: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(selection.Path, data, 0o644); err != nil {
		return fmt.Errorf("write report projection selection: %w", err)
	}
	return nil
}

func validateSelectableReportProjectionDispatchExecution(root string, execution ReportProjectionDispatchExecution) error {
	if execution.Version != 1 {
		return fmt.Errorf("report projection dispatch execution version is unsupported: %d", execution.Version)
	}
	if reportProjectionDisplayStatus(execution.Status) != "completed" || !execution.Written {
		return fmt.Errorf("report projection dispatch execution must be completed and written before selection")
	}
	if reportProjectionDisplayStatus(execution.Mode) != "execute" {
		return fmt.Errorf("report projection dispatch execution must be execute mode before selection")
	}
	if execution.Annotation == nil || reportProjectionDisplayStatus(execution.Annotation.Decision) != "accepted" {
		return fmt.Errorf("report projection dispatch execution selection requires accepted annotation")
	}
	expectedReportPath, err := reportProjectionPath(root, execution.RunID)
	if err != nil {
		return err
	}
	if strings.TrimSpace(execution.ReportPath) != expectedReportPath {
		return fmt.Errorf("report projection dispatch execution report path mismatch: %s", execution.ReportPath)
	}
	if err := rejectUnsafeReportProjectionReportFile(execution.ReportPath); err != nil {
		return err
	}
	info, err := os.Stat(execution.ReportPath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("report projection dispatch execution report is missing: %s", execution.ReportPath)
		}
		return fmt.Errorf("stat report projection dispatch execution report: %w", err)
	}
	if info.IsDir() {
		return fmt.Errorf("report projection dispatch execution report is a directory: %s", execution.ReportPath)
	}
	return nil
}

func readReportProjectionDispatchFile(path string) (ReportProjectionDispatch, error) {
	if err := rejectUnsafeReportProjectionDispatchFile(path); err != nil {
		return ReportProjectionDispatch{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ReportProjectionDispatch{}, fmt.Errorf("read report projection dispatch %s: %w", path, err)
	}
	var dispatch ReportProjectionDispatch
	if err := json.Unmarshal(data, &dispatch); err != nil {
		return ReportProjectionDispatch{}, fmt.Errorf("decode report projection dispatch %s: %w", path, err)
	}
	dispatch.Path = path
	return dispatch, nil
}

func readReportProjectionDispatchExecutionFile(path string) (ReportProjectionDispatchExecution, error) {
	if err := rejectUnsafeReportProjectionDispatchExecutionFile(path); err != nil {
		return ReportProjectionDispatchExecution{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ReportProjectionDispatchExecution{}, fmt.Errorf("read report projection dispatch execution %s: %w", path, err)
	}
	var execution ReportProjectionDispatchExecution
	if err := json.Unmarshal(data, &execution); err != nil {
		return ReportProjectionDispatchExecution{}, fmt.Errorf("decode report projection dispatch execution %s: %w", path, err)
	}
	execution.Path = path
	return execution, nil
}

func resolveReportProjectionDispatchPath(root, dispatchPath string) (string, error) {
	trimmed := strings.TrimSpace(dispatchPath)
	if trimmed == "" {
		return "", fmt.Errorf("report projection dispatch path is required")
	}
	dir, err := filepath.Abs(reportProjectionDispatchDir(root))
	if err != nil {
		return "", fmt.Errorf("resolve report projection dispatch directory: %w", err)
	}
	if err := rejectUnsafeReportProjectionDispatchDir(dir); err != nil {
		return "", err
	}
	candidate := trimmed
	if !filepath.IsAbs(candidate) {
		if filepath.Ext(candidate) == "" && !strings.Contains(candidate, string(os.PathSeparator)) {
			candidate += ".json"
		}
		candidate = filepath.Join(dir, candidate)
	}
	candidate, err = filepath.Abs(candidate)
	if err != nil {
		return "", fmt.Errorf("resolve report projection dispatch path: %w", err)
	}
	rel, err := filepath.Rel(dir, candidate)
	if err != nil {
		return "", fmt.Errorf("resolve report projection dispatch relative path: %w", err)
	}
	if rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("report projection dispatch path must stay under %s", dir)
	}
	if strings.Contains(rel, string(os.PathSeparator)) {
		return "", fmt.Errorf("report projection dispatch path must be a direct json file under %s", dir)
	}
	if filepath.Ext(candidate) != ".json" {
		return "", fmt.Errorf("report projection dispatch path must be a json file")
	}
	return candidate, nil
}

func resolveReportProjectionDispatchExecutionPath(root, executionPath string) (string, error) {
	trimmed := strings.TrimSpace(executionPath)
	if trimmed == "" {
		return "", fmt.Errorf("report projection dispatch execution path is required")
	}
	dir, err := filepath.Abs(reportProjectionDispatchExecutionDir(root))
	if err != nil {
		return "", fmt.Errorf("resolve report projection dispatch execution directory: %w", err)
	}
	if err := rejectUnsafeReportProjectionDispatchExecutionDir(dir); err != nil {
		return "", err
	}
	candidate := trimmed
	if !filepath.IsAbs(candidate) {
		if filepath.Ext(candidate) == "" && !strings.Contains(candidate, string(os.PathSeparator)) {
			candidate += ".json"
		}
		candidate = filepath.Join(dir, candidate)
	}
	candidate, err = filepath.Abs(candidate)
	if err != nil {
		return "", fmt.Errorf("resolve report projection dispatch execution path: %w", err)
	}
	rel, err := filepath.Rel(dir, candidate)
	if err != nil {
		return "", fmt.Errorf("resolve report projection dispatch execution relative path: %w", err)
	}
	if rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("report projection dispatch execution path must stay under %s", dir)
	}
	if strings.Contains(rel, string(os.PathSeparator)) {
		return "", fmt.Errorf("report projection dispatch execution path must be a direct json file under %s", dir)
	}
	if filepath.Ext(candidate) != ".json" {
		return "", fmt.Errorf("report projection dispatch execution path must be a json file")
	}
	return candidate, nil
}

func reportProjectionDispatchDir(root string) string {
	return filepath.Join(root, "reports", "report-projection-dispatches")
}

func reportProjectionDispatchExecutionDir(root string) string {
	return filepath.Join(root, "reports", "report-projection-dispatch-executions")
}

func reportProjectionDispatchExecutionSelectionPath(root string) string {
	return filepath.Join(root, "reports", "report-projection-selection.json")
}

func safeReportProjectionDispatchName(dispatch ReportProjectionDispatch) string {
	parts := []string{
		"report-projection-dispatch",
		dispatch.CreatedAt,
		dispatch.RunID,
		dispatch.Status,
		dispatch.Mode,
		dispatch.Actor,
		fmt.Sprintf("events=%d", dispatch.EventCount),
	}
	name := safeInstallRequestName(strings.Join(parts, "|"))
	if index := strings.LastIndex(name, "-"); index >= 0 && len(name[index+1:]) == 12 {
		return "report-projection-dispatch-" + name[index+1:]
	}
	return name
}

func safeReportProjectionDispatchExecutionName(execution ReportProjectionDispatchExecution) string {
	parts := []string{
		"report-projection-dispatch-execution",
		execution.ExecutedAt,
		execution.RunID,
		execution.Status,
		execution.Mode,
		execution.Actor,
		fmt.Sprintf("events=%d", execution.EventCount),
	}
	name := safeInstallRequestName(strings.Join(parts, "|"))
	if index := strings.LastIndex(name, "-"); index >= 0 && len(name[index+1:]) == 12 {
		return "report-projection-dispatch-execution-" + name[index+1:]
	}
	return name
}

func reportProjectionDisplayStatus(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "unknown"
	}
	return value
}

func normalizeReportProjectionDispatchExecutionAnnotation(options ReportProjectionDispatchExecutionAnnotationOptions) (ReportProjectionDispatchExecutionAnnotation, error) {
	decision := strings.ToLower(strings.TrimSpace(options.Decision))
	switch decision {
	case "accepted", "deferred", "rejected":
	case "":
		return ReportProjectionDispatchExecutionAnnotation{}, fmt.Errorf("report projection dispatch execution annotation decision is required")
	default:
		return ReportProjectionDispatchExecutionAnnotation{}, fmt.Errorf("unsupported report projection dispatch execution annotation decision %q", options.Decision)
	}
	reviewer := strings.TrimSpace(options.Reviewer)
	if reviewer == "" {
		reviewer = "operator"
	}
	reviewedAt := options.ReviewedAt
	if reviewedAt.IsZero() {
		reviewedAt = time.Now().UTC()
	}
	return ReportProjectionDispatchExecutionAnnotation{
		Decision:   decision,
		Reason:     strings.TrimSpace(options.Reason),
		Reviewer:   reviewer,
		ReviewedAt: reviewedAt.UTC().Format(time.RFC3339),
	}, nil
}

func rejectUnsafeReportProjectionDispatchDir(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("inspect report projection dispatch directory: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("report projection dispatch directory must not be a symlink: %s", path)
	}
	if !info.IsDir() {
		return fmt.Errorf("report projection dispatch path is not a directory: %s", path)
	}
	return nil
}

func rejectUnsafeReportProjectionDispatchExecutionDir(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("inspect report projection dispatch execution directory: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("report projection dispatch execution directory must not be a symlink: %s", path)
	}
	if !info.IsDir() {
		return fmt.Errorf("report projection dispatch execution path is not a directory: %s", path)
	}
	return nil
}

func rejectUnsafeReportProjectionDispatchFile(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("inspect report projection dispatch file: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("report projection dispatch file must not be a symlink: %s", path)
	}
	if info.IsDir() {
		return fmt.Errorf("write report projection dispatch: path is a directory: %s", path)
	}
	return nil
}

func rejectUnsafeReportProjectionDispatchExecutionFile(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("inspect report projection dispatch execution file: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("report projection dispatch execution file must not be a symlink: %s", path)
	}
	if info.IsDir() {
		return fmt.Errorf("write report projection dispatch execution: path is a directory: %s", path)
	}
	return nil
}

func rejectUnsafeReportProjectionSelectionFile(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("inspect report projection selection file: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("report projection selection file must not be a symlink: %s", path)
	}
	if info.IsDir() {
		return fmt.Errorf("write report projection selection: path is a directory: %s", path)
	}
	return nil
}
