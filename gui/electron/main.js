import { app } from 'electron';
import { existsSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import {
  applyMemoryEnvironment,
  buildMemoryRelaunchArgs,
  hasMemoryLaunchProfile,
  memoryV8Flags,
  resolveMemoryProfile,
} from './memory-profile.js';

const profile = resolveMemoryProfile(process.env.OURO_DESKTOP_MEMORY_MODE, app.isPackaged);
applyMemoryEnvironment(process.env, profile);

const currentArgs = process.argv.slice(1);
if (app.isPackaged && !hasMemoryLaunchProfile(currentArgs, profile)) {
  app.relaunch({ args: buildMemoryRelaunchArgs(currentArgs, profile) });
  app.exit(0);
} else {
  app.commandLine.appendSwitch('renderer-process-limit', profile.rendererProcessLimit);
  app.commandLine.appendSwitch('js-flags', memoryV8Flags(profile));
  if (profile.mode === 'low') {
    app.commandLine.appendSwitch('disable-site-isolation-trials');
    app.commandLine.appendSwitch('process-per-site');
  }

  const packagedApplicationMain = fileURLToPath(new URL('./application-main.js', import.meta.url));
  if (existsSync(packagedApplicationMain)) {
    await import('./application-main.js');
  } else {
    await import('../src/main/main.js');
  }
}
