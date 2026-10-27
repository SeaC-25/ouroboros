import type {
  AgentCounts,
  AgentProfile,
  AgentProfileCreateRequest,
  AgentProfileUpdateRequest,
  AgentProviderBatchRun,
  AgentProviderBatchRunRequest,
  AgentProviderRun,
  AgentProviderRunRequest,
  AgentState,
  AgentTask,
  AgentTaskCreateRequest,
  AgentTaskEvent,
  AgentTaskEventCreateRequest,
  AgentTaskUpdateRequest,
  AIAction,
  AIActionRunRequest,
  AIActionRunResponse,
  AIActionTraceItem,
  AIChatRequest,
  AIChatResponse,
  AIChatStreamEvent,
  AIContextReference,
  AIConversation,
  AIConversationExportRequest,
  AIConversationExportResult,
  AIConversationRuntime,
  AIConversationSearchResponse,
  AIConversationSearchResult,
  AIConversationSessionOperationResponse,
  AIConversationSessionSummary,
  AIMessage,
  AIMessageRole,
  AIChatActionOutcome,
  AIProviderToolCall,
  AIProviderAttempt,
  AIRoutingMetadata,
  AIUsage,
  BlackboardItem,
  BlackboardBatchUpdateRequest,
  BlackboardBatchUpdateResult,
  BlackboardDependencyLinkRequest,
  BlackboardDependencyLinkResult,
  BlackboardItemCreateRequest,
  BlackboardItemUpdateRequest,
  BlackboardQuery,
  BlackboardObservation,
  BlackboardObservationQuery,
  BlackboardObservationState,
  BlackboardState,
  DesktopStatus,
  DesktopRuntimeInfo,
  AIActionFileChange,
  EvidenceItem,
  ExecutionArtifact,
  ExecutionLog,
  ExecutionStep,
  EventlogEvents,
  EventlogQuery,
  EventlogRun,
  GraphEdge,
  GraphExportResult,
  GraphNode,
  HarnessCase,
  HarnessCaseCreateRequest,
  HarnessCounts,
  HarnessEventlogReference,
  HarnessFileDiff,
  HarnessProviderSnapshot,
  HarnessProviderSummary,
  HarnessResult,
  HarnessState,
  HarnessToolchainActionSnapshot,
  HarnessToolchainDependencySnapshot,
  HarnessToolchainPackSnapshot,
  HarnessToolchainSnapshot,
  FrontierDependencyCandidate,
  FrontierDependencyCluster,
  FrontierDependencySelection,
  FrontierDependencySummary,
  FrontierMonitor,
  FrontierMonitorSummary,
  FrontierQueue,
  FrontierQueueQuery,
  FrontierPriorityItem,
  FrontierPrioritySummary,
  OrchestratorCounts,
  OrchestratorAdvance,
  OrchestratorObservation,
  OrchestratorRun,
  OrchestratorState,
  PlannerCounts,
  PlannerPlan,
  PlannerState,
  IDEData,
  IDEEvent,
  MissionState,
  PromptDiff,
  PromptHarnessBatchComparison,
  PromptHarnessBatchComparisonItem,
  PromptHarnessComparison,
  ProviderConfig,
  ProviderTestResult,
  ProviderUsageRecord,
  ProviderUsageState,
  ProviderUsageSummary,
  ProviderSettings,
  PromptDefinition,
  PromptRegistry,
  PromptVariable,
  PromptVersion,
  ReportFinding,
  ReportDocument,
  ReportFileContent,
  ReportExportResult,
  ReplayEvent,
  RuntimeWorker,
  RuntimeWorkerOrchestration,
  RuntimeWorkerState,
  Severity,
  StatusSummary,
  ToolAction,
  ToolActionRunRequest,
  ToolchainDependency,
  ToolchainPackSettings,
  WorkspaceItem,
  WorkspaceFile,
  WorkspaceFileContent,
  WorkspaceSearchResult,
  WorkspaceSettings,
} from './types';
import { zhCN } from './zh-cn';

const blackboardPath = '/api/ai/blackboard';
const agentTasksPath = '/api/agents/tasks';
const agentProfilesPath = '/api/agents/profiles';

const REQUEST_TIMEOUT_MS = 30_000;

class AgentProfileContractError extends Error {}

declare global {
  interface ImportMetaEnv {
    readonly VITE_OURO_API_BASE_URL?: string;
  }

  interface ImportMeta {
    readonly env: ImportMetaEnv;
  }

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
      showWorkspaceFile?: (path: string) => Promise<unknown>;
      windowState?: () => Promise<unknown>;
      runtimeInfo?: () => Promise<unknown>;
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

interface EventlogRunResponse {
  id?: string;
  status?: string;
  startedAt?: number;
}

interface EventlogEventResponse {
  id?: string;
  rawEventId?: number;
  runId?: string;
  type?: string;
  lane?: string;
  createdAt?: number;
  detail?: string;
  payloadJson?: string;
  nodeId?: string;
  executionId?: string;
  filePath?: string;
}

interface EventlogEventsResponse {
  runs?: EventlogRunResponse[];
  events?: EventlogEventResponse[];
  count?: number;
  runId?: string;
  type?: string;
  lane?: string;
  limit?: number;
}

interface ToolchainHealthResponse {
  status?: string;
  toolsReady?: number;
  packs?: Record<string, ToolchainPackHealthResponse>;
}

interface ToolchainPackHealthResponse {
  status: string;
  toolsReady: number;
  name?: string;
  manifestPath?: string;
  dependencies?: ToolchainDependencyResponse[];
}

interface ToolchainResponse {
  status?: string;
  toolsReady?: number;
  packs?: ToolchainPackResponse[];
}

interface ToolchainActionsResponse {
  actions?: ToolActionResponse[];
}

interface GUIBootstrapResponse {
  // Agent Orchestrator v1 API mapping: GET /api/agents feeds bootstrap agentState;
  // POST /api/agents/tasks is exposed through the agent.tasks.create AI action.
  workspace?: WorkspaceResponse;
  mission?: { name?: string; goal?: string; command?: string; path?: string; status?: string };
  workflow?: { status?: string; nextAction?: string };
  provider?: ProviderResponse;
  providerConfig?: ProviderConfigResponse;
  providerUsage?: ProviderUsageStateResponse;
  toolchain?: { status?: string; toolsReady?: number; packs?: ToolchainPackResponse[] };
  toolActions?: ToolActionResponse[];
  graph?: EvidenceGraphResponse;
  replayEvents?: Array<{ id?: string; rawEventId?: number; runId?: string; type?: string; lane?: string; createdAt?: number; detail?: string; payloadJson?: string; nodeId?: string; executionId?: string; filePath?: string }>;
  executions?: ExecutionResponse[];
  evidenceItems?: EvidenceItemResponse[];
  reports?: ReportResponse[];
  reportFindings?: ReportFindingResponse[];
  files?: Array<{ id?: string; name?: string; path?: string; kind?: string; size?: number; modifiedAt?: number }>;
  blackboard?: BlackboardResponse;
  agentState?: AgentStateResponse;
  frontierQueue?: FrontierQueueResponse;
  frontier_queue?: FrontierQueueResponse;
  conversation?: AIConversationResponse;
  // Harness Engineering v1 API mapping: GET /api/harness feeds bootstrap harness;
  // POST /api/harness/cases creates persisted cases; POST /api/harness/cases/{id}/run starts a real run.
  harness?: HarnessStateResponse;
  orchestrator?: OrchestratorStateResponse;
  planner?: PlannerStateResponse;
  backend?: { status?: string };
}

interface PlannerStateResponse {
  path?: string;
  plans?: PlannerPlanResponse[];
  counts?: Record<string, unknown>;
}

interface PlannerPlanResponse {
  id?: string;
  title?: string;
  goal?: string;
  status?: string;
  priority?: string;
  statusFilter?: string;
  status_filter?: string;
  priorityFilter?: string;
  priority_filter?: string;
  source?: string;
  filePath?: string;
  contextSummary?: string;
  nextActionTitle?: string;
  nextActionGoal?: string;
  nextActionCommand?: string;
  blackboardItemId?: string;
  agentTaskId?: string;
  orchestratorRunId?: string;
  harnessCaseId?: string;
  executionId?: string;
  executionStatus?: string;
  executedAt?: number;
  observedAt?: number;
  observationSummary?: string;
  logPath?: string;
  artifactDir?: string;
  artifactCount?: number;
  exitCode?: number;
  harnessResultId?: string;
  createdAt?: number;
  updatedAt?: number;
}

interface OrchestratorStateResponse {
  path?: string;
  observationPath?: string;
  advancePath?: string;
  runs?: OrchestratorRunResponse[];
  observations?: OrchestratorObservationResponse[];
  advances?: OrchestratorAdvanceResponse[];
  counts?: Record<string, unknown>;
}

interface OrchestratorRunResponse {
  id?: string;
  title?: string;
  goal?: string;
  strategy?: string;
  status?: string;
  command?: string;
  expectedResult?: string;
  priority?: string;
  source?: string;
  filePath?: string;
  blackboardItemId?: string;
  agentTaskId?: string;
  executionId?: string;
  harnessCaseId?: string;
  createdAt?: number;
  updatedAt?: number;
}

interface OrchestratorObservationResponse {
  id?: string;
  runId?: string;
  executionId?: string;
  status?: string;
  exitCode?: number;
  logPath?: string;
  artifactDir?: string;
  artifactCount?: number;
  logPreview?: string;
  summary?: string;
  blackboardItemId?: string;
  agentTaskId?: string;
  harnessResultId?: string;
  autoParsedEvidenceItems?: EvidenceItemResponse[];
  autoParsedReportFindings?: ReportFindingResponse[];
  autoParsedEvidenceItemIds?: string[];
  autoParsedReportFindingIds?: string[];
  autoParsedEvidenceCount?: number;
  autoParsedFindingCount?: number;
  parsedSource?: string;
  parsedBatchTitle?: string;
  createdAt?: number;
  updatedAt?: number;
}

interface OrchestratorAdvanceResponse {
  id?: string;
  runId?: string;
  observationId?: string;
  parentExecutionId?: string;
  executionId?: string;
  title?: string;
  goal?: string;
  strategy?: string;
  status?: string;
  command?: string;
  expectedResult?: string;
  priority?: string;
  source?: string;
  filePath?: string;
  blackboardItemId?: string;
  agentTaskId?: string;
  harnessCaseId?: string;
  autoParsedEvidenceItems?: EvidenceItemResponse[];
  autoParsedReportFindings?: ReportFindingResponse[];
  autoParsedEvidenceItemIds?: string[];
  autoParsedReportFindingIds?: string[];
  autoParsedEvidenceCount?: number;
  autoParsedFindingCount?: number;
  createdAt?: number;
  updatedAt?: number;
}

interface AgentStateResponse {
  path?: string;
  eventPath?: string;
  event_path?: string;
  profilePath?: string;
  profile_path?: string;
  profiles?: AgentProfileResponse[];
  tasks?: AgentTaskResponse[];
  events?: AgentTaskEventResponse[];
  counts?: Record<string, unknown>;
}

interface AgentProfileResponse {
  id?: string;
  name?: string;
  role?: string;
  description?: string;
  instructions?: string;
  capabilities?: string[];
  providerName?: string;
  provider_name?: string;
  model?: string;
  promptId?: string;
  prompt_id?: string;
  promptVersionId?: string;
  prompt_version_id?: string;
  createdAt?: number;
  created_at?: number;
  updatedAt?: number;
  updated_at?: number;
}

interface AgentProfilesResponse {
  profiles?: AgentProfileResponse[];
}

interface AgentTaskResponse {
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
  resultSummary?: string;
  resultDetail?: string;
  artifactPath?: string;
  handoffSummary?: string;
  handoffTarget?: string;
  latestEventId?: string;
  latestResultEventId?: string;
  latestHandoffEventId?: string;
  runId?: string;
  providerName?: string;
  model?: string;
  promptId?: string;
  promptVersionId?: string;
  inputTokens?: number;
  outputTokens?: number;
  totalTokens?: number;
  durationMs?: number;
  estimatedCostUsd?: number;
  estimatedCostAvailable?: boolean;
  error?: string;
  startedAt?: string;
  completedAt?: string;
  executionId?: string;
  replayEventId?: string;
  reportFindingId?: string;
}

interface AgentTaskEventResponse {
  id?: string;
  taskId?: string;
  kind?: string;
  status?: string;
  agentId?: string;
  handoffTarget?: string;
  title?: string;
  summary?: string;
  detail?: string;
  source?: string;
  filePath?: string;
  artifactPath?: string;
  runId?: string;
  providerName?: string;
  model?: string;
  promptId?: string;
  promptVersionId?: string;
  inputTokens?: number;
  outputTokens?: number;
  totalTokens?: number;
  durationMs?: number;
  estimatedCostUsd?: number;
  estimatedCostAvailable?: boolean;
  error?: string;
  executionId?: string;
  replayEventId?: string;
  reportFindingId?: string;
  createdAt?: number;
}

interface AgentProviderRunResponse {
  runId?: string;
  run_id?: string;
  status?: string;
  agentProfileId?: string;
  agent_profile_id?: string;
  agentProfileName?: string;
  agent_profile_name?: string;
  agentProfileRole?: string;
  agent_profile_role?: string;
  agentProfileDescription?: string;
  agent_profile_description?: string;
  agentProfileInstructions?: string;
  agent_profile_instructions?: string;
  agentProfileCapabilities?: string[];
  agent_profile_capabilities?: string[];
  error?: string;
  output?: string;
  startedAt?: string;
  started_at?: string;
  completedAt?: string;
  completed_at?: string;
  durationMs?: number;
  duration_ms?: number;
  providerName?: string;
  provider_name?: string;
  model?: string;
  promptId?: string;
  prompt_id?: string;
  promptVersionId?: string;
  prompt_version_id?: string;
  inputTokens?: number;
  input_tokens?: number;
  outputTokens?: number;
  output_tokens?: number;
  totalTokens?: number;
  total_tokens?: number;
  estimatedCostUsd?: number;
  estimated_cost_usd?: number;
  estimatedCostAvailable?: boolean;
  estimated_cost_available?: boolean;
  artifactPath?: string;
  artifact_path?: string;
  eventlogRunId?: string;
  eventlog_run_id?: string;
  task?: AgentTaskResponse;
  event?: AgentTaskEventResponse;
  startedEvent?: AgentTaskEventResponse;
  started_event?: AgentTaskEventResponse;
  blackboardItem?: BlackboardItemResponse;
  blackboard_item?: BlackboardItemResponse;
  providerAttempts?: AIProviderAttemptResponse[];
  provider_attempts?: AIProviderAttemptResponse[];
}

interface AgentProviderBatchRunResponse {
  batchId?: string;
  batch_id?: string;
  status?: string;
  error?: string;
  taskIds?: string[];
  task_ids?: string[];
  requestedCount?: number;
  requested_count?: number;
  completedCount?: number;
  completed_count?: number;
  failedCount?: number;
  failed_count?: number;
  startedAt?: string;
  started_at?: string;
  completedAt?: string;
  completed_at?: string;
  durationMs?: number;
  duration_ms?: number;
  artifactPath?: string;
  artifact_path?: string;
  eventlogRunId?: string;
  eventlog_run_id?: string;
  runs?: AgentProviderRunResponse[];
}

interface HarnessStateResponse {
  path?: string;
  resultPath?: string;
  cases?: HarnessCaseResponse[];
  results?: HarnessResultResponse[];
  counts?: HarnessCountsResponse;
}

interface HarnessCaseResponse {
  id?: string;
  title?: string;
  goal?: string;
  kind?: string;
  command?: string;
  expectedResult?: string;
  promptId?: string;
  promptVersionId?: string;
  agentRole?: string;
  source?: string;
  filePath?: string;
  executionId?: string;
  replayRunId?: string;
  snapshotId?: string;
  snapshotPath?: string;
  agentTaskId?: string;
  blackboardItemId?: string;
  providerName?: string;
  model?: string;
  maxTokens?: number;
  providerSnapshot?: HarnessProviderSnapshotResponse;
  toolchainSnapshot?: HarnessToolchainSnapshotResponse;
  createdAt?: number;
  updatedAt?: number;
}

interface HarnessProviderSnapshotResponse {
  configPath?: string;
  routingStrategy?: string;
  status?: string;
  error?: string;
  active?: HarnessProviderSummaryResponse;
  providers?: HarnessProviderSummaryResponse[];
}

interface HarnessProviderSummaryResponse {
  name?: string;
  type?: string;
  model?: string;
  baseUrl?: string;
  apiKeyEnv?: string;
  enabled?: boolean;
  timeoutMs?: number;
  contextWindowTokens?: number;
  capabilities?: string[];
  status?: string;
}

interface HarnessToolchainSnapshotResponse {
  status?: string;
  error?: string;
  toolsReady?: number;
  packs?: HarnessToolchainPackSnapshotResponse[];
  actions?: HarnessToolchainActionSnapshotResponse[];
}

interface HarnessToolchainPackSnapshotResponse {
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
  dependencyCount?: number;
  dependencies?: HarnessToolchainDependencySnapshotResponse[];
}

interface HarnessToolchainDependencySnapshotResponse {
  name?: string;
  kind?: string;
  version?: string;
  source?: string;
  optional?: boolean;
  dependsOn?: string[];
  installHint?: string;
  installCommand?: string[];
}

interface HarnessToolchainActionSnapshotResponse {
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

interface HarnessResultResponse {
  id?: string;
  caseId?: string;
  caseTitle?: string;
  status?: string;
  error?: string;
  command?: string;
  expectedResult?: string;
  runId?: string;
  run_id?: string;
  executionId?: string;
  startedAt?: number;
  completedAt?: number;
  exitCode?: number;
  durationMs?: number;
  logBytes?: number;
  artifactCount?: number;
  artifactBytes?: number;
  summary?: string;
  logPath?: string;
  artifactDir?: string;
  promptId?: string;
  promptVersionId?: string;
  agentRole?: string;
  filePath?: string;
  replayRunId?: string;
  snapshotId?: string;
  snapshotPath?: string;
  agentTaskId?: string;
  blackboardItemId?: string;
  aiMessageIds?: string[];
  toolCallIds?: string[];
  fileDiffs?: HarnessFileDiffResponse[];
  eventlogReferences?: HarnessEventlogReferenceResponse[];
  references?: AIContextReferenceResponse[];
  provider?: string;
  model?: string;
  inputTokens?: number;
  input_tokens?: number;
  outputTokens?: number;
  output_tokens?: number;
  totalTokens?: number;
  total_tokens?: number;
  latencyMs?: number;
  latency_ms?: number;
  costUsd?: number;
  cost_usd?: number;
  costKnown?: boolean;
  cost_known?: boolean;
  usage?: AIUsageResponse;
  output?: string;
  outputSummary?: string;
  output_summary?: string;
  providerAttempts?: AIProviderAttemptResponse[];
  provider_attempts?: AIProviderAttemptResponse[];
}

interface HarnessFileDiffResponse {
  path?: string;
  operation?: string;
  summary?: string;
  diff?: string;
  actionId?: string;
  messageId?: string;
  toolCallId?: string;
}

interface HarnessEventlogReferenceResponse {
  id?: string;
  rawEventId?: string | number;
  replayEventId?: string;
  eventType?: string;
  lane?: string;
  summary?: string;
}

interface HarnessRunResponseResponse {
  case?: HarnessCaseResponse;
  result?: HarnessResultResponse;
  execution?: ExecutionResponse;
}

interface HarnessCountsResponse {
  totalCases?: number;
  totalResults?: number;
  running?: number;
  passed?: number;
  failed?: number;
}

interface BlackboardResponse {
  path?: string;
  items?: BlackboardItemResponse[];
  observations?: BlackboardObservationResponse[];
}

interface BlackboardObservationResponse {
  id?: string;
  kind?: string;
  target?: string;
  payload?: string;
  source?: string;
  createdAt?: number;
}

interface BlackboardObservationsResponse {
  path?: string;
  observations?: BlackboardObservationResponse[];
  count?: number;
}

interface BlackboardItemResponse {
  id?: string;
  kind?: string;
  title?: string;
  content?: string;
  status?: string;
  priority?: string;
  source?: string;
  dependencyIds?: unknown;
  dependentIds?: unknown;
  filePath?: string;
  executionId?: string;
  graphNodeId?: string;
  replayEventId?: string;
  reportId?: string;
  createdAt?: number;
  updatedAt?: number;
}

interface BlackboardBatchUpdateResponse extends BlackboardResponse {
  count?: number;
}

interface BlackboardDependencyLinkResponse {
  path?: string;
  count?: number;
  item?: BlackboardItemResponse;
  dependencies?: BlackboardItemResponse[];
}

interface ReportResponse {
  id?: string;
  title?: string;
  status?: string;
  path?: string;
  size?: number;
  modifiedAt?: number;
}

interface ReportFileResponse {
  report?: ReportResponse;
  file?: WorkspaceFileContent;
}

interface ReportFindingResponse {
  id?: string;
  title?: string;
  severity?: string;
  endpoint?: string;
  description?: string;
  recommendation?: string;
  evidence?: string;
  path?: string;
  source?: string;
  nodeId?: string;
  graphNodeId?: string;
  filePath?: string;
  runId?: string;
  executionId?: string;
  replayEventId?: string;
  agentTaskId?: string;
  agent_task_id?: string;
  blackboardItemId?: string;
  blackboard_item_id?: string;
  promptId?: string;
  prompt_id?: string;
  promptVersionId?: string;
  prompt_version_id?: string;
  aiMessageIds?: string[];
  ai_message_ids?: string[];
  toolCallIds?: string[];
  tool_call_ids?: string[];
  actionTraceIds?: string[];
  action_trace_ids?: string[];
}

interface EvidenceItemResponse {
  id?: string;
  title?: string;
  summary?: string;
  severity?: string;
  source?: string;
  path?: string;
  filePath?: string;
  artifactPath?: string;
  executionId?: string;
  graphNodeId?: string;
  replayEventId?: string;
  agentTaskId?: string;
  agent_task_id?: string;
  blackboardItemId?: string;
  blackboard_item_id?: string;
  promptId?: string;
  prompt_id?: string;
  promptVersionId?: string;
  prompt_version_id?: string;
  aiMessageIds?: string[];
  ai_message_ids?: string[];
  toolCallIds?: string[];
  tool_call_ids?: string[];
  actionTraceIds?: string[];
  action_trace_ids?: string[];
  createdAt?: number;
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

interface WorkspaceFileResponse {
  id?: string;
  path?: string;
  name?: string;
  kind?: string;
  size?: number;
  modifiedAt?: number;
}

interface ProviderResponse {
  name?: string;
  type?: string;
  model?: string;
  baseUrl?: string;
  apiKeyEnv?: string;
  apiKeyValue?: string;
  apiKeyConfigured?: boolean;
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

interface ProviderTestResultResponse {
  provider?: string;
  model?: string;
  status?: string;
  content?: string;
  latencyMs?: number;
  latency_ms?: number;
  usage?: AIUsageResponse;
  attempts?: AIProviderAttemptResponse[];
  providerAttempts?: AIProviderAttemptResponse[];
  provider_attempts?: AIProviderAttemptResponse[];
  config?: ProviderConfigResponse;
  providerConfig?: ProviderConfigResponse;
  provider_config?: ProviderConfigResponse;
}

interface ProviderUsageSummaryResponse {
  recordCount?: number;
  record_count?: number;
  providers?: string[];
  models?: string[];
  inputTokens?: number;
  input_tokens?: number;
  outputTokens?: number;
  output_tokens?: number;
  totalTokens?: number;
  total_tokens?: number;
  estimatedCostUsd?: number;
  estimated_cost_usd?: number;
  estimatedCostRecordCount?: number;
  estimated_cost_records?: number;
  budgetEnforcedCount?: number;
  budget_enforced_records?: number;
  budgetWithinLimitCount?: number;
  budget_within_limit_records?: number;
  firstCreatedAt?: string;
  first_created_at?: string;
  lastCreatedAt?: string;
  last_created_at?: string;
}

interface ProviderUsageRecordResponse {
  schemaVersion?: number;
  schema_version?: number;
  path?: string;
  id?: string;
  recordId?: string;
  record_id?: string;
  source?: string;
  createdAt?: string;
  created_at?: string;
  provider?: string;
  providerName?: string;
  provider_name?: string;
  model?: string;
  tokens?: {
    input?: number;
    output?: number;
    total?: number;
    contextWindow?: number;
    context_window?: number;
  };
  inputTokens?: number;
  input_tokens?: number;
  outputTokens?: number;
  output_tokens?: number;
  totalTokens?: number;
  total_tokens?: number;
  contextWindowTokens?: number;
  context_window_tokens?: number;
  capabilities?: string[];
  estimatedCostUsd?: number;
  estimated_cost_usd?: number;
  estimatedCostAvailable?: boolean;
  estimated_cost_available?: boolean;
  budgetEnforced?: boolean;
  budget_enforced?: boolean;
  budgetStatus?: string;
  budget_status?: string;
  budgetMaxEstimatedCostUsd?: number;
  budget_max_estimated_cost_usd?: number;
}

interface ProviderUsageStateResponse {
  path?: string;
  summary?: ProviderUsageSummaryResponse;
  recentRecords?: ProviderUsageRecordResponse[];
  recent_records?: ProviderUsageRecordResponse[];
  records?: ProviderUsageRecordResponse[];
}

interface PromptRegistryResponse {
  version?: number;
  path?: string;
  prompts?: PromptDefinitionResponse[];
}

interface PromptDefinitionResponse {
  id?: string;
  title?: string;
  kind?: string;
  activeVersion?: string;
  versions?: PromptVersionResponse[];
}

interface PromptVersionResponse {
  id?: string;
  title?: string;
  content?: string;
  providerName?: string;
  provider_name?: string;
  model?: string;
  variables?: PromptVariableResponse[];
}

interface PromptVariableResponse {
  name?: string;
  description?: string;
  required?: boolean;
  default?: string;
  source?: string;
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
  dependencies?: ToolchainDependencyResponse[];
}

interface ToolchainDependencyResponse {
  name?: string;
  kind?: string;
  required?: boolean;
  status?: string;
  ready?: boolean;
  version?: string;
  installedVersion?: string;
  latestVersion?: string;
  source?: string;
  imageDigest?: string;
  latestImageDigest?: string;
  metadataSource?: string;
  optional?: boolean;
  dependsOn?: string[];
  message?: string;
  installHint?: string;
  installCommand?: string[];
}

interface ToolActionResponse {
  id?: string;
  source?: string;
  kind?: string;
  toolId?: string;
  category?: string;
  adapter?: string;
  operation?: string;
  binary?: string;
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
  targetKind?: string;
  targetKinds?: string[];
  dependencyName?: string;
  dependencyKind?: string;
  dependencyStatus?: string;
  dependencyReady?: boolean;
  dependencyVersion?: string;
  dependencyMessage?: string;
  dependencySource?: string;
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

interface DesktopShowWorkspaceFileResult {
  shown?: boolean;
  path?: string;
  reason?: string;
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

interface ExecutionArtifactResponse {
  path?: string;
  filePath?: string;
  name?: string;
  size?: number;
  modifiedAt?: number;
  contentPreview?: string;
}

interface ExecutionLogResponse {
  id?: string;
  path?: string;
  content?: string;
}

interface AIProviderToolCallResponse {
  id?: string;
  name?: string;
  actionId?: string;
}

interface AIMessageResponse {
  id?: string;
  role?: string;
  content?: string;
  createdAt?: string | number;
  timestamp?: string | number;
  status?: string;
  tokenCount?: number;
  toolCallId?: string;
  tool_call_id?: string;
  actionId?: string;
  action_id?: string;
  toolName?: string;
  tool_name?: string;
  nativeToolCall?: boolean;
  native_tool_call?: boolean;
  nativeToolResultForwarded?: boolean;
  providerToolCalls?: AIProviderToolCallResponse[];
  contextReferences?: AIContextReferenceResponse[];
  context_references?: AIContextReferenceResponse[];
  actions?: AIChatActionOutcomeResponse[];
  usage?: AIUsageResponse;
  routing?: AIRoutingMetadataResponse;
  routingStrategy?: string;
  routing_strategy?: string;
  provider?: string;
  model?: string;
  finalProvider?: string;
  final_provider?: string;
  finalModel?: string;
  final_model?: string;
  fallbackFrom?: string;
  fallback_from?: string;
  failover?: boolean;
  providerAttempts?: AIProviderAttemptResponse[];
  provider_attempts?: AIProviderAttemptResponse[];
  attempts?: AIProviderAttemptResponse[];
}

interface AIChatActionOutcomeResponse {
  kind?: string;
  summary?: string;
  path?: string;
  output?: string;
  diff?: string;
  status?: string;
  exitCode?: number;
  executionId?: string;
  execution_id?: string;
}

interface AIContextReferenceResponse {
  id?: string;
  label?: string;
  kind?: string;
  title?: string;
  path?: string;
  detail?: string;
  summary?: string;
  line?: number;
  source?: string;
}

interface AIUsageResponse {
  inputTokens?: number;
  input_tokens?: number;
  outputTokens?: number;
  output_tokens?: number;
  promptTokens?: number;
  prompt_tokens?: number;
  completionTokens?: number;
  completion_tokens?: number;
  totalTokens?: number;
  total_tokens?: number;
  latencyMs?: number;
  latency_ms?: number;
  costUsd?: number;
  cost_usd?: number;
  costKnown?: boolean;
  cost_known?: boolean;
}

interface AIProviderAttemptResponse {
  provider?: string;
  model?: string;
  type?: string;
  status?: string;
  error?: string;
  latencyMs?: number;
  latency_ms?: number;
  usage?: AIUsageResponse;
  costUsd?: number;
  cost_usd?: number;
  costKnown?: boolean;
  cost_known?: boolean;
}

interface AIRoutingMetadataResponse {
  routing?: AIRoutingMetadataResponse;
  routingStrategy?: string;
  routing_strategy?: string;
  provider?: string;
  model?: string;
  finalProvider?: string;
  final_provider?: string;
  finalModel?: string;
  final_model?: string;
  fallbackFrom?: string;
  fallback_from?: string;
  failover?: boolean;
  providerAttempts?: AIProviderAttemptResponse[];
  provider_attempts?: AIProviderAttemptResponse[];
  attempts?: AIProviderAttemptResponse[];
  usage?: AIUsageResponse;
}

interface AIConversationRuntimeResponse {
  id?: string;
  status?: string;
  phase?: string;
  startedAt?: string | number;
  started_at?: string | number;
  updatedAt?: string | number;
  updated_at?: string | number;
  durationSeconds?: number;
  duration_seconds?: number;
  messageCount?: number;
  message_count?: number;
  actionCount?: number;
  action_count?: number;
  toolResultCount?: number;
  tool_result_count?: number;
  modelReplyCount?: number;
  model_reply_count?: number;
  finalReplyCount?: number;
  final_reply_count?: number;
  providerCallCount?: number;
  provider_call_count?: number;
  provider?: string;
  model?: string;
  routingStrategy?: string;
  routing_strategy?: string;
  lastMessageId?: string;
  last_message_id?: string;
  lastMessageRole?: string;
  last_message_role?: string;
  lastActionId?: string;
  last_action_id?: string;
  lastActionKind?: string;
  last_action_kind?: string;
  lastActionStatus?: string;
  last_action_status?: string;
  lastExecutionId?: string;
  last_execution_id?: string;
  totalTokens?: number;
  total_tokens?: number;
  costKnown?: boolean;
  cost_known?: boolean;
  costUsd?: number;
  cost_usd?: number;
}

interface AIConversationResponse {
  messages?: AIMessageResponse[];
  contextReferences?: AIContextReferenceResponse[];
  context_references?: AIContextReferenceResponse[];
  actionTrace?: AIActionTraceItemResponse[];
  action_trace?: AIActionTraceItemResponse[];
  provider?: string;
  model?: string;
  updatedAt?: string | number;
  updated_at?: string | number;
  usage?: AIUsageResponse;
  routing?: AIRoutingMetadataResponse;
  routingStrategy?: string;
  routing_strategy?: string;
  finalProvider?: string;
  final_provider?: string;
  finalModel?: string;
  final_model?: string;
  fallbackFrom?: string;
  fallback_from?: string;
  failover?: boolean;
  providerAttempts?: AIProviderAttemptResponse[];
  provider_attempts?: AIProviderAttemptResponse[];
  attempts?: AIProviderAttemptResponse[];
  runtime?: AIConversationRuntimeResponse;
}

interface LegacyChatConversationResponse {
  messages?: AIMessageResponse[];
  workspace?: string;
  summary?: string;
}

interface LegacyChatPostResponse {
  message?: AIMessageResponse;
  context?: string[];
}

interface AIConversationSessionSummaryResponse {
  id?: string;
  path?: string;
  current?: boolean;
  messageCount?: number;
  message_count?: number;
  userCount?: number;
  user_count?: number;
  assistantCount?: number;
  assistant_count?: number;
  toolCount?: number;
  tool_count?: number;
  startedAt?: string | number;
  started_at?: string | number;
  updatedAt?: string | number;
  updated_at?: string | number;
  latestUserGoal?: string;
  latest_user_goal?: string;
  latestAssistantSummary?: string;
  latest_assistant_summary?: string;
  provider?: string;
  model?: string;
  totalTokens?: number;
  total_tokens?: number;
  costUsd?: number;
  cost_usd?: number;
  costKnown?: boolean;
  cost_known?: boolean;
}

interface AIConversationSessionsResponsePayload {
  sessions?: AIConversationSessionSummaryResponse[];
}

interface AIConversationSessionOperationResponsePayload {
  session?: AIConversationSessionSummaryResponse;
  sessions?: AIConversationSessionSummaryResponse[];
  conversation?: AIConversationResponse;
}

interface AIConversationSearchResultResponse {
  id?: string;
  session?: AIConversationSessionSummaryResponse | string;
  sessionId?: string;
  session_id?: string;
  sessionPath?: string;
  session_path?: string;
  sessionCurrent?: boolean;
  session_current?: boolean;
  current?: boolean;
  message?: AIMessageResponse;
  messageId?: string;
  message_id?: string;
  role?: string;
  contentSnippet?: string;
  content_snippet?: string;
  snippet?: string;
  content?: string;
  path?: string;
  messagePath?: string;
  message_path?: string;
  createdAt?: string | number;
  created_at?: string | number;
  time?: string | number;
  provider?: string;
  model?: string;
  contextReferenceCount?: number;
  context_reference_count?: number;
}

interface AIConversationSearchResponsePayload {
  query?: string;
  results?: AIConversationSearchResultResponse[];
  count?: number;
}

interface AIConversationExportRequestPayload {
  sessionId?: string;
  format?: string;
  includeContextReferences?: boolean;
}

interface AIConversationExportResponsePayload {
  sessionId?: string;
  session_id?: string;
  path?: string;
  filePath?: string;
  file_path?: string;
  format?: string;
  messageCount?: number;
  message_count?: number;
  exportedAt?: string | number;
  exported_at?: string | number;
  summary?: string;
}

const aiConversationSessionsPath = '/api/ai/conversation/sessions';
const aiConversationSessionNewPath = '/api/ai/conversation/sessions/new';
const aiConversationSessionRestorePath = '/api/ai/conversation/sessions/restore';
const aiConversationSearchPath = '/api/ai/conversation/search';
const aiConversationExportPath = '/api/ai/conversation/export';

interface AIActionTraceItemResponse {
  id?: string;
  messageId?: string;
  message_id?: string;
  index?: number;
  role?: string;
  phase?: string;
  toolCallId?: string;
  tool_call_id?: string;
  actionId?: string;
  action_id?: string;
  toolName?: string;
  tool_name?: string;
  nativeToolCall?: boolean;
  native_tool_call?: boolean;
  nativeToolResultForwarded?: boolean;
  actionKind?: string;
  action_kind?: string;
  status?: string;
  title?: string;
  summary?: string;
  createdAt?: string | number;
  created_at?: string | number;
  provider?: string;
  model?: string;
  contextReferences?: AIContextReferenceResponse[];
  context_references?: AIContextReferenceResponse[];
}

interface AIChatResponsePayload {
  conversation?: AIConversationResponse;
  message?: AIMessageResponse;
  provider?: string;
  model?: string;
  usage?: AIUsageResponse;
  routing?: AIRoutingMetadataResponse;
  routingStrategy?: string;
  routing_strategy?: string;
  finalProvider?: string;
  final_provider?: string;
  finalModel?: string;
  final_model?: string;
  fallbackFrom?: string;
  fallback_from?: string;
  failover?: boolean;
  providerAttempts?: AIProviderAttemptResponse[];
  provider_attempts?: AIProviderAttemptResponse[];
  attempts?: AIProviderAttemptResponse[];
  contextReferences?: AIContextReferenceResponse[];
  context_references?: AIContextReferenceResponse[];
  actionResults?: AIActionRunResponsePayload[];
  action_results?: AIActionRunResponsePayload[];
}

interface AIChatStreamEventPayload {
  event?: string;
  phase?: string;
  status?: string;
  message?: string;
  index?: number;
  total?: number;
  batchIndex?: number;
  batch_index?: number;
  tokenIndex?: number;
  token_index?: number;
  delta?: string;
  textDelta?: string;
  text_delta?: string;
  contentDelta?: string;
  content_delta?: string;
  toolCallId?: string;
  tool_call_id?: string;
  toolName?: string;
  tool_name?: string;
  nativeToolCall?: boolean;
  native_tool_call?: boolean;
  actionId?: string;
  action_id?: string;
  actionKind?: string;
  action_kind?: string;
  actionLabel?: string;
  action_label?: string;
  startedAt?: string;
  started_at?: string;
  finishedAt?: string;
  finished_at?: string;
  elapsedMs?: number;
  elapsed_ms?: number;
  resultKind?: string;
  result_kind?: string;
  resultSummary?: string;
  result_summary?: string;
  referenceCount?: number;
  reference_count?: number;
  contextReferences?: AIContextReferenceResponse[];
  context_references?: AIContextReferenceResponse[];
  conversation?: AIConversationResponse;
  response?: AIChatResponsePayload;
  actionResult?: AIActionRunResponsePayload;
  action_result?: AIActionRunResponsePayload;
  runtime?: AIConversationRuntimeResponse;
  provider?: string;
  model?: string;
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
  targetKinds?: string[];
  argumentHint?: string;
}

interface AIActionsResponsePayload {
  actions?: AIActionResponse[];
}

interface AIActionRunResponsePayload {
  action?: AIActionResponse;
  execution?: ExecutionResponse;
  result?: AIActionResultResponse;
  message?: AIMessageResponse;
  conversation?: AIConversationResponse;
  frontierQueue?: FrontierQueueResponse;
  frontier_queue?: FrontierQueueResponse;
}

type AIActionRunInputPayload = NonNullable<AIActionRunRequest['input']> & {
  paths?: string[];
  lane?: string;
  type?: string;
  limit?: number;
  orchestratorRunId?: string;
  orchestratorObservationId?: string;
  intentId?: string;
  leaseId?: string;
  attemptId?: string;
  transcript?: string;
  failureKind?: string;
  failureMessage?: string;
  statusFilter?: string;
  priorityFilter?: string;
  maxTokens?: number;
  budget?: number;
  maxItems?: number;
  max_items?: number;
  summary?: string;
  severity?: string;
  artifactPath?: string;
  endpoint?: string;
  description?: string;
  recommendation?: string;
  evidence?: string;
};

interface RuntimeWorkerResponse {
  workerId?: string;
  worker_id?: string;
  mode?: string;
  status?: string;
  endpoint?: string;
  capabilities?: string[];
  labels?: Record<string, string>;
  registeredAt?: string;
  registered_at?: string;
  heartbeatAt?: string;
  heartbeat_at?: string;
  stale?: boolean;
  path?: string;
}

interface RuntimeWorkerOrchestrationResponse {
  path?: string;
  status?: string;
  mode?: string;
  runId?: string;
  run_id?: string;
  transportMode?: string;
  transport_mode?: string;
  createdAt?: string;
  created_at?: string;
  maxCandidates?: number;
  max_candidates?: number;
  placementReady?: number;
  placement_ready?: number;
  placementSkipped?: number;
  placement_skipped?: number;
  placementPlanPath?: string;
  placement_plan_path?: string;
  claimedCount?: number;
  claimed_count?: number;
  claimSkipped?: number;
  claim_skipped?: number;
  claimFailed?: number;
  claim_failed?: number;
  claimExecutionPaths?: string[];
  claim_execution_paths?: string[];
  placementDispatchPlanPath?: string;
  placement_dispatch_plan_path?: string;
  dispatchReady?: number;
  dispatch_ready?: number;
  dispatchSkipped?: number;
  dispatch_skipped?: number;
  placementDispatchBatchPath?: string;
  placement_dispatch_batch_path?: string;
  placementDispatchBatchStatus?: string;
  placement_dispatch_batch_status?: string;
  dispatchedCount?: number;
  dispatched_count?: number;
  dispatchFailed?: number;
  dispatch_failed?: number;
  includeLifecycleMaintenance?: boolean;
  include_lifecycle_maintenance?: boolean;
  executeLifecycleRetryDispatches?: boolean;
  execute_lifecycle_retry_dispatches?: boolean;
  dispatchLifecycleRetryClaims?: boolean;
  dispatch_lifecycle_retry_claims?: boolean;
  lifecycleRetryTransportMode?: string;
  lifecycle_retry_transport_mode?: string;
  lifecycleMaintenancePath?: string;
  lifecycle_maintenance_path?: string;
  lifecycleAuditPath?: string;
  lifecycle_audit_path?: string;
  retryDispatchExecutionsCreated?: number;
  retry_dispatch_executions_created?: number;
  retryDispatchClaimsCreated?: number;
  retry_dispatch_claims_created?: number;
  retryDispatchClaimsDispatched?: number;
  retry_dispatch_claims_dispatched?: number;
  retryDispatchTransportRequestsCreated?: number;
  retry_dispatch_transport_requests_created?: number;
  retryDispatchExecutionPaths?: string[];
  retry_dispatch_execution_paths?: string[];
  error?: string;
}

interface RuntimeWorkerStateResponse {
  workers?: RuntimeWorkerResponse[];
  orchestrations?: RuntimeWorkerOrchestrationResponse[];
  counts?: Record<string, unknown>;
  updatedAt?: string;
  updated_at?: string;
  staleAfterSeconds?: number;
  stale_after_seconds?: number;
}

interface AIActionResultResponse {
  kind?: string;
  operation?: string;
  path?: string;
  targetPath?: string;
  goal?: string;
  query?: string;
  file?: {
    path?: string;
    name?: string;
    kind?: string;
    content?: string;
    size?: number;
    modifiedAt?: number;
  };
  fileContents?: Array<{
    path?: string;
    name?: string;
    kind?: string;
    content?: string;
    size?: number;
    modifiedAt?: number;
  }>;
  files?: WorkspaceFileResponse[];
  fileChanges?: AIActionFileChangeResponse[];
  file_changes?: AIActionFileChangeResponse[];
  fileDiffs?: HarnessFileDiffResponse[];
  file_diffs?: HarnessFileDiffResponse[];
  deletedFile?: WorkspaceFileResponse;
  searchResults?: Array<{
    path?: string;
    line?: number;
    text?: string;
  }>;
  patch?: string;
  evidenceItem?: EvidenceItemResponse;
  evidenceItems?: EvidenceItemResponse[];
  reports?: ReportResponse[];
  reportFinding?: ReportFindingResponse;
  reportFindings?: ReportFindingResponse[];
  executions?: ExecutionResponse[];
  executionLogs?: ExecutionLogResponse[];
  executionArtifacts?: ExecutionArtifactResponse[];
  reportExport?: ReportExportResponse;
  blackboardItems?: BlackboardItemResponse[];
  blackboardItem?: BlackboardItemResponse;
  agentState?: AgentStateResponse;
  agentProfile?: AgentProfileResponse;
  agent_profile?: AgentProfileResponse;
  agentTasks?: AgentTaskResponse[];
  agentTask?: AgentTaskResponse;
  agentTaskEvents?: AgentTaskEventResponse[];
  agentTaskEvent?: AgentTaskEventResponse;
  agentProviderRun?: AgentProviderRunResponse;
  agent_provider_run?: AgentProviderRunResponse;
  agentProviderBatchRun?: AgentProviderBatchRunResponse;
  agent_provider_batch_run?: AgentProviderBatchRunResponse;
  runtimeWorkerState?: RuntimeWorkerStateResponse;
  runtime_worker_state?: RuntimeWorkerStateResponse;
  runtimeWorkers?: RuntimeWorkerResponse[];
  runtime_workers?: RuntimeWorkerResponse[];
  runtimeWorkerOrchestrations?: RuntimeWorkerOrchestrationResponse[];
  runtime_worker_orchestrations?: RuntimeWorkerOrchestrationResponse[];
  runtimeWorkerOrchestration?: RuntimeWorkerOrchestrationResponse;
  runtime_worker_orchestration?: RuntimeWorkerOrchestrationResponse;
  harnessState?: HarnessStateResponse;
  harnessCases?: HarnessCaseResponse[];
  harnessCase?: HarnessCaseResponse;
  harnessResults?: HarnessResultResponse[];
  harnessResult?: HarnessResultResponse;
  eventlogEvents?: EventlogEventsResponse;
  eventlog_events?: EventlogEventsResponse;
  aiMessageIds?: string[];
  toolCallIds?: string[];
  orchestratorState?: OrchestratorStateResponse;
  orchestratorRuns?: OrchestratorRunResponse[];
  orchestratorRun?: OrchestratorRunResponse;
  orchestratorObservations?: OrchestratorObservationResponse[];
  orchestratorObservation?: OrchestratorObservationResponse;
  orchestratorAdvances?: OrchestratorAdvanceResponse[];
  orchestratorAdvance?: OrchestratorAdvanceResponse;
  plannerState?: PlannerStateResponse;
  plannerPlans?: PlannerPlanResponse[];
  plannerPlan?: PlannerPlanResponse;
  frontierItems?: FrontierPriorityItemResponse[];
  frontier_items?: FrontierPriorityItemResponse[];
  frontierSummary?: FrontierPrioritySummaryResponse;
  frontier_summary?: FrontierPrioritySummaryResponse;
  frontierDependencies?: FrontierDependencySelectionResponse;
  frontier_dependencies?: FrontierDependencySelectionResponse;
  refreshedFrontierItems?: FrontierPriorityItemResponse[];
  refreshed_frontier_items?: FrontierPriorityItemResponse[];
  refreshedFrontierSummary?: FrontierPrioritySummaryResponse;
  refreshed_frontier_summary?: FrontierPrioritySummaryResponse;
  budget?: number;
  queueSource?: string;
  queue_source?: string;
  queueStatus?: string;
  queue_status?: string;
  promptRegistry?: PromptRegistryResponse;
  prompt_registry?: PromptRegistryResponse;
  prompt?: PromptDefinitionResponse;
  promptVersion?: PromptVersionResponse;
  prompt_version?: PromptVersionResponse;
  promptDiff?: PromptDiffResponse;
  prompt_diff?: PromptDiffResponse;
  promptComparison?: PromptHarnessComparisonResponse;
  comparison?: PromptHarnessComparisonResponse;
  batchComparison?: PromptHarnessBatchComparisonResponse;
  batch_comparison?: PromptHarnessBatchComparisonResponse;
  promptId?: string;
  baseVersionId?: string;
  targetVersionId?: string;
  baseProvider?: string;
  targetProvider?: string;
  baseModel?: string;
  targetModel?: string;
  baseUsage?: AIUsageResponse;
  targetUsage?: AIUsageResponse;
  totalTokens?: number;
  baseOutput?: string;
  targetOutput?: string;
  baseOutputSummary?: string;
  targetOutputSummary?: string;
  baseResultId?: string;
  targetResultId?: string;
  baseHarnessResultId?: string;
  targetHarnessResultId?: string;
  baseStatus?: string;
  targetStatus?: string;
  baseHarnessStatus?: string;
  targetHarnessStatus?: string;
  baseResult?: HarnessResultResponse;
  targetResult?: HarnessResultResponse;
  baseHarnessResult?: HarnessResultResponse;
  targetHarnessResult?: HarnessResultResponse;
  results?: {
    base?: HarnessResultResponse;
    target?: HarnessResultResponse;
  };
  provider?: string;
  model?: string;
  usage?: AIUsageResponse;
  output?: string;
  outputSummary?: string;
  count?: number;
  summary?: string;
}

interface AIActionFileChangeResponse {
  path?: string;
  targetPath?: string;
  target_path?: string;
  operation?: string;
  status?: string;
  summary?: string;
  patch?: string;
  diff?: string;
  source?: string;
  actionId?: string;
  action_id?: string;
  messageId?: string;
  message_id?: string;
  toolCallId?: string;
  tool_call_id?: string;
  createdAt?: string | number;
  created_at?: string | number;
}

interface FrontierPriorityItemResponse {
  id?: string;
  rank?: number;
  score?: number;
  title?: string;
  target?: string;
  hypothesis?: string;
  expectedUnlock?: string;
  expected_unlock?: string;
  reason?: string;
  suggestedAction?: string;
  suggested_action?: string;
  ready?: boolean;
  dependencyIds?: string[];
  dependency_ids?: string[];
  blockedByIds?: string[];
  blocked_by_ids?: string[];
  blockedReason?: string;
  blocked_reason?: string;
  status?: string;
  priority?: string;
  source?: string;
  sourceKind?: string;
  source_kind?: string;
  sourceId?: string;
  source_id?: string;
  blackboardItemId?: string;
  blackboard_item_id?: string;
  agentTaskId?: string;
  agent_task_id?: string;
  filePath?: string;
  file_path?: string;
  executionId?: string;
  execution_id?: string;
  missionValue?: number;
  mission_value?: number;
  unlockPotential?: number;
  unlock_potential?: number;
  evidenceConfidence?: number;
  evidence_confidence?: number;
  novelty?: number;
  cost?: number;
  duplicateRisk?: number;
  duplicate_risk?: number;
  noise?: number;
  exhaustedPenalty?: number;
  exhausted_penalty?: number;
}

interface FrontierPrioritySummaryResponse {
  goal?: string;
  source?: string;
  statusFilter?: string;
  status_filter?: string;
  priorityFilter?: string;
  priority_filter?: string;
  readyFilter?: string;
  ready_filter?: string;
  candidateCount?: number;
  candidate_count?: number;
  totalCandidates?: number;
  total_candidates?: number;
  selectedCount?: number;
  selected_count?: number;
  skippedCount?: number;
  skipped_count?: number;
  readyCount?: number;
  ready_count?: number;
  blockedCount?: number;
  blocked_count?: number;
  budget?: number;
  budgetUsed?: number;
  budget_used?: number;
  budgetRemaining?: number;
  budget_remaining?: number;
  maxItems?: number;
  max_items?: number;
  exhaustedCount?: number;
  exhausted_count?: number;
  duplicateCount?: number;
  duplicate_count?: number;
  openBlackboard?: number;
  open_blackboard?: number;
  queuedTasks?: number;
  queued_tasks?: number;
  summary?: string;
}

interface FrontierQueueResponse {
  path?: string;
  queueSource?: string;
  queue_source?: string;
  queueStatus?: string;
  queue_status?: string;
  items?: FrontierPriorityItemResponse[];
  frontierItems?: FrontierPriorityItemResponse[];
  frontier_items?: FrontierPriorityItemResponse[];
  summary?: FrontierPrioritySummaryResponse;
  frontierSummary?: FrontierPrioritySummaryResponse;
  frontier_summary?: FrontierPrioritySummaryResponse;
  blackboardItems?: BlackboardItemResponse[];
  blackboard_items?: BlackboardItemResponse[];
  agentTasks?: AgentTaskResponse[];
  agent_tasks?: AgentTaskResponse[];
  agentState?: AgentStateResponse;
  agent_state?: AgentStateResponse;
  count?: number;
}

interface FrontierMonitorResponse {
  queue?: FrontierQueueResponse;
  summary?: FrontierMonitorSummaryResponse;
  agentState?: AgentStateResponse;
  agent_state?: AgentStateResponse;
  plannerState?: PlannerStateResponse;
  planner_state?: PlannerStateResponse;
  activeAgentTasks?: AgentTaskResponse[];
  active_agent_tasks?: AgentTaskResponse[];
  activePlannerPlans?: PlannerPlanResponse[];
  active_planner_plans?: PlannerPlanResponse[];
  runningExecutions?: ExecutionResponse[];
  running_executions?: ExecutionResponse[];
  recentAgentEvents?: AgentTaskEventResponse[];
  recent_agent_events?: AgentTaskEventResponse[];
}

interface FrontierMonitorSummaryResponse {
  status?: string;
  source?: string;
  queueStatus?: string;
  queue_status?: string;
  queueItems?: number;
  queue_items?: number;
  readyItems?: number;
  ready_items?: number;
  blockedItems?: number;
  blocked_items?: number;
  activeAgentTasks?: number;
  active_agent_tasks?: number;
  activePlannerPlans?: number;
  active_planner_plans?: number;
  runningExecutions?: number;
  running_executions?: number;
  recentEvents?: number;
  recent_events?: number;
  lastUpdatedAt?: number;
  last_updated_at?: number;
  summary?: string;
}

interface FrontierDependencySelectionResponse {
  path?: string;
  queueSource?: string;
  queue_source?: string;
  status?: string;
  queue?: FrontierQueueResponse;
  summary?: FrontierDependencySummaryResponse;
  clusters?: FrontierDependencyClusterResponse[];
  candidates?: FrontierDependencyCandidateResponse[];
  blackboardItems?: BlackboardItemResponse[];
  blackboard_items?: BlackboardItemResponse[];
  agentTasks?: AgentTaskResponse[];
  agent_tasks?: AgentTaskResponse[];
  agentState?: AgentStateResponse;
  agent_state?: AgentStateResponse;
  count?: number;
}

interface FrontierDependencySummaryResponse {
  goal?: string;
  source?: string;
  status?: string;
  queueStatus?: string;
  queue_status?: string;
  queueItems?: number;
  queue_items?: number;
  readyItems?: number;
  ready_items?: number;
  blockedItems?: number;
  blocked_items?: number;
  clusterCount?: number;
  cluster_count?: number;
  candidateCount?: number;
  candidate_count?: number;
  selectedCount?: number;
  selected_count?: number;
  blockedFrontierItemCount?: number;
  blocked_frontier_item_count?: number;
  missingDependencyCount?: number;
  missing_dependency_count?: number;
  satisfiedDependencyCount?: number;
  satisfied_dependency_count?: number;
  readyDependencyCount?: number;
  ready_dependency_count?: number;
  linkedAgentTaskCount?: number;
  linked_agent_task_count?: number;
  unlockValue?: number;
  unlock_value?: number;
  maxItems?: number;
  max_items?: number;
  budget?: number;
  summary?: string;
}

interface FrontierDependencyCandidateResponse {
  rank?: number;
  id?: string;
  dependencyId?: string;
  dependency_id?: string;
  dependencyTitle?: string;
  dependency_title?: string;
  dependencyStatus?: string;
  dependency_status?: string;
  dependencyReady?: boolean;
  dependency_ready?: boolean;
  dependencySatisfied?: boolean;
  dependency_satisfied?: boolean;
  missingDependency?: boolean;
  missing_dependency?: boolean;
  unlockCount?: number;
  unlock_count?: number;
  unlockValue?: number;
  unlock_value?: number;
  score?: number;
  suggestedActionId?: string;
  suggested_action_id?: string;
  suggestedAction?: string;
  suggested_action?: string;
  suggestedBlackboardItemId?: string;
  suggested_blackboard_item_id?: string;
  suggestedAgentTaskId?: string;
  suggested_agent_task_id?: string;
  suggestedGoal?: string;
  suggested_goal?: string;
  reason?: string;
}

interface FrontierDependencyClusterResponse extends FrontierDependencyCandidateResponse {
  dependency?: BlackboardItemResponse;
  dependencyFound?: boolean;
  dependency_found?: boolean;
  blockedItems?: FrontierPriorityItemResponse[];
  blocked_items?: FrontierPriorityItemResponse[];
  blockedBlackboardItems?: BlackboardItemResponse[];
  blocked_blackboard_items?: BlackboardItemResponse[];
  linkedAgentTasks?: AgentTaskResponse[];
  linked_agent_tasks?: AgentTaskResponse[];
}

interface PromptDiffResponse {
  promptId?: string;
  baseVersionId?: string;
  targetVersionId?: string;
  baseTitle?: string;
  targetTitle?: string;
  baseContent?: string;
  targetContent?: string;
  diff?: string;
  addedLines?: number;
  removedLines?: number;
  changed?: boolean;
}

interface PromptHarnessComparisonResponse {
  promptId?: string;
  baseVersionId?: string;
  targetVersionId?: string;
  basePrompt?: PromptDefinitionResponse;
  targetPrompt?: PromptDefinitionResponse;
  baseVersion?: PromptVersionResponse;
  targetVersion?: PromptVersionResponse;
  baseProvider?: string;
  targetProvider?: string;
  baseModel?: string;
  targetModel?: string;
  baseUsage?: AIUsageResponse;
  targetUsage?: AIUsageResponse;
  totalTokens?: number;
  baseOutput?: string;
  targetOutput?: string;
  baseOutputSummary?: string;
  targetOutputSummary?: string;
  baseResultId?: string;
  targetResultId?: string;
  baseHarnessResultId?: string;
  targetHarnessResultId?: string;
  baseStatus?: string;
  targetStatus?: string;
  baseHarnessStatus?: string;
  targetHarnessStatus?: string;
  baseResult?: HarnessResultResponse;
  targetResult?: HarnessResultResponse;
  baseHarnessResult?: HarnessResultResponse;
  targetHarnessResult?: HarnessResultResponse;
  results?: {
    base?: HarnessResultResponse;
    target?: HarnessResultResponse;
  };
}

interface PromptHarnessBatchComparisonItemResponse {
  versionId?: string;
  version_id?: string;
  version?: PromptVersionResponse | string;
  resultId?: string;
  result_id?: string;
  result?: HarnessResultResponse | string;
  provider?: string;
  model?: string;
  usage?: AIUsageResponse;
  totalTokens?: number;
  total_tokens?: number;
  costUsd?: number;
  cost_usd?: number;
  costKnown?: boolean;
  cost_known?: boolean;
  status?: string;
  output?: string;
  outputSummary?: string;
  output_summary?: string;
  summary?: string;
}

interface PromptHarnessBatchComparisonResponse {
  promptId?: string;
  prompt_id?: string;
  prompt?: PromptDefinitionResponse;
  prompts?: PromptDefinitionResponse[];
  versionIds?: string[];
  version_ids?: string[];
  versions?: Array<PromptVersionResponse | string>;
  results?: HarnessResultResponse[] | Record<string, HarnessResultResponse | undefined>;
  items?: PromptHarnessBatchComparisonItemResponse[];
  sameProvider?: boolean;
  same_provider?: boolean;
  sameModel?: boolean;
  same_model?: boolean;
  providers?: string[];
  models?: string[];
  totalTokens?: number;
  total_tokens?: number;
  totalCostUsd?: number;
  total_cost_usd?: number;
  costKnown?: boolean;
  cost_known?: boolean;
  summary?: string;
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
  } catch (error) {
    if (error instanceof AgentProfileContractError) {
      throw error;
    }
    return mapBootstrapUnavailableData('bootstrap-error', latency());
  }

}

export async function loadEventlogEvents(query: EventlogQuery = {}): Promise<EventlogEvents> {
  const baseURL = await apiBaseURL();
  if (!baseURL) {
    throw new Error(zhCN.shell.statusOffline);
  }
  const params = new URLSearchParams();
  appendEventlogQuery(params, 'runId', query.runId);
  appendEventlogQuery(params, 'lane', query.lane);
  appendEventlogQuery(params, 'type', query.type);
  appendEventlogQuery(params, 'limit', query.limit);
  const response = await requestJSON<EventlogEventsResponse>(baseURL, `/api/events${params.toString() ? `?${params.toString()}` : ''}`);
  return mapEventlogEventsResponse(response);
}

export async function loadFrontierQueue(query: FrontierQueueQuery = {}): Promise<FrontierQueue> {
  const baseURL = await apiBaseURL();
  if (!baseURL) {
    throw new Error(zhCN.shell.statusOffline);
  }
  const params = new URLSearchParams();
  appendFrontierQueueQuery(params, 'goal', query.goal);
  appendFrontierQueueQuery(params, 'status', query.status);
  appendFrontierQueueQuery(params, 'priority', query.priority);
  appendFrontierQueueQuery(params, 'source', query.source);
  appendFrontierQueueQuery(params, 'q', query.q);
  appendFrontierQueueQuery(params, 'readyFilter', query.readyFilter && query.readyFilter !== 'all' ? query.readyFilter : undefined);
  appendFrontierQueueQuery(params, 'budget', query.budget);
  appendFrontierQueueQuery(params, 'maxItems', query.maxItems);
  const path = `/api/frontier/queue${params.toString() ? `?${params.toString()}` : ''}`;
  try {
    const payload = await requestJSON<FrontierQueueResponse>(baseURL, path);
    return mapFrontierQueueResponse(payload);
  } catch (error) {
    if (isUnavailableOptionalEndpoint(error, path)) {
      return mapFrontierQueueResponse({});
    }
    throw error;
  }
}

export async function loadFrontierMonitor(query: FrontierQueueQuery = {}): Promise<FrontierMonitor> {
  const baseURL = await apiBaseURL();
  if (!baseURL) {
    throw new Error(zhCN.shell.statusOffline);
  }
  const params = new URLSearchParams();
  appendFrontierQueueQuery(params, 'goal', query.goal);
  appendFrontierQueueQuery(params, 'status', query.status);
  appendFrontierQueueQuery(params, 'priority', query.priority);
  appendFrontierQueueQuery(params, 'source', query.source);
  appendFrontierQueueQuery(params, 'q', query.q);
  appendFrontierQueueQuery(params, 'readyFilter', query.readyFilter && query.readyFilter !== 'all' ? query.readyFilter : undefined);
  appendFrontierQueueQuery(params, 'budget', query.budget);
  appendFrontierQueueQuery(params, 'maxItems', query.maxItems);
  const path = `/api/frontier/monitor${params.toString() ? `?${params.toString()}` : ''}`;
  try {
    const payload = await requestJSON<FrontierMonitorResponse>(baseURL, path);
    return mapFrontierMonitorResponse(payload);
  } catch (error) {
    if (isUnavailableOptionalEndpoint(error, path)) {
      return mapFrontierMonitorResponse({});
    }
    throw error;
  }
}

export async function loadFrontierDependencies(query: FrontierQueueQuery = {}): Promise<FrontierDependencySelection> {
  const baseURL = await apiBaseURL();
  if (!baseURL) {
    throw new Error(zhCN.shell.statusOffline);
  }
  const params = new URLSearchParams();
  appendFrontierQueueQuery(params, 'goal', query.goal);
  appendFrontierQueueQuery(params, 'status', query.status);
  appendFrontierQueueQuery(params, 'priority', query.priority);
  appendFrontierQueueQuery(params, 'source', query.source);
  appendFrontierQueueQuery(params, 'q', query.q);
  appendFrontierQueueQuery(params, 'readyFilter', query.readyFilter && query.readyFilter !== 'all' ? query.readyFilter : undefined);
  appendFrontierQueueQuery(params, 'budget', query.budget);
  appendFrontierQueueQuery(params, 'maxItems', query.maxItems);
  const path = `/api/frontier/dependencies${params.toString() ? `?${params.toString()}` : ''}`;
  try {
    const payload = await requestJSON<FrontierDependencySelectionResponse>(baseURL, path);
    return mapFrontierDependencySelectionResponse(payload);
  } catch (error) {
    if (isUnavailableOptionalEndpoint(error, path)) {
      return mapFrontierDependencySelectionResponse({});
    }
    throw error;
  }
}

function isUnavailableOptionalEndpoint(error: unknown, path: string): boolean {
  return error instanceof Error && error.message === `${path}: 404`;
}

function appendFrontierQueueQuery(params: URLSearchParams, key: string, value: string | number | undefined) {
  if (typeof value === 'number' && Number.isFinite(value)) {
    params.set(key, String(value));
    return;
  }
  if (typeof value === 'string' && value.trim() !== '') {
    params.set(key, value.trim());
  }
}

function appendEventlogQuery(params: URLSearchParams, key: string, value: string | number | undefined) {
  if (typeof value === 'number' && Number.isFinite(value) && value > 0) {
    params.set(key, String(Math.floor(value)));
    return;
  }
  if (typeof value === 'string' && value.trim() !== '') {
    params.set(key, value.trim());
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

export async function showDesktopWorkspaceFile(path: string): Promise<DesktopShowWorkspaceFileResult | null> {
  if (!window.ouroboros?.showWorkspaceFile) {
    return null;
  }
  return window.ouroboros.showWorkspaceFile(path) as Promise<DesktopShowWorkspaceFileResult>;
}

export async function loadDesktopStatus(): Promise<DesktopStatus> {
  if (!window.ouroboros?.backendStatus) {
    return emptyDesktopStatus();
  }
  try {
    const [backendResult, recentWorkspacesResult, windowStateResult, runtimeResult] = await Promise.allSettled([
      window.ouroboros.backendStatus(),
      window.ouroboros?.recentWorkspaces?.() ?? Promise.resolve([]),
      window.ouroboros?.windowState?.() ?? Promise.resolve({}),
      window.ouroboros?.runtimeInfo?.() ?? Promise.resolve({}),
    ]);
    const recentWorkspaces = recentWorkspacesResult.status === 'fulfilled' ? recentWorkspacesResult.value : [];
    const windowState = windowStateResult.status === 'fulfilled' ? windowStateResult.value : {};
    const runtime = runtimeResult.status === 'fulfilled' ? runtimeResult.value : {};
    if (backendResult.status === 'fulfilled') {
      return mapDesktopStatus(backendResult.value, recentWorkspaces, windowState, runtime);
    }
    return mapDesktopStatus({ status: 'unknown', lastError: errorMessage(backendResult.reason) }, recentWorkspaces, windowState, runtime);
  } catch (error) {
    return mapDesktopStatus({ status: 'unknown', lastError: errorMessage(error) });
  }
}

export async function restartDesktopBackend(): Promise<DesktopStatus> {
  if (!window.ouroboros?.restartBackend) {
    return loadDesktopStatus();
  }
  try {
    const backend = await window.ouroboros.restartBackend();
    const [recentWorkspacesResult, windowStateResult, runtimeResult] = await Promise.allSettled([
      window.ouroboros?.recentWorkspaces?.() ?? Promise.resolve([]),
      window.ouroboros?.windowState?.() ?? Promise.resolve({}),
      window.ouroboros?.runtimeInfo?.() ?? Promise.resolve({}),
    ]);
    const recentWorkspaces = recentWorkspacesResult.status === 'fulfilled' ? recentWorkspacesResult.value : [];
    const windowState = windowStateResult.status === 'fulfilled' ? windowStateResult.value : {};
    const runtime = runtimeResult.status === 'fulfilled' ? runtimeResult.value : {};
    return mapDesktopStatus(backend, recentWorkspaces, windowState, runtime);
  } catch (error) {
    const [backendResult, recentWorkspacesResult, windowStateResult, runtimeResult] = await Promise.allSettled([
      window.ouroboros?.backendStatus?.() ?? Promise.resolve({ status: 'unknown', lastError: errorMessage(error) }),
      window.ouroboros?.recentWorkspaces?.() ?? Promise.resolve([]),
      window.ouroboros?.windowState?.() ?? Promise.resolve({}),
      window.ouroboros?.runtimeInfo?.() ?? Promise.resolve({}),
    ]);
    const backend = backendResult.status === 'fulfilled'
      ? backendResult.value
      : { status: 'unknown', lastError: errorMessage(error) };
    const recentWorkspaces = recentWorkspacesResult.status === 'fulfilled' ? recentWorkspacesResult.value : [];
    const windowState = windowStateResult.status === 'fulfilled' ? windowStateResult.value : {};
    const runtime = runtimeResult.status === 'fulfilled' ? runtimeResult.value : {};
    const status = mapDesktopStatus(backend, recentWorkspaces, windowState, runtime);
    return {
      ...status,
      lastError: status.lastError || errorMessage(error),
    };
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
  const baseURL = await waitForAPIBaseURL();
  if (!baseURL || typeof WebSocket === 'undefined') {
    return () => undefined;
  }
  const url = new URL('/api/events/stream', baseURL);
  url.protocol = url.protocol === 'https:' ? 'wss:' : 'ws:';

  // Auto-reconnect with exponential backoff: the backend restarts during
  // normal operation (restartBackend, desktop updates). Without reconnect
  // every live push (frontier queue, execution status) silently dies until
  // the whole GUI is reopened — only the 30s poll kept working.
  let disposed = false;
  let attempt = 0;
  let socket: WebSocket | null = null;
  let reconnectTimer: number | undefined;

  const connect = () => {
    if (disposed) {
      return;
    }
    socket = new WebSocket(url.toString());
    socket.addEventListener('message', event => {
      attempt = 0; // healthy traffic resets the backoff
      try {
        onEvent(JSON.parse(event.data) as IDEEvent);
      } catch {
        onEvent({ type: 'stream:invalid-message' });
      }
    });
    socket.addEventListener('error', () => onEvent({ type: 'stream:error' }));
    socket.addEventListener('close', () => {
      socket = null;
      if (disposed) {
        return;
      }
      onEvent({ type: 'stream:closed' });
      const delay = Math.min(1000 * 2 ** attempt, 30_000);
      attempt += 1;
      reconnectTimer = window.setTimeout(connect, delay);
    });
  };
  connect();

  return () => {
    disposed = true;
    if (reconnectTimer !== undefined) {
      window.clearTimeout(reconnectTimer);
    }
    socket?.close();
  };
}

async function waitForAPIBaseURL(): Promise<string> {
  const deadline = Date.now() + 30_000;
  while (Date.now() < deadline) {
    const baseURL = await apiBaseURL();
    if (baseURL) {
      return baseURL;
    }
    if (!window.ouroboros?.getAPIBaseURL) {
      return '';
    }
    await new Promise(resolve => window.setTimeout(resolve, 250));
  }
  return '';
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
    signal: AbortSignal.timeout(REQUEST_TIMEOUT_MS),
  });
  if (!response.ok) {
    throw new Error(`${path}: ${response.status}`);
  }
  return response.json() as Promise<WorkspaceFileContent>;
}

export async function loadReportFile(path: string): Promise<ReportFileContent> {
  const baseURL = await apiBaseURL();
  if (!baseURL) {
    throw new Error(zhCN.shell.statusOffline);
  }
  const response = await requestJSON<ReportFileResponse>(baseURL, `/api/reports/file?path=${encodeURIComponent(path)}`);
  return mapReportFileResponse(response, path);
}

export async function saveReportFile(path: string, content: string): Promise<ReportFileContent> {
  const baseURL = await apiBaseURL();
  if (!baseURL) {
    throw new Error(zhCN.shell.statusOffline);
  }
  const response = await fetch(`${baseURL}/api/reports/file?path=${encodeURIComponent(path)}`, {
    method: 'PUT',
    headers: { Accept: 'application/json', 'Content-Type': 'application/json' },
    body: JSON.stringify({ content }),
    signal: AbortSignal.timeout(REQUEST_TIMEOUT_MS),
  });
  if (!response.ok) {
    throw new Error(`${path}: ${response.status}`);
  }
  return mapReportFileResponse(await response.json() as ReportFileResponse, path);
}

export async function renameWorkspaceFile(path: string, targetPath: string): Promise<WorkspaceFileContent> {
  const baseURL = await apiBaseURL();
  if (!baseURL) {
    throw new Error(zhCN.shell.statusOffline);
  }
  const response = await fetch(`${baseURL}/api/workspace/file?path=${encodeURIComponent(path)}`, {
    method: 'PATCH',
    headers: { Accept: 'application/json', 'Content-Type': 'application/json' },
    body: JSON.stringify({ targetPath }),
    signal: AbortSignal.timeout(REQUEST_TIMEOUT_MS),
  });
  if (!response.ok) {
    throw new Error(`${path}: ${response.status}`);
  }
  return response.json() as Promise<WorkspaceFileContent>;
}

export async function deleteWorkspaceFile(path: string): Promise<WorkspaceFile> {
  const baseURL = await apiBaseURL();
  if (!baseURL) {
    throw new Error(zhCN.shell.statusOffline);
  }
  const response = await fetch(`${baseURL}/api/workspace/file?path=${encodeURIComponent(path)}`, {
    method: 'DELETE',
    headers: { Accept: 'application/json' },
    signal: AbortSignal.timeout(REQUEST_TIMEOUT_MS),
  });
  if (!response.ok) {
    throw new Error(`${path}: ${response.status}`);
  }
  return response.json() as Promise<WorkspaceFile>;
}

export async function searchWorkspace(query: string): Promise<WorkspaceSearchResult[]> {
  const baseURL = await apiBaseURL();
  if (!baseURL || query.trim() === '') {
    return [];
  }
  const response = await requestJSON<{ results?: WorkspaceSearchResult[] }>(baseURL, `/api/workspace/search?q=${encodeURIComponent(query)}`);
  return response.results ?? [];
}

export async function fetchAIConversation(): Promise<AIConversation> {
  const baseURL = await apiBaseURL();
  if (!baseURL) {
    throw new Error(zhCN.shell.statusOffline);
  }
  const payload = await requestJSON<AIConversationResponse & LegacyChatConversationResponse>(baseURL, '/api/ai/chat');
  if (Array.isArray(payload.messages) && !payload.contextReferences && !payload.context_references) {
    return mapLegacyChatConversationResponse(payload);
  }
  return mapAIConversationResponse(payload, zhCN.ai.conversationMissing);
}

export async function fetchAIConversationSessions(): Promise<AIConversationSessionSummary[]> {
  const baseURL = await apiBaseURL();
  if (!baseURL) {
    throw new Error(zhCN.shell.statusOffline);
  }
  const payload = await requestJSON<AIConversationSessionsResponsePayload>(baseURL, aiConversationSessionsPath);
  return mapAIConversationSessions(payload.sessions);
}

export async function startNewAIConversationSession(): Promise<AIConversationSessionOperationResponse> {
  const baseURL = await apiBaseURL();
  if (!baseURL) {
    throw new Error(zhCN.shell.statusOffline);
  }
  const response = await fetch(`${baseURL}${aiConversationSessionNewPath}`, {
    method: 'POST',
    headers: { Accept: 'application/json', 'Content-Type': 'application/json' },
    signal: AbortSignal.timeout(REQUEST_TIMEOUT_MS),
  });
  if (!response.ok) {
    throw new Error(await responseErrorMessage(response, zhCN.ai.sessionOperationFailed));
  }
  return mapAIConversationSessionOperationResponse(await response.json() as AIConversationSessionOperationResponsePayload);
}

export async function restoreAIConversationSession(sessionId: string): Promise<AIConversationSessionOperationResponse> {
  const normalized = sessionId.trim();
  if (!normalized) {
    throw new Error(zhCN.ai.sessionMissing);
  }
  const baseURL = await apiBaseURL();
  if (!baseURL) {
    throw new Error(zhCN.shell.statusOffline);
  }
  const response = await fetch(`${baseURL}${aiConversationSessionRestorePath}`, {
    method: 'POST',
    headers: { Accept: 'application/json', 'Content-Type': 'application/json' },
    body: JSON.stringify({ sessionId: normalized }),
    signal: AbortSignal.timeout(REQUEST_TIMEOUT_MS),
  });
  if (!response.ok) {
    throw new Error(await responseErrorMessage(response, zhCN.ai.sessionOperationFailed));
  }
  return mapAIConversationSessionOperationResponse(await response.json() as AIConversationSessionOperationResponsePayload);
}

async function fetchAIConversationSessionSearch(query: string): Promise<AIConversationSearchResponse> {
  const normalized = query.trim();
  const baseURL = await apiBaseURL();
  if (!baseURL) {
    throw new Error(zhCN.shell.statusOffline);
  }
  if (!normalized) {
    return { query: '', results: [], count: 0 };
  }
  const payload = await requestJSON<AIConversationSearchResponsePayload>(baseURL, `${aiConversationSearchPath}?q=${encodeURIComponent(normalized)}`);
  return mapAIConversationSearchResponse(payload, normalized);
}

export async function searchAIConversationSessions(query: string): Promise<AIConversationSearchResult[]> {
  const response = await fetchAIConversationSessionSearch(query);
  return response.results;
}

export async function exportAIConversationSession(request: AIConversationExportRequest = {}): Promise<AIConversationExportResult> {
  const baseURL = await apiBaseURL();
  if (!baseURL) {
    throw new Error(zhCN.shell.statusOffline);
  }
  const body: AIConversationExportRequestPayload = {
    format: request.format?.trim() || 'markdown',
    includeContextReferences: request.includeContextReferences,
  };
  const normalizedSessionId = request.sessionId?.trim();
  if (normalizedSessionId) {
    body.sessionId = normalizedSessionId;
  }
  const response = await fetch(`${baseURL}${aiConversationExportPath}`, {
    method: 'POST',
    headers: { Accept: 'application/json', 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
    signal: AbortSignal.timeout(REQUEST_TIMEOUT_MS),
  });
  if (!response.ok) {
    throw new Error(await responseErrorMessage(response, zhCN.ai.sessionExportFailed));
  }
  return mapAIConversationExportResult(await response.json() as AIConversationExportResponsePayload);
}

export async function sendAIChatMessage(message: AIChatRequest['message'], contextReferences: AIContextReference[] = []): Promise<AIChatResponse> {
  if (message.trim() === '') {
    throw new Error(zhCN.ai.emptyMessage);
  }
  const baseURL = await apiBaseURL();
  if (!baseURL) {
    throw new Error(zhCN.shell.statusOffline);
  }
  const response = await fetch(`${baseURL}/api/ai/chat`, {
    method: 'POST',
    headers: { Accept: 'application/json', 'Content-Type': 'application/json' },
    body: JSON.stringify({ message, contextReferences }),
    signal: AbortSignal.timeout(REQUEST_TIMEOUT_MS),
  });
  if (!response.ok) {
    const fallback = [409, 412, 424, 428].includes(response.status) ? zhCN.ai.providerMissing : zhCN.ai.sendFailed;
    throw new Error(await responseErrorMessage(response, fallback));
  }
  const payload = await response.json() as AIChatResponsePayload & LegacyChatPostResponse;
  if (!payload.conversation || !payload.message || !payload.provider || !payload.model || !payload.usage) {
    return mapLegacyChatPostResponse(payload);
  }
  return mapAIChatResponse(payload);
}

export async function streamAIChatMessage(
  message: AIChatRequest['message'],
  contextReferences: AIContextReference[] = [],
  onEvent: (event: AIChatStreamEvent) => void,
  options: { signal?: AbortSignal } = {},
): Promise<AIChatResponse> {
  if (message.trim() === '') {
    throw new Error(zhCN.ai.emptyMessage);
  }
  const baseURL = await apiBaseURL();
  if (!baseURL) {
    throw new Error(zhCN.shell.statusOffline);
  }
  const { signal } = options;
  const response = await fetch(`${baseURL}/api/ai/chat/stream`, {
    method: 'POST',
    headers: { Accept: 'text/event-stream', 'Content-Type': 'application/json' },
    body: JSON.stringify({ message, contextReferences }),
    signal,
  });
  if (!response.ok) {
    const fallback = [404, 405, 501].includes(response.status)
      ? zhCN.ai.streamUnsupported
      : [409, 412, 424, 428].includes(response.status)
        ? zhCN.ai.providerMissing
        : zhCN.ai.sendFailed;
    throw new Error(await responseErrorMessage(response, fallback));
  }

  const reader = response.body && typeof response.body.getReader === 'function' ? response.body.getReader() : null;
  if (!reader) {
    throw new Error(zhCN.ai.streamReaderMissing);
  }

  const decoder = new TextDecoder();
  let buffer = '';
  let eventType = 'message';
  let dataLines: string[] = [];
  let finalResponse: AIChatResponse | null = null;
  let streamErrorMessage = '';

  const dispatchEvent = () => {
    if (dataLines.length === 0) {
      eventType = 'message';
      return;
    }
    const rawData = dataLines.join('\n');
    dataLines = [];
    let payload: AIChatStreamEventPayload;
    try {
      payload = JSON.parse(rawData) as AIChatStreamEventPayload;
    } catch {
      throw new Error(zhCN.ai.streamEventInvalid);
    }
    if (!payload || typeof payload !== 'object' || Array.isArray(payload)) {
      throw new Error(zhCN.ai.streamEventInvalid);
    }
    const streamEvent = mapAIChatStreamEvent(payload, eventType);
    eventType = 'message';
    onEvent(streamEvent);
    if (streamEvent.response) {
      finalResponse = streamEvent.response;
    }
    if (isAIChatStreamErrorEvent(streamEvent)) {
      streamErrorMessage = streamEvent.message || zhCN.ai.sendFailed;
    }
  };

  const processLine = (rawLine: string) => {
    const line = rawLine.endsWith('\r') ? rawLine.slice(0, -1) : rawLine;
    if (line === '') {
      dispatchEvent();
      return;
    }
    if (line.startsWith(':')) {
      return;
    }
    const separatorIndex = line.indexOf(':');
    const field = separatorIndex === -1 ? line : line.slice(0, separatorIndex);
    let value = separatorIndex === -1 ? '' : line.slice(separatorIndex + 1);
    if (value.startsWith(' ')) {
      value = value.slice(1);
    }
    if (field === 'event') {
      eventType = value || 'message';
    } else if (field === 'data') {
      dataLines.push(value);
    }
  };

  const drainBuffer = () => {
    let lineBreakIndex = buffer.search(/\r\n|\r|\n/);
    while (lineBreakIndex >= 0) {
      const line = buffer.slice(0, lineBreakIndex);
      const lineBreakLength = buffer.startsWith('\r\n', lineBreakIndex) ? 2 : 1;
      buffer = buffer.slice(lineBreakIndex + lineBreakLength);
      processLine(line);
      lineBreakIndex = buffer.search(/\r\n|\r|\n/);
    }
  };

  try {
    while (true) {
      const { done, value } = await reader.read();
      if (done) {
        break;
      }
      buffer += decoder.decode(value, { stream: true });
      drainBuffer();
    }
    buffer += decoder.decode();
    drainBuffer();
    if (buffer !== '') {
      processLine(buffer);
      buffer = '';
    }
    dispatchEvent();
  } finally {
    reader.releaseLock();
  }

  if (!finalResponse) {
    throw new Error(streamErrorMessage || zhCN.ai.streamResponseMissing);
  }
  return finalResponse;
}

export async function loadRuntimeWorkerState(): Promise<RuntimeWorkerState> {
  const baseURL = await apiBaseURL();
  if (!baseURL) {
    throw new Error(zhCN.shell.statusOffline);
  }
  const payload = await requestJSON<RuntimeWorkerStateResponse>(baseURL, '/api/runtime/workers');
  return mapRuntimeWorkerStateResponse(payload);
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
  const input: AIActionRunInputPayload | undefined = request.input;
  request = {
    ...request,
    actionId: request.actionId,
    input,
  };
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
    signal: AbortSignal.timeout(REQUEST_TIMEOUT_MS),
  });
  if (!response.ok) {
    throw new Error(await responseErrorMessage(response, zhCN.aiActions.runFailed));
  }
  const payload = await response.json() as AIActionRunResponsePayload;
  return mapAIActionRunResponse(payload);
}

export async function createBlackboardItem(request: BlackboardItemCreateRequest): Promise<BlackboardItem> {
  const baseURL = await apiBaseURL();
  if (!baseURL) {
    throw new Error(zhCN.shell.statusOffline);
  }
  const response = await fetch(`${baseURL}${blackboardPath}`, {
    method: 'POST',
    headers: { Accept: 'application/json', 'Content-Type': 'application/json' },
    body: JSON.stringify(request),
    signal: AbortSignal.timeout(REQUEST_TIMEOUT_MS),
  });
  if (!response.ok) {
    throw new Error(await responseErrorMessage(response, zhCN.ai.collaborationDispatchFailed));
  }
  const mapped = mapBlackboardItem(await response.json() as BlackboardItemResponse);
  if (!mapped) {
    throw new Error(`${zhCN.ai.collaborationDispatchFailed}: ${zhCN.ai.blackboardItemMissing}`);
  }
  return mapped;
}

export async function fetchBlackboard(query: BlackboardQuery = {}): Promise<BlackboardState> {
  const baseURL = await apiBaseURL();
  if (!baseURL) {
    throw new Error(zhCN.shell.statusOffline);
  }
  const queryString = blackboardQueryString(query);
  const response = await fetch(`${baseURL}${blackboardPath}${queryString}`, {
    headers: { Accept: 'application/json' },
    signal: AbortSignal.timeout(REQUEST_TIMEOUT_MS),
  });
  if (!response.ok) {
    throw new Error(await responseErrorMessage(response, zhCN.blackboard.filterFailed));
  }
  return mapBlackboard(await response.json() as BlackboardResponse);
}

export async function queryBlackboardItems(query: BlackboardQuery = {}): Promise<BlackboardState> {
  return fetchBlackboard(query);
}

// 假定后端端点：GET /api/ai/blackboard/observations（与 /api/blackboard/observations 同一实现）。
// 支持 kind/target/source/q 过滤与 limit；返回判定前的原始观测，不做任何解释。
export async function fetchBlackboardObservations(query: BlackboardObservationQuery = {}): Promise<BlackboardObservationState> {
  const baseURL = await apiBaseURL();
  if (!baseURL) {
    throw new Error(zhCN.shell.statusOffline);
  }
  const response = await fetch(`${baseURL}${blackboardPath}/observations${blackboardObservationQueryString(query)}`, {
    headers: { Accept: 'application/json' },
    signal: AbortSignal.timeout(REQUEST_TIMEOUT_MS),
  });
  if (!response.ok) {
    throw new Error(await responseErrorMessage(response, zhCN.blackboard.observationLoadFailed));
  }
  const payload = await response.json() as BlackboardObservationsResponse;
  const observations = (payload.observations ?? []).flatMap(observation => {
    const mapped = mapBlackboardObservation(observation);
    return mapped ? [mapped] : [];
  });
  return {
    path: payload.path || '',
    observations,
    count: isFiniteNumber(payload.count) ? payload.count : observations.length,
  };
}

export async function updateBlackboardItem(id: string, request: BlackboardItemUpdateRequest): Promise<BlackboardItem> {
  const baseURL = await apiBaseURL();
  if (!baseURL) {
    throw new Error(zhCN.shell.statusOffline);
  }
  const response = await fetch(`${baseURL}${blackboardPath}/items/${encodeURIComponent(id)}`, {
    method: 'PATCH',
    headers: { Accept: 'application/json', 'Content-Type': 'application/json' },
    body: JSON.stringify(request),
    signal: AbortSignal.timeout(REQUEST_TIMEOUT_MS),
  });
  if (!response.ok) {
    throw new Error(await responseErrorMessage(response, zhCN.blackboard.updateFailed));
  }
  const mapped = mapBlackboardItem(await response.json() as BlackboardItemResponse);
  if (!mapped) {
    throw new Error(`${zhCN.blackboard.updateFailed}: ${zhCN.ai.blackboardItemMissing}`);
  }
  return mapped;
}

export async function batchUpdateBlackboardItems(request: BlackboardBatchUpdateRequest): Promise<BlackboardBatchUpdateResult> {
  const baseURL = await apiBaseURL();
  if (!baseURL) {
    throw new Error(zhCN.shell.statusOffline);
  }
  const response = await fetch(`${baseURL}${blackboardPath}/items/batch`, {
    method: 'POST',
    headers: { Accept: 'application/json', 'Content-Type': 'application/json' },
    body: JSON.stringify(request),
    signal: AbortSignal.timeout(REQUEST_TIMEOUT_MS),
  });
  if (!response.ok) {
    throw new Error(await responseErrorMessage(response, zhCN.blackboard.batchUpdateFailed));
  }
  return mapBlackboardBatchUpdateResult(await response.json() as BlackboardBatchUpdateResponse);
}

export async function linkBlackboardItemDependencies(id: string, request: BlackboardDependencyLinkRequest): Promise<BlackboardDependencyLinkResult> {
  const baseURL = await apiBaseURL();
  if (!baseURL) {
    throw new Error(zhCN.shell.statusOffline);
  }
  const response = await fetch(`${baseURL}${blackboardPath}/items/${encodeURIComponent(id)}/dependencies`, {
    method: 'POST',
    headers: { Accept: 'application/json', 'Content-Type': 'application/json' },
    body: JSON.stringify(request),
    signal: AbortSignal.timeout(REQUEST_TIMEOUT_MS),
  });
  if (!response.ok) {
    throw new Error(await responseErrorMessage(response, zhCN.blackboard.dependencyLinkFailed));
  }
  const mapped = mapBlackboardDependencyLinkResult(await response.json() as BlackboardDependencyLinkResponse);
  if (!mapped) {
    throw new Error(`${zhCN.blackboard.dependencyLinkFailed}: ${zhCN.ai.blackboardItemMissing}`);
  }
  return mapped;
}

export async function fetchAgentProfiles(): Promise<AgentProfile[]> {
  const baseURL = await apiBaseURL();
  if (!baseURL) {
    throw new Error(zhCN.shell.statusOffline);
  }
  const response = await fetch(`${baseURL}${agentProfilesPath}`, {
    method: 'GET',
    headers: { Accept: 'application/json' },
    signal: AbortSignal.timeout(REQUEST_TIMEOUT_MS),
  });
  if (!response.ok) {
    throw new Error(await responseErrorMessage(response, zhCN.agents.profileLoadFailed));
  }
  const payload = await response.json() as AgentProfileResponse[] | AgentProfilesResponse;
  return mapAgentProfilesPayload(payload, zhCN.agents.profileLoadFailed);
}

export async function createAgentProfile(request: AgentProfileCreateRequest): Promise<AgentProfile> {
  const baseURL = await apiBaseURL();
  if (!baseURL) {
    throw new Error(zhCN.shell.statusOffline);
  }
  const response = await fetch(`${baseURL}${agentProfilesPath}`, {
    method: 'POST',
    headers: { Accept: 'application/json', 'Content-Type': 'application/json' },
    body: JSON.stringify(request),
    signal: AbortSignal.timeout(REQUEST_TIMEOUT_MS),
  });
  if (!response.ok) {
    throw new Error(await responseErrorMessage(response, zhCN.agents.profileCreateFailed));
  }
  const mapped = mapAgentProfile(await response.json() as AgentProfileResponse);
  if (!mapped) {
    throw new Error(`${zhCN.agents.profileCreateFailed}: ${zhCN.agents.profileMissing}`);
  }
  return mapped;
}

export async function updateAgentProfile(id: string, request: AgentProfileUpdateRequest): Promise<AgentProfile> {
  const baseURL = await apiBaseURL();
  if (!baseURL) {
    throw new Error(zhCN.shell.statusOffline);
  }
  const response = await fetch(`${baseURL}${agentProfilesPath}/${encodeURIComponent(id)}`, {
    method: 'PATCH',
    headers: { Accept: 'application/json', 'Content-Type': 'application/json' },
    body: JSON.stringify(request),
    signal: AbortSignal.timeout(REQUEST_TIMEOUT_MS),
  });
  if (!response.ok) {
    throw new Error(await responseErrorMessage(response, zhCN.agents.profileUpdateFailed));
  }
  const mapped = mapAgentProfile(await response.json() as AgentProfileResponse);
  if (!mapped) {
    throw new Error(`${zhCN.agents.profileUpdateFailed}: ${zhCN.agents.profileMissing}`);
  }
  return mapped;
}

export async function deleteAgentProfile(id: string): Promise<void> {
  const baseURL = await apiBaseURL();
  if (!baseURL) {
    throw new Error(zhCN.shell.statusOffline);
  }
  const response = await fetch(`${baseURL}${agentProfilesPath}/${encodeURIComponent(id)}`, {
    method: 'DELETE',
    headers: { Accept: 'application/json' },
    signal: AbortSignal.timeout(REQUEST_TIMEOUT_MS),
  });
  if (!response.ok) {
    throw new Error(await responseErrorMessage(response, zhCN.agents.profileDeleteFailed));
  }
}

export async function createAgentTask(request: AgentTaskCreateRequest): Promise<AgentTask> {
  const baseURL = await apiBaseURL();
  if (!baseURL) {
    throw new Error(zhCN.shell.statusOffline);
  }
  const response = await fetch(`${baseURL}${agentTasksPath}`, {
    method: 'POST',
    headers: { Accept: 'application/json', 'Content-Type': 'application/json' },
    body: JSON.stringify(request),
    signal: AbortSignal.timeout(REQUEST_TIMEOUT_MS),
  });
  if (!response.ok) {
    throw new Error(await responseErrorMessage(response, zhCN.ai.collaborationDispatchFailed));
  }
  const mapped = mapAgentTask(await response.json() as AgentTaskResponse);
  if (!mapped) {
    throw new Error(`${zhCN.ai.collaborationDispatchFailed}: ${zhCN.ai.agentTaskMissing}`);
  }
  return mapped;
}

export async function updateAgentTask(id: string, request: AgentTaskUpdateRequest): Promise<AgentTask> {
  const baseURL = await apiBaseURL();
  if (!baseURL) {
    throw new Error(zhCN.shell.statusOffline);
  }
  const response = await fetch(`${baseURL}${agentTasksPath}/${encodeURIComponent(id)}`, {
    method: 'PATCH',
    headers: { Accept: 'application/json', 'Content-Type': 'application/json' },
    body: JSON.stringify(request),
    signal: AbortSignal.timeout(REQUEST_TIMEOUT_MS),
  });
  if (!response.ok) {
    throw new Error(await responseErrorMessage(response, zhCN.ai.agentTaskLifecycleFailed));
  }
  const mapped = mapAgentTask(await response.json() as AgentTaskResponse);
  if (!mapped) {
    throw new Error(`${zhCN.ai.agentTaskLifecycleFailed}: ${zhCN.ai.agentTaskMissing}`);
  }
  return mapped;
}

export async function createAgentTaskEvent(taskId: string, request: AgentTaskEventCreateRequest): Promise<AgentTaskEvent> {
  const baseURL = await apiBaseURL();
  if (!baseURL) {
    throw new Error(zhCN.shell.statusOffline);
  }
  const response = await fetch(`${baseURL}${agentTasksPath}/${encodeURIComponent(taskId)}/events`, {
    method: 'POST',
    headers: { Accept: 'application/json', 'Content-Type': 'application/json' },
    body: JSON.stringify(request),
    signal: AbortSignal.timeout(REQUEST_TIMEOUT_MS),
  });
  if (!response.ok) {
    throw new Error(await responseErrorMessage(response, zhCN.agents.eventCreateFailed));
  }
  const mapped = mapAgentTaskEvent(await response.json() as AgentTaskEventResponse);
  if (!mapped) {
    throw new Error(`${zhCN.agents.eventCreateFailed}: ${zhCN.agents.eventMissing}`);
  }
  return mapped;
}

export async function runAgentTaskWithProvider(taskId: string, request: AgentProviderRunRequest): Promise<AgentProviderRun> {
  const baseURL = await apiBaseURL();
  if (!baseURL) {
    throw new Error(zhCN.shell.statusOffline);
  }
  const response = await fetch(`${baseURL}${agentTasksPath}/${encodeURIComponent(taskId)}/run`, {
    method: 'POST',
    headers: { Accept: 'application/json', 'Content-Type': 'application/json' },
    body: JSON.stringify(request),
    signal: AbortSignal.timeout(REQUEST_TIMEOUT_MS),
  });
  if (!response.ok) {
    throw new Error(await responseErrorMessage(response, zhCN.agents.providerRunFailed));
  }
  const mapped = mapAgentProviderRun(await response.json() as AgentProviderRunResponse);
  if (!mapped) {
    throw new Error(`${zhCN.agents.providerRunFailed}: ${zhCN.agents.providerRunMissing}`);
  }
  return mapped;
}

export async function runAgentTasksWithProviderBatch(request: AgentProviderBatchRunRequest): Promise<AgentProviderBatchRun> {
  const baseURL = await apiBaseURL();
  if (!baseURL) {
    throw new Error(zhCN.shell.statusOffline);
  }
  const response = await fetch(`${baseURL}${agentTasksPath}/run`, {
    method: 'POST',
    headers: { Accept: 'application/json', 'Content-Type': 'application/json' },
    body: JSON.stringify(request),
    signal: AbortSignal.timeout(REQUEST_TIMEOUT_MS),
  });
  if (!response.ok) {
    throw new Error(await responseErrorMessage(response, zhCN.agents.providerBatchRunFailed));
  }
  const mapped = mapAgentProviderBatchRun(await response.json() as AgentProviderBatchRunResponse);
  if (!mapped) {
    throw new Error(`${zhCN.agents.providerBatchRunFailed}: ${zhCN.agents.providerBatchRunMissing}`);
  }
  return mapped;
}

export async function fetchPromptRegistry(): Promise<PromptRegistry> {
  const baseURL = await apiBaseURL();
  if (!baseURL) {
    throw new Error(zhCN.shell.statusOffline);
  }
  const payload = await requestJSON<PromptRegistryResponse>(baseURL, '/api/ai/prompts');
  return mapPromptRegistryResponse(payload);
}

export async function savePromptRegistry(registry: PromptRegistry): Promise<PromptRegistry> {
  const baseURL = await apiBaseURL();
  if (!baseURL) {
    throw new Error(zhCN.shell.statusOffline);
  }
  const response = await fetch(`${baseURL}/api/prompts`, {
    method: 'PUT',
    headers: { Accept: 'application/json', 'Content-Type': 'application/json' },
    body: JSON.stringify(registry),
    signal: AbortSignal.timeout(REQUEST_TIMEOUT_MS),
  });
  if (!response.ok) {
    throw new Error(await responseErrorMessage(response, zhCN.settings.actions.savePromptsFailed));
  }
  const payload = await response.json() as PromptRegistryResponse;
  return mapPromptRegistryResponse(payload);
}

export async function exportEvidenceGraph(): Promise<GraphExportResult> {
  const baseURL = await apiBaseURL();
  if (!baseURL) {
    throw new Error(zhCN.shell.statusOffline);
  }
  const response = await fetch(`${baseURL}/api/evidence/graph/export`, {
    method: 'POST',
    headers: { Accept: 'application/json' },
    signal: AbortSignal.timeout(REQUEST_TIMEOUT_MS),
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
    signal: AbortSignal.timeout(REQUEST_TIMEOUT_MS),
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
    signal: AbortSignal.timeout(REQUEST_TIMEOUT_MS),
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
    signal: AbortSignal.timeout(REQUEST_TIMEOUT_MS),
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

export async function saveProviderConfig(provider: ProviderSettings): Promise<ProviderConfig> {
  const baseURL = await apiBaseURL();
  if (!baseURL) {
    throw new Error(zhCN.shell.statusOffline);
  }
  const response = await fetch(`${baseURL}/api/provider`, {
    method: 'PUT',
    headers: { Accept: 'application/json', 'Content-Type': 'application/json' },
    signal: AbortSignal.timeout(REQUEST_TIMEOUT_MS),
    body: JSON.stringify({
      name: provider.name,
      type: provider.type,
      model: provider.model,
      baseUrl: provider.baseUrl,
      apiKeyEnv: provider.apiKeyEnv,
      apiKeyValue: provider.apiKeyValue,
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

export async function deleteProviderConfig(providerName: string): Promise<ProviderConfig> {
  const baseURL = await apiBaseURL();
  if (!baseURL) {
    throw new Error(zhCN.shell.statusOffline);
  }
  const response = await fetch(`${baseURL}/api/provider/${encodeURIComponent(providerName)}`, {
    method: 'DELETE',
    headers: { Accept: 'application/json' },
    signal: AbortSignal.timeout(REQUEST_TIMEOUT_MS),
  });
  if (!response.ok) {
    throw new Error(`${zhCN.settings.actions.deleteProviderFailed}: ${response.status}`);
  }
  const payload = await response.json() as ProviderConfigResponse;
  return mapRequiredProviderConfigResponse(payload, zhCN.settings.actions.deleteProviderFailed);
}

export async function testProviderConfig(providerName: string, model?: string): Promise<ProviderTestResult> {
  const baseURL = await apiBaseURL();
  if (!baseURL) {
    throw new Error(zhCN.shell.statusOffline);
  }
  const response = await fetch(`${baseURL}/api/provider/test`, {
    method: 'POST',
    headers: { Accept: 'application/json', 'Content-Type': 'application/json' },
    body: JSON.stringify({ providerName, model }),
    signal: AbortSignal.timeout(REQUEST_TIMEOUT_MS),
  });
  if (!response.ok) {
    throw new Error(`${zhCN.settings.actions.testProviderFailed}: ${response.status}`);
  }
  const payload = await response.json() as ProviderTestResultResponse;
  return mapProviderTestResult(payload);
}

export async function loadToolchainPacks(): Promise<ToolchainPackSettings[]> {
  const baseURL = await apiBaseURL();
  if (!baseURL) {
    throw new Error(zhCN.shell.statusOffline);
  }
  const [metadataResponse, healthResponse] = await Promise.all([
    fetch(`${baseURL}/api/toolchain`, { headers: { Accept: 'application/json' }, signal: AbortSignal.timeout(REQUEST_TIMEOUT_MS) }),
    fetch(`${baseURL}/api/toolchain/health`, { headers: { Accept: 'application/json' }, signal: AbortSignal.timeout(REQUEST_TIMEOUT_MS) }),
  ]);
  if (!metadataResponse.ok) {
    throw new Error(`${zhCN.settings.actions.loadToolchainFailed}: ${metadataResponse.status}`);
  }
  if (!healthResponse.ok) {
    throw new Error(`${zhCN.settings.actions.loadToolchainFailed}: ${healthResponse.status}`);
  }
  const metadata = mapRequiredToolchainResponse(
    await metadataResponse.json() as ToolchainResponse,
    zhCN.settings.actions.loadToolchainFailed,
  );
  return mergeToolchainHealth(
    metadata.packs,
    await healthResponse.json() as ToolchainHealthResponse,
    zhCN.settings.actions.loadToolchainFailed,
  );
}

export async function loadToolchainActions(): Promise<ToolAction[]> {
  const baseURL = await apiBaseURL();
  if (!baseURL) {
    throw new Error(zhCN.shell.statusOffline);
  }
  const response = await fetch(`${baseURL}/api/toolchain/actions`, {
    headers: { Accept: 'application/json' },
    signal: AbortSignal.timeout(REQUEST_TIMEOUT_MS),
  });
  if (!response.ok) {
    throw new Error(`${zhCN.settings.actions.loadToolchainActionsFailed}: ${response.status}`);
  }
  const payload = await response.json() as ToolchainActionsResponse;
  return mapToolActions(payload.actions);
}

export async function saveToolchainPack(pack: ToolchainPackSettings): Promise<ToolchainPackSettings> {
  const baseURL = await apiBaseURL();
  if (!baseURL) {
    throw new Error(zhCN.shell.statusOffline);
  }
  const response = await fetch(`${baseURL}/api/toolchain/packs/${encodeURIComponent(pack.id)}`, {
    method: 'PUT',
    headers: { Accept: 'application/json', 'Content-Type': 'application/json' },
    signal: AbortSignal.timeout(REQUEST_TIMEOUT_MS),
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

export async function runToolchainAction(actionID: string, request: ToolActionRunRequest): Promise<{ action: ToolAction; execution: ExecutionStep }> {
  const baseURL = await apiBaseURL();
  if (!baseURL) {
    throw new Error(zhCN.shell.statusOffline);
  }
  const body = actionID.startsWith('catalog.')
    ? {
        target: request.target,
        targetKind: request.targetKind,
        args: request.args,
      }
    : { arguments: request.arguments };
  const response = await fetch(`${baseURL}/api/toolchain/actions/${encodeURIComponent(actionID)}/run`, {
    method: 'POST',
    headers: { Accept: 'application/json', 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
    signal: AbortSignal.timeout(REQUEST_TIMEOUT_MS),
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

export async function saveWorkspaceSettings(settings: WorkspaceSettings): Promise<WorkspaceSettings> {
  const baseURL = await apiBaseURL();
  if (!baseURL) {
    throw new Error(zhCN.shell.statusOffline);
  }
  const response = await fetch(`${baseURL}/api/workspace/settings`, {
    method: 'PUT',
    headers: { Accept: 'application/json', 'Content-Type': 'application/json' },
    signal: AbortSignal.timeout(REQUEST_TIMEOUT_MS),
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

export async function fetchHarnessState(): Promise<HarnessState> {
  const baseURL = await apiBaseURL();
  if (!baseURL) {
    throw new Error(zhCN.shell.statusOffline);
  }
  return mapHarnessState(await requestJSON<HarnessStateResponse>(baseURL, '/api/harness'));
}

export async function createHarnessCase(testCase: HarnessCaseCreateRequest): Promise<HarnessCase> {
  const baseURL = await apiBaseURL();
  if (!baseURL) {
    throw new Error(zhCN.shell.statusOffline);
  }
  const response = await fetch(`${baseURL}/api/harness/cases`, {
    method: 'POST',
    headers: { Accept: 'application/json', 'Content-Type': 'application/json' },
    body: JSON.stringify(testCase),
    signal: AbortSignal.timeout(REQUEST_TIMEOUT_MS),
  });
  if (!response.ok) {
    throw new Error(await responseErrorMessage(response, zhCN.harness.createFailed));
  }
  const mapped = mapHarnessCase(await response.json() as HarnessCaseResponse);
  if (!mapped) {
    throw new Error(`${zhCN.harness.createFailed}: ${zhCN.harness.caseMissing}`);
  }
  return mapped;
}

export async function runHarnessCase(id: string): Promise<HarnessResult> {
  const baseURL = await apiBaseURL();
  if (!baseURL) {
    throw new Error(zhCN.shell.statusOffline);
  }
  const response = await fetch(`${baseURL}/api/harness/cases/${encodeURIComponent(id)}/run?wait=1&timeoutMs=5000`, {
    method: 'POST',
    headers: { Accept: 'application/json' },
    signal: AbortSignal.timeout(REQUEST_TIMEOUT_MS),
  });
  if (!response.ok) {
    throw new Error(await responseErrorMessage(response, zhCN.harness.runFailed));
  }
  const payload = await response.json() as HarnessRunResponseResponse;
  const mapped = mapHarnessResult(payload.result);
  if (!mapped) {
    throw new Error(`${zhCN.harness.runFailed}: ${zhCN.harness.resultMissing}`);
  }
  return mapped;
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
    signal: AbortSignal.timeout(REQUEST_TIMEOUT_MS),
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
    signal: AbortSignal.timeout(REQUEST_TIMEOUT_MS),
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
  // Browser API access is a development-only harness. Packaged builds must
  // use the Electron preload bridge so a missing desktop runtime is visible.
  if (import.meta.env.DEV) {
    return (import.meta.env.VITE_OURO_API_BASE_URL as string | undefined)?.trim() ?? '';
  }
  return '';
}

function emptyDesktopStatus(): DesktopStatus {
  return {
    status: 'unavailable',
    apiBaseURL: '',
    workspace: '',
    running: false,
    lastError: '',
    recentWorkspaces: [],
    runtime: {},
  };
}

function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : String(error);
}

function stringField(value: unknown): string | undefined {
  return typeof value === 'string' ? value : undefined;
}

function eventField(value: unknown): string | undefined {
  if (typeof value === 'string') {
    return value;
  }
  if (isRecord(value)) {
    try {
      return JSON.stringify(value);
    } catch {
      return undefined;
    }
  }
  return undefined;
}

function exitCodeField(value: unknown): number | null | undefined {
  if (typeof value === 'number') {
    return value >= 0 ? value : null;
  }
  if (value === null) {
    return null;
  }
  return undefined;
}

function mapDesktopStatus(backend: unknown, recentWorkspaces: unknown = [], windowState: unknown = {}, runtime: unknown = {}): DesktopStatus {
  const source = isRecord(backend) ? backend : {};
  const state = isRecord(windowState) && isRecord(windowState.windowState)
    ? windowState.windowState
    : isRecord(windowState)
      ? windowState
      : {};
  const runtimeSource = isRecord(runtime) && Object.keys(runtime).length > 0
    ? runtime
    : source;
  const runtimeInfo = mapDesktopRuntimeInfo(runtimeSource);
  const status = normalizeDesktopBackendStatus(source, runtimeInfo);
  return {
    status,
    apiBaseURL: typeof source.apiBaseURL === 'string' ? source.apiBaseURL : '',
    workspace: typeof source.workspace === 'string' ? source.workspace : '',
    running: typeof source.running === 'boolean' ? source.running : false,
    lastError: typeof source.lastError === 'string' ? source.lastError : '',
    lastEvent: eventField(source.lastEvent),
    recentWorkspaces: Array.isArray(recentWorkspaces)
      ? recentWorkspaces.filter(item => typeof item === 'string') as string[]
      : [],
    runtime: runtimeInfo,
    windowState: {
      x: typeof state.x === 'number' ? state.x : undefined,
      y: typeof state.y === 'number' ? state.y : undefined,
      width: typeof state.width === 'number' ? state.width : undefined,
      height: typeof state.height === 'number' ? state.height : undefined,
      maximized: typeof state.maximized === 'boolean' ? state.maximized : undefined,
    },
  };
}

function normalizeDesktopBackendStatus(source: Record<string, unknown>, runtime: DesktopRuntimeInfo): string {
  const status = typeof source.status === 'string' ? source.status : 'offline';
  if (status !== 'crashed') {
    return status;
  }
  const lastError = typeof source.lastError === 'string' ? source.lastError.trim() : '';
  if (lastError || runtime.lastHealthError || runtime.lastStderr) {
    return status;
  }
  if (isCleanDesktopBackendExit(runtime.lastExitCode, runtime.lastExitSignal)) {
    return 'offline';
  }
  return status;
}

function isCleanDesktopBackendExit(code?: number | null, signal?: string): boolean {
  return code === 0 || signal === 'SIGTERM' || signal === 'SIGINT';
}

function mapDesktopRuntimeInfo(source: Record<string, unknown>): DesktopRuntimeInfo {
  return {
    isPackaged: typeof source.isPackaged === 'boolean' ? source.isPackaged : undefined,
    platform: stringField(source.platform),
    arch: stringField(source.arch),
    appVersion: stringField(source.appVersion),
    appPath: stringField(source.appPath),
    executablePath: stringField(source.executablePath),
    resourcesPath: stringField(source.resourcesPath),
    userDataPath: stringField(source.userDataPath),
    backendBinaryPath: stringField(source.backendBinaryPath),
    backendBinaryExists: typeof source.backendBinaryExists === 'boolean' ? source.backendBinaryExists : undefined,
    backendWorkingDirectory: stringField(source.backendWorkingDirectory),
    backendWorkingDirectoryExists: typeof source.backendWorkingDirectoryExists === 'boolean' ? source.backendWorkingDirectoryExists : undefined,
    rendererPath: stringField(source.rendererPath),
    rendererExists: typeof source.rendererExists === 'boolean' ? source.rendererExists : undefined,
    preloadPath: stringField(source.preloadPath),
    preloadExists: typeof source.preloadExists === 'boolean' ? source.preloadExists : undefined,
    diagnosticLogPath: stringField(source.diagnosticLogPath),
    diagnosticLogExists: typeof source.diagnosticLogExists === 'boolean' ? source.diagnosticLogExists : undefined,
    desktopRuntimeStatePath: stringField(source.desktopRuntimeStatePath),
    desktopRuntimeSessionId: stringField(source.desktopRuntimeSessionId),
    desktopRuntimeStartedAt: stringField(source.desktopRuntimeStartedAt),
    lastDesktopRuntimeIssue: mapDesktopRuntimeIssue(source.lastDesktopRuntimeIssue),
    processId: typeof source.processId === 'number' ? source.processId : undefined,
    lastCommandLine: stringField(source.lastCommandLine),
    lastExitCode: exitCodeField(source.lastExitCode) ?? exitCodeField(source.exitCode),
    lastExitSignal: stringField(source.lastExitSignal) ?? stringField(source.signal),
    lastStartedAt: stringField(source.lastStartedAt),
    lastHealthError: stringField(source.lastHealthError),
    lastStdout: stringField(source.lastStdout) ?? stringField(source.stdout),
    lastStderr: stringField(source.lastStderr) ?? stringField(source.stderr),
    lastLog: stringField(source.lastLog) ?? stringField(source.log),
    lastEvent: eventField(source.lastEvent),
  };
}

function mapDesktopRuntimeIssue(value: unknown) {
  if (!isRecord(value)) {
    return undefined;
  }
  return {
    event: stringField(value.event),
    recordedAt: stringField(value.recordedAt),
    reason: stringField(value.reason),
    type: stringField(value.type),
    serviceName: stringField(value.serviceName),
    name: stringField(value.name),
    message: stringField(value.message),
    exitCode: exitCodeField(value.exitCode),
    errorCode: exitCodeField(value.errorCode),
    errorDescription: stringField(value.errorDescription),
    validatedURL: stringField(value.validatedURL),
    isMainFrame: typeof value.isMainFrame === 'boolean' ? value.isMainFrame : value.isMainFrame === null ? null : undefined,
    windowId: typeof value.windowId === 'number' ? value.windowId : undefined,
    processId: typeof value.processId === 'number' ? value.processId : undefined,
    previousSessionId: stringField(value.previousSessionId),
    previousProcessId: typeof value.previousProcessId === 'number' ? value.previousProcessId : undefined,
    previousStartedAt: stringField(value.previousStartedAt),
    previousHeartbeatAt: stringField(value.previousHeartbeatAt),
    previousWorkspace: stringField(value.previousWorkspace),
    previousStatus: stringField(value.previousStatus),
    statePath: stringField(value.statePath),
  };
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}

function isFiniteNumber(value: unknown): value is number {
  return typeof value === 'number' && Number.isFinite(value);
}

function isOptionalString(value: unknown): value is string | undefined {
  return value === undefined || typeof value === 'string';
}

function isOptionalPositiveInteger(value: unknown): value is number | undefined {
  return value === undefined || (typeof value === 'number' && Number.isInteger(value) && value > 0);
}

function mapStringList(items: unknown): string[] | undefined {
  if (!Array.isArray(items)) {
    return undefined;
  }
  return items.filter((item): item is string => typeof item === 'string' && item.trim() !== '');
}

function mapHarnessFileDiffs(items: unknown): HarnessFileDiff[] {
  if (!Array.isArray(items)) {
    return [];
  }
  return items.flatMap(item => {
    if (!isRecord(item)) {
      return [];
    }
    const diff: HarnessFileDiff = {
      path: typeof item.path === 'string' ? item.path : undefined,
      operation: typeof item.operation === 'string' ? item.operation : undefined,
      summary: typeof item.summary === 'string' ? item.summary : undefined,
      diff: typeof item.diff === 'string' ? item.diff : undefined,
      actionId: typeof item.actionId === 'string' ? item.actionId : undefined,
      messageId: typeof item.messageId === 'string' ? item.messageId : undefined,
      toolCallId: typeof item.toolCallId === 'string' ? item.toolCallId : undefined,
    };
    if (!diff.path && !diff.operation && !diff.summary && !diff.diff && !diff.actionId && !diff.messageId && !diff.toolCallId) {
      return [];
    }
    return [diff];
  });
}

function mapHarnessEventlogReferences(items: unknown): HarnessEventlogReference[] {
  if (!Array.isArray(items)) {
    return [];
  }
  return items.flatMap(item => {
    if (!isRecord(item)) {
      return [];
    }
    const reference: HarnessEventlogReference = {
      id: typeof item.id === 'string' ? item.id : undefined,
      rawEventId: typeof item.rawEventId === 'string' || isFiniteNumber(item.rawEventId) ? item.rawEventId : undefined,
      replayEventId: typeof item.replayEventId === 'string' ? item.replayEventId : undefined,
      eventType: typeof item.eventType === 'string' ? item.eventType : undefined,
      lane: typeof item.lane === 'string' ? item.lane : undefined,
      summary: typeof item.summary === 'string' ? item.summary : undefined,
    };
    if (!reference.id && reference.rawEventId === undefined && !reference.replayEventId && !reference.eventType && !reference.lane && !reference.summary) {
      return [];
    }
    return [reference];
  });
}

function mapHarnessProviderSnapshot(snapshot: HarnessProviderSnapshotResponse | undefined): HarnessProviderSnapshot | undefined {
  if (!snapshot || !isRecord(snapshot)) {
    return undefined;
  }
  const active = mapHarnessProviderSummary(snapshot.active);
  const providers = Array.isArray(snapshot.providers)
    ? snapshot.providers.flatMap(item => {
      const mapped = mapHarnessProviderSummary(item);
      return mapped ? [mapped] : [];
    })
    : [];
  if (!snapshot.configPath && !snapshot.routingStrategy && !snapshot.status && !snapshot.error && !active && providers.length === 0) {
    return undefined;
  }
  return {
    configPath: stringField(snapshot.configPath),
    routingStrategy: stringField(snapshot.routingStrategy),
    status: translateOptionalBackendStatus(snapshot.status),
    error: stringField(snapshot.error),
    active,
    providers,
  };
}

function mapHarnessProviderSummary(summary: unknown): HarnessProviderSummary | undefined {
  if (!summary || !isRecord(summary)) {
    return undefined;
  }
  const capabilities = mapStringList(summary.capabilities) ?? [];
  if (!summary.name && !summary.type && !summary.model && !summary.baseUrl && !summary.apiKeyEnv && capabilities.length === 0) {
    return undefined;
  }
  return {
    name: stringField(summary.name),
    type: stringField(summary.type),
    model: stringField(summary.model),
    baseUrl: stringField(summary.baseUrl),
    apiKeyEnv: stringField(summary.apiKeyEnv),
    enabled: typeof summary.enabled === 'boolean' ? summary.enabled : undefined,
    timeoutMs: isFiniteNumber(summary.timeoutMs) ? summary.timeoutMs : undefined,
    contextWindowTokens: isFiniteNumber(summary.contextWindowTokens) ? summary.contextWindowTokens : undefined,
    capabilities,
    status: translateOptionalBackendStatus(summary.status),
  };
}

function mapHarnessToolchainSnapshot(snapshot: HarnessToolchainSnapshotResponse | undefined): HarnessToolchainSnapshot | undefined {
  if (!snapshot || !isRecord(snapshot)) {
    return undefined;
  }
  const packs = Array.isArray(snapshot.packs)
    ? snapshot.packs.flatMap(item => {
      const mapped = mapHarnessToolchainPackSnapshot(item);
      return mapped ? [mapped] : [];
    })
    : [];
  const actions = Array.isArray(snapshot.actions)
    ? snapshot.actions.flatMap(item => {
      const mapped = mapHarnessToolchainActionSnapshot(item);
      return mapped ? [mapped] : [];
    })
    : [];
  if (!snapshot.status && !snapshot.error && packs.length === 0 && actions.length === 0) {
    return undefined;
  }
  return {
    status: translateOptionalBackendStatus(snapshot.status),
    error: stringField(snapshot.error),
    toolsReady: isFiniteNumber(snapshot.toolsReady) ? snapshot.toolsReady : undefined,
    packs,
    actions,
  };
}

function mapHarnessToolchainPackSnapshot(pack: HarnessToolchainPackSnapshotResponse | undefined): HarnessToolchainPackSnapshot | undefined {
  if (!pack || !isRecord(pack)) {
    return undefined;
  }
  const capabilities = mapStringList(pack.capabilities) ?? [];
  const runCommand = mapStringList(pack.runCommand) ?? [];
  const healthCommand = mapStringList(pack.healthCommand) ?? [];
  const dependencies = Array.isArray(pack.dependencies)
    ? pack.dependencies.flatMap(item => {
      const mapped = mapHarnessToolchainDependencySnapshot(item);
      return mapped ? [mapped] : [];
    })
    : [];
  if (!pack.id && !pack.name && !pack.manifestPath && capabilities.length === 0 && runCommand.length === 0) {
    return undefined;
  }
  return {
    id: stringField(pack.id),
    name: stringField(pack.name),
    version: stringField(pack.version),
    status: translateOptionalBackendStatus(pack.status),
    toolsReady: isFiniteNumber(pack.toolsReady) ? pack.toolsReady : undefined,
    manifestPath: stringField(pack.manifestPath),
    capabilities,
    runner: stringField(pack.runner),
    runCommand,
    runTimeoutMs: isFiniteNumber(pack.runTimeoutMs) ? pack.runTimeoutMs : undefined,
    parser: stringField(pack.parser),
    healthcheck: stringField(pack.healthcheck),
    healthCommand,
    dependencyCount: isFiniteNumber(pack.dependencyCount) ? pack.dependencyCount : undefined,
    dependencies,
  };
}

function mapHarnessToolchainDependencySnapshot(dependency: HarnessToolchainDependencySnapshotResponse | undefined): HarnessToolchainDependencySnapshot | undefined {
  if (!dependency || !isRecord(dependency)) {
    return undefined;
  }
  if (!dependency.name && !dependency.kind && !dependency.version && !dependency.source) {
    return undefined;
  }
  return {
    name: stringField(dependency.name),
    kind: stringField(dependency.kind),
    version: stringField(dependency.version),
    source: stringField(dependency.source),
    optional: typeof dependency.optional === 'boolean' ? dependency.optional : undefined,
    dependsOn: mapStringList(dependency.dependsOn) ?? [],
    installHint: stringField(dependency.installHint),
    installCommand: mapStringList(dependency.installCommand) ?? [],
  };
}

function mapHarnessToolchainActionSnapshot(action: HarnessToolchainActionSnapshotResponse | undefined): HarnessToolchainActionSnapshot | undefined {
  if (!action || !isRecord(action)) {
    return undefined;
  }
  const command = mapStringList(action.command) ?? [];
  const capabilities = mapStringList(action.capabilities) ?? [];
  if (!action.id && !action.packId && !action.commandText && command.length === 0) {
    return undefined;
  }
  return {
    id: stringField(action.id),
    packId: stringField(action.packId),
    packName: stringField(action.packName),
    label: stringField(action.label),
    status: translateOptionalBackendStatus(action.status),
    command,
    commandText: stringField(action.commandText),
    runTimeoutMs: isFiniteNumber(action.runTimeoutMs) ? action.runTimeoutMs : undefined,
    manifestPath: stringField(action.manifestPath),
    capabilities,
    runner: stringField(action.runner),
    parser: stringField(action.parser),
    argumentHint: stringField(action.argumentHint),
  };
}

function mapBootstrapData(response: GUIBootstrapResponse, latency: string): IDEData {
  const toolchain = mapBootstrapToolchain(response.toolchain);
  const graphNodes = mapGraphNodes(response.graph);
  const graphEdges = mapGraphEdges(response.graph);
  const replayEvents = mapBootstrapReplayEvents(response.replayEvents);
  const executionSteps = mapExecutionSteps(response.executions);
  const evidenceItems = mapBootstrapEvidenceItems(response.evidenceItems);
  const reportFindings = mapBootstrapReportFindings(response.reportFindings);
  const reports = mapBootstrapReports(response.reports);
  const workspaceSettings = mapWorkspaceSettings(response.workspace);
  const toolActions = mapToolActions(response.toolActions);
  const workspace = workspaceSettings.name;
  const providerConfig = mapProviderConfig(response.providerConfig, response.provider);
  const provider = providerConfig.active.name ? providerConfig.active : mapProviderSettings(response.provider);
  const providerUsage = mapProviderUsageState(response.providerUsage);
  const toolchainPacks = mapToolchainPacks(response.toolchain?.packs);
  const blackboard = mapBlackboard(response.blackboard);
  const agentState = mapAgentState(response.agentState);
  const frontierQueue = mapFrontierQueueResponse(response.frontierQueue ?? response.frontier_queue ?? {});
  const harness = mapHarnessState(response.harness);
  const orchestratorState = mapOrchestratorState(response.orchestrator);
  const plannerState = mapPlannerState(response.planner);

  return {
    workspace: workspaceSettings,
    mission: mapMissionResponse(response.mission),
    provider,
    providerConfig,
    providerUsage,
    toolchainPacks,
    toolActions,
    workspaceItems: mapWorkspaceItems(response.files),
    graphNodes,
    graphEdges,
    executionSteps,
    replayEvents,
    evidenceItems,
    reportFindings,
    reports,
    blackboard,
    agentState,
    frontierQueue,
    harness,
    orchestratorState,
    plannerState,
    aiConversation: mapBootstrapAIConversation(response.conversation),
    status: mapStatusBar(response, workspace, provider, toolchain, latency),
    loadedAt: new Date().toISOString(),
  };
}

function mapBootstrapAIConversation(response: AIConversationResponse | undefined): AIConversation | null {
  if (!response) {
    return null;
  }
  return mapAIConversationResponse(response, zhCN.ai.conversationMissing);
}

function emptyPlannerState(): PlannerState {
  return {
    path: '',
    plans: [],
    counts: {},
  };
}

function emptyHarnessState(): HarnessState {
  return {
    path: '',
    resultPath: '',
    cases: [],
    results: [],
    counts: {
      totalCases: 0,
      totalResults: 0,
      running: 0,
      passed: 0,
      failed: 0,
    },
  };
}

function mapHarnessState(response: HarnessStateResponse | undefined): HarnessState {
  if (!response?.path || !response.resultPath || !Array.isArray(response.cases) || !Array.isArray(response.results)) {
    return emptyHarnessState();
  }
  return {
    path: response.path,
    resultPath: response.resultPath,
    cases: response.cases.flatMap(testCase => {
      const mapped = mapHarnessCase(testCase);
      return mapped ? [mapped] : [];
    }),
    results: response.results.flatMap(result => {
      const mapped = mapHarnessResult(result);
      return mapped ? [mapped] : [];
    }),
    counts: mapHarnessCounts(response.counts),
  };
}

function mapHarnessCase(testCase: HarnessCaseResponse | undefined): HarnessCase | null {
  if (
    !testCase?.id
    || !testCase.title
    || !testCase.goal
    || !testCase.kind
    || !testCase.command
    || typeof testCase.expectedResult !== 'string'
    || !isFiniteNumber(testCase.createdAt)
    || !isFiniteNumber(testCase.updatedAt)
  ) {
    return null;
  }
  return {
    id: testCase.id,
    title: testCase.title,
    goal: testCase.goal,
    kind: testCase.kind,
    command: testCase.command,
    expectedResult: testCase.expectedResult,
    promptId: testCase.promptId,
    promptVersionId: testCase.promptVersionId,
    agentRole: testCase.agentRole,
    source: testCase.source,
    filePath: testCase.filePath,
    executionId: testCase.executionId,
    replayRunId: testCase.replayRunId,
    snapshotId: testCase.snapshotId,
    snapshotPath: testCase.snapshotPath,
    agentTaskId: testCase.agentTaskId,
    blackboardItemId: testCase.blackboardItemId,
    providerName: testCase.providerName,
    model: testCase.model,
    maxTokens: isFiniteNumber(testCase.maxTokens) ? testCase.maxTokens : undefined,
    providerSnapshot: mapHarnessProviderSnapshot(testCase.providerSnapshot),
    toolchainSnapshot: mapHarnessToolchainSnapshot(testCase.toolchainSnapshot),
    createdAt: testCase.createdAt,
    updatedAt: testCase.updatedAt,
  };
}

function mapHarnessResult(result: HarnessResultResponse | undefined): HarnessResult | null {
  if (
    !result?.id
    || !result.caseId
    || !result.caseTitle
    || !result.status
    || !result.command
    || typeof result.expectedResult !== 'string'
    || !isFiniteNumber(result.startedAt)
  ) {
    return null;
  }
  const usage = mapAIUsage(result.usage, zhCN.ai.usageMissing) ?? mapAIUsage(result, zhCN.ai.usageMissing);
  return {
    id: result.id,
    caseId: result.caseId,
    caseTitle: result.caseTitle,
    status: result.status,
    error: result.error,
    command: result.command,
    expectedResult: result.expectedResult,
    runId: result.runId ?? result.run_id,
    executionId: result.executionId,
    startedAt: result.startedAt,
    completedAt: result.completedAt,
    exitCode: isFiniteNumber(result.exitCode) ? result.exitCode : undefined,
    durationMs: isFiniteNumber(result.durationMs) ? result.durationMs : undefined,
    logBytes: isFiniteNumber(result.logBytes) ? result.logBytes : undefined,
    artifactCount: isFiniteNumber(result.artifactCount) ? result.artifactCount : undefined,
    artifactBytes: isFiniteNumber(result.artifactBytes) ? result.artifactBytes : undefined,
    summary: result.summary,
    logPath: result.logPath,
    artifactDir: result.artifactDir,
    promptId: result.promptId,
    promptVersionId: result.promptVersionId,
    agentRole: result.agentRole,
    filePath: result.filePath,
    replayRunId: result.replayRunId,
    snapshotId: result.snapshotId,
    snapshotPath: result.snapshotPath,
    agentTaskId: result.agentTaskId,
    blackboardItemId: result.blackboardItemId,
    aiMessageIds: mapStringList(result.aiMessageIds) ?? [],
    toolCallIds: mapStringList(result.toolCallIds) ?? [],
    fileDiffs: mapHarnessFileDiffs(result.fileDiffs),
    eventlogReferences: mapHarnessEventlogReferences(result.eventlogReferences),
    references: mapLooseAIContextReferences(result.references),
    provider: result.provider,
    model: result.model,
    inputTokens: result.inputTokens ?? result.input_tokens ?? usage?.inputTokens,
    outputTokens: result.outputTokens ?? result.output_tokens ?? usage?.outputTokens,
    totalTokens: result.totalTokens ?? result.total_tokens ?? usage?.totalTokens,
    latencyMs: result.latencyMs ?? result.latency_ms ?? usage?.latencyMs,
    costUsd: result.costUsd ?? result.cost_usd ?? usage?.costUsd,
    costKnown: result.costKnown ?? result.cost_known ?? usage?.costKnown,
    usage,
    output: result.output,
    outputSummary: result.outputSummary || result.output_summary,
    providerAttempts: mapAIProviderAttempts(result.providerAttempts ?? result.provider_attempts),
  };
}

function mapHarnessCounts(counts: HarnessCountsResponse | undefined): HarnessCounts {
  return {
    totalCases: isFiniteNumber(counts?.totalCases) ? counts.totalCases : 0,
    totalResults: isFiniteNumber(counts?.totalResults) ? counts.totalResults : 0,
    running: isFiniteNumber(counts?.running) ? counts.running : 0,
    passed: isFiniteNumber(counts?.passed) ? counts.passed : 0,
    failed: isFiniteNumber(counts?.failed) ? counts.failed : 0,
  };
}

function mapOrchestratorState(response: OrchestratorStateResponse | undefined): OrchestratorState {
  if (!response?.path || !Array.isArray(response.runs)) {
    return { path: '', observationPath: '', advancePath: '', runs: [], observations: [], advances: [], counts: {} };
  }
  return {
    path: response.path,
    observationPath: response.observationPath ?? '',
    advancePath: response.advancePath ?? '',
    runs: response.runs.flatMap(run => {
      const mapped = mapOrchestratorRun(run);
      return mapped ? [mapped] : [];
    }),
    observations: (response.observations ?? []).flatMap(observation => {
      const mapped = mapOrchestratorObservation(observation);
      return mapped ? [mapped] : [];
    }),
    advances: (response.advances ?? []).flatMap(advance => {
      const mapped = mapOrchestratorAdvance(advance);
      return mapped ? [mapped] : [];
    }),
    counts: mapOrchestratorCounts(response.counts),
  };
}

function mapOrchestratorCounts(counts: OrchestratorStateResponse['counts']): OrchestratorCounts {
  if (!counts) {
    return {};
  }
  return Object.fromEntries(
    Object.entries(counts).filter(([, value]) => isFiniteNumber(value)),
  ) as OrchestratorCounts;
}

function mapPlannerState(response: PlannerStateResponse | undefined): PlannerState {
  if (!response?.path || !Array.isArray(response.plans)) {
    return emptyPlannerState();
  }
  return {
    path: response.path,
    plans: response.plans.flatMap(plan => {
      const mapped = mapPlannerPlan(plan);
      return mapped ? [mapped] : [];
    }),
    counts: mapPlannerCounts(response.counts),
  };
}

function mapPlannerCounts(counts: PlannerStateResponse['counts']): PlannerCounts {
  if (!counts) {
    return {};
  }
  return Object.fromEntries(
    Object.entries(counts).filter(([, value]) => isFiniteNumber(value)),
  ) as PlannerCounts;
}

function mapPlannerPlan(plan: PlannerPlanResponse | undefined): PlannerPlan | null {
  if (
    !plan?.id
    || !plan.title
    || !plan.goal
    || !plan.status
    || !plan.nextActionTitle
    || !plan.nextActionGoal
    || !plan.nextActionCommand
    || !isFiniteNumber(plan.createdAt)
    || !isFiniteNumber(plan.updatedAt)
  ) {
    return null;
  }
  return {
    id: plan.id,
    title: plan.title,
    goal: plan.goal,
    status: plan.status,
    priority: plan.priority,
    source: plan.source,
    filePath: plan.filePath,
    contextSummary: plan.contextSummary,
    nextActionTitle: plan.nextActionTitle,
    nextActionGoal: plan.nextActionGoal,
    nextActionCommand: plan.nextActionCommand,
    blackboardItemId: plan.blackboardItemId,
    agentTaskId: plan.agentTaskId,
    orchestratorRunId: plan.orchestratorRunId,
    harnessCaseId: plan.harnessCaseId,
    executionId: plan.executionId,
    executionStatus: plan.executionStatus,
    executedAt: isFiniteNumber(plan.executedAt) ? plan.executedAt : undefined,
    observedAt: isFiniteNumber(plan.observedAt) ? plan.observedAt : undefined,
    observationSummary: plan.observationSummary,
    logPath: plan.logPath,
    artifactDir: plan.artifactDir,
    artifactCount: isFiniteNumber(plan.artifactCount) ? plan.artifactCount : undefined,
    exitCode: isFiniteNumber(plan.exitCode) ? plan.exitCode : undefined,
    harnessResultId: plan.harnessResultId,
    createdAt: plan.createdAt,
    updatedAt: plan.updatedAt,
  };
}

function mapOrchestratorRun(run: OrchestratorRunResponse | undefined): OrchestratorRun | null {
  if (
    !run?.id
    || !run.title
    || !run.goal
    || !run.strategy
    || !run.status
    || !isFiniteNumber(run.createdAt)
    || !isFiniteNumber(run.updatedAt)
  ) {
    return null;
  }
  return {
    id: run.id,
    title: run.title,
    goal: run.goal,
    strategy: run.strategy,
    status: run.status,
    command: run.command,
    expectedResult: run.expectedResult,
    priority: run.priority,
    source: run.source,
    filePath: run.filePath,
    blackboardItemId: run.blackboardItemId,
    agentTaskId: run.agentTaskId,
    executionId: run.executionId,
    harnessCaseId: run.harnessCaseId,
    createdAt: run.createdAt,
    updatedAt: run.updatedAt,
  };
}

function mapOrchestratorObservation(observation: OrchestratorObservationResponse | undefined): OrchestratorObservation | null {
  if (
    !observation?.id
    || !observation.runId
    || !observation.executionId
    || !observation.status
    || !isFiniteNumber(observation.artifactCount)
    || !isFiniteNumber(observation.createdAt)
    || !isFiniteNumber(observation.updatedAt)
  ) {
    return null;
  }
  return {
    id: observation.id,
    runId: observation.runId,
    executionId: observation.executionId,
    status: observation.status,
    exitCode: isFiniteNumber(observation.exitCode) ? observation.exitCode : undefined,
    logPath: observation.logPath,
    artifactDir: observation.artifactDir,
    artifactCount: observation.artifactCount,
    logPreview: observation.logPreview,
    summary: observation.summary,
    blackboardItemId: observation.blackboardItemId,
    agentTaskId: observation.agentTaskId,
    harnessResultId: observation.harnessResultId,
    autoParsedEvidenceItems: mapAIActionEvidenceItems(observation.autoParsedEvidenceItems),
    autoParsedReportFindings: mapAIActionReportFindings(observation.autoParsedReportFindings),
    autoParsedEvidenceItemIds: mapStringList(observation.autoParsedEvidenceItemIds),
    autoParsedReportFindingIds: mapStringList(observation.autoParsedReportFindingIds),
    autoParsedEvidenceCount: isFiniteNumber(observation.autoParsedEvidenceCount) ? observation.autoParsedEvidenceCount : undefined,
    autoParsedFindingCount: isFiniteNumber(observation.autoParsedFindingCount) ? observation.autoParsedFindingCount : undefined,
    parsedSource: observation.parsedSource,
    parsedBatchTitle: observation.parsedBatchTitle,
    createdAt: observation.createdAt,
    updatedAt: observation.updatedAt,
  };
}

function mapOrchestratorAdvance(advance: OrchestratorAdvanceResponse | undefined): OrchestratorAdvance | null {
  if (
    !advance?.id
    || !advance.runId
    || !advance.executionId
    || !advance.title
    || !advance.goal
    || !advance.strategy
    || !advance.status
    || !isFiniteNumber(advance.createdAt)
    || !isFiniteNumber(advance.updatedAt)
  ) {
    return null;
  }
  return {
    id: advance.id,
    runId: advance.runId,
    observationId: advance.observationId,
    parentExecutionId: advance.parentExecutionId,
    executionId: advance.executionId,
    title: advance.title,
    goal: advance.goal,
    strategy: advance.strategy,
    status: advance.status,
    command: advance.command,
    expectedResult: advance.expectedResult,
    priority: advance.priority,
    source: advance.source,
    filePath: advance.filePath,
    blackboardItemId: advance.blackboardItemId,
    agentTaskId: advance.agentTaskId,
    harnessCaseId: advance.harnessCaseId,
    autoParsedEvidenceItems: mapAIActionEvidenceItems(advance.autoParsedEvidenceItems),
    autoParsedReportFindings: mapAIActionReportFindings(advance.autoParsedReportFindings),
    autoParsedEvidenceItemIds: mapStringList(advance.autoParsedEvidenceItemIds),
    autoParsedReportFindingIds: mapStringList(advance.autoParsedReportFindingIds),
    autoParsedEvidenceCount: isFiniteNumber(advance.autoParsedEvidenceCount) ? advance.autoParsedEvidenceCount : undefined,
    autoParsedFindingCount: isFiniteNumber(advance.autoParsedFindingCount) ? advance.autoParsedFindingCount : undefined,
    createdAt: advance.createdAt,
    updatedAt: advance.updatedAt,
  };
}

function mapAgentState(response: AgentStateResponse | undefined): AgentState {
  if (!response) {
    return { path: '', eventPath: '', profilePath: '', profiles: [], tasks: [], events: [], counts: {} };
  }
  return {
    path: response.path ?? '',
    eventPath: response.eventPath ?? response.event_path ?? '',
    profilePath: response.profilePath ?? response.profile_path ?? '',
    profiles: mapAgentProfilesPayload(response.profiles ?? [], zhCN.agents.profileLoadFailed),
    tasks: (response.tasks ?? []).flatMap(task => {
      const mapped = mapAgentTask(task);
      return mapped ? [mapped] : [];
    }),
    events: (response.events ?? []).flatMap(event => {
      const mapped = mapAgentTaskEvent(event);
      return mapped ? [mapped] : [];
    }),
    counts: mapAgentCounts(response.counts),
  };
}

function mapAgentProfile(profile: AgentProfileResponse | undefined): AgentProfile | null {
  if (!profile) {
    return null;
  }
  const createdAt = profile?.createdAt ?? profile?.created_at;
  const updatedAt = profile?.updatedAt ?? profile?.updated_at;
  const capabilities = profile.capabilities;
  if (
    typeof profile.id !== 'string'
    || profile.id.trim() === ''
    || typeof profile.name !== 'string'
    || profile.name.trim() === ''
    || typeof profile.role !== 'string'
    || profile.role.trim() === ''
    || !Array.isArray(capabilities)
    || !capabilities.every(capability => typeof capability === 'string')
    || !isOptionalString(profile.description)
    || !isOptionalString(profile.instructions)
    || !isOptionalString(profile.providerName)
    || !isOptionalString(profile.provider_name)
    || !isOptionalString(profile.model)
    || !isOptionalString(profile.promptId)
    || !isOptionalString(profile.prompt_id)
    || !isOptionalString(profile.promptVersionId)
    || !isOptionalString(profile.prompt_version_id)
    || !isOptionalPositiveInteger(profile.createdAt)
    || !isOptionalPositiveInteger(profile.created_at)
    || !isOptionalPositiveInteger(profile.updatedAt)
    || !isOptionalPositiveInteger(profile.updated_at)
    || typeof createdAt !== 'number'
    || typeof updatedAt !== 'number'
    || updatedAt < createdAt
  ) {
    return null;
  }
  return {
    id: profile.id,
    name: profile.name,
    role: profile.role,
    description: typeof profile.description === 'string' ? profile.description : '',
    instructions: typeof profile.instructions === 'string' ? profile.instructions : '',
    capabilities,
    providerName: typeof (profile.providerName ?? profile.provider_name) === 'string' ? (profile.providerName ?? profile.provider_name) as string : '',
    model: typeof profile.model === 'string' ? profile.model : '',
    promptId: typeof (profile.promptId ?? profile.prompt_id) === 'string' ? (profile.promptId ?? profile.prompt_id) as string : '',
    promptVersionId: typeof (profile.promptVersionId ?? profile.prompt_version_id) === 'string' ? (profile.promptVersionId ?? profile.prompt_version_id) as string : '',
    createdAt,
    updatedAt,
  };
}

function mapAgentProfilesPayload(payload: AgentProfileResponse[] | AgentProfilesResponse, context: string): AgentProfile[] {
  const profiles = Array.isArray(payload) ? payload : payload.profiles;
  if (!Array.isArray(profiles)) {
    throw new AgentProfileContractError(context);
  }
  return profiles.map(profile => {
    const mapped = mapAgentProfile(profile);
    if (!mapped) {
      throw new AgentProfileContractError(`${context}: ${zhCN.agents.profileMissing}`);
    }
    return mapped;
  });
}

function mapAgentCounts(counts: AgentStateResponse['counts']): AgentCounts {
  if (!counts) {
    return {};
  }
  return Object.fromEntries(
    Object.entries(counts).filter(([, value]) => isFiniteNumber(value)),
  ) as AgentCounts;
}

function mapAgentTask(task: AgentTaskResponse | undefined): AgentTask | null {
  if (
    !task?.id
    || !task.title
    || !task.goal
    || !task.role
    || !task.status
    || !isFiniteNumber(task.createdAt)
    || !isFiniteNumber(task.updatedAt)
  ) {
    return null;
  }
  return {
    id: task.id,
    title: task.title,
    goal: task.goal,
    role: task.role,
    agentId: task.agentId,
    status: task.status,
    priority: task.priority,
    source: task.source,
    createdAt: task.createdAt,
    updatedAt: task.updatedAt,
    blackboardItemId: task.blackboardItemId,
    filePath: task.filePath,
    resultSummary: task.resultSummary,
    resultDetail: task.resultDetail,
    artifactPath: task.artifactPath,
    handoffSummary: task.handoffSummary,
    handoffTarget: task.handoffTarget,
    latestEventId: task.latestEventId,
    latestResultEventId: task.latestResultEventId,
    latestHandoffEventId: task.latestHandoffEventId,
    runId: task.runId,
    providerName: task.providerName,
    model: task.model,
    promptId: task.promptId,
    promptVersionId: task.promptVersionId,
    inputTokens: finiteNumberOr(task.inputTokens),
    outputTokens: finiteNumberOr(task.outputTokens),
    totalTokens: finiteNumberOr(task.totalTokens),
    durationMs: finiteNumberOr(task.durationMs),
    estimatedCostUsd: finiteNumberOr(task.estimatedCostUsd),
    estimatedCostAvailable: typeof task.estimatedCostAvailable === 'boolean' ? task.estimatedCostAvailable : undefined,
    error: task.error,
    startedAt: task.startedAt,
    completedAt: task.completedAt,
    executionId: task.executionId,
    replayEventId: task.replayEventId,
    reportFindingId: task.reportFindingId,
  };
}

function mapAgentTaskEvent(event: AgentTaskEventResponse | undefined): AgentTaskEvent | null {
  if (
    !event?.id
    || !event.taskId
    || !event.kind
    || !isFiniteNumber(event.createdAt)
  ) {
    return null;
  }
  return {
    id: event.id,
    taskId: event.taskId,
    kind: event.kind,
    status: event.status,
    agentId: event.agentId,
    handoffTarget: event.handoffTarget,
    title: event.title,
    summary: event.summary,
    detail: event.detail,
    source: event.source,
    filePath: event.filePath,
    artifactPath: event.artifactPath,
    runId: event.runId,
    providerName: event.providerName,
    model: event.model,
    promptId: event.promptId,
    promptVersionId: event.promptVersionId,
    inputTokens: finiteNumberOr(event.inputTokens),
    outputTokens: finiteNumberOr(event.outputTokens),
    totalTokens: finiteNumberOr(event.totalTokens),
    durationMs: finiteNumberOr(event.durationMs),
    estimatedCostUsd: finiteNumberOr(event.estimatedCostUsd),
    estimatedCostAvailable: typeof event.estimatedCostAvailable === 'boolean' ? event.estimatedCostAvailable : undefined,
    error: event.error,
    executionId: event.executionId,
    replayEventId: event.replayEventId,
    reportFindingId: event.reportFindingId,
    createdAt: event.createdAt,
  };
}

function mapAgentProviderRun(response: AgentProviderRunResponse | undefined): AgentProviderRun | null {
  const runId = cleanOptionalString(response?.runId ?? response?.run_id);
  const status = cleanOptionalString(response?.status);
  if (!response || !runId || !status) {
    return null;
  }
  const durationMs = response.durationMs ?? response.duration_ms;
  const inputTokens = response.inputTokens ?? response.input_tokens;
  const outputTokens = response.outputTokens ?? response.output_tokens;
  const totalTokens = response.totalTokens ?? response.total_tokens;
  const estimatedCostUsd = response.estimatedCostUsd ?? response.estimated_cost_usd;
  const agentProfileCapabilitiesResponse = response.agentProfileCapabilities ?? response.agent_profile_capabilities;
  const agentProfileCapabilities = mapStringList(agentProfileCapabilitiesResponse);
  for (const value of [durationMs, inputTokens, outputTokens, totalTokens, estimatedCostUsd]) {
    if (value !== undefined && !isFiniteNumber(value)) {
      throw new Error(zhCN.agents.providerRunMissing);
    }
  }
  const estimatedCostAvailable = response.estimatedCostAvailable ?? response.estimated_cost_available ?? false;
  if (typeof estimatedCostAvailable !== 'boolean') {
    throw new Error(zhCN.agents.providerRunMissing);
  }
  if (agentProfileCapabilitiesResponse !== undefined && !agentProfileCapabilities) {
    throw new Error(zhCN.agents.providerRunMissing);
  }
  return {
    runId,
    status,
    agentProfileId: cleanOptionalString(response.agentProfileId ?? response.agent_profile_id),
    agentProfileName: cleanOptionalString(response.agentProfileName ?? response.agent_profile_name),
    agentProfileRole: cleanOptionalString(response.agentProfileRole ?? response.agent_profile_role),
    agentProfileDescription: cleanOptionalString(response.agentProfileDescription ?? response.agent_profile_description),
    agentProfileInstructions: cleanOptionalString(response.agentProfileInstructions ?? response.agent_profile_instructions),
    agentProfileCapabilities,
    error: typeof response.error === 'string' ? response.error : undefined,
    output: cleanOptionalString(response.output),
    startedAt: cleanOptionalString(response.startedAt ?? response.started_at),
    completedAt: cleanOptionalString(response.completedAt ?? response.completed_at),
    durationMs,
    providerName: cleanOptionalString(response.providerName ?? response.provider_name),
    model: cleanOptionalString(response.model),
    promptId: cleanOptionalString(response.promptId ?? response.prompt_id),
    promptVersionId: cleanOptionalString(response.promptVersionId ?? response.prompt_version_id),
    inputTokens,
    outputTokens,
    totalTokens,
    estimatedCostUsd,
    estimatedCostAvailable,
    artifactPath: cleanOptionalString(response.artifactPath ?? response.artifact_path),
    eventlogRunId: cleanOptionalString(response.eventlogRunId ?? response.eventlog_run_id),
    task: mapAgentTask(response.task) ?? undefined,
    event: mapAgentTaskEvent(response.event) ?? undefined,
    startedEvent: mapAgentTaskEvent(response.startedEvent ?? response.started_event) ?? undefined,
    blackboardItem: mapBlackboardItem(response.blackboardItem ?? response.blackboard_item) ?? undefined,
    providerAttempts: mapAIProviderAttempts(response.providerAttempts ?? response.provider_attempts),
  };
}

function mapAgentProviderBatchRun(response: AgentProviderBatchRunResponse | undefined): AgentProviderBatchRun | null {
  const batchId = cleanOptionalString(response?.batchId ?? response?.batch_id);
  const status = cleanOptionalString(response?.status);
  const artifactPath = cleanOptionalString(response?.artifactPath ?? response?.artifact_path);
  const eventlogRunId = cleanOptionalString(response?.eventlogRunId ?? response?.eventlog_run_id);
  if (!response || !batchId || !status || !artifactPath || !eventlogRunId || !Array.isArray(response.runs)) {
    return null;
  }
  const taskIDs = response.taskIds ?? response.task_ids;
  const requestedCount = response.requestedCount ?? response.requested_count;
  const completedCount = response.completedCount ?? response.completed_count;
  const failedCount = response.failedCount ?? response.failed_count;
  const durationMs = response.durationMs ?? response.duration_ms;
  if (
    !Array.isArray(taskIDs)
    || taskIDs.some(taskID => !cleanOptionalString(taskID))
    || !isNonNegativeInteger(requestedCount)
    || !isNonNegativeInteger(completedCount)
    || !isNonNegativeInteger(failedCount)
    || !isNonNegativeInteger(durationMs)
  ) {
    throw new Error(zhCN.agents.providerBatchRunMissing);
  }
  const runs = response.runs.map(run => {
    const mapped = mapAgentProviderRun(run);
    if (!mapped) {
      throw new Error(zhCN.agents.providerBatchRunMissing);
    }
    return mapped;
  });
  return {
    batchId,
    status,
    error: typeof response.error === 'string' ? response.error : undefined,
    taskIds: taskIDs,
    requestedCount,
    completedCount,
    failedCount,
    startedAt: cleanOptionalString(response.startedAt ?? response.started_at),
    completedAt: cleanOptionalString(response.completedAt ?? response.completed_at),
    durationMs,
    artifactPath,
    eventlogRunId,
    runs,
  };
}

function mapBlackboard(response: BlackboardResponse | undefined): BlackboardState {
  if (!response?.path || !Array.isArray(response.items)) {
    return { path: '', items: [], observations: [] };
  }
  return {
    path: response.path,
    items: response.items.flatMap(item => {
      const mapped = mapBlackboardItem(item);
      return mapped ? [mapped] : [];
    }),
    observations: (response.observations ?? []).flatMap(observation => {
      const mapped = mapBlackboardObservation(observation);
      return mapped ? [mapped] : [];
    }),
  };
}

function mapBlackboardObservation(observation: BlackboardObservationResponse | undefined): BlackboardObservation | null {
  if (!observation?.id || !observation.kind || typeof observation.target !== 'string' || !isFiniteNumber(observation.createdAt)) {
    return null;
  }
  return {
    id: observation.id,
    kind: observation.kind,
    target: observation.target,
    payload: observation.payload,
    source: observation.source,
    createdAt: observation.createdAt,
  };
}

function blackboardObservationQueryString(query: BlackboardObservationQuery): string {
  const params = new URLSearchParams();
  const entries: Array<[keyof BlackboardObservationQuery, string | number | undefined]> = [
    ['kind', query.kind],
    ['target', query.target],
    ['source', query.source],
    ['q', query.q],
    ['limit', query.limit],
  ];
  entries.forEach(([key, value]) => {
    if (key === 'limit') {
      if (typeof value === 'number' && Number.isFinite(value) && value >= 0) {
        params.set(key, String(value));
      }
      return;
    }
    const normalized = value?.toString().trim();
    if (normalized) {
      params.set(key, normalized);
    }
  });
  const queryString = params.toString();
  return queryString ? `?${queryString}` : '';
}

function mapBlackboardBatchUpdateResult(response: BlackboardBatchUpdateResponse | undefined): BlackboardBatchUpdateResult {
  const blackboard = mapBlackboard(response);
  return {
    ...blackboard,
    count: isFiniteNumber(response?.count) ? response.count : blackboard.items.length,
  };
}

function mapBlackboardDependencyLinkResult(response: BlackboardDependencyLinkResponse | undefined): BlackboardDependencyLinkResult | null {
  const item = mapBlackboardItem(response?.item);
  if (!item) {
    return null;
  }
  return {
    path: response?.path || '',
    count: isFiniteNumber(response?.count) ? response.count : 0,
    item,
    dependencies: (response?.dependencies || []).flatMap(dependency => {
      const mapped = mapBlackboardItem(dependency);
      return mapped ? [mapped] : [];
    }),
  };
}

function mapBlackboardItem(item: BlackboardItemResponse | undefined): BlackboardItem | null {
  if (
    !item?.id
    || !item.kind
    || !item.title
    || typeof item.content !== 'string'
    || !item.status
    || !isFiniteNumber(item.createdAt)
    || !isFiniteNumber(item.updatedAt)
  ) {
    return null;
  }
  return {
    id: item.id,
    kind: item.kind,
    title: item.title,
    content: item.content,
    status: item.status,
    priority: item.priority,
    source: item.source,
    dependencyIds: mapStringList(item.dependencyIds),
    dependentIds: mapStringList(item.dependentIds),
    filePath: item.filePath,
    executionId: item.executionId,
    graphNodeId: item.graphNodeId,
    replayEventId: item.replayEventId,
    reportId: item.reportId,
    createdAt: item.createdAt,
    updatedAt: item.updatedAt,
  };
}

function blackboardQueryString(query: BlackboardQuery): string {
  const params = new URLSearchParams();
  const entries: Array<[keyof BlackboardQuery, string | undefined]> = [
    ['status', query.status],
    ['priority', query.priority],
    ['kind', query.kind],
    ['source', query.source],
    ['q', query.q],
  ];
  entries.forEach(([key, value]) => {
    const normalized = value?.trim();
    if (normalized) {
      params.set(key, normalized);
    }
  });
  const queryString = params.toString();
  return queryString ? `?${queryString}` : '';
}

function mapStatusBar(
  response: GUIBootstrapResponse,
  workspace: string,
  provider: ProviderSettings,
  toolchain: ToolchainHealthResponse,
  latency: string,
): StatusSummary {
  const backendStatus = response.backend?.status;
  const apiStatus = mapBackendAPIStatus(backendStatus);
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

function mapBackendAPIStatus(backendStatus?: string): ApiStatus {
  switch ((backendStatus ?? '').trim().toLowerCase()) {
    case 'ready':
      return 'connected';
    case 'offline':
    case 'crashed':
      return 'offline';
    case 'degraded':
    case 'disabled':
    case 'idle':
    case 'incomplete':
    case 'invalid':
    case 'starting':
    case 'missing':
    case 'update_available':
    default:
      return 'degraded';
  }
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

function mapRequiredToolchainResponse(response: ToolchainResponse | undefined, context: string): { status: string; toolsReady: number; packs: ToolchainPackSettings[] } {
  if (!response || !Array.isArray(response.packs)) {
    throw new Error(`${context}: ${zhCN.settings.actions.loadToolchainFailed}`);
  }
  return {
    status: translateBackendStatus(response.status),
    toolsReady: isFiniteNumber(response.toolsReady) ? response.toolsReady as number : 0,
    packs: mapToolchainPacks(response.packs),
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
    source: action.source || '',
    kind: action.kind || '',
    toolId: action.toolId || '',
    category: action.category || '',
    adapter: action.adapter || '',
    operation: action.operation || '',
    binary: action.binary || '',
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
    targetKind: action.targetKind || '',
    targetKinds: action.targetKinds ?? [],
    dependencyName: action.dependencyName || '',
    dependencyKind: action.dependencyKind || '',
    dependencyStatus: action.dependencyStatus || '',
    dependencyReady: action.dependencyReady === true,
    dependencyVersion: action.dependencyVersion || '',
    dependencyMessage: action.dependencyMessage || '',
    dependencySource: action.dependencySource || '',
    argumentHint: action.argumentHint || '',
  };
}

function mapRequiredToolActionResponse(action: ToolActionResponse | undefined, context: string): ToolAction {
  if (!action || !isCompleteToolActionResponse(action)) {
    throw new Error(`${context}: ${zhCN.toolLaunch.failed}`);
  }
  return mapToolAction(action);
}

function isCompleteToolActionResponse(action: ToolActionResponse): boolean {
  const completeBase = Boolean(
    action.id &&
    action.packId &&
    action.label &&
    action.status &&
    Array.isArray(action.command),
  );
  if (!completeBase) {
    return false;
  }
  if (action.source === 'catalog' || action.id?.startsWith('catalog.')) {
    return Boolean(
      action.toolId &&
      action.operation &&
      action.targetKind &&
      Array.isArray(action.targetKinds),
    );
  }
  return Boolean(action.command?.length && action.commandText);
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
  const mapped = {
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
    dependencies: mapToolchainDependencies(pack.dependencies),
  };
  return {
    ...mapped,
    manifestYaml: buildToolchainManifestYaml(mapped),
  };
}

function mapToolchainDependencies(dependencies?: ToolchainDependencyResponse[]): ToolchainDependency[] {
  return (dependencies ?? []).flatMap(dependency => {
    if (!dependency.name) {
      return [];
    }
    return [{
      name: dependency.name,
      kind: dependency.kind || '',
      required: dependency.required === true,
      status: translateBackendStatus(dependency.status),
      ready: dependency.ready === true,
      version: dependency.version || '',
      installedVersion: dependency.installedVersion || '',
      latestVersion: dependency.latestVersion || '',
      source: dependency.source || '',
      imageDigest: dependency.imageDigest || '',
      latestImageDigest: dependency.latestImageDigest || '',
      metadataSource: dependency.metadataSource || '',
      optional: dependency.optional === true,
      dependsOn: dependency.dependsOn ?? [],
      message: dependency.message || '',
      installHint: dependency.installHint || '',
      installCommand: dependency.installCommand ?? [],
    }];
  });
}

function mergeToolchainHealth(
  packs: ToolchainPackSettings[],
  health: ToolchainHealthResponse | undefined,
  context: string,
): ToolchainPackSettings[] {
  if (!health?.status || !isFiniteNumber(health.toolsReady) || !isRecord(health.packs)) {
    throw new Error(context);
  }
  return packs.map(pack => {
    const checked = health.packs?.[pack.id];
    if (!checked?.status || !isFiniteNumber(checked.toolsReady) || !Array.isArray(checked.dependencies)) {
      throw new Error(`${context}: ${pack.id}`);
    }
    return {
      ...pack,
      status: translateBackendStatus(checked.status),
      toolsReady: checked.toolsReady,
      dependencies: mapToolchainDependencies(checked.dependencies),
    };
  });
}

export function buildToolchainManifestYaml(pack: Pick<ToolchainPackSettings, 'name' | 'version' | 'capabilities' | 'runner' | 'runCommand' | 'runTimeoutMs' | 'parser' | 'healthcheck' | 'healthCommand'>): string {
  const lines = [
    `name: ${yamlScalar(pack.name)}`,
    `version: ${yamlScalar(pack.version)}`,
    yamlList('capabilities', pack.capabilities),
    `runner: ${yamlScalar(pack.runner)}`,
    yamlList('run_command', pack.runCommand),
    `run_timeout_ms: ${Number.isFinite(pack.runTimeoutMs) ? pack.runTimeoutMs : 0}`,
    `parser: ${yamlScalar(pack.parser)}`,
    `healthcheck: ${yamlScalar(pack.healthcheck)}`,
    yamlList('health_command', pack.healthCommand),
  ].filter(Boolean);
  return `${lines.join('\n')}\n`;
}

function yamlScalar(value: string): string {
  const text = value.trim();
  if (text === '') {
    return "''";
  }
  if (/^[A-Za-z0-9_.:/@+-]+$/.test(text)) {
    return text;
  }
  return JSON.stringify(text);
}

function yamlList(key: string, values: string[]): string {
  const normalized = values.map(value => value.trim()).filter(Boolean);
  if (normalized.length === 0) {
    return `${key}: []`;
  }
  return `${key}:\n${normalized.map(value => `  - ${yamlScalar(value)}`).join('\n')}`;
}

function mapBootstrapReplayEvents(replayEvents: GUIBootstrapResponse['replayEvents']): ReplayEvent[] {
  return (replayEvents ?? []).flatMap(event => {
    if (!event.id || !event.type || !event.lane || !isFiniteNumber(event.createdAt)) {
      return [];
    }
    const eventLabel = translateReplayEventType(event.type);
    return [{
      id: event.id,
      rawEventId: event.rawEventId,
      runId: event.runId,
      type: event.type,
      lane: event.lane,
      createdAt: event.createdAt,
      time: formatTime(event.createdAt),
      event: eventLabel,
      detail: event.detail || '',
      payloadJson: event.payloadJson || '',
      nodeId: event.nodeId,
      executionId: event.executionId,
      filePath: event.filePath,
    }];
  });
}

function mapEventlogEventsResponse(response: EventlogEventsResponse): EventlogEvents {
  const events = (response.events ?? []).flatMap(mapEventlogReplayEvent);
  return {
    runs: mapEventlogRuns(response.runs),
    events,
    count: isFiniteNumber(response.count) ? response.count : events.length,
    runId: response.runId || undefined,
    type: response.type || undefined,
    lane: response.lane || undefined,
    limit: isFiniteNumber(response.limit) ? response.limit : undefined,
  };
}

function mapEventlogRuns(runs: EventlogEventsResponse['runs']): EventlogRun[] {
  return (runs ?? []).flatMap(run => {
    if (!run.id) {
      return [];
    }
    const startedAt = isFiniteNumber(run.startedAt) ? run.startedAt : undefined;
    return [{
      id: run.id,
      status: run.status || '',
      startedAt,
      time: startedAt !== undefined ? formatTime(startedAt) : '',
    }];
  });
}

function mapEventlogReplayEvent(event: EventlogEventResponse): ReplayEvent[] {
  if (!event.id || !event.type || !event.lane || !isFiniteNumber(event.createdAt)) {
    return [];
  }
  const eventLabel = translateReplayEventType(event.type);
  return [{
    id: event.id,
    rawEventId: event.rawEventId,
    runId: event.runId,
    type: event.type,
    lane: event.lane,
    createdAt: event.createdAt,
    time: formatTime(event.createdAt),
    event: eventLabel,
    detail: event.detail || '',
    payloadJson: event.payloadJson || '',
    nodeId: event.nodeId,
    executionId: event.executionId,
    filePath: event.filePath,
  }];
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
      rawSeverity: unknownSeverityValue(finding.severity),
      endpoint: finding.endpoint || '',
      description: finding.description,
      recommendation: finding.recommendation,
      evidence: finding.evidence,
      path: finding.path,
      source: finding.source,
      nodeId: finding.nodeId || finding.graphNodeId,
      filePath: finding.filePath,
      runId: finding.runId,
      executionId: finding.executionId,
      replayEventId: finding.replayEventId,
      agentTaskId: cleanOptionalString(finding.agentTaskId ?? finding.agent_task_id),
      blackboardItemId: cleanOptionalString(finding.blackboardItemId ?? finding.blackboard_item_id),
      promptId: cleanOptionalString(finding.promptId ?? finding.prompt_id),
      promptVersionId: cleanOptionalString(finding.promptVersionId ?? finding.prompt_version_id),
      aiMessageIds: mapStringList(finding.aiMessageIds ?? finding.ai_message_ids) ?? [],
      toolCallIds: mapStringList(finding.toolCallIds ?? finding.tool_call_ids) ?? [],
      actionTraceIds: mapStringList(finding.actionTraceIds ?? finding.action_trace_ids) ?? [],
    }];
  });
}

function mapBootstrapEvidenceItems(items: GUIBootstrapResponse['evidenceItems']): EvidenceItem[] {
  return (items ?? []).flatMap(item => {
    const mapped = mapAIActionEvidenceItem(item);
    return mapped ? [mapped] : [];
  });
}

function mapBootstrapReports(reports: GUIBootstrapResponse['reports']): ReportDocument[] {
  return (reports ?? []).flatMap(report => {
    const mapped = mapReportDocument(report);
    return mapped ? [mapped] : [];
  });
}

function mapReportFileResponse(response: ReportFileResponse, requestedPath: string): ReportFileContent {
  const report = mapReportDocument(response.report);
  const file = response.file;
  if (!report || !file || !file.path || typeof file.content !== 'string') {
    throw new Error(`${requestedPath}: ${zhCN.ai.messageMissing}`);
  }
  return { report, file };
}

function mapReportDocument(report?: ReportResponse): ReportDocument | undefined {
  if (!report) {
    return undefined;
  }
  const id = report.id || '';
  const title = report.title || '';
  const status = report.status || '';
  const path = report.path || '';
  if (!id || !title || !status || !path) {
    return undefined;
  }
  return {
    id,
    title,
    status: translateBackendStatus(status),
    path,
    size: isFiniteNumber(report.size) ? report.size : undefined,
    modifiedAt: isFiniteNumber(report.modifiedAt) ? report.modifiedAt : undefined,
  };
}

function providerStatus(provider?: Pick<ProviderSettings, 'model' | 'status'>): string {
  if (!provider) {
    return zhCN.shell.statusOffline;
  }
  return provider.model ? `${provider.status} · ${provider.model}` : provider.status;
}

function mapPromptRegistryResponse(response: PromptRegistryResponse | undefined): PromptRegistry {
  if (!response || !isFiniteNumber(response.version) || !response.path || !Array.isArray(response.prompts)) {
    throw new Error(zhCN.settings.promptRegistryMissing);
  }
  return {
    version: response.version,
    path: response.path,
    prompts: response.prompts.map(prompt => mapPromptDefinitionResponse(prompt)),
  };
}

function mapPromptDefinitionResponse(prompt: PromptDefinitionResponse): PromptDefinition {
  if (!prompt.id || !prompt.title || !prompt.kind || !prompt.activeVersion || !Array.isArray(prompt.versions)) {
    throw new Error(zhCN.settings.promptRegistryMissing);
  }
  return {
    id: prompt.id,
    title: prompt.title,
    kind: prompt.kind,
    activeVersion: prompt.activeVersion,
    versions: prompt.versions.map(version => mapPromptVersionResponse(version)),
  };
}

function mapPromptVersionResponse(version: PromptVersionResponse): PromptVersion {
  if (!version.id || !version.title || typeof version.content !== 'string') {
    throw new Error(zhCN.settings.promptRegistryMissing);
  }
  return {
    id: version.id,
    title: version.title,
    content: version.content,
    providerName: cleanOptionalString(version.providerName ?? version.provider_name),
    model: cleanOptionalString(version.model),
    variables: Array.isArray(version.variables)
      ? version.variables.map(variable => mapPromptVariableResponse(variable)).filter((variable): variable is PromptVariable => Boolean(variable))
      : [],
  };
}

function mapPromptVariableResponse(variable: PromptVariableResponse): PromptVariable | undefined {
  const name = variable.name?.trim();
  if (!name) {
    return undefined;
  }
  return {
    name,
    description: variable.description || '',
    required: Boolean(variable.required),
    default: variable.default || '',
    source: variable.source || '',
  };
}

async function requestJSON<T>(baseURL: string, path: string): Promise<T> {
  const response = await fetch(`${baseURL}${path}`, {
    headers: { Accept: 'application/json' },
    cache: 'no-store',
    signal: AbortSignal.timeout(REQUEST_TIMEOUT_MS),
  });
  if (!response.ok) {
    throw new Error(`${path}: ${response.status}`);
  }
  return response.json() as Promise<T>;
}

async function responseErrorMessage(response: Response, fallback: string): Promise<string> {
  try {
    const payload = await response.json() as { error?: unknown; message?: unknown; detail?: unknown };
    const backendMessage = [payload.error, payload.message, payload.detail].find(value => typeof value === 'string' && value.trim() !== '');
    return backendMessage ? `${fallback}: ${backendMessage}` : `${fallback}: ${response.status}`;
  } catch {
    return `${fallback}: ${response.status}`;
  }
}

function mapAIConversationResponse(response: AIConversationResponse | undefined, context: string): AIConversation {
  if (!response) {
    throw new Error(context);
  }
  const messages = Array.isArray(response.messages)
    ? response.messages.map(message => mapRequiredAIMessage(message, zhCN.ai.messageMissing))
    : [];
  const contextReferences = Array.isArray(response.contextReferences ?? response.context_references)
    ? mapAIContextReferences(response.contextReferences ?? response.context_references ?? [], zhCN.ai.contextMissing)
    : [];
  return {
    messages,
    contextReferences,
    actionTrace: mapAIActionTrace(response.actionTrace ?? response.action_trace),
    provider: response.provider,
    model: response.model,
    updatedAt: normalizeAIInstantValue(response.updatedAt ?? response.updated_at),
    usage: mapAIUsage(response.usage, zhCN.ai.usageMissing),
    routing: mapAIRoutingMetadata(response),
    runtime: mapAIConversationRuntime(response.runtime),
  };
}

function mapLegacyChatConversationResponse(response: LegacyChatConversationResponse): AIConversation {
  const messages = Array.isArray(response.messages)
    ? response.messages.map(message => mapRequiredAIMessage(message, zhCN.ai.messageMissing))
    : [];
  return {
    messages,
    contextReferences: [],
    actionTrace: [],
    updatedAt: messages.at(-1)?.createdAt,
  };
}

function mapLegacyChatPostResponse(response: LegacyChatPostResponse): AIChatResponse {
  const message = mapRequiredAIMessage(response.message, zhCN.ai.messageMissing);
  const conversation: AIConversation = {
    messages: [message],
    contextReferences: [],
    actionTrace: [],
    updatedAt: message.createdAt,
  };
  return {
    conversation,
    message,
    contextReferences: [],
    actionResults: [],
  };
}

function mapAIConversationSessions(sessions: AIConversationSessionSummaryResponse[] | undefined): AIConversationSessionSummary[] {
  return Array.isArray(sessions) ? sessions.map(mapAIConversationSessionSummary) : [];
}

function mapAIConversationSessionSummary(response: AIConversationSessionSummaryResponse | undefined): AIConversationSessionSummary {
  const id = cleanOptionalString(response?.id);
  if (!id) {
    throw new Error(zhCN.ai.sessionMissing);
  }
  return {
    id,
    path: cleanOptionalString(response?.path),
    current: Boolean(response?.current),
    messageCount: finiteNumberOr(response?.messageCount ?? response?.message_count, 0),
    userCount: finiteNumberOr(response?.userCount ?? response?.user_count, 0),
    assistantCount: finiteNumberOr(response?.assistantCount ?? response?.assistant_count, 0),
    toolCount: finiteNumberOr(response?.toolCount ?? response?.tool_count, 0),
    startedAt: normalizeAIInstantValue(response?.startedAt ?? response?.started_at),
    updatedAt: normalizeAIInstantValue(response?.updatedAt ?? response?.updated_at),
    latestUserGoal: cleanOptionalString(response?.latestUserGoal ?? response?.latest_user_goal),
    latestAssistantSummary: cleanOptionalString(response?.latestAssistantSummary ?? response?.latest_assistant_summary),
    provider: cleanOptionalString(response?.provider),
    model: cleanOptionalString(response?.model),
    totalTokens: finiteNumberOr(response?.totalTokens ?? response?.total_tokens),
    costUsd: finiteNumberOr(response?.costUsd ?? response?.cost_usd),
    costKnown: response?.costKnown ?? response?.cost_known,
  };
}

function mapAIConversationSessionOperationResponse(payload: AIConversationSessionOperationResponsePayload): AIConversationSessionOperationResponse {
  return {
    session: mapAIConversationSessionSummary(payload.session),
    sessions: mapAIConversationSessions(payload.sessions),
    conversation: mapAIConversationResponse(payload.conversation, zhCN.ai.conversationMissing),
  };
}

function mapAIConversationSearchResponse(payload: AIConversationSearchResponsePayload, query: string): AIConversationSearchResponse {
  const results = Array.isArray(payload.results)
    ? payload.results.map(mapAIConversationSearchResult).filter((result): result is AIConversationSearchResult => result !== null)
    : [];
  return {
    query: cleanOptionalString(payload.query) ?? query,
    results,
    count: finiteNumberOr(payload.count, results.length),
  };
}

function mapAIConversationSearchResult(response: AIConversationSearchResultResponse | undefined): AIConversationSearchResult | null {
  if (!response) {
    return null;
  }
  const nestedSession = typeof response.session === 'object' ? response.session : undefined;
  const nestedMessage = response.message;
  const sessionId = cleanOptionalString(response.sessionId ?? response.session_id)
    ?? cleanOptionalString(nestedSession?.id)
    ?? (typeof response.session === 'string' ? cleanOptionalString(response.session) : undefined);
  if (!sessionId) {
    return null;
  }
  const messageId = cleanOptionalString(response.messageId ?? response.message_id ?? nestedMessage?.id);
  if (!messageId) {
    return null;
  }
  const snippet = cleanOptionalString(response.contentSnippet ?? response.content_snippet ?? response.snippet ?? response.content ?? nestedMessage?.content)
    ?? zhCN.ai.searchSnippetMissing;
  const path = cleanOptionalString(response.path ?? response.messagePath ?? response.message_path ?? response.sessionPath ?? response.session_path)
    ?? cleanOptionalString(nestedSession?.path)
    ?? '';
  const role = cleanOptionalString(response.role ?? nestedMessage?.role) ?? zhCN.ai.actionTraceValueMissing;
  const contextReferences = nestedMessage?.contextReferences ?? nestedMessage?.context_references;
  return {
    id: messageId,
    sessionId,
    sessionPath: cleanOptionalString(response.sessionPath ?? response.session_path ?? response.path) ?? cleanOptionalString(nestedSession?.path),
    sessionCurrent: response.sessionCurrent ?? response.session_current ?? response.current ?? nestedSession?.current,
    messageId,
    role,
    snippet,
    contentSnippet: snippet,
    path,
    createdAt: normalizeAIInstantValue(response.createdAt ?? response.created_at ?? response.time ?? nestedMessage?.createdAt),
    provider: cleanOptionalString(response.provider ?? nestedMessage?.provider ?? nestedSession?.provider),
    model: cleanOptionalString(response.model ?? nestedMessage?.model ?? nestedSession?.model),
    contextReferenceCount: finiteNumberOr(
      response.contextReferenceCount ?? response.context_reference_count,
      Array.isArray(contextReferences) ? contextReferences.length : 0,
    ),
  };
}

function mapAIConversationExportResult(payload: AIConversationExportResponsePayload): AIConversationExportResult {
  const sessionId = cleanOptionalString(payload.sessionId ?? payload.session_id);
  const path = cleanOptionalString(payload.path ?? payload.filePath ?? payload.file_path);
  const format = cleanOptionalString(payload.format);
  const exportedAt = normalizeAIInstantValue(payload.exportedAt ?? payload.exported_at);
  if (!sessionId || !path || !format || !exportedAt) {
    throw new Error(zhCN.ai.sessionExportFailed);
  }
  return {
    sessionId,
    path,
    format,
    messageCount: finiteNumberOr(payload.messageCount ?? payload.message_count, 0),
    exportedAt,
    summary: cleanOptionalString(payload.summary),
  };
}

function mapAIConversationRuntime(runtime: AIConversationRuntimeResponse | undefined): AIConversationRuntime | undefined {
  if (!runtime) {
    return undefined;
  }
  const mapped: AIConversationRuntime = {
    id: cleanOptionalString(runtime.id),
    status: cleanOptionalString(runtime.status),
    phase: cleanOptionalString(runtime.phase),
    startedAt: normalizeAIInstantValue(runtime.startedAt ?? runtime.started_at),
    updatedAt: normalizeAIInstantValue(runtime.updatedAt ?? runtime.updated_at),
    durationSeconds: finiteNumberOr(runtime.durationSeconds ?? runtime.duration_seconds),
    messageCount: finiteNumberOr(runtime.messageCount ?? runtime.message_count),
    actionCount: finiteNumberOr(runtime.actionCount ?? runtime.action_count),
    toolResultCount: finiteNumberOr(runtime.toolResultCount ?? runtime.tool_result_count),
    modelReplyCount: finiteNumberOr(runtime.modelReplyCount ?? runtime.model_reply_count),
    finalReplyCount: finiteNumberOr(runtime.finalReplyCount ?? runtime.final_reply_count),
    providerCallCount: finiteNumberOr(runtime.providerCallCount ?? runtime.provider_call_count),
    provider: cleanOptionalString(runtime.provider),
    model: cleanOptionalString(runtime.model),
    routingStrategy: cleanOptionalString(runtime.routingStrategy ?? runtime.routing_strategy),
    lastMessageId: cleanOptionalString(runtime.lastMessageId ?? runtime.last_message_id),
    lastMessageRole: cleanOptionalString(runtime.lastMessageRole ?? runtime.last_message_role),
    lastActionId: cleanOptionalString(runtime.lastActionId ?? runtime.last_action_id),
    lastActionKind: cleanOptionalString(runtime.lastActionKind ?? runtime.last_action_kind),
    lastActionStatus: cleanOptionalString(runtime.lastActionStatus ?? runtime.last_action_status),
    lastExecutionId: cleanOptionalString(runtime.lastExecutionId ?? runtime.last_execution_id),
    totalTokens: finiteNumberOr(runtime.totalTokens ?? runtime.total_tokens),
    costKnown: runtime.costKnown ?? runtime.cost_known,
    costUsd: finiteNumberOr(runtime.costUsd ?? runtime.cost_usd),
  };
  return Object.values(mapped).some(value => value !== undefined && (typeof value !== 'string' || value.trim() !== ''))
    ? mapped
    : undefined;
}

function mapAIChatResponse(payload: AIChatResponsePayload): AIChatResponse {
  if (!Array.isArray(payload.actionResults) && Array.isArray(payload.action_results)) {
    payload = { ...payload, actionResults: payload.action_results };
  }
  if (!payload.conversation || !payload.message) {
    throw new Error(zhCN.ai.conversationMissing);
  }
  if (!payload.provider || !payload.model) {
    throw new Error(zhCN.ai.providerMissing);
  }
  const usage = mapAIUsage(payload.usage, zhCN.ai.usageMissing);
  if (!usage) {
    throw new Error(zhCN.ai.usageMissing);
  }
  const routing = mapAIRoutingMetadata(payload);
  const rawContextReferences = payload.contextReferences
    ?? payload.context_references
    ?? payload.conversation.contextReferences
    ?? payload.conversation.context_references
    ?? [];
  const contextReferences = Array.isArray(rawContextReferences)
    ? mapAIContextReferences(rawContextReferences, zhCN.ai.contextMissing)
    : [];
  const conversation = mapAIConversationResponse({
    ...payload.conversation,
    provider: payload.provider,
    model: payload.model,
    usage,
    routing: routing ?? payload.conversation.routing,
    contextReferences: payload.conversation.contextReferences ?? payload.conversation.context_references ?? contextReferences,
  }, zhCN.ai.conversationMissing);
  return {
    conversation,
    message: mapRequiredAIMessage(payload.message, zhCN.ai.messageMissing),
    provider: payload.provider,
    model: payload.model,
    usage,
    routing: mapAIRoutingMetadata(payload),
    contextReferences,
    actionResults: Array.isArray(payload.actionResults)
      ? payload.actionResults.flatMap(result => mapAIActionRunResponseForStream(result, conversation))
      : [],
  };
}

function mapAIChatStreamEvent(payload: AIChatStreamEventPayload, sseEventType: string): AIChatStreamEvent {
  const responsePayload = payload.response
    ? {
        ...payload.response,
        conversation: payload.response.conversation ?? payload.conversation,
        provider: payload.response.provider ?? payload.provider,
        model: payload.response.model ?? payload.model,
      }
    : undefined;
  const response = responsePayload ? mapAIChatResponse(responsePayload) : undefined;
  const conversation = payload.conversation
    ? mapAIConversationResponse(payload.conversation, zhCN.ai.conversationMissing)
    : response?.conversation;
  const actionResultPayload = payload.actionResult ?? payload.action_result;
  const actionResult = actionResultPayload
    ? mapAIActionRunResponseForStream(actionResultPayload, conversation ?? response?.conversation)[0]
    : undefined;
  const runtime = mapAIConversationRuntime(payload.runtime)
    ?? conversation?.runtime
    ?? response?.conversation.runtime;
  return {
    event: cleanOptionalString(payload.event) ?? cleanOptionalString(sseEventType) ?? 'message',
    phase: cleanOptionalString(payload.phase),
    status: cleanOptionalString(payload.status),
    message: cleanOptionalString(payload.message),
    index: finiteNumberOr(payload.index),
    total: finiteNumberOr(payload.total),
    batchIndex: finiteNumberOr(payload.batchIndex ?? payload.batch_index),
    tokenIndex: finiteNumberOr(payload.tokenIndex ?? payload.token_index),
    delta: cleanOptionalString(payload.delta),
    textDelta: cleanOptionalString(payload.textDelta ?? payload.text_delta),
    contentDelta: cleanOptionalString(payload.contentDelta ?? payload.content_delta),
    toolCallId: cleanOptionalString(payload.toolCallId ?? payload.tool_call_id),
    toolName: cleanOptionalString(payload.toolName ?? payload.tool_name),
    nativeToolCall: payload.nativeToolCall ?? payload.native_tool_call,
    actionId: cleanOptionalString(payload.actionId ?? payload.action_id),
    actionKind: cleanOptionalString(payload.actionKind ?? payload.action_kind),
    actionLabel: cleanOptionalString(payload.actionLabel ?? payload.action_label),
    startedAt: cleanOptionalString(payload.startedAt ?? payload.started_at),
    finishedAt: cleanOptionalString(payload.finishedAt ?? payload.finished_at),
    elapsedMs: finiteNumberOr(payload.elapsedMs ?? payload.elapsed_ms),
    resultKind: cleanOptionalString(payload.resultKind ?? payload.result_kind),
    resultSummary: cleanOptionalString(payload.resultSummary ?? payload.result_summary),
    referenceCount: finiteNumberOr(payload.referenceCount ?? payload.reference_count),
    contextReferences: Array.isArray(payload.contextReferences ?? payload.context_references)
      ? mapAIContextReferences((payload.contextReferences ?? payload.context_references) as AIContextReferenceResponse[], zhCN.ai.contextMissing)
      : [],
    conversation,
    response,
    actionResult,
    runtime,
    provider: cleanOptionalString(payload.provider) ?? response?.provider ?? conversation?.provider ?? runtime?.provider,
    model: cleanOptionalString(payload.model) ?? response?.model ?? conversation?.model ?? runtime?.model,
  };
}

function isAIChatStreamErrorEvent(event: AIChatStreamEvent): boolean {
  const eventName = event.event.toLowerCase();
  const status = event.status?.toLowerCase();
  return eventName === 'error' || status === 'error' || status === 'failed' || status === 'failure';
}

function mapAIActionsResponse(payload: AIActionsResponsePayload): AIAction[] {
  if (!Array.isArray(payload.actions)) {
    throw new Error(zhCN.aiActions.actionsMissing);
  }
  return payload.actions.map(action => mapRequiredAIActionResponse(action, zhCN.aiActions.actionsMissing));
}

function mapAIActionRunResponse(payload: AIActionRunResponsePayload): AIActionRunResponse {
  if (!payload.conversation || !payload.message) {
    throw new Error(zhCN.aiActions.runFailed);
  }
  return {
    action: mapRequiredAIActionResponse(payload.action, zhCN.aiActions.actionMissing),
    execution: payload.execution?.id ? mapRequiredExecutionResponse(payload.execution, zhCN.aiActions.executionMissing) : undefined,
    result: mapAIActionResultResponse(payload.result),
    message: mapRequiredAIMessage(payload.message, zhCN.ai.messageMissing),
    conversation: mapAIConversationResponse(payload.conversation, zhCN.ai.conversationMissing),
    frontierQueue: payload.frontierQueue || payload.frontier_queue ? mapFrontierQueueResponse(payload.frontierQueue ?? payload.frontier_queue ?? {}) : undefined,
  };
}

function mapAIActionRunResponseForStream(
  payload: AIActionRunResponsePayload,
  fallbackConversation?: AIConversation,
): AIActionRunResponse[] {
  const action = mapOptionalAIActionResponse(payload.action);
  const result = mapOptionalAIActionResultResponse(payload.result);
  if (!action || !result) {
    return [];
  }
  const message = mapOptionalAIMessage(payload.message)
    ?? streamAIActionResultMessage(payload, result);
  const conversation = payload.conversation
    ? mapAIConversationResponse(payload.conversation, zhCN.ai.conversationMissing)
    : fallbackConversation
      ?? {
        messages: message ? [message] : [],
        contextReferences: message.contextReferences ?? [],
        actionTrace: [],
      };
  const execution = mapExecutionResponse(payload.execution);
  return [{
    action,
    execution: execution ?? undefined,
    result,
    message,
    conversation,
    frontierQueue: payload.frontierQueue || payload.frontier_queue ? mapFrontierQueueResponse(payload.frontierQueue ?? payload.frontier_queue ?? {}) : undefined,
  }];
}

function mapOptionalAIActionResponse(action: AIActionResponse | undefined): AIAction | undefined {
  try {
    return mapRequiredAIActionResponse(action, zhCN.aiActions.actionMissing);
  } catch {
    return undefined;
  }
}

function mapOptionalAIActionResultResponse(result: AIActionResultResponse | undefined): AIActionRunResponse['result'] | undefined {
  if (!result?.kind) {
    return undefined;
  }
  try {
    return mapAIActionResultResponse(result);
  } catch {
    return {
      kind: normalizeAIActionResultKind(result.kind),
      operation: result.operation,
      provider: result.provider,
      model: result.model,
      output: result.output,
      outputSummary: result.outputSummary,
      count: isFiniteNumber(result.count) ? result.count : undefined,
      summary: result.summary,
    };
  }
}

function mapOptionalAIMessage(message: AIMessageResponse | undefined): AIMessage | undefined {
  try {
    return mapRequiredAIMessage(message, zhCN.ai.messageMissing);
  } catch {
    return undefined;
  }
}

function streamAIActionResultMessage(
  payload: AIActionRunResponsePayload,
  result: AIActionRunResponse['result'],
): AIMessage {
  const references = mapOptionalAIContextReferences(payload.message?.contextReferences ?? payload.message?.context_references, zhCN.ai.contextMissing) ?? [];
  const message = payload.message;
  return {
    id: cleanOptionalString(message?.id),
    role: 'tool',
    content: cleanOptionalString(message?.content)
      ?? result.summary
      ?? result.outputSummary
      ?? result.output
      ?? zhCN.ai.streamEventActionResultRecovered,
    createdAt: normalizeAIInstantValue(message?.createdAt),
    status: cleanOptionalString(message?.status),
    tokenCount: finiteNumberOr(message?.tokenCount),
    contextReferences: references,
    usage: mapAIUsage(message?.usage, zhCN.ai.usageMissing),
    routing: message ? mapAIRoutingMetadata(message) : undefined,
  };
}

function mapAIActionResultResponse(result: AIActionResultResponse | undefined): AIActionRunResponse['result'] {
  if (!result?.kind) {
    throw new Error(zhCN.aiActions.resultMissing);
  }
  const kind = normalizeAIActionResultKind(result.kind);
  const agentProviderRun = mapAgentProviderRun(result.agentProviderRun ?? result.agent_provider_run);
  if (kind === 'agent_provider_run' && !agentProviderRun) {
    throw new Error(`${zhCN.aiActions.resultMissing}: ${zhCN.agents.providerRunMissing}`);
  }
  const agentProviderBatchRun = mapAgentProviderBatchRun(result.agentProviderBatchRun ?? result.agent_provider_batch_run);
  if (kind === 'agent_provider_batch_run' && !agentProviderBatchRun) {
    throw new Error(`${zhCN.aiActions.resultMissing}: ${zhCN.agents.providerBatchRunMissing}`);
  }
  return {
    kind,
    operation: result.operation,
    path: result.path,
    targetPath: result.targetPath,
    goal: result.goal,
    query: result.query,
    file: mapAIActionResultFile(result.file),
    fileContents: mapAIActionFileContents(result.fileContents),
    files: mapAIActionFiles(result.files),
    fileChanges: mapAIActionFileChanges(result.fileChanges ?? result.file_changes),
    fileDiffs: mapHarnessFileDiffs(result.fileDiffs ?? result.file_diffs),
    deletedFile: mapAIActionDeletedFile(result.deletedFile),
    searchResults: mapAIActionSearchResults(result.searchResults),
    patch: result.patch,
    evidenceItem: mapAIActionEvidenceItem(result.evidenceItem),
    evidenceItems: mapAIActionEvidenceItems(result.evidenceItems),
    reports: mapAIActionReports(result.reports),
    reportFinding: mapAIActionReportFinding(result.reportFinding),
    reportFindings: mapAIActionReportFindings(result.reportFindings),
    executions: mapAIActionExecutions(result.executions),
    executionLogs: mapAIActionExecutionLogs(result.executionLogs),
    executionArtifacts: mapAIActionExecutionArtifacts(result.executionArtifacts),
    reportExport: mapAIActionReportExport(result.reportExport),
    blackboardItems: mapAIActionBlackboardItems(result.blackboardItems),
    blackboardItem: mapAIActionBlackboardItem(result.blackboardItem),
    agentState: result.agentState ? mapAgentState(result.agentState) : undefined,
    agentProfile: mapAgentProfile(result.agentProfile ?? result.agent_profile) ?? undefined,
    agentTasks: mapAIActionAgentTasks(result.agentTasks),
    agentTask: mapAIActionAgentTask(result.agentTask),
    agentTaskEvents: mapAIActionAgentTaskEvents(result.agentTaskEvents),
    agentTaskEvent: mapAIActionAgentTaskEvent(result.agentTaskEvent),
    agentProviderRun: agentProviderRun ?? undefined,
    agentProviderBatchRun: agentProviderBatchRun ?? undefined,
    runtimeWorkerState: mapOptionalRuntimeWorkerStateResponse(result.runtimeWorkerState ?? result.runtime_worker_state),
    runtimeWorkers: mapRuntimeWorkerResponses(result.runtimeWorkers ?? result.runtime_workers),
    runtimeWorkerOrchestrations: mapRuntimeWorkerOrchestrationResponses(result.runtimeWorkerOrchestrations ?? result.runtime_worker_orchestrations),
    runtimeWorkerOrchestration: mapOptionalRuntimeWorkerOrchestrationResponse(result.runtimeWorkerOrchestration ?? result.runtime_worker_orchestration),
    harnessState: mapAIActionHarnessState(result.harnessState),
    harnessCases: mapAIActionHarnessCases(result.harnessCases),
    harnessCase: mapAIActionHarnessCase(result.harnessCase),
    harnessResults: mapAIActionHarnessResults(result.harnessResults),
    harnessResult: mapAIActionHarnessResult(result.harnessResult),
    eventlogEvents: mapAIActionEventlogEvents(result.eventlogEvents ?? result.eventlog_events),
    aiMessageIds: mapStringList(result.aiMessageIds),
    toolCallIds: mapStringList(result.toolCallIds),
    orchestratorState: mapAIActionOrchestratorState(result.orchestratorState),
    orchestratorRuns: mapAIActionOrchestratorRuns(result.orchestratorRuns),
    orchestratorRun: mapAIActionOrchestratorRun(result.orchestratorRun),
    orchestratorObservations: mapAIActionOrchestratorObservations(result.orchestratorObservations),
    orchestratorObservation: mapAIActionOrchestratorObservation(result.orchestratorObservation),
    orchestratorAdvances: mapAIActionOrchestratorAdvances(result.orchestratorAdvances),
    orchestratorAdvance: mapAIActionOrchestratorAdvance(result.orchestratorAdvance),
    plannerState: mapAIActionPlannerState(result.plannerState),
    plannerPlans: mapAIActionPlannerPlans(result.plannerPlans),
    plannerPlan: mapAIActionPlannerPlan(result.plannerPlan),
    frontierItems: mapAIActionFrontierItems(result.frontierItems ?? result.frontier_items),
    frontierSummary: mapAIActionFrontierSummary(result.frontierSummary ?? result.frontier_summary),
    frontierDependencies: result.frontierDependencies || result.frontier_dependencies ? mapFrontierDependencySelectionResponse(result.frontierDependencies ?? result.frontier_dependencies ?? {}) : undefined,
    refreshedFrontierItems: mapAIActionFrontierItems(result.refreshedFrontierItems ?? result.refreshed_frontier_items),
    refreshedFrontierSummary: mapAIActionFrontierSummary(result.refreshedFrontierSummary ?? result.refreshed_frontier_summary),
    budget: isFiniteNumber(result.budget) ? result.budget : undefined,
    queueSource: cleanOptionalString(result.queueSource ?? result.queue_source),
    queueStatus: cleanOptionalString(result.queueStatus ?? result.queue_status),
    promptRegistry: mapAIActionPromptRegistry(result.promptRegistry ?? result.prompt_registry),
    prompt: mapAIActionPromptDefinition(result.prompt),
    promptVersion: mapAIActionPromptVersion(result.promptVersion ?? result.prompt_version),
    promptDiff: mapAIActionPromptDiff(result.promptDiff ?? result.prompt_diff),
    promptComparison: mapAIActionPromptComparison(result.promptComparison ?? result.comparison) ?? mapAIActionPromptComparison(result),
    batchComparison: mapAIActionPromptBatchComparison(result.batchComparison ?? result.batch_comparison),
    provider: result.provider,
    model: result.model,
    usage: mapAIUsage(result.usage, zhCN.ai.usageMissing),
    output: result.output,
    outputSummary: result.outputSummary,
    count: isFiniteNumber(result.count) ? result.count : undefined,
    summary: result.summary,
  };
}

function mapAIActionFileChanges(items: unknown): AIActionFileChange[] | undefined {
  if (!Array.isArray(items)) {
    return undefined;
  }
  const changes = items.flatMap(item => {
    if (!isRecord(item)) {
      return [];
    }
    const createdAt = typeof item.createdAt === 'string' || typeof item.createdAt === 'number'
      ? item.createdAt
      : typeof item.created_at === 'string' || typeof item.created_at === 'number'
        ? item.created_at
        : undefined;
    const change: AIActionFileChange = {
      path: cleanOptionalString(item.path),
      targetPath: cleanOptionalString(item.targetPath ?? item.target_path),
      operation: cleanOptionalString(item.operation),
      status: cleanOptionalString(item.status),
      summary: cleanOptionalString(item.summary),
      patch: cleanOptionalString(item.patch),
      diff: cleanOptionalString(item.diff),
      source: cleanOptionalString(item.source),
      actionId: cleanOptionalString(item.actionId ?? item.action_id),
      messageId: cleanOptionalString(item.messageId ?? item.message_id),
      toolCallId: cleanOptionalString(item.toolCallId ?? item.tool_call_id),
      createdAt: normalizeAIInstantValue(createdAt),
    };
    if (!change.path && !change.targetPath && !change.operation && !change.summary && !change.patch && !change.diff) {
      return [];
    }
    return [change];
  });
  return changes.length > 0 ? changes : undefined;
}

function normalizeAIActionResultKind(kind: string): string {
  switch (kind) {
    case 'file':
      return 'file';
    case 'file_contents':
      return 'file_contents';
    case 'file_list':
      return 'file_list';
    case 'patch':
      return 'patch';
    case 'search_results':
      return 'search_results';
    case 'deleted_file':
      return 'deleted_file';
    case 'evidence_context':
      return 'evidence_context';
    case 'report_file':
      return 'report_file';
    case 'report_finding_file':
      return 'report_finding_file';
    case 'evidence_item_file':
      return 'evidence_item_file';
    case 'tool_result_parse':
      return 'tool_result_parse';
    case 'eventlog_events':
      return 'eventlog_events';
    case 'blackboard_items':
      return 'blackboard_items';
    case 'blackboard_item':
      return 'blackboard_item';
    case 'agent':
      return 'agent';
    case 'agent_tasks':
      return 'agent_tasks';
    case 'agent_task':
      return 'agent_task';
    case 'agent_provider_run':
      return 'agent_provider_run';
    case 'agent_provider_batch_run':
      return 'agent_provider_batch_run';
    case 'harness':
      return 'harness';
    case 'harness_cases':
      return 'harness_cases';
    case 'harness_case':
      return 'harness_case';
    case 'harness_case_from_failure':
      return 'harness_case_from_failure';
    case 'harness_case_from_eventlog':
      return 'harness_case_from_eventlog';
    case 'harness_case_from_workspace_snapshot':
      return 'harness_case_from_workspace_snapshot';
    case 'harness_case_from_environment_snapshot':
      return 'harness_case_from_environment_snapshot';
    case 'harness_case_from_ai_conversation':
      return 'harness_case_from_ai_conversation';
    case 'harness_results':
      return 'harness_results';
    case 'harness_result':
      return 'harness_result';
    case 'sisyphus_conclusion':
      return 'sisyphus_conclusion';
    case 'orchestrator':
      return 'orchestrator';
    case 'orchestrator_run':
      return 'orchestrator_run';
    case 'orchestrator_observation':
      return 'orchestrator_observation';
    case 'orchestrator_advance':
      return 'orchestrator_advance';
    case 'orchestrator_loop':
      return 'orchestrator_loop';
    case 'planner':
      return 'planner';
    case 'planner_plan':
      return 'planner_plan';
    case 'planner_dispatch':
      return 'planner_dispatch';
    case 'planner_execute':
      return 'planner_execute';
    case 'planner_observation':
      return 'planner_observation';
    case 'planner_advance':
      return 'planner_advance';
    case 'planner_step':
      return 'planner_step';
    case 'frontier_priorities':
      return 'frontier_priorities';
    case 'frontier_execute_next':
      return 'frontier_execute_next';
    case 'frontier_step':
      return 'frontier_step';
    case 'frontier_loop':
      return 'frontier_loop';
    case 'frontier_autopilot':
      return 'frontier_autopilot';
    case 'prompt_registry':
      return 'prompt_registry';
    case 'prompt_active_version':
      return 'prompt_active_version';
    case 'prompt_diff':
      return 'prompt_diff';
    case 'prompt_harness_batch_compare':
      return 'prompt_harness_batch_compare';
    case 'prompt_harness_batch_comparison':
      return 'prompt_harness_batch_comparison';
    case 'prompt_harness_compare':
      return 'prompt_harness_compare';
    case 'prompt_harness_comparison':
      return 'prompt_harness_comparison';
    case 'prompt_harness_promotion':
      return 'prompt_harness_promotion';
    default:
      return kind;
  }
}

function mapAIActionResultFile(file: AIActionResultResponse['file']): WorkspaceFileContent | undefined {
  if (!file?.path || !file.name || !file.kind || typeof file.content !== 'string') {
    return undefined;
  }
  return {
    path: file.path,
    name: file.name,
    kind: file.kind,
    content: file.content,
    size: isFiniteNumber(file.size) ? file.size : undefined,
    modifiedAt: isFiniteNumber(file.modifiedAt) ? file.modifiedAt : undefined,
  };
}

function mapAIActionFileContents(files: AIActionResultResponse['fileContents']): WorkspaceFileContent[] | undefined {
  if (!Array.isArray(files)) {
    return undefined;
  }
  const mapped = files.flatMap(file => {
    const mappedFile = mapAIActionResultFile(file);
    return mappedFile ? [mappedFile] : [];
  });
  return mapped.length > 0 ? mapped : undefined;
}

function mapAIActionDeletedFile(file: AIActionResultResponse['deletedFile']): WorkspaceFile | undefined {
  if (!file?.id || !file.path || !file.name || !file.kind) {
    return undefined;
  }
  return {
    id: file.id,
    path: file.path,
    name: file.name,
    kind: file.kind,
    size: isFiniteNumber(file.size) ? file.size : undefined,
    modifiedAt: isFiniteNumber(file.modifiedAt) ? file.modifiedAt : undefined,
  };
}

function mapAIActionFiles(files: AIActionResultResponse['files']): WorkspaceFile[] | undefined {
  if (!Array.isArray(files)) {
    return undefined;
  }
  const mapped = files.flatMap(file => {
    if (!file?.id || !file.path || !file.name || !file.kind) {
      return [];
    }
    return [{
      id: file.id,
      path: file.path,
      name: file.name,
      kind: file.kind,
      size: isFiniteNumber(file.size) ? file.size : undefined,
      modifiedAt: isFiniteNumber(file.modifiedAt) ? file.modifiedAt : undefined,
    }];
  });
  return mapped.length > 0 ? mapped : undefined;
}

function mapAIActionSearchResults(results: AIActionResultResponse['searchResults']): WorkspaceSearchResult[] | undefined {
  if (!Array.isArray(results)) {
    return undefined;
  }
  return results.flatMap(result => (
    result.path && isFiniteNumber(result.line) && typeof result.text === 'string'
      ? [{ path: result.path, line: result.line, text: result.text }]
      : []
  ));
}

function mapAIActionEvidenceItem(item: AIActionResultResponse['evidenceItem']): EvidenceItem | undefined {
  if (!item?.id || !item.title || !item.summary) {
    return undefined;
  }
  return {
    id: item.id,
    title: item.title,
    summary: item.summary,
    severity: normalizeSeverity(item.severity),
    rawSeverity: unknownSeverityValue(item.severity),
    source: item.source,
    path: item.path,
    filePath: item.filePath,
    artifactPath: item.artifactPath,
    executionId: item.executionId,
    graphNodeId: item.graphNodeId,
    replayEventId: item.replayEventId,
    agentTaskId: cleanOptionalString(item.agentTaskId ?? item.agent_task_id),
    blackboardItemId: cleanOptionalString(item.blackboardItemId ?? item.blackboard_item_id),
    promptId: cleanOptionalString(item.promptId ?? item.prompt_id),
    promptVersionId: cleanOptionalString(item.promptVersionId ?? item.prompt_version_id),
    aiMessageIds: mapStringList(item.aiMessageIds ?? item.ai_message_ids) ?? [],
    toolCallIds: mapStringList(item.toolCallIds ?? item.tool_call_ids) ?? [],
    actionTraceIds: mapStringList(item.actionTraceIds ?? item.action_trace_ids) ?? [],
    createdAt: isFiniteNumber(item.createdAt) ? item.createdAt : undefined,
  };
}

function mapAIActionEvidenceItems(items: AIActionResultResponse['evidenceItems']): EvidenceItem[] | undefined {
  if (!Array.isArray(items)) {
    return undefined;
  }
  return items.flatMap(item => {
    const mapped = mapAIActionEvidenceItem(item);
    return mapped ? [mapped] : [];
  });
}

function mapAIActionReports(reports: AIActionResultResponse['reports']): ReportDocument[] | undefined {
  if (!Array.isArray(reports)) {
    return undefined;
  }
  return mapBootstrapReports(reports);
}

function mapAIActionReportFindings(findings: AIActionResultResponse['reportFindings']): ReportFinding[] | undefined {
  if (!Array.isArray(findings)) {
    return undefined;
  }
  return mapBootstrapReportFindings(findings);
}

function mapAIActionReportFinding(finding: AIActionResultResponse['reportFinding']): ReportFinding | undefined {
  if (!finding) {
    return undefined;
  }
  return mapBootstrapReportFindings([finding])[0];
}

function mapAIActionExecutions(executions: AIActionResultResponse['executions']): ExecutionStep[] | undefined {
  if (!Array.isArray(executions)) {
    return undefined;
  }
  return mapExecutionSteps(executions);
}

function mapAIActionExecutionLogs(logs: AIActionResultResponse['executionLogs']): ExecutionLog[] | undefined {
  if (!Array.isArray(logs)) {
    return undefined;
  }
  return logs.flatMap(log => (
    log.id && log.path && typeof log.content === 'string'
      ? [{ id: log.id, path: log.path, content: log.content }]
      : []
  ));
}

function mapAIActionExecutionArtifacts(artifacts: AIActionResultResponse['executionArtifacts']): ExecutionArtifact[] | undefined {
  if (!Array.isArray(artifacts)) {
    return undefined;
  }
  return artifacts.flatMap(artifact => (
    isCompleteExecutionArtifactResponse(artifact) ? [mapExecutionArtifact(artifact)] : []
  ));
}

function mapAIActionReportExport(exportResult: AIActionResultResponse['reportExport']): ReportExportResult | undefined {
  if (!exportResult) {
    return undefined;
  }
  return mapReportExportResponse(exportResult);
}

function mapAIActionBlackboardItems(items: AIActionResultResponse['blackboardItems']): BlackboardItem[] | undefined {
  if (!Array.isArray(items)) {
    return undefined;
  }
  return items.flatMap(item => {
    const mapped = mapBlackboardItem(item);
    return mapped ? [mapped] : [];
  });
}

function mapAIActionBlackboardItem(item: AIActionResultResponse['blackboardItem']): BlackboardItem | undefined {
  const mapped = mapBlackboardItem(item);
  return mapped ?? undefined;
}

function mapAIActionAgentTasks(tasks: AIActionResultResponse['agentTasks']): AgentTask[] | undefined {
  if (!Array.isArray(tasks)) {
    return undefined;
  }
  return tasks.flatMap(task => {
    const mapped = mapAgentTask(task);
    return mapped ? [mapped] : [];
  });
}

function mapAIActionAgentTask(task: AIActionResultResponse['agentTask']): AgentTask | undefined {
  const mapped = mapAgentTask(task);
  return mapped ?? undefined;
}

function mapAIActionAgentTaskEvents(events: AIActionResultResponse['agentTaskEvents']): AgentTaskEvent[] | undefined {
  if (!Array.isArray(events)) {
    return undefined;
  }
  return events.flatMap(event => {
    const mapped = mapAgentTaskEvent(event);
    return mapped ? [mapped] : [];
  });
}

function mapAIActionAgentTaskEvent(event: AIActionResultResponse['agentTaskEvent']): AgentTaskEvent | undefined {
  const mapped = mapAgentTaskEvent(event);
  return mapped ?? undefined;
}

function mapRuntimeWorkerStateResponse(payload: RuntimeWorkerStateResponse): RuntimeWorkerState {
  const updatedAt = cleanOptionalString(payload.updatedAt ?? payload.updated_at);
  const staleAfterSeconds = payload.staleAfterSeconds ?? payload.stale_after_seconds;
  if (
    !Array.isArray(payload.workers)
    || !Array.isArray(payload.orchestrations)
    || !isRecord(payload.counts)
    || !updatedAt
    || !isFiniteNumber(staleAfterSeconds)
  ) {
    throw new Error(zhCN.runtimeWorkers.responseMissing);
  }
  return {
    workers: payload.workers.map(mapRuntimeWorkerResponse),
    orchestrations: payload.orchestrations.map(mapRuntimeWorkerOrchestrationResponse),
    counts: mapRuntimeWorkerCounts(payload.counts),
    updatedAt,
    staleAfterSeconds,
  };
}

function mapOptionalRuntimeWorkerStateResponse(payload?: RuntimeWorkerStateResponse): RuntimeWorkerState | undefined {
  return payload ? mapRuntimeWorkerStateResponse(payload) : undefined;
}

function mapRuntimeWorkerResponses(payload?: RuntimeWorkerResponse[]): RuntimeWorker[] | undefined {
  return Array.isArray(payload) ? payload.map(mapRuntimeWorkerResponse) : undefined;
}

function mapRuntimeWorkerResponse(payload: RuntimeWorkerResponse): RuntimeWorker {
  const workerId = cleanOptionalString(payload.workerId ?? payload.worker_id);
  const mode = cleanOptionalString(payload.mode);
  const status = cleanOptionalString(payload.status);
  const registeredAt = cleanOptionalString(payload.registeredAt ?? payload.registered_at);
  const heartbeatAt = cleanOptionalString(payload.heartbeatAt ?? payload.heartbeat_at);
  const path = cleanOptionalString(payload.path);
  const labels = payload.labels ?? {};
  if (
    !workerId
    || !mode
    || !status
    || !registeredAt
    || !heartbeatAt
    || !path
    || !Array.isArray(payload.capabilities)
    || !payload.capabilities.every(capability => typeof capability === 'string')
    || !isRecord(labels)
    || !Object.values(labels).every(label => typeof label === 'string')
    || typeof payload.stale !== 'boolean'
  ) {
    throw new Error(zhCN.runtimeWorkers.workerMissing);
  }
  return {
    workerId,
    mode,
    status,
    endpoint: cleanOptionalString(payload.endpoint) ?? '',
    capabilities: payload.capabilities.map(capability => capability.trim()).filter(Boolean),
    labels: Object.fromEntries(Object.entries(labels).map(([key, value]) => [key, String(value)])),
    registeredAt,
    heartbeatAt,
    stale: payload.stale,
    path,
  };
}

function mapRuntimeWorkerOrchestrationResponses(payload?: RuntimeWorkerOrchestrationResponse[]): RuntimeWorkerOrchestration[] | undefined {
  return Array.isArray(payload) ? payload.map(mapRuntimeWorkerOrchestrationResponse) : undefined;
}

function mapOptionalRuntimeWorkerOrchestrationResponse(payload?: RuntimeWorkerOrchestrationResponse): RuntimeWorkerOrchestration | undefined {
  return payload ? mapRuntimeWorkerOrchestrationResponse(payload) : undefined;
}

function mapRuntimeWorkerOrchestrationResponse(payload: RuntimeWorkerOrchestrationResponse): RuntimeWorkerOrchestration {
  const path = cleanOptionalString(payload.path);
  const status = cleanOptionalString(payload.status);
  const mode = cleanOptionalString(payload.mode);
  const runId = cleanOptionalString(payload.runId ?? payload.run_id);
  const transportMode = cleanOptionalString(payload.transportMode ?? payload.transport_mode);
  const createdAt = cleanOptionalString(payload.createdAt ?? payload.created_at);
  const placementReady = payload.placementReady ?? payload.placement_ready;
  const placementSkipped = payload.placementSkipped ?? payload.placement_skipped;
  const claimedCount = payload.claimedCount ?? payload.claimed_count;
  const claimSkipped = payload.claimSkipped ?? payload.claim_skipped;
  const claimFailed = payload.claimFailed ?? payload.claim_failed;
  const dispatchReady = payload.dispatchReady ?? payload.dispatch_ready;
  const dispatchSkipped = payload.dispatchSkipped ?? payload.dispatch_skipped;
  const dispatchedCount = payload.dispatchedCount ?? payload.dispatched_count;
  const dispatchFailed = payload.dispatchFailed ?? payload.dispatch_failed;
  if (
    !path
    || !status
    || !mode
    || !runId
    || !transportMode
    || !createdAt
    || !isFiniteNumber(placementReady)
    || !isFiniteNumber(placementSkipped)
    || !isFiniteNumber(claimedCount)
    || !isFiniteNumber(claimSkipped)
    || !isFiniteNumber(claimFailed)
    || !isFiniteNumber(dispatchReady)
    || !isFiniteNumber(dispatchSkipped)
    || !isFiniteNumber(dispatchedCount)
    || !isFiniteNumber(dispatchFailed)
  ) {
    throw new Error(zhCN.runtimeWorkers.orchestrationMissing);
  }
  return compactRuntimeWorkerOrchestration({
    path,
    status,
    mode,
    runId,
    transportMode,
    createdAt,
    maxCandidates: finiteNumberOr(payload.maxCandidates ?? payload.max_candidates),
    placementReady,
    placementSkipped,
    placementPlanPath: cleanOptionalString(payload.placementPlanPath ?? payload.placement_plan_path),
    claimedCount,
    claimSkipped,
    claimFailed,
    claimExecutionPaths: mapStringList(payload.claimExecutionPaths ?? payload.claim_execution_paths),
    placementDispatchPlanPath: cleanOptionalString(payload.placementDispatchPlanPath ?? payload.placement_dispatch_plan_path),
    dispatchReady,
    dispatchSkipped,
    placementDispatchBatchPath: cleanOptionalString(payload.placementDispatchBatchPath ?? payload.placement_dispatch_batch_path),
    placementDispatchBatchStatus: cleanOptionalString(payload.placementDispatchBatchStatus ?? payload.placement_dispatch_batch_status),
    dispatchedCount,
    dispatchFailed,
    includeLifecycleMaintenance: optionalBoolean(payload.includeLifecycleMaintenance ?? payload.include_lifecycle_maintenance),
    executeLifecycleRetryDispatches: optionalBoolean(payload.executeLifecycleRetryDispatches ?? payload.execute_lifecycle_retry_dispatches),
    dispatchLifecycleRetryClaims: optionalBoolean(payload.dispatchLifecycleRetryClaims ?? payload.dispatch_lifecycle_retry_claims),
    lifecycleRetryTransportMode: cleanOptionalString(payload.lifecycleRetryTransportMode ?? payload.lifecycle_retry_transport_mode),
    lifecycleMaintenancePath: cleanOptionalString(payload.lifecycleMaintenancePath ?? payload.lifecycle_maintenance_path),
    lifecycleAuditPath: cleanOptionalString(payload.lifecycleAuditPath ?? payload.lifecycle_audit_path),
    retryDispatchExecutionsCreated: finiteNumberOr(payload.retryDispatchExecutionsCreated ?? payload.retry_dispatch_executions_created),
    retryDispatchClaimsCreated: finiteNumberOr(payload.retryDispatchClaimsCreated ?? payload.retry_dispatch_claims_created),
    retryDispatchClaimsDispatched: finiteNumberOr(payload.retryDispatchClaimsDispatched ?? payload.retry_dispatch_claims_dispatched),
    retryDispatchTransportRequestsCreated: finiteNumberOr(payload.retryDispatchTransportRequestsCreated ?? payload.retry_dispatch_transport_requests_created),
    retryDispatchExecutionPaths: mapStringList(payload.retryDispatchExecutionPaths ?? payload.retry_dispatch_execution_paths),
    error: cleanOptionalString(payload.error),
  });
}

function compactRuntimeWorkerOrchestration(value: RuntimeWorkerOrchestration): RuntimeWorkerOrchestration {
  return Object.fromEntries(Object.entries(value).filter(([, item]) => item !== undefined)) as unknown as RuntimeWorkerOrchestration;
}

function mapRuntimeWorkerCounts(payload: Record<string, unknown>): Record<string, number> {
  const entries = Object.entries(payload);
  if (!entries.every(([, count]) => isFiniteNumber(count))) {
    throw new Error(zhCN.runtimeWorkers.countsMissing);
  }
  return Object.fromEntries(entries) as Record<string, number>;
}

function optionalBoolean(value: unknown): boolean | undefined {
  return typeof value === 'boolean' ? value : undefined;
}

function mapAIActionHarnessState(state: AIActionResultResponse['harnessState']): HarnessState | undefined {
  if (!state) {
    return undefined;
  }
  return mapHarnessState(state);
}

function mapAIActionHarnessCases(cases: AIActionResultResponse['harnessCases']): HarnessCase[] | undefined {
  if (!Array.isArray(cases)) {
    return undefined;
  }
  return cases.flatMap(testCase => {
    const mapped = mapHarnessCase(testCase);
    return mapped ? [mapped] : [];
  });
}

function mapAIActionHarnessCase(testCase: AIActionResultResponse['harnessCase']): HarnessCase | undefined {
  const mapped = mapHarnessCase(testCase);
  return mapped ?? undefined;
}

function mapAIActionHarnessResults(results: AIActionResultResponse['harnessResults']): HarnessResult[] | undefined {
  if (!Array.isArray(results)) {
    return undefined;
  }
  return results.flatMap(result => {
    const mapped = mapHarnessResult(result);
    return mapped ? [mapped] : [];
  });
}

function mapAIActionHarnessResult(result: AIActionResultResponse['harnessResult']): HarnessResult | undefined {
  const mapped = mapHarnessResult(result);
  return mapped ?? undefined;
}

function mapAIActionEventlogEvents(events: AIActionResultResponse['eventlogEvents']): EventlogEvents | undefined {
  return events ? mapEventlogEventsResponse(events) : undefined;
}

function mapAIActionOrchestratorState(state: AIActionResultResponse['orchestratorState']): OrchestratorState | undefined {
  if (!state) {
    return undefined;
  }
  return mapOrchestratorState(state);
}

function mapAIActionOrchestratorRuns(runs: AIActionResultResponse['orchestratorRuns']): OrchestratorRun[] | undefined {
  if (!Array.isArray(runs)) {
    return undefined;
  }
  return runs.flatMap(run => {
    const mapped = mapOrchestratorRun(run);
    return mapped ? [mapped] : [];
  });
}

function mapAIActionOrchestratorRun(run: AIActionResultResponse['orchestratorRun']): OrchestratorRun | undefined {
  const mapped = mapOrchestratorRun(run);
  return mapped ?? undefined;
}

function mapAIActionOrchestratorObservations(observations: AIActionResultResponse['orchestratorObservations']): OrchestratorObservation[] | undefined {
  if (!Array.isArray(observations)) {
    return undefined;
  }
  return observations.flatMap(observation => {
    const mapped = mapOrchestratorObservation(observation);
    return mapped ? [mapped] : [];
  });
}

function mapAIActionOrchestratorObservation(observation: AIActionResultResponse['orchestratorObservation']): OrchestratorObservation | undefined {
  const mapped = mapOrchestratorObservation(observation);
  return mapped ?? undefined;
}

function mapAIActionOrchestratorAdvances(advances: AIActionResultResponse['orchestratorAdvances']): OrchestratorAdvance[] | undefined {
  if (!Array.isArray(advances)) {
    return undefined;
  }
  return advances.flatMap(advance => {
    const mapped = mapOrchestratorAdvance(advance);
    return mapped ? [mapped] : [];
  });
}

function mapAIActionOrchestratorAdvance(advance: AIActionResultResponse['orchestratorAdvance']): OrchestratorAdvance | undefined {
  const mapped = mapOrchestratorAdvance(advance);
  return mapped ?? undefined;
}

function mapAIActionPlannerState(state: AIActionResultResponse['plannerState']): PlannerState | undefined {
  if (!state) {
    return undefined;
  }
  return mapPlannerState(state);
}

function mapAIActionPlannerPlans(plans: AIActionResultResponse['plannerPlans']): PlannerPlan[] | undefined {
  if (!Array.isArray(plans)) {
    return undefined;
  }
  return plans.flatMap(plan => {
    const mapped = mapPlannerPlan(plan);
    return mapped ? [mapped] : [];
  });
}

function mapAIActionPlannerPlan(plan: AIActionResultResponse['plannerPlan']): PlannerPlan | undefined {
  const mapped = mapPlannerPlan(plan);
  return mapped ?? undefined;
}

function mapAIActionFrontierItems(items: AIActionResultResponse['frontierItems']): FrontierPriorityItem[] | undefined {
  if (!Array.isArray(items)) {
    return undefined;
  }
  return items.map((item, index) => mapAIActionFrontierItem(item, index));
}

function mapFrontierQueueResponse(response: FrontierQueueResponse): FrontierQueue {
  const items = mapAIActionFrontierItems(response.items ?? response.frontierItems ?? response.frontier_items) ?? [];
  return {
    path: cleanOptionalString(response.path),
    queueSource: cleanOptionalString(response.queueSource ?? response.queue_source),
    queueStatus: cleanOptionalString(response.queueStatus ?? response.queue_status),
    items,
    summary: mapAIActionFrontierSummary(response.summary ?? response.frontierSummary ?? response.frontier_summary),
    blackboardItems: mapAIActionBlackboardItems(response.blackboardItems ?? response.blackboard_items),
    agentTasks: mapAIActionAgentTasks(response.agentTasks ?? response.agent_tasks),
    agentState: response.agentState || response.agent_state ? mapAgentState(response.agentState ?? response.agent_state) : undefined,
    count: finiteNumberOr(response.count, items.length) ?? items.length,
  };
}

function mapFrontierMonitorResponse(response: FrontierMonitorResponse): FrontierMonitor {
  const queue = mapFrontierQueueResponse(response.queue ?? {});
  const activeAgentTasks = mapAIActionAgentTasks(response.activeAgentTasks ?? response.active_agent_tasks) ?? [];
  const activePlannerPlans = (response.activePlannerPlans ?? response.active_planner_plans ?? []).flatMap(plan => {
    const mapped = mapPlannerPlan(plan);
    return mapped ? [mapped] : [];
  });
  const recentAgentEvents = (response.recentAgentEvents ?? response.recent_agent_events ?? []).flatMap(event => {
    const mapped = mapAgentTaskEvent(event);
    return mapped ? [mapped] : [];
  });
  return {
    queue,
    summary: mapFrontierMonitorSummaryResponse(response.summary, queue, activeAgentTasks.length, activePlannerPlans.length, response.runningExecutions ?? response.running_executions, recentAgentEvents.length),
    agentState: response.agentState || response.agent_state ? mapAgentState(response.agentState ?? response.agent_state) : undefined,
    plannerState: response.plannerState || response.planner_state ? mapPlannerState(response.plannerState ?? response.planner_state) : undefined,
    activeAgentTasks,
    activePlannerPlans,
    runningExecutions: mapExecutionSteps(response.runningExecutions ?? response.running_executions),
    recentAgentEvents,
  };
}

function mapFrontierMonitorSummaryResponse(
  response: FrontierMonitorSummaryResponse | undefined,
  queue: FrontierQueue,
  activeAgentTaskCount: number,
  activePlannerPlanCount: number,
  runningExecutions: ExecutionResponse[] | undefined,
  recentEventCount: number,
): FrontierMonitorSummary {
  return {
    status: cleanOptionalString(response?.status),
    source: cleanOptionalString(response?.source),
    queueStatus: cleanOptionalString(response?.queueStatus ?? response?.queue_status),
    queueItems: finiteNumberOr(response?.queueItems ?? response?.queue_items, queue.count) ?? queue.count,
    readyItems: finiteNumberOr(response?.readyItems ?? response?.ready_items, queue.summary?.readyCount ?? 0) ?? 0,
    blockedItems: finiteNumberOr(response?.blockedItems ?? response?.blocked_items, queue.summary?.blockedCount ?? 0) ?? 0,
    activeAgentTasks: finiteNumberOr(response?.activeAgentTasks ?? response?.active_agent_tasks, activeAgentTaskCount) ?? activeAgentTaskCount,
    activePlannerPlans: finiteNumberOr(response?.activePlannerPlans ?? response?.active_planner_plans, activePlannerPlanCount) ?? activePlannerPlanCount,
    runningExecutions: finiteNumberOr(response?.runningExecutions ?? response?.running_executions, runningExecutions?.length ?? 0) ?? 0,
    recentEvents: finiteNumberOr(response?.recentEvents ?? response?.recent_events, recentEventCount) ?? recentEventCount,
    lastUpdatedAt: isFiniteNumber(response?.lastUpdatedAt ?? response?.last_updated_at) ? response?.lastUpdatedAt ?? response?.last_updated_at : undefined,
    summary: cleanOptionalString(response?.summary),
  };
}

function mapFrontierDependencySelectionResponse(response: FrontierDependencySelectionResponse): FrontierDependencySelection {
  const queue = mapFrontierQueueResponse(response.queue ?? {});
  const clusters = (response.clusters ?? []).map((cluster, index) => mapFrontierDependencyClusterResponse(cluster, index));
  const candidates = (response.candidates ?? []).map((candidate, index) => mapFrontierDependencyCandidateResponse(candidate, index));
  return {
    path: cleanOptionalString(response.path),
    queueSource: cleanOptionalString(response.queueSource ?? response.queue_source),
    status: cleanOptionalString(response.status),
    queue,
    summary: mapFrontierDependencySummaryResponse(response.summary, queue, clusters.length, candidates.length),
    clusters,
    candidates,
    blackboardItems: mapAIActionBlackboardItems(response.blackboardItems ?? response.blackboard_items),
    agentTasks: mapAIActionAgentTasks(response.agentTasks ?? response.agent_tasks),
    agentState: response.agentState || response.agent_state ? mapAgentState(response.agentState ?? response.agent_state) : undefined,
    count: finiteNumberOr(response.count, clusters.length) ?? clusters.length,
  };
}

function mapFrontierDependencySummaryResponse(
  response: FrontierDependencySummaryResponse | undefined,
  queue: FrontierQueue,
  clusterCount: number,
  candidateCount: number,
): FrontierDependencySummary {
  return {
    goal: cleanOptionalString(response?.goal),
    source: cleanOptionalString(response?.source),
    status: cleanOptionalString(response?.status),
    queueStatus: cleanOptionalString(response?.queueStatus ?? response?.queue_status),
    queueItems: finiteNumberOr(response?.queueItems ?? response?.queue_items, queue.count) ?? queue.count,
    readyItems: finiteNumberOr(response?.readyItems ?? response?.ready_items, queue.summary?.readyCount ?? 0) ?? 0,
    blockedItems: finiteNumberOr(response?.blockedItems ?? response?.blocked_items, queue.summary?.blockedCount ?? 0) ?? 0,
    clusterCount: finiteNumberOr(response?.clusterCount ?? response?.cluster_count, clusterCount) ?? clusterCount,
    candidateCount: finiteNumberOr(response?.candidateCount ?? response?.candidate_count, candidateCount) ?? candidateCount,
    selectedCount: finiteNumberOr(response?.selectedCount ?? response?.selected_count, clusterCount) ?? clusterCount,
    blockedFrontierItemCount: finiteNumberOr(response?.blockedFrontierItemCount ?? response?.blocked_frontier_item_count, 0) ?? 0,
    missingDependencyCount: finiteNumberOr(response?.missingDependencyCount ?? response?.missing_dependency_count, 0) ?? 0,
    satisfiedDependencyCount: finiteNumberOr(response?.satisfiedDependencyCount ?? response?.satisfied_dependency_count, 0) ?? 0,
    readyDependencyCount: finiteNumberOr(response?.readyDependencyCount ?? response?.ready_dependency_count, 0) ?? 0,
    linkedAgentTaskCount: finiteNumberOr(response?.linkedAgentTaskCount ?? response?.linked_agent_task_count, 0) ?? 0,
    unlockValue: finiteNumberOr(response?.unlockValue ?? response?.unlock_value, 0) ?? 0,
    maxItems: finiteNumberOr(response?.maxItems ?? response?.max_items),
    budget: finiteNumberOr(response?.budget),
    summary: cleanOptionalString(response?.summary),
  };
}

function mapFrontierDependencyCandidateResponse(response: FrontierDependencyCandidateResponse, index: number): FrontierDependencyCandidate {
  return {
    rank: finiteNumberOr(response.rank, index + 1) ?? index + 1,
    id: cleanOptionalString(response.id) ?? cleanOptionalString(response.dependencyId ?? response.dependency_id) ?? `dependency-${index + 1}`,
    dependencyId: cleanOptionalString(response.dependencyId ?? response.dependency_id) ?? '',
    dependencyTitle: cleanOptionalString(response.dependencyTitle ?? response.dependency_title),
    dependencyStatus: cleanOptionalString(response.dependencyStatus ?? response.dependency_status),
    dependencyReady: Boolean(response.dependencyReady ?? response.dependency_ready),
    dependencySatisfied: Boolean(response.dependencySatisfied ?? response.dependency_satisfied),
    missingDependency: Boolean(response.missingDependency ?? response.missing_dependency),
    unlockCount: finiteNumberOr(response.unlockCount ?? response.unlock_count, 0) ?? 0,
    unlockValue: finiteNumberOr(response.unlockValue ?? response.unlock_value, 0) ?? 0,
    score: finiteNumberOr(response.score, 0) ?? 0,
    suggestedActionId: cleanOptionalString(response.suggestedActionId ?? response.suggested_action_id),
    suggestedAction: cleanOptionalString(response.suggestedAction ?? response.suggested_action),
    suggestedBlackboardItemId: cleanOptionalString(response.suggestedBlackboardItemId ?? response.suggested_blackboard_item_id),
    suggestedAgentTaskId: cleanOptionalString(response.suggestedAgentTaskId ?? response.suggested_agent_task_id),
    suggestedGoal: cleanOptionalString(response.suggestedGoal ?? response.suggested_goal),
    reason: cleanOptionalString(response.reason),
  };
}

function mapFrontierDependencyClusterResponse(response: FrontierDependencyClusterResponse, index: number): FrontierDependencyCluster {
  const candidate = mapFrontierDependencyCandidateResponse(response, index);
  return {
    ...candidate,
    dependency: mapAIActionBlackboardItem(response.dependency),
    dependencyFound: Boolean(response.dependencyFound ?? response.dependency_found),
    blockedItems: mapAIActionFrontierItems(response.blockedItems ?? response.blocked_items) ?? [],
    blockedBlackboardItems: mapAIActionBlackboardItems(response.blockedBlackboardItems ?? response.blocked_blackboard_items) ?? [],
    linkedAgentTasks: mapAIActionAgentTasks(response.linkedAgentTasks ?? response.linked_agent_tasks) ?? [],
  };
}

function mapAIActionFrontierItem(item: FrontierPriorityItemResponse, index: number): FrontierPriorityItem {
  const knownKeys = new Set([
    'id',
    'rank',
    'score',
    'title',
    'target',
    'hypothesis',
    'expectedUnlock',
    'expected_unlock',
    'reason',
    'suggestedAction',
    'suggested_action',
    'ready',
    'dependencyIds',
    'dependency_ids',
    'blockedByIds',
    'blocked_by_ids',
    'blockedReason',
    'blocked_reason',
    'status',
    'priority',
    'source',
    'sourceKind',
    'source_kind',
    'sourceId',
    'source_id',
    'blackboardItemId',
    'blackboard_item_id',
    'agentTaskId',
    'agent_task_id',
    'filePath',
    'file_path',
    'executionId',
    'execution_id',
    'missionValue',
    'mission_value',
    'unlockPotential',
    'unlock_potential',
    'evidenceConfidence',
    'evidence_confidence',
    'novelty',
    'cost',
    'duplicateRisk',
    'duplicate_risk',
    'noise',
    'exhaustedPenalty',
    'exhausted_penalty',
  ]);
  const metadata = Object.fromEntries(Object.entries(item).filter(([key]) => !knownKeys.has(key)));
  return {
    id: cleanOptionalString(item.id),
    rank: finiteNumberOr(item.rank, index + 1),
    score: finiteNumberOr(item.score),
    title: cleanOptionalString(item.title),
    target: cleanOptionalString(item.target),
    hypothesis: cleanOptionalString(item.hypothesis),
    expectedUnlock: cleanOptionalString(item.expectedUnlock ?? item.expected_unlock),
    reason: cleanOptionalString(item.reason),
    suggestedAction: cleanOptionalString(item.suggestedAction ?? item.suggested_action),
    ready: typeof item.ready === 'boolean' ? item.ready : undefined,
    dependencyIds: mapStringList(item.dependencyIds ?? item.dependency_ids),
    blockedByIds: mapStringList(item.blockedByIds ?? item.blocked_by_ids),
    blockedReason: cleanOptionalString(item.blockedReason ?? item.blocked_reason),
    status: cleanOptionalString(item.status),
    priority: cleanOptionalString(item.priority),
    source: cleanOptionalString(item.source),
    sourceKind: cleanOptionalString(item.sourceKind ?? item.source_kind),
    sourceId: cleanOptionalString(item.sourceId ?? item.source_id),
    blackboardItemId: cleanOptionalString(item.blackboardItemId ?? item.blackboard_item_id),
    agentTaskId: cleanOptionalString(item.agentTaskId ?? item.agent_task_id),
    filePath: cleanOptionalString(item.filePath ?? item.file_path),
    executionId: cleanOptionalString(item.executionId ?? item.execution_id),
    missionValue: finiteNumberOr(item.missionValue ?? item.mission_value),
    unlockPotential: finiteNumberOr(item.unlockPotential ?? item.unlock_potential),
    evidenceConfidence: finiteNumberOr(item.evidenceConfidence ?? item.evidence_confidence),
    novelty: finiteNumberOr(item.novelty),
    cost: finiteNumberOr(item.cost),
    duplicateRisk: finiteNumberOr(item.duplicateRisk ?? item.duplicate_risk),
    noise: finiteNumberOr(item.noise),
    exhaustedPenalty: finiteNumberOr(item.exhaustedPenalty ?? item.exhausted_penalty),
    metadata: Object.keys(metadata).length > 0 ? metadata : undefined,
  };
}

function mapAIActionFrontierSummary(summary: AIActionResultResponse['frontierSummary']): FrontierPrioritySummary | undefined {
  if (!summary) {
    return undefined;
  }
  const knownKeys = new Set([
    'goal',
    'source',
    'statusFilter',
    'status_filter',
    'priorityFilter',
    'priority_filter',
    'readyFilter',
    'ready_filter',
    'candidateCount',
    'candidate_count',
    'totalCandidates',
    'total_candidates',
    'selectedCount',
    'selected_count',
    'skippedCount',
    'skipped_count',
    'readyCount',
    'ready_count',
    'blockedCount',
    'blocked_count',
    'budget',
    'budgetUsed',
    'budget_used',
    'budgetRemaining',
    'budget_remaining',
    'maxItems',
    'max_items',
    'exhaustedCount',
    'exhausted_count',
    'duplicateCount',
    'duplicate_count',
    'openBlackboard',
    'open_blackboard',
    'queuedTasks',
    'queued_tasks',
    'summary',
  ]);
  const metadata = Object.fromEntries(Object.entries(summary).filter(([key]) => !knownKeys.has(key)));
  const mapped: FrontierPrioritySummary = {
    goal: cleanOptionalString(summary.goal),
    source: cleanOptionalString(summary.source),
    statusFilter: cleanOptionalString(summary.statusFilter ?? summary.status_filter),
    priorityFilter: cleanOptionalString(summary.priorityFilter ?? summary.priority_filter),
    readyFilter: cleanOptionalString(summary.readyFilter ?? summary.ready_filter),
    candidateCount: finiteNumberOr(summary.candidateCount ?? summary.candidate_count),
    totalCandidates: finiteNumberOr(summary.totalCandidates ?? summary.total_candidates),
    selectedCount: finiteNumberOr(summary.selectedCount ?? summary.selected_count),
    skippedCount: finiteNumberOr(summary.skippedCount ?? summary.skipped_count),
    readyCount: finiteNumberOr(summary.readyCount ?? summary.ready_count),
    blockedCount: finiteNumberOr(summary.blockedCount ?? summary.blocked_count),
    budget: finiteNumberOr(summary.budget),
    budgetUsed: finiteNumberOr(summary.budgetUsed ?? summary.budget_used),
    budgetRemaining: finiteNumberOr(summary.budgetRemaining ?? summary.budget_remaining),
    maxItems: finiteNumberOr(summary.maxItems ?? summary.max_items),
    exhaustedCount: finiteNumberOr(summary.exhaustedCount ?? summary.exhausted_count),
    duplicateCount: finiteNumberOr(summary.duplicateCount ?? summary.duplicate_count),
    openBlackboard: finiteNumberOr(summary.openBlackboard ?? summary.open_blackboard),
    queuedTasks: finiteNumberOr(summary.queuedTasks ?? summary.queued_tasks),
    summary: cleanOptionalString(summary.summary),
    metadata: Object.keys(metadata).length > 0 ? metadata : undefined,
  };
  return Object.values(mapped).some(value => value !== undefined) ? mapped : undefined;
}

function mapAIActionPromptRegistry(registry: AIActionResultResponse['promptRegistry']): PromptRegistry | undefined {
  if (!registry) {
    return undefined;
  }
  return mapPromptRegistryResponse(registry);
}

function mapAIActionPromptDefinition(prompt: AIActionResultResponse['prompt']): PromptDefinition | undefined {
  if (!prompt) {
    return undefined;
  }
  return mapPromptDefinitionResponse(prompt);
}

function mapAIActionPromptVersion(version: AIActionResultResponse['promptVersion']): PromptVersion | undefined {
  if (!version) {
    return undefined;
  }
  return mapPromptVersionResponse(version);
}

function mapAIActionPromptDiff(diff: AIActionResultResponse['promptDiff']): PromptDiff | undefined {
  if (
    !diff?.promptId
    || !diff.baseVersionId
    || !diff.targetVersionId
    || typeof diff.diff !== 'string'
    || typeof diff.changed !== 'boolean'
  ) {
    return undefined;
  }
  return {
    promptId: diff.promptId,
    baseVersionId: diff.baseVersionId,
    targetVersionId: diff.targetVersionId,
    baseTitle: diff.baseTitle,
    targetTitle: diff.targetTitle,
    baseContent: diff.baseContent,
    targetContent: diff.targetContent,
    diff: diff.diff,
    addedLines: isFiniteNumber(diff.addedLines) ? diff.addedLines : 0,
    removedLines: isFiniteNumber(diff.removedLines) ? diff.removedLines : 0,
    changed: diff.changed,
  };
}

function mapAIActionPromptComparison(comparison: AIActionResultResponse['promptComparison']): PromptHarnessComparison | undefined {
  if (!comparison) {
    return undefined;
  }
  const baseResult = mapHarnessResult(comparison.baseResult ?? comparison.baseHarnessResult ?? comparison.results?.base) ?? undefined;
  const targetResult = mapHarnessResult(comparison.targetResult ?? comparison.targetHarnessResult ?? comparison.results?.target) ?? undefined;
  const baseUsage = mapAIUsage(comparison.baseUsage, zhCN.ai.usageMissing) ?? baseResult?.usage;
  const targetUsage = mapAIUsage(comparison.targetUsage, zhCN.ai.usageMissing) ?? targetResult?.usage;
  const totalTokens = isFiniteNumber(comparison.totalTokens)
    ? comparison.totalTokens
    : promptComparisonTotalTokens(baseUsage, targetUsage);
  const mapped: PromptHarnessComparison = {
    promptId: comparison.promptId || comparison.basePrompt?.id || comparison.targetPrompt?.id || baseResult?.promptId || targetResult?.promptId,
    baseVersionId: comparison.baseVersionId || comparison.baseVersion?.id || baseResult?.promptVersionId,
    targetVersionId: comparison.targetVersionId || comparison.targetVersion?.id || targetResult?.promptVersionId,
    baseProvider: comparison.baseProvider || baseResult?.provider,
    targetProvider: comparison.targetProvider || targetResult?.provider,
    baseModel: comparison.baseModel || baseResult?.model,
    targetModel: comparison.targetModel || targetResult?.model,
    baseUsage,
    targetUsage,
    totalTokens,
    baseOutput: comparison.baseOutput || baseResult?.output,
    targetOutput: comparison.targetOutput || targetResult?.output,
    baseOutputSummary: comparison.baseOutputSummary || baseResult?.outputSummary,
    targetOutputSummary: comparison.targetOutputSummary || targetResult?.outputSummary,
    baseResultId: comparison.baseResultId || comparison.baseHarnessResultId || baseResult?.id,
    targetResultId: comparison.targetResultId || comparison.targetHarnessResultId || targetResult?.id,
    baseStatus: comparison.baseStatus || comparison.baseHarnessStatus || baseResult?.status,
    targetStatus: comparison.targetStatus || comparison.targetHarnessStatus || targetResult?.status,
    baseResult,
    targetResult,
  };
  return Object.values(mapped).some(value => value !== undefined && value !== '') ? mapped : undefined;
}

function mapAIActionPromptBatchComparison(comparison: AIActionResultResponse['batchComparison']): PromptHarnessBatchComparison | undefined {
  if (!comparison) {
    return undefined;
  }
  const results = mapPromptHarnessBatchResults(comparison.results);
  const items = mapPromptHarnessBatchComparisonItems(comparison, results);
  const versions = Array.isArray(comparison.versions)
    ? comparison.versions.map(version => (typeof version === 'string' ? version : mapPromptVersionResponse(version)))
    : [];
  const versionIds = [
    ...(Array.isArray(comparison.versionIds) ? comparison.versionIds : []),
    ...(Array.isArray(comparison.version_ids) ? comparison.version_ids : []),
  ].filter(Boolean);
  return {
    promptId: comparison.promptId || comparison.prompt_id || comparison.prompt?.id || items[0]?.result?.promptId,
    prompt: comparison.prompt ? mapPromptDefinitionResponse(comparison.prompt) : undefined,
    versionIds: versionIds.length > 0 ? versionIds : items.map(item => item.versionId).filter(Boolean) as string[],
    versions: versions.length > 0
      ? versions
      : items.map(item => item.version).filter((version): version is PromptVersion | string => version !== undefined),
    results: results.length > 0 ? results : items.flatMap(item => (item.result ? [item.result] : [])),
    items,
    sameProvider: comparison.sameProvider ?? comparison.same_provider,
    sameModel: comparison.sameModel ?? comparison.same_model,
    providers: Array.isArray(comparison.providers) ? comparison.providers : undefined,
    models: Array.isArray(comparison.models) ? comparison.models : undefined,
    totalTokens: isFiniteNumber(comparison.totalTokens)
      ? comparison.totalTokens
      : isFiniteNumber(comparison.total_tokens)
        ? comparison.total_tokens
        : promptBatchComparisonTotalTokens(items),
    totalCostUsd: isFiniteNumber(comparison.totalCostUsd)
      ? comparison.totalCostUsd
      : isFiniteNumber(comparison.total_cost_usd)
        ? comparison.total_cost_usd
        : promptBatchComparisonTotalCostUsd(items),
    costKnown: comparison.costKnown ?? comparison.cost_known ?? promptBatchComparisonCostKnown(items),
    summary: comparison.summary,
  };
}

function mapPromptHarnessBatchResults(results: PromptHarnessBatchComparisonResponse['results']): HarnessResult[] {
  if (Array.isArray(results)) {
    return results.flatMap(result => {
      const mapped = mapHarnessResult(result);
      return mapped ? [mapped] : [];
    });
  }
  if (results && typeof results === 'object') {
    return Object.values(results).flatMap(result => {
      const mapped = mapHarnessResult(result);
      return mapped ? [mapped] : [];
    });
  }
  return [];
}

function mapPromptHarnessBatchComparisonItems(
  comparison: PromptHarnessBatchComparisonResponse,
  mappedResults: HarnessResult[],
): PromptHarnessBatchComparisonItem[] {
  const resultByVersion = new Map(mappedResults.map(result => [result.promptVersionId, result]));
  if (Array.isArray(comparison.items) && comparison.items.length > 0) {
    return comparison.items.map(item => {
      const version = item.version;
      const versionId = item.versionId
        || item.version_id
        || (typeof version === 'string' ? version : version?.id)
        || (typeof item.result === 'string' ? undefined : item.result?.promptVersionId);
      const result = typeof item.result === 'string'
        ? mappedResults.find(candidate => candidate.id === item.result || candidate.promptVersionId === versionId)
        : mapHarnessResult(item.result) ?? (versionId ? resultByVersion.get(versionId) : undefined);
      const usage = mapAIUsage(item.usage, zhCN.ai.usageMissing) ?? result?.usage;
      const totalTokens = isFiniteNumber(item.totalTokens)
        ? item.totalTokens
        : isFiniteNumber(item.total_tokens)
          ? item.total_tokens
          : usage?.totalTokens ?? result?.totalTokens;
      const costUsd = isFiniteNumber(item.costUsd)
        ? item.costUsd
        : isFiniteNumber(item.cost_usd)
          ? item.cost_usd
          : usage?.costUsd ?? result?.costUsd;
      return {
        versionId: versionId || result?.promptVersionId,
        version: typeof version === 'string' ? version : (version ? mapPromptVersionResponse(version) : versionId),
        resultId: item.resultId || item.result_id || result?.id,
        result,
        provider: item.provider || result?.provider,
        model: item.model || result?.model,
        usage,
        totalTokens,
        costUsd,
        costKnown: item.costKnown ?? item.cost_known ?? usage?.costKnown ?? result?.costKnown,
        status: item.status || result?.status,
        output: item.output || result?.output,
        outputSummary: item.outputSummary || item.output_summary || result?.outputSummary,
        summary: item.summary || result?.summary,
      };
    });
  }
  return mappedResults.map(result => ({
    versionId: result.promptVersionId,
    version: result.promptVersionId,
    resultId: result.id,
    result,
    provider: result.provider,
    model: result.model,
    usage: result.usage,
    totalTokens: result.usage?.totalTokens ?? result.totalTokens,
    costUsd: result.usage?.costUsd ?? result.costUsd,
    costKnown: result.usage?.costKnown ?? result.costKnown,
    status: result.status,
    output: result.output,
    outputSummary: result.outputSummary,
    summary: result.summary,
  }));
}

function promptBatchComparisonTotalTokens(items: PromptHarnessBatchComparisonItem[]): number | undefined {
  const values = items.map(item => item.totalTokens ?? item.usage?.totalTokens).filter(isFiniteNumber);
  return values.length > 0 ? values.reduce((total, value) => total + value, 0) : undefined;
}

function promptBatchComparisonTotalCostUsd(items: PromptHarnessBatchComparisonItem[]): number | undefined {
  const values = items.map(item => item.costUsd ?? item.usage?.costUsd).filter(isFiniteNumber);
  return values.length > 0 ? values.reduce((total, value) => total + value, 0) : undefined;
}

function promptBatchComparisonCostKnown(items: PromptHarnessBatchComparisonItem[]): boolean | undefined {
  const values = items.map(item => item.costKnown ?? item.usage?.costKnown).filter(value => value !== undefined);
  return values.length > 0 ? values.every(Boolean) : undefined;
}

function promptComparisonTotalTokens(baseUsage?: AIUsage, targetUsage?: AIUsage): number | undefined {
  const values = [baseUsage?.totalTokens, targetUsage?.totalTokens].filter(isFiniteNumber);
  return values.length > 0 ? values.reduce((total, value) => total + value, 0) : undefined;
}

function mapRequiredAIActionResponse(action: AIActionResponse | undefined, context: string): AIAction {
  if (!action?.id || !action.kind || !action.label) {
    throw new Error(context);
  }
  const kind = normalizeAIActionKind(action.kind);
  if (!kind) {
    throw new Error(`${zhCN.aiActions.unknownKind} ${action.kind}`);
  }
  const targetKinds = (mapStringList(action.targetKinds) ?? []).map(targetKind => targetKind.trim());
  if (kind === 'tool') {
    const catalogAction = action.id.startsWith('catalog.');
    const commonToolFieldsMissing = !action.packId || !action.packName || !action.manifestPath;
    const invocationFieldsMissing = catalogAction
      ? targetKinds.length === 0
      : !Array.isArray(action.command) || action.command.length === 0 || !action.commandText;
    if (commonToolFieldsMissing || invocationFieldsMissing) {
      throw new Error(context);
    }
  }
  const status = translateOptionalBackendStatus(action.status);
  return {
    id: action.id,
    kind,
    label: action.label,
    ...(status ? { status } : {}),
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
    targetKinds,
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
    case 'tool_result':
      return 'tool_result';
    case 'workspace_file':
      return 'file';
    case 'evidence':
      return 'evidence';
    case 'report':
      return 'report';
    case 'blackboard':
      return 'blackboard';
    case 'agent':
      return 'agent';
    case 'worker':
      return 'worker';
    case 'execution':
      return 'execution';
    case 'harness':
      return 'harness';
    case 'strategy':
      return 'strategy';
    case 'sisyphus':
      return 'sisyphus';
    case 'frontier':
      return 'frontier';
    case 'orchestrator':
      return 'orchestrator';
    case 'planner':
      return 'planner';
    case 'prompt':
      return 'prompt';
    case 'goal':
      return 'goal';
    case 'eventlog':
      return 'eventlog';
    default:
      return null;
  }
}

function mapAIProviderToolCalls(calls: AIProviderToolCallResponse[] | undefined): AIProviderToolCall[] | undefined {
  if (!Array.isArray(calls)) {
    return undefined;
  }
  return calls
    .map(call => {
      if (!call || typeof call !== 'object') {
        return null;
      }
      const id = cleanOptionalString(call.id);
      const name = cleanOptionalString(call.name);
      const actionId = cleanOptionalString(call.actionId);
      if (!id || !name || !actionId) {
        return null;
      }
      return { id, name, actionId };
    })
    .filter((call): call is AIProviderToolCall => call !== null);
}

function mapRequiredAIMessage(message: AIMessageResponse | undefined, context: string): AIMessage {
  const role = normalizeAIMessageRole(message?.role);
  const toolCallId = cleanOptionalString(message?.toolCallId ?? message?.tool_call_id);
  const actionId = cleanOptionalString(message?.actionId ?? message?.action_id);
  const toolName = cleanOptionalString(message?.toolName ?? message?.tool_name);
  const nativeToolCall = message?.nativeToolCall ?? message?.native_tool_call;
  const nativeToolResultForwarded = message?.nativeToolResultForwarded;
  const providerToolCalls = mapAIProviderToolCalls(message?.providerToolCalls);
  const hasToolCorrelation = Boolean(toolCallId || actionId || toolName || nativeToolCall || nativeToolResultForwarded || providerToolCalls?.length);
  if (typeof message?.content !== 'string' || (!message.content && !hasToolCorrelation) || !role) {
    throw new Error(context);
  }
  return {
    id: cleanOptionalString(message.id),
    role,
    content: message.content,
    createdAt: normalizeAIInstantValue(message.createdAt ?? message.timestamp),
    status: message.status,
    tokenCount: isFiniteNumber(message.tokenCount) ? message.tokenCount : undefined,
    toolCallId: cleanOptionalString(message.toolCallId ?? message.tool_call_id),
    actionId: cleanOptionalString(message.actionId ?? message.action_id),
    toolName: cleanOptionalString(message.toolName ?? message.tool_name),
    nativeToolCall: message.nativeToolCall ?? message.native_tool_call,
    nativeToolResultForwarded,
    providerToolCalls,
    contextReferences: mapOptionalAIContextReferences(message.contextReferences ?? message.context_references, zhCN.ai.contextMissing),
    actions: mapAIChatActionOutcomes(message.actions),
    usage: mapAIUsage(message.usage, zhCN.ai.usageMissing),
    routing: mapAIRoutingMetadata(message),
  };
}

function mapAIChatActionOutcomes(actions: AIChatActionOutcomeResponse[] | undefined): AIChatActionOutcome[] {
  return (actions ?? []).flatMap(action => {
    if (!action?.kind || typeof action.summary !== 'string') {
      return [];
    }
    return [{
      kind: action.kind,
      summary: action.summary,
      path: action.path,
      output: action.output,
      diff: action.diff,
      status: action.status,
      exitCode: isFiniteNumber(action.exitCode) ? action.exitCode : undefined,
      executionId: action.executionId ?? action.execution_id,
    }];
  });
}

function normalizeAIInstantValue(value: string | number | undefined): string | undefined {
  if (value === undefined || value === '') {
    return undefined;
  }
  if (typeof value === 'number') {
    return new Date(value * 1000).toISOString();
  }
  return value;
}

function normalizeAIMessageRole(role?: string): AIMessageRole | null {
  switch ((role ?? '').toLowerCase()) {
    case 'user':
      return 'user';
    case 'assistant':
      return 'assistant';
    case 'system':
      return 'system';
    case 'tool':
      return 'tool';
    default:
      return null;
  }
}

function mapAIActionTrace(trace: AIActionTraceItemResponse[] | undefined): AIActionTraceItem[] {
  if (!Array.isArray(trace)) {
    return [];
  }
  return trace
    .map(mapAIActionTraceItem)
    .filter((item): item is AIActionTraceItem => item !== null);
}

function mapAIActionTraceItem(item: AIActionTraceItemResponse | undefined): AIActionTraceItem | null {
  if (!item || typeof item !== 'object') {
    return null;
  }
  const id = cleanOptionalString(item.id);
  const messageId = cleanOptionalString(item.messageId ?? item.message_id);
  const index = isNonNegativeInteger(item.index) ? item.index : undefined;
  const role = cleanOptionalString(item.role);
  const phase = cleanOptionalString(item.phase);
  const toolCallId = cleanOptionalString(item.toolCallId ?? item.tool_call_id);
  const actionId = cleanOptionalString(item.actionId ?? item.action_id);
  const mapped: AIActionTraceItem = {
    id,
    messageId,
    index,
    role,
    phase,
    toolCallId,
    actionId,
    toolName: cleanOptionalString(item.toolName ?? item.tool_name),
    nativeToolCall: item.nativeToolCall ?? item.native_tool_call,
    nativeToolResultForwarded: item.nativeToolResultForwarded,
    actionKind: cleanOptionalString(item.actionKind ?? item.action_kind),
    status: cleanOptionalString(item.status),
    title: cleanOptionalString(item.title),
    summary: cleanOptionalString(item.summary),
    createdAt: normalizeAIInstantValue(item.createdAt ?? item.created_at),
    provider: cleanOptionalString(item.provider),
    model: cleanOptionalString(item.model),
    contextReferences: mapLooseAIContextReferences(item.contextReferences ?? item.context_references),
  };
  return Object.entries(mapped).some(([key, value]) => (
    key !== 'contextReferences'
    && value !== undefined
    && (typeof value !== 'string' || value.trim() !== '')
  )) || mapped.contextReferences.length > 0
    ? mapped
    : null;
}

function mapAIContextReferences(references: AIContextReferenceResponse[], context: string): AIContextReference[] {
  void context;
  return references
    .map(mapAIContextReference)
    .filter((reference): reference is AIContextReference => reference !== null);
}

function mapAIContextReference(reference: AIContextReferenceResponse | undefined): AIContextReference | null {
  if (!reference || typeof reference !== 'object') {
    return null;
  }
  const id = cleanOptionalString(reference.id);
  const title = cleanOptionalString(reference.title);
  const path = cleanOptionalString(reference.path);
  const detail = cleanOptionalString(reference.detail);
  const summary = cleanOptionalString(reference.summary);
  const source = cleanOptionalString(reference.source);
  const label = cleanOptionalString(reference.label) ?? title ?? path ?? id;
  const kind = cleanOptionalString(reference.kind);
  const line = isFiniteNumber(reference.line) ? reference.line : undefined;
  const mapped: AIContextReference = {
    id,
    label,
    kind,
    title,
    path,
    detail,
    summary,
    source,
    line,
  };
  return Object.values(mapped).some(value => value !== undefined && (typeof value !== 'string' || value.trim() !== ''))
    ? mapped
    : null;
}

function mapLooseAIContextReferences(references: AIContextReferenceResponse[] | undefined): AIContextReference[] {
  if (!Array.isArray(references)) {
    return [];
  }
  return references
    .map(mapAIContextReference)
    .filter((reference): reference is AIContextReference => reference !== null);
}

function mapOptionalAIContextReferences(references: AIContextReferenceResponse[] | undefined, context: string): AIContextReference[] | undefined {
  if (references === undefined) {
    return undefined;
  }
  if (!Array.isArray(references)) {
    throw new Error(context);
  }
  return mapAIContextReferences(references, context);
}

function mapAIRoutingMetadata(response: AIRoutingMetadataResponse | undefined): AIRoutingMetadata | undefined {
  if (!response) {
    return undefined;
  }
  const nested = response.routing;
  const usage = mapAIUsage(response.usage, zhCN.ai.usageMissing) ?? mapAIUsage(nested?.usage, zhCN.ai.usageMissing);
  const providerAttempts = mapAIProviderAttempts(
    response.providerAttempts
      ?? response.provider_attempts
      ?? response.attempts
      ?? nested?.providerAttempts
      ?? nested?.provider_attempts
      ?? nested?.attempts,
  );
  const rawFailover = response.failover ?? nested?.failover;
  const inferredFailover = inferProviderFailover(providerAttempts);
  const failover = typeof rawFailover === 'boolean' ? rawFailover : inferredFailover.failover;
  const routing: AIRoutingMetadata = {
    routingStrategy: cleanOptionalString(response.routingStrategy ?? response.routing_strategy ?? nested?.routingStrategy ?? nested?.routing_strategy),
    provider: cleanOptionalString(response.provider ?? nested?.provider),
    model: cleanOptionalString(response.model ?? nested?.model),
    finalProvider: cleanOptionalString(response.finalProvider ?? response.final_provider ?? nested?.finalProvider ?? nested?.final_provider) ?? inferredFailover.finalProvider,
    finalModel: cleanOptionalString(response.finalModel ?? response.final_model ?? nested?.finalModel ?? nested?.final_model) ?? inferredFailover.finalModel,
    fallbackFrom: cleanOptionalString(response.fallbackFrom ?? response.fallback_from ?? nested?.fallbackFrom ?? nested?.fallback_from) ?? inferredFailover.fallbackFrom,
    failover,
    providerAttempts,
    attempts: providerAttempts,
    usage,
  };
  return Object.values(routing).some(value => Array.isArray(value) ? value.length > 0 : value !== undefined) ? routing : undefined;
}

function mapAIProviderAttempts(attempts: AIProviderAttemptResponse[] | undefined): AIProviderAttempt[] | undefined {
  if (!Array.isArray(attempts)) {
    return undefined;
  }
  const mapped = attempts.map(attempt => {
    const usage = mapAIUsage(attempt.usage, zhCN.ai.usageMissing);
    const latencyMs = attempt.latencyMs ?? attempt.latency_ms;
    const costUsd = attempt.costUsd ?? attempt.cost_usd ?? usage?.costUsd;
    const costKnown = attempt.costKnown ?? attempt.cost_known ?? usage?.costKnown;
    for (const value of [latencyMs, costUsd]) {
      if (value !== undefined && !isFiniteNumber(value)) {
        throw new Error(zhCN.ai.usageMissing);
      }
    }
    if (costKnown !== undefined && typeof costKnown !== 'boolean') {
      throw new Error(zhCN.ai.usageMissing);
    }
    const mappedAttempt: AIProviderAttempt = {
      provider: cleanOptionalString(attempt.provider),
      model: cleanOptionalString(attempt.model),
      type: cleanOptionalString(attempt.type),
      status: cleanOptionalString(attempt.status),
      error: cleanOptionalString(attempt.error),
      latencyMs,
      usage,
      costUsd,
      costKnown,
    };
    return Object.values(mappedAttempt).some(value => value !== undefined) ? mappedAttempt : null;
  }).filter((attempt): attempt is AIProviderAttempt => attempt !== null);
  return mapped.length > 0 ? mapped : undefined;
}

function inferProviderFailover(attempts: AIProviderAttempt[] | undefined): Pick<AIRoutingMetadata, 'failover' | 'fallbackFrom' | 'finalProvider' | 'finalModel'> {
  if (!attempts || attempts.length === 0) {
    return {};
  }
  const successIndex = attempts.findIndex(attempt => attempt.status === 'succeeded');
  const finalAttempt = successIndex >= 0 ? attempts[successIndex] : attempts[attempts.length - 1];
  const failedBeforeSuccess = successIndex > 0 && attempts.slice(0, successIndex).some(attempt => attempt.status === 'failed');
  const fallbackSource = failedBeforeSuccess ? attempts.slice(0, successIndex).find(attempt => attempt.status === 'failed') : undefined;
  const fallbackFrom = fallbackSource ? [fallbackSource.provider, fallbackSource.model].filter(Boolean).join(' / ') : undefined;
  return {
    failover: failedBeforeSuccess ? true : undefined,
    fallbackFrom: fallbackFrom || undefined,
    finalProvider: finalAttempt?.provider,
    finalModel: finalAttempt?.model,
  };
}

function cleanOptionalString(value: unknown): string | undefined {
  return typeof value === 'string' && value.trim() !== '' ? value : undefined;
}

function isNonNegativeInteger(value: unknown): value is number {
  return isFiniteNumber(value) && Number.isInteger(value) && value >= 0;
}

function mapAIUsage(usage: AIUsageResponse | undefined, context: string): AIUsage | undefined {
  if (!usage) {
    return undefined;
  }
  const inputTokens = usage.inputTokens ?? usage.input_tokens;
  const outputTokens = usage.outputTokens ?? usage.output_tokens;
  const promptTokens = usage.promptTokens ?? usage.prompt_tokens ?? inputTokens;
  const completionTokens = usage.completionTokens ?? usage.completion_tokens ?? outputTokens;
  const explicitTotalTokens = usage.totalTokens ?? usage.total_tokens;
  const totalTokens = explicitTotalTokens
    ?? (inputTokens !== undefined || outputTokens !== undefined ? (inputTokens ?? 0) + (outputTokens ?? 0) : undefined)
    ?? (promptTokens !== undefined || completionTokens !== undefined ? (promptTokens ?? 0) + (completionTokens ?? 0) : undefined);
  const latencyMs = usage.latencyMs ?? usage.latency_ms;
  const costUsd = usage.costUsd ?? usage.cost_usd;
  const costKnown = usage.costKnown ?? usage.cost_known;
  for (const value of [inputTokens, outputTokens, promptTokens, completionTokens, totalTokens, latencyMs, costUsd]) {
    if (value !== undefined && !isFiniteNumber(value)) {
      throw new Error(context);
    }
  }
  if (costKnown !== undefined && typeof costKnown !== 'boolean') {
    throw new Error(context);
  }
  if (
    inputTokens === undefined
    && outputTokens === undefined
    && promptTokens === undefined
    && completionTokens === undefined
    && totalTokens === undefined
    && latencyMs === undefined
    && costUsd === undefined
    && costKnown === undefined
  ) {
    return undefined;
  }
  return {
    inputTokens,
    outputTokens,
    promptTokens,
    completionTokens,
    totalTokens,
    latencyMs,
    costUsd,
    costKnown,
  };
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
    apiKeyValue: '',
    apiKeyConfigured: false,
    enabled: false,
    timeoutMs: 0,
    contextWindowTokens: 0,
    capabilities: [],
    routingStrategy: '',
    status: zhCN.backendStatus.unknown,
  };
}

function emptyProviderConfig(active: ProviderSettings = emptyProvider()): ProviderConfig {
  return {
    path: '',
    routingStrategy: active.routingStrategy || '',
    providers: active.name ? [active] : [],
    active,
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
    apiKeyValue: provider?.apiKeyValue ?? fallback.apiKeyValue,
    apiKeyConfigured: provider?.apiKeyConfigured ?? fallback.apiKeyConfigured,
    enabled: provider?.enabled ?? fallback.enabled,
    timeoutMs: provider?.timeoutMs ?? fallback.timeoutMs,
    contextWindowTokens: provider?.contextWindowTokens ?? fallback.contextWindowTokens,
    capabilities: provider?.capabilities ?? fallback.capabilities,
    routingStrategy: provider?.routingStrategy || fallback.routingStrategy,
    status: translateBackendStatus(provider?.status),
  };
}

function mapProviderConfig(config?: ProviderConfigResponse, activeFallback?: ProviderResponse | ProviderSettings): ProviderConfig {
  const fallbackActive = mapProviderSettings(activeFallback);
  if (!config) {
    return emptyProviderConfig(fallbackActive);
  }
  const providers = Array.isArray(config.providers)
    ? config.providers.map(provider => mapProviderSettings(provider))
    : [];
  const active = config.active ? mapProviderSettings(config.active) : fallbackActive;
  const mergedProviders = providers.length > 0
    ? providers
    : active.name ? [active] : [];
  return {
    path: config.path ?? '',
    routingStrategy: config.routingStrategy || active.routingStrategy || fallbackActive.routingStrategy,
    providers: mergedProviders,
    active,
  };
}

function mapProviderTestResult(response: ProviderTestResultResponse): ProviderTestResult {
  const usage = mapAIUsage(response.usage, zhCN.ai.usageMissing);
  const latencyMs = response.latencyMs ?? response.latency_ms ?? usage?.latencyMs;
  if (latencyMs !== undefined && !isFiniteNumber(latencyMs)) {
    throw new Error(zhCN.ai.usageMissing);
  }
  const attempts = mapAIProviderAttempts(response.providerAttempts ?? response.provider_attempts ?? response.attempts);
  const configResponse = response.config ?? response.providerConfig ?? response.provider_config;
  const config = configResponse ? mapProviderConfig(configResponse) : undefined;
  const provider = cleanOptionalString(response.provider) ?? attempts?.find(attempt => attempt.provider)?.provider ?? config?.active.name ?? '';
  const model = cleanOptionalString(response.model) ?? attempts?.find(attempt => attempt.model)?.model ?? config?.active.model ?? '';
  return {
    provider,
    model,
    status: translateBackendStatus(response.status),
    content: cleanOptionalString(response.content),
    latencyMs,
    usage,
    attempts,
    config,
  };
}

function emptyProviderUsageSummary(): ProviderUsageSummary {
  return {
    recordCount: 0,
    providers: [],
    models: [],
    inputTokens: 0,
    outputTokens: 0,
    totalTokens: 0,
    estimatedCostUsd: 0,
    estimatedCostRecordCount: 0,
    budgetEnforcedCount: 0,
    budgetWithinLimitCount: 0,
  };
}

function emptyProviderUsageState(): ProviderUsageState {
  return {
    path: '',
    summary: emptyProviderUsageSummary(),
    records: [],
  };
}

function mapProviderUsageState(response?: ProviderUsageStateResponse): ProviderUsageState {
  if (!response) {
    return emptyProviderUsageState();
  }
  const records = response.recentRecords ?? response.recent_records ?? response.records;
  return {
    path: response.path ?? '',
    summary: mapProviderUsageSummary(response.summary),
    records: Array.isArray(records)
      ? records.flatMap(record => {
          const mapped = mapProviderUsageRecord(record);
          return mapped ? [mapped] : [];
        })
      : [],
  };
}

function mapProviderUsageSummary(summary?: ProviderUsageSummaryResponse): ProviderUsageSummary {
  const fallback = emptyProviderUsageSummary();
  if (!summary) {
    return fallback;
  }
  return {
    recordCount: finiteNumberOr(summary.recordCount ?? summary.record_count, fallback.recordCount),
    providers: mapStringList(summary.providers) ?? [],
    models: mapStringList(summary.models) ?? [],
    inputTokens: finiteNumberOr(summary.inputTokens ?? summary.input_tokens, fallback.inputTokens),
    outputTokens: finiteNumberOr(summary.outputTokens ?? summary.output_tokens, fallback.outputTokens),
    totalTokens: finiteNumberOr(summary.totalTokens ?? summary.total_tokens, fallback.totalTokens),
    estimatedCostUsd: finiteNumberOr(summary.estimatedCostUsd ?? summary.estimated_cost_usd, fallback.estimatedCostUsd),
    estimatedCostRecordCount: finiteNumberOr(summary.estimatedCostRecordCount ?? summary.estimated_cost_records, fallback.estimatedCostRecordCount),
    budgetEnforcedCount: finiteNumberOr(summary.budgetEnforcedCount ?? summary.budget_enforced_records, fallback.budgetEnforcedCount),
    budgetWithinLimitCount: finiteNumberOr(summary.budgetWithinLimitCount ?? summary.budget_within_limit_records, fallback.budgetWithinLimitCount),
    firstCreatedAt: cleanOptionalString(summary.firstCreatedAt ?? summary.first_created_at),
    lastCreatedAt: cleanOptionalString(summary.lastCreatedAt ?? summary.last_created_at),
  };
}

function mapProviderUsageRecord(record?: ProviderUsageRecordResponse): ProviderUsageRecord | null {
  if (!record) {
    return null;
  }
  const inputTokens = record.tokens?.input ?? record.inputTokens ?? record.input_tokens;
  const outputTokens = record.tokens?.output ?? record.outputTokens ?? record.output_tokens;
  const totalTokens = record.tokens?.total ?? record.totalTokens ?? record.total_tokens;
  const estimatedCostUsd = record.estimatedCostUsd ?? record.estimated_cost_usd;
  const contextWindowTokens = record.tokens?.contextWindow ?? record.tokens?.context_window ?? record.contextWindowTokens ?? record.context_window_tokens;
  const budgetMaxEstimatedCostUsd = record.budgetMaxEstimatedCostUsd ?? record.budget_max_estimated_cost_usd;
  const createdAt = record.createdAt ?? record.created_at;
  if (
    !createdAt
    || !isFiniteNumber(inputTokens)
    || !isFiniteNumber(outputTokens)
    || !isFiniteNumber(totalTokens)
    || (estimatedCostUsd !== undefined && !isFiniteNumber(estimatedCostUsd))
    || (contextWindowTokens !== undefined && !isFiniteNumber(contextWindowTokens))
    || (budgetMaxEstimatedCostUsd !== undefined && !isFiniteNumber(budgetMaxEstimatedCostUsd))
  ) {
    return null;
  }
  return {
    schemaVersion: finiteNumberOr(record.schemaVersion ?? record.schema_version, 1),
    path: record.path ?? '',
    recordId: record.recordId ?? record.record_id ?? record.id ?? '',
    source: record.source ?? '',
    createdAt,
    providerName: record.providerName ?? record.provider_name ?? record.provider ?? '',
    model: record.model ?? '',
    inputTokens,
    outputTokens,
    totalTokens,
    contextWindowTokens,
    capabilities: mapStringList(record.capabilities) ?? [],
    estimatedCostUsd,
    estimatedCostAvailable: record.estimatedCostAvailable ?? record.estimated_cost_available ?? false,
    budgetEnforced: record.budgetEnforced ?? record.budget_enforced,
    budgetStatus: record.budgetStatus ?? record.budget_status,
    budgetMaxEstimatedCostUsd,
  };
}

function finiteNumberOr(value: unknown): number | undefined;
function finiteNumberOr(value: unknown, fallback: number): number;
function finiteNumberOr(value: unknown, fallback?: number): number | undefined {
  return isFiniteNumber(value) ? value : fallback;
}

function mapRequiredProviderConfigResponse(payload: ProviderConfigResponse, context: string): ProviderConfig {
  const config = mapProviderConfig(payload);
  if (!config.active.name && config.providers.length === 0) {
    throw new Error(`${context}: ${zhCN.settings.actions.saveProviderFailed}`);
  }
  return config;
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
    providerConfig: emptyProviderConfig(),
    providerUsage: emptyProviderUsageState(),
    toolchainPacks: [],
    toolActions: [],
    workspaceItems: mapWorkspaceItems(),
    graphNodes: [],
    graphEdges: [],
    executionSteps: [],
    replayEvents: [],
    evidenceItems: [],
    reportFindings: [],
    reports: [],
    blackboard: { path: '', items: [], observations: [] },
    agentState: { path: '', eventPath: '', profilePath: '', profiles: [], tasks: [], events: [], counts: {} },
    frontierQueue: mapFrontierQueueResponse({}),
    harness: emptyHarnessState(),
    orchestratorState: mapOrchestratorState(undefined),
    plannerState: emptyPlannerState(),
    aiConversation: null,
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
    return files.flatMap(file => {
      const id = file.id || '';
      const label = file.path || '';
      if (!id || !label || !file.kind) {
        return [];
      }
      const meta = translateFileKind(file.kind);
      return [{
        id,
        label,
        meta,
        filePath: file.kind === 'directory' ? undefined : file.path,
        kind: file.kind,
        size: isFiniteNumber(file.size) ? file.size : undefined,
        modifiedAt: isFiniteNumber(file.modifiedAt) ? file.modifiedAt : undefined,
      }];
    });
  }
  return [];
}

function translateFileKind(kind: string): string {
  const labels = zhCN.explorer.fileKinds as Record<string, string>;
  return labels[kind.toLowerCase()] ?? unknownBackendValueLabel(zhCN.explorer.fileKinds.unknown, kind);
}

function mapGraphNodes(response?: EvidenceGraphResponse): GraphNode[] {
  const sourceNodes = (response?.nodes ?? []).flatMap(node => {
    if (!node.id || !node.label || !node.type) {
      return [];
    }
    const type = node.type;
    const kind = normalizeKind(type);
    return [{ node, kind, type }];
  });
  const layout = layoutFor(sourceNodes.length);
  return sourceNodes.map(({ node, kind, type }, index) => {
    const data = node.data ?? {};
    const id = node.id as string;
    return {
      id,
      label: translateEvidenceLabel(node.label as string),
      subtitle: graphNodeKindLabel(kind, type),
      kind,
      rawKind: type,
      severity: normalizeSeverity(data.severity),
      rawSeverity: unknownSeverityValue(data.severity),
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
  return {
    id: execution.id,
    label: execution.label,
    command: execution.command,
    status,
    rawStatus: status === 'unknown' ? execution.status.trim() : undefined,
    retryOf: execution.retryOf,
    startedAt: execution.startedAt,
    completedAt: execution.completedAt,
    exitCode: execution.exitCode,
    logPath: execution.logPath,
    artifactDir: execution.artifactDir,
  };
}

function normalizeExecutionStatus(status: string): ExecutionStep['status'] {
  switch (status.trim().toLowerCase()) {
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
      return 'unknown';
  }
}

function toolchainStatus(response?: ToolchainHealthResponse): string {
  if (!response) {
    return zhCN.shell.statusOffline;
  }
  const ready = Object.values(response.packs ?? {}).reduce((sum, pack) => sum + pack.toolsReady, 0);
  return `${translateBackendStatus(response.status)} · ${ready} 个工具`;
}

function normalizeKind(value: string): GraphNode['kind'] {
  switch (value.trim().toLowerCase()) {
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
      return 'unknown';
  }
}

function graphNodeKindLabel(kind: GraphNode['kind'], rawKind: string): string {
  return kind === 'unknown'
    ? unknownBackendValueLabel(zhCN.node.kinds.unknown, rawKind)
    : zhCN.node.kinds[kind];
}

function normalizeSeverity(value: unknown): Severity | undefined {
  if (typeof value !== 'string') {
    return undefined;
  }
  const normalized = value.trim().toLowerCase();
  if (!normalized) {
    return undefined;
  }
  if (['critical', 'high', 'medium', 'low', 'info'].includes(normalized)) {
    return normalized as Severity;
  }
  return 'unknown';
}

function unknownSeverityValue(value: unknown): string | undefined {
  if (typeof value !== 'string') {
    return undefined;
  }
  const normalized = value.trim();
  return normalized && normalizeSeverity(normalized) === 'unknown' ? normalized : undefined;
}

function translateEvidenceLabel(label: string): string {
  const known: Record<string, string> = {
    Target: zhCN.node.kinds.target,
    Surface: zhCN.node.kinds.surface,
    Finding: zhCN.node.kinds.finding,
  };
  return known[label] ?? label;
}

function translateOptionalBackendStatus(status: unknown): string | undefined {
  const value = cleanOptionalString(status);
  return value ? translateBackendStatus(value) : undefined;
}

function translateBackendStatus(status?: string): string {
  const value = (status ?? '').trim();
  if (!value) {
    return zhCN.common.unknownStatus;
  }
  if (/[\u3400-\u9fff]/.test(value)) {
    return value;
  }
  const normalized = value.toLowerCase().replaceAll('-', '_');
  const labels = zhCN.common.statuses as Record<string, string>;
  return labels[normalized] ?? unknownBackendValueLabel(zhCN.common.unknownStatus, value);
}

function translateReplayEventType(type: string): string {
  const key = type.toLowerCase();
  return (zhCN.replay.eventTypes as Record<string, string>)[key] ?? unknownBackendValueLabel(zhCN.replay.unknownType, type);
}

function unknownBackendValueLabel(fallback: string, value?: string): string {
  const backendValue = (value ?? '').trim();
  return backendValue ? `${fallback}（${backendValue}）` : fallback;
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
      return unknownBackendValueLabel(zhCN.backendStatus.unknown, action);
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
