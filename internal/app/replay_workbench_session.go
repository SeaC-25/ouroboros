package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type ReplayWorkbenchSessionArtifact struct {
	Path          string                            `json:"path,omitempty"`
	SchemaVersion int                               `json:"schema_version"`
	Status        string                            `json:"status"`
	Mode          string                            `json:"mode"`
	PrimaryRunID  string                            `json:"primary_run_id"`
	SelectedRunID string                            `json:"selected_run_id"`
	RunIDs        []string                          `json:"run_ids"`
	Filters       map[string]string                 `json:"filters"`
	Endpoints     map[string]string                 `json:"endpoints"`
	CreatedBy     string                            `json:"created_by"`
	CreatedAt     string                            `json:"created_at"`
	Reason        string                            `json:"reason,omitempty"`
	Annotation    *ReplayWorkbenchSessionAnnotation `json:"annotation,omitempty"`
}

type ReplayWorkbenchSessionAnnotation struct {
	Decision   string `json:"decision"`
	Reason     string `json:"reason,omitempty"`
	Reviewer   string `json:"reviewer,omitempty"`
	ReviewedAt string `json:"reviewed_at,omitempty"`
}

type ReplayWorkbenchSessionAnnotationOptions struct {
	Decision   string
	Reason     string
	Reviewer   string
	ReviewedAt time.Time
}

type ReplayWorkbenchSessionSelection struct {
	Path            string   `json:"path,omitempty"`
	Status          string   `json:"status"`
	Mode            string   `json:"mode"`
	SessionPath     string   `json:"session_path,omitempty"`
	PrimaryRunID    string   `json:"primary_run_id,omitempty"`
	SelectedRunID   string   `json:"selected_run_id,omitempty"`
	RunIDs          int      `json:"run_ids,omitempty"`
	Filters         int      `json:"filters,omitempty"`
	Endpoints       int      `json:"endpoints,omitempty"`
	SupersededPaths []string `json:"superseded_paths,omitempty"`
	SelectedBy      string   `json:"selected_by,omitempty"`
	SelectedAt      string   `json:"selected_at,omitempty"`
	ClearedBy       string   `json:"cleared_by,omitempty"`
	ClearedAt       string   `json:"cleared_at,omitempty"`
	Reason          string   `json:"reason,omitempty"`
}

type ReplayWorkbenchSessionSelectionOptions struct {
	SelectedBy string
	SelectedAt time.Time
	Reason     string
}

type ReplayWorkbenchSessionSelectionClearOptions struct {
	ClearedBy string
	ClearedAt time.Time
	Reason    string
}

type ReplayWorkbenchSessionOptions struct {
	PrimaryRunID  string
	CompareRunIDs []string
	SelectedRunID string
	Filters       map[string]string
	CreatedBy     string
	CreatedAt     time.Time
	Reason        string
}

func PreviewReplayWorkbenchSession(ctx context.Context, root string, options ReplayWorkbenchSessionOptions) (ReplayWorkbenchSessionArtifact, error) {
	session, err := buildReplayWorkbenchSession(ctx, root, options)
	if err != nil {
		return ReplayWorkbenchSessionArtifact{}, err
	}
	session.Status = "dry-run"
	return session, nil
}

func CreateReplayWorkbenchSession(ctx context.Context, root string, options ReplayWorkbenchSessionOptions) (ReplayWorkbenchSessionArtifact, error) {
	session, err := buildReplayWorkbenchSession(ctx, root, options)
	if err != nil {
		return ReplayWorkbenchSessionArtifact{}, err
	}
	session.Status = "saved"
	dir := replayWorkbenchSessionDir(root)
	if err := ensureReplayWorkbenchDirectory(dir, "replay workbench session directory"); err != nil {
		return ReplayWorkbenchSessionArtifact{}, err
	}
	session.Path = filepath.Join(dir, safeReplayWorkbenchSessionName(session)+".json")
	if err := rejectUnsafeReplayWorkbenchFile(session.Path, "replay workbench session file"); err != nil {
		return ReplayWorkbenchSessionArtifact{}, err
	}
	data, err := json.MarshalIndent(session, "", "  ")
	if err != nil {
		return ReplayWorkbenchSessionArtifact{}, fmt.Errorf("encode replay workbench session: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(session.Path, data, 0o644); err != nil {
		return ReplayWorkbenchSessionArtifact{}, fmt.Errorf("write replay workbench session: %w", err)
	}
	return session, nil
}

func ListReplayWorkbenchSessions(root string) ([]ReplayWorkbenchSessionArtifact, error) {
	doctor, err := DoctorWorkspace(root)
	if err != nil {
		return nil, err
	}
	if !doctor.Ready {
		return nil, fmt.Errorf("workspace is not ready: %v", doctor.Errors)
	}
	dir := replayWorkbenchSessionDir(root)
	if err := rejectUnsafeReplayWorkbenchDirectory(dir, "replay workbench session directory"); err != nil {
		return nil, err
	}
	paths, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		return nil, fmt.Errorf("glob replay workbench sessions: %w", err)
	}
	sort.Strings(paths)
	sessions := make([]ReplayWorkbenchSessionArtifact, 0, len(paths))
	for _, path := range paths {
		session, err := readReplayWorkbenchSessionFile(path)
		if err != nil {
			return nil, err
		}
		sessions = append(sessions, session)
	}
	return sessions, nil
}

func ReadReplayWorkbenchSession(root, sessionPath string) (ReplayWorkbenchSessionArtifact, error) {
	doctor, err := DoctorWorkspace(root)
	if err != nil {
		return ReplayWorkbenchSessionArtifact{}, err
	}
	if !doctor.Ready {
		return ReplayWorkbenchSessionArtifact{}, fmt.Errorf("workspace is not ready: %v", doctor.Errors)
	}
	path, err := resolveReplayWorkbenchSessionPath(root, sessionPath)
	if err != nil {
		return ReplayWorkbenchSessionArtifact{}, err
	}
	return readReplayWorkbenchSessionFile(path)
}

func AnnotateReplayWorkbenchSession(root, sessionPath string, options ReplayWorkbenchSessionAnnotationOptions) (ReplayWorkbenchSessionArtifact, error) {
	annotation, err := normalizeReplayWorkbenchSessionAnnotation(options)
	if err != nil {
		return ReplayWorkbenchSessionArtifact{}, err
	}
	doctor, err := DoctorWorkspace(root)
	if err != nil {
		return ReplayWorkbenchSessionArtifact{}, err
	}
	if !doctor.Ready {
		return ReplayWorkbenchSessionArtifact{}, fmt.Errorf("workspace is not ready: %v", doctor.Errors)
	}
	path, err := resolveReplayWorkbenchSessionPath(root, sessionPath)
	if err != nil {
		return ReplayWorkbenchSessionArtifact{}, err
	}
	session, err := readReplayWorkbenchSessionFile(path)
	if err != nil {
		return ReplayWorkbenchSessionArtifact{}, err
	}
	session.Annotation = &annotation
	data, err := json.MarshalIndent(session, "", "  ")
	if err != nil {
		return ReplayWorkbenchSessionArtifact{}, fmt.Errorf("encode replay workbench session annotation: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return ReplayWorkbenchSessionArtifact{}, fmt.Errorf("write replay workbench session annotation: %w", err)
	}
	session.Path = path
	return session, nil
}

func SelectReplayWorkbenchSession(root, sessionPath string, options ReplayWorkbenchSessionSelectionOptions) (ReplayWorkbenchSessionSelection, error) {
	session, err := ReadReplayWorkbenchSession(root, sessionPath)
	if err != nil {
		return ReplayWorkbenchSessionSelection{}, err
	}
	sessions, err := ListReplayWorkbenchSessions(root)
	if err != nil {
		return ReplayWorkbenchSessionSelection{}, err
	}
	selectedBy := strings.TrimSpace(options.SelectedBy)
	if selectedBy == "" {
		selectedBy = "operator"
	}
	selectedAt := options.SelectedAt
	if selectedAt.IsZero() {
		selectedAt = time.Now().UTC()
	}
	superseded := make([]string, 0, len(sessions))
	for _, candidate := range sessions {
		if candidate.Path == "" || candidate.Path == session.Path {
			continue
		}
		superseded = append(superseded, candidate.Path)
	}
	sort.Strings(superseded)
	selection := ReplayWorkbenchSessionSelection{
		Path:            replayWorkbenchSessionSelectionPath(root),
		Status:          "current",
		Mode:            "metadata-only",
		SessionPath:     session.Path,
		PrimaryRunID:    session.PrimaryRunID,
		SelectedRunID:   session.SelectedRunID,
		RunIDs:          len(session.RunIDs),
		Filters:         len(session.Filters),
		Endpoints:       len(session.Endpoints),
		SupersededPaths: superseded,
		SelectedBy:      selectedBy,
		SelectedAt:      selectedAt.UTC().Format(time.RFC3339),
		Reason:          strings.TrimSpace(options.Reason),
	}
	if err := writeReplayWorkbenchSessionSelection(selection); err != nil {
		return ReplayWorkbenchSessionSelection{}, err
	}
	return selection, nil
}

func ClearReplayWorkbenchSessionSelection(root string, options ReplayWorkbenchSessionSelectionClearOptions) (ReplayWorkbenchSessionSelection, error) {
	selection, found, err := ReadReplayWorkbenchSessionSelection(root)
	if err != nil {
		return ReplayWorkbenchSessionSelection{}, err
	}
	if !found {
		selection = ReplayWorkbenchSessionSelection{
			Path:   replayWorkbenchSessionSelectionPath(root),
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
	selection.Path = replayWorkbenchSessionSelectionPath(root)
	selection.Status = "cleared"
	selection.Mode = "metadata-only"
	selection.ClearedBy = clearedBy
	selection.ClearedAt = clearedAt.UTC().Format(time.RFC3339)
	selection.Reason = strings.TrimSpace(options.Reason)
	if err := writeReplayWorkbenchSessionSelection(selection); err != nil {
		return ReplayWorkbenchSessionSelection{}, err
	}
	return selection, nil
}

func ReadReplayWorkbenchSessionSelection(root string) (ReplayWorkbenchSessionSelection, bool, error) {
	doctor, err := DoctorWorkspace(root)
	if err != nil {
		return ReplayWorkbenchSessionSelection{}, false, err
	}
	if !doctor.Ready {
		return ReplayWorkbenchSessionSelection{}, false, fmt.Errorf("workspace is not ready: %v", doctor.Errors)
	}
	path := replayWorkbenchSessionSelectionPath(root)
	if err := rejectUnsafeReplayWorkbenchFile(path, "replay workbench session selection file"); err != nil {
		return ReplayWorkbenchSessionSelection{}, false, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return ReplayWorkbenchSessionSelection{}, false, nil
		}
		return ReplayWorkbenchSessionSelection{}, false, fmt.Errorf("read replay workbench session selection %s: %w", path, err)
	}
	var selection ReplayWorkbenchSessionSelection
	if err := json.Unmarshal(data, &selection); err != nil {
		return ReplayWorkbenchSessionSelection{}, false, fmt.Errorf("decode replay workbench session selection %s: %w", path, err)
	}
	selection.Path = path
	return selection, true, nil
}

func writeReplayWorkbenchSessionSelection(selection ReplayWorkbenchSessionSelection) error {
	if err := ensureReplayWorkbenchDirectory(filepath.Dir(selection.Path), "replay workbench directory"); err != nil {
		return err
	}
	if err := rejectUnsafeReplayWorkbenchFile(selection.Path, "replay workbench session selection file"); err != nil {
		return err
	}
	data, err := json.MarshalIndent(selection, "", "  ")
	if err != nil {
		return fmt.Errorf("encode replay workbench session selection: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(selection.Path, data, 0o644); err != nil {
		return fmt.Errorf("write replay workbench session selection: %w", err)
	}
	return nil
}

func replayWorkbenchSessionSelectionIsCurrent(selection ReplayWorkbenchSessionSelection) bool {
	return strings.TrimSpace(selection.Status) == "current"
}

func replayWorkbenchSessionSelectionIsCleared(selection ReplayWorkbenchSessionSelection) bool {
	return strings.TrimSpace(selection.Status) == "cleared"
}

func buildReplayWorkbenchSession(ctx context.Context, root string, options ReplayWorkbenchSessionOptions) (ReplayWorkbenchSessionArtifact, error) {
	primaryRunID := strings.TrimSpace(options.PrimaryRunID)
	if primaryRunID == "" {
		return ReplayWorkbenchSessionArtifact{}, fmt.Errorf("primary replay run id is required")
	}
	runIDs := []string{}
	seen := map[string]bool{}
	appendRunID := func(runID string) {
		runID = strings.TrimSpace(runID)
		if runID == "" || seen[runID] {
			return
		}
		seen[runID] = true
		runIDs = append(runIDs, runID)
	}
	appendRunID(primaryRunID)
	for _, runID := range options.CompareRunIDs {
		appendRunID(runID)
	}
	for _, runID := range runIDs {
		if _, err := ReplayWorkspace(ctx, root, runID); err != nil {
			return ReplayWorkbenchSessionArtifact{}, err
		}
	}
	selectedRunID := strings.TrimSpace(options.SelectedRunID)
	if selectedRunID == "" {
		selectedRunID = primaryRunID
	}
	if !seen[selectedRunID] {
		return ReplayWorkbenchSessionArtifact{}, fmt.Errorf("selected replay run %s is not part of the workbench session", selectedRunID)
	}
	filters, err := normalizeReplayWorkbenchSessionFilters(options.Filters)
	if err != nil {
		return ReplayWorkbenchSessionArtifact{}, err
	}
	createdBy := strings.TrimSpace(options.CreatedBy)
	if createdBy == "" {
		createdBy = "operator"
	}
	createdAt := options.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}
	return ReplayWorkbenchSessionArtifact{
		SchemaVersion: 1,
		Status:        "dry-run",
		Mode:          "metadata-only",
		PrimaryRunID:  primaryRunID,
		SelectedRunID: selectedRunID,
		RunIDs:        runIDs,
		Filters:       filters,
		Endpoints:     replayWorkbenchSessionEndpoints(selectedRunID, filters),
		CreatedBy:     createdBy,
		CreatedAt:     createdAt.UTC().Format(time.RFC3339),
		Reason:        strings.TrimSpace(options.Reason),
	}, nil
}

func normalizeReplayWorkbenchSessionFilters(input map[string]string) (map[string]string, error) {
	filters := map[string]string{}
	for key, value := range input {
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		switch key {
		case "lane":
			if strings.EqualFold(value, "all") {
				continue
			}
			filters[key] = value
		case "q", "node", "edge":
			filters[key] = value
		default:
			return nil, fmt.Errorf("unsupported replay workbench session filter %q", key)
		}
	}
	return filters, nil
}

func replayWorkbenchSessionEndpoints(selectedRunID string, filters map[string]string) map[string]string {
	filterQuery := replayWorkbenchSessionQuery(filters)
	htmlQuery := replayWorkbenchSessionQuery(filters)
	if selectedRunID = strings.TrimSpace(selectedRunID); selectedRunID != "" {
		htmlQuery.Set("run", selectedRunID)
	}
	return map[string]string{
		"html":                   replayWorkbenchSessionPathWithQuery("/", htmlQuery),
		"replay":                 replayWorkbenchSessionPathWithQuery("/api/replay", filterQuery),
		"replays":                replayWorkbenchSessionPathWithQuery("/api/replays", filterQuery),
		"sessions":               "/api/sessions",
		"provider_usage_summary": "/api/provider-usage-summary",
		"graph":                  replayWorkbenchSessionPathWithQuery("/api/graph", filterQuery),
		"graphs":                 replayWorkbenchSessionPathWithQuery("/api/graphs", filterQuery),
		"graph_ndjson":           replayWorkbenchSessionPathWithQuery("/api/graph.ndjson", filterQuery),
		"timeline_ndjson":        replayWorkbenchSessionPathWithQuery("/api/timeline.ndjson", filterQuery),
	}
}

func replayWorkbenchSessionQuery(filters map[string]string) url.Values {
	values := url.Values{}
	for _, key := range []string{"edge", "lane", "node", "q"} {
		if value := strings.TrimSpace(filters[key]); value != "" {
			values.Set(key, value)
		}
	}
	return values
}

func replayWorkbenchSessionPathWithQuery(path string, values url.Values) string {
	if encoded := values.Encode(); encoded != "" {
		return path + "?" + encoded
	}
	return path
}

func normalizeReplayWorkbenchSessionAnnotation(options ReplayWorkbenchSessionAnnotationOptions) (ReplayWorkbenchSessionAnnotation, error) {
	decision := strings.ToLower(strings.TrimSpace(options.Decision))
	switch decision {
	case "accepted", "deferred", "rejected":
	case "":
		return ReplayWorkbenchSessionAnnotation{}, fmt.Errorf("replay workbench session annotation decision is required")
	default:
		return ReplayWorkbenchSessionAnnotation{}, fmt.Errorf("unsupported replay workbench session annotation decision %q", options.Decision)
	}
	reviewer := strings.TrimSpace(options.Reviewer)
	if reviewer == "" {
		reviewer = "operator"
	}
	reviewedAt := options.ReviewedAt
	if reviewedAt.IsZero() {
		reviewedAt = time.Now().UTC()
	}
	return ReplayWorkbenchSessionAnnotation{
		Decision:   decision,
		Reason:     strings.TrimSpace(options.Reason),
		Reviewer:   reviewer,
		ReviewedAt: reviewedAt.UTC().Format(time.RFC3339),
	}, nil
}

func readReplayWorkbenchSessionFile(path string) (ReplayWorkbenchSessionArtifact, error) {
	if err := rejectUnsafeReplayWorkbenchFile(path, "replay workbench session file"); err != nil {
		return ReplayWorkbenchSessionArtifact{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ReplayWorkbenchSessionArtifact{}, fmt.Errorf("read replay workbench session: %w", err)
	}
	var session ReplayWorkbenchSessionArtifact
	if err := json.Unmarshal(data, &session); err != nil {
		return ReplayWorkbenchSessionArtifact{}, fmt.Errorf("decode replay workbench session: %w", err)
	}
	session.Path = path
	return session, nil
}

func resolveReplayWorkbenchSessionPath(root, sessionPath string) (string, error) {
	return resolveReplayWorkbenchJSONPath(replayWorkbenchSessionDir(root), sessionPath, "replay workbench session")
}

func replayWorkbenchSessionDir(root string) string {
	return filepath.Join(root, "replay", "workbench-sessions")
}

func replayWorkbenchSessionSelectionPath(root string) string {
	return filepath.Join(root, "replay", "workbench-session-selection.json")
}

func safeReplayWorkbenchSessionName(session ReplayWorkbenchSessionArtifact) string {
	parts := []string{
		"replay-workbench-session",
		session.PrimaryRunID,
		session.SelectedRunID,
		strings.Join(session.RunIDs, ","),
		session.CreatedAt,
		session.Reason,
	}
	for _, key := range []string{"edge", "lane", "node", "q"} {
		parts = append(parts, key+"="+session.Filters[key])
	}
	return safeInstallRequestName(strings.Join(parts, "|"))
}
