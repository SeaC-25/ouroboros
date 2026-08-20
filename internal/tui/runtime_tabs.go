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
	ArtifactID                 string
	ArtifactPath               string
	ArchivePath                string
	ReleaseBundlePath          string
	DispatchPath               string
	ReportDispatchPath         string
	ReportExecutionPath        string
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
	case "run-model-generation":
		return "action: run model generation selected read-only"
	case "model-generation-artifact":
		return "action: model generation artifact selected " + strings.TrimSpace(a.ArtifactPath)
	case "model-tool-plan-preview":
		return "action: model tool plan preview selected " + strings.TrimSpace(a.ArtifactPath)
	case "model-tool-plan-preview-acceptance":
		return "action: model tool plan preview acceptance selected " + strings.TrimSpace(a.ArtifactPath)
	case "compile-preview-review-execution-dispatch-artifact":
		return "action: compile preview review execution dispatch artifact selected " + strings.TrimSpace(a.ArtifactPath)
	case "compile-preview-review-execution-artifact":
		return "action: compile preview review execution artifact selected " + strings.TrimSpace(a.ArtifactPath)
	case "toolchain-contract":
		return "action: toolchain contract selected " + valueOrDefault(strings.TrimSpace(a.State), "unknown")
	case "toolchain-contract-error":
		return "action: toolchain contract error selected"
	case "managed-bundle":
		name := strings.TrimSpace(a.Name)
		if name == "" {
			name = strings.TrimSpace(a.Row)
		}
		return "action: managed bundle selected " + name
	case "managed-update-request":
		return "action: managed update request selected " + strings.TrimSpace(a.ArtifactPath)
	case "release-managed-status-source":
		return "action: release managed status source selected " + strings.TrimSpace(a.ArtifactPath)
	case "release-managed-status":
		name := strings.TrimSpace(a.Name)
		if name == "" {
			name = strings.TrimSpace(a.Row)
		}
		return "action: release managed status selected " + name
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
	if archivePath := strings.TrimSpace(a.ArchivePath); archivePath != "" {
		rows = append(rows, "archive path: "+archivePath)
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
	if reportDispatch := strings.TrimSpace(a.ReportDispatchPath); reportDispatch != "" {
		rows = append(rows, "report dispatch path: "+reportDispatch)
	}
	if reportExecution := strings.TrimSpace(a.ReportExecutionPath); reportExecution != "" {
		rows = append(rows, "report execution path: "+reportExecution)
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
	if digest := strings.TrimSpace(a.Digest); digest != "" {
		rows = append(rows, "digest: "+digest)
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
	rows = append(rows, t.activeRenderedRows(height-1-len(rows))...)
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
	if strings.HasPrefix(trimmed, "retry execution result refs summary ") {
		return runtimeRetryExecutionResultRefsSummaryActionForRow(trimmed), true
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
	if strings.HasPrefix(trimmed, "run model generation:") {
		return runtimeRunModelGenerationActionForRow(trimmed), true
	}
	if strings.HasPrefix(trimmed, "model generation artifact:") {
		return runtimeModelGenerationArtifactActionForRow(trimmed), true
	}
	if strings.HasPrefix(trimmed, "model tool plan preview:") {
		return runtimeModelToolPlanPreviewActionForRow(trimmed), true
	}
	if strings.HasPrefix(trimmed, "model tool plan preview acceptance artifact:") {
		return runtimeModelToolPlanPreviewAcceptanceActionForRow(trimmed), true
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
	if strings.HasPrefix(trimmed, "workflow status:") {
		return runtimeWorkflowStatusActionForRow(trimmed), true
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
	if strings.HasPrefix(trimmed, "toolchain contract:") {
		return runtimeToolchainContractActionForRow(trimmed), true
	}
	if strings.HasPrefix(trimmed, "toolchain contract error:") {
		return runtimeToolchainContractErrorActionForRow(trimmed), true
	}
	if strings.HasPrefix(trimmed, "managed bundle:") {
		return runtimeManagedBundleActionForRow(trimmed), true
	}
	if strings.HasPrefix(trimmed, "managed update apply execution artifact:") {
		return runtimeManagedUpdateApplyExecutionActionForRow(trimmed), true
	}
	if strings.HasPrefix(trimmed, "managed update download execution artifact:") {
		return runtimeManagedUpdateDownloadExecutionActionForRow(trimmed), true
	}
	if strings.HasPrefix(trimmed, "managed update download plan artifact:") {
		return runtimeManagedUpdateDownloadPlanActionForRow(trimmed), true
	}
	if strings.HasPrefix(trimmed, "managed update request artifact:") {
		return runtimeManagedUpdateRequestActionForRow(trimmed), true
	}
	if strings.HasPrefix(trimmed, "release managed status source:") {
		return runtimeReleaseManagedStatusSourceActionForRow(trimmed), true
	}
	if strings.HasPrefix(trimmed, "release managed status:") {
		return runtimeReleaseManagedStatusActionForRow(trimmed), true
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
	if strings.HasPrefix(trimmed, "compile-preview review execution dispatch artifact:") {
		return runtimeCompilePreviewReviewExecutionDispatchArtifactActionForRow(trimmed), true
	}
	if strings.HasPrefix(trimmed, "compile-preview review execution artifact:") {
		return runtimeCompilePreviewReviewExecutionArtifactActionForRow(trimmed), true
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

func runtimeToolchainContractActionForRow(row string) RuntimeAction {
	fields := runtimeSimpleKeyValueFields(strings.TrimSpace(strings.TrimPrefix(row, "toolchain contract:")))
	return RuntimeAction{
		Kind:          "toolchain-contract",
		State:         valueOrDefault(fields["status"], "blocked"),
		ExecutionMode: valueOrDefault(fields["mode"], "metadata-only"),
		Ready: strings.Join(nonEmptyRuntimeParts([]string{
			"manifests=" + fields["manifests"],
			"tools=" + fields["tools"],
			"domain_packs=" + fields["domain_packs"],
			"patterns=" + fields["patterns"],
			"semantic_actions=" + fields["semantic_actions"],
		}), " "),
		Failed: "errors=" + valueOrDefault(fields["errors"], "0"),
		Output: "contract check is read-only metadata",
		Row:    row,
	}
}

func runtimeToolchainContractErrorActionForRow(row string) RuntimeAction {
	return RuntimeAction{
		Kind:          "toolchain-contract-error",
		State:         "blocked",
		ExecutionMode: "metadata-only",
		Reason:        strings.TrimSpace(strings.TrimPrefix(row, "toolchain contract error:")),
		Output:        "contract validation error is read-only metadata",
		Row:           row,
	}
}

func runtimeManagedBundleActionForRow(row string) RuntimeAction {
	details := strings.TrimSpace(strings.TrimPrefix(row, "managed bundle:"))
	rawFields := runtimeSimpleKeyValueFields(details)
	machineDetails := runtimeRowWithoutLegacyTextFields(details, "source", "payload", "install", "manifest")
	fields := runtimeSimpleKeyValueFields(machineDetails)
	provenance := strings.Join(nonEmptyRuntimeParts([]string{
		"provenance_status=" + fields["provenance_status"],
		"provenance_source_url=" + fields["provenance_source_url"],
		"provenance_upstream_version=" + fields["provenance_upstream_version"],
		"provenance_retrieved_at=" + fields["provenance_retrieved_at"],
		"provenance_verified_by=" + fields["provenance_verified_by"],
		"corpus_status=" + fields["corpus_status"],
		"corpus_name=" + fields["corpus_name"],
		"corpus_source=" + fields["corpus_source"],
		"corpus_retrieved_at=" + fields["corpus_retrieved_at"],
		"corpus_file_count=" + fields["corpus_file_count"],
		"corpus_sha256=" + fields["corpus_sha256"],
		"corpus_digest_status=" + fields["corpus_digest_status"],
		"corpus_digest_algorithm=" + fields["corpus_digest_algorithm"],
		"corpus_digest_scope=" + fields["corpus_digest_scope"],
		"signature_status=" + fields["signature_status"],
		"signature_algorithm=" + fields["signature_algorithm"],
		"signature_key_id=" + fields["signature_key_id"],
		"signature_signed_at=" + fields["signature_signed_at"],
		"signature_signed_by=" + fields["signature_signed_by"],
	}), " ")
	if strings.TrimSpace(provenance) == "" {
		provenance = "managed bundle metadata is read-only"
	}
	return RuntimeAction{
		Kind:          "managed-bundle",
		State:         valueOrDefault(fields["status"], "available"),
		ExecutionMode: valueOrDefault(fields["mode"], "metadata-only"),
		Name:          fields["name"],
		Source:        runtimeDecodedRowField(details, rawFields, "source"),
		Digest:        fields["sha256"],
		ResultPath:    runtimeDecodedRowFieldWithFallbackRow(details, rawFields, "payload", machineDetails),
		Target:        runtimeDecodedRowFieldWithFallbackRow(details, rawFields, "install", machineDetails),
		ArtifactPath:  runtimeDecodedRowFieldWithFallbackRow(details, rawFields, "manifest", machineDetails),
		Output:        provenance,
		Hint:          runtimeManagedBundleUpdateHint(fields),
		Failed:        runtimeManagedBundleError(fields),
		Row:           row,
	}
}

func runtimeManagedUpdateRequestActionForRow(row string) RuntimeAction {
	details := strings.TrimSpace(strings.TrimPrefix(row, "managed update request artifact:"))
	rawFields := runtimeSimpleKeyValueFields(details)
	machineDetails := runtimeRowWithoutLegacyTextFields(details, "artifact", "release_root", "release_bundle", "release_manifest", "release_payload", "requested_by", "update_message", "reason")
	fields := runtimeSimpleKeyValueFields(machineDetails)
	updateMessage := runtimeDecodedRowField(details, rawFields, "update_message")
	requestedBy := runtimeDecodedRowField(details, rawFields, "requested_by")
	return RuntimeAction{
		Kind:          "managed-update-request",
		State:         valueOrDefault(fields["status"], "unknown"),
		ExecutionMode: valueOrDefault(fields["mode"], "metadata-only"),
		Name:          fields["name"],
		TargetKind:    fields["kind"],
		Digest:        fields["current_sha256"],
		Source:        runtimeDecodedRowFieldWithFallbackRow(details, rawFields, "release_root", machineDetails),
		Target:        runtimeDecodedRowFieldWithFallbackRow(details, rawFields, "release_bundle", machineDetails),
		RequestPath:   runtimeDecodedRowFieldWithFallbackRow(details, rawFields, "release_manifest", machineDetails),
		ResultPath:    runtimeDecodedRowFieldWithFallbackRow(details, rawFields, "release_payload", machineDetails),
		ArtifactPath:  runtimeDecodedRowFieldWithFallbackRow(details, rawFields, "artifact", machineDetails),
		Output: strings.Join(nonEmptyRuntimeParts([]string{
			"update_status=" + fields["update_status"],
			"current_version=" + fields["current_version"],
			"latest_version=" + fields["latest_version"],
			"latest_sha256=" + fields["latest_sha256"],
		}), " "),
		Hint: strings.Join(nonEmptyRuntimeParts([]string{
			"requested_by=" + requestedBy,
			"current_sha256=" + fields["current_sha256"],
			"checked_at=" + fields["update_checked_at"],
			"message=" + updateMessage,
		}), " "),
		Reason: runtimeDecodedRowField(details, rawFields, "reason"),
		Row:    row,
	}
}

func runtimeManagedUpdateDownloadPlanActionForRow(row string) RuntimeAction {
	details := strings.TrimSpace(strings.TrimPrefix(row, "managed update download plan artifact:"))
	rawFields := runtimeSimpleKeyValueFields(details)
	machineDetails := runtimeRowWithoutLegacyTextFields(details, "artifact", "request", "source_url", "release_root", "release_bundle", "release_manifest", "release_payload", "planned_by", "update_message", "reason")
	fields := runtimeSimpleKeyValueFields(machineDetails)
	updateMessage := runtimeDecodedRowField(details, rawFields, "update_message")
	plannedBy := runtimeDecodedRowField(details, rawFields, "planned_by")
	releaseBundle := runtimeDecodedRowFieldWithFallbackRow(details, rawFields, "release_bundle", machineDetails)
	return RuntimeAction{
		Kind:              "managed-update-download-plan",
		State:             valueOrDefault(fields["status"], "unknown"),
		ExecutionMode:     valueOrDefault(fields["mode"], "metadata-only"),
		Name:              fields["name"],
		TargetKind:        fields["kind"],
		Digest:            fields["expected_sha256"],
		Source:            runtimeDecodedRowFieldWithFallbackRow(details, rawFields, "source_url", machineDetails),
		Target:            releaseBundle,
		RequestPath:       runtimeDecodedRowFieldWithFallbackRow(details, rawFields, "request", machineDetails),
		ResultPath:        runtimeDecodedRowFieldWithFallbackRow(details, rawFields, "release_payload", machineDetails),
		ArtifactPath:      runtimeDecodedRowFieldWithFallbackRow(details, rawFields, "artifact", machineDetails),
		ReleaseBundlePath: releaseBundle,
		Output: strings.Join(nonEmptyRuntimeParts([]string{
			"update_status=" + fields["update_status"],
			"latest_version=" + fields["latest_version"],
			"expected_sha256_status=" + fields["expected_sha256_status"],
			runtimeOptionalOutputPart("archive_format", fields["archive_format"]),
			runtimeOptionalOutputPart("archive_expected_sha256_status", fields["archive_expected_sha256_status"]),
		}), " "),
		Hint: strings.Join(nonEmptyRuntimeParts([]string{
			"planned_by=" + plannedBy,
			"latest_sha256=" + fields["latest_sha256"],
			"checked_at=" + fields["update_checked_at"],
			"message=" + updateMessage,
		}), " "),
		Reason: runtimeDecodedRowField(details, rawFields, "reason"),
		Row:    row,
	}
}

func runtimeManagedUpdateDownloadExecutionActionForRow(row string) RuntimeAction {
	details := strings.TrimSpace(strings.TrimPrefix(row, "managed update download execution artifact:"))
	rawFields := runtimeSimpleKeyValueFields(details)
	machineDetails := runtimeRowWithoutLegacyTextFields(details, "artifact", "plan", "request", "source_url", "redirect_location", "release_root", "release_bundle", "release_manifest", "release_payload", "archive", "payload", "actor", "update_message", "reason", "error")
	fields := runtimeSimpleKeyValueFields(machineDetails)
	updateMessage := runtimeDecodedRowField(details, rawFields, "update_message")
	actor := runtimeDecodedRowField(details, rawFields, "actor")
	releaseBundle := runtimeDecodedRowFieldWithFallbackRow(details, rawFields, "release_bundle", machineDetails)
	redirectLocation := runtimeDecodedRowFieldWithFallbackRow(details, rawFields, "redirect_location", machineDetails)
	return RuntimeAction{
		Kind:              "managed-update-download-execution",
		State:             valueOrDefault(fields["status"], "unknown"),
		ExecutionMode:     valueOrDefault(fields["mode"], "execute"),
		Name:              fields["name"],
		TargetKind:        fields["kind"],
		Digest:            valueOrDefault(fields["actual_sha256"], fields["expected_sha256"]),
		Source:            runtimeDecodedRowFieldWithFallbackRow(details, rawFields, "source_url", machineDetails),
		Target:            releaseBundle,
		RequestPath:       runtimeDecodedRowFieldWithFallbackRow(details, rawFields, "request", machineDetails),
		ResultPath:        runtimeDecodedRowFieldWithFallbackRow(details, rawFields, "payload", machineDetails),
		ArtifactPath:      runtimeDecodedRowFieldWithFallbackRow(details, rawFields, "artifact", machineDetails),
		ArchivePath:       runtimeNonNoneField(runtimeDecodedRowFieldWithFallbackRow(details, rawFields, "archive", machineDetails)),
		DispatchPath:      runtimeDecodedRowFieldWithFallbackRow(details, rawFields, "plan", machineDetails),
		ReleaseBundlePath: releaseBundle,
		Output: strings.Join(nonEmptyRuntimeParts([]string{
			"digest_status=" + fields["digest_status"],
			"digest_algorithm=" + fields["digest_algorithm"],
			"digest_scope=" + fields["digest_scope"],
			"expected_sha256=" + fields["expected_sha256"],
			runtimeOptionalOutputPart("archive_format", fields["archive_format"]),
			runtimeOptionalOutputPart("archive_digest_status", fields["archive_digest_status"]),
			runtimeOptionalOutputPart("archive_digest_algorithm", fields["archive_digest_algorithm"]),
			runtimeOptionalOutputPart("archive_digest_scope", fields["archive_digest_scope"]),
			"bytes_written=" + fields["bytes_written"],
			"http_status=" + fields["http_status"],
			"content_length=" + fields["content_length"],
			runtimeOptionalOutputPart("redirect_policy", fields["redirect_policy"]),
			runtimeOptionalOutputPart("redirect_location", redirectLocation),
			runtimeOptionalOutputPart("redirect_location_status", fields["redirect_location_status"]),
		}), " "),
		Hint: strings.Join(nonEmptyRuntimeParts([]string{
			"actor=" + actor,
			"latest_version=" + fields["latest_version"],
			"checked_at=" + fields["update_checked_at"],
			"message=" + updateMessage,
		}), " "),
		Reason: runtimeDecodedRowField(details, rawFields, "reason"),
		Failed: runtimeDecodedRowField(details, rawFields, "error"),
		Row:    row,
	}
}

func runtimeManagedUpdateApplyExecutionActionForRow(row string) RuntimeAction {
	details := strings.TrimSpace(strings.TrimPrefix(row, "managed update apply execution artifact:"))
	rawFields := runtimeSimpleKeyValueFields(details)
	machineDetails := runtimeRowWithoutLegacyTextFields(details, "artifact", "download_execution", "plan", "request", "source_url", "release_root", "release_bundle", "release_manifest", "release_payload", "staged_payload", "workspace_bundle", "workspace_manifest", "workspace_payload", "install", "actor", "update_message", "reason", "error")
	fields := runtimeSimpleKeyValueFields(machineDetails)
	updateMessage := runtimeDecodedRowField(details, rawFields, "update_message")
	actor := runtimeDecodedRowField(details, rawFields, "actor")
	releaseBundle := runtimeDecodedRowFieldWithFallbackRow(details, rawFields, "release_bundle", machineDetails)
	workspaceBundle := runtimeDecodedRowFieldWithFallbackRow(details, rawFields, "workspace_bundle", machineDetails)
	return RuntimeAction{
		Kind:                "managed-update-apply-execution",
		State:               valueOrDefault(fields["status"], "unknown"),
		ExecutionMode:       valueOrDefault(fields["mode"], "apply"),
		Name:                fields["name"],
		TargetKind:          fields["kind"],
		Digest:              valueOrDefault(fields["actual_sha256"], fields["expected_sha256"]),
		Source:              runtimeDecodedRowFieldWithFallbackRow(details, rawFields, "source_url", machineDetails),
		Target:              runtimeDecodedRowFieldWithFallbackRow(details, rawFields, "workspace_payload", machineDetails),
		TargetID:            workspaceBundle,
		RequestPath:         runtimeDecodedRowFieldWithFallbackRow(details, rawFields, "request", machineDetails),
		ResultPath:          runtimeDecodedRowFieldWithFallbackRow(details, rawFields, "staged_payload", machineDetails),
		ArtifactPath:        runtimeDecodedRowFieldWithFallbackRow(details, rawFields, "artifact", machineDetails),
		DispatchPath:        runtimeDecodedRowFieldWithFallbackRow(details, rawFields, "plan", machineDetails),
		SourceExecutionPath: runtimeDecodedRowFieldWithFallbackRow(details, rawFields, "download_execution", machineDetails),
		ReleaseBundlePath:   releaseBundle,
		Output: strings.Join(nonEmptyRuntimeParts([]string{
			"digest_status=" + fields["digest_status"],
			"digest_algorithm=" + fields["digest_algorithm"],
			"digest_scope=" + fields["digest_scope"],
			"expected_sha256=" + fields["expected_sha256"],
			"latest_version=" + fields["latest_version"],
			"workspace_manifest=" + runtimeDecodedRowFieldWithFallbackRow(details, rawFields, "workspace_manifest", machineDetails),
			"install=" + runtimeDecodedRowFieldWithFallbackRow(details, rawFields, "install", machineDetails),
		}), " "),
		Hint: strings.Join(nonEmptyRuntimeParts([]string{
			"actor=" + actor,
			"checked_at=" + fields["update_checked_at"],
			"message=" + updateMessage,
			"release_bundle=" + releaseBundle,
			"workspace_bundle=" + workspaceBundle,
		}), " "),
		Reason: runtimeDecodedRowField(details, rawFields, "reason"),
		Failed: runtimeDecodedRowField(details, rawFields, "error"),
		Row:    row,
	}
}

func runtimeReleaseManagedStatusSourceActionForRow(row string) RuntimeAction {
	details := strings.TrimSpace(strings.TrimPrefix(row, "release managed status source:"))
	rawFields := runtimeSimpleKeyValueFields(details)
	machineDetails := runtimeRowWithoutLegacyTextFields(details, "release_root", "marker", "reason")
	fields := runtimeSimpleKeyValueFields(machineDetails)
	return RuntimeAction{
		Kind:          "release-managed-status-source",
		State:         valueOrDefault(fields["status"], "unknown"),
		ExecutionMode: valueOrDefault(fields["mode"], "metadata-only"),
		ResultPath:    runtimeDecodedRowFieldWithFallbackRow(details, rawFields, "release_root", machineDetails),
		ArtifactPath:  runtimeDecodedRowFieldWithFallbackRow(details, rawFields, "marker", machineDetails),
		Ready: strings.Join(nonEmptyRuntimeParts([]string{
			"total=" + fields["total"],
			"available=" + fields["available"],
			"not_imported=" + fields["not_imported"],
			"imported=" + fields["imported"],
			"linked=" + fields["linked"],
		}), " "),
		Failed: strings.Join(nonEmptyRuntimeParts([]string{
			"blocked=" + fields["blocked"],
			"conflict=" + fields["conflict"],
		}), " "),
		Reason: runtimeDecodedRowField(details, rawFields, "reason"),
		Output: "release managed status source is metadata-only",
		Hint:   runtimeReleaseManagedExpectedPinHint(fields),
		Row:    row,
	}
}

func runtimeReleaseManagedStatusActionForRow(row string) RuntimeAction {
	details := strings.TrimSpace(strings.TrimPrefix(row, "release managed status:"))
	rawFields := runtimeSimpleKeyValueFields(details)
	machineDetails := runtimeRowWithoutLegacyTextFields(details, "source", "release_bundle", "release_payload", "release_manifest", "workspace_bundle", "workspace_payload", "workspace_manifest", "install", "error")
	fields := runtimeSimpleKeyValueFields(machineDetails)
	provenance := strings.Join(nonEmptyRuntimeParts([]string{
		"provenance_status=" + fields["provenance_status"],
		"provenance_source_url=" + fields["provenance_source_url"],
		"provenance_upstream_version=" + fields["provenance_upstream_version"],
		"provenance_retrieved_at=" + fields["provenance_retrieved_at"],
		"provenance_verified_by=" + fields["provenance_verified_by"],
		"corpus_status=" + fields["corpus_status"],
		"corpus_name=" + fields["corpus_name"],
		"corpus_source=" + fields["corpus_source"],
		"corpus_retrieved_at=" + fields["corpus_retrieved_at"],
		"corpus_file_count=" + fields["corpus_file_count"],
		"corpus_sha256=" + fields["corpus_sha256"],
		"corpus_digest_status=" + fields["corpus_digest_status"],
		"corpus_digest_algorithm=" + fields["corpus_digest_algorithm"],
		"corpus_digest_scope=" + fields["corpus_digest_scope"],
		"signature_status=" + fields["signature_status"],
		"signature_algorithm=" + fields["signature_algorithm"],
		"signature_key_id=" + fields["signature_key_id"],
		"signature_signed_at=" + fields["signature_signed_at"],
		"signature_signed_by=" + fields["signature_signed_by"],
	}), " ")
	output := strings.Join(nonEmptyRuntimeParts([]string{
		"digest_status=" + fields["digest_status"],
		"import_status=" + fields["import_status"],
		"link_status=" + fields["link_status"],
	}), " ")
	return RuntimeAction{
		Kind:              "release-managed-status",
		State:             valueOrDefault(fields["status"], "blocked"),
		ExecutionMode:     valueOrDefault(fields["mode"], "read-only"),
		Name:              fields["name"],
		Source:            runtimeReleaseManagedStatusSource(details, rawFields, machineDetails),
		Digest:            fields["sha256"],
		ReleaseBundlePath: runtimeDecodedRowFieldWithFallbackRow(details, rawFields, "release_bundle", machineDetails),
		ResultPath:        runtimeDecodedRowFieldWithFallbackRow(details, rawFields, "release_payload", machineDetails),
		Target:            runtimeDecodedRowFieldWithFallbackRow(details, rawFields, "install", machineDetails),
		ArtifactPath:      runtimeDecodedRowFieldWithFallbackRow(details, rawFields, "release_manifest", machineDetails),
		RequestPath:       runtimeDecodedRowFieldWithFallbackRow(details, rawFields, "workspace_manifest", machineDetails),
		DispatchPath:      runtimeDecodedRowFieldWithFallbackRow(details, rawFields, "workspace_bundle", machineDetails),
		ExecutionPath:     runtimeDecodedRowFieldWithFallbackRow(details, rawFields, "workspace_payload", machineDetails),
		Ready: strings.Join(nonEmptyRuntimeParts([]string{
			"digest_status=" + fields["digest_status"],
			"import_status=" + fields["import_status"],
			"link_status=" + fields["link_status"],
		}), " "),
		Failed: runtimeReleaseManagedStatusError(details, rawFields),
		Output: output,
		Hint: strings.Join(nonEmptyRuntimeParts([]string{
			runtimeReleaseManagedExpectedPinHint(fields),
			runtimeManagedBundleUpdateHint(fields),
		}), " "),
		Reason: provenance,
		Row:    row,
	}
}

func runtimeReleaseManagedStatusSource(row string, fields map[string]string, fallbackRow string) string {
	if decoded, ok := runtimeDecodedBase64RowField(row, "source"); ok {
		return decoded
	}
	if decoded, ok := runtimeDecodedBase64Field(fields, "source"); ok {
		return decoded
	}
	if value := runtimeReleaseManagedStatusLegacyField(fallbackRow, "source"); strings.TrimSpace(value) != "" {
		return value
	}
	if value := strings.TrimSpace(fields["source"]); value != "" {
		return value
	}
	return runtimeDecodedRowField(row, fields, "source")
}

func runtimeReleaseManagedStatusLegacyField(row string, key string) string {
	search := " " + row
	marker := " " + key + "="
	index := strings.Index(search, marker)
	if index < 0 {
		return ""
	}
	start := index + len(marker)
	end := len(search)
	for _, candidate := range []string{
		"source_b64",
		"update_status",
		"sha256",
		"provenance_status",
		"signature_status",
		"signature_algorithm",
		"signature_key_id",
		"signature_signed_at",
		"signature_signed_by",
		"update_latest_version",
		"update_latest_sha256",
		"update_checked_at",
		"update_message",
		"provenance_source_url",
		"provenance_upstream_version",
		"provenance_retrieved_at",
		"provenance_verified_by",
		"digest_status",
		"digest_algorithm",
		"digest_scope",
		"import_status",
		"link_status",
		"release_bundle",
		"release_bundle_b64",
		"release_payload",
		"release_payload_b64",
		"release_manifest",
		"release_manifest_b64",
		"workspace_bundle",
		"workspace_bundle_b64",
		"workspace_payload",
		"workspace_payload_b64",
		"workspace_manifest",
		"workspace_manifest_b64",
		"install",
		"install_b64",
		"error",
		"error_b64",
	} {
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

func runtimeManagedBundleUpdateHint(fields map[string]string) string {
	return strings.Join(nonEmptyRuntimeParts([]string{
		"update_status=" + fields["update_status"],
		"update_latest_version=" + runtimeNonNoneField(fields["update_latest_version"]),
		"update_latest_sha256=" + runtimeNonNoneField(fields["update_latest_sha256"]),
		"update_checked_at=" + runtimeNonNoneField(fields["update_checked_at"]),
		"update_message=" + runtimeNonNoneField(fields["update_message"]),
	}), " ")
}

func runtimeReleaseManagedExpectedPinHint(fields map[string]string) string {
	return strings.Join(nonEmptyRuntimeParts([]string{
		"expected_sha256_bundle=" + runtimeNonNoneField(fields["expected_sha256_bundle"]),
		"expected_sha256_status=" + runtimeNonNoneField(fields["expected_sha256_status"]),
		"expected_sha256=" + runtimeNonNoneField(fields["expected_sha256"]),
	}), " ")
}

func runtimeReleaseManagedStatusError(row string, fields map[string]string) string {
	errorText := runtimeDecodedRowField(row, fields, "error")
	if strings.TrimSpace(errorText) == "" {
		return ""
	}
	return "error=" + errorText
}

func runtimeManagedBundleError(fields map[string]string) string {
	if strings.TrimSpace(fields["error"]) == "" {
		return ""
	}
	return "error=" + fields["error"]
}

func runtimeNonNoneField(value string) string {
	if strings.TrimSpace(value) == "none" {
		return ""
	}
	return value
}

func runtimeOptionalOutputPart(key string, value string) string {
	value = strings.TrimSpace(value)
	if value == "" || value == "none" {
		return ""
	}
	return key + "=" + value
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
	path := ""
	if decoded, ok := runtimeDecodedBase64RowField(details, "path"); ok {
		path = decoded
	}
	rest := details
	if strings.TrimSpace(path) == "" {
		path = runtimeLastKeyValueField(details, "path")
	}
	if strings.TrimSpace(path) == "" {
		path = details
		rest = ""
		if before, after, ok := strings.Cut(details, " "); ok {
			path = strings.TrimSpace(before)
			rest = strings.TrimSpace(after)
		}
	}
	fields := runtimeSimpleKeyValueFields(rest)
	machineRest := runtimeRowWithoutLegacyTextFields(rest, "reason", "reviewer", "reviewed_at", "path")
	reviewer := runtimeRowField(machineRest, "reviewer")
	if decoded, ok := runtimeDecodedBase64RowField(rest, "reviewer"); ok {
		reviewer = decoded
	}
	reviewedAt := runtimeRowField(machineRest, "reviewed_at")
	if decoded, ok := runtimeDecodedBase64RowField(rest, "reviewed_at"); ok {
		reviewedAt = decoded
	}
	return RuntimeAction{
		Kind:          "compile-preview-review-artifact",
		State:         runtimeRowField(machineRest, "status"),
		ExecutionMode: runtimeRowField(machineRest, "mode"),
		ArtifactPath:  path,
		ActionType:    runtimeRowField(machineRest, "action"),
		Nodes:         runtimeRowField(machineRest, "nodes"),
		Warnings:      runtimeRowField(machineRest, "warnings"),
		Current:       runtimeRowField(machineRest, "current"),
		Decision:      runtimeRowField(machineRest, "decision"),
		Reviewer:      reviewer,
		ReviewedAt:    reviewedAt,
		Reason:        runtimeDecodedRowField(rest, fields, "reason"),
		Row:           row,
	}
}

func runtimeCompilePreviewReviewComparisonActionForRow(row string) RuntimeAction {
	details := strings.TrimSpace(strings.TrimPrefix(row, "compile-preview review comparison:"))
	machineDetails := runtimeRowWithoutLegacyTextFields(details, "base", "candidate")
	basePath := runtimeRowField(machineDetails, "base")
	if decoded, ok := runtimeDecodedBase64RowField(details, "base"); ok {
		basePath = decoded
	}
	candidatePath := runtimeRowField(machineDetails, "candidate")
	if decoded, ok := runtimeDecodedBase64RowField(details, "candidate"); ok {
		candidatePath = decoded
	}
	return RuntimeAction{
		Kind:          "compile-preview-review-comparison",
		ExecutionMode: runtimeRowField(machineDetails, "mode"),
		BasePath:      basePath,
		CandidatePath: candidatePath,
		ActionType:    runtimeRowField(machineDetails, "action"),
		NodeDelta:     runtimeRowField(machineDetails, "node_delta"),
		WarningDelta:  runtimeRowField(machineDetails, "warning_delta"),
		Added:         runtimeRowField(machineDetails, "added"),
		Removed:       runtimeRowField(machineDetails, "removed"),
		Changed:       runtimeRowField(machineDetails, "changed"),
		Reordered:     runtimeRowField(machineDetails, "reordered"),
		ScoreChanged:  runtimeRowField(machineDetails, "score_changed"),
		SignalChanged: runtimeRowField(machineDetails, "signal_changed"),
		Row:           row,
	}
}

func runtimeCompilePreviewReviewSelectionActionForRow(row string) RuntimeAction {
	details := strings.TrimSpace(strings.TrimPrefix(row, "compile-preview review selection:"))
	fields := runtimeSimpleKeyValueFields(details)
	machineDetails := runtimeRowWithoutLegacyTextFields(details, "reason", "review", "selected_by", "selected_at", "cleared_by", "cleared_at")
	reviewPath := runtimeRowField(machineDetails, "review")
	if decoded, ok := runtimeDecodedBase64RowField(details, "review"); ok {
		reviewPath = decoded
	}
	selectedBy := runtimeRowField(machineDetails, "selected_by")
	if decoded, ok := runtimeDecodedBase64RowField(details, "selected_by"); ok {
		selectedBy = decoded
	}
	selectedAt := runtimeRowField(machineDetails, "selected_at")
	if decoded, ok := runtimeDecodedBase64RowField(details, "selected_at"); ok {
		selectedAt = decoded
	}
	clearedBy := runtimeRowField(machineDetails, "cleared_by")
	if decoded, ok := runtimeDecodedBase64RowField(details, "cleared_by"); ok {
		clearedBy = decoded
	}
	clearedAt := runtimeRowField(machineDetails, "cleared_at")
	if decoded, ok := runtimeDecodedBase64RowField(details, "cleared_at"); ok {
		clearedAt = decoded
	}
	return RuntimeAction{
		Kind:          "compile-preview-review-selection",
		State:         runtimeRowField(machineDetails, "status"),
		ExecutionMode: runtimeRowField(machineDetails, "mode"),
		ReviewPath:    reviewPath,
		ActionType:    runtimeRowField(machineDetails, "action"),
		Superseded:    runtimeRowField(machineDetails, "superseded"),
		SelectedBy:    selectedBy,
		SelectedAt:    selectedAt,
		ClearedBy:     clearedBy,
		ClearedAt:     clearedAt,
		Reason:        runtimeDecodedRowField(details, fields, "reason"),
		Row:           row,
	}
}

func runtimeCompilePreviewReviewExecutionArtifactActionForRow(row string) RuntimeAction {
	details := strings.TrimSpace(strings.TrimPrefix(row, "compile-preview review execution artifact:"))
	fields := runtimeSimpleKeyValueFields(details)
	machineDetails := runtimeRowWithoutLegacyTextFields(details, "reason", "error")
	events := runtimeRowField(machineDetails, "events")
	results := ""
	if strings.TrimSpace(events) != "" {
		results = "events=" + events
	}
	return RuntimeAction{
		Kind:           "compile-preview-review-execution-artifact",
		State:          runtimeRowField(machineDetails, "status"),
		ExecutionMode:  runtimeRowField(machineDetails, "mode"),
		ActionType:     runtimeRowField(machineDetails, "action"),
		Nodes:          runtimeRowField(machineDetails, "nodes"),
		Warnings:       runtimeRowField(machineDetails, "warnings"),
		Results:        results,
		CreatedBy:      runtimeRowField(machineDetails, "actor"),
		CreatedAt:      runtimeRowField(machineDetails, "executed_at"),
		RunID:          runtimeRowField(machineDetails, "run"),
		DispatchPath:   runtimeRowField(machineDetails, "dispatch"),
		ReviewPath:     runtimeRowField(machineDetails, "review"),
		RequestPath:    runtimeRowField(machineDetails, "selection"),
		ArtifactPath:   runtimeRowField(machineDetails, "path"),
		Reason:         runtimeDecodedRowField(details, fields, "reason"),
		ExecutionError: runtimeDecodedRowField(details, fields, "error"),
		Row:            row,
	}
}

func runtimeCompilePreviewReviewExecutionDispatchArtifactActionForRow(row string) RuntimeAction {
	details := strings.TrimSpace(strings.TrimPrefix(row, "compile-preview review execution dispatch artifact:"))
	fields := runtimeSimpleKeyValueFields(details)
	machineDetails := runtimeRowWithoutLegacyTextFields(details, "reason")
	return RuntimeAction{
		Kind:          "compile-preview-review-execution-dispatch-artifact",
		State:         runtimeRowField(machineDetails, "status"),
		ExecutionMode: runtimeRowField(machineDetails, "mode"),
		ActionType:    runtimeRowField(machineDetails, "action"),
		Nodes:         runtimeRowField(machineDetails, "nodes"),
		Warnings:      runtimeRowField(machineDetails, "warnings"),
		CreatedBy:     runtimeRowField(machineDetails, "actor"),
		CreatedAt:     runtimeRowField(machineDetails, "created_at"),
		ReviewPath:    runtimeRowField(machineDetails, "review"),
		RequestPath:   runtimeRowField(machineDetails, "selection"),
		ArtifactPath:  runtimeRowField(machineDetails, "path"),
		Reason:        runtimeDecodedRowField(details, fields, "reason"),
		Row:           row,
	}
}

func runtimeReplayWorkbenchSessionArtifactActionForRow(row string) RuntimeAction {
	details := strings.TrimSpace(strings.TrimPrefix(row, "replay workbench session artifact:"))
	machineDetails := runtimeRowWithoutLegacyTextFields(details, "reason", "review_reason")
	fields := runtimeSimpleKeyValueFields(machineDetails)
	return RuntimeAction{
		Kind:          "replay-workbench-session-artifact",
		State:         runtimeRowField(machineDetails, "status"),
		ExecutionMode: runtimeRowField(machineDetails, "mode"),
		RunID:         runtimeRowField(machineDetails, "primary"),
		SelectedRunID: runtimeRowField(machineDetails, "selected"),
		RunIDs:        runtimeRowField(machineDetails, "runs"),
		Filters:       runtimeRowField(machineDetails, "filters"),
		Endpoints:     runtimeRowField(machineDetails, "endpoints"),
		CreatedBy:     runtimeRowField(machineDetails, "created_by"),
		CreatedAt:     runtimeRowField(machineDetails, "created_at"),
		Reason:        runtimeDecodedRowField(details, fields, "reason"),
		Decision:      runtimeRowField(machineDetails, "decision"),
		Reviewer:      runtimeRowField(machineDetails, "reviewer"),
		ReviewedAt:    runtimeRowField(machineDetails, "reviewed_at"),
		ReviewReason:  runtimeDecodedRowField(details, fields, "review_reason"),
		Current:       runtimeRowField(machineDetails, "current"),
		ArtifactPath:  runtimeRowField(machineDetails, "path"),
		Row:           row,
	}
}

func runtimeReplayWorkbenchCollaborationArtifactActionForRow(row string) RuntimeAction {
	details := strings.TrimSpace(strings.TrimPrefix(row, "replay workbench collaboration artifact:"))
	machineDetails := runtimeRowWithoutLegacyTextFields(details, "reason", "review_reason")
	fields := runtimeSimpleKeyValueFields(machineDetails)
	return RuntimeAction{
		Kind:          "replay-workbench-collaboration-artifact",
		State:         runtimeRowField(machineDetails, "status"),
		ExecutionMode: runtimeRowField(machineDetails, "mode"),
		SessionPath:   runtimeRowField(machineDetails, "session_path"),
		RunID:         runtimeRowField(machineDetails, "primary"),
		SelectedRunID: runtimeRowField(machineDetails, "selected"),
		RunIDs:        runtimeRowField(machineDetails, "runs"),
		Filters:       runtimeRowField(machineDetails, "filters"),
		Endpoints:     runtimeRowField(machineDetails, "endpoints"),
		Participants:  runtimeRowField(machineDetails, "participants"),
		CreatedBy:     runtimeRowField(machineDetails, "created_by"),
		CreatedAt:     runtimeRowField(machineDetails, "created_at"),
		Reason:        runtimeDecodedRowField(details, fields, "reason"),
		Decision:      runtimeRowField(machineDetails, "decision"),
		Reviewer:      runtimeRowField(machineDetails, "reviewer"),
		ReviewedAt:    runtimeRowField(machineDetails, "reviewed_at"),
		ReviewReason:  runtimeDecodedRowField(details, fields, "review_reason"),
		Current:       runtimeRowField(machineDetails, "current"),
		ArtifactPath:  runtimeRowField(machineDetails, "path"),
		Row:           row,
	}
}

func runtimeReplayWorkbenchCollaborationPresenceActionForRow(row string) RuntimeAction {
	details := strings.TrimSpace(strings.TrimPrefix(row, "replay workbench collaboration presence artifact:"))
	machineDetails := runtimeRowWithoutLegacyTextFields(details, "reason")
	fields := runtimeSimpleKeyValueFields(machineDetails)
	return RuntimeAction{
		Kind:              "replay-workbench-collaboration-presence",
		State:             runtimeRowField(machineDetails, "status"),
		ExecutionMode:     runtimeRowField(machineDetails, "mode"),
		RunID:             runtimeRowField(machineDetails, "primary"),
		SelectedRunID:     runtimeRowField(machineDetails, "selected"),
		Participant:       runtimeRowField(machineDetails, "participant"),
		SeenBy:            runtimeRowField(machineDetails, "seen_by"),
		SeenAt:            runtimeRowField(machineDetails, "seen_at"),
		Reason:            runtimeDecodedRowField(details, fields, "reason"),
		ArtifactPath:      runtimeRowField(machineDetails, "path"),
		CollaborationPath: runtimeRowField(machineDetails, "collaboration"),
		SessionPath:       runtimeRowField(machineDetails, "session_path"),
		Row:               row,
	}
}

func runtimeReplayWorkbenchCollaborationNoteActionForRow(row string) RuntimeAction {
	details := strings.TrimSpace(strings.TrimPrefix(row, "replay workbench collaboration note artifact:"))
	machineDetails := runtimeRowWithoutLegacyTextFields(details, "reason", "body", "review_reason")
	fields := runtimeSimpleKeyValueFields(machineDetails)
	return RuntimeAction{
		Kind:              "replay-workbench-collaboration-note",
		State:             runtimeRowField(machineDetails, "status"),
		ExecutionMode:     runtimeRowField(machineDetails, "mode"),
		RunID:             runtimeRowField(machineDetails, "primary"),
		SelectedRunID:     runtimeRowField(machineDetails, "selected"),
		Author:            runtimeRowField(machineDetails, "author"),
		Target:            runtimeRowField(machineDetails, "target"),
		TargetKind:        runtimeRowField(machineDetails, "target_kind"),
		TargetID:          runtimeRowField(machineDetails, "target_id"),
		CreatedAt:         runtimeRowField(machineDetails, "created_at"),
		Reason:            runtimeDecodedRowField(details, fields, "reason"),
		Body:              runtimeDecodedRowField(details, fields, "body"),
		Decision:          runtimeRowField(machineDetails, "decision"),
		Reviewer:          runtimeRowField(machineDetails, "reviewer"),
		ReviewedAt:        runtimeRowField(machineDetails, "reviewed_at"),
		ReviewReason:      runtimeDecodedRowField(details, fields, "review_reason"),
		Current:           runtimeRowField(machineDetails, "current"),
		ArtifactPath:      runtimeRowField(machineDetails, "path"),
		CollaborationPath: runtimeRowField(machineDetails, "collaboration"),
		SessionPath:       runtimeRowField(machineDetails, "session_path"),
		Row:               row,
	}
}

func runtimeReplayWorkbenchCollaborationNoteSelectionActionForRow(row string) RuntimeAction {
	details := strings.TrimSpace(strings.TrimPrefix(row, "replay workbench collaboration note selection:"))
	machineDetails := runtimeRowWithoutLegacyTextFields(details, "reason")
	fields := runtimeSimpleKeyValueFields(machineDetails)
	return RuntimeAction{
		Kind:              "replay-workbench-collaboration-note-selection",
		State:             runtimeRowField(machineDetails, "status"),
		ExecutionMode:     runtimeRowField(machineDetails, "mode"),
		ArtifactPath:      runtimeRowField(machineDetails, "note"),
		CollaborationPath: runtimeRowField(machineDetails, "collaboration"),
		SessionPath:       runtimeRowField(machineDetails, "session"),
		RunID:             runtimeRowField(machineDetails, "primary"),
		SelectedRunID:     runtimeRowField(machineDetails, "selected"),
		Author:            runtimeRowField(machineDetails, "author"),
		Target:            runtimeRowField(machineDetails, "target"),
		TargetKind:        runtimeRowField(machineDetails, "target_kind"),
		TargetID:          runtimeRowField(machineDetails, "target_id"),
		Superseded:        runtimeRowField(machineDetails, "superseded"),
		SelectedBy:        runtimeRowField(machineDetails, "selected_by"),
		SelectedAt:        runtimeRowField(machineDetails, "selected_at"),
		ClearedBy:         runtimeRowField(machineDetails, "cleared_by"),
		ClearedAt:         runtimeRowField(machineDetails, "cleared_at"),
		Reason:            runtimeDecodedRowField(details, fields, "reason"),
		Row:               row,
	}
}

func runtimeReplayWorkbenchCollaborationSelectionActionForRow(row string) RuntimeAction {
	details := strings.TrimSpace(strings.TrimPrefix(row, "replay workbench collaboration selection:"))
	machineDetails := runtimeRowWithoutLegacyTextFields(details, "reason")
	fields := runtimeSimpleKeyValueFields(machineDetails)
	return RuntimeAction{
		Kind:          "replay-workbench-collaboration-selection",
		State:         runtimeRowField(machineDetails, "status"),
		ExecutionMode: runtimeRowField(machineDetails, "mode"),
		ArtifactPath:  runtimeRowField(machineDetails, "collaboration"),
		SessionPath:   runtimeRowField(machineDetails, "session"),
		RunID:         runtimeRowField(machineDetails, "primary"),
		SelectedRunID: runtimeRowField(machineDetails, "selected"),
		RunIDs:        runtimeRowField(machineDetails, "runs"),
		Filters:       runtimeRowField(machineDetails, "filters"),
		Endpoints:     runtimeRowField(machineDetails, "endpoints"),
		Participants:  runtimeRowField(machineDetails, "participants"),
		Superseded:    runtimeRowField(machineDetails, "superseded"),
		SelectedBy:    runtimeRowField(machineDetails, "selected_by"),
		SelectedAt:    runtimeRowField(machineDetails, "selected_at"),
		ClearedBy:     runtimeRowField(machineDetails, "cleared_by"),
		ClearedAt:     runtimeRowField(machineDetails, "cleared_at"),
		Reason:        runtimeDecodedRowField(details, fields, "reason"),
		Row:           row,
	}
}

func runtimeReplayWorkbenchSessionSelectionActionForRow(row string) RuntimeAction {
	details := strings.TrimSpace(strings.TrimPrefix(row, "replay workbench session selection:"))
	machineDetails := runtimeRowWithoutLegacyTextFields(details, "reason")
	fields := runtimeSimpleKeyValueFields(machineDetails)
	return RuntimeAction{
		Kind:          "replay-workbench-session-selection",
		State:         runtimeRowField(machineDetails, "status"),
		ExecutionMode: runtimeRowField(machineDetails, "mode"),
		ArtifactPath:  runtimeRowField(machineDetails, "session"),
		RunID:         runtimeRowField(machineDetails, "primary"),
		SelectedRunID: runtimeRowField(machineDetails, "selected"),
		RunIDs:        runtimeRowField(machineDetails, "runs"),
		Filters:       runtimeRowField(machineDetails, "filters"),
		Endpoints:     runtimeRowField(machineDetails, "endpoints"),
		Superseded:    runtimeRowField(machineDetails, "superseded"),
		SelectedBy:    runtimeRowField(machineDetails, "selected_by"),
		SelectedAt:    runtimeRowField(machineDetails, "selected_at"),
		ClearedBy:     runtimeRowField(machineDetails, "cleared_by"),
		ClearedAt:     runtimeRowField(machineDetails, "cleared_at"),
		Reason:        runtimeDecodedRowField(details, fields, "reason"),
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
	if decoded, after, ok := runtimeLeadingDecodedTextField(details, "artifact"); ok {
		path = decoded
		rest = after
	} else if before, after, ok := strings.Cut(details, " "); ok {
		path = strings.TrimSpace(before)
		rest = strings.TrimSpace(after)
	}
	fields := runtimeSimpleKeyValueFields(rest)
	machineRest := runtimeRowWithoutLegacyTextFields(rest, "dispatch")
	return RuntimeAction{
		Kind:          "install-execution-artifact",
		State:         runtimeRowField(machineRest, "status"),
		ExecutionMode: runtimeRowField(machineRest, "mode"),
		ArtifactPath:  path,
		DispatchPath:  runtimeDecodedRowFieldWithFallbackRow(rest, fields, "dispatch", machineRest),
		Results:       runtimeRowField(machineRest, "results"),
		Skipped:       runtimeRowField(machineRest, "skipped"),
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
	fields := runtimeSimpleKeyValueFields(rest)
	machineRest := runtimeRowWithoutLegacyTextFields(rest, "output", "command", "artifact", "dispatch")
	return RuntimeAction{
		Kind:         "install-execution-result",
		Pack:         pack,
		Name:         dependency,
		State:        firstRuntimeToken(machineRest),
		Command:      runtimeDecodedRowFieldWithFallbackRow(rest, fields, "command", machineRest),
		Output:       runtimeDecodedRowField(rest, fields, "output"),
		ArtifactPath: runtimeDecodedRowFieldWithFallbackRow(rest, fields, "artifact", machineRest),
		DispatchPath: runtimeDecodedRowFieldWithFallbackRow(rest, fields, "dispatch", machineRest),
		Row:          row,
	}
}

func runtimeDispatchArtifactActionForRow(row string) RuntimeAction {
	details := strings.TrimSpace(strings.TrimPrefix(row, "dispatch artifact:"))
	path := details
	rest := ""
	if decoded, after, ok := runtimeLeadingDecodedTextField(details, "artifact"); ok {
		path = decoded
		rest = after
	} else if before, after, ok := strings.Cut(details, " "); ok {
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
	if decoded, after, ok := runtimeLeadingDecodedTextField(details, "artifact"); ok {
		path = decoded
		rest = after
	} else if before, after, ok := strings.Cut(details, " "); ok {
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
	if decoded, after, ok := runtimeLeadingDecodedTextField(details, "artifact"); ok {
		path = decoded
		rest = after
	} else if before, after, ok := strings.Cut(details, " "); ok {
		path = strings.TrimSpace(before)
		rest = strings.TrimSpace(after)
	}
	fields := runtimeSimpleKeyValueFields(rest)
	machineRest := runtimeRowWithoutLegacyTextFields(rest, "error", "dispatch", "execution", "retry_dispatch")
	return RuntimeAction{
		Kind:              "install-orchestration-artifact",
		State:             runtimeRowField(machineRest, "status"),
		ExecutionMode:     runtimeRowField(machineRest, "mode"),
		ArtifactPath:      path,
		Ready:             runtimeRowField(machineRest, "ready"),
		Skipped:           runtimeRowField(machineRest, "skipped"),
		Results:           runtimeRowField(machineRest, "results"),
		DispatchPath:      runtimeDecodedRowFieldWithFallbackRow(rest, fields, "dispatch", machineRest),
		ExecutionPath:     runtimeDecodedRowFieldWithFallbackRow(rest, fields, "execution", machineRest),
		RetryDispatchPath: runtimeDecodedRowFieldWithFallbackRow(rest, fields, "retry_dispatch", machineRest),
		ExecutionError:    runtimeDecodedRowField(rest, fields, "error"),
		Row:               row,
	}
}

func runtimeWorkerOrchestrationArtifactActionForRow(row string) RuntimeAction {
	details := strings.TrimSpace(strings.TrimPrefix(row, "worker orchestration artifact:"))
	path := details
	rest := ""
	if decoded, after, ok := runtimeLeadingDecodedTextField(details, "artifact"); ok {
		path = decoded
		rest = after
	} else if before, after, ok := strings.Cut(details, " "); ok {
		path = strings.TrimSpace(before)
		rest = strings.TrimSpace(after)
	}
	fields := runtimeSimpleKeyValueFields(rest)
	machineRest := runtimeRowWithoutLegacyTextFields(rest, "placement_plan", "placement_dispatch", "placement_batch", "lifecycle_maintenance", "lifecycle_audit")
	return RuntimeAction{
		Kind:                       "worker-orchestration-artifact",
		State:                      runtimeRowField(machineRest, "status"),
		ExecutionMode:              runtimeRowField(machineRest, "mode"),
		TransportMode:              runtimeRowField(machineRest, "transport"),
		ArtifactPath:               path,
		Ready:                      runtimeRowField(machineRest, "placement_ready"),
		Skipped:                    runtimeRowField(machineRest, "placement_skipped"),
		Claimed:                    runtimeRowField(machineRest, "claimed"),
		Results:                    runtimeRowField(machineRest, "dispatched"),
		Failed:                     runtimeRowField(machineRest, "failed"),
		PlacementPlanPath:          runtimeDecodedRowField(rest, fields, "placement_plan"),
		PlacementDispatchPlanPath:  runtimeDecodedRowField(rest, fields, "placement_dispatch"),
		PlacementDispatchBatchPath: runtimeDecodedRowField(rest, fields, "placement_batch"),
		LifecycleMaintenancePath:   runtimeDecodedRowField(rest, fields, "lifecycle_maintenance"),
		LifecycleAuditPath:         runtimeDecodedRowField(rest, fields, "lifecycle_audit"),
		Row:                        row,
	}
}

func runtimeWorkerOrchestrationSupervisorArtifactActionForRow(row string) RuntimeAction {
	details := strings.TrimSpace(strings.TrimPrefix(row, "worker orchestration supervisor artifact:"))
	path := details
	rest := ""
	if decoded, after, ok := runtimeLeadingDecodedTextField(details, "artifact"); ok {
		path = decoded
		rest = after
	} else if before, after, ok := strings.Cut(details, " "); ok {
		path = strings.TrimSpace(before)
		rest = strings.TrimSpace(after)
	}
	fields := runtimeSimpleKeyValueFields(rest)
	machineRest := runtimeRowWithoutLegacyTextFields(rest, "orchestration_path")
	results := []string{}
	for _, key := range []string{"max_runs", "orchestrations_created", "lifecycle_maintenance", "lifecycle_audit", "retry_dispatch_executions_created", "retry_dispatch_claims_created", "retry_dispatch_claims_dispatched", "retry_dispatch_transport_requests_created", "dispatch_lifecycle_retry_claims", "lifecycle_retry_transport", "claimed", "dispatched"} {
		if value := runtimeRowField(machineRest, key); value != "" {
			results = append(results, key+"="+value)
		}
	}
	return RuntimeAction{
		Kind:          "worker-orchestration-supervisor-artifact",
		State:         runtimeRowField(machineRest, "status"),
		ExecutionMode: runtimeRowField(machineRest, "mode"),
		TransportMode: runtimeRowField(machineRest, "transport"),
		ArtifactPath:  path,
		Ready:         runtimeRowField(machineRest, "runs"),
		Results:       strings.Join(results, " "),
		Skipped:       runtimeRowField(machineRest, "skipped"),
		Failed:        runtimeRowField(machineRest, "failed"),
		DispatchPath:  runtimeDecodedRowField(rest, fields, "orchestration_path"),
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
	fields := runtimeSimpleKeyValueFields(rest)
	machineRest := runtimeRowWithoutLegacyTextFields(rest, "reason")
	results := strings.Join(nonEmptyRuntimeParts([]string{
		"total=" + runtimeRowField(machineRest, "total"),
		"ready=" + runtimeRowField(machineRest, "ready"),
		"results=" + runtimeRowField(machineRest, "results"),
		"actions=" + runtimeRowField(machineRest, "actions"),
		"ack_actions=" + runtimeRowField(machineRest, "ack_actions"),
		"run_actions=" + runtimeRowField(machineRest, "run_actions"),
		"daemon_control_actions=" + runtimeRowField(machineRest, "daemon_control_actions"),
		"retry_dispatch_execute=" + runtimeRowField(machineRest, "retry_dispatch_execute"),
		"reclaim_actions=" + runtimeRowField(machineRest, "reclaim_actions"),
		"reconciliation_actions=" + runtimeRowField(machineRest, "reconciliation_actions"),
		"placement_ready=" + runtimeRowField(machineRest, "placement_ready"),
		"placement_skipped=" + runtimeRowField(machineRest, "placement_skipped"),
		"dispatch_ready=" + runtimeRowField(machineRest, "dispatch_ready"),
		"dispatch_skipped=" + runtimeRowField(machineRest, "dispatch_skipped"),
		"runs=" + runtimeRowField(machineRest, "runs"),
		"max_runs=" + runtimeRowField(machineRest, "max_runs"),
		"orchestrations_created=" + runtimeRowField(machineRest, "orchestrations_created"),
		"lifecycle_maintenance=" + runtimeRowField(machineRest, "lifecycle_maintenance"),
		"lifecycle_audit=" + runtimeRowField(machineRest, "lifecycle_audit"),
		"claimed=" + runtimeRowField(machineRest, "claimed"),
		"dispatched=" + runtimeRowField(machineRest, "dispatched"),
	}), " ")
	skipped := strings.Join(nonEmptyRuntimeParts([]string{
		"skipped_unsafe=" + runtimeRowField(machineRest, "skipped_unsafe"),
		"skipped=" + runtimeRowField(machineRest, "skipped"),
	}), " ")
	failed := strings.Join(nonEmptyRuntimeParts([]string{
		"failed=" + runtimeRowField(machineRest, "failed"),
	}), " ")
	return RuntimeAction{
		Kind:          "worker-lifecycle-audit-item",
		Name:          runtimeRowField(machineRest, "kind"),
		State:         runtimeRowField(machineRest, "status"),
		ExecutionMode: runtimeOptionalRowField(machineRest, "mode"),
		Gate:          runtimeOptionalRowField(machineRest, "gate"),
		WorkerID:      runtimeOptionalRowField(machineRest, "worker"),
		RunID:         runtimeOptionalRowField(machineRest, "run"),
		ExecutionID:   runtimeOptionalRowField(machineRest, "execution"),
		Reason:        runtimeOptionalDecodedRowField(rest, fields, "reason"),
		ArtifactPath:  runtimeOptionalRowField(machineRest, "path"),
		DispatchPath:  runtimeOptionalRowField(machineRest, "linked"),
		Decision:      runtimeRowField(machineRest, "decision"),
		Reviewer:      runtimeRowField(machineRest, "reviewer"),
		ReviewedAt:    runtimeRowField(machineRest, "reviewed_at"),
		SelectedBy:    runtimeRowField(machineRest, "selected_by"),
		SelectedAt:    runtimeRowField(machineRest, "selected_at"),
		ClearedBy:     runtimeRowField(machineRest, "cleared_by"),
		ClearedAt:     runtimeRowField(machineRest, "cleared_at"),
		Results:       results,
		Skipped:       skipped,
		Failed:        failed,
		Row:           row,
	}
}

func runtimeWorkerLifecycleActionPlanArtifactActionForRow(row string) RuntimeAction {
	details := strings.TrimSpace(strings.TrimPrefix(row, "worker lifecycle action plan artifact:"))
	path, rest, ok := runtimeLeadingDecodedTextField(details, "artifact")
	if !ok {
		path = details
		rest = ""
		if before, after, ok := strings.Cut(details, " "); ok {
			path = strings.TrimSpace(before)
			rest = strings.TrimSpace(after)
		}
	}
	fields := runtimeSimpleKeyValueFields(rest)
	machineRest := runtimeRowWithoutLegacyTextFields(rest, "review_reason", "review", "reviewer", "reviewed_at")
	results := []string{}
	for _, key := range []string{"ack", "run", "daemon_control", "retry_dispatch_execute", "reclaim", "reconcile"} {
		if value := runtimeRowField(machineRest, key); value != "" {
			results = append(results, key+"="+value)
		}
	}
	reviewPath := runtimeRowField(machineRest, "review")
	if decoded, ok := runtimeDecodedBase64RowField(rest, "review"); ok {
		reviewPath = decoded
	}
	reviewer := runtimeRowField(machineRest, "reviewer")
	if decoded, ok := runtimeDecodedBase64RowField(rest, "reviewer"); ok {
		reviewer = decoded
	}
	reviewedAt := runtimeRowField(machineRest, "reviewed_at")
	if decoded, ok := runtimeDecodedBase64RowField(rest, "reviewed_at"); ok {
		reviewedAt = decoded
	}
	return RuntimeAction{
		Kind:          "worker-lifecycle-action-plan-artifact",
		State:         runtimeRowField(machineRest, "status"),
		ExecutionMode: runtimeRowField(machineRest, "mode"),
		ArtifactPath:  path,
		ReviewPath:    reviewPath,
		Ready:         runtimeRowField(machineRest, "actions"),
		Results:       strings.Join(results, " "),
		Current:       runtimeRowField(machineRest, "current"),
		Decision:      runtimeRowField(machineRest, "decision"),
		Reviewer:      reviewer,
		ReviewedAt:    reviewedAt,
		ReviewReason:  runtimeDecodedRowField(rest, fields, "review_reason"),
		Row:           row,
	}
}

func runtimeWorkerLifecycleActionPlanSelectionActionForRow(row string) RuntimeAction {
	details := strings.TrimSpace(strings.TrimPrefix(row, "worker lifecycle action plan selection:"))
	fields := runtimeSimpleKeyValueFields(details)
	machineDetails := runtimeRowWithoutLegacyTextFields(details, "reason", "plan", "selection", "review", "selected_by", "selected_at", "cleared_by", "cleared_at")
	results := []string{}
	for _, key := range []string{"ack", "run", "daemon_control", "retry_dispatch_execute", "reclaim", "reconcile"} {
		if value := runtimeRowField(machineDetails, key); value != "" {
			results = append(results, key+"="+value)
		}
	}
	planPath := runtimeRowField(machineDetails, "plan")
	if decoded, ok := runtimeDecodedBase64RowField(details, "plan"); ok {
		planPath = decoded
	}
	selectionPath := runtimeRowField(machineDetails, "selection")
	if decoded, ok := runtimeDecodedBase64RowField(details, "selection"); ok {
		selectionPath = decoded
	}
	reviewPath := runtimeRowField(machineDetails, "review")
	if decoded, ok := runtimeDecodedBase64RowField(details, "review"); ok {
		reviewPath = decoded
	}
	selectedBy := runtimeRowField(machineDetails, "selected_by")
	if decoded, ok := runtimeDecodedBase64RowField(details, "selected_by"); ok {
		selectedBy = decoded
	}
	selectedAt := runtimeRowField(machineDetails, "selected_at")
	if decoded, ok := runtimeDecodedBase64RowField(details, "selected_at"); ok {
		selectedAt = decoded
	}
	clearedBy := runtimeRowField(machineDetails, "cleared_by")
	if decoded, ok := runtimeDecodedBase64RowField(details, "cleared_by"); ok {
		clearedBy = decoded
	}
	clearedAt := runtimeRowField(machineDetails, "cleared_at")
	if decoded, ok := runtimeDecodedBase64RowField(details, "cleared_at"); ok {
		clearedAt = decoded
	}
	return RuntimeAction{
		Kind:          "worker-lifecycle-action-plan-selection",
		State:         runtimeRowField(machineDetails, "status"),
		ExecutionMode: runtimeRowField(machineDetails, "mode"),
		ArtifactPath:  planPath,
		RequestPath:   selectionPath,
		ReviewPath:    reviewPath,
		Current:       runtimeRowField(machineDetails, "current"),
		Ready:         runtimeRowField(machineDetails, "actions"),
		Results:       strings.Join(results, " "),
		Superseded:    runtimeRowField(machineDetails, "superseded"),
		SelectedBy:    selectedBy,
		SelectedAt:    selectedAt,
		ClearedBy:     clearedBy,
		ClearedAt:     clearedAt,
		Reason:        runtimeDecodedRowField(details, fields, "reason"),
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
	path, rest, ok := runtimeLeadingDecodedTextField(details, "artifact")
	if !ok {
		path = details
		rest = ""
		if before, after, ok := strings.Cut(details, " "); ok {
			path = strings.TrimSpace(before)
			rest = strings.TrimSpace(after)
		}
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
	path, rest, ok := runtimeLeadingDecodedTextField(details, "artifact")
	if !ok {
		path = details
		rest = ""
		if before, after, ok := strings.Cut(details, " "); ok {
			path = strings.TrimSpace(before)
			rest = strings.TrimSpace(after)
		}
	}
	fields := runtimeSimpleKeyValueFields(rest)
	machineRest := runtimeRowWithoutLegacyTextFields(rest, "skip", "policy", "selection")
	results := strings.Join(nonEmptyRuntimeParts([]string{
		"max_runs=" + runtimeRowField(machineRest, "max_runs"),
		"poll_interval_ms=" + runtimeRowField(machineRest, "poll_interval_ms"),
		"include_lifecycle_maintenance=" + runtimeRowField(machineRest, "include_lifecycle_maintenance"),
		"execute_lifecycle_retries=" + runtimeRowField(machineRest, "execute_lifecycle_retries"),
		"dispatch_lifecycle_retry_claims=" + runtimeRowField(machineRest, "dispatch_lifecycle_retry_claims"),
		"lifecycle_retry_transport=" + runtimeRowField(machineRest, "lifecycle_retry_transport"),
		"retry_dispatch_claims_dispatched=" + runtimeRowField(machineRest, "retry_dispatch_claims_dispatched"),
		"retry_dispatch_transport_requests_created=" + runtimeRowField(machineRest, "retry_dispatch_transport_requests_created"),
		"execute_lease_recovery=" + runtimeRowField(machineRest, "execute_lease_recovery"),
		"supervisor_created=" + runtimeRowField(machineRest, "supervisor_created"),
	}), " ")
	policyPath := runtimeRowField(machineRest, "policy")
	if decoded, ok := runtimeDecodedBase64RowField(rest, "policy"); ok {
		policyPath = decoded
	}
	selectionPath := runtimeRowField(machineRest, "selection")
	if decoded, ok := runtimeDecodedBase64RowField(rest, "selection"); ok {
		selectionPath = decoded
	}
	return RuntimeAction{
		Kind:          "worker-lifecycle-policy-run-artifact",
		State:         runtimeRowField(machineRest, "status"),
		ExecutionMode: runtimeRowField(machineRest, "mode"),
		ArtifactPath:  path,
		Source:        policyPath,
		RequestPath:   selectionPath,
		TransportMode: runtimeRowField(machineRest, "transport"),
		Results:       results,
		Skipped:       runtimeDecodedRowField(rest, fields, "skip"),
		Row:           row,
	}
}

func runtimeWorkerLifecyclePolicySchedulerRunArtifactActionForRow(row string) RuntimeAction {
	details := strings.TrimSpace(strings.TrimPrefix(row, "worker lifecycle policy scheduler run artifact:"))
	path, rest, ok := runtimeLeadingDecodedTextField(details, "artifact")
	if !ok {
		path = details
		rest = ""
		if before, after, ok := strings.Cut(details, " "); ok {
			path = strings.TrimSpace(before)
			rest = strings.TrimSpace(after)
		}
	}
	fields := runtimeSimpleKeyValueFields(rest)
	machineRest := runtimeRowWithoutLegacyTextFields(rest, "skip", "error", "policy", "selection", "policy_basename", "selection_basename")
	results := strings.Join(nonEmptyRuntimeParts([]string{
		"max_runs=" + runtimeRowField(machineRest, "max_runs"),
		"poll_interval_ms=" + runtimeRowField(machineRest, "poll_interval_ms"),
		"policy_runs_created=" + runtimeRowField(machineRest, "policy_runs_created"),
		"supervisor_created=" + runtimeRowField(machineRest, "supervisor_created"),
		"include_lifecycle_maintenance=" + runtimeRowField(machineRest, "include_lifecycle_maintenance"),
		"execute_lifecycle_retries=" + runtimeRowField(machineRest, "execute_lifecycle_retries"),
		"dispatch_lifecycle_retry_claims=" + runtimeRowField(machineRest, "dispatch_lifecycle_retry_claims"),
		"lifecycle_retry_transport=" + runtimeRowField(machineRest, "lifecycle_retry_transport"),
		"execute_lease_recovery=" + runtimeRowField(machineRest, "execute_lease_recovery"),
	}), " ")
	skipped := runtimeOptionalDecodedRowField(rest, fields, "skip")
	failed := runtimeOptionalDecodedRowField(rest, fields, "error")
	policyPath := runtimeRowField(machineRest, "policy")
	if decoded, ok := runtimeDecodedBase64RowField(rest, "policy"); ok {
		policyPath = decoded
	}
	selectionPath := runtimeRowField(machineRest, "selection")
	if decoded, ok := runtimeDecodedBase64RowField(rest, "selection"); ok {
		selectionPath = decoded
	}
	policyName := runtimeRowField(machineRest, "policy_basename")
	if decoded, ok := runtimeDecodedBase64RowField(rest, "policy_basename"); ok {
		policyName = decoded
	}
	selectionName := runtimeRowField(machineRest, "selection_basename")
	if decoded, ok := runtimeDecodedBase64RowField(rest, "selection_basename"); ok {
		selectionName = decoded
	}
	return RuntimeAction{
		Kind:          "worker-lifecycle-policy-scheduler-run-artifact",
		Name:          policyName,
		State:         runtimeRowField(machineRest, "status"),
		ExecutionMode: runtimeRowField(machineRest, "mode"),
		ArtifactPath:  path,
		Source:        policyPath,
		Target:        selectionName,
		RequestPath:   selectionPath,
		TransportMode: runtimeRowField(machineRest, "transport"),
		Results:       results,
		Skipped:       skipped,
		Failed:        failed,
		Row:           row,
	}
}

func runtimeWorkerLifecyclePolicySelectionActionForRow(row string) RuntimeAction {
	details := strings.TrimSpace(strings.TrimPrefix(row, "worker lifecycle policy selection:"))
	fields := runtimeSimpleKeyValueFields(details)
	machineDetails := runtimeRowWithoutLegacyTextFields(details, "reason", "policy", "selection", "selected_by", "selected_at", "cleared_by", "cleared_at")
	results := strings.Join(nonEmptyRuntimeParts([]string{
		"poll_interval_ms=" + runtimeRowField(machineDetails, "poll_interval_ms"),
		"max_runs=" + runtimeRowField(machineDetails, "max_runs"),
		"dispatch_lifecycle_retry_claims=" + runtimeRowField(machineDetails, "dispatch_lifecycle_retry_claims"),
		"lifecycle_retry_transport=" + runtimeRowField(machineDetails, "lifecycle_retry_transport"),
	}), " ")
	policyPath := runtimeRowField(machineDetails, "policy")
	if decoded, ok := runtimeDecodedBase64RowField(details, "policy"); ok {
		policyPath = decoded
	}
	selectionPath := runtimeRowField(machineDetails, "selection")
	if decoded, ok := runtimeDecodedBase64RowField(details, "selection"); ok {
		selectionPath = decoded
	}
	selectedBy := runtimeRowField(machineDetails, "selected_by")
	if decoded, ok := runtimeDecodedBase64RowField(details, "selected_by"); ok {
		selectedBy = decoded
	}
	selectedAt := runtimeRowField(machineDetails, "selected_at")
	if decoded, ok := runtimeDecodedBase64RowField(details, "selected_at"); ok {
		selectedAt = decoded
	}
	clearedBy := runtimeRowField(machineDetails, "cleared_by")
	if decoded, ok := runtimeDecodedBase64RowField(details, "cleared_by"); ok {
		clearedBy = decoded
	}
	clearedAt := runtimeRowField(machineDetails, "cleared_at")
	if decoded, ok := runtimeDecodedBase64RowField(details, "cleared_at"); ok {
		clearedAt = decoded
	}
	return RuntimeAction{
		Kind:          "worker-lifecycle-policy-selection",
		State:         runtimeRowField(machineDetails, "status"),
		ExecutionMode: runtimeRowField(machineDetails, "mode"),
		ArtifactPath:  policyPath,
		RequestPath:   selectionPath,
		TransportMode: runtimeRowField(machineDetails, "transport"),
		Results:       results,
		Superseded:    runtimeRowField(machineDetails, "superseded"),
		SelectedBy:    selectedBy,
		SelectedAt:    selectedAt,
		ClearedBy:     clearedBy,
		ClearedAt:     clearedAt,
		Reason:        runtimeDecodedRowField(details, fields, "reason"),
		Row:           row,
	}
}

func runtimeWorkerLifecycleActionForRow(row string) RuntimeAction {
	rest := strings.TrimSpace(strings.TrimPrefix(row, "worker lifecycle action:"))
	fields := runtimeSimpleKeyValueFields(rest)
	machineRest := runtimeRowWithoutLegacyTextFields(rest, "reason", "worker", "run", "execution")
	return RuntimeAction{
		Kind:              "worker-lifecycle-action",
		Name:              runtimeRowField(machineRest, "type"),
		Gate:              runtimeRowField(machineRest, "gate"),
		State:             runtimeRowField(machineRest, "status"),
		ExecutionMode:     runtimeRowField(machineRest, "mode"),
		WorkerID:          runtimeDecodedRowField(rest, fields, "worker"),
		RunID:             runtimeDecodedRowField(rest, fields, "run"),
		ExecutionID:       runtimeDecodedRowField(rest, fields, "execution"),
		Reason:            runtimeDecodedRowField(rest, fields, "reason"),
		RequestPath:       runtimeOptionalRowField(machineRest, "request_path"),
		LeasePath:         runtimeOptionalRowField(machineRest, "lease_path"),
		ResultPath:        runtimeOptionalRowField(machineRest, "result_path"),
		DispatchPath:      runtimeOptionalRowField(machineRest, "dispatch"),
		RecommendedStatus: runtimeOptionalRowField(machineRest, "recommended"),
		Row:               row,
	}
}

func runtimeWorkerLeaseResultReconciliationArtifactActionForRow(row string) RuntimeAction {
	details := strings.TrimSpace(strings.TrimPrefix(row, "worker lease result reconciliation artifact:"))
	path := details
	rest := ""
	if decoded, after, ok := runtimeLeadingDecodedTextField(details, "artifact"); ok {
		path = decoded
		rest = after
	} else if before, after, ok := strings.Cut(details, " "); ok {
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
	if decoded, after, ok := runtimeLeadingDecodedTextField(details, "artifact"); ok {
		path = decoded
		rest = after
	} else if before, after, ok := strings.Cut(details, " "); ok {
		path = strings.TrimSpace(before)
		rest = strings.TrimSpace(after)
	}
	fields := runtimeSimpleKeyValueFields(rest)
	machineRest := runtimeRowWithoutLegacyTextFields(rest, "dispatch")
	return RuntimeAction{
		Kind:          "worker-lease-result-reconciliation-execution-artifact",
		State:         runtimeRowField(machineRest, "status"),
		ExecutionMode: runtimeRowField(machineRest, "mode"),
		ArtifactPath:  path,
		DispatchPath:  runtimeDecodedRowField(rest, fields, "dispatch"),
		Results:       runtimeRowField(machineRest, "results"),
		Skipped:       runtimeRowField(machineRest, "skipped"),
		Row:           row,
	}
}

func runtimeWorkerLeaseResultReconciliationExecutionResultActionForRow(row string) RuntimeAction {
	details := strings.TrimSpace(strings.TrimPrefix(row, "lease result reconciliation execution result "))
	fields := runtimeSimpleKeyValueFields(details)
	machineDetails := runtimeRowWithoutLegacyTextFields(details, "run", "execution", "worker", "lease_path", "result_path", "dispatch")
	runID := runtimeDecodedRowField(details, fields, "run")
	executionID := runtimeDecodedRowField(details, fields, "execution")
	name := strings.TrimSpace(runID)
	if name != "" && executionID != "" {
		name += "/" + executionID
	}
	if name == "" {
		name = details
	}
	workerID := runtimeDecodedRowField(details, fields, "worker")
	return RuntimeAction{
		Kind:         "worker-lease-result-reconciliation-execution-result",
		Name:         name,
		State:        runtimeRowField(machineDetails, "lease"),
		Output:       runtimeRowField(machineDetails, "result"),
		Source:       workerID,
		WorkerID:     workerID,
		RunID:        runID,
		ExecutionID:  executionID,
		LeasePath:    runtimeDecodedRowField(details, fields, "lease_path"),
		ResultPath:   runtimeDecodedRowField(details, fields, "result_path"),
		DispatchPath: runtimeDecodedRowField(details, fields, "dispatch"),
		Row:          row,
	}
}

func runtimeWorkerLeaseReclaimArtifactActionForRow(row string) RuntimeAction {
	details := strings.TrimSpace(strings.TrimPrefix(row, "worker lease reclaim artifact:"))
	path := details
	rest := ""
	if decoded, after, ok := runtimeLeadingDecodedTextField(details, "artifact"); ok {
		path = decoded
		rest = after
	} else if before, after, ok := strings.Cut(details, " "); ok {
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
	if decoded, after, ok := runtimeLeadingDecodedTextField(details, "artifact"); ok {
		path = decoded
		rest = after
	} else if before, after, ok := strings.Cut(details, " "); ok {
		path = strings.TrimSpace(before)
		rest = strings.TrimSpace(after)
	}
	fields := runtimeSimpleKeyValueFields(rest)
	machineRest := runtimeRowWithoutLegacyTextFields(rest, "dispatch")
	return RuntimeAction{
		Kind:          "worker-lease-reclaim-execution-artifact",
		State:         runtimeRowField(machineRest, "status"),
		ExecutionMode: runtimeRowField(machineRest, "mode"),
		ArtifactPath:  path,
		DispatchPath:  runtimeDecodedRowField(rest, fields, "dispatch"),
		Results:       runtimeRowField(machineRest, "results"),
		Skipped:       runtimeRowField(machineRest, "skipped"),
		Row:           row,
	}
}

func runtimeWorkerLeaseReclaimExecutionResultActionForRow(row string) RuntimeAction {
	details := strings.TrimSpace(strings.TrimPrefix(row, "worker lease reclaim execution result "))
	fields := runtimeSimpleKeyValueFields(details)
	machineDetails := runtimeRowWithoutLegacyTextFields(details, "run", "execution", "worker", "lease_path")
	runID := runtimeDecodedRowField(details, fields, "run")
	executionID := runtimeDecodedRowField(details, fields, "execution")
	name := strings.TrimSpace(runID)
	if name != "" && executionID != "" {
		name += "/" + executionID
	}
	if name == "" {
		name = details
	}
	workerID := runtimeDecodedRowField(details, fields, "worker")
	return RuntimeAction{
		Kind:        "worker-lease-reclaim-execution-result",
		Name:        name,
		State:       runtimeRowField(machineDetails, "status"),
		Source:      workerID,
		WorkerID:    workerID,
		RunID:       runID,
		ExecutionID: executionID,
		LeasePath:   runtimeDecodedRowField(details, fields, "lease_path"),
		Row:         row,
	}
}

func runtimeWorkerQueueClaimArtifactActionForRow(row string) RuntimeAction {
	details := strings.TrimSpace(strings.TrimPrefix(row, "worker queue claim artifact:"))
	path := details
	rest := ""
	if decoded, after, ok := runtimeLeadingDecodedTextField(details, "artifact"); ok {
		path = decoded
		rest = after
	} else if before, after, ok := strings.Cut(details, " "); ok {
		path = strings.TrimSpace(before)
		rest = strings.TrimSpace(after)
	}
	fields := runtimeSimpleKeyValueFields(rest)
	machineRest := runtimeRowWithoutLegacyTextFields(rest, "worker", "run", "execution")
	executionID := runtimeDecodedRowField(rest, fields, "execution")
	return RuntimeAction{
		Kind:          "worker-queue-claim-artifact",
		State:         runtimeRowField(machineRest, "status"),
		ExecutionMode: runtimeRowField(machineRest, "mode"),
		ArtifactPath:  path,
		Name:          executionID,
		WorkerID:      runtimeDecodedRowField(rest, fields, "worker"),
		RunID:         runtimeDecodedRowField(rest, fields, "run"),
		ExecutionID:   executionID,
		Row:           row,
	}
}

func runtimeWorkerPlacementArtifactActionForRow(row string) RuntimeAction {
	details := strings.TrimSpace(strings.TrimPrefix(row, "worker placement artifact:"))
	path := details
	rest := ""
	if decoded, after, ok := runtimeLeadingDecodedTextField(details, "artifact"); ok {
		path = decoded
		rest = after
	} else if before, after, ok := strings.Cut(details, " "); ok {
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
	if decoded, after, ok := runtimeLeadingDecodedTextField(details, "artifact"); ok {
		path = decoded
		rest = after
	} else if before, after, ok := strings.Cut(details, " "); ok {
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
	if decoded, after, ok := runtimeLeadingDecodedTextField(details, "artifact"); ok {
		path = decoded
		rest = after
	} else if before, after, ok := strings.Cut(details, " "); ok {
		path = strings.TrimSpace(before)
		rest = strings.TrimSpace(after)
	}
	fields := runtimeSimpleKeyValueFields(rest)
	machineRest := runtimeRowWithoutLegacyTextFields(rest, "claim", "worker", "execution")
	workerID := runtimeDecodedRowField(rest, fields, "worker")
	executionID := runtimeDecodedRowField(rest, fields, "execution")
	return RuntimeAction{
		Kind:          "worker-queue-dispatch-execution-artifact",
		State:         runtimeRowField(machineRest, "status"),
		ExecutionMode: runtimeRowField(machineRest, "mode"),
		ArtifactPath:  path,
		DispatchPath:  runtimeDecodedRowField(rest, fields, "claim"),
		Source:        workerID,
		Name:          executionID,
		WorkerID:      workerID,
		ExecutionID:   executionID,
		Row:           row,
	}
}

func runtimeWorkerQueueDispatchResultArtifactActionForRow(row string) RuntimeAction {
	details := strings.TrimSpace(strings.TrimPrefix(row, "worker queue dispatch result artifact:"))
	path := details
	rest := ""
	if decoded, after, ok := runtimeLeadingDecodedTextField(details, "artifact"); ok {
		path = decoded
		rest = after
	} else if before, after, ok := strings.Cut(details, " "); ok {
		path = strings.TrimSpace(before)
		rest = strings.TrimSpace(after)
	}
	fields := runtimeSimpleKeyValueFields(rest)
	machineRest := runtimeRowWithoutLegacyTextFields(rest, "dispatch", "worker", "execution")
	workerID := runtimeDecodedRowField(rest, fields, "worker")
	executionID := runtimeDecodedRowField(rest, fields, "execution")
	return RuntimeAction{
		Kind:          "worker-queue-dispatch-result-artifact",
		State:         runtimeRowField(machineRest, "status"),
		ExecutionMode: runtimeRowField(machineRest, "mode"),
		ArtifactPath:  path,
		DispatchPath:  runtimeDecodedRowField(rest, fields, "dispatch"),
		Source:        workerID,
		Name:          executionID,
		WorkerID:      workerID,
		ExecutionID:   executionID,
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
	if decoded, after, ok := runtimeLeadingDecodedTextField(details, "artifact"); ok {
		path = decoded
		rest = after
	} else if before, after, ok := strings.Cut(details, " "); ok {
		path = strings.TrimSpace(before)
		rest = strings.TrimSpace(after)
	}
	fields := runtimeSimpleKeyValueFields(rest)
	machineRest := runtimeRowWithoutLegacyTextFields(rest, "retry_dispatch")
	return RuntimeAction{
		Kind:              "worker-dispatch-retry-dispatch-execution-artifact",
		State:             runtimeRowField(machineRest, "status"),
		ExecutionMode:     runtimeRowField(machineRest, "mode"),
		ArtifactPath:      path,
		RetryDispatchPath: runtimeDecodedRowField(rest, fields, "retry_dispatch"),
		Claimed:           runtimeRowField(machineRest, "claimed"),
		Skipped:           runtimeRowField(machineRest, "skipped"),
		Failed:            runtimeRowField(machineRest, "failed"),
		Row:               row,
	}
}

func runtimeWorkerDispatchRetryDispatchExecutionResultActionForRow(row string) RuntimeAction {
	rest := strings.TrimSpace(strings.TrimPrefix(row, "worker dispatch retry execution result:"))
	fields := runtimeSimpleKeyValueFields(rest)
	machineRest := runtimeRowWithoutLegacyTextFields(rest, "source_result", "source_dispatch", "source_claim", "retry_claim", "queue_dispatch", "request_path", "worker", "run", "execution", "reason")
	return RuntimeAction{
		Kind:             "worker-dispatch-retry-dispatch-execution-result",
		State:            runtimeRowField(machineRest, "status"),
		ArtifactPath:     runtimeDecodedRowField(rest, fields, "retry_claim"),
		SourceResultPath: runtimeDecodedRowField(rest, fields, "source_result"),
		DispatchPath:     runtimeDecodedRowField(rest, fields, "source_dispatch"),
		SourceClaimPath:  runtimeDecodedRowField(rest, fields, "source_claim"),
		ExecutionPath:    runtimeDecodedRowField(rest, fields, "queue_dispatch"),
		RequestPath:      runtimeDecodedRowField(rest, fields, "request_path"),
		TransportMode:    runtimeRowField(machineRest, "transport"),
		WorkerID:         runtimeDecodedRowField(rest, fields, "worker"),
		RunID:            runtimeDecodedRowField(rest, fields, "run"),
		ExecutionID:      runtimeDecodedRowField(rest, fields, "execution"),
		Reason:           runtimeDecodedRowField(rest, fields, "reason"),
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
	fields := runtimeSimpleKeyValueFields(rest)
	machineRest := runtimeRowWithoutLegacyTextFields(rest, "skip", "error")
	results := strings.Join(nonEmptyRuntimeParts([]string{
		"max_runs=" + runtimeRowField(machineRest, "max_runs"),
		"retry_plans_created=" + runtimeRowField(machineRest, "retry_plans_created"),
		"retry_dispatches_created=" + runtimeRowField(machineRest, "retry_dispatches_created"),
		"retry_dispatch_executions_created=" + runtimeRowField(machineRest, "retry_dispatch_executions_created"),
		"dispatch_retry_claims=" + runtimeRowField(machineRest, "dispatch_retry_claims"),
		"retry_transport=" + runtimeRowField(machineRest, "retry_transport"),
		"retry_dispatch_claims_created=" + runtimeRowField(machineRest, "retry_dispatch_claims_created"),
		"retry_dispatch_claims_dispatched=" + runtimeRowField(machineRest, "retry_dispatch_claims_dispatched"),
		"retry_dispatch_transport_requests_created=" + runtimeRowField(machineRest, "retry_dispatch_transport_requests_created"),
	}), " ")
	skipped := runtimeOptionalDecodedRowField(rest, fields, "skip")
	failed := runtimeOptionalDecodedRowField(rest, fields, "error")
	return RuntimeAction{
		Kind:          "worker-dispatch-retry-scheduler-run-artifact",
		State:         runtimeRowField(machineRest, "status"),
		ExecutionMode: runtimeRowField(machineRest, "mode"),
		ArtifactPath:  path,
		TransportMode: runtimeRowField(machineRest, "retry_transport"),
		Results:       results,
		Skipped:       skipped,
		Failed:        failed,
		Row:           row,
	}
}

func runtimeReportProjectionDispatchArtifactActionForRow(row string) RuntimeAction {
	details := strings.TrimSpace(strings.TrimPrefix(row, "report projection dispatch artifact:"))
	path := details
	rest := ""
	if decoded, after, ok := runtimeLeadingDecodedTextField(details, "artifact"); ok {
		path = decoded
		rest = after
	} else if before, after, ok := strings.Cut(details, " "); ok {
		path = strings.TrimSpace(before)
		rest = strings.TrimSpace(after)
	}
	fields := runtimeSimpleKeyValueFields(rest)
	machineRest := runtimeRowWithoutLegacyTextFields(rest, "reason", "planned_report", "actor", "created_at")
	plannedReport := runtimeRowFieldWithFallback(machineRest, "planned_report", "report")
	if decoded, ok := runtimeDecodedBase64RowField(rest, "planned_report"); ok {
		plannedReport = decoded
	}
	actor := runtimeRowField(machineRest, "actor")
	if decoded, ok := runtimeDecodedBase64RowField(rest, "actor"); ok {
		actor = decoded
	}
	createdAt := runtimeRowField(machineRest, "created_at")
	if decoded, ok := runtimeDecodedBase64RowField(rest, "created_at"); ok {
		createdAt = decoded
	}
	return RuntimeAction{
		Kind:              "report-projection-dispatch-artifact",
		State:             runtimeRowField(machineRest, "status"),
		ExecutionMode:     runtimeRowField(machineRest, "mode"),
		ArtifactPath:      path,
		RunID:             runtimeRowField(machineRest, "run"),
		PlannedReportPath: plannedReport,
		Results:           "events=" + runtimeRowField(machineRest, "events"),
		CreatedBy:         actor,
		CreatedAt:         createdAt,
		Reason:            runtimeDecodedRowField(rest, fields, "reason"),
		Row:               row,
	}
}

func runtimeReportProjectionDispatchExecutionArtifactActionForRow(row string) RuntimeAction {
	details := strings.TrimSpace(strings.TrimPrefix(row, "report projection dispatch execution artifact:"))
	path := details
	rest := ""
	if decoded, after, ok := runtimeLeadingDecodedTextField(details, "artifact"); ok {
		path = decoded
		rest = after
	} else if before, after, ok := strings.Cut(details, " "); ok {
		path = strings.TrimSpace(before)
		rest = strings.TrimSpace(after)
	}
	fields := runtimeSimpleKeyValueFields(rest)
	machineRest := runtimeRowWithoutLegacyTextFields(rest, "reason", "review_reason", "dispatch", "report", "actor", "executed_at", "reviewer", "reviewed_at")
	dispatchPath := runtimeRowField(machineRest, "dispatch")
	if decoded, ok := runtimeDecodedBase64RowField(rest, "dispatch"); ok {
		dispatchPath = decoded
	}
	resultPath := runtimeRowField(machineRest, "report")
	if decoded, ok := runtimeDecodedBase64RowField(rest, "report"); ok {
		resultPath = decoded
	}
	actor := runtimeRowField(machineRest, "actor")
	if decoded, ok := runtimeDecodedBase64RowField(rest, "actor"); ok {
		actor = decoded
	}
	executedAt := runtimeRowField(machineRest, "executed_at")
	if decoded, ok := runtimeDecodedBase64RowField(rest, "executed_at"); ok {
		executedAt = decoded
	}
	reviewer := runtimeRowField(machineRest, "reviewer")
	if decoded, ok := runtimeDecodedBase64RowField(rest, "reviewer"); ok {
		reviewer = decoded
	}
	reviewedAt := runtimeRowField(machineRest, "reviewed_at")
	if decoded, ok := runtimeDecodedBase64RowField(rest, "reviewed_at"); ok {
		reviewedAt = decoded
	}
	return RuntimeAction{
		Kind:          "report-projection-dispatch-execution-artifact",
		State:         runtimeRowField(machineRest, "status"),
		ExecutionMode: runtimeRowField(machineRest, "mode"),
		ArtifactPath:  path,
		DispatchPath:  dispatchPath,
		RunID:         runtimeRowField(machineRest, "run"),
		ResultPath:    resultPath,
		Results: strings.Join([]string{
			"events=" + runtimeRowField(machineRest, "events"),
			"written=" + runtimeRowField(machineRest, "written"),
			"executed_at=" + executedAt,
		}, " "),
		CreatedBy:    actor,
		CreatedAt:    executedAt,
		Reason:       runtimeDecodedRowField(rest, fields, "reason"),
		Decision:     runtimeRowField(machineRest, "decision"),
		Reviewer:     reviewer,
		ReviewedAt:   reviewedAt,
		ReviewReason: runtimeDecodedRowField(rest, fields, "review_reason"),
		Current:      runtimeRowField(machineRest, "current"),
		Row:          row,
	}
}

func runtimeReportProjectionExecutionReviewActionForRow(row string) RuntimeAction {
	details := strings.TrimSpace(strings.TrimPrefix(row, "report projection execution review:"))
	fields := runtimeSimpleKeyValueFields(details)
	machineDetails := runtimeRowWithoutLegacyTextFields(details, "review_reason", "reviewer", "reviewed_at", "execution", "dispatch", "report", "review")
	reviewer := runtimeRowField(machineDetails, "reviewer")
	if decoded, ok := runtimeDecodedBase64RowField(details, "reviewer"); ok {
		reviewer = decoded
	}
	reviewedAt := runtimeRowField(machineDetails, "reviewed_at")
	if decoded, ok := runtimeDecodedBase64RowField(details, "reviewed_at"); ok {
		reviewedAt = decoded
	}
	executionPath := runtimeRowField(machineDetails, "execution")
	if decoded, ok := runtimeDecodedBase64RowField(details, "execution"); ok {
		executionPath = decoded
	}
	dispatchPath := runtimeRowField(machineDetails, "dispatch")
	if decoded, ok := runtimeDecodedBase64RowField(details, "dispatch"); ok {
		dispatchPath = decoded
	}
	resultPath := runtimeRowField(machineDetails, "report")
	if decoded, ok := runtimeDecodedBase64RowField(details, "report"); ok {
		resultPath = decoded
	}
	reviewPath := runtimeRowField(machineDetails, "review")
	if decoded, ok := runtimeDecodedBase64RowField(details, "review"); ok {
		reviewPath = decoded
	}
	return RuntimeAction{
		Kind:          "report-projection-execution-review",
		State:         "reviewed",
		ExecutionMode: runtimeRowField(machineDetails, "mode"),
		ReviewPath:    reviewPath,
		ExecutionPath: executionPath,
		DispatchPath:  dispatchPath,
		RunID:         runtimeRowField(machineDetails, "run"),
		ResultPath:    resultPath,
		Decision:      runtimeRowField(machineDetails, "decision"),
		Reviewer:      reviewer,
		ReviewedAt:    reviewedAt,
		ReviewReason:  runtimeDecodedRowField(details, fields, "review_reason"),
		Row:           row,
	}
}

func runtimeReportProjectionExecutionSelectionActionForRow(row string) RuntimeAction {
	details := strings.TrimSpace(strings.TrimPrefix(row, "report projection execution selection:"))
	fields := runtimeSimpleKeyValueFields(details)
	machineDetails := runtimeRowWithoutLegacyTextFields(details, "reason", "selected_by", "selected_at", "cleared_by", "cleared_at", "execution", "selection", "dispatch", "report")
	state := runtimeRowField(machineDetails, "status")
	current := runtimeRowField(machineDetails, "current")
	if current == "" {
		current = "false"
		if state == "current" {
			current = "true"
		}
	}
	executionPath := runtimeRowField(machineDetails, "execution")
	if decoded, ok := runtimeDecodedBase64RowField(details, "execution"); ok {
		executionPath = decoded
	}
	selectionPath := runtimeRowField(machineDetails, "selection")
	if decoded, ok := runtimeDecodedBase64RowField(details, "selection"); ok {
		selectionPath = decoded
	}
	dispatchPath := runtimeRowField(machineDetails, "dispatch")
	if decoded, ok := runtimeDecodedBase64RowField(details, "dispatch"); ok {
		dispatchPath = decoded
	}
	resultPath := runtimeRowField(machineDetails, "report")
	if decoded, ok := runtimeDecodedBase64RowField(details, "report"); ok {
		resultPath = decoded
	}
	selectedBy := runtimeRowField(machineDetails, "selected_by")
	if decoded, ok := runtimeDecodedBase64RowField(details, "selected_by"); ok {
		selectedBy = decoded
	}
	selectedAt := runtimeRowField(machineDetails, "selected_at")
	if decoded, ok := runtimeDecodedBase64RowField(details, "selected_at"); ok {
		selectedAt = decoded
	}
	clearedBy := runtimeRowField(machineDetails, "cleared_by")
	if decoded, ok := runtimeDecodedBase64RowField(details, "cleared_by"); ok {
		clearedBy = decoded
	}
	clearedAt := runtimeRowField(machineDetails, "cleared_at")
	if decoded, ok := runtimeDecodedBase64RowField(details, "cleared_at"); ok {
		clearedAt = decoded
	}
	return RuntimeAction{
		Kind:          "report-projection-execution-selection",
		State:         state,
		ExecutionMode: runtimeRowField(machineDetails, "mode"),
		ArtifactPath:  executionPath,
		RequestPath:   selectionPath,
		DispatchPath:  dispatchPath,
		RunID:         runtimeRowField(machineDetails, "run"),
		ResultPath:    resultPath,
		Results: strings.Join([]string{
			"events=" + runtimeRowField(machineDetails, "events"),
			"written=" + runtimeRowField(machineDetails, "written"),
		}, " "),
		SelectedBy: selectedBy,
		SelectedAt: selectedAt,
		ClearedBy:  clearedBy,
		ClearedAt:  clearedAt,
		Superseded: runtimeRowField(machineDetails, "superseded"),
		Reason:     runtimeDecodedRowField(details, fields, "reason"),
		Current:    current,
		Row:        row,
	}
}

func runtimeWorkerTransportRequestArtifactActionForRow(row string) RuntimeAction {
	details := strings.TrimSpace(strings.TrimPrefix(row, "worker transport request:"))
	path := details
	rest := ""
	if decoded, after, ok := runtimeLeadingDecodedTextField(details, "request"); ok {
		path = decoded
		rest = after
	} else if before, after, ok := strings.Cut(details, " "); ok {
		path = strings.TrimSpace(before)
		rest = strings.TrimSpace(after)
	}
	fields := runtimeSimpleKeyValueFields(rest)
	machineRest := runtimeRowWithoutLegacyTextFields(rest, "dispatch", "claim", "worker", "execution")
	workerID := runtimeDecodedRowField(rest, fields, "worker")
	return RuntimeAction{
		Kind:          "worker-transport-request-artifact",
		State:         runtimeRowField(machineRest, "status"),
		ExecutionMode: runtimeRowField(machineRest, "transport"),
		ArtifactPath:  path,
		DispatchPath:  runtimeDecodedRowField(rest, fields, "dispatch"),
		Source:        runtimeDecodedRowField(rest, fields, "claim"),
		Name:          workerID,
		WorkerID:      workerID,
		ExecutionID:   runtimeDecodedRowField(rest, fields, "execution"),
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
	if decoded, after, ok := runtimeLeadingDecodedTextField(details, "artifact"); ok {
		path = decoded
		rest = after
	} else if before, after, ok := strings.Cut(details, " "); ok {
		path = strings.TrimSpace(before)
		rest = strings.TrimSpace(after)
	}
	fields := runtimeSimpleKeyValueFields(rest)
	machineRest := runtimeRowWithoutLegacyTextFields(rest, "retry_dispatch")
	return RuntimeAction{
		Kind:              "install-retry-execution-artifact",
		State:             runtimeRowField(machineRest, "status"),
		ExecutionMode:     runtimeRowField(machineRest, "mode"),
		ArtifactPath:      path,
		RetryDispatchPath: runtimeDecodedRowFieldWithFallbackRow(rest, fields, "retry_dispatch", machineRest),
		Results:           runtimeRowField(machineRest, "results"),
		Skipped:           runtimeRowField(machineRest, "skipped"),
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
	fields := runtimeSimpleKeyValueFields(rest)
	machineRest := runtimeRowWithoutLegacyTextFields(rest, "output", "error", "command", "artifact", "retry_dispatch", "source_execution", "dispatch")
	return RuntimeAction{
		Kind:                "install-retry-execution-result",
		Pack:                pack,
		Name:                dependency,
		State:               firstRuntimeToken(machineRest),
		Command:             runtimeDecodedRowFieldWithFallbackRow(rest, fields, "command", machineRest),
		Output:              runtimeDecodedRowField(rest, fields, "output"),
		ExecutionError:      runtimeDecodedRowField(rest, fields, "error"),
		ArtifactPath:        runtimeDecodedRowFieldWithFallbackRow(rest, fields, "artifact", machineRest),
		DispatchPath:        runtimeDecodedRowFieldWithFallbackRow(rest, fields, "dispatch", machineRest),
		RetryDispatchPath:   runtimeDecodedRowFieldWithFallbackRow(rest, fields, "retry_dispatch", machineRest),
		SourceExecutionPath: runtimeDecodedRowFieldWithFallbackRow(rest, fields, "source_execution", machineRest),
		Row:                 row,
	}
}

func runtimeRetryExecutionResultRefsSummaryActionForRow(row string) RuntimeAction {
	details := strings.TrimSpace(strings.TrimPrefix(row, "retry execution result refs summary "))
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
	fields := runtimeSimpleKeyValueFields(rest)
	return RuntimeAction{
		Kind:                "install-retry-execution-result-refs-summary",
		Pack:                pack,
		Name:                dependency,
		State:               "summary",
		ArtifactPath:        runtimeDecodedRowField(rest, fields, "artifact"),
		DispatchPath:        runtimeDecodedRowField(rest, fields, "dispatch"),
		RetryDispatchPath:   runtimeDecodedRowField(rest, fields, "retry_dispatch"),
		SourceExecutionPath: runtimeDecodedRowField(rest, fields, "source_execution"),
		Row:                 row,
	}
}

func runtimeMissingActionPath(value string) bool {
	value = strings.TrimSpace(value)
	return value == "" || value == "-" || value == "none"
}

func runtimeActionIsReadOnly(action RuntimeAction) bool {
	kind := strings.TrimSpace(action.Kind)
	if kind == "workflow-status" {
		if strings.TrimSpace(action.State) != "ready" {
			return true
		}
		switch strings.TrimSpace(action.RequestPath) {
		case "create-compile-preview-review-execution-dispatch":
			return strings.TrimSpace(action.Source) != "model-tool-plan" || runtimeMissingActionPath(action.ReviewPath)
		case "execute-compile-preview-review-execution-dispatch":
			return strings.TrimSpace(action.Source) != "model-tool-plan" || runtimeMissingActionPath(action.DispatchPath)
		case "create-report-projection-dispatch":
			return strings.TrimSpace(action.Source) != "report-projection" || runtimeMissingActionPath(action.RunID)
		case "execute-report-projection-dispatch":
			return strings.TrimSpace(action.Source) != "report-projection" || runtimeMissingActionPath(valueOrDefault(action.ReportDispatchPath, action.DispatchPath))
		case "review-report-projection-execution":
			return strings.TrimSpace(action.Source) != "report-projection" || runtimeMissingActionPath(action.ReportExecutionPath)
		case "select-report-projection-execution":
			return strings.TrimSpace(action.Source) != "report-projection" || runtimeMissingActionPath(action.ReportExecutionPath)
		default:
			return true
		}
	}
	if kind == "compile-preview-review-selection" {
		return strings.TrimSpace(action.State) != "current"
	}
	if kind == "compile-preview-review-artifact" {
		return false
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
	if kind == "compile-preview-review-execution-dispatch-artifact" {
		return strings.TrimSpace(action.State) != "dry-run" || strings.TrimSpace(action.ExecutionMode) != "metadata-only"
	}
	if kind == "report-projection-dispatch-execution-artifact" {
		return strings.TrimSpace(action.Current) != "false" || strings.TrimSpace(action.State) != "completed" || !strings.Contains(action.Results, "written=true")
	}
	if kind == "run-model-generation" {
		return true
	}
	if kind == "model-generation-artifact" {
		return true
	}
	if kind == "model-tool-plan-preview" {
		return strings.TrimSpace(action.State) != "valid" || strings.TrimSpace(action.ExecutionMode) != "metadata-only" || strings.TrimSpace(action.ArtifactPath) == ""
	}
	return kind == "provider-usage-summary" || kind == "managed-bundle" || kind == "managed-update-request" || kind == "managed-update-download-plan" || kind == "managed-update-download-execution" || kind == "managed-update-apply-execution" || strings.HasPrefix(kind, "release-managed-status") || strings.HasPrefix(kind, "toolchain-contract") || strings.HasPrefix(kind, "compile-preview-signal") || kind == "compile-preview-review-comparison" || kind == "compile-preview-review-execution-artifact" || kind == "model-tool-plan-preview-acceptance" || kind == "replay-workbench-collaboration-presence" || strings.HasPrefix(kind, "workspace-safety") || strings.HasPrefix(kind, "install-execution") || strings.HasPrefix(kind, "install-retry-execution") || kind == "install-orchestration-artifact" || kind == "report-projection-execution-review" || kind == "worker-orchestration-artifact" || kind == "worker-orchestration-supervisor-artifact" || kind == "worker-lifecycle-audit-artifact" || kind == "worker-lifecycle-audit-item" || kind == "worker-lifecycle-review-artifact" || kind == "worker-lifecycle-action-plan-artifact" || kind == "worker-lifecycle-maintenance-artifact" || kind == "worker-lifecycle-maintenance-supervisor-artifact" || kind == "worker-lifecycle-policy-run-artifact" || kind == "worker-lifecycle-policy-scheduler-run-artifact" || kind == "worker-lifecycle-action" || strings.HasPrefix(kind, "worker-lease-result-reconciliation-execution") || strings.HasPrefix(kind, "worker-lease-reclaim-execution") || strings.HasPrefix(kind, "worker-queue-dispatch-execution") || strings.HasPrefix(kind, "worker-queue-dispatch-result") || kind == "worker-dispatch-retry-plan-artifact" || kind == "worker-dispatch-retry-dispatch-artifact" || strings.HasPrefix(kind, "worker-dispatch-retry-dispatch-execution") || kind == "worker-dispatch-retry-scheduler-run-artifact" || kind == "worker-transport-daemon-supervisor-artifact" || kind == "worker-transport-daemon-control-artifact"
}

func runtimeWorkflowStatusActionForRow(row string) RuntimeAction {
	details := strings.TrimSpace(strings.TrimPrefix(row, "workflow status:"))
	fields := runtimeSimpleKeyValueFields(details)
	command := runtimeRowField(details, "command")
	if command == "" {
		command = fields["command"]
	}
	nextGate := fields["next_gate"]
	previewPath := runtimeWorkflowStatusDecodedField(details, fields, "preview")
	acceptancePath := runtimeWorkflowStatusDecodedField(details, fields, "acceptance")
	reviewPath := runtimeWorkflowStatusDecodedField(details, fields, "review")
	selectionPath := runtimeWorkflowStatusDecodedField(details, fields, "selection")
	dispatchPath := valueOrDefault(runtimeWorkflowStatusDecodedField(details, fields, "dispatch"), runtimeWorkflowStatusDecodedField(details, fields, "report_dispatch"))
	executionPath := valueOrDefault(runtimeWorkflowStatusDecodedField(details, fields, "execution"), runtimeWorkflowStatusDecodedField(details, fields, "report_execution"))
	reportDispatchPath := runtimeWorkflowStatusDecodedField(details, fields, "report_dispatch")
	reportExecutionPath := runtimeWorkflowStatusDecodedField(details, fields, "report_execution")
	reportSelectionPath := runtimeWorkflowStatusDecodedField(details, fields, "report_selection")
	reportPath := runtimeWorkflowStatusDecodedField(details, fields, "report")
	artifactPath := valueOrDefault(previewPath, reportExecutionPath)
	if nextGate == "execute-compile-preview-review-execution-dispatch" {
		artifactPath = dispatchPath
	}
	if nextGate == "execute-report-projection-dispatch" || nextGate == "review-report-projection-execution" || nextGate == "select-report-projection-execution" {
		artifactPath = reportDispatchPath
		dispatchPath = reportDispatchPath
		executionPath = reportExecutionPath
	}
	if nextGate == "review-report-projection-execution" || nextGate == "select-report-projection-execution" {
		artifactPath = reportExecutionPath
	}
	return RuntimeAction{
		Kind:                "workflow-status",
		State:               valueOrDefault(fields["status"], "unknown"),
		ExecutionMode:       valueOrDefault(fields["mode"], "read-only"),
		Source:              fields["stage"],
		ActionType:          fields["action"],
		ArtifactPath:        artifactPath,
		ReviewPath:          valueOrDefault(reviewPath, reportDispatchPath),
		DispatchPath:        dispatchPath,
		ReportDispatchPath:  reportDispatchPath,
		ReportExecutionPath: reportExecutionPath,
		ExecutionPath:       executionPath,
		RequestPath:         nextGate,
		RunID:               fields["report_run"],
		PlannedReportPath:   reportPath,
		Reason:              fields["reason"],
		Output: strings.Join(nonEmptyRuntimeParts([]string{
			"command=" + command,
			"acceptance=" + acceptancePath,
			"selection=" + selectionPath,
			"report_selection=" + reportSelectionPath,
		}), " "),
		Row: row,
	}
}

func runtimeWorkflowStatusDecodedField(row string, fields map[string]string, key string) string {
	if decoded, ok := runtimeDecodedBase64RowField(row, key); ok {
		return decoded
	}
	if value := strings.TrimSpace(fields[key]); value != "" {
		return value
	}
	if decoded, ok := runtimeDecodedBase64Field(fields, key); ok {
		return decoded
	}
	return ""
}

func runtimeRunModelGenerationActionForRow(row string) RuntimeAction {
	details := strings.TrimSpace(strings.TrimPrefix(row, "run model generation:"))
	fields := runtimeSimpleKeyValueFields(details)
	provider := fields["provider"]
	model := fields["model"]
	tokens := valueOrDefault(fields["tokens"], strings.Join(nonEmptyRuntimeParts([]string{
		"input_tokens=" + fields["input_tokens"],
		"output_tokens=" + fields["output_tokens"],
		"total_tokens=" + fields["total_tokens"],
	}), " "))
	cost := valueOrDefault(fields["cost"], fields["estimated_cost_usd"])
	budget := valueOrDefault(fields["budget"], fields["budget_status"])
	if budget == "" {
		budget = strings.Join(nonEmptyRuntimeParts([]string{
			"budget_enforced=" + fields["budget_enforced"],
			"budget_within_limit=" + fields["budget_within_limit"],
		}), " ")
	}
	mode := valueOrDefault(fields["mode"], fields["execution_mode"])
	preview := runtimeDecodedRowField(details, fields, "preview")
	streamPreview := runtimeDecodedRowField(details, fields, "stream_preview")
	return RuntimeAction{
		Kind:          "run-model-generation",
		Pack:          provider,
		Name:          model,
		State:         valueOrDefault(fields["status"], "unknown"),
		ExecutionMode: valueOrDefault(mode, "provider-generation"),
		Source:        valueOrDefault(fields["source"], "model-generation"),
		Digest:        fields["digest"],
		Ready: strings.Join(nonEmptyRuntimeParts([]string{
			"provider=" + provider,
			"model=" + model,
		}), " "),
		Results: strings.Join(nonEmptyRuntimeParts([]string{
			"tokens=" + tokens,
			"cost=" + cost,
		}), " "),
		Skipped: strings.Join(nonEmptyRuntimeParts([]string{
			"budget=" + budget,
			"ledger_write=" + fields["ledger_write"],
		}), " "),
		Output: strings.Join(nonEmptyRuntimeParts([]string{
			"assistant_text_persisted=" + fields["assistant_text_persisted"],
			runtimeStreamingOutputPart(fields, streamPreview),
			"preview=" + preview,
		}), " "),
		Row: row,
	}
}

func runtimeModelGenerationArtifactActionForRow(row string) RuntimeAction {
	details := strings.TrimSpace(strings.TrimPrefix(row, "model generation artifact:"))
	fields := runtimeSimpleKeyValueFields(details)
	provider := fields["provider"]
	model := fields["model"]
	tokens := valueOrDefault(fields["tokens"], strings.Join(nonEmptyRuntimeParts([]string{
		"input_tokens=" + fields["input_tokens"],
		"output_tokens=" + fields["output_tokens"],
		"total_tokens=" + fields["total_tokens"],
	}), " "))
	cost := valueOrDefault(fields["cost"], fields["estimated_cost_usd"])
	budget := valueOrDefault(fields["budget"], fields["budget_status"])
	preview := runtimeDecodedRowField(details, fields, "preview")
	streamPreview := runtimeDecodedRowField(details, fields, "stream_preview")
	artifactID := valueOrDefault(fields["artifact"], fields["artifact_id"])
	return RuntimeAction{
		Kind:          "model-generation-artifact",
		Pack:          provider,
		Name:          model,
		State:         valueOrDefault(fields["status"], "unknown"),
		ExecutionMode: valueOrDefault(fields["mode"], "metadata-only"),
		Source:        valueOrDefault(fields["source"], "model-generation"),
		RunID:         fields["run"],
		Digest:        fields["digest"],
		ArtifactID:    artifactID,
		ArtifactPath:  fields["path"],
		Ready: strings.Join(nonEmptyRuntimeParts([]string{
			"provider=" + provider,
			"model=" + model,
			"artifact=" + artifactID,
		}), " "),
		Results: strings.Join(nonEmptyRuntimeParts([]string{
			"tokens=" + tokens,
			"cost=" + cost,
		}), " "),
		Skipped: strings.Join(nonEmptyRuntimeParts([]string{
			"budget=" + budget,
			"ledger_write=" + fields["ledger_write"],
		}), " "),
		Output: strings.Join(nonEmptyRuntimeParts([]string{
			"assistant_text_persisted=" + fields["assistant_text_persisted"],
			runtimeStreamingOutputPart(fields, streamPreview),
			"preview=" + preview,
		}), " "),
		Row: row,
	}
}

func runtimeModelToolPlanPreviewActionForRow(row string) RuntimeAction {
	details := strings.TrimSpace(strings.TrimPrefix(row, "model tool plan preview:"))
	machineDetails := runtimeRowWithoutLegacyTextFields(details, "preview", "path")
	if before, _, ok := strings.Cut(machineDetails, " preview="); ok {
		machineDetails = strings.TrimSpace(before)
	}
	fields := runtimeSimpleKeyValueFields(machineDetails)
	preview := runtimeDecodedRowField(details, fields, "preview")
	artifactPath := runtimeDecodedRowField(details, fields, "path")
	validity := valueOrDefault(fields["validity"], fields["status"])
	return RuntimeAction{
		Kind:          "model-tool-plan-preview",
		Pack:          fields["provider"],
		Name:          fields["model"],
		State:         valueOrDefault(validity, "unknown"),
		ExecutionMode: valueOrDefault(fields["mode"], "metadata-only"),
		Source:        valueOrDefault(fields["source"], "model-tool-plan"),
		ActionType:    fields["action"],
		ArtifactPath:  valueOrDefault(artifactPath, fields["artifact_path"]),
		Digest:        valueOrDefault(fields["artifact_digest"], fields["digest"]),
		Results: strings.Join(nonEmptyRuntimeParts([]string{
			"tokens=" + fields["tokens"],
			"cost=" + fields["cost"],
		}), " "),
		Output: strings.Join(nonEmptyRuntimeParts([]string{
			"preview=" + preview,
		}), " "),
		Row: row,
	}
}

func runtimeModelToolPlanPreviewAcceptanceActionForRow(row string) RuntimeAction {
	details := strings.TrimSpace(strings.TrimPrefix(row, "model tool plan preview acceptance artifact:"))
	machineDetails := runtimeRowWithoutLegacyTextFields(details, "reason", "accepted_by", "preview", "review", "selection", "path")
	fields := runtimeSimpleKeyValueFields(machineDetails)
	return RuntimeAction{
		Kind:          "model-tool-plan-preview-acceptance",
		State:         valueOrDefault(fields["status"], "accepted"),
		ExecutionMode: valueOrDefault(fields["mode"], "metadata-only"),
		ActionType:    fields["action"],
		ArtifactPath:  runtimeDecodedRowField(details, fields, "path"),
		ReviewPath:    runtimeDecodedRowField(details, fields, "review"),
		RequestPath:   runtimeDecodedRowField(details, fields, "selection"),
		Source:        runtimeDecodedRowField(details, fields, "preview"),
		SelectedBy:    runtimeDecodedRowField(details, fields, "accepted_by"),
		SelectedAt:    fields["accepted_at"],
		Reason:        runtimeDecodedRowField(details, fields, "reason"),
		Nodes:         fields["nodes"],
		Warnings:      fields["warnings"],
		Digest:        valueOrDefault(fields["digest"], fields["acceptance_digest"]),
		Output: strings.Join(nonEmptyRuntimeParts([]string{
			"assistant_text_persisted=" + fields["assistant_text_persisted"],
			"executed_tools=" + fields["executed_tools"],
			"worker_dispatch=" + fields["worker_dispatch"],
			"target_contact=" + fields["target_contact"],
			"execution_dispatch_created=" + fields["execution_dispatch_created"],
		}), " "),
		Row: row,
	}
}

func runtimeStreamingOutputPart(fields map[string]string, streamPreview string) string {
	if fields["streaming"] != "true" {
		return ""
	}
	return strings.Join(nonEmptyRuntimeParts([]string{
		"streaming=true",
		"stream_completed=" + fields["stream_completed"],
		"stream_chunk_count=" + fields["stream_chunk_count"],
		"stream_digest=" + fields["stream_digest"],
		"stream_preview=" + streamPreview,
	}), " ")
}

func runtimeProviderUsageSummaryActionForRow(row string) RuntimeAction {
	fields := runtimeSimpleKeyValueFields(strings.TrimSpace(strings.TrimPrefix(row, "provider usage summary:")))
	source := fields["source"]
	if decoded, ok := runtimeDecodedBase64Field(fields, "source"); ok {
		source = decoded
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

func runtimeDecodedRowField(row string, fields map[string]string, key string) string {
	if decoded, ok := runtimeDecodedBase64RowField(row, key); ok {
		return decoded
	}
	readable := strings.TrimSpace(runtimeRowField(row, key))
	allFields := fields
	if allFields == nil {
		allFields = map[string]string{}
	}
	if readable == "" {
		if decoded, ok := runtimeDecodedBase64Field(allFields, key); ok {
			return decoded
		}
		if _, ok := allFields[key+"_b64"]; !ok {
			allFields = runtimeSimpleKeyValueFields(row)
			if decoded, ok := runtimeDecodedBase64Field(allFields, key); ok {
				return decoded
			}
		}
	} else {
		return readable
	}
	return allFields[key]
}

func runtimeOptionalDecodedRowField(row string, fields map[string]string, key string) string {
	value := strings.TrimSpace(runtimeDecodedRowField(row, fields, key))
	if value == "" || value == "none" {
		return ""
	}
	return value
}

func runtimeDecodedRowFieldWithFallbackRow(row string, fields map[string]string, key string, fallbackRow string) string {
	if decoded, ok := runtimeDecodedBase64RowField(row, key); ok {
		return decoded
	}
	if value := runtimeRowField(fallbackRow, key); strings.TrimSpace(value) != "" {
		return value
	}
	if strings.TrimSpace(runtimeRowField(row, key)) == "" {
		if decoded, ok := runtimeDecodedBase64Field(fields, key); ok {
			return decoded
		}
	}
	return runtimeDecodedRowField(row, fields, key)
}

func runtimeDecodedBase64Field(fields map[string]string, key string) (string, bool) {
	if fields == nil {
		return "", false
	}
	encoded := strings.TrimSpace(fields[key+"_b64"])
	if encoded == "" {
		return "", false
	}
	decoded, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return "", false
	}
	return string(decoded), true
}

func runtimeRowWithoutLegacyTextFields(row string, keys ...string) string {
	cleaned := row
	for _, key := range keys {
		cleaned = runtimeRowWithoutLegacyTextField(cleaned, key)
	}
	return cleaned
}

func runtimeRowWithoutLegacyTextField(row string, key string) string {
	search := " " + row
	valueStart, sidecarStart, _, ok := runtimeLegacyTextSidecar(search, key)
	if !ok {
		return row
	}
	return strings.TrimSpace(search[:valueStart] + search[sidecarStart:])
}

func runtimeDecodedBase64RowField(row string, key string) (string, bool) {
	_, _, decoded, ok := runtimeLegacyTextSidecar(" "+row, key)
	return decoded, ok
}

func runtimeLeadingDecodedTextField(row string, sidecarKey string) (string, string, bool) {
	row = strings.TrimSpace(row)
	sidecarMarker := " " + sidecarKey + "_b64="
	searchStart := 0
	bestEncodedEnd := -1
	bestDecoded := ""
	for {
		sidecarOffset := strings.Index(row[searchStart:], sidecarMarker)
		if sidecarOffset < 0 {
			break
		}
		sidecarStart := searchStart + sidecarOffset
		encodedStart := sidecarStart + len(sidecarMarker)
		encodedEnd := encodedStart
		for encodedEnd < len(row) && row[encodedEnd] != ' ' {
			encodedEnd++
		}
		encoded := row[encodedStart:encodedEnd]
		if decoded, err := base64.RawURLEncoding.DecodeString(encoded); err == nil {
			decodedText := string(decoded)
			legacyText := strings.TrimSpace(row[:sidecarStart])
			if runtimeReadableTextMatchesDecodedSidecar(legacyText, decodedText) {
				bestEncodedEnd = encodedEnd
				bestDecoded = decodedText
			}
		}
		searchStart = encodedEnd
		if searchStart <= sidecarStart {
			searchStart = sidecarStart + len(sidecarMarker)
		}
	}
	if bestEncodedEnd < 0 {
		return "", "", false
	}
	return bestDecoded, strings.TrimSpace(row[bestEncodedEnd:]), true
}

func runtimeLegacyTextSidecar(search string, key string) (int, int, string, bool) {
	marker := " " + key + "="
	sidecarMarker := " " + key + "_b64="
	valueSearchStart := 0
	for {
		valueOffset := strings.Index(search[valueSearchStart:], marker)
		if valueOffset < 0 {
			return 0, 0, "", false
		}
		valueStart := valueSearchStart + valueOffset + len(marker)
		bestSidecarStart := -1
		bestDecoded := ""
		sidecarSearchStart := valueStart
		for {
			sidecarOffset := strings.Index(search[sidecarSearchStart:], sidecarMarker)
			if sidecarOffset < 0 {
				break
			}
			sidecarStart := sidecarSearchStart + sidecarOffset
			encodedStart := sidecarStart + len(sidecarMarker)
			encodedEnd := encodedStart
			for encodedEnd < len(search) && search[encodedEnd] != ' ' {
				encodedEnd++
			}
			encoded := search[encodedStart:encodedEnd]
			if decoded, err := base64.RawURLEncoding.DecodeString(encoded); err == nil {
				decodedText := string(decoded)
				legacyText := strings.TrimSpace(search[valueStart:sidecarStart])
				if runtimeReadableTextMatchesDecodedSidecar(legacyText, decodedText) {
					bestSidecarStart = sidecarStart
					bestDecoded = decodedText
				}
			}
			sidecarSearchStart = encodedEnd
			if sidecarSearchStart <= sidecarStart {
				sidecarSearchStart = sidecarStart + len(sidecarMarker)
			}
		}
		if bestSidecarStart >= 0 {
			return valueStart, bestSidecarStart, bestDecoded, true
		}
		valueSearchStart = valueStart
	}
}

func runtimeReadableTextMatchesDecodedSidecar(legacyText string, decodedText string) bool {
	legacyText = strings.TrimSpace(legacyText)
	if runtimeCompactRowTextValue(decodedText) == legacyText {
		return true
	}
	return strings.Join(strings.Fields(decodedText), "_") == legacyText
}

func runtimeCompactRowTextValue(value string) string {
	return strings.Join(strings.Fields(value), " ")
}

func runtimeLastKeyValueField(row string, key string) string {
	fields := strings.Fields(row)
	for i := len(fields) - 1; i >= 0; i-- {
		fieldKey, value, ok := strings.Cut(fields[i], "=")
		if !ok {
			continue
		}
		if strings.TrimSpace(fieldKey) == key {
			return strings.TrimSpace(value)
		}
	}
	return ""
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
	for _, candidate := range []string{"required", "pack", "command", "hint", "source", "source_url", "digest", "depends_on", "type", "kind", "gate", "status", "mode", "transport", "artifact", "archive", "path", "payload", "install", "release_root", "marker", "release_bundle", "release_payload", "release_manifest", "workspace_bundle", "workspace_payload", "workspace_manifest", "linked", "dispatch", "queue_dispatch", "retry_dispatch", "retry_plan", "source_execution", "source_result", "source_dispatch", "source_claim", "retry_claim", "claim", "request", "worker", "action", "run", "primary", "selected", "plan", "policy", "selection", "policy_basename", "selection_basename", "session_path", "session", "collaboration", "participant", "seen_by", "seen_at", "author", "actor", "planned_by", "target_kind", "target_id", "body", "execution", "manifest", "capability", "domain", "weight", "target", "score", "nodes", "warnings", "current", "base", "candidate", "node_delta", "warning_delta", "added", "removed", "changed", "reordered", "score_changed", "signal_changed", "decision", "reviewer", "reviewed_at", "review_reason", "signature_status", "signature_algorithm", "signature_key_id", "signature_signed_at", "signature_signed_by", "expected_sha256", "expected_sha256_status", "redirect_policy", "redirect_location", "redirect_location_status", "artifact_b64", "archive_b64", "payload_b64", "install_b64", "manifest_b64", "release_root_b64", "marker_b64", "release_bundle_b64", "release_payload_b64", "release_manifest_b64", "workspace_bundle_b64", "workspace_payload_b64", "workspace_manifest_b64", "request_b64", "source_url_b64", "redirect_location_b64", "planned_by_b64", "dispatch_b64", "claim_b64", "retry_dispatch_b64", "source_execution_b64", "source_result_b64", "source_dispatch_b64", "source_claim_b64", "retry_claim_b64", "queue_dispatch_b64", "request_path_b64", "lease_path_b64", "result_path_b64", "path_b64", "plan_b64", "policy_b64", "review_b64", "selection_b64", "policy_basename_b64", "selection_basename_b64", "planned_report_b64", "report_b64", "accepted_by_b64", "selected_by_b64", "selected_at_b64", "cleared_by_b64", "cleared_at_b64", "actor_b64", "created_at_b64", "executed_at_b64", "reviewer_b64", "reviewed_at_b64", "base_b64", "candidate_b64", "reason_b64", "worker_b64", "run_b64", "execution_b64", "review_reason_b64", "body_b64", "preview_b64", "stream_preview_b64", "source_b64", "output_b64", "skip_b64", "error_b64", "command_b64", "placement_plan_b64", "placement_dispatch_b64", "placement_batch_b64", "lifecycle_maintenance_b64", "lifecycle_audit_b64", "orchestration_path_b64", "selected_by", "selected_at", "accepted_by", "accepted_at", "cleared_by", "cleared_at", "superseded", "max_plan_nodes", "domain_priorities", "frontier_scores", "historical_stats", "filters", "endpoints", "participants", "created_by", "created_at", "planned_at", "executed_at", "successes", "failures", "total", "ready", "reason", "planned_report", "report", "events", "written", "request_path", "lease_path", "result_path", "review_path", "action_plan_path", "reclaim_path", "reconciliation_path", "reclaim_execution", "reconciliation_execution", "maintenance_path", "audit_path", "orchestration_path", "recommended", "result", "results", "retry_plans", "retry_dispatches", "retry_dispatch_execute", "retry_dispatch_executions", "retry_plans_created", "retry_dispatches_created", "retry_dispatch_executions_created", "dispatch_retry_claims", "retry_transport", "retry_dispatch_claims_created", "retry_dispatch_claims_dispatched", "retry_dispatch_transport_requests_created", "dispatch_lifecycle_retry_claims", "lifecycle_retry_transport", "reclaim_executions_created", "reclaim_results_created", "reconciliation_executions_created", "reconciliation_results_created", "execute_lease_recovery", "lease_recovery", "lease", "leases", "active", "stale", "stale_after", "worker_stale_after", "poll_interval_ms", "poll_interval", "stale_leases", "lifecycle_retries", "lifecycle_policies", "policy_selections", "policy_runs", "policy_scheduler_runs", "lifecycle_policy_selections", "lifecycle_policy_runs", "include_lifecycle_maintenance", "execute_lifecycle_retries", "policy_runs_created", "supervisor_created", "skip", "skipped", "output", "error", "placement_ready", "placement_skipped", "claimed", "dispatch_ready", "dispatch_skipped", "dispatched", "dispatches", "failed", "reclaim_ready", "reclaim_skipped", "reconciliation_ready", "reconciliation_skipped", "placement_plan", "placement_dispatch", "placement_batch", "lifecycle_maintenance", "lifecycle_audit", "review", "action_plan", "actions", "ack", "ack_actions", "run_actions", "daemon_control", "daemon_control_actions", "skipped_unsafe", "review_created", "action_plan_created", "reclaim_created", "reconciliation_created", "reclaim", "reclaims", "reclaim_actions", "reconcile", "reconciliation_actions", "reconciliations", "workers", "stale_workers", "placements", "claims", "requests", "daemon_runs", "supervisor_runs", "controls", "lifecycle_reviews", "action_plans", "maintenances", "maintenance_supervisors", "orchestration_supervisors", "runs", "max_runs", "max_runs_requested", "max_supervisor_runs", "orchestrations_created", "maintenance_created", "audit_created"} {
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

func (t RuntimeTabs) activeRenderedRows(maxRows int) []string {
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
	start := 0
	end := len(rows)
	if maxRows > 0 && len(rows) > maxRows {
		start = index - maxRows/2
		if start < 0 {
			start = 0
		}
		end = start + maxRows
		if end > len(rows) {
			end = len(rows)
			start = end - maxRows
		}
	}
	rendered := make([]string, 0, end-start)
	for i := start; i < end; i++ {
		row := rows[i]
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
