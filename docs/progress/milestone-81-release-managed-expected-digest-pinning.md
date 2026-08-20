# Milestone 81: Release-Managed Expected Digest Pinning

## Scope

Release-managed bundle status, install planning, and install execution now support an optional operator-supplied expected SHA-256 pin for a named release bundle.

This pin is intentionally separate from the release manifest/payload self-validation digest:

- `digest_status` continues to describe whether the release manifest and payload validate.
- `expected_sha256_status` describes whether the operator-provided expected digest matched the verified release payload digest.
- A mismatch blocks import, link, plan, or install before workspace writes while keeping the release digest result visible for diagnosis.

List-mode `release-managed-status` rejects `--expected-sha256` unless a bundle name is supplied, because one flag cannot unambiguously pin multiple release bundles.

## Changes

- Added release-specific status, install-plan, and install option structs with `ExpectedSHA256` and `ExpectedSHA256Provided`.
- Added `ReadReleaseManagedToolBundleStatusWithOptions` and `CreateReleaseManagedToolBundleInstallPlanWithOptions`, while keeping the existing wrapper APIs for callers that do not supply pin policy.
- Extended `ExecuteReleaseManagedToolBundleInstall` to accept release-managed install options and pass only `Execute` into the lower workspace-managed install gate.
- Added normalized `expected_sha256` and `expected_sha256_status=matched|mismatch` fields to app status, plan, install result, and CLI output.
- Added CLI `--expected-sha256` flags for named `release-managed-status`, `release-managed-install-plan`, and `release-managed-install`.
- Documented that `release-managed-status-source` persists only the release root and unpinned matrix counts; TUI status-source rows are not expected-pinned unless a future persisted pin-policy marker is added.

## Validation

RED first failed on missing option APIs and CLI flags for release-managed expected SHA-256 pinning. GREEN added the option APIs, CLI flags, mismatch blocking, normalized output fields, and list-mode ambiguity rejection.

Focused validation passed:

```bash
GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./internal/app ./cmd/ouro -run 'TestReleaseManagedRuleBundleRequiresDigestAndLinksRulesDirectory|TestToolchainReleaseManagedStatusCommandReportsRuleBundleCanonicalDigestScope|TestReadReleaseManagedToolBundleStatusWithExpectedSHA256PinsDigest|TestReleaseManagedInstallWithExpectedSHA256RejectsMismatchBeforeWorkspaceWrites|TestToolchainReleaseManagedCommandsEnforceExpectedSHA256' -count=1 -v
```

A read-only verification subagent also passed:

```bash
GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./internal/app ./cmd/ouro -run 'ReleaseManaged|ToolchainReleaseManaged|RootCommandIncludesProductionCommands' -count=1 -v
```

During final validation, the first full `go test ./...` exposed one stale root-package call site in `release_scripts_test.go` that still passed `ManagedToolBundleInstallOptions` to `ExecuteReleaseManagedToolBundleInstall`. The root cause was an API signature migration missed by the focused package tests. The call was updated to `ReleaseManagedToolBundleInstallOptions`, and the previously failing root test passed:

```bash
GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test . -run 'TestReleaseScriptPackagesBuiltInManagedRuleBundle' -count=1 -v
```

Final validation passed:

```bash
GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./internal/app ./cmd/ouro -run 'ReleaseManaged|ToolchainReleaseManaged|RootCommandIncludesProductionCommands' -count=1 -v
GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./... -count=1
GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct make doctor-strict
GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct make build
sh -n scripts/release.sh
git diff --check
gofmt -l internal/app/toolchain.go internal/app/toolchain_test.go cmd/ouro/main.go cmd/ouro/main_test.go release_scripts_test.go
```

Release smoke passed with a temporary `DIST_DIR`: the generated archive README includes the release-managed `--expected-sha256` examples, and the extracted `nuclei-templates` status with the canonical digest pin reported:

```text
expected_sha256_status=matched digest_status=verified digest_scope=directory_canonical_records
```
