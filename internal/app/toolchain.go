package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/netip"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/ouroboros/ouroboros/internal/compiler"
	"github.com/ouroboros/ouroboros/internal/contracts"
	"github.com/ouroboros/ouroboros/internal/eventlog"
	"github.com/ouroboros/ouroboros/internal/evidence"
	"github.com/ouroboros/ouroboros/internal/frontier"
	"github.com/ouroboros/ouroboros/internal/toolchain"
	"gopkg.in/yaml.v3"
)

type ToolchainStatus struct {
	Name            string
	Version         string
	Runner          string
	Parser          string
	CapabilityCount int
	Health          toolchain.HealthReport
	Dependencies    []toolchain.DependencyReport
	InstallPlan     []toolchain.DependencyInstallStep
}

type ToolchainContractCheck struct {
	Status              string
	Mode                string
	ManifestCount       int
	ToolCount           int
	DomainPackCount     int
	PatternCount        int
	SemanticActionCount int
	Errors              []string
}

type AuthoredToolArtifact struct {
	Status       string   `json:"status"`
	Name         string   `json:"name"`
	Version      string   `json:"version"`
	Parser       string   `json:"parser"`
	AdapterID    string   `json:"adapter_id"`
	Operation    string   `json:"operation"`
	Capabilities []string `json:"capabilities"`
	Dir          string   `json:"dir"`
	SpecPath     string   `json:"spec_path"`
	ManifestPath string   `json:"manifest_path"`
}

type ManagedToolBundle struct {
	Status       string `json:"status"`
	Name         string `json:"name"`
	Kind         string `json:"kind"`
	Version      string `json:"version"`
	Source       string `json:"source"`
	SHA256       string `json:"sha256"`
	BundleDir    string `json:"bundle_dir"`
	ManifestPath string `json:"manifest_path"`
	PayloadPath  string `json:"payload_path"`
	InstallPath  string `json:"install_path"`
}

type ManagedToolBundleInstallPlan struct {
	Status      string `json:"status"`
	Mode        string `json:"mode"`
	Action      string `json:"action"`
	Pack        string `json:"pack"`
	Dependency  string `json:"dependency"`
	Kind        string `json:"kind"`
	Version     string `json:"version"`
	Source      string `json:"source"`
	SHA256      string `json:"sha256"`
	PayloadPath string `json:"payload_path"`
	InstallPath string `json:"install_path"`
}

type ManagedToolBundleInstallOptions struct {
	Execute bool
}

type ManagedToolBundleInstallResult struct {
	Status      string `json:"status"`
	Mode        string `json:"mode"`
	Action      string `json:"action"`
	Pack        string `json:"pack"`
	Dependency  string `json:"dependency"`
	Kind        string `json:"kind"`
	Version     string `json:"version"`
	Source      string `json:"source"`
	SHA256      string `json:"sha256"`
	PayloadPath string `json:"payload_path"`
	InstallPath string `json:"install_path"`
}

type ReleaseManagedToolBundleInstallPlan struct {
	Status               string `json:"status"`
	Mode                 string `json:"mode"`
	Action               string `json:"action"`
	Pack                 string `json:"pack"`
	Dependency           string `json:"dependency"`
	Kind                 string `json:"kind"`
	Version              string `json:"version"`
	Source               string `json:"source"`
	SHA256               string `json:"sha256"`
	ReleaseBundleDir     string `json:"release_bundle_dir"`
	ReleasePayloadPath   string `json:"release_payload_path"`
	WorkspaceBundleDir   string `json:"workspace_bundle_dir"`
	WorkspacePayloadPath string `json:"workspace_payload_path"`
	InstallPath          string `json:"install_path"`
}

type ReleaseManagedToolBundleInstallResult struct {
	Status               string `json:"status"`
	Mode                 string `json:"mode"`
	Action               string `json:"action"`
	Pack                 string `json:"pack"`
	Dependency           string `json:"dependency"`
	Kind                 string `json:"kind"`
	Version              string `json:"version"`
	Source               string `json:"source"`
	SHA256               string `json:"sha256"`
	ReleaseBundleDir     string `json:"release_bundle_dir"`
	ReleasePayloadPath   string `json:"release_payload_path"`
	WorkspaceBundleDir   string `json:"workspace_bundle_dir"`
	WorkspacePayloadPath string `json:"workspace_payload_path"`
	InstallPath          string `json:"install_path"`
}

type ToolCompilePreviewOptions struct {
	Action           compiler.SemanticAction
	Budget           compiler.CompileBudget
	DomainPriorities []compiler.DomainPriority
	FrontierScores   []compiler.CandidateFrontierScore
	HistoricalStats  []compiler.CandidateHistory
}

type ToolCompilePreview struct {
	Workspace string                  `json:"workspace"`
	Action    compiler.SemanticAction `json:"action"`
	Mode      string                  `json:"mode"`
	Nodes     []compiler.PlanNode     `json:"nodes"`
	Warnings  []string                `json:"warnings,omitempty"`
}

type ToolCompilePreviewReview struct {
	Status     string                              `json:"status"`
	Mode       string                              `json:"mode"`
	Path       string                              `json:"path,omitempty"`
	Preview    ToolCompilePreview                  `json:"preview"`
	Annotation *ToolCompilePreviewReviewAnnotation `json:"annotation,omitempty"`
}

type ToolCompilePreviewReviewAnnotation struct {
	Decision   string `json:"decision"`
	Reason     string `json:"reason,omitempty"`
	Reviewer   string `json:"reviewer,omitempty"`
	ReviewedAt string `json:"reviewed_at,omitempty"`
}

type ToolCompilePreviewReviewAnnotationOptions struct {
	Decision   string
	Reason     string
	Reviewer   string
	ReviewedAt time.Time
}

type ToolCompilePreviewReviewComparison struct {
	Status       string                                  `json:"status"`
	Mode         string                                  `json:"mode"`
	Base         ToolCompilePreviewReviewComparisonSide  `json:"base"`
	Candidate    ToolCompilePreviewReviewComparisonSide  `json:"candidate"`
	Summary      ToolCompilePreviewReviewComparisonStats `json:"summary"`
	NodeDiffs    []ToolCompilePreviewReviewNodeDiff      `json:"node_diffs,omitempty"`
	WarningDiffs []ToolCompilePreviewReviewWarningDiff   `json:"warning_diffs,omitempty"`
}

type ToolCompilePreviewReviewComparisonSide struct {
	Path     string                  `json:"path"`
	Action   compiler.SemanticAction `json:"action"`
	Nodes    int                     `json:"nodes"`
	Warnings int                     `json:"warnings"`
	Decision string                  `json:"decision,omitempty"`
}

type ToolCompilePreviewReviewComparisonStats struct {
	NodeDelta          int `json:"node_delta"`
	WarningDelta       int `json:"warning_delta"`
	AddedNodes         int `json:"added_nodes"`
	RemovedNodes       int `json:"removed_nodes"`
	ChangedNodes       int `json:"changed_nodes"`
	ReorderedNodes     int `json:"reordered_nodes"`
	ScoreChangedNodes  int `json:"score_changed_nodes"`
	SignalChangedNodes int `json:"signal_changed_nodes"`
}

type ToolCompilePreviewReviewNodeDiff struct {
	Change           string   `json:"change"`
	ManifestName     string   `json:"manifest_name"`
	ToolID           string   `json:"tool_id,omitempty"`
	AdapterID        string   `json:"adapter_id,omitempty"`
	Operation        string   `json:"operation,omitempty"`
	ParserID         string   `json:"parser_id,omitempty"`
	TargetKind       string   `json:"target_kind,omitempty"`
	DependencyName   string   `json:"dependency_name,omitempty"`
	DependencyKind   string   `json:"dependency_kind,omitempty"`
	Capability       string   `json:"capability"`
	BaseID           string   `json:"base_id,omitempty"`
	CandidateID      string   `json:"candidate_id,omitempty"`
	BaseIndex        int      `json:"base_index"`
	CandidateIndex   int      `json:"candidate_index"`
	BaseScore        float64  `json:"base_score,omitempty"`
	CandidateScore   float64  `json:"candidate_score,omitempty"`
	BaseSignals      []string `json:"base_signals,omitempty"`
	CandidateSignals []string `json:"candidate_signals,omitempty"`
}

type ToolCompilePreviewReviewWarningDiff struct {
	Change  string `json:"change"`
	Warning string `json:"warning"`
}

type ToolCompilePreviewReviewSelection struct {
	Path            string                  `json:"path,omitempty"`
	Status          string                  `json:"status"`
	Mode            string                  `json:"mode"`
	ReviewPath      string                  `json:"review_path"`
	Action          compiler.SemanticAction `json:"action"`
	ReviewNodes     int                     `json:"review_nodes"`
	ReviewWarnings  int                     `json:"review_warnings"`
	SupersededPaths []string                `json:"superseded_paths,omitempty"`
	SelectedBy      string                  `json:"selected_by,omitempty"`
	SelectedAt      string                  `json:"selected_at,omitempty"`
	ClearedBy       string                  `json:"cleared_by,omitempty"`
	ClearedAt       string                  `json:"cleared_at,omitempty"`
	Reason          string                  `json:"reason,omitempty"`
}

type ToolCompilePreviewReviewSelectionOptions struct {
	SelectedBy string
	SelectedAt time.Time
	Reason     string
}

type ToolCompilePreviewReviewSelectionClearOptions struct {
	ClearedBy string
	ClearedAt time.Time
	Reason    string
}

type ToolCompilePreviewSignalArtifact struct {
	Path    string                    `json:"path"`
	Status  string                    `json:"status"`
	Mode    string                    `json:"mode"`
	Signals ToolCompilePreviewSignals `json:"signals"`
}

type ToolCompilePreviewSignalDeriveOptions struct {
	Write        bool
	MaxPlanNodes int
}

type ToolCompilePreviewSignalDerivation struct {
	Path    string                                    `json:"path"`
	Status  string                                    `json:"status"`
	Mode    string                                    `json:"mode"`
	RunID   string                                    `json:"run_id"`
	Signals ToolCompilePreviewSignals                 `json:"signals"`
	Summary ToolCompilePreviewSignalDerivationSummary `json:"summary"`
}

type ToolCompilePreviewSignalDerivationSummary struct {
	ToolExecutions int `json:"tool_executions"`
	Successes      int `json:"successes"`
	Failures       int `json:"failures"`
	Ignored        int `json:"ignored"`
	FrontierScores int `json:"frontier_scores"`
}

type ToolCompilePreviewSignals struct {
	SchemaVersion    int                                      `json:"schema_version,omitempty"`
	Budget           ToolCompilePreviewSignalBudget           `json:"budget,omitempty"`
	DomainPriorities []ToolCompilePreviewSignalDomainPriority `json:"domain_priorities,omitempty"`
	FrontierScores   []ToolCompilePreviewSignalFrontierScore  `json:"frontier_scores,omitempty"`
	HistoricalStats  []ToolCompilePreviewSignalHistory        `json:"historical_stats,omitempty"`
}

type ToolCompilePreviewSignalBudget struct {
	MaxPlanNodes int `json:"max_plan_nodes,omitempty"`
}

type ToolCompilePreviewSignalDomainPriority struct {
	Domain string  `json:"domain"`
	Weight float64 `json:"weight"`
}

type ToolCompilePreviewSignalFrontierScore struct {
	ManifestName string                       `json:"manifest_name"`
	Capability   string                       `json:"capability"`
	Path         ToolCompilePreviewSignalPath `json:"path"`
}

type ToolCompilePreviewSignalPath struct {
	ID                 string  `json:"id,omitempty"`
	Target             string  `json:"target,omitempty"`
	Hypothesis         string  `json:"hypothesis,omitempty"`
	ExpectedUnlock     string  `json:"expected_unlock,omitempty"`
	MissionValue       float64 `json:"mission_value,omitempty"`
	UnlockPotential    float64 `json:"unlock_potential,omitempty"`
	EvidenceConfidence float64 `json:"evidence_confidence,omitempty"`
	Novelty            float64 `json:"novelty,omitempty"`
	Cost               float64 `json:"cost,omitempty"`
	DuplicateRisk      float64 `json:"duplicate_risk,omitempty"`
	Noise              float64 `json:"noise,omitempty"`
	ExhaustedPenalty   float64 `json:"exhausted_penalty,omitempty"`
}

type ToolCompilePreviewSignalHistory struct {
	ManifestName string `json:"manifest_name"`
	Capability   string `json:"capability"`
	Successes    int    `json:"successes,omitempty"`
	Failures     int    `json:"failures,omitempty"`
}

type ToolchainInstallRequest struct {
	Status      string   `json:"status"`
	Pack        string   `json:"pack"`
	Dependency  string   `json:"dependency"`
	Kind        string   `json:"kind"`
	State       string   `json:"state"`
	Required    bool     `json:"required"`
	Command     []string `json:"command"`
	Hint        string   `json:"hint,omitempty"`
	Source      string   `json:"source,omitempty"`
	ImageDigest string   `json:"image_digest,omitempty"`
	DependsOn   []string `json:"depends_on,omitempty"`
	Path        string   `json:"path,omitempty"`
}

type ToolchainInstallRequestSummary struct {
	Total    int `json:"total"`
	Pending  int `json:"pending"`
	Approved int `json:"approved"`
	Denied   int `json:"denied"`
}

type ToolchainInstallDispatchPlan struct {
	Summary ToolchainInstallRequestSummary `json:"summary"`
	Ready   []ToolchainInstallRequest      `json:"ready"`
	Skipped []ToolchainInstallDispatchSkip `json:"skipped"`
}

type ToolchainInstallDispatchSkip struct {
	Request ToolchainInstallRequest `json:"request"`
	Reason  string                  `json:"reason"`
}

type ToolchainInstallDispatch struct {
	Status  string                         `json:"status"`
	Mode    string                         `json:"mode"`
	Path    string                         `json:"path,omitempty"`
	Summary ToolchainInstallRequestSummary `json:"summary"`
	Ready   []ToolchainInstallRequest      `json:"ready"`
	Skipped []ToolchainInstallDispatchSkip `json:"skipped"`
}

type ToolchainInstallDispatchExecutionOptions struct {
	Execute bool
}

type ToolchainInstallDispatchExecution struct {
	Status       string                                    `json:"status"`
	Mode         string                                    `json:"mode"`
	Path         string                                    `json:"path,omitempty"`
	DispatchPath string                                    `json:"dispatch_path"`
	Warnings     []string                                  `json:"warnings,omitempty"`
	Results      []ToolchainInstallDispatchExecutionResult `json:"results"`
	Skipped      []ToolchainInstallDispatchSkip            `json:"skipped"`
}

type ToolchainInstallDispatchExecutionResult struct {
	RequestPath string   `json:"request_path,omitempty"`
	Pack        string   `json:"pack"`
	Dependency  string   `json:"dependency"`
	Command     []string `json:"command"`
	Status      string   `json:"status"`
	Output      string   `json:"output,omitempty"`
	Error       string   `json:"error,omitempty"`
}

type ToolchainInstallRetryPlan struct {
	Summary ToolchainInstallRetrySummary     `json:"summary"`
	Ready   []ToolchainInstallRetryCandidate `json:"ready"`
	Skipped []ToolchainInstallRetrySkip      `json:"skipped"`
}

type ToolchainInstallRetrySummary struct {
	Total   int `json:"total"`
	Ready   int `json:"ready"`
	Skipped int `json:"skipped"`
}

type ToolchainInstallRetryCandidate struct {
	ExecutionPath string   `json:"execution_path"`
	DispatchPath  string   `json:"dispatch_path"`
	RequestPath   string   `json:"request_path,omitempty"`
	Pack          string   `json:"pack"`
	Dependency    string   `json:"dependency"`
	Command       []string `json:"command"`
	LastStatus    string   `json:"last_status"`
	LastOutput    string   `json:"last_output,omitempty"`
	LastError     string   `json:"last_error,omitempty"`
}

type ToolchainInstallRetrySkip struct {
	ExecutionPath string `json:"execution_path"`
	DispatchPath  string `json:"dispatch_path"`
	RequestPath   string `json:"request_path,omitempty"`
	Pack          string `json:"pack"`
	Dependency    string `json:"dependency"`
	Status        string `json:"status"`
	Reason        string `json:"reason"`
}

type ToolchainInstallRetryDispatch struct {
	Status  string                           `json:"status"`
	Mode    string                           `json:"mode"`
	Path    string                           `json:"path,omitempty"`
	Summary ToolchainInstallRetrySummary     `json:"summary"`
	Ready   []ToolchainInstallRetryCandidate `json:"ready"`
	Skipped []ToolchainInstallRetrySkip      `json:"skipped"`
}

type ToolchainInstallRetryDispatchExecutionOptions struct {
	Execute bool
}

type ToolchainInstallRetryDispatchExecution struct {
	Status            string                                `json:"status"`
	Mode              string                                `json:"mode"`
	Path              string                                `json:"path,omitempty"`
	RetryDispatchPath string                                `json:"retry_dispatch_path"`
	Warnings          []string                              `json:"warnings,omitempty"`
	Results           []ToolchainInstallRetryDispatchResult `json:"results"`
	Skipped           []ToolchainInstallRetrySkip           `json:"skipped"`
}

type ToolchainInstallRetryDispatchResult struct {
	ExecutionPath string   `json:"execution_path"`
	DispatchPath  string   `json:"dispatch_path"`
	RequestPath   string   `json:"request_path,omitempty"`
	Pack          string   `json:"pack"`
	Dependency    string   `json:"dependency"`
	Command       []string `json:"command"`
	LastStatus    string   `json:"last_status"`
	LastOutput    string   `json:"last_output,omitempty"`
	LastError     string   `json:"last_error,omitempty"`
	Status        string   `json:"status"`
	Output        string   `json:"output,omitempty"`
	Error         string   `json:"error,omitempty"`
}

func SummarizeToolchainInstallRequests(requests []ToolchainInstallRequest) ToolchainInstallRequestSummary {
	summary := ToolchainInstallRequestSummary{Total: len(requests)}
	for _, request := range requests {
		switch strings.ToLower(strings.TrimSpace(request.Status)) {
		case "pending":
			summary.Pending++
		case "approved":
			summary.Approved++
		case "denied":
			summary.Denied++
		}
	}
	return summary
}

func (s ToolchainInstallRequestSummary) Line() string {
	return fmt.Sprintf("total=%d pending=%d approved=%d denied=%d", s.Total, s.Pending, s.Approved, s.Denied)
}

func (s ToolchainInstallRequestSummary) Compact() string {
	return fmt.Sprintf("p%d/a%d/d%d", s.Pending, s.Approved, s.Denied)
}

func LoadToolCompilePreviewSignals(root string) (ToolCompilePreviewSignals, bool, error) {
	path := toolCompilePreviewSignalsPath(root)
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return ToolCompilePreviewSignals{}, false, nil
	}
	if err != nil {
		return ToolCompilePreviewSignals{}, false, fmt.Errorf("read compile preview signals: %w", err)
	}
	var signals ToolCompilePreviewSignals
	if err := json.Unmarshal(data, &signals); err != nil {
		return ToolCompilePreviewSignals{}, false, fmt.Errorf("decode compile preview signals: %w", err)
	}
	if signals.SchemaVersion != 0 && signals.SchemaVersion != 1 {
		return ToolCompilePreviewSignals{}, false, fmt.Errorf("unsupported compile preview signals schema_version: %d", signals.SchemaVersion)
	}
	return signals, true, nil
}

func WriteToolCompilePreviewSignals(root string, signals ToolCompilePreviewSignals) (ToolCompilePreviewSignalArtifact, error) {
	normalized, err := normalizeToolCompilePreviewSignals(signals)
	if err != nil {
		return ToolCompilePreviewSignalArtifact{}, err
	}
	if _, err := toolCompilePreviewOptionsFromSignals(normalized); err != nil {
		return ToolCompilePreviewSignalArtifact{}, err
	}
	dir := filepath.Join(root, "toolchain")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return ToolCompilePreviewSignalArtifact{}, fmt.Errorf("create compile preview signals directory: %w", err)
	}
	path := toolCompilePreviewSignalsPath(root)
	data, err := json.MarshalIndent(normalized, "", "  ")
	if err != nil {
		return ToolCompilePreviewSignalArtifact{}, fmt.Errorf("encode compile preview signals: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return ToolCompilePreviewSignalArtifact{}, fmt.Errorf("write compile preview signals: %w", err)
	}
	return ToolCompilePreviewSignalArtifact{
		Path:    path,
		Status:  "written",
		Mode:    "metadata-only",
		Signals: normalized,
	}, nil
}

func DeriveToolCompilePreviewSignalsFromReplay(ctx context.Context, root, runID string, options ToolCompilePreviewSignalDeriveOptions) (ToolCompilePreviewSignalDerivation, error) {
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return ToolCompilePreviewSignalDerivation{}, fmt.Errorf("run id is required")
	}
	if options.MaxPlanNodes < 0 {
		return ToolCompilePreviewSignalDerivation{}, fmt.Errorf("max_plan_nodes must be non-negative")
	}
	eventDBPath := filepath.Join(root, "replay", "events.db")
	if _, err := os.Stat(eventDBPath); err != nil {
		if os.IsNotExist(err) {
			return ToolCompilePreviewSignalDerivation{}, fmt.Errorf("replay event log not found: %s", eventDBPath)
		}
		return ToolCompilePreviewSignalDerivation{}, fmt.Errorf("stat replay event log: %w", err)
	}
	store, err := eventlog.OpenSQLite(eventDBPath)
	if err != nil {
		return ToolCompilePreviewSignalDerivation{}, err
	}
	defer store.Close()

	events, err := store.Replay(ctx, runID)
	if err != nil {
		return ToolCompilePreviewSignalDerivation{}, err
	}
	if len(events) == 0 {
		return ToolCompilePreviewSignalDerivation{}, fmt.Errorf("run %s has no replay events", runID)
	}
	manifests, err := toolchain.LoadManifests(filepath.Join(root, "toolchain"))
	if err != nil {
		return ToolCompilePreviewSignalDerivation{}, fmt.Errorf("load toolchain manifests: %w", err)
	}
	signals, summary, err := toolCompilePreviewSignalsFromReplayEvents(events, manifests, options)
	if err != nil {
		return ToolCompilePreviewSignalDerivation{}, err
	}
	derivation := ToolCompilePreviewSignalDerivation{
		Path:    toolCompilePreviewSignalsPath(root),
		Status:  "dry-run",
		Mode:    "metadata-only",
		RunID:   runID,
		Signals: signals,
		Summary: summary,
	}
	if options.Write {
		artifact, err := WriteToolCompilePreviewSignals(root, signals)
		if err != nil {
			return ToolCompilePreviewSignalDerivation{}, err
		}
		derivation.Path = artifact.Path
		derivation.Status = artifact.Status
		derivation.Signals = artifact.Signals
	}
	return derivation, nil
}

func PreviewToolCompilePlan(root string, options ToolCompilePreviewOptions) (ToolCompilePreview, error) {
	persistedSignals, found, err := LoadToolCompilePreviewSignals(root)
	if err != nil {
		return ToolCompilePreview{}, err
	}
	if found {
		merged, err := mergeToolCompilePreviewOptions(options, persistedSignals)
		if err != nil {
			return ToolCompilePreview{}, err
		}
		options = merged
	}
	action := options.Action
	if strings.TrimSpace(string(action)) == "" {
		action = compiler.ActionDiscoverAssets
	}
	manifests, err := toolchain.LoadManifests(filepath.Join(root, "toolchain"))
	if err != nil {
		return ToolCompilePreview{}, fmt.Errorf("load toolchain manifests: %w", err)
	}
	plan, err := compiler.NewCompiler().Compile(compiler.CompileRequest{
		Action: action,
		Evidence: compiler.EvidenceSlice{
			Nodes: []evidence.Node{
				{
					ID:         "fact-workspace",
					Type:       evidence.NodeTarget,
					Label:      filepath.Base(root),
					SourceKind: "workspace",
				},
			},
		},
		Manifests:        manifests,
		Budget:           options.Budget,
		DomainPriorities: append([]compiler.DomainPriority(nil), options.DomainPriorities...),
		FrontierScores:   append([]compiler.CandidateFrontierScore(nil), options.FrontierScores...),
		HistoricalStats:  append([]compiler.CandidateHistory(nil), options.HistoricalStats...),
	})
	if err != nil {
		return ToolCompilePreview{}, err
	}
	return ToolCompilePreview{
		Workspace: root,
		Action:    action,
		Mode:      "metadata-only",
		Nodes:     append([]compiler.PlanNode(nil), plan.Nodes...),
		Warnings:  append([]string(nil), plan.Warnings...),
	}, nil
}

func CreateToolCompilePreviewReview(root string, options ToolCompilePreviewOptions) (ToolCompilePreviewReview, error) {
	preview, err := PreviewToolCompilePlan(root, options)
	if err != nil {
		return ToolCompilePreviewReview{}, err
	}
	review := ToolCompilePreviewReview{
		Status:  "dry-run",
		Mode:    "metadata-only",
		Preview: preview,
	}
	dir := toolCompilePreviewReviewDir(root)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return ToolCompilePreviewReview{}, fmt.Errorf("create compile preview review directory: %w", err)
	}
	review.Path = filepath.Join(dir, safeToolCompilePreviewReviewName(review)+".json")
	data, err := json.MarshalIndent(review, "", "  ")
	if err != nil {
		return ToolCompilePreviewReview{}, fmt.Errorf("encode compile preview review: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(review.Path, data, 0o644); err != nil {
		return ToolCompilePreviewReview{}, fmt.Errorf("write compile preview review: %w", err)
	}
	return review, nil
}

func ListToolCompilePreviewReviews(root string) ([]ToolCompilePreviewReview, error) {
	doctor, err := doctorWorkspaceStrictMetadataOnly(root)
	if err != nil {
		return nil, err
	}
	if !doctor.Ready {
		return nil, fmt.Errorf("workspace is not ready: %v", doctor.Errors)
	}
	paths, err := filepath.Glob(filepath.Join(toolCompilePreviewReviewDir(root), "*.json"))
	if err != nil {
		return nil, fmt.Errorf("glob compile preview reviews: %w", err)
	}
	sort.Strings(paths)
	reviews := make([]ToolCompilePreviewReview, 0, len(paths))
	for _, path := range paths {
		review, err := readToolCompilePreviewReviewFile(path)
		if err != nil {
			return nil, err
		}
		reviews = append(reviews, review)
	}
	return reviews, nil
}

func ReadToolCompilePreviewReview(root, reviewPath string) (ToolCompilePreviewReview, error) {
	doctor, err := doctorWorkspaceStrictMetadataOnly(root)
	if err != nil {
		return ToolCompilePreviewReview{}, err
	}
	if !doctor.Ready {
		return ToolCompilePreviewReview{}, fmt.Errorf("workspace is not ready: %v", doctor.Errors)
	}
	path, err := resolveToolCompilePreviewReviewPath(root, reviewPath)
	if err != nil {
		return ToolCompilePreviewReview{}, err
	}
	return readToolCompilePreviewReviewFile(path)
}

func AnnotateToolCompilePreviewReview(root, reviewPath string, options ToolCompilePreviewReviewAnnotationOptions) (ToolCompilePreviewReview, error) {
	annotation, err := normalizeToolCompilePreviewReviewAnnotation(options)
	if err != nil {
		return ToolCompilePreviewReview{}, err
	}
	doctor, err := DoctorWorkspaceWithOptions(root, DoctorOptions{RequireSafetyAuthorization: true})
	if err != nil {
		return ToolCompilePreviewReview{}, err
	}
	if !doctor.Ready {
		return ToolCompilePreviewReview{}, fmt.Errorf("workspace is not ready: %v", doctor.Errors)
	}
	path, err := resolveToolCompilePreviewReviewPath(root, reviewPath)
	if err != nil {
		return ToolCompilePreviewReview{}, err
	}
	review, err := readToolCompilePreviewReviewFile(path)
	if err != nil {
		return ToolCompilePreviewReview{}, err
	}
	review.Annotation = &annotation
	data, err := json.MarshalIndent(review, "", "  ")
	if err != nil {
		return ToolCompilePreviewReview{}, fmt.Errorf("encode compile preview review annotation: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return ToolCompilePreviewReview{}, fmt.Errorf("write compile preview review annotation: %w", err)
	}
	review.Path = path
	return review, nil
}

func CompareToolCompilePreviewReviews(root, baseReviewPath, candidateReviewPath string) (ToolCompilePreviewReviewComparison, error) {
	base, err := ReadToolCompilePreviewReview(root, baseReviewPath)
	if err != nil {
		return ToolCompilePreviewReviewComparison{}, err
	}
	candidate, err := ReadToolCompilePreviewReview(root, candidateReviewPath)
	if err != nil {
		return ToolCompilePreviewReviewComparison{}, err
	}
	return compareToolCompilePreviewReviewValues(base, candidate), nil
}

func SelectToolCompilePreviewReview(root, reviewPath string, options ToolCompilePreviewReviewSelectionOptions) (ToolCompilePreviewReviewSelection, error) {
	review, err := ReadToolCompilePreviewReview(root, reviewPath)
	if err != nil {
		return ToolCompilePreviewReviewSelection{}, err
	}
	reviews, err := ListToolCompilePreviewReviews(root)
	if err != nil {
		return ToolCompilePreviewReviewSelection{}, err
	}
	selectedBy := strings.TrimSpace(options.SelectedBy)
	if selectedBy == "" {
		selectedBy = "operator"
	}
	selectedAt := options.SelectedAt
	if selectedAt.IsZero() {
		selectedAt = time.Now().UTC()
	}
	superseded := make([]string, 0, len(reviews))
	for _, candidate := range reviews {
		if candidate.Path == "" || candidate.Path == review.Path {
			continue
		}
		superseded = append(superseded, candidate.Path)
	}
	sort.Strings(superseded)
	selection := ToolCompilePreviewReviewSelection{
		Path:            toolCompilePreviewReviewSelectionPath(root),
		Status:          "current",
		Mode:            "metadata-only",
		ReviewPath:      review.Path,
		Action:          review.Preview.Action,
		ReviewNodes:     len(review.Preview.Nodes),
		ReviewWarnings:  len(review.Preview.Warnings),
		SupersededPaths: superseded,
		SelectedBy:      selectedBy,
		SelectedAt:      selectedAt.UTC().Format(time.RFC3339),
		Reason:          strings.TrimSpace(options.Reason),
	}
	if err := writeToolCompilePreviewReviewSelection(selection); err != nil {
		return ToolCompilePreviewReviewSelection{}, err
	}
	return selection, nil
}

func ClearToolCompilePreviewReviewSelection(root string, options ToolCompilePreviewReviewSelectionClearOptions) (ToolCompilePreviewReviewSelection, error) {
	selection, found, err := ReadToolCompilePreviewReviewSelection(root)
	if err != nil {
		return ToolCompilePreviewReviewSelection{}, err
	}
	if !found {
		selection = ToolCompilePreviewReviewSelection{
			Path:   toolCompilePreviewReviewSelectionPath(root),
			Mode:   "metadata-only",
			Status: "cleared",
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
	selection.Path = toolCompilePreviewReviewSelectionPath(root)
	selection.Status = "cleared"
	selection.Mode = "metadata-only"
	selection.ClearedBy = clearedBy
	selection.ClearedAt = clearedAt.UTC().Format(time.RFC3339)
	selection.Reason = strings.TrimSpace(options.Reason)
	if err := writeToolCompilePreviewReviewSelection(selection); err != nil {
		return ToolCompilePreviewReviewSelection{}, err
	}
	return selection, nil
}

func writeToolCompilePreviewReviewSelection(selection ToolCompilePreviewReviewSelection) error {
	if err := os.MkdirAll(filepath.Dir(selection.Path), 0o755); err != nil {
		return fmt.Errorf("create compile preview review selection directory: %w", err)
	}
	data, err := json.MarshalIndent(selection, "", "  ")
	if err != nil {
		return fmt.Errorf("encode compile preview review selection: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(selection.Path, data, 0o644); err != nil {
		return fmt.Errorf("write compile preview review selection: %w", err)
	}
	return nil
}

func toolCompilePreviewReviewSelectionIsCurrent(selection ToolCompilePreviewReviewSelection) bool {
	return strings.TrimSpace(selection.Status) == "current"
}

func toolCompilePreviewReviewSelectionIsCleared(selection ToolCompilePreviewReviewSelection) bool {
	return strings.TrimSpace(selection.Status) == "cleared"
}

func ReadToolCompilePreviewReviewSelection(root string) (ToolCompilePreviewReviewSelection, bool, error) {
	doctor, err := doctorWorkspaceStrictMetadataOnly(root)
	if err != nil {
		return ToolCompilePreviewReviewSelection{}, false, err
	}
	if !doctor.Ready {
		return ToolCompilePreviewReviewSelection{}, false, fmt.Errorf("workspace is not ready: %v", doctor.Errors)
	}
	path := toolCompilePreviewReviewSelectionPath(root)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return ToolCompilePreviewReviewSelection{}, false, nil
		}
		return ToolCompilePreviewReviewSelection{}, false, fmt.Errorf("read compile preview review selection %s: %w", path, err)
	}
	var selection ToolCompilePreviewReviewSelection
	if err := json.Unmarshal(data, &selection); err != nil {
		return ToolCompilePreviewReviewSelection{}, false, fmt.Errorf("decode compile preview review selection %s: %w", path, err)
	}
	selection.Path = path
	return selection, true, nil
}

func readToolCompilePreviewReviewFile(path string) (ToolCompilePreviewReview, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return ToolCompilePreviewReview{}, fmt.Errorf("read compile preview review %s: %w", path, err)
	}
	var review ToolCompilePreviewReview
	if err := json.Unmarshal(data, &review); err != nil {
		return ToolCompilePreviewReview{}, fmt.Errorf("decode compile preview review %s: %w", path, err)
	}
	review.Path = path
	return review, nil
}

func compareToolCompilePreviewReviewValues(base, candidate ToolCompilePreviewReview) ToolCompilePreviewReviewComparison {
	comparison := ToolCompilePreviewReviewComparison{
		Status: "compared",
		Mode:   "metadata-only",
		Base: ToolCompilePreviewReviewComparisonSide{
			Path:     base.Path,
			Action:   base.Preview.Action,
			Nodes:    len(base.Preview.Nodes),
			Warnings: len(base.Preview.Warnings),
			Decision: compilePreviewReviewDecision(base),
		},
		Candidate: ToolCompilePreviewReviewComparisonSide{
			Path:     candidate.Path,
			Action:   candidate.Preview.Action,
			Nodes:    len(candidate.Preview.Nodes),
			Warnings: len(candidate.Preview.Warnings),
			Decision: compilePreviewReviewDecision(candidate),
		},
	}
	comparison.Summary.NodeDelta = len(candidate.Preview.Nodes) - len(base.Preview.Nodes)
	comparison.Summary.WarningDelta = len(candidate.Preview.Warnings) - len(base.Preview.Warnings)
	baseNodes := map[string]compiler.PlanNode{}
	baseIndexes := map[string]int{}
	for index, node := range base.Preview.Nodes {
		key := compilePreviewReviewNodeKey(node)
		baseNodes[key] = node
		baseIndexes[key] = index
	}
	candidateNodes := map[string]compiler.PlanNode{}
	candidateIndexes := map[string]int{}
	for index, node := range candidate.Preview.Nodes {
		key := compilePreviewReviewNodeKey(node)
		candidateNodes[key] = node
		candidateIndexes[key] = index
	}
	for index, node := range candidate.Preview.Nodes {
		key := compilePreviewReviewNodeKey(node)
		baseNode, ok := baseNodes[key]
		if !ok {
			comparison.Summary.AddedNodes++
			comparison.NodeDiffs = append(comparison.NodeDiffs, ToolCompilePreviewReviewNodeDiff{
				Change:         "added",
				ManifestName:   node.ManifestName,
				ToolID:         node.ToolID,
				AdapterID:      node.AdapterID,
				Operation:      node.Operation,
				ParserID:       node.ParserID,
				TargetKind:     node.TargetKind,
				DependencyName: node.DependencyName,
				DependencyKind: node.DependencyKind,
				Capability:     node.Capability,
				CandidateID:    node.ID,
				BaseIndex:      -1,
				CandidateIndex: index,
				CandidateScore: node.SelectionScore,
				CandidateSignals: append(
					[]string(nil),
					node.SelectionSignals...,
				),
			})
			continue
		}
		changes := []string{}
		baseIndex := baseIndexes[key]
		if baseIndex != index {
			changes = append(changes, "reordered")
			comparison.Summary.ReorderedNodes++
		}
		if baseNode.SelectionScore != node.SelectionScore {
			changes = append(changes, "score")
			comparison.Summary.ScoreChangedNodes++
		}
		if strings.Join(baseNode.SelectionSignals, "\x00") != strings.Join(node.SelectionSignals, "\x00") {
			changes = append(changes, "signals")
			comparison.Summary.SignalChangedNodes++
		}
		if len(changes) == 0 {
			continue
		}
		comparison.Summary.ChangedNodes++
		comparison.NodeDiffs = append(comparison.NodeDiffs, ToolCompilePreviewReviewNodeDiff{
			Change:           strings.Join(changes, ","),
			ManifestName:     node.ManifestName,
			ToolID:           node.ToolID,
			AdapterID:        node.AdapterID,
			Operation:        node.Operation,
			ParserID:         node.ParserID,
			TargetKind:       node.TargetKind,
			DependencyName:   node.DependencyName,
			DependencyKind:   node.DependencyKind,
			Capability:       node.Capability,
			BaseID:           baseNode.ID,
			CandidateID:      node.ID,
			BaseIndex:        baseIndex,
			CandidateIndex:   index,
			BaseScore:        baseNode.SelectionScore,
			CandidateScore:   node.SelectionScore,
			BaseSignals:      append([]string(nil), baseNode.SelectionSignals...),
			CandidateSignals: append([]string(nil), node.SelectionSignals...),
		})
	}
	for index, node := range base.Preview.Nodes {
		key := compilePreviewReviewNodeKey(node)
		if _, ok := candidateNodes[key]; ok {
			continue
		}
		comparison.Summary.RemovedNodes++
		comparison.NodeDiffs = append(comparison.NodeDiffs, ToolCompilePreviewReviewNodeDiff{
			Change:         "removed",
			ManifestName:   node.ManifestName,
			ToolID:         node.ToolID,
			AdapterID:      node.AdapterID,
			Operation:      node.Operation,
			ParserID:       node.ParserID,
			TargetKind:     node.TargetKind,
			DependencyName: node.DependencyName,
			DependencyKind: node.DependencyKind,
			Capability:     node.Capability,
			BaseID:         node.ID,
			BaseIndex:      index,
			CandidateIndex: -1,
			BaseScore:      node.SelectionScore,
			BaseSignals:    append([]string(nil), node.SelectionSignals...),
		})
	}
	comparison.WarningDiffs = compilePreviewReviewWarningDiffs(base.Preview.Warnings, candidate.Preview.Warnings)
	return comparison
}

func compilePreviewReviewDecision(review ToolCompilePreviewReview) string {
	if review.Annotation == nil {
		return ""
	}
	return strings.TrimSpace(review.Annotation.Decision)
}

func compilePreviewReviewNodeKey(node compiler.PlanNode) string {
	return strings.Join([]string{
		strings.TrimSpace(node.ManifestName),
		strings.TrimSpace(node.ToolID),
		strings.TrimSpace(node.AdapterID),
		strings.TrimSpace(node.Operation),
		strings.TrimSpace(node.ParserID),
		strings.TrimSpace(node.TargetKind),
		strings.TrimSpace(node.DependencyName),
		strings.TrimSpace(node.DependencyKind),
		strings.TrimSpace(node.Capability),
		strings.TrimSpace(node.Runner),
		strings.TrimSpace(node.Parser),
		strings.Join(node.RunCommand, "\x00"),
	}, "\x00")
}

func compilePreviewReviewWarningDiffs(baseWarnings, candidateWarnings []string) []ToolCompilePreviewReviewWarningDiff {
	baseCounts := map[string]int{}
	for _, warning := range baseWarnings {
		trimmed := strings.TrimSpace(warning)
		if trimmed == "" {
			continue
		}
		baseCounts[trimmed]++
	}
	candidateCounts := map[string]int{}
	for _, warning := range candidateWarnings {
		trimmed := strings.TrimSpace(warning)
		if trimmed == "" {
			continue
		}
		candidateCounts[trimmed]++
	}
	keys := make([]string, 0, len(baseCounts)+len(candidateCounts))
	seen := map[string]bool{}
	for warning := range baseCounts {
		keys = append(keys, warning)
		seen[warning] = true
	}
	for warning := range candidateCounts {
		if !seen[warning] {
			keys = append(keys, warning)
		}
	}
	sort.Strings(keys)
	diffs := []ToolCompilePreviewReviewWarningDiff{}
	for _, warning := range keys {
		if candidateCounts[warning] > baseCounts[warning] {
			for i := 0; i < candidateCounts[warning]-baseCounts[warning]; i++ {
				diffs = append(diffs, ToolCompilePreviewReviewWarningDiff{Change: "added", Warning: warning})
			}
		}
		if baseCounts[warning] > candidateCounts[warning] {
			for i := 0; i < baseCounts[warning]-candidateCounts[warning]; i++ {
				diffs = append(diffs, ToolCompilePreviewReviewWarningDiff{Change: "removed", Warning: warning})
			}
		}
	}
	return diffs
}

func normalizeToolCompilePreviewReviewAnnotation(options ToolCompilePreviewReviewAnnotationOptions) (ToolCompilePreviewReviewAnnotation, error) {
	decision := strings.ToLower(strings.TrimSpace(options.Decision))
	switch decision {
	case "accepted", "deferred", "rejected":
	case "":
		return ToolCompilePreviewReviewAnnotation{}, fmt.Errorf("compile preview review annotation decision is required")
	default:
		return ToolCompilePreviewReviewAnnotation{}, fmt.Errorf("unsupported compile preview review annotation decision %q", options.Decision)
	}
	reviewer := strings.TrimSpace(options.Reviewer)
	if reviewer == "" {
		reviewer = "operator"
	}
	reviewedAt := options.ReviewedAt
	if reviewedAt.IsZero() {
		reviewedAt = time.Now().UTC()
	}
	return ToolCompilePreviewReviewAnnotation{
		Decision:   decision,
		Reason:     strings.TrimSpace(options.Reason),
		Reviewer:   reviewer,
		ReviewedAt: reviewedAt.UTC().Format(time.RFC3339),
	}, nil
}

func toolCompilePreviewSignalsPath(root string) string {
	return filepath.Join(root, "toolchain", "compile-preview-signals.json")
}

func normalizeToolCompilePreviewSignals(signals ToolCompilePreviewSignals) (ToolCompilePreviewSignals, error) {
	if signals.SchemaVersion == 0 {
		signals.SchemaVersion = 1
	}
	if signals.SchemaVersion != 1 {
		return ToolCompilePreviewSignals{}, fmt.Errorf("unsupported compile preview signals schema_version: %d", signals.SchemaVersion)
	}
	if signals.Budget.MaxPlanNodes < 0 {
		return ToolCompilePreviewSignals{}, fmt.Errorf("compile preview signals budget max_plan_nodes must be non-negative")
	}
	return signals, nil
}

func toolCompilePreviewSignalsFromReplayEvents(events []eventlog.Event, manifests []toolchain.Manifest, options ToolCompilePreviewSignalDeriveOptions) (ToolCompilePreviewSignals, ToolCompilePreviewSignalDerivationSummary, error) {
	signals := ToolCompilePreviewSignals{
		SchemaVersion: 1,
		Budget:        ToolCompilePreviewSignalBudget{MaxPlanNodes: options.MaxPlanNodes},
	}
	summary := ToolCompilePreviewSignalDerivationSummary{}
	historyByKey := map[string]ToolCompilePreviewSignalHistory{}
	frontierByKey := map[string]ToolCompilePreviewSignalFrontierScore{}
	for _, event := range events {
		switch event.Type {
		case eventlog.EventTool:
			var execution struct {
				Tool            string `json:"tool"`
				ExecutionStatus string `json:"execution_status"`
				Capability      string `json:"capability"`
			}
			if err := json.Unmarshal(event.Payload, &execution); err != nil {
				return ToolCompilePreviewSignals{}, ToolCompilePreviewSignalDerivationSummary{}, fmt.Errorf("decode replay tool event %d: %w", event.ID, err)
			}
			status := strings.ToLower(strings.TrimSpace(execution.ExecutionStatus))
			if status == "" {
				continue
			}
			summary.ToolExecutions++
			manifestName := strings.TrimSpace(execution.Tool)
			capability := strings.TrimSpace(execution.Capability)
			if manifestName == "" || capability == "" {
				summary.Ignored++
				continue
			}
			key := manifestName + "\x00" + capability
			stat := historyByKey[key]
			stat.ManifestName = manifestName
			stat.Capability = capability
			switch status {
			case "completed":
				stat.Successes++
				summary.Successes++
			case "failed":
				stat.Failures++
				summary.Failures++
			default:
				summary.Ignored++
				continue
			}
			historyByKey[key] = stat
		case eventlog.EventFact:
			var candidatePath toolCompilePreviewReplayCandidatePathPayload
			if err := json.Unmarshal(event.Payload, &candidatePath); err != nil {
				return ToolCompilePreviewSignals{}, ToolCompilePreviewSignalDerivationSummary{}, fmt.Errorf("decode replay fact event %d: %w", event.ID, err)
			}
			scores, err := toolCompilePreviewFrontierScoresFromReplayCandidatePath(candidatePath, manifests)
			if err != nil {
				return ToolCompilePreviewSignals{}, ToolCompilePreviewSignalDerivationSummary{}, fmt.Errorf("derive frontier score from fact event %d: %w", event.ID, err)
			}
			for _, score := range scores {
				key := strings.TrimSpace(score.ManifestName) + "\x00" + strings.TrimSpace(score.Capability)
				if strings.TrimSpace(key) == "\x00" {
					continue
				}
				if current, ok := frontierByKey[key]; ok && frontier.Score(current.Path.toCandidatePath()) >= frontier.Score(score.Path.toCandidatePath()) {
					continue
				}
				frontierByKey[key] = score
			}
		case eventlog.EventIntent:
			var intent toolCompilePreviewReplayIntentPayload
			if err := json.Unmarshal(event.Payload, &intent); err != nil {
				return ToolCompilePreviewSignals{}, ToolCompilePreviewSignalDerivationSummary{}, fmt.Errorf("decode replay intent event %d: %w", event.ID, err)
			}
			scores, err := toolCompilePreviewFrontierScoresFromReplayIntent(intent, manifests)
			if err != nil {
				return ToolCompilePreviewSignals{}, ToolCompilePreviewSignalDerivationSummary{}, fmt.Errorf("derive frontier score from intent event %d: %w", event.ID, err)
			}
			for _, score := range scores {
				key := strings.TrimSpace(score.ManifestName) + "\x00" + strings.TrimSpace(score.Capability)
				if strings.TrimSpace(key) == "\x00" {
					continue
				}
				if current, ok := frontierByKey[key]; ok && frontier.Score(current.Path.toCandidatePath()) >= frontier.Score(score.Path.toCandidatePath()) {
					continue
				}
				frontierByKey[key] = score
			}
		default:
			continue
		}
	}
	keys := make([]string, 0, len(historyByKey))
	for key := range historyByKey {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		signals.HistoricalStats = append(signals.HistoricalStats, historyByKey[key])
	}
	frontierKeys := make([]string, 0, len(frontierByKey))
	for key := range frontierByKey {
		frontierKeys = append(frontierKeys, key)
	}
	sort.Strings(frontierKeys)
	for _, key := range frontierKeys {
		signals.FrontierScores = append(signals.FrontierScores, frontierByKey[key])
	}
	summary.FrontierScores = len(signals.FrontierScores)
	return signals, summary, nil
}

type toolCompilePreviewReplayIntentPayload struct {
	ID                 string  `json:"id"`
	IntentID           string  `json:"intent_id"`
	Kind               string  `json:"kind"`
	Goal               string  `json:"goal"`
	Target             string  `json:"target"`
	Rationale          string  `json:"rationale"`
	Action             string  `json:"action"`
	Domain             string  `json:"domain"`
	ManifestName       string  `json:"manifest_name"`
	Capability         string  `json:"capability"`
	Confidence         float64 `json:"confidence"`
	MissionValue       float64 `json:"mission_value"`
	UnlockPotential    float64 `json:"unlock_potential"`
	EvidenceConfidence float64 `json:"evidence_confidence"`
	Novelty            float64 `json:"novelty"`
	Cost               float64 `json:"cost"`
	DuplicateRisk      float64 `json:"duplicate_risk"`
	Noise              float64 `json:"noise"`
	ExhaustedPenalty   float64 `json:"exhausted_penalty"`
}

type toolCompilePreviewReplayCandidatePathPayload struct {
	ID                 string  `json:"id"`
	FactID             string  `json:"fact_id"`
	Kind               string  `json:"kind"`
	Statement          string  `json:"statement"`
	Target             string  `json:"target"`
	ExpectedUnlock     string  `json:"expected_unlock"`
	Action             string  `json:"action"`
	Domain             string  `json:"domain"`
	ManifestName       string  `json:"manifest_name"`
	Capability         string  `json:"capability"`
	Confidence         float64 `json:"confidence"`
	MissionValue       float64 `json:"mission_value"`
	UnlockPotential    float64 `json:"unlock_potential"`
	EvidenceConfidence float64 `json:"evidence_confidence"`
	Novelty            float64 `json:"novelty"`
	Cost               float64 `json:"cost"`
	DuplicateRisk      float64 `json:"duplicate_risk"`
	Noise              float64 `json:"noise"`
	ExhaustedPenalty   float64 `json:"exhausted_penalty"`
}

func toolCompilePreviewFrontierScoresFromReplayIntent(intent toolCompilePreviewReplayIntentPayload, manifests []toolchain.Manifest) ([]ToolCompilePreviewSignalFrontierScore, error) {
	if strings.TrimSpace(intent.Kind) != "hypothesis" {
		return nil, nil
	}
	path := toolCompilePreviewSignalPathFromIntent(intent)
	return toolCompilePreviewFrontierScoresFromMetadata(manifests, toolCompilePreviewFrontierMetadata{
		ManifestName: intent.ManifestName,
		Capability:   intent.Capability,
		Action:       intent.Action,
		Domain:       intent.Domain,
		Target:       intent.Target,
		ID:           firstNonEmptyString(intent.IntentID, intent.ID),
		Path:         path,
	})
}

func toolCompilePreviewFrontierScoresFromReplayCandidatePath(candidate toolCompilePreviewReplayCandidatePathPayload, manifests []toolchain.Manifest) ([]ToolCompilePreviewSignalFrontierScore, error) {
	if !toolCompilePreviewCandidatePathKind(candidate.Kind) {
		return nil, nil
	}
	path := toolCompilePreviewSignalPathFromCandidatePath(candidate)
	return toolCompilePreviewFrontierScoresFromMetadata(manifests, toolCompilePreviewFrontierMetadata{
		ManifestName: candidate.ManifestName,
		Capability:   candidate.Capability,
		Action:       candidate.Action,
		Domain:       candidate.Domain,
		Target:       candidate.Target,
		ID:           firstNonEmptyString(candidate.FactID, candidate.ID),
		Path:         path,
	})
}

type toolCompilePreviewFrontierMetadata struct {
	ManifestName string
	Capability   string
	Action       string
	Domain       string
	Target       string
	ID           string
	Path         ToolCompilePreviewSignalPath
}

func toolCompilePreviewFrontierScoresFromMetadata(manifests []toolchain.Manifest, metadata toolCompilePreviewFrontierMetadata) ([]ToolCompilePreviewSignalFrontierScore, error) {
	if toolCompilePreviewSignalPathScore(metadata.Path) == 0 {
		return nil, nil
	}
	manifestName := strings.TrimSpace(metadata.ManifestName)
	capability := strings.TrimSpace(metadata.Capability)
	if manifestName != "" && capability != "" {
		return []ToolCompilePreviewSignalFrontierScore{{
			ManifestName: manifestName,
			Capability:   capability,
			Path:         metadata.Path,
		}}, nil
	}
	action := compiler.SemanticAction(strings.TrimSpace(metadata.Action))
	if action == "" {
		return nil, nil
	}
	domain, ok := toolCompilePreviewDomainForFields(metadata.Domain, metadata.Target, metadata.ID)
	if !ok {
		return nil, nil
	}
	plan, err := compiler.NewCompiler().Compile(compiler.CompileRequest{
		Action: action,
		Evidence: compiler.EvidenceSlice{
			Nodes: []evidence.Node{
				{
					ID:         "frontier-" + strings.TrimSpace(metadata.Path.ID),
					Type:       evidence.NodeHypothesis,
					Label:      strings.TrimSpace(metadata.Path.Hypothesis),
					SourceKind: "replay-frontier",
				},
			},
		},
		Manifests: manifests,
	})
	if err != nil {
		return nil, nil
	}
	scores := []ToolCompilePreviewSignalFrontierScore{}
	for _, node := range plan.Nodes {
		nodeDomain, ok := toolCompilePreviewDomainForCapability(node.Capability)
		if !ok || nodeDomain != domain {
			continue
		}
		scores = append(scores, ToolCompilePreviewSignalFrontierScore{
			ManifestName: node.ManifestName,
			Capability:   node.Capability,
			Path:         metadata.Path,
		})
	}
	return scores, nil
}

func toolCompilePreviewCandidatePathKind(kind string) bool {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "candidate_path", "frontier_path", "frontier_candidate":
		return true
	default:
		return false
	}
}

func toolCompilePreviewSignalPathFromIntent(intent toolCompilePreviewReplayIntentPayload) ToolCompilePreviewSignalPath {
	confidence := intent.Confidence
	path := ToolCompilePreviewSignalPath{
		ID:                 firstNonEmptyString(intent.IntentID, intent.ID),
		Target:             strings.TrimSpace(intent.Target),
		Hypothesis:         strings.TrimSpace(intent.Goal),
		ExpectedUnlock:     strings.TrimSpace(intent.Rationale),
		MissionValue:       intent.MissionValue,
		UnlockPotential:    intent.UnlockPotential,
		EvidenceConfidence: intent.EvidenceConfidence,
		Novelty:            intent.Novelty,
		Cost:               intent.Cost,
		DuplicateRisk:      intent.DuplicateRisk,
		Noise:              intent.Noise,
		ExhaustedPenalty:   intent.ExhaustedPenalty,
	}
	return toolCompilePreviewSignalPathWithConfidence(path, confidence)
}

func toolCompilePreviewSignalPathFromCandidatePath(candidate toolCompilePreviewReplayCandidatePathPayload) ToolCompilePreviewSignalPath {
	path := ToolCompilePreviewSignalPath{
		ID:                 firstNonEmptyString(candidate.FactID, candidate.ID),
		Target:             strings.TrimSpace(candidate.Target),
		Hypothesis:         strings.TrimSpace(candidate.Statement),
		ExpectedUnlock:     strings.TrimSpace(candidate.ExpectedUnlock),
		MissionValue:       candidate.MissionValue,
		UnlockPotential:    candidate.UnlockPotential,
		EvidenceConfidence: candidate.EvidenceConfidence,
		Novelty:            candidate.Novelty,
		Cost:               candidate.Cost,
		DuplicateRisk:      candidate.DuplicateRisk,
		Noise:              candidate.Noise,
		ExhaustedPenalty:   candidate.ExhaustedPenalty,
	}
	return toolCompilePreviewSignalPathWithConfidence(path, candidate.Confidence)
}

func toolCompilePreviewSignalPathWithConfidence(path ToolCompilePreviewSignalPath, confidence float64) ToolCompilePreviewSignalPath {
	if confidence > 0 {
		if path.MissionValue == 0 {
			path.MissionValue = confidence
		}
		if path.UnlockPotential == 0 {
			path.UnlockPotential = confidence
		}
		if path.EvidenceConfidence == 0 {
			path.EvidenceConfidence = confidence
		}
	}
	return path
}

func toolCompilePreviewSignalPathScore(path ToolCompilePreviewSignalPath) float64 {
	return frontier.Score(path.toCandidatePath())
}

func toolCompilePreviewIntentDomain(intent toolCompilePreviewReplayIntentPayload) (frontier.Domain, bool) {
	return toolCompilePreviewDomainForFields(intent.Domain, intent.Target, firstNonEmptyString(intent.IntentID, intent.ID))
}

func toolCompilePreviewDomainForFields(domainText, targetText, idText string) (frontier.Domain, bool) {
	if domain, err := parseToolCompilePreviewSignalDomain(domainText); err == nil {
		return domain, true
	}
	target := strings.TrimSpace(targetText)
	if parsed, err := url.Parse(target); err == nil && parsed.Scheme != "" && parsed.Host != "" {
		return frontier.DomainWebAPI, true
	}
	if _, err := netip.ParsePrefix(target); err == nil {
		return frontier.DomainInfra, true
	}
	if _, err := netip.ParseAddr(target); err == nil {
		return frontier.DomainInfra, true
	}
	id := strings.ToLower(strings.TrimSpace(idText))
	switch {
	case strings.HasPrefix(id, "web-"), strings.HasPrefix(id, "web_"):
		return frontier.DomainWebAPI, true
	case strings.HasPrefix(id, "infra-"), strings.HasPrefix(id, "infra_"):
		return frontier.DomainInfra, true
	case strings.HasPrefix(id, "ctf-"), strings.HasPrefix(id, "ctf_"):
		return frontier.DomainCTF, true
	}
	if target != "" {
		return frontier.DomainCTF, true
	}
	return "", false
}

func toolCompilePreviewDomainForCapability(capability string) (frontier.Domain, bool) {
	switch {
	case strings.HasPrefix(capability, "web."):
		return frontier.DomainWebAPI, true
	case strings.HasPrefix(capability, "infra."):
		return frontier.DomainInfra, true
	case strings.HasPrefix(capability, "ctf."):
		return frontier.DomainCTF, true
	default:
		return "", false
	}
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func mergeToolCompilePreviewOptions(options ToolCompilePreviewOptions, signals ToolCompilePreviewSignals) (ToolCompilePreviewOptions, error) {
	persisted, err := toolCompilePreviewOptionsFromSignals(signals)
	if err != nil {
		return ToolCompilePreviewOptions{}, err
	}
	merged := options
	if merged.Budget.MaxPlanNodes == 0 {
		merged.Budget = persisted.Budget
	}
	merged.DomainPriorities = append(append([]compiler.DomainPriority(nil), persisted.DomainPriorities...), merged.DomainPriorities...)
	merged.FrontierScores = append(append([]compiler.CandidateFrontierScore(nil), persisted.FrontierScores...), merged.FrontierScores...)
	merged.HistoricalStats = append(append([]compiler.CandidateHistory(nil), persisted.HistoricalStats...), merged.HistoricalStats...)
	return merged, nil
}

func toolCompilePreviewOptionsFromSignals(signals ToolCompilePreviewSignals) (ToolCompilePreviewOptions, error) {
	options := ToolCompilePreviewOptions{
		Budget: compiler.CompileBudget{MaxPlanNodes: signals.Budget.MaxPlanNodes},
	}
	for _, priority := range signals.DomainPriorities {
		domain, err := parseToolCompilePreviewSignalDomain(priority.Domain)
		if err != nil {
			return ToolCompilePreviewOptions{}, err
		}
		options.DomainPriorities = append(options.DomainPriorities, compiler.DomainPriority{
			Domain: domain,
			Weight: priority.Weight,
		})
	}
	for _, score := range signals.FrontierScores {
		manifestName := strings.TrimSpace(score.ManifestName)
		capability := strings.TrimSpace(score.Capability)
		if manifestName == "" && capability == "" {
			continue
		}
		options.FrontierScores = append(options.FrontierScores, compiler.CandidateFrontierScore{
			ManifestName: manifestName,
			Capability:   capability,
			Path:         score.Path.toCandidatePath(),
		})
	}
	for _, history := range signals.HistoricalStats {
		manifestName := strings.TrimSpace(history.ManifestName)
		capability := strings.TrimSpace(history.Capability)
		if manifestName == "" && capability == "" {
			continue
		}
		options.HistoricalStats = append(options.HistoricalStats, compiler.CandidateHistory{
			ManifestName: manifestName,
			Capability:   capability,
			Successes:    history.Successes,
			Failures:     history.Failures,
		})
	}
	return options, nil
}

func parseToolCompilePreviewSignalDomain(value string) (frontier.Domain, error) {
	switch strings.TrimSpace(value) {
	case string(frontier.DomainWebAPI), "web":
		return frontier.DomainWebAPI, nil
	case string(frontier.DomainInfra), "infra":
		return frontier.DomainInfra, nil
	case string(frontier.DomainCTF), "ctf":
		return frontier.DomainCTF, nil
	default:
		return "", fmt.Errorf("unsupported compile preview signal domain: %s", value)
	}
}

func (path ToolCompilePreviewSignalPath) toCandidatePath() frontier.CandidatePath {
	return frontier.CandidatePath{
		ID:                 strings.TrimSpace(path.ID),
		Target:             strings.TrimSpace(path.Target),
		Hypothesis:         strings.TrimSpace(path.Hypothesis),
		ExpectedUnlock:     strings.TrimSpace(path.ExpectedUnlock),
		MissionValue:       path.MissionValue,
		UnlockPotential:    path.UnlockPotential,
		EvidenceConfidence: path.EvidenceConfidence,
		Novelty:            path.Novelty,
		Cost:               path.Cost,
		DuplicateRisk:      path.DuplicateRisk,
		Noise:              path.Noise,
		ExhaustedPenalty:   path.ExhaustedPenalty,
	}
}

func PlanApprovedToolchainInstalls(root string) (ToolchainInstallDispatchPlan, error) {
	requests, err := ListToolchainInstallRequests(root)
	if err != nil {
		return ToolchainInstallDispatchPlan{}, err
	}
	return toolchainInstallDispatchPlanFromRequests(requests), nil
}

func PlanFailedToolchainInstallRetries(root string) (ToolchainInstallRetryPlan, error) {
	executions, err := ListToolchainInstallDispatchExecutions(root)
	if err != nil {
		return ToolchainInstallRetryPlan{}, err
	}
	return toolchainInstallRetryPlanFromExecutions(executions), nil
}

func CreateToolchainInstallRetryDispatch(root string) (ToolchainInstallRetryDispatch, error) {
	plan, err := PlanFailedToolchainInstallRetries(root)
	if err != nil {
		return ToolchainInstallRetryDispatch{}, err
	}
	dispatch := ToolchainInstallRetryDispatch{
		Status:  "dry-run",
		Mode:    "metadata-only",
		Summary: plan.Summary,
		Ready:   append([]ToolchainInstallRetryCandidate(nil), plan.Ready...),
		Skipped: append([]ToolchainInstallRetrySkip(nil), plan.Skipped...),
	}
	dir := toolchainInstallRetryDispatchDir(root)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return ToolchainInstallRetryDispatch{}, fmt.Errorf("create install retry dispatch directory: %w", err)
	}
	dispatch.Path = filepath.Join(dir, safeInstallRetryDispatchName(plan)+".json")
	data, err := json.MarshalIndent(dispatch, "", "  ")
	if err != nil {
		return ToolchainInstallRetryDispatch{}, fmt.Errorf("encode install retry dispatch: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(dispatch.Path, data, 0o644); err != nil {
		return ToolchainInstallRetryDispatch{}, fmt.Errorf("write install retry dispatch: %w", err)
	}
	return dispatch, nil
}

func ListToolchainInstallRetryDispatches(root string) ([]ToolchainInstallRetryDispatch, error) {
	doctor, err := doctorWorkspaceMetadataOnly(root)
	if err != nil {
		return nil, err
	}
	if !doctor.Ready {
		return nil, fmt.Errorf("workspace is not ready: %v", doctor.Errors)
	}

	paths, err := filepath.Glob(filepath.Join(toolchainInstallRetryDispatchDir(root), "*.json"))
	if err != nil {
		return nil, fmt.Errorf("glob install retry dispatches: %w", err)
	}
	sort.Strings(paths)
	dispatches := make([]ToolchainInstallRetryDispatch, 0, len(paths))
	for _, path := range paths {
		dispatch, err := readToolchainInstallRetryDispatchFile(path)
		if err != nil {
			return nil, err
		}
		dispatches = append(dispatches, dispatch)
	}
	return dispatches, nil
}

func ReadToolchainInstallRetryDispatch(root, dispatchPath string) (ToolchainInstallRetryDispatch, error) {
	doctor, err := doctorWorkspaceMetadataOnly(root)
	if err != nil {
		return ToolchainInstallRetryDispatch{}, err
	}
	if !doctor.Ready {
		return ToolchainInstallRetryDispatch{}, fmt.Errorf("workspace is not ready: %v", doctor.Errors)
	}
	path, err := resolveToolchainInstallRetryDispatchPath(root, dispatchPath)
	if err != nil {
		return ToolchainInstallRetryDispatch{}, err
	}
	return readToolchainInstallRetryDispatchFile(path)
}

func readToolchainInstallRetryDispatchFile(path string) (ToolchainInstallRetryDispatch, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return ToolchainInstallRetryDispatch{}, fmt.Errorf("read install retry dispatch %s: %w", path, err)
	}
	var dispatch ToolchainInstallRetryDispatch
	if err := json.Unmarshal(data, &dispatch); err != nil {
		return ToolchainInstallRetryDispatch{}, fmt.Errorf("decode install retry dispatch %s: %w", path, err)
	}
	dispatch.Path = path
	return dispatch, nil
}

func ExecuteToolchainInstallRetryDispatch(root, dispatchPath string, options ToolchainInstallRetryDispatchExecutionOptions) (ToolchainInstallRetryDispatchExecution, error) {
	if !options.Execute {
		return ToolchainInstallRetryDispatchExecution{}, fmt.Errorf("install retry dispatch execution requires explicit execute")
	}
	doctor, err := DoctorWorkspaceWithOptions(root, DoctorOptions{RequireSafetyAuthorization: true})
	if err != nil {
		return ToolchainInstallRetryDispatchExecution{}, err
	}
	if !doctor.Ready {
		return ToolchainInstallRetryDispatchExecution{}, fmt.Errorf("workspace is not ready: %v", doctor.Errors)
	}
	dispatch, err := ReadToolchainInstallRetryDispatch(root, dispatchPath)
	if err != nil {
		return ToolchainInstallRetryDispatchExecution{}, err
	}
	if installRequestDisplayStatus(dispatch.Status) != "dry-run" || installRequestDisplayStatus(dispatch.Mode) != "metadata-only" {
		return ToolchainInstallRetryDispatchExecution{}, fmt.Errorf("install retry dispatch must be dry-run metadata-only before execution")
	}
	if err := revalidateRetryDispatchRequestsApproved(root, dispatch.Ready); err != nil {
		return ToolchainInstallRetryDispatchExecution{}, err
	}
	execution := ToolchainInstallRetryDispatchExecution{
		Status:            "completed",
		Mode:              "execute",
		RetryDispatchPath: dispatch.Path,
		Warnings:          append([]string(nil), doctor.Warnings...),
		Skipped:           append([]ToolchainInstallRetrySkip(nil), dispatch.Skipped...),
	}
	for _, candidate := range dispatch.Ready {
		result := ToolchainInstallRetryDispatchResult{
			ExecutionPath: candidate.ExecutionPath,
			DispatchPath:  candidate.DispatchPath,
			RequestPath:   candidate.RequestPath,
			Pack:          candidate.Pack,
			Dependency:    candidate.Dependency,
			Command:       append([]string(nil), candidate.Command...),
			LastStatus:    candidate.LastStatus,
			LastOutput:    candidate.LastOutput,
			LastError:     candidate.LastError,
			Status:        "completed",
		}
		output, err := runToolchainInstallRetryCommand(root, dispatch.Path, candidate)
		result.Output = strings.TrimSpace(output)
		if err != nil {
			result.Status = "failed"
			result.Error = err.Error()
			execution.Status = "failed"
			execution.Results = append(execution.Results, result)
			if writeErr := writeToolchainInstallRetryDispatchExecution(root, &execution); writeErr != nil {
				return execution, writeErr
			}
			return execution, fmt.Errorf("execute retry install command for %s/%s: %w", candidate.Pack, candidate.Dependency, err)
		}
		execution.Results = append(execution.Results, result)
	}
	if err := writeToolchainInstallRetryDispatchExecution(root, &execution); err != nil {
		return execution, err
	}
	return execution, nil
}

func revalidateRetryDispatchRequestsApproved(root string, candidates []ToolchainInstallRetryCandidate) error {
	for _, candidate := range candidates {
		request, err := readToolchainInstallRequestForRetry(root, candidate.RequestPath)
		if err != nil {
			return err
		}
		status := installRequestDisplayStatus(request.Status)
		if status != "approved" {
			return fmt.Errorf("install retry dispatch ready request must be approved: %s/%s status=%s", candidate.Pack, candidate.Dependency, status)
		}
		if strings.TrimSpace(request.Pack) != strings.TrimSpace(candidate.Pack) || strings.TrimSpace(request.Dependency) != strings.TrimSpace(candidate.Dependency) {
			return fmt.Errorf("install retry dispatch request mismatch: candidate=%s/%s request=%s/%s", candidate.Pack, candidate.Dependency, request.Pack, request.Dependency)
		}
	}
	return nil
}

func readToolchainInstallRequestForRetry(root string, requestPath string) (ToolchainInstallRequest, error) {
	path, err := resolveToolchainInstallRequestPath(root, requestPath)
	if err != nil {
		return ToolchainInstallRequest{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ToolchainInstallRequest{}, fmt.Errorf("read install request %s: %w", path, err)
	}
	var request ToolchainInstallRequest
	if err := json.Unmarshal(data, &request); err != nil {
		return ToolchainInstallRequest{}, fmt.Errorf("decode install request %s: %w", path, err)
	}
	request.Path = path
	return request, nil
}

func runToolchainInstallRetryCommand(root, retryDispatchPath string, candidate ToolchainInstallRetryCandidate) (string, error) {
	if len(candidate.Command) == 0 {
		return "", fmt.Errorf("retry install command is empty for %s/%s", candidate.Pack, candidate.Dependency)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, candidate.Command[0], candidate.Command[1:]...)
	cmd.Dir = root
	cmd.Env = append(os.Environ(),
		"OURO_WORKSPACE="+root,
		"OURO_INSTALL_RETRY_DISPATCH="+retryDispatchPath,
		"OURO_INSTALL_SOURCE_EXECUTION="+candidate.ExecutionPath,
		"OURO_INSTALL_DISPATCH="+candidate.DispatchPath,
		"OURO_INSTALL_REQUEST="+candidate.RequestPath,
		"OURO_INSTALL_PACK="+candidate.Pack,
		"OURO_INSTALL_DEPENDENCY="+candidate.Dependency,
	)
	output, err := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		return string(output), fmt.Errorf("retry install command timed out")
	}
	return string(output), err
}

func writeToolchainInstallRetryDispatchExecution(root string, execution *ToolchainInstallRetryDispatchExecution) error {
	dir := toolchainInstallRetryExecutionDir(root)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create install retry execution directory: %w", err)
	}
	name := safeInstallRequestName(strings.Join([]string{
		"install-retry-execution",
		execution.RetryDispatchPath,
		time.Now().UTC().Format(time.RFC3339Nano),
	}, "|"))
	execution.Path = filepath.Join(dir, name+".json")
	data, err := json.MarshalIndent(execution, "", "  ")
	if err != nil {
		return fmt.Errorf("encode install retry execution: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(execution.Path, data, 0o644); err != nil {
		return fmt.Errorf("write install retry execution: %w", err)
	}
	return nil
}

func ListToolchainInstallRetryDispatchExecutions(root string) ([]ToolchainInstallRetryDispatchExecution, error) {
	doctor, err := doctorWorkspaceMetadataOnly(root)
	if err != nil {
		return nil, err
	}
	if !doctor.Ready {
		return nil, fmt.Errorf("workspace is not ready: %v", doctor.Errors)
	}

	paths, err := filepath.Glob(filepath.Join(toolchainInstallRetryExecutionDir(root), "*.json"))
	if err != nil {
		return nil, fmt.Errorf("glob install retry executions: %w", err)
	}
	sort.Strings(paths)
	executions := make([]ToolchainInstallRetryDispatchExecution, 0, len(paths))
	for _, path := range paths {
		execution, err := readToolchainInstallRetryDispatchExecutionFile(path)
		if err != nil {
			return nil, err
		}
		executions = append(executions, execution)
	}
	return executions, nil
}

func ReadToolchainInstallRetryDispatchExecution(root, executionPath string) (ToolchainInstallRetryDispatchExecution, error) {
	doctor, err := doctorWorkspaceMetadataOnly(root)
	if err != nil {
		return ToolchainInstallRetryDispatchExecution{}, err
	}
	if !doctor.Ready {
		return ToolchainInstallRetryDispatchExecution{}, fmt.Errorf("workspace is not ready: %v", doctor.Errors)
	}
	path, err := resolveToolchainInstallRetryExecutionPath(root, executionPath)
	if err != nil {
		return ToolchainInstallRetryDispatchExecution{}, err
	}
	return readToolchainInstallRetryDispatchExecutionFile(path)
}

func readToolchainInstallRetryDispatchExecutionFile(path string) (ToolchainInstallRetryDispatchExecution, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return ToolchainInstallRetryDispatchExecution{}, fmt.Errorf("read install retry execution %s: %w", path, err)
	}
	var execution ToolchainInstallRetryDispatchExecution
	if err := json.Unmarshal(data, &execution); err != nil {
		return ToolchainInstallRetryDispatchExecution{}, fmt.Errorf("decode install retry execution %s: %w", path, err)
	}
	execution.Path = path
	return execution, nil
}

func CreateToolchainInstallDispatch(root string) (ToolchainInstallDispatch, error) {
	plan, err := PlanApprovedToolchainInstalls(root)
	if err != nil {
		return ToolchainInstallDispatch{}, err
	}
	dispatch := ToolchainInstallDispatch{
		Status:  "dry-run",
		Mode:    "metadata-only",
		Summary: plan.Summary,
		Ready:   append([]ToolchainInstallRequest(nil), plan.Ready...),
		Skipped: append([]ToolchainInstallDispatchSkip(nil), plan.Skipped...),
	}
	dir := toolchainInstallDispatchDir(root)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return ToolchainInstallDispatch{}, fmt.Errorf("create install dispatch directory: %w", err)
	}
	dispatch.Path = filepath.Join(dir, safeInstallDispatchName(plan)+".json")
	data, err := json.MarshalIndent(dispatch, "", "  ")
	if err != nil {
		return ToolchainInstallDispatch{}, fmt.Errorf("encode install dispatch: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(dispatch.Path, data, 0o644); err != nil {
		return ToolchainInstallDispatch{}, fmt.Errorf("write install dispatch: %w", err)
	}
	return dispatch, nil
}

func ListToolchainInstallDispatches(root string) ([]ToolchainInstallDispatch, error) {
	doctor, err := doctorWorkspaceMetadataOnly(root)
	if err != nil {
		return nil, err
	}
	if !doctor.Ready {
		return nil, fmt.Errorf("workspace is not ready: %v", doctor.Errors)
	}

	paths, err := filepath.Glob(filepath.Join(toolchainInstallDispatchDir(root), "*.json"))
	if err != nil {
		return nil, fmt.Errorf("glob install dispatches: %w", err)
	}
	sort.Strings(paths)
	dispatches := make([]ToolchainInstallDispatch, 0, len(paths))
	for _, path := range paths {
		dispatch, err := readToolchainInstallDispatchFile(path)
		if err != nil {
			return nil, err
		}
		dispatches = append(dispatches, dispatch)
	}
	return dispatches, nil
}

func ReadToolchainInstallDispatch(root, dispatchPath string) (ToolchainInstallDispatch, error) {
	doctor, err := doctorWorkspaceMetadataOnly(root)
	if err != nil {
		return ToolchainInstallDispatch{}, err
	}
	if !doctor.Ready {
		return ToolchainInstallDispatch{}, fmt.Errorf("workspace is not ready: %v", doctor.Errors)
	}
	path, err := resolveToolchainInstallDispatchPath(root, dispatchPath)
	if err != nil {
		return ToolchainInstallDispatch{}, err
	}
	return readToolchainInstallDispatchFile(path)
}

func readToolchainInstallDispatchFile(path string) (ToolchainInstallDispatch, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return ToolchainInstallDispatch{}, fmt.Errorf("read install dispatch %s: %w", path, err)
	}
	var dispatch ToolchainInstallDispatch
	if err := json.Unmarshal(data, &dispatch); err != nil {
		return ToolchainInstallDispatch{}, fmt.Errorf("decode install dispatch %s: %w", path, err)
	}
	dispatch.Path = path
	return dispatch, nil
}

func ExecuteToolchainInstallDispatch(root, dispatchPath string, options ToolchainInstallDispatchExecutionOptions) (ToolchainInstallDispatchExecution, error) {
	if !options.Execute {
		return ToolchainInstallDispatchExecution{}, fmt.Errorf("install dispatch execution requires explicit execute")
	}
	doctor, err := DoctorWorkspaceWithOptions(root, DoctorOptions{RequireSafetyAuthorization: true})
	if err != nil {
		return ToolchainInstallDispatchExecution{}, err
	}
	if !doctor.Ready {
		return ToolchainInstallDispatchExecution{}, fmt.Errorf("workspace is not ready: %v", doctor.Errors)
	}
	dispatch, err := ReadToolchainInstallDispatch(root, dispatchPath)
	if err != nil {
		return ToolchainInstallDispatchExecution{}, err
	}
	if installRequestDisplayStatus(dispatch.Status) != "dry-run" || installRequestDisplayStatus(dispatch.Mode) != "metadata-only" {
		return ToolchainInstallDispatchExecution{}, fmt.Errorf("install dispatch must be dry-run metadata-only before execution")
	}
	execution := ToolchainInstallDispatchExecution{
		Status:       "completed",
		Mode:         "execute",
		DispatchPath: dispatch.Path,
		Warnings:     append([]string(nil), doctor.Warnings...),
		Skipped:      append([]ToolchainInstallDispatchSkip(nil), dispatch.Skipped...),
	}
	for _, request := range dispatch.Ready {
		if installRequestDisplayStatus(request.Status) != "approved" {
			return execution, fmt.Errorf("install dispatch ready request must be approved: %s/%s status=%s", request.Pack, request.Dependency, installRequestDisplayStatus(request.Status))
		}
		result := ToolchainInstallDispatchExecutionResult{
			RequestPath: request.Path,
			Pack:        request.Pack,
			Dependency:  request.Dependency,
			Command:     append([]string(nil), request.Command...),
			Status:      "completed",
		}
		output, err := runToolchainInstallCommand(root, dispatch.Path, request)
		result.Output = strings.TrimSpace(output)
		if err != nil {
			result.Status = "failed"
			result.Error = err.Error()
			execution.Status = "failed"
			execution.Results = append(execution.Results, result)
			if writeErr := writeToolchainInstallDispatchExecution(root, &execution); writeErr != nil {
				return execution, writeErr
			}
			return execution, fmt.Errorf("execute install command for %s/%s: %w", request.Pack, request.Dependency, err)
		}
		execution.Results = append(execution.Results, result)
	}
	if err := writeToolchainInstallDispatchExecution(root, &execution); err != nil {
		return execution, err
	}
	return execution, nil
}

func runToolchainInstallCommand(root, dispatchPath string, request ToolchainInstallRequest) (string, error) {
	if len(request.Command) == 0 {
		return "", fmt.Errorf("install command is empty for %s/%s", request.Pack, request.Dependency)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, request.Command[0], request.Command[1:]...)
	cmd.Dir = root
	cmd.Env = append(os.Environ(),
		"OURO_WORKSPACE="+root,
		"OURO_INSTALL_DISPATCH="+dispatchPath,
		"OURO_INSTALL_REQUEST="+request.Path,
		"OURO_INSTALL_PACK="+request.Pack,
		"OURO_INSTALL_DEPENDENCY="+request.Dependency,
	)
	output, err := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		return string(output), fmt.Errorf("install command timed out")
	}
	return string(output), err
}

func writeToolchainInstallDispatchExecution(root string, execution *ToolchainInstallDispatchExecution) error {
	dir := toolchainInstallExecutionDir(root)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create install execution directory: %w", err)
	}
	name := safeInstallRequestName(strings.Join([]string{
		"install-execution",
		execution.DispatchPath,
		time.Now().UTC().Format(time.RFC3339Nano),
	}, "|"))
	execution.Path = filepath.Join(dir, name+".json")
	data, err := json.MarshalIndent(execution, "", "  ")
	if err != nil {
		return fmt.Errorf("encode install execution: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(execution.Path, data, 0o644); err != nil {
		return fmt.Errorf("write install execution: %w", err)
	}
	return nil
}

func ListToolchainInstallDispatchExecutions(root string) ([]ToolchainInstallDispatchExecution, error) {
	doctor, err := doctorWorkspaceMetadataOnly(root)
	if err != nil {
		return nil, err
	}
	if !doctor.Ready {
		return nil, fmt.Errorf("workspace is not ready: %v", doctor.Errors)
	}

	paths, err := filepath.Glob(filepath.Join(toolchainInstallExecutionDir(root), "*.json"))
	if err != nil {
		return nil, fmt.Errorf("glob install executions: %w", err)
	}
	sort.Strings(paths)
	executions := make([]ToolchainInstallDispatchExecution, 0, len(paths))
	for _, path := range paths {
		execution, err := readToolchainInstallDispatchExecutionFile(path)
		if err != nil {
			return nil, err
		}
		executions = append(executions, execution)
	}
	return executions, nil
}

func ReadToolchainInstallDispatchExecution(root, executionPath string) (ToolchainInstallDispatchExecution, error) {
	doctor, err := doctorWorkspaceMetadataOnly(root)
	if err != nil {
		return ToolchainInstallDispatchExecution{}, err
	}
	if !doctor.Ready {
		return ToolchainInstallDispatchExecution{}, fmt.Errorf("workspace is not ready: %v", doctor.Errors)
	}
	path, err := resolveToolchainInstallExecutionPath(root, executionPath)
	if err != nil {
		return ToolchainInstallDispatchExecution{}, err
	}
	return readToolchainInstallDispatchExecutionFile(path)
}

func readToolchainInstallDispatchExecutionFile(path string) (ToolchainInstallDispatchExecution, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return ToolchainInstallDispatchExecution{}, fmt.Errorf("read install execution %s: %w", path, err)
	}
	var execution ToolchainInstallDispatchExecution
	if err := json.Unmarshal(data, &execution); err != nil {
		return ToolchainInstallDispatchExecution{}, fmt.Errorf("decode install execution %s: %w", path, err)
	}
	execution.Path = path
	return execution, nil
}

func toolchainInstallDispatchPlanFromRequests(requests []ToolchainInstallRequest) ToolchainInstallDispatchPlan {
	plan := ToolchainInstallDispatchPlan{
		Summary: SummarizeToolchainInstallRequests(requests),
	}
	requestByDependency := map[string]ToolchainInstallRequest{}
	for _, request := range requests {
		requestByDependency[toolchainInstallRequestDependencyKey(request.Pack, request.Dependency)] = request
	}
	skipped := map[string]bool{}
	emitted := map[string]bool{}
	waiting := map[string]ToolchainInstallRequest{}
	for _, request := range requests {
		status := installRequestDisplayStatus(request.Status)
		key := toolchainInstallRequestKey(request)
		if status != "approved" {
			plan.Skipped = append(plan.Skipped, ToolchainInstallDispatchSkip{
				Request: request,
				Reason:  "not-approved: " + status,
			})
			skipped[key] = true
			continue
		}
		if reason := toolchainInstallMissingOrUnapprovedDependencyReason(request, requestByDependency); reason != "" {
			plan.Skipped = append(plan.Skipped, ToolchainInstallDispatchSkip{Request: request, Reason: reason})
			skipped[key] = true
			continue
		}
		waiting[key] = request
	}
	for len(waiting) > 0 {
		progress := false
		for _, request := range requests {
			key := toolchainInstallRequestKey(request)
			if _, ok := waiting[key]; !ok {
				continue
			}
			if reason := toolchainInstallBlockedDependencyReason(request, requestByDependency, skipped, emitted); reason != "" {
				plan.Skipped = append(plan.Skipped, ToolchainInstallDispatchSkip{Request: request, Reason: reason})
				skipped[key] = true
				delete(waiting, key)
				progress = true
				continue
			}
			if !toolchainInstallDependenciesEmitted(request, requestByDependency, emitted) {
				continue
			}
			plan.Ready = append(plan.Ready, request)
			emitted[key] = true
			delete(waiting, key)
			progress = true
		}
		if progress {
			continue
		}
		for _, request := range requests {
			key := toolchainInstallRequestKey(request)
			if _, ok := waiting[key]; !ok {
				continue
			}
			reason := "dependency-not-approved: unresolved status=blocked"
			if len(request.DependsOn) > 0 {
				reason = fmt.Sprintf("dependency-not-approved: %s status=blocked", strings.TrimSpace(request.DependsOn[0]))
			}
			plan.Skipped = append(plan.Skipped, ToolchainInstallDispatchSkip{Request: request, Reason: reason})
			skipped[key] = true
			delete(waiting, key)
		}
	}
	return plan
}

func toolchainInstallMissingOrUnapprovedDependencyReason(request ToolchainInstallRequest, requestsByDependency map[string]ToolchainInstallRequest) string {
	for _, dependency := range request.DependsOn {
		dependency = strings.TrimSpace(dependency)
		if dependency == "" {
			continue
		}
		prerequisite, ok := requestsByDependency[toolchainInstallRequestDependencyKey(request.Pack, dependency)]
		if !ok {
			return fmt.Sprintf("dependency-not-approved: %s status=missing-request", dependency)
		}
		status := installRequestDisplayStatus(prerequisite.Status)
		if status != "approved" {
			return fmt.Sprintf("dependency-not-approved: %s status=%s", dependency, status)
		}
	}
	return ""
}

func toolchainInstallBlockedDependencyReason(request ToolchainInstallRequest, requestsByDependency map[string]ToolchainInstallRequest, skipped map[string]bool, emitted map[string]bool) string {
	for _, dependency := range request.DependsOn {
		dependency = strings.TrimSpace(dependency)
		if dependency == "" {
			continue
		}
		prerequisite, ok := requestsByDependency[toolchainInstallRequestDependencyKey(request.Pack, dependency)]
		if !ok {
			return fmt.Sprintf("dependency-not-approved: %s status=missing-request", dependency)
		}
		prerequisiteKey := toolchainInstallRequestKey(prerequisite)
		status := installRequestDisplayStatus(prerequisite.Status)
		switch {
		case status != "approved":
			return fmt.Sprintf("dependency-not-approved: %s status=%s", dependency, status)
		case skipped[prerequisiteKey]:
			return fmt.Sprintf("dependency-not-approved: %s status=blocked", dependency)
		case !emitted[prerequisiteKey]:
			continue
		}
	}
	return ""
}

func toolchainInstallDependenciesEmitted(request ToolchainInstallRequest, requestsByDependency map[string]ToolchainInstallRequest, emitted map[string]bool) bool {
	for _, dependency := range request.DependsOn {
		dependency = strings.TrimSpace(dependency)
		if dependency == "" {
			continue
		}
		prerequisite, ok := requestsByDependency[toolchainInstallRequestDependencyKey(request.Pack, dependency)]
		if !ok {
			return false
		}
		if !emitted[toolchainInstallRequestKey(prerequisite)] {
			return false
		}
	}
	return true
}

func toolchainInstallRequestKey(request ToolchainInstallRequest) string {
	return strings.Join([]string{
		strings.TrimSpace(request.Path),
		strings.TrimSpace(request.Pack),
		strings.TrimSpace(request.Dependency),
	}, "\x00")
}

func toolchainInstallRequestDependencyKey(pack string, dependency string) string {
	return strings.TrimSpace(pack) + "\x00" + strings.TrimSpace(dependency)
}

func toolchainInstallRetryPlanFromExecutions(executions []ToolchainInstallDispatchExecution) ToolchainInstallRetryPlan {
	var plan ToolchainInstallRetryPlan
	for _, execution := range executions {
		for _, result := range execution.Results {
			status := installRequestDisplayStatus(result.Status)
			plan.Summary.Total++
			if status == "failed" {
				plan.Ready = append(plan.Ready, ToolchainInstallRetryCandidate{
					ExecutionPath: execution.Path,
					DispatchPath:  execution.DispatchPath,
					RequestPath:   result.RequestPath,
					Pack:          result.Pack,
					Dependency:    result.Dependency,
					Command:       append([]string(nil), result.Command...),
					LastStatus:    status,
					LastOutput:    result.Output,
					LastError:     result.Error,
				})
				plan.Summary.Ready++
				continue
			}
			plan.Skipped = append(plan.Skipped, ToolchainInstallRetrySkip{
				ExecutionPath: execution.Path,
				DispatchPath:  execution.DispatchPath,
				RequestPath:   result.RequestPath,
				Pack:          result.Pack,
				Dependency:    result.Dependency,
				Status:        status,
				Reason:        "not-failed: " + status,
			})
			plan.Summary.Skipped++
		}
	}
	return plan
}

func InspectToolchains(root string) ([]ToolchainStatus, error) {
	doctor, err := DoctorWorkspace(root)
	if err != nil {
		return nil, err
	}
	if !doctor.Ready {
		return nil, fmt.Errorf("workspace is not ready: %v", doctor.Errors)
	}

	manifests, err := toolchain.LoadManifests(filepath.Join(root, "toolchain"))
	if err != nil {
		return nil, err
	}
	statuses := make([]ToolchainStatus, 0, len(manifests))
	healthRunner := toolchain.DefaultHealthRunner{}
	dependencyRunner := toolchain.DefaultDependencyRunner{}
	metadataProvider := toolchain.DefaultMetadataProvider{}
	for _, manifest := range manifests {
		dependencies := toolchain.CheckDependenciesWithMetadata(context.Background(), manifest, dependencyRunner, metadataProvider)
		installPlan := toolchain.PlanDependencyInstalls(manifest, dependencies)
		health := toolchain.CheckHealth(manifest, healthRunner)
		health = toolchain.ApplyDependencyHealth(health, dependencies)
		statuses = append(statuses, ToolchainStatus{
			Name:            manifest.Name,
			Version:         manifest.Version,
			Runner:          manifest.Runner,
			Parser:          manifest.Parser,
			CapabilityCount: len(manifest.Capabilities),
			Health:          health,
			Dependencies:    dependencies,
			InstallPlan:     installPlan,
		})
	}
	return statuses, nil
}

func CheckToolchainContracts(root string) (ToolchainContractCheck, error) {
	doctor, err := DoctorWorkspaceWithOptions(root, DoctorOptions{SkipContainerRuntimeCheck: true})
	if err != nil {
		return ToolchainContractCheck{}, err
	}
	if !doctor.Ready {
		return ToolchainContractCheck{}, fmt.Errorf("workspace is not ready: %v", doctor.Errors)
	}

	manifests, err := toolchain.LoadManifests(filepath.Join(root, "toolchain"))
	if err != nil {
		return ToolchainContractCheck{}, err
	}
	catalog := toolchain.BuiltInAppSecTools()
	packs := contracts.BuiltInDomainPacks()
	patternContracts := contracts.BuiltInPatternContracts()
	check := ToolchainContractCheck{
		Status:              "ready",
		Mode:                "read-only",
		ManifestCount:       len(manifests),
		ToolCount:           len(catalog),
		DomainPackCount:     len(packs),
		PatternCount:        len(patternContracts),
		SemanticActionCount: len(compiler.DeclaredSemanticActions()),
	}
	if err := contracts.Validate(contracts.Inputs{
		Manifests:   manifests,
		ToolCatalog: catalog,
		DomainPacks: packs,
		Patterns:    patternContracts,
	}); err != nil {
		check.Status = "blocked"
		check.Errors = splitContractValidationErrors(err)
	}
	return check, nil
}

func splitContractValidationErrors(err error) []string {
	if err == nil {
		return nil
	}
	lines := strings.Split(err.Error(), "\n")
	errors := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		errors = append(errors, line)
	}
	return errors
}

func CreateAuthoredTool(root string, spec toolchain.AuthoredToolSpec) (AuthoredToolArtifact, error) {
	artifact, manifest, err := prepareAuthoredToolArtifact(root, spec, "written")
	if err != nil {
		return AuthoredToolArtifact{}, err
	}
	if err := rejectExistingAuthoredToolDir(artifact.Dir); err != nil {
		return AuthoredToolArtifact{}, err
	}

	specData, err := yaml.Marshal(spec)
	if err != nil {
		return AuthoredToolArtifact{}, fmt.Errorf("encode authored tool spec: %w", err)
	}
	manifestData, err := yaml.Marshal(manifest)
	if err != nil {
		return AuthoredToolArtifact{}, fmt.Errorf("encode authored tool manifest: %w", err)
	}

	if err := os.Mkdir(artifact.Dir, 0o755); err != nil {
		return AuthoredToolArtifact{}, fmt.Errorf("create authored tool directory: %w", err)
	}
	published := false
	defer func() {
		if !published {
			_ = os.RemoveAll(artifact.Dir)
		}
	}()
	if err := requireAuthoredToolDirectory(artifact.Dir, "authored tool directory"); err != nil {
		return AuthoredToolArtifact{}, err
	}
	if err := writeAuthoredToolArtifactFile(artifact.SpecPath, specData, "authored tool spec"); err != nil {
		return AuthoredToolArtifact{}, err
	}
	if err := writeAuthoredToolArtifactFile(artifact.ManifestPath, manifestData, "authored tool manifest"); err != nil {
		return AuthoredToolArtifact{}, err
	}
	published = true
	return artifact, nil
}

func ListAuthoredTools(root string) ([]AuthoredToolArtifact, error) {
	if err := requireAuthoredToolWorkspace(root); err != nil {
		return nil, err
	}
	dir, err := authoredToolchainRoot(root)
	if err != nil {
		return nil, err
	}
	if err := requireAuthoredToolDirectory(dir, "toolchain directory"); err != nil {
		return nil, err
	}
	paths, err := filepath.Glob(filepath.Join(dir, "*", "authored-tool.yaml"))
	if err != nil {
		return nil, fmt.Errorf("glob authored tools: %w", err)
	}
	sort.Strings(paths)
	artifacts := make([]AuthoredToolArtifact, 0, len(paths))
	for _, path := range paths {
		artifact, err := readAuthoredToolFromDir(filepath.Dir(path), "available")
		if err != nil {
			return nil, err
		}
		artifacts = append(artifacts, artifact)
	}
	return artifacts, nil
}

func ReadAuthoredTool(root, name string) (AuthoredToolArtifact, error) {
	if err := requireAuthoredToolWorkspace(root); err != nil {
		return AuthoredToolArtifact{}, err
	}
	dir, err := authoredToolDir(root, name)
	if err != nil {
		return AuthoredToolArtifact{}, err
	}
	return readAuthoredToolFromDir(dir, "available")
}

func prepareAuthoredToolArtifact(root string, spec toolchain.AuthoredToolSpec, status string) (AuthoredToolArtifact, toolchain.Manifest, error) {
	if err := requireAuthoredToolWorkspace(root); err != nil {
		return AuthoredToolArtifact{}, toolchain.Manifest{}, err
	}
	manifest, err := toolchain.AuthoredToolToManifest(spec)
	if err != nil {
		return AuthoredToolArtifact{}, toolchain.Manifest{}, err
	}
	dir, err := authoredToolDir(root, manifest.Name)
	if err != nil {
		return AuthoredToolArtifact{}, toolchain.Manifest{}, err
	}
	return authoredToolArtifactFromSpec(status, dir, spec, manifest), manifest, nil
}

func readAuthoredToolFromDir(dir string, status string) (AuthoredToolArtifact, error) {
	if err := requireAuthoredToolDirectory(dir, "authored tool directory"); err != nil {
		return AuthoredToolArtifact{}, err
	}
	specPath := filepath.Join(dir, "authored-tool.yaml")
	if err := rejectUnsafeAuthoredToolFile(specPath, "authored tool spec"); err != nil {
		return AuthoredToolArtifact{}, err
	}
	spec, err := toolchain.LoadAuthoredToolSpec(specPath)
	if err != nil {
		return AuthoredToolArtifact{}, err
	}
	manifestPath := filepath.Join(dir, "manifest.yaml")
	if err := rejectUnsafeAuthoredToolFile(manifestPath, "authored tool manifest"); err != nil {
		return AuthoredToolArtifact{}, err
	}
	manifest, err := toolchain.LoadManifest(manifestPath)
	if err != nil {
		return AuthoredToolArtifact{}, err
	}
	expected, err := toolchain.AuthoredToolToManifest(spec)
	if err != nil {
		return AuthoredToolArtifact{}, err
	}
	actual := manifest
	actual.Path = ""
	if !reflect.DeepEqual(expected, actual) {
		return AuthoredToolArtifact{}, fmt.Errorf("authored tool manifest %s does not match authored spec", manifestPath)
	}
	return authoredToolArtifactFromSpec(status, dir, spec, manifest), nil
}

func authoredToolArtifactFromSpec(status string, dir string, spec toolchain.AuthoredToolSpec, manifest toolchain.Manifest) AuthoredToolArtifact {
	return AuthoredToolArtifact{
		Status:       status,
		Name:         manifest.Name,
		Version:      manifest.Version,
		Parser:       manifest.Parser,
		AdapterID:    strings.TrimSpace(spec.Adapter.ID),
		Operation:    strings.TrimSpace(spec.Adapter.Operation),
		Capabilities: append([]string(nil), manifest.Capabilities...),
		Dir:          dir,
		SpecPath:     filepath.Join(dir, "authored-tool.yaml"),
		ManifestPath: filepath.Join(dir, "manifest.yaml"),
	}
}

func authoredToolDir(root, name string) (string, error) {
	safeName, err := validateAuthoredToolName(name)
	if err != nil {
		return "", err
	}
	dir, err := authoredToolchainRoot(root)
	if err != nil {
		return "", err
	}
	candidate := filepath.Join(dir, safeName)
	rel, err := filepath.Rel(dir, candidate)
	if err != nil {
		return "", fmt.Errorf("resolve authored tool relative path: %w", err)
	}
	if rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || filepath.IsAbs(rel) || strings.Contains(rel, string(os.PathSeparator)) {
		return "", fmt.Errorf("authored tool path must stay directly under %s", dir)
	}
	return candidate, nil
}

func authoredToolchainRoot(root string) (string, error) {
	dir, err := filepath.Abs(filepath.Join(root, "toolchain"))
	if err != nil {
		return "", fmt.Errorf("resolve toolchain directory: %w", err)
	}
	return dir, nil
}

func requireAuthoredToolWorkspace(root string) error {
	doctor, err := DoctorWorkspaceWithOptions(root, DoctorOptions{SkipContainerRuntimeCheck: true})
	if err != nil {
		return err
	}
	if !doctor.Ready {
		return fmt.Errorf("workspace is not ready: %v", doctor.Errors)
	}
	return nil
}

func validateAuthoredToolName(name string) (string, error) {
	trimmed := strings.TrimSpace(name)
	switch {
	case trimmed == "":
		return "", fmt.Errorf("authored tool name is required")
	case trimmed != name:
		return "", fmt.Errorf("authored tool name must not contain surrounding whitespace")
	case trimmed == "." || trimmed == "..":
		return "", fmt.Errorf("authored tool name is unsafe: %s", name)
	case filepath.IsAbs(trimmed):
		return "", fmt.Errorf("authored tool name must be a direct path segment: %s", name)
	case strings.Contains(trimmed, "/") || strings.Contains(trimmed, "\\"):
		return "", fmt.Errorf("authored tool name must be a direct path segment: %s", name)
	}
	for _, r := range trimmed {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case r == '.', r == '_', r == '-':
		default:
			return "", fmt.Errorf("authored tool name contains unsupported character %q", r)
		}
	}
	return trimmed, nil
}

func rejectExistingAuthoredToolDir(dir string) error {
	parent := filepath.Dir(dir)
	if err := requireAuthoredToolDirectory(parent, "toolchain directory"); err != nil {
		return err
	}
	info, err := os.Lstat(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("inspect authored tool directory: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("authored tool directory must not be a symlink: %s", dir)
	}
	return fmt.Errorf("authored tool directory already exists: %s", dir)
}

func requireAuthoredToolDirectory(path string, label string) error {
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

func rejectUnsafeAuthoredToolFile(path string, label string) error {
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("inspect %s: %w", label, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("%s must not be a symlink: %s", label, path)
	}
	if info.IsDir() {
		return fmt.Errorf("write %s: path is a directory: %s", label, path)
	}
	return nil
}

func writeAuthoredToolArtifactFile(path string, data []byte, label string) error {
	if err := rejectUnsafeAuthoredToolFile(path, label); err != nil {
		return err
	}
	data = append(append([]byte(nil), data...), '\n')
	tmpPath := path + ".tmp"
	if err := rejectUnsafeAuthoredToolFile(tmpPath, label+" temp file"); err != nil {
		return err
	}
	if err := os.WriteFile(tmpPath, data, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", label, err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("publish %s: %w", label, err)
	}
	return nil
}

type managedToolBundleManifest struct {
	Name    string `yaml:"name"`
	Kind    string `yaml:"kind"`
	Version string `yaml:"version"`
	Source  string `yaml:"source"`
	Payload string `yaml:"payload"`
	SHA256  string `yaml:"sha256"`
}

func ListManagedToolBundles(root string) ([]ManagedToolBundle, error) {
	if err := requireAuthoredToolWorkspace(root); err != nil {
		return nil, err
	}
	dir, err := managedToolBundleRoot(root)
	if err != nil {
		return nil, err
	}
	if err := rejectUnsafeAuthoredToolDirectory(dir, "managed bundle directory"); err != nil {
		return nil, err
	}
	if _, err := os.Lstat(dir); os.IsNotExist(err) {
		return []ManagedToolBundle{}, nil
	}
	paths, err := filepath.Glob(filepath.Join(dir, "*", "bundle.yaml"))
	if err != nil {
		return nil, fmt.Errorf("glob managed tool bundles: %w", err)
	}
	sort.Strings(paths)
	bundles := make([]ManagedToolBundle, 0, len(paths))
	for _, path := range paths {
		bundle, err := readManagedToolBundle(root, filepath.Dir(path))
		if err != nil {
			return nil, err
		}
		bundles = append(bundles, bundle)
	}
	return bundles, nil
}

func CreateManagedToolBundleInstallPlan(root, pack, dependency string) (ManagedToolBundleInstallPlan, error) {
	if err := requireAuthoredToolWorkspace(root); err != nil {
		return ManagedToolBundleInstallPlan{}, err
	}
	declared, err := findManagedInstallDependency(root, pack, dependency)
	if err != nil {
		return ManagedToolBundleInstallPlan{}, err
	}
	bundle, err := ReadManagedToolBundle(root, declared.Name)
	if err != nil {
		return ManagedToolBundleInstallPlan{}, err
	}
	if bundle.Kind != dependencyKindForManagedInstall(declared) {
		return ManagedToolBundleInstallPlan{}, fmt.Errorf("managed bundle kind %s does not match dependency kind %s", bundle.Kind, dependencyKindForManagedInstall(declared))
	}
	return ManagedToolBundleInstallPlan{
		Status:      "ready",
		Mode:        "metadata-only",
		Action:      "link",
		Pack:        strings.TrimSpace(pack),
		Dependency:  strings.TrimSpace(dependency),
		Kind:        bundle.Kind,
		Version:     bundle.Version,
		Source:      bundle.Source,
		SHA256:      bundle.SHA256,
		PayloadPath: bundle.PayloadPath,
		InstallPath: bundle.InstallPath,
	}, nil
}

func ExecuteManagedToolBundleInstall(root, pack, dependency string, options ManagedToolBundleInstallOptions) (ManagedToolBundleInstallResult, error) {
	if !options.Execute {
		return ManagedToolBundleInstallResult{}, fmt.Errorf("managed tool bundle install requires --execute")
	}
	plan, err := CreateManagedToolBundleInstallPlan(root, pack, dependency)
	if err != nil {
		return ManagedToolBundleInstallResult{}, err
	}
	installDir := filepath.Dir(plan.InstallPath)
	if err := ensureAuthoredToolDirectory(installDir, "managed install directory"); err != nil {
		return ManagedToolBundleInstallResult{}, err
	}
	if err := rejectUnsafeAuthoredToolFile(plan.InstallPath, "managed install link"); err != nil {
		return ManagedToolBundleInstallResult{}, err
	}
	if _, err := os.Lstat(plan.InstallPath); err == nil {
		return ManagedToolBundleInstallResult{}, fmt.Errorf("managed install path already exists: %s", plan.InstallPath)
	} else if !os.IsNotExist(err) {
		return ManagedToolBundleInstallResult{}, fmt.Errorf("inspect managed install path: %w", err)
	}
	if err := os.Symlink(plan.PayloadPath, plan.InstallPath); err != nil {
		return ManagedToolBundleInstallResult{}, fmt.Errorf("link managed tool bundle payload: %w", err)
	}
	return ManagedToolBundleInstallResult{
		Status:      "installed",
		Mode:        "workspace-link",
		Action:      plan.Action,
		Pack:        plan.Pack,
		Dependency:  plan.Dependency,
		Kind:        plan.Kind,
		Version:     plan.Version,
		Source:      plan.Source,
		SHA256:      plan.SHA256,
		PayloadPath: plan.PayloadPath,
		InstallPath: plan.InstallPath,
	}, nil
}

func CreateReleaseManagedToolBundleInstallPlan(root, releaseRoot, pack, dependency string) (ReleaseManagedToolBundleInstallPlan, error) {
	if err := requireAuthoredToolWorkspace(root); err != nil {
		return ReleaseManagedToolBundleInstallPlan{}, err
	}
	declared, err := findManagedInstallDependency(root, pack, dependency)
	if err != nil {
		return ReleaseManagedToolBundleInstallPlan{}, err
	}
	releaseBundleDir, err := releaseManagedToolBundleDir(releaseRoot, declared.Name)
	if err != nil {
		return ReleaseManagedToolBundleInstallPlan{}, err
	}
	bundle, err := readManagedToolBundle(root, releaseBundleDir)
	if err != nil {
		return ReleaseManagedToolBundleInstallPlan{}, err
	}
	if bundle.Kind != dependencyKindForManagedInstall(declared) {
		return ReleaseManagedToolBundleInstallPlan{}, fmt.Errorf("release managed bundle kind %s does not match dependency kind %s", bundle.Kind, dependencyKindForManagedInstall(declared))
	}
	workspaceBundleDir := filepath.Join(root, "toolchain", "managed-bundles", bundle.Name)
	workspacePayloadPath, err := releaseWorkspacePayloadPath(bundle.BundleDir, bundle.PayloadPath, workspaceBundleDir)
	if err != nil {
		return ReleaseManagedToolBundleInstallPlan{}, err
	}
	return ReleaseManagedToolBundleInstallPlan{
		Status:               "ready",
		Mode:                 "metadata-only",
		Action:               "copy-and-link",
		Pack:                 strings.TrimSpace(pack),
		Dependency:           strings.TrimSpace(dependency),
		Kind:                 bundle.Kind,
		Version:              bundle.Version,
		Source:               bundle.Source,
		SHA256:               bundle.SHA256,
		ReleaseBundleDir:     bundle.BundleDir,
		ReleasePayloadPath:   bundle.PayloadPath,
		WorkspaceBundleDir:   workspaceBundleDir,
		WorkspacePayloadPath: workspacePayloadPath,
		InstallPath:          bundle.InstallPath,
	}, nil
}

func ExecuteReleaseManagedToolBundleInstall(root, releaseRoot, pack, dependency string, options ManagedToolBundleInstallOptions) (ReleaseManagedToolBundleInstallResult, error) {
	if !options.Execute {
		return ReleaseManagedToolBundleInstallResult{}, fmt.Errorf("release managed tool bundle install requires --execute")
	}
	plan, err := CreateReleaseManagedToolBundleInstallPlan(root, releaseRoot, pack, dependency)
	if err != nil {
		return ReleaseManagedToolBundleInstallResult{}, err
	}
	if err := ensureAuthoredToolDirectory(filepath.Dir(plan.WorkspaceBundleDir), "managed bundle directory"); err != nil {
		return ReleaseManagedToolBundleInstallResult{}, err
	}
	if err := rejectUnsafeAuthoredToolDirectory(plan.WorkspaceBundleDir, "workspace managed bundle directory"); err != nil {
		return ReleaseManagedToolBundleInstallResult{}, err
	}
	if _, err := os.Lstat(plan.WorkspaceBundleDir); err == nil {
		return ReleaseManagedToolBundleInstallResult{}, fmt.Errorf("workspace managed bundle already exists: %s", plan.WorkspaceBundleDir)
	} else if !os.IsNotExist(err) {
		return ReleaseManagedToolBundleInstallResult{}, fmt.Errorf("inspect workspace managed bundle: %w", err)
	}
	if err := copyManagedBundleDirectory(plan.ReleaseBundleDir, plan.WorkspaceBundleDir); err != nil {
		return ReleaseManagedToolBundleInstallResult{}, err
	}
	install, err := ExecuteManagedToolBundleInstall(root, pack, dependency, options)
	if err != nil {
		return ReleaseManagedToolBundleInstallResult{}, err
	}
	return ReleaseManagedToolBundleInstallResult{
		Status:               install.Status,
		Mode:                 "workspace-copy-link",
		Action:               plan.Action,
		Pack:                 plan.Pack,
		Dependency:           plan.Dependency,
		Kind:                 plan.Kind,
		Version:              plan.Version,
		Source:               plan.Source,
		SHA256:               plan.SHA256,
		ReleaseBundleDir:     plan.ReleaseBundleDir,
		ReleasePayloadPath:   plan.ReleasePayloadPath,
		WorkspaceBundleDir:   plan.WorkspaceBundleDir,
		WorkspacePayloadPath: plan.WorkspacePayloadPath,
		InstallPath:          install.InstallPath,
	}, nil
}

func ReadManagedToolBundle(root, name string) (ManagedToolBundle, error) {
	if err := requireAuthoredToolWorkspace(root); err != nil {
		return ManagedToolBundle{}, err
	}
	safeName, err := validateAuthoredToolName(name)
	if err != nil {
		return ManagedToolBundle{}, err
	}
	dir, err := managedToolBundleRoot(root)
	if err != nil {
		return ManagedToolBundle{}, err
	}
	return readManagedToolBundle(root, filepath.Join(dir, safeName))
}

func readManagedToolBundle(root, dir string) (ManagedToolBundle, error) {
	if err := requireAuthoredToolDirectory(dir, "managed bundle directory"); err != nil {
		return ManagedToolBundle{}, err
	}
	manifestPath := filepath.Join(dir, "bundle.yaml")
	if err := rejectUnsafeAuthoredToolFile(manifestPath, "managed bundle manifest"); err != nil {
		return ManagedToolBundle{}, err
	}
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return ManagedToolBundle{}, fmt.Errorf("read managed bundle manifest %s: %w", manifestPath, err)
	}
	var manifest managedToolBundleManifest
	if err := yaml.Unmarshal(data, &manifest); err != nil {
		return ManagedToolBundle{}, fmt.Errorf("parse managed bundle manifest %s: %w", manifestPath, err)
	}
	if err := validateManagedToolBundleManifest(manifest, filepath.Base(dir)); err != nil {
		return ManagedToolBundle{}, err
	}
	payloadPath, err := resolveManagedBundlePayloadPath(dir, manifest.Payload)
	if err != nil {
		return ManagedToolBundle{}, err
	}
	if err := validateManagedBundlePayload(payloadPath, manifest); err != nil {
		return ManagedToolBundle{}, err
	}
	installPath, err := managedToolBundleInstallPath(root, manifest)
	if err != nil {
		return ManagedToolBundle{}, err
	}
	return ManagedToolBundle{
		Status:       "available",
		Name:         strings.TrimSpace(manifest.Name),
		Kind:         strings.TrimSpace(manifest.Kind),
		Version:      strings.TrimSpace(manifest.Version),
		Source:       strings.TrimSpace(manifest.Source),
		SHA256:       strings.TrimSpace(manifest.SHA256),
		BundleDir:    dir,
		ManifestPath: manifestPath,
		PayloadPath:  payloadPath,
		InstallPath:  installPath,
	}, nil
}

func validateManagedToolBundleManifest(manifest managedToolBundleManifest, dirName string) error {
	name, err := validateAuthoredToolName(manifest.Name)
	if err != nil {
		return err
	}
	if name != dirName {
		return fmt.Errorf("managed bundle name %s does not match directory %s", name, dirName)
	}
	switch strings.TrimSpace(manifest.Kind) {
	case "binary", "rule_bundle":
	default:
		return fmt.Errorf("managed bundle kind is unsupported: %s", manifest.Kind)
	}
	if strings.TrimSpace(manifest.Version) == "" {
		return fmt.Errorf("managed bundle version is required")
	}
	if strings.TrimSpace(manifest.Payload) == "" {
		return fmt.Errorf("managed bundle payload is required")
	}
	if strings.TrimSpace(manifest.Kind) == "binary" || strings.TrimSpace(manifest.Kind) == "rule_bundle" {
		digest := strings.TrimSpace(manifest.SHA256)
		if digest == "" {
			return fmt.Errorf("managed %s bundle sha256 is required", strings.TrimSpace(manifest.Kind))
		}
		decoded, err := hex.DecodeString(digest)
		if err != nil || len(decoded) != sha256.Size {
			return fmt.Errorf("managed %s bundle sha256 is invalid", strings.TrimSpace(manifest.Kind))
		}
	}
	return nil
}

func resolveManagedBundlePayloadPath(bundleDir, payload string) (string, error) {
	trimmed := strings.TrimSpace(payload)
	if filepath.IsAbs(trimmed) {
		return "", fmt.Errorf("managed bundle payload path must stay under %s", bundleDir)
	}
	base, err := filepath.Abs(bundleDir)
	if err != nil {
		return "", fmt.Errorf("resolve managed bundle directory: %w", err)
	}
	candidate, err := filepath.Abs(filepath.Join(base, trimmed))
	if err != nil {
		return "", fmt.Errorf("resolve managed bundle payload: %w", err)
	}
	rel, err := filepath.Rel(base, candidate)
	if err != nil {
		return "", fmt.Errorf("resolve managed bundle payload relative path: %w", err)
	}
	if rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("managed bundle payload path must stay under %s", base)
	}
	return candidate, nil
}

func validateManagedBundlePayload(path string, manifest managedToolBundleManifest) error {
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("managed bundle payload missing: %s", path)
		}
		return fmt.Errorf("inspect managed bundle payload: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("managed bundle payload must not be a symlink: %s", path)
	}
	if strings.TrimSpace(manifest.Kind) == "binary" && info.IsDir() {
		return fmt.Errorf("managed binary payload is a directory: %s", path)
	}
	if strings.TrimSpace(manifest.Kind) == "rule_bundle" && !info.IsDir() {
		return fmt.Errorf("managed rule_bundle payload is not a directory: %s", path)
	}
	if strings.TrimSpace(manifest.SHA256) != "" && info.IsDir() {
		actual, err := digestManagedBundleDirectory(path)
		if err != nil {
			return err
		}
		if actual != strings.TrimSpace(manifest.SHA256) {
			return fmt.Errorf("managed bundle payload sha256 mismatch: %s", path)
		}
	}
	if strings.TrimSpace(manifest.SHA256) != "" && !info.IsDir() {
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read managed bundle payload for digest: %w", err)
		}
		sum := sha256.Sum256(data)
		actual := hex.EncodeToString(sum[:])
		if actual != strings.TrimSpace(manifest.SHA256) {
			return fmt.Errorf("managed bundle payload sha256 mismatch: %s", path)
		}
	}
	return nil
}

func releaseManagedToolBundleDir(releaseRoot, name string) (string, error) {
	if strings.TrimSpace(releaseRoot) == "" {
		return "", fmt.Errorf("release root is required")
	}
	safeName, err := validateAuthoredToolName(name)
	if err != nil {
		return "", err
	}
	root, err := filepath.Abs(filepath.Join(releaseRoot, "toolchains", "managed-bundles"))
	if err != nil {
		return "", fmt.Errorf("resolve release managed bundle root: %w", err)
	}
	if err := rejectUnsafeAuthoredToolDirectory(root, "release managed bundle root"); err != nil {
		return "", err
	}
	return filepath.Join(root, safeName), nil
}

func releaseWorkspacePayloadPath(releaseBundleDir, releasePayloadPath, workspaceBundleDir string) (string, error) {
	rel, err := filepath.Rel(releaseBundleDir, releasePayloadPath)
	if err != nil {
		return "", fmt.Errorf("resolve release payload relative path: %w", err)
	}
	if rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("release managed payload path must stay under %s", releaseBundleDir)
	}
	return filepath.Join(workspaceBundleDir, rel), nil
}

func copyManagedBundleDirectory(src, dst string) error {
	if err := requireAuthoredToolDirectory(src, "release managed bundle directory"); err != nil {
		return err
	}
	if err := filepath.WalkDir(src, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return fmt.Errorf("resolve managed bundle copy path: %w", err)
		}
		if rel == "." {
			return nil
		}
		if rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || filepath.IsAbs(rel) {
			return fmt.Errorf("managed bundle copy path must stay under %s", src)
		}
		target := filepath.Join(dst, rel)
		info, err := os.Lstat(path)
		if err != nil {
			return fmt.Errorf("inspect managed bundle source: %w", err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("managed bundle source must not contain symlinks: %s", path)
		}
		if entry.IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return fmt.Errorf("create managed bundle target directory: %w", err)
			}
			return nil
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("managed bundle source must contain only regular files: %s", path)
		}
		if err := copyManagedBundleFile(path, target, info.Mode().Perm()); err != nil {
			return err
		}
		return nil
	}); err != nil {
		_ = os.RemoveAll(dst)
		return err
	}
	return nil
}

func copyManagedBundleFile(src, dst string, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return fmt.Errorf("create managed bundle target file directory: %w", err)
	}
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open managed bundle source file: %w", err)
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return fmt.Errorf("create managed bundle target file: %w", err)
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return fmt.Errorf("copy managed bundle target file: %w", err)
	}
	if err := out.Chmod(mode); err != nil {
		return fmt.Errorf("chmod managed bundle target file: %w", err)
	}
	return nil
}

func digestManagedBundleDirectory(dir string) (string, error) {
	paths := []string{}
	if err := filepath.WalkDir(dir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := os.Lstat(path)
		if err != nil {
			return fmt.Errorf("inspect managed bundle digest path: %w", err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("managed bundle digest path must not be a symlink: %s", path)
		}
		if entry.IsDir() {
			return nil
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("managed bundle digest path must be a regular file: %s", path)
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return fmt.Errorf("resolve managed bundle digest path: %w", err)
		}
		paths = append(paths, filepath.ToSlash(rel))
		return nil
	}); err != nil {
		return "", err
	}
	sort.Strings(paths)
	hash := sha256.New()
	for _, rel := range paths {
		data, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(rel)))
		if err != nil {
			return "", fmt.Errorf("read managed bundle digest file: %w", err)
		}
		hash.Write([]byte(rel))
		hash.Write([]byte{0})
		hash.Write(data)
		hash.Write([]byte{0})
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func findManagedInstallDependency(root, pack, dependency string) (toolchain.Dependency, error) {
	manifests, err := toolchain.LoadManifests(filepath.Join(root, "toolchain"))
	if err != nil {
		return toolchain.Dependency{}, err
	}
	for _, manifest := range manifests {
		if manifest.Name != strings.TrimSpace(pack) {
			continue
		}
		for _, declared := range manifest.Dependencies {
			if strings.TrimSpace(declared.Name) == strings.TrimSpace(dependency) {
				return declared, nil
			}
		}
		return toolchain.Dependency{}, fmt.Errorf("toolchain dependency not found: %s/%s", pack, dependency)
	}
	return toolchain.Dependency{}, fmt.Errorf("toolchain pack not found: %s", pack)
}

func managedToolBundleRoot(root string) (string, error) {
	dir, err := filepath.Abs(filepath.Join(root, "toolchain", "managed-bundles"))
	if err != nil {
		return "", fmt.Errorf("resolve managed bundle directory: %w", err)
	}
	return dir, nil
}

func managedToolBundleInstallPath(root string, manifest managedToolBundleManifest) (string, error) {
	name, err := validateAuthoredToolName(manifest.Name)
	if err != nil {
		return "", err
	}
	switch strings.TrimSpace(manifest.Kind) {
	case "binary":
		return filepath.Join(root, "toolchain", "bin", name), nil
	case "rule_bundle":
		return filepath.Join(root, "toolchain", "rules", name), nil
	default:
		return "", fmt.Errorf("managed bundle kind is unsupported: %s", manifest.Kind)
	}
}

func dependencyKindForManagedInstall(dependency toolchain.Dependency) string {
	if strings.TrimSpace(dependency.Kind) == "" {
		return "binary"
	}
	return strings.TrimSpace(dependency.Kind)
}

func rejectUnsafeAuthoredToolDirectory(path string, label string) error {
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
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

func ensureAuthoredToolDirectory(path string, label string) error {
	if err := rejectUnsafeAuthoredToolDirectory(path, label); err != nil {
		return err
	}
	if err := os.MkdirAll(path, 0o755); err != nil {
		return fmt.Errorf("create %s: %w", label, err)
	}
	return requireAuthoredToolDirectory(path, label)
}

func CreateToolchainInstallRequest(root, pack, dependency string) (ToolchainInstallRequest, error) {
	statuses, err := InspectToolchains(root)
	if err != nil {
		return ToolchainInstallRequest{}, err
	}

	for _, status := range statuses {
		if status.Name != pack {
			continue
		}
		for _, step := range status.InstallPlan {
			if step.Name != dependency {
				continue
			}
			request := ToolchainInstallRequest{
				Status:      "pending",
				Pack:        status.Name,
				Dependency:  step.Name,
				Kind:        step.Kind,
				State:       string(step.State),
				Required:    step.Required,
				Command:     append([]string(nil), step.Command...),
				Hint:        step.Hint,
				Source:      step.Source,
				ImageDigest: step.ImageDigest,
				DependsOn:   append([]string(nil), step.DependsOn...),
			}
			dir := toolchainInstallRequestDir(root)
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return ToolchainInstallRequest{}, fmt.Errorf("create install request directory: %w", err)
			}
			request.Path = filepath.Join(dir, safeInstallRequestName(pack+"-"+dependency)+".json")
			data, err := json.MarshalIndent(request, "", "  ")
			if err != nil {
				return ToolchainInstallRequest{}, fmt.Errorf("encode install request: %w", err)
			}
			data = append(data, '\n')
			if err := os.WriteFile(request.Path, data, 0o644); err != nil {
				return ToolchainInstallRequest{}, fmt.Errorf("write install request: %w", err)
			}
			return request, nil
		}
		return ToolchainInstallRequest{}, fmt.Errorf("install plan step not found for %s/%s", pack, dependency)
	}
	return ToolchainInstallRequest{}, fmt.Errorf("toolchain pack not found: %s", pack)
}

func ListToolchainInstallRequests(root string) ([]ToolchainInstallRequest, error) {
	doctor, err := doctorWorkspaceMetadataOnly(root)
	if err != nil {
		return nil, err
	}
	if !doctor.Ready {
		return nil, fmt.Errorf("workspace is not ready: %v", doctor.Errors)
	}

	paths, err := filepath.Glob(filepath.Join(toolchainInstallRequestDir(root), "*.json"))
	if err != nil {
		return nil, fmt.Errorf("glob install requests: %w", err)
	}
	sort.Strings(paths)
	requests := make([]ToolchainInstallRequest, 0, len(paths))
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read install request %s: %w", path, err)
		}
		var request ToolchainInstallRequest
		if err := json.Unmarshal(data, &request); err != nil {
			return nil, fmt.Errorf("decode install request %s: %w", path, err)
		}
		request.Path = path
		requests = append(requests, request)
	}
	return requests, nil
}

func UpdateToolchainInstallRequestStatus(root, requestPath, status string) (ToolchainInstallRequest, error) {
	nextStatus, err := normalizeInstallRequestStatus(status)
	if err != nil {
		return ToolchainInstallRequest{}, err
	}
	doctor, err := DoctorWorkspace(root)
	if err != nil {
		return ToolchainInstallRequest{}, err
	}
	if !doctor.Ready {
		return ToolchainInstallRequest{}, fmt.Errorf("workspace is not ready: %v", doctor.Errors)
	}
	path, err := resolveToolchainInstallRequestPath(root, requestPath)
	if err != nil {
		return ToolchainInstallRequest{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ToolchainInstallRequest{}, fmt.Errorf("read install request %s: %w", path, err)
	}
	var request ToolchainInstallRequest
	if err := json.Unmarshal(data, &request); err != nil {
		return ToolchainInstallRequest{}, fmt.Errorf("decode install request %s: %w", path, err)
	}
	request.Status = nextStatus
	request.Path = path
	encoded, err := json.MarshalIndent(request, "", "  ")
	if err != nil {
		return ToolchainInstallRequest{}, fmt.Errorf("encode install request: %w", err)
	}
	encoded = append(encoded, '\n')
	if err := os.WriteFile(path, encoded, 0o644); err != nil {
		return ToolchainInstallRequest{}, fmt.Errorf("write install request %s: %w", path, err)
	}
	return request, nil
}

func normalizeInstallRequestStatus(status string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "approved":
		return "approved", nil
	case "denied":
		return "denied", nil
	default:
		return "", fmt.Errorf("unsupported install request status: %s", status)
	}
}

func installRequestDisplayStatus(status string) string {
	normalized := strings.ToLower(strings.TrimSpace(status))
	if normalized == "" {
		return "unknown"
	}
	return normalized
}

func resolveToolchainInstallRequestPath(root, requestPath string) (string, error) {
	trimmed := strings.TrimSpace(requestPath)
	if trimmed == "" {
		return "", fmt.Errorf("install request path is required")
	}
	dir, err := filepath.Abs(toolchainInstallRequestDir(root))
	if err != nil {
		return "", fmt.Errorf("resolve install request directory: %w", err)
	}
	candidate := trimmed
	if !filepath.IsAbs(candidate) {
		candidate = filepath.Join(dir, candidate)
	}
	candidate, err = filepath.Abs(candidate)
	if err != nil {
		return "", fmt.Errorf("resolve install request path: %w", err)
	}
	rel, err := filepath.Rel(dir, candidate)
	if err != nil {
		return "", fmt.Errorf("resolve install request relative path: %w", err)
	}
	if rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("install request path must stay under %s", dir)
	}
	if filepath.Ext(candidate) != ".json" {
		return "", fmt.Errorf("install request path must be a json file")
	}
	return candidate, nil
}

func resolveToolchainInstallDispatchPath(root, dispatchPath string) (string, error) {
	trimmed := strings.TrimSpace(dispatchPath)
	if trimmed == "" {
		return "", fmt.Errorf("install dispatch path is required")
	}
	dir, err := filepath.Abs(toolchainInstallDispatchDir(root))
	if err != nil {
		return "", fmt.Errorf("resolve install dispatch directory: %w", err)
	}
	candidate := trimmed
	if !filepath.IsAbs(candidate) {
		candidate = filepath.Join(dir, candidate)
	}
	candidate, err = filepath.Abs(candidate)
	if err != nil {
		return "", fmt.Errorf("resolve install dispatch path: %w", err)
	}
	rel, err := filepath.Rel(dir, candidate)
	if err != nil {
		return "", fmt.Errorf("resolve install dispatch relative path: %w", err)
	}
	if rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("install dispatch path must stay under %s", dir)
	}
	if filepath.Ext(candidate) != ".json" {
		return "", fmt.Errorf("install dispatch path must be a json file")
	}
	return candidate, nil
}

func resolveToolCompilePreviewReviewPath(root, reviewPath string) (string, error) {
	trimmed := strings.TrimSpace(reviewPath)
	if trimmed == "" {
		return "", fmt.Errorf("compile preview review path is required")
	}
	dir, err := filepath.Abs(toolCompilePreviewReviewDir(root))
	if err != nil {
		return "", fmt.Errorf("resolve compile preview review directory: %w", err)
	}
	candidate := trimmed
	if !filepath.IsAbs(candidate) {
		candidate = filepath.Join(dir, candidate)
	}
	candidate, err = filepath.Abs(candidate)
	if err != nil {
		return "", fmt.Errorf("resolve compile preview review path: %w", err)
	}
	rel, err := filepath.Rel(dir, candidate)
	if err != nil {
		return "", fmt.Errorf("resolve compile preview review relative path: %w", err)
	}
	if rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("compile preview review path must stay under %s", dir)
	}
	if filepath.Ext(candidate) != ".json" {
		return "", fmt.Errorf("compile preview review path must be a json file")
	}
	return candidate, nil
}

func resolveToolchainInstallExecutionPath(root, executionPath string) (string, error) {
	trimmed := strings.TrimSpace(executionPath)
	if trimmed == "" {
		return "", fmt.Errorf("install execution path is required")
	}
	dir, err := filepath.Abs(toolchainInstallExecutionDir(root))
	if err != nil {
		return "", fmt.Errorf("resolve install execution directory: %w", err)
	}
	candidate := trimmed
	if !filepath.IsAbs(candidate) {
		candidate = filepath.Join(dir, candidate)
	}
	candidate, err = filepath.Abs(candidate)
	if err != nil {
		return "", fmt.Errorf("resolve install execution path: %w", err)
	}
	rel, err := filepath.Rel(dir, candidate)
	if err != nil {
		return "", fmt.Errorf("resolve install execution relative path: %w", err)
	}
	if rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("install execution path must stay under %s", dir)
	}
	if filepath.Ext(candidate) != ".json" {
		return "", fmt.Errorf("install execution path must be a json file")
	}
	return candidate, nil
}

func resolveToolchainInstallRetryDispatchPath(root, dispatchPath string) (string, error) {
	trimmed := strings.TrimSpace(dispatchPath)
	if trimmed == "" {
		return "", fmt.Errorf("install retry dispatch path is required")
	}
	dir, err := filepath.Abs(toolchainInstallRetryDispatchDir(root))
	if err != nil {
		return "", fmt.Errorf("resolve install retry dispatch directory: %w", err)
	}
	candidate := trimmed
	if !filepath.IsAbs(candidate) {
		candidate = filepath.Join(dir, candidate)
	}
	candidate, err = filepath.Abs(candidate)
	if err != nil {
		return "", fmt.Errorf("resolve install retry dispatch path: %w", err)
	}
	rel, err := filepath.Rel(dir, candidate)
	if err != nil {
		return "", fmt.Errorf("resolve install retry dispatch relative path: %w", err)
	}
	if rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("install retry dispatch path must stay under %s", dir)
	}
	if filepath.Ext(candidate) != ".json" {
		return "", fmt.Errorf("install retry dispatch path must be a json file")
	}
	return candidate, nil
}

func resolveToolchainInstallRetryExecutionPath(root, executionPath string) (string, error) {
	trimmed := strings.TrimSpace(executionPath)
	if trimmed == "" {
		return "", fmt.Errorf("install retry execution path is required")
	}
	dir, err := filepath.Abs(toolchainInstallRetryExecutionDir(root))
	if err != nil {
		return "", fmt.Errorf("resolve install retry execution directory: %w", err)
	}
	candidate := trimmed
	if !filepath.IsAbs(candidate) {
		candidate = filepath.Join(dir, candidate)
	}
	candidate, err = filepath.Abs(candidate)
	if err != nil {
		return "", fmt.Errorf("resolve install retry execution path: %w", err)
	}
	rel, err := filepath.Rel(dir, candidate)
	if err != nil {
		return "", fmt.Errorf("resolve install retry execution relative path: %w", err)
	}
	if rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("install retry execution path must stay under %s", dir)
	}
	if filepath.Ext(candidate) != ".json" {
		return "", fmt.Errorf("install retry execution path must be a json file")
	}
	return candidate, nil
}

func toolchainInstallRequestDir(root string) string {
	return filepath.Join(root, "toolchain", "install-requests")
}

func toolCompilePreviewReviewDir(root string) string {
	return filepath.Join(root, "toolchain", "compile-preview-reviews")
}

func toolCompilePreviewReviewSelectionPath(root string) string {
	return filepath.Join(root, "toolchain", "compile-preview-review-selection.json")
}

func toolchainInstallDispatchDir(root string) string {
	return filepath.Join(root, "toolchain", "install-dispatches")
}

func toolchainInstallExecutionDir(root string) string {
	return filepath.Join(root, "toolchain", "install-executions")
}

func toolchainInstallRetryDispatchDir(root string) string {
	return filepath.Join(root, "toolchain", "install-retry-dispatches")
}

func toolchainInstallRetryExecutionDir(root string) string {
	return filepath.Join(root, "toolchain", "install-retry-executions")
}

func safeInstallDispatchName(plan ToolchainInstallDispatchPlan) string {
	parts := []string{"install-dispatch", plan.Summary.Line()}
	for _, request := range plan.Ready {
		parts = append(parts, request.Path, request.Status, request.Pack, request.Dependency)
	}
	for _, skipped := range plan.Skipped {
		request := skipped.Request
		parts = append(parts, request.Path, request.Status, request.Pack, request.Dependency, skipped.Reason)
	}
	return safeInstallRequestName(strings.Join(parts, "|"))
}

func safeToolCompilePreviewReviewName(review ToolCompilePreviewReview) string {
	parts := []string{
		"compile-preview-review",
		string(review.Preview.Action),
		fmt.Sprintf("nodes=%d warnings=%d", len(review.Preview.Nodes), len(review.Preview.Warnings)),
	}
	for _, warning := range review.Preview.Warnings {
		parts = append(parts, warning)
	}
	for _, node := range review.Preview.Nodes {
		parts = append(
			parts,
			node.ID,
			node.ManifestName,
			node.ToolID,
			node.AdapterID,
			node.Operation,
			node.ParserID,
			node.TargetKind,
			node.DependencyName,
			node.DependencyKind,
			node.Capability,
			node.Runner,
			node.Parser,
			strconv.FormatFloat(node.SelectionScore, 'g', -1, 64),
			strings.Join(node.SelectionSignals, ","),
			strings.Join(node.RunCommand, "\x00"),
		)
	}
	return safeInstallRequestName(strings.Join(parts, "|"))
}

func safeInstallRetryDispatchName(plan ToolchainInstallRetryPlan) string {
	parts := []string{
		"install-retry-dispatch",
		fmt.Sprintf("total=%d ready=%d skipped=%d", plan.Summary.Total, plan.Summary.Ready, plan.Summary.Skipped),
	}
	for _, candidate := range plan.Ready {
		parts = append(parts, candidate.ExecutionPath, candidate.DispatchPath, candidate.RequestPath, candidate.Pack, candidate.Dependency, candidate.LastStatus, candidate.LastError)
	}
	for _, skipped := range plan.Skipped {
		parts = append(parts, skipped.ExecutionPath, skipped.DispatchPath, skipped.RequestPath, skipped.Pack, skipped.Dependency, skipped.Status, skipped.Reason)
	}
	return safeInstallRequestName(strings.Join(parts, "|"))
}

func safeInstallRequestName(value string) string {
	sum := sha256.Sum256([]byte(value))
	digest := hex.EncodeToString(sum[:])[:12]

	var builder strings.Builder
	for _, r := range strings.TrimSpace(value) {
		switch {
		case r >= 'a' && r <= 'z':
			builder.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			builder.WriteRune(unicode.ToLower(r))
		case r >= '0' && r <= '9', r == '.', r == '_', r == '-':
			builder.WriteRune(r)
		default:
			builder.WriteRune('-')
		}
	}
	slug := strings.Trim(builder.String(), "-._")
	if slug == "" {
		slug = "install-request"
	}
	if len(slug) > 80 {
		slug = strings.Trim(slug[:80], "-._")
	}
	return slug + "-" + digest
}
