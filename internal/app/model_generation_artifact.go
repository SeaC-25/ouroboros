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

type ModelGenerationArtifact struct {
	SchemaVersion             int                       `json:"schema_version"`
	Path                      string                    `json:"-"`
	ArtifactID                string                    `json:"artifact_id"`
	RunID                     string                    `json:"run_id"`
	Source                    string                    `json:"source"`
	AgentID                   string                    `json:"agent_id"`
	Role                      string                    `json:"role"`
	Status                    string                    `json:"status"`
	ExecutionMode             string                    `json:"execution_mode"`
	CreatedAt                 string                    `json:"created_at"`
	ProviderName              string                    `json:"provider_name"`
	Model                     string                    `json:"model"`
	InputTokens               int                       `json:"input_tokens"`
	OutputTokens              int                       `json:"output_tokens"`
	TotalTokens               int                       `json:"total_tokens"`
	ContextWindowTokens       int                       `json:"context_window_tokens,omitempty"`
	Capabilities              []string                  `json:"capabilities,omitempty"`
	EstimatedCostUSD          float64                   `json:"estimated_cost_usd,omitempty"`
	EstimatedCostAvailable    bool                      `json:"estimated_cost_available"`
	BudgetEnforced            bool                      `json:"budget_enforced,omitempty"`
	BudgetStatus              string                    `json:"budget_status"`
	BudgetMaxEstimatedCostUSD float64                   `json:"budget_max_estimated_cost_usd,omitempty"`
	LedgerWrite               bool                      `json:"ledger_write"`
	AssistantTextPersisted    bool                      `json:"assistant_text_persisted"`
	TextDigest                string                    `json:"text_digest"`
	TextPreview               string                    `json:"text_preview"`
	LatencyMS                 int64                     `json:"latency_ms,omitempty"`
	RoutingStrategy           string                    `json:"routing_strategy,omitempty"`
	ProviderAttemptCount      int                       `json:"provider_attempt_count,omitempty"`
	ProviderFailover          bool                      `json:"provider_failover,omitempty"`
	SelectedProviderName      string                    `json:"selected_provider_name,omitempty"`
	FailedProviderNames       []string                  `json:"failed_provider_names,omitempty"`
	RequiredCapabilities      []string                  `json:"required_capabilities,omitempty"`
	CapabilityMatch           bool                      `json:"capability_match,omitempty"`
	SkippedProviders          []ProviderSkippedProvider `json:"skipped_providers,omitempty"`
	Streaming                 bool                      `json:"streaming,omitempty"`
	StreamChunkCount          int                       `json:"stream_chunk_count,omitempty"`
	StreamCompleted           bool                      `json:"stream_completed,omitempty"`
	StreamDigest              string                    `json:"stream_digest,omitempty"`
	StreamPreview             string                    `json:"stream_preview,omitempty"`
}

type ModelGenerationArtifactOptions struct {
	Now time.Time
}

func RecordModelGenerationArtifact(root string, runID string, result ProviderGenerationResult, budget ProviderGenerationBudgetDecision, ledgerWrite bool, options ModelGenerationArtifactOptions) (ModelGenerationArtifact, error) {
	artifactDir := modelGenerationArtifactDirectory(root)
	if err := ensureModelGenerationArtifactDirectory(artifactDir); err != nil {
		return ModelGenerationArtifact{}, err
	}
	now := options.Now.UTC()
	if now.IsZero() {
		now = time.Now().UTC()
	}
	budgetStatus := "not_enforced"
	if budget.Enforced {
		budgetStatus = budget.Status
	}
	artifactID := modelGenerationArtifactID(now, runID, result.ProviderName, result.Model)
	artifactPath := filepath.Join(artifactDir, artifactID+".json")
	if err := rejectUnsafeModelGenerationArtifactFile(artifactPath); err != nil {
		return ModelGenerationArtifact{}, err
	}
	artifact := ModelGenerationArtifact{
		SchemaVersion:          1,
		Path:                   artifactPath,
		ArtifactID:             artifactID,
		RunID:                  strings.TrimSpace(runID),
		Source:                 closedLoopPlannerGenerationStageID,
		AgentID:                "closed-loop-planner",
		Role:                   "planner",
		Status:                 "completed",
		ExecutionMode:          "provider-generation",
		CreatedAt:              now.Format(time.RFC3339),
		ProviderName:           strings.TrimSpace(result.ProviderName),
		Model:                  strings.TrimSpace(result.Model),
		InputTokens:            result.InputTokens,
		OutputTokens:           result.OutputTokens,
		TotalTokens:            result.TotalTokens,
		ContextWindowTokens:    result.ContextWindowTokens,
		Capabilities:           append([]string{}, result.Capabilities...),
		EstimatedCostUSD:       result.EstimatedCostUSD,
		EstimatedCostAvailable: result.EstimatedCostAvailable,
		BudgetStatus:           budgetStatus,
		LedgerWrite:            ledgerWrite,
		AssistantTextPersisted: false,
		TextDigest:             providerGenerationTextDigest(result.Text),
		TextPreview:            providerGenerationTextPreview(result.Text),
		LatencyMS:              int64(result.Latency / time.Millisecond),
		RoutingStrategy:        strings.TrimSpace(result.RoutingStrategy),
		ProviderAttemptCount:   result.ProviderAttemptCount,
		ProviderFailover:       result.ProviderFailover,
		SelectedProviderName:   strings.TrimSpace(result.SelectedProviderName),
		FailedProviderNames:    append([]string{}, result.FailedProviderNames...),
		RequiredCapabilities:   append([]string{}, result.RequiredCapabilities...),
		CapabilityMatch:        result.CapabilityMatch,
		SkippedProviders:       appendProviderSkippedProviders(nil, result.SkippedProviders...),
		Streaming:              result.Streaming,
		StreamChunkCount:       result.StreamChunkCount,
		StreamCompleted:        result.StreamCompleted,
		StreamDigest:           strings.TrimSpace(result.StreamDigest),
		StreamPreview:          providerGenerationTextPreview(result.StreamPreview),
	}
	if budget.Enforced {
		artifact.BudgetEnforced = true
		artifact.BudgetMaxEstimatedCostUSD = budget.MaxEstimatedCostUSD
	}
	if err := writeModelGenerationArtifact(artifactPath, artifact); err != nil {
		return ModelGenerationArtifact{}, err
	}
	return artifact, nil
}

func ListModelGenerationArtifacts(root string) ([]ModelGenerationArtifact, error) {
	artifactDir := modelGenerationArtifactDirectory(root)
	if err := requireModelGenerationArtifactDirectoryIfPresent(artifactDir); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(artifactDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("list model generation artifacts: %w", err)
	}
	artifacts := make([]ModelGenerationArtifact, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		artifact, err := ReadModelGenerationArtifact(root, entry.Name())
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

func ReadModelGenerationArtifact(root string, artifactPath string) (ModelGenerationArtifact, error) {
	path, err := resolveModelGenerationArtifactPath(modelGenerationArtifactDirectory(root), artifactPath)
	if err != nil {
		return ModelGenerationArtifact{}, err
	}
	if err := rejectUnsafeModelGenerationArtifactFile(path); err != nil {
		return ModelGenerationArtifact{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ModelGenerationArtifact{}, fmt.Errorf("read model generation artifact: %w", err)
	}
	var artifact ModelGenerationArtifact
	if err := json.Unmarshal(data, &artifact); err != nil {
		return ModelGenerationArtifact{}, fmt.Errorf("decode model generation artifact: %w", err)
	}
	if artifact.SchemaVersion != 1 {
		return ModelGenerationArtifact{}, fmt.Errorf("unsupported model generation artifact schema_version %d", artifact.SchemaVersion)
	}
	artifact.Path = path
	return artifact, nil
}

func modelGenerationArtifactDirectory(root string) string {
	return filepath.Join(root, "runtime", "model-generations")
}

func ensureModelGenerationArtifactDirectory(path string) error {
	if err := rejectUnsafeModelGenerationArtifactDirectory(path); err != nil {
		return err
	}
	if err := os.MkdirAll(path, 0o755); err != nil {
		return fmt.Errorf("create model generation artifact directory: %w", err)
	}
	return requireModelGenerationArtifactDirectory(path)
}

func requireModelGenerationArtifactDirectoryIfPresent(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("inspect model generation artifact directory: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("model generation artifact directory must not be a symlink: %s", path)
	}
	if !info.IsDir() {
		return fmt.Errorf("model generation artifact path is not a directory: %s", path)
	}
	return nil
}

func requireModelGenerationArtifactDirectory(path string) error {
	if err := requireModelGenerationArtifactDirectoryIfPresent(path); err != nil {
		return err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("inspect model generation artifact directory: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("model generation artifact path is not a directory: %s", path)
	}
	return nil
}

func rejectUnsafeModelGenerationArtifactDirectory(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("inspect model generation artifact directory: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("model generation artifact directory must not be a symlink: %s", path)
	}
	if !info.IsDir() {
		return fmt.Errorf("model generation artifact path is not a directory: %s", path)
	}
	return nil
}

func rejectUnsafeModelGenerationArtifactFile(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("inspect model generation artifact: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("model generation artifact must not be a symlink: %s", path)
	}
	if info.IsDir() {
		return fmt.Errorf("model generation artifact path is a directory: %s", path)
	}
	return nil
}

func resolveModelGenerationArtifactPath(dir string, input string) (string, error) {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return "", fmt.Errorf("model generation artifact path is required")
	}
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return "", fmt.Errorf("resolve model generation artifact directory: %w", err)
	}
	if err := requireModelGenerationArtifactDirectory(absDir); err != nil {
		return "", err
	}
	candidate := trimmed
	if !filepath.IsAbs(candidate) {
		candidate = filepath.Join(absDir, candidate)
	}
	candidate, err = filepath.Abs(candidate)
	if err != nil {
		return "", fmt.Errorf("resolve model generation artifact path: %w", err)
	}
	rel, err := filepath.Rel(absDir, candidate)
	if err != nil {
		return "", fmt.Errorf("resolve model generation artifact relative path: %w", err)
	}
	if rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("model generation artifact path must stay under %s", absDir)
	}
	if strings.Contains(rel, string(os.PathSeparator)) {
		return "", fmt.Errorf("model generation artifact path must be a direct json file under %s", absDir)
	}
	if filepath.Ext(candidate) != ".json" {
		return "", fmt.Errorf("model generation artifact path must be a json file")
	}
	return candidate, nil
}

func writeModelGenerationArtifact(path string, artifact ModelGenerationArtifact) error {
	dir := filepath.Dir(path)
	temp, err := os.CreateTemp(dir, ".model-generation-*.json")
	if err != nil {
		return fmt.Errorf("create model generation temp artifact: %w", err)
	}
	tempName := temp.Name()
	defer func() {
		_ = os.Remove(tempName)
	}()
	encoder := json.NewEncoder(temp)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(artifact); err != nil {
		_ = temp.Close()
		return fmt.Errorf("encode model generation artifact: %w", err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("close model generation temp artifact: %w", err)
	}
	if err := os.Rename(tempName, path); err != nil {
		return fmt.Errorf("write model generation artifact: %w", err)
	}
	return nil
}

func modelGenerationArtifactID(now time.Time, runID string, providerName string, model string) string {
	timestamp := strings.ReplaceAll(now.UTC().Format("20060102T150405.000000000Z"), ".", "")
	return strings.Join([]string{
		"model-generation",
		timestamp,
		safeProviderUsageIDSegment(runID),
		safeProviderUsageIDSegment(providerName),
		safeProviderUsageIDSegment(model),
	}, "-")
}
