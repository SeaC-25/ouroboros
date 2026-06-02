package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ouroboros/ouroboros/internal/eventlog"
	"github.com/ouroboros/ouroboros/internal/report"
)

type ReportProjectionOptions struct {
	Write                bool
	ExpectedEventCount   int
	ExpectedReplayDigest string
}

type ReportProjectionResult struct {
	RunID        string
	Status       string
	Mode         string
	ReportPath   string
	EventCount   int
	ReplayDigest string
	Written      bool
}

func ProjectRunReport(ctx context.Context, root string, runID string, options ReportProjectionOptions) (ReportProjectionResult, error) {
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return ReportProjectionResult{}, fmt.Errorf("run id is required")
	}
	if err := validateReportProjectionWorkspace(root); err != nil {
		return ReportProjectionResult{}, err
	}

	store, err := eventlog.OpenSQLite(filepath.Join(root, "replay", "events.db"))
	if err != nil {
		return ReportProjectionResult{}, err
	}
	defer store.Close()

	events, err := store.Replay(ctx, runID)
	if err != nil {
		return ReportProjectionResult{}, err
	}
	if len(events) == 0 {
		return ReportProjectionResult{}, fmt.Errorf("run %s has no replay events", runID)
	}
	if existing := replayReportPath(events); strings.TrimSpace(existing) != "" {
		return ReportProjectionResult{}, fmt.Errorf("run %s already has a report projection: %s", runID, existing)
	}
	replayDigest := reportProjectionReplayDigest(events)
	if options.ExpectedEventCount > 0 && len(events) != options.ExpectedEventCount {
		return ReportProjectionResult{}, fmt.Errorf("report projection replay event count mismatch: expected=%d current=%d", options.ExpectedEventCount, len(events))
	}
	if expectedDigest := strings.TrimSpace(options.ExpectedReplayDigest); expectedDigest != "" && replayDigest != expectedDigest {
		return ReportProjectionResult{}, fmt.Errorf("report projection replay digest mismatch: expected=%s current=%s", expectedDigest, replayDigest)
	}
	snapshot, err := eventlog.BuildReplaySnapshot(events)
	if err != nil {
		return ReportProjectionResult{}, err
	}

	reportPath, err := reportProjectionPath(root, runID)
	if err != nil {
		return ReportProjectionResult{}, err
	}
	result := ReportProjectionResult{
		RunID:        runID,
		Status:       "dry-run",
		Mode:         "metadata-only",
		ReportPath:   reportPath,
		EventCount:   len(events),
		ReplayDigest: replayDigest,
	}
	if !options.Write {
		return result, nil
	}

	if _, err := store.Append(ctx, stageEvent(runID, "report-projection", "Project evidence graph into Markdown report", "started")); err != nil {
		return ReportProjectionResult{}, err
	}
	markdown := report.RenderMarkdown(report.Project(snapshot.Evidence))
	if _, err := writeReportProjectionMarkdown(root, runID, markdown); err != nil {
		return ReportProjectionResult{}, err
	}
	if _, err := store.Append(ctx, eventlog.Event{
		RunID: runID,
		Type:  eventlog.EventReplay,
		Payload: mustJSON(map[string]any{
			"status":      "report_ready",
			"report_path": reportPath,
		}),
	}); err != nil {
		return ReportProjectionResult{}, err
	}
	if _, err := store.Append(ctx, stageEvent(runID, "report-projection", "Project evidence graph into Markdown report", "completed")); err != nil {
		return ReportProjectionResult{}, err
	}
	events, err = store.Replay(ctx, runID)
	if err != nil {
		return ReportProjectionResult{}, err
	}
	result.Status = "completed"
	result.Mode = "write"
	result.EventCount = len(events)
	result.Written = true
	return result, nil
}

func reportProjectionReplayDigest(events []eventlog.Event) string {
	hash := sha256.New()
	for _, event := range events {
		_, _ = fmt.Fprintf(
			hash,
			"%d\n%s\n%s\n%s\n",
			event.ID,
			event.RunID,
			event.Type,
			event.CreatedAt.UTC().Format(time.RFC3339Nano),
		)
		_, _ = hash.Write(event.Payload)
		_, _ = hash.Write([]byte{'\n'})
	}
	return hex.EncodeToString(hash.Sum(nil))
}

func validateReportProjectionWorkspace(root string) error {
	if strings.TrimSpace(root) == "" {
		return fmt.Errorf("workspace path is required")
	}
	info, err := os.Stat(root)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("workspace root missing: %s", root)
		}
		return fmt.Errorf("stat workspace root: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("workspace root is not a directory: %s", root)
	}
	replayDir := filepath.Join(root, "replay")
	if err := requireReportProjectionDirectory(replayDir, "replay directory"); err != nil {
		return err
	}
	eventLogPath := filepath.Join(replayDir, "events.db")
	info, err = os.Lstat(eventLogPath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("replay event log missing: %s", eventLogPath)
		}
		return fmt.Errorf("inspect replay event log: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("replay event log must not be a symlink: %s", eventLogPath)
	}
	if info.IsDir() {
		return fmt.Errorf("replay event log is a directory: %s", eventLogPath)
	}
	return nil
}

func reportProjectionPath(root string, runID string) (string, error) {
	runID = strings.TrimSpace(runID)
	if !safeReportProjectionRunID(runID) {
		return "", fmt.Errorf("report projection run id must be a safe basename")
	}
	reportsDir := filepath.Join(root, "reports")
	if err := rejectUnsafeReportProjectionReportsDir(reportsDir); err != nil {
		return "", err
	}
	reportPath := filepath.Join(reportsDir, runID+".md")
	absReportsDir, err := filepath.Abs(reportsDir)
	if err != nil {
		return "", fmt.Errorf("resolve reports directory: %w", err)
	}
	absReportPath, err := filepath.Abs(reportPath)
	if err != nil {
		return "", fmt.Errorf("resolve report path: %w", err)
	}
	rel, err := filepath.Rel(absReportsDir, absReportPath)
	if err != nil {
		return "", fmt.Errorf("resolve report relative path: %w", err)
	}
	if rel == "." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || rel == ".." || filepath.IsAbs(rel) {
		return "", fmt.Errorf("report projection path must stay under %s", absReportsDir)
	}
	return absReportPath, nil
}

func writeReportProjectionMarkdown(root string, runID string, markdown string) (string, error) {
	reportPath, err := reportProjectionPath(root, runID)
	if err != nil {
		return "", err
	}
	reportsDir := filepath.Dir(reportPath)
	if err := os.MkdirAll(reportsDir, 0o755); err != nil {
		return "", fmt.Errorf("create reports directory: %w", err)
	}
	if err := requireReportProjectionDirectory(reportsDir, "reports directory"); err != nil {
		return "", err
	}
	if err := rejectUnsafeReportProjectionReportFile(reportPath); err != nil {
		return "", err
	}
	if err := os.WriteFile(reportPath, []byte(markdown), 0o644); err != nil {
		return "", fmt.Errorf("write report: %w", err)
	}
	return reportPath, nil
}

func requireReportProjectionDirectory(path string, label string) error {
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("%s missing: %s", label, path)
		}
		return fmt.Errorf("inspect %s: %w", label, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("%s must not be a symlink: %s", label, path)
	}
	if !info.IsDir() {
		return fmt.Errorf("%s is not a directory: %s", label, path)
	}
	return nil
}

func rejectUnsafeReportProjectionReportsDir(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("inspect reports directory: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("reports directory must not be a symlink: %s", path)
	}
	if !info.IsDir() {
		return fmt.Errorf("reports path is not a directory: %s", path)
	}
	return nil
}

func rejectUnsafeReportProjectionReportFile(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("inspect report file: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("report file must not be a symlink: %s", path)
	}
	if info.IsDir() {
		return fmt.Errorf("write report: report path is a directory: %s", path)
	}
	return nil
}

func safeReportProjectionRunID(runID string) bool {
	runID = strings.TrimSpace(runID)
	if runID == "" || runID == "." || runID == ".." {
		return false
	}
	return filepath.Base(runID) == runID && !strings.Contains(runID, "/") && !strings.Contains(runID, "\\")
}
