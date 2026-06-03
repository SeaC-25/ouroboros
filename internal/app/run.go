package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/ouroboros/ouroboros/internal/compiler"
	"github.com/ouroboros/ouroboros/internal/domain"
	"github.com/ouroboros/ouroboros/internal/domain/ctf"
	"github.com/ouroboros/ouroboros/internal/domain/infra"
	"github.com/ouroboros/ouroboros/internal/domain/webapi"
	"github.com/ouroboros/ouroboros/internal/eventlog"
	"github.com/ouroboros/ouroboros/internal/evidence"
	"github.com/ouroboros/ouroboros/internal/report"
	"github.com/ouroboros/ouroboros/internal/toolchain"
	"github.com/ouroboros/ouroboros/internal/tui"
	"gopkg.in/yaml.v3"
)

type RunResult struct {
	RunID      string
	EventCount int
	ReportPath string
	StatusLine string
	TUIView    string
	TUIModel   tui.Model
}

type RunOptions struct {
	OnEvent                    func(eventlog.Event) error
	OnSnapshot                 func(eventlog.ReplaySnapshot) error
	ToolRunner                 ToolExecutionRunner
	ToolConcurrency            int
	PauseController            *RunPauseController
	DeferReportProjection      bool
	RequireSafetyAuthorization bool
}

type RunPauseController struct {
	mu     sync.Mutex
	paused bool
	notify chan struct{}
}

func NewRunPauseController() *RunPauseController {
	return &RunPauseController{notify: make(chan struct{})}
}

func (c *RunPauseController) Pause() {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ensureNotifyLocked()
	c.paused = true
}

func (c *RunPauseController) Resume() {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	notify := c.ensureNotifyLocked()
	if !c.paused {
		return
	}
	c.paused = false
	close(notify)
	c.notify = make(chan struct{})
}

func (c *RunPauseController) IsPaused() bool {
	if c == nil {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.paused
}

func (c *RunPauseController) waitIfPaused(ctx context.Context, onWaiting func() error) error {
	if c == nil {
		return nil
	}
	reported := false
	for {
		c.mu.Lock()
		paused := c.paused
		notify := c.ensureNotifyLocked()
		c.mu.Unlock()
		if !paused {
			return nil
		}
		if !reported {
			if onWaiting != nil {
				if err := onWaiting(); err != nil {
					return err
				}
			}
			reported = true
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-notify:
		}
	}
}

func (c *RunPauseController) ensureNotifyLocked() chan struct{} {
	if c.notify == nil {
		c.notify = make(chan struct{})
	}
	return c.notify
}

type ToolExecutionRunner interface {
	Execute(context.Context, ToolExecutionRequest) (ToolExecutionResult, error)
}

type ToolExecutionRequest struct {
	RunID                    string
	WorkspaceRoot            string
	Node                     compiler.PlanNode
	ScopeTargets             []ScopeTarget
	ScopeExclusions          []string
	ScopeTargetPolicy        string
	IsolationMode            string
	Container                ContainerExecutionConfig
	ExecutionPolicy          string
	PolicyReason             string
	WorkerID                 string
	WorkerPoolMaxConcurrency int
	ContainerOverrides       bool
	PackMaxConcurrency       int
	ResourceCPUUnits         int
	ResourceMemoryMB         int
	ResourceLimitCPUUnits    int
	ResourceLimitMemoryMB    int
}

type ToolExecutionResult struct {
	ExecutionID string
	Status      string
	Transcript  string
	AttemptDir  string
}

type ScopeConfig struct {
	Version int `yaml:"version"`
	Scope   struct {
		Targets      []ScopeTarget `yaml:"targets"`
		Exclusions   []string      `yaml:"exclusions"`
		TargetPolicy string        `yaml:"target_policy,omitempty"`
	} `yaml:"scope"`
	Execution struct {
		Isolation       string                   `yaml:"isolation,omitempty"`
		ToolConcurrency int                      `yaml:"tool_concurrency,omitempty"`
		ResourceLimits  ExecutionResourceLimits  `yaml:"resource_limits,omitempty"`
		Container       ContainerExecutionConfig `yaml:"container,omitempty"`
		PackPolicies    []PackExecutionPolicy    `yaml:"pack_policies,omitempty"`
		WorkerPools     []WorkerPoolPolicy       `yaml:"worker_pools,omitempty"`
	} `yaml:"execution,omitempty"`
}

type ExecutionResourceLimits struct {
	CPUUnits int `yaml:"cpu_units,omitempty"`
	MemoryMB int `yaml:"memory_mb,omitempty"`
}

type PackExecutionPolicy struct {
	Pack           string                   `yaml:"pack"`
	Mode           string                   `yaml:"mode"`
	Reason         string                   `yaml:"reason,omitempty"`
	MaxConcurrency int                      `yaml:"max_concurrency,omitempty"`
	CPUUnits       int                      `yaml:"cpu_units,omitempty"`
	MemoryMB       int                      `yaml:"memory_mb,omitempty"`
	Container      ContainerExecutionConfig `yaml:"container,omitempty"`
}

type WorkerPoolPolicy struct {
	ID             string   `yaml:"id"`
	Mode           string   `yaml:"mode,omitempty"`
	MaxConcurrency int      `yaml:"max_concurrency,omitempty"`
	Packs          []string `yaml:"packs,omitempty"`
}

type ContainerExecutionConfig struct {
	Engine          string   `yaml:"engine,omitempty"`
	Image           string   `yaml:"image,omitempty"`
	Network         string   `yaml:"network,omitempty"`
	User            string   `yaml:"user,omitempty"`
	ReadOnlyRootFS  bool     `yaml:"read_only_rootfs,omitempty"`
	NoNewPrivileges *bool    `yaml:"no_new_privileges,omitempty"`
	CapDrop         []string `yaml:"cap_drop,omitempty"`
	Memory          string   `yaml:"memory,omitempty"`
	CPUs            string   `yaml:"cpus,omitempty"`
	PidsLimit       int      `yaml:"pids_limit,omitempty"`
	Seccomp         string   `yaml:"seccomp,omitempty"`
	AppArmor        string   `yaml:"apparmor,omitempty"`
	Tmpfs           []string `yaml:"tmpfs,omitempty"`
}

type ScopeTarget struct {
	Kind  string `yaml:"kind"`
	Value string `yaml:"value"`
}

type ProviderRuntime struct {
	ProviderName string
	Model        string
}

type ProviderAdapter interface {
	Prepare(context.Context, Provider) (ProviderRuntime, error)
}

func RunWorkspace(ctx context.Context, root string) (RunResult, error) {
	return RunWorkspaceWithOptions(ctx, root, RunOptions{})
}

func RunWorkspaceWithOptions(ctx context.Context, root string, options RunOptions) (RunResult, error) {
	doctor, err := DoctorWorkspaceWithOptions(root, DoctorOptions{RequireSafetyAuthorization: options.RequireSafetyAuthorization})
	if err != nil {
		return RunResult{}, err
	}
	if !doctor.Ready {
		if options.RequireSafetyAuthorization {
			if auditErr := persistBlockedWorkspaceSafetyAudit(ctx, root, doctor, options); auditErr != nil {
				return RunResult{}, auditErr
			}
		}
		return RunResult{}, fmt.Errorf("workspace is not ready: %s", strings.Join(doctor.Errors, "; "))
	}

	providerConfig, err := LoadProviderConfig(filepath.Join(root, "providers.yaml"))
	if err != nil {
		return RunResult{}, fmt.Errorf("load provider config: %w", err)
	}
	provider, ok := EnabledProvider(providerConfig)
	if !ok {
		return RunResult{}, fmt.Errorf("provider config has no enabled provider")
	}
	adapter, err := providerAdapterFor(provider)
	if err != nil {
		return RunResult{}, err
	}
	providerRuntime, err := adapter.Prepare(ctx, provider)
	if err != nil {
		return RunResult{}, err
	}

	manifests, err := toolchain.LoadManifests(filepath.Join(root, "toolchain"))
	if err != nil {
		return RunResult{}, fmt.Errorf("load toolchain manifests: %w", err)
	}
	scopeConfig, err := LoadScopeConfig(filepath.Join(root, "scope.yaml"))
	if err != nil {
		return RunResult{}, fmt.Errorf("load scope config: %w", err)
	}
	if err := ValidateScopeConfig(scopeConfig); err != nil {
		return RunResult{}, fmt.Errorf("invalid scope config: %w", err)
	}
	if err := validatePackExecutionPoliciesAgainstManifests(scopeConfig.Execution.PackPolicies, manifests); err != nil {
		return RunResult{}, fmt.Errorf("invalid scope config: %w", err)
	}

	if err := os.MkdirAll(filepath.Join(root, "replay"), 0o755); err != nil {
		return RunResult{}, fmt.Errorf("create replay directory: %w", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "reports"), 0o755); err != nil {
		return RunResult{}, fmt.Errorf("create reports directory: %w", err)
	}

	store, err := eventlog.OpenSQLite(filepath.Join(root, "replay", "events.db"))
	if err != nil {
		return RunResult{}, err
	}
	defer store.Close()

	runID := fmt.Sprintf("run-%d", time.Now().UTC().UnixNano())
	events, err := runEvents(root, runID, providerRuntime, manifests, scopeConfig)
	if err != nil {
		return RunResult{}, err
	}
	events = append([]eventlog.Event{workspaceSafetyEvent(runID, doctor, scopeConfig, options.RequireSafetyAuthorization)}, events...)
	persistedEvents := []eventlog.Event{}
	for _, event := range events {
		if err := appendRunEvent(ctx, store, event, options, &persistedEvents); err != nil {
			return RunResult{}, err
		}
	}
	if err := runToolExecutionStage(ctx, root, runID, manifests, scopeConfig.Scope.Targets, scopeConfig.Scope.Exclusions, scopeConfig.Scope.TargetPolicy, scopeConfig.Execution.Isolation, scopeConfig.Execution.Container, scopeConfig.Execution.ResourceLimits, scopeConfig.Execution.PackPolicies, scopeConfig.Execution.WorkerPools, toolExecutionConcurrency(scopeConfig, options), toolRunnerFromOptions(options), store, options, &persistedEvents); err != nil {
		return RunResult{}, err
	}

	if options.DeferReportProjection {
		if err := appendRunEvent(ctx, store, stageEventWithReason(runID, "report-projection", "Project evidence graph into Markdown report", "skipped", "deferred"), options, &persistedEvents); err != nil {
			return RunResult{}, err
		}
		replayedEvents, err := store.Replay(ctx, runID)
		if err != nil {
			return RunResult{}, err
		}
		snapshot, err := eventlog.BuildReplaySnapshot(replayedEvents)
		if err != nil {
			return RunResult{}, err
		}
		tuiModel := tuiModelFromEventsForWorkspace(root, replayedEvents)
		return RunResult{
			RunID:      runID,
			EventCount: len(replayedEvents),
			StatusLine: tui.RenderStatusBar(100, tui.StatusFromReplay(snapshot)),
			TUIView:    tuiModel.View(),
			TUIModel:   tuiModel,
		}, nil
	}

	reportPath, err := reportProjectionPath(root, runID)
	if err != nil {
		return RunResult{}, err
	}
	if err := appendRunEvent(ctx, store, stageEvent(runID, "report-projection", "Project evidence graph into Markdown report", "started"), options, &persistedEvents); err != nil {
		return RunResult{}, err
	}

	replayedEvents, err := store.Replay(ctx, runID)
	if err != nil {
		return RunResult{}, err
	}
	snapshot, err := eventlog.BuildReplaySnapshot(replayedEvents)
	if err != nil {
		return RunResult{}, err
	}
	statusLine := tui.RenderStatusBar(100, tui.StatusFromReplay(snapshot))
	tuiModel := tuiModelFromEventsForWorkspace(root, replayedEvents)
	markdown := report.RenderMarkdown(report.Project(snapshot.Evidence))
	reportPath, err = writeReportProjectionMarkdown(root, runID, markdown)
	if err != nil {
		return RunResult{}, err
	}
	if err := appendRunEvent(ctx, store, eventlog.Event{
		RunID: runID,
		Type:  eventlog.EventReplay,
		Payload: mustJSON(map[string]any{
			"status":      "report_ready",
			"report_path": reportPath,
		}),
	}, options, &persistedEvents); err != nil {
		return RunResult{}, err
	}
	if err := appendRunEvent(ctx, store, stageEvent(runID, "report-projection", "Project evidence graph into Markdown report", "completed"), options, &persistedEvents); err != nil {
		return RunResult{}, err
	}
	replayedEvents, err = store.Replay(ctx, runID)
	if err != nil {
		return RunResult{}, err
	}
	snapshot, err = eventlog.BuildReplaySnapshot(replayedEvents)
	if err != nil {
		return RunResult{}, err
	}
	statusLine = tui.RenderStatusBar(100, tui.StatusFromReplay(snapshot))
	tuiModel = tuiModelFromEventsForWorkspace(root, replayedEvents)

	return RunResult{
		RunID:      runID,
		EventCount: len(replayedEvents),
		ReportPath: reportPath,
		StatusLine: statusLine,
		TUIView:    tuiModel.View(),
		TUIModel:   tuiModel,
	}, nil
}

func appendRunEvent(ctx context.Context, store *eventlog.SQLiteStore, event eventlog.Event, options RunOptions, persistedEvents *[]eventlog.Event) error {
	persisted, err := store.Append(ctx, event)
	if err != nil {
		return err
	}
	*persistedEvents = append(*persistedEvents, persisted)
	if options.OnEvent != nil {
		if err := options.OnEvent(persisted); err != nil {
			return fmt.Errorf("event observer: %w", err)
		}
	}
	if options.OnSnapshot != nil {
		snapshot, err := eventlog.BuildReplaySnapshot(*persistedEvents)
		if err != nil {
			return fmt.Errorf("build observer snapshot: %w", err)
		}
		if err := options.OnSnapshot(snapshot); err != nil {
			return fmt.Errorf("snapshot observer: %w", err)
		}
	}
	return nil
}

func persistBlockedWorkspaceSafetyAudit(ctx context.Context, root string, doctor DoctorReport, options RunOptions) error {
	defaultDoctor, err := DoctorWorkspaceWithOptions(root, DoctorOptions{})
	if err != nil {
		return err
	}
	if !defaultDoctor.Ready || len(doctor.SafetyBypasses) == 0 {
		return nil
	}
	scopeConfig, err := LoadScopeConfig(filepath.Join(root, "scope.yaml"))
	if err != nil {
		return fmt.Errorf("load scope config for workspace safety audit: %w", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "replay"), 0o755); err != nil {
		return fmt.Errorf("create replay directory: %w", err)
	}
	store, err := eventlog.OpenSQLite(filepath.Join(root, "replay", "events.db"))
	if err != nil {
		return err
	}
	defer store.Close()
	persistedEvents := []eventlog.Event{}
	runID := fmt.Sprintf("run-%d", time.Now().UTC().UnixNano())
	if err := appendRunEvent(ctx, store, workspaceSafetyEvent(runID, doctor, scopeConfig, options.RequireSafetyAuthorization), options, &persistedEvents); err != nil {
		return fmt.Errorf("record workspace safety audit: %w", err)
	}
	return nil
}

func workspaceSafetyEvent(runID string, report DoctorReport, scopeConfig ScopeConfig, requireSafetyAuthorization bool) eventlog.Event {
	targetPolicy, _ := normalizeScopeTargetPolicy(scopeConfig.Scope.TargetPolicy)
	isolation := executionIsolationMode(scopeConfig.Execution.Isolation)
	status := "ready"
	if len(report.Errors) > 0 {
		status = "blocked"
	} else if len(report.Warnings) > 0 || len(report.SafetyBypasses) > 0 {
		status = "warning"
	}
	gateMode := "audit"
	gateStatus := status
	gateErrorCount := 0
	if requireSafetyAuthorization {
		gateMode = "strict"
		gateStatus = "ready"
		gateErrorCount = safetyStrictGateErrorCount(report)
		if gateErrorCount > 0 {
			gateStatus = "blocked"
		}
	}
	return eventlog.Event{
		RunID: runID,
		Type:  eventlog.EventVerification,
		Payload: mustJSON(map[string]any{
			"kind":                         "workspace_safety",
			"status":                       status,
			"target_policy":                targetPolicy,
			"isolation":                    isolation,
			"require_safety_authorization": requireSafetyAuthorization,
			"warning_count":                len(report.Warnings),
			"error_count":                  len(report.Errors),
			"warnings":                     append([]string{}, report.Warnings...),
			"errors":                       append([]string{}, report.Errors...),
			"safety_bypasses":              append([]DoctorSafetyBypass{}, report.SafetyBypasses...),
			"strict_gate": map[string]any{
				"mode":        gateMode,
				"status":      gateStatus,
				"error_count": gateErrorCount,
			},
		}),
	}
}

func safetyStrictGateErrorCount(report DoctorReport) int {
	count := 0
	for _, bypass := range report.SafetyBypasses {
		if bypass.MissingAuthorization || bypass.ExpiredAuthorization {
			count++
		}
	}
	return count
}

func toolRunnerFromOptions(options RunOptions) ToolExecutionRunner {
	if options.ToolRunner != nil {
		return options.ToolRunner
	}
	return defaultToolExecutionRunner{}
}

func toolExecutionConcurrency(config ScopeConfig, options RunOptions) int {
	if options.ToolConcurrency > 0 {
		return options.ToolConcurrency
	}
	if config.Execution.ToolConcurrency > 0 {
		return config.Execution.ToolConcurrency
	}
	return 1
}

func runToolExecutionStage(ctx context.Context, root string, runID string, manifests []toolchain.Manifest, targets []ScopeTarget, exclusions []string, targetPolicy string, isolationMode string, container ContainerExecutionConfig, resourceLimits ExecutionResourceLimits, packPolicies []PackExecutionPolicy, workerPools []WorkerPoolPolicy, concurrency int, runner ToolExecutionRunner, store *eventlog.SQLiteStore, options RunOptions, persistedEvents *[]eventlog.Event) error {
	if err := appendRunEvent(ctx, store, stageEvent(runID, "tool-execution", "Execute compiled tool plan nodes", "started"), options, persistedEvents); err != nil {
		return err
	}

	plan, err := compiler.NewCompiler().Compile(compiler.CompileRequest{
		Action: compiler.ActionDiscoverAssets,
		Evidence: compiler.EvidenceSlice{
			Nodes: []evidence.Node{
				{
					ID:         "fact-workspace",
					Type:       evidence.NodeTarget,
					Label:      filepath.Base(root),
					SourceKind: "workspace",
				},
			},
		},
		Manifests: manifests,
	})
	if err != nil {
		return fmt.Errorf("compile tool execution plan: %w", err)
	}

	requests := toolExecutionRequests(root, runID, plan.Nodes, targets, exclusions, targetPolicy, isolationMode, container, resourceLimits, packPolicies, workerPools)
	var lifecycleMu sync.Mutex
	appendLifecycleEvent := func(event eventlog.Event) error {
		lifecycleMu.Lock()
		defer lifecycleMu.Unlock()
		return appendRunEvent(ctx, store, event, options, persistedEvents)
	}
	runnableRequests := make([]ToolExecutionRequest, 0, len(requests))
	for _, request := range requests {
		if err := appendLifecycleEvent(toolSchedulerAdmissionEvent(runID, request)); err != nil {
			return err
		}
		if packExecutionPolicyDisabled(request.ExecutionPolicy) {
			if err := appendLifecycleEvent(toolExecutionEvent(runID, request, skippedToolExecutionResult(request))); err != nil {
				return err
			}
			continue
		}
		runnableRequests = append(runnableRequests, request)
	}
	for _, request := range runnableRequests {
		if err := appendLifecycleEvent(toolWorkerQueuedEvent(runID, request)); err != nil {
			return err
		}
	}
	outcomes, err := executeToolRequests(ctx, runnableRequests, concurrency, runner, options.PauseController, func(request ToolExecutionRequest, reason string) error {
		return appendLifecycleEvent(toolWorkerWaitingEvent(runID, request, reason))
	}, func(request ToolExecutionRequest) error {
		if err := acquireWorkerLease(root, runID, request, options.RequireSafetyAuthorization); err != nil {
			return err
		}
		if err := appendLifecycleEvent(toolWorkerStartedEvent(runID, request)); err != nil {
			_ = completeWorkerLease(root, runID, request, ToolExecutionResult{Status: "failed", Transcript: err.Error()}, err, options.RequireSafetyAuthorization)
			return err
		}
		return nil
	}, func(outcome toolExecutionOutcome) error {
		result := toolExecutionResultForOutcome(outcome)
		leaseErr := completeWorkerLease(root, runID, outcome.request, result, outcome.err, options.RequireSafetyAuthorization)
		if err := appendLifecycleEvent(toolExecutionEvent(runID, outcome.request, result)); err != nil {
			if outcome.err != nil {
				return errors.Join(fmt.Errorf("execute tool plan node %s: %w", outcome.request.Node.ID, outcome.err), fmt.Errorf("record failed tool execution: %w", err), leaseErr)
			}
			return errors.Join(err, leaseErr)
		}
		return leaseErr
	})
	if err != nil {
		return err
	}
	var firstToolErr error
	for _, outcome := range outcomes {
		if outcome.err != nil && firstToolErr == nil {
			firstToolErr = fmt.Errorf("execute tool plan node %s: %w", outcome.request.Node.ID, outcome.err)
		}
	}
	if firstToolErr != nil {
		return firstToolErr
	}

	if err := appendRunEvent(ctx, store, stageEvent(runID, "tool-execution", "Execute compiled tool plan nodes", "completed"), options, persistedEvents); err != nil {
		return err
	}
	return nil
}

type toolExecutionOutcome struct {
	request ToolExecutionRequest
	result  ToolExecutionResult
	err     error
	index   int
}

func toolExecutionResultForOutcome(outcome toolExecutionOutcome) ToolExecutionResult {
	result := outcome.result
	if outcome.err != nil {
		if strings.TrimSpace(result.Status) == "" {
			result.Status = "failed"
		}
		if strings.TrimSpace(result.Transcript) == "" {
			result.Transcript = outcome.err.Error()
		}
	}
	return result
}

func executeToolRequests(ctx context.Context, requests []ToolExecutionRequest, concurrency int, runner ToolExecutionRunner, pauseController *RunPauseController, onWaiting func(ToolExecutionRequest, string) error, onStarted func(ToolExecutionRequest) error, onCompleted func(toolExecutionOutcome) error) ([]toolExecutionOutcome, error) {
	admissionLimiter := newToolAdmissionLimiter(requests, concurrency)
	if concurrency <= 1 || len(requests) <= 1 {
		outcomes := make([]toolExecutionOutcome, 0, len(requests))
		for index, request := range requests {
			if err := waitForRunResume(ctx, pauseController, request, onWaiting); err != nil {
				return outcomes, err
			}
			releaseAdmission, err := admissionLimiter.acquire(ctx, request, func(reason string) error {
				if onWaiting == nil {
					return nil
				}
				return onWaiting(request, reason)
			})
			if err != nil {
				return outcomes, err
			}
			if onStarted != nil {
				if err := onStarted(request); err != nil {
					releaseAdmission()
					return nil, err
				}
			}
			result, err := runner.Execute(ctx, request)
			releaseAdmission()
			outcome := toolExecutionOutcome{request: request, result: result, err: err, index: index}
			outcomes = append(outcomes, outcome)
			if onCompleted != nil {
				if completeErr := onCompleted(outcome); completeErr != nil {
					return outcomes, completeErr
				}
			}
			if err != nil {
				break
			}
		}
		return outcomes, nil
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	results := make(chan toolExecutionOutcome, len(requests))
	var wg sync.WaitGroup

	go func() {
		for index, request := range requests {
			wg.Add(1)
			go func(index int, request ToolExecutionRequest) {
				defer wg.Done()
				if err := waitForRunResume(ctx, pauseController, request, onWaiting); err != nil {
					cancel()
					results <- toolExecutionOutcome{request: request, err: err, index: index}
					return
				}
				releaseAdmission, err := admissionLimiter.acquire(ctx, request, func(reason string) error {
					if onWaiting == nil {
						return nil
					}
					return onWaiting(request, reason)
				})
				if err != nil {
					cancel()
					results <- toolExecutionOutcome{request: request, err: err, index: index}
					return
				}
				defer releaseAdmission()
				if onStarted != nil {
					if err := onStarted(request); err != nil {
						cancel()
						results <- toolExecutionOutcome{request: request, err: err, index: index}
						return
					}
				}
				result, err := runner.Execute(ctx, request)
				if err != nil {
					cancel()
				}
				results <- toolExecutionOutcome{request: request, result: result, err: err, index: index}
			}(index, request)
		}

		wg.Wait()
		close(results)
	}()
	outcomes := make([]toolExecutionOutcome, 0, len(requests))
	var completionErr error
	for outcome := range results {
		outcomes = append(outcomes, outcome)
		if completionErr == nil && onCompleted != nil {
			if err := onCompleted(outcome); err != nil && completionErr == nil {
				completionErr = err
				cancel()
			}
		}
	}
	sort.SliceStable(outcomes, func(i int, j int) bool {
		if outcomes[i].err != nil && outcomes[j].err == nil {
			return true
		}
		if outcomes[i].err == nil && outcomes[j].err != nil {
			return false
		}
		return outcomes[i].index < outcomes[j].index
	})
	if completionErr != nil {
		return outcomes, completionErr
	}
	return outcomes, nil
}

func waitForRunResume(ctx context.Context, pauseController *RunPauseController, request ToolExecutionRequest, onWaiting func(ToolExecutionRequest, string) error) error {
	if pauseController == nil {
		return nil
	}
	return pauseController.waitIfPaused(ctx, func() error {
		if onWaiting == nil {
			return nil
		}
		return onWaiting(request, "run_paused")
	})
}

type toolAdmissionLimiter struct {
	mu           sync.Mutex
	globalLimit  int
	activeGlobal int
	cpuLimit     int
	activeCPU    int
	memoryLimit  int
	activeMemory int
	packLimits   map[string]int
	activePack   map[string]int
	workerLimits map[string]int
	activeWorker map[string]int
	notify       chan struct{}
}

func newToolAdmissionLimiter(requests []ToolExecutionRequest, globalLimit int) *toolAdmissionLimiter {
	if globalLimit <= 0 {
		globalLimit = 1
	}
	limiter := &toolAdmissionLimiter{
		globalLimit:  globalLimit,
		packLimits:   map[string]int{},
		activePack:   map[string]int{},
		workerLimits: map[string]int{},
		activeWorker: map[string]int{},
		notify:       make(chan struct{}),
	}
	for _, request := range requests {
		if request.ResourceLimitCPUUnits > 0 && limiter.cpuLimit == 0 {
			limiter.cpuLimit = request.ResourceLimitCPUUnits
		}
		if request.ResourceLimitMemoryMB > 0 && limiter.memoryLimit == 0 {
			limiter.memoryLimit = request.ResourceLimitMemoryMB
		}
		if request.PackMaxConcurrency > 0 {
			key := packConcurrencyKey(request)
			if key != "" {
				if _, exists := limiter.packLimits[key]; !exists {
					limiter.packLimits[key] = request.PackMaxConcurrency
				}
			}
		}
		if request.WorkerPoolMaxConcurrency <= 0 {
			continue
		}
		workerKey := workerConcurrencyKey(request)
		if workerKey == "" {
			continue
		}
		if _, exists := limiter.workerLimits[workerKey]; !exists {
			limiter.workerLimits[workerKey] = request.WorkerPoolMaxConcurrency
		}
	}
	return limiter
}

func (l *toolAdmissionLimiter) acquire(ctx context.Context, request ToolExecutionRequest, onWaiting func(string) error) (func(), error) {
	cpu := request.ResourceCPUUnits
	memory := request.ResourceMemoryMB
	packKey := packConcurrencyKey(request)
	workerKey := workerConcurrencyKey(request)
	if l.cpuLimit > 0 && cpu > l.cpuLimit {
		return nil, fmt.Errorf("resource cpu_units request %d exceeds limit %d", cpu, l.cpuLimit)
	}
	if l.memoryLimit > 0 && memory > l.memoryLimit {
		return nil, fmt.Errorf("resource memory_mb request %d exceeds limit %d", memory, l.memoryLimit)
	}
	reportedWaiting := false
	for {
		l.mu.Lock()
		if l.canAcquireLocked(packKey, workerKey, cpu, memory) {
			l.activeGlobal++
			l.activeCPU += cpu
			l.activeMemory += memory
			if packKey != "" && l.packLimits[packKey] > 0 {
				l.activePack[packKey]++
			}
			if workerKey != "" && l.workerLimits[workerKey] > 0 {
				l.activeWorker[workerKey]++
			}
			l.mu.Unlock()
			return func() { l.release(packKey, workerKey, cpu, memory) }, nil
		}
		waitReason := l.waitReasonLocked(packKey, workerKey, cpu, memory)
		notify := l.notify
		l.mu.Unlock()
		if !reportedWaiting && onWaiting != nil {
			if err := onWaiting(waitReason); err != nil {
				return nil, err
			}
			reportedWaiting = true
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-notify:
		}
	}
}

func (l *toolAdmissionLimiter) waitReasonLocked(packKey string, workerKey string, cpu int, memory int) string {
	if l.globalLimit > 0 && l.activeGlobal+1 > l.globalLimit {
		return "global_concurrency"
	}
	if limit := l.packLimits[packKey]; limit > 0 && l.activePack[packKey]+1 > limit {
		return "pack_max_concurrency"
	}
	if limit := l.workerLimits[workerKey]; limit > 0 && l.activeWorker[workerKey]+1 > limit {
		return "worker_pool_max_concurrency"
	}
	if l.cpuLimit > 0 && cpu > 0 && l.activeCPU+cpu > l.cpuLimit {
		return "cpu_units"
	}
	if l.memoryLimit > 0 && memory > 0 && l.activeMemory+memory > l.memoryLimit {
		return "memory_mb"
	}
	return "unknown"
}

func (l *toolAdmissionLimiter) canAcquireLocked(packKey string, workerKey string, cpu int, memory int) bool {
	if l.globalLimit > 0 && l.activeGlobal+1 > l.globalLimit {
		return false
	}
	if limit := l.packLimits[packKey]; limit > 0 && l.activePack[packKey]+1 > limit {
		return false
	}
	if limit := l.workerLimits[workerKey]; limit > 0 && l.activeWorker[workerKey]+1 > limit {
		return false
	}
	if l.cpuLimit > 0 && cpu > 0 && l.activeCPU+cpu > l.cpuLimit {
		return false
	}
	if l.memoryLimit > 0 && memory > 0 && l.activeMemory+memory > l.memoryLimit {
		return false
	}
	return true
}

func (l *toolAdmissionLimiter) release(packKey string, workerKey string, cpu int, memory int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.activeGlobal--
	l.activeCPU -= cpu
	l.activeMemory -= memory
	if packKey != "" && l.packLimits[packKey] > 0 {
		l.activePack[packKey]--
	}
	if workerKey != "" && l.workerLimits[workerKey] > 0 {
		l.activeWorker[workerKey]--
	}
	close(l.notify)
	l.notify = make(chan struct{})
}

func packConcurrencyKey(request ToolExecutionRequest) string {
	return strings.ToLower(strings.TrimSpace(request.Node.ManifestName))
}

func workerConcurrencyKey(request ToolExecutionRequest) string {
	return strings.ToLower(strings.TrimSpace(request.WorkerID))
}

func toolExecutionRequests(root string, runID string, nodes []compiler.PlanNode, targets []ScopeTarget, exclusions []string, targetPolicy string, isolationMode string, container ContainerExecutionConfig, resourceLimits ExecutionResourceLimits, packPolicies []PackExecutionPolicy, workerPools []WorkerPoolPolicy) []ToolExecutionRequest {
	requests := make([]ToolExecutionRequest, 0, len(nodes))
	for _, node := range nodes {
		requests = append(requests, toolExecutionRequest(root, runID, node, targets, exclusions, targetPolicy, isolationMode, container, resourceLimits, packPolicies, workerPools))
	}
	return requests
}

func toolExecutionRequest(root string, runID string, node compiler.PlanNode, targets []ScopeTarget, exclusions []string, targetPolicy string, isolationMode string, container ContainerExecutionConfig, resourceLimits ExecutionResourceLimits, packPolicies []PackExecutionPolicy, workerPools []WorkerPoolPolicy) ToolExecutionRequest {
	policy, hasPolicy := packExecutionPolicyForManifest(node.ManifestName, packPolicies)
	policyMode := ""
	policyReason := ""
	requestContainer := container
	containerOverrides := false
	packMaxConcurrency := 0
	resourceCPUUnits := 0
	resourceMemoryMB := 0
	if hasPolicy {
		requestContainer = mergeContainerExecutionConfig(container, policy.Container)
		containerOverrides = containerConfigHasValues(policy.Container)
		if mode, ok := normalizePackExecutionPolicyMode(policy.Mode); ok {
			policyMode = mode
		}
		policyReason = strings.TrimSpace(policy.Reason)
		if policy.MaxConcurrency > 0 {
			packMaxConcurrency = policy.MaxConcurrency
		}
		if policy.CPUUnits > 0 {
			resourceCPUUnits = policy.CPUUnits
		}
		if policy.MemoryMB > 0 {
			resourceMemoryMB = policy.MemoryMB
		}
	}
	workerID, workerMaxConcurrency := workerPoolPlacementForManifest(node.ManifestName, workerPools)
	return ToolExecutionRequest{
		RunID:                    runID,
		WorkspaceRoot:            root,
		Node:                     node,
		ScopeTargets:             append([]ScopeTarget(nil), targets...),
		ScopeExclusions:          append([]string(nil), exclusions...),
		ScopeTargetPolicy:        targetPolicy,
		IsolationMode:            isolationMode,
		Container:                requestContainer,
		ExecutionPolicy:          policyMode,
		PolicyReason:             policyReason,
		WorkerID:                 workerID,
		WorkerPoolMaxConcurrency: workerMaxConcurrency,
		ContainerOverrides:       containerOverrides,
		PackMaxConcurrency:       packMaxConcurrency,
		ResourceCPUUnits:         resourceCPUUnits,
		ResourceMemoryMB:         resourceMemoryMB,
		ResourceLimitCPUUnits:    resourceLimits.CPUUnits,
		ResourceLimitMemoryMB:    resourceLimits.MemoryMB,
	}
}

func workerIDForManifest(manifestName string, workerPools []WorkerPoolPolicy) string {
	workerID, _ := workerPoolPlacementForManifest(manifestName, workerPools)
	return workerID
}

func workerPoolPlacementForManifest(manifestName string, workerPools []WorkerPoolPolicy) (string, int) {
	pack := strings.ToLower(strings.TrimSpace(manifestName))
	for _, pool := range workerPools {
		for _, candidate := range pool.Packs {
			if strings.ToLower(strings.TrimSpace(candidate)) == pack {
				return strings.TrimSpace(pool.ID), pool.MaxConcurrency
			}
		}
	}
	return "local", 0
}

func toolSchedulerAdmissionEvent(runID string, request ToolExecutionRequest) eventlog.Event {
	policy := strings.TrimSpace(request.ExecutionPolicy)
	if policy == "" {
		policy = "default"
	}
	decision := "admitted"
	if packExecutionPolicyDisabled(request.ExecutionPolicy) {
		decision = "skipped"
	}
	payload := map[string]any{
		"kind":                 "scheduler_admission",
		"admission_id":         "admission-" + strings.ReplaceAll(request.Node.ID, ":", "-"),
		"tool":                 request.Node.ManifestName,
		"admission_action":     string(request.Node.Action),
		"admission_capability": request.Node.Capability,
		"admission_decision":   decision,
		"execution_policy":     policy,
		"isolation":            executionIsolationMode(request.IsolationMode),
		"runner":               request.Node.Runner,
		"parser":               request.Node.Parser,
		"worker_id":            request.WorkerID,
		"container_overrides":  request.ContainerOverrides,
		"input_node_ids":       request.Node.InputNodeIDs,
	}
	if request.WorkerPoolMaxConcurrency > 0 {
		payload["worker_pool_max_concurrency"] = request.WorkerPoolMaxConcurrency
	}
	if request.PackMaxConcurrency > 0 {
		payload["pack_max_concurrency"] = request.PackMaxConcurrency
	}
	if request.ResourceCPUUnits > 0 {
		payload["resource_cpu_units"] = request.ResourceCPUUnits
	}
	if request.ResourceMemoryMB > 0 {
		payload["resource_memory_mb"] = request.ResourceMemoryMB
	}
	if request.ResourceLimitCPUUnits > 0 {
		payload["resource_limit_cpu_units"] = request.ResourceLimitCPUUnits
	}
	if request.ResourceLimitMemoryMB > 0 {
		payload["resource_limit_memory_mb"] = request.ResourceLimitMemoryMB
	}
	if reason := strings.TrimSpace(request.PolicyReason); reason != "" {
		payload["policy_reason"] = reason
	}
	return eventlog.Event{
		RunID:   runID,
		Type:    eventlog.EventTool,
		Payload: mustJSON(payload),
	}
}

func toolWorkerQueuedEvent(runID string, request ToolExecutionRequest) eventlog.Event {
	return toolWorkerLifecycleEvent(runID, request, "queued", "")
}

func toolWorkerWaitingEvent(runID string, request ToolExecutionRequest, reason string) eventlog.Event {
	return toolWorkerLifecycleEvent(runID, request, "waiting", reason)
}

func toolWorkerStartedEvent(runID string, request ToolExecutionRequest) eventlog.Event {
	return toolWorkerLifecycleEvent(runID, request, "started", "")
}

func toolWorkerLifecycleEvent(runID string, request ToolExecutionRequest, status string, waitReason string) eventlog.Event {
	executionID := "tool-" + strings.ReplaceAll(request.Node.ID, ":", "-")
	payload := map[string]any{
		"tool":              request.Node.ManifestName,
		"execution_id":      executionID,
		"worker_status":     status,
		"worker_capability": request.Node.Capability,
		"action":            string(request.Node.Action),
		"runner":            request.Node.Runner,
		"parser":            request.Node.Parser,
		"worker_id":         request.WorkerID,
		"isolation":         executionIsolationMode(request.IsolationMode),
		"input_node_ids":    request.Node.InputNodeIDs,
	}
	if request.WorkerPoolMaxConcurrency > 0 {
		payload["worker_pool_max_concurrency"] = request.WorkerPoolMaxConcurrency
	}
	if strings.TrimSpace(waitReason) != "" {
		payload["wait_reason"] = strings.TrimSpace(waitReason)
	}
	return eventlog.Event{
		RunID:   runID,
		Type:    eventlog.EventTool,
		Payload: mustJSON(payload),
	}
}

func toolExecutionEvent(runID string, request ToolExecutionRequest, result ToolExecutionResult) eventlog.Event {
	executionID := strings.TrimSpace(result.ExecutionID)
	if executionID == "" {
		executionID = "tool-" + strings.ReplaceAll(request.Node.ID, ":", "-")
	}
	status := strings.TrimSpace(result.Status)
	if status == "" {
		status = "completed"
	}
	payload := map[string]any{
		"tool":             request.Node.ManifestName,
		"execution_id":     executionID,
		"execution_status": status,
		"action":           string(request.Node.Action),
		"capability":       request.Node.Capability,
		"runner":           request.Node.Runner,
		"parser":           request.Node.Parser,
		"worker_id":        request.WorkerID,
		"isolation":        executionIsolationMode(request.IsolationMode),
		"input_node_ids":   request.Node.InputNodeIDs,
		"transcript":       strings.TrimSpace(result.Transcript),
		"attempt_dir":      strings.TrimSpace(result.AttemptDir),
	}
	if request.WorkerPoolMaxConcurrency > 0 {
		payload["worker_pool_max_concurrency"] = request.WorkerPoolMaxConcurrency
	}
	if policy := strings.TrimSpace(request.ExecutionPolicy); policy != "" {
		payload["execution_policy"] = policy
	}
	if reason := strings.TrimSpace(request.PolicyReason); reason != "" {
		payload["policy_reason"] = reason
	}
	if executionIsolationMode(request.IsolationMode) == "container" {
		payload["container_engine"] = containerEngine(request.Container)
		payload["container_image"] = strings.TrimSpace(request.Container.Image)
		payload["container_network"] = containerNetwork(request.Container)
		payload["container_user"] = strings.TrimSpace(request.Container.User)
		payload["container_read_only_rootfs"] = request.Container.ReadOnlyRootFS
		payload["container_no_new_privileges"] = containerNoNewPrivileges(request.Container)
		payload["container_cap_drop"] = containerCapDrop(request.Container)
		payload["container_memory"] = strings.TrimSpace(request.Container.Memory)
		payload["container_cpus"] = strings.TrimSpace(request.Container.CPUs)
		payload["container_pids_limit"] = request.Container.PidsLimit
		payload["container_seccomp"] = strings.TrimSpace(request.Container.Seccomp)
		payload["container_apparmor"] = strings.TrimSpace(request.Container.AppArmor)
		payload["container_tmpfs"] = request.Container.Tmpfs
	}
	return eventlog.Event{
		RunID:   runID,
		Type:    eventlog.EventTool,
		Payload: mustJSON(payload),
	}
}

func skippedToolExecutionResult(request ToolExecutionRequest) ToolExecutionResult {
	transcript := "pack execution policy disabled"
	if reason := strings.TrimSpace(request.PolicyReason); reason != "" {
		transcript += ": " + reason
	}
	return ToolExecutionResult{
		Status:     "skipped",
		Transcript: transcript,
	}
}

func LoadScopeConfig(path string) (ScopeConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return ScopeConfig{}, err
	}
	var config ScopeConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return ScopeConfig{}, err
	}
	return config, nil
}

func ValidateScopeConfig(config ScopeConfig) error {
	if config.Version == 0 {
		return fmt.Errorf("version is required")
	}
	if _, ok := normalizeScopeTargetPolicy(config.Scope.TargetPolicy); !ok {
		return fmt.Errorf("scope target_policy is unsupported: %s", config.Scope.TargetPolicy)
	}
	isolation, ok := normalizeExecutionIsolation(config.Execution.Isolation)
	if !ok {
		return fmt.Errorf("execution isolation is unsupported: %s", config.Execution.Isolation)
	}
	if isolation == "container" {
		if err := validateContainerConfig(config.Execution.Container); err != nil {
			return err
		}
	}
	if config.Execution.ToolConcurrency < 0 {
		return fmt.Errorf("execution tool_concurrency must be non-negative")
	}
	if config.Execution.ResourceLimits.CPUUnits < 0 {
		return fmt.Errorf("execution resource_limits cpu_units must be non-negative")
	}
	if config.Execution.ResourceLimits.MemoryMB < 0 {
		return fmt.Errorf("execution resource_limits memory_mb must be non-negative")
	}
	seenWorkerPools := map[string]bool{}
	for i, pool := range config.Execution.WorkerPools {
		id := strings.TrimSpace(pool.ID)
		if id == "" {
			return fmt.Errorf("execution worker pool %d id is required", i)
		}
		mode := strings.TrimSpace(pool.Mode)
		if mode == "" {
			mode = "local"
		}
		if !strings.EqualFold(mode, "local") {
			return fmt.Errorf("execution worker pool %s mode is unsupported: %s", id, pool.Mode)
		}
		if pool.MaxConcurrency < 0 {
			return fmt.Errorf("execution worker pool %s max_concurrency must be non-negative", id)
		}
		key := strings.ToLower(id)
		if seenWorkerPools[key] {
			return fmt.Errorf("execution worker pool %s is duplicated", id)
		}
		seenWorkerPools[key] = true
		for packIndex, pack := range pool.Packs {
			if strings.TrimSpace(pack) == "" {
				return fmt.Errorf("execution worker pool %s pack %d is empty", id, packIndex)
			}
		}
	}
	seenPolicies := map[string]bool{}
	for i, policy := range config.Execution.PackPolicies {
		pack := strings.TrimSpace(policy.Pack)
		if pack == "" {
			return fmt.Errorf("execution pack policy %d pack is required", i)
		}
		if _, ok := normalizePackExecutionPolicyMode(policy.Mode); !ok {
			return fmt.Errorf("execution pack policy %s mode is unsupported: %s", pack, policy.Mode)
		}
		if policy.MaxConcurrency < 0 {
			return fmt.Errorf("execution pack policy %s max_concurrency must be non-negative", pack)
		}
		if policy.CPUUnits < 0 {
			return fmt.Errorf("execution pack policy %s cpu_units must be non-negative", pack)
		}
		if config.Execution.ResourceLimits.CPUUnits > 0 && policy.CPUUnits > config.Execution.ResourceLimits.CPUUnits {
			return fmt.Errorf("execution pack policy %s cpu_units request exceeds resource limit", pack)
		}
		if policy.MemoryMB < 0 {
			return fmt.Errorf("execution pack policy %s memory_mb must be non-negative", pack)
		}
		if config.Execution.ResourceLimits.MemoryMB > 0 && policy.MemoryMB > config.Execution.ResourceLimits.MemoryMB {
			return fmt.Errorf("execution pack policy %s memory_mb request exceeds resource limit", pack)
		}
		key := strings.ToLower(pack)
		if seenPolicies[key] {
			return fmt.Errorf("execution pack policy %s is duplicated", pack)
		}
		seenPolicies[key] = true
		if isolation == "container" && containerConfigHasValues(policy.Container) {
			mergedContainer := mergeContainerExecutionConfig(config.Execution.Container, policy.Container)
			if err := validateContainerConfig(mergedContainer); err != nil {
				return fmt.Errorf("execution pack policy %s container: %w", pack, err)
			}
		}
	}
	for i, target := range config.Scope.Targets {
		if strings.TrimSpace(target.Kind) == "" {
			return fmt.Errorf("target %d kind is required", i)
		}
		if strings.TrimSpace(target.Value) == "" {
			return fmt.Errorf("target %d value is required", i)
		}
		if _, ok := packForTargetKind(domain.TargetKind(strings.TrimSpace(target.Kind))); !ok {
			return fmt.Errorf("target %d kind is unsupported: %s", i, target.Kind)
		}
	}
	return nil
}

func validatePackExecutionPoliciesAgainstManifests(policies []PackExecutionPolicy, manifests []toolchain.Manifest) error {
	if len(policies) == 0 {
		return nil
	}
	manifestNames := map[string]bool{}
	for _, manifest := range manifests {
		manifestNames[strings.ToLower(strings.TrimSpace(manifest.Name))] = true
	}
	for _, policy := range policies {
		pack := strings.TrimSpace(policy.Pack)
		if pack == "" {
			continue
		}
		if !manifestNames[strings.ToLower(pack)] {
			return fmt.Errorf("execution pack policy references unknown pack: %s", pack)
		}
	}
	return nil
}

func validateContainerConfig(config ContainerExecutionConfig) error {
	if strings.TrimSpace(config.Image) == "" {
		return fmt.Errorf("execution container image is required")
	}
	switch containerNetwork(config) {
	case "bridge", "none", "host":
	default:
		return fmt.Errorf("execution container network is unsupported: %s", config.Network)
	}
	if config.PidsLimit < 0 {
		return fmt.Errorf("execution container pids_limit must be non-negative")
	}
	for i, capability := range config.CapDrop {
		if strings.TrimSpace(capability) == "" {
			return fmt.Errorf("execution container cap_drop %d is empty", i)
		}
	}
	for i, tmpfs := range config.Tmpfs {
		if strings.TrimSpace(tmpfs) == "" {
			return fmt.Errorf("execution container tmpfs %d is empty", i)
		}
	}
	return nil
}

func runEvents(root string, runID string, provider ProviderRuntime, manifests []toolchain.Manifest, scopeConfig ScopeConfig) ([]eventlog.Event, error) {
	goal := readMissionGoal(root)
	events := []eventlog.Event{
		stageEvent(runID, "workspace-bootstrap", "Load workspace mission, provider, and scope", "started"),
		{
			RunID: runID,
			Type:  eventlog.EventGoal,
			Payload: mustJSON(map[string]any{
				"goal":            goal,
				"context_percent": 1,
			}),
		},
		{
			RunID: runID,
			Type:  eventlog.EventAgent,
			Payload: mustJSON(map[string]any{
				"agent_id":         "agent-runner",
				"role":             "planner",
				"state":            "Complete",
				"progress_percent": 100,
				"model_usage": map[string]any{
					"provider":      provider.ProviderName,
					"model":         provider.Model,
					"input_tokens":  0,
					"output_tokens": 0,
					"cost":          0,
				},
			}),
		},
		{
			RunID: runID,
			Type:  eventlog.EventFact,
			Payload: mustJSON(map[string]any{
				"fact_id":    "fact-workspace",
				"kind":       "target",
				"statement":  "workspace:" + filepath.Base(root),
				"confidence": 1,
				"source":     "workspace",
			}),
		},
		{
			RunID: runID,
			Type:  eventlog.EventFact,
			Payload: mustJSON(map[string]any{
				"fact_id":    "artifact-replay-log",
				"kind":       "artifact",
				"statement":  "artifact://replay/" + runID + "/events.db",
				"confidence": 1,
				"source":     "runner",
			}),
		},
		{
			RunID: runID,
			Type:  eventlog.EventFact,
			Payload: mustJSON(map[string]any{
				"fact_id":    "suggestion-provider-adapter",
				"kind":       "suggestion",
				"statement":  "configure a real provider adapter before live target execution",
				"confidence": 1,
				"source":     "runner",
			}),
		},
		{
			RunID: runID,
			Type:  eventlog.EventIntent,
			Payload: mustJSON(map[string]any{
				"intent_id":    "intent-production-run",
				"kind":         "run",
				"goal":         "start production workspace execution path",
				"source_facts": []string{"fact-workspace"},
				"state":        "Open",
			}),
		},
		{
			RunID: runID,
			Type:  eventlog.EventHint,
			Payload: mustJSON(map[string]any{
				"hint_id": "hint-next-run",
				"kind":    "suggestion",
				"message": "configure a real provider adapter before live target execution",
				"source":  "runner",
			}),
		},
		stageEvent(runID, "workspace-bootstrap", "Load workspace mission, provider, and scope", "completed"),
		stageEvent(runID, "toolchain-health", "Validate workspace native toolchain manifests", "started"),
	}

	for _, manifest := range manifests {
		dependencies := toolchain.CheckDependenciesWithMetadata(context.Background(), manifest, toolchain.DefaultDependencyRunner{}, toolchain.DefaultMetadataProvider{})
		installPlan := toolchain.PlanDependencyInstalls(manifest, dependencies)
		health := toolchain.CheckHealth(manifest, toolchain.DefaultHealthRunner{})
		health = toolchain.ApplyDependencyHealth(health, dependencies)
		events = append(events, eventlog.Event{
			RunID:   runID,
			Type:    eventlog.EventTool,
			Payload: toolchainHealthPayload(health, dependencies, installPlan),
		})
	}
	events = append(events, stageEvent(runID, "toolchain-health", "Validate workspace native toolchain manifests", "completed"))

	events = append(events, stageEvent(runID, "hypothesis-seeding", "Seed initial hypotheses from domain packs", "started"))
	hypothesisEvents, err := domainHypothesisEvents(runID, scopeConfig)
	if err != nil {
		return nil, err
	}
	events = append(events, hypothesisEvents...)
	events = append(events, stageEvent(runID, "hypothesis-seeding", "Seed initial hypotheses from domain packs", "completed"))
	return events, nil
}

func toolchainHealthPayload(health toolchain.HealthReport, dependencies []toolchain.DependencyReport, installPlan []toolchain.DependencyInstallStep) json.RawMessage {
	payload := map[string]any{
		"tool":              health.ManifestName,
		"health":            string(health.State),
		"installed_version": health.InstalledVersion,
		"latest_version":    health.LatestVersion,
		"message":           health.Message,
	}
	if len(dependencies) > 0 {
		rows := make([]map[string]any, 0, len(dependencies))
		for _, dependency := range dependencies {
			rows = append(rows, map[string]any{
				"name":                dependency.Name,
				"kind":                dependency.Kind,
				"state":               string(dependency.State),
				"required":            dependency.Required,
				"installed_version":   dependency.InstalledVersion,
				"latest_version":      dependency.LatestVersion,
				"source":              dependency.Source,
				"image_digest":        dependency.ImageDigest,
				"latest_image_digest": dependency.LatestImageDigest,
				"metadata_source":     dependency.MetadataSource,
				"message":             dependency.Message,
			})
		}
		payload["dependencies"] = rows
	}
	if len(installPlan) > 0 {
		rows := make([]map[string]any, 0, len(installPlan))
		for _, step := range installPlan {
			rows = append(rows, map[string]any{
				"name":     step.Name,
				"kind":     step.Kind,
				"state":    string(step.State),
				"required": step.Required,
				"command":  step.Command,
				"hint":     step.Hint,
				"source":   step.Source,
				"digest":   step.ImageDigest,
			})
		}
		payload["install_plan"] = rows
	}
	return mustJSON(payload)
}

func stageEvent(runID string, stageID string, title string, status string) eventlog.Event {
	return stageEventWithReason(runID, stageID, title, status, "")
}

func stageEventWithReason(runID string, stageID string, title string, status string, reason string) eventlog.Event {
	payload := map[string]any{
		"attempt_id": "stage-" + stageID + "-" + status,
		"kind":       "stage",
		"stage_id":   stageID,
		"title":      title,
		"status":     status,
	}
	if strings.TrimSpace(reason) != "" {
		payload["reason"] = strings.TrimSpace(reason)
	}
	return eventlog.Event{
		RunID:   runID,
		Type:    eventlog.EventAttempt,
		Payload: mustJSON(payload),
	}
}

func domainHypothesisEvents(runID string, scopeConfig ScopeConfig) ([]eventlog.Event, error) {
	events := []eventlog.Event{}
	for _, target := range scopeConfig.Scope.Targets {
		kind := domain.TargetKind(strings.TrimSpace(target.Kind))
		pack, ok := packForTargetKind(kind)
		if !ok {
			return nil, fmt.Errorf("unsupported target kind: %s", target.Kind)
		}
		hypotheses, err := pack.InitialHypotheses(domain.Target{Kind: kind, Value: target.Value})
		if err != nil {
			return nil, err
		}
		for _, hypothesis := range hypotheses {
			events = append(events, eventlog.Event{
				RunID: runID,
				Type:  eventlog.EventIntent,
				Payload: mustJSON(map[string]any{
					"intent_id":          hypothesis.ID,
					"kind":               "hypothesis",
					"goal":               hypothesis.Title,
					"source_facts":       []string{"fact-workspace"},
					"state":              "Open",
					"target":             hypothesis.Target,
					"rationale":          hypothesis.Rationale,
					"action":             string(hypothesis.Action),
					"coverage_dimension": string(hypothesis.CoverageDimension),
					"confidence":         hypothesis.Confidence,
				}),
			})
		}
	}
	return events, nil
}

func packForTargetKind(kind domain.TargetKind) (domain.Pack, bool) {
	switch kind {
	case domain.TargetURL:
		return webapi.NewPack(), true
	case domain.TargetCIDR:
		return infra.NewPack(), true
	case domain.TargetChallenge:
		return ctf.NewPack(), true
	default:
		return nil, false
	}
}

type offlineProviderAdapter struct{}

func (offlineProviderAdapter) Prepare(ctx context.Context, provider Provider) (ProviderRuntime, error) {
	if err := ctx.Err(); err != nil {
		return ProviderRuntime{}, err
	}
	return ProviderRuntime{
		ProviderName: provider.Name,
		Model:        provider.Model,
	}, nil
}

func providerAdapterFor(provider Provider) (ProviderAdapter, error) {
	switch strings.TrimSpace(provider.Type) {
	case "offline":
		return offlineProviderAdapter{}, nil
	default:
		return nil, fmt.Errorf("provider adapter is not available for type %s", provider.Type)
	}
}

type defaultToolExecutionRunner struct{}

func (defaultToolExecutionRunner) Execute(ctx context.Context, request ToolExecutionRequest) (ToolExecutionResult, error) {
	if len(request.Node.RunCommand) > 0 || strings.TrimSpace(request.Node.Runner) == "command" {
		return commandToolExecutionRunner{}.Execute(ctx, request)
	}
	if strings.TrimSpace(request.Node.Runner) == "native" {
		nativeCommand, ok, err := nativeManagedRunCommand(request)
		if err != nil {
			result := nativeManagedToolExecutionResult(request)
			result.Transcript = err.Error()
			return result, err
		}
		if ok {
			nativeRequest := request
			nativeRequest.Node.RunCommand = nativeCommand
			return commandToolExecutionRunner{}.Execute(ctx, nativeRequest)
		}
	}
	return offlineToolExecutionRunner{}.Execute(ctx, request)
}

type commandToolExecutionRunner struct{}

func (commandToolExecutionRunner) Execute(ctx context.Context, request ToolExecutionRequest) (ToolExecutionResult, error) {
	if err := ctx.Err(); err != nil {
		return ToolExecutionResult{}, err
	}
	executionID := "command-" + safeExecutionID(request.Node.ID)
	result := ToolExecutionResult{
		ExecutionID: executionID,
		Status:      "failed",
		AttemptDir:  filepath.Join(request.WorkspaceRoot, "attempts", request.RunID, executionID),
	}
	if len(request.Node.RunCommand) == 0 {
		result.Transcript = "command runner requires run_command"
		return result, fmt.Errorf("command runner requires run_command for %s", request.Node.ID)
	}
	runCommand, err := expandRunCommand(request.Node.RunCommand, request)
	if err != nil {
		result.Transcript = err.Error()
		return result, err
	}
	if err := requireCommandScopeTarget(request); err != nil {
		result.Transcript = err.Error()
		return result, err
	}
	if err := validateRunCommandArguments(runCommand, request); err != nil {
		result.Transcript = err.Error()
		return result, err
	}
	if err := os.MkdirAll(result.AttemptDir, 0o755); err != nil {
		result.Transcript = err.Error()
		return result, fmt.Errorf("create attempt directory: %w", err)
	}
	if executionIsolationMode(request.IsolationMode) == "container" {
		return executeContainerRunCommand(ctx, request, result, runCommand)
	}
	command := strings.TrimSpace(runCommand[0])
	resolved, err := resolveRunCommandExecutable(command, request)
	if err != nil {
		result.Transcript = err.Error()
		return result, fmt.Errorf("resolve run command %s: %w", command, err)
	}

	commandCtx := ctx
	var cancel context.CancelFunc
	if request.Node.RunTimeoutMS > 0 {
		commandCtx, cancel = context.WithTimeout(ctx, time.Duration(request.Node.RunTimeoutMS)*time.Millisecond)
		defer cancel()
	}

	cmd := exec.CommandContext(commandCtx, resolved, runCommand[1:]...)
	cmd.Dir = commandWorkingDirectory(request, result.AttemptDir)
	cmd.Env = commandWorkerEnvironment(request, result.AttemptDir, executionID)
	output, err := cmd.CombinedOutput()
	result.Transcript = strings.TrimSpace(string(output))
	if commandCtx.Err() != nil {
		if errors.Is(commandCtx.Err(), context.DeadlineExceeded) && request.Node.RunTimeoutMS > 0 {
			result.Transcript = commandTimeoutTranscript(request.Node.RunTimeoutMS, result.Transcript)
			return result, fmt.Errorf("execute run command %s timed out after %dms: %w", command, request.Node.RunTimeoutMS, commandCtx.Err())
		}
		if result.Transcript == "" {
			result.Transcript = commandCtx.Err().Error()
		}
		return result, fmt.Errorf("execute run command %s: %w", command, commandCtx.Err())
	}
	if err != nil {
		if result.Transcript == "" {
			result.Transcript = err.Error()
		}
		return result, fmt.Errorf("execute run command %s: %w", command, err)
	}
	if result.Transcript == "" {
		result.Transcript = "command completed without output"
	}
	result.Status = "completed"
	return result, nil
}

func nativeManagedRunCommand(request ToolExecutionRequest) ([]string, bool, error) {
	if executionIsolationMode(request.IsolationMode) == "container" {
		return nil, false, nil
	}
	if catalogToolLevelNodeRequiresAdapterArgvBuilder(request.Node) {
		command, ok, err := managedBinaryRunCommand(request)
		if err != nil {
			return nil, true, err
		}
		if !ok {
			return nil, false, nil
		}
		if resolved, ok, err := workspaceToolchainExecutable(request.WorkspaceRoot, command[0]); err != nil {
			return nil, true, err
		} else if !ok {
			return nil, false, nil
		} else {
			command[0] = resolved
		}
		return command, true, nil
	}
	candidates := nativeManagedToolCandidates(request)
	for _, candidate := range candidates {
		if _, ok, err := workspaceToolchainExecutable(request.WorkspaceRoot, candidate); err != nil {
			return nil, true, err
		} else if ok {
			command := []string{candidate}
			if target, ok := scopedTargetForNode(request.Node, request.ScopeTargets); ok {
				command = append(command, target.Value)
			}
			return command, true, nil
		}
	}
	return nil, false, nil
}

func catalogToolLevelNodeRequiresAdapterArgvBuilder(node compiler.PlanNode) bool {
	return strings.TrimSpace(node.ToolID) != "" &&
		(strings.TrimSpace(node.AdapterID) != "" || strings.TrimSpace(node.Operation) != "")
}

func managedBinaryRunCommand(request ToolExecutionRequest) ([]string, bool, error) {
	node := request.Node
	if strings.TrimSpace(node.AdapterID) != "managed_binary" {
		return nil, false, nil
	}
	operation, ok := toolchain.DefaultAuthoredToolRegistry().AdapterOperation(node.AdapterID, node.Operation)
	if !ok {
		return nil, false, nil
	}
	binary := managedBinaryDependencyName(node)
	if binary == "" || !containsTrimmedValue(operation.DependencyNames, binary) {
		return nil, false, nil
	}
	targetValue, hasTarget := managedBinaryTargetValue(request)
	if !hasTarget {
		return nil, false, nil
	}
	return managedBinaryCommandFor(binary, node.Operation, targetValue)
}

func managedBinaryDependencyName(node compiler.PlanNode) string {
	if strings.TrimSpace(node.DependencyKind) == "binary" {
		if dependency := strings.TrimSpace(node.DependencyName); dependency != "" {
			return dependency
		}
	}
	if dependency := singleNativeBinaryDependency(node.Dependencies); dependency != "" {
		return dependency
	}
	return ""
}

func managedBinaryTargetValue(request ToolExecutionRequest) (string, bool) {
	if target, ok := scopedTargetForNode(request.Node, request.ScopeTargets); ok {
		return strings.TrimSpace(target.Value), true
	}
	return "", false
}

func managedBinaryCommandFor(binary string, operation string, targetValue string) ([]string, bool, error) {
	binary = strings.TrimSpace(binary)
	targetValue = strings.TrimSpace(targetValue)
	if binary == "" || targetValue == "" {
		return nil, false, nil
	}
	switch strings.TrimSpace(operation) {
	case "http_probe":
		if binary == "httpx" {
			return []string{binary, "-json", "-u", targetValue}, true, nil
		}
	case "service_fingerprint":
		if binary == "nmap" {
			return []string{binary, "-oX", "-", targetValue}, true, nil
		}
	}
	return nil, false, nil
}

func containsTrimmedValue(values []string, want string) bool {
	want = strings.TrimSpace(want)
	for _, value := range values {
		if strings.TrimSpace(value) == want {
			return true
		}
	}
	return false
}

func nativeManagedToolCandidates(request ToolExecutionRequest) []string {
	candidates := []string{}
	if name := strings.TrimSpace(request.Node.ManifestName); name != "" {
		candidates = append(candidates, name)
	}
	if dependency := singleNativeBinaryDependency(request.Node.Dependencies); dependency != "" && !containsStringValue(candidates, dependency) {
		candidates = append(candidates, dependency)
	}
	return candidates
}

func singleNativeBinaryDependency(dependencies []toolchain.Dependency) string {
	binaryDependencies := []string{}
	for _, dependency := range dependencies {
		if dependencyKindForManagedInstall(dependency) != "binary" {
			continue
		}
		name := strings.TrimSpace(dependency.Name)
		if name == "" {
			continue
		}
		binaryDependencies = append(binaryDependencies, name)
	}
	if len(binaryDependencies) != 1 {
		return ""
	}
	return binaryDependencies[0]
}

func containsStringValue(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func nativeManagedToolExecutionResult(request ToolExecutionRequest) ToolExecutionResult {
	executionID := "native-" + safeExecutionID(request.Node.ID)
	return ToolExecutionResult{
		ExecutionID: executionID,
		Status:      "failed",
		AttemptDir:  filepath.Join(request.WorkspaceRoot, "attempts", request.RunID, executionID),
	}
}

func resolveRunCommandExecutable(command string, request ToolExecutionRequest) (string, error) {
	if resolved, ok, err := workspaceToolchainExecutable(request.WorkspaceRoot, command); err != nil || ok {
		return resolved, err
	}
	return exec.LookPath(command)
}

func workspaceToolchainExecutable(root string, command string) (string, bool, error) {
	name, err := validateAuthoredToolName(command)
	if err != nil {
		return "", false, nil
	}
	candidate := filepath.Join(root, "toolchain", "bin", name)
	info, err := os.Lstat(candidate)
	if err != nil {
		if os.IsNotExist(err) {
			return "", false, nil
		}
		return "", false, fmt.Errorf("inspect workspace toolchain binary: %w", err)
	}
	if info.IsDir() {
		return "", true, fmt.Errorf("workspace toolchain binary is a directory: %s", candidate)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		resolved, err := filepath.EvalSymlinks(candidate)
		if err != nil {
			return "", true, fmt.Errorf("resolve workspace toolchain binary symlink: %w", err)
		}
		if err := requireWorkspaceManagedBundleTarget(root, resolved); err != nil {
			return "", true, err
		}
		return resolved, true, nil
	}
	if !info.Mode().IsRegular() {
		return "", true, fmt.Errorf("workspace toolchain binary must be a regular file or managed symlink: %s", candidate)
	}
	return candidate, true, nil
}

func requireWorkspaceManagedBundleTarget(root string, target string) error {
	managedRoot, err := filepath.Abs(filepath.Join(root, "toolchain", "managed-bundles"))
	if err != nil {
		return fmt.Errorf("resolve managed bundle root: %w", err)
	}
	resolvedTarget, err := filepath.Abs(target)
	if err != nil {
		return fmt.Errorf("resolve workspace toolchain binary target: %w", err)
	}
	rel, err := filepath.Rel(managedRoot, resolvedTarget)
	if err != nil {
		return fmt.Errorf("resolve workspace toolchain binary target relative path: %w", err)
	}
	if rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || filepath.IsAbs(rel) {
		return fmt.Errorf("workspace toolchain binary target must stay under %s", managedRoot)
	}
	info, err := os.Stat(resolvedTarget)
	if err != nil {
		return fmt.Errorf("inspect workspace toolchain binary target: %w", err)
	}
	if info.IsDir() || !info.Mode().IsRegular() {
		return fmt.Errorf("workspace toolchain binary target must be a regular file: %s", resolvedTarget)
	}
	return nil
}

func executeContainerRunCommand(ctx context.Context, request ToolExecutionRequest, result ToolExecutionResult, runCommand []string) (ToolExecutionResult, error) {
	engine := containerEngine(request.Container)
	resolved, err := exec.LookPath(engine)
	if err != nil {
		result.Transcript = err.Error()
		return result, fmt.Errorf("resolve container engine %s: %w", engine, err)
	}
	commandCtx := ctx
	var cancel context.CancelFunc
	if request.Node.RunTimeoutMS > 0 {
		commandCtx, cancel = context.WithTimeout(ctx, time.Duration(request.Node.RunTimeoutMS)*time.Millisecond)
		defer cancel()
	}

	args := containerRunArguments(request, result.ExecutionID, runCommand)
	cmd := exec.CommandContext(commandCtx, resolved, args...)
	cmd.Dir = request.WorkspaceRoot
	cmd.Env = containerEngineEnvironment()
	output, err := cmd.CombinedOutput()
	result.Transcript = strings.TrimSpace(string(output))
	if commandCtx.Err() != nil {
		if errors.Is(commandCtx.Err(), context.DeadlineExceeded) && request.Node.RunTimeoutMS > 0 {
			result.Transcript = commandTimeoutTranscript(request.Node.RunTimeoutMS, result.Transcript)
			return result, fmt.Errorf("execute container command %s timed out after %dms: %w", engine, request.Node.RunTimeoutMS, commandCtx.Err())
		}
		if result.Transcript == "" {
			result.Transcript = commandCtx.Err().Error()
		}
		return result, fmt.Errorf("execute container command %s: %w", engine, commandCtx.Err())
	}
	if err != nil {
		if result.Transcript == "" {
			result.Transcript = err.Error()
		}
		return result, fmt.Errorf("execute container command %s: %w", engine, err)
	}
	if result.Transcript == "" {
		result.Transcript = "container command completed without output"
	}
	result.Status = "completed"
	return result, nil
}

func containerRunArguments(request ToolExecutionRequest, executionID string, runCommand []string) []string {
	containerAttemptDir := containerAttemptDirectory(request.RunID, executionID)
	args := []string{
		"run",
		"--rm",
	}
	args = append(args, containerSandboxArguments(request.Container)...)
	args = append(args,
		"--volume", request.WorkspaceRoot+":/workspace",
		"--workdir", containerAttemptDir,
	)
	for _, env := range commandContainerContextEnvironment(request, containerAttemptDir, executionID) {
		args = append(args, "--env", env)
	}
	args = append(args, strings.TrimSpace(request.Container.Image))
	args = append(args, runCommand...)
	return args
}

func containerSandboxArguments(config ContainerExecutionConfig) []string {
	args := []string{"--network", containerNetwork(config)}
	if user := strings.TrimSpace(config.User); user != "" {
		args = append(args, "--user", user)
	}
	if config.ReadOnlyRootFS {
		args = append(args, "--read-only")
	}
	if containerNoNewPrivileges(config) {
		args = append(args, "--security-opt", "no-new-privileges")
	}
	for _, capability := range containerCapDrop(config) {
		args = append(args, "--cap-drop", capability)
	}
	if memory := strings.TrimSpace(config.Memory); memory != "" {
		args = append(args, "--memory", memory)
	}
	if cpus := strings.TrimSpace(config.CPUs); cpus != "" {
		args = append(args, "--cpus", cpus)
	}
	if config.PidsLimit > 0 {
		args = append(args, "--pids-limit", fmt.Sprintf("%d", config.PidsLimit))
	}
	if seccomp := strings.TrimSpace(config.Seccomp); seccomp != "" {
		args = append(args, "--security-opt", "seccomp="+seccomp)
	}
	if appArmor := strings.TrimSpace(config.AppArmor); appArmor != "" {
		args = append(args, "--security-opt", "apparmor="+appArmor)
	}
	for _, tmpfs := range config.Tmpfs {
		args = append(args, "--tmpfs", strings.TrimSpace(tmpfs))
	}
	return args
}

func containerNetwork(config ContainerExecutionConfig) string {
	if network := strings.TrimSpace(config.Network); network != "" {
		return strings.ToLower(network)
	}
	return "bridge"
}

func containerNoNewPrivileges(config ContainerExecutionConfig) bool {
	if config.NoNewPrivileges == nil {
		return true
	}
	return *config.NoNewPrivileges
}

func containerCapDrop(config ContainerExecutionConfig) []string {
	if config.CapDrop == nil {
		return []string{"ALL"}
	}
	capabilities := make([]string, 0, len(config.CapDrop))
	for _, capability := range config.CapDrop {
		capabilities = append(capabilities, strings.TrimSpace(capability))
	}
	return capabilities
}

func mergeContainerExecutionConfig(base ContainerExecutionConfig, override ContainerExecutionConfig) ContainerExecutionConfig {
	merged := base
	if value := strings.TrimSpace(override.Engine); value != "" {
		merged.Engine = value
	}
	if value := strings.TrimSpace(override.Image); value != "" {
		merged.Image = value
	}
	if value := strings.TrimSpace(override.Network); value != "" {
		merged.Network = value
	}
	if value := strings.TrimSpace(override.User); value != "" {
		merged.User = value
	}
	if override.ReadOnlyRootFS {
		merged.ReadOnlyRootFS = true
	}
	if override.NoNewPrivileges != nil {
		merged.NoNewPrivileges = override.NoNewPrivileges
	}
	if override.CapDrop != nil {
		merged.CapDrop = append([]string(nil), override.CapDrop...)
	}
	if value := strings.TrimSpace(override.Memory); value != "" {
		merged.Memory = value
	}
	if value := strings.TrimSpace(override.CPUs); value != "" {
		merged.CPUs = value
	}
	if override.PidsLimit > 0 {
		merged.PidsLimit = override.PidsLimit
	}
	if value := strings.TrimSpace(override.Seccomp); value != "" {
		merged.Seccomp = value
	}
	if value := strings.TrimSpace(override.AppArmor); value != "" {
		merged.AppArmor = value
	}
	if override.Tmpfs != nil {
		merged.Tmpfs = append([]string(nil), override.Tmpfs...)
	}
	return merged
}

func containerConfigHasValues(config ContainerExecutionConfig) bool {
	return strings.TrimSpace(config.Engine) != "" ||
		strings.TrimSpace(config.Image) != "" ||
		strings.TrimSpace(config.Network) != "" ||
		strings.TrimSpace(config.User) != "" ||
		config.ReadOnlyRootFS ||
		config.NoNewPrivileges != nil ||
		config.CapDrop != nil ||
		strings.TrimSpace(config.Memory) != "" ||
		strings.TrimSpace(config.CPUs) != "" ||
		config.PidsLimit != 0 ||
		strings.TrimSpace(config.Seccomp) != "" ||
		strings.TrimSpace(config.AppArmor) != "" ||
		config.Tmpfs != nil
}

func containerAttemptDirectory(runID string, executionID string) string {
	return path.Join("/workspace", "attempts", runID, executionID)
}

func packExecutionPolicyForManifest(manifestName string, policies []PackExecutionPolicy) (PackExecutionPolicy, bool) {
	for _, policy := range policies {
		if !strings.EqualFold(strings.TrimSpace(policy.Pack), strings.TrimSpace(manifestName)) {
			continue
		}
		return policy, true
	}
	return PackExecutionPolicy{}, false
}

func packExecutionPolicyDisabled(policy string) bool {
	normalized, _ := normalizePackExecutionPolicyMode(policy)
	return normalized == "disabled"
}

func normalizePackExecutionPolicyMode(mode string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "enabled":
		return "enabled", true
	case "disabled":
		return "disabled", true
	default:
		return "", false
	}
}

func commandContainerContextEnvironment(request ToolExecutionRequest, containerAttemptDir string, executionID string) []string {
	return commandContextEnvironmentFor(request, "/workspace", containerAttemptDir, executionID)
}

func containerEngineEnvironment() []string {
	if pathValue := os.Getenv("PATH"); strings.TrimSpace(pathValue) != "" {
		return []string{"PATH=" + pathValue}
	}
	return nil
}

func containerEngine(config ContainerExecutionConfig) string {
	if engine := strings.TrimSpace(config.Engine); engine != "" {
		return engine
	}
	return "docker"
}

func commandTimeoutTranscript(timeoutMS int, output string) string {
	message := fmt.Sprintf("command timed out after %dms", timeoutMS)
	if strings.TrimSpace(output) == "" {
		return message
	}
	return message + "\n" + strings.TrimSpace(output)
}

func requireCommandScopeTarget(request ToolExecutionRequest) error {
	requiredKind := targetKindForCapability(request.Node.Capability)
	if requiredKind == "" {
		return nil
	}
	if targetPolicyDisabled(request.ScopeTargetPolicy) {
		return nil
	}
	target, ok := scopedTargetForNode(request.Node, request.ScopeTargets)
	if !ok {
		return fmt.Errorf("command runner requires %s scope target for capability %s", requiredKind, request.Node.Capability)
	}
	if scopeTargetExcluded(target, request.ScopeExclusions) {
		return fmt.Errorf("scope target %s is excluded for capability %s", target.Value, request.Node.Capability)
	}
	return nil
}

func validateRunCommandArguments(command []string, request ToolExecutionRequest) error {
	if targetPolicyDisabled(request.ScopeTargetPolicy) {
		return nil
	}
	targetKind := targetKindForCapability(request.Node.Capability)
	if targetKind != string(domain.TargetURL) && targetKind != string(domain.TargetCIDR) && targetKind != string(domain.TargetChallenge) {
		return nil
	}
	target, ok := scopedTargetForNode(request.Node, request.ScopeTargets)
	if !ok {
		return nil
	}
	if targetKind == string(domain.TargetCIDR) {
		return validateCIDRRunCommandArguments(command, request, target)
	}
	if targetKind == string(domain.TargetChallenge) {
		return validateChallengeRunCommandArguments(command, request, target)
	}
	return validateWebRunCommandArguments(command, request, target)
}

func validateWebRunCommandArguments(command []string, request ToolExecutionRequest, target ScopeTarget) error {
	for _, argument := range command {
		for _, candidate := range webURLLiterals(argument) {
			if !webURLWithinScope(candidate, target.Value) {
				return fmt.Errorf("out-of-scope URL argument %s for capability %s", candidate, request.Node.Capability)
			}
		}
	}
	return nil
}

func validateCIDRRunCommandArguments(command []string, request ToolExecutionRequest, target ScopeTarget) error {
	scopedPrefix, ok := scopedIPPrefix(target.Value)
	if !ok {
		return nil
	}
	for _, argument := range command {
		for _, candidate := range cidrLiterals(argument) {
			if !ipArgumentWithinScope(candidate, scopedPrefix) {
				return fmt.Errorf("out-of-scope CIDR argument %s for capability %s", candidate, request.Node.Capability)
			}
		}
	}
	return nil
}

func validateChallengeRunCommandArguments(command []string, request ToolExecutionRequest, target ScopeTarget) error {
	for _, argument := range command {
		for _, candidate := range challengeLiterals(argument) {
			if strings.TrimSpace(candidate.Value) != strings.TrimSpace(target.Value) {
				return fmt.Errorf("out-of-scope challenge argument %s for capability %s", candidate.Raw, request.Node.Capability)
			}
		}
	}
	return nil
}

func webURLLiterals(argument string) []string {
	literals := []string{}
	for _, candidate := range targetLiteralFragments(argument) {
		if parsedWebURL(candidate) != nil {
			literals = append(literals, candidate)
		}
	}
	return literals
}

func webURLWithinScope(candidate string, scoped string) bool {
	candidateURL := parsedWebURL(candidate)
	scopedURL := parsedWebURL(scoped)
	if candidateURL == nil || scopedURL == nil {
		return strings.TrimSpace(candidate) == strings.TrimSpace(scoped)
	}
	if !strings.EqualFold(candidateURL.Scheme, scopedURL.Scheme) {
		return false
	}
	if canonicalWebURLHost(candidateURL) != canonicalWebURLHost(scopedURL) {
		return false
	}
	return urlPathWithinScope(candidateURL.EscapedPath(), scopedURL.EscapedPath())
}

func canonicalWebURLHost(parsed *url.URL) string {
	host := strings.ToLower(strings.TrimSpace(parsed.Hostname()))
	port := parsed.Port()
	if port == "" {
		return host
	}
	switch strings.ToLower(parsed.Scheme) {
	case "http":
		if port == "80" {
			return host
		}
	case "https":
		if port == "443" {
			return host
		}
	}
	return host + ":" + port
}

func parsedWebURL(value string) *url.URL {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil {
		return nil
	}
	if parsed.Host == "" {
		return nil
	}
	switch strings.ToLower(parsed.Scheme) {
	case "http", "https":
		return parsed
	default:
		return nil
	}
}

func urlPathWithinScope(candidatePath string, scopedPath string) bool {
	scopedPath = strings.TrimRight(scopedPath, "/")
	if scopedPath == "" {
		return true
	}
	candidatePath = strings.TrimRight(candidatePath, "/")
	return candidatePath == scopedPath || strings.HasPrefix(candidatePath, scopedPath+"/")
}

func cidrLiterals(argument string) []string {
	literals := []string{}
	for _, candidate := range targetLiteralFragments(argument) {
		if ipArgumentLiteral(candidate) {
			literals = append(literals, candidate)
		}
	}
	return literals
}

type challengeLiteral struct {
	Raw   string
	Value string
}

func challengeLiterals(argument string) []challengeLiteral {
	literals := []challengeLiteral{}
	for _, candidate := range targetLiteralFragments(argument) {
		switch {
		case strings.HasPrefix(candidate, "challenge:"):
			literals = append(literals, challengeLiteral{
				Raw:   candidate,
				Value: strings.TrimPrefix(candidate, "challenge:"),
			})
		case strings.HasPrefix(candidate, "ctf://"):
			literals = append(literals, challengeLiteral{
				Raw:   candidate,
				Value: strings.TrimPrefix(candidate, "ctf://"),
			})
		}
	}
	return literals
}

func targetLiteralFragments(argument string) []string {
	fragments := []string{}
	seen := map[string]bool{}
	for _, field := range strings.Fields(argument) {
		token := trimTargetLiteralFragment(field)
		appendTargetLiteralFragments(&fragments, seen, token)
		if _, rhs, ok := strings.Cut(token, "="); ok {
			appendTargetLiteralFragments(&fragments, seen, rhs)
		}
	}
	return fragments
}

func appendTargetLiteralFragments(fragments *[]string, seen map[string]bool, value string) {
	value = trimTargetLiteralFragment(value)
	if value == "" {
		return
	}
	if strings.Contains(value, ",") {
		for _, piece := range strings.Split(value, ",") {
			appendTargetLiteralFragments(fragments, seen, piece)
		}
		return
	}
	if seen[value] {
		return
	}
	seen[value] = true
	*fragments = append(*fragments, value)
}

func trimTargetLiteralFragment(value string) string {
	return strings.Trim(strings.TrimSpace(value), "\"'()[]{}<>,;")
}

func ipArgumentLiteral(value string) bool {
	if _, err := netip.ParseAddr(strings.TrimSpace(value)); err == nil {
		return true
	}
	if _, err := netip.ParsePrefix(strings.TrimSpace(value)); err == nil {
		return true
	}
	return false
}

func ipArgumentWithinScope(candidate string, scoped netip.Prefix) bool {
	if prefix, err := netip.ParsePrefix(strings.TrimSpace(candidate)); err == nil {
		return ipPrefixWithinScope(prefix, scoped)
	}
	if addr, err := netip.ParseAddr(strings.TrimSpace(candidate)); err == nil {
		return scoped.Contains(addr)
	}
	return true
}

func ipPrefixWithinScope(candidate netip.Prefix, scoped netip.Prefix) bool {
	candidate = candidate.Masked()
	scoped = scoped.Masked()
	return scoped.Contains(candidate.Addr()) && candidate.Bits() >= scoped.Bits()
}

func scopedIPPrefix(value string) (netip.Prefix, bool) {
	if prefix, err := netip.ParsePrefix(strings.TrimSpace(value)); err == nil {
		return prefix.Masked(), true
	}
	if addr, err := netip.ParseAddr(strings.TrimSpace(value)); err == nil {
		return netip.PrefixFrom(addr, addr.BitLen()), true
	}
	return netip.Prefix{}, false
}

func commandWorkerEnvironment(request ToolExecutionRequest, attemptDir string, executionID string) []string {
	env := commandEnvironmentBase(request)
	env = append(env, commandContextEnvironment(request, attemptDir, executionID)...)
	return env
}

func commandEnvironmentBase(request ToolExecutionRequest) []string {
	if executionIsolationMode(request.IsolationMode) == "disabled" {
		return append([]string(nil), os.Environ()...)
	}
	env := []string{}
	if path := os.Getenv("PATH"); strings.TrimSpace(path) != "" {
		env = append(env, "PATH="+path)
	}
	return env
}

func commandContextEnvironment(request ToolExecutionRequest, attemptDir string, executionID string) []string {
	return commandContextEnvironmentFor(request, request.WorkspaceRoot, attemptDir, executionID)
}

func commandContextEnvironmentFor(request ToolExecutionRequest, workspaceRoot string, attemptDir string, executionID string) []string {
	toolchainBin := filepath.Join(workspaceRoot, "toolchain", "bin")
	toolchainRules := filepath.Join(workspaceRoot, "toolchain", "rules")
	env := []string{
		"PATH=" + prependPathValue(toolchainBin, os.Getenv("PATH")),
		"OURO_WORKSPACE=" + workspaceRoot,
		"OURO_RUN_ID=" + request.RunID,
		"OURO_EXECUTION_ID=" + executionID,
		"OURO_ATTEMPT_DIR=" + attemptDir,
		"OURO_ACTION=" + string(request.Node.Action),
		"OURO_CAPABILITY=" + request.Node.Capability,
		"OURO_TOOL=" + request.Node.ManifestName,
		"OURO_TOOLCHAIN_BIN=" + toolchainBin,
		"OURO_TOOLCHAIN_RULES=" + toolchainRules,
	}
	if target, ok := scopedTargetForNode(request.Node, request.ScopeTargets); ok {
		targetKind := strings.TrimSpace(request.Node.TargetKind)
		if targetKind == "" {
			targetKind = target.Kind
		}
		env = append(env,
			"OURO_TARGET="+target.Value,
			"OURO_TARGET_KIND="+targetKind,
			"OURO_SCOPE_TARGET_KIND="+target.Kind,
		)
	}
	env = append(env, toolLevelCommandEnvironment(request)...)
	return env
}

func toolLevelCommandEnvironment(request ToolExecutionRequest) []string {
	node := request.Node
	return []string{
		"OURO_TOOL_ID=" + strings.TrimSpace(node.ToolID),
		"OURO_ADAPTER_ID=" + strings.TrimSpace(node.AdapterID),
		"OURO_OPERATION=" + strings.TrimSpace(node.Operation),
		"OURO_PARSER_ID=" + strings.TrimSpace(node.ParserID),
		"OURO_DEPENDENCY_NAME=" + strings.TrimSpace(node.DependencyName),
		"OURO_DEPENDENCY_KIND=" + strings.TrimSpace(node.DependencyKind),
	}
}

func prependPathValue(first string, rest string) string {
	first = strings.TrimSpace(first)
	rest = strings.TrimSpace(rest)
	switch {
	case first == "":
		return rest
	case rest == "":
		return first
	default:
		return first + string(os.PathListSeparator) + rest
	}
}

func commandWorkingDirectory(request ToolExecutionRequest, attemptDir string) string {
	if executionIsolationMode(request.IsolationMode) == "disabled" {
		return request.WorkspaceRoot
	}
	return attemptDir
}

func expandRunCommand(command []string, request ToolExecutionRequest) ([]string, error) {
	expanded := make([]string, 0, len(command))
	target, hasTarget := scopedTargetForNode(request.Node, request.ScopeTargets)
	for _, part := range command {
		value := part
		needsTarget := strings.Contains(value, "{{target}}") || strings.Contains(value, "{{target_kind}}")
		if needsTarget && !hasTarget {
			return nil, fmt.Errorf("target template requires matching scope target for capability %s", request.Node.Capability)
		}
		if hasTarget {
			value = strings.ReplaceAll(value, "{{target}}", target.Value)
			value = strings.ReplaceAll(value, "{{target_kind}}", target.Kind)
		}
		value = strings.ReplaceAll(value, "{{workspace}}", request.WorkspaceRoot)
		value = strings.ReplaceAll(value, "{{run_id}}", request.RunID)
		expanded = append(expanded, value)
	}
	return expanded, nil
}

func scopedTargetForNode(node compiler.PlanNode, targets []ScopeTarget) (ScopeTarget, bool) {
	wantedKind := targetKindForCapability(node.Capability)
	if wantedKind != "" {
		for _, target := range targets {
			if strings.TrimSpace(target.Kind) == wantedKind && strings.TrimSpace(target.Value) != "" {
				return ScopeTarget{Kind: strings.TrimSpace(target.Kind), Value: strings.TrimSpace(target.Value)}, true
			}
		}
		return ScopeTarget{}, false
	}
	for _, target := range targets {
		if strings.TrimSpace(target.Value) != "" {
			return ScopeTarget{Kind: strings.TrimSpace(target.Kind), Value: strings.TrimSpace(target.Value)}, true
		}
	}
	return ScopeTarget{}, false
}

func scopeTargetExcluded(target ScopeTarget, exclusions []string) bool {
	targetValue := strings.TrimSpace(target.Value)
	if targetValue == "" {
		return false
	}
	for _, exclusion := range exclusions {
		exclusion = strings.TrimSpace(exclusion)
		if exclusion == targetValue {
			return true
		}
		switch strings.TrimSpace(target.Kind) {
		case string(domain.TargetURL):
			if webURLWithinScope(targetValue, exclusion) {
				return true
			}
			if scopeURLWildcardExcluded(targetValue, exclusion) {
				return true
			}
		case string(domain.TargetCIDR):
			if scopeCIDRTargetExcluded(targetValue, exclusion) {
				return true
			}
			if wildcardPatternMatches(exclusion, canonicalIPTargetValue(targetValue)) || wildcardPatternMatches(exclusion, targetValue) {
				return true
			}
		default:
			if wildcardPatternMatches(exclusion, targetValue) {
				return true
			}
		}
	}
	return false
}

func scopeURLWildcardExcluded(targetValue string, exclusion string) bool {
	if !strings.Contains(exclusion, "*") {
		return false
	}
	targetURL := parsedWebURL(targetValue)
	exclusionURL := parsedWebURL(exclusion)
	if targetURL == nil || exclusionURL == nil {
		return wildcardPatternMatches(exclusion, targetValue)
	}
	return wildcardPatternMatches(canonicalWebURLForWildcard(exclusionURL), canonicalWebURLForWildcard(targetURL))
}

func canonicalWebURLForWildcard(parsed *url.URL) string {
	pathValue := strings.TrimRight(parsed.EscapedPath(), "/")
	if pathValue == "" {
		pathValue = "/"
	}
	return strings.ToLower(strings.TrimSpace(parsed.Scheme)) + "://" + canonicalWebURLHost(parsed) + pathValue
}

func scopeCIDRTargetExcluded(targetValue string, exclusion string) bool {
	excludedPrefix, ok := scopedIPPrefix(exclusion)
	if !ok {
		return false
	}
	if targetPrefix, err := netip.ParsePrefix(strings.TrimSpace(targetValue)); err == nil {
		return ipPrefixWithinScope(targetPrefix, excludedPrefix)
	}
	if targetAddr, err := netip.ParseAddr(strings.TrimSpace(targetValue)); err == nil {
		return excludedPrefix.Contains(targetAddr)
	}
	return false
}

func canonicalIPTargetValue(value string) string {
	if prefix, err := netip.ParsePrefix(strings.TrimSpace(value)); err == nil {
		return prefix.Masked().String()
	}
	if addr, err := netip.ParseAddr(strings.TrimSpace(value)); err == nil {
		return addr.String()
	}
	return strings.TrimSpace(value)
}

func wildcardPatternMatches(pattern string, value string) bool {
	pattern = strings.TrimSpace(pattern)
	value = strings.TrimSpace(value)
	if pattern == "" || !strings.Contains(pattern, "*") {
		return false
	}
	expression := "^" + strings.ReplaceAll(regexp.QuoteMeta(pattern), `\*`, ".*") + "$"
	matched, err := regexp.MatchString(expression, value)
	return err == nil && matched
}

func targetPolicyDisabled(policy string) bool {
	normalized, _ := normalizeScopeTargetPolicy(policy)
	return normalized == "disabled"
}

func normalizeScopeTargetPolicy(policy string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(policy)) {
	case "", "enforce":
		return "enforce", true
	case "disabled":
		return "disabled", true
	default:
		return "", false
	}
}

func executionIsolationMode(mode string) string {
	normalized, _ := normalizeExecutionIsolation(mode)
	return normalized
}

func normalizeExecutionIsolation(mode string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "", "process":
		return "process", true
	case "container":
		return "container", true
	case "disabled":
		return "disabled", true
	default:
		return "", false
	}
}

func targetKindForCapability(capability string) string {
	switch {
	case strings.HasPrefix(capability, "web."):
		return string(domain.TargetURL)
	case strings.HasPrefix(capability, "infra."):
		return string(domain.TargetCIDR)
	case strings.HasPrefix(capability, "ctf."):
		return string(domain.TargetChallenge)
	default:
		return ""
	}
}

type offlineToolExecutionRunner struct{}

func (offlineToolExecutionRunner) Execute(ctx context.Context, request ToolExecutionRequest) (ToolExecutionResult, error) {
	if err := ctx.Err(); err != nil {
		return ToolExecutionResult{}, err
	}
	return ToolExecutionResult{
		ExecutionID: "offline-" + strings.ReplaceAll(request.Node.ID, ":", "-"),
		Status:      "completed",
		Transcript:  fmt.Sprintf("offline tool execution boundary for %s/%s", request.Node.ManifestName, request.Node.Capability),
	}, nil
}

func safeExecutionID(value string) string {
	replacer := strings.NewReplacer(":", "-", "/", "-", "\\", "-")
	return replacer.Replace(strings.TrimSpace(value))
}

func readMissionGoal(root string) string {
	data, err := os.ReadFile(filepath.Join(root, "mission.yaml"))
	if err != nil {
		return "workspace run"
	}
	var decoded struct {
		Mission struct {
			Goal string `yaml:"goal"`
		} `yaml:"mission"`
	}
	if err := yaml.Unmarshal(data, &decoded); err != nil {
		return "workspace run"
	}
	if strings.TrimSpace(decoded.Mission.Goal) == "" {
		return "workspace run"
	}
	return strings.TrimSpace(decoded.Mission.Goal)
}

func mustJSON(value any) json.RawMessage {
	data, err := json.Marshal(value)
	if err != nil {
		return json.RawMessage(`{}`)
	}
	return data
}
