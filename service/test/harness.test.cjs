'use strict';

// The harness ends a service before it removes the service's data dir. A removal
// that ran first could meet a late write there, fail, skip the kill, and leave the
// service holding the test run open forever.

const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const { startService } = require('./harness.cjs');

function running(pid) {
  try {
    process.kill(pid, 0);
    return true;
  } catch {
    return false;
  }
}

test('a service has exited before its data dir is removed', async (t) => {
  const rmSync = fs.rmSync;
  t.after(() => { fs.rmSync = rmSync; });
  let service = null;
  const removals = [];
  fs.rmSync = (target, options) => {
    if (service && target === service.tmp) removals.push({ serviceRunning: running(service.ready.pid) });
    return rmSync(target, options);
  };

  await t.test('a service started and left running', async (st) => {
    service = await startService(st);
    assert.equal(running(service.ready.pid), true);
  });

  assert.deepEqual(removals, [{ serviceRunning: false }]);
  assert.equal(fs.existsSync(service.tmp), false);
});
