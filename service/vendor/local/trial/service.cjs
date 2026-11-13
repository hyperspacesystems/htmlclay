'use strict';

// The managed trial's JS service (trial/protocol/v1.md): Local's room code run
// beside the Go host. Go is the only writer of a served root and holds a
// document's lock while it consults this process. Each consultation runs
// Local's own lane halves (document-host.js `lanes`) inside this process's own
// document boundary, which stays open from `admit` to `settle` so the write
// token Local's authority asserts is the one the write was admitted under. No
// file is written here: what the room code writes (a safety version, a
// recovered copy) leaves as an artifact that Go writes and receipts.

const http = require('http');
const path = require('path');
const { realpathSync } = require('fs');
const { randomBytes, timingSafeEqual } = require('crypto');
const { AsyncLocalStorage } = require('async_hooks');

const { createLocalAuthority } = require('../src/main/local-authority.js');
const { createLocalCheckpoint } = require('../src/main/local-checkpoint.js');
const { createRootDocumentHost } = require('../src/main/document-host.js');
const { withDocumentWrite, withStructureChange } = require('../src/main/utils/write-queue.js');
const { withReader, disk } = require('../src/main/document-reader.js');
const { createRootLive } = require('../src/main/utils/root-live.js');
const { RootObserver } = require('../src/main/root-observer.js');
const { pageWriteFailure, createApp, getAndClearSnapshot } = require('../src/main/server.js');
const { writeContexts } = require('../src/main/write-contexts.js');
const { documentEtag } = require('../src/main/spec-wire.js');
const { logicalBytes } = require('../src/main/page-state-store.js');
const { newestVersionFiles } = require('../src/main/page-rebase.js');
const { loadCheckpoint } = require('../src/main/utils/page-api.js');
const { setConnection, getConnection } = require('./go-process.cjs');
const {
  createGoDocumentHost, checkpointContext, configureService, markCheckpointUnproven, checkpointUnproven,
} = require('./go-document-host.cjs');

const roomsPackage = () => require('@panphora/clayjs/server/rooms');

const BEARER = 'Bearer ';
const MAX_BODY = 64 * 1024 * 1024;
const RECONCILE_MEMORY_MS = 5 * 60 * 1000;
// How long a new service keeps asking Go for the documents an earlier service
// left unconfirmed, and how long it waits between asks.
const RECOVER_ATTEMPTS = 20;
const RECOVER_DELAY_MS = 500;
// The room-owning public routes Go forwards arrive under this prefix.
const ROUTE_PREFIX = '/v1/route';
const SUPERVISOR_PATHS = new Set(['/v1/flush', '/v1/peer', '/v1/snapshot/take', '/v1/checkpoint-wait', '/v1/roots']);

// The consultation a piece of room code is running for, so the authority's
// recovered-copy writer knows where its artifact goes.
const consultScope = new AsyncLocalStorage();

function text(value) {
  if (!value) return null;
  return value.encoding === 'base64' ? Buffer.from(value.data, 'base64').toString('utf8') : value.data;
}

function raw(value) {
  if (!value) return null;
  return value.encoding === 'base64' ? Buffer.from(value.data, 'base64') : Buffer.from(value.data, 'utf8');
}

function encode(value) {
  if (value === null || value === undefined) return null;
  if (Buffer.isBuffer(value)) return { encoding: 'base64', data: value.toString('base64') };
  return { encoding: 'utf8', data: String(value) };
}

function missing(file) {
  return Object.assign(new Error(`ENOENT: no such file or directory, open '${file}'`), { code: 'ENOENT' });
}

// A stat answered from the identity Go reported: its fields as BigInts for a
// `{ bigint: true }` stat, as numbers otherwise.
function identityStat(identity, big) {
  const field = (value) => (big ? BigInt(value) : Number(value));
  const { dev, ino, size, mtimeNs } = identity;
  return {
    dev: field(dev), ino: field(ino), size: field(size), mtimeNs: field(mtimeNs),
    isFile: () => true, isDirectory: () => false, isSymbolicLink: () => false,
  };
}

// The reader a handler runs under while Go holds the document's lock (S5): the
// document, its blockeditor-state sidecar and the version files Go sent come from the
// call's snapshot, and the version list is exactly the names Go sent. A call
// that carries no `versions` (observe) lists and reads the document's history
// from the disk, read-only: Go is its only writer and holds the document's lock.
// Any other path (a legacy sidecar probe, a sibling's stat) is read from the
// disk, read-only. Once Go has written the document (`settled`), the document
// and its sidecar are read from the disk Go just wrote under the lock it still
// holds, and the document's stat answers with the identity Go reported, so the
// stamp a confirmation records is the written file's.
function snapshotReader(root, body) {
  const current = body.current ?? null;
  const document = path.resolve(body.canonicalPath);
  const sidecar = path.resolve(root.path, '.hyperclay', 'blockeditor-state', `${body.name}.json`);
  const versionsDir = path.resolve(root.path, '.hyperclay', 'versions', body.name.replace(/\.(html|htmlclay)$/i, ''));
  const sentVersions = Array.isArray(body.versions);
  const versions = new Map((body.versions ?? []).map((version) => [version.name, raw(version.bytes)]));
  // `done` once the handler that made this reader has finished: work Local
  // scheduled inside it (a checkpoint wake, a timer) inherits the reader's
  // scope and from then on reads the disk, read-only, like any other read
  // outside a consultation.
  const state = { settled: false, done: false, identity: current?.identity ?? null };
  return {
    state,
    async readFile(file, encoding) {
      if (state.done) return disk.readFile(file, encoding);
      const resolved = path.resolve(file);
      let bytes;
      if (!state.settled && resolved === document) {
        if (!current || !current.exists) throw missing(file);
        bytes = raw(current.bytes);
      } else if (!state.settled && resolved === sidecar && current) {
        if (!current.sidecar) throw missing(file);
        bytes = Buffer.from(JSON.stringify(current.sidecar), 'utf8');
      } else if (path.dirname(resolved) === versionsDir && versions.has(path.basename(resolved))) {
        bytes = versions.get(path.basename(resolved));
      } else {
        return disk.readFile(file, encoding);
      }
      return encoding ? bytes.toString(encoding) : bytes;
    },
    async stat(file, options) {
      if (state.done || path.resolve(file) !== document) return disk.stat(file, options);
      if (!state.identity) throw missing(file);
      return identityStat(state.identity, Boolean(options && options.bigint));
    },
    versionFiles: async (dir, baseDir) => (state.done || !sentVersions ? newestVersionFiles(dir, baseDir) : [...versions.keys()]),
  };
}

// One consultation: a handler running in this process while Go holds the
// document's lock, answering each of Go's calls in turn. `reply` is the answer
// function of the call Go is waiting on. An artifact the handler needs written
// is sent as a pending batch on that call, and the handler stays suspended
// until Go's receipt arrives on the next one.
class Consultation {
  constructor(id, reply) {
    this.id = id;
    this.reply = reply;
    this.batch = [];
    this.waiting = new Map();
    this.resume = null;
    this.aborted = null;
    this.flushing = false;
  }

  answer(body) {
    const reply = this.reply;
    this.reply = null;
    if (reply) reply(body);
  }

  // Artifacts written in the same turn leave as one batch, on whichever of Go's
  // calls is waiting for this consultation's answer.
  flush() {
    if (this.flushing) return;
    this.flushing = true;
    setImmediate(() => {
      this.flushing = false;
      if (this.batch.length && this.reply) this.answer({ pending: true, artifacts: this.batch.splice(0) });
    });
  }

  write(artifact) {
    if (this.aborted) return Promise.resolve({ artifactId: null, error: `aborted: ${this.aborted}` });
    const artifactId = randomBytes(8).toString('hex');
    const receipt = new Promise((resolve) => this.waiting.set(artifactId, resolve));
    this.batch.push({ artifactId, ...artifact });
    this.flush();
    return receipt;
  }

  receive(receipts, reply) {
    this.reply = reply;
    for (const receipt of receipts ?? []) {
      const resolve = this.waiting.get(receipt.artifactId);
      this.waiting.delete(receipt.artifactId);
      if (resolve) resolve(receipt);
    }
    if (this.batch.length) this.flush();
  }

  // The handler's wait for Go's settle or abort. An abort that came first
  // answers at once, so a handler never holds its boundary for a write Go gave up.
  next() {
    if (this.aborted) return Promise.resolve({ abort: this.aborted });
    return new Promise((resolve) => { this.resume = resolve; });
  }

  continueWith(step, reply) {
    if (reply) this.reply = reply;
    const resume = this.resume;
    this.resume = null;
    if (resume) resume(step);
    if (this.batch.length) this.flush();
  }

  abort(reason) {
    this.aborted = reason;
    for (const [artifactId, resolve] of this.waiting) resolve({ artifactId, error: `aborted: ${reason}` });
    this.waiting.clear();
    this.batch = [];
    this.continueWith({ abort: reason }, null);
  }
}

// Local's answer to a write its admission refused, shaped exactly as the route
// the write came through answers it: the data API's catch (server.js
// `handleApiWrite`) for lane `api`, /save's catch otherwise. `stamp` is the
// ETag /save adds to a stale-lineage or nothing-to-write refusal: the stored
// document's, as /save computes it. The replace lane's refusal codes are mapped
// by Go's restore routes themselves, as Local's are.
function refusalFor(lane, error, stamp = null) {
  const message = error && error.message ? error.message : String(error);
  const movedOrDeleted = { status: 409, code: 'moved-or-deleted', body: { msg: message, msgType: 'error', code: 'moved-or-deleted' } };
  if (lane === 'api') {
    if (error && error.code === 'moved-or-deleted') return movedOrDeleted;
    if (error && error.name === 'PersistentReservationError') {
      const status = error.code === 'checkpointing' ? 503 : 409;
      return { status, code: error.code, body: { code: error.code, error: error.message } };
    }
    const failure = pageWriteFailure(error);
    if (failure) {
      return {
        status: failure.statusCode, code: failure.code,
        body: { code: failure.code, reason: failure.reason, minimum: failure.minimum, msg: failure.message },
      };
    }
    return { status: 500, code: 'failed', body: { error: 'Internal server error', message: 'An unexpected error occurred' } };
  }
  const failure = pageWriteFailure(error);
  if (failure) {
    const headers = {};
    if (failure.retryAfter !== undefined) headers['Retry-After'] = String(failure.retryAfter);
    const etag = failure.code === 'blockeditor-lineage-stale' || failure.code === 'blockeditor-nothing-to-write' ? stamp : null;
    if (etag) headers.ETag = etag;
    return {
      status: failure.statusCode,
      code: failure.code,
      headers,
      body: {
        msg: failure.message,
        msgType: 'error',
        code: failure.code,
        ...(failure.reason === undefined ? {} : { reason: failure.reason }),
        ...(failure.minimum === undefined ? {} : { minimum: failure.minimum }),
        ...(etag ? { etag } : {}),
      },
    };
  }
  if (error && error.code === 'moved-or-deleted') return movedOrDeleted;
  if (error && error.code === 'unsaved-edits') {
    return { status: 409, code: 'unsaved-edits', body: { msg: message, msgType: 'error', code: 'unsaved-edits' } };
  }
  return { status: 500, code: 'failed', body: { msg: `Server error saving file: ${message}`, msgType: 'error' } };
}

// The stamp /save puts on a stale-lineage refusal: the stored document's
// logical bytes, read from the call's snapshot (run inside its reader scope).
async function storedStamp(root, body) {
  const current = body.current;
  if (!current || !current.exists) return null;
  try {
    return documentEtag(await logicalBytes(root.path, body.name, raw(current.bytes)));
  } catch {
    return null;
  }
}

// Local's own in-process host, whose lane halves the handlers run. It is built
// with the trial switch off for this one call, whatever the process sets for
// the routes, which write through Go.
function inProcessLanes(ctx) {
  const saved = process.env.HYPERCLAY_LOCAL_DOCUMENT_HOST;
  delete process.env.HYPERCLAY_LOCAL_DOCUMENT_HOST;
  try {
    return createRootDocumentHost({ ctx, live: ctx.live, getObserver: () => ctx.observer }).lanes;
  } finally {
    if (saved !== undefined) process.env.HYPERCLAY_LOCAL_DOCUMENT_HOST = saved;
  }
}

function saveOrigin(lane, origin) {
  if (origin === 'checkpoint') return 'checkpoint';
  return lane === 'api' ? 'api' : 'save';
}

function createTrialService({
  secret = randomBytes(32).toString('hex'),
  epoch = randomBytes(8).toString('hex'),
  logError = (...args) => console.error('[trial-service]', ...args),
} = {}) {
  const roots = new Map();
  const rootsByPath = new Map();
  const consultations = new Map();
  const touched = new Map();
  const tombstones = new Map();
  const handlers = new Map();
  let go = null;

  // Every Local host this process builds writes through Go: the app mounted on
  // the forwarded routes, the observer's, the checkpoint's. The lane halves the
  // handlers run are the one exception (inProcessLanes), and they never write.
  process.env.HYPERCLAY_LOCAL_DOCUMENT_HOST = 'go';

  const authority = createLocalAuthority({ withDocument: withDocumentWrite, publishRecovered });
  const checkpoint = createLocalCheckpoint({ authority });
  authority.setCheckpointService(checkpoint);
  // The adapter records the admission claim of each checkpoint it sends, which
  // the render-checkpoint handler validates inside its boundary, as Local's
  // in-process checkpoint validates its pre-lock claim.
  configureService({ checkpointClaim: (target) => authority.captureAdmission(target) });

  function rootFor(id) {
    const root = roots.get(id);
    if (!root) throw Object.assign(new Error(`unknown root ${id}`), { code: 'unknown-root' });
    return root;
  }

  // The authority's recovered-copy writer: inside a consultation the copy is an
  // artifact of that consultation's next batch; outside one it is a
  // POST /v1/artifact. Either way it counts as published only on Go's receipt.
  async function publishRecovered(rootPath, rel, html) {
    const root = rootsByPath.get(path.resolve(rootPath));
    if (!root) throw new Error(`recovered copy for an unknown root ${rootPath}`);
    const current = openConsultation();
    const receipt = current
      ? await current.write({ type: 'recovered-copy', root: root.id, name: rel, logical: encode(html) })
      : await goCall('POST', '/v1/artifact', {
        opId: randomBytes(16).toString('hex'), root: root.id, name: rel,
        artifactId: randomBytes(8).toString('hex'), type: 'recovered-copy', logical: encode(html),
      });
    if (receipt.error) throw new Error(`recovered copy of ${rel} was not written: ${receipt.error}`);
    return { filename: receipt.version };
  }

  function artifactWriters(consultation, root, name) {
    return {
      safetyVersion: async (html) => {
        const receipt = await consultation.write({ type: 'safety-version', logical: encode(html) });
        return receipt.error ? null : { filename: receipt.version };
      },
      recoveredCopy: async (html) => {
        const receipt = await consultation.write({ type: 'recovered-copy', root: root.id, name, logical: encode(html) });
        if (receipt.error) throw new Error(`recovered copy of ${name} was not written: ${receipt.error}`);
        return { filename: receipt.version };
      },
    };
  }

  // A call to Go's control API. Never made from a handler Go is waiting on:
  // Go holds the document's lock for the whole consultation.
  function goCall(method, pathname, body) {
    const connection = getConnection();
    const payload = body === undefined ? null : Buffer.from(JSON.stringify(body), 'utf8');
    const headers = {
      authorization: `${BEARER}${connection.secret}`,
      'x-trial-epoch': epoch,
      'x-trial-peer-epoch': connection.peerEpoch ?? '',
    };
    if (payload) {
      headers['content-type'] = 'application/json';
      headers['content-length'] = String(payload.length);
    }
    return new Promise((resolve, reject) => {
      const request = http.request(`${connection.url}${pathname}`, { method, headers }, (response) => {
        const chunks = [];
        response.on('data', (chunk) => chunks.push(chunk));
        response.on('end', () => {
          const answer = Buffer.concat(chunks).toString('utf8');
          if (response.statusCode !== 200) {
            reject(Object.assign(new Error(`Go ${pathname} answered ${response.statusCode}: ${answer}`), { status: response.statusCode }));
            return;
          }
          try {
            resolve(answer ? JSON.parse(answer) : null);
          } catch (error) {
            reject(error);
          }
        });
        response.on('error', reject);
      });
      request.on('error', reject);
      if (payload) request.write(payload);
      request.end();
    });
  }

  function open(consultId, reply) {
    const consultation = new Consultation(consultId, reply);
    consultations.set(consultId, consultation);
    return consultation;
  }

  // Bounded per-consultation memory for the current Go epoch: the document each
  // consultation touched (so a `settle-failed` abort can reconcile it even when
  // its settle never arrived) and the consultations aborted before their admit
  // reached this process.
  function prune(map) {
    const now = Date.now();
    for (const [id, entry] of map) {
      if (now - entry.at > RECONCILE_MEMORY_MS) map.delete(id);
    }
  }

  function touch(body) {
    prune(touched);
    if (body.name && !touched.has(body.consultId)) {
      touched.set(body.consultId, { root: body.root, name: body.name, canonicalPath: body.canonicalPath, at: Date.now() });
    }
  }

  // Runs one handler for a consultation, in its scope, and forgets the
  // consultation once the handler is done. A handler that throws before it
  // answered answers with `fallback`.
  function run(consultation, fn, fallback) {
    consultScope.run(consultation, fn).catch((error) => {
      logError(`consultation ${consultation.id} failed:`, error);
      consultation.answer(fallback(error));
    }).finally(() => {
      consultations.delete(consultation.id);
    });
  }

  // The consultation a piece of room code runs for, if it is still open. A
  // timer Local schedules inside a handler inherits that handler's scope; once
  // the handler is done, its work is no longer part of the consultation.
  function openConsultation() {
    const current = consultScope.getStore();
    return current && consultations.get(current.id) === current && !current.aborted ? current : null;
  }

  async function admitLane(consultation, root, body, target, token, context) {
    const { lane, name } = body;
    const filePath = body.canonicalPath;
    const headers = body.headers ?? {};
    const incoming = text(body.incoming);
    const writers = artifactWriters(consultation, root, name);
    if (lane === 'save' || lane === 'api') {
      const blockeditorConversion = headers['blockeditor-conversion'] ?? null;
      const prepared = await root.lanes.prepareSave({ name, bytes: incoming, blockeditorConversion });
      return root.lanes.admitSave({
        name, filePath, origin: saveOrigin(lane, body.origin),
        content: prepared.content, fileContent: prepared.fileContent,
        dataLossPrev: text(body.dataLossPrev) ?? text(body.current?.logical),
        ifMatch: headers['if-match'] ?? null, blockeditorConversion, blockeditorWrite: headers['blockeditor-write'] ?? null,
        pageFormat: headers['blockeditor-format'] ?? null, persistentToken: context?.persistentToken ?? null,
        target, token, writeSafetyVersion: writers.safetyVersion,
      });
    }
    if (lane === 'replace') {
      const prepared = await root.lanes.prepareReplace({ name, bytes: incoming });
      const admitted = await root.lanes.admitReplace({
        name, filePath, content: prepared.content, edit: Boolean(body.edit), safety: Boolean(body.safety),
        target, token, writeSafetyVersion: writers.safetyVersion, writeRecoveredCopy: writers.recoveredCopy,
      });
      return {
        content: prepared.content, fileContent: prepared.fileContent,
        pageToken: admitted.pageToken, refusal: admitted.refusal,
      };
    }
    if (lane === 'rebase') {
      await root.lanes.admitRebase({ target, token });
      return { content: incoming, fileContent: roomsPackage().splitPage(incoming).file, pageToken: null };
    }
    throw new TypeError(`admit: unknown lane ${lane}`);
  }

  async function settleLane(root, body, settle, admitted, target, token, context) {
    const { lane, name } = body;
    const logical = text(settle.logical);
    const file = text(settle.file);
    if (lane === 'save' || lane === 'api') {
      await root.lanes.settleSave({
        name, origin: saveOrigin(lane, body.origin), content: logical, fileContent: file,
        pageToken: admitted.pageToken, target, token, persistentToken: context?.persistentToken ?? null,
      });
      root.lanes.recordSaveProvenance(name, settle.userDriven ?? undefined);
      return;
    }
    if (lane === 'replace') {
      await root.lanes.settleReplace({ name, content: logical, pageToken: admitted.pageToken, target, token });
      return;
    }
    if (lane === 'rebase') {
      // Local's observer writes the rebase, then observes it (root-observer.js
      // `_classifyHtml`): the room continues or is replaced and the tabs are told.
      await root.lanes.settleRebase({ bytes: logical, target, token });
      await root.observer._applyExternal(name, target, token, Buffer.from(logical, 'utf8'));
    }
  }

  // POST /v1/admit: the write's admission, then, in the same boundary, its
  // settle or its abort.
  async function runWrite(consultation, body) {
    const root = rootFor(body.root);
    const filePath = body.canonicalPath;
    const location = { rootId: root.id, logicalPath: body.name, canonicalPath: filePath };
    const reader = snapshotReader(root, body);
    const context = body.opHandle ? writeContexts.get(body.opHandle) ?? null : null;
    const claim = authority.captureAdmission(location);
    await withReader(reader, () => withDocumentWrite(async () => ({ canonicalPath: filePath }), async (_locked, token) => {
      if (consultation.aborted) return;
      let target;
      let admitted;
      try {
        target = authority.admitWriteTarget(claim, { ...location, origin: root.origin, live: root.live }, token,
          { explicitCreate: Boolean(body.explicitCreate) });
        admitted = await admitLane(consultation, root, body, target, token, context);
      } catch (error) {
        const stamp = body.lane === 'save' ? await storedStamp(root, body) : null;
        consultation.answer({ decision: 'refuse', ...refusalFor(body.lane, error, stamp) });
        return;
      }
      try {
        if (admitted.refusal) {
          consultation.answer({ decision: 'refuse', status: 500, code: admitted.refusal, body: {} });
          return;
        }
        if (consultation.aborted) return;
        const next = consultation.next();
        consultation.answer({
          decision: 'proceed', logical: encode(admitted.content), file: encode(admitted.fileContent),
          reservation: admitted.pageToken ? consultation.id : null, artifacts: [],
        });
        const step = await next;
        if (!step.settle) return;
        try {
          if (step.settle.outcome === 'committed') {
            reader.state.settled = true;
            reader.state.identity = step.settle.identity ?? null;
            await settleLane(root, body, step.settle, admitted, target, token, context);
          } else if (step.settle.outcome === 'indeterminate' && body.origin === 'checkpoint') {
            markCheckpointUnproven(filePath);
          }
          consultation.answer({ ok: true });
        } catch (error) {
          logError(`settle of ${body.name} failed:`, error);
          consultation.answer({ ok: false, error: error && error.message ? error.message : String(error) });
        }
      } finally {
        if (admitted.pageToken) authority.rooms.releasePage(admitted.pageToken);
      }
    })).finally(() => {
      reader.state.done = true;
    });
  }

  // POST /v1/settle for an `exact` or `create` write, which has no admit.
  async function runSettleOnly(consultation, body) {
    if (body.outcome !== 'committed') {
      consultation.answer({ ok: true });
      return;
    }
    const root = rootFor(body.root);
    const location = { rootId: root.id, logicalPath: body.name, canonicalPath: body.canonicalPath };
    const reader = snapshotReader(root, { ...body, current: null });
    reader.state.settled = true;
    reader.state.identity = body.identity ?? null;
    const claim = authority.captureAdmission(location);
    await withReader(reader, () => withDocumentWrite(async () => ({ canonicalPath: body.canonicalPath }), async (_locked, token) => {
      const target = authority.admitWriteTarget(claim, { ...location, origin: root.origin, live: root.live }, token,
        { explicitCreate: body.lane === 'create' });
      if (body.lane === 'exact') await root.lanes.settleExact({ name: body.name, stored: text(body.file), target, token });
      else await root.lanes.settleCreate({ bytes: text(body.logical), target, token });
      consultation.answer({ ok: true });
    })).finally(() => {
      reader.state.done = true;
    });
  }

  // A document whose settle failed or was lost: Local's room is brought back to
  // what is on disk, read from Go once its lock is free, and Go is told.
  async function reconcile({ root: rootId, name, canonicalPath }) {
    const root = rootFor(rootId);
    const query = `?root=${encodeURIComponent(rootId)}&name=${encodeURIComponent(name)}`;
    const snapshot = await goCall('GET', `/v1/read${query}`);
    const reader = snapshotReader(root, { name, canonicalPath, current: snapshot });
    await withReader(reader, () => withDocumentWrite(async () => ({ canonicalPath }), async (_locked, token) => {
      const target = authority.bindTarget({ rootId, logicalPath: name, canonicalPath, origin: root.origin, live: root.live });
      if (!snapshot.exists) {
        await authority.observeExternal(target, null, token, { removed: true });
        return;
      }
      await authority.reconcileDisk(target, text(snapshot.logical) ?? text(snapshot.bytes), token);
    })).finally(() => {
      reader.state.done = true;
    });
    await goCall('POST', '/v1/reconciled', { root: rootId, name });
  }

  // Documents Go still holds unconfirmed, because a settle was lost with an
  // earlier service process: each is reconciled from disk and released, as a
  // settle-failed abort would have done. Without this, every later save of such
  // a document answers 503 until the whole host restarts.
  async function recoverUnconfirmed() {
    for (let attempt = 0; attempt < RECOVER_ATTEMPTS; attempt += 1) {
      let answer = null;
      try {
        answer = await goCall('GET', '/v1/unconfirmed');
      } catch {
        await new Promise((resolve) => setTimeout(resolve, RECOVER_DELAY_MS));
        continue;
      }
      let failed = 0;
      for (const entry of (answer && answer.documents) || []) {
        const record = roots.get(entry.root);
        if (!record) continue;
        try {
          await reconcile({ root: entry.root, name: entry.name, canonicalPath: path.join(record.path, entry.name) });
        } catch (error) {
          failed += 1;
          logError(`reconcile of ${entry.name} after a service restart failed:`, error);
        }
      }
      if (failed === 0) return;
      await new Promise((resolve) => setTimeout(resolve, RECOVER_DELAY_MS));
    }
  }

  // POST /v1/render-checkpoint: Local's renderCheckpoint for a checkpoint the
  // service started through Go (the adapter's POST /v1/checkpoint). The room,
  // its target and the admission claim are what the adapter recorded under the
  // opHandle. The handler leaves its boundary before answering: on `render`, Go
  // runs its save lane, which consults `admit` and `settle` like any save.
  async function runRenderCheckpoint(consultation, body) {
    const root = rootFor(body.root);
    const pending = body.opHandle ? checkpointContext(body.opHandle) : null;
    if (!pending) {
      consultation.answer({ kind: 'refused', reason: 'invalid-identity', artifacts: [] });
      return;
    }
    let core;
    try {
      core = await loadCheckpoint();
    } catch (error) {
      consultation.answer({ kind: 'refused', reason: 'checkpoint-unavailable', artifacts: [] });
      return;
    }
    const reader = snapshotReader(root, body);
    const rendered = await withReader(reader, () => withDocumentWrite(async () => ({ canonicalPath: body.canonicalPath }), async (_locked, token) => {
      if (consultation.aborted) return null;
      return root.lanes.renderCheckpoint({
        core, key: body.canonicalPath, roomIdentity: body.roomIdentity, generation: body.generation ?? null,
        target: pending.target, room: pending.room, claim: pending.claim, token,
        readCurrent: async () => {
          if (!body.current || !body.current.exists) throw missing(body.canonicalPath);
          return { html: text(body.current.bytes), readIdentity: identityStat(body.current.identity, true) };
        },
      });
    })).finally(() => {
      reader.state.done = true;
    });
    if (!rendered) return;
    if (rendered.kind === 'render') {
      consultation.answer({ kind: 'render', logical: encode(rendered.html), revision: rendered.revision ?? null, captureId: null, artifacts: [] });
      return;
    }
    consultation.answer({ kind: rendered.kind, reason: rendered.reason ?? '', artifacts: [] });
  }

  // POST /v1/observe: Local's observer judging an outside add, change or
  // removal of a page document that Go's watcher saw (root-observer.js
  // `_observeHtml`, `_observeUnlink`). Go's own writes reach it too and are
  // judged `self`. A rebase is answered for Go to write through its external
  // rebase lane, whose settle tells the room (`settleLane`); an outside edit is
  // observed here and the tabs told.
  async function runObserve(consultation, body) {
    const root = rootFor(body.root);
    const { name, canonicalPath, event } = body;
    const reader = snapshotReader(root, body);
    let answer = { action: 'none', logical: null, artifacts: [] };
    let removal = null;
    let removed = false;
    await withReader(reader, () => withDocumentWrite(async () => ({ canonicalPath }), async (_locked, token) => {
      if (consultation.aborted) return;
      const current = body.current;
      if (!current || !current.exists) {
        if (event !== 'unlink') return;
        removal = await authority.observeExternal({ rootId: root.id, logicalPath: name, canonicalPath, live: root.live }, null, token, { removed: true });
        removed = true;
        return;
      }
      // A file that is back on disk: the unlink was a delayed event for a replacement.
      if (event === 'unlink') return;
      const bytes = raw(current.bytes);
      if (bytes.length === 0) return;
      // The file a checkpoint wrote without a proven fsync: the room keeps those
      // edits unsaved until the adapter's /v1/durable proof (protocol v1), and
      // its next checkpoint reads the file through renderCheckpoint's own
      // reconcileDisk, so nothing is applied from it here.
      if (checkpointUnproven(canonicalPath)) return;
      const target = authority.bindTarget({ rootId: root.id, logicalPath: name, canonicalPath, origin: null, live: root.live });
      const judged = await root.observer._judgeHtml(canonicalPath, name, target, bytes, identityStat(current.identity, true));
      if (judged.kind === 'self') return;
      if (judged.kind === 'rebase') {
        answer = { action: 'rebase', logical: encode(judged.observed.toString('utf8')), artifacts: [] };
        return;
      }
      await root.observer._applyExternal(name, target, token, judged.observed);
    })).finally(() => {
      reader.state.done = true;
    });
    if (removed) root.observer._notifyRemoved(name, removal);
    consultation.answer(answer);
  }

  // POST /v1/structure/prepare: Local's structure plan for a move, trash or
  // copy Go is about to make inside its structure gate, held in the service's
  // own structure boundary until Go's `/v1/structure/settle` (or abort).
  async function runStructure(consultation, body) {
    const root = rootFor(body.root);
    const steps = body.steps ?? [];
    const kind = body.origin || steps[0]?.kind;
    const copy = kind === 'duplicate';
    const entries = steps.map((step) => ({
      from: step.from ? {
        location: step.from.location, canonical: step.from.canonicalPath,
        isDir: Boolean(step.from.isDir), isSymlink: Boolean(step.from.isSymlink),
      } : null,
      to: step.to ? { location: step.to.location, canonical: step.to.canonicalPath } : null,
    }));
    const localSteps = steps.map((step) => ({ from: step.from?.name ?? null, to: step.to?.name ?? null }));
    await withStructureChange(async (token) => {
      if (consultation.aborted) return;
      let plan = null;
      try {
        plan = root.lanes.prepareRestructure({ kind, rootId: root.id, steps: localSteps, entries, token });
      } catch (error) {
        const code = error && error.code ? error.code : 'failed';
        consultation.answer({ decision: 'refuse', status: code === 'checkpointing' ? 503 : 409, code });
        return;
      }
      try {
        if (consultation.aborted) return;
        const next = consultation.next();
        consultation.answer({ decision: 'proceed', holdId: consultation.id });
        const step = await next;
        if (!step.settle) return;
        try {
          const { completed = 0, copiedSteps = [] } = step.settle;
          if (copy) {
            for (const index of copiedSteps) {
              root.lanes.admitCopy({ rootId: root.id, step: localSteps[index], entry: entries[index], token });
            }
          } else {
            const held = plan;
            plan = null;
            root.lanes.settleRestructure({ plan: held, completed, copiedSteps, token });
          }
          consultation.answer({ ok: true });
        } catch (error) {
          logError('structure settle failed:', error);
          consultation.answer({ ok: false, error: error && error.message ? error.message : String(error) });
        }
      } finally {
        if (plan) authority.releaseStructure(plan, token);
      }
    });
  }

  // Opens one consultation Go starts, unless an abort for it already arrived.
  function begin(body, reply, runner, fallback) {
    prune(tombstones);
    if (tombstones.has(body.consultId)) {
      reply({ error: 'aborted' }, 409);
      return;
    }
    touch(body);
    const consultation = open(body.consultId, reply);
    run(consultation, () => runner(consultation, body), fallback);
  }

  // A handler that throws answers a kind Go does not accept, so Go treats the
  // call as failed: a checkpoint whose render threw is retried by Local's
  // scheduler as a transient failure, as it is in process.
  const failedWith = (error) => (error && error.message ? error.message : String(error));

  handlers.set('/v1/render-checkpoint', (body, reply) => begin(body, reply, runRenderCheckpoint,
    (error) => ({ kind: 'failed', error: failedWith(error), artifacts: [] })));

  handlers.set('/v1/observe', (body, reply) => begin(body, reply, runObserve,
    (error) => ({ action: 'failed', error: failedWith(error), artifacts: [] })));

  handlers.set('/v1/structure/prepare', (body, reply) => begin(body, reply, runStructure,
    (error) => ({ decision: 'refuse', status: 500, code: 'failed', error: failedWith(error) })));

  handlers.set('/v1/structure/settle', (body, reply) => {
    const consultation = consultations.get(body.consultId);
    if (!consultation) {
      reply({ ok: false, error: 'unknown consultation' });
      return;
    }
    consultation.continueWith({ settle: body }, reply);
  });

  handlers.set('/v1/admit', (body, reply) => {
    prune(tombstones);
    if (tombstones.has(body.consultId)) {
      reply({ error: 'aborted' }, 409);
      return;
    }
    touch(body);
    const consultation = open(body.consultId, reply);
    run(consultation, () => runWrite(consultation, body), (error) => ({
      decision: 'refuse', status: 500, code: 'failed', body: { error: error && error.message ? error.message : String(error) },
    }));
  });

  handlers.set('/v1/artifacts-written', (body, reply) => {
    const consultation = consultations.get(body.consultId);
    if (!consultation) {
      reply({ ok: false, error: 'unknown consultation' });
      return;
    }
    consultation.receive(body.receipts, reply);
  });

  handlers.set('/v1/settle', (body, reply) => {
    touch(body);
    const consultation = consultations.get(body.consultId);
    if (consultation) {
      consultation.continueWith({ settle: body }, reply);
      return;
    }
    if (body.lane !== 'exact' && body.lane !== 'create') {
      reply({ ok: false, error: 'unknown consultation' });
      return;
    }
    const fresh = open(body.consultId, reply);
    run(fresh, () => runSettleOnly(fresh, body), (error) => ({
      ok: false, error: error && error.message ? error.message : String(error),
    }));
  });

  // Releases what the consultation holds (its handler leaves its boundary and
  // releases its reservation), then, for `settle-failed`, reconciles the
  // document from a fresh read, in that order.
  handlers.set('/v1/abort', (body, reply) => {
    reply({ ok: true });
    const consultation = consultations.get(body.consultId);
    // A lost structure settle: Go moved `completed` steps on disk and could not
    // tell this process. While the plan is still held, settle it now with Go's
    // count, so the rooms follow the files instead of staying keyed to paths
    // that no longer exist (protocol v1, "Gaps recorded").
    const lateSettle = body.reason === 'settle-failed' && body.structure === true &&
      Number.isInteger(body.completed) && consultation && consultation.resume;
    if (lateSettle) {
      const copiedSteps = Array.isArray(body.copiedSteps) ? body.copiedSteps.filter(Number.isInteger) : [];
      consultation.continueWith({ settle: { completed: body.completed, copiedSteps } }, () => {});
    } else if (consultation) consultation.abort(body.reason);
    else {
      prune(tombstones);
      tombstones.set(body.consultId, { at: Date.now() });
    }
    const location = touched.get(body.consultId);
    if (body.reason === 'settle-failed' && location) {
      touched.delete(body.consultId);
      setImmediate(() => reconcile(location).catch((error) => logError(`reconcile of ${location.name} failed:`, error)));
    }
    // Go marked every page document the structure touched unconfirmed, at its
    // old and new path, before this abort; each is reconciled from disk and
    // released. A late settle above runs first: reconcile waits for the
    // structure boundary it holds.
    if (body.reason === 'settle-failed' && body.structure === true) {
      touched.delete(body.consultId);
      setImmediate(() => recoverUnconfirmed().catch((error) => logError('recovering unconfirmed documents failed:', error)));
    }
  });

  function authorized(req) {
    const given = Buffer.from(String(req.headers.authorization ?? ''));
    const expected = Buffer.from(`${BEARER}${secret}`);
    return given.length === expected.length && timingSafeEqual(given, expected);
  }

  // Go sends its own epoch and the one it knows for this process. The
  // supervisor is not the service's peer: its calls carry no epoch, as its
  // calls to Go carry none (control.go `controlEpochAuthorized`).
  function epochCurrent(req, pathname) {
    if (req.headers['x-trial-epoch'] === undefined && SUPERVISOR_PATHS.has(pathname)) return true;
    return Boolean(go) && req.headers['x-trial-epoch'] === go.epoch && req.headers['x-trial-peer-epoch'] === epoch;
  }

  // A room-owning public request Go forwarded (protocol v1, `/v1/route/*`):
  // Local's own app for that root answers it, seeing the request as the tab
  // sent it to Go's public listener. Go set the root and the public host from
  // its own resolution; the private headers go no further.
  function forwardToApp(req, res) {
    const record = roots.get(String(req.headers['x-trial-root'] ?? ''));
    const publicHost = req.headers['x-trial-public-host'];
    if (typeof publicHost !== 'string' || !publicHost) return send(res, 400, { error: 'route' });
    if (!record || !record.app) return send(res, 404, { error: 'unknown-root' });
    // Removing the root ends its open event streams (removeRoot); every other
    // response is left to finish.
    record.responses.add(res);
    res.on('close', () => record.responses.delete(res));
    req.url = req.url.slice(ROUTE_PREFIX.length) || '/';
    if (!req.url.startsWith('/')) req.url = `/${req.url}`;
    req.headers.host = publicHost;
    delete req.headers.authorization;
    for (const name of Object.keys(req.headers)) {
      if (name.startsWith('x-trial-')) delete req.headers[name];
    }
    record.app(req, res);
    return undefined;
  }

  // Go's notices for a path (the data-loss guard raised or resolved, data API
  // warnings) arrive on its event stream and go to the tabs through this
  // process's live hub, in stream order (protocol v1, "Go's notices reach the
  // tabs through the event stream"). The stream is followed from attach, and
  // again from where it stood after any disconnect.
  let events = null;
  function followEvents() {
    let since = '';
    let stopped = false;
    let request = null;
    const connect = () => {
      if (stopped) return;
      const connection = getConnection();
      request = http.request(`${connection.url}/v1/events?since=${encodeURIComponent(since)}`, {
        method: 'GET', headers: { authorization: `${BEARER}${connection.secret}`, accept: 'application/x-ndjson' },
      }, (response) => {
        let buffer = '';
        response.setEncoding('utf8');
        response.on('data', (chunk) => {
          buffer += chunk;
          for (let index = buffer.indexOf('\n'); index >= 0; index = buffer.indexOf('\n')) {
            const line = buffer.slice(0, index);
            buffer = buffer.slice(index + 1);
            let frame;
            try {
              frame = JSON.parse(line);
            } catch {
              continue;
            }
            if (frame.cursor) since = frame.cursor;
            if (frame.type === 'notice') roots.get(frame.root)?.live.notify(frame.rel, frame.payload);
          }
        });
        response.on('error', () => {});
        response.on('close', () => {
          if (!stopped) setTimeout(connect, 200).unref();
        });
      });
      request.on('error', () => {
        if (!stopped) setTimeout(connect, 500).unref();
      });
      request.end();
    };
    connect();
    return () => {
      stopped = true;
      request?.destroy();
    };
  }

  // The supervisor's calls (protocol v1, "The supervisor calls the service").
  function describeUnsaved(keys) {
    return keys.map((key) => {
      const target = authority.checkpointTargets(key)[0];
      return target ? { root: target.rootId, name: target.logicalPath } : { root: null, name: key };
    });
  }

  // Roots come and go while the service runs: Go adds one when a folder starts
  // being served and removes it when it stops (protocol v1, POST /v1/roots). A
  // removal flushes the root's dirty rooms first; if any stays unsaved, or the
  // deadline passes, the root stays and the answer lists it under `kept`, so Go
  // keeps the folder served and the person can retry.
  handlers.set('/v1/roots', (body, reply) => {
    const add = body.add ?? [];
    const remove = body.remove ?? [];
    const validEntry = (e) => e && typeof e.id === 'string' && e.id && typeof e.path === 'string' && e.path &&
      typeof e.url === 'string' && e.url;
    if (!Array.isArray(add) || !Array.isArray(remove) || !add.every(validEntry) ||
      !remove.every((id) => typeof id === 'string' && id)) {
      reply({ error: 'bad-request' }, 400);
      return;
    }
    if (!go) {
      reply({ error: 'not-attached' }, 409);
      return;
    }
    const deadlineMs = Number(body.deadlineMs) || 5000;
    (async () => {
      const added = [];
      const removed = [];
      const kept = [];
      for (const entry of add) {
        await addRoot(entry);
        added.push(entry.id);
      }
      for (const id of remove) {
        const outcome = await removeRoot(id, deadlineMs);
        if (outcome.removed) removed.push(id);
        else kept.push({ id, unsaved: outcome.unsaved, timedOut: outcome.timedOut });
      }
      if (added.length) recoverUnconfirmed().catch((error) => logError('recovering unconfirmed documents failed:', error));
      reply({ added, removed, kept });
    })().catch((error) => reply({ error: failedWith(error), code: error.code }, error.code === 'root-conflict' ? 409 : 500));
  });

  handlers.set('/v1/flush', (body, reply) => {
    checkpoint.flushDirty({ timeoutMs: Number(body.deadlineMs) || 5000 }).then((result) => {
      reply({ unsaved: describeUnsaved(result.unsaved), timedOut: Boolean(result.timedOut) });
    }, (error) => reply({ error: failedWith(error) }, 500));
  });

  // Local's HTTP suite's /checkpoint-wait, answered where the rooms live: until
  // the document's room is clean and no checkpoint is pending, or the budget
  // runs out (tests/helpers/host-http-fixture.cjs, the same loop).
  handlers.set('/v1/checkpoint-wait', (body, reply) => {
    const record = roots.get(body.root);
    if (!record || typeof body.name !== 'string' || !body.name) {
      reply({ error: 'bad-request' }, 400);
      return;
    }
    let key = path.join(record.path, body.name);
    try {
      key = realpathSync(key);
    } catch {
      // A document that is not there has no room to wait for.
    }
    const timeoutMs = Number.isFinite(body.timeoutMs) ? Math.max(0, body.timeoutMs) : 10000;
    const started = Date.now();
    const deadline = started + timeoutMs;
    let observed = false;
    (async () => {
      for (;;) {
        const pending = checkpoint.pending(key);
        if (pending) {
          observed = true;
          try {
            await pending;
          } catch {
            // A failed attempt leaves the room dirty, which the next pass sees.
          }
          continue;
        }
        const room = authority.rooms.get(key);
        if (room && authority.roomIsDirty(room)) {
          observed = true;
          if (Date.now() >= deadline) {
            reply({ error: 'checkpoint-timeout', detail: `room still dirty after ${Date.now() - started} ms`, observed }, 504);
            return;
          }
          await new Promise((resolve) => setTimeout(resolve, 25));
          continue;
        }
        reply({ ok: true, dirty: false, observed, waitedMs: Date.now() - started });
        return;
      }
    })().catch((error) => reply({ error: failedWith(error) }, 500));
  });

  // The snapshot a tab's relay left for the next upload, taken once. A repeat
  // with the same takeId answers the same, so a lost reply does not erase the
  // snapshot the upload needs.
  const takes = new Map();
  handlers.set('/v1/snapshot/take', (body, reply) => {
    prune(takes);
    if (takes.has(body.takeId)) {
      reply(takes.get(body.takeId).answer);
      return;
    }
    const answer = { snapshot: getAndClearSnapshot(body.name, body.root) };
    takes.set(body.takeId, { answer, at: Date.now() });
    reply(answer);
  });

  // A restarted Go: every consultation of the old one is aborted (its handlers
  // release their reservations and holds), and every open room is reconciled
  // from what the new Go reads before it admits edits (protocol v1, Failures,
  // "Go restart").
  handlers.set('/v1/peer', (body, reply) => {
    if (typeof body.url !== 'string' || typeof body.secret !== 'string' || typeof body.epoch !== 'string') {
      reply({ error: 'peer needs url, secret and epoch' }, 400);
      return;
    }
    for (const consultation of consultations.values()) consultation.abort('epoch');
    tombstones.clear();
    touched.clear();
    takes.clear();
    go = { url: body.url, secret: body.secret, epoch: body.epoch };
    const byPath = {};
    for (const record of roots.values()) byPath[record.path] = record.id;
    setConnection({ url: body.url, secret: body.secret, epoch, peerEpoch: body.epoch, roots: byPath });
    events?.();
    events = followEvents();
    reply({ ok: true, epoch });
    for (const key of authority.rooms.keys()) {
      for (const target of authority.checkpointTargets(key).slice(0, 1)) {
        reconcile({ root: target.rootId, name: target.logicalPath, canonicalPath: key })
          .catch((error) => logError(`reconcile of ${target.logicalPath} after a Go restart failed:`, error));
      }
    }
  });

  function send(res, status, body) {
    const payload = Buffer.from(JSON.stringify(body), 'utf8');
    res.writeHead(status, { 'content-type': 'application/json', 'content-length': String(payload.length) });
    res.end(payload);
  }

  function readJson(req) {
    return new Promise((resolve, reject) => {
      const chunks = [];
      let size = 0;
      req.on('data', (chunk) => {
        size += chunk.length;
        if (size > MAX_BODY) {
          reject(Object.assign(new Error('too-large'), { status: 413 }));
          req.destroy();
          return;
        }
        chunks.push(chunk);
      });
      req.on('end', () => {
        try {
          resolve(chunks.length ? JSON.parse(Buffer.concat(chunks).toString('utf8')) : {});
        } catch {
          reject(Object.assign(new Error('invalid JSON'), { status: 400 }));
        }
      });
      req.on('error', reject);
    });
  }

  async function handle(req, res) {
    if (req.headers.host !== `127.0.0.1:${server.address().port}`) return send(res, 403, { error: 'host' });
    if (!authorized(req)) return send(res, 401, { error: 'unauthorized' });
    const { pathname } = new URL(req.url, 'http://service');
    if (!epochCurrent(req, pathname)) return send(res, 409, { error: 'epoch', epoch });
    if (pathname === ROUTE_PREFIX || pathname.startsWith(`${ROUTE_PREFIX}/`)) return forwardToApp(req, res);
    const handler = req.method === 'POST' ? handlers.get(pathname) : null;
    if (!handler) return send(res, 404, { error: 'not found' });
    const body = await readJson(req);
    let answered = false;
    handler(body, (answer, status = 200) => {
      if (answered) return;
      answered = true;
      send(res, status, answer);
    });
  }

  // One root Go serves: its live hub, observer, Go adapter and Local's own app on
  // the forwarded routes. Adding a root that is already there from the same folder
  // changes nothing.
  async function addRoot(entry) {
    const canonical = realpathSync(entry.path);
    const existing = roots.get(entry.id);
    if (existing) {
      if (existing.path === canonical) return;
      throw Object.assign(new Error(`root ${entry.id} is already served from another folder`), { code: 'root-conflict' });
    }
    getConnection().roots[canonical] = entry.id;
    // `origin` is the root's whole public origin, the one Go serves it at:
    // Local's own-origin and Host checks compare a tab's with it (server.js
    // `isOwnOrigin`, `isOwnHostHeader`), because folders can share one port.
    const publicURL = new URL(entry.url);
    const root = { id: entry.id, path: canonical, port: Number(publicURL.port), origin: publicURL.origin };
    const live = createRootLive(root);
    const ctx = { root, live, localAuthority: authority, localCheckpoint: checkpoint };
    ctx.observer = new RootObserver(root, { live, localAuthority: authority });
    await withStructureChange(() => authority.registerRoot({ id: root.id, path: canonical, canonicalPath: canonical }));
    // Local's checkpoint scheduler writes a dirty room through this root's
    // host; here that is the Go adapter (POST /v1/checkpoint), as it is for
    // the app Local mounts on the forwarded routes (server.js registerHost).
    const goHost = createGoDocumentHost({ ctx });
    checkpoint.registerHost(root.id, goHost);
    const record = {
      id: root.id, path: canonical, origin: entry.url, live, ctx, observer: ctx.observer, goHost, lanes: inProcessLanes(ctx),
      responses: new Set(),
    };
    // Local's own app answers the routes Go forwards. Its document host is
    // the Go adapter (this process runs with the trial switch on), which it
    // registers with the checkpoint service in place of goHost.
    record.app = createApp({ ...ctx, devHooks: null, isKnownPath: null });
    roots.set(root.id, record);
    rootsByPath.set(canonical, record);
  }

  // Stops serving one root: flush its dirty rooms through Go, then end its event
  // streams and forget it. A room still unsaved after the flush keeps the root.
  async function removeRoot(id, deadlineMs) {
    const record = roots.get(id);
    if (!record) return { removed: true };
    const flushed = await checkpoint.flushDirty({ timeoutMs: deadlineMs, rootId: id });
    if (flushed.unsaved.length > 0 || flushed.timedOut) {
      return { removed: false, unsaved: describeUnsaved(flushed.unsaved), timedOut: Boolean(flushed.timedOut) };
    }
    roots.delete(id);
    rootsByPath.delete(record.path);
    const connection = getConnection();
    if (connection) delete connection.roots[record.path];
    for (const res of record.responses) {
      if (String(res.getHeader('content-type') ?? '').startsWith('text/event-stream')) res.end();
    }
    checkpoint.unregisterWriter(id);
    await withStructureChange(() => authority.closeRoot(id));
    await record.observer.stop();
    return { removed: true };
  }

  const server = http.createServer((req, res) => {
    handle(req, res).catch((error) => {
      if (!res.headersSent) send(res, error.status || 500, { error: error.message });
    });
  });

  return {
    secret,
    epoch,
    authority,
    checkpoint,
    roots,
    consultations,
    handlers,
    server,
    rootFor,
    goCall,
    // Listens on a free loopback port and answers the service URL.
    listen() {
      return new Promise((resolve, reject) => {
        server.once('error', reject);
        server.listen(0, '127.0.0.1', () => resolve(`http://127.0.0.1:${server.address().port}`));
      });
    },
    // Called once Go is READY: its control URL, secret and epoch, and the roots
    // it serves (`{ id, path, url }` from its READY line).
    async attachGo({ url, secret: goSecret, epoch: goEpoch, roots: served }) {
      go = { url, secret: goSecret, epoch: goEpoch };
      setConnection({ url, secret: goSecret, epoch, peerEpoch: goEpoch, roots: {} });
      for (const entry of served) await addRoot(entry);
      events?.();
      events = followEvents();
      recoverUnconfirmed().catch((error) => logError('recovering unconfirmed documents failed:', error));
    },
    close() {
      events?.();
      events = null;
      checkpoint.stopSweep();
      authority.close();
      server.closeAllConnections?.();
      return new Promise((resolve) => server.close(() => resolve()));
    },
  };
}

module.exports = { createTrialService, snapshotReader, refusalFor, Consultation };
