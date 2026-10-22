import { existsSync, readFileSync, readdirSync } from 'node:fs';
import { join } from 'node:path';
import postcss from 'postcss';

// The Go orchestrator passes the repository root as argv[2]; the embedded
// driver is materialised inside gui/ only so bare `import 'postcss'`
// resolves exactly as it does for gui/scripts/check-product-contract.mjs.
const root = process.argv[2];
if (!root) {
  console.error('product contract driver: missing repository root argument');
  process.exit(2);
}

// The master product plan is the latest *.md document in
// docs/superpowers/plans (filenames are date-prefixed, so lexicographic
// order == chronological order). A missing/empty plans directory is an
// environment problem, not a contract violation: exit 2.
const plansDir = join(root, 'docs/superpowers/plans');
let planEntries = [];
try {
  planEntries = readdirSync(plansDir).filter(name => name.endsWith('.md')).sort();
} catch {
  planEntries = [];
}
if (planEntries.length === 0) {
  console.error(`missing product master plan: docs/superpowers/plans must contain at least one *.md plan document`);
  process.exit(2);
}
const masterPlanFile = `docs/superpowers/plans/${planEntries[planEntries.length - 1]}`;

const files = [
  'PRODUCT.md',
  'DESIGN.md',
  'docs/superpowers/specs/2026-07-03-gui-rebuild-design.md',
  masterPlanFile,
  '.planning/2026-07-03-gui-rebuild/task_plan.md',
];

const required = [
  '桌面',
  '真实',
  '中文',
];

const forbidden = [
  'safety gate',
  'safety-gate',
  'approval gate',
  'authorization blocker',
  'scope/authorization',
  '安全门控',
  '审批门控',
  '授权阻断',
  '范围校验',
  '人工门控',
];

const requiredDesignDirection = [
  'layout, density, and interaction references',
  'red-black Ouroboros brand system',
  'real biting-tail snake assets',
  'green is limited to semantic success indicators',
  'attack graph must not introduce a blue or green visual theme',
];

const rendererFile = 'gui/src/app/OuroborosIDE.tsx';
const typesFile = 'gui/src/app/types.ts';
const apiFile = 'gui/src/app/api.ts';
const localeFile = 'gui/src/app/zh-cn.ts';
const electronMainFile = 'gui/src/main/main.js';
const themeFile = 'gui/src/styles/ouroboros-ide.css';
const shellSurfaceClasses = [
  'title-bar',
  'activity-bar',
  'workspace-explorer',
  'workbench',
  'right-rail',
  'status-bar',
];
const rendererAssets = [
  {
    file: 'ouroboros-mark.webp',
    importPattern: /import\s+([A-Za-z_$][\w$]*)\s+from\s+['"]\.\.\/assets\/ouroboros-mark\.webp['"]\s*;?/,
  },
  {
    file: 'ouroboros-empty-mark.webp',
    importPattern: /import\s+([A-Za-z_$][\w$]*)\s+from\s+['"]\.\.\/assets\/ouroboros-empty-mark\.webp['"]\s*;?/,
  },
];
const brandAssets = [
  { file: 'gui/src/assets/ouroboros-mark.webp', format: 'WebP' },
  { file: 'gui/src/assets/ouroboros-empty-mark.webp', format: 'WebP' },
  { file: 'gui/electron/assets/ouroboros-icon.png', format: 'PNG' },
];
const redBlackBaseTokens = [
  '--bg',
  '--bg-2',
  '--panel',
  '--panel-2',
  '--panel-3',
  '--line',
  '--line-strong',
];
const oldBlueBlackBaseValues = new Set([
  '#07090b',
  '#0b0f12',
  '#10151a',
  '#141a20',
  '#192129',
]);
const minimumBrandAssetBytes = 4 * 1024;

let failed = false;

for (const file of files) {
  const text = readFileSync(join(root, file), 'utf8');
  for (const phrase of forbidden) {
    if (text.includes(phrase)) {
      console.error(`${file} contains old product direction: ${phrase}`);
      failed = true;
    }
  }
}

const masterPlan = readFileSync(join(root, masterPlanFile), 'utf8');
for (const phrase of required) {
  if (!masterPlan.includes(phrase)) {
    console.error(`master plan missing required direction: ${phrase}`);
    failed = true;
  }
}

const design = readFileSync(join(root, 'DESIGN.md'), 'utf8');
for (const phrase of requiredDesignDirection) {
  if (!design.includes(phrase)) {
    console.error(`DESIGN.md missing red-black snake brand precedence: ${phrase}`);
    failed = true;
  }
}

const renderer = readFileSync(join(root, rendererFile), 'utf8');
const types = readFileSync(join(root, typesFile), 'utf8');
const api = readFileSync(join(root, apiFile), 'utf8');
const locale = readFileSync(join(root, localeFile), 'utf8');
const rendererAssetImports = new Map();
const buildAIExecutionInputSource = renderer.match(/^function buildAIExecutionInput[\s\S]*?^\}/m)?.[0] ?? '';
const runSelectedAIActionSource = renderer.match(/^  async function runSelectedAIAction[\s\S]*?^  \}/m)?.[0] ?? '';
const refreshSelectedExecutionDetailsSource = renderer.match(/^  async function refreshSelectedExecutionDetails[\s\S]*?^  \}/m)?.[0] ?? '';
const renderAIActionResultSource = renderer.match(/^function renderAIActionResult[\s\S]*?^\}/m)?.[0] ?? '';
for (const asset of rendererAssets) {
  const assetImport = renderer.match(asset.importPattern);
  if (!assetImport) {
    console.error(`${rendererFile} must import the real brand asset: ${asset.file}`);
    failed = true;
    continue;
  }

  rendererAssetImports.set(asset.file, assetImport[1]);
  const importedName = escapeRegExp(assetImport[1]);
  const imageUsage = new RegExp(`<img\\b[^>]*\\bsrc\\s*=\\s*\\{\\s*${importedName}\\s*\\}[^>]*>`, 's');
  if (!imageUsage.test(renderer)) {
    console.error(`${rendererFile} must render ${asset.file} with an img element`);
    failed = true;
  }
}

const brandMarkImport = rendererAssetImports.get('ouroboros-mark.webp');
const brandMarkElements = [...renderer.matchAll(/<([A-Za-z][\w:.-]*)\b[^>]*\b(?:className|class)\s*=\s*(["'])([^"']*)\2[^>]*>/g)]
  .filter(([, , , classNames]) => classNames.split(/\s+/).includes('brand-mark'));
if (brandMarkElements.length === 0) {
  console.error(`${rendererFile} must render the product mark as img.brand-mark`);
  failed = true;
}
for (const [openingTag, element] of brandMarkElements) {
  if (element.toLowerCase() !== 'img') {
    console.error(`${rendererFile} must not replace img.brand-mark with an abstract <${element}> element`);
    failed = true;
    continue;
  }
  if (!brandMarkImport || !new RegExp(`\\bsrc\\s*=\\s*\\{\\s*${escapeRegExp(brandMarkImport)}\\s*\\}`).test(openingTag)) {
    console.error(`${rendererFile} img.brand-mark must use the imported ouroboros-mark.webp asset`);
    failed = true;
  }
}

if (!renderer.includes('workspace-empty') || !renderer.includes('focus-mode')) {
  console.error(`${rendererFile} must expose the workspace-empty shell state`);
  failed = true;
}
if (!renderer.includes("!needsWorkspace && !['graph', 'replay', 'report', 'settings'].includes(mode)")) {
  console.error(`${rendererFile} must hide the empty graph rail until a workspace is connected`);
  failed = true;
}
if (!/function graphPosition\([\s\S]*?const visibleNodes = nodes[\s\S]*?graphPosition\(index, visibleNodes\.length, compact\)/.test(renderer)) {
  console.error(`${rendererFile} must render and reflow every node in the attack graph`);
  failed = true;
}

const executionActionContracts = [
  {
    file: typesFile,
    source: types,
    pattern: /export type AIActionKind\s*=\s*[^;]*'execution'/,
    message: 'AIActionKind must include backend execution actions',
  },
  {
    file: typesFile,
    source: types,
    pattern: /\bwaitTimeoutSeconds\?:\s*number\s*;/,
    message: 'AIActionRunInput must preserve waitTimeoutSeconds',
  },
  {
    file: apiFile,
    source: api,
    pattern: /case ['"]execution['"]:\s*return ['"]execution['"];/,
    message: 'API action mapping must accept the backend execution kind',
  },
  {
    file: apiFile,
    source: api,
    pattern: /execution:\s*payload\.execution\?\.id\s*\?\s*mapRequiredExecutionResponse/,
    message: 'AI action responses must preserve the real top-level execution',
  },
  {
    file: apiFile,
    source: api,
    pattern: /executionLogs:\s*mapAIActionExecutionLogs\(result\.executionLogs\)/,
    message: 'AI action responses must preserve real execution logs',
  },
  {
    file: apiFile,
    source: api,
    pattern: /executionArtifacts:\s*mapAIActionExecutionArtifacts\(result\.executionArtifacts\)/,
    message: 'AI action responses must preserve real execution artifacts',
  },
  {
    file: apiFile,
    source: api,
    pattern: /retryOf:\s*execution\.retryOf/,
    message: 'execution mapping must preserve retryOf',
  },
  {
    file: rendererFile,
    source: renderer,
    pattern: /function isExecutionAIAction[\s\S]*?execution\.read[\s\S]*?execution\.wait[\s\S]*?execution\.stop[\s\S]*?execution\.retry/,
    message: 'AI Action panel must recognize all execution supervision actions',
  },
  {
    file: rendererFile,
    source: buildAIExecutionInputSource,
    pattern: /executionId[\s\S]*?waitTimeoutSeconds[\s\S]*?Number\.isInteger/,
    message: 'execution action requests must require an execution ID and accept only positive integer wait seconds',
  },
  {
    file: rendererFile,
    source: renderer,
    pattern: /type=['"]number['"][\s\S]{0,160}min=['"]1['"][\s\S]{0,80}step=['"]1['"][\s\S]{0,80}inputMode=['"]numeric['"]/,
    message: 'execution.wait controls must match the positive integer backend schema',
  },
  {
    file: rendererFile,
    source: renderer,
    pattern: /const executionInputError = aiExecutionInputError\(action, aiActionInputs\)[\s\S]{0,180}setAIActionStatus\(executionInputError\)/,
    message: 'invalid execution inputs must stop submission with a visible status',
  },
  {
    file: rendererFile,
    source: renderer,
    pattern: /data-ai-action-execution-result=['"]true['"]/,
    message: 'execution supervision results must use a structured result view',
  },
  {
    file: rendererFile,
    source: runSelectedAIActionSource,
    pattern: /isExecutionAIAction\(action\)\s*&&\s*result\.execution[\s\S]{0,240}refreshSelectedExecutionDetails\(result\.execution\.id\)/,
    message: 'manual execution actions must refresh the returned execution without navigating away',
  },
  {
    file: rendererFile,
    source: refreshSelectedExecutionDetailsSource,
    pattern: /^(?![\s\S]*setMode\()[\s\S]*setSelectedExecutionId\(executionId\)[\s\S]*showExecutionLogs\(executionId\)[\s\S]*showExecutionArtifacts\(executionId\)/,
    message: 'automatic execution detail refresh must update selection, logs, and artifacts without changing mode',
  },
  {
    file: rendererFile,
    source: runSelectedAIActionSource,
    pattern: /setAIActionResult\(result\)[\s\S]*?try\s*\{\s*await refresh\(\);\s*\}\s*catch\s*\(refreshError\)[\s\S]*?resultRefreshFailed[\s\S]*?if\s*\(isExecutionAIAction/,
    message: 'a post-action workspace refresh failure must preserve the successful execution result',
  },
  {
    file: rendererFile,
    source: renderAIActionResultSource,
    pattern: /\[zhCN\.aiActions\.toolMessage,\s*isExecutionSupervisionResult\s*\?\s*undefined\s*:\s*result\.message\.content\]/,
    message: 'execution supervision results must not duplicate the persisted tool message above structured logs',
  },
  {
    file: localeFile,
    source: locale,
    pattern: /executionKind:\s*['"]执行监督动作['"][\s\S]*?executionWaitTimeoutSecondsInput:[\s\S]*?executionWaitTimeoutSecondsInvalid:\s*['"]等待秒数必须是大于零的整数['"][\s\S]*?executionInspectResult:/,
    message: 'execution action controls and results must have Chinese product copy',
  },
];

for (const contract of executionActionContracts) {
  if (!contract.pattern.test(contract.source)) {
    console.error(`${contract.file} ${contract.message}`);
    failed = true;
  }
}

const electronMain = readFileSync(join(root, electronMainFile), 'utf8');
if (!/nativeImage\.createFromPath\(\s*path\.join\(\s*__dirname\s*,\s*['"]assets['"]\s*,\s*['"]ouroboros-icon\.png['"]\s*\)\s*\)/m.test(electronMain)) {
  console.error(`${electronMainFile} must load ouroboros-icon.png as a native Electron window icon`);
  failed = true;
}
if (!/window\.setIcon\(windowIcon\)/m.test(electronMain)) {
  console.error(`${electronMainFile} must explicitly publish the snake icon to the native Linux/Windows window`);
  failed = true;
}

for (const asset of brandAssets) {
  checkBrandAsset(asset);
}

const theme = readFileSync(join(root, themeFile), 'utf8');
try {
  checkParsedThemeContract(postcss.parse(theme, { from: themeFile }));
} catch (error) {
  console.error(`${themeFile} must remain valid parseable CSS: ${error instanceof Error ? error.message : String(error)}`);
  failed = true;
}
if (!/\.ai-action-execution-result\b/.test(theme)) {
  console.error(`${themeFile} must style the structured execution supervision result`);
  failed = true;
}
if (!/\.ouro-body\.workspace-empty\s*\{/.test(theme) || !/\.workspace-welcome\s*\{/.test(theme)) {
  console.error(`${themeFile} must define the compact workspace onboarding layout`);
  failed = true;
}
if (!/\.ai-message-row\[data-role=["']user["']\]\s*\{[\s\S]*?grid-template-areas:\s*["']body avatar["']/.test(theme)) {
  console.error(`${themeFile} must keep user message content in the wide grid column`);
  failed = true;
}
if (!/\.visually-hidden\s*\{[\s\S]*?clip-path:\s*inset\(50%\)/.test(theme)) {
  console.error(`${themeFile} must visually hide accessible-only labels without removing them`);
  failed = true;
}
if (!/\.right-rail\.no-selection\s*\{[\s\S]*?grid-template-rows:/.test(theme) || !/\.right-rail\.has-selection\s+\.inspector\s*>\s*\.node-summary\s*\{[\s\S]*?overflow:\s*auto/.test(theme)) {
  console.error(`${themeFile} must bound the inspector in both empty and selected states`);
  failed = true;
}
if (!/\.ouro-body\s*\{[\s\S]*?grid-template-rows:\s*minmax\(0,\s*1fr\)/.test(theme)) {
  console.error(`${themeFile} must bound connected workspace content to the desktop body row`);
  failed = true;
}
const executionOutputRule = theme.match(/\.ai-action-execution-logs pre,\s*\.ai-action-execution-artifacts pre\s*\{([\s\S]*?)\}/);
if (!executionOutputRule || !/\bborder:\s*1px\s+solid\s+var\(--line\)/.test(executionOutputRule[1]) || /border-left/.test(executionOutputRule[1])) {
  console.error(`${themeFile} execution log and artifact output must use a complete border`);
  failed = true;
}
if (!/\.ai-action-execution-result[^\{]*data-status=['"]completed['"][\s\S]*?data-status=['"]failed['"][\s\S]*?data-status=['"]stopped['"]/.test(theme)) {
  console.error(`${themeFile} execution result styles must cover real backend terminal statuses`);
  failed = true;
}
const rootTheme = theme.match(/:root\s*\{([\s\S]*?)\}/);
if (!rootTheme) {
  console.error(`${themeFile} must define the product theme in :root`);
  failed = true;
} else {
  const tokens = new Map(
    [...rootTheme[1].matchAll(/^\s*(--[\w-]+)\s*:\s*([^;]+);/gm)]
      .map(([, name, value]) => [name, value.trim().toLowerCase()]),
  );

  for (const token of redBlackBaseTokens) {
    const value = tokens.get(token);
    if (!value) {
      console.error(`${themeFile} :root missing red-black base token: ${token}`);
      failed = true;
      continue;
    }
    if (oldBlueBlackBaseValues.has(value)) {
      console.error(`${themeFile} :root must not restore old blue-black base token ${token}: ${value}`);
      failed = true;
      continue;
    }
    if (!isRedDominantHex(value)) {
      console.error(`${themeFile} :root token ${token} must be an explicit red-tinted black hex color, received: ${value}`);
      failed = true;
    }
  }

  const brandRed = tokens.get('--brand-red');
  if (!brandRed || !isRedDominantHex(brandRed)) {
    console.error(`${themeFile} :root must define --brand-red as an explicit red-dominant hex color`);
    failed = true;
  }
  if (tokens.get('--red') !== 'var(--brand-red)') {
    console.error(`${themeFile} :root must keep --red linked to --brand-red`);
    failed = true;
  }
}

if (failed) {
  process.exit(1);
}

function checkBrandAsset(asset) {
  const fullPath = join(root, asset.file);
  if (!existsSync(fullPath)) {
    console.error(`missing real Ouroboros brand asset: ${asset.file}`);
    failed = true;
    return;
  }

  const bytes = readFileSync(fullPath);
  if (bytes.length < minimumBrandAssetBytes) {
    console.error(`${asset.file} is too small to be a real brand asset: ${bytes.length} bytes`);
    failed = true;
  }
  if (!hasExpectedImageMagic(bytes, asset.format)) {
    console.error(`${asset.file} does not have a valid ${asset.format} signature`);
    failed = true;
  }
}

function hasExpectedImageMagic(bytes, format) {
  if (format === 'WebP') {
    return bytes.length >= 12
      && bytes.subarray(0, 4).toString('ascii') === 'RIFF'
      && bytes.subarray(8, 12).toString('ascii') === 'WEBP';
  }

  const pngSignature = [0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a];
  return bytes.length >= pngSignature.length
    && pngSignature.every((value, index) => bytes[index] === value);
}

function isRedDominantHex(value) {
  const color = parseHexColor(value);
  return color !== null && color.red > color.green && color.red > color.blue;
}

function parseHexColor(value) {
  const match = /^#([\da-f]{2})([\da-f]{2})([\da-f]{2})$/i.exec(value);
  if (!match) {
    return null;
  }
  return {
    red: Number.parseInt(match[1], 16),
    green: Number.parseInt(match[2], 16),
    blue: Number.parseInt(match[3], 16),
  };
}

function checkParsedThemeContract(themeRoot) {
  const declarations = collectCSSDeclarations(themeRoot);
  const variables = collectRootVariables(declarations);

  for (const className of shellSurfaceClasses) {
    const candidates = declarations.filter(declaration => (
      isSurfaceColorProperty(declaration.property)
      && selectorSubjectHasClass(declaration.selector, className)
    ));
    checkRedBlackSurface(`.${className}`, winningDeclaration(candidates), variables);
  }

  const activeSelectors = new Set(
    declarations
      .map(declaration => declaration.selector)
      .filter(selector => isOrdinaryActiveSelector(selector) && !isGraphSelector(selector)),
  );
  for (const selector of activeSelectors) {
    const candidates = declarations.filter(declaration => (
      declaration.selector === selector && isSurfaceColorProperty(declaration.property)
    ));
    checkRedBlackSurface(selector, winningDeclaration(candidates), variables);
  }

  const finalBrandRoot = winningDeclaration(declarations.filter(declaration => (
    declaration.selector === ':root' && declaration.property === '--ouro-surface-0'
  )));
  if (!finalBrandRoot) {
    console.error(`${themeFile} must define the final red-black brand surface tokens in :root`);
    failed = true;
    return;
  }

  for (const declaration of declarations) {
    if (declaration.order < finalBrandRoot.order || declaration.property.startsWith('--')) {
      continue;
    }
    const colors = parseCSSColors(resolveCSSVariables(declaration.value, variables));
    if (colors.some(isBlueDominantColor) && !isGraphSelector(declaration.selector)) {
      console.error(`${themeFile} blue is reserved for graph selectors; ${declaration.selector} ${declaration.property}: ${declaration.value}`);
      failed = true;
    }
  }
}

function collectCSSDeclarations(themeRoot) {
  const declarations = [];
  let order = 0;

  themeRoot.walkDecls(declaration => {
    const rule = declaration.parent;
    const declarationOrder = order;
    order += 1;
    if (!rule || rule.type !== 'rule') {
      return;
    }

    for (const selector of rule.selectors) {
      const normalizedSelector = normalizeSelector(selector);
      declarations.push({
        selector: normalizedSelector,
        property: declaration.prop.trim().toLowerCase(),
        value: declaration.value.trim(),
        important: declaration.important,
        specificity: selectorSpecificity(normalizedSelector),
        order: declarationOrder,
      });
    }
  });

  return declarations;
}

function collectRootVariables(declarations) {
  const winners = new Map();
  for (const declaration of declarations) {
    if (declaration.selector !== ':root' || !declaration.property.startsWith('--')) {
      continue;
    }
    const current = winners.get(declaration.property);
    if (!current || compareCascadePriority(declaration, current) > 0) {
      winners.set(declaration.property, declaration);
    }
  }
  return new Map([...winners].map(([property, declaration]) => [property, declaration.value]));
}

function checkRedBlackSurface(selector, declaration, variables) {
  if (!declaration) {
    console.error(`${themeFile} ${selector} must have a final red-black background source`);
    failed = true;
    return;
  }

  const resolvedValue = resolveCSSVariables(declaration.value, variables);
  const colors = parseCSSColors(resolvedValue);
  const surfaceColor = colors.at(-1);
  if (!surfaceColor || !isRedDominantBlackColor(surfaceColor)) {
    console.error(`${themeFile} ${selector} final ${declaration.property} must resolve to a red-dominant black color; received ${declaration.value} (${resolvedValue}) from ${declaration.selector}`);
    failed = true;
  }
}

function winningDeclaration(declarations) {
  return declarations.reduce((winner, declaration) => (
    !winner || compareCascadePriority(declaration, winner) > 0 ? declaration : winner
  ), null);
}

function compareCascadePriority(left, right) {
  if (left.important !== right.important) {
    return left.important ? 1 : -1;
  }
  for (let index = 0; index < left.specificity.length; index += 1) {
    if (left.specificity[index] !== right.specificity[index]) {
      return left.specificity[index] > right.specificity[index] ? 1 : -1;
    }
  }
  return Math.sign(left.order - right.order);
}

function isSurfaceColorProperty(property) {
  return property === 'background' || property === 'background-color';
}

function normalizeSelector(selector) {
  return selector.replace(/\s+/g, ' ').trim();
}

function selectorSubjectHasClass(selector, className) {
  const subject = selectorSubject(selector);
  return !subject.includes('::') && new RegExp(`\\.${escapeRegExp(className)}(?![\\w-])`).test(subject);
}

function selectorSubject(selector) {
  let bracketDepth = 0;
  let parenthesisDepth = 0;
  let quote = '';
  let boundary = -1;

  for (let index = 0; index < selector.length; index += 1) {
    const character = selector[index];
    if (quote) {
      if (character === '\\') {
        index += 1;
      } else if (character === quote) {
        quote = '';
      }
      continue;
    }
    if (character === '"' || character === "'") {
      quote = character;
    } else if (character === '[') {
      bracketDepth += 1;
    } else if (character === ']') {
      bracketDepth = Math.max(0, bracketDepth - 1);
    } else if (character === '(') {
      parenthesisDepth += 1;
    } else if (character === ')') {
      parenthesisDepth = Math.max(0, parenthesisDepth - 1);
    } else if (bracketDepth === 0 && parenthesisDepth === 0 && (character === '>' || character === '+' || character === '~' || /\s/.test(character))) {
      boundary = index;
    }
  }

  return selector.slice(boundary + 1).trim();
}

function isOrdinaryActiveSelector(selector) {
  const subject = selectorSubject(selector);
  if (subject.includes('::')) {
    return false;
  }
  return /\.(?:active|selected)(?![\w-])/.test(subject)
    || /\[(?:aria-selected|data-[\w-]*(?:active|selected)[\w-]*)\s*=\s*["']?true["']?\]/i.test(subject);
}

function isGraphSelector(selector) {
  const identifiers = [...selector.matchAll(/[.#]([_a-zA-Z][\w-]*)/g)].map(([, identifier]) => identifier.toLowerCase());
  return identifiers.some(identifier => (
    identifier === 'graph'
    || identifier.startsWith('graph-')
    || identifier.endsWith('-graph')
    || identifier.includes('-graph-')
  )) || /\[[^\]]*(?:^|[-_])graph(?:[-_]|\b)[^\]]*\]/i.test(selector);
}

function selectorSpecificity(selector) {
  const ids = selector.match(/#[\w-]+/g)?.length ?? 0;
  const classes = selector.match(/\.[\w-]+/g)?.length ?? 0;
  const attributes = selector.match(/\[[^\]]+\]/g)?.length ?? 0;
  const pseudoClasses = selector.match(/:(?!:)[\w-]+/g)?.length ?? 0;
  const pseudoElements = selector.match(/::[\w-]+/g)?.length ?? 0;
  return [ids, classes + attributes + pseudoClasses, pseudoElements];
}

function resolveCSSVariables(value, variables, stack = []) {
  return value.replace(/var\(\s*(--[\w-]+)\s*(?:,\s*([^()]*))?\)/g, (match, name, fallback) => {
    if (stack.includes(name)) {
      return match;
    }
    const replacement = variables.get(name) ?? fallback;
    return replacement === undefined ? match : resolveCSSVariables(replacement.trim(), variables, [...stack, name]);
  });
}

function parseCSSColors(value) {
  const colors = [];
  const colorPattern = /#([\da-f]{3,8})\b|rgba?\(([^)]*)\)/gi;
  for (const match of value.matchAll(colorPattern)) {
    const color = match[1] ? parseFlexibleHexColor(match[1]) : parseRGBColor(match[2]);
    if (color) {
      colors.push(color);
    }
  }
  return colors;
}

function parseFlexibleHexColor(hex) {
  if (![3, 4, 6, 8].includes(hex.length)) {
    return null;
  }
  const expanded = hex.length <= 4 ? [...hex].map(character => character.repeat(2)).join('') : hex;
  return {
    red: Number.parseInt(expanded.slice(0, 2), 16),
    green: Number.parseInt(expanded.slice(2, 4), 16),
    blue: Number.parseInt(expanded.slice(4, 6), 16),
    alpha: expanded.length === 8 ? Number.parseInt(expanded.slice(6, 8), 16) / 255 : 1,
  };
}

function parseRGBColor(body) {
  const [channels, alpha = '1'] = body.split('/').map(part => part.trim());
  const values = channels.split(/[\s,]+/).filter(Boolean);
  if (values.length < 3) {
    return null;
  }
  const parsed = values.slice(0, 3).map(parseColorChannel);
  if (parsed.some(channel => channel === null)) {
    return null;
  }
  return {
    red: parsed[0],
    green: parsed[1],
    blue: parsed[2],
    alpha: parseAlphaChannel(alpha.replace(/^,\s*/, '')),
  };
}

function parseColorChannel(value) {
  const number = Number.parseFloat(value);
  if (!Number.isFinite(number)) {
    return null;
  }
  return value.endsWith('%') ? Math.round(number * 2.55) : Math.round(number);
}

function parseAlphaChannel(value) {
  const number = Number.parseFloat(value);
  if (!Number.isFinite(number)) {
    return 1;
  }
  return value.endsWith('%') ? number / 100 : number;
}

function isRedDominantBlackColor(color) {
  return color.alpha > 0 && color.red > color.green && color.red > color.blue && Math.max(color.red, color.green, color.blue) <= 64;
}

function isBlueDominantColor(color) {
  return color.alpha > 0 && color.blue > color.red && color.blue > color.green;
}

function escapeRegExp(value) {
  return value.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
}
