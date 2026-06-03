# Milestone 45: Release Managed Rule Bundle Seed

## Summary

Milestone 45 adds the first built-in release-managed payload seed.

Release archives now include `toolchains/managed-bundles/nuclei-templates/`, a minimal managed `rule_bundle` with a digest-verified payload directory. This gives the release-managed install path a real default rule-bundle artifact to copy into workspaces and link under `workspace/toolchain/rules/`.

## Execution Boundary

- The bundled asset is a small Ouroboros-owned Nuclei template seed, not the full upstream Nuclei template corpus.
- No binary payloads are bundled in this milestone.
- Release install remains local-only and checksum-gated when `SHA256SUMS` is present.
- Importing the rule bundle into a workspace still requires the explicit `release-managed-install --execute` gate.
- The import path copies metadata/payload files and creates the workspace rules symlink; it does not run `nuclei`, contact targets, update templates, or execute tools.

## Validation

Validated during this milestone with:

```text
GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test . -run 'TestReleaseScriptPackagesBuiltInManagedRuleBundle' -count=1 -v
GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test . -run 'Release|InstallScript|MakeRelease' -count=1 -v
GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./internal/app -run 'ReleaseManaged|ManagedRule|ManagedToolBundle|Nuclei' -count=1
GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./cmd/ouro -run 'ReleaseManaged|ManagedBundle' -count=1
GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./internal/toolchain -count=1
sh -n scripts/install.sh scripts/release.sh
GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct go test ./... -count=1
GOCACHE=$PWD/.cache/go-build GOPROXY=https://goproxy.cn,direct make doctor-strict
git diff --check
gofmt -l release_scripts_test.go
```

The RED run failed because the release archive contained only an empty `toolchains/managed-bundles/` directory. After adding the built-in rule bundle seed, the same test passed by building a release archive, verifying the bundled manifest/template members, installing the archive locally, initializing a workspace, importing `web-api-pack/nuclei-templates`, and confirming `workspace/toolchain/rules/nuclei-templates` is a symlink to the copied managed payload.

Strict doctor printed `workspace: ready`; diff hygiene and touched-file formatting checks returned no output.

## Remaining Gaps

The IDE is still not complete. Current effective gaps include binary production payload bundling, fuller managed rule corpus packaging/provenance, provider generation calls and model capability handling beyond readiness preflight, and autonomous distributed worker scheduling beyond bounded/gated audit runs.
