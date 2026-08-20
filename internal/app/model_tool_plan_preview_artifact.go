package app

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/ouroboros/ouroboros/internal/compiler"
)

type ModelToolPlanPreviewExtraction struct {
	Found             bool
	Action            compiler.SemanticAction
	Valid             bool
	Reason            string
	MarkerPreview     string
	ModelOutputDigest string
}

type ModelToolPlanPreviewArtifact struct {
	SchemaVersion               int                     `json:"schema_version"`
	Path                        string                  `json:"-"`
	ArtifactID                  string                  `json:"artifact_id"`
	RunID                       string                  `json:"run_id"`
	Source                      string                  `json:"source"`
	AgentID                     string                  `json:"agent_id"`
	Role                        string                  `json:"role"`
	Status                      string                  `json:"status"`
	ExecutionMode               string                  `json:"execution_mode"`
	CreatedAt                   string                  `json:"created_at"`
	ProviderName                string                  `json:"provider_name"`
	Model                       string                  `json:"model"`
	Action                      compiler.SemanticAction `json:"action"`
	Valid                       bool                    `json:"valid"`
	Reason                      string                  `json:"reason,omitempty"`
	MarkerPreview               string                  `json:"marker_preview,omitempty"`
	ModelOutputDigest           string                  `json:"model_output_digest"`
	ModelGenerationArtifactPath string                  `json:"model_generation_artifact_path,omitempty"`
	ReviewPath                  string                  `json:"compile_preview_review_path,omitempty"`
	ReviewNodes                 int                     `json:"compile_preview_nodes,omitempty"`
	ReviewWarnings              int                     `json:"compile_preview_warnings,omitempty"`
	ArtifactDigest              string                  `json:"artifact_digest"`
	AssistantTextPersisted      bool                    `json:"assistant_text_persisted"`
	ExecutedTools               int                     `json:"executed_tools"`
	WorkerDispatch              bool                    `json:"worker_dispatch"`
	TargetContact               bool                    `json:"target_contact"`
}

type ModelToolPlanPreviewArtifactOptions struct {
	Now     time.Time
	Source  string
	AgentID string
	Role    string
}

type ModelToolPlanPreviewAcceptance struct {
	SchemaVersion            int                     `json:"schema_version"`
	Path                     string                  `json:"-"`
	ArtifactID               string                  `json:"artifact_id"`
	Source                   string                  `json:"source"`
	Status                   string                  `json:"status"`
	ExecutionMode            string                  `json:"execution_mode"`
	AcceptedAt               string                  `json:"accepted_at"`
	AcceptedBy               string                  `json:"accepted_by"`
	Reason                   string                  `json:"reason,omitempty"`
	RunID                    string                  `json:"run_id"`
	ProviderName             string                  `json:"provider_name"`
	Model                    string                  `json:"model"`
	Action                   compiler.SemanticAction `json:"action"`
	PreviewArtifactPath      string                  `json:"model_tool_plan_preview_path"`
	ReviewPath               string                  `json:"compile_preview_review_path"`
	SelectionPath            string                  `json:"compile_preview_selection_path"`
	ReviewNodes              int                     `json:"compile_preview_nodes"`
	ReviewWarnings           int                     `json:"compile_preview_warnings"`
	ModelOutputDigest        string                  `json:"model_output_digest"`
	ModelGenerationArtifact  string                  `json:"model_generation_artifact_path,omitempty"`
	AcceptanceDigest         string                  `json:"acceptance_digest"`
	AssistantTextPersisted   bool                    `json:"assistant_text_persisted"`
	ExecutedTools            int                     `json:"executed_tools"`
	WorkerDispatch           bool                    `json:"worker_dispatch"`
	TargetContact            bool                    `json:"target_contact"`
	ExecutionDispatchCreated bool                    `json:"execution_dispatch_created"`
}

type ModelToolPlanPreviewAcceptanceOptions struct {
	AcceptedBy string
	AcceptedAt time.Time
	Reason     string
}

func ExtractModelToolPlanPreview(text string) ModelToolPlanPreviewExtraction {
	outputDigest := providerGenerationTextDigest(text)
	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		key, value, ok := splitModelToolPlanMarker(trimmed)
		if !ok {
			continue
		}
		fields := strings.Fields(value)
		if len(fields) == 0 {
			return ModelToolPlanPreviewExtraction{
				Found:             true,
				Reason:            "empty_semantic_action",
				MarkerPreview:     key + ":",
				ModelOutputDigest: outputDigest,
			}
		}
		action := compiler.SemanticAction(fields[0])
		extraction := ModelToolPlanPreviewExtraction{
			Found:             true,
			Action:            action,
			MarkerPreview:     key + ": " + string(action),
			ModelOutputDigest: outputDigest,
		}
		if len(fields) != 1 {
			extraction.Reason = "malformed_action_marker"
			return extraction
		}
		if !isDeclaredSemanticAction(action) {
			extraction.Reason = "unsupported_semantic_action"
			return extraction
		}
		extraction.Valid = true
		extraction.Reason = "accepted"
		return extraction
	}
	return ModelToolPlanPreviewExtraction{
		Reason:            "no_action_marker",
		ModelOutputDigest: outputDigest,
	}
}

func RecordModelToolPlanPreviewArtifact(root string, runID string, result ProviderGenerationResult, generationArtifactPath string, extraction ModelToolPlanPreviewExtraction, review ToolCompilePreviewReview, options ModelToolPlanPreviewArtifactOptions) (ModelToolPlanPreviewArtifact, error) {
	if !extraction.Valid {
		return ModelToolPlanPreviewArtifact{}, fmt.Errorf("model tool-plan preview artifact requires a valid semantic action")
	}
	dir := modelToolPlanPreviewArtifactDirectory(root)
	if err := ensureAuthoredToolDirectory(dir, "model tool-plan preview artifact directory"); err != nil {
		return ModelToolPlanPreviewArtifact{}, err
	}
	now := options.Now.UTC()
	if now.IsZero() {
		now = time.Now().UTC()
	}
	artifactID := modelToolPlanPreviewArtifactID(now, runID, result.ProviderName, result.Model, extraction.Action)
	path := filepath.Join(dir, artifactID+".json")
	if err := rejectUnsafeAuthoredToolFile(path, "model tool-plan preview artifact"); err != nil {
		return ModelToolPlanPreviewArtifact{}, err
	}
	artifact := ModelToolPlanPreviewArtifact{
		SchemaVersion:               1,
		Path:                        path,
		ArtifactID:                  artifactID,
		RunID:                       strings.TrimSpace(runID),
		Source:                      modelToolPlanPreviewOptionValue(options.Source, closedLoopPlannerGenerationStageID),
		AgentID:                     modelToolPlanPreviewOptionValue(options.AgentID, "closed-loop-planner"),
		Role:                        modelToolPlanPreviewOptionValue(options.Role, "planner"),
		Status:                      "completed",
		ExecutionMode:               "metadata-only",
		CreatedAt:                   now.Format(time.RFC3339),
		ProviderName:                strings.TrimSpace(result.ProviderName),
		Model:                       strings.TrimSpace(result.Model),
		Action:                      extraction.Action,
		Valid:                       true,
		Reason:                      extraction.Reason,
		MarkerPreview:               extraction.MarkerPreview,
		ModelOutputDigest:           strings.TrimSpace(extraction.ModelOutputDigest),
		ModelGenerationArtifactPath: strings.TrimSpace(generationArtifactPath),
		ReviewPath:                  strings.TrimSpace(review.Path),
		ReviewNodes:                 len(review.Preview.Nodes),
		ReviewWarnings:              len(review.Preview.Warnings),
		AssistantTextPersisted:      false,
		ExecutedTools:               0,
		WorkerDispatch:              false,
		TargetContact:               false,
	}
	artifact.ArtifactDigest = modelToolPlanPreviewArtifactDigest(artifact)
	data, err := json.MarshalIndent(artifact, "", "  ")
	if err != nil {
		return ModelToolPlanPreviewArtifact{}, fmt.Errorf("encode model tool-plan preview artifact: %w", err)
	}
	if err := writeAuthoredToolArtifactFile(path, data, "model tool-plan preview artifact"); err != nil {
		return ModelToolPlanPreviewArtifact{}, err
	}
	return artifact, nil
}

func modelToolPlanPreviewOptionValue(value string, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}

func AcceptModelToolPlanPreviewArtifact(root string, artifactPath string, options ModelToolPlanPreviewAcceptanceOptions) (ModelToolPlanPreviewAcceptance, error) {
	artifact, err := ReadModelToolPlanPreviewArtifact(root, artifactPath)
	if err != nil {
		return ModelToolPlanPreviewAcceptance{}, err
	}
	if !artifact.Valid {
		return ModelToolPlanPreviewAcceptance{}, fmt.Errorf("model tool-plan preview artifact is not valid: %s", artifact.Reason)
	}
	if strings.TrimSpace(artifact.Status) != "completed" {
		return ModelToolPlanPreviewAcceptance{}, fmt.Errorf("model tool-plan preview artifact status must be completed")
	}
	if strings.TrimSpace(artifact.ExecutionMode) != "metadata-only" {
		return ModelToolPlanPreviewAcceptance{}, fmt.Errorf("model tool-plan preview artifact mode must be metadata-only")
	}
	if strings.TrimSpace(artifact.ReviewPath) == "" {
		return ModelToolPlanPreviewAcceptance{}, fmt.Errorf("model tool-plan preview artifact is missing compile preview review path")
	}
	review, err := ReadToolCompilePreviewReview(root, artifact.ReviewPath)
	if err != nil {
		return ModelToolPlanPreviewAcceptance{}, err
	}
	if strings.TrimSpace(review.Status) != "dry-run" || strings.TrimSpace(review.Mode) != "metadata-only" {
		return ModelToolPlanPreviewAcceptance{}, fmt.Errorf("compile preview review must be dry-run metadata-only")
	}
	if review.Preview.Action != artifact.Action {
		return ModelToolPlanPreviewAcceptance{}, fmt.Errorf("compile preview review action %s does not match model tool-plan preview action %s", review.Preview.Action, artifact.Action)
	}
	acceptedBy := strings.TrimSpace(options.AcceptedBy)
	if acceptedBy == "" {
		acceptedBy = "operator"
	}
	acceptedAt := options.AcceptedAt.UTC()
	if acceptedAt.IsZero() {
		acceptedAt = time.Now().UTC()
	}
	dir := modelToolPlanPreviewAcceptanceDirectory(root)
	if err := ensureAuthoredToolDirectory(dir, "model tool-plan preview acceptance directory"); err != nil {
		return ModelToolPlanPreviewAcceptance{}, err
	}
	artifactID := modelToolPlanPreviewAcceptanceID(acceptedAt, artifact)
	path := filepath.Join(dir, artifactID+".json")
	if err := rejectUnsafeAuthoredToolFile(path, "model tool-plan preview acceptance"); err != nil {
		return ModelToolPlanPreviewAcceptance{}, err
	}
	selectionReason := "accepted model tool-plan preview"
	if reason := strings.TrimSpace(options.Reason); reason != "" {
		selectionReason += ": " + reason
	}
	selection, err := SelectToolCompilePreviewReview(root, review.Path, ToolCompilePreviewReviewSelectionOptions{
		SelectedBy: acceptedBy,
		SelectedAt: acceptedAt,
		Reason:     selectionReason,
	})
	if err != nil {
		return ModelToolPlanPreviewAcceptance{}, err
	}
	acceptance := ModelToolPlanPreviewAcceptance{
		SchemaVersion:            1,
		Path:                     path,
		ArtifactID:               artifactID,
		Source:                   "model-tool-plan-preview-acceptance",
		Status:                   "accepted",
		ExecutionMode:            "metadata-only",
		AcceptedAt:               acceptedAt.Format(time.RFC3339),
		AcceptedBy:               acceptedBy,
		Reason:                   strings.TrimSpace(options.Reason),
		RunID:                    strings.TrimSpace(artifact.RunID),
		ProviderName:             strings.TrimSpace(artifact.ProviderName),
		Model:                    strings.TrimSpace(artifact.Model),
		Action:                   artifact.Action,
		PreviewArtifactPath:      strings.TrimSpace(artifact.Path),
		ReviewPath:               review.Path,
		SelectionPath:            selection.Path,
		ReviewNodes:              len(review.Preview.Nodes),
		ReviewWarnings:           len(review.Preview.Warnings),
		ModelOutputDigest:        strings.TrimSpace(artifact.ModelOutputDigest),
		ModelGenerationArtifact:  strings.TrimSpace(artifact.ModelGenerationArtifactPath),
		AssistantTextPersisted:   false,
		ExecutedTools:            0,
		WorkerDispatch:           false,
		TargetContact:            false,
		ExecutionDispatchCreated: false,
	}
	acceptance.AcceptanceDigest = modelToolPlanPreviewAcceptanceDigest(acceptance)
	data, err := json.MarshalIndent(acceptance, "", "  ")
	if err != nil {
		return ModelToolPlanPreviewAcceptance{}, fmt.Errorf("encode model tool-plan preview acceptance: %w", err)
	}
	if err := writeAuthoredToolArtifactFile(path, data, "model tool-plan preview acceptance"); err != nil {
		return ModelToolPlanPreviewAcceptance{}, err
	}
	return acceptance, nil
}

func ListModelToolPlanPreviewArtifacts(root string) ([]ModelToolPlanPreviewArtifact, error) {
	dir := modelToolPlanPreviewArtifactDirectory(root)
	if err := rejectUnsafeAuthoredToolDirectory(dir, "model tool-plan preview artifact directory"); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("list model tool-plan preview artifacts: %w", err)
	}
	artifacts := make([]ModelToolPlanPreviewArtifact, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		artifact, err := ReadModelToolPlanPreviewArtifact(root, entry.Name())
		if err != nil {
			return nil, err
		}
		artifacts = append(artifacts, artifact)
	}
	sort.Slice(artifacts, func(i, j int) bool {
		if artifacts[i].CreatedAt == artifacts[j].CreatedAt {
			return artifacts[i].ArtifactID < artifacts[j].ArtifactID
		}
		return artifacts[i].CreatedAt < artifacts[j].CreatedAt
	})
	return artifacts, nil
}

func ReadModelToolPlanPreviewArtifact(root string, artifactPath string) (ModelToolPlanPreviewArtifact, error) {
	path, err := resolveDirectJSONArtifactPath(modelToolPlanPreviewArtifactDirectory(root), artifactPath, "model tool-plan preview artifact")
	if err != nil {
		return ModelToolPlanPreviewArtifact{}, err
	}
	if err := rejectUnsafeAuthoredToolFile(path, "model tool-plan preview artifact"); err != nil {
		return ModelToolPlanPreviewArtifact{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ModelToolPlanPreviewArtifact{}, fmt.Errorf("read model tool-plan preview artifact: %w", err)
	}
	var artifact ModelToolPlanPreviewArtifact
	if err := json.Unmarshal(data, &artifact); err != nil {
		return ModelToolPlanPreviewArtifact{}, fmt.Errorf("decode model tool-plan preview artifact: %w", err)
	}
	if artifact.SchemaVersion != 1 {
		return ModelToolPlanPreviewArtifact{}, fmt.Errorf("unsupported model tool-plan preview artifact schema_version %d", artifact.SchemaVersion)
	}
	artifact.Path = path
	return artifact, nil
}

func ListModelToolPlanPreviewAcceptances(root string) ([]ModelToolPlanPreviewAcceptance, error) {
	dir := modelToolPlanPreviewAcceptanceDirectory(root)
	if err := rejectUnsafeAuthoredToolDirectory(dir, "model tool-plan preview acceptance directory"); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("list model tool-plan preview acceptances: %w", err)
	}
	acceptances := make([]ModelToolPlanPreviewAcceptance, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		acceptance, err := ReadModelToolPlanPreviewAcceptance(root, entry.Name())
		if err != nil {
			return nil, err
		}
		acceptances = append(acceptances, acceptance)
	}
	sort.Slice(acceptances, func(i, j int) bool {
		if acceptances[i].AcceptedAt == acceptances[j].AcceptedAt {
			return acceptances[i].ArtifactID < acceptances[j].ArtifactID
		}
		return acceptances[i].AcceptedAt < acceptances[j].AcceptedAt
	})
	return acceptances, nil
}

func ReadModelToolPlanPreviewAcceptance(root string, acceptancePath string) (ModelToolPlanPreviewAcceptance, error) {
	path, err := resolveDirectJSONArtifactPath(modelToolPlanPreviewAcceptanceDirectory(root), acceptancePath, "model tool-plan preview acceptance")
	if err != nil {
		return ModelToolPlanPreviewAcceptance{}, err
	}
	if err := rejectUnsafeAuthoredToolFile(path, "model tool-plan preview acceptance"); err != nil {
		return ModelToolPlanPreviewAcceptance{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ModelToolPlanPreviewAcceptance{}, fmt.Errorf("read model tool-plan preview acceptance: %w", err)
	}
	var acceptance ModelToolPlanPreviewAcceptance
	if err := json.Unmarshal(data, &acceptance); err != nil {
		return ModelToolPlanPreviewAcceptance{}, fmt.Errorf("decode model tool-plan preview acceptance: %w", err)
	}
	if acceptance.SchemaVersion != 1 {
		return ModelToolPlanPreviewAcceptance{}, fmt.Errorf("unsupported model tool-plan preview acceptance schema_version %d", acceptance.SchemaVersion)
	}
	acceptance.Path = path
	return acceptance, nil
}

func modelToolPlanPreviewArtifactDirectory(root string) string {
	return filepath.Join(root, "runtime", "model-tool-plan-previews")
}

func modelToolPlanPreviewAcceptanceDirectory(root string) string {
	return filepath.Join(root, "runtime", "model-tool-plan-preview-acceptances")
}

func splitModelToolPlanMarker(line string) (string, string, bool) {
	key, value, ok := strings.Cut(line, ":")
	if !ok {
		return "", "", false
	}
	key = strings.ToLower(strings.TrimSpace(key))
	if key != "ouro_action" && key != "semantic_action" {
		return "", "", false
	}
	return key, strings.TrimSpace(value), true
}

func isDeclaredSemanticAction(action compiler.SemanticAction) bool {
	for _, declared := range compiler.DeclaredSemanticActions() {
		if declared == action {
			return true
		}
	}
	return false
}

func modelToolPlanPreviewArtifactID(now time.Time, runID string, providerName string, model string, action compiler.SemanticAction) string {
	timestamp := strings.ReplaceAll(now.UTC().Format("20060102T150405.000000000Z"), ".", "")
	return strings.Join([]string{
		"model-tool-plan-preview",
		timestamp,
		safeProviderUsageIDSegment(runID),
		safeProviderUsageIDSegment(providerName),
		safeProviderUsageIDSegment(model),
		safeProviderUsageIDSegment(string(action)),
	}, "-")
}

func modelToolPlanPreviewArtifactDigest(artifact ModelToolPlanPreviewArtifact) string {
	return providerGenerationTextDigest(strings.Join([]string{
		artifact.ArtifactID,
		artifact.RunID,
		artifact.Source,
		string(artifact.Action),
		artifact.MarkerPreview,
		artifact.ModelOutputDigest,
		artifact.ModelGenerationArtifactPath,
		artifact.ReviewPath,
		fmt.Sprintf("%d", artifact.ReviewNodes),
		fmt.Sprintf("%d", artifact.ReviewWarnings),
	}, "\x00"))
}

func modelToolPlanPreviewAcceptanceID(now time.Time, artifact ModelToolPlanPreviewArtifact) string {
	timestamp := strings.ReplaceAll(now.UTC().Format("20060102T150405.000000000Z"), ".", "")
	return strings.Join([]string{
		"model-tool-plan-preview-acceptance",
		timestamp,
		safeProviderUsageIDSegment(artifact.RunID),
		safeProviderUsageIDSegment(artifact.ProviderName),
		safeProviderUsageIDSegment(artifact.Model),
		safeProviderUsageIDSegment(string(artifact.Action)),
	}, "-")
}

func modelToolPlanPreviewAcceptanceDigest(acceptance ModelToolPlanPreviewAcceptance) string {
	return providerGenerationTextDigest(strings.Join([]string{
		acceptance.ArtifactID,
		acceptance.RunID,
		acceptance.Source,
		string(acceptance.Action),
		acceptance.PreviewArtifactPath,
		acceptance.ReviewPath,
		acceptance.SelectionPath,
		acceptance.AcceptedBy,
		acceptance.AcceptedAt,
		acceptance.Reason,
		fmt.Sprintf("%d", acceptance.ReviewNodes),
		fmt.Sprintf("%d", acceptance.ReviewWarnings),
	}, "\x00"))
}

func resolveDirectJSONArtifactPath(dir string, input string, label string) (string, error) {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return "", fmt.Errorf("%s path is required", label)
	}
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return "", fmt.Errorf("resolve %s directory: %w", label, err)
	}
	if err := requireAuthoredToolDirectory(absDir, label+" directory"); err != nil {
		return "", err
	}
	candidate := trimmed
	if !filepath.IsAbs(candidate) {
		candidate = filepath.Join(absDir, candidate)
	}
	candidate, err = filepath.Abs(candidate)
	if err != nil {
		return "", fmt.Errorf("resolve %s path: %w", label, err)
	}
	rel, err := filepath.Rel(absDir, candidate)
	if err != nil {
		return "", fmt.Errorf("resolve %s relative path: %w", label, err)
	}
	if rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("%s path must stay under %s", label, absDir)
	}
	if strings.Contains(rel, string(os.PathSeparator)) {
		return "", fmt.Errorf("%s path must be a direct json file under %s", label, absDir)
	}
	if filepath.Ext(candidate) != ".json" {
		return "", fmt.Errorf("%s path must be a json file", label)
	}
	return candidate, nil
}
