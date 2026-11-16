'use strict';

// The move from Hyperclay Local, as the sync engine sees it. A folder synced by
// one engine ("Local") is taken over by a second one ("HTML Clay") the way
// internal/localimport does it: the session's bookkeeping folder is copied, and
// the folder id is rewritten in identity.json and in the folder's marker.
// Proves: (1) the new owner moves no file and holds no conflict, (2) it syncs
// on with the same nodes, (3) the old owner, started again, is refused and
// moves nothing. MOVE_TEST_LEAVE_OUT=no-copy|no-identity|no-marker leaves one
// step out, and the test must then fail.

const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const { startService } = require('./harness.cjs');
const { call, SKIP, FAKE, KEY, wait, until, decode, recordingGo, commitsOf } = require('./sync-fixture.cjs');

const OLD = '0d9c2f0e-6a51-4d6c-9a39-2f5a8f6f1c11'; // Local's root ids are UUIDs
const NEW = 'b1b2b3b4b5b6b7b8'; // HTML Clay's folder ids are 16 hex
const SESSION = 'f3a1c7d2-58a4-4c0b-8a57-6d3e2b1a9c44';

const mutating = (fake, from) => fake.requests.slice(from).filter((r) => !['GET', 'HEAD'].includes(r.method));
const writesOf = (go, from) => go.calls.slice(from).filter((c) => !['/v1/unconfirmed', '/v1/read', '/v1/snapshot/take', '/v1/sync/report'].includes(c.path));

test('import from Local: copy the bookkeeping, rewrite the folder id', { skip: SKIP }, async (t) => {
  const { startFakeSyncHttp } = require(FAKE);
  const fake = await startFakeSyncHttp({
    nodes: [
      { id: 1, type: 'site', name: 'index.html', parentId: 0, content: '<p>home</p>' },
      { id: 2, type: 'folder', name: 'notes', parentId: 0 },
      { id: 3, type: 'site', name: 'todo.html', parentId: 2, content: '<p>todo</p>' },
      { id: 4, type: 'site', name: 'board.html', parentId: 0, content: '<p>board</p>' },
    ],
    key: KEY,
    actor: { id: 7, username: 'studio', person: { id: 7, name: 'Studio' } },
    accounts: [{ id: 7, kind: 'team', username: 'studio', displayName: 'Studio', role: 'owner', syncBase: '/_/sync', lifecycle: 'active', sync: { enabled: true, reason: null } }],
  });
  t.after(() => fake.close());

  const roots = fs.realpathSync(fs.mkdtempSync(path.join(os.tmpdir(), 'htmlclay-import-')));
  t.after(() => fs.rmSync(roots, { recursive: true, force: true }));
  const folder = path.join(roots, 'studio');
  fs.mkdirSync(folder);
  const marker = path.join(folder, '.hyperclay', 'sync-root.json');
  const row = (rootId) => ({ id: SESSION, rootId, accountId: 7, kind: 'team', cached: { username: 'studio', displayName: 'Studio', role: 'owner' }, paused: null, legacyMetaDir: null, enabled: true });
  const key = async (s) => { s.send({ sync: { v: 1, key: KEY, serverUrl: fake.url } }); await s.line('SYNC KEY HELD'); };
  const configure = (s, rootId) => call(s, 'POST', '/sync/v1/configure', { body: { deviceId: 'device-1', paused: false, sessions: [row(rootId)], roots: [{ id: rootId, path: folder }] } });
  const status = async (s) => (await call(s, 'POST', '/sync/v1/status', { body: {} })).body.sessions;
  const metaOf = (s) => path.join(s.tmp, 'data', 'sync-meta', 'v2', SESSION);

  // 1. "Local": the first download, then a running session, with Local's own root id.
  const goOld = await recordingGo(t, folder);
  const local = await startService(t, { go: goOld.url, roots: [{ id: OLD, path: folder, url: 'http://studio.htmlclay.localhost:1/' }] });
  await key(local);
  const bound = await call(local, 'POST', '/sync/v1/bind', { body: { deviceId: 'device-1', session: row(OLD), root: { id: OLD, path: folder }, mode: 'empty' } });
  assert.equal(bound.body.ok, true, JSON.stringify(bound.body));
  assert.deepEqual((await configure(local, OLD)).body, { ok: true, running: [SESSION] });
  await call(local, 'POST', '/sync/v1/idle', { body: { timeoutMs: 20000 } });
  assert.equal(fs.readFileSync(path.join(folder, 'notes', 'todo.html'), 'utf8'), '<p>todo</p>');
  assert.equal(JSON.parse(fs.readFileSync(marker, 'utf8')).rootId, OLD);
  const identity = JSON.parse(fs.readFileSync(path.join(metaOf(local), 'identity.json'), 'utf8'));
  console.log('# identity.json as the old owner wrote it:', JSON.stringify(identity));
  console.log('# bookkeeping files:', fs.readdirSync(metaOf(local)).sort().join(', '));

  // 2. Local stops (a clean quit).
  local.send({ sync: { v: 1, key: null } });
  await local.line('SYNC KEY CLEARED');

  // 3. The import: copy the bookkeeping, then rewrite the id in identity.json and the marker.
  const goNew = await recordingGo(t, folder);
  const clay = await startService(t, { go: goNew.url, roots: [{ id: NEW, path: folder, url: 'http://studio.htmlclay.localhost:1/' }] });
  fs.mkdirSync(path.dirname(metaOf(clay)), { recursive: true });
  // MOVE_TEST_LEAVE_OUT names one step to leave out, to show the test can fail:
  // no-copy (no bookkeeping), no-identity (identity.json keeps the old id), no-marker (the folder keeps the old marker).
  const mutate = process.env.MOVE_TEST_LEAVE_OUT || '';
  if (mutate !== 'no-copy') {
    fs.cpSync(metaOf(local), metaOf(clay), { recursive: true });
    const identityFile = path.join(metaOf(clay), 'identity.json');
    if (mutate !== 'no-identity') fs.writeFileSync(identityFile, JSON.stringify({ ...JSON.parse(fs.readFileSync(identityFile, 'utf8')), rootId: NEW }, null, 2));
  }
  if (mutate !== 'no-marker') {
    const oldMarker = JSON.parse(fs.readFileSync(marker, 'utf8'));
    fs.writeFileSync(`${marker}.tmp`, JSON.stringify({ ...oldMarker, rootId: NEW }, null, 2));
    fs.renameSync(`${marker}.tmp`, marker);
  }

  // 4. HTML Clay starts the same session id on the new folder id.
  const requestsBefore = fake.requests.length;
  await key(clay);
  assert.deepEqual((await configure(clay, NEW)).body, { ok: true, running: [SESSION] });
  await call(clay, 'POST', '/sync/v1/idle', { body: { timeoutMs: 20000 } });
  await wait(1500);
  const [mine] = await status(clay);
  console.log('# new owner status:', JSON.stringify({ status: mine.status, paused: mine.paused, pendingCount: mine.pendingCount, conflicts: mine.conflicts, running: mine.running }));
  console.log(`# new owner so far: server changes=${JSON.stringify(mutating(fake, requestsBefore).map((r) => `${r.method} ${r.url || r.path}`))} folder writes=${JSON.stringify(writesOf(goNew, 0).map((c) => `${c.path} ${c.body && (c.body.name || c.body.rel) || ''}`))}`);
  assert.equal(mine.paused, null, 'the imported session is not paused');
  assert.equal(mine.conflicts, 0, 'no conflict was raised');
  assert.equal(mine.pendingCount, 0);
  assert.deepEqual(mutating(fake, requestsBefore).map((r) => `${r.method} ${r.url || r.path}`), [], 'the new owner sent no change to the server');
  assert.deepEqual(writesOf(goNew, 0).map((c) => c.path), [], 'the new owner wrote nothing in the folder');
  console.log(`# new owner after import: server reads=${fake.requests.length - requestsBefore}, server changes=0, folder writes=0`);

  // 5. It is live on the same nodes: a server edit to node 3 lands on notes/todo.html.
  await fake.server.putNodeContent(null, 3, '<p>todo, edited online</p>');
  fake.push({ type: 'node-saved', nodeId: 3, nodeType: 'site' });
  await until('the download after the import', () => commitsOf(goNew).length > 0);
  assert.deepEqual(commitsOf(goNew).map((c) => [c.body.root, c.body.name, decode(c.body.bytes).toString('utf8')]), [[NEW, 'notes/todo.html', '<p>todo, edited online</p>']]);
  await call(clay, 'POST', '/sync/v1/idle', { body: { timeoutMs: 20000 } });

  // 6. The old owner starts again with its own bookkeeping and its own id: refused, and it moves nothing.
  const oldCalls = goOld.calls.length;
  const beforeOld = fake.requests.length;
  await key(local);
  const again = await configure(local, OLD);
  await wait(2500);
  const [theirs] = await status(local);
  console.log('# old owner configure answer:', JSON.stringify(again.body), 'status:', JSON.stringify(theirs ? { status: theirs.status, paused: theirs.paused, running: theirs.running } : null));
  assert.ok(theirs && theirs.paused && theirs.paused.reason === 'folder-replaced', 'the old owner is paused as folder-replaced');
  assert.deepEqual(mutating(fake, beforeOld).map((r) => `${r.method} ${r.url || r.path}`), [], 'the old owner sent no change to the server');
  assert.deepEqual(writesOf(goOld, oldCalls).map((c) => c.path), [], 'the old owner wrote nothing in the folder');
  assert.equal(JSON.parse(fs.readFileSync(marker, 'utf8')).rootId, NEW, 'the marker still names the new owner');
  assert.equal(fs.readFileSync(path.join(folder, 'notes', 'todo.html'), 'utf8'), '<p>todo, edited online</p>');
});
