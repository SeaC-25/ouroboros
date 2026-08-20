package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ouroboros/ouroboros/internal/compiler"
)

func TestAcceptModelToolPlanPreviewArtifactSelectsCompilePreviewReviewMetadataOnly(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	assistantSecret := "RAW_MODEL_INSTRUCTION_SHOULD_NOT_PERSIST"
	review, err := CreateToolCompilePreviewReview(root, ToolCompilePreviewOptions{Action: compiler.ActionEnumeratePaths})
	if err != nil {
		t.Fatalf("create compile preview review: %v", err)
	}
	extraction := ExtractModelToolPlanPreview("ouro_action: enumerate_paths\n" + strings.Repeat("safe padding ", 16) + assistantSecret)
	artifact, err := RecordModelToolPlanPreviewArtifact(root, "run-accept", ProviderGenerationResult{
		ProviderName: "local-openai",
		Model:        "gpt-plan",
	}, "/tmp/model-generation.json", extraction, review, ModelToolPlanPreviewArtifactOptions{
		Now: time.Date(2026, 6, 5, 2, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("record model tool-plan preview artifact: %v", err)
	}

	acceptedAt := time.Date(2026, 6, 5, 2, 5, 0, 0, time.UTC)
	acceptance, err := AcceptModelToolPlanPreviewArtifact(root, filepath.Base(artifact.Path), ModelToolPlanPreviewAcceptanceOptions{
		AcceptedBy: "operator",
		AcceptedAt: acceptedAt,
		Reason:     "reviewed marker and plan",
	})
	if err != nil {
		t.Fatalf("accept model tool-plan preview artifact: %v", err)
	}
	if acceptance.Status != "accepted" || acceptance.ExecutionMode != "metadata-only" || acceptance.Action != compiler.ActionEnumeratePaths {
		t.Fatalf("unexpected acceptance status/action: %#v", acceptance)
	}
	if acceptance.AcceptedBy != "operator" || acceptance.AcceptedAt != "2026-06-05T02:05:00Z" || acceptance.Reason != "reviewed marker and plan" {
		t.Fatalf("unexpected acceptance actor metadata: %#v", acceptance)
	}
	if acceptance.PreviewArtifactPath != artifact.Path || acceptance.ReviewPath != review.Path || acceptance.SelectionPath != filepath.Join(root, "toolchain", "compile-preview-review-selection.json") {
		t.Fatalf("unexpected acceptance links: %#v", acceptance)
	}
	if acceptance.AssistantTextPersisted || acceptance.ExecutedTools != 0 || acceptance.WorkerDispatch || acceptance.TargetContact || acceptance.ExecutionDispatchCreated {
		t.Fatalf("acceptance must remain metadata-only without dispatch/execution side effects: %#v", acceptance)
	}
	if acceptance.AcceptanceDigest == "" || acceptance.ArtifactID == "" || acceptance.Path == "" {
		t.Fatalf("expected acceptance artifact identity/digest, got %#v", acceptance)
	}

	selection, found, err := ReadToolCompilePreviewReviewSelection(root)
	if err != nil {
		t.Fatalf("read compile preview review selection: %v", err)
	}
	if !found || selection.Status != "current" || selection.ReviewPath != review.Path || selection.SelectedBy != "operator" || selection.Reason != "accepted model tool-plan preview: reviewed marker and plan" {
		t.Fatalf("expected current compile preview review selection from acceptance, found=%v selection=%#v", found, selection)
	}
	acceptances, err := ListModelToolPlanPreviewAcceptances(root)
	if err != nil {
		t.Fatalf("list model tool-plan preview acceptances: %v", err)
	}
	if len(acceptances) != 1 || acceptances[0].Path != acceptance.Path {
		t.Fatalf("expected one acceptance artifact, got %#v", acceptances)
	}
	read, err := ReadModelToolPlanPreviewAcceptance(root, filepath.Base(acceptance.Path))
	if err != nil {
		t.Fatalf("read model tool-plan preview acceptance: %v", err)
	}
	if read.AcceptanceDigest != acceptance.AcceptanceDigest || read.SelectionPath != acceptance.SelectionPath {
		t.Fatalf("unexpected read acceptance: %#v", read)
	}
	rawAcceptance, err := os.ReadFile(acceptance.Path)
	if err != nil {
		t.Fatalf("read acceptance json: %v", err)
	}
	for _, forbidden := range []string{assistantSecret, "PROMPT_SHOULD_NOT_PERSIST", "secret-value"} {
		if strings.Contains(string(rawAcceptance), forbidden) {
			t.Fatalf("acceptance leaked forbidden material %q: %s", forbidden, string(rawAcceptance))
		}
	}
	dispatches, err := filepath.Glob(filepath.Join(toolCompilePreviewReviewExecutionDispatchDir(root), "*.json"))
	if err != nil {
		t.Fatalf("glob compile-preview execution dispatches: %v", err)
	}
	if len(dispatches) != 0 {
		t.Fatalf("accepting model tool-plan preview must not create execution dispatches, got %#v", dispatches)
	}
	executions, err := filepath.Glob(filepath.Join(toolCompilePreviewReviewExecutionDir(root), "*.json"))
	if err != nil {
		t.Fatalf("glob compile-preview executions: %v", err)
	}
	if len(executions) != 0 {
		t.Fatalf("accepting model tool-plan preview must not execute review nodes, got %#v", executions)
	}
}

func TestAcceptModelToolPlanPreviewArtifactRejectsInvalidArtifactWithoutSelection(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	review, err := CreateToolCompilePreviewReview(root, ToolCompilePreviewOptions{Action: compiler.ActionEnumeratePaths})
	if err != nil {
		t.Fatalf("create compile preview review: %v", err)
	}
	dir := modelToolPlanPreviewArtifactDirectory(root)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("create artifact dir: %v", err)
	}
	invalid := ModelToolPlanPreviewArtifact{
		SchemaVersion:  1,
		ArtifactID:     "invalid-preview",
		RunID:          "run-invalid",
		Source:         closedLoopPlannerGenerationStageID,
		Status:         "skipped",
		ExecutionMode:  "metadata-only",
		Action:         compiler.SemanticAction("shell_exec"),
		Valid:          false,
		Reason:         "unsupported_semantic_action",
		ReviewPath:     review.Path,
		ReviewNodes:    len(review.Preview.Nodes),
		ReviewWarnings: len(review.Preview.Warnings),
	}
	data, err := json.MarshalIndent(invalid, "", "  ")
	if err != nil {
		t.Fatalf("marshal invalid artifact: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "invalid-preview.json"), append(data, '\n'), 0o644); err != nil {
		t.Fatalf("write invalid artifact: %v", err)
	}

	if _, err := AcceptModelToolPlanPreviewArtifact(root, "invalid-preview.json", ModelToolPlanPreviewAcceptanceOptions{AcceptedBy: "operator"}); err == nil {
		t.Fatalf("expected invalid model tool-plan preview artifact to be rejected")
	}
	if selection, found, err := ReadToolCompilePreviewReviewSelection(root); err != nil || found {
		t.Fatalf("invalid preview acceptance must not write selection, found=%v selection=%#v err=%v", found, selection, err)
	}
	if acceptances, err := ListModelToolPlanPreviewAcceptances(root); err != nil || len(acceptances) != 0 {
		t.Fatalf("invalid preview acceptance must not write audit artifacts, acceptances=%#v err=%v", acceptances, err)
	}
}

func TestAcceptModelToolPlanPreviewArtifactRejectsMismatchedReview(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	review, err := CreateToolCompilePreviewReview(root, ToolCompilePreviewOptions{Action: compiler.ActionDiscoverAssets})
	if err != nil {
		t.Fatalf("create compile preview review: %v", err)
	}
	extraction := ExtractModelToolPlanPreview("ouro_action: enumerate_paths")
	artifact, err := RecordModelToolPlanPreviewArtifact(root, "run-mismatch", ProviderGenerationResult{
		ProviderName: "local-openai",
		Model:        "gpt-plan",
	}, "/tmp/model-generation.json", extraction, review, ModelToolPlanPreviewArtifactOptions{
		Now: time.Date(2026, 6, 5, 2, 10, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("record mismatched model tool-plan preview artifact: %v", err)
	}

	if _, err := AcceptModelToolPlanPreviewArtifact(root, filepath.Base(artifact.Path), ModelToolPlanPreviewAcceptanceOptions{AcceptedBy: "operator"}); err == nil {
		t.Fatalf("expected mismatched compile preview review action to be rejected")
	}
	if selection, found, err := ReadToolCompilePreviewReviewSelection(root); err != nil || found {
		t.Fatalf("mismatched preview acceptance must not write selection, found=%v selection=%#v err=%v", found, selection, err)
	}
}

func TestAcceptModelToolPlanPreviewArtifactRejectsUnsafeArtifactPath(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	if _, err := AcceptModelToolPlanPreviewArtifact(root, filepath.Join("..", "outside.json"), ModelToolPlanPreviewAcceptanceOptions{AcceptedBy: "operator"}); err == nil {
		t.Fatalf("expected unsafe model tool-plan preview artifact path to be rejected")
	}
	if selection, found, err := ReadToolCompilePreviewReviewSelection(root); err != nil || found {
		t.Fatalf("unsafe preview acceptance must not write selection, found=%v selection=%#v err=%v", found, selection, err)
	}
}
