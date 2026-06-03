package tui

import (
	"encoding/base64"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

type RuntimeTab string

const (
	RuntimeTabNodeInspector   RuntimeTab = "Node Inspector"
	RuntimeTabModelRuntime    RuntimeTab = "Model Runtime"
	RuntimeTabAgentRuntime    RuntimeTab = "Agent Runtime"
	RuntimeTabToolchainHealth RuntimeTab = "Toolchain Health"
	RuntimeTabCoverageMatrix  RuntimeTab = "Coverage Matrix"
	RuntimeTabFrontierQueue   RuntimeTab = "Frontier Queue"
	RuntimeTabTimelineLanes   RuntimeTab = "Timeline Lanes"
)

type RuntimeTabs struct {
	Tabs     []RuntimeTab
	Active   RuntimeTab
	Panels   map[RuntimeTab][]string
	Selected map[RuntimeTab]int
	Actions  map[RuntimeTab]RuntimeAction
}

type RuntimeAction struct {
	Kind                       string
	Pack                       string
	Name                       string
	State                      string
	ExecutionMode              string
	Command                    string
	Output                     string
	Hint                       string
	Source                     string
	Target                     string
	Author                     string
	Body                       string
	TargetKind                 string
	TargetID                   string
	Score                      string
	Weight                     string
	Gate                       string
	WorkerID                   string
	RunID                      string
	SelectedRunID              string
	SessionPath                string
	RunIDs                     string
	ExecutionID                string
	Reason                     string
	RecommendedStatus          string
	LeasePath                  string
	ResultPath                 string
	PlannedReportPath          string
	TransportMode              string
	Digest                     string
	DependsOn                  string
	ArtifactPath               string
	DispatchPath               string
	RetryDispatchPath          string
	SourceExecutionPath        string
	SourceResultPath           string
	SourceClaimPath            string
	PlacementPlanPath          string
	PlacementDispatchPlanPath  string
	PlacementDispatchBatchPath string
	LifecycleMaintenancePath   string
	LifecycleAuditPath         string
	Ready                      string
	Results                    string
	Skipped                    string
	Claimed                    string
	Failed                     string
	ExecutionStatus            string
	ExecutionPath              string
	ExecutionError             string
	Row                        string
	RequestStatus              string
	RequestPath                string
	RequestError               string
	ControlAction              string
	ActionType                 string
	Nodes                      string
	Warnings                   string
	MaxPlanNodes               string
	DomainPriorities           string
	FrontierScores             string
	HistoricalStats            string
	Filters                    string
	Endpoints                  string
	Participants               string
	Participant                string
	CollaborationPath          string
	SeenBy                     string
	SeenAt                     string
	CreatedBy                  string
	CreatedAt                  string
	Successes                  string
	Failures                   string
	Decision                   string
	Reviewer                   string
	ReviewedAt                 string
	ReviewReason               string
	Current                    string
	BasePath                   string
	CandidatePath              string
	ReviewPath                 string
	SelectedBy                 string
	SelectedAt                 string
	ClearedBy                  string
	ClearedAt                  string
	Superseded                 string
	NodeDelta                  string
	WarningDelta               string
	Added                      string
	Removed                    string
	Changed                    string
	Reordered                  string
	ScoreChanged               string
	SignalChanged              string
}

type RuntimeActionRequestResult struct {
	Status string
	Path   string
}

func (a RuntimeAction) FeedbackRow() string {
	switch strings.TrimSpace(a.Kind) {
	case "install":
		name := strings.TrimSpace(a.Name)
		if name == "" {
			return "action: remediation selected " + strings.TrimSpace(a.Row)
		}
		return "action: remediation selected install " + name
	case "install-execution-artifact":
		return "action: install execution artifact selected " + strings.TrimSpace(a.ArtifactPath)
	case "install-execution-result":
		name := strings.TrimSpace(a.Name)
		if strings.TrimSpace(a.Pack) != "" && name != "" {
			name = strings.TrimSpace(a.Pack) + "/" + name
		}
		if name == "" {
			name = strings.TrimSpace(a.Row)
		}
		return "action: install execution result selected " + name
	case "install-dispatch-artifact":
		return "action: install dispatch artifact selected " + strings.TrimSpace(a.ArtifactPath)
	case "install-retry-dispatch-artifact":
		return "action: install retry dispatch artifact selected " + strings.TrimSpace(a.ArtifactPath)
	case "install-retry-execution-artifact":
		return "action: install retry execution artifact selected " + strings.TrimSpace(a.ArtifactPath)
	case "install-retry-execution-result":
		name := strings.TrimSpace(a.Name)
		if strings.TrimSpace(a.Pack) != "" && name != "" {
			name = strings.TrimSpace(a.Pack) + "/" + name
		}
		if name == "" {
			name = strings.TrimSpace(a.Row)
		}
		return "action: install retry execution result selected " + name
	case "provider-usage-summary":
		return "action: provider usage summary selected read-only"
	case "install-orchestration-artifact":
		return "action: install orchestration artifact selected " + strings.TrimSpace(a.ArtifactPath)
	case "worker-orchestration-supervisor-artifact":
		return "action: worker orchestration supervisor artifact selected " + strings.TrimSpace(a.ArtifactPath)
	case "worker-orchestration-artifact":
		return "action: worker orchestration artifact selected " + strings.TrimSpace(a.ArtifactPath)
	case "worker-lifecycle-audit-artifact":
		return "action: worker lifecycle audit artifact selected " + strings.TrimSpace(a.ArtifactPath)
	case "worker-lifecycle-audit-item":
		name := strings.TrimSpace(a.Name)
		if name == "" {
			name = strings.TrimSpace(a.Row)
		}
		return "action: worker lifecycle audit item selected " + name
	case "worker-lifecycle-review-artifact":
		return "action: worker lifecycle review artifact selected " + strings.TrimSpace(a.ArtifactPath)
	case "worker-lifecycle-action-plan-artifact":
		return "action: worker lifecycle action plan artifact selected " + strings.TrimSpace(a.ArtifactPath)
	case "worker-lifecycle-maintenance-artifact":
		return "action: worker lifecycle maintenance artifact selected " + strings.TrimSpace(a.ArtifactPath)
	case "worker-lifecycle-maintenance-supervisor-artifact":
		return "action: worker lifecycle maintenance supervisor artifact selected " + strings.TrimSpace(a.ArtifactPath)
	case "worker-lifecycle-policy-artifact":
		return "action: worker lifecycle policy artifact selected " + strings.TrimSpace(a.ArtifactPath)
	case "worker-lifecycle-policy-run-artifact":
		return "action: worker lifecycle policy run artifact selected " + strings.TrimSpace(a.ArtifactPath)
	case "worker-lifecycle-policy-scheduler-run-artifact":
		return "action: worker lifecycle policy scheduler run artifact selected " + strings.TrimSpace(a.ArtifactPath)
	case "worker-lifecycle-policy-selection":
		if a.State == "cleared" {
			return "action: worker lifecycle policy selection cleared " + strings.TrimSpace(a.ArtifactPath)
		}
		return "action: worker lifecycle policy selection selected " + strings.TrimSpace(a.ArtifactPath)
	case "worker-lifecycle-action":
		name := strings.TrimSpace(a.Name)
		if name == "" {
			name = strings.TrimSpace(a.Row)
		}
		return "action: worker lifecycle action selected " + name
	case "worker-lease-result-reconciliation-artifact":
		return "action: worker lease result reconciliation artifact selected " + strings.TrimSpace(a.ArtifactPath)
	case "worker-lease-result-reconciliation-execution-artifact":
		return "action: worker lease result reconciliation execution artifact selected " + strings.TrimSpace(a.ArtifactPath)
	case "worker-lease-result-reconciliation-execution-result":
		name := strings.TrimSpace(a.Name)
		if name == "" {
			name = strings.TrimSpace(a.Row)
		}
		return "action: worker lease result reconciliation execution result selected " + name
	case "worker-lease-reclaim-artifact":
		return "action: worker lease reclaim artifact selected " + strings.TrimSpace(a.ArtifactPath)
	case "worker-lease-reclaim-execution-artifact":
		return "action: worker lease reclaim execution artifact selected " + strings.TrimSpace(a.ArtifactPath)
	case "worker-lease-reclaim-execution-result":
		name := strings.TrimSpace(a.Name)
		if name == "" {
			name = strings.TrimSpace(a.Row)
		}
		return "action: worker lease reclaim execution result selected " + name
	case "worker-placement-artifact":
		return "action: worker placement artifact selected " + strings.TrimSpace(a.ArtifactPath)
	case "worker-placement-dispatch-artifact":
		return "action: worker placement dispatch artifact selected " + strings.TrimSpace(a.ArtifactPath)
	case "worker-queue-claim-artifact":
		return "action: worker queue claim artifact selected " + strings.TrimSpace(a.ArtifactPath)
	case "worker-queue-dispatch-execution-artifact":
		return "action: worker queue dispatch execution artifact selected " + strings.TrimSpace(a.ArtifactPath)
	case "worker-queue-dispatch-result-artifact":
		return "action: worker queue dispatch result artifact selected " + strings.TrimSpace(a.ArtifactPath)
	case "worker-dispatch-retry-plan-artifact":
		return "action: worker dispatch retry plan artifact selected " + strings.TrimSpace(a.ArtifactPath)
	case "worker-dispatch-retry-dispatch-artifact":
		return "action: worker dispatch retry dispatch artifact selected " + strings.TrimSpace(a.ArtifactPath)
	case "worker-dispatch-retry-dispatch-execution-artifact":
		return "action: worker dispatch retry dispatch execution artifact selected " + strings.TrimSpace(a.ArtifactPath)
	case "worker-dispatch-retry-dispatch-execution-result":
		return "action: worker dispatch retry dispatch execution result selected " + strings.TrimSpace(a.ArtifactPath)
	case "worker-dispatch-retry-scheduler-run-artifact":
		return "action: worker dispatch retry scheduler run artifact selected " + strings.TrimSpace(a.ArtifactPath)
	case "report-projection-dispatch-artifact":
		return "action: report projection dispatch artifact selected " + strings.TrimSpace(a.ArtifactPath)
	case "report-projection-dispatch-execution-artifact":
		return "action: report projection dispatch execution artifact selected " + strings.TrimSpace(a.ArtifactPath)
	case "report-projection-execution-review":
		return "action: report projection execution review selected " + strings.TrimSpace(a.ReviewPath)
	case "report-projection-execution-selection":
		if a.State == "cleared" {
			return "action: report projection execution selection cleared " + strings.TrimSpace(a.ArtifactPath)
		}
		return "action: report projection execution selection selected " + strings.TrimSpace(a.ArtifactPath)
	case "worker-transport-request-artifact":
		return "action: worker transport request selected " + strings.TrimSpace(a.ArtifactPath)
	case "worker-transport-daemon-artifact":
		return "action: worker transport daemon artifact selected " + strings.TrimSpace(a.ArtifactPath)
	case "worker-transport-daemon-supervisor-artifact":
		return "action: worker transport daemon supervisor artifact selected " + strings.TrimSpace(a.ArtifactPath)
	case "worker-transport-daemon-control-artifact":
		return "action: worker transport daemon control artifact selected " + strings.TrimSpace(a.ArtifactPath)
	case "workspace-safety-strict-gate":
		return "action: workspace safety strict gate selected " + valueOrDefault(strings.TrimSpace(a.State), "unknown")
	case "workspace-safety-strict-error":
		return "action: workspace safety strict error selected"
	case "workspace-safety-warning":
		return "action: workspace safety warning selected"
	case "workspace-safety-error":
		return "action: workspace safety error selected"
	case "compile-preview":
		action := strings.TrimSpace(a.ActionType)
		if action == "" {
			action = strings.TrimSpace(a.Row)
		}
		return "action: compile preview selected " + action
	case "compile-preview-signals":
		return "action: compile-preview signals selected " + strings.TrimSpace(a.ArtifactPath)
	case "compile-preview-signal-domain":
		return "action: compile-preview signal domain selected " + strings.TrimSpace(a.Name)
	case "compile-preview-signal-frontier":
		name := strings.TrimSpace(a.Name)
		if strings.TrimSpace(a.Pack) != "" && name != "" {
			name = strings.TrimSpace(a.Pack) + "/" + name
		}
		if name == "" {
			name = strings.TrimSpace(a.Row)
		}
		return "action: compile-preview signal frontier selected " + name
	case "compile-preview-signal-history":
		name := strings.TrimSpace(a.Name)
		if strings.TrimSpace(a.Pack) != "" && name != "" {
			name = strings.TrimSpace(a.Pack) + "/" + name
		}
		if name == "" {
			name = strings.TrimSpace(a.Row)
		}
		return "action: compile-preview signal history selected " + name
	case "compile-preview-review-artifact":
		return "action: compile-preview review artifact selected " + strings.TrimSpace(a.ArtifactPath)
	case "compile-preview-review-comparison":
		return "action: compile-preview review comparison selected " + strings.TrimSpace(a.CandidatePath)
	case "compile-preview-review-selection":
		if a.State == "cleared" {
			return "action: compile-preview review selection cleared " + strings.TrimSpace(a.ReviewPath)
		}
		return "action: compile-preview review selection selected " + strings.TrimSpace(a.ReviewPath)
	case "replay-workbench-session-artifact":
		return "action: replay workbench session artifact selected " + strings.TrimSpace(a.ArtifactPath)
	case "replay-workbench-collaboration-artifact":
		return "action: replay workbench collaboration artifact selected " + strings.TrimSpace(a.ArtifactPath)
	case "replay-workbench-collaboration-note":
		return "action: replay workbench collaboration note selected " + strings.TrimSpace(a.ArtifactPath)
	case "replay-workbench-collaboration-presence":
		name := strings.TrimSpace(a.Participant)
		if name == "" {
			name = strings.TrimSpace(a.ArtifactPath)
		}
		return "action: replay workbench collaboration presence selected " + name
	case "replay-workbench-session-selection":
		if a.State == "cleared" {
			return "action: replay workbench session selection cleared " + strings.TrimSpace(a.ArtifactPath)
		}
		return "action: replay workbench session selection selected " + strings.TrimSpace(a.ArtifactPath)
	case "replay-workbench-collaboration-selection":
		if a.State == "cleared" {
			return "action: replay workbench collaboration selection cleared " + strings.TrimSpace(a.ArtifactPath)
		}
		return "action: replay workbench collaboration selection selected " + strings.TrimSpace(a.ArtifactPath)
	case "replay-workbench-collaboration-note-selection":
		if a.State == "cleared" {
			return "action: replay workbench collaboration note selection cleared " + strings.TrimSpace(a.ArtifactPath)
		}
		return "action: replay workbench collaboration note selection selected " + strings.TrimSpace(a.ArtifactPath)
	case "worker-lifecycle-action-plan-selection":
		if a.State == "cleared" {
			return "action: worker lifecycle action plan selection cleared " + strings.TrimSpace(a.ArtifactPath)
		}
		return "action: worker lifecycle action plan selection selected " + strings.TrimSpace(a.ArtifactPath)
	default:
		return "action: " + strings.TrimSpace(a.Row)
	}
}

func (a RuntimeAction) RenderRows() []string {
	rows := []string{a.FeedbackRow()}
	isWorkerLifecycleAction := strings.TrimSpace(a.Kind) == "worker-lifecycle-action"
	if strings.TrimSpace(a.Kind) == "worker-lifecycle-policy-scheduler-run-artifact" {
		if name := strings.TrimSpace(a.Name); name != "" {
			rows = append(rows, "name: "+name)
		}
	}
	if command := strings.TrimSpace(a.Command); command != "" {
		rows = append(rows, "action command: "+command)
	}
	if output := strings.TrimSpace(a.Output); output != "" {
		rows = append(rows, "action output: "+output)
	}
	if hint := strings.TrimSpace(a.Hint); hint != "" {
		rows = append(rows, "action hint: "+hint)
	}
	if name := strings.TrimSpace(a.Name); name != "" {
		rows = append(rows, "name: "+name)
	}
	if dependsOn := strings.TrimSpace(a.DependsOn); dependsOn != "" {
		rows = append(rows, "action depends_on: "+dependsOn)
	}
	if reason := strings.TrimSpace(a.Reason); reason != "" && strings.TrimSpace(a.Kind) != "worker-lifecycle-action" {
		rows = append(rows, "reason: "+reason)
	}
	if strings.TrimSpace(a.Kind) == "worker-lifecycle-action" {
		if name := strings.TrimSpace(a.Name); name != "" {
			rows = append(rows, "action type: "+name)
		}
		if gate := strings.TrimSpace(a.Gate); gate != "" {
			rows = append(rows, "action gate: "+gate)
		}
		if worker := strings.TrimSpace(a.WorkerID); worker != "" {
			rows = append(rows, "worker: "+worker)
		}
		if runID := strings.TrimSpace(a.RunID); runID != "" {
			rows = append(rows, "run: "+runID)
		}
		if executionID := strings.TrimSpace(a.ExecutionID); executionID != "" {
			rows = append(rows, "execution: "+executionID)
		}
		if reason := strings.TrimSpace(a.Reason); reason != "" {
			rows = append(rows, "reason: "+reason)
		}
		if recommended := strings.TrimSpace(a.RecommendedStatus); recommended != "" {
			rows = append(rows, "recommended status: "+recommended)
		}
		if requestPath := strings.TrimSpace(a.RequestPath); requestPath != "" {
			rows = append(rows, "request path: "+requestPath)
		}
		if leasePath := strings.TrimSpace(a.LeasePath); leasePath != "" {
			rows = append(rows, "lease path: "+leasePath)
		}
		if resultPath := strings.TrimSpace(a.ResultPath); resultPath != "" {
			rows = append(rows, "result path: "+resultPath)
		}
		if dispatchPath := strings.TrimSpace(a.DispatchPath); dispatchPath != "" {
			rows = append(rows, "dispatch path: "+dispatchPath)
		}
	}
	if artifact := strings.TrimSpace(a.ArtifactPath); artifact != "" {
		if strings.TrimSpace(a.Kind) == "worker-lifecycle-policy-selection" {
			rows = append(rows, "policy path: "+artifact)
		} else {
			rows = append(rows, "artifact path: "+artifact)
		}
	}
	if reviewPath := strings.TrimSpace(a.ReviewPath); reviewPath != "" {
		rows = append(rows, "review path: "+reviewPath)
	}
	if strings.TrimSpace(a.Kind) == "worker-lifecycle-policy-selection" || strings.TrimSpace(a.Kind) == "report-projection-execution-selection" {
		if status := strings.TrimSpace(a.State); status != "" {
			rows = append(rows, "selection status: "+status)
		}
		if selectionPath := strings.TrimSpace(a.RequestPath); selectionPath != "" {
			rows = append(rows, "selection path: "+selectionPath)
		}
	}
	if dispatch := strings.TrimSpace(a.DispatchPath); dispatch != "" && !isWorkerLifecycleAction {
		rows = append(rows, "dispatch path: "+dispatch)
	}
	if retryDispatch := strings.TrimSpace(a.RetryDispatchPath); retryDispatch != "" {
		rows = append(rows, "retry dispatch path: "+retryDispatch)
	}
	if source := strings.TrimSpace(a.Source); source != "" {
		rows = append(rows, "source: "+source)
	}
	if target := strings.TrimSpace(a.Target); target != "" {
		rows = append(rows, "target: "+target)
	}
	if transport := strings.TrimSpace(a.TransportMode); transport != "" {
		rows = append(rows, "transport: "+transport)
	}
	if score := strings.TrimSpace(a.Score); score != "" {
		rows = append(rows, "score: "+score)
	}
	if weight := strings.TrimSpace(a.Weight); weight != "" {
		rows = append(rows, "weight: "+weight)
	}
	if sourceExecution := strings.TrimSpace(a.SourceExecutionPath); sourceExecution != "" {
		rows = append(rows, "source execution path: "+sourceExecution)
	}
	if leasePath := strings.TrimSpace(a.LeasePath); leasePath != "" && !isWorkerLifecycleAction {
		rows = append(rows, "lease path: "+leasePath)
	}
	if plannedReportPath := strings.TrimSpace(a.PlannedReportPath); plannedReportPath != "" {
		rows = append(rows, "planned report path: "+plannedReportPath)
	} else if resultPath := strings.TrimSpace(a.ResultPath); resultPath != "" && !isWorkerLifecycleAction {
		rows = append(rows, "result path: "+resultPath)
	}
	if ready := strings.TrimSpace(a.Ready); ready != "" {
		rows = append(rows, "ready: "+ready)
	}
	if results := strings.TrimSpace(a.Results); results != "" {
		rows = append(rows, "results: "+results)
	}
	if skipped := strings.TrimSpace(a.Skipped); skipped != "" {
		rows = append(rows, "skipped: "+skipped)
	}
	if failed := strings.TrimSpace(a.Failed); failed != "" {
		rows = append(rows, "failed: "+failed)
	}
	if failed := strings.TrimSpace(a.Failed); failed != "" {
		rows = append(rows, "failed: "+failed)
	}
	if maxPlanNodes := strings.TrimSpace(a.MaxPlanNodes); maxPlanNodes != "" {
		rows = append(rows, "max plan nodes: "+maxPlanNodes)
	}
	if domainPriorities := strings.TrimSpace(a.DomainPriorities); domainPriorities != "" {
		rows = append(rows, "domain priorities: "+domainPriorities)
	}
	if frontierScores := strings.TrimSpace(a.FrontierScores); frontierScores != "" {
		rows = append(rows, "frontier scores: "+frontierScores)
	}
	if historicalStats := strings.TrimSpace(a.HistoricalStats); historicalStats != "" {
		rows = append(rows, "historical stats: "+historicalStats)
	}
	if runID := strings.TrimSpace(a.RunID); runID != "" && !isWorkerLifecycleAction {
		rows = append(rows, "run: "+runID)
	}
	if selectedRun := strings.TrimSpace(a.SelectedRunID); selectedRun != "" {
		rows = append(rows, "selected run: "+selectedRun)
	}
	if sessionPath := strings.TrimSpace(a.SessionPath); sessionPath != "" {
		rows = append(rows, "session path: "+sessionPath)
	}
	if runIDs := strings.TrimSpace(a.RunIDs); runIDs != "" {
		rows = append(rows, "run ids: "+runIDs)
	}
	if filters := strings.TrimSpace(a.Filters); filters != "" {
		rows = append(rows, "filters: "+filters)
	}
	if endpoints := strings.TrimSpace(a.Endpoints); endpoints != "" {
		rows = append(rows, "endpoints: "+endpoints)
	}
	if participants := strings.TrimSpace(a.Participants); participants != "" {
		rows = append(rows, "participants: "+participants)
	}
	if participant := strings.TrimSpace(a.Participant); participant != "" {
		rows = append(rows, "participant: "+participant)
	}
	if collaboration := strings.TrimSpace(a.CollaborationPath); collaboration != "" {
		rows = append(rows, "collaboration path: "+collaboration)
	}
	if seenBy := strings.TrimSpace(a.SeenBy); seenBy != "" {
		rows = append(rows, "seen by: "+seenBy)
	}
	if seenAt := strings.TrimSpace(a.SeenAt); seenAt != "" {
		rows = append(rows, "seen at: "+seenAt)
	}
	if createdBy := strings.TrimSpace(a.CreatedBy); createdBy != "" {
		rows = append(rows, "created by: "+createdBy)
	}
	if createdAt := strings.TrimSpace(a.CreatedAt); createdAt != "" {
		rows = append(rows, "created at: "+createdAt)
	}
	if successes := strings.TrimSpace(a.Successes); successes != "" {
		rows = append(rows, "successes: "+successes)
	}
	if failures := strings.TrimSpace(a.Failures); failures != "" {
		rows = append(rows, "failures: "+failures)
	}
	if status := strings.TrimSpace(a.RequestStatus); status != "" {
		rows = append(rows, "request status: "+status)
	}
	if path := strings.TrimSpace(a.RequestPath); path != "" && !isWorkerLifecycleAction {
		rows = append(rows, "request path: "+path)
	}
	if requestError := strings.TrimSpace(a.RequestError); requestError != "" {
		rows = append(rows, "request error: "+requestError)
	}
	if controlAction := strings.TrimSpace(a.ControlAction); controlAction != "" {
		rows = append(rows, "control action: "+controlAction)
	}
	if actionType := strings.TrimSpace(a.ActionType); actionType != "" {
		rows = append(rows, "action type: "+actionType)
	}
	if nodes := strings.TrimSpace(a.Nodes); nodes != "" {
		rows = append(rows, "nodes: "+nodes)
	}
	if warnings := strings.TrimSpace(a.Warnings); warnings != "" {
		rows = append(rows, "warnings: "+warnings)
	}
	if current := strings.TrimSpace(a.Current); current != "" {
		rows = append(rows, "current: "+current)
	}
	if selectedBy := strings.TrimSpace(a.SelectedBy); selectedBy != "" {
		rows = append(rows, "selected by: "+selectedBy)
	}
	if selectedAt := strings.TrimSpace(a.SelectedAt); selectedAt != "" {
		rows = append(rows, "selected at: "+selectedAt)
	}
	if clearedBy := strings.TrimSpace(a.ClearedBy); clearedBy != "" {
		rows = append(rows, "cleared by: "+clearedBy)
	}
	if clearedAt := strings.TrimSpace(a.ClearedAt); clearedAt != "" {
		rows = append(rows, "cleared at: "+clearedAt)
	}
	if superseded := strings.TrimSpace(a.Superseded); superseded != "" {
		rows = append(rows, "superseded: "+superseded)
	}
	if decision := strings.TrimSpace(a.Decision); decision != "" && decision != "unreviewed" {
		rows = append(rows, "decision: "+decision)
	}
	if reviewer := strings.TrimSpace(a.Reviewer); reviewer != "" && reviewer != "unknown" {
		rows = append(rows, "reviewer: "+reviewer)
	}
	if reviewedAt := strings.TrimSpace(a.ReviewedAt); reviewedAt != "" && reviewedAt != "unknown" {
		rows = append(rows, "reviewed at: "+reviewedAt)
	}
	if reviewReason := strings.TrimSpace(a.ReviewReason); reviewReason != "" && reviewReason != "none" {
		rows = append(rows, "review reason: "+reviewReason)
	}
	if status := strings.TrimSpace(a.ExecutionStatus); status != "" {
		rows = append(rows, "execution status: "+status)
		if status == "confirm-required" {
			confirmKey := "x"
			if strings.TrimSpace(a.ControlAction) == "restart" {
				confirmKey = "R"
			}
			if strings.TrimSpace(a.ControlAction) == "cancel" {
				confirmKey = "C"
			}
			if strings.TrimSpace(a.ControlAction) == "expire" {
				confirmKey = "E"
			}
			if strings.TrimSpace(a.ControlAction) == "renew" {
				confirmKey = "L"
			}
			rows = append(rows, "execution confirm: press "+confirmKey+" again")
		}
	}
	if path := strings.TrimSpace(a.ExecutionPath); path != "" {
		rows = append(rows, "execution path: "+path)
	}
	if executionError := strings.TrimSpace(a.ExecutionError); executionError != "" {
		rows = append(rows, "execution error: "+executionError)
	}
	if runtimeActionIsReadOnly(a) {
		rows = append(rows, "action mode: read-only")
	} else {
		rows = append(rows, "action mode: metadata-only")
	}
	return rows
}

func DefaultRuntimeTabs() RuntimeTabs {
	tabs := []RuntimeTab{
		RuntimeTabNodeInspector,
		RuntimeTabModelRuntime,
		RuntimeTabAgentRuntime,
		RuntimeTabToolchainHealth,
		RuntimeTabCoverageMatrix,
		RuntimeTabFrontierQueue,
		RuntimeTabTimelineLanes,
	}
	return RuntimeTabs{
		Tabs:     tabs,
		Active:   RuntimeTabNodeInspector,
		Selected: map[RuntimeTab]int{},
		Actions:  map[RuntimeTab]RuntimeAction{},
		Panels: map[RuntimeTab][]string{
			RuntimeTabNodeInspector:   []string{"selected: none"},
			RuntimeTabModelRuntime:    []string{"model: not-set", "context: 0%"},
			RuntimeTabAgentRuntime:    []string{"agents: 0", "running: 0"},
			RuntimeTabToolchainHealth: []string{"toolchain: unknown"},
			RuntimeTabCoverageMatrix:  []string{"coverage: no data"},
			RuntimeTabFrontierQueue:   []string{"frontier: empty"},
			RuntimeTabTimelineLanes:   []string{"timeline: no events"},
		},
	}
}

func (t RuntimeTabs) Select(tab RuntimeTab) RuntimeTabs {
	t = t.ensure()
	for _, candidate := range t.Tabs {
		if candidate == tab {
			t.Active = tab
			return t
		}
	}
	return t
}

func (t RuntimeTabs) Next() RuntimeTabs {
	t = t.ensure()
	if len(t.Tabs) == 0 {
		return t
	}
	for i, tab := range t.Tabs {
		if tab == t.Active {
			t.Active = t.Tabs[(i+1)%len(t.Tabs)]
			return t
		}
	}
	t.Active = t.Tabs[0]
	return t
}

func (t RuntimeTabs) WithRows(tab RuntimeTab, rows []string) RuntimeTabs {
	t = t.ensure()
	panels := make(map[RuntimeTab][]string, len(t.Panels)+1)
	for key, value := range t.Panels {
		panels[key] = append([]string(nil), value...)
	}
	panels[tab] = append([]string(nil), rows...)
	t.Panels = panels
	t = t.clampSelected(tab)
	t = t.clearAction(tab)
	return t
}

func (t RuntimeTabs) AppendRows(tab RuntimeTab, rows []string) RuntimeTabs {
	t = t.ensure()
	existing := append([]string(nil), t.Panels[tab]...)
	existing = append(existing, rows...)
	return t.WithRows(tab, existing)
}

func (t RuntimeTabs) AppendRowsPreservingAction(tab RuntimeTab, rows []string) RuntimeTabs {
	t = t.ensure()
	if len(rows) == 0 {
		return t
	}
	panels := make(map[RuntimeTab][]string, len(t.Panels)+1)
	for key, value := range t.Panels {
		panels[key] = append([]string(nil), value...)
	}
	panels[tab] = append(panels[tab], rows...)
	t.Panels = panels
	t = t.clampSelected(tab)
	return t
}

func (t RuntimeTabs) ReplaceRowsByPrefixesPreservingAction(tab RuntimeTab, prefixes []string, rows []string) RuntimeTabs {
	t = t.ensure()
	prefixes = runtimeRowPrefixesWithPolicySchedulerRows(prefixes)
	panels := make(map[RuntimeTab][]string, len(t.Panels)+1)
	for key, value := range t.Panels {
		panels[key] = append([]string(nil), value...)
	}
	filtered := make([]string, 0, len(panels[tab])+len(rows))
	insertAt := -1
	for _, row := range panels[tab] {
		trimmed := strings.TrimSpace(row)
		remove := false
		for _, prefix := range prefixes {
			if strings.HasPrefix(trimmed, strings.TrimSpace(prefix)) {
				remove = true
				break
			}
		}
		if !remove {
			filtered = append(filtered, row)
			continue
		}
		if insertAt < 0 {
			insertAt = len(filtered)
		}
	}
	if insertAt < 0 {
		filtered = append(filtered, rows...)
	} else {
		next := make([]string, 0, len(filtered)+len(rows))
		next = append(next, filtered[:insertAt]...)
		next = append(next, rows...)
		next = append(next, filtered[insertAt:]...)
		filtered = next
	}
	panels[tab] = filtered
	t.Panels = panels
	t = t.clampSelected(tab)
	return t
}

func runtimeRowPrefixesWithPolicySchedulerRows(prefixes []string) []string {
	includeScheduler := false
	for _, prefix := range prefixes {
		trimmed := strings.TrimSpace(prefix)
		if trimmed == "worker lifecycle policy runs:" || trimmed == "worker lifecycle policy run artifact:" || trimmed == "worker lifecycle policy artifacts:" || trimmed == "worker lifecycle policy selection:" {
			includeScheduler = true
			break
		}
	}
	if !includeScheduler {
		return prefixes
	}
	next := append([]string(nil), prefixes...)
	for _, schedulerPrefix := range []string{
		"worker lifecycle policy scheduler runs:",
		"worker lifecycle policy scheduler run artifact:",
		"worker lifecycle policy scheduler run compact:",
		"worker lifecycle policy scheduler run gates:",
		"worker lifecycle policy scheduler run skip:",
		"worker lifecycle policy scheduler run error:",
		"worker lifecycle policy scheduler run path:",
	} {
		found := false
		for _, prefix := range next {
			if strings.TrimSpace(prefix) == schedulerPrefix {
				found = true
				break
			}
		}
		if !found {
			next = append(next, schedulerPrefix)
		}
	}
	return next
}

func (t RuntimeTabs) PrependRowsPreservingAction(tab RuntimeTab, rows []string) RuntimeTabs {
	t = t.ensure()
	if len(rows) == 0 {
		return t
	}
	panels := make(map[RuntimeTab][]string, len(t.Panels)+1)
	for key, value := range t.Panels {
		panels[key] = append([]string(nil), value...)
	}
	existing := append([]string(nil), panels[tab]...)
	panels[tab] = append(append([]string(nil), rows...), existing...)
	t.Panels = panels
	t = t.clampSelected(tab)
	return t
}

func (t RuntimeTabs) SelectFirstRowMatching(tab RuntimeTab, prefix string, contains string) RuntimeTabs {
	t = t.ensure()
	rows := t.Panels[tab]
	for index, row := range rows {
		trimmed := strings.TrimSpace(row)
		if prefix != "" && !strings.HasPrefix(trimmed, prefix) {
			continue
		}
		if contains != "" && !strings.Contains(trimmed, contains) {
			continue
		}
		t.Selected[tab] = index
		return t
	}
	return t
}

func (t RuntimeTabs) Render(width int, height int) string {
	t = t.ensure()
	rows := t.tabHeaderRows(width)
	if action, ok := t.ActiveRuntimeAction(); ok {
		rows = append(rows, action.RenderRows()...)
	}
	rows = append(rows, t.activeRenderedRows()...)
	return renderPanel("Runtime Tabs", rows, width, height)
}

func (t RuntimeTabs) ActiveRows() []string {
	t = t.ensure()
	return append([]string(nil), t.Panels[t.Active]...)
}

func (t RuntimeTabs) SelectRow(delta int) RuntimeTabs {
	t = t.ensure()
	rows := t.Panels[t.Active]
	if len(rows) == 0 {
		t.Selected[t.Active] = 0
		t = t.clearAction(t.Active)
		return t
	}
	index := t.Selected[t.Active] + delta
	if index < 0 {
		index = 0
	}
	if index >= len(rows) {
		index = len(rows) - 1
	}
	t.Selected[t.Active] = index
	t = t.clearAction(t.Active)
	return t
}

func (t RuntimeTabs) ActiveSelectedRow() (string, bool) {
	t = t.ensure()
	rows := t.Panels[t.Active]
	if len(rows) == 0 {
		return "", false
	}
	index := t.Selected[t.Active]
	if index < 0 {
		index = 0
	}
	if index >= len(rows) {
		index = len(rows) - 1
	}
	return rows[index], true
}

func (t RuntimeTabs) ActivateSelectedRow() RuntimeTabs {
	t = t.ensure()
	row, ok := t.ActiveSelectedRow()
	if !ok {
		return t.clearAction(t.Active)
	}
	action, ok := runtimeActionForRow(row)
	if !ok {
		return t.clearAction(t.Active)
	}
	t.Actions[t.Active] = action
	return t
}

func (t RuntimeTabs) WithActiveRuntimeAction(action RuntimeAction) RuntimeTabs {
	t = t.ensure()
	t.Actions[t.Active] = action
	return t
}

func (t RuntimeTabs) ActiveAction() (string, bool) {
	action, ok := t.ActiveRuntimeAction()
	if !ok {
		return "", false
	}
	return action.FeedbackRow(), true
}

func (t RuntimeTabs) ActiveRuntimeAction() (RuntimeAction, bool) {
	t = t.ensure()
	action := t.Actions[t.Active]
	if strings.TrimSpace(action.Kind) == "" {
		return RuntimeAction{}, false
	}
	return action, true
}

func (t RuntimeTabs) ensure() RuntimeTabs {
	if len(t.Tabs) == 0 {
		return DefaultRuntimeTabs()
	}
	if t.Active == "" {
		t.Active = t.Tabs[0]
	}
	if t.Panels == nil {
		t.Panels = map[RuntimeTab][]string{}
	}
	if t.Selected == nil {
		t.Selected = map[RuntimeTab]int{}
	}
	if t.Actions == nil {
		t.Actions = map[RuntimeTab]RuntimeAction{}
	}
	t = t.clampSelected(t.Active)
	return t
}

func (t RuntimeTabs) clampSelected(tab RuntimeTab) RuntimeTabs {
	if t.Selected == nil {
		t.Selected = map[RuntimeTab]int{}
	}
	rows := t.Panels[tab]
	index := t.Selected[tab]
	if index < 0 || len(rows) == 0 {
		t.Selected[tab] = 0
		return t
	}
	if index >= len(rows) {
		t.Selected[tab] = len(rows) - 1
	}
	return t
}

func (t RuntimeTabs) clearAction(tab RuntimeTab) RuntimeTabs {
	if t.Actions != nil {
		delete(t.Actions, tab)
	}
	return t
}

func runtimeActionForRow(row string) (RuntimeAction, bool) {
	trimmed := strings.TrimSpace(row)
	if strings.HasPrefix(trimmed, "execution artifact:") {
		return runtimeExecutionArtifactActionForRow(trimmed), true
	}
	if strings.HasPrefix(trimmed, "execution result ") {
		return runtimeExecutionResultActionForRow(trimmed), true
	}
	if strings.HasPrefix(trimmed, "dispatch artifact:") {
		return runtimeDispatchArtifactActionForRow(trimmed), true
	}
	if strings.HasPrefix(trimmed, "retry dispatch artifact:") {
		return runtimeRetryDispatchArtifactActionForRow(trimmed), true
	}
	if strings.HasPrefix(trimmed, "retry execution artifact:") {
		return runtimeRetryExecutionArtifactActionForRow(trimmed), true
	}
	if strings.HasPrefix(trimmed, "retry execution result refs ") {
		return runtimeRetryExecutionResultActionForRow(trimmed), true
	}
	if strings.HasPrefix(trimmed, "retry execution result ") {
		return runtimeRetryExecutionResultActionForRow(trimmed), true
	}
	if strings.HasPrefix(trimmed, "provider usage summary:") {
		return runtimeProviderUsageSummaryActionForRow(trimmed), true
	}
	if strings.HasPrefix(trimmed, "install orchestration artifact:") {
		return runtimeInstallOrchestrationArtifactActionForRow(trimmed), true
	}
	if strings.HasPrefix(trimmed, "worker orchestration supervisor artifact:") {
		return runtimeWorkerOrchestrationSupervisorArtifactActionForRow(trimmed), true
	}
	if strings.HasPrefix(trimmed, "worker orchestration artifact:") {
		return runtimeWorkerOrchestrationArtifactActionForRow(trimmed), true
	}
	if strings.HasPrefix(trimmed, "worker lifecycle audit artifact:") {
		return runtimeWorkerLifecycleAuditArtifactActionForRow(trimmed), true
	}
	if strings.HasPrefix(trimmed, "worker lifecycle audit item:") {
		return runtimeWorkerLifecycleAuditItemActionForRow(trimmed), true
	}
	if strings.HasPrefix(trimmed, "worker lifecycle review artifact:") {
		return runtimeWorkerLifecycleReviewArtifactActionForRow(trimmed), true
	}
	if strings.HasPrefix(trimmed, "worker lifecycle action plan artifact:") {
		return runtimeWorkerLifecycleActionPlanArtifactActionForRow(trimmed), true
	}
	if strings.HasPrefix(trimmed, "worker lifecycle action plan selection:") {
		return runtimeWorkerLifecycleActionPlanSelectionActionForRow(trimmed), true
	}
	if strings.HasPrefix(trimmed, "worker lifecycle maintenance artifact:") {
		return runtimeWorkerLifecycleMaintenanceArtifactActionForRow(trimmed), true
	}
	if strings.HasPrefix(trimmed, "worker lifecycle maintenance supervisor artifact:") {
		return runtimeWorkerLifecycleMaintenanceSupervisorArtifactActionForRow(trimmed), true
	}
	if strings.HasPrefix(trimmed, "worker lifecycle policy artifact:") {
		return runtimeWorkerLifecyclePolicyArtifactActionForRow(trimmed), true
	}
	if strings.HasPrefix(trimmed, "worker lifecycle policy run artifact:") {
		return runtimeWorkerLifecyclePolicyRunArtifactActionForRow(trimmed), true
	}
	if strings.HasPrefix(trimmed, "worker lifecycle policy scheduler run artifact:") {
		return runtimeWorkerLifecyclePolicySchedulerRunArtifactActionForRow(trimmed), true
	}
	if strings.HasPrefix(trimmed, "worker lifecycle policy selection:") {
		return runtimeWorkerLifecyclePolicySelectionActionForRow(trimmed), true
	}
	if strings.HasPrefix(trimmed, "worker lifecycle action:") {
		return runtimeWorkerLifecycleActionForRow(trimmed), true
	}
	if strings.HasPrefix(trimmed, "worker lease result reconciliation artifact:") {
		return runtimeWorkerLeaseResultReconciliationArtifactActionForRow(trimmed), true
	}
	if strings.HasPrefix(trimmed, "worker lease result reconciliation execution artifact:") {
		return runtimeWorkerLeaseResultReconciliationExecutionArtifactActionForRow(trimmed), true
	}
	if strings.HasPrefix(trimmed, "lease result reconciliation execution result ") {
		return runtimeWorkerLeaseResultReconciliationExecutionResultActionForRow(trimmed), true
	}
	if strings.HasPrefix(trimmed, "worker lease reclaim artifact:") {
		return runtimeWorkerLeaseReclaimArtifactActionForRow(trimmed), true
	}
	if strings.HasPrefix(trimmed, "worker lease reclaim execution artifact:") {
		return runtimeWorkerLeaseReclaimExecutionArtifactActionForRow(trimmed), true
	}
	if strings.HasPrefix(trimmed, "worker lease reclaim execution result ") {
		return runtimeWorkerLeaseReclaimExecutionResultActionForRow(trimmed), true
	}
	if strings.HasPrefix(trimmed, "worker placement artifact:") {
		return runtimeWorkerPlacementArtifactActionForRow(trimmed), true
	}
	if strings.HasPrefix(trimmed, "worker placement dispatch artifact:") {
		return runtimeWorkerPlacementDispatchArtifactActionForRow(trimmed), true
	}
	if strings.HasPrefix(trimmed, "worker queue claim artifact:") {
		return runtimeWorkerQueueClaimArtifactActionForRow(trimmed), true
	}
	if strings.HasPrefix(trimmed, "worker queue dispatch execution artifact:") {
		return runtimeWorkerQueueDispatchExecutionArtifactActionForRow(trimmed), true
	}
	if strings.HasPrefix(trimmed, "worker queue dispatch result artifact:") {
		return runtimeWorkerQueueDispatchResultArtifactActionForRow(trimmed), true
	}
	if strings.HasPrefix(trimmed, "worker dispatch retry plan artifact:") {
		return runtimeWorkerDispatchRetryPlanArtifactActionForRow(trimmed), true
	}
	if strings.HasPrefix(trimmed, "worker dispatch retry dispatch artifact:") {
		return runtimeWorkerDispatchRetryDispatchArtifactActionForRow(trimmed), true
	}
	if strings.HasPrefix(trimmed, "worker dispatch retry execution artifact:") {
		return runtimeWorkerDispatchRetryDispatchExecutionArtifactActionForRow(trimmed), true
	}
	if strings.HasPrefix(trimmed, "worker dispatch retry execution result:") {
		return runtimeWorkerDispatchRetryDispatchExecutionResultActionForRow(trimmed), true
	}
	if strings.HasPrefix(trimmed, "worker dispatch retry scheduler run artifact:") {
		return runtimeWorkerDispatchRetrySchedulerRunArtifactActionForRow(trimmed), true
	}
	if strings.HasPrefix(trimmed, "report projection execution selection:") {
		return runtimeReportProjectionExecutionSelectionActionForRow(trimmed), true
	}
	if strings.HasPrefix(trimmed, "report projection execution review:") {
		return runtimeReportProjectionExecutionReviewActionForRow(trimmed), true
	}
	if strings.HasPrefix(trimmed, "report projection dispatch execution artifact:") {
		return runtimeReportProjectionDispatchExecutionArtifactActionForRow(trimmed), true
	}
	if strings.HasPrefix(trimmed, "report projection dispatch artifact:") {
		return runtimeReportProjectionDispatchArtifactActionForRow(trimmed), true
	}
	if strings.HasPrefix(trimmed, "worker transport request:") {
		return runtimeWorkerTransportRequestArtifactActionForRow(trimmed), true
	}
	if strings.HasPrefix(trimmed, "worker transport daemon artifact:") {
		return runtimeWorkerTransportDaemonArtifactActionForRow(trimmed), true
	}
	if strings.HasPrefix(trimmed, "worker transport daemon supervisor artifact:") {
		return runtimeWorkerTransportDaemonSupervisorArtifactActionForRow(trimmed), true
	}
	if strings.HasPrefix(trimmed, "worker transport daemon control artifact:") {
		return runtimeWorkerTransportDaemonControlArtifactActionForRow(trimmed), true
	}
	if strings.HasPrefix(trimmed, "workspace safety strict gate:") {
		return runtimeWorkspaceSafetyStrictGateActionForRow(trimmed), true
	}
	if strings.HasPrefix(trimmed, "safety strict error:") {
		return RuntimeAction{
			Kind:          "workspace-safety-strict-error",
			State:         "blocked",
			ExecutionMode: "strict",
			Reason:        strings.TrimSpace(strings.TrimPrefix(trimmed, "safety strict error:")),
			Row:           trimmed,
		}, true
	}
	if strings.HasPrefix(trimmed, "safety warning:") {
		return RuntimeAction{
			Kind:   "workspace-safety-warning",
			State:  "warning",
			Reason: strings.TrimSpace(strings.TrimPrefix(trimmed, "safety warning:")),
			Row:    trimmed,
		}, true
	}
	if strings.HasPrefix(trimmed, "safety error:") {
		return RuntimeAction{
			Kind:   "workspace-safety-error",
			State:  "error",
			Reason: strings.TrimSpace(strings.TrimPrefix(trimmed, "safety error:")),
			Row:    trimmed,
		}, true
	}
	if strings.HasPrefix(trimmed, "compile preview:") {
		return runtimeCompilePreviewActionForRow(trimmed), true
	}
	if strings.HasPrefix(trimmed, "compile-preview signals:") {
		return runtimeCompilePreviewSignalsActionForRow(trimmed), true
	}
	if strings.HasPrefix(trimmed, "compile-preview signal domain:") {
		return runtimeCompilePreviewSignalDomainActionForRow(trimmed), true
	}
	if strings.HasPrefix(trimmed, "compile-preview signal frontier:") {
		return runtimeCompilePreviewSignalFrontierActionForRow(trimmed), true
	}
	if strings.HasPrefix(trimmed, "compile-preview signal history:") {
		return runtimeCompilePreviewSignalHistoryActionForRow(trimmed), true
	}
	if strings.HasPrefix(trimmed, "compile-preview review artifact:") {
		return runtimeCompilePreviewReviewArtifactActionForRow(trimmed), true
	}
	if strings.HasPrefix(trimmed, "compile-preview review comparison:") {
		return runtimeCompilePreviewReviewComparisonActionForRow(trimmed), true
	}
	if strings.HasPrefix(trimmed, "compile-preview review selection:") {
		return runtimeCompilePreviewReviewSelectionActionForRow(trimmed), true
	}
	if strings.HasPrefix(trimmed, "replay workbench session selection:") {
		return runtimeReplayWorkbenchSessionSelectionActionForRow(trimmed), true
	}
	if strings.HasPrefix(trimmed, "replay workbench session artifact:") {
		return runtimeReplayWorkbenchSessionArtifactActionForRow(trimmed), true
	}
	if strings.HasPrefix(trimmed, "replay workbench collaboration selection:") {
		return runtimeReplayWorkbenchCollaborationSelectionActionForRow(trimmed), true
	}
	if strings.HasPrefix(trimmed, "replay workbench collaboration note selection:") {
		return runtimeReplayWorkbenchCollaborationNoteSelectionActionForRow(trimmed), true
	}
	if strings.HasPrefix(trimmed, "replay workbench collaboration note artifact:") {
		return runtimeReplayWorkbenchCollaborationNoteActionForRow(trimmed), true
	}
	if strings.HasPrefix(trimmed, "replay workbench collaboration presence artifact:") {
		return runtimeReplayWorkbenchCollaborationPresenceActionForRow(trimmed), true
	}
	if strings.HasPrefix(trimmed, "replay workbench collaboration artifact:") {
		return runtimeReplayWorkbenchCollaborationArtifactActionForRow(trimmed), true
	}
	if !strings.HasPrefix(trimmed, "install ") {
		return RuntimeAction{}, false
	}
	installDetails := strings.TrimSpace(strings.TrimPrefix(trimmed, "install "))
	name := installDetails
	rest := ""
	if before, after, ok := strings.Cut(installDetails, ":"); ok {
		name = strings.TrimSpace(before)
		rest = strings.TrimSpace(after)
	}
	return RuntimeAction{
		Kind:      "install",
		Pack:      runtimeRowField(rest, "pack"),
		Name:      name,
		State:     firstRuntimeToken(rest),
		Command:   runtimeRowField(rest, "command"),
		Hint:      runtimeRowField(rest, "hint"),
		Source:    runtimeRowField(rest, "source"),
		Digest:    runtimeRowField(rest, "digest"),
		DependsOn: runtimeRowField(rest, "depends_on"),
		Row:       trimmed,
	}, true
}

func runtimeCompilePreviewActionForRow(row string) RuntimeAction {
	details := strings.TrimSpace(strings.TrimPrefix(row, "compile preview:"))
	return RuntimeAction{
		Kind:          "compile-preview",
		ExecutionMode: runtimeRowField(details, "mode"),
		ActionType:    runtimeRowField(details, "action"),
		Nodes:         runtimeRowField(details, "nodes"),
		Warnings:      runtimeRowField(details, "warnings"),
		Row:           row,
	}
}

func runtimeCompilePreviewSignalsActionForRow(row string) RuntimeAction {
	details := strings.TrimSpace(strings.TrimPrefix(row, "compile-preview signals:"))
	return RuntimeAction{
		Kind:             "compile-preview-signals",
		State:            firstRuntimeToken(details),
		ExecutionMode:    runtimeRowField(details, "mode"),
		ArtifactPath:     runtimeRowField(details, "path"),
		MaxPlanNodes:     runtimeRowField(details, "max_plan_nodes"),
		DomainPriorities: runtimeRowField(details, "domain_priorities"),
		FrontierScores:   runtimeRowField(details, "frontier_scores"),
		HistoricalStats:  runtimeRowField(details, "historical_stats"),
		Row:              row,
	}
}

func runtimeCompilePreviewSignalDomainActionForRow(row string) RuntimeAction {
	details := strings.TrimSpace(strings.TrimPrefix(row, "compile-preview signal domain:"))
	return RuntimeAction{
		Kind:   "compile-preview-signal-domain",
		Name:   runtimeRowField(details, "domain"),
		Weight: runtimeRowField(details, "weight"),
		Row:    row,
	}
}

func runtimeCompilePreviewSignalFrontierActionForRow(row string) RuntimeAction {
	details := strings.TrimSpace(strings.TrimPrefix(row, "compile-preview signal frontier:"))
	return RuntimeAction{
		Kind:   "compile-preview-signal-frontier",
		Pack:   runtimeRowField(details, "manifest"),
		Name:   runtimeRowField(details, "capability"),
		Source: runtimeRowField(details, "path"),
		Target: runtimeRowField(details, "target"),
		Score:  runtimeRowField(details, "score"),
		Row:    row,
	}
}

func runtimeCompilePreviewSignalHistoryActionForRow(row string) RuntimeAction {
	details := strings.TrimSpace(strings.TrimPrefix(row, "compile-preview signal history:"))
	return RuntimeAction{
		Kind:      "compile-preview-signal-history",
		Pack:      runtimeRowField(details, "manifest"),
		Name:      runtimeRowField(details, "capability"),
		Successes: runtimeRowField(details, "successes"),
		Failures:  runtimeRowField(details, "failures"),
		Row:       row,
	}
}

func runtimeCompilePreviewReviewArtifactActionForRow(row string) RuntimeAction {
	details := strings.TrimSpace(strings.TrimPrefix(row, "compile-preview review artifact:"))
	path := runtimeRowField(details, "path")
	rest := details
	if strings.TrimSpace(path) == "" {
		path = details
		rest = ""
		if before, after, ok := strings.Cut(details, " "); ok {
			path = strings.TrimSpace(before)
			rest = strings.TrimSpace(after)
		}
	}
	return RuntimeAction{
		Kind:          "compile-preview-review-artifact",
		State:         runtimeRowField(details, "status"),
		ExecutionMode: runtimeRowField(details, "mode"),
		ArtifactPath:  path,
		ActionType:    runtimeRowField(rest, "action"),
		Nodes:         runtimeRowField(rest, "nodes"),
		Warnings:      runtimeRowField(rest, "warnings"),
		Current:       runtimeRowField(rest, "current"),
		Decision:      runtimeRowField(rest, "decision"),
		Reviewer:      runtimeRowField(rest, "reviewer"),
		ReviewedAt:    runtimeRowField(rest, "reviewed_at"),
		Reason:        runtimeRowField(rest, "reason"),
		Row:           row,
	}
}

func runtimeCompilePreviewReviewComparisonActionForRow(row string) RuntimeAction {
	details := strings.TrimSpace(strings.TrimPrefix(row, "compile-preview review comparison:"))
	return RuntimeAction{
		Kind:          "compile-preview-review-comparison",
		ExecutionMode: runtimeRowField(details, "mode"),
		BasePath:      runtimeRowField(details, "base"),
		CandidatePath: runtimeRowField(details, "candidate"),
		ActionType:    runtimeRowField(details, "action"),
		NodeDelta:     runtimeRowField(details, "node_delta"),
		WarningDelta:  runtimeRowField(details, "warning_delta"),
		Added:         runtimeRowField(details, "added"),
		Removed:       runtimeRowField(details, "removed"),
		Changed:       runtimeRowField(details, "changed"),
		Reordered:     runtimeRowField(details, "reordered"),
		ScoreChanged:  runtimeRowField(details, "score_changed"),
		SignalChanged: runtimeRowField(details, "signal_changed"),
		Row:           row,
	}
}

func runtimeCompilePreviewReviewSelectionActionForRow(row string) RuntimeAction {
	details := strings.TrimSpace(strings.TrimPrefix(row, "compile-preview review selection:"))
	return RuntimeAction{
		Kind:          "compile-preview-review-selection",
		State:         runtimeRowField(details, "status"),
		ExecutionMode: runtimeRowField(details, "mode"),
		ReviewPath:    runtimeRowField(details, "review"),
		ActionType:    runtimeRowField(details, "action"),
		Superseded:    runtimeRowField(details, "superseded"),
		SelectedBy:    runtimeRowField(details, "selected_by"),
		SelectedAt:    runtimeRowField(details, "selected_at"),
		ClearedBy:     runtimeRowField(details, "cleared_by"),
		ClearedAt:     runtimeRowField(details, "cleared_at"),
		Reason:        runtimeRowField(details, "reason"),
		Row:           row,
	}
}

func runtimeReplayWorkbenchSessionArtifactActionForRow(row string) RuntimeAction {
	details := strings.TrimSpace(strings.TrimPrefix(row, "replay workbench session artifact:"))
	return RuntimeAction{
		Kind:          "replay-workbench-session-artifact",
		State:         runtimeRowField(details, "status"),
		ExecutionMode: runtimeRowField(details, "mode"),
		RunID:         runtimeRowField(details, "primary"),
		SelectedRunID: runtimeRowField(details, "selected"),
		RunIDs:        runtimeRowField(details, "runs"),
		Filters:       runtimeRowField(details, "filters"),
		Endpoints:     runtimeRowField(details, "endpoints"),
		CreatedBy:     runtimeRowField(details, "created_by"),
		CreatedAt:     runtimeRowField(details, "created_at"),
		Reason:        runtimeRowField(details, "reason"),
		Decision:      runtimeRowField(details, "decision"),
		Reviewer:      runtimeRowField(details, "reviewer"),
		ReviewedAt:    runtimeRowField(details, "reviewed_at"),
		ReviewReason:  runtimeRowField(details, "review_reason"),
		Current:       runtimeRowField(details, "current"),
		ArtifactPath:  runtimeRowField(details, "path"),
		Row:           row,
	}
}

func runtimeReplayWorkbenchCollaborationArtifactActionForRow(row string) RuntimeAction {
	details := strings.TrimSpace(strings.TrimPrefix(row, "replay workbench collaboration artifact:"))
	return RuntimeAction{
		Kind:          "replay-workbench-collaboration-artifact",
		State:         runtimeRowField(details, "status"),
		ExecutionMode: runtimeRowField(details, "mode"),
		SessionPath:   runtimeRowField(details, "session_path"),
		RunID:         runtimeRowField(details, "primary"),
		SelectedRunID: runtimeRowField(details, "selected"),
		RunIDs:        runtimeRowField(details, "runs"),
		Filters:       runtimeRowField(details, "filters"),
		Endpoints:     runtimeRowField(details, "endpoints"),
		Participants:  runtimeRowField(details, "participants"),
		CreatedBy:     runtimeRowField(details, "created_by"),
		CreatedAt:     runtimeRowField(details, "created_at"),
		Reason:        runtimeRowField(details, "reason"),
		Decision:      runtimeRowField(details, "decision"),
		Reviewer:      runtimeRowField(details, "reviewer"),
		ReviewedAt:    runtimeRowField(details, "reviewed_at"),
		ReviewReason:  runtimeRowField(details, "review_reason"),
		Current:       runtimeRowField(details, "current"),
		ArtifactPath:  runtimeRowField(details, "path"),
		Row:           row,
	}
}

func runtimeReplayWorkbenchCollaborationPresenceActionForRow(row string) RuntimeAction {
	details := strings.TrimSpace(strings.TrimPrefix(row, "replay workbench collaboration presence artifact:"))
	return RuntimeAction{
		Kind:              "replay-workbench-collaboration-presence",
		State:             runtimeRowField(details, "status"),
		ExecutionMode:     runtimeRowField(details, "mode"),
		RunID:             runtimeRowField(details, "primary"),
		SelectedRunID:     runtimeRowField(details, "selected"),
		Participant:       runtimeRowField(details, "participant"),
		SeenBy:            runtimeRowField(details, "seen_by"),
		SeenAt:            runtimeRowField(details, "seen_at"),
		Reason:            runtimeRowField(details, "reason"),
		ArtifactPath:      runtimeRowField(details, "path"),
		CollaborationPath: runtimeRowField(details, "collaboration"),
		SessionPath:       runtimeRowField(details, "session_path"),
		Row:               row,
	}
}

func runtimeReplayWorkbenchCollaborationNoteActionForRow(row string) RuntimeAction {
	details := strings.TrimSpace(strings.TrimPrefix(row, "replay workbench collaboration note artifact:"))
	return RuntimeAction{
		Kind:              "replay-workbench-collaboration-note",
		State:             runtimeRowField(details, "status"),
		ExecutionMode:     runtimeRowField(details, "mode"),
		RunID:             runtimeRowField(details, "primary"),
		SelectedRunID:     runtimeRowField(details, "selected"),
		Author:            runtimeRowField(details, "author"),
		Target:            runtimeRowField(details, "target"),
		TargetKind:        runtimeRowField(details, "target_kind"),
		TargetID:          runtimeRowField(details, "target_id"),
		CreatedAt:         runtimeRowField(details, "created_at"),
		Reason:            runtimeRowField(details, "reason"),
		Body:              runtimeRowField(details, "body"),
		Decision:          runtimeRowField(details, "decision"),
		Reviewer:          runtimeRowField(details, "reviewer"),
		ReviewedAt:        runtimeRowField(details, "reviewed_at"),
		ReviewReason:      runtimeRowField(details, "review_reason"),
		Current:           runtimeRowField(details, "current"),
		ArtifactPath:      runtimeRowField(details, "path"),
		CollaborationPath: runtimeRowField(details, "collaboration"),
		SessionPath:       runtimeRowField(details, "session_path"),
		Row:               row,
	}
}

func runtimeReplayWorkbenchCollaborationNoteSelectionActionForRow(row string) RuntimeAction {
	details := strings.TrimSpace(strings.TrimPrefix(row, "replay workbench collaboration note selection:"))
	return RuntimeAction{
		Kind:              "replay-workbench-collaboration-note-selection",
		State:             runtimeRowField(details, "status"),
		ExecutionMode:     runtimeRowField(details, "mode"),
		ArtifactPath:      runtimeRowField(details, "note"),
		CollaborationPath: runtimeRowField(details, "collaboration"),
		SessionPath:       runtimeRowField(details, "session"),
		RunID:             runtimeRowField(details, "primary"),
		SelectedRunID:     runtimeRowField(details, "selected"),
		Author:            runtimeRowField(details, "author"),
		Target:            runtimeRowField(details, "target"),
		TargetKind:        runtimeRowField(details, "target_kind"),
		TargetID:          runtimeRowField(details, "target_id"),
		Superseded:        runtimeRowField(details, "superseded"),
		SelectedBy:        runtimeRowField(details, "selected_by"),
		SelectedAt:        runtimeRowField(details, "selected_at"),
		ClearedBy:         runtimeRowField(details, "cleared_by"),
		ClearedAt:         runtimeRowField(details, "cleared_at"),
		Reason:            runtimeRowField(details, "reason"),
		Row:               row,
	}
}

func runtimeReplayWorkbenchCollaborationSelectionActionForRow(row string) RuntimeAction {
	details := strings.TrimSpace(strings.TrimPrefix(row, "replay workbench collaboration selection:"))
	return RuntimeAction{
		Kind:          "replay-workbench-collaboration-selection",
		State:         runtimeRowField(details, "status"),
		ExecutionMode: runtimeRowField(details, "mode"),
		ArtifactPath:  runtimeRowField(details, "collaboration"),
		SessionPath:   runtimeRowField(details, "session"),
		RunID:         runtimeRowField(details, "primary"),
		SelectedRunID: runtimeRowField(details, "selected"),
		RunIDs:        runtimeRowField(details, "runs"),
		Filters:       runtimeRowField(details, "filters"),
		Endpoints:     runtimeRowField(details, "endpoints"),
		Participants:  runtimeRowField(details, "participants"),
		Superseded:    runtimeRowField(details, "superseded"),
		SelectedBy:    runtimeRowField(details, "selected_by"),
		SelectedAt:    runtimeRowField(details, "selected_at"),
		ClearedBy:     runtimeRowField(details, "cleared_by"),
		ClearedAt:     runtimeRowField(details, "cleared_at"),
		Reason:        runtimeRowField(details, "reason"),
		Row:           row,
	}
}

func runtimeReplayWorkbenchSessionSelectionActionForRow(row string) RuntimeAction {
	details := strings.TrimSpace(strings.TrimPrefix(row, "replay workbench session selection:"))
	return RuntimeAction{
		Kind:          "replay-workbench-session-selection",
		State:         runtimeRowField(details, "status"),
		ExecutionMode: runtimeRowField(details, "mode"),
		ArtifactPath:  runtimeRowField(details, "session"),
		RunID:         runtimeRowField(details, "primary"),
		SelectedRunID: runtimeRowField(details, "selected"),
		RunIDs:        runtimeRowField(details, "runs"),
		Filters:       runtimeRowField(details, "filters"),
		Endpoints:     runtimeRowField(details, "endpoints"),
		Superseded:    runtimeRowField(details, "superseded"),
		SelectedBy:    runtimeRowField(details, "selected_by"),
		SelectedAt:    runtimeRowField(details, "selected_at"),
		ClearedBy:     runtimeRowField(details, "cleared_by"),
		ClearedAt:     runtimeRowField(details, "cleared_at"),
		Reason:        runtimeRowField(details, "reason"),
		Row:           row,
	}
}

func runtimeWorkspaceSafetyStrictGateActionForRow(row string) RuntimeAction {
	details := strings.TrimSpace(strings.TrimPrefix(row, "workspace safety strict gate:"))
	state := firstRuntimeToken(details)
	reason := "all disabled safety boundaries have matching authorization"
	if state == "blocked" {
		reason = "missing required safety bypass authorization"
	}
	return RuntimeAction{
		Kind:          "workspace-safety-strict-gate",
		State:         state,
		ExecutionMode: "strict",
		Failed:        runtimeRowField(details, "errors"),
		Reason:        reason,
		Row:           row,
	}
}

func runtimeExecutionArtifactActionForRow(row string) RuntimeAction {
	details := strings.TrimSpace(strings.TrimPrefix(row, "execution artifact:"))
	path := details
	rest := ""
	if before, after, ok := strings.Cut(details, " "); ok {
		path = strings.TrimSpace(before)
		rest = strings.TrimSpace(after)
	}
	return RuntimeAction{
		Kind:          "install-execution-artifact",
		State:         runtimeRowField(rest, "status"),
		ExecutionMode: runtimeRowField(rest, "mode"),
		ArtifactPath:  path,
		DispatchPath:  runtimeRowField(rest, "dispatch"),
		Results:       runtimeRowField(rest, "results"),
		Skipped:       runtimeRowField(rest, "skipped"),
		Row:           row,
	}
}

func runtimeExecutionResultActionForRow(row string) RuntimeAction {
	details := strings.TrimSpace(strings.TrimPrefix(row, "execution result "))
	name := details
	rest := ""
	if before, after, ok := strings.Cut(details, ":"); ok {
		name = strings.TrimSpace(before)
		rest = strings.TrimSpace(after)
	}
	pack := ""
	dependency := name
	if before, after, ok := strings.Cut(name, "/"); ok {
		pack = strings.TrimSpace(before)
		dependency = strings.TrimSpace(after)
	}
	return RuntimeAction{
		Kind:         "install-execution-result",
		Pack:         pack,
		Name:         dependency,
		State:        firstRuntimeToken(rest),
		Command:      runtimeRowField(rest, "command"),
		Output:       runtimeRowField(rest, "output"),
		ArtifactPath: runtimeRowField(rest, "artifact"),
		DispatchPath: runtimeRowField(rest, "dispatch"),
		Row:          row,
	}
}

func runtimeDispatchArtifactActionForRow(row string) RuntimeAction {
	details := strings.TrimSpace(strings.TrimPrefix(row, "dispatch artifact:"))
	path := details
	rest := ""
	if before, after, ok := strings.Cut(details, " "); ok {
		path = strings.TrimSpace(before)
		rest = strings.TrimSpace(after)
	}
	return RuntimeAction{
		Kind:          "install-dispatch-artifact",
		State:         runtimeRowField(rest, "status"),
		ExecutionMode: runtimeRowField(rest, "mode"),
		ArtifactPath:  path,
		Ready:         runtimeRowField(rest, "ready"),
		Skipped:       runtimeRowField(rest, "skipped"),
		Row:           row,
	}
}

func runtimeRetryDispatchArtifactActionForRow(row string) RuntimeAction {
	details := strings.TrimSpace(strings.TrimPrefix(row, "retry dispatch artifact:"))
	path := details
	rest := ""
	if before, after, ok := strings.Cut(details, " "); ok {
		path = strings.TrimSpace(before)
		rest = strings.TrimSpace(after)
	}
	return RuntimeAction{
		Kind:          "install-retry-dispatch-artifact",
		State:         runtimeRowField(rest, "status"),
		ExecutionMode: runtimeRowField(rest, "mode"),
		ArtifactPath:  path,
		Ready:         runtimeRowField(rest, "ready"),
		Skipped:       runtimeRowField(rest, "skipped"),
		Row:           row,
	}
}

func runtimeInstallOrchestrationArtifactActionForRow(row string) RuntimeAction {
	details := strings.TrimSpace(strings.TrimPrefix(row, "install orchestration artifact:"))
	path := details
	rest := ""
	if before, after, ok := strings.Cut(details, " "); ok {
		path = strings.TrimSpace(before)
		rest = strings.TrimSpace(after)
	}
	return RuntimeAction{
		Kind:              "install-orchestration-artifact",
		State:             runtimeRowField(rest, "status"),
		ExecutionMode:     runtimeRowField(rest, "mode"),
		ArtifactPath:      path,
		Ready:             runtimeRowField(rest, "ready"),
		Skipped:           runtimeRowField(rest, "skipped"),
		Results:           runtimeRowField(rest, "results"),
		DispatchPath:      runtimeRowField(rest, "dispatch"),
		ExecutionPath:     runtimeRowField(rest, "execution"),
		RetryDispatchPath: runtimeRowField(rest, "retry_dispatch"),
		ExecutionError:    runtimeRowField(rest, "error"),
		Row:               row,
	}
}

func runtimeWorkerOrchestrationArtifactActionForRow(row string) RuntimeAction {
	details := strings.TrimSpace(strings.TrimPrefix(row, "worker orchestration artifact:"))
	path := details
	rest := ""
	if before, after, ok := strings.Cut(details, " "); ok {
		path = strings.TrimSpace(before)
		rest = strings.TrimSpace(after)
	}
	return RuntimeAction{
		Kind:                       "worker-orchestration-artifact",
		State:                      runtimeRowField(rest, "status"),
		ExecutionMode:              runtimeRowField(rest, "mode"),
		TransportMode:              runtimeRowField(rest, "transport"),
		ArtifactPath:               path,
		Ready:                      runtimeRowField(rest, "placement_ready"),
		Skipped:                    runtimeRowField(rest, "placement_skipped"),
		Claimed:                    runtimeRowField(rest, "claimed"),
		Results:                    runtimeRowField(rest, "dispatched"),
		Failed:                     runtimeRowField(rest, "failed"),
		PlacementPlanPath:          runtimeRowField(rest, "placement_plan"),
		PlacementDispatchPlanPath:  runtimeRowField(rest, "placement_dispatch"),
		PlacementDispatchBatchPath: runtimeRowField(rest, "placement_batch"),
		LifecycleMaintenancePath:   runtimeRowField(rest, "lifecycle_maintenance"),
		LifecycleAuditPath:         runtimeRowField(rest, "lifecycle_audit"),
		Row:                        row,
	}
}

func runtimeWorkerOrchestrationSupervisorArtifactActionForRow(row string) RuntimeAction {
	details := strings.TrimSpace(strings.TrimPrefix(row, "worker orchestration supervisor artifact:"))
	path := details
	rest := ""
	if before, after, ok := strings.Cut(details, " "); ok {
		path = strings.TrimSpace(before)
		rest = strings.TrimSpace(after)
	}
	results := []string{}
	for _, key := range []string{"max_runs", "orchestrations_created", "lifecycle_maintenance", "lifecycle_audit", "retry_dispatch_executions_created", "retry_dispatch_claims_created", "retry_dispatch_claims_dispatched", "retry_dispatch_transport_requests_created", "dispatch_lifecycle_retry_claims", "lifecycle_retry_transport", "claimed", "dispatched"} {
		if value := runtimeRowField(rest, key); value != "" {
			results = append(results, key+"="+value)
		}
	}
	return RuntimeAction{
		Kind:          "worker-orchestration-supervisor-artifact",
		State:         runtimeRowField(rest, "status"),
		ExecutionMode: runtimeRowField(rest, "mode"),
		TransportMode: runtimeRowField(rest, "transport"),
		ArtifactPath:  path,
		Ready:         runtimeRowField(rest, "runs"),
		Results:       strings.Join(results, " "),
		Skipped:       runtimeRowField(rest, "skipped"),
		Failed:        runtimeRowField(rest, "failed"),
		DispatchPath:  runtimeOptionalRowField(rest, "orchestration_path"),
		Row:           row,
	}
}

func runtimeWorkerLifecycleReviewArtifactActionForRow(row string) RuntimeAction {
	details := strings.TrimSpace(strings.TrimPrefix(row, "worker lifecycle review artifact:"))
	path := details
	rest := ""
	if before, after, ok := strings.Cut(details, " "); ok {
		path = strings.TrimSpace(before)
		rest = strings.TrimSpace(after)
	}
	return RuntimeAction{
		Kind:          "worker-lifecycle-review-artifact",
		State:         runtimeRowField(rest, "status"),
		ExecutionMode: runtimeRowField(rest, "mode"),
		ArtifactPath:  path,
		Ready:         runtimeRowField(rest, "reclaim_ready"),
		Skipped:       runtimeRowField(rest, "reclaim_skipped"),
		Results:       runtimeRowField(rest, "reconciliation_ready"),
		Row:           row,
	}
}

func runtimeWorkerLifecycleAuditArtifactActionForRow(row string) RuntimeAction {
	details := strings.TrimSpace(strings.TrimPrefix(row, "worker lifecycle audit artifact:"))
	path := details
	rest := ""
	if before, after, ok := strings.Cut(details, " "); ok {
		path = strings.TrimSpace(before)
		rest = strings.TrimSpace(after)
	}
	ready := strings.Join(nonEmptyRuntimeParts([]string{
		"workers=" + runtimeRowField(rest, "workers"),
		"stale_workers=" + runtimeRowField(rest, "stale_workers"),
		"placements=" + runtimeRowField(rest, "placements"),
	}), " ")
	results := strings.Join(nonEmptyRuntimeParts([]string{
		"claims=" + runtimeRowField(rest, "claims"),
		"dispatches=" + runtimeRowField(rest, "dispatches"),
		"requests=" + runtimeRowField(rest, "requests"),
		"results=" + runtimeRowField(rest, "results"),
		"retry_plans=" + runtimeRowField(rest, "retry_plans"),
		"retry_dispatches=" + runtimeRowField(rest, "retry_dispatches"),
		"retry_dispatch_executions=" + runtimeRowField(rest, "retry_dispatch_executions"),
		"leases=" + runtimeRowField(rest, "leases"),
	}), " ")
	skipped := strings.Join(nonEmptyRuntimeParts([]string{
		"reclaims=" + runtimeRowField(rest, "reclaims"),
		"reconciliations=" + runtimeRowField(rest, "reconciliations"),
	}), " ")
	claimed := strings.Join(nonEmptyRuntimeParts([]string{
		"lifecycle_policies=" + runtimeRowField(rest, "lifecycle_policies"),
		"policy_selections=" + runtimeRowFieldWithFallback(rest, "policy_selections", "lifecycle_policy_selections"),
		"policy_runs=" + runtimeRowFieldWithFallback(rest, "policy_runs", "lifecycle_policy_runs"),
		"policy_scheduler_runs=" + runtimeRowFieldWithFallback(rest, "policy_scheduler_runs", "lifecycle_policy_scheduler_runs"),
		"lifecycle_reviews=" + runtimeRowField(rest, "lifecycle_reviews"),
		"action_plans=" + runtimeRowField(rest, "action_plans"),
		"maintenances=" + runtimeRowField(rest, "maintenances"),
		"maintenance_supervisors=" + runtimeRowField(rest, "maintenance_supervisors"),
		"orchestration_supervisors=" + runtimeRowField(rest, "orchestration_supervisors"),
		"actions=" + runtimeRowField(rest, "actions"),
	}), " ")
	failed := strings.Join(nonEmptyRuntimeParts([]string{
		"daemon_runs=" + runtimeRowField(rest, "daemon_runs"),
		"supervisor_runs=" + runtimeRowField(rest, "supervisor_runs"),
		"controls=" + runtimeRowField(rest, "controls"),
	}), " ")
	return RuntimeAction{
		Kind:          "worker-lifecycle-audit-artifact",
		State:         runtimeRowField(rest, "status"),
		ExecutionMode: runtimeRowField(rest, "mode"),
		ArtifactPath:  path,
		Ready:         ready,
		Results:       results,
		Skipped:       skipped,
		Claimed:       claimed,
		Failed:        failed,
		Row:           row,
	}
}

func runtimeWorkerLifecycleAuditItemActionForRow(row string) RuntimeAction {
	rest := strings.TrimSpace(strings.TrimPrefix(row, "worker lifecycle audit item:"))
	results := strings.Join(nonEmptyRuntimeParts([]string{
		"total=" + runtimeRowField(rest, "total"),
		"ready=" + runtimeRowField(rest, "ready"),
		"results=" + runtimeRowField(rest, "results"),
		"actions=" + runtimeRowField(rest, "actions"),
		"ack_actions=" + runtimeRowField(rest, "ack_actions"),
		"run_actions=" + runtimeRowField(rest, "run_actions"),
		"daemon_control_actions=" + runtimeRowField(rest, "daemon_control_actions"),
		"retry_dispatch_execute=" + runtimeRowField(rest, "retry_dispatch_execute"),
		"reclaim_actions=" + runtimeRowField(rest, "reclaim_actions"),
		"reconciliation_actions=" + runtimeRowField(rest, "reconciliation_actions"),
		"placement_ready=" + runtimeRowField(rest, "placement_ready"),
		"placement_skipped=" + runtimeRowField(rest, "placement_skipped"),
		"dispatch_ready=" + runtimeRowField(rest, "dispatch_ready"),
		"dispatch_skipped=" + runtimeRowField(rest, "dispatch_skipped"),
		"runs=" + runtimeRowField(rest, "runs"),
		"max_runs=" + runtimeRowField(rest, "max_runs"),
		"orchestrations_created=" + runtimeRowField(rest, "orchestrations_created"),
		"lifecycle_maintenance=" + runtimeRowField(rest, "lifecycle_maintenance"),
		"lifecycle_audit=" + runtimeRowField(rest, "lifecycle_audit"),
		"claimed=" + runtimeRowField(rest, "claimed"),
		"dispatched=" + runtimeRowField(rest, "dispatched"),
	}), " ")
	skipped := strings.Join(nonEmptyRuntimeParts([]string{
		"skipped_unsafe=" + runtimeRowField(rest, "skipped_unsafe"),
		"skipped=" + runtimeRowField(rest, "skipped"),
	}), " ")
	failed := strings.Join(nonEmptyRuntimeParts([]string{
		"failed=" + runtimeRowField(rest, "failed"),
	}), " ")
	return RuntimeAction{
		Kind:          "worker-lifecycle-audit-item",
		Name:          runtimeRowField(rest, "kind"),
		State:         runtimeRowField(rest, "status"),
		ExecutionMode: runtimeOptionalRowField(rest, "mode"),
		Gate:          runtimeOptionalRowField(rest, "gate"),
		WorkerID:      runtimeOptionalRowField(rest, "worker"),
		RunID:         runtimeOptionalRowField(rest, "run"),
		ExecutionID:   runtimeOptionalRowField(rest, "execution"),
		Reason:        runtimeOptionalRowField(rest, "reason"),
		ArtifactPath:  runtimeOptionalRowField(rest, "path"),
		DispatchPath:  runtimeOptionalRowField(rest, "linked"),
		Decision:      runtimeRowField(rest, "decision"),
		Reviewer:      runtimeRowField(rest, "reviewer"),
		ReviewedAt:    runtimeRowField(rest, "reviewed_at"),
		SelectedBy:    runtimeRowField(rest, "selected_by"),
		SelectedAt:    runtimeRowField(rest, "selected_at"),
		ClearedBy:     runtimeRowField(rest, "cleared_by"),
		ClearedAt:     runtimeRowField(rest, "cleared_at"),
		Results:       results,
		Skipped:       skipped,
		Failed:        failed,
		Row:           row,
	}
}

func runtimeWorkerLifecycleActionPlanArtifactActionForRow(row string) RuntimeAction {
	details := strings.TrimSpace(strings.TrimPrefix(row, "worker lifecycle action plan artifact:"))
	path := details
	rest := ""
	if before, after, ok := strings.Cut(details, " "); ok {
		path = strings.TrimSpace(before)
		rest = strings.TrimSpace(after)
	}
	results := []string{}
	for _, key := range []string{"ack", "run", "daemon_control", "retry_dispatch_execute", "reclaim", "reconcile"} {
		if value := runtimeRowField(rest, key); value != "" {
			results = append(results, key+"="+value)
		}
	}
	return RuntimeAction{
		Kind:          "worker-lifecycle-action-plan-artifact",
		State:         runtimeRowField(rest, "status"),
		ExecutionMode: runtimeRowField(rest, "mode"),
		ArtifactPath:  path,
		ReviewPath:    runtimeRowField(rest, "review"),
		Ready:         runtimeRowField(rest, "actions"),
		Results:       strings.Join(results, " "),
		Current:       runtimeRowField(rest, "current"),
		Decision:      runtimeRowField(rest, "decision"),
		Reviewer:      runtimeRowField(rest, "reviewer"),
		ReviewedAt:    runtimeRowField(rest, "reviewed_at"),
		ReviewReason:  runtimeRowField(rest, "review_reason"),
		Row:           row,
	}
}

func runtimeWorkerLifecycleActionPlanSelectionActionForRow(row string) RuntimeAction {
	details := strings.TrimSpace(strings.TrimPrefix(row, "worker lifecycle action plan selection:"))
	results := []string{}
	for _, key := range []string{"ack", "run", "daemon_control", "retry_dispatch_execute", "reclaim", "reconcile"} {
		if value := runtimeRowField(details, key); value != "" {
			results = append(results, key+"="+value)
		}
	}
	return RuntimeAction{
		Kind:          "worker-lifecycle-action-plan-selection",
		State:         runtimeRowField(details, "status"),
		ExecutionMode: runtimeRowField(details, "mode"),
		ArtifactPath:  runtimeRowField(details, "plan"),
		RequestPath:   runtimeRowField(details, "selection"),
		ReviewPath:    runtimeRowField(details, "review"),
		Current:       runtimeRowField(details, "current"),
		Ready:         runtimeRowField(details, "actions"),
		Results:       strings.Join(results, " "),
		Superseded:    runtimeRowField(details, "superseded"),
		SelectedBy:    runtimeRowField(details, "selected_by"),
		SelectedAt:    runtimeRowField(details, "selected_at"),
		ClearedBy:     runtimeRowField(details, "cleared_by"),
		ClearedAt:     runtimeRowField(details, "cleared_at"),
		Reason:        runtimeRowField(details, "reason"),
		Row:           row,
	}
}

func runtimeWorkerLifecycleMaintenanceArtifactActionForRow(row string) RuntimeAction {
	details := strings.TrimSpace(strings.TrimPrefix(row, "worker lifecycle maintenance artifact:"))
	path := details
	rest := ""
	if before, after, ok := strings.Cut(details, " "); ok {
		path = strings.TrimSpace(before)
		rest = strings.TrimSpace(after)
	}
	results := []string{}
	for _, key := range []string{"ack", "run", "daemon_control", "retry_dispatch_execute", "reclaim", "reconcile", "review_created", "action_plan_created", "reclaim_created", "reconciliation_created", "retry_dispatch_executions_created", "retry_dispatch_claims_created", "retry_dispatch_claims_dispatched", "retry_dispatch_transport_requests_created", "dispatch_lifecycle_retry_claims", "lifecycle_retry_transport", "reclaim_executions_created", "reclaim_results_created", "reconciliation_executions_created", "reconciliation_results_created", "execute_lease_recovery"} {
		if value := runtimeRowField(rest, key); value != "" {
			results = append(results, key+"="+value)
		}
	}
	return RuntimeAction{
		Kind:          "worker-lifecycle-maintenance-artifact",
		State:         runtimeRowField(rest, "status"),
		ExecutionMode: runtimeRowField(rest, "mode"),
		ArtifactPath:  path,
		Ready:         runtimeRowField(rest, "actions"),
		Skipped:       runtimeRowField(rest, "skipped_unsafe"),
		DispatchPath:  runtimeOptionalRowFieldWithFallback(rest, "review_path", "review"),
		RequestPath:   runtimeOptionalRowFieldWithFallback(rest, "action_plan_path", "action_plan"),
		LeasePath:     runtimeOptionalRowFieldWithFallback(rest, "reclaim_path", "reclaim"),
		ResultPath:    runtimeOptionalRowFieldWithFallback(rest, "reconciliation_path", "reconciliation"),
		Results:       strings.Join(results, " "),
		Row:           row,
	}
}

func runtimeWorkerLifecycleMaintenanceSupervisorArtifactActionForRow(row string) RuntimeAction {
	details := strings.TrimSpace(strings.TrimPrefix(row, "worker lifecycle maintenance supervisor artifact:"))
	path := details
	rest := ""
	if before, after, ok := strings.Cut(details, " "); ok {
		path = strings.TrimSpace(before)
		rest = strings.TrimSpace(after)
	}
	results := []string{}
	for _, key := range []string{"max_runs", "actions", "skipped_unsafe", "retry_dispatch_execute", "maintenance_created", "audit_created", "retry_dispatch_executions_created", "retry_dispatch_claims_created", "retry_dispatch_claims_dispatched", "retry_dispatch_transport_requests_created", "dispatch_lifecycle_retry_claims", "lifecycle_retry_transport", "reclaim_executions_created", "reclaim_results_created", "reconciliation_executions_created", "reconciliation_results_created", "execute_lease_recovery"} {
		if value := runtimeRowField(rest, key); value != "" {
			results = append(results, key+"="+value)
		}
	}
	return RuntimeAction{
		Kind:          "worker-lifecycle-maintenance-supervisor-artifact",
		State:         runtimeRowField(rest, "status"),
		ExecutionMode: runtimeRowField(rest, "mode"),
		ArtifactPath:  path,
		Ready:         runtimeRowField(rest, "runs"),
		DispatchPath:  runtimeOptionalRowField(rest, "maintenance_path"),
		ResultPath:    runtimeOptionalRowField(rest, "audit_path"),
		Results:       strings.Join(results, " "),
		Row:           row,
	}
}

func runtimeWorkerLifecyclePolicyArtifactActionForRow(row string) RuntimeAction {
	details := strings.TrimSpace(strings.TrimPrefix(row, "worker lifecycle policy artifact:"))
	path := details
	rest := ""
	if before, after, ok := strings.Cut(details, " "); ok {
		path = strings.TrimSpace(before)
		rest = strings.TrimSpace(after)
	}
	ready := strings.Join(nonEmptyRuntimeParts([]string{
		"workers=" + runtimeRowField(rest, "workers"),
		"stale_workers=" + runtimeRowField(rest, "stale_workers"),
		"stale_leases=" + runtimeRowField(rest, "stale_leases"),
	}), " ")
	pollInterval := runtimeRowField(rest, "poll_interval_ms")
	pollIntervalField := "poll_interval_ms"
	if pollInterval == "" {
		pollInterval = runtimeRowField(rest, "poll_interval")
		pollIntervalField = "poll_interval"
	}
	results := strings.Join(nonEmptyRuntimeParts([]string{
		"stale_after=" + runtimeRowField(rest, "stale_after"),
		"worker_stale_after=" + runtimeRowField(rest, "worker_stale_after"),
		pollIntervalField + "=" + pollInterval,
		"max_runs=" + runtimeRowField(rest, "max_runs"),
		"lifecycle_maintenance=" + runtimeRowField(rest, "lifecycle_maintenance"),
		"lifecycle_retries=" + runtimeRowField(rest, "lifecycle_retries"),
		"dispatch_lifecycle_retry_claims=" + runtimeRowField(rest, "dispatch_lifecycle_retry_claims"),
		"lifecycle_retry_transport=" + runtimeRowField(rest, "lifecycle_retry_transport"),
		"lease_recovery=" + runtimeRowField(rest, "lease_recovery"),
	}), " ")
	return RuntimeAction{
		Kind:          "worker-lifecycle-policy-artifact",
		State:         runtimeRowField(rest, "status"),
		ExecutionMode: runtimeRowField(rest, "mode"),
		TransportMode: runtimeRowField(rest, "transport"),
		ArtifactPath:  path,
		Current:       runtimeRowField(rest, "current"),
		Ready:         ready,
		Results:       results,
		Row:           row,
	}
}

func runtimeWorkerLifecyclePolicyRunArtifactActionForRow(row string) RuntimeAction {
	details := strings.TrimSpace(strings.TrimPrefix(row, "worker lifecycle policy run artifact:"))
	path := details
	rest := ""
	if before, after, ok := strings.Cut(details, " "); ok {
		path = strings.TrimSpace(before)
		rest = strings.TrimSpace(after)
	}
	results := strings.Join(nonEmptyRuntimeParts([]string{
		"max_runs=" + runtimeRowField(rest, "max_runs"),
		"poll_interval_ms=" + runtimeRowField(rest, "poll_interval_ms"),
		"include_lifecycle_maintenance=" + runtimeRowField(rest, "include_lifecycle_maintenance"),
		"execute_lifecycle_retries=" + runtimeRowField(rest, "execute_lifecycle_retries"),
		"dispatch_lifecycle_retry_claims=" + runtimeRowField(rest, "dispatch_lifecycle_retry_claims"),
		"lifecycle_retry_transport=" + runtimeRowField(rest, "lifecycle_retry_transport"),
		"retry_dispatch_claims_dispatched=" + runtimeRowField(rest, "retry_dispatch_claims_dispatched"),
		"retry_dispatch_transport_requests_created=" + runtimeRowField(rest, "retry_dispatch_transport_requests_created"),
		"execute_lease_recovery=" + runtimeRowField(rest, "execute_lease_recovery"),
		"supervisor_created=" + runtimeRowField(rest, "supervisor_created"),
	}), " ")
	return RuntimeAction{
		Kind:          "worker-lifecycle-policy-run-artifact",
		State:         runtimeRowField(rest, "status"),
		ExecutionMode: runtimeRowField(rest, "mode"),
		ArtifactPath:  path,
		Source:        runtimeRowField(rest, "policy"),
		RequestPath:   runtimeRowField(rest, "selection"),
		TransportMode: runtimeRowField(rest, "transport"),
		Results:       results,
		Skipped:       runtimeRowField(rest, "skip"),
		Row:           row,
	}
}

func runtimeWorkerLifecyclePolicySchedulerRunArtifactActionForRow(row string) RuntimeAction {
	details := strings.TrimSpace(strings.TrimPrefix(row, "worker lifecycle policy scheduler run artifact:"))
	path := details
	rest := ""
	if before, after, ok := strings.Cut(details, " "); ok {
		path = strings.TrimSpace(before)
		rest = strings.TrimSpace(after)
	}
	results := strings.Join(nonEmptyRuntimeParts([]string{
		"max_runs=" + runtimeRowField(rest, "max_runs"),
		"poll_interval_ms=" + runtimeRowField(rest, "poll_interval_ms"),
		"policy_runs_created=" + runtimeRowField(rest, "policy_runs_created"),
		"supervisor_created=" + runtimeRowField(rest, "supervisor_created"),
		"include_lifecycle_maintenance=" + runtimeRowField(rest, "include_lifecycle_maintenance"),
		"execute_lifecycle_retries=" + runtimeRowField(rest, "execute_lifecycle_retries"),
		"dispatch_lifecycle_retry_claims=" + runtimeRowField(rest, "dispatch_lifecycle_retry_claims"),
		"lifecycle_retry_transport=" + runtimeRowField(rest, "lifecycle_retry_transport"),
		"execute_lease_recovery=" + runtimeRowField(rest, "execute_lease_recovery"),
	}), " ")
	return RuntimeAction{
		Kind:          "worker-lifecycle-policy-scheduler-run-artifact",
		Name:          runtimeRowField(rest, "policy_basename"),
		State:         runtimeRowField(rest, "status"),
		ExecutionMode: runtimeRowField(rest, "mode"),
		ArtifactPath:  path,
		Source:        runtimeRowField(rest, "policy"),
		Target:        runtimeRowField(rest, "selection_basename"),
		RequestPath:   runtimeRowField(rest, "selection"),
		TransportMode: runtimeRowField(rest, "transport"),
		Results:       results,
		Skipped:       runtimeOptionalRowField(rest, "skip"),
		Failed:        runtimeOptionalRowField(rest, "error"),
		Row:           row,
	}
}

func runtimeWorkerLifecyclePolicySelectionActionForRow(row string) RuntimeAction {
	details := strings.TrimSpace(strings.TrimPrefix(row, "worker lifecycle policy selection:"))
	results := strings.Join(nonEmptyRuntimeParts([]string{
		"poll_interval_ms=" + runtimeRowField(details, "poll_interval_ms"),
		"max_runs=" + runtimeRowField(details, "max_runs"),
		"dispatch_lifecycle_retry_claims=" + runtimeRowField(details, "dispatch_lifecycle_retry_claims"),
		"lifecycle_retry_transport=" + runtimeRowField(details, "lifecycle_retry_transport"),
	}), " ")
	return RuntimeAction{
		Kind:          "worker-lifecycle-policy-selection",
		State:         runtimeRowField(details, "status"),
		ExecutionMode: runtimeRowField(details, "mode"),
		ArtifactPath:  runtimeRowField(details, "policy"),
		RequestPath:   runtimeRowField(details, "selection"),
		TransportMode: runtimeRowField(details, "transport"),
		Results:       results,
		Superseded:    runtimeRowField(details, "superseded"),
		SelectedBy:    runtimeRowField(details, "selected_by"),
		SelectedAt:    runtimeRowField(details, "selected_at"),
		ClearedBy:     runtimeRowField(details, "cleared_by"),
		ClearedAt:     runtimeRowField(details, "cleared_at"),
		Reason:        runtimeRowField(details, "reason"),
		Row:           row,
	}
}

func runtimeWorkerLifecycleActionForRow(row string) RuntimeAction {
	rest := strings.TrimSpace(strings.TrimPrefix(row, "worker lifecycle action:"))
	return RuntimeAction{
		Kind:              "worker-lifecycle-action",
		Name:              runtimeRowField(rest, "type"),
		Gate:              runtimeRowField(rest, "gate"),
		State:             runtimeRowField(rest, "status"),
		ExecutionMode:     runtimeRowField(rest, "mode"),
		WorkerID:          runtimeRowField(rest, "worker"),
		RunID:             runtimeRowField(rest, "run"),
		ExecutionID:       runtimeRowField(rest, "execution"),
		Reason:            runtimeRowField(rest, "reason"),
		RequestPath:       runtimeOptionalRowField(rest, "request_path"),
		LeasePath:         runtimeOptionalRowField(rest, "lease_path"),
		ResultPath:        runtimeOptionalRowField(rest, "result_path"),
		DispatchPath:      runtimeOptionalRowField(rest, "dispatch"),
		RecommendedStatus: runtimeOptionalRowField(rest, "recommended"),
		Row:               row,
	}
}

func runtimeWorkerLeaseResultReconciliationArtifactActionForRow(row string) RuntimeAction {
	details := strings.TrimSpace(strings.TrimPrefix(row, "worker lease result reconciliation artifact:"))
	path := details
	rest := ""
	if before, after, ok := strings.Cut(details, " "); ok {
		path = strings.TrimSpace(before)
		rest = strings.TrimSpace(after)
	}
	return RuntimeAction{
		Kind:          "worker-lease-result-reconciliation-artifact",
		State:         runtimeRowField(rest, "status"),
		ExecutionMode: runtimeRowField(rest, "mode"),
		ArtifactPath:  path,
		Ready:         runtimeRowField(rest, "ready"),
		Skipped:       runtimeRowField(rest, "skipped"),
		Row:           row,
	}
}

func runtimeWorkerLeaseResultReconciliationExecutionArtifactActionForRow(row string) RuntimeAction {
	details := strings.TrimSpace(strings.TrimPrefix(row, "worker lease result reconciliation execution artifact:"))
	path := details
	rest := ""
	if before, after, ok := strings.Cut(details, " "); ok {
		path = strings.TrimSpace(before)
		rest = strings.TrimSpace(after)
	}
	return RuntimeAction{
		Kind:          "worker-lease-result-reconciliation-execution-artifact",
		State:         runtimeRowField(rest, "status"),
		ExecutionMode: runtimeRowField(rest, "mode"),
		ArtifactPath:  path,
		Results:       runtimeRowField(rest, "results"),
		Skipped:       runtimeRowField(rest, "skipped"),
		Row:           row,
	}
}

func runtimeWorkerLeaseResultReconciliationExecutionResultActionForRow(row string) RuntimeAction {
	details := strings.TrimSpace(strings.TrimPrefix(row, "lease result reconciliation execution result "))
	name := strings.TrimSpace(runtimeRowField(" "+details, "run"))
	executionID := strings.TrimSpace(runtimeRowField(" "+details, "execution"))
	if name != "" && executionID != "" {
		name += "/" + executionID
	}
	if name == "" {
		name = details
	}
	return RuntimeAction{
		Kind:   "worker-lease-result-reconciliation-execution-result",
		Name:   name,
		State:  runtimeRowField(" "+details, "lease"),
		Output: runtimeRowField(" "+details, "result"),
		Source: runtimeRowField(" "+details, "worker"),
		Row:    row,
	}
}

func runtimeWorkerLeaseReclaimArtifactActionForRow(row string) RuntimeAction {
	details := strings.TrimSpace(strings.TrimPrefix(row, "worker lease reclaim artifact:"))
	path := details
	rest := ""
	if before, after, ok := strings.Cut(details, " "); ok {
		path = strings.TrimSpace(before)
		rest = strings.TrimSpace(after)
	}
	return RuntimeAction{
		Kind:          "worker-lease-reclaim-artifact",
		State:         runtimeRowField(rest, "status"),
		ExecutionMode: runtimeRowField(rest, "mode"),
		ArtifactPath:  path,
		Ready:         runtimeRowField(rest, "ready"),
		Skipped:       runtimeRowField(rest, "skipped"),
		Row:           row,
	}
}

func runtimeWorkerLeaseReclaimExecutionArtifactActionForRow(row string) RuntimeAction {
	details := strings.TrimSpace(strings.TrimPrefix(row, "worker lease reclaim execution artifact:"))
	path := details
	rest := ""
	if before, after, ok := strings.Cut(details, " "); ok {
		path = strings.TrimSpace(before)
		rest = strings.TrimSpace(after)
	}
	return RuntimeAction{
		Kind:          "worker-lease-reclaim-execution-artifact",
		State:         runtimeRowField(rest, "status"),
		ExecutionMode: runtimeRowField(rest, "mode"),
		ArtifactPath:  path,
		DispatchPath:  runtimeRowField(rest, "dispatch"),
		Results:       runtimeRowField(rest, "results"),
		Skipped:       runtimeRowField(rest, "skipped"),
		Row:           row,
	}
}

func runtimeWorkerLeaseReclaimExecutionResultActionForRow(row string) RuntimeAction {
	details := strings.TrimSpace(strings.TrimPrefix(row, "worker lease reclaim execution result "))
	name := details
	rest := ""
	if before, after, ok := strings.Cut(details, " "); ok {
		name = strings.TrimSpace(before)
		rest = strings.TrimSpace(after)
	}
	return RuntimeAction{
		Kind:  "worker-lease-reclaim-execution-result",
		Name:  name,
		State: runtimeRowField(rest, "status"),
		Row:   row,
	}
}

func runtimeWorkerQueueClaimArtifactActionForRow(row string) RuntimeAction {
	details := strings.TrimSpace(strings.TrimPrefix(row, "worker queue claim artifact:"))
	path := details
	rest := ""
	if before, after, ok := strings.Cut(details, " "); ok {
		path = strings.TrimSpace(before)
		rest = strings.TrimSpace(after)
	}
	return RuntimeAction{
		Kind:          "worker-queue-claim-artifact",
		State:         runtimeRowField(rest, "status"),
		ExecutionMode: runtimeRowField(rest, "mode"),
		ArtifactPath:  path,
		Name:          runtimeRowField(rest, "execution"),
		Row:           row,
	}
}

func runtimeWorkerPlacementArtifactActionForRow(row string) RuntimeAction {
	details := strings.TrimSpace(strings.TrimPrefix(row, "worker placement artifact:"))
	path := details
	rest := ""
	if before, after, ok := strings.Cut(details, " "); ok {
		path = strings.TrimSpace(before)
		rest = strings.TrimSpace(after)
	}
	return RuntimeAction{
		Kind:          "worker-placement-artifact",
		State:         runtimeRowField(rest, "status"),
		ExecutionMode: runtimeRowField(rest, "mode"),
		ArtifactPath:  path,
		Ready:         runtimeRowField(rest, "ready"),
		Skipped:       runtimeRowField(rest, "skipped"),
		Row:           row,
	}
}

func runtimeWorkerPlacementDispatchArtifactActionForRow(row string) RuntimeAction {
	details := strings.TrimSpace(strings.TrimPrefix(row, "worker placement dispatch artifact:"))
	path := details
	rest := ""
	if before, after, ok := strings.Cut(details, " "); ok {
		path = strings.TrimSpace(before)
		rest = strings.TrimSpace(after)
	}
	return RuntimeAction{
		Kind:          "worker-placement-dispatch-artifact",
		State:         runtimeRowField(rest, "status"),
		ExecutionMode: runtimeRowField(rest, "mode"),
		ArtifactPath:  path,
		Ready:         runtimeRowField(rest, "ready"),
		Skipped:       runtimeRowField(rest, "skipped"),
		Source:        runtimeRowField(rest, "transport"),
		Row:           row,
	}
}

func runtimeWorkerQueueDispatchExecutionArtifactActionForRow(row string) RuntimeAction {
	details := strings.TrimSpace(strings.TrimPrefix(row, "worker queue dispatch execution artifact:"))
	path := details
	rest := ""
	if before, after, ok := strings.Cut(details, " "); ok {
		path = strings.TrimSpace(before)
		rest = strings.TrimSpace(after)
	}
	return RuntimeAction{
		Kind:          "worker-queue-dispatch-execution-artifact",
		State:         runtimeRowField(rest, "status"),
		ExecutionMode: runtimeRowField(rest, "mode"),
		ArtifactPath:  path,
		DispatchPath:  runtimeRowField(rest, "claim"),
		Name:          runtimeRowField(rest, "execution"),
		Row:           row,
	}
}

func runtimeWorkerQueueDispatchResultArtifactActionForRow(row string) RuntimeAction {
	details := strings.TrimSpace(strings.TrimPrefix(row, "worker queue dispatch result artifact:"))
	path := details
	rest := ""
	if before, after, ok := strings.Cut(details, " "); ok {
		path = strings.TrimSpace(before)
		rest = strings.TrimSpace(after)
	}
	return RuntimeAction{
		Kind:          "worker-queue-dispatch-result-artifact",
		State:         runtimeRowField(rest, "status"),
		ExecutionMode: runtimeRowField(rest, "mode"),
		ArtifactPath:  path,
		DispatchPath:  runtimeRowField(rest, "dispatch"),
		Source:        runtimeRowField(rest, "worker"),
		Name:          runtimeRowField(rest, "execution"),
		Row:           row,
	}
}

func runtimeWorkerDispatchRetryPlanArtifactActionForRow(row string) RuntimeAction {
	details := strings.TrimSpace(strings.TrimPrefix(row, "worker dispatch retry plan artifact:"))
	path := details
	rest := ""
	if before, after, ok := strings.Cut(details, " "); ok {
		path = strings.TrimSpace(before)
		rest = strings.TrimSpace(after)
	}
	return RuntimeAction{
		Kind:          "worker-dispatch-retry-plan-artifact",
		State:         runtimeRowField(rest, "status"),
		ExecutionMode: runtimeRowField(rest, "mode"),
		ArtifactPath:  path,
		Ready:         runtimeRowField(rest, "ready"),
		Skipped:       runtimeRowField(rest, "skipped"),
		Row:           row,
	}
}

func runtimeWorkerDispatchRetryDispatchArtifactActionForRow(row string) RuntimeAction {
	details := strings.TrimSpace(strings.TrimPrefix(row, "worker dispatch retry dispatch artifact:"))
	path := details
	rest := ""
	if before, after, ok := strings.Cut(details, " "); ok {
		path = strings.TrimSpace(before)
		rest = strings.TrimSpace(after)
	}
	return RuntimeAction{
		Kind:          "worker-dispatch-retry-dispatch-artifact",
		State:         runtimeRowField(rest, "status"),
		ExecutionMode: runtimeRowField(rest, "mode"),
		ArtifactPath:  path,
		DispatchPath:  runtimeRowField(rest, "retry_plan"),
		Ready:         runtimeRowField(rest, "ready"),
		Skipped:       runtimeRowField(rest, "skipped"),
		Row:           row,
	}
}

func runtimeWorkerDispatchRetryDispatchExecutionArtifactActionForRow(row string) RuntimeAction {
	details := strings.TrimSpace(strings.TrimPrefix(row, "worker dispatch retry execution artifact:"))
	path := details
	rest := ""
	if before, after, ok := strings.Cut(details, " "); ok {
		path = strings.TrimSpace(before)
		rest = strings.TrimSpace(after)
	}
	return RuntimeAction{
		Kind:              "worker-dispatch-retry-dispatch-execution-artifact",
		State:             runtimeRowField(rest, "status"),
		ExecutionMode:     runtimeRowField(rest, "mode"),
		ArtifactPath:      path,
		RetryDispatchPath: runtimeRowField(rest, "retry_dispatch"),
		Claimed:           runtimeRowField(rest, "claimed"),
		Skipped:           runtimeRowField(rest, "skipped"),
		Failed:            runtimeRowField(rest, "failed"),
		Row:               row,
	}
}

func runtimeWorkerDispatchRetryDispatchExecutionResultActionForRow(row string) RuntimeAction {
	rest := strings.TrimSpace(strings.TrimPrefix(row, "worker dispatch retry execution result:"))
	return RuntimeAction{
		Kind:             "worker-dispatch-retry-dispatch-execution-result",
		State:            runtimeRowField(rest, "status"),
		ArtifactPath:     runtimeRowField(rest, "retry_claim"),
		SourceResultPath: runtimeRowField(rest, "source_result"),
		DispatchPath:     runtimeRowField(rest, "source_dispatch"),
		SourceClaimPath:  runtimeRowField(rest, "source_claim"),
		ExecutionPath:    runtimeRowField(rest, "queue_dispatch"),
		RequestPath:      runtimeRowField(rest, "request_path"),
		TransportMode:    runtimeRowField(rest, "transport"),
		WorkerID:         runtimeRowField(rest, "worker"),
		RunID:            runtimeRowField(rest, "run"),
		ExecutionID:      runtimeRowField(rest, "execution"),
		Reason:           runtimeRowField(rest, "reason"),
		Row:              row,
	}
}

func runtimeWorkerDispatchRetrySchedulerRunArtifactActionForRow(row string) RuntimeAction {
	details := strings.TrimSpace(strings.TrimPrefix(row, "worker dispatch retry scheduler run artifact:"))
	path := details
	rest := ""
	if before, after, ok := strings.Cut(details, " "); ok {
		path = strings.TrimSpace(before)
		rest = strings.TrimSpace(after)
	}
	results := strings.Join(nonEmptyRuntimeParts([]string{
		"max_runs=" + runtimeRowField(rest, "max_runs"),
		"retry_plans_created=" + runtimeRowField(rest, "retry_plans_created"),
		"retry_dispatches_created=" + runtimeRowField(rest, "retry_dispatches_created"),
		"retry_dispatch_executions_created=" + runtimeRowField(rest, "retry_dispatch_executions_created"),
		"dispatch_retry_claims=" + runtimeRowField(rest, "dispatch_retry_claims"),
		"retry_transport=" + runtimeRowField(rest, "retry_transport"),
		"retry_dispatch_claims_created=" + runtimeRowField(rest, "retry_dispatch_claims_created"),
		"retry_dispatch_claims_dispatched=" + runtimeRowField(rest, "retry_dispatch_claims_dispatched"),
		"retry_dispatch_transport_requests_created=" + runtimeRowField(rest, "retry_dispatch_transport_requests_created"),
	}), " ")
	return RuntimeAction{
		Kind:          "worker-dispatch-retry-scheduler-run-artifact",
		State:         runtimeRowField(rest, "status"),
		ExecutionMode: runtimeRowField(rest, "mode"),
		ArtifactPath:  path,
		TransportMode: runtimeRowField(rest, "retry_transport"),
		Results:       results,
		Skipped:       runtimeOptionalRowField(rest, "skip"),
		Failed:        runtimeOptionalRowField(rest, "error"),
		Row:           row,
	}
}

func runtimeReportProjectionDispatchArtifactActionForRow(row string) RuntimeAction {
	details := strings.TrimSpace(strings.TrimPrefix(row, "report projection dispatch artifact:"))
	path := details
	rest := ""
	if before, after, ok := strings.Cut(details, " "); ok {
		path = strings.TrimSpace(before)
		rest = strings.TrimSpace(after)
	}
	return RuntimeAction{
		Kind:              "report-projection-dispatch-artifact",
		State:             runtimeRowField(rest, "status"),
		ExecutionMode:     runtimeRowField(rest, "mode"),
		ArtifactPath:      path,
		RunID:             runtimeRowField(rest, "run"),
		PlannedReportPath: runtimeRowFieldWithFallback(rest, "planned_report", "report"),
		Results:           "events=" + runtimeRowField(rest, "events"),
		CreatedBy:         runtimeRowField(rest, "actor"),
		CreatedAt:         runtimeRowField(rest, "created_at"),
		Reason:            runtimeRowField(rest, "reason"),
		Row:               row,
	}
}

func runtimeReportProjectionDispatchExecutionArtifactActionForRow(row string) RuntimeAction {
	details := strings.TrimSpace(strings.TrimPrefix(row, "report projection dispatch execution artifact:"))
	path := details
	rest := ""
	if before, after, ok := strings.Cut(details, " "); ok {
		path = strings.TrimSpace(before)
		rest = strings.TrimSpace(after)
	}
	return RuntimeAction{
		Kind:          "report-projection-dispatch-execution-artifact",
		State:         runtimeRowField(rest, "status"),
		ExecutionMode: runtimeRowField(rest, "mode"),
		ArtifactPath:  path,
		DispatchPath:  runtimeRowField(rest, "dispatch"),
		RunID:         runtimeRowField(rest, "run"),
		ResultPath:    runtimeRowField(rest, "report"),
		Results: strings.Join([]string{
			"events=" + runtimeRowField(rest, "events"),
			"written=" + runtimeRowField(rest, "written"),
			"executed_at=" + runtimeRowField(rest, "executed_at"),
		}, " "),
		CreatedBy:    runtimeRowField(rest, "actor"),
		CreatedAt:    runtimeRowField(rest, "executed_at"),
		Reason:       runtimeRowField(rest, "reason"),
		Decision:     runtimeRowField(rest, "decision"),
		Reviewer:     runtimeRowField(rest, "reviewer"),
		ReviewedAt:   runtimeRowField(rest, "reviewed_at"),
		ReviewReason: runtimeRowField(rest, "review_reason"),
		Current:      runtimeRowField(rest, "current"),
		Row:          row,
	}
}

func runtimeReportProjectionExecutionReviewActionForRow(row string) RuntimeAction {
	details := strings.TrimSpace(strings.TrimPrefix(row, "report projection execution review:"))
	return RuntimeAction{
		Kind:          "report-projection-execution-review",
		State:         "reviewed",
		ExecutionMode: runtimeRowField(details, "mode"),
		ReviewPath:    runtimeRowField(details, "review"),
		ExecutionPath: runtimeRowField(details, "execution"),
		DispatchPath:  runtimeRowField(details, "dispatch"),
		RunID:         runtimeRowField(details, "run"),
		ResultPath:    runtimeRowField(details, "report"),
		Decision:      runtimeRowField(details, "decision"),
		Reviewer:      runtimeRowField(details, "reviewer"),
		ReviewedAt:    runtimeRowField(details, "reviewed_at"),
		ReviewReason:  runtimeRowField(details, "review_reason"),
		Row:           row,
	}
}

func runtimeReportProjectionExecutionSelectionActionForRow(row string) RuntimeAction {
	details := strings.TrimSpace(strings.TrimPrefix(row, "report projection execution selection:"))
	state := runtimeRowField(details, "status")
	current := runtimeRowField(details, "current")
	if current == "" {
		current = "false"
		if state == "current" {
			current = "true"
		}
	}
	return RuntimeAction{
		Kind:          "report-projection-execution-selection",
		State:         state,
		ExecutionMode: runtimeRowField(details, "mode"),
		ArtifactPath:  runtimeRowField(details, "execution"),
		RequestPath:   runtimeRowField(details, "selection"),
		DispatchPath:  runtimeRowField(details, "dispatch"),
		RunID:         runtimeRowField(details, "run"),
		ResultPath:    runtimeRowField(details, "report"),
		Results: strings.Join([]string{
			"events=" + runtimeRowField(details, "events"),
			"written=" + runtimeRowField(details, "written"),
		}, " "),
		SelectedBy: runtimeRowField(details, "selected_by"),
		SelectedAt: runtimeRowField(details, "selected_at"),
		ClearedBy:  runtimeRowField(details, "cleared_by"),
		ClearedAt:  runtimeRowField(details, "cleared_at"),
		Superseded: runtimeRowField(details, "superseded"),
		Reason:     runtimeRowField(details, "reason"),
		Current:    current,
		Row:        row,
	}
}

func runtimeWorkerTransportRequestArtifactActionForRow(row string) RuntimeAction {
	details := strings.TrimSpace(strings.TrimPrefix(row, "worker transport request:"))
	path := details
	rest := ""
	if before, after, ok := strings.Cut(details, " "); ok {
		path = strings.TrimSpace(before)
		rest = strings.TrimSpace(after)
	}
	return RuntimeAction{
		Kind:          "worker-transport-request-artifact",
		State:         runtimeRowField(rest, "status"),
		ExecutionMode: runtimeRowField(rest, "transport"),
		ArtifactPath:  path,
		DispatchPath:  runtimeRowField(rest, "dispatch"),
		Source:        runtimeRowField(rest, "claim"),
		Name:          runtimeRowField(rest, "worker"),
		Row:           row,
	}
}

func runtimeWorkerTransportDaemonArtifactActionForRow(row string) RuntimeAction {
	details := strings.TrimSpace(strings.TrimPrefix(row, "worker transport daemon artifact:"))
	path := details
	rest := ""
	if before, after, ok := strings.Cut(details, " "); ok {
		path = strings.TrimSpace(before)
		rest = strings.TrimSpace(after)
	}
	return RuntimeAction{
		Kind:          "worker-transport-daemon-artifact",
		State:         runtimeRowField(rest, "status"),
		ExecutionMode: runtimeRowField(rest, "mode"),
		ArtifactPath:  path,
		Name:          runtimeRowField(rest, "worker"),
		Source:        runtimeRowField(rest, "source"),
		Row:           row,
	}
}

func runtimeWorkerTransportDaemonControlArtifactActionForRow(row string) RuntimeAction {
	details := strings.TrimSpace(strings.TrimPrefix(row, "worker transport daemon control artifact:"))
	path := details
	rest := ""
	if before, after, ok := strings.Cut(details, " "); ok {
		path = strings.TrimSpace(before)
		rest = strings.TrimSpace(after)
	}
	return RuntimeAction{
		Kind:          "worker-transport-daemon-control-artifact",
		State:         runtimeRowField(rest, "status"),
		ExecutionMode: runtimeRowField(rest, "mode"),
		ArtifactPath:  path,
		Name:          runtimeRowField(rest, "worker"),
		ControlAction: runtimeRowField(rest, "action"),
		Row:           row,
	}
}

func runtimeWorkerTransportDaemonSupervisorArtifactActionForRow(row string) RuntimeAction {
	details := strings.TrimSpace(strings.TrimPrefix(row, "worker transport daemon supervisor artifact:"))
	path := details
	rest := ""
	if before, after, ok := strings.Cut(details, " "); ok {
		path = strings.TrimSpace(before)
		rest = strings.TrimSpace(after)
	}
	return RuntimeAction{
		Kind:          "worker-transport-daemon-supervisor-artifact",
		State:         runtimeRowField(rest, "status"),
		ExecutionMode: runtimeRowField(rest, "mode"),
		ArtifactPath:  path,
		Name:          runtimeRowField(rest, "worker"),
		Source:        runtimeRowField(rest, "source"),
		Row:           row,
	}
}

func runtimeRetryExecutionArtifactActionForRow(row string) RuntimeAction {
	details := strings.TrimSpace(strings.TrimPrefix(row, "retry execution artifact:"))
	path := details
	rest := ""
	if before, after, ok := strings.Cut(details, " "); ok {
		path = strings.TrimSpace(before)
		rest = strings.TrimSpace(after)
	}
	return RuntimeAction{
		Kind:              "install-retry-execution-artifact",
		State:             runtimeRowField(rest, "status"),
		ExecutionMode:     runtimeRowField(rest, "mode"),
		ArtifactPath:      path,
		RetryDispatchPath: runtimeRowField(rest, "retry_dispatch"),
		Results:           runtimeRowField(rest, "results"),
		Skipped:           runtimeRowField(rest, "skipped"),
		Row:               row,
	}
}

func runtimeRetryExecutionResultActionForRow(row string) RuntimeAction {
	details := strings.TrimSpace(strings.TrimPrefix(row, "retry execution result "))
	details = strings.TrimSpace(strings.TrimPrefix(details, "refs "))
	name := details
	rest := ""
	if before, after, ok := strings.Cut(details, ":"); ok {
		name = strings.TrimSpace(before)
		rest = strings.TrimSpace(after)
	}
	pack := ""
	dependency := name
	if before, after, ok := strings.Cut(name, "/"); ok {
		pack = strings.TrimSpace(before)
		dependency = strings.TrimSpace(after)
	}
	return RuntimeAction{
		Kind:                "install-retry-execution-result",
		Pack:                pack,
		Name:                dependency,
		State:               firstRuntimeToken(rest),
		Command:             runtimeRowField(rest, "command"),
		Output:              runtimeRowField(rest, "output"),
		ExecutionError:      runtimeRowField(rest, "error"),
		ArtifactPath:        runtimeRowField(rest, "artifact"),
		DispatchPath:        runtimeRowField(rest, "dispatch"),
		RetryDispatchPath:   runtimeRowField(rest, "retry_dispatch"),
		SourceExecutionPath: runtimeRowField(rest, "source_execution"),
		Row:                 row,
	}
}

func runtimeActionIsReadOnly(action RuntimeAction) bool {
	kind := strings.TrimSpace(action.Kind)
	if kind == "compile-preview-review-selection" {
		return strings.TrimSpace(action.State) != "current"
	}
	if kind == "compile-preview-review-artifact" {
		return strings.TrimSpace(action.Current) != "false"
	}
	if kind == "worker-lifecycle-policy-selection" {
		return strings.TrimSpace(action.State) != "current"
	}
	if kind == "worker-lifecycle-policy-artifact" {
		return strings.TrimSpace(action.Current) != "false"
	}
	if kind == "worker-lifecycle-action-plan-selection" {
		return strings.TrimSpace(action.State) != "current"
	}
	if kind == "worker-lifecycle-action-plan-artifact" {
		return strings.TrimSpace(action.Current) != "false"
	}
	if kind == "replay-workbench-session-selection" {
		return strings.TrimSpace(action.State) != "current"
	}
	if kind == "replay-workbench-session-artifact" {
		return strings.TrimSpace(action.Current) != "false"
	}
	if kind == "replay-workbench-collaboration-selection" {
		return strings.TrimSpace(action.State) != "current"
	}
	if kind == "replay-workbench-collaboration-artifact" {
		return strings.TrimSpace(action.Current) != "false"
	}
	if kind == "replay-workbench-collaboration-note-selection" {
		return strings.TrimSpace(action.State) != "current"
	}
	if kind == "replay-workbench-collaboration-note" {
		return strings.TrimSpace(action.Current) != "false"
	}
	if kind == "report-projection-execution-selection" {
		return strings.TrimSpace(action.State) != "current"
	}
	if kind == "report-projection-dispatch-artifact" {
		return strings.TrimSpace(action.State) != "dry-run" || strings.TrimSpace(action.ExecutionMode) != "metadata-only"
	}
	if kind == "report-projection-dispatch-execution-artifact" {
		return strings.TrimSpace(action.Current) != "false" || strings.TrimSpace(action.State) != "completed" || !strings.Contains(action.Results, "written=true")
	}
	return kind == "provider-usage-summary" || strings.HasPrefix(kind, "compile-preview-signal") || kind == "compile-preview-review-comparison" || kind == "replay-workbench-collaboration-presence" || strings.HasPrefix(kind, "workspace-safety") || strings.HasPrefix(kind, "install-execution") || strings.HasPrefix(kind, "install-retry-execution") || kind == "install-orchestration-artifact" || kind == "report-projection-execution-review" || kind == "worker-orchestration-artifact" || kind == "worker-orchestration-supervisor-artifact" || kind == "worker-lifecycle-audit-artifact" || kind == "worker-lifecycle-audit-item" || kind == "worker-lifecycle-review-artifact" || kind == "worker-lifecycle-action-plan-artifact" || kind == "worker-lifecycle-maintenance-artifact" || kind == "worker-lifecycle-maintenance-supervisor-artifact" || kind == "worker-lifecycle-policy-run-artifact" || kind == "worker-lifecycle-policy-scheduler-run-artifact" || kind == "worker-lifecycle-action" || strings.HasPrefix(kind, "worker-lease-result-reconciliation-execution") || strings.HasPrefix(kind, "worker-lease-reclaim-execution") || strings.HasPrefix(kind, "worker-queue-dispatch-execution") || strings.HasPrefix(kind, "worker-queue-dispatch-result") || kind == "worker-dispatch-retry-plan-artifact" || kind == "worker-dispatch-retry-dispatch-artifact" || strings.HasPrefix(kind, "worker-dispatch-retry-dispatch-execution") || kind == "worker-dispatch-retry-scheduler-run-artifact" || kind == "worker-transport-daemon-supervisor-artifact" || kind == "worker-transport-daemon-control-artifact"
}

func runtimeProviderUsageSummaryActionForRow(row string) RuntimeAction {
	fields := runtimeSimpleKeyValueFields(strings.TrimSpace(strings.TrimPrefix(row, "provider usage summary:")))
	source := fields["source"]
	if encoded := strings.TrimSpace(fields["source_b64"]); encoded != "" {
		if decoded, err := base64.RawURLEncoding.DecodeString(encoded); err == nil {
			source = string(decoded)
		}
	}
	providerCount := valueOrDefault(fields["provider_count"], strconv.Itoa(pipeSeparatedValueCount(fields["providers"])))
	modelCount := valueOrDefault(fields["model_count"], strconv.Itoa(pipeSeparatedValueCount(fields["models"])))
	return RuntimeAction{
		Kind:          "provider-usage-summary",
		State:         "ready",
		ExecutionMode: valueOrDefault(fields["mode"], "metadata-only"),
		Source:        source,
		Ready:         "records=" + valueOrDefault(fields["records"], "0") + " providers=" + providerCount + " models=" + modelCount,
		Results: strings.Join(nonEmptyRuntimeParts([]string{
			"input_tokens=" + fields["input_tokens"],
			"output_tokens=" + fields["output_tokens"],
			"total_tokens=" + fields["total_tokens"],
			"estimated_cost_usd=" + fields["estimated_cost_usd"],
			"estimated_cost_records=" + fields["estimated_cost_records"],
		}), " "),
		Output: "provider usage summary is read-only metadata from manual provider previews",
		Row:    row,
	}
}

func runtimeSimpleKeyValueFields(row string) map[string]string {
	values := map[string]string{}
	for _, field := range strings.Fields(row) {
		key, value, ok := strings.Cut(field, "=")
		if !ok {
			continue
		}
		values[strings.TrimSpace(key)] = strings.TrimSpace(value)
	}
	return values
}

func pipeSeparatedValueCount(value string) int {
	value = strings.TrimSpace(value)
	if value == "" || value == "none" {
		return 0
	}
	return strings.Count(value, "|") + 1
}

func firstRuntimeToken(value string) string {
	fields := strings.Fields(value)
	if len(fields) == 0 {
		return ""
	}
	return fields[0]
}

func runtimeRowField(row string, key string) string {
	search := " " + row
	marker := " " + key + "="
	index := strings.Index(search, marker)
	if index < 0 {
		return ""
	}
	start := index + len(marker)
	end := len(search)
	for _, candidate := range []string{"required", "pack", "command", "hint", "source", "digest", "depends_on", "type", "kind", "gate", "status", "mode", "transport", "artifact", "path", "linked", "dispatch", "queue_dispatch", "retry_dispatch", "retry_plan", "source_execution", "source_result", "source_dispatch", "source_claim", "retry_claim", "claim", "worker", "action", "run", "primary", "selected", "plan", "policy", "selection", "policy_basename", "selection_basename", "session_path", "session", "collaboration", "participant", "seen_by", "seen_at", "author", "actor", "target_kind", "target_id", "body", "execution", "manifest", "capability", "domain", "weight", "target", "score", "nodes", "warnings", "current", "base", "candidate", "node_delta", "warning_delta", "added", "removed", "changed", "reordered", "score_changed", "signal_changed", "decision", "reviewer", "reviewed_at", "review_reason", "selected_by", "selected_at", "cleared_by", "cleared_at", "superseded", "max_plan_nodes", "domain_priorities", "frontier_scores", "historical_stats", "filters", "endpoints", "participants", "created_by", "created_at", "executed_at", "successes", "failures", "total", "ready", "reason", "planned_report", "report", "events", "written", "request_path", "lease_path", "result_path", "review_path", "action_plan_path", "reclaim_path", "reconciliation_path", "reclaim_execution", "reconciliation_execution", "maintenance_path", "audit_path", "orchestration_path", "recommended", "result", "results", "retry_plans", "retry_dispatches", "retry_dispatch_execute", "retry_dispatch_executions", "retry_plans_created", "retry_dispatches_created", "retry_dispatch_executions_created", "dispatch_retry_claims", "retry_transport", "retry_dispatch_claims_created", "retry_dispatch_claims_dispatched", "retry_dispatch_transport_requests_created", "dispatch_lifecycle_retry_claims", "lifecycle_retry_transport", "reclaim_executions_created", "reclaim_results_created", "reconciliation_executions_created", "reconciliation_results_created", "execute_lease_recovery", "lease_recovery", "lease", "leases", "active", "stale", "stale_after", "worker_stale_after", "poll_interval_ms", "poll_interval", "stale_leases", "lifecycle_retries", "lifecycle_policies", "policy_selections", "policy_runs", "policy_scheduler_runs", "lifecycle_policy_selections", "lifecycle_policy_runs", "lifecycle_policy_scheduler_runs", "include_lifecycle_maintenance", "execute_lifecycle_retries", "policy_runs_created", "supervisor_created", "skip", "skipped", "output", "error", "placement_ready", "placement_skipped", "claimed", "dispatch_ready", "dispatch_skipped", "dispatched", "dispatches", "failed", "reclaim_ready", "reclaim_skipped", "reconciliation_ready", "reconciliation_skipped", "placement_plan", "placement_dispatch", "placement_batch", "lifecycle_maintenance", "lifecycle_audit", "review", "action_plan", "actions", "ack", "ack_actions", "run_actions", "daemon_control", "daemon_control_actions", "skipped_unsafe", "review_created", "action_plan_created", "reclaim_created", "reconciliation_created", "reclaim", "reclaims", "reclaim_actions", "reconcile", "reconciliation_actions", "reconciliations", "workers", "stale_workers", "placements", "claims", "requests", "daemon_runs", "supervisor_runs", "controls", "lifecycle_reviews", "action_plans", "maintenances", "maintenance_supervisors", "orchestration_supervisors", "runs", "max_runs", "max_runs_requested", "max_supervisor_runs", "orchestrations_created", "maintenance_created", "audit_created"} {
		if candidate == key {
			continue
		}
		candidateMarker := " " + candidate + "="
		if next := strings.Index(search[start:], candidateMarker); next >= 0 && start+next < end {
			end = start + next
		}
	}
	return strings.TrimSpace(search[start:end])
}

func runtimeOptionalRowField(row string, key string) string {
	value := strings.TrimSpace(runtimeRowField(row, key))
	if value == "" || value == "none" {
		return ""
	}
	return value
}

func runtimeRowFieldWithFallback(row string, key string, fallback string) string {
	value := strings.TrimSpace(runtimeRowField(row, key))
	if value != "" {
		return value
	}
	return runtimeRowField(row, fallback)
}

func nonEmptyRuntimeParts(parts []string) []string {
	filtered := make([]string, 0, len(parts))
	for _, part := range parts {
		_, value, ok := strings.Cut(part, "=")
		if !ok || strings.TrimSpace(value) == "" {
			continue
		}
		filtered = append(filtered, part)
	}
	return filtered
}

func runtimeOptionalRowFieldWithFallback(row string, key string, fallback string) string {
	value := runtimeOptionalRowField(row, key)
	if strings.TrimSpace(value) != "" {
		return value
	}
	return runtimeOptionalRowField(row, fallback)
}

func (t RuntimeTabs) activeRenderedRows() []string {
	rows := t.ActiveRows()
	if len(rows) == 0 {
		return rows
	}
	index := t.Selected[t.Active]
	if index < 0 {
		index = 0
	}
	if index >= len(rows) {
		index = len(rows) - 1
	}
	rendered := make([]string, 0, len(rows))
	for i, row := range rows {
		if i == index {
			rendered = append(rendered, "> "+row)
			continue
		}
		rendered = append(rendered, "  "+row)
	}
	return rendered
}

func (t RuntimeTabs) tabHeaderRows(width int) []string {
	if width <= 0 {
		return nil
	}
	rows := []string{}
	current := ""
	for _, tab := range t.Tabs {
		label := string(tab)
		if tab == t.Active {
			label = "[" + label + "]"
		}
		if current == "" {
			current = label
			continue
		}
		candidate := current + " | " + label
		if lipgloss.Width(candidate) > width {
			rows = append(rows, current)
			current = label
			continue
		}
		current = candidate
	}
	if current != "" {
		rows = append(rows, current)
	}
	return rows
}
