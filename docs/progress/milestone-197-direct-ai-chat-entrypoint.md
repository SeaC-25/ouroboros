# Milestone 197 - Direct AI Chat Entrypoint

## Summary

Added `ouro chat <workspace>` as a direct security-IDE chat entrypoint. Operators can now open the workspace-bound TUI with the Mission Conversation AI composer already focused instead of starting `run --interactive` or `replay --interactive` and then pressing `i`.

## Behavior

- `cmd/ouro` now registers `chat` as a root command.
- `ouro chat <workspace>` launches the existing workspace-bound Bubble Tea model from `app.NewTUIModelForWorkspace`.
- The command calls `StartAIDialogInput()` before launching the TUI, so the first screen is ready for operator input.
- The command does not start a run, replay a run, print run/replay summaries, or create `replay/events.db`.
- `scripts/smoke_direct_use.sh` now records `chat --help` evidence as `chat-help.txt`, so the built binary smoke proves the direct IDE entrypoint is present.

## Safety Boundary

This milestone does not add autonomous model execution. The chat entrypoint reuses the existing TUI AI dialog submitter, provider configuration, metadata-only tool-plan preview bridge, and operator-reviewed gates. No provider call happens until the operator submits a prompt, and model output still does not execute tools, dispatch workers, contact targets, or select workflow gates without later explicit operator action.

## Validation

RED first proved the root command lacked `chat` and `ouro chat <workspace>` returned an unknown-command error. GREEN validation then passed:

```bash
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./cmd/ouro -run 'Test(RootCommandIncludesProductionCommands|ChatCommandLaunchesAIDialogComposerWithoutRunningWorkspace)' -count=1 -v
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test . -run 'TestDirectUseSmokeScriptExercisesBuiltBinaryPath' -count=1 -v
sh -n scripts/smoke_direct_use.sh
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test . -run 'Test(DirectUseSmokeScriptExercisesBuiltBinaryPath|DocumentationBashBlocksPassShellSyntaxCheck|ReleaseMakefileAndCIEntrypoints)' -count=1 -v
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./cmd/ouro -run 'Test(RootCommandIncludesProductionCommands|ChatCommandLaunchesAIDialogComposerWithoutRunningWorkspace|RunCommandInteractiveStreamsSnapshotsToTUI|ReplayCommandCanLaunchInteractiveTUI)' -count=1 -v
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct make smoke-direct-use
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct make smoke-ide
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./... -count=1
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct make build
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct make doctor-strict
sh -n scripts/smoke_direct_use.sh scripts/smoke_ide.sh scripts/smoke_ai_provider.sh scripts/smoke_production_acceptance.sh scripts/audit_completion.sh scripts/smoke_release_managed.sh scripts/smoke_managed_payloads.sh scripts/smoke_workflow_closure.sh
gofmt -l cmd/ouro/main.go cmd/ouro/main_test.go release_scripts_test.go
git diff --check
```

The first full repository validation run exposed a fixture-only gap: the aggregate IDE smoke fake `ouro` did not yet implement `chat --help`, while the real direct-use smoke had already gained that check. The fake was updated to return the same help text and the aggregate smoke regression now asserts the `chat --help` invocation. The final full repository validation passed with root package runtime about 398s. `make smoke-direct-use` printed `chat: available`; `make smoke-ide` printed `ide-smoke: ok`; `make build` printed `built bin/ouro`; and `make doctor-strict` printed `workspace: ready`.

## Remaining

This improves first-run usability for the security-focused Codex/ClaudeCode direction. It still does not claim 100% completion: true completion requires real operator-owned production evidence, approved attestation, `make audit-completion`, manifest/digest review, and final human/operator acceptance.
