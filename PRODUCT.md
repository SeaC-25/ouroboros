# Product

## Register

product

## Users
Ouroboros is for security operators, red-team engineers, internal security teams, and lab/CTF users who need a mission workspace that makes evidence, tools, AI assistance, execution state, replay, and reports visible in one desktop environment.

## Product Purpose
Ouroboros is a GUI-first penetration testing IDE. It turns operator-written project materials and backend runtime state into a complete desktop workspace with AI-assisted planning, evidence graphs, tool execution visibility, replay, reports, and settings.

The CLI and TUI remain important for automation, smoke tests, fallback workflows, and backend capability development, but they no longer define the primary product experience.

## Brand Personality
Precise, controlled, and operational. The interface should feel like a serious security workbench: dense when needed, calm under pressure, and explicit about evidence, execution state, backend connectivity, and next operator action.

## Anti-references
- Do not copy Warp's terminal-first shell model.
- Do not make a generic SaaS dashboard.
- Do not make a decorative cyberpunk toy UI with oversized cards, purple gradients, or animated visual noise.
- Do not let TUI panels and command output dictate the GUI's information architecture.

## Design Principles
- **Mission first:** every primary screen should answer what the operator is doing, what is known, what is running, and what output changed.
- **Evidence over decoration:** graph nodes, logs, reports, and inspector details should be grounded in workspace artifacts and replay state.
- **Operator-directed execution:** execution controls are normal software controls: run, stop, retry, inspect output, and record results.
- **IDE familiarity:** use proven desktop IDE patterns: activity bar, explorer, workbench, command palette, panels, status bar, and settings.
- **Backend preservation:** keep the Go workspace, replay, toolchain, provider, worker, evidence, and report logic as the capability core.

## Software Boundary
Ouroboros exposes IDE, backend, tool execution, AI assistance, replay, evidence, reporting, settings, and workspace-file capabilities as direct product capabilities. Project-specific operating policy belongs to the operator's workflow and source material, not to app-level product state.

All workspace files are regular project files. The IDE can list, open, edit, search, index, and display them through normal product features, but it must not turn any specific file type into a blocking product state.

## Accessibility & Inclusion
Target WCAG AA contrast for text and controls. Keyboard navigation, visible focus states, reduced-motion behavior, non-color-only status indicators, and readable monospace data views are required for the desktop GUI.
