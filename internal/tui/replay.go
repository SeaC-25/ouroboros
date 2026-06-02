package tui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/ouroboros/ouroboros/internal/eventlog"
	"github.com/ouroboros/ouroboros/internal/swarm"
)

type ReplaySnapshotMsg struct {
	Snapshot eventlog.ReplaySnapshot
}

type ReplayTimelineMsg struct {
	Events []eventlog.Event
}

func StatusFromReplay(snapshot eventlog.ReplaySnapshot) Status {
	status := snapshot.Status
	model := valueOrDefault(status.Model, "not-set")
	tools := valueOrDefault(status.ToolHealth, "unknown")
	return Status{
		Model:          model,
		ContextPercent: status.ContextPercent,
		Agents:         status.AgentCount,
		BreadthPercent: status.BreadthPercent,
		DepthChains:    status.DepthChains,
		Tools:          tools,
		Cost:           fmt.Sprintf("$%.2f", status.Cost),
	}
}

func (m Model) WithReplaySnapshot(snapshot eventlog.ReplaySnapshot) Model {
	installRequests := m.status.InstallRequests
	m.status = StatusFromReplay(snapshot)
	m.status.InstallRequests = installRequests

	stack := m.graphStack
	if len(stack.RuntimeTabs.Tabs) == 0 {
		stack.RuntimeTabs = DefaultRuntimeTabs()
	}
	stack.Attack = AttackGraph{
		Nodes: snapshot.Evidence.Nodes(),
		Edges: snapshot.Evidence.Edges(),
	}
	stack.Agents = AgentGraph{Agents: agentGraphNodesFromReplay(snapshot.Agents)}
	agentRows := agentRuntimeRows(snapshot)
	if len(m.runtimeControlRows) > 0 {
		agentRows = append(agentRows, m.runtimeControlRows...)
	}
	stack.RuntimeTabs = stack.RuntimeTabs.
		WithRows(RuntimeTabModelRuntime, modelRuntimeRows(snapshot)).
		WithRows(RuntimeTabAgentRuntime, agentRows).
		WithRows(RuntimeTabToolchainHealth, toolchainHealthRows(snapshot)).
		WithRows(RuntimeTabTimelineLanes, timelineLaneRows(snapshot.Timeline))
	m.graphStack = stack
	return m
}

func (m Model) WithReplayEvents(events []eventlog.Event) Model {
	m.replayEvents = append([]eventlog.Event(nil), events...)
	m.replayCursor = len(m.replayEvents)
	return m.applyReplayCursor()
}

func (m Model) StepReplay(delta int) Model {
	if len(m.replayEvents) == 0 {
		return m
	}
	m.replayCursor += delta
	if m.replayCursor < 0 {
		m.replayCursor = 0
	}
	if m.replayCursor > len(m.replayEvents) {
		m.replayCursor = len(m.replayEvents)
	}
	return m.applyReplayCursor()
}

func (m Model) JumpReplay(cursor int) Model {
	if len(m.replayEvents) == 0 {
		return m
	}
	m.replayCursor = cursor
	return m.applyReplayCursor()
}

func (m Model) ReplayCursor() (int, int) {
	return m.replayCursor, len(m.replayEvents)
}

func (m Model) applyReplayCursor() Model {
	if m.replayCursor < 0 {
		m.replayCursor = 0
	}
	if m.replayCursor > len(m.replayEvents) {
		m.replayCursor = len(m.replayEvents)
	}
	snapshot, err := eventlog.BuildReplaySnapshot(m.replayEvents[:m.replayCursor])
	if err != nil {
		return m
	}
	m = m.WithReplaySnapshot(snapshot)
	stack := m.graphStack
	stack.RuntimeTabs = stack.RuntimeTabs.WithRows(
		RuntimeTabTimelineLanes,
		replayTimelineRows(snapshot, m.replayEvents, m.replayCursor),
	).Select(RuntimeTabTimelineLanes)
	m.graphStack = stack
	return m
}

func replayTimelineRows(snapshot eventlog.ReplaySnapshot, events []eventlog.Event, cursor int) []string {
	rows := timelineLaneRows(snapshot.Timeline)
	total := len(events)
	rows = append(rows, fmt.Sprintf("cursor: %d/%d", cursor, total))
	if cursor > 0 && cursor <= total {
		event := events[cursor-1]
		rows = append(rows, fmt.Sprintf("event: #%d %s lane:%s", event.ID, event.Type, eventlog.ReplayLaneForEventType(event.Type)))
	} else {
		rows = append(rows, "event: start")
	}
	rows = append(rows, replayTimelineWindowRows(events, cursor)...)
	rows = append(rows, "keys: left/right home/end")
	return rows
}

func replayTimelineWindowRows(events []eventlog.Event, cursor int) []string {
	total := len(events)
	if cursor < 0 {
		cursor = 0
	}
	if cursor > total {
		cursor = total
	}
	rows := make([]string, 0, 3)
	if cursor > 1 {
		rows = append(rows, "prev: "+replayTimelineEventLabel(events[cursor-2]))
	} else {
		rows = append(rows, "prev: start")
	}
	if cursor > 0 {
		rows = append(rows, "current: "+replayTimelineEventLabel(events[cursor-1]))
	} else {
		rows = append(rows, "current: start")
	}
	if cursor < total {
		rows = append(rows, "next: "+replayTimelineEventLabel(events[cursor]))
	} else {
		rows = append(rows, "next: end")
	}
	return rows
}

func replayTimelineEventLabel(event eventlog.Event) string {
	return fmt.Sprintf("#%d %s lane:%s", event.ID, event.Type, eventlog.ReplayLaneForEventType(event.Type))
}

func agentGraphNodesFromReplay(agents []eventlog.ReplayAgent) []AgentGraphNode {
	nodes := make([]AgentGraphNode, 0, len(agents))
	for _, agent := range agents {
		nodes = append(nodes, AgentGraphNode{
			AgentID:         agent.ID,
			Role:            swarm.AgentRole(agent.Role),
			State:           swarm.AgentState(strings.ToLower(agent.State)),
			ProgressPercent: agent.ProgressPercent,
		})
	}
	return nodes
}

func modelRuntimeRows(snapshot eventlog.ReplaySnapshot) []string {
	return []string{
		"model: " + valueOrDefault(snapshot.Status.Model, "not-set"),
		fmt.Sprintf("tokens: %d in / %d out", snapshot.ModelUsage.InputTokens, snapshot.ModelUsage.OutputTokens),
		fmt.Sprintf("cache: %d read / %d write", snapshot.ModelUsage.CacheReadTokens, snapshot.ModelUsage.CacheWriteTokens),
		fmt.Sprintf("cost: $%.2f", snapshot.ModelUsage.Cost),
		fmt.Sprintf("failures: %d", snapshot.ModelUsage.FailureCount),
	}
}

func agentRuntimeRows(snapshot eventlog.ReplaySnapshot) []string {
	rows := []string{fmt.Sprintf("agents: %d", len(snapshot.Agents))}
	agents := append([]eventlog.ReplayAgent(nil), snapshot.Agents...)
	sort.SliceStable(agents, func(i int, j int) bool {
		return agents[i].ID < agents[j].ID
	})
	for _, agent := range agents {
		rows = append(rows, fmt.Sprintf("%s %s %s %d%%", agent.ID, valueOrDefault(agent.Role, "unknown"), valueOrDefault(agent.State, "unknown"), agent.ProgressPercent))
	}
	if len(snapshot.ToolExecutions) > 0 {
		queued, waiting, active, completed, failed, skipped := toolExecutionCounts(snapshot.ToolExecutions)
		rows = append(rows, fmt.Sprintf("tool workers: queued=%d waiting=%d active=%d completed=%d failed=%d skipped=%d", queued, waiting, active, completed, failed, skipped))
		for _, execution := range snapshot.ToolExecutions {
			status := strings.TrimSpace(execution.Status)
			if status != "queued" && status != "waiting" && status != "started" && status != "running" {
				continue
			}
			row := fmt.Sprintf("worker %s %s %s %s", execution.ExecutionID, valueOrDefault(execution.Tool, "unknown"), valueOrDefault(execution.Capability, "unknown"), execution.Status)
			if status == "waiting" && strings.TrimSpace(execution.WaitReason) != "" {
				row += " reason=" + strings.TrimSpace(execution.WaitReason)
			}
			rows = append(rows, row)
		}
	}
	if len(snapshot.ToolAdmissions) > 0 {
		admitted, skipped := toolAdmissionCounts(snapshot.ToolAdmissions)
		rows = append(rows, fmt.Sprintf("tool admissions: admitted=%d skipped=%d", admitted, skipped))
		for _, admission := range snapshot.ToolAdmissions {
			row := fmt.Sprintf("admission %s %s %s policy=%s",
				valueOrDefault(admission.Tool, "unknown"),
				valueOrDefault(admission.Capability, "unknown"),
				valueOrDefault(admission.Decision, "unknown"),
				valueOrDefault(admission.ExecutionPolicy, "default"),
			)
			if strings.TrimSpace(admission.WorkerID) != "" {
				row += " worker=" + strings.TrimSpace(admission.WorkerID)
			}
			if admission.WorkerPoolMaxConcurrency > 0 {
				row += fmt.Sprintf(" worker_max_concurrency=%d", admission.WorkerPoolMaxConcurrency)
			}
			if admission.PackMaxConcurrency > 0 {
				row += fmt.Sprintf(" max_concurrency=%d", admission.PackMaxConcurrency)
			}
			if admission.ResourceCPUUnits > 0 || admission.ResourceLimitCPUUnits > 0 {
				row += fmt.Sprintf(" cpu=%d/%d", admission.ResourceCPUUnits, admission.ResourceLimitCPUUnits)
			}
			if admission.ResourceMemoryMB > 0 || admission.ResourceLimitMemoryMB > 0 {
				row += fmt.Sprintf(" memory_mb=%d/%d", admission.ResourceMemoryMB, admission.ResourceLimitMemoryMB)
			}
			if admission.ContainerOverrides {
				row += " container_override=true"
			}
			if strings.TrimSpace(admission.PolicyReason) != "" {
				row += " reason=" + strings.TrimSpace(admission.PolicyReason)
			}
			rows = append(rows, row)
		}
	}
	return rows
}

func toolAdmissionCounts(admissions []eventlog.ReplayToolAdmission) (admitted int, skipped int) {
	for _, admission := range admissions {
		switch strings.TrimSpace(admission.Decision) {
		case "admitted":
			admitted++
		case "skipped":
			skipped++
		}
	}
	return admitted, skipped
}

func toolExecutionCounts(executions []eventlog.ReplayToolExecution) (queued int, waiting int, active int, completed int, failed int, skipped int) {
	for _, execution := range executions {
		switch strings.TrimSpace(execution.Status) {
		case "queued":
			queued++
		case "waiting":
			waiting++
		case "started", "running":
			active++
		case "completed":
			completed++
		case "failed":
			failed++
		case "skipped":
			skipped++
		}
	}
	return queued, waiting, active, completed, failed, skipped
}

func toolchainHealthRows(snapshot eventlog.ReplaySnapshot) []string {
	rows := []string{"toolchain: " + valueOrDefault(snapshot.Status.ToolHealth, "unknown")}
	for _, status := range snapshot.Toolchain {
		health := valueOrDefault(status.Health, "unknown")
		row := fmt.Sprintf("%s: %s", status.Name, health)
		if status.Message != "" {
			row += " " + status.Message
		}
		rows = append(rows, row)
		for _, dependency := range status.Dependencies {
			depRow := fmt.Sprintf("dep %s: %s required=%t", dependency.Name, valueOrDefault(dependency.State, "unknown"), dependency.Required)
			if dependency.Kind != "" {
				depRow += " kind=" + dependency.Kind
			}
			if dependency.Source != "" {
				depRow += " source=" + dependency.Source
			}
			if dependency.ImageDigest != "" {
				depRow += " digest=" + dependency.ImageDigest
			}
			if dependency.LatestImageDigest != "" {
				depRow += " latest_digest=" + dependency.LatestImageDigest
			}
			if dependency.MetadataSource != "" {
				depRow += " metadata=" + dependency.MetadataSource
			}
			if dependency.Message != "" {
				depRow += " " + dependency.Message
			}
			rows = append(rows, depRow)
		}
		for _, step := range status.InstallPlan {
			installRow := fmt.Sprintf("install %s: %s required=%t pack=%s", step.Name, valueOrDefault(step.State, "unknown"), step.Required, status.Name)
			if len(step.Command) > 0 {
				installRow += " command=" + strings.Join(step.Command, " ")
			}
			if step.Hint != "" {
				installRow += " hint=" + step.Hint
			}
			if step.Source != "" {
				installRow += " source=" + step.Source
			}
			if step.ImageDigest != "" {
				installRow += " digest=" + step.ImageDigest
			}
			rows = append(rows, installRow)
		}
	}
	return rows
}

func timelineLaneRows(timeline []eventlog.ReplayTimelineEvent) []string {
	counts := map[eventlog.ReplayLane]int{}
	for _, event := range timeline {
		counts[event.Lane]++
	}
	lanes := []eventlog.ReplayLane{
		eventlog.ReplayLaneMission,
		eventlog.ReplayLaneAgent,
		eventlog.ReplayLaneObserver,
		eventlog.ReplayLaneBlackboard,
		eventlog.ReplayLaneToolchain,
		eventlog.ReplayLaneSubmissionReport,
	}
	rows := make([]string, 0, len(lanes))
	for _, lane := range lanes {
		count := counts[lane]
		rows = append(rows, fmt.Sprintf("%s: %d [%s]", lane, count, timelineLaneBar(count)))
	}
	return rows
}

func timelineLaneBar(count int) string {
	const width = 6
	if count < 0 {
		count = 0
	}
	if count > width {
		count = width
	}
	return strings.Repeat("#", count) + strings.Repeat("-", width-count)
}
