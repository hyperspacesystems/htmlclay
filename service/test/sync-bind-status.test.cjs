'use strict';

// While /sync/v1/bind downloads a folder's first files, /sync/v1/status must say
// that session is syncing. The engine's runner is stopped during a first download,
// so without the check on entry.bindAbort in _statusFor it reads idle: after a
// restart in the middle of a first download the folder would read up to date.

const test = require('node:test');
const assert = require('node:assert/strict');
const http = require('node:http');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const { startService } = require('./harness.cjs');
const { call, FAKE, KEY, sha, wait, decode } = require('./sync-fixture.cjs');

const ROOT_ID = 'e1e2e3e4e5e6e7e8';

function fakeGo(t, folder) {
  const reports = [];
  const eventStreams = new Set();
  const file = (name) => path.join(folder, ...name.split('/'));

  const server = http.createServer((req, res) => {
    const { pathname, searchParams } = new URL(req.url, 'http://go');
    if (pathname === '/v1/events') {
      res.writeHead(200, { 'content-type': 'application/x-ndjson' });
      res.flushHeaders();
      eventStreams.add(res);
      req.on('close', () => eventStreams.delete(res));
      return;
    }
    const chunks = [];
    req.on('data', (chunk) => chunks.push(chunk));
    req.on('end', () => {
      const text = Buffer.concat(chunks).toString('utf8');
      const body = text ? JSON.parse(text) : null;
      let reply = {};
      if (pathname === '/v1/unconfirmed') reply = { documents: [] };
      else if (pathname === '/v1/snapshot/take') reply = { snapshot: null };
      else if (pathname === '/v1/sync/report') {
        reports.push(body);
        reply = { ok: true, seq: body.seq };
      } else if (pathname === '/v1/sync/session') {
        reply = { ok: true };
      } else if (pathname === '/v1/read') {
        const target = file(searchParams.get('name'));
        if (!fs.existsSync(target) || !fs.statSync(target).isFile()) reply = { exists: false };
        else {
          const bytes = fs.readFileSync(target);
          reply = { exists: true, bytes: { encoding: 'base64', data: bytes.toString('base64') }, checksum: sha(bytes) };
        }
      } else if (pathname === '/v1/commit') {
        const target = file(body.name);
        const bytes = decode(body.bytes);
        fs.mkdirSync(path.dirname(target), { recursive: true });
        fs.writeFileSync(target, bytes);
        reply = { outcome: 'committed', reason: null, checksum: sha(bytes), etag: sha(bytes), stored: null, content: null, opId: body.opId };
      } else if (pathname === '/v1/sync/file') {
        const target = file(body.rel);
        if (body.op === 'remove') fs.rmSync(target, { force: true });
        else {
          fs.mkdirSync(path.dirname(target), { recursive: true });
          fs.writeFileSync(target, decode(body.bytes));
        }
        reply = { outcome: 'committed', opId: body.opId };
      }
      res.writeHead(200, { 'content-type': 'application/json' });
      res.end(JSON.stringify(reply));
    });
  });

  t.after(() => {
    for (const res of eventStreams) res.destroy();
    server.closeAllConnections();
    server.close();
  });

  return new Promise((resolve) => {
    server.listen(0, '127.0.0.1', () => resolve({
      url: `http://127.0.0.1:${server.address().port}`,
      reports,
    }));
  });
}

test('a first download reads syncing, never idle', async (t) => {
  const { startFakeSyncHttp } = require(FAKE);
  const fake = await startFakeSyncHttp({
    nodes: [
      { id: 1, type: 'site', name: 'one.html', parentId: 0, content: '<p>one</p>' },
      { id: 2, type: 'site', name: 'two.html', parentId: 0, content: '<p>two</p>' },
    ],
    key: KEY,
    actor: { id: 5, username: 'sam', person: { id: 5, name: 'Sam' } },
    accounts: [{ id: 7, kind: 'team', username: 'studio', displayName: 'Studio', role: 'editor', syncBase: '/_/sync', lifecycle: 'active', sync: { enabled: true, reason: null } }],
  });
  t.after(() => fake.close());

  let releaseSecondDownload;
  const secondDownloadStarted = new Promise((resolve) => {
    const origGet = fake.server.getNodeContent.bind(fake.server);
    fake.server.getNodeContent = async (account, nodeId, ...rest) => {
      if (Number(nodeId) === 2) {
        resolve();
        await new Promise((r) => { releaseSecondDownload = r; });
      }
      return origGet(account, nodeId, ...rest);
    };
  });

  const base = fs.realpathSync(fs.mkdtempSync(path.join(os.tmpdir(), 'bind-status-')));
  t.after(() => fs.rmSync(base, { recursive: true, force: true }));
  const folder = path.join(base, 'Studio');
  fs.mkdirSync(folder);

  const go = await fakeGo(t, folder);
  const s = await startService(t, {
    go: go.url,
    roots: [{ id: ROOT_ID, path: folder, url: 'http://studio.htmlclay.localhost:1/' }],
  });
  s.send({ sync: { v: 1, key: KEY, serverUrl: fake.url } });
  await s.line('SYNC KEY HELD');

  const session = { id: 's-bind', rootId: ROOT_ID, accountId: 7, kind: 'team', cached: { username: 'studio', displayName: 'Studio', role: 'editor' }, paused: null, legacyMetaDir: null, enabled: true };
  const bindPromise = call(s, 'POST', '/sync/v1/bind', { body: { deviceId: 'dev-1', session, root: { id: ROOT_ID, path: folder }, mode: 'empty' } });

  await secondDownloadStarted;
  await wait(150);

  const midStatus = await call(s, 'POST', '/sync/v1/status', { body: {} });
  assert.equal(midStatus.body.sessions.length, 1);
  assert.equal(midStatus.body.sessions[0].status, 'syncing', `mid-bind status was ${JSON.stringify(midStatus.body.sessions[0])}`);

  releaseSecondDownload();
  const bindRes = await bindPromise;
  assert.equal(bindRes.body.ok, true);

  s.send({ sync: { v: 1, key: null } });
  await s.line('SYNC KEY CLEARED');
});

