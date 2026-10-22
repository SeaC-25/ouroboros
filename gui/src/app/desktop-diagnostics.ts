import { zhCN } from './zh-cn';
import type { DesktopRuntimeIssue, DesktopStatus } from './types';

export interface DesktopDiagnosticItem {
  label: string;
  value: string;
}

interface DesktopDiagnosis {
  summary: string;
  action: string;
}

const MAX_OUTPUT_LENGTH = 360;

export function buildDesktopDiagnosticItems(desktopStatus: DesktopStatus): DesktopDiagnosticItem[] {
  const details: DesktopDiagnosticItem[] = [];
  const seen = new Set<string>();
  const runtime = desktopStatus.runtime;
  const issue = runtime?.lastDesktopRuntimeIssue;
  const diagnosis = inferDesktopDiagnosis(desktopStatus);

  appendDesktopStatusDetail(details, seen, zhCN.desktop.detailDiagnosticSummary, diagnosis?.summary);
  appendDesktopStatusDetail(details, seen, zhCN.desktop.detailAction, diagnosis?.action);
  appendDesktopStatusDetail(details, seen, zhCN.desktop.detailError, desktopStatus.lastError);
  appendDesktopStatusDetail(details, seen, zhCN.desktop.apiBaseUrl, desktopStatus.apiBaseURL);
  appendDesktopStatusDetail(details, seen, zhCN.settings.rows.workspacePath, desktopStatus.workspace);
  appendDesktopStatusDetail(details, seen, zhCN.desktop.detailLastEvent, desktopStatus.lastEvent || runtime?.lastEvent);

  if (!runtime) {
    return details;
  }

  appendDesktopStatusDetail(details, seen, zhCN.desktop.detailHealthError, runtime.lastHealthError);
  appendDesktopStatusDetail(
    details,
    seen,
    zhCN.desktop.detailBackendBinary,
    formatDesktopDiagnosticPath(runtime.backendBinaryPath, runtime.backendBinaryExists),
  );
  appendDesktopStatusDetail(
    details,
    seen,
    zhCN.desktop.detailBackendWorkingDirectory,
    formatDesktopDiagnosticPath(runtime.backendWorkingDirectory, runtime.backendWorkingDirectoryExists),
  );
  appendDesktopStatusDetail(
    details,
    seen,
    zhCN.desktop.detailDiagnosticLog,
    formatDesktopDiagnosticPath(runtime.diagnosticLogPath, runtime.diagnosticLogExists),
  );
  appendDesktopStatusDetail(details, seen, zhCN.desktop.detailRuntimeState, runtime.desktopRuntimeStatePath);
  appendDesktopStatusDetail(details, seen, zhCN.desktop.detailRuntimeIssue, formatDesktopRuntimeIssue(issue));
  appendDesktopStatusDetail(details, seen, zhCN.desktop.detailRuntimeIssueReason, formatDesktopRuntimeIssueReason(issue));
  const issueExit = formatDesktopExit(issue?.exitCode, undefined);
  if (issueExit !== zhCN.settings.runtimeModes.none) {
    appendDesktopStatusDetail(details, seen, zhCN.desktop.detailRuntimeIssueExit, issueExit);
  }
  appendDesktopStatusDetail(details, seen, zhCN.desktop.detailPreviousRuntime, formatPreviousDesktopRuntime(issue));
  if (desktopRuntimeIssueLooksProcessKilled(issue)) {
    appendDesktopStatusDetail(details, seen, zhCN.desktop.detailProcessHint, zhCN.desktop.runtimeIssueHint);
  }
  appendDesktopStatusDetail(details, seen, zhCN.desktop.detailLastCommand, runtime.lastCommandLine);
  const exit = formatDesktopExit(runtime.lastExitCode, runtime.lastExitSignal);
  if (exit !== zhCN.settings.runtimeModes.none) {
    appendDesktopStatusDetail(details, seen, zhCN.desktop.detailExitInfo, exit);
  }
  appendDesktopStatusDetail(details, seen, zhCN.desktop.detailLastStderr, compactDiagnosticOutput(runtime.lastStderr));
  appendDesktopStatusDetail(details, seen, zhCN.desktop.detailLastStdout, compactDiagnosticOutput(runtime.lastStdout));
  appendDesktopStatusDetail(details, seen, zhCN.desktop.detailLastLog, compactDiagnosticOutput(runtime.lastLog));
  return details;
}

function inferDesktopDiagnosis(desktopStatus: DesktopStatus): DesktopDiagnosis | null {
  const runtime = desktopStatus.runtime;
  const lastError = desktopStatus.lastError || '';
  const stderr = runtime?.lastStderr || '';
  const stdout = runtime?.lastStdout || '';
  const log = runtime?.lastLog || '';
  const issue = runtime?.lastDesktopRuntimeIssue;
  const issueText = [
    issue?.event,
    issue?.reason,
    issue?.message,
    issue?.type,
    issue?.serviceName,
    issue?.errorDescription,
  ].filter(Boolean).join('\n');
  const combinedOutput = [lastError, stderr, stdout, log, issueText].filter(Boolean).join('\n');

  if (runtime?.backendBinaryExists === false) {
    return {
      summary: zhCN.desktop.diagnosisBackendBinaryMissing,
      action: zhCN.desktop.actionMissingBackendPath,
    };
  }
  if (runtime?.backendWorkingDirectoryExists === false) {
    return {
      summary: zhCN.desktop.diagnosisBackendWorkingDirectoryMissing,
      action: zhCN.desktop.actionMissingBackendPath,
    };
  }
  if (hasOomClue(combinedOutput)) {
    return {
      summary: issue ? zhCN.desktop.diagnosisRendererOom : zhCN.desktop.diagnosisOom,
      action: issue ? zhCN.desktop.actionDesktopProcessGone : zhCN.desktop.actionOom,
    };
  }
  if (issue?.event === 'desktop.previous_unclean_exit' || issue?.event === 'desktop.previous_runtime_still_marked_running') {
    return {
      summary: zhCN.desktop.diagnosisPreviousUncleanExit,
      action: zhCN.desktop.actionDesktopProcessGone,
    };
  }
  if (desktopRuntimeIssueLooksProcessKilled(issue)) {
    return {
      summary: zhCN.desktop.diagnosisRendererOom,
      action: zhCN.desktop.actionDesktopProcessGone,
    };
  }
  if (runtime?.lastExitSignal === 'SIGKILL' || runtime?.lastExitCode === 137) {
    return {
      summary: zhCN.desktop.diagnosisKilled,
      action: zhCN.desktop.actionKilled,
    };
  }
  if (isCleanDesktopBackendExit(runtime?.lastExitCode, runtime?.lastExitSignal)) {
    return {
      summary: zhCN.desktop.diagnosisCleanExit,
      action: zhCN.desktop.actionCleanExit,
    };
  }
  if (runtime?.lastHealthError || desktopStatusLooksLikeHealthFailure(lastError)) {
    return {
      summary: zhCN.desktop.diagnosisHealthFailed,
      action: zhCN.desktop.actionHealthFailed,
    };
  }
  if (runtime?.lastExitSignal) {
    return {
      summary: `${zhCN.desktop.diagnosisSignalPrefix}${runtime.lastExitSignal}${zhCN.desktop.diagnosisSignalSuffix}`,
      action: zhCN.desktop.actionGenericCrash,
    };
  }
  if (typeof runtime?.lastExitCode === 'number' && runtime.lastExitCode >= 0) {
    return {
      summary: `${zhCN.desktop.diagnosisExitCodePrefix}${runtime.lastExitCode}${zhCN.desktop.diagnosisExitCodeSuffix}`,
      action: zhCN.desktop.actionGenericCrash,
    };
  }
  if (runtime?.lastExitCode === null && desktopStatus.status === 'crashed') {
    return {
      summary: zhCN.desktop.diagnosisExitCodeUnknown,
      action: zhCN.desktop.actionGenericCrash,
    };
  }
  if (stderr) {
    return {
      summary: zhCN.desktop.diagnosisStderr,
      action: zhCN.desktop.actionGenericCrash,
    };
  }
  if (desktopStatus.status === 'offline' && desktopStatus.workspace) {
    return {
      summary: zhCN.desktop.diagnosisOffline,
      action: zhCN.desktop.actionOffline,
    };
  }
  if (lastError) {
    return {
      summary: zhCN.desktop.diagnosisErrorOnly,
      action: zhCN.desktop.actionGenericCrash,
    };
  }
  return null;
}

function hasOomClue(value: string): boolean {
  return /out of memory|oom|cannot allocate memory|内存不足|内存耗尽|被系统终止/i.test(value);
}

function desktopRuntimeIssueLooksProcessKilled(issue?: DesktopRuntimeIssue): boolean {
  if (!issue) {
    return false;
  }
  const text = [
    issue.event,
    issue.reason,
    issue.message,
    issue.type,
    issue.serviceName,
    issue.errorDescription,
  ].filter(Boolean).join('\n');
  return /oom|killed|abnormal-exit|crashed|gone|unclean|SIGKILL|内存/i.test(text)
    || issue.exitCode === 137;
}

function desktopStatusLooksLikeHealthFailure(message?: string): boolean {
  return Boolean(message && /health|健康检查|timed out|timeout/i.test(message));
}

function isCleanDesktopBackendExit(code?: number | null, signal?: string): boolean {
  return code === 0 || signal === 'SIGTERM' || signal === 'SIGINT';
}

function appendDesktopStatusDetail(
  details: DesktopDiagnosticItem[],
  seen: Set<string>,
  label: string,
  value?: string,
) {
  const normalized = value?.trim();
  if (!normalized || seen.has(`${label}:${normalized}`)) {
    return;
  }
  seen.add(`${label}:${normalized}`);
  details.push({ label, value: normalized });
}

function formatDesktopDiagnosticPath(value?: string, exists?: boolean): string {
  if (!value) {
    return '';
  }
  if (exists === false) {
    return `${value} / ${zhCN.settings.runtimeModes.missing}`;
  }
  if (exists === true) {
    return `${value} / ${zhCN.settings.runtimeModes.exists}`;
  }
  return value;
}

function formatDesktopExit(code?: number | null, signal?: string): string {
  if (signal) {
    if (typeof code === 'number' && code >= 0) {
      return `${zhCN.details.status} ${code} / ${signal}`;
    }
    return signal;
  }
  if (typeof code === 'number' && code >= 0) {
    return `${zhCN.details.status} ${code}`;
  }
  if (code === null) {
    return zhCN.desktop.exitCodeUnknown;
  }
  return zhCN.settings.runtimeModes.none;
}

function formatDesktopRuntimeIssue(issue?: DesktopRuntimeIssue): string {
  if (!issue) {
    return '';
  }
  return [
    issue.event,
    issue.recordedAt,
    issue.type,
    issue.serviceName,
    issue.name,
  ].filter(Boolean).join(' / ');
}

function formatDesktopRuntimeIssueReason(issue?: DesktopRuntimeIssue): string {
  if (!issue) {
    return '';
  }
  return [
    issue.reason,
    issue.message,
    issue.errorDescription,
    issue.validatedURL,
  ].filter(Boolean).join(' / ');
}

function formatPreviousDesktopRuntime(issue?: DesktopRuntimeIssue): string {
  if (!issue || (!issue.previousSessionId && !issue.previousProcessId && !issue.previousStartedAt && !issue.previousHeartbeatAt)) {
    return '';
  }
  return [
    issue.previousSessionId,
    issue.previousProcessId ? `pid ${issue.previousProcessId}` : '',
    issue.previousStartedAt,
    issue.previousHeartbeatAt,
    issue.previousWorkspace,
    issue.statePath,
  ].filter(Boolean).join(' / ');
}

function compactDiagnosticOutput(value?: string): string {
  const normalized = value?.trim().replace(/\s+/g, ' ');
  if (!normalized) {
    return '';
  }
  if (normalized.length <= MAX_OUTPUT_LENGTH) {
    return normalized;
  }
  return `${normalized.slice(0, MAX_OUTPUT_LENGTH)}...`;
}
