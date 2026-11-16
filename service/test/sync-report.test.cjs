'use strict';

// 8.18: the service's reports to Go (C4). seq rises; an event leaves the queue only
// once Go acknowledged the report that carried it; a refused report is sent again
// from the newest state; at most EVENT_LIMIT events wait.

const test = require('node:test');
const assert = require('node:assert/strict');
const { EventEmitter } = require('node:events');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const { createReporter, createSync, EVENT_LIMIT } = require('../sync.cjs');

const wait = (ms) => new Promise((resolve) => setTimeout(resolve, ms));
const event = (n) => ({ id: `s1:${n}`, sessionId: 's1', rootId: 'r', at: 't', action: 'download', file: `f${n}`, type: 'site' });

function harness({ answer }) {
  const sent = [];
  let current = { sessions: [], keyRefused: false, accounts: null };
  const reporter = createReporter({
    send: async (report) => {
      sent.push(report);
      return answer(report, sent.length);
    },
    state: () => current,
    delayMs: 1,
    retryMs: [5],
  });
  return { sent, reporter, set: (next) => { current = next; } };
}

async function until(what, predicate) {
  const deadline = Date.now() + 5000;
  while (Date.now() < deadline) {
    if (predicate()) return;
    await wait(2);
  }
  assert.fail(`timed out waiting for ${what}`);
}

test('an acknowledged report takes its events off the queue', async (t) => {
  const h = harness({ answer: (report) => ({ ok: true, seq: report.seq }) });
  t.after(() => h.reporter.stop());
  h.reporter.push(event(1));
  h.reporter.push(event(2));
  await until('the first report', () => h.sent.length === 1);
  await until('the queue to empty', () => h.reporter.queued().length === 0);
  assert.equal(h.sent[0].seq, 1);
  assert.deepEqual(h.sent[0].events.map((e) => e.id), ['s1:1', 's1:2']);
  h.reporter.push(event(3));
  await until('the second report', () => h.sent.length === 2);
  assert.equal(h.sent[1].seq, 2);
  assert.deepEqual(h.sent[1].events.map((e) => e.id), ['s1:3'], 'acknowledged events are not sent again');
});

test('a refused report is sent again with the newest state', async (t) => {
  const h = harness({ answer: (report, n) => (n === 1 ? Promise.reject(new Error('Go answered 503')) : { ok: true, seq: report.seq }) });
  t.after(() => h.reporter.stop());
  h.reporter.push(event(1));
  await until('the first try', () => h.sent.length === 1);
  h.set({ sessions: [{ sessionId: 's1', status: 'syncing' }], keyRefused: false, accounts: null });
  await until('the retry', () => h.sent.length === 2);
  assert.deepEqual(h.sent[1].events.map((e) => e.id), ['s1:1'], 'the unacknowledged event is still there');
  assert.deepEqual(h.sent[1].sessions, [{ sessionId: 's1', status: 'syncing' }], 'the retry is rebuilt from the newest state');
  assert.ok(h.sent[1].seq > h.sent[0].seq);
  await until('the queue to empty', () => h.reporter.queued().length === 0);
});

test('an answer for another seq is not an acknowledgement', async (t) => {
  const h = harness({ answer: (report, n) => ({ ok: true, seq: n === 1 ? report.seq + 7 : report.seq }) });
  t.after(() => h.reporter.stop());
  h.reporter.push(event(1));
  await until('the retry', () => h.sent.length === 2);
  assert.deepEqual(h.sent[1].events.map((e) => e.id), ['s1:1']);
});

test('the queue keeps the newest events and counts the dropped ones', async (t) => {
  const h = harness({ answer: () => new Promise(() => {}) });
  t.after(() => h.reporter.stop());
  for (let n = 1; n <= EVENT_LIMIT + 5; n += 1) h.reporter.push(event(n));
  const queued = h.reporter.queued();
  assert.equal(queued.length, EVENT_LIMIT);
  assert.equal(queued[0].id, 's1:6');
  assert.equal(queued.at(-1).id, `s1:${EVENT_LIMIT + 5}`);
  assert.equal(h.reporter.dropped(), 5);
});

test('a queue that drains with nothing to send reaches Go', async (t) => {
  const row = { sessionId: 's1', rootId: 'r1', accountId: 7, status: 'syncing', running: true, paused: null, pendingCount: 1, conflicts: [], blocked: [], lastSyncAt: null, lastError: null };
  const managers = [];
  class StandInManager extends EventEmitter {
    constructor() {
      super();
      this.sessions = new Map();
      managers.push(this);
    }
    statuses() { return [{ ...row }]; }
    async startEnabledSessions() {}
    async refreshAccounts() { return { accounts: [] }; }
    async stopAll() {}
  }
  const managerPath = require.resolve('../vendor/local/src/main/sync-manager.js');
  const real = require.cache[managerPath];
  require.cache[managerPath] = { id: managerPath, filename: managerPath, loaded: true, exports: { SyncManager: StandInManager } };
  t.after(() => {
    if (real) require.cache[managerPath] = real;
    else delete require.cache[managerPath];
  });
  const dataDir = fs.mkdtempSync(path.join(os.tmpdir(), 'htmlclay-sync-report-'));
  t.after(() => fs.rmSync(dataDir, { recursive: true, force: true }));
  const reports = [];
  const service = {
    goCall: async (method, pathname, report) => {
      reports.push(report);
      return { ok: true, seq: report.seq };
    },
    roots: new Map(),
    authority: null,
  };
  const sync = createSync({ gate: { hold() {}, clear() {} }, service, dataDir });
  t.after(() => sync.handle({ v: 1, key: null }));
  assert.equal(await sync.handle({ v: 1, key: `hcsk_${'5e'.repeat(32)}`, serverUrl: 'http://127.0.0.1:1' }), 'SYNC KEY HELD');
  assert.equal((await sync.configure({ deviceId: 'device-1', paused: false, sessions: [], roots: [] })).ok, true);
  await until('a report of the queued file', () => reports.some((report) => report.sessions[0]?.pendingCount === 1));
  const sent = reports.length;
  Object.assign(row, { status: 'idle', pendingCount: 0 });
  managers[0].emit('sync-stats', { sessionId: 's1', rootId: 'r1' });
  await until('a report of the drained queue', () => reports.length > sent);
  assert.deepEqual(reports.at(-1).sessions.map((s) => [s.status, s.pendingCount]), [['idle', 0]]);
});
