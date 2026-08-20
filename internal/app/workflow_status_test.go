package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ouroboros/ouroboros/internal/compiler"
)

func TestSummarizeWorkspaceWorkflowRecommendsAcceptingValidModelToolPlanPreview(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	review, preview := createWorkflowStatusPreviewArtifact(t, root)

	status, err := SummarizeWorkspaceWorkflow(root)
	if err != nil {
		t.Fatalf("summarize workflow: %v", err)
	}
	if status.Status != "ready" || status.Mode != "read-only" {
		t.Fatalf("unexpected workflow status: %#v", status)
	}
	if status.NextGate != "accept-model-tool-plan-preview" || status.Reason != "valid-preview-awaits-acceptance" {
		t.Fatalf("expected accept preview recommendation, got %#v", status)
	}
	if status.PreviewPath != preview.Path || status.ReviewPath != review.Path || status.Action != compiler.ActionEnumeratePaths {
		t.Fatalf("unexpected preview/review links: %#v", status)
	}
	if !strings.Contains(status.Command, "provider tool-plan-preview-accept") || !strings.Contains(status.Command, "--accept") {
		t.Fatalf("expected accept command hint, got %#v", status)
	}
	if _, found, err := ReadToolCompilePreviewReviewSelection(root); err != nil || found {
		t.Fatalf("workflow summary must not select reviews, found=%v err=%v", found, err)
	}
	if acceptances, err := ListModelToolPlanPreviewAcceptances(root); err != nil || len(acceptances) != 0 {
		t.Fatalf("workflow summary must not write acceptance artifacts, acceptances=%#v err=%v", acceptances, err)
	}
}

func TestSummarizeWorkspaceWorkflowSurfacesSafetyAuthorizationBeforePreviewAcceptance(t *testing.T) {
	t.Skip("Safety authorization checks disabled")
}

func TestSummarizeWorkspaceWorkflowRecommendsDispatchAfterAcceptedPreview(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	review, preview := createWorkflowStatusPreviewArtifact(t, root)
	acceptance, err := AcceptModelToolPlanPreviewArtifact(root, filepath.Base(preview.Path), ModelToolPlanPreviewAcceptanceOptions{
		AcceptedBy: "operator",
		AcceptedAt: time.Date(2026, 6, 5, 3, 5, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("accept preview: %v", err)
	}

	status, err := SummarizeWorkspaceWorkflow(root)
	if err != nil {
		t.Fatalf("summarize workflow: %v", err)
	}
	if status.NextGate != "create-compile-preview-review-execution-dispatch" || status.Reason != "current-review-awaits-dispatch" {
		t.Fatalf("expected dispatch recommendation, got %#v", status)
	}
	if status.AcceptancePath != acceptance.Path || status.SelectionPath != acceptance.SelectionPath || status.ReviewPath != review.Path {
		t.Fatalf("unexpected accepted workflow links: %#v", status)
	}
	if !strings.Contains(status.Command, "toolchain compile-preview-review-execution-dispatch") {
		t.Fatalf("expected dispatch command hint, got %#v", status)
	}
}

func TestSummarizeWorkspaceWorkflowRecommendsExecutingDispatch(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	_, preview := createWorkflowStatusPreviewArtifact(t, root)
	if _, err := AcceptModelToolPlanPreviewArtifact(root, filepath.Base(preview.Path), ModelToolPlanPreviewAcceptanceOptions{AcceptedBy: "operator"}); err != nil {
		t.Fatalf("accept preview: %v", err)
	}
	dispatch, err := CreateToolCompilePreviewReviewExecutionDispatch(root, preview.ReviewPath, ToolCompilePreviewReviewExecutionDispatchOptions{
		Actor: "operator",
	})
	if err != nil {
		t.Fatalf("create dispatch: %v", err)
	}

	status, err := SummarizeWorkspaceWorkflow(root)
	if err != nil {
		t.Fatalf("summarize workflow: %v", err)
	}
	if status.NextGate != "execute-compile-preview-review-execution-dispatch" || status.Reason != "dispatch-awaits-execution" {
		t.Fatalf("expected execute-dispatch recommendation, got %#v", status)
	}
	if status.DispatchPath != dispatch.Path || status.ReviewPath != preview.ReviewPath {
		t.Fatalf("unexpected dispatch workflow links: %#v", status)
	}
	if !strings.Contains(status.Command, "toolchain execute-compile-preview-review-execution-dispatch") || !strings.Contains(status.Command, "--execute") {
		t.Fatalf("expected execute dispatch command hint, got %#v", status)
	}
}

func TestSummarizeWorkspaceWorkflowSurfacesSafetyAuthorizationBeforeCompileExecution(t *testing.T) {
	t.Skip("Safety authorization checks disabled")
}

func TestSummarizeWorkspaceWorkflowRecommendsReportProjectionDispatchAfterCompileExecution(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "workspace")
	_, preview := createWorkflowStatusPreviewArtifact(t, root)
	if _, err := AcceptModelToolPlanPreviewArtifact(root, filepath.Base(preview.Path), ModelToolPlanPreviewAcceptanceOptions{AcceptedBy: "operator"}); err != nil {
		t.Fatalf("accept preview: %v", err)
	}
	dispatch, err := CreateToolCompilePreviewReviewExecutionDispatch(root, preview.ReviewPath, ToolCompilePreviewReviewExecutionDispatchOptions{
		Actor: "operator",
	})
	if err != nil {
		t.Fatalf("create compile dispatch: %v", err)
	}
	compileExecution, err := ExecuteToolCompilePreviewReviewExecutionDispatch(ctx, root, filepath.Base(dispatch.Path), ToolCompilePreviewReviewExecutionOptions{
		Execute:    true,
		Actor:      "operator",
		ToolRunner: &recordingToolExecutionRunner{},
	})
	if err != nil {
		t.Fatalf("execute compile dispatch: %v", err)
	}
	reportRun, err := RunWorkspaceWithOptions(ctx, root, RunOptions{DeferReportProjection: true})
	if err != nil {
		t.Fatalf("run workspace with deferred report projection: %v", err)
	}

	status, err := SummarizeWorkspaceWorkflow(root)
	if err != nil {
		t.Fatalf("summarize workflow: %v", err)
	}
	if status.NextGate != "create-report-projection-dispatch" || status.Reason != "compile-execution-awaits-report-dispatch" || status.Stage != "report-projection" {
		t.Fatalf("expected report dispatch recommendation after compile execution, got %#v", status)
	}
	if status.ExecutionPath != compileExecution.Path || status.ReportRunID != reportRun.RunID {
		t.Fatalf("unexpected compile/report links: %#v", status)
	}
	if !strings.Contains(status.Command, "report-project-dispatch") || !strings.Contains(status.Command, reportRun.RunID) {
		t.Fatalf("expected report dispatch command hint, got %#v", status)
	}
}

func TestSummarizeWorkspaceWorkflowRecommendsExecutingReportProjectionDispatch(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "workspace")
	_, preview := createWorkflowStatusPreviewArtifact(t, root)
	if _, err := AcceptModelToolPlanPreviewArtifact(root, filepath.Base(preview.Path), ModelToolPlanPreviewAcceptanceOptions{AcceptedBy: "operator"}); err != nil {
		t.Fatalf("accept preview: %v", err)
	}
	dispatch, err := CreateToolCompilePreviewReviewExecutionDispatch(root, preview.ReviewPath, ToolCompilePreviewReviewExecutionDispatchOptions{Actor: "operator"})
	if err != nil {
		t.Fatalf("create compile dispatch: %v", err)
	}
	if _, err := ExecuteToolCompilePreviewReviewExecutionDispatch(ctx, root, filepath.Base(dispatch.Path), ToolCompilePreviewReviewExecutionOptions{
		Execute:    true,
		Actor:      "operator",
		ToolRunner: &recordingToolExecutionRunner{},
	}); err != nil {
		t.Fatalf("execute compile dispatch: %v", err)
	}
	reportRun, err := RunWorkspaceWithOptions(ctx, root, RunOptions{DeferReportProjection: true})
	if err != nil {
		t.Fatalf("run workspace with deferred report projection: %v", err)
	}
	reportDispatch, err := CreateReportProjectionDispatch(ctx, root, reportRun.RunID, ReportProjectionDispatchOptions{Actor: "operator"})
	if err != nil {
		t.Fatalf("create report dispatch: %v", err)
	}

	status, err := SummarizeWorkspaceWorkflow(root)
	if err != nil {
		t.Fatalf("summarize workflow: %v", err)
	}
	if status.NextGate != "execute-report-projection-dispatch" || status.Reason != "report-dispatch-awaits-execution" || status.Stage != "report-projection" {
		t.Fatalf("expected report execution recommendation, got %#v", status)
	}
	if status.ReportDispatchPath != reportDispatch.Path || status.ReportRunID != reportRun.RunID {
		t.Fatalf("unexpected report dispatch links: %#v", status)
	}
	if !strings.Contains(status.Command, "execute-report-project-dispatch") || !strings.Contains(status.Command, "--execute") {
		t.Fatalf("expected report execution command hint, got %#v", status)
	}
}

func TestSummarizeWorkspaceWorkflowDoesNotEmitRunnableProviderPreviewCommandWithoutPrompt(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}

	status, err := SummarizeWorkspaceWorkflow(root)
	if err != nil {
		t.Fatalf("summarize workflow: %v", err)
	}
	if status.NextGate != "run-provider-tool-plan-preview" {
		t.Fatalf("expected provider preview recommendation, got %#v", status)
	}
	if !strings.Contains(status.Command, "--provider-generate-prompt") || !strings.Contains(status.Command, "<prompt>") {
		t.Fatalf("provider preview command must include a prompt placeholder, got %#v", status)
	}
}

func TestSummarizeWorkspaceWorkflowIsReadOnlyAcrossAcceptDispatchReportArtifacts(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "workspace")
	_, preview := createWorkflowStatusPreviewArtifact(t, root)
	if _, err := AcceptModelToolPlanPreviewArtifact(root, filepath.Base(preview.Path), ModelToolPlanPreviewAcceptanceOptions{AcceptedBy: "operator"}); err != nil {
		t.Fatalf("accept preview: %v", err)
	}
	dispatch, err := CreateToolCompilePreviewReviewExecutionDispatch(root, preview.ReviewPath, ToolCompilePreviewReviewExecutionDispatchOptions{Actor: "operator"})
	if err != nil {
		t.Fatalf("create compile dispatch: %v", err)
	}
	if _, err := ExecuteToolCompilePreviewReviewExecutionDispatch(ctx, root, filepath.Base(dispatch.Path), ToolCompilePreviewReviewExecutionOptions{
		Execute:    true,
		Actor:      "operator",
		ToolRunner: &recordingToolExecutionRunner{},
	}); err != nil {
		t.Fatalf("execute compile dispatch: %v", err)
	}
	reportRun, err := RunWorkspaceWithOptions(ctx, root, RunOptions{DeferReportProjection: true})
	if err != nil {
		t.Fatalf("run workspace with deferred report projection: %v", err)
	}
	reportDispatch, err := CreateReportProjectionDispatch(ctx, root, reportRun.RunID, ReportProjectionDispatchOptions{Actor: "operator"})
	if err != nil {
		t.Fatalf("create report dispatch: %v", err)
	}
	beforeAcceptances := workflowStatusGlobCount(t, root, "runtime/model-tool-plan-preview-acceptances/*.json")
	beforeCompileDispatches := workflowStatusGlobCount(t, root, "toolchain/compile-preview-review-execution-dispatches/*.json")
	beforeCompileExecutions := workflowStatusGlobCount(t, root, "toolchain/compile-preview-review-executions/*.json")
	beforeReportDispatches := workflowStatusGlobCount(t, root, "reports/report-projection-dispatches/*.json")
	beforeReportExecutions := workflowStatusGlobCount(t, root, "reports/report-projection-dispatch-executions/*.json")

	status, err := SummarizeWorkspaceWorkflow(root)
	if err != nil {
		t.Fatalf("summarize workflow: %v", err)
	}
	if status.ReportDispatchPath != reportDispatch.Path {
		t.Fatalf("expected existing report dispatch link, got %#v", status)
	}
	if got := workflowStatusGlobCount(t, root, "runtime/model-tool-plan-preview-acceptances/*.json"); got != beforeAcceptances {
		t.Fatalf("summary must not write acceptances: before=%d after=%d", beforeAcceptances, got)
	}
	if got := workflowStatusGlobCount(t, root, "toolchain/compile-preview-review-execution-dispatches/*.json"); got != beforeCompileDispatches {
		t.Fatalf("summary must not write compile dispatches: before=%d after=%d", beforeCompileDispatches, got)
	}
	if got := workflowStatusGlobCount(t, root, "toolchain/compile-preview-review-executions/*.json"); got != beforeCompileExecutions {
		t.Fatalf("summary must not write compile executions: before=%d after=%d", beforeCompileExecutions, got)
	}
	if got := workflowStatusGlobCount(t, root, "reports/report-projection-dispatches/*.json"); got != beforeReportDispatches {
		t.Fatalf("summary must not write report dispatches: before=%d after=%d", beforeReportDispatches, got)
	}
	if got := workflowStatusGlobCount(t, root, "reports/report-projection-dispatch-executions/*.json"); got != beforeReportExecutions {
		t.Fatalf("summary must not write report executions: before=%d after=%d", beforeReportExecutions, got)
	}
}

func createWorkflowStatusPreviewArtifact(t *testing.T, root string) (ToolCompilePreviewReview, ModelToolPlanPreviewArtifact) {
	t.Helper()
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	review, err := CreateToolCompilePreviewReview(root, ToolCompilePreviewOptions{Action: compiler.ActionEnumeratePaths})
	if err != nil {
		t.Fatalf("create compile preview review: %v", err)
	}
	extraction := ExtractModelToolPlanPreview("ouro_action: enumerate_paths\n" + strings.Repeat("padding ", 16))
	preview, err := RecordModelToolPlanPreviewArtifact(root, "run-workflow", ProviderGenerationResult{
		ProviderName: "local-openai",
		Model:        "gpt-plan",
	}, "", extraction, review, ModelToolPlanPreviewArtifactOptions{
		Now: time.Date(2026, 6, 5, 3, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("record preview: %v", err)
	}
	return review, preview
}

func workflowStatusGlobCount(t *testing.T, root, pattern string) int {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(root, filepath.FromSlash(pattern)))
	if err != nil {
		t.Fatalf("glob %s: %v", pattern, err)
	}
	return len(matches)
}

func writeWorkflowStatusDisabledScope(t *testing.T, root string) {
	t.Helper()
	scope := `version: 1
scope:
  target_policy: disabled
  targets:
    - kind: url
      value: https://example.test
  exclusions: []
execution:
  isolation: process
`
	if err := os.WriteFile(filepath.Join(root, "scope.yaml"), []byte(scope), 0o644); err != nil {
		t.Fatalf("write disabled scope: %v", err)
	}
}
