# Milestone 132: Managed Update Binary Archive Download

## Implemented
- Extended managed update archive metadata to `kind=binary` while preserving existing `rule_bundle` archive behavior.
- Binary archive download plans now use `archive_expected_sha256` for downloaded tar.gz/zip bytes and keep `expected_sha256` for the final extracted binary file digest.
- Download execution verifies archive bytes before extraction, extracts through the existing safe tar.gz/zip path, rejects unsafe paths, symlinks, unsupported entry types, zero regular files, and multiple regular files, then publishes exactly one staged non-executable `payload` file with mode `0644`.
- Apply revalidates request, plan, download audit, source URL, digest, staged payload, and archive metadata before copying the staged binary into `workspace/toolchain/managed-bundles/<name>/payload/<name>`.
- CLI output and existing TUI projections reuse the generic archive fields for binary archive plans/executions, including `archive_format`, archive SHA-256 fields, `digest_scope=file`, `archive_digest_scope=file`, and staged payload paths.
- Updated `README.md`, `docs/development.md`, `task_plan.md`, and `findings.md` to describe direct binary downloads versus binary archive downloads.

## Not Implemented
- No final `workspace/toolchain/bin/<name>` link is created by the download or apply gates.
- No executable bit is preserved from archives, and no staged or applied binary is run.
- No package manager, health probe, provider, worker, endpoint, or target contact is added.
- No broader real third-party binary payload corpus is bundled in release artifacts yet.
- No production end-to-end acceptance with real managed binary payloads is claimed by this milestone.

## Validation
- RED first: `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/app -run 'BinaryArchive' -count=1 -v` failed because archive metadata was supported only for `rule_bundle`.
- GREEN focused app archive validation passed: `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/app -run 'BinaryArchive|RuleBundleArchive|ZipRuleBundleArchive|TamperedArchiveMetadata' -count=1 -v`.
- Managed update app/cmd/tui regression passed: `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/app ./cmd/ouro ./internal/tui -run 'ManagedToolBundleUpdateDownload|ManagedUpdateDownload|ManagedUpdateApply|RuntimeManagedUpdateDownload|NewTUIModelForWorkspaceShowsManagedUpdate|RootCommandIncludesProductionCommands' -count=1 -v`.
- Binary archive focused app/CLI validation passed: `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/app ./cmd/ouro -run 'BinaryArchive|ExtractsBinaryArchive|RootCommandIncludesProductionCommands' -count=1 -v`.
- Affected package validation passed: `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/app ./cmd/ouro ./internal/tui -count=1`.
- Full repository validation passed: `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./... -count=1`.
- Strict readiness passed: `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct make doctor-strict` printed `workspace: ready`.
- Build validation passed: `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct make build` printed `built bin/ouro`.
- Core workflow smoke passed: `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct make smoke-workflow` ended with `workflow-smoke: ok`.
- Release-managed acceptance smoke passed: `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct make smoke-release-managed` ended with `release-managed-smoke: ok`.
- Script syntax, diff hygiene, and touched-file gofmt passed with `sh -n scripts/release.sh scripts/build.sh scripts/install.sh scripts/smoke_workflow.sh scripts/smoke_release_managed.sh`, `git diff --check`, and `gofmt -l internal/app/toolchain.go internal/app/toolchain_test.go cmd/ouro/main.go cmd/ouro/main_test.go`.

## Next Step
- Continue with production end-to-end acceptance using real managed payload candidates and broader packaged corpus provenance.

## Risks / Decisions
- Binary archive support deliberately extracts only one regular file. Archives with multiple candidate files are treated as ambiguous and rejected instead of guessing a binary by filename.
- `expected_sha256` remains the final payload digest, never the archive digest; `archive_expected_sha256` is the separate trust pin for downloaded archive bytes.
- This milestone improves binary distribution mechanics but does not yet solve broad payload acquisition, signature trust policy for real third-party binaries, or release packaging of a complete binary corpus.
