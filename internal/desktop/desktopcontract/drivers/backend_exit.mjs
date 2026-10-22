// Dynamic backend exit normalization contract driver. Byte-for-byte port of
// checkBackendExitContract() from gui/scripts/check-desktop-contract.mjs.
// Usage: node backend_exit.mjs <absolute path of src/main/backend.js>
import assert from 'node:assert/strict';
import { pathToFileURL } from 'node:url';

const lifecycle = await import(pathToFileURL(process.argv[2]).href);
try {
  assert.equal(lifecycle.normalizeExitCode(-1), null);
  assert.equal(lifecycle.normalizeExitCode(null), null);
  assert.equal(lifecycle.normalizeExitCode(1.5), null);
  assert.equal(lifecycle.normalizeExitCode(0), 0);
  assert.equal(lifecycle.isCleanBackendExit(null, 'SIGTERM'), false);
  assert.equal(lifecycle.isCleanBackendExit(null, null, true), true);
  assert.equal(lifecycle.isCleanBackendExit(null, null, false), false);
  assert.equal(lifecycle.isLiveLinuxProcessState('S'), true);
  assert.equal(lifecycle.isLiveLinuxProcessState('Z'), false);
  assert.equal(lifecycle.isLiveLinuxProcessState('X'), false);
} catch (error) {
  console.error(`backend exit normalization must preserve normal null exits without synthesizing -1: ${error.message}`);
  process.exit(1);
}
