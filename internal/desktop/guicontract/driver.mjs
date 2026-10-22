import { existsSync, readFileSync } from 'node:fs';
import { join } from 'node:path';
import { fileURLToPath } from 'node:url';
import ts from 'typescript';

function findNamedDeclaration(sourceFile, name) {
  let match;
  function visit(node) {
    if (match) {
      return;
    }
    if (
      (ts.isFunctionDeclaration(node) || ts.isInterfaceDeclaration(node) || ts.isVariableDeclaration(node))
      && node.name
      && ts.isIdentifier(node.name)
      && node.name.text === name
    ) {
      match = node;
      return;
    }
    ts.forEachChild(node, visit);
  }
  visit(sourceFile);
  return match;
}

function collectNodes(rootNode, predicate) {
  const matches = [];
  function visit(node) {
    if (predicate(node)) {
      matches.push(node);
    }
    ts.forEachChild(node, visit);
  }
  visit(rootNode);
  return matches;
}

function namedDeclarationSource(sourceFile, name) {
  return findNamedDeclaration(sourceFile, name)?.getText(sourceFile) || '';
}

function isAIActionIDComparison(expression, actionId) {
  if (!ts.isBinaryExpression(expression) || expression.operatorToken.kind !== ts.SyntaxKind.EqualsEqualsEqualsToken) {
    return false;
  }
  const pairs = [
    [expression.left, expression.right],
    [expression.right, expression.left],
  ];
  return pairs.some(([candidate, value]) => (
    ts.isPropertyAccessExpression(candidate)
    && ts.isIdentifier(candidate.expression)
    && candidate.expression.text === 'action'
    && candidate.name.text === 'id'
    && ts.isStringLiteral(value)
    && value.text === actionId
  ));
}

function findAIActionBranch(sourceFile, functionName, actionId) {
  const declaration = findNamedDeclaration(sourceFile, functionName);
  if (!declaration) {
    return '';
  }
  const branch = collectNodes(
    declaration,
    node => ts.isIfStatement(node) && isAIActionIDComparison(node.expression, actionId),
  )[0];
  return branch?.getText(sourceFile) || '';
}

function jsxAttributeText(openingElement, attributeName, sourceFile) {
  const attribute = openingElement.attributes.properties.find(item => (
    ts.isJsxAttribute(item) && item.name.getText(sourceFile) === attributeName
  ));
  return attribute?.getText(sourceFile) || '';
}

function jsxAttributeExpression(element, attributeName, sourceFile) {
  const attribute = element.attributes.properties.find(item => (
    ts.isJsxAttribute(item) && item.name.getText(sourceFile) === attributeName
  ));
  return attribute?.initializer && ts.isJsxExpression(attribute.initializer)
    ? attribute.initializer.expression
    : undefined;
}

function isPropertyAccessPath(node, path) {
  if (!node) {
    return false;
  }
  const parts = [];
  let current = node;
  while (ts.isPropertyAccessExpression(current)) {
    parts.unshift(current.name.text);
    current = current.expression;
  }
  if (!ts.isIdentifier(current)) {
    return false;
  }
  parts.unshift(current.text);
  return parts.length === path.length && parts.every((part, index) => part === path[index]);
}

function isNodesLengthZero(expression) {
  if (!ts.isBinaryExpression(expression) || expression.operatorToken.kind !== ts.SyntaxKind.EqualsEqualsEqualsToken) {
    return false;
  }
  return [
    [expression.left, expression.right],
    [expression.right, expression.left],
  ].some(([lengthExpression, zeroExpression]) => (
    isPropertyAccessPath(lengthExpression, ['nodes', 'length'])
    && ts.isNumericLiteral(zeroExpression)
    && zeroExpression.text === '0'
  ));
}

function isNegatedIdentifier(expression, identifier) {
  return ts.isPrefixUnaryExpression(expression)
    && expression.operator === ts.SyntaxKind.ExclamationToken
    && ts.isIdentifier(expression.operand)
    && expression.operand.text === identifier;
}

function emptyStateUsesMissionCopy(rootNode, key, sourceFile) {
  return collectNodes(
    rootNode,
    node => ts.isJsxSelfClosingElement(node)
      && node.tagName.getText(sourceFile) === 'EmptyState'
      && isPropertyAccessPath(jsxAttributeExpression(node, 'text', sourceFile), ['zhCN', 'mission', key]),
  );
}

const root = fileURLToPath(new URL('..', import.meta.url));

const requiredFiles = [
  'src/assets/ouroboros-empty-mark.webp',
  'src/assets/ouroboros-mark.webp',
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

const expectedIDEModeDeclaration = "export type IDEMode = 'mission' | 'ai' | 'blackboard' | 'agents' | 'harness' | 'graph' | 'replay' | 'report' | 'runtime' | 'settings'";

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

const indexPath = join(root, 'index.html');
if (!existsSync(indexPath) || !readFileSync(indexPath, 'utf8').includes('<html lang="zh-CN">')) {
  console.error('renderer document language must match the fully Chinese desktop UI');
  failed = true;
}

const brandStylesPath = join(root, 'src/styles/ouroboros-ide.css');
if (existsSync(brandStylesPath)) {
  const brandStyles = readFileSync(brandStylesPath, 'utf8');
  for (const snippet of [
    'Red-black Ouroboros brand layer',
    'Final React GUI brand and semantic-color contract',
    '--brand-red: #ef3f49',
    'img.brand-mark',
    'img.ai-empty-brand',
    'object-fit: contain',
    '.activity-button.active',
    '.tree-icon.directory',
    '.ai-file-change-item header span[data-status="running"]',
    '.ai-message-avatar.assistant',
    '.agent-task-result-details',
    '.graph-node.selected',
    '@media (max-height: 720px)',
  ]) {
    if (!brandStyles.includes(snippet)) {
      console.error(`Ouroboros GUI must use the red-black snake brand system: ${snippet}`);
      failed = true;
    }
  }
  for (const obsoleteLogo of [
    'background: conic-gradient(from 20deg, #68bdff',
    'background: conic-gradient(from 20deg, #63727c',
  ]) {
    if (brandStyles.includes(obsoleteLogo)) {
      console.error(`Ouroboros GUI must not render the obsolete abstract ring logo: ${obsoleteLogo}`);
      failed = true;
    }
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
if (existsSync(shellPath)) {
  const shell = readFileSync(shellPath, 'utf8');
  const shellSourceFile = ts.createSourceFile('OuroborosIDE.tsx', shell, ts.ScriptTarget.Latest, true, ts.ScriptKind.TSX);
  for (const snippet of [
    "import ouroborosEmptyMark from '../assets/ouroboros-empty-mark.webp'",
    "import ouroborosMark from '../assets/ouroboros-mark.webp'",
    '<img className="brand-mark" src={ouroborosMark}',
    '<img className="welcome-brand-mark" src={ouroborosEmptyMark}',
    '<img className="ai-empty-brand" src={ouroborosEmptyMark}',
    '<img className="report-logo" src={ouroborosMark}',
  ]) {
    if (!shell.includes(snippet)) {
      console.error(`Ouroboros GUI must render the real snake image in every brand surface: ${snippet}`);
      failed = true;
    }
  }
  const requiredImports = ['loadIDEData', 'loadDesktopStatus', 'restartDesktopBackend', 'zhCN'];
  for (const imported of requiredImports) {
    if (!shell.includes(imported)) {
      console.error(`OuroborosIDE.tsx must use ${imported}`);
      failed = true;
    }
  }
  const forbiddenUiText = [
    'Demo',
    'Mock',
    'Preview',
    'Web UI',
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
  for (const [functionName, onClickSnippet, labelSnippet] of [
    ['TitleBar', 'onOpenPalette', 'zhCN.shell.commandPalette'],
    ['TitleBar', 'onOpenWorkspace', 'zhCN.shell.openWorkspace'],
    ['TitleBar', 'onOpenSettings', 'zhCN.modes.settings'],
    ['TitleBar', 'onMinimizeWindow', 'zhCN.windowControls.minimize'],
    ['TitleBar', 'onToggleMaximizeWindow', 'zhCN.windowControls.maximize'],
    ['TitleBar', 'onCloseWindow', 'zhCN.windowControls.close'],
    ['ActivityBar', 'onModeChange(item.id)', 'zhCN.modes[item.id]'],
    ['WorkspaceExplorer', 'onRefresh', 'zhCN.ai.refresh'],
    ['WorkspaceExplorer', 'onRunSearch', 'zhCN.shell.search'],
    ['AIWorkbench', 'onRefreshConversation', 'zhCN.ai.refresh'],
    ['AIWorkbench', 'setCapabilitiesOpen(false)', 'zhCN.ai.closeCapabilityDrawer'],
    ['RailPanelHeader', 'onOpen', 'zhCN.navigation.open'],
  ]) {
    const declaration = findNamedDeclaration(shellSourceFile, functionName);
    const buttons = declaration
      ? collectNodes(
        declaration,
        node => ts.isJsxOpeningElement(node)
          && node.tagName.getText(shellSourceFile) === 'button'
          && jsxAttributeText(node, 'onClick', shellSourceFile).includes(onClickSnippet),
      )
      : [];
    const button = buttons.find(node => jsxAttributeText(node, 'title', shellSourceFile).includes(labelSnippet));
    if (!button) {
      console.error(`${functionName} icon control ${onClickSnippet} must expose a Chinese title tooltip using ${labelSnippet}`);
      failed = true;
      continue;
    }
    if (!jsxAttributeText(button, 'aria-label', shellSourceFile).includes(labelSnippet)) {
      console.error(`${functionName} icon control ${onClickSnippet} must pair its title tooltip with aria-label using ${labelSnippet}`);
      failed = true;
    }
  }
  const workspaceExplorerDeclaration = findNamedDeclaration(shellSourceFile, 'WorkspaceExplorer');
  const workspaceFileButton = workspaceExplorerDeclaration
    ? collectNodes(
      workspaceExplorerDeclaration,
      node => ts.isJsxOpeningElement(node)
        && node.tagName.getText(shellSourceFile) === 'button'
        && jsxAttributeText(node, 'onClick', shellSourceFile).includes('onOpenFile(item)'),
    )[0]
    : undefined;
  for (const [attributeName, values] of [
    ['title', ['item.label', 'metadataLabel']],
    ['aria-label', ['item.label', 'metadataLabel']],
  ]) {
    const attribute = workspaceFileButton ? jsxAttributeText(workspaceFileButton, attributeName, shellSourceFile) : '';
    if (values.some(value => !attribute.includes(value))) {
      console.error(`WorkspaceExplorer file rows must expose real path and metadata through ${attributeName}`);
      failed = true;
    }
  }
  const graphCanvasDeclaration = findNamedDeclaration(shellSourceFile, 'GraphCanvas');
  const rightRailDeclaration = findNamedDeclaration(shellSourceFile, 'RightRail');
  const nodeSummaryDeclaration = findNamedDeclaration(shellSourceFile, 'NodeSummary');
  const graphNoDataStates = graphCanvasDeclaration
    ? emptyStateUsesMissionCopy(graphCanvasDeclaration, 'noGraph', shellSourceFile)
    : [];
  const graphNoDataState = graphNoDataStates.find(node => (
    ts.isBinaryExpression(node.parent)
    && node.parent.operatorToken.kind === ts.SyntaxKind.AmpersandAmpersandToken
    && node.parent.right === node
    && isNodesLengthZero(node.parent.left)
  ));
  if (!graphNoDataState) {
    console.error('GraphCanvas must use mission.noGraph only for an empty real attack-graph node list');
    failed = true;
  }
  const inspectorPanel = rightRailDeclaration
    ? collectNodes(
      rightRailDeclaration,
      node => ts.isJsxSelfClosingElement(node)
        && node.tagName.getText(shellSourceFile) === 'PanelTitle'
        && isPropertyAccessPath(jsxAttributeExpression(node, 'title', shellSourceFile), ['zhCN', 'mission', 'inspector']),
    )[0]
    : undefined;
  const inspectorMeta = inspectorPanel ? jsxAttributeExpression(inspectorPanel, 'meta', shellSourceFile) : undefined;
  if (
    !inspectorMeta
    || !ts.isConditionalExpression(inspectorMeta)
    || !ts.isIdentifier(inspectorMeta.condition)
    || inspectorMeta.condition.text !== 'inspectedNode'
    || !isPropertyAccessPath(inspectorMeta.whenFalse, ['zhCN', 'mission', 'noSelectedGraphNode'])
  ) {
    console.error('RightRail inspector must use mission.noSelectedGraphNode when no graph node is inspected');
    failed = true;
  }
  const rightRailNoGraphReferences = rightRailDeclaration
    ? collectNodes(rightRailDeclaration, node => isPropertyAccessPath(node, ['zhCN', 'mission', 'noGraph']))
    : [];
  if (rightRailNoGraphReferences.length > 0) {
    console.error('RightRail inspector must not describe an absent selection as missing attack-graph data');
    failed = true;
  }
  const noNodeBranch = nodeSummaryDeclaration
    ? collectNodes(
      nodeSummaryDeclaration,
      node => ts.isIfStatement(node) && isNegatedIdentifier(node.expression, 'node'),
    )[0]
    : undefined;
  if (!noNodeBranch || emptyStateUsesMissionCopy(noNodeBranch.thenStatement, 'noSelectedGraphNode', shellSourceFile).length === 0) {
    console.error('NodeSummary must use mission.noSelectedGraphNode for its no-selection state');
    failed = true;
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
  for (const snippet of ["{ id: 'agents', icon:", "mode === 'agents'", 'AgentWorkbench', 'agentState={ideData.agentState}', "event.type.startsWith('ai:')", 'zhCN.modes.agents', 'zhCN.agents.title', 'agent.tasks.read', 'agent.tasks.create', 'buildAIAgentTaskInput', 'isAgentAIAction(action)', 'AIActionAgentTask', 'AIActionAgentTaskList']) {
    if (!shell.includes(snippet)) {
      console.error(`OuroborosIDE.tsx must expose Agent Orchestrator v1 workspace and AI action UI: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['ToolLaunchPanel', 'toolActions', 'toolActionDrafts', 'draft.arguments', 'zhCN.toolLaunch.title', 'zhCN.toolLaunch.run']) {
    if (!shell.includes(snippet)) {
      console.error(`OuroborosIDE.tsx must expose manifest-backed tool launch UI: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['isToolResultAIAction(action)', "action.id === 'tool.parse_execution_result'", 'function buildAIToolResultInput', 'zhCN.aiActions.toolResultParseRequired', 'zhCN.aiActions.toolResultExecutionInput', 'zhCN.aiActions.toolResultTitleInput', 'zhCN.aiActions.toolResultSourceInput', 'zhCN.aiActions.toolResultGraphInput', 'zhCN.aiActions.toolResultReplayInput', 'AIActionToolResultParse', "result.result.kind === 'tool_result_parse'"]) {
    if (!shell.includes(snippet)) {
      console.error(`OuroborosIDE.tsx must expose parse execution result AI action input and result UI: ${snippet}`);
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
  for (const snippet of ['<FileEditor', 'file={openFile}', 'saveOpenFile', 'saveWorkspaceFile(openFile.path, fileDraft)', 'setFileStatus(zhCN.editor.saving)', 'setFileStatus(zhCN.editor.saveFailed)', 'onSaveFile={() => void saveOpenFile()}']) {
    if (!shell.includes(snippet)) {
      console.error(`OuroborosIDE.tsx must expose real workspace file save workflow: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['renameFilePath', 'renameOpenFile', 'renameWorkspaceFile(openFile.path, targetPath)', 'setRenameFilePath(file.path)', 'setFileStatus(zhCN.editor.renaming)', 'setFileStatus(zhCN.editor.renameFailed)', 'onRenameFile={() => void renameOpenFile()}', 'zhCN.editor.renameFile', 'zhCN.editor.renamePath']) {
    if (!shell.includes(snippet)) {
      console.error(`OuroborosIDE.tsx must expose real workspace file rename workflow: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['deleteOpenFile', 'deleteWorkspaceFile(openFile.path)', 'setOpenFile(null)', 'setFileDraft(\'\')', 'setRenameFilePath(\'\')', 'setFileStatus(zhCN.editor.deleting)', 'setFileStatus(zhCN.editor.deleteFailed)', 'onDeleteFile={() => void deleteOpenFile()}', 'zhCN.editor.deleteFile']) {
    if (!shell.includes(snippet)) {
      console.error(`OuroborosIDE.tsx must expose real workspace file delete workflow: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['newFilePath', 'newFileContent', 'createWorkspaceFile', 'setNewFilePath', 'setNewFileContent', 'saveWorkspaceFile(newFilePath', 'setOpenFile(file)', 'setFileDraft(file.content)', 'await refresh()', 'onCreateFile={() => void createWorkspaceFile()}', 'zhCN.editor.createFile', 'zhCN.editor.createPath', 'zhCN.editor.createContent']) {
    if (!shell.includes(snippet)) {
      console.error(`OuroborosIDE.tsx must expose real workspace file creation workflow: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['searchWorkspace(searchQuery)', 'setSearchResults(results)', 'openSearchResult(result: WorkspaceSearchResult)', 'onRunSearch={() => void runSearch()}', 'onOpenSearchResult={result => void openSearchResult(result)}', 'zhCN.search.results']) {
    if (!shell.includes(snippet)) {
      console.error(`OuroborosIDE.tsx must expose real workspace search workflow: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['onSaveProvider', 'onDeleteProvider', 'onTestProvider', 'saveProviderConfig', 'deleteProviderConfig', 'testProviderConfig', 'zhCN.settings.actions.saveProvider', 'data-provider-delete-action="true"', 'data-provider-test-action="true"', 'data-provider-test-result="true"']) {
    if (!shell.includes(snippet)) {
      console.error(`OuroborosIDE.tsx must expose editable provider settings: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['onSaveToolchain', 'saveToolchainPack', 'createToolchainDraft', 'zhCN.settings.actions.createToolchain', 'zhCN.settings.rows.toolchainPackId', 'zhCN.settings.placeholders.toolchainPackId']) {
    if (!shell.includes(snippet)) {
      console.error(`OuroborosIDE.tsx must expose editable and creatable toolchain settings: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['loadToolchainPacks', 'toolchainPacks', 'setToolchainPacks', 'toolchainManifestYaml', 'syncToolchainManifestYaml', 'applyToolchainManifestYaml', 'zhCN.settings.rows.toolchainManifestYaml', 'zhCN.settings.actions.reloadToolchain']) {
    if (!shell.includes(snippet)) {
      console.error(`OuroborosIDE.tsx must expose real /api/toolchain manifest management loop: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['loadToolchainActions', 'settingsToolActions', 'ToolchainActionCatalog', 'data-settings-toolchain-actions="true"', 'data-settings-toolchain-action-id', 'data-settings-toolchain-execution-id', 'zhCN.settings.toolchainActions', 'zhCN.settings.toolchainOpenExecution']) {
    if (!shell.includes(snippet)) {
      console.error(`OuroborosIDE.tsx must expose Settings toolchain manifest action run loop: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['onSaveWorkspaceSettings', 'saveWorkspaceSettings', 'zhCN.settings.actions.saveWorkspace']) {
    if (!shell.includes(snippet)) {
      console.error(`OuroborosIDE.tsx must expose editable workspace settings: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['fetchPromptRegistry', 'savePromptRegistry', 'promptRegistry', 'settingsActiveSection === \'prompts\'', 'updatePromptContent', 'saveCurrentPrompts', 'zhCN.settings.sections.prompts', 'zhCN.settings.actions.savePrompts']) {
    if (!shell.includes(snippet)) {
      console.error(`OuroborosIDE.tsx must expose real Prompt Registry editing: ${snippet}`);
      failed = true;
    }
  }
  const settingsWorkbenchBlock = shell.match(/function SettingsWorkbench\([\s\S]*?function SettingsSectionContent/)?.[0] || '';
  if (!settingsWorkbenchBlock) {
    console.error('OuroborosIDE.tsx must define SettingsWorkbench before SettingsSectionContent');
    failed = true;
  }
  for (const snippet of [
    'selectedPromptVersions',
    'promptEngineeringDiffResult',
    'promptEngineeringHarnessResult',
    'copyPromptVersionContent',
    'runPromptEngineeringDiff',
    'runPromptEngineeringHarness',
    'prompt.registry.diff',
    'prompt.harness.run',
    'zhCN.settings.actions.copyPrompt',
    'zhCN.settings.actions.comparePromptVersions',
    'zhCN.settings.actions.runPromptHarness',
    'data-prompt-engineering-action="copy"',
    'data-prompt-engineering-action="compare"',
    'data-prompt-engineering-action="run-harness"',
    'AIActionPromptDiffResult',
    'AIActionPromptHarnessRunResult',
    'PromptVariablesEditor',
    'data-prompt-template-variables="true"',
    'updatePromptVariable',
    'addPromptVariable',
	    'removePromptVariable',
	    'updatePromptVersionBinding',
	    'data-prompt-provider-binding',
	    'zhCN.settings.rows.promptProviderName',
	    'zhCN.settings.rows.promptProviderModel',
	    'templateVariables: promptHarnessTemplateVariables',
    'renamePromptHarnessTemplateVariable',
    'deletePromptHarnessTemplateVariable',
    'promptVariableNameAt',
    'templateVariables: promptHarnessTemplateVariables(version, harnessDraft)',
  ]) {
    if (!settingsWorkbenchBlock.includes(snippet)) {
      console.error(`SettingsWorkbench must expose real Prompt Engineering version selection, copy, diff, and harness UI: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['promptKindLabel(prompt.kind)', 'function promptKindLabel(kind: string)', 'zhCN.settings.promptKinds.system', 'zhCN.settings.promptKinds.unknown']) {
    if (!shell.includes(snippet)) {
      console.error(`OuroborosIDE.tsx must render prompt kind as Chinese UI copy: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of [
    'promptKindGroups(promptRegistry?.prompts ?? [])',
    'data-prompt-kind={prompt.kind}',
    'data-prompt-kind-category={promptKindCategory(prompt.kind)}',
    'promptKindCategory(prompt.kind)',
    'function promptKindGroups(prompts: PromptRegistry[\'prompts\'])',
    'function isRolePromptKind(kind: string): boolean',
    'zhCN.settings.promptKindGroups.role',
    'zhCN.settings.promptKinds.prompt_engineer',
    'zhCN.settings.promptKinds.harness',
    'zhCN.settings.promptKinds.tool',
  ]) {
    if (!settingsWorkbenchBlock.includes(snippet) && !shell.includes(snippet)) {
      console.error(`SettingsWorkbench must expose role prompt grouping and real prompt.kind categories, not system-only prompt rendering: ${snippet}`);
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
  for (const snippet of ['data-eventlog-inspector="true"', 'data-eventlog-payload="true"', 'zhCN.replay.eventlogInspector', 'formatEventPayload(event.payloadJson)', 'zhCN.details.rawEventId']) {
    if (!shell.includes(snippet)) {
      console.error(`OuroborosIDE.tsx must render raw eventlog inspector from backend replay DTOs: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['loadEventlogEvents', 'EventlogBrowser', 'eventlogEvents={eventlogEvents.events}', 'eventlogRuns={eventlogEvents.runs}', 'eventlogFilter', 'refreshEventlogEvents', 'data-eventlog-browser="true"', 'data-eventlog-run-filter="true"', 'data-eventlog-lane-filter="true"', 'data-eventlog-raw-record="true"', 'event.rawEventId', 'event.runId', 'event.type', 'event.lane', 'formatEventPayload(event.payloadJson)', 'onInspectExecution(event.executionId as string)', 'onOpenExecutionFile(event.filePath as string)']) {
    if (!shell.includes(snippet)) {
      console.error(`OuroborosIDE.tsx must browse raw SQLite eventlog records from /api/events with filters and navigation: ${snippet}`);
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
  for (const snippet of [
    "const [selectedExecutionId, setSelectedExecutionId] = useState('')",
    'setSelectedExecutionId(executionId)',
    'setSelectedExecutionId(id)',
    'selectedExecutionId={selectedExecutionId}',
    'selectedExecutionId: string',
  ]) {
    if (!shell.includes(snippet)) {
      console.error(`OuroborosIDE.tsx must track and pass the explicitly selected execution id for execution detail references: ${snippet}`);
      failed = true;
    }
  }
  const toolExecutionBlock = shell.match(/function ToolExecution\([\s\S]*?const referenceTargets:/)?.[0] || '';
  const activeStepBlock = toolExecutionBlock.match(/const activeStep =[\s\S]*?;/)?.[0] || '';
  if (!activeStepBlock.includes('selectedExecutionId')) {
    console.error('ToolExecution activeStep must derive from selectedExecutionId so cross references follow the inspected execution');
    failed = true;
  }
  if (activeStepBlock.includes('log?.id') && activeStepBlock.indexOf('log?.id') < activeStepBlock.indexOf('selectedExecutionId')) {
    console.error('ToolExecution activeStep must not prioritize log?.id over selectedExecutionId');
    failed = true;
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
  for (const snippet of [
    'const [selectedReportPath, setSelectedReportPath]',
    'const [reportMarkdownDraft, setReportMarkdownDraft]',
    'data-report-markdown-editor="true"',
    'value={reportMarkdownDraft}',
    'onChange={event => setReportMarkdownDraft(event.target.value)}',
    'zhCN.report.loadReportMarkdown',
    'zhCN.report.saveReportMarkdown',
    'zhCN.report.markdownSource',
    'onSaveReportMarkdown={() => void saveReportMarkdown()}',
  ]) {
    if (!shell.includes(snippet)) {
      console.error(`ReportWorkbench must directly load, edit, and save real report Markdown files: ${snippet}`);
      failed = true;
    }
  }
  const reportWorkbenchBlock = shell.match(/function ReportWorkbench\([\s\S]*?function ReportMarkdownSaveButton/)?.[0] || '';
  if (!reportWorkbenchBlock) {
    console.error('OuroborosIDE.tsx must define ReportWorkbench before ReportMarkdownSaveButton');
    failed = true;
  }
  for (const snippet of [
    'loadReportFile(selectedReportPath)',
    'saveReportFile(selectedReportPath, reportMarkdownDraft)',
  ]) {
    if (!reportWorkbenchBlock.includes(snippet)) {
      console.error(`ReportWorkbench must use first-class report file APIs: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of [
    'data-report-preview-panel',
    'data-report-preview-content',
    'data-report-preview-empty',
    'data-report-preview-heading',
    'data-report-preview-list',
    'data-report-preview-code',
    'data-report-preview-link',
    'zhCN.report.preview',
    'zhCN.report.previewEmpty',
    'zhCN.report.editSource',
  ]) {
    if (!reportWorkbenchBlock.includes(snippet)) {
      console.error(`ReportWorkbench must render a real report Markdown preview with stable selectors and Chinese copy: ${snippet}`);
      failed = true;
    }
  }
  if (!reportWorkbenchBlock.includes('data-report-preview-toggle') && !reportWorkbenchBlock.includes('data-report-source-editor')) {
    console.error('ReportWorkbench must expose a stable selector for toggling preview/source editing');
    failed = true;
  }
  if (![
    'markdown={reportMarkdownDraft}',
    'content={reportMarkdownDraft}',
    'source={reportMarkdownDraft}',
    'renderReportMarkdown(reportMarkdownDraft',
    'parseReportMarkdown(reportMarkdownDraft',
  ].some(snippet => reportWorkbenchBlock.includes(snippet))) {
    console.error('ReportWorkbench preview must render from the real loaded/saved reportMarkdownDraft, not mock/demo content');
    failed = true;
  }
  const reportWorkbenchInvocationBlock = shell.match(/<ReportWorkbench\b[\s\S]*?\/>/)?.[0] || '';
  if (!reportWorkbenchInvocationBlock.includes('onRefresh={refresh}')) {
    console.error('ReportWorkbench must receive the real IDE refresh callback after report Markdown saves');
    failed = true;
  }
  for (const snippet of [
    'loadWorkspaceFile(selectedReportPath)',
    'saveWorkspaceFile(selectedReportPath, reportMarkdownDraft)',
  ]) {
    if (reportWorkbenchBlock.includes(snippet)) {
      console.error(`ReportWorkbench must not use generic workspace file APIs for report Markdown editing: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of [
    'Report Preview',
    'Preview empty',
    'Empty preview',
    'Edit source',
    'Source editor',
    'Mock report preview',
    'Demo report preview',
    'Sample report preview',
    'mockReportPreview',
    'demoReportPreview',
    'sampleReportPreview',
    'reportPreviewMock',
    'reportPreviewDemo',
    'reportPreviewSample',
    '示例报告预览',
    '样例报告预览',
    '模拟报告预览',
    '报告预览示例',
    '报告预览样例',
  ]) {
    if (reportWorkbenchBlock.includes(snippet)) {
      console.error(`ReportWorkbench must not use mock/demo report preview copy or data: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of [
    "kind === 'frontierDependency'",
    "frontierDependencyReference('frontierDependency'",
    "source: 'frontier.dependencies'",
    'setMode(frontierDependencyReferenceMode(reference))',
    'function frontierDependencyReferenceMode(reference: AIContextReference): IDEMode',
  ]) {
    if (!shell.includes(snippet)) {
      console.error(`OuroborosIDE.tsx must recognize and navigate AI context references with kind=frontierDependency: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of [
    '<AICollaborationTopologyPanel',
    'function AICollaborationTopologyPanel',
    'function buildAICollaborationTopology',
    'data-ai-collaboration-topology="true"',
    'data-ai-topology-node="true"',
    'data-ai-topology-node-kind',
    'data-ai-topology-node-id',
    'data-ai-topology-edge="true"',
    'data-ai-topology-edge-kind',
    'data-ai-topology-edge-source',
    'data-ai-topology-edge-target',
    'data-ai-topology-action="refresh"',
    'data-ai-topology-action="select-strategy"',
    'data-ai-topology-action="execute-next"',
    'data-ai-topology-action="execute-node"',
    'data-ai-topology-action="settle-harness"',
    "onRunFrontierQueueAction('frontier.execute_next', node.frontierItem)",
    "onRunFollowupAction('harness.cases.from_ai_conversation', harnessInput)",
    "source: 'desktop.ai.collaboration_topology'",
    'zhCN.ai.collaborationTopology',
    'zhCN.ai.collaborationTopologyNodes',
    'zhCN.ai.collaborationTopologyEdges',
    'zhCN.ai.collaborationTopologyBlackboardNode',
    'zhCN.ai.collaborationTopologyAgentNode',
    'zhCN.ai.collaborationTopologyFrontierNode',
    'zhCN.ai.collaborationTopologyBlackboardNodes',
    'zhCN.ai.collaborationTopologyAgentNodes',
    'zhCN.ai.collaborationTopologyFrontierNodes',
    'zhCN.ai.collaborationTopologyDependencyEdge',
    'zhCN.ai.collaborationTopologyDependentEdge',
    'zhCN.ai.collaborationTopologyAgentBlackboardEdge',
    'zhCN.ai.collaborationTopologyFrontierBlackboardEdge',
    'zhCN.ai.collaborationTopologyFrontierAgentEdge',
    'zhCN.ai.collaborationTopologyBlockedByEdge',
    'zhCN.ai.collaborationTopologyEmpty',
    'zhCN.ai.collaborationTopologyEdgesEmpty',
    'zhCN.ai.collaborationTopologyNodeList',
    'zhCN.ai.collaborationTopologyEdgeList',
    'zhCN.ai.collaborationTopologySourceNode',
    'zhCN.ai.collaborationTopologyTargetNode',
    'zhCN.ai.collaborationTopologyEdgeArrow',
    'zhCN.ai.collaborationTopologyHarnessSource',
    'zhCN.ai.collaborationTopologyHarnessReady',
    'zhCN.ai.collaborationTopologyHarnessMissing',
    'zhCN.ai.collaborationTopologySelectStrategy',
    'zhCN.ai.collaborationTopologyExecuteNext',
    'zhCN.ai.collaborationTopologySettleHarness',
    'zhCN.ai.collaborationTopologySettlingHarness',
  ]) {
    if (!shell.includes(snippet)) {
      console.error(`OuroborosIDE.tsx must expose the real AI Workbench collaboration topology panel contract: ${snippet}`);
      failed = true;
    }
  }
  const collaborationTopologyPanel = findNamedDeclaration(shellSourceFile, 'AICollaborationTopologyPanel');
  if (!collaborationTopologyPanel || !ts.isFunctionDeclaration(collaborationTopologyPanel)) {
    console.error('OuroborosIDE.tsx must declare AICollaborationTopologyPanel');
    failed = true;
  } else {
    const variableDeclarations = collectNodes(collaborationTopologyPanel, ts.isVariableDeclaration);
    const topologyGoalDeclarations = variableDeclarations.filter(node => ts.isIdentifier(node.name) && node.name.text === 'topologyGoal');
    const harnessInputDeclarations = variableDeclarations.filter(node => ts.isIdentifier(node.name) && node.name.text === 'harnessInput');
    const flattenLogicalOr = expression => (
      ts.isBinaryExpression(expression) && expression.operatorToken.kind === ts.SyntaxKind.BarBarToken
        ? [...flattenLogicalOr(expression.left), ...flattenLogicalOr(expression.right)]
        : [expression]
    );
    const expectedGoalSources = [
      'frontierQueueQuery.goal',
      'frontierQueue?.summary?.goal',
      'topology.readyFrontierNode?.frontierItem?.hypothesis',
      'topology.readyFrontierNode?.frontierItem?.title',
      'undefined',
    ];
    const actualGoalSources = topologyGoalDeclarations.length === 1 && topologyGoalDeclarations[0].initializer
      ? flattenLogicalOr(topologyGoalDeclarations[0].initializer).map(node => node.getText(shellSourceFile))
      : [];
    if (JSON.stringify(actualGoalSources) !== JSON.stringify(expectedGoalSources)) {
      console.error(`AICollaborationTopologyPanel topologyGoal must use real DTO fields without readyFrontierNode?.label fallback: ${actualGoalSources.join(' | ') || '(missing)'}`);
      failed = true;
    }

    const harnessInputInitializer = harnessInputDeclarations.length === 1 ? harnessInputDeclarations[0].initializer : undefined;
    const harnessProperties = harnessInputInitializer && ts.isObjectLiteralExpression(harnessInputInitializer)
      ? harnessInputInitializer.properties.filter(ts.isPropertyAssignment)
      : [];
    const harnessGoal = harnessProperties.find(property => property.name.getText(shellSourceFile) === 'goal');
    const harnessSource = harnessProperties.find(property => property.name.getText(shellSourceFile) === 'source');
    if (harnessGoal?.initializer.getText(shellSourceFile) !== 'topologyGoal') {
      console.error('AICollaborationTopologyPanel must bind topologyGoal directly to harnessInput.goal');
      failed = true;
    }
    if (!harnessSource || !ts.isStringLiteral(harnessSource.initializer) || harnessSource.initializer.text !== 'desktop.ai.collaboration_topology') {
      console.error('AICollaborationTopologyPanel must keep the real topology source on harnessInput');
      failed = true;
    }
    const followupCalls = collectNodes(
      collaborationTopologyPanel,
      node => ts.isCallExpression(node)
        && ts.isIdentifier(node.expression)
        && node.expression.text === 'onRunFollowupAction',
    );
    if (!followupCalls.some(call => (
      call.arguments.length === 2
      && ts.isStringLiteral(call.arguments[0])
      && call.arguments[0].text === 'harness.cases.from_ai_conversation'
      && ts.isIdentifier(call.arguments[1])
      && call.arguments[1].text === 'harnessInput'
    ))) {
      console.error('AICollaborationTopologyPanel must pass the same harnessInput object to onRunFollowupAction');
      failed = true;
    }
  }
  for (const snippet of [
    "kind === 'provider' || kind === 'settings' || kind === 'prompt' || kind === 'promptVersion'",
    'settingsFocusForAIReference(reference',
    'setSettingsFocus(current =>',
    'focus={settingsFocus}',
    'resolvePromptFocusTarget(promptRegistry, focus)',
    'data-prompt-id={prompt.id}',
    "data-prompt-version-selected={selectedVersion?.id || ''}",
  ]) {
    if (!shell.includes(snippet)) {
      console.error(`OuroborosIDE.tsx must navigate promptVersion AI context references to the real prompt/settings workbench: ${snippet}`);
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
  const createToolchainDraftSource = namedDeclarationSource(shellSourceFile, 'createToolchainDraft');
  for (const snippet of [
    'toolchainPacks.length',
    "'local'",
    "'0.1.0'",
    "runner: 'command'",
    "parser: 'jsonl_parser'",
    "healthcheck: 'built_in'",
    'runTimeoutMs: 10000',
  ]) {
    if (createToolchainDraftSource.includes(snippet)) {
      console.error(`OuroborosIDE.tsx must not fabricate new toolchain business fields in the frontend: ${snippet}`);
      failed = true;
    }
  }
  if (!createToolchainDraftSource.includes('emptyToolchainPack()')) {
    console.error('OuroborosIDE.tsx must start new toolchain editing from an empty user-authored draft');
    failed = true;
  }
  for (const snippet of [
    'const [creatingToolchain, setCreatingToolchain] = useState(false)',
    'if (creatingToolchain) {',
    'setCreatingToolchain(true)',
    'creatingToolchain && !selectedToolchainID',
  ]) {
    if (!shell.includes(snippet)) {
      console.error(`OuroborosIDE.tsx must keep an empty toolchain draft editable without inventing a persisted pack ID: ${snippet}`);
      failed = true;
    }
  }
  const emptyToolchainPackSource = namedDeclarationSource(shellSourceFile, 'emptyToolchainPack');
  if (!emptyToolchainPackSource.includes('status: zhCN.settings.notConfigured')) {
    console.error('OuroborosIDE.tsx must render the empty toolchain status with Chinese unconfigured copy');
    failed = true;
  }
  const emptyIDEDataSource = namedDeclarationSource(shellSourceFile, 'emptyData');
  const emptyProviderDraftSource = namedDeclarationSource(shellSourceFile, 'emptyProviderDraft');
  for (const [label, source] of [
    ['empty IDE data', emptyIDEDataSource],
    ['empty provider draft', emptyProviderDraftSource],
  ]) {
    if (source.includes("status: 'unknown'")) {
      console.error(`OuroborosIDE.tsx must not expose raw English status in ${label}`);
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
  for (const snippet of ['data.messages', 'MessageBubble', 'message.author', 'message.label', 'message.body']) {
    if (shell.includes(snippet)) {
      console.error(`OuroborosIDE.tsx must not keep dormant mission message rows without backend DTO support: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['zhCN.mission.agentGraph', '<AgentGraphCanvas data={data}', 'data.agentState.tasks.slice', 'data.orchestratorState.runs.length']) {
    if (!shell.includes(snippet)) {
      console.error(`OuroborosIDE.tsx must render the right-rail agent graph from real agent/orchestrator DTOs: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['workspace-summary-flow', '<span>{data.workspaceItems.length} {zhCN.shell.workspace}</span>']) {
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
  if (!/<SettingsWorkbench\b[\s\S]*?\bdata=\{ideData\}/.test(shell)) {
    console.error('SettingsWorkbench must render real IDE data, not fixed defaults');
    failed = true;
  }
  for (const snippet of ['DesktopStatusBanner', 'desktopStatus', 'zhCN.desktop.backendCrashed', 'zhCN.desktop.restartBackend', 'desktopStatus={desktopStatus}', 'desktopStatus?.runtime', 'data-desktop-runtime-info', 'data-desktop-backend-sidecar']) {
    if (!shell.includes(snippet)) {
      console.error(`OuroborosIDE.tsx must expose real desktop status surface: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['desktopStatusLabel(desktopStatus)', 'buildDesktopDiagnosticItems(desktopStatus)', 'zhCN.desktop.backendHealthFailed', 'zhCN.desktop.backendUnknown', 'desktopRuntime?.lastStdout', 'desktopRuntime?.lastStderr']) {
    if (!shell.includes(snippet)) {
      console.error(`OuroborosIDE.tsx must render precise desktop backend failure diagnostics instead of generic crash/-1 copy: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ["{ id: 'runtime', icon:", "mode === 'runtime'", '<RuntimeWorkbench', 'function RuntimeWorkbench', 'desktopStatus={desktopStatus}', 'onRefresh={() => void refresh()}', 'onRestart={() => void restartBackend()}']) {
    if (!shell.includes(snippet)) {
      console.error(`OuroborosIDE.tsx must expose a first-class desktop runtime workbench mode backed by real status actions: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of [
    'data-runtime-workbench="true"',
    'data-runtime-summary="true"',
    'data-runtime-refresh="true"',
    'data-runtime-restart="true"',
    "'data-runtime-backend-status'",
    "'data-runtime-package-status'",
    "'data-runtime-diagnostics'",
    "'data-runtime-paths'",
    "'data-runtime-events'",
  ]) {
    if (!shell.includes(snippet)) {
      console.error(`RuntimeWorkbench must expose stable desktop runtime selectors: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['zhCN.modes[item.id]', 'zhCN.runtime.title', 'zhCN.runtime.refreshStatus', 'zhCN.runtime.restartBackend', 'zhCN.runtime.backendStatus', 'zhCN.runtime.packageStatus', 'zhCN.runtime.diagnostics', 'zhCN.runtime.backendProcess']) {
    if (!shell.includes(snippet)) {
      console.error(`RuntimeWorkbench must use centralized Chinese zhCN runtime copy: ${snippet}`);
      failed = true;
    }
  }
  const desktopDiagnostics = readFileSync(join(root, 'src/app/desktop-diagnostics.ts'), 'utf8');
  for (const snippet of ['zhCN.desktop.detailDiagnosticSummary', 'zhCN.desktop.detailAction', 'zhCN.desktop.detailDiagnosticLog', 'zhCN.desktop.detailHealthError', 'zhCN.desktop.detailRuntimeIssue', 'zhCN.desktop.diagnosisRendererOom', 'zhCN.desktop.diagnosisPreviousUncleanExit', 'zhCN.desktop.diagnosisCleanExit', 'zhCN.desktop.actionCleanExit', 'isCleanDesktopBackendExit(runtime?.lastExitCode, runtime?.lastExitSignal)', 'formatDesktopExit(runtime.lastExitCode, runtime.lastExitSignal)', 'desktopRuntimeIssueLooksProcessKilled(issue)', 'compactDiagnosticOutput(runtime.lastStderr)']) {
    if (!desktopDiagnostics.includes(snippet)) {
      console.error(`desktop-diagnostics.ts must render precise desktop backend and Electron process diagnostics: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['fetchAIConversation', 'sendAIChatMessage', "id: 'ai'", 'AIWorkbench', 'aiConversation', 'aiStatus', 'aiDraft', 'onSendAIMessage={() => void sendAIMessage()}']) {
    if (!shell.includes(snippet)) {
      console.error(`OuroborosIDE.tsx must expose a backend-backed AI workbench: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['AIToolMessageContent', 'parseLineOrientedAIMessage', 'data-ai-tool-summary="true"', 'zhCN.ai.toolResultDetails']) {
    if (!shell.includes(snippet)) {
      console.error(`OuroborosIDE.tsx must present real line-oriented tool output as a structured summary: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['ai-workbench-state', 'ai-composer-surface', 'ai-composer-footer', 'ai-message-references', 'ai-empty-subtitle', 'ai-empty-message', 'zhCN.brand.subtitle', 'structured.fields.slice(0, 4)']) {
    if (!shell.includes(snippet)) {
      console.error(`OuroborosIDE.tsx must preserve the design-aligned compact AI conversation hierarchy: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of [
    'selectedAIFileChangeKey',
    'setSelectedAIFileChangeKey',
    'selectedDiffChange',
    'AIDiffInspector',
    'onSelectDiff',
    'data-ai-diff-inspector="true"',
    'data-ai-diff-selected="true"',
    'data-ai-file-change-diff-action="select"',
    'zhCN.ai.diffInspector',
    'zhCN.ai.diffInspectorMeta',
    'zhCN.ai.diffInspectorEmpty',
    'zhCN.ai.fileChangeViewDiff',
  ]) {
    if (!shell.includes(snippet)) {
      console.error(`OuroborosIDE.tsx must expose selectable AI file diff inspector from real action traces: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ["id: 'blackboard'", 'BlackboardWorkbench', 'ideData.blackboard', "event.type.startsWith('blackboard:')", 'zhCN.blackboard.title', 'zhCN.blackboard.items', 'zhCN.blackboard.summary', 'blackboard.items.map', 'BlackboardItemCard']) {
    if (!shell.includes(snippet)) {
      console.error(`OuroborosIDE.tsx must expose backend-projected Blackboard v1 UI: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['blackboardStatusLabel(item.status)', 'blackboardKindLabel(item.kind)', 'blackboardPriorityLabel(item.priority)', 'zhCN.blackboard.openFile', 'zhCN.blackboard.inspectExecution', 'zhCN.blackboard.focusGraph', 'zhCN.blackboard.focusReplay', 'zhCN.blackboard.focusReport']) {
    if (!shell.includes(snippet)) {
      console.error(`OuroborosIDE.tsx must render Blackboard item status and cross-navigation in Chinese: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['fetchAIActions', 'runAIAction', 'AIActionPanel', 'aiActions', 'aiActionInputs', 'aiActionResult', 'refreshAIActions', 'runSelectedAIAction', 'zhCN.aiActions.title', 'zhCN.aiActions.run', 'onRunAction={action => void runSelectedAIAction(action)}']) {
    if (!shell.includes(snippet)) {
      console.error(`OuroborosIDE.tsx must expose backend-backed AI Actions UI: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['ai-action-search', 'visibleActions', 'data-ai-action-id={action.id}', 'zhCN.aiActions.searchPlaceholder', 'action.status ? localizedStatusLabel(action.status) : undefined']) {
    if (!shell.includes(snippet)) {
      console.error(`AIActionPanel must expose every real action through compact searchable disclosure without inventing status: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ["{ id: 'harness', icon:", "mode === 'harness'", 'HarnessWorkbench', 'harness={ideData.harness}', 'zhCN.modes[item.id]', 'zhCN.harness.title', 'onRefreshHarness={() => void refreshHarness()}', 'onCreateHarnessCase={(draft) => void createHarnessCaseDraft(draft)}', 'onRunHarnessCase={id => void runHarnessCaseById(id)}']) {
    if (!shell.includes(snippet)) {
      console.error(`OuroborosIDE.tsx must expose Harness Engineering v1 workspace UI: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['runHarnessCase', 'createHarnessCase', 'fetchHarnessState', 'HarnessCaseCard', 'HarnessResultCard', 'harnessStatusLabel(result.status)', 'harnessKindLabel(testCase.kind)', 'zhCN.harness.openResultFile', 'zhCN.harness.openLogFile', 'zhCN.harness.openArtifactDir', 'zhCN.harness.inspectExecution']) {
    if (!shell.includes(snippet)) {
      console.error(`OuroborosIDE.tsx must render Harness cases/results/counts and navigation from backend DTOs: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['HarnessInputSnapshotSummary', 'data-harness-input-snapshot="true"', 'data-harness-provider-snapshot="true"', 'data-harness-toolchain-snapshot="true"', 'zhCN.harness.inputSnapshots', 'zhCN.harness.providerSnapshot', 'zhCN.harness.toolchainSnapshot', 'testCase.providerSnapshot', 'testCase.toolchainSnapshot']) {
    if (!shell.includes(snippet)) {
      console.error(`OuroborosIDE.tsx must render Harness provider/toolchain input snapshots from backend DTOs: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['data-harness-result-id={result.id}', 'result.references.length > 0', '<AIReferenceActions references={result.references} onOpenReference={onOpenReference} compact />', "kind === 'replayEvent' || kind === 'replayRun'", "case 'promptVersion':", "case 'workspaceSnapshot':", "case 'executionLog':"]) {
    if (!shell.includes(snippet)) {
      console.error(`OuroborosIDE.tsx must render and navigate real Harness result references from backend DTOs: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['result.aiMessageIds.length > 0', 'result.toolCallIds.length > 0', 'result.fileDiffs.length > 0', 'result.eventlogReferences.length > 0', 'zhCN.harness.aiMessages', 'zhCN.harness.toolCalls', 'zhCN.harness.fileDiffs', 'zhCN.harness.eventlogReferences', 'data-harness-ai-message-id', 'data-harness-tool-call-id', 'data-harness-file-diff="true"', 'data-harness-file-diff-content="true"', 'data-harness-eventlog-reference="true"', 'diff.diff &&', 'reference.replayEventId || reference.rawEventId']) {
    if (!shell.includes(snippet)) {
      console.error(`OuroborosIDE.tsx must render Harness result trace fields from backend DTOs with Chinese labels: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['[zhCN.harness.exitCode, result.exitCode]', '[zhCN.harness.durationMs, result.durationMs]', '[zhCN.harness.logBytes, result.logBytes]', '[zhCN.harness.artifactCount, result.artifactCount]', '[zhCN.harness.artifactBytes, result.artifactBytes]']) {
    const occurrenceCount = shell.split(snippet).length - 1;
    if (occurrenceCount < 2) {
      console.error(`OuroborosIDE.tsx must render real Harness execution metrics in HarnessResultCard and AIActionHarnessResult: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ["action.kind === 'harness'", 'isHarnessAIAction', 'zhCN.aiActions.harnessKind', 'buildAIHarnessInput', 'harness.cases.read', 'harness.cases.create', 'harness.cases.from_failure', 'harness.cases.from_eventlog', 'harness.cases.from_workspace_snapshot', 'harness.cases.from_ai_conversation', 'harness.run', 'harnessTitleInput', 'harnessGoalInput', 'harnessCommandInput', 'harnessExpectedResultInput', 'harnessPromptVersionInput', 'harnessFailureExecutionInput', 'harnessFailureTranscriptInput', 'harnessEventlogRunInput', 'harnessEventlogRunPlaceholder', 'harnessSnapshotIdInput', 'harnessSnapshotPathInput', 'harnessSnapshotFilesInput', 'harnessAIMessageIdsInput', 'harnessToolCallIdsInput', 'harnessAIMessageIdsPlaceholder', 'harnessToolCallIdsPlaceholder', 'harnessFromAIConversationRequired', 'snapshotId', 'snapshotPath', 'paths', 'runId', 'replayRunId', 'aiMessageIds', 'toolCallIds', 'promptVersionId', 'harnessCaseId', 'input: harnessInput']) {
    if (!shell.includes(snippet)) {
      console.error(`OuroborosIDE.tsx must expose structured Harness AI action input fields: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ["action.kind === 'eventlog'", 'isEventlogAIAction', 'zhCN.aiActions.eventlogKind', 'buildAIEventlogInput', 'eventlog.query', 'eventlogRunInput', 'eventlogLaneInput', 'eventlogTypeInput', 'eventlogLimitInput', 'eventlogRunPlaceholder', 'eventlogLanePlaceholder', 'eventlogTypePlaceholder', 'eventlogLimitPlaceholder', 'input: eventlogInput']) {
    if (!shell.includes(snippet)) {
      console.error(`OuroborosIDE.tsx must expose structured eventlog.query AI action input fields: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['DEFAULT_HARNESS_ENVIRONMENT_SNAPSHOT_SOURCE', '桌面环境快照', 'harness.cases.from_environment_snapshot', 'harnessEnvironmentSnapshotSourcePlaceholder', 'harnessFromEnvironmentSnapshotRequired', 'harnessFromEnvironmentSnapshotResult']) {
    if (!shell.includes(snippet)) {
      console.error(`OuroborosIDE.tsx must expose environment snapshot Harness AI action support: ${snippet}`);
      failed = true;
    }
  }
  const harnessInputBuilderBlock = namedDeclarationSource(shellSourceFile, 'buildAIHarnessInput');
  const workspaceSnapshotBuilderBlock = findAIActionBranch(shellSourceFile, 'buildAIHarnessInput', 'harness.cases.from_workspace_snapshot');
  for (const snippet of ["const command = aiActionInputValue(aiActionInputs, action.id, 'command').trim();", 'command,']) {
    if (!workspaceSnapshotBuilderBlock.includes(snippet)) {
      console.error(`buildAIHarnessInput must require and send the real harness.cases.from_workspace_snapshot command: ${snippet}`);
      failed = true;
    }
  }
  if (!/if \(!command\)\s*\{\s*return null;\s*\}/.test(workspaceSnapshotBuilderBlock)) {
    console.error('buildAIHarnessInput must reject an empty harness.cases.from_workspace_snapshot command before sending the request');
    failed = true;
  }
  for (const snippet of ["action.id === 'harness.cases.from_environment_snapshot'", "aiActionInputValue(aiActionInputs, action.id, 'snapshotId')", "source: aiActionInputValue(aiActionInputs, action.id, 'source').trim() || DEFAULT_HARNESS_ENVIRONMENT_SNAPSHOT_SOURCE", 'agentTaskId', 'blackboardItemId', 'promptVersionId']) {
    if (!harnessInputBuilderBlock.includes(snippet)) {
      console.error(`buildAIHarnessInput must send structured harness.cases.from_environment_snapshot fields: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ["action.id === 'harness.cases.from_ai_conversation'", "aiActionInputValue(aiActionInputs, action.id, 'aiMessageIds')", "aiActionInputValue(aiActionInputs, action.id, 'toolCallIds')", 'aiMessageIds', 'toolCallIds']) {
    if (!harnessInputBuilderBlock.includes(snippet)) {
      console.error(`buildAIHarnessInput must send structured harness.cases.from_ai_conversation fields: ${snippet}`);
      failed = true;
    }
  }
  const harnessActionInputBlock = namedDeclarationSource(shellSourceFile, 'renderAIActionInput');
  const workspaceSnapshotInputBlock = findAIActionBranch(shellSourceFile, 'renderAIActionInput', 'harness.cases.from_workspace_snapshot');
  for (const snippet of ['zhCN.aiActions.harnessCommandInput', 'zhCN.aiActions.harnessSnapshotCommandPlaceholder', "aiActionInputValue(inputs, action.id, 'command')", "updateStructuredInput('command'"]) {
    if (!workspaceSnapshotInputBlock.includes(snippet)) {
      console.error(`renderAIActionInput must expose the real workspace-snapshot Harness command in Chinese: ${snippet}`);
      failed = true;
    }
  }
  const environmentSnapshotInputBlock = findAIActionBranch(shellSourceFile, 'renderAIActionInput', 'harness.cases.from_environment_snapshot');
  for (const snippet of ["action.id === 'harness.cases.from_environment_snapshot'", 'zhCN.aiActions.harnessEnvironmentSnapshotIdInput', 'zhCN.aiActions.harnessTitleInput', 'zhCN.aiActions.harnessGoalInput', 'zhCN.aiActions.harnessExpectedResultInput', 'zhCN.aiActions.harnessPromptVersionInput', 'zhCN.aiActions.harnessSourceInput', 'zhCN.aiActions.harnessAgentTaskInput', 'zhCN.aiActions.harnessBlackboardInput', "aiActionInputValue(inputs, action.id, 'snapshotId')", "aiActionInputValue(inputs, action.id, 'title')", "aiActionInputValue(inputs, action.id, 'goal')", "aiActionInputValue(inputs, action.id, 'expectedResult')", "aiActionInputValue(inputs, action.id, 'source')", "aiActionInputValue(inputs, action.id, 'agentTaskId')", "aiActionInputValue(inputs, action.id, 'blackboardItemId')", "aiActionInputValue(inputs, action.id, 'promptVersionId')"]) {
    if (!environmentSnapshotInputBlock.includes(snippet)) {
      console.error(`renderAIActionInput must expose Chinese harness.cases.from_environment_snapshot controls without content: ${snippet}`);
      failed = true;
    }
  }
  if (environmentSnapshotInputBlock.includes("'content'") || environmentSnapshotInputBlock.includes('harnessContent')) {
    console.error('renderAIActionInput must not require content for harness.cases.from_environment_snapshot');
    failed = true;
  }
  for (const snippet of ["action.id === 'harness.cases.from_ai_conversation'", 'zhCN.aiActions.harnessAIMessageIdsInput', 'zhCN.aiActions.harnessToolCallIdsInput', "aiActionInputValue(inputs, action.id, 'aiMessageIds')", "aiActionInputValue(inputs, action.id, 'toolCallIds')", "updateStructuredInput('aiMessageIds'", "updateStructuredInput('toolCallIds'"]) {
    if (!harnessActionInputBlock.includes(snippet)) {
      console.error(`renderAIActionInput must expose Chinese harness.cases.from_ai_conversation controls: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['result.result.harnessState', 'result.result.harnessCases', 'result.result.harnessCase', 'result.result.harnessResults', 'result.result.harnessResult', 'AIActionHarnessCase', 'AIActionHarnessResult', 'zhCN.aiActions.harnessReadResult', 'zhCN.aiActions.harnessCreateResult', 'zhCN.aiActions.harnessFromFailureResult', 'zhCN.aiActions.harnessFromEventlogResult', 'zhCN.aiActions.harnessFromWorkspaceSnapshotResult', 'zhCN.aiActions.harnessFromAIConversationResult', 'zhCN.aiActions.harnessRunResult', "case 'harness_case_from_failure'", "case 'harness_case_from_eventlog'", "case 'harness_case_from_workspace_snapshot'", "case 'harness_case_from_ai_conversation'", 'return zhCN.aiActions.harnessFromAIConversationResult', '[zhCN.harness.executionId, testCase.executionId]', '[zhCN.harness.replayRunId, testCase.replayRunId]', '[zhCN.harness.snapshotId, testCase.snapshotId]', '[zhCN.harness.snapshotPath, testCase.snapshotPath]']) {
    if (!shell.includes(snippet)) {
      console.error(`OuroborosIDE.tsx must render backend-authored Harness AI action results: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['result.result.eventlogEvents', 'AIActionEventlogEvents', 'zhCN.aiActions.eventlogQueryResult', "case 'eventlog_events'", 'return zhCN.aiActions.eventlogQueryResult', 'data-ai-action-eventlog-result="true"', '[zhCN.aiActions.eventlogRunInput, event.runId]', '[zhCN.aiActions.eventlogLaneInput, eventlogLaneLabel(event.lane)]', '[zhCN.aiActions.eventlogTypeInput, eventlogTypeLabel(event.type)]', '[zhCN.replay.rawEventId, event.rawEventId]', 'contentPreview(event.payloadJson || event.detail)']) {
    if (!shell.includes(snippet)) {
      console.error(`OuroborosIDE.tsx must render real eventlog.query ProjectEventlogEvents results: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['zhCN.aiActions.harnessFromEnvironmentSnapshotResult', "case 'harness_case_from_environment_snapshot'", 'return zhCN.aiActions.harnessFromEnvironmentSnapshotResult', "case 'environment_snapshot'", 'return zhCN.harness.kinds.environment_snapshot', 'HarnessInputSnapshotSummary', 'data-harness-provider-snapshot="true"', 'data-harness-toolchain-snapshot="true"', '[zhCN.harness.snapshotPath, testCase.snapshotPath]']) {
    if (!shell.includes(snippet)) {
      console.error(`OuroborosIDE.tsx must render environment snapshot Harness case/result metadata: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ["action.kind === 'sisyphus'", 'isSisyphusAIAction', 'zhCN.aiActions.sisyphusKind', 'buildAISisyphusInput', 'sisyphus.conclude_failure', 'sisyphusExecutionInput', 'sisyphusFailureKindInput', 'sisyphusFailureMessageInput', 'sisyphusTranscriptInput', 'zhCN.aiActions.sisyphusRequired', 'input: sisyphusInput']) {
    if (!shell.includes(snippet)) {
      console.error(`OuroborosIDE.tsx must expose structured Sisyphus AI action input fields: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ["result.result.kind === 'sisyphus_conclusion'", 'data-ai-action-sisyphus-result', 'zhCN.aiActions.sisyphusResult', 'zhCN.aiActions.sisyphusSummary', 'zhCN.aiActions.sisyphusBlackboard', 'zhCN.aiActions.sisyphusAgentTask']) {
    if (!shell.includes(snippet)) {
      console.error(`OuroborosIDE.tsx must render backend-authored Sisyphus AI action results: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ["action.kind === 'strategy'", 'isStrategyAIAction', 'zhCN.aiActions.strategyKind', 'buildAIStrategyInput', 'strategy.selector.select', 'strategyGoalInput', 'strategyBudgetInput', 'strategyReadyFilterInput', 'input: strategyInput']) {
    if (!shell.includes(snippet)) {
      console.error(`OuroborosIDE.tsx must expose structured strategy.selector.select AI action input fields: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ["result.result.kind === 'strategy_selection'", 'data-ai-action-strategy-result', 'AIActionStrategySelection', 'zhCN.aiActions.strategySelectionResult', 'zhCN.aiActions.strategyCandidateQueue', 'zhCN.aiActions.strategySelectedBlackboard', 'zhCN.aiActions.strategySelectedTask']) {
    if (!shell.includes(snippet)) {
      console.error(`OuroborosIDE.tsx must render backend-authored strategy selector action results: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['shouldRefreshFrontierStateForAIAction', 'return isFrontierAIAction(action) || isStrategyAIAction(action)', 'result.frontierQueue', 'refreshFrontierDependencies(frontierQueueQuery)', 'refreshFrontierQueue(frontierQueueQuery)', 'refreshFrontierMonitor(frontierQueueQuery)']) {
    if (!shell.includes(snippet)) {
      console.error(`OuroborosIDE.tsx must synchronize real Frontier state after strategy.selector.select results: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['AgentStrategySummary', 'data-agent-strategy-summary="true"', 'isStrategyAgentTask', 'isStrategySelectorAgentTask', 'isStrategyMutatorAgentTask', 'data-agent-strategy-task-id', 'zhCN.agents.strategySummary', 'zhCN.agents.strategySelectorCount', 'zhCN.agents.strategyMutatorCount']) {
    if (!shell.includes(snippet)) {
      console.error(`OuroborosIDE.tsx must expose first-class strategy selector/mutator state in the Agent workbench: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ["case 'strategy_selection':", 'zhCN.blackboard.kinds.strategySelection']) {
    if (!shell.includes(snippet)) {
      console.error(`OuroborosIDE.tsx must render strategy_selection blackboard items with Chinese product copy: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['AIActionPromptDiffResult', 'result.result.promptDiff', 'zhCN.aiActions.promptDiffResult', 'promptDiff.baseVersionId', 'promptDiff.targetVersionId', 'contentPreview(promptDiff.baseContent)', 'contentPreview(promptDiff.targetContent)', 'promptDiff.addedLines', 'promptDiff.removedLines', 'promptDiff.diff']) {
    if (!shell.includes(snippet)) {
      console.error(`OuroborosIDE.tsx must render backend-authored PromptDiff details with Chinese labels: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ["action.kind === 'orchestrator'", 'isOrchestratorAIAction', 'zhCN.aiActions.orchestratorKind', 'buildAIOrchestratorInput', 'orchestrator.run', 'orchestrator.observe', 'orchestrator.advance', 'orchestrator.auto_advance', 'orchestrator.step', 'orchestrator.loop', 'zhCN.aiActions.orchestratorAutoAdvanceRequired', 'zhCN.aiActions.orchestratorStepRequired', 'zhCN.aiActions.orchestratorLoopRequired', 'orchestratorTitleInput', 'orchestratorGoalInput', 'orchestratorCommandInput', 'orchestratorExpectedResultInput', 'orchestratorPriorityInput', 'orchestratorSourceInput', 'orchestratorFilePathInput', 'orchestratorRunIdInput', 'orchestratorObservationIdInput', 'orchestratorFollowupCommandInput', 'orchestratorMaxStepsInput', 'maxSteps', 'input: orchestratorInput']) {
    if (!shell.includes(snippet)) {
      console.error(`OuroborosIDE.tsx must expose structured Orchestrator AI action input fields: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['result.result.orchestratorState', 'result.result.orchestratorRuns', 'result.result.orchestratorRun', 'result.result.orchestratorObservation', 'result.result.orchestratorObservations', 'result.result.orchestratorAdvance', 'result.result.orchestratorAdvances', 'AIActionOrchestratorRun', 'AIActionOrchestratorObservation', 'AIActionOrchestratorAdvance', 'zhCN.aiActions.orchestratorRunResult', 'zhCN.aiActions.orchestratorObserveResult', 'zhCN.aiActions.orchestratorAdvanceResult', 'zhCN.aiActions.noOrchestratorRuns', 'zhCN.aiActions.noOrchestratorObservations', 'zhCN.aiActions.noOrchestratorAdvances']) {
    if (!shell.includes(snippet)) {
      console.error(`OuroborosIDE.tsx must render backend-authored Orchestrator AI action results: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['AIActionAutoParsedEvidence', 'autoParsedEvidenceItems={observation.autoParsedEvidenceItems}', 'autoParsedReportFindings={observation.autoParsedReportFindings}', 'autoParsedEvidenceItemIds={observation.autoParsedEvidenceItemIds}', 'autoParsedReportFindingIds={observation.autoParsedReportFindingIds}', 'autoParsedEvidenceCount={observation.autoParsedEvidenceCount}', 'autoParsedFindingCount={observation.autoParsedFindingCount}', 'autoParsedEvidenceItems={advance.autoParsedEvidenceItems}', 'autoParsedReportFindings={advance.autoParsedReportFindings}', 'autoParsedEvidenceCount={advance.autoParsedEvidenceCount}', 'zhCN.aiActions.autoParsedEvidence', 'zhCN.aiActions.autoParsedFindings']) {
    if (!shell.includes(snippet)) {
      console.error(`OuroborosIDE.tsx must render backend-authored Orchestrator auto-parse evidence/finding fields: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ["action.kind === 'planner'", "action.id === 'planner.dispatch'", "action.id === 'planner.execute'", "action.id === 'planner.observe'", "action.id === 'planner.advance'", "action.id === 'planner.step'", 'isPlannerAIAction', 'buildAIPlannerInput', 'planner.dispatch', 'planner.execute', 'planner.observe', 'planner.advance', 'planner.step', 'zhCN.aiActions.plannerKind', 'zhCN.aiActions.plannerDispatchRequired', 'zhCN.aiActions.plannerExecuteRequired', 'zhCN.aiActions.plannerObserveRequired', 'zhCN.aiActions.plannerAdvanceRequired', 'zhCN.aiActions.plannerStepRequired', 'input: plannerInput']) {
    if (!shell.includes(snippet)) {
      console.error(`OuroborosIDE.tsx must expose structured Planner Dispatch/Execute/Observe AI action input: ${snippet}`);
      failed = true;
    }
  }
  const plannerInputBlock = shell.match(/function buildAIPlannerInput[\s\S]*?function buildAIEvidenceInput/)?.[0] || '';
  for (const snippet of ["action.id === 'planner.execute' || action.id === 'planner.observe' || action.id === 'planner.advance' || action.id === 'planner.step'", "action.id !== 'planner.dispatch'", "aiActionInputValue(aiActionInputs, action.id, 'plannerPlanId')", "aiActionInputValue(aiActionInputs, action.id, 'executionId')", "aiActionInputValue(aiActionInputs, action.id, 'title')", "aiActionInputValue(aiActionInputs, action.id, 'goal')", "aiActionInputValue(aiActionInputs, action.id, 'summary')", "aiActionInputValue(aiActionInputs, action.id, 'command')", "aiActionInputValue(aiActionInputs, action.id, 'expectedResult')", "aiActionInputValue(aiActionInputs, action.id, 'priority')", "aiActionInputValue(aiActionInputs, action.id, 'source')", "aiActionInputValue(aiActionInputs, action.id, 'filePath')"]) {
    if (!plannerInputBlock.includes(snippet)) {
      console.error(`OuroborosIDE.tsx must send Planner Dispatch/Execute/Observe backend input fields: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['ai-action-planner-fields', 'zhCN.aiActions.plannerDispatchHint', 'zhCN.aiActions.plannerExecuteHint', 'zhCN.aiActions.plannerObserveHint', 'zhCN.aiActions.plannerAdvanceHint', 'zhCN.aiActions.plannerStepHint', 'zhCN.aiActions.plannerPlanIdInput', 'zhCN.aiActions.plannerExecutionInput', 'zhCN.aiActions.plannerTitleInput', 'zhCN.aiActions.plannerGoalInput', 'zhCN.aiActions.plannerSummaryInput', 'zhCN.aiActions.plannerCommandInput', 'zhCN.aiActions.plannerExpectedResultInput', 'zhCN.aiActions.plannerPriorityInput', 'zhCN.aiActions.plannerSourceInput', 'zhCN.aiActions.plannerFilePathInput']) {
    if (!shell.includes(snippet)) {
      console.error(`OuroborosIDE.tsx must render Chinese Planner Dispatch/Execute/Observe input controls: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['result.result.plannerState', 'result.result.plannerPlans', 'result.result.plannerPlan', 'AIPlannerStateSummary', 'AIPlannerPlanCard', 'zhCN.aiActions.plannerDispatchResult', 'zhCN.aiActions.plannerExecuteResult', 'zhCN.aiActions.plannerObserveResult', 'zhCN.aiActions.plannerAdvanceResult', 'zhCN.aiActions.plannerStepResult', 'zhCN.aiActions.noPlannerPlans', 'zhCN.aiActions.plannerStatePath', 'zhCN.aiActions.plannerPlanCount', 'zhCN.aiActions.plannerContextSummary', 'zhCN.aiActions.plannerNextActionTitle', 'zhCN.aiActions.plannerNextActionGoal', 'zhCN.aiActions.plannerNextActionCommand', 'zhCN.aiActions.plannerExecutionInput', 'zhCN.aiActions.plannerExecutionStatus', 'zhCN.aiActions.plannerExecutedAt', 'zhCN.aiActions.plannerObservedAt', 'zhCN.aiActions.plannerObservationSummary', 'zhCN.aiActions.plannerLogPath', 'zhCN.aiActions.plannerArtifactDir', 'zhCN.aiActions.plannerArtifactCount', 'zhCN.aiActions.plannerExitCode', 'zhCN.aiActions.plannerHarnessResultInput', 'plan.nextActionTitle', 'plan.nextActionGoal', 'plan.nextActionCommand', 'plan.executionId', 'plan.executionStatus', 'plan.executedAt', 'plan.observedAt', 'plan.observationSummary', 'plan.logPath', 'plan.artifactDir', 'plan.artifactCount', 'plan.exitCode', 'plan.harnessResultId']) {
    if (!shell.includes(snippet)) {
      console.error(`OuroborosIDE.tsx must render backend-authored Planner Dispatch/Execute/Observe result fields: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ["action.kind === 'frontier'", "action.id === 'frontier.prioritize'", "action.id === 'frontier.execute_next'", "action.id === 'frontier.dependencies'", "action.id === 'frontier.step'", "action.id === 'frontier.loop'", "action.id === 'frontier.autopilot'", 'isFrontierAIAction', 'buildAIFrontierInput', 'zhCN.aiActions.frontierKind', 'zhCN.aiActions.frontierPrioritizeRequired', 'zhCN.aiActions.frontierExecuteNextRequired', 'zhCN.aiActions.frontierStepRequired', 'zhCN.aiActions.frontierLoopRequired', 'zhCN.aiActions.frontierAutopilotRequired', 'frontierGoalInput', 'frontierBudgetInput', 'frontierLimitInput', 'frontierMaxStepsInput', 'frontierSourceInput', 'frontierStatusFilterInput', 'frontierPriorityFilterInput', 'frontierAgentTaskPlaceholder', 'frontierPlannerPlanPlaceholder', 'frontierExecutionPlaceholder', 'frontierCommandPlaceholder', 'input: frontierInput']) {
    if (!shell.includes(snippet)) {
      console.error(`OuroborosIDE.tsx must expose structured frontier.prioritize AI action input fields: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ["action.kind === 'goal'", "action.id === 'goal.decompose'", 'isGoalAIAction', 'buildAIGoalInput', 'goalTitleInput', 'goalSourceInput', 'goalPriorityInput', 'goalMaxItemsInput', 'zhCN.aiActions.goalKind', 'zhCN.aiActions.goalDecomposeRequired', 'input: goalInput']) {
    if (!shell.includes(snippet)) {
      console.error(`OuroborosIDE.tsx must expose structured goal.decompose AI action input fields: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['result.result.frontierItems', 'result.result.frontierSummary', 'result.refreshedFrontierItems', 'result.refreshedFrontierSummary', 'AIActionFrontierPriorities', 'AIActionFrontierPriorityItem', 'AIActionFrontierExecuteNext', "result.result.kind === 'frontier_priorities'", "result.result.kind === 'frontier_execute_next'", "result.result.kind === 'frontier_step'", "result.result.kind === 'frontier_loop'", "result.result.kind === 'frontier_autopilot'", 'zhCN.aiActions.frontierPrioritiesResult', 'zhCN.aiActions.frontierExecuteNextResult', 'zhCN.aiActions.frontierStepResult', 'zhCN.aiActions.frontierLoopResult', 'zhCN.aiActions.frontierAutopilotResult', 'zhCN.aiActions.frontierQueueTitle', 'zhCN.aiActions.frontierSelectedItem', 'zhCN.aiActions.frontierRefreshedQueue', 'zhCN.aiActions.frontierRefreshedSummary', 'zhCN.aiActions.frontierQueueSource', 'zhCN.aiActions.frontierQueueStatus', 'zhCN.aiActions.frontierTransitionCount', 'zhCN.aiActions.frontierTaskEvent', 'zhCN.aiActions.frontierPlannerPlan', 'zhCN.aiActions.frontierLoopPlans', 'zhCN.aiActions.frontierLoopEvents', 'zhCN.aiActions.frontierLoopLogs', 'zhCN.aiActions.frontierLoopArtifacts', 'zhCN.aiActions.frontierRank', 'zhCN.aiActions.frontierScore', 'zhCN.aiActions.frontierReason', 'zhCN.aiActions.frontierSuggestedAction', 'zhCN.aiActions.frontierReady', 'zhCN.aiActions.frontierDependencyIds', 'zhCN.aiActions.frontierBlockedByIds', 'zhCN.aiActions.frontierBlockedReason', 'zhCN.aiActions.noFrontierItems']) {
    if (!shell.includes(snippet)) {
      console.error(`OuroborosIDE.tsx must render backend-authored frontier priority result fields: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ["result.result.kind === 'goal_decomposition'", 'AIActionGoalDecomposition', 'zhCN.aiActions.goalDecomposeResult', 'zhCN.aiActions.goalTotalGoal', 'zhCN.aiActions.goalBlackboardItems', 'zhCN.aiActions.goalAgentTasks', 'zhCN.aiActions.goalFrontierQueue', 'result.frontierQueue', "type AIActionFollowupID = 'frontier.execute_next' | 'frontier.autopilot' | 'harness.cases.from_ai_conversation';", 'type AIActionFollowupRunner = (actionId: AIActionFollowupID, input: AIActionRunInput) => void;', 'async function runAIActionFollowup(actionId: AIActionFollowupID, input: AIActionRunInput)', 'await runAIAction({ actionId, input });', 'setAIActionResult(result);', 'data-ai-goal-followup-actions="true"', 'data-ai-goal-followup-action="frontier.execute_next"', 'data-ai-goal-followup-action="frontier.autopilot"', "onRunFollowupAction('frontier.execute_next', executeNextInput)", "onRunFollowupAction('frontier.autopilot', autopilotInput)", 'zhCN.aiActions.goalFollowupExecuteNext', 'zhCN.aiActions.goalFollowupAutopilot']) {
    if (!shell.includes(snippet)) {
      console.error(`OuroborosIDE.tsx must render backend-authored goal.decompose action results: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of [
    '<AIStreamEventsPanel',
    'events={streamEvents}',
    'sending={sending}',
    'onOpenReportActionFile={onOpenReportActionFile}',
    'onOpenContextReference={onOpenContextReference}',
    'onRunFollowupAction={onRunFollowupAction}',
    'function AIStreamEventsPanel({ events, sending, onOpenReportActionFile, onOpenContextReference, onRunFollowupAction, actionRunningId }',
    'onOpenReportActionFile: (filePath: string) => void;',
    'onOpenContextReference: (reference: AIContextReference) => void;',
    'onRunFollowupAction: AIActionFollowupRunner;',
    'event.actionResult &&',
    'data-ai-stream-event-action-result="true"',
    'renderAIActionResult(event.actionResult, onOpenReportActionFile, onOpenContextReference, onRunFollowupAction, actionRunningId',
    '<AIActionGoalDecomposition result={result} onOpenFile={onOpenReportActionFile} onOpenReference={onOpenReference} onRunFollowupAction={onRunFollowupAction} />',
  ]) {
    if (!shell.includes(snippet)) {
      console.error(`OuroborosIDE.tsx must render AI stream event actionResult through the full action result cards, including goal.decompose decomposition cards: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['loadFrontierQueue', 'FrontierQueue', 'FrontierQueueQuery', 'AIFrontierQueuePanel', 'frontierQueueStatus', 'frontierQueueBusy', 'onRunFrontierQueueAction', 'normalizeFrontierQueueQuery', 'shouldRefreshFrontierQueueForEvent', "event.type.startsWith('frontier:')", "onRunAction('frontier.execute_next', item)", "onRunAction('frontier.autopilot')", 'data-ai-frontier-queue="true"', 'data-frontier-queue-item="true"', 'data-frontier-queue-query="goal"', 'data-frontier-queue-ready-filter', 'data-frontier-queue-mode={value}', 'frontierQueueModeReady', 'frontierQueueModeBlocked', 'data-frontier-queue-action="refresh"', 'data-frontier-queue-action="execute-next"', 'data-frontier-queue-action="autopilot"', 'data-frontier-queue-status="true"', 'data-frontier-queue-dependency-id', 'data-frontier-queue-blocked-by-id', 'result.frontierQueue', 'zhCN.ai.frontierQueue']) {
    if (!shell.includes(snippet)) {
      console.error(`OuroborosIDE.tsx must expose persistent real Frontier queue controls: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['AIStrategyStatePanel', '<AIStrategyStatePanel', 'blackboard={ideData.blackboard}', 'agentState={ideData.agentState}', 'frontierQueue={frontierQueue}', 'frontierQueueQuery={frontierQueueQuery}', 'onRunStrategySelection', "actionId: 'strategy.selector.select'", 'data-ai-strategy-panel="true"', 'data-ai-strategy-blackboard-id', 'data-ai-strategy-task-id', 'data-ai-strategy-action="select"', 'zhCN.ai.strategyPanel']) {
    if (!shell.includes(snippet)) {
      console.error(`OuroborosIDE.tsx must expose first-class AI Workbench strategy selector panel backed by real state: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of [
    'AIFailureStrategyPanel',
    '<AIFailureStrategyPanel',
    'executions={ideData.executionSteps}',
    'onRunFailureStrategySisyphus',
    'onRunFailureStrategyFollowup',
    "actionId: 'sisyphus.conclude_failure'",
    "actionId: 'frontier.autopilot'",
    "actionId: 'harness.cases.from_failure'",
    'data-ai-failure-strategy-panel="true"',
    'data-ai-failure-execution-select',
    'data-ai-failure-action="conclude"',
    'data-ai-failure-action="frontier-autopilot"',
    'data-ai-failure-action="harness-from-failure"',
    'data-ai-failure-blackboard-id',
    'data-ai-failure-agent-task-id',
    'data-ai-failure-prompt-version',
    'data-ai-failure-mutation-hint',
    'zhCN.ai.failureStrategyPanel',
  ]) {
    if (!shell.includes(snippet)) {
      console.error(`OuroborosIDE.tsx must expose first-class AI Workbench failure strategy loop backed by real execution DTOs and backend AI actions: ${snippet}`);
      failed = true;
    }
  }
  const aiWorkbenchBlock = shell.match(/function AIWorkbench\([\s\S]*?function AIFileChangeSetPanel/)?.[0] || '';
  if (!aiWorkbenchBlock.includes('<AIFrontierQueuePanel') || !aiWorkbenchBlock.includes('<AIStrategyStatePanel')) {
    console.error('AIWorkbench must render AIStrategyStatePanel in the AI Workbench after the Frontier queue panel');
    failed = true;
  } else if (aiWorkbenchBlock.indexOf('<AIStrategyStatePanel') < aiWorkbenchBlock.indexOf('<AIFrontierQueuePanel')) {
    console.error('AIWorkbench must render AIStrategyStatePanel after AIFrontierQueuePanel');
    failed = true;
  }
  for (const snippet of ['executions,', 'executions: ExecutionStep[];', '<AIFailureStrategyPanel']) {
    if (!aiWorkbenchBlock.includes(snippet)) {
      console.error(`AIWorkbench must accept real execution DTOs and render the failure strategy panel: ${snippet}`);
      failed = true;
    }
  }
  if (!aiWorkbenchBlock.includes('<AIFrontierQueuePanel') || !aiWorkbenchBlock.includes('<AIFailureStrategyPanel')) {
    console.error('AIWorkbench must render AIFailureStrategyPanel in the AI Workbench after the Frontier queue panel');
    failed = true;
  } else if (aiWorkbenchBlock.indexOf('<AIFailureStrategyPanel') < aiWorkbenchBlock.indexOf('<AIFrontierQueuePanel')) {
    console.error('AIWorkbench must render AIFailureStrategyPanel after AIFrontierQueuePanel');
    failed = true;
  }
  const aiWorkbenchInvocationBlock = shell.match(/<AIWorkbench\b[\s\S]*?\/>/)?.[0] || '';
  if (!aiWorkbenchInvocationBlock.includes('executions={ideData.executionSteps}')) {
    console.error('OuroborosIDE.tsx must pass backend execution DTOs into AIWorkbench with executions={ideData.executionSteps}');
    failed = true;
  }
  const strategyPanelBlock = shell.match(/function AIStrategyStatePanel[\s\S]*?function AIFrontierDependenciesPanel/)?.[0] || '';
  if (!strategyPanelBlock) {
    console.error('OuroborosIDE.tsx must define AIStrategyStatePanel before the Frontier dependency panel');
    failed = true;
  }
  for (const snippet of ['blackboard.items', 'isStrategyBlackboardItem', 'isStrategySelectorAgentTask', 'isStrategyMutatorAgentTask', 'frontierQueue', 'frontierQueueQuery', 'onClick={onRunStrategySelection}', 'zhCN.ai.strategyPanelMeta', 'zhCN.ai.strategySelect', 'zhCN.ai.strategySelecting', 'zhCN.ai.strategyNoState', 'zhCN.ai.strategyLatestSelection', 'zhCN.ai.strategyLatestTask', 'zhCN.ai.strategySelectorTasks', 'zhCN.ai.strategyMutatorTasks']) {
    if (!strategyPanelBlock.includes(snippet)) {
      console.error(`AIStrategyStatePanel must render real blackboard strategy_selection, agent strategy tasks, frontier query/queue state, and Chinese copy: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ["kind === 'strategy_selection'", 'strategy_selector', 'strategy_mutator']) {
    if (!shell.includes(snippet)) {
      console.error(`OuroborosIDE.tsx must back AIStrategyStatePanel helpers with real strategy state literals: ${snippet}`);
      failed = true;
    }
  }
  const failureStrategyPanelBlock = shell.match(/function AIFailureStrategyPanel[\s\S]*?function AIFrontierDependenciesPanel/)?.[0] || '';
  if (!failureStrategyPanelBlock) {
    console.error('OuroborosIDE.tsx must define AIFailureStrategyPanel before the Frontier dependency panel');
    failed = true;
  }
  for (const snippet of [
    'executions',
    'executions.length',
    'selectedExecutionId',
    'selectedExecution',
    'onRunFailureStrategySisyphus',
    'onRunFailureStrategyFollowup',
    'data-ai-failure-strategy-panel="true"',
    'data-ai-failure-execution-select',
    'data-ai-failure-action="conclude"',
    'data-ai-failure-action="frontier-autopilot"',
    'data-ai-failure-action="harness-from-failure"',
    'data-ai-failure-blackboard-id',
    'data-ai-failure-agent-task-id',
    'data-ai-failure-prompt-version',
    'data-ai-failure-mutation-hint',
    'zhCN.ai.failureStrategyTitle',
    'zhCN.ai.failureStrategyStatus',
    'zhCN.ai.failureStrategyConclude',
    'zhCN.ai.failureStrategyFrontierAutopilot',
    'zhCN.ai.failureStrategyHarnessFromFailure',
    'zhCN.ai.failureStrategyExecution',
    'zhCN.ai.failureStrategyBlackboard',
    'zhCN.ai.failureStrategyAgentTask',
    'zhCN.ai.failureStrategyPromptVersion',
    'zhCN.ai.failureStrategyMutationHint',
  ]) {
    if (!failureStrategyPanelBlock.includes(snippet)) {
      console.error(`AIFailureStrategyPanel must render real execution-backed failure analysis, strategy mutation, Frontier autopilot controls, stable selectors, and Chinese copy: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of [
    'function runFailureStrategySisyphus',
    "actionId: 'sisyphus.conclude_failure'",
    "actionId: 'frontier.autopilot'",
    "actionId: 'harness.cases.from_failure'",
    'executionId: selectedExecutionId',
  ]) {
    if (!shell.includes(snippet)) {
      console.error(`OuroborosIDE.tsx must wire failure strategy handlers to backend AI action ids without fake data: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['loadFrontierDependencies', 'FrontierDependencySelection', 'FrontierDependencyCluster', 'AIFrontierDependenciesPanel', 'frontierDependenciesStatus', 'frontierDependenciesBusy', 'onRefreshFrontierDependencies', 'onRunFrontierDependency', 'refreshFrontierDependencies(frontierQueueQueryRef.current)', 'refreshFrontierDependencies(frontierQueueQuery)', 'runFrontierDependencyAction', "actionId: 'frontier.execute_next'", 'data-ai-frontier-dependencies="true"', 'data-frontier-dependency-cluster-id', 'data-frontier-dependency-blocked-id', 'data-frontier-dependency-unblock-id', 'data-frontier-dependency-action="refresh"', 'data-frontier-dependency-action="execute-next"', 'zhCN.ai.frontierDependencies']) {
    if (!shell.includes(snippet)) {
      console.error(`OuroborosIDE.tsx must expose real Frontier dependency selection controls: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['loadFrontierMonitor', 'FrontierMonitor', 'AIFrontierMonitorPanel', 'frontierMonitorStatus', 'frontierMonitorBusy', 'onRefreshFrontierMonitor', 'refreshFrontierMonitor(frontierQueueQueryRef.current)', 'refreshFrontierMonitor(frontierQueueQuery)', 'data-ai-frontier-monitor="true"', 'data-frontier-monitor-status="true"', 'data-frontier-monitor-task-id', 'data-frontier-monitor-plan-id', 'data-frontier-monitor-execution-id', 'data-frontier-monitor-event-id', 'data-frontier-monitor-action="refresh"', 'frontierMonitorStatusLabel', 'frontierMonitorStatusTone', 'zhCN.ai.frontierMonitor']) {
    if (!shell.includes(snippet)) {
      console.error(`OuroborosIDE.tsx must expose real Frontier monitor projection controls: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ["action.kind === 'blackboard'", 'isBlackboardAIAction', 'zhCN.aiActions.blackboardKind', 'buildAIBlackboardInput', 'blackboard.read', 'blackboard.write', 'blackboardItemKindInput', 'blackboardTitleInput', 'blackboardContentInput', 'blackboardStatusInput', 'blackboardPriorityInput', 'blackboardSourceInput', 'blackboardFilePathInput', 'blackboardExecutionInput', 'input: blackboardInput']) {
    if (!shell.includes(snippet)) {
      console.error(`OuroborosIDE.tsx must expose structured Blackboard AI action input fields: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['isWorkspaceFileAIAction', 'buildAIActionRequest', 'renderAIActionInput', 'renderAIActionResult', 'zhCN.aiActions.fileKind', 'zhCN.aiActions.pathInput', 'zhCN.aiActions.contentInput', 'zhCN.aiActions.queryInput', 'zhCN.aiActions.targetPathInput']) {
    if (!shell.includes(snippet)) {
      console.error(`OuroborosIDE.tsx must expose structured workspace file AI action UI: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['workspace.list_files', 'workspace.read_file', 'workspace.read_files', 'workspace.search', 'workspace.write_file', 'workspace.preview_patch', 'workspace.apply_patch', 'workspace.rename_file', 'workspace.delete_file', 'const workspaceInput = buildAIWorkspaceFileInput(action, aiActionInputs)', 'input: workspaceInput']) {
    if (!shell.includes(snippet)) {
      console.error(`OuroborosIDE.tsx must send workspace file AI action fields through structured arguments: ${snippet}`);
      failed = true;
    }
  }
  const workspaceFileInputBlock = shell.match(/function buildAIWorkspaceFileInput[\s\S]*?function buildAIBlackboardInput/)?.[0] || '';
  for (const snippet of ["case 'workspace.read_files':", "aiActionInputValue(aiActionInputs, action.id, 'paths')", 'paths.length > 0 ? { paths } : null']) {
    if (!workspaceFileInputBlock.includes(snippet)) {
      console.error(`OuroborosIDE.tsx must build workspace.read_files paths[] input without falling back to raw arguments: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['zhCN.aiActions.pathsInput', 'zhCN.aiActions.pathsPlaceholder']) {
    if (!shell.includes(snippet)) {
      console.error(`OuroborosIDE.tsx must render Chinese workspace.read_files paths input controls: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['result.result.kind', 'result.result.file?.path', 'result.result.files', 'result.result.fileContents', 'data-ai-action-file-list="true"', 'data-ai-action-file-path', 'data-ai-action-file-contents="true"', 'data-ai-action-file-content="true"', 'data-workspace-file-path={file.path}', 'result.result.searchResults', 'result.result.deletedFile?.path', 'result.result.patch', 'zhCN.aiActions.resultStatus', 'zhCN.aiActions.patchDiff', 'zhCN.aiActions.fileContents']) {
    if (!shell.includes(snippet)) {
      console.error(`OuroborosIDE.tsx must render backend-authored AI action result/status without fake rows: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ["case 'file_contents':", 'return zhCN.aiActions.fileContents;', 'actionResult.fileContents ?? []']) {
    if (!shell.includes(snippet)) {
      console.error(`OuroborosIDE.tsx must render workspace.read_files file_contents results with Chinese labels and workspace file actions: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['AIFileChangeSetPanel', 'const fileChanges = useMemo', 'buildAIFileChangeSet(displayedActionTrace, streamEvents, actionResult)', 'changes={fileChanges}', 'const displayedActionTrace = conversation?.actionTrace.filter', 'streamEvents', 'actionResult', 'onOpenFile={onOpenReportActionFile}']) {
    if (!shell.includes(snippet)) {
      console.error(`OuroborosIDE.tsx must expose an AI file change set derived from real conversation/actionTrace/actionResult data: ${snippet}`);
      failed = true;
    }
  }
  const evidenceReferenceBlock = shell.match(/function appendEvidenceItemReferences[\s\S]*?function referencesForEvidenceItem/)?.[0] || '';
  for (const snippet of ['appendAIProvenanceReferences(references, item)']) {
    if (!evidenceReferenceBlock.includes(snippet)) {
      console.error(`OuroborosIDE.tsx must turn AI evidence provenance into clickable GUI references: ${snippet}`);
      failed = true;
    }
  }
  const findingReferenceBlock = shell.match(/function appendReportFindingReferences[\s\S]*?function referencesForReportFinding/)?.[0] || '';
  for (const snippet of ['appendAIProvenanceReferences(references, finding)']) {
    if (!findingReferenceBlock.includes(snippet)) {
      console.error(`OuroborosIDE.tsx must turn report finding provenance into clickable GUI references: ${snippet}`);
      failed = true;
    }
  }
  const provenanceReferenceBlock = shell.match(/function appendAIProvenanceReferences[\s\S]*?function referencesForReportFinding/)?.[0] || '';
  for (const snippet of ['item.aiMessageIds', 'item.toolCallIds', 'item.actionTraceIds', 'item.agentTaskId', 'item.blackboardItemId', 'item.promptId', 'item.promptVersionId', "'aiMessage'", "'aiActionTrace'", "'agentTask'", "'blackboard'", "'promptVersion'"]) {
    if (!provenanceReferenceBlock.includes(snippet)) {
      console.error(`OuroborosIDE.tsx must turn evidence/finding AI provenance into clickable GUI references: ${snippet}`);
      failed = true;
    }
  }
  const aiFileChangeSetBlock = shell.match(/function AIFileChangeSetPanel\([\s\S]*?function buildAIFileChangeSet/)?.[0] || '';
  if (!aiFileChangeSetBlock) {
    console.error('OuroborosIDE.tsx must define AIFileChangeSetPanel before building AI file change set helpers');
    failed = true;
  }
  for (const snippet of ['data-ai-file-change-set="true"', 'data-ai-file-change-item="true"', 'data-ai-file-change-open="primary"', 'data-ai-file-change-open="target"', 'data-workspace-file-path={change.path}', 'onOpenFile(change.path)', 'changes.map(change =>', 'data-ai-file-change-no-patch="true"', 'zhCN.ai.fileChangeSet', 'zhCN.ai.fileChangeSetMeta', 'zhCN.ai.fileChangeSetEmpty', 'zhCN.ai.fileChangeOpenFile', 'zhCN.ai.fileChangeOpenTarget', 'zhCN.ai.fileChangeNoPatch']) {
    if (!aiFileChangeSetBlock.includes(snippet)) {
      console.error(`AIFileChangeSetPanel must render testable Chinese file change set controls: ${snippet}`);
      failed = true;
    }
  }
  const aiFileChangeSetBuilderBlock = shell.match(/function buildAIFileChangeSet[\s\S]*?function AIFrontierQueuePanel/)?.[0] || '';
  if (!aiFileChangeSetBuilderBlock) {
    console.error('OuroborosIDE.tsx must build AI file change set data before rendering AIFileChangeSetPanel');
    failed = true;
  }
  for (const snippet of ['traces: AIActionTraceItem[]', 'streamEvents: AIChatStreamEvent[]', 'manualActionResult: AIActionRunResponse | null', "pushAIActionResultFileChanges(manualActionResult, 'result', pushChange)", 'streamEvents.forEach', 'traces.forEach', 'trace.contextReferences', "reference.kind === 'fileDiff'", 'workspace.write_file', 'workspace.apply_patch', 'workspace.rename_file', 'workspace.delete_file', 'result.fileChanges ?? []', 'result.fileDiffs ?? []', 'aiToolMessagePatch(messageItem.content)']) {
    if (!aiFileChangeSetBuilderBlock.includes(snippet)) {
      console.error(`buildAIFileChangeSet must derive file changes from real AI conversation/actionTrace/actionResult payloads: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['fakeFileChange', 'mockFileChange', 'demoFileChange', 'sampleFileChange', 'fileChanges: []']) {
    if (shell.includes(snippet)) {
      console.error(`OuroborosIDE.tsx must not fabricate AI file change set rows: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ["action.kind === 'report'", 'zhCN.aiActions.reportKind', 'report.create_finding', 'buildAIReportInput', 'reportTitleInput', 'reportSeverityInput', 'reportEndpointInput', 'reportDescriptionInput', 'reportEvidenceInput', 'reportRecommendationInput', 'reportFindingRequired', "'description'", "'recommendation'", "'evidence'", 'result.result.reportFinding', 'result.result.reportFindings', 'result.result.executions', 'result.result.executionLogs', 'result.result.executionArtifacts', 'result.result.reportExport', 'zhCN.aiActions.reportFindings', 'zhCN.aiActions.reportArtifacts']) {
    if (!shell.includes(snippet)) {
      console.error(`OuroborosIDE.tsx must render backend-authored AI report action results: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ["evidence.ingest", 'buildAIEvidenceInput', 'evidenceTitleInput', 'evidenceSummaryInput', 'evidenceSeverityInput', 'evidenceExecutionInput', 'evidenceArtifactInput', 'evidenceFilePathInput', 'evidenceSourceInput', 'evidenceNodeInput', 'evidenceReplayInput', 'evidenceIngestRequired', 'input: evidenceInput']) {
    if (!shell.includes(snippet)) {
      console.error(`OuroborosIDE.tsx must expose structured AI evidence ingestion fields: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['result.result.evidenceItem', 'AIActionEvidenceItem', 'zhCN.aiActions.evidenceIngestResult', 'zhCN.aiActions.evidenceTitle', 'zhCN.aiActions.evidenceSummary', 'zhCN.aiActions.evidenceSeverity', 'zhCN.aiActions.evidenceExecutionId', 'zhCN.aiActions.evidenceArtifactPath', 'zhCN.aiActions.evidenceFilePath', 'zhCN.aiActions.evidenceGraphNodeId', 'zhCN.aiActions.evidenceSource']) {
    if (!shell.includes(snippet)) {
      console.error(`OuroborosIDE.tsx must render backend-authored AI evidence ingestion result: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['const actionResultReferences = aiActionResultReferences(result);', '<AIActionStructuredReferenceActions references={actionResultReferences} onOpenReference={onOpenReference} />', 'function aiActionResultReferences(result: AIActionRunResponse): AIContextReference[]', 'appendEvidenceItemReferences', 'appendReportFindingReferences', 'appendHarnessResultReferences', 'appendPromptReferences', 'appendReportExportReferences', 'dedupeAIContextReferences', '<AIActionHarnessResult key={item.id} result={item} onOpenFile={onOpenReportActionFile} onOpenReference={onOpenReference} />', 'renderAIReportActionResult(reportActionResult, onOpenReportActionFile, onOpenReference)']) {
    if (!shell.includes(snippet)) {
      console.error(`OuroborosIDE.tsx must render AI action result evidence/report/finding/harness/prompt references as clickable Chinese IDE controls: ${snippet}`);
      failed = true;
    }
  }
  const reportInputBlock = shell.match(/function buildAIReportInput[\s\S]*?function buildAIActionRequest/)?.[0] || '';
  const reportFieldsBlock = shell.match(/if \(action\.id === 'report\.create_finding'\)[\s\S]*?return <span className="ai-action-no-input"/)?.[0] || '';
  for (const snippet of ["'content'", "'expectedResult'"]) {
    if (reportInputBlock.includes(snippet) || reportFieldsBlock.includes(`updateStructuredInput(${snippet}`)) {
      console.error(`OuroborosIDE.tsx must send report.create_finding fields using backend contract names, not legacy report field aliases: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['result.result.blackboardItems', 'result.result.blackboardItem', 'AIActionBlackboardItem', 'zhCN.aiActions.blackboardReadResult', 'zhCN.aiActions.blackboardWriteResult', 'zhCN.aiActions.noBlackboardItems']) {
    if (!shell.includes(snippet)) {
      console.error(`OuroborosIDE.tsx must render backend-authored Blackboard AI action results: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['zhCN.ai.title', 'zhCN.ai.loadingConversation', 'zhCN.ai.send', 'zhCN.ai.providerMissing', 'zhCN.ai.sendFailed', 'zhCN.ai.tokenUsage']) {
    if (!shell.includes(snippet)) {
      console.error(`OuroborosIDE.tsx must render Chinese AI workbench state copy: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['zhCN.aiActions.loading', 'zhCN.aiActions.empty', 'zhCN.aiActions.commandInput', 'zhCN.aiActions.parametersInput', 'zhCN.aiActions.executionId', 'zhCN.aiActions.executionStatus', 'zhCN.aiActions.runFailed']) {
    if (!shell.includes(snippet)) {
      console.error(`OuroborosIDE.tsx must render Chinese AI Actions state copy: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ["role: 'assistant'", "provider: 'mock'", "provider: 'demo'", "model: 'demo'", "message: { role: 'assistant'", 'fakeAssistant', 'mockAssistant', 'demoConversation']) {
    if (shell.includes(snippet)) {
      console.error(`OuroborosIDE.tsx must not fabricate AI assistant/provider/message fallbacks: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['fakeAIAction', 'mockAIAction', 'demoAIAction', 'fallbackAIAction', 'sampleAIAction', 'actions: [] as AIAction[]']) {
    if (shell.includes(snippet)) {
      console.error(`OuroborosIDE.tsx must not fabricate AI Actions or fake action fallbacks: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['fakeEvidence', 'mockEvidence', 'demoEvidence', 'sampleEvidence', '证据内容示例', '示例证据内容', '样例证据内容']) {
    if (shell.includes(snippet)) {
      console.error(`OuroborosIDE.tsx must not mock AI evidence ingestion content in the frontend: ${snippet}`);
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
  'PlannerDispatch',
  'PlannerPlanStep',
  'dispatches?: PlannerDispatch[]',
  'dispatches: PlannerDispatch[]',
  'steps?: PlannerPlanStep[]',
  'steps: PlannerPlanStep[]',
  'plannerDispatches',
  'plannerSteps',
  'fakePlanner',
  'mockPlanner',
  'demoPlanner',
  'samplePlanner',
  'plannerDemo',
  'plannerMock',
  'plannerSample',
  '示例规划',
  '样例规划',
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
  const apiSourceFile = ts.createSourceFile('api.ts', api, ts.ScriptTarget.Latest, true, ts.ScriptKind.TS);
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
	  for (const snippet of [
	    'export async function loadReportFile(path: string)',
	    'export async function saveReportFile(path: string, content: string)',
	    '/api/reports/file',
	  ]) {
    if (!api.includes(snippet)) {
      console.error(`api.ts must expose first-class report file API helpers: ${snippet}`);
	      failed = true;
	    }
	  }
	  for (const snippet of ['providerName?: string', 'provider_name?: string', 'model?: string', 'providerName: cleanOptionalString(version.providerName ?? version.provider_name)', 'model: cleanOptionalString(version.model)']) {
	    if (!api.includes(snippet)) {
	      console.error(`api.ts must preserve prompt version provider/model binding from /api/prompts: ${snippet}`);
	      failed = true;
	    }
	  }
	  for (const snippet of ['ToolAction', 'ToolActionRunResponse', 'runToolchainAction', 'loadToolchainActions', 'ToolchainActionsResponse', 'toolActions?:', '/api/toolchain/actions', 'mapToolActions']) {
	    if (!api.includes(snippet)) {
	      console.error(`api.ts must consume real manifest-backed tool action API: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['BlackboardResponse', 'BlackboardItemResponse', 'blackboard?: BlackboardResponse', 'blackboard?:', 'const blackboard = mapBlackboard(response.blackboard)', 'blackboard,', 'function mapBlackboard(', 'function mapBlackboardItem(', 'path: response.path', 'items: response.items.flatMap', 'createdAt: item.createdAt', 'updatedAt: item.updatedAt']) {
    if (!api.includes(snippet)) {
      console.error(`api.ts must consume backend Blackboard v1 bootstrap projection: ${snippet}`);
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
  for (const snippet of ['saveProviderConfig', 'deleteProviderConfig', 'testProviderConfig', '/api/provider', '/api/provider/test', 'ProviderTestResult', 'provider?:', 'ProviderSettings']) {
    if (!api.includes(snippet)) {
      console.error(`api.ts must expose real provider settings API: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['loadToolchainPacks', '/api/toolchain', '/api/toolchain/health', 'mapRequiredToolchainResponse', 'mergeToolchainHealth', 'saveToolchainPack', '/api/toolchain/packs', 'ToolchainPackSettings', 'manifestYaml']) {
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
  for (const snippet of ['fetchPromptRegistry', 'savePromptRegistry', '/api/prompts', 'PromptRegistryResponse', 'PromptVariableResponse', 'mapPromptRegistryResponse', 'mapPromptDefinitionResponse', 'mapPromptVersionResponse', 'mapPromptVariableResponse', 'variables: Array.isArray(version.variables)']) {
    if (!api.includes(snippet)) {
      console.error(`api.ts must expose real Prompt Registry API: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['searchWorkspace(query: string)', '/api/workspace/search?q=', 'encodeURIComponent(query)', 'WorkspaceSearchResult']) {
    if (!api.includes(snippet)) {
      console.error(`api.ts must expose real workspace search API: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['renameWorkspaceFile(path: string, targetPath: string)', "method: 'PATCH'", 'JSON.stringify({ targetPath })', '/api/workspace/file?path=']) {
    if (!api.includes(snippet)) {
      console.error(`api.ts must expose real workspace file rename API: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['deleteWorkspaceFile(path: string)', "method: 'DELETE'", 'Promise<WorkspaceFile>', '/api/workspace/file?path=']) {
    if (!api.includes(snippet)) {
      console.error(`api.ts must expose real workspace file delete API: ${snippet}`);
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
  for (const snippet of ['loadEventlogEvents(query: EventlogQuery = {})', '/api/events', 'appendEventlogQuery(params, \'runId\', query.runId)', 'appendEventlogQuery(params, \'lane\', query.lane)', 'appendEventlogQuery(params, \'type\', query.type)', 'appendEventlogQuery(params, \'limit\', query.limit)', 'mapEventlogEventsResponse', 'mapEventlogReplayEvent', 'runs: mapEventlogRuns(response.runs)', 'const events = (response.events ?? []).flatMap(mapEventlogReplayEvent)', 'count: isFiniteNumber(response.count) ? response.count : events.length']) {
    if (!api.includes(snippet)) {
      console.error(`api.ts must fetch and map raw /api/events SQLite eventlog records: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['DesktopStatus', 'DesktopRuntimeInfo', 'loadDesktopStatus', 'restartDesktopBackend', 'openRecentDesktopWorkspace', 'window.ouroboros?.backendStatus', 'window.ouroboros?.runtimeInfo', 'window.ouroboros?.restartBackend', 'window.ouroboros?.openRecentWorkspace']) {
    if (!api.includes(snippet)) {
      console.error(`api.ts must expose real desktop lifecycle API: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['export async function loadDesktopStatus(): Promise<DesktopStatus>', 'window.ouroboros.backendStatus()', 'window.ouroboros?.runtimeInfo?.() ?? Promise.resolve({})', 'export async function restartDesktopBackend(): Promise<DesktopStatus>', 'await window.ouroboros.restartBackend()', 'const status = mapDesktopStatus(backend, recentWorkspaces, windowState, runtime)']) {
    if (!api.includes(snippet)) {
      console.error(`api.ts must load/restart desktop status through the real preload backend/runtime bridge: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['Promise.allSettled', "status: 'unknown'", "backendResult.status === 'fulfilled'", 'exitCodeField(source.lastExitCode)', 'lastStdout: stringField(source.lastStdout)', 'lastStderr: stringField(source.lastStderr)']) {
    if (!api.includes(snippet)) {
      console.error(`api.ts must preserve partial desktop status/runtime diagnostics without misclassifying status read failures: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['normalizeDesktopBackendStatus', 'isCleanDesktopBackendExit(runtime.lastExitCode, runtime.lastExitSignal)', "return 'offline'", "code === 0 || signal === 'SIGTERM' || signal === 'SIGINT'"]) {
    if (!api.includes(snippet)) {
      console.error(`api.ts must defensively map clean backend exits to offline instead of crashed: ${snippet}`);
      failed = true;
    }
  }
  if (api.includes("return mapDesktopStatus({ status: 'crashed', lastError: errorMessage(error) });")) {
    console.error('api.ts must not map desktop status read/restart exceptions directly to crashed');
    failed = true;
  }
  for (const snippet of ['replayEvents?:', 'rawEventId?: number', 'payloadJson?: string', 'reportFindings?:', 'mapBootstrapReplayEvents', 'mapBootstrapReportFindings']) {
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
  const bootstrapReplayMapperSource = namedDeclarationSource(apiSourceFile, 'mapBootstrapReplayEvents');
  const eventlogReplayMapperSource = namedDeclarationSource(apiSourceFile, 'mapEventlogReplayEvent');
  const replayTypeTranslatorSource = namedDeclarationSource(apiSourceFile, 'translateReplayEventType');
  if (!replayTypeTranslatorSource.includes('unknownBackendValueLabel(zhCN.replay.unknownType, type)')) {
    console.error('api.ts must render unknown backend replay event types with Chinese copy and the real backend enum value');
    failed = true;
  }
  for (const [label, source] of [
    ['bootstrap replay mapper', bootstrapReplayMapperSource],
    ['eventlog replay mapper', eventlogReplayMapperSource],
  ]) {
    if (source.includes('if (!eventLabel)') || source.includes('?? event.type')) {
      console.error(`api.ts ${label} must preserve unknown real events through the Chinese event-type translator`);
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
  const unavailableBlock = namedDeclarationSource(apiSourceFile, 'mapBootstrapUnavailableData');
  if (!unavailableBlock.includes('plannerState: emptyPlannerState()')) {
    console.error('api.ts bootstrap-unavailable IDE data must include empty plannerState so the AI IDE can render while backend bootstrap recovers');
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
  const apiStatusMapperSource = namedDeclarationSource(apiSourceFile, 'mapBackendAPIStatus');
  if (!apiStatusMapperSource) {
    console.error('api.ts must strictly map real backend status to the status-bar API state');
    failed = true;
  } else {
    try {
      const executableMapperSource = apiStatusMapperSource
        .replace('backendStatus?: string', 'backendStatus')
        .replace('): ApiStatus', ')');
      const mapBackendAPIStatus = Function(`return (${executableMapperSource});`)();
      const expectedAPIStatuses = [
        ['ready', 'connected'],
        [' READY ', 'connected'],
        ['degraded', 'degraded'],
        ['configured', 'degraded'],
        ['invalid', 'degraded'],
        ['disabled', 'degraded'],
        ['idle', 'degraded'],
        ['incomplete', 'degraded'],
        ['update_available', 'degraded'],
        ['empty', 'degraded'],
        ['unknown', 'degraded'],
        ['starting', 'degraded'],
        ['offline', 'offline'],
        ['crashed', 'offline'],
        ['missing', 'degraded'],
        ['healthy', 'degraded'],
        ['unexpected', 'degraded'],
        [undefined, 'degraded'],
      ];
      for (const [backendStatus, expectedAPIStatus] of expectedAPIStatuses) {
        const actualAPIStatus = mapBackendAPIStatus(backendStatus);
        if (actualAPIStatus !== expectedAPIStatus) {
          console.error(`api.ts backend status ${String(backendStatus)} must map to ${expectedAPIStatus}, got ${String(actualAPIStatus)}`);
          failed = true;
        }
      }
    } catch (error) {
      console.error(`api.ts backend status mapper must be executable JavaScript after removing TypeScript annotations: ${error.message}`);
      failed = true;
    }
  }
  if (!api.includes('const apiStatus = mapBackendAPIStatus(backendStatus);')) {
    console.error('mapStatusBar must derive API state from the strict backend status mapper');
    failed = true;
  }
  const backendStatusTranslatorSource = namedDeclarationSource(apiSourceFile, 'translateBackendStatus');
  const unknownBackendValueLabelSource = namedDeclarationSource(apiSourceFile, 'unknownBackendValueLabel');
  if (!backendStatusTranslatorSource) {
    console.error('api.ts must translate backend-authored status values into understandable Chinese');
    failed = true;
  } else if (!unknownBackendValueLabelSource) {
    console.error('api.ts must preserve unknown backend status values behind Chinese copy');
    failed = true;
  } else {
    try {
      const executableTranslatorSource = backendStatusTranslatorSource
        .replace('status?: string', 'status')
        .replace('): string', ')')
        .replace(' as Record<string, string>', '');
      const executableUnknownValueSource = unknownBackendValueLabelSource
        .replace('fallback: string', 'fallback')
        .replace('value?: string', 'value')
        .replace('): string', ')');
      const translateBackendStatus = Function(
        'zhCN',
        `const unknownBackendValueLabel = ${executableUnknownValueSource}; return (${executableTranslatorSource});`,
      )({
        common: {
          unknownStatus: '未知状态',
          statuses: {
            configured: '已配置',
            ready: '就绪',
            healthy: '健康',
            degraded: '部分可用',
            invalid: '无效',
            disabled: '已禁用',
            incomplete: '未完成',
            update_available: '有可用更新',
            idle: '空闲',
            completed: '已完成',
            empty: '空',
            missing: '缺失',
            unknown: '未知状态',
            complete: '完成',
            running: '运行中',
            queued: '排队',
            failed: '失败',
            stopped: '已停止',
          },
        },
        backendStatus: {
          ready: '就绪',
          healthy: '健康',
          completed: '已完成',
          empty: '空',
          missing: '缺失',
          unknown: '未知',
        },
        executionStatus: {
          complete: '完成',
          running: '运行中',
          queued: '排队',
          failed: '失败',
          stopped: '已停止',
        },
      });
      const expectedChineseStatuses = [
        ['ready', '就绪'],
        ['configured', '已配置'],
        [' INVALID ', '无效'],
        ['disabled', '已禁用'],
        ['degraded', '部分可用'],
        ['idle', '空闲'],
        ['incomplete', '未完成'],
        ['update_available', '有可用更新'],
        ['missing', '缺失'],
        ['empty', '空'],
        ['unknown', '未知状态'],
        ['unexpected_backend_state', '未知状态（unexpected_backend_state）'],
        [undefined, '未知状态'],
      ];
      for (const [backendStatus, expectedChineseStatus] of expectedChineseStatuses) {
        const actualChineseStatus = translateBackendStatus(backendStatus);
        if (actualChineseStatus !== expectedChineseStatus) {
          console.error(`api.ts backend status ${String(backendStatus)} must translate to ${expectedChineseStatus}, got ${String(actualChineseStatus)}`);
          failed = true;
        }
      }
    } catch (error) {
      console.error(`api.ts backend status translator must be executable JavaScript after removing TypeScript annotations: ${error.message}`);
      failed = true;
    }
  }
  for (const snippet of ["case 'invalid':", "case 'disabled':", "case 'incomplete':", "case 'update_available':"]) {
    if (backendStatusTranslatorSource.includes(snippet)) {
      console.error(`api.ts backend status translator must use zhCN.common.statuses instead of hardcoded label branches: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of [
    'status: translateOptionalBackendStatus(snapshot.status)',
    'status: translateOptionalBackendStatus(summary.status)',
    'status: translateOptionalBackendStatus(pack.status)',
    'status: translateOptionalBackendStatus(action.status)',
  ]) {
    if (!api.includes(snippet)) {
      console.error(`api.ts must translate user-visible provider/toolchain snapshot status: ${snippet}`);
      failed = true;
    }
  }
  const providerTestMapperSource = namedDeclarationSource(apiSourceFile, 'mapProviderTestResult');
  if (!providerTestMapperSource.includes('status: translateBackendStatus(response.status)')) {
    console.error('api.ts provider test mapper must translate the backend status without fabricating success when status is missing');
    failed = true;
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
  const workspaceItemsMapperSource = namedDeclarationSource(apiSourceFile, 'mapWorkspaceItems');
  const fileKindTranslatorSource = namedDeclarationSource(apiSourceFile, 'translateFileKind');
  if (workspaceItemsMapperSource.includes('.slice(')) {
    console.error('api.ts must preserve every backend workspace file, including workspaces with more than 120 entries');
    failed = true;
  }
  if (workspaceItemsMapperSource.includes('!meta')) {
    console.error('api.ts must not discard real workspace files solely because their backend kind is new');
    failed = true;
  }
  if (!fileKindTranslatorSource.includes('unknownBackendValueLabel(zhCN.explorer.fileKinds.unknown, kind)')) {
    console.error('api.ts must preserve unknown workspace file kinds with Chinese copy and the real backend enum value');
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
  for (const snippet of ["id: 'evidence'", "id: 'runtime'", "id: 'reports'", "id: 'toolchain'"]) {
    if (api.includes(snippet)) {
      console.error(`api.ts must not synthesize workspace explorer nodes: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['fetchAIConversation', 'sendAIChatMessage', 'streamAIChatMessage', '/api/ai/conversation', '/api/ai/chat', '/api/ai/chat/stream', "method: 'POST'", 'AIConversation', 'AIChatResponse', 'AIChatRequest', 'AIChatStreamEvent', 'mapAIConversationResponse', 'mapAIChatResponse', 'mapRequiredAIMessage']) {
    if (!api.includes(snippet)) {
      console.error(`api.ts must expose validated AI chat API client: ${snippet}`);
      failed = true;
    }
  }
  const aiTypes = existsSync(join(root, 'src/app/types.ts')) ? readFileSync(join(root, 'src/app/types.ts'), 'utf8') : '';
  const aiRuntimeMapperSource = namedDeclarationSource(apiSourceFile, 'mapAIConversationRuntime');
  const aiTraceMapperSource = namedDeclarationSource(apiSourceFile, 'mapAIActionTraceItem');
  const aiReferenceMapperSource = namedDeclarationSource(apiSourceFile, 'mapAIContextReference');
  const looseAIReferencesMapperSource = namedDeclarationSource(apiSourceFile, 'mapLooseAIContextReferences');
  const aiConversationSearchMapperSource = namedDeclarationSource(apiSourceFile, 'mapAIConversationSearchResult');
  for (const snippet of ['ai-current-run', "?? 'unknown'", 'message_count, 0', 'action_count, 0', 'tool_result_count, 0', 'model_reply_count, 0', 'final_reply_count, 0', 'provider_call_count, 0']) {
    if (aiRuntimeMapperSource.includes(snippet)) {
      console.error(`api.ts must not fabricate incomplete AI conversation runtime DTO fields: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['!id', '!status', '!phase', 'messageCount === undefined', 'actionCount === undefined', 'toolResultCount === undefined', 'modelReplyCount === undefined', 'finalReplyCount === undefined', 'providerCallCount === undefined']) {
    if (aiRuntimeMapperSource.includes(snippet)) {
      console.error(`api.ts must preserve partial AI conversation runtime DTO facts instead of rejecting them: ${snippet}`);
      failed = true;
    }
  }
  if (!aiRuntimeMapperSource.includes('const mapped: AIConversationRuntime') || !aiRuntimeMapperSource.includes('Object.values(mapped).some')) {
    console.error('api.ts must return partial AI runtime DTOs only when the backend supplied at least one real fact');
    failed = true;
  }
  if (aiTraceMapperSource.includes('fallbackIndex') || aiTraceMapperSource.includes('action-trace-')) {
    console.error('api.ts must not fabricate AI action trace identities or indexes in the frontend');
    failed = true;
  }
  for (const snippet of ['!id', '!messageId', 'index === undefined', '!role', '!phase']) {
    if (aiTraceMapperSource.includes(snippet)) {
      console.error(`api.ts must preserve partial AI action trace DTO facts instead of rejecting them: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['reference-', "kind: cleanOptionalString(reference.kind) ?? 'reference'", 'zhCN.ai.referenceGeneric']) {
    if (aiReferenceMapperSource.includes(snippet) || looseAIReferencesMapperSource.includes(snippet)) {
      console.error(`api.ts must not fabricate AI context reference fields: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['!id', '!label', '!kind']) {
    if (aiReferenceMapperSource.includes(snippet) || looseAIReferencesMapperSource.includes(snippet)) {
      console.error(`api.ts must preserve AI context reference facts when another field is temporarily absent: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['id,', 'label,', 'kind,', 'title,', 'path,', 'detail,', 'summary,', 'source,', 'line,']) {
    if (!aiReferenceMapperSource.includes(snippet)) {
      console.error(`api.ts must preserve the complete backend AI context reference DTO: ${snippet}`);
      failed = true;
    }
  }
  if (!aiConversationSearchMapperSource.includes('id: messageId') || aiConversationSearchMapperSource.includes('`${sessionId}:')) {
    console.error('api.ts AI conversation search results must use the backend message ID without frontend identity concatenation');
    failed = true;
  }
  for (const snippet of [
    'contextReferences?: AIContextReference[]',
    'contextReferences: AIContextReference[] = []',
    'body: JSON.stringify({ message, contextReferences })',
    'actionResults?: AIActionRunResponse[]',
    'actionResults?: AIActionRunResponsePayload[]',
    'payload.actionResults.flatMap(result => mapAIActionRunResponseForStream(result, conversation))',
    'function mapAIActionRunResponseForStream(',
    'mapOptionalAIActionResponse(payload.action)',
    'mapOptionalAIActionResultResponse(payload.result)',
    'streamAIActionResultMessage(payload, result)',
    'const execution = mapExecutionResponse(payload.execution);',
  ]) {
    if (!api.includes(snippet) && !aiTypes.includes(snippet)) {
      console.error(`AI chat contract must carry IDE context and automatic action results: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['conversation?: AIConversationResponse', 'aiConversation: mapBootstrapAIConversation(response.conversation)', 'function mapBootstrapAIConversation']) {
    if (!api.includes(snippet)) {
      console.error(`api.ts must hydrate AI conversation from bootstrap state: ${snippet}`);
      failed = true;
    }
  }
  if (!aiTypes.includes('aiConversation: AIConversation | null')) {
    console.error('types.ts IDEData must carry bootstrap AI conversation state');
    failed = true;
  }
  const aiShell = existsSync(join(root, 'src/app/OuroborosIDE.tsx')) ? readFileSync(join(root, 'src/app/OuroborosIDE.tsx'), 'utf8') : '';
  for (const snippet of ['aiConversation: null', 'syncAIConversationFromIDEData(next)', 'setAIConversation(next.aiConversation)', 'function currentAIContextReferences', 'streamAIChatMessage(message, contextReferences', 'sendAIChatMessage(message, contextReferences)']) {
    if (!aiShell.includes(snippet)) {
      console.error(`OuroborosIDE.tsx must hydrate the AI panel from bootstrap conversation state: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['fetchAIActions', 'runAIAction', '/api/ai/actions', '/api/ai/actions/run', 'AIAction', 'AIActionRunRequest', 'AIActionRunResponse', 'AIActionResponse', 'AIActionRunResponsePayload', 'mapAIActionsResponse', 'mapAIActionRunResponse', 'mapRequiredAIActionResponse', 'mapRequiredExecutionResponse']) {
    if (!api.includes(snippet)) {
      console.error(`api.ts must expose validated real AI Actions API client: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['agentState?: AgentStateResponse', 'interface AgentStateResponse', 'interface AgentTaskResponse', 'GET /api/agents', 'POST /api/agents/tasks']) {
    if (!api.includes(snippet)) {
      console.error(`api.ts must document and consume Agent Orchestrator v1 API mapping: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['harness?: HarnessStateResponse', 'interface HarnessStateResponse', 'interface HarnessCaseResponse', 'interface HarnessResultResponse', 'interface HarnessCountsResponse', 'GET /api/harness', 'POST /api/harness/cases', 'POST /api/harness/cases/{id}/run']) {
    if (!api.includes(snippet)) {
      console.error(`api.ts must document and consume Harness Engineering v1 API mapping: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['export async function fetchHarnessState', "requestJSON<HarnessStateResponse>(baseURL, '/api/harness')", 'export async function createHarnessCase', '`${baseURL}/api/harness/cases`', 'export async function runHarnessCase', '`${baseURL}/api/harness/cases/${encodeURIComponent(id)}/run?wait=1&timeoutMs=5000`', 'function mapHarnessState', 'function mapHarnessCase', 'function mapHarnessResult', 'function mapHarnessCounts', 'promptVersionId?: string', 'executionId?: string', 'replayRunId?: string', 'providerSnapshot?: HarnessProviderSnapshotResponse', 'toolchainSnapshot?: HarnessToolchainSnapshotResponse', 'function mapHarnessProviderSnapshot', 'function mapHarnessToolchainSnapshot', 'providerSnapshot: mapHarnessProviderSnapshot(testCase.providerSnapshot)', 'toolchainSnapshot: mapHarnessToolchainSnapshot(testCase.toolchainSnapshot)', 'executionId: testCase.executionId', 'replayRunId: testCase.replayRunId', 'const harness = mapHarnessState(response.harness)', 'harness,']) {
    if (!api.includes(snippet)) {
      console.error(`api.ts must expose Harness API client and bootstrap mapper: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['mapAgentState(response.agentState)', 'function mapAgentState', 'function mapAgentTask', 'function mapAgentCounts', 'agentTasks?: AgentTaskResponse[]', 'agentTask?: AgentTaskResponse', 'mapAIActionAgentTasks', 'mapAIActionAgentTask', "case 'agent'", "return 'agent'", "case 'agent_tasks'", "return 'agent_tasks'", "case 'agent_task'", "return 'agent_task'"]) {
    if (!api.includes(snippet)) {
      console.error(`api.ts must consume Agent Orchestrator v1 bootstrap/API/action DTOs: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['EvidenceItem', 'interface EvidenceItemResponse', 'evidenceItem?: EvidenceItemResponse', 'evidenceItem: mapAIActionEvidenceItem(result.evidenceItem)', 'function mapAIActionEvidenceItem', "case 'evidence_item_file'", "return 'evidence_item_file'"]) {
    if (!api.includes(snippet)) {
      console.error(`api.ts must consume AI evidence ingestion action DTOs without frontend mocks: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ["case 'tool_result'", "return 'tool_result'", "case 'tool_result_parse'", "return 'tool_result_parse'", 'evidenceItems?: EvidenceItemResponse[]', 'evidenceItems: mapAIActionEvidenceItems(result.evidenceItems)', 'function mapAIActionEvidenceItems']) {
    if (!api.includes(snippet)) {
      console.error(`api.ts must consume parse execution result AI action DTOs without frontend mocks: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ["case 'eventlog'", "return 'eventlog'", "case 'eventlog_events'", "return 'eventlog_events'", 'eventlogEvents?: EventlogEventsResponse', 'eventlogEvents: mapAIActionEventlogEvents(result.eventlogEvents ?? result.eventlog_events)', 'function mapAIActionEventlogEvents']) {
    if (!api.includes(snippet)) {
      console.error(`api.ts must consume eventlog.query action DTOs from real ProjectEventlogEvents: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['title?: string', 'summary?: string', 'severity?: string', 'executionId?: string', 'artifactPath?: string', 'filePath?: string', 'source?: string', 'graphNodeId?: string', 'replayEventId?: string']) {
    if (!api.includes(snippet)) {
      console.error(`api.ts must expose AI evidence ingestion DTO field: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['fakeEvidence', 'mockEvidence', 'demoEvidence', 'sampleEvidence', '证据内容示例', '示例证据内容', '样例证据内容']) {
    if (api.includes(snippet)) {
      console.error(`api.ts must not mock AI evidence ingestion content in the frontend: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['mapHarnessState(response.harness)', 'harnessState?: HarnessStateResponse', 'harnessCases?: HarnessCaseResponse[]', 'harnessCase?: HarnessCaseResponse', 'harnessResults?: HarnessResultResponse[]', 'harnessResult?: HarnessResultResponse', 'snapshotId?: string', 'snapshotPath?: string', 'snapshotId: testCase.snapshotId', 'snapshotPath: testCase.snapshotPath', 'references?: AIContextReferenceResponse[]', 'references: mapLooseAIContextReferences(result.references)', 'filePath: result.filePath', 'agentTaskId: result.agentTaskId', 'blackboardItemId: result.blackboardItemId', 'aiMessageIds: mapStringList(result.aiMessageIds)', 'toolCallIds: mapStringList(result.toolCallIds)', 'fileDiffs: mapHarnessFileDiffs(result.fileDiffs)', 'eventlogReferences: mapHarnessEventlogReferences(result.eventlogReferences)', 'function mapHarnessFileDiffs', 'function mapHarnessEventlogReferences', 'mapAIActionHarnessState', 'mapAIActionHarnessCases', 'mapAIActionHarnessCase', 'mapAIActionHarnessResults', 'mapAIActionHarnessResult', 'promptVersionId: result.promptVersionId', "case 'harness'", "return 'harness'", "case 'harness_cases'", "return 'harness_cases'", "case 'harness_case'", "return 'harness_case'", "case 'harness_case_from_failure'", "return 'harness_case_from_failure'", "case 'harness_case_from_eventlog'", "return 'harness_case_from_eventlog'", "case 'harness_case_from_workspace_snapshot'", "return 'harness_case_from_workspace_snapshot'", "case 'harness_case_from_ai_conversation'", "return 'harness_case_from_ai_conversation'", "case 'harness_results'", "return 'harness_results'", "case 'harness_result'", "return 'harness_result'"]) {
    if (!api.includes(snippet)) {
      console.error(`api.ts must consume Harness Engineering v1 bootstrap/API/action DTOs: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ["case 'harness_case_from_environment_snapshot'", "return 'harness_case_from_environment_snapshot'"]) {
    if (!api.includes(snippet)) {
      console.error(`api.ts must preserve harness_case_from_environment_snapshot action result kind: ${snippet}`);
      failed = true;
    }
  }
  const harnessResultResponse = api.match(/interface HarnessResultResponse \{([\s\S]*?)\n\}/)?.[1] ?? '';
  for (const snippet of ['exitCode?: number', 'durationMs?: number', 'logBytes?: number', 'artifactCount?: number', 'artifactBytes?: number', 'aiMessageIds?: string[]', 'toolCallIds?: string[]', 'fileDiffs?: HarnessFileDiffResponse[]', 'eventlogReferences?: HarnessEventlogReferenceResponse[]']) {
    if (!harnessResultResponse.includes(snippet)) {
      console.error(`api.ts HarnessResultResponse must expose real execution metric field: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['interface HarnessFileDiffResponse', 'path?: string', 'operation?: string', 'summary?: string', 'diff?: string', 'actionId?: string', 'messageId?: string', 'toolCallId?: string', 'interface HarnessEventlogReferenceResponse', 'rawEventId?: string', 'replayEventId?: string', 'eventType?: string', 'lane?: string']) {
    if (!api.includes(snippet)) {
      console.error(`api.ts must expose Harness result trace DTO field: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['exitCode: isFiniteNumber(result.exitCode) ? result.exitCode : undefined', 'durationMs: isFiniteNumber(result.durationMs) ? result.durationMs : undefined', 'logBytes: isFiniteNumber(result.logBytes) ? result.logBytes : undefined', 'artifactCount: isFiniteNumber(result.artifactCount) ? result.artifactCount : undefined', 'artifactBytes: isFiniteNumber(result.artifactBytes) ? result.artifactBytes : undefined']) {
    if (!api.includes(snippet)) {
      console.error(`api.ts mapHarnessResult must map real execution metric from backend DTO: ${snippet}`);
      failed = true;
    }
  }
  if (!api.includes('/run?wait=1&timeoutMs=5000')) {
    console.error('api.ts runHarnessCase must request terminal harness execution metrics for the desktop Harness workbench');
    failed = true;
  }
  for (const snippet of ['orchestrator?: OrchestratorStateResponse', 'interface OrchestratorStateResponse', 'interface OrchestratorRunResponse', 'interface OrchestratorObservationResponse', 'interface OrchestratorAdvanceResponse', 'observationPath?: string', 'advancePath?: string', 'observations?: OrchestratorObservationResponse[]', 'advances?: OrchestratorAdvanceResponse[]', 'orchestratorState?: OrchestratorStateResponse', 'orchestratorRuns?: OrchestratorRunResponse[]', 'orchestratorRun?: OrchestratorRunResponse', 'orchestratorObservation?: OrchestratorObservationResponse', 'orchestratorObservations?: OrchestratorObservationResponse[]', 'orchestratorAdvance?: OrchestratorAdvanceResponse', 'orchestratorAdvances?: OrchestratorAdvanceResponse[]', 'function mapOrchestratorState', 'function mapOrchestratorRun', 'function mapOrchestratorObservation', 'function mapOrchestratorAdvance', 'mapAIActionOrchestratorState', 'mapAIActionOrchestratorRuns', 'mapAIActionOrchestratorRun', 'mapAIActionOrchestratorObservation', 'mapAIActionOrchestratorObservations', 'mapAIActionOrchestratorAdvance', 'mapAIActionOrchestratorAdvances', "case 'orchestrator'", "return 'orchestrator'", "case 'orchestrator_run'", "return 'orchestrator_run'", "case 'orchestrator_observation'", "return 'orchestrator_observation'", "case 'orchestrator_advance'", "return 'orchestrator_advance'"]) {
    if (!api.includes(snippet)) {
      console.error(`api.ts must consume Orchestrator AI action DTOs without fabricated state: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['autoParsedEvidenceItems?: EvidenceItemResponse[]', 'autoParsedReportFindings?: ReportFindingResponse[]', 'autoParsedEvidenceItemIds?: string[]', 'autoParsedReportFindingIds?: string[]', 'autoParsedEvidenceCount?: number', 'autoParsedFindingCount?: number', 'parsedSource?: string', 'parsedBatchTitle?: string', 'autoParsedEvidenceItems: mapAIActionEvidenceItems(observation.autoParsedEvidenceItems)', 'autoParsedReportFindings: mapAIActionReportFindings(observation.autoParsedReportFindings)', 'autoParsedEvidenceItemIds: mapStringList(observation.autoParsedEvidenceItemIds)', 'autoParsedReportFindingIds: mapStringList(observation.autoParsedReportFindingIds)', 'autoParsedEvidenceCount: isFiniteNumber(observation.autoParsedEvidenceCount) ? observation.autoParsedEvidenceCount : undefined', 'autoParsedFindingCount: isFiniteNumber(observation.autoParsedFindingCount) ? observation.autoParsedFindingCount : undefined', 'parsedSource: observation.parsedSource', 'parsedBatchTitle: observation.parsedBatchTitle']) {
    if (!api.includes(snippet)) {
      console.error(`api.ts must map backend-authored Orchestrator auto-parse evidence/finding DTO fields without log inference: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['planner?: PlannerStateResponse', 'interface PlannerStateResponse', 'plans?: PlannerPlanResponse[]', 'counts?: Record<string, unknown>', 'interface PlannerPlanResponse', 'nextActionTitle?: string', 'nextActionGoal?: string', 'nextActionCommand?: string', 'executionId?: string', 'executionStatus?: string', 'executedAt?: number', 'observedAt?: number', 'observationSummary?: string', 'logPath?: string', 'artifactDir?: string', 'artifactCount?: number', 'exitCode?: number', 'harnessResultId?: string', 'plannerState?: PlannerStateResponse', 'plannerPlans?: PlannerPlanResponse[]', 'plannerPlan?: PlannerPlanResponse', 'const plannerState = mapPlannerState(response.planner)', 'function mapPlannerState', 'function mapPlannerCounts', 'function mapPlannerPlan', 'mapAIActionPlannerState', 'mapAIActionPlannerPlans', 'mapAIActionPlannerPlan', "case 'planner'", "return 'planner'", "case 'planner_plan'", "return 'planner_plan'", "case 'planner_dispatch'", "return 'planner_dispatch'", "case 'planner_execute'", "return 'planner_execute'", "case 'planner_observation'", "return 'planner_observation'", "case 'planner_advance'", "return 'planner_advance'", "case 'planner_step'", "return 'planner_step'"]) {
    if (!api.includes(snippet)) {
      console.error(`api.ts must consume Planner Dispatch/Execute/Observe backend DTOs without stale dispatch/step types: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ["case 'frontier'", "return 'frontier'", "case 'frontier_priorities'", "return 'frontier_priorities'", "case 'frontier_execute_next'", "return 'frontier_execute_next'", "case 'frontier_step'", "return 'frontier_step'", "case 'frontier_loop'", "return 'frontier_loop'", "case 'frontier_autopilot'", "return 'frontier_autopilot'", 'interface FrontierPriorityItemResponse', 'interface FrontierPrioritySummaryResponse', 'frontierItems?: FrontierPriorityItemResponse[]', 'frontier_items?: FrontierPriorityItemResponse[]', 'frontierSummary?: FrontierPrioritySummaryResponse', 'frontier_summary?: FrontierPrioritySummaryResponse', 'refreshedFrontierItems?: FrontierPriorityItemResponse[]', 'refreshed_frontier_items?: FrontierPriorityItemResponse[]', 'refreshedFrontierSummary?: FrontierPrioritySummaryResponse', 'refreshed_frontier_summary?: FrontierPrioritySummaryResponse', 'queueSource?: string', 'queue_source?: string', 'queueStatus?: string', 'queue_status?: string', 'queueSource: cleanOptionalString(result.queueSource ?? result.queue_source)', 'queueStatus: cleanOptionalString(result.queueStatus ?? result.queue_status)', 'budget?: number', 'maxItems?: number', 'statusFilter?: string', 'priorityFilter?: string', 'mapAIActionFrontierItems(result.frontierItems ?? result.frontier_items)', 'mapAIActionFrontierSummary(result.frontierSummary ?? result.frontier_summary)', 'mapAIActionFrontierItems(result.refreshedFrontierItems ?? result.refreshed_frontier_items)', 'mapAIActionFrontierSummary(result.refreshedFrontierSummary ?? result.refreshed_frontier_summary)', 'function mapAIActionFrontierItems', 'function mapAIActionFrontierItem', 'function mapAIActionFrontierSummary', 'blackboardItemId: cleanOptionalString(item.blackboardItemId ?? item.blackboard_item_id)', 'agentTaskId: cleanOptionalString(item.agentTaskId ?? item.agent_task_id)', 'suggestedAction: cleanOptionalString(item.suggestedAction ?? item.suggested_action)', "ready: typeof item.ready === 'boolean' ? item.ready : undefined", 'dependencyIds: mapStringList(item.dependencyIds ?? item.dependency_ids)', 'blockedByIds: mapStringList(item.blockedByIds ?? item.blocked_by_ids)', 'blockedReason: cleanOptionalString(item.blockedReason ?? item.blocked_reason)', 'missionValue: finiteNumberOr(item.missionValue ?? item.mission_value)']) {
    if (!api.includes(snippet)) {
      console.error(`api.ts must preserve frontier.prioritize action/result DTO fields: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['loadFrontierQueue', 'FrontierQueue', 'FrontierQueueQuery', 'FrontierQueueResponse', '/api/frontier/queue', 'frontierQueue?: FrontierQueueResponse', 'frontier_queue?: FrontierQueueResponse', 'frontierQueue: payload.frontierQueue', 'appendFrontierQueueQuery', 'mapFrontierQueueResponse', 'blackboardItems?: BlackboardItemResponse[]', 'agentTasks?: AgentTaskResponse[]', 'agentState?: AgentStateResponse', 'candidateCount?: number', 'readyFilter?: string', 'readyCount?: number', 'blockedCount?: number', 'budgetUsed?: number', 'budgetRemaining?: number', 'openBlackboard?: number', 'queuedTasks?: number']) {
    if (!api.includes(snippet)) {
      console.error(`api.ts must consume persistent Frontier queue API DTOs: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['loadFrontierMonitor', 'FrontierMonitor', 'FrontierMonitorSummary', 'FrontierMonitorResponse', 'FrontierMonitorSummaryResponse', '/api/frontier/monitor', 'queue?: FrontierQueueResponse', 'summary?: FrontierMonitorSummaryResponse', 'activeAgentTasks?: AgentTaskResponse[]', 'activePlannerPlans?: PlannerPlanResponse[]', 'runningExecutions?: ExecutionResponse[]', 'recentAgentEvents?: AgentTaskEventResponse[]', 'mapFrontierMonitorResponse', 'mapFrontierMonitorSummaryResponse', 'mapExecutionSteps(response.runningExecutions ?? response.running_executions)', 'mapAgentState(response.agentState ?? response.agent_state)', 'mapPlannerState(response.plannerState ?? response.planner_state)']) {
    if (!api.includes(snippet)) {
      console.error(`api.ts must consume real Frontier monitor API DTOs: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['loadFrontierDependencies', 'FrontierDependencySelection', 'FrontierDependencySummary', 'FrontierDependencyCluster', 'FrontierDependencyCandidate', 'FrontierDependencySelectionResponse', 'FrontierDependencySummaryResponse', 'FrontierDependencyClusterResponse', 'FrontierDependencyCandidateResponse', '/api/frontier/dependencies', 'frontierDependencies?: FrontierDependencySelectionResponse', 'frontier_dependencies?: FrontierDependencySelectionResponse', 'mapFrontierDependencySelectionResponse', 'mapFrontierDependencySummaryResponse', 'mapFrontierDependencyClusterResponse', 'mapFrontierDependencyCandidateResponse', 'blockedItems?: FrontierPriorityItemResponse[]', 'blockedBlackboardItems?: BlackboardItemResponse[]', 'linkedAgentTasks?: AgentTaskResponse[]', 'suggestedActionId?: string', 'suggestedAgentTaskId?: string', 'suggestedBlackboardItemId?: string', 'unlockValue?: number']) {
    if (!api.includes(snippet)) {
      console.error(`api.ts must consume real Frontier dependency selection API DTOs: ${snippet}`);
      failed = true;
    }
  }
  const plannerStateResponseBlock = api.match(/interface PlannerStateResponse[\s\S]*?interface PlannerPlanResponse/)?.[0] || '';
  for (const snippet of ['path?: string', 'plans?: PlannerPlanResponse[]', 'counts?: Record<string, unknown>']) {
    if (!plannerStateResponseBlock.includes(snippet)) {
      console.error(`api.ts PlannerStateResponse must mirror backend path/plans/counts shape: ${snippet}`);
      failed = true;
    }
  }
  const plannerPlanResponseBlock = api.match(/interface PlannerPlanResponse[\s\S]*?interface OrchestratorStateResponse/)?.[0] || '';
  for (const snippet of ['id?: string', 'title?: string', 'goal?: string', 'status?: string', 'contextSummary?: string', 'nextActionTitle?: string', 'nextActionGoal?: string', 'nextActionCommand?: string', 'blackboardItemId?: string', 'agentTaskId?: string', 'orchestratorRunId?: string', 'harnessCaseId?: string', 'executionId?: string', 'executionStatus?: string', 'executedAt?: number', 'observedAt?: number', 'observationSummary?: string', 'logPath?: string', 'artifactDir?: string', 'artifactCount?: number', 'exitCode?: number', 'harnessResultId?: string', 'createdAt?: number', 'updatedAt?: number']) {
    if (!plannerPlanResponseBlock.includes(snippet)) {
      console.error(`api.ts PlannerPlanResponse must mirror backend PlannerPlan fields: ${snippet}`);
      failed = true;
    }
  }
  const plannerMapperBlock = ['mapPlannerState', 'mapPlannerCounts', 'mapPlannerPlan']
    .map(name => namedDeclarationSource(apiSourceFile, name))
    .join('\n');
  for (const snippet of ['response.plans.flatMap', 'counts: mapPlannerCounts(response.counts)', 'plan.nextActionTitle', 'plan.nextActionGoal', 'plan.nextActionCommand', 'plan.executionId', 'plan.executionStatus', 'plan.executedAt', 'plan.observedAt', 'plan.observationSummary', 'plan.logPath', 'plan.artifactDir', 'plan.artifactCount', 'plan.exitCode', 'plan.harnessResultId', 'nextActionTitle: plan.nextActionTitle', 'nextActionGoal: plan.nextActionGoal', 'nextActionCommand: plan.nextActionCommand', 'executionId: plan.executionId', 'executionStatus: plan.executionStatus']) {
    if (!plannerMapperBlock.includes(snippet)) {
      console.error(`api.ts must map backend PlannerState/PlannerPlan fields directly: ${snippet}`);
      failed = true;
    }
  }
  for (const pattern of [
    { label: 'Planner dispatches field', regex: /\bdispatches\s*\??:/ },
    { label: 'Planner step field', regex: /\bsteps\s*\??:\s*(Planner|PlannerPlan|PlannerDispatch)/ },
  ]) {
    if (pattern.regex.test(api)) {
      console.error(`api.ts must use backend PlannerState.plans and PlannerPlan.nextAction fields, not stale dispatch/step contract: ${pattern.label}`);
      failed = true;
    }
  }
  for (const snippet of ['goal?: string', 'role?: string', 'agentId?: string', 'blackboardItemId?: string', 'reportFindingId?: string']) {
    if (!api.includes(snippet)) {
      console.error(`api.ts must expose structured Agent task fields without frontend synthesis: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['AIActionResultResponse', 'result?: AIActionResultResponse', 'result: mapAIActionResultResponse(payload.result)', "case 'workspace_file'", "return 'file'", "return 'patch'", "return 'search_results'", "return 'deleted_file'", "case 'file_list'", "case 'file_contents'", "return 'file_contents'", 'files?: WorkspaceFileResponse[]', 'fileContents?: Array<{', 'fileChanges?: AIActionFileChangeResponse[]', 'fileDiffs?: HarnessFileDiffResponse[]', 'mapAIActionFiles(result.files)', 'fileContents: mapAIActionFileContents(result.fileContents)', 'fileChanges: mapAIActionFileChanges(result.fileChanges ?? result.file_changes)', 'fileDiffs: mapHarnessFileDiffs(result.fileDiffs ?? result.file_diffs)', 'function mapAIActionFileChanges', 'function mapAIActionFileContents', 'patch?: string']) {
    if (!api.includes(snippet)) {
      console.error(`api.ts must consume workspace file AI action DTOs and result payloads: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ["case 'goal':", "return 'goal'", 'goal?: string', 'goal: result.goal', 'maxItems?: number', 'max_items?: number']) {
    if (!api.includes(snippet)) {
      console.error(`api.ts must map backend-authored goal.decompose action payloads: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ["case 'report'", "return 'report'", 'reports?: ReportResponse[]', 'reportFindings?: ReportFindingResponse[]', 'reportFinding?: ReportFindingResponse', 'executions?: ExecutionResponse[]', 'executionLogs?: ExecutionLogResponse[]', 'executionArtifacts?: ExecutionArtifactResponse[]', 'reportExport?: ReportExportResponse', 'mapAIActionReportFinding', 'mapAIActionExecutions', 'mapAIActionExecutionLogs', 'mapAIActionExecutionArtifacts', 'mapAIActionReportExport', "case 'report_finding_file'", "return 'report_finding_file'"]) {
    if (!api.includes(snippet)) {
      console.error(`api.ts must consume AI report action DTOs without fabricated state: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of [
    'agentTaskId: cleanOptionalString(item.agentTaskId ?? item.agent_task_id)',
    'blackboardItemId: cleanOptionalString(item.blackboardItemId ?? item.blackboard_item_id)',
    'promptId: cleanOptionalString(item.promptId ?? item.prompt_id)',
    'promptVersionId: cleanOptionalString(item.promptVersionId ?? item.prompt_version_id)',
    'aiMessageIds: mapStringList(item.aiMessageIds ?? item.ai_message_ids) ?? []',
    'toolCallIds: mapStringList(item.toolCallIds ?? item.tool_call_ids) ?? []',
    'actionTraceIds: mapStringList(item.actionTraceIds ?? item.action_trace_ids) ?? []',
  ]) {
    if (!api.includes(snippet)) {
      console.error(`api.ts must preserve AI evidence provenance from backend DTOs into GUI models: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ["case 'blackboard'", "return 'blackboard'", 'blackboardItems?: BlackboardItemResponse[]', 'blackboardItem?: BlackboardItemResponse', "case 'blackboard_items'", "return 'blackboard_items'", "case 'blackboard_item'", "return 'blackboard_item'", 'mapAIActionBlackboardItems', 'mapAIActionBlackboardItem']) {
    if (!api.includes(snippet)) {
      console.error(`api.ts must consume Blackboard AI action DTOs without fabricated state: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['path?: string', 'content?: string', 'query?: string', 'targetPath?: string', 'kind?: string', 'title?: string', 'severity?: string', 'endpoint?: string', 'description?: string', 'recommendation?: string', 'evidence?: string', 'goal?: string', 'role?: string', 'agentId?: string', 'status?: string', 'priority?: string', 'source?: string', 'filePath?: string', 'executionId?: string', 'runId?: string', 'lane?: string', 'type?: string', 'limit?: number', 'aiMessageIds?: string[]', 'toolCallIds?: string[]', 'orchestratorRunId?: string', 'orchestratorObservationId?: string', 'graphNodeId?: string', 'replayEventId?: string', 'reportId?: string', 'blackboardItemId?: string', 'reportFindingId?: string']) {
    if (!api.includes(snippet)) {
      console.error(`api.ts must expose structured workspace file, Harness, Blackboard, and Agent AI action input fields: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['JSON.stringify(request)', 'actionId: request.actionId', 'if (request.actionId.trim() === \'\')', 'zhCN.aiActions.actionMissing', 'zhCN.aiActions.runFailed']) {
    if (!api.includes(snippet)) {
      console.error(`api.ts must validate outgoing AI Action run requests and real error states: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['fakeAIAction', 'mockAIAction', 'demoAIAction', 'fallbackAIAction', 'sampleAIAction', 'return [] as AIAction[]', 'return { action: mapAIAction({})']) {
    if (api.includes(snippet)) {
      console.error(`api.ts must not fabricate AI Actions or successful executions: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['JSON.stringify({ message, contextReferences })', 'if (message.trim() === \'\')', 'zhCN.ai.emptyMessage', 'zhCN.ai.providerMissing']) {
    if (!api.includes(snippet)) {
      console.error(`api.ts must validate outgoing AI chat messages and real error states: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ["role: 'assistant'", "provider: 'mock'", "provider: 'demo'", "model: 'demo'", "message: { role: 'assistant'", 'fakeAssistant', 'mockAssistant', 'demoConversation', 'fallbackAssistant']) {
    if (api.includes(snippet)) {
      console.error(`api.ts must not fabricate AI assistant/provider/message fallbacks: ${snippet}`);
      failed = true;
    }
  }
}

const typesPath = join(root, 'src/app/types.ts');
if (existsSync(typesPath)) {
  const types = readFileSync(typesPath, 'utf8');
  for (const snippet of ['export interface ToolchainDependency', 'dependencies: ToolchainDependency[]', 'manifestYaml: string']) {
    if (!types.includes(snippet)) {
      console.error(`types.ts must model real toolchain manifest DTO state: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['interface MissionMessage', "author: 'operator' | 'agent' | 'system'", 'messages: MissionMessage[]']) {
    if (types.includes(snippet)) {
      console.error(`types.ts must not expose dormant mission message types without backend DTO support: ${snippet}`);
      failed = true;
    }
  }
	  for (const snippet of ['export type AIMessageRole', 'export interface AIMessage', 'export interface AIContextReference', 'export interface AIUsage', 'export interface AIConversation', 'export interface AIChatRequest', 'export interface AIChatResponse', 'messages: AIMessage[]', 'contextReferences: AIContextReference[]']) {
	    if (!types.includes(snippet)) {
	      console.error(`types.ts must expose AI chat DTO/UI contract: ${snippet}`);
	      failed = true;
	    }
	  }
	  const aiActionTraceType = types.match(/export interface AIActionTraceItem \{([\s\S]*?)\n\}/)?.[1] ?? '';
	  for (const snippet of ['id?: string', 'messageId?: string', 'index?: number', 'role?: string', 'phase?: string']) {
	    if (!aiActionTraceType.includes(snippet)) {
	      console.error(`types.ts AIActionTraceItem must preserve a partial backend-authored trace field without frontend defaults: ${snippet}`);
	      failed = true;
	    }
	  }
	  const promptVersionType = types.match(/export interface PromptVersion \{([\s\S]*?)\n\}/)?.[1] ?? '';
	  for (const snippet of ['providerName?: string', 'model?: string']) {
	    if (!promptVersionType.includes(snippet)) {
	      console.error(`types.ts PromptVersion must expose provider/model binding: ${snippet}`);
	      failed = true;
	    }
	  }
	  const aiActionRunInputType = types.match(/export interface AIActionRunInput \{([\s\S]*?)\n\}/)?.[1] ?? '';
	  for (const snippet of ['providerName?: string', 'model?: string']) {
	    if (!aiActionRunInputType.includes(snippet)) {
	      console.error(`types.ts AIActionRunInput must allow prompt registry provider/model binding writes: ${snippet}`);
	      failed = true;
	    }
	  }
	  for (const snippet of ['aiMessageIds?: string[]', 'toolCallIds?: string[]', 'actionTraceIds?: string[]']) {
	    if (!aiActionRunInputType.includes(snippet)) {
	      console.error(`types.ts AIActionRunInput must allow harness.cases.from_ai_conversation to identify real AI conversation messages: ${snippet}`);
	      failed = true;
	    }
	  }
  const evidenceItemType = types.match(/export interface EvidenceItem \{([\s\S]*?)\n\}/)?.[1] ?? '';
  for (const snippet of ['agentTaskId?: string', 'blackboardItemId?: string', 'promptId?: string', 'promptVersionId?: string', 'aiMessageIds?: string[]', 'toolCallIds?: string[]', 'actionTraceIds?: string[]']) {
    if (!evidenceItemType.includes(snippet)) {
      console.error(`types.ts EvidenceItem must carry durable AI provenance fields: ${snippet}`);
      failed = true;
    }
  }
  const reportFindingType = types.match(/export interface ReportFinding \{([\s\S]*?)\n\}/)?.[1] ?? '';
  for (const snippet of ['agentTaskId?: string', 'blackboardItemId?: string', 'promptId?: string', 'promptVersionId?: string', 'aiMessageIds?: string[]', 'toolCallIds?: string[]', 'actionTraceIds?: string[]']) {
    if (!reportFindingType.includes(snippet)) {
      console.error(`types.ts ReportFinding must carry durable AI provenance fields: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['rawEventId?: number', 'payloadJson?: string']) {
    if (!types.includes(snippet)) {
      console.error(`types.ts must preserve raw eventlog inspector fields: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['export interface EventlogRun', 'export interface EventlogEvents', 'export interface EventlogQuery', 'runs: EventlogRun[]', 'events: ReplayEvent[]', 'runId?: string', 'lane?: string', 'type?: string', 'limit?: number']) {
    if (!types.includes(snippet)) {
      console.error(`types.ts must expose raw eventlog browser DTO contract: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['export interface DesktopRuntimeInfo', 'runtime?: DesktopRuntimeInfo', 'backendBinaryPath?: string', 'diagnosticLogPath?: string', 'lastExitCode?: number | null', 'lastStdout?: string', 'lastStderr?: string']) {
    if (!types.includes(snippet)) {
      console.error(`types.ts must expose desktop runtime diagnostics: ${snippet}`);
      failed = true;
    }
  }
  const desktopStatusType = types.match(/export interface DesktopStatus[\s\S]*?export interface DesktopRuntimeInfo/)?.[0] || '';
  for (const snippet of ['status:', 'apiBaseURL: string', 'workspace: string', 'running: boolean', 'lastError?: string', 'lastEvent?: string', 'recentWorkspaces?: string[]', 'runtime?: DesktopRuntimeInfo', 'windowState?:']) {
    if (!desktopStatusType.includes(snippet)) {
      console.error(`types.ts DesktopStatus must expose the runtime workbench status field: ${snippet}`);
      failed = true;
    }
  }
  const desktopRuntimeInfoType = types.match(/export interface DesktopRuntimeInfo[\s\S]*?export interface DesktopRuntimeIssue/)?.[0] || '';
  for (const snippet of ['isPackaged?: boolean', 'platform?: string', 'arch?: string', 'appVersion?: string', 'appPath?: string', 'executablePath?: string', 'resourcesPath?: string', 'userDataPath?: string', 'backendBinaryPath?: string', 'backendBinaryExists?: boolean', 'backendWorkingDirectory?: string', 'backendWorkingDirectoryExists?: boolean', 'rendererPath?: string', 'rendererExists?: boolean', 'preloadPath?: string', 'preloadExists?: boolean', 'diagnosticLogPath?: string', 'diagnosticLogExists?: boolean', 'desktopRuntimeStatePath?: string', 'desktopRuntimeSessionId?: string', 'desktopRuntimeStartedAt?: string', 'lastDesktopRuntimeIssue?: DesktopRuntimeIssue', 'processId?: number', 'lastCommandLine?: string', 'lastExitCode?: number | null', 'lastExitSignal?: string', 'lastStartedAt?: string', 'lastHealthError?: string', 'lastStdout?: string', 'lastStderr?: string', 'lastLog?: string', 'lastEvent?: string']) {
    if (!desktopRuntimeInfoType.includes(snippet)) {
      console.error(`types.ts DesktopRuntimeInfo must expose the runtime workbench diagnostic field: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['export type AIActionKind', 'export interface AIAction', 'export interface AIActionRunRequest', 'export interface AIActionRunResponse', "export type AIActionKind = 'command' | 'tool' | 'tool_result' | 'file' | 'evidence' | 'report' | 'blackboard' | 'agent' | 'harness' | 'strategy' | 'sisyphus' | 'frontier' | 'orchestrator'", 'execution?: ExecutionStep', 'export interface AIActionResult', 'result: AIActionResult', 'paths?: string[]', 'path?: string', 'content?: string', 'query?: string', 'targetPath?: string', 'files?: WorkspaceFile[]', 'fileContents?: WorkspaceFileContent[]', 'severity?: string', 'endpoint?: string', 'description?: string', 'recommendation?: string', 'evidence?: string', 'patch?: string', 'reports?: ReportDocument[]', 'reportFinding?: ReportFinding', 'reportFindings?: ReportFinding[]', 'executionLogs?: ExecutionLog[]', 'executionArtifacts?: ExecutionArtifact[]', 'reportExport?: ReportExportResult', 'failureKind?: string', 'failureMessage?: string', 'transcript?: string', 'attemptId?: string', 'intentId?: string']) {
    if (!types.includes(snippet)) {
      console.error(`types.ts must expose AI Actions DTO/UI contract: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of [expectedIDEModeDeclaration, "export type AIActionKind = 'command' | 'tool' | 'tool_result' | 'file' | 'evidence' | 'report' | 'blackboard' | 'agent' | 'harness' | 'strategy' | 'sisyphus' | 'frontier' | 'orchestrator'", 'export interface AgentState', 'export interface AgentTask', 'export type AgentCounts', 'agentState: AgentState', 'agentTasks?: AgentTask[]', 'agentTask?: AgentTask', 'blackboardItemId?: string', 'reportFindingId?: string']) {
    if (!types.includes(snippet)) {
      console.error(`types.ts must expose Agent Orchestrator v1 DTO/UI contract: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of [expectedIDEModeDeclaration, "export type AIActionKind = 'command' | 'tool' | 'tool_result' | 'file' | 'evidence' | 'report' | 'blackboard' | 'agent' | 'harness' | 'strategy' | 'sisyphus' | 'frontier' | 'orchestrator'", 'export interface HarnessState', 'export interface HarnessCase', 'export interface HarnessResult', 'export interface HarnessCounts', 'export interface HarnessCaseCreateRequest', 'export interface HarnessFileDiff', 'export interface HarnessEventlogReference', 'export interface HarnessProviderSnapshot', 'export interface HarnessToolchainSnapshot', 'providerSnapshot?: HarnessProviderSnapshot', 'toolchainSnapshot?: HarnessToolchainSnapshot', 'harness: HarnessState', 'harnessState?: HarnessState', 'harnessCases?: HarnessCase[]', 'harnessCase?: HarnessCase', 'harnessResults?: HarnessResult[]', 'harnessResult?: HarnessResult', 'references: AIContextReference[]', 'expectedResult?: string', 'promptId?: string', 'promptVersionId?: string', 'versionId?: string', 'agentRole?: string', 'agentTaskId?: string', 'runId?: string', 'replayRunId?: string', 'snapshotId?: string', 'snapshotPath?: string']) {
    if (!types.includes(snippet)) {
      console.error(`types.ts must expose Harness Engineering v1 DTO/UI contract: ${snippet}`);
      failed = true;
    }
  }
  const harnessResultType = types.match(/export interface HarnessResult \{([\s\S]*?)\n\}/)?.[1] ?? '';
  for (const snippet of ['exitCode?: number', 'durationMs?: number', 'logBytes?: number', 'artifactCount?: number', 'artifactBytes?: number', 'aiMessageIds: string[]', 'toolCallIds: string[]', 'fileDiffs: HarnessFileDiff[]', 'eventlogReferences: HarnessEventlogReference[]']) {
    if (!harnessResultType.includes(snippet)) {
      console.error(`types.ts HarnessResult must expose real execution metric/trace field: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of [expectedIDEModeDeclaration, "export type AIActionKind = 'command' | 'tool' | 'tool_result' | 'file' | 'evidence' | 'report' | 'blackboard' | 'agent' | 'harness' | 'strategy' | 'sisyphus' | 'frontier' | 'orchestrator'", 'export interface BlackboardState', 'export interface BlackboardItem', 'blackboardItems?: BlackboardItem[]', 'blackboardItem?: BlackboardItem', 'kind?: string', 'title?: string', 'status?: string', 'priority?: string', 'source?: string', 'filePath?: string', 'executionId?: string', 'graphNodeId?: string', 'replayEventId?: string', 'reportId?: string', 'blackboard: BlackboardState']) {
    if (!types.includes(snippet)) {
      console.error(`types.ts must expose Blackboard v1 DTO/UI contract: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['export interface PromptRegistry', 'export interface PromptDefinition', 'export interface PromptVersion', 'export interface PromptVariable', 'variables: PromptVariable[]', 'templateVariables?: Record<string, string>', 'export interface AIActionFileChange', 'fileChanges?: AIActionFileChange[]', 'fileDiffs?: HarnessFileDiff[]', 'export interface PromptDiff', 'promptDiff?: PromptDiff', 'activeVersion: string', 'versions: PromptVersion[]', 'baseVersionId: string', 'targetVersionId: string']) {
    if (!types.includes(snippet)) {
      console.error(`types.ts must expose Prompt Registry DTO/UI contract: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['export interface OrchestratorState', 'export interface OrchestratorRun', 'export interface OrchestratorObservation', 'export interface OrchestratorAdvance', 'export type OrchestratorCounts', 'observationPath: string', 'advancePath: string', 'observations: OrchestratorObservation[]', 'advances: OrchestratorAdvance[]', 'orchestratorRunId?: string', 'orchestratorObservationId?: string', 'orchestratorObservation?: OrchestratorObservation', 'orchestratorObservations?: OrchestratorObservation[]', 'orchestratorAdvance?: OrchestratorAdvance', 'orchestratorAdvances?: OrchestratorAdvance[]', 'orchestratorState?: OrchestratorState', 'orchestratorRuns?: OrchestratorRun[]', 'orchestratorRun?: OrchestratorRun', 'blackboardItemId?: string', 'agentTaskId?: string', 'harnessCaseId?: string']) {
    if (!types.includes(snippet)) {
      console.error(`types.ts must expose Orchestrator AI action DTO/UI contract: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['autoParsedEvidenceItems?: EvidenceItem[]', 'autoParsedReportFindings?: ReportFinding[]', 'autoParsedEvidenceItemIds?: string[]', 'autoParsedReportFindingIds?: string[]', 'autoParsedEvidenceCount?: number', 'autoParsedFindingCount?: number', 'parsedSource?: string', 'parsedBatchTitle?: string']) {
    if (!types.includes(snippet)) {
      console.error(`types.ts must expose Orchestrator auto-parse evidence/finding fields from backend DTOs: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ["export type AIActionKind = 'command' | 'tool' | 'tool_result' | 'file' | 'evidence' | 'report' | 'blackboard' | 'agent' | 'harness' | 'strategy' | 'sisyphus' | 'frontier' | 'orchestrator' | 'planner' | 'prompt' | 'goal' | 'eventlog';", 'export type PlannerCounts', 'export interface PlannerState', 'export interface PlannerPlan', 'plannerState: PlannerState', 'plannerState?: PlannerState', 'plannerPlans?: PlannerPlan[]', 'plannerPlan?: PlannerPlan', 'summary?: string', 'expectedResult?: string', 'priority?: string', 'source?: string', 'filePath?: string', 'plannerPlanId?: string', 'logPath?: string']) {
    if (!types.includes(snippet)) {
      console.error(`types.ts must expose Planner Dispatch/Execute/Observe DTO/UI contract: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ["export type AIActionKind = 'command' | 'tool' | 'tool_result' | 'file' | 'evidence' | 'report' | 'blackboard' | 'agent' | 'harness' | 'strategy' | 'sisyphus' | 'frontier' | 'orchestrator' | 'planner' | 'prompt' | 'goal' | 'eventlog';", 'eventlogEvents?: EventlogEvents', 'runId?: string', 'lane?: string', 'type?: string', 'limit?: number']) {
    if (!types.includes(snippet)) {
      console.error(`types.ts must expose eventlog.query AI action DTO/UI contract: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ["'goal'", 'goal?: string', 'title?: string', 'priority?: string', 'source?: string', 'maxItems?: number', 'blackboardItems?: BlackboardItem[]', 'agentTasks?: AgentTask[]', 'frontierItems?: FrontierPriorityItem[]', 'frontierSummary?: FrontierPrioritySummary']) {
    if (!types.includes(snippet)) {
      console.error(`types.ts must expose goal.decompose DTO/UI contract: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ["'frontier'", 'export interface FrontierPriorityItem', 'export interface FrontierPrioritySummary', 'frontierItems?: FrontierPriorityItem[]', 'frontierSummary?: FrontierPrioritySummary', 'refreshedFrontierItems?: FrontierPriorityItem[]', 'refreshedFrontierSummary?: FrontierPrioritySummary', 'queueSource?: string', 'queueStatus?: string', 'budget?: number', 'maxItems?: number', 'statusFilter?: string', 'priorityFilter?: string', 'blackboardItemId?: string', 'agentTaskId?: string', 'suggestedAction?: string', 'ready?: boolean', 'dependencyIds?: string[]', 'blockedByIds?: string[]', 'blockedReason?: string', 'missionValue?: number', 'unlockPotential?: number', 'evidenceConfidence?: number', 'metadata?: Record<string, unknown>']) {
    if (!types.includes(snippet)) {
      console.error(`types.ts must expose frontier.prioritize DTO/UI contract: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['export interface FrontierQueueQuery', 'export interface FrontierQueue', 'items: FrontierPriorityItem[]', 'summary?: FrontierPrioritySummary', 'blackboardItems?: BlackboardItem[]', 'agentTasks?: AgentTask[]', 'agentState?: AgentState', 'count: number', 'sourceFilter?: string', 'maxSteps?: number', 'candidateCount?: number', 'budgetUsed?: number', 'budgetRemaining?: number', 'openBlackboard?: number', 'queuedTasks?: number']) {
    if (!types.includes(snippet)) {
      console.error(`types.ts must expose persistent Frontier queue DTO/UI contract: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['export interface FrontierMonitorSummary', 'export interface FrontierMonitor', 'queue: FrontierQueue', 'summary: FrontierMonitorSummary', 'activeAgentTasks: AgentTask[]', 'activePlannerPlans: PlannerPlan[]', 'runningExecutions: ExecutionStep[]', 'recentAgentEvents: AgentTaskEvent[]', 'lastUpdatedAt?: number', 'readyItems: number', 'blockedItems: number']) {
    if (!types.includes(snippet)) {
      console.error(`types.ts must expose Frontier monitor DTO/UI contract: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['export interface FrontierDependencySummary', 'export interface FrontierDependencyCandidate', 'export interface FrontierDependencyCluster', 'export interface FrontierDependencySelection', 'frontierDependencies?: FrontierDependencySelection', 'clusters: FrontierDependencyCluster[]', 'candidates: FrontierDependencyCandidate[]', 'blockedItems: FrontierPriorityItem[]', 'blockedBlackboardItems: BlackboardItem[]', 'linkedAgentTasks: AgentTask[]', 'unlockValue: number', 'suggestedActionId?: string', 'suggestedBlackboardItemId?: string', 'suggestedAgentTaskId?: string', 'dependencyReady: boolean', 'missingDependency: boolean']) {
    if (!types.includes(snippet)) {
      console.error(`types.ts must expose Frontier dependency selection DTO/UI contract: ${snippet}`);
      failed = true;
    }
  }
  const plannerStateTypeBlock = types.match(/export interface PlannerState[\s\S]*?export interface PlannerPlan/)?.[0] || '';
  for (const snippet of ['path: string', 'plans: PlannerPlan[]', 'counts: PlannerCounts']) {
    if (!plannerStateTypeBlock.includes(snippet)) {
      console.error(`types.ts PlannerState must match backend path/plans/counts shape: ${snippet}`);
      failed = true;
    }
  }
  const plannerPlanTypeBlock = types.match(/export interface PlannerPlan[\s\S]*?export interface StatusSummary/)?.[0] || '';
  for (const snippet of ['id: string', 'title: string', 'goal: string', 'status: string', 'priority?: string', 'source?: string', 'filePath?: string', 'contextSummary?: string', 'nextActionTitle: string', 'nextActionGoal: string', 'nextActionCommand: string', 'blackboardItemId?: string', 'agentTaskId?: string', 'orchestratorRunId?: string', 'harnessCaseId?: string', 'executionId?: string', 'executionStatus?: string', 'executedAt?: number', 'observedAt?: number', 'observationSummary?: string', 'logPath?: string', 'artifactDir?: string', 'artifactCount?: number', 'exitCode?: number', 'harnessResultId?: string', 'createdAt: number', 'updatedAt: number']) {
    if (!plannerPlanTypeBlock.includes(snippet)) {
      console.error(`types.ts PlannerPlan must match backend PlannerPlan fields: ${snippet}`);
      failed = true;
    }
  }
  for (const pattern of [
    { label: 'PlannerDispatch stale type', regex: /\bPlannerDispatch\b/ },
    { label: 'PlannerPlanStep stale type', regex: /\bPlannerPlanStep\b/ },
    { label: 'PlannerState dispatches array', regex: /\bdispatches\s*\??:/ },
    { label: 'PlannerPlan steps array', regex: /\bsteps\s*\??:\s*(Planner|PlannerPlan|PlannerDispatch)/ },
  ]) {
    if (pattern.regex.test(types)) {
      console.error(`types.ts must use backend PlannerState.plans and PlannerPlan.nextAction fields, not stale dispatch/step contract: ${pattern.label}`);
      failed = true;
    }
  }
  for (const snippet of ['export interface EvidenceItem', 'evidenceItem?: EvidenceItem', 'artifactPath?: string', 'filePath?: string', 'graphNodeId?: string', 'replayEventId?: string']) {
    if (!types.includes(snippet)) {
      console.error(`types.ts must expose AI Evidence Ingestion v1 type contract: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ["export type AIActionKind = 'command' | 'tool' | 'tool_result' | 'file' | 'evidence' | 'report' | 'blackboard' | 'agent' | 'harness' | 'strategy' | 'sisyphus' | 'frontier' | 'orchestrator'", 'evidenceItems?: EvidenceItem[]', 'reportFindings?: ReportFinding[]', 'executionLogs?: ExecutionLog[]', 'executionArtifacts?: ExecutionArtifact[]', 'count?: number', 'summary?: string']) {
    if (!types.includes(snippet)) {
      console.error(`types.ts must expose parse execution result AI action DTO/UI contract: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['fakeEvidence', 'mockEvidence', 'demoEvidence', 'sampleEvidence']) {
    if (types.includes(snippet)) {
      console.error(`types.ts must not define mock AI evidence ingestion data: ${snippet}`);
      failed = true;
    }
  }
}

const zhCNPath = join(root, 'src/app/zh-cn.ts');
if (existsSync(zhCNPath)) {
  const zh = readFileSync(zhCNPath, 'utf8');
  const commonStatusBlock = zh.match(/common:\s*\{[\s\S]*?statuses:\s*\{([\s\S]*?)\},\s*priorities:/)?.[1] || '';
  const requiredCommonStatusLabels = {
    configured: '已配置',
    degraded: '部分可用',
    invalid: '无效',
    disabled: '已禁用',
    incomplete: '未完成',
    update_available: '有可用更新',
    missing: '缺失',
    empty: '空',
    unknown: '未知状态',
  };
  for (const [status, label] of Object.entries(requiredCommonStatusLabels)) {
    if (!commonStatusBlock.includes(`${status}: '${label}'`)) {
      console.error(`zh-cn.ts common.statuses must provide stable Chinese status mapping: ${status} -> ${label}`);
      failed = true;
    }
  }
  for (const snippet of ['frontierKind', 'frontierPrioritiesResult', 'frontierExecuteNextResult', 'frontierDependenciesResult', 'frontierStepResult', 'frontierLoopResult', 'frontierAutopilotResult', 'frontierQueueTitle', 'frontierSelectedItem', 'frontierRefreshedQueue', 'frontierRefreshedSummary', 'frontierGoalInput', 'frontierExecuteGoalPlaceholder', 'frontierStepGoalPlaceholder', 'frontierLoopGoalPlaceholder', 'frontierAutopilotGoalPlaceholder', 'frontierBudgetInput', 'frontierLimitInput', 'frontierMaxStepsInput', 'frontierSourceInput', 'frontierStatusFilterInput', 'frontierPriorityFilterInput', 'frontierPrioritizeRequired', 'frontierExecuteNextRequired', 'frontierStepRequired', 'frontierLoopRequired', 'frontierAutopilotRequired', 'frontierAgentTaskPlaceholder', 'frontierPlannerPlanPlaceholder', 'frontierExecutionPlaceholder', 'frontierCommandPlaceholder', 'frontierLoopSourcePlaceholder', 'frontierAutopilotSourcePlaceholder', 'frontierQueueSource', 'frontierQueueStatus', 'frontierTransitionCount', 'frontierTaskEvent', 'frontierPlannerPlan', 'frontierLoopPlans', 'frontierLoopEvents', 'frontierLoopLogs', 'frontierLoopArtifacts', 'frontierRank', 'frontierScore', 'frontierReason', 'frontierSuggestedAction', 'frontierReady', 'frontierDependencyIds', 'frontierBlockedByIds', 'frontierBlockedReason', 'noFrontierItems']) {
    if (!zh.includes(snippet)) {
      console.error(`zh-cn.ts must provide Chinese frontier.prioritize copy: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['goalKind', 'goalDecomposeResult', 'goalTotalGoal', 'goalTitleInput', 'goalSourceInput', 'goalPriorityInput', 'goalMaxItemsInput', 'goalDecomposeRequired', 'goalBlackboardItems', 'goalAgentTasks', 'goalFrontierQueue', 'goalFrontierSummary', 'goalNoBlackboardItems', 'goalNoAgentTasks', 'goalNoFrontierItems', 'goalFollowupExecuteNext', 'goalFollowupAutopilot', 'goalFollowupRunning', 'goalFollowupFailed']) {
    if (!zh.includes(snippet)) {
      console.error(`zh-cn.ts must provide Chinese goal.decompose copy: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['frontierQueueMeta', 'frontierQueueLoading', 'frontierQueueLoaded', 'frontierQueueLoadFailed', 'frontierQueueItemCount', 'frontierQueueActionFailed', 'frontierQueueDefaultGoal', 'frontierQueueRefresh', 'frontierQueueExecuteNext', 'frontierQueueAutopilot', 'frontierQueueExecuteRunning', 'frontierQueueAutopilotRunning', 'frontierQueueQueryGoal', 'frontierQueueQueryGoalPlaceholder', 'frontierQueueQueryStatus', 'frontierQueueQueryPriority', 'frontierQueueQuerySource', 'frontierQueueQueryKeyword', 'frontierQueueQueryMode', 'frontierQueueModeAll', 'frontierQueueModeReady', 'frontierQueueModeBlocked', 'frontierQueueQueryBudget', 'frontierQueueQueryMaxItems', 'frontierQueueEmpty', 'frontierQueueSummary', 'frontierQueuePath', 'frontierQueueAgentState', 'frontierQueueOpenBlackboard', 'frontierQueueQueuedTasks', 'frontierQueueBudgetUsed', 'frontierQueueBudgetRemaining', 'frontierQueueReadyCount', 'frontierQueueBlockedCount', 'frontierQueueDependencies', 'frontierQueueBlockedBy', 'frontierQueueOpenDependency', 'frontierQueueOpenBlocked', 'frontierQueueBlocked']) {
    if (!zh.includes(snippet)) {
      console.error(`zh-cn.ts must provide Chinese persistent Frontier queue copy: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['strategyPanel', 'strategyPanelMeta', 'strategySelect', 'strategySelecting', 'strategyNoState', 'strategyLatestSelection', 'strategyLatestTask', 'strategySelectorTasks', 'strategyMutatorTasks']) {
    if (!zh.includes(snippet)) {
      console.error(`zh-cn.ts must provide Chinese first-class AI strategy selector panel copy: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of [
    'collaborationTopology',
    'collaborationTopologyMeta',
    'collaborationTopologyNodes',
    'collaborationTopologyEdges',
    'collaborationTopologyBlackboardNode',
    'collaborationTopologyAgentNode',
    'collaborationTopologyFrontierNode',
    'collaborationTopologyBlackboardNodes',
    'collaborationTopologyAgentNodes',
    'collaborationTopologyFrontierNodes',
    'collaborationTopologyDependencyEdge',
    'collaborationTopologyDependentEdge',
    'collaborationTopologyAgentBlackboardEdge',
    'collaborationTopologyFrontierBlackboardEdge',
    'collaborationTopologyFrontierAgentEdge',
    'collaborationTopologyBlockedByEdge',
    'collaborationTopologyEmpty',
    'collaborationTopologyEdgesEmpty',
    'collaborationTopologyNodeList',
    'collaborationTopologyEdgeList',
    'collaborationTopologySourceNode',
    'collaborationTopologyTargetNode',
    'collaborationTopologyEdgeArrow',
    'collaborationTopologyHarnessSource',
    'collaborationTopologyHarnessReady',
    'collaborationTopologyHarnessMissing',
    'collaborationTopologyRefresh',
    'collaborationTopologySelectStrategy',
    'collaborationTopologyExecuteNext',
    'collaborationTopologySettleHarness',
    'collaborationTopologySettlingHarness',
  ]) {
    if (!zh.includes(snippet)) {
      console.error(`zh-cn.ts must provide Chinese AI Workbench collaboration topology copy: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['frontierDependencies', 'frontierDependenciesMeta', 'frontierDependenciesLoading', 'frontierDependenciesLoaded', 'frontierDependenciesLoadFailed', 'frontierDependenciesRefresh', 'frontierDependenciesMissing', 'frontierDependenciesDefaultGoal', 'frontierDependenciesExecute', 'frontierDependenciesExecuteRunning', 'frontierDependenciesActionFailed', 'frontierDependenciesSummary', 'frontierDependenciesSource', 'frontierDependenciesStatus', 'frontierDependenciesQueueStatus', 'frontierDependenciesQueueItems', 'frontierDependenciesBlockedItems', 'frontierDependenciesClusterCount', 'frontierDependenciesCandidateCount', 'frontierDependenciesBlockedFrontierItems', 'frontierDependenciesReadyCount', 'frontierDependenciesMissingCount', 'frontierDependenciesLinkedTasks', 'frontierDependenciesUnlockValue', 'frontierDependenciesEmpty', 'frontierDependenciesOpenDependency', 'frontierDependenciesOpenTask', 'frontierDependenciesSuggestedAction', 'frontierDependenciesSuggestedGoal', 'frontierDependenciesUnlocks', 'frontierDependenciesReady', 'frontierDependenciesMissingStatus', 'frontierDependenciesSatisfiedStatus', 'frontierDependenciesBlockedStatus']) {
    if (!zh.includes(snippet)) {
      console.error(`zh-cn.ts must provide Chinese Frontier dependency selection copy: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['loadReportMarkdown', 'saveReportMarkdown', 'reportMarkdownLoaded', 'reportMarkdownSaved', 'reportMarkdownLoadFailed', 'reportMarkdownSaveFailed', 'markdownSource', 'markdownEmpty', 'markdownEditorPlaceholder', 'selectReportFile']) {
    if (!zh.includes(snippet)) {
      console.error(`zh-cn.ts must provide Chinese report Markdown workbench copy: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['preview', 'previewEmpty', 'editSource', '报告预览', '当前报告没有可预览的内容', '编辑源码']) {
    if (!zh.includes(snippet)) {
      console.error(`zh-cn.ts must provide Chinese report Markdown preview copy: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of [
    'Report Preview',
    'Preview empty',
    'Empty preview',
    'Edit source',
    'Source editor',
    'Mock report preview',
    'Demo report preview',
    'Sample report preview',
    '示例报告预览',
    '样例报告预览',
    '模拟报告预览',
    '报告预览示例',
    '报告预览样例',
  ]) {
    if (zh.includes(snippet)) {
      console.error(`zh-cn.ts must not include mock/demo or English report preview copy: ${snippet}`);
      failed = true;
    }
  }
	  for (const key of ['copyPrompt', 'comparePromptVersions', 'runPromptHarness', 'promptProviderName', 'promptProviderModel']) {
	    const keyPattern = new RegExp(`\\b${key}\\s*:\\s*['"\`]`);
	    if (!keyPattern.test(zh)) {
	      console.error(`zh-cn.ts must provide Chinese Prompt Engineering settings action copy key: ${key}`);
      failed = true;
    }
  }
  for (const snippet of ['frontierMonitor', 'frontierMonitorMeta', 'frontierMonitorLoading', 'frontierMonitorLoaded', 'frontierMonitorLoadFailed', 'frontierMonitorRefresh', 'frontierMonitorMissing', 'frontierMonitorStatus', 'frontierMonitorSource', 'frontierMonitorQueueStatus', 'frontierMonitorQueueItems', 'frontierMonitorReadyItems', 'frontierMonitorBlockedItems', 'frontierMonitorActiveAgentTasks', 'frontierMonitorActivePlannerPlans', 'frontierMonitorRunningExecutions', 'frontierMonitorRecentEvents', 'frontierMonitorLastUpdatedAt', 'frontierMonitorTasks', 'frontierMonitorPlans', 'frontierMonitorExecutions', 'frontierMonitorEvents', 'frontierMonitorNoTasks', 'frontierMonitorNoPlans', 'frontierMonitorNoExecutions', 'frontierMonitorNoEvents', 'frontierMonitorRunning', 'frontierMonitorBlocked', 'frontierMonitorReady', 'frontierMonitorIdle', 'frontierMonitorUnknown']) {
    if (!zh.includes(snippet)) {
      console.error(`zh-cn.ts must provide Chinese Frontier monitor copy: ${snippet}`);
      failed = true;
    }
  }
}

const apiBackendPath = join(root, '..', 'internal/api/ai.go');
if (existsSync(apiBackendPath)) {
  const backendAI = readFileSync(apiBackendPath, 'utf8');
  for (const snippet of ['frontierQueueAfterAIAction', 's.BroadcastEvent("frontier:queue-updated"', 's.BroadcastEvent("ai:action-completed"', 'FrontierQueue: frontierQueue']) {
    if (!backendAI.includes(snippet)) {
      console.error(`internal/api/ai.go must broadcast and return real Frontier queue updates after AI actions: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['ID:           "frontier.dependencies"', 'runAIFrontierDependenciesAction', 'FrontierDependencies: &selection', 'renderFrontierDependencySelectionOutput', 'frontier_dependencies']) {
    if (!backendAI.includes(snippet)) {
      console.error(`internal/api/ai.go must expose real Frontier dependency AI action: ${snippet}`);
      failed = true;
    }
  }
}

const frontierBackendPath = join(root, '..', 'internal/api/frontier.go');
if (existsSync(frontierBackendPath)) {
  const backendFrontier = readFileSync(frontierBackendPath, 'utf8');
  const frontierProjectionUnavailable = backendFrontier.includes('frontierUnavailableResponse')
    && backendFrontier.includes('frontier_projection_unavailable');
  const monitorSnippets = frontierProjectionUnavailable
    ? ['handleFrontierMonitor', 'frontierUnavailableResponse', 'frontier_projection_unavailable']
    : ['handleFrontierMonitor', 'projectFrontierMonitorDTO', 'frontierMonitorActiveAgentTasks', 'frontierMonitorActivePlannerPlans', 'frontierMonitorRunningExecutions', 'frontierMonitorRecentAgentEvents', 'frontierMonitorSummary'];
  for (const snippet of monitorSnippets) {
    if (!backendFrontier.includes(snippet)) {
      console.error(`internal/api/frontier.go must project real Frontier monitor state: ${snippet}`);
      failed = true;
    }
  }
  const dependencySnippets = frontierProjectionUnavailable
    ? ['handleFrontierDependencies', 'frontierUnavailableResponse', 'frontier_projection_unavailable']
    : ['handleFrontierDependencies', 'projectFrontierDependenciesDTO', 'frontierDependencySelectionSummary', 'frontierDependencyClusterScore', 'hydrateFrontierDependencyCluster', 'frontierDependencyCandidateDTO', 'frontierDependencyBlockedItemKey'];
  for (const snippet of dependencySnippets) {
    if (!backendFrontier.includes(snippet)) {
      console.error(`internal/api/frontier.go must project real Frontier dependency selection state: ${snippet}`);
      failed = true;
    }
  }
}

const apiServerPath = join(root, '..', 'internal/api/server.go');
if (existsSync(apiServerPath)) {
  const backendServer = readFileSync(apiServerPath, 'utf8');
  if (!backendServer.includes('/api/frontier/monitor')) {
    console.error('internal/api/server.go must register GET /api/frontier/monitor');
    failed = true;
  }
  if (!backendServer.includes('/api/frontier/dependencies')) {
    console.error('internal/api/server.go must register GET /api/frontier/dependencies');
    failed = true;
  }
}

const stylePath = join(root, 'src/styles/ouroboros-ide.css');
if (existsSync(stylePath)) {
  const styles = readFileSync(stylePath, 'utf8');
  for (const snippet of ['.ai-frontier-monitor-panel', '.ai-frontier-monitor-toolbar', '.ai-frontier-monitor-status', '.ai-frontier-monitor-sections', '.ai-frontier-monitor-section', '.ai-frontier-monitor-actions']) {
    if (!styles.includes(snippet)) {
      console.error(`ouroboros-ide.css must style Frontier monitor panel: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['.ai-frontier-dependencies-panel', '.ai-frontier-dependencies-toolbar', '.ai-frontier-dependencies-status', '.ai-frontier-dependencies-list', '.ai-frontier-dependencies-head', '.ai-frontier-dependencies-actions', '.ai-frontier-dependencies-blocked']) {
    if (!styles.includes(snippet)) {
      console.error(`ouroboros-ide.css must style Frontier dependency selection panel: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['.eventlog-inspector', '.eventlog-inspector pre']) {
    if (!styles.includes(snippet)) {
      console.error(`ouroboros-ide.css must style raw eventlog inspector: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['.runtime-workbench', '.runtime-header', '.runtime-actions', '.runtime-summary', '.runtime-grid', '.runtime-panel', '.runtime-detail-grid', '.runtime-empty']) {
    if (!styles.includes(snippet)) {
      console.error(`ouroboros-ide.css must style the desktop runtime/package status workbench: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['.message.agent', '.message.system', '.message-label']) {
    if (styles.includes(snippet)) {
      console.error(`ouroboros-ide.css must not keep stale mission message styling without backend DTO support: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['.ai-workbench', '.ai-layout', '.ai-conversation-panel', '.ai-message-row', '.ai-composer', '.ai-context-list']) {
    if (!styles.includes(snippet)) {
      console.error(`ouroboros-ide.css must style the AI Agent workbench shell: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['ouroboros-mark.webp', '.ai-workbench-state', '.ai-composer-surface', '.ai-composer-footer', '.ai-message-references', '.ai-tool-message-summary', '.ai-tool-message-fields', '.ai-tool-message-details', '.graph-canvas.compact > .graph-node:only-of-type']) {
    if (!styles.includes(snippet)) {
      console.error(`ouroboros-ide.css must style the design-aligned empty brand and structured tool result: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['.ai-action-panel', '.ai-action-row', '.ai-action-form', '.ai-action-result']) {
    if (!styles.includes(snippet)) {
      console.error(`ouroboros-ide.css must style the AI Actions panel: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['.ai-frontier-queue-panel', '.ai-frontier-queue-form', '.ai-frontier-queue-mode', '.ai-frontier-queue-toolbar', '.ai-frontier-queue-list', '.ai-frontier-queue-row', '.ai-frontier-queue-head', '.ai-frontier-queue-actions', '.ai-frontier-queue-dependencies']) {
    if (!styles.includes(snippet)) {
      console.error(`ouroboros-ide.css must style the persistent Frontier queue panel: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['.agent-workbench', '.agent-grid', '.agent-task-list', '.agent-task-card', '.agent-count-grid', '.ai-action-agent-fields', '.ai-action-agent-result', '.ai-action-agent-list']) {
    if (!styles.includes(snippet)) {
      console.error(`ouroboros-ide.css must style the Agent Orchestrator v1 workspace and action result: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['.harness-workbench', '.harness-grid', '.harness-case-list', '.harness-result-list', '.harness-case-card', '.harness-result-card', '.harness-count-grid', '.harness-create-form', '.ai-action-harness-fields', '.ai-action-harness-result', '.ai-action-harness-list']) {
    if (!styles.includes(snippet)) {
      console.error(`ouroboros-ide.css must style the Harness Engineering v1 workspace and action result: ${snippet}`);
      failed = true;
    }
  }
}

const desktopGUISmokePath = join(root, '..', 'scripts/smoke_desktop_gui.sh');
if (existsSync(desktopGUISmokePath)) {
  const smoke = readFileSync(desktopGUISmokePath, 'utf8');
  const enforceExtendedSmokeContract = smoke.includes('OUROBOROS_GUI_SMOKE_CONTRACT=2');
  if (enforceExtendedSmokeContract) for (const snippet of [
    'async function editOpenWorkspaceFile(path, content)',
    'async function verifyWorkspaceFileContent(path, needle)',
    'async function createWorkspaceFileThroughGUI(path, content)',
    'async function fillMissionLaunch(name, goal, command)',
    'async function typeMissionLaunchField(labelText, value)',
    'async function clickMissionLaunchAction(label)',
    'async function waitForMissionLaunchValue(needle)',
    'async function waitForExecutionByCommand(commandNeedle)',
    'async function clickExecutionAction(commandNeedle, label)',
    "await clickWorkspaceTreeFile('mission.md');",
    "const createdWorkspaceFilePath = 'notes/gui-created.md';",
    'await createWorkspaceFileThroughGUI(createdWorkspaceFilePath, createdWorkspaceFileContent);',
    'await waitForWorkspaceFile(createdWorkspaceFilePath);',
    "await waitForEditorContent(createdWorkspaceFilePath, '通过中文界面新建的真实工作区文件');",
    "await verifyWorkspaceFileContent(createdWorkspaceFilePath, '通过中文界面新建的真实工作区文件');",
    'async function renameWorkspaceFileThroughGUI(path, targetPath)',
    "const renamedWorkspaceFilePath = 'notes/gui-renamed.md';",
    'await renameWorkspaceFileThroughGUI(createdWorkspaceFilePath, renamedWorkspaceFilePath);',
    'await verifyWorkspaceFileMoved(createdWorkspaceFilePath, renamedWorkspaceFilePath, \'通过中文界面新建的真实工作区文件\');',
    'async function deleteWorkspaceFileThroughGUI(path)',
    'await deleteWorkspaceFileThroughGUI(renamedWorkspaceFilePath);',
    'await verifyWorkspaceFileDeleted(renamedWorkspaceFilePath);',
    '[ ! -e "$WORKSPACE/notes/gui-created.md" ] || fail "renamed workspace file left old path on disk"',
    '[ ! -e "$WORKSPACE/notes/gui-renamed.md" ] || fail "deleted workspace file remained on disk"',
    "await editOpenWorkspaceFile('mission.md', editedMissionContent);",
    'async function runWorkspaceSearch(query)',
    'async function clickWorkspaceSearchResult(path)',
    'await runWorkspaceSearch(\'关键字\');',
    "await clickWorkspaceSearchResult('notes/search-target.md');",
    "await waitForEditorContent('notes/search-target.md', '真实搜索关键字');",
    "await fillMissionLaunch('GUI 表单真实任务'",
    "await clickMissionLaunchAction('启动任务');",
    "await clickExecutionAction('GUI 表单任务日志', '日志');",
    'assert_contains "$WORKSPACE/mission.md" "通过界面保存的真实工作区文件"',
    'assert_contains "$WORKSPACE/mission.yaml" "GUI 表单真实任务"',
  ]) {
    if (!smoke.includes(snippet)) {
      console.error(`smoke_desktop_gui.sh must verify real workspace file editing persistence: ${snippet}`);
      failed = true;
    }
  }
  if (enforceExtendedSmokeContract) for (const snippet of [
    'BLACKBOARD_ITEMS="$WORKSPACE/runtime/blackboard/items.jsonl"',
    "BLACKBOARD_SMOKE_TITLE='GUI 黑板真实条目'",
    'async function runAIBlackboardActionsDraft()',
    'async function waitForBlackboardItemsFile(path, needles)',
    "await runAIBlackboardAction('blackboard.write'",
    "await runAIBlackboardAction('blackboard.read')",
    "await clickMode('黑板')",
    "await waitForText('共享黑板')",
    'await waitForText(blackboardSmokeContent)',
    'test -f "$BLACKBOARD_ITEMS" || fail "Blackboard items JSONL was not written"',
    'assert_contains "$BLACKBOARD_ITEMS" "$BLACKBOARD_SMOKE_TITLE"',
    "'AI 黑板动作'",
    "'blackboard.write'",
    "'blackboard.read'",
    'assert_contains "$DOM_OUT" "黑板"',
    'assert_contains "$DOM_OUT" "$BLACKBOARD_SMOKE_CONTENT"',
  ]) {
    if (!smoke.includes(snippet)) {
      console.error(`smoke_desktop_gui.sh must verify real Blackboard v1 AI action persistence and Chinese UI: ${snippet}`);
      failed = true;
    }
  }
  if (enforceExtendedSmokeContract) for (const snippet of [
    'ORCHESTRATOR_RUNS="$WORKSPACE/runtime/orchestrator/runs.jsonl"',
    'ORCHESTRATOR_OBSERVATIONS="$WORKSPACE/runtime/orchestrator/observations.jsonl"',
    'ORCHESTRATOR_ADVANCES="$WORKSPACE/runtime/orchestrator/advances.jsonl"',
    "actionId: 'orchestrator.run'",
    "actionId: 'orchestrator.observe'",
    "actionId: 'orchestrator.advance'",
    "'AI 编排观察'",
    "'AI 编排推进'",
    'orchestratorObservationsPath',
    'orchestratorAdvancesPath',
    'orchestrator_observation',
    'orchestrator_advance',
    'test -f "$ORCHESTRATOR_OBSERVATIONS" || fail "AI orchestrator observations JSONL was not written"',
    'test -f "$ORCHESTRATOR_ADVANCES" || fail "AI orchestrator advances JSONL was not written"',
    'assert_contains "$ORCHESTRATOR_OBSERVATIONS" "GUI AI orchestrator run ok"',
    'assert_contains "$ORCHESTRATOR_ADVANCES" "GUI AI orchestrator advance ok"',
    'assert_contains "$HARNESS_RESULTS" "$ORCHESTRATOR_AI_TITLE"',
  ]) {
    if (!smoke.includes(snippet)) {
      console.error(`smoke_desktop_gui.sh must verify real Orchestrator observe result ingestion: ${snippet}`);
      failed = true;
    }
  }
}

const smokeDesktopGUI = existsSync(desktopGUISmokePath) ? readFileSync(desktopGUISmokePath, 'utf8') : '';
const desktopPackageRuntimeSmokePath = join(root, '..', 'scripts/smoke_desktop_package_runtime.sh');
const smokeDesktopPackageRuntime = existsSync(desktopPackageRuntimeSmokePath) ? readFileSync(desktopPackageRuntimeSmokePath, 'utf8') : '';

const enforceEvidenceSmokeContract = smokeDesktopGUI.includes('OUROBOROS_GUI_SMOKE_CONTRACT=2')
  || smokeDesktopPackageRuntime.includes('OUROBOROS_GUI_SMOKE_CONTRACT=2');
if (enforceEvidenceSmokeContract) for (const [sourceName, source] of [
  ['smoke_desktop_gui.sh', smokeDesktopGUI],
  ['smoke_desktop_package_runtime.sh', smokeDesktopPackageRuntime],
]) {
  for (const snippet of [
    'node - "$WORKSPACE" "$WORKSPACE/providers.yaml" "$WORKSPACE/mission.yaml" "$HARNESS_RESULTS" "$AI_EVIDENCE_REPORT_FILE" "$AI_CONVERSATION" "$PROVIDER_LOG" "$DOM_OUT"',
    'function parseJSONL(path, label) {',
    'assertYAMLSource(providersPath, providersRaw, [',
    'assertMarkdownSource(reportPath, reportRaw, [',
    'const harnessRecords = parseJSONL(harnessResultsPath, \'harness results\');',
    'const conversationRecords = parseJSONL(conversationPath, \'AI conversation\');',
    'const providerRecords = parseJSONL(providerLogPath, \'provider request log\');',
    'assertRealReferences(conversationRecords, domRaw, workspacePath);',
    'assertEvidenceDirectory(path.join(workspacePath, \'evidence\'), evidenceNeedles, \'evidence\');',
    'assertEvidenceDirectory(path.join(workspacePath, \'findings\'), evidenceNeedles, \'findings\');',
  ]) {
    if (!source.includes(snippet)) {
      console.error(`${sourceName} must verify evidence/report/tool/harness output against real workspace JSONL, Markdown, YAML, and references: ${snippet}`);
      failed = true;
    }
  }
}

const copyPath = join(root, 'src/app/zh-cn.ts');
if (existsSync(copyPath)) {
  const copy = readFileSync(copyPath, 'utf8');
  const shellForCopyChecks = existsSync(shellPath) ? readFileSync(shellPath, 'utf8') : '';
  for (const snippet of ['Demo', 'Mock', 'Preview', 'Web UI', 'Start Pentest', 'Command Palette', 'AI Conversation', 'Tool Execution', 'toolchain manifest', 'Manifest 路径']) {
    const escaped = snippet.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
    const quotedVisibleCopy = new RegExp(`["'\`]${escaped}["'\`]`);
    if (quotedVisibleCopy.test(copy)) {
      console.error(`zh-cn.ts contains old Web UI/demo/English visible copy: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['AI Agent', 'Agent Orchestrator', 'harness fixture', 'transcript', 'partial transcript', 'prompt.harness', 'quick runtime', '例如 npm test', 'workspace.name=真实工作区', 'custom.target=评测目标']) {
    if (copy.includes(snippet)) {
      console.error(`zh-cn.ts contains mixed English visible copy: ${snippet}`);
      failed = true;
    }
  }
  if (copy.includes('生成预览')) {
    console.error('zh-cn.ts still contains placeholder preview copy');
    failed = true;
  }
  if (copy.includes('commandFallback')) {
    console.error('zh-cn.ts must not keep unused execution command fallback copy after backend-authored execution labels');
    failed = true;
  }
  if (copy.includes('AI 对话')) {
    console.error('zh-cn.ts must not label the mission panel as AI chat without backend-projected chat messages');
    failed = true;
  }
  if (copy.includes('// contract compatibility')) {
    console.error('zh-cn.ts must not retain stale visible-copy assertions in compatibility comments');
    failed = true;
  }
  for (const snippet of ["agent: '智能体'"]) {
    if (copy.includes(snippet)) {
      console.error(`zh-cn.ts must not expose stale agent/next-step copy without backend DTO support: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['智能体图谱', '当前没有智能体任务或编排运行', '编排器']) {
    if (!copy.includes(snippet)) {
      console.error(`zh-cn.ts missing real agent graph copy: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of [
    "noGraph: '暂无攻击图谱数据'",
    "noSelectedGraphNode: '尚未选择图谱节点'",
    "refresh: '刷新'",
    "collaborationContextEmpty: '当前黑板、智能体、编排与规划状态没有可显示的协作数据'",
    "collaborationReferencesEmpty: '当前协作状态没有可定位的文件、执行或事件引用'",
    "collaborationTopologyEmpty: '当前黑板、智能体和前沿队列没有可绘制的拓扑节点'",
    "collaborationTopologyEdgesEmpty: '当前拓扑节点之间没有依赖关系'",
    "collaborationTopologyHarnessMissing: '当前 AI 会话没有可沉淀的消息'",
    "agentTaskQueueEmpty: '当前智能体状态中没有任务'",
    "agentTaskEventsEmpty: '当前智能体状态中没有任务事件'",
    "noEvents: '当前任务没有可显示的事件'",
    "rawEventlogEmpty: '当前筛选条件下没有事件日志原始记录'",
    "previewEmpty: '当前报告没有可预览的内容'",
    "empty: '当前选择没有可显示的详情'",
    "actionDesktopProcessGone: '查看桌面运行状态文件、诊断日志和系统内存记录；释放内存后重新打开应用并运行快速运行时检查。'",
  ]) {
    if (!copy.includes(snippet)) {
      console.error(`zh-cn.ts missing specific Chinese empty-state, tooltip, or diagnostic copy: ${snippet}`);
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
  for (const snippet of ['事件日志检查器', '原始事件载荷', '事件类型', '事件泳道', '原始事件 ID']) {
    if (!copy.includes(snippet)) {
      console.error(`zh-cn.ts missing eventlog inspector copy: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['原始事件记录', '真实 SQLite 事件日志记录', '筛选事件日志', '重置筛选', '全部运行', '全部泳道', '全部类型', '读取事件日志失败', '读取事件日志中', '筛选后记录', '原始记录']) {
    if (!copy.includes(snippet)) {
      console.error(`zh-cn.ts missing raw eventlog browser copy: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['环境快照评测', '从环境快照生成评测用例', '环境快照 ID', '输入环境快照 ID', '输入环境快照评测标题', '输入要固定为评测夹具的桌面环境目标', '输入环境恢复和参与评测的期望条件', '输入来源；留空使用桌面环境快照', '从环境快照生成评测用例需要环境快照 ID']) {
    if (!copy.includes(snippet)) {
      console.error(`zh-cn.ts missing environment snapshot Harness copy: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['输入要在隔离快照夹具中实际执行的命令', '从工作区快照生成评测用例需要在隔离快照夹具中真实执行的命令']) {
    if (!copy.includes(snippet)) {
      console.error(`zh-cn.ts missing real workspace-snapshot Harness command copy: ${snippet}`);
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
  for (const snippet of ['正在保存文件', '文件保存失败', '新建文件', '文件路径', '文件内容', '正在创建文件', '文件已创建', '文件创建失败', '重命名/移动文件', '目标路径', '正在重命名文件', '文件已重命名', '文件重命名失败', '删除文件', '正在删除文件', '文件已删除', '文件删除失败']) {
    if (!copy.includes(snippet)) {
      console.error(`zh-cn.ts missing workspace file save copy: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['保存模型服务', '模型服务已保存', '删除模型服务', '测试连接', '模型服务连通性结果', 'API 密钥环境变量', '上下文窗口']) {
    if (!copy.includes(snippet)) {
      console.error(`zh-cn.ts missing editable provider settings copy: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['保存工具链', '工具链已保存', '新增工具链包', '包 ID', '运行器', '解析器', '健康检查', '清单内容', '重新读取工具链', '清单保存到', '健康与清单证据', '依赖 / 安装证据', '清单动作', '运行动作', '查看执行', '清单动作已启动']) {
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
  for (const snippet of ['桌面后端已崩溃', '正在启动后端', '重启后端', '最近工作区', '窗口状态已保存', '桌面运行时', '后端伴随进程', '诊断日志', '最近标准输出', '最近错误输出']) {
    if (!copy.includes(snippet)) {
      console.error(`zh-cn.ts missing desktop lifecycle copy: ${snippet}`);
      failed = true;
    }
  }
  if (!/modes:\s*\{[\s\S]*?runtime:\s*['"`][^'"`]*运行时[^'"`]*['"`]/m.test(copy)) {
    console.error('zh-cn.ts must expose a Chinese runtime mode label under zhCN.modes.runtime');
    failed = true;
  }
  for (const snippet of ['runtime:', '桌面运行时', '打包状态', '诊断日志', '后端进程', '刷新状态', '重启后端', '运行总览', '路径证据', '桌面事件']) {
    if (!copy.includes(snippet)) {
      console.error(`zh-cn.ts missing first-class desktop runtime workbench copy: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['后端健康检查失败', '后端状态未知', '后端启动或运行失败', '错误详情', '健康检查错误', '诊断日志', '后端程序', '后端工作目录', '最近启动命令', '退出信息', '未记录退出码']) {
    if (!copy.includes(snippet)) {
      console.error(`zh-cn.ts missing precise desktop backend diagnostic copy: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['工具启动', '来自工具链清单的真实可执行动作', '运行工具', '工具参数', '暂无可直接运行的工具动作']) {
    if (!copy.includes(snippet)) {
      console.error(`zh-cn.ts missing manifest-backed tool launch copy: ${snippet}`);
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
  for (const snippet of ['当前软件状态', '工作区配置', '模型服务配置', '工具链配置', '提示词工程', '提示词注册表', '提示词内容', '保存提示词', '系统提示词', '提示词']) {
    if (!copy.includes(snippet)) {
      console.error(`zh-cn.ts missing settings section copy: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['Prompt 工程', 'Prompt Registry', 'Prompt 内容', '保存 Prompt']) {
    if (copy.includes(snippet)) {
      console.error(`zh-cn.ts must not expose mixed English Prompt settings copy: ${snippet}`);
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
  for (const snippet of ['ai:', 'AI 工作台', '正在连接 AI 会话', '发送消息', '请输入要发送给 AI 的内容', '缺少模型服务配置', 'AI 回复失败', '模型', '令牌用量', '上下文引用', '暂无上下文引用', '后端未返回 AI 会话']) {
    if (!copy.includes(snippet)) {
      console.error(`zh-cn.ts missing Chinese AI workbench copy: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['fileChangeSet:', 'AI 文件修改集', '来自真实动作结果、流式事件和持久化轨迹', '当前 AI 会话还没有真实文件修改记录', '文件操作', '来源', '目标路径', '打开文件', '打开目标文件', '写入文件', '应用补丁', '重命名文件', '删除文件', '会话轨迹']) {
    if (!copy.includes(snippet)) {
      console.error(`zh-cn.ts missing Chinese AI file change set copy: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['Token 用量', 'Token / 成本', '输入 Token', '输出 Token', '总 Token', '最大 Token', '累计 Token', 'Prompt Token', 'Completion Token']) {
    if (copy.includes(snippet)) {
      console.error(`zh-cn.ts must not expose mixed English token usage copy: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ["tokenUnit: '令牌'"]) {
    if (!copy.includes(snippet)) {
      console.error(`zh-cn.ts must centralize the visible token unit in Chinese: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ["`${formatNumber(runtime.totalTokens)} ${zhCN.ai.tokenUnit}`", "`${usage.totalTokens} ${zhCN.ai.tokenUnit}`"]) {
    if (!shellForCopyChecks.includes(snippet)) {
      console.error(`OuroborosIDE.tsx must render AI token totals with Chinese copy instead of hardcoded English units: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ["`${formatNumber(runtime.totalTokens)} Token`", "`${usage.totalTokens} Token`"]) {
    if (shellForCopyChecks.includes(snippet)) {
      console.error(`OuroborosIDE.tsx must not expose hardcoded English token units: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['aiActions:', 'AI 动作', '正在加载 AI 动作', '执行动作', '命令输入', '参数输入', '执行 ID', '执行状态', 'AI 动作执行失败', '后端未返回 AI 动作', '文件动作', '路径', '内容', '查询', '新路径', '结果状态', '搜索结果', '补丁差异']) {
    if (!copy.includes(snippet)) {
      console.error(`zh-cn.ts missing Chinese AI Actions copy: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ["pathsInput: '路径列表'", 'pathsPlaceholder:', "fileContents: '文件内容'"]) {
    if (!copy.includes(snippet)) {
      console.error(`zh-cn.ts missing Chinese workspace.read_files AI action input/result copy: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['执行结果解析', '解析执行结果', '结果标题', '结果来源', '解析执行结果需要执行 ID', 'AI 执行结果解析', '证据项', '解析日志', '解析产物']) {
    if (!copy.includes(snippet)) {
      console.error(`zh-cn.ts missing Chinese parse execution result AI action copy: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['事件日志查询', '查询事件日志', '事件日志动作', '运行 ID', '泳道', '事件类型', '记录上限', '输入运行 ID；留空查询全部运行', '输入泳道；留空查询全部泳道', '输入事件类型；留空查询全部类型', '输入最多返回的事件数', 'AI 事件日志查询结果', '当前查询没有匹配的事件日志记录', '事件载荷摘录']) {
    if (!copy.includes(snippet)) {
      console.error(`zh-cn.ts missing Chinese eventlog.query AI action copy: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['失败归纳', 'Sisyphus 失败归纳', '失败执行 ID', '失败类型', '失败说明', '策略目标', '失败记录文本', '失败归纳需要失败执行 ID 或失败记录', '失败黑板条目', '策略变异任务', '归纳摘要']) {
    if (!copy.includes(snippet)) {
      console.error(`zh-cn.ts missing Chinese Sisyphus AI action copy: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['失败闭环链路证据', '闭环动作', '黑板证据', '任务证据', '评测证据', '前沿证据']) {
    if (!copy.includes(snippet)) {
      console.error(`zh-cn.ts missing Chinese AI failure loop evidence copy: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['data-ai-failure-loop-evidence="true"', 'data-ai-failure-loop-action=', 'data-ai-failure-loop-field=', 'buildAIFailureLoopEvidenceRows', 'AIFailureLoopEvidence', "'sisyphus.conclude_failure'", "'harness.cases.from_failure'", "'frontier.autopilot'", 'blackboardItem?.id', 'agentTask?.id', 'harnessCase?.id', 'frontierSummary?.summary']) {
    if (!shellForCopyChecks.includes(snippet)) {
      console.error(`OuroborosIDE.tsx missing AI failure loop evidence rendering/selector: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['目标拆解', '总目标', '标题', '来源', '优先级', '最大子目标数', '目标拆解需要总目标', '写入的黑板条目', '创建的智能体任务', '前沿队列摘要']) {
    if (!copy.includes(snippet)) {
      console.error(`zh-cn.ts missing Chinese goal.decompose AI action copy: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['提示词版本差异', '基准版本 ID', '目标版本 ID', '基准内容摘录', '目标内容摘录', '是否有变化', '新增行数', '删除行数', '差异内容', '提示词版本']) {
    if (!copy.includes(snippet)) {
      console.error(`zh-cn.ts missing Chinese prompt diff/harness prompt version copy: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['模板变量', '变量名', '真实来源', '本次评测值', '新增变量', '移除变量', '每行填写一个变量，格式为：变量名=评测值']) {
    if (!copy.includes(snippet)) {
      console.error(`zh-cn.ts missing Chinese prompt template variable copy: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['连续循环编排', '最大步数', '输入最大循环步数', '连续循环需要编排运行 ID', '由后端根据真实状态循环观察/推进']) {
    if (!copy.includes(snippet)) {
      console.error(`zh-cn.ts missing Chinese orchestrator loop AI action copy: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['报告动作', '该动作不需要额外输入', '报告文件', '报告发现数', '已有报告数', '报告发现', '证据执行', '执行日志', '执行产物', '已有报告']) {
    if (!copy.includes(snippet)) {
      console.error(`zh-cn.ts missing Chinese AI report action copy: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['证据入库', '证据标题', '证据摘要', '风险等级', '执行 ID', '产物路径', '证据文件', '图谱节点 ID', '来源', '回放事件 ID', '写入证据需要标题、摘要和风险等级', 'AI 证据入库结果']) {
    if (!copy.includes(snippet)) {
      console.error(`zh-cn.ts missing Chinese AI evidence ingestion copy: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['fakeEvidence', 'mockEvidence', 'demoEvidence', 'sampleEvidence', '证据内容示例', '示例证据内容', '样例证据内容']) {
    if (copy.includes(snippet)) {
      console.error(`zh-cn.ts must not include mock AI evidence ingestion content: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['agents:', '智能体', '多智能体工作台', '智能体任务', '暂无智能体任务', '任务统计', '任务目标', '执行角色', '智能体 ID', '关联黑板条目', '创建智能体任务', '读取智能体任务', '后端返回的智能体任务为空']) {
    if (!copy.includes(snippet)) {
      console.error(`zh-cn.ts missing Chinese Agent Orchestrator v1 copy: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['harness:', '评测', '评测工作台', '评测用例', '评测结果', '评测统计', '暂无评测用例', '暂无评测结果', '期望结果', '用例标题', '用例目标', '输入状态快照', '模型服务快照', '工具链快照', '模型服务配置', '工具动作数量', '运行用例', '创建用例', '刷新评测', '打开结果文件', '打开日志文件', '打开产物目录', '查看执行', '退出码', '耗时', '日志字节', '产物数量', '产物字节', 'AI 消息', '工具调用', '文件差异', '事件日志引用', '原始事件 ID', '回放事件 ID']) {
    if (!copy.includes(snippet)) {
      console.error(`zh-cn.ts missing Chinese Harness Engineering v1 copy: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['blackboard:', '黑板', '黑板条目', '暂无黑板条目', '黑板摘要', '全部条目', '打开条目', '完成条目', '打开文件', '查看执行', '定位图谱', '定位回放', '定位报告', '目标', '假设', '发现', '问题', '失败项', '打开', '完成', '高', '中', '低']) {
    if (!copy.includes(snippet)) {
      console.error(`zh-cn.ts missing Chinese Blackboard copy: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['黑板动作', '条目类型', '黑板标题', '黑板内容', '状态', '优先级', '来源', '关联文件', '关联执行', '黑板条目', '后端返回的黑板条目为空']) {
    if (!copy.includes(snippet)) {
      console.error(`zh-cn.ts missing Chinese Blackboard AI action copy: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['观察编排结果', '编排运行 ID', '观察记录', '后端返回的编排观察为空']) {
    if (!copy.includes(snippet)) {
      console.error(`zh-cn.ts missing Chinese Orchestrator observe action copy: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['自动推进下一步', '自动推进需要编排运行 ID', '观察 ID 可不填，后端会使用最近一次观察', '由后端根据真实观察自动生成标题、目标和命令']) {
    if (!copy.includes(snippet)) {
      console.error(`zh-cn.ts missing Chinese Orchestrator auto-advance action copy: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['连续步进编排', '连续步进需要编排运行 ID', '由后端根据真实状态自动选择观察或推进']) {
    if (!copy.includes(snippet)) {
      console.error(`zh-cn.ts missing Chinese Orchestrator step action copy: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['自动解析证据', '自动解析发现', '证据引用', '发现引用', '证据数量', '发现数量', '解析来源', '解析批次']) {
    if (!copy.includes(snippet)) {
      console.error(`zh-cn.ts missing Chinese Orchestrator auto-parse evidence/finding copy: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['规划分派', '规划状态', '来自真实工作区规划文件', '后端会读取真实工作区、黑板、智能体、评测和编排状态，再写入规划与分派对象', '规划标题', '规划目标', '下一步标题', '下一步目标', '下一步命令', '期望结果', '规划分派需要标题和目标', 'AI 规划分派', '规划状态文件', '规划数量', '规划记录', '真实上下文摘要', '关联编排运行', '关联评测用例', '后端返回的规划记录为空']) {
    if (!copy.includes(snippet)) {
      console.error(`zh-cn.ts missing Chinese Planner Dispatch v1 action copy: ${snippet}`);
      failed = true;
    }
  }
  for (const snippet of ['Planner', 'Planning', 'Dispatch', 'Plan']) {
    const visibleString = new RegExp(`:\\s*['"\`][^'"\`]*${snippet}[^'"\`]*['"\`]`);
    if (visibleString.test(copy)) {
      console.error(`zh-cn.ts must not expose English Planner Dispatch visible copy: ${snippet}`);
      failed = true;
    }
  }
  if (!/modes:\s*\{[\s\S]*?blackboard:\s*'黑板'/m.test(copy)) {
    console.error("zh-cn.ts must expose the Blackboard mode label as Chinese visible copy: blackboard: '黑板'");
    failed = true;
  }
  if (!/modes:\s*\{[\s\S]*?agents:\s*'智能体'/m.test(copy)) {
    console.error("zh-cn.ts must expose the Agent mode label as Chinese visible copy: agents: '智能体'");
    failed = true;
  }
  if (!/modes:\s*\{[\s\S]*?harness:\s*'评测'/m.test(copy)) {
    console.error("zh-cn.ts must expose the Harness mode label as Chinese visible copy: harness: '评测'");
    failed = true;
  }
  for (const snippet of ['Harness', 'Case', 'Result']) {
    const visibleString = new RegExp(`:\\s*['"\`][^'"\`]*${snippet}[^'"\`]*['"\`]`);
    if (visibleString.test(copy)) {
      console.error(`zh-cn.ts must not expose Harness/Case/Result English visible copy: ${snippet}`);
      failed = true;
    }
  }
}

if (failed) {
  process.exit(1);
}
