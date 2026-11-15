// Probe: Moving or renaming a synced folder on disk and relinking it via HTML Clay's
// "Find it" (`folder.relink` -> `cmd/htmlclay/folders.go:864-915, 1007-1028`) updates
// `config.json` and remounts the served root at the new path, but never updates
// `rootRealpath` inside `sync-meta/v2/<sessionId>/identity.json` (`first-bind.js:311-318`).
// On restart of the root, `SyncManager.localRefusal` (`sync-manager.js:294-309`) sees
// `identity.rootRealpath !== realpath(newPath)` and permanently pauses the session with
// `identity-mismatch` (`sync-manager.js:828` never resumes it), stranding all subsequent
// local saves from hyperclay.com with no recovery path in the UI.
import { randomBytes } from 'node:crypto';
import { existsSync, mkdirSync, mkdtempSync, readFileSync, realpathSync, renameSync, writeFileSync } from 'node:fs';
import { createRequire } from 'node:module';
import os from 'node:os';
import path from 'node:path';
import { freePort, shellRequest, startShellHost } from '../served-product.mjs';
import { build, page, repo, save, wait } from '../sync-harness.mjs';

function parseOut(argv) {
  const idx = argv.indexOf('--out');
  const dir = idx !== -1 && argv[idx + 1]
    ? path.resolve(argv[idx + 1])
    : mkdtempSync(path.join(os.tmpdir(), 'probe-relink-sync-'));
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

const LOCAL = process.env.HTMLCLAY_SYNC_LOCAL || path.join(repo, '..', 'hyperclay-local');
const require = createRequire(import.meta.url);
const { startFakeSyncHttp } = require(path.join(LOCAL, 'trial', 'fake-sync-http.cjs'));

const out = parseOut(process.argv.slice(2));
const built = build(out);
const key = `hcsk_${randomBytes(32).toString('hex')}`;

const fake = await startFakeSyncHttp({
  nodes: [{ id: 11, type: 'site', name: 'notes.html', parentId: 0, content: page('notes.html', 'INITIAL-V1') }],
  key,
  actor: { id: 12, username: 'maya', person: { id: 12, name: 'Maya' } },
  accounts: [
    { id: 12, kind: 'personal', username: 'maya', displayName: 'maya', role: 'owner', syncBase: '/_/sync', lifecycle: 'active', sync: { enabled: true, reason: null } },
  ],
});

const base = realpathSync(mkdtempSync(path.join(realpathSync(os.homedir()), 'htmlclay-relink-probe-')));
const configBase = path.join(base, 'config');
const configDir = path.join(configBase, 'htmlclay');
mkdirSync(configDir, { recursive: true });
const port = await freePort();
writeFileSync(path.join(configDir, 'config.json'), `${JSON.stringify({ startOnLogin: false, port, updates: 'off', servedFolders: [] }, null, 2)}\n`);

const host = await startShellHost({
  bin: built.bin,
  cwd: configBase,
  log: path.join(out, 'relink-host.log'),
  env: {
    ...process.env,
    HTMLCLAY_CONFIG_BASE: configBase,
    HTMLCLAY_SECRET_STORE: 'file',
    HTMLCLAY_RELEASE_INFO_URL: 'http://127.0.0.1:1/',
    HTMLCLAY_SYNC_SERVER: fake.url,
    HTMLCLAY_SERVICE_CMD: built.serviceCmd,
  },
});

let seq = 0;
async function act(type, args) {
  seq += 1;
  const res = await shellRequest(host, 'POST', '/shell/v1/actions', { id: `relink-${seq}`, type, args });
  const body = res.body ? JSON.parse(res.body) : {};
  if (!body.ok) throw new Error(`${type} refused: ${JSON.stringify(body)}`);
  return body;
}
async function snapshot() {
  const res = await shellRequest(host, 'GET', '/shell/v1/snapshot');
  return JSON.parse(res.body);
}

try {
  await act('sync.connect', { key, username: 'maya' });
  const row = await until('maya account row', async () =>
    (await snapshot()).folders.find((f) => f.origin?.kind === 'account' && f.origin?.username === 'maya' && f.state === 'not-on-this-computer'));

  const oldFolder = path.join(base, 'maya');
  mkdirSync(oldFolder, { recursive: true });
  await act('folder.setUpTeam', { id: row.id, path: oldFolder, existing: false, confirmed: true });

  await until('notes.html landed and up-to-date', async () => {
    if (!existsSync(path.join(oldFolder, 'notes.html'))) return false;
    const snap = await snapshot();
    const f = snap.folders.find((x) => x.id === row.id);
    return f && f.synced && f.sync?.state === 'up-to-date';
  });

  // User moves/renames their synced folder in Finder / Explorer (`maya` -> `Projects/maya`)
  // and clicks "Find it" (`folder.relink`) in HTML Clay:
  const projectsDir = path.join(base, 'Projects');
  mkdirSync(projectsDir, { recursive: true });
  const newFolder = path.join(projectsDir, 'maya');
  renameSync(oldFolder, newFolder);

  await act('folder.relink', { id: row.id, path: newFolder });

  // Immediately on `folder.relink`, `moveRelinkedMount` -> `removeRoot` stops the session
  // and `mountServed` -> `addRoot` calls `sendSyncWanted()`, which runs `SyncManager.start` ->
  // `localRefusal`: because `identity.json` still holds the old `rootRealpath`, the session
  // immediately pauses with `identity-mismatch`!
  const pausedAfterRelink = await until('session wedged in identity-mismatch right after relink', async () => {
    const syncJson = JSON.parse(readFileSync(path.join(configDir, 'sync.json'), 'utf8'));
    return syncJson.syncSessions?.[0]?.paused?.reason || null;
  }, 15000).catch(() => null);

  const relinkedRow = (await snapshot()).folders.find((x) => x.id === row.id);
  const servedUrl1 = new URL(relinkedRow.url);
  const savedAfterRelink = await save({
    port: Number(servedUrl1.port),
    host: servedUrl1.host,
    doc: 'notes.html',
    body: page('notes.html', 'POST-RELINK-EDIT-STRANDED'),
  });

  // Neither toggling folder sync off/on nor restarting HTML Clay can clear `identity-mismatch`:
  await act('folder.setSynced', { id: row.id, on: false });
  await act('folder.setSynced', { id: row.id, on: true });
  await wait(1500);

  await host.stop();
  const host2 = await startShellHost({
    bin: built.bin,
    cwd: configBase,
    log: path.join(out, 'relink-host2.log'),
    env: {
      ...process.env,
      HTMLCLAY_CONFIG_BASE: configBase,
      HTMLCLAY_SECRET_STORE: 'file',
      HTMLCLAY_RELEASE_INFO_URL: 'http://127.0.0.1:1/',
      HTMLCLAY_SYNC_SERVER: fake.url,
      HTMLCLAY_SERVICE_CMD: built.serviceCmd,
    },
  });
  try {
    await wait(2500);
    const syncJsonAfterRestart = JSON.parse(readFileSync(path.join(configDir, 'sync.json'), 'utf8'));
    const pausedAfterRestart = syncJsonAfterRestart.syncSessions?.[0]?.paused?.reason || null;
    const uploadedAfterRelink = fake.server.get(11).content.toString('utf8').includes('POST-RELINK-EDIT-STRANDED');

    if (pausedAfterRelink === 'identity-mismatch' || pausedAfterRestart === 'identity-mismatch' || !uploadedAfterRelink) {
      console.log(
        `FAILED: folder.relink permanently wedged synced folder in identity-mismatch and stranded edits: ` +
        `pausedAfterRelink=${pausedAfterRelink} pausedAfterToggleAndRestart=${pausedAfterRestart} ` +
        `localSaveStatus=${savedAfterRelink.status} uploadedToServer=${uploadedAfterRelink}`
      );
      process.exitCode = 1;
    } else {
      console.log('OK: folder.relink updated sync session rootRealpath and continued syncing');
    }
  } finally {
    await host2.stop().catch(() => {});
  }
} finally {
  await host.stop().catch(() => {});
  await fake.close();
}
