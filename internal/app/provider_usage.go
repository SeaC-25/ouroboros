package app

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type ProviderUsageRecordOptions struct {
	Now                 time.Time
	MaxEstimatedCostUSD *float64
}

type ProviderUsageRecord struct {
	SchemaVersion             int      `json:"schema_version"`
	Path                      string   `json:"-"`
	RecordID                  string   `json:"record_id"`
	Source                    string   `json:"source"`
	CreatedAt                 string   `json:"created_at"`
	ProviderName              string   `json:"provider_name"`
	Model                     string   `json:"model"`
	InputTokens               int      `json:"input_tokens"`
	OutputTokens              int      `json:"output_tokens"`
	TotalTokens               int      `json:"total_tokens"`
	ContextWindowTokens       int      `json:"context_window_tokens,omitempty"`
	Capabilities              []string `json:"capabilities,omitempty"`
	EstimatedCostUSD          float64  `json:"estimated_cost_usd,omitempty"`
	EstimatedCostAvailable    bool     `json:"estimated_cost_available"`
	BudgetEnforced            bool     `json:"budget_enforced,omitempty"`
	BudgetStatus              string   `json:"budget_status,omitempty"`
	BudgetMaxEstimatedCostUSD float64  `json:"budget_max_estimated_cost_usd,omitempty"`
}

type ProviderGenerationBudgetOptions struct {
	MaxEstimatedCostUSD *float64
}

type ProviderGenerationBudgetDecision struct {
	Enforced            bool
	Status              string
	MaxEstimatedCostUSD float64
	EstimatedCostUSD    float64
}

func RecordProviderGenerationUsage(root string, result ProviderGenerationResult, options ProviderUsageRecordOptions) (ProviderUsageRecord, error) {
	budget, err := EnforceProviderGenerationBudget(result, ProviderGenerationBudgetOptions{
		MaxEstimatedCostUSD: options.MaxEstimatedCostUSD,
	})
	if err != nil {
		return ProviderUsageRecord{}, err
	}
	usageDir := providerUsageDirectory(root)
	if err := ensureProviderUsageDirectory(usageDir); err != nil {
		return ProviderUsageRecord{}, err
	}
	now := options.Now.UTC()
	if now.IsZero() {
		now = time.Now().UTC()
	}
	recordID := providerUsageRecordID(now, result.ProviderName, result.Model)
	recordPath := filepath.Join(usageDir, recordID+".json")
	if err := rejectUnsafeProviderUsageFile(recordPath); err != nil {
		return ProviderUsageRecord{}, err
	}
	record := ProviderUsageRecord{
		SchemaVersion:          1,
		Path:                   recordPath,
		RecordID:               recordID,
		Source:                 "provider-generate",
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
	}
	if budget.Enforced {
		record.BudgetEnforced = true
		record.BudgetStatus = budget.Status
		record.BudgetMaxEstimatedCostUSD = budget.MaxEstimatedCostUSD
	}
	if err := writeProviderUsageRecord(recordPath, record); err != nil {
		return ProviderUsageRecord{}, err
	}
	return record, nil
}

func EnforceProviderGenerationBudget(result ProviderGenerationResult, options ProviderGenerationBudgetOptions) (ProviderGenerationBudgetDecision, error) {
	if options.MaxEstimatedCostUSD == nil {
		return ProviderGenerationBudgetDecision{}, nil
	}
	maxCost := *options.MaxEstimatedCostUSD
	if math.IsNaN(maxCost) || math.IsInf(maxCost, 0) {
		return ProviderGenerationBudgetDecision{}, fmt.Errorf("provider generation max_estimated_cost_usd must be finite")
	}
	if maxCost < 0 {
		return ProviderGenerationBudgetDecision{}, fmt.Errorf("provider generation max_estimated_cost_usd must be non-negative")
	}
	if !result.EstimatedCostAvailable {
		return ProviderGenerationBudgetDecision{}, fmt.Errorf("provider generation budget requires estimated cost metadata")
	}
	if math.IsNaN(result.EstimatedCostUSD) || math.IsInf(result.EstimatedCostUSD, 0) {
		return ProviderGenerationBudgetDecision{}, fmt.Errorf("provider generation estimated cost must be finite")
	}
	if result.EstimatedCostUSD < 0 {
		return ProviderGenerationBudgetDecision{}, fmt.Errorf("provider generation estimated cost must be non-negative")
	}
	if result.EstimatedCostUSD > maxCost {
		return ProviderGenerationBudgetDecision{}, fmt.Errorf("provider generation estimated cost %.6f exceeds budget %.6f", result.EstimatedCostUSD, maxCost)
	}
	return ProviderGenerationBudgetDecision{
		Enforced:            true,
		Status:              "within_limit",
		MaxEstimatedCostUSD: maxCost,
		EstimatedCostUSD:    result.EstimatedCostUSD,
	}, nil
}

func ListProviderUsageRecords(root string) ([]ProviderUsageRecord, error) {
	usageDir := providerUsageDirectory(root)
	if err := requireProviderUsageDirectoryIfPresent(usageDir); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(usageDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("list provider usage records: %w", err)
	}
	records := make([]ProviderUsageRecord, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		record, err := ReadProviderUsageRecord(root, entry.Name())
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	sort.Slice(records, func(i, j int) bool {
		if records[i].CreatedAt == records[j].CreatedAt {
			return records[i].RecordID < records[j].RecordID
		}
		return records[i].CreatedAt < records[j].CreatedAt
	})
	return records, nil
}

func ReadProviderUsageRecord(root string, recordPath string) (ProviderUsageRecord, error) {
	path, err := resolveProviderUsageRecordPath(providerUsageDirectory(root), recordPath)
	if err != nil {
		return ProviderUsageRecord{}, err
	}
	if err := rejectUnsafeProviderUsageFile(path); err != nil {
		return ProviderUsageRecord{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ProviderUsageRecord{}, fmt.Errorf("read provider usage record: %w", err)
	}
	var record ProviderUsageRecord
	if err := json.Unmarshal(data, &record); err != nil {
		return ProviderUsageRecord{}, fmt.Errorf("decode provider usage record: %w", err)
	}
	if record.SchemaVersion != 1 {
		return ProviderUsageRecord{}, fmt.Errorf("unsupported provider usage record schema_version %d", record.SchemaVersion)
	}
	record.Path = path
	return record, nil
}

func providerUsageDirectory(root string) string {
	return filepath.Join(root, "runtime", "model-usage")
}

func ensureProviderUsageDirectory(path string) error {
	if err := rejectUnsafeProviderUsageDirectory(path); err != nil {
		return err
	}
	if err := os.MkdirAll(path, 0o755); err != nil {
		return fmt.Errorf("create provider usage directory: %w", err)
	}
	return requireProviderUsageDirectory(path)
}

func requireProviderUsageDirectoryIfPresent(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("inspect provider usage directory: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("provider usage directory must not be a symlink: %s", path)
	}
	if !info.IsDir() {
		return fmt.Errorf("provider usage path is not a directory: %s", path)
	}
	return nil
}

func requireProviderUsageDirectory(path string) error {
	if err := requireProviderUsageDirectoryIfPresent(path); err != nil {
		return err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("inspect provider usage directory: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("provider usage path is not a directory: %s", path)
	}
	return nil
}

func rejectUnsafeProviderUsageDirectory(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("inspect provider usage directory: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("provider usage directory must not be a symlink: %s", path)
	}
	if !info.IsDir() {
		return fmt.Errorf("provider usage path is not a directory: %s", path)
	}
	return nil
}

func rejectUnsafeProviderUsageFile(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("inspect provider usage record: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("provider usage record must not be a symlink: %s", path)
	}
	if info.IsDir() {
		return fmt.Errorf("provider usage record path is a directory: %s", path)
	}
	return nil
}

func resolveProviderUsageRecordPath(dir string, input string) (string, error) {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return "", fmt.Errorf("provider usage record path is required")
	}
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return "", fmt.Errorf("resolve provider usage directory: %w", err)
	}
	if err := requireProviderUsageDirectory(absDir); err != nil {
		return "", err
	}
	candidate := trimmed
	if !filepath.IsAbs(candidate) {
		candidate = filepath.Join(absDir, candidate)
	}
	candidate, err = filepath.Abs(candidate)
	if err != nil {
		return "", fmt.Errorf("resolve provider usage record path: %w", err)
	}
	rel, err := filepath.Rel(absDir, candidate)
	if err != nil {
		return "", fmt.Errorf("resolve provider usage record relative path: %w", err)
	}
	if rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("provider usage record path must stay under %s", absDir)
	}
	if strings.Contains(rel, string(os.PathSeparator)) {
		return "", fmt.Errorf("provider usage record path must be a direct json file under %s", absDir)
	}
	if filepath.Ext(candidate) != ".json" {
		return "", fmt.Errorf("provider usage record path must be a json file")
	}
	return candidate, nil
}

func writeProviderUsageRecord(path string, record ProviderUsageRecord) error {
	dir := filepath.Dir(path)
	temp, err := os.CreateTemp(dir, ".provider-usage-*.json")
	if err != nil {
		return fmt.Errorf("create provider usage temp record: %w", err)
	}
	tempName := temp.Name()
	defer func() {
		_ = os.Remove(tempName)
	}()
	encoder := json.NewEncoder(temp)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(record); err != nil {
		_ = temp.Close()
		return fmt.Errorf("encode provider usage record: %w", err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("close provider usage temp record: %w", err)
	}
	if err := os.Rename(tempName, path); err != nil {
		return fmt.Errorf("write provider usage record: %w", err)
	}
	return nil
}

func providerUsageRecordID(now time.Time, providerName, model string) string {
	timestamp := strings.ReplaceAll(now.UTC().Format("20060102T150405.000000000Z"), ".", "")
	return strings.Join([]string{
		"provider-generate",
		timestamp,
		safeProviderUsageIDSegment(providerName),
		safeProviderUsageIDSegment(model),
	}, "-")
}

func safeProviderUsageIDSegment(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var builder strings.Builder
	lastDash := false
	for _, r := range value {
		allowed := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
		if allowed {
			builder.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			builder.WriteByte('-')
			lastDash = true
		}
	}
	segment := strings.Trim(builder.String(), "-")
	if segment == "" {
		return "unknown"
	}
	return segment
}
