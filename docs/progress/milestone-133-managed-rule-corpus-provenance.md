# Milestone 133: Managed Rule Corpus Provenance Index

## Implemented
- Added optional offline `corpus:` metadata for `kind: rule_bundle` managed bundle manifests.
- Corpus metadata records `schema`, `name`, `source`, `retrieved_at`, `file_count`, `digest_algorithm`, `digest_scope`, and `sha256`.
- Valid corpus metadata is accepted only when `schema=ouroboros.rule_corpus.v1`, `source` is HTTP(S), `retrieved_at` is RFC3339, `file_count` matches the payload directory's regular-file count, `digest_algorithm=sha256`, `digest_scope=directory_canonical_records`, and `sha256` matches the rule bundle canonical directory digest.
- Missing corpus metadata reports `corpus_status=missing` and `corpus_digest_status=missing`; missing remains an explicit review state rather than a workspace conflict.
- Invalid present corpus metadata blocks strict release/workspace reads, inventory/status rows, install plans, and install before workspace copy/link writes.
- CLI output, workspace-bound Toolchain Health rows, RuntimeAction/Inspector details, release-managed install plan/result rows, and release-managed smoke assertions now surface corpus status, identity, source, retrieval time, file count, digest, algorithm, and scope.
- The bundled `nuclei-templates` seed now carries matched corpus metadata as an offline acceptance index.
- Release archive README generation now documents the corpus metadata fields, and release script tests inspect the generated README content.

## Not Implemented
- No corpus source is fetched, synced, or treated as a trust root.
- No package manager, scanner payload execution, health probe, provider, worker, endpoint, or target contact is added.
- No final bin/rules link is created by metadata reads or release-managed status checks.
- No replay/report state is mutated outside the existing offline smoke path.
- No full upstream Nuclei template corpus distribution is claimed.

## Validation
- RED first: `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/app ./cmd/ouro -run 'MissingCorpusMetadataStaysNonConflicting|RuleBundleCanonicalDigestScope' -count=1 -v` failed because missing corpus did not set `corpus_digest_status=missing` and CLI printed an empty corpus digest status.
- GREEN focused corpus missing/conflict validation passed with the same command after making missing digest status explicit and removing optional corpus metadata from release/workspace conflict criteria.
- Focused corpus propagation validation passed earlier with `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/app ./cmd/ouro ./internal/tui -run 'Corpus|ManagedBundle|ReleaseManaged|RuntimeManaged|NewTUIModelForWorkspaceShowsManagedBundle|NewTUIModelForWorkspaceShowsReleaseManaged|RootCommandIncludesProductionCommands' -count=1 -v`.
- Affected package validation passed earlier with `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./internal/app ./cmd/ouro ./internal/tui -count=1`.
- Release script README regression passed: `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test . -run 'TestReleaseScriptPackagesBuiltInManagedRuleBundle|TestReleaseScriptCreatesArchiveAndChecksums' -count=1 -v`.
- Release-managed smoke script regression passed: `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test . -run 'TestReleaseManagedSmokeScriptExercisesInstalledReleaseManagedPath' -count=1 -v`.
- Full repository validation passed: `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./... -count=1`.
- Strict readiness passed: `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct make doctor-strict` printed `workspace: ready`.
- Build validation passed: `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct make build` printed `built bin/ouro`.
- Core workflow smoke passed: `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct make smoke-workflow` ended with `workflow-smoke: ok`.
- Release-managed acceptance smoke passed: `GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct make smoke-release-managed` ended with `release-managed-smoke: ok`.
- Script syntax, diff hygiene, and touched-file gofmt passed with `sh -n scripts/release.sh scripts/build.sh scripts/install.sh scripts/smoke_workflow.sh scripts/smoke_release_managed.sh`, `git diff --check`, and `gofmt -l release_scripts_test.go cmd/ouro/main.go cmd/ouro/main_test.go internal/app/toolchain.go internal/app/toolchain_test.go internal/app/tui_snapshot.go internal/app/tui_snapshot_test.go internal/tui/runtime_tabs.go internal/tui/runtime_tabs_test.go internal/tui/inspector.go`.

## Next Step
- Continue toward production end-to-end acceptance with real managed binary/rule payloads beyond the bundled seed and deeper autonomous model/worker closure.

## Risks / Decisions
- Corpus metadata is deliberately optional. Missing corpus is visible but does not block release/workspace import status or create conflicts when the payload identity still matches.
- Invalid present corpus metadata fails closed before workspace writes because it claims a concrete file count and directory digest.
- Corpus metadata is review and acceptance metadata only. It is not an updater, expected digest pin, signature, source marker policy, downloader, or full upstream corpus claim.
