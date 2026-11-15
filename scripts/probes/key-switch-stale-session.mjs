// Probe: Connecting a new API key while already connected (`cmd/htmlclay/sync.go:259-279`,
// `service/sync.cjs:340-346`) neither disables/stops sessions belonging to a previous
// `ActorID` nor calls `manager.adoptKey()` on running `SyncEngine` instances:
//   1) Cross-account security/privacy leak: when `@alex` connects his API key on a
//      machine where `@maya` was connected, HTML Clay shows `connected: true, username: "alex"`,
//      yet Maya's SyncEngine stays live in memory holding Maya's API key — uploading local
//      edits to Maya's hyperclay.com account and downloading Maya's new private files!
//   2) Broken key rotation for the same account: when `@maya` connects a new key `keyMayaNew`
//      and then revokes her old `keyMayaOld` on hyperclay.com, the running engine still sends
//      `keyMayaOld` (401 -> local saves fail to upload), and when hyperclay.com ends the
//      revoked key's SSE stream with `account-changed: key-revoked`, Go marks the brand-new
//      valid `keyMayaNew` as `syncKeyRefused` and stops sync!
import { randomBytes } from 'node:crypto';
import http from 'node:http';
import { createRequire } from 'node:module';
import { existsSync, mkdirSync, mkdtempSync, readFileSync, realpathSync } from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { build, driver, page, repo, save, wait } from '../sync-harness.mjs';

function parseOut(argv) {
  const idx = argv.indexOf('--out');
  const dir = idx !== -1 && argv[idx + 1]
    ? path.resolve(argv[idx + 1])
    : mkdtempSync(path.join(os.tmpdir(), 'probe-key-switch-'));
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

// --- Scenario 1: Same user (@maya) rotates API key, then revokes old key ---
const keyMayaOld = `hcsk_${randomBytes(32).toString('hex')}`;
const keyMayaNew = `hcsk_${randomBytes(32).toString('hex')}`;
const fake1 = await startFakeSyncHttp({
  nodes: [{ id: 11, type: 'site', name: 'secret.html', parentId: 0, content: page('secret.html', 'MAYA-V1') }],
  key: keyMayaOld,
  actor: { id: 12, username: 'maya', person: { id: 12, name: 'Maya' } },
  accounts: [
    { id: 12, kind: 'personal', username: 'maya', displayName: 'maya', role: 'owner', syncBase: '/_/sync', lifecycle: 'active', sync: { enabled: true, reason: null } },
  ],
});

let revokeOldMayaKey = false;
let oldKeyUploadRejected401 = false;
const oldMayaStreams = new Set();

const proxy1 = http.createServer((req, res) => {
  const apiKey = req.headers['x-api-key'];
  if (apiKey === keyMayaOld && revokeOldMayaKey) {
    if (req.method === 'PUT') oldKeyUploadRejected401 = true;
    res.writeHead(401, { 'Content-Type': 'application/json' });
    res.end(JSON.stringify({ code: 'key-revoked', error: 'API key revoked' }));
    return;
  }
  const headers = { ...req.headers };
  if (apiKey === keyMayaNew) headers['x-api-key'] = keyMayaOld;

  const target = new URL(req.url, fake1.url);
  const upstream = http.request(target, { method: req.method, headers }, (upRes) => {
    res.writeHead(upRes.statusCode, upRes.headers);
    if (req.url.startsWith('/_/sync/stream') && apiKey === keyMayaOld) {
      oldMayaStreams.add(res);
      res.on('close', () => oldMayaStreams.delete(res));
    }
    upRes.pipe(res);
  });
  upstream.on('error', () => {});
  res.on('close', () => upstream.destroy());
  req.pipe(upstream);
});
await new Promise((resolve) => proxy1.listen(0, '127.0.0.1', resolve));
const proxy1Url = `http://127.0.0.1:${proxy1.address().port}`;

const base1 = realpathSync(mkdtempSync(path.join(realpathSync(os.homedir()), '.htmlclay-key-rot-')));
const folder1 = path.join(base1, 'maya');
const d1 = driver();
await d1.ready;

let rotatedUploadDropped = false;
let validRotatedKeyFalselyRefused = false;

try {
  await d1.send('start', { bin: built.bin, configBase: path.join(base1, 'config'), log: path.join(out, 'host1.log'), syncServer: proxy1Url, serviceCmd: built.serviceCmd });
  await d1.send('connect', { key: keyMayaOld });
  await d1.send('setUp', { account: 'maya', folder: folder1 });
  await until('secret.html landed', async () => 'secret.html' in (await d1.send('checksums', { folder: folder1 })));
  await d1.send('idle', { timeoutMs: 30000 });

  // Maya connects her new valid key BEFORE revoking her old key on hyperclay.com:
  await d1.send('connect', { key: keyMayaNew, username: 'maya' });
  revokeOldMayaKey = true;

  const row1 = (await d1.send('snapshot')).folders.find((f) => f.fullPath === folder1 || f.path === folder1);
  const served1 = new URL(row1.url);
  await save({ port: Number(served1.port), host: served1.host, doc: 'secret.html', body: page('secret.html', 'MAYA-AFTER-ROTATE') });

  await until('running engine still sent revoked old key on PUT', () => oldKeyUploadRejected401, 10000).catch(() => null);
  rotatedUploadDropped = oldKeyUploadRejected401 && !fake1.server.get(11).content.toString('utf8').includes('MAYA-AFTER-ROTATE');

  // hyperclay.com also ends revoked key streams with account-changed { reason: 'key-revoked' }:
  for (const streamRes of [...oldMayaStreams]) {
    streamRes.write('data: {"type":"account-changed","reason":"key-revoked"}\n\n');
    streamRes.end();
  }

  validRotatedKeyFalselyRefused = await until('new valid key falsely marked syncKeyRefused', async () => {
    const snap = await d1.send('snapshot');
    return snap.waiting?.some((w) => w.kind === 'syncKeyRefused') || false;
  }, 10000).catch(() => false);
} finally {
  await d1.send('stop').catch(() => {});
  d1.child.disconnect();
  await new Promise((resolve) => proxy1.close(resolve));
  await fake1.close();
}

// --- Scenario 2: Second user (@alex) connects his key without disconnecting @maya ---
const keyMaya2 = `hcsk_${randomBytes(32).toString('hex')}`;
const keyAlex = `hcsk_${randomBytes(32).toString('hex')}`;
const fake2 = await startFakeSyncHttp({
  nodes: [{ id: 11, type: 'site', name: 'secret.html', parentId: 0, content: page('secret.html', 'MAYA-INITIAL') }],
  key: keyMaya2,
  actor: { id: 12, username: 'maya', person: { id: 12, name: 'Maya' } },
  accounts: [
    { id: 12, kind: 'personal', username: 'maya', displayName: 'maya', role: 'owner', syncBase: '/_/sync', lifecycle: 'active', sync: { enabled: true, reason: null } },
  ],
});

let alexConnected = false;
const seenMayaKeyRequestsWhileAlexSignedIn = [];

const proxy2 = http.createServer((req, res) => {
  const apiKey = req.headers['x-api-key'];
  if (alexConnected && apiKey === keyMaya2) {
    seenMayaKeyRequestsWhileAlexSignedIn.push({ method: req.method, url: req.url });
  }
  if (req.url === '/_/sync/accounts' && req.method === 'GET' && apiKey === keyAlex) {
    res.writeHead(200, { 'Content-Type': 'application/json' });
    res.end(JSON.stringify({
      protocol: 2,
      actor: { id: 99, username: 'alex', person: { id: 99, name: 'Alex' } },
      features: { accountScopes: true, accountEvents: true, conditionalContent: true, conditionalStructure: true, completeInventory: true },
      accounts: [
        { id: 99, kind: 'personal', username: 'alex', displayName: 'alex', role: 'owner', syncBase: '/_/sync', lifecycle: 'active', sync: { enabled: true, reason: null } },
      ],
    }));
    return;
  }
  const target = new URL(req.url, fake2.url);
  const upstream = http.request(target, { method: req.method, headers: req.headers }, (upRes) => {
    res.writeHead(upRes.statusCode, upRes.headers);
    upRes.pipe(res);
  });
  upstream.on('error', () => {});
  res.on('close', () => upstream.destroy());
  req.pipe(upstream);
});
await new Promise((resolve) => proxy2.listen(0, '127.0.0.1', resolve));
const proxy2Url = `http://127.0.0.1:${proxy2.address().port}`;

const base2 = realpathSync(mkdtempSync(path.join(realpathSync(os.homedir()), '.htmlclay-key-leak-')));
const folder2 = path.join(base2, 'maya');
const d2 = driver();
await d2.ready;

let signedInUser = null;
let sentStaleMayaKeyAfterAlexConnect = false;
let uploadedToMayaAccountWhileAlexConnected = false;
let downloadedMayaPrivateFileWhileAlexConnected = false;

try {
  await d2.send('start', { bin: built.bin, configBase: path.join(base2, 'config'), log: path.join(out, 'host2.log'), syncServer: proxy2Url, serviceCmd: built.serviceCmd });
  await d2.send('connect', { key: keyMaya2 });
  await d2.send('setUp', { account: 'maya', folder: folder2 });
  await until('secret.html landed', async () => 'secret.html' in (await d2.send('checksums', { folder: folder2 })));
  await d2.send('idle', { timeoutMs: 30000 });

  // Alex connects his API key directly without clicking Disconnect first:
  const connectedAsAlex = await d2.send('connect', { key: keyAlex });
  signedInUser = connectedAsAlex.username;
  alexConnected = true;

  // Local edit inside maya2 while HTML Clay says Connected as @alex:
  const row2 = (await d2.send('snapshot')).folders.find((f) => f.fullPath === folder2 || f.path === folder2);
  const served2 = new URL(row2.url);
  await save({ port: Number(served2.port), host: served2.host, doc: 'secret.html', body: page('secret.html', 'LEAKED-WHILE-ALEX-CONNECTED') });

  uploadedToMayaAccountWhileAlexConnected = await until('Maya server overwritten while Alex signed in', () =>
    fake2.server.get(11).content.toString('utf8').includes('LEAKED-WHILE-ALEX-CONNECTED'), 10000).catch(() => false);
  sentStaleMayaKeyAfterAlexConnect = seenMayaKeyRequestsWhileAlexSignedIn.length > 0;

  // And Maya creating a private file online streams straight down while signed in as @alex:
  await fake2.server.createNode(null, { type: 'site', name: 'maya-private.html', parentId: 0, content: page('maya-private.html', 'MAYA-PRIVATE-DOC') });
  downloadedMayaPrivateFileWhileAlexConnected = await until('Maya private file downloaded while Alex signed in', () => {
    const p = path.join(folder2, 'maya-private.html');
    return existsSync(p) && readFileSync(p, 'utf8').includes('MAYA-PRIVATE-DOC');
  }, 10000).catch(() => false);
} finally {
  await d2.send('stop').catch(() => {});
  d2.child.disconnect();
  await new Promise((resolve) => proxy2.close(resolve));
  await fake2.close();
}

if (rotatedUploadDropped || validRotatedKeyFalselyRefused || sentStaleMayaKeyAfterAlexConnect || uploadedToMayaAccountWhileAlexConnected || downloadedMayaPrivateFileWhileAlexConnected) {
  console.log(
    `FAILED: sync.connect kept running engine on stale key/session: ` +
    `rotatedUploadDropped=${rotatedUploadDropped} ` +
    `validRotatedKeyFalselyRefused=${validRotatedKeyFalselyRefused} ` +
    `signedInUser=${signedInUser} ` +
    `sentStaleMayaKeyAfterAlexConnect=${sentStaleMayaKeyAfterAlexConnect} ` +
    `uploadedToMayaAccountWhileAlexConnected=${uploadedToMayaAccountWhileAlexConnected} ` +
    `downloadedMayaPrivateFileWhileAlexConnected=${downloadedMayaPrivateFileWhileAlexConnected}`
  );
  process.exitCode = 1;
} else {
  console.log('OK: key rotation and account switch cleanly updated/stopped sessions');
}
