'use strict';

// Shared by the service's sync tests: a stub Go control listener that records each
// call and writes to disk as Go does, and where L1's fake sync server is.

const http = require('node:http');
const fs = require('node:fs');
const path = require('node:path');
const assert = require('node:assert/strict');
const { createHash, randomBytes } = require('node:crypto');
const { GO_EPOCH, call: rawCall } = require('./harness.cjs');

// Go's calls carry both epochs (C3).
const call = (s, method, pathname, options = {}) => rawCall(s, method, pathname, { ...options, headers: { 'x-trial-epoch': GO_EPOCH, 'x-trial-peer-epoch': s.ready.epoch } });

const LOCAL = process.env.HTMLCLAY_SYNC_LOCAL || path.join(__dirname, '..', '..', '..', 'hyperclay-local');
const FAKE = path.join(LOCAL, 'trial', 'fake-sync-http.cjs');
// The box names its checkout, so there a missing fake fails instead of skipping.
const SKIP = fs.existsSync(FAKE) || process.env.HTMLCLAY_SYNC_LOCAL ? false : `no Hyperclay Local checkout with trial/fake-sync-http.cjs at ${LOCAL} (set HTMLCLAY_SYNC_LOCAL)`;

const KEY = `hcsk_${'5e'.repeat(32)}`;
// Local's checksum, which Go answers too: the first 16 hex of SHA-256.
const sha = (bytes) => createHash('sha256').update(bytes).digest('hex').slice(0, 16);
const wait = (ms) => new Promise((resolve) => setTimeout(resolve, ms));

async function until(what, predicate, timeoutMs = 30000) {
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    if (predicate()) return;
    await wait(50);
  }
  assert.fail(`timed out waiting for ${what}`);
}

function decode(bytes) {
  if (!bytes) return Buffer.alloc(0);
  return bytes.encoding === 'base64' ? Buffer.from(bytes.data, 'base64') : Buffer.from(bytes.data, 'utf8');
}

// A Go control listener that records every call and keeps the folder on disk.
// With { consult: true } it also calls the service back as real Go does once
// attach(s) names the service: a document commit settles (/v1/settle) after its
// write, and a sync move or trash is prepared and settled
// (/v1/structure/prepare, /v1/structure/settle), each with Go's 10 s deadline
// shortened to deadlineMs. A callback with no good answer makes the commit
// indeterminate or the move refused, as Go answers then.
function recordingGo(t, folders, { consult = false, deadlineMs = 2000 } = {}) {
  const folderOf = typeof folders === 'string' ? () => folders : (root) => folders[root];
  const calls = [];
  const consults = [];
  let service = null;
  const file = (name, root) => path.join(folderOf(root), ...name.split('/'));
  const slash = (p) => p.split(path.sep).join('/');
  const ask = async (pathname, body) => {
    const reply = await Promise.race([
      call(service, 'POST', pathname, { body }).then((res) => res.body, () => null),
      wait(deadlineMs).then(() => null),
    ]);
    consults.push({ path: pathname, name: body.name ?? null, reply });
    return reply;
  };
  const head = (body, name, canonicalPath, lane) => ({
    consultId: randomBytes(16).toString('hex'), opId: body.opId, opHandle: body.opHandle ?? null,
    root: body.root, name, canonicalPath, lane, origin: body.kind,
  });
  const isDocument = (name) => /\.(html|htmlclay)$/i.test(name);
  const answer = async (method, url, body) => {
    const { pathname, searchParams } = new URL(url, 'http://go');
    if (pathname === '/v1/unconfirmed') return { documents: [] };
    if (pathname === '/v1/snapshot/take') return { snapshot: null };
    if (pathname === '/v1/read') {
      const target = file(searchParams.get('name'), searchParams.get('root'));
      if (!fs.existsSync(target) || !fs.statSync(target).isFile()) return { exists: false };
      const bytes = fs.readFileSync(target);
      return { exists: true, bytes: { encoding: 'base64', data: bytes.toString('base64') }, checksum: sha(bytes) };
    }
    if (pathname === '/v1/commit') {
      const target = file(body.name, body.root);
      const current = fs.existsSync(target) ? fs.readFileSync(target) : null;
      const expected = body.expected;
      if (expected && ((expected.type === 'absent' && current) || (expected.type === 'checksum' && (!current || sha(current) !== expected.value)))) {
        return { outcome: 'refused', reason: 'changed', opId: body.opId };
      }
      const bytes = decode(body.bytes);
      fs.mkdirSync(path.dirname(target), { recursive: true });
      fs.writeFileSync(target, bytes);
      if (consult && service && body.bytes?.encoding !== 'base64' && isDocument(body.name)) {
        const stat = fs.statSync(target, { bigint: true });
        const settled = await ask('/v1/settle', {
          ...head(body, body.name, slash(fs.realpathSync(target)), 'exact'),
          reservation: null, outcome: 'committed', logical: body.bytes, file: body.bytes, etag: null,
          sender: 'sync-engine', userDriven: null, explicitCreate: current === null, captureId: null, saveId: null,
          identity: { dev: String(stat.dev), ino: String(stat.ino), size: Number(stat.size), mtimeNs: String(stat.mtimeNs) },
        });
        if (!settled || settled.ok !== true) return { outcome: 'indeterminate', reason: 'after-rename', opId: body.opId };
      }
      return { outcome: 'committed', reason: null, checksum: sha(bytes), etag: sha(bytes), stored: null, content: null, opId: body.opId };
    }
    if (pathname === '/v1/sync/file') {
      const target = file(body.rel, body.root);
      if (body.op === 'remove') fs.rmSync(target, { force: true });
      else {
        fs.mkdirSync(path.dirname(target), { recursive: true });
        fs.writeFileSync(target, decode(body.bytes));
      }
      return { outcome: 'committed', opId: body.opId };
    }
    if (pathname === '/v1/restructure') {
      if (body.kind === 'folder') {
        for (const step of body.steps) fs.mkdirSync(file(step.to, body.root), { recursive: true });
        return { outcome: 'committed', completed: body.steps.length, reason: null, opId: body.opId };
      }
      const steps = body.steps.map((step) => {
        const from = file(step.from, body.root);
        const isDir = fs.existsSync(from) && fs.statSync(from).isDirectory();
        return {
          kind: body.kind,
          from: { name: step.from, location: slash(from), canonicalPath: slash(from), isDir, isSymlink: false },
          to: { name: step.to, location: slash(file(step.to, body.root)), canonicalPath: slash(file(step.to, body.root)) },
        };
      });
      let prepared = null;
      const consultHead = head(body, body.steps[0]?.from ?? '', steps[0]?.from.canonicalPath ?? '', 'structure');
      if (consult && service) {
        prepared = await ask('/v1/structure/prepare', { ...consultHead, steps });
        if (!prepared) return { outcome: 'refused', reason: 'service-unavailable', completed: 0, opId: body.opId };
        if (prepared.decision === 'refuse') return { outcome: 'refused', reason: prepared.code, completed: 0, opId: body.opId };
      }
      for (const step of body.steps) {
        const to = file(step.to, body.root);
        fs.mkdirSync(path.dirname(to), { recursive: true });
        fs.renameSync(file(step.from, body.root), to);
      }
      if (prepared) {
        await ask('/v1/structure/settle', { ...consultHead, holdId: prepared.holdId, completed: body.steps.length, copiedSteps: [], outcome: 'committed' });
      }
      return { outcome: 'committed', completed: body.steps.length, reason: null, opId: body.opId };
    }
    return {};
  };
  const server = http.createServer((req, res) => {
    if (req.url.startsWith('/v1/events')) {
      res.writeHead(200, { 'content-type': 'application/x-ndjson' });
      res.flushHeaders();
      return;
    }
    const chunks = [];
    req.on('data', (chunk) => chunks.push(chunk));
    req.on('end', async () => {
      const text = Buffer.concat(chunks).toString('utf8');
      const body = text ? JSON.parse(text) : null;
      calls.push({ method: req.method, path: req.url.split('?')[0], body });
      const reply = await answer(req.method, req.url, body);
      res.writeHead(200, { 'content-type': 'application/json' });
      res.end(JSON.stringify(reply));
    });
  });
  t.after(() => server.closeAllConnections() || server.close());
  const attach = (s) => { service = s; };
  return new Promise((resolve) => server.listen(0, '127.0.0.1', () => resolve({ url: `http://127.0.0.1:${server.address().port}`, calls, consults, attach })));
}


const commitsOf = (go, root = null) => go.calls.filter((c) => c.path === '/v1/commit' && c.body.kind === 'sync-download' && (root === null || c.body.root === root));

module.exports = { call, SKIP, FAKE, KEY, sha, wait, until, decode, recordingGo, commitsOf };
