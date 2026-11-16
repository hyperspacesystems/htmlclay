const { loadCheckpoint } = require('./utils/page-api');
const { normCanonical, roomAt } = require('./local-authority');

const RETRY_MS = Object.freeze([1000, 5000, 30000]);
function createLocalCheckpoint({
  authority, immediate = setImmediate, cancelImmediate = clearImmediate,
  setTimer = setTimeout, clearTimer = clearTimeout, retryMs = RETRY_MS,
  logError = console.error, loadCore = loadCheckpoint, createDocument = null,
} = {}) {
  if (!authority) throw new TypeError('Checkpoint needs authority');
  const hosts = new Map();
  const running = new Map();
  const scheduled = new Map();
  const retries = new Map();
  const rerun = new Set();
  const permanentRefusals = new WeakSet();
  const permanentReasons = new Set(['invalid-document', 'unavailable', 'unsupported-projection', 'needs-adoption']);
  let autoNamer = null;
  let sweepTimer = null;
  const identityOf = (room) => String(room.nodeId ?? room.key);
  const dirty = (room) => Boolean(room) && authority.roomIsDirty(room);

  function clearRetry(identity) {
    const retry = retries.get(identity);
    if (retry?.timer) clearTimer(retry.timer);
    retries.delete(identity);
  }

  function clearPending(identity) {
    const waiting = scheduled.get(identity);
    if (waiting) {
      cancelImmediate(waiting.handle);
      waiting.resolve({ kind: 'cancelled' });
      scheduled.delete(identity);
    }
    const retry = retries.get(identity);
    if (retry?.timer) {
      clearTimer(retry.timer);
      retry.timer = null;
    }
  }

  function refusal(room, reason, error) {
    if (permanentReasons.has(reason)) {
      if (permanentRefusals.has(room)) return { kind: 'refused', reason };
      permanentRefusals.add(room);
    }
    logError('[local-checkpoint] checkpoint refused', {
      key: room.key, nodeId: room.nodeId, reason,
      ...(error ? { error: error.message } : {}),
    });
    return { kind: 'refused', reason };
  }

  function failure(room, reason, error) {
    const identity = identityOf(room);
    const retry = retries.get(identity) ?? { failures: 0, timer: null, exhausted: false };
    retries.set(identity, retry);
    if (retry.exhausted) return { kind: 'transient', reason, exhausted: true };
    retry.failures++;
    const delay = retryMs[retry.failures - 1];
    if (delay === undefined) {
      retry.exhausted = true;
      logError('[local-checkpoint] unsaved live room', {
        key: room.key, nodeId: room.nodeId, attempts: retry.failures, reason,
        ...(error ? { error: error.message } : {}),
      });
      return { kind: 'transient', reason, exhausted: true };
    }
    retry.timer = setTimer(() => {
      retry.timer = null;
      attempt(room.key).catch(() => {});
    }, delay);
    retry.timer.unref?.();
    return { kind: 'transient', reason, retryInMs: delay };
  }

  async function perform(key, room) {
    const target = authority.checkpointTargets(key).find((item) => hosts.has(item.rootId));
    if (!target || !dirty(room)) return target ? { kind: 'clean' } : refusal(room, 'invalid-identity');
    const generation = room.blockeditorTransition?.id ?? null;
    try {
      const result = await hosts.get(target.rootId).checkpoint(
        { name: target.logicalPath, filePath: key },
        { roomIdentity: target.identity, generation },
        { target, room });
      if (result.kind === 'ok' || result.kind === 'current') {
        clearRetry(identityOf(room));
        permanentRefusals.delete(room);
      }
      if (result.kind === 'refused') return refusal(room, result.reason, result.error ?? (result.message ? { message: result.message } : undefined));
      return result;
    } catch (error) {
      if (error.code === 'moved' || error.code === 'moved-or-deleted') return refusal(room, 'invalid-identity', error);
      return failure(room, 'operation-failed', error);
    }
  }

  function attempt(key, expectedRoom = null) {
    const room = roomAt(authority.rooms, key);
    key = normCanonical(key);
    if (expectedRoom && room !== expectedRoom) {
      return Promise.resolve(refusal(expectedRoom, 'invalid-identity'));
    }
    if (!room) return Promise.resolve({ kind: 'gone' });
    const identity = identityOf(room);
    if (running.has(identity)) return running.get(identity);
    clearPending(identity);
    const startedAt = room.seq;
    const work = Promise.resolve().then(() => perform(key, room));
    const settled = work.finally(() => {
      if (running.get(identity) === settled) running.delete(identity);
      const departedDuringWrite = rerun.delete(identity);
      if (roomAt(authority.rooms, room.key) === room && dirty(room) &&
          !authority.hasParticipatingEditors(room.key) &&
          (departedDuringWrite || room.seq !== startedAt) && !permanentRefusals.has(room)) {
        service.departed(room);
      }
      if (autoNamer && !dirty(room) && room.emptySince !== null && !authority.hasParticipatingEditors(room.key)) {
        autoNamer.departed(room);
      }
    });
    running.set(identity, settled);
    return settled;
  }

  const service = {
    registerHost(rootId, host) { hosts.set(rootId, host); },
    unregisterWriter(rootId) { hosts.delete(rootId); },
    setAutoNamer(service) { autoNamer = service; },
    confirmedWrite(key, stamp) { return autoNamer?.confirmedWrite(key, stamp); },
    mayExpire(room) { return permanentRefusals.has(room); },
    dirty,
    async safetyBackup(key, currentHtml) {
      const { splitPage } = require('@panphora/clayjs/server/rooms');
      const room = roomAt(authority.rooms, key);
      key = normCanonical(key);
      if (!dirty(room) || !/<article\b[^>]*\bdata-blockeditor(?:\s|=|>)/i.test(currentHtml)) return splitPage(currentHtml).file;
      const core = await loadCore();
      const capture = core.captureCheckpoint(room, { nodeId: room.nodeId, roomKey: key });
      const { JSDOM } = require('jsdom');
      const rendered = core.renderCheckpoint(currentHtml, capture, {
        createDocument: createDocument ?? (() => new JSDOM('<!doctype html><html><body></body></html>', {
          runScripts: undefined, resources: undefined,
        }).window.document),
      });
      if (rendered.kind !== 'ok') throw new Error(`Cannot back up live room: ${rendered.reason}`);
      return splitPage(rendered.html).file;
    },
    attempt,
    pending(key) {
      const room = roomAt(authority.rooms, key);
      if (!room) return null;
      const identity = identityOf(room);
      return running.get(identity) || scheduled.get(identity)?.promise || null;
    },
    departed(room) {
      if (!dirty(room)) {
        autoNamer?.departed(room);
        return false;
      }
      const identity = identityOf(room);
      if (running.has(identity)) { rerun.add(identity); return false; }
      if (scheduled.has(identity)) return false;
      let resolve;
      const promise = new Promise(done => { resolve = done; });
      const handle = immediate(async () => {
        scheduled.delete(identity);
        try { resolve(await attempt(room.key, room)); }
        catch (error) { resolve({ kind: 'transient', reason: 'operation-failed', error }); }
      });
      scheduled.set(identity, { handle, promise, resolve });
      autoNamer?.departed(room);
      return true;
    },
    arrived(key) {
      const room = roomAt(authority.rooms, key);
      if (!room) return false;
      const identity = identityOf(room);
      const waiting = scheduled.get(identity);
      if (!waiting) return false;
      cancelImmediate(waiting.handle);
      waiting.resolve({ kind: 'cancelled' });
      scheduled.delete(identity);
      return true;
    },
    wake(key) {
      const room = roomAt(authority.rooms, key);
      key = normCanonical(key);
      const identity = room ? identityOf(room) : key;
      clearRetry(identity);
      if (room && dirty(room) && !authority.hasParticipatingEditors(key)) service.departed(room);
    },
    retry(key) {
      const room = roomAt(authority.rooms, key);
      if (room) clearRetry(identityOf(room));
      return attempt(key);
    },
    async flushDirty({ timeoutMs = 5000, rootId = null } = {}) {
      const pending = authority.rooms.keys().filter((key) => dirty(authority.rooms.get(key)) &&
        (rootId === null || authority.checkpointTargets(key).some((target) => target.rootId === rootId)))
        .map((key) => attempt(key));
      let timedOut = false;
      let handle;
      await Promise.race([
        Promise.allSettled(pending),
        new Promise((resolve) => { handle = setTimer(() => { timedOut = true; resolve(); }, timeoutMs); }),
      ]);
      if (handle) clearTimer(handle);
      const unsaved = authority.rooms.keys().filter((key) => dirty(authority.rooms.get(key)) &&
        (rootId === null || authority.checkpointTargets(key).some((target) => target.rootId === rootId)));
      for (const key of unsaved) logError('[local-checkpoint] room still unsaved at shutdown', {
        key, nodeId: authority.rooms.get(key).nodeId,
      });
      return { attempted: pending.length, unsaved, timedOut };
    },
    startSweep() {
      if (sweepTimer) return;
      sweepTimer = setInterval(() => authority.sweep(), 30000);
      sweepTimer.unref?.();
    },
    stopSweep() {
      if (!sweepTimer) return;
      clearInterval(sweepTimer);
      sweepTimer = null;
    },
  };
  return service;
}

module.exports = { createLocalCheckpoint, RETRY_MS };
