// 08-sync.md step 8.35: a held both-edited conflict survives a killed service, a
// killed host, a start while paused and a start with the folder's sync off. After
// each, the host still lists it, both byte sets are on disk, and the server has
// received no upload of the file. Then Keep both leaves both under two names.
//
// The real htmlclay and live-room service, driven by scripts/sync-product-driver.mjs,
// against L1's fake sync server (HTMLCLAY_SYNC_LOCAL, default service/test/fake);
// skips without that checkout.
import test from 'node:test';
import assert from 'node:assert/strict';
import { execFileSync, fork } from 'node:child_process';
import { createHash, randomBytes } from 'node:crypto';
import { chmodSync, existsSync, mkdtempSync, readFileSync, realpathSync, rmSync, writeFileSync } from 'node:fs';
import { createRequire } from 'node:module';
import os from 'node:os';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const REPO = path.resolve(fileURLToPath(new URL('../..', import.meta.url)));
const LOCAL = process.env.HTMLCLAY_SYNC_LOCAL || path.join(REPO, 'service', 'test', 'fake');
const FAKE = path.join(LOCAL, 'trial', 'fake-sync-http.cjs');
const SKIP = existsSync(FAKE) || process.env.HTMLCLAY_SYNC_LOCAL ? false : `no fake sync server at ${LOCAL}`;

const sha = (text) => createHash('sha256').update(text).digest('hex');
const KEY = `hcsk_${randomBytes(32).toString('hex')}`;
const BOARD = 10;
const BASE = '<p>board</p>';
const THEIRS = '<p>board, edited there</p>';
const MINE = '<p>board, edited here</p>';

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

async function until(what, check, timeoutMs = 30000) {
  const deadline = Date.now() + timeoutMs;
  for (;;) {
    const value = await check();
    if (value) return value;
    if (Date.now() > deadline) throw new Error(`timed out waiting for ${what}`);
    await new Promise((resolve) => setTimeout(resolve, 250));
  }
}

test('a held conflict survives restarts, pause and sync off (8.35)', { skip: SKIP, timeout: 300000 }, async (t) => {
  const base = realpathSync(mkdtempSync(path.join(os.homedir(), '.htmlclay-held-test-')));
  t.after(() => rmSync(base, { recursive: true, force: true }));
  const bin = path.join(base, 'htmlclay');
  execFileSync('go', ['build', '-tags', 'synctest', '-o', bin, './cmd/htmlclay'], { cwd: REPO, stdio: 'inherit' });
  const serviceCmd = path.join(base, 'service.sh');
  writeFileSync(serviceCmd, `#!/bin/sh\nexec ${JSON.stringify(process.execPath)} ${JSON.stringify(path.join(REPO, 'service', 'main.cjs'))} "$@"\n`);
  chmodSync(serviceCmd, 0o755);

  const require = createRequire(import.meta.url);
  const { startFakeSyncHttp } = require(FAKE);
  const fake = await startFakeSyncHttp({
    nodes: [{ id: BOARD, type: 'site', name: 'board.html', parentId: 0, content: BASE }],
    key: KEY,
    actor: { id: 12, username: 'maya', person: { id: 12, name: 'Maya' } },
    accounts: [
      { id: 12, kind: 'personal', username: 'maya', displayName: 'maya', role: 'owner', syncBase: '/_/sync', lifecycle: 'active', sync: { enabled: true, reason: null } },
    ],
  });
  t.after(() => fake.close());

  const d = driver();
  t.after(() => d.child.connected && d.child.disconnect());
  await d.ready;
  const folder = path.join(base, 'roots', 'maya');
  const board = path.join(folder, 'board.html');
  const uploads = () => fake.requests.filter((r) => r.method === 'PUT' && r.path === `/_/sync/nodes/${BOARD}/content`).length;
  let conflict = null;

  // The conflict is still held: Go lists it as a waiting item with the same id,
  // the file keeps my bytes, the server's bytes are kept beside it, and nothing
  // of mine reached the server.
  async function stillHeld() {
    assert.deepEqual(await d.send('conflicts'), [conflict]);
    // The engine writes its held list before its report reaches Go, so Go's
    // waiting item can follow a moment later.
    await until(`the waiting item w-${conflict.id}`, async () =>
      (await d.send('snapshot')).waiting.some((item) => item.id === `w-${conflict.id}`), 10000);
    assert.equal(readFileSync(board, 'utf8'), MINE);
    assert.equal(readFileSync(path.join(folder, conflict.remoteCopy), 'utf8'), THEIRS);
    assert.equal(uploads(), 0);
    assert.equal((await fake.server.getNodeContent(null, BOARD)).content, THEIRS);
  }

  await t.test('a both-edited conflict is held', async () => {
    await d.send('start', { bin, configBase: path.join(base, 'config'), log: path.join(base, 'host.log'), syncServer: fake.url, serviceCmd });
    await d.send('connect', { key: KEY });
    await d.send('setUp', { account: 'maya', folder });
    await d.send('landed', { folder, files: { 'board.html': sha(BASE) }, timeoutMs: 60000 });
    await d.send('idle', { timeoutMs: 60000 });
    await d.send('pause', { paused: true });
    writeFileSync(board, MINE);
    await fake.server.putNodeContent(null, BOARD, THEIRS);
    await d.send('pause', { paused: false });
    const held = await until('the conflict', async () => {
      const list = await d.send('conflicts');
      return list.length === 1 ? list : null;
    }, 60000);
    conflict = held[0];
    assert.equal(conflict.kind, 'both-edited');
    assert.equal(conflict.path, 'board.html');
    assert.ok(conflict.remoteCopy, JSON.stringify(conflict));
    await stillHeld();
  });

  await t.test('a killed service', async () => {
    await d.send('restartService');
    await d.send('idle', { timeoutMs: 60000 });
    await stillHeld();
  });

  await t.test('a killed host', async () => {
    await d.send('restartHost');
    await d.send('idle', { timeoutMs: 60000 });
    await stillHeld();
  });

  await t.test('a start while paused', async () => {
    await d.send('pause', { paused: true });
    await d.send('restartHost');
    assert.equal((await d.send('snapshot')).sync.paused, true);
    await stillHeld();
    await d.send('pause', { paused: false });
    await d.send('idle', { timeoutMs: 60000 });
    await stillHeld();
  });

  await t.test("a start with the folder's sync off", async () => {
    await d.send('setSynced', { folderID: conflict.folderID, on: false });
    await d.send('restartHost');
    await stillHeld();
    await d.send('setSynced', { folderID: conflict.folderID, on: true });
    await d.send('idle', { timeoutMs: 60000 });
    await stillHeld();
  });

  await t.test('Keep both leaves both byte sets under their two names', async () => {
    await d.send('resolve', { conflict, keep: 'both' });
    await d.send('landed', { folder, files: { 'board.html': sha(THEIRS), 'board-conflicted-copy.html': sha(MINE) }, timeoutMs: 60000 });
    await until('the conflict to clear', async () => (await d.send('conflicts')).length === 0, 60000);
  });

  await t.test('stop', async () => {
    assert.equal(await d.send('stop'), true);
  });
});
