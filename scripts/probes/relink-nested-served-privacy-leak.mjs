import { randomBytes } from 'node:crypto';
import http from 'node:http';
import { createRequire } from 'node:module';
import { existsSync, mkdirSync, mkdtempSync, readFileSync, realpathSync, renameSync, rmSync, writeFileSync } from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { freePort, shellRequest, startShellHost } from '../served-product.mjs';
import { build, page, repo, save, sha, wait } from '../sync-harness.mjs';

const LOCAL = process.env.HTMLCLAY_SYNC_LOCAL || path.join(repo, '..', 'hyperclay-local');
const require = createRequire(import.meta.url);
const { startFakeSyncHttp } = require(path.join(LOCAL, 'trial', 'fake-sync-http.cjs'));

function parseOut(argv) {
  const idx = argv.indexOf('--out');
  const dir = idx !== -1 && argv[idx + 1]
    ? path.resolve(argv[idx + 1])
    : mkdtempSync(path.join(os.tmpdir(), 'probe-relink-nested-leak-'));
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

async function shellCheck(host, type, args) {
  const res = await shellRequest(host, 'POST', '/shell/v1/check', { id: 'check-1', type, args });
  return res.body ? JSON.parse(res.body) : {};
}

async function shellSnap(host) {
  const res = await shellRequest(host, 'GET', '/shell/v1/snapshot');
  if (res.status !== 200) throw new Error(`snapshot status ${res.status}`);
  return JSON.parse(res.body);
}

const out = parseOut(process.argv.slice(2));
const built = build(out);
const key = `hcsk_${randomBytes(32).toString('hex')}`;
const passportBytes = Buffer.from('PRIVATE-PASSPORT-SCAN-DO-NOT-SHARE\n');

const fakePersonal = await startFakeSyncHttp({
  nodes: [
    { id: 11, type: 'site', name: 'diary.html', parentId: 0, content: page('diary.html', 'PRIVATE-DIARY-SECRET') },
    { id: 20, type: 'folder', name: 'uploads', parentId: 0 },
    { id: 21, type: 'upload', name: 'passport.png', parentId: 20, content: passportBytes },
  ],
  key,
  actor: { id: 12, username: 'maya', person: { id: 12, name: 'Maya' } },
  accounts: [
    { id: 12, kind: 'personal', username: 'maya', displayName: 'maya', role: 'owner', syncBase: '/_/sync', lifecycle: 'active', sync: { enabled: true, reason: null } },
  ],
});

const fakeTeam = await startFakeSyncHttp({
  nodes: [
    { id: 101, type: 'site', name: 'roadmap.html', parentId: 0, content: page('roadmap.html', 'SHARED-TEAM-ROADMAP') },
  ],
  key,
  actor: { id: 12, username: 'maya', person: { id: 12, name: 'Maya' } },
  accounts: [
    { id: 77, kind: 'team', username: 'acme', displayName: 'Acme Team', role: 'admin', syncBase: '/_/sync', lifecycle: 'active', sync: { enabled: true, reason: null } },
  ],
});

const router = http.createServer((req, res) => {
  if (req.url === '/_/sync/accounts' && req.method === 'GET') {
    if (req.headers['x-api-key'] !== key) {
      res.writeHead(401, { 'Content-Type': 'application/json' });
      res.end(JSON.stringify({ code: 'key-refused', error: 'Invalid API key' }));
      return;
    }
    res.writeHead(200, { 'Content-Type': 'application/json' });
    res.end(JSON.stringify({
      protocol: 2,
      actor: { id: 12, username: 'maya', person: { id: 12, name: 'Maya' } },
      features: { accountScopes: true, accountEvents: true, conditionalContent: true, conditionalStructure: true, completeInventory: true },
      accounts: [
        { id: 12, kind: 'personal', username: 'maya', displayName: 'maya', role: 'owner', syncBase: '/_/sync', lifecycle: 'active', sync: { enabled: true, reason: null } },
        { id: 77, kind: 'team', username: 'acme', displayName: 'Acme Team', role: 'admin', syncBase: '/_/teams/acme/sync', lifecycle: 'active', sync: { enabled: true, reason: null } },
      ],
    }));
    return;
  }

  const isTeam = req.url.startsWith('/_/teams/acme/sync') || req.headers['x-sync-account-id'] === '77';
  const rewrittenUrl = req.url.startsWith('/_/teams/acme/sync')
    ? `/_/sync${req.url.slice('/_/teams/acme/sync'.length)}`
    : req.url;
  const target = new URL(rewrittenUrl, isTeam ? fakeTeam.url : fakePersonal.url);
  const upstream = http.request(target, { method: req.method, headers: req.headers }, (upRes) => {
    res.writeHead(upRes.statusCode, upRes.headers);
    upRes.pipe(res);
  });
  upstream.on('error', () => res.destroy());
  res.on('close', () => upstream.destroy());
  req.pipe(upstream);
});

await new Promise((resolve) => router.listen(0, '127.0.0.1', resolve));
const routerUrl = `http://127.0.0.1:${router.address().port}`;

const home = realpathSync(os.homedir());
const base = realpathSync(mkdtempSync(path.join(home, 'htmlclay-nested-leak-')));
const configBase = path.join(base, 'config');
const configDir = path.join(configBase, 'htmlclay');
const teamFolder = path.join(base, 'acme');
const personalFolder = path.join(base, 'personal');
const nestedPersonal = path.join(teamFolder, 'personal');

mkdirSync(configDir, { recursive: true });
const port = await freePort();
writeFileSync(
  path.join(configDir, 'config.json'),
  `${JSON.stringify({ startOnLogin: false, port, updates: 'off', servedFolders: [] }, null, 2)}\n`,
);

let host = null;
try {
  host = await startShellHost({
    bin: built.bin,
    cwd: configBase,
    env: {
      ...process.env,
      HTMLCLAY_CONFIG_BASE: configBase,
      HTMLCLAY_SECRET_STORE: 'file',
      HTMLCLAY_RELEASE_INFO_URL: 'http://127.0.0.1:1/',
      HTMLCLAY_SYNC_SERVER: routerUrl,
      HTMLCLAY_SERVICE_CMD: built.serviceCmd,
    },
    log: path.join(out, 'host.log'),
  });

  await shellAct(host, 'sync.connect', { key, username: '' });
  await until('service running', async () => (await shellSnap(host)).host?.service === 'running');
  const snap0 = await shellSnap(host);
  const teamRow = snap0.folders.find((f) => f.origin?.kind === 'team' && f.origin?.team === 'acme');
  const personalRow = snap0.folders.find((f) => f.origin?.kind === 'account' && f.origin?.username === 'maya');

  mkdirSync(teamFolder, { recursive: true });
  await shellAct(host, 'folder.setUpTeam', { id: teamRow.id, path: teamFolder, existing: false, confirmed: true });
  await until('roadmap.html landed in team folder', () => existsSync(path.join(teamFolder, 'roadmap.html')));

  // Verify direct setup inside teamFolder is refused with overlaps-served:
  let directNestedRefusal = null;
  mkdirSync(path.join(teamFolder, 'try-direct'), { recursive: true });
  try {
    await shellAct(host, 'folder.setUpTeam', { id: personalRow.id, path: path.join(teamFolder, 'try-direct'), existing: false, confirmed: true });
  } catch (err) {
    directNestedRefusal = err.code;
  }
  rmSync(path.join(teamFolder, 'try-direct'), { recursive: true, force: true });

  mkdirSync(personalFolder, { recursive: true });
  await shellAct(host, 'folder.setUpTeam', { id: personalRow.id, path: personalFolder, existing: false, confirmed: true });
  await until('personal files landed', () =>
    existsSync(path.join(personalFolder, 'diary.html')) &&
    existsSync(path.join(personalFolder, 'uploads', 'passport.png')));

  // User moves ~/personal into ~/acme/personal and clicks "Find it" (folder.relink) with ZERO warnings acknowledged:
  renameSync(personalFolder, nestedPersonal);
  const checkAnswer = await shellCheck(host, 'folder.relink', { id: personalRow.id, path: nestedPersonal });
  let relinkRefusal = null;
  try {
    await shellAct(host, 'folder.relink', { id: personalRow.id, path: nestedPersonal, acknowledged: [] });
  } catch (err) {
    relinkRefusal = err.code;
  }

  let diaryLeakedToTeam = false;
  let passportLeakedToTeam = false;
  let liveEditLeakedToTeam = false;
  let teamCleanupDeletedPersonalAccountFile = false;

  if (!relinkRefusal) {
    diaryLeakedToTeam = await until('private diary.html uploaded to shared @acme team server', () => {
      for (const [, node] of fakeTeam.server.nodes) {
        if (node.type === 'site' && fakeTeam.server.relPathOf(node.id) === 'personal/diary.html') {
          return node.content.toString('utf8').includes('PRIVATE-DIARY-SECRET');
        }
      }
      return false;
    }, 20000).catch(() => false);

    passportLeakedToTeam = await until('private passport.png uploaded to shared @acme team server', () => {
      for (const [, node] of fakeTeam.server.nodes) {
        if (node.type === 'upload' && fakeTeam.server.relPathOf(node.id) === 'personal/uploads/passport.png') {
          return sha(Buffer.from(node.content)) === sha(passportBytes);
        }
      }
      return false;
    }, 20000).catch(() => false);

    // Save a new secret in personal's served URL and verify it leaks to @acme too:
    const snapAfter = await shellSnap(host);
    const personalServed = new URL(snapAfter.folders.find((f) => f.id === personalRow.id).url);
    await save({
      port: Number(personalServed.port),
      host: personalServed.host,
      doc: 'diary.html',
      body: page('diary.html', 'PRIVATE-DIARY-SECOND-SECRET'),
    });

    liveEditLeakedToTeam = await until('live personal save uploaded to @acme team server', () => {
      for (const [, node] of fakeTeam.server.nodes) {
        if (fakeTeam.server.relPathOf(node.id) === 'personal/diary.html') {
          return node.content.toString('utf8').includes('PRIVATE-DIARY-SECOND-SECRET');
        }
      }
      return false;
    }, 15000).catch(() => false);

    // Teammate deletes leaked personal/diary.html from @acme online -> propagates back and deletes diary.html from Maya's personal account!
    const teamDiaryEntry = [...fakeTeam.server.nodes.values()].find((n) => fakeTeam.server.relPathOf(n.id) === 'personal/diary.html');
    if (teamDiaryEntry) {
      await fakeTeam.server.deleteNode(null, teamDiaryEntry.id);
      fakeTeam.push({ type: 'node-deleted', nodeId: teamDiaryEntry.id, nodeType: 'site', path: 'personal/diary.html' });
      teamCleanupDeletedPersonalAccountFile = await until(
        'teammate cleanup in @acme deleted diary.html from Maya personal account',
        () => !fakePersonal.server.hasPath('diary.html'),
        15000,
      ).catch(() => false);
    }
  }

  console.log(
    `directSetUpRefusal=${directNestedRefusal} relinkPreviewWarnings=${JSON.stringify(checkAnswer.preview?.warnings ?? [])} ` +
    `relinkRefusal=${relinkRefusal} diaryLeakedToTeam=${diaryLeakedToTeam} passportLeakedToTeam=${passportLeakedToTeam} ` +
    `liveEditLeakedToTeam=${liveEditLeakedToTeam} teamCleanupDeletedPersonalAccountFile=${teamCleanupDeletedPersonalAccountFile}`,
  );

  if (!relinkRefusal && (diaryLeakedToTeam || passportLeakedToTeam)) {
    console.log(
      `FAILED relink-nested-served-privacy-leak: folder.relink allowed nesting personal synced folder inside team synced folder ` +
      `with 0 warnings (direct setup refused with ${directNestedRefusal}); private diary.html (${diaryLeakedToTeam}) and ` +
      `uploads/passport.png (${passportLeakedToTeam}) leaked to shared @acme team server, live saves leaked=${liveEditLeakedToTeam}, ` +
      `and teammate deleting leaked file from @acme deleted diary.html from Maya's personal account=${teamCleanupDeletedPersonalAccountFile}`,
    );
    process.exit(1);
  }

  console.log('OK relink-nested-served-privacy-leak: folder.relink refused overlapping served/synced folders');
} finally {
  if (host) await host.stop().catch(() => {});
  await new Promise((resolve) => router.close(resolve));
  await fakePersonal.close().catch(() => {});
  await fakeTeam.close().catch(() => {});
  rmSync(base, { recursive: true, force: true });
}
