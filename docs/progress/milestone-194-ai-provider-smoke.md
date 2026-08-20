# Milestone 194 - AI Provider Smoke

## Summary

Added a repeatable direct AI provider smoke for the security-operator IDE path. The project now has a single command that proves a fresh workspace can configure an `openai_compatible` provider, call the manual generation path, and inspect metadata-only usage accounting without hand-editing config or contacting a real provider.

## Behavior

- `make smoke-ai-provider` runs `scripts/smoke_ai_provider.sh`.
- The script uses the built `bin/ouro` by default; `OURO_AI_PROVIDER_SMOKE_BIN` can point to an absolute alternate executable.
- A local fake OpenAI-compatible server listens only on `127.0.0.1` and exposes the model preflight and chat-completions endpoints needed by the existing provider adapter.
- The smoke creates a fresh temporary workspace by default, runs `init`, configures `openai_compatible` with `provider configure-openai-compatible`, runs `doctor`, calls `provider generate --record-usage`, and verifies `provider usage --summary` reports one record for the configured provider.
- When `WORKSPACE` is set, missing paths or empty directories are accepted; existing non-empty workspaces require `OURO_AI_PROVIDER_SMOKE_ALLOW_EXISTING_WORKSPACE=1`.

## Safety Boundary

The smoke uses only a fixed temporary API key value exported for the local fake provider. It checks that value is not printed or written to `providers.yaml` or usage records. Provided binary, workspace, and log-directory paths must be absolute, must not be symlink leaves, and must not pass through existing symlink path components. The smoke does not contact real providers, endpoints, or targets; it does not run tools, dispatch workers, mutate replay/report state, or claim production completion.

## Validation

Focused RED/GREEN, direct smoke, and regression passed:

```bash
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test . -run 'TestReleaseMakefileAndCIEntrypoints|TestAIProviderSmokeScriptExercisesConfigureAndGeneratePath' -count=1 -v
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct make build
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct make smoke-ai-provider
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test . -run 'TestDocumentationBashBlocksPassShellSyntaxCheck|TestFinalEvidenceRunbookBashBlocksPassShellSyntaxCheck' -count=1 -v
sh -n scripts/smoke_ai_provider.sh
gofmt -l release_scripts_test.go
git diff --check
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./cmd/ouro ./internal/app ./internal/tui -count=1
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./... -count=1
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct make doctor-strict
```

`make build` printed `built bin/ouro`. The real smoke printed `provider-configure: ok`, `doctor: ready`, `provider-generate: ok`, `usage-summary: records=1`, and `ai-provider-smoke: ok`. `go test ./...` passed with root runtime about 381s, `internal/app` about 98s, and `cmd/ouro` about 26s. `make doctor-strict` printed `workspace: ready`.

## Remaining

This closes the repeatable local direct-AI usability proof for the configured provider and manual usage ledger path. It is not the final 100% production-completion claim: final completion still requires real operator-owned production evidence, an approved attestation, `make audit-completion`, manifest/digest sidecar review, and final human/operator acceptance.
