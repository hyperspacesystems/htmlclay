'use strict';

const http = require('http');
const path = require('path');
const { realpathSync } = require('fs');
const { randomBytes } = require('crypto');
const { getConnection } = require('./go-process.cjs');

// Local's KINDS: the write kinds its document host knows, and refuses the rest
// with the same message the in-process host uses.
const KINDS = {
  'browser-save': true,
  'data-api': true,
  restore: true,
  'guard-restore': true,
  'sync-download': true,
  'external-rebase': true,
  workspace: true,
  create: true,
};

// Local's STRUCTURE_KINDS: the restructure kinds its host knows.
const STRUCTURE_KINDS = {
  move: true,
  trash: true,
  duplicate: true,
  'sync-move': true,
  'sync-trash': true,
};

// Request fields the Go host resolves itself, so no request carries them.
const UNSENDABLE = new Set(['context', 'filePath', 'hooks']);

// A commit whose reply was lost (a dropped connection, say) may have landed. The
// adapter asks Go what became of the op before it reports a failure: for up to
// ten seconds it waits out a running op, then takes the answer /v1/op recorded
// (protocol v1, "GET /v1/op/<opId>").
const LOST_REPLY_ATTEMPTS = 50;
const LOST_REPLY_DELAY_MS = 100;

const delay = (ms) => new Promise((resolve) => setTimeout(resolve, ms));

const tally = { kinds: {} };
// Every subscription to a root sees every commit frame, and a root has more than
// one subscriber, so a commit is counted once per frame cursor, which is unique
// within one Go process.
const countedCommits = new Set();

function countKind(kind) {
  if (!tally.kinds[kind]) tally.kinds[kind] = { adapter: 0, committed: 0 };
  return tally.kinds[kind];
}

// P3's adapter-counts.json: what this process sent, and how many of the
// stream's commit frames for the kind it saw committed.
function adapterCounts() {
  const kinds = {};
  for (const [kind, counts] of Object.entries(tally.kinds)) {
    kinds[kind] = { adapter: counts.adapter, committed: counts.committed };
  }
  return { kinds };
}

// The checkpoints this process has asked the Go host for, by opId, until Go
// answers. Go's render-checkpoint call names the opId as opHandle; the service's
// handler reads what Local's checkpoint holds across its lock from here: the
// room, its target, and the admission claim captured before the lock.
const pendingCheckpoints = new Map();

// Rooms whose last checkpoint may have landed without its fsync (an
// indeterminate answer), by document path. Until POST /v1/durable proves the
// bytes on disk, the room's next checkpoint must not take them as saved
// (protocol v1, /v1/durable).
const unprovenCheckpoints = new Set();

function checkpointContext(opHandle) {
  return pendingCheckpoints.get(opHandle) ?? null;
}

// Set by the managed trial's service before it mounts Local's app on this
// adapter: the host consults the service back while it holds a document's lock,
// so a route that writes must release its own boundary first, and a checkpoint
// captures the authority's admission claim before the lock.
let serviceMode = null;

function configureService(mode) {
  serviceMode = mode ? { checkpointClaim: mode.checkpointClaim ?? null } : null;
}

function encodeBytes(value) {
  if (typeof value === 'string') return { encoding: 'utf8', data: value };
  if (Buffer.isBuffer(value)) return { encoding: 'base64', data: value.toString('base64') };
  return null;
}

// An answer's bytes: text comes back as a string, anything else as a Buffer,
// the way the caller handed them in.
function decodeBytes(value) {
  if (!value) return null;
  if (value.encoding === 'base64') return Buffer.from(value.data, 'base64');
  return value.data;
}

// A document's bytes are always a Buffer to a reader, whatever the answer said.
function decodeReadBytes(value) {
  if (!value) return null;
  if (value.encoding === 'base64') return Buffer.from(value.data, 'base64');
  return Buffer.from(value.data, 'utf8');
}

function sendable(value) {
  if (typeof value === 'function' || typeof value === 'symbol' || typeof value === 'bigint') return false;
  return value !== undefined;
}

// Local stamps a download with `new Date(modifiedAt)`, and the engine hands it a
// server ISO string or a stat's Date, while Go takes milliseconds. A value Local
// would skip (falsy) is not sent.
function stampOf(value) {
  if (!value) return undefined;
  const ms = new Date(value).getTime();
  return Number.isFinite(ms) ? ms : undefined;
}

function bodyFor(request) {
  const body = {};
  for (const [key, raw] of Object.entries(request)) {
    const value = key === 'modifiedAt' ? stampOf(raw) : raw;
    if (UNSENDABLE.has(key) || !sendable(value)) continue;
    body[key] = key === 'bytes' ? encodeBytes(value) : value;
  }
  return body;
}

function transportError(error, opId) {
  const failure = new Error(`the Go document host is unreachable: ${error && error.message ? error.message : error}`);
  failure.code = 'host-unreachable';
  if (opId) failure.opId = opId;
  return failure;
}

function answerError(answer, opId) {
  let message = `the Go document host answered ${answer.status}`;
  let parsed = null;
  try {
    parsed = JSON.parse(answer.body.toString('utf8'));
    if (parsed && typeof parsed.error === 'string') message = parsed.error;
  } catch {
    // A body that is not JSON leaves the status as the message.
  }
  const error = new Error(message);
  error.status = answer.status;
  if (parsed && typeof parsed === 'object') error.body = parsed;
  if (opId) error.opId = opId;
  return error;
}

function createGoDocumentHost({ ctx, checkpointClaim = serviceMode ? serviceMode.checkpointClaim : null }) {
  function rootId(root = ctx.root) {
    const connection = getConnection();
    if (typeof root === 'string') {
      if (!Object.values(connection.roots).includes(root)) {
        throw new Error(`the Go document host does not serve ${root}`);
      }
      return root;
    }
    const real = realpathSync(root.path);
    const id = connection.roots[real] ?? connection.roots[root.path];
    if (!id) throw new Error(`the Go document host does not serve ${root.path}`);
    return id;
  }

  async function request(method, pathname, { body = null, opId = null } = {}) {
    const connection = getConnection();
    const url = new URL(pathname, connection.url);
    const payload = body === null ? null : Buffer.from(JSON.stringify(body), 'utf8');
    let answer;
    try {
      answer = await new Promise((resolve, reject) => {
        const headers = { authorization: `Bearer ${connection.secret}` };
        if (connection.epoch && connection.peerEpoch) {
          headers['x-trial-epoch'] = connection.epoch;
          headers['x-trial-peer-epoch'] = connection.peerEpoch;
        }
        if (payload) {
          headers['content-type'] = 'application/json';
          headers['content-length'] = String(payload.length);
        }
        const req = http.request(url, { method, headers }, (response) => {
          const chunks = [];
          response.on('data', (chunk) => chunks.push(chunk));
          response.on('error', reject);
          response.on('end', () => resolve({ status: response.statusCode, body: Buffer.concat(chunks) }));
        });
        req.on('error', reject);
        if (payload) req.write(payload);
        req.end();
      });
    } catch (error) {
      throw transportError(error, opId);
    }
    if (answer.status !== 200) throw answerError(answer, opId);
    if (answer.body.length === 0) return null;
    return JSON.parse(answer.body.toString('utf8'));
  }

  async function commit(request_) {
    if (!request_ || !Object.hasOwn(KINDS, request_.kind)) {
      throw new TypeError(`Unknown document write kind: ${request_ && request_.kind}`);
    }
    if (request_.hooks) throw new TypeError('hooks ended in M16');
    const opId = randomBytes(16).toString('hex');
    countKind(request_.kind).adapter += 1;
    let answer;
    try {
      answer = await request('POST', '/v1/commit', {
        body: { opId, root: rootId(), ...bodyFor(request_) },
        opId,
      });
    } catch (error) {
      if (error.code !== 'host-unreachable') throw error;
      answer = await reconcileLostReply(opId, error);
    }
    const result = {
      outcome: answer.outcome,
      reason: answer.reason ?? null,
      etag: answer.etag ?? null,
      stored: decodeBytes(answer.stored),
      content: decodeBytes(answer.content),
    };
    if (answer.checksum !== null && answer.checksum !== undefined) result.checksum = answer.checksum;
    if (answer.refusal) result.refusal = answer.refusal;
    if (result.outcome === 'indeterminate') {
      const error = new Error(`${request_.kind} of ${request_.name} may not have landed`);
      error.code = result.reason ?? 'after-rename';
      error.opId = opId;
      Object.defineProperty(result, 'error', { value: error, enumerable: false });
    }
    return result;
  }

  // A commit whose reply was lost may have landed (P12). Before anyone retries,
  // ask Go what became of the op: a finished commit's record carries the exact
  // reply /v1/commit answered (CONTROL-PROTOCOL.md, /v1/op). An op Go does not
  // know (it never arrived, or Go restarted), or one that finished without a
  // reply, keeps the original failure, and nothing is retried here.
  async function reconcileLostReply(opId, failure) {
    for (let attempt = 0; attempt < LOST_REPLY_ATTEMPTS; attempt += 1) {
      let status;
      try {
        status = await request('GET', `/v1/op/${opId}`);
      } catch {
        await delay(LOST_REPLY_DELAY_MS);
        continue;
      }
      if (status && status.state === 'running') {
        await delay(LOST_REPLY_DELAY_MS);
        continue;
      }
      if (status && status.answer && typeof status.answer === 'object') return status.answer;
      throw failure;
    }
    throw failure;
  }

  // Local's read goes by the file path alone, and the sync engine passes no name
  // when it has only the path (reconcile/execute.js readLocalBytes). So a given
  // path decides the name: its place under the served root. A path whose folder
  // does not exist is a missing file, as Local's ENOENT is; one outside the root
  // is refused.
  function nameOf(name, filePath, root = ctx.root) {
    if (typeof filePath !== 'string' || !filePath) return name;
    const base = realpathSync(root.path);
    let folder;
    try {
      folder = realpathSync(path.dirname(filePath));
    } catch (error) {
      if (error.code === 'ENOENT') return null;
      throw error;
    }
    const rel = path.relative(base, path.join(folder, path.basename(filePath)));
    if (!rel || rel === '..' || rel.startsWith(`..${path.sep}`) || path.isAbsolute(rel)) {
      throw new Error(`the Go document host does not serve ${filePath}`);
    }
    return rel.split(path.sep).join('/');
  }

  async function read(name, filePath) {
    const rel = nameOf(name, filePath);
    if (rel === null) return null;
    const query = `?root=${encodeURIComponent(rootId())}&name=${encodeURIComponent(rel)}`;
    const answer = await request('GET', `/v1/read${query}`);
    if (!answer || answer.exists !== true) return null;
    return { bytes: decodeReadBytes(answer.bytes), checksum: answer.checksum ?? null };
  }

  async function restructure({ kind, steps = [] } = {}) {
    if (!Object.hasOwn(STRUCTURE_KINDS, kind)) throw new TypeError(`Unknown structure kind: ${kind}`);
    const opId = randomBytes(16).toString('hex');
    const answer = await request('POST', '/v1/restructure', {
      body: {
        opId,
        root: rootId(),
        kind,
        steps: steps.map((step) => ({ from: step.from, to: step.to })),
      },
      opId,
    });
    const result = { outcome: answer.outcome, completed: answer.completed, reason: answer.reason ?? null };
    if (result.outcome !== 'committed') {
      const error = new Error(`restructure ${kind} did not complete: ${result.reason}`);
      error.code = result.reason ?? 'failed';
      error.opId = opId;
      Object.defineProperty(result, 'error', { value: error, enumerable: false });
    }
    return result;
  }

  async function blockeditorState({ op, name, to = null, record = null } = {}) {
    const opId = randomBytes(16).toString('hex');
    await request('POST', '/v1/blockeditor-state', {
      body: { opId, root: rootId(), op, name, to, record },
      opId,
    });
  }

  // G9b: what this root's server owes platform sync for one document, cleared as
  // it is read, the same shape getAndClearSnapshot answers in-process.
  async function takeSnapshot(name, root = ctx.root) {
    const answer = await request('POST', '/v1/snapshot/take', {
      body: { root: rootId(root), name },
    });
    const snapshot = answer && answer.snapshot;
    if (!snapshot) return null;
    return { html: snapshot.html ?? null, userDriven: snapshot.userDriven };
  }

  // Local's checkpoint for a dirty room, run by the Go host: Go takes the
  // document's lock and asks the service for the room half by this write's
  // opId, so what the checkpoint holds waits in pendingCheckpoints until Go
  // answers. An indeterminate answer throws, and the scheduler retries it.
  async function checkpoint({ name, filePath }, { roomIdentity, generation }, { target, room }) {
    if (unprovenCheckpoints.has(filePath)) await proveDurable(name, filePath);
    const opId = randomBytes(16).toString('hex');
    const claim = checkpointClaim ? checkpointClaim(target) : null;
    pendingCheckpoints.set(opId, { target, room, claim, filePath });
    try {
      let answer;
      try {
        answer = await request('POST', '/v1/checkpoint', {
          body: { opId, root: rootId(), name, roomIdentity: String(roomIdentity), generation: generation ?? null },
          opId,
        });
      } catch (error) {
        if (error.body && typeof error.body.code === 'string') error.code = error.body.code;
        throw error;
      }
      if (answer.kind === 'indeterminate') {
        const error = new Error(`the checkpoint of ${name} may not have reached disk`);
        error.code = answer.reason ?? 'indeterminate';
        error.indeterminate = true;
        error.opId = opId;
        unprovenCheckpoints.add(filePath);
        throw error;
      }
      if (answer.kind === 'ok') unprovenCheckpoints.delete(filePath);
      if (answer.written) answer.written = decodeBytes(answer.written);
      return answer;
    } finally {
      pendingCheckpoints.delete(opId);
    }
  }

  // The bytes an indeterminate checkpoint left are taken as saved only once Go
  // has fsynced them. A failed or raced proof throws, so the scheduler retries
  // and the room stays dirty.
  async function proveDurable(name, filePath) {
    const current = await read(name, filePath);
    if (!current) {
      unprovenCheckpoints.delete(filePath);
      return;
    }
    const answer = await request('POST', '/v1/durable', {
      body: { root: rootId(), name: nameOf(name, filePath), checksum: current.checksum },
    });
    if (!answer || answer.durable !== true) {
      const error = new Error(`${name} changed before its last checkpoint was proven on disk`);
      error.code = 'durable-unproven';
      throw error;
    }
    unprovenCheckpoints.delete(filePath);
  }

  // One newline-delimited stream of the host's frames, resumed from the last
  // cursor it delivered after a drop, a restart or a failed connection. A feed
  // that reconnects before any frame gave it a cursor cannot tell what it
  // missed, so it reports a gap.
  function events(since, listener) {
    let cursor = since ?? null;
    let connected = false;
    let served = null;
    let closed = false;
    let timer = null;
    let open = null;

    function schedule() {
      if (closed || timer) return;
      timer = setTimeout(() => {
        timer = null;
        connect();
      }, 250);
    }

    function handle(line) {
      if (!line.trim()) return;
      let frame;
      try {
        frame = JSON.parse(line);
      } catch {
        return;
      }
      if (!frame || typeof frame.type !== 'string') return;
      if (typeof frame.cursor === 'string') cursor = frame.cursor;
      if (frame.type === 'gap') {
        listener({ type: 'gap', since: frame.cursor });
        return;
      }
      if (frame.root !== undefined && frame.root !== served) return;
      if (frame.type === 'change') {
        listener({ type: 'change', event: frame.event, rel: frame.rel });
        return;
      }
      if (frame.type === 'commit' && frame.outcome === 'committed' && !countedCommits.has(frame.cursor)) {
        countedCommits.add(frame.cursor);
        countKind(frame.kind).committed += 1;
      }
    }

    function connect() {
      if (closed) return;
      let connection;
      try {
        connection = getConnection();
        served = rootId();
      } catch {
        schedule();
        return;
      }
      const url = new URL('/v1/events', connection.url);
      if (cursor !== null) url.searchParams.set('since', cursor);
      const req = http.request(url, {
        method: 'GET',
        headers: { authorization: `Bearer ${connection.secret}`, accept: 'application/x-ndjson' },
      }, (response) => {
        if (response.statusCode !== 200) {
          response.resume();
          schedule();
          return;
        }
        if (connected && cursor === null) listener({ type: 'gap', since: null });
        connected = true;
        let buffer = '';
        response.setEncoding('utf8');
        response.on('data', (chunk) => {
          buffer += chunk;
          let index = buffer.indexOf('\n');
          while (index >= 0) {
            const line = buffer.slice(0, index);
            buffer = buffer.slice(index + 1);
            handle(line);
            index = buffer.indexOf('\n');
          }
        });
        response.on('error', schedule);
        response.on('close', schedule);
        response.on('end', schedule);
      });
      open = req;
      req.on('error', schedule);
      req.end();
    }

    connect();

    return function unsubscribe() {
      closed = true;
      if (timer) {
        clearTimeout(timer);
        timer = null;
      }
      if (open) {
        open.destroy();
        open = null;
      }
    };
  }

  return {
    commit, read, restructure, checkpoint, events, blockeditorState, takeSnapshot, isGoDocumentHost: true,
    releasesBoundary: Boolean(serviceMode),
  };
}

// The service marks a checkpoint whose settle came back indeterminate before
// Go answers the adapter, and its observer asks while the mark stands.
function markCheckpointUnproven(filePath) {
  unprovenCheckpoints.add(filePath);
}

function checkpointUnproven(filePath) {
  return unprovenCheckpoints.has(filePath);
}

module.exports = {
  createGoDocumentHost, adapterCounts, checkpointContext, configureService, markCheckpointUnproven, checkpointUnproven,
};
