import { mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { build, hex, page, sha, startWorld, wait, walk } from '../sync-harness.mjs';

function parseOut(argv) {
  const idx = argv.indexOf('--out');
  const dir = idx !== -1 && argv[idx + 1]
    ? path.resolve(argv[idx + 1])
    : mkdtempSync(path.join(os.tmpdir(), 'probe-remote-rename-overwrite-'));
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

function findBytesLocal(folder, targetSha) {
  return walk(folder).filter((file) => sha(readFileSync(file)) === targetSha).map((f) => path.relative(folder, f));
}

function findBytesOnServer(server, targetSha) {
  const hits = [];
  for (const [id, node] of server.nodes) {
    if (node.content && sha(Buffer.from(node.content)) === targetSha) hits.push(`active:${server.relPathOf(id)}`);
  }
  for (const [id, node] of server.trash) {
    if (node.content && sha(Buffer.from(node.content)) === targetSha) hits.push(`trash:${id}`);
  }
  return hits;
}

const out = parseOut(process.argv.slice(2));
const built = build(out);

const teamHeroBytes = Buffer.from(`TEAM-PUBLISHED-HERO-${hex()}\n`);
const mayaPlaceholderBytes = Buffer.from(`MAYA-UNRELATED-LOCAL-HERO-${hex()}\n`);
const teamDocHtml = page('draft.html', `TEAM-PUBLISHED-DOC-${hex()}`);
const mayaNotesHtml = page('launch.html', `MAYA-UNRELATED-LOCAL-DOC-${hex()}`);
const teamLiveBytes = Buffer.from(`TEAM-LIVE-ASSET-${hex()}\n`);
const mayaLiveBytes = Buffer.from(`MAYA-LIVE-OTHER-${hex()}\n`);

const world = await startWorld({
  out,
  built,
  nodes: [
    { id: 11, type: 'site', name: 'draft.html', parentId: 0, content: teamDocHtml },
    { id: 40, type: 'folder', name: 'assets', parentId: 0 },
    { id: 41, type: 'upload', name: 'draft-hero.png', parentId: 40, content: teamHeroBytes },
    { id: 42, type: 'upload', name: 'live-draft.png', parentId: 40, content: teamLiveBytes },
  ],
});

try {
  await world.d.send('landed', {
    folder: world.folder,
    files: {
      'draft.html': sha(teamDocHtml),
      'assets/draft-hero.png': sha(teamHeroBytes),
      'assets/live-draft.png': sha(teamLiveBytes),
    },
    timeoutMs: 45000,
  });
  await world.d.send('idle', { timeoutMs: 30000 });

  // --- Case 1: Offline / paused reconcile (correlateServerFile in engine-initial-sync.js:451-492) ---
  await world.d.send('pause', { paused: true });
  rmSync(path.join(world.folder, 'draft.html'));
  rmSync(path.join(world.folder, 'assets', 'draft-hero.png'));
  writeFileSync(path.join(world.folder, 'launch.html'), mayaNotesHtml);
  writeFileSync(path.join(world.folder, 'assets', 'hero.png'), mayaPlaceholderBytes);

  await world.server.renameNode(null, 11, 'launch.html');
  await world.server.renameNode(null, 41, 'hero.png');

  await world.d.send('pause', { paused: false });
  await world.d.send('idle', { timeoutMs: 45000 });
  await wait(1500);

  const conflictsAfterOffline = await world.d.send('conflicts');
  const teamHeroLocal = findBytesLocal(world.folder, sha(teamHeroBytes));
  const teamHeroServer = findBytesOnServer(world.server, sha(teamHeroBytes));
  const teamDocLocal = findBytesLocal(world.folder, sha(teamDocHtml));
  const teamDocServer = findBytesOnServer(world.server, sha(teamDocHtml));
  const serverNode41Sha = sha(Buffer.from(world.server.get(41).content));
  const serverNode11Sha = sha(Buffer.from(world.server.get(11).content));

  // --- Case 2: Live SSE rename within 3s delete hold (applyRemotePath in engine-session.js:270-278) ---
  rmSync(path.join(world.folder, 'assets', 'live-draft.png'));
  writeFileSync(path.join(world.folder, 'assets', 'live-final.png'), mayaLiveBytes);
  await world.server.renameNode(null, 42, 'live-final.png');
  world.fake.push({
    type: 'node-renamed',
    nodeId: 42,
    nodeType: 'upload',
    oldPath: 'assets/live-draft.png',
    newPath: 'assets/live-final.png',
  });

  await until('live node 42 overwritten on server', () =>
    sha(Buffer.from(world.server.get(42).content)) === sha(mayaLiveBytes), 10000).catch(() => null);
  await world.d.send('idle', { timeoutMs: 30000 });

  const teamLiveLocal = findBytesLocal(world.folder, sha(teamLiveBytes));
  const teamLiveServer = findBytesOnServer(world.server, sha(teamLiveBytes));
  const serverNode42Sha = sha(Buffer.from(world.server.get(42).content));
  const conflictsAfterLive = await world.d.send('conflicts');

  const offlineHeroLost = teamHeroLocal.length === 0 && teamHeroServer.length === 0 && serverNode41Sha === sha(mayaPlaceholderBytes);
  const offlineDocLost = teamDocLocal.length === 0 && teamDocServer.length === 0 && serverNode11Sha === sha(mayaNotesHtml);
  const liveAssetLost = teamLiveLocal.length === 0 && teamLiveServer.length === 0 && serverNode42Sha === sha(mayaLiveBytes);

  console.log(
    `offline: conflicts=${conflictsAfterOffline.length} heroLost=${offlineHeroLost} ` +
    `(local=[${teamHeroLocal.join(',')}], server=[${teamHeroServer.join(',')}]) ` +
    `docLost=${offlineDocLost} (local=[${teamDocLocal.join(',')}], server=[${teamDocServer.join(',')}])`,
  );
  console.log(
    `live: conflicts=${conflictsAfterLive.length} liveAssetLost=${liveAssetLost} ` +
    `(local=[${teamLiveLocal.join(',')}], server=[${teamLiveServer.join(',')}])`,
  );

  if (offlineHeroLost || offlineDocLost || liveAssetLost) {
    console.log(
      `FAILED remote-rename-onto-unrelated-local-overwrites-server: remote rename to a path occupied by an unrelated local file ` +
      `after the old path was deleted locally adopted the unrelated file with the old baseline and overwrote the server node ` +
      `with 0 conflicts and 0 backups anywhere (offlineHeroLost=${offlineHeroLost}, offlineDocLost=${offlineDocLost}, liveAssetLost=${liveAssetLost})`,
    );
    process.exit(1);
  }

  console.log('OK remote-rename-onto-unrelated-local-overwrites-server: both local and server versions were preserved');
} finally {
  await world.close();
}
