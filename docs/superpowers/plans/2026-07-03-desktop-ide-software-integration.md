# Desktop IDE Software Integration Execution Plan

> **For agentic workers:** This plan is subordinate to `docs/superpowers/plans/2026-07-03-desktop-ide-software-master-plan.zh-CN.md`. Implement the Chinese master plan as the source of truth.

**Goal:** Turn Ouroboros into a packaged desktop IDE with an Electron/React frontend, Go backend sidecar, real workspace data, direct execution controls, replay, evidence graph, reports, settings, and Chinese UI.

**Architecture:** Electron owns the desktop shell, workspace selection, backend process lifecycle, preload bridge, and packaged resources. Go owns workspace, eventlog/replay, evidence, toolchain, provider, worker/runtime, execution, reports, and GUI DTO projection. React consumes `/api/gui/bootstrap` and event streams through the desktop bridge.

**Tech Stack:** Go, Cobra, SQLite eventlog, Electron, Vite, React, TypeScript, lucide-react, Chromium screenshot checks.

---

## Priority Order

1. Update product/design contracts and checks so the project points at desktop software, real data, Chinese UI, and direct software capabilities.
2. Replace mock Go API responses with GUI DTOs projected from real workspace artifacts.
3. Add Electron preload, workspace manager, and Go backend sidecar lifecycle.
4. Move React data loading to `/api/gui/bootstrap` through `window.ouroboros`.
5. Add workspace file tree, file open/edit, search, recent workspace, and settings backed by real config.
6. Add execution APIs and UI for run, stop, retry, logs, exit code, artifacts, and event history. **Complete for the first real slice.**
7. Project evidence graph, replay timeline, and reports from real artifacts. **Complete for eventlog replay rows, findings/report file projection, graph/replay/report cross-navigation, and DTO-backed detail panels.**
8. Package the desktop app with the Go backend binary and add smoke tests. **Complete for runnable directory packaging.**
9. Add direct mission-start workflow from `mission.yaml` to real execution. **Complete for task save/start and smoke coverage.**
10. Make settings editable for provider/model/toolchain/workspace configuration. **Complete for provider/model, toolchain, and workspace settings.**
11. Harden Electron desktop lifecycle: menu actions, recent workspaces, window state, backend crash/restart, and packaged lifecycle smoke. **Complete for the first desktop lifecycle slice.**
12. Add manifest-backed GUI tool launch from real toolchain manifests. **Complete for action projection, editable arguments, execution records, eventlog metadata, contracts, and smoke coverage.**
13. Turn command palette into a real IDE action surface. **Complete for searchable real actions, keyboard shortcut, contract coverage, and GUI smoke coverage.**
14. Open execution logs and artifacts as workspace files. **Complete for backend artifact file paths, frontend file-opening actions, contract coverage, and GUI smoke coverage.**
15. Link execution details back into graph, replay, and report views through backend DTO references. **Complete for execution detail target derivation, focused report selection, contract coverage, and GUI smoke coverage.**
16. Link report finding details back into execution files. **Complete for linked execution DTO matching, report-side log/artifact loading, backend-authored file opening, contract coverage, and GUI smoke coverage.**
17. Link replay event details back into execution files and report findings. **Complete for replay-side execution/finding DTO matching, log/artifact loading, report focus, contract coverage, and GUI smoke coverage.**
18. Link graph node details back into execution files and report findings. **Complete for graph-side execution/finding DTO matching, log/artifact loading, report focus, contract coverage, and GUI smoke coverage.**
19. Replace the static graph export affordance with a real backend export action. **Complete for `/api/evidence/graph/export`, workspace file writing, frontend open-file flow, contract coverage, and GUI smoke coverage.**
20. Add real report export from backend-projected findings. **Complete for `/api/reports/export`, Markdown workspace file writing, frontend open-file flow, contract coverage, and GUI smoke coverage.**

## Current P0 Tasks

### Task 1: Contracts

Files:
- `PRODUCT.md`
- `DESIGN.md`
- `docs/superpowers/specs/2026-07-03-gui-rebuild-design.md`
- `docs/superpowers/plans/2026-07-03-desktop-ide-software-master-plan.zh-CN.md`
- `gui/scripts/check-product-contract.mjs`
- `gui/scripts/check-gui-contract.mjs`

Acceptance:
- Product direction is desktop IDE software, not Web UI preview.
- Master plan is in Chinese and emphasizes complete software capabilities.
- GUI contract catches fixture data, visible English labels, old field names, and mock literals.

### Task 2: Real Go GUI DTO

Files:
- Create `internal/api/dto.go`
- Create `internal/api/projector.go`
- Modify `internal/api/server.go`
- Add focused tests under `internal/api/`

Acceptance:
- `/api/gui/bootstrap` returns workspace, workflow, provider, toolchain, graph, runs, reports, settings, and backend status.
- `/api/gui/bootstrap` returns eventlog-derived `replayEvents` and workspace-derived `reportFindings`.
- Graph/toolchain/runs/report endpoints no longer return demo nodes, demo findings, generated IP addresses, or fixed tool counts.
- Empty workspaces return empty states, not fabricated business data.

### Task 3: Desktop Sidecar

Files:
- Create `gui/src/main/preload.js`
- Create `gui/src/main/backend.js`
- Create `gui/src/main/workspace.js`
- Modify `gui/src/main/main.js`
- Create `gui/scripts/check-desktop-contract.mjs`

Acceptance:
- Electron can choose a workspace, start Go backend on an available localhost port, wait for `/api/health`, expose `apiBaseURL`, and stop the backend on quit.

### Task 4: Frontend Bootstrap Data Flow

Files:
- `gui/src/app/api.ts`
- `gui/src/app/types.ts`
- `gui/src/app/OuroborosIDE.tsx`
- `gui/src/app/zh-cn.ts`

Acceptance:
- Renderer loads `/api/gui/bootstrap` via `window.ouroboros` when running in Electron.
- Dev fallback can still use an environment API URL.
- Visible UI is Chinese.
- Current UI shows real empty/error/loading states.
- First-run UI can open an existing workspace or initialize a new workspace through the desktop bridge, with Chinese feedback for cancel/failure states.
- The explorer displays only backend-returned file entries; it does not synthesize evidence/runtime/report/toolchain rows.

### Task 5: Real Execution Slice

Files:
- `internal/api/execution.go`
- `internal/api/dto.go`
- `internal/api/projector.go`
- `internal/api/server.go`
- `internal/api/server_test.go`
- `gui/src/app/api.ts`
- `gui/src/app/types.ts`
- `gui/src/app/OuroborosIDE.tsx`
- `gui/src/app/zh-cn.ts`
- `gui/src/styles/ouroboros-ide.css`
- `scripts/smoke_desktop_gui.sh`
- `scripts/smoke_desktop_package.sh`
- `Makefile`

Acceptance:
- `POST /api/executions` runs a real command in the selected workspace.
- Execution metadata, log, artifact directory, status, exit code, and retry linkage are persisted under `executions/<id>/`.
- Stop, retry, logs, and artifacts are exposed as direct API actions.
- `/api/gui/bootstrap` includes real persisted executions.
- The mission workbench command composer calls the backend; it is not a preview-only UI.
- The execution panel shows real status, logs, artifacts, and exit code in Chinese.
- `/api/events/stream` broadcasts execution lifecycle events and the frontend refreshes from those events.
- `make smoke-desktop-gui` verifies API + frontend against a temporary real workspace.
- The smoke verifies bootstrap `replayEvents` and `reportFindings` after a real execution and a real findings file.

### Task 6: Direct Mission Start Workflow

Files:
- `internal/api/dto.go`
- `internal/api/mission.go`
- `internal/api/projector.go`
- `internal/api/server.go`
- `internal/api/server_test.go`
- `gui/src/app/api.ts`
- `gui/src/app/types.ts`
- `gui/src/app/OuroborosIDE.tsx`
- `gui/src/app/zh-cn.ts`
- `gui/src/styles/ouroboros-ide.css`
- `scripts/smoke_desktop_gui.sh`

Acceptance:
- `GET /api/mission` reads real `mission.yaml`.
- `PUT /api/mission` saves task name, goal, and command while preserving existing mission fields where possible.
- `POST /api/mission/start` persists the task, starts a real execution, writes a goal event to eventlog, and returns both mission and execution DTOs.
- `/api/gui/bootstrap` includes `mission`.
- The mission workbench renders a Chinese task-start panel backed by real API calls.
- The desktop GUI smoke starts work through `/api/mission/start`, not a fake UI flow.

### Task 7: Real Replay And Report Projection

Files:
- `internal/api/dto.go`
- `internal/api/projector.go`
- `internal/api/projector_test.go`
- `gui/src/app/api.ts`
- `gui/src/app/types.ts`
- `gui/src/app/zh-cn.ts`
- `scripts/smoke_desktop_gui.sh`

Acceptance:
- `ReplayEventDTO` rows are projected from SQLite eventlog events with event type, lane, timestamp, run id, and detail.
- `ReportFindingDTO` rows are projected from `findings/` JSON/Markdown files and real graph finding nodes.
- `GraphNodeDTO`, `ReplayEventDTO`, and `ReportFindingDTO` carry real node/file/execution/replay references when the backend can prove them from eventlog, workspace files, execution metadata, or findings JSON.
- Findings JSON preserves explicit `node_id`, `file_path`, `execution_id`, and `replay_event_id` references instead of forcing the frontend to infer them.
- React report/replay modes consume backend-projected fields directly instead of fabricating rows from runs or graph nodes.
- React report/replay rows render Chinese cross-navigation actions only when the DTO includes the corresponding target.
- React graph/replay/report/file/execution surfaces expose dedicated Chinese detail panels backed by DTO fields, workspace file APIs, and execution APIs.
- Detail panels preserve raw backend fields where useful (`sourceKind`, `createdAt`, raw node `data`, replay lane/run/timestamp) and hide empty fields instead of showing fake placeholders.
- `npm --prefix gui run test:contract` rejects frontend mapper regressions that stop consuming bootstrap projections.
- `scripts/smoke_desktop_gui.sh` verifies graph/replay/report cross-navigation and detail copy by driving mission, attack graph, report, evidence-file, and replay modes against a temporary real workspace.

### Task 8: Runnable Desktop Directory Packaging

Files:
- `gui/scripts/package-desktop.mjs`
- `gui/package.json`
- `gui/scripts/check-desktop-contract.mjs`
- `scripts/smoke_desktop_package.sh`
- `Makefile`

Acceptance:
- `npm --prefix gui run package:desktop:dir` builds the frontend and backend sidecar, copies the Electron runtime, writes `resources/app/package.json`, and creates `gui/dist/desktop/Ouroboros-<platform>-<arch>/`.
- The package directory contains the Electron app binary, renderer assets, main/preload scripts, and the Go backend sidecar under `resources/`.
- `npm --prefix gui run package:desktop:dir:dry-run` prints the package steps without mutating state.
- `make smoke-desktop-package` verifies the package layout and starts the packaged backend sidecar against a real temporary workspace.

### Task 9: Editable Provider And Model Settings

Files:
- `internal/api/dto.go`
- `internal/api/provider.go`
- `internal/api/projector.go`
- `internal/api/server.go`
- `internal/api/server_test.go`
- `gui/src/app/api.ts`
- `gui/src/app/types.ts`
- `gui/src/app/OuroborosIDE.tsx`
- `gui/src/app/zh-cn.ts`
- `gui/src/styles/ouroboros-ide.css`
- `gui/scripts/check-gui-contract.mjs`
- `scripts/smoke_desktop_gui.sh`

Acceptance:
- `GET /api/provider` projects real `providers.yaml` provider configuration.
- `PUT /api/provider` persists provider name, Base URL, model, API key environment variable name, enabled state, timeout, context window, capabilities, and routing strategy to `providers.yaml`.
- `/api/gui/bootstrap` exposes active provider fields from the same backend projection.
- The settings workbench renders Chinese editable provider/model controls and saves through the backend API.
- The frontend never stores a raw API key value; it stores the environment variable name.
- The desktop GUI smoke verifies provider settings persistence in `providers.yaml`, bootstrap JSON, and the rendered DOM.

### Task 10: Editable Toolchain Settings

Files:
- `internal/api/dto.go`
- `internal/api/toolchain.go`
- `internal/api/projector.go`
- `internal/api/server.go`
- `internal/api/server_test.go`
- `gui/src/app/api.ts`
- `gui/src/app/types.ts`
- `gui/src/app/OuroborosIDE.tsx`
- `gui/src/app/zh-cn.ts`
- `gui/scripts/check-gui-contract.mjs`
- `scripts/smoke_desktop_gui.sh`

Acceptance:
- `GET /api/toolchain` projects real workspace toolchain manifests.
- `PUT /api/toolchain/packs/{id}` persists editable pack fields to `toolchain/<pack>/manifest.yaml`.
- The writer preserves dependencies while updating name, version, capabilities, runner, run command, timeout, parser, healthcheck, and health command.
- `/api/gui/bootstrap` exposes editable manifest fields from the same backend projection.
- The settings workbench renders Chinese editable toolchain controls and saves through the backend API.
- The desktop GUI smoke verifies toolchain settings persistence in the manifest file and bootstrap JSON.

### Task 11: Editable Workspace Settings

Files:
- `internal/api/dto.go`
- `internal/api/workspace_settings.go`
- `internal/api/projector.go`
- `internal/api/server.go`
- `internal/api/server_test.go`
- `gui/src/app/api.ts`
- `gui/src/app/types.ts`
- `gui/src/app/OuroborosIDE.tsx`
- `gui/src/app/zh-cn.ts`
- `gui/scripts/check-gui-contract.mjs`
- `scripts/smoke_desktop_gui.sh`

Acceptance:
- `GET /api/workspace/settings` projects real `workspace.yaml` metadata, with directory-name fallback for older workspaces.
- `PUT /api/workspace/settings` persists workspace display name, description, operator, and tags to `workspace.yaml`.
- `/api/gui/bootstrap` exposes workspace metadata from the same backend projection.
- The settings workbench renders Chinese editable workspace controls and saves through the backend API.
- The desktop GUI smoke verifies workspace settings persistence in `workspace.yaml`, bootstrap JSON, and the rendered DOM.

### Task 12: Desktop Lifecycle Hardening

Files:
- `gui/src/main/backend.js`
- `gui/src/main/main.js`
- `gui/src/main/preload.js`
- `gui/src/main/desktop-state.js`
- `gui/src/main/workspace.js`
- `gui/src/app/api.ts`
- `gui/src/app/types.ts`
- `gui/src/app/OuroborosIDE.tsx`
- `gui/src/app/zh-cn.ts`
- `gui/src/styles/ouroboros-ide.css`
- `gui/scripts/check-desktop-contract.mjs`
- `gui/scripts/check-gui-contract.mjs`
- `scripts/smoke_desktop_package.sh`

Acceptance:
- Electron builds a Chinese application menu for workspace open/create, recent workspaces, view modes, refresh, execution access, and backend restart.
- Recent workspaces are persisted in Electron user data and drive the menu from real workspace paths.
- Window bounds and maximized state are persisted in `window-state.json`.
- The backend sidecar manager emits lifecycle status, records `lastError`, marks unexpected exits as `crashed`, and supports `restart()`.
- Preload exposes backend restart, recent workspace, open recent workspace, window state, menu action, and backend status bridge methods.
- React consumes the desktop lifecycle bridge through `loadDesktopStatus` and `restartDesktopBackend` and renders Chinese starting/crashed/offline recovery UI.
- The desktop package smoke verifies that lifecycle bridge files, IPC strings, crash handling, and Chinese recovery copy are present in the packaged app bundle.

### Task 13: Manifest-Backed Tool Launch

Status: **Complete**

Files:
- `internal/api/dto.go`
- `internal/api/toolchain.go`
- `internal/api/execution.go`
- `internal/api/projector.go`
- `internal/api/server.go`
- `internal/api/server_test.go`
- `gui/src/app/api.ts`
- `gui/src/app/types.ts`
- `gui/src/app/OuroborosIDE.tsx`
- `gui/src/app/zh-cn.ts`
- `gui/scripts/check-gui-contract.mjs`
- `scripts/smoke_desktop_gui.sh`

Acceptance:
- `/api/gui/bootstrap` exposes executable actions derived from real `toolchain/*/manifest.yaml` fields rather than frontend-authored tool rows.
- The GUI renders Chinese tool-launch controls from backend DTOs with editable arguments.
- Launching a tool uses the existing execution API path so metadata, logs, artifacts, retry linkage, eventlog replay, and report/graph references stay real.
- Empty or non-executable toolchain manifests produce an empty state, not fabricated tool actions.
- Contract and smoke coverage verify a manifest-backed tool launch against a temporary real workspace.

Completion notes:
- Backend action projection only emits actions for manifests with `run_command`.
- `/api/toolchain/actions/{id}/run` starts normal executions and records tool action metadata in eventlog payloads.
- React consumes `toolActions` from backend DTOs, renders Chinese launch controls, and refreshes from the returned execution DTO.
- `scripts/smoke_desktop_gui.sh` creates a real manifest command, launches it through the backend action endpoint, verifies logs/artifacts/bootstrap/DOM, and covers the evidence-to-replay navigation path.
- The replay smoke race after opening an evidence file was fixed by switching mission mode before the async file read and by waiting for the real file editor panel before clicking replay.

### Task 14: Real Command Palette Actions

Status: **Complete for the first action slice**

Files:
- `gui/src/app/OuroborosIDE.tsx`
- `gui/src/app/zh-cn.ts`
- `gui/src/styles/ouroboros-ide.css`
- `gui/scripts/check-gui-contract.mjs`
- `scripts/smoke_desktop_gui.sh`

Acceptance:
- The command palette input is searchable and editable, not a read-only visual placeholder.
- `Ctrl/Cmd+K` opens the palette and `Escape` closes it.
- Palette actions call existing real callbacks for refresh, workspace open/create, backend restart, current command run, mission save/start, and workbench switching.
- Palette copy is Chinese and describes real software actions.
- GUI contract rejects regressions to read-only/static command palette behavior.
- Desktop GUI smoke opens the palette, verifies real command labels, executes refresh, and continues through the real workspace graph/report/replay workflow.

Completion notes:
- This task did not add new backend endpoints; it productized existing desktop bridge and backend API actions through the IDE command surface.
- Workspace open/create actions are disabled only when the Electron desktop bridge is unavailable, such as Vite preview smoke. They still render as desktop actions rather than fake browser behavior.

### Task 15: Execution Log And Artifact File Jumps

Status: **Complete**

Files:
- `internal/api/dto.go`
- `internal/api/execution.go`
- `internal/api/server_test.go`
- `gui/src/app/api.ts`
- `gui/src/app/types.ts`
- `gui/src/app/OuroborosIDE.tsx`
- `gui/src/app/zh-cn.ts`
- `gui/scripts/check-gui-contract.mjs`
- `scripts/smoke_desktop_gui.sh`

Acceptance:
- Execution artifact DTOs include a backend-authored workspace-relative `filePath`.
- The frontend consumes artifact `filePath` from the backend DTO rather than reconstructing it from execution metadata.
- The execution panel can open log files and artifact files through the existing workspace file reader/editor.
- Visible execution file-jump actions are Chinese.
- GUI contract rejects regressions where log/artifact file opening or artifact `filePath` mapping disappears.
- Desktop GUI smoke verifies the full path: load logs/artifacts, click `打开产物文件`, and show the real artifact file in the editor.

Completion notes:
- This slice deepens execution/file/artifact navigation without adding new product gates.
- A runtime `ReferenceError` caused by a missing React prop was caught by Chromium smoke/CDP diagnostics and fixed before final verification.

### Task 16: Execution Detail Graph/Replay/Report Jumps

Status: **Complete**

Files:
- `gui/src/app/OuroborosIDE.tsx`
- `gui/src/app/zh-cn.ts`
- `gui/src/styles/ouroboros-ide.css`
- `gui/scripts/check-gui-contract.mjs`
- `scripts/smoke_desktop_gui.sh`

Acceptance:
- Execution details derive related graph, replay, and report targets from backend DTO reference fields rather than frontend-authored navigation rows.
- The report workbench can be opened on a specific backend-projected finding from another workbench.
- Execution detail actions are visible in Chinese and are shown only when a real backend DTO target exists.
- GUI contract rejects regressions where execution target derivation, report finding focus, or Chinese navigation copy disappears.
- Desktop GUI smoke proves execution-to-report, execution-to-replay, and execution-to-graph navigation against a temporary real workspace.

Completion notes:
- The shell now owns focused report finding state so cross-workbench actions can select a concrete report finding.
- `ExecutionReferenceTargets` matches `GraphNode.executionId`, `ReplayEvent.executionId`, and `ReportFinding.executionId` against the selected execution.
- This slice does not add product gates or scope checks; it only connects existing real DTO references into IDE navigation.

### Task 17: Report Finding Detail Execution File Jumps

Status: **Complete**

Files:
- `gui/src/app/OuroborosIDE.tsx`
- `gui/src/app/zh-cn.ts`
- `gui/src/styles/ouroboros-ide.css`
- `gui/scripts/check-gui-contract.mjs`
- `scripts/smoke_desktop_gui.sh`

Acceptance:
- Report finding details match `ReportFinding.executionId` to the backend-projected execution list instead of fabricating execution rows.
- The report detail surface can load the linked execution log and artifact listing through existing backend APIs.
- Log opening uses the backend-projected execution `logPath`; artifact opening uses backend-authored artifact `filePath`.
- Visible report detail actions and headings are Chinese.
- GUI contract rejects regressions where report detail execution-file references disappear.
- Desktop GUI smoke proves report finding detail log/artifact loading and artifact opening against a temporary real workspace.

Completion notes:
- The renderer tracks which execution owns the currently loaded artifact list so report details do not attach stale artifacts to another finding.
- This slice reuses the existing workspace file reader/editor and execution APIs; no frontend path reconstruction or product gate was added.

### Task 18: Replay Detail Execution File And Report Jumps

Status: **Complete**

Files:
- `gui/src/app/OuroborosIDE.tsx`
- `gui/scripts/check-gui-contract.mjs`
- `scripts/smoke_desktop_gui.sh`
- `.planning/2026-07-03-gui-rebuild/task_plan.md`
- `.planning/2026-07-03-gui-rebuild/findings.md`
- `.planning/2026-07-03-gui-rebuild/progress.md`

Acceptance:
- Replay event details match `ReplayEvent.executionId` to backend-projected executions instead of fabricating execution context.
- Replay event details match report findings through backend DTO references where `ReportFinding.replayEventId` equals the selected event id.
- The replay detail surface can load linked execution logs/artifacts and open backend-authored log/artifact file paths.
- Replay detail can jump to the related report finding when the backend DTO reference exists.
- GUI contract rejects regressions where replay-side execution/finding matching or file-opening actions disappear.
- Desktop GUI smoke proves replay detail log/artifact loading, artifact opening, and report focus against a temporary real workspace.

Completion notes:
- This mirrors the report-detail execution file linkage on the replay surface and reuses the same execution APIs and workspace file editor.
- Artifact rows are shown only for the execution that owns the currently loaded artifact list; no artifact path is reconstructed in React.

### Task 19: Graph Detail Execution File And Report Jumps

Status: **Complete**

Files:
- `gui/src/app/OuroborosIDE.tsx`
- `gui/scripts/check-gui-contract.mjs`
- `scripts/smoke_desktop_gui.sh`
- `.planning/2026-07-03-gui-rebuild/task_plan.md`
- `.planning/2026-07-03-gui-rebuild/findings.md`
- `.planning/2026-07-03-gui-rebuild/progress.md`

Acceptance:
- Graph node details match `GraphNode.executionId` to backend-projected executions instead of fabricating execution context.
- Graph node details match report findings through backend DTO references, preferring `ReportFinding.nodeId` and falling back to explicit execution/replay references when the selected graph node is an execution-derived node.
- The graph detail surface can load linked execution logs/artifacts and open backend-authored log/artifact file paths.
- Graph detail can jump to the related report finding when the backend DTO reference exists.
- GUI contract rejects regressions where graph-side execution/finding matching or file-opening actions disappear.
- Desktop GUI smoke proves graph detail log/artifact loading, artifact opening, and report focus against a temporary real workspace.

Completion notes:
- This makes graph nodes a first-class IDE navigation surface rather than only a visual map.
- Artifact rows are shown only for the execution that owns the currently loaded artifact list; React still does not reconstruct artifact paths.

### Task 20: Real Graph Export Action

Status: **Complete**

Files:
- `internal/api/dto.go`
- `internal/api/graph_export.go`
- `internal/api/server.go`
- `internal/api/server_test.go`
- `gui/src/app/api.ts`
- `gui/src/app/types.ts`
- `gui/src/app/OuroborosIDE.tsx`
- `gui/src/app/zh-cn.ts`
- `gui/src/styles/ouroboros-ide.css`
- `gui/scripts/check-gui-contract.mjs`
- `scripts/smoke_desktop_gui.sh`

Acceptance:
- `POST /api/evidence/graph/export` projects the current real evidence graph and writes it to a workspace file.
- The exported file is `exports/attack-graph.json` and contains graph nodes/edges from backend DTO projection.
- The API response returns a backend-authored workspace-relative `filePath`, node count, edge count, and modified time.
- The graph toolbar `导出图谱` button calls the backend export API and opens the returned file in the existing workspace file editor.
- The toolbar no longer has a static export affordance with no behavior.
- GUI contract rejects regressions where the frontend stops calling the backend export API.
- Desktop GUI smoke proves export by clicking `导出图谱`, opening `exports/attack-graph.json`, and verifying editor content from the real graph.

Completion notes:
- React does not construct graph export files itself; it calls the backend and opens the returned file path.
- The default graph view button now resets selection to the first backend-projected node instead of being a no-op.

### Task 21: Real Report Export Action

Status: **Complete**

Files:
- `internal/api/dto.go`
- `internal/api/report_export.go`
- `internal/api/server.go`
- `internal/api/server_test.go`
- `gui/src/app/api.ts`
- `gui/src/app/types.ts`
- `gui/src/app/OuroborosIDE.tsx`
- `gui/src/app/zh-cn.ts`
- `gui/src/styles/ouroboros-ide.css`
- `gui/scripts/check-gui-contract.mjs`
- `scripts/smoke_desktop_gui.sh`

Acceptance:
- `POST /api/reports/export` projects current workspace metadata, existing report files, and backend report findings into Markdown.
- The exported file is `reports/ouroboros-report.md` and includes real finding titles, severities, targets, evidence files, graph node ids, execution ids, and replay event ids when available.
- The API response returns a backend-authored workspace-relative `filePath`, finding count, report count, and modified time.
- The report page `导出报告` button calls the backend export API and opens the returned file in the existing workspace file editor.
- GUI contract rejects regressions where the frontend stops calling the backend report export API.
- Desktop GUI smoke proves export by clicking `导出报告`, opening `reports/ouroboros-report.md`, and verifying editor content from real findings.

Completion notes:
- React does not synthesize report Markdown; it calls the backend and opens the returned file path.
- The exported report is intentionally a normal workspace file so users can edit, search, and version it like other workspace artifacts.

### Task 22: Real Report TOC Sections

Status: **Complete**

Files:
- `gui/src/app/types.ts`
- `gui/src/app/api.ts`
- `gui/src/app/OuroborosIDE.tsx`
- `gui/src/app/zh-cn.ts`
- `gui/src/styles/ouroboros-ide.css`
- `gui/scripts/check-gui-contract.mjs`
- `scripts/smoke_desktop_gui.sh`

Acceptance:
- `/api/gui/bootstrap` report files are preserved as frontend `IDEData.reports` instead of being dropped by the API mapper.
- The report TOC uses `reportActiveSection`, `reportSections`, and `onSelectReportSection` to switch content instead of rendering a fixed active button.
- `ReportSectionContent` renders real sections from workspace DTOs, report findings, executions, and backend-projected report files.
- The report page exposes Chinese sections for `风险分布`, `项目资料`, `已有报告文件`, and `关联证据与执行`.
- The findings section keeps report finding detail, graph/replay/file/execution jumps, and linked execution file actions backed by existing backend DTOs.
- GUI contract rejects regressions where the report TOC becomes static again.
- Desktop GUI smoke clicks report TOC sections and verifies section-specific content through `data-report-section`.

Completion notes:
- React only derives counts and grouping from existing DTO arrays; it does not invent findings, executions, or report files.
- The appendix uses backend-projected report paths and finding evidence/execution references so exported report files appear as normal workspace artifacts.

### Task 23: Real Settings Navigation Sections

Status: **Complete**

Files:
- `gui/src/app/OuroborosIDE.tsx`
- `gui/src/app/zh-cn.ts`
- `gui/scripts/check-gui-contract.mjs`
- `scripts/smoke_desktop_gui.sh`

Acceptance:
- The settings navigation uses `settingsActiveSection`, `settingsSections`, and `onSelectSettingsSection` to switch content instead of rendering a fixed first active button.
- `SettingsSectionContent` renders the current status, workspace config, provider config, and toolchain config sections with `data-settings-section`.
- Existing workspace/provider/toolchain forms remain backed by the same backend DTOs and save APIs.
- The current-status section renders backend/bootstrap status fields from `IDEData.status`.
- GUI contract rejects regressions where settings navigation becomes static again.
- Desktop GUI smoke clicks every settings section and verifies persisted workspace/provider/toolchain values through `data-settings-section`.

Completion notes:
- This is frontend interaction state only; it does not move configuration facts into React.
- The smoke reader includes form control values because persisted settings appear in input/select values rather than plain text.

### Task 24: Real Title-Bar Window Controls

Status: **Complete**

Files:
- `gui/src/main/main.js`
- `gui/src/main/preload.js`
- `gui/src/app/api.ts`
- `gui/src/app/OuroborosIDE.tsx`
- `gui/src/app/zh-cn.ts`
- `gui/src/styles/ouroboros-ide.css`
- `gui/scripts/check-desktop-contract.mjs`
- `scripts/smoke_desktop_package.sh`

Acceptance:
- Electron main process exposes `window:minimize`, `window:toggle-maximize`, and `window:close` IPC handlers for the current BrowserWindow.
- Preload exposes `minimizeWindow`, `toggleMaximizeWindow`, and `closeWindow` methods through the existing `window.ouroboros` bridge.
- React title-bar window controls are buttons with Chinese accessible labels, not static decorative spans.
- Browser/dev rendering disables these buttons when the Electron bridge is unavailable.
- Desktop contract rejects regressions where the bridge or renderer callbacks disappear.
- Desktop package smoke verifies packaged main/preload IPC strings and Chinese renderer copy.

Completion notes:
- This is desktop shell behavior only; it does not affect workspace/backend business state.
- The visual form stays compact and consistent with the existing title bar while becoming real software control.

### Task 25: Recent Workspace Command Actions

Status: **Complete**

Files:
- `gui/src/app/api.ts`
- `gui/src/app/OuroborosIDE.tsx`
- `gui/src/app/zh-cn.ts`
- `gui/scripts/check-gui-contract.mjs`
- `scripts/smoke_desktop_gui.sh`

Acceptance:
- Frontend API exposes `openRecentDesktopWorkspace` backed by `window.ouroboros.openRecentWorkspace`.
- Command palette receives real `DesktopStatus.recentWorkspaces` from Electron desktop state.
- Each recent workspace path becomes an executable `打开最近工作区` command palette action.
- Empty recent lists produce no fake actions.
- GUI contract rejects regressions where recent workspace actions disappear from the command palette.
- Desktop GUI smoke injects the Electron bridge shape, exposes the temporary workspace as a recent workspace, executes `打开最近工作区`, and continues against the same real backend API.

Completion notes:
- This does not create synthetic workspace history; it only surfaces paths already provided by Electron desktop state.
- The native menu remains available, but recent workspaces are now also reachable inside the IDE's command surface.
- Final verification after this task covered desktop contracts, GUI contracts, product contracts, no-mock literal scan, focused execution API test, full `internal/api` package, renderer build, real GUI smoke, desktop directory packaging, and packaged backend smoke.

### Task 26: Settings Empty-State Provider Cleanup

Status: **Complete**

Files:
- `gui/src/app/OuroborosIDE.tsx`
- `gui/src/app/api.ts`
- `gui/src/app/zh-cn.ts`
- `gui/scripts/check-gui-contract.mjs`

Acceptance:
- Renderer empty IDE data does not fabricate provider type, enabled state, timeout, or routing defaults.
- API fallback empty provider state stays unconfigured until real backend DTO data is available.
- Provider type, toolchain runner, and healthcheck settings controls keep their real persisted values while showing Chinese option labels.
- GUI contract rejects regressions where empty provider state carries hardcoded business defaults or settings controls expose internal values as visible copy.
- Real settings persistence still passes through the desktop GUI smoke, and the regenerated desktop package still passes package smoke.

Completion notes:
- The saved config values remain unchanged for compatibility with the Go backend and manifest files.
- This cleanup reduces frontend-authored business state without removing any operator capability.

### Task 27: Backend-Only Replay And Report Projection

Status: **Complete**

Files:
- `gui/src/app/api.ts`
- `gui/scripts/check-gui-contract.mjs`

Acceptance:
- Bootstrap replay rows are mapped only from backend `replayEvents` DTOs.
- Bootstrap report finding rows are mapped only from backend `reportFindings` DTOs.
- Legacy endpoint fallback does not derive replay rows from `/api/runs`.
- Legacy endpoint fallback does not derive report findings from graph nodes or report files.
- Existing report files still appear as report documents through backend `reports` DTOs.
- GUI contract rejects regressions where React recreates replay/report business rows from adjacent data.
- Real desktop GUI smoke still verifies replay/report views through backend bootstrap projections.

Completion notes:
- This intentionally leaves legacy fallback with empty replay/report arrays when bootstrap projection is unavailable.
- The change makes missing backend projection visible as an empty state instead of hiding it with frontend inference.

### Task 28: Mission Operation Message Cleanup

Status: **Complete**

Files:
- `gui/src/app/OuroborosIDE.tsx`
- `gui/src/app/api.ts`
- `gui/src/app/zh-cn.ts`
- `gui/scripts/check-gui-contract.mjs`

Acceptance:
- Frontend bootstrap mapping does not synthesize mission conversation rows.
- Legacy endpoint fallback does not synthesize mission conversation rows.
- Empty IDE data does not include fake loading/system messages in `messages`.
- The mission panel is labeled as `任务操作`, not `AI 对话`, until real backend-projected chat/message data exists.
- When there are no backend-projected task messages, the panel shows a Chinese empty state instead of generated system/agent rows.
- GUI contract rejects regressions that reintroduce `mapMessages`, frontend `agent` rows, or stale AI-chat copy.

Completion notes:
- The real mission launch form, manifest-backed tool launcher, and command composer remain available in the same panel.
- This removes fake conversation content without blocking future backend-projected task or chat messages.

### Task 29: Workspace Overview And Workflow Status Cleanup

Status: **Complete**

Files:
- `gui/src/app/OuroborosIDE.tsx`
- `gui/src/app/api.ts`
- `gui/src/app/zh-cn.ts`
- `gui/src/styles/ouroboros-ide.css`
- `gui/scripts/check-gui-contract.mjs`

Acceptance:
- The right rail does not present file/execution/graph counts as an agent graph.
- The count summary is labeled as `工作区概览` with Chinese metadata describing real counts.
- Frontend workflow status does not infer `下一步` from graph node counts.
- Bootstrap workflow status comes from backend `workflow.nextAction` or `workflow.status` when present.
- Missing workflow projection renders explicit unknown/offline state instead of frontend-invented next-step state.
- GUI contract rejects regressions that reintroduce stale `智能体图`, `下一步`, `agent-flow`, or graph-count workflow fallback.

Completion notes:
- Real counts remain visible because they come from backend DTO/workspace data already loaded by the IDE.
- This changes labels and fallback behavior only; it does not remove execution, graph, report, replay, or settings capabilities.

### Task 30: Toolchain Settings Unconfigured Defaults

Status: **Complete**

Files:
- `gui/src/app/OuroborosIDE.tsx`
- `gui/src/app/api.ts`
- `gui/scripts/check-gui-contract.mjs`

Acceptance:
- Empty toolchain settings draft does not fabricate `native` runner or `built_in` healthcheck values.
- Toolchain DTO mapping preserves missing backend manifest runner/healthcheck fields as unconfigured.
- Toolchain runner and healthcheck selects include Chinese `未配置` options.
- Existing real saved manifest values still display and save through the existing settings form.
- GUI contract rejects regressions that reintroduce frontend-authored runner/healthcheck defaults.

Completion notes:
- This does not change manifest persistence semantics for workspaces that already specify runner or healthcheck values.
- Missing manifest fields now remain visible as missing instead of being hidden by frontend defaults.

### Task 31: WebSocket Event Stream Write Serialization

Status: **Complete**

Files:
- `internal/api/server.go`

Acceptance:
- WebSocket welcome messages, ping heartbeats, periodic heartbeat events, and execution/event broadcasts share one serialized server-side write path.
- Execution started/completed broadcasts do not panic with `concurrent write to websocket connection`.
- Focused WebSocket execution event test passes repeatedly.
- Full `internal/api` package, desktop GUI smoke, and packaged desktop smoke pass after the fix.

Completion notes:
- The fix preserves the existing event stream API and payloads.
- This hardens real desktop refresh behavior rather than adding a UI gate or restriction.

### Task 32: Frontend Object Identity And Config-Path Fallback Cleanup

Status: **Complete**

Files:
- `gui/src/app/api.ts`
- `gui/src/app/OuroborosIDE.tsx`
- `gui/src/app/zh-cn.ts`
- `gui/scripts/check-gui-contract.mjs`

Acceptance:
- Frontend bootstrap mapping does not generate replay IDs such as `event-1` when backend replay DTOs omit stable identifiers.
- Frontend report mapping does not generate finding/report IDs such as `finding-1` or `report-1`.
- Frontend graph mapping drops nodes without backend IDs instead of inventing `node-1` identities.
- Workspace file mapping drops incomplete file DTOs instead of rendering generic `file` / `未命名文件` rows.
- Workspace settings do not derive `workspace.yaml` paths in the renderer when backend DTOs omit `configPath`; the settings UI shows Chinese `未配置`.
- Mission empty state does not fabricate `mission.yaml` as a path before backend projection arrives.
- Execution action APIs reject missing execution DTOs with Chinese error copy instead of returning an empty `execution` row.
- GUI contract rejects regressions that reintroduce synthetic object identities, config-path derivation, or `payload.execution ?? {}` mapping.

Completion notes:
- This keeps backend-authored IDs such as eventlog-derived event IDs valid; the cleanup only removes renderer-authored fallbacks.
- Existing real graph/replay/report/file/execution workflows continue to pass GUI smoke and packaged smoke.

### Task 33: Bootstrap-Only Aggregate IDE Data Loading

Status: **Complete**

Files:
- `gui/src/app/api.ts`
- `gui/src/app/zh-cn.ts`
- `gui/scripts/check-gui-contract.mjs`

Acceptance:
- `loadIDEData` maps aggregate IDE state only from `/api/gui/bootstrap`.
- Renderer no longer calls legacy fallback endpoints `/api/health`, `/api/workspace/info`, `/api/evidence/graph`, `/api/runs`, or `/api/toolchain/health` to assemble GUI state.
- Renderer no longer keeps `loadIDEDataFromLegacyEndpoints`, `LoadedResponses`, or `mapIDEData(responses)` paths.
- When bootstrap is unavailable, the frontend renders a Chinese unavailable state (`后端未返回启动数据`) with empty real-data lists instead of composing partial workspace/graph/toolchain/run facts in React.
- GUI contract rejects regressions that reintroduce legacy endpoint aggregate-state assembly.

Completion notes:
- Direct action APIs remain unchanged for files, execution, mission, provider, toolchain, graph export, report export, and desktop lifecycle calls.
- This makes the backend GUI bootstrap DTO the single source of aggregate software state while preserving explicit offline/degraded UI behavior.

### Task 34: Dormant Mission Message UI Removal

Status: **Complete**

Files:
- `gui/src/app/OuroborosIDE.tsx`
- `gui/src/app/api.ts`
- `gui/src/app/types.ts`
- `gui/src/styles/ouroboros-ide.css`
- `gui/scripts/check-gui-contract.mjs`

Acceptance:
- The mission panel no longer keeps `MissionMessage`, `MessageBubble`, or `data.messages` rendering paths without a backend message DTO.
- API mapping no longer injects `messages: []` into `IDEData`.
- Stale `.message.agent`, `.message.system`, and `.message-label` styles are removed.
- The panel still shows the Chinese empty task-message state `暂无后端投影的任务消息`.
- GUI contract rejects regressions that reintroduce dormant mission message rows, stale agent/system message styling, or frontend message synthesis.

Completion notes:
- Real mission launch, tool launch, raw command execution, file editor, execution panel, and cross-navigation remain unchanged.
- A future task-message surface should be added only when the backend exposes an explicit message DTO.

### Task 35: Bot And Agent UI Semantics Cleanup

Status: **Complete**

Files:
- `gui/src/app/OuroborosIDE.tsx`
- `gui/src/app/zh-cn.ts`
- `gui/scripts/check-gui-contract.mjs`

Acceptance:
- Mission operation surface no longer imports or uses the `Bot` icon.
- Shared icon typing uses `LucideIcon` instead of anchoring component contracts to `typeof Bot`.
- The mission operation panel uses a neutral command icon.
- Replay event type `agent` remains compatible with backend event values but renders as Chinese `执行单元` instead of visible `智能体`.
- GUI contract rejects regressions that reintroduce Bot visual semantics or stale `agent: '智能体'` copy.

Completion notes:
- This does not remove backend `agent` event compatibility; it only removes stale visible AI/agent product semantics from the active GUI.
- Real mission launch, command execution, tool launch, replay, graph, report, and settings workflows remain unchanged.

### Task 36: Settings Placeholder Chinese Cleanup

Status: **Complete**

Files:
- `gui/src/app/zh-cn.ts`
- `gui/scripts/check-gui-contract.mjs`

Acceptance:
- Settings page labels no longer expose `Base URL` as visible English UI copy.
- Provider, toolchain, and workspace settings placeholders no longer contain concrete example values such as provider names, model names, sample service URLs, capability lists, shell command samples, or tag samples.
- Placeholder text is Chinese instructional copy that explains what to enter without pretending those values are real workspace/provider/toolchain state.
- Real saved provider/toolchain/workspace values still flow from backend DTOs and editable drafts; this task does not add frontend defaults.
- GUI contract rejects regressions that reintroduce the removed English/example settings copy.
- Full desktop contract, GUI contract, product contract, mock scan, API test, renderer build, GUI smoke, desktop packaging, and package smoke pass.

Completion notes:
- This is a visible UI cleanup only; provider/toolchain/workspace persistence APIs and DTO mapping remain unchanged.
- Technical option values such as provider type or runner IDs remain saved as their real config values while visible labels stay Chinese.

### Task 37: Replay And Report ID Fallback Cleanup

Status: **Complete**

Files:
- `gui/src/app/api.ts`
- `gui/scripts/check-gui-contract.mjs`

Acceptance:
- Replay event mapping requires `event.id` from the backend DTO and no longer uses `runId` as a renderer-authored identity fallback.
- Report finding mapping requires `finding.id` from the backend DTO and no longer uses `path` as a renderer-authored identity fallback.
- Report document mapping requires `report.id` from the backend DTO and no longer uses `path` as a renderer-authored identity fallback.
- Rows missing backend-authored IDs are dropped instead of being rendered under an inferred identity.
- GUI contract rejects regressions that reintroduce those fallback expressions.
- Full desktop contract, GUI contract, product contract, mock scan, API test, renderer build, GUI smoke, desktop packaging, and package smoke pass.

Completion notes:
- Backend projections already emit stable IDs for real replay events, report findings, and report documents, so existing real GUI smoke workflows remain intact.
- This removes identity synthesis only; paths still display and open as real workspace file references when the backend supplies them.

### Task 38: Toolchain Action And File ID Fallback Cleanup

Status: **Complete**

Files:
- `gui/src/app/api.ts`
- `gui/scripts/check-gui-contract.mjs`

Acceptance:
- Toolchain health pack mapping requires `pack.id` from the backend DTO and no longer uses pack name or a generic `toolchain` key.
- Editable toolchain pack mapping requires `pack.id` from the backend DTO and no longer uses pack name as an ID fallback.
- Runnable tool action mapping requires both backend `action.id` and backend `action.packId`; it no longer uses one identity field to fill the other.
- Workspace file mapping requires backend `file.id` and no longer uses file path or file name as an ID fallback.
- Rows missing required backend-authored IDs are dropped instead of being rendered under inferred identities.
- GUI contract rejects regressions that reintroduce those fallback expressions.
- Full desktop contract, GUI contract, product contract, mock scan, API test, renderer build, GUI smoke, desktop packaging, and package smoke pass.

Completion notes:
- Backend projections already emit stable IDs for real toolchain packs, manifest-backed actions, and workspace files, so existing real GUI smoke workflows remain intact.
- This keeps real labels, paths, names, and pack names visible where supplied by the backend; it only removes their use as object identities.

### Task 39: Graph Edge ID Fallback Cleanup

Status: **Complete**

Files:
- `gui/src/app/api.ts`
- `gui/scripts/check-gui-contract.mjs`

Acceptance:
- Evidence graph edge response typing includes backend `edge.id`.
- Graph edge mapping requires backend `edge.id`, `edge.from`, and `edge.to`.
- Renderer no longer generates graph edge IDs from `from`, `to`, and render index.
- Edges missing backend-authored IDs are dropped instead of being rendered under inferred identities.
- GUI contract rejects regressions that reintroduce renderer-generated edge IDs.
- Full desktop contract, GUI contract, product contract, mock scan, API test, renderer build, GUI smoke, desktop packaging, and package smoke pass.

Completion notes:
- Backend `GraphEdgeDTO` already emits stable IDs, so existing real graph/report/replay smoke workflows remain intact.
- This keeps real edge endpoints and edge type visible where supplied by the backend; it only removes renderer-authored graph edge identity.

### Task 40: Graph Edge Type Fallback Cleanup

Status: **Complete**

Files:
- `gui/src/app/api.ts`
- `gui/scripts/check-gui-contract.mjs`

Acceptance:
- Graph edge mapping requires backend `edge.type`.
- Renderer no longer assigns missing graph edge types to `related`.
- Edges missing backend-authored relation type are dropped instead of being rendered under inferred relation semantics.
- GUI contract rejects regressions that reintroduce the `related` fallback.
- Full desktop contract, GUI contract, product contract, mock scan, API test, renderer build, GUI smoke, desktop packaging, and package smoke pass.

Completion notes:
- Backend `GraphEdgeDTO` already emits relation types, so existing real graph/report/replay smoke workflows remain intact.
- This keeps real backend relation types visible while removing renderer-authored graph semantics.

### Task 41: Execution Artifact Path And Name Fallback Cleanup

Status: **Complete**

Files:
- `gui/src/app/api.ts`
- `gui/scripts/check-gui-contract.mjs`

Acceptance:
- Execution artifact mapping requires backend `artifact.path`.
- Execution artifact mapping requires backend `artifact.name`.
- Renderer no longer uses artifact name to fill missing artifact path.
- Renderer no longer uses artifact path to fill missing artifact name.
- Artifacts missing backend-authored path or name are dropped instead of being rendered under inferred file fields.
- GUI contract rejects regressions that reintroduce artifact path/name fallback expressions.
- Full desktop contract, GUI contract, product contract, mock scan, API test, renderer build, GUI smoke, desktop packaging, and package smoke pass.

Completion notes:
- Backend execution artifact DTOs already emit both path and name, so existing real artifact loading/opening smoke workflows remain intact.
- This keeps backend `filePath` based opening behavior unchanged; it only removes renderer-authored artifact row fields.

### Task 42: Tool Action Label Fallback Cleanup

Status: **Complete**

Files:
- `gui/src/app/api.ts`
- `gui/scripts/check-gui-contract.mjs`

Acceptance:
- Runnable tool action mapping requires backend `action.label`.
- Renderer no longer uses `packName` or action `id` as visible label fallback.
- Tool actions missing backend-authored labels are dropped instead of being rendered with inferred labels.
- GUI contract rejects regressions that reintroduce tool action label fallback expressions.
- Full desktop contract, GUI contract, product contract, mock scan, API test, renderer build, GUI smoke, desktop packaging, and package smoke pass.

Completion notes:
- Backend manifest action projection already emits labels, so existing manifest-backed tool launch smoke workflows remain intact.
- This keeps pack name and action ID available as backend fields where useful; it only removes renderer-authored visible action labels.

### Task 43: Tool Action Command Text Fallback Cleanup

Status: **Complete**

Files:
- `gui/src/app/api.ts`
- `gui/scripts/check-gui-contract.mjs`

Acceptance:
- Runnable tool action mapping requires backend `action.commandText`.
- Renderer no longer joins backend `action.command` arrays into visible command text.
- Tool actions missing backend-authored `commandText` are dropped instead of being rendered with inferred display text.
- GUI contract rejects regressions that reintroduce the command-text fallback expression.
- Full desktop contract, GUI contract, product contract, mock scan, API test, renderer build, GUI smoke, desktop packaging, package smoke, fixed-string fallback scan, and diff whitespace check pass.

Completion notes:
- Backend manifest action projection already emits display-ready command text, so existing manifest-backed tool launch smoke workflows remain intact.
- This keeps raw command arrays available as backend data where useful; it only removes renderer-authored visible command text.

### Task 44: Replay Detail Run ID Fallback Cleanup

Status: **Complete**

Files:
- `gui/src/app/api.ts`
- `gui/scripts/check-gui-contract.mjs`

Acceptance:
- Replay event mapping uses backend `event.detail` as the only visible detail source.
- Renderer no longer displays technical `runId` values as fallback replay detail text.
- Replay events missing backend-authored detail remain valid but render with empty detail instead of inferred pseudo-description text.
- GUI contract rejects regressions that reintroduce the replay detail fallback expression.
- Full desktop contract, GUI contract, product contract, mock scan, API test, renderer build, GUI smoke, desktop packaging, package smoke, fixed-string fallback scan, and diff whitespace check pass.

Completion notes:
- Backend replay projection already emits detail for real eventlog-derived replay rows used by GUI smoke.
- This removes visible frontend-authored replay text only; `runId` remains available as a backend reference field for cross-navigation and compatibility.

### Task 45: Report Finding Endpoint Fallback Cleanup

Status: **Complete**

Files:
- `gui/src/app/api.ts`
- `gui/scripts/check-gui-contract.mjs`

Acceptance:
- Report finding mapping uses backend `finding.endpoint` as the only affected-endpoint source.
- Renderer no longer displays finding `path` or `source` values as fallback endpoint text.
- Report findings missing backend-authored endpoint remain valid but render with empty endpoint instead of inferred pseudo-endpoint text.
- GUI contract rejects regressions that reintroduce the report endpoint fallback expression.
- Full desktop contract, GUI contract, product contract, mock scan, API test, renderer build, GUI smoke, desktop packaging, package smoke, fixed-string fallback scan, and diff whitespace check pass.

Completion notes:
- Backend report-finding projection continues to provide file path/source fields for evidence opening and provenance, but those fields are no longer reused as affected-endpoint copy.
- This removes visible frontend-authored endpoint text only; real file/source references remain available where the backend supplies them.

### Task 46: Report And Finding Title Fallback Cleanup

Status: **Complete**

Files:
- `gui/src/app/api.ts`
- `gui/scripts/check-gui-contract.mjs`

Acceptance:
- Report finding mapping requires backend `finding.title`.
- Report document mapping requires backend `report.title`.
- Renderer no longer displays finding/report paths or generic report copy as fallback title text.
- Report findings and documents missing backend-authored titles are dropped instead of being rendered with inferred titles.
- GUI contract rejects regressions that reintroduce the title fallback expressions.
- Full desktop contract, GUI contract, product contract, mock scan, API test, renderer build, GUI smoke, desktop packaging, package smoke, fixed-string fallback scans, and diff whitespace check pass.

Completion notes:
- Backend report and finding projections already emit titles from Markdown headings, finding JSON fields, graph labels, or backend report-title projection, so existing real report smoke workflows remain intact.
- This removes visible frontend-authored title text only; path/source fields remain available as backend-authored provenance and file-opening references.

### Task 47: Workspace File Label Fallback Cleanup

Status: **Complete**

Files:
- `gui/src/app/api.ts`
- `gui/scripts/check-gui-contract.mjs`

Acceptance:
- Workspace file tree mapping requires backend `file.path` for visible labels.
- Renderer no longer displays `file.name` as a fallback path/label when `file.path` is missing.
- Workspace file rows missing backend-authored paths are dropped instead of being rendered under inferred labels.
- GUI contract rejects regressions that reintroduce the file label fallback expression.
- Full desktop contract, GUI contract, product contract, mock scan, API test, renderer build, GUI smoke, desktop packaging, package smoke, fixed-string fallback scan, and diff whitespace check pass.

Completion notes:
- Backend workspace file projection already emits `id`, `name`, `path`, and `kind` for real workspace files, so explorer and file-opening smoke workflows remain intact.
- This removes frontend-authored visible file labels only; backend `name` remains available as raw DTO data if a later UI explicitly needs basename display.

### Task 48: Backend-Authored Execution Labels

Status: **Complete**

Files:
- `internal/api/dto.go`
- `internal/api/execution.go`
- `internal/api/server_test.go`
- `gui/src/app/api.ts`
- `gui/src/app/zh-cn.ts`
- `gui/scripts/check-gui-contract.mjs`

Acceptance:
- `ExecutionDTO` includes backend-authored `label`.
- Execution create/read/list responses populate `label` from backend execution metadata.
- Renderer requires `execution.label` for execution rows and action responses.
- Renderer no longer splits command text or uses generic execution copy to synthesize visible execution labels.
- The unused `zhCN.execution.commandFallback` copy is removed and contract-banned.
- GUI contract rejects regressions that reintroduce frontend command-splitting labels.
- Focused execution API test, full desktop contract, GUI contract, product contract, mock scan, API test, renderer build, GUI smoke, desktop packaging, package smoke, fallback scan, and diff whitespace check pass.

Completion notes:
- The backend currently authors execution labels as the same display command used by `ExecutionDTO.command`; future backend work can upgrade label semantics without changing the renderer contract.
- This removes frontend-authored execution summary text while preserving real command visibility and execution cross-navigation.

### Task 49: Execution Command Fallback Cleanup

Status: **Complete**

Files:
- `gui/src/app/api.ts`
- `gui/scripts/check-gui-contract.mjs`

Acceptance:
- Execution row mapping requires backend `execution.command`.
- Execution action responses require backend `execution.command`.
- Renderer no longer maps missing execution commands to empty strings.
- GUI contract rejects regressions that reintroduce the execution command fallback expression.
- Full desktop contract, GUI contract, product contract, mock scan, API test, renderer build, GUI smoke, desktop packaging, package smoke, fixed-string fallback scan, and diff whitespace check pass.

Completion notes:
- Backend `ExecutionDTO.command` is already required and populated by the execution API, so existing real command execution and packaged smoke workflows remain intact.
- This removes frontend tolerance for incomplete execution DTOs; missing command text now keeps the row/action response out of the GUI rather than producing empty visible command state.

### Task 50: Execution Status Fallback Cleanup

Status: **Complete**

Files:
- `gui/src/app/api.ts`
- `gui/scripts/check-gui-contract.mjs`

Acceptance:
- Execution row mapping requires backend `execution.status`.
- Execution action responses require backend `execution.status`.
- Renderer no longer maps missing or unrecognized execution statuses to `ready`.
- GUI contract rejects regressions that reintroduce the old incomplete execution checks or optional status normalizer.
- Full desktop contract, GUI contract, product contract, mock scan, API test, renderer build, GUI smoke, desktop packaging, package smoke, and fixed-string fallback scans pass.

Completion notes:
- Backend `ExecutionDTO.status` is already required and populated by execution metadata, so existing real execution, replay/report references, GUI smoke, and packaged smoke workflows remain intact.
- `ready` remains a valid rendered execution state only when the backend explicitly returns that status; missing or unknown status no longer becomes visible ready state in React.

### Task 51: Export Result Fallback Cleanup

Status: **Complete**

Files:
- `gui/src/app/api.ts`
- `gui/scripts/check-gui-contract.mjs`

Acceptance:
- Graph export responses require backend `filePath`, `nodeCount`, `edgeCount`, and `modifiedAt`.
- Report export responses require backend `filePath`, `findingCount`, `reportCount`, and `modifiedAt`.
- Renderer no longer maps missing export paths to empty strings or missing export counts/timestamps to zero.
- Incomplete export responses enter the existing Chinese export-failed path instead of reporting success or opening an empty workspace path.
- GUI contract rejects regressions that reintroduce export result fallback expressions.
- Full desktop contract, GUI contract, product contract, mock scan, API test, renderer build, GUI smoke, desktop packaging, package smoke, fixed-string fallback scans, and diff whitespace check pass.

Completion notes:
- Backend `GraphExportDTO` and `ReportExportDTO` already return complete export file paths, counts, and modification timestamps, so the real graph/report export smoke remains intact.
- This removes frontend-authored export result facts while preserving the existing Chinese export success/failure UI.

### Task 52: Graph Node Label And Type Fallback Cleanup

Status: **Complete**

Files:
- `gui/src/app/api.ts`
- `gui/src/app/types.ts`
- `gui/src/app/zh-cn.ts`
- `gui/scripts/check-gui-contract.mjs`

Acceptance:
- Graph node mapping requires backend `id`, `label`, and `type`.
- Renderer no longer displays graph node IDs as fallback visible labels.
- Renderer no longer maps missing or unrecognized graph node types to `action`.
- Frontend graph node kind support includes backend `Artifact` nodes, rendered as Chinese `产物`.
- GUI contract rejects regressions that reintroduce id-only graph node filtering, `node.label || id`, optional node type normalization, or default action semantics.
- Full desktop contract, GUI contract, product contract, mock scan, API test, renderer build, GUI smoke, desktop packaging, package smoke, fixed-string fallback scans, and diff whitespace check pass.

Completion notes:
- Backend `ProjectEvidenceGraph` already authors `ID`, `Type`, and `Label` for eventlog and workspace-file graph nodes, including `Artifact` for evidence/artifact files.
- This removes frontend-authored graph labels and default semantics while preserving real backend artifact nodes in the graph UI.

### Task 53: Execution Artifact Metadata Fallback Cleanup

Status: **Complete**

Files:
- `gui/src/app/api.ts`
- `gui/src/app/types.ts`
- `gui/scripts/check-gui-contract.mjs`

Acceptance:
- Execution artifact rows require backend `path`, `filePath`, `name`, `size`, and `modifiedAt`.
- Renderer no longer displays missing artifact sizes or modification timestamps as zero values.
- Renderer no longer exposes artifact rows that cannot be opened through a backend-authored workspace `filePath`.
- GUI contract rejects regressions that reintroduce incomplete artifact filtering or zero metadata fallback expressions.
- Full desktop contract, GUI contract, product contract, mock scan, API test, renderer build, GUI smoke, desktop packaging, package smoke, fixed-string fallback scans, and diff whitespace check pass.

Completion notes:
- Backend `ExecutionArtifactDTO` already populates path, workspace file path, size, and modification timestamp from real files under the execution artifact directory.
- This removes frontend-authored artifact metadata while preserving the existing execution/file/report/replay/graph artifact-opening workflows.

### Task 54: Tool Action Complete DTO Cleanup

Status: **Complete**

Files:
- `gui/src/app/api.ts`
- `gui/scripts/check-gui-contract.mjs`

Acceptance:
- Runnable tool action rows require backend `id`, `packId`, `label`, `status`, non-empty `command`, and `commandText`.
- Tool action run responses require a complete backend action DTO and a complete backend execution DTO.
- Renderer no longer maps missing action-run response actions to `{}`.
- Renderer no longer maps missing tool action commands to empty arrays.
- GUI contract rejects regressions that reintroduce incomplete action filtering, missing action fallback, or command-array fallback expressions.
- Full desktop contract, GUI contract, product contract, mock scan, API test, renderer build, GUI smoke, desktop packaging, package smoke, fixed-string fallback scans, and diff whitespace check pass.

Completion notes:
- Backend `ProjectToolchainActions` already emits status, command array, and display command text from real toolchain manifests.
- This removes frontend-authored tool action records while preserving manifest-backed launch, execution logging, eventlog metadata, and packaged smoke workflows.

### Task 55: Toolchain Pack Status And Count Fallback Cleanup

Status: **Complete**

Files:
- `gui/src/app/api.ts`
- `gui/scripts/check-gui-contract.mjs`

Acceptance:
- Toolchain pack rows require backend `id`, `name`, `status`, and numeric `toolsReady`.
- Toolchain ready-count summaries only sum complete backend-authored pack DTOs.
- Renderer no longer maps missing pack IDs or names to empty strings.
- Renderer no longer maps missing pack `toolsReady` values to zero.
- GUI contract rejects regressions that reintroduce id-only pack filtering, empty pack id/name fallback, or zero ready-count fallback expressions.
- Full desktop contract, GUI contract, product contract, mock scan, API test, renderer build, GUI smoke, desktop packaging, package smoke, fixed-string fallback scans, and diff whitespace check pass.

Completion notes:
- Backend `ProjectToolchain` already emits `id`, `name`, `status`, and `toolsReady` for real manifest-backed packs, so the settings and package smoke workflows remain intact.
- Optional manifest configuration fields such as runner, parser, healthcheck, manifest path, command arrays, and capabilities remain allowed to be unconfigured; this task only removes frontend-authored visible pack state and count facts.

### Task 56: Workspace Name Fallback Cleanup

Status: **Complete**

Files:
- `gui/src/app/api.ts`
- `gui/scripts/check-gui-contract.mjs`

Acceptance:
- Connected workspace display names require backend `workspace.name`.
- Renderer no longer derives workspace display names by splitting `workspace.path`.
- Incomplete workspace DTOs fall back to the existing Chinese empty workspace state rather than fabricating names from paths.
- GUI contract rejects regressions that reintroduce the frontend `workspaceName()` path-splitting helper or workspace-name fallback expressions.
- Full desktop contract, GUI contract, product contract, mock scan, API test, renderer build, GUI smoke, desktop packaging, package smoke, fixed-string fallback scans, and diff whitespace check pass.

Completion notes:
- Backend `ProjectWorkspace` and `ProjectWorkspaceSettings` already handle old-workspace directory-name compatibility and `workspace.yaml` metadata projection, so the renderer can treat `workspace.name` as backend-authored product state.
- This removes frontend-authored workspace titles while preserving first-run empty states and packaged desktop workspace loading.

### Task 57: Report Severity And Document Completeness Cleanup

Status: **Complete**

Files:
- `gui/src/app/api.ts`
- `gui/scripts/check-gui-contract.mjs`

Acceptance:
- Report finding rows require backend-authored severity that maps to a recognized frontend `Severity`.
- Renderer no longer maps missing or unrecognized finding severity to `info`.
- Report document rows require backend-authored `status` and `path`.
- Renderer no longer translates missing report document status into a visible unknown state for a rendered row.
- Renderer no longer maps missing report document paths to empty strings.
- GUI contract rejects regressions that reintroduce finding severity fallback, report status optional translation, or report path empty-string fallback expressions.
- Full desktop contract, GUI contract, product contract, mock scan, API test, renderer build, GUI smoke, desktop packaging, package smoke, fixed-string fallback scans, and diff whitespace check pass.

Completion notes:
- Backend `ProjectReportFindings` already normalizes finding severity from graph/workspace files and uses `info` only as backend-authored projection state.
- Backend `ProjectReports` already emits ready status and workspace-relative report paths for real Markdown report files, so the renderer can require those fields before exposing a row or open-file action.

### Task 58: Report Appendix Evidence Fallback Cleanup

Status: **Complete**

Files:
- `gui/src/app/OuroborosIDE.tsx`
- `gui/scripts/check-gui-contract.mjs`

Acceptance:
- Report appendix evidence rows render only when a finding has a backend-authored `filePath` or `path`.
- Renderer no longer displays finding `source` or finding IDs as evidence file text.
- Renderer no longer displays a generic empty placeholder as a relation/reference value in appendix rows.
- Report document open actions use already-required backend report paths directly.
- GUI contract rejects regressions that reintroduce appendix evidence fallback expressions.
- Full desktop contract, GUI contract, product contract, mock scan, API test, renderer build, GUI smoke, desktop packaging, package smoke, fixed-string fallback scans, and diff whitespace check pass.

Completion notes:
- Backend report finding DTOs already carry workspace-relative evidence paths when findings originate from files or graph nodes with file references.
- This keeps appendix evidence links tied to real openable workspace files and avoids presenting source categories or technical IDs as evidence artifacts.

### Task 59: Mission Path And Status Completeness Cleanup

Status: **Complete**

Files:
- `gui/src/app/api.ts`
- `gui/src/app/zh-cn.ts`
- `gui/scripts/check-gui-contract.mjs`

Acceptance:
- Mission save responses require backend-authored `MissionDTO.path` and `MissionDTO.status`.
- Mission start responses require backend-authored `MissionDTO.path` and `MissionDTO.status` before returning a task state to the renderer.
- Incomplete mission bootstrap DTOs fall back to the existing Chinese empty mission state instead of fabricating a mission file path or status.
- Editable draft fields (`name`, `goal`, `command`) remain empty when the backend returns empty strings, so the operator can still create a real task from the GUI.
- GUI contract rejects regressions that reintroduce mission path empty-string fallback or optional mission status translation.
- Full desktop contract, GUI contract, product contract, mock scan, API test, renderer build, GUI smoke, desktop packaging, and package smoke pass.

Completion notes:
- Backend `ProjectMission` already emits `path: mission.yaml` plus a concrete status for missing, empty, and ready task files.
- This keeps the direct mission workflow editable while making visible mission file/status state a backend DTO responsibility.

### Task 60: Replay Event Type And Timestamp Completeness Cleanup

Status: **Complete**

Files:
- `gui/src/app/api.ts`
- `gui/scripts/check-gui-contract.mjs`

Acceptance:
- Replay timeline rows require backend-authored `ReplayEventDTO.id`, `type`, `lane`, and numeric `createdAt`.
- Renderer no longer renders replay rows from ID-only DTOs.
- Renderer no longer maps missing or unknown replay event types to a generic visible event label.
- Renderer no longer formats missing replay timestamps as `--:--:--`.
- Unknown replay event types are dropped until the backend/frontend contract explicitly supports them.
- GUI contract rejects regressions that reintroduce ID-only filtering, optional replay type translation, generic event-label fallback, optional timestamp formatting, or placeholder replay time.
- Full desktop contract, GUI contract, product contract, mock scan, API test, renderer build, GUI smoke, desktop packaging, and package smoke pass.

Completion notes:
- Backend `ProjectReplayEvents` already emits `Type`, `Lane`, and `CreatedAt` from real eventlog rows, so the renderer can require these fields before displaying a replay timeline row.
- This keeps replay timeline facts tied to eventlog projection instead of frontend-authored placeholders.

### Task 61: Workspace File Kind Completeness Cleanup

Status: **Complete**

Files:
- `gui/src/app/api.ts`
- `gui/scripts/check-gui-contract.mjs`

Acceptance:
- Workspace file tree rows require backend-authored `WorkspaceFileDTO.kind`.
- Renderer no longer maps a missing file kind to visible generic `文件` copy.
- Renderer no longer maps unknown file kind values to visible generic `文件` copy.
- Backend-authored `kind: "file"` remains the explicit generic file kind and maps to Chinese `文件`.
- GUI contract rejects regressions that reintroduce missing-kind fallback or unknown-kind default mapping.
- Full desktop contract, GUI contract, product contract, mock scan, API test, renderer build, GUI smoke, desktop packaging, and package smoke pass.

Completion notes:
- Backend `ProjectWorkspaceFiles` always emits `Kind` through `workspaceFileKind`, including `directory`, extension-specific kinds, and explicit `file`.
- This keeps file-tree metadata tied to backend workspace projection instead of frontend-authored defaults.

## Verification

Run after each P0 task:

```bash
npm --prefix gui run test:product-contract
npm --prefix gui run test:contract
npm --prefix gui run build
GOPROXY=https://goproxy.cn,direct go test ./internal/api ./cmd/ouro -count=1
git diff --check -- PRODUCT.md DESIGN.md docs/superpowers/specs docs/superpowers/plans .planning gui internal/api cmd/ouro
```

Known verification scope note: `go test ./cmd/ouro` has unrelated dirty-tree CLI drift in this workspace. For the desktop GUI/API slice, use fresh `go test ./internal/api -count=1` plus GUI contracts/build/smokes as the required signal.
