// 08-sync.md step 8.26: one sync session of the real pair. The real htmlclay,
// built with the tag synctest, runs headless on a throwaway config with the
// live-room service started through HTMLCLAY_SERVICE_CMD (a wrapper that guards or
// traces it), and syncs a team folder against L1's fake server. The run covers a
// set-up through `htmlclay sync setup`, twenty downloads into nested folders, an
// upload, a rename, a removal and one conflict, then stops the app so the service
// exits and writes its report.
//
// node scripts/sync-pair.mjs --bin <htmlclay> --service-cmd <wrapper> --roots <dir
// inside home> --local <hyperclay-local checkout> --config-base <empty dir>
// Prints PAIR RUN OK on success, and exits 1 naming the step that failed.
import { spawn } from 'node:child_process';
import { randomBytes } from 'node:crypto';
import { createRequire } from 'node:module';
import { existsSync, mkdirSync, readFileSync, readdirSync, writeFileSync } from 'node:fs';
import path from 'node:path';
import { freePort } from './served-product.mjs';

const args = Object.fromEntries(process.argv.slice(2).reduce((pairs, value, i, all) => {
  if (value.startsWith('--')) pairs.push([value.slice(2), all[i + 1]]);
  return pairs;
}, []));
for (const name of ['bin', 'service-cmd', 'roots', 'local', 'config-base']) {
  if (!args[name]) {
    console.error(`sync-pair: --${name} is required`);
    process.exit(2);
  }
}

const STEP_TIMEOUT_MS = Number(process.env.SYNC_PAIR_STEP_MS) || 120000;
const wait = (ms) => new Promise((resolve) => setTimeout(resolve, ms));
const log = (line) => console.log(`${new Date().toISOString().slice(11, 19)} ${line}`);

async function until(what, check, within = STEP_TIMEOUT_MS) {
  const deadline = Date.now() + within;
  for (;;) {
    if (await check()) return;
    if (Date.now() > deadline) throw new Error(`timed out waiting for ${what}`);
    await wait(200);
  }
}

const KEY = `hcsk_${randomBytes(32).toString('hex')}`;
const FOLDERS = [
  { id: 1, type: 'folder', name: 'a', parentId: 0 },
  { id: 2, type: 'folder', name: 'b', parentId: 1 },
  { id: 3, type: 'folder', name: 'c', parentId: 2 },
];
const PARENTS = [0, 1, 2, 3];
const DIRS = { 0: '', 1: 'a', 2: 'a/b', 3: 'a/b/c' };
const DOCS = Array.from({ length: 20 }, (_, i) => ({
  id: 10 + i, type: 'site', name: `doc-${i}.html`, parentId: PARENTS[i % 4], content: `<p>document ${i}</p>`,
}));
const relOf = (doc) => (DIRS[doc.parentId] ? `${DIRS[doc.parentId]}/${doc.name}` : doc.name);

const require = createRequire(import.meta.url);
const { startFakeSyncHttp } = require(path.join(args.local, 'trial', 'fake-sync-http.cjs'));
const fake = await startFakeSyncHttp({
  nodes: [...FOLDERS, ...DOCS],
  key: KEY,
  actor: { id: 12, username: 'maya', person: { id: 12, name: 'Maya' } },
  accounts: [
    { id: 12, kind: 'personal', username: 'maya', displayName: 'maya', role: 'owner', syncBase: '/_/sync', lifecycle: 'active', sync: { enabled: true, reason: null } },
    { id: 40, kind: 'team', username: 'studio', displayName: 'Studio', role: 'editor', syncBase: '/_/sync', lifecycle: 'active', sync: { enabled: true, reason: null } },
  ],
});

const configBase = args['config-base'];
const configDir = path.join(configBase, 'htmlclay');
mkdirSync(configDir, { recursive: true });
const port = await freePort();
writeFileSync(path.join(configDir, 'config.json'), `${JSON.stringify({ startOnLogin: false, port, updates: 'off', servedFolders: [] }, null, 2)}\n`);
const env = {
  ...process.env,
  HTMLCLAY_CONFIG_BASE: configBase,
  HTMLCLAY_SYNC_SERVER: fake.url,
  HTMLCLAY_SECRET_STORE: 'file',
  HTMLCLAY_SERVICE_CMD: args['service-cmd'],
};

const appLog = path.join(configBase, 'app.log');
const app = spawn(args.bin, ['--no-tray'], { env, stdio: ['ignore', 'pipe', 'pipe'] });
const appLines = [];
for (const stream of [app.stdout, app.stderr]) {
  stream.setEncoding('utf8');
  stream.on('data', (chunk) => appLines.push(chunk));
}
const appExit = new Promise((resolve) => app.once('exit', (code, signal) => resolve({ code, signal })));

function cli(argv, stdin = '') {
  return new Promise((resolve) => {
    const child = spawn(args.bin, argv, { env, stdio: ['pipe', 'pipe', 'pipe'] });
    let out = '';
    child.stdout.on('data', (chunk) => { out += chunk; });
    child.stderr.on('data', (chunk) => { out += chunk; });
    child.on('exit', (code) => resolve({ code, out: out.trim() }));
    child.stdin.end(stdin);
  });
}

async function must(what, argv, stdin) {
  const answer = await cli(argv, stdin);
  log(`${what}: exit ${answer.code} ${answer.out.split('\n').join(' / ')}`);
  if (answer.code !== 0) throw new Error(`${what} exited ${answer.code}: ${answer.out}`);
  return answer;
}

const folder = path.join(args.roots, 'Studio');
const read = (rel) => {
  try {
    return readFileSync(path.join(folder, rel), 'utf8');
  } catch {
    return null;
  }
};

let failed = null;
try {
  // The app answers on its single-instance socket once it is up; before that the
  // command line exits 2 (not running).
  let connected = null;
  await until('the app to answer', async () => {
    connected = await cli(['sync', 'connect', 'maya'], `${KEY}\n`);
    return connected.code !== 2;
  }, 30000);
  log(`connect: exit ${connected.code} ${connected.out}`);
  if (connected.code !== 0) throw new Error(`connect exited ${connected.code}: ${connected.out}`);

  const preview = await must('setup', ['sync', 'setup', 'studio', folder], 'y\n');
  if (!/20 files/.test(preview.out)) throw new Error(`the preview did not count 20 files: ${preview.out}`);

  await until('twenty downloads', () => DOCS.every((doc) => read(relOf(doc)) === doc.content));
  log('downloaded 20 documents into a, a/b and a/b/c');

  writeFileSync(path.join(folder, 'a', 'upload.html'), '<p>made on this computer</p>');
  await until('the upload', () => fake.server.paths().includes('a/upload.html'));
  log('uploaded a/upload.html');

  const renamed = DOCS[0];
  await fake.server.renameNode(null, renamed.id, 'renamed.html');
  fake.push({ type: 'node-renamed', nodeId: renamed.id });
  await until('the rename', () => read('renamed.html') === renamed.content && read(relOf(renamed)) === null);
  log(`renamed ${relOf(renamed)} to renamed.html`);

  const removed = DOCS[5];
  await fake.server.deleteNode(null, removed.id);
  fake.push({ type: 'node-deleted', nodeId: removed.id });
  await until('the removal', () => read(relOf(removed)) === null);
  log(`removed ${relOf(removed)}`);

  const both = DOCS[2];
  await must('pause', ['sync', 'pause']);
  writeFileSync(path.join(folder, relOf(both)), '<p>edited on this computer</p>');
  await fake.server.putNodeContent(null, both.id, '<p>edited on the server</p>');
  await must('resume', ['sync', 'resume']);
  const metaRoot = path.join(configDir, 'service', 'sync-meta', 'v2');
  const heldConflicts = () => (existsSync(metaRoot) ? readdirSync(metaRoot) : []).reduce((count, session) => {
    try {
      return count + Object.keys(JSON.parse(readFileSync(path.join(metaRoot, session, 'conflicts.json'), 'utf8'))).length;
    } catch {
      return count;
    }
  }, 0);
  await until('the conflict', () => heldConflicts() > 0);
  log(`held ${heldConflicts()} conflict on ${relOf(both)}`);
} catch (error) {
  failed = error;
}

app.kill('SIGTERM');
const gone = await Promise.race([appExit, wait(20000).then(() => null)]);
if (!gone) {
  app.kill('SIGKILL');
  await appExit;
}
writeFileSync(appLog, appLines.join(''));
await fake.close();
if (failed) {
  console.log(`PAIR RUN FAILED: ${failed.message}`);
  console.log(appLines.join('').trimEnd().split('\n').slice(-30).join('\n'));
  process.exit(1);
}
log(`the app exited ${gone ? (gone.signal ?? gone.code) : 'after SIGKILL'}`);
console.log('PAIR RUN OK');
