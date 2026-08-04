import {
  Activity,
  ChevronRight,
  CircleDot,
  Command,
  FileText,
  FolderOpen,
  FolderPlus,
  GitBranch,
  LayoutDashboard,
  Lock,
  Maximize2,
  Minus,
  Network,
  Play,
  Radar,
  RefreshCw,
  Save,
  Search,
  Settings,
  ShieldCheck,
  TerminalSquare,
  X,
} from 'lucide-react';
import type { LucideIcon } from 'lucide-react';
import { useEffect, useMemo, useState } from 'react';
import type { KeyboardEvent } from 'react';
import { closeDesktopWindow, connectIDEEventStream, createDesktopWorkspace, exportEvidenceGraph, exportReport, fetchAIActions, fetchAIChat, loadDesktopStatus, loadExecutionArtifacts, loadExecutionLogs, loadIDEData, loadWorkspaceFile, minimizeDesktopWindow, onDesktopBackendStatus, onDesktopMenuAction, openRecentDesktopWorkspace, restartDesktopBackend, retryExecution, runAIAction, runToolchainAction, saveMission, saveProviderConfig, saveToolchainPack, saveWorkspaceFile, saveWorkspaceSettings, searchWorkspace, selectDesktopWorkspace, sendAIChatMessage, startExecution, startMission, stopExecution, toggleMaximizeDesktopWindow } from './api';
import type { AgentTask, AIAction, AIActionRunRequest, AIActionRunResponse, BlackboardItem, ChatMessage, DesktopStatus, ExecutionArtifact, ExecutionLog, ExecutionStep, GraphEdge, GraphNode, HarnessCase, IDEData, IDEEvent, IDEMode, MissionState, OrchestratorRun, ProviderSettings, ReplayEvent, ReportDocument, ReportFinding, Severity, ToolAction, ToolchainPackSettings, WorkspaceFileContent, WorkspaceItem, WorkspaceSearchResult, WorkspaceSettings } from './types';
import { zhCN } from './zh-cn';

const modes: Array<{ id: IDEMode; icon: LucideIcon }> = [
  { id: 'mission', icon: LayoutDashboard },
  { id: 'graph', icon: Network },
  { id: 'replay', icon: Play },
  { id: 'report', icon: FileText },
  { id: 'settings', icon: Settings },
];

type CommandPaletteAction = {
  id: string;
  label: string;
  meta: string;
  icon: LucideIcon;
  disabled?: boolean;
  run: () => void | Promise<void>;
};

type ExecutionReferenceTargets = {
  node?: GraphNode;
  event?: ReplayEvent;
  finding?: ReportFinding;
};

const emptyData: IDEData = {
  workspace: {
    name: zhCN.explorer.rootFallback,
    path: '',
    status: zhCN.backendStatus.unknown,
    configPath: '',
    description: '',
    operator: '',
    tags: [],
  },
  mission: {
    name: '',
    goal: '',
    command: '',
    path: '',
    status: zhCN.backendStatus.unknown,
  },
  provider: {
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
  },
  toolchainPacks: [],
  toolActions: [],
  workspaceItems: [],
  graphNodes: [],
  graphEdges: [],
  executionSteps: [],
  replayEvents: [],
  reportFindings: [],
  reports: [],
  status: {
    workspace: zhCN.explorer.rootFallback,
    workflow: zhCN.shell.loading,
    provider: zhCN.shell.statusOffline,
    toolchain: zhCN.shell.statusOffline,
    backend: zhCN.shell.statusOffline,
    latency: '--',
    api: 'offline',
  },
  loadedAt: '',
};

function shouldRefreshForEvent(event: IDEEvent): boolean {
  return event.type.startsWith('execution:') || event.type === 'workspace:changed' || event.type === 'provider:updated' || event.type === 'toolchain:updated';
}

export function OuroborosIDE() {
  const [mode, setMode] = useState<IDEMode>('mission');
  const [selectedNodeId, setSelectedNodeId] = useState('');
  const [selectedReplayEventId, setSelectedReplayEventId] = useState('');
  const [selectedReportFindingId, setSelectedReportFindingId] = useState('');
  const [paletteOpen, setPaletteOpen] = useState(false);
  const [ideData, setIDEData] = useState<IDEData>(emptyData);
  const [openFile, setOpenFile] = useState<WorkspaceFileContent | null>(null);
  const [fileDraft, setFileDraft] = useState('');
  const [fileStatus, setFileStatus] = useState(zhCN.editor.empty);
  const [searchQuery, setSearchQuery] = useState('');
  const [searchResults, setSearchResults] = useState<WorkspaceSearchResult[]>([]);
  const [commandDraft, setCommandDraft] = useState('');
  const [executionStatus, setExecutionStatus] = useState(zhCN.execution.empty);
  const [executionLog, setExecutionLog] = useState<ExecutionLog | null>(null);
  const [executionArtifacts, setExecutionArtifacts] = useState<ExecutionArtifact[]>([]);
  const [executionArtifactsExecutionId, setExecutionArtifactsExecutionId] = useState('');
  const [graphExportStatus, setGraphExportStatus] = useState('');
  const [reportExportStatus, setReportExportStatus] = useState('');
  const [workspaceActionStatus, setWorkspaceActionStatus] = useState('');
  const [missionNameDraft, setMissionNameDraft] = useState('');
  const [missionGoalDraft, setMissionGoalDraft] = useState('');
  const [missionCommandDraft, setMissionCommandDraft] = useState('');
  const [missionStatus, setMissionStatus] = useState('');
  const [missionDirty, setMissionDirty] = useState(false);
  const [desktopStatus, setDesktopStatus] = useState<DesktopStatus | null>(null);
  const [toolActionArguments, setToolActionArguments] = useState<Record<string, string>>({});
  const [toolLaunchStatus, setToolLaunchStatus] = useState('');
  const [aiActions, setAIActions] = useState<AIAction[]>([]);
  const [aiActionInputs, setAIActionInputs] = useState<Record<string, string>>({});
  const [aiActionStatus, setAIActionStatus] = useState('');
  const [aiActionResult, setAIActionResult] = useState<AIActionRunResponse | null>(null);
  const [aiActionRunningId, setAIActionRunningId] = useState('');
  const [chatMessages, setChatMessages] = useState<ChatMessage[]>([]);
  const [chatDraft, setChatDraft] = useState('');
  const [chatStatus, setChatStatus] = useState('');
  const [chatSummary, setChatSummary] = useState('');
  const [chatRefs, setChatRefs] = useState<string[]>([]);

  useEffect(() => {
    function handleShortcut(event: KeyboardEvent) {
      if ((event.ctrlKey || event.metaKey) && event.key.toLowerCase() === 'k') {
        event.preventDefault();
        setPaletteOpen(true);
      }
      if (event.key === 'Escape') {
        setPaletteOpen(false);
      }
    }
    window.addEventListener('keydown', handleShortcut);
    return () => window.removeEventListener('keydown', handleShortcut);
  }, []);

  async function refresh() {
    const [next, desktop] = await Promise.all([loadIDEData(), loadDesktopStatus()]);
    setIDEData(next);
    setDesktopStatus(desktop);
    setSelectedNodeId(current => {
      if (next.graphNodes.some(node => node.id === current)) {
        return current;
      }
      return next.graphNodes[0]?.id ?? '';
    });
    setSelectedReplayEventId(current => {
      if (next.replayEvents.some(event => event.id === current)) {
        return current;
      }
      return next.replayEvents[0]?.id ?? '';
    });
    setSelectedReportFindingId(current => {
      if (next.reportFindings.some(finding => finding.id === current)) {
        return current;
      }
      return next.reportFindings[0]?.id ?? '';
    });
  }

  async function refreshAIActions() {
    setAIActionStatus(zhCN.aiActions.loading);
    try {
      const actions = await fetchAIActions();
      setAIActions(actions);
      setAIActionStatus(`${zhCN.aiActions.loaded}: ${actions.length}`);
    } catch {
      setAIActionStatus(zhCN.aiActions.loadFailed);
    }
  }

  useEffect(() => {
    let cancelled = false;
    let disconnect: (() => void) | undefined;

    async function refreshIfMounted() {
      const [next, desktop] = await Promise.all([loadIDEData(), loadDesktopStatus()]);
      if (cancelled) {
        return;
      }
      setIDEData(next);
      setDesktopStatus(desktop);
      setSelectedNodeId(current => {
        if (next.graphNodes.some(node => node.id === current)) {
          return current;
        }
        return next.graphNodes[0]?.id ?? '';
      });
      setSelectedReplayEventId(current => {
        if (next.replayEvents.some(event => event.id === current)) {
          return current;
        }
        return next.replayEvents[0]?.id ?? '';
      });
      setSelectedReportFindingId(current => {
        if (next.reportFindings.some(finding => finding.id === current)) {
          return current;
        }
        return next.reportFindings[0]?.id ?? '';
      });
    }

    void refreshIfMounted();
    void refreshAIActions();
    void refreshAIChat();
    void connectIDEEventStream(event => {
      if (shouldRefreshForEvent(event)) {
        void refreshIfMounted();
      }
    }).then(cleanup => {
      disconnect = cleanup;
    });
    const interval = window.setInterval(() => void refreshIfMounted(), 30_000);
    return () => {
      cancelled = true;
      disconnect?.();
      window.clearInterval(interval);
    };
  }, []);

  const selectedNode = useMemo(
    () => ideData.graphNodes.find(node => node.id === selectedNodeId) ?? ideData.graphNodes[0],
    [ideData.graphNodes, selectedNodeId],
  );
  const selectedReplayEvent = useMemo(
    () => ideData.replayEvents.find(event => event.id === selectedReplayEventId) ?? ideData.replayEvents[0],
    [ideData.replayEvents, selectedReplayEventId],
  );

  useEffect(() => {
    if (missionDirty) {
      return;
    }
    setMissionNameDraft(ideData.mission.name);
    setMissionGoalDraft(ideData.mission.goal);
    setMissionCommandDraft(ideData.mission.command);
    setMissionStatus(ideData.mission.status);
  }, [ideData.mission, missionDirty]);

  async function openWorkspace() {
    setWorkspaceActionStatus(zhCN.workspace.opening);
    try {
      const result = await selectDesktopWorkspace();
      setWorkspaceActionStatus(result?.workspace ? `${zhCN.workspace.opened}: ${result.workspace}` : zhCN.workspace.noSelection);
      await refresh();
    } catch {
      setWorkspaceActionStatus(zhCN.workspace.operationFailed);
    }
  }

  async function createWorkspace() {
    setWorkspaceActionStatus(zhCN.workspace.creating);
    try {
      const result = await createDesktopWorkspace();
      setWorkspaceActionStatus(result?.workspace ? `${zhCN.workspace.created}: ${result.workspace}` : zhCN.workspace.noSelection);
      await refresh();
    } catch {
      setWorkspaceActionStatus(zhCN.workspace.operationFailed);
    }
  }

  async function openRecentWorkspace(path: string) {
    setWorkspaceActionStatus(`${zhCN.commandPalette.openRecentWorkspace}: ${path}`);
    try {
      const result = await openRecentDesktopWorkspace(path);
      setWorkspaceActionStatus(result?.workspace ? `${zhCN.workspace.opened}: ${result.workspace}` : zhCN.workspace.noSelection);
      await refresh();
    } catch {
      setWorkspaceActionStatus(zhCN.workspace.operationFailed);
    }
  }

  async function openWorkspaceFile(item: WorkspaceItem) {
    if (!item.filePath) {
      return;
    }
    setFileStatus(zhCN.editor.loading);
    try {
      const file = await loadWorkspaceFile(item.filePath);
      setOpenFile(file);
      setFileDraft(file.content);
      setFileStatus(file.path);
    } catch {
      setFileStatus(zhCN.editor.error);
    }
  }

  async function saveOpenFile() {
    if (!openFile) {
      return;
    }
    const file = await saveWorkspaceFile(openFile.path, fileDraft);
    setOpenFile(file);
    setFileDraft(file.content);
    setFileStatus(zhCN.editor.saved);
    await refresh();
  }

  async function runSearch() {
    const results = await searchWorkspace(searchQuery);
    setSearchResults(results);
  }

  async function openSearchResult(result: WorkspaceSearchResult) {
    await openWorkspaceFile({ id: result.path, label: result.path, meta: `${result.line}`, filePath: result.path });
  }

  async function exportGraph() {
    setGraphExportStatus(zhCN.graph.exporting);
    try {
      const result = await exportEvidenceGraph();
      setGraphExportStatus(`${zhCN.graph.exported}: ${result.filePath}`);
      await refresh();
      if (result.filePath) {
        setMode('mission');
        await openWorkspaceFile({ id: result.filePath, label: result.filePath, meta: zhCN.graph.exported, filePath: result.filePath });
      }
    } catch {
      setGraphExportStatus(zhCN.graph.exportFailed);
    }
  }

  async function exportCurrentReport() {
    setReportExportStatus(zhCN.report.exporting);
    try {
      const result = await exportReport();
      setReportExportStatus(`${zhCN.report.exported}: ${result.filePath}`);
      await refresh();
      if (result.filePath) {
        setMode('mission');
        await openWorkspaceFile({ id: result.filePath, label: result.filePath, meta: zhCN.report.exported, filePath: result.filePath });
      }
    } catch {
      setReportExportStatus(zhCN.report.exportFailed);
    }
  }

  function onFocusGraphNode(nodeId: string) {
    if (!nodeId.trim()) {
      return;
    }
    setSelectedNodeId(nodeId);
    setMode('graph');
  }

  function onFocusReplayEvent(eventId: string) {
    if (!eventId.trim()) {
      return;
    }
    setSelectedReplayEventId(eventId);
    setMode('replay');
  }

  function onFocusReportFinding(findingId: string) {
    if (!findingId.trim()) {
      return;
    }
    setSelectedReportFindingId(findingId);
    setMode('report');
  }

  async function onOpenFindingFile(filePath: string) {
    if (!filePath.trim()) {
      return;
    }
    setMode('mission');
    await openWorkspaceFile({ id: filePath, label: filePath, meta: '证据', filePath });
  }

  async function inspectExecution(executionId: string) {
    if (!executionId.trim()) {
      return;
    }
    setMode('mission');
    setExecutionStatus(`${zhCN.execution.selected}: ${executionId}`);
    await Promise.allSettled([
      showExecutionLogs(executionId),
      showExecutionArtifacts(executionId),
    ]);
  }

  async function onOpenExecutionFile(filePath: string) {
    if (!filePath.trim()) {
      return;
    }
    setMode('mission');
    await openWorkspaceFile({ id: filePath, label: filePath, meta: '执行文件', filePath });
  }

  async function runCommand() {
    const command = commandDraft.trim();
    if (!command) {
      return;
    }
    setExecutionStatus(zhCN.execution.starting);
    try {
      const execution = await startExecution(command);
      setCommandDraft('');
      setExecutionStatus(`${zhCN.execution.selected}: ${execution.id}`);
      await refresh();
      window.setTimeout(() => void refresh(), 1000);
    } catch {
      setExecutionStatus(zhCN.execution.actionFailed);
    }
  }

  function updateMissionDraft(field: keyof Pick<MissionState, 'name' | 'goal' | 'command'>, value: string) {
    setMissionDirty(true);
    if (field === 'name') {
      setMissionNameDraft(value);
      return;
    }
    if (field === 'goal') {
      setMissionGoalDraft(value);
      return;
    }
    setMissionCommandDraft(value);
  }

  async function saveCurrentMission() {
    try {
      const mission = await saveMission({ name: missionNameDraft, goal: missionGoalDraft, command: missionCommandDraft });
      setMissionDirty(false);
      setMissionStatus(`${zhCN.missionFlow.saved}: ${mission.path}`);
      await refresh();
    } catch {
      setMissionStatus(zhCN.missionFlow.saveFailed);
    }
  }

  async function startCurrentMission() {
    if (missionCommandDraft.trim() === '') {
      setMissionStatus(zhCN.missionFlow.commandRequired);
      return;
    }
    setMissionStatus(zhCN.execution.starting);
    try {
      const result = await startMission({ name: missionNameDraft, goal: missionGoalDraft, command: missionCommandDraft });
      setMissionDirty(false);
      setMissionStatus(`${zhCN.missionFlow.started}: ${result.execution.id}`);
      setExecutionStatus(`${zhCN.execution.selected}: ${result.execution.id}`);
      await refresh();
      window.setTimeout(() => void refresh(), 1000);
    } catch {
      setMissionStatus(zhCN.missionFlow.startFailed);
    }
  }

  async function handleSaveProvider(provider: ProviderSettings) {
    await saveProviderConfig(provider);
    await refresh();
  }

  async function handleSaveToolchain(pack: ToolchainPackSettings) {
    await saveToolchainPack(pack);
    await refresh();
  }

  async function handleSaveWorkspaceSettings(settings: WorkspaceSettings) {
    await saveWorkspaceSettings(settings);
    await refresh();
  }

  async function stopSelectedExecution(id: string) {
    try {
      const execution = await stopExecution(id);
      setExecutionStatus(`${zhCN.execution.stop}: ${execution.id}`);
      await refresh();
    } catch {
      setExecutionStatus(zhCN.execution.actionFailed);
    }
  }

  async function retrySelectedExecution(id: string) {
    try {
      const execution = await retryExecution(id);
      setExecutionStatus(`${zhCN.execution.retry}: ${execution.id}`);
      await refresh();
    } catch {
      setExecutionStatus(zhCN.execution.actionFailed);
    }
  }

  async function showExecutionLogs(id: string) {
    try {
      const logs = await loadExecutionLogs(id);
      setExecutionLog(logs);
      setExecutionStatus(logs.path);
    } catch {
      setExecutionStatus(zhCN.execution.actionFailed);
    }
  }

  async function showExecutionArtifacts(id: string) {
    try {
      const artifacts = await loadExecutionArtifacts(id);
      setExecutionArtifacts(artifacts);
      setExecutionArtifactsExecutionId(id);
      setExecutionStatus(`${artifacts.length} ${zhCN.execution.artifacts}`);
    } catch {
      setExecutionStatus(zhCN.execution.actionFailed);
    }
  }

  async function runManifestToolAction(action: ToolAction) {
    if (!action.id) {
      return;
    }
    setToolLaunchStatus(`${zhCN.toolLaunch.starting}: ${action.label}`);
    try {
      const result = await runToolchainAction(action.id, toolActionArguments[action.id] ?? '');
      setToolLaunchStatus(`${zhCN.toolLaunch.started}: ${result.execution.id}`);
      setExecutionStatus(`${zhCN.execution.selected}: ${result.execution.id}`);
      await refresh();
      window.setTimeout(() => void refresh(), 1000);
    } catch {
      setToolLaunchStatus(zhCN.toolLaunch.failed);
    }
  }

  async function refreshAIChat() {
    setChatStatus(zhCN.aiChat.sending);
    try {
      const chat = await fetchAIChat();
      setChatMessages(chat.messages);
      setChatSummary(chat.summary);
      setChatStatus('');
    } catch {
      setChatStatus(zhCN.aiChat.loadFailed);
    }
  }

  async function sendChatMessage() {
    const message = chatDraft.trim();
    if (!message) {
      return;
    }
    setChatStatus(zhCN.aiChat.sending);
    const optimistic: ChatMessage = {
      id: `local-${Date.now()}`,
      role: 'user',
      content: message,
      timestamp: Math.floor(Date.now() / 1000),
    };
    setChatMessages(current => [...current, optimistic]);
    setChatDraft('');
    try {
      const response = await sendAIChatMessage(message);
      setChatMessages(current => [...current, response.message]);
      setChatRefs(response.context ?? []);
      setChatStatus('');
      await refresh();
    } catch {
      setChatStatus(zhCN.aiChat.sendFailed);
    }
  }

  async function runSelectedAIAction(action: AIAction) {
    const request = buildAIActionRequest(action, aiActionInputs);
    if (!request) {
      setAIActionStatus(zhCN.aiActions.orchestratorRequired);
      return;
    }
    setAIActionRunningId(action.id);
    setAIActionStatus(`${zhCN.aiActions.running}: ${action.label}`);
    try {
      const result = await runAIAction(request);
      setAIActionResult(result);
      if (result.execution?.id) {
        setExecutionStatus(`${zhCN.execution.selected}: ${result.execution.id}`);
      }
      setAIActionStatus(`${zhCN.aiActions.result}: ${result.result.kind}`);
      await refresh();
      window.setTimeout(() => void refresh(), 1000);
    } catch {
      setAIActionStatus(zhCN.aiActions.runFailed);
    } finally {
      setAIActionRunningId('');
    }
  }

  async function restartBackend() {
    setDesktopStatus(current => ({
      status: 'starting',
      apiBaseURL: current?.apiBaseURL ?? '',
      workspace: current?.workspace ?? '',
      running: current?.running ?? false,
      lastError: '',
      recentWorkspaces: current?.recentWorkspaces ?? [],
      windowState: current?.windowState,
    }));
    const status = await restartDesktopBackend();
    setDesktopStatus(status);
    await refresh();
  }

  useEffect(() => {
    const cleanupMenu = onDesktopMenuAction(action => {
      if (typeof action !== 'string') {
        return;
      }
      if (action.startsWith('mode:')) {
        const nextMode = action.slice('mode:'.length) as IDEMode;
        if (modes.some(item => item.id === nextMode)) {
          setMode(nextMode);
        }
        return;
      }
      if (action === 'refresh' || action === 'workspace:opened') {
        void refresh();
        return;
      }
      if (action === 'backend:restart') {
        void restartBackend();
        return;
      }
      if (action === 'window-state:saved') {
        setWorkspaceActionStatus(zhCN.desktop.windowStateSaved);
      }
    });
    const cleanupBackend = onDesktopBackendStatus(status => {
      setDesktopStatus(status);
    });
    return () => {
      cleanupMenu();
      cleanupBackend();
    };
  }, []);

  const needsWorkspace = ideData.status.api === 'offline' && ideData.workspaceItems.length === 0;
  const desktopWorkspaceActions = Boolean(window.ouroboros?.selectWorkspace || window.ouroboros?.createWorkspace);
  const desktopWindowControlsAvailable = Boolean(window.ouroboros?.minimizeWindow && window.ouroboros?.toggleMaximizeWindow && window.ouroboros?.closeWindow);

  return (
    <div className="ouro-shell">
      <TitleBar
        desktopWindowControlsAvailable={desktopWindowControlsAvailable}
        onOpenPalette={() => setPaletteOpen(true)}
        onOpenWorkspace={() => void openWorkspace()}
        onMinimizeWindow={() => void minimizeDesktopWindow()}
        onToggleMaximizeWindow={() => void toggleMaximizeDesktopWindow()}
        onCloseWindow={() => void closeDesktopWindow()}
      />
      <div className="ouro-body">
        <ActivityBar mode={mode} onModeChange={setMode} />
        <WorkspaceExplorer
          items={ideData.workspaceItems}
          workspace={ideData.status.workspace}
          status={ideData.status}
          searchQuery={searchQuery}
          searchResults={searchResults}
          onSearchQueryChange={setSearchQuery}
          onRunSearch={() => void runSearch()}
          onOpenFile={openWorkspaceFile}
          onOpenSearchResult={result => void openSearchResult(result)}
        />
        <main className="workbench">
          <DesktopStatusBanner desktopStatus={desktopStatus} onRestart={() => void restartBackend()} />
          {needsWorkspace ? (
            <WorkspaceWelcome
              desktopAvailable={desktopWorkspaceActions}
              status={workspaceActionStatus}
              onOpenWorkspace={() => void openWorkspace()}
              onCreateWorkspace={() => void createWorkspace()}
            />
          ) : (
            <>
              {mode === 'mission' && (
                <MissionWorkbench
                  data={ideData}
                  selectedNode={selectedNode}
                  openFile={openFile}
                  fileDraft={fileDraft}
                  fileStatus={fileStatus}
                  commandDraft={commandDraft}
                  missionNameDraft={missionNameDraft}
                  missionGoalDraft={missionGoalDraft}
                  missionCommandDraft={missionCommandDraft}
                  missionStatus={missionStatus}
                  executionStatus={executionStatus}
                  executionLog={executionLog}
                  executionArtifacts={executionArtifacts}
                  executionArtifactsExecutionId={executionArtifactsExecutionId}
                  toolActionArguments={toolActionArguments}
                  toolLaunchStatus={toolLaunchStatus}
                  aiActions={aiActions}
                  aiActionInputs={aiActionInputs}
                  aiActionStatus={aiActionStatus}
                  aiActionResult={aiActionResult}
                  aiActionRunningId={aiActionRunningId}
                  onMissionDraftChange={updateMissionDraft}
                  onSaveMission={() => void saveCurrentMission()}
                  onStartMission={() => void startCurrentMission()}
                  onFileDraftChange={setFileDraft}
                  onSaveFile={() => void saveOpenFile()}
                  onCommandDraftChange={setCommandDraft}
                  onRunCommand={() => void runCommand()}
                  onToolActionArgumentChange={(actionID, value) => setToolActionArguments(current => ({ ...current, [actionID]: value }))}
                  onRunToolAction={action => void runManifestToolAction(action)}
                  onAIActionInputChange={(inputID, value) => setAIActionInputs(current => ({ ...current, [inputID]: value }))}
                  onRefreshAIActions={() => void refreshAIActions()}
                  onRunAIAction={action => void runSelectedAIAction(action)}
                  chatMessages={chatMessages}
                  chatDraft={chatDraft}
                  chatStatus={chatStatus}
                  chatSummary={chatSummary}
                  chatRefs={chatRefs}
                  onChatDraftChange={setChatDraft}
                  onSendChatMessage={() => void sendChatMessage()}
                  onStopExecution={id => void stopSelectedExecution(id)}
                  onRetryExecution={id => void retrySelectedExecution(id)}
                  onLoadExecutionLogs={id => void showExecutionLogs(id)}
                  onLoadExecutionArtifacts={id => void showExecutionArtifacts(id)}
                  onOpenExecutionFile={filePath => void onOpenExecutionFile(filePath)}
                  onFocusGraphNode={onFocusGraphNode}
                  onFocusReplayEvent={onFocusReplayEvent}
                  onFocusReportFinding={onFocusReportFinding}
                />
              )}
              {mode === 'graph' && (
                <GraphWorkbench
                  nodes={ideData.graphNodes}
                  edges={ideData.graphEdges}
                  executions={ideData.executionSteps}
                  findings={ideData.reportFindings}
                  selectedNodeId={selectedNode?.id ?? ''}
                  exportStatus={graphExportStatus}
                  executionLog={executionLog}
                  executionArtifacts={executionArtifacts}
                  executionArtifactsExecutionId={executionArtifactsExecutionId}
                  onSelectNode={setSelectedNodeId}
                  onOpenFindingFile={filePath => void onOpenFindingFile(filePath)}
                  onFocusReplayEvent={onFocusReplayEvent}
                  onInspectExecution={executionId => void inspectExecution(executionId)}
                  onFocusReportFinding={onFocusReportFinding}
                  onLoadExecutionLogs={executionId => void showExecutionLogs(executionId)}
                  onLoadExecutionArtifacts={executionId => void showExecutionArtifacts(executionId)}
                  onOpenExecutionFile={filePath => void onOpenExecutionFile(filePath)}
                  onExportGraph={() => void exportGraph()}
                />
              )}
              {mode === 'replay' && (
                <ReplayWorkbench
                  events={ideData.replayEvents}
                  nodes={ideData.graphNodes}
                  edges={ideData.graphEdges}
                  executions={ideData.executionSteps}
                  findings={ideData.reportFindings}
                  selectedEventId={selectedReplayEventId}
                  selectedEvent={selectedReplayEvent}
                  selectedNodeId={selectedNode?.id ?? ''}
                  executionLog={executionLog}
                  executionArtifacts={executionArtifacts}
                  executionArtifactsExecutionId={executionArtifactsExecutionId}
                  onSelectReplayEvent={setSelectedReplayEventId}
                  onFocusGraphNode={onFocusGraphNode}
                  onOpenFindingFile={filePath => void onOpenFindingFile(filePath)}
                  onInspectExecution={executionId => void inspectExecution(executionId)}
                  onFocusReportFinding={onFocusReportFinding}
                  onLoadExecutionLogs={executionId => void showExecutionLogs(executionId)}
                  onLoadExecutionArtifacts={executionId => void showExecutionArtifacts(executionId)}
                  onOpenExecutionFile={filePath => void onOpenExecutionFile(filePath)}
                />
              )}
              {mode === 'report' && (
                <ReportWorkbench
                  workspace={ideData.workspace}
                  findings={ideData.reportFindings}
                  reports={ideData.reports}
                  executions={ideData.executionSteps}
                  selectedFindingId={selectedReportFindingId}
                  exportStatus={reportExportStatus}
                  executionLog={executionLog}
                  executionArtifacts={executionArtifacts}
                  executionArtifactsExecutionId={executionArtifactsExecutionId}
                  onSelectFinding={setSelectedReportFindingId}
                  onOpenFindingFile={filePath => void onOpenFindingFile(filePath)}
                  onFocusGraphNode={onFocusGraphNode}
                  onFocusReplayEvent={onFocusReplayEvent}
                  onInspectExecution={executionId => void inspectExecution(executionId)}
                  onLoadExecutionLogs={executionId => void showExecutionLogs(executionId)}
                  onLoadExecutionArtifacts={executionId => void showExecutionArtifacts(executionId)}
                  onOpenExecutionFile={filePath => void onOpenExecutionFile(filePath)}
                  onExportReport={() => void exportCurrentReport()}
                />
              )}
              {mode === 'settings' && <SettingsWorkbench data={ideData} onSaveProvider={handleSaveProvider} onSaveToolchain={handleSaveToolchain} onSaveWorkspaceSettings={handleSaveWorkspaceSettings} />}
            </>
          )}
        </main>
        <RightRail data={ideData} selectedNode={selectedNode} onSelectNode={setSelectedNodeId} />
      </div>
      <StatusBar status={ideData.status} />
      {paletteOpen && (
        <CommandPalette
          mode={mode}
          status={ideData.status}
          desktopAvailable={desktopWorkspaceActions}
          recentWorkspaces={desktopStatus?.recentWorkspaces ?? []}
          canRunCommand={commandDraft.trim() !== ''}
          canStartMission={missionCommandDraft.trim() !== ''}
          onClose={() => setPaletteOpen(false)}
          onSelectMode={setMode}
          onRefresh={() => void refresh()}
          onOpenWorkspace={() => void openWorkspace()}
          onOpenRecentWorkspace={path => void openRecentWorkspace(path)}
          onCreateWorkspace={() => void createWorkspace()}
          onRestartBackend={() => void restartBackend()}
          onRunCommand={() => void runCommand()}
          onSaveMission={() => void saveCurrentMission()}
          onStartMission={() => void startCurrentMission()}
        />
      )}
    </div>
  );
}

function aiActionInputKey(actionID: string, field: string) {
  return `${actionID}:${field}`;
}

function aiActionInputValue(inputs: Record<string, string>, actionID: string, field: string) {
  return inputs[aiActionInputKey(actionID, field)] ?? '';
}

function isOrchestratorAIAction(action: AIAction) {
  return action.kind === 'orchestrator' && action.id === 'orchestrator.run';
}

function buildAIOrchestratorInput(action: AIAction, aiActionInputs: Record<string, string>): AIActionRunRequest['input'] | null {
  const title = aiActionInputValue(aiActionInputs, action.id, 'title').trim();
  const goal = aiActionInputValue(aiActionInputs, action.id, 'goal').trim();
  const kind = aiActionInputValue(aiActionInputs, action.id, 'kind').trim();
  const command = aiActionInputValue(aiActionInputs, action.id, 'command').trim();
  const expectedResult = aiActionInputValue(aiActionInputs, action.id, 'expectedResult').trim();
  if (!title || !goal || !kind || !command || !expectedResult) {
    return null;
  }
  const optionalFields = {
    priority: aiActionInputValue(aiActionInputs, action.id, 'priority').trim(),
    source: aiActionInputValue(aiActionInputs, action.id, 'source').trim(),
    filePath: aiActionInputValue(aiActionInputs, action.id, 'filePath').trim(),
  };
  return Object.fromEntries(
    Object.entries({ title, goal, kind, command, expectedResult, ...optionalFields }).filter(([, value]) => value),
  ) as AIActionRunRequest['input'];
}

function buildAIActionRequest(action: AIAction, aiActionInputs: Record<string, string>): AIActionRunRequest | null {
  if (isOrchestratorAIAction(action)) {
    const orchestratorInput = buildAIOrchestratorInput(action, aiActionInputs);
    return orchestratorInput ? { actionId: action.id, input: orchestratorInput } : null;
  }
  return null;
}

function aiActionKindLabel(action: AIAction) {
  if (isOrchestratorAIAction(action)) {
    return zhCN.aiActions.orchestratorKind;
  }
  if (action.kind === 'command') {
    return zhCN.aiActions.commandKind;
  }
  if (action.kind === 'file') {
    return zhCN.aiActions.fileKind;
  }
  if (action.kind === 'report') {
    return zhCN.aiActions.reportKind;
  }
  return zhCN.aiActions.toolKind;
}

function DesktopStatusBanner({
  desktopStatus,
  onRestart,
}: {
  desktopStatus: DesktopStatus | null;
  onRestart: () => void;
}) {
  if (!desktopStatus || desktopStatus.status === 'ready' || desktopStatus.status === 'unavailable') {
    return null;
  }
  if (desktopStatus.status === 'offline' && !desktopStatus.workspace && !desktopStatus.lastError) {
    return null;
  }
  const label = desktopStatus.status === 'crashed'
    ? zhCN.desktop.backendCrashed
    : desktopStatus.status === 'starting'
      ? zhCN.desktop.backendStarting
      : zhCN.desktop.backendOffline;
  const detail = desktopStatus.lastError || desktopStatus.apiBaseURL || desktopStatus.workspace;
  return (
    <section className={`desktop-status-banner ${desktopStatus.status}`}>
      <div>
        <Activity size={15} />
        <strong>{label}</strong>
        {detail && <span>{detail}</span>}
      </div>
      <button type="button" onClick={onRestart}>
        {zhCN.desktop.restartBackend}
      </button>
    </section>
  );
}

function WorkspaceWelcome({
  desktopAvailable,
  status,
  onOpenWorkspace,
  onCreateWorkspace,
}: {
  desktopAvailable: boolean;
  status: string;
  onOpenWorkspace: () => void;
  onCreateWorkspace: () => void;
}) {
  return (
    <section className="workspace-welcome">
      <div className="welcome-copy">
        <p className="eyeline">{zhCN.mission.eyeline}</p>
        <h1>{zhCN.mission.onboardingTitle}</h1>
        <p>{desktopAvailable ? zhCN.mission.onboardingBody : zhCN.mission.onboardingDesktopOnly}</p>
      </div>
      <div className="welcome-actions">
        <button type="button" className="primary-action" onClick={onOpenWorkspace} disabled={!desktopAvailable}>
          <FolderOpen size={16} />
          {zhCN.mission.onboardingOpen}
        </button>
        <button type="button" className="workspace-trigger" onClick={onCreateWorkspace} disabled={!desktopAvailable}>
          <FolderPlus size={16} />
          {zhCN.mission.onboardingCreate}
        </button>
      </div>
      {status && <p className="welcome-status">{status}</p>}
    </section>
  );
}

function TitleBar({
  desktopWindowControlsAvailable,
  onOpenPalette,
  onOpenWorkspace,
  onMinimizeWindow,
  onToggleMaximizeWindow,
  onCloseWindow,
}: {
  desktopWindowControlsAvailable: boolean;
  onOpenPalette: () => void;
  onOpenWorkspace: () => void;
  onMinimizeWindow: () => void;
  onToggleMaximizeWindow: () => void;
  onCloseWindow: () => void;
}) {
  return (
    <header className="title-bar">
      <div className="brand">
        <div className="brand-mark" aria-hidden="true" />
        <div>
          <div className="brand-name">{zhCN.brand.name}</div>
          <div className="brand-subtitle">{zhCN.brand.subtitle}</div>
        </div>
      </div>
      <button className="command-trigger" type="button" onClick={onOpenPalette}>
        <Command size={15} />
        <span>{zhCN.shell.commandPalette}</span>
        <kbd>{zhCN.shell.commandHint}</kbd>
      </button>
      <button className="workspace-trigger" type="button" onClick={onOpenWorkspace}>
        <FileText size={15} />
        <span>{zhCN.shell.openWorkspace}</span>
      </button>
      <div className="window-tools" aria-label={zhCN.shell.windowControls}>
        <button type="button" onClick={onMinimizeWindow} disabled={!desktopWindowControlsAvailable} aria-label={zhCN.windowControls.minimize} title={zhCN.windowControls.minimize}>
          <Minus size={13} />
        </button>
        <button type="button" onClick={onToggleMaximizeWindow} disabled={!desktopWindowControlsAvailable} aria-label={zhCN.windowControls.maximize} title={zhCN.windowControls.maximize}>
          <Maximize2 size={12} />
        </button>
        <button type="button" onClick={onCloseWindow} disabled={!desktopWindowControlsAvailable} aria-label={zhCN.windowControls.close} title={zhCN.windowControls.close}>
          <X size={13} />
        </button>
      </div>
    </header>
  );
}

function ActivityBar({ mode, onModeChange }: { mode: IDEMode; onModeChange: (mode: IDEMode) => void }) {
  return (
    <nav className="activity-bar" aria-label={zhCN.shell.activityLabel}>
      {modes.map(item => {
        const Icon = item.icon;
        return (
          <button
            key={item.id}
            type="button"
            className={mode === item.id ? 'activity-button active' : 'activity-button'}
            onClick={() => onModeChange(item.id)}
            title={zhCN.modes[item.id]}
            aria-label={zhCN.modes[item.id]}
          >
            <Icon size={19} />
          </button>
        );
      })}
    </nav>
  );
}

function WorkspaceExplorer({
  items,
  workspace,
  status,
  searchQuery,
  searchResults,
  onSearchQueryChange,
  onRunSearch,
  onOpenFile,
  onOpenSearchResult,
}: {
  items: WorkspaceItem[];
  workspace: string;
  status: IDEData['status'];
  searchQuery: string;
  searchResults: WorkspaceSearchResult[];
  onSearchQueryChange: (value: string) => void;
  onRunSearch: () => void;
  onOpenFile: (item: WorkspaceItem) => void;
  onOpenSearchResult: (result: WorkspaceSearchResult) => void;
}) {
  return (
    <aside className="workspace-explorer">
      <div className="panel-heading">
        <span>{zhCN.shell.workspace}</span>
        <Search size={14} />
      </div>
      <div className="workspace-title">
        <ChevronRight size={13} />
        <span>{workspace}</span>
      </div>
      <div className="workspace-search">
        <input value={searchQuery} onChange={event => onSearchQueryChange(event.target.value)} onKeyDown={event => { if (event.key === 'Enter') onRunSearch(); }} placeholder={zhCN.search.placeholder} aria-label={zhCN.search.placeholder} />
        <button type="button" onClick={onRunSearch}>{zhCN.shell.search}</button>
      </div>
      {searchResults.length > 0 && (
        <div className="search-results">
          <span>{zhCN.search.results}</span>
          {searchResults.map(result => (
            <button key={`${result.path}:${result.line}`} type="button" onClick={() => onOpenSearchResult(result)}>
              <strong>{result.path}:{result.line}</strong>
              <small>{result.text || zhCN.search.empty}</small>
            </button>
          ))}
        </div>
      )}
      <div className="tree-list">
        {items.length === 0 && <EmptyState text={zhCN.explorer.emptyFiles} />}
        {items.map(item => (
          <button key={item.id} type="button" className={item.active ? 'tree-row active' : 'tree-row'} onClick={() => onOpenFile(item)}>
            <span>{item.label}</span>
            <small>{item.meta}</small>
          </button>
        ))}
      </div>
      <div className="preflight-card">
        <div className="preflight-title">
          <ShieldCheck size={15} />
          <span>{zhCN.explorer.preflight}</span>
        </div>
        <div className="check-row"><CircleDot size={14} /> {zhCN.shell.statusWorkspace}: {status.workspace}</div>
        <div className="check-row"><CircleDot size={14} /> {zhCN.shell.statusBackend}: {status.backend}</div>
        <div className="check-row"><CircleDot size={14} /> {zhCN.shell.statusToolchain}: {status.toolchain}</div>
      </div>
    </aside>
  );
}

function MissionWorkbench({
  data,
  selectedNode,
  openFile,
  fileDraft,
  fileStatus,
  commandDraft,
  missionNameDraft,
  missionGoalDraft,
  missionCommandDraft,
  missionStatus,
  executionStatus,
  executionLog,
  executionArtifacts,
  executionArtifactsExecutionId,
  toolActionArguments,
  toolLaunchStatus,
  aiActions,
  aiActionInputs,
  aiActionStatus,
  aiActionResult,
  aiActionRunningId,
  chatMessages,
  chatDraft,
  chatStatus,
  chatSummary,
  chatRefs,
  onChatDraftChange,
  onSendChatMessage,
  onMissionDraftChange,
  onSaveMission,
  onStartMission,
  onFileDraftChange,
  onSaveFile,
  onCommandDraftChange,
  onRunCommand,
  onToolActionArgumentChange,
  onRunToolAction,
  onAIActionInputChange,
  onRefreshAIActions,
  onRunAIAction,
  onStopExecution,
  onRetryExecution,
  onLoadExecutionLogs,
  onLoadExecutionArtifacts,
  onOpenExecutionFile,
  onFocusGraphNode,
  onFocusReplayEvent,
  onFocusReportFinding,
}: {
  data: IDEData;
  selectedNode?: GraphNode;
  openFile: WorkspaceFileContent | null;
  fileDraft: string;
  fileStatus: string;
  commandDraft: string;
  missionNameDraft: string;
  missionGoalDraft: string;
  missionCommandDraft: string;
  missionStatus: string;
  executionStatus: string;
  executionLog: ExecutionLog | null;
  executionArtifacts: ExecutionArtifact[];
  executionArtifactsExecutionId: string;
  toolActionArguments: Record<string, string>;
  toolLaunchStatus: string;
  aiActions: AIAction[];
  aiActionInputs: Record<string, string>;
  aiActionStatus: string;
  aiActionResult: AIActionRunResponse | null;
  aiActionRunningId: string;
  chatMessages: ChatMessage[];
  chatDraft: string;
  chatStatus: string;
  chatSummary: string;
  chatRefs: string[];
  onChatDraftChange: (value: string) => void;
  onSendChatMessage: () => void;
  onMissionDraftChange: (field: keyof Pick<MissionState, 'name' | 'goal' | 'command'>, value: string) => void;
  onSaveMission: () => void;
  onStartMission: () => void;
  onFileDraftChange: (value: string) => void;
  onSaveFile: () => void;
  onCommandDraftChange: (value: string) => void;
  onRunCommand: () => void;
  onToolActionArgumentChange: (actionID: string, value: string) => void;
  onRunToolAction: (action: ToolAction) => void;
  onAIActionInputChange: (inputID: string, value: string) => void;
  onRefreshAIActions: () => void;
  onRunAIAction: (action: AIAction) => void;
  onStopExecution: (id: string) => void;
  onRetryExecution: (id: string) => void;
  onLoadExecutionLogs: (id: string) => void;
  onLoadExecutionArtifacts: (id: string) => void;
  onOpenExecutionFile: (filePath: string) => void;
  onFocusGraphNode: (nodeId: string) => void;
  onFocusReplayEvent: (eventId: string) => void;
  onFocusReportFinding: (findingId: string) => void;
}) {
  return (
    <section className="mission-workbench">
      <div className="workbench-header">
        <div>
          <p className="eyeline">{zhCN.mission.eyeline}</p>
          <h1>{zhCN.mission.title}</h1>
        </div>
        <button type="button" className="primary-action" onClick={onRunCommand} disabled={commandDraft.trim() === ''}>
          <Radar size={16} />
          {zhCN.mission.runCommand}
        </button>
      </div>
      <div className="mission-grid">
        <div className="conversation-panel">
          <PanelTitle icon={Command} title={zhCN.mission.conversation} meta={zhCN.mission.conversationMeta} />
          <MissionLaunch
            name={missionNameDraft}
            goal={missionGoalDraft}
            command={missionCommandDraft}
            status={missionStatus}
            onDraftChange={onMissionDraftChange}
            onSaveMission={onSaveMission}
            onStartMission={onStartMission}
          />
          <div>
            <ToolLaunchPanel
              toolActions={data.toolActions}
              toolActionArguments={toolActionArguments}
              status={toolLaunchStatus}
              onArgumentChange={onToolActionArgumentChange}
              onRunToolAction={onRunToolAction}
            />
            <AIActionPanel
              actions={aiActions}
              inputs={aiActionInputs}
              status={aiActionStatus}
              result={aiActionResult}
              runningActionId={aiActionRunningId}
              onInputChange={onAIActionInputChange}
              onRefreshActions={onRefreshAIActions}
              onRunAction={onRunAIAction}
            />
          </div>
          <AIChatPanel
            messages={chatMessages}
            draft={chatDraft}
            status={chatStatus}
            summary={chatSummary}
            refs={chatRefs}
            onDraftChange={onChatDraftChange}
            onSendMessage={onSendChatMessage}
          />
          <div className="composer">
            <span>{zhCN.aiChat.title} &gt;</span>
            <input value={chatDraft} onChange={event => onChatDraftChange(event.target.value)} onKeyDown={event => { if (event.key === 'Enter') onSendChatMessage(); }} placeholder={zhCN.aiChat.placeholder} aria-label={zhCN.aiChat.placeholder} />
            <button type="button" onClick={onSendChatMessage} disabled={chatDraft.trim() === ''}>{chatStatus || zhCN.aiChat.send}</button>
          </div>
        </div>
        <div className="mission-side">
          <FileEditor file={openFile} draft={fileDraft} status={fileStatus} onDraftChange={onFileDraftChange} onSave={onSaveFile} />
          <ToolExecution
            steps={data.executionSteps}
            nodes={data.graphNodes}
            events={data.replayEvents}
            findings={data.reportFindings}
            status={executionStatus}
            log={executionLog}
            artifacts={executionArtifacts}
            artifactsExecutionId={executionArtifactsExecutionId}
            onStop={onStopExecution}
            onRetry={onRetryExecution}
            onLoadLogs={onLoadExecutionLogs}
            onLoadArtifacts={onLoadExecutionArtifacts}
            onOpenExecutionFile={onOpenExecutionFile}
            onFocusGraphNode={onFocusGraphNode}
            onFocusReplayEvent={onFocusReplayEvent}
            onFocusReportFinding={onFocusReportFinding}
          />
          <NodeSummary node={selectedNode} />
        </div>
      </div>
    </section>
  );
}

function MissionLaunch({
  name,
  goal,
  command,
  status,
  onDraftChange,
  onSaveMission,
  onStartMission,
}: {
  name: string;
  goal: string;
  command: string;
  status: string;
  onDraftChange: (field: keyof Pick<MissionState, 'name' | 'goal' | 'command'>, value: string) => void;
  onSaveMission: () => void;
  onStartMission: () => void;
}) {
  return (
    <section className="mission-launch">
      <PanelTitle icon={Radar} title={zhCN.missionFlow.title} meta={status || zhCN.missionFlow.meta} />
      <div className="mission-launch-grid">
        <label>
          <span>{zhCN.missionFlow.name}</span>
          <input value={name} onChange={event => onDraftChange('name', event.target.value)} placeholder={zhCN.missionFlow.namePlaceholder} />
        </label>
        <label>
          <span>{zhCN.missionFlow.goal}</span>
          <input value={goal} onChange={event => onDraftChange('goal', event.target.value)} placeholder={zhCN.missionFlow.goalPlaceholder} />
        </label>
        <label className="mission-command">
          <span>{zhCN.missionFlow.command}</span>
          <input value={command} onChange={event => onDraftChange('command', event.target.value)} onKeyDown={event => { if (event.key === 'Enter') onStartMission(); }} placeholder={zhCN.missionFlow.commandPlaceholder} />
        </label>
      </div>
      <div className="mission-launch-actions">
        <button type="button" onClick={onSaveMission}>{zhCN.missionFlow.save}</button>
        <button type="button" className="primary-action" onClick={onStartMission} disabled={command.trim() === ''}>
          <Radar size={15} />
          {zhCN.missionFlow.start}
        </button>
      </div>
    </section>
  );
}

function ToolLaunchPanel({
  toolActions,
  toolActionArguments,
  status,
  onArgumentChange,
  onRunToolAction,
}: {
  toolActions: ToolAction[];
  toolActionArguments: Record<string, string>;
  status: string;
  onArgumentChange: (actionID: string, value: string) => void;
  onRunToolAction: (action: ToolAction) => void;
}) {
  return (
    <section className="tool-launch-panel">
      <PanelTitle icon={TerminalSquare} title={zhCN.toolLaunch.title} meta={status || zhCN.toolLaunch.meta} />
      {toolActions.length === 0 && <EmptyState text={zhCN.toolLaunch.empty} />}
      {toolActions.map(action => (
        <article key={action.id} className="tool-action-row">
          <div className="tool-action-main">
            <strong>{action.label}</strong>
            <code>{action.commandText}</code>
            <small>{action.capabilities.length > 0 ? action.capabilities.join(', ') : action.manifestPath}</small>
          </div>
          <label>
            <span>{zhCN.toolLaunch.arguments}</span>
            <input
              value={toolActionArguments[action.id] ?? ''}
              onChange={event => onArgumentChange(action.id, event.target.value)}
              onKeyDown={event => { if (event.key === 'Enter') onRunToolAction(action); }}
              placeholder={action.argumentHint || zhCN.toolLaunch.argumentPlaceholder}
              aria-label={`${zhCN.toolLaunch.arguments}: ${action.label}`}
            />
          </label>
          <button type="button" onClick={() => onRunToolAction(action)}>
            <Radar size={14} />
            {zhCN.toolLaunch.run}
          </button>
        </article>
      ))}
    </section>
  );
}

function AIActionPanel({
  actions,
  inputs,
  status,
  result,
  runningActionId,
  onInputChange,
  onRefreshActions,
  onRunAction,
}: {
  actions: AIAction[];
  inputs: Record<string, string>;
  status: string;
  result: AIActionRunResponse | null;
  runningActionId: string;
  onInputChange: (inputID: string, value: string) => void;
  onRefreshActions: () => void;
  onRunAction: (action: AIAction) => void;
}) {
  const orchestratorActions = actions.filter(isOrchestratorAIAction);
  return (
    <section className="tool-launch-panel">
      <PanelTitle icon={Network} title={zhCN.aiActions.title} meta={status || zhCN.aiActions.meta} />
      <div className="row-actions">
        <button type="button" onClick={onRefreshActions}>
          <RefreshCw size={14} />
          {zhCN.commandPalette.refresh}
        </button>
      </div>
      {orchestratorActions.length === 0 && <EmptyState text={status || zhCN.aiActions.noOrchestratorAction} />}
      {orchestratorActions.map(action => {
        const running = runningActionId === action.id;
        return (
          <article key={action.id} className="tool-action-row" style={{ gridTemplateColumns: 'minmax(180px, 0.8fr) minmax(320px, 1.2fr)', alignItems: 'start' }}>
            <div className="tool-action-main">
              <strong>{action.label}</strong>
              <DetailGrid rows={[
                [zhCN.aiActions.actionKind, aiActionKindLabel(action)],
                [zhCN.aiActions.actionStatus, action.status],
                [zhCN.aiActions.id, action.id],
              ]} />
              {action.description && <small>{action.description}</small>}
            </div>
            <div>
              <OrchestratorActionInput action={action} inputs={inputs} onInputChange={onInputChange} onRunAction={onRunAction} />
              <div className="mission-launch-actions">
                <button type="button" className="primary-action" onClick={() => onRunAction(action)} disabled={running || !buildAIActionRequest(action, inputs)}>
                  <Radar size={14} />
                  {running ? zhCN.aiActions.running : zhCN.aiActions.orchestratorRun}
                </button>
              </div>
            </div>
          </article>
        );
      })}
      {result && <AIActionResultPanel result={result} />}
    </section>
  );
}

function OrchestratorActionInput({
  action,
  inputs,
  onInputChange,
  onRunAction,
}: {
  action: AIAction;
  inputs: Record<string, string>;
  onInputChange: (inputID: string, value: string) => void;
  onRunAction: (action: AIAction) => void;
}) {
  const updateStructuredInput = (field: string, value: string) => onInputChange(aiActionInputKey(action.id, field), value);
  const runOnEnter = (event: KeyboardEvent<HTMLInputElement>) => {
    if (event.key === 'Enter') {
      onRunAction(action);
    }
  };
  return (
    <div className="mission-launch-grid">
      <label>
        <span>{zhCN.aiActions.orchestratorTitleInput}</span>
        <input value={aiActionInputValue(inputs, action.id, 'title')} onChange={event => updateStructuredInput('title', event.target.value)} onKeyDown={runOnEnter} placeholder={zhCN.aiActions.orchestratorTitlePlaceholder} />
      </label>
      <label>
        <span>{zhCN.aiActions.orchestratorGoalInput}</span>
        <input value={aiActionInputValue(inputs, action.id, 'goal')} onChange={event => updateStructuredInput('goal', event.target.value)} onKeyDown={runOnEnter} placeholder={zhCN.aiActions.orchestratorGoalPlaceholder} />
      </label>
      <label>
        <span>{zhCN.aiActions.orchestratorKindInput}</span>
        <input value={aiActionInputValue(inputs, action.id, 'kind')} onChange={event => updateStructuredInput('kind', event.target.value)} onKeyDown={runOnEnter} placeholder={zhCN.aiActions.orchestratorKindPlaceholder} />
      </label>
      <label className="mission-command">
        <span>{zhCN.aiActions.orchestratorCommandInput}</span>
        <input value={aiActionInputValue(inputs, action.id, 'command')} onChange={event => updateStructuredInput('command', event.target.value)} onKeyDown={runOnEnter} placeholder={zhCN.aiActions.orchestratorCommandPlaceholder} />
      </label>
      <label className="mission-command">
        <span>{zhCN.aiActions.orchestratorExpectedResultInput}</span>
        <input value={aiActionInputValue(inputs, action.id, 'expectedResult')} onChange={event => updateStructuredInput('expectedResult', event.target.value)} onKeyDown={runOnEnter} placeholder={zhCN.aiActions.orchestratorExpectedResultPlaceholder} />
      </label>
      <label>
        <span>{zhCN.aiActions.orchestratorPriorityInput}</span>
        <input value={aiActionInputValue(inputs, action.id, 'priority')} onChange={event => updateStructuredInput('priority', event.target.value)} onKeyDown={runOnEnter} placeholder={zhCN.aiActions.orchestratorPriorityPlaceholder} />
      </label>
      <label>
        <span>{zhCN.aiActions.orchestratorSourceInput}</span>
        <input value={aiActionInputValue(inputs, action.id, 'source')} onChange={event => updateStructuredInput('source', event.target.value)} onKeyDown={runOnEnter} placeholder={zhCN.aiActions.orchestratorSourcePlaceholder} />
      </label>
      <label>
        <span>{zhCN.aiActions.orchestratorFilePathInput}</span>
        <input value={aiActionInputValue(inputs, action.id, 'filePath')} onChange={event => updateStructuredInput('filePath', event.target.value)} onKeyDown={runOnEnter} placeholder={zhCN.aiActions.orchestratorFilePathPlaceholder} />
      </label>
    </div>
  );
}

function AIActionResultPanel({ result }: { result: AIActionRunResponse }) {
  return (
    <section className="detail-panel compact-detail">
      <PanelTitle icon={Network} title={zhCN.aiActions.result} meta={result.result.kind} />
      <DetailGrid rows={[
        [zhCN.aiActions.actionLabel, result.action.label],
        [zhCN.aiActions.actionStatus, result.action.status],
        [zhCN.aiActions.resultStatus, result.result.kind],
        [zhCN.aiActions.resultSummary, result.result.summary],
        [zhCN.aiActions.executionId, result.execution?.id || result.result.execution?.id],
      ]} />
      {result.result.kind === 'orchestrator_run' && (
        <div className="execution-detail">
          <strong>{zhCN.aiActions.orchestratorRunResult}</strong>
          {result.result.orchestratorRun ? <AIActionOrchestratorRun run={result.result.orchestratorRun} /> : <EmptyState text={zhCN.aiActions.noOrchestratorRun} />}
          {result.result.blackboardItem && <AIActionBlackboardItem item={result.result.blackboardItem} />}
          {result.result.agentTask && <AIActionAgentTask task={result.result.agentTask} />}
          {result.result.harnessCase && <AIActionHarnessCase testCase={result.result.harnessCase} />}
          {result.result.execution && <AIActionExecution execution={result.result.execution} />}
        </div>
      )}
    </section>
  );
}

function AIActionOrchestratorRun({ run }: { run: OrchestratorRun }) {
  return (
    <article>
      <strong>{zhCN.aiActions.orchestratorRunResult}</strong>
      <DetailGrid rows={[
        [zhCN.aiActions.id, run.id],
        [zhCN.aiActions.status, run.status],
        [zhCN.aiActions.titleField, run.title],
        [zhCN.aiActions.goal, run.goal],
        [zhCN.aiActions.kind, run.kind],
        [zhCN.aiActions.command, run.command],
        [zhCN.aiActions.expectedResult, run.expectedResult],
        [zhCN.aiActions.priority, run.priority],
        [zhCN.aiActions.source, run.source],
        [zhCN.aiActions.filePath, run.filePath],
        [zhCN.aiActions.blackboardItemId, run.blackboardItemId],
        [zhCN.aiActions.agentTaskId, run.agentTaskId],
        [zhCN.aiActions.harnessCaseId, run.harnessCaseId],
        [zhCN.aiActions.executionId, run.executionId],
        [zhCN.aiActions.createdAt, formatDetailTime(run.createdAt)],
        [zhCN.aiActions.updatedAt, formatDetailTime(run.updatedAt)],
      ]} />
      <RawFieldList fields={run} />
    </article>
  );
}

function AIActionBlackboardItem({ item }: { item: BlackboardItem }) {
  return (
    <article>
      <strong>{zhCN.aiActions.blackboardItem}</strong>
      <DetailGrid rows={[
        [zhCN.aiActions.id, item.id],
        [zhCN.aiActions.status, item.status],
        [zhCN.aiActions.titleField, item.title],
        [zhCN.aiActions.kind, item.kind],
        [zhCN.aiActions.priority, item.priority],
        [zhCN.aiActions.source, item.source],
        [zhCN.aiActions.filePath, item.filePath],
        [zhCN.aiActions.executionId, item.executionId],
        [zhCN.aiActions.createdAt, formatDetailTime(item.createdAt)],
        [zhCN.aiActions.updatedAt, formatDetailTime(item.updatedAt)],
      ]} />
      {item.content && <p className="detail-body">{item.content}</p>}
      <RawFieldList fields={item} />
    </article>
  );
}

function AIActionAgentTask({ task }: { task: AgentTask }) {
  return (
    <article>
      <strong>{zhCN.aiActions.agentTask}</strong>
      <DetailGrid rows={[
        [zhCN.aiActions.id, task.id],
        [zhCN.aiActions.status, task.status],
        [zhCN.aiActions.titleField, task.title],
        [zhCN.aiActions.goal, task.goal],
        [zhCN.aiActions.kind, task.role],
        [zhCN.aiActions.priority, task.priority],
        [zhCN.aiActions.source, task.source],
        [zhCN.aiActions.filePath, task.filePath],
        [zhCN.aiActions.blackboardItemId, task.blackboardItemId],
        [zhCN.aiActions.executionId, task.executionId],
        [zhCN.aiActions.createdAt, formatDetailTime(task.createdAt)],
        [zhCN.aiActions.updatedAt, formatDetailTime(task.updatedAt)],
      ]} />
      <RawFieldList fields={task} />
    </article>
  );
}

function AIActionHarnessCase({ testCase }: { testCase: HarnessCase }) {
  return (
    <article>
      <strong>{zhCN.aiActions.harnessCase}</strong>
      <DetailGrid rows={[
        [zhCN.aiActions.id, testCase.id],
        [zhCN.aiActions.titleField, testCase.title],
        [zhCN.aiActions.goal, testCase.goal],
        [zhCN.aiActions.kind, testCase.kind],
        [zhCN.aiActions.command, testCase.command],
        [zhCN.aiActions.expectedResult, testCase.expectedResult],
        [zhCN.aiActions.source, testCase.source],
        [zhCN.aiActions.filePath, testCase.filePath],
        [zhCN.aiActions.agentTaskId, testCase.agentTaskId],
        [zhCN.aiActions.blackboardItemId, testCase.blackboardItemId],
        [zhCN.aiActions.createdAt, formatDetailTime(testCase.createdAt)],
        [zhCN.aiActions.updatedAt, formatDetailTime(testCase.updatedAt)],
      ]} />
      <RawFieldList fields={testCase} />
    </article>
  );
}

function AIActionExecution({ execution }: { execution: ExecutionStep }) {
  return (
    <article>
      <strong>{zhCN.aiActions.execution}</strong>
      <DetailGrid rows={[
        [zhCN.aiActions.id, execution.id],
        [zhCN.aiActions.status, zhCN.executionStatus[execution.status]],
        [zhCN.aiActions.titleField, execution.label],
        [zhCN.aiActions.command, execution.command],
        [zhCN.execution.exitCode, execution.exitCode !== undefined ? String(execution.exitCode) : ''],
        [zhCN.details.logPath, execution.logPath],
        [zhCN.details.artifactDir, execution.artifactDir],
        [zhCN.aiActions.createdAt, formatDetailTime(execution.startedAt)],
        [zhCN.aiActions.updatedAt, formatDetailTime(execution.completedAt)],
      ]} />
    </article>
  );
}

function FileEditor({
  file,
  draft,
  status,
  onDraftChange,
  onSave,
}: {
  file: WorkspaceFileContent | null;
  draft: string;
  status: string;
  onDraftChange: (value: string) => void;
  onSave: () => void;
}) {
  return (
    <section className="file-editor-panel">
      <PanelTitle icon={FileText} title={zhCN.editor.title} meta={status} />
      {!file && <EmptyState text={zhCN.editor.empty} />}
      {file && (
        <div className="file-editor-body">
          <EvidenceFileDetail file={file} />
          <textarea value={draft} onChange={event => onDraftChange(event.target.value)} aria-label={file.path} spellCheck={false} />
          <button type="button" onClick={onSave}>{zhCN.shell.save}</button>
        </div>
      )}
    </section>
  );
}

function ToolExecution({
  steps,
  nodes,
  events,
  findings,
  status,
  log,
  artifacts,
  artifactsExecutionId,
  onStop,
  onRetry,
  onLoadLogs,
  onLoadArtifacts,
  onOpenExecutionFile,
  onFocusGraphNode,
  onFocusReplayEvent,
  onFocusReportFinding,
}: {
  steps: ExecutionStep[];
  nodes: GraphNode[];
  events: ReplayEvent[];
  findings: ReportFinding[];
  status: string;
  log: ExecutionLog | null;
  artifacts: ExecutionArtifact[];
  artifactsExecutionId: string;
  onStop: (id: string) => void;
  onRetry: (id: string) => void;
  onLoadLogs: (id: string) => void;
  onLoadArtifacts: (id: string) => void;
  onOpenExecutionFile: (filePath: string) => void;
  onFocusGraphNode: (nodeId: string) => void;
  onFocusReplayEvent: (eventId: string) => void;
  onFocusReportFinding: (findingId: string) => void;
}) {
  const activeStep = steps.find(step => step.id === log?.id) ?? steps.find(step => step.id === artifactsExecutionId) ?? steps[0];
  const referenceTargets: ExecutionReferenceTargets = activeStep ? {
    node: nodes.find(node => node.executionId === activeStep.id),
    event: events.find(event => event.executionId === activeStep.id),
    finding: findings.find(finding => finding.executionId === activeStep.id),
  } : {};
  return (
    <section className="execution-panel">
      <PanelTitle icon={TerminalSquare} title={zhCN.mission.execution} meta={status || zhCN.mission.executionMeta} />
      {steps.length === 0 && <EmptyState text={zhCN.execution.empty} />}
      {activeStep && (
        <ExecutionDetailPanel
          step={activeStep}
          log={log}
          artifacts={artifacts}
          references={referenceTargets}
          onFocusGraphNode={onFocusGraphNode}
          onFocusReplayEvent={onFocusReplayEvent}
          onFocusReportFinding={onFocusReportFinding}
        />
      )}
      {steps.map(step => (
        <div key={step.id} className={`execution-step ${step.status}`}>
          <CircleDot size={13} />
          <div>
            <strong>{step.label}</strong>
            <code>{step.command}</code>
            {step.exitCode !== undefined && <small>{zhCN.execution.exitCode}: {step.exitCode}</small>}
          </div>
          <div className="execution-actions">
            {step.status === 'running' && <button type="button" onClick={() => onStop(step.id)}>{zhCN.execution.stop}</button>}
            <button type="button" onClick={() => onRetry(step.id)}>{zhCN.execution.retry}</button>
            <button type="button" onClick={() => onLoadLogs(step.id)}>{zhCN.execution.logs}</button>
            <button type="button" onClick={() => onLoadArtifacts(step.id)}>{zhCN.execution.artifacts}</button>
          </div>
          <span>{zhCN.executionStatus[step.status]}</span>
        </div>
      ))}
      {(log || artifacts.length > 0) && (
        <div className="execution-detail">
          {log && (
            <div>
              <strong>{zhCN.execution.logs}: {log.path}</strong>
              <button type="button" onClick={() => onOpenExecutionFile(log.path)}>{zhCN.execution.openLogFile}</button>
              <pre>{log.content || zhCN.execution.noLogs}</pre>
            </div>
          )}
          {artifacts.length > 0 && (
            <div>
              <strong>{zhCN.execution.artifacts}</strong>
              {artifacts.map(artifact => (
                <article key={artifact.path}>
                  <span>{artifact.path}</span>
                  <small>{artifact.size} bytes</small>
                  {artifact.filePath && <button type="button" onClick={() => onOpenExecutionFile(artifact.filePath as string)}>{zhCN.execution.openArtifactFile}</button>}
                  {artifact.contentPreview && <code>{artifact.contentPreview}</code>}
                </article>
              ))}
            </div>
          )}
        </div>
      )}
    </section>
  );
}

function RightRail({
  data,
  selectedNode,
  onSelectNode,
}: {
  data: IDEData;
  selectedNode?: GraphNode;
  onSelectNode: (id: string) => void;
}) {
  return (
    <aside className="right-rail">
      <section className="rail-section graph-mini">
        <PanelTitle
          icon={GitBranch}
          title={zhCN.mission.attackGraph}
          meta={`${data.graphNodes.length} ${zhCN.graph.nodeCount} / ${data.graphEdges.length} ${zhCN.graph.edgeCount}`}
        />
        <GraphCanvas compact nodes={data.graphNodes} edges={data.graphEdges} selectedNodeId={selectedNode?.id ?? ''} onSelectNode={onSelectNode} />
      </section>
      <section className="rail-section">
        <PanelTitle icon={Activity} title={zhCN.mission.workspaceOverview} meta={zhCN.mission.workspaceOverviewMeta} />
        <div className="workspace-summary-flow">
          <span>{data.workspaceItems.length} {zhCN.shell.workspace}</span>
          <ChevronRight size={14} />
          <span>{data.executionSteps.length} {zhCN.mission.execution}</span>
          <ChevronRight size={14} />
          <span>{data.graphNodes.length} {zhCN.graph.nodeCount}</span>
        </div>
      </section>
      <section className="rail-section inspector">
        <PanelTitle icon={Lock} title={zhCN.mission.inspector} meta={selectedNode ? zhCN.node.kinds[selectedNode.kind] : zhCN.mission.noGraph} />
        <NodeSummary node={selectedNode} />
      </section>
    </aside>
  );
}

function GraphWorkbench({
  nodes,
  edges,
  executions,
  findings,
  selectedNodeId,
  exportStatus,
  executionLog,
  executionArtifacts,
  executionArtifactsExecutionId,
  onSelectNode,
  onOpenFindingFile,
  onFocusReplayEvent,
  onInspectExecution,
  onFocusReportFinding,
  onLoadExecutionLogs,
  onLoadExecutionArtifacts,
  onOpenExecutionFile,
  onExportGraph,
}: {
  nodes: GraphNode[];
  edges: GraphEdge[];
  executions: ExecutionStep[];
  findings: ReportFinding[];
  selectedNodeId: string;
  exportStatus: string;
  executionLog: ExecutionLog | null;
  executionArtifacts: ExecutionArtifact[];
  executionArtifactsExecutionId: string;
  onSelectNode: (id: string) => void;
  onOpenFindingFile: (filePath: string) => void;
  onFocusReplayEvent: (eventId: string) => void;
  onInspectExecution: (executionId: string) => void;
  onFocusReportFinding: (findingId: string) => void;
  onLoadExecutionLogs: (executionId: string) => void;
  onLoadExecutionArtifacts: (executionId: string) => void;
  onOpenExecutionFile: (filePath: string) => void;
  onExportGraph: () => void;
}) {
  const selectedNode = nodes.find(node => node.id === selectedNodeId) ?? nodes[0];
  const graphFinding = findings.find(finding => finding.nodeId === selectedNode?.id)
    ?? findings.find(finding => finding.executionId === selectedNode?.executionId)
    ?? findings.find(finding => finding.replayEventId === selectedNode?.replayEventId);
  const graphExecution = executions.find(execution => execution.id === selectedNode?.executionId)
    ?? executions.find(execution => execution.id === graphFinding?.executionId);
  const graphLog = graphExecution && executionLog?.id === graphExecution.id ? executionLog : null;
  const graphArtifacts = graphExecution && executionArtifactsExecutionId === graphExecution.id ? executionArtifacts : [];

  return (
    <section className="graph-workbench">
      <div className="graph-toolbar">
        <strong>{zhCN.graph.toolbarTitle}</strong>
        <div>
          {exportStatus && <span>{exportStatus}</span>}
          <button type="button" onClick={() => onSelectNode(nodes[0]?.id ?? '')}>{zhCN.graph.defaultView}</button>
          <button type="button" onClick={onExportGraph}>{zhCN.graph.export}</button>
        </div>
      </div>
      <div className="graph-workbench-body">
        <GraphCanvas nodes={nodes} edges={edges} selectedNodeId={selectedNodeId} onSelectNode={onSelectNode} />
        <GraphDetailPanel
          node={selectedNode}
          execution={graphExecution}
          finding={graphFinding}
          log={graphLog}
          artifacts={graphArtifacts}
          onOpenFindingFile={onOpenFindingFile}
          onFocusReplayEvent={onFocusReplayEvent}
          onInspectExecution={onInspectExecution}
          onFocusReportFinding={onFocusReportFinding}
          onLoadExecutionLogs={onLoadExecutionLogs}
          onLoadExecutionArtifacts={onLoadExecutionArtifacts}
          onOpenExecutionFile={onOpenExecutionFile}
        />
      </div>
    </section>
  );
}

function GraphCanvas({
  compact = false,
  nodes,
  edges,
  selectedNodeId,
  onSelectNode,
}: {
  compact?: boolean;
  nodes: GraphNode[];
  edges: GraphEdge[];
  selectedNodeId: string;
  onSelectNode: (id: string) => void;
}) {
  const byID = new Map(nodes.map(node => [node.id, node]));
  return (
    <div className={compact ? 'graph-canvas compact' : 'graph-canvas'}>
      {nodes.length === 0 && <EmptyState text={zhCN.mission.noGraph} />}
      <svg className="graph-lines" viewBox="0 0 100 100" preserveAspectRatio="none" aria-hidden="true">
        {edges.map(edge => {
          const from = byID.get(edge.from);
          const to = byID.get(edge.to);
          if (!from || !to) {
            return null;
          }
          return <path key={edge.id} d={`M${from.x} ${from.y} L${to.x} ${to.y}`} />;
        })}
      </svg>
      {nodes.map(node => (
        <button
          key={node.id}
          type="button"
          className={`graph-node ${node.kind} ${node.id === selectedNodeId ? 'selected' : ''}`}
          style={{ left: `${node.x}%`, top: `${node.y}%` }}
          onClick={() => onSelectNode(node.id)}
        >
          <strong>{node.label}</strong>
          <span>{node.subtitle}</span>
        </button>
      ))}
    </div>
  );
}

function GraphDetailPanel({
  node,
  execution,
  finding,
  log,
  artifacts,
  onOpenFindingFile,
  onFocusReplayEvent,
  onInspectExecution,
  onFocusReportFinding,
  onLoadExecutionLogs,
  onLoadExecutionArtifacts,
  onOpenExecutionFile,
}: {
  node?: GraphNode;
  execution?: ExecutionStep;
  finding?: ReportFinding;
  log: ExecutionLog | null;
  artifacts: ExecutionArtifact[];
  onOpenFindingFile: (filePath: string) => void;
  onFocusReplayEvent: (eventId: string) => void;
  onInspectExecution: (executionId: string) => void;
  onFocusReportFinding: (findingId: string) => void;
  onLoadExecutionLogs: (executionId: string) => void;
  onLoadExecutionArtifacts: (executionId: string) => void;
  onOpenExecutionFile: (filePath: string) => void;
}) {
  if (!node) {
    return <section className="detail-panel"><EmptyState text={zhCN.details.empty} /></section>;
  }
  return (
    <section className="detail-panel">
      <PanelTitle icon={Network} title={zhCN.details.node} meta={node.id} />
      <DetailGrid rows={[
        [zhCN.node.label, node.label],
        [zhCN.node.type, node.subtitle],
        [zhCN.node.risk, node.severity ? zhCN.severity[node.severity] : ''],
        [zhCN.details.source, node.sourceKind],
        [zhCN.details.createdAt, formatDetailTime(node.createdAt)],
        [zhCN.details.path, node.path],
      ]} />
      <h4>{zhCN.details.links}</h4>
      <DetailGrid rows={[
        [zhCN.details.runId, node.runId],
        [zhCN.details.executionId, node.executionId],
        [zhCN.details.replayEventId, node.replayEventId],
        [zhCN.details.finding, finding?.id],
      ]} />
      <div className="row-actions detail-actions">
        {node.path && <button type="button" onClick={() => onOpenFindingFile(node.path as string)}>{zhCN.navigation.openEvidenceFile}</button>}
        {node.replayEventId && <button type="button" onClick={() => onFocusReplayEvent(node.replayEventId as string)}>{zhCN.navigation.focusReplay}</button>}
        {node.executionId && <button type="button" onClick={() => onInspectExecution(node.executionId as string)}>{zhCN.navigation.inspectExecution}</button>}
        {finding?.id && <button type="button" onClick={() => onFocusReportFinding(finding.id)}>{zhCN.navigation.focusReport}</button>}
      </div>
      {execution && (
        <div className="linked-execution-files">
          <h4>{zhCN.details.relatedExecutionFiles}</h4>
          <DetailGrid rows={[
            [zhCN.details.command, execution.command],
            [zhCN.details.status, zhCN.executionStatus[execution.status]],
            [zhCN.details.logPath, log?.path || execution.logPath],
            [zhCN.details.artifactDir, execution.artifactDir],
          ]} />
          <div className="row-actions detail-actions">
            <button type="button" onClick={() => onLoadExecutionLogs(execution.id)}>{zhCN.execution.loadLogs}</button>
            {execution.logPath && <button type="button" onClick={() => onOpenExecutionFile(execution.logPath as string)}>{zhCN.execution.openLogFile}</button>}
            <button type="button" onClick={() => onLoadExecutionArtifacts(execution.id)}>{zhCN.execution.loadArtifacts}</button>
          </div>
          {artifacts.length > 0 && (
            <div className="linked-artifact-list">
              {artifacts.map(artifact => (
                <article key={artifact.path}>
                  <span>{artifact.path}</span>
                  <small>{artifact.size} bytes</small>
                  {artifact.filePath && <button type="button" onClick={() => onOpenExecutionFile(artifact.filePath as string)}>{zhCN.execution.openArtifactFile}</button>}
                </article>
              ))}
            </div>
          )}
        </div>
      )}
      <RawFieldList fields={node.data} />
    </section>
  );
}

function ReplayDetailPanel({
  event,
  execution,
  finding,
  log,
  artifacts,
  onFocusGraphNode,
  onOpenFindingFile,
  onInspectExecution,
  onFocusReportFinding,
  onLoadExecutionLogs,
  onLoadExecutionArtifacts,
  onOpenExecutionFile,
}: {
  event?: ReplayEvent;
  execution?: ExecutionStep;
  finding?: ReportFinding;
  log: ExecutionLog | null;
  artifacts: ExecutionArtifact[];
  onFocusGraphNode: (nodeId: string) => void;
  onOpenFindingFile: (filePath: string) => void;
  onInspectExecution: (executionId: string) => void;
  onFocusReportFinding: (findingId: string) => void;
  onLoadExecutionLogs: (executionId: string) => void;
  onLoadExecutionArtifacts: (executionId: string) => void;
  onOpenExecutionFile: (filePath: string) => void;
}) {
  if (!event) {
    return <section className="detail-panel"><EmptyState text={zhCN.details.empty} /></section>;
  }
  return (
    <section className="detail-panel">
      <PanelTitle icon={Play} title={zhCN.details.replay} meta={event.id} />
      <DetailGrid rows={[
        [zhCN.details.createdAt, formatDetailTime(event.createdAt) || event.time],
        [zhCN.details.source, event.lane],
        [zhCN.details.runId, event.runId],
        [zhCN.details.path, event.filePath],
      ]} />
      <p className="detail-body">{event.detail}</p>
      <h4>{zhCN.details.links}</h4>
      <DetailGrid rows={[
        [zhCN.node.id, event.nodeId],
        [zhCN.details.executionId, event.executionId],
        [zhCN.details.finding, finding?.id],
      ]} />
      <div className="row-actions detail-actions">
        {event.nodeId && <button type="button" onClick={() => onFocusGraphNode(event.nodeId as string)}>{zhCN.navigation.focusGraph}</button>}
        {event.filePath && <button type="button" onClick={() => onOpenFindingFile(event.filePath as string)}>{zhCN.navigation.openEvidenceFile}</button>}
        {event.executionId && <button type="button" onClick={() => onInspectExecution(event.executionId as string)}>{zhCN.navigation.inspectExecution}</button>}
        {finding?.id && <button type="button" onClick={() => onFocusReportFinding(finding.id)}>{zhCN.navigation.focusReport}</button>}
      </div>
      {execution && (
        <div className="linked-execution-files">
          <h4>{zhCN.details.relatedExecutionFiles}</h4>
          <DetailGrid rows={[
            [zhCN.details.command, execution.command],
            [zhCN.details.status, zhCN.executionStatus[execution.status]],
            [zhCN.details.logPath, log?.path || execution.logPath],
            [zhCN.details.artifactDir, execution.artifactDir],
          ]} />
          <div className="row-actions detail-actions">
            <button type="button" onClick={() => onLoadExecutionLogs(execution.id)}>{zhCN.execution.loadLogs}</button>
            {execution.logPath && <button type="button" onClick={() => onOpenExecutionFile(execution.logPath as string)}>{zhCN.execution.openLogFile}</button>}
            <button type="button" onClick={() => onLoadExecutionArtifacts(execution.id)}>{zhCN.execution.loadArtifacts}</button>
          </div>
          {artifacts.length > 0 && (
            <div className="linked-artifact-list">
              {artifacts.map(artifact => (
                <article key={artifact.path}>
                  <span>{artifact.path}</span>
                  <small>{artifact.size} bytes</small>
                  {artifact.filePath && <button type="button" onClick={() => onOpenExecutionFile(artifact.filePath as string)}>{zhCN.execution.openArtifactFile}</button>}
                </article>
              ))}
            </div>
          )}
        </div>
      )}
    </section>
  );
}

function ReportFindingDetail({
  finding,
  execution,
  log,
  artifacts,
  onOpenFindingFile,
  onFocusGraphNode,
  onFocusReplayEvent,
  onInspectExecution,
  onLoadExecutionLogs,
  onLoadExecutionArtifacts,
  onOpenExecutionFile,
}: {
  finding?: ReportFinding;
  execution?: ExecutionStep;
  log: ExecutionLog | null;
  artifacts: ExecutionArtifact[];
  onOpenFindingFile: (filePath: string) => void;
  onFocusGraphNode: (nodeId: string) => void;
  onFocusReplayEvent: (eventId: string) => void;
  onInspectExecution: (executionId: string) => void;
  onLoadExecutionLogs: (executionId: string) => void;
  onLoadExecutionArtifacts: (executionId: string) => void;
  onOpenExecutionFile: (filePath: string) => void;
}) {
  if (!finding) {
    return <section className="detail-panel"><EmptyState text={zhCN.details.empty} /></section>;
  }
  return (
    <section className="detail-panel">
      <PanelTitle icon={FileText} title={zhCN.details.finding} meta={zhCN.severity[finding.severity]} />
      <DetailGrid rows={[
        [zhCN.node.label, finding.title],
        [zhCN.details.path, finding.endpoint],
        [zhCN.details.source, finding.source],
        [zhCN.details.filePath, finding.filePath || finding.path],
      ]} />
      <h4>{zhCN.details.links}</h4>
      <DetailGrid rows={[
        [zhCN.node.id, finding.nodeId],
        [zhCN.details.runId, finding.runId],
        [zhCN.details.executionId, finding.executionId],
        [zhCN.details.replayEventId, finding.replayEventId],
      ]} />
      <div className="row-actions detail-actions">
        {finding.nodeId && <button type="button" onClick={() => onFocusGraphNode(finding.nodeId as string)}>{zhCN.navigation.focusGraph}</button>}
        {finding.replayEventId && <button type="button" onClick={() => onFocusReplayEvent(finding.replayEventId as string)}>{zhCN.navigation.focusReplay}</button>}
        {finding.filePath && <button type="button" onClick={() => onOpenFindingFile(finding.filePath as string)}>{zhCN.navigation.openEvidenceFile}</button>}
        {finding.executionId && <button type="button" onClick={() => onInspectExecution(finding.executionId as string)}>{zhCN.navigation.inspectExecution}</button>}
      </div>
      {execution && (
        <div className="linked-execution-files">
          <h4>{zhCN.details.relatedExecutionFiles}</h4>
          <DetailGrid rows={[
            [zhCN.details.command, execution.command],
            [zhCN.details.status, zhCN.executionStatus[execution.status]],
            [zhCN.details.logPath, log?.path || execution.logPath],
            [zhCN.details.artifactDir, execution.artifactDir],
          ]} />
          <div className="row-actions detail-actions">
            <button type="button" onClick={() => onLoadExecutionLogs(execution.id)}>{zhCN.execution.loadLogs}</button>
            {execution.logPath && <button type="button" onClick={() => onOpenExecutionFile(execution.logPath as string)}>{zhCN.execution.openLogFile}</button>}
            <button type="button" onClick={() => onLoadExecutionArtifacts(execution.id)}>{zhCN.execution.loadArtifacts}</button>
          </div>
          {artifacts.length > 0 && (
            <div className="linked-artifact-list">
              {artifacts.map(artifact => (
                <article key={artifact.path}>
                  <span>{artifact.path}</span>
                  <small>{artifact.size} bytes</small>
                  {artifact.filePath && <button type="button" onClick={() => onOpenExecutionFile(artifact.filePath as string)}>{zhCN.execution.openArtifactFile}</button>}
                </article>
              ))}
            </div>
          )}
        </div>
      )}
    </section>
  );
}

function EvidenceFileDetail({ file }: { file: WorkspaceFileContent }) {
  return (
    <section className="detail-panel compact-detail">
      <PanelTitle icon={FileText} title={zhCN.details.evidenceFile} meta={file.path} />
      <DetailGrid rows={[
        [zhCN.details.path, file.path],
        [zhCN.node.type, file.kind],
        [zhCN.details.size, file.size !== undefined ? `${file.size} 字节` : ''],
        [zhCN.details.modifiedAt, formatDetailTime(file.modifiedAt)],
      ]} />
    </section>
  );
}

function ExecutionDetailPanel({
  step,
  log,
  artifacts,
  references,
  onFocusGraphNode,
  onFocusReplayEvent,
  onFocusReportFinding,
}: {
  step: ExecutionStep;
  log: ExecutionLog | null;
  artifacts: ExecutionArtifact[];
  references: ExecutionReferenceTargets;
  onFocusGraphNode: (nodeId: string) => void;
  onFocusReplayEvent: (eventId: string) => void;
  onFocusReportFinding: (findingId: string) => void;
}) {
  return (
    <section className="detail-panel compact-detail">
      <PanelTitle icon={TerminalSquare} title={zhCN.details.execution} meta={step.id} />
      <DetailGrid rows={[
        [zhCN.details.command, step.command],
        [zhCN.details.status, zhCN.executionStatus[step.status]],
        [zhCN.execution.exitCode, step.exitCode !== undefined ? String(step.exitCode) : ''],
        [zhCN.details.createdAt, formatDetailTime(step.startedAt)],
        [zhCN.details.logPath, log?.path || step.logPath],
        [zhCN.details.artifactDir, step.artifactDir],
        [zhCN.details.executionId, step.retryOf],
        [zhCN.node.id, references.node?.id],
        [zhCN.details.replayEventId, references.event?.id],
        [zhCN.details.finding, references.finding?.id],
      ]} />
      <div className="row-actions detail-actions">
        {references.node?.id && <button type="button" onClick={() => onFocusGraphNode(references.node?.id as string)}>{zhCN.navigation.focusGraph}</button>}
        {references.event?.id && <button type="button" onClick={() => onFocusReplayEvent(references.event?.id as string)}>{zhCN.navigation.focusReplay}</button>}
        {references.finding?.id && <button type="button" onClick={() => onFocusReportFinding(references.finding?.id as string)}>{zhCN.navigation.focusReport}</button>}
      </div>
      {artifacts.length > 0 && <small className="detail-muted">{zhCN.execution.artifacts}: {artifacts.length}</small>}
    </section>
  );
}

function DetailGrid({ rows }: { rows: Array<[string, string | number | undefined]> }) {
  const visibleRows = rows.filter(([, value]) => value !== undefined && String(value).trim() !== '');
  if (visibleRows.length === 0) {
    return <EmptyState text={zhCN.details.empty} />;
  }
  return (
    <dl className="detail-grid">
      {visibleRows.map(([label, value]) => (
        <div key={`${label}-${String(value)}`}>
          <dt>{label}</dt>
          <dd>{value}</dd>
        </div>
      ))}
    </dl>
  );
}

function RawFieldList({ fields }: { fields?: Record<string, unknown> }) {
  const entries = Object.entries(fields ?? {})
    .filter(([, value]) => value !== undefined && value !== null && String(value).trim() !== '')
    .slice(0, 12);
  if (entries.length === 0) {
    return null;
  }
  return (
    <div className="raw-field-list">
      <h4>{zhCN.details.rawFields}</h4>
      {entries.map(([key, value]) => (
        <code key={key}>{key}: {formatRawValue(value)}</code>
      ))}
    </div>
  );
}

function formatRawValue(value: unknown): string {
  if (typeof value === 'string') {
    return value;
  }
  if (typeof value === 'number' || typeof value === 'boolean') {
    return String(value);
  }
  try {
    return JSON.stringify(value);
  } catch {
    return String(value);
  }
}

function formatDetailTime(unixSeconds?: number): string {
  if (!unixSeconds) {
    return '';
  }
  return new Date(unixSeconds * 1000).toLocaleString('zh-CN', { hour12: false });
}

function ReplayWorkbench({
  events,
  nodes,
  edges,
  executions,
  findings,
  selectedEventId,
  selectedEvent,
  selectedNodeId,
  executionLog,
  executionArtifacts,
  executionArtifactsExecutionId,
  onSelectReplayEvent,
  onFocusGraphNode,
  onOpenFindingFile,
  onInspectExecution,
  onFocusReportFinding,
  onLoadExecutionLogs,
  onLoadExecutionArtifacts,
  onOpenExecutionFile,
}: {
  events: ReplayEvent[];
  nodes: GraphNode[];
  edges: GraphEdge[];
  executions: ExecutionStep[];
  findings: ReportFinding[];
  selectedEventId: string;
  selectedEvent?: ReplayEvent;
  selectedNodeId: string;
  executionLog: ExecutionLog | null;
  executionArtifacts: ExecutionArtifact[];
  executionArtifactsExecutionId: string;
  onSelectReplayEvent: (eventId: string) => void;
  onFocusGraphNode: (nodeId: string) => void;
  onOpenFindingFile: (filePath: string) => void;
  onInspectExecution: (executionId: string) => void;
  onFocusReportFinding: (findingId: string) => void;
  onLoadExecutionLogs: (executionId: string) => void;
  onLoadExecutionArtifacts: (executionId: string) => void;
  onOpenExecutionFile: (filePath: string) => void;
}) {
  const replayExecution = executions.find(execution => execution.id === selectedEvent?.executionId);
  const replayFinding = findings.find(finding => finding.replayEventId === selectedEvent?.id);
  const replayLog = replayExecution && executionLog?.id === replayExecution.id ? executionLog : null;
  const replayArtifacts = replayExecution && executionArtifactsExecutionId === replayExecution.id ? executionArtifacts : [];

  return (
    <section className="mode-workbench">
      <div className="timeline">
        <div className="timeline-header">
          <strong>{zhCN.replay.timeline}</strong>
          <span>{events.length} {zhCN.replay.events}</span>
        </div>
        <div className="timeline-track"><span /></div>
      </div>
      <div className="split-workbench">
        <div className="event-table">
          <PanelTitle icon={Play} title={zhCN.replay.events} meta={`${events.length} ${zhCN.replay.events}`} />
          {events.length === 0 && <EmptyState text={zhCN.replay.empty} />}
          {events.map(event => (
            <div key={event.id} className={event.id === selectedEventId ? 'event-row active' : 'event-row'}>
              <span>{event.time}</span>
              <button type="button" className="detail-link" onClick={() => onSelectReplayEvent(event.id)}>{event.event}</button>
              <small>{event.detail}</small>
              <div className="row-actions">
                {event.nodeId && <button type="button" onClick={() => onFocusGraphNode(event.nodeId as string)}>{zhCN.navigation.focusGraph}</button>}
                {event.filePath && <button type="button" onClick={() => onOpenFindingFile(event.filePath as string)}>{zhCN.navigation.openEvidenceFile}</button>}
                {event.executionId && <button type="button" onClick={() => onInspectExecution(event.executionId as string)}>{zhCN.navigation.inspectExecution}</button>}
              </div>
            </div>
          ))}
        </div>
        <div className="snapshot-panel">
          <PanelTitle icon={GitBranch} title={zhCN.replay.snapshot} meta={zhCN.graph.toolbarTitle} />
          <GraphCanvas nodes={nodes} edges={edges} selectedNodeId={selectedNodeId} onSelectNode={onFocusGraphNode} />
          <ReplayDetailPanel
            event={selectedEvent}
            execution={replayExecution}
            finding={replayFinding}
            log={replayLog}
            artifacts={replayArtifacts}
            onFocusGraphNode={onFocusGraphNode}
            onOpenFindingFile={onOpenFindingFile}
            onInspectExecution={onInspectExecution}
            onFocusReportFinding={onFocusReportFinding}
            onLoadExecutionLogs={onLoadExecutionLogs}
            onLoadExecutionArtifacts={onLoadExecutionArtifacts}
            onOpenExecutionFile={onOpenExecutionFile}
          />
        </div>
      </div>
    </section>
  );
}

type ReportSectionID = 'summary' | 'project' | 'method' | 'findings' | 'risk' | 'remediation' | 'appendix';

const reportSections: Array<{ id: ReportSectionID; label: string }> = [
  { id: 'summary', label: zhCN.report.sections.summary },
  { id: 'project', label: zhCN.report.sections.project },
  { id: 'method', label: zhCN.report.sections.method },
  { id: 'findings', label: zhCN.report.sections.findings },
  { id: 'risk', label: zhCN.report.sections.risk },
  { id: 'remediation', label: zhCN.report.sections.remediation },
  { id: 'appendix', label: zhCN.report.sections.appendix },
];

function ReportWorkbench({
  workspace,
  findings,
  reports,
  executions,
  selectedFindingId,
  exportStatus,
  executionLog,
  executionArtifacts,
  executionArtifactsExecutionId,
  onSelectFinding,
  onOpenFindingFile,
  onFocusGraphNode,
  onFocusReplayEvent,
  onInspectExecution,
  onLoadExecutionLogs,
  onLoadExecutionArtifacts,
  onOpenExecutionFile,
  onExportReport,
}: {
  workspace: WorkspaceSettings;
  findings: ReportFinding[];
  reports: ReportDocument[];
  executions: ExecutionStep[];
  selectedFindingId: string;
  exportStatus: string;
  executionLog: ExecutionLog | null;
  executionArtifacts: ExecutionArtifact[];
  executionArtifactsExecutionId: string;
  onSelectFinding: (findingId: string) => void;
  onOpenFindingFile: (filePath: string) => void;
  onFocusGraphNode: (nodeId: string) => void;
  onFocusReplayEvent: (eventId: string) => void;
  onInspectExecution: (executionId: string) => void;
  onLoadExecutionLogs: (executionId: string) => void;
  onLoadExecutionArtifacts: (executionId: string) => void;
  onOpenExecutionFile: (filePath: string) => void;
  onExportReport: () => void;
}) {
  const [reportActiveSection, setReportActiveSection] = useState<ReportSectionID>('findings');
  const selectedFinding = findings.find(finding => finding.id === selectedFindingId) ?? findings[0];
  const findingExecution = executions.find(execution => execution.id === selectedFinding?.executionId);
  const findingLog = findingExecution && executionLog?.id === findingExecution.id ? executionLog : null;
  const findingArtifacts = findingExecution && executionArtifactsExecutionId === findingExecution.id ? executionArtifacts : [];

  function onSelectReportSection(sectionID: ReportSectionID) {
    setReportActiveSection(sectionID);
  }

  return (
    <section className="report-workbench">
      <aside className="report-toc">
        {reportSections.map((section, index) => (
          <button
            key={section.id}
            type="button"
            className={section.id === reportActiveSection ? 'active' : ''}
            onClick={() => onSelectReportSection(section.id)}
          >
            {index + 1}. {section.label}
          </button>
        ))}
      </aside>
      <article className="report-page">
        <div className="report-masthead">
          <div className="report-logo" />
          <div>
            <h2>{zhCN.report.mastheadTitle}</h2>
            <p>{zhCN.report.mastheadSubtitle}</p>
          </div>
          <div className="report-actions">
            {exportStatus && <span>{exportStatus}</span>}
            <button type="button" onClick={onExportReport}>{zhCN.report.export}</button>
          </div>
        </div>
        <ReportSectionContent
          section={reportActiveSection}
          workspace={workspace}
          findings={findings}
          reports={reports}
          executions={executions}
          selectedFinding={selectedFinding}
          findingExecution={findingExecution}
          findingLog={findingLog}
          findingArtifacts={findingArtifacts}
          onSelectFinding={onSelectFinding}
          onOpenFindingFile={onOpenFindingFile}
          onFocusGraphNode={onFocusGraphNode}
          onFocusReplayEvent={onFocusReplayEvent}
          onInspectExecution={onInspectExecution}
          onLoadExecutionLogs={onLoadExecutionLogs}
          onLoadExecutionArtifacts={onLoadExecutionArtifacts}
          onOpenExecutionFile={onOpenExecutionFile}
        />
      </article>
    </section>
  );
}

function ReportSectionContent({
  section,
  workspace,
  findings,
  reports,
  executions,
  selectedFinding,
  findingExecution,
  findingLog,
  findingArtifacts,
  onSelectFinding,
  onOpenFindingFile,
  onFocusGraphNode,
  onFocusReplayEvent,
  onInspectExecution,
  onLoadExecutionLogs,
  onLoadExecutionArtifacts,
  onOpenExecutionFile,
}: {
  section: ReportSectionID;
  workspace: WorkspaceSettings;
  findings: ReportFinding[];
  reports: ReportDocument[];
  executions: ExecutionStep[];
  selectedFinding?: ReportFinding;
  findingExecution?: ExecutionStep;
  findingLog: ExecutionLog | null;
  findingArtifacts: ExecutionArtifact[];
  onSelectFinding: (findingId: string) => void;
  onOpenFindingFile: (filePath: string) => void;
  onFocusGraphNode: (nodeId: string) => void;
  onFocusReplayEvent: (eventId: string) => void;
  onInspectExecution: (executionId: string) => void;
  onLoadExecutionLogs: (executionId: string) => void;
  onLoadExecutionArtifacts: (executionId: string) => void;
  onOpenExecutionFile: (filePath: string) => void;
}) {
  const severityCounts = countFindingsBySeverity(findings);
  const evidenceRows = findings.flatMap(finding => {
    const filePath = finding.filePath || finding.path;
    if (!filePath) {
      return [];
    }
    return [{
      finding,
      filePath,
      referenceId: finding.executionId || finding.replayEventId || finding.nodeId,
    }];
  });

  return (
    <div className="report-section-content" data-report-section={section}>
      {section === 'summary' && (
        <>
          <h3>{zhCN.report.sectionHeadings.summary}</h3>
          <p>{zhCN.report.summaryText}</p>
          <div className="report-metric-grid">
            <MetricCard label={zhCN.report.fields.workspaceName} value={workspace.name} />
            <MetricCard label={zhCN.report.fields.findings} value={findings.length} />
            <MetricCard label={zhCN.report.fields.executions} value={executions.length} />
            <MetricCard label={zhCN.report.fields.reports} value={reports.length} />
          </div>
        </>
      )}

      {section === 'project' && (
        <>
          <h3>{zhCN.report.sectionHeadings.project}</h3>
          <DetailGrid rows={[
            [zhCN.report.fields.workspaceName, workspace.name],
            [zhCN.report.fields.workspacePath, workspace.path],
            [zhCN.report.fields.workspaceStatus, workspace.status],
            [zhCN.report.fields.workspaceConfig, workspace.configPath],
            [zhCN.report.fields.description, workspace.description],
            [zhCN.report.fields.operator, workspace.operator],
            [zhCN.report.fields.tags, workspace.tags.join(', ')],
          ]} />
        </>
      )}

      {section === 'method' && (
        <>
          <h3>{zhCN.report.sectionHeadings.method}</h3>
          {executions.length === 0 && <EmptyState text={zhCN.execution.empty} />}
          <div className="report-list">
            {executions.map(execution => (
              <article key={execution.id}>
                <strong>{execution.label}</strong>
                <code>{execution.command}</code>
                <span>{zhCN.executionStatus[execution.status]}</span>
                <button type="button" onClick={() => onInspectExecution(execution.id)}>{zhCN.navigation.inspectExecution}</button>
              </article>
            ))}
          </div>
        </>
      )}

      {section === 'findings' && (
        <>
          <h3>{zhCN.report.sectionHeadings.findings}</h3>
          <FindingTable
            findings={findings}
            selectedFinding={selectedFinding}
            onSelectFinding={onSelectFinding}
            onOpenFindingFile={onOpenFindingFile}
            onFocusGraphNode={onFocusGraphNode}
            onFocusReplayEvent={onFocusReplayEvent}
            onInspectExecution={onInspectExecution}
          />
          <ReportFindingDetail
            finding={selectedFinding}
            execution={findingExecution}
            log={findingLog}
            artifacts={findingArtifacts}
            onOpenFindingFile={onOpenFindingFile}
            onFocusGraphNode={onFocusGraphNode}
            onFocusReplayEvent={onFocusReplayEvent}
            onInspectExecution={onInspectExecution}
            onLoadExecutionLogs={onLoadExecutionLogs}
            onLoadExecutionArtifacts={onLoadExecutionArtifacts}
            onOpenExecutionFile={onOpenExecutionFile}
          />
        </>
      )}

      {section === 'risk' && (
        <>
          <h3>{zhCN.report.sectionHeadings.risk}</h3>
          <div className="risk-distribution">
            {severityOrder.map(severity => (
              <MetricCard key={severity} label={zhCN.severity[severity]} value={severityCounts[severity]} />
            ))}
          </div>
          <FindingTable
            findings={findings}
            selectedFinding={selectedFinding}
            onSelectFinding={onSelectFinding}
            onOpenFindingFile={onOpenFindingFile}
            onFocusGraphNode={onFocusGraphNode}
            onFocusReplayEvent={onFocusReplayEvent}
            onInspectExecution={onInspectExecution}
          />
        </>
      )}

      {section === 'remediation' && (
        <>
          <h3>{zhCN.report.sectionHeadings.remediation}</h3>
          <EmptyState text={zhCN.report.noRemediation} />
          <FindingTable
            findings={findings}
            selectedFinding={selectedFinding}
            onSelectFinding={onSelectFinding}
            onOpenFindingFile={onOpenFindingFile}
            onFocusGraphNode={onFocusGraphNode}
            onFocusReplayEvent={onFocusReplayEvent}
            onInspectExecution={onInspectExecution}
          />
        </>
      )}

      {section === 'appendix' && (
        <>
          <h3>{zhCN.report.sectionHeadings.appendix}</h3>
          {evidenceRows.length === 0 && executions.length === 0 && <EmptyState text={zhCN.report.noEvidenceLinks} />}
          <div className="report-list">
            {evidenceRows.map(({ finding, filePath, referenceId }) => (
              <article key={finding.id}>
                <strong>{finding.title}</strong>
                <code>{filePath}</code>
                {referenceId && <span>{referenceId}</span>}
                <div className="row-actions">
                  <button type="button" onClick={() => onOpenFindingFile(filePath)}>{zhCN.navigation.openEvidenceFile}</button>
                  {finding.executionId && <button type="button" onClick={() => onInspectExecution(finding.executionId as string)}>{zhCN.navigation.inspectExecution}</button>}
                </div>
              </article>
            ))}
          </div>
          <h3>{zhCN.report.sectionHeadings.reports}</h3>
          {reports.length === 0 && <EmptyState text={zhCN.report.noReports} />}
          <div className="report-list">
            {reports.map(report => (
              <article key={report.id}>
                <strong>{report.title}</strong>
                <code>{report.path}</code>
                <span>{report.status}</span>
                <button type="button" onClick={() => onOpenFindingFile(report.path)}>{zhCN.report.openReportFile}</button>
              </article>
            ))}
          </div>
        </>
      )}
    </div>
  );
}

function FindingTable({
  findings,
  selectedFinding,
  onSelectFinding,
  onOpenFindingFile,
  onFocusGraphNode,
  onFocusReplayEvent,
  onInspectExecution,
}: {
  findings: ReportFinding[];
  selectedFinding?: ReportFinding;
  onSelectFinding: (findingId: string) => void;
  onOpenFindingFile: (filePath: string) => void;
  onFocusGraphNode: (nodeId: string) => void;
  onFocusReplayEvent: (eventId: string) => void;
  onInspectExecution: (executionId: string) => void;
}) {
  return (
    <div className="finding-table">
      {findings.length === 0 && <EmptyState text={zhCN.report.empty} />}
      {findings.map(finding => (
        <div key={finding.id} className={finding.id === selectedFinding?.id ? `finding-row ${finding.severity} active` : `finding-row ${finding.severity}`}>
          <span>{zhCN.severity[finding.severity]}</span>
          <button type="button" className="detail-link" onClick={() => onSelectFinding(finding.id)}>{finding.title}</button>
          <code>{finding.endpoint}</code>
          <div className="row-actions">
            {finding.nodeId && <button type="button" onClick={() => onFocusGraphNode(finding.nodeId as string)}>{zhCN.navigation.focusGraph}</button>}
            {finding.replayEventId && <button type="button" onClick={() => onFocusReplayEvent(finding.replayEventId as string)}>{zhCN.navigation.focusReplay}</button>}
            {finding.filePath && <button type="button" onClick={() => onOpenFindingFile(finding.filePath as string)}>{zhCN.navigation.openEvidenceFile}</button>}
            {finding.executionId && <button type="button" onClick={() => onInspectExecution(finding.executionId as string)}>{zhCN.navigation.inspectExecution}</button>}
          </div>
        </div>
      ))}
    </div>
  );
}

const severityOrder: Severity[] = ['critical', 'high', 'medium', 'low', 'info'];

function countFindingsBySeverity(findings: ReportFinding[]): Record<Severity, number> {
  return findings.reduce<Record<Severity, number>>((counts, finding) => {
    counts[finding.severity] += 1;
    return counts;
  }, { critical: 0, high: 0, medium: 0, low: 0, info: 0 });
}

function MetricCard({ label, value }: { label: string; value: string | number }) {
  return (
    <div className="metric-card">
      <span>{label}</span>
      <strong>{value}</strong>
    </div>
  );
}

type SettingsSectionID = 'status' | 'workspace' | 'provider' | 'toolchain';

const settingsSections: Array<{ id: SettingsSectionID; label: string }> = [
  { id: 'status', label: zhCN.settings.sections.status },
  { id: 'workspace', label: zhCN.settings.sections.workspace },
  { id: 'provider', label: zhCN.settings.sections.provider },
  { id: 'toolchain', label: zhCN.settings.sections.toolchain },
];

function SettingsWorkbench({
  data,
  onSaveProvider,
  onSaveToolchain,
  onSaveWorkspaceSettings,
}: {
  data: IDEData;
  onSaveProvider: (provider: ProviderSettings) => Promise<void>;
  onSaveToolchain: (pack: ToolchainPackSettings) => Promise<void>;
  onSaveWorkspaceSettings: (settings: WorkspaceSettings) => Promise<void>;
}) {
  const [workspaceDraft, setWorkspaceDraft] = useState<WorkspaceSettings>(data.workspace);
  const [workspaceStatus, setWorkspaceStatus] = useState(data.workspace.status);
  const [draft, setDraft] = useState<ProviderSettings>(data.provider);
  const [status, setStatus] = useState(data.provider.status);
  const [selectedToolchainID, setSelectedToolchainID] = useState(data.toolchainPacks[0]?.id ?? '');
  const [toolchainDraft, setToolchainDraft] = useState<ToolchainPackSettings>(data.toolchainPacks[0] ?? emptyToolchainPack());
  const [toolchainStatus, setToolchainStatus] = useState(data.toolchainPacks[0]?.status ?? zhCN.backendStatus.unknown);
  const [settingsActiveSection, setSettingsActiveSection] = useState<SettingsSectionID>('workspace');
  const loadedAt = data.loadedAt
    ? new Date(data.loadedAt).toLocaleString('zh-CN', { hour12: false })
    : '--';

  useEffect(() => {
    setWorkspaceDraft(data.workspace);
    setWorkspaceStatus(data.workspace.status);
  }, [data.workspace]);

  useEffect(() => {
    setDraft(data.provider);
    setStatus(data.provider.status);
  }, [data.provider]);

  useEffect(() => {
    const selected = data.toolchainPacks.find(pack => pack.id === selectedToolchainID) ?? data.toolchainPacks[0] ?? emptyToolchainPack();
    setSelectedToolchainID(selected.id);
    setToolchainDraft(selected);
    setToolchainStatus(selected.status);
  }, [data.toolchainPacks, selectedToolchainID]);

  function updateWorkspaceDraft(field: keyof WorkspaceSettings, value: string | string[]) {
    setWorkspaceDraft(current => ({ ...current, [field]: value }));
  }

  async function saveCurrentWorkspaceSettings() {
    setWorkspaceStatus(zhCN.settings.actions.savingWorkspace);
    try {
      await onSaveWorkspaceSettings(workspaceDraft);
      setWorkspaceStatus(zhCN.settings.actions.workspaceSaved);
    } catch {
      setWorkspaceStatus(zhCN.settings.actions.saveWorkspaceFailed);
    }
  }

  function updateProviderDraft(field: keyof ProviderSettings, value: string | boolean | number | string[]) {
    setDraft(current => ({ ...current, [field]: value }));
  }

  async function saveCurrentProvider() {
    setStatus(zhCN.settings.actions.savingProvider);
    try {
      await onSaveProvider(draft);
      setStatus(zhCN.settings.actions.providerSaved);
    } catch {
      setStatus(zhCN.settings.actions.saveProviderFailed);
    }
  }

  function selectToolchainPack(id: string) {
    const selected = data.toolchainPacks.find(pack => pack.id === id) ?? emptyToolchainPack();
    setSelectedToolchainID(selected.id);
    setToolchainDraft(selected);
    setToolchainStatus(selected.status);
  }

  function updateToolchainDraft(field: keyof ToolchainPackSettings, value: string | number | string[]) {
    setToolchainDraft(current => ({ ...current, [field]: value }));
  }

  async function saveCurrentToolchain() {
    setToolchainStatus(zhCN.settings.actions.savingToolchain);
    try {
      await onSaveToolchain(toolchainDraft);
      setToolchainStatus(zhCN.settings.actions.toolchainSaved);
    } catch {
      setToolchainStatus(zhCN.settings.actions.saveToolchainFailed);
    }
  }

  function onSelectSettingsSection(sectionID: SettingsSectionID) {
    setSettingsActiveSection(sectionID);
  }

  return (
    <section className="settings-workbench">
      <aside className="settings-nav">
        {settingsSections.map(section => (
          <button
            key={section.id}
            type="button"
            className={section.id === settingsActiveSection ? 'active' : ''}
            onClick={() => onSelectSettingsSection(section.id)}
          >
            {section.label}
          </button>
        ))}
      </aside>
      <SettingsSectionContent section={settingsActiveSection}>
        {settingsActiveSection === 'status' && (
          <>
            <SettingsGroup title={zhCN.settings.groups.general}>
              <SettingRow label={zhCN.settings.rows.connection}><SettingValue>{data.status.api}</SettingValue></SettingRow>
              <SettingRow label={zhCN.settings.rows.latency}><SettingValue>{data.status.latency}</SettingValue></SettingRow>
              <SettingRow label={zhCN.settings.rows.loadedAt}><SettingValue>{loadedAt}</SettingValue></SettingRow>
            </SettingsGroup>
            <SettingsGroup title={zhCN.settings.groups.workspace}>
              <SettingRow label={zhCN.settings.rows.workspace}><SettingValue>{data.status.workspace}</SettingValue></SettingRow>
              <SettingRow label={zhCN.settings.rows.workflow}><SettingValue>{data.status.workflow}</SettingValue></SettingRow>
              <SettingRow label={zhCN.settings.rows.source}><SettingValue>/api/gui/bootstrap</SettingValue></SettingRow>
            </SettingsGroup>
            <SettingsGroup title={zhCN.settings.groups.backend}>
              <SettingRow label={zhCN.settings.rows.provider}><SettingValue>{data.status.provider}</SettingValue></SettingRow>
              <SettingRow label={zhCN.settings.rows.toolchain}><SettingValue>{data.status.toolchain}</SettingValue></SettingRow>
              <SettingRow label={zhCN.settings.rows.backend}><SettingValue>{data.status.backend}</SettingValue></SettingRow>
            </SettingsGroup>
          </>
        )}
        {settingsActiveSection === 'workspace' && (
          <SettingsGroup title={zhCN.settings.groups.workspaceSettings}>
            <SettingRow label={zhCN.settings.rows.workspaceName}>
              <input value={workspaceDraft.name} onChange={event => updateWorkspaceDraft('name', event.target.value)} placeholder={zhCN.settings.placeholders.workspaceName} />
            </SettingRow>
            <SettingRow label={zhCN.settings.rows.workspaceDescription}>
              <input value={workspaceDraft.description} onChange={event => updateWorkspaceDraft('description', event.target.value)} placeholder={zhCN.settings.placeholders.workspaceDescription} />
            </SettingRow>
            <SettingRow label={zhCN.settings.rows.workspaceOperator}>
              <input value={workspaceDraft.operator} onChange={event => updateWorkspaceDraft('operator', event.target.value)} placeholder={zhCN.settings.placeholders.workspaceOperator} />
            </SettingRow>
            <SettingRow label={zhCN.settings.rows.workspaceTags}>
              <input value={workspaceDraft.tags.join(', ')} onChange={event => updateWorkspaceDraft('tags', splitCommaList(event.target.value))} placeholder={zhCN.settings.placeholders.workspaceTags} />
            </SettingRow>
            <SettingRow label={zhCN.settings.rows.workspacePath}><SettingValue>{workspaceDraft.path || '--'}</SettingValue></SettingRow>
            <SettingRow label={zhCN.settings.rows.workspaceConfigPath}><SettingValue>{workspaceDraft.configPath || zhCN.settings.notConfigured}</SettingValue></SettingRow>
            <SettingRow label={zhCN.settings.rows.workspaceStatus}><SettingValue>{workspaceStatus}</SettingValue></SettingRow>
            <div className="settings-actions">
              <button type="button" onClick={() => void saveCurrentWorkspaceSettings()}>{zhCN.settings.actions.saveWorkspace}</button>
            </div>
          </SettingsGroup>
        )}
        {settingsActiveSection === 'provider' && (
          <SettingsGroup title={zhCN.settings.groups.provider}>
            <SettingRow label={zhCN.settings.rows.providerName}>
              <input value={draft.name} onChange={event => updateProviderDraft('name', event.target.value)} placeholder={zhCN.settings.placeholders.providerName} />
            </SettingRow>
            <SettingRow label={zhCN.settings.rows.providerType}>
              <select value={draft.type} onChange={event => updateProviderDraft('type', event.target.value)}>
                <option value="">{zhCN.settings.notConfigured}</option>
                <option value="openai_compatible">{zhCN.settings.providerTypes.openaiCompatible}</option>
              </select>
            </SettingRow>
            <SettingRow label={zhCN.settings.rows.providerModel}>
              <input value={draft.model} onChange={event => updateProviderDraft('model', event.target.value)} placeholder={zhCN.settings.placeholders.providerModel} />
            </SettingRow>
            <SettingRow label={zhCN.settings.rows.providerBaseUrl}>
              <input value={draft.baseUrl} onChange={event => updateProviderDraft('baseUrl', event.target.value)} placeholder={zhCN.settings.placeholders.providerBaseUrl} />
            </SettingRow>
            <SettingRow label={zhCN.settings.rows.providerApiKeyEnv}>
              <input value={draft.apiKeyEnv} onChange={event => updateProviderDraft('apiKeyEnv', event.target.value)} placeholder={zhCN.settings.placeholders.providerApiKeyEnv} />
            </SettingRow>
            <SettingRow label={zhCN.settings.rows.providerEnabled}>
              <input type="checkbox" checked={draft.enabled} onChange={event => updateProviderDraft('enabled', event.target.checked)} />
            </SettingRow>
            <SettingRow label={zhCN.settings.rows.providerTimeout}>
              <input type="number" min="0" value={draft.timeoutMs} onChange={event => updateProviderDraft('timeoutMs', Number(event.target.value || 0))} />
            </SettingRow>
            <SettingRow label={zhCN.settings.rows.providerContext}>
              <input type="number" min="0" value={draft.contextWindowTokens} onChange={event => updateProviderDraft('contextWindowTokens', Number(event.target.value || 0))} />
            </SettingRow>
            <SettingRow label={zhCN.settings.rows.providerCapabilities}>
              <input value={draft.capabilities.join(', ')} onChange={event => updateProviderDraft('capabilities', event.target.value.split(',').map(item => item.trim()).filter(Boolean))} placeholder={zhCN.settings.placeholders.providerCapabilities} />
            </SettingRow>
            <SettingRow label={zhCN.settings.rows.providerRouting}>
              <select value={draft.routingStrategy} onChange={event => updateProviderDraft('routingStrategy', event.target.value)}>
                <option value="">{zhCN.settings.notConfigured}</option>
                <option value="first_enabled">{zhCN.settings.routing.firstEnabled}</option>
                <option value="enabled_order_failover">{zhCN.settings.routing.failover}</option>
              </select>
            </SettingRow>
            <SettingRow label={zhCN.settings.rows.providerStatus}><SettingValue>{status}</SettingValue></SettingRow>
            <div className="settings-actions">
              <button type="button" onClick={() => void saveCurrentProvider()}>{zhCN.settings.actions.saveProvider}</button>
            </div>
          </SettingsGroup>
        )}
        {settingsActiveSection === 'toolchain' && (
          <SettingsGroup title={zhCN.settings.groups.toolchain}>
            <SettingRow label={zhCN.settings.rows.toolchainPack}>
              <select value={selectedToolchainID} onChange={event => selectToolchainPack(event.target.value)}>
                {data.toolchainPacks.map(pack => (
                  <option key={pack.id} value={pack.id}>{pack.name}</option>
                ))}
              </select>
            </SettingRow>
            <SettingRow label={zhCN.settings.rows.toolchainName}>
              <input value={toolchainDraft.name} onChange={event => updateToolchainDraft('name', event.target.value)} />
            </SettingRow>
            <SettingRow label={zhCN.settings.rows.toolchainVersion}>
              <input value={toolchainDraft.version} onChange={event => updateToolchainDraft('version', event.target.value)} />
            </SettingRow>
            <SettingRow label={zhCN.settings.rows.toolchainCapabilities}>
              <input value={toolchainDraft.capabilities.join(', ')} onChange={event => updateToolchainDraft('capabilities', splitCommaList(event.target.value))} placeholder={zhCN.settings.placeholders.toolchainCapabilities} />
            </SettingRow>
            <SettingRow label={zhCN.settings.rows.toolchainRunner}>
              <select value={toolchainDraft.runner} onChange={event => updateToolchainDraft('runner', event.target.value)}>
                <option value="">{zhCN.settings.notConfigured}</option>
                <option value="native">{zhCN.settings.toolchainRunners.native}</option>
                <option value="command">{zhCN.settings.toolchainRunners.command}</option>
              </select>
            </SettingRow>
            <SettingRow label={zhCN.settings.rows.toolchainRunCommand}>
              <input value={toolchainDraft.runCommand.join(', ')} onChange={event => updateToolchainDraft('runCommand', splitCommaList(event.target.value))} placeholder={zhCN.settings.placeholders.toolchainRunCommand} />
            </SettingRow>
            <SettingRow label={zhCN.settings.rows.toolchainTimeout}>
              <input type="number" min="0" value={toolchainDraft.runTimeoutMs} onChange={event => updateToolchainDraft('runTimeoutMs', Number(event.target.value || 0))} />
            </SettingRow>
            <SettingRow label={zhCN.settings.rows.toolchainParser}>
              <input value={toolchainDraft.parser} onChange={event => updateToolchainDraft('parser', event.target.value)} />
            </SettingRow>
            <SettingRow label={zhCN.settings.rows.toolchainHealthcheck}>
              <select value={toolchainDraft.healthcheck} onChange={event => updateToolchainDraft('healthcheck', event.target.value)}>
                <option value="">{zhCN.settings.notConfigured}</option>
                <option value="built_in">{zhCN.settings.healthchecks.builtIn}</option>
                <option value="exec">{zhCN.settings.healthchecks.exec}</option>
              </select>
            </SettingRow>
            <SettingRow label={zhCN.settings.rows.toolchainHealthCommand}>
              <input value={toolchainDraft.healthCommand.join(', ')} onChange={event => updateToolchainDraft('healthCommand', splitCommaList(event.target.value))} placeholder={zhCN.settings.placeholders.toolchainHealthCommand} />
            </SettingRow>
            <SettingRow label={zhCN.settings.rows.toolchainManifest}><SettingValue>{toolchainDraft.manifestPath}</SettingValue></SettingRow>
            <SettingRow label={zhCN.settings.rows.toolchainPackStatus}><SettingValue>{toolchainStatus}</SettingValue></SettingRow>
            <div className="settings-actions">
              <button type="button" onClick={() => void saveCurrentToolchain()} disabled={!toolchainDraft.id}>{zhCN.settings.actions.saveToolchain}</button>
            </div>
          </SettingsGroup>
        )}
      </SettingsSectionContent>
    </section>
  );
}

function SettingsSectionContent({ section, children }: { section: SettingsSectionID; children: React.ReactNode }) {
  return (
    <div className="settings-grid" data-settings-section={section}>
      {children}
    </div>
  );
}

function SettingsGroup({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <fieldset className="settings-group">
      <legend>{title}</legend>
      {children}
    </fieldset>
  );
}

function emptyToolchainPack(): ToolchainPackSettings {
  return {
    id: '',
    name: '',
    version: '',
    status: zhCN.backendStatus.unknown,
    toolsReady: 0,
    manifestPath: '',
    capabilities: [],
    runner: '',
    runCommand: [],
    runTimeoutMs: 0,
    parser: '',
    healthcheck: '',
    healthCommand: [],
  };
}

function splitCommaList(value: string): string[] {
  return value.split(',').map(item => item.trim()).filter(Boolean);
}

function SettingRow({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <label className="setting-row">
      <span>{label}</span>
      {children}
    </label>
  );
}

function SettingValue({ children }: { children: React.ReactNode }) {
  return <span className="setting-value">{children}</span>;
}

function NodeSummary({ node }: { node?: GraphNode }) {
  if (!node) {
    return <EmptyState text={zhCN.mission.noGraph} />;
  }
  return (
    <div className="node-summary">
      <div>
        <span>{zhCN.node.id}</span>
        <strong>{node.id}</strong>
      </div>
      <div>
        <span>{zhCN.node.type}</span>
        <strong>{zhCN.node.kinds[node.kind]}</strong>
      </div>
      <div>
        <span>{zhCN.node.label}</span>
        <strong>{node.label}</strong>
      </div>
      <div>
        <span>{zhCN.node.risk}</span>
        <strong>{node.severity ? zhCN.severity[node.severity] : zhCN.node.tracked}</strong>
      </div>
    </div>
  );
}

function PanelTitle({ icon: Icon, title, meta }: { icon: LucideIcon; title: string; meta?: string }) {
  return (
    <div className="panel-title">
      <div>
        <Icon size={15} />
        <span>{title}</span>
      </div>
      {meta && <small>{meta}</small>}
    </div>
  );
}


function formatChatTime(unixSeconds: number): string {
  const date = new Date(unixSeconds * 1000);
  return `${date.getHours().toString().padStart(2, '0')}:${date.getMinutes().toString().padStart(2, '0')}`;
}

function AIChatPanel({
  messages,
  draft,
  status,
  summary,
  refs,
  onDraftChange,
  onSendMessage,
}: {
  messages: ChatMessage[];
  draft: string;
  status: string;
  summary: string;
  refs: string[];
  onDraftChange: (value: string) => void;
  onSendMessage: () => void;
}) {
  return (
    <div className="ai-chat-panel">
      <PanelTitle icon={Command} title={zhCN.aiChat.title} meta={status || zhCN.aiChat.meta} />
      <div className="message-list">
        {messages.length === 0 && <EmptyState text={zhCN.aiChat.empty} />}
        {messages.map(message => (
          <article key={message.id} className={`chat-message ${message.role === 'user' ? 'chat-message-user' : 'chat-message-assistant'}`}>
            <header>
              <strong>{message.role === 'user' ? zhCN.aiChat.user : zhCN.aiChat.assistant}</strong>
              <time>{formatChatTime(message.timestamp)}</time>
            </header>
            <p>{message.content}</p>
            {(message.refs ?? []).map(ref => <small className="chat-ref" key={ref}>{zhCN.aiChat.contextLabel}: {ref}</small>)}
          </article>
        ))}
      </div>
      <div className="chat-workspace-summary">
        <strong>{zhCN.aiChat.workspaceLabel}</strong>
        <pre>{summary || zhCN.mission.noMessages}</pre>
      </div>
    </div>
  );
}

function EmptyState({ text }: { text: string }) {
  return <div className="empty-state">{text}</div>;
}

function CommandPalette({
  mode,
  status,
  desktopAvailable,
  recentWorkspaces,
  canRunCommand,
  canStartMission,
  onClose,
  onSelectMode,
  onRefresh,
  onOpenWorkspace,
  onOpenRecentWorkspace,
  onCreateWorkspace,
  onRestartBackend,
  onRunCommand,
  onSaveMission,
  onStartMission,
}: {
  mode: IDEMode;
  status: IDEData['status'];
  desktopAvailable: boolean;
  recentWorkspaces: string[];
  canRunCommand: boolean;
  canStartMission: boolean;
  onClose: () => void;
  onSelectMode: (mode: IDEMode) => void;
  onRefresh: () => void;
  onOpenWorkspace: () => void;
  onOpenRecentWorkspace: (path: string) => void;
  onCreateWorkspace: () => void;
  onRestartBackend: () => void;
  onRunCommand: () => void;
  onSaveMission: () => void;
  onStartMission: () => void;
}) {
  const [query, setQuery] = useState('');
  const paletteActions = useMemo<CommandPaletteAction[]>(() => [
    {
      id: 'refresh',
      label: zhCN.commandPalette.refresh,
      meta: status.workspace,
      icon: RefreshCw,
      run: onRefresh,
    },
    {
      id: 'open-workspace',
      label: zhCN.commandPalette.openWorkspace,
      meta: zhCN.commandPalette.desktopAction,
      icon: FolderOpen,
      disabled: !desktopAvailable,
      run: onOpenWorkspace,
    },
    ...recentWorkspaces.map(path => ({
      id: `recent-workspace-${path}`,
      label: zhCN.commandPalette.openRecentWorkspace,
      meta: path,
      icon: FolderOpen,
      disabled: !desktopAvailable,
      run: () => onOpenRecentWorkspace(path),
    })),
    {
      id: 'create-workspace',
      label: zhCN.commandPalette.createWorkspace,
      meta: zhCN.commandPalette.desktopAction,
      icon: FolderPlus,
      disabled: !desktopAvailable,
      run: onCreateWorkspace,
    },
    {
      id: 'restart-backend',
      label: zhCN.commandPalette.restartBackend,
      meta: status.backend,
      icon: Activity,
      run: onRestartBackend,
    },
    {
      id: 'run-command',
      label: zhCN.commandPalette.runCommand,
      meta: status.workspace,
      icon: TerminalSquare,
      disabled: !canRunCommand,
      run: onRunCommand,
    },
    {
      id: 'save-mission',
      label: zhCN.commandPalette.saveMission,
      meta: status.workflow,
      icon: Save,
      run: onSaveMission,
    },
    {
      id: 'start-mission',
      label: zhCN.commandPalette.startMission,
      meta: status.workflow,
      icon: Play,
      disabled: !canStartMission,
      run: onStartMission,
    },
    ...modes.map(item => ({
      id: `mode-${item.id}`,
      label: `${zhCN.commandPalette.switchTo}${zhCN.modes[item.id]}`,
      meta: item.id === mode ? zhCN.commandPalette.currentMode : zhCN.commandPalette.modeAction,
      icon: item.icon,
      run: () => onSelectMode(item.id),
    })),
  ], [canRunCommand, canStartMission, desktopAvailable, mode, onCreateWorkspace, onOpenRecentWorkspace, onOpenWorkspace, onRefresh, onRestartBackend, onRunCommand, onSaveMission, onSelectMode, onStartMission, recentWorkspaces, status.backend, status.workflow, status.workspace]);
  const normalizedQuery = query.trim().toLowerCase();
  const visibleActions = normalizedQuery
    ? paletteActions.filter(action => `${action.label} ${action.meta}`.toLowerCase().includes(normalizedQuery))
    : paletteActions;

  async function executeAction(action: CommandPaletteAction) {
    if (action.disabled) {
      return;
    }
    await action.run();
    onClose();
  }

  async function executeFirstAction() {
    const action = visibleActions.find(item => !item.disabled);
    if (action) {
      await executeAction(action);
    }
  }

  return (
    <div className="palette-backdrop" role="presentation" onClick={onClose}>
      <div className="command-palette" role="dialog" aria-modal="true" aria-label={zhCN.shell.commandPalette} onClick={event => event.stopPropagation()}>
        <div className="palette-input">
          <Command size={17} />
          <input
            autoFocus
            value={query}
            onChange={event => setQuery(event.target.value)}
            onKeyDown={event => { if (event.key === 'Enter') void executeFirstAction(); }}
            placeholder={zhCN.commandPalette.placeholder}
            aria-label={zhCN.shell.commandPalette}
          />
        </div>
        {visibleActions.map(action => {
          const Icon = action.icon;
          return (
            <button
              key={action.id}
              type="button"
              onClick={() => void executeAction(action)}
              disabled={action.disabled}
            >
              <Icon size={16} />
              <span>{action.label}</span>
              <small>{action.meta}</small>
            </button>
          );
        })}
        {visibleActions.length === 0 && <EmptyState text={zhCN.commandPalette.empty} />}
      </div>
    </div>
  );
}

function StatusBar({ status }: { status: IDEData['status'] }) {
  const connection = status.api === 'connected'
    ? `${zhCN.shell.statusConnected} ${status.latency}`
    : status.api === 'degraded'
      ? `${zhCN.shell.statusDegraded} ${status.latency}`
      : zhCN.shell.statusOffline;
  return (
    <footer className="status-bar">
      <span>{zhCN.shell.statusWorkspace}: {status.workspace}</span>
      <span>{zhCN.shell.statusWorkflow}: {status.workflow}</span>
      <span>{zhCN.shell.statusToolchain}: {status.toolchain}</span>
      <span>{zhCN.shell.statusProvider}: {status.provider}</span>
      <span>{zhCN.shell.statusBackend}: {status.backend}</span>
      <span className={status.api === 'offline' ? 'offline' : 'connected'}>{connection}</span>
    </footer>
  );
}
