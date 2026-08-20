import type {
  AgentTask,
  ChatMessage,
  ChatPostResponse,
  ChatResponse,
  AIAction,
  AIActionResult,
  AIActionRunRequest,
  AIActionRunResponse,
  BlackboardItem,
  DesktopStatus,
  ExecutionArtifact,
  ExecutionLog,
  ExecutionStep,
  GraphEdge,
  GraphExportResult,
  GraphNode,
  HarnessCase,
  HarnessRun,
  IDEData,
  IDEEvent,
  MissionState,
  OrchestratorRun,
  PromptRecord,
  ProviderSettings,
  ReportFinding,
  ReportDocument,
  ReportExportResult,
  ReplayEvent,
  Severity,
  StatusSummary,
  ToolAction,
  ToolchainPackSettings,
  WorkspaceItem,
  WorkspaceFileContent,
  WorkspaceSearchResult,
  WorkspaceSettings,
} from './types';
import { zhCN } from './zh-cn';

const DEFAULT_API_BASE_URL = 'http://127.0.0.1:8080';

declare global {
  interface Window {
    ouroboros?: {
      getAPIBaseURL?: () => Promise<string>;
      backendStatus?: () => Promise<unknown>;
      restartBackend?: () => Promise<unknown>;
      selectWorkspace?: () => Promise<unknown>;
      openWorkspace?: (path: string) => Promise<unknown>;
      createWorkspace?: () => Promise<unknown>;
      recentWorkspaces?: () => Promise<unknown>;
      openRecentWorkspace?: (path: string) => Promise<unknown>;
      windowState?: () => Promise<unknown>;
      minimizeWindow?: () => Promise<unknown>;
      toggleMaximizeWindow?: () => Promise<unknown>;
      closeWindow?: () => Promise<unknown>;
      onMenuAction?: (callback: (action: unknown) => void) => () => void;
      onBackendStatus?: (callback: (status: unknown) => void) => () => void;
    };
  }
}

type ApiStatus = StatusSummary['api'];

interface EvidenceGraphResponse {
  nodes?: Array<{
    id?: string;
    type?: string;
    label?: string;
    sourceKind?: string;
    createdAt?: number;
    path?: string;
    runId?: string;
    executionId?: string;
    replayEventId?: string;
    data?: Record<string, unknown>;
  }>;
  edges?: Array<{
    id?: string;
    from?: string;
    to?: string;
    type?: string;
  }>;
}

interface GraphExportResponse {
  filePath?: string;
  nodeCount?: number;
  edgeCount?: number;
  modifiedAt?: number;
}

interface ReportExportResponse {
  filePath?: string;
  findingCount?: number;
  reportCount?: number;
  modifiedAt?: number;
}

interface ToolchainHealthResponse {
  status?: string;
  packs?: Record<string, { status: string; toolsReady: number }>;
}

interface GUIBootstrapResponse {
  workspace?: WorkspaceResponse;
  mission?: { name?: string; goal?: string; command?: string; path?: string; status?: string };
  workflow?: { status?: string; nextAction?: string };
  provider?: ProviderResponse;
  toolchain?: { status?: string; toolsReady?: number; packs?: ToolchainPackResponse[] };
  toolActions?: ToolActionResponse[];
  graph?: EvidenceGraphResponse;
  replayEvents?: Array<{ id?: string; runId?: string; type?: string; lane?: string; createdAt?: number; detail?: string; nodeId?: string; executionId?: string; filePath?: string }>;
  executions?: ExecutionResponse[];
  reports?: Array<{ id?: string; title?: string; status?: string; path?: string }>;
  reportFindings?: Array<{ id?: string; title?: string; severity?: string; endpoint?: string; path?: string; source?: string; nodeId?: string; filePath?: string; runId?: string; executionId?: string; replayEventId?: string }>;
  files?: Array<{ id?: string; name?: string; path?: string; kind?: string; size?: number; modifiedAt?: number }>;
  backend?: { status?: string };
}

interface WorkspaceResponse {
  name?: string;
  path?: string;
  status?: string;
  configPath?: string;
  description?: string;
  operator?: string;
  tags?: string[];
}

interface ProviderResponse {
  name?: string;
  type?: string;
  model?: string;
  baseUrl?: string;
  apiKeyEnv?: string;
  enabled?: boolean;
  timeoutMs?: number;
  contextWindowTokens?: number;
  capabilities?: string[];
  routingStrategy?: string;
  status?: string;
}

interface ProviderConfigResponse {
  path?: string;
  routingStrategy?: string;
  providers?: ProviderResponse[];
  active?: ProviderResponse;
}

interface ToolchainPackResponse {
  id?: string;
  name?: string;
  version?: string;
  status?: string;
  toolsReady?: number;
  manifestPath?: string;
  capabilities?: string[];
  runner?: string;
  runCommand?: string[];
  runTimeoutMs?: number;
  parser?: string;
  healthcheck?: string;
  healthCommand?: string[];
}

interface ToolActionResponse {
  id?: string;
  packId?: string;
  packName?: string;
  label?: string;
  status?: string;
  command?: string[];
  commandText?: string;
  runTimeoutMs?: number;
  manifestPath?: string;
  capabilities?: string[];
  runner?: string;
  parser?: string;
  argumentHint?: string;
}

interface ExecutionResponse {
  id?: string;
  label?: string;
  retryOf?: string;
  command?: string;
  status?: string;
  startedAt?: number;
  completedAt?: number;
  exitCode?: number;
  logPath?: string;
  artifactDir?: string;
}

interface DesktopWorkspaceResult {
  workspace?: string;
  recentWorkspaces?: string[];
  backend?: {
    status?: string;
    apiBaseURL?: string;
    workspace?: string;
    running?: boolean;
    lastError?: string;
  };
}

interface MissionResponse {
  name?: string;
  goal?: string;
  command?: string;
  path?: string;
  status?: string;
}

interface MissionStartResponse {
  mission?: MissionResponse;
  execution?: ExecutionResponse;
}

interface ToolActionRunResponse {
  action?: ToolActionResponse;
  execution?: ExecutionResponse;
}

interface AIActionResponse {
  id?: string;
  kind?: string;
  label?: string;
  status?: string;
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

interface AIActionsResponsePayload {
  actions?: AIActionResponse[];
}

interface BlackboardItemResponse extends Record<string, unknown> {
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
}

interface AgentTaskResponse extends Record<string, unknown> {
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
}

interface HarnessCaseResponse extends Record<string, unknown> {
  id?: string;
  title?: string;
  goal?: string;
  kind?: string;
  command?: string;
  expectedResult?: string;
  promptId?: string;
  promptName?: string;
  inputText?: string;
  agentRole?: string;
  source?: string;
  filePath?: string;
  agentTaskId?: string;
  blackboardItemId?: string;
  createdAt?: number;
  updatedAt?: number;
}

interface PromptRecordResponse extends Record<string, unknown> {
  id?: string;
  name?: string;
  content?: string;
  createdAt?: number;
}

interface HarnessRunResponse extends Record<string, unknown> {
  id?: string;
  promptName?: string;
  input?: string;
  output?: string;
  model?: string;
  createdAt?: number;
}

interface AIBlackboardListResponse {
  items?: BlackboardItemResponse[];
}

interface AIAgentsListResponse {
  items?: AgentTaskResponse[];
}

interface AIPromptsListResponse {
  items?: PromptRecordResponse[];
}

interface AIHarnessCasesListResponse {
  items?: HarnessCaseResponse[];
}

interface AIHarnessResultsListResponse {
  items?: HarnessRunResponse[];
}

interface AISwarmRunsListResponse {
  items?: SwarmWorkflowRunResponse[];
}

interface OrchestratorRunResponse extends Record<string, unknown> {
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
}

interface AIActionResultResponse {
  kind?: string;
  operation?: string;
  summary?: string;
  fileDiff?: string;
  orchestratorRun?: OrchestratorRunResponse;
  blackboardItem?: BlackboardItemResponse;
  agentTask?: AgentTaskResponse;
  harnessCase?: HarnessCaseResponse;
  prompt?: PromptRecordResponse;
  harnessRun?: HarnessRunResponse;
  swarmRun?: SwarmWorkflowRunResponse;
  execution?: ExecutionResponse;
}

interface AIActionRunResponsePayload {
  action?: AIActionResponse;
  execution?: ExecutionResponse;
  result?: AIActionResultResponse;
}

interface ExecutionArtifactResponse {
  path?: string;
  filePath?: string;
  name?: string;
  size?: number;
  modifiedAt?: number;
  contentPreview?: string;
}

export async function loadIDEData(): Promise<IDEData> {
  const startedAt = performance.now();
  const baseURL = await apiBaseURL();
  const latency = () => `${Math.round(performance.now() - startedAt)}ms`;

  if (!baseURL) {
    return mapBootstrapUnavailableData('offline', latency());
  }

  try {
    const bootstrap = await requestJSON<GUIBootstrapResponse>(baseURL, '/api/gui/bootstrap');
    return mapBootstrapData(bootstrap, latency());
  } catch {
    return mapBootstrapUnavailableData('bootstrap-error', latency());
  }

}

export async function selectDesktopWorkspace(): Promise<DesktopWorkspaceResult | null> {
  if (!window.ouroboros?.selectWorkspace) {
    return null;
  }
  return window.ouroboros.selectWorkspace() as Promise<DesktopWorkspaceResult>;
}

export async function createDesktopWorkspace(): Promise<DesktopWorkspaceResult | null> {
  if (!window.ouroboros?.createWorkspace) {
    return null;
  }
  return window.ouroboros.createWorkspace() as Promise<DesktopWorkspaceResult>;
}

export async function openRecentDesktopWorkspace(path: string): Promise<DesktopWorkspaceResult | null> {
  if (!window.ouroboros?.openRecentWorkspace) {
    return null;
  }
  return window.ouroboros.openRecentWorkspace(path) as Promise<DesktopWorkspaceResult>;
}

export async function loadDesktopStatus(): Promise<DesktopStatus> {
  if (!window.ouroboros?.backendStatus) {
    return emptyDesktopStatus();
  }
  try {
    const [backend, recentWorkspaces, windowState] = await Promise.all([
      window.ouroboros.backendStatus(),
      window.ouroboros?.recentWorkspaces?.() ?? Promise.resolve([]),
      window.ouroboros?.windowState?.() ?? Promise.resolve({}),
    ]);
    return mapDesktopStatus(backend, recentWorkspaces, windowState);
  } catch (error) {
    return mapDesktopStatus({ status: 'crashed', lastError: errorMessage(error) });
  }
}

export async function restartDesktopBackend(): Promise<DesktopStatus> {
  if (!window.ouroboros?.restartBackend) {
    return loadDesktopStatus();
  }
  try {
    const backend = await window.ouroboros.restartBackend();
    const recentWorkspaces = await (window.ouroboros?.recentWorkspaces?.() ?? Promise.resolve([]));
    const windowState = await (window.ouroboros?.windowState?.() ?? Promise.resolve({}));
    return mapDesktopStatus(backend, recentWorkspaces, windowState);
  } catch (error) {
    return mapDesktopStatus({ status: 'crashed', lastError: errorMessage(error) });
  }
}

export async function minimizeDesktopWindow(): Promise<void> {
  await window.ouroboros?.minimizeWindow?.();
}

export async function toggleMaximizeDesktopWindow(): Promise<void> {
  await window.ouroboros?.toggleMaximizeWindow?.();
}

export async function closeDesktopWindow(): Promise<void> {
  await window.ouroboros?.closeWindow?.();
}

export function onDesktopMenuAction(callback: (action: unknown) => void): () => void {
  return window.ouroboros?.onMenuAction?.(callback) ?? (() => undefined);
}

export function onDesktopBackendStatus(callback: (status: DesktopStatus) => void): () => void {
  return window.ouroboros?.onBackendStatus?.(status => callback(mapDesktopStatus(status))) ?? (() => undefined);
}

export async function connectIDEEventStream(onEvent: (event: IDEEvent) => void): Promise<() => void> {
  const baseURL = await apiBaseURL();
  if (!baseURL || typeof WebSocket === 'undefined') {
    return () => undefined;
  }
  const url = new URL('/api/events/stream', baseURL);
  url.protocol = url.protocol === 'https:' ? 'wss:' : 'ws:';
  const socket = new WebSocket(url.toString());
  socket.addEventListener('message', event => {
    try {
      onEvent(JSON.parse(event.data) as IDEEvent);
    } catch {
      onEvent({ type: 'stream:invalid-message' });
    }
  });
  socket.addEventListener('error', () => onEvent({ type: 'stream:error' }));
  socket.addEventListener('close', () => onEvent({ type: 'stream:closed' }));
  return () => {
    socket.close();
  };
}

export async function loadWorkspaceFile(path: string): Promise<WorkspaceFileContent> {
  const baseURL = await apiBaseURL();
  if (!baseURL) {
    throw new Error(zhCN.shell.statusOffline);
  }
  return requestJSON<WorkspaceFileContent>(baseURL, `/api/workspace/file?path=${encodeURIComponent(path)}`);
}

export async function saveWorkspaceFile(path: string, content: string): Promise<WorkspaceFileContent> {
  const baseURL = await apiBaseURL();
  if (!baseURL) {
    throw new Error(zhCN.shell.statusOffline);
  }
  const response = await fetch(`${baseURL}/api/workspace/file?path=${encodeURIComponent(path)}`, {
    method: 'PUT',
    headers: { Accept: 'application/json', 'Content-Type': 'application/json' },
    body: JSON.stringify({ content }),
  });
  if (!response.ok) {
    throw new Error(`${path}: ${response.status}`);
  }
  return response.json() as Promise<WorkspaceFileContent>;
}

export async function searchWorkspace(query: string): Promise<WorkspaceSearchResult[]> {
  const baseURL = await apiBaseURL();
  if (!baseURL || query.trim() === '') {
    return [];
  }
  const response = await requestJSON<{ results?: WorkspaceSearchResult[] }>(baseURL, `/api/workspace/search?q=${encodeURIComponent(query)}`);
  return response.results ?? [];
}

export async function exportEvidenceGraph(): Promise<GraphExportResult> {
  const baseURL = await apiBaseURL();
  if (!baseURL) {
    throw new Error(zhCN.shell.statusOffline);
  }
  const response = await fetch(`${baseURL}/api/evidence/graph/export`, {
    method: 'POST',
    headers: { Accept: 'application/json' },
  });
  if (!response.ok) {
    throw new Error(`${zhCN.graph.exportFailed}: ${response.status}`);
  }
  const payload = await response.json() as GraphExportResponse;
  return mapGraphExportResponse(payload);
}

export async function exportReport(): Promise<ReportExportResult> {
  const baseURL = await apiBaseURL();
  if (!baseURL) {
    throw new Error(zhCN.shell.statusOffline);
  }
  const response = await fetch(`${baseURL}/api/reports/export`, {
    method: 'POST',
    headers: { Accept: 'application/json' },
  });
  if (!response.ok) {
    throw new Error(`${zhCN.report.exportFailed}: ${response.status}`);
  }
  const payload = await response.json() as ReportExportResponse;
  return mapReportExportResponse(payload);
}

function mapGraphExportResponse(payload: GraphExportResponse): GraphExportResult {
  if (!payload.filePath || !isFiniteNumber(payload.nodeCount) || !isFiniteNumber(payload.edgeCount) || !isFiniteNumber(payload.modifiedAt)) {
    throw new Error(zhCN.graph.exportFailed);
  }
  return {
    filePath: payload.filePath,
    nodeCount: payload.nodeCount,
    edgeCount: payload.edgeCount,
    modifiedAt: payload.modifiedAt,
  };
}

function mapReportExportResponse(payload: ReportExportResponse): ReportExportResult {
  if (!payload.filePath || !isFiniteNumber(payload.findingCount) || !isFiniteNumber(payload.reportCount) || !isFiniteNumber(payload.modifiedAt)) {
    throw new Error(zhCN.report.exportFailed);
  }
  return {
    filePath: payload.filePath,
    findingCount: payload.findingCount,
    reportCount: payload.reportCount,
    modifiedAt: payload.modifiedAt,
  };
}

export async function saveMission(mission: Pick<MissionState, 'name' | 'goal' | 'command'>): Promise<MissionState> {
  const baseURL = await apiBaseURL();
  if (!baseURL) {
    throw new Error(zhCN.shell.statusOffline);
  }
  const response = await fetch(`${baseURL}/api/mission`, {
    method: 'PUT',
    headers: { Accept: 'application/json', 'Content-Type': 'application/json' },
    body: JSON.stringify(mission),
  });
  if (!response.ok) {
    throw new Error(`${zhCN.missionFlow.saveFailed}: ${response.status}`);
  }
  return mapRequiredMissionResponse(await response.json() as MissionResponse, zhCN.missionFlow.saveFailed);
}

export async function startMission(mission: Pick<MissionState, 'name' | 'goal' | 'command'>): Promise<{ mission: MissionState; execution: ExecutionStep }> {
  const baseURL = await apiBaseURL();
  if (!baseURL) {
    throw new Error(zhCN.shell.statusOffline);
  }
  const response = await fetch(`${baseURL}/api/mission/start`, {
    method: 'POST',
    headers: { Accept: 'application/json', 'Content-Type': 'application/json' },
    body: JSON.stringify(mission),
  });
  if (!response.ok) {
    throw new Error(`${zhCN.missionFlow.startFailed}: ${response.status}`);
  }
  const payload = await response.json() as MissionStartResponse;
  return {
    mission: mapRequiredMissionResponse(payload.mission, zhCN.missionFlow.startFailed),
    execution: mapRequiredExecutionResponse(payload.execution, zhCN.missionFlow.startFailed),
  };
}

export async function saveProviderConfig(provider: ProviderSettings): Promise<ProviderSettings> {
  const baseURL = await apiBaseURL();
  if (!baseURL) {
    throw new Error(zhCN.shell.statusOffline);
  }
  const response = await fetch(`${baseURL}/api/provider`, {
    method: 'PUT',
    headers: { Accept: 'application/json', 'Content-Type': 'application/json' },
    body: JSON.stringify({
      name: provider.name,
      type: provider.type,
      model: provider.model,
      baseUrl: provider.baseUrl,
      apiKeyEnv: provider.apiKeyEnv,
      enabled: provider.enabled,
      timeoutMs: provider.timeoutMs,
      contextWindowTokens: provider.contextWindowTokens,
      capabilities: provider.capabilities,
      routingStrategy: provider.routingStrategy,
    }),
  });
  if (!response.ok) {
    throw new Error(`${zhCN.settings.actions.saveProviderFailed}: ${response.status}`);
  }
  const payload = await response.json() as ProviderConfigResponse;
  return mapRequiredProviderConfigResponse(payload, zhCN.settings.actions.saveProviderFailed);
}

export async function saveToolchainPack(pack: ToolchainPackSettings): Promise<ToolchainPackSettings> {
  const baseURL = await apiBaseURL();
  if (!baseURL) {
    throw new Error(zhCN.shell.statusOffline);
  }
  const response = await fetch(`${baseURL}/api/toolchain/packs/${encodeURIComponent(pack.id)}`, {
    method: 'PUT',
    headers: { Accept: 'application/json', 'Content-Type': 'application/json' },
    body: JSON.stringify({
      name: pack.name,
      version: pack.version,
      capabilities: pack.capabilities,
      runner: pack.runner,
      runCommand: pack.runCommand,
      runTimeoutMs: pack.runTimeoutMs,
      parser: pack.parser,
      healthcheck: pack.healthcheck,
      healthCommand: pack.healthCommand,
    }),
  });
  if (!response.ok) {
    throw new Error(`${zhCN.settings.actions.saveToolchainFailed}: ${response.status}`);
  }
  return mapToolchainPack(await response.json() as ToolchainPackResponse);
}

export async function runToolchainAction(actionID: string, argumentsText: string): Promise<{ action: ToolAction; execution: ExecutionStep }> {
  const baseURL = await apiBaseURL();
  if (!baseURL) {
    throw new Error(zhCN.shell.statusOffline);
  }
  const response = await fetch(`${baseURL}/api/toolchain/actions/${encodeURIComponent(actionID)}/run`, {
    method: 'POST',
    headers: { Accept: 'application/json', 'Content-Type': 'application/json' },
    body: JSON.stringify({ arguments: argumentsText }),
  });
  if (!response.ok) {
    throw new Error(`${zhCN.toolLaunch.failed}: ${response.status}`);
  }
  const payload = await response.json() as ToolActionRunResponse;
  return {
    action: mapRequiredToolActionResponse(payload.action, zhCN.toolLaunch.failed),
    execution: mapRequiredExecutionResponse(payload.execution, zhCN.toolLaunch.failed),
  };
}

export async function fetchAIActions(): Promise<AIAction[]> {
  const baseURL = await apiBaseURL();
  if (!baseURL) {
    throw new Error(zhCN.shell.statusOffline);
  }
  const payload = await requestJSON<AIActionsResponsePayload>(baseURL, '/api/ai/actions');
  return mapAIActionsResponse(payload);
}

export async function runAIAction(request: AIActionRunRequest): Promise<AIActionRunResponse> {
  if (request.actionId.trim() === '') {
    throw new Error(zhCN.aiActions.actionMissing);
  }
  const baseURL = await apiBaseURL();
  if (!baseURL) {
    throw new Error(zhCN.shell.statusOffline);
  }
  const response = await fetch(`${baseURL}/api/ai/actions/run`, {
    method: 'POST',
    headers: { Accept: 'application/json', 'Content-Type': 'application/json' },
    body: JSON.stringify(request),
  });
  if (!response.ok) {
    throw new Error(`${zhCN.aiActions.runFailed}: ${response.status}`);
  }
  return mapAIActionRunResponse(await response.json() as AIActionRunResponsePayload);
}

export async function fetchAIChat(): Promise<ChatResponse> {
  const baseURL = await apiBaseURL();
  if (!baseURL) {
    throw new Error(zhCN.shell.statusOffline);
  }
  return requestJSON<ChatResponse>(baseURL, '/api/ai/chat');
}

export async function fetchAIBlackboard(): Promise<BlackboardItem[]> {
  const baseURL = await apiBaseURL();
  if (!baseURL) {
    throw new Error(zhCN.shell.statusOffline);
  }
  const payload = await requestJSON<AIBlackboardListResponse>(baseURL, '/api/ai/blackboard');
  return (payload.items ?? []).map(mapAIActionBlackboardItem).filter((item): item is BlackboardItem => Boolean(item));
}

export async function fetchAIAgents(): Promise<AgentTask[]> {
  const baseURL = await apiBaseURL();
  if (!baseURL) {
    throw new Error(zhCN.shell.statusOffline);
  }
  const payload = await requestJSON<AIAgentsListResponse>(baseURL, '/api/ai/agents');
  return (payload.items ?? []).map(mapAIActionAgentTask).filter((item): item is AgentTask => Boolean(item));
}

export async function fetchAIPrompts(): Promise<PromptRecord[]> {
  const baseURL = await apiBaseURL();
  if (!baseURL) {
    throw new Error(zhCN.shell.statusOffline);
  }
  const payload = await requestJSON<AIPromptsListResponse>(baseURL, '/api/ai/prompts');
  return (payload.items ?? []).map(mapAIActionPrompt).filter((item): item is PromptRecord => Boolean(item));
}

export async function fetchAIHarnessCases(): Promise<HarnessCase[]> {
  const baseURL = await apiBaseURL();
  if (!baseURL) {
    throw new Error(zhCN.shell.statusOffline);
  }
  const payload = await requestJSON<AIHarnessCasesListResponse>(baseURL, '/api/ai/harness/cases');
  return (payload.items ?? []).map(mapAIActionHarnessCase).filter((item): item is HarnessCase => Boolean(item));
}

export async function fetchAIHarnessResults(): Promise<HarnessRun[]> {
  const baseURL = await apiBaseURL();
  if (!baseURL) {
    throw new Error(zhCN.shell.statusOffline);
  }
  const payload = await requestJSON<AIHarnessResultsListResponse>(baseURL, '/api/ai/harness/results');
  return (payload.items ?? []).map(mapAIActionHarnessRun).filter((item): item is HarnessRun => Boolean(item));
}

export async function fetchAISwarmRuns(): Promise<SwarmWorkflowRun[]> {
  const baseURL = await apiBaseURL();
  if (!baseURL) {
    throw new Error(zhCN.shell.statusOffline);
  }
  const payload = await requestJSON<AISwarmRunsListResponse>(baseURL, '/api/ai/swarm/runs');
  return (payload.items ?? []).map(mapAIActionSwarmRun).filter((run): run is SwarmWorkflowRun => Boolean(run));
}

export async function sendAIChatMessage(message: string, context?: { filePath?: string; fileContent?: string }): Promise<ChatPostResponse> {
  const baseURL = await apiBaseURL();
  if (!baseURL) {
    throw new Error(zhCN.shell.statusOffline);
  }
  if (message.trim() === '') {
    throw new Error(zhCN.aiChat.messageRequired);
  }
  const response = await fetch(`${baseURL}/api/ai/chat`, {
    method: 'POST',
    headers: { Accept: 'application/json', 'Content-Type': 'application/json' },
    body: JSON.stringify({ message, context }),
  });
  if (!response.ok) {
    throw new Error(`${zhCN.aiChat.sendFailed}: ${response.status}`);
  }
  return await response.json() as ChatPostResponse;
}

export async function saveWorkspaceSettings(settings: WorkspaceSettings): Promise<WorkspaceSettings> {
  const baseURL = await apiBaseURL();
  if (!baseURL) {
    throw new Error(zhCN.shell.statusOffline);
  }
  const response = await fetch(`${baseURL}/api/workspace/settings`, {
    method: 'PUT',
    headers: { Accept: 'application/json', 'Content-Type': 'application/json' },
    body: JSON.stringify({
      name: settings.name,
      description: settings.description,
      operator: settings.operator,
      tags: settings.tags,
    }),
  });
  if (!response.ok) {
    throw new Error(`${zhCN.settings.actions.saveWorkspaceFailed}: ${response.status}`);
  }
  return mapWorkspaceSettings(await response.json() as WorkspaceResponse);
}

export async function startExecution(command: string): Promise<ExecutionStep> {
  const baseURL = await apiBaseURL();
  if (!baseURL) {
    throw new Error(zhCN.shell.statusOffline);
  }
  const response = await fetch(`${baseURL}/api/executions`, {
    method: 'POST',
    headers: { Accept: 'application/json', 'Content-Type': 'application/json' },
    body: JSON.stringify({ command }),
  });
  if (!response.ok) {
    throw new Error(`${zhCN.execution.startFailed}: ${response.status}`);
  }
  return mapRequiredExecutionResponse(await response.json() as ExecutionResponse, zhCN.execution.startFailed);
}

export async function stopExecution(id: string): Promise<ExecutionStep> {
  return postExecutionAction(id, 'stop');
}

export async function retryExecution(id: string): Promise<ExecutionStep> {
  return postExecutionAction(id, 'retry');
}

export async function loadExecutionLogs(id: string): Promise<ExecutionLog> {
  const baseURL = await apiBaseURL();
  if (!baseURL) {
    throw new Error(zhCN.shell.statusOffline);
  }
  return requestJSON<ExecutionLog>(baseURL, `/api/executions/${encodeURIComponent(id)}/logs`);
}

export async function loadExecutionArtifacts(id: string): Promise<ExecutionArtifact[]> {
  const baseURL = await apiBaseURL();
  if (!baseURL) {
    throw new Error(zhCN.shell.statusOffline);
  }
  const response = await requestJSON<{ artifacts?: ExecutionArtifactResponse[] }>(baseURL, `/api/executions/${encodeURIComponent(id)}/artifacts`);
  return (response.artifacts ?? []).flatMap(artifact => (
    isCompleteExecutionArtifactResponse(artifact) ? [mapExecutionArtifact(artifact)] : []
  ));
}

async function postExecutionAction(id: string, action: 'stop' | 'retry'): Promise<ExecutionStep> {
  const baseURL = await apiBaseURL();
  if (!baseURL) {
    throw new Error(zhCN.shell.statusOffline);
  }
  const response = await fetch(`${baseURL}/api/executions/${encodeURIComponent(id)}/${action}`, {
    method: 'POST',
    headers: { Accept: 'application/json' },
  });
  if (!response.ok) {
    throw new Error(`${id}: ${response.status}`);
  }
  return mapRequiredExecutionResponse(await response.json() as ExecutionResponse, zhCN.execution.actionFailed);
}

async function apiBaseURL(): Promise<string> {
  if (window.ouroboros?.getAPIBaseURL) {
    const desktopURL = await window.ouroboros.getAPIBaseURL();
    return desktopURL?.trim() ?? '';
  }
  return (import.meta.env.VITE_OURO_API_BASE_URL as string | undefined) || DEFAULT_API_BASE_URL;
}

function emptyDesktopStatus(): DesktopStatus {
  return {
    status: 'unavailable',
    apiBaseURL: '',
    workspace: '',
    running: false,
    lastError: '',
    recentWorkspaces: [],
  };
}

function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : String(error);
}

function mapDesktopStatus(backend: unknown, recentWorkspaces: unknown = [], windowState: unknown = {}): DesktopStatus {
  const source = isRecord(backend) ? backend : {};
  const state = isRecord(windowState) && isRecord(windowState.windowState)
    ? windowState.windowState
    : isRecord(windowState)
      ? windowState
      : {};
  return {
    status: typeof source.status === 'string' ? source.status : 'offline',
    apiBaseURL: typeof source.apiBaseURL === 'string' ? source.apiBaseURL : '',
    workspace: typeof source.workspace === 'string' ? source.workspace : '',
    running: typeof source.running === 'boolean' ? source.running : false,
    lastError: typeof source.lastError === 'string' ? source.lastError : '',
    recentWorkspaces: Array.isArray(recentWorkspaces)
      ? recentWorkspaces.filter(item => typeof item === 'string') as string[]
      : [],
    windowState: {
      x: typeof state.x === 'number' ? state.x : undefined,
      y: typeof state.y === 'number' ? state.y : undefined,
      width: typeof state.width === 'number' ? state.width : undefined,
      height: typeof state.height === 'number' ? state.height : undefined,
      maximized: typeof state.maximized === 'boolean' ? state.maximized : undefined,
    },
  };
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}

function isFiniteNumber(value: unknown): value is number {
  return typeof value === 'number' && Number.isFinite(value);
}

function mapBootstrapData(response: GUIBootstrapResponse, latency: string): IDEData {
  const toolchain = mapBootstrapToolchain(response.toolchain);
  const graphNodes = mapGraphNodes(response.graph);
  const graphEdges = mapGraphEdges(response.graph);
  const replayEvents = mapBootstrapReplayEvents(response.replayEvents);
  const executionSteps = mapExecutionSteps(response.executions);
  const reportFindings = mapBootstrapReportFindings(response.reportFindings);
  const reports = mapBootstrapReports(response.reports);
  const workspaceSettings = mapWorkspaceSettings(response.workspace);
  const toolActions = mapToolActions(response.toolActions);
  const workspace = workspaceSettings.name;
  const provider = mapProviderSettings(response.provider);
  const toolchainPacks = mapToolchainPacks(response.toolchain?.packs);

  return {
    workspace: workspaceSettings,
    mission: mapMissionResponse(response.mission),
    provider,
    toolchainPacks,
    toolActions,
    workspaceItems: mapWorkspaceItems(response.files),
    graphNodes,
    graphEdges,
    executionSteps,
    replayEvents,
    reportFindings,
    reports,
    status: mapStatusBar(response, workspace, provider, toolchain, latency),
    loadedAt: new Date().toISOString(),
  };
}

function mapStatusBar(
  response: GUIBootstrapResponse,
  workspace: string,
  provider: ProviderSettings,
  toolchain: ToolchainHealthResponse,
  latency: string,
): StatusSummary {
  const backendStatus = response.backend?.status;
  const apiStatus: ApiStatus = backendStatus && backendStatus.toLowerCase() !== 'missing'
    ? 'connected'
    : 'degraded';
  return {
    workspace,
    workflow: response.workflow?.nextAction
      ? translateBackendAction(response.workflow.nextAction)
      : response.workflow?.status
        ? translateBackendStatus(response.workflow.status)
        : zhCN.backendStatus.unknown,
    provider: providerStatus(provider),
    toolchain: toolchainStatus(toolchain),
    backend: backendStatus ? translateBackendStatus(backendStatus) : zhCN.backendStatus.unknown,
    latency,
    api: apiStatus,
  };
}

function mapBootstrapToolchain(toolchain?: GUIBootstrapResponse['toolchain']): ToolchainHealthResponse {
  const packs = (toolchain?.packs ?? []).filter(isCompleteToolchainPackResponse);
  return {
    status: toolchain?.status,
    packs: Object.fromEntries(packs.map(pack => [
      pack.id as string,
      { status: pack.status as string, toolsReady: pack.toolsReady as number },
    ])),
  };
}

function mapToolchainPacks(packs?: ToolchainPackResponse[]): ToolchainPackSettings[] {
  return (packs ?? []).flatMap(pack => (
    isCompleteToolchainPackResponse(pack) ? [mapToolchainPack(pack)] : []
  ));
}

function mapToolActions(actions?: ToolActionResponse[]): ToolAction[] {
  return (actions ?? []).flatMap(action => (
    isCompleteToolActionResponse(action) ? [mapToolAction(action)] : []
  ));
}

function mapToolAction(action: ToolActionResponse): ToolAction {
  return {
    id: action.id as string,
    packId: action.packId as string,
    packName: action.packName || '',
    label: action.label as string,
    status: translateBackendStatus(action.status),
    command: action.command as string[],
    commandText: action.commandText as string,
    runTimeoutMs: action.runTimeoutMs ?? 0,
    manifestPath: action.manifestPath || '',
    capabilities: action.capabilities ?? [],
    runner: action.runner || '',
    parser: action.parser || '',
    argumentHint: action.argumentHint || '',
  };
}

function mapRequiredToolActionResponse(action: ToolActionResponse | undefined, context: string): ToolAction {
  if (!action || !isCompleteToolActionResponse(action)) {
    throw new Error(`${context}: ${zhCN.toolLaunch.failed}`);
  }
  return mapToolAction(action);
}

function mapAIActionsResponse(payload: AIActionsResponsePayload): AIAction[] {
  if (!Array.isArray(payload.actions)) {
    throw new Error(zhCN.aiActions.actionsMissing);
  }
  return payload.actions.map(action => mapRequiredAIActionResponse(action, zhCN.aiActions.actionsMissing));
}

function mapAIActionRunResponse(payload: AIActionRunResponsePayload): AIActionRunResponse {
  return {
    action: mapRequiredAIActionResponse(payload.action, zhCN.aiActions.actionMissing),
    execution: payload.execution?.id ? mapRequiredExecutionResponse(payload.execution, zhCN.aiActions.executionMissing) : undefined,
    result: mapAIActionResultResponse(payload.result),
  };
}

function mapAIActionResultResponse(result: AIActionResultResponse | undefined): AIActionResult & { orchestratorRun?: OrchestratorRun } {
  if (!result?.kind) {
    throw new Error(zhCN.aiActions.resultMissing);
  }
  return {
    kind: normalizeAIActionResultKind(result.kind),
    operation: result.operation,
    summary: result.summary,
    fileDiff: result.fileDiff,
    orchestratorRun: mapAIActionOrchestratorRun(result.orchestratorRun),
    blackboardItem: mapAIActionBlackboardItem(result.blackboardItem),
    agentTask: mapAIActionAgentTask(result.agentTask),
    harnessCase: mapAIActionHarnessCase(result.harnessCase),
    prompt: mapAIActionPrompt(result.prompt),
    harnessRun: mapAIActionHarnessRun(result.harnessRun),
    swarmRun: mapAIActionSwarmRun(result.swarmRun),
    execution: result.execution?.id ? mapRequiredExecutionResponse(result.execution, zhCN.aiActions.executionMissing) : undefined,
  };
}

function normalizeAIActionResultKind(kind: string): string {
  switch (kind) {
    case 'orchestrator_run':
      return 'orchestrator_run';
    default:
      return kind;
  }
}

function mapAIActionOrchestratorRun(run: OrchestratorRunResponse | undefined): OrchestratorRun | undefined {
  if (!run) {
    return undefined;
  }
  return { ...run };
}

function mapAIActionBlackboardItem(item: BlackboardItemResponse | undefined): BlackboardItem | undefined {
  if (!item) {
    return undefined;
  }
  return { ...item };
}

function mapAIActionAgentTask(task: AgentTaskResponse | undefined): AgentTask | undefined {
  if (!task) {
    return undefined;
  }
  return { ...task };
}

function mapAIActionHarnessCase(testCase: HarnessCaseResponse | undefined): HarnessCase | undefined {
  if (!testCase) {
    return undefined;
  }
  return { ...testCase };
}

function mapAIActionPrompt(prompt: PromptRecordResponse | undefined): PromptRecord | undefined {
  if (!prompt) {
    return undefined;
  }
  return { ...prompt };
}

function mapAIActionHarnessRun(run: HarnessRunResponse | undefined): HarnessRun | undefined {
  if (!run) {
    return undefined;
  }
  return { ...run };
}
function mapAIActionSwarmRun(run: SwarmWorkflowRunResponse | undefined): SwarmWorkflowRun | undefined {
  if (!run) {
    return undefined;
  }
  return {
    ...run,
    tasks: (run.tasks ?? []).map(mapAIActionAgentTask).filter((task): task is AgentTask => Boolean(task)),
    facts: (run.facts ?? []).map(mapAIActionBlackboardItem).filter((item): item is BlackboardItem => Boolean(item)),
    executions: (run.executions ?? []).map(mapExecutionResponse).filter((execution): execution is ExecutionStep => Boolean(execution)),
  };
}

function mapRequiredAIActionResponse(action: AIActionResponse | undefined, context: string): AIAction {
  if (!action?.id || !action.kind || !action.label || !action.status) {
    throw new Error(context);
  }
  const kind = normalizeAIActionKind(action.kind);
  if (!kind) {
    throw new Error(context);
  }
  if (kind === 'tool' && (!action.packId || !action.packName || !Array.isArray(action.command) || action.command.length === 0 || !action.commandText || !action.manifestPath)) {
    throw new Error(context);
  }
  return {
    id: action.id,
    kind,
    label: action.label,
    status: action.status,
    description: action.description,
    packId: action.packId,
    packName: action.packName,
    command: action.command,
    commandText: action.commandText,
    runTimeoutMs: action.runTimeoutMs,
    manifestPath: action.manifestPath,
    capabilities: action.capabilities ?? [],
    runner: action.runner,
    parser: action.parser,
    argumentHint: action.argumentHint,
  };
}

function normalizeAIActionKind(kind: string): AIAction['kind'] | null {
  switch (kind.toLowerCase()) {
    case 'command':
      return 'command';
    case 'tool':
    case 'toolchain':
      return 'tool';
    case 'workspace_file':
      return 'file';
    case 'report':
      return 'report';
    case 'blackboard':
      return 'blackboard';
    case 'agent':
      return 'agent';
    case 'harness':
      return 'harness';
    case 'prompt':
      return 'prompt';
    case 'orchestrator':
      return 'orchestrator';
    case 'swarm':
      return 'swarm';
    case 'failure':
      return 'failure';
    default:
      return null;
  }
}

function isCompleteToolActionResponse(action: ToolActionResponse): boolean {
  return Boolean(
    action.id &&
    action.packId &&
    action.label &&
    action.status &&
    Array.isArray(action.command) &&
    action.command.length > 0 &&
    action.commandText,
  );
}

function mapExecutionArtifact(artifact: ExecutionArtifactResponse): ExecutionArtifact {
  return {
    path: artifact.path as string,
    filePath: artifact.filePath as string,
    name: artifact.name as string,
    size: artifact.size as number,
    modifiedAt: artifact.modifiedAt as number,
    contentPreview: artifact.contentPreview,
  };
}

function isCompleteExecutionArtifactResponse(artifact: ExecutionArtifactResponse): boolean {
  return Boolean(
    artifact.path &&
    artifact.filePath &&
    artifact.name &&
    isFiniteNumber(artifact.size) &&
    isFiniteNumber(artifact.modifiedAt),
  );
}

function isCompleteToolchainPackResponse(pack: ToolchainPackResponse): boolean {
  return Boolean(
    pack.id &&
    pack.name &&
    pack.status &&
    pack.manifestPath &&
    isFiniteNumber(pack.toolsReady),
  );
}

function mapToolchainPack(pack: ToolchainPackResponse): ToolchainPackSettings {
  return {
    id: pack.id as string,
    name: pack.name as string,
    version: pack.version || '',
    status: translateBackendStatus(pack.status as string),
    toolsReady: pack.toolsReady as number,
    manifestPath: pack.manifestPath as string,
    capabilities: pack.capabilities ?? [],
    runner: pack.runner || '',
    runCommand: pack.runCommand ?? [],
    runTimeoutMs: pack.runTimeoutMs ?? 0,
    parser: pack.parser || '',
    healthcheck: pack.healthcheck || '',
    healthCommand: pack.healthCommand ?? [],
  };
}

function mapBootstrapReplayEvents(replayEvents: GUIBootstrapResponse['replayEvents']): ReplayEvent[] {
  return (replayEvents ?? []).flatMap(event => {
    if (!event.id || !event.type || !event.lane || !isFiniteNumber(event.createdAt)) {
      return [];
    }
    const eventLabel = translateReplayEventType(event.type);
    if (!eventLabel) {
      return [];
    }
    return [{
      id: event.id,
      runId: event.runId,
      type: event.type,
      lane: event.lane,
      createdAt: event.createdAt,
      time: formatTime(event.createdAt),
      event: eventLabel,
      detail: event.detail || '',
      nodeId: event.nodeId,
      executionId: event.executionId,
      filePath: event.filePath,
    }];
  });
}

function mapBootstrapReportFindings(findings: GUIBootstrapResponse['reportFindings']): ReportFinding[] {
  return (findings ?? []).flatMap(finding => {
    const id = finding.id || '';
    const title = finding.title || '';
    const severity = normalizeSeverity(finding.severity);
    if (!id || !title || !severity) {
      return [];
    }
    return [{
      id,
      title,
      severity,
      endpoint: finding.endpoint || '',
      path: finding.path,
      source: finding.source,
      nodeId: finding.nodeId,
      filePath: finding.filePath,
      runId: finding.runId,
      executionId: finding.executionId,
      replayEventId: finding.replayEventId,
    }];
  });
}

function mapBootstrapReports(reports: GUIBootstrapResponse['reports']): ReportDocument[] {
  return (reports ?? []).flatMap(report => {
    const id = report.id || '';
    const title = report.title || '';
    const status = report.status || '';
    const path = report.path || '';
    if (!id || !title || !status || !path) {
      return [];
    }
    return [{
      id,
      title,
      status: translateBackendStatus(status),
      path,
    }];
  });
}

function providerStatus(provider?: Pick<ProviderSettings, 'model' | 'status'>): string {
  if (!provider) {
    return zhCN.shell.statusOffline;
  }
  return provider.model ? `${provider.status} · ${provider.model}` : provider.status;
}

async function requestJSON<T>(baseURL: string, path: string): Promise<T> {
  const response = await fetch(`${baseURL}${path}`, {
    headers: { Accept: 'application/json' },
  });
  if (!response.ok) {
    throw new Error(`${path}: ${response.status}`);
  }
  return response.json() as Promise<T>;
}

function mapMissionResponse(mission?: MissionResponse): MissionState {
  const mapped = mapOptionalMissionResponse(mission);
  return mapped ?? emptyMission();
}

function mapRequiredMissionResponse(mission: MissionResponse | undefined, context: string): MissionState {
  const mapped = mapOptionalMissionResponse(mission);
  if (!mapped) {
    throw new Error(`${context}: ${zhCN.missionFlow.missingRecord}`);
  }
  return mapped;
}

function mapOptionalMissionResponse(mission?: MissionResponse): MissionState | null {
  if (!mission?.path || !mission.status) {
    return null;
  }
  return {
    name: mission?.name || '',
    goal: mission?.goal || '',
    command: mission?.command || '',
    path: mission.path,
    status: translateBackendStatus(mission.status),
  };
}

function emptyMission(): MissionState {
  return {
    name: '',
    goal: '',
    command: '',
    path: '',
    status: zhCN.backendStatus.unknown,
  };
}

function emptyProvider(): ProviderSettings {
  return {
    name: '',
    type: '',
    model: '',
    baseUrl: '',
    apiKeyEnv: '',
    enabled: false,
    timeoutMs: 0,
    contextWindowTokens: 0,
    capabilities: [],
    routingStrategy: '',
    status: zhCN.backendStatus.unknown,
  };
}

function mapProviderSettings(provider?: ProviderResponse | ProviderSettings): ProviderSettings {
  const fallback = emptyProvider();
  return {
    name: provider?.name ?? fallback.name,
    type: provider?.type || fallback.type,
    model: provider?.model ?? fallback.model,
    baseUrl: provider?.baseUrl ?? fallback.baseUrl,
    apiKeyEnv: provider?.apiKeyEnv ?? fallback.apiKeyEnv,
    enabled: provider?.enabled ?? fallback.enabled,
    timeoutMs: provider?.timeoutMs ?? fallback.timeoutMs,
    contextWindowTokens: provider?.contextWindowTokens ?? fallback.contextWindowTokens,
    capabilities: provider?.capabilities ?? fallback.capabilities,
    routingStrategy: provider?.routingStrategy || fallback.routingStrategy,
    status: translateBackendStatus(provider?.status),
  };
}

function mapRequiredProviderConfigResponse(payload: ProviderConfigResponse, context: string): ProviderSettings {
  const provider = [payload.active, ...(payload.providers ?? [])].find(isCompleteProviderResponse);
  if (!provider) {
    throw new Error(`${context}: ${zhCN.settings.actions.saveProviderFailed}`);
  }
  return mapProviderSettings(provider);
}

function isCompleteProviderResponse(provider: ProviderResponse | undefined): provider is ProviderResponse {
  return Boolean(
    provider?.name &&
    provider.model &&
    typeof provider.enabled === 'boolean' &&
    provider.status,
  );
}

function emptyWorkspaceSettings(): WorkspaceSettings {
  return {
    name: zhCN.explorer.rootFallback,
    path: '',
    status: zhCN.backendStatus.unknown,
    configPath: '',
    description: '',
    operator: '',
    tags: [],
  };
}

function mapWorkspaceSettings(workspace?: WorkspaceResponse): WorkspaceSettings {
  const fallback = emptyWorkspaceSettings();
  if (!workspace?.name || !workspace.path || !workspace.status) {
    return fallback;
  }
  return {
    name: workspace.name,
    path: workspace.path,
    status: translateBackendStatus(workspace.status),
    configPath: workspace?.configPath || fallback.configPath,
    description: workspace?.description || '',
    operator: workspace?.operator || '',
    tags: workspace?.tags ?? [],
  };
}

function mapBootstrapUnavailableData(apiStatus: ApiStatus, latency: string): IDEData {
  const unavailable = apiStatus === 'offline' ? zhCN.shell.statusOffline : zhCN.backendStatus.bootstrapUnavailable;
  return {
    workspace: emptyWorkspaceSettings(),
    mission: emptyMission(),
    provider: emptyProvider(),
    toolchainPacks: [],
    toolActions: [],
    workspaceItems: mapWorkspaceItems(),
    graphNodes: [],
    graphEdges: [],
    executionSteps: [],
    replayEvents: [],
    reportFindings: [],
    reports: [],
    status: {
      workspace: zhCN.explorer.rootFallback,
      workflow: unavailable,
      provider: unavailable,
      toolchain: unavailable,
      backend: unavailable,
      latency,
      api: apiStatus,
    },
    loadedAt: new Date().toISOString(),
  };
}

function mapWorkspaceItems(files?: GUIBootstrapResponse['files']): WorkspaceItem[] {
  if (files?.length) {
    return files.slice(0, 120).flatMap(file => {
      const id = file.id || '';
      const label = file.path || '';
      const meta = file.kind ? translateFileKind(file.kind) : null;
      if (!id || !label || !file.kind || !meta) {
        return [];
      }
      return [{
        id,
        label,
        meta,
        filePath: file.kind === 'directory' ? undefined : file.path,
        kind: file.kind,
      }];
    });
  }
  return [];
}

function translateFileKind(kind: string): string | null {
  switch (kind) {
    case 'directory':
      return '目录';
    case 'markdown':
      return 'Markdown';
    case 'yaml':
      return 'YAML';
    case 'json':
      return 'JSON';
    case 'text':
      return '文本';
    case 'file':
      return '文件';
    default:
      return null;
  }
}

function mapGraphNodes(response?: EvidenceGraphResponse): GraphNode[] {
  const sourceNodes = (response?.nodes ?? []).flatMap(node => {
    if (!node.id || !node.label || !node.type) {
      return [];
    }
    const type = node.type;
    const kind = normalizeKind(type);
    if (!kind) {
      return [];
    }
    return [{ node, kind }];
  });
  const layout = layoutFor(sourceNodes.length);
  return sourceNodes.map(({ node, kind }, index) => {
    const data = node.data ?? {};
    const id = node.id as string;
    return {
      id,
      label: translateEvidenceLabel(node.label as string),
      subtitle: zhCN.node.kinds[kind],
      kind,
      severity: normalizeSeverity(data.severity),
      sourceKind: node.sourceKind,
      createdAt: node.createdAt,
      path: node.path,
      runId: node.runId,
      executionId: node.executionId,
      replayEventId: node.replayEventId,
      data: data,
      x: layout[index]?.x ?? 50,
      y: layout[index]?.y ?? 50,
    };
  });
}

function mapGraphEdges(response?: EvidenceGraphResponse): GraphEdge[] {
  return (response?.edges ?? [])
    .filter(edge => edge.id && edge.from && edge.to && edge.type)
    .map(edge => ({
      id: edge.id as string,
      from: edge.from as string,
      to: edge.to as string,
      kind: edge.type as string,
    }));
}

function mapExecutionSteps(executions?: ExecutionResponse[]): ExecutionStep[] {
  return (executions ?? []).flatMap(execution => {
    const mapped = mapExecutionResponse(execution);
    return mapped ? [mapped] : [];
  });
}

function mapRequiredExecutionResponse(execution: ExecutionResponse | undefined, context: string): ExecutionStep {
  const mapped = mapExecutionResponse(execution);
  if (!mapped) {
    throw new Error(`${context}: ${zhCN.execution.missingRecord}`);
  }
  return mapped;
}

function mapExecutionResponse(execution: ExecutionResponse | undefined): ExecutionStep | null {
  if (!execution?.id || !execution.label || !execution.command || !execution.status) {
    return null;
  }
  const status = normalizeExecutionStatus(execution.status);
  if (!status) {
    return null;
  }
  return {
    id: execution.id,
    label: execution.label,
    command: execution.command,
    status,
    retryOf: execution.retryOf,
    startedAt: execution.startedAt,
    completedAt: execution.completedAt,
    exitCode: execution.exitCode,
    logPath: execution.logPath,
    artifactDir: execution.artifactDir,
  };
}

function normalizeExecutionStatus(status: string): ExecutionStep['status'] | null {
  switch (status.toLowerCase()) {
    case 'queued':
      return 'queued';
    case 'running':
      return 'running';
    case 'completed':
    case 'complete':
      return 'complete';
    case 'failed':
      return 'failed';
    case 'stopped':
      return 'stopped';
    case 'ready':
      return 'ready';
    default:
      return null;
  }
}

function toolchainStatus(response?: ToolchainHealthResponse): string {
  if (!response) {
    return zhCN.shell.statusOffline;
  }
  const ready = Object.values(response.packs ?? {}).reduce((sum, pack) => sum + pack.toolsReady, 0);
  return `${translateBackendStatus(response.status)} · ${ready} 个工具`;
}

function normalizeKind(value: string): GraphNode['kind'] | null {
  switch (value.toLowerCase()) {
    case 'target':
      return 'target';
    case 'surface':
      return 'surface';
    case 'finding':
      return 'finding';
    case 'identity':
      return 'identity';
    case 'artifact':
      return 'artifact';
    case 'action':
      return 'action';
    default:
      return null;
  }
}

function normalizeSeverity(value: unknown): Severity | undefined {
  if (typeof value !== 'string') {
    return undefined;
  }
  if (['critical', 'high', 'medium', 'low', 'info'].includes(value)) {
    return value as Severity;
  }
  return undefined;
}

function translateEvidenceLabel(label: string): string {
  const known: Record<string, string> = {
    Target: zhCN.node.kinds.target,
    Surface: zhCN.node.kinds.surface,
    Finding: zhCN.node.kinds.finding,
  };
  return known[label] ?? label;
}

function translateBackendStatus(status?: string): string {
  switch ((status ?? '').toLowerCase()) {
    case 'ready':
      return zhCN.backendStatus.ready;
    case 'healthy':
      return zhCN.backendStatus.healthy;
    case 'completed':
      return zhCN.backendStatus.completed;
    case 'empty':
      return zhCN.backendStatus.empty;
    case 'missing':
      return zhCN.backendStatus.missing;
    case 'complete':
      return zhCN.executionStatus.complete;
    case 'running':
      return zhCN.executionStatus.running;
    case 'queued':
      return zhCN.executionStatus.queued;
    case 'failed':
      return zhCN.executionStatus.failed;
    case 'stopped':
      return zhCN.executionStatus.stopped;
    default:
      return zhCN.backendStatus.unknown;
  }
}

function translateReplayEventType(type: string): string | null {
  const key = type.toLowerCase();
  return (zhCN.replay.eventTypes as Record<string, string>)[key] ?? null;
}

function translateBackendAction(action?: string): string {
  switch ((action ?? '').toLowerCase()) {
    case 'inspect_output':
      return '查看输出';
    case 'continue_execution':
      return '继续执行';
    case 'open_workspace_files':
      return '打开工作区文件';
    default:
      return zhCN.backendStatus.unknown;
  }
}

function formatTime(unixSeconds: number): string {
  return new Date(unixSeconds * 1000).toLocaleTimeString('zh-CN', {
    hour12: false,
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
  });
}

function layoutFor(count: number): Array<{ x: number; y: number }> {
  if (count <= 0) {
    return [];
  }
  const preset = [
    { x: 52, y: 16 },
    { x: 31, y: 38 },
    { x: 68, y: 38 },
    { x: 28, y: 64 },
    { x: 72, y: 64 },
    { x: 50, y: 84 },
  ];
  return Array.from({ length: count }, (_, index) => preset[index] ?? {
    x: 20 + ((index * 17) % 60),
    y: 20 + ((index * 23) % 65),
  });
}

interface SwarmWorkflowRunResponse extends Record<string, unknown> {
  id?: string;
  title?: string;
  goal?: string;
  status?: string;
  tasks?: AgentTaskResponse[];
  facts?: BlackboardItemResponse[];
  executions?: ExecutionResponse[];
  summary?: string;
  createdAt?: number;
  updatedAt?: number;
  agentOutput?: Record<string, string>;
  timeline?: Array<Record<string, unknown>>;
  inputs?: Record<string, unknown>;
}
