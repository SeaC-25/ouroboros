# Milestone 16: Managed Tool Bundle Substrate

## Implemented
- Added workspace-local managed bundle metadata under `workspace/toolchain/managed-bundles/<tool>/bundle.yaml`.
- Added app-layer managed bundle APIs:
  - `ListManagedToolBundles(root)`
  - `ReadManagedToolBundle(root, name)`
  - `CreateManagedToolBundleInstallPlan(root, pack, dependency)`
  - `ExecuteManagedToolBundleInstall(root, pack, dependency, options)`
- Added CLI commands:
  - `ouro toolchain managed-bundles <workspace> [name]`
  - `ouro toolchain managed-install-plan <workspace> <pack> <dependency>`
  - `ouro toolchain managed-install <workspace> <pack> <dependency> --execute`
- Managed bundle manifests declare name, kind, version, source, payload path, and SHA-256 digest for binary payloads.
- Managed install planning validates the selected pack/dependency against the workspace toolchain manifest, resolves the matching local bundle, verifies the payload digest, and prints a metadata-only `action=link` plan.
- Managed install execution requires explicit `--execute` and creates only a workspace-local symlink such as `workspace/toolchain/bin/httpx`.
- Managed bundle operations reject payload path traversal, absolute payload paths, symlinked payloads, unsupported kinds, digest mismatch, existing install paths, and mismatched bundle/dependency kinds.
- Managed bundle operations do not run bundled payloads, execute health probes, invoke package managers, fetch remote assets, contact targets, dispatch workers, or use the arbitrary `install_command` execution path.

## Missing
- Real release-packaged payloads for tools such as `nmap`, `nuclei`, `httpx`, `apktool`, and `jadx` are not bundled yet.
- Rule-bundle directory integrity needs a stronger manifest or file-list digest model before `nuclei-templates` can be treated as fully verified.
- The native runner does not yet add `workspace/toolchain/bin` to execution lookup or execute managed tools.
- Parser bindings for high-value tool outputs are still thin.
- The existing `install-request` / `install-dispatch` command execution path remains separate; this slice adds a safer local-bundle path but does not remove the older approved command runner.

## Validation
- RED app checks confirmed managed bundle APIs were absent.
- RED CLI checks confirmed managed bundle commands were absent.
- Focused GREEN checks:
  - `GOPROXY=https://goproxy.cn,direct go test ./internal/app -run 'TestListManagedToolBundles|TestManagedToolBundle' -count=1 -v`
  - `GOPROXY=https://goproxy.cn,direct go test ./cmd/ouro -run TestToolchainManagedBundleCommandsListPlanAndInstall -count=1 -v`

## Next Step
- Add release/package layout support so bundled payloads can ship beside the Ouroboros binary and be copied into workspaces.
- Add rule-bundle integrity manifests for `nuclei-templates`.
- Wire managed install results into toolchain health and native runner lookup without weakening target, isolation, or authorization gates.

## Risks And Decisions
- This slice deliberately links local payloads instead of running installers. That makes progress toward "users do not manually install tools" without introducing package-manager or shell execution risk.
- The local bundle path is workspace-confined. Future global release-bundle support should keep the same digest, symlink, and path traversal checks.
- `--execute` here means "create a workspace link", not "run a tool". Target-contacting tool execution remains a separate native-runner milestone.
