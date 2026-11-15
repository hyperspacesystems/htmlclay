// Probe: Remote folder deletion breaks both offline and live subfolder edits:
//   A) Offline/paused (`service/vendor/local/src/sync-engine/reconcile/execute.js:696`):
//      When subfolder `projects` is deleted on hyperclay.com while user edited
//      `projects/spec.html` locally, `performInitialFolderSync` forgets `projects`
//      from `nodeMap` and `performInitialSync` holds a `remote-deleted` conflict on
//      `projects/spec.html`. Clicking "Keep mine" calls `resolveParentIdByPath("projects")`
//      without recreating `projects`, throwing `Target folder not tracked in nodeMap: projects`!
//   B) Live SSE (`service/vendor/local/src/sync-engine/engine-session.js:192-194` ->
//      `_applyFolderDelete` in `engine-sse.js:389`):
//      When `projects/spec.html` has an open conflict (or un-uploaded local work) and
//      a teammate deletes `projects` online, `refreshNodeInLane` trashes the entire
//      `projects/` directory immediately without checking descendant conflicts/edits,
//      leaving a ghost conflict in Needs You whose local file is missing (`ENOENT`)!
import { existsSync, mkdirSync, mkdtempSync } from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { build, page, save, startWorld, wait, withMarker } from '../sync-harness.mjs';

function parseOut(argv) {
  const idx = argv.indexOf('--out');
  const dir = idx !== -1 && argv[idx + 1]
    ? path.resolve(argv[idx + 1])
    : mkdtempSync(path.join(os.tmpdir(), 'probe-deleted-parent-'));
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
const world = await startWorld({
  out,
  built,
  nodes: [
    { id: 10, type: 'folder', name: 'projects', parentId: 0 },
    { id: 11, type: 'site', name: 'spec.html', parentId: 10, content: page('spec.html', 'SPEC-V0') },
    { id: 20, type: 'folder', name: 'live', parentId: 0 },
    { id: 21, type: 'site', name: 'draft.html', parentId: 20, content: page('draft.html', 'DRAFT-V0') },
  ],
});

try {
  const { host, port } = await world.served();

  // Part A: Offline/paused folder deletion -> `remote-deleted` conflict -> Keep mine fails!
  await world.d.send('pause', { paused: true });
  await save({ port, host, doc: 'projects/spec.html', body: page('spec.html', 'LOCAL-SPEC-EDIT') });
  await world.server.deleteNode(null, 10, { cascade: true }); // cascade-deletes folder 10 and child 11

  // Also set up a held both-edited conflict in `live/draft.html` for Part B:
  await save({ port, host, doc: 'live/draft.html', body: page('draft.html', 'LOCAL-DRAFT-CONFLICTED') });
  await world.server.putNodeContent(null, 21, withMarker(page('draft.html', 'DRAFT-V0'), 'REMOTE-DRAFT-V1'));

  await world.d.send('pause', { paused: false });
  const specConflict = await until('remote-deleted conflict on projects/spec.html in Needs You', async () => {
    const held = (await world.d.send('conflicts')).find((c) => c.path === 'projects/spec.html');
    if (!held) return null;
    const snap = await world.d.send('snapshot');
    return snap.waiting?.some((w) => w.id === `w-${held.id}`) ? held : null;
  });
  await until('both-edited conflict on live/draft.html in Needs You', async () => {
    const held = (await world.d.send('conflicts')).find((c) => c.path === 'live/draft.html');
    if (!held) return null;
    const snap = await world.d.send('snapshot');
    return snap.waiting?.some((w) => w.id === `w-${held.id}`) ? held : null;
  });
  await world.d.send('idle', { timeoutMs: 30000 });

  let keepMineOfflineError = null;
  try {
    await world.d.send('resolve', { conflict: specConflict, keep: 'mine' });
  } catch (err) {
    keepMineOfflineError = err.code || err.message;
  }

  // Part B: While session is LIVE and `live/draft.html` has an open conflict in Needs You,
  // teammate deletes folder `live` (id 20) on hyperclay.com!
  await world.server.deleteNode(null, 20, { cascade: true });
  world.fake.push({ type: 'node-deleted', nodeId: 20, nodeType: 'folder', path: 'live' });
  // 8.55: the folder holds a file with unsent work, so it is decided file by file
  // and draft.html stays; give the live delete time to run before checking.
  await wait(3000);
  await world.d.send('idle', { timeoutMs: 30000 });

  const ghostConflictStillOpen = (await world.d.send('conflicts')).find((c) => c.path === 'live/draft.html');
  let keepMineLiveError = null;
  if (ghostConflictStillOpen) {
    try {
      await world.d.send('resolve', { conflict: ghostConflictStillOpen, keep: 'mine' });
    } catch (err) {
      keepMineLiveError = err.code || err.message;
    }
  }

  if (keepMineOfflineError || !existsSync(path.join(world.folder, 'live', 'draft.html')) || keepMineLiveError) {
    console.log(
      `FAILED: remote folder delete stranded subfolder edits: ` +
      `offlineKeepMineRefused=${keepMineOfflineError} ` +
      `liveConflictedFileStillPresent=${existsSync(path.join(world.folder, 'live', 'draft.html'))} ` +
      `liveGhostConflictKeepMineRefused=${keepMineLiveError}`
    );
    process.exitCode = 1;
  } else {
    console.log('OK: subfolder edits survived remote folder delete');
  }
} finally {
  await world.close();
}
