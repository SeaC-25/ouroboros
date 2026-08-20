# Milestone 199: AI Chat Provider Setup Guidance

## Summary

Fresh `ouro chat <workspace>` sessions now explain how to configure the AI provider from inside the IDE instead of showing only an empty composer and then a generic offline-provider error.

Workspace-bound TUI models add an `AI setup:` row to Mission Conversation and Model Runtime. For the default fresh workspace, the row reports `status=needs-provider`, `reason=enabled-provider-offline`, `api_key_value_stored=false`, and the command shape:

```bash
ouro provider configure-openai-compatible <workspace> --name <provider-name> --base-url <https://provider.example/v1> --model <model> --api-key-env <ENV_VAR> --capability chat --capability tool_planning --enable
```

If an operator submits a prompt before configuring a real provider, the AI dialog error repeats the same setup command so the next step is visible without leaving the IDE.

## Safety Boundary

- The setup projection reads local provider metadata and checks environment-variable presence only.
- API key values are not printed, written to `providers.yaml`, written to runtime rows, written to errors, or persisted to usage/model artifacts.
- The setup row is read-only metadata.
- The change does not start runs, create replay events, write provider usage records, write model-generation artifacts, create execution dispatches, dispatch workers, execute tools, call providers by itself, or contact targets.
- Model output remains advisory and still goes through the existing metadata-only tool-plan preview and explicit operator gates.

## Validation

RED first:

```bash
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/app -run TestNewTUIModelForWorkspaceAIDialogShowsProviderSetupGuidance -count=1 -v
```

The test failed because the fresh chat TUI did not contain `AI setup:` guidance.

GREEN:

```bash
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/app -run TestNewTUIModelForWorkspaceAIDialogShowsProviderSetupGuidance -count=1 -v
```

Adjacent regression:

```bash
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/tui ./internal/app ./cmd/ouro -run 'AIDialog|ProviderConfigure|ConfigureOpenAICompatible|GenerateProviderMessageRejectsOfflineProvider|ChatCommand' -count=1 -v
```

Both passed.

Final validation also passed:

```bash
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/app ./internal/tui ./cmd/ouro -count=1
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./... -count=1
make build
make doctor-strict
gofmt -l internal/app/tui_snapshot.go internal/app/tui_snapshot_test.go internal/tui/app.go
git diff --check
rg -n "[ \t]+$" README.md docs/development.md docs/progress/milestone-199-ai-chat-provider-setup-guidance.md findings.md progress.md task_plan.md internal/app/tui_snapshot.go internal/app/tui_snapshot_test.go internal/tui/app.go
```

## Remaining Completion Gate

This milestone improves direct local usability but does not change the final completion gate. True 100% still requires real operator-owned production evidence, approved attestation, `make audit-completion`, manifest/digest sidecar review, and final human/operator acceptance.
