# Milestone 15: Authored Tool Workspace Artifacts

## Implemented
- Added `toolchain.LoadAuthoredToolSpec` for YAML authored native tool specs.
- Added app-layer authored tool artifact APIs:
  - `CreateAuthoredTool(root, spec)`
  - `ListAuthoredTools(root)`
  - `ReadAuthoredTool(root, name)`
- Valid authored specs are persisted under `workspace/toolchain/<tool-name>/authored-tool.yaml`.
- Each persisted spec also writes a generated `workspace/toolchain/<tool-name>/manifest.yaml` that is loadable by the existing toolchain manifest loader.
- Generated manifests use `runner: native`, keep parser/capability/dependency metadata, and do not include `run_command`.
- App writes validate the full authored spec before creating the tool directory, so invalid specs do not leave partial tool directories.
- Tool names must be direct path-safe segments. Writes reject empty names, surrounding whitespace, absolute paths, slash/backslash traversal, `.` / `..`, unsupported characters, existing toolchain directories, symlinked tool directories, and symlinked artifact files.
- Authored metadata paths use workspace validation that skips container runtime probing, so write/list/read remain metadata-only even when `execution.isolation: container` is configured.
- Authored reads re-derive the manifest from `authored-tool.yaml` and reject drift in runner, run command, healthcheck, parser, capabilities, or dependencies.
- The global manifest loader now rejects symlinked manifest directories and symlinked `manifest.yaml` files before reading, so authored or built-in toolchain manifests cannot be loaded through outside symlink paths.
- Added CLI commands:
  - `ouro toolchain authored-tool-validate <spec-file>`
  - `ouro toolchain authored-tool-write <workspace> <spec-file>`
  - `ouro toolchain authored-tools <workspace> [name]`
- Validate/write/list/read paths are metadata-only. They do not execute binaries, run health probes, dispatch workers, or contact targets.

## Missing
- There is still no real native executor for authored tools.
- Managed binary and rule-bundle installation is still incomplete. Milestone 16 adds workspace-local bundle discovery and link creation, but release-packaged real tool payloads are not bundled yet.
- Persisted authored manifests are visible to `LoadManifests`, but runtime execution still needs native runner bindings, parser contracts, target-kind checks, and authorization-aware execution gates.
- Parser bindings for real appsec output formats such as Nmap XML, nuclei JSONL, and httpx JSONL remain thin.

## Validation
- RED app checks confirmed `CreateAuthoredTool`, `ListAuthoredTools`, and `ReadAuthoredTool` were absent.
- RED CLI checks confirmed authored-tool subcommands were absent.
- Focused GREEN checks:
  - `GOPROXY=https://goproxy.cn,direct go test ./internal/app -run 'TestCreateAuthoredTool' -count=1 -v`
  - `GOPROXY=https://goproxy.cn,direct go test ./internal/app -run 'TestAuthoredToolMetadataPathsDoNotRunContainerRuntimeDoctor|TestReadAuthoredToolRejectsDerivedManifestDrift|TestCreateAuthoredTool' -count=1 -v`
  - `GOPROXY=https://goproxy.cn,direct go test ./internal/toolchain -run 'TestLoadManifestRejectsSymlinkedManifestFile|TestLoadManifestsRejectsSymlinkedToolDirectory' -count=1 -v`
  - `GOPROXY=https://goproxy.cn,direct go test ./cmd/ouro -run 'TestToolchainAuthoredTool' -count=1 -v`
  - `GOPROXY=https://goproxy.cn,direct go test ./internal/toolchain ./internal/app ./cmd/ouro -count=1`

## Next Step
- Implement the managed tool bundle/installer layer for declared tools such as `nmap`, `nuclei`, `httpx`, and `apktool` without automatic target contact.
- Add native runner bindings that can execute authored native tools only through scoped target, isolation, authorization, and parser gates.
- Extend parser bindings for the high-value appsec tools before allowing authored native tools to produce first-class evidence.

## Risks And Decisions
- Persisting `manifest.yaml` makes authored tools visible to the existing compiler surface. This is intentional, but execution remains gated because `runner: native` has no authored-tool executor yet. Read/list reject derived-manifest drift to keep the manifest tied to the structured authored spec.
- Authored spec safety fields can describe closeable target-policy or isolation behavior, but they do not authorize workspace execution. `authorization.yaml`, strict doctor, and runtime execution gates remain authoritative.
- The writer refuses to overwrite existing toolchain directories so model-authored tools cannot replace bundled packs such as `web-api-pack`, `infra-pack`, or `ctf-pack`.
