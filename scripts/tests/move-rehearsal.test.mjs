// Group 1 step 5.13: the move from Hyperclay Local with the real host, the real
// live-room service and the fake sync server. A service acting as "Local" binds a
// team folder and stops; its data is laid out as Local's data folder; a synctest
// host connects and sends move.start. Then: both folders are served, the move
// caused no upload, no download and no conflict, an edit goes each way, Local's
// old port redirects a page navigation, a second move finds nothing to move, and
// "Local" started again on its old data is refused and moves nothing.
// Never touches a real Local data folder: HTMLCLAY_LOCAL_USERDATA names a
// disposable one. Run three times in a row for the gate.
import test from 'node:test';
import assert from 'node:assert/strict';
import { cpSync, existsSync, mkdirSync, mkdtempSync, readFileSync, realpathSync, rmSync, writeFileSync } from 'node:fs';
import http from 'node:http';
import net from 'node:net';
import { createRequire } from 'node:module';
import os from 'node:os';
import path from 'node:path';
import { build, driver, sha, wait } from '../sync-harness.mjs';

const require = createRequire(import.meta.url);
const REPO = path.resolve(path.dirname(new URL(import.meta.url).pathname), '..', '..');
const LOCAL = process.env.HTMLCLAY_SYNC_LOCAL || path.join(REPO, 'service', 'test', 'fake');
const FAKE = path.join(LOCAL, 'trial', 'fake-sync-http.cjs');
const SKIP = existsSync(FAKE) ? false : `no fake sync server at ${LOCAL}`;

const OLD_TEAM = '0d9c2f0e-6a51-4d6c-9a39-2f5a8f6f1c11';
const OLD_PLAIN = '7a0c6e52-1d3b-4c4f-8e6a-5b2d9f1e3c77';
const SESSION = 'f3a1c7d2-58a4-4c0b-8a57-6d3e2b1a9c44';
const TODO = 3;

const mutating = (fake, from) => fake.requests.slice(from).filter((r) => !['GET', 'HEAD'].includes(r.method)).map((r) => `${r.method} ${r.url || r.path}`);

function freePort() {
  return new Promise((resolve) => {
    const server = net.createServer();
    server.listen(0, '127.0.0.1', () => {
      const { port } = server.address();
      server.close(() => resolve(port));
    });
  });
}

// One page navigation to Local's old address, as a bookmark opens it.
function navigate(port, rel) {
  return new Promise((resolve) => {
    const request = http.request({
      host: '127.0.0.1', port, path: `/${rel}`, method: 'GET', timeout: 5000,
      headers: { Host: `localhost:${port}`, 'Sec-Fetch-Mode': 'navigate', 'Sec-Fetch-Dest': 'document', 'Sec-Fetch-Site': 'none', 'Sec-Fetch-User': '?1' },
    }, (response) => {
      response.resume();
      resolve({ status: response.statusCode, location: response.headers.location || '' });
    });
    request.on('timeout', () => request.destroy(new Error('timeout')));
    request.on('error', (error) => resolve({ status: 0, location: error.code || error.message }));
    request.end();
  });
}

async function until(what, check, timeoutMs = 30000) {
  const deadline = Date.now() + timeoutMs;
  for (;;) {
    const value = await check();
    if (value) return value;
    if (Date.now() > deadline) throw new Error(`timed out waiting for ${what}`);
    await wait(250);
  }
}

test('the move from Hyperclay Local, end to end', { skip: SKIP, timeout: 600000 }, async (t) => {
  const { startService } = require(path.join(REPO, 'service', 'test', 'harness.cjs'));
  const { call, KEY, recordingGo } = require(path.join(REPO, 'service', 'test', 'sync-fixture.cjs'));
  const { startFakeSyncHttp } = require(FAKE);

  const fake = await startFakeSyncHttp({
    nodes: [
      { id: 1, type: 'site', name: 'index.html', parentId: 0, content: '<p>home</p>' },
      { id: 2, type: 'folder', name: 'notes', parentId: 0 },
      { id: TODO, type: 'site', name: 'todo.html', parentId: 2, content: '<p>todo</p>' },
      { id: 4, type: 'site', name: 'board.html', parentId: 0, content: '<p>board</p>' },
    ],
    key: KEY,
    actor: { id: 12, username: 'maya', person: { id: 12, name: 'Maya' } },
    accounts: [
      { id: 40, kind: 'team', username: 'studio', displayName: 'Studio', role: 'editor', syncBase: '/_/sync', lifecycle: 'active', sync: { enabled: true, reason: null } },
    ],
  });
  t.after(() => fake.close());

  // Served folders sit inside home, so the disposable world does too.
  const base = realpathSync(mkdtempSync(path.join(realpathSync(os.homedir()), 'htmlclay-move-rehearsal-')));
  // One cleanup, in order: stop the host, keep its logs for the box output, then delete.
  const cleanup = [];
  t.after(async () => {
    for (const step of cleanup.reverse()) await step().catch(() => {});
    const out = process.env.TESTBOX_OUT;
    if (out) {
      for (const name of ['host.log', 'driver.log']) {
        if (existsSync(path.join(base, name))) cpSync(path.join(base, name), path.join(out, `move-rehearsal-${name}`));
      }
    }
    rmSync(base, { recursive: true, force: true });
  });
  const studio = path.join(base, 'Studio');
  const recipes = path.join(base, 'Recipes');
  mkdirSync(studio);
  mkdirSync(recipes);
  writeFileSync(path.join(recipes, 'index.html'), '<p>recipes</p>');
  const marker = path.join(studio, '.hyperclay', 'sync-root.json');

  // 1. "Local": a service binds the team folder with Local's own root id, then quits.
  const row = { id: SESSION, rootId: OLD_TEAM, accountId: 40, kind: 'team', cached: { username: 'studio', displayName: 'Studio', role: 'editor' }, paused: null, legacyMetaDir: null, enabled: true };
  const key = async (s) => { s.send({ sync: { v: 1, key: KEY, serverUrl: fake.url } }); await s.line('SYNC KEY HELD'); };
  const configureLocal = (s) => call(s, 'POST', '/sync/v1/configure', { body: { deviceId: 'local-device', paused: false, sessions: [row], roots: [{ id: OLD_TEAM, path: studio }] } });
  const goLocal = await recordingGo(t, studio);
  const local = await startService(t, { go: goLocal.url, roots: [{ id: OLD_TEAM, path: studio, url: 'http://studio.htmlclay.localhost:1/' }] });
  await key(local);
  const bound = await call(local, 'POST', '/sync/v1/bind', { body: { deviceId: 'local-device', session: row, root: { id: OLD_TEAM, path: studio }, mode: 'empty' } });
  assert.equal(bound.body.ok, true, JSON.stringify(bound.body));
  assert.deepEqual((await configureLocal(local)).body, { ok: true, running: [SESSION] });
  await call(local, 'POST', '/sync/v1/idle', { body: { timeoutMs: 20000 } });
  assert.equal(readFileSync(path.join(studio, 'notes', 'todo.html'), 'utf8'), '<p>todo</p>');
  local.send({ sync: { v: 1, key: null } });
  await local.line('SYNC KEY CLEARED');

  // Local's data folder, as Local lays it out.
  const userData = path.join(base, 'Hyperclay Local');
  const studioPort = await freePort();
  const recipesPort = await freePort();
  mkdirSync(path.join(userData, 'sync-meta', 'v2'), { recursive: true });
  cpSync(path.join(local.tmp, 'data', 'sync-meta', 'v2', SESSION), path.join(userData, 'sync-meta', 'v2', SESSION), { recursive: true });
  writeFileSync(path.join(userData, 'settings.json'), JSON.stringify({
    settingsVersion: 2,
    deviceId: 'local-device',
    actor: { id: 12, username: 'maya' },
    roots: [
      { id: OLD_TEAM, kind: 'team', path: studio, port: studioPort },
      { id: OLD_PLAIN, kind: 'plain', path: recipes, port: recipesPort },
    ],
    syncSessions: [{ id: SESSION, rootId: OLD_TEAM, accountId: 40, kind: 'team', legacyMetaDir: null }],
  }, null, 2));
  const localIdentity = readFileSync(path.join(userData, 'sync-meta', 'v2', SESSION, 'identity.json'), 'utf8');
  const before = { index: sha(readFileSync(path.join(studio, 'index.html'))), todo: sha(readFileSync(path.join(studio, 'notes', 'todo.html'))) };

  // 2. HTML Clay: a synctest host on its own config, connected with the same key.
  process.env.HTMLCLAY_LOCAL_USERDATA = userData;
  const built = build(path.join(base, 'bin'));
  const d = driver();
  cleanup.push(async () => { await d.send('stop').catch(() => {}); d.child.disconnect(); });
  await d.ready;
  await d.send('start', { bin: built.bin, configBase: path.join(base, 'config'), log: path.join(base, 'host.log'), syncServer: fake.url, serviceCmd: built.serviceCmd });
  await d.send('connect', { key: KEY });
  const offer = await until('the move offer', async () => {
    const move = (await d.send('snapshot')).move;
    return move && move.folders && move.folders.length === 2 && !move.needsKey && !move.localRunning ? move : null;
  });
  const requestsBefore = fake.requests.length;
  const moved = await d.send('act', { type: 'move.start', args: { folderIDs: offer.folders.map((f) => f.id) } });
  assert.equal(moved.refused, undefined, `move.start was refused: ${moved.refused}`);

  // 3a. Both folders are served.
  const served = await until('both folders served', async () => {
    const folders = (await d.send('snapshot')).folders;
    const of = (dir) => folders.find((f) => (f.fullPath === dir || f.path === dir) && f.url);
    return of(studio) && of(recipes) ? { studio: of(studio), recipes: of(recipes) } : null;
  }, 60000);
  await d.send('idle', { timeoutMs: 60000 });
  await wait(1500);

  // 3b. The move moved no file and raised no conflict.
  assert.deepEqual(mutating(fake, requestsBefore), [], 'the move sent no change to the server');
  assert.deepEqual({ index: sha(readFileSync(path.join(studio, 'index.html'))), todo: sha(readFileSync(path.join(studio, 'notes', 'todo.html'))) }, before, 'the move changed no file');
  assert.deepEqual(await d.send('conflicts'), [], 'the move held no conflict');
  assert.equal(JSON.parse(readFileSync(marker, 'utf8')).rootId, served.studio.id, 'the marker names the moved folder');
  assert.equal(readFileSync(path.join(userData, 'sync-meta', 'v2', SESSION, 'identity.json'), 'utf8'), localIdentity, "Local's data folder is unchanged");

  // 3c. An edit on disk goes up, and an edit on the server comes down.
  writeFileSync(path.join(studio, 'board.html'), '<p>board, edited here</p>');
  await until('the upload', () => fake.server.get(4).content.toString('utf8') === '<p>board, edited here</p>', 60000);
  await fake.server.putNodeContent(null, TODO, '<p>todo, edited online</p>');
  fake.push({ type: 'node-saved', nodeId: TODO, nodeType: 'site' });
  await until('the download', () => readFileSync(path.join(studio, 'notes', 'todo.html'), 'utf8') === '<p>todo, edited online</p>', 60000);

  // 3d. Local's old port sends a bookmark on to the folder's address now.
  const hop = await navigate(studioPort, 'notes/todo.html');
  assert.equal(hop.status, 302, `the old port answered ${hop.status} ${hop.location}`);
  assert.equal(hop.location, `${served.studio.url.replace(/\/$/, '')}/notes/todo.html`);

  // 3e. A second move finds nothing to move.
  assert.deepEqual(await d.send('act', { type: 'move.start', args: { folderIDs: offer.folders.map((f) => f.id) } }), { refused: 'nothing-to-move' });

  // 4. "Local" started again on its old data is refused and moves nothing.
  const beforeOld = fake.requests.length;
  const oldCalls = goLocal.calls.length;
  await key(local);
  await configureLocal(local);
  await wait(2500);
  const [theirs] = (await call(local, 'POST', '/sync/v1/status', { body: {} })).body.sessions;
  assert.ok(theirs && theirs.paused && theirs.paused.reason === 'folder-replaced', `the old owner: ${JSON.stringify(theirs)}`);
  assert.deepEqual(mutating(fake, beforeOld), [], 'the old owner sent no change to the server');
  assert.deepEqual(goLocal.calls.slice(oldCalls).filter((c) => !['/v1/unconfirmed', '/v1/read', '/v1/snapshot/take', '/v1/sync/report'].includes(c.path)).map((c) => c.path), [], 'the old owner wrote nothing in the folder');
});
