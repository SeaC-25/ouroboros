# Milestone 26: Artifact App Managed Builders

## Implemented
- Added explicit `artifact` scope support for production runs.
- Mapped artifact targets to the CTF/Lab pack and added an artifact-specific initial hypothesis for local artifact analysis.
- Added `RunOptions.Action` and `ouro run --action <semantic_action>` so app/artifact analysis can be selected explicitly through `fingerprint_surface` while the default run action remains `discover_assets`.
- Added the first artifact/app managed argv builder:
  - `mobsfscan` / `static_code_scan`: `mobsfscan --json --no-fail <artifact>`
- Restricted artifact targets to regular files under `workspace/artifacts/`.
- Resolved relative artifact targets below `workspace/artifacts/` and accepted absolute artifact targets only when they resolve under that same directory.
- Rejected artifact traversal, symlink escape, outside-workspace paths, and non-regular files before payload execution.
- Rejected symlinked `workspace/artifacts` roots and outside artifact command arguments even when the path contains `=`.
- Kept artifact path confinement as a filesystem boundary that is not disabled by `scope.target_policy: disabled`.
- Preserved workspace-managed payload resolution: `mobsfscan` must be installed under `workspace/toolchain/bin/` through managed bundle linking and does not fall through to ambient `PATH`.
- Preserved process-mode only behavior for managed builders; container-mode remains offline for this path.
- Fixed the concurrent `recordingToolExecutionRunner` test helper so concurrent scheduler tests do not lose request records through unsynchronized appends.

## Not Implemented
- This is not full app testing, full APK/mobile assessment, or an end-to-end static testing platform.
- `apktool`, `jadx`, `semgrep`, `gitleaks`, `trufflehog`, `binwalk`, and `exiftool` remain catalog/registry entries until their own builders and output boundaries are implemented.
- Parser dispatch still does not convert real `mobsfscan` JSON into evidence graph facts; current output remains transcript-level execution evidence.
- Real release payload packaging for app/artifact tools is still separate from this slice.
- No broad TUI/browser scan launcher was added; workspace TUI rows remain metadata/read-only unless an existing explicit gate already handles that action kind.
- Container-mode artifact execution remains offline until a mount-safe adapter design exists.

## Validation
- RED first:
  - `GOPROXY=https://goproxy.cn,direct go test ./internal/app ./internal/domain/ctf ./cmd/ouro -run 'TestValidateScopeConfigAcceptsArtifactTargets|TestNativeManagedRunCommand.*Mobsfscan|TestRunWorkspaceExecutesToolLevelMobsfscanThroughExplicitArtifactScope|TestRunWorkspaceRejectsExcludedArtifactBeforeMobsfscanExecution|TestArtifactTargetProducesArtifactAnalysisHypothesis|TestRunCommandCanSelectFingerprintSurfaceActionForArtifactScope|TestRunCommandRejectsUnknownAction' -count=1 -v`
  - Failed because `RunOptions.Action` was missing, artifact CTF hypotheses rejected `kind=artifact`, `scope.yaml` rejected artifact targets, and CLI `run --action` was unknown.
- Focused GREEN:
  - `GOPROXY=https://goproxy.cn,direct go test ./internal/app ./internal/domain/ctf ./cmd/ouro -run 'TestValidateScopeConfigAcceptsArtifactTargets|TestNativeManagedRunCommand.*Mobsfscan|TestRunWorkspaceExecutesToolLevelMobsfscanThroughExplicitArtifactScope|TestRunWorkspaceRejectsExcludedArtifactBeforeMobsfscanExecution|TestArtifactTargetProducesArtifactAnalysisHypothesis|TestRunCommandCanSelectFingerprintSurfaceActionForArtifactScope|TestRunCommandRejectsUnknownAction' -count=1 -v`
- Broader artifact/app regression:
  - `GOPROXY=https://goproxy.cn,direct go test ./internal/app ./internal/domain/ctf ./cmd/ouro -run 'Artifact|Mobsfscan|RunCommandCanSelectFingerprintSurfaceAction|RunCommandRejectsUnknownAction' -count=1 -v`
- Full repo validation:
  - `GOPROXY=https://goproxy.cn,direct go test ./... -count=1`
  - `make doctor-strict`
  - `git diff --check`
- Safety review fix:
  - `GOPROXY=https://goproxy.cn,direct go test ./internal/app -run 'TestNativeManagedRunCommandRejects(SymlinkedArtifactRoot|EscapingArtifactSymlink)|TestValidateArtifactRunCommandArgumentsRejectsAbsolutePathContainingEquals|TestNativeManagedRunCommandBuildsMobsfscanStaticCodeScanArgvForArtifactScope|TestRunWorkspaceExecutesToolLevelMobsfscanThroughExplicitArtifactScope|TestRunWorkspaceRejectsExcludedArtifactBeforeMobsfscanExecution' -count=1 -v`
  - `GOPROXY=https://goproxy.cn,direct go test ./internal/app -count=1`
- Stale wording search:
  - `grep -RInE 'full app|end-to-end app|complete app|static testing platform|production payload|real payload|parser dispatch|parse real|facts/evidence|ambient PATH' README.md docs/development.md docs/progress/milestone-26-artifact-app-managed-builders.md || true`
  - Matches are limited to explicit Not Implemented / follow-up boundary wording and intentional deferred parser/payload statements.

## Next Step
- Add parser dispatch for `mobsfscan` JSON and evidence graph promotion.
- Add the next artifact builder only after its output boundary is explicit, likely `semgrep` with a local config boundary or a metadata-only `exiftool` path.
- Continue release managed payload packaging for app/artifact binaries.

## Risks / Decisions
- Artifact scope is explicit local filesystem authorization. A CTF challenge target does not authorize arbitrary local artifact scanning.
- Artifact paths are workspace-confined even when target policy is disabled; target-policy bypass is not a filesystem traversal or symlinked-root bypass.
- `mobsfscan` is the first low-blast-radius app/static builder because it has a single input path and no output directory or rule-bundle side effect in this slice.
- App/artifact tool outputs stay transcript-only until parser/evidence handling is implemented.
