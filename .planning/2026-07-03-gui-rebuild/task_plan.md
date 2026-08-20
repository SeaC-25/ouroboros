# Task Plan: GUI Rebuild

## Goal
Rebuild Ouroboros as a GUI-first penetration testing IDE using `ui-designs/` as the visual source of truth while preserving the Go backend, workspace model, replay, evidence, tool execution, provider, worker, and reporting capabilities. The desktop app exposes workspace and execution capabilities as direct software actions.

## Current Phase
Phase 8

## Phases

### Phase 1: Requirements & Discovery
- [x] Confirm user intent: discard the old GUI/Warp direction.
- [x] Inspect current `gui/`, `internal/api/`, README, Makefile, and design images.
- [x] Record design-image findings in `findings.md`.
- **Status:** complete

### Phase 2: Product & Design Contracts
- [x] Create `PRODUCT.md` to fix the GUI-first product position.
- [x] Create `DESIGN.md` to define the visual and interaction system.
- [x] Create a formal GUI rebuild design spec under `docs/superpowers/specs/`.
- **Status:** complete

### Phase 3: GUI Shell Replacement
- [x] Preserve `gui/package.json`, build config, and Electron/Vite stack.
- [x] Remove or quarantine the old renderer implementation and backup UI.
- [x] Rebuild the app shell around the main design: title bar, activity bar, workspace explorer, mission workbench, right panels, and status bar.
- **Status:** complete

### Phase 4: Read-Only Data Contract
- [x] Define first-pass frontend DTOs for workspace, mission, graph, execution, replay, report, and status state.
- [x] Add fixture-backed UI state first, with clean seams for Go API replacement.
- [x] Replace fixture data with a frontend API client that reads the current Go API endpoints.
- [x] Replace mock-shaped endpoints with explicit read-only API contracts.
- [x] Add `/api/gui/bootstrap` for desktop GUI aggregation.
- [x] Add `/api/workspace/files` so the GUI can display real workspace files.
- **Status:** complete

### Phase 5: Vertical Product Slice
- [x] Implement desktop workspace open action and backend startup status in the active shell.
- [x] Implement file open/edit on top of the real workspace file list.
- [x] Implement workspace search.
- [x] Implement real execution actions for run, stop, retry, logs, artifacts, exit code, and execution history.
- [x] Wire the mission execution panel to real backend execution APIs.
- [x] Add a desktop GUI smoke that verifies Chinese UI against real workspace execution data.
- [x] Add backend execution event stream and frontend event-stream refresh.
- [x] Add a runnable desktop package directory with Electron runtime, renderer, main process, preload, and backend sidecar.
- [x] Implement empty/onboarding state and real workspace creation.
- [x] Implement direct mission-start workflow without demo data or product-level gates.
- [x] Implement editable settings for provider/model/toolchain/workspace configuration.
  - [x] Provider/model settings are editable through `/api/provider` and persisted to `providers.yaml`.
  - [x] Toolchain settings are editable through `/api/toolchain/packs/{id}` and persisted to `toolchain/<pack>/manifest.yaml`.
  - [x] Workspace settings are editable through `/api/workspace/settings` and persisted to `workspace.yaml`.
- [x] Deepen graph/replay/report cross-navigation and detail views from backend DTOs.
  - [x] Carry real node/file/execution/replay references through Go DTOs, frontend types, and API mapping.
  - [x] Render Chinese graph/replay/report row actions only when backend DTOs expose real targets.
  - [x] Preserve explicit `node_id`, `file_path`, `execution_id`, and `replay_event_id` references from findings JSON.
  - [x] Add richer dedicated detail views for selected graph nodes, replay events, evidence files, and executions.
- [x] Implement main IDE, attack graph, inspector, command palette, tool execution, replay, report, and settings as first-pass separate workbench modes.
- [x] Verify the GUI builds and renders against desktop viewport screenshots.
- **Status:** complete

### Phase 6: Desktop Experience Hardening
- [x] Add real desktop menu actions for workspace, view modes, refresh, and execution access.
- [x] Add Chinese error surfaces for backend crash, API disconnect, workspace failures, and save failures.
- [x] Persist recent workspaces and window state through Electron storage.
- [x] Improve backend sidecar crash detection and restart flow.
- [x] Extend desktop contracts and smoke coverage for desktop lifecycle behavior.
- **Status:** complete

### Phase 7: Tool Execution Productization
- [x] Project executable tool actions from real `toolchain/*/manifest.yaml` data into the GUI.
- [x] Let operators launch manifest-backed tools with editable arguments from the Chinese IDE, writing normal execution records.
- [x] Connect tool runs back into logs, artifacts, replay, graph/report references, and settings without frontend-fabricated rows.
- [x] Add contract and smoke coverage for manifest-backed tool launch.
- **Status:** complete

### Phase 8: Integrated IDE Workflow Depth
- [x] Expand command palette entries so they call real desktop/backend actions instead of static UI shortcuts.
- [ ] Deepen file, execution, artifact, graph, replay, and report cross-jumps as more backend DTO references become available.
  - [x] Execution logs and artifacts can open as real workspace files in the mission file editor.
  - [x] Execution details can jump to related graph nodes, replay events, and report findings using backend DTO references.
  - [x] Report finding details can load linked execution logs/artifacts and open backend-authored log/artifact file paths.
  - [x] Replay event details can load linked execution logs/artifacts, open backend-authored file paths, and jump to related report findings.
  - [x] Graph node details can load linked execution logs/artifacts, open backend-authored file paths, and jump to related report findings.
- [ ] Continue scanning active GUI/API/package surfaces for English copy, demo literals, stale Web UI assumptions, and frontend-fabricated business state.
  - [x] Graph toolbar export is a real backend action that writes `exports/attack-graph.json` and opens it as a workspace file.
  - [x] Report page export is a real backend action that writes `reports/ouroboros-report.md` and opens it as a workspace file.
  - [x] Graph/report export action responses now require backend-authored file paths, counts, and timestamps; the frontend no longer fabricates empty export paths or zero counts.
  - [x] Report TOC buttons switch real report sections for summary, project data, findings, risk distribution, evidence/execution links, and existing report files instead of staying static.
  - [x] Settings navigation switches real current-state, workspace, provider, and toolchain configuration sections instead of pinning the first button active.
  - [x] Custom title-bar window controls call real Electron minimize, maximize/restore, and close actions instead of rendering static squares.
  - [x] Command palette exposes real recent-workspace actions from Electron desktop state instead of leaving recent workspaces only in the native menu.
  - [x] Empty provider settings no longer fabricate `openai_compatible`, enabled state, timeout, or routing defaults in the renderer/API fallback, and settings option labels are Chinese while preserving real config values.
  - [x] Replay events and report findings are no longer derived from legacy runs, graph nodes, or report files in the frontend; they render only when backend bootstrap DTOs provide them.
  - [x] Mission operation messages are no longer synthesized as system/agent conversation rows in the frontend; the panel shows a real empty state unless backend data provides messages later.
  - [x] Right-rail count summary is labeled as a workspace overview instead of an agent graph, and workflow status no longer falls back to frontend graph-count inference.
  - [x] Toolchain settings no longer fabricate `native` runner or `built_in` healthcheck defaults when backend manifests omit those fields.
  - [x] Frontend API mapping no longer fabricates replay/report/graph/file/execution identities, empty execution DTOs, or workspace config paths when backend fields are missing.
  - [x] Frontend bootstrap loading no longer assembles GUI state from legacy `/api/health`, `/api/workspace/info`, `/api/evidence/graph`, `/api/runs`, or `/api/toolchain/health` fallbacks.
  - [x] Dormant task-message types, rows, and `.message.agent` / `.message.system` styles were removed until a real backend message DTO exists.
  - [x] Mission operation panel no longer uses Bot/AI visual semantics, and replay `agent` events render as neutral execution units in Chinese.
  - [x] Settings labels and placeholders no longer expose `Base URL` or concrete example values such as `chat, tools`, shell command samples, or workspace tag samples as visible UI copy.
  - [x] Replay events, report findings, and report documents now require backend-authored IDs; the frontend no longer uses `runId` or `path` as object identity fallbacks.
  - [x] Toolchain packs, tool actions, and workspace files now require backend-authored IDs; the frontend no longer uses pack names, pack IDs, paths, or file names as identity fallbacks.
  - [x] Graph edges now require backend-authored IDs; the frontend no longer generates edge identities from `from`, `to`, and render index.
  - [x] Graph edges now require backend-authored relation types; the frontend no longer assigns missing edge types to `related`.
  - [x] Graph nodes now require backend-authored labels and recognized backend-authored types; the frontend no longer displays node IDs as labels or maps missing/unknown types to `action`.
  - [x] Graph node type support now includes real backend `Artifact` nodes instead of collapsing them into action semantics.
  - [x] Execution artifacts now require backend-authored `path` and `name`; the frontend no longer uses one artifact field to fill the other.
  - [x] Execution artifacts now require backend-authored `filePath`, `size`, and `modifiedAt`; the frontend no longer displays missing artifact metadata as zero or exposes artifacts without backend workspace file paths.
  - [x] Runnable tool actions now require backend-authored labels; the frontend no longer uses pack name or action ID as visible label fallback.
  - [x] Runnable tool actions now require backend-authored visible command text; the frontend no longer joins `command` arrays into display text.
  - [x] Runnable tool actions now require backend-authored status and command arrays, and action-run responses require a complete backend action DTO instead of mapping missing actions to `{}`.
  - [x] Replay event visible detail now stays backend-authored; the frontend no longer displays `runId` as fallback detail text.
  - [x] Report finding affected endpoint now stays backend-authored; the frontend no longer displays path/source as endpoint fallback text.
  - [x] Report finding and report document titles now require backend-authored titles; the frontend no longer displays path or generic report copy as title fallback text.
  - [x] Report finding severity and report document status/path now require complete backend DTOs; the frontend no longer maps missing finding severity to `info` or missing report paths to empty strings.
  - [x] Report appendix evidence rows now require backend-authored file paths; the frontend no longer displays finding `source`, finding IDs, or empty placeholders as evidence text.
  - [x] Workspace file tree labels now require backend-authored paths; the frontend no longer displays file names as path fallback text.
  - [x] Workspace display names now require backend-authored `workspace.name`; the frontend no longer derives workspace names by splitting workspace paths.
  - [x] Execution row labels now require backend-authored labels; the frontend no longer derives visible execution labels from command text.
  - [x] Execution command text now requires backend-authored command fields; the frontend no longer maps missing execution commands to empty strings.
  - [x] Execution row statuses now require backend-authored status fields; the frontend no longer maps missing or unknown execution status to `ready`.
  - [x] Toolchain pack rows and toolchain ready-count summaries now require backend-authored `id`, `name`, `status`, and `toolsReady`; the frontend no longer displays empty pack names/IDs or zero tool counts when backend pack state is incomplete.
  - [x] Mission save/start responses now require backend-authored `path` and `status`; the frontend no longer fabricates mission file paths or task status from incomplete mission DTOs.
  - [x] Replay timeline rows now require backend-authored `type`, `lane`, and `createdAt`; the frontend no longer fabricates generic replay event labels or placeholder timestamps.
  - [x] Workspace file tree metadata now requires backend-authored recognized `kind`; the frontend no longer maps missing or unknown file kinds to generic file copy.
  - [x] Toolchain settings visible metadata now requires backend-authored pack names and manifest paths; the frontend no longer displays pack IDs or placeholder manifest text as fallback UI.
  - [ ] Backend projector still needs cleanup for toolchain health, finding projection, event graph identity, and run timestamp/status synthesis discovered during Phase 8 audit.
  - [ ] Frontend bootstrap and provider-save error handling still needs cleanup so backend/API failures cannot render as ready empty business state.
- [x] Harden event stream reliability for real desktop refresh.
  - [x] WebSocket welcome, heartbeat, and event writes are serialized through one server-side write path to prevent concurrent writer panics during execution events.
- [ ] Keep package smoke aligned with real desktop workflows after each new capability.
  - [x] Desktop GUI smoke now drives report TOC sections and desktop package smoke still passes after regenerating the package directory.
  - [x] Desktop GUI smoke now drives settings sections and verifies persisted workspace/provider/toolchain values through `data-settings-section`.
  - [x] Desktop package smoke now verifies packaged window-control IPC, preload bridge methods, and Chinese window-control copy.
  - [x] Desktop GUI smoke and package smoke pass after requiring complete backend graph/report export result DTOs.
  - [x] Desktop GUI smoke now injects the real desktop bridge shape, executes `打开最近工作区`, and continues against the same real workspace API.
  - [x] Desktop GUI smoke and package smoke pass after removing frontend-fabricated provider defaults and Chinese-localizing settings option labels.
  - [x] Desktop GUI smoke and package smoke pass after removing frontend-derived replay/report fallback data.
  - [x] Desktop GUI smoke and package smoke pass after removing frontend-synthesized mission conversation rows.
  - [x] Desktop GUI smoke and package smoke pass after removing stale agent-graph/next-step UI semantics.
  - [x] Desktop GUI smoke and package smoke pass after removing frontend-fabricated toolchain runner/healthcheck defaults and after the WebSocket event-stream write serialization fix.
  - [x] Desktop GUI smoke and package smoke pass after removing frontend-synthesized object identity/config-path fallbacks and requiring real execution DTOs.
  - [x] Desktop GUI smoke and package smoke pass after making `/api/gui/bootstrap` the only renderer source for aggregate IDE state.
  - [x] Desktop GUI smoke and package smoke pass after removing dormant mission message UI/types/styles while preserving the Chinese empty state.
  - [x] Desktop GUI smoke and package smoke pass after removing Bot/智能体 visual/copy semantics from active GUI surfaces.
  - [x] Desktop GUI smoke and package smoke pass after replacing settings placeholder examples with Chinese instructional copy and adding contract coverage.
  - [x] Desktop GUI smoke and package smoke pass after removing replay/report ID fallback mapping from the renderer.
  - [x] Desktop GUI smoke and package smoke pass after removing toolchain/action/file ID fallback mapping from the renderer.
  - [x] Desktop GUI smoke and package smoke pass after removing graph edge ID generation from the renderer.
  - [x] Desktop GUI smoke and package smoke pass after removing graph edge type fallback mapping from the renderer.
  - [x] Desktop GUI smoke and package smoke pass after requiring backend graph node labels/types and rendering backend `Artifact` nodes as `产物`.
  - [x] Desktop GUI smoke and package smoke pass after removing execution artifact path/name fallback mapping from the renderer.
  - [x] Desktop GUI smoke and package smoke pass after requiring complete backend execution artifact metadata and workspace file paths.
  - [x] Desktop GUI smoke and package smoke pass after removing tool action label fallback mapping from the renderer.
  - [x] Desktop GUI smoke and package smoke pass after removing tool action command-text fallback mapping from the renderer.
  - [x] Desktop GUI smoke and package smoke pass after requiring complete backend tool action DTOs for listing and run responses.
  - [x] Desktop GUI smoke and package smoke pass after removing replay detail `runId` fallback mapping from the renderer.
  - [x] Desktop GUI smoke and package smoke pass after removing report finding endpoint path/source fallback mapping from the renderer.
  - [x] Desktop GUI smoke and package smoke pass after removing report/finding title fallback mapping from the renderer.
  - [x] Desktop GUI smoke and package smoke pass after removing report finding severity and report document status/path fallback mapping from the renderer.
  - [x] Desktop GUI smoke and package smoke pass after removing report appendix evidence source/id fallback mapping from the renderer.
  - [x] Desktop GUI smoke and package smoke pass after removing workspace file label name fallback mapping from the renderer.
  - [x] Desktop GUI smoke and package smoke pass after moving execution row labels into the backend DTO.
  - [x] Desktop GUI smoke and package smoke pass after removing execution command empty-string fallback mapping from the renderer.
  - [x] Desktop GUI smoke and package smoke pass after removing execution status missing/unknown fallback mapping from the renderer.
  - [x] Desktop GUI smoke and package smoke pass after removing toolchain pack status/name/count fallback mapping from the renderer.
  - [x] Desktop GUI smoke and package smoke pass after removing frontend workspace path-to-name fallback mapping.
  - [x] Desktop GUI smoke and package smoke pass after requiring complete backend mission path/status DTOs for save/start responses.
  - [x] Desktop GUI smoke and package smoke pass after requiring complete backend replay event type/lane/timestamp DTOs.
  - [x] Desktop GUI smoke and package smoke pass after requiring complete backend workspace file kind DTOs.
  - [x] Desktop GUI smoke and package smoke pass after requiring complete backend toolchain pack manifest paths for visible settings metadata.
  - [ ] Add a true packaged Electron runtime smoke that starts the packaged app binary and verifies renderer/preload/backend sidecar readiness without injecting a fake desktop bridge.
- **Status:** in progress

## Decisions Made
| Decision | Rationale |
|----------|-----------|
| Stop the Warp route | Removing shell/terminal behavior from Warp is a framework war, not a shortcut. |
| Treat `ui-designs/*.png` as source of truth | The designs already define the product surface better than the current implementation. |
| Keep Go backend capabilities | The backend owns valuable workspace, evidence, replay, toolchain, provider, worker, execution, and report logic. |
| Rebuild inside `gui/` | The stack is suitable; the app-layer implementation is the part that is wrong. |
| GUI-first, CLI/TUI-supporting | The main product must be a desktop IDE, while CLI/TUI remain smoke, automation, and fallback surfaces. |

## Errors Encountered
| Error | Attempt | Resolution |
|-------|---------|------------|
| `node .agents/skills/impeccable/scripts/context.mjs` missing | 1 | Used the installed impeccable reference files and created root `PRODUCT.md` / `DESIGN.md` directly. |
| Tried old paths `gui/src/renderer/styles/globals.css` and `gui/src/renderer/components/IDELayout.tsx` | 1 | Re-read the actual paths under `gui/src/styles/` and `gui/src/components/Layout/`. |
| 1440px desktop screenshot clipped the Start Pentest button | 1 | Reduced mission-grid minimums and added explicit shrink/ellipsis rules for header and panel metadata. |
| Frontend still used fixture data and English UI strings | 1 | Added `api.ts`, centralized Chinese copy in `zh-cn.ts`, deleted `fixtures.ts`, and extended the GUI contract. |
| Desktop GUI smoke expected a different log substring than the command emitted | 1 | Inspected DOM/API evidence and corrected the smoke assertion to the actual real command output. |
| `createWorkspace` cancel path could reinitialize the current workspace | 1 | Added a desktop contract and changed cancel to return an empty selection. |
| `node .agents/skills/impeccable/scripts/context.mjs` still missing | 2 | Followed installed product UI register and recorded the missing local script again. |
| Desktop GUI smoke intermittently failed after opening evidence then switching to replay | 1 | Reproduced the race, moved evidence-file mode switching before the async file read, and made the smoke wait for the real file editor panel before clicking replay. |

## Boundaries
- Do not delete or modify `ui-designs/`.
- Do not touch unrelated dirty Go/TUI/API changes unless explicitly needed for the GUI contract.
- Do not continue the Warp fork path.
- Do not let TUI terminology define the GUI information architecture.
- Expose execution and workspace capabilities as normal software actions.
