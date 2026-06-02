# Milestone 17: Release Managed Tool Bundles

## Implemented
- Added release bundle layout support:
  - `<release-root>/toolchains/managed-bundles/<tool>/bundle.yaml`
  - `<release-root>/toolchains/managed-bundles/<tool>/payload/...`
- Added app-layer release managed bundle APIs:
  - `CreateReleaseManagedToolBundleInstallPlan(root, releaseRoot, pack, dependency)`
  - `ExecuteReleaseManagedToolBundleInstall(root, releaseRoot, pack, dependency, options)`
- Added CLI commands:
  - `ouro toolchain release-managed-install-plan <workspace> <release-root> <pack> <dependency>`
  - `ouro toolchain release-managed-install <workspace> <release-root> <pack> <dependency> --execute`
- Release install planning validates the selected workspace pack dependency, resolves the matching release bundle, verifies payload integrity, and prints a metadata-only `action=copy-and-link` plan.
- Release install execution requires explicit `--execute`, copies the selected release bundle into `workspace/toolchain/managed-bundles/<tool>/`, then creates a workspace-local link under `workspace/toolchain/bin/` for binaries or `workspace/toolchain/rules/` for rule bundles.
- Rule-bundle payloads now require SHA-256 integrity and are hashed as sorted relative file paths plus file contents, giving `nuclei-templates`-style bundles an integrity boundary.
- Release bundle copy rejects path traversal, absolute payload paths, symlinked payloads, non-regular files, unsupported kinds, digest mismatch, existing workspace bundles, existing install links, and mismatched bundle/dependency kinds.
- `scripts/release.sh` now includes `toolchains/managed-bundles` in the archive when present.
- `scripts/install.sh` now installs bundled metadata and payloads to `${PREFIX}/share/ouroboros/toolchains/managed-bundles` without writing any workspace.

## Missing
- Real release payload build assets for `nmap`, `nuclei`, `httpx`, `subfinder`, `ffuf`, `katana`, `naabu`, `dnsx`, `apktool`, `jadx`, and related tools are not bundled yet.
- Native runner execution still does not resolve `workspace/toolchain/bin` or `workspace/toolchain/rules`.
- Model-authored native tools still need a typed adapter registry and parser contract; current authored specs are safe metadata artifacts, not executable adapters.
- Parser bindings for Nmap XML, nuclei JSONL, httpx JSONL, and similar high-value outputs remain pending.

## Validation
- RED app checks confirmed release managed bundle APIs were absent.
- RED CLI checks confirmed release managed bundle commands were absent.
- RED release/install script checks confirmed archives omitted `toolchains/managed-bundles` and installer ignored bundled payloads.
- Focused GREEN checks:
  - `GOPROXY=https://goproxy.cn,direct go test ./internal/app -run 'TestReleaseManaged' -count=1`
  - `GOPROXY=https://goproxy.cn,direct go test ./cmd/ouro -run 'TestToolchainReleaseManaged' -count=1`
  - `GOPROXY=https://goproxy.cn,direct go test . -run 'TestReleaseScriptCreatesArchiveAndChecksums|TestInstallScriptInstallsFromLocalArchiveWithoutNetwork' -count=1`

## Next Step
- Add native runner lookup that prefers workspace-local managed bundle paths while reusing existing target-policy, isolation, and authorization gates.
- Tighten model-authored tool specs so authored dependencies cannot smuggle command arrays, then add native adapter and parser registries.
- Add real release payload generation and platform metadata for the most important tools.

## Risks And Decisions
- `--execute` for release managed install means "copy a bundle and create a workspace link"; it does not run the tool.
- Workspaces copy release bundles instead of linking directly to `${PREFIX}/share/ouroboros`, so workspace state is not broken by Ouroboros upgrades or uninstall operations.
- Installer bundle installation is local-only and offline; it does not fetch remote assets, execute payloads, or modify workspaces.
- Real offensive/security tools must still be packaged through an auditable build process with per-platform metadata before claiming they are bundled.
