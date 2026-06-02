package tui

import (
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ouroboros/ouroboros/internal/eventlog"
)

type Model struct {
	layout                  Layout
	status                  Status
	workspacePanel          WorkspacePanel
	conversation            MissionConversation
	agentWorkbench          *AgentWorkbench
	graphStack              GraphStack
	workspace               viewport.Model
	workbench               viewport.Model
	graphs                  viewport.Model
	replayEvents            []eventlog.Event
	replayCursor            int
	runtimeActionRequester  RuntimeActionRequester
	runtimeActionExecutor   RuntimeActionExecutor
	runtimeControlRequester RuntimeControlRequester
	runtimeControlRows      []string
	runCancelArmed          bool
	runPaused               bool
}

type RuntimeActionRequester func(RuntimeAction) (RuntimeActionRequestResult, error)
type RuntimeActionExecutor func(RuntimeAction) (RuntimeActionExecutionResult, error)
type RuntimeControlRequester func(RuntimeControlAction) (RuntimeControlResult, error)

type RuntimeControlAction struct {
	Kind string
}

type RuntimeControlResult struct {
	Status  string
	Message string
}

type RuntimeControlResultMsg struct {
	Action RuntimeControlAction
	Result RuntimeControlResult
	Err    error
}

type RuntimeActionRequestResultMsg struct {
	Action RuntimeAction
	Result RuntimeActionRequestResult
	Err    error
}

type RuntimeActionExecutionResult struct {
	Status      string
	Path        string
	Output      string
	RuntimeRows []string
}

type RuntimeActionExecutionResultMsg struct {
	Action RuntimeAction
	Result RuntimeActionExecutionResult
	Err    error
}

func NewModel() Model {
	model := Model{
		layout:         NewLayout(MinTotalWidth, MinTotalHeight),
		status:         DefaultStatus(),
		workspacePanel: DefaultWorkspacePanel(),
		conversation:   DefaultMissionConversation(),
		graphStack:     DefaultGraphStack(),
	}
	model.resizeViewports()
	return model
}

func NewProgram() *tea.Program {
	return tea.NewProgram(NewModel(), tea.WithAltScreen())
}

func (m Model) Init() tea.Cmd {
	return nil
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch typed := msg.(type) {
	case ReplaySnapshotMsg:
		return m.WithReplaySnapshot(typed.Snapshot), nil
	case ReplayTimelineMsg:
		return m.WithReplayEvents(typed.Events), nil
	case RuntimeActionRequestResultMsg:
		return m.WithRuntimeActionRequestResult(typed), nil
	case RuntimeActionExecutionResultMsg:
		return m.WithRuntimeActionExecutionResult(typed), nil
	case RuntimeControlResultMsg:
		return m.WithRuntimeControlResult(typed), nil
	case tea.WindowSizeMsg:
		m.layout = NewLayout(typed.Width, typed.Height)
		m.resizeViewports()
		return m, nil
	case tea.KeyMsg:
		switch typed.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "esc":
			return m.ClearAgentFocus(), nil
		case "left":
			return m.StepReplay(-1), nil
		case "right":
			return m.StepReplay(1), nil
		case "home", "g":
			return m.JumpReplay(0), nil
		case "end", "G":
			return m.JumpReplay(len(m.replayEvents)), nil
		case "up":
			return m.MoveRuntimeSelection(-1), nil
		case "down":
			return m.MoveRuntimeSelection(1), nil
		case "enter":
			return m.ActivateRuntimeSelection(), nil
		case "d":
			return m.ShowRuntimeActionDetails(), nil
		case "r":
			return m.RequestRuntimeAction()
		case "R":
			return m.ConfirmOrExecuteRuntimeActionWithControl("restart")
		case "L":
			return m.ConfirmOrExecuteRuntimeActionWithControl("renew")
		case "C":
			return m.ConfirmOrExecuteRuntimeActionWithControl("cancel")
		case "E":
			return m.ConfirmOrExecuteRuntimeActionWithControl("expire")
		case "x":
			return m.ConfirmOrExecuteRuntimeAction()
		case "c":
			return m.ConfirmOrRequestRunCancel()
		case "p":
			return m.RequestRunPauseToggle()
		}
	}
	return m, nil
}

func (m Model) FocusAgent(workbench AgentWorkbench) Model {
	m.agentWorkbench = &workbench
	return m
}

func (m Model) ClearAgentFocus() Model {
	m.agentWorkbench = nil
	return m
}

func (m Model) HasAgentFocus() bool {
	return m.agentWorkbench != nil
}

func (m Model) WithGraphStack(stack GraphStack) Model {
	m.graphStack = stack
	return m
}

func (m Model) WithRuntimeActionRequester(requester RuntimeActionRequester) Model {
	m.runtimeActionRequester = requester
	return m
}

func (m Model) WithRuntimeActionExecutor(executor RuntimeActionExecutor) Model {
	m.runtimeActionExecutor = executor
	return m
}

func (m Model) WithRuntimeControlRequester(requester RuntimeControlRequester) Model {
	m.runtimeControlRequester = requester
	return m
}

func (m Model) WithInstallRequestStatus(summary string) Model {
	m.status.InstallRequests = strings.TrimSpace(summary)
	return m
}

func (m Model) SelectRuntimeTab(tab RuntimeTab) Model {
	stack := m.graphStack
	stack.RuntimeTabs = stack.RuntimeTabs.Select(tab)
	m.graphStack = stack
	return m
}

func (m Model) AppendRuntimeRows(tab RuntimeTab, rows []string) Model {
	stack := m.graphStack
	stack.RuntimeTabs = stack.RuntimeTabs.AppendRows(tab, rows)
	m.graphStack = stack
	return m
}

func (m Model) PrependRuntimeRows(tab RuntimeTab, rows []string) Model {
	stack := m.graphStack
	stack.RuntimeTabs = stack.RuntimeTabs.PrependRowsPreservingAction(tab, rows)
	m.graphStack = stack
	return m
}

func (m Model) MoveRuntimeSelection(delta int) Model {
	stack := m.graphStack
	stack.RuntimeTabs = stack.RuntimeTabs.SelectRow(delta)
	m.graphStack = stack
	return m
}

func (m Model) WithRuntimeControlRow(row string) Model {
	row = strings.TrimSpace(row)
	if row == "" {
		return m
	}
	m.runtimeControlRows = append(m.runtimeControlRows, row)
	stack := m.graphStack
	if len(stack.RuntimeTabs.Tabs) == 0 {
		stack.RuntimeTabs = DefaultRuntimeTabs()
	}
	stack.RuntimeTabs = stack.RuntimeTabs.AppendRows(RuntimeTabAgentRuntime, []string{row})
	m.graphStack = stack
	return m
}

func (m Model) ActivateRuntimeSelection() Model {
	stack := m.graphStack
	stack.RuntimeTabs = stack.RuntimeTabs.ActivateSelectedRow()
	m.graphStack = stack
	return m
}

func (m Model) ConfirmOrRequestRunCancel() (Model, tea.Cmd) {
	if m.runtimeControlRequester == nil {
		m.runCancelArmed = false
		return m.WithRuntimeControlRow("run control: cancel unavailable requester-not-configured"), nil
	}
	if !m.runCancelArmed {
		m.runCancelArmed = true
		return m.WithRuntimeControlRow("run control: cancel confirm-required"), nil
	}
	m.runCancelArmed = false
	action := RuntimeControlAction{Kind: "cancel-run"}
	requester := m.runtimeControlRequester
	m = m.WithRuntimeControlRow("run control: cancel requesting")
	return m, func() tea.Msg {
		result, err := requester(action)
		return RuntimeControlResultMsg{Action: action, Result: result, Err: err}
	}
}

func (m Model) RequestRunPauseToggle() (Model, tea.Cmd) {
	if m.runtimeControlRequester == nil {
		return m.WithRuntimeControlRow("run control: pause unavailable requester-not-configured"), nil
	}
	action := RuntimeControlAction{Kind: "pause-run"}
	label := "pause"
	if m.runPaused {
		action.Kind = "resume-run"
		label = "resume"
	}
	requester := m.runtimeControlRequester
	m = m.WithRuntimeControlRow("run control: " + label + " requesting")
	return m, func() tea.Msg {
		result, err := requester(action)
		return RuntimeControlResultMsg{Action: action, Result: result, Err: err}
	}
}

func (m Model) WithRuntimeControlResult(msg RuntimeControlResultMsg) Model {
	label := runtimeControlLabel(msg.Action.Kind)
	if msg.Err != nil {
		return m.WithRuntimeControlRow("run control: " + label + " failed " + msg.Err.Error())
	}
	switch strings.TrimSpace(msg.Action.Kind) {
	case "pause-run":
		m.runPaused = true
	case "resume-run":
		m.runPaused = false
	}
	status := valueOrDefault(msg.Result.Status, "requested")
	message := strings.TrimSpace(msg.Result.Message)
	row := "run control: " + label + " " + status
	if message != "" {
		row += " " + message
	}
	return m.WithRuntimeControlRow(row)
}

func runtimeControlLabel(kind string) string {
	switch strings.TrimSpace(kind) {
	case "pause-run":
		return "pause"
	case "resume-run":
		return "resume"
	default:
		return "cancel"
	}
}

func (m Model) RequestRuntimeAction() (Model, tea.Cmd) {
	stack := m.graphStack
	tabs := stack.RuntimeTabs
	if len(tabs.Tabs) == 0 {
		tabs = DefaultRuntimeTabs()
	}
	action, ok := tabs.ActiveRuntimeAction()
	if !ok {
		tabs = tabs.ActivateSelectedRow()
		action, ok = tabs.ActiveRuntimeAction()
	}
	if !ok {
		stack.RuntimeTabs = tabs
		m.graphStack = stack
		return m, nil
	}
	if runtimeActionIsReadOnly(action) {
		action = runtimeActionWithUnsupportedRequest(action, "runtime action is read-only")
		tabs = tabs.WithActiveRuntimeAction(action)
		stack.RuntimeTabs = tabs
		m.graphStack = stack
		return m, nil
	}
	if !runtimeActionSupportsRequest(action) {
		action = runtimeActionWithUnsupportedRequest(action, "runtime action does not support requests")
		tabs = tabs.WithActiveRuntimeAction(action)
		stack.RuntimeTabs = tabs
		m.graphStack = stack
		return m, nil
	}
	if m.runtimeActionRequester == nil {
		action.RequestStatus = "unavailable"
		action.RequestError = "runtime action requester is not configured"
		tabs = tabs.WithActiveRuntimeAction(action)
		stack.RuntimeTabs = tabs
		m.graphStack = stack
		return m, nil
	}
	action.RequestStatus = "requesting"
	action.RequestPath = ""
	action.RequestError = ""
	tabs = tabs.WithActiveRuntimeAction(action)
	stack.RuntimeTabs = tabs
	m.graphStack = stack
	requester := m.runtimeActionRequester
	return m, func() tea.Msg {
		result, err := requester(action)
		return RuntimeActionRequestResultMsg{Action: action, Result: result, Err: err}
	}
}

func runtimeActionSupportsRequest(action RuntimeAction) bool {
	return strings.TrimSpace(action.Kind) == "install"
}

func runtimeActionWithUnsupportedRequest(action RuntimeAction, requestError string) RuntimeAction {
	action.RequestStatus = "unsupported"
	action.RequestPath = ""
	action.RequestError = requestError
	return action
}

func (m Model) WithRuntimeActionRequestResult(msg RuntimeActionRequestResultMsg) Model {
	stack := m.graphStack
	tabs := stack.RuntimeTabs
	if len(tabs.Tabs) == 0 {
		tabs = DefaultRuntimeTabs()
	}
	action, ok := tabs.ActiveRuntimeAction()
	if !ok {
		action = msg.Action
	}
	if msg.Err != nil {
		action.RequestStatus = "failed"
		action.RequestPath = ""
		action.RequestError = msg.Err.Error()
	} else {
		action.RequestStatus = valueOrDefault(msg.Result.Status, "pending")
		action.RequestPath = msg.Result.Path
		action.RequestError = ""
	}
	tabs = tabs.WithActiveRuntimeAction(action)
	stack.RuntimeTabs = tabs
	if stack.Focus == GraphFocusRuntimeAction && stack.HasRuntimeAction {
		stack.RuntimeAction = action
	}
	m.graphStack = stack
	return m
}

func (m Model) ConfirmOrExecuteRuntimeAction() (Model, tea.Cmd) {
	return m.ConfirmOrExecuteRuntimeActionWithControl("")
}

func (m Model) ConfirmOrExecuteRuntimeActionWithControl(controlAction string) (Model, tea.Cmd) {
	stack := m.graphStack
	tabs := stack.RuntimeTabs
	if len(tabs.Tabs) == 0 {
		tabs = DefaultRuntimeTabs()
	}
	action, ok := tabs.ActiveRuntimeAction()
	if !ok {
		tabs = tabs.ActivateSelectedRow()
		action, ok = tabs.ActiveRuntimeAction()
	}
	if !ok {
		stack.RuntimeTabs = tabs
		m.graphStack = stack
		return m, nil
	}
	kind := strings.TrimSpace(action.Kind)
	controlAction = strings.TrimSpace(controlAction)
	if kind != "compile-preview" && kind != "compile-preview-review-artifact" && kind != "compile-preview-review-selection" && kind != "report-projection-dispatch-artifact" && kind != "report-projection-dispatch-execution-artifact" && kind != "report-projection-execution-selection" && kind != "replay-workbench-session-artifact" && kind != "replay-workbench-session-selection" && kind != "replay-workbench-collaboration-artifact" && kind != "replay-workbench-collaboration-selection" && kind != "replay-workbench-collaboration-note" && kind != "replay-workbench-collaboration-note-selection" && kind != "install-dispatch-artifact" && kind != "install-retry-dispatch-artifact" && kind != "worker-lifecycle-action-plan-artifact" && kind != "worker-lifecycle-action-plan-selection" && kind != "worker-lifecycle-policy-artifact" && kind != "worker-lifecycle-policy-selection" && kind != "worker-lease-result-reconciliation-artifact" && kind != "worker-lease-reclaim-artifact" && kind != "worker-placement-artifact" && kind != "worker-placement-dispatch-artifact" && kind != "worker-queue-claim-artifact" && kind != "worker-transport-request-artifact" && kind != "worker-transport-daemon-artifact" {
		action.ExecutionStatus = "unsupported"
		action.ExecutionError = "runtime action is not executable"
		tabs = tabs.WithActiveRuntimeAction(action)
		stack.RuntimeTabs = tabs
		m.graphStack = stack
		return m, nil
	}
	if kind == "compile-preview-review-selection" && strings.TrimSpace(action.State) != "current" {
		action.ExecutionStatus = "unsupported"
		action.ExecutionError = "runtime action is not executable"
		tabs = tabs.WithActiveRuntimeAction(action)
		stack.RuntimeTabs = tabs
		m.graphStack = stack
		return m, nil
	}
	if kind == "worker-lifecycle-policy-selection" && strings.TrimSpace(action.State) != "current" {
		action.ExecutionStatus = "unsupported"
		action.ExecutionError = "runtime action is not executable"
		tabs = tabs.WithActiveRuntimeAction(action)
		stack.RuntimeTabs = tabs
		m.graphStack = stack
		return m, nil
	}
	if kind == "worker-lifecycle-policy-artifact" && strings.TrimSpace(action.Current) != "false" {
		action.ExecutionStatus = "unsupported"
		action.ExecutionError = "runtime action is not executable"
		tabs = tabs.WithActiveRuntimeAction(action)
		stack.RuntimeTabs = tabs
		m.graphStack = stack
		return m, nil
	}
	if kind == "worker-lifecycle-action-plan-selection" && strings.TrimSpace(action.State) != "current" {
		action.ExecutionStatus = "unsupported"
		action.ExecutionError = "runtime action is not executable"
		tabs = tabs.WithActiveRuntimeAction(action)
		stack.RuntimeTabs = tabs
		m.graphStack = stack
		return m, nil
	}
	if kind == "worker-lifecycle-action-plan-artifact" && strings.TrimSpace(action.Current) != "false" {
		action.ExecutionStatus = "unsupported"
		action.ExecutionError = "runtime action is not executable"
		tabs = tabs.WithActiveRuntimeAction(action)
		stack.RuntimeTabs = tabs
		m.graphStack = stack
		return m, nil
	}
	if kind == "replay-workbench-session-selection" && strings.TrimSpace(action.State) != "current" {
		action.ExecutionStatus = "unsupported"
		action.ExecutionError = "runtime action is not executable"
		tabs = tabs.WithActiveRuntimeAction(action)
		stack.RuntimeTabs = tabs
		m.graphStack = stack
		return m, nil
	}
	if kind == "replay-workbench-collaboration-selection" && strings.TrimSpace(action.State) != "current" {
		action.ExecutionStatus = "unsupported"
		action.ExecutionError = "runtime action is not executable"
		tabs = tabs.WithActiveRuntimeAction(action)
		stack.RuntimeTabs = tabs
		m.graphStack = stack
		return m, nil
	}
	if kind == "replay-workbench-collaboration-note-selection" && strings.TrimSpace(action.State) != "current" {
		action.ExecutionStatus = "unsupported"
		action.ExecutionError = "runtime action is not executable"
		tabs = tabs.WithActiveRuntimeAction(action)
		stack.RuntimeTabs = tabs
		m.graphStack = stack
		return m, nil
	}
	if kind == "report-projection-execution-selection" && strings.TrimSpace(action.State) != "current" {
		action.ExecutionStatus = "unsupported"
		action.ExecutionError = "runtime action is not executable"
		tabs = tabs.WithActiveRuntimeAction(action)
		stack.RuntimeTabs = tabs
		m.graphStack = stack
		return m, nil
	}
	if (kind == "report-projection-dispatch-artifact" || kind == "report-projection-dispatch-execution-artifact") && runtimeActionIsReadOnly(action) {
		action.ExecutionStatus = "unsupported"
		action.ExecutionError = "runtime action is not executable"
		tabs = tabs.WithActiveRuntimeAction(action)
		stack.RuntimeTabs = tabs
		m.graphStack = stack
		return m, nil
	}
	controlChanged := false
	if controlAction != "" {
		if kind != "worker-transport-daemon-artifact" && !(kind == "worker-queue-claim-artifact" && (controlAction == "cancel" || controlAction == "expire" || controlAction == "renew")) {
			action.ExecutionStatus = "unsupported"
			action.ExecutionError = "runtime action does not support control action " + controlAction
			tabs = tabs.WithActiveRuntimeAction(action)
			stack.RuntimeTabs = tabs
			m.graphStack = stack
			return m, nil
		}
		if strings.TrimSpace(action.ControlAction) != controlAction {
			controlChanged = true
		}
		action.ControlAction = controlAction
	} else if kind == "worker-transport-daemon-artifact" {
		if strings.TrimSpace(action.ControlAction) != "stop" {
			controlChanged = true
		}
		action.ControlAction = "stop"
	} else if strings.TrimSpace(action.ControlAction) != "" {
		controlChanged = true
		action.ControlAction = ""
	}
	if strings.TrimSpace(action.ExecutionStatus) != "confirm-required" || controlChanged {
		action.ExecutionStatus = "confirm-required"
		action.ExecutionPath = ""
		action.ExecutionError = ""
		tabs = tabs.WithActiveRuntimeAction(action)
		stack.RuntimeTabs = tabs
		m.graphStack = stack
		return m, nil
	}
	if m.runtimeActionExecutor == nil {
		action.ExecutionStatus = "unavailable"
		action.ExecutionPath = ""
		action.ExecutionError = "runtime action executor is not configured"
		tabs = tabs.WithActiveRuntimeAction(action)
		stack.RuntimeTabs = tabs
		m.graphStack = stack
		return m, nil
	}
	action.ExecutionStatus = "executing"
	action.ExecutionPath = ""
	action.ExecutionError = ""
	tabs = tabs.WithActiveRuntimeAction(action)
	stack.RuntimeTabs = tabs
	m.graphStack = stack
	executor := m.runtimeActionExecutor
	return m, func() tea.Msg {
		result, err := executor(action)
		return RuntimeActionExecutionResultMsg{Action: action, Result: result, Err: err}
	}
}

func (m Model) WithRuntimeActionExecutionResult(msg RuntimeActionExecutionResultMsg) Model {
	stack := m.graphStack
	tabs := stack.RuntimeTabs
	if len(tabs.Tabs) == 0 {
		tabs = DefaultRuntimeTabs()
	}
	action, ok := tabs.ActiveRuntimeAction()
	if !ok {
		action = msg.Action
	}
	if msg.Err != nil {
		action.ExecutionStatus = valueOrDefault(msg.Result.Status, "failed")
		action.ExecutionPath = msg.Result.Path
		action.Output = msg.Result.Output
		action.ExecutionError = msg.Err.Error()
	} else {
		action.ExecutionStatus = valueOrDefault(msg.Result.Status, "completed")
		action.ExecutionPath = msg.Result.Path
		action.ExecutionError = ""
		action.Output = msg.Result.Output
	}
	if len(msg.Result.RuntimeRows) > 0 {
		targetTab := RuntimeTabToolchainHealth
		if strings.TrimSpace(action.Kind) == "report-projection-dispatch-artifact" || strings.TrimSpace(action.Kind) == "report-projection-dispatch-execution-artifact" || strings.TrimSpace(action.Kind) == "report-projection-execution-selection" || strings.TrimSpace(action.Kind) == "replay-workbench-session-artifact" || strings.TrimSpace(action.Kind) == "replay-workbench-session-selection" || strings.TrimSpace(action.Kind) == "replay-workbench-collaboration-artifact" || strings.TrimSpace(action.Kind) == "replay-workbench-collaboration-selection" || strings.TrimSpace(action.Kind) == "replay-workbench-collaboration-note" || strings.TrimSpace(action.Kind) == "replay-workbench-collaboration-note-selection" {
			targetTab = RuntimeTabTimelineLanes
		}
		if strings.TrimSpace(action.Kind) == "worker-lifecycle-action-plan-artifact" || strings.TrimSpace(action.Kind) == "worker-lifecycle-action-plan-selection" || strings.TrimSpace(action.Kind) == "worker-lifecycle-policy-artifact" || strings.TrimSpace(action.Kind) == "worker-lifecycle-policy-selection" || strings.TrimSpace(action.Kind) == "worker-lease-result-reconciliation-artifact" || strings.TrimSpace(action.Kind) == "worker-lease-reclaim-artifact" || strings.TrimSpace(action.Kind) == "worker-placement-artifact" || strings.TrimSpace(action.Kind) == "worker-placement-dispatch-artifact" || strings.TrimSpace(action.Kind) == "worker-queue-claim-artifact" || strings.TrimSpace(action.Kind) == "worker-transport-request-artifact" || strings.TrimSpace(action.Kind) == "worker-transport-daemon-artifact" {
			targetTab = RuntimeTabAgentRuntime
		}
		path := strings.TrimSpace(msg.Result.Path)
		if strings.TrimSpace(action.Kind) == "compile-preview-review-artifact" || strings.TrimSpace(action.Kind) == "compile-preview-review-selection" {
			tabs = tabs.ReplaceRowsByPrefixesPreservingAction(targetTab, compilePreviewReviewRuntimeRowPrefixes(), msg.Result.RuntimeRows)
		} else if strings.TrimSpace(action.Kind) == "report-projection-dispatch-artifact" || strings.TrimSpace(action.Kind) == "report-projection-dispatch-execution-artifact" || strings.TrimSpace(action.Kind) == "report-projection-execution-selection" {
			tabs = tabs.ReplaceRowsByPrefixesPreservingAction(targetTab, reportProjectionDispatchExecutionRuntimeRowPrefixes(), msg.Result.RuntimeRows)
		} else if strings.TrimSpace(action.Kind) == "replay-workbench-session-artifact" || strings.TrimSpace(action.Kind) == "replay-workbench-session-selection" {
			tabs = tabs.ReplaceRowsByPrefixesPreservingAction(targetTab, replayWorkbenchSessionRuntimeRowPrefixes(), msg.Result.RuntimeRows)
		} else if strings.TrimSpace(action.Kind) == "replay-workbench-collaboration-artifact" || strings.TrimSpace(action.Kind) == "replay-workbench-collaboration-selection" {
			tabs = tabs.ReplaceRowsByPrefixesPreservingAction(targetTab, replayWorkbenchCollaborationRuntimeRowPrefixes(), msg.Result.RuntimeRows)
		} else if strings.TrimSpace(action.Kind) == "replay-workbench-collaboration-note" || strings.TrimSpace(action.Kind) == "replay-workbench-collaboration-note-selection" {
			tabs = tabs.ReplaceRowsByPrefixesPreservingAction(targetTab, replayWorkbenchCollaborationNoteRuntimeRowPrefixes(), msg.Result.RuntimeRows)
		} else if strings.TrimSpace(action.Kind) == "worker-lifecycle-action-plan-artifact" || strings.TrimSpace(action.Kind) == "worker-lifecycle-action-plan-selection" {
			tabs = tabs.ReplaceRowsByPrefixesPreservingAction(targetTab, workerLifecycleActionPlanRuntimeRowPrefixes(), msg.Result.RuntimeRows)
		} else if strings.TrimSpace(action.Kind) == "worker-lifecycle-policy-artifact" || strings.TrimSpace(action.Kind) == "worker-lifecycle-policy-selection" {
			tabs = tabs.ReplaceRowsByPrefixesPreservingAction(targetTab, workerLifecyclePolicyRuntimeRowPrefixes(), msg.Result.RuntimeRows)
		} else if strings.Contains(path, "worker-placement-dispatch-batches") || strings.Contains(path, "worker-lease-result-reconciliation-executions") || strings.Contains(path, "worker-transport-outbox") || strings.Contains(path, "worker-queue-claims") || (strings.TrimSpace(action.Kind) == "worker-queue-claim-artifact" && (strings.TrimSpace(action.ControlAction) == "cancel" || strings.TrimSpace(action.ControlAction) == "expire" || strings.TrimSpace(action.ControlAction) == "renew")) {
			tabs = tabs.PrependRowsPreservingAction(targetTab, msg.Result.RuntimeRows)
		} else {
			tabs = tabs.AppendRowsPreservingAction(targetTab, msg.Result.RuntimeRows)
		}
		if strings.TrimSpace(action.Kind) == "compile-preview-review-artifact" || strings.TrimSpace(action.Kind) == "compile-preview-review-selection" {
			reviewPath := strings.TrimSpace(action.ReviewPath)
			if reviewPath == "" {
				reviewPath = strings.TrimSpace(action.ArtifactPath)
			}
			tabs = tabs.SelectFirstRowMatching(targetTab, "compile-preview review selection:", reviewPath)
		}
		if strings.TrimSpace(action.Kind) == "replay-workbench-session-artifact" || strings.TrimSpace(action.Kind) == "replay-workbench-session-selection" {
			tabs = tabs.SelectFirstRowMatching(targetTab, "replay workbench session selection:", strings.TrimSpace(action.ArtifactPath))
		}
		if strings.TrimSpace(action.Kind) == "replay-workbench-collaboration-artifact" || strings.TrimSpace(action.Kind) == "replay-workbench-collaboration-selection" {
			tabs = tabs.SelectFirstRowMatching(targetTab, "replay workbench collaboration selection:", strings.TrimSpace(action.ArtifactPath))
		}
		if strings.TrimSpace(action.Kind) == "replay-workbench-collaboration-note" || strings.TrimSpace(action.Kind) == "replay-workbench-collaboration-note-selection" {
			tabs = tabs.SelectFirstRowMatching(targetTab, "replay workbench collaboration note selection:", strings.TrimSpace(action.ArtifactPath))
		}
		if strings.TrimSpace(action.Kind) == "report-projection-dispatch-execution-artifact" || strings.TrimSpace(action.Kind) == "report-projection-execution-selection" {
			tabs = tabs.SelectFirstRowMatching(targetTab, "report projection execution selection:", strings.TrimSpace(action.ArtifactPath))
			for _, row := range msg.Result.RuntimeRows {
				if strings.HasPrefix(strings.TrimSpace(row), "report projection execution selection:") {
					refreshed := runtimeReportProjectionExecutionSelectionActionForRow(strings.TrimSpace(row))
					refreshed.ExecutionStatus = action.ExecutionStatus
					refreshed.ExecutionPath = action.ExecutionPath
					refreshed.Output = action.Output
					refreshed.ExecutionError = action.ExecutionError
					action = refreshed
					break
				}
			}
		}
		if strings.TrimSpace(action.Kind) == "worker-lifecycle-action-plan-artifact" || strings.TrimSpace(action.Kind) == "worker-lifecycle-action-plan-selection" {
			tabs = tabs.SelectFirstRowMatching(targetTab, "worker lifecycle action plan selection:", strings.TrimSpace(action.ArtifactPath))
			for _, row := range msg.Result.RuntimeRows {
				if strings.HasPrefix(strings.TrimSpace(row), "worker lifecycle action plan selection:") {
					refreshed := runtimeWorkerLifecycleActionPlanSelectionActionForRow(strings.TrimSpace(row))
					refreshed.ExecutionStatus = action.ExecutionStatus
					refreshed.ExecutionPath = action.ExecutionPath
					refreshed.Output = action.Output
					refreshed.ExecutionError = action.ExecutionError
					action = refreshed
					break
				}
			}
		}
		if strings.TrimSpace(action.Kind) == "worker-lifecycle-policy-artifact" || strings.TrimSpace(action.Kind) == "worker-lifecycle-policy-selection" || strings.TrimSpace(action.Kind) == "worker-lifecycle-policy-scheduler-run-artifact" {
			tabs = tabs.SelectFirstRowMatching(targetTab, "worker lifecycle policy selection:", strings.TrimSpace(action.ArtifactPath))
			for _, row := range msg.Result.RuntimeRows {
				if strings.HasPrefix(strings.TrimSpace(row), "worker lifecycle policy selection:") {
					refreshed := runtimeWorkerLifecyclePolicySelectionActionForRow(strings.TrimSpace(row))
					refreshed.ExecutionStatus = action.ExecutionStatus
					refreshed.ExecutionPath = action.ExecutionPath
					refreshed.Output = action.Output
					refreshed.ExecutionError = action.ExecutionError
					action = refreshed
					break
				}
			}
		}
		if path != "" {
			prefix := "execution artifact:"
			if strings.TrimSpace(action.Kind) == "install-retry-dispatch-artifact" {
				prefix = "retry execution artifact:"
			}
			if strings.TrimSpace(action.Kind) == "compile-preview" {
				prefix = "compile-preview review artifact:"
			}
			if strings.TrimSpace(action.Kind) == "worker-lease-reclaim-artifact" {
				prefix = "worker lease reclaim execution artifact:"
			}
			if strings.TrimSpace(action.Kind) == "worker-lease-result-reconciliation-artifact" {
				prefix = "worker lease result reconciliation execution artifact:"
			}
			if strings.TrimSpace(action.Kind) == "worker-placement-artifact" {
				prefix = "worker placement claim execution artifact:"
			}
			if strings.TrimSpace(action.Kind) == "worker-placement-dispatch-artifact" {
				prefix = "worker placement dispatch execution artifact:"
				if strings.Contains(path, "worker-placement-dispatch-batches") {
					prefix = "worker placement dispatch batch artifact:"
				}
			}
			if strings.TrimSpace(action.Kind) == "worker-queue-claim-artifact" {
				prefix = "worker queue dispatch execution artifact:"
				if strings.TrimSpace(action.ControlAction) == "cancel" || strings.TrimSpace(action.ControlAction) == "expire" || strings.TrimSpace(action.ControlAction) == "renew" {
					prefix = "worker queue claim artifact:"
				}
			}
			if strings.TrimSpace(action.Kind) == "worker-transport-request-artifact" {
				prefix = "worker transport request:"
			}
			if strings.TrimSpace(action.Kind) == "worker-transport-daemon-artifact" {
				prefix = "worker transport daemon control artifact:"
			}
			if strings.TrimSpace(action.Kind) == "report-projection-dispatch-artifact" {
				prefix = "report projection dispatch execution artifact:"
			}
			tabs = tabs.SelectFirstRowMatching(targetTab, prefix, path)
		}
	}
	tabs = tabs.WithActiveRuntimeAction(action)
	stack.RuntimeTabs = tabs
	if stack.Focus == GraphFocusRuntimeAction && stack.HasRuntimeAction {
		stack.RuntimeAction = action
	}
	m.graphStack = stack
	return m
}

func compilePreviewReviewRuntimeRowPrefixes() []string {
	return []string{
		"compile-preview reviews:",
		"compile-preview review selection:",
		"compile-preview review superseded:",
		"compile-preview review artifact:",
		"compile-preview review warning:",
		"compile-preview review node:",
		"compile-preview review comparison:",
		"compile-preview review comparison node:",
		"compile-preview review comparison warning:",
	}
}

func reportProjectionDispatchExecutionRuntimeRowPrefixes() []string {
	return []string{
		"report projection dispatch executions:",
		"report projection execution selection:",
		"report projection execution superseded:",
		"report projection dispatch execution artifact:",
		"report projection dispatch execution compact:",
		"report projection execution review:",
		"report projection dispatch execution annotation:",
		"report projection dispatch execution path:",
	}
}

func replayWorkbenchSessionRuntimeRowPrefixes() []string {
	return []string{
		"replay workbench sessions:",
		"replay workbench session selection:",
		"replay workbench session superseded:",
		"replay workbench session artifact:",
		"replay workbench session annotation:",
		"replay workbench session reason:",
		"replay workbench session filter:",
		"replay workbench session endpoint:",
	}
}

func replayWorkbenchCollaborationRuntimeRowPrefixes() []string {
	return []string{
		"replay workbench collaborations:",
		"replay workbench collaboration selection:",
		"replay workbench collaboration selection actor:",
		"replay workbench collaboration selection reason:",
		"replay workbench collaboration current marker:",
		"replay workbench collaboration noncurrent marker:",
		"replay workbench collaboration superseded:",
		"replay workbench collaboration artifact:",
		"replay workbench collaboration current:",
		"replay workbench collaboration reason:",
		"replay workbench collaboration participant:",
		"replay workbench collaboration endpoint:",
	}
}

func replayWorkbenchCollaborationNoteRuntimeRowPrefixes() []string {
	return []string{
		"replay workbench collaboration notes:",
		"replay workbench collaboration note selection:",
		"replay workbench collaboration note selection actor:",
		"replay workbench collaboration note selection reason:",
		"replay workbench collaboration note current marker:",
		"replay workbench collaboration note noncurrent marker:",
		"replay workbench collaboration note superseded:",
		"replay workbench collaboration note artifact:",
		"replay workbench collaboration note compact:",
		"replay workbench collaboration note review:",
		"replay workbench collaboration note author:",
		"replay workbench collaboration note body:",
	}
}

func workerLifecycleActionPlanRuntimeRowPrefixes() []string {
	return []string{
		"worker lifecycle action plan artifacts:",
		"worker lifecycle action plan selection:",
		"worker lifecycle action plan superseded:",
		"worker lifecycle action plan artifact status=",
		"worker lifecycle action plan artifact:",
		"worker lifecycle action plan artifact state:",
		"worker lifecycle action plan artifact review:",
		"worker lifecycle action plan artifact counts:",
		"worker lifecycle action plan daemon controls:",
		"worker lifecycle action plan retry dispatch executions:",
		"worker lifecycle action plan review:",
		"worker lifecycle action plan annotation:",
		"worker lifecycle action plan artifact path:",
		"worker lifecycle action:",
		"daemon control action:",
		"retry dispatch action:",
	}
}

func workerLifecyclePolicyRuntimeRowPrefixes() []string {
	return []string{
		"worker lifecycle policy artifacts:",
		"worker lifecycle policy selection:",
		"worker lifecycle policy selection cadence:",
		"worker lifecycle policy selection actor:",
		"worker lifecycle policy selection clear:",
		"worker lifecycle policy selected:",
		"worker lifecycle policy selection path:",
		"worker lifecycle policy superseded:",
		"worker lifecycle policy artifact:",
		"worker lifecycle policy artifact current:",
		"worker lifecycle policy artifact state:",
		"worker lifecycle policy artifact cadence:",
		"worker lifecycle policy artifact gates:",
		"worker lifecycle policy artifact summary:",
		"worker lifecycle policy artifact path:",
		"worker lifecycle policy runs:",
		"worker lifecycle policy run artifact:",
		"worker lifecycle policy run cadence:",
		"worker lifecycle policy run gates:",
		"worker lifecycle policy run refs:",
		"worker lifecycle policy run skip:",
		"worker lifecycle policy run path:",
	}
}

func (m Model) ShowRuntimeActionDetails() Model {
	stack := m.graphStack
	tabs := stack.RuntimeTabs
	if len(tabs.Tabs) == 0 {
		tabs = DefaultRuntimeTabs()
	}
	var action RuntimeAction
	var ok bool
	selectedTabs := tabs.ActivateSelectedRow()
	if selectedAction, selectedOK := selectedTabs.ActiveRuntimeAction(); selectedOK {
		tabs = selectedTabs
		action = selectedAction
		ok = true
	} else {
		action, ok = tabs.ActiveRuntimeAction()
	}
	stack.RuntimeTabs = tabs
	if ok {
		stack = stack.WithRuntimeAction(action)
	}
	m.graphStack = stack
	return m
}

func (m Model) View() string {
	if !m.layout.Ready {
		message := fitWidth(m.layout.Message, m.layout.Width)
		return strings.Join([]string{
			message,
			RenderStatusBar(m.layout.Width, m.status),
		}, "\n")
	}

	content := lipgloss.JoinHorizontal(
		lipgloss.Top,
		m.workspacePanel.Render(m.layout.Workspace.Width, m.layout.ContentHeight),
		m.renderCenterPanel(),
		m.graphStack.Render(m.layout.GraphStack.Width, m.layout.ContentHeight),
	)

	return strings.Join([]string{
		content,
		RenderStatusBar(m.layout.Width, m.status),
	}, "\n")
}

func (m Model) renderCenterPanel() string {
	if m.agentWorkbench != nil {
		return m.agentWorkbench.Render(m.layout.Workbench.Width, m.layout.ContentHeight)
	}
	return m.conversation.Render(m.layout.Workbench.Width, m.layout.ContentHeight)
}

func (m *Model) resizeViewports() {
	m.workspace = viewport.New(maxInt(0, m.layout.Workspace.Width), maxInt(0, m.layout.Workspace.Height))
	m.workbench = viewport.New(maxInt(0, m.layout.Workbench.Width), maxInt(0, m.layout.Workbench.Height))
	m.graphs = viewport.New(maxInt(0, m.layout.GraphStack.Width), maxInt(0, m.layout.GraphStack.Height))
}

func renderPanel(title string, rows []string, width int, height int) string {
	if width <= 0 || height <= 0 {
		return ""
	}
	lines := make([]string, 0, height)
	lines = append(lines, fitWidth(title, width))
	for _, row := range rows {
		if len(lines) >= height {
			break
		}
		lines = append(lines, fitWidth(row, width))
	}
	for len(lines) < height {
		lines = append(lines, strings.Repeat(" ", width))
	}
	return strings.Join(lines, "\n")
}
