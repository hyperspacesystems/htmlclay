'use strict';

// Shared by the service's sync tests: a stub Go control listener that records each
// call and writes to disk as Go does, and where L1's fake sync server is.

const http = require('node:http');
const fs = require('node:fs');
const path = require('node:path');
const assert = require('node:assert/strict');
const { createHash } = require('node:crypto');
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
function recordingGo(t, folders) {
  const folderOf = typeof folders === 'string' ? () => folders : (root) => folders[root];
  const calls = [];
  const file = (name, root) => path.join(folderOf(root), ...name.split('/'));
  const answer = (method, url, body) => {
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
      if (body.kind === 'folder') for (const step of body.steps) fs.mkdirSync(file(step.to, body.root), { recursive: true });
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
    req.on('end', () => {
      const text = Buffer.concat(chunks).toString('utf8');
      const body = text ? JSON.parse(text) : null;
      calls.push({ method: req.method, path: req.url.split('?')[0], body });
      res.writeHead(200, { 'content-type': 'application/json' });
      res.end(JSON.stringify(answer(req.method, req.url, body)));
    });
  });
  t.after(() => server.closeAllConnections() || server.close());
  return new Promise((resolve) => server.listen(0, '127.0.0.1', () => resolve({ url: `http://127.0.0.1:${server.address().port}`, calls })));
}


const commitsOf = (go, root = null) => go.calls.filter((c) => c.path === '/v1/commit' && c.body.kind === 'sync-download' && (root === null || c.body.root === root));

module.exports = { call, SKIP, FAKE, KEY, sha, wait, until, decode, recordingGo, commitsOf };
