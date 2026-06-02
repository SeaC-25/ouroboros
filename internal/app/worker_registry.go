package app

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const DefaultRuntimeWorkerStaleAfter = 15 * time.Minute

type RuntimeWorkerArtifact struct {
	Version      int               `json:"version"`
	Path         string            `json:"-"`
	WorkerID     string            `json:"worker_id"`
	Mode         string            `json:"mode"`
	Status       string            `json:"status"`
	Endpoint     string            `json:"endpoint,omitempty"`
	Capabilities []string          `json:"capabilities,omitempty"`
	Labels       map[string]string `json:"labels,omitempty"`
	RegisteredAt string            `json:"registered_at"`
	HeartbeatAt  string            `json:"heartbeat_at"`
	Stale        bool              `json:"-"`
}

type RuntimeWorkerRegistrationOptions struct {
	Now time.Time
}

type RuntimeWorkerHeartbeatOptions struct {
	Now    time.Time
	Status string
}

func RegisterRuntimeWorker(root string, worker RuntimeWorkerArtifact, options RuntimeWorkerRegistrationOptions) (RuntimeWorkerArtifact, error) {
	workerID := strings.TrimSpace(worker.WorkerID)
	if workerID == "" {
		return RuntimeWorkerArtifact{}, fmt.Errorf("runtime worker id is required")
	}
	mode, err := normalizeRuntimeWorkerMode(worker.Mode)
	if err != nil {
		return RuntimeWorkerArtifact{}, err
	}
	status, err := normalizeRuntimeWorkerStatus(worker.Status)
	if err != nil {
		return RuntimeWorkerArtifact{}, err
	}
	now := options.Now.UTC()
	if now.IsZero() {
		now = time.Now().UTC()
	}
	worker.Version = 1
	worker.WorkerID = workerID
	worker.Mode = mode
	worker.Status = status
	worker.Endpoint = strings.TrimSpace(worker.Endpoint)
	worker.Capabilities = normalizeRuntimeWorkerCapabilities(worker.Capabilities)
	worker.Labels = normalizeRuntimeWorkerLabels(worker.Labels)
	worker.RegisteredAt = now.Format(time.RFC3339Nano)
	worker.HeartbeatAt = now.Format(time.RFC3339Nano)
	worker.Path = runtimeWorkerPath(root, workerID)

	if err := writeRuntimeWorkerArtifact(worker); err != nil {
		return RuntimeWorkerArtifact{}, err
	}
	return worker, nil
}

func UpdateRuntimeWorkerHeartbeat(root, workerPath string, options RuntimeWorkerHeartbeatOptions) (RuntimeWorkerArtifact, error) {
	worker, err := ReadRuntimeWorker(root, workerPath)
	if err != nil {
		return RuntimeWorkerArtifact{}, err
	}
	if strings.TrimSpace(options.Status) != "" {
		status, err := normalizeRuntimeWorkerStatus(options.Status)
		if err != nil {
			return RuntimeWorkerArtifact{}, err
		}
		worker.Status = status
	}
	now := options.Now.UTC()
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if worker.Version == 0 {
		worker.Version = 1
	}
	worker.HeartbeatAt = now.Format(time.RFC3339Nano)
	worker.Stale = false
	if err := writeRuntimeWorkerArtifact(worker); err != nil {
		return RuntimeWorkerArtifact{}, err
	}
	return worker, nil
}

func ListRuntimeWorkers(root string) ([]RuntimeWorkerArtifact, error) {
	paths, err := filepath.Glob(filepath.Join(runtimeWorkerDir(root), "*.json"))
	if err != nil {
		return nil, fmt.Errorf("glob runtime workers: %w", err)
	}
	sort.Strings(paths)
	workers := make([]RuntimeWorkerArtifact, 0, len(paths))
	for _, path := range paths {
		worker, err := readRuntimeWorkerFile(path)
		if err != nil {
			return nil, err
		}
		workers = append(workers, worker)
	}
	sort.SliceStable(workers, func(i int, j int) bool {
		if workers[i].WorkerID != workers[j].WorkerID {
			return workers[i].WorkerID < workers[j].WorkerID
		}
		return workers[i].Path < workers[j].Path
	})
	return workers, nil
}

func ListRuntimeWorkersWithStaleness(root string, now time.Time, staleAfter time.Duration) ([]RuntimeWorkerArtifact, error) {
	workers, err := ListRuntimeWorkers(root)
	if err != nil {
		return nil, err
	}
	now = now.UTC()
	for i := range workers {
		workers[i].Stale = runtimeWorkerIsStale(workers[i], now, staleAfter)
	}
	return workers, nil
}

func ReadRuntimeWorker(root, workerPath string) (RuntimeWorkerArtifact, error) {
	path, err := resolveRuntimeWorkerPath(root, workerPath)
	if err != nil {
		return RuntimeWorkerArtifact{}, err
	}
	return readRuntimeWorkerFile(path)
}

func readRuntimeWorkerFile(path string) (RuntimeWorkerArtifact, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return RuntimeWorkerArtifact{}, fmt.Errorf("read runtime worker %s: %w", path, err)
	}
	var worker RuntimeWorkerArtifact
	if err := json.Unmarshal(data, &worker); err != nil {
		return RuntimeWorkerArtifact{}, fmt.Errorf("decode runtime worker %s: %w", path, err)
	}
	worker.Path = path
	return worker, nil
}

func writeRuntimeWorkerArtifact(worker RuntimeWorkerArtifact) error {
	if err := os.MkdirAll(filepath.Dir(worker.Path), 0o755); err != nil {
		return fmt.Errorf("create runtime worker registry directory: %w", err)
	}
	data, err := json.MarshalIndent(worker, "", "  ")
	if err != nil {
		return fmt.Errorf("encode runtime worker: %w", err)
	}
	data = append(data, '\n')
	tmp := worker.Path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("write runtime worker: %w", err)
	}
	if err := os.Rename(tmp, worker.Path); err != nil {
		return fmt.Errorf("publish runtime worker: %w", err)
	}
	return nil
}

func runtimeWorkerDir(root string) string {
	return filepath.Join(root, "runtime", "workers")
}

func runtimeWorkerPath(root string, workerID string) string {
	return filepath.Join(runtimeWorkerDir(root), safeRuntimeWorkerFileName(workerID)+".json")
}

func resolveRuntimeWorkerPath(root, workerPath string) (string, error) {
	trimmed := strings.TrimSpace(workerPath)
	if trimmed == "" {
		return "", fmt.Errorf("runtime worker path is required")
	}
	dir, err := filepath.Abs(runtimeWorkerDir(root))
	if err != nil {
		return "", fmt.Errorf("resolve runtime worker registry directory: %w", err)
	}
	candidate := trimmed
	if !filepath.IsAbs(candidate) {
		if filepath.Ext(candidate) == "" && !strings.Contains(candidate, string(os.PathSeparator)) {
			candidate = safeRuntimeWorkerFileName(candidate) + ".json"
		}
		candidate = filepath.Join(dir, candidate)
	}
	candidate, err = filepath.Abs(candidate)
	if err != nil {
		return "", fmt.Errorf("resolve runtime worker path: %w", err)
	}
	rel, err := filepath.Rel(dir, candidate)
	if err != nil {
		return "", fmt.Errorf("resolve runtime worker relative path: %w", err)
	}
	if rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("runtime worker path must stay under %s", dir)
	}
	if filepath.Ext(candidate) != ".json" {
		return "", fmt.Errorf("runtime worker path must be a json file")
	}
	return candidate, nil
}

func normalizeRuntimeWorkerMode(mode string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "":
		return "local", nil
	case "local", "remote":
		return strings.ToLower(strings.TrimSpace(mode)), nil
	default:
		return "", fmt.Errorf("unsupported runtime worker mode: %s", mode)
	}
}

func normalizeRuntimeWorkerStatus(status string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "":
		return "available", nil
	case "available", "busy", "draining", "offline":
		return strings.ToLower(strings.TrimSpace(status)), nil
	default:
		return "", fmt.Errorf("unsupported runtime worker status: %s", status)
	}
}

func normalizeRuntimeWorkerCapabilities(capabilities []string) []string {
	seen := map[string]bool{}
	var normalized []string
	for _, capability := range capabilities {
		capability = strings.TrimSpace(capability)
		if capability == "" || seen[capability] {
			continue
		}
		seen[capability] = true
		normalized = append(normalized, capability)
	}
	sort.Strings(normalized)
	return normalized
}

func normalizeRuntimeWorkerLabels(labels map[string]string) map[string]string {
	normalized := map[string]string{}
	for key, value := range labels {
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" || value == "" {
			continue
		}
		normalized[key] = value
	}
	if len(normalized) == 0 {
		return nil
	}
	return normalized
}

func runtimeWorkerIsStale(worker RuntimeWorkerArtifact, now time.Time, staleAfter time.Duration) bool {
	if staleAfter <= 0 || strings.TrimSpace(worker.Status) == "offline" {
		return false
	}
	timestamp := strings.TrimSpace(worker.HeartbeatAt)
	if timestamp == "" {
		timestamp = strings.TrimSpace(worker.RegisteredAt)
	}
	if timestamp == "" {
		return true
	}
	heartbeat, err := time.Parse(time.RFC3339Nano, timestamp)
	if err != nil {
		return true
	}
	return now.Sub(heartbeat) > staleAfter
}

func safeRuntimeWorkerFileName(workerID string) string {
	trimmed := strings.TrimSpace(workerID)
	var builder strings.Builder
	for _, r := range trimmed {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '.', r == '_', r == '-':
			builder.WriteRune(r)
		default:
			builder.WriteRune('-')
		}
	}
	slug := strings.Trim(builder.String(), "-._")
	if slug == "" {
		return safeInstallRequestName("runtime-worker-" + trimmed)
	}
	if slug == trimmed && len(slug) <= 80 {
		return slug
	}
	return safeInstallRequestName("runtime-worker-" + trimmed)
}
