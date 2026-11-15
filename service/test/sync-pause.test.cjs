'use strict';

// 8.23: paused, the service asks the server nothing and holds no stream open;
// switched back on, it syncs again. Discovery runs every 200 ms here, so a timer
// left running would show within the paused seconds.

const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const { startService } = require('./harness.cjs');
const { call, SKIP, FAKE, KEY, wait, until } = require('./sync-fixture.cjs');

const ROOT = 'e1e2e3e4e5e6e7e8';

test('pausing sync', { skip: SKIP }, async (t) => {
  const { startFakeSyncHttp } = require(FAKE);
  const fake = await startFakeSyncHttp({
    nodes: [{ id: 1, type: 'site', name: 'index.html', parentId: 0, content: '<p>one</p>' }],
    key: KEY,
    actor: { id: 5, username: 'maya', person: { id: 5, name: 'Maya' } },
    accounts: [{ id: 5, kind: 'personal', username: 'maya', displayName: 'Maya', role: 'owner', syncBase: '/_/sync', lifecycle: 'active', sync: { enabled: true, reason: null } }],
  });
  t.after(() => fake.close());
  const base = fs.realpathSync(fs.mkdtempSync(path.join(os.tmpdir(), 'htmlclay-pause-')));
  t.after(() => fs.rmSync(base, { recursive: true, force: true }));
  const folder = path.join(base, 'Maya');
  fs.mkdirSync(folder);
  const go = await require('./sync-fixture.cjs').recordingGo(t, { [ROOT]: folder });
  const s = await startService(t, { go: go.url, roots: [{ id: ROOT, path: folder, url: 'http://maya.htmlclay.localhost:1/' }], env: { HTMLCLAY_SYNC_DISCOVERY_MS: '200' } });
  s.send({ sync: { v: 1, key: KEY, serverUrl: fake.url } });
  await s.line('SYNC KEY HELD');

  const session = { id: 's-maya', rootId: ROOT, accountId: 5, kind: 'personal', cached: { username: 'maya', displayName: 'Maya', role: 'owner' }, paused: null, legacyMetaDir: null, enabled: true };
  const configure = (paused) => call(s, 'POST', '/sync/v1/configure', { body: { deviceId: 'device-1', paused, sessions: [session], roots: [{ id: ROOT, path: folder }] } });

  assert.deepEqual((await configure(false)).body, { ok: true, running: ['s-maya'] });
  await until('the stream to open', () => fake.openStreams() === 1);
  await until('a discovery from the timer', () => fake.requests.filter((r) => r.path === '/_/sync/accounts').length >= 3);

  await t.test('paused asks nothing and holds no stream', async () => {
    assert.deepEqual((await configure(true)).body, { ok: true, running: [] });
    await until('the stream to close', () => fake.openStreams() === 0);
    const mark = fake.requests.length;
    await wait(3000);
    assert.deepEqual(fake.requests.slice(mark), [], 'requests while paused');
    assert.equal(fake.openStreams(), 0);
  });

  await t.test('resumed syncs again', async () => {
    const mark = fake.requests.length;
    assert.deepEqual((await configure(false)).body, { ok: true, running: ['s-maya'] });
    await until('the stream to open again', () => fake.openStreams() === 1);
    assert.ok(fake.requests.length > mark);
  });

  s.send({ sync: { v: 1, key: null } });
  await s.line('SYNC KEY CLEARED');
});
