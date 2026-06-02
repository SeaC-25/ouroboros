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
)

type ReplayWorkbenchCollaborationArtifact struct {
	Path          string                                  `json:"path,omitempty"`
	SchemaVersion int                                     `json:"schema_version"`
	Status        string                                  `json:"status"`
	Mode          string                                  `json:"mode"`
	SessionPath   string                                  `json:"session_path"`
	PrimaryRunID  string                                  `json:"primary_run_id"`
	SelectedRunID string                                  `json:"selected_run_id"`
	RunIDs        int                                     `json:"run_ids"`
	Filters       int                                     `json:"filters"`
	Participants  []string                                `json:"participants"`
	Endpoints     map[string]string                       `json:"endpoints"`
	CreatedBy     string                                  `json:"created_by"`
	CreatedAt     string                                  `json:"created_at"`
	Reason        string                                  `json:"reason,omitempty"`
	Annotation    *ReplayWorkbenchCollaborationAnnotation `json:"annotation,omitempty"`
}

type ReplayWorkbenchCollaborationOptions struct {
	SessionPath  string
	Participants []string
	CreatedBy    string
	CreatedAt    time.Time
	Reason       string
}

type ReplayWorkbenchCollaborationAnnotation struct {
	Decision   string `json:"decision"`
	Reason     string `json:"reason,omitempty"`
	Reviewer   string `json:"reviewer,omitempty"`
	ReviewedAt string `json:"reviewed_at,omitempty"`
}

type ReplayWorkbenchCollaborationAnnotationOptions struct {
	Decision   string
	Reason     string
	Reviewer   string
	ReviewedAt time.Time
}

type ReplayWorkbenchCollaborationSelection struct {
	Path              string   `json:"path,omitempty"`
	Status            string   `json:"status"`
	Mode              string   `json:"mode"`
	CollaborationPath string   `json:"collaboration_path,omitempty"`
	SessionPath       string   `json:"session_path,omitempty"`
	PrimaryRunID      string   `json:"primary_run_id,omitempty"`
	SelectedRunID     string   `json:"selected_run_id,omitempty"`
	RunIDs            int      `json:"run_ids,omitempty"`
	Filters           int      `json:"filters,omitempty"`
	Endpoints         int      `json:"endpoints,omitempty"`
	Participants      int      `json:"participants,omitempty"`
	SupersededPaths   []string `json:"superseded_paths,omitempty"`
	SelectedBy        string   `json:"selected_by,omitempty"`
	SelectedAt        string   `json:"selected_at,omitempty"`
	ClearedBy         string   `json:"cleared_by,omitempty"`
	ClearedAt         string   `json:"cleared_at,omitempty"`
	Reason            string   `json:"reason,omitempty"`
}

type ReplayWorkbenchCollaborationSelectionOptions struct {
	SelectedBy string
	SelectedAt time.Time
	Reason     string
}

type ReplayWorkbenchCollaborationSelectionClearOptions struct {
	ClearedBy string
	ClearedAt time.Time
	Reason    string
}

type ReplayWorkbenchCollaborationPresence struct {
	Path              string `json:"path,omitempty"`
	SchemaVersion     int    `json:"schema_version"`
	Status            string `json:"status"`
	Mode              string `json:"mode"`
	CollaborationPath string `json:"collaboration_path"`
	SessionPath       string `json:"session_path"`
	PrimaryRunID      string `json:"primary_run_id"`
	SelectedRunID     string `json:"selected_run_id"`
	Participant       string `json:"participant"`
	SeenBy            string `json:"seen_by"`
	SeenAt            string `json:"seen_at"`
	Reason            string `json:"reason,omitempty"`
}

type ReplayWorkbenchCollaborationNote struct {
	Path              string                                      `json:"path,omitempty"`
	SchemaVersion     int                                         `json:"schema_version"`
	Status            string                                      `json:"status"`
	Mode              string                                      `json:"mode"`
	CollaborationPath string                                      `json:"collaboration_path"`
	SessionPath       string                                      `json:"session_path"`
	PrimaryRunID      string                                      `json:"primary_run_id"`
	SelectedRunID     string                                      `json:"selected_run_id"`
	Author            string                                      `json:"author"`
	Body              string                                      `json:"body"`
	TargetKind        string                                      `json:"target_kind,omitempty"`
	TargetID          string                                      `json:"target_id,omitempty"`
	CreatedAt         string                                      `json:"created_at"`
	Reason            string                                      `json:"reason,omitempty"`
	Annotation        *ReplayWorkbenchCollaborationNoteAnnotation `json:"annotation,omitempty"`
}

type ReplayWorkbenchCollaborationPresenceOptions struct {
	CollaborationPath string
	Participant       string
	Status            string
	SeenBy            string
	SeenAt            time.Time
	Reason            string
}

type ReplayWorkbenchCollaborationNoteOptions struct {
	CollaborationPath string
	Author            string
	Body              string
	TargetKind        string
	TargetID          string
	CreatedAt         time.Time
	Reason            string
}

type ReplayWorkbenchCollaborationNoteAnnotation struct {
	Decision   string `json:"decision"`
	Reason     string `json:"reason,omitempty"`
	Reviewer   string `json:"reviewer,omitempty"`
	ReviewedAt string `json:"reviewed_at,omitempty"`
}

type ReplayWorkbenchCollaborationNoteAnnotationOptions struct {
	Decision   string
	Reason     string
	Reviewer   string
	ReviewedAt time.Time
}

type ReplayWorkbenchCollaborationNoteSelection struct {
	Path              string   `json:"path,omitempty"`
	Status            string   `json:"status"`
	Mode              string   `json:"mode"`
	NotePath          string   `json:"note_path,omitempty"`
	CollaborationPath string   `json:"collaboration_path,omitempty"`
	SessionPath       string   `json:"session_path,omitempty"`
	PrimaryRunID      string   `json:"primary_run_id,omitempty"`
	SelectedRunID     string   `json:"selected_run_id,omitempty"`
	Author            string   `json:"author,omitempty"`
	TargetKind        string   `json:"target_kind,omitempty"`
	TargetID          string   `json:"target_id,omitempty"`
	SupersededPaths   []string `json:"superseded_paths,omitempty"`
	SelectedBy        string   `json:"selected_by,omitempty"`
	SelectedAt        string   `json:"selected_at,omitempty"`
	ClearedBy         string   `json:"cleared_by,omitempty"`
	ClearedAt         string   `json:"cleared_at,omitempty"`
	Reason            string   `json:"reason,omitempty"`
}

type ReplayWorkbenchCollaborationNoteSelectionOptions struct {
	SelectedBy string
	SelectedAt time.Time
	Reason     string
}

type ReplayWorkbenchCollaborationNoteSelectionClearOptions struct {
	ClearedBy string
	ClearedAt time.Time
	Reason    string
}

func PreviewReplayWorkbenchCollaboration(ctx context.Context, root string, options ReplayWorkbenchCollaborationOptions) (ReplayWorkbenchCollaborationArtifact, error) {
	collaboration, err := buildReplayWorkbenchCollaboration(ctx, root, options)
	if err != nil {
		return ReplayWorkbenchCollaborationArtifact{}, err
	}
	collaboration.Status = "dry-run"
	return collaboration, nil
}

func CreateReplayWorkbenchCollaboration(ctx context.Context, root string, options ReplayWorkbenchCollaborationOptions) (ReplayWorkbenchCollaborationArtifact, error) {
	collaboration, err := buildReplayWorkbenchCollaboration(ctx, root, options)
	if err != nil {
		return ReplayWorkbenchCollaborationArtifact{}, err
	}
	collaboration.Status = "saved"
	dir := replayWorkbenchCollaborationDir(root)
	if err := ensureReplayWorkbenchDirectory(dir, "replay workbench collaboration directory"); err != nil {
		return ReplayWorkbenchCollaborationArtifact{}, err
	}
	collaboration.Path = filepath.Join(dir, safeReplayWorkbenchCollaborationName(collaboration)+".json")
	if err := rejectUnsafeReplayWorkbenchFile(collaboration.Path, "replay workbench collaboration file"); err != nil {
		return ReplayWorkbenchCollaborationArtifact{}, err
	}
	if err := writeReplayWorkbenchCollaboration(collaboration); err != nil {
		return ReplayWorkbenchCollaborationArtifact{}, err
	}
	return collaboration, nil
}

func ListReplayWorkbenchCollaborations(root string) ([]ReplayWorkbenchCollaborationArtifact, error) {
	doctor, err := DoctorWorkspace(root)
	if err != nil {
		return nil, err
	}
	if !doctor.Ready {
		return nil, fmt.Errorf("workspace is not ready: %v", doctor.Errors)
	}
	dir := replayWorkbenchCollaborationDir(root)
	if err := rejectUnsafeReplayWorkbenchDirectory(dir, "replay workbench collaboration directory"); err != nil {
		return nil, err
	}
	paths, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		return nil, fmt.Errorf("glob replay workbench collaborations: %w", err)
	}
	sort.Strings(paths)
	collaborations := make([]ReplayWorkbenchCollaborationArtifact, 0, len(paths))
	for _, path := range paths {
		collaboration, err := readReplayWorkbenchCollaborationFile(path)
		if err != nil {
			return nil, err
		}
		collaborations = append(collaborations, collaboration)
	}
	return collaborations, nil
}

func ReadReplayWorkbenchCollaboration(root, collaborationPath string) (ReplayWorkbenchCollaborationArtifact, error) {
	doctor, err := DoctorWorkspace(root)
	if err != nil {
		return ReplayWorkbenchCollaborationArtifact{}, err
	}
	if !doctor.Ready {
		return ReplayWorkbenchCollaborationArtifact{}, fmt.Errorf("workspace is not ready: %v", doctor.Errors)
	}
	path, err := resolveReplayWorkbenchCollaborationPath(root, collaborationPath)
	if err != nil {
		return ReplayWorkbenchCollaborationArtifact{}, err
	}
	return readReplayWorkbenchCollaborationFile(path)
}

func AnnotateReplayWorkbenchCollaboration(root, collaborationPath string, options ReplayWorkbenchCollaborationAnnotationOptions) (ReplayWorkbenchCollaborationArtifact, error) {
	annotation, err := normalizeReplayWorkbenchCollaborationAnnotation(options)
	if err != nil {
		return ReplayWorkbenchCollaborationArtifact{}, err
	}
	doctor, err := DoctorWorkspace(root)
	if err != nil {
		return ReplayWorkbenchCollaborationArtifact{}, err
	}
	if !doctor.Ready {
		return ReplayWorkbenchCollaborationArtifact{}, fmt.Errorf("workspace is not ready: %v", doctor.Errors)
	}
	path, err := resolveReplayWorkbenchCollaborationPath(root, collaborationPath)
	if err != nil {
		return ReplayWorkbenchCollaborationArtifact{}, err
	}
	collaboration, err := readReplayWorkbenchCollaborationFile(path)
	if err != nil {
		return ReplayWorkbenchCollaborationArtifact{}, err
	}
	collaboration.Annotation = &annotation
	if err := writeReplayWorkbenchCollaboration(collaboration); err != nil {
		return ReplayWorkbenchCollaborationArtifact{}, err
	}
	collaboration.Path = path
	return collaboration, nil
}

func SelectReplayWorkbenchCollaboration(root, collaborationPath string, options ReplayWorkbenchCollaborationSelectionOptions) (ReplayWorkbenchCollaborationSelection, error) {
	collaboration, err := ReadReplayWorkbenchCollaboration(root, collaborationPath)
	if err != nil {
		return ReplayWorkbenchCollaborationSelection{}, err
	}
	collaborations, err := ListReplayWorkbenchCollaborations(root)
	if err != nil {
		return ReplayWorkbenchCollaborationSelection{}, err
	}
	selectedBy := strings.TrimSpace(options.SelectedBy)
	if selectedBy == "" {
		selectedBy = "operator"
	}
	selectedAt := options.SelectedAt
	if selectedAt.IsZero() {
		selectedAt = time.Now().UTC()
	}
	superseded := make([]string, 0, len(collaborations))
	for _, candidate := range collaborations {
		if candidate.Path == "" || candidate.Path == collaboration.Path {
			continue
		}
		superseded = append(superseded, candidate.Path)
	}
	sort.Strings(superseded)
	selection := ReplayWorkbenchCollaborationSelection{
		Path:              replayWorkbenchCollaborationSelectionPath(root),
		Status:            "current",
		Mode:              "metadata-only",
		CollaborationPath: collaboration.Path,
		SessionPath:       collaboration.SessionPath,
		PrimaryRunID:      collaboration.PrimaryRunID,
		SelectedRunID:     collaboration.SelectedRunID,
		RunIDs:            collaboration.RunIDs,
		Filters:           collaboration.Filters,
		Endpoints:         len(collaboration.Endpoints),
		Participants:      len(collaboration.Participants),
		SupersededPaths:   superseded,
		SelectedBy:        selectedBy,
		SelectedAt:        selectedAt.UTC().Format(time.RFC3339),
		Reason:            strings.TrimSpace(options.Reason),
	}
	if err := writeReplayWorkbenchCollaborationSelection(selection); err != nil {
		return ReplayWorkbenchCollaborationSelection{}, err
	}
	return selection, nil
}

func ClearReplayWorkbenchCollaborationSelection(root string, options ReplayWorkbenchCollaborationSelectionClearOptions) (ReplayWorkbenchCollaborationSelection, error) {
	selection, found, err := ReadReplayWorkbenchCollaborationSelection(root)
	if err != nil {
		return ReplayWorkbenchCollaborationSelection{}, err
	}
	if !found {
		selection = ReplayWorkbenchCollaborationSelection{
			Path:   replayWorkbenchCollaborationSelectionPath(root),
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
	selection.Path = replayWorkbenchCollaborationSelectionPath(root)
	selection.Status = "cleared"
	selection.Mode = "metadata-only"
	selection.ClearedBy = clearedBy
	selection.ClearedAt = clearedAt.UTC().Format(time.RFC3339)
	selection.Reason = strings.TrimSpace(options.Reason)
	if err := writeReplayWorkbenchCollaborationSelection(selection); err != nil {
		return ReplayWorkbenchCollaborationSelection{}, err
	}
	return selection, nil
}

func ReadReplayWorkbenchCollaborationSelection(root string) (ReplayWorkbenchCollaborationSelection, bool, error) {
	doctor, err := DoctorWorkspace(root)
	if err != nil {
		return ReplayWorkbenchCollaborationSelection{}, false, err
	}
	if !doctor.Ready {
		return ReplayWorkbenchCollaborationSelection{}, false, fmt.Errorf("workspace is not ready: %v", doctor.Errors)
	}
	path := replayWorkbenchCollaborationSelectionPath(root)
	if err := rejectUnsafeReplayWorkbenchFile(path, "replay workbench collaboration selection file"); err != nil {
		return ReplayWorkbenchCollaborationSelection{}, false, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return ReplayWorkbenchCollaborationSelection{}, false, nil
		}
		return ReplayWorkbenchCollaborationSelection{}, false, fmt.Errorf("read replay workbench collaboration selection %s: %w", path, err)
	}
	var selection ReplayWorkbenchCollaborationSelection
	if err := json.Unmarshal(data, &selection); err != nil {
		return ReplayWorkbenchCollaborationSelection{}, false, fmt.Errorf("decode replay workbench collaboration selection %s: %w", path, err)
	}
	selection.Path = path
	return selection, true, nil
}

func PreviewReplayWorkbenchCollaborationPresence(ctx context.Context, root string, options ReplayWorkbenchCollaborationPresenceOptions) (ReplayWorkbenchCollaborationPresence, error) {
	return buildReplayWorkbenchCollaborationPresence(ctx, root, options)
}

func UpdateReplayWorkbenchCollaborationPresence(ctx context.Context, root string, options ReplayWorkbenchCollaborationPresenceOptions) (ReplayWorkbenchCollaborationPresence, error) {
	presence, err := buildReplayWorkbenchCollaborationPresence(ctx, root, options)
	if err != nil {
		return ReplayWorkbenchCollaborationPresence{}, err
	}
	presence.Path = filepath.Join(replayWorkbenchCollaborationPresenceDir(root), safeReplayWorkbenchCollaborationPresenceName(presence)+".json")
	if err := ensureReplayWorkbenchDirectory(filepath.Dir(presence.Path), "replay workbench collaboration presence directory"); err != nil {
		return ReplayWorkbenchCollaborationPresence{}, err
	}
	if err := rejectUnsafeReplayWorkbenchFile(presence.Path, "replay workbench collaboration presence file"); err != nil {
		return ReplayWorkbenchCollaborationPresence{}, err
	}
	if err := writeReplayWorkbenchCollaborationPresence(presence); err != nil {
		return ReplayWorkbenchCollaborationPresence{}, err
	}
	return presence, nil
}

func ListReplayWorkbenchCollaborationPresence(root string) ([]ReplayWorkbenchCollaborationPresence, error) {
	doctor, err := DoctorWorkspace(root)
	if err != nil {
		return nil, err
	}
	if !doctor.Ready {
		return nil, fmt.Errorf("workspace is not ready: %v", doctor.Errors)
	}
	dir := replayWorkbenchCollaborationPresenceDir(root)
	if err := rejectUnsafeReplayWorkbenchDirectory(dir, "replay workbench collaboration presence directory"); err != nil {
		return nil, err
	}
	paths, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		return nil, fmt.Errorf("glob replay workbench collaboration presence: %w", err)
	}
	sort.Strings(paths)
	presence := make([]ReplayWorkbenchCollaborationPresence, 0, len(paths))
	for _, path := range paths {
		item, err := readReplayWorkbenchCollaborationPresenceFile(path)
		if err != nil {
			return nil, err
		}
		presence = append(presence, item)
	}
	return presence, nil
}

func ReadReplayWorkbenchCollaborationPresence(root, presencePath string) (ReplayWorkbenchCollaborationPresence, error) {
	doctor, err := DoctorWorkspace(root)
	if err != nil {
		return ReplayWorkbenchCollaborationPresence{}, err
	}
	if !doctor.Ready {
		return ReplayWorkbenchCollaborationPresence{}, fmt.Errorf("workspace is not ready: %v", doctor.Errors)
	}
	path, err := resolveReplayWorkbenchCollaborationPresencePath(root, presencePath)
	if err != nil {
		return ReplayWorkbenchCollaborationPresence{}, err
	}
	return readReplayWorkbenchCollaborationPresenceFile(path)
}

func PreviewReplayWorkbenchCollaborationNote(ctx context.Context, root string, options ReplayWorkbenchCollaborationNoteOptions) (ReplayWorkbenchCollaborationNote, error) {
	return buildReplayWorkbenchCollaborationNote(ctx, root, options)
}

func CreateReplayWorkbenchCollaborationNote(ctx context.Context, root string, options ReplayWorkbenchCollaborationNoteOptions) (ReplayWorkbenchCollaborationNote, error) {
	note, err := buildReplayWorkbenchCollaborationNote(ctx, root, options)
	if err != nil {
		return ReplayWorkbenchCollaborationNote{}, err
	}
	note.Status = "saved"
	note.Path = filepath.Join(replayWorkbenchCollaborationNotesDir(root), safeReplayWorkbenchCollaborationNoteName(note)+".json")
	if err := ensureReplayWorkbenchDirectory(filepath.Dir(note.Path), "replay workbench collaboration note directory"); err != nil {
		return ReplayWorkbenchCollaborationNote{}, err
	}
	if err := rejectUnsafeReplayWorkbenchFile(note.Path, "replay workbench collaboration note file"); err != nil {
		return ReplayWorkbenchCollaborationNote{}, err
	}
	if err := writeReplayWorkbenchCollaborationNote(note); err != nil {
		return ReplayWorkbenchCollaborationNote{}, err
	}
	return note, nil
}

func ListReplayWorkbenchCollaborationNotes(root string) ([]ReplayWorkbenchCollaborationNote, error) {
	doctor, err := DoctorWorkspace(root)
	if err != nil {
		return nil, err
	}
	if !doctor.Ready {
		return nil, fmt.Errorf("workspace is not ready: %v", doctor.Errors)
	}
	dir := replayWorkbenchCollaborationNotesDir(root)
	if err := rejectUnsafeReplayWorkbenchDirectory(dir, "replay workbench collaboration note directory"); err != nil {
		return nil, err
	}
	paths, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		return nil, fmt.Errorf("glob replay workbench collaboration notes: %w", err)
	}
	sort.Strings(paths)
	notes := make([]ReplayWorkbenchCollaborationNote, 0, len(paths))
	for _, path := range paths {
		note, err := readReplayWorkbenchCollaborationNoteFile(path)
		if err != nil {
			return nil, err
		}
		notes = append(notes, note)
	}
	return notes, nil
}

func ReadReplayWorkbenchCollaborationNote(root, notePath string) (ReplayWorkbenchCollaborationNote, error) {
	doctor, err := DoctorWorkspace(root)
	if err != nil {
		return ReplayWorkbenchCollaborationNote{}, err
	}
	if !doctor.Ready {
		return ReplayWorkbenchCollaborationNote{}, fmt.Errorf("workspace is not ready: %v", doctor.Errors)
	}
	path, err := resolveReplayWorkbenchCollaborationNotePath(root, notePath)
	if err != nil {
		return ReplayWorkbenchCollaborationNote{}, err
	}
	return readReplayWorkbenchCollaborationNoteFile(path)
}

func AnnotateReplayWorkbenchCollaborationNote(root, notePath string, options ReplayWorkbenchCollaborationNoteAnnotationOptions) (ReplayWorkbenchCollaborationNote, error) {
	annotation, err := normalizeReplayWorkbenchCollaborationNoteAnnotation(options)
	if err != nil {
		return ReplayWorkbenchCollaborationNote{}, err
	}
	doctor, err := DoctorWorkspace(root)
	if err != nil {
		return ReplayWorkbenchCollaborationNote{}, err
	}
	if !doctor.Ready {
		return ReplayWorkbenchCollaborationNote{}, fmt.Errorf("workspace is not ready: %v", doctor.Errors)
	}
	path, err := resolveReplayWorkbenchCollaborationNotePath(root, notePath)
	if err != nil {
		return ReplayWorkbenchCollaborationNote{}, err
	}
	note, err := readReplayWorkbenchCollaborationNoteFile(path)
	if err != nil {
		return ReplayWorkbenchCollaborationNote{}, err
	}
	note.Annotation = &annotation
	if err := writeReplayWorkbenchCollaborationNote(note); err != nil {
		return ReplayWorkbenchCollaborationNote{}, err
	}
	note.Path = path
	return note, nil
}

func SelectReplayWorkbenchCollaborationNote(root, notePath string, options ReplayWorkbenchCollaborationNoteSelectionOptions) (ReplayWorkbenchCollaborationNoteSelection, error) {
	note, err := ReadReplayWorkbenchCollaborationNote(root, notePath)
	if err != nil {
		return ReplayWorkbenchCollaborationNoteSelection{}, err
	}
	notes, err := ListReplayWorkbenchCollaborationNotes(root)
	if err != nil {
		return ReplayWorkbenchCollaborationNoteSelection{}, err
	}
	selectedBy := strings.TrimSpace(options.SelectedBy)
	if selectedBy == "" {
		selectedBy = "operator"
	}
	selectedAt := options.SelectedAt
	if selectedAt.IsZero() {
		selectedAt = time.Now().UTC()
	}
	superseded := make([]string, 0, len(notes))
	for _, candidate := range notes {
		if candidate.Path == "" || candidate.Path == note.Path {
			continue
		}
		superseded = append(superseded, candidate.Path)
	}
	sort.Strings(superseded)
	selection := ReplayWorkbenchCollaborationNoteSelection{
		Path:              replayWorkbenchCollaborationNoteSelectionPath(root),
		Status:            "current",
		Mode:              "metadata-only",
		NotePath:          note.Path,
		CollaborationPath: note.CollaborationPath,
		SessionPath:       note.SessionPath,
		PrimaryRunID:      note.PrimaryRunID,
		SelectedRunID:     note.SelectedRunID,
		Author:            note.Author,
		TargetKind:        note.TargetKind,
		TargetID:          note.TargetID,
		SupersededPaths:   superseded,
		SelectedBy:        selectedBy,
		SelectedAt:        selectedAt.UTC().Format(time.RFC3339),
		Reason:            strings.TrimSpace(options.Reason),
	}
	if err := writeReplayWorkbenchCollaborationNoteSelection(selection); err != nil {
		return ReplayWorkbenchCollaborationNoteSelection{}, err
	}
	return selection, nil
}

func ClearReplayWorkbenchCollaborationNoteSelection(root string, options ReplayWorkbenchCollaborationNoteSelectionClearOptions) (ReplayWorkbenchCollaborationNoteSelection, error) {
	selection, found, err := ReadReplayWorkbenchCollaborationNoteSelection(root)
	if err != nil {
		return ReplayWorkbenchCollaborationNoteSelection{}, err
	}
	if !found {
		selection = ReplayWorkbenchCollaborationNoteSelection{
			Path:   replayWorkbenchCollaborationNoteSelectionPath(root),
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
	selection.Path = replayWorkbenchCollaborationNoteSelectionPath(root)
	selection.Status = "cleared"
	selection.Mode = "metadata-only"
	selection.ClearedBy = clearedBy
	selection.ClearedAt = clearedAt.UTC().Format(time.RFC3339)
	selection.Reason = strings.TrimSpace(options.Reason)
	if err := writeReplayWorkbenchCollaborationNoteSelection(selection); err != nil {
		return ReplayWorkbenchCollaborationNoteSelection{}, err
	}
	return selection, nil
}

func ReadReplayWorkbenchCollaborationNoteSelection(root string) (ReplayWorkbenchCollaborationNoteSelection, bool, error) {
	doctor, err := DoctorWorkspace(root)
	if err != nil {
		return ReplayWorkbenchCollaborationNoteSelection{}, false, err
	}
	if !doctor.Ready {
		return ReplayWorkbenchCollaborationNoteSelection{}, false, fmt.Errorf("workspace is not ready: %v", doctor.Errors)
	}
	path := replayWorkbenchCollaborationNoteSelectionPath(root)
	if err := rejectUnsafeReplayWorkbenchFile(path, "replay workbench collaboration note selection file"); err != nil {
		return ReplayWorkbenchCollaborationNoteSelection{}, false, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return ReplayWorkbenchCollaborationNoteSelection{}, false, nil
		}
		return ReplayWorkbenchCollaborationNoteSelection{}, false, fmt.Errorf("read replay workbench collaboration note selection %s: %w", path, err)
	}
	var selection ReplayWorkbenchCollaborationNoteSelection
	if err := json.Unmarshal(data, &selection); err != nil {
		return ReplayWorkbenchCollaborationNoteSelection{}, false, fmt.Errorf("decode replay workbench collaboration note selection %s: %w", path, err)
	}
	selection.Path = path
	return selection, true, nil
}

func buildReplayWorkbenchCollaboration(_ context.Context, root string, options ReplayWorkbenchCollaborationOptions) (ReplayWorkbenchCollaborationArtifact, error) {
	session, err := ReadReplayWorkbenchSession(root, options.SessionPath)
	if err != nil {
		return ReplayWorkbenchCollaborationArtifact{}, err
	}
	createdBy := strings.TrimSpace(options.CreatedBy)
	if createdBy == "" {
		createdBy = "operator"
	}
	participants := normalizeReplayWorkbenchCollaborationParticipants(options.Participants)
	if len(participants) == 0 {
		participants = []string{createdBy}
	}
	createdAt := options.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}
	return ReplayWorkbenchCollaborationArtifact{
		SchemaVersion: 1,
		Status:        "dry-run",
		Mode:          "metadata-only",
		SessionPath:   session.Path,
		PrimaryRunID:  session.PrimaryRunID,
		SelectedRunID: session.SelectedRunID,
		RunIDs:        len(session.RunIDs),
		Filters:       len(session.Filters),
		Participants:  participants,
		Endpoints:     replayWorkbenchCollaborationEndpoints(session),
		CreatedBy:     createdBy,
		CreatedAt:     createdAt.UTC().Format(time.RFC3339),
		Reason:        strings.TrimSpace(options.Reason),
	}, nil
}

func buildReplayWorkbenchCollaborationPresence(_ context.Context, root string, options ReplayWorkbenchCollaborationPresenceOptions) (ReplayWorkbenchCollaborationPresence, error) {
	collaboration, err := ReadReplayWorkbenchCollaboration(root, options.CollaborationPath)
	if err != nil {
		return ReplayWorkbenchCollaborationPresence{}, err
	}
	participant := strings.TrimSpace(options.Participant)
	if participant == "" {
		return ReplayWorkbenchCollaborationPresence{}, fmt.Errorf("replay workbench collaboration presence participant is required")
	}
	status, err := normalizeReplayWorkbenchCollaborationPresenceStatus(options.Status)
	if err != nil {
		return ReplayWorkbenchCollaborationPresence{}, err
	}
	seenBy := strings.TrimSpace(options.SeenBy)
	if seenBy == "" {
		seenBy = participant
	}
	seenAt := options.SeenAt
	if seenAt.IsZero() {
		seenAt = time.Now().UTC()
	}
	return ReplayWorkbenchCollaborationPresence{
		SchemaVersion:     1,
		Status:            status,
		Mode:              "metadata-only",
		CollaborationPath: collaboration.Path,
		SessionPath:       collaboration.SessionPath,
		PrimaryRunID:      collaboration.PrimaryRunID,
		SelectedRunID:     collaboration.SelectedRunID,
		Participant:       participant,
		SeenBy:            seenBy,
		SeenAt:            seenAt.UTC().Format(time.RFC3339),
		Reason:            strings.TrimSpace(options.Reason),
	}, nil
}

func buildReplayWorkbenchCollaborationNote(_ context.Context, root string, options ReplayWorkbenchCollaborationNoteOptions) (ReplayWorkbenchCollaborationNote, error) {
	collaboration, err := ReadReplayWorkbenchCollaboration(root, options.CollaborationPath)
	if err != nil {
		return ReplayWorkbenchCollaborationNote{}, err
	}
	body := strings.TrimSpace(options.Body)
	if body == "" {
		return ReplayWorkbenchCollaborationNote{}, fmt.Errorf("replay workbench collaboration note body is required")
	}
	author := strings.TrimSpace(options.Author)
	if author == "" {
		author = "operator"
	}
	targetKind := strings.TrimSpace(options.TargetKind)
	targetID := strings.TrimSpace(options.TargetID)
	if targetKind == "" && targetID != "" {
		targetKind = "artifact"
	}
	createdAt := options.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}
	return ReplayWorkbenchCollaborationNote{
		SchemaVersion:     1,
		Status:            "dry-run",
		Mode:              "metadata-only",
		CollaborationPath: collaboration.Path,
		SessionPath:       collaboration.SessionPath,
		PrimaryRunID:      collaboration.PrimaryRunID,
		SelectedRunID:     collaboration.SelectedRunID,
		Author:            author,
		Body:              body,
		TargetKind:        targetKind,
		TargetID:          targetID,
		CreatedAt:         createdAt.UTC().Format(time.RFC3339),
		Reason:            strings.TrimSpace(options.Reason),
	}, nil
}

func normalizeReplayWorkbenchCollaborationParticipants(input []string) []string {
	seen := map[string]bool{}
	participants := make([]string, 0, len(input))
	for _, participant := range input {
		participant = strings.TrimSpace(participant)
		if participant == "" || seen[participant] {
			continue
		}
		seen[participant] = true
		participants = append(participants, participant)
	}
	return participants
}

func normalizeReplayWorkbenchCollaborationPresenceStatus(input string) (string, error) {
	status := strings.ToLower(strings.TrimSpace(input))
	if status == "" {
		status = "online"
	}
	switch status {
	case "online", "away", "offline":
		return status, nil
	default:
		return "", fmt.Errorf("unsupported replay workbench collaboration presence status %q", input)
	}
}

func normalizeReplayWorkbenchCollaborationAnnotation(options ReplayWorkbenchCollaborationAnnotationOptions) (ReplayWorkbenchCollaborationAnnotation, error) {
	decision := strings.ToLower(strings.TrimSpace(options.Decision))
	switch decision {
	case "accepted", "deferred", "rejected":
	case "":
		return ReplayWorkbenchCollaborationAnnotation{}, fmt.Errorf("replay workbench collaboration annotation decision is required")
	default:
		return ReplayWorkbenchCollaborationAnnotation{}, fmt.Errorf("unsupported replay workbench collaboration annotation decision %q", options.Decision)
	}
	reviewer := strings.TrimSpace(options.Reviewer)
	if reviewer == "" {
		reviewer = "operator"
	}
	reviewedAt := options.ReviewedAt
	if reviewedAt.IsZero() {
		reviewedAt = time.Now().UTC()
	}
	return ReplayWorkbenchCollaborationAnnotation{
		Decision:   decision,
		Reason:     strings.TrimSpace(options.Reason),
		Reviewer:   reviewer,
		ReviewedAt: reviewedAt.UTC().Format(time.RFC3339),
	}, nil
}

func normalizeReplayWorkbenchCollaborationNoteAnnotation(options ReplayWorkbenchCollaborationNoteAnnotationOptions) (ReplayWorkbenchCollaborationNoteAnnotation, error) {
	decision := strings.ToLower(strings.TrimSpace(options.Decision))
	switch decision {
	case "accepted", "deferred", "rejected":
	case "":
		return ReplayWorkbenchCollaborationNoteAnnotation{}, fmt.Errorf("replay workbench collaboration note annotation decision is required")
	default:
		return ReplayWorkbenchCollaborationNoteAnnotation{}, fmt.Errorf("unsupported replay workbench collaboration note annotation decision %q", options.Decision)
	}
	reviewer := strings.TrimSpace(options.Reviewer)
	if reviewer == "" {
		reviewer = "operator"
	}
	reviewedAt := options.ReviewedAt
	if reviewedAt.IsZero() {
		reviewedAt = time.Now().UTC()
	}
	return ReplayWorkbenchCollaborationNoteAnnotation{
		Decision:   decision,
		Reason:     strings.TrimSpace(options.Reason),
		Reviewer:   reviewer,
		ReviewedAt: reviewedAt.UTC().Format(time.RFC3339),
	}, nil
}

func replayWorkbenchCollaborationEndpoints(session ReplayWorkbenchSessionArtifact) map[string]string {
	endpoints := map[string]string{
		"session":        replayWorkbenchSessionPathWithQuery("/api/session", replayWorkbenchSessionQuery(session.Filters)),
		"sessions":       "/api/sessions",
		"collaborations": "/api/collaborations",
	}
	for _, key := range []string{"html", "replay", "replays", "graph", "graphs", "graph_ndjson", "timeline_ndjson"} {
		if value := strings.TrimSpace(session.Endpoints[key]); value != "" {
			endpoints[key] = value
		}
	}
	return endpoints
}

func writeReplayWorkbenchCollaboration(collaboration ReplayWorkbenchCollaborationArtifact) error {
	if err := rejectUnsafeReplayWorkbenchFile(collaboration.Path, "replay workbench collaboration file"); err != nil {
		return err
	}
	data, err := json.MarshalIndent(collaboration, "", "  ")
	if err != nil {
		return fmt.Errorf("encode replay workbench collaboration: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(collaboration.Path, data, 0o644); err != nil {
		return fmt.Errorf("write replay workbench collaboration: %w", err)
	}
	return nil
}

func writeReplayWorkbenchCollaborationPresence(presence ReplayWorkbenchCollaborationPresence) error {
	if err := rejectUnsafeReplayWorkbenchFile(presence.Path, "replay workbench collaboration presence file"); err != nil {
		return err
	}
	data, err := json.MarshalIndent(presence, "", "  ")
	if err != nil {
		return fmt.Errorf("encode replay workbench collaboration presence: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(presence.Path, data, 0o644); err != nil {
		return fmt.Errorf("write replay workbench collaboration presence: %w", err)
	}
	return nil
}

func writeReplayWorkbenchCollaborationNote(note ReplayWorkbenchCollaborationNote) error {
	if err := rejectUnsafeReplayWorkbenchFile(note.Path, "replay workbench collaboration note file"); err != nil {
		return err
	}
	data, err := json.MarshalIndent(note, "", "  ")
	if err != nil {
		return fmt.Errorf("encode replay workbench collaboration note: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(note.Path, data, 0o644); err != nil {
		return fmt.Errorf("write replay workbench collaboration note: %w", err)
	}
	return nil
}

func writeReplayWorkbenchCollaborationNoteSelection(selection ReplayWorkbenchCollaborationNoteSelection) error {
	if err := ensureReplayWorkbenchDirectory(filepath.Dir(selection.Path), "replay workbench directory"); err != nil {
		return err
	}
	if err := rejectUnsafeReplayWorkbenchFile(selection.Path, "replay workbench collaboration note selection file"); err != nil {
		return err
	}
	data, err := json.MarshalIndent(selection, "", "  ")
	if err != nil {
		return fmt.Errorf("encode replay workbench collaboration note selection: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(selection.Path, data, 0o644); err != nil {
		return fmt.Errorf("write replay workbench collaboration note selection: %w", err)
	}
	return nil
}

func writeReplayWorkbenchCollaborationSelection(selection ReplayWorkbenchCollaborationSelection) error {
	if err := ensureReplayWorkbenchDirectory(filepath.Dir(selection.Path), "replay workbench directory"); err != nil {
		return err
	}
	if err := rejectUnsafeReplayWorkbenchFile(selection.Path, "replay workbench collaboration selection file"); err != nil {
		return err
	}
	data, err := json.MarshalIndent(selection, "", "  ")
	if err != nil {
		return fmt.Errorf("encode replay workbench collaboration selection: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(selection.Path, data, 0o644); err != nil {
		return fmt.Errorf("write replay workbench collaboration selection: %w", err)
	}
	return nil
}

func readReplayWorkbenchCollaborationFile(path string) (ReplayWorkbenchCollaborationArtifact, error) {
	if err := rejectUnsafeReplayWorkbenchFile(path, "replay workbench collaboration file"); err != nil {
		return ReplayWorkbenchCollaborationArtifact{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ReplayWorkbenchCollaborationArtifact{}, fmt.Errorf("read replay workbench collaboration: %w", err)
	}
	var collaboration ReplayWorkbenchCollaborationArtifact
	if err := json.Unmarshal(data, &collaboration); err != nil {
		return ReplayWorkbenchCollaborationArtifact{}, fmt.Errorf("decode replay workbench collaboration: %w", err)
	}
	collaboration.Path = path
	return collaboration, nil
}

func readReplayWorkbenchCollaborationPresenceFile(path string) (ReplayWorkbenchCollaborationPresence, error) {
	if err := rejectUnsafeReplayWorkbenchFile(path, "replay workbench collaboration presence file"); err != nil {
		return ReplayWorkbenchCollaborationPresence{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ReplayWorkbenchCollaborationPresence{}, fmt.Errorf("read replay workbench collaboration presence: %w", err)
	}
	var presence ReplayWorkbenchCollaborationPresence
	if err := json.Unmarshal(data, &presence); err != nil {
		return ReplayWorkbenchCollaborationPresence{}, fmt.Errorf("decode replay workbench collaboration presence: %w", err)
	}
	presence.Path = path
	return presence, nil
}

func readReplayWorkbenchCollaborationNoteFile(path string) (ReplayWorkbenchCollaborationNote, error) {
	if err := rejectUnsafeReplayWorkbenchFile(path, "replay workbench collaboration note file"); err != nil {
		return ReplayWorkbenchCollaborationNote{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ReplayWorkbenchCollaborationNote{}, fmt.Errorf("read replay workbench collaboration note: %w", err)
	}
	var note ReplayWorkbenchCollaborationNote
	if err := json.Unmarshal(data, &note); err != nil {
		return ReplayWorkbenchCollaborationNote{}, fmt.Errorf("decode replay workbench collaboration note: %w", err)
	}
	note.Path = path
	return note, nil
}

func resolveReplayWorkbenchCollaborationPath(root, collaborationPath string) (string, error) {
	return resolveReplayWorkbenchJSONPath(replayWorkbenchCollaborationDir(root), collaborationPath, "replay workbench collaboration")
}

func resolveReplayWorkbenchCollaborationNotePath(root, notePath string) (string, error) {
	return resolveReplayWorkbenchJSONPath(replayWorkbenchCollaborationNotesDir(root), notePath, "replay workbench collaboration note")
}

func resolveReplayWorkbenchCollaborationPresencePath(root, presencePath string) (string, error) {
	return resolveReplayWorkbenchJSONPath(replayWorkbenchCollaborationPresenceDir(root), presencePath, "replay workbench collaboration presence")
}

func replayWorkbenchCollaborationDir(root string) string {
	return filepath.Join(root, "replay", "workbench-collaborations")
}

func replayWorkbenchCollaborationPresenceDir(root string) string {
	return filepath.Join(root, "replay", "workbench-collaboration-presence")
}

func replayWorkbenchCollaborationNotesDir(root string) string {
	return filepath.Join(root, "replay", "workbench-collaboration-notes")
}

func replayWorkbenchCollaborationSelectionPath(root string) string {
	return filepath.Join(root, "replay", "workbench-collaboration-selection.json")
}

func replayWorkbenchCollaborationNoteSelectionPath(root string) string {
	return filepath.Join(root, "replay", "workbench-collaboration-note-selection.json")
}

func safeReplayWorkbenchCollaborationName(collaboration ReplayWorkbenchCollaborationArtifact) string {
	return safeInstallRequestName(strings.Join([]string{
		"replay-workbench-collaboration",
		collaboration.SessionPath,
		collaboration.PrimaryRunID,
		collaboration.SelectedRunID,
		strings.Join(collaboration.Participants, ","),
		collaboration.CreatedAt,
		collaboration.Reason,
	}, "|"))
}

func safeReplayWorkbenchCollaborationNoteName(note ReplayWorkbenchCollaborationNote) string {
	return safeInstallRequestName(strings.Join([]string{
		"replay-workbench-collaboration-note",
		note.CollaborationPath,
		note.Author,
		note.TargetKind,
		note.TargetID,
		note.CreatedAt,
		note.Body,
	}, "|"))
}

func safeReplayWorkbenchCollaborationPresenceName(presence ReplayWorkbenchCollaborationPresence) string {
	return safeInstallRequestName(strings.Join([]string{
		"replay-workbench-collaboration-presence",
		presence.CollaborationPath,
		presence.Participant,
	}, "|"))
}
