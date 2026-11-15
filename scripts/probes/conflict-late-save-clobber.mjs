// Probe: Guarded conflict choice (08-sync.md Decision 15) fails to detect edits
// saved after a conflict was detected (`service/vendor/local/src/sync-engine/reconcile/execute.js:175, 658-667`):
//   1) `recordConflict` preserves `open.detectedAt`, so Go's conflict id never
//      changes when new local saves arrive while the conflict sits in Needs You.
//   2) `resolveConflict` ignores `found.localChecksum`: on `both-edited` Keep theirs
//      reads the *current* disk checksum right inside `resolveConflict`, silently
//      overwriting the new save instead of refusing with `conflict-changed`; and on
//      `remote-deleted` Keep theirs calls `_applyFileDelete` (`sync-trash`) with NO
//      `expectLocal` check and NO `.hyperclay/versions/` backup at all.
import { existsSync, mkdirSync, mkdtempSync, readFileSync, writeFileSync } from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { build, page, save, startWorld, wait, walk, withMarker } from '../sync-harness.mjs';

function parseOut(argv) {
  const idx = argv.indexOf('--out');
  const dir = idx !== -1 && argv[idx + 1]
    ? path.resolve(argv[idx + 1])
    : mkdtempSync(path.join(os.tmpdir(), 'probe-conflict-late-save-'));
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
    { id: 11, type: 'site', name: 'both.html', parentId: 0, content: page('both.html', 'SEED-BOTH') },
    { id: 12, type: 'site', name: 'deleted.html', parentId: 0, content: page('deleted.html', 'SEED-DELETED') },
  ],
});

try {
  await world.d.send('pause', { paused: true });
  const { host, port } = await world.served();

  // Trigger both-edited on both.html and remote-deleted on deleted.html.
  await save({ port, host, doc: 'both.html', body: page('both.html', 'LOCAL-V1-AT-CONFLICT') });
  await world.server.putNodeContent(null, 11, withMarker(page('both.html', 'SEED-BOTH'), 'REMOTE-V1'));

  await save({ port, host, doc: 'deleted.html', body: page('deleted.html', 'LOCAL-DEL-V1') });
  await world.server.deleteNode(null, 12);

  await world.d.send('pause', { paused: false });
  const bothConflictBefore = await until('both-edited conflict on both.html in Needs You', async () => {
    const held = (await world.d.send('conflicts')).find((c) => c.path === 'both.html');
    if (!held) return null;
    const snap = await world.d.send('snapshot');
    return snap.waiting?.some((w) => w.id === `w-${held.id}`) ? held : null;
  });
  const delConflictBefore = await until('remote-deleted conflict on deleted.html in Needs You', async () => {
    const held = (await world.d.send('conflicts')).find((c) => c.path === 'deleted.html');
    if (!held) return null;
    const snap = await world.d.send('snapshot');
    return snap.waiting?.some((w) => w.id === `w-${held.id}`) ? held : null;
  });
  await world.d.send('idle', { timeoutMs: 30000 });

  // User already has Needs You open (holding bothConflictBefore / delConflictBefore).
  // Before clicking "Keep theirs", user saves important new edits in another tab / editor!
  const lateTabMarker = 'LATE-TAB-SAVE-AFTER-CONFLICT-OPENED';
  await save({ port, host, doc: 'both.html', body: page('both.html', lateTabMarker) });

  const lateEditorMarker = 'LATE-EDITOR-SAVE-ON-REMOTE-DELETED';
  writeFileSync(path.join(world.folder, 'deleted.html'), page('deleted.html', lateEditorMarker));
  await wait(2500);

  // User clicks "Keep theirs" using the conflict snapshot they opened earlier.
  let bothRefused = false;
  try {
    await world.d.send('resolve', { conflict: bothConflictBefore, keep: 'theirs' });
  } catch (err) {
    bothRefused = true;
  }

  let delRefused = false;
  try {
    await world.d.send('resolve', { conflict: delConflictBefore, keep: 'theirs' });
  } catch (err) {
    delRefused = true;
  }

  // Decision 15: Keep theirs goes ahead, and the bytes it replaces are kept, as a
  // version (both-edited) or in .trash under a free name (remote-deleted). A late
  // save is lost only when it is in none of the live file, the versions or .trash.
  const kept = (dir) => (existsSync(dir) ? walk(dir) : []).map((f) => readFileSync(f, 'utf8'));
  const versions = kept(path.join(world.folder, '.hyperclay', 'versions'));
  const trash = kept(path.join(world.folder, '.trash'));
  const liveBoth = readFileSync(path.join(world.folder, 'both.html'), 'utf8');
  const lateTabKept = liveBoth.includes(lateTabMarker) || versions.some((b) => b.includes(lateTabMarker));
  const lateEditorKept = versions.concat(trash).some((b) => b.includes(lateEditorMarker))
    || (existsSync(path.join(world.folder, 'deleted.html')) && readFileSync(path.join(world.folder, 'deleted.html'), 'utf8').includes(lateEditorMarker));

  if (!lateTabKept || !lateEditorKept) {
    console.log(`FAILED: a save made before Keep theirs is gone: lateTabKept=${lateTabKept} lateEditorKept=${lateEditorKept} bothRefused=${bothRefused} remoteDeletedRefused=${delRefused}`);
    process.exitCode = 1;
  } else {
    console.log(`OK: every save made before Keep theirs is kept (bothRefused=${bothRefused} remoteDeletedRefused=${delRefused})`);
  }
} finally {
  await world.close();
}
