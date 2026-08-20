# TUI AI Dialog Tool Plan Preview Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let the TUI AI dialog turn a safe `semantic_action:` / `ouro_action:` marker in the assistant response into an existing metadata-only model tool-plan preview artifact.

**Architecture:** Reuse the existing model tool-plan preview extraction, compile-preview review creation, artifact recording, Model Runtime rows, and TUI double-confirm acceptance gate. The AI dialog submitter remains advisory and non-executing; it may write only review metadata when the provider response contains a valid declared semantic-action marker.

**Tech Stack:** Go, Bubble Tea TUI model, existing app provider and compile-preview APIs, existing workspace JSON artifact conventions.

---

### Task 1: RED Test For Dialog-Created Preview

**Files:**
- Modify: `internal/app/tui_snapshot_test.go`

- [x] **Step 1: Write the failing test**

Add a test that initializes a workspace, configures an `openai_compatible` test provider, submits an AI dialog prompt through `NewTUIModelForWorkspace`, and has the provider return assistant text containing `semantic_action: enumerate_paths` plus a secret suffix. The test must assert:
- one `ModelToolPlanPreviewArtifact` is written,
- one compile-preview review is written and linked from the artifact,
- no compile-preview execution dispatch or execution is written,
- the TUI view shows a `model tool plan preview:` row with `action=enumerate_paths`,
- the artifact JSON and runtime preview row do not contain the prompt, full assistant secret text, bearer token, or provider response body,
- the row remains an existing `model-tool-plan-preview` runtime action that can be accepted by the existing `x` path.

- [x] **Step 2: Run the focused test and verify RED**

Run:

```bash
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/app -run TestNewTUIModelForWorkspaceAIDialogCreatesModelToolPlanPreview -count=1 -v
```

Expected: FAIL because the AI dialog currently displays the provider response but does not create a model tool-plan preview artifact.

### Task 2: GREEN Implementation

**Files:**
- Modify: `internal/tui/app.go`
- Modify: `internal/tui/conversation.go`
- Modify: `internal/app/model_tool_plan_preview_artifact.go`
- Modify: `internal/app/tui_snapshot.go`

- [x] **Step 1: Extend `tui.AIDialogResult` metadata**

Add optional preview fields to carry the created artifact path, action, marker preview, validity, and status into the in-session conversation rows without persisting prompt or full response text.

- [x] **Step 2: Create preview in workspace submitter only after provider success**

In `tuiAIDialogSubmitter`, call `ExtractModelToolPlanPreview(result.Text)`. If valid, create a metadata-only compile-preview review and call `RecordModelToolPlanPreviewArtifact` with source metadata that still records `AssistantTextPersisted=false`, `ExecutedTools=0`, `WorkerDispatch=false`, and `TargetContact=false`.

- [x] **Step 3: Refresh Model Runtime rows after dialog result**

Add the generated `model tool plan preview:` row to the TUI result update so the user can see and select the existing `x` acceptance path in the same session.

- [x] **Step 4: Run focused GREEN**

Run:

```bash
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/app -run 'TestNewTUIModelForWorkspaceAIDialog(CreatesModelToolPlanPreview|UsesConfiguredProvider)' -count=1 -v
```

Expected: PASS.

### Task 3: Documentation And Milestone

**Files:**
- Modify: `README.md`
- Modify: `docs/development.md`
- Modify: `task_plan.md`
- Modify: `progress.md`
- Add: `docs/progress/milestone-192-tui-ai-dialog-tool-plan-preview.md`

- [x] **Step 1: Document the behavior**

State that the dialog can create metadata-only model tool-plan preview artifacts only from a valid declared semantic-action marker, and that the resulting preview still requires the existing operator acceptance and execution gates.

- [x] **Step 2: Record milestone progress**

Add M192 progress entries and a milestone document with summary, behavior, safety boundary, validation, and remaining final production evidence.

### Task 4: Verification

**Files:**
- No new source files.

- [x] **Step 1: Run focused and adjacent tests**

```bash
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/tui ./internal/app -run 'AIDialog|ModelToolPlanPreview|NewTUIModelForWorkspaceAIDialog' -count=1 -v
```

- [x] **Step 2: Run formatting and hygiene**

```bash
gofmt -w internal/tui/app.go internal/tui/conversation.go internal/app/tui_snapshot.go internal/app/tui_snapshot_test.go
git diff --check
```

- [x] **Step 3: Run broader validation if focused checks pass**

```bash
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/app ./internal/tui ./cmd/ouro -count=1
```
