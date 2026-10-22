package api

type GUIBootstrapDTO struct {
	Workspace      WorkspaceDTO         `json:"workspace"`
	Mission        MissionDTO           `json:"mission"`
	Workflow       WorkflowDTO          `json:"workflow"`
	Provider       ProviderDTO          `json:"provider"`
	Toolchain      ToolchainDTO         `json:"toolchain"`
	ToolActions    []ToolchainActionDTO `json:"toolActions"`
	Graph          EvidenceGraphDTO     `json:"graph"`
	Runs           []RunDTO             `json:"runs"`
	ReplayEvents   []ReplayEventDTO     `json:"replayEvents"`
	Executions     []ExecutionDTO       `json:"executions"`
	Reports        []ReportDTO          `json:"reports"`
	ReportFindings []ReportFindingDTO   `json:"reportFindings"`
	Files          []WorkspaceFileDTO   `json:"files"`
	Blackboard     any                  `json:"blackboard"`
	AgentState     any                  `json:"agentState"`
	Harness        any                  `json:"harness"`
	Settings       SettingsDTO          `json:"settings"`
	Backend        BackendDTO           `json:"backend"`
}

type WorkspaceDTO struct {
	Name        string   `json:"name"`
	Path        string   `json:"path"`
	Status      string   `json:"status"`
	ConfigPath  string   `json:"configPath,omitempty"`
	Description string   `json:"description,omitempty"`
	Operator    string   `json:"operator,omitempty"`
	Tags        []string `json:"tags,omitempty"`
}

type WorkspaceSettingsUpdateRequestDTO struct {
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Operator    string   `json:"operator,omitempty"`
	Tags        []string `json:"tags,omitempty"`
}

type MissionDTO struct {
	Name    string `json:"name"`
	Goal    string `json:"goal"`
	Command string `json:"command"`
	Path    string `json:"path"`
	Status  string `json:"status"`
}

type MissionUpdateRequestDTO struct {
	Name    string `json:"name"`
	Goal    string `json:"goal"`
	Command string `json:"command"`
}

type MissionStartResponseDTO struct {
	Mission   MissionDTO   `json:"mission"`
	Execution ExecutionDTO `json:"execution"`
}

type WorkflowDTO struct {
	Status     string `json:"status"`
	NextAction string `json:"nextAction"`
}

type ProviderDTO struct {
	Name                string   `json:"name"`
	Type                string   `json:"type,omitempty"`
	Model               string   `json:"model"`
	BaseURL             string   `json:"baseUrl,omitempty"`
	APIKeyEnv           string   `json:"apiKeyEnv,omitempty"`
	Enabled             bool     `json:"enabled"`
	TimeoutMS           int      `json:"timeoutMs,omitempty"`
	ContextWindowTokens int      `json:"contextWindowTokens,omitempty"`
	Capabilities        []string `json:"capabilities,omitempty"`
	RoutingStrategy     string   `json:"routingStrategy,omitempty"`
	Status              string   `json:"status"`
}

type ProviderConfigDTO struct {
	Path            string        `json:"path"`
	RoutingStrategy string        `json:"routingStrategy"`
	Providers       []ProviderDTO `json:"providers"`
	Active          ProviderDTO   `json:"active"`
}

type ProviderUpdateRequestDTO struct {
	Name                string   `json:"name"`
	Type                string   `json:"type,omitempty"`
	Model               string   `json:"model"`
	BaseURL             string   `json:"baseUrl"`
	APIKeyEnv           string   `json:"apiKeyEnv"`
	Enabled             bool     `json:"enabled"`
	TimeoutMS           int      `json:"timeoutMs,omitempty"`
	ContextWindowTokens int      `json:"contextWindowTokens,omitempty"`
	Capabilities        []string `json:"capabilities,omitempty"`
	RoutingStrategy     string   `json:"routingStrategy,omitempty"`
}

type ToolchainDTO struct {
	Status     string             `json:"status"`
	ToolsReady int                `json:"toolsReady"`
	Packs      []ToolchainPackDTO `json:"packs"`
}

type ToolchainPackDTO struct {
	ID            string                   `json:"id"`
	Name          string                   `json:"name"`
	Version       string                   `json:"version,omitempty"`
	Status        string                   `json:"status"`
	ToolsReady    int                      `json:"toolsReady"`
	ManifestPath  string                   `json:"manifestPath,omitempty"`
	Capabilities  []string                 `json:"capabilities,omitempty"`
	Runner        string                   `json:"runner,omitempty"`
	RunCommand    []string                 `json:"runCommand,omitempty"`
	RunTimeoutMS  int                      `json:"runTimeoutMs,omitempty"`
	Parser        string                   `json:"parser,omitempty"`
	Healthcheck   string                   `json:"healthcheck,omitempty"`
	HealthCommand []string                 `json:"healthCommand,omitempty"`
	Dependencies  []ToolchainDependencyDTO `json:"dependencies,omitempty"`
}

type ToolchainDependencyDTO struct {
	Name           string   `json:"name"`
	Kind           string   `json:"kind,omitempty"`
	Version        string   `json:"version,omitempty"`
	Source         string   `json:"source,omitempty"`
	Optional       bool     `json:"optional"`
	DependsOn      []string `json:"dependsOn,omitempty"`
	InstallHint    string   `json:"installHint,omitempty"`
	InstallCommand []string `json:"installCommand,omitempty"`
}

type ToolchainActionDTO struct {
	ID           string   `json:"id"`
	PackID       string   `json:"packId"`
	PackName     string   `json:"packName"`
	Label        string   `json:"label"`
	Status       string   `json:"status"`
	Command      []string `json:"command"`
	CommandText  string   `json:"commandText"`
	RunTimeoutMS int      `json:"runTimeoutMs,omitempty"`
	ManifestPath string   `json:"manifestPath,omitempty"`
	Capabilities []string `json:"capabilities,omitempty"`
	Runner       string   `json:"runner,omitempty"`
	Parser       string   `json:"parser,omitempty"`
	ArgumentHint string   `json:"argumentHint,omitempty"`
}

type ToolchainActionRunRequestDTO struct {
	Arguments string            `json:"arguments,omitempty"`
	CWD       string            `json:"cwd,omitempty"`
	Env       map[string]string `json:"env,omitempty"`
}

type ToolchainActionRunResponseDTO struct {
	Action    ToolchainActionDTO `json:"action"`
	Execution ExecutionDTO       `json:"execution"`
}

type ToolchainPackUpdateRequestDTO struct {
	Name          string   `json:"name"`
	Version       string   `json:"version"`
	Capabilities  []string `json:"capabilities"`
	Runner        string   `json:"runner"`
	RunCommand    []string `json:"runCommand,omitempty"`
	RunTimeoutMS  int      `json:"runTimeoutMs,omitempty"`
	Parser        string   `json:"parser"`
	Healthcheck   string   `json:"healthcheck"`
	HealthCommand []string `json:"healthCommand,omitempty"`
}

type EvidenceGraphDTO struct {
	Nodes []GraphNodeDTO `json:"nodes"`
	Edges []GraphEdgeDTO `json:"edges"`
}

type GraphExportDTO struct {
	FilePath   string `json:"filePath"`
	NodeCount  int    `json:"nodeCount"`
	EdgeCount  int    `json:"edgeCount"`
	ModifiedAt int64  `json:"modifiedAt"`
}

type GraphNodeDTO struct {
	ID            string         `json:"id"`
	Type          string         `json:"type"`
	Label         string         `json:"label"`
	SourceKind    string         `json:"sourceKind,omitempty"`
	CreatedAt     int64          `json:"createdAt,omitempty"`
	Path          string         `json:"path,omitempty"`
	RunID         string         `json:"runId,omitempty"`
	ExecutionID   string         `json:"executionId,omitempty"`
	ReplayEventID string         `json:"replayEventId,omitempty"`
	Data          map[string]any `json:"data,omitempty"`
}

type GraphEdgeDTO struct {
	ID   string `json:"id"`
	From string `json:"from"`
	To   string `json:"to"`
	Type string `json:"type"`
}

type RunDTO struct {
	ID        string `json:"id"`
	Status    string `json:"status"`
	StartedAt int64  `json:"startedAt"`
}

type ReplayEventDTO struct {
	ID          string `json:"id"`
	RunID       string `json:"runId"`
	Type        string `json:"type"`
	Lane        string `json:"lane"`
	CreatedAt   int64  `json:"createdAt"`
	Detail      string `json:"detail"`
	NodeID      string `json:"nodeId,omitempty"`
	ExecutionID string `json:"executionId,omitempty"`
	FilePath    string `json:"filePath,omitempty"`
}

type ExecutionCreateRequestDTO struct {
	Command string            `json:"command"`
	Args    []string          `json:"args,omitempty"`
	CWD     string            `json:"cwd,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
	RetryOf string            `json:"retryOf,omitempty"`
}

type ExecutionDTO struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	RetryOf     string `json:"retryOf,omitempty"`
	Command     string `json:"command"`
	Status      string `json:"status"`
	StartedAt   int64  `json:"startedAt"`
	CompletedAt int64  `json:"completedAt,omitempty"`
	ExitCode    *int   `json:"exitCode,omitempty"`
	LogPath     string `json:"logPath"`
	ArtifactDir string `json:"artifactDir"`
}

type ExecutionLogDTO struct {
	ID      string `json:"id"`
	Path    string `json:"path"`
	Content string `json:"content"`
}

type ExecutionArtifactDTO struct {
	Path           string `json:"path"`
	FilePath       string `json:"filePath"`
	Name           string `json:"name"`
	Size           int64  `json:"size"`
	ModifiedAt     int64  `json:"modifiedAt"`
	ContentPreview string `json:"contentPreview,omitempty"`
}

type ReportDTO struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Status string `json:"status"`
	Path   string `json:"path"`
}

type ReportExportDTO struct {
	FilePath     string `json:"filePath"`
	FindingCount int    `json:"findingCount"`
	ReportCount  int    `json:"reportCount"`
	ModifiedAt   int64  `json:"modifiedAt"`
}

type ReportFindingDTO struct {
	ID            string `json:"id"`
	Title         string `json:"title"`
	Severity      string `json:"severity"`
	Endpoint      string `json:"endpoint"`
	Path          string `json:"path,omitempty"`
	Source        string `json:"source,omitempty"`
	NodeID        string `json:"nodeId,omitempty"`
	FilePath      string `json:"filePath,omitempty"`
	RunID         string `json:"runId,omitempty"`
	ExecutionID   string `json:"executionId,omitempty"`
	ReplayEventID string `json:"replayEventId,omitempty"`
}

type WorkspaceFileDTO struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Path       string `json:"path"`
	Kind       string `json:"kind"`
	Size       int64  `json:"size"`
	ModifiedAt int64  `json:"modifiedAt"`
}

type WorkspaceFileContentDTO struct {
	Path       string `json:"path"`
	Name       string `json:"name"`
	Kind       string `json:"kind"`
	Content    string `json:"content"`
	Size       int64  `json:"size"`
	ModifiedAt int64  `json:"modifiedAt"`
}

type WorkspaceSearchResultDTO struct {
	Path string `json:"path"`
	Line int    `json:"line"`
	Text string `json:"text"`
}

type SettingsDTO struct {
	Theme         string `json:"theme"`
	APIBaseURL    string `json:"apiBaseUrl"`
	LastWorkspace string `json:"lastWorkspace"`
}

type BackendDTO struct {
	Status       string `json:"status"`
	EventsStream string `json:"eventsStream"`
}
