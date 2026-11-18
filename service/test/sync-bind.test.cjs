'use strict';

// 8.20: the first download. preview counts an account's files; bind downloads them
// into a folder Go serves, every write a commit to Go; a folder with files is
// refused and left exactly as it was.

const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const { startService } = require('./harness.cjs');
const { call, SKIP, FAKE, KEY, decode, recordingGo, commitsOf } = require('./sync-fixture.cjs');

const EMPTY = 'b1b2b3b4b5b6b7b8';
const FULL = 'c1c2c3c4c5c6c7c8';
const ONE = '<p>one</p>';
const TWO = '<p>the second page</p>';

test('the first download', { skip: SKIP }, async (t) => {
  const { startFakeSyncHttp } = require(FAKE);
  const fake = await startFakeSyncHttp({
    nodes: [
      { id: 1, type: 'site', name: 'index.html', parentId: 0, content: ONE },
      { id: 2, type: 'folder', name: 'apps', parentId: 0 },
      { id: 3, type: 'site', name: 'two.html', parentId: 2, content: TWO },
    ],
    key: KEY,
    actor: { id: 5, username: 'maya', person: { id: 5, name: 'Maya' } },
    accounts: [{ id: 7, kind: 'team', username: 'studio', displayName: 'Studio', role: 'editor', syncBase: '/_/sync', lifecycle: 'active', sync: { enabled: true, reason: null } }],
  });
  t.after(() => fake.close());

  const base = fs.realpathSync(fs.mkdtempSync(path.join(os.tmpdir(), 'htmlclay-bind-')));
  t.after(() => fs.rmSync(base, { recursive: true, force: true }));
  const empty = path.join(base, 'Studio');
  const full = path.join(base, 'Full');
  fs.mkdirSync(empty);
  fs.mkdirSync(full);
  fs.writeFileSync(path.join(full, 'mine.html'), '<p>mine</p>');
  const go = await recordingGo(t, { [EMPTY]: empty, [FULL]: full });
  const s = await startService(t, {
    go: go.url,
    roots: [{ id: EMPTY, path: empty, url: 'http://studio.htmlclay.localhost:1/' }, { id: FULL, path: full, url: 'http://full.htmlclay.localhost:1/' }],
  });
  s.send({ sync: { v: 1, key: KEY, serverUrl: fake.url } });
  await s.line('SYNC KEY HELD');

  const session = (id, rootId) => ({ id, rootId, accountId: 7, kind: 'team', cached: { username: 'studio', displayName: 'Studio', role: 'editor' }, paused: null, legacyMetaDir: null, enabled: true });
  const bind = (body) => call(s, 'POST', '/sync/v1/bind', { body: { deviceId: 'device-1', ...body } });

  await t.test('preview counts the account files', async () => {
    const answer = await call(s, 'POST', '/sync/v1/preview', { body: { accountId: 7 } });
    assert.deepEqual(answer.body, { ok: true, files: 2, bytes: Buffer.byteLength(ONE) + Buffer.byteLength(TWO) });
    const missing = await call(s, 'POST', '/sync/v1/preview', { body: { accountId: 99 } });
    assert.deepEqual(missing.body, { ok: false, error: 'not-found' });
  });

  await t.test('a folder with files is refused and unchanged', async () => {
    const answer = await bind({ session: session('s-full', FULL), root: { id: FULL, path: full }, mode: 'empty' });
    assert.deepEqual(answer.body, { ok: false, error: 'folder-not-empty', resumable: false });
    const existing = await bind({ session: session('s-full', FULL), root: { id: FULL, path: full }, mode: 'existing' });
    assert.deepEqual(existing.body, { ok: false, error: 'folder-not-empty', resumable: false });
    assert.deepEqual(fs.readdirSync(full), ['mine.html']);
    assert.equal(fs.readFileSync(path.join(full, 'mine.html'), 'utf8'), '<p>mine</p>');
    assert.deepEqual(commitsOf(go, FULL), []);
  });

  await t.test('an empty folder gets every file, each a commit to Go', async () => {
    const answer = await bind({ session: session('s-studio', EMPTY), root: { id: EMPTY, path: empty }, mode: 'empty' });
    assert.equal(answer.body.ok, true, JSON.stringify(answer.body));
    assert.equal(answer.body.files, 2);
    const commits = commitsOf(go, EMPTY).map((c) => [c.body.name, decode(c.body.bytes).toString('utf8')]).sort();
    assert.deepEqual(commits, [['apps/two.html', TWO], ['index.html', ONE]]);
    assert.equal(fs.readFileSync(path.join(empty, 'index.html'), 'utf8'), ONE);
    assert.equal(fs.readFileSync(path.join(empty, 'apps', 'two.html'), 'utf8'), TWO);
    const status = await call(s, 'POST', '/sync/v1/status', { body: {} });
    assert.deepEqual(status.body.sessions.map((row) => row.sessionId), ['s-studio']);
  });

  await t.test('configure keeps a bound session running', async () => {
    const answer = await call(s, 'POST', '/sync/v1/configure', { body: { deviceId: 'device-1', paused: false, sessions: [session('s-studio', EMPTY)], roots: [{ id: EMPTY, path: empty }] } });
    assert.deepEqual(answer.body, { ok: true, running: ['s-studio'] });
  });

  await t.test('a cleared key stops the bound session', async () => {
    s.send({ sync: { v: 1, key: null } });
    await s.line('SYNC KEY CLEARED');
    const status = await call(s, 'POST', '/sync/v1/status', { body: {} });
    assert.deepEqual(status.body.sessions, []);
  });
});

test('an existing folder merges and loses nothing', { skip: SKIP }, async (t) => {
  const { startFakeSyncHttp } = require(FAKE);
  const fake = await startFakeSyncHttp({
    nodes: [
      { id: 1, type: 'site', name: 'index.html', parentId: 0, content: ONE },
      { id: 2, type: 'folder', name: 'apps', parentId: 0 },
      { id: 3, type: 'site', name: 'two.html', parentId: 2, content: TWO },
    ],
    key: KEY,
    actor: { id: 5, username: 'maya', person: { id: 5, name: 'Maya' } },
    accounts: [{ id: 5, kind: 'personal', username: 'maya', displayName: 'Maya', role: 'owner', syncBase: '/_/sync', lifecycle: 'active', sync: { enabled: true, reason: null } }],
  });
  t.after(() => fake.close());
  const base = fs.realpathSync(fs.mkdtempSync(path.join(os.tmpdir(), 'htmlclay-merge-')));
  t.after(() => fs.rmSync(base, { recursive: true, force: true }));
  const mine = path.join(base, 'Maya');
  const MINE = 'd1d2d3d4d5d6d7d8';
  fs.mkdirSync(path.join(mine, 'apps'), { recursive: true });
  fs.writeFileSync(path.join(mine, 'index.html'), ONE);
  fs.writeFileSync(path.join(mine, 'apps', 'two.html'), '<p>my own two</p>');
  fs.writeFileSync(path.join(mine, 'extra.html'), '<p>only here</p>');
  const go = await recordingGo(t, { [MINE]: mine });
  const s = await startService(t, { go: go.url, roots: [{ id: MINE, path: mine, url: 'http://maya.htmlclay.localhost:1/' }] });
  s.send({ sync: { v: 1, key: KEY, serverUrl: fake.url } });
  await s.line('SYNC KEY HELD');

  const preview = await call(s, 'POST', '/sync/v1/preview', { body: { accountId: 5, root: { id: MINE, path: mine } } });
  assert.deepEqual(preview.body, { ok: true, files: 2, bytes: Buffer.byteLength(ONE) + Buffer.byteLength(TWO), toUpload: 1, toDownload: 0, differ: 1 });
  assert.deepEqual(go.calls.filter((c) => c.path !== '/v1/unconfirmed'), [], 'a preview asks Go for nothing');

  const session = { id: 's-mine', rootId: MINE, accountId: 5, kind: 'personal', cached: { username: 'maya', displayName: 'Maya', role: 'owner' }, paused: null, legacyMetaDir: null, enabled: true };
  const bound = await call(s, 'POST', '/sync/v1/bind', { body: { deviceId: 'device-1', session, root: { id: MINE, path: mine }, mode: 'existing' } });
  assert.equal(bound.body.ok, true, JSON.stringify(bound.body));
  const idle = await call(s, 'POST', '/sync/v1/idle', { body: { timeoutMs: 30000 } });
  assert.equal(idle.body.idle, true);

  assert.equal(fs.readFileSync(path.join(mine, 'index.html'), 'utf8'), ONE);
  assert.equal(fs.readFileSync(path.join(mine, 'apps', 'two.html'), 'utf8'), '<p>my own two</p>');
  assert.equal(fs.readFileSync(path.join(mine, 'extra.html'), 'utf8'), '<p>only here</p>');
  assert.ok(fake.server.paths().includes('extra.html'), `the server has ${fake.server.paths()}`);
  const serverTwo = await fake.server.getNodeContent(null, 3);
  assert.equal(String(serverTwo.content), TWO);
  assert.equal(fake.requests.filter((r) => r.method === 'DELETE').length, 0, 'nothing was deleted on the server');

  s.send({ sync: { v: 1, key: null } });
  await s.line('SYNC KEY CLEARED');
});

// A served folder whose saved path differs from the disk in letter case (or, on
// Windows, uses a short name like RUNNER~1) is still one folder: the service and
// the sync engine must resolve it the same way, or the bind reports offline.
test('a folder saved under another letter case still binds', { skip: SKIP }, async (t) => {
  const { startFakeSyncHttp } = require(FAKE);
  const fake = await startFakeSyncHttp({
    nodes: [{ id: 1, type: 'site', name: 'index.html', parentId: 0, content: ONE }],
    key: KEY,
    actor: { id: 5, username: 'maya', person: { id: 5, name: 'Maya' } },
    accounts: [{ id: 7, kind: 'team', username: 'studio', displayName: 'Studio', role: 'editor', syncBase: '/_/sync', lifecycle: 'active', sync: { enabled: true, reason: null } }],
  });
  t.after(() => fake.close());
  const base = fs.realpathSync.native(fs.mkdtempSync(path.join(os.tmpdir(), 'htmlclay-case-')));
  t.after(() => fs.rmSync(base, { recursive: true, force: true }));
  fs.mkdirSync(path.join(base, 'Studio'));
  const saved = path.join(base, 'studio');
  if (!fs.existsSync(saved)) {
    t.skip('this disk tells letter case apart');
    return;
  }
  const go = await recordingGo(t, { [EMPTY]: saved });
  const s = await startService(t, { go: go.url, roots: [{ id: EMPTY, path: saved, url: 'http://studio.htmlclay.localhost:1/' }] });
  s.send({ sync: { v: 1, key: KEY, serverUrl: fake.url } });
  await s.line('SYNC KEY HELD');

  const session = { id: 's-case', rootId: EMPTY, accountId: 7, kind: 'team', cached: { username: 'studio', displayName: 'Studio', role: 'editor' }, paused: null, legacyMetaDir: null, enabled: true };
  const answer = await call(s, 'POST', '/sync/v1/bind', { body: { deviceId: 'device-1', session, root: { id: EMPTY, path: saved }, mode: 'empty' } });
  assert.equal(answer.body.ok, true, JSON.stringify(answer.body));
  assert.equal(fs.readFileSync(path.join(base, 'Studio', 'index.html'), 'utf8'), ONE);
});
