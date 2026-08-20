# Design

## Source Of Truth
The GUI source of truth is `/home/zss/ouroboros/ui-designs/`.

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

## Visual Theme
Dark desktop IDE with restrained red and green operational accents, thin technical borders, compact spacing, and clear panel separation. Avoid purple-dominant gradients, decorative glow fields, hover scaling, and nested card layouts.

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
Use a single product UI sans stack for labels and interface text. Use a monospace stack for commands, payloads, paths, timestamps, and log output. Keep UI labels compact and avoid display-sized type inside panels.

## Color Roles
- Background: near-black app base.
- Panel: slightly lifted black-gray surfaces.
- Border: low-contrast gray with stronger active/focus border.
- Text: high-contrast off-white.
- Muted text: readable gray, never below contrast requirements.
- Success/active: green for connected services, completed actions, replay progress, and ready backend state.
- Danger/critical: red for high-risk findings, destructive actions, failed executions, and brand accent.
- Info/selection: cool blue only where the attack graph requires node selection clarity.

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
