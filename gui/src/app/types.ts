export type IDEMode = 'mission' | 'graph' | 'replay' | 'report' | 'settings';

export type Severity = 'critical' | 'high' | 'medium' | 'low' | 'info';

export interface WorkspaceItem {
  id: string;
  label: string;
  meta: string;
  active?: boolean;
  filePath?: string;
  kind?: string;
}

export interface WorkspaceFileContent {
  path: string;
  name: string;
  kind: string;
  content: string;
  size?: number;
  modifiedAt?: number;
}

export interface WorkspaceSearchResult {
  path: string;
  line: number;
  text: string;
}

export interface WorkspaceSettings {
  name: string;
  path: string;
  status: string;
  configPath: string;
  description: string;
  operator: string;
  tags: string[];
}

export interface MissionState {
  name: string;
  goal: string;
  command: string;
  path: string;
  status: string;
}

export interface ProviderSettings {
  name: string;
  type: string;
  model: string;
  baseUrl: string;
  apiKeyEnv: string;
  enabled: boolean;
  timeoutMs: number;
  contextWindowTokens: number;
  capabilities: string[];
  routingStrategy: string;
  status: string;
}

export interface ToolchainPackSettings {
  id: string;
  name: string;
  version: string;
  status: string;
  toolsReady: number;
  manifestPath: string;
  capabilities: string[];
  runner: string;
  runCommand: string[];
  runTimeoutMs: number;
  parser: string;
  healthcheck: string;
  healthCommand: string[];
}

export interface ToolAction {
  id: string;
  packId: string;
  packName: string;
  label: string;
  status: string;
  command: string[];
  commandText: string;
  runTimeoutMs: number;
  manifestPath: string;
  capabilities: string[];
  runner: string;
  parser: string;
  argumentHint: string;
}

export type AIActionKind = 'command' | 'tool' | 'file' | 'report' | 'blackboard' | 'agent' | 'harness' | 'orchestrator';

export interface AIAction {
  id: string;
  kind: AIActionKind;
  label: string;
  status: string;
  description?: string;
  packId?: string;
  packName?: string;
  command?: string[];
  commandText?: string;
  runTimeoutMs?: number;
  manifestPath?: string;
  capabilities?: string[];
  runner?: string;
  parser?: string;
  argumentHint?: string;
}

export interface AIActionRunRequest {
  actionId: string;
  input?: {
    title?: string;
    goal?: string;
    kind?: string;
    command?: string;
    expectedResult?: string;
    priority?: string;
    source?: string;
    filePath?: string;
    blackboardItemId?: string;
    agentTaskId?: string;
    harnessCaseId?: string;
  };
  arguments?: string;
}

export interface BlackboardItem {
  id?: string;
  kind?: string;
  title?: string;
  content?: string;
  status?: string;
  priority?: string;
  source?: string;
  filePath?: string;
  executionId?: string;
  graphNodeId?: string;
  replayEventId?: string;
  reportId?: string;
  createdAt?: number;
  updatedAt?: number;
  [key: string]: unknown;
}

export interface AgentTask {
  id?: string;
  title?: string;
  goal?: string;
  role?: string;
  agentId?: string;
  status?: string;
  priority?: string;
  source?: string;
  createdAt?: number;
  updatedAt?: number;
  blackboardItemId?: string;
  filePath?: string;
  executionId?: string;
  replayEventId?: string;
  reportFindingId?: string;
  [key: string]: unknown;
}

export interface HarnessCase {
  id?: string;
  title?: string;
  goal?: string;
  kind?: string;
  command?: string;
  expectedResult?: string;
  promptId?: string;
  agentRole?: string;
  source?: string;
  filePath?: string;
  agentTaskId?: string;
  blackboardItemId?: string;
  createdAt?: number;
  updatedAt?: number;
  [key: string]: unknown;
}

export interface OrchestratorRun {
  id?: string;
  title?: string;
  goal?: string;
  kind?: string;
  command?: string;
  expectedResult?: string;
  priority?: string;
  source?: string;
  filePath?: string;
  status?: string;
  blackboardItemId?: string;
  agentTaskId?: string;
  harnessCaseId?: string;
  executionId?: string;
  createdAt?: number;
  updatedAt?: number;
  [key: string]: unknown;
}

export interface AIActionResult {
  kind: string;
  operation?: string;
  summary?: string;
  orchestratorRun?: OrchestratorRun;
  blackboardItem?: BlackboardItem;
  agentTask?: AgentTask;
  harnessCase?: HarnessCase;
  execution?: ExecutionStep;
}

export interface AIActionRunResponse {
  action: AIAction;
  execution?: ExecutionStep;
  result: AIActionResult;
}

export interface GraphNode {
  id: string;
  label: string;
  subtitle: string;
  kind: 'target' | 'surface' | 'finding' | 'identity' | 'action' | 'artifact';
  severity?: Severity;
  sourceKind?: string;
  createdAt?: number;
  path?: string;
  runId?: string;
  executionId?: string;
  replayEventId?: string;
  data?: Record<string, unknown>;
  x: number;
  y: number;
}

export interface GraphEdge {
  id: string;
  from: string;
  to: string;
  kind: string;
}

export interface GraphExportResult {
  filePath: string;
  nodeCount: number;
  edgeCount: number;
  modifiedAt: number;
}

export interface ExecutionStep {
  id: string;
  label: string;
  command: string;
  status: 'queued' | 'running' | 'ready' | 'complete' | 'failed' | 'stopped';
  retryOf?: string;
  startedAt?: number;
  completedAt?: number;
  exitCode?: number;
  logPath?: string;
  artifactDir?: string;
}

export interface ExecutionLog {
  id: string;
  path: string;
  content: string;
}

export interface ExecutionArtifact {
  path: string;
  filePath: string;
  name: string;
  size: number;
  modifiedAt: number;
  contentPreview?: string;
}

export interface IDEEvent {
  type: string;
  payload?: unknown;
  time?: number;
}

export interface DesktopStatus {
  status: 'offline' | 'starting' | 'ready' | 'crashed' | string;
  apiBaseURL: string;
  workspace: string;
  running: boolean;
  lastError?: string;
  recentWorkspaces?: string[];
  windowState?: {
    x?: number;
    y?: number;
    width?: number;
    height?: number;
    maximized?: boolean;
  };
}

export interface ReplayEvent {
  id: string;
  runId?: string;
  type?: string;
  lane?: string;
  createdAt?: number;
  time: string;
  event: string;
  detail: string;
  nodeId?: string;
  executionId?: string;
  filePath?: string;
}

export interface ReportFinding {
  id: string;
  title: string;
  severity: Severity;
  endpoint: string;
  path?: string;
  source?: string;
  nodeId?: string;
  filePath?: string;
  runId?: string;
  executionId?: string;
  replayEventId?: string;
}

export interface ReportDocument {
  id: string;
  title: string;
  status: string;
  path: string;
}

export interface ReportExportResult {
  filePath: string;
  findingCount: number;
  reportCount: number;
  modifiedAt: number;
}

export interface StatusSummary {
  workspace: string;
  workflow: string;
  provider: string;
  toolchain: string;
  backend: string;
  latency: string;
  api: 'connected' | 'offline' | 'degraded' | 'bootstrap-error';
}

export interface IDEData {
  workspace: WorkspaceSettings;
  mission: MissionState;
  provider: ProviderSettings;
  toolchainPacks: ToolchainPackSettings[];
  toolActions: ToolAction[];
  workspaceItems: WorkspaceItem[];
  graphNodes: GraphNode[];
  graphEdges: GraphEdge[];
  executionSteps: ExecutionStep[];
  replayEvents: ReplayEvent[];
  reportFindings: ReportFinding[];
  reports: ReportDocument[];
  status: StatusSummary;
  loadedAt: string;
}

export interface ChatMessage {
  id: string;
  role: 'user' | 'assistant';
  content: string;
  timestamp: number;
  refs?: string[];
}

export interface ChatResponse {
  messages: ChatMessage[];
  workspace: string;
  summary: string;
}

export interface ChatPostResponse {
  message: ChatMessage;
  context?: string[];
}
