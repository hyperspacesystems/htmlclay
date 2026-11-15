'use strict';

// 8.17: the service's sync sessions run Local's engine against L1's fake server,
// and every write lands as a call to Go. The stub Go below records each call and
// writes to disk as Go does, so the engine's own scans see what it wrote.

const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const { startService } = require('./harness.cjs');
const { call, SKIP, FAKE, KEY, wait, until, decode, recordingGo, commitsOf } = require('./sync-fixture.cjs');

const ROOT_ID = 'a1a2a3a4a5a6a7a8';

test('sync sessions in the service', { skip: SKIP }, async (t) => {
  const { startFakeSyncHttp } = require(FAKE);
  const fake = await startFakeSyncHttp({
    nodes: [{ id: 1, type: 'site', name: 'index.html', parentId: 0, content: '<p>from the server</p>' }],
    key: KEY,
    actor: { id: 7, username: 'studio', person: { id: 7, name: 'Studio' } },
    accounts: [{ id: 7, kind: 'team', username: 'studio', displayName: 'Studio', role: 'owner', syncBase: '/_/sync', lifecycle: 'active', sync: { enabled: true, reason: null } }],
  });
  t.after(() => fake.close());

  const roots = fs.realpathSync(fs.mkdtempSync(path.join(require('node:os').tmpdir(), 'htmlclay-sync-')));
  t.after(() => fs.rmSync(roots, { recursive: true, force: true }));
  const folder = path.join(roots, 'studio');
  fs.mkdirSync(folder);
  const go = await recordingGo(t, folder);
  const s = await startService(t, { go: go.url, roots: [{ id: ROOT_ID, path: folder, url: 'http://studio.htmlclay.localhost:1/' }] });

  const session = { id: 's1', rootId: ROOT_ID, accountId: 7, kind: 'team', cached: { username: 'studio', displayName: 'Studio', role: 'owner' }, paused: null, legacyMetaDir: null, enabled: true };
  const configure = () => call(s, 'POST', '/sync/v1/configure', { body: { deviceId: 'device-1', paused: false, sessions: [session], roots: [{ id: ROOT_ID, path: folder }] } });

  await t.test('nothing starts before the key', async () => {
    const answer = await configure();
    assert.deepEqual(answer.body, { ok: false, error: 'not-connected' });
    await wait(300);
    assert.equal(fake.requests.length, 0, 'the server heard nothing');
    assert.deepEqual(go.calls.filter((c) => c.path !== '/v1/unconfirmed'), [], 'Go was asked for nothing');
  });

  await t.test('a remote file is downloaded as a commit to Go', async () => {
    s.send({ sync: { v: 1, key: KEY, serverUrl: fake.url } });
    await s.line('SYNC KEY HELD');
    const answer = await configure();
    assert.deepEqual(answer.body, { ok: true, running: ['s1'] });
    await until('the download commit', () => commitsOf(go).length > 0);
    const [commit] = commitsOf(go);
    assert.equal(commit.body.root, ROOT_ID);
    assert.equal(commit.body.name, 'index.html');
    assert.equal(decode(commit.body.bytes).toString('utf8'), '<p>from the server</p>');
    assert.equal(fs.readFileSync(path.join(folder, 'index.html'), 'utf8'), '<p>from the server</p>');
  });

  await t.test('a node-saved frame is only a reason to compare', async () => {
    await call(s, 'POST', '/sync/v1/idle', { body: { timeoutMs: 20000 } });
    const before = commitsOf(go).length;
    await fake.server.putNodeContent(null, 1, '<p>the server, edited</p>');
    fake.push({ type: 'node-saved', nodeId: 1, nodeType: 'site', content: '<p>wrong on purpose</p>' });
    await until('the second download commit', () => commitsOf(go).length > before);
    const latest = commitsOf(go).slice(before).map((c) => decode(c.body.bytes).toString('utf8'));
    assert.deepEqual(latest, ['<p>the server, edited</p>']);
    assert.equal(fs.readFileSync(path.join(folder, 'index.html'), 'utf8'), '<p>the server, edited</p>');
  });

  await t.test('status reports the running session', async () => {
    const answer = await call(s, 'POST', '/sync/v1/status', { body: {} });
    assert.equal(answer.status, 200);
    assert.deepEqual(answer.body.sessions.map((row) => [row.sessionId, row.rootId]), [['s1', ROOT_ID]]);
  });

  await t.test('a cleared key stops the sessions', async () => {
    s.send({ sync: { v: 1, key: null } });
    await s.line('SYNC KEY CLEARED');
    const answer = await call(s, 'POST', '/sync/v1/status', { body: {} });
    assert.deepEqual(answer.body.sessions, []);
  });
});
