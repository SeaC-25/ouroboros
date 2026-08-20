# Milestone 202 - AI Chat Local Status Command

## Summary

Ouroboros AI chat now supports local workflow-status commands inside the Mission Conversation dialog. Operators can submit `/status`, `/workflow`, or `/workflow-status` and get the current read-only workflow status and next gate without configuring or calling an AI provider.

This improves first-run usability for `ouro chat <workspace>`: a fresh offline workspace can still answer "where am I and what is next?" inside the chat surface.

## Implementation

- Added a local command branch before provider generation in the workspace-bound AI dialog submitter.
- Handled `/status`, `/workflow`, and `/workflow-status` through `SummarizeWorkspaceWorkflow`.
- Rendered a compact conversation response with status, stage, next gate, reason, safety-error count, and `stored=false`.
- Kept normal prompts on the existing provider path with workspace context and current-session history.

## Safety Boundary

- Local status commands do not call providers.
- They do not append provider setup errors.
- They do not create replay events, provider usage records, model-generation artifacts, model tool-plan preview artifacts, compile-preview reviews, execution dispatches, worker artifacts, tool execution, or target contact.
- The existing AI setup guidance row remains visible for fresh offline workspaces, but `/status` itself succeeds locally.

## Validation

- RED first confirmed `/status` on a fresh offline workspace still tried provider generation and rendered an offline-provider error.
- Focused GREEN: `go test ./internal/app -run TestNewTUIModelForWorkspaceAIDialogStatusCommandDoesNotRequireProvider -count=1 -v`.
- Adjacent AI dialog, provider, chat, and workflow-status regression passed.
- Package regression passed with `go test ./internal/app ./internal/tui ./cmd/ouro -count=1`.
- Documentation command-block tests passed.
- Full repository validation passed with `go test ./... -count=1`.
- `make build` printed `built bin/ouro`.
- `make doctor-strict` printed `workspace: ready`.

Completion remains about 99.99%, not 100%, until real operator-owned production evidence, approved attestation, `make audit-completion`, manifest/digest sidecar review, and final human/operator acceptance are complete.
