package api

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ouroboros/ouroboros/internal/app"
	"github.com/ouroboros/ouroboros/internal/eventlog"
	"github.com/ouroboros/ouroboros/internal/toolchain"
)

func ProjectGUIBootstrap(ctx context.Context, workspace string, store *eventlog.SQLiteStore) (GUIBootstrapDTO, error) {
	workspace = strings.TrimSpace(workspace)
	graph, err := ProjectEvidenceGraph(ctx, workspace, store)
	if err != nil {
		return GUIBootstrapDTO{}, err
	}
	mission, err := ProjectMission(workspace)
	if err != nil {
		return GUIBootstrapDTO{}, err
	}
	runs, err := ProjectRuns(ctx, store)
	if err != nil {
		return GUIBootstrapDTO{}, err
	}
	replayEvents, err := ProjectReplayEvents(ctx, store)
	if err != nil {
		return GUIBootstrapDTO{}, err
	}
	executions, err := ProjectExecutions(workspace)
	if err != nil {
		return GUIBootstrapDTO{}, err
	}
	toolchainDTO, err := ProjectToolchain(workspace)
	if err != nil {
		return GUIBootstrapDTO{}, err
	}
	toolActions := ProjectToolchainActions(toolchainDTO)
	provider, err := ProjectProvider(workspace)
	if err != nil {
		return GUIBootstrapDTO{}, err
	}
	reports, err := ProjectReports(workspace)
	if err != nil {
		return GUIBootstrapDTO{}, err
	}
	reportFindings, err := ProjectReportFindings(workspace, graph)
	if err != nil {
		return GUIBootstrapDTO{}, err
	}
	files, err := ProjectWorkspaceFiles(workspace)
	if err != nil {
		return GUIBootstrapDTO{}, err
	}
	blackboard := projectBootstrapBlackboard(workspace)
	agentState := projectBootstrapAgentState(workspace)
	harness := projectBootstrapHarness(workspace)

	return GUIBootstrapDTO{
		Workspace: ProjectWorkspace(workspace),
		Mission:   mission,
		Workflow: WorkflowDTO{
			Status:     workflowStatusFrom(runs, graph),
			NextAction: workflowNextActionFrom(runs, graph),
		},
		Provider:       provider,
		Toolchain:      toolchainDTO,
		ToolActions:    toolActions,
		Graph:          graph,
		Runs:           runs,
		ReplayEvents:   replayEvents,
		Executions:     executions,
		Reports:        reports,
		ReportFindings: reportFindings,
		Files:          files,
		Blackboard:     blackboard,
		AgentState:     agentState,
		Harness:        harness,
		Settings: SettingsDTO{
			Theme:         "dark",
			LastWorkspace: workspace,
		},
		Backend: BackendDTO{
			Status:       "ready",
			EventsStream: "/api/events/stream",
		},
	}, nil
}

func projectBootstrapBlackboard(workspace string) map[string]any {
	server := &Server{workspace: workspace}
	facts := server.loadBlackboardFacts()
	items := make([]map[string]any, 0, len(facts))
	for _, fact := range facts {
		items = append(items, map[string]any{
			"id": fact.ID, "kind": fact.Kind, "title": fact.Statement, "content": fact.Statement,
			"status": "open", "priority": "normal", "source": fact.Source,
			"createdAt": fact.CreatedAt, "updatedAt": fact.CreatedAt,
		})
	}
	// 观测维度：原始观测与结论笔记分开投影——观测是判定前证据。
	observations := server.loadBlackboardObservations()
	observationItems := make([]map[string]any, 0, len(observations))
	for _, observation := range observations {
		observationItems = append(observationItems, blackboardObservationDTO(observation))
	}
	return map[string]any{"path": filepath.ToSlash(filepath.Join(workspace, "runtime", "blackboard.jsonl")), "items": items, "observations": observationItems}
}

func projectBootstrapAgentState(workspace string) map[string]any {
	server := &Server{workspace: workspace}
	tasks := server.loadAgentTasks()
	profiles := server.loadWorkspaceAgentProfiles()
	events := server.loadAgentTaskEvents()
	projected := make([]map[string]any, 0, len(tasks))
	counts := map[string]int{}
	for _, task := range tasks {
		status := task.Status
		counts[status]++
		if status == "running" || status == "in_progress" {
			counts["inProgress"]++
		}
		if status == "complete" || status == "completed" {
			counts["completed"]++
		}
		if status == "error" || status == "failed" {
			counts["failed"]++
		}
		projected = append(projected, map[string]any{
			"id": task.ID, "title": firstNonEmpty(task.Title, task.Goal), "goal": task.Goal, "role": task.Role, "status": task.Status,
			"source": task.Source, "priority": task.Priority, "createdAt": task.CreatedAt, "updatedAt": task.UpdatedAt,
			"executionId": task.ExecutionID, "agentId": task.WorkerID,
		})
	}
	return map[string]any{
		"path":        filepath.ToSlash(filepath.Join(workspace, "runtime", "agent-tasks.jsonl")),
		"eventPath":   filepath.ToSlash(filepath.Join(workspace, "runtime", "agent-task-events.jsonl")),
		"profilePath": filepath.ToSlash(filepath.Join(workspace, "runtime", "agent-profiles.jsonl")),
		"profiles":    profiles, "tasks": projected, "events": events, "counts": counts,
	}
}

func projectBootstrapHarness(workspace string) map[string]any {
	server := &Server{workspace: workspace}
	cases := server.loadHarnessCases()
	results := server.loadWorkspaceHarnessResults()
	return map[string]any{
		"path":       filepath.ToSlash(filepath.Join(workspace, "runtime", "harness-cases.jsonl")),
		"resultPath": filepath.ToSlash(filepath.Join(workspace, "runtime", "harness-results.jsonl")),
		"cases":      cases, "results": results,
		"counts": map[string]int{"totalCases": len(cases), "totalResults": len(results)},
	}
}

func ProjectWorkspaceFiles(workspace string) ([]WorkspaceFileDTO, error) {
	if info, err := os.Stat(workspace); err != nil || !info.IsDir() {
		return []WorkspaceFileDTO{}, nil
	}
	files := []WorkspaceFileDTO{}
	err := filepath.WalkDir(workspace, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == workspace {
			return nil
		}
		if entry.IsDir() && shouldSkipWorkspaceDir(entry.Name()) {
			return filepath.SkipDir
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(workspace, path)
		if err != nil {
			return err
		}
		files = append(files, WorkspaceFileDTO{
			ID:         stableFileID("workspace", rel),
			Name:       entry.Name(),
			Path:       filepath.ToSlash(rel),
			Kind:       workspaceFileKind(entry, rel),
			Size:       info.Size(),
			ModifiedAt: info.ModTime().Unix(),
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.SliceStable(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return files, nil
}

func ReadWorkspaceFile(workspace string, relativePath string) (WorkspaceFileContentDTO, error) {
	path, rel, err := resolveWorkspaceFile(workspace, relativePath)
	if err != nil {
		return WorkspaceFileContentDTO{}, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return WorkspaceFileContentDTO{}, err
	}
	if info.IsDir() {
		return WorkspaceFileContentDTO{}, fmt.Errorf("workspace file is a directory: %s", rel)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return WorkspaceFileContentDTO{}, err
	}
	return WorkspaceFileContentDTO{
		Path:       filepath.ToSlash(rel),
		Name:       filepath.Base(path),
		Kind:       workspacePathKind(false, rel),
		Content:    string(data),
		Size:       info.Size(),
		ModifiedAt: info.ModTime().Unix(),
	}, nil
}

func WriteWorkspaceFile(workspace string, relativePath string, content string) (WorkspaceFileContentDTO, error) {
	path, _, err := resolveWorkspaceFile(workspace, relativePath)
	if err != nil {
		return WorkspaceFileContentDTO{}, err
	}
	info, err := os.Stat(path)
	if err == nil && info.IsDir() {
		return WorkspaceFileContentDTO{}, fmt.Errorf("workspace file is a directory: %s", relativePath)
	}
	if err != nil && !os.IsNotExist(err) {
		return WorkspaceFileContentDTO{}, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return WorkspaceFileContentDTO{}, err
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return WorkspaceFileContentDTO{}, err
	}
	return ReadWorkspaceFile(workspace, relativePath)
}

func SearchWorkspace(workspace string, query string) ([]WorkspaceSearchResultDTO, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return []WorkspaceSearchResultDTO{}, nil
	}
	if info, err := os.Stat(workspace); err != nil || !info.IsDir() {
		return []WorkspaceSearchResultDTO{}, nil
	}
	results := []WorkspaceSearchResultDTO{}
	lowerQuery := strings.ToLower(query)
	err := filepath.WalkDir(workspace, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == workspace {
			return nil
		}
		if entry.IsDir() {
			if shouldSkipWorkspaceDir(entry.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if len(results) >= 100 || !isSearchableWorkspaceFile(path) {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Size() > 1024*1024 {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if strings.IndexByte(string(data), 0) >= 0 {
			return nil
		}
		rel, err := filepath.Rel(workspace, path)
		if err != nil {
			return err
		}
		for index, line := range strings.Split(string(data), "\n") {
			if strings.Contains(strings.ToLower(line), lowerQuery) {
				results = append(results, WorkspaceSearchResultDTO{Path: filepath.ToSlash(rel), Line: index + 1, Text: strings.TrimSpace(line)})
				if len(results) >= 100 {
					return nil
				}
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return results, nil
}

func isSearchableWorkspaceFile(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".md", ".txt", ".log", ".json", ".yaml", ".yml", ".toml", ".go", ".ts", ".tsx", ".js", ".jsx", ".css", ".html", ".sh":
		return true
	default:
		return false
	}
}

func resolveWorkspaceFile(workspace string, relativePath string) (string, string, error) {
	if strings.TrimSpace(relativePath) == "" {
		return "", "", fmt.Errorf("workspace file path is required")
	}
	root, err := filepath.Abs(workspace)
	if err != nil {
		return "", "", err
	}
	candidate := filepath.Clean(relativePath)
	if filepath.IsAbs(candidate) {
		return "", "", fmt.Errorf("workspace file path must be relative")
	}
	path := filepath.Join(root, candidate)
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", "", err
	}
	rel, err := filepath.Rel(root, absPath)
	if err != nil {
		return "", "", err
	}
	if rel == "." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || rel == ".." {
		return "", "", fmt.Errorf("workspace file path must stay under workspace")
	}
	// Symlink escape: lexical checks stop ".." but not a link inside the
	// workspace pointing at an outside target (ReadFile/WriteFile/RemoveAll
	// follow links). Resolve the real path and re-verify containment. For a
	// not-yet-existing file (create path) resolve the deepest existing
	// ancestor — the parent chain is what a symlink attack would poison.
	resolved := absPath
	if _, statErr := os.Lstat(absPath); statErr == nil {
		resolved, err = filepath.EvalSymlinks(absPath)
		if err != nil {
			return "", "", fmt.Errorf("workspace file path cannot be resolved: %w", err)
		}
	} else {
		// Create path: walk up to the deepest existing ancestor and resolve
		// it, then re-append the missing tail. Missing intermediate dirs are
		// created later by the caller (MkdirAll) — what matters for the
		// symlink guard is that every existing hop is verified.
		existing := filepath.Dir(absPath)
		for {
			if _, statErr := os.Lstat(existing); statErr == nil {
				break
			}
			parent := filepath.Dir(existing)
			if parent == existing {
				break
			}
			existing = parent
		}

		resolvedExisting, evalErr := filepath.EvalSymlinks(existing)
		if evalErr != nil {
			return "", "", fmt.Errorf("workspace file directory cannot be resolved: %w", evalErr)
		}
		resolved = filepath.Join(resolvedExisting, strings.TrimPrefix(absPath, existing))
	}
	resolvedRel, err := filepath.Rel(root, resolved)
	if err != nil {
		return "", "", err
	}
	if resolvedRel == "." || strings.HasPrefix(resolvedRel, ".."+string(filepath.Separator)) || resolvedRel == ".." {
		return "", "", fmt.Errorf("workspace file path must stay under workspace")
	}
	return resolved, resolvedRel, nil
}

func ProjectWorkspace(workspace string) WorkspaceDTO {
	dto, err := ProjectWorkspaceSettings(workspace)
	if err == nil {
		return dto
	}
	return projectWorkspaceBase(workspace)
}

func projectWorkspaceBase(workspace string) WorkspaceDTO {
	name := filepath.Base(workspace)
	if strings.TrimSpace(name) == "." || strings.TrimSpace(name) == string(filepath.Separator) || strings.TrimSpace(name) == "" {
		name = workspace
	}
	status := "missing"
	if info, err := os.Stat(workspace); err == nil && info.IsDir() {
		status = "ready"
	}
	return WorkspaceDTO{Name: name, Path: workspace, Status: status, ConfigPath: filepath.Join(workspace, "workspace.yaml")}
}

func ProjectEvidenceGraph(ctx context.Context, workspace string, store *eventlog.SQLiteStore) (EvidenceGraphDTO, error) {
	nodesByID := map[string]GraphNodeDTO{}
	edgesByID := map[string]GraphEdgeDTO{}

	if store != nil {
		runIDs, err := store.ListRunIDs(ctx)
		if err != nil {
			return EvidenceGraphDTO{}, err
		}
		for _, runID := range runIDs {
			events, err := store.Replay(ctx, runID)
			if err != nil {
				return EvidenceGraphDTO{}, err
			}
			projectEventsIntoGraph(events, nodesByID, edgesByID)
		}
	}

	if err := projectWorkspaceFilesIntoGraph(workspace, nodesByID); err != nil {
		return EvidenceGraphDTO{}, err
	}

	nodes := make([]GraphNodeDTO, 0, len(nodesByID))
	for _, node := range nodesByID {
		nodes = append(nodes, node)
	}
	sort.SliceStable(nodes, func(i, j int) bool { return nodes[i].ID < nodes[j].ID })

	edges := make([]GraphEdgeDTO, 0, len(edgesByID))
	for _, edge := range edgesByID {
		edges = append(edges, edge)
	}
	sort.SliceStable(edges, func(i, j int) bool { return edges[i].ID < edges[j].ID })

	return EvidenceGraphDTO{Nodes: nodes, Edges: edges}, nil
}

func ProjectRuns(ctx context.Context, store *eventlog.SQLiteStore) ([]RunDTO, error) {
	if store == nil {
		return []RunDTO{}, nil
	}
	runIDs, err := store.ListRunIDs(ctx)
	if err != nil {
		return nil, err
	}
	runs := make([]RunDTO, 0, len(runIDs))
	for _, runID := range runIDs {
		events, err := store.Replay(ctx, runID)
		if err != nil {
			return nil, err
		}
		runs = append(runs, RunDTO{
			ID:        runID,
			Status:    runStatus(events),
			StartedAt: firstEventUnix(events),
		})
	}
	return runs, nil
}

func ProjectReplayEvents(ctx context.Context, store *eventlog.SQLiteStore) ([]ReplayEventDTO, error) {
	if store == nil {
		return []ReplayEventDTO{}, nil
	}
	runIDs, err := store.ListRunIDs(ctx)
	if err != nil {
		return nil, err
	}
	replayEvents := []ReplayEventDTO{}
	for _, runID := range runIDs {
		events, err := store.Replay(ctx, runID)
		if err != nil {
			return nil, err
		}
		for _, event := range events {
			payload := map[string]any{}
			_ = json.Unmarshal(event.Payload, &payload)
			eventID := fmt.Sprintf("%s-%d", event.RunID, event.ID)
			replayEvents = append(replayEvents, ReplayEventDTO{
				ID:          eventID,
				RunID:       event.RunID,
				Type:        string(event.Type),
				Lane:        string(eventlog.ReplayLaneForEventType(event.Type)),
				CreatedAt:   event.CreatedAt.Unix(),
				Detail:      replayEventDetail(event, payload),
				PayloadJSON: string(event.Payload),
				NodeID:      replayEventNodeID(event, payload),
				ExecutionID: replayEventExecutionID(event, payload),
				FilePath:    firstNonEmpty(stringField(payload, "path"), stringField(payload, "log_path")),
			})
		}
	}
	sort.SliceStable(replayEvents, func(i, j int) bool {
		if replayEvents[i].CreatedAt == replayEvents[j].CreatedAt {
			return replayEvents[i].ID < replayEvents[j].ID
		}
		return replayEvents[i].CreatedAt < replayEvents[j].CreatedAt
	})
	return replayEvents, nil
}

func ProjectToolchain(workspace string) (ToolchainDTO, error) {
	root := filepath.Join(workspace, "toolchain")
	if info, err := os.Stat(root); err != nil || !info.IsDir() {
		return ToolchainDTO{Status: "missing", Packs: []ToolchainPackDTO{}}, nil
	}
	manifests, err := toolchain.LoadManifests(root)
	if err != nil {
		return ToolchainDTO{}, err
	}
	packs := make([]ToolchainPackDTO, 0, len(manifests))
	for _, manifest := range manifests {
		packs = append(packs, ToolchainPackDTO{
			ID:           strings.TrimSuffix(filepath.Base(filepath.Dir(manifest.Path)), "-pack"),
			Name:         manifest.Name,
			Version:      manifest.Version,
			Status:       "unknown",
			ToolsReady:   0,
			ManifestPath: manifest.Path,
			Capabilities: append([]string{}, manifest.Capabilities...),
			Runner:       manifest.Runner,
			RunCommand:   append([]string{}, manifest.RunCommand...),
			RunTimeoutMS: manifest.RunTimeoutMS,
			Parser:       manifest.Parser,
			Healthcheck:  manifest.Healthcheck,
			HealthCommand: append([]string{},
				manifest.HealthCommand...),
			Dependencies: projectToolchainDependencies(manifest.Dependencies),
		})
	}
	return ToolchainDTO{Status: "unknown", ToolsReady: 0, Packs: packs}, nil
}

func ProjectToolchainActions(toolchainDTO ToolchainDTO) []ToolchainActionDTO {
	actions := []ToolchainActionDTO{}
	for _, pack := range toolchainDTO.Packs {
		if len(pack.RunCommand) == 0 {
			continue
		}
		action := projectToolchainAction(pack)
		actions = append(actions, action)
	}
	sort.SliceStable(actions, func(i, j int) bool { return actions[i].ID < actions[j].ID })
	return actions
}

func projectToolchainAction(pack ToolchainPackDTO) ToolchainActionDTO {
	label := pack.Name
	if strings.TrimSpace(label) == "" {
		label = pack.ID
	}
	status := pack.Status
	if strings.TrimSpace(status) == "" {
		status = "unknown"
	}
	return ToolchainActionDTO{
		ID:           pack.ID,
		PackID:       pack.ID,
		PackName:     pack.Name,
		Label:        label,
		Status:       status,
		Command:      append([]string{}, pack.RunCommand...),
		CommandText:  commandText(pack.RunCommand),
		RunTimeoutMS: pack.RunTimeoutMS,
		ManifestPath: pack.ManifestPath,
		Capabilities: append([]string{}, pack.Capabilities...),
		Runner:       pack.Runner,
		Parser:       pack.Parser,
		ArgumentHint: strings.Join(pack.Capabilities, ", "),
	}
}

func commandText(parts []string) string {
	quoted := make([]string, 0, len(parts))
	for _, part := range parts {
		quoted = append(quoted, shellQuote(part))
	}
	return strings.Join(quoted, " ")
}

func shellQuote(value string) string {
	if value == "" {
		return "''"
	}
	if !strings.ContainsAny(value, " \t\n'\"\\$`") {
		return value
	}
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

func projectToolchainDependencies(dependencies []toolchain.Dependency) []ToolchainDependencyDTO {
	projected := make([]ToolchainDependencyDTO, 0, len(dependencies))
	for _, dependency := range dependencies {
		kind := dependency.Kind
		if strings.TrimSpace(kind) == "" {
			kind = "binary"
		}
		projected = append(projected, ToolchainDependencyDTO{
			Name:           dependency.Name,
			Kind:           kind,
			Version:        dependency.Version,
			Source:         dependency.Source,
			Optional:       dependency.Optional,
			DependsOn:      append([]string{}, dependency.DependsOn...),
			InstallHint:    dependency.InstallHint,
			InstallCommand: append([]string{}, dependency.InstallCommand...),
		})
	}
	return projected
}

func ProjectProvider(workspace string) (ProviderDTO, error) {
	config, err := ProjectProviderConfig(workspace)
	if err != nil {
		return ProviderDTO{}, err
	}
	return config.Active, nil
}

func ProjectProviderConfig(workspace string) (ProviderConfigDTO, error) {
	path := filepath.Join(workspace, "providers.yaml")
	config, err := app.LoadProviderConfig(path)
	if err != nil {
		if os.IsNotExist(err) {
			return ProviderConfigDTO{
				Path:            path,
				RoutingStrategy: "first_enabled",
				Providers:       []ProviderDTO{},
				Active:          ProviderDTO{Status: "missing"},
			}, nil
		}
		return ProviderConfigDTO{}, err
	}
	routingStrategy := providerRoutingStrategyDTO(config.Routing.Strategy)
	providers := make([]ProviderDTO, 0, len(config.Providers))
	active := ProviderDTO{Status: "missing", RoutingStrategy: routingStrategy}
	for _, provider := range config.Providers {
		dto := projectProviderDTO(provider, routingStrategy)
		providers = append(providers, dto)
		if provider.Enabled && active.Status == "missing" {
			active = dto
		}
	}
	return ProviderConfigDTO{
		Path:            path,
		RoutingStrategy: routingStrategy,
		Providers:       providers,
		Active:          active,
	}, nil
}

func projectProviderDTO(provider app.Provider, routingStrategy string) ProviderDTO {
	status := "disabled"
	if provider.Enabled {
		status = "ready"
	}
	return ProviderDTO{
		Name:                provider.Name,
		Type:                provider.Type,
		Model:               provider.Model,
		BaseURL:             provider.BaseURL,
		APIKeyEnv:           provider.APIKeyEnv,
		Enabled:             provider.Enabled,
		TimeoutMS:           provider.TimeoutMS,
		ContextWindowTokens: provider.ContextWindowTokens,
		Capabilities:        append([]string{}, provider.Capabilities...),
		RoutingStrategy:     routingStrategy,
		Status:              status,
	}
}

func providerRoutingStrategyDTO(strategy string) string {
	strategy = strings.TrimSpace(strategy)
	if strategy == "" {
		return "first_enabled"
	}
	return strategy
}

func ProjectReports(workspace string) ([]ReportDTO, error) {
	root := filepath.Join(workspace, "reports")
	if info, err := os.Stat(root); err != nil || !info.IsDir() {
		return []ReportDTO{}, nil
	}
	reports := []ReportDTO{}
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || strings.ToLower(filepath.Ext(entry.Name())) != ".md" {
			return nil
		}
		rel, err := filepath.Rel(workspace, path)
		if err != nil {
			return err
		}
		reports = append(reports, ReportDTO{
			ID:     stableFileID("report", rel),
			Title:  reportTitle(path),
			Status: "ready",
			Path:   rel,
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.SliceStable(reports, func(i, j int) bool { return reports[i].Path < reports[j].Path })
	return reports, nil
}

func ProjectReportFindings(workspace string, graph EvidenceGraphDTO) ([]ReportFindingDTO, error) {
	findingsByID := map[string]ReportFindingDTO{}

	for _, node := range graph.Nodes {
		if !strings.EqualFold(node.Type, "Finding") {
			continue
		}
		if node.SourceKind == "findings" {
			continue
		}
		finding := ReportFindingDTO{
			ID:            node.ID,
			Title:         node.Label,
			Severity:      normalizeSeverityString(firstNonEmpty(stringField(node.Data, "severity"), stringField(node.Data, "risk"))),
			Endpoint:      firstNonEmpty(stringField(node.Data, "endpoint"), stringField(node.Data, "url"), stringField(node.Data, "target"), stringField(node.Data, "path")),
			Source:        firstNonEmpty(node.SourceKind, "graph"),
			NodeID:        node.ID,
			FilePath:      node.Path,
			RunID:         node.RunID,
			ExecutionID:   node.ExecutionID,
			ReplayEventID: node.ReplayEventID,
		}
		if finding.Severity == "" {
			finding.Severity = "info"
		}
		findingsByID[finding.ID] = finding
	}

	root := filepath.Join(workspace, "findings")
	if info, err := os.Stat(root); err != nil || !info.IsDir() {
		return sortedReportFindings(findingsByID), nil
	}
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(workspace, path)
		if err != nil {
			return err
		}
		for _, finding := range findingsFromFile(path, filepath.ToSlash(rel)) {
			if strings.TrimSpace(finding.ID) == "" {
				finding.ID = stableFileID("finding", rel+"-"+finding.Title)
			}
			if finding.Source == "" {
				finding.Source = "file"
			}
			if finding.FilePath == "" {
				finding.FilePath = filepath.ToSlash(rel)
			}
			if finding.NodeID == "" {
				finding.NodeID = workspaceCategoryNodeID("findings", rel)
			}
			findingsByID[finding.ID] = finding
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return sortedReportFindings(findingsByID), nil
}

func projectEventsIntoGraph(events []eventlog.Event, nodes map[string]GraphNodeDTO, edges map[string]GraphEdgeDTO) {
	for _, event := range events {
		payload := map[string]any{}
		_ = json.Unmarshal(event.Payload, &payload)

		switch event.Type {
		case eventlog.EventFact:
			id := stringField(payload, "fact_id")
			if id == "" {
				continue
			}
			kind := evidenceType(stringField(payload, "kind"))
			label := firstNonEmpty(stringField(payload, "statement"), stringField(payload, "message"), id)
			nodes[id] = GraphNodeDTO{ID: id, Type: kind, Label: label, SourceKind: stringField(payload, "source"), CreatedAt: event.CreatedAt.Unix(), RunID: event.RunID, ReplayEventID: fmt.Sprintf("%s-%d", event.RunID, event.ID), Path: stringField(payload, "path"), Data: payload}
		case eventlog.EventIntent:
			id := stringField(payload, "intent_id")
			if id == "" {
				continue
			}
			label := firstNonEmpty(stringField(payload, "goal"), stringField(payload, "message"), id)
			nodes[id] = GraphNodeDTO{ID: id, Type: "Action", Label: label, SourceKind: stringField(payload, "kind"), CreatedAt: event.CreatedAt.Unix(), RunID: event.RunID, ReplayEventID: fmt.Sprintf("%s-%d", event.RunID, event.ID), Data: payload}
			for _, sourceID := range stringSliceField(payload, "source_facts") {
				addEdge(edges, id, sourceID, "requires")
			}
			if result := stringField(payload, "result_fact"); result != "" {
				addEdge(edges, result, id, "derived_from")
			}
		case eventlog.EventTool:
			id := stringField(payload, "execution_id")
			if id == "" {
				continue
			}
			label := firstNonEmpty(stringField(payload, "tool"), stringField(payload, "action"), id)
			nodes[id] = GraphNodeDTO{ID: id, Type: "Action", Label: label, SourceKind: stringField(payload, "worker_capability"), CreatedAt: event.CreatedAt.Unix(), RunID: event.RunID, ExecutionID: stringField(payload, "execution_id"), ReplayEventID: fmt.Sprintf("%s-%d", event.RunID, event.ID), Path: stringField(payload, "log_path"), Data: payload}
			for _, sourceID := range stringSliceField(payload, "input_node_ids") {
				addEdge(edges, id, sourceID, "requires")
			}
		}
	}
}

func projectWorkspaceFilesIntoGraph(workspace string, nodes map[string]GraphNodeDTO) error {
	for _, dir := range []struct {
		name     string
		nodeType string
	}{
		{name: "evidence", nodeType: "Artifact"},
		{name: "findings", nodeType: "Artifact"},
		{name: "artifacts", nodeType: "Artifact"},
		{name: "attempts", nodeType: "Action"},
	} {
		root := filepath.Join(workspace, dir.name)
		if info, err := os.Stat(root); err != nil || !info.IsDir() {
			continue
		}
		if err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() {
				return nil
			}
			rel, err := filepath.Rel(workspace, path)
			if err != nil {
				return err
			}
			relWithinDir, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			id := workspaceCategoryNodeID(dir.name, relWithinDir)
			if _, exists := nodes[id]; exists {
				return nil
			}
			node := GraphNodeDTO{
				ID:         id,
				Type:       dir.nodeType,
				Label:      rel,
				SourceKind: dir.name,
				Path:       filepath.ToSlash(rel),
				Data:       map[string]any{"path": rel},
			}
			if dir.name == "findings" {
				node = projectFindingFileGraphNode(path, filepath.ToSlash(rel), id, node)
			}
			nodes[id] = node
			return nil
		}); err != nil {
			return err
		}
	}
	return nil
}

func projectFindingFileGraphNode(path string, rel string, id string, fallback GraphNodeDTO) GraphNodeDTO {
	findings := findingsFromFile(path, rel)
	if len(findings) == 0 {
		return fallback
	}
	finding := findings[0]
	data := map[string]any{"path": rel}
	if finding.Title != "" {
		data["title"] = finding.Title
	}
	if finding.Severity != "" {
		data["severity"] = finding.Severity
	}
	if finding.Endpoint != "" {
		data["endpoint"] = finding.Endpoint
	}
	if finding.ExecutionID != "" {
		data["execution_id"] = finding.ExecutionID
	}
	if finding.RunID != "" {
		data["run_id"] = finding.RunID
	}
	if finding.ReplayEventID != "" {
		data["replay_event_id"] = finding.ReplayEventID
	}
	if finding.FilePath != "" {
		data["file_path"] = finding.FilePath
	}
	return GraphNodeDTO{
		ID:            id,
		Type:          "Finding",
		Label:         firstNonEmpty(finding.Title, rel),
		SourceKind:    "findings",
		Path:          rel,
		RunID:         finding.RunID,
		ExecutionID:   finding.ExecutionID,
		ReplayEventID: finding.ReplayEventID,
		Data:          data,
	}
}

func shouldSkipWorkspaceDir(name string) bool {
	switch name {
	case ".git", "node_modules", "dist", "build", ".cache":
		return true
	default:
		return false
	}
}

func workspaceFileKind(entry os.DirEntry, rel string) string {
	return workspacePathKind(entry.IsDir(), rel)
}

func workspacePathKind(isDir bool, rel string) string {
	if isDir {
		return "directory"
	}
	switch strings.ToLower(filepath.Ext(rel)) {
	case ".md":
		return "markdown"
	case ".yaml", ".yml":
		return "yaml"
	case ".json":
		return "json"
	case ".log", ".txt":
		return "text"
	default:
		return "file"
	}
}

func addEdge(edges map[string]GraphEdgeDTO, from, to, edgeType string) {
	from = strings.TrimSpace(from)
	to = strings.TrimSpace(to)
	if from == "" || to == "" {
		return
	}
	id := from + "-" + edgeType + "-" + to
	edges[id] = GraphEdgeDTO{ID: id, From: from, To: to, Type: edgeType}
}

func workflowStatusFrom(runs []RunDTO, graph EvidenceGraphDTO) string {
	if len(runs) > 0 {
		return "active"
	}
	if len(graph.Nodes) > 0 {
		return "ready"
	}
	return "empty"
}

func workflowNextActionFrom(runs []RunDTO, graph EvidenceGraphDTO) string {
	if len(runs) > 0 {
		return "inspect_output"
	}
	if len(graph.Nodes) > 0 {
		return "continue_execution"
	}
	return "open_workspace_files"
}

func runStatus(events []eventlog.Event) string {
	if len(events) == 0 {
		return "unknown"
	}
	for i := len(events) - 1; i >= 0; i-- {
		payload := map[string]any{}
		if err := json.Unmarshal(events[i].Payload, &payload); err != nil {
			continue
		}
		if status := stringField(payload, "execution_status"); status != "" {
			return normalizeStatus(status)
		}
		if status := stringField(payload, "worker_status"); status != "" {
			return normalizeStatus(status)
		}
	}
	return "unknown"
}

func normalizeStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "queued", "waiting":
		return "queued"
	case "started", "running":
		return "running"
	case "failed", "error":
		return "failed"
	case "stopped", "canceled", "cancelled":
		return "stopped"
	case "completed", "complete", "done":
		return "completed"
	default:
		return strings.ToLower(strings.TrimSpace(status))
	}
}

func firstEventUnix(events []eventlog.Event) int64 {
	for _, event := range events {
		if !event.CreatedAt.IsZero() {
			return event.CreatedAt.Unix()
		}
	}
	return 0
}

func evidenceType(kind string) string {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "target":
		return "Target"
	case "surface":
		return "Surface"
	case "finding":
		return "Finding"
	case "identity":
		return "Identity"
	case "attempt", "action", "suggestion":
		return "Action"
	default:
		return "Artifact"
	}
}

func reportTitle(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "#") {
			return strings.TrimSpace(strings.TrimLeft(line, "#"))
		}
	}
	return strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
}

func replayEventDetail(event eventlog.Event, payload map[string]any) string {
	return firstNonEmpty(
		stringField(payload, "statement"),
		stringField(payload, "goal"),
		stringField(payload, "message"),
		stringField(payload, "tool"),
		stringField(payload, "action"),
		stringField(payload, "command"),
		stringField(payload, "execution_status"),
		stringField(payload, "worker_status"),
		stringField(payload, "fact_id"),
		stringField(payload, "intent_id"),
		string(event.Type),
	)
}

func replayEventNodeID(event eventlog.Event, payload map[string]any) string {
	switch event.Type {
	case eventlog.EventFact:
		return stringField(payload, "fact_id")
	case eventlog.EventIntent:
		return stringField(payload, "intent_id")
	case eventlog.EventTool:
		return stringField(payload, "execution_id")
	default:
		return ""
	}
}

func replayEventExecutionID(event eventlog.Event, payload map[string]any) string {
	return stringField(payload, "execution_id")
}

func findingsFromFile(path string, rel string) []ReportFindingDTO {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	switch strings.ToLower(filepath.Ext(path)) {
	case ".json":
		var value any
		if err := json.Unmarshal(data, &value); err != nil {
			return nil
		}
		return collectReportFindings(value, rel)
	}
	return nil
}

func collectReportFindings(value any, rel string) []ReportFindingDTO {
	switch typed := value.(type) {
	case []any:
		findings := []ReportFindingDTO{}
		for index, item := range typed {
			for _, finding := range collectReportFindings(item, fmt.Sprintf("%s-%d", rel, index+1)) {
				findings = append(findings, finding)
			}
		}
		return findings
	case map[string]any:
		for _, key := range []string{"findings", "results", "pocs"} {
			if nested, ok := typed[key]; ok {
				if findings := collectReportFindings(nested, rel); len(findings) > 0 {
					return findings
				}
			}
		}

		info := mapField(typed, "info")
		title := firstNonEmpty(
			stringField(typed, "title"),
			stringField(typed, "name"),
			stringField(typed, "statement"),
			stringField(typed, "message"),
			stringField(info, "name"),
			stringField(typed, "template-id"),
			stringField(typed, "id"),
		)
		endpoint := firstNonEmpty(
			stringField(typed, "endpoint"),
			stringField(typed, "url"),
			stringField(typed, "matched-at"),
			stringField(typed, "matched_at"),
			stringField(typed, "matchedAt"),
			stringField(typed, "matchedURL"),
			stringField(typed, "target"),
			stringField(typed, "data"),
			stringField(typed, "host"),
			stringField(typed, "path"),
			rel,
		)
		if title == "" || !isStructuredFindingPayload(typed, info) {
			return nil
		}
		severity := normalizeSeverityString(firstNonEmpty(stringField(typed, "severity"), stringField(info, "severity")))
		if severity == "" {
			severity = "info"
		}
		return []ReportFindingDTO{{
			ID:            stableFileID("finding", firstNonEmpty(stringField(typed, "id"), stringField(typed, "template-id"), rel+"-"+title)),
			Title:         title,
			Severity:      severity,
			Endpoint:      endpoint,
			Path:          rel,
			Source:        "file",
			NodeID:        reportFindingNodeID(typed, rel),
			FilePath:      reportFindingFilePath(typed, rel),
			RunID:         firstNonEmpty(stringField(typed, "run_id"), stringField(typed, "runId")),
			ExecutionID:   firstNonEmpty(stringField(typed, "execution_id"), stringField(typed, "executionId")),
			ReplayEventID: firstNonEmpty(stringField(typed, "replay_event_id"), stringField(typed, "replayEventId")),
		}}
	default:
		return nil
	}
}

func isStructuredFindingPayload(payload map[string]any, info map[string]any) bool {
	for _, key := range []string{"kind", "type", "category"} {
		if strings.EqualFold(stringField(payload, key), "finding") {
			return true
		}
	}
	if normalizeSeverityString(firstNonEmpty(stringField(payload, "severity"), stringField(payload, "risk"), stringField(info, "severity"))) != "" {
		return true
	}
	for _, key := range []string{"endpoint", "url", "matched-at", "matched_at", "matchedAt", "matchedURL", "target", "host"} {
		if stringField(payload, key) != "" {
			return true
		}
	}
	return false
}

func reportFindingNodeID(payload map[string]any, rel string) string {
	return firstNonEmpty(
		stringField(payload, "node_id"),
		stringField(payload, "nodeId"),
		stringField(payload, "graph_node_id"),
		stringField(payload, "graphNodeId"),
		workspaceCategoryNodeID("findings", rel),
	)
}

func reportFindingFilePath(payload map[string]any, rel string) string {
	return firstNonEmpty(
		stringField(payload, "file_path"),
		stringField(payload, "filePath"),
		stringField(payload, "evidence_path"),
		stringField(payload, "evidencePath"),
		stringField(payload, "artifact_path"),
		stringField(payload, "artifactPath"),
		rel,
	)
}

func sortedReportFindings(findingsByID map[string]ReportFindingDTO) []ReportFindingDTO {
	findings := make([]ReportFindingDTO, 0, len(findingsByID))
	for _, finding := range findingsByID {
		findings = append(findings, finding)
	}
	sort.SliceStable(findings, func(i, j int) bool {
		if findings[i].Severity == findings[j].Severity {
			return findings[i].Title < findings[j].Title
		}
		return severityRank(findings[i].Severity) > severityRank(findings[j].Severity)
	})
	return findings
}

func severityRank(severity string) int {
	switch normalizeSeverityString(severity) {
	case "critical":
		return 5
	case "high":
		return 4
	case "medium":
		return 3
	case "low":
		return 2
	case "info":
		return 1
	default:
		return 0
	}
}

func normalizeSeverityString(severity string) string {
	switch strings.ToLower(strings.TrimSpace(severity)) {
	case "critical", "crit", "严重":
		return "critical"
	case "high", "高":
		return "high"
	case "medium", "med", "中":
		return "medium"
	case "low", "低":
		return "low"
	case "info", "informational", "信息":
		return "info"
	default:
		return ""
	}
}

func mapField(payload map[string]any, key string) map[string]any {
	value, ok := payload[key]
	if !ok {
		return nil
	}
	typed, _ := value.(map[string]any)
	return typed
}

func stableFileID(prefix, value string) string {
	value = strings.Trim(strings.ReplaceAll(value, string(filepath.Separator), "/"), "/")
	value = strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			return r
		default:
			return '-'
		}
	}, value)
	value = strings.Trim(value, "-")
	if value == "" {
		value = "root"
	}
	return prefix + "-" + value
}

func workspaceCategoryNodeID(category, value string) string {
	value = filepath.ToSlash(strings.TrimSpace(value))
	value = strings.TrimPrefix(value, category+"/")
	return stableFileID(category, value)
}

func stringField(payload map[string]any, key string) string {
	value, ok := payload[key]
	if !ok {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case fmt.Stringer:
		return strings.TrimSpace(typed.String())
	default:
		return ""
	}
}

func stringSliceField(payload map[string]any, key string) []string {
	value, ok := payload[key]
	if !ok {
		return nil
	}
	var out []string
	switch typed := value.(type) {
	case []string:
		out = append(out, typed...)
	case []any:
		for _, item := range typed {
			if text, ok := item.(string); ok && strings.TrimSpace(text) != "" {
				out = append(out, strings.TrimSpace(text))
			}
		}
	}
	return out
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
