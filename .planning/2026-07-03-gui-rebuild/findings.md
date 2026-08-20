# Findings & Decisions

## Requirements
- Build Ouroboros as a GUI-first desktop IDE for authorized penetration testing.
- Discard the old mixed GUI implementation and the failed Warp adaptation route.
- Preserve backend capabilities: workspace lifecycle, replay, evidence graph, tool execution, toolchain, reporting, provider configuration, and worker/runtime status.
- Use `/home/zss/ouroboros/ui-designs/` as the UI source of truth.

## Repository Findings
- `/home/zss/ouroboros` is a multi-project parent. The active Git repository is `/home/zss/ouroboros/Ouroboros`.
- The worktree is already very dirty. Many Go/TUI/API files are modified or untracked; this GUI rebuild work must avoid unrelated changes.
- `gui/` is currently untracked but contains Electron + Vite + React + Zustand + React Query + Cytoscape. The stack is acceptable.
- `gui/src/renderer/App.tsx` now renders the rebuilt `src/app/OuroborosIDE.tsx` shell directly.
- The old active renderer/router/store/component paths have been removed from the GUI contract surface; `gui/scripts/check-gui-contract.mjs` fails if they return.
- `gui/scripts/check-product-contract.mjs` fixes the product direction around desktop software, real data, and Chinese UI.
- `gui/scripts/check-desktop-contract.mjs` verifies preload, workspace IPC, and backend sidecar wiring.
- `scripts/check_no_gui_mock_literals.sh` prevents demo graph/toolchain literals from reappearing in `internal/api` or active frontend code.
- `internal/api/server.go` exposes health, GUI bootstrap, workspace info/files, evidence graph/nodes, runs, toolchain health, and WebSocket. Evidence graph/toolchain/runs now use API projector data instead of mock-shaped responses.
- README still describes the product as TUI-first. This conflicts with the new GUI-first decision and should be corrected in a later doc pass.

## Visual Findings
- `main_ui.png`: primary v0 shell. It shows a professional security IDE with top brand bar, left activity bar and workspace explorer, central AI/mission conversation, right attack graph/agent graph/inspector panels, and bottom status bar.
- `empty_state.png`, `onboarding.png`, `start_pentest_modal.png`, `preflight_check.png`: first-run and mission-start flow. These should form the first vertical slice before broad feature work.
- `command_palette.png`: global command launcher overlay. It should be a shell service, not a page.
- `tool_execution.png`: dedicated execution monitor with command, logs, status, and operator review context.
- `node_inspector.png`: detailed node side panel for selected graph/workflow entities.
- `replay_view.png`: independent replay mode with timeline, event table, graph snapshot, and playback controls.
- `report_view.png`: independent report reader/editor with table of contents, Markdown-style report body, and search/layout controls.
- `settings_panel.png`: dense product settings page with category navigation and consistent controls.
- `ouro_attack_graph.png`: full attack graph workspace. Use it as the graph-mode reference, not as the only app shell.
- `粘贴的文件.png`: brand/startup visual reference; do not let it replace the task-first IDE shell.

## Technical Decisions
| Decision | Rationale |
|----------|-----------|
| Rebuild the renderer app-layer | Current code is split between new VSCode-like components and an unused old router. Incremental patching will preserve the split. |
| Keep Electron/Vite/React | The existing stack can support the desired IDE and avoids a new framework migration. |
| Replace fixtures with projector-backed API data | The active renderer consumes Go API data, and `/api/gui/bootstrap` is the aggregate contract for the desktop shell. |
| Separate modes by route | Main IDE, attack graph, replay, report, and settings have different information architecture and should not be forced into one component. |
| Create explicit DTOs | Frontend should consume stable GUI contracts, not infer behavior from Go internals or mock responses. |
| Keep empty states real | Empty workspaces and empty evidence graphs render as empty states instead of fabricated nodes, runs, or findings. |

## Implementation Findings
- `gui/src/app/OuroborosIDE.tsx` now carries the v0 desktop shell and first-pass modes: mission, attack graph, replay, report, and settings.
- `gui/src/app/types.ts` defines frontend DTOs for workspace items, mission state, graph nodes, execution steps, replay events, report findings, reports, tool actions, settings, and status summary.
- `gui/src/app/api.ts` maps `/api/gui/bootstrap` into `IDEData` as the sole aggregate desktop GUI data source; legacy endpoint aggregate fallback and `fixtures.ts` are removed from the active renderer path.
- `gui/src/app/zh-cn.ts` centralizes visible Chinese UI copy so TSX does not carry hardcoded English labels.
- `gui/src/styles/ouroboros-ide.css` implements the dark product IDE theme with red/green operational accents, compact panels, right-rail collapse at 1180px, and text-overflow constraints for dense controls.
- `internal/api/execution.go` now provides real workspace command execution. It stores metadata/logs/artifacts under `executions/<id>/`, appends eventlog tool events, and supports run, stop, retry, logs, and artifacts.
- `gui/src/app/api.ts` exposes execution API methods and maps backend persisted executions into the IDE execution panel. It no longer maps toolchain packs into fake execution steps.
- `gui/src/app/OuroborosIDE.tsx` uses a real Chinese command composer and execution panel actions. The old preview-only composer has been removed.
- `scripts/smoke_desktop_gui.sh` verifies a real command execution, real log/artifact files, the API bootstrap projection, and the built Chinese frontend DOM together.
- The settings view now renders current API/workspace/provider/toolchain/backend status from `IDEData`; it no longer ships fixed default fields such as proxy host, port, scan intensity, or managed-bundle paths.
- The startup card and right-rail flow now show live status/counts instead of fixed “ready” or stage labels.
- Execution events are now broadcast on `/api/events/stream`. The frontend subscribes with `connectIDEEventStream` and refreshes bootstrap data for `execution:*` events.
- The desktop GUI smoke uses the real WebSocket stream to require `execution:started` and `execution:completed` for the command it submits.
- `gui/scripts/package-desktop.mjs --dir` now creates a runnable Electron directory under `gui/dist/desktop/Ouroboros-<platform>-<arch>/`, including Electron runtime files, `resources/app`, renderer/main/preload assets, and the Go backend sidecar in `resources/`.
- `scripts/smoke_desktop_package.sh` verifies the package layout and starts the packaged backend binary against a real temporary workspace.
- Electron `workspace:create` now initializes the selected directory through `ouro init` and returns an empty selection on cancel, so canceling the dialog does not reinitialize the current workspace.
- The first-run workbench renders a Chinese workspace welcome screen with open/create actions and visible Chinese operation feedback for opening, initializing, canceling, or failed workspace operations.
- The workspace explorer now shows only file entries returned by the backend `files` projection. When a workspace has no files, it renders `暂无工作区文件` instead of synthesizing evidence/runtime/report/toolchain tree rows.
- `/api/gui/bootstrap` now includes `replayEvents` projected from the SQLite eventlog and `reportFindings` projected from `findings/` files plus real graph findings.
- The frontend consumes backend-projected `replayEvents` and `reportFindings` directly; it only falls back to legacy run-derived replay rows when the aggregate bootstrap endpoint is unavailable.
- `scripts/smoke_desktop_gui.sh` now verifies that a real execution appears in bootstrap replay projection and that a real `findings/high.json` file appears in report findings.
- Added real mission workflow APIs: `GET/PUT /api/mission` read and write `mission.yaml`; `POST /api/mission/start` saves the task and starts a real execution through the existing execution engine.
- `POST /api/mission/start` appends a real `goal` event into the same execution run, so replay shows the operator-provided mission goal instead of a fabricated row.
- `/api/gui/bootstrap` now includes `mission` from `mission.yaml`; the React mission workbench renders and edits that data through a Chinese task-start panel.
- `scripts/smoke_desktop_gui.sh` now starts work through `/api/mission/start`, verifies `mission.yaml`, execution logs/artifacts, bootstrap mission/replay/report fields, and the Chinese DOM.
- Added real provider/model settings APIs: `GET /api/provider` projects `providers.yaml`; `PUT /api/provider` saves OpenAI-compatible provider settings through the existing app provider configuration path.
- `/api/gui/bootstrap` now includes the active provider's type, Base URL, API key environment variable name, enabled state, timeout, context window, capabilities, routing strategy, and status.
- The React settings workbench now has a Chinese editable provider form wired to `saveProviderConfig`; saving writes `providers.yaml` and refreshes bootstrap instead of mutating frontend-only state.
- `scripts/smoke_desktop_gui.sh` now saves provider settings through `/api/provider` and verifies `providers.yaml`, bootstrap, and DOM reflect the real saved model.
- Added real toolchain settings APIs: `GET /api/toolchain` projects workspace `toolchain/*/manifest.yaml`; `PUT /api/toolchain/packs/{id}` updates a pack manifest's name, version, capabilities, runner, run command, timeout, parser, healthcheck, and health command while preserving dependencies.
- `/api/gui/bootstrap` now includes editable toolchain pack manifest fields so React settings consumes backend DTOs rather than deriving toolchain state.
- The React settings workbench now has a Chinese editable toolchain form wired to `saveToolchainPack`; saving writes the pack's real `manifest.yaml` and refreshes bootstrap.
- `scripts/smoke_desktop_gui.sh` now saves toolchain settings through `/api/toolchain/packs/local` and verifies the manifest file and bootstrap reflect the saved values.
- Added real workspace settings APIs: `GET/PUT /api/workspace/settings` read and write ordinary workspace metadata in `workspace.yaml`.
- `/api/gui/bootstrap` now uses the same workspace settings projection so the workspace display name, description, operator, tags, and config path come from backend DTOs.
- The React settings workbench now has a Chinese editable workspace form wired to `saveWorkspaceSettings`; saving writes `workspace.yaml` and refreshes bootstrap.
- `scripts/smoke_desktop_gui.sh` now saves workspace settings and verifies `workspace.yaml`, bootstrap, and DOM reflect the saved workspace name.
- Graph, replay, and report DTOs now carry real cross-reference fields: graph nodes expose paths/run/execution/replay IDs; replay events expose node/execution/file references; report findings expose node/file/run/execution/replay references.
- `findings/` JSON projection now preserves explicit `node_id`, `file_path`, `execution_id`, and `replay_event_id` fields, so report rows can navigate to real graph nodes, replay events, evidence files, and executions without frontend guessing.
- The React replay and report workbenches render Chinese row actions (`定位图谱`, `定位回放`, `打开证据文件`, `查看执行`) only when the backend DTO supplies the corresponding target field.
- `scripts/smoke_desktop_gui.sh` now uses Chromium remote debugging to switch into mission, report, and replay modes and verify real cross-navigation copy against a temporary workspace with real execution/eventlog/findings data.
- The frontend now preserves backend detail fields (`sourceKind`, `createdAt`, raw `data`, replay `runId`, `lane`, `createdAt`) so inspectors can display real DTO content instead of simplified labels.
- Dedicated detail panels now exist for graph nodes, replay events, report findings, evidence files, and execution records. They render Chinese labels and hide empty fields rather than inventing placeholder values.
- The desktop GUI smoke now switches through mission, attack graph, report, evidence file, and replay views, verifying `节点详情`, `执行详情`, `发现详情`, `证据文件`, `回放详情`, `原始字段`, and `关联对象` against real workspace data.
- Electron now builds a real Chinese desktop menu with workspace open/create, recent workspaces, mode switching, refresh, execution-panel access, and backend restart actions.
- `BackendManager` now emits lifecycle status, records `lastError`, distinguishes intentional stop from crash, marks unexpected process exit as `crashed`, and exposes `restart()`.
- `DesktopStateManager` persists window bounds/maximized state to `window-state.json`; `WorkspaceManager` persists `recentWorkspaces` in the same real Electron user-data state path.
- Preload now exposes desktop lifecycle bridge methods for backend restart, recent workspaces, open recent workspace, window state, menu actions, and backend status events.
- React now consumes real desktop lifecycle state through `loadDesktopStatus` and `restartDesktopBackend`, rendering a Chinese desktop status banner for starting/crashed/offline states instead of inferring backend state in the renderer.
- The packaged desktop smoke now verifies that the built app bundle contains lifecycle main/preload/backend/state files, restart/window-state IPC strings, and Chinese crash/restart UI text in renderer assets.
- `/api/gui/bootstrap` now exposes `toolActions` projected from real workspace toolchain manifests. Only packs with a real `run_command` become executable GUI actions, so empty/non-executable manifests produce a real empty state instead of frontend-authored tool rows.
- `/api/toolchain/actions` lists the same manifest-backed actions and `/api/toolchain/actions/{id}/run` launches them through the existing execution pipeline. The execution receives `OURO_TOOL_ACTION_ID`, `OURO_TOOLCHAIN_PACK_ID`, `OURO_TOOLCHAIN_PACK_NAME`, `OURO_TOOL_ARGUMENTS`, and `OURO_TOOL_MANIFEST_PATH`, and eventlog payloads keep action/pack/argument metadata.
- The frontend maps backend `toolActions` directly into `IDEData`, renders a Chinese `ToolLaunchPanel`, accepts editable arguments per action, calls `runToolchainAction`, and refreshes execution state from the returned real execution DTO. It does not infer runnable tool rows from pack lists.
- The desktop GUI smoke now writes a real `toolchain/local-pack/manifest.yaml` run command, launches it through `/api/toolchain/actions/local/run`, verifies `工具动作日志`, verifies the argument artifact under the execution artifact directory, verifies bootstrap `toolActions`, and checks the rendered Chinese tool-launch controls.
- A replay smoke failure after clicking `打开证据文件` was traced to a UI race, not missing backend replay data. `onOpenFindingFile` previously awaited the file API before switching to mission mode; when the smoke immediately clicked replay, the late mission-mode update could overwrite replay. Moving `setMode('mission')` before the await fixes the product navigation behavior.
- The smoke now waits for the real file editor panel (`文件编辑器` + `findings/high.json` + `证据文件`) before switching to replay, so a report-page `打开证据文件` button label cannot satisfy the evidence-file assertion.
- The command palette is now a real searchable command surface. It calls existing workspace/backend/mission/execution/mode actions for refresh, workspace open/create, backend restart, current command run, mission save/start, and workbench switching instead of rendering static mode shortcuts.
- The visible `Ctrl K` hint is now backed by a real `Ctrl/Cmd+K` keyboard listener, and `Escape` closes the palette. This removes a stale UI affordance that previously looked like software behavior without doing anything.
- The desktop GUI smoke now exercises the command palette by opening it, verifying `刷新真实数据`, `运行当前命令`, and `保存当前任务`, executing refresh, and then continuing through graph/report/evidence/replay assertions.
- Execution artifact listings now include backend-authored workspace-relative `filePath` values, for example `executions/<id>/artifacts/result.txt`. The renderer consumes that DTO field directly instead of joining `artifactDir` and artifact names.
- The mission execution panel now exposes `打开日志文件` and `打开产物文件`. These actions call the same workspace file API as the explorer/evidence views, so logs and artifacts open in the real file editor with actual file content.
- GUI smoke now covers the execution-to-file path by loading logs/artifacts, clicking `打开产物文件`, and waiting for the file editor to show the real `result.txt` artifact.
- Execution details now derive related graph/replay/report targets from backend DTO references: `GraphNode.executionId`, `ReplayEvent.executionId`, and `ReportFinding.executionId`. The detail panel renders `定位图谱`, `定位回放`, and `定位报告` only when those references exist.
- Report finding selection now lives in the main shell, not only inside `ReportWorkbench`, so external actions can open the report view on a specific backend-projected finding.
- GUI smoke now proves execution-detail cross-navigation by jumping from execution to report, back to execution, to replay, back to execution, and to graph.
- Report finding details now match `ReportFinding.executionId` to the backend-projected execution list, can load the linked execution log/artifact list, and open log/artifact files through backend-authored `logPath` and artifact `filePath` values.
- The renderer tracks `executionArtifactsExecutionId` so artifacts loaded for one execution are not displayed as if they belonged to another report finding.
- GUI smoke now proves report-detail execution-file linkage by loading logs/artifacts from the report finding detail, opening the real `result.txt` artifact, and continuing normal report/execution/replay/graph navigation.
- Replay event details now match `ReplayEvent.executionId` to backend-projected executions and match report findings through `ReportFinding.replayEventId === selectedEvent.id`, so replay can open linked execution files and focus the related report finding without frontend-authored rows.
- GUI smoke now proves replay-detail execution-file linkage by loading logs/artifacts from the replay detail, opening the real `result.txt` artifact, returning to replay, and jumping to the related report finding.
- Graph node details now match selected nodes to backend-projected report findings and executions. The matching prefers `ReportFinding.nodeId === selectedNode.id`, then explicit `executionId` / `replayEventId` DTO references for execution-derived graph nodes.
- Graph node details can load linked execution logs/artifacts, open backend-authored log/artifact paths, and focus the related report finding without frontend-authored rows.
- GUI smoke now proves graph-detail execution-file linkage by selecting the real `findings/high.json` graph node, loading logs/artifacts from node detail, opening the real `result.txt` artifact, and jumping to the related report finding.
- The graph toolbar `导出图谱` action is now backed by `POST /api/evidence/graph/export`. The backend writes `exports/attack-graph.json` from `ProjectEvidenceGraph` output and returns a workspace-relative `filePath` for the renderer to open.
- The renderer calls `exportEvidenceGraph`, refreshes backend bootstrap data, and opens the exported graph JSON through the existing workspace file reader/editor instead of constructing export content in React.
- The report page `导出报告` action is now backed by `POST /api/reports/export`. The backend writes `reports/ouroboros-report.md` from workspace metadata, existing report files, and `ProjectReportFindings` output.
- The renderer calls `exportReport`, refreshes backend bootstrap data, and opens the exported Markdown through the existing workspace file reader/editor instead of synthesizing report content in React.
- Graph/report export action responses now require backend-authored `filePath`, count fields, and `modifiedAt`. The renderer throws into the existing Chinese export-failed state instead of opening an empty path or displaying fabricated zero-count export results when a backend response is incomplete.
- `/api/gui/bootstrap` already returned `reports`, but the frontend dropped them during mapping. `IDEData.reports` now preserves those backend-projected report files for report-page sections.
- The report TOC had real-looking buttons but no section state. It now switches `ReportSectionContent` views for summary, project data, methods, findings, risk distribution, remediation state, and appendix evidence/report links.
- The settings navigation had the same static-affordance problem: it rendered categories with a fixed first active button while all groups were shown at once. It now switches real sections for current status, workspace settings, provider settings, and toolchain settings.
- The custom title bar rendered three window-control squares without any Electron bridge. It now calls real `window:minimize`, `window:toggle-maximize`, and `window:close` IPC handlers through preload and React API wrappers.
- Electron desktop state already tracked recent workspaces, but the React command palette could not open them. The palette now projects real `DesktopStatus.recentWorkspaces` entries into executable `打开最近工作区` actions.
- Desktop visual verification found and fixed a 1440px clipping issue in the mission header/side panel.
- API-backed visual verification used `test-workspace` through `go run ./cmd/ouro serve test-workspace --port 18080`; the shell rendered real workspace files, `offline-model`, toolchain totals from manifests, and connection latency from backend responses.
- `test-workspace` currently has no evidence graph nodes in eventlog/evidence directories, so `/api/evidence/graph` correctly returns empty arrays instead of fabricated nodes.
- `authorization.yaml` and `scope.yaml` appear only as ordinary workspace YAML files in the file tree, not as product state.
- Runnable tool actions now require backend-authored `commandText` before they appear in the GUI. The renderer no longer joins `command` arrays into visible text, so display-ready command copy remains a backend/toolchain DTO responsibility.
- Runnable tool actions now require backend-authored `status` and non-empty `command` arrays before they appear in the GUI. Tool action run responses also require a complete backend action DTO; the renderer no longer maps a missing action response to an empty frontend action object.
- Replay event visible detail now stays backend-authored. If the backend omits `detail`, the renderer leaves the detail empty instead of displaying `runId` as a pseudo-description.
- Report finding affected endpoints now stay backend-authored. If the backend omits `endpoint`, the renderer leaves the field empty instead of displaying the finding path or source as a pseudo-endpoint.
- Report finding and report document titles now stay backend-authored. Rows missing titles are dropped instead of displaying file paths or generic report copy as pseudo-titles.
- Workspace file tree labels now require backend `file.path`. Rows missing file paths are dropped instead of displaying `file.name` as a pseudo-path.
- Execution row labels now come from `ExecutionDTO.label`. The backend sets the label to the execution display command, the renderer drops execution rows/action responses that lack a backend label instead of splitting command text, and the old `zhCN.execution.commandFallback` copy was removed.
- Execution command text now stays backend-authored. The renderer drops execution rows/action responses that lack `ExecutionDTO.command` instead of mapping missing commands to empty strings.
- Execution status now stays backend-authored. The renderer drops execution rows/action responses that lack `ExecutionDTO.status` or contain an unrecognized execution status instead of defaulting missing/unknown state to `ready`.
- Graph node labels and types now stay backend-authored. The renderer drops graph nodes missing backend `label`/`type` or carrying unrecognized types instead of displaying node IDs as labels or mapping unknown graph semantics to `action`.
- Backend `Artifact` graph nodes now have a first-class frontend kind and Chinese `产物` copy, so evidence/artifact file nodes no longer collapse into generic action semantics.
- Execution artifact metadata now stays backend-authored. The renderer drops artifact rows missing backend `filePath`, `size`, or `modifiedAt` instead of displaying fabricated zero-byte/zero-time metadata or exposing artifacts that cannot be opened through a backend-authored workspace path.

## Issues Encountered
| Issue | Resolution |
|-------|------------|
| Missing project-local impeccable scripts | Followed installed skill references and created contracts manually. |
| Planning files were template-only after timeout | Replaced them with a concrete rebuild plan and findings. |
| Current API returned mock graph/status | Added API DTO/projector code, GUI bootstrap, workspace files, tests, and mock-literal scanner. |
| 1440px shell screenshot clipped the primary action | Reduced mission grid minimums and gave header/panel text explicit shrink behavior. |
| Execution panel displayed toolchain readiness as pseudo-steps | Added persisted execution APIs and changed the frontend to render only real backend execution records. |
| Desktop GUI smoke expected a different log phrase than the command emitted | Inspected API log JSON and Chromium DOM, then corrected the assertion to the actual command output. |
| Frontend synthesized explorer rows when no file list was present | Added a GUI contract and changed the explorer to render a real empty-file state. |
| Replay/report views depended on frontend inference from runs/graph | Added backend `replayEvents` and `reportFindings` projections and mapped them directly in the frontend. |
| Mission workbench only had a raw command composer | Added a direct mission-start form backed by `mission.yaml` and `/api/mission/start`, while keeping raw command execution as an operator shortcut. |
| Report rows had no real way to jump back to graph/replay/file/execution targets | Added cross-reference DTO fields, preserved explicit references from findings JSON, and rendered row actions only when those fields are present. |
| `--dump-dom` smoke only covered the default mission view | Replaced the final DOM smoke with Chromium DevTools interaction that switches to report and replay modes before asserting navigation copy. |
| Detail views were mostly summaries instead of inspectors | Added real DTO-backed detail panels for graph, replay, finding, evidence file, and execution surfaces. |
| Phase 6 lifecycle features could exist only in source without package coverage | Extended `scripts/smoke_desktop_package.sh` to assert packaged lifecycle bridge files, IPC channels, crash status handling, and Chinese recovery copy. |
| Manifest-backed tool launch could have been inferred in the renderer | Added backend action DTO projection and action run endpoints so the GUI only renders executable tool actions the backend can prove from manifest `run_command`. |
| Evidence-file smoke wait matched stale report button copy | Added a real file-editor wait and moved evidence-file mode switching before the async file read so later replay navigation cannot be overwritten. |
| Command palette was a static mode switcher with read-only input | Replaced it with searchable real actions wired to existing desktop/backend/mission/execution callbacks and added GUI contract/smoke coverage. |
| Execution artifacts were visible but not openable as files | Added backend `filePath` to artifact DTOs and frontend log/artifact open actions that route through the existing workspace file reader. |
| Vite build missed an undefined runtime prop | Chromium smoke and CDP exception capture found `ReferenceError: onOpenExecutionFile is not defined`; fixed by passing the callback through `MissionWorkbench`. |
| Execution details showed IDs but did not navigate back to graph/replay/report | Added backend-DTO reference matching and lifted report selection state so execution details can jump to the corresponding graph node, replay event, and report finding. |
| Report finding details showed execution IDs but could not open linked execution files | Passed backend-projected executions, logs, artifacts, and artifact ownership state into `ReportWorkbench` so finding details can load/open real execution files without path reconstruction. |
| Replay event details could inspect execution but not open linked execution files or report finding | Passed backend-projected executions, findings, logs, artifacts, and artifact ownership state into `ReplayWorkbench` so replay details can load/open real execution files and focus the related report finding. |
| Graph node details could inspect execution but not open linked execution files or report finding | Passed backend-projected executions, findings, logs, artifacts, and artifact ownership state into `GraphWorkbench`; graph finding matching now uses explicit node/execution/replay DTO references. |
| Graph toolbar had a static export button with no real software action | Added backend graph export API, frontend API client, Chinese export status copy, and GUI smoke coverage that opens the exported workspace file. |
| Report page had static report copy but no persisted report export action | Added backend report export API, frontend API client, Chinese export status copy, and GUI smoke coverage that opens the exported Markdown workspace file. |
| Report TOC buttons looked interactive but did not switch content | Added `reportActiveSection`, real report section definitions, backend-projected report document mapping, and GUI smoke clicks for risk/project/appendix sections. |
| Settings navigation looked interactive but kept the first item active | Added `settingsActiveSection`, real settings section definitions, `data-settings-section` smoke coverage, and kept all editable fields backed by existing DTO/save APIs. |
| Title-bar window controls were decorative squares | Added main-process window IPC, preload bridge methods, React title-bar buttons with Chinese labels, and package smoke checks for packaged IPC/copy. |
| Recent workspaces were only reachable through the native desktop menu | Added `openRecentDesktopWorkspace`, command palette actions for real recent paths, and GUI smoke bridge injection that executes `打开最近工作区` against the temporary workspace. |
| A previous full API test run reported `unexpected end of JSON input` in `TestServerExecutionRunLogsArtifactsAndEvents` | Re-ran the focused execution API test and the full `internal/api` package; both passed on fresh runs, so no code change was made for the non-reproduced failure. |
| Empty provider state still carried frontend-authored defaults | Changed empty provider data in the renderer and API fallback to remain unconfigured until backend DTOs provide real values, and added contract checks for type/enabled/routing defaults. |
| Settings select controls exposed internal config values as visible labels | Kept the saved values (`openai_compatible`, `native`, `command`, `built_in`, `exec`) but changed the visible labels to Chinese copy from `zh-cn.ts`. |
| Replay/report fallback still derived business rows in React | Removed frontend derivation from legacy runs, graph nodes, and report files; replay events and report findings now render only from backend bootstrap DTO projections. |
| Mission workbench generated fake system/agent conversation rows | Removed `mapMessages` frontend synthesis, renamed the panel to `任务操作`, and render a Chinese empty state unless backend-projected task messages are introduced later. |
| Right rail presented real counts as an agent graph and workflow fallback invented a next step | Renamed the count summary to `工作区概览`, removed stale `下一步` copy, and changed workflow status fallback to backend DTO status or explicit unknown/offline. |
| Toolchain settings still fabricated runner and healthcheck defaults | Preserved missing manifest runner/healthcheck values as unconfigured in the API mapper and empty settings draft, with explicit Chinese `未配置` select options. |
| WebSocket event stream could panic on concurrent writes | Serialized welcome, heartbeat, ping, and execution event writes through a shared server-side WebSocket write helper so desktop refresh events do not race the Gorilla writer. |
| Frontend mapper generated object identities and config paths when backend fields were absent | Added GUI contract checks and changed the mapper to drop incomplete replay/report/graph/file/execution rows, keep missing workspace config paths unconfigured, and reject action responses without a real execution DTO. |
| Renderer still assembled aggregate IDE state from legacy API endpoints when bootstrap failed | Removed the legacy endpoint fallback path and made `/api/gui/bootstrap` the sole aggregate GUI data source; bootstrap failure now produces a Chinese unavailable state without composing graph/toolchain/run/workspace facts in React. |
| Mission panel kept dormant `MissionMessage` / `MessageBubble` / `.message.agent` structures without backend DTO support | Removed the unused message type, row renderer, API field, and stale styles; the panel now shows a Chinese empty state until a real backend message projection is introduced. |
| Mission panel still used a `Bot` icon and replay `agent` events rendered as `智能体` | Switched the mission operation panel to a neutral command icon/type and localized backend `agent` replay events as `执行单元`, with contract coverage to prevent stale Bot/智能体 UI semantics. |
| Settings placeholders still used concrete English/example values | Added GUI contract bans for `Base URL`, provider capability samples, shell command samples, and workspace tag samples; replaced them with Chinese instructional copy while preserving real backend DTO/config values. |
| Replay/report mappers still used real secondary fields as ID fallbacks | Added GUI contract bans for `event.id || event.runId`, `finding.id || finding.path`, and `report.id || report.path`; renderer now drops rows without backend-authored IDs instead of assigning identities from other fields. |
| Toolchain/action/file mappers still used real secondary fields as ID fallbacks | Added GUI contract bans for pack name, action pack ID, file path/name, and generic `toolchain` key fallbacks; renderer now drops those rows unless backend DTOs provide explicit IDs, and runnable tool actions also require `packId`. |
| Graph edge mapper still generated edge identities from render data | Added a GUI contract ban for the `from`/`to`/index template ID; renderer now consumes backend `GraphEdgeDTO.id` and drops edges without backend-authored IDs. |
| Graph edge mapper still invented a default relation type | Added a GUI contract ban for `kind: edge.type || 'related'`; renderer now requires backend `GraphEdgeDTO.type` and drops incomplete edge DTOs instead of inventing relation semantics. |
| Graph node mapper still inferred visible labels and semantics | Added GUI contract bans for id-only node filtering, `node.label || id`, optional node type normalization, and default action mapping; renderer now requires backend-authored labels and recognized backend-authored types. |
| Backend `Artifact` graph nodes were previously collapsed into action semantics | Added frontend `artifact` graph node kind and Chinese `产物` copy so backend `Artifact` DTOs render as real artifact nodes instead of using the generic action bucket. |
| Execution artifact mapper still used path/name interchangeably | Added GUI contract bans for artifact `path` from `name` and `name` from `path`; renderer now drops artifact rows unless backend DTOs provide both fields. |
| Execution artifact mapper still fabricated metadata | Added GUI contract bans for incomplete artifact filtering and `size` / `modifiedAt` zero fallbacks; renderer now requires backend `filePath`, `size`, and `modifiedAt` before showing artifact rows. |
| Tool action mapper still inferred visible labels | Added a GUI contract ban for label fallback from `packName` or action `id`; renderer now requires backend-authored tool action labels before exposing runnable actions. |
| Tool action mapper still inferred visible command text | Added a GUI contract ban for `commandText: action.commandText || (action.command ?? []).join(' ')`; renderer now requires backend-authored `commandText` before exposing runnable actions. |
| Tool action mapper still accepted incomplete action DTOs | Added GUI contract bans for incomplete tool-action filtering, missing action-run response fallback to `{}`, and empty command-array fallback; renderer now requires complete backend action DTOs for listing and run responses. |
| Export action mappers still fabricated empty paths and zero counts | Added GUI contract bans for graph/report export `filePath`, count, and timestamp fallback expressions; renderer now requires complete backend export DTOs before reporting success or opening the exported workspace file. |
| Replay mapper still used a technical run ID as visible detail | Added a GUI contract ban for `detail: event.detail || event.runId || ''`; renderer now consumes backend `detail` directly and leaves missing detail empty. |
| Report finding mapper still used file/source metadata as visible endpoint | Added a GUI contract ban for `endpoint: finding.endpoint || finding.path || finding.source || ''`; renderer now consumes backend `endpoint` directly and leaves missing endpoint empty. |
| Report mappers still inferred visible titles | Added GUI contract bans for finding/report title fallbacks; renderer now drops report findings/documents without backend-authored titles instead of showing paths or generic report copy. |
| Report mappers still tolerated incomplete severity/status/path DTO fields | Added GUI contract bans for finding severity fallback to `info`, report status translation from optional fields, and report path fallback to empty strings; renderer now drops report findings without valid backend severity and report documents without backend status/path. |
| Report appendix still displayed technical fallback text as evidence | Added GUI contract bans for appendix `filePath/path/source/id` and relation placeholder fallbacks; appendix now renders only findings with backend-authored file paths and shows relation IDs only when backend DTOs include them. |
| Workspace file mapper still used file names as path labels | Added a GUI contract ban for `const label = file.path || file.name || ''`; renderer now requires backend `file.path` for file-tree rows. |
| Workspace mapper still derived display names from workspace paths | Backend `ProjectWorkspace` already provides old-workspace directory-name compatibility; added GUI contract bans for frontend `workspaceName()` path splitting and changed the renderer to use backend `workspace.name` only for connected workspace display names. |
| Execution mapper still derived visible labels from command text | Added `ExecutionDTO.label`, a focused API test, and a GUI contract ban for command-splitting labels; renderer now requires backend-authored execution labels. |
| Execution mapper still tolerated missing command text | Added a GUI contract ban for `const command = execution.command || ''`; renderer now requires backend `ExecutionDTO.command` for execution rows and action responses. |
| Execution mapper still treated missing status as ready | Added GUI contract bans for the old incomplete execution checks and optional status normalizer; renderer now requires backend-authored execution status and drops missing or unrecognized execution states. |
| Toolchain pack mapper still tolerated incomplete visible pack state | Added GUI contract bans for id-only pack filtering, empty id/name fallbacks, and zero `toolsReady` fallbacks; renderer now drops pack rows and summary count inputs unless backend DTOs provide `id`, `name`, `status`, and numeric `toolsReady`. |
| Mission mapper still tolerated incomplete task DTO state | Added GUI contract bans for mission `path` and `status` fallbacks; renderer now requires backend-authored mission path/status for save/start responses and falls back to the real empty mission state for incomplete bootstrap DTOs. |
| Replay event mapper still tolerated incomplete timeline DTO state | Added GUI contract bans for missing replay type/timestamp handling; renderer now requires backend-authored replay `id`, `type`, `lane`, and numeric `createdAt`, and drops unknown event types instead of displaying a generic event label or placeholder time. |
| Workspace file mapper still tolerated incomplete file-kind metadata | Added GUI contract bans for missing/unknown workspace file kind fallbacks; renderer now requires backend-authored recognized file kinds before displaying file-tree metadata, while backend `file` remains the explicit generic file kind. |

## Resources
- `ui-designs/main_ui.png`
- `ui-designs/empty_state.png`
- `ui-designs/start_pentest_modal.png`
- `ui-designs/preflight_check.png`
- `ui-designs/command_palette.png`
- `ui-designs/tool_execution.png`
- `ui-designs/node_inspector.png`
- `ui-designs/replay_view.png`
- `ui-designs/report_view.png`
- `ui-designs/settings_panel.png`
- `ui-designs/ouro_attack_graph.png`
- `gui/package.json`
- `gui/src/renderer/App.tsx`
- `gui/src/app/OuroborosIDE.tsx`
- `gui/src/app/types.ts`
- `gui/src/app/api.ts`
- `gui/src/app/zh-cn.ts`
- `gui/src/main/preload.js`
- `gui/src/main/backend.js`
- `gui/src/main/workspace.js`
- `gui/src/styles/ouroboros-ide.css`
- `gui/scripts/check-gui-contract.mjs`
- `gui/scripts/check-product-contract.mjs`
- `gui/scripts/check-desktop-contract.mjs`
- `internal/api/server.go`
- `internal/api/dto.go`
- `internal/api/projector.go`
- `internal/api/execution.go`
- `scripts/check_no_gui_mock_literals.sh`
- `scripts/smoke_desktop_gui.sh`
- `scripts/smoke_desktop_package.sh`
