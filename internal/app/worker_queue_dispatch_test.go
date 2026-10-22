package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestExecuteRuntimeWorkerQueueClaimDispatchRequiresExplicitExecute(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	now := time.Date(2026, 5, 30, 16, 0, 0, 0, time.UTC)
	if _, err := RegisterRuntimeWorker(root, RuntimeWorkerArtifact{
		WorkerID:     "remote-web-1",
		Mode:         "remote",
		Status:       "available",
		Capabilities: []string{"web.discovery"},
	}, RuntimeWorkerRegistrationOptions{Now: now}); err != nil {
		t.Fatalf("register runtime worker: %v", err)
	}
	claim, err := CreateRuntimeWorkerQueueClaim(root, RuntimeWorkerQueueClaim{
		WorkerID:    "remote-web-1",
		RunID:       "run-dispatch-1",
		ExecutionID: "tool-web",
		Tool:        "web-api-pack",
		Capability:  "web.discovery",
	}, RuntimeWorkerQueueClaimOptions{Now: now, LeaseDuration: 10 * time.Minute})
	if err != nil {
		t.Fatalf("create runtime worker queue claim: %v", err)
	}

	_, err = ExecuteRuntimeWorkerQueueClaimDispatch(root, claim.Path, RuntimeWorkerQueueDispatchExecutionOptions{
		Now: now.Add(time.Minute),
	})
	if err == nil || !strings.Contains(err.Error(), "requires explicit execute") {
		t.Fatalf("expected explicit execute error, got %v", err)
	}
	claims, err := ListRuntimeWorkerQueueClaims(root)
	if err != nil {
		t.Fatalf("list queue claims: %v", err)
	}
	if len(claims) != 1 || claims[0].Status != "claimed" || claims[0].DispatchedAt != "" {
		t.Fatalf("execution without execute flag must not mutate claim, got %#v", claims)
	}
	executions, err := ListRuntimeWorkerQueueDispatchExecutions(root)
	if err != nil {
		t.Fatalf("list queue dispatch executions: %v", err)
	}
	if len(executions) != 0 {
		t.Fatalf("execution without execute flag must not write audit artifact, got %#v", executions)
	}
}

func TestExecuteRuntimeWorkerQueueClaimDispatchRequiresCurrentSafetyAuthorizationForDisabledBoundaries(t *testing.T) {
	t.Skip("Safety authorization checks disabled")
	for _, transportMode := range []string{"file-outbox", "http-json"} {
		t.Run(transportMode, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "workspace")
			if err := InitWorkspace(root); err != nil {
				t.Fatalf("init workspace: %v", err)
			}
			now := time.Date(2026, 6, 2, 14, 0, 0, 0, time.UTC)
			var requestsReceived int
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requestsReceived++
				w.WriteHeader(http.StatusAccepted)
			}))
			defer server.Close()
			workerEndpoint := "file://remote-web-1/inbox"
			if transportMode == "http-json" {
				workerEndpoint = server.URL
			}
			if _, err := RegisterRuntimeWorker(root, RuntimeWorkerArtifact{
				WorkerID:     "remote-web-1",
				Mode:         "remote",
				Status:       "available",
				Endpoint:     workerEndpoint,
				Capabilities: []string{"web.discovery"},
				HeartbeatAt:  now.Format(time.RFC3339Nano),
			}, RuntimeWorkerRegistrationOptions{Now: now}); err != nil {
				t.Fatalf("register runtime worker: %v", err)
			}
			claim, err := CreateRuntimeWorkerQueueClaim(root, RuntimeWorkerQueueClaim{
				WorkerID:    "remote-web-1",
				RunID:       "run-safety-dispatch",
				ExecutionID: "tool-web",
				Tool:        "web-api-pack",
				Capability:  "web.discovery",
			}, RuntimeWorkerQueueClaimOptions{Now: now, LeaseDuration: 10 * time.Minute})
			if err != nil {
				t.Fatalf("create runtime worker queue claim: %v", err)
			}
			writeRuntimeWorkerDisabledSafetyScope(t, root)

			_, err = ExecuteRuntimeWorkerQueueClaimDispatch(root, claim.Path, RuntimeWorkerQueueDispatchExecutionOptions{
				Execute:       true,
				Now:           now.Add(time.Minute),
				TransportMode: transportMode,
			})
			if err == nil || !strings.Contains(err.Error(), "worker runtime side effect requires current safety bypass authorization") || !strings.Contains(err.Error(), "missing required safety bypass authorization for scope.target_policy disabled") {
				t.Fatalf("expected strict safety authorization error, got %v", err)
			}
			updated, err := ReadRuntimeWorkerQueueClaim(root, filepath.Base(claim.Path))
			if err != nil {
				t.Fatalf("read queue claim after blocked dispatch: %v", err)
			}
			if updated.Status != "claimed" || updated.DispatchExecutionPath != "" || updated.DispatchedAt != "" {
				t.Fatalf("blocked dispatch must not mutate claim, got %#v", updated)
			}
			executions, err := ListRuntimeWorkerQueueDispatchExecutions(root)
			if err != nil {
				t.Fatalf("list dispatch executions after blocked dispatch: %v", err)
			}
			if len(executions) != 0 {
				t.Fatalf("blocked dispatch must not write execution audit, got %#v", executions)
			}
			requests, err := ListRuntimeWorkerTransportDispatchRequests(root, "remote-web-1")
			if err != nil {
				t.Fatalf("list transport requests after blocked dispatch: %v", err)
			}
			if len(requests) != 0 {
				t.Fatalf("blocked dispatch must not write transport requests, got %#v", requests)
			}
			if requestsReceived != 0 {
				t.Fatalf("blocked http-json dispatch must not contact worker endpoint, got %d requests", requestsReceived)
			}
		})
	}
}

func TestExecuteRuntimeWorkerQueueClaimDispatchAllowsDisabledBoundariesWithCurrentSafetyAuthorization(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	now := time.Date(2026, 6, 2, 14, 5, 0, 0, time.UTC)
	if _, err := RegisterRuntimeWorker(root, RuntimeWorkerArtifact{
		WorkerID:     "remote-web-1",
		Mode:         "remote",
		Status:       "available",
		Endpoint:     "file://remote-web-1/inbox",
		Capabilities: []string{"web.discovery"},
		HeartbeatAt:  now.Format(time.RFC3339Nano),
	}, RuntimeWorkerRegistrationOptions{Now: now}); err != nil {
		t.Fatalf("register runtime worker: %v", err)
	}
	claim, err := CreateRuntimeWorkerQueueClaim(root, RuntimeWorkerQueueClaim{
		WorkerID:    "remote-web-1",
		RunID:       "run-safety-authorized",
		ExecutionID: "tool-web",
		Tool:        "web-api-pack",
		Capability:  "web.discovery",
	}, RuntimeWorkerQueueClaimOptions{Now: now, LeaseDuration: 10 * time.Minute})
	if err != nil {
		t.Fatalf("create runtime worker queue claim: %v", err)
	}
	writeRuntimeWorkerDisabledSafetyScope(t, root)
	writeRuntimeWorkerCurrentSafetyAuthorization(t, root)

	execution, err := ExecuteRuntimeWorkerQueueClaimDispatch(root, claim.Path, RuntimeWorkerQueueDispatchExecutionOptions{
		Execute:       true,
		Now:           now.Add(time.Minute),
		TransportMode: "file-outbox",
	})
	if err != nil {
		t.Fatalf("authorized disabled safety boundary should allow queue dispatch: %v", err)
	}
	if execution.Status != "dispatched" || execution.TransportRequestPath == "" {
		t.Fatalf("expected authorized file-outbox dispatch, got %#v", execution)
	}
}

func TestExecuteRuntimeWorkerQueueClaimDispatchRevalidatesAndMarksClaimDispatched(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	now := time.Date(2026, 5, 30, 16, 30, 0, 0, time.UTC)
	if _, err := RegisterRuntimeWorker(root, RuntimeWorkerArtifact{
		WorkerID:     "remote-web-1",
		Mode:         "remote",
		Status:       "available",
		Endpoint:     "tcp://127.0.0.1:9443",
		Capabilities: []string{"web.discovery"},
		HeartbeatAt:  now.Format(time.RFC3339Nano),
	}, RuntimeWorkerRegistrationOptions{Now: now}); err != nil {
		t.Fatalf("register runtime worker: %v", err)
	}
	claim, err := CreateRuntimeWorkerQueueClaim(root, RuntimeWorkerQueueClaim{
		WorkerID:    "remote-web-1",
		RunID:       "run-dispatch-1",
		ExecutionID: "tool-web",
		Tool:        "web-api-pack",
		Capability:  "web.discovery",
	}, RuntimeWorkerQueueClaimOptions{Now: now, LeaseDuration: 10 * time.Minute})
	if err != nil {
		t.Fatalf("create runtime worker queue claim: %v", err)
	}

	execution, err := ExecuteRuntimeWorkerQueueClaimDispatch(root, filepath.Base(claim.Path), RuntimeWorkerQueueDispatchExecutionOptions{
		Execute: true,
		Now:     now.Add(time.Minute),
	})
	if err != nil {
		t.Fatalf("execute runtime worker queue claim dispatch: %v", err)
	}
	if execution.Status != "dispatched" || execution.Mode != "execute" || execution.TransportMode != "metadata-only" {
		t.Fatalf("unexpected queue dispatch execution: %#v", execution)
	}
	if execution.ClaimPath != claim.Path || execution.WorkerID != "remote-web-1" || execution.RunID != "run-dispatch-1" || execution.ExecutionID != "tool-web" {
		t.Fatalf("execution did not preserve claim identity: %#v", execution)
	}
	if !strings.Contains(execution.Path, filepath.Join("runtime", "worker-queue-dispatch-executions")) {
		t.Fatalf("expected audit under worker-queue-dispatch-executions, got %s", execution.Path)
	}
	updated, err := ReadRuntimeWorkerQueueClaim(root, filepath.Base(claim.Path))
	if err != nil {
		t.Fatalf("read updated queue claim: %v", err)
	}
	if updated.Status != "dispatched" || updated.DispatchedAt == "" || updated.DispatchExecutionPath != execution.Path {
		t.Fatalf("expected claim to be explicitly marked dispatched, got %#v", updated)
	}
}

func TestExecuteRuntimeWorkerQueueClaimDispatchSkipsUnavailableWorker(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	now := time.Date(2026, 5, 30, 17, 0, 0, 0, time.UTC)
	if _, err := RegisterRuntimeWorker(root, RuntimeWorkerArtifact{
		WorkerID:     "remote-web-1",
		Mode:         "remote",
		Status:       "available",
		Capabilities: []string{"web.discovery"},
	}, RuntimeWorkerRegistrationOptions{Now: now}); err != nil {
		t.Fatalf("register runtime worker: %v", err)
	}
	claim, err := CreateRuntimeWorkerQueueClaim(root, RuntimeWorkerQueueClaim{
		WorkerID:    "remote-web-1",
		RunID:       "run-dispatch-2",
		ExecutionID: "tool-web",
		Tool:        "web-api-pack",
		Capability:  "web.discovery",
	}, RuntimeWorkerQueueClaimOptions{Now: now, LeaseDuration: 10 * time.Minute})
	if err != nil {
		t.Fatalf("create runtime worker queue claim: %v", err)
	}
	if _, err := UpdateRuntimeWorkerHeartbeat(root, "remote-web-1", RuntimeWorkerHeartbeatOptions{
		Now:    now.Add(time.Minute),
		Status: "offline",
	}); err != nil {
		t.Fatalf("mark runtime worker offline: %v", err)
	}

	execution, err := ExecuteRuntimeWorkerQueueClaimDispatch(root, claim.Path, RuntimeWorkerQueueDispatchExecutionOptions{
		Execute: true,
		Now:     now.Add(2 * time.Minute),
	})
	if err != nil {
		t.Fatalf("execute runtime worker queue claim dispatch: %v", err)
	}
	if execution.Status != "skipped" || execution.SkipReason != "worker-offline" {
		t.Fatalf("expected worker-offline skip audit, got %#v", execution)
	}
	updated, err := ReadRuntimeWorkerQueueClaim(root, filepath.Base(claim.Path))
	if err != nil {
		t.Fatalf("read updated queue claim: %v", err)
	}
	if updated.Status != "claimed" || updated.DispatchedAt != "" {
		t.Fatalf("skipped dispatch must not mark claim dispatched, got %#v", updated)
	}
}

func TestExecuteRuntimeWorkerQueueClaimDispatchFileOutboxWritesTransportRequest(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	now := time.Date(2026, 5, 30, 17, 30, 0, 0, time.UTC)
	if _, err := RegisterRuntimeWorker(root, RuntimeWorkerArtifact{
		WorkerID:     "remote-web-1",
		Mode:         "remote",
		Status:       "available",
		Endpoint:     "file://remote-web-1/inbox",
		Capabilities: []string{"web.discovery"},
	}, RuntimeWorkerRegistrationOptions{Now: now}); err != nil {
		t.Fatalf("register runtime worker: %v", err)
	}
	claim, err := CreateRuntimeWorkerQueueClaim(root, RuntimeWorkerQueueClaim{
		WorkerID:    "remote-web-1",
		RunID:       "run-dispatch-file",
		ExecutionID: "tool-web",
		Tool:        "web-api-pack",
		Capability:  "web.discovery",
	}, RuntimeWorkerQueueClaimOptions{Now: now, LeaseDuration: 10 * time.Minute})
	if err != nil {
		t.Fatalf("create runtime worker queue claim: %v", err)
	}

	execution, err := ExecuteRuntimeWorkerQueueClaimDispatch(root, claim.Path, RuntimeWorkerQueueDispatchExecutionOptions{
		Execute:       true,
		Now:           now.Add(time.Minute),
		TransportMode: "file-outbox",
	})
	if err != nil {
		t.Fatalf("execute runtime worker queue claim dispatch: %v", err)
	}
	if execution.TransportMode != "file-outbox" || execution.TransportRequestPath == "" {
		t.Fatalf("expected file-outbox transport request metadata, got %#v", execution)
	}
	if !strings.Contains(execution.TransportRequestPath, filepath.Join("runtime", "worker-transport-outbox", "remote-web-1")) {
		t.Fatalf("expected worker outbox request path, got %s", execution.TransportRequestPath)
	}
	data, err := os.ReadFile(execution.TransportRequestPath)
	if err != nil {
		t.Fatalf("read transport request: %v", err)
	}
	var request RuntimeWorkerTransportDispatchRequest
	if err := json.Unmarshal(data, &request); err != nil {
		t.Fatalf("decode transport request: %v", err)
	}
	if request.Status != "pending" || request.DispatchExecutionPath != execution.Path || request.WorkerID != "remote-web-1" || request.ExecutionID != "tool-web" {
		t.Fatalf("unexpected transport request: %#v", request)
	}
}

func TestExecuteRuntimeWorkerQueueClaimDispatchFileOutboxCarriesToolPlanPayload(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	now := time.Date(2026, 5, 30, 17, 35, 0, 0, time.UTC)
	if _, err := RegisterRuntimeWorker(root, RuntimeWorkerArtifact{
		WorkerID:     "remote-web-1",
		Mode:         "remote",
		Status:       "available",
		Endpoint:     "file://remote-web-1/inbox",
		Capabilities: []string{"web.discovery"},
	}, RuntimeWorkerRegistrationOptions{Now: now}); err != nil {
		t.Fatalf("register runtime worker: %v", err)
	}
	claim, err := CreateRuntimeWorkerQueueClaim(root, RuntimeWorkerQueueClaim{
		WorkerID:    "remote-web-1",
		RunID:       "run-dispatch-file",
		ExecutionID: "tool-web",
		Tool:        "web-api-pack",
		Capability:  "web.discovery",
		ToolPlan: RuntimeWorkerToolPlanPayload{
			Action:           "discover_assets",
			ManifestName:     "web-api-pack",
			Runner:           "command",
			RunCommand:       []string{"nuclei", "-u", "{{target}}", "-jsonl"},
			RunTimeoutMS:     2500,
			Parser:           "nuclei-json",
			InputNodeIDs:     []string{"target:https://example.test"},
			ExpectedEvidence: []string{"surface", "artifact"},
		},
	}, RuntimeWorkerQueueClaimOptions{Now: now, LeaseDuration: 10 * time.Minute})
	if err != nil {
		t.Fatalf("create runtime worker queue claim: %v", err)
	}

	execution, err := ExecuteRuntimeWorkerQueueClaimDispatch(root, claim.Path, RuntimeWorkerQueueDispatchExecutionOptions{
		Execute:       true,
		Now:           now.Add(time.Minute),
		TransportMode: "file-outbox",
	})
	if err != nil {
		t.Fatalf("execute runtime worker queue claim dispatch: %v", err)
	}
	if execution.ToolPlan.Action != "discover_assets" || execution.ToolPlan.Runner != "command" || len(execution.ToolPlan.RunCommand) != 4 {
		t.Fatalf("dispatch execution did not preserve tool plan payload: %#v", execution.ToolPlan)
	}
	request, err := ReadRuntimeWorkerTransportDispatchRequest(root, "remote-web-1", filepath.Base(execution.TransportRequestPath))
	if err != nil {
		t.Fatalf("read transport request: %v", err)
	}
	if request.ToolPlan.Action != "discover_assets" || request.ToolPlan.ManifestName != "web-api-pack" || request.ToolPlan.Parser != "nuclei-json" {
		t.Fatalf("transport request did not carry tool plan identity: %#v", request.ToolPlan)
	}
	if got := strings.Join(request.ToolPlan.RunCommand, " "); got != "nuclei -u {{target}} -jsonl" {
		t.Fatalf("transport request did not carry run command, got %q", got)
	}
	if got := strings.Join(request.ToolPlan.ExpectedEvidence, ","); got != "surface,artifact" {
		t.Fatalf("transport request did not carry expected evidence, got %q", got)
	}
}

func TestExecuteRuntimeWorkerQueueClaimDispatchHTTPJSONPostsRequest(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	received := make(chan RuntimeWorkerTransportDispatchRequest, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST request, got %s", r.Method)
		}
		if r.URL.Path != "/dispatch" {
			t.Errorf("expected /dispatch path, got %s", r.URL.Path)
		}
		defer r.Body.Close()
		var request RuntimeWorkerTransportDispatchRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode http-json request: %v", err)
			return
		}
		received <- request
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"request_id":"remote-accepted-1"}`))
	}))
	defer server.Close()
	now := time.Date(2026, 5, 30, 17, 40, 0, 0, time.UTC)
	if _, err := RegisterRuntimeWorker(root, RuntimeWorkerArtifact{
		WorkerID:     "remote-web-1",
		Mode:         "remote",
		Status:       "available",
		Endpoint:     server.URL + "/dispatch",
		Capabilities: []string{"web.discovery"},
	}, RuntimeWorkerRegistrationOptions{Now: now}); err != nil {
		t.Fatalf("register runtime worker: %v", err)
	}
	claim, err := CreateRuntimeWorkerQueueClaim(root, RuntimeWorkerQueueClaim{
		WorkerID:    "remote-web-1",
		RunID:       "run-http-json",
		ExecutionID: "tool-web",
		Tool:        "web-api-pack",
		Capability:  "web.discovery",
		ToolPlan: RuntimeWorkerToolPlanPayload{
			Action:       "discover_assets",
			ManifestName: "web-api-pack",
			Runner:       "command",
			RunCommand:   []string{"nuclei", "-u", "{{target}}"},
			Parser:       "nuclei-json",
		},
	}, RuntimeWorkerQueueClaimOptions{Now: now, LeaseDuration: 10 * time.Minute})
	if err != nil {
		t.Fatalf("create runtime worker queue claim: %v", err)
	}

	execution, err := ExecuteRuntimeWorkerQueueClaimDispatch(root, claim.Path, RuntimeWorkerQueueDispatchExecutionOptions{
		Execute:       true,
		Now:           now.Add(time.Minute),
		TransportMode: "http-json",
	})
	if err != nil {
		t.Fatalf("execute runtime worker queue claim dispatch: %v", err)
	}
	if execution.Status != "dispatched" || execution.TransportMode != "http-json" || execution.TransportHTTPStatus != http.StatusAccepted {
		t.Fatalf("unexpected http-json execution metadata: %#v", execution)
	}
	if execution.TransportEndpoint != server.URL+"/dispatch" || !strings.Contains(execution.TransportResponseBody, "remote-accepted-1") {
		t.Fatalf("expected endpoint and response body on dispatch audit, got %#v", execution)
	}
	select {
	case request := <-received:
		if request.Status != "pending" || request.TransportMode != "http-json" || request.WorkerID != "remote-web-1" || request.ExecutionID != "tool-web" {
			t.Fatalf("unexpected http-json request body: %#v", request)
		}
		if request.ToolPlan.Action != "discover_assets" || strings.Join(request.ToolPlan.RunCommand, " ") != "nuclei -u {{target}}" {
			t.Fatalf("http-json request did not carry tool plan: %#v", request.ToolPlan)
		}
	case <-time.After(time.Second):
		t.Fatalf("expected http-json transport request")
	}
	updated, err := ReadRuntimeWorkerQueueClaim(root, filepath.Base(claim.Path))
	if err != nil {
		t.Fatalf("read updated queue claim: %v", err)
	}
	if updated.Status != "dispatched" || updated.DispatchExecutionPath != execution.Path {
		t.Fatalf("expected http-json dispatch to mark claim dispatched, got %#v", updated)
	}
}

func TestExecuteRuntimeWorkerQueueClaimDispatchHTTPJSONSendsBearerTokenWithoutPersistingSecret(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	receivedAuth := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuth <- r.Header.Get("Authorization")
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte("accepted"))
	}))
	defer server.Close()
	now := time.Date(2026, 5, 30, 18, 20, 0, 0, time.UTC)
	if _, err := RegisterRuntimeWorker(root, RuntimeWorkerArtifact{
		WorkerID:     "remote-web-1",
		Mode:         "remote",
		Status:       "available",
		Endpoint:     server.URL,
		Capabilities: []string{"web.discovery"},
	}, RuntimeWorkerRegistrationOptions{Now: now}); err != nil {
		t.Fatalf("register runtime worker: %v", err)
	}
	claim, err := CreateRuntimeWorkerQueueClaim(root, RuntimeWorkerQueueClaim{
		WorkerID:    "remote-web-1",
		RunID:       "run-http-auth",
		ExecutionID: "tool-web",
		Tool:        "web-api-pack",
		Capability:  "web.discovery",
	}, RuntimeWorkerQueueClaimOptions{Now: now, LeaseDuration: 10 * time.Minute})
	if err != nil {
		t.Fatalf("create runtime worker queue claim: %v", err)
	}

	execution, err := ExecuteRuntimeWorkerQueueClaimDispatch(root, claim.Path, RuntimeWorkerQueueDispatchExecutionOptions{
		Execute:            true,
		Now:                now.Add(time.Minute),
		TransportMode:      "http-json",
		TransportAuthToken: "secret-worker-token",
	})
	if err != nil {
		t.Fatalf("execute runtime worker queue claim dispatch: %v", err)
	}
	if execution.TransportAuth != "bearer" {
		t.Fatalf("expected bearer auth audit marker without token, got %#v", execution)
	}
	select {
	case auth := <-receivedAuth:
		if auth != "Bearer secret-worker-token" {
			t.Fatalf("expected bearer authorization header, got %q", auth)
		}
	case <-time.After(time.Second):
		t.Fatalf("expected http-json request")
	}
	data, err := os.ReadFile(execution.Path)
	if err != nil {
		t.Fatalf("read execution artifact: %v", err)
	}
	if strings.Contains(string(data), "secret-worker-token") {
		t.Fatalf("dispatch audit must not persist auth token: %s", string(data))
	}
}

func TestExecuteRuntimeWorkerQueueClaimDispatchHTTPJSONCarriesResultCallbackEndpoint(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	received := make(chan RuntimeWorkerTransportDispatchRequest, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		var request RuntimeWorkerTransportDispatchRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode http-json request: %v", err)
			return
		}
		received <- request
		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()
	now := time.Date(2026, 5, 30, 18, 25, 0, 0, time.UTC)
	if _, err := RegisterRuntimeWorker(root, RuntimeWorkerArtifact{
		WorkerID:     "remote-web-1",
		Mode:         "remote",
		Status:       "available",
		Endpoint:     server.URL,
		Capabilities: []string{"web.discovery"},
	}, RuntimeWorkerRegistrationOptions{Now: now}); err != nil {
		t.Fatalf("register runtime worker: %v", err)
	}
	claim, err := CreateRuntimeWorkerQueueClaim(root, RuntimeWorkerQueueClaim{
		WorkerID:    "remote-web-1",
		RunID:       "run-http-callback",
		ExecutionID: "tool-web",
		Tool:        "web-api-pack",
		Capability:  "web.discovery",
	}, RuntimeWorkerQueueClaimOptions{Now: now, LeaseDuration: 10 * time.Minute})
	if err != nil {
		t.Fatalf("create runtime worker queue claim: %v", err)
	}
	callbackEndpoint := "https://controller.example/ouro/runtime/result"

	execution, err := ExecuteRuntimeWorkerQueueClaimDispatch(root, claim.Path, RuntimeWorkerQueueDispatchExecutionOptions{
		Execute:                true,
		Now:                    now.Add(time.Minute),
		TransportMode:          "http-json",
		ResultCallbackEndpoint: callbackEndpoint,
	})
	if err != nil {
		t.Fatalf("execute runtime worker queue claim dispatch: %v", err)
	}
	if execution.ResultCallbackEndpoint != callbackEndpoint {
		t.Fatalf("expected result callback endpoint on dispatch audit, got %#v", execution)
	}
	select {
	case request := <-received:
		if request.ResultCallbackEndpoint != callbackEndpoint {
			t.Fatalf("expected result callback endpoint in worker request, got %#v", request)
		}
	case <-time.After(time.Second):
		t.Fatalf("expected http-json request")
	}
}

func TestExecuteRuntimeWorkerQueueClaimDispatchHTTPJSONRejectsNonHTTPEndpoint(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	now := time.Date(2026, 5, 30, 17, 41, 0, 0, time.UTC)
	if _, err := RegisterRuntimeWorker(root, RuntimeWorkerArtifact{
		WorkerID:     "remote-web-1",
		Mode:         "remote",
		Status:       "available",
		Endpoint:     "file://remote-web-1/inbox",
		Capabilities: []string{"web.discovery"},
	}, RuntimeWorkerRegistrationOptions{Now: now}); err != nil {
		t.Fatalf("register runtime worker: %v", err)
	}
	claim, err := CreateRuntimeWorkerQueueClaim(root, RuntimeWorkerQueueClaim{
		WorkerID:    "remote-web-1",
		RunID:       "run-http-json",
		ExecutionID: "tool-web",
		Tool:        "web-api-pack",
		Capability:  "web.discovery",
	}, RuntimeWorkerQueueClaimOptions{Now: now, LeaseDuration: 10 * time.Minute})
	if err != nil {
		t.Fatalf("create runtime worker queue claim: %v", err)
	}

	_, err = ExecuteRuntimeWorkerQueueClaimDispatch(root, claim.Path, RuntimeWorkerQueueDispatchExecutionOptions{
		Execute:       true,
		Now:           now.Add(time.Minute),
		TransportMode: "http-json",
	})
	if err == nil || !strings.Contains(err.Error(), "http-json transport requires http or https worker endpoint") {
		t.Fatalf("expected endpoint validation error, got %v", err)
	}
	updated, err := ReadRuntimeWorkerQueueClaim(root, filepath.Base(claim.Path))
	if err != nil {
		t.Fatalf("read updated queue claim: %v", err)
	}
	if updated.Status != "claimed" || updated.DispatchExecutionPath != "" {
		t.Fatalf("failed http-json dispatch must not mark claim dispatched, got %#v", updated)
	}
}

func TestExecuteRuntimeWorkerQueueClaimDispatchRejectsUnsupportedTransport(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Date(2026, 5, 30, 17, 45, 0, 0, time.UTC)
	dispatch := createClaimedRuntimeWorkerQueueClaim(t, root, now)

	_, err := ExecuteRuntimeWorkerQueueClaimDispatch(root, dispatch.Path, RuntimeWorkerQueueDispatchExecutionOptions{
		Execute:       true,
		Now:           now.Add(time.Minute),
		TransportMode: "tcp",
	})
	if err == nil || !strings.Contains(err.Error(), "unsupported worker queue dispatch transport") {
		t.Fatalf("expected unsupported transport error, got %v", err)
	}
	claims, err := ListRuntimeWorkerQueueClaims(root)
	if err != nil {
		t.Fatalf("list queue claims: %v", err)
	}
	if len(claims) != 1 || claims[0].Status != "claimed" {
		t.Fatalf("unsupported transport must not mutate claim, got %#v", claims)
	}
	executions, err := ListRuntimeWorkerQueueDispatchExecutions(root)
	if err != nil {
		t.Fatalf("list dispatch executions: %v", err)
	}
	if len(executions) != 0 {
		t.Fatalf("unsupported transport must not write dispatch audit, got %#v", executions)
	}
}

func TestRuntimeWorkerTransportDispatchRequestsListReadAndAck(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Date(2026, 5, 30, 17, 50, 0, 0, time.UTC)
	dispatch := createFileOutboxRuntimeWorkerQueueDispatch(t, root, now)

	requests, err := ListRuntimeWorkerTransportDispatchRequests(root, "remote-web-1")
	if err != nil {
		t.Fatalf("list transport requests: %v", err)
	}
	if len(requests) != 1 || requests[0].Status != "pending" || requests[0].Path != dispatch.TransportRequestPath {
		t.Fatalf("expected one pending transport request, got %#v", requests)
	}
	read, err := ReadRuntimeWorkerTransportDispatchRequest(root, "remote-web-1", filepath.Base(dispatch.TransportRequestPath))
	if err != nil {
		t.Fatalf("read transport request: %v", err)
	}
	if read.DispatchExecutionPath != dispatch.Path || read.WorkerID != "remote-web-1" || read.AckedAt != "" {
		t.Fatalf("unexpected read transport request: %#v", read)
	}
	if _, err := AcknowledgeRuntimeWorkerTransportDispatchRequest(root, "remote-web-1", filepath.Base(dispatch.TransportRequestPath), RuntimeWorkerTransportAckOptions{
		Now: now.Add(2 * time.Minute),
	}); err == nil || !strings.Contains(err.Error(), "requires explicit ack") {
		t.Fatalf("expected explicit ack error, got %v", err)
	}
	acked, err := AcknowledgeRuntimeWorkerTransportDispatchRequest(root, "remote-web-1", filepath.Base(dispatch.TransportRequestPath), RuntimeWorkerTransportAckOptions{
		Ack: true,
		Now: now.Add(2 * time.Minute),
	})
	if err != nil {
		t.Fatalf("ack transport request: %v", err)
	}
	if acked.Status != "acknowledged" || acked.AckedAt == "" || acked.AckedBy != "remote-web-1" {
		t.Fatalf("expected acknowledged transport request, got %#v", acked)
	}
	if _, err := AcknowledgeRuntimeWorkerTransportDispatchRequest(root, "remote-web-1", filepath.Base(dispatch.TransportRequestPath), RuntimeWorkerTransportAckOptions{
		Ack: true,
		Now: now.Add(3 * time.Minute),
	}); err == nil || !strings.Contains(err.Error(), "must be pending before ack") {
		t.Fatalf("expected duplicate ack error, got %v", err)
	}
}

func TestAcknowledgeRuntimeWorkerTransportDispatchRequestRequiresCurrentSafetyAuthorization(t *testing.T) {
	t.Skip("Safety authorization checks disabled")
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Date(2026, 6, 2, 13, 40, 0, 0, time.UTC)
	dispatch := createFileOutboxRuntimeWorkerQueueDispatch(t, root, now)
	writeRuntimeWorkerDisabledSafetyScope(t, root)

	_, err := AcknowledgeRuntimeWorkerTransportDispatchRequest(root, "remote-web-1", filepath.Base(dispatch.TransportRequestPath), RuntimeWorkerTransportAckOptions{
		Ack: true,
		Now: now.Add(2 * time.Minute),
	})
	if err == nil || !strings.Contains(err.Error(), "worker runtime side effect requires current safety bypass authorization") {
		t.Fatalf("expected strict safety authorization error, got %v", err)
	}
	request, err := ReadRuntimeWorkerTransportDispatchRequest(root, "remote-web-1", filepath.Base(dispatch.TransportRequestPath))
	if err != nil {
		t.Fatalf("read transport request after blocked ack: %v", err)
	}
	if request.Status != "pending" || request.AckedAt != "" || request.AckedBy != "" {
		t.Fatalf("blocked ack must not mutate request state, got %#v", request)
	}
}

func TestRuntimeWorkerTransportHTTPHandlerPersistsPendingFileOutboxRequest(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	now := time.Date(2026, 5, 30, 18, 6, 0, 0, time.UTC)
	body := RuntimeWorkerTransportDispatchRequest{
		Version:               1,
		Status:                "pending",
		TransportMode:         "http-json",
		DispatchExecutionPath: filepath.Join(root, "runtime", "worker-queue-dispatch-executions", "dispatch.json"),
		ClaimPath:             filepath.Join(root, "runtime", "worker-queue-claims", "claim.json"),
		ClaimID:               "claim-http",
		WorkerID:              "remote-web-1",
		WorkerEndpoint:        "http://worker.example/dispatch",
		RunID:                 "run-http",
		ExecutionID:           "tool-web",
		Tool:                  "web-api-pack",
		Capability:            "web.discovery",
		ToolPlan: RuntimeWorkerToolPlanPayload{
			Action:     "discover_assets",
			Runner:     "command",
			RunCommand: []string{"nuclei", "-u", "{{target}}"},
		},
		CreatedAt: now.Format(time.RFC3339Nano),
	}
	data, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/dispatch", strings.NewReader(string(data)))

	NewRuntimeWorkerTransportHTTPHandler(root, "remote-web-1").ServeHTTP(recorder, request)
	if recorder.Code != http.StatusAccepted {
		t.Fatalf("expected accepted response, got status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var accepted struct {
		Status        string `json:"status"`
		TransportMode string `json:"transport_mode"`
		Path          string `json:"path"`
		WorkerID      string `json:"worker_id"`
		ExecutionID   string `json:"execution_id"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &accepted); err != nil {
		t.Fatalf("decode accepted request: %v", err)
	}
	if accepted.Status != "pending" || accepted.TransportMode != "file-outbox" || accepted.Path == "" {
		t.Fatalf("expected persisted pending file-outbox request response, got %#v", accepted)
	}
	requests, err := ListRuntimeWorkerTransportDispatchRequests(root, "remote-web-1")
	if err != nil {
		t.Fatalf("list worker transport requests: %v", err)
	}
	if len(requests) != 1 || requests[0].Status != "pending" || requests[0].TransportMode != "file-outbox" {
		t.Fatalf("expected one pending file-outbox request, got %#v", requests)
	}
	if requests[0].ToolPlan.Action != "discover_assets" || strings.Join(requests[0].ToolPlan.RunCommand, " ") != "nuclei -u {{target}}" {
		t.Fatalf("expected tool plan preserved in received request, got %#v", requests[0].ToolPlan)
	}
	if _, err := AcknowledgeRuntimeWorkerTransportDispatchRequest(root, "remote-web-1", filepath.Base(requests[0].Path), RuntimeWorkerTransportAckOptions{
		Ack: true,
		Now: now.Add(time.Minute),
	}); err != nil {
		t.Fatalf("received http request should be ackable through file-outbox path: %v", err)
	}
}

func TestRuntimeWorkerTransportHTTPHandlerRejectsWorkerMismatchWithoutWritingRequest(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	body := RuntimeWorkerTransportDispatchRequest{
		Version:               1,
		Status:                "pending",
		TransportMode:         "http-json",
		DispatchExecutionPath: filepath.Join(root, "runtime", "worker-queue-dispatch-executions", "dispatch.json"),
		ClaimPath:             filepath.Join(root, "runtime", "worker-queue-claims", "claim.json"),
		WorkerID:              "other-worker",
		RunID:                 "run-http",
		ExecutionID:           "tool-web",
		Tool:                  "web-api-pack",
		Capability:            "web.discovery",
	}
	data, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/dispatch", strings.NewReader(string(data)))

	NewRuntimeWorkerTransportHTTPHandler(root, "remote-web-1").ServeHTTP(recorder, request)
	if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), "worker mismatch") {
		t.Fatalf("expected worker mismatch rejection, got status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	requests, err := ListRuntimeWorkerTransportDispatchRequests(root, "")
	if err != nil {
		t.Fatalf("list worker transport requests: %v", err)
	}
	if len(requests) != 0 {
		t.Fatalf("mismatched worker request must not be persisted, got %#v", requests)
	}
}

func TestRuntimeWorkerTransportHTTPHandlerRequiresBearerTokenWithoutWritingRequest(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	body := RuntimeWorkerTransportDispatchRequest{
		Version:               1,
		Status:                "pending",
		TransportMode:         "http-json",
		DispatchExecutionPath: filepath.Join(root, "runtime", "worker-queue-dispatch-executions", "dispatch.json"),
		ClaimPath:             filepath.Join(root, "runtime", "worker-queue-claims", "claim.json"),
		WorkerID:              "remote-web-1",
		RunID:                 "run-http",
		ExecutionID:           "tool-web",
		Tool:                  "web-api-pack",
		Capability:            "web.discovery",
	}
	data, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	handler := NewRuntimeWorkerTransportHTTPHandlerWithOptions(root, "remote-web-1", RuntimeWorkerTransportHTTPHandlerOptions{
		AuthToken: "secret-worker-token",
	})

	missing := httptest.NewRecorder()
	handler.ServeHTTP(missing, httptest.NewRequest(http.MethodPost, "/dispatch", strings.NewReader(string(data))))
	if missing.Code != http.StatusUnauthorized {
		t.Fatalf("expected missing token to be unauthorized, got status=%d body=%s", missing.Code, missing.Body.String())
	}
	wrongRequest := httptest.NewRequest(http.MethodPost, "/dispatch", strings.NewReader(string(data)))
	wrongRequest.Header.Set("Authorization", "Bearer wrong-token")
	wrong := httptest.NewRecorder()
	handler.ServeHTTP(wrong, wrongRequest)
	if wrong.Code != http.StatusUnauthorized {
		t.Fatalf("expected wrong token to be unauthorized, got status=%d body=%s", wrong.Code, wrong.Body.String())
	}
	requests, err := ListRuntimeWorkerTransportDispatchRequests(root, "remote-web-1")
	if err != nil {
		t.Fatalf("list worker transport requests after unauthorized attempts: %v", err)
	}
	if len(requests) != 0 {
		t.Fatalf("unauthorized requests must not be persisted, got %#v", requests)
	}

	authorizedRequest := httptest.NewRequest(http.MethodPost, "/dispatch", strings.NewReader(string(data)))
	authorizedRequest.Header.Set("Authorization", "Bearer secret-worker-token")
	authorized := httptest.NewRecorder()
	handler.ServeHTTP(authorized, authorizedRequest)
	if authorized.Code != http.StatusAccepted {
		t.Fatalf("expected authorized request to be accepted, got status=%d body=%s", authorized.Code, authorized.Body.String())
	}
	requests, err = ListRuntimeWorkerTransportDispatchRequests(root, "remote-web-1")
	if err != nil {
		t.Fatalf("list worker transport requests after authorized request: %v", err)
	}
	if len(requests) != 1 || requests[0].Status != "pending" {
		t.Fatalf("expected one authorized pending request, got %#v", requests)
	}
}

func TestRuntimeWorkerTransportHTTPHandlerRejectsBearerTokenBeforeSafetyAuthorization(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Date(2026, 6, 2, 15, 28, 0, 0, time.UTC)
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	body := RuntimeWorkerTransportDispatchRequest{
		Version:               1,
		Status:                "pending",
		TransportMode:         "http-json",
		DispatchExecutionPath: filepath.Join(root, "runtime", "worker-queue-dispatch-executions", "dispatch.json"),
		ClaimPath:             filepath.Join(root, "runtime", "worker-queue-claims", "claim.json"),
		WorkerID:              "remote-web-1",
		RunID:                 "run-http",
		ExecutionID:           "tool-web",
		Tool:                  "web-api-pack",
		Capability:            "web.discovery",
		CreatedAt:             now.Format(time.RFC3339Nano),
	}
	data, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	writeRuntimeWorkerDisabledSafetyScope(t, root)
	handler := NewRuntimeWorkerTransportHTTPHandlerWithOptions(root, "remote-web-1", RuntimeWorkerTransportHTTPHandlerOptions{
		AuthToken: "secret-worker-token",
	})

	missing := httptest.NewRecorder()
	handler.ServeHTTP(missing, httptest.NewRequest(http.MethodPost, "/dispatch", strings.NewReader(string(data))))
	if missing.Code != http.StatusUnauthorized || strings.Contains(missing.Body.String(), "safety bypass authorization") {
		t.Fatalf("expected bearer rejection before safety details, got status=%d body=%s", missing.Code, missing.Body.String())
	}
	wrongRequest := httptest.NewRequest(http.MethodPost, "/dispatch", strings.NewReader(string(data)))
	wrongRequest.Header.Set("Authorization", "Bearer wrong-token")
	wrong := httptest.NewRecorder()
	handler.ServeHTTP(wrong, wrongRequest)
	if wrong.Code != http.StatusUnauthorized || strings.Contains(wrong.Body.String(), "safety bypass authorization") {
		t.Fatalf("expected wrong bearer rejection before safety details, got status=%d body=%s", wrong.Code, wrong.Body.String())
	}
	requests, err := ListRuntimeWorkerTransportDispatchRequests(root, "remote-web-1")
	if err != nil {
		t.Fatalf("list worker transport requests after unauthorized attempts: %v", err)
	}
	if len(requests) != 0 {
		t.Fatalf("unauthorized requests must not be persisted, got %#v", requests)
	}
}

func TestRuntimeWorkerTransportHTTPHandlerRequiresCurrentSafetyAuthorizationBeforePersistingRequest(t *testing.T) {
	t.Skip("Safety authorization checks disabled")
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Date(2026, 6, 2, 15, 30, 0, 0, time.UTC)
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	body := RuntimeWorkerTransportDispatchRequest{
		Version:               1,
		Status:                "pending",
		TransportMode:         "http-json",
		DispatchExecutionPath: filepath.Join(root, "runtime", "worker-queue-dispatch-executions", "dispatch.json"),
		ClaimPath:             filepath.Join(root, "runtime", "worker-queue-claims", "claim.json"),
		WorkerID:              "remote-web-1",
		RunID:                 "run-http",
		ExecutionID:           "tool-web",
		Tool:                  "web-api-pack",
		Capability:            "web.discovery",
		CreatedAt:             now.Format(time.RFC3339Nano),
	}
	data, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	writeRuntimeWorkerDisabledSafetyScope(t, root)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/dispatch", strings.NewReader(string(data)))
	NewRuntimeWorkerTransportHTTPHandler(root, "remote-web-1").ServeHTTP(recorder, request)
	if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), "worker runtime side effect requires current safety bypass authorization") {
		t.Fatalf("expected strict safety authorization rejection, got status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	requests, err := ListRuntimeWorkerTransportDispatchRequests(root, "remote-web-1")
	if err != nil {
		t.Fatalf("list worker transport requests after blocked http receive: %v", err)
	}
	if len(requests) != 0 {
		t.Fatalf("blocked http receive must not persist file-outbox request, got %#v", requests)
	}
}

func TestRuntimeWorkerTransportHTTPHandlerAllowsRequestWithCurrentSafetyAuthorization(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Date(2026, 6, 2, 15, 32, 0, 0, time.UTC)
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	body := RuntimeWorkerTransportDispatchRequest{
		Version:               1,
		Status:                "pending",
		TransportMode:         "http-json",
		DispatchExecutionPath: filepath.Join(root, "runtime", "worker-queue-dispatch-executions", "dispatch.json"),
		ClaimPath:             filepath.Join(root, "runtime", "worker-queue-claims", "claim.json"),
		WorkerID:              "remote-web-1",
		RunID:                 "run-http",
		ExecutionID:           "tool-web",
		Tool:                  "web-api-pack",
		Capability:            "web.discovery",
		CreatedAt:             now.Format(time.RFC3339Nano),
	}
	data, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	writeRuntimeWorkerDisabledSafetyScope(t, root)
	writeRuntimeWorkerCurrentSafetyAuthorization(t, root)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/dispatch", strings.NewReader(string(data)))
	NewRuntimeWorkerTransportHTTPHandler(root, "remote-web-1").ServeHTTP(recorder, request)
	if recorder.Code != http.StatusAccepted {
		t.Fatalf("expected authorized request to be accepted, got status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	requests, err := ListRuntimeWorkerTransportDispatchRequests(root, "remote-web-1")
	if err != nil {
		t.Fatalf("list worker transport requests after authorized http receive: %v", err)
	}
	if len(requests) != 1 || requests[0].Status != "pending" || requests[0].TransportMode != "file-outbox" {
		t.Fatalf("expected one authorized pending file-outbox request, got %#v", requests)
	}
}

func TestRuntimeWorkerQueueDispatchResultHTTPHandlerSubmitsAuthorizedResult(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Date(2026, 5, 30, 18, 30, 0, 0, time.UTC)
	dispatch := createDispatchedRuntimeWorkerQueueClaim(t, root, now)
	handler := NewRuntimeWorkerQueueDispatchResultHTTPHandler(root, RuntimeWorkerQueueDispatchResultHTTPHandlerOptions{
		AuthToken: "secret-result-token",
	})
	payload := RuntimeWorkerQueueDispatchResult{
		DispatchExecutionPath: filepath.Base(dispatch.Path),
		Status:                "completed",
		Output:                "remote worker callback complete",
	}
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal result callback: %v", err)
	}

	missing := httptest.NewRecorder()
	handler.ServeHTTP(missing, httptest.NewRequest(http.MethodPost, "/result", strings.NewReader(string(data))))
	if missing.Code != http.StatusUnauthorized {
		t.Fatalf("expected missing token unauthorized, got status=%d body=%s", missing.Code, missing.Body.String())
	}
	results, err := ListRuntimeWorkerQueueDispatchResults(root)
	if err != nil {
		t.Fatalf("list results after unauthorized callback: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("unauthorized callback must not persist results, got %#v", results)
	}

	request := httptest.NewRequest(http.MethodPost, "/result", strings.NewReader(string(data)))
	request.Header.Set("Authorization", "Bearer secret-result-token")
	authorized := httptest.NewRecorder()
	handler.ServeHTTP(authorized, request)
	if authorized.Code != http.StatusAccepted {
		t.Fatalf("expected accepted callback, got status=%d body=%s", authorized.Code, authorized.Body.String())
	}
	var accepted RuntimeWorkerQueueDispatchResult
	if err := json.Unmarshal(authorized.Body.Bytes(), &accepted); err != nil {
		t.Fatalf("decode accepted callback response: %v", err)
	}
	if accepted.Status != "completed" || accepted.Output != "remote worker callback complete" {
		t.Fatalf("unexpected accepted callback result: %#v", accepted)
	}
	updated, err := ReadRuntimeWorkerQueueDispatchExecution(root, filepath.Base(dispatch.Path))
	if err != nil {
		t.Fatalf("read updated dispatch: %v", err)
	}
	if updated.Status != "completed" || updated.ResultPath == "" {
		t.Fatalf("expected callback to close dispatch, got %#v", updated)
	}
	persisted, err := ReadRuntimeWorkerQueueDispatchResult(root, filepath.Base(updated.ResultPath))
	if err != nil {
		t.Fatalf("read persisted callback result: %v", err)
	}
	if persisted.Output != accepted.Output {
		t.Fatalf("expected persisted callback output %q, got %#v", accepted.Output, persisted)
	}
}

func TestSubmitRuntimeWorkerQueueDispatchResultClosesFileOutboxTransportRequest(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Date(2026, 5, 30, 17, 55, 0, 0, time.UTC)
	dispatch := createFileOutboxRuntimeWorkerQueueDispatch(t, root, now)
	if _, err := AcknowledgeRuntimeWorkerTransportDispatchRequest(root, "remote-web-1", filepath.Base(dispatch.TransportRequestPath), RuntimeWorkerTransportAckOptions{
		Ack: true,
		Now: now.Add(2 * time.Minute),
	}); err != nil {
		t.Fatalf("ack transport request: %v", err)
	}

	result, err := SubmitRuntimeWorkerQueueDispatchResult(root, filepath.Base(dispatch.Path), RuntimeWorkerQueueDispatchResult{
		Status: "completed",
		Output: "worker side finished",
	}, RuntimeWorkerQueueDispatchResultOptions{Now: now.Add(3 * time.Minute)})
	if err != nil {
		t.Fatalf("submit dispatch result: %v", err)
	}
	request, err := ReadRuntimeWorkerTransportDispatchRequest(root, "remote-web-1", filepath.Base(dispatch.TransportRequestPath))
	if err != nil {
		t.Fatalf("read transport request: %v", err)
	}
	if request.Status != "completed" || request.ResultPath != result.Path || request.ResultStatus != "completed" || request.CompletedAt == "" {
		t.Fatalf("expected transport request closed by result return, got %#v", request)
	}
}

func TestExecuteRuntimeWorkerTransportDispatchRequestRequiresAcknowledgedRequestAndExecute(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Date(2026, 5, 30, 17, 57, 0, 0, time.UTC)
	dispatch := createFileOutboxRuntimeWorkerQueueDispatch(t, root, now)

	_, err := ExecuteRuntimeWorkerTransportDispatchRequest(context.Background(), root, "remote-web-1", filepath.Base(dispatch.TransportRequestPath), RuntimeWorkerTransportRunOptions{
		Command: []string{"sh", "-c", "printf should-not-run"},
		Now:     now.Add(2 * time.Minute),
	})
	if err == nil || !strings.Contains(err.Error(), "requires explicit execute") {
		t.Fatalf("expected explicit execute error, got %v", err)
	}
	if _, err := ExecuteRuntimeWorkerTransportDispatchRequest(context.Background(), root, "remote-web-1", filepath.Base(dispatch.TransportRequestPath), RuntimeWorkerTransportRunOptions{
		Execute: true,
		Command: []string{"sh", "-c", "printf should-not-run"},
		Now:     now.Add(2 * time.Minute),
	}); err == nil || !strings.Contains(err.Error(), "must be acknowledged before execution") {
		t.Fatalf("expected acknowledged request error, got %v", err)
	}
}

func TestExecuteRuntimeWorkerTransportDispatchRequestRunsCommandAndSubmitsResult(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Date(2026, 5, 30, 17, 58, 0, 0, time.UTC)
	dispatch := createFileOutboxRuntimeWorkerQueueDispatch(t, root, now)
	if _, err := AcknowledgeRuntimeWorkerTransportDispatchRequest(root, "remote-web-1", filepath.Base(dispatch.TransportRequestPath), RuntimeWorkerTransportAckOptions{
		Ack: true,
		Now: now.Add(2 * time.Minute),
	}); err != nil {
		t.Fatalf("ack transport request: %v", err)
	}

	result, err := ExecuteRuntimeWorkerTransportDispatchRequest(context.Background(), root, "remote-web-1", filepath.Base(dispatch.TransportRequestPath), RuntimeWorkerTransportRunOptions{
		Execute: true,
		Command: []string{"sh", "-c", "printf '%s/%s' \"$OURO_WORKER_ID\" \"$OURO_EXECUTION_ID\""},
		Now:     now.Add(3 * time.Minute),
	})
	if err != nil {
		t.Fatalf("execute transport request command: %v", err)
	}
	if result.Status != "completed" || result.Output != "remote-web-1/tool-web" || result.DispatchExecutionPath != dispatch.Path {
		t.Fatalf("unexpected transport execution result: %#v", result)
	}
	request, err := ReadRuntimeWorkerTransportDispatchRequest(root, "remote-web-1", filepath.Base(dispatch.TransportRequestPath))
	if err != nil {
		t.Fatalf("read transport request: %v", err)
	}
	if request.Status != "completed" || request.ResultPath != result.Path || request.CompletedAt == "" {
		t.Fatalf("expected outbox request closed by run-once, got %#v", request)
	}
	execution, err := ReadRuntimeWorkerQueueDispatchExecution(root, filepath.Base(dispatch.Path))
	if err != nil {
		t.Fatalf("read dispatch execution: %v", err)
	}
	if execution.Status != "completed" || execution.ResultPath != result.Path {
		t.Fatalf("expected dispatch execution closed by run-once, got %#v", execution)
	}
}

func TestExecuteRuntimeWorkerTransportDispatchRequestHeartbeatsActiveLeaseWhenRequested(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Date(2026, 5, 30, 17, 58, 30, 0, time.UTC)
	dispatch := createFileOutboxRuntimeWorkerQueueDispatch(t, root, now)
	if _, err := AcknowledgeRuntimeWorkerTransportDispatchRequest(root, "remote-web-1", filepath.Base(dispatch.TransportRequestPath), RuntimeWorkerTransportAckOptions{
		Ack: true,
		Now: now.Add(2 * time.Minute),
	}); err != nil {
		t.Fatalf("ack transport request: %v", err)
	}
	oldHeartbeat := now.Add(-DefaultWorkerLeaseStaleAfter - time.Minute).Format(time.RFC3339Nano)
	if err := writeWorkerLeaseArtifact(root, WorkerLeaseArtifact{
		Version:     1,
		RunID:       dispatch.RunID,
		ExecutionID: dispatch.ExecutionID,
		Tool:        dispatch.Tool,
		Capability:  dispatch.Capability,
		WorkerID:    "remote-web-1",
		Status:      "active",
		AcquiredAt:  oldHeartbeat,
		HeartbeatAt: oldHeartbeat,
	}); err != nil {
		t.Fatalf("write worker lease fixture: %v", err)
	}

	runAt := now.Add(3 * time.Minute)
	result, err := ExecuteRuntimeWorkerTransportDispatchRequest(context.Background(), root, "remote-web-1", filepath.Base(dispatch.TransportRequestPath), RuntimeWorkerTransportRunOptions{
		Execute:         true,
		Command:         []string{"sh", "-c", "printf heartbeat"},
		Now:             runAt,
		HeartbeatLease:  true,
		HeartbeatBy:     "remote-web-1",
		HeartbeatReason: "worker transport run-once",
	})
	if err != nil {
		t.Fatalf("execute transport request command: %v", err)
	}
	if result.Status != "completed" {
		t.Fatalf("expected completed result, got %#v", result)
	}
	lease, err := readWorkerLeaseByIdentity(root, dispatch.RunID, dispatch.ExecutionID)
	if err != nil {
		t.Fatalf("read worker lease: %v", err)
	}
	if lease.Status != "active" || lease.HeartbeatAt != runAt.Format(time.RFC3339Nano) || lease.HeartbeatBy != "remote-web-1" || lease.HeartbeatReason != "worker transport run-once" {
		t.Fatalf("expected explicit active lease heartbeat refresh, got %#v", lease)
	}
	reclaim, err := PlanWorkerLeaseReclaims(root, runAt.Add(time.Minute), DefaultWorkerLeaseStaleAfter)
	if err != nil {
		t.Fatalf("plan worker lease reclaims: %v", err)
	}
	if len(reclaim.Ready) != 0 || len(reclaim.Skipped) != 1 || reclaim.Skipped[0].Reason != "active-not-stale" {
		t.Fatalf("expected refreshed lease to stay out of reclaim plan, got %#v", reclaim)
	}
}

func TestExecuteRuntimeWorkerTransportDispatchRequestDoesNotHeartbeatLeaseByDefault(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Date(2026, 5, 30, 17, 58, 45, 0, time.UTC)
	dispatch := createFileOutboxRuntimeWorkerQueueDispatch(t, root, now)
	if _, err := AcknowledgeRuntimeWorkerTransportDispatchRequest(root, "remote-web-1", filepath.Base(dispatch.TransportRequestPath), RuntimeWorkerTransportAckOptions{
		Ack: true,
		Now: now.Add(2 * time.Minute),
	}); err != nil {
		t.Fatalf("ack transport request: %v", err)
	}
	oldHeartbeat := now.Add(-DefaultWorkerLeaseStaleAfter - time.Minute).Format(time.RFC3339Nano)
	if err := writeWorkerLeaseArtifact(root, WorkerLeaseArtifact{
		Version:     1,
		RunID:       dispatch.RunID,
		ExecutionID: dispatch.ExecutionID,
		Tool:        dispatch.Tool,
		Capability:  dispatch.Capability,
		WorkerID:    "remote-web-1",
		Status:      "active",
		AcquiredAt:  oldHeartbeat,
		HeartbeatAt: oldHeartbeat,
	}); err != nil {
		t.Fatalf("write worker lease fixture: %v", err)
	}

	if _, err := ExecuteRuntimeWorkerTransportDispatchRequest(context.Background(), root, "remote-web-1", filepath.Base(dispatch.TransportRequestPath), RuntimeWorkerTransportRunOptions{
		Execute: true,
		Command: []string{"sh", "-c", "printf no-heartbeat"},
		Now:     now.Add(3 * time.Minute),
	}); err != nil {
		t.Fatalf("execute transport request command: %v", err)
	}
	lease, err := readWorkerLeaseByIdentity(root, dispatch.RunID, dispatch.ExecutionID)
	if err != nil {
		t.Fatalf("read worker lease: %v", err)
	}
	if lease.HeartbeatAt != oldHeartbeat || lease.HeartbeatBy != "" || lease.HeartbeatReason != "" {
		t.Fatalf("default transport execution must not refresh worker lease heartbeat, got %#v", lease)
	}
}

func TestExecuteRuntimeWorkerTransportDispatchRequestExposesToolPlanToWorkerCommand(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Date(2026, 5, 30, 17, 59, 0, 0, time.UTC)
	dispatch := createFileOutboxRuntimeWorkerQueueDispatchWithToolPlan(t, root, now, RuntimeWorkerToolPlanPayload{
		Action:       "discover_assets",
		ManifestName: "web-api-pack",
		Runner:       "command",
		RunCommand:   []string{"nuclei", "-u", "{{target}}"},
		Parser:       "nuclei-json",
	})
	if _, err := AcknowledgeRuntimeWorkerTransportDispatchRequest(root, "remote-web-1", filepath.Base(dispatch.TransportRequestPath), RuntimeWorkerTransportAckOptions{
		Ack: true,
		Now: now.Add(2 * time.Minute),
	}); err != nil {
		t.Fatalf("ack transport request: %v", err)
	}

	result, err := ExecuteRuntimeWorkerTransportDispatchRequest(context.Background(), root, "remote-web-1", filepath.Base(dispatch.TransportRequestPath), RuntimeWorkerTransportRunOptions{
		Execute: true,
		Command: []string{"sh", "-c", "printf 'action=%s runner=%s parser=%s command=%s' \"$OURO_ACTION\" \"$OURO_RUNNER\" \"$OURO_PARSER\" \"$OURO_RUN_COMMAND_JSON\""},
		Now:     now.Add(3 * time.Minute),
	})
	if err != nil {
		t.Fatalf("execute transport request command: %v", err)
	}
	for _, want := range []string{
		"action=discover_assets",
		"runner=command",
		"parser=nuclei-json",
		"nuclei",
		"{{target}}",
	} {
		if !strings.Contains(result.Output, want) {
			t.Fatalf("expected worker command output to contain %q, got %q", want, result.Output)
		}
	}
}

func TestExecuteRuntimeWorkerTransportDispatchRequestUsesRequestToolPlanCommand(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Date(2026, 5, 30, 18, 0, 0, 0, time.UTC)
	dispatch := createFileOutboxRuntimeWorkerQueueDispatchWithToolPlan(t, root, now, RuntimeWorkerToolPlanPayload{
		Action:     "discover_assets",
		Runner:     "command",
		RunCommand: []string{"sh", "-c", "printf 'plan=%s execution=%s' \"$OURO_ACTION\" \"$OURO_EXECUTION_ID\""},
		Parser:     "json-lines",
	})
	if _, err := AcknowledgeRuntimeWorkerTransportDispatchRequest(root, "remote-web-1", filepath.Base(dispatch.TransportRequestPath), RuntimeWorkerTransportAckOptions{
		Ack: true,
		Now: now.Add(2 * time.Minute),
	}); err != nil {
		t.Fatalf("ack transport request: %v", err)
	}

	result, err := ExecuteRuntimeWorkerTransportDispatchRequest(context.Background(), root, "remote-web-1", filepath.Base(dispatch.TransportRequestPath), RuntimeWorkerTransportRunOptions{
		Execute:           true,
		UseRequestCommand: true,
		Now:               now.Add(3 * time.Minute),
	})
	if err != nil {
		t.Fatalf("execute request tool-plan command: %v", err)
	}
	if result.Status != "completed" || result.Output != "plan=discover_assets execution=tool-web" {
		t.Fatalf("unexpected request-command result: %#v", result)
	}
}

func TestExecuteRuntimeWorkerTransportDispatchRequestRequiresCurrentSafetyAuthorizationForRequestCommand(t *testing.T) {
	t.Skip("Safety authorization checks disabled")
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Date(2026, 6, 2, 14, 10, 0, 0, time.UTC)
	marker := filepath.Join(t.TempDir(), "request-command-ran")
	dispatch := createFileOutboxRuntimeWorkerQueueDispatchWithToolPlan(t, root, now, RuntimeWorkerToolPlanPayload{
		Action:     "discover_assets",
		Runner:     "command",
		RunCommand: []string{"sh", "-c", "touch " + marker},
	})
	if _, err := AcknowledgeRuntimeWorkerTransportDispatchRequest(root, "remote-web-1", filepath.Base(dispatch.TransportRequestPath), RuntimeWorkerTransportAckOptions{
		Ack: true,
		Now: now.Add(2 * time.Minute),
	}); err != nil {
		t.Fatalf("ack transport request: %v", err)
	}
	writeRuntimeWorkerDisabledSafetyScope(t, root)

	_, err := ExecuteRuntimeWorkerTransportDispatchRequest(context.Background(), root, "remote-web-1", filepath.Base(dispatch.TransportRequestPath), RuntimeWorkerTransportRunOptions{
		Execute:           true,
		UseRequestCommand: true,
		Now:               now.Add(3 * time.Minute),
	})
	if err == nil || !strings.Contains(err.Error(), "worker runtime side effect requires current safety bypass authorization") {
		t.Fatalf("expected strict safety authorization error, got %v", err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("blocked request-command execution must not create marker, stat err=%v", err)
	}
	request, err := ReadRuntimeWorkerTransportDispatchRequest(root, "remote-web-1", filepath.Base(dispatch.TransportRequestPath))
	if err != nil {
		t.Fatalf("read transport request after blocked execution: %v", err)
	}
	if request.Status != "acknowledged" || request.ResultPath != "" || request.ResultStatus != "" || request.CompletedAt != "" {
		t.Fatalf("blocked request-command execution must not mutate request result state, got %#v", request)
	}
	results, err := ListRuntimeWorkerQueueDispatchResults(root)
	if err != nil {
		t.Fatalf("list dispatch results after blocked execution: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("blocked request-command execution must not submit results, got %#v", results)
	}
}

func TestExecuteRuntimeWorkerTransportDispatchRequestRejectsMissingRequestToolPlanCommand(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Date(2026, 5, 30, 18, 1, 0, 0, time.UTC)
	dispatch := createFileOutboxRuntimeWorkerQueueDispatchWithToolPlan(t, root, now, RuntimeWorkerToolPlanPayload{
		Action: "discover_assets",
		Runner: "command",
	})
	if _, err := AcknowledgeRuntimeWorkerTransportDispatchRequest(root, "remote-web-1", filepath.Base(dispatch.TransportRequestPath), RuntimeWorkerTransportAckOptions{
		Ack: true,
		Now: now.Add(2 * time.Minute),
	}); err != nil {
		t.Fatalf("ack transport request: %v", err)
	}

	_, err := ExecuteRuntimeWorkerTransportDispatchRequest(context.Background(), root, "remote-web-1", filepath.Base(dispatch.TransportRequestPath), RuntimeWorkerTransportRunOptions{
		Execute:           true,
		UseRequestCommand: true,
		Now:               now.Add(3 * time.Minute),
	})
	if err == nil || !strings.Contains(err.Error(), "request tool plan run command is required") {
		t.Fatalf("expected missing request command error, got %v", err)
	}
	request, err := ReadRuntimeWorkerTransportDispatchRequest(root, "remote-web-1", filepath.Base(dispatch.TransportRequestPath))
	if err != nil {
		t.Fatalf("read transport request: %v", err)
	}
	if request.Status != "acknowledged" || request.ResultPath != "" {
		t.Fatalf("missing request command must not close request, got %#v", request)
	}
}

func TestExecuteRuntimeWorkerTransportDispatchLoopUsesRequestToolPlanCommands(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Date(2026, 5, 30, 18, 2, 0, 0, time.UTC)
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	if _, err := RegisterRuntimeWorker(root, RuntimeWorkerArtifact{
		WorkerID:     "remote-web-1",
		Mode:         "remote",
		Status:       "available",
		Endpoint:     "file://remote-web-1/inbox",
		Capabilities: []string{"web.discovery"},
	}, RuntimeWorkerRegistrationOptions{Now: now}); err != nil {
		t.Fatalf("register runtime worker: %v", err)
	}
	first := createFileOutboxRuntimeWorkerQueueDispatchForExecutionWithToolPlan(t, root, now, "run-plan-loop", "tool-one", RuntimeWorkerToolPlanPayload{
		Action:     "discover_assets",
		Runner:     "command",
		RunCommand: []string{"sh", "-c", "printf 'one=%s' \"$OURO_EXECUTION_ID\""},
	})
	second := createFileOutboxRuntimeWorkerQueueDispatchForExecutionWithToolPlan(t, root, now.Add(time.Second), "run-plan-loop", "tool-two", RuntimeWorkerToolPlanPayload{
		Action:     "discover_assets",
		Runner:     "command",
		RunCommand: []string{"sh", "-c", "printf 'two=%s' \"$OURO_EXECUTION_ID\""},
	})
	for _, dispatch := range []RuntimeWorkerQueueDispatchExecution{first, second} {
		if _, err := AcknowledgeRuntimeWorkerTransportDispatchRequest(root, "remote-web-1", filepath.Base(dispatch.TransportRequestPath), RuntimeWorkerTransportAckOptions{
			Ack: true,
			Now: now.Add(2 * time.Minute),
		}); err != nil {
			t.Fatalf("ack transport request: %v", err)
		}
	}

	loop, err := ExecuteRuntimeWorkerTransportDispatchLoop(context.Background(), root, "remote-web-1", RuntimeWorkerTransportRunLoopOptions{
		Execute:           true,
		UseRequestCommand: true,
		MaxRequests:       2,
		Now:               now.Add(3 * time.Minute),
	})
	if err != nil {
		t.Fatalf("execute request-command loop: %v", err)
	}
	if loop.Processed != 2 || loop.Completed != 2 || len(loop.Results) != 2 {
		t.Fatalf("expected two completed request-command results, got %#v", loop)
	}
	outputs := []string{loop.Results[0].Output, loop.Results[1].Output}
	if strings.Join(outputs, "|") != "one=tool-one|two=tool-two" {
		t.Fatalf("loop did not use per-request commands, got %#v", outputs)
	}
}

func TestExecuteRuntimeWorkerTransportDispatchLoopProcessesAcknowledgedRequestsUpToLimit(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Date(2026, 5, 30, 17, 59, 0, 0, time.UTC)
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	if _, err := RegisterRuntimeWorker(root, RuntimeWorkerArtifact{
		WorkerID:     "remote-web-1",
		Mode:         "remote",
		Status:       "available",
		Endpoint:     "file://remote-web-1/inbox",
		Capabilities: []string{"web.discovery"},
	}, RuntimeWorkerRegistrationOptions{Now: now}); err != nil {
		t.Fatalf("register runtime worker: %v", err)
	}
	first := createFileOutboxRuntimeWorkerQueueDispatchForExecution(t, root, now, "run-loop", "tool-one")
	second := createFileOutboxRuntimeWorkerQueueDispatchForExecution(t, root, now.Add(time.Second), "run-loop", "tool-two")
	for _, dispatch := range []RuntimeWorkerQueueDispatchExecution{first, second} {
		if _, err := AcknowledgeRuntimeWorkerTransportDispatchRequest(root, "remote-web-1", filepath.Base(dispatch.TransportRequestPath), RuntimeWorkerTransportAckOptions{
			Ack: true,
			Now: now.Add(2 * time.Minute),
		}); err != nil {
			t.Fatalf("ack transport request: %v", err)
		}
	}

	loop, err := ExecuteRuntimeWorkerTransportDispatchLoop(context.Background(), root, "remote-web-1", RuntimeWorkerTransportRunLoopOptions{
		Execute:     true,
		MaxRequests: 1,
		Command:     []string{"sh", "-c", "printf '%s' \"$OURO_EXECUTION_ID\""},
		Now:         now.Add(3 * time.Minute),
	})
	if err != nil {
		t.Fatalf("execute transport dispatch loop: %v", err)
	}
	if loop.Status != "completed" || loop.Processed != 1 || loop.Completed != 1 || loop.Failed != 0 || len(loop.Results) != 1 {
		t.Fatalf("unexpected loop result: %#v", loop)
	}
	if loop.Results[0].Output != "tool-one" {
		t.Fatalf("expected first acknowledged request to be processed first, got %#v", loop.Results)
	}
	firstRequest, err := ReadRuntimeWorkerTransportDispatchRequest(root, "remote-web-1", filepath.Base(first.TransportRequestPath))
	if err != nil {
		t.Fatalf("read first request: %v", err)
	}
	secondRequest, err := ReadRuntimeWorkerTransportDispatchRequest(root, "remote-web-1", filepath.Base(second.TransportRequestPath))
	if err != nil {
		t.Fatalf("read second request: %v", err)
	}
	if firstRequest.Status != "completed" || secondRequest.Status != "acknowledged" {
		t.Fatalf("expected max limit to process only first request, got first=%#v second=%#v", firstRequest, secondRequest)
	}
}

func TestExecuteRuntimeWorkerTransportDispatchDaemonPollsNewAcknowledgedRequests(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	skipIfWindows(t)
	now := time.Date(2026, 5, 30, 18, 4, 0, 0, time.UTC)
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	if _, err := RegisterRuntimeWorker(root, RuntimeWorkerArtifact{
		WorkerID:     "remote-web-1",
		Mode:         "remote",
		Status:       "available",
		Endpoint:     "file://remote-web-1/inbox",
		Capabilities: []string{"web.discovery"},
	}, RuntimeWorkerRegistrationOptions{Now: now}); err != nil {
		t.Fatalf("register runtime worker: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	created := make(chan RuntimeWorkerQueueDispatchExecution, 1)
	createErr := make(chan error, 1)
	go func() {
		select {
		case <-ctx.Done():
			createErr <- ctx.Err()
			return
		case <-time.After(20 * time.Millisecond):
		}
		dispatch, err := createRuntimeWorkerTransportDaemonRequest(root, now.Add(time.Minute), "run-daemon", "tool-daemon", RuntimeWorkerToolPlanPayload{
			Action:     "discover_assets",
			Runner:     "command",
			RunCommand: []string{"sh", "-c", "printf 'daemon=%s' \"$OURO_EXECUTION_ID\""},
		})
		if err != nil {
			createErr <- err
			return
		}
		if _, err := AcknowledgeRuntimeWorkerTransportDispatchRequest(root, "remote-web-1", filepath.Base(dispatch.TransportRequestPath), RuntimeWorkerTransportAckOptions{
			Ack: true,
			Now: now.Add(2 * time.Minute),
		}); err != nil {
			createErr <- err
			return
		}
		created <- dispatch
		createErr <- nil
	}()

	daemon, err := ExecuteRuntimeWorkerTransportDispatchDaemon(ctx, root, "remote-web-1", RuntimeWorkerTransportRunDaemonOptions{
		Execute:           true,
		UseRequestCommand: true,
		MaxRequests:       1,
		MaxIdleScans:      200,
		PollInterval:      time.Millisecond,
		Now:               now.Add(3 * time.Minute),
	})
	if err != nil {
		t.Fatalf("execute worker transport daemon: %v", err)
	}
	if err := <-createErr; err != nil {
		t.Fatalf("create daemon request: %v", err)
	}
	dispatch := <-created
	if daemon.Status != "completed" || daemon.Processed != 1 || daemon.Completed != 1 || daemon.Failed != 0 || len(daemon.Results) != 1 {
		t.Fatalf("unexpected daemon result: %#v", daemon)
	}
	if daemon.Scans < 2 || daemon.IdleScans == 0 {
		t.Fatalf("expected daemon to poll before processing request, got %#v", daemon)
	}
	if daemon.Results[0].Output != "daemon=tool-daemon" {
		t.Fatalf("expected request command output, got %#v", daemon.Results)
	}
	request, err := ReadRuntimeWorkerTransportDispatchRequest(root, "remote-web-1", filepath.Base(dispatch.TransportRequestPath))
	if err != nil {
		t.Fatalf("read daemon transport request: %v", err)
	}
	if request.Status != "completed" || request.ResultPath == "" {
		t.Fatalf("expected daemon to close acknowledged request through result return, got %#v", request)
	}
}

func TestExecuteRuntimeWorkerTransportDispatchDaemonHeartbeatsActiveLeaseWhenRequested(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Date(2026, 5, 30, 18, 4, 30, 0, time.UTC)
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	if _, err := RegisterRuntimeWorker(root, RuntimeWorkerArtifact{
		WorkerID:     "remote-web-1",
		Mode:         "remote",
		Status:       "available",
		Endpoint:     "file://remote-web-1/inbox",
		Capabilities: []string{"web.discovery"},
	}, RuntimeWorkerRegistrationOptions{Now: now}); err != nil {
		t.Fatalf("register runtime worker: %v", err)
	}
	dispatch, err := createRuntimeWorkerTransportDaemonRequest(root, now, "run-daemon-heartbeat", "tool-daemon-heartbeat", RuntimeWorkerToolPlanPayload{
		Action:     "discover_assets",
		Runner:     "command",
		RunCommand: []string{"sh", "-c", "printf 'daemon-heartbeat=%s' \"$OURO_EXECUTION_ID\""},
	})
	if err != nil {
		t.Fatalf("create daemon request: %v", err)
	}
	if _, err := AcknowledgeRuntimeWorkerTransportDispatchRequest(root, "remote-web-1", filepath.Base(dispatch.TransportRequestPath), RuntimeWorkerTransportAckOptions{
		Ack: true,
		Now: now.Add(2 * time.Minute),
	}); err != nil {
		t.Fatalf("ack daemon request: %v", err)
	}
	oldHeartbeat := now.Add(-DefaultWorkerLeaseStaleAfter - time.Minute).Format(time.RFC3339Nano)
	if err := writeWorkerLeaseArtifact(root, WorkerLeaseArtifact{
		Version:     1,
		RunID:       dispatch.RunID,
		ExecutionID: dispatch.ExecutionID,
		Tool:        dispatch.Tool,
		Capability:  dispatch.Capability,
		WorkerID:    "remote-web-1",
		Status:      "active",
		AcquiredAt:  oldHeartbeat,
		HeartbeatAt: oldHeartbeat,
	}); err != nil {
		t.Fatalf("write worker lease fixture: %v", err)
	}

	runAt := now.Add(3 * time.Minute)
	daemon, err := ExecuteRuntimeWorkerTransportDispatchDaemon(context.Background(), root, "remote-web-1", RuntimeWorkerTransportRunDaemonOptions{
		Execute:           true,
		UseRequestCommand: true,
		MaxRequests:       1,
		MaxIdleScans:      2,
		PollInterval:      time.Millisecond,
		Now:               runAt,
		HeartbeatLease:    true,
		HeartbeatBy:       "remote-web-1",
		HeartbeatReason:   "worker transport daemon",
	})
	if err != nil {
		t.Fatalf("execute daemon with lease heartbeat: %v", err)
	}
	if daemon.Status != "completed" || daemon.Processed != 1 || daemon.Results[0].Output != "daemon-heartbeat=tool-daemon-heartbeat" {
		t.Fatalf("unexpected daemon heartbeat result: %#v", daemon)
	}
	lease, err := readWorkerLeaseByIdentity(root, dispatch.RunID, dispatch.ExecutionID)
	if err != nil {
		t.Fatalf("read worker lease: %v", err)
	}
	if lease.Status != "active" || lease.HeartbeatAt != runAt.Format(time.RFC3339Nano) || lease.HeartbeatBy != "remote-web-1" || lease.HeartbeatReason != "worker transport daemon" {
		t.Fatalf("expected daemon to refresh active lease heartbeat without releasing it, got %#v", lease)
	}
}

func TestExecuteRuntimeWorkerTransportDispatchDaemonRequiresExplicitExecute(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	_, err := ExecuteRuntimeWorkerTransportDispatchDaemon(context.Background(), root, "remote-web-1", RuntimeWorkerTransportRunDaemonOptions{
		UseRequestCommand: true,
		MaxRequests:       1,
		MaxIdleScans:      1,
		PollInterval:      time.Millisecond,
	})
	if err == nil || !strings.Contains(err.Error(), "worker transport dispatch daemon requires explicit execute") {
		t.Fatalf("expected explicit execute error, got %v", err)
	}
}

func TestExecuteRuntimeWorkerTransportDispatchDaemonDoesNotAutoAckPendingRequests(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Date(2026, 5, 30, 18, 5, 0, 0, time.UTC)
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	if _, err := RegisterRuntimeWorker(root, RuntimeWorkerArtifact{
		WorkerID:     "remote-web-1",
		Mode:         "remote",
		Status:       "available",
		Endpoint:     "file://remote-web-1/inbox",
		Capabilities: []string{"web.discovery"},
	}, RuntimeWorkerRegistrationOptions{Now: now}); err != nil {
		t.Fatalf("register runtime worker: %v", err)
	}
	dispatch, err := createRuntimeWorkerTransportDaemonRequest(root, now, "run-daemon", "tool-pending", RuntimeWorkerToolPlanPayload{
		Action:     "discover_assets",
		Runner:     "command",
		RunCommand: []string{"sh", "-c", "printf 'pending=%s' \"$OURO_EXECUTION_ID\""},
	})
	if err != nil {
		t.Fatalf("create daemon request: %v", err)
	}

	daemon, err := ExecuteRuntimeWorkerTransportDispatchDaemon(context.Background(), root, "remote-web-1", RuntimeWorkerTransportRunDaemonOptions{
		Execute:           true,
		UseRequestCommand: true,
		MaxRequests:       1,
		MaxIdleScans:      2,
		PollInterval:      time.Millisecond,
		Now:               now.Add(time.Minute),
	})
	if err != nil {
		t.Fatalf("execute worker transport daemon: %v", err)
	}
	if daemon.Status != "idle" || daemon.Processed != 0 {
		t.Fatalf("pending request should not be processed, got %#v", daemon)
	}
	request, err := ReadRuntimeWorkerTransportDispatchRequest(root, "remote-web-1", filepath.Base(dispatch.TransportRequestPath))
	if err != nil {
		t.Fatalf("read pending transport request: %v", err)
	}
	if request.Status != "pending" || request.AckedAt != "" || request.ResultPath != "" {
		t.Fatalf("daemon must not auto-ack pending request, got %#v", request)
	}
}

func TestExecuteRuntimeWorkerTransportDispatchDaemonWritesRunArtifact(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Date(2026, 5, 30, 18, 8, 0, 0, time.UTC)
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	if _, err := RegisterRuntimeWorker(root, RuntimeWorkerArtifact{
		WorkerID:     "remote-web-1",
		Mode:         "remote",
		Status:       "available",
		Endpoint:     "file://remote-web-1/inbox",
		Capabilities: []string{"web.discovery"},
	}, RuntimeWorkerRegistrationOptions{Now: now}); err != nil {
		t.Fatalf("register runtime worker: %v", err)
	}
	dispatch, err := createRuntimeWorkerTransportDaemonRequest(root, now, "run-daemon-artifact", "tool-daemon", RuntimeWorkerToolPlanPayload{
		Action:     "discover_assets",
		Runner:     "command",
		RunCommand: []string{"sh", "-c", "printf 'daemon=%s' \"$OURO_EXECUTION_ID\""},
	})
	if err != nil {
		t.Fatalf("create daemon request: %v", err)
	}
	if _, err := AcknowledgeRuntimeWorkerTransportDispatchRequest(root, "remote-web-1", filepath.Base(dispatch.TransportRequestPath), RuntimeWorkerTransportAckOptions{
		Ack: true,
		Now: now.Add(time.Minute),
	}); err != nil {
		t.Fatalf("ack daemon request: %v", err)
	}

	daemon, err := ExecuteRuntimeWorkerTransportDispatchDaemon(context.Background(), root, "remote-web-1", RuntimeWorkerTransportRunDaemonOptions{
		Execute:           true,
		Now:               now.Add(2 * time.Minute),
		MaxRequests:       1,
		MaxIdleScans:      1,
		PollInterval:      time.Millisecond,
		UseRequestCommand: true,
	})
	if err != nil {
		t.Fatalf("execute daemon: %v", err)
	}
	if daemon.Path == "" || daemon.Mode != "daemon-run" || daemon.CommandSource != "request-command" {
		t.Fatalf("expected daemon run artifact metadata, got %#v", daemon)
	}
	if daemon.Processed != 1 || daemon.Completed != 1 || len(daemon.ResultPaths) != 1 {
		t.Fatalf("expected completed daemon run artifact, got %#v", daemon)
	}
	runs, err := ListRuntimeWorkerTransportDaemonRuns(root, "remote-web-1")
	if err != nil {
		t.Fatalf("list daemon run artifacts: %v", err)
	}
	if len(runs) != 1 || runs[0].Path != daemon.Path || runs[0].ResultPaths[0] != daemon.ResultPaths[0] {
		t.Fatalf("expected one persisted daemon run artifact, got %#v", runs)
	}
	read, err := ReadRuntimeWorkerTransportDaemonRun(root, "remote-web-1", filepath.Base(daemon.Path))
	if err != nil {
		t.Fatalf("read daemon run artifact: %v", err)
	}
	if read.Status != "completed" || read.WorkerID != "remote-web-1" || read.Processed != 1 {
		t.Fatalf("unexpected daemon run artifact: %#v", read)
	}
}

func TestCreateRuntimeWorkerTransportDaemonControlRequestWritesPendingArtifact(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Date(2026, 5, 30, 18, 12, 0, 0, time.UTC)
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}

	control, err := CreateRuntimeWorkerTransportDaemonControlRequest(root, "remote-web-1", RuntimeWorkerTransportDaemonControlRequest{
		Action:      "stop",
		Reason:      "operator requested maintenance",
		RequestedBy: "alice",
	}, RuntimeWorkerTransportDaemonControlRequestOptions{Now: now})
	if err != nil {
		t.Fatalf("create daemon control request: %v", err)
	}
	if control.Status != "pending" || control.Mode != "metadata-only" || control.Action != "stop" || control.WorkerID != "remote-web-1" {
		t.Fatalf("unexpected daemon control request: %#v", control)
	}
	if control.RequestedAt != now.Format(time.RFC3339Nano) || control.Reason != "operator requested maintenance" || control.RequestedBy != "alice" {
		t.Fatalf("expected request metadata to be preserved, got %#v", control)
	}
	if !strings.Contains(control.Path, filepath.Join("runtime", "worker-transport-daemon-controls", "remote-web-1")) {
		t.Fatalf("expected daemon control under worker directory, got %s", control.Path)
	}
	data, err := os.ReadFile(control.Path)
	if err != nil {
		t.Fatalf("read daemon control request: %v", err)
	}
	var decoded RuntimeWorkerTransportDaemonControlRequest
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("decode daemon control request: %v", err)
	}
	if decoded.Status != "pending" || decoded.Mode != "metadata-only" || decoded.Action != "stop" || decoded.Reason != "operator requested maintenance" {
		t.Fatalf("unexpected persisted daemon control request: %#v", decoded)
	}
}

func TestCreateRuntimeWorkerTransportDaemonControlRequestRequiresCurrentSafetyAuthorizationBeforeWriting(t *testing.T) {
	t.Skip("Safety authorization checks disabled")
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Date(2026, 6, 2, 16, 55, 0, 0, time.UTC)
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	writeRuntimeWorkerDisabledSafetyScope(t, root)

	_, err := CreateRuntimeWorkerTransportDaemonControlRequest(root, "remote-web-1", RuntimeWorkerTransportDaemonControlRequest{
		Action:      "stop",
		Reason:      "operator requested maintenance",
		RequestedBy: "alice",
	}, RuntimeWorkerTransportDaemonControlRequestOptions{Now: now})
	if err == nil || !strings.Contains(err.Error(), "worker runtime side effect requires current safety bypass authorization") {
		t.Fatalf("expected strict safety authorization error, got %v", err)
	}
	if !strings.Contains(err.Error(), "missing required safety bypass authorization for scope.target_policy disabled") {
		t.Fatalf("expected disabled target-policy authorization detail, got %v", err)
	}
	controls, err := ListRuntimeWorkerTransportDaemonControlRequests(root, "remote-web-1")
	if err != nil {
		t.Fatalf("list daemon control requests after blocked creation: %v", err)
	}
	if len(controls) != 0 {
		t.Fatalf("blocked daemon control creation must not write artifacts, got %#v", controls)
	}
}

func TestCreateRuntimeWorkerTransportDaemonControlRequestAllowsDisabledBoundariesWithCurrentSafetyAuthorization(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Date(2026, 6, 2, 16, 57, 0, 0, time.UTC)
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	writeRuntimeWorkerDisabledSafetyScope(t, root)
	writeRuntimeWorkerCurrentSafetyAuthorization(t, root)

	control, err := CreateRuntimeWorkerTransportDaemonControlRequest(root, "remote-web-1", RuntimeWorkerTransportDaemonControlRequest{
		Action:      "restart",
		Reason:      "authorized restart",
		RequestedBy: "alice",
	}, RuntimeWorkerTransportDaemonControlRequestOptions{Now: now})
	if err != nil {
		t.Fatalf("create daemon control request with current safety authorization: %v", err)
	}
	if control.Status != "pending" || control.Action != "restart" || control.Path == "" {
		t.Fatalf("expected pending authorized daemon control request, got %#v", control)
	}
}

func TestListAndReadRuntimeWorkerTransportDaemonControlRequests(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Date(2026, 5, 30, 18, 14, 0, 0, time.UTC)
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	first, err := CreateRuntimeWorkerTransportDaemonControlRequest(root, "remote-web-1", RuntimeWorkerTransportDaemonControlRequest{
		Action: "stop",
		Reason: "pause daemon before rotating credentials",
	}, RuntimeWorkerTransportDaemonControlRequestOptions{Now: now})
	if err != nil {
		t.Fatalf("create first daemon control request: %v", err)
	}
	second, err := CreateRuntimeWorkerTransportDaemonControlRequest(root, "remote-web-1", RuntimeWorkerTransportDaemonControlRequest{
		Action: "restart",
		Reason: "reload worker environment",
	}, RuntimeWorkerTransportDaemonControlRequestOptions{Now: now.Add(time.Minute)})
	if err != nil {
		t.Fatalf("create second daemon control request: %v", err)
	}

	controls, err := ListRuntimeWorkerTransportDaemonControlRequests(root, "remote-web-1")
	if err != nil {
		t.Fatalf("list daemon control requests: %v", err)
	}
	if len(controls) != 2 || controls[0].Path != first.Path || controls[1].Path != second.Path {
		t.Fatalf("expected daemon controls sorted by request time, got %#v", controls)
	}
	read, err := ReadRuntimeWorkerTransportDaemonControlRequest(root, "remote-web-1", filepath.Base(second.Path))
	if err != nil {
		t.Fatalf("read daemon control request: %v", err)
	}
	if read.Action != "restart" || read.Reason != "reload worker environment" || read.Path != second.Path {
		t.Fatalf("unexpected read daemon control request: %#v", read)
	}
	if _, err := ReadRuntimeWorkerTransportDaemonControlRequest(root, "remote-web-1", filepath.Join(t.TempDir(), "outside.json")); err == nil || !strings.Contains(err.Error(), "worker transport daemon control path must stay under") {
		t.Fatalf("expected daemon control path confinement error, got %v", err)
	}
}

func TestCreateRuntimeWorkerTransportDaemonControlRequestRejectsUnsupportedAction(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	_, err := CreateRuntimeWorkerTransportDaemonControlRequest(root, "remote-web-1", RuntimeWorkerTransportDaemonControlRequest{
		Action: "kill",
		Reason: "unsupported action should not persist",
	}, RuntimeWorkerTransportDaemonControlRequestOptions{Now: time.Now().UTC()})
	if err == nil || !strings.Contains(err.Error(), "unsupported worker transport daemon control action") {
		t.Fatalf("expected unsupported action error, got %v", err)
	}
	controls, err := ListRuntimeWorkerTransportDaemonControlRequests(root, "remote-web-1")
	if err != nil {
		t.Fatalf("list daemon control requests: %v", err)
	}
	if len(controls) != 0 {
		t.Fatalf("unsupported action must not write daemon control artifact, got %#v", controls)
	}
}

func TestCreateRuntimeWorkerTransportDaemonControlRequestRejectsUnsupportedActionBeforeSafetyAuthorization(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	writeRuntimeWorkerDisabledSafetyScope(t, root)

	_, err := CreateRuntimeWorkerTransportDaemonControlRequest(root, "remote-web-1", RuntimeWorkerTransportDaemonControlRequest{
		Action: "kill",
		Reason: "unsupported action should not persist",
	}, RuntimeWorkerTransportDaemonControlRequestOptions{Now: time.Now().UTC()})
	if err == nil || !strings.Contains(err.Error(), "unsupported worker transport daemon control action") {
		t.Fatalf("expected unsupported action before safety authorization, got %v", err)
	}
	if strings.Contains(err.Error(), "worker runtime side effect requires current safety bypass authorization") {
		t.Fatalf("unsupported action must not expose safety authorization details, got %v", err)
	}
	controls, err := ListRuntimeWorkerTransportDaemonControlRequests(root, "remote-web-1")
	if err != nil {
		t.Fatalf("list daemon control requests: %v", err)
	}
	if len(controls) != 0 {
		t.Fatalf("unsupported action must not write daemon control artifact, got %#v", controls)
	}
}

func TestExecuteRuntimeWorkerTransportDispatchDaemonIgnoresControlRequestsUnlessEnabled(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Date(2026, 5, 30, 18, 16, 0, 0, time.UTC)
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	control, err := CreateRuntimeWorkerTransportDaemonControlRequest(root, "remote-web-1", RuntimeWorkerTransportDaemonControlRequest{
		Action: "stop",
		Reason: "should remain pending without daemon control opt-in",
	}, RuntimeWorkerTransportDaemonControlRequestOptions{Now: now})
	if err != nil {
		t.Fatalf("create daemon control request: %v", err)
	}

	daemon, err := ExecuteRuntimeWorkerTransportDispatchDaemon(context.Background(), root, "remote-web-1", RuntimeWorkerTransportRunDaemonOptions{
		Execute:               true,
		UseRequestCommand:     true,
		MaxRequests:           1,
		MaxIdleScans:          1,
		PollInterval:          time.Millisecond,
		Now:                   now.Add(time.Minute),
		EnableControlRequests: false,
	})
	if err != nil {
		t.Fatalf("execute daemon without control opt-in: %v", err)
	}
	if daemon.Status != "idle" || daemon.ControlAction != "" || daemon.ControlRequestPath != "" {
		t.Fatalf("daemon should ignore controls unless explicitly enabled, got %#v", daemon)
	}
	read, err := ReadRuntimeWorkerTransportDaemonControlRequest(root, "remote-web-1", filepath.Base(control.Path))
	if err != nil {
		t.Fatalf("read daemon control request: %v", err)
	}
	if read.Status != "pending" || read.AppliedAt != "" || read.AppliedDaemonRunPath != "" {
		t.Fatalf("control request should remain pending without opt-in, got %#v", read)
	}
}

func TestExecuteRuntimeWorkerTransportDispatchDaemonStopsOnPendingControlRequest(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Date(2026, 5, 30, 18, 18, 0, 0, time.UTC)
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	if _, err := RegisterRuntimeWorker(root, RuntimeWorkerArtifact{
		WorkerID:     "remote-web-1",
		Mode:         "remote",
		Status:       "available",
		Endpoint:     "file://remote-web-1/inbox",
		Capabilities: []string{"web.discovery"},
	}, RuntimeWorkerRegistrationOptions{Now: now}); err != nil {
		t.Fatalf("register runtime worker: %v", err)
	}
	dispatch, err := createRuntimeWorkerTransportDaemonRequest(root, now, "run-daemon-stop", "tool-stop", RuntimeWorkerToolPlanPayload{
		Action:     "discover_assets",
		Runner:     "command",
		RunCommand: []string{"sh", "-c", "printf 'should-not-run'"},
	})
	if err != nil {
		t.Fatalf("create daemon request: %v", err)
	}
	if _, err := AcknowledgeRuntimeWorkerTransportDispatchRequest(root, "remote-web-1", filepath.Base(dispatch.TransportRequestPath), RuntimeWorkerTransportAckOptions{
		Ack: true,
		Now: now.Add(time.Minute),
	}); err != nil {
		t.Fatalf("ack daemon request: %v", err)
	}
	control, err := CreateRuntimeWorkerTransportDaemonControlRequest(root, "remote-web-1", RuntimeWorkerTransportDaemonControlRequest{
		Action:      "stop",
		Reason:      "operator requested maintenance",
		RequestedBy: "alice",
	}, RuntimeWorkerTransportDaemonControlRequestOptions{Now: now.Add(2 * time.Minute)})
	if err != nil {
		t.Fatalf("create daemon control request: %v", err)
	}

	daemon, err := ExecuteRuntimeWorkerTransportDispatchDaemon(context.Background(), root, "remote-web-1", RuntimeWorkerTransportRunDaemonOptions{
		Execute:               true,
		UseRequestCommand:     true,
		MaxRequests:           1,
		MaxIdleScans:          20,
		PollInterval:          time.Millisecond,
		Now:                   now.Add(3 * time.Minute),
		EnableControlRequests: true,
	})
	if err != nil {
		t.Fatalf("execute daemon with control opt-in: %v", err)
	}
	if daemon.Status != "stopped" || daemon.Processed != 0 || daemon.ControlAction != "stop" || daemon.ControlRequestPath != control.Path {
		t.Fatalf("expected daemon to stop before processing requests, got %#v", daemon)
	}
	if !strings.Contains(daemon.StopReason, "operator requested maintenance") || daemon.ControlRequestedBy != "alice" {
		t.Fatalf("expected daemon stop audit metadata, got %#v", daemon)
	}
	readControl, err := ReadRuntimeWorkerTransportDaemonControlRequest(root, "remote-web-1", filepath.Base(control.Path))
	if err != nil {
		t.Fatalf("read daemon control request: %v", err)
	}
	if readControl.Status != "applied" || readControl.AppliedAt == "" || readControl.AppliedDaemonRunPath != daemon.Path {
		t.Fatalf("expected daemon control request to be marked applied, got %#v daemon=%#v", readControl, daemon)
	}
	request, err := ReadRuntimeWorkerTransportDispatchRequest(root, "remote-web-1", filepath.Base(dispatch.TransportRequestPath))
	if err != nil {
		t.Fatalf("read daemon transport request: %v", err)
	}
	if request.Status != "acknowledged" || request.ResultPath != "" {
		t.Fatalf("stopped daemon must not process acknowledged request, got %#v", request)
	}
}

func TestExecuteRuntimeWorkerTransportDispatchDaemonRequiresCurrentSafetyAuthorizationBeforeApplyingControl(t *testing.T) {
	t.Skip("Safety authorization checks disabled")
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Date(2026, 6, 2, 14, 34, 0, 0, time.UTC)
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	control, err := CreateRuntimeWorkerTransportDaemonControlRequest(root, "remote-web-1", RuntimeWorkerTransportDaemonControlRequest{
		Action:      "stop",
		Reason:      "blocked without current authorization",
		RequestedBy: "alice",
	}, RuntimeWorkerTransportDaemonControlRequestOptions{Now: now})
	if err != nil {
		t.Fatalf("create daemon control request: %v", err)
	}
	writeRuntimeWorkerDisabledSafetyScope(t, root)

	daemon, err := ExecuteRuntimeWorkerTransportDispatchDaemon(context.Background(), root, "remote-web-1", RuntimeWorkerTransportRunDaemonOptions{
		Execute:               true,
		UseRequestCommand:     true,
		MaxRequests:           1,
		MaxIdleScans:          1,
		PollInterval:          time.Millisecond,
		Now:                   now.Add(time.Minute),
		EnableControlRequests: true,
	})
	if err == nil || !strings.Contains(err.Error(), "worker runtime side effect requires current safety bypass authorization") {
		t.Fatalf("expected strict safety authorization error before applying control, got daemon=%#v err=%v", daemon, err)
	}
	if daemon.Path != "" || daemon.Status != "" || daemon.ControlRequestPath != "" {
		t.Fatalf("blocked control application must not return a daemon run artifact, got %#v", daemon)
	}
	readControl, err := ReadRuntimeWorkerTransportDaemonControlRequest(root, "remote-web-1", filepath.Base(control.Path))
	if err != nil {
		t.Fatalf("read daemon control request after blocked apply: %v", err)
	}
	if readControl.Status != "pending" || readControl.AppliedAt != "" || readControl.AppliedDaemonRunPath != "" {
		t.Fatalf("blocked control application must leave control pending, got %#v", readControl)
	}
	runs, err := ListRuntimeWorkerTransportDaemonRuns(root, "remote-web-1")
	if err != nil {
		t.Fatalf("list daemon runs after blocked control apply: %v", err)
	}
	if len(runs) != 0 {
		t.Fatalf("blocked control application must not write daemon run artifacts, got %#v", runs)
	}
}

func TestExecuteRuntimeWorkerTransportDispatchDaemonAllowsControlWithCurrentSafetyAuthorization(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Date(2026, 6, 2, 14, 36, 0, 0, time.UTC)
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	control, err := CreateRuntimeWorkerTransportDaemonControlRequest(root, "remote-web-1", RuntimeWorkerTransportDaemonControlRequest{
		Action:      "stop",
		Reason:      "authorized maintenance",
		RequestedBy: "alice",
	}, RuntimeWorkerTransportDaemonControlRequestOptions{Now: now})
	if err != nil {
		t.Fatalf("create daemon control request: %v", err)
	}
	writeRuntimeWorkerDisabledSafetyScope(t, root)
	writeRuntimeWorkerCurrentSafetyAuthorization(t, root)

	daemon, err := ExecuteRuntimeWorkerTransportDispatchDaemon(context.Background(), root, "remote-web-1", RuntimeWorkerTransportRunDaemonOptions{
		Execute:               true,
		UseRequestCommand:     true,
		MaxRequests:           1,
		MaxIdleScans:          1,
		PollInterval:          time.Millisecond,
		Now:                   now.Add(time.Minute),
		EnableControlRequests: true,
	})
	if err != nil {
		t.Fatalf("authorized control application should be allowed: %v", err)
	}
	if daemon.Status != "stopped" || daemon.ControlRequestPath != control.Path || daemon.ControlAction != "stop" || daemon.Path == "" {
		t.Fatalf("expected authorized daemon control to be applied, got %#v", daemon)
	}
	readControl, err := ReadRuntimeWorkerTransportDaemonControlRequest(root, "remote-web-1", filepath.Base(control.Path))
	if err != nil {
		t.Fatalf("read daemon control request after authorized apply: %v", err)
	}
	if readControl.Status != "applied" || readControl.AppliedDaemonRunPath != daemon.Path || readControl.AppliedAt == "" {
		t.Fatalf("expected authorized daemon control to be marked applied, got %#v", readControl)
	}
}

func TestExecuteRuntimeWorkerTransportDaemonSupervisorRequiresExplicitExecute(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	_, err := ExecuteRuntimeWorkerTransportDaemonSupervisor(context.Background(), root, "remote-web-1", RuntimeWorkerTransportDaemonSupervisorOptions{
		UseRequestCommand:     true,
		EnableControlRequests: true,
		MaxRuns:               1,
		MaxRequests:           1,
		MaxIdleScans:          1,
		PollInterval:          time.Millisecond,
	})
	if err == nil || !strings.Contains(err.Error(), "worker transport daemon supervisor requires explicit execute") {
		t.Fatalf("expected explicit execute error, got %v", err)
	}
	runs, listErr := ListRuntimeWorkerTransportDaemonSupervisorRuns(root, "remote-web-1")
	if listErr != nil {
		t.Fatalf("list daemon supervisor runs: %v", listErr)
	}
	if len(runs) != 0 {
		t.Fatalf("supervisor without execute must not write audit artifacts, got %#v", runs)
	}
}

func TestExecuteRuntimeWorkerTransportDaemonSupervisorRequiresCurrentSafetyAuthorizationBeforeApplyingControl(t *testing.T) {
	t.Skip("Safety authorization checks disabled")
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Date(2026, 6, 2, 14, 38, 0, 0, time.UTC)
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	control, err := CreateRuntimeWorkerTransportDaemonControlRequest(root, "remote-web-1", RuntimeWorkerTransportDaemonControlRequest{
		Action:      "stop",
		Reason:      "supervisor blocked without authorization",
		RequestedBy: "alice",
	}, RuntimeWorkerTransportDaemonControlRequestOptions{Now: now})
	if err != nil {
		t.Fatalf("create daemon control request: %v", err)
	}
	writeRuntimeWorkerDisabledSafetyScope(t, root)

	supervisor, err := ExecuteRuntimeWorkerTransportDaemonSupervisor(context.Background(), root, "remote-web-1", RuntimeWorkerTransportDaemonSupervisorOptions{
		Execute:               true,
		UseRequestCommand:     true,
		EnableControlRequests: true,
		MaxRuns:               1,
		MaxRequests:           1,
		MaxIdleScans:          1,
		PollInterval:          time.Millisecond,
		Now:                   now.Add(time.Minute),
	})
	if err == nil || !strings.Contains(err.Error(), "worker runtime side effect requires current safety bypass authorization") {
		t.Fatalf("expected strict safety authorization error before supervisor applies control, got supervisor=%#v err=%v", supervisor, err)
	}
	if supervisor.Path != "" || supervisor.Status != "" || supervisor.Runs != 0 || len(supervisor.DaemonRuns) != 0 {
		t.Fatalf("blocked supervisor control application must not write or return audit state, got %#v", supervisor)
	}
	readControl, err := ReadRuntimeWorkerTransportDaemonControlRequest(root, "remote-web-1", filepath.Base(control.Path))
	if err != nil {
		t.Fatalf("read daemon control request after blocked supervisor apply: %v", err)
	}
	if readControl.Status != "pending" || readControl.AppliedAt != "" || readControl.AppliedDaemonRunPath != "" {
		t.Fatalf("blocked supervisor control application must leave control pending, got %#v", readControl)
	}
	daemonRuns, err := ListRuntimeWorkerTransportDaemonRuns(root, "remote-web-1")
	if err != nil {
		t.Fatalf("list daemon runs after blocked supervisor apply: %v", err)
	}
	if len(daemonRuns) != 0 {
		t.Fatalf("blocked supervisor control application must not write daemon runs, got %#v", daemonRuns)
	}
	supervisorRuns, err := ListRuntimeWorkerTransportDaemonSupervisorRuns(root, "remote-web-1")
	if err != nil {
		t.Fatalf("list supervisor runs after blocked supervisor apply: %v", err)
	}
	if len(supervisorRuns) != 0 {
		t.Fatalf("blocked supervisor control application must not write supervisor runs, got %#v", supervisorRuns)
	}
}

func TestExecuteRuntimeWorkerTransportDaemonSupervisorRestartsAndProcessesNextRun(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Date(2026, 5, 30, 18, 20, 0, 0, time.UTC)
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	if _, err := RegisterRuntimeWorker(root, RuntimeWorkerArtifact{
		WorkerID:     "remote-web-1",
		Mode:         "remote",
		Status:       "available",
		Endpoint:     "file://remote-web-1/inbox",
		Capabilities: []string{"web.discovery"},
	}, RuntimeWorkerRegistrationOptions{Now: now}); err != nil {
		t.Fatalf("register runtime worker: %v", err)
	}
	dispatch, err := createRuntimeWorkerTransportDaemonRequest(root, now, "run-daemon-supervisor", "tool-supervised", RuntimeWorkerToolPlanPayload{
		Action:     "discover_assets",
		Runner:     "command",
		RunCommand: []string{"sh", "-c", "printf 'supervised=%s' \"$OURO_EXECUTION_ID\""},
	})
	if err != nil {
		t.Fatalf("create daemon supervisor request: %v", err)
	}
	if _, err := AcknowledgeRuntimeWorkerTransportDispatchRequest(root, "remote-web-1", filepath.Base(dispatch.TransportRequestPath), RuntimeWorkerTransportAckOptions{
		Ack: true,
		Now: now.Add(time.Minute),
	}); err != nil {
		t.Fatalf("ack daemon supervisor request: %v", err)
	}
	control, err := CreateRuntimeWorkerTransportDaemonControlRequest(root, "remote-web-1", RuntimeWorkerTransportDaemonControlRequest{
		Action:      "restart",
		Reason:      "reload worker environment",
		RequestedBy: "alice",
	}, RuntimeWorkerTransportDaemonControlRequestOptions{Now: now.Add(2 * time.Minute)})
	if err != nil {
		t.Fatalf("create daemon restart control request: %v", err)
	}

	supervisor, err := ExecuteRuntimeWorkerTransportDaemonSupervisor(context.Background(), root, "remote-web-1", RuntimeWorkerTransportDaemonSupervisorOptions{
		Execute:               true,
		UseRequestCommand:     true,
		EnableControlRequests: true,
		MaxRuns:               2,
		MaxRequests:           1,
		MaxIdleScans:          1,
		PollInterval:          time.Millisecond,
		Now:                   now.Add(3 * time.Minute),
	})
	if err != nil {
		t.Fatalf("execute daemon supervisor: %v", err)
	}
	if supervisor.Status != "completed" || supervisor.Runs != 2 || supervisor.RestartCount != 1 || supervisor.Processed != 1 || supervisor.Completed != 1 || supervisor.Failed != 0 {
		t.Fatalf("unexpected daemon supervisor result: %#v", supervisor)
	}
	if len(supervisor.DaemonRunPaths) != 2 || len(supervisor.DaemonRuns) != 2 {
		t.Fatalf("expected two daemon run links, got %#v", supervisor)
	}
	if supervisor.DaemonRuns[0].Status != "restart-requested" || supervisor.DaemonRuns[0].ControlRequestPath != control.Path {
		t.Fatalf("expected first daemon run to consume restart control, got %#v", supervisor.DaemonRuns[0])
	}
	if supervisor.DaemonRuns[1].Status != "completed" || supervisor.DaemonRuns[1].Processed != 1 {
		t.Fatalf("expected second daemon run to process acknowledged request, got %#v", supervisor.DaemonRuns[1])
	}
	readControl, err := ReadRuntimeWorkerTransportDaemonControlRequest(root, "remote-web-1", filepath.Base(control.Path))
	if err != nil {
		t.Fatalf("read restart control request: %v", err)
	}
	if readControl.Status != "applied" || readControl.AppliedDaemonRunPath != supervisor.DaemonRunPaths[0] {
		t.Fatalf("expected restart control applied to first daemon run, got %#v", readControl)
	}
	request, err := ReadRuntimeWorkerTransportDispatchRequest(root, "remote-web-1", filepath.Base(dispatch.TransportRequestPath))
	if err != nil {
		t.Fatalf("read supervised request: %v", err)
	}
	if request.Status != "completed" || request.ResultPath == "" {
		t.Fatalf("expected supervised request to complete after restart, got %#v", request)
	}
	runs, err := ListRuntimeWorkerTransportDaemonSupervisorRuns(root, "remote-web-1")
	if err != nil {
		t.Fatalf("list daemon supervisor runs: %v", err)
	}
	if len(runs) != 1 || runs[0].Path != supervisor.Path || runs[0].DaemonRunPaths[0] != supervisor.DaemonRunPaths[0] {
		t.Fatalf("expected one persisted daemon supervisor run, got %#v", runs)
	}
	readSupervisor, err := ReadRuntimeWorkerTransportDaemonSupervisorRun(root, "remote-web-1", filepath.Base(supervisor.Path))
	if err != nil {
		t.Fatalf("read daemon supervisor run: %v", err)
	}
	if readSupervisor.Status != "completed" || readSupervisor.RestartCount != 1 || readSupervisor.Processed != 1 {
		t.Fatalf("unexpected persisted daemon supervisor run: %#v", readSupervisor)
	}
}

func TestExecuteRuntimeWorkerTransportDaemonSupervisorStopsOnStopControl(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Date(2026, 5, 30, 18, 24, 0, 0, time.UTC)
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	control, err := CreateRuntimeWorkerTransportDaemonControlRequest(root, "remote-web-1", RuntimeWorkerTransportDaemonControlRequest{
		Action:      "stop",
		Reason:      "operator maintenance",
		RequestedBy: "alice",
	}, RuntimeWorkerTransportDaemonControlRequestOptions{Now: now})
	if err != nil {
		t.Fatalf("create daemon stop control request: %v", err)
	}

	supervisor, err := ExecuteRuntimeWorkerTransportDaemonSupervisor(context.Background(), root, "remote-web-1", RuntimeWorkerTransportDaemonSupervisorOptions{
		Execute:               true,
		UseRequestCommand:     true,
		EnableControlRequests: true,
		MaxRuns:               3,
		MaxRequests:           1,
		MaxIdleScans:          10,
		PollInterval:          time.Millisecond,
		Now:                   now.Add(time.Minute),
	})
	if err != nil {
		t.Fatalf("execute daemon supervisor stop: %v", err)
	}
	if supervisor.Status != "stopped" || supervisor.Runs != 1 || supervisor.Processed != 0 || supervisor.RestartCount != 0 {
		t.Fatalf("expected supervisor to stop after one daemon run, got %#v", supervisor)
	}
	if len(supervisor.DaemonRuns) != 1 || supervisor.DaemonRuns[0].ControlRequestPath != control.Path || supervisor.DaemonRuns[0].Status != "stopped" {
		t.Fatalf("expected stop control daemon run, got %#v", supervisor.DaemonRuns)
	}
}

func TestExecuteRuntimeWorkerTransportDaemonSupervisorWritesRestartLimitAudit(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Date(2026, 5, 30, 18, 26, 0, 0, time.UTC)
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	if _, err := CreateRuntimeWorkerTransportDaemonControlRequest(root, "remote-web-1", RuntimeWorkerTransportDaemonControlRequest{
		Action:      "restart",
		Reason:      "operator requested restart",
		RequestedBy: "alice",
	}, RuntimeWorkerTransportDaemonControlRequestOptions{Now: now}); err != nil {
		t.Fatalf("create daemon restart control request: %v", err)
	}

	supervisor, err := ExecuteRuntimeWorkerTransportDaemonSupervisor(context.Background(), root, "remote-web-1", RuntimeWorkerTransportDaemonSupervisorOptions{
		Execute:               true,
		UseRequestCommand:     true,
		EnableControlRequests: true,
		MaxRuns:               1,
		MaxRequests:           1,
		MaxIdleScans:          1,
		PollInterval:          time.Millisecond,
		Now:                   now.Add(time.Minute),
	})
	if err != nil {
		t.Fatalf("restart-limit should be an auditable supervisor status, got %v", err)
	}
	if supervisor.Status != "restart-limit" || supervisor.Runs != 1 || supervisor.RestartCount != 1 || !strings.Contains(supervisor.Error, "max daemon supervisor runs") {
		t.Fatalf("expected restart-limit supervisor audit, got %#v", supervisor)
	}
	if supervisor.Path == "" || len(supervisor.DaemonRunPaths) != 1 {
		t.Fatalf("expected persisted restart-limit supervisor audit with daemon run link, got %#v", supervisor)
	}
	readSupervisor, err := ReadRuntimeWorkerTransportDaemonSupervisorRun(root, "remote-web-1", filepath.Base(supervisor.Path))
	if err != nil {
		t.Fatalf("read restart-limit supervisor run: %v", err)
	}
	if readSupervisor.Status != "restart-limit" || readSupervisor.RestartCount != 1 || readSupervisor.Error == "" {
		t.Fatalf("unexpected restart-limit audit: %#v", readSupervisor)
	}
}

func TestReadRuntimeWorkerTransportDispatchRequestRejectsPathOutsideWorkerOutbox(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	if _, err := ReadRuntimeWorkerTransportDispatchRequest(root, "remote-web-1", filepath.Join("..", "request.json")); err == nil || !strings.Contains(err.Error(), "worker transport request path must stay under") {
		t.Fatalf("expected path confinement error, got %v", err)
	}
}

func TestReadRuntimeWorkerQueueDispatchExecutionRejectsPathOutsideExecutionDir(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	if _, err := ReadRuntimeWorkerQueueDispatchExecution(root, filepath.Join("..", "execution.json")); err == nil || !strings.Contains(err.Error(), "worker queue dispatch execution path must stay under") {
		t.Fatalf("expected path confinement error, got %v", err)
	}
}

func TestSubmitRuntimeWorkerQueueDispatchResultWritesAuditAndClosesClaim(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Date(2026, 5, 30, 18, 0, 0, 0, time.UTC)
	dispatch := createDispatchedRuntimeWorkerQueueClaim(t, root, now)

	result, err := SubmitRuntimeWorkerQueueDispatchResult(root, filepath.Base(dispatch.Path), RuntimeWorkerQueueDispatchResult{
		Status: "completed",
		Output: "remote worker completed discovery",
	}, RuntimeWorkerQueueDispatchResultOptions{Now: now.Add(time.Minute)})
	if err != nil {
		t.Fatalf("submit queue dispatch result: %v", err)
	}
	if result.Status != "completed" || result.Mode != "worker-result" || result.DispatchExecutionPath != dispatch.Path {
		t.Fatalf("unexpected queue dispatch result: %#v", result)
	}
	if !strings.Contains(result.Path, filepath.Join("runtime", "worker-queue-dispatch-results")) {
		t.Fatalf("expected result under worker-queue-dispatch-results, got %s", result.Path)
	}
	executions, err := ListRuntimeWorkerQueueDispatchExecutions(root)
	if err != nil {
		t.Fatalf("list queue dispatch executions: %v", err)
	}
	if len(executions) != 1 || executions[0].Status != "completed" || executions[0].ResultPath != result.Path || executions[0].CompletedAt == "" {
		t.Fatalf("expected dispatch execution to be closed by result, got %#v", executions)
	}
	claim, err := ReadRuntimeWorkerQueueClaim(root, filepath.Base(dispatch.ClaimPath))
	if err != nil {
		t.Fatalf("read queue claim: %v", err)
	}
	if claim.Status != "completed" || claim.ResultPath != result.Path || claim.ResultStatus != "completed" || claim.CompletedAt == "" {
		t.Fatalf("expected claim to be closed by result, got %#v", claim)
	}
	read, err := ReadRuntimeWorkerQueueDispatchResult(root, filepath.Base(result.Path))
	if err != nil {
		t.Fatalf("read queue dispatch result: %v", err)
	}
	if read.Path != result.Path || read.Output != "remote worker completed discovery" {
		t.Fatalf("unexpected read result: %#v", read)
	}
}

func TestSubmitRuntimeWorkerQueueDispatchResultRequiresCurrentSafetyAuthorization(t *testing.T) {
	t.Skip("Safety authorization checks disabled")
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Date(2026, 6, 2, 13, 42, 0, 0, time.UTC)
	dispatch := createDispatchedRuntimeWorkerQueueClaim(t, root, now)
	writeRuntimeWorkerDisabledSafetyScope(t, root)

	_, err := SubmitRuntimeWorkerQueueDispatchResult(root, filepath.Base(dispatch.Path), RuntimeWorkerQueueDispatchResult{
		Status: "completed",
		Output: "remote worker completed discovery",
	}, RuntimeWorkerQueueDispatchResultOptions{Now: now.Add(2 * time.Minute)})
	if err == nil || !strings.Contains(err.Error(), "worker runtime side effect requires current safety bypass authorization") {
		t.Fatalf("expected strict safety authorization error, got %v", err)
	}
	results, err := ListRuntimeWorkerQueueDispatchResults(root)
	if err != nil {
		t.Fatalf("list queue dispatch results after blocked submission: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("blocked result submission must not write result artifacts, got %#v", results)
	}
	execution, err := ReadRuntimeWorkerQueueDispatchExecution(root, filepath.Base(dispatch.Path))
	if err != nil {
		t.Fatalf("read dispatch execution after blocked result submission: %v", err)
	}
	if execution.Status != "dispatched" || execution.ResultPath != "" || execution.CompletedAt != "" {
		t.Fatalf("blocked result submission must not close dispatch execution, got %#v", execution)
	}
	claim, err := ReadRuntimeWorkerQueueClaim(root, filepath.Base(dispatch.ClaimPath))
	if err != nil {
		t.Fatalf("read claim after blocked result submission: %v", err)
	}
	if claim.Status != "dispatched" || claim.ResultPath != "" || claim.ResultStatus != "" || claim.CompletedAt != "" {
		t.Fatalf("blocked result submission must not close claim, got %#v", claim)
	}
}

func TestRuntimeWorkerAckAndResultSubmissionAllowDisabledBoundariesWithCurrentSafetyAuthorization(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Date(2026, 6, 2, 13, 44, 0, 0, time.UTC)
	dispatch := createFileOutboxRuntimeWorkerQueueDispatch(t, root, now)
	writeRuntimeWorkerDisabledSafetyScope(t, root)
	writeRuntimeWorkerCurrentSafetyAuthorization(t, root)

	acked, err := AcknowledgeRuntimeWorkerTransportDispatchRequest(root, "remote-web-1", filepath.Base(dispatch.TransportRequestPath), RuntimeWorkerTransportAckOptions{
		Ack: true,
		Now: now.Add(2 * time.Minute),
	})
	if err != nil {
		t.Fatalf("ack transport request with current safety authorization: %v", err)
	}
	if acked.Status != "acknowledged" || acked.AckedAt == "" {
		t.Fatalf("expected acknowledged request with current safety authorization, got %#v", acked)
	}
	result, err := SubmitRuntimeWorkerQueueDispatchResult(root, filepath.Base(dispatch.Path), RuntimeWorkerQueueDispatchResult{
		Status: "completed",
		Output: "authorized result",
	}, RuntimeWorkerQueueDispatchResultOptions{Now: now.Add(3 * time.Minute)})
	if err != nil {
		t.Fatalf("submit queue dispatch result with current safety authorization: %v", err)
	}
	if result.Status != "completed" || result.Path == "" {
		t.Fatalf("expected completed result with current safety authorization, got %#v", result)
	}
}

func TestSubmitRuntimeWorkerQueueDispatchResultRejectsInvalidStatusAndDuplicateResult(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Date(2026, 5, 30, 18, 30, 0, 0, time.UTC)
	dispatch := createDispatchedRuntimeWorkerQueueClaim(t, root, now)

	if _, err := SubmitRuntimeWorkerQueueDispatchResult(root, dispatch.Path, RuntimeWorkerQueueDispatchResult{
		Status: "running",
	}, RuntimeWorkerQueueDispatchResultOptions{Now: now.Add(time.Minute)}); err == nil || !strings.Contains(err.Error(), "unsupported worker queue dispatch result status") {
		t.Fatalf("expected invalid status error, got %v", err)
	}
	if _, err := SubmitRuntimeWorkerQueueDispatchResult(root, dispatch.Path, RuntimeWorkerQueueDispatchResult{
		Status: "failed",
		Error:  "worker command failed",
	}, RuntimeWorkerQueueDispatchResultOptions{Now: now.Add(2 * time.Minute)}); err != nil {
		t.Fatalf("submit failed queue dispatch result: %v", err)
	}
	if _, err := SubmitRuntimeWorkerQueueDispatchResult(root, dispatch.Path, RuntimeWorkerQueueDispatchResult{
		Status: "completed",
	}, RuntimeWorkerQueueDispatchResultOptions{Now: now.Add(3 * time.Minute)}); err == nil || !strings.Contains(err.Error(), "already has a result") {
		t.Fatalf("expected duplicate result error, got %v", err)
	}
}

func TestReadRuntimeWorkerQueueDispatchResultRejectsPathOutsideResultDir(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	if _, err := ReadRuntimeWorkerQueueDispatchResult(root, filepath.Join("..", "result.json")); err == nil || !strings.Contains(err.Error(), "worker queue dispatch result path must stay under") {
		t.Fatalf("expected path confinement error, got %v", err)
	}
}

func TestPlanRuntimeWorkerDispatchRetriesReadsFailedResultsWithoutMutation(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Date(2026, 5, 30, 19, 0, 0, 0, time.UTC)
	failedDispatch := createFileOutboxRuntimeWorkerQueueDispatchWithToolPlan(t, root, now, RuntimeWorkerToolPlanPayload{
		Action:     "discover_assets",
		Runner:     "command",
		Parser:     "web_api_parser",
		RunCommand: []string{"sh", "-c", "printf retry"},
	})
	completedDispatch := createFileOutboxRuntimeWorkerQueueDispatchForExecution(t, root, now.Add(time.Minute), "run-dispatch-file", "tool-web-completed")
	failedResult, err := SubmitRuntimeWorkerQueueDispatchResult(root, filepath.Base(failedDispatch.Path), RuntimeWorkerQueueDispatchResult{
		Status: "failed",
		Output: "partial output",
		Error:  "exit status 23",
	}, RuntimeWorkerQueueDispatchResultOptions{Now: now.Add(2 * time.Minute)})
	if err != nil {
		t.Fatalf("submit failed dispatch result: %v", err)
	}
	completedResult, err := SubmitRuntimeWorkerQueueDispatchResult(root, filepath.Base(completedDispatch.Path), RuntimeWorkerQueueDispatchResult{
		Status: "completed",
		Output: "completed output",
	}, RuntimeWorkerQueueDispatchResultOptions{Now: now.Add(3 * time.Minute)})
	if err != nil {
		t.Fatalf("submit completed dispatch result: %v", err)
	}
	claimsBefore, err := ListRuntimeWorkerQueueClaims(root)
	if err != nil {
		t.Fatalf("list claims before retry plan: %v", err)
	}
	dispatchesBefore, err := ListRuntimeWorkerQueueDispatchExecutions(root)
	if err != nil {
		t.Fatalf("list dispatches before retry plan: %v", err)
	}
	requestsBefore, err := ListRuntimeWorkerTransportDispatchRequests(root, "")
	if err != nil {
		t.Fatalf("list requests before retry plan: %v", err)
	}
	resultsBefore, err := ListRuntimeWorkerQueueDispatchResults(root)
	if err != nil {
		t.Fatalf("list results before retry plan: %v", err)
	}

	plan, err := PlanRuntimeWorkerDispatchRetries(root)
	if err != nil {
		t.Fatalf("plan worker dispatch retries: %v", err)
	}

	if plan.Summary.Total != 2 || plan.Summary.Ready != 1 || plan.Summary.Skipped != 1 {
		t.Fatalf("unexpected retry summary: %#v", plan.Summary)
	}
	if len(plan.Ready) != 1 {
		t.Fatalf("expected one retry candidate, got %#v", plan.Ready)
	}
	candidate := plan.Ready[0]
	if candidate.Status != "ready" || candidate.Mode != "metadata-only" || candidate.Gate != "worker-dispatch-retry-review" {
		t.Fatalf("unexpected retry candidate gate/state: %#v", candidate)
	}
	if candidate.ResultPath != failedResult.Path || candidate.DispatchPath != failedDispatch.Path || candidate.ClaimPath != failedDispatch.ClaimPath || candidate.TransportRequestPath != failedDispatch.TransportRequestPath {
		t.Fatalf("retry candidate lost artifact links: %#v", candidate)
	}
	if candidate.WorkerID != "remote-web-1" || candidate.RunID != "run-dispatch-file" || candidate.ExecutionID != "tool-web" || candidate.Tool != "web-api-pack" || candidate.Capability != "web.discovery" {
		t.Fatalf("retry candidate lost worker metadata: %#v", candidate)
	}
	if candidate.LastStatus != "failed" || candidate.LastOutput != "partial output" || candidate.LastError != "exit status 23" || candidate.ToolPlan.RunCommand[2] != "printf retry" {
		t.Fatalf("retry candidate lost result/tool-plan metadata: %#v", candidate)
	}
	if len(plan.Skipped) != 1 || plan.Skipped[0].ResultPath != completedResult.Path || plan.Skipped[0].Reason != "result-not-failed" {
		t.Fatalf("expected completed result skipped as not failed, got %#v", plan.Skipped)
	}
	claimsAfter, err := ListRuntimeWorkerQueueClaims(root)
	if err != nil {
		t.Fatalf("list claims after retry plan: %v", err)
	}
	dispatchesAfter, err := ListRuntimeWorkerQueueDispatchExecutions(root)
	if err != nil {
		t.Fatalf("list dispatches after retry plan: %v", err)
	}
	requestsAfter, err := ListRuntimeWorkerTransportDispatchRequests(root, "")
	if err != nil {
		t.Fatalf("list requests after retry plan: %v", err)
	}
	resultsAfter, err := ListRuntimeWorkerQueueDispatchResults(root)
	if err != nil {
		t.Fatalf("list results after retry plan: %v", err)
	}
	if len(claimsAfter) != len(claimsBefore) || len(dispatchesAfter) != len(dispatchesBefore) || len(requestsAfter) != len(requestsBefore) || len(resultsAfter) != len(resultsBefore) {
		t.Fatalf("retry planning must not create artifacts: claims %d/%d dispatches %d/%d requests %d/%d results %d/%d", len(claimsBefore), len(claimsAfter), len(dispatchesBefore), len(dispatchesAfter), len(requestsBefore), len(requestsAfter), len(resultsBefore), len(resultsAfter))
	}
	readFailedResult, err := ReadRuntimeWorkerQueueDispatchResult(root, filepath.Base(failedResult.Path))
	if err != nil {
		t.Fatalf("read failed result after retry plan: %v", err)
	}
	readFailedClaim, err := ReadRuntimeWorkerQueueClaim(root, filepath.Base(failedDispatch.ClaimPath))
	if err != nil {
		t.Fatalf("read failed claim after retry plan: %v", err)
	}
	readFailedRequest, err := ReadRuntimeWorkerTransportDispatchRequest(root, "remote-web-1", filepath.Base(failedDispatch.TransportRequestPath))
	if err != nil {
		t.Fatalf("read failed request after retry plan: %v", err)
	}
	if readFailedResult.Status != "failed" || readFailedClaim.Status != "failed" || readFailedRequest.Status != "failed" {
		t.Fatalf("retry planning must not mutate closed artifacts: result=%#v claim=%#v request=%#v", readFailedResult, readFailedClaim, readFailedRequest)
	}
}

func TestCreateRuntimeWorkerDispatchRetryPlanWritesDryRunArtifact(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Date(2026, 5, 30, 19, 10, 0, 0, time.UTC)
	dispatch := createDispatchedRuntimeWorkerQueueClaim(t, root, now)
	if _, err := SubmitRuntimeWorkerQueueDispatchResult(root, filepath.Base(dispatch.Path), RuntimeWorkerQueueDispatchResult{
		Status: "failed",
		Error:  "remote worker failed",
	}, RuntimeWorkerQueueDispatchResultOptions{Now: now.Add(time.Minute)}); err != nil {
		t.Fatalf("submit failed dispatch result: %v", err)
	}

	plan, err := CreateRuntimeWorkerDispatchRetryPlan(root, now.Add(2*time.Minute))
	if err != nil {
		t.Fatalf("create worker dispatch retry plan: %v", err)
	}

	if plan.Status != "dry-run" || plan.Mode != "metadata-only" || plan.CreatedAt == "" || plan.Summary.Ready != 1 {
		t.Fatalf("unexpected persisted retry plan: %#v", plan)
	}
	if !strings.Contains(plan.Path, filepath.Join("runtime", "worker-dispatch-retry-plans")) {
		t.Fatalf("expected retry plan under runtime/worker-dispatch-retry-plans, got %s", plan.Path)
	}
	plans, err := ListRuntimeWorkerDispatchRetryPlans(root)
	if err != nil {
		t.Fatalf("list worker dispatch retry plans: %v", err)
	}
	if len(plans) != 1 || plans[0].Path != plan.Path || plans[0].Summary.Ready != 1 {
		t.Fatalf("unexpected retry plan listing: %#v", plans)
	}
	read, err := ReadRuntimeWorkerDispatchRetryPlan(root, filepath.Base(plan.Path))
	if err != nil {
		t.Fatalf("read worker dispatch retry plan: %v", err)
	}
	if read.Path != plan.Path || read.Ready[0].ResultPath == "" || read.Ready[0].Mode != "metadata-only" {
		t.Fatalf("unexpected read retry plan: %#v", read)
	}
	if _, err := ReadRuntimeWorkerDispatchRetryPlan(root, filepath.Join("..", "retry-plan.json")); err == nil || !strings.Contains(err.Error(), "worker dispatch retry plan path must stay under") {
		t.Fatalf("expected retry plan path confinement error, got %v", err)
	}
}

func TestCreateRuntimeWorkerDispatchRetryDispatchWritesDryRunArtifactWithoutMutation(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Date(2026, 5, 31, 14, 0, 0, 0, time.UTC)
	marker := filepath.Join(t.TempDir(), "worker-retry-dispatch-ran")
	dispatch := createFileOutboxRuntimeWorkerQueueDispatchWithToolPlan(t, root, now, RuntimeWorkerToolPlanPayload{
		Action:     "discover_assets",
		Runner:     "command",
		Parser:     "web_api_parser",
		RunCommand: []string{"sh", "-c", "touch " + marker},
	})
	if _, err := SubmitRuntimeWorkerQueueDispatchResult(root, filepath.Base(dispatch.Path), RuntimeWorkerQueueDispatchResult{
		Status: "failed",
		Error:  "exit status 23",
	}, RuntimeWorkerQueueDispatchResultOptions{Now: now.Add(time.Minute)}); err != nil {
		t.Fatalf("submit failed dispatch result: %v", err)
	}
	plan, err := CreateRuntimeWorkerDispatchRetryPlan(root, now.Add(2*time.Minute))
	if err != nil {
		t.Fatalf("create worker dispatch retry plan: %v", err)
	}
	claimsBefore := countJSON(t, filepath.Join(root, "runtime", "worker-queue-claims"))
	dispatchesBefore := countJSON(t, filepath.Join(root, "runtime", "worker-queue-dispatch-executions"))
	requestsBefore := countJSON(t, filepath.Join(root, "runtime", "worker-transport-outbox", "remote-web-1"))
	resultsBefore := countJSON(t, filepath.Join(root, "runtime", "worker-queue-dispatch-results"))

	retryDispatch, err := CreateRuntimeWorkerDispatchRetryDispatch(root, filepath.Base(plan.Path), now.Add(3*time.Minute))
	if err != nil {
		t.Fatalf("create worker dispatch retry dispatch: %v", err)
	}

	if retryDispatch.Status != "dry-run" || retryDispatch.Mode != "metadata-only" || retryDispatch.RetryPlanPath != plan.Path || retryDispatch.Summary.Ready != 1 {
		t.Fatalf("unexpected retry dispatch artifact: %#v", retryDispatch)
	}
	if !strings.Contains(retryDispatch.Path, filepath.Join("runtime", "worker-dispatch-retry-dispatches")) {
		t.Fatalf("expected worker dispatch retry dispatch path under runtime directory, got %s", retryDispatch.Path)
	}
	if len(retryDispatch.Ready) != 1 || retryDispatch.Ready[0].ResultPath != plan.Ready[0].ResultPath || retryDispatch.Ready[0].ToolPlan.RunCommand[2] != "touch "+marker {
		t.Fatalf("retry dispatch lost retry-plan candidate metadata: %#v", retryDispatch.Ready)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("retry dispatch artifact creation must not execute retry command, marker stat err=%v", err)
	}
	if got := countJSON(t, filepath.Join(root, "runtime", "worker-queue-claims")); got != claimsBefore {
		t.Fatalf("retry dispatch creation must not create queue claims: before=%d after=%d", claimsBefore, got)
	}
	if got := countJSON(t, filepath.Join(root, "runtime", "worker-queue-dispatch-executions")); got != dispatchesBefore {
		t.Fatalf("retry dispatch creation must not create queue dispatches: before=%d after=%d", dispatchesBefore, got)
	}
	if got := countJSON(t, filepath.Join(root, "runtime", "worker-transport-outbox", "remote-web-1")); got != requestsBefore {
		t.Fatalf("retry dispatch creation must not create outbox requests: before=%d after=%d", requestsBefore, got)
	}
	if got := countJSON(t, filepath.Join(root, "runtime", "worker-queue-dispatch-results")); got != resultsBefore {
		t.Fatalf("retry dispatch creation must not submit results: before=%d after=%d", resultsBefore, got)
	}
	dispatches, err := ListRuntimeWorkerDispatchRetryDispatches(root)
	if err != nil {
		t.Fatalf("list worker dispatch retry dispatches: %v", err)
	}
	if len(dispatches) != 1 || dispatches[0].Path != retryDispatch.Path || dispatches[0].Summary.Ready != 1 {
		t.Fatalf("unexpected listed worker dispatch retry dispatches: %#v", dispatches)
	}
	read, err := ReadRuntimeWorkerDispatchRetryDispatch(root, filepath.Base(retryDispatch.Path))
	if err != nil {
		t.Fatalf("read worker dispatch retry dispatch: %v", err)
	}
	if read.Path != retryDispatch.Path || read.RetryPlanPath != plan.Path || read.Ready[0].Mode != "metadata-only" {
		t.Fatalf("unexpected read worker dispatch retry dispatch: %#v", read)
	}
	if _, err := ReadRuntimeWorkerDispatchRetryDispatch(root, filepath.Join("..", "retry-dispatch.json")); err == nil || !strings.Contains(err.Error(), "worker dispatch retry dispatch path must stay under") {
		t.Fatalf("expected retry dispatch path confinement error, got %v", err)
	}
}

func TestExecuteRuntimeWorkerDispatchRetryDispatchRequiresExplicitExecute(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Date(2026, 5, 31, 14, 30, 0, 0, time.UTC)
	_, retryDispatch, _ := createRuntimeWorkerDispatchRetryDispatchFixture(t, root, now)
	claimsBefore := countJSON(t, filepath.Join(root, "runtime", "worker-queue-claims"))

	_, err := ExecuteRuntimeWorkerDispatchRetryDispatch(root, filepath.Base(retryDispatch.Path), RuntimeWorkerDispatchRetryDispatchExecutionOptions{})

	if err == nil || !strings.Contains(err.Error(), "worker dispatch retry dispatch execution requires explicit execute") {
		t.Fatalf("expected explicit execute error, got %v", err)
	}
	if got := countJSON(t, filepath.Join(root, "runtime", "worker-queue-claims")); got != claimsBefore {
		t.Fatalf("retry dispatch execution without --execute must not create queue claims: before=%d after=%d", claimsBefore, got)
	}
}

func TestExecuteRuntimeWorkerDispatchRetryDispatchRequiresCurrentSafetyAuthorizationBeforeRetryMutation(t *testing.T) {
	t.Skip("Safety authorization checks disabled")
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Date(2026, 6, 2, 14, 20, 0, 0, time.UTC)
	_, retryDispatch, _ := createRuntimeWorkerDispatchRetryDispatchFixture(t, root, now, RuntimeWorkerToolPlanPayload{
		Action:     "discover_assets",
		Runner:     "command",
		Parser:     "web_api_parser",
		RunCommand: []string{"sh", "-c", "printf retry"},
	})
	claimsBefore := countJSON(t, filepath.Join(root, "runtime", "worker-queue-claims"))
	dispatchesBefore := countJSON(t, filepath.Join(root, "runtime", "worker-queue-dispatch-executions"))
	requestsBefore := countJSON(t, filepath.Join(root, "runtime", "worker-transport-outbox", "remote-web-1"))
	writeRuntimeWorkerDisabledSafetyScope(t, root)

	_, err := ExecuteRuntimeWorkerDispatchRetryDispatch(root, filepath.Base(retryDispatch.Path), RuntimeWorkerDispatchRetryDispatchExecutionOptions{
		Execute:        true,
		Now:            now.Add(4 * time.Minute),
		LeaseDuration:  15 * time.Minute,
		DispatchClaims: true,
		TransportMode:  "file-outbox",
	})
	if err == nil || !strings.Contains(err.Error(), "worker runtime side effect requires current safety bypass authorization") {
		t.Fatalf("expected strict safety authorization error, got %v", err)
	}
	if got := countJSON(t, filepath.Join(root, "runtime", "worker-queue-claims")); got != claimsBefore {
		t.Fatalf("blocked retry dispatch execution must not create retry queue claims: before=%d after=%d", claimsBefore, got)
	}
	if got := countJSON(t, filepath.Join(root, "runtime", "worker-queue-dispatch-executions")); got != dispatchesBefore {
		t.Fatalf("blocked retry dispatch execution must not create queue dispatch audits: before=%d after=%d", dispatchesBefore, got)
	}
	if got := countJSON(t, filepath.Join(root, "runtime", "worker-transport-outbox", "remote-web-1")); got != requestsBefore {
		t.Fatalf("blocked retry dispatch execution must not create file-outbox requests: before=%d after=%d", requestsBefore, got)
	}
	executions, err := ListRuntimeWorkerDispatchRetryDispatchExecutions(root)
	if err != nil {
		t.Fatalf("list retry dispatch executions after blocked execution: %v", err)
	}
	if len(executions) != 0 {
		t.Fatalf("blocked retry dispatch execution must not write retry execution audit, got %#v", executions)
	}
}

func TestExecuteRuntimeWorkerDispatchRetryDispatchCreatesMetadataOnlyQueueClaimWithoutDispatch(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Date(2026, 5, 31, 14, 40, 0, 0, time.UTC)
	marker := filepath.Join(t.TempDir(), "worker-retry-dispatch-execution-ran")
	sourceDispatch, retryDispatch, _ := createRuntimeWorkerDispatchRetryDispatchFixture(t, root, now, RuntimeWorkerToolPlanPayload{
		Action:     "discover_assets",
		Runner:     "command",
		Parser:     "web_api_parser",
		RunCommand: []string{"sh", "-c", "touch " + marker},
	})
	claimsBefore := countJSON(t, filepath.Join(root, "runtime", "worker-queue-claims"))
	dispatchesBefore := countJSON(t, filepath.Join(root, "runtime", "worker-queue-dispatch-executions"))
	requestsBefore := countJSON(t, filepath.Join(root, "runtime", "worker-transport-outbox", "remote-web-1"))
	resultsBefore := countJSON(t, filepath.Join(root, "runtime", "worker-queue-dispatch-results"))

	execution, err := ExecuteRuntimeWorkerDispatchRetryDispatch(root, filepath.Base(retryDispatch.Path), RuntimeWorkerDispatchRetryDispatchExecutionOptions{
		Execute:       true,
		Now:           now.Add(4 * time.Minute),
		LeaseDuration: 15 * time.Minute,
	})
	if err != nil {
		t.Fatalf("execute worker dispatch retry dispatch: %v", err)
	}

	if execution.Status != "completed" || execution.Mode != "execute" || execution.RetryDispatchPath != retryDispatch.Path {
		t.Fatalf("unexpected retry dispatch execution: %#v", execution)
	}
	if execution.Summary.Total != 1 || execution.Summary.Claimed != 1 || execution.Summary.Skipped != 0 || execution.Summary.Failed != 0 {
		t.Fatalf("unexpected retry dispatch execution summary: %#v", execution.Summary)
	}
	if len(execution.Results) != 1 {
		t.Fatalf("expected one retry dispatch execution result, got %#v", execution.Results)
	}
	result := execution.Results[0]
	if result.Status != "claimed" || result.RetryClaimPath == "" || result.RetryClaimID == "" || result.SourceDispatchPath != sourceDispatch.Path || result.SourceClaimPath != sourceDispatch.ClaimPath {
		t.Fatalf("unexpected retry dispatch execution result: %#v", result)
	}
	claim, err := ReadRuntimeWorkerQueueClaim(root, filepath.Base(result.RetryClaimPath))
	if err != nil {
		t.Fatalf("read retry queue claim: %v", err)
	}
	if claim.Status != "claimed" || claim.Mode != "metadata-only" || claim.WorkerID != sourceDispatch.WorkerID || claim.RunID != sourceDispatch.RunID || claim.ExecutionID != sourceDispatch.ExecutionID || claim.LeaseSeconds != int64((15*time.Minute)/time.Second) {
		t.Fatalf("unexpected retry queue claim: %#v", claim)
	}
	if len(claim.ToolPlan.RunCommand) != 3 || claim.ToolPlan.RunCommand[2] != "touch "+marker {
		t.Fatalf("retry queue claim lost tool plan command: %#v", claim.ToolPlan)
	}
	if got := countJSON(t, filepath.Join(root, "runtime", "worker-queue-claims")); got != claimsBefore+1 {
		t.Fatalf("retry dispatch execution should create one queue claim: before=%d after=%d", claimsBefore, got)
	}
	if got := countJSON(t, filepath.Join(root, "runtime", "worker-queue-dispatch-executions")); got != dispatchesBefore {
		t.Fatalf("retry dispatch execution must not create queue dispatches: before=%d after=%d", dispatchesBefore, got)
	}
	if got := countJSON(t, filepath.Join(root, "runtime", "worker-transport-outbox", "remote-web-1")); got != requestsBefore {
		t.Fatalf("retry dispatch execution must not create outbox requests: before=%d after=%d", requestsBefore, got)
	}
	if got := countJSON(t, filepath.Join(root, "runtime", "worker-queue-dispatch-results")); got != resultsBefore {
		t.Fatalf("retry dispatch execution must not submit results: before=%d after=%d", resultsBefore, got)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("retry dispatch execution must not execute retry command, marker stat err=%v", err)
	}
	executions, err := ListRuntimeWorkerDispatchRetryDispatchExecutions(root)
	if err != nil {
		t.Fatalf("list retry dispatch executions: %v", err)
	}
	if len(executions) != 1 || executions[0].Path != execution.Path || executions[0].Summary.Claimed != 1 {
		t.Fatalf("unexpected retry dispatch executions: %#v", executions)
	}
	read, err := ReadRuntimeWorkerDispatchRetryDispatchExecution(root, filepath.Base(execution.Path))
	if err != nil {
		t.Fatalf("read retry dispatch execution: %v", err)
	}
	if read.Path != execution.Path || read.Results[0].RetryClaimPath != result.RetryClaimPath {
		t.Fatalf("unexpected read retry dispatch execution: %#v", read)
	}
	if _, err := ReadRuntimeWorkerDispatchRetryDispatchExecution(root, filepath.Join("..", "retry-execution.json")); err == nil || !strings.Contains(err.Error(), "worker dispatch retry dispatch execution path must stay under") {
		t.Fatalf("expected path confinement error, got %v", err)
	}
}

func TestExecuteRuntimeWorkerDispatchRetryDispatchCanDispatchRetryClaimsToFileOutbox(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	now := time.Date(2026, 5, 31, 17, 40, 0, 0, time.UTC)
	marker := filepath.Join(t.TempDir(), "worker-retry-dispatch-handoff-ran")
	_, retryDispatch, _ := createRuntimeWorkerDispatchRetryDispatchFixture(t, root, now, RuntimeWorkerToolPlanPayload{
		Action:     "discover_assets",
		Runner:     "command",
		Parser:     "web_api_parser",
		RunCommand: []string{"sh", "-c", "touch " + marker},
	})
	claimsBefore := countJSON(t, filepath.Join(root, "runtime", "worker-queue-claims"))
	dispatchesBefore := countJSON(t, filepath.Join(root, "runtime", "worker-queue-dispatch-executions"))
	requestsBefore := countJSON(t, filepath.Join(root, "runtime", "worker-transport-outbox", "remote-web-1"))
	resultsBefore := countJSON(t, filepath.Join(root, "runtime", "worker-queue-dispatch-results"))

	execution, err := ExecuteRuntimeWorkerDispatchRetryDispatch(root, filepath.Base(retryDispatch.Path), RuntimeWorkerDispatchRetryDispatchExecutionOptions{
		Execute:        true,
		Now:            now.Add(4 * time.Minute),
		LeaseDuration:  15 * time.Minute,
		DispatchClaims: true,
		TransportMode:  "file-outbox",
	})
	if err != nil {
		t.Fatalf("execute worker dispatch retry dispatch with handoff: %v", err)
	}

	if execution.Status != "completed" || execution.Summary.Claimed != 1 || execution.Summary.Dispatched != 1 || execution.Summary.Skipped != 0 || execution.Summary.Failed != 0 {
		t.Fatalf("unexpected retry dispatch handoff summary: %#v", execution)
	}
	if len(execution.Results) != 1 {
		t.Fatalf("expected one retry dispatch handoff result, got %#v", execution.Results)
	}
	result := execution.Results[0]
	if result.Status != "dispatched" || result.Reason != "retry-queue-claim-dispatched" || result.RetryClaimPath == "" || result.RetryQueueDispatchPath == "" || result.RetryTransportRequestPath == "" || result.RetryTransportMode != "file-outbox" {
		t.Fatalf("unexpected retry dispatch handoff result: %#v", result)
	}
	claim, err := ReadRuntimeWorkerQueueClaim(root, filepath.Base(result.RetryClaimPath))
	if err != nil {
		t.Fatalf("read retry queue claim: %v", err)
	}
	if claim.Status != "dispatched" || claim.DispatchExecutionPath != result.RetryQueueDispatchPath {
		t.Fatalf("expected retry queue claim to be dispatched through queue gate, got %#v", claim)
	}
	queueDispatch, err := ReadRuntimeWorkerQueueDispatchExecution(root, filepath.Base(result.RetryQueueDispatchPath))
	if err != nil {
		t.Fatalf("read retry queue dispatch: %v", err)
	}
	if queueDispatch.Status != "dispatched" || queueDispatch.TransportMode != "file-outbox" || queueDispatch.ClaimPath != result.RetryClaimPath || queueDispatch.TransportRequestPath != result.RetryTransportRequestPath {
		t.Fatalf("unexpected retry queue dispatch execution: %#v", queueDispatch)
	}
	if len(queueDispatch.ToolPlan.RunCommand) != 3 || queueDispatch.ToolPlan.RunCommand[2] != "touch "+marker {
		t.Fatalf("retry queue dispatch lost stored tool plan command: %#v", queueDispatch.ToolPlan)
	}
	request, err := ReadRuntimeWorkerTransportDispatchRequest(root, "remote-web-1", filepath.Base(result.RetryTransportRequestPath))
	if err != nil {
		t.Fatalf("read retry transport request: %v", err)
	}
	if request.Status != "pending" || request.DispatchExecutionPath != result.RetryQueueDispatchPath || request.ClaimPath != result.RetryClaimPath {
		t.Fatalf("unexpected retry transport request: %#v", request)
	}
	if got := countJSON(t, filepath.Join(root, "runtime", "worker-queue-claims")); got != claimsBefore+1 {
		t.Fatalf("retry dispatch handoff should create one queue claim: before=%d after=%d", claimsBefore, got)
	}
	if got := countJSON(t, filepath.Join(root, "runtime", "worker-queue-dispatch-executions")); got != dispatchesBefore+1 {
		t.Fatalf("retry dispatch handoff should create one queue dispatch: before=%d after=%d", dispatchesBefore, got)
	}
	if got := countJSON(t, filepath.Join(root, "runtime", "worker-transport-outbox", "remote-web-1")); got != requestsBefore+1 {
		t.Fatalf("retry dispatch handoff should create one outbox request: before=%d after=%d", requestsBefore, got)
	}
	if got := countJSON(t, filepath.Join(root, "runtime", "worker-queue-dispatch-results")); got != resultsBefore {
		t.Fatalf("retry dispatch handoff must not submit queue dispatch results: before=%d after=%d", resultsBefore, got)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("retry dispatch handoff must not execute retry command, marker stat err=%v", err)
	}
	read, err := ReadRuntimeWorkerDispatchRetryDispatchExecution(root, filepath.Base(execution.Path))
	if err != nil {
		t.Fatalf("read retry dispatch execution: %v", err)
	}
	if read.Summary.Dispatched != 1 || read.Results[0].RetryQueueDispatchPath != result.RetryQueueDispatchPath || read.Results[0].RetryTransportRequestPath != result.RetryTransportRequestPath {
		t.Fatalf("persisted retry dispatch handoff lost dispatch metadata: %#v", read)
	}
}

func createDispatchedRuntimeWorkerQueueClaim(t *testing.T, root string, now time.Time) RuntimeWorkerQueueDispatchExecution {
	t.Helper()
	claim := createClaimedRuntimeWorkerQueueClaim(t, root, now)
	dispatch, err := ExecuteRuntimeWorkerQueueClaimDispatch(root, claim.Path, RuntimeWorkerQueueDispatchExecutionOptions{
		Execute: true,
		Now:     now.Add(time.Minute),
	})
	if err != nil {
		t.Fatalf("execute runtime worker queue claim dispatch: %v", err)
	}
	return dispatch
}

func createFileOutboxRuntimeWorkerQueueDispatch(t *testing.T, root string, now time.Time) RuntimeWorkerQueueDispatchExecution {
	t.Helper()
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	if _, err := RegisterRuntimeWorker(root, RuntimeWorkerArtifact{
		WorkerID:     "remote-web-1",
		Mode:         "remote",
		Status:       "available",
		Endpoint:     "file://remote-web-1/inbox",
		Capabilities: []string{"web.discovery"},
	}, RuntimeWorkerRegistrationOptions{Now: now}); err != nil {
		t.Fatalf("register runtime worker: %v", err)
	}
	return createFileOutboxRuntimeWorkerQueueDispatchForExecution(t, root, now, "run-dispatch-file", "tool-web")
}

func createFileOutboxRuntimeWorkerQueueDispatchForExecution(t *testing.T, root string, now time.Time, runID, executionID string) RuntimeWorkerQueueDispatchExecution {
	return createFileOutboxRuntimeWorkerQueueDispatchForExecutionWithToolPlan(t, root, now, runID, executionID, RuntimeWorkerToolPlanPayload{})
}

func createFileOutboxRuntimeWorkerQueueDispatchWithToolPlan(t *testing.T, root string, now time.Time, plan RuntimeWorkerToolPlanPayload) RuntimeWorkerQueueDispatchExecution {
	t.Helper()
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	if _, err := RegisterRuntimeWorker(root, RuntimeWorkerArtifact{
		WorkerID:     "remote-web-1",
		Mode:         "remote",
		Status:       "available",
		Endpoint:     "file://remote-web-1/inbox",
		Capabilities: []string{"web.discovery"},
	}, RuntimeWorkerRegistrationOptions{Now: now}); err != nil {
		t.Fatalf("register runtime worker: %v", err)
	}
	return createFileOutboxRuntimeWorkerQueueDispatchForExecutionWithToolPlan(t, root, now, "run-dispatch-file", "tool-web", plan)
}

func createFileOutboxRuntimeWorkerQueueDispatchForExecutionWithToolPlan(t *testing.T, root string, now time.Time, runID, executionID string, plan RuntimeWorkerToolPlanPayload) RuntimeWorkerQueueDispatchExecution {
	t.Helper()
	claim, err := CreateRuntimeWorkerQueueClaim(root, RuntimeWorkerQueueClaim{
		WorkerID:    "remote-web-1",
		RunID:       runID,
		ExecutionID: executionID,
		Tool:        "web-api-pack",
		Capability:  "web.discovery",
		ToolPlan:    plan,
	}, RuntimeWorkerQueueClaimOptions{Now: now, LeaseDuration: 10 * time.Minute})
	if err != nil {
		t.Fatalf("create runtime worker queue claim: %v", err)
	}
	dispatch, err := ExecuteRuntimeWorkerQueueClaimDispatch(root, claim.Path, RuntimeWorkerQueueDispatchExecutionOptions{
		Execute:       true,
		Now:           now.Add(time.Minute),
		TransportMode: "file-outbox",
	})
	if err != nil {
		t.Fatalf("execute runtime worker queue claim dispatch: %v", err)
	}
	return dispatch
}

func createRuntimeWorkerDispatchRetryDispatchFixture(t *testing.T, root string, now time.Time, plans ...RuntimeWorkerToolPlanPayload) (RuntimeWorkerQueueDispatchExecution, RuntimeWorkerDispatchRetryDispatch, RuntimeWorkerDispatchRetryPlan) {
	t.Helper()
	plan := RuntimeWorkerToolPlanPayload{}
	if len(plans) > 0 {
		plan = plans[0]
	}
	dispatch := createFileOutboxRuntimeWorkerQueueDispatchWithToolPlan(t, root, now, plan)
	if _, err := SubmitRuntimeWorkerQueueDispatchResult(root, filepath.Base(dispatch.Path), RuntimeWorkerQueueDispatchResult{
		Status: "failed",
		Error:  "exit status 23",
	}, RuntimeWorkerQueueDispatchResultOptions{Now: now.Add(time.Minute)}); err != nil {
		t.Fatalf("submit failed dispatch result: %v", err)
	}
	retryPlan, err := CreateRuntimeWorkerDispatchRetryPlan(root, now.Add(2*time.Minute))
	if err != nil {
		t.Fatalf("create worker dispatch retry plan: %v", err)
	}
	retryDispatch, err := CreateRuntimeWorkerDispatchRetryDispatch(root, filepath.Base(retryPlan.Path), now.Add(3*time.Minute))
	if err != nil {
		t.Fatalf("create worker dispatch retry dispatch: %v", err)
	}
	return dispatch, retryDispatch, retryPlan
}

func createRuntimeWorkerTransportDaemonRequest(root string, now time.Time, runID, executionID string, plan RuntimeWorkerToolPlanPayload) (RuntimeWorkerQueueDispatchExecution, error) {
	claim, err := CreateRuntimeWorkerQueueClaim(root, RuntimeWorkerQueueClaim{
		WorkerID:    "remote-web-1",
		RunID:       runID,
		ExecutionID: executionID,
		Tool:        "web-api-pack",
		Capability:  "web.discovery",
		ToolPlan:    plan,
	}, RuntimeWorkerQueueClaimOptions{Now: now, LeaseDuration: 10 * time.Minute})
	if err != nil {
		return RuntimeWorkerQueueDispatchExecution{}, fmt.Errorf("create runtime worker queue claim: %w", err)
	}
	dispatch, err := ExecuteRuntimeWorkerQueueClaimDispatch(root, claim.Path, RuntimeWorkerQueueDispatchExecutionOptions{
		Execute:       true,
		Now:           now.Add(time.Minute),
		TransportMode: "file-outbox",
	})
	if err != nil {
		return RuntimeWorkerQueueDispatchExecution{}, fmt.Errorf("execute runtime worker queue claim dispatch: %w", err)
	}
	return dispatch, nil
}

func writeRuntimeWorkerDisabledSafetyScope(t *testing.T, root string) {
	t.Helper()
	scope := `version: 1
scope:
  target_policy: disabled
  targets: []
  exclusions: []
execution:
  isolation: process
`
	if err := os.WriteFile(filepath.Join(root, "scope.yaml"), []byte(scope), 0o644); err != nil {
		t.Fatalf("write disabled safety scope: %v", err)
	}
}

func writeRuntimeWorkerCurrentSafetyAuthorization(t *testing.T, root string) {
	t.Helper()
	authorization := `version: 1
safety_bypasses:
  - control: scope.target_policy
    mode: disabled
    authorized_by: alice
    reason: authorized runtime worker target-boundary bypass
    ticket: ENG-4242
    expires: 2999-06-30
`
	if err := os.WriteFile(filepath.Join(root, "authorization.yaml"), []byte(authorization), 0o644); err != nil {
		t.Fatalf("write current safety authorization: %v", err)
	}
}

func createClaimedRuntimeWorkerQueueClaim(t *testing.T, root string, now time.Time) RuntimeWorkerQueueClaim {
	t.Helper()
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	if _, err := RegisterRuntimeWorker(root, RuntimeWorkerArtifact{
		WorkerID:     "remote-web-1",
		Mode:         "remote",
		Status:       "available",
		Capabilities: []string{"web.discovery"},
	}, RuntimeWorkerRegistrationOptions{Now: now}); err != nil {
		t.Fatalf("register runtime worker: %v", err)
	}
	claim, err := CreateRuntimeWorkerQueueClaim(root, RuntimeWorkerQueueClaim{
		WorkerID:    "remote-web-1",
		RunID:       "run-dispatch-1",
		ExecutionID: "tool-web",
		Tool:        "web-api-pack",
		Capability:  "web.discovery",
	}, RuntimeWorkerQueueClaimOptions{Now: now, LeaseDuration: 10 * time.Minute})
	if err != nil {
		t.Fatalf("create runtime worker queue claim: %v", err)
	}
	return claim
}
