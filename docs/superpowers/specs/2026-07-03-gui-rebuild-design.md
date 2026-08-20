# GUI Rebuild Design

## Status
Approved direction from user: discard the old mixed GUI implementation and failed Warp adaptation route. Preserve backend capabilities and rebuild the GUI as a desktop penetration testing IDE.

## Problem
Ouroboros has strong Go backend capabilities and a complete set of GUI design images, but the current product is split across TUI-first documentation, a partially migrated Electron renderer, an unused old router, backup components, mock API responses, and abandoned Warp migration ideas. Continuing to patch the current surface will preserve the split.

## Product Direction
Ouroboros becomes a GUI-first IDE for authorized penetration testing. CLI and TUI remain support surfaces for automation, smoke tests, fallback operation, and backend development. The GUI defines the primary operator experience.

## Non-Goals
- Do not continue the Warp fork or terminal-shell extraction path.
- Do not delete Go backend workspace, replay, evidence, provider, toolchain, worker, or report logic.
- Expose workspace files uniformly through normal IDE capabilities: list, open, edit, search, index, and display.
- Keep execution controls direct and software-native: run, stop, retry, inspect logs, copy commands, and open artifacts.
- Do not build a generic dashboard.
- Do not start by wiring every backend feature into the UI.

## Architecture
Keep the existing Electron + Vite + React stack in `gui/`.

Replace the renderer app-layer with these boundaries:
- `app/`: providers, router, shell state, keyboard command services.
- `features/workspace/`: explorer, workspace summary, mission files.
- `features/mission/`: empty state, onboarding, start pentest, backend readiness, AI workbench.
- `features/graph/`: attack graph canvas, mini graph, node inspector.
- `features/execution/`: tool execution monitor and review state.
- `features/replay/`: timeline, event list, snapshot player.
- `features/report/`: report reader/editor.
- `features/settings/`: settings categories and forms.
- `shared/ui/`: reusable controls, tokens, icons, layout primitives.
- `shared/api/`: DTOs, fixtures, API client, WebSocket client.

## Primary Views
The v0 route map should be:
- `/` -> shell with main mission workbench.
- `/graph` -> full attack graph mode.
- `/replay` -> replay timeline and graph snapshot.
- `/report` -> report view.
- `/settings` -> settings.

Global overlays:
- Command palette.
- Start pentest modal.
- Node inspector drawer/panel.
- Tool execution details when invoked from workflow state.

## First Vertical Slice
Build only enough live behavior to prove the product:
1. Empty state / onboarding.
2. Start pentest modal.
3. Backend readiness and workspace file check.
4. Main IDE shell.
5. Attack graph preview.
6. Node inspector.
7. Status bar with workspace, provider, toolchain, backend, and connection state.

This slice should be design-faithful before deeper API integration.

## Data Contract
Frontend should consume GUI DTOs, not Go internal types:
- `WorkspaceDTO`
- `MissionDTO`
- `WorkflowStatusDTO`
- `ProviderStatusDTO`
- `ToolchainStatusDTO`
- `EvidenceGraphDTO`
- `GraphNodeDTO`
- `GraphEdgeDTO`
- `ExecutionDTO`
- `ReplayTimelineDTO`
- `ReportDTO`
- `SettingsDTO`

The old fixture-first stage is complete. The next stage must consume real Go API data and must remove fixture or mock-shaped product state.

## Error Handling
The shell must represent disconnected backend, empty workspace, stale replay database, missing provider, missing toolchain, and failed execution as first-class UI states. These are not toast-only errors.

## Testing
Minimum verification for the GUI rebuild:
- `npm run build` from `gui/`.
- TypeScript compile must pass.
- Screenshot checks for main shell, empty state, start modal, preflight, graph, replay, report, and settings.
- Manual keyboard checks for command palette, focus states, modal dismissal, and route switching.

## Acceptance Criteria
- The first screen resembles `ui-designs/main_ui.png` in structure, density, and tone.
- Old Warp and terminal-shell assumptions are absent from the renderer architecture.
- The unused old router and backup implementation are removed or quarantined outside the active app path.
- GUI state is backed by explicit DTOs from the Go API, without fixture data or generated demo state.
- The renderer exposes execution and workspace capabilities as direct software actions.
- No UI page relies on purple gradients, hover-scale card lists, or generic dashboard composition.
