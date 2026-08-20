package tui

import (
	"fmt"
	"strings"

	"github.com/ouroboros/ouroboros/internal/evidence"
)

type GraphFocus string

const (
	GraphFocusAttack        GraphFocus = "attack"
	GraphFocusAgent         GraphFocus = "agent"
	GraphFocusInspector     GraphFocus = "inspector"
	GraphFocusRuntimeAction GraphFocus = "runtime-action"
)

type GraphStack struct {
	Attack           AttackGraph
	Agents           AgentGraph
	RuntimeTabs      RuntimeTabs
	Focus            GraphFocus
	RuntimeAction    RuntimeAction
	HasRuntimeAction bool
}

type Inspector struct {
	Rows []string
}

func DefaultGraphStack() GraphStack {
	return GraphStack{
		RuntimeTabs: DefaultRuntimeTabs(),
		Focus:       GraphFocusAttack,
	}
}

func (s GraphStack) FocusPane(focus GraphFocus) GraphStack {
	if focus == "" {
		focus = GraphFocusAttack
	}
	s.Focus = focus
	return s
}

func (s GraphStack) SelectAttackNode(nodeID string) GraphStack {
	s.Attack = s.Attack.SelectNode(nodeID)
	s.Focus = GraphFocusAttack
	s.HasRuntimeAction = false
	return s
}

func (s GraphStack) SelectAgent(agentID string) GraphStack {
	s.Agents = s.Agents.SelectAgent(agentID)
	s.Focus = GraphFocusAgent
	s.HasRuntimeAction = false
	return s
}

func (s GraphStack) WithRuntimeAction(action RuntimeAction) GraphStack {
	s.RuntimeAction = action
	s.HasRuntimeAction = true
	s.Focus = GraphFocusRuntimeAction
	if len(s.RuntimeTabs.Tabs) == 0 {
		s.RuntimeTabs = DefaultRuntimeTabs()
	}
	s.RuntimeTabs = s.RuntimeTabs.Select(RuntimeTabNodeInspector)
	return s
}

func (s GraphStack) Render(width int, height int) string {
	if width <= 0 || height <= 0 {
		return ""
	}
	attackHeight, agentHeight, inspectorHeight := graphSectionHeights(height)
	tabs := s.RuntimeTabs
	if len(tabs.Tabs) == 0 {
		tabs = DefaultRuntimeTabs()
	}
	tabs = tabs.WithRows(RuntimeTabNodeInspector, s.Inspector().Rows)
	return joinVertical(
		s.Attack.Render(width, attackHeight),
		s.Agents.Render(width, agentHeight),
		tabs.Render(width, inspectorHeight),
	)
}

func (s GraphStack) Inspector() Inspector {
	switch s.Focus {
	case GraphFocusRuntimeAction:
		if s.HasRuntimeAction {
			return inspectorForRuntimeAction(s.RuntimeAction)
		}
	case GraphFocusAgent:
		if agent, ok := s.Agents.SelectedAgent(); ok {
			return Inspector{Rows: []string{
				"selected: " + agent.AgentID,
				"role: " + valueOrDefault(string(agent.Role), "unknown"),
				"state: " + valueOrDefault(string(agent.State), "unknown"),
				fmt.Sprintf("progress: %d%%", agent.ProgressPercent),
			}}
		}
	default:
		if node, ok := s.Attack.SelectedNode(); ok {
			return inspectorForEvidenceNode(node)
		}
	}
	return Inspector{Rows: []string{"selected: none"}}
}

func inspectorForRuntimeAction(action RuntimeAction) Inspector {
	if strings.TrimSpace(action.Kind) == "worker-lifecycle-action-plan-selection" {
		return inspectorForWorkerLifecycleActionPlanSelection(action)
	}
	if strings.TrimSpace(action.Kind) == "worker-lifecycle-policy-selection" {
		return inspectorForWorkerLifecyclePolicySelection(action)
	}
	isWorkerLifecycleAction := action.Kind == "worker-lifecycle-action"
	isWorkerLifecycleAuditItem := action.Kind == "worker-lifecycle-audit-item"
	rows := []string{
		"runtime action: " + valueOrDefault(action.Kind, "unknown"),
	}
	if action.Name != "" && action.Kind != "worker-lifecycle-action" && action.Kind != "worker-lifecycle-audit-item" {
		rows = append(rows, "name: "+action.Name)
	}
	if action.Pack != "" {
		rows = append(rows, "pack: "+action.Pack)
	}
	if action.State != "" {
		rows = append(rows, "state: "+action.State)
	}
	if action.Kind == "worker-lifecycle-policy-selection" && action.State != "" {
		rows = append(rows, "selection status: "+action.State)
	}
	if action.ExecutionMode != "" {
		rows = append(rows, "execution mode: "+action.ExecutionMode)
	}
	if action.Command != "" {
		rows = append(rows, "command: "+action.Command)
	}
	if action.Output != "" {
		rows = append(rows, "output: "+action.Output)
	}
	if action.Hint != "" {
		rows = append(rows, "hint: "+action.Hint)
		if action.Kind == "managed-bundle" || action.Kind == "release-managed-status" {
			rows = append(rows, "action hint: "+action.Hint)
		}
	}
	if action.Source != "" {
		rows = append(rows, "source: "+action.Source)
	}
	if action.Target != "" {
		rows = append(rows, "target: "+action.Target)
	}
	if action.Author != "" {
		rows = append(rows, "author: "+action.Author)
	}
	if action.TargetKind != "" {
		rows = append(rows, "target kind: "+action.TargetKind)
	}
	if action.TargetID != "" {
		rows = append(rows, "target id: "+action.TargetID)
	}
	if action.Body != "" {
		rows = append(rows, "body: "+action.Body)
	}
	if action.Score != "" {
		rows = append(rows, "score: "+action.Score)
	}
	if action.Weight != "" {
		rows = append(rows, "weight: "+action.Weight)
	}
	if action.WorkerID != "" && !isWorkerLifecycleAction && !isWorkerLifecycleAuditItem {
		rows = append(rows, "worker: "+action.WorkerID)
	}
	if action.RunID != "" && !isWorkerLifecycleAction && !isWorkerLifecycleAuditItem {
		rows = append(rows, "run: "+action.RunID)
	}
	if action.SelectedRunID != "" {
		rows = append(rows, "selected run: "+action.SelectedRunID)
	}
	if action.SessionPath != "" {
		rows = append(rows, "session path: "+action.SessionPath)
	}
	if action.RunIDs != "" {
		rows = append(rows, "run ids: "+action.RunIDs)
	}
	if action.ExecutionID != "" && !isWorkerLifecycleAction && !isWorkerLifecycleAuditItem {
		rows = append(rows, "execution: "+action.ExecutionID)
	}
	if action.Kind == "worker-lifecycle-action" {
		if action.Name != "" {
			rows = append(rows, "action type: "+action.Name)
		}
		if action.Gate != "" {
			rows = append(rows, "action gate: "+action.Gate)
		}
		if action.WorkerID != "" {
			rows = append(rows, "worker: "+action.WorkerID)
		}
		if action.RunID != "" {
			rows = append(rows, "run: "+action.RunID)
		}
		if action.ExecutionID != "" {
			rows = append(rows, "execution: "+action.ExecutionID)
		}
		if action.Reason != "" {
			rows = append(rows, "reason: "+action.Reason)
		}
		if action.RecommendedStatus != "" {
			rows = append(rows, "recommended status: "+action.RecommendedStatus)
		}
		if action.RequestPath != "" {
			rows = append(rows, "request path: "+action.RequestPath)
		}
		if action.LeasePath != "" {
			rows = append(rows, "lease path: "+action.LeasePath)
		}
		if action.ResultPath != "" {
			rows = append(rows, "result path: "+action.ResultPath)
		}
		if action.DispatchPath != "" {
			rows = append(rows, "dispatch path: "+action.DispatchPath)
		}
	}
	if action.Kind == "worker-lifecycle-audit-item" {
		if action.Name != "" {
			rows = append(rows, "audit item kind: "+action.Name)
		}
		if action.Gate != "" {
			rows = append(rows, "action gate: "+action.Gate)
		}
		if action.WorkerID != "" {
			rows = append(rows, "worker: "+action.WorkerID)
		}
		if action.RunID != "" {
			rows = append(rows, "run: "+action.RunID)
		}
		if action.ExecutionID != "" {
			rows = append(rows, "execution: "+action.ExecutionID)
		}
		if action.Reason != "" {
			rows = append(rows, "reason: "+action.Reason)
		}
	}
	if action.Reason != "" && !isWorkerLifecycleAction && !isWorkerLifecycleAuditItem {
		rows = append(rows, "reason: "+action.Reason)
	}
	if action.TransportMode != "" {
		rows = append(rows, "transport: "+action.TransportMode)
	}
	if action.Digest != "" {
		rows = append(rows, "digest: "+action.Digest)
	}
	if action.Kind == "worker-lifecycle-policy-selection" && action.ArtifactPath != "" {
		rows = append(rows, "policy path: "+action.ArtifactPath)
	}
	if action.ArtifactPath != "" {
		rows = append(rows, "artifact path: "+action.ArtifactPath)
	}
	if action.ArchivePath != "" {
		rows = append(rows, "archive path: "+action.ArchivePath)
	}
	if action.ReleaseBundlePath != "" {
		rows = append(rows, "release bundle path: "+action.ReleaseBundlePath)
	}
	if action.BasePath != "" {
		rows = append(rows, "base path: "+action.BasePath)
	}
	if action.CandidatePath != "" {
		rows = append(rows, "candidate path: "+action.CandidatePath)
	}
	if action.ReviewPath != "" {
		rows = append(rows, "review path: "+action.ReviewPath)
	}
	if action.DispatchPath != "" && !isWorkerLifecycleAction {
		rows = append(rows, "dispatch path: "+action.DispatchPath)
	}
	if action.RetryDispatchPath != "" {
		rows = append(rows, "retry dispatch path: "+action.RetryDispatchPath)
	}
	if action.ExecutionPath != "" {
		rows = append(rows, "execution path: "+action.ExecutionPath)
	}
	if action.RequestPath != "" && !isWorkerLifecycleAction && !isWorkerLifecycleAuditItem {
		rows = append(rows, "request path: "+action.RequestPath)
	}
	if (action.Kind == "worker-lifecycle-policy-selection" || action.Kind == "report-projection-execution-selection") && action.RequestPath != "" {
		rows = append(rows, "selection path: "+action.RequestPath)
	}
	if action.SourceExecutionPath != "" {
		rows = append(rows, "source execution path: "+action.SourceExecutionPath)
	}
	if action.SourceResultPath != "" {
		rows = append(rows, "source result path: "+action.SourceResultPath)
	}
	if action.SourceClaimPath != "" {
		rows = append(rows, "claim path: "+action.SourceClaimPath)
	}
	if action.LeasePath != "" && !isWorkerLifecycleAction {
		rows = append(rows, "lease path: "+action.LeasePath)
	}
	if action.PlannedReportPath != "" {
		rows = append(rows, "planned report path: "+action.PlannedReportPath)
	} else if action.ResultPath != "" && !isWorkerLifecycleAction {
		rows = append(rows, "result path: "+action.ResultPath)
	}
	if action.PlacementPlanPath != "" {
		rows = append(rows, "placement plan path: "+action.PlacementPlanPath)
	}
	if action.PlacementDispatchPlanPath != "" {
		rows = append(rows, "placement dispatch plan path: "+action.PlacementDispatchPlanPath)
	}
	if action.PlacementDispatchBatchPath != "" {
		rows = append(rows, "placement dispatch batch path: "+action.PlacementDispatchBatchPath)
	}
	if action.LifecycleMaintenancePath != "" {
		rows = append(rows, "lifecycle maintenance path: "+action.LifecycleMaintenancePath)
	}
	if action.LifecycleAuditPath != "" {
		rows = append(rows, "lifecycle audit path: "+action.LifecycleAuditPath)
	}
	if action.Ready != "" {
		rows = append(rows, "ready: "+action.Ready)
	}
	if action.Results != "" {
		rows = append(rows, "results: "+action.Results)
	}
	if action.Skipped != "" {
		rows = append(rows, "skipped: "+action.Skipped)
	}
	if action.Claimed != "" {
		rows = append(rows, "claimed: "+action.Claimed)
	}
	if action.Failed != "" {
		rows = append(rows, "failed: "+action.Failed)
	}
	if action.MaxPlanNodes != "" {
		rows = append(rows, "max plan nodes: "+action.MaxPlanNodes)
	}
	if action.DomainPriorities != "" {
		rows = append(rows, "domain priorities: "+action.DomainPriorities)
	}
	if action.FrontierScores != "" {
		rows = append(rows, "frontier scores: "+action.FrontierScores)
	}
	if action.HistoricalStats != "" {
		rows = append(rows, "historical stats: "+action.HistoricalStats)
	}
	if action.Filters != "" {
		rows = append(rows, "filters: "+action.Filters)
	}
	if action.Endpoints != "" {
		rows = append(rows, "endpoints: "+action.Endpoints)
	}
	if action.Participants != "" {
		rows = append(rows, "participants: "+action.Participants)
	}
	if action.Participant != "" {
		rows = append(rows, "participant: "+action.Participant)
	}
	if action.CollaborationPath != "" {
		rows = append(rows, "collaboration path: "+action.CollaborationPath)
	}
	if action.SeenBy != "" {
		rows = append(rows, "seen by: "+action.SeenBy)
	}
	if action.SeenAt != "" {
		rows = append(rows, "seen at: "+action.SeenAt)
	}
	if action.CreatedBy != "" {
		rows = append(rows, "created by: "+action.CreatedBy)
	}
	if action.CreatedAt != "" {
		rows = append(rows, "created at: "+action.CreatedAt)
	}
	if action.Successes != "" {
		rows = append(rows, "successes: "+action.Successes)
	}
	if action.Failures != "" {
		rows = append(rows, "failures: "+action.Failures)
	}
	if action.NodeDelta != "" {
		rows = append(rows, "node delta: "+action.NodeDelta)
	}
	if action.WarningDelta != "" {
		rows = append(rows, "warning delta: "+action.WarningDelta)
	}
	if action.Added != "" {
		rows = append(rows, "added: "+action.Added)
	}
	if action.Removed != "" {
		rows = append(rows, "removed: "+action.Removed)
	}
	if action.Changed != "" {
		rows = append(rows, "changed: "+action.Changed)
	}
	if action.Reordered != "" {
		rows = append(rows, "reordered: "+action.Reordered)
	}
	if action.ScoreChanged != "" {
		rows = append(rows, "score changed: "+action.ScoreChanged)
	}
	if action.SignalChanged != "" {
		rows = append(rows, "signal changed: "+action.SignalChanged)
	}
	if action.Superseded != "" {
		rows = append(rows, "superseded: "+action.Superseded)
	}
	if action.SelectedBy != "" {
		rows = append(rows, "selected by: "+action.SelectedBy)
	}
	if action.SelectedAt != "" {
		rows = append(rows, "selected at: "+action.SelectedAt)
	}
	if action.ClearedBy != "" {
		rows = append(rows, "cleared by: "+action.ClearedBy)
	}
	if action.ClearedAt != "" {
		rows = append(rows, "cleared at: "+action.ClearedAt)
	}
	if action.Decision != "" && action.Decision != "unreviewed" {
		rows = append(rows, "decision: "+action.Decision)
	}
	if action.Reviewer != "" && action.Reviewer != "unknown" {
		rows = append(rows, "reviewer: "+action.Reviewer)
	}
	if action.ReviewedAt != "" && action.ReviewedAt != "unknown" {
		rows = append(rows, "reviewed at: "+action.ReviewedAt)
	}
	if action.ReviewReason != "" && action.ReviewReason != "none" {
		rows = append(rows, "review reason: "+action.ReviewReason)
	}
	if action.RequestStatus != "" {
		rows = append(rows, "request status: "+action.RequestStatus)
	}
	if action.RequestError != "" {
		rows = append(rows, "request error: "+action.RequestError)
	}
	if action.ControlAction != "" {
		rows = append(rows, "control action: "+action.ControlAction)
	}
	if action.ActionType != "" {
		rows = append(rows, "action type: "+action.ActionType)
	}
	if action.Nodes != "" {
		rows = append(rows, "nodes: "+action.Nodes)
	}
	if action.Warnings != "" {
		rows = append(rows, "warnings: "+action.Warnings)
	}
	if action.Current != "" {
		rows = append(rows, "current: "+action.Current)
	}
	if action.ExecutionStatus != "" {
		rows = append(rows, "execution status: "+action.ExecutionStatus)
	}
	if action.ExecutionError != "" {
		rows = append(rows, "execution error: "+action.ExecutionError)
	}
	if runtimeActionIsReadOnly(action) {
		rows = append(rows, "mode: read-only")
	} else {
		rows = append(rows, "mode: metadata-only")
	}
	return Inspector{Rows: rows}
}

func inspectorForWorkerLifecycleActionPlanSelection(action RuntimeAction) Inspector {
	rows := []string{
		"runtime action: " + valueOrDefault(strings.TrimSpace(action.Kind), "unknown"),
	}
	if status := strings.TrimSpace(action.State); status != "" {
		rows = append(rows, "selection status: "+status)
		rows = append(rows, "state: "+status)
	}
	if mode := strings.TrimSpace(action.ExecutionMode); mode != "" {
		rows = append(rows, "execution mode: "+mode)
	}
	if current := strings.TrimSpace(action.Current); current != "" {
		rows = append(rows, "current: "+current)
	}
	if planPath := strings.TrimSpace(action.ArtifactPath); planPath != "" {
		rows = append(rows, "plan path: "+planPath)
	}
	if selectionPath := strings.TrimSpace(action.RequestPath); selectionPath != "" {
		rows = append(rows, "selection path: "+selectionPath)
	}
	if reviewPath := strings.TrimSpace(action.ReviewPath); reviewPath != "" {
		rows = append(rows, "review path: "+reviewPath)
	}
	if ready := strings.TrimSpace(action.Ready); ready != "" {
		rows = append(rows, "ready: "+ready)
	}
	if results := strings.TrimSpace(action.Results); results != "" {
		rows = append(rows, "results: "+results)
	}
	if superseded := strings.TrimSpace(action.Superseded); superseded != "" {
		rows = append(rows, "superseded: "+superseded)
	}
	if selectedBy := strings.TrimSpace(action.SelectedBy); selectedBy != "" {
		rows = append(rows, "selected by: "+selectedBy)
	}
	if selectedAt := strings.TrimSpace(action.SelectedAt); selectedAt != "" {
		rows = append(rows, "selected at: "+selectedAt)
	}
	if clearedBy := strings.TrimSpace(action.ClearedBy); clearedBy != "" {
		rows = append(rows, "cleared by: "+clearedBy)
	}
	if clearedAt := strings.TrimSpace(action.ClearedAt); clearedAt != "" {
		rows = append(rows, "cleared at: "+clearedAt)
	}
	if reason := strings.TrimSpace(action.Reason); reason != "" {
		rows = append(rows, "reason: "+reason)
	}
	if runtimeActionIsReadOnly(action) {
		rows = append(rows, "mode: read-only")
	} else {
		rows = append(rows, "mode: metadata-only")
	}
	return Inspector{Rows: rows}
}

func inspectorForWorkerLifecyclePolicySelection(action RuntimeAction) Inspector {
	rows := []string{
		"runtime action: " + valueOrDefault(strings.TrimSpace(action.Kind), "unknown"),
	}
	if status := strings.TrimSpace(action.State); status != "" {
		rows = append(rows, "selection status: "+status)
	}
	if policyPath := strings.TrimSpace(action.ArtifactPath); policyPath != "" {
		rows = append(rows, "policy path: "+policyPath)
	}
	if selectionPath := strings.TrimSpace(action.RequestPath); selectionPath != "" {
		rows = append(rows, "selection path: "+selectionPath)
	}
	if selectedBy := strings.TrimSpace(action.SelectedBy); selectedBy != "" {
		rows = append(rows, "selected by: "+selectedBy)
	}
	if selectedAt := strings.TrimSpace(action.SelectedAt); selectedAt != "" {
		rows = append(rows, "selected at: "+selectedAt)
	}
	if clearedBy := strings.TrimSpace(action.ClearedBy); clearedBy != "" {
		rows = append(rows, "cleared by: "+clearedBy)
	}
	if clearedAt := strings.TrimSpace(action.ClearedAt); clearedAt != "" {
		rows = append(rows, "cleared at: "+clearedAt)
	}
	if reason := strings.TrimSpace(action.Reason); reason != "" {
		rows = append(rows, "reason: "+reason)
	}
	if superseded := strings.TrimSpace(action.Superseded); superseded != "" {
		rows = append(rows, "superseded: "+superseded)
	}
	if runtimeActionIsReadOnly(action) {
		rows = append(rows, "mode: read-only")
	} else {
		rows = append(rows, "mode: metadata-only")
	}
	if transport := strings.TrimSpace(action.TransportMode); transport != "" {
		rows = append(rows, "transport: "+transport)
	}
	if results := strings.TrimSpace(action.Results); results != "" {
		rows = append(rows, "results: "+results)
	}
	return Inspector{Rows: rows}
}

func inspectorForEvidenceNode(node evidence.Node) Inspector {
	return Inspector{Rows: []string{
		"selected: " + node.ID,
		"type: " + string(node.Type),
		"label: " + node.Label,
		"source: " + valueOrDefault(node.SourceKind, "unknown"),
	}}
}

func (i Inspector) Render(width int, height int) string {
	return renderPanel("Inspector", i.Rows, width, height)
}

func graphSectionHeights(height int) (int, int, int) {
	if height < 3 {
		return height, 0, 0
	}
	attackHeight := maxInt(1, height/4)
	agentHeight := maxInt(1, height/4)
	inspectorHeight := height - attackHeight - agentHeight
	if inspectorHeight < 1 {
		inspectorHeight = 1
	}
	return attackHeight, agentHeight, inspectorHeight
}

func joinVertical(blocks ...string) string {
	parts := make([]string, 0, len(blocks))
	for _, block := range blocks {
		if block == "" {
			continue
		}
		parts = append(parts, block)
	}
	return strings.Join(parts, "\n")
}
