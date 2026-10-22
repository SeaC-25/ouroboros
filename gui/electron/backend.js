import fs from 'node:fs';
import path from 'node:path';
import { BackendManager as BaseBackendManager, isBackendStartupExitError } from './base-backend.js';

const DEFAULT_RESTART_LIMIT = 3;
const MAX_RESTART_LIMIT = 10;
const INITIAL_RESTART_DELAY_MS = 750;
const MAX_RESTART_DELAY_MS = 6000;
const RECOVERY_STABLE_RESET_MS = 30000;

export class BackendManager extends BaseBackendManager {
  constructor({ app }) {
    super({ app });
    this.restartLimit = readRestartLimit(process.env.OURO_DESKTOP_SIDECAR_RESTART_LIMIT);
    this.restartAttempts = 0;
    this.recoveryTimer = null;
    this.recoveryResetTimer = null;
    this.recoveryDisabled = false;
    this.shutdownRequested = false;
    this.startTransition = false;
    this.baseStartStopDispatch = false;
    this.startingPromise = null;
    this.pendingStartRequests = new Set();
    this.lifecycleRequest = 0;
    this.desiredWorkspace = '';
    this.sidecarPermissionRepaired = false;

    this.on('status', status => {
      if (status.status === 'ready') {
        this.cancelRecovery('ready');
        this.scheduleRecoveryReset();
        return;
      }
      if (status.status === 'crashed') {
        this.cancelRecoveryReset();
      }
      if (status.status === 'crashed' && !this.startTransition && !this.startingPromise) {
        this.scheduleRecovery();
      }
    });

    const disableRecovery = reason => {
      this.shutdownRequested = true;
      this.recoveryDisabled = true;
      this.desiredWorkspace = '';
      this.lifecycleRequest += 1;
      this.cancelRecovery(reason);
      this.cancelRecoveryReset();
    };
    app.on('before-quit', () => disableRecovery('before-quit'));
    app.on('will-quit', () => disableRecovery('will-quit'));
  }

  async start(workspace) {
    const normalizedWorkspace = typeof workspace === 'string' ? workspace.trim() : '';
    if (this.shutdownRequested) {
      this.writeDiagnostic('sidecar.start_skipped', { reason: 'app-quit', workspace: normalizedWorkspace });
      return this.snapshot();
    }

    const request = ++this.lifecycleRequest;
    if (!normalizedWorkspace) {
      this.desiredWorkspace = '';
      this.recoveryDisabled = true;
      this.cancelRecovery('empty-workspace');
      this.cancelRecoveryReset();
      this.restartAttempts = 0;
      await settle(this.startingPromise);
      if (request !== this.lifecycleRequest) {
        return this.snapshot();
      }
      return super.start(workspace);
    }

    this.desiredWorkspace = normalizedWorkspace;
    this.recoveryDisabled = false;
    this.cancelRecovery('start');
    this.cancelRecoveryReset();
    this.restartAttempts = 0;
    return this.startRequestedWorkspace(normalizedWorkspace, request);
  }

  async startRequestedWorkspace(workspace, request) {
    this.pendingStartRequests.add(request);
    try {
      await settle(this.startingPromise);
      if (!this.canStartRequest(workspace, request)) {
        return this.snapshot();
      }

      const startingPromise = this.startOnce(workspace);
      this.startingPromise = startingPromise;
      try {
        return await startingPromise;
      } finally {
        if (this.startingPromise === startingPromise) {
          this.startingPromise = null;
        }
        if (this.canStartRequest(workspace, request) && this.status === 'crashed') {
          this.scheduleRecovery();
        }
      }
    } finally {
      this.pendingStartRequests.delete(request);
    }
  }

  canStartRequest(workspace, request) {
    return !this.shutdownRequested
      && !this.recoveryDisabled
      && request === this.lifecycleRequest
      && workspace === this.desiredWorkspace;
  }

  async startOnce(workspace) {
    this.startTransition = true;
    try {
      const permission = ensurePackagedSidecarExecutable(this.app);
      this.sidecarPermissionRepaired ||= permission.repaired;
      if (permission.repaired) {
        this.writeDiagnostic('sidecar.permission_repaired', permission);
      }
      // BaseBackendManager.start() synchronously dispatches its initial stop through this override.
      this.baseStartStopDispatch = true;
      let startPromise;
      try {
        startPromise = super.start(workspace);
      } finally {
        this.baseStartStopDispatch = false;
      }
      return await startPromise;
    } catch (error) {
      if (isBackendStartupExitError(error)) {
        throw error;
      }
      if (this.status !== 'crashed') {
        this.workspace = workspace;
        this.clearBackendConnection();
        this.status = 'crashed';
        this.lastError = describeSidecarPreflightFailure(error, this.diagnosticLogPath);
        this.writeDiagnostic('sidecar.preflight_failed', diagnosticError(error));
        this.emitStatus();
      }
      throw error;
    } finally {
      this.startTransition = false;
    }
  }

  async stop() {
    if (this.baseStartStopDispatch) {
      return super.stop();
    }

    const request = ++this.lifecycleRequest;
    this.desiredWorkspace = '';
    this.recoveryDisabled = true;
    this.cancelRecovery('stop');
    this.cancelRecoveryReset();
    await settle(this.startingPromise);
    if (request !== this.lifecycleRequest) {
      return this.snapshot();
    }
    await super.stop();
    this.restartAttempts = 0;
    return this.snapshot();
  }

  async restart() {
    const workspace = this.desiredWorkspace || this.workspace;
    const request = ++this.lifecycleRequest;
    this.cancelRecovery('manual-restart');
    this.cancelRecoveryReset();
    this.restartAttempts = 0;
    this.recoveryDisabled = true;
    await settle(this.startingPromise);
    if (request !== this.lifecycleRequest || this.shutdownRequested) {
      return this.snapshot();
    }
    await super.stop();
    if (request !== this.lifecycleRequest || this.shutdownRequested || !workspace) {
      return this.snapshot();
    }
    this.desiredWorkspace = workspace;
    this.recoveryDisabled = false;
    return this.startRequestedWorkspace(workspace, request);
  }

  scheduleRecovery() {
    const workspace = this.desiredWorkspace || this.workspace;
    if (this.shutdownRequested || this.recoveryDisabled || this.recoveryTimer || this.startingPromise || !workspace) {
      return;
    }
    if (this.restartAttempts >= this.restartLimit) {
      this.recoveryDisabled = true;
      this.writeDiagnostic('sidecar.recovery_exhausted', {
        attempts: this.restartAttempts,
        limit: this.restartLimit,
        workspace,
        lastExitCode: this.lastExitCode,
        lastExitSignal: this.lastExitSignal,
      });
      return;
    }

    const attempt = this.restartAttempts + 1;
    const delayMs = Math.min(INITIAL_RESTART_DELAY_MS * (2 ** (attempt - 1)), MAX_RESTART_DELAY_MS);
    this.writeDiagnostic('sidecar.recovery_scheduled', {
      attempt,
      limit: this.restartLimit,
      delayMs,
      workspace,
      lastExitCode: this.lastExitCode,
      lastExitSignal: this.lastExitSignal,
    });
    const request = this.lifecycleRequest;
    this.recoveryTimer = setTimeout(() => {
      this.recoveryTimer = null;
      if (!this.canStartRequest(workspace, request)) {
        return;
      }
      this.restartAttempts = attempt;
      this.writeDiagnostic('sidecar.recovery_attempt', { attempt, workspace });
      void this.startRequestedWorkspace(workspace, request).catch(error => {
        this.writeDiagnostic('sidecar.recovery_failed', {
          attempt,
          workspace,
          ...diagnosticError(error),
        });
      });
    }, delayMs);
    this.recoveryTimer.unref?.();
  }

  cancelRecovery(reason) {
    if (!this.recoveryTimer) {
      return;
    }
    clearTimeout(this.recoveryTimer);
    this.recoveryTimer = null;
    this.writeDiagnostic('sidecar.recovery_cancelled', { reason });
  }

  scheduleRecoveryReset() {
    if (this.restartAttempts === 0 || this.recoveryDisabled || this.shutdownRequested || this.recoveryResetTimer) {
      return;
    }
    const child = this.process;
    this.recoveryResetTimer = setTimeout(() => {
      this.recoveryResetTimer = null;
      if (this.status !== 'ready' || this.process !== child || this.recoveryDisabled || this.shutdownRequested) {
        return;
      }
      const attempts = this.restartAttempts;
      this.restartAttempts = 0;
      this.writeDiagnostic('sidecar.recovery_stable', { attempts, stableForMs: RECOVERY_STABLE_RESET_MS });
    }, RECOVERY_STABLE_RESET_MS);
    this.recoveryResetTimer.unref?.();
  }

  cancelRecoveryReset() {
    if (!this.recoveryResetTimer) {
      return;
    }
    clearTimeout(this.recoveryResetTimer);
    this.recoveryResetTimer = null;
  }

  recordProcessExit(child, code, signal, details = {}) {
    const normalizedCode = normalizeRecordedExitCode(code);
    this.lastExitCode = normalizedCode;
    this.lastExitSignal = signal || '';
    this.writeDiagnostic('backend.exit', {
      pid: child.pid,
      code: normalizedCode,
      rawCode: typeof code === 'number' ? code : null,
      signal: this.lastExitSignal,
      expectedStop: Boolean(details.expectedStop),
      startupHealthFailed: Boolean(details.startupHealthFailed),
    });
    return normalizedCode;
  }

  snapshot() {
    const snapshot = super.snapshot();
    return {
      ...snapshot,
      running: snapshot.running || this.startTransition || Boolean(this.startingPromise) || this.pendingStartRequests.size > 0,
      memoryMode: process.env.OURO_DESKTOP_MEMORY_MODE || 'standard',
      sidecarPermissionRepaired: this.sidecarPermissionRepaired,
      sidecarRecovery: this.recoverySnapshot(),
    };
  }

  runtimeInfo() {
    return {
      ...super.runtimeInfo(),
      memoryMode: process.env.OURO_DESKTOP_MEMORY_MODE || 'standard',
      goMemoryLimit: process.env.GOMEMLIMIT || '',
      goGC: process.env.GOGC || '',
      sidecarPermissionRepaired: this.sidecarPermissionRepaired,
      sidecarRecovery: this.recoverySnapshot(),
    };
  }

  recoverySnapshot() {
    return {
      enabled: !this.recoveryDisabled,
      pending: Boolean(this.recoveryTimer),
      attempts: this.restartAttempts,
      limit: this.restartLimit,
    };
  }
}

export function ensurePackagedSidecarExecutable(app) {
  if (!app.isPackaged) {
    return { path: '', exists: false, executable: false, repaired: false };
  }
  const binaryPath = path.join(process.resourcesPath || '', process.platform === 'win32' ? 'ouro.exe' : 'ouro');
  let stat;
  try {
    stat = fs.statSync(binaryPath);
  } catch (error) {
    throw new Error(`打包后端不存在：${binaryPath}（${error.message}）`);
  }
  if (!stat.isFile()) {
    throw new Error(`打包后端不是普通文件：${binaryPath}`);
  }
  if (process.platform === 'win32') {
    return { path: binaryPath, exists: true, executable: true, repaired: false };
  }

  let repaired = false;
  if ((stat.mode & 0o111) === 0) {
    fs.chmodSync(binaryPath, stat.mode | 0o111);
    repaired = true;
  }
  fs.accessSync(binaryPath, fs.constants.X_OK);
  return { path: binaryPath, exists: true, executable: true, repaired };
}

export function normalizeRecordedExitCode(code) {
  return typeof code === 'number' && Number.isInteger(code) && code >= 0 ? code : null;
}

function readRestartLimit(value) {
  if (value === undefined || value === '') {
    return DEFAULT_RESTART_LIMIT;
  }
  const parsed = Number.parseInt(value, 10);
  if (!Number.isFinite(parsed)) {
    return DEFAULT_RESTART_LIMIT;
  }
  return Math.max(0, Math.min(parsed, MAX_RESTART_LIMIT));
}

function describeSidecarPreflightFailure(error, diagnosticLogPath) {
  const logHint = diagnosticLogPath ? `；诊断日志 ${diagnosticLogPath}` : '';
  return `后端进程启动前检查失败；${error?.message || String(error)}${logHint}`;
}

function diagnosticError(error) {
  return {
    message: error?.message ? String(error.message).slice(-2000) : String(error).slice(-2000),
    code: error?.code ? String(error.code).slice(-2000) : '',
    stack: error?.stack ? String(error.stack).slice(-4000) : '',
  };
}

async function settle(promise) {
  if (!promise) {
    return;
  }
  try {
    await promise;
  } catch {
    // A newer lifecycle request supersedes the previous start result.
  }
}
