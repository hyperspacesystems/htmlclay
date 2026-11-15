// 08-sync.md step 8.27: the product driver against L1's fake sync server. One
// desktop: start, connect, set up a team folder, idle, checksums, landed (an edit from
// the server), pause, a killed service, a killed host, conflicts, stop. resolve is
// proved by 8.29's round trip, once conflict.resolve exists (8.32).
//
// Needs Go and a Hyperclay Local checkout with trial/fake-sync-http.cjs
// (HTMLCLAY_SYNC_LOCAL, default ../hyperclay-local); skips without the checkout.
import test from 'node:test';
import assert from 'node:assert/strict';
import { execFileSync, fork } from 'node:child_process';
import { createHash, randomBytes } from 'node:crypto';
import { chmodSync, existsSync, mkdtempSync, realpathSync, rmSync, writeFileSync } from 'node:fs';
import { createRequire } from 'node:module';
import os from 'node:os';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const REPO = path.resolve(fileURLToPath(new URL('../..', import.meta.url)));
const LOCAL = process.env.HTMLCLAY_SYNC_LOCAL || path.join(REPO, '..', 'hyperclay-local');
const FAKE = path.join(LOCAL, 'trial', 'fake-sync-http.cjs');
const SKIP = existsSync(FAKE) || process.env.HTMLCLAY_SYNC_LOCAL ? false : `no Hyperclay Local checkout at ${LOCAL}`;

const sha = (text) => createHash('sha256').update(text).digest('hex');
const KEY = `hcsk_${randomBytes(32).toString('hex')}`;
const DOCS = [
  { id: 10, type: 'site', name: 'index.html', parentId: 0, content: '<p>index</p>' },
  { id: 1, type: 'folder', name: 'apps', parentId: 0 },
  { id: 11, type: 'site', name: 'two.html', parentId: 1, content: '<p>two</p>' },
];

function driver() {
  const child = fork(path.join(REPO, 'scripts', 'sync-product-driver.mjs'), [], { stdio: ['ignore', 'inherit', 'inherit', 'ipc'] });
  let next = 0;
  const pending = new Map();
  const ready = new Promise((resolve) => child.once('message', resolve));
  child.on('message', (message) => {
    const waiter = pending.get(message.id);
    if (!waiter) return;
    pending.delete(message.id);
    if (message.ok) waiter.resolve(message.result);
    else waiter.reject(Object.assign(new Error(message.error.message), { code: message.error.code }));
  });
  const send = (cmd, args = {}) => new Promise((resolve, reject) => {
    next += 1;
    pending.set(next, { resolve, reject });
    child.send({ id: next, cmd, args });
  });
  return { child, ready, send };
}

test('the product driver', { skip: SKIP, timeout: 240000 }, async (t) => {
  const base = realpathSync(mkdtempSync(path.join(os.homedir(), '.htmlclay-driver-test-')));
  t.after(() => rmSync(base, { recursive: true, force: true }));
  const bin = path.join(base, 'htmlclay');
  execFileSync('go', ['build', '-tags', 'synctest', '-o', bin, './cmd/htmlclay'], { cwd: REPO, stdio: 'inherit' });
  const serviceCmd = path.join(base, 'service.sh');
  writeFileSync(serviceCmd, `#!/bin/sh\nexec ${JSON.stringify(process.execPath)} ${JSON.stringify(path.join(REPO, 'service', 'main.cjs'))} "$@"\n`);
  chmodSync(serviceCmd, 0o755);

  const require = createRequire(import.meta.url);
  const { startFakeSyncHttp } = require(FAKE);
  const fake = await startFakeSyncHttp({
    nodes: DOCS,
    key: KEY,
    actor: { id: 12, username: 'maya', person: { id: 12, name: 'Maya' } },
    accounts: [
      { id: 12, kind: 'personal', username: 'maya', displayName: 'maya', role: 'owner', syncBase: '/_/sync', lifecycle: 'active', sync: { enabled: true, reason: null } },
      { id: 40, kind: 'team', username: 'studio', displayName: 'Studio', role: 'editor', syncBase: '/_/sync', lifecycle: 'active', sync: { enabled: true, reason: null } },
    ],
  });
  t.after(() => fake.close());

  const d = driver();
  t.after(() => d.child.connected && d.child.disconnect());
  await d.ready;
  const folder = path.join(base, 'roots', 'Studio');

  await t.test('start, connect and set up a team folder', async () => {
    const started = await d.send('start', { bin, configBase: path.join(base, 'config'), log: path.join(base, 'host.log'), syncServer: fake.url, serviceCmd });
    assert.ok(started.port > 0);
    assert.deepEqual(await d.send('connect', { key: KEY }), { connected: true, username: 'maya' });
    const setUp = await d.send('setUp', { account: 'studio', folder });
    assert.match(setUp.folderID, /^[0-9a-f]{16}$/);
  });

  await t.test('idle answers once every file is here, and checksums match the server', async () => {
    await d.send('idle', { timeoutMs: 60000 });
    assert.deepEqual(await d.send('checksums', { folder }), { 'apps/two.html': sha(DOCS[2].content), 'index.html': sha(DOCS[0].content) });
  });

  await t.test('an edit on the server lands, and landed waits for it', async () => {
    await fake.server.putNodeContent(null, 10, '<p>index, edited</p>');
    fake.push({ type: 'node-saved', nodeId: 10, nodeType: 'site' });
    const sums = await d.send('landed', { folder, files: { 'index.html': sha('<p>index, edited</p>') }, timeoutMs: 30000 });
    assert.equal(sums['apps/two.html'], sha(DOCS[2].content));
    await assert.rejects(d.send('landed', { folder, files: { 'index.html': null }, timeoutMs: 1000 }), { code: 'timeout' });
  });

  await t.test('pause and resume', async () => {
    assert.deepEqual(await d.send('pause', { paused: true }), { paused: true });
    assert.deepEqual(await d.send('pause', { paused: false }), { paused: false });
  });

  await t.test('a killed service comes back and the folder settles again', async () => {
    const { before, after } = await d.send('restartService');
    assert.notEqual(before, after);
    await d.send('idle', { timeoutMs: 60000 });
  });

  await t.test('a killed host comes back on the same config, still connected', async () => {
    const { before, after } = await d.send('restartHost');
    assert.notEqual(before, after);
    await d.send('idle', { timeoutMs: 60000 });
    assert.equal((await d.send('snapshot')).sync.connected, true);
    assert.deepEqual(await d.send('conflicts'), []);
  });

  await t.test('stop', async () => {
    assert.equal(await d.send('stop'), true);
  });
});
