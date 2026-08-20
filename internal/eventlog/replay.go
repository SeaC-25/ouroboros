package eventlog

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/ouroboros/ouroboros/internal/blackboard"
	"github.com/ouroboros/ouroboros/internal/evidence"
	"github.com/ouroboros/ouroboros/internal/modelruntime"
)

type ReplayLane string

const (
	ReplayLaneMission          ReplayLane = "mission"
	ReplayLaneAgent            ReplayLane = "agent"
	ReplayLaneObserver         ReplayLane = "observer"
	ReplayLaneBlackboard       ReplayLane = "blackboard"
	ReplayLaneToolchain        ReplayLane = "toolchain"
	ReplayLaneSubmissionReport ReplayLane = "submission-report"
)

type ReplayTimelineEvent struct {
	EventID   int64
	RunID     string
	Lane      ReplayLane
	Type      EventType
	Payload   json.RawMessage
	CreatedAt time.Time
}

type ReplayAgent struct {
	ID              string
	Role            string
	State           string
	ProgressPercent int
}

type ReplayStatus struct {
	Model          string
	ContextPercent int
	AgentCount     int
	BreadthPercent int
	DepthChains    int
	ToolHealth     string
	Cost           float64
}

type ReplayModelUsagePreview struct {
	Kind                        string
	Source                      string
	AgentID                     string
	Role                        string
	Provider                    string
	Model                       string
	Status                      string
	ExecutionMode               string
	BudgetStatus                string
	LedgerWrite                 bool
	AssistantTextPersisted      bool
	TextDigest                  string
	TextPreview                 string
	Streaming                   bool
	StreamChunkCount            int
	StreamCompleted             bool
	StreamDigest                string
	StreamPreview               string
	SemanticAction              string
	Validity                    string
	MarkerPreview               string
	ModelOutputDigest           string
	ModelGenerationArtifactPath string
	CompilePreviewReviewPath    string
	CompilePreviewNodes         int
	CompilePreviewWarnings      int
	ArtifactPath                string
	ArtifactDigest              string
	ExecutedTools               int
	WorkerDispatch              bool
	TargetContact               bool
	InputTokens                 int
	OutputTokens                int
	Cost                        float64
	LatencyMS                   int64
}

type ReplayToolchainStatus struct {
	Name             string
	Health           string
	InstalledVersion string
	LatestVersion    string
	Message          string
	Dependencies     []ReplayDependencyStatus
	InstallPlan      []ReplayInstallStep
}

type ReplayInstallStep struct {
	Name        string
	Kind        string
	State       string
	Required    bool
	Command     []string
	Hint        string
	Source      string
	ImageDigest string
}

type ReplayToolExecution struct {
	ExecutionID string
	Tool        string
	Status      string
	Capability  string
	WaitReason  string
}

type ReplayToolAdmission struct {
	AdmissionID              string
	Tool                     string
	Action                   string
	Capability               string
	Decision                 string
	ExecutionPolicy          string
	PolicyReason             string
	WorkerID                 string
	WorkerPoolMaxConcurrency int
	Isolation                string
	ContainerOverrides       bool
	PackMaxConcurrency       int
	ResourceCPUUnits         int
	ResourceMemoryMB         int
	ResourceLimitCPUUnits    int
	ResourceLimitMemoryMB    int
}

type ReplayDependencyStatus struct {
	Name              string
	Kind              string
	State             string
	Required          bool
	InstalledVersion  string
	LatestVersion     string
	Source            string
	ImageDigest       string
	LatestImageDigest string
	MetadataSource    string
	Message           string
}

type ReplaySnapshot struct {
	RunID              string
	Events             []Event
	Blackboard         blackboard.ProjectState
	Evidence           evidence.Graph
	Timeline           []ReplayTimelineEvent
	ModelUsage         modelruntime.Aggregate
	ModelUsagePreviews []ReplayModelUsagePreview
	Agents             []ReplayAgent
	Status             ReplayStatus
	Toolchain          []ReplayToolchainStatus
	ToolExecutions     []ReplayToolExecution
	ToolAdmissions     []ReplayToolAdmission
}

func BuildReplaySnapshot(events []Event) (ReplaySnapshot, error) {
	if len(events) == 0 {
		return ReplaySnapshot{
			Events:   []Event{},
			Timeline: []ReplayTimelineEvent{},
			Agents:   []ReplayAgent{},
		}, nil
	}

	sortedEvents, runID, err := sortedReplayEvents(events)
	if err != nil {
		return ReplaySnapshot{}, err
	}

	state := blackboard.ProjectState{}
	ledger := modelruntime.NewLedger()
	status := ReplayStatus{Model: "not-set", ToolHealth: "unknown"}
	agentsByID := map[string]ReplayAgent{}
	toolchainByName := map[string]ReplayToolchainStatus{}
	toolExecutionsByID := map[string]ReplayToolExecution{}
	toolAdmissionsByID := map[string]ReplayToolAdmission{}
	modelUsagePreviews := []ReplayModelUsagePreview{}
	timeline := make([]ReplayTimelineEvent, 0, len(sortedEvents))

	for _, event := range sortedEvents {
		payload, err := decodeReplayPayload(event.Payload)
		if err != nil {
			return ReplaySnapshot{}, fmt.Errorf("decode event %d payload: %w", event.ID, err)
		}

		timeline = append(timeline, ReplayTimelineEvent{
			EventID:   event.ID,
			RunID:     event.RunID,
			Lane:      ReplayLaneForEventType(event.Type),
			Type:      event.Type,
			Payload:   append(json.RawMessage(nil), event.Payload...),
			CreatedAt: event.CreatedAt,
		})

		switch event.Type {
		case EventFact:
			fact, ok, err := payload.fact(event)
			if err != nil {
				return ReplaySnapshot{}, fmt.Errorf("replay fact event %d: %w", event.ID, err)
			}
			if ok {
				state.Facts = append(state.Facts, fact)
			}
		case EventIntent:
			intent, ok, err := payload.intent(event)
			if err != nil {
				return ReplaySnapshot{}, fmt.Errorf("replay intent event %d: %w", event.ID, err)
			}
			if ok {
				state.Intents = append(state.Intents, intent)
			}
		case EventHint:
			hint, ok, err := payload.hint(event)
			if err != nil {
				return ReplaySnapshot{}, fmt.Errorf("replay hint event %d: %w", event.ID, err)
			}
			if ok {
				state.Hints = append(state.Hints, hint)
			}
		case EventAgent:
			if agent, ok := payload.agent(); ok {
				agentsByID[agent.ID] = agent
			}
		case EventTool:
			if toolchainStatus, ok := payload.toolchainStatus(); ok {
				toolchainByName[toolchainStatus.Name] = toolchainStatus
			}
			if toolExecution, ok := payload.toolExecution(); ok {
				if previous, exists := toolExecutionsByID[toolExecution.ExecutionID]; exists {
					toolExecution = mergeReplayToolExecution(previous, toolExecution)
				}
				toolExecutionsByID[toolExecution.ExecutionID] = toolExecution
			}
			if admission, ok := payload.toolAdmission(); ok {
				toolAdmissionsByID[admission.AdmissionID] = admission
			}
			if toolHealth := payload.toolHealth(); toolHealth != "" {
				status.ToolHealth = aggregateToolHealth(status.ToolHealth, toolHealth)
			}
		}

		if payload.ContextPercent > 0 {
			status.ContextPercent = payload.ContextPercent
		}
		if payload.BreadthPercent > 0 {
			status.BreadthPercent = payload.BreadthPercent
		}
		if payload.DepthChains > 0 {
			status.DepthChains = payload.DepthChains
		}
		if preview, ok := payload.modelToolPlanPreview(); ok {
			modelUsagePreviews = append(modelUsagePreviews, preview)
			continue
		}
		if usage, ok := payload.usageRecord(event.RunID); ok {
			ledger.Record(usage)
			if usage.Model != "" {
				status.Model = usage.Model
			}
			if preview, ok := payload.modelUsagePreview(usage); ok {
				modelUsagePreviews = append(modelUsagePreviews, preview)
			}
		}
	}

	agents := sortedReplayAgents(agentsByID)
	toolchainStatuses := sortedReplayToolchainStatuses(toolchainByName)
	toolExecutions := sortedReplayToolExecutions(toolExecutionsByID)
	toolAdmissions := sortedReplayToolAdmissions(toolAdmissionsByID)
	status.AgentCount = len(agents)
	status.Cost = ledger.ByRun(runID).Cost

	return ReplaySnapshot{
		RunID:              runID,
		Events:             sortedEvents,
		Blackboard:         state,
		Evidence:           evidence.Project(state),
		Timeline:           timeline,
		ModelUsage:         ledger.ByRun(runID),
		ModelUsagePreviews: modelUsagePreviews,
		Agents:             agents,
		Status:             status,
		Toolchain:          toolchainStatuses,
		ToolExecutions:     toolExecutions,
		ToolAdmissions:     toolAdmissions,
	}, nil
}

func ReplayLaneForEventType(eventType EventType) ReplayLane {
	switch eventType {
	case EventGoal:
		return ReplayLaneMission
	case EventAgent, EventAttempt:
		return ReplayLaneAgent
	case EventFact, EventIntent, EventHint:
		return ReplayLaneBlackboard
	case EventTool, EventParser:
		return ReplayLaneToolchain
	case EventReplay:
		return ReplayLaneSubmissionReport
	case EventVerification, EventMutation:
		return ReplayLaneObserver
	default:
		return ReplayLaneObserver
	}
}

func sortedReplayEvents(events []Event) ([]Event, string, error) {
	sortedEvents := append([]Event(nil), events...)
	sort.SliceStable(sortedEvents, func(i int, j int) bool {
		if sortedEvents[i].CreatedAt.Equal(sortedEvents[j].CreatedAt) {
			return sortedEvents[i].ID < sortedEvents[j].ID
		}
		return sortedEvents[i].CreatedAt.Before(sortedEvents[j].CreatedAt)
	})

	runID := strings.TrimSpace(sortedEvents[0].RunID)
	if runID == "" {
		return nil, "", errors.New("replay event run id is required")
	}
	for _, event := range sortedEvents {
		if strings.TrimSpace(event.RunID) != runID {
			return nil, "", fmt.Errorf("replay events must belong to one run: %s != %s", event.RunID, runID)
		}
	}
	return sortedEvents, runID, nil
}

type replayPayload struct {
	ID                          string               `json:"id"`
	FactID                      string               `json:"fact_id"`
	IntentID                    string               `json:"intent_id"`
	HintID                      string               `json:"hint_id"`
	Kind                        string               `json:"kind"`
	Statement                   string               `json:"statement"`
	Confidence                  float64              `json:"confidence"`
	Source                      string               `json:"source"`
	Goal                        string               `json:"goal"`
	SourceFacts                 []string             `json:"source_facts"`
	State                       string               `json:"state"`
	ResultFact                  string               `json:"result_fact"`
	Message                     string               `json:"message"`
	AgentID                     string               `json:"agent_id"`
	Role                        string               `json:"role"`
	ProgressPercent             int                  `json:"progress_percent"`
	Provider                    string               `json:"provider"`
	Model                       string               `json:"model"`
	InputTokens                 int                  `json:"input_tokens"`
	OutputTokens                int                  `json:"output_tokens"`
	CacheReadTokens             int                  `json:"cache_read_tokens"`
	CacheWriteTokens            int                  `json:"cache_write_tokens"`
	Cost                        float64              `json:"cost"`
	LatencyMS                   int64                `json:"latency_ms"`
	Failed                      bool                 `json:"failed"`
	ModelUsage                  *usagePayload        `json:"model_usage"`
	ExecutionMode               string               `json:"execution_mode"`
	BudgetStatus                string               `json:"budget_status"`
	LedgerWrite                 bool                 `json:"ledger_write"`
	AssistantTextPersisted      bool                 `json:"assistant_text_persisted"`
	TextDigest                  string               `json:"text_digest"`
	TextPreview                 string               `json:"text_preview"`
	Streaming                   bool                 `json:"streaming"`
	StreamChunkCount            int                  `json:"stream_chunk_count"`
	StreamCompleted             bool                 `json:"stream_completed"`
	StreamDigest                string               `json:"stream_digest"`
	StreamPreview               string               `json:"stream_preview"`
	Action                      string               `json:"action"`
	SemanticAction              string               `json:"semantic_action"`
	Valid                       bool                 `json:"valid"`
	Validity                    string               `json:"validity"`
	MarkerPreview               string               `json:"marker_preview"`
	ModelOutputDigest           string               `json:"model_output_digest"`
	ModelGenerationArtifactPath string               `json:"model_generation_artifact_path"`
	CompilePreviewReviewPath    string               `json:"compile_preview_review_path"`
	CompilePreviewNodes         int                  `json:"compile_preview_nodes"`
	CompilePreviewWarnings      int                  `json:"compile_preview_warnings"`
	ArtifactPath                string               `json:"artifact_path"`
	ArtifactDigest              string               `json:"artifact_digest"`
	ExecutedTools               int                  `json:"executed_tools"`
	WorkerDispatch              bool                 `json:"worker_dispatch"`
	TargetContact               bool                 `json:"target_contact"`
	ContextPercent              int                  `json:"context_percent"`
	BreadthPercent              int                  `json:"breadth_percent"`
	DepthChains                 int                  `json:"depth_chains"`
	ToolHealth                  string               `json:"tool_health"`
	Tool                        string               `json:"tool"`
	Health                      string               `json:"health"`
	Status                      string               `json:"status"`
	ExecutionID                 string               `json:"execution_id"`
	ExecutionStatus             string               `json:"execution_status"`
	Capability                  string               `json:"capability"`
	WorkerStatus                string               `json:"worker_status"`
	WorkerCapability            string               `json:"worker_capability"`
	WaitReason                  string               `json:"wait_reason"`
	AdmissionID                 string               `json:"admission_id"`
	AdmissionAction             string               `json:"admission_action"`
	AdmissionCapability         string               `json:"admission_capability"`
	AdmissionDecision           string               `json:"admission_decision"`
	WorkerID                    string               `json:"worker_id"`
	ExecutionPolicy             string               `json:"execution_policy"`
	PolicyReason                string               `json:"policy_reason"`
	Isolation                   string               `json:"isolation"`
	ContainerOverrides          bool                 `json:"container_overrides"`
	WorkerPoolMaxConcurrency    int                  `json:"worker_pool_max_concurrency"`
	PackMaxConcurrency          int                  `json:"pack_max_concurrency"`
	ResourceCPUUnits            int                  `json:"resource_cpu_units"`
	ResourceMemoryMB            int                  `json:"resource_memory_mb"`
	ResourceLimitCPUUnits       int                  `json:"resource_limit_cpu_units"`
	ResourceLimitMemoryMB       int                  `json:"resource_limit_memory_mb"`
	InstalledVersion            string               `json:"installed_version"`
	LatestVersion               string               `json:"latest_version"`
	Dependencies                []dependencyPayload  `json:"dependencies"`
	InstallPlan                 []installPlanPayload `json:"install_plan"`
}

type dependencyPayload struct {
	Name              string `json:"name"`
	Kind              string `json:"kind"`
	State             string `json:"state"`
	Required          bool   `json:"required"`
	InstalledVersion  string `json:"installed_version"`
	LatestVersion     string `json:"latest_version"`
	Source            string `json:"source"`
	ImageDigest       string `json:"image_digest"`
	LatestImageDigest string `json:"latest_image_digest"`
	MetadataSource    string `json:"metadata_source"`
	Message           string `json:"message"`
}

type installPlanPayload struct {
	Name        string   `json:"name"`
	Kind        string   `json:"kind"`
	State       string   `json:"state"`
	Required    bool     `json:"required"`
	Command     []string `json:"command"`
	Hint        string   `json:"hint"`
	Source      string   `json:"source"`
	ImageDigest string   `json:"digest"`
}

type usagePayload struct {
	RunID            string  `json:"run_id"`
	AgentID          string  `json:"agent_id"`
	Role             string  `json:"role"`
	Provider         string  `json:"provider"`
	Model            string  `json:"model"`
	InputTokens      int     `json:"input_tokens"`
	OutputTokens     int     `json:"output_tokens"`
	CacheReadTokens  int     `json:"cache_read_tokens"`
	CacheWriteTokens int     `json:"cache_write_tokens"`
	Cost             float64 `json:"cost"`
	LatencyMS        int64   `json:"latency_ms"`
	Failed           bool    `json:"failed"`
}

func decodeReplayPayload(payload json.RawMessage) (replayPayload, error) {
	if len(payload) == 0 {
		payload = json.RawMessage(`{}`)
	}
	var decoded replayPayload
	if err := json.Unmarshal(payload, &decoded); err != nil {
		return replayPayload{}, err
	}
	return decoded, nil
}

func (p replayPayload) fact(event Event) (blackboard.Fact, bool, error) {
	id := firstNonEmpty(p.FactID, p.ID, fmt.Sprintf("fact-%d", event.ID))
	if strings.TrimSpace(p.Kind) == "" && strings.TrimSpace(p.Statement) == "" {
		return blackboard.Fact{}, false, nil
	}
	if strings.TrimSpace(p.Kind) == "" {
		return blackboard.Fact{}, false, errors.New("fact kind is required")
	}
	if strings.TrimSpace(p.Statement) == "" {
		return blackboard.Fact{}, false, errors.New("fact statement is required")
	}
	return blackboard.Fact{
		ID:         id,
		Kind:       strings.TrimSpace(p.Kind),
		Statement:  strings.TrimSpace(p.Statement),
		Confidence: p.Confidence,
		Source:     strings.TrimSpace(p.Source),
		CreatedAt:  event.CreatedAt,
	}, true, nil
}

func (p replayPayload) intent(event Event) (blackboard.Intent, bool, error) {
	id := firstNonEmpty(p.IntentID, p.ID, fmt.Sprintf("intent-%d", event.ID))
	if strings.TrimSpace(p.Kind) == "" && strings.TrimSpace(p.Goal) == "" {
		return blackboard.Intent{}, false, nil
	}
	if strings.TrimSpace(p.Kind) == "" {
		return blackboard.Intent{}, false, errors.New("intent kind is required")
	}
	if strings.TrimSpace(p.Goal) == "" {
		return blackboard.Intent{}, false, errors.New("intent goal is required")
	}
	state := blackboard.IntentState(strings.TrimSpace(p.State))
	if state == "" {
		state = blackboard.IntentOpen
	}
	return blackboard.Intent{
		ID:          id,
		Kind:        strings.TrimSpace(p.Kind),
		Goal:        strings.TrimSpace(p.Goal),
		SourceFacts: append([]string(nil), p.SourceFacts...),
		State:       state,
		ResultFact:  strings.TrimSpace(p.ResultFact),
		CreatedAt:   event.CreatedAt,
		UpdatedAt:   event.CreatedAt,
	}, true, nil
}

func (p replayPayload) hint(event Event) (blackboard.Hint, bool, error) {
	id := firstNonEmpty(p.HintID, p.ID, fmt.Sprintf("hint-%d", event.ID))
	if strings.TrimSpace(p.Kind) == "" && strings.TrimSpace(p.Message) == "" {
		return blackboard.Hint{}, false, nil
	}
	if strings.TrimSpace(p.Kind) == "" {
		return blackboard.Hint{}, false, errors.New("hint kind is required")
	}
	if strings.TrimSpace(p.Message) == "" {
		return blackboard.Hint{}, false, errors.New("hint message is required")
	}
	return blackboard.Hint{
		ID:        id,
		Kind:      strings.TrimSpace(p.Kind),
		Message:   strings.TrimSpace(p.Message),
		Source:    strings.TrimSpace(p.Source),
		CreatedAt: event.CreatedAt,
	}, true, nil
}

func (p replayPayload) agent() (ReplayAgent, bool) {
	id := strings.TrimSpace(p.AgentID)
	if id == "" {
		return ReplayAgent{}, false
	}
	return ReplayAgent{
		ID:              id,
		Role:            strings.TrimSpace(p.Role),
		State:           strings.TrimSpace(p.State),
		ProgressPercent: p.ProgressPercent,
	}, true
}

func (p replayPayload) toolchainStatus() (ReplayToolchainStatus, bool) {
	name := strings.TrimSpace(p.Tool)
	if name == "" {
		return ReplayToolchainStatus{}, false
	}
	health := firstNonEmpty(p.ToolHealth, p.Health, p.Status)
	if health == "" && p.InstalledVersion == "" && p.LatestVersion == "" && p.Message == "" && len(p.Dependencies) == 0 && len(p.InstallPlan) == 0 {
		return ReplayToolchainStatus{}, false
	}
	dependencies := make([]ReplayDependencyStatus, 0, len(p.Dependencies))
	for _, dependency := range p.Dependencies {
		dependencies = append(dependencies, ReplayDependencyStatus{
			Name:              strings.TrimSpace(dependency.Name),
			Kind:              strings.TrimSpace(dependency.Kind),
			State:             strings.TrimSpace(dependency.State),
			Required:          dependency.Required,
			InstalledVersion:  strings.TrimSpace(dependency.InstalledVersion),
			LatestVersion:     strings.TrimSpace(dependency.LatestVersion),
			Source:            strings.TrimSpace(dependency.Source),
			ImageDigest:       strings.TrimSpace(dependency.ImageDigest),
			LatestImageDigest: strings.TrimSpace(dependency.LatestImageDigest),
			MetadataSource:    strings.TrimSpace(dependency.MetadataSource),
			Message:           strings.TrimSpace(dependency.Message),
		})
	}
	sort.SliceStable(dependencies, func(i int, j int) bool {
		return dependencies[i].Name < dependencies[j].Name
	})
	installPlan := make([]ReplayInstallStep, 0, len(p.InstallPlan))
	for _, step := range p.InstallPlan {
		installPlan = append(installPlan, ReplayInstallStep{
			Name:        strings.TrimSpace(step.Name),
			Kind:        strings.TrimSpace(step.Kind),
			State:       strings.TrimSpace(step.State),
			Required:    step.Required,
			Command:     trimmedReplayCommand(step.Command),
			Hint:        strings.TrimSpace(step.Hint),
			Source:      strings.TrimSpace(step.Source),
			ImageDigest: strings.TrimSpace(step.ImageDigest),
		})
	}
	sort.SliceStable(installPlan, func(i int, j int) bool {
		return installPlan[i].Name < installPlan[j].Name
	})
	return ReplayToolchainStatus{
		Name:             name,
		Health:           strings.TrimSpace(health),
		InstalledVersion: strings.TrimSpace(p.InstalledVersion),
		LatestVersion:    strings.TrimSpace(p.LatestVersion),
		Message:          strings.TrimSpace(p.Message),
		Dependencies:     dependencies,
		InstallPlan:      installPlan,
	}, true
}

func (p replayPayload) toolHealth() string {
	if explicit := firstNonEmpty(p.ToolHealth, p.Health, p.Status); explicit != "" {
		return explicit
	}
	if strings.TrimSpace(p.ExecutionStatus) == "failed" {
		return "failed"
	}
	return ""
}

func (p replayPayload) toolExecution() (ReplayToolExecution, bool) {
	executionID := strings.TrimSpace(p.ExecutionID)
	if executionID == "" {
		return ReplayToolExecution{}, false
	}
	status := strings.TrimSpace(p.ExecutionStatus)
	if status == "" {
		status = firstNonEmpty(p.WorkerStatus, p.Status)
	}
	return ReplayToolExecution{
		ExecutionID: executionID,
		Tool:        strings.TrimSpace(p.Tool),
		Status:      status,
		Capability:  firstNonEmpty(p.Capability, p.WorkerCapability),
		WaitReason:  strings.TrimSpace(p.WaitReason),
	}, true
}

func (p replayPayload) toolAdmission() (ReplayToolAdmission, bool) {
	if strings.TrimSpace(p.Kind) != "scheduler_admission" {
		return ReplayToolAdmission{}, false
	}
	admissionID := strings.TrimSpace(p.AdmissionID)
	if admissionID == "" {
		return ReplayToolAdmission{}, false
	}
	return ReplayToolAdmission{
		AdmissionID:              admissionID,
		Tool:                     strings.TrimSpace(p.Tool),
		Action:                   strings.TrimSpace(p.AdmissionAction),
		Capability:               strings.TrimSpace(p.AdmissionCapability),
		Decision:                 strings.TrimSpace(p.AdmissionDecision),
		ExecutionPolicy:          strings.TrimSpace(p.ExecutionPolicy),
		PolicyReason:             strings.TrimSpace(p.PolicyReason),
		WorkerID:                 strings.TrimSpace(p.WorkerID),
		WorkerPoolMaxConcurrency: p.WorkerPoolMaxConcurrency,
		Isolation:                strings.TrimSpace(p.Isolation),
		ContainerOverrides:       p.ContainerOverrides,
		PackMaxConcurrency:       p.PackMaxConcurrency,
		ResourceCPUUnits:         p.ResourceCPUUnits,
		ResourceMemoryMB:         p.ResourceMemoryMB,
		ResourceLimitCPUUnits:    p.ResourceLimitCPUUnits,
		ResourceLimitMemoryMB:    p.ResourceLimitMemoryMB,
	}, true
}

func mergeReplayToolExecution(previous ReplayToolExecution, next ReplayToolExecution) ReplayToolExecution {
	if next.Tool == "" {
		next.Tool = previous.Tool
	}
	if next.Status == "" {
		next.Status = previous.Status
	}
	if next.Capability == "" {
		next.Capability = previous.Capability
	}
	if next.WaitReason == "" {
		next.WaitReason = previous.WaitReason
	}
	return next
}

func sortedReplayToolchainStatuses(statusesByName map[string]ReplayToolchainStatus) []ReplayToolchainStatus {
	statuses := make([]ReplayToolchainStatus, 0, len(statusesByName))
	for _, status := range statusesByName {
		statuses = append(statuses, status)
	}
	sort.SliceStable(statuses, func(i int, j int) bool {
		return statuses[i].Name < statuses[j].Name
	})
	return statuses
}

func sortedReplayToolExecutions(executionsByID map[string]ReplayToolExecution) []ReplayToolExecution {
	executions := make([]ReplayToolExecution, 0, len(executionsByID))
	for _, execution := range executionsByID {
		executions = append(executions, execution)
	}
	sort.SliceStable(executions, func(i int, j int) bool {
		return executions[i].ExecutionID < executions[j].ExecutionID
	})
	return executions
}

func sortedReplayToolAdmissions(admissionsByID map[string]ReplayToolAdmission) []ReplayToolAdmission {
	admissions := make([]ReplayToolAdmission, 0, len(admissionsByID))
	for _, admission := range admissionsByID {
		admissions = append(admissions, admission)
	}
	sort.SliceStable(admissions, func(i int, j int) bool {
		return admissions[i].AdmissionID < admissions[j].AdmissionID
	})
	return admissions
}

func aggregateToolHealth(current string, next string) string {
	current = strings.TrimSpace(current)
	next = strings.TrimSpace(next)
	if current == "" || current == "unknown" {
		return next
	}
	if next == "" || next == "unknown" {
		return current
	}
	if toolHealthRank(next) > toolHealthRank(current) {
		return next
	}
	return current
}

func toolHealthRank(value string) int {
	switch strings.TrimSpace(value) {
	case "failed":
		return 50
	case "missing":
		return 40
	case "update_available":
		return 30
	case "ready":
		return 20
	default:
		return 10
	}
}

func (p replayPayload) usageRecord(defaultRunID string) (modelruntime.UsageRecord, bool) {
	usage := usagePayload{
		RunID:            defaultRunID,
		AgentID:          p.AgentID,
		Role:             p.Role,
		Provider:         p.Provider,
		Model:            p.Model,
		InputTokens:      p.InputTokens,
		OutputTokens:     p.OutputTokens,
		CacheReadTokens:  p.CacheReadTokens,
		CacheWriteTokens: p.CacheWriteTokens,
		Cost:             p.Cost,
		LatencyMS:        p.LatencyMS,
		Failed:           p.Failed,
	}
	if p.ModelUsage != nil {
		usage = *p.ModelUsage
		usage.RunID = firstNonEmpty(usage.RunID, defaultRunID)
		usage.AgentID = firstNonEmpty(usage.AgentID, p.AgentID)
		usage.Role = firstNonEmpty(usage.Role, p.Role)
	}
	if strings.TrimSpace(usage.Model) == "" && usage.InputTokens == 0 && usage.OutputTokens == 0 && usage.Cost == 0 {
		return modelruntime.UsageRecord{}, false
	}
	return modelruntime.UsageRecord{
		RunID:            firstNonEmpty(usage.RunID, defaultRunID),
		AgentID:          strings.TrimSpace(usage.AgentID),
		Role:             strings.TrimSpace(usage.Role),
		Provider:         strings.TrimSpace(usage.Provider),
		Model:            strings.TrimSpace(usage.Model),
		InputTokens:      usage.InputTokens,
		OutputTokens:     usage.OutputTokens,
		CacheReadTokens:  usage.CacheReadTokens,
		CacheWriteTokens: usage.CacheWriteTokens,
		Cost:             usage.Cost,
		Latency:          time.Duration(usage.LatencyMS) * time.Millisecond,
		Failed:           usage.Failed,
	}, true
}

func (p replayPayload) modelUsagePreview(usage modelruntime.UsageRecord) (ReplayModelUsagePreview, bool) {
	kind := strings.TrimSpace(p.Kind)
	if kind != "model_usage_preview" && kind != "model_generation" {
		return ReplayModelUsagePreview{}, false
	}
	source := firstNonEmpty(p.Source, "model-usage")
	executionMode := firstNonEmpty(p.ExecutionMode, "metadata-only")
	budgetStatus := firstNonEmpty(p.BudgetStatus, "not_enforced")
	if kind == "model_generation" {
		source = firstNonEmpty(p.Source, "model-generation")
		executionMode = firstNonEmpty(p.ExecutionMode, "provider-generation")
	}
	return ReplayModelUsagePreview{
		Kind:                   kind,
		Source:                 source,
		AgentID:                usage.AgentID,
		Role:                   usage.Role,
		Provider:               usage.Provider,
		Model:                  usage.Model,
		Status:                 strings.TrimSpace(p.Status),
		ExecutionMode:          executionMode,
		BudgetStatus:           budgetStatus,
		LedgerWrite:            p.LedgerWrite,
		AssistantTextPersisted: p.AssistantTextPersisted,
		TextDigest:             strings.TrimSpace(p.TextDigest),
		TextPreview:            compactReplayText(p.TextPreview),
		Streaming:              p.Streaming,
		StreamChunkCount:       p.StreamChunkCount,
		StreamCompleted:        p.StreamCompleted,
		StreamDigest:           strings.TrimSpace(p.StreamDigest),
		StreamPreview:          compactReplayText(p.StreamPreview),
		InputTokens:            usage.InputTokens,
		OutputTokens:           usage.OutputTokens,
		Cost:                   usage.Cost,
		LatencyMS:              int64(usage.Latency / time.Millisecond),
	}, true
}

func (p replayPayload) modelToolPlanPreview() (ReplayModelUsagePreview, bool) {
	if strings.TrimSpace(p.Kind) != "model_tool_plan_preview" {
		return ReplayModelUsagePreview{}, false
	}
	action := firstNonEmpty(p.SemanticAction, p.Action)
	validity := strings.TrimSpace(p.Validity)
	if validity == "" {
		if p.Valid {
			validity = "valid"
		} else {
			validity = "invalid"
		}
	}
	return ReplayModelUsagePreview{
		Kind:                        "model_tool_plan_preview",
		Source:                      firstNonEmpty(p.Source, "model-tool-plan-preview"),
		AgentID:                     strings.TrimSpace(p.AgentID),
		Role:                        strings.TrimSpace(p.Role),
		Provider:                    strings.TrimSpace(p.Provider),
		Model:                       strings.TrimSpace(p.Model),
		Status:                      strings.TrimSpace(p.Status),
		ExecutionMode:               firstNonEmpty(p.ExecutionMode, "metadata-only"),
		AssistantTextPersisted:      p.AssistantTextPersisted,
		TextDigest:                  strings.TrimSpace(p.ModelOutputDigest),
		TextPreview:                 compactReplayText(p.MarkerPreview),
		SemanticAction:              strings.TrimSpace(action),
		Validity:                    validity,
		MarkerPreview:               compactReplayText(p.MarkerPreview),
		ModelOutputDigest:           strings.TrimSpace(p.ModelOutputDigest),
		ModelGenerationArtifactPath: strings.TrimSpace(p.ModelGenerationArtifactPath),
		CompilePreviewReviewPath:    strings.TrimSpace(p.CompilePreviewReviewPath),
		CompilePreviewNodes:         p.CompilePreviewNodes,
		CompilePreviewWarnings:      p.CompilePreviewWarnings,
		ArtifactPath:                strings.TrimSpace(p.ArtifactPath),
		ArtifactDigest:              strings.TrimSpace(p.ArtifactDigest),
		ExecutedTools:               p.ExecutedTools,
		WorkerDispatch:              p.WorkerDispatch,
		TargetContact:               p.TargetContact,
	}, true
}

func sortedReplayAgents(agentsByID map[string]ReplayAgent) []ReplayAgent {
	agents := make([]ReplayAgent, 0, len(agentsByID))
	for _, agent := range agentsByID {
		agents = append(agents, agent)
	}
	sort.SliceStable(agents, func(i int, j int) bool {
		return agents[i].ID < agents[j].ID
	})
	return agents
}

func trimmedReplayCommand(command []string) []string {
	if len(command) == 0 {
		return nil
	}
	trimmed := make([]string, 0, len(command))
	for _, part := range command {
		trimmed = append(trimmed, strings.TrimSpace(part))
	}
	return trimmed
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func compactReplayText(value string) string {
	return strings.Join(strings.Fields(value), " ")
}
