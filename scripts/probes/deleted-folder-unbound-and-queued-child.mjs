import { randomBytes } from 'node:crypto';
import { createRequire } from 'node:module';
import { existsSync, mkdirSync, mkdtempSync, readFileSync, realpathSync, rmSync, writeFileSync } from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { build, converge, driver, hex, page, repo, sha, wait, walk } from '../sync-harness.mjs';

const LOCAL = process.env.HTMLCLAY_SYNC_LOCAL || path.join(repo, '..', 'hyperclay-local');
const require = createRequire(import.meta.url);
const { startFakeSyncHttp } = require(path.join(LOCAL, 'trial', 'fake-sync-http.cjs'));

function parseOut(argv) {
  const idx = argv.indexOf('--out');
  const dir = idx !== -1 && argv[idx + 1]
    ? path.resolve(argv[idx + 1])
    : mkdtempSync(path.join(os.tmpdir(), 'probe-deleted-folder-unbound-'));
  mkdirSync(dir, { recursive: true });
  return dir;
}

async function until(what, check, timeoutMs = 45000) {
  const deadline = Date.now() + timeoutMs;
  for (;;) {
    const value = await check();
    if (value) return value;
    if (Date.now() > deadline) throw new Error(`timed out waiting for ${what}`);
    await wait(150);
  }
}

const out = parseOut(process.argv.slice(2));
const built = build(out);
const key = `hcsk_${randomBytes(32).toString('hex')}`;

const serverSpec = page('spec.html', `SERVER-SPEC-${hex()}`);
const mayaSpec = page('spec.html', `MAYA-SPEC-${hex()}`);
const serverRoadmap = page('roadmap.html', `SERVER-ROADMAP-${hex()}`);
const mayaRoadmap = page('roadmap.html', `MAYA-ROADMAP-${hex()}`);
const freshDoc = page('fresh.html', `MAYA-FRESH-DOC-${hex()}`);

const fake = await startFakeSyncHttp({
  nodes: [
    { id: 10, type: 'site', name: 'root.html', parentId: 0, content: page('root.html', 'ROOT') },
    { id: 40, type: 'folder', name: 'projects', parentId: 0 },
    { id: 41, type: 'site', name: 'spec.html', parentId: 40, content: serverSpec },
    { id: 42, type: 'site', name: 'roadmap.html', parentId: 40, content: serverRoadmap },
    { id: 50, type: 'folder', name: 'notes', parentId: 0 },
    { id: 51, type: 'site', name: 'old.html', parentId: 50, content: page('old.html', 'OLD-NOTE') },
  ],
  key,
  actor: { id: 12, username: 'maya', person: { id: 12, name: 'Maya' } },
  accounts: [
    { id: 12, kind: 'personal', username: 'maya', displayName: 'maya', role: 'owner', syncBase: '/_/sync', lifecycle: 'active', sync: { enabled: true, reason: null } },
  ],
});

const home = realpathSync(os.homedir());
const base = realpathSync(mkdtempSync(path.join(home, 'htmlclay-unbound-del-')));
const folder = path.join(base, 'maya');
mkdirSync(path.join(folder, 'projects'), { recursive: true });
writeFileSync(path.join(folder, 'projects', 'spec.html'), mayaSpec);
writeFileSync(path.join(folder, 'projects', 'roadmap.html'), mayaRoadmap);

const d = driver();
await d.ready;

try {
  await d.send('start', {
    bin: built.bin,
    configBase: path.join(base, 'config'),
    log: path.join(out, 'host.log'),
    syncServer: fake.url,
    serviceCmd: built.serviceCmd,
  });
  await d.send('connect', { key });
  await d.send('setUp', { account: 'maya', folder, existing: true });

  await until('root.html and notes/old.html landed', () =>
    existsSync(path.join(folder, 'root.html')) && existsSync(path.join(folder, 'notes', 'old.html')));
  const specConflict = await until('unbound conflict on projects/spec.html', async () =>
    (await d.send('conflicts')).find((c) => c.path === 'projects/spec.html' && c.kind === 'unbound'));
  const roadmapConflict = await until('unbound conflict on projects/roadmap.html', async () =>
    (await d.send('conflicts')).find((c) => c.path === 'projects/roadmap.html' && c.kind === 'unbound'));
  await d.send('idle', { timeoutMs: 30000 });

  // --- Case A: Teammate deletes folder `projects` (cascade-deleting nodes 41 and 42) online ---
  await fake.server.deleteNode(null, 40, { cascade: true });
  fake.push({ type: 'node-deleted', nodeId: 40, nodeType: 'folder', path: 'projects' });
  await wait(2500);
  await d.send('idle', { timeoutMs: 30000 });

  // 1) Maya clicks Keep mine on projects/spec.html -> resolveConflict calls putNodeContent(41) -> 404 -> refuses with failed!
  let keepMineError = null;
  try {
    await d.send('resolve', { conflict: specConflict, keep: 'mine' });
  } catch (err) {
    keepMineError = err.code || err.message;
  }
  const specUploadedAfterKeepMine = fake.server.hasPath('projects/spec.html');

  // 2) Maya clicks Keep both on projects/roadmap.html -> writes serverRoadmap to projects/roadmap.html bound to dead node 42,
  //    then the next comparison sees node 42 deleted on server with local == baseline and TRASHES projects/roadmap.html!
  let keepBothError = null;
  try {
    await d.send('resolve', { conflict: roadmapConflict, keep: 'both' });
  } catch (err) {
    keepBothError = err.code || err.message;
  }
  await d.send('restartService', { timeoutMs: 45000 });
  await d.send('idle', { timeoutMs: 45000 });
  await wait(2000);

  const roadmapTheirsStillInFolder = existsSync(path.join(folder, 'projects', 'roadmap.html')) &&
    sha(readFileSync(path.join(folder, 'projects', 'roadmap.html'))) === sha(serverRoadmap);
  const roadmapTheirsOnServer = [...fake.server.nodes.values()].some((n) =>
    n.content && sha(Buffer.from(n.content)) === sha(serverRoadmap));

  // --- Case B: New child file queued in notes/ right before teammate deletes notes/ online ---
  writeFileSync(path.join(folder, 'notes', 'fresh.html'), freshDoc);
  // Structure job runs after QUIET_MS (500ms) and queues 'add' on syncQueue (500ms timer -> 1000ms):
  await wait(700);
  if (!fake.server.hasPath('notes/fresh.html')) {
    await fake.server.deleteNode(null, 50, { cascade: true });
    fake.push({ type: 'node-deleted', nodeId: 50, nodeType: 'folder', path: 'notes' });
  }
  await wait(8000);
  await d.send('idle', { timeoutMs: 30000 });
  const freshExistsLocally = existsSync(path.join(folder, 'notes', 'fresh.html'));
  const freshUploadedToServer = fake.server.hasPath('notes/fresh.html');
  const hostLog = readFileSync(path.join(out, 'host.log'), 'utf8');
  const missingParentLogged = hostLog.includes('Target folder not tracked in nodeMap: notes');

  console.log(
    `Case A: keepMineError=${keepMineError} specUploadedAfterKeepMine=${specUploadedAfterKeepMine} ` +
    `keepBothError=${keepBothError} roadmapTheirsStillInFolder=${roadmapTheirsStillInFolder} roadmapTheirsOnServer=${roadmapTheirsOnServer}`,
  );
  console.log(
    `Case B: freshExistsLocally=${freshExistsLocally} freshUploadedToServer=${freshUploadedToServer} missingParentLogged=${missingParentLogged}`,
  );

  const caseAFailed = Boolean(keepMineError) || !specUploadedAfterKeepMine || !roadmapTheirsStillInFolder || !roadmapTheirsOnServer;
  const caseBFailed = freshExistsLocally && !freshUploadedToServer;

  if (caseAFailed || caseBFailed) {
    console.log(
      `FAILED deleted-folder-unbound-and-queued-child: after online folder delete, ` +
      `unbound Keep mine failed (${keepMineError}, uploaded=${specUploadedAfterKeepMine}), ` +
      `unbound Keep both self-trashed theirs (inFolder=${roadmapTheirsStillInFolder}, onServer=${roadmapTheirsOnServer}), ` +
      `and new child notes/fresh.html stranded unsynced=${caseBFailed} (missingParentLogged=${missingParentLogged})`,
    );
    process.exit(1);
  }

  console.log('OK deleted-folder-unbound-and-queued-child: unbound conflicts and queued child uploads survived online folder delete');
} finally {
  await d.send('stop').catch(() => {});
  d.child.disconnect();
  await fake.close().catch(() => {});
  rmSync(base, { recursive: true, force: true });
}
