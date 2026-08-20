# Milestone 195 - Local IDE Smoke

## Summary

Added a single local usability smoke for the security-operator IDE path. Operators can now run one command to prove the built binary can start the core offline TUI/replay/workflow-status path and the direct AI provider/manual usage path.

## Behavior

- `make smoke-ide` runs `scripts/smoke_ide.sh`.
- The aggregate script uses built `bin/ouro` by default; `OURO_IDE_SMOKE_BIN` can point to an absolute alternate executable.
- It creates one aggregate workspace root and one aggregate log directory.
- It runs `scripts/smoke_direct_use.sh` under `direct-use-workspace`.
- It runs `scripts/smoke_ai_provider.sh` under `ai-provider-workspace`.
- Child stage stdout/stderr are captured to `direct-use.txt` and `ai-provider.txt`, while each child smoke keeps its own detailed log directory.

## Safety Boundary

The aggregate smoke reuses the child smoke boundaries instead of reimplementing them. It refuses provided binary, workspace-root, and log-directory paths that are relative, symlink leaves, or pass through existing symlink path components. Provided existing non-empty aggregate workspace roots require `OURO_IDE_SMOKE_ALLOW_EXISTING_WORKSPACE=1`. The AI provider stage uses only the local fake OpenAI-compatible server from `smoke_ai_provider.sh` unless the caller deliberately overrides that child smoke behavior. The aggregate smoke does not contact real providers, endpoints, or targets; it does not execute managed payload acceptance, mutate production evidence, or claim production completion.

## Validation

Focused RED/GREEN, direct smoke, and root regression passed:

```bash
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test . -run 'TestReleaseMakefileAndCIEntrypoints|TestIDESmokeScriptExercisesDirectUseAndAIProviderPath' -count=1 -v
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct make smoke-ide
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test . -run 'TestReleaseMakefileAndCIEntrypoints|TestIDESmokeScriptExercisesDirectUseAndAIProviderPath|TestDocumentationBashBlocksPassShellSyntaxCheck|TestFinalEvidenceRunbookBashBlocksPassShellSyntaxCheck' -count=1 -v
sh -n scripts/smoke_ide.sh scripts/smoke_direct_use.sh scripts/smoke_ai_provider.sh
gofmt -l release_scripts_test.go
git diff --check
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test . -count=1
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct make build
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct make doctor-strict
```

The real aggregate smoke printed `direct-use: ok`, `ai-provider: ok`, and `ide-smoke: ok`. `go test . -count=1` passed with root runtime about 345s. `make build` printed `built bin/ouro`; `make doctor-strict` printed `workspace: ready`.

## Remaining

This closes a local operator usability proof by combining the built-binary core IDE smoke and direct AI provider smoke into one command. It is not the final 100% production-completion claim: final completion still requires real operator-owned production evidence, an approved attestation, `make audit-completion`, manifest/digest sidecar review, and final human/operator acceptance.
