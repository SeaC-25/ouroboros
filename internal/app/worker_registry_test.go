package app

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRegisterRuntimeWorkerWritesMetadataArtifact(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	now := time.Date(2026, 5, 30, 10, 11, 12, 0, time.UTC)

	worker, err := RegisterRuntimeWorker(root, RuntimeWorkerArtifact{
		WorkerID:     "remote-web-1",
		Mode:         "remote",
		Status:       "available",
		Endpoint:     "tcp://127.0.0.1:9443",
		Capabilities: []string{"web.discovery", "web.enumeration"},
		Labels:       map[string]string{"region": "lab"},
	}, RuntimeWorkerRegistrationOptions{Now: now})
	if err != nil {
		t.Fatalf("register runtime worker: %v", err)
	}

	if worker.Version != 1 || worker.WorkerID != "remote-web-1" || worker.Mode != "remote" || worker.Status != "available" {
		t.Fatalf("unexpected worker artifact: %#v", worker)
	}
	if worker.RegisteredAt != now.Format(time.RFC3339Nano) || worker.HeartbeatAt != now.Format(time.RFC3339Nano) {
		t.Fatalf("expected deterministic timestamps, got %#v", worker)
	}
	if !strings.Contains(worker.Path, filepath.Join("runtime", "workers", "remote-web-1.json")) {
		t.Fatalf("expected worker path under runtime/workers, got %s", worker.Path)
	}

	workers, err := ListRuntimeWorkers(root)
	if err != nil {
		t.Fatalf("list runtime workers: %v", err)
	}
	if len(workers) != 1 || workers[0].Path != worker.Path || workers[0].Endpoint != "tcp://127.0.0.1:9443" {
		t.Fatalf("expected listed worker artifact, got %#v", workers)
	}

	read, err := ReadRuntimeWorker(root, "remote-web-1")
	if err != nil {
		t.Fatalf("read runtime worker by id: %v", err)
	}
	if read.Path != worker.Path || len(read.Capabilities) != 2 || read.Labels["region"] != "lab" {
		t.Fatalf("expected read worker artifact, got %#v", read)
	}
}

func TestRegisterRuntimeWorkerDefaultsLocalAvailableMetadata(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}

	worker, err := RegisterRuntimeWorker(root, RuntimeWorkerArtifact{
		WorkerID:     "local",
		Capabilities: []string{"ctf.fast_hypothesis"},
	}, RuntimeWorkerRegistrationOptions{})
	if err != nil {
		t.Fatalf("register runtime worker: %v", err)
	}
	if worker.Mode != "local" || worker.Status != "available" || worker.RegisteredAt == "" || worker.HeartbeatAt == "" {
		t.Fatalf("expected local available defaults, got %#v", worker)
	}
}

func TestUpdateRuntimeWorkerHeartbeatRefreshesTimestampAndStatus(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	registeredAt := time.Date(2026, 5, 30, 9, 0, 0, 0, time.UTC)
	now := registeredAt.Add(10 * time.Minute)
	registered, err := RegisterRuntimeWorker(root, RuntimeWorkerArtifact{
		WorkerID:     "remote-web-1",
		Mode:         "remote",
		Status:       "available",
		Endpoint:     "tcp://127.0.0.1:9443",
		Capabilities: []string{"web.discovery"},
	}, RuntimeWorkerRegistrationOptions{Now: registeredAt})
	if err != nil {
		t.Fatalf("register runtime worker: %v", err)
	}

	updated, err := UpdateRuntimeWorkerHeartbeat(root, "remote-web-1", RuntimeWorkerHeartbeatOptions{
		Now:    now,
		Status: "busy",
	})
	if err != nil {
		t.Fatalf("update runtime worker heartbeat: %v", err)
	}
	if updated.Path != registered.Path || updated.RegisteredAt != registered.RegisteredAt {
		t.Fatalf("heartbeat update should preserve path and registration time, got %#v", updated)
	}
	if updated.Status != "busy" || updated.HeartbeatAt != now.Format(time.RFC3339Nano) {
		t.Fatalf("expected busy heartbeat update at %s, got %#v", now.Format(time.RFC3339Nano), updated)
	}
	read, err := ReadRuntimeWorker(root, filepath.Base(registered.Path))
	if err != nil {
		t.Fatalf("read runtime worker by basename: %v", err)
	}
	if read.Status != "busy" || read.HeartbeatAt != updated.HeartbeatAt {
		t.Fatalf("expected persisted heartbeat update, got %#v", read)
	}
}

func TestListRuntimeWorkersWithStalenessMarksOldOnlineWorkers(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	now := time.Date(2026, 5, 30, 11, 0, 0, 0, time.UTC)
	old := now.Add(-DefaultRuntimeWorkerStaleAfter - time.Minute)
	if _, err := RegisterRuntimeWorker(root, RuntimeWorkerArtifact{
		WorkerID:     "remote-web-1",
		Mode:         "remote",
		Status:       "available",
		Capabilities: []string{"web.discovery"},
	}, RuntimeWorkerRegistrationOptions{Now: old}); err != nil {
		t.Fatalf("register stale worker: %v", err)
	}
	if _, err := RegisterRuntimeWorker(root, RuntimeWorkerArtifact{
		WorkerID:     "remote-offline",
		Mode:         "remote",
		Status:       "offline",
		Capabilities: []string{"infra.host_discovery"},
	}, RuntimeWorkerRegistrationOptions{Now: old}); err != nil {
		t.Fatalf("register offline worker: %v", err)
	}

	workers, err := ListRuntimeWorkersWithStaleness(root, now, DefaultRuntimeWorkerStaleAfter)
	if err != nil {
		t.Fatalf("list runtime workers with staleness: %v", err)
	}
	stale := map[string]bool{}
	for _, worker := range workers {
		stale[worker.WorkerID] = worker.Stale
	}
	if !stale["remote-web-1"] {
		t.Fatalf("expected old available worker to be stale, got %#v", workers)
	}
	if stale["remote-offline"] {
		t.Fatalf("offline worker should not be marked stale, got %#v", workers)
	}
}

func TestReadRuntimeWorkerRejectsPathOutsideRegistry(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}

	if _, err := ReadRuntimeWorker(root, filepath.Join("..", "worker.json")); err == nil || !strings.Contains(err.Error(), "runtime worker path must stay under") {
		t.Fatalf("expected path confinement error, got %v", err)
	}
}

func TestUpdateRuntimeWorkerHeartbeatRejectsPathOutsideRegistry(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := InitWorkspace(root); err != nil {
		t.Fatalf("init workspace: %v", err)
	}

	if _, err := UpdateRuntimeWorkerHeartbeat(root, filepath.Join("..", "worker.json"), RuntimeWorkerHeartbeatOptions{}); err == nil || !strings.Contains(err.Error(), "runtime worker path must stay under") {
		t.Fatalf("expected path confinement error, got %v", err)
	}
}
