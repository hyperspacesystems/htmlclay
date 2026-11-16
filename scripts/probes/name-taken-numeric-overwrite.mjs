// Probe: Resolving a `name-taken` conflict ("Keep mine") on a filename starting
// with digits (e.g. `11-notes.html`) parses `parseInt("11-notes.html", 10)` -> `11`
// in `resolveConflict` (service/local/src/sync-engine/reconcile/execute.js:70-73,
// 633, 692, 714) and PUTs `11-notes.html`'s bytes unconditionally (`ifMatch: null`)
// onto unrelated server node 11 (`welcome.html`), destroying `welcome.html` on both
// the server and local disk.
import { existsSync, mkdirSync, mkdtempSync, readFileSync } from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { build, page, save, startWorld, wait } from '../sync-harness.mjs';

function parseOut(argv) {
  const idx = argv.indexOf('--out');
  const dir = idx !== -1 && argv[idx + 1]
    ? path.resolve(argv[idx + 1])
    : mkdtempSync(path.join(os.tmpdir(), 'probe-name-taken-'));
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
const victimMarker = 'VICTIM-WELCOME-ORIGINAL-DATA';
const otherDeviceMarker = 'OTHER-DEVICE-11-NOTES';
const localNotesMarker = 'LOCAL-11-NOTES-CONTENT';

const world = await startWorld({
  out,
  built,
  nodes: [
    { id: 11, type: 'site', name: 'welcome.html', parentId: 0, content: page('welcome.html', victimMarker) },
  ],
});

try {
  // Another device creates `11-notes.html` on hyperclay.com (gets node id 12)
  // just before this device uploads its own `11-notes.html`.
  await world.server.createNode(null, {
    type: 'site',
    name: '11-notes.html',
    parentId: 0,
    content: page('11-notes.html', otherDeviceMarker),
  });

  // User creates/saves `11-notes.html` locally in HTML Clay.
  const { host, port } = await world.served();
  const saved = await save({ port, host, doc: '11-notes.html', body: page('11-notes.html', localNotesMarker) });
  if (saved.status !== 200) throw new Error(`save failed: ${JSON.stringify(saved)}`);

  // Upload hits 409; the engine meets the node now holding the name (8.55), so the
  // file becomes a conflict on 11-notes.html against node 12, never a rename.
  const conflict = await until('a conflict on 11-notes.html in Needs You', async () => {
    const held = (await world.d.send('conflicts')).find((c) => c.path === '11-notes.html');
    if (!held) return null;
    const snap = await world.d.send('snapshot');
    return snap.waiting?.some((w) => w.id === `w-${held.id}`) ? held : null;
  });

  // User clicks "Keep mine".
  await world.d.send('resolve', { conflict, keep: 'mine' });

  const serverWelcomeAfterResolve = world.server.get(11).content.toString('utf8');
  await world.d.send('restartService', { timeoutMs: 60000 });
  await world.d.send('idle', { timeoutMs: 60000 });
  const welcomePath = path.join(world.folder, 'welcome.html');
  const localWelcomeAfterSync = existsSync(welcomePath) ? readFileSync(welcomePath, 'utf8') : '<MISSING>';

  const serverLostVictim = !serverWelcomeAfterResolve.includes(victimMarker) && serverWelcomeAfterResolve.includes(localNotesMarker);
  const localLostVictim = !localWelcomeAfterSync.includes(victimMarker);

  if (serverLostVictim || localLostVictim) {
    console.log(
      `FAILED: name-taken Keep mine on 11-notes.html overwrote unrelated node 11 (welcome.html): ` +
      `serverHasVictim=${serverWelcomeAfterResolve.includes(victimMarker)} ` +
      `serverWelcomeNowHas11Notes=${serverWelcomeAfterResolve.includes(localNotesMarker)} ` +
      `localWelcomeExists=${existsSync(welcomePath)} ` +
      `localHasVictim=${localWelcomeAfterSync.includes(victimMarker)}`
    );
    process.exitCode = 1;
  } else {
    console.log('OK: welcome.html preserved on both server and disk');
  }
} finally {
  await world.close();
}
