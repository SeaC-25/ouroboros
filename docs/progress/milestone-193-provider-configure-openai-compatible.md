# Milestone 193 - Provider Configure OpenAI Compatible

## Summary

Added a direct provider setup command for the security-operator IDE path. Fresh workspaces no longer require hand-editing `providers.yaml` before the TUI AI dialog or manual provider previews can use a real `openai_compatible` model.

## Behavior

- `ouro provider configure-openai-compatible <workspace>` upserts a named `openai_compatible` provider into `providers.yaml`.
- The command accepts `--name`, `--base-url`, `--model`, `--api-key-env`, `--timeout-ms`, `--context-window-tokens`, repeatable `--capability`, `--routing-strategy`, and `--enable`.
- API keys remain environment-only. The command stores only the `api_key_env` variable name, does not read the variable value, and prints `api_key_value_stored=false`.
- Enabled configured providers are moved before preserved existing providers so first-enabled provider selection reaches the real model.
- When a real provider is enabled, the default `offline` provider is preserved but disabled.
- Capabilities are normalized and default to `chat` when omitted.
- The final config is validated with existing provider validation before publication.

## Safety Boundary

The configure command does not call providers, run tools, dispatch workers, contact targets, write replay events, write usage records, or persist prompt/assistant text. It rejects invalid base URLs, invalid environment variable names, negative runtime metadata, unsupported routing strategies, invalid final provider configs, and symlinked `providers.yaml` leaves before writing. YAML is published through a same-directory temporary file and rename.

## Validation

Focused RED/GREEN and regression passed:

```bash
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/app ./cmd/ouro -run 'ConfigureOpenAICompatible|ProviderConfigure' -count=1 -v
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/app ./cmd/ouro -run 'ConfigureOpenAICompatible|ProviderConfigure|ProviderGenerate|ValidateProviderConfig|RunWorkspaceProvider|NewTUIModelForWorkspaceAIDialog' -count=1 -v
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/app ./cmd/ouro ./internal/tui -count=1
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./... -count=1
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct make build
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct make doctor-strict
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test . -run 'TestDocumentationBashBlocksPassShellSyntaxCheck|TestFinalEvidenceRunbookBashBlocksPassShellSyntaxCheck' -count=1 -v
gofmt -l internal/app/provider_config.go internal/app/provider_config_test.go cmd/ouro/main.go cmd/ouro/main_test.go
git diff --check
```

`go test ./...` passed with root runtime about 358s, `internal/app` about 97s, and `cmd/ouro` about 24s. `make build` printed `built bin/ouro`; `make doctor-strict` printed `workspace: ready`.

## Remaining

This closes the direct provider setup gap for the interactive AI dialog and manual provider previews. It is not the final 100% production-completion claim: final completion still requires real operator-owned production evidence, an approved attestation, `make audit-completion`, manifest/digest sidecar review, and final human/operator acceptance.
