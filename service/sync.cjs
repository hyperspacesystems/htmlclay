'use strict';

// Sync in the service. Go holds the sync key in the system's secret store and
// hands it over on stdin (08-sync.md, C2); the service keeps it only in memory and
// never writes, logs or echoes it, or any part of the line that carried it.

const path = require('path');
const { randomBytes } = require('crypto');
const { getAccounts, listNodes } = require('./vendor/local/src/sync-engine/api-client.js');
const { inventoryTotals } = require('./vendor/local/src/sync-engine/reconcile/first-bind.js');
const { snapshotDisk } = require('./vendor/local/src/sync-engine/reconcile/disk-snapshot.js');
const { isIgnoredRel } = require('./vendor/local/src/sync-engine/ignored-paths.js');
const { calculateChecksum } = require('./vendor/local/src/sync-engine/utils.js');
const fs = require('fs/promises');

const KEY = /^hcsk_[0-9a-f]{64}$/;
const PREFIX = '/sync/v1/';
const BODY_LIMIT = 64 * 1024;
// Local's rule (sync-manager.js REQUIRED_FEATURES): a server missing any of these
// is one that needs updating before it can sync, not one to sync with blind.
const REQUIRED_FEATURES = ['accountScopes', 'accountEvents', 'conditionalContent', 'conditionalStructure', 'completeInventory'];
const LOOPBACK = new Set(['127.0.0.1', 'localhost', '[::1]']);
// C4: the service keeps at most this many unacknowledged events, dropping the oldest.
const EVENT_LIMIT = 1000;
const EVENT_ACTIONS = new Set(['download', 'upload', 'create', 'delete', 'relocate', 'trash', 'conflict', 'conflict-copy', 'resolved']);
// Manager events after which Go should hear the newest state.
const REPORTED = ['status-changed', 'file-synced', 'sync-complete', 'sync-error', 'sync-failed', 'accounts', 'credentials-rejected'];
const RETRY_MS = [1000, 2000, 5000, 10000, 30000];
const SESSION_KINDS = new Set(['personal', 'team']);
// Discovery runs again this often while sync is on, as Local's does, so a team
// added or a plan changed online reaches this computer. A test shortens it.
const DISCOVERY_MS = Number(process.env.HTMLCLAY_SYNC_DISCOVERY_MS) > 0 ? Number(process.env.HTMLCLAY_SYNC_DISCOVERY_MS) : 5 * 60 * 1000;

// syncBaseOf is where an account's sync routes live: discovery names it, and Local's
// rule gives it for an older answer that does not.
function syncBaseOf(account) {
  if (account.syncBase) return account.syncBase;
  return account.kind === 'personal' ? '/_/sync' : `/_/team/${account.username}/sync`;
}

const relPathOf = (node) => (node.path ? `${node.path}/${node.name}` : node.name);

// compareFolder counts what a first sync of an existing folder would do, read-only:
// files only here go up, files only there come down, and files on both sides with
// different bytes become conflicts that keep both. Nothing is ever deleted.
async function compareFolder(folder, nodes) {
  const { entries } = await snapshotDisk(folder, { ignore: isIgnoredRel });
  const local = new Map([...entries].filter(([, entry]) => entry.type === 'file'));
  // On a disk that folds case, logo.png here and Logo.png there are one file: it waits as a
  // name clash, so it is counted with the files that differ, not as one up and one down.
  const folds = process.platform === 'darwin' || process.platform === 'win32';
  const foldKey = (rel) => rel.normalize('NFC').toLowerCase();
  const localByFold = new Map([...local.keys()].map((rel) => [foldKey(rel), rel]));
  let toDownload = 0;
  let differ = 0;
  for (const node of nodes) {
    if (!node || node.type === 'folder') continue;
    const rel = relPathOf(node);
    if (isIgnoredRel(rel)) continue;
    if (!local.has(rel)) {
      const twin = folds ? localByFold.get(foldKey(rel)) : undefined;
      if (twin !== undefined && local.has(twin)) {
        local.delete(twin);
        differ += 1;
        continue;
      }
      toDownload += 1;
      continue;
    }
    local.delete(rel);
    let bytes;
    try {
      bytes = await fs.readFile(path.join(folder, ...rel.split('/')));
    } catch {
      differ += 1;
      continue;
    }
    if (!node.checksum || (await calculateChecksum(bytes)) !== node.checksum) differ += 1;
  }
  return { toUpload: local.size, toDownload, differ };
}

function validSession(session) {
  return Boolean(session) && typeof session.id === 'string' && session.id !== '' && typeof session.rootId === 'string'
    && Number.isInteger(session.accountId) && SESSION_KINDS.has(session.kind);
}

// createReporter sends C4's reports to Go: each carries the newest state and every
// event not yet acknowledged, under a rising seq. A report Go does not acknowledge
// is sent again, rebuilt from the newest state, after a growing wait.
function createReporter({ send, state, delayMs = 100, retryMs = RETRY_MS, onAcked = () => {} }) {
  let seq = 0;
  let events = [];
  let dropped = 0;
  let timer = null;
  let inflight = false;
  let again = false;
  let attempt = 0;
  let stopped = false;

  function schedule(wait = delayMs) {
    if (stopped) return;
    if (inflight) {
      again = true;
      return;
    }
    if (timer) return;
    timer = setTimeout(flush, wait);
  }

  async function flush() {
    timer = null;
    if (stopped) return;
    inflight = true;
    again = false;
    const report = { seq: ++seq, ...state(), events: events.slice() };
    let acked = false;
    try {
      const answer = await send(report);
      acked = Boolean(answer && answer.ok === true && answer.seq === report.seq);
    } catch {
      acked = false;
    }
    inflight = false;
    if (acked) {
      attempt = 0;
      const sent = new Set(report.events.map((event) => event.id));
      events = events.filter((event) => !sent.has(event.id));
      onAcked(report);
      if (again) schedule();
      return;
    }
    const wait = retryMs[Math.min(attempt, retryMs.length - 1)];
    attempt += 1;
    schedule(wait);
  }

  return {
    schedule,
    push(event) {
      events.push(event);
      if (events.length > EVENT_LIMIT) {
        dropped += events.length - EVENT_LIMIT;
        events = events.slice(-EVENT_LIMIT);
      }
      schedule();
    },
    seq: () => seq,
    queued: () => events.slice(),
    dropped: () => dropped,
    stop() {
      stopped = true;
      if (timer) clearTimeout(timer);
      timer = null;
    },
  };
}

// serverOrigin answers the origin a key may be sent to, or null. https always; http
// only to loopback, which is where a test's stand-in server listens.
function serverOrigin(serverUrl) {
  let url;
  try {
    url = new URL(serverUrl);
  } catch {
    return null;
  }
  if (url.username || url.password) return null;
  if (url.protocol === 'https:') return url.origin;
  if (url.protocol === 'http:' && LOOPBACK.has(url.hostname)) return url.origin;
  return null;
}

// createSync is the service's sync: the held key (C2), discovery (C3) and, once the
// service and its data dir are given, the sessions Go asks for. A session runs Local's
// SyncManager, whose every write in a folder goes to Go through the root's document
// host (this process runs with the Go host switched on).
function createSync({ gate, service = null, dataDir = null }) {
  let held = null;
  let manager = null;
  // The wanted state the last configure gave, as Local's settings. `save` keeps the
  // rows the manager changed (a pause it recorded) for the next report to Go.
  let settings = { syncEnabled: true, roots: [], syncSessions: [] };
  const changedRows = new Map();
  const settingsStore = {
    get: () => settings,
    save(next) {
      settings = next;
      for (const row of next.syncSessions || []) changedRows.set(row.id, { ...row });
    },
  };
  const observers = new Map();
  // Sessions whose first download is running. Go leaves them out of configure until
  // the bind ends, so configure neither stops them nor drops their rows.
  const binding = new Set();
  let keyRefused = false;
  let accounts = null;
  let discoveryTimer = null;

  function startDiscovery() {
    if (discoveryTimer || !manager) return;
    const sync = manager;
    discoveryTimer = setInterval(() => {
      sync.refreshAccounts().catch((error) => console.error('[htmlclay-service] sync discovery failed:', error && error.message));
    }, DISCOVERY_MS);
    discoveryTimer.unref();
  }

  function stopDiscovery() {
    if (discoveryTimer) clearInterval(discoveryTimer);
    discoveryTimer = null;
  }
  // Event ids are "<session id>:<n>"; n carries this process's own prefix, so ids a
  // restarted service makes never repeat ones Go already stored and drops.
  const runId = randomBytes(4).toString('hex');
  let eventCount = 0;

  // heldRemoval is a session's removal of many attachments at once that the
  // engine holds for the person (engine-structure.js _holdMassDelete), or null.
  function heldRemoval(sessionId) {
    const entry = manager && manager.sessions.get(sessionId);
    const held = entry && entry.engine && entry.engine._structureState().massDelete;
    return held ? { files: held.files, paths: held.paths } : null;
  }

  const progress = new Map();
  function sessionsNow() {
    const rows = new Map((settings.syncSessions || []).map((row) => [row.id, row]));
    return manager ? manager.statuses().map((entry) => ({
      sessionId: entry.sessionId,
      rootId: entry.rootId,
      accountId: entry.accountId,
      status: entry.status,
      running: entry.running,
      paused: entry.paused,
      pendingCount: entry.pendingCount,
      conflicts: entry.conflicts.length,
      blocked: entry.blocked.length,
      blockedBy: entry.blocked.reduce((counts, file) => ({ ...counts, [file.reason || 'too-large']: (counts[file.reason || 'too-large'] || 0) + 1 }), {}),
      lastSyncAt: entry.lastSyncAt,
      lastError: entry.lastError,
      progress: progress.get(entry.sessionId) || null,
      removalHeld: heldRemoval(entry.sessionId),
      record: changedRows.get(entry.sessionId) || rows.get(entry.sessionId) || null,
    })) : [];
  }
  const state = () => ({ sessions: sessionsNow(), keyRefused, accounts });
  const reporter = service ? createReporter({
    send: (report) => service.goCall('POST', '/v1/sync/report', report),
    state,
    onAcked(report) {
      if (report.accounts === accounts) accounts = null;
    },
  }) : null;

  function listen(sync) {
    // A first download's progress (first-bind.js onProgress): reported while it
    // runs and dropped once the last file is in, so Go shows a bar only then.
    sync.on('sync-progress', (data) => {
      if (!data || !data.sessionId) return;
      if (data.phase === 'download' && Number.isInteger(data.done) && Number.isInteger(data.total) && data.done < data.total) {
        progress.set(data.sessionId, { phase: 'download', done: data.done, total: data.total, bytesDone: data.bytesDone || 0, bytesTotal: data.bytesTotal || 0 });
      } else {
        progress.delete(data.sessionId);
      }
      reporter.schedule();
    });
    for (const name of REPORTED) {
      sync.on(name, (data) => {
        if (name === 'credentials-rejected') keyRefused = true;
        if (name === 'accounts') accounts = (data && Array.isArray(data.accounts)) ? data.accounts : null;
        if (name === 'file-synced' && data && EVENT_ACTIONS.has(data.action) && data.sessionId) {
          eventCount += 1;
          reporter.push({
            id: `${data.sessionId}:${runId}-${eventCount}`,
            sessionId: data.sessionId,
            rootId: data.rootId,
            at: new Date().toISOString(),
            action: data.action,
            file: data.file,
            type: data.type || null,
          });
          return;
        }
        reporter.schedule();
      });
    }
  }

  async function accountById(accountId) {
    let answer;
    try {
      answer = await getAccounts({ serverUrl: held.serverOrigin, apiKey: held.key });
    } catch (error) {
      const status = error && error.statusCode;
      return { error: status === 401 || status === 403 ? 'key-refused' : 'offline' };
    }
    const account = ((answer && answer.accounts) || []).find((candidate) => candidate.id === accountId);
    return account ? { account } : { error: 'not-found' };
  }

  function managerFor(deviceId) {
    if (manager) return manager;
    const { SyncManager } = require('./vendor/local/src/main/sync-manager.js');
    const { passiveObserver } = require('./vendor/local/trial/sync-observer.cjs');
    manager = new SyncManager({
      userData: dataDir,
      logsDir: path.join(dataDir, 'logs'),
      deviceId,
      serverUrl: held.serverOrigin,
      getApiKey: () => (held ? held.key : null),
      settingsStore,
      // The engine follows Go's event stream itself (engine-watcher.js); an observer it
      // is given only stops it starting a file watcher of its own.
      observerFor: (rootId) => {
        if (!observers.has(rootId)) observers.set(rootId, passiveObserver());
        return observers.get(rootId);
      },
      takeSnapshot: (rel, rootId) => {
        const record = service.roots.get(rootId);
        return record ? record.goHost.takeSnapshot(rel) : null;
      },
      localAuthority: service.authority,
    });
    listen(manager);
    return manager;
  }

  async function stopAll() {
    stopDiscovery();
    if (!manager) return;
    const stopping = manager;
    manager = null;
    await stopping.stopAll();
    stopping.removeAllListeners();
    if (reporter) reporter.schedule();
  }

  return {
    // handle answers the stdout line for one {"sync":...} message. A cleared key, or
    // one for another server, stops every session first.
    async handle(message) {
      if (!message || message.v !== 1) return 'SYNC KEY REFUSED';
      if (message.key === null) {
        await stopAll();
        held = null;
        gate.clear();
        return 'SYNC KEY CLEARED';
      }
      const origin = serverOrigin(message.serverUrl);
      if (typeof message.key !== 'string' || !KEY.test(message.key) || origin === null) {
        return 'SYNC KEY REFUSED';
      }
      // Engines read the key they started with. A new key, even for the same server,
      // stops them all; Go's next configure starts the wanted sessions on it, so no
      // engine keeps writing to the previous key's account.
      if (held && (held.serverOrigin !== origin || held.key !== message.key)) await stopAll();
      if (!held || held.key !== message.key) keyRefused = false;
      held = { key: message.key, serverOrigin: origin };
      gate.hold(origin);
      return 'SYNC KEY HELD';
    },
    connection() { return held && { ...held }; },
    discover() { return discover(held); },

    // configure is C3's whole wanted state: each session that is enabled, not paused by
    // the switch and whose root Go gave runs; every other one stops. Nothing starts
    // without a key.
    async configure(body) {
      if (!held) return { ok: false, error: 'not-connected' };
      if (!service || !dataDir) return { ok: false, error: 'not-ready' };
      if (!body || !Array.isArray(body.sessions) || !Array.isArray(body.roots)) return { ok: false, error: 'bad-request' };
      const roots = new Map(body.roots.filter((root) => root && typeof root.id === 'string' && typeof root.path === 'string')
        .map((root) => [root.id, root]));
      const wanted = body.paused === true ? [] : body.sessions.filter((session) => session && session.enabled !== false && roots.has(session.rootId));
      const sync = managerFor(body.deviceId);
      // Paused: every session stops, a first download too, and nothing asks the
      // server anything until sync is switched back on.
      if (body.paused === true) {
        stopDiscovery();
        for (const id of [...sync.sessions.keys()]) await sync.stop(id);
        settings = { ...settings, roots: [], syncSessions: [] };
        reporter.schedule();
        return { ok: true, running: [] };
      }
      const wantedIds = new Set(wanted.map((session) => session.id));
      for (const id of [...sync.sessions.keys()]) {
        if (!wantedIds.has(id) && !binding.has(id)) await sync.stop(id);
      }
      const bound = (settings.syncSessions || []).filter((session) => binding.has(session.id) && !wantedIds.has(session.id));
      const usedRoots = new Map((settings.roots || []).filter((root) => bound.some((session) => session.rootId === root.id)).map((root) => [root.id, root]));
      for (const session of wanted) {
        const root = roots.get(session.rootId);
        usedRoots.set(root.id, { id: root.id, path: root.path, kind: session.kind });
      }
      settings = { ...settings, syncEnabled: true, roots: [...usedRoots.values()], syncSessions: [...wanted.map((session) => ({ ...session })), ...bound] };
      await sync.startEnabledSessions();
      startDiscovery();
      reporter.schedule();
      return { ok: true, running: [...sync.sessions.keys()] };
    },

    // preview is C3's count of what a first download of one account would bring.
    async preview(body) {
      if (!held) return { ok: false, error: 'not-connected' };
      if (!body || !Number.isInteger(body.accountId)) return { ok: false, error: 'bad-request' };
      const found = await accountById(body.accountId);
      if (found.error) return { ok: false, error: found.error };
      let inventory;
      try {
        inventory = await listNodes({ serverUrl: held.serverOrigin, apiKey: held.key, protocol: 2, syncBase: syncBaseOf(found.account) });
      } catch (error) {
        return { ok: false, error: 'offline' };
      }
      if (!inventory || inventory.complete !== true) return { ok: false, error: 'inventory-incomplete' };
      const nodes = [...inventory];
      const answer = { ok: true, ...inventoryTotals(nodes) };
      // With a folder, the counts for syncing into what is already there.
      if (body.root && typeof body.root.path === 'string' && path.isAbsolute(body.root.path)) {
        try {
          Object.assign(answer, await compareFolder(body.root.path, nodes));
        } catch {
          return { ok: false, error: 'not-found' };
        }
      }
      return answer;
    },

    // bind is C3's first download into one folder Go already serves. A team's first
    // download always starts empty: Go checks, and this checks again.
    async bind(body) {
      if (!held) return { ok: false, error: 'not-connected', resumable: false };
      if (!service || !dataDir) return { ok: false, error: 'not-ready', resumable: false };
      const session = body && body.session;
      const root = body && body.root;
      const mode = body && body.mode;
      if (!validSession(session) || !root || root.id !== session.rootId || typeof root.path !== 'string' || !['empty', 'existing'].includes(mode)) {
        return { ok: false, error: 'bad-request', resumable: false };
      }
      if (mode === 'existing' && session.kind === 'team') return { ok: false, error: 'folder-not-empty', resumable: false };
      if (!service.roots.has(root.id)) return { ok: false, error: 'not-found', resumable: false };
      const sync = managerFor(body.deviceId);
      if (binding.has(session.id) || sync.sessions.has(session.id)) return { ok: false, error: 'cancelled', resumable: true };
      binding.add(session.id);
      try {
        let discovery;
        try {
          discovery = await sync.refreshAccounts();
        } catch {
          return { ok: false, error: 'cancelled', resumable: true };
        }
        const account = ((discovery && discovery.accounts) || []).find((candidate) => candidate.id === session.accountId);
        if (!account) return { ok: false, error: 'not-found', resumable: false };
        // bind adds the row and the root to the settings itself.
        settings = {
          ...settings,
          roots: (settings.roots || []).filter((r) => r.id !== root.id),
          syncSessions: (settings.syncSessions || []).filter((s) => s.id !== session.id),
        };
        const result = await sync.bind({
          session: { ...session },
          root: { id: root.id, path: root.path, kind: session.kind },
          account,
          mode: mode === 'existing' ? 'existing' : 'new',
        });
        if (result.ok) return { ok: true, files: result.files || 0, bytes: result.bytes || 0 };
        return { ok: false, error: result.error || 'cancelled', resumable: Boolean(result.resumable) };
      } finally {
        binding.delete(session.id);
        reporter.schedule();
      }
    },

    // status is C4's report body, read by Go after an attach.
    status() {
      return { seq: reporter ? reporter.seq() : 0, ...state(), events: reporter ? reporter.queued() : [] };
    },

    // idle waits until every running session has nothing queued, or the timeout passes.
    async idle(body) {
      const timeoutMs = Number.isFinite(body && body.timeoutMs) ? Math.min(body.timeoutMs, 600000) : 60000;
      if (!manager) return { idle: true };
      const waits = [...manager.sessions.values()].map((entry) => (entry.runner ? entry.runner.whenIdle() : Promise.resolve()));
      let timer;
      const timedOut = new Promise((resolve) => { timer = setTimeout(() => resolve(false), timeoutMs); });
      const idle = await Promise.race([Promise.all(waits).then(() => true), timedOut]);
      clearTimeout(timer);
      return { idle };
    },

    // resolve is C3's conflict choice: the engine keeps mine, theirs or both for
    // one held conflict, writing through Go as every engine write does.
    async resolve(body) {
      if (!held) return { ok: false, error: 'not-connected' };
      const keep = body && body.keep;
      if (!['mine', 'theirs', 'both'].includes(keep)) return { ok: false, error: 'bad-choice' };
      if (!manager || typeof body.sessionId !== 'string' || typeof body.path !== 'string') return { ok: false, error: 'no-conflict' };
      const result = await manager.resolveConflict({ sessionId: body.sessionId, path: body.path, choice: keep });
      if (reporter) reporter.schedule();
      if (!result.ok && result.error === 'unknown') return { ok: false, error: 'no-conflict' };
      return result;
    },

    // removal is the person's choice for a held removal: restore puts the
    // attachments back here, delete removes them on the server too.
    async removal(body) {
      if (!held) return { ok: false, error: 'not-connected' };
      const choice = body && body.choice;
      if (choice !== 'restore' && choice !== 'delete') return { ok: false, error: 'bad-choice' };
      if (typeof body.sessionId !== 'string' || !heldRemoval(body.sessionId)) return { ok: false, error: 'no-removal' };
      const result = await manager.resolveMassDelete({ sessionId: body.sessionId, choice });
      if (reporter) reporter.schedule();
      return result;
    },

    async stopAll() {
      await stopAll();
    },
    close() {
      stopDiscovery();
      if (reporter) reporter.stop();
    },
  };
}

// discover answers C3's /sync/v1/discover body for the held key.
async function discover(held) {
  if (!held) return { ok: false, code: 'not-connected' };
  let answer;
  try {
    answer = await getAccounts({ serverUrl: held.serverOrigin, apiKey: held.key });
  } catch (error) {
    const status = error && error.statusCode;
    if (status === 401 || status === 403) return { ok: false, code: 'key-refused', status };
    return { ok: false, code: 'offline', ...(status ? { status } : {}) };
  }
  const features = (answer && answer.features) || {};
  if (!REQUIRED_FEATURES.every((name) => features[name] === true) || !(answer.protocol >= 2)) {
    return { ok: false, code: 'server-update-required' };
  }
  const actor = answer.actor;
  if (!actor || !Number.isInteger(actor.id) || typeof actor.username !== 'string' || !Array.isArray(answer.accounts)) {
    return { ok: false, code: 'bad-answer' };
  }
  return {
    ok: true,
    actor: { id: actor.id, username: actor.username, fullName: (actor.person && actor.person.name) || actor.username },
    accounts: answer.accounts,
  };
}

function readJson(req) {
  return new Promise((resolve) => {
    let size = 0;
    const chunks = [];
    req.on('data', (chunk) => {
      size += chunk.length;
      if (size > BODY_LIMIT) {
        resolve(undefined);
        req.destroy();
        return;
      }
      chunks.push(chunk);
    });
    req.on('end', () => {
      try {
        resolve(chunks.length ? JSON.parse(Buffer.concat(chunks).toString('utf8')) : {});
      } catch {
        resolve(undefined);
      }
    });
    req.on('error', () => resolve(undefined));
  });
}

function send(res, status, body) {
  res.writeHead(status, { 'content-type': 'application/json' });
  res.end(JSON.stringify(body));
}

// mountSync answers Go's calls under /sync/v1/ (C3). The listener has already
// checked the host, Go's bearer and both epochs before a mount is reached.
function mountSync(service, sync) {
  const routes = new Map([
    [`${PREFIX}discover`, () => sync.discover()],
    [`${PREFIX}configure`, (body) => sync.configure(body)],
    [`${PREFIX}status`, () => sync.status()],
    [`${PREFIX}idle`, (body) => sync.idle(body)],
    [`${PREFIX}preview`, (body) => sync.preview(body)],
    [`${PREFIX}bind`, (body) => sync.bind(body)],
    [`${PREFIX}resolve`, (body) => sync.resolve(body)],
    [`${PREFIX}removal`, (body) => sync.removal(body)],
  ]);
  service.mount(PREFIX, async (req, res) => {
    const { pathname } = new URL(req.url, 'http://service');
    const route = req.method === 'POST' ? routes.get(pathname) : undefined;
    if (!route) return send(res, 404, { ok: false, error: 'not-found' });
    const body = await readJson(req);
    if (body === undefined) return send(res, 400, { ok: false, error: 'bad-request' });
    try {
      send(res, 200, await route(body));
    } catch (error) {
      console.error('[htmlclay-service] sync route failed:', error && error.message);
      send(res, 500, { ok: false, error: 'failed' });
    }
  });
}

module.exports = { createSync, createReporter, mountSync, serverOrigin, EVENT_LIMIT };
