// Probe: Resolving a `remote-deleted` conflict with "Keep mine"
// (service/vendor/local/src/sync-engine/reconcile/execute.js:692-708) creates a
// new nodeId on the server and inserts it into `engine.repo` (`node-map.json`),
// but forgets to delete the dead `found.key` (old nodeId) from `engine.repo`.
// Because `NodeRepository.getByPath` iterates `Map` in insertion order, the dead
// nodeId stays first for that path:
//   1) Subsequent saves to the document target the deleted nodeId (404) and
//      never reach hyperclay.com;
//   2) Any reconnect/restart sees the dead nodeId missing from the server
//      inventory: if the file still has post-baseline edits it raises a phantom
//      `remote-deleted` conflict whose "Keep mine" fails with 409 name-conflict,
//      and if the user reverted to the original text (`SEED-V0`) it trashes the
//      live file locally and deletes the newly created server node too!
import { existsSync, mkdirSync, mkdtempSync, readFileSync } from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { build, page, save, startWorld, wait } from '../sync-harness.mjs';

function parseOut(argv) {
  const idx = argv.indexOf('--out');
  const dir = idx !== -1 && argv[idx + 1]
    ? path.resolve(argv[idx + 1])
    : mkdtempSync(path.join(os.tmpdir(), 'probe-remote-deleted-stale-'));
  mkdirSync(dir, { recursive: true });
  return dir;
}

async function until(what, check, timeoutMs = 30000) {
  const deadline = Date.now() + timeoutMs;
  for (;;) {
    const value = await check();
    if (value) return value;
    if (Date.now() > deadline) throw new Error(`timed out waiting for ${what}`);
    await wait(100);
  }
}

const out = parseOut(process.argv.slice(2));
const built = build(out);
const v0 = page('board.html', 'SEED-V0');
const v1 = page('board.html', 'LOCAL-V1-KEPT');
const v2 = page('board.html', 'LOCAL-V2-AFTER-KEEP-MINE');

const world = await startWorld({
  out,
  built,
  nodes: [{ id: 11, type: 'site', name: 'board.html', parentId: 0, content: v0 }],
});

try {
  // While paused/offline, user edits board.html locally and teammate deletes it online.
  await world.d.send('pause', { paused: true });
  const { host, port } = await world.served();
  if ((await save({ port, host, doc: 'board.html', body: v1 })).status !== 200) {
    throw new Error('save v1 failed');
  }
  await world.server.deleteNode(null, 11);

  await world.d.send('pause', { paused: false });
  const conflict1 = await until('remote-deleted conflict on board.html in Needs You', async () => {
    const held = (await world.d.send('conflicts')).find((c) => c.path === 'board.html' && c.kind === 'remote-deleted');
    if (!held) return null;
    const snap = await world.d.send('snapshot');
    return snap.waiting?.some((w) => w.id === `w-${held.id}`) ? held : null;
  });
  await world.d.send('idle', { timeoutMs: 30000 });

  // User clicks "Keep mine": recreates board.html on server under a new node id (12),
  // leaving dead node "11" in node-map.json ahead of "12".
  await world.d.send('resolve', { conflict: conflict1, keep: 'mine' });
  const recreatedId = world.idOf('board.html');
  if (!recreatedId) throw new Error('expected board.html to be recreated on server');

  // 1) User continues editing board.html and saves LOCAL-V2.
  if ((await save({ port, host, doc: 'board.html', body: v2 })).status !== 200) {
    throw new Error('save v2 failed');
  }
  await wait(3000);
  const serverAfterV2 = world.server.get(recreatedId).content.toString('utf8');
  const v2Uploaded = serverAfterV2.includes('LOCAL-V2-AFTER-KEEP-MINE');

  // 2) Service restarts (or user reconnects): stale node "11" raises a phantom
  //    remote-deleted conflict on board.html whose "Keep mine" is refused (409)!
  await world.d.send('restartService', { timeoutMs: 60000 });
  await world.d.send('idle', { timeoutMs: 60000 });
  const phantom = (await world.d.send('conflicts')).find((c) => c.path === 'board.html');
  let phantomKeepMineError = null;
  if (phantom) {
    try {
      await world.d.send('resolve', { conflict: phantom, keep: 'mine' });
    } catch (err) {
      phantomKeepMineError = err.code || err.message;
    }
  }

  // 3) Even worse: on another file (`revert.html`, id 21), if user reverts to V0
  //    after Keep mine and reconnects, stale node "21" has local == baseline -> TRASH_LOCAL!
  await world.server.createNode(null, { type: 'site', name: 'revert.html', parentId: 0, content: page('revert.html', 'REVERT-V0') });
  const revertOrigId = world.idOf('revert.html');
  world.fake.push({ type: 'node-saved', nodeId: revertOrigId, nodeType: 'site' });
  await world.d.send('landed', { folder: world.folder, files: {}, timeoutMs: 30000 });
  await until('revert.html on disk', () => existsSync(path.join(world.folder, 'revert.html')));
  await world.d.send('idle', { timeoutMs: 30000 });

  await world.d.send('pause', { paused: true });
  await save({ port, host, doc: 'revert.html', body: page('revert.html', 'REVERT-V1') });
  await world.server.deleteNode(null, revertOrigId);
  await world.d.send('pause', { paused: false });
  const revertConflict = await until('remote-deleted conflict on revert.html in Needs You', async () => {
    const held = (await world.d.send('conflicts')).find((c) => c.path === 'revert.html');
    if (!held) return null;
    const snap = await world.d.send('snapshot');
    return snap.waiting?.some((w) => w.id === `w-${held.id}`) ? held : null;
  });
  await world.d.send('resolve', { conflict: revertConflict, keep: 'mine' });
  // User undoes their edit back to REVERT-V0, then service restarts:
  await save({ port, host, doc: 'revert.html', body: page('revert.html', 'REVERT-V0') });
  await world.d.send('restartService', { timeoutMs: 60000 });
  await world.d.send('idle', { timeoutMs: 60000 });
  await wait(2500);
  const revertExistsLocally = existsSync(path.join(world.folder, 'revert.html'));
  const revertExistsOnServer = world.idOf('revert.html') !== null;

  if (!v2Uploaded || phantom || !revertExistsLocally || !revertExistsOnServer) {
    console.log(
      `FAILED: stale nodeId after remote-deleted Keep mine broke future sync: ` +
      `v2Uploaded=${v2Uploaded} ` +
      `phantomConflict=${phantom ? phantom.kind : 'none'} ` +
      `phantomKeepMineRefused=${phantomKeepMineError} ` +
      `revertedFileStillOnDisk=${revertExistsLocally} ` +
      `revertedFileStillOnServer=${revertExistsOnServer}`
    );
    process.exitCode = 1;
  } else {
    console.log('OK: post-Keep-mine edits uploaded and no phantom conflict or self-trash occurred');
  }
} finally {
  await world.close();
}
