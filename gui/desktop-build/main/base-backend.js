import { spawn } from 'node:child_process';
import { EventEmitter } from 'node:events';
import fs from 'node:fs';
import net from 'node:net';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);
const BACKEND_GRACEFUL_TERMINATION_MS = 15000;
const BACKEND_FORCE_TERMINATION_MS = 20000;
const BACKEND_TERMINATION_POLL_MS = 50;
const BACKEND_PROCESS_GROUP = Symbol('ouroborosBackendProcessGroup');
const BACKEND_EXPECTED_STOP = Symbol('ouroborosBackendExpectedStop');

class BackendStartupExitError extends Error {
  constructor(code, signal, cause) {
    const normalizedCode = normalizeExitCode(code);
    const detail = signal
      ? `信号 ${signal}`
      : normalizedCode === null
        ? '未记录退出码'
        : `退出码 ${normalizedCode}`;
    super(`后端进程在健康检查完成前退出（${detail}）`);
    this.name = 'BackendStartupExitError';
    this.exitCode = normalizedCode;
    this.signal = signal || '';
    this.cause = cause;
  }
}

export function isBackendStartupExitError(error) {
  return error instanceof BackendStartupExitError;
}

export class BackendManager extends EventEmitter {
  constructor({ app }) {
    super();
    this.app = app;
    this.process = null;
    this.apiBaseURL = '';
    this.workspace = '';
    this.status = 'offline';
    this.lastError = '';
    this.stoppingProcess = null;
    this.stoppingPromise = null;
    this.failedStartupProcess = null;
    this.backendBinaryPath = '';
    this.backendBinaryExists = false;
    this.backendWorkingDirectory = '';
    this.backendWorkingDirectoryExists = false;
    this.diagnosticLogPath = path.join(app.getPath('userData'), 'backend.log');
    this.desktopRuntimeStatePath = path.join(app.getPath('userData'), 'desktop-runtime-state.json');
    this.desktopRuntimeStateTempPath = path.join(app.getPath('userData'), 'desktop-runtime-state.json.tmp');
    this.desktopRuntimeSessionId = `${Date.now()}-${process.pid}`;
    this.desktopRuntimeProcessStartToken = readProcessStartToken(process.pid);
    this.desktopRuntimeStartedAt = '';
    this.desktopRuntimeHeartbeatTimer = null;
    this.desktopRuntimeFinalStatus = '';
    this.lastDesktopRuntimeIssue = null;
    this.lastCommandLine = '';
    this.lastExitCode = null;
    this.lastExitSignal = '';
    this.lastStartedAt = '';
    this.lastStdout = '';
    this.lastStderr = '';
    this.lastHealthError = '';
    this.lastEvent = '';
    this.wireAppQuitExpectedStop();
  }

  async start(workspace) {
    if (!workspace || workspace.trim() === '') {
      this.status = 'offline';
      this.clearBackendConnection();
      this.emitStatus();
      return this.snapshot();
    }
    if (this.process && this.workspace === workspace && this.apiBaseURL) {
      return this.snapshot();
    }

    await this.stop();
    const port = await choosePort();
    const command = backendCommand(this.app);
    const args = backendArgs(this.app, workspace, port);
    const cwd = backendWorkingDirectory(this.app);
    this.backendBinaryPath = command;
    this.backendBinaryExists = fileExists(command);
    this.backendWorkingDirectory = cwd;
    this.backendWorkingDirectoryExists = directoryExists(cwd);
    this.lastCommandLine = formatCommandLine(command, args);
    this.lastExitCode = null;
    this.lastExitSignal = '';
    this.lastStartedAt = new Date().toISOString();
    this.lastStdout = '';
    this.lastStderr = '';
    this.lastHealthError = '';

    this.workspace = workspace;
    this.apiBaseURL = `http://127.0.0.1:${port}`;
    this.status = 'starting';
    this.lastError = '';
    this.emitStatus();
    this.writeDiagnostic('backend.start', {
      command,
      args,
      cwd,
      workspace,
      apiBaseURL: this.apiBaseURL,
      backendBinaryExists: this.backendBinaryExists,
      backendWorkingDirectoryExists: this.backendWorkingDirectoryExists,
    });
    const useProcessGroup = process.platform !== 'win32';
    this.process = spawn(command, args, {
      cwd,
      stdio: ['ignore', 'pipe', 'pipe'],
      env: process.env,
      detached: useProcessGroup,
    });

    const child = this.process;
    if (useProcessGroup && Number.isSafeInteger(child.pid) && child.pid > 1) {
      child[BACKEND_PROCESS_GROUP] = child.pid;
    }
    child.stdout.on('data', chunk => {
      const message = chunk.toString().trim();
      if (message) {
        this.lastStdout = message.slice(-2000);
        this.writeDiagnostic('backend.stdout', { pid: child.pid, message: this.lastStdout });
      }
    });
    child.stderr.on('data', chunk => {
      const message = chunk.toString().trim();
      if (message) {
        this.lastStderr = message.slice(-2000);
        this.writeDiagnostic('backend.stderr', { pid: child.pid, message: this.lastStderr });
      }
    });
    child.once('error', error => {
      if (this.process !== child) {
        return;
      }
      this.process = null;
      this.clearBackendConnection();
      this.status = 'crashed';
      this.lastError = describeBackendSpawnFailure(error, command, cwd, this.diagnosticLogPath);
      this.writeDiagnostic('backend.error', {
        pid: child.pid,
        ...normalizeSpawnErrorDetails(error, command, cwd),
      });
      this.emitStatus();
    });
    child.once('exit', (code, signal) => {
      const expectedStop = this.stoppingProcess === child || Boolean(child[BACKEND_EXPECTED_STOP]);
      const startupHealthFailed = this.failedStartupProcess === child;
      const normalizedCode = this.recordProcessExit(child, code, signal, { expectedStop, startupHealthFailed });
      this.cleanupExitedProcessGroup(child, expectedStop);
      if (startupHealthFailed) {
        return;
      }
      if (this.process === child) {
        this.process = null;
      }
      if (this.stoppingProcess === child) {
        this.stoppingProcess = null;
      }
      if (expectedStop) {
        this.clearBackendConnection();
        this.status = 'offline';
        this.emitStatus();
        return;
      }
      if (isCleanBackendExit(normalizedCode, signal)) {
        this.writeDiagnostic('backend.clean_exit', {
          pid: child.pid,
          code: normalizedCode,
          signal: signal || '',
          workspace: this.workspace,
          apiBaseURL: this.apiBaseURL,
          status: this.status,
        });
        this.clearBackendConnection();
        this.status = 'offline';
        this.lastError = '';
        this.emitStatus();
        return;
      }
      this.clearBackendConnection();
      this.status = 'crashed';
      this.lastError = describeBackendFailure(normalizedCode, signal, this.lastStderr, this.lastStdout, this.diagnosticLogPath);
      this.emitStatus();
    });

    try {
      await waitForHealth(this.apiBaseURL, child);
    } catch (error) {
      if (error instanceof BackendStartupExitError) {
        this.writeDiagnostic('backend.health_aborted', {
          pid: child.pid,
          code: error.exitCode,
          signal: error.signal,
          message: error.message,
          cause: error.cause?.message || '',
        });
        throw error;
      }
      const healthFailure = describeBackendHealthFailure(error, command, args, cwd, this.apiBaseURL, this.lastStderr, this.lastStdout, this.diagnosticLogPath);
      this.failedStartupProcess = child;
      this.process = null;
      this.lastHealthError = describeHealthError(error);
      this.writeDiagnostic('backend.health_failed', {
        pid: child.pid,
        message: error.message,
        apiBaseURL: this.apiBaseURL,
        command,
        args,
        cwd,
        stdout: this.lastStdout,
        stderr: this.lastStderr,
      });
      await this.terminateBackendProcess(child);
      this.failedStartupProcess = null;
      this.clearBackendConnection();
      this.status = 'crashed';
      this.lastError = healthFailure;
      this.emitStatus();
      throw error;
    }
    this.status = 'ready';
    this.emitStatus();
    return this.snapshot();
  }

  async initializeWorkspace(workspace) {
    if (!workspace || workspace.trim() === '') {
      throw new Error('工作区路径不能为空');
    }
    const command = backendCommand(this.app);
    const args = backendInitArgs(this.app, workspace);
    await runBackendCommand(command, args, backendWorkingDirectory(this.app), this.diagnosticLogPath);
    return { workspace };
  }

  async stop() {
    if (!this.process) {
      await this.stoppingPromise;
      this.clearBackendConnection();
      this.status = 'offline';
      this.emitStatus();
      return;
    }
    const child = this.process;
    this.process = null;
    await this.terminateBackendProcess(child);
    this.clearBackendConnection();
    this.status = 'offline';
    this.emitStatus();
  }

  async terminateBackendProcess(child) {
    if (child) {
      child[BACKEND_EXPECTED_STOP] = true;
    }
    if (this.stoppingProcess === child && this.stoppingPromise) {
      await this.stoppingPromise;
      return;
    }

    this.stoppingProcess = child;
    const stoppingPromise = terminateProcess(child);
    this.stoppingPromise = stoppingPromise;
    try {
      const result = await stoppingPromise;
      if (!result.terminated) {
        this.writeDiagnostic('backend.termination_incomplete', {
          pid: child?.pid ?? 0,
          processGroupId: result.processGroupId,
          forced: result.forced,
        });
      }
      return result;
    } finally {
      if (this.stoppingProcess === child) {
        this.stoppingProcess = null;
      }
      if (this.stoppingPromise === stoppingPromise) {
        this.stoppingPromise = null;
      }
    }
  }

  cleanupExitedProcessGroup(child, expectedStop) {
    if (expectedStop || !backendProcessGroupMayBeAlive(child)) {
      return;
    }
    queueMicrotask(() => {
      if (!backendProcessGroupMayBeAlive(child)) {
        return;
      }
      this.writeDiagnostic('backend.process_group_lingering', {
        pid: child?.pid ?? 0,
        processGroupId: backendProcessGroupId(child),
      });
      void this.terminateBackendProcess(child);
    });
  }

  markExpectedStop(reason) {
    const child = this.process;
    if (!child || child.exitCode !== null || child.signalCode !== null) {
      return;
    }
    child[BACKEND_EXPECTED_STOP] = true;
    this.stoppingProcess = child;
    this.writeDiagnostic('backend.expected_stop', {
      reason,
      pid: child.pid,
      status: this.status,
      workspace: this.workspace,
      apiBaseURL: this.apiBaseURL,
    });
  }

  wireAppQuitExpectedStop() {
    const mark = reason => {
      this.markExpectedStop(reason);
    };
    this.app.on('before-quit', () => mark('before-quit'));
    this.app.on('will-quit', () => mark('will-quit'));
    this.app.on('quit', () => mark('quit'));
  }

  async restart() {
    const workspace = this.workspace;
    if (!workspace || workspace.trim() === '') {
      this.clearBackendConnection();
      this.status = 'offline';
      this.emitStatus();
      return this.snapshot();
    }
    await this.stop();
    return this.start(workspace);
  }

  emitStatus() {
    const snapshot = this.snapshot();
    if (this.desktopRuntimeStartedAt) {
      this.writeDesktopRuntimeState('running', { reason: 'backend-status' });
    }
    this.emit('status', snapshot);
    return snapshot;
  }

  clearBackendConnection() {
    this.apiBaseURL = '';
  }

  snapshot() {
    return {
      status: this.status,
      apiBaseURL: this.apiBaseURL,
      workspace: this.workspace,
      running: Boolean(this.process || this.stoppingProcess || this.stoppingPromise),
      lastError: this.lastError,
      backendBinaryPath: this.backendBinaryPath,
      backendBinaryExists: this.backendBinaryExists,
      backendWorkingDirectory: this.backendWorkingDirectory,
      backendWorkingDirectoryExists: this.backendWorkingDirectoryExists,
      diagnosticLogPath: this.diagnosticLogPath,
      lastEvent: this.lastEvent,
      desktopRuntimeStatePath: this.desktopRuntimeStatePath,
      desktopRuntimeSessionId: this.desktopRuntimeSessionId,
      desktopRuntimeStartedAt: this.desktopRuntimeStartedAt,
      lastDesktopRuntimeIssue: this.lastDesktopRuntimeIssue,
      processId: this.process?.pid ?? this.stoppingProcess?.pid ?? 0,
      lastCommandLine: this.lastCommandLine,
      lastExitCode: this.lastExitCode,
      lastExitSignal: this.lastExitSignal,
      lastStartedAt: this.lastStartedAt,
      lastHealthError: this.lastHealthError,
      lastStdout: this.lastStdout,
      lastStderr: this.lastStderr,
    };
  }

  runtimeInfo() {
    const command = backendCommand(this.app);
    const cwd = backendWorkingDirectory(this.app);
    const rendererPath = path.join(__dirname, '../renderer/index.html');
    const preloadPath = path.join(__dirname, 'preload.js');
    return {
      isPackaged: this.app.isPackaged,
      platform: process.platform,
      arch: process.arch,
      appVersion: this.app.getVersion(),
      appPath: this.app.getAppPath(),
      executablePath: this.app.getPath('exe'),
      resourcesPath: process.resourcesPath || '',
      userDataPath: this.app.getPath('userData'),
      backendBinaryPath: command,
      backendBinaryExists: fileExists(command),
      backendWorkingDirectory: cwd,
      backendWorkingDirectoryExists: directoryExists(cwd),
      rendererPath,
      rendererExists: fileExists(rendererPath),
      preloadPath,
      preloadExists: fileExists(preloadPath),
      diagnosticLogPath: this.diagnosticLogPath,
      diagnosticLogExists: fileExists(this.diagnosticLogPath),
      lastEvent: this.lastEvent,
      desktopRuntimeStatePath: this.desktopRuntimeStatePath,
      desktopRuntimeSessionId: this.desktopRuntimeSessionId,
      desktopRuntimeStartedAt: this.desktopRuntimeStartedAt,
      lastDesktopRuntimeIssue: this.lastDesktopRuntimeIssue,
      processId: this.process?.pid ?? this.stoppingProcess?.pid ?? 0,
      lastCommandLine: this.lastCommandLine,
      lastExitCode: this.lastExitCode,
      lastExitSignal: this.lastExitSignal,
      lastStartedAt: this.lastStartedAt,
      lastHealthError: this.lastHealthError,
      lastStdout: this.lastStdout,
      lastStderr: this.lastStderr,
    };
  }

  writeDiagnostic(event, details = {}) {
    this.lastEvent = event;
    appendDiagnostic(this.diagnosticLogPath, event, details);
  }

  startDesktopRuntimeSession() {
    if (this.desktopRuntimeHeartbeatTimer) {
      return;
    }
    this.desktopRuntimeFinalStatus = '';
    this.desktopRuntimeStartedAt = new Date().toISOString();
    const previous = readJSONFile(this.desktopRuntimeStatePath);
    if (previous?.status === 'running' && previous.sessionId !== this.desktopRuntimeSessionId) {
      const previousProcessId = normalizeProcessID(previous.processId);
      this.recordDesktopRuntimeIssue(
        isSameDesktopRuntimeProcess(previous) ? 'desktop.previous_runtime_still_marked_running' : 'desktop.previous_unclean_exit',
        {
          statePath: this.desktopRuntimeStatePath,
          previousSessionId: stringOrEmpty(previous.sessionId),
          previousProcessId,
          previousProcessStartToken: stringOrEmpty(previous.processStartToken),
          previousExecutablePath: stringOrEmpty(previous.executablePath),
          previousStartedAt: stringOrEmpty(previous.startedAt),
          previousHeartbeatAt: stringOrEmpty(previous.heartbeatAt),
          previousWorkspace: stringOrEmpty(previous.workspace),
          previousStatus: stringOrEmpty(previous.status),
        },
      );
    }
    this.writeDesktopRuntimeState('running', { reason: 'startup' });
    this.desktopRuntimeHeartbeatTimer = setInterval(() => {
      this.writeDesktopRuntimeState('running', { reason: 'heartbeat' });
    }, 10000);
    this.desktopRuntimeHeartbeatTimer.unref?.();
  }

  stopDesktopRuntimeSession(reason, status = 'clean') {
    if (this.desktopRuntimeHeartbeatTimer) {
      clearInterval(this.desktopRuntimeHeartbeatTimer);
      this.desktopRuntimeHeartbeatTimer = null;
    }
    const finalStatus = status === 'crashed' || this.desktopRuntimeFinalStatus === 'crashed'
      ? 'crashed'
      : 'clean';
    this.desktopRuntimeFinalStatus = finalStatus;
    this.writeDesktopRuntimeState(finalStatus, { reason });
  }

  recordDesktopRuntimeIssue(event, details = {}) {
    const issue = normalizeDesktopRuntimeIssue(event, details);
    this.lastDesktopRuntimeIssue = issue;
    this.writeDiagnostic(event, issue);
    return issue;
  }

  writeDesktopRuntimeState(status, details = {}) {
    try {
      fs.mkdirSync(path.dirname(this.desktopRuntimeStatePath), { recursive: true });
      fs.writeFileSync(this.desktopRuntimeStateTempPath, JSON.stringify({
        status,
        sessionId: this.desktopRuntimeSessionId,
        processId: process.pid,
        processStartToken: this.desktopRuntimeProcessStartToken,
        executablePath: process.execPath,
        startedAt: this.desktopRuntimeStartedAt,
        heartbeatAt: new Date().toISOString(),
        workspace: this.workspace,
        apiBaseURL: this.apiBaseURL,
        backendStatus: this.status,
        ...details,
      }, null, 2) + '\n');
      fs.renameSync(this.desktopRuntimeStateTempPath, this.desktopRuntimeStatePath);
    } catch {
      try {
        fs.rmSync(this.desktopRuntimeStateTempPath, { force: true });
      } catch {
        // Runtime state is diagnostic data only.
      }
      // Runtime state is diagnostic data only.
    }
  }

  recordProcessExit(child, code, signal, details = {}) {
    const normalizedCode = normalizeExitCode(code);
    this.lastExitCode = normalizedCode;
    this.lastExitSignal = signal || '';
    this.writeDiagnostic('backend.exit', {
      pid: child.pid,
      code: this.lastExitCode,
      rawCode: typeof code === 'number' ? code : null,
      signal: this.lastExitSignal,
      expectedStop: Boolean(details.expectedStop),
      startupHealthFailed: Boolean(details.startupHealthFailed),
    });
    return normalizedCode;
  }
}

export function normalizeExitCode(code) {
  return typeof code === 'number' && Number.isInteger(code) && code >= 0 ? code : null;
}

export function isCleanBackendExit(code, signal, expectedStop = false) {
  return expectedStop || code === 0;
}

function describeExit(code, signal) {
  if (signal) {
    return `后端进程非预期收到 ${signal} 后退出`;
  }
  if (code === null) {
    return '后端进程非预期退出，未记录退出码';
  }
  return `后端进程非预期退出，退出码 ${code}`;
}

function describeBackendFailure(code, signal, stderr, stdout, diagnosticLogPath) {
  const output = (stderr || stdout || '').trim();
  const exit = describeExit(code, signal);
  const logHint = diagnosticLogPath ? `；诊断日志 ${diagnosticLogPath}` : '';
  return output ? `${exit}；${output}${logHint}` : `${exit}${logHint}`;
}

export function describeBackendSpawnFailure(error, command, cwd, diagnosticLogPath) {
  const details = normalizeSpawnErrorDetails(error, command, cwd);
  const parts = [
    '后端进程启动失败',
    details.message,
    details.code ? `错误码 ${details.code}` : '',
    details.syscall ? `系统调用 ${details.syscall}` : '',
    details.path ? `路径 ${details.path}` : '',
    details.command ? `命令 ${details.command}` : '',
    details.cwd ? `工作目录 ${details.cwd}` : '',
    diagnosticLogPath ? `诊断日志 ${diagnosticLogPath}` : '',
  ].filter(Boolean);
  return parts.join('；');
}

export function describeBackendHealthFailure(error, command, args, cwd, apiBaseURL, stderr, stdout, diagnosticLogPath) {
  const output = (stderr || stdout || '').trim().slice(-2000);
  const parts = [
    '后端健康检查失败',
    describeHealthError(error),
    apiBaseURL ? `API ${apiBaseURL}` : '',
    command ? `命令 ${formatCommandLine(command, Array.isArray(args) ? args : [])}` : '',
    cwd ? `工作目录 ${cwd}` : '',
    output ? `最近输出 ${output}` : '',
    diagnosticLogPath ? `诊断日志 ${diagnosticLogPath}` : '',
  ].filter(Boolean);
  return parts.join('；');
}

function describeBackendCommandExit(code) {
  return code === null
    ? '后端命令退出，未记录退出码'
    : `后端命令退出，退出码 ${code}`;
}

function describeHealthError(error) {
  const message = error?.message ? String(error.message).slice(-2000) : String(error || '');
  if (/timed\s*out/i.test(message) || message.includes('后端健康检查超时')) {
    return '后端健康检查超时';
  }
  const status = message.match(/(?:returned|返回)\s+(?:HTTP\s+)?(\d{3})/i);
  if (status) {
    return `后端健康检查返回 HTTP ${status[1]}`;
  }
  if (!message) {
    return '后端健康检查请求失败';
  }
  return `后端健康检查请求失败：${message}`;
}

function terminateProcess(child) {
  return new Promise(resolve => {
    const processGroupId = backendProcessGroupId(child);
    if (!backendProcessTreeMayBeAlive(child)) {
      resolve({ terminated: true, forced: false, processGroupId });
      return;
    }
    let resolved = false;
    let forced = false;
    let killTimer = null;
    let forceTimer = null;
    let pollTimer = null;
    const finish = terminated => {
      if (resolved) {
        return;
      }
      resolved = true;
      clearTimeout(killTimer);
      clearTimeout(forceTimer);
      clearInterval(pollTimer);
      child?.removeListener('exit', checkFinished);
      resolve({ terminated, forced, processGroupId });
    };
    const checkFinished = () => {
      if (!backendProcessTreeMayBeAlive(child)) {
        finish(true);
      }
    };
    killTimer = setTimeout(() => {
      forced = true;
      signalBackendProcessTree(child, 'SIGKILL');
      checkFinished();
    }, BACKEND_GRACEFUL_TERMINATION_MS);
    forceTimer = setTimeout(() => finish(!backendProcessTreeMayBeAlive(child)), BACKEND_FORCE_TERMINATION_MS);
    pollTimer = setInterval(checkFinished, BACKEND_TERMINATION_POLL_MS);
    child?.once('exit', checkFinished);
    signalBackendProcessTree(child, 'SIGTERM');
    checkFinished();
  });
}

function signalBackendProcessTree(child, signal) {
  const processGroupId = backendProcessGroupId(child);
  if (processGroupId) {
    try {
      process.kill(-processGroupId, signal);
      return;
    } catch (error) {
      if (error?.code !== 'ESRCH') {
        // Fall back to the direct child when process-group signaling is unavailable.
      }
    }
  }
  try {
    if (child && child.exitCode === null && child.signalCode === null) {
      child.kill(signal);
    }
  } catch {
    // The polling loop resolves once the process tree is gone.
  }
}

function backendProcessTreeMayBeAlive(child) {
  const directChildMayBeAlive = child
    && child.exitCode === null
    && child.signalCode === null
    && processMayBeAlive(child.pid);
  return Boolean(directChildMayBeAlive || backendProcessGroupMayBeAlive(child));
}

function backendProcessGroupId(child) {
  const processGroupId = child?.[BACKEND_PROCESS_GROUP];
  return Number.isSafeInteger(processGroupId) && processGroupId > 1 ? processGroupId : 0;
}

function backendProcessGroupMayBeAlive(child) {
  const processGroupId = backendProcessGroupId(child);
  if (!processGroupId || process.platform === 'win32') {
    return false;
  }
  if (process.platform === 'linux') {
    const liveGroup = linuxProcessGroupHasLiveMember(processGroupId);
    if (liveGroup !== null) {
      return liveGroup;
    }
  }
  try {
    process.kill(-processGroupId, 0);
    return true;
  } catch (error) {
    return error?.code === 'EPERM';
  }
}

function backendCommand(app) {
  if (!app.isPackaged) {
    return 'go';
  }
  return process.platform === 'win32'
    ? path.join(process.resourcesPath, 'ouro.exe')
    : path.join(process.resourcesPath, 'ouro');
}

function backendWorkingDirectory(app) {
  if (app.isPackaged) {
    return process.resourcesPath || path.dirname(backendCommand(app));
  }
  return repoRoot();
}

function backendArgs(app, workspace, port) {
  if (!app.isPackaged) {
    return ['run', './cmd/ouro', 'serve', workspace, '--port', String(port)];
  }
  return ['serve', workspace, '--port', String(port)];
}

function backendInitArgs(app, workspace) {
  if (!app.isPackaged) {
    return ['run', './cmd/ouro', 'init', workspace];
  }
  return ['init', workspace];
}

function runBackendCommand(command, args, cwd, diagnosticLogPath) {
  return new Promise((resolve, reject) => {
    appendDiagnostic(diagnosticLogPath, 'backend.command.start', { command, args, cwd });
    const child = spawn(command, args, {
      cwd,
      stdio: ['ignore', 'pipe', 'pipe'],
      env: process.env,
    });
    let stderr = '';
    let stdout = '';
    child.stdout.on('data', chunk => {
      stdout += chunk.toString();
    });
    child.stderr.on('data', chunk => {
      stderr += chunk.toString();
    });
    child.once('error', error => {
      appendDiagnostic(diagnosticLogPath, 'backend.command.error', { command, cwd, message: error.message });
      reject(error);
    });
    child.once('exit', code => {
      const normalizedCode = normalizeExitCode(code);
      appendDiagnostic(diagnosticLogPath, 'backend.command.exit', {
        command,
        cwd,
        code: normalizedCode,
        rawCode: typeof code === 'number' ? code : null,
        stdout: stdout.trim().slice(-2000),
        stderr: stderr.trim().slice(-2000),
      });
      if (normalizedCode === 0) {
        resolve();
        return;
      }
      reject(new Error(stderr.trim() || describeBackendCommandExit(normalizedCode)));
    });
  });
}

function repoRoot() {
  return path.resolve(__dirname, '../../..');
}

function fileExists(targetPath) {
  return executableCandidates(targetPath).some(candidate => {
    try {
      return fs.statSync(candidate).isFile();
    } catch {
      return false;
    }
  });
}

function executableCandidates(targetPath) {
  if (typeof targetPath !== 'string' || targetPath.trim() === '') {
    return [];
  }
  if (path.isAbsolute(targetPath) || targetPath.includes(path.sep)) {
    return [targetPath];
  }
  const pathEntries = (process.env.PATH || '').split(path.delimiter).filter(Boolean);
  const extensions = process.platform === 'win32'
    ? ['', ...(process.env.PATHEXT || '.EXE;.CMD;.BAT;.COM').split(';')]
    : [''];
  return pathEntries.flatMap(entry => extensions.map(extension => path.join(entry, `${targetPath}${extension}`)));
}

function directoryExists(targetPath) {
  try {
    return fs.statSync(targetPath).isDirectory();
  } catch {
    return false;
  }
}

function readJSONFile(targetPath) {
  try {
    return JSON.parse(fs.readFileSync(targetPath, 'utf8'));
  } catch {
    return null;
  }
}

function normalizeProcessID(value) {
  return typeof value === 'number' && Number.isSafeInteger(value) && value > 1 ? value : 0;
}

function isSameDesktopRuntimeProcess(state) {
  const processId = normalizeProcessID(state?.processId);
  if (!processMayBeAlive(processId)) {
    return false;
  }

  const expectedStartToken = stringOrEmpty(state?.processStartToken);
  if (expectedStartToken && readProcessStartToken(processId) !== expectedStartToken) {
    return false;
  }

  const expectedExecutablePath = stringOrEmpty(state?.executablePath);
  if (expectedExecutablePath && process.platform === 'linux') {
    const actualExecutablePath = readProcessExecutablePath(processId);
    if (!actualExecutablePath || canonicalPath(actualExecutablePath) !== canonicalPath(expectedExecutablePath)) {
      return false;
    }
  }
  return true;
}

function processMayBeAlive(processId) {
  if (!processId) {
    return false;
  }
  try {
    process.kill(processId, 0);
    if (process.platform !== 'linux') {
      return true;
    }
    return isLiveLinuxProcessState(linuxProcessFields(processId)?.[0] || '');
  } catch (error) {
    return error?.code === 'EPERM';
  }
}

export function isLiveLinuxProcessState(state) {
  return state !== 'Z' && state !== 'X';
}

function linuxProcessGroupHasLiveMember(processGroupId) {
  let entries;
  try {
    entries = fs.readdirSync('/proc', { withFileTypes: true });
  } catch {
    return null;
  }
  for (const entry of entries) {
    if (!entry.isDirectory() || !/^\d+$/.test(entry.name)) {
      continue;
    }
    const fields = linuxProcessFields(Number(entry.name));
    if (Number(fields?.[2]) === processGroupId && isLiveLinuxProcessState(fields?.[0] || '')) {
      return true;
    }
  }
  return false;
}

function linuxProcessFields(processId) {
  if (process.platform !== 'linux') {
    return null;
  }
  try {
    const stat = fs.readFileSync(`/proc/${processId}/stat`, 'utf8');
    const commandEnd = stat.lastIndexOf(')');
    return commandEnd < 0 ? null : stat.slice(commandEnd + 1).trim().split(/\s+/);
  } catch {
    return null;
  }
}

function readProcessStartToken(processId) {
  const startTime = linuxProcessFields(processId)?.[19] || '';
  return /^\d+$/.test(startTime) ? `linux:${startTime}` : '';
}

function readProcessExecutablePath(processId) {
  if (process.platform !== 'linux') {
    return '';
  }
  try {
    return fs.readlinkSync(`/proc/${processId}/exe`);
  } catch {
    return '';
  }
}

function canonicalPath(value) {
  try {
    return fs.realpathSync(value);
  } catch {
    return path.resolve(value);
  }
}

function normalizeDesktopRuntimeIssue(event, details) {
  const issue = {
    event,
    recordedAt: new Date().toISOString(),
  };
  for (const [key, value] of Object.entries(details || {})) {
    if (value === undefined || typeof value === 'function') {
      continue;
    }
    if (typeof value === 'string') {
      issue[key] = value.slice(-2000);
      continue;
    }
    if (typeof value === 'number' || typeof value === 'boolean' || value === null) {
      issue[key] = value;
      continue;
    }
    try {
      issue[key] = JSON.stringify(value).slice(-2000);
    } catch {
      issue[key] = String(value).slice(-2000);
    }
  }
  return issue;
}

function normalizeSpawnErrorDetails(error, command, cwd) {
  return {
    message: error?.message ? String(error.message).slice(-2000) : '',
    code: error?.code ? String(error.code).slice(-2000) : '',
    errno: typeof error?.errno === 'number' ? error.errno : undefined,
    syscall: error?.syscall ? String(error.syscall).slice(-2000) : '',
    path: error?.path ? String(error.path).slice(-2000) : '',
    spawnargs: Array.isArray(error?.spawnargs) ? error.spawnargs.map(value => String(value).slice(-2000)) : undefined,
    command,
    cwd,
  };
}

function stringOrEmpty(value) {
  return typeof value === 'string' ? value : '';
}

function formatCommandLine(command, args) {
  return [command, ...args].map(shellQuote).join(' ');
}

function shellQuote(value) {
  const text = String(value);
  if (/^[A-Za-z0-9_./:=@+-]+$/.test(text)) {
    return text;
  }
  return `'${text.replace(/'/g, "'\\''")}'`;
}

function appendDiagnostic(logPath, event, details) {
  try {
    fs.mkdirSync(path.dirname(logPath), { recursive: true });
    fs.appendFileSync(logPath, JSON.stringify({
      time: new Date().toISOString(),
      event,
      ...details,
    }) + '\n');
  } catch {
    // Diagnostic logging must not prevent the backend from starting.
  }
}

function choosePort() {
  return new Promise((resolve, reject) => {
    const server = net.createServer();
    server.once('error', reject);
    server.listen(0, '127.0.0.1', () => {
      const address = server.address();
      server.close(() => {
        if (address && typeof address === 'object') {
          resolve(address.port);
          return;
        }
        reject(new Error('无法为后端选择端口'));
      });
    });
  });
}

async function waitForHealth(apiBaseURL, child) {
  const controller = new AbortController();
  let removeProcessListeners = () => {};
  const processExit = new Promise((_, reject) => {
    if (!child) {
      return;
    }
    const onExit = (code, signal) => reject(new BackendStartupExitError(code, signal));
    const onError = error => reject(new BackendStartupExitError(child.exitCode, child.signalCode, error));
    if (child.exitCode !== null || child.signalCode !== null) {
      onExit(child.exitCode, child.signalCode);
      return;
    }
    child.once('exit', onExit);
    child.once('error', onError);
    removeProcessListeners = () => {
      child.removeListener('exit', onExit);
      child.removeListener('error', onError);
    };
  });

  const healthCheck = pollHealth(apiBaseURL, controller.signal);
  try {
    await Promise.race([healthCheck, processExit]);
  } finally {
    controller.abort();
    removeProcessListeners();
  }
}

async function pollHealth(apiBaseURL, signal) {
  const deadline = Date.now() + 30000;
  let lastError;
  while (Date.now() < deadline) {
    try {
      const response = await fetch(`${apiBaseURL}/api/health`, { signal });
      if (response.ok) {
        return;
      }
      lastError = new Error(`后端健康检查返回 HTTP ${response.status}`);
    } catch (error) {
      if (signal.aborted) {
        throw error;
      }
      lastError = error;
    }
    if (signal.aborted) {
      return;
    }
    await new Promise(resolve => setTimeout(resolve, 300));
  }
  throw lastError ?? new Error('后端健康检查超时');
}
