import assert from 'node:assert/strict';
import { EventEmitter } from 'node:events';
import { chmodSync, existsSync, mkdtempSync, mkdirSync, readFileSync, rmSync, statSync, writeFileSync } from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';
import { spawnSync } from 'node:child_process';

const guiRoot = fileURLToPath(new URL('..', import.meta.url));
const temporaryRoot = mkdtempSync(path.join(os.tmpdir(), 'ouro-electron-runtime-'));
const outputMain = path.join(temporaryRoot, 'dist', 'main');
const resourcesPathDescriptor = Object.getOwnPropertyDescriptor(process, 'resourcesPath');

class ContractApp extends EventEmitter {
  constructor(userData) {
    super();
    this.isPackaged = true;
    this.userData = userData;
  }

  getPath(name) {
    return name === 'userData' ? this.userData : process.execPath;
  }

  getVersion() {
    return 'contract';
  }

  getAppPath() {
    return this.userData;
  }
}

try {
  writeFileSync(path.join(temporaryRoot, 'package.json'), '{"type":"module"}\n');
  // 桌面 main 构建由 Go 工具链 (cmd/desktop build-main) 负责;此处调用它,
  // 保留 OURO_DESKTOP_MAIN_OUTPUT 语义。
  const repoRoot = path.dirname(guiRoot);
  const build = spawnSync('go', ['run', './cmd/desktop', 'build-main'], {
    cwd: repoRoot,
    encoding: 'utf8',
    env: { ...process.env, OURO_DESKTOP_MAIN_OUTPUT: outputMain },
  });
  assert.equal(build.status, 0, build.stderr || build.stdout);

  const memoryModule = await import(pathToFileURL(path.join(outputMain, 'memory-profile.js')).href);
  checkMemoryProfileSelection(memoryModule);
  checkElectronMemoryLimits(memoryModule, temporaryRoot);
  await checkPackagedBootstrapRelaunch(outputMain, temporaryRoot);

  const backendModule = await import(pathToFileURL(path.join(outputMain, 'backend.js')).href);
  assert.equal(backendModule.normalizeRecordedExitCode(-1), null);
  assert.equal(backendModule.normalizeRecordedExitCode(null), null);
  assert.equal(backendModule.normalizeRecordedExitCode(0), 0);
  assert.equal(backendModule.normalizeRecordedExitCode(7), 7);

  const resources = path.join(temporaryRoot, 'resources');
  mkdirSync(resources, { recursive: true });
  const sidecar = path.join(resources, process.platform === 'win32' ? 'ouro.exe' : 'ouro');
  writeFileSync(sidecar, 'sidecar');
  if (process.platform !== 'win32') {
    chmodSync(sidecar, 0o644);
  }
  Object.defineProperty(process, 'resourcesPath', {
    configurable: true,
    value: resources,
  });
  const permission = backendModule.ensurePackagedSidecarExecutable({ isPackaged: true });
  assert.equal(permission.path, sidecar);
  assert.equal(permission.executable, true);
  if (process.platform !== 'win32') {
    assert.equal(permission.repaired, true);
    assert.notEqual(statSync(sidecar).mode & 0o111, 0);
  }

  await checkDevelopmentCommandDiscovery(backendModule, resources);

  await checkBackendLifecycleContracts({ backendModule, resources, sidecar });
} finally {
  if (resourcesPathDescriptor) {
    Object.defineProperty(process, 'resourcesPath', resourcesPathDescriptor);
  } else {
    delete process.resourcesPath;
  }
  rmSync(temporaryRoot, { recursive: true, force: true });
}

console.log('electron runtime contract: ok');

function checkMemoryProfileSelection({
  applyMemoryEnvironment,
  buildMemoryRelaunchArgs,
  hasMemoryLaunchProfile,
  resolveMemoryProfile,
}) {
  const packagedDefault = resolveMemoryProfile('', true);
  assert.equal(packagedDefault.mode, 'low', 'packaged Electron must default to the low-memory profile');
  assert.equal(resolveMemoryProfile('balanced', true).mode, 'balanced');
  assert.equal(resolveMemoryProfile('standard', true).mode, 'standard');
  assert.equal(resolveMemoryProfile('invalid', true).mode, 'low');
  assert.equal(resolveMemoryProfile('', false).mode, 'standard');

  const inheritedEnvironment = {};
  applyMemoryEnvironment(inheritedEnvironment, packagedDefault);
  assert.deepEqual(inheritedEnvironment, {
    OURO_DESKTOP_MEMORY_MODE: 'low',
    GOMEMLIMIT: '320MiB',
    GOGC: '50',
  });

  const clampedEnvironment = { GOMEMLIMIT: '8GiB', GOGC: '500' };
  applyMemoryEnvironment(clampedEnvironment, packagedDefault);
  assert.equal(clampedEnvironment.GOMEMLIMIT, '320MiB');
  assert.equal(clampedEnvironment.GOGC, '50');

  const stricterEnvironment = { GOMEMLIMIT: '256MiB', GOGC: '40' };
  applyMemoryEnvironment(stricterEnvironment, packagedDefault);
  assert.equal(stricterEnvironment.GOMEMLIMIT, '256MiB');
  assert.equal(stricterEnvironment.GOGC, '40');

  const launchArgs = buildMemoryRelaunchArgs([
    '--user-argument',
    '--js-flags=--max-old-space-size=2048',
    '--renderer-process-limit=9',
  ], packagedDefault);
  assert.ok(launchArgs.includes('--user-argument'));
  assert.ok(launchArgs.includes('--ouro-desktop-memory-profile=low'));
  assert.ok(launchArgs.includes('--js-flags=--max-old-space-size=256 --max-semi-space-size=8'));
  assert.ok(launchArgs.includes('--renderer-process-limit=1'));
  assert.equal(launchArgs.some(argument => argument.includes('max-old-space-size=2048')), false);
  assert.equal(hasMemoryLaunchProfile(launchArgs, packagedDefault), true);
  assert.equal(hasMemoryLaunchProfile([], packagedDefault), false);
}

function checkElectronMemoryLimits({ buildMemoryRelaunchArgs, resolveMemoryProfile }, root) {
  const probeRoot = path.join(root, 'electron-memory-probe');
  mkdirSync(probeRoot, { recursive: true });
  writeFileSync(path.join(probeRoot, 'package.json'), JSON.stringify({
    name: 'ouro-electron-memory-probe',
    type: 'module',
    main: 'main.mjs',
  }));
  writeFileSync(path.join(probeRoot, 'main.mjs'), `
import v8 from 'node:v8';
import { app, BrowserWindow } from 'electron';

app.commandLine.appendSwitch('disable-gpu');
app.whenReady().then(async () => {
  const window = new BrowserWindow({
    show: false,
    webPreferences: { contextIsolation: true, nodeIntegration: false, sandbox: true },
  });
  await window.loadURL('data:text/html,<title>memory probe</title>');
  const rendererHeapLimit = await window.webContents.executeJavaScript('performance.memory?.jsHeapSizeLimit || 0');
  console.log('OURO_MEMORY_PROBE ' + JSON.stringify({
    mainHeapLimit: v8.getHeapStatistics().heap_size_limit,
    rendererHeapLimit,
    rendererProcessLimit: app.commandLine.getSwitchValue('renderer-process-limit'),
  }));
  app.exit(0);
}).catch(error => {
  console.error(error);
  app.exit(1);
});
`);

  const profile = resolveMemoryProfile('', true);
  const electronBinary = path.join(guiRoot, 'node_modules', 'electron', 'dist', process.platform === 'win32' ? 'electron.exe' : 'electron');
  const electronArgs = [...buildMemoryRelaunchArgs(['--no-sandbox'], profile), probeRoot];
  const command = process.platform === 'linux' ? 'xvfb-run' : electronBinary;
  const args = process.platform === 'linux' ? ['-a', electronBinary, ...electronArgs] : electronArgs;
  const probe = spawnSync(command, args, {
    cwd: guiRoot,
    encoding: 'utf8',
    timeout: 15000,
    env: {
      ...process.env,
      ELECTRON_DISABLE_SECURITY_WARNINGS: 'true',
      NODE_OPTIONS: undefined,
    },
  });
  assert.equal(probe.status, 0, [probe.error?.message, probe.stdout, probe.stderr].filter(Boolean).join('\n'));
  const marker = probe.stdout.split('\n').find(line => line.startsWith('OURO_MEMORY_PROBE '));
  assert.ok(marker, probe.stdout || probe.stderr);
  const result = JSON.parse(marker.slice('OURO_MEMORY_PROBE '.length));
  const maximumHeapLimit = 320 * 1024 * 1024;
  assert.ok(result.mainHeapLimit > 0 && result.mainHeapLimit <= maximumHeapLimit, `main heap limit ${result.mainHeapLimit}`);
  assert.ok(result.rendererHeapLimit > 0 && result.rendererHeapLimit <= maximumHeapLimit, `renderer heap limit ${result.rendererHeapLimit}`);
  assert.equal(result.rendererProcessLimit, '1');
}

async function checkPackagedBootstrapRelaunch(outputMain, root) {
  const fakeElectronPackage = path.join(root, 'node_modules', 'electron');
  mkdirSync(fakeElectronPackage, { recursive: true });
  writeFileSync(path.join(fakeElectronPackage, 'package.json'), JSON.stringify({
    name: 'electron',
    type: 'module',
    exports: './index.js',
  }));
  writeFileSync(path.join(fakeElectronPackage, 'index.js'), 'export const app = globalThis.__OURO_ELECTRON_BOOTSTRAP_CONTRACT__.app;\n');
  writeFileSync(path.join(outputMain, 'application-main.js'), 'globalThis.__OURO_ELECTRON_BOOTSTRAP_CONTRACT__.applicationLoads += 1;\n');

  const switches = new Map();
  const state = {
    applicationLoads: 0,
    exitCodes: [],
    relaunchArgs: [],
    app: {
      isPackaged: true,
      commandLine: {
        appendSwitch(name, value = '') {
          switches.set(name, value);
        },
      },
      exit(code) {
        state.exitCodes.push(code);
      },
      relaunch({ args }) {
        state.relaunchArgs.push(args);
      },
    },
  };

  const originalArgv = process.argv;
  const environmentKeys = ['OURO_DESKTOP_MEMORY_MODE', 'GOMEMLIMIT', 'GOGC'];
  const originalEnvironment = new Map(environmentKeys.map(key => [key, process.env[key]]));
  globalThis.__OURO_ELECTRON_BOOTSTRAP_CONTRACT__ = state;
  try {
    for (const key of environmentKeys) {
      delete process.env[key];
    }
    process.argv = [process.execPath, '--contract-user-argument'];
    const mainURL = pathToFileURL(path.join(outputMain, 'main.js')).href;
    await import(`${mainURL}?bootstrap=first`);

    assert.equal(state.applicationLoads, 0, 'the unconstrained packaged process must not load the application');
    assert.equal(state.relaunchArgs.length, 1);
    assert.equal(state.exitCodes.at(-1), 0);
    assert.ok(state.relaunchArgs[0].includes('--contract-user-argument'));
    assert.ok(state.relaunchArgs[0].includes('--ouro-desktop-memory-profile=low'));
    assert.equal(process.env.OURO_DESKTOP_MEMORY_MODE, 'low');
    assert.equal(process.env.GOMEMLIMIT, '320MiB');
    assert.equal(process.env.GOGC, '50');

    process.argv = [process.execPath, ...state.relaunchArgs[0]];
    await import(`${mainURL}?bootstrap=second`);
    assert.equal(state.relaunchArgs.length, 1, 'the constrained packaged process must not relaunch again');
    assert.equal(state.applicationLoads, 1, 'the constrained packaged process must load the application exactly once');
    assert.equal(switches.get('renderer-process-limit'), '1');
    assert.equal(switches.get('js-flags'), '--max-old-space-size=256 --max-semi-space-size=8');
  } finally {
    process.argv = originalArgv;
    for (const [key, value] of originalEnvironment) {
      if (value === undefined) {
        delete process.env[key];
      } else {
        process.env[key] = value;
      }
    }
    delete globalThis.__OURO_ELECTRON_BOOTSTRAP_CONTRACT__;
  }
}

async function checkBackendLifecycleContracts({ backendModule, resources, sidecar }) {
  if (process.platform === 'win32') {
    return;
  }

  const sidecarModule = path.join(resources, 'sidecar-contract.mjs');
  writeFileSync(sidecarModule, `
import { appendFileSync } from 'node:fs';
import { spawn } from 'node:child_process';
import http from 'node:http';

const args = process.argv.slice(2);
const portIndex = args.indexOf('--port');
const port = Number(args[portIndex + 1]);
appendFileSync(process.env.OURO_TEST_SIDECAR_LOG, JSON.stringify({
  pid: process.pid,
  workspace: args[1] || '',
  memoryMode: process.env.OURO_DESKTOP_MEMORY_MODE || '',
  goMemoryLimit: process.env.GOMEMLIMIT || '',
  goGC: process.env.GOGC || '',
}) + '\\n');

if (process.env.OURO_TEST_SIDECAR_DESCENDANT_LOG) {
  const descendant = spawn(process.execPath, ['-e', 'setInterval(() => {}, 1000)'], {
    stdio: 'ignore',
  });
  appendFileSync(process.env.OURO_TEST_SIDECAR_DESCENDANT_LOG, String(descendant.pid) + '\\n');
}

const server = http.createServer((request, response) => {
  response.writeHead(request.url === '/api/health' ? 200 : 404, { 'content-type': 'application/json' });
  response.end('{}');
});
setTimeout(() => server.listen(port, '127.0.0.1'), Number(process.env.OURO_TEST_SIDECAR_LISTEN_DELAY_MS || 0));
const crashAfterMs = Number(process.env.OURO_TEST_SIDECAR_CRASH_MS || 0);
if (crashAfterMs > 0) {
  setTimeout(() => process.exit(17), crashAfterMs);
}
const exitAfterMs = Number(process.env.OURO_TEST_SIDECAR_EXIT_MS || 0);
if (exitAfterMs > 0) {
  const exitCode = Number.parseInt(process.env.OURO_TEST_SIDECAR_EXIT_CODE || '0', 10);
  setTimeout(() => process.exit(Number.isInteger(exitCode) ? exitCode : 0), exitAfterMs);
}
process.on('SIGTERM', () => server.close(() => process.exit(0)));
`);
  writeFileSync(sidecar, `#!/bin/sh\nexec ${shellQuote(process.execPath)} ${shellQuote(sidecarModule)} "$@"\n`);
  chmodSync(sidecar, 0o755);

  await withEnvironment({
    OURO_DESKTOP_MEMORY_MODE: 'low',
    GOMEMLIMIT: '320MiB',
    GOGC: '50',
    OURO_DESKTOP_SIDECAR_RESTART_LIMIT: '2',
    OURO_TEST_SIDECAR_LISTEN_DELAY_MS: '150',
  }, async () => {
    await checkStartupExitDiagnosis(backendModule, resources);
    await checkStopDuringStart(backendModule, resources);
    await checkLatestWorkspaceWins(backendModule, resources);
    await checkRestartDuringStart(backendModule, resources);
    await checkDescendantCleanup(backendModule, resources);
    await checkExpectedStopSurvivesTerminationSettlement(backendModule, resources);
    await checkUnexpectedSignalPreservesCrash(backendModule, resources);
    await checkQuitPreventsStart(backendModule, resources);
    await checkRecoveryLimitAndEnvironment(backendModule, resources);
  });
}

async function checkUnexpectedSignalPreservesCrash({ BackendManager }, resources) {
  const previousLimit = process.env.OURO_DESKTOP_SIDECAR_RESTART_LIMIT;
  const { app, logPath } = contractApp(resources, 'unexpected-signal-crash');
  process.env.OURO_DESKTOP_SIDECAR_RESTART_LIMIT = '0';
  process.env.OURO_TEST_SIDECAR_LOG = logPath;
  const manager = new BackendManager({ app });
  try {
    await manager.start('/workspace/unexpected-signal-crash');
    const child = manager.process;
    assert.ok(child, 'unexpected-signal contract requires a running sidecar');
    assert.equal(child.kill('SIGINT'), true, 'unexpected-signal contract must signal the real sidecar');
    await waitFor(
      () => child.exitCode !== null || child.signalCode !== null,
      'unexpectedly signaled sidecar exit',
    );
    await waitFor(
      () => manager.snapshot().status !== 'ready',
      'unexpectedly signaled sidecar status',
    );
    const snapshot = manager.snapshot();
    assert.equal(snapshot.status, 'crashed', 'an unrequested sidecar signal must remain crashed');
    assert.equal(snapshot.lastExitSignal, 'SIGINT');
    assert.match(snapshot.lastError, /SIGINT/);
  } finally {
    if (previousLimit === undefined) {
      delete process.env.OURO_DESKTOP_SIDECAR_RESTART_LIMIT;
    } else {
      process.env.OURO_DESKTOP_SIDECAR_RESTART_LIMIT = previousLimit;
    }
    await manager.stop();
  }
}

async function checkExpectedStopSurvivesTerminationSettlement({ BackendManager }, resources) {
  const { app, logPath } = contractApp(resources, 'expected-stop-race');
  process.env.OURO_TEST_SIDECAR_LOG = logPath;
  process.env.OURO_TEST_SIDECAR_EXIT_MS = '1500';
  process.env.OURO_TEST_SIDECAR_EXIT_CODE = '0';
  const manager = new BackendManager({ app });
  try {
    await manager.start('/workspace/expected-stop-race');
    const child = manager.process;
    assert.ok(child, 'expected-stop race contract requires a running sidecar');
    manager.markExpectedStop('contract-termination-settled');
    manager.stoppingProcess = null;
    await waitFor(
      () => child.exitCode !== null || child.signalCode !== null,
      'expected-stop race sidecar exit',
    );
    const records = readFileSync(manager.diagnosticLogPath, 'utf8')
      .split('\n')
      .filter(Boolean)
      .map(line => JSON.parse(line));
    const exit = records.filter(record => record.event === 'backend.exit').pop();
    assert.equal(exit?.expectedStop, true, 'expected stop must survive termination promise settlement');
  } finally {
    delete process.env.OURO_TEST_SIDECAR_EXIT_MS;
    delete process.env.OURO_TEST_SIDECAR_EXIT_CODE;
    await manager.stop();
  }
}

async function checkDescendantCleanup({ BackendManager }, resources) {
  const { app, logPath } = contractApp(resources, 'descendant-cleanup');
  const descendantLogPath = path.join(resources, 'contract-user-data', 'descendant-cleanup', 'descendant.pid');
  process.env.OURO_TEST_SIDECAR_LOG = logPath;
  process.env.OURO_TEST_SIDECAR_DESCENDANT_LOG = descendantLogPath;
  const manager = new BackendManager({ app });
  let descendantPID = 0;
  try {
    await manager.start('/workspace/descendant-cleanup');
    await waitFor(() => existsSync(descendantLogPath), 'sidecar descendant PID');
    descendantPID = Number.parseInt(readFileSync(descendantLogPath, 'utf8').trim(), 10);
    assert.ok(Number.isSafeInteger(descendantPID) && descendantPID > 1);
    await manager.stop();
    await waitFor(() => !processIsRunning(descendantPID), 'sidecar descendant cleanup');
  } finally {
    delete process.env.OURO_TEST_SIDECAR_DESCENDANT_LOG;
    await manager.stop();
    if (processIsRunning(descendantPID)) {
      process.kill(descendantPID, 'SIGKILL');
    }
  }
}

async function checkStartupExitDiagnosis({ BackendManager }, resources) {
  const previousLimit = process.env.OURO_DESKTOP_SIDECAR_RESTART_LIMIT;
  process.env.OURO_DESKTOP_SIDECAR_RESTART_LIMIT = '0';
  try {
    await checkStartupExitCase({
      BackendManager,
      resources,
      label: 'startup-crash-before-health',
      exitCode: '17',
      expectedStatus: 'crashed',
      expectedError: '后端进程非预期退出',
    });
    await checkStartupExitCase({
      BackendManager,
      resources,
      label: 'startup-clean-exit-before-health',
      exitCode: '0',
      expectedStatus: 'offline',
      expectedError: '',
    });
  } finally {
    if (previousLimit === undefined) {
      delete process.env.OURO_DESKTOP_SIDECAR_RESTART_LIMIT;
    } else {
      process.env.OURO_DESKTOP_SIDECAR_RESTART_LIMIT = previousLimit;
    }
  }
}

async function checkStartupExitCase({ BackendManager, resources, label, exitCode, expectedStatus, expectedError }) {
  const { app, logPath } = contractApp(resources, label);
  process.env.OURO_TEST_SIDECAR_LOG = logPath;
  process.env.OURO_TEST_SIDECAR_LISTEN_DELAY_MS = '10000';
  process.env.OURO_TEST_SIDECAR_EXIT_MS = '100';
  process.env.OURO_TEST_SIDECAR_EXIT_CODE = exitCode;
  const manager = new BackendManager({ app });
  try {
    await assert.rejects(manager.start(`/workspace/${label}`));
    const snapshot = manager.snapshot();
    assert.equal(snapshot.status, expectedStatus, `${label} must preserve the process exit status`);
    assert.equal(snapshot.lastError.includes('后端健康检查失败'), false, `${label} must not replace exit diagnosis with health failure`);
    if (expectedError) {
      assert.ok(snapshot.lastError.includes(expectedError), snapshot.lastError);
    } else {
      assert.equal(snapshot.lastError, '', snapshot.lastError);
    }
  } finally {
    delete process.env.OURO_TEST_SIDECAR_EXIT_MS;
    delete process.env.OURO_TEST_SIDECAR_EXIT_CODE;
    process.env.OURO_TEST_SIDECAR_LISTEN_DELAY_MS = '150';
    await manager.stop();
  }
}

async function checkDevelopmentCommandDiscovery({ BackendManager, ensurePackagedSidecarExecutable }, resources) {
  assert.deepEqual(
    ensurePackagedSidecarExecutable({ isPackaged: false }),
    { path: '', exists: false, executable: false, repaired: false },
    'development mode must not claim a packaged sidecar exists',
  );
  const fakeGo = path.join(resources, 'go');
  writeFileSync(fakeGo, '#!/bin/sh\nexit 0\n');
  chmodSync(fakeGo, 0o755);
  const previousPath = process.env.PATH;
  process.env.PATH = resources;
  const app = new EventEmitter();
  app.isPackaged = false;
  app.getPath = name => name === 'userData' ? resources : process.execPath;
  app.getVersion = () => 'contract';
  app.getAppPath = () => resources;
  const manager = new BackendManager({ app });
  try {
    assert.equal(manager.runtimeInfo().backendBinaryExists, true, 'development Go command must be resolved through PATH');
  } finally {
    await manager.stop();
    if (previousPath === undefined) {
      delete process.env.PATH;
    } else {
      process.env.PATH = previousPath;
    }
  }
}

async function checkStopDuringStart({ BackendManager }, resources) {
  const { app, logPath } = contractApp(resources, 'stop-during-start');
  process.env.OURO_TEST_SIDECAR_LOG = logPath;
  const manager = new BackendManager({ app });
  try {
    const startResult = manager.start('/workspace/stop-during-start');
    assert.equal(manager.snapshot().running, true, 'a starting sidecar must keep before-quit cleanup active');
    const stopResult = manager.stop();
    await Promise.allSettled([startResult, stopResult]);
    const snapshot = manager.snapshot();
    assert.equal(snapshot.status, 'offline', 'stop during start must leave the sidecar offline');
    assert.equal(snapshot.running, false, 'stop during start must not leave a child process running');
    assert.equal(manager.desiredWorkspace, '', 'stop during start must clear the recovery workspace');
  } finally {
    await manager.stop();
  }
}

async function checkLatestWorkspaceWins({ BackendManager }, resources) {
  const { app, logPath } = contractApp(resources, 'latest-workspace');
  process.env.OURO_TEST_SIDECAR_LOG = logPath;
  const manager = new BackendManager({ app });
  try {
    const firstStart = manager.start('/workspace/first');
    await waitFor(() => readSidecarStarts(logPath).length === 1, 'first sidecar launch');
    const latestStart = manager.start('/workspace/latest');
    await Promise.all([firstStart, latestStart]);
    const snapshot = manager.snapshot();
    assert.equal(snapshot.status, 'ready');
    assert.equal(snapshot.workspace, '/workspace/latest', 'the latest concurrent start request must win');
    assert.deepEqual(readSidecarStarts(logPath).map(start => start.workspace), [
      '/workspace/first',
      '/workspace/latest',
    ]);
  } finally {
    await manager.stop();
  }
}

async function checkRestartDuringStart({ BackendManager }, resources) {
  const { app, logPath } = contractApp(resources, 'restart-during-start');
  process.env.OURO_TEST_SIDECAR_LOG = logPath;
  const manager = new BackendManager({ app });
  try {
    const firstStart = manager.start('/workspace/restart-during-start');
    await waitFor(() => readSidecarStarts(logPath).length === 1, 'sidecar launch before restart');
    const restart = manager.restart();
    await Promise.all([firstStart, restart]);
    assert.equal(manager.snapshot().status, 'ready');
    assert.equal(readSidecarStarts(logPath).length, 2, 'restart during start must perform a fresh second launch');
  } finally {
    await manager.stop();
  }
}

async function checkQuitPreventsStart({ BackendManager }, resources) {
  const { app, logPath } = contractApp(resources, 'quit-prevents-start');
  process.env.OURO_TEST_SIDECAR_LOG = logPath;
  const manager = new BackendManager({ app });
  try {
    app.emit('before-quit');
    await manager.start('/workspace/after-quit');
    assert.equal(manager.snapshot().running, false, 'an app that is quitting must not launch the sidecar');
    assert.equal(readSidecarStarts(logPath).length, 0);
  } finally {
    await manager.stop();
  }
}

async function checkRecoveryLimitAndEnvironment({ BackendManager }, resources) {
  const { app, logPath } = contractApp(resources, 'recovery-limit');
  process.env.OURO_TEST_SIDECAR_LOG = logPath;
  process.env.OURO_TEST_SIDECAR_CRASH_MS = '550';
  const manager = new BackendManager({ app });
  try {
    await manager.start('/workspace/recovery-limit');
    const initialStart = readSidecarStarts(logPath)[0];
    assert.deepEqual({
      memoryMode: initialStart.memoryMode,
      goMemoryLimit: initialStart.goMemoryLimit,
      goGC: initialStart.goGC,
    }, {
      memoryMode: 'low',
      goMemoryLimit: '320MiB',
      goGC: '50',
    }, 'the real sidecar process must inherit the selected memory profile');

    await waitFor(() => {
      const snapshot = manager.snapshot();
      return snapshot.status === 'crashed'
        && snapshot.sidecarRecovery.attempts === 2
        && snapshot.sidecarRecovery.pending === false;
    }, 'bounded sidecar recovery exhaustion', 7000);

    const snapshot = manager.snapshot();
    assert.equal(readSidecarStarts(logPath).length, 3, 'restart limit 2 must allow exactly two automatic recoveries');
    assert.equal(snapshot.sidecarRecovery.attempts, 2);
    assert.ok(readFileSync(snapshot.diagnosticLogPath, 'utf8').includes('sidecar.recovery_exhausted'));
  } finally {
    delete process.env.OURO_TEST_SIDECAR_CRASH_MS;
    await manager.stop();
  }
}

function contractApp(resources, label) {
  const userData = path.join(resources, 'contract-user-data', label);
  mkdirSync(userData, { recursive: true });
  return {
    app: new ContractApp(userData),
    logPath: path.join(userData, 'sidecar-starts.jsonl'),
  };
}

function readSidecarStarts(logPath) {
  if (!existsSync(logPath)) {
    return [];
  }
  return readFileSync(logPath, 'utf8')
    .split('\n')
    .filter(Boolean)
    .map(line => JSON.parse(line));
}

function processIsRunning(processId) {
  if (!Number.isSafeInteger(processId) || processId <= 1) {
    return false;
  }
  try {
    process.kill(processId, 0);
  } catch {
    return false;
  }
  if (process.platform !== 'linux') {
    return true;
  }
  try {
    const stat = readFileSync(`/proc/${processId}/stat`, 'utf8');
    const commandEnd = stat.lastIndexOf(')');
    const state = commandEnd < 0 ? '' : stat.slice(commandEnd + 1).trim().split(/\s+/)[0];
    return state !== 'Z' && state !== 'X';
  } catch {
    return false;
  }
}

async function waitFor(predicate, label, timeoutMs = 3000) {
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    if (predicate()) {
      return;
    }
    await new Promise(resolve => setTimeout(resolve, 10));
  }
  assert.fail(`timed out waiting for ${label}`);
}

async function withEnvironment(values, callback) {
  const previous = new Map(Object.keys(values).map(key => [key, process.env[key]]));
  try {
    for (const [key, value] of Object.entries(values)) {
      process.env[key] = value;
    }
    await callback();
  } finally {
    for (const [key, value] of previous) {
      if (value === undefined) {
        delete process.env[key];
      } else {
        process.env[key] = value;
      }
    }
    delete process.env.OURO_TEST_SIDECAR_LOG;
  }
}

function shellQuote(value) {
  return `'${String(value).replaceAll("'", "'\\''")}'`;
}
