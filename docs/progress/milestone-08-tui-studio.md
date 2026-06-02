# Milestone 08: TUI Studio

## Implemented
- Added Bubble Tea, Lip Gloss, and Bubbles dependencies.
- Added TUI shell model in `internal/tui/app.go`.
- Added viewport resize handling through `tea.WindowSizeMsg`.
- Added three-column layout calculation in `internal/tui/layout.go` for Workspace, Mission Conversation / Workbench, and Graph Stack.
- Added bottom status bar rendering in `internal/tui/statusbar.go`.
- Added viewport minimum tests and shell render tests in `internal/tui/layout_test.go`.
- Added Workspace Panel model in `internal/tui/workspace.go`.
- Added canonical workspace entries for `mission.yaml`, `scope.yaml`, `artifacts`, `evidence`, `attempts`, `findings`, `reports`, `replay`, and `toolchain`.
- Added selected path state and sorted tree row rendering.
- Wired Workspace Panel into the left column of the TUI shell.
- Added tests in `internal/tui/workspace_test.go` for canonical order, unknown entry sorting, and selected entry rendering.
- Added Mission Conversation model in `internal/tui/conversation.go`.
- Added Agent Workbench model in `internal/tui/workbench.go`.
- Wired the center panel to show Mission Conversation by default and Agent Workbench when an agent is focused.
- Added `Esc` handling to return from Agent Workbench to Mission Conversation.
- Added tests in `internal/tui/workbench_test.go` for mission runtime state, default conversation rendering, agent-focused workbench rendering, and `Esc` focus clearing.
- Added text Attack Graph rendering in `internal/tui/attack_graph.go`.
- Added Agent Graph status and progress rendering in `internal/tui/agent_graph.go`.
- Added Graph Stack and Inspector detail rendering in `internal/tui/inspector.go`.
- Wired Graph Stack into the right column of the TUI shell.
- Added tests in `internal/tui/graph_stack_test.go` for Attack Graph rows, Agent Graph progress, selected node inspection, focus changes, and model integration.
- Added Runtime Tabs in `internal/tui/runtime_tabs.go`.
- Added tabs for Node Inspector, Model Runtime, Agent Runtime, Toolchain Health, Coverage Matrix, Frontier Queue, and Timeline Lanes.
- Replaced the right-column bottom Inspector render with Runtime Tabs while keeping Node Inspector as the default active tab.
- Added tests in `internal/tui/runtime_tabs_test.go` for default tab rendering, tab switching, and Graph Stack selected-node integration.

## Not Implemented
- The TUI is not yet wired into `ouro run`; production run path still returns not implemented.
- Runtime tab panels currently show placeholder rows; real model/runtime/toolchain/frontier/timeline projections are implemented by later milestones.

## Validation
- `GOPROXY=https://goproxy.cn,direct go test ./internal/tui -v`
- `GOPROXY=https://goproxy.cn,direct go test ./...`

## Next Step
- Continue Milestone 9 with Task 9.1: Domain Pack SDK.

## Risks / Decisions
- The first TUI slice uses a minimum viewport of `80x9` to keep the three-column IDE readable.
- The status bar switches to a compact format when full telemetry does not fit, preserving model, context, agent count, tool health, and cost first.
- Panel content is intentionally skeletal in this slice; later tasks replace placeholder rows with real workspace, graph, runtime, and event projections.
- Workspace sorting uses a canonical pentest workspace order before sorting unknown entries by display name.
- The center panel keeps the title `Mission Conversation / Workbench` in conversation mode to preserve the product-level layout label, while selected agents render as `Agent Workbench`.
- The right column is now a vertical Graph Stack. Inspector content is derived from the current attack-node or agent selection instead of being an independent text panel.
- Runtime Tabs wrap tab headers across rows to preserve tab names in narrow right-column widths.
