import { existsSync, readFileSync } from 'node:fs';
import { join } from 'node:path';

const root = new URL('..', import.meta.url).pathname;

const requiredFiles = [
  'src/app/OuroborosIDE.tsx',
  'src/app/api.ts',
  'src/app/types.ts',
  'src/app/zh-cn.ts',
  'src/styles/ouroboros-ide.css',
];

const forbiddenPaths = [
  'backup-old-implementation',
  'src/app/fixtures.ts',
  'src/router.tsx',
  'src/components',
  'src/stores',
  'src/hooks',
  'src/lib',
];

let failed = false;

for (const file of requiredFiles) {
  if (!existsSync(join(root, file))) {
    console.error(`missing required GUI rebuild file: ${file}`);
    failed = true;
  }
}

for (const path of forbiddenPaths) {
  if (existsSync(join(root, path))) {
    console.error(`old GUI implementation path must be removed: ${path}`);
    failed = true;
  }
}

const appPath = join(root, 'src/renderer/App.tsx');
if (!existsSync(appPath)) {
  console.error('missing renderer App.tsx entry');
  failed = true;
} else {
  const app = readFileSync(appPath, 'utf8');
  if (!app.includes('../app/OuroborosIDE')) {
    console.error('renderer App.tsx must import the rebuilt OuroborosIDE shell');
    failed = true;
  }
}

const shellPath = join(root, 'src/app/OuroborosIDE.tsx');
const shellSource = existsSync(shellPath) ? readFileSync(shellPath, 'utf8') : '';
if (existsSync(shellPath)) {
  const shell = shellSource;
  const requiredImports = ['loadIDEData', 'loadDesktopStatus', 'restartDesktopBackend', 'zhCN'];
  for (const imported of requiredImports) {
    if (!shell.includes(imported)) {
      console.error(`OuroborosIDE.tsx must use ${imported}`);
      failed = true;
    }
  }
  const forbiddenUiText = [
    'Mission',
    'Attack Graph',
    'Replay',
    'Report',
    'Settings',
    'Command Palette',
    'Workspace',
    'Start Pentest',
    'AI Conversation',
    'Tool Execution',
    'Inspector',
    'CONNECTED',
    'Safety',
    'Authorization',
    'Scope',
  ];
  const escapeRegExp = value => value.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
  for (const text of forbiddenUiText) {
    const literal = new RegExp(`["'\`]${escapeRegExp(text)}["'\`]`);
    const jsxText = new RegExp(`>${escapeRegExp(text)}<`);
    if (literal.test(shell) || jsxText.test(shell)) {
      console.error(`frontend UI text must be Chinese/centralized, found: ${text}`);
      failed = true;
    }
  }
  if (shell.includes('ideFixture')) {
    console.error('OuroborosIDE.tsx must not use fixture data');
    failed = true;
  }
  for (const snippet of ['startExecution', 'stopExecution', 'retryExecution', 'loadExecutionLogs', 'loadExecutionArtifacts', 'runToolchainAction']) {
    if (!shell.includes(snippet)) {
      console.error(`OuroborosIDE.tsx must wire real execution action: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['ToolLaunchPanel', 'toolActions', 'toolActionArguments', 'zhCN.toolLaunch.title', 'zhCN.toolLaunch.run']) {
    if (!shell.includes(snippet)) {
      console.error(`OuroborosIDE.tsx must expose manifest-backed tool launch UI: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['CommandPalette', 'paletteActions', 'onRunCommand', 'onSaveMission', 'onStartMission', 'onRefresh', 'onOpenWorkspace', 'onCreateWorkspace', 'onRestartBackend', 'zhCN.commandPalette.refresh', 'zhCN.commandPalette.restartBackend', 'keydown', 'ctrlKey', 'metaKey']) {
    if (!shell.includes(snippet)) {
      console.error(`OuroborosIDE.tsx must expose real command palette action: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['openRecentDesktopWorkspace', 'onOpenRecentWorkspace', 'recentWorkspaces', 'zhCN.commandPalette.openRecentWorkspace', 'recentWorkspaces.map']) {
    if (!shell.includes(snippet)) {
      console.error(`OuroborosIDE.tsx must expose recent workspace command palette actions: ${snippet}`);
      failed = true;
    }
  }
  if (shell.includes('placeholder={zhCN.shell.commandPalette} readOnly')) {
    console.error('CommandPalette input must be editable search, not a read-only placeholder');
    failed = true;
  }
  if (!shell.includes('connectIDEEventStream')) {
    console.error('OuroborosIDE.tsx must subscribe to the backend event stream');
    failed = true;
  }
  for (const snippet of ['WorkspaceWelcome', 'onCreateWorkspace', 'createWorkspace']) {
    if (!shell.includes(snippet)) {
      console.error(`OuroborosIDE.tsx must expose real workspace onboarding action: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['MissionLaunch', 'onSaveMission', 'onStartMission']) {
    if (!shell.includes(snippet)) {
      console.error(`OuroborosIDE.tsx must expose direct mission workflow: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['onSaveProvider', 'saveProviderConfig', 'zhCN.settings.actions.saveProvider']) {
    if (!shell.includes(snippet)) {
      console.error(`OuroborosIDE.tsx must expose editable provider settings: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['onSaveToolchain', 'saveToolchainPack', 'zhCN.settings.actions.saveToolchain']) {
    if (!shell.includes(snippet)) {
      console.error(`OuroborosIDE.tsx must expose editable toolchain settings: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['onSaveWorkspaceSettings', 'saveWorkspaceSettings', 'zhCN.settings.actions.saveWorkspace']) {
    if (!shell.includes(snippet)) {
      console.error(`OuroborosIDE.tsx must expose editable workspace settings: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['onOpenFindingFile', 'onFocusGraphNode', 'onFocusReplayEvent', 'zhCN.navigation.focusGraph', 'zhCN.navigation.openEvidenceFile']) {
    if (!shell.includes(snippet)) {
      console.error(`OuroborosIDE.tsx must expose graph/replay/report cross navigation: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['GraphDetailPanel', 'ReplayDetailPanel', 'ReportFindingDetail', 'EvidenceFileDetail', 'ExecutionDetailPanel']) {
    if (!shell.includes(snippet)) {
      console.error(`OuroborosIDE.tsx must render real detail view component: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['onOpenExecutionFile', 'zhCN.execution.openLogFile', 'zhCN.execution.openArtifactFile', 'artifact.filePath']) {
    if (!shell.includes(snippet)) {
      console.error(`OuroborosIDE.tsx must open execution logs/artifacts as real workspace files: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['ExecutionReferenceTargets', 'selectedReportFindingId', 'onFocusReportFinding', 'zhCN.navigation.focusReport', 'finding.executionId === activeStep.id', 'event.executionId === activeStep.id', 'node.executionId === activeStep.id']) {
    if (!shell.includes(snippet)) {
      console.error(`OuroborosIDE.tsx must expose execution detail cross references from backend DTOs: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['executionArtifactsExecutionId', 'findingExecution', 'findingArtifacts', 'execution={findingExecution}', 'artifacts={findingArtifacts}', 'onLoadExecutionArtifacts={onLoadExecutionArtifacts}', 'artifact.filePath as string']) {
    if (!shell.includes(snippet)) {
      console.error(`OuroborosIDE.tsx must expose report finding detail execution file jumps from backend DTOs: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['replayExecution', 'replayFinding', 'replayArtifacts', 'finding.replayEventId === selectedEvent?.id', 'execution={replayExecution}', 'finding={replayFinding}', 'onFocusReportFinding={onFocusReportFinding}', 'onOpenExecutionFile={onOpenExecutionFile}']) {
    if (!shell.includes(snippet)) {
      console.error(`OuroborosIDE.tsx must expose replay detail report/execution file jumps from backend DTOs: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['graphExecution', 'graphFinding', 'graphArtifacts', 'finding.nodeId === selectedNode?.id', 'finding.executionId === selectedNode?.executionId', 'execution.id === graphFinding?.executionId', 'execution={graphExecution}', 'finding={graphFinding}', 'onFocusReportFinding={onFocusReportFinding}', 'executions={ideData.executionSteps}']) {
    if (!shell.includes(snippet)) {
      console.error(`OuroborosIDE.tsx must expose graph detail report/execution file jumps from backend DTOs: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['exportEvidenceGraph', 'onExportGraph', 'graphExportStatus', 'zhCN.graph.exported', 'onClick={onExportGraph}']) {
    if (!shell.includes(snippet)) {
      console.error(`OuroborosIDE.tsx must wire graph export button to a real backend export action: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['exportReport', 'onExportReport', 'reportExportStatus', 'zhCN.report.exported', 'onClick={onExportReport}']) {
    if (!shell.includes(snippet)) {
      console.error(`OuroborosIDE.tsx must wire report export button to a real backend export action: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of [
    'finding.filePath || finding.path || finding.source || finding.id',
    'finding.executionId || finding.replayEventId || finding.nodeId || zhCN.details.empty',
  ]) {
    if (shell.includes(snippet)) {
      console.error(`OuroborosIDE.tsx must not synthesize report appendix evidence text from technical fallbacks: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['reportActiveSection', 'reportSections', 'onSelectReportSection', 'ReportSectionContent', 'section.id === reportActiveSection', 'data-report-section={section}', 'workspace={ideData.workspace}']) {
    if (!shell.includes(snippet)) {
      console.error(`OuroborosIDE.tsx must make report TOC buttons switch real report sections: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['settingsActiveSection', 'settingsSections', 'onSelectSettingsSection', 'SettingsSectionContent', 'section.id === settingsActiveSection', 'data-settings-section={section}']) {
    if (!shell.includes(snippet)) {
      console.error(`OuroborosIDE.tsx must make settings navigation switch real settings sections: ${snippet}`);
      failed = true;
    }
  }
  if (shell.includes("className={index === 0 ? 'active' : ''}>{item}</button>")) {
    console.error('Settings navigation must not keep a fixed first active button');
    failed = true;
  }
  for (const snippet of ['zhCN.mission.preview', 'composerPlaceholder} readOnly']) {
    if (shell.includes(snippet)) {
      console.error(`OuroborosIDE.tsx still contains placeholder execution UI: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['defaultValue="toolchains/managed-bundles"', 'defaultValue="127.0.0.1"', 'defaultValue="8080"', 'defaultValue="70"']) {
    if (shell.includes(snippet)) {
      console.error(`OuroborosIDE.tsx contains hardcoded settings default: ${snippet}`);
      failed = true;
    }
  }
  for (const pattern of [
    { label: 'emptyData provider type', regex: /provider:\s*\{[\s\S]*?type:\s*'openai_compatible'/ },
    { label: 'emptyData provider enabled', regex: /provider:\s*\{[\s\S]*?enabled:\s*true/ },
    { label: 'emptyData provider routing', regex: /provider:\s*\{[\s\S]*?routingStrategy:\s*'first_enabled'/ },
    { label: 'emptyToolchainPack runner', regex: /function emptyToolchainPack\(\)[\s\S]*?runner:\s*'native'/ },
    { label: 'emptyToolchainPack healthcheck', regex: /function emptyToolchainPack\(\)[\s\S]*?healthcheck:\s*'built_in'/ },
  ]) {
    if (pattern.regex.test(shell)) {
      console.error(`OuroborosIDE.tsx must not fabricate settings in empty IDE data: ${pattern.label}`);
      failed = true;
    }
  }
  for (const pattern of [
    { label: 'toolchain runner not configured option', regex: /value=\{toolchainDraft\.runner\}[\s\S]*?<option value="">\{zhCN\.settings\.notConfigured\}<\/option>/ },
    { label: 'toolchain healthcheck not configured option', regex: /value=\{toolchainDraft\.healthcheck\}[\s\S]*?<option value="">\{zhCN\.settings\.notConfigured\}<\/option>/ },
  ]) {
    if (!pattern.regex.test(shell)) {
      console.error(`OuroborosIDE.tsx must expose Chinese unconfigured option for ${pattern.label}`);
      failed = true;
    }
  }
  for (const snippet of [
    '<option value="openai_compatible">openai_compatible</option>',
    '<option value="native">native</option>',
    '<option value="command">command</option>',
    '<option value="built_in">built_in</option>',
    '<option value="exec">exec</option>',
    '{pack.name || pack.id}',
    'toolchainDraft.manifestPath || \'--\'',
  ]) {
    if (shell.includes(snippet)) {
      console.error(`OuroborosIDE.tsx must not expose internal option value as visible settings copy: ${snippet}`);
      failed = true;
    }
  }
  if (!shell.includes('zhCN.mission.noMessages')) {
    console.error('OuroborosIDE.tsx must render a real Chinese empty state when backend provides no mission messages');
    failed = true;
  }
  for (const snippet of ['data.messages', 'MessageBubble', 'message.author', 'message.label', 'message.body', 'message.status']) {
    if (shell.includes(snippet)) {
      console.error(`OuroborosIDE.tsx must not keep dormant mission message rows without backend DTO support: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['zhCN.mission.agentGraph', 'agent-flow']) {
    if (shell.includes(snippet)) {
      console.error(`OuroborosIDE.tsx must not present synthesized workspace counts as an agent graph: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ["Bot,", 'icon={Bot}', 'typeof Bot']) {
    if (shell.includes(snippet)) {
      console.error(`OuroborosIDE.tsx must not keep AI/bot visual semantics in the mission operation surface: ${snippet}`);
      failed = true;
    }
  }
  if (!shell.includes('zhCN.mission.workspaceOverview') || !shell.includes('workspace-summary-flow')) {
    console.error('OuroborosIDE.tsx must label the right-rail count summary as a workspace overview');
    failed = true;
  }
  if (!shell.includes('SettingsWorkbench data={ideData}')) {
    console.error('SettingsWorkbench must render real IDE data, not fixed defaults');
    failed = true;
  }
  for (const snippet of ['DesktopStatusBanner', 'desktopStatus', 'zhCN.desktop.backendCrashed', 'zhCN.desktop.restartBackend']) {
    if (!shell.includes(snippet)) {
      console.error(`OuroborosIDE.tsx must expose real desktop status surface: ${snippet}`);
      failed = true;
    }
  }
}

const scannedSourceFiles = [
  'src/app/OuroborosIDE.tsx',
  'src/app/api.ts',
  'src/app/types.ts',
  'src/app/zh-cn.ts',
];

const forbiddenSourceSnippets = [
  'statusSafety',
  'scopeReady',
  'authorizationValid',
  'nextGate',
  'secureWorkspace',
  'sandbox',
  '人工门控',
  '安全边界',
  '授权有效',
  '范围已就绪',
  '待审阅',
  '下一门控',
  "providerBaseUrl: 'Base URL'",
  "providerName: '例如 local-openai'",
  "providerModel: '例如 qwen-coder'",
  "providerBaseUrl: 'https://host.example/v1'",
  "providerApiKeyEnv: '例如 OURO_API_KEY'",
  "providerCapabilities: 'chat, tools'",
  "toolchainCapabilities: 'local.command, evidence.collection'",
  "toolchainRunCommand: 'sh, -lc, command'",
  "toolchainHealthCommand: 'sh, -c, printf ok'",
  "workspaceTags: 'desktop, lab'",
];

for (const file of scannedSourceFiles) {
  const fullPath = join(root, file);
  if (!existsSync(fullPath)) {
    continue;
  }
  const source = readFileSync(fullPath, 'utf8');
  for (const snippet of forbiddenSourceSnippets) {
    if (source.includes(snippet)) {
      console.error(`${file} contains old UI/product state: ${snippet}`);
      failed = true;
    }
  }
}

const apiPath = join(root, 'src/app/api.ts');
if (existsSync(apiPath)) {
  const api = readFileSync(apiPath, 'utf8');
  for (const snippet of ['SQL Injection', 'Port 80 (HTTP)', '10.0.0.1', '192.168.1.']) {
    if (api.includes(snippet)) {
      console.error(`api.ts contains mock response translation: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['startExecution', 'stopExecution', 'retryExecution', 'loadExecutionLogs', 'loadExecutionArtifacts', '/api/executions']) {
    if (!api.includes(snippet)) {
      console.error(`api.ts must expose real execution API: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['ToolAction', 'ToolActionRunResponse', 'runToolchainAction', 'toolActions?:', '/api/toolchain/actions', 'mapToolActions']) {
    if (!api.includes(snippet)) {
      console.error(`api.ts must consume real manifest-backed tool action API: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['AIAction', 'AIActionRunRequest', 'AIActionRunResponse', 'fetchAIActions', 'runAIAction', '/api/ai/actions', '/api/ai/actions/run']) {
    if (!api.includes(snippet)) {
      console.error(`api.ts must expose real AI action API: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['OrchestratorRun', 'orchestratorRun?: OrchestratorRun', 'orchestratorRun?: OrchestratorRunResponse', "case 'orchestrator'", "return 'orchestrator'", "case 'orchestrator_run'", "return 'orchestrator_run'", 'mapAIActionOrchestratorRun', 'orchestratorRun: mapAIActionOrchestratorRun(result.orchestratorRun)']) {
    if (!api.includes(snippet)) {
      console.error(`api.ts must map real orchestrator.run AI action responses: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['export async function exportEvidenceGraph', '/api/evidence/graph/export', "method: 'POST'"]) {
    if (!api.includes(snippet)) {
      console.error(`api.ts must expose real graph export API calls: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['export async function exportReport', '/api/reports/export', "method: 'POST'"]) {
    if (!api.includes(snippet)) {
      console.error(`api.ts must expose real report export API calls: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of [
    "filePath: payload.filePath ?? ''",
    'nodeCount: payload.nodeCount ?? 0',
    'edgeCount: payload.edgeCount ?? 0',
    'findingCount: payload.findingCount ?? 0',
    'reportCount: payload.reportCount ?? 0',
    'modifiedAt: payload.modifiedAt ?? 0',
  ]) {
    if (api.includes(snippet)) {
      console.error(`api.ts must require backend-authored export result fields instead of fabricating export results: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['filePath?: string', 'filePath: artifact.filePath']) {
    if (!api.includes(snippet)) {
      console.error(`api.ts must consume execution artifact file path from backend DTO: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['saveMission', 'startMission', '/api/mission/start', 'mission?:']) {
    if (!api.includes(snippet)) {
      console.error(`api.ts must expose real mission API: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of [
    "path: mission?.path || ''",
    'status: translateBackendStatus(mission?.status)',
  ]) {
    if (api.includes(snippet)) {
      console.error(`api.ts must require backend-authored mission path/status instead of fabricating mission DTO state: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['saveProviderConfig', '/api/provider', 'provider?:', 'ProviderSettings']) {
    if (!api.includes(snippet)) {
      console.error(`api.ts must expose real provider settings API: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['saveToolchainPack', '/api/toolchain/packs', 'ToolchainPackSettings']) {
    if (!api.includes(snippet)) {
      console.error(`api.ts must expose real toolchain settings API: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['saveWorkspaceSettings', '/api/workspace/settings', 'WorkspaceSettings']) {
    if (!api.includes(snippet)) {
      console.error(`api.ts must expose real workspace settings API: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['selectDesktopWorkspace', 'createDesktopWorkspace']) {
    if (!api.includes(snippet)) {
      console.error(`api.ts must expose desktop workspace API: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['connectIDEEventStream', 'WebSocket', '/api/events/stream']) {
    if (!api.includes(snippet)) {
      console.error(`api.ts must expose real event stream API: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['DesktopStatus', 'loadDesktopStatus', 'restartDesktopBackend', 'openRecentDesktopWorkspace', 'window.ouroboros?.backendStatus', 'window.ouroboros?.restartBackend', 'window.ouroboros?.openRecentWorkspace']) {
    if (!api.includes(snippet)) {
      console.error(`api.ts must expose real desktop lifecycle API: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['replayEvents?:', 'reportFindings?:', 'mapBootstrapReplayEvents', 'mapBootstrapReportFindings']) {
    if (!api.includes(snippet)) {
      console.error(`api.ts must consume backend bootstrap projection: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of [
    "const id = event.id || '';",
    'event: translateReplayEventType(event.type)',
    'function translateReplayEventType(type?: string)',
    'return (zhCN.replay.eventTypes as Record<string, string>)[key] ?? zhCN.replay.events;',
    'function formatTime(unixSeconds?: number)',
    "return '--:--:--';",
  ]) {
    if (api.includes(snippet)) {
      console.error(`api.ts must require complete backend replay event type/timestamp DTOs instead of fabricating replay timeline fields: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of [
    'loadIDEDataFromLegacyEndpoints',
    'LoadedResponses',
    'mapIDEData(responses',
    "'/api/health'",
    "'/api/workspace/info'",
    "'/api/evidence/graph'",
    "'/api/runs'",
    "'/api/toolchain/health'",
  ]) {
    if (api.includes(snippet)) {
      console.error(`api.ts must not assemble GUI state from legacy endpoint fallbacks: ${snippet}`);
      failed = true;
    }
  }
  if (!api.includes('mapBootstrapUnavailableData')) {
    console.error('api.ts must expose a Chinese bootstrap-unavailable state instead of legacy GUI assembly');
    failed = true;
  }
  for (const snippet of [
    "return mapBootstrapUnavailableData('degraded', latency());",
    "backend: response.backend?.status ? translateBackendStatus(response.backend.status) : zhCN.explorer.backendReady",
    'return mapProviderSettings(payload.active ?? payload.providers?.find(item => item.enabled) ?? provider);',
  ]) {
    if (api.includes(snippet)) {
      console.error(`api.ts must not fabricate backend/API state from failed or incomplete DTOs: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of [
    "return mapBootstrapUnavailableData('bootstrap-error', latency());",
    'function mapStatusBar(',
    'function mapRequiredProviderConfigResponse(',
  ]) {
    if (!api.includes(snippet)) {
      console.error(`api.ts must explicitly represent bootstrap/provider API errors: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['nodeId?:', 'filePath?:', 'executionId?:', 'replayEventId?:']) {
    if (!api.includes(snippet)) {
      console.error(`api.ts must consume backend cross-reference field: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['sourceKind?:', 'createdAt?:', 'data?: Record<string, unknown>', 'sourceKind: node.sourceKind', 'createdAt: node.createdAt', 'data: data', 'runId: event.runId', 'lane: event.lane', 'createdAt: event.createdAt']) {
    if (!api.includes(snippet)) {
      console.error(`api.ts must preserve backend detail field for real inspectors: ${snippet}`);
      failed = true;
    }
  }
  for (const pattern of [
    { label: 'emptyProvider type', regex: /function emptyProvider\(\)[\s\S]*?type:\s*'openai_compatible'/ },
    { label: 'emptyProvider enabled', regex: /function emptyProvider\(\)[\s\S]*?enabled:\s*true/ },
    { label: 'emptyProvider routing', regex: /function emptyProvider\(\)[\s\S]*?routingStrategy:\s*'first_enabled'/ },
    { label: 'mapToolchainPack runner', regex: /function mapToolchainPack\([^)]*\)[\s\S]*?runner:\s*pack\.runner\s*\|\|\s*'native'/ },
    { label: 'mapToolchainPack healthcheck', regex: /function mapToolchainPack\([^)]*\)[\s\S]*?healthcheck:\s*pack\.healthcheck\s*\|\|\s*'built_in'/ },
  ]) {
    if (pattern.regex.test(api)) {
      console.error(`api.ts must not fabricate settings in empty or partial backend state: ${pattern.label}`);
      failed = true;
    }
  }
  for (const snippet of [
    'mapMessages(',
    "author: 'agent'",
    '后端已返回',
    'messages: []',
  ]) {
    if (api.includes(snippet)) {
      console.error(`api.ts must not synthesize mission conversation messages in the frontend: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of [
    'workflowStatus(apiStatus, graphNodes)',
    'workflowStatus(responses.apiStatus, graphNodes)',
    'nodes.length > 0 ? zhCN.mission.nextAction',
  ]) {
    if (api.includes(snippet)) {
      console.error(`api.ts must not synthesize workflow state from frontend graph counts: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of [
    'return mapReplayEvents({ runs });',
    'const replayEvents = mapReplayEvents(responses.runs);',
    'const reportFindings = mapReportFindings(graphNodes);',
    'if (reports?.length) {',
    'reports.map(report =>',
    'mapBootstrapReports(response.reports, graphNodes)',
  ]) {
    if (api.includes(snippet)) {
      console.error(`api.ts must not derive projected GUI data in the frontend: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of [
    '`event-${index + 1}`',
    '`finding-${index + 1}`',
    '`report-${index + 1}`',
    '`node-${index + 1}`',
    '`${edge.from}-${edge.to}-${index}`',
    "const id = event.id || event.runId || '';",
    "const id = finding.id || finding.path || '';",
    "const id = report.id || report.path || '';",
    "pack.id || pack.name || 'toolchain'",
    "id: action.id || action.packId || ''",
    "packId: action.packId || action.id || ''",
    "label: action.label || action.packName || action.id || ''",
    "commandText: action.commandText || (action.command ?? []).join(' ')",
    "action.id && action.packId && action.label && action.commandText ? [mapToolAction(action)] : []",
    "action: mapToolAction(payload.action ?? {})",
    "command: action.command ?? []",
    "id: pack.id || pack.name || ''",
    'return (packs ?? []).flatMap(pack => (pack.id ? [mapToolchainPack(pack)] : []));',
    "id: pack.id || ''",
    "name: pack.name || ''",
    "toolsReady: pack.toolsReady ?? 0",
    "manifestPath: pack.manifestPath || ''",
    "sum + (pack.toolsReady ?? 0)",
    "path: artifact.path ?? artifact.name ?? ''",
    "name: artifact.name ?? artifact.path ?? ''",
    "artifact.path && artifact.name ? [mapExecutionArtifact(artifact)] : []",
    "size: artifact.size ?? 0",
    "modifiedAt: artifact.modifiedAt ?? 0",
    "const id = file.id || file.path || file.name || '';",
    "const label = file.path || file.name || '';",
    "meta: file.kind ? translateFileKind(file.kind) : '文件'",
    "id: file.id || file.path || file.name || 'file'",
    "kind: edge.type || 'related'",
    "label: file.path || file.name || '未命名文件'",
    "configPath: 'workspace.yaml'",
    "workspace?.path ? `${workspace.path}/workspace.yaml`",
    "const workspace = workspaceSettings.name || workspaceName(response.workspace);",
    "name: workspace?.name || workspaceName(workspace)",
    'function workspaceName(response?: WorkspaceResponse)',
    "id: execution.id || command || 'execution'",
    "const command = execution.command || '';",
    "label: command ? command.split(/\\s+/).slice(0, 3).join(' ') : zhCN.execution.commandFallback",
  ]) {
    if (api.includes(snippet)) {
      console.error(`api.ts must not synthesize object identities or workspace paths in the frontend: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of [
    'const sourceNodes = (response?.nodes ?? []).filter(node => Boolean(node.id));',
    'const kind = normalizeKind(node.type);',
    'label: translateEvidenceLabel(node.label || id)',
    'function normalizeKind(value?: string)',
    "switch ((value ?? '').toLowerCase())",
  ]) {
    if (api.includes(snippet)) {
      console.error(`api.ts must require backend-authored graph node labels and types instead of fabricating graph semantics: ${snippet}`);
      failed = true;
    }
  }
  if (api.includes("default:\n      return '文件';")) {
    console.error("api.ts must require backend-authored workspace file kind values instead of mapping unknown file kinds to generic copy");
    failed = true;
  }
  for (const snippet of [
    'payload.execution ?? {}',
    'return mapExecutionResponse(await response.json() as ExecutionResponse);',
  ]) {
    if (api.includes(snippet)) {
      console.error(`api.ts must reject missing execution DTOs instead of mapping empty execution objects: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of [
    'Boolean(execution.id && execution.label && execution.command)',
    'if (!execution?.id || !execution?.label || !execution?.command)',
    'function normalizeExecutionStatus(status?: string)',
  ]) {
    if (api.includes(snippet)) {
      console.error(`api.ts must require backend-authored execution status instead of defaulting missing status: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of [
    "detail: event.detail || event.runId || ''",
    "endpoint: finding.endpoint || finding.path || finding.source || ''",
    "severity: normalizeSeverity(finding.severity) ?? 'info'",
    "title: finding.title || finding.path || zhCN.report.empty",
    "title: report.title || report.path || zhCN.report.mastheadSubtitle",
    'status: translateBackendStatus(report.status)',
    "path: report.path || ''",
  ]) {
    if (api.includes(snippet)) {
      console.error(`api.ts must not synthesize visible detail text from secondary technical fields: ${snippet}`);
      failed = true;
    }
  }
  if (!api.includes('mapRequiredExecutionResponse')) {
    console.error('api.ts must validate execution DTOs before returning action results');
    failed = true;
  }
  for (const snippet of ["result.kind === 'orchestrator_run'", 'result.result.orchestratorRun', 'result.result.blackboardItem', 'result.result.agentTask', 'result.result.harnessCase', 'result.result.execution']) {
    if (!api.includes(snippet) && !shellSource.includes(snippet)) {
      console.error(`frontend must preserve real orchestrator.run result objects: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ["id: 'evidence'", "id: 'runtime'", "id: 'reports'", "id: 'toolchain'"]) {
    if (api.includes(snippet)) {
      console.error(`api.ts must not synthesize workspace explorer nodes: ${snippet}`);
      failed = true;
    }
  }
}

const typesPath = join(root, 'src/app/types.ts');
if (existsSync(typesPath)) {
  const types = readFileSync(typesPath, 'utf8');
  for (const snippet of ['interface MissionMessage', "author: 'operator' | 'agent' | 'system'", 'messages: MissionMessage[]']) {
    if (types.includes(snippet)) {
      console.error(`types.ts must not expose dormant mission message types without backend DTO support: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['export type AIActionKind', "export type AIActionKind = 'command' | 'tool' | 'file' | 'report' | 'blackboard' | 'agent' | 'harness' | 'orchestrator'", 'export interface AIActionRunRequest', 'export interface AIActionRunResponse', 'export interface AIActionResult', 'export interface OrchestratorRun', 'orchestratorRun?: OrchestratorRun', 'blackboardItem?: BlackboardItem', 'agentTask?: AgentTask', 'harnessCase?: HarnessCase', 'execution?: ExecutionStep']) {
    if (!types.includes(snippet)) {
      console.error(`types.ts must expose orchestrator.run AI action contract: ${snippet}`);
      failed = true;
    }
  }
}

const stylePath = join(root, 'src/styles/ouroboros-ide.css');
if (existsSync(stylePath)) {
  const styles = readFileSync(stylePath, 'utf8');
  for (const snippet of ['.message.agent', '.message.system', '.message-label']) {
    if (styles.includes(snippet)) {
      console.error(`ouroboros-ide.css must not keep stale mission message styling without backend DTO support: ${snippet}`);
      failed = true;
    }
  }
}

const copyPath = join(root, 'src/app/zh-cn.ts');
if (existsSync(copyPath)) {
  const copy = readFileSync(copyPath, 'utf8');
  if (copy.includes('生成预览')) {
    console.error('zh-cn.ts still contains placeholder preview copy');
    failed = true;
  }
  if (copy.includes('commandFallback')) {
    console.error('zh-cn.ts must not keep unused execution command fallback copy after backend-authored execution labels');
    failed = true;
  }
  if (!copy.includes('aiChat')) {
    console.error('zh-cn.ts must define real AI chat copy for backend-projected chat messages');
    failed = true;
  }
  if (copy.includes('AI 对话') && !copy.includes('aiChat')) {
    console.error('zh-cn.ts must not label the mission panel as AI chat without real aiChat copy key');
    failed = true;
  }
  for (const snippet of ['智能体图', '下一步', "agent: '智能体'"]) {
    if (copy.includes(snippet)) {
      console.error(`zh-cn.ts must not expose stale agent/next-step copy without backend DTO support: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['工作区概览', '真实文件、执行与图谱计数']) {
    if (!copy.includes(snippet)) {
      console.error(`zh-cn.ts missing workspace overview copy: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['任务操作', '真实任务、工具与命令入口', '暂无后端投影的任务消息']) {
    if (!copy.includes(snippet)) {
      console.error(`zh-cn.ts missing real mission operation copy: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['新建工作区', '选择一个现有目录，或者创建一个新目录并初始化为 Ouroboros 工作区']) {
    if (!copy.includes(snippet)) {
      console.error(`zh-cn.ts missing workspace onboarding copy: ${snippet}`);
      failed = true;
    }
  }
  if (!copy.includes('暂无工作区文件')) {
    console.error('zh-cn.ts missing real empty workspace file copy');
    failed = true;
  }
  for (const snippet of ['工作区操作失败', '未选择工作区', '正在初始化工作区']) {
    if (!copy.includes(snippet)) {
      console.error(`zh-cn.ts missing workspace operation copy: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['任务目标', '启动任务', '保存任务', '任务启动失败']) {
    if (!copy.includes(snippet)) {
      console.error(`zh-cn.ts missing direct mission workflow copy: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['保存模型服务', '模型服务已保存', 'API Key 环境变量', '上下文窗口']) {
    if (!copy.includes(snippet)) {
      console.error(`zh-cn.ts missing editable provider settings copy: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['保存工具链', '工具链已保存', '运行器', '解析器', '健康检查']) {
    if (!copy.includes(snippet)) {
      console.error(`zh-cn.ts missing editable toolchain settings copy: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['保存工作区', '工作区已保存', '工作区描述', '操作者', '标签']) {
    if (!copy.includes(snippet)) {
      console.error(`zh-cn.ts missing editable workspace settings copy: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['定位图谱', '定位回放', '定位报告', '打开证据文件', '查看执行']) {
    if (!copy.includes(snippet)) {
      console.error(`zh-cn.ts missing cross-navigation copy: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['节点详情', '回放详情', '发现详情', '证据文件', '执行详情', '原始字段', '关联对象']) {
    if (!copy.includes(snippet)) {
      console.error(`zh-cn.ts missing detail view copy: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['桌面后端已崩溃', '正在启动后端', '重启后端', '最近工作区', '窗口状态已保存']) {
    if (!copy.includes(snippet)) {
      console.error(`zh-cn.ts missing desktop lifecycle copy: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['工具启动', '来自 toolchain manifest 的真实可执行动作', '运行工具', '工具参数', '暂无可直接运行的工具动作']) {
    if (!copy.includes(snippet)) {
      console.error(`zh-cn.ts missing manifest-backed tool launch copy: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['AI 动作', '编排动作', '运行编排', '编排运行结果', '后端返回的编排运行为空', '编排标题', '编排目标', '期望结果', '来源', '关联文件']) {
    if (!copy.includes(snippet)) {
      console.error(`zh-cn.ts missing orchestrator.run AI action copy: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['打开日志文件', '打开产物文件']) {
    if (!copy.includes(snippet)) {
      console.error(`zh-cn.ts missing execution file jump copy: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['导出图谱', '正在导出图谱', '图谱已导出', '图谱导出失败']) {
    if (!copy.includes(snippet)) {
      console.error(`zh-cn.ts missing graph export copy: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['导出报告', '正在导出报告', '报告已导出', '报告导出失败']) {
    if (!copy.includes(snippet)) {
      console.error(`zh-cn.ts missing report export copy: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['风险分布', '项目资料', '已有报告文件', '关联证据与执行']) {
    if (!copy.includes(snippet)) {
      console.error(`zh-cn.ts missing report section copy: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['当前软件状态', '工作区配置', '模型服务配置', '工具链配置']) {
    if (!copy.includes(snippet)) {
      console.error(`zh-cn.ts missing settings section copy: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['notConfigured:', 'providerTypes:', 'openaiCompatible:', 'toolchainRunners:', 'native:', 'command:', 'healthchecks:', 'builtIn:', 'exec:']) {
    if (!copy.includes(snippet)) {
      console.error(`zh-cn.ts missing Chinese settings option copy: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['关联执行文件', '载入执行日志', '载入产物列表']) {
    if (!copy.includes(snippet)) {
      console.error(`zh-cn.ts missing report detail execution file jump copy: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['commandPalette:', '刷新真实数据', '重启后端', '运行当前命令', '保存当前任务', '启动当前任务', '打开最近工作区', '切换到']) {
    if (!copy.includes(snippet)) {
      console.error(`zh-cn.ts missing real command palette copy: ${snippet}`);
      failed = true;
    }
  }
}

if (failed) {
  process.exit(1);
}
