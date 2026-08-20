# Milestone 124: Release-Managed Acceptance Smoke

Date: 2026-06-07

## Summary

Milestone 124 adds a local release-managed acceptance smoke entrypoint:

```sh
make smoke-release-managed
```

The Make target runs `scripts/smoke_release_managed.sh`. The script builds a local release archive, installs it through the checksum-verifying installer, and then uses the installed `${PREFIX}/bin/ouro` to exercise the release-managed import workflow against a fresh workspace.

## Scope

- Build a local release archive with `scripts/release.sh`.
- Install the archive with `scripts/install.sh` into a temporary `PREFIX`.
- Use the installed binary, not source `go run`, for workspace `init`, strict `doctor`, release-managed commands, and `run --tui`.
- Inspect the installed release root with `toolchain release-managed-bundles`.
- Extract the release-managed bundle SHA-256 and require it as `--expected-sha256` for status, status-source, install-plan, and install checks.
- Verify pre-install `release-managed-status` reports `digest_status=verified`, `digest_scope=directory_canonical_records`, `expected_sha256_status=matched`, `import_status=not_imported`, and `link_status=missing`.
- Set and read a pinned `release-managed-status-source` marker for `nuclei-templates`.
- Verify `release-managed-install-plan` remains metadata-only and does not create a workspace copy or final link.
- Verify `release-managed-install` without `--execute` fails and does not create a workspace copy or final link.
- Execute `release-managed-install --execute` for `web-api-pack nuclei-templates`, then verify the workspace payload copy and `toolchain/rules/nuclei-templates` symlink.
- Verify post-install `release-managed-status` reports `import_status=linked` and `link_status=linked`.
- Run `run --tui` and assert the core non-interactive TUI snapshot still renders over that workspace.
- Refuse provided existing non-empty `WORKSPACE` values unless `OURO_RELEASE_MANAGED_SMOKE_ALLOW_EXISTING_WORKSPACE=1` is explicitly set.

## Boundaries

This milestone is a local acceptance smoke, not an updater.

It does not download remote payloads, sync updates, call package managers, install the full upstream rule corpus, execute the rule payload as a scanner, contact providers, dispatch workers, contact endpoints or targets, or mutate replay/report state outside the normal offline `run --tui` path.

The script uses fresh temporary `DIST_DIR`, `PREFIX`, `WORKSPACE`, and log directories by default because `scripts/install.sh` replaces `${PREFIX}/share/ouroboros/toolchains/managed-bundles` and `release-managed-install --execute` writes workspace copy/link state.

The script intentionally does not grep release-managed Toolchain Health rows from default `run --tui` output, because the non-interactive TUI snapshot does not guarantee the active tab or enough width/height for every tab label. Detailed release-managed Toolchain Health row projection remains covered by app/TUI tests.

## Validation

RED validation first failed because the Make target and script did not exist:

```sh
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test . -run 'ReleaseManagedSmoke|ReleaseMakefileAndCIEntrypoints' -count=1 -v
```

Focused GREEN validation passed after implementation and review-driven hardening:

```sh
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test . -run 'ReleaseManagedSmoke|ReleaseMakefileAndCIEntrypoints' -count=1 -v
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct make smoke-release-managed
```

Final validation passed after documentation synchronization:

```sh
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct go test ./... -count=1
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct make doctor-strict
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct make build
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct make smoke-workflow
GOCACHE=$PWD/.cache/go-build GOTMPDIR=/home/zss/t GOPROXY=https://goproxy.cn,direct make smoke-release-managed
sh -n scripts/release.sh scripts/build.sh scripts/install.sh scripts/smoke_workflow.sh scripts/smoke_release_managed.sh
git diff --check
gofmt -l release_scripts_test.go
```
