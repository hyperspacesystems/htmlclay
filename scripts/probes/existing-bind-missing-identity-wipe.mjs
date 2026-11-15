import { randomBytes } from 'node:crypto';
import { createRequire } from 'node:module';
import { existsSync, mkdirSync, mkdtempSync, readFileSync, realpathSync, renameSync, rmSync, writeFileSync } from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { freePort, shellRequest, startShellHost } from '../served-product.mjs';
import { build, hex, page, repo, sha, wait, walk } from '../sync-harness.mjs';

const LOCAL = process.env.HTMLCLAY_SYNC_LOCAL || path.join(repo, '..', 'hyperclay-local');
const require = createRequire(import.meta.url);
const { startFakeSyncHttp } = require(path.join(LOCAL, 'trial', 'fake-sync-http.cjs'));

function parseOut(argv) {
  const idx = argv.indexOf('--out');
  const dir = idx !== -1 && argv[idx + 1]
    ? path.resolve(argv[idx + 1])
    : mkdtempSync(path.join(os.tmpdir(), 'probe-existing-identity-'));
  mkdirSync(dir, { recursive: true });
  return dir;
}

async function until(what, check, timeoutMs = 45000) {
  const deadline = Date.now() + timeoutMs;
  for (;;) {
    const value = await check();
    if (value) return value;
    if (Date.now() > deadline) throw new Error(`timed out waiting for ${what}`);
    await wait(200);
  }
}

let actionSeq = 0;
async function shellAct(host, type, args) {
  actionSeq += 1;
  const res = await shellRequest(host, 'POST', '/shell/v1/actions', { id: `probe-${actionSeq}`, type, args });
  const body = res.body ? JSON.parse(res.body) : {};
  if (!body.ok) {
    const err = new Error(`${type} refused: ${body.error?.code ?? res.status}`);
    err.code = body.error?.code ?? `status-${res.status}`;
    throw err;
  }
  return body;
}

async function shellSnap(host) {
  const res = await shellRequest(host, 'GET', '/shell/v1/snapshot');
  if (res.status !== 200) throw new Error(`snapshot status ${res.status}`);
  return JSON.parse(res.body);
}

async function waitSyncSettled(host, timeoutMs = 45000) {
  let quiet = 0;
  return until('sync settled', async () => {
    const snap = await shellSnap(host);
    const synced = snap.folders.filter((f) => f.synced);
    const ok = synced.length > 0 && snap.sync.syncingFiles === 0 &&
      synced.every((f) => ['up-to-date', 'conflicts', 'paused'].includes(f.sync.state));
    quiet = ok ? quiet + 1 : 0;
    return quiet >= 3 ? snap : null;
  }, timeoutMs);
}

function makeSeedNodes(tag) {
  const heroBytes = Buffer.from(`PNG-HERO-${tag}-${hex()}\n`);
  return {
    heroBytes,
    nodes: [
      { id: 11, type: 'site', name: 'alpha.html', parentId: 0, content: page('alpha.html', `ALPHA-${tag}`) },
      { id: 12, type: 'site', name: 'beta.html', parentId: 0, content: page('beta.html', `BETA-${tag}`) },
      { id: 13, type: 'site', name: 'gamma.html', parentId: 0, content: page('gamma.html', `GAMMA-${tag}`) },
      { id: 40, type: 'folder', name: 'assets', parentId: 0 },
      { id: 41, type: 'upload', name: 'hero.png', parentId: 40, content: heroBytes },
    ],
  };
}

function serverFilePaths(server) {
  return [...server.nodes.keys()]
    .filter((id) => server.get(id).type !== 'folder')
    .map((id) => server.relPathOf(id))
    .sort();
}

async function runScenario({ out, built, existing, method }) {
  const label = `${existing ? 'existing' : 'empty'}-${method}`;
  const home = realpathSync(os.homedir());
  const base = realpathSync(mkdtempSync(path.join(home, `htmlclay-idprobe-${label}-`)));
  const configBase = path.join(base, 'config');
  const configDir = path.join(configBase, 'htmlclay');
  const folder = path.join(base, 'maya');
  const movedAway = path.join(base, 'maya-moved-away');
  const wrongFolder = path.join(base, 'other-project');
  const log = path.join(out, `${label}.log`);

  mkdirSync(configDir, { recursive: true });
  const port = await freePort();
  writeFileSync(
    path.join(configDir, 'config.json'),
    `${JSON.stringify({ startOnLogin: false, port, updates: 'off', servedFolders: [] }, null, 2)}\n`,
  );

  const { heroBytes, nodes } = makeSeedNodes(label);
  const key = `hcsk_${randomBytes(32).toString('hex')}`;
  const fake = await startFakeSyncHttp({
    nodes,
    key,
    actor: { id: 12, username: 'maya', person: { id: 12, name: 'Maya' } },
    accounts: [
      { id: 12, kind: 'personal', username: 'maya', displayName: 'maya', role: 'owner', syncBase: '/_/sync', lifecycle: 'active', sync: { enabled: true, reason: null } },
    ],
  });

  const env = {
    ...process.env,
    HTMLCLAY_CONFIG_BASE: configBase,
    HTMLCLAY_SECRET_STORE: 'file',
    HTMLCLAY_RELEASE_INFO_URL: 'http://127.0.0.1:1/',
    HTMLCLAY_SYNC_SERVER: fake.url,
    HTMLCLAY_SERVICE_CMD: built.serviceCmd,
  };

  let host = null;
  try {
    mkdirSync(folder, { recursive: true });
    if (existing) {
      writeFileSync(path.join(folder, 'welcome.html'), page('welcome.html', `WELCOME-${label}`));
    }

    host = await startShellHost({ bin: built.bin, cwd: configBase, env, log });
    await shellAct(host, 'sync.connect', { key, username: '' });
    await until('service running', async () => (await shellSnap(host)).host?.service === 'running');
    const accountRow = await until('maya folder row', async () =>
      (await shellSnap(host)).folders.find((f) => f.origin?.kind === 'account' && f.origin?.username === 'maya' && f.state === 'not-on-this-computer'));

    await shellAct(host, 'folder.setUpTeam', {
      id: accountRow.id,
      path: folder,
      existing,
      confirmed: true,
    });

    await until('all seeded files synced', () => {
      const hasSites = ['alpha.html', 'beta.html', 'gamma.html'].every((n) => existsSync(path.join(folder, n)));
      const heroPath = path.join(folder, 'assets', 'hero.png');
      const hasHero = existsSync(heroPath) && sha(readFileSync(heroPath)) === sha(heroBytes);
      const hasWelcome = !existing || fake.server.hasPath('welcome.html');
      return hasSites && hasHero && hasWelcome;
    });
    await waitSyncSettled(host);

    const syncJson = JSON.parse(readFileSync(path.join(configDir, 'sync.json'), 'utf8'));
    const sessionId = syncJson.syncSessions[0].id;
    const identityPath = path.join(configDir, 'service', 'sync-meta', 'v2', sessionId, 'identity.json');
    const identityExists = existsSync(identityPath);
    const beforeFiles = serverFilePaths(fake.server);

    await host.stop();
    host = null;

    if (method === 'relink') {
      renameSync(folder, movedAway);
      mkdirSync(wrongFolder, { recursive: true });
      writeFileSync(path.join(wrongFolder, 'draft.html'), page('draft.html', `UNRELATED-DRAFT-${label}`));

      host = await startShellHost({ bin: built.bin, cwd: configBase, env, log });
      await until('folder reported missing', async () =>
        (await shellSnap(host)).folders.find((f) => f.id === accountRow.id)?.state === 'missing');

      await shellAct(host, 'folder.relink', {
        id: accountRow.id,
        path: wrongFolder,
        acknowledged: ['different-folder'],
      });
    } else {
      for (const name of ['alpha.html', 'beta.html', 'gamma.html', 'welcome.html']) {
        rmSync(path.join(folder, name), { force: true });
      }
      rmSync(path.join(folder, 'assets'), { recursive: true, force: true });
      rmSync(path.join(folder, '.hyperclay', 'sync-root.json'), { force: true });
      writeFileSync(path.join(folder, 'placeholder.html'), page('placeholder.html', `PLACEHOLDER-${label}`));

      host = await startShellHost({ bin: built.bin, cwd: configBase, env, log });
    }

    await wait(5000);
    const snapAfter = await waitSyncSettled(host);
    const folderSnap = snapAfter.folders.find((f) => f.id === accountRow.id);
    const afterFiles = serverFilePaths(fake.server);
    const deletes = fake.server.calls.filter((c) => c.name === 'deleteNode').map((c) => c.pathBefore);
    const activeDir = method === 'relink' ? wrongFolder : folder;
    const localCopies = existsSync(activeDir) ? walk(activeDir) : [];
    const heroAnywhereLocal = localCopies.some((f) => sha(readFileSync(f)) === sha(heroBytes));
    const heroInServerTrash = [...fake.server.trash.values()].some((n) => n.content && sha(Buffer.from(n.content)) === sha(heroBytes));

    return {
      label,
      identityExists,
      syncState: folderSnap?.sync?.state ?? 'unknown',
      beforeFiles,
      afterFiles,
      deletes,
      heroStillOnServer: fake.server.hasPath('assets/hero.png'),
      heroAnywhereLocal,
      heroInServerTrash,
    };
  } finally {
    if (host) await host.stop().catch(() => {});
    await fake.close().catch(() => {});
    rmSync(base, { recursive: true, force: true });
  }
}

const out = parseOut(process.argv.slice(2));
const built = build(out);

const controlRelink = await runScenario({ out, built, existing: false, method: 'relink' });
const bugRelink = await runScenario({ out, built, existing: true, method: 'relink' });
const controlEvict = await runScenario({ out, built, existing: false, method: 'evict' });
const bugEvict = await runScenario({ out, built, existing: true, method: 'evict' });

for (const r of [controlRelink, bugRelink, controlEvict, bugEvict]) {
  console.log(
    `${r.label}: identity.json=${r.identityExists} syncState=${r.syncState} ` +
    `before=[${r.beforeFiles.join(', ')}] after=[${r.afterFiles.join(', ')}] ` +
    `deletes=[${r.deletes.join(', ')}] heroOnServer=${r.heroStillOnServer} ` +
    `heroLocal=${r.heroAnywhereLocal} heroServerTrash=${r.heroInServerTrash}`,
  );
}

const relinkWiped = !bugRelink.identityExists && bugRelink.deletes.length > 0 && !bugRelink.heroStillOnServer;
const evictWiped = !bugEvict.identityExists && bugEvict.deletes.length > 0 && !bugEvict.heroStillOnServer;

if (relinkWiped || evictWiped) {
  console.log(
    `FAILED existing-bind-missing-identity-wipe: mode="existing" never writes identity.json, ` +
    `so after restart rootMarkerRequired resets to false; ` +
    `relink deleted ${bugRelink.deletes.length}/${bugRelink.beforeFiles.length} server files (${bugRelink.deletes.join(', ')}) ` +
    `while empty-folder control paused (${controlRelink.syncState}, ${controlRelink.deletes.length} deletes); ` +
    `in-place eviction deleted ${bugEvict.deletes.length}/${bugEvict.beforeFiles.length} server files (${bugEvict.deletes.join(', ')}) ` +
    `while empty-folder control paused (${controlEvict.syncState}, ${controlEvict.deletes.length} deletes)`,
  );
  process.exit(1);
}

console.log('OK existing-bind-missing-identity-wipe: existing-folder binds write identity.json and pause on replaced/evicted roots');
