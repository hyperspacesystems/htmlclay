'use strict';

// 8.26: the engine's writes with a Go that consults the service back. Real Go
// settles a document commit with /v1/settle and prepares a sync move or trash
// with /v1/structure/prepare while it holds its lock; the engine must hold no
// boundary of its own across those calls, or each waits out Go's deadline and the
// first download fails with "may not have landed". The stand-in Go here makes
// those calls, with the deadline shortened to 2 s.

const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const { startService } = require('./harness.cjs');
const { call, SKIP, FAKE, KEY, until, recordingGo } = require('./sync-fixture.cjs');

const ROOT = 'd1d2d3d4d5d6d7d8';
const ONE = '<p>one</p>';
const TWO = '<p>two</p>';

test('the engine with a Go that consults back', { skip: SKIP }, async (t) => {
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
  const base = fs.realpathSync(fs.mkdtempSync(path.join(os.tmpdir(), 'htmlclay-consult-')));
  t.after(() => fs.rmSync(base, { recursive: true, force: true }));
  const folder = path.join(base, 'Studio');
  fs.mkdirSync(folder);
  const go = await recordingGo(t, { [ROOT]: folder }, { consult: true, deadlineMs: 2000 });
  const s = await startService(t, { go: go.url, roots: [{ id: ROOT, path: folder, url: 'http://studio.htmlclay.localhost:1/' }] });
  go.attach(s);
  s.send({ sync: { v: 1, key: KEY, serverUrl: fake.url } });
  await s.line('SYNC KEY HELD');
  const read = (rel) => (fs.existsSync(path.join(folder, rel)) ? fs.readFileSync(path.join(folder, rel), 'utf8') : null);
  const failedConsults = () => go.consults.filter((c) => !c.reply || c.reply.ok === false || c.reply.decision === 'refuse');

  await t.test('the first download settles every document', async () => {
    const session = { id: 's-consult', rootId: ROOT, accountId: 7, kind: 'team', cached: { username: 'studio', displayName: 'Studio', role: 'editor' }, paused: null, legacyMetaDir: null, enabled: true };
    const answer = await call(s, 'POST', '/sync/v1/bind', { body: { deviceId: 'device-1', session, root: { id: ROOT, path: folder }, mode: 'empty' } });
    assert.equal(answer.body.ok, true, JSON.stringify(answer.body));
    assert.equal(read('index.html'), ONE);
    assert.equal(read('apps/two.html'), TWO);
    const settles = go.consults.filter((c) => c.path === '/v1/settle');
    assert.ok(settles.length >= 2, `settles: ${JSON.stringify(go.consults)}`);
    assert.deepEqual(failedConsults(), []);
  });

  await t.test('an edit on the server lands and settles', async () => {
    await fake.server.putNodeContent(null, 1, '<p>one, edited</p>');
    fake.push({ type: 'node-saved', nodeId: 1, nodeType: 'site' });
    await until('the edit', () => read('index.html') === '<p>one, edited</p>', 15000);
    assert.deepEqual(failedConsults(), []);
  });

  await t.test('a rename on the server is a prepared and settled move', async () => {
    await fake.server.renameNode(null, 3, 'renamed.html');
    fake.push({ type: 'node-renamed', nodeId: 3 });
    await until('the rename', () => read('apps/renamed.html') === TWO && read('apps/two.html') === null, 15000);
    assert.ok(go.consults.some((c) => c.path === '/v1/structure/prepare'), JSON.stringify(go.consults));
    assert.deepEqual(failedConsults(), []);
  });

  await t.test('a removal on the server is a prepared and settled trash', async () => {
    await fake.server.deleteNode(null, 1);
    fake.push({ type: 'node-deleted', nodeId: 1 });
    await until('the removal', () => read('index.html') === null, 15000);
    assert.deepEqual(failedConsults(), []);
  });

  await t.test('resolve answers for a file with no conflict and a choice it does not know', async () => {
    const resolve = (body) => call(s, 'POST', '/sync/v1/resolve', { body });
    assert.deepEqual((await resolve({ sessionId: 's-consult', path: 'apps/renamed.html', keep: 'mine' })).body, { ok: false, error: 'no-conflict' });
    assert.deepEqual((await resolve({ sessionId: 's-consult', path: 'apps/renamed.html', keep: 'all' })).body, { ok: false, error: 'bad-choice' });
    assert.deepEqual((await resolve({ sessionId: 'nobody', path: 'apps/renamed.html', keep: 'mine' })).body, { ok: false, error: 'no-conflict' });
  });

  s.send({ sync: { v: 1, key: null } });
  await s.line('SYNC KEY CLEARED');
});
