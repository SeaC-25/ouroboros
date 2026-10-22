// Dynamic memory relaunch contract driver. Byte-for-byte port of
// checkMemoryArgumentContract() from gui/scripts/check-desktop-contract.mjs.
// Usage: node memory.mjs <absolute path of electron/memory-profile.js>
import assert from 'node:assert/strict';
import { pathToFileURL } from 'node:url';

const memory = await import(pathToFileURL(process.argv[2]).href);
const profile = memory.resolveMemoryProfile('', true);
const marker = '--ouro-desktop-memory-profile=low';
try {
  assert.equal(memory.hasMemoryLaunchProfile([marker], profile), false);
  assert.equal(memory.hasMemoryLaunchProfile([
    marker,
    '--js-flags=--max-old-space-size=256 --max-semi-space-size=8',
    '--renderer-process-limit=1',
    '--renderer-process-limit=9',
  ], profile), false);
  const args = memory.buildMemoryRelaunchArgs([
    '--user-argument',
    '--js-flags',
    '--max-old-space-size=2048',
    '--renderer-process-limit',
    '9',
  ], profile);
  assert.deepEqual(args, [
    '--user-argument',
    marker,
    '--js-flags=--max-old-space-size=256 --max-semi-space-size=8',
    '--renderer-process-limit=1',
  ]);
  assert.equal(memory.hasMemoryLaunchProfile(args, profile), true);
} catch (error) {
  console.error(`packaged Electron memory relaunch arguments are not canonical: ${error.message}`);
  process.exit(1);
}
