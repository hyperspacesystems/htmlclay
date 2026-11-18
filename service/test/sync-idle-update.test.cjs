'use strict';

// An update restart asks /sync/v1/idle with forUpdate. That answer comes from the
// engine's update test (whenQuiescentForUpdate), not from the queues alone: a
// session that is starting has an empty queue and still holds work a restart drops.

const test = require('node:test');
const assert = require('node:assert/strict');
const { EventEmitter } = require('node:events');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const { createSync } = require('../sync.cjs');

test('idle with forUpdate is the answer of the engine update test', async (t) => {
  const asked = [];
  const managers = [];
  let quiet = false;
  class StandInManager extends EventEmitter {
    constructor() {
      super();
      this.sessions = new Map();
      managers.push(this);
    }
    statuses() { return []; }
    async startEnabledSessions() {}
    async stop() {}
    async refreshAccounts() { return { accounts: [] }; }
    async stopAll() {}
    async whenQuiescentForUpdate(options) {
      asked.push(options);
      return quiet;
    }
  }
  const managerPath = require.resolve('../local/src/main/sync-manager.js');
  const real = require.cache[managerPath];
  require.cache[managerPath] = { id: managerPath, filename: managerPath, loaded: true, exports: { SyncManager: StandInManager } };
  t.after(() => {
    if (real) require.cache[managerPath] = real;
    else delete require.cache[managerPath];
  });
  const dataDir = fs.mkdtempSync(path.join(os.tmpdir(), 'htmlclay-sync-idle-'));
  t.after(() => fs.rmSync(dataDir, { recursive: true, force: true }));
  const service = { goCall: async (method, pathname, report) => ({ ok: true, seq: report.seq }), roots: new Map(), authority: null };
  const sync = createSync({ gate: { hold() {}, clear() {} }, service, dataDir });

  assert.deepEqual(await sync.idle({ timeoutMs: 1000, forUpdate: true }), { idle: true }, 'with no key held there is no sync to wait for');

  t.after(() => sync.handle({ v: 1, key: null }));
  assert.equal(await sync.handle({ v: 1, key: `hcsk_${'5e'.repeat(32)}`, serverUrl: 'http://127.0.0.1:1' }), 'SYNC KEY HELD');
  assert.equal((await sync.configure({ deviceId: 'device-1', paused: false, sessions: [], roots: [] })).ok, true);
  // One session whose queue is empty: the plain idle question says yes.
  managers[0].sessions.set('s1', { runner: { whenIdle: async () => {} } });

  assert.deepEqual(await sync.idle({ timeoutMs: 1000 }), { idle: true }, 'the plain question reads the queues');
  assert.deepEqual(asked, [], 'the plain question does not run the update test');

  assert.deepEqual(await sync.idle({ timeoutMs: 1234, forUpdate: true }), { idle: false });
  assert.deepEqual(asked, [{ timeoutMs: 1234 }], 'the update test gets the caller time limit');

  quiet = true;
  assert.deepEqual(await sync.idle({ timeoutMs: 1234, forUpdate: true }), { idle: true });
});
