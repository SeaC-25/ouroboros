package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type RuntimeWorkerQueueDispatchExecutionOptions struct {
	Execute                bool
	Now                    time.Time
	TransportMode          string
	TransportAuthToken     string
	ResultCallbackEndpoint string
}

type RuntimeWorkerQueueDispatchExecution struct {
	Version                int                          `json:"version"`
	Path                   string                       `json:"-"`
	Status                 string                       `json:"status"`
	Mode                   string                       `json:"mode"`
	TransportMode          string                       `json:"transport_mode"`
	TransportAuth          string                       `json:"transport_auth,omitempty"`
	ClaimPath              string                       `json:"claim_path"`
	ClaimID                string                       `json:"claim_id"`
	WorkerID               string                       `json:"worker_id"`
	WorkerMode             string                       `json:"worker_mode"`
	WorkerEndpoint         string                       `json:"worker_endpoint,omitempty"`
	RunID                  string                       `json:"run_id"`
	ExecutionID            string                       `json:"execution_id"`
	Tool                   string                       `json:"tool"`
	Capability             string                       `json:"capability"`
	ToolPlan               RuntimeWorkerToolPlanPayload `json:"tool_plan,omitempty"`
	ExecutedAt             string                       `json:"executed_at"`
	TransportRequestPath   string                       `json:"transport_request_path,omitempty"`
	TransportEndpoint      string                       `json:"transport_endpoint,omitempty"`
	TransportHTTPStatus    int                          `json:"transport_http_status,omitempty"`
	TransportResponseBody  string                       `json:"transport_response_body,omitempty"`
	ResultCallbackEndpoint string                       `json:"result_callback_endpoint,omitempty"`
	ResultPath             string                       `json:"result_path,omitempty"`
	CompletedAt            string                       `json:"completed_at,omitempty"`
	SkipReason             string                       `json:"skip_reason,omitempty"`
	Error                  string                       `json:"error,omitempty"`
}

type RuntimeWorkerTransportDispatchRequest struct {
	Version                int                          `json:"version"`
	Path                   string                       `json:"-"`
	Status                 string                       `json:"status"`
	TransportMode          string                       `json:"transport_mode"`
	DispatchExecutionPath  string                       `json:"dispatch_execution_path"`
	ClaimPath              string                       `json:"claim_path"`
	ClaimID                string                       `json:"claim_id"`
	WorkerID               string                       `json:"worker_id"`
	WorkerEndpoint         string                       `json:"worker_endpoint,omitempty"`
	ResultCallbackEndpoint string                       `json:"result_callback_endpoint,omitempty"`
	RunID                  string                       `json:"run_id"`
	ExecutionID            string                       `json:"execution_id"`
	Tool                   string                       `json:"tool"`
	Capability             string                       `json:"capability"`
	ToolPlan               RuntimeWorkerToolPlanPayload `json:"tool_plan,omitempty"`
	CreatedAt              string                       `json:"created_at"`
	AckedAt                string                       `json:"acked_at,omitempty"`
	AckedBy                string                       `json:"acked_by,omitempty"`
	ResultStatus           string                       `json:"result_status,omitempty"`
	ResultPath             string                       `json:"result_path,omitempty"`
	CompletedAt            string                       `json:"completed_at,omitempty"`
}

type RuntimeWorkerTransportAckOptions struct {
	Ack bool
	Now time.Time
}

type RuntimeWorkerTransportRunOptions struct {
	Execute           bool
	Now               time.Time
	Command           []string
	Timeout           time.Duration
	UseRequestCommand bool
	HeartbeatLease    bool
	HeartbeatBy       string
	HeartbeatReason   string
}

type RuntimeWorkerTransportRunLoopOptions struct {
	Execute           bool
	Now               time.Time
	Command           []string
	Timeout           time.Duration
	MaxRequests       int
	UseRequestCommand bool
	HeartbeatLease    bool
	HeartbeatBy       string
	HeartbeatReason   string
}

type RuntimeWorkerTransportRunDaemonOptions struct {
	Execute               bool
	Now                   time.Time
	Command               []string
	Timeout               time.Duration
	MaxRequests           int
	MaxIdleScans          int
	PollInterval          time.Duration
	UseRequestCommand     bool
	EnableControlRequests bool
	HeartbeatLease        bool
	HeartbeatBy           string
	HeartbeatReason       string
}

type RuntimeWorkerTransportDaemonSupervisorOptions struct {
	Execute               bool
	Now                   time.Time
	Command               []string
	Timeout               time.Duration
	MaxRuns               int
	MaxRequests           int
	MaxIdleScans          int
	PollInterval          time.Duration
	UseRequestCommand     bool
	EnableControlRequests bool
	HeartbeatLease        bool
	HeartbeatBy           string
	HeartbeatReason       string
}

type RuntimeWorkerTransportDaemonControlRequestOptions struct {
	Now time.Time
}

type RuntimeWorkerTransportHTTPHandlerOptions struct {
	AuthToken string
}

type RuntimeWorkerQueueDispatchResultHTTPHandlerOptions struct {
	AuthToken string
}

type RuntimeWorkerTransportRunLoopResult struct {
	Status          string
	WorkerID        string
	Processed       int
	Completed       int
	Failed          int
	Skipped         int
	Results         []RuntimeWorkerQueueDispatchResult
	SkippedRequests []RuntimeWorkerTransportRunLoopSkip
}

type RuntimeWorkerTransportRunDaemonResult struct {
	Version            int                                 `json:"version"`
	Path               string                              `json:"-"`
	Status             string                              `json:"status"`
	Mode               string                              `json:"mode"`
	WorkerID           string                              `json:"worker_id"`
	StartedAt          string                              `json:"started_at"`
	CompletedAt        string                              `json:"completed_at"`
	CommandSource      string                              `json:"command_source"`
	MaxRequests        int                                 `json:"max_requests"`
	MaxIdleScans       int                                 `json:"max_idle_scans"`
	PollIntervalMS     int                                 `json:"poll_interval_ms"`
	UseRequestCommand  bool                                `json:"use_request_command"`
	Processed          int                                 `json:"processed"`
	Completed          int                                 `json:"completed"`
	Failed             int                                 `json:"failed"`
	Skipped            int                                 `json:"skipped"`
	Scans              int                                 `json:"scans"`
	IdleScans          int                                 `json:"idle_scans"`
	ResultPaths        []string                            `json:"result_paths,omitempty"`
	Results            []RuntimeWorkerQueueDispatchResult  `json:"results,omitempty"`
	SkippedRequests    []RuntimeWorkerTransportRunLoopSkip `json:"skipped_requests,omitempty"`
	ControlRequestPath string                              `json:"control_request_path,omitempty"`
	ControlAction      string                              `json:"control_action,omitempty"`
	ControlRequestedBy string                              `json:"control_requested_by,omitempty"`
	StopReason         string                              `json:"stop_reason,omitempty"`
	Error              string                              `json:"error,omitempty"`
}

type RuntimeWorkerTransportDaemonSupervisorRun struct {
	Version               int                                     `json:"version"`
	Path                  string                                  `json:"-"`
	Status                string                                  `json:"status"`
	Mode                  string                                  `json:"mode"`
	WorkerID              string                                  `json:"worker_id"`
	StartedAt             string                                  `json:"started_at"`
	CompletedAt           string                                  `json:"completed_at"`
	CommandSource         string                                  `json:"command_source"`
	MaxRuns               int                                     `json:"max_runs"`
	MaxRequests           int                                     `json:"max_requests"`
	MaxIdleScans          int                                     `json:"max_idle_scans"`
	PollIntervalMS        int                                     `json:"poll_interval_ms"`
	UseRequestCommand     bool                                    `json:"use_request_command"`
	EnableControlRequests bool                                    `json:"enable_control_requests"`
	Runs                  int                                     `json:"runs"`
	RestartCount          int                                     `json:"restart_count"`
	Processed             int                                     `json:"processed"`
	Completed             int                                     `json:"completed"`
	Failed                int                                     `json:"failed"`
	Skipped               int                                     `json:"skipped"`
	DaemonRunPaths        []string                                `json:"daemon_run_paths,omitempty"`
	DaemonRuns            []RuntimeWorkerTransportRunDaemonResult `json:"daemon_runs,omitempty"`
	LastDaemonStatus      string                                  `json:"last_daemon_status,omitempty"`
	Error                 string                                  `json:"error,omitempty"`
}

type RuntimeWorkerTransportDaemonControlRequest struct {
	Version              int    `json:"version"`
	Path                 string `json:"-"`
	Status               string `json:"status"`
	Mode                 string `json:"mode"`
	WorkerID             string `json:"worker_id"`
	Action               string `json:"action"`
	Reason               string `json:"reason,omitempty"`
	RequestedBy          string `json:"requested_by,omitempty"`
	RequestedAt          string `json:"requested_at"`
	AppliedAt            string `json:"applied_at,omitempty"`
	AppliedDaemonRunPath string `json:"applied_daemon_run_path,omitempty"`
}

type RuntimeWorkerTransportRunLoopSkip struct {
	RequestPath string
	WorkerID    string
	ExecutionID string
	Status      string
	Reason      string
}

type RuntimeWorkerQueueDispatchResultOptions struct {
	Now time.Time
}

type RuntimeWorkerQueueDispatchResult struct {
	Version               int                          `json:"version"`
	Path                  string                       `json:"-"`
	Status                string                       `json:"status"`
	Mode                  string                       `json:"mode"`
	DispatchExecutionPath string                       `json:"dispatch_execution_path"`
	ClaimPath             string                       `json:"claim_path"`
	ClaimID               string                       `json:"claim_id"`
	WorkerID              string                       `json:"worker_id"`
	WorkerMode            string                       `json:"worker_mode"`
	WorkerEndpoint        string                       `json:"worker_endpoint,omitempty"`
	RunID                 string                       `json:"run_id"`
	ExecutionID           string                       `json:"execution_id"`
	Tool                  string                       `json:"tool"`
	Capability            string                       `json:"capability"`
	ToolPlan              RuntimeWorkerToolPlanPayload `json:"tool_plan,omitempty"`
	SubmittedAt           string                       `json:"submitted_at"`
	Output                string                       `json:"output,omitempty"`
	Error                 string                       `json:"error,omitempty"`
}

func ExecuteRuntimeWorkerQueueClaimDispatch(root, claimPath string, options RuntimeWorkerQueueDispatchExecutionOptions) (RuntimeWorkerQueueDispatchExecution, error) {
	if !options.Execute {
		return RuntimeWorkerQueueDispatchExecution{}, fmt.Errorf("worker queue claim dispatch requires explicit execute")
	}
	transportMode, err := normalizeRuntimeWorkerQueueDispatchTransport(options.TransportMode)
	if err != nil {
		return RuntimeWorkerQueueDispatchExecution{}, err
	}
	resultCallbackEndpoint, err := normalizeRuntimeWorkerResultCallbackEndpoint(options.ResultCallbackEndpoint)
	if err != nil {
		return RuntimeWorkerQueueDispatchExecution{}, err
	}
	claim, err := ReadRuntimeWorkerQueueClaim(root, claimPath)
	if err != nil {
		return RuntimeWorkerQueueDispatchExecution{}, err
	}
	now := options.Now.UTC()
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if err := requireRuntimeWorkerSideEffectSafetyAuthorization(root, now); err != nil {
		return RuntimeWorkerQueueDispatchExecution{}, err
	}
	execution := runtimeWorkerQueueDispatchExecutionFromClaim(claim, now)
	execution.TransportMode = transportMode
	execution.ResultCallbackEndpoint = resultCallbackEndpoint
	if strings.TrimSpace(options.TransportAuthToken) != "" {
		execution.TransportAuth = "bearer"
	}
	if reason := runtimeWorkerQueueDispatchSkipReason(root, claim, now); reason != "" {
		execution.Status = "skipped"
		execution.SkipReason = reason
		if err := writeRuntimeWorkerQueueDispatchExecution(root, &execution); err != nil {
			return execution, err
		}
		return execution, nil
	}
	execution.Status = "dispatched"
	ensureRuntimeWorkerQueueDispatchExecutionPath(root, &execution)
	if transportMode == "file-outbox" {
		request, err := writeRuntimeWorkerTransportDispatchRequest(root, execution)
		if err != nil {
			return execution, err
		}
		execution.TransportRequestPath = request.Path
	}
	if transportMode == "http-json" {
		httpResult, err := postRuntimeWorkerHTTPJSONDispatch(execution, options.TransportAuthToken)
		execution.TransportEndpoint = httpResult.Endpoint
		execution.TransportHTTPStatus = httpResult.StatusCode
		execution.TransportResponseBody = httpResult.ResponseBody
		if err != nil {
			execution.Status = "failed"
			execution.Error = err.Error()
			if writeErr := writeRuntimeWorkerQueueDispatchExecution(root, &execution); writeErr != nil {
				return execution, writeErr
			}
			return execution, err
		}
	}
	if err := writeRuntimeWorkerQueueDispatchExecution(root, &execution); err != nil {
		return execution, err
	}
	claim.Status = "dispatched"
	claim.DispatchedAt = execution.ExecutedAt
	claim.DispatchExecutionPath = execution.Path
	if err := writeRuntimeWorkerQueueClaimArtifact(claim); err != nil {
		execution.Status = "failed"
		execution.Error = err.Error()
		if writeErr := writeRuntimeWorkerQueueDispatchExecution(root, &execution); writeErr != nil {
			return execution, writeErr
		}
		return execution, fmt.Errorf("mark worker queue claim dispatched: %w", err)
	}
	return execution, nil
}

func ListRuntimeWorkerQueueDispatchExecutions(root string) ([]RuntimeWorkerQueueDispatchExecution, error) {
	paths, err := filepath.Glob(filepath.Join(runtimeWorkerQueueDispatchExecutionDir(root), "*.json"))
	if err != nil {
		return nil, fmt.Errorf("glob worker queue dispatch executions: %w", err)
	}
	sort.Strings(paths)
	executions := make([]RuntimeWorkerQueueDispatchExecution, 0, len(paths))
	for _, path := range paths {
		execution, err := readRuntimeWorkerQueueDispatchExecutionFile(path)
		if err != nil {
			return nil, err
		}
		executions = append(executions, execution)
	}
	sort.SliceStable(executions, func(i int, j int) bool {
		if executions[i].ExecutedAt != executions[j].ExecutedAt {
			return executions[i].ExecutedAt < executions[j].ExecutedAt
		}
		if executions[i].WorkerID != executions[j].WorkerID {
			return executions[i].WorkerID < executions[j].WorkerID
		}
		return executions[i].Path < executions[j].Path
	})
	return executions, nil
}

func ReadRuntimeWorkerQueueDispatchExecution(root, executionPath string) (RuntimeWorkerQueueDispatchExecution, error) {
	path, err := resolveRuntimeWorkerQueueDispatchExecutionPath(root, executionPath)
	if err != nil {
		return RuntimeWorkerQueueDispatchExecution{}, err
	}
	return readRuntimeWorkerQueueDispatchExecutionFile(path)
}

func ListRuntimeWorkerTransportDispatchRequests(root, workerID string) ([]RuntimeWorkerTransportDispatchRequest, error) {
	var paths []string
	var err error
	if strings.TrimSpace(workerID) == "" {
		paths, err = filepath.Glob(filepath.Join(runtimeWorkerTransportOutboxDir(root), "*", "*.json"))
	} else {
		paths, err = filepath.Glob(filepath.Join(runtimeWorkerTransportWorkerOutboxDir(root, workerID), "*.json"))
	}
	if err != nil {
		return nil, fmt.Errorf("glob worker transport dispatch requests: %w", err)
	}
	sort.Strings(paths)
	requests := make([]RuntimeWorkerTransportDispatchRequest, 0, len(paths))
	for _, path := range paths {
		request, err := readRuntimeWorkerTransportDispatchRequestFile(path)
		if err != nil {
			return nil, err
		}
		requests = append(requests, request)
	}
	sort.SliceStable(requests, func(i int, j int) bool {
		if requests[i].CreatedAt != requests[j].CreatedAt {
			return requests[i].CreatedAt < requests[j].CreatedAt
		}
		if requests[i].WorkerID != requests[j].WorkerID {
			return requests[i].WorkerID < requests[j].WorkerID
		}
		return requests[i].Path < requests[j].Path
	})
	return requests, nil
}

func ReadRuntimeWorkerTransportDispatchRequest(root, workerID, requestPath string) (RuntimeWorkerTransportDispatchRequest, error) {
	path, err := resolveRuntimeWorkerTransportDispatchRequestPath(root, workerID, requestPath)
	if err != nil {
		return RuntimeWorkerTransportDispatchRequest{}, err
	}
	request, err := readRuntimeWorkerTransportDispatchRequestFile(path)
	if err != nil {
		return RuntimeWorkerTransportDispatchRequest{}, err
	}
	if strings.TrimSpace(request.WorkerID) != strings.TrimSpace(workerID) {
		return RuntimeWorkerTransportDispatchRequest{}, fmt.Errorf("worker transport request worker mismatch: %s", request.WorkerID)
	}
	return request, nil
}

func AcknowledgeRuntimeWorkerTransportDispatchRequest(root, workerID, requestPath string, options RuntimeWorkerTransportAckOptions) (RuntimeWorkerTransportDispatchRequest, error) {
	if !options.Ack {
		return RuntimeWorkerTransportDispatchRequest{}, fmt.Errorf("worker transport request ack requires explicit ack")
	}
	request, err := ReadRuntimeWorkerTransportDispatchRequest(root, workerID, requestPath)
	if err != nil {
		return RuntimeWorkerTransportDispatchRequest{}, err
	}
	if strings.TrimSpace(request.TransportMode) != "file-outbox" {
		return RuntimeWorkerTransportDispatchRequest{}, fmt.Errorf("worker transport request must be file-outbox before ack")
	}
	if strings.TrimSpace(request.Status) != "pending" {
		return RuntimeWorkerTransportDispatchRequest{}, fmt.Errorf("worker transport request must be pending before ack")
	}
	now := options.Now.UTC()
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if err := requireRuntimeWorkerSideEffectSafetyAuthorization(root, now); err != nil {
		return RuntimeWorkerTransportDispatchRequest{}, err
	}
	request.Status = "acknowledged"
	request.AckedAt = now.Format(time.RFC3339Nano)
	request.AckedBy = strings.TrimSpace(workerID)
	if err := writeRuntimeWorkerTransportDispatchRequestArtifact(&request); err != nil {
		return RuntimeWorkerTransportDispatchRequest{}, err
	}
	return request, nil
}

func ExecuteRuntimeWorkerTransportDispatchRequest(ctx context.Context, root, workerID, requestPath string, options RuntimeWorkerTransportRunOptions) (RuntimeWorkerQueueDispatchResult, error) {
	if !options.Execute {
		return RuntimeWorkerQueueDispatchResult{}, fmt.Errorf("worker transport request execution requires explicit execute")
	}
	request, err := ReadRuntimeWorkerTransportDispatchRequest(root, workerID, requestPath)
	if err != nil {
		return RuntimeWorkerQueueDispatchResult{}, err
	}
	if strings.TrimSpace(request.TransportMode) != "file-outbox" {
		return RuntimeWorkerQueueDispatchResult{}, fmt.Errorf("worker transport request must be file-outbox before execution")
	}
	if strings.TrimSpace(request.Status) != "acknowledged" {
		return RuntimeWorkerQueueDispatchResult{}, fmt.Errorf("worker transport request must be acknowledged before execution")
	}
	if strings.TrimSpace(request.ResultPath) != "" {
		return RuntimeWorkerQueueDispatchResult{}, fmt.Errorf("worker transport request already has a result: %s", request.ResultPath)
	}
	if strings.TrimSpace(request.DispatchExecutionPath) == "" {
		return RuntimeWorkerQueueDispatchResult{}, fmt.Errorf("worker transport request dispatch execution path is required")
	}
	if _, err := runtimeWorkerTransportCommandForRequest(request, options); err != nil {
		return RuntimeWorkerQueueDispatchResult{}, err
	}
	now := options.Now.UTC()
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if err := requireRuntimeWorkerSideEffectSafetyAuthorization(root, now); err != nil {
		return RuntimeWorkerQueueDispatchResult{}, err
	}
	if options.HeartbeatLease {
		if _, err := heartbeatWorkerTransportLease(root, workerID, request, now, options.HeartbeatBy, options.HeartbeatReason); err != nil {
			return RuntimeWorkerQueueDispatchResult{}, fmt.Errorf("refresh worker lease heartbeat before transport execution: %w", err)
		}
	}
	output, runErr := runWorkerTransportCommand(ctx, root, request, options)
	resultNow := now
	if options.Now.IsZero() {
		resultNow = time.Now().UTC()
	}
	if options.HeartbeatLease {
		if _, err := heartbeatWorkerTransportLease(root, workerID, request, resultNow, options.HeartbeatBy, options.HeartbeatReason); err != nil && runErr == nil {
			runErr = fmt.Errorf("refresh worker lease heartbeat after transport execution: %w", err)
		}
	}
	status := "completed"
	resultError := ""
	if runErr != nil {
		status = "failed"
		resultError = runErr.Error()
	}
	result, submitErr := SubmitRuntimeWorkerQueueDispatchResult(root, request.DispatchExecutionPath, RuntimeWorkerQueueDispatchResult{
		Status: status,
		Output: strings.TrimSpace(output),
		Error:  resultError,
	}, RuntimeWorkerQueueDispatchResultOptions{Now: resultNow})
	if submitErr != nil {
		return result, submitErr
	}
	if runErr != nil {
		return result, fmt.Errorf("execute worker transport command: %w", runErr)
	}
	return result, nil
}

func ExecuteRuntimeWorkerTransportDispatchLoop(ctx context.Context, root, workerID string, options RuntimeWorkerTransportRunLoopOptions) (RuntimeWorkerTransportRunLoopResult, error) {
	if !options.Execute {
		return RuntimeWorkerTransportRunLoopResult{}, fmt.Errorf("worker transport dispatch loop requires explicit execute")
	}
	if !options.UseRequestCommand && len(trimRuntimeWorkerToolPlanStrings(options.Command)) == 0 {
		return RuntimeWorkerTransportRunLoopResult{}, fmt.Errorf("worker transport dispatch loop command is required")
	}
	limit := options.MaxRequests
	if limit <= 0 {
		limit = 1
	}
	requests, err := ListRuntimeWorkerTransportDispatchRequests(root, workerID)
	if err != nil {
		return RuntimeWorkerTransportRunLoopResult{}, err
	}
	loop := RuntimeWorkerTransportRunLoopResult{
		Status:   "idle",
		WorkerID: strings.TrimSpace(workerID),
	}
	var firstErr error
	for _, request := range requests {
		if loop.Processed >= limit {
			break
		}
		if reason := runtimeWorkerTransportLoopSkipReason(request); reason != "" {
			loop.Skipped++
			loop.SkippedRequests = append(loop.SkippedRequests, RuntimeWorkerTransportRunLoopSkip{
				RequestPath: request.Path,
				WorkerID:    request.WorkerID,
				ExecutionID: request.ExecutionID,
				Status:      request.Status,
				Reason:      reason,
			})
			continue
		}
		result, err := ExecuteRuntimeWorkerTransportDispatchRequest(ctx, root, workerID, filepath.Base(request.Path), RuntimeWorkerTransportRunOptions{
			Execute:           true,
			Now:               options.Now,
			Command:           append([]string(nil), options.Command...),
			Timeout:           options.Timeout,
			UseRequestCommand: options.UseRequestCommand,
			HeartbeatLease:    options.HeartbeatLease,
			HeartbeatBy:       options.HeartbeatBy,
			HeartbeatReason:   options.HeartbeatReason,
		})
		if strings.TrimSpace(result.Status) != "" || strings.TrimSpace(result.Path) != "" {
			loop.Processed++
			loop.Results = append(loop.Results, result)
			switch strings.TrimSpace(result.Status) {
			case "completed":
				loop.Completed++
			case "failed":
				loop.Failed++
			}
		}
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			if strings.TrimSpace(result.Status) == "" && strings.TrimSpace(result.Path) == "" {
				break
			}
		}
	}
	if loop.Processed > 0 {
		loop.Status = "completed"
		if loop.Failed > 0 {
			loop.Status = "failed"
		}
	}
	return loop, firstErr
}

func ExecuteRuntimeWorkerTransportDispatchDaemon(ctx context.Context, root, workerID string, options RuntimeWorkerTransportRunDaemonOptions) (RuntimeWorkerTransportRunDaemonResult, error) {
	if !options.Execute {
		return RuntimeWorkerTransportRunDaemonResult{}, fmt.Errorf("worker transport dispatch daemon requires explicit execute")
	}
	if !options.UseRequestCommand && len(trimRuntimeWorkerToolPlanStrings(options.Command)) == 0 {
		return RuntimeWorkerTransportRunDaemonResult{}, fmt.Errorf("worker transport dispatch daemon command is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	maxRequests := options.MaxRequests
	if maxRequests <= 0 {
		maxRequests = 1
	}
	maxIdleScans := options.MaxIdleScans
	if maxIdleScans <= 0 {
		maxIdleScans = 1
	}
	pollInterval := options.PollInterval
	if pollInterval <= 0 {
		pollInterval = time.Second
	}
	startedAt := options.Now.UTC()
	if startedAt.IsZero() {
		startedAt = time.Now().UTC()
	}
	commandSource := "operator-command"
	if options.UseRequestCommand {
		commandSource = "request-command"
	}
	daemon := RuntimeWorkerTransportRunDaemonResult{
		Version:           1,
		Status:            "idle",
		Mode:              "daemon-run",
		WorkerID:          strings.TrimSpace(workerID),
		StartedAt:         startedAt.Format(time.RFC3339Nano),
		CommandSource:     commandSource,
		MaxRequests:       maxRequests,
		MaxIdleScans:      maxIdleScans,
		PollIntervalMS:    int(pollInterval / time.Millisecond),
		UseRequestCommand: options.UseRequestCommand,
	}
	var appliedControl RuntimeWorkerTransportDaemonControlRequest
	finish := func(runErr error) (RuntimeWorkerTransportRunDaemonResult, error) {
		completedAt := options.Now.UTC()
		if completedAt.IsZero() {
			completedAt = time.Now().UTC()
		}
		daemon.CompletedAt = completedAt.Format(time.RFC3339Nano)
		if runErr != nil {
			if strings.TrimSpace(daemon.Status) == "" || daemon.Status == "idle" {
				daemon.Status = "failed"
			}
			daemon.Error = strings.TrimSpace(runErr.Error())
		}
		if err := writeRuntimeWorkerTransportDaemonRun(root, &daemon); err != nil {
			if runErr != nil {
				return daemon, fmt.Errorf("%v; write worker transport daemon run artifact: %w", runErr, err)
			}
			return daemon, err
		}
		if strings.TrimSpace(appliedControl.Path) != "" && strings.TrimSpace(appliedControl.Status) == "pending" {
			appliedControl.Status = "applied"
			appliedControl.AppliedAt = daemon.CompletedAt
			appliedControl.AppliedDaemonRunPath = daemon.Path
			if err := writeRuntimeWorkerTransportDaemonControlRequest(root, &appliedControl); err != nil {
				if runErr != nil {
					return daemon, fmt.Errorf("%v; apply worker transport daemon control request: %w", runErr, err)
				}
				return daemon, err
			}
		}
		return daemon, runErr
	}
	applyPendingControl := func() (bool, bool, error) {
		if !options.EnableControlRequests {
			return false, false, nil
		}
		control, ok, err := nextPendingRuntimeWorkerTransportDaemonControlRequest(root, workerID)
		if err != nil || !ok {
			return false, false, err
		}
		controlNow := options.Now.UTC()
		if controlNow.IsZero() {
			controlNow = time.Now().UTC()
		}
		if err := requireRuntimeWorkerSideEffectSafetyAuthorization(root, controlNow); err != nil {
			return false, true, err
		}
		daemon.ControlRequestPath = control.Path
		daemon.ControlAction = control.Action
		daemon.ControlRequestedBy = control.RequestedBy
		daemon.StopReason = runtimeWorkerTransportDaemonControlStopReason(control)
		appliedControl = control
		switch control.Action {
		case "stop":
			daemon.Status = "stopped"
		case "restart":
			daemon.Status = "restart-requested"
		default:
			daemon.Status = "failed"
			return true, false, fmt.Errorf("unsupported worker transport daemon control action: %s", control.Action)
		}
		return true, false, nil
	}
	idleStreak := 0
	for daemon.Processed < maxRequests && idleStreak < maxIdleScans {
		controlled, blockedBeforeApply, err := applyPendingControl()
		if blockedBeforeApply {
			return RuntimeWorkerTransportRunDaemonResult{}, err
		}
		if err != nil || controlled {
			return finish(err)
		}
		if err := ctx.Err(); err != nil {
			return finish(err)
		}
		remaining := maxRequests - daemon.Processed
		daemon.Scans++
		loop, err := ExecuteRuntimeWorkerTransportDispatchLoop(ctx, root, workerID, RuntimeWorkerTransportRunLoopOptions{
			Execute:           true,
			Now:               options.Now,
			Command:           append([]string(nil), options.Command...),
			Timeout:           options.Timeout,
			MaxRequests:       remaining,
			UseRequestCommand: options.UseRequestCommand,
			HeartbeatLease:    options.HeartbeatLease,
			HeartbeatBy:       options.HeartbeatBy,
			HeartbeatReason:   options.HeartbeatReason,
		})
		daemon.Processed += loop.Processed
		daemon.Completed += loop.Completed
		daemon.Failed += loop.Failed
		daemon.Skipped += loop.Skipped
		daemon.Results = append(daemon.Results, loop.Results...)
		for _, result := range loop.Results {
			if strings.TrimSpace(result.Path) != "" {
				daemon.ResultPaths = append(daemon.ResultPaths, result.Path)
			}
		}
		daemon.SkippedRequests = append(daemon.SkippedRequests, loop.SkippedRequests...)
		if loop.Processed > 0 {
			idleStreak = 0
		} else {
			idleStreak++
			daemon.IdleScans++
		}
		if daemon.Processed > 0 {
			daemon.Status = "completed"
			if daemon.Failed > 0 {
				daemon.Status = "failed"
			}
		}
		if err != nil {
			if daemon.Status == "" {
				daemon.Status = "failed"
			}
			return finish(err)
		}
		if daemon.Processed >= maxRequests || idleStreak >= maxIdleScans {
			break
		}
		controlled, blockedBeforeApply, err = applyPendingControl()
		if blockedBeforeApply {
			return RuntimeWorkerTransportRunDaemonResult{}, err
		}
		if err != nil || controlled {
			return finish(err)
		}
		timer := time.NewTimer(pollInterval)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			return finish(ctx.Err())
		case <-timer.C:
		}
	}
	return finish(nil)
}

func ExecuteRuntimeWorkerTransportDaemonSupervisor(ctx context.Context, root, workerID string, options RuntimeWorkerTransportDaemonSupervisorOptions) (RuntimeWorkerTransportDaemonSupervisorRun, error) {
	if !options.Execute {
		return RuntimeWorkerTransportDaemonSupervisorRun{}, fmt.Errorf("worker transport daemon supervisor requires explicit execute")
	}
	workerID = strings.TrimSpace(workerID)
	if workerID == "" {
		return RuntimeWorkerTransportDaemonSupervisorRun{}, fmt.Errorf("worker transport daemon supervisor worker id is required")
	}
	if !options.UseRequestCommand && len(trimRuntimeWorkerToolPlanStrings(options.Command)) == 0 {
		return RuntimeWorkerTransportDaemonSupervisorRun{}, fmt.Errorf("worker transport daemon supervisor command is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	maxRuns := options.MaxRuns
	if maxRuns <= 0 {
		maxRuns = 1
	}
	maxRequests := options.MaxRequests
	if maxRequests <= 0 {
		maxRequests = 1
	}
	maxIdleScans := options.MaxIdleScans
	if maxIdleScans <= 0 {
		maxIdleScans = 1
	}
	pollInterval := options.PollInterval
	if pollInterval <= 0 {
		pollInterval = time.Second
	}
	startedAt := options.Now.UTC()
	if startedAt.IsZero() {
		startedAt = time.Now().UTC()
	}
	commandSource := "operator-command"
	if options.UseRequestCommand {
		commandSource = "request-command"
	}
	supervisor := RuntimeWorkerTransportDaemonSupervisorRun{
		Version:               1,
		Status:                "idle",
		Mode:                  "daemon-supervisor-run",
		WorkerID:              workerID,
		StartedAt:             startedAt.Format(time.RFC3339Nano),
		CommandSource:         commandSource,
		MaxRuns:               maxRuns,
		MaxRequests:           maxRequests,
		MaxIdleScans:          maxIdleScans,
		PollIntervalMS:        int(pollInterval / time.Millisecond),
		UseRequestCommand:     options.UseRequestCommand,
		EnableControlRequests: options.EnableControlRequests,
	}
	finish := func(runErr error) (RuntimeWorkerTransportDaemonSupervisorRun, error) {
		completedAt := options.Now.UTC()
		if completedAt.IsZero() {
			completedAt = time.Now().UTC()
		}
		supervisor.CompletedAt = completedAt.Format(time.RFC3339Nano)
		if runErr != nil {
			if strings.TrimSpace(supervisor.Status) == "" || supervisor.Status == "idle" {
				supervisor.Status = "failed"
			}
			if strings.TrimSpace(supervisor.Error) == "" {
				supervisor.Error = strings.TrimSpace(runErr.Error())
			}
		}
		if err := writeRuntimeWorkerTransportDaemonSupervisorRun(root, &supervisor); err != nil {
			if runErr != nil {
				return supervisor, fmt.Errorf("%v; write worker transport daemon supervisor run artifact: %w", runErr, err)
			}
			return supervisor, err
		}
		return supervisor, runErr
	}
	for supervisor.Runs < maxRuns {
		if err := ctx.Err(); err != nil {
			return finish(err)
		}
		runNow := startedAt.Add(time.Duration(supervisor.Runs) * time.Nanosecond)
		daemon, err := ExecuteRuntimeWorkerTransportDispatchDaemon(ctx, root, workerID, RuntimeWorkerTransportRunDaemonOptions{
			Execute:               true,
			Now:                   runNow,
			Command:               append([]string(nil), options.Command...),
			Timeout:               options.Timeout,
			MaxRequests:           maxRequests,
			MaxIdleScans:          maxIdleScans,
			PollInterval:          pollInterval,
			UseRequestCommand:     options.UseRequestCommand,
			EnableControlRequests: options.EnableControlRequests,
			HeartbeatLease:        options.HeartbeatLease,
			HeartbeatBy:           options.HeartbeatBy,
			HeartbeatReason:       options.HeartbeatReason,
		})
		if err != nil && strings.TrimSpace(daemon.Status) == "" && strings.TrimSpace(daemon.Path) == "" {
			return RuntimeWorkerTransportDaemonSupervisorRun{}, err
		}
		supervisor.Runs++
		supervisor.LastDaemonStatus = daemon.Status
		supervisor.Processed += daemon.Processed
		supervisor.Completed += daemon.Completed
		supervisor.Failed += daemon.Failed
		supervisor.Skipped += daemon.Skipped
		if strings.TrimSpace(daemon.Path) != "" {
			supervisor.DaemonRunPaths = append(supervisor.DaemonRunPaths, daemon.Path)
		}
		if strings.TrimSpace(daemon.Status) != "" || strings.TrimSpace(daemon.Path) != "" {
			supervisor.DaemonRuns = append(supervisor.DaemonRuns, daemon)
		}
		if err != nil {
			return finish(err)
		}
		switch daemon.Status {
		case "restart-requested":
			supervisor.RestartCount++
			if supervisor.Runs >= maxRuns {
				supervisor.Status = "restart-limit"
				supervisor.Error = fmt.Sprintf("restart requested after max daemon supervisor runs reached: %d", maxRuns)
				return finish(nil)
			}
			continue
		case "stopped":
			supervisor.Status = "stopped"
			return finish(nil)
		case "failed":
			supervisor.Status = "failed"
			return finish(nil)
		default:
			if supervisor.Processed > 0 {
				supervisor.Status = "completed"
				if supervisor.Failed > 0 {
					supervisor.Status = "failed"
				}
			} else if strings.TrimSpace(daemon.Status) != "" {
				supervisor.Status = daemon.Status
			}
			return finish(nil)
		}
	}
	return finish(nil)
}

func ListRuntimeWorkerTransportDaemonSupervisorRuns(root, workerID string) ([]RuntimeWorkerTransportDaemonSupervisorRun, error) {
	if strings.TrimSpace(workerID) == "" {
		return nil, fmt.Errorf("worker transport daemon supervisor worker id is required")
	}
	paths, err := filepath.Glob(filepath.Join(runtimeWorkerTransportDaemonSupervisorRunWorkerDir(root, workerID), "*.json"))
	if err != nil {
		return nil, fmt.Errorf("glob worker transport daemon supervisor runs: %w", err)
	}
	sort.Strings(paths)
	runs := make([]RuntimeWorkerTransportDaemonSupervisorRun, 0, len(paths))
	for _, path := range paths {
		run, err := readRuntimeWorkerTransportDaemonSupervisorRunFile(path)
		if err != nil {
			return nil, err
		}
		runs = append(runs, run)
	}
	sort.SliceStable(runs, func(i, j int) bool {
		if runs[i].StartedAt != runs[j].StartedAt {
			return runs[i].StartedAt < runs[j].StartedAt
		}
		return runs[i].Path < runs[j].Path
	})
	return runs, nil
}

func ReadRuntimeWorkerTransportDaemonSupervisorRun(root, workerID, runPath string) (RuntimeWorkerTransportDaemonSupervisorRun, error) {
	path, err := resolveRuntimeWorkerTransportDaemonSupervisorRunPath(root, workerID, runPath)
	if err != nil {
		return RuntimeWorkerTransportDaemonSupervisorRun{}, err
	}
	return readRuntimeWorkerTransportDaemonSupervisorRunFile(path)
}

func ListRuntimeWorkerTransportDaemonRuns(root, workerID string) ([]RuntimeWorkerTransportRunDaemonResult, error) {
	if strings.TrimSpace(workerID) == "" {
		return nil, fmt.Errorf("worker transport daemon run worker id is required")
	}
	paths, err := filepath.Glob(filepath.Join(runtimeWorkerTransportDaemonRunWorkerDir(root, workerID), "*.json"))
	if err != nil {
		return nil, fmt.Errorf("glob worker transport daemon runs: %w", err)
	}
	sort.Strings(paths)
	runs := make([]RuntimeWorkerTransportRunDaemonResult, 0, len(paths))
	for _, path := range paths {
		run, err := readRuntimeWorkerTransportDaemonRunFile(path)
		if err != nil {
			return nil, err
		}
		runs = append(runs, run)
	}
	sort.SliceStable(runs, func(i, j int) bool {
		if runs[i].StartedAt != runs[j].StartedAt {
			return runs[i].StartedAt < runs[j].StartedAt
		}
		return runs[i].Path < runs[j].Path
	})
	return runs, nil
}

func ReadRuntimeWorkerTransportDaemonRun(root, workerID, runPath string) (RuntimeWorkerTransportRunDaemonResult, error) {
	path, err := resolveRuntimeWorkerTransportDaemonRunPath(root, workerID, runPath)
	if err != nil {
		return RuntimeWorkerTransportRunDaemonResult{}, err
	}
	return readRuntimeWorkerTransportDaemonRunFile(path)
}

func CreateRuntimeWorkerTransportDaemonControlRequest(root, workerID string, request RuntimeWorkerTransportDaemonControlRequest, options RuntimeWorkerTransportDaemonControlRequestOptions) (RuntimeWorkerTransportDaemonControlRequest, error) {
	workerID = strings.TrimSpace(workerID)
	if workerID == "" {
		return RuntimeWorkerTransportDaemonControlRequest{}, fmt.Errorf("worker transport daemon control worker id is required")
	}
	action, err := normalizeRuntimeWorkerTransportDaemonControlAction(request.Action)
	if err != nil {
		return RuntimeWorkerTransportDaemonControlRequest{}, err
	}
	now := options.Now.UTC()
	if now.IsZero() {
		now = time.Now().UTC()
	}
	control := RuntimeWorkerTransportDaemonControlRequest{
		Version:     1,
		Status:      "pending",
		Mode:        "metadata-only",
		WorkerID:    workerID,
		Action:      action,
		Reason:      strings.TrimSpace(request.Reason),
		RequestedBy: strings.TrimSpace(request.RequestedBy),
		RequestedAt: now.Format(time.RFC3339Nano),
	}
	if err := writeRuntimeWorkerTransportDaemonControlRequest(root, &control); err != nil {
		return RuntimeWorkerTransportDaemonControlRequest{}, err
	}
	return control, nil
}

func ListRuntimeWorkerTransportDaemonControlRequests(root, workerID string) ([]RuntimeWorkerTransportDaemonControlRequest, error) {
	if strings.TrimSpace(workerID) == "" {
		return nil, fmt.Errorf("worker transport daemon control worker id is required")
	}
	paths, err := filepath.Glob(filepath.Join(runtimeWorkerTransportDaemonControlWorkerDir(root, workerID), "*.json"))
	if err != nil {
		return nil, fmt.Errorf("glob worker transport daemon controls: %w", err)
	}
	return readRuntimeWorkerTransportDaemonControlRequestFiles(paths)
}

func ReadRuntimeWorkerTransportDaemonControlRequest(root, workerID, controlPath string) (RuntimeWorkerTransportDaemonControlRequest, error) {
	path, err := resolveRuntimeWorkerTransportDaemonControlPath(root, workerID, controlPath)
	if err != nil {
		return RuntimeWorkerTransportDaemonControlRequest{}, err
	}
	return readRuntimeWorkerTransportDaemonControlRequestFile(path)
}

func nextPendingRuntimeWorkerTransportDaemonControlRequest(root, workerID string) (RuntimeWorkerTransportDaemonControlRequest, bool, error) {
	controls, err := ListRuntimeWorkerTransportDaemonControlRequests(root, workerID)
	if err != nil {
		return RuntimeWorkerTransportDaemonControlRequest{}, false, err
	}
	for _, control := range controls {
		if strings.TrimSpace(control.Status) == "pending" {
			return control, true, nil
		}
	}
	return RuntimeWorkerTransportDaemonControlRequest{}, false, nil
}

func runtimeWorkerTransportDaemonControlStopReason(control RuntimeWorkerTransportDaemonControlRequest) string {
	reason := strings.TrimSpace(control.Reason)
	switch strings.TrimSpace(control.Action) {
	case "restart":
		if reason == "" {
			return "restart requested"
		}
		return "restart requested: " + reason
	default:
		if reason == "" {
			return "stop requested"
		}
		return reason
	}
}

func runtimeWorkerTransportLoopSkipReason(request RuntimeWorkerTransportDispatchRequest) string {
	if strings.TrimSpace(request.TransportMode) != "file-outbox" {
		return "not-file-outbox"
	}
	if strings.TrimSpace(request.ResultPath) != "" {
		return "already-result"
	}
	if strings.TrimSpace(request.Status) != "acknowledged" {
		return "not-acknowledged"
	}
	return ""
}

func heartbeatWorkerTransportLease(root, workerID string, request RuntimeWorkerTransportDispatchRequest, now time.Time, heartbeatBy, heartbeatReason string) (WorkerLeaseArtifact, error) {
	by := strings.TrimSpace(heartbeatBy)
	if by == "" {
		by = strings.TrimSpace(workerID)
	}
	reason := strings.TrimSpace(heartbeatReason)
	if reason == "" {
		reason = "worker transport request execution"
	}
	return UpdateWorkerLeaseHeartbeat(root, request.RunID, request.ExecutionID, WorkerLeaseHeartbeatOptions{
		Heartbeat:   true,
		Now:         now,
		HeartbeatBy: by,
		Reason:      reason,
	})
}

func SubmitRuntimeWorkerQueueDispatchResult(root, executionPath string, result RuntimeWorkerQueueDispatchResult, options RuntimeWorkerQueueDispatchResultOptions) (RuntimeWorkerQueueDispatchResult, error) {
	status, err := normalizeRuntimeWorkerQueueDispatchResultStatus(result.Status)
	if err != nil {
		return RuntimeWorkerQueueDispatchResult{}, err
	}
	execution, err := ReadRuntimeWorkerQueueDispatchExecution(root, executionPath)
	if err != nil {
		return RuntimeWorkerQueueDispatchResult{}, err
	}
	if strings.TrimSpace(execution.ResultPath) != "" {
		return RuntimeWorkerQueueDispatchResult{}, fmt.Errorf("worker queue dispatch execution already has a result: %s", execution.ResultPath)
	}
	if strings.TrimSpace(execution.Status) != "dispatched" {
		return RuntimeWorkerQueueDispatchResult{}, fmt.Errorf("worker queue dispatch execution must be dispatched before result submission")
	}
	now := options.Now.UTC()
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if err := requireRuntimeWorkerSideEffectSafetyAuthorization(root, now); err != nil {
		return RuntimeWorkerQueueDispatchResult{}, err
	}
	result = runtimeWorkerQueueDispatchResultFromExecution(execution, status, result.Output, result.Error, now)
	if err := writeRuntimeWorkerQueueDispatchResult(root, &result); err != nil {
		return RuntimeWorkerQueueDispatchResult{}, err
	}
	execution.Status = status
	execution.ResultPath = result.Path
	execution.CompletedAt = result.SubmittedAt
	if status == "failed" {
		execution.Error = strings.TrimSpace(result.Error)
	}
	if err := writeRuntimeWorkerQueueDispatchExecution(root, &execution); err != nil {
		return result, fmt.Errorf("update worker queue dispatch execution result metadata: %w", err)
	}
	claim, err := ReadRuntimeWorkerQueueClaim(root, execution.ClaimPath)
	if err != nil {
		return result, fmt.Errorf("read worker queue claim for result: %w", err)
	}
	claim.Status = status
	claim.ResultStatus = status
	claim.ResultPath = result.Path
	claim.CompletedAt = result.SubmittedAt
	if err := writeRuntimeWorkerQueueClaimArtifact(claim); err != nil {
		return result, fmt.Errorf("update worker queue claim result metadata: %w", err)
	}
	if strings.TrimSpace(execution.TransportMode) == "file-outbox" && strings.TrimSpace(execution.TransportRequestPath) != "" {
		request, err := ReadRuntimeWorkerTransportDispatchRequest(root, execution.WorkerID, execution.TransportRequestPath)
		if err != nil {
			return result, fmt.Errorf("read worker transport request for result: %w", err)
		}
		request.Status = status
		request.ResultStatus = status
		request.ResultPath = result.Path
		request.CompletedAt = result.SubmittedAt
		if err := writeRuntimeWorkerTransportDispatchRequestArtifact(&request); err != nil {
			return result, fmt.Errorf("update worker transport request result metadata: %w", err)
		}
	}
	return result, nil
}

func runWorkerTransportCommand(ctx context.Context, root string, request RuntimeWorkerTransportDispatchRequest, options RuntimeWorkerTransportRunOptions) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	timeout := runtimeWorkerTransportCommandTimeout(request, options)
	commandCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	workDir := runtimeWorkerTransportRunDir(root, request)
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		return "", fmt.Errorf("create worker transport run directory: %w", err)
	}
	command, err := runtimeWorkerTransportCommandForRequest(request, options)
	if err != nil {
		return "", err
	}
	cmd := exec.CommandContext(commandCtx, command[0], command[1:]...)
	cmd.Dir = workDir
	cmd.Env = workerTransportCommandEnv(root, workDir, request)
	output, err := cmd.CombinedOutput()
	if commandCtx.Err() == context.DeadlineExceeded {
		return string(output), fmt.Errorf("worker transport command timed out")
	}
	return string(output), err
}

func runtimeWorkerTransportCommandForRequest(request RuntimeWorkerTransportDispatchRequest, options RuntimeWorkerTransportRunOptions) ([]string, error) {
	if options.UseRequestCommand {
		command := trimRuntimeWorkerToolPlanStrings(request.ToolPlan.RunCommand)
		if len(command) == 0 {
			return nil, fmt.Errorf("request tool plan run command is required")
		}
		return command, nil
	}
	command := trimRuntimeWorkerToolPlanStrings(options.Command)
	if len(command) == 0 {
		return nil, fmt.Errorf("worker transport request command is required")
	}
	return command, nil
}

func runtimeWorkerTransportCommandTimeout(request RuntimeWorkerTransportDispatchRequest, options RuntimeWorkerTransportRunOptions) time.Duration {
	if options.Timeout > 0 {
		return options.Timeout
	}
	if options.UseRequestCommand && request.ToolPlan.RunTimeoutMS > 0 {
		return time.Duration(request.ToolPlan.RunTimeoutMS) * time.Millisecond
	}
	return 5 * time.Minute
}

func workerTransportCommandEnv(root, workDir string, request RuntimeWorkerTransportDispatchRequest) []string {
	return []string{
		"PATH=" + os.Getenv("PATH"),
		"OURO_WORKSPACE=" + root,
		"OURO_WORKER_RUN_DIR=" + workDir,
		"OURO_WORKER_ID=" + request.WorkerID,
		"OURO_TRANSPORT_REQUEST=" + request.Path,
		"OURO_QUEUE_DISPATCH_EXECUTION=" + request.DispatchExecutionPath,
		"OURO_QUEUE_CLAIM=" + request.ClaimPath,
		"OURO_RUN_ID=" + request.RunID,
		"OURO_EXECUTION_ID=" + request.ExecutionID,
		"OURO_TOOL=" + request.Tool,
		"OURO_CAPABILITY=" + request.Capability,
		"OURO_ACTION=" + request.ToolPlan.Action,
		"OURO_MANIFEST_NAME=" + request.ToolPlan.ManifestName,
		"OURO_RUNNER=" + request.ToolPlan.Runner,
		"OURO_PARSER=" + request.ToolPlan.Parser,
		"OURO_RUN_TIMEOUT_MS=" + fmt.Sprint(request.ToolPlan.RunTimeoutMS),
		"OURO_TOOL_PLAN_JSON=" + runtimeWorkerTransportJSONEnvValue(request.ToolPlan),
		"OURO_RUN_COMMAND_JSON=" + runtimeWorkerTransportJSONEnvValue(request.ToolPlan.RunCommand),
		"OURO_INPUT_NODE_IDS_JSON=" + runtimeWorkerTransportJSONEnvValue(request.ToolPlan.InputNodeIDs),
		"OURO_EXPECTED_EVIDENCE_JSON=" + runtimeWorkerTransportJSONEnvValue(request.ToolPlan.ExpectedEvidence),
	}
}

func runtimeWorkerTransportJSONEnvValue(value any) string {
	data, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	return string(data)
}

func ListRuntimeWorkerQueueDispatchResults(root string) ([]RuntimeWorkerQueueDispatchResult, error) {
	paths, err := filepath.Glob(filepath.Join(runtimeWorkerQueueDispatchResultDir(root), "*.json"))
	if err != nil {
		return nil, fmt.Errorf("glob worker queue dispatch results: %w", err)
	}
	sort.Strings(paths)
	results := make([]RuntimeWorkerQueueDispatchResult, 0, len(paths))
	for _, path := range paths {
		result, err := readRuntimeWorkerQueueDispatchResultFile(path)
		if err != nil {
			return nil, err
		}
		results = append(results, result)
	}
	sort.SliceStable(results, func(i int, j int) bool {
		if results[i].SubmittedAt != results[j].SubmittedAt {
			return results[i].SubmittedAt < results[j].SubmittedAt
		}
		if results[i].WorkerID != results[j].WorkerID {
			return results[i].WorkerID < results[j].WorkerID
		}
		return results[i].Path < results[j].Path
	})
	return results, nil
}

func ReadRuntimeWorkerQueueDispatchResult(root, resultPath string) (RuntimeWorkerQueueDispatchResult, error) {
	path, err := resolveRuntimeWorkerQueueDispatchResultPath(root, resultPath)
	if err != nil {
		return RuntimeWorkerQueueDispatchResult{}, err
	}
	return readRuntimeWorkerQueueDispatchResultFile(path)
}

func runtimeWorkerQueueDispatchExecutionFromClaim(claim RuntimeWorkerQueueClaim, now time.Time) RuntimeWorkerQueueDispatchExecution {
	return RuntimeWorkerQueueDispatchExecution{
		Version:        1,
		Mode:           "execute",
		TransportMode:  "metadata-only",
		ClaimPath:      claim.Path,
		ClaimID:        claim.ClaimID,
		WorkerID:       claim.WorkerID,
		WorkerMode:     claim.WorkerMode,
		WorkerEndpoint: claim.WorkerEndpoint,
		RunID:          claim.RunID,
		ExecutionID:    claim.ExecutionID,
		Tool:           claim.Tool,
		Capability:     claim.Capability,
		ToolPlan:       normalizeRuntimeWorkerToolPlanPayload(claim.ToolPlan, claim.Tool),
		ExecutedAt:     now.Format(time.RFC3339Nano),
	}
}

func runtimeWorkerQueueDispatchResultFromExecution(execution RuntimeWorkerQueueDispatchExecution, status, output, resultError string, now time.Time) RuntimeWorkerQueueDispatchResult {
	return RuntimeWorkerQueueDispatchResult{
		Version:               1,
		Status:                status,
		Mode:                  "worker-result",
		DispatchExecutionPath: execution.Path,
		ClaimPath:             execution.ClaimPath,
		ClaimID:               execution.ClaimID,
		WorkerID:              execution.WorkerID,
		WorkerMode:            execution.WorkerMode,
		WorkerEndpoint:        execution.WorkerEndpoint,
		RunID:                 execution.RunID,
		ExecutionID:           execution.ExecutionID,
		Tool:                  execution.Tool,
		Capability:            execution.Capability,
		ToolPlan:              normalizeRuntimeWorkerToolPlanPayload(execution.ToolPlan, execution.Tool),
		SubmittedAt:           now.Format(time.RFC3339Nano),
		Output:                strings.TrimSpace(output),
		Error:                 strings.TrimSpace(resultError),
	}
}

func runtimeWorkerQueueDispatchSkipReason(root string, claim RuntimeWorkerQueueClaim, now time.Time) string {
	if strings.TrimSpace(claim.Status) != "claimed" {
		return "claim-not-claimed"
	}
	if strings.TrimSpace(claim.Mode) != "metadata-only" {
		return "claim-not-metadata-only"
	}
	expiresAt := strings.TrimSpace(claim.ExpiresAt)
	if expiresAt == "" {
		return "claim-expiry-missing"
	}
	expiry, err := time.Parse(time.RFC3339Nano, expiresAt)
	if err != nil {
		return "claim-expiry-invalid"
	}
	if now.After(expiry) {
		return "claim-expired"
	}
	worker, err := ReadRuntimeWorker(root, claim.WorkerID)
	if err != nil {
		return "worker-missing"
	}
	if strings.TrimSpace(worker.Status) == "offline" {
		return "worker-offline"
	}
	if runtimeWorkerIsStale(worker, now, DefaultRuntimeWorkerStaleAfter) {
		return "worker-stale"
	}
	if !runtimeWorkerHasCapability(worker, claim.Capability) {
		return "capability-mismatch"
	}
	return ""
}

func writeRuntimeWorkerQueueDispatchExecution(root string, execution *RuntimeWorkerQueueDispatchExecution) error {
	dir := runtimeWorkerQueueDispatchExecutionDir(root)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create worker queue dispatch execution directory: %w", err)
	}
	ensureRuntimeWorkerQueueDispatchExecutionPath(root, execution)
	data, err := json.MarshalIndent(execution, "", "  ")
	if err != nil {
		return fmt.Errorf("encode worker queue dispatch execution: %w", err)
	}
	data = append(data, '\n')
	tmp := execution.Path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("write worker queue dispatch execution: %w", err)
	}
	if err := os.Rename(tmp, execution.Path); err != nil {
		return fmt.Errorf("publish worker queue dispatch execution: %w", err)
	}
	return nil
}

func writeRuntimeWorkerTransportDispatchRequest(root string, execution RuntimeWorkerQueueDispatchExecution) (RuntimeWorkerTransportDispatchRequest, error) {
	request := runtimeWorkerTransportDispatchRequestFromExecution(execution)
	request.Path = filepath.Join(runtimeWorkerTransportWorkerOutboxDir(root, execution.WorkerID), safeRuntimeWorkerTransportDispatchRequestName(request)+".json")
	if err := writeRuntimeWorkerTransportDispatchRequestArtifact(&request); err != nil {
		return RuntimeWorkerTransportDispatchRequest{}, err
	}
	return request, nil
}

func runtimeWorkerTransportDispatchRequestFromExecution(execution RuntimeWorkerQueueDispatchExecution) RuntimeWorkerTransportDispatchRequest {
	return RuntimeWorkerTransportDispatchRequest{
		Version:                1,
		Status:                 "pending",
		TransportMode:          execution.TransportMode,
		DispatchExecutionPath:  execution.Path,
		ClaimPath:              execution.ClaimPath,
		ClaimID:                execution.ClaimID,
		WorkerID:               execution.WorkerID,
		WorkerEndpoint:         execution.WorkerEndpoint,
		ResultCallbackEndpoint: execution.ResultCallbackEndpoint,
		RunID:                  execution.RunID,
		ExecutionID:            execution.ExecutionID,
		Tool:                   execution.Tool,
		Capability:             execution.Capability,
		ToolPlan:               normalizeRuntimeWorkerToolPlanPayload(execution.ToolPlan, execution.Tool),
		CreatedAt:              execution.ExecutedAt,
	}
}

type runtimeWorkerHTTPJSONDispatchResult struct {
	Endpoint     string
	StatusCode   int
	ResponseBody string
}

func postRuntimeWorkerHTTPJSONDispatch(execution RuntimeWorkerQueueDispatchExecution, authToken string) (runtimeWorkerHTTPJSONDispatchResult, error) {
	endpoint := strings.TrimSpace(execution.WorkerEndpoint)
	result := runtimeWorkerHTTPJSONDispatchResult{Endpoint: endpoint}
	if endpoint == "" {
		return result, fmt.Errorf("http-json transport worker endpoint is required")
	}
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return result, fmt.Errorf("parse http-json worker endpoint: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return result, fmt.Errorf("http-json transport requires http or https worker endpoint")
	}
	body, err := json.Marshal(runtimeWorkerTransportDispatchRequestFromExecution(execution))
	if err != nil {
		return result, fmt.Errorf("encode http-json worker dispatch request: %w", err)
	}
	request, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return result, fmt.Errorf("create http-json worker dispatch request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Ouroboros-Worker-ID", execution.WorkerID)
	request.Header.Set("X-Ouroboros-Execution-ID", execution.ExecutionID)
	if token := strings.TrimSpace(authToken); token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response, err := (&http.Client{Timeout: 10 * time.Second}).Do(request)
	if err != nil {
		return result, fmt.Errorf("post http-json worker dispatch request: %w", err)
	}
	defer response.Body.Close()
	responseBody, readErr := io.ReadAll(io.LimitReader(response.Body, 4096))
	result.StatusCode = response.StatusCode
	result.ResponseBody = strings.TrimSpace(string(responseBody))
	if readErr != nil {
		return result, fmt.Errorf("read http-json worker dispatch response: %w", readErr)
	}
	if response.StatusCode < 200 || response.StatusCode > 299 {
		return result, fmt.Errorf("http-json worker dispatch returned status %d", response.StatusCode)
	}
	return result, nil
}

func NewRuntimeWorkerQueueDispatchResultHTTPHandler(root string, options RuntimeWorkerQueueDispatchResultHTTPHandlerOptions) http.Handler {
	authToken := strings.TrimSpace(options.AuthToken)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "worker queue dispatch result receiver requires POST", http.StatusMethodNotAllowed)
			return
		}
		if authToken != "" && r.Header.Get("Authorization") != "Bearer "+authToken {
			http.Error(w, "worker queue dispatch result receiver unauthorized", http.StatusUnauthorized)
			return
		}
		defer r.Body.Close()
		var callback RuntimeWorkerQueueDispatchResult
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&callback); err != nil {
			http.Error(w, "decode worker queue dispatch result callback: "+err.Error(), http.StatusBadRequest)
			return
		}
		if strings.TrimSpace(callback.DispatchExecutionPath) == "" {
			http.Error(w, "worker queue dispatch result callback requires dispatch_execution_path", http.StatusBadRequest)
			return
		}
		result, err := SubmitRuntimeWorkerQueueDispatchResult(root, callback.DispatchExecutionPath, callback, RuntimeWorkerQueueDispatchResultOptions{Now: time.Now().UTC()})
		if err != nil {
			http.Error(w, "submit worker queue dispatch result callback: "+err.Error(), runtimeWorkerQueueDispatchResultHTTPErrorStatus(err))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(result)
	})
}

func runtimeWorkerQueueDispatchResultHTTPErrorStatus(err error) int {
	if err == nil {
		return http.StatusAccepted
	}
	message := err.Error()
	if strings.Contains(message, "already has a result") || strings.Contains(message, "must be dispatched") {
		return http.StatusConflict
	}
	return http.StatusBadRequest
}

func NewRuntimeWorkerTransportHTTPHandler(root, workerID string) http.Handler {
	return NewRuntimeWorkerTransportHTTPHandlerWithOptions(root, workerID, RuntimeWorkerTransportHTTPHandlerOptions{})
}

func NewRuntimeWorkerTransportHTTPHandlerWithOptions(root, workerID string, options RuntimeWorkerTransportHTTPHandlerOptions) http.Handler {
	authToken := strings.TrimSpace(options.AuthToken)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "worker transport receiver requires POST", http.StatusMethodNotAllowed)
			return
		}
		workerID = strings.TrimSpace(workerID)
		if workerID == "" {
			http.Error(w, "worker id is required", http.StatusBadRequest)
			return
		}
		if authToken != "" && r.Header.Get("Authorization") != "Bearer "+authToken {
			http.Error(w, "worker transport receiver unauthorized", http.StatusUnauthorized)
			return
		}
		defer r.Body.Close()
		var request RuntimeWorkerTransportDispatchRequest
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&request); err != nil {
			http.Error(w, "decode worker transport request: "+err.Error(), http.StatusBadRequest)
			return
		}
		if strings.TrimSpace(request.WorkerID) != workerID {
			http.Error(w, "worker mismatch: "+strings.TrimSpace(request.WorkerID), http.StatusBadRequest)
			return
		}
		if strings.TrimSpace(request.TransportMode) != "http-json" {
			http.Error(w, "worker transport receiver requires http-json request", http.StatusBadRequest)
			return
		}
		request.Status = "pending"
		request.TransportMode = "file-outbox"
		request.AckedAt = ""
		request.AckedBy = ""
		request.ResultStatus = ""
		request.ResultPath = ""
		request.CompletedAt = ""
		if strings.TrimSpace(request.CreatedAt) == "" {
			request.CreatedAt = time.Now().UTC().Format(time.RFC3339Nano)
		}
		request.Path = filepath.Join(runtimeWorkerTransportWorkerOutboxDir(root, workerID), safeRuntimeWorkerTransportDispatchRequestName(request)+".json")
		if err := writeRuntimeWorkerTransportDispatchRequestArtifact(&request); err != nil {
			http.Error(w, "persist worker transport request: "+err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(struct {
			Status        string `json:"status"`
			TransportMode string `json:"transport_mode"`
			Path          string `json:"path"`
			WorkerID      string `json:"worker_id"`
			ExecutionID   string `json:"execution_id"`
		}{
			Status:        request.Status,
			TransportMode: request.TransportMode,
			Path:          request.Path,
			WorkerID:      request.WorkerID,
			ExecutionID:   request.ExecutionID,
		})
	})
}

func writeRuntimeWorkerTransportDispatchRequestArtifact(request *RuntimeWorkerTransportDispatchRequest) error {
	if strings.TrimSpace(request.Path) == "" {
		return fmt.Errorf("worker transport dispatch request path is required")
	}
	if err := os.MkdirAll(filepath.Dir(request.Path), 0o755); err != nil {
		return fmt.Errorf("create worker transport outbox directory: %w", err)
	}
	data, err := json.MarshalIndent(request, "", "  ")
	if err != nil {
		return fmt.Errorf("encode worker transport dispatch request: %w", err)
	}
	data = append(data, '\n')
	tmp := request.Path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("write worker transport dispatch request: %w", err)
	}
	if err := os.Rename(tmp, request.Path); err != nil {
		return fmt.Errorf("publish worker transport dispatch request: %w", err)
	}
	return nil
}

func writeRuntimeWorkerTransportDaemonRun(root string, daemon *RuntimeWorkerTransportRunDaemonResult) error {
	if strings.TrimSpace(daemon.WorkerID) == "" {
		return fmt.Errorf("worker transport daemon run worker id is required")
	}
	dir := runtimeWorkerTransportDaemonRunWorkerDir(root, daemon.WorkerID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create worker transport daemon run directory: %w", err)
	}
	if strings.TrimSpace(daemon.Path) == "" {
		daemon.Path = filepath.Join(dir, safeRuntimeWorkerTransportDaemonRunName(*daemon)+".json")
	}
	data, err := json.MarshalIndent(daemon, "", "  ")
	if err != nil {
		return fmt.Errorf("encode worker transport daemon run: %w", err)
	}
	data = append(data, '\n')
	tmp := daemon.Path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("write worker transport daemon run: %w", err)
	}
	if err := os.Rename(tmp, daemon.Path); err != nil {
		return fmt.Errorf("publish worker transport daemon run: %w", err)
	}
	return nil
}

func writeRuntimeWorkerTransportDaemonSupervisorRun(root string, supervisor *RuntimeWorkerTransportDaemonSupervisorRun) error {
	if strings.TrimSpace(supervisor.WorkerID) == "" {
		return fmt.Errorf("worker transport daemon supervisor worker id is required")
	}
	dir := runtimeWorkerTransportDaemonSupervisorRunWorkerDir(root, supervisor.WorkerID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create worker transport daemon supervisor run directory: %w", err)
	}
	if strings.TrimSpace(supervisor.Path) == "" {
		supervisor.Path = filepath.Join(dir, safeRuntimeWorkerTransportDaemonSupervisorRunName(*supervisor)+".json")
	}
	data, err := json.MarshalIndent(supervisor, "", "  ")
	if err != nil {
		return fmt.Errorf("encode worker transport daemon supervisor run: %w", err)
	}
	data = append(data, '\n')
	tmp := supervisor.Path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("write worker transport daemon supervisor run: %w", err)
	}
	if err := os.Rename(tmp, supervisor.Path); err != nil {
		return fmt.Errorf("publish worker transport daemon supervisor run: %w", err)
	}
	return nil
}

func writeRuntimeWorkerTransportDaemonControlRequest(root string, control *RuntimeWorkerTransportDaemonControlRequest) error {
	if strings.TrimSpace(control.WorkerID) == "" {
		return fmt.Errorf("worker transport daemon control worker id is required")
	}
	dir := runtimeWorkerTransportDaemonControlWorkerDir(root, control.WorkerID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create worker transport daemon control directory: %w", err)
	}
	if strings.TrimSpace(control.Path) == "" {
		control.Path = filepath.Join(dir, safeRuntimeWorkerTransportDaemonControlRequestName(*control)+".json")
	}
	data, err := json.MarshalIndent(control, "", "  ")
	if err != nil {
		return fmt.Errorf("encode worker transport daemon control request: %w", err)
	}
	data = append(data, '\n')
	tmp := control.Path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("write worker transport daemon control request: %w", err)
	}
	if err := os.Rename(tmp, control.Path); err != nil {
		return fmt.Errorf("publish worker transport daemon control request: %w", err)
	}
	return nil
}

func writeRuntimeWorkerQueueDispatchResult(root string, result *RuntimeWorkerQueueDispatchResult) error {
	dir := runtimeWorkerQueueDispatchResultDir(root)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create worker queue dispatch result directory: %w", err)
	}
	if strings.TrimSpace(result.Path) == "" {
		result.Path = filepath.Join(dir, safeRuntimeWorkerQueueDispatchResultName(*result)+".json")
	}
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return fmt.Errorf("encode worker queue dispatch result: %w", err)
	}
	data = append(data, '\n')
	tmp := result.Path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("write worker queue dispatch result: %w", err)
	}
	if err := os.Rename(tmp, result.Path); err != nil {
		return fmt.Errorf("publish worker queue dispatch result: %w", err)
	}
	return nil
}

func readRuntimeWorkerQueueDispatchExecutionFile(path string) (RuntimeWorkerQueueDispatchExecution, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return RuntimeWorkerQueueDispatchExecution{}, fmt.Errorf("read worker queue dispatch execution %s: %w", path, err)
	}
	var execution RuntimeWorkerQueueDispatchExecution
	if err := json.Unmarshal(data, &execution); err != nil {
		return RuntimeWorkerQueueDispatchExecution{}, fmt.Errorf("decode worker queue dispatch execution %s: %w", path, err)
	}
	execution.Path = path
	return execution, nil
}

func readRuntimeWorkerQueueDispatchResultFile(path string) (RuntimeWorkerQueueDispatchResult, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return RuntimeWorkerQueueDispatchResult{}, fmt.Errorf("read worker queue dispatch result %s: %w", path, err)
	}
	var result RuntimeWorkerQueueDispatchResult
	if err := json.Unmarshal(data, &result); err != nil {
		return RuntimeWorkerQueueDispatchResult{}, fmt.Errorf("decode worker queue dispatch result %s: %w", path, err)
	}
	result.Path = path
	return result, nil
}

func readRuntimeWorkerTransportDispatchRequestFile(path string) (RuntimeWorkerTransportDispatchRequest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return RuntimeWorkerTransportDispatchRequest{}, fmt.Errorf("read worker transport dispatch request %s: %w", path, err)
	}
	var request RuntimeWorkerTransportDispatchRequest
	if err := json.Unmarshal(data, &request); err != nil {
		return RuntimeWorkerTransportDispatchRequest{}, fmt.Errorf("decode worker transport dispatch request %s: %w", path, err)
	}
	request.Path = path
	return request, nil
}

func readRuntimeWorkerTransportDaemonRunFile(path string) (RuntimeWorkerTransportRunDaemonResult, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return RuntimeWorkerTransportRunDaemonResult{}, fmt.Errorf("read worker transport daemon run %s: %w", path, err)
	}
	var run RuntimeWorkerTransportRunDaemonResult
	if err := json.Unmarshal(data, &run); err != nil {
		return RuntimeWorkerTransportRunDaemonResult{}, fmt.Errorf("decode worker transport daemon run %s: %w", path, err)
	}
	run.Path = path
	return run, nil
}

func readRuntimeWorkerTransportDaemonSupervisorRunFile(path string) (RuntimeWorkerTransportDaemonSupervisorRun, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return RuntimeWorkerTransportDaemonSupervisorRun{}, fmt.Errorf("read worker transport daemon supervisor run %s: %w", path, err)
	}
	var run RuntimeWorkerTransportDaemonSupervisorRun
	if err := json.Unmarshal(data, &run); err != nil {
		return RuntimeWorkerTransportDaemonSupervisorRun{}, fmt.Errorf("decode worker transport daemon supervisor run %s: %w", path, err)
	}
	run.Path = path
	return run, nil
}

func readRuntimeWorkerTransportDaemonControlRequestFile(path string) (RuntimeWorkerTransportDaemonControlRequest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return RuntimeWorkerTransportDaemonControlRequest{}, fmt.Errorf("read worker transport daemon control request %s: %w", path, err)
	}
	var control RuntimeWorkerTransportDaemonControlRequest
	if err := json.Unmarshal(data, &control); err != nil {
		return RuntimeWorkerTransportDaemonControlRequest{}, fmt.Errorf("decode worker transport daemon control request %s: %w", path, err)
	}
	control.Path = path
	return control, nil
}

func readRuntimeWorkerTransportDaemonControlRequestFiles(paths []string) ([]RuntimeWorkerTransportDaemonControlRequest, error) {
	sort.Strings(paths)
	controls := make([]RuntimeWorkerTransportDaemonControlRequest, 0, len(paths))
	for _, path := range paths {
		control, err := readRuntimeWorkerTransportDaemonControlRequestFile(path)
		if err != nil {
			return nil, err
		}
		controls = append(controls, control)
	}
	sort.SliceStable(controls, func(i, j int) bool {
		if controls[i].RequestedAt != controls[j].RequestedAt {
			return controls[i].RequestedAt < controls[j].RequestedAt
		}
		return controls[i].Path < controls[j].Path
	})
	return controls, nil
}

func runtimeWorkerQueueDispatchExecutionDir(root string) string {
	return filepath.Join(root, "runtime", "worker-queue-dispatch-executions")
}

func runtimeWorkerQueueDispatchResultDir(root string) string {
	return filepath.Join(root, "runtime", "worker-queue-dispatch-results")
}

func runtimeWorkerTransportOutboxDir(root string) string {
	return filepath.Join(root, "runtime", "worker-transport-outbox")
}

func runtimeWorkerTransportWorkerOutboxDir(root, workerID string) string {
	return filepath.Join(runtimeWorkerTransportOutboxDir(root), safeRuntimeWorkerFileName(workerID))
}

func runtimeWorkerTransportDaemonRunDir(root string) string {
	return filepath.Join(root, "runtime", "worker-transport-daemon-runs")
}

func runtimeWorkerTransportDaemonRunWorkerDir(root, workerID string) string {
	return filepath.Join(runtimeWorkerTransportDaemonRunDir(root), safeRuntimeWorkerFileName(workerID))
}

func runtimeWorkerTransportDaemonSupervisorRunDir(root string) string {
	return filepath.Join(root, "runtime", "worker-transport-daemon-supervisor-runs")
}

func runtimeWorkerTransportDaemonSupervisorRunWorkerDir(root, workerID string) string {
	return filepath.Join(runtimeWorkerTransportDaemonSupervisorRunDir(root), safeRuntimeWorkerFileName(workerID))
}

func runtimeWorkerTransportDaemonControlDir(root string) string {
	return filepath.Join(root, "runtime", "worker-transport-daemon-controls")
}

func runtimeWorkerTransportDaemonControlWorkerDir(root, workerID string) string {
	return filepath.Join(runtimeWorkerTransportDaemonControlDir(root), safeRuntimeWorkerFileName(workerID))
}

func runtimeWorkerTransportRunDir(root string, request RuntimeWorkerTransportDispatchRequest) string {
	return filepath.Join(root, "runtime", "worker-transport-runs", safeRuntimeWorkerFileName(request.WorkerID), safeInstallRequestName(strings.Join([]string{
		request.RunID,
		request.ExecutionID,
		request.Tool,
		request.Capability,
		request.Path,
	}, "|")))
}

func ensureRuntimeWorkerQueueDispatchExecutionPath(root string, execution *RuntimeWorkerQueueDispatchExecution) {
	if strings.TrimSpace(execution.Path) == "" {
		execution.Path = filepath.Join(runtimeWorkerQueueDispatchExecutionDir(root), safeRuntimeWorkerQueueDispatchExecutionName(*execution)+".json")
	}
}

func resolveRuntimeWorkerQueueDispatchExecutionPath(root, executionPath string) (string, error) {
	trimmed := strings.TrimSpace(executionPath)
	if trimmed == "" {
		return "", fmt.Errorf("worker queue dispatch execution path is required")
	}
	dir, err := filepath.Abs(runtimeWorkerQueueDispatchExecutionDir(root))
	if err != nil {
		return "", fmt.Errorf("resolve worker queue dispatch execution directory: %w", err)
	}
	candidate := trimmed
	if !filepath.IsAbs(candidate) {
		if filepath.Ext(candidate) == "" && !strings.Contains(candidate, string(os.PathSeparator)) {
			candidate += ".json"
		}
		candidate = filepath.Join(dir, candidate)
	}
	candidate, err = filepath.Abs(candidate)
	if err != nil {
		return "", fmt.Errorf("resolve worker queue dispatch execution path: %w", err)
	}
	rel, err := filepath.Rel(dir, candidate)
	if err != nil {
		return "", fmt.Errorf("resolve worker queue dispatch execution relative path: %w", err)
	}
	if rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("worker queue dispatch execution path must stay under %s", dir)
	}
	if filepath.Ext(candidate) != ".json" {
		return "", fmt.Errorf("worker queue dispatch execution path must be a json file")
	}
	return candidate, nil
}

func resolveRuntimeWorkerQueueDispatchResultPath(root, resultPath string) (string, error) {
	trimmed := strings.TrimSpace(resultPath)
	if trimmed == "" {
		return "", fmt.Errorf("worker queue dispatch result path is required")
	}
	dir, err := filepath.Abs(runtimeWorkerQueueDispatchResultDir(root))
	if err != nil {
		return "", fmt.Errorf("resolve worker queue dispatch result directory: %w", err)
	}
	candidate := trimmed
	if !filepath.IsAbs(candidate) {
		if filepath.Ext(candidate) == "" && !strings.Contains(candidate, string(os.PathSeparator)) {
			candidate += ".json"
		}
		candidate = filepath.Join(dir, candidate)
	}
	candidate, err = filepath.Abs(candidate)
	if err != nil {
		return "", fmt.Errorf("resolve worker queue dispatch result path: %w", err)
	}
	rel, err := filepath.Rel(dir, candidate)
	if err != nil {
		return "", fmt.Errorf("resolve worker queue dispatch result relative path: %w", err)
	}
	if rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("worker queue dispatch result path must stay under %s", dir)
	}
	if filepath.Ext(candidate) != ".json" {
		return "", fmt.Errorf("worker queue dispatch result path must be a json file")
	}
	return candidate, nil
}

func resolveRuntimeWorkerTransportDispatchRequestPath(root, workerID, requestPath string) (string, error) {
	if strings.TrimSpace(workerID) == "" {
		return "", fmt.Errorf("worker transport request worker id is required")
	}
	trimmed := strings.TrimSpace(requestPath)
	if trimmed == "" {
		return "", fmt.Errorf("worker transport request path is required")
	}
	dir, err := filepath.Abs(runtimeWorkerTransportWorkerOutboxDir(root, workerID))
	if err != nil {
		return "", fmt.Errorf("resolve worker transport request directory: %w", err)
	}
	candidate := trimmed
	if !filepath.IsAbs(candidate) {
		if filepath.Ext(candidate) == "" && !strings.Contains(candidate, string(os.PathSeparator)) {
			candidate += ".json"
		}
		candidate = filepath.Join(dir, candidate)
	}
	candidate, err = filepath.Abs(candidate)
	if err != nil {
		return "", fmt.Errorf("resolve worker transport request path: %w", err)
	}
	rel, err := filepath.Rel(dir, candidate)
	if err != nil {
		return "", fmt.Errorf("resolve worker transport request relative path: %w", err)
	}
	if rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("worker transport request path must stay under %s", dir)
	}
	if filepath.Ext(candidate) != ".json" {
		return "", fmt.Errorf("worker transport request path must be a json file")
	}
	return candidate, nil
}

func resolveRuntimeWorkerTransportDaemonRunPath(root, workerID, runPath string) (string, error) {
	if strings.TrimSpace(workerID) == "" {
		return "", fmt.Errorf("worker transport daemon run worker id is required")
	}
	trimmed := strings.TrimSpace(runPath)
	if trimmed == "" {
		return "", fmt.Errorf("worker transport daemon run path is required")
	}
	dir, err := filepath.Abs(runtimeWorkerTransportDaemonRunWorkerDir(root, workerID))
	if err != nil {
		return "", fmt.Errorf("resolve worker transport daemon run directory: %w", err)
	}
	candidate := trimmed
	if !filepath.IsAbs(candidate) {
		if filepath.Ext(candidate) == "" && !strings.Contains(candidate, string(os.PathSeparator)) {
			candidate += ".json"
		}
		candidate = filepath.Join(dir, candidate)
	}
	candidate, err = filepath.Abs(candidate)
	if err != nil {
		return "", fmt.Errorf("resolve worker transport daemon run path: %w", err)
	}
	rel, err := filepath.Rel(dir, candidate)
	if err != nil {
		return "", fmt.Errorf("resolve worker transport daemon run relative path: %w", err)
	}
	if rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("worker transport daemon run path must stay under %s", dir)
	}
	if filepath.Ext(candidate) != ".json" {
		return "", fmt.Errorf("worker transport daemon run path must be a json file")
	}
	return candidate, nil
}

func resolveRuntimeWorkerTransportDaemonSupervisorRunPath(root, workerID, runPath string) (string, error) {
	if strings.TrimSpace(workerID) == "" {
		return "", fmt.Errorf("worker transport daemon supervisor worker id is required")
	}
	trimmed := strings.TrimSpace(runPath)
	if trimmed == "" {
		return "", fmt.Errorf("worker transport daemon supervisor run path is required")
	}
	dir, err := filepath.Abs(runtimeWorkerTransportDaemonSupervisorRunWorkerDir(root, workerID))
	if err != nil {
		return "", fmt.Errorf("resolve worker transport daemon supervisor run directory: %w", err)
	}
	candidate := trimmed
	if !filepath.IsAbs(candidate) {
		if filepath.Ext(candidate) == "" && !strings.Contains(candidate, string(os.PathSeparator)) {
			candidate += ".json"
		}
		candidate = filepath.Join(dir, candidate)
	}
	candidate, err = filepath.Abs(candidate)
	if err != nil {
		return "", fmt.Errorf("resolve worker transport daemon supervisor run path: %w", err)
	}
	rel, err := filepath.Rel(dir, candidate)
	if err != nil {
		return "", fmt.Errorf("resolve worker transport daemon supervisor run relative path: %w", err)
	}
	if rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("worker transport daemon supervisor run path must stay under %s", dir)
	}
	if filepath.Ext(candidate) != ".json" {
		return "", fmt.Errorf("worker transport daemon supervisor run path must be a json file")
	}
	return candidate, nil
}

func resolveRuntimeWorkerTransportDaemonControlPath(root, workerID, controlPath string) (string, error) {
	if strings.TrimSpace(workerID) == "" {
		return "", fmt.Errorf("worker transport daemon control worker id is required")
	}
	trimmed := strings.TrimSpace(controlPath)
	if trimmed == "" {
		return "", fmt.Errorf("worker transport daemon control path is required")
	}
	dir, err := filepath.Abs(runtimeWorkerTransportDaemonControlWorkerDir(root, workerID))
	if err != nil {
		return "", fmt.Errorf("resolve worker transport daemon control directory: %w", err)
	}
	candidate := trimmed
	if !filepath.IsAbs(candidate) {
		if filepath.Ext(candidate) == "" && !strings.Contains(candidate, string(os.PathSeparator)) {
			candidate += ".json"
		}
		candidate = filepath.Join(dir, candidate)
	}
	candidate, err = filepath.Abs(candidate)
	if err != nil {
		return "", fmt.Errorf("resolve worker transport daemon control path: %w", err)
	}
	rel, err := filepath.Rel(dir, candidate)
	if err != nil {
		return "", fmt.Errorf("resolve worker transport daemon control relative path: %w", err)
	}
	if rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("worker transport daemon control path must stay under %s", dir)
	}
	if filepath.Ext(candidate) != ".json" {
		return "", fmt.Errorf("worker transport daemon control path must be a json file")
	}
	return candidate, nil
}

func normalizeRuntimeWorkerQueueDispatchResultStatus(status string) (string, error) {
	trimmed := strings.TrimSpace(status)
	switch trimmed {
	case "completed", "failed":
		return trimmed, nil
	default:
		return "", fmt.Errorf("unsupported worker queue dispatch result status: %s", status)
	}
}

func normalizeRuntimeWorkerTransportDaemonControlAction(action string) (string, error) {
	trimmed := strings.TrimSpace(action)
	switch trimmed {
	case "stop", "restart":
		return trimmed, nil
	default:
		return "", fmt.Errorf("unsupported worker transport daemon control action: %s", action)
	}
}

func normalizeRuntimeWorkerResultCallbackEndpoint(endpoint string) (string, error) {
	trimmed := strings.TrimSpace(endpoint)
	if trimmed == "" {
		return "", nil
	}
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return "", fmt.Errorf("parse worker result callback endpoint: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("worker result callback endpoint requires http or https")
	}
	return trimmed, nil
}

func normalizeRuntimeWorkerQueueDispatchTransport(transportMode string) (string, error) {
	trimmed := strings.TrimSpace(transportMode)
	if trimmed == "" {
		return "metadata-only", nil
	}
	switch trimmed {
	case "metadata-only", "file-outbox", "http-json":
		return trimmed, nil
	default:
		return "", fmt.Errorf("unsupported worker queue dispatch transport: %s", transportMode)
	}
}

func safeRuntimeWorkerQueueDispatchExecutionName(execution RuntimeWorkerQueueDispatchExecution) string {
	return safeInstallRequestName(strings.Join([]string{
		"worker-queue-dispatch-execution",
		execution.Status,
		execution.TransportMode,
		execution.ClaimPath,
		execution.ClaimID,
		execution.WorkerID,
		execution.RunID,
		execution.ExecutionID,
		execution.Capability,
		execution.ExecutedAt,
		execution.SkipReason,
	}, "|"))
}

func safeRuntimeWorkerQueueDispatchResultName(result RuntimeWorkerQueueDispatchResult) string {
	return safeInstallRequestName(strings.Join([]string{
		"worker-queue-dispatch-result",
		result.Status,
		result.DispatchExecutionPath,
		result.ClaimPath,
		result.ClaimID,
		result.WorkerID,
		result.RunID,
		result.ExecutionID,
		result.Capability,
		result.SubmittedAt,
	}, "|"))
}

func safeRuntimeWorkerTransportDispatchRequestName(request RuntimeWorkerTransportDispatchRequest) string {
	return safeInstallRequestName(strings.Join([]string{
		"worker-transport-dispatch-request",
		request.TransportMode,
		request.DispatchExecutionPath,
		request.ClaimPath,
		request.ClaimID,
		request.WorkerID,
		request.RunID,
		request.ExecutionID,
		request.Capability,
		request.CreatedAt,
	}, "|"))
}

func safeRuntimeWorkerTransportDaemonRunName(daemon RuntimeWorkerTransportRunDaemonResult) string {
	return safeInstallRequestName(strings.Join([]string{
		"worker-transport-daemon-run",
		daemon.Status,
		daemon.WorkerID,
		daemon.StartedAt,
		daemon.CompletedAt,
		fmt.Sprint(daemon.Processed),
		fmt.Sprint(daemon.Completed),
		fmt.Sprint(daemon.Failed),
	}, "|"))
}

func safeRuntimeWorkerTransportDaemonSupervisorRunName(supervisor RuntimeWorkerTransportDaemonSupervisorRun) string {
	return safeInstallRequestName(strings.Join([]string{
		"worker-transport-daemon-supervisor-run",
		supervisor.Status,
		supervisor.WorkerID,
		supervisor.StartedAt,
		supervisor.CompletedAt,
		fmt.Sprint(supervisor.Runs),
		fmt.Sprint(supervisor.RestartCount),
		fmt.Sprint(supervisor.Processed),
		fmt.Sprint(supervisor.Completed),
		fmt.Sprint(supervisor.Failed),
	}, "|"))
}

func safeRuntimeWorkerTransportDaemonControlRequestName(control RuntimeWorkerTransportDaemonControlRequest) string {
	return safeInstallRequestName(strings.Join([]string{
		"worker-transport-daemon-control",
		control.Status,
		control.WorkerID,
		control.Action,
		control.RequestedAt,
		control.RequestedBy,
		control.Reason,
	}, "|"))
}
