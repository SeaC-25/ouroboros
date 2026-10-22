# Design

## Source Of Truth
The GUI source of truth is `F:\Ouroboros\ui-designs\` (portable repository-relative path: `../ui-designs/`).

Primary v0 references:
- `main_ui.png`
- `empty_state.png`
- `start_pentest_modal.png`
- `preflight_check.png`
- `command_palette.png`
- `tool_execution.png`
- `node_inspector.png`
- `replay_view.png`
- `report_view.png`
- `settings_panel.png`
- `ouro_attack_graph.png`

## Product Register
Product UI. Design serves expert workflow, not marketing. Density, consistency, predictable controls, and fast scanning matter more than visual spectacle.

## Runtime Boundary
Ouroboros is a desktop IDE. Electron owns the window, preload bridge, workspace dialogs, window controls, and Go sidecar lifecycle. Vite is only the development/build tool; browser preview is a test harness, not the product runtime.

## Visual Theme
Dark desktop IDE with a strict red-black Ouroboros brand system, thin technical borders, compact spacing, and clear panel separation. Red is reserved for brand, focus, active controls, danger, and critical findings; green is limited to semantic success indicators. Avoid blue-green primary styling, purple-dominant gradients, decorative glow fields, hover scaling, and nested card layouts.
The layout, density, and interaction references are the supplied `ui-designs/` boards, and the real biting-tail snake assets take precedence over abstract logo substitutes.

## Layout System
The primary shell follows `main_ui.png`:
- Top title/brand bar.
- Left activity bar for global modes.
- Left workspace explorer for mission files, targets, artifacts, runs, and reports.
- Central mission workbench for AI conversation and active workflow content.
- Right panel stack for attack graph, agent graph, and inspector.
- Bottom status bar for workspace, workflow, toolchain, provider, backend, and connection state.

Secondary modes are separate routes or workbench modes:
- Attack Graph
- Replay
- Report
- Settings

## Typography
Use one native desktop UI sans stack for labels and interface text: `Segoe UI Variable Text`, `Microsoft YaHei UI`, `PingFang SC`, then system fallbacks. Use `Cascadia Code`, `JetBrains Mono`, or another monospace stack for commands, payloads, paths, timestamps, and log output. Keep UI labels compact, use tabular numerals for status data, and avoid display-sized type inside panels.

## Color Roles
- Background: near-black app base.
- Panel: slightly lifted black-gray surfaces.
- Border: low-contrast gray with stronger active/focus border.
- Text: high-contrast off-white.
- Muted text: readable gray, never below contrast requirements.
- Success: green only for connected services, completed actions, replay progress, and ready backend state.
- Danger/critical: red for high-risk findings, destructive actions, failed executions, and brand accent.
- Info/selection: neutral gray with a red focus border; the attack graph must not introduce a blue or green visual theme.

## Components
Core components must include stable default, hover, focus, active, disabled, loading, empty, and error states:
- Activity bar buttons
- Explorer tree rows
- Workbench tabs
- Command palette
- Modal/dialog
- Status pills
- Toolbar icon buttons
- Graph node and edge renderers
- Inspector sections
- Log/output panes
- Form controls and settings rows

## Motion
Use short state transitions only. No page-load choreography. Respect reduced motion. Motion should communicate selection, panel expansion, command palette open/close, execution progress, and graph focus changes.

## Implementation Rules
- Do not mix the old router shell with the new IDE shell.
- Do not treat mock API shapes as final contracts.
- Expose workspace files uniformly through normal IDE capabilities: list, open, edit, search, index, and display.
- Keep execution controls direct and software-native: run, stop, retry, inspect logs, copy commands, and open artifacts.
- Do not implement broad pages before the main shell and first mission-start vertical slice are coherent.
- Use `ui-designs/` image names in PRs/specs when describing visual acceptance.

## Visual Acceptance Checks
- At 1440x900 the title bar, three-column body, and status bar remain inside the viewport; the right rail must not grow beyond the body height.
- At the minimum supported 960x640 window, secondary rails collapse according to the responsive rules without horizontal scrolling.
- Empty AI workspaces show a compact workbench prompt, not a full-page hero treatment.
- The right-rail attack graph may reflow nodes for its compact viewport, but it must render every backend node and edge; never summarize by truncation.
- Graph, replay, report, and settings views use a focused workbench layout without duplicating the mission right rail; their content remains internally scrollable.
- Scrollable workbench surfaces use the shared dark-red scrollbar treatment and must respect `prefers-reduced-motion`.
- Primary actions and focus states use the brand red; green remains reserved for semantic success and ready states.
