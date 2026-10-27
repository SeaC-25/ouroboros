export type IDEMode = 'mission' | 'ai' | 'blackboard' | 'agents' | 'harness' | 'graph' | 'replay' | 'report' | 'runtime' | 'settings';

export type AIMessageRole = 'user' | 'assistant' | 'system' | 'tool';

export interface AIProviderToolCall {
  id: string;
  name: string;
  actionId: string;
}

export interface AIMessage {
  id?: string;
  role: AIMessageRole;
  content: string;
  createdAt?: string;
  status?: string;
  tokenCount?: number;
  toolCallId?: string;
  actionId?: string;
  toolName?: string;
  nativeToolCall?: boolean;
  nativeToolResultForwarded?: boolean;
  providerToolCalls?: AIProviderToolCall[];
  contextReferences?: AIContextReference[];
  usage?: AIUsage;
  routing?: AIRoutingMetadata;
  // AI 对话内联动作结果（后端 ChatMessage.Actions）：observe=原始观测，blackboard=结论笔记。
  actions?: AIChatActionOutcome[];
}

// AIChatActionOutcome 是聊天消息内联动作的结构化结果（观测-推理循环中
// ACTION:OBSERVE / ACTION:BLACKBOARD 的回传），原样透传不判定。
export interface AIChatActionOutcome {
  kind: string;
  summary: string;
  path?: string;
  output?: string;
  diff?: string;
  status?: string;
  exitCode?: number;
  executionId?: string;
}

export interface AIContextReference {
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

export interface AIUsage {
  inputTokens?: number;
  outputTokens?: number;
  promptTokens?: number;
  completionTokens?: number;
  totalTokens?: number;
  latencyMs?: number;
  costUsd?: number;
  costKnown?: boolean;
}

export interface AIProviderAttempt {
  provider?: string;
  model?: string;
  type?: string;
  status?: string;
  error?: string;
  latencyMs?: number;
  usage?: AIUsage;
  costUsd?: number;
  costKnown?: boolean;
}

export interface AIRoutingMetadata {
  routingStrategy?: string;
  provider?: string;
  model?: string;
  finalProvider?: string;
  finalModel?: string;
  fallbackFrom?: string;
  failover?: boolean;
  providerAttempts?: AIProviderAttempt[];
  attempts?: AIProviderAttempt[];
  usage?: AIUsage;
}

export interface AIConversationRuntime {
  id?: string;
  status?: string;
  phase?: string;
  startedAt?: string;
  updatedAt?: string;
  durationSeconds?: number;
  messageCount?: number;
  actionCount?: number;
  toolResultCount?: number;
  modelReplyCount?: number;
  finalReplyCount?: number;
  providerCallCount?: number;
  provider?: string;
  model?: string;
  routingStrategy?: string;
  lastMessageId?: string;
  lastMessageRole?: string;
  lastActionId?: string;
  lastActionKind?: string;
  lastActionStatus?: string;
  lastExecutionId?: string;
  totalTokens?: number;
  costKnown?: boolean;
  costUsd?: number;
}

export interface AIConversation {
  messages: AIMessage[];
  contextReferences: AIContextReference[];
  actionTrace: AIActionTraceItem[];
  provider?: string;
  model?: string;
  updatedAt?: string;
  usage?: AIUsage;
  routing?: AIRoutingMetadata;
  runtime?: AIConversationRuntime;
}

export interface AIConversationSessionSummary {
  id: string;
  path?: string;
  current: boolean;
  messageCount: number;
  userCount: number;
  assistantCount: number;
  toolCount: number;
  startedAt?: string;
  updatedAt?: string;
  latestUserGoal?: string;
  latestAssistantSummary?: string;
  provider?: string;
  model?: string;
  totalTokens?: number;
  costUsd?: number;
  costKnown?: boolean;
}

export interface AIConversationSessionOperationResponse {
  session: AIConversationSessionSummary;
  sessions: AIConversationSessionSummary[];
  conversation: AIConversation;
}

export interface AIConversationSearchResult {
  id: string;
  sessionId: string;
  sessionPath?: string;
  sessionCurrent?: boolean;
  messageId: string;
  role: string;
  snippet: string;
  contentSnippet: string;
  path?: string;
  createdAt?: string;
  provider?: string;
  model?: string;
  contextReferenceCount: number;
}

export interface AIConversationSearchResponse {
  query: string;
  results: AIConversationSearchResult[];
  count: number;
}

export interface AIConversationExportRequest {
  sessionId?: string;
  format?: string;
  includeContextReferences?: boolean;
}

export interface AIConversationExportResult {
  sessionId: string;
  path: string;
  format: string;
  messageCount: number;
  exportedAt: string;
  summary?: string;
}

export interface AIActionTraceItem {
  id?: string;
  messageId?: string;
  index?: number;
  role?: string;
  phase?: string;
  toolCallId?: string;
  actionId?: string;
  toolName?: string;
  nativeToolCall?: boolean;
  nativeToolResultForwarded?: boolean;
  actionKind?: string;
  status?: string;
  title?: string;
  summary?: string;
  createdAt?: string;
  provider?: string;
  model?: string;
  contextReferences: AIContextReference[];
}

export interface AIChatRequest {
  message: string;
  contextReferences?: AIContextReference[];
}

export interface AIChatResponse {
  conversation: AIConversation;
  message: AIMessage;
  provider?: string;
  model?: string;
  usage?: AIUsage;
  routing?: AIRoutingMetadata;
  contextReferences: AIContextReference[];
  actionResults?: AIActionRunResponse[];
}

export interface AIChatStreamEvent {
  event: string;
  phase?: string;
  status?: string;
  message?: string;
  index?: number;
  total?: number;
  batchIndex?: number;
  tokenIndex?: number;
  delta?: string;
  textDelta?: string;
  contentDelta?: string;
  toolCallId?: string;
  toolName?: string;
  nativeToolCall?: boolean;
  actionId?: string;
  actionKind?: string;
  actionLabel?: string;
  startedAt?: string;
  finishedAt?: string;
  elapsedMs?: number;
  resultKind?: string;
  resultSummary?: string;
  referenceCount?: number;
  contextReferences?: AIContextReference[];
  conversation?: AIConversation;
  response?: AIChatResponse;
  actionResult?: AIActionRunResponse;
  runtime?: AIConversationRuntime;
  provider?: string;
  model?: string;
}

// Kept verbatim because the static GUI contracts also recognize this pre-worker baseline:
// export type AIActionKind = 'command' | 'tool' | 'tool_result' | 'file' | 'evidence' | 'report' | 'blackboard' | 'agent' | 'harness' | 'strategy' | 'sisyphus' | 'frontier' | 'orchestrator' | 'planner' | 'prompt' | 'goal' | 'eventlog';
export type AIActionKind = 'command' | 'tool' | 'tool_result' | 'file' | 'evidence' | 'report' | 'blackboard' | 'agent' | 'harness' | 'strategy' | 'sisyphus' | 'frontier' | 'orchestrator' | 'planner' | 'prompt' | 'goal' | 'eventlog' | 'worker' | 'execution';

export interface AIAction {
  id: string;
  kind: AIActionKind;
  label: string;
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

export interface RuntimeWorker {
  workerId: string;
  mode: string;
  status: string;
  endpoint: string;
  capabilities: string[];
  labels: Record<string, string>;
  registeredAt: string;
  heartbeatAt: string;
  stale: boolean;
  path: string;
}

export interface RuntimeWorkerOrchestration {
  path: string;
  status: string;
  mode: string;
  runId: string;
  transportMode: string;
  createdAt: string;
  maxCandidates?: number;
  placementReady: number;
  placementSkipped: number;
  placementPlanPath?: string;
  claimedCount: number;
  claimSkipped: number;
  claimFailed: number;
  claimExecutionPaths?: string[];
  placementDispatchPlanPath?: string;
  dispatchReady: number;
  dispatchSkipped: number;
  placementDispatchBatchPath?: string;
  placementDispatchBatchStatus?: string;
  dispatchedCount: number;
  dispatchFailed: number;
  includeLifecycleMaintenance?: boolean;
  executeLifecycleRetryDispatches?: boolean;
  dispatchLifecycleRetryClaims?: boolean;
  lifecycleRetryTransportMode?: string;
  lifecycleMaintenancePath?: string;
  lifecycleAuditPath?: string;
  retryDispatchExecutionsCreated?: number;
  retryDispatchClaimsCreated?: number;
  retryDispatchClaimsDispatched?: number;
  retryDispatchTransportRequestsCreated?: number;
  retryDispatchExecutionPaths?: string[];
  error?: string;
}

export type RuntimeWorkerCounts = Record<string, number>;

export interface RuntimeWorkerState {
  workers: RuntimeWorker[];
  orchestrations: RuntimeWorkerOrchestration[];
  counts: RuntimeWorkerCounts;
  updatedAt: string;
  staleAfterSeconds: number;
}

export interface AIActionRunInput {
  command?: string;
  target?: string;
  targetKind?: string;
  args?: string[];
  arguments?: string;
  path?: string;
  paths?: string[];
  ids?: string[];
  content?: string;
  query?: string;
  targetPath?: string;
  kind?: string;
  title?: string;
  summary?: string;
  severity?: string;
  artifactPath?: string;
  endpoint?: string;
  name?: string;
  description?: string;
  instructions?: string;
  capabilities?: string[];
  recommendation?: string;
  evidence?: string;
  goal?: string;
  role?: string;
  agentId?: string;
  agentProfileId?: string;
  intentId?: string;
  leaseId?: string;
  attemptId?: string;
  transcript?: string;
  failureKind?: string;
  failureMessage?: string;
  status?: string;
  priority?: string;
  statusFilter?: string;
  priorityFilter?: string;
  readyFilter?: string;
  sourceFilter?: string;
  source?: string;
  filePath?: string;
  executionId?: string;
  waitTimeoutSeconds?: number;
  runId?: string;
  staleAfterSeconds?: number;
  transportMode?: string;
  maxCandidates?: number;
  leaseDurationSeconds?: number;
  transportAuthToken?: string;
  resultCallbackEndpoint?: string;
  includeLifecycleMaintenance?: boolean;
  executeLifecycleRetryDispatches?: boolean;
  dispatchLifecycleRetryClaims?: boolean;
  lifecycleRetryTransportMode?: string;
  lane?: string;
  type?: string;
  limit?: number;
  replayRunId?: string;
  snapshotId?: string;
  snapshotPath?: string;
  orchestratorRunId?: string;
  orchestratorObservationId?: string;
  graphNodeId?: string;
  replayEventId?: string;
  reportId?: string;
  blackboardItemId?: string;
  reportFindingId?: string;
  expectedResult?: string;
  promptId?: string;
  versionId?: string;
  baseVersionId?: string;
  targetVersionId?: string;
  promptVersionId?: string;
  versionIds?: string[];
  templateVariables?: Record<string, string>;
  providerName?: string;
  model?: string;
  agentRole?: string;
  agentTaskId?: string;
  aiMessageIds?: string[];
  toolCallIds?: string[];
  actionTraceIds?: string[];
  maxTokens?: number;
  budget?: number;
  maxItems?: number;
  maxSteps?: number;
  caseId?: string;
  harnessCaseId?: string;
  plannerPlanId?: string;
  queueSource?: string;
  queueStatus?: string;
  logPath?: string;
  cwd?: string;
  env?: Record<string, string>;
}

export interface AIActionRunRequest {
  actionId: string;
  input?: AIActionRunInput;
  arguments?: string;
  cwd?: string;
  env?: Record<string, string>;
}

export interface AIActionFileChange {
  path?: string;
  targetPath?: string;
  operation?: string;
  status?: string;
  summary?: string;
  patch?: string;
  diff?: string;
  source?: string;
  actionId?: string;
  messageId?: string;
  toolCallId?: string;
  createdAt?: string;
}

export interface AIActionResult {
  kind: string;
  operation?: string;
  path?: string;
  targetPath?: string;
  goal?: string;
  query?: string;
  file?: WorkspaceFileContent;
  fileContents?: WorkspaceFileContent[];
  files?: WorkspaceFile[];
  fileChanges?: AIActionFileChange[];
  fileDiffs?: HarnessFileDiff[];
  deletedFile?: WorkspaceFile;
  searchResults?: WorkspaceSearchResult[];
  patch?: string;
  evidenceItem?: EvidenceItem;
  evidenceItems?: EvidenceItem[];
  reports?: ReportDocument[];
  reportFinding?: ReportFinding;
  reportFindings?: ReportFinding[];
  executions?: ExecutionStep[];
  executionLogs?: ExecutionLog[];
  executionArtifacts?: ExecutionArtifact[];
  reportExport?: ReportExportResult;
  blackboardItems?: BlackboardItem[];
  blackboardItem?: BlackboardItem;
  agentState?: AgentState;
  agentProfile?: AgentProfile;
  agentTasks?: AgentTask[];
  agentTask?: AgentTask;
  agentTaskEvents?: AgentTaskEvent[];
  agentTaskEvent?: AgentTaskEvent;
  agentProviderRun?: AgentProviderRun;
  agentProviderBatchRun?: AgentProviderBatchRun;
  runtimeWorkerState?: RuntimeWorkerState;
  runtimeWorkers?: RuntimeWorker[];
  runtimeWorkerOrchestrations?: RuntimeWorkerOrchestration[];
  runtimeWorkerOrchestration?: RuntimeWorkerOrchestration;
  harnessState?: HarnessState;
  harnessCases?: HarnessCase[];
  harnessCase?: HarnessCase;
  harnessResults?: HarnessResult[];
  harnessResult?: HarnessResult;
  eventlogEvents?: EventlogEvents;
  aiMessageIds?: string[];
  toolCallIds?: string[];
  orchestratorState?: OrchestratorState;
  orchestratorRuns?: OrchestratorRun[];
  orchestratorRun?: OrchestratorRun;
  orchestratorObservations?: OrchestratorObservation[];
  orchestratorObservation?: OrchestratorObservation;
  orchestratorAdvances?: OrchestratorAdvance[];
  orchestratorAdvance?: OrchestratorAdvance;
  plannerState?: PlannerState;
  plannerPlans?: PlannerPlan[];
  plannerPlan?: PlannerPlan;
  frontierItems?: FrontierPriorityItem[];
  frontierSummary?: FrontierPrioritySummary;
  frontierDependencies?: FrontierDependencySelection;
  refreshedFrontierItems?: FrontierPriorityItem[];
  refreshedFrontierSummary?: FrontierPrioritySummary;
  budget?: number;
  promptRegistry?: PromptRegistry;
  prompt?: PromptDefinition;
  promptVersion?: PromptVersion;
  promptDiff?: PromptDiff;
  promptComparison?: PromptHarnessComparison;
  batchComparison?: PromptHarnessBatchComparison;
  provider?: string;
  model?: string;
  queueSource?: string;
  queueStatus?: string;
  usage?: AIUsage;
  output?: string;
  outputSummary?: string;
  count?: number;
  summary?: string;
}

export interface FrontierPriorityItem {
  id?: string;
  rank?: number;
  score?: number;
  title?: string;
  target?: string;
  hypothesis?: string;
  expectedUnlock?: string;
  reason?: string;
  suggestedAction?: string;
  ready?: boolean;
  dependencyIds?: string[];
  blockedByIds?: string[];
  blockedReason?: string;
  status?: string;
  priority?: string;
  source?: string;
  sourceKind?: string;
  sourceId?: string;
  blackboardItemId?: string;
  agentTaskId?: string;
  filePath?: string;
  executionId?: string;
  missionValue?: number;
  unlockPotential?: number;
  evidenceConfidence?: number;
  novelty?: number;
  cost?: number;
  duplicateRisk?: number;
  noise?: number;
  exhaustedPenalty?: number;
  metadata?: Record<string, unknown>;
}

export interface FrontierPrioritySummary {
  goal?: string;
  source?: string;
  statusFilter?: string;
  priorityFilter?: string;
  candidateCount?: number;
  totalCandidates?: number;
  selectedCount?: number;
  skippedCount?: number;
  readyFilter?: string;
  readyCount?: number;
  blockedCount?: number;
  budget?: number;
  maxItems?: number;
  budgetUsed?: number;
  budgetRemaining?: number;
  exhaustedCount?: number;
  duplicateCount?: number;
  openBlackboard?: number;
  queuedTasks?: number;
  summary?: string;
  metadata?: Record<string, unknown>;
}

export interface FrontierQueueQuery {
  goal?: string;
  status?: string;
  priority?: string;
  source?: string;
  q?: string;
  readyFilter?: 'all' | 'ready' | 'blocked';
  budget?: number;
  maxItems?: number;
}

export interface FrontierQueue {
  path?: string;
  queueSource?: string;
  queueStatus?: string;
  items: FrontierPriorityItem[];
  summary?: FrontierPrioritySummary;
  blackboardItems?: BlackboardItem[];
  agentTasks?: AgentTask[];
  agentState?: AgentState;
  count: number;
}

export interface FrontierMonitorSummary {
  status?: string;
  source?: string;
  queueStatus?: string;
  queueItems: number;
  readyItems: number;
  blockedItems: number;
  activeAgentTasks: number;
  activePlannerPlans: number;
  runningExecutions: number;
  recentEvents: number;
  lastUpdatedAt?: number;
  summary?: string;
}

export interface FrontierMonitor {
  queue: FrontierQueue;
  summary: FrontierMonitorSummary;
  agentState?: AgentState;
  plannerState?: PlannerState;
  activeAgentTasks: AgentTask[];
  activePlannerPlans: PlannerPlan[];
  runningExecutions: ExecutionStep[];
  recentAgentEvents: AgentTaskEvent[];
}

export interface FrontierDependencySummary {
  goal?: string;
  source?: string;
  status?: string;
  queueStatus?: string;
  queueItems: number;
  readyItems: number;
  blockedItems: number;
  clusterCount: number;
  candidateCount: number;
  selectedCount: number;
  blockedFrontierItemCount: number;
  missingDependencyCount: number;
  satisfiedDependencyCount: number;
  readyDependencyCount: number;
  linkedAgentTaskCount: number;
  unlockValue: number;
  maxItems?: number;
  budget?: number;
  summary?: string;
}

export interface FrontierDependencyCandidate {
  rank: number;
  id: string;
  dependencyId: string;
  dependencyTitle?: string;
  dependencyStatus?: string;
  dependencyReady: boolean;
  dependencySatisfied: boolean;
  missingDependency: boolean;
  unlockCount: number;
  unlockValue: number;
  score: number;
  suggestedActionId?: string;
  suggestedAction?: string;
  suggestedBlackboardItemId?: string;
  suggestedAgentTaskId?: string;
  suggestedGoal?: string;
  reason?: string;
}

export interface FrontierDependencyCluster extends FrontierDependencyCandidate {
  dependency?: BlackboardItem;
  dependencyFound: boolean;
  blockedItems: FrontierPriorityItem[];
  blockedBlackboardItems: BlackboardItem[];
  linkedAgentTasks: AgentTask[];
}

export interface FrontierDependencySelection {
  path?: string;
  queueSource?: string;
  status?: string;
  queue: FrontierQueue;
  summary: FrontierDependencySummary;
  clusters: FrontierDependencyCluster[];
  candidates: FrontierDependencyCandidate[];
  blackboardItems?: BlackboardItem[];
  agentTasks?: AgentTask[];
  agentState?: AgentState;
  count: number;
}

export interface AIActionRunResponse {
  action: AIAction;
  execution?: ExecutionStep;
  result: AIActionResult;
  message: AIMessage;
  conversation: AIConversation;
  frontierQueue?: FrontierQueue;
}

export type Severity = 'critical' | 'high' | 'medium' | 'low' | 'info' | 'unknown';

export interface WorkspaceItem {
  id: string;
  label: string;
  meta: string;
  active?: boolean;
  filePath?: string;
  kind?: string;
  size?: number;
  modifiedAt?: number;
}

export interface WorkspaceFile {
  id: string;
  path: string;
  name: string;
  kind: string;
  size?: number;
  modifiedAt?: number;
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
  apiKeyValue: string;
  apiKeyConfigured?: boolean;
  enabled: boolean;
  timeoutMs: number;
  contextWindowTokens: number;
  capabilities: string[];
  routingStrategy: string;
  status: string;
}

export interface ProviderConfig {
  path: string;
  routingStrategy: string;
  providers: ProviderSettings[];
  active: ProviderSettings;
}

export interface ProviderTestResult {
  provider: string;
  model: string;
  status: string;
  content?: string;
  latencyMs?: number;
  usage?: AIUsage;
  attempts?: AIProviderAttempt[];
  config?: ProviderConfig;
}

export interface HarnessProviderSnapshot {
  configPath?: string;
  routingStrategy?: string;
  status?: string;
  error?: string;
  active?: HarnessProviderSummary;
  providers: HarnessProviderSummary[];
}

export interface HarnessProviderSummary {
  name?: string;
  type?: string;
  model?: string;
  baseUrl?: string;
  apiKeyEnv?: string;
  enabled?: boolean;
  timeoutMs?: number;
  contextWindowTokens?: number;
  capabilities: string[];
  status?: string;
}

export interface ProviderUsageSummary {
  recordCount: number;
  providers: string[];
  models: string[];
  inputTokens: number;
  outputTokens: number;
  totalTokens: number;
  estimatedCostUsd: number;
  estimatedCostRecordCount: number;
  budgetEnforcedCount: number;
  budgetWithinLimitCount: number;
  firstCreatedAt?: string;
  lastCreatedAt?: string;
}

export interface ProviderUsageRecord {
  schemaVersion: number;
  path: string;
  recordId: string;
  source: string;
  createdAt: string;
  providerName: string;
  model: string;
  inputTokens: number;
  outputTokens: number;
  totalTokens: number;
  contextWindowTokens?: number;
  capabilities: string[];
  estimatedCostUsd?: number;
  estimatedCostAvailable: boolean;
  budgetEnforced?: boolean;
  budgetStatus?: string;
  budgetMaxEstimatedCostUsd?: number;
}

export interface ProviderUsageState {
  path: string;
  summary: ProviderUsageSummary;
  records: ProviderUsageRecord[];
}

export interface PromptRegistry {
  version: number;
  path: string;
  prompts: PromptDefinition[];
}

export interface PromptDefinition {
  id: string;
  title: string;
  kind: string;
  activeVersion: string;
  versions: PromptVersion[];
}

export interface PromptVersion {
  id: string;
  title: string;
  content: string;
  providerName?: string;
  model?: string;
  variables: PromptVariable[];
}

export interface PromptVariable {
  name: string;
  description?: string;
  required?: boolean;
  default?: string;
  source?: string;
}

export interface PromptDiff {
  promptId: string;
  baseVersionId: string;
  targetVersionId: string;
  baseTitle?: string;
  targetTitle?: string;
  baseContent?: string;
  targetContent?: string;
  diff: string;
  addedLines: number;
  removedLines: number;
  changed: boolean;
}

export interface PromptHarnessComparison {
  promptId?: string;
  baseVersionId?: string;
  targetVersionId?: string;
  baseProvider?: string;
  targetProvider?: string;
  baseModel?: string;
  targetModel?: string;
  baseUsage?: AIUsage;
  targetUsage?: AIUsage;
  totalTokens?: number;
  baseOutput?: string;
  targetOutput?: string;
  baseOutputSummary?: string;
  targetOutputSummary?: string;
  baseResultId?: string;
  targetResultId?: string;
  baseStatus?: string;
  targetStatus?: string;
  baseResult?: HarnessResult;
  targetResult?: HarnessResult;
}

export interface PromptHarnessBatchComparisonItem {
  versionId?: string;
  version?: PromptVersion | string;
  resultId?: string;
  result?: HarnessResult;
  provider?: string;
  model?: string;
  usage?: AIUsage;
  totalTokens?: number;
  costUsd?: number;
  costKnown?: boolean;
  status?: string;
  output?: string;
  outputSummary?: string;
  summary?: string;
}

export interface PromptHarnessBatchComparison {
  promptId?: string;
  prompt?: PromptDefinition;
  versionIds?: string[];
  versions?: Array<PromptVersion | string>;
  results?: HarnessResult[];
  items: PromptHarnessBatchComparisonItem[];
  sameProvider?: boolean;
  sameModel?: boolean;
  providers?: string[];
  models?: string[];
  totalTokens?: number;
  totalCostUsd?: number;
  costKnown?: boolean;
  summary?: string;
}

export interface ToolchainDependency {
  name: string;
  kind: string;
  required: boolean;
  status: string;
  ready: boolean;
  version: string;
  installedVersion: string;
  latestVersion: string;
  source: string;
  imageDigest: string;
  latestImageDigest: string;
  metadataSource: string;
  optional: boolean;
  dependsOn: string[];
  message: string;
  installHint: string;
  installCommand: string[];
}

export interface ToolchainPackSettings {
  id: string;
  name: string;
  version: string;
  status: string;
  toolsReady: number;
  manifestPath: string;
  manifestYaml: string;
  capabilities: string[];
  runner: string;
  runCommand: string[];
  runTimeoutMs: number;
  parser: string;
  healthcheck: string;
  healthCommand: string[];
  dependencies: ToolchainDependency[];
}

export interface HarnessToolchainSnapshot {
  status?: string;
  error?: string;
  toolsReady?: number;
  packs: HarnessToolchainPackSnapshot[];
  actions: HarnessToolchainActionSnapshot[];
}

export interface HarnessToolchainPackSnapshot {
  id?: string;
  name?: string;
  version?: string;
  status?: string;
  toolsReady?: number;
  manifestPath?: string;
  capabilities: string[];
  runner?: string;
  runCommand: string[];
  runTimeoutMs?: number;
  parser?: string;
  healthcheck?: string;
  healthCommand: string[];
  dependencyCount?: number;
  dependencies: HarnessToolchainDependencySnapshot[];
}

export interface HarnessToolchainDependencySnapshot {
  name?: string;
  kind?: string;
  version?: string;
  source?: string;
  optional?: boolean;
  dependsOn: string[];
  installHint?: string;
  installCommand: string[];
}

export interface HarnessToolchainActionSnapshot {
  id?: string;
  packId?: string;
  packName?: string;
  label?: string;
  status?: string;
  command: string[];
  commandText?: string;
  runTimeoutMs?: number;
  manifestPath?: string;
  capabilities: string[];
  runner?: string;
  parser?: string;
  argumentHint?: string;
}

export interface ToolAction {
  id: string;
  source: string;
  kind: string;
  toolId: string;
  category: string;
  adapter: string;
  operation: string;
  binary: string;
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
  targetKind: string;
  targetKinds: string[];
  dependencyName: string;
  dependencyKind: string;
  dependencyStatus: string;
  dependencyReady: boolean;
  dependencyVersion: string;
  dependencyMessage: string;
  dependencySource: string;
  argumentHint: string;
}

export interface ToolActionRunRequest {
  target?: string;
  targetKind?: string;
  args?: string[];
  arguments?: string;
}

export interface GraphNode {
  id: string;
  label: string;
  subtitle: string;
  kind: 'target' | 'surface' | 'finding' | 'identity' | 'action' | 'artifact' | 'unknown';
  rawKind: string;
  severity?: Severity;
  rawSeverity?: string;
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
  status: 'queued' | 'running' | 'ready' | 'complete' | 'failed' | 'stopped' | 'unknown';
  rawStatus?: string;
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
  lastEvent?: string;
  recentWorkspaces?: string[];
  runtime?: DesktopRuntimeInfo;
  windowState?: {
    x?: number;
    y?: number;
    width?: number;
    height?: number;
    maximized?: boolean;
  };
}

export interface DesktopRuntimeInfo {
  isPackaged?: boolean;
  platform?: string;
  arch?: string;
  appVersion?: string;
  appPath?: string;
  executablePath?: string;
  resourcesPath?: string;
  userDataPath?: string;
  backendBinaryPath?: string;
  backendBinaryExists?: boolean;
  backendWorkingDirectory?: string;
  backendWorkingDirectoryExists?: boolean;
  rendererPath?: string;
  rendererExists?: boolean;
  preloadPath?: string;
  preloadExists?: boolean;
  diagnosticLogPath?: string;
  diagnosticLogExists?: boolean;
  desktopRuntimeStatePath?: string;
  desktopRuntimeSessionId?: string;
  desktopRuntimeStartedAt?: string;
  lastDesktopRuntimeIssue?: DesktopRuntimeIssue;
  processId?: number;
  lastCommandLine?: string;
  lastExitCode?: number | null;
  lastExitSignal?: string;
  lastStartedAt?: string;
  lastHealthError?: string;
  lastStdout?: string;
  lastStderr?: string;
  lastLog?: string;
  lastEvent?: string;
}

export interface DesktopRuntimeIssue {
  event?: string;
  recordedAt?: string;
  reason?: string;
  type?: string;
  serviceName?: string;
  name?: string;
  message?: string;
  exitCode?: number | null;
  errorCode?: number | null;
  errorDescription?: string;
  validatedURL?: string;
  isMainFrame?: boolean | null;
  windowId?: number;
  processId?: number;
  previousSessionId?: string;
  previousProcessId?: number;
  previousStartedAt?: string;
  previousHeartbeatAt?: string;
  previousWorkspace?: string;
  previousStatus?: string;
  statePath?: string;
}

export interface ReplayEvent {
  id: string;
  rawEventId?: number;
  runId?: string;
  type?: string;
  lane?: string;
  createdAt?: number;
  time: string;
  event: string;
  detail: string;
  payloadJson?: string;
  nodeId?: string;
  executionId?: string;
  filePath?: string;
}

export interface EventlogRun {
  id: string;
  status: string;
  startedAt?: number;
  time: string;
}

export interface EventlogEvents {
  runs: EventlogRun[];
  events: ReplayEvent[];
  count: number;
  runId?: string;
  type?: string;
  lane?: string;
  limit?: number;
}

export interface EventlogQuery {
  runId?: string;
  type?: string;
  lane?: string;
  limit?: number;
}

export interface ReportFinding {
  id: string;
  title: string;
  severity: Severity;
  rawSeverity?: string;
  endpoint: string;
  description?: string;
  recommendation?: string;
  evidence?: string;
  path?: string;
  source?: string;
  nodeId?: string;
  filePath?: string;
  runId?: string;
  executionId?: string;
  replayEventId?: string;
  agentTaskId?: string;
  blackboardItemId?: string;
  promptId?: string;
  promptVersionId?: string;
  aiMessageIds?: string[];
  toolCallIds?: string[];
  actionTraceIds?: string[];
}

export interface ReportDocument {
  id: string;
  title: string;
  status: string;
  path: string;
  size?: number;
  modifiedAt?: number;
}

export interface ReportFileContent {
  report: ReportDocument;
  file: WorkspaceFileContent;
}

export interface ReportExportResult {
  filePath: string;
  findingCount: number;
  reportCount: number;
  modifiedAt: number;
}

export interface EvidenceItem {
  id: string;
  title: string;
  summary: string;
  severity?: Severity;
  rawSeverity?: string;
  source?: string;
  path?: string;
  filePath?: string;
  artifactPath?: string;
  executionId?: string;
  graphNodeId?: string;
  replayEventId?: string;
  agentTaskId?: string;
  blackboardItemId?: string;
  promptId?: string;
  promptVersionId?: string;
  aiMessageIds?: string[];
  toolCallIds?: string[];
  actionTraceIds?: string[];
  createdAt?: number;
}

export interface BlackboardState {
  path: string;
  items: BlackboardItem[];
  observations: BlackboardObservation[];
}

// BlackboardObservation 是判定前的原始观测：工具回传的原始测量
// （http_observe 输出、端口探测时延等），模型推理后才升级为结论条目。
export interface BlackboardObservation {
  id: string;
  kind: string;
  target: string;
  payload?: string;
  source?: string;
  createdAt: number;
}

export interface BlackboardObservationQuery {
  kind?: string;
  target?: string;
  source?: string;
  q?: string;
  limit?: number;
}

export interface BlackboardObservationState {
  path: string;
  observations: BlackboardObservation[];
  count: number;
}

export interface BlackboardItem {
  id: string;
  kind: string;
  title: string;
  content: string;
  status: string;
  priority?: string;
  source?: string;
  dependencyIds?: string[];
  dependentIds?: string[];
  filePath?: string;
  executionId?: string;
  graphNodeId?: string;
  replayEventId?: string;
  reportId?: string;
  createdAt: number;
  updatedAt: number;
}

export interface BlackboardItemCreateRequest {
  title: string;
  content: string;
  kind?: string;
  status?: string;
  priority?: string;
  source?: string;
  dependencyIds?: string[];
  dependentIds?: string[];
  filePath?: string;
  executionId?: string;
  graphNodeId?: string;
  replayEventId?: string;
  reportId?: string;
}

export interface BlackboardItemUpdateRequest {
  title?: string;
  content?: string;
  kind?: string;
  status?: string;
  priority?: string;
  source?: string;
  dependencyIds?: string[];
  dependentIds?: string[];
  filePath?: string;
  executionId?: string;
  graphNodeId?: string;
  replayEventId?: string;
  reportId?: string;
}

export interface BlackboardQuery {
  status?: string;
  priority?: string;
  kind?: string;
  source?: string;
  q?: string;
}

export interface BlackboardBatchUpdateRequest {
  ids: string[];
  update: BlackboardItemUpdateRequest;
}

export interface BlackboardBatchUpdateResult extends BlackboardState {
  count: number;
}

export interface BlackboardDependencyLinkRequest {
  dependencyIds: string[];
}

export interface BlackboardDependencyLinkResult {
  path: string;
  count: number;
  item: BlackboardItem;
  dependencies: BlackboardItem[];
}

export type AgentCounts = Record<string, number>;

export interface AgentProfile {
  id: string;
  name: string;
  role: string;
  description: string;
  instructions: string;
  capabilities: string[];
  providerName: string;
  model: string;
  promptId: string;
  promptVersionId: string;
  createdAt: number;
  updatedAt: number;
}

export interface AgentProfileCreateRequest {
  name: string;
  role: string;
  description?: string;
  instructions?: string;
  capabilities?: string[];
  providerName?: string;
  model?: string;
  promptId?: string;
  promptVersionId?: string;
}

export interface AgentProfileUpdateRequest {
  name?: string;
  role?: string;
  description?: string;
  instructions?: string;
  capabilities?: string[];
  providerName?: string;
  model?: string;
  promptId?: string;
  promptVersionId?: string;
}

export interface AgentState {
  path: string;
  eventPath: string;
  profilePath: string;
  profiles: AgentProfile[];
  tasks: AgentTask[];
  events: AgentTaskEvent[];
  counts: AgentCounts;
}

export interface AgentTask {
  id: string;
  title: string;
  goal: string;
  role: string;
  agentId?: string;
  status: string;
  priority?: string;
  source?: string;
  createdAt: number;
  updatedAt: number;
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

export interface AgentTaskCreateRequest {
  title: string;
  goal: string;
  role: string;
  agentId?: string;
  priority?: string;
  source?: string;
  blackboardItemId?: string;
  filePath?: string;
  artifactPath?: string;
  executionId?: string;
  replayEventId?: string;
  reportFindingId?: string;
}

export interface AgentTaskUpdateRequest {
  id?: string;
  title?: string;
  goal?: string;
  role?: string;
  agentId?: string;
  status?: string;
  priority?: string;
  source?: string;
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
  executionId?: string;
  replayEventId?: string;
  reportFindingId?: string;
}

export interface AgentTaskEvent {
  id: string;
  taskId: string;
  kind: string;
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
  createdAt: number;
}

export interface AgentTaskEventCreateRequest {
  id?: string;
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
  executionId?: string;
  replayEventId?: string;
  reportFindingId?: string;
}

export interface AgentProviderRunRequest {
  providerName?: string;
  model?: string;
  maxTokens?: number;
  source?: string;
}

export interface AgentProviderBatchRunRequest {
  taskIds?: string[];
  providerName?: string;
  model?: string;
  maxTokens?: number;
  source?: string;
}

export interface AgentProviderRun {
  runId: string;
  status: string;
  agentProfileId?: string;
  agentProfileName?: string;
  agentProfileRole?: string;
  agentProfileDescription?: string;
  agentProfileInstructions?: string;
  agentProfileCapabilities?: string[];
  error?: string;
  output?: string;
  startedAt?: string;
  completedAt?: string;
  durationMs?: number;
  providerName?: string;
  model?: string;
  promptId?: string;
  promptVersionId?: string;
  inputTokens?: number;
  outputTokens?: number;
  totalTokens?: number;
  estimatedCostUsd?: number;
  estimatedCostAvailable: boolean;
  artifactPath?: string;
  eventlogRunId?: string;
  task?: AgentTask;
  event?: AgentTaskEvent;
  startedEvent?: AgentTaskEvent;
  blackboardItem?: BlackboardItem;
  providerAttempts?: AIProviderAttempt[];
}

export interface AgentProviderBatchRun {
  batchId: string;
  status: string;
  error?: string;
  taskIds: string[];
  requestedCount: number;
  completedCount: number;
  failedCount: number;
  startedAt?: string;
  completedAt?: string;
  durationMs: number;
  artifactPath: string;
  eventlogRunId: string;
  runs: AgentProviderRun[];
}

export interface HarnessState {
  path: string;
  resultPath: string;
  cases: HarnessCase[];
  results: HarnessResult[];
  counts: HarnessCounts;
}

export interface HarnessCase {
  id: string;
  title: string;
  goal: string;
  kind: string;
  command: string;
  expectedResult: string;
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
  providerSnapshot?: HarnessProviderSnapshot;
  toolchainSnapshot?: HarnessToolchainSnapshot;
  createdAt: number;
  updatedAt: number;
}

export interface HarnessResult {
  id: string;
  caseId: string;
  caseTitle: string;
  status: string;
  error?: string;
  command: string;
  expectedResult: string;
  runId?: string;
  executionId?: string;
  startedAt: number;
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
  aiMessageIds: string[];
  toolCallIds: string[];
  fileDiffs: HarnessFileDiff[];
  eventlogReferences: HarnessEventlogReference[];
  references: AIContextReference[];
  provider?: string;
  model?: string;
  inputTokens?: number;
  outputTokens?: number;
  totalTokens?: number;
  latencyMs?: number;
  costUsd?: number;
  costKnown?: boolean;
  usage?: AIUsage;
  output?: string;
  outputSummary?: string;
  providerAttempts?: AIProviderAttempt[];
}

export interface HarnessFileDiff {
  path?: string;
  operation?: string;
  summary?: string;
  diff?: string;
  actionId?: string;
  messageId?: string;
  toolCallId?: string;
}

export interface HarnessEventlogReference {
  id?: string;
  rawEventId?: string | number;
  replayEventId?: string;
  eventType?: string;
  lane?: string;
  summary?: string;
}

export interface HarnessCounts {
  totalCases: number;
  totalResults: number;
  running: number;
  passed: number;
  failed: number;
}

export interface HarnessCaseCreateRequest {
  title: string;
  goal: string;
  kind?: string;
  command: string;
  expectedResult: string;
  promptId?: string;
  promptVersionId?: string;
  versionId?: string;
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
  providerSnapshot?: HarnessProviderSnapshot;
  toolchainSnapshot?: HarnessToolchainSnapshot;
}

export type OrchestratorCounts = Record<string, number>;

export interface OrchestratorState {
  path: string;
  observationPath: string;
  advancePath: string;
  runs: OrchestratorRun[];
  observations: OrchestratorObservation[];
  advances: OrchestratorAdvance[];
  counts: OrchestratorCounts;
}

export interface OrchestratorRun {
  id: string;
  title: string;
  goal: string;
  strategy: string;
  status: string;
  command?: string;
  expectedResult?: string;
  priority?: string;
  source?: string;
  filePath?: string;
  blackboardItemId?: string;
  agentTaskId?: string;
  executionId?: string;
  harnessCaseId?: string;
  createdAt: number;
  updatedAt: number;
}

export interface OrchestratorObservation {
  id: string;
  runId: string;
  executionId: string;
  status: string;
  exitCode?: number;
  logPath?: string;
  artifactDir?: string;
  artifactCount: number;
  logPreview?: string;
  summary?: string;
  blackboardItemId?: string;
  agentTaskId?: string;
  harnessResultId?: string;
  autoParsedEvidenceItems?: EvidenceItem[];
  autoParsedReportFindings?: ReportFinding[];
  autoParsedEvidenceItemIds?: string[];
  autoParsedReportFindingIds?: string[];
  autoParsedEvidenceCount?: number;
  autoParsedFindingCount?: number;
  parsedSource?: string;
  parsedBatchTitle?: string;
  createdAt: number;
  updatedAt: number;
}

export interface OrchestratorAdvance {
  id: string;
  runId: string;
  observationId?: string;
  parentExecutionId?: string;
  executionId: string;
  title: string;
  goal: string;
  strategy: string;
  status: string;
  command?: string;
  expectedResult?: string;
  priority?: string;
  source?: string;
  filePath?: string;
  blackboardItemId?: string;
  agentTaskId?: string;
  harnessCaseId?: string;
  autoParsedEvidenceItems?: EvidenceItem[];
  autoParsedReportFindings?: ReportFinding[];
  autoParsedEvidenceItemIds?: string[];
  autoParsedReportFindingIds?: string[];
  autoParsedEvidenceCount?: number;
  autoParsedFindingCount?: number;
  createdAt: number;
  updatedAt: number;
}

export type PlannerCounts = Record<string, number>;

export interface PlannerState {
  path: string;
  plans: PlannerPlan[];
  counts: PlannerCounts;
}

export interface PlannerPlan {
  id: string;
  title: string;
  goal: string;
  status: string;
  priority?: string;
  source?: string;
  filePath?: string;
  contextSummary?: string;
  nextActionTitle: string;
  nextActionGoal: string;
  nextActionCommand: string;
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
  createdAt: number;
  updatedAt: number;
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
  providerConfig: ProviderConfig;
  providerUsage: ProviderUsageState;
  toolchainPacks: ToolchainPackSettings[];
  toolActions: ToolAction[];
  workspaceItems: WorkspaceItem[];
  graphNodes: GraphNode[];
  graphEdges: GraphEdge[];
  executionSteps: ExecutionStep[];
  replayEvents: ReplayEvent[];
  evidenceItems: EvidenceItem[];
  reportFindings: ReportFinding[];
  reports: ReportDocument[];
  blackboard: BlackboardState;
  agentState: AgentState;
  frontierQueue: FrontierQueue;
  harness: HarnessState;
  orchestratorState: OrchestratorState;
  plannerState: PlannerState;
  aiConversation: AIConversation | null;
  status: StatusSummary;
  loadedAt: string;
}
